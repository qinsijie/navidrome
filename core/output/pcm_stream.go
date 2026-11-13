package output

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
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

const wavHeaderSize = 44

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

	if isRange {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	pacingRate := stream.CalculatePCMPacingRate(pcmFmt.SampleRate, pcmFmt.Channels, pcmFmt.BitDepth)
	pw := stream.NewPacedResponseWriter(r.Context(), w, pacingRate, 128*1024)

	remaining := contentLen
	currPos := start

	// 1. Serve WAV header (or requested slice of it)
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

	// 2. Decode audio into raw PCM via ffmpeg (context-bound to avoid zombie processes)
	pcmOffset := currPos - wavHeaderSize
	alignedBytes := (pcmOffset / int64(pcmFmt.BlockAlign)) * int64(pcmFmt.BlockAlign)
	skipBytes := pcmOffset - alignedBytes
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

	cmdCtx, cancelCmd := context.WithCancel(r.Context())
	defer cancelCmd()

	args := []string{"-v", "error", "-nostdin"}
	if localPath != "" {
		if seekSec > 0.05 {
			args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
		}
		args = append(args, "-i", localPath)
	} else {
		args = append(args, "-i", "pipe:0")
		if seekSec > 0.05 {
			args = append(args, "-ss", fmt.Sprintf("%.3f", seekSec))
		}
	}
	args = append(args,
		"-vn", "-sn", "-dn",
		"-ac", strconv.Itoa(pcmFmt.Channels),
		"-ar", strconv.Itoa(pcmFmt.SampleRate),
		"-acodec", codec,
		"-f", outFmt,
		"pipe:1",
	)

	cmd := exec.CommandContext(cmdCtx, ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error(r.Context(), "PCM Stream: Failed to attach stdout pipe", err)
		return
	}

	var pipeReader *io.PipeReader
	var pipeWriter *io.PipeWriter
	if relay != nil {
		pipeReader, pipeWriter = io.Pipe()
		cmd.Stdin = pipeReader
		go func() {
			defer pipeWriter.Close()
			if rerr := relay.StreamTo(cmdCtx, pipeWriter); rerr != nil && cmdCtx.Err() == nil {
				log.Warn(cmdCtx, "PCM Stream: Online relay stream interrupted", "id", mf.ID, rerr)
			}
		}()
	}

	if err := cmd.Start(); err != nil {
		if pipeReader != nil {
			_ = pipeReader.Close()
		}
		if pipeWriter != nil {
			_ = pipeWriter.Close()
		}
		log.Error(r.Context(), "PCM Stream: Failed to start ffmpeg", err)
		return
	}

	defer func() {
		cancelCmd()
		if pipeReader != nil {
			_ = pipeReader.Close()
		}
		if pipeWriter != nil {
			_ = pipeWriter.Close()
		}
		_ = stdout.Close()
		_ = cmd.Wait()
	}()

	if skipBytes > 0 {
		if _, err := io.CopyN(io.Discard, stdout, skipBytes); err != nil {
			return
		}
	}

	buf := make([]byte, 4*1024)
	written, _ := io.CopyBuffer(pw, io.LimitReader(stdout, remaining), buf)
	remaining -= written

	// Pad minor rounding gap (e.g. duration rounded up by <5s) with silence so Content-Length matches exactly
	if remaining > 0 && remaining <= int64(pcmFmt.ByteRate)*5 && r.Context().Err() == nil {
		zeroBuf := make([]byte, 4*1024)
		for remaining > 0 && r.Context().Err() == nil {
			toWrite := int64(len(zeroBuf))
			if remaining < toWrite {
				toWrite = remaining
			}
			n, werr := pw.Write(zeroBuf[:toWrite])
			remaining -= int64(n)
			if werr != nil {
				break
			}
		}
	}
}
