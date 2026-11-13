package stream

import (
	"context"
	"io"
	"net/http"
	"strings"

	"golang.org/x/time/rate"
)

// PacedResponseWriter wraps an http.ResponseWriter with a Token Bucket rate limiter.
// It delivers an initial burst (e.g. 384KB) for instant header parsing and decoder priming,
// followed by smooth, paced delivery at targetBytesPerSec to eliminate TCP burst spikes.
type PacedResponseWriter struct {
	http.ResponseWriter
	ctx       context.Context
	limiter   *rate.Limiter
	flusher   http.Flusher
	chunkSize int
}

// CalculateAudioPacingRate computes the target streaming speed in bytes/sec with 1.30x headroom.
func CalculateAudioPacingRate(fileSize int64, durationSec float32, suffix string) float64 {
	if fileSize > 0 && durationSec > 5 {
		nominal := float64(fileSize) / float64(durationSec)
		// 1.30x headroom provides a 30% safety margin above real-time decoding speed for raw passthrough (DLNA/HQPE raw)
		paced := nominal * 1.30
		if paced < 180*1024 {
			paced = 180 * 1024
		}
		return paced
	}
	ext := strings.ToLower(strings.TrimPrefix(suffix, "."))
	switch ext {
	case "dsf", "dff":
		return 750 * 1024 // ~750 KB/s for DSD64
	case "wav", "wave":
		return 350 * 1024 // ~350 KB/s for uncompressed WAV
	case "mp3", "aac", "ogg":
		return 80 * 1024 // ~80 KB/s for lossy
	default:
		return 220 * 1024 // ~220 KB/s for standard 16/44.1 FLAC (Roon baseline)
	}
}

// CalculatePCMPacingRate computes the steady-state 1.12x byte rate for uncompressed PCM/WAV streams.
func CalculatePCMPacingRate(sampleRate, channels, bitDepth int) float64 {
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	if channels <= 0 {
		channels = 2
	}
	if bitDepth <= 0 {
		bitDepth = 16
	}
	byteRate := sampleRate * channels * (bitDepth / 8)
	if byteRate <= 0 {
		return 176400 * 1.12
	}
	return float64(byteRate) * 1.12
}

// NewPacedResponseWriter constructs a token-bucket rate pacer wrapping an http.ResponseWriter.
func NewPacedResponseWriter(ctx context.Context, w http.ResponseWriter, targetBytesPerSec float64, burstBytes int) *PacedResponseWriter {
	if targetBytesPerSec <= 0 {
		targetBytesPerSec = 220 * 1024
	}
	if burstBytes <= 0 {
		burstBytes = 512 * 1024 // 512 KB default initial burst
	}
	flusher, _ := w.(http.Flusher)
	return &PacedResponseWriter{
		ResponseWriter: w,
		ctx:            ctx,
		limiter:        rate.NewLimiter(rate.Limit(targetBytesPerSec), burstBytes),
		flusher:        flusher,
		chunkSize:      8 * 1024, // 8 KB chunks for smooth Roon-like constant-bitrate streaming with low syscall overhead
	}
}

func (pw *PacedResponseWriter) Write(p []byte) (int, error) {
	totalWritten := 0
	for len(p) > 0 {
		select {
		case <-pw.ctx.Done():
			return totalWritten, pw.ctx.Err()
		default:
		}

		toWrite := len(p)
		if toWrite > pw.chunkSize {
			toWrite = pw.chunkSize
		}

		if err := pw.limiter.WaitN(pw.ctx, toWrite); err != nil {
			return totalWritten, err
		}

		n, err := pw.ResponseWriter.Write(p[:toWrite])
		totalWritten += n
		if pw.flusher != nil {
			pw.flusher.Flush()
		}
		if err != nil {
			return totalWritten, err
		}
		p = p[n:]
	}
	return totalWritten, nil
}

func (pw *PacedResponseWriter) Flush() {
	if pw.flusher != nil {
		pw.flusher.Flush()
	}
}

// ReadFrom implements io.ReaderFrom to pace streams handled by io.Copy or http.ServeContent.
func (pw *PacedResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, pw.chunkSize)
	var total int64
	for {
		select {
		case <-pw.ctx.Done():
			return total, pw.ctx.Err()
		default:
		}

		n, rerr := r.Read(buf)
		if n > 0 {
			if err := pw.limiter.WaitN(pw.ctx, n); err != nil {
				return total, err
			}
			wn, werr := pw.ResponseWriter.Write(buf[:n])
			total += int64(wn)
			if pw.flusher != nil {
				pw.flusher.Flush()
			}
			if werr != nil {
				return total, werr
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				return total, nil
			}
			return total, rerr
		}
	}
}

