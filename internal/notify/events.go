package notify

import (
	"sync"
	"time"
)

type EventType string

const (
	EventBackupStarted      EventType = "BACKUP_STARTED"
	EventBackupProgress     EventType = "BACKUP_PROGRESS"
	EventBackupCompleted    EventType = "BACKUP_COMPLETED"
	EventBackupFailed       EventType = "BACKUP_FAILED"
	EventCloudRetrying      EventType = "CLOUD_RETRYING"
	EventCloudCompleted     EventType = "CLOUD_COMPLETED"
	EventLowDiskSpace       EventType = "LOW_DISK_SPACE"
	EventTestNotification   EventType = "TEST_NOTIFICATION"
	EventSystemUpdating     EventType = "SYSTEM_UPDATING"
	EventShutdownTray       EventType = "SHUTDOWN_TRAY"
)

type Event struct {
	Type        EventType `json:"type"`
	Title       string    `json:"title"`
	Message     string    `json:"message"`
	Severity    string    `json:"severity"` // "info" | "success" | "warning" | "error"
	ShowToast   bool      `json:"showToast"`
	ExecutionID int64     `json:"executionId,omitempty"`
	DBName      string    `json:"dbName,omitempty"`
	Percent     int       `json:"percent,omitempty"`
	TargetUser  string    `json:"targetUser,omitempty"` // Optional filter for SHUTDOWN_TRAY
	Timestamp   string    `json:"timestamp"`
}

// Broadcaster fans out events to connected IPC subscribers using non-blocking bounded channels.
type Broadcaster struct {
	mu          sync.RWMutex
	nextID      uint64
	subscribers map[uint64]chan Event
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[uint64]chan Event),
	}
}

func (b *Broadcaster) Subscribe() (uint64, <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	ch := make(chan Event, 32)
	b.subscribers[id] = ch
	return id, ch
}

func (b *Broadcaster) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
}

func (b *Broadcaster) Publish(ev Event) {
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
			// Drop oldest or skip if subscriber is lagging so service never blocks or leaks memory.
		}
	}
}
