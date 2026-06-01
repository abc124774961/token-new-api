package scheduler

import (
	"sort"
	"time"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

type SyncedRuntimeSnapshotStore struct {
	local *MemoryRuntimeSnapshotStore
	sync  RuntimeSyncStore
}

func NewSyncedRuntimeSnapshotStore(local *MemoryRuntimeSnapshotStore, syncStore RuntimeSyncStore) *SyncedRuntimeSnapshotStore {
	if local == nil {
		local = NewMemoryRuntimeSnapshotStore()
	}
	return &SyncedRuntimeSnapshotStore{
		local: local,
		sync:  syncStore,
	}
}

func (s *SyncedRuntimeSnapshotStore) Get(key core.RuntimeKey) (core.RuntimeSnapshot, bool) {
	if s == nil {
		return core.RuntimeSnapshot{}, false
	}
	key = normalizeRuntimeKey(key)
	if s.local != nil {
		if snapshot, ok := s.local.Get(key); ok {
			return snapshot, true
		}
	}
	for _, snapshot := range s.remoteSnapshots(&core.DispatchRequest{ModelName: key.RequestedModel}) {
		if snapshot.Key == key {
			return snapshot, true
		}
	}
	return core.RuntimeSnapshot{}, false
}

func (s *SyncedRuntimeSnapshotStore) Put(snapshot core.RuntimeSnapshot) {
	if s == nil {
		return
	}
	snapshot = normalizeRuntimeSnapshot(snapshot)
	if s.local != nil {
		s.local.Put(snapshot)
	}
	if s.sync != nil {
		s.sync.PutSnapshot(snapshot)
	}
}

func (s *SyncedRuntimeSnapshotStore) ListCandidates(req *core.DispatchRequest) []core.RuntimeSnapshot {
	if s == nil {
		return nil
	}
	snapshots := make(map[core.RuntimeKey]core.RuntimeSnapshot)
	if s.local != nil {
		for _, snapshot := range s.local.ListCandidates(req) {
			snapshot = normalizeRuntimeSnapshot(snapshot)
			snapshots[snapshot.Key] = snapshot
		}
	}
	for _, snapshot := range s.remoteSnapshots(req) {
		snapshot = normalizeRuntimeSnapshot(snapshot)
		if current, ok := snapshots[snapshot.Key]; ok && current.SampleCount >= snapshot.SampleCount {
			continue
		}
		snapshots[snapshot.Key] = snapshot
	}
	out := make([]core.RuntimeSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, snapshot)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key.ChannelID != out[j].Key.ChannelID {
			return out[i].Key.ChannelID < out[j].Key.ChannelID
		}
		if out[i].Key.RequestedModel != out[j].Key.RequestedModel {
			return out[i].Key.RequestedModel < out[j].Key.RequestedModel
		}
		return out[i].Key.Group < out[j].Key.Group
	})
	return out
}

func (s *SyncedRuntimeSnapshotStore) remoteSnapshots(req *core.DispatchRequest) []core.RuntimeSnapshot {
	if s == nil || s.sync == nil {
		return nil
	}
	return s.sync.ListSnapshots(req)
}

type SyncedCircuitBreaker struct {
	local *CircuitBreaker
	sync  RuntimeSyncStore
}

func NewSyncedCircuitBreaker(local *CircuitBreaker, syncStore RuntimeSyncStore) *SyncedCircuitBreaker {
	return &SyncedCircuitBreaker{
		local: local,
		sync:  syncStore,
	}
}

func (b *SyncedCircuitBreaker) Snapshot(key core.RuntimeKey) core.CircuitSnapshot {
	if b == nil {
		return core.CircuitSnapshot{Key: key, State: core.CircuitStateClosed}
	}
	key = normalizeRuntimeKey(key)
	local := core.CircuitSnapshot{Key: key, State: core.CircuitStateClosed}
	if b.local != nil {
		local = b.local.Snapshot(key)
	}
	remote, ok := b.remoteSnapshot(key)
	if !ok {
		return local
	}
	selected := moreSevereCircuitSnapshot(local, remote)
	if b.local != nil && selected.State != core.CircuitStateClosed && circuitSnapshotSameKey(selected, remote) {
		b.local.applySnapshot(selected)
	}
	return selected
}

