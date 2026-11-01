package output

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/log"
)

// PlayerSession implements a Roon-Core style autonomous server-side playback session.
// It manages the active queue, auto-advances tracks when they finish, guards against
// race conditions with Epoch counters, and continues streaming even if frontend clients disconnect.
type PlayerSession struct {
	mu           sync.RWMutex
	mgr          *Manager
	deviceID     string
	queue        []TrackMetadata
	currentIndex int
	state        string // "playing", "paused", "stopped"
	epoch        uint64
	startedAt    int64 // unix milli
	timeOffset   float64
	playMode     string // "order", "orderLoop", "singleLoop", "shufflePlay"
	timer        *time.Timer
}

func NewPlayerSession(mgr *Manager, deviceID string) *PlayerSession {
	return &PlayerSession{
		mgr:          mgr,
		deviceID:     deviceID,
		queue:        make([]TrackMetadata, 0),
		currentIndex: -1,
		state:        "stopped",
		playMode:     "orderLoop",
	}
}

func (s *PlayerSession) SetDeviceID(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceID = deviceID
}

func (s *PlayerSession) SetPlayMode(mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mode != "" {
		s.playMode = mode
		log.Info(fmt.Sprintf("PlayerSession [%s]: Play mode updated to %s", s.deviceID, mode))
	}
}

func (s *PlayerSession) SetQueue(tracks []TrackMetadata, startIndex int, timeOffset float64, playMode string) error {
	s.mu.Lock()

	if playMode != "" {
		s.playMode = playMode
	}

	// P0 Guard: If current session is actively playing the same track at startIndex,
	// only update upcoming queue in memory. DO NOT cancel timer, DO NOT increment epoch,
	// and DO NOT send cut-off commands to remote DAC!
	if s.state == "playing" && len(s.queue) > 0 && s.currentIndex >= 0 && s.currentIndex < len(s.queue) &&
		startIndex >= 0 && startIndex < len(tracks) &&
		s.queue[s.currentIndex].TrackID == tracks[startIndex].TrackID &&
		tracks[startIndex].TrackID != "" {
		s.queue = make([]TrackMetadata, len(tracks))
		copy(s.queue, tracks)
		s.currentIndex = startIndex
		devID := s.deviceID
		s.mu.Unlock()
		log.Info(fmt.Sprintf("PlayerSession [%s]: Dynamically updated queue (%d tracks) without interrupting playback: %s",
			devID, len(tracks), tracks[startIndex].Title))
		return nil
	}

	s.cancelTimerLocked()
	curEpoch := atomic.AddUint64(&s.epoch, 1)

	s.queue = make([]TrackMetadata, len(tracks))
	copy(s.queue, tracks)
	if startIndex < 0 || startIndex >= len(s.queue) {
		startIndex = 0
	}
	s.currentIndex = startIndex
	s.timeOffset = timeOffset
	s.startedAt = time.Now().UnixMilli()
	s.state = "playing"

	if len(s.queue) == 0 {
		s.state = "stopped"
		s.mu.Unlock()
		return nil
	}

	track := s.queue[s.currentIndex]
	s.armTimerLocked(track.Duration, timeOffset, curEpoch)
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s]: SetQueue loaded %d tracks, starting at track %d: %s (%s)",
		devID, len(tracks), startIndex, track.Title, track.Artist))

	return s.mgr.playTrackOnDevice(context.Background(), devID, track, timeOffset)
}

func (s *PlayerSession) Enqueue(tracks []TrackMetadata) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, tracks...)
	log.Info(fmt.Sprintf("PlayerSession [%s]: Enqueued %d tracks, total queue size %d", s.deviceID, len(tracks), len(s.queue)))
}

