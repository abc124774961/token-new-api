package testkit

import (
	"context"
	"sync"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type StaticSettingsProvider struct {
	Settings core.SchedulerSettings
}

func (p StaticSettingsProvider) Get() core.SchedulerSettings {
	return p.Settings
}

type FakeGroupPermissionService struct {
	UsableGroups map[string]map[string]string
	AutoGroups   map[string][]string
}

func (s *FakeGroupPermissionService) GetUserUsableGroups(userGroup string) map[string]string {
	groups := s.UsableGroups[userGroup]
	if groups == nil {
		groups = map[string]string{}
	}
	if _, ok := groups[userGroup]; userGroup != "" && !ok {
		copied := copyStringMap(groups)
		copied[userGroup] = "用户分组"
		return copied
	}
	return copyStringMap(groups)
}

func (s *FakeGroupPermissionService) GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := s.GetUserUsableGroups(userGroup)[groupName]
	return ok
}

func (s *FakeGroupPermissionService) GetUserAutoGroup(userGroup string) []string {
	return append([]string(nil), s.AutoGroups[userGroup]...)
}

type FakeLegacyChannelSelector struct {
	Channel *model.Channel
	Group   string
	Err     error
	Calls   int
}

func (s *FakeLegacyChannelSelector) Select(param *service.RetryParam) (*model.Channel, string, error) {
	s.Calls++
	return s.Channel, s.Group, s.Err
}

type FakeSmartChannelSelector struct {
	Plan    *core.DispatchPlan
	Handled bool
	Err     *types.NewAPIError
	Calls   int
}

func (s *FakeSmartChannelSelector) Select(c *gin.Context, param *service.RetryParam, policy core.GroupSmartPolicy) (*core.DispatchPlan, bool, *types.NewAPIError) {
	s.Calls++
	return s.Plan, s.Handled, s.Err
}

type FakeExecutionRecorder struct {
	mu      sync.Mutex
	Records []core.DispatchRecord
	Results []core.AttemptResult
}

func (r *FakeExecutionRecorder) Record(ctx context.Context, record core.DispatchRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Records = append(r.Records, record)
}

func (r *FakeExecutionRecorder) Report(ctx context.Context, result core.AttemptResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Results = append(r.Results, result)
}

func (r *FakeExecutionRecorder) SnapshotRecords() []core.DispatchRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]core.DispatchRecord(nil), r.Records...)
}

func (r *FakeExecutionRecorder) SnapshotResults() []core.AttemptResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]core.AttemptResult(nil), r.Results...)
}

type FakeRuntimeSnapshotStore struct {
	mu        sync.RWMutex
	snapshots map[core.RuntimeKey]core.RuntimeSnapshot
}

func NewFakeRuntimeSnapshotStore() *FakeRuntimeSnapshotStore {
	return &FakeRuntimeSnapshotStore{snapshots: map[core.RuntimeKey]core.RuntimeSnapshot{}}
}

func (s *FakeRuntimeSnapshotStore) Get(key core.RuntimeKey) (core.RuntimeSnapshot, bool) {
	if s == nil {
		return core.RuntimeSnapshot{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	snapshot, ok := s.snapshots[key]
	return snapshot, ok
}

func (s *FakeRuntimeSnapshotStore) Put(snapshot core.RuntimeSnapshot) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.snapshots == nil {
		s.snapshots = map[core.RuntimeKey]core.RuntimeSnapshot{}
	}
	s.snapshots[snapshot.Key] = snapshot
}

func (s *FakeRuntimeSnapshotStore) ListCandidates(req *core.DispatchRequest) []core.RuntimeSnapshot {
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

type FakeRuntimeStateProvider struct {
	ActiveConcurrencyByChannel map[int]int
	CooldownByChannel          map[int]bool
	FailureAvoidanceByChannel  map[int]bool
}

func (p *FakeRuntimeStateProvider) ActiveConcurrency(channelID int) int {
	if p == nil {
		return 0
	}
	return p.ActiveConcurrencyByChannel[channelID]
}

func (p *FakeRuntimeStateProvider) ConcurrencyCooldownActive(channelID int) bool {
	if p == nil {
		return false
	}
	return p.CooldownByChannel[channelID]
}

func (p *FakeRuntimeStateProvider) FailureAvoidanceActive(channelID int) bool {
	if p == nil {
		return false
	}
	return p.FailureAvoidanceByChannel[channelID]
}

func copyStringMap(src map[string]string) map[string]string {
	copied := make(map[string]string, len(src))
	for k, v := range src {
		copied[k] = v
	}
	return copied
}
