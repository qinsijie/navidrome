package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/navidrome/navidrome/conf"
)

type TerminalQueueState struct {
	ClientID   string          `json:"clientId"`
	ClientName string          `json:"clientName"`
	Queue      []TrackMetadata `json:"queue"`
	CurrentIdx int             `json:"currentIndex"`
	TimeOffset float64         `json:"timeOffset"`
	UpdatedAt  int64           `json:"updatedAt"`
}

type TerminalStore struct {
	mu        sync.RWMutex
	terminals map[string]*TerminalQueueState
}

var (
	terminalStoreInstance *TerminalStore
	terminalStoreOnce     sync.Once
)

func GetTerminalStore() *TerminalStore {
	terminalStoreOnce.Do(func() {
		terminalStoreInstance = &TerminalStore{
			terminals: make(map[string]*TerminalQueueState),
		}
		terminalStoreInstance.load()
	})
	return terminalStoreInstance
}

func (s *TerminalStore) storeFilePath() string {
	dir := conf.Server.DataFolder.String()
	if dir == "" {
		pkgVar := os.Getenv("TRIM_PKGVAR")
		if pkgVar != "" {
			dir = filepath.Join(pkgVar, "data")
		} else {
			dir = "./var/data"
		}
	}
	return filepath.Join(dir, "terminal_queues.json")
}

func (s *TerminalStore) load() {
	p := s.storeFilePath()
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var loaded map[string]*TerminalQueueState
	if err := json.Unmarshal(data, &loaded); err == nil && loaded != nil {
		s.mu.Lock()
		s.terminals = loaded
		s.mu.Unlock()
	}
}

func (s *TerminalStore) save() {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.terminals, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return
	}
	p := s.storeFilePath()
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	tmpPath := p + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err == nil {
		_ = os.Rename(tmpPath, p)
	}
}

func (s *TerminalStore) Get(clientID string) (*TerminalQueueState, bool) {
	if clientID == "" {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.terminals[clientID]
	if !ok || val == nil {
		return nil, false
	}
	qCopy := make([]TrackMetadata, len(val.Queue))
	copy(qCopy, val.Queue)
	return &TerminalQueueState{
		ClientID:   val.ClientID,
		ClientName: val.ClientName,
		Queue:      qCopy,
		CurrentIdx: val.CurrentIdx,
		TimeOffset: val.TimeOffset,
		UpdatedAt:  val.UpdatedAt,
	}, true
}

func (s *TerminalStore) Save(clientID, clientName string, queue []TrackMetadata, currentIdx int, timeOffset float64) {
	if clientID == "" {
		return
	}
	s.mu.Lock()
	qCopy := make([]TrackMetadata, len(queue))
	copy(qCopy, queue)
	existingName := clientName
	if existingName == "" && s.terminals[clientID] != nil {
		existingName = s.terminals[clientID].ClientName
	}
	s.terminals[clientID] = &TerminalQueueState{
		ClientID:   clientID,
		ClientName: existingName,
		Queue:      qCopy,
		CurrentIdx: currentIdx,
		TimeOffset: timeOffset,
		UpdatedAt:  time.Now().UnixMilli(),
	}
	s.mu.Unlock()
	go s.save()
}
