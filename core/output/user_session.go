package output

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/navidrome/navidrome/log"
)

type UserSessionState struct {
	UserID       string        `json:"userId"`
	UserName     string        `json:"userName"`
	Track        TrackMetadata `json:"track"`
	State        string        `json:"state"` // "playing", "paused", "stopped"
	TimeOffset   float64       `json:"timeOffset"`
	StartedAt    int64         `json:"startedAt"`
	Epoch        uint64        `json:"epoch"`
	OutputTarget string        `json:"outputTarget"`
	PlayMode     string        `json:"playMode"`
	CurrentIndex int           `json:"currentIndex"`
	SourceClient string        `json:"sourceClient,omitempty"`
}

type StateBroadcaster func(ctx context.Context, username string, state UserSessionState)

type UserSession struct {
	mu           sync.RWMutex
	mgr          *UserSessionManager
	userID       string
	userName     string
	queue        []TrackMetadata
	currentIndex int
	currentTrack TrackMetadata
	state        string // "playing", "paused", "stopped"
	timeOffset   float64
	startedAt    int64
	epoch        uint64
	playMode     string
	outputTarget string
}

func (s *UserSession) Snapshot() UserSessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	offset := s.timeOffset
	if s.state == "playing" && s.startedAt > 0 {
		elapsed := float64(time.Now().UnixMilli()-s.startedAt) / 1000.0
		offset += elapsed
	}
	return UserSessionState{
		UserID:       s.userID,
		UserName:     s.userName,
		Track:        s.currentTrack,
		State:        s.state,
		TimeOffset:   offset,
		StartedAt:    s.startedAt,
		Epoch:        s.epoch,
		OutputTarget: s.outputTarget,
		PlayMode:     s.playMode,
		CurrentIndex: s.currentIndex,
	}
}

func (s *UserSession) UpdatePlayback(ctx context.Context, state string, track TrackMetadata, index int, offset float64, playMode string, outputTarget string, sourceClient string) UserSessionState {
	s.mu.Lock()
	curEpoch := atomic.AddUint64(&s.epoch, 1)
	s.state = state
	if track.TrackID != "" {
		s.currentTrack = track
	}
	if index >= 0 {
		s.currentIndex = index
	}
	s.timeOffset = offset
	if state == "playing" {
		s.startedAt = time.Now().UnixMilli()
	} else {
		s.startedAt = 0
	}
	if playMode != "" {
		s.playMode = playMode
	}
	if outputTarget != "" {
		s.outputTarget = outputTarget
	}
	snapshot := UserSessionState{
		UserID:       s.userID,
		UserName:     s.userName,
		Track:        s.currentTrack,
		State:        s.state,
		TimeOffset:   s.timeOffset,
		StartedAt:    s.startedAt,
		Epoch:        curEpoch,
		OutputTarget: s.outputTarget,
		PlayMode:     s.playMode,
		CurrentIndex: s.currentIndex,
		SourceClient: sourceClient,
	}
	s.mu.Unlock()

	s.mgr.broadcast(ctx, s.userName, snapshot)
	return snapshot
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

func (m *UserSessionManager) broadcast(ctx context.Context, username string, state UserSessionState) {
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
		mgr:          m,
		userID:       userID,
		userName:     userName,
		queue:        make([]TrackMetadata, 0),
		currentIndex: -1,
		state:        "stopped",
		playMode:     "orderLoop",
		outputTarget: string(DeviceTypeLocal),
	}
	m.sessions[userID] = sess
	log.Debug(fmt.Sprintf("UserSessionManager: Initialized new user playback session for user %s (%s)", userName, userID))
	return sess
}
