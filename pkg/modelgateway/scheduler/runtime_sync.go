package scheduler

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/samber/hot"
)

const (
	runtimeSyncSnapshotNamespace = "new-api:modelgateway:runtime_snapshot:v1"
	runtimeSyncCircuitNamespace  = "new-api:modelgateway:circuit:v1"
	runtimeSyncQueueNamespace    = "new-api:modelgateway:queue:v1"
	defaultRuntimeSyncTTL        = 90 * time.Second
	defaultRuntimeSyncMaxEntries = 100000
)

type RuntimeSyncStore interface {
	PutSnapshot(snapshot core.RuntimeSnapshot)
	ListSnapshots(req *core.DispatchRequest) []core.RuntimeSnapshot
	PutCircuit(snapshot core.CircuitSnapshot)
	GetCircuit(key core.RuntimeKey) (core.CircuitSnapshot, bool)
	ListCircuits() []core.CircuitSnapshot
	PutQueueSnapshot(nodeID string, snapshot core.RuntimeQueueSnapshot)
	ListQueueSnapshots() []core.RuntimeQueueSnapshot
}

type RuntimeSyncStoreOptions struct {
	TTL          time.Duration
	MaxEntries   int
	RedisEnabled func() bool
}

type RuntimeSnapshotCodec struct{}

func (c RuntimeSnapshotCodec) Encode(v core.RuntimeSnapshot) (string, error) {
	data, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c RuntimeSnapshotCodec) Decode(s string) (core.RuntimeSnapshot, error) {
	var snapshot core.RuntimeSnapshot
	if err := common.UnmarshalJsonStr(s, &snapshot); err != nil {
		return core.RuntimeSnapshot{}, err
	}
	return snapshot, nil
}

type CircuitSnapshotCodec struct{}

func (c CircuitSnapshotCodec) Encode(v core.CircuitSnapshot) (string, error) {
	data, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c CircuitSnapshotCodec) Decode(s string) (core.CircuitSnapshot, error) {
	var snapshot core.CircuitSnapshot
	if err := common.UnmarshalJsonStr(s, &snapshot); err != nil {
		return core.CircuitSnapshot{}, err
	}
	return snapshot, nil
}

type QueueSnapshotCodec struct{}

func (c QueueSnapshotCodec) Encode(v core.RuntimeQueueSnapshot) (string, error) {
	data, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c QueueSnapshotCodec) Decode(s string) (core.RuntimeQueueSnapshot, error) {
	var snapshot core.RuntimeQueueSnapshot
	if err := common.UnmarshalJsonStr(s, &snapshot); err != nil {
		return core.RuntimeQueueSnapshot{}, err
	}
	return snapshot, nil
}

type HybridRuntimeSyncStore struct {
	ttl       time.Duration
	snapshots *cachex.HybridCache[core.RuntimeSnapshot]
	circuits  *cachex.HybridCache[core.CircuitSnapshot]
	queues    *cachex.HybridCache[core.RuntimeQueueSnapshot]
}

