package scheduler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/go-redis/redis/v8"
)

const (
	defaultRuntimeSyncEventChannel  = "new-api:modelgateway:runtime_events:v1"
	defaultRuntimeSyncPubSubBuffer  = 128
	defaultRuntimeSyncPubSubTimeout = 500 * time.Millisecond
	defaultRuntimeSyncReconnectMin  = 200 * time.Millisecond
	defaultRuntimeSyncReconnectMax  = 5 * time.Second
	defaultRuntimeSyncMaxWatermarks = 4096
)

type RuntimeSyncEvent struct {
	Kind          string          `json:"kind"`
	CacheKey      string          `json:"cache_key,omitempty"`
	NodeID        string          `json:"node_id,omitempty"`
	RuntimeKey    core.RuntimeKey `json:"runtime_key,omitempty"`
	UpdatedAt     int64           `json:"updated_at,omitempty"`
	UpdatedAtNano int64           `json:"updated_at_nano,omitempty"`
}

type RuntimeSyncEventPublisher interface {
	Publish(event RuntimeSyncEvent) error
}

type RuntimeSyncEventSubscriber interface {
	Close() error
}

type RedisRuntimeSyncEventPublisherOptions struct {
	Client  *redis.Client
	Channel string
	Timeout time.Duration
	Enabled func() bool
}

type RedisRuntimeSyncEventPublisher struct {
	client  *redis.Client
	channel string
	timeout time.Duration
	enabled func() bool
}

func NewRedisRuntimeSyncEventPublisher(options RedisRuntimeSyncEventPublisherOptions) *RedisRuntimeSyncEventPublisher {
	channel := options.Channel
	if channel == "" {
		channel = defaultRuntimeSyncEventChannel
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultRuntimeSyncPubSubTimeout
	}
	enabled := options.Enabled
	if enabled == nil {
		enabled = func() bool {
			return true
		}
	}
	return &RedisRuntimeSyncEventPublisher{
		client:  options.Client,
		channel: channel,
		timeout: timeout,
		enabled: enabled,
	}
}

func (p *RedisRuntimeSyncEventPublisher) Publish(event RuntimeSyncEvent) error {
	if p == nil || p.client == nil || !p.enabled() {
		return nil
	}
	now := time.Now()
	if event.UpdatedAt == 0 {
		event.UpdatedAt = now.Unix()
	}
	if event.UpdatedAtNano == 0 {
		event.UpdatedAtNano = now.UnixNano()
	}
	payload, err := common.Marshal(event)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	return p.client.Publish(ctx, p.channel, string(payload)).Err()
}

type RedisRuntimeSyncEventSubscriberOptions struct {
	Client              *redis.Client
	Channel             string
	Buffer              int
	CallbackBuffer      int
	ReconnectMinBackoff time.Duration
	ReconnectMaxBackoff time.Duration
	MaxWatermarks       int
	Enabled             func() bool
	OnReady             func()
	OnEvent             func(RuntimeSyncEvent)
	OnError             func(error)
}

type RedisRuntimeSyncEventSubscriber struct {
	client              *redis.Client
	channel             string
	buffer              int
	reconnectMinBackoff time.Duration
	reconnectMaxBackoff time.Duration
	maxWatermarks       int
	enabled             func() bool
	onReady             func()
	onEvent             func(RuntimeSyncEvent)
	onError             func(error)

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	events chan RuntimeSyncEvent

	closeOnce   sync.Once
	watermarkMu sync.Mutex
	watermarks  map[string]runtimeSyncEventWatermark
}

type runtimeSyncEventWatermark struct {
	updatedAt int64
}

var ErrRuntimeSyncEventCallbackQueueFull = errors.New("runtime sync event callback queue full")

func NewRedisRuntimeSyncEventSubscriber(options RedisRuntimeSyncEventSubscriberOptions) *RedisRuntimeSyncEventSubscriber {
	channel := options.Channel
	if channel == "" {
		channel = defaultRuntimeSyncEventChannel
	}
	buffer := options.Buffer
	if buffer <= 0 {
		buffer = defaultRuntimeSyncPubSubBuffer
	}
	callbackBuffer := options.CallbackBuffer
	if callbackBuffer <= 0 {
		callbackBuffer = buffer
	}
	reconnectMinBackoff := options.ReconnectMinBackoff
	if reconnectMinBackoff <= 0 {
		reconnectMinBackoff = defaultRuntimeSyncReconnectMin
	}
	reconnectMaxBackoff := options.ReconnectMaxBackoff
	if reconnectMaxBackoff <= 0 {
		reconnectMaxBackoff = defaultRuntimeSyncReconnectMax
	}
	if reconnectMaxBackoff < reconnectMinBackoff {
		reconnectMaxBackoff = reconnectMinBackoff
	}
	maxWatermarks := options.MaxWatermarks
	if maxWatermarks <= 0 {
		maxWatermarks = defaultRuntimeSyncMaxWatermarks
	}
	enabled := options.Enabled
	if enabled == nil {
		enabled = func() bool {
			return true
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	subscriber := &RedisRuntimeSyncEventSubscriber{
		client:              options.Client,
		channel:             channel,
		buffer:              buffer,
		reconnectMinBackoff: reconnectMinBackoff,
		reconnectMaxBackoff: reconnectMaxBackoff,
		maxWatermarks:       maxWatermarks,
		enabled:             enabled,
		onReady:             options.OnReady,
		onEvent:             options.OnEvent,
		onError:             options.OnError,
		ctx:                 ctx,
		cancel:              cancel,
		done:                make(chan struct{}),
		events:              make(chan RuntimeSyncEvent, callbackBuffer),
		watermarks:          map[string]runtimeSyncEventWatermark{},
	}
	go subscriber.dispatchEvents()
	go subscriber.run()
	return subscriber
}

func (s *RedisRuntimeSyncEventSubscriber) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.cancel()
		<-s.done
	})
	return nil
}