func (b *SyncedCircuitBreaker) ListSnapshots() []core.CircuitSnapshot {
	if b == nil {
		return nil
	}
	snapshots := make(map[core.RuntimeKey]core.CircuitSnapshot)
	if b.local != nil {
		for _, snapshot := range b.local.ListSnapshots() {
			snapshot = normalizeCircuitSnapshot(snapshot)
			snapshots[snapshot.Key] = snapshot
		}
	}
	if b.sync != nil {
		for _, snapshot := range b.sync.ListCircuits() {
			snapshot, _ = b.normalizeRemoteSnapshot(snapshot)
			current, ok := snapshots[snapshot.Key]
			if !ok {
				snapshots[snapshot.Key] = snapshot
				continue
			}
			snapshots[snapshot.Key] = moreSevereCircuitSnapshot(current, snapshot)
		}
	}
	out := make([]core.CircuitSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		out = append(out, snapshot)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Key.ChannelID != out[j].Key.ChannelID {
			return out[i].Key.ChannelID < out[j].Key.ChannelID
		}
		return out[i].Key.RequestedModel < out[j].Key.RequestedModel
	})
	return out
}

func (b *SyncedCircuitBreaker) AllowProbe(key core.RuntimeKey) bool {
	if b == nil || b.local == nil {
		return true
	}
	snapshot := b.Snapshot(key)
	if snapshot.State == core.CircuitStateOpen {
		return false
	}
	allowed := b.local.AllowProbe(key)
	if allowed && b.sync != nil {
		b.sync.PutCircuit(b.local.Snapshot(key))
	}
	return allowed
}

func (b *SyncedCircuitBreaker) Report(result core.AttemptResult) {
	if b == nil {
		return
	}
	result.Key = normalizeRuntimeKey(result.RuntimeKey())
	if b.local != nil {
		b.local.Report(result)
	}
	if b.sync == nil {
		return
	}
	snapshot := core.CircuitSnapshot{Key: result.RuntimeKey(), State: core.CircuitStateClosed}
	if b.local != nil {
		snapshot = b.local.Snapshot(result.RuntimeKey())
	} else {
		snapshot = b.Snapshot(result.RuntimeKey())
	}
	if snapshot.Key.ChannelID > 0 {
		b.sync.PutCircuit(snapshot)
	}
}

func (b *SyncedCircuitBreaker) Reset(key core.RuntimeKey) bool {
	if b == nil {
		return false
	}
	key = normalizeRuntimeKey(key)
	if key.ChannelID <= 0 {
		return false
	}
	changed := false
	if b.local != nil && b.local.Reset(key) {
		changed = true
	}
	if b.sync != nil {
		if snapshot, ok := b.sync.GetCircuit(key); ok && circuitSnapshotNeedsReset(snapshot) {
			changed = true
		}
		b.sync.PutCircuit(core.CircuitSnapshot{Key: key, State: core.CircuitStateClosed})
	}
	return changed
}

func (b *SyncedCircuitBreaker) ResetChannel(channelID int) int {
	if b == nil || channelID <= 0 {
		return 0
	}
	keys := map[core.RuntimeKey]struct{}{}
	if b.local != nil {
		for _, snapshot := range b.local.ListSnapshots() {
			snapshot = normalizeCircuitSnapshot(snapshot)
			if snapshot.Key.ChannelID == channelID && circuitSnapshotNeedsReset(snapshot) {
				keys[snapshot.Key] = struct{}{}
			}
		}
		_ = b.local.ResetChannel(channelID)
	}
	if b.sync != nil {
		for _, snapshot := range b.sync.ListCircuits() {
			snapshot = normalizeCircuitSnapshot(snapshot)
			if snapshot.Key.ChannelID == channelID && circuitSnapshotNeedsReset(snapshot) {
				keys[snapshot.Key] = struct{}{}
			}
		}
		for key := range keys {
			b.sync.PutCircuit(core.CircuitSnapshot{Key: key, State: core.CircuitStateClosed})
		}
	}
	return len(keys)
}

