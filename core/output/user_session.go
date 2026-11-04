package output

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/log"
)

// ChannelSessionState represents the playback snapshot of a single independent audio track/channel.
type ChannelSessionState struct {
	Channel      string          `json:"channel"`      // "local" or "remote"
	DeviceID     string          `json:"deviceId"`     // "local", "hqplayer", or DLNA UUID
	DeviceName   string          `json:"deviceName"`   // e.g. "本机播放", "HQPlayer - 客厅"
	Track        TrackMetadata   `json:"track"`
	Queue        []TrackMetadata `json:"queue"`
	State        string          `json:"state"` // "playing", "paused", "stopped"
	TimeOffset   float64         `json:"timeOffset"`
	StartedAt    int64           `json:"startedAt"`
	Epoch        uint64          `json:"epoch"`
	PlayMode     string          `json:"playMode"`
	CurrentIndex int             `json:"currentIndex"`
	SourceClient string          `json:"sourceClient,omitempty"`
}

// DualSessionState represents the complete dual-track state (Local + Remote) of a user session.
type DualSessionState struct {
	UserID       string              `json:"userId"`
	UserName     string              `json:"userName"`
	ActiveEvent  string              `json:"activeEvent"` // "local" or "remote" (which channel triggered the update)
	Local        ChannelSessionState `json:"local"`
	Remote       ChannelSessionState `json:"remote"`
	SourceClient string              `json:"sourceClient,omitempty"`

	// Backward compatibility top-level fields for legacy callers
	Track        TrackMetadata   `json:"track"`
	Queue        []TrackMetadata `json:"queue"`
	State        string          `json:"state"` // "playing", "paused", "stopped"
	TimeOffset   float64         `json:"timeOffset"`
	StartedAt    int64           `json:"startedAt"`
	Epoch        uint64          `json:"epoch"`
	OutputTarget string          `json:"outputTarget"`
	PlayMode     string          `json:"playMode"`
	CurrentIndex int             `json:"currentIndex"`
}

// Alias UserSessionState to DualSessionState for backwards compatibility
type UserSessionState = DualSessionState

type StateBroadcaster func(ctx context.Context, username string, state DualSessionState)

type channelData struct {
	channel      string
	deviceID     string
	deviceName   string
	queue        []TrackMetadata
	currentIndex int
	currentTrack TrackMetadata
	state        string // "playing", "paused", "stopped"
	timeOffset   float64
	startedAt    int64
	epoch        uint64
	playMode     string
}

func (c *channelData) snapshot() ChannelSessionState {
	offset := c.timeOffset
	if c.state == "playing" && c.startedAt > 0 {
		elapsed := float64(time.Now().UnixMilli()-c.startedAt) / 1000.0
		offset += elapsed
	}
	qCopy := make([]TrackMetadata, len(c.queue))
	copy(qCopy, c.queue)
	return ChannelSessionState{
		Channel:      c.channel,
		DeviceID:     c.deviceID,
		DeviceName:   c.deviceName,
		Track:        c.currentTrack,
		Queue:        qCopy,
		State:        c.state,
		TimeOffset:   offset,
		StartedAt:    c.startedAt,
		Epoch:        c.epoch,
		PlayMode:     c.playMode,
		CurrentIndex: c.currentIndex,
	}
}

// UserSession maintains two fully independent playback channels (Local and Remote) for a User.
type UserSession struct {
	mu       sync.RWMutex
	mgr      *UserSessionManager
	userID   string
	userName string
	local    channelData
	remote   channelData
}

// NormalizeChannel resolves the target channel ("local" or "remote").
func NormalizeChannel(channel, deviceID string) string {
	if channel == "remote" || channel == "local" {
		return channel
	}
	if deviceID == "" || deviceID == string(DeviceTypeLocal) {
		return "local"
	}
	return "remote"
}

func (s *UserSession) getChannelData(channel string) *channelData {
	if channel == "remote" {
		return &s.remote
	}
	return &s.local
}

func (s *UserSession) dualSnapshotLocked(activeEvent string, sourceClient string) DualSessionState {
	localSnap := s.local.snapshot()
	remoteSnap := s.remote.snapshot()

	activeSnap := localSnap
	if activeEvent == "remote" {
		activeSnap = remoteSnap
	}

	return DualSessionState{
		UserID:       s.userID,
		UserName:     s.userName,
		ActiveEvent:  activeEvent,
		Local:        localSnap,
		Remote:       remoteSnap,
		SourceClient: sourceClient,
		// Backward compatibility top-level fields mirror active event:
		Track:        activeSnap.Track,
		Queue:        activeSnap.Queue,
		State:        activeSnap.State,
		TimeOffset:   activeSnap.TimeOffset,
		StartedAt:    activeSnap.StartedAt,
		Epoch:        activeSnap.Epoch,
		OutputTarget: activeSnap.DeviceID,
		PlayMode:     activeSnap.PlayMode,
		CurrentIndex: activeSnap.CurrentIndex,
	}
}

// DualSnapshot returns the combined snapshot of both channels.
func (s *UserSession) DualSnapshot(activeEvent string, sourceClient string) DualSessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dualSnapshotLocked(activeEvent, sourceClient)
}

// Snapshot returns the snapshot of a specific channel (defaults to "local").
func (s *UserSession) Snapshot(channel ...string) ChannelSessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ch := "local"
	if len(channel) > 0 && channel[0] != "" {
		ch = channel[0]
	}
	return s.getChannelData(ch).snapshot()
}