func (s *RedisRuntimeSyncEventSubscriber) run() {
	defer close(s.done)
	if s == nil || s.client == nil || !s.enabled() {
		if s != nil {
			close(s.events)
		}
		return
	}
	defer close(s.events)
	backoff := s.reconnectMinBackoff
	for s.ctx.Err() == nil && s.enabled() {
		err := s.subscribeOnce()
		if s.ctx.Err() != nil || !s.enabled() {
			return
		}
		s.reportError(err)
		if !s.waitReconnect(backoff) {
			return
		}
		backoff *= 2
		if backoff > s.reconnectMaxBackoff {
			backoff = s.reconnectMaxBackoff
		}
	}
}

func (s *RedisRuntimeSyncEventSubscriber) subscribeOnce() error {
	pubsub := s.client.Subscribe(s.ctx, s.channel)
	defer func() {
		_ = pubsub.Close()
	}()
	if _, err := pubsub.Receive(s.ctx); err != nil {
		return err
	}
	if s.onReady != nil {
		s.onReady()
	}
	ch := pubsub.Channel(redis.WithChannelSize(s.buffer))
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if msg == nil {
				continue
			}
			if !s.enabled() {
				return nil
			}
			var event RuntimeSyncEvent
			if err := common.UnmarshalJsonStr(msg.Payload, &event); err != nil {
				s.reportError(err)
				continue
			}
			if !s.acceptEvent(event) {
				continue
			}
			s.enqueueEvent(event)
		}
	}
}

func (s *RedisRuntimeSyncEventSubscriber) waitReconnect(delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *RedisRuntimeSyncEventSubscriber) dispatchEvents() {
	if s == nil {
		return
	}
	for event := range s.events {
		if s.onEvent != nil {
			s.onEvent(event)
		}
	}
}

func (s *RedisRuntimeSyncEventSubscriber) enqueueEvent(event RuntimeSyncEvent) {
	if s == nil || s.onEvent == nil {
		return
	}
	select {
	case s.events <- event:
	default:
		s.reportError(ErrRuntimeSyncEventCallbackQueueFull)
	}
}

func (s *RedisRuntimeSyncEventSubscriber) acceptEvent(event RuntimeSyncEvent) bool {
	key := runtimeSyncEventDedupeKey(event)
	eventUpdatedAt := runtimeSyncEventUpdatedAt(event)
	if s == nil || key == "" || eventUpdatedAt == 0 {
		return true
	}
	s.watermarkMu.Lock()
	defer s.watermarkMu.Unlock()
	if s.maxWatermarks > 0 && len(s.watermarks) >= s.maxWatermarks {
		if _, exists := s.watermarks[key]; !exists {
			s.watermarks = map[string]runtimeSyncEventWatermark{}
		}
	}
	previous := s.watermarks[key]
	if previous.updatedAt >= eventUpdatedAt {
		return false
	}
	previous.updatedAt = eventUpdatedAt
	s.watermarks[key] = previous
	return true
}

func runtimeSyncEventUpdatedAt(event RuntimeSyncEvent) int64 {
	if event.UpdatedAtNano > 0 {
		return event.UpdatedAtNano
	}
	return event.UpdatedAt
}

func runtimeSyncEventDedupeKey(event RuntimeSyncEvent) string {
	if event.Kind == "" {
		return ""
	}
	key := event.CacheKey
	if key == "" {
		key = event.NodeID
	}
	if key == "" && event.RuntimeKey.ChannelID > 0 {
		key = runtimeKeyCacheKey(event.RuntimeKey)
	}
	if key == "" {
		return ""
	}
	return event.Kind + "|" + key
}

func (s *RedisRuntimeSyncEventSubscriber) reportError(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	if s != nil && s.onError != nil {
		s.onError(err)
	}
}

var _ RuntimeSyncEventPublisher = (*RedisRuntimeSyncEventPublisher)(nil)
var _ RuntimeSyncEventSubscriber = (*RedisRuntimeSyncEventSubscriber)(nil)
