package musicengine

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/log"
)

const (
	SlabSize            = 64 * 1024        // 64 KB per chunk
	DefaultMaxSlabs     = 512              // 32 MB max in-memory window
	MaxForwardSlop      = 2 * 1024 * 1024  // 2 MB forward seek tolerance
	ConditionWaitTime   = 2500 * time.Millisecond // 2.5s safe timeout (below HQPE 3-5s read timeout)
)

var slabPool = sync.Pool{
	New: func() any {
		b := make([]byte, SlabSize)
		return &b
	},
}

// SniffResult holds metadata detected from the initial audio chunk
type SniffResult struct {
	Format      string
	ContentType string
	BitRate     int
	SampleRate  int
	BitDepth    int
	Channels    int
	DurationSec float64
}

// SniffAudioFormat inspects the first chunk for magic numbers and extracts format details
func SniffAudioFormat(data []byte) SniffResult {
	res := SniffResult{
		Format:      "flac",
		ContentType: "audio/flac",
		BitRate:     960,
		SampleRate:  44100,
		BitDepth:    16,
		Channels:    2,
	}

	if len(data) < 4 {
		return res
	}

	// 1. FLAC magic number: "fLaC"
	if bytes.HasPrefix(data, []byte("fLaC")) {
		res.Format = "flac"
		res.ContentType = "audio/flac"
		if len(data) >= 38 {
			// STREAMINFO block
			// Sample rate (20 bits), channels (3 bits), bits per sample (5 bits), total samples (36 bits)
			sr := (uint32(data[18]) << 12) | (uint32(data[19]) << 4) | (uint32(data[20]) >> 4)
			channels := ((data[20] >> 1) & 0x07) + 1
			bps := (((data[20] & 0x01) << 4) | (data[21] >> 4)) + 1
			totalSamples := (uint64(data[21]&0x0F) << 32) | (uint64(data[22]) << 24) |
				(uint64(data[23]) << 16) | (uint64(data[24]) << 8) | uint64(data[25])

			if sr > 0 {
				res.SampleRate = int(sr)
				res.Channels = int(channels)
				res.BitDepth = int(bps)
				if totalSamples > 0 {
					res.DurationSec = float64(totalSamples) / float64(sr)
				}
			}
		}
		return res
	}

	// 2. MP3 ID3 header: "ID3"
	if bytes.HasPrefix(data, []byte("ID3")) || (data[0] == 0xFF && (data[1]&0xE0) == 0xE0) {
		res.Format = "mp3"
		res.ContentType = "audio/mpeg"
		res.BitRate = 320
		res.SampleRate = 44100
		res.BitDepth = 16
		res.Channels = 2
		return res
	}

	// 3. Ogg header: "OggS"
	if bytes.HasPrefix(data, []byte("OggS")) {
		res.Format = "ogg"
		res.ContentType = "audio/ogg"
		res.BitRate = 320
		return res
	}

	// 4. AAC ADTS: 0xFFF
	if data[0] == 0xFF && (data[1]&0xF0) == 0xF0 {
		res.Format = "aac"
		res.ContentType = "audio/aac"
		res.BitRate = 256
		return res
	}

	return res
}

// SlabRingBuffer implements a high-performance, bounded sliding window buffer
type SlabRingBuffer struct {
	mu           sync.Mutex
	cond         *sync.Cond
	slabs        [][]byte
	baseOffset   int64 // starting byte offset of slabs[0]
	writeCursor  int64 // absolute total bytes written
	isDone       atomic.Bool
	isClosed     atomic.Bool
	err          error
	sniffed      bool
	sniffResult  SniffResult
	initialChunk []byte
}

// NewSlabRingBuffer initializes a ring buffer
func NewSlabRingBuffer(initialOffset int64) *SlabRingBuffer {
	buf := &SlabRingBuffer{
		baseOffset:  initialOffset,
		writeCursor: initialOffset,
	}
	buf.cond = sync.NewCond(&buf.mu)
	return buf
}

