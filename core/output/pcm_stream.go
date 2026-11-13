package output

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/navidrome/navidrome/core/ffmpeg"
	"github.com/navidrome/navidrome/core/musicengine"
	"github.com/navidrome/navidrome/core/stream"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
)

const (
	wavHeaderSize   = 44
	minPCMBurstSize = 512 * 1024      // 512 KB minimum initial burst
	maxPCMBurstSize = 4 * 1024 * 1024 // 4 MB cap for high-res 192kHz/24bit streams
)

func calculatePCMBurstBytes(byteRate int) int {
	burst := byteRate * 3 // ~3 seconds pre-buffer to prime HQPE DSD256 modulator
	if burst < minPCMBurstSize {
		return minPCMBurstSize
	}
	if burst > maxPCMBurstSize {
		return maxPCMBurstSize
	}
	return burst
}

// IsDSDMedia reports whether the track is native DSD (.dsf / .dff), which must always remain raw passthrough.
func IsDSDMedia(suffix, path string) bool {
	s := strings.ToLower(strings.TrimPrefix(suffix, "."))
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	return s == "dsf" || s == "dff" || ext == "dsf" || ext == "dff"
}

// PCMFormat describes the exact uncompressed PCM stream layout and precomputed WAV sizes.
type PCMFormat struct {
	SampleRate int
	Channels   int
	BitDepth   int
	BlockAlign int
	ByteRate   int
	DataBytes  int64
	TotalBytes int64
}

// ResolvePCMFormat determines native PCM parameters without unnecessary resampling and precomputes exact WAV sizes.
func ResolvePCMFormat(ctx context.Context, mf *model.MediaFile, localFilePath string) PCMFormat {
	sr := mf.SampleRate
	ch := mf.Channels
	bd := 0
	if mf.BitDepth != nil {
		bd = *mf.BitDepth
	}

	if localFilePath != "" && (sr <= 0 || ch <= 0 || bd <= 0) {
		if probe, err := ffmpeg.New().ProbeAudioStream(ctx, localFilePath); err == nil && probe != nil {
			if sr <= 0 && probe.SampleRate > 0 {
				sr = probe.SampleRate
			}
			if ch <= 0 && probe.Channels > 0 {
				ch = probe.Channels
			}
			if bd <= 0 && probe.BitDepth > 0 {
				bd = probe.BitDepth
			}
		}
	}

	if sr <= 0 {
		sr = 44100
	}
	if ch <= 0 {
		ch = 2
	}
	if bd != 16 && bd != 24 && bd != 32 {
		bd = 16
	}

	blockAlign := ch * (bd / 8)
	byteRate := sr * blockAlign
	dur := float64(mf.Duration)
	if dur <= 0 {
		dur = 240.0
	}
	numFrames := int64(dur * float64(sr))
	if numFrames <= 0 {
		numFrames = int64(sr)
	}
	dataBytes := numFrames * int64(blockAlign)

	return PCMFormat{
		SampleRate: sr,
		Channels:   ch,
		BitDepth:   bd,
		BlockAlign: blockAlign,
		ByteRate:   byteRate,
		DataBytes:  dataBytes,
		TotalBytes: wavHeaderSize + dataBytes,
	}
}

// BuildWAVHeader constructs a standard 44-byte RIFF/WAVE header with exact precomputed chunk lengths.
func BuildWAVHeader(f PCMFormat) []byte {
	hdr := make([]byte, wavHeaderSize)
	const maxUint32 = int64(0xFFFFFFFF)

	riffSize := int64(36) + f.DataBytes
	if riffSize > maxUint32 {
		riffSize = maxUint32
	}
	dataSize := f.DataBytes
	if dataSize > maxUint32 {
		dataSize = maxUint32
	}

	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(riffSize))
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16) // Subchunk1Size for PCM
	binary.LittleEndian.PutUint16(hdr[20:22], 1)  // AudioFormat = 1 (Linear PCM)
	binary.LittleEndian.PutUint16(hdr[22:24], uint16(f.Channels))
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(f.SampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(f.ByteRate))
	binary.LittleEndian.PutUint16(hdr[32:34], uint16(f.BlockAlign))
	binary.LittleEndian.PutUint16(hdr[34:36], uint16(f.BitDepth))
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], uint32(dataSize))
	return hdr
}

type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.buf.Len() < c.max {
		remain := c.max - c.buf.Len()
		if len(p) > remain {
			_, _ = c.buf.Write(p[:remain])
		} else {
			_, _ = c.buf.Write(p)
		}
	}
	return len(p), nil
}

