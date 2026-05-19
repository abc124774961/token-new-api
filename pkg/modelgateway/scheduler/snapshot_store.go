package scheduler

import (
	"sync"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

type MemoryRuntimeSnapshotStore struct {
	mu        sync.RWMutex
	snapshots map[core.RuntimeKey]core.RuntimeSnapshot
}

func NewMemoryRuntimeSnapshotStore() *MemoryRuntimeSnapshotStore {
	return &MemoryRuntimeSnapshotStore{
		snapshots: make(map[core.RuntimeKey]core.RuntimeSnapshot),
	}
}

func (s *MemoryRuntimeSnapshotStore) Get(key core.RuntimeKey) (core.RuntimeSnapshot, bool) {
	if s == nil {
		return core.RuntimeSnapshot{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.snapshots[key]
	return snapshot, ok
}

func (s *MemoryRuntimeSnapshotStore) Put(snapshot core.RuntimeSnapshot) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshots[snapshot.Key] = snapshot
}

func (s *MemoryRuntimeSnapshotStore) ListCandidates(req *core.DispatchRequest) []core.RuntimeSnapshot {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]core.RuntimeSnapshot, 0, len(s.snapshots))
	for _, snapshot := range s.snapshots {
		if req != nil && req.ModelName != "" && snapshot.Key.RequestedModel != "" && snapshot.Key.RequestedModel != req.ModelName {
			continue
		}
		result = append(result, snapshot)
	}
	return result
}

var _ core.RuntimeSnapshotStore = (*MemoryRuntimeSnapshotStore)(nil)
