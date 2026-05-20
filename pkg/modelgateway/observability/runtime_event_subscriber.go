package observability

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/go-redis/redis/v8"
)

const defaultRuntimeEventSubscriberRecentLimit = 64

type RuntimeEventSubscriberOptions struct {
	RedisOptions scheduler.RedisRuntimeSyncEventSubscriberOptions
	Now          func() time.Time
	RecentLimit  int
}

type RuntimeEventSubscriberSnapshot struct {
	UpdatedAt int64                        `json:"updated_at,omitempty"`
	Total     int                          `json:"total"`
	ByKind    map[string]int               `json:"by_kind,omitempty"`
	LastEvent scheduler.RuntimeSyncEvent   `json:"last_event,omitempty"`
	Recent    []scheduler.RuntimeSyncEvent `json:"recent,omitempty"`
}

type RuntimeEventSubscriber struct {
	subscriber scheduler.RuntimeSyncEventSubscriber
	now        func() time.Time
	limit      int

	mu        sync.Mutex
	updatedAt int64
	total     int
	byKind    map[string]int
	lastEvent scheduler.RuntimeSyncEvent
	recent    []scheduler.RuntimeSyncEvent

	closeOnce sync.Once
	closeErr  error
}

// NewRuntimeEventSubscriber starts the configured Redis subscriber immediately.
// Use NewRuntimeEventSubscriberRecorder when only an in-memory recorder is needed.
func NewRuntimeEventSubscriber(options RuntimeEventSubscriberOptions) *RuntimeEventSubscriber {
	companion := NewRuntimeEventSubscriberRecorder(options)
	redisOptions := options.RedisOptions
	previousOnEvent := redisOptions.OnEvent
	redisOptions.OnEvent = func(event scheduler.RuntimeSyncEvent) {
		companion.Observe(event)
		if previousOnEvent != nil {
			previousOnEvent(event)
		}
	}
	companion.subscriber = scheduler.NewRedisRuntimeSyncEventSubscriber(redisOptions)
	return companion
}

// NewRuntimeEventSubscriberRecorder only records events passed to Observe.
// It does not start Redis Pub/Sub or any background goroutine.
func NewRuntimeEventSubscriberRecorder(options RuntimeEventSubscriberOptions) *RuntimeEventSubscriber {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	limit := options.RecentLimit
	if limit <= 0 {
		limit = defaultRuntimeEventSubscriberRecentLimit
	}
	companion := &RuntimeEventSubscriber{
		now:    now,
		limit:  limit,
		byKind: map[string]int{},
	}
	return companion
}

// NewRedisRuntimeEventSubscriber starts an explicit Redis-backed companion.
func NewRedisRuntimeEventSubscriber(client *redis.Client, options RuntimeEventSubscriberOptions) *RuntimeEventSubscriber {
	options.RedisOptions.Client = client
	return NewRuntimeEventSubscriber(options)
}

func (s *RuntimeEventSubscriber) Close() error {
	if s == nil || s.subscriber == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.closeErr = s.subscriber.Close()
	})
	return s.closeErr
}

func (s *RuntimeEventSubscriber) Observe(event scheduler.RuntimeSyncEvent) {
	if s == nil {
		return
	}
	if event.UpdatedAt == 0 {
		event.UpdatedAt = s.now().Unix()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updatedAt = s.now().Unix()
	s.total++
	if event.Kind != "" {
		s.byKind[event.Kind]++
	}
	s.lastEvent = event
	s.recent = append(s.recent, event)
	if len(s.recent) > s.limit {
		copy(s.recent, s.recent[len(s.recent)-s.limit:])
		s.recent = s.recent[:s.limit]
	}
}

func (s *RuntimeEventSubscriber) Snapshot() RuntimeEventSubscriberSnapshot {
	if s == nil {
		return RuntimeEventSubscriberSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	byKind := make(map[string]int, len(s.byKind))
	for kind, count := range s.byKind {
		byKind[kind] = count
	}
	recent := append([]scheduler.RuntimeSyncEvent(nil), s.recent...)
	return RuntimeEventSubscriberSnapshot{
		UpdatedAt: s.updatedAt,
		Total:     s.total,
		ByKind:    byKind,
		LastEvent: s.lastEvent,
		Recent:    recent,
	}
}