// ServePCMStream transcodes local or online audio into an uncompressed constant-bitrate WAV stream
// with exact Content-Length, HTTP Range support, and 1.06x Roon-like pacing.
func ServePCMStream(w http.ResponseWriter, r *http.Request, mf *model.MediaFile, ds model.DataStore) {
	isOnline := strings.HasPrefix(mf.Path, "online://")
	var localPath string

	if isOnline {
		if cachedPath, ok := musicengine.GetCacheManager().GetCachedFilePath(mf.ID); ok {
			localPath = cachedPath
		}
	} else {
		localPath = mf.AbsolutePath()
	}

	pcmFmt := ResolvePCMFormat(r.Context(), mf, localPath)

	// Parse optional HTTP Range header
	start := int64(0)
	end := pcmFmt.TotalBytes - 1
	isRange := false

	if rangeHdr := r.Header.Get("Range"); strings.HasPrefix(rangeHdr, "bytes=") {
		parts := strings.Split(strings.TrimPrefix(rangeHdr, "bytes="), "-")
		if len(parts) >= 1 && parts[0] != "" {
			if s, err := strconv.ParseInt(parts[0], 10, 64); err == nil && s >= 0 {
				start = s
				isRange = true
			}
		}
		if len(parts) >= 2 && parts[1] != "" {
			if e, err := strconv.ParseInt(parts[1], 10, 64); err == nil && e >= start {
				end = e
			}
		}
	}

	if start >= pcmFmt.TotalBytes {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", pcmFmt.TotalBytes))
		http.Error(w, "Requested Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if end >= pcmFmt.TotalBytes {
		end = pcmFmt.TotalBytes - 1
	}
	contentLen := end - start + 1

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(contentLen, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if mf.Duration > 0 {
		w.Header().Set("X-Content-Duration", fmt.Sprintf("%.3f", mf.Duration))
	}
	if isRange {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, pcmFmt.TotalBytes))
	}

	if r.Method == http.MethodHead {
		if isRange {
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		return
	}

	remaining := contentLen
	currPos := start
	pacingRate := stream.CalculatePCMPacingRate(pcmFmt.SampleRate, pcmFmt.Channels, pcmFmt.BitDepth)
	burstBytes := calculatePCMBurstBytes(pcmFmt.ByteRate)

	log.Info(r.Context(), "PCM Stream: Serving WAV stream",
		"id", mf.ID,
		"method", r.Method,
		"range", r.Header.Get("Range"),
		"sr", pcmFmt.SampleRate,
		"ch", pcmFmt.Channels,
		"bd", pcmFmt.BitDepth,
		"contentLen", contentLen,
		"burstKB", burstBytes/1024,
	)

	// Fast path: client only requested bytes inside the 44-byte WAV header
	if currPos+remaining <= wavHeaderSize {
		if isRange {
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		pw := stream.NewPacedResponseWriter(r.Context(), w, pacingRate, burstBytes)
		hdr := BuildWAVHeader(pcmFmt)
		_, _ = pw.Write(hdr[currPos : currPos+remaining])
		return
	}

	// Prepare online relay only for GET requests that miss the disk cache
	var relay *musicengine.StreamRelay
	if isOnline && localPath == "" {
		source, songID, quality, ok := musicengine.ParseProtocolPath(mf.Path)
		if !ok {
			source, songID, ok = musicengine.ParseUniversalID(mf.ID)
		}
		if !ok {
			http.Error(w, "invalid online track path", http.StatusBadRequest)
			return
		}
		engine := musicengine.GetEngine(ds)
		resolved, err := engine.ResolveStreamURL(r.Context(), source, songID, quality)
		if err != nil {
			log.Error(r.Context(), "PCM Stream: Failed to resolve online stream URL", "id", mf.ID, err)
			http.Error(w, "stream resolve error: "+err.Error(), http.StatusBadGateway)
			return
		}
		relay = musicengine.NewStreamRelay(resolved, 0, int64(mf.Size), mf.ID)
		defer relay.Close()
	}

	ffmpegPath, err := ffmpeg.New().CmdPath()
	if err != nil {
		log.Error(r.Context(), "PCM Stream: ffmpeg unavailable", err)
		http.Error(w, "ffmpeg unavailable", http.StatusInternalServerError)
		return
	}

	cmdCtx, cancelCmd := context.WithCancel(r.Context())
	defer cancelCmd()

	// For uncached online tracks, expose relay on a temporary loopback HTTP server so ffmpeg can HTTP-Range seek (needed for M4A/MP4 moov atoms)
	inputTarget := localPath
	if relay != nil {
		ln, lerr := net.Listen("tcp", "127.0.0.1:0")
		if lerr != nil {
			log.Error(r.Context(), "PCM Stream: Failed to create loopback listener", lerr)
			http.Error(w, "loopback listener error", http.StatusInternalServerError)
			return
		}
		srv := &http.Server{Handler: relay}
		defer func() {
			_ = srv.Close()
			_ = ln.Close()
		}()
		go func() {
			<-cmdCtx.Done()
			_ = srv.Close()
			_ = ln.Close()
		}()
		go func() {
			_ = srv.Serve(ln)
		}()
		inputTarget = fmt.Sprintf("http://%s/stream", ln.Addr().String())
	}

	pcmStartOffset := int64(0)
	if currPos > wavHeaderSize {
		pcmStartOffset = currPos - wavHeaderSize
	}
	alignedBytes := (pcmStartOffset / int64(pcmFmt.BlockAlign)) * int64(pcmFmt.BlockAlign)
	skipBytes := pcmStartOffset - alignedBytes
	seekSec := float64(alignedBytes) / float64(pcmFmt.ByteRate)

	codec := "pcm_s16le"
	outFmt := "s16le"
	switch pcmFmt.BitDepth {
	case 24:
		codec = "pcm_s24le"
		outFmt = "s24le"
	case 32:
		codec = "pcm_s32le"
		outFmt = "s32le"
	}

	args := []string{"-v", "error", "-nostdin"}
	if seekSec > 0.05 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
	}
	args = append(args,
		"-i", inputTarget,
		"-vn", "-sn", "-dn",
		"-ac", strconv.Itoa(pcmFmt.Channels),
		"-ar", strconv.Itoa(pcmFmt.SampleRate),
		"-acodec", codec,
		"-f", outFmt,
		"pipe:1",
	)

	cmd := exec.CommandContext(cmdCtx, ffmpegPath, args...)
	stderrBuf := &cappedBuffer{max: 4096}
	cmd.Stderr = stderrBuf
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error(r.Context(), "PCM Stream: Failed to attach stdout pipe", err)
		http.Error(w, "ffmpeg stdout pipe error", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Error(r.Context(), "PCM Stream: Failed to start ffmpeg", err)
		http.Error(w, "ffmpeg start error", http.StatusInternalServerError)
		return
	}

	defer func() {
		cancelCmd()
		_ = stdout.Close()
		_ = cmd.Wait()
	}()

	if skipBytes > 0 {
		if _, err := io.CopyN(io.Discard, stdout, skipBytes); err != nil {
			log.Error(r.Context(), "PCM Stream: Failed skipping PCM alignment bytes", "id", mf.ID, "stderr", strings.TrimSpace(stderrBuf.buf.String()), err)
			http.Error(w, "ffmpeg decode failed", http.StatusBadGateway)
			return
		}
	}

	// Pre-read initial PCM chunk BEFORE sending HTTP 200/206 + WAV header so decode errors are never masked as silent streams
	firstChunk := make([]byte, 4*1024)
	firstN, firstErr := stdout.Read(firstChunk)
	if firstN == 0 {
		errDetail := strings.TrimSpace(stderrBuf.buf.String())
		log.Error(r.Context(), "PCM Stream: ffmpeg produced 0 PCM bytes", "id", mf.ID, "input", inputTarget, "stderr", errDetail, firstErr)
		http.Error(w, "ffmpeg decoded 0 bytes: "+errDetail, http.StatusBadGateway)
		return
	}

	if isRange {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	pw := stream.NewPacedResponseWriter(r.Context(), w, pacingRate, burstBytes)

	// 1. Write WAV header slice if requested range starts inside header
	if currPos < wavHeaderSize {
		hdr := BuildWAVHeader(pcmFmt)
		hdrEnd := int64(wavHeaderSize)
		if currPos+remaining < hdrEnd {
			hdrEnd = currPos + remaining
		}
		n, werr := pw.Write(hdr[currPos:hdrEnd])
		remaining -= int64(n)
		currPos += int64(n)
		if werr != nil || remaining <= 0 {
			return
		}
	}

	// 2. Flush pre-read initial PCM chunk
	toWriteFirst := int64(firstN)
	if toWriteFirst > remaining {
		toWriteFirst = remaining
	}
	n, werr := pw.Write(firstChunk[:toWriteFirst])
	remaining -= int64(n)
	if werr != nil || remaining <= 0 {
		return
	}

	// 3. Stream remaining PCM audio at 1.12x constant rate
	buf := make([]byte, 4*1024)
	written, copyErr := io.CopyBuffer(pw, io.LimitReader(stdout, remaining), buf)
	remaining -= written

	log.Info(r.Context(), "PCM Stream: Finished WAV stream",
		"id", mf.ID,
		"written", int64(firstN)+written,
		"remaining", remaining,
		"ctxErr", r.Context().Err(),
		"copyErr", copyErr,
		"stderr", strings.TrimSpace(stderrBuf.buf.String()),
	)

	// Pad minor rounding gap (e.g. duration rounded up by <5s) with silence so Content-Length matches exactly
	if remaining > 0 && remaining <= int64(pcmFmt.ByteRate)*5 && r.Context().Err() == nil {
		zeroBuf := make([]byte, 4*1024)
		for remaining > 0 && r.Context().Err() == nil {
			toWrite := int64(len(zeroBuf))
			if remaining < toWrite {
				toWrite = remaining
			}
			nw, werr := pw.Write(zeroBuf[:toWrite])
			remaining -= int64(nw)
			if werr != nil {
				break
			}
		}
	}
}