func (s *PlayerSession) PlayTrackAt(index int, timeOffset float64) error {
	s.mu.Lock()
	s.cancelTimerLocked()
	curEpoch := atomic.AddUint64(&s.epoch, 1)

	if index < 0 || index >= len(s.queue) {
		s.state = "stopped"
		devID := s.deviceID
		s.mu.Unlock()
		return s.mgr.controlDevice(context.Background(), devID, "stop", 0, 0)
	}

	s.currentIndex = index
	s.timeOffset = timeOffset
	s.startedAt = time.Now().UnixMilli()
	s.state = "playing"
	track := s.queue[index]
	s.armTimerLocked(track.Duration, timeOffset, curEpoch)
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s]: Switched to track %d/%d: %s - %s", devID, index+1, len(s.queue), track.Title, track.Artist))
	return s.mgr.playTrackOnDevice(context.Background(), devID, track, timeOffset)
}

func (s *PlayerSession) Next(manual bool) error {
	s.mu.Lock()
	s.cancelTimerLocked()
	curEpoch := atomic.AddUint64(&s.epoch, 1)

	nextIdx := s.currentIndex + 1
	if nextIdx >= len(s.queue) {
		log.Info(fmt.Sprintf("PlayerSession [%s]: Next reached end of queue", s.deviceID))
		s.state = "stopped"
		s.startedAt = 0
		s.timeOffset = 0
		devID := s.deviceID
		s.mu.Unlock()
		return s.mgr.controlDevice(context.Background(), devID, "stop", 0, 0)
	}

	s.currentIndex = nextIdx
	s.timeOffset = 0
	s.startedAt = time.Now().UnixMilli()
	s.state = "playing"
	track := s.queue[nextIdx]
	s.armTimerLocked(track.Duration, 0, curEpoch)
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s] Next: track %d/%d: %s - %s", devID, nextIdx+1, len(s.queue), track.Title, track.Artist))
	return s.mgr.playTrackOnDevice(context.Background(), devID, track, 0)
}

func (s *PlayerSession) Prev() error {
	s.mu.Lock()
	s.cancelTimerLocked()
	curEpoch := atomic.AddUint64(&s.epoch, 1)

	elapsed := s.timeOffset
	if s.state == "playing" && s.startedAt > 0 {
		elapsed += float64(time.Now().UnixMilli()-s.startedAt) / 1000.0
	}

	targetIdx := s.currentIndex
	if elapsed <= 3.0 && targetIdx > 0 {
		targetIdx--
	}
	if targetIdx < 0 {
		targetIdx = 0
	}

	if targetIdx >= len(s.queue) {
		s.mu.Unlock()
		return nil
	}

	s.currentIndex = targetIdx
	s.timeOffset = 0
	s.startedAt = time.Now().UnixMilli()
	s.state = "playing"
	track := s.queue[targetIdx]
	s.armTimerLocked(track.Duration, 0, curEpoch)
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s] Prev: track %d/%d: %s - %s", devID, targetIdx+1, len(s.queue), track.Title, track.Artist))
	return s.mgr.playTrackOnDevice(context.Background(), devID, track, 0)
}

func (s *PlayerSession) Pause() error {
	s.mu.Lock()
	s.cancelTimerLocked()
	atomic.AddUint64(&s.epoch, 1)

	if s.state == "playing" && s.startedAt > 0 {
		s.timeOffset += float64(time.Now().UnixMilli()-s.startedAt) / 1000.0
	}
	s.state = "paused"
	s.startedAt = 0
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s]: Paused at offset %.1fs", devID, s.timeOffset))
	return s.mgr.controlDevice(context.Background(), devID, "pause", 0, 0)
}

func (s *PlayerSession) Resume() error {
	s.mu.Lock()
	s.cancelTimerLocked()
	curEpoch := atomic.AddUint64(&s.epoch, 1)

	if s.currentIndex < 0 || s.currentIndex >= len(s.queue) {
		s.mu.Unlock()
		return nil
	}

	s.state = "playing"
	s.startedAt = time.Now().UnixMilli()
	track := s.queue[s.currentIndex]
	offset := s.timeOffset
	s.armTimerLocked(track.Duration, offset, curEpoch)
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s]: Resumed track %d (%s) from offset %.1fs", devID, s.currentIndex, track.Title, offset))
	return s.mgr.playTrackOnDevice(context.Background(), devID, track, offset)
}

