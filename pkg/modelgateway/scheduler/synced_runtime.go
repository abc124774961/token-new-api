package scheduler

import (
	"sort"

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
			snapshots[snapshot.Key] = snapshot
		}
	}
	for _, snapshot := range s.remoteSnapshots(req) {
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
	local := core.CircuitSnapshot{Key: key, State: core.CircuitStateClosed}
	if b.local != nil {
		local = b.local.Snapshot(key)
	}
	remote, ok := b.remoteSnapshot(key)
	if !ok {
		return local
	}
	return moreSevereCircuitSnapshot(local, remote)
}

func (b *SyncedCircuitBreaker) ListSnapshots() []core.CircuitSnapshot {
	if b == nil {
		return nil
	}
	snapshots := make(map[core.RuntimeKey]core.CircuitSnapshot)
	if b.local != nil {
		for _, snapshot := range b.local.ListSnapshots() {
			snapshots[snapshot.Key] = snapshot
		}
	}
	if b.sync != nil {
		for _, snapshot := range b.sync.ListCircuits() {
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
	return b.local.AllowProbe(key)
}

func (b *SyncedCircuitBreaker) Report(result core.AttemptResult) {
	if b == nil {
		return
	}
	if b.local != nil {
		b.local.Report(result)
	}
	if b.sync == nil {
		return
	}
	snapshot := b.Snapshot(result.RuntimeKey())
	if snapshot.Key.ChannelID > 0 {
		b.sync.PutCircuit(snapshot)
	}
}

func (b *SyncedCircuitBreaker) remoteSnapshot(key core.RuntimeKey) (core.CircuitSnapshot, bool) {
	if b == nil || b.sync == nil || key.ChannelID <= 0 {
		return core.CircuitSnapshot{}, false
	}
	return b.sync.GetCircuit(key)
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