func NewHybridRuntimeSyncStore(options RuntimeSyncStoreOptions) *HybridRuntimeSyncStore {
	ttl := options.TTL
	if ttl <= 0 {
		ttl = defaultRuntimeSyncTTL
	}
	maxEntries := options.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultRuntimeSyncMaxEntries
	}
	redisEnabled := options.RedisEnabled
	if redisEnabled == nil {
		redisEnabled = func() bool {
			return common.RedisEnabled && common.RDB != nil
		}
	}
	redisAvailable := func() bool {
		return common.RedisEnabled && common.RDB != nil
	}
	redisEnabled = combineRuntimeSyncRedisEnabled(redisAvailable, redisEnabled)
	return &HybridRuntimeSyncStore{
		ttl: ttl,
		snapshots: cachex.NewHybridCache[core.RuntimeSnapshot](cachex.HybridCacheConfig[core.RuntimeSnapshot]{
			Namespace:    cachex.Namespace(runtimeSyncSnapshotNamespace),
			Redis:        common.RDB,
			RedisCodec:   RuntimeSnapshotCodec{},
			RedisEnabled: redisEnabled,
			Memory: func() *hot.HotCache[string, core.RuntimeSnapshot] {
				return hot.NewHotCache[string, core.RuntimeSnapshot](hot.LRU, maxEntries).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		}),
		circuits: cachex.NewHybridCache[core.CircuitSnapshot](cachex.HybridCacheConfig[core.CircuitSnapshot]{
			Namespace:    cachex.Namespace(runtimeSyncCircuitNamespace),
			Redis:        common.RDB,
			RedisCodec:   CircuitSnapshotCodec{},
			RedisEnabled: redisEnabled,
			Memory: func() *hot.HotCache[string, core.CircuitSnapshot] {
				return hot.NewHotCache[string, core.CircuitSnapshot](hot.LRU, maxEntries).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		}),
		queues: cachex.NewHybridCache[core.RuntimeQueueSnapshot](cachex.HybridCacheConfig[core.RuntimeQueueSnapshot]{
			Namespace:    cachex.Namespace(runtimeSyncQueueNamespace),
			Redis:        common.RDB,
			RedisCodec:   QueueSnapshotCodec{},
			RedisEnabled: redisEnabled,
			Memory: func() *hot.HotCache[string, core.RuntimeQueueSnapshot] {
				return hot.NewHotCache[string, core.RuntimeQueueSnapshot](hot.LRU, maxEntries).
					WithTTL(ttl).
					WithJanitor().
					Build()
			},
		}),
	}
}

func combineRuntimeSyncRedisEnabled(defaultEnabled func() bool, configured func() bool) func() bool {
	return func() bool {
		if defaultEnabled != nil && !defaultEnabled() {
			return false
		}
		if configured == nil {
			return true
		}
		return configured()
	}
}

func (s *HybridRuntimeSyncStore) PutSnapshot(snapshot core.RuntimeSnapshot) {
	if s == nil || s.snapshots == nil || snapshot.Key.ChannelID <= 0 {
		return
	}
	_ = s.snapshots.SetWithTTL(runtimeKeyCacheKey(snapshot.Key), snapshot, s.ttl)
}

func (s *HybridRuntimeSyncStore) ListSnapshots(req *core.DispatchRequest) []core.RuntimeSnapshot {
	if s == nil || s.snapshots == nil {
		return nil
	}
	keys, err := s.snapshots.Keys()
	if err != nil {
		return nil
	}
	snapshots := make([]core.RuntimeSnapshot, 0, len(keys))
	for _, key := range keys {
		snapshot, found, err := s.snapshots.Get(key)
		if err != nil || !found {
			continue
		}
		if req != nil && req.ModelName != "" && snapshot.Key.RequestedModel != "" && snapshot.Key.RequestedModel != req.ModelName {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *HybridRuntimeSyncStore) PutCircuit(snapshot core.CircuitSnapshot) {
	if s == nil || s.circuits == nil || snapshot.Key.ChannelID <= 0 {
		return
	}
	_ = s.circuits.SetWithTTL(runtimeKeyCacheKey(snapshot.Key), snapshot, s.ttl)
}

func (s *HybridRuntimeSyncStore) GetCircuit(key core.RuntimeKey) (core.CircuitSnapshot, bool) {
	if s == nil || s.circuits == nil || key.ChannelID <= 0 {
		return core.CircuitSnapshot{}, false
	}
	snapshot, found, err := s.circuits.Get(runtimeKeyCacheKey(key))
	if err != nil || !found {
		return core.CircuitSnapshot{}, false
	}
	return snapshot, true
}

func (s *HybridRuntimeSyncStore) ListCircuits() []core.CircuitSnapshot {
	if s == nil || s.circuits == nil {
		return nil
	}
	keys, err := s.circuits.Keys()
	if err != nil {
		return nil
	}
	snapshots := make([]core.CircuitSnapshot, 0, len(keys))
	for _, key := range keys {
		snapshot, found, err := s.circuits.Get(key)
		if err != nil || !found {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (s *HybridRuntimeSyncStore) PutQueueSnapshot(nodeID string, snapshot core.RuntimeQueueSnapshot) {
	if s == nil || s.queues == nil {
		return
	}
	nodeID = sanitizeRuntimeKeyPart(nodeID)
	if nodeID == "_" {
		nodeID = "local"
	}
	snapshot.NodeID = nodeID
	if snapshot.UpdatedAt == 0 {
		snapshot.UpdatedAt = time.Now().Unix()
	}
	if snapshot.Summary.UpdatedAt == 0 {
		snapshot.Summary.UpdatedAt = snapshot.UpdatedAt
	}
	_ = s.queues.SetWithTTL(nodeID, snapshot, s.ttl)
}

func (s *HybridRuntimeSyncStore) ListQueueSnapshots() []core.RuntimeQueueSnapshot {
	if s == nil || s.queues == nil {
		return nil
	}
	keys, err := s.queues.Keys()
	if err != nil {
		return nil
	}
	snapshots := make([]core.RuntimeQueueSnapshot, 0, len(keys))
	for _, key := range keys {
		snapshot, found, err := s.queues.Get(key)
		if err != nil || !found {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func runtimeKeyCacheKey(key core.RuntimeKey) string {
	parts := []string{
		key.RequestedModel,
		key.UpstreamModel,
		key.Group,
		string(key.EndpointType),
		key.CapabilityFingerprint,
		strconv.Itoa(key.ChannelID),
	}
	for i, part := range parts {
		parts[i] = sanitizeRuntimeKeyPart(part)
	}
	return strings.Join(parts, "|")
}

func sanitizeRuntimeKeyPart(part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return "_"
	}
	part = strings.ReplaceAll(part, "|", "_")
	part = strings.ReplaceAll(part, ":", "_")
	return part
}

var _ RuntimeSyncStore = (*HybridRuntimeSyncStore)(nil)