func (s *PlayerSession) Stop() error {
	s.mu.Lock()
	s.cancelTimerLocked()
	atomic.AddUint64(&s.epoch, 1)

	s.state = "stopped"
	s.startedAt = 0
	s.timeOffset = 0
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s]: Stopped", devID))
	return s.mgr.controlDevice(context.Background(), devID, "stop", 0, 0)
}

func (s *PlayerSession) Seek(offset float64) error {
	s.mu.Lock()
	s.cancelTimerLocked()
	curEpoch := atomic.AddUint64(&s.epoch, 1)

	s.timeOffset = offset
	s.startedAt = time.Now().UnixMilli()
	if s.currentIndex >= 0 && s.currentIndex < len(s.queue) {
		track := s.queue[s.currentIndex]
		if s.state == "playing" {
			s.armTimerLocked(track.Duration, offset, curEpoch)
		}
	}
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s]: Seek to %.1fs", devID, offset))
	return s.mgr.controlDevice(context.Background(), devID, "seek", offset, 0)
}

func (s *PlayerSession) armTimerLocked(duration float64, offset float64, epoch uint64) {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if duration <= 0 {
		return
	}
	rem := duration - offset
	if rem <= 0 {
		rem = 0.5
	}

	// 800ms hardware DAC crystal & FIFO buffer drain window (prevents tail truncation on high-res audio)
	timerDuration := time.Duration(rem*float64(time.Second)) + 800*time.Millisecond
	s.timer = time.AfterFunc(timerDuration, func() {
		s.onTrackEnded(epoch)
	})
}

func (s *PlayerSession) cancelTimerLocked() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

func (s *PlayerSession) onTrackEnded(epoch uint64) {
	s.mu.Lock()
	if atomic.LoadUint64(&s.epoch) != epoch || s.state != "playing" {
		s.mu.Unlock()
		return
	}

	var nextIdx int
	if s.playMode == "singleLoop" {
		nextIdx = s.currentIndex
	} else {
		nextIdx = s.currentIndex + 1
		if nextIdx >= len(s.queue) {
			if s.playMode == "orderLoop" && len(s.queue) > 0 {
				nextIdx = 0
			} else {
				log.Info(fmt.Sprintf("PlayerSession [%s]: Auto-Advance completed all tracks in queue", s.deviceID))
				s.state = "stopped"
				s.startedAt = 0
				s.timeOffset = 0
				s.cancelTimerLocked()
				atomic.AddUint64(&s.epoch, 1)
				devID := s.deviceID
				s.mu.Unlock()
				_ = s.mgr.controlDevice(context.Background(), devID, "stop", 0, 0)
				return
			}
		}
	}

	curEpoch := atomic.AddUint64(&s.epoch, 1)
	s.currentIndex = nextIdx
	s.timeOffset = 0
	s.startedAt = time.Now().UnixMilli()
	track := s.queue[nextIdx]
	s.armTimerLocked(track.Duration, 0, curEpoch)
	devID := s.deviceID
	s.mu.Unlock()

	log.Info(fmt.Sprintf("PlayerSession [%s] Auto-Advance (mode=%s): playing track %d/%d: %s - %s (%.1fs)",
		devID, s.playMode, nextIdx+1, len(s.queue), track.Title, track.Artist, track.Duration))

	_ = s.mgr.playTrackOnDevice(context.Background(), devID, track, 0)
}

func (s *PlayerSession) GetStatus() NowPlayingInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	offset := s.timeOffset
	if s.state == "playing" && s.startedAt > 0 {
		elapsed := float64(time.Now().UnixMilli()-s.startedAt) / 1000.0
		offset += elapsed
	}

	var curTrack TrackMetadata
	if s.currentIndex >= 0 && s.currentIndex < len(s.queue) {
		curTrack = s.queue[s.currentIndex]
	}

	qCopy := make([]TrackMetadata, len(s.queue))
	copy(qCopy, s.queue)

	return NowPlayingInfo{
		DeviceID:     s.deviceID,
		Track:        curTrack,
		State:        s.state,
		TimeOffset:   offset,
		StartedAt:    s.startedAt,
		Queue:        qCopy,
		CurrentIndex: s.currentIndex,
		Epoch:        atomic.LoadUint64(&s.epoch),
		PlayMode:     s.playMode,
	}
}