func (b *SyncedCircuitBreaker) remoteSnapshot(key core.RuntimeKey) (core.CircuitSnapshot, bool) {
	key = normalizeRuntimeKey(key)
	if b == nil || b.sync == nil || key.ChannelID <= 0 {
		return core.CircuitSnapshot{}, false
	}
	snapshot, ok := b.sync.GetCircuit(key)
	if !ok {
		return core.CircuitSnapshot{}, false
	}
	snapshot, _ = b.normalizeRemoteSnapshot(snapshot)
	return snapshot, true
}

func (b *SyncedCircuitBreaker) normalizeRemoteSnapshot(snapshot core.CircuitSnapshot) (core.CircuitSnapshot, bool) {
	snapshot = normalizeCircuitSnapshot(snapshot)
	changed := false
	if snapshot.State == "" {
		snapshot.State = core.CircuitStateClosed
		changed = true
	}
	if snapshot.State == core.CircuitStateOpen && !snapshot.OpenUntil.IsZero() && !snapshot.OpenUntil.After(b.now()) {
		snapshot.State = core.CircuitStateHalfOpen
		snapshot.HalfOpenProbeUsed = 0
		if snapshot.HalfOpenProbeMax <= 0 {
			snapshot.HalfOpenProbeMax = b.defaultProbeCount()
		}
		changed = true
	}
	if changed && b.sync != nil && snapshot.Key.ChannelID > 0 {
		b.sync.PutCircuit(snapshot)
	}
	return snapshot, changed
}

func (b *SyncedCircuitBreaker) now() time.Time {
	if b != nil && b.local != nil && b.local.now != nil {
		return b.local.now()
	}
	return time.Now()
}

func (b *SyncedCircuitBreaker) defaultProbeCount() int {
	if b != nil && b.local != nil && b.local.options.HalfOpenProbeCount > 0 {
		return b.local.options.HalfOpenProbeCount
	}
	return defaultCircuitProbeCount
}

func circuitSnapshotSameKey(left core.CircuitSnapshot, right core.CircuitSnapshot) bool {
	return normalizeRuntimeKey(left.Key) == normalizeRuntimeKey(right.Key)
}

func circuitSnapshotNeedsReset(snapshot core.CircuitSnapshot) bool {
	snapshot = normalizeCircuitSnapshot(snapshot)
	return snapshot.State != "" && snapshot.State != core.CircuitStateClosed ||
		snapshot.FailureCount > 0 ||
		snapshot.SuccessCount > 0 ||
		snapshot.SampleCount > 0 ||
		snapshot.OpenReason != "" ||
		len(snapshot.ErrorCounts) > 0 ||
		!snapshot.OpenUntil.IsZero() ||
		snapshot.HalfOpenProbeUsed > 0
}

func moreSevereCircuitSnapshot(left core.CircuitSnapshot, right core.CircuitSnapshot) core.CircuitSnapshot {
	if circuitSeverity(right.State) > circuitSeverity(left.State) {
		return right
	}
	if circuitSeverity(right.State) < circuitSeverity(left.State) {
		return left
	}
	if right.FailureRate > left.FailureRate {
		return right
	}
	if right.SampleCount > left.SampleCount {
		return right
	}
	return left
}

func circuitSeverity(state core.CircuitState) int {
	switch state {
	case core.CircuitStateOpen:
		return 3
	case core.CircuitStateHalfOpen:
		return 2
	default:
		return 1
	}
}

var _ core.RuntimeSnapshotStore = (*SyncedRuntimeSnapshotStore)(nil)
var _ core.CircuitBreaker = (*SyncedCircuitBreaker)(nil)
