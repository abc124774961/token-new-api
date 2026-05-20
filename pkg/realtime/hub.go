package realtime

import (
	"sync"
	"sync/atomic"
	"time"
)

type Hub struct {
	topics  map[string]Topic
	mu      sync.RWMutex
	seq     atomic.Int64
	nowFunc func() time.Time
}

func NewHub() *Hub {
	return &Hub{
		topics:  map[string]Topic{},
		nowFunc: time.Now,
	}
}

func (h *Hub) Register(topic Topic) {
	if h == nil || topic == nil || topic.Name() == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.topics[topic.Name()] = topic
}

func (h *Hub) Topic(name string) (Topic, bool) {
	if h == nil {
		return nil, false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	topic, ok := h.topics[name]
	return topic, ok
}

func (h *Hub) Prepare(message ServerMessage) ServerMessage {
	if h == nil {
		return message
	}
	if message.Sequence == 0 {
		message.Sequence = h.seq.Add(1)
	}
	if message.SentAt == 0 {
		message.SentAt = h.nowFunc().UnixMilli()
	}
	return message
}

func (h *Hub) Subscribe(subscriber Subscriber, subscription Subscription) bool {
	topic, ok := h.Topic(subscription.Topic)
	if !ok {
		return false
	}
	topic.Subscribe(subscriber, subscription)
	return true
}

func (h *Hub) Unsubscribe(subscriber Subscriber, subscription Subscription) {
	topic, ok := h.Topic(subscription.Topic)
	if !ok {
		return
	}
	topic.Unsubscribe(subscriber, subscription.ID)
}
