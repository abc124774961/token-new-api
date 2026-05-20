package scheduler

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	defaultRuntimeSyncEventFlushInterval = 500 * time.Millisecond
	defaultRuntimeSyncEventQueueSize     = 1024
)

type RuntimeSyncEventStoreOptions struct {
	Store         RuntimeSyncStore
	FlushInterval time.Duration
	QueueSize     int
	Publisher     RuntimeSyncEventPublisher
}

type runtimeSyncEventKind string

const (
	runtimeSyncEventSnapshot runtimeSyncEventKind = "snapshot"
	runtimeSyncEventCircuit  runtimeSyncEventKind = "circuit"
	runtimeSyncEventQueue    runtimeSyncEventKind = "queue"
)

type runtimeSyncEvent struct {
	kind     runtimeSyncEventKind
	cacheKey string
	version  uint64
	nodeID   string
	snapshot core.RuntimeSnapshot
	circuit  core.CircuitSnapshot
	queue    core.RuntimeQueueSnapshot
}

type RuntimeSyncEventStore struct {
	store         RuntimeSyncStore
	flushInterval time.Duration
	queueSize     int
	publisher     RuntimeSyncEventPublisher

	mu      sync.Mutex
	pending map[string]runtimeSyncEvent
	version uint64
	closed  bool
	flushMu sync.Mutex

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func NewRuntimeSyncEventStore(options RuntimeSyncEventStoreOptions) *RuntimeSyncEventStore {
	flushInterval := options.FlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultRuntimeSyncEventFlushInterval
	}
	queueSize := options.QueueSize
	if queueSize <= 0 {
		queueSize = defaultRuntimeSyncEventQueueSize
	}
	store := &RuntimeSyncEventStore{
		store:         options.Store,
		flushInterval: flushInterval,
		queueSize:     queueSize,
		publisher:     options.Publisher,
		pending:       map[string]runtimeSyncEvent{},
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
	}
	go store.run()
	return store
}

func (s *RuntimeSyncEventStore) PutSnapshot(snapshot core.RuntimeSnapshot) {
	if s == nil || s.store == nil || snapshot.Key.ChannelID <= 0 {
		return
	}
	event := runtimeSyncEvent{
		kind:     runtimeSyncEventSnapshot,
		cacheKey: string(runtimeSyncEventSnapshot) + "|" + runtimeKeyCacheKey(snapshot.Key),
		snapshot: snapshot,
	}
	s.enqueueOrFallback(event)
}

func (s *RuntimeSyncEventStore) ListSnapshots(req *core.DispatchRequest) []core.RuntimeSnapshot {
	if s == nil || s.store == nil {
		return nil
	}
	snapshots := s.store.ListSnapshots(req)
	pending := s.pendingEvents(runtimeSyncEventSnapshot)
	if len(pending) == 0 {
		return snapshots
	}
	byKey := make(map[string]int, len(snapshots)+len(pending))
	for i, snapshot := range snapshots {
		byKey[runtimeKeyCacheKey(snapshot.Key)] = i
	}
	for _, event := range pending {
		snapshot := event.snapshot
		if req != nil && req.ModelName != "" && snapshot.Key.RequestedModel != "" && snapshot.Key.RequestedModel != req.ModelName {
			continue
		}
		key := runtimeKeyCacheKey(snapshot.Key)
		if index, ok := byKey[key]; ok {
			snapshots[index] = snapshot
			continue
		}
		byKey[key] = len(snapshots)
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *RuntimeSyncEventStore) PutCircuit(snapshot core.CircuitSnapshot) {
	if s == nil || s.store == nil || snapshot.Key.ChannelID <= 0 {
		return
	}
	event := runtimeSyncEvent{
		kind:     runtimeSyncEventCircuit,
		cacheKey: string(runtimeSyncEventCircuit) + "|" + runtimeKeyCacheKey(snapshot.Key),
		circuit:  snapshot,
	}
	s.enqueueOrFallback(event)
}

func (s *RuntimeSyncEventStore) GetCircuit(key core.RuntimeKey) (core.CircuitSnapshot, bool) {
	if s == nil || s.store == nil || key.ChannelID <= 0 {
		return core.CircuitSnapshot{}, false
	}
	cacheKey := string(runtimeSyncEventCircuit) + "|" + runtimeKeyCacheKey(key)
	s.mu.Lock()
	event, ok := s.pending[cacheKey]
	s.mu.Unlock()
	if ok {
		return event.circuit, true
	}
	return s.store.GetCircuit(key)
}

func (s *RuntimeSyncEventStore) ListCircuits() []core.CircuitSnapshot {
	if s == nil || s.store == nil {
		return nil
	}
	circuits := s.store.ListCircuits()
	pending := s.pendingEvents(runtimeSyncEventCircuit)
	if len(pending) == 0 {
		return circuits
	}
	byKey := make(map[string]int, len(circuits)+len(pending))
	for i, snapshot := range circuits {
		byKey[runtimeKeyCacheKey(snapshot.Key)] = i
	}
	for _, event := range pending {
		key := runtimeKeyCacheKey(event.circuit.Key)
		if index, ok := byKey[key]; ok {
			circuits[index] = event.circuit
			continue
		}
		byKey[key] = len(circuits)
		circuits = append(circuits, event.circuit)
	}
	return circuits
}

func (s *RuntimeSyncEventStore) PutQueueSnapshot(nodeID string, snapshot core.RuntimeQueueSnapshot) {
	if s == nil || s.store == nil {
		return
	}
	nodeID = sanitizeRuntimeKeyPart(nodeID)
	if nodeID == "_" {
		nodeID = defaultRuntimeQueueNodeID
	}
	snapshot.NodeID = nodeID
	if snapshot.UpdatedAt == 0 {
		snapshot.UpdatedAt = time.Now().Unix()
	}
	if snapshot.Summary.UpdatedAt == 0 {
		snapshot.Summary.UpdatedAt = snapshot.UpdatedAt
	}
	event := runtimeSyncEvent{
		kind:     runtimeSyncEventQueue,
		cacheKey: string(runtimeSyncEventQueue) + "|" + nodeID,
		nodeID:   nodeID,
		queue:    snapshot,
	}
	s.enqueueOrFallback(event)
}

func (s *RuntimeSyncEventStore) ListQueueSnapshots() []core.RuntimeQueueSnapshot {
	if s == nil || s.store == nil {
		return nil
	}
	snapshots := s.store.ListQueueSnapshots()
	pending := s.pendingEvents(runtimeSyncEventQueue)
	if len(pending) == 0 {
		return snapshots
	}
	byNode := make(map[string]int, len(snapshots)+len(pending))
	for i, snapshot := range snapshots {
		nodeID := sanitizeRuntimeKeyPart(snapshot.NodeID)
		if nodeID == "_" {
			nodeID = defaultRuntimeQueueNodeID
		}
		byNode[nodeID] = i
	}
	for _, event := range pending {
		snapshot := event.queue
		snapshot.NodeID = event.nodeID
		if index, ok := byNode[event.nodeID]; ok {
			snapshots[index] = snapshot
			continue
		}
		byNode[event.nodeID] = len(snapshots)
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *RuntimeSyncEventStore) Flush() {
	if s == nil || s.store == nil {
		return
	}
	s.flushMu.Lock()
	defer s.flushMu.Unlock()
	events := s.flushCandidates()
	for _, event := range events {
		s.flushEvent(event)
		s.publishEvent(event)
		s.completeFlush(event)
	}
}

func (s *RuntimeSyncEventStore) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		close(s.stop)
		<-s.done
	})
}

