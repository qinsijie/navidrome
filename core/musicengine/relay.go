package musicengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/navidrome/navidrome/log"
)

var hopByHopHeaders = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailers":            true,
	"transfer-encoding":   true,
	"upgrade":             true,
	"host":                true,
}

// StreamRelay coordinates pulling audio from an upstream CDN into a SlabRingBuffer
// and serving it to downstream clients (HQPlayer, web player, Subsonic)
type StreamRelay struct {
	stream      *ResolvedStream
	buffer      *SlabRingBuffer
	client      *http.Client
	cancelPull  context.CancelFunc
	pullLock    sync.Mutex
	totalSize   int64
	currOffset  int64
	isChunked   bool
	estimatedLen int64
}

// NewStreamRelay creates a new relay instance
func NewStreamRelay(stream *ResolvedStream, initialOffset int64, defaultEstimatedLen int64) *StreamRelay {
	return &StreamRelay{
		stream:       stream,
		buffer:       NewSlabRingBuffer(initialOffset),
		client:       &http.Client{Timeout: 60 * time.Second},
		totalSize:    0,
		currOffset:   initialOffset,
		estimatedLen: defaultEstimatedLen,
	}
}

// StartPull begins or restarts fetching from upstream CDN at the specified offset
func (r *StreamRelay) StartPull(parentCtx context.Context, startOffset int64) error {
	r.pullLock.Lock()
	defer r.pullLock.Unlock()

	// Cancel any active upstream pull
	if r.cancelPull != nil {
		r.cancelPull()
	}

	ctx, cancel := context.WithCancel(parentCtx)
	r.cancelPull = cancel
	r.currOffset = startOffset

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.stream.URL, nil)
	if err != nil {
		return fmt.Errorf("creating upstream request: %w", err)
	}

	// 1. Inject anti-leech context headers
	for k, v := range r.stream.Headers {
		if !hopByHopHeaders[strings.ToLower(k)] {
			req.Header.Set(k, v)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	}

	// 2. Add Range header (prompts upstream CDN to provide Content-Range with total length)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("connecting to upstream CDN: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return fmt.Errorf("upstream CDN returned error status: %d", resp.StatusCode)
	}

	// 3. Extract length and Content-Range
	contentRange := resp.Header.Get("Content-Range")
	if contentRange != "" {
		// E.g. "bytes 0-1000/2849182"
		if slashIdx := strings.LastIndex(contentRange, "/"); slashIdx != -1 {
			totalStr := contentRange[slashIdx+1:]
			if total, parseErr := strconv.ParseInt(totalStr, 10, 64); parseErr == nil {
				r.totalSize = total
			}
		}
	} else if resp.ContentLength > 0 {
		r.totalSize = startOffset + resp.ContentLength
	} else {
		r.isChunked = true
	}

	// 4. Background pull into SlabRingBuffer
	go func(body io.ReadCloser) {
		defer body.Close()
		buf := make([]byte, SlabSize)
		for {
			select {
			case <-ctx.Done():
				r.buffer.MarkDone(ctx.Err())
				return
			default:
			}

			n, readErr := body.Read(buf)
			if n > 0 {
				_, _ = r.buffer.Write(buf[:n])
			}
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					r.buffer.MarkDone(nil)
				} else {
					r.buffer.MarkDone(readErr)
				}
				return
			}
		}
	}(resp.Body)

	return nil
}

// ServeHTTP handles downstream HTTP request with Range support and Seek detection
func (r *StreamRelay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Parse downstream Range header
	rangeHeader := req.Header.Get("Range")
	var reqStart, reqEnd int64 = 0, -1
	isRange := false

	if strings.HasPrefix(rangeHeader, "bytes=") {
		parts := strings.Split(strings.TrimPrefix(rangeHeader, "bytes="), "-")
		if len(parts) >= 1 && parts[0] != "" {
			if s, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
				reqStart = s
				isRange = true
			}
		}
		if len(parts) >= 2 && parts[1] != "" {
			if e, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				reqEnd = e
			}
		}
	}

	// Check if this is an out-of-window discontinuous Seek
	r.pullLock.Lock()
	pullStartNeeded := false
	if r.cancelPull == nil || reqStart < r.currOffset || reqStart > (r.buffer.writeCursor+MaxForwardSlop) {
		// Discontinuous seek detected: re-anchor puller
		pullStartNeeded = true
		r.buffer = NewSlabRingBuffer(reqStart)
	}
	r.pullLock.Unlock()

	if pullStartNeeded {
		log.Debug(ctx, "MusicEngine: Discontinuous Seek detected. Re-anchoring upstream CDN pull", "reqStart", reqStart)
		if err := r.StartPull(ctx, reqStart); err != nil {
			log.Error(ctx, "MusicEngine: Failed to start upstream pull on seek", err)
			http.Error(w, "Failed to stream audio", http.StatusBadGateway)
			return
		}
	}

	// Wait briefly for first chunk to sniff format
	time.Sleep(50 * time.Millisecond)
	sniff := r.buffer.SniffedResult()

	// Determine total content length
	effectiveTotal := r.totalSize
	if effectiveTotal <= 0 {
		if sniff.DurationSec > 0 && sniff.BitRate > 0 {
			effectiveTotal = int64(sniff.DurationSec * float64(sniff.BitRate*1000/8))
		} else if r.estimatedLen > 0 {
			effectiveTotal = r.estimatedLen
		} else {
			effectiveTotal = 30 * 1024 * 1024 // 30 MB fallback
		}
	}

	if reqEnd == -1 || reqEnd >= effectiveTotal {
		reqEnd = effectiveTotal - 1
	}

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", sniff.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	var status int
	var streamLen int64

	if isRange {
		status = http.StatusPartialContent
		streamLen = reqEnd - reqStart + 1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", reqStart, reqEnd, effectiveTotal))
		w.Header().Set("Content-Length", strconv.FormatInt(streamLen, 10))
	} else {
		status = http.StatusOK
		streamLen = effectiveTotal
		w.Header().Set("Content-Length", strconv.FormatInt(streamLen, 10))
	}

	w.WriteHeader(status)

	// Stream to downstream client in 64KB chunks
	buf := make([]byte, SlabSize)
	var written int64 = 0
	currOff := reqStart

	for written < streamLen {
		select {
		case <-ctx.Done():
			return
		default:
		}

		toRead := int64(len(buf))
		if remaining := streamLen - written; toRead > remaining {
			toRead = remaining
		}

		n, err := r.buffer.ReadAt(buf[:toRead], currOff)
		if n > 0 {
			wn, werr := w.Write(buf[:n])
			written += int64(wn)
			currOff += int64(wn)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			if werr != nil {
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			log.Warn("MusicEngine: Downstream stream read error", "err", err)
			break
		}
	}
}

// Close terminates the relay and cleans up memory
func (r *StreamRelay) Close() {
	r.pullLock.Lock()
	if r.cancelPull != nil {
		r.cancelPull()
	}
	r.pullLock.Unlock()
	if r.buffer != nil {
		_ = r.buffer.Close()
	}
}