// Write appends bytes to the buffer
func (b *SlabRingBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.isClosed.Load() {
		return 0, io.ErrClosedPipe
	}

	// Capture initial bytes for format sniffing
	if !b.sniffed {
		b.initialChunk = append(b.initialChunk, p...)
		if len(b.initialChunk) >= 64*1024 || len(p) < SlabSize {
			b.sniffResult = SniffAudioFormat(b.initialChunk)
			b.sniffed = true
		}
	}

	remaining := p
	for len(remaining) > 0 {
		var activeSlab []byte
		if len(b.slabs) == 0 || len(b.slabs[len(b.slabs)-1]) == SlabSize {
			slabPtr := slabPool.Get().(*[]byte)
			activeSlab = (*slabPtr)[:0]
			b.slabs = append(b.slabs, activeSlab)
		} else {
			activeSlab = b.slabs[len(b.slabs)-1]
		}

		space := SlabSize - len(activeSlab)
		toWrite := len(remaining)
		if toWrite > space {
			toWrite = space
		}

		activeSlab = append(activeSlab, remaining[:toWrite]...)
		b.slabs[len(b.slabs)-1] = activeSlab
		b.writeCursor += int64(toWrite)
		remaining = remaining[toWrite:]
		n += toWrite
	}

	// Trim oldest slabs if exceeding max window
	for len(b.slabs) > DefaultMaxSlabs {
		old := b.slabs[0]
		b.slabs = b.slabs[1:]
		b.baseOffset += int64(len(old))
		oldPtr := &old
		slabPool.Put(oldPtr)
	}

	b.cond.Broadcast()
	return n, nil
}

// ReadAt reads slice of bytes at the given absolute offset
func (b *SlabRingBuffer) ReadAt(p []byte, off int64) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for {
		if b.isClosed.Load() {
			return 0, io.ErrClosedPipe
		}

		// Available window: [baseOffset, writeCursor]
		if off < b.baseOffset {
			return 0, errors.New("read offset fallen behind sliding window")
		}

		if off < b.writeCursor {
			// Data available to copy
			copied := 0
			currOff := off
			for copied < len(p) && currOff < b.writeCursor {
				slabIdx := (currOff - b.baseOffset) / SlabSize
				slabInnerOff := (currOff - b.baseOffset) % SlabSize

				if int(slabIdx) >= len(b.slabs) {
					break
				}

				slab := b.slabs[slabIdx]
				if int(slabInnerOff) >= len(slab) {
					break
				}

				chunk := copy(p[copied:], slab[slabInnerOff:])
				copied += chunk
				currOff += int64(chunk)
			}
			return copied, nil
		}

		// If upstream reached EOF
		if b.isDone.Load() {
			if b.err != nil {
				return 0, b.err
			}
			return 0, io.EOF
		}

		// Wait for more data with 2.5s safe timeout
		waitCh := make(chan struct{})
		go func() {
			b.mu.Lock()
			b.cond.Wait()
			b.mu.Unlock()
			close(waitCh)
		}()

		b.mu.Unlock()
		select {
		case <-waitCh:
			b.mu.Lock()
		case <-time.After(ConditionWaitTime):
			b.mu.Lock()
			if off >= b.writeCursor {
				log.Warn("MusicEngine: Downstream read stalled for 2.5s. Returning graceful EOF to prevent player hang")
				return 0, io.EOF
			}
		}
	}
}

// MarkDone signals that upstream has completed downloading
func (b *SlabRingBuffer) MarkDone(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.err = err
	b.isDone.Store(true)
	b.cond.Broadcast()
}

// Close releases allocated slabs back to the pool
func (b *SlabRingBuffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.isClosed.Swap(true) {
		return nil
	}
	for _, slab := range b.slabs {
		slabPtr := &slab
		slabPool.Put(slabPtr)
	}
	b.slabs = nil
	b.cond.Broadcast()
	return nil
}

// SniffedResult returns audio metadata detected from the initial chunk
func (b *SlabRingBuffer) SniffedResult() SniffResult {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.sniffed && len(b.initialChunk) > 0 {
		b.sniffResult = SniffAudioFormat(b.initialChunk)
		b.sniffed = true
	}
	return b.sniffResult
}