// GetQueue returns the playlist of a specific channel (defaults to "local").
func (s *UserSession) GetQueue(channel ...string) []TrackMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ch := "local"
	if len(channel) > 0 && channel[0] != "" {
		ch = channel[0]
	}
	c := s.getChannelData(ch)
	qCopy := make([]TrackMetadata, len(c.queue))
	copy(qCopy, c.queue)
	return qCopy
}

// SetQueue updates the playlist and starts/stops playback for the specified channel only.
func (s *UserSession) SetQueue(ctx context.Context, channel string, tracks []TrackMetadata, index int, offset float64, playMode string, autoPlay bool, deviceID string, sourceClient string) DualSessionState {
	ch := NormalizeChannel(channel, deviceID)

	s.mu.Lock()
	c := s.getChannelData(ch)
	c.epoch = atomic.AddUint64(&c.epoch, 1)
	c.queue = make([]TrackMetadata, len(tracks))
	copy(c.queue, tracks)

	if index >= 0 && index < len(tracks) {
		c.currentIndex = index
		c.currentTrack = tracks[index]
	} else if len(tracks) > 0 {
		c.currentIndex = 0
		c.currentTrack = tracks[0]
	} else {
		c.currentIndex = -1
		c.currentTrack = TrackMetadata{}
	}

	c.timeOffset = offset
	if autoPlay {
		c.state = "playing"
		c.startedAt = time.Now().UnixMilli()
	} else {
		c.state = "stopped"
		c.startedAt = 0
	}
	if playMode != "" {
		c.playMode = playMode
	}
	if deviceID != "" {
		c.deviceID = deviceID
	}

	snap := s.dualSnapshotLocked(ch, sourceClient)
	s.mu.Unlock()

	s.mgr.broadcast(ctx, s.userName, snap)
	return snap
}

// ClearQueue clears the playlist for the specified channel only.
func (s *UserSession) ClearQueue(ctx context.Context, channel string, sourceClient string) DualSessionState {
	ch := NormalizeChannel(channel, "")

	s.mu.Lock()
	c := s.getChannelData(ch)
	c.epoch = atomic.AddUint64(&c.epoch, 1)
	c.queue = make([]TrackMetadata, 0)
	c.currentIndex = -1
	c.currentTrack = TrackMetadata{}
	c.state = "stopped"
	c.startedAt = 0
	c.timeOffset = 0

	snap := s.dualSnapshotLocked(ch, sourceClient)
	s.mu.Unlock()

	s.mgr.broadcast(ctx, s.userName, snap)
	return snap
}

// UpdatePlayback updates playback state, time offset, and track on the specified channel only.
func (s *UserSession) UpdatePlayback(ctx context.Context, channel string, state string, track TrackMetadata, index int, offset float64, playMode string, deviceID string, sourceClient string) DualSessionState {
	ch := NormalizeChannel(channel, deviceID)

	s.mu.Lock()
	c := s.getChannelData(ch)
	c.epoch = atomic.AddUint64(&c.epoch, 1)
	if state != "" {
		c.state = state
	}
	if track.TrackID != "" {
		c.currentTrack = track
	}
	if index >= 0 && index < len(c.queue) {
		c.currentIndex = index
		c.currentTrack = c.queue[index]
	} else if index >= 0 {
		c.currentIndex = index
	}
	c.timeOffset = offset
	if c.state == "playing" {
		c.startedAt = time.Now().UnixMilli()
	} else {
		c.startedAt = 0
	}
	if playMode != "" {
		c.playMode = playMode
	}
	if deviceID != "" {
		c.deviceID = deviceID
	}

	snap := s.dualSnapshotLocked(ch, sourceClient)
	s.mu.Unlock()

	s.mgr.broadcast(ctx, s.userName, snap)
	return snap
}

type UserSessionManager struct {
	mu          sync.RWMutex
	sessions    map[string]*UserSession
	broadcaster StateBroadcaster
}

var (
	userSessionMgrInstance *UserSessionManager
	userSessionMgrOnce     sync.Once
)

func GetUserSessionManager() *UserSessionManager {
	userSessionMgrOnce.Do(func() {
		userSessionMgrInstance = &UserSessionManager{
			sessions: make(map[string]*UserSession),
		}
	})
	return userSessionMgrInstance
}

func (m *UserSessionManager) SetBroadcaster(b StateBroadcaster) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.broadcaster = b
}

func (m *UserSessionManager) broadcast(ctx context.Context, username string, state DualSessionState) {
	m.mu.RLock()
	b := m.broadcaster
	m.mu.RUnlock()
	if b != nil {
		b(ctx, username, state)
	}
}

func (m *UserSessionManager) GetSession(userID, userName string) *UserSession {
	if userID == "" {
		userID = "anonymous"
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if sess, ok := m.sessions[userID]; ok {
		if userName != "" && sess.userName != userName {
			sess.userName = userName
		}
		return sess
	}

	sess := &UserSession{
		mgr:      m,
		userID:   userID,
		userName: userName,
		local: channelData{
			channel:      "local",
			deviceID:     string(DeviceTypeLocal),
			deviceName:   "本机播放",
			queue:        make([]TrackMetadata, 0),
			currentIndex: -1,
			state:        "stopped",
			playMode:     "orderLoop",
		},
		remote: channelData{
			channel:      "remote",
			deviceID:     "hqplayer",
			deviceName:   "HQPlayer",
			queue:        make([]TrackMetadata, 0),
			currentIndex: -1,
			state:        "stopped",
			playMode:     "orderLoop",
		},
	}
	m.sessions[userID] = sess
	log.Debug(fmt.Sprintf("UserSessionManager: Initialized new dual-track user session for user %s (%s)", userName, userID))
	return sess
}