func (s *RuntimeSyncEventStore) run() {
	ticker := time.NewTicker(s.flushInterval)
	defer ticker.Stop()
	defer close(s.done)
	for {
		select {
		case <-ticker.C:
			s.Flush()
		case <-s.stop:
			s.Flush()
			return
		}
	}
}

func (s *RuntimeSyncEventStore) enqueueOrFallback(event runtimeSyncEvent) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.flushEvent(event)
		return
	}
	if s.queueSize > 0 && len(s.pending) >= s.queueSize {
		if _, exists := s.pending[event.cacheKey]; !exists {
			s.mu.Unlock()
			s.flushEvent(event)
			return
		}
	}
	s.version++
	event.version = s.version
	s.pending[event.cacheKey] = event
	s.mu.Unlock()
}

func (s *RuntimeSyncEventStore) pendingEvents(kind runtimeSyncEventKind) []runtimeSyncEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	events := make([]runtimeSyncEvent, 0, len(s.pending))
	for _, event := range s.pending {
		if event.kind == kind {
			events = append(events, event)
		}
	}
	return events
}

func (s *RuntimeSyncEventStore) flushCandidates() []runtimeSyncEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	events := make([]runtimeSyncEvent, 0, len(s.pending))
	for _, event := range s.pending {
		events = append(events, event)
	}
	return events
}

func (s *RuntimeSyncEventStore) completeFlush(event runtimeSyncEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.pending[event.cacheKey]
	if ok && current.version == event.version {
		delete(s.pending, event.cacheKey)
	}
}

func (s *RuntimeSyncEventStore) flushEvent(event runtimeSyncEvent) {
	if s == nil || s.store == nil {
		return
	}
	switch event.kind {
	case runtimeSyncEventSnapshot:
		s.store.PutSnapshot(event.snapshot)
	case runtimeSyncEventCircuit:
		s.store.PutCircuit(event.circuit)
	case runtimeSyncEventQueue:
		s.store.PutQueueSnapshot(event.nodeID, event.queue)
	}
}

func (s *RuntimeSyncEventStore) publishEvent(event runtimeSyncEvent) {
	if s == nil || s.publisher == nil {
		return
	}
	now := time.Now()
	_ = s.publisher.Publish(RuntimeSyncEvent{
		Kind:          string(event.kind),
		CacheKey:      event.cacheKey,
		NodeID:        event.nodeID,
		RuntimeKey:    runtimeSyncEventRuntimeKey(event),
		UpdatedAt:     now.Unix(),
		UpdatedAtNano: now.UnixNano(),
	})
}

func runtimeSyncEventRuntimeKey(event runtimeSyncEvent) core.RuntimeKey {
	switch event.kind {
	case runtimeSyncEventSnapshot:
		return event.snapshot.Key
	case runtimeSyncEventCircuit:
		return event.circuit.Key
	case runtimeSyncEventQueue:
		if len(event.queue.RuntimeKeys) > 0 {
			return event.queue.RuntimeKeys[0].RuntimeKey
		}
	}
	return core.RuntimeKey{}
}

var _ RuntimeSyncStore = (*RuntimeSyncEventStore)(nil)
