package scheduler

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
)

type QueueAcquireStatus string

const (
	QueueAcquireAcquired QueueAcquireStatus = "acquired"
	QueueAcquireQueued   QueueAcquireStatus = "queued"
	QueueAcquireRejected QueueAcquireStatus = "rejected"
)

type QueueAcquireResult struct {
	Lease       *service.ChannelConcurrencyLease
	Status      QueueAcquireStatus
	WaitTime    time.Duration
	Plan        *core.DispatchPlan
	StopWaiting bool
}

// QueueAcquireOptions carries optional admission metadata for queued requests.
// The zero value preserves the legacy channel-level queue behavior.
type QueueAcquireOptions struct {
	Group      string
	Priority   int
	RuntimeKey core.RuntimeKey
	MaxDepth   int
}

// QueuePoolAcquireOptions describes a group-level resource pool queue. The
// TryAcquire callback should re-run the pool's selection logic and return an
// acquired lease when any member resource is available.
type QueuePoolAcquireOptions struct {
	PoolKey    string
	Group      string
	Priority   int
	RuntimeKey core.RuntimeKey
	MaxDepth   int
	TryAcquire func() QueueAcquireResult
}

// QueueAdmissionContext describes the current channel queue state before a
// request is admitted. GroupDepths is a snapshot and can be inspected safely by
// admission policies.
type QueueAdmissionContext struct {
	ChannelID         int
	Group             string
	Priority          int
	CurrentDepth      int
	CurrentGroupDepth int
	GroupDepths       map[string]int
	MaxDepth          int
}

type queueWaiterState struct {
	Group        string
	Priority     int
	HighPriority bool
	RuntimeKey   core.RuntimeKey
	Sequence     int64
}

// QueueAdmissionPolicy decides whether a request may wait in the channel queue.
type QueueAdmissionPolicy interface {
	AllowQueue(ctx QueueAdmissionContext) bool
}

// QueueAdmissionPolicyFunc adapts a function into QueueAdmissionPolicy.
type QueueAdmissionPolicyFunc func(ctx QueueAdmissionContext) bool

func (f QueueAdmissionPolicyFunc) AllowQueue(ctx QueueAdmissionContext) bool {
	if f == nil {
		return true
	}
	return f(ctx)
}

// QueueFairnessOptions configures PriorityQueueAdmissionPolicy.
// Zero values preserve the default max-depth-only admission behavior.
type QueueFairnessOptions struct {
	// HighPriorityGroups are group names admitted as high priority.
	HighPriorityGroups []string
	// HighPriorityThreshold admits requests with Priority >= threshold as high priority.
	// A zero or negative threshold disables priority-number based promotion.
	HighPriorityThreshold int
	// HighPriorityExtraDepth lets high-priority requests exceed MaxDepth by this many slots.
	HighPriorityExtraDepth int
	// HighPriorityReservedDepth keeps this many MaxDepth slots unavailable to normal groups.
	HighPriorityReservedDepth int
	// AbsoluteMaxDepth caps total queue depth after any high-priority extra capacity is applied.
	// A zero or negative value means there is no cap beyond the computed policy limit.
	AbsoluteMaxDepth int
}

// PriorityQueueAdmissionPolicy adds high-priority extra capacity and normal
// group reservation while keeping zero-value options compatible with the default policy.
type PriorityQueueAdmissionPolicy struct {
	options            QueueFairnessOptions
	highPriorityGroups map[string]struct{}
}

// NewPriorityQueueAdmissionPolicy creates a priority-aware queue admission policy.
func NewPriorityQueueAdmissionPolicy(options QueueFairnessOptions) *PriorityQueueAdmissionPolicy {
	policy := &PriorityQueueAdmissionPolicy{
		options:            options,
		highPriorityGroups: map[string]struct{}{},
	}
	for _, group := range options.HighPriorityGroups {
		if group != "" {
			policy.highPriorityGroups[group] = struct{}{}
		}
	}
	return policy
}

func (p *PriorityQueueAdmissionPolicy) AllowQueue(ctx QueueAdmissionContext) bool {
	if p == nil {
		return ctx.CurrentDepth < ctx.MaxDepth
	}
	if p.isHighPriority(ctx) {
		return ctx.CurrentDepth < p.highPriorityLimit(ctx.MaxDepth)
	}
	return p.allowNormalPriorityQueue(ctx)
}

func (p *PriorityQueueAdmissionPolicy) isHighPriority(ctx QueueAdmissionContext) bool {
	if p.isHighPriorityGroup(ctx.Group) {
		return true
	}
	return p.options.HighPriorityThreshold > 0 && ctx.Priority >= p.options.HighPriorityThreshold
}

func (p *PriorityQueueAdmissionPolicy) highPriorityLimit(maxDepth int) int {
	return capQueueLimit(maxDepth+p.highPriorityExtraDepth(), p.options.AbsoluteMaxDepth)
}

func (p *PriorityQueueAdmissionPolicy) allowNormalPriorityQueue(ctx QueueAdmissionContext) bool {
	if ctx.CurrentDepth >= capQueueLimit(ctx.MaxDepth, p.options.AbsoluteMaxDepth) {
		return false
	}
	reservedDepth := p.highPriorityReservedDepth(ctx.MaxDepth)
	if reservedDepth <= 0 {
		return true
	}
	normalLimit := ctx.MaxDepth - reservedDepth
	if normalLimit <= 0 {
		return false
	}
	normalDepth := ctx.CurrentDepth - p.currentHighPriorityDepth(ctx.GroupDepths)
	if normalDepth < 0 {
		normalDepth = 0
	}
	return normalDepth < normalLimit
}

func (p *PriorityQueueAdmissionPolicy) isHighPriorityGroup(group string) bool {
	if group == "" {
		return false
	}
	_, ok := p.highPriorityGroups[group]
	return ok
}

func (p *PriorityQueueAdmissionPolicy) currentHighPriorityDepth(groupDepths map[string]int) int {
	depth := 0
	for group, groupDepth := range groupDepths {
		if p.isHighPriorityGroup(group) {
			depth += groupDepth
		}
	}
	return depth
}

func (p *PriorityQueueAdmissionPolicy) highPriorityExtraDepth() int {
	if p.options.HighPriorityExtraDepth <= 0 {
		return 0
	}
	return p.options.HighPriorityExtraDepth
}

func (p *PriorityQueueAdmissionPolicy) highPriorityReservedDepth(maxDepth int) int {
	if p.options.HighPriorityReservedDepth <= 0 {
		return 0
	}
	if p.options.HighPriorityReservedDepth > maxDepth {
		return maxDepth
	}
	return p.options.HighPriorityReservedDepth
}

func capQueueLimit(limit int, absoluteMaxDepth int) int {
	if absoluteMaxDepth > 0 && limit > absoluteMaxDepth {
		return absoluteMaxDepth
	}
	if limit < 0 {
		return 0
	}
	return limit
}

type QueueManager struct {
	mu               sync.Mutex
	queueDepths      map[int]int
	queueGroupDepths map[int]map[string]int
	queueWaiters     map[int][]queueWaiterState
	poolDepths       map[string]int
	poolGroupDepths  map[string]map[string]int
	poolWaiters      map[string][]queueWaiterState
	rejectReasons    map[string]int
	nextWaiterSeq    int64
	timeout          time.Duration
	tick             time.Duration
	maxDepth         int
	admissionPolicy  QueueAdmissionPolicy
}

func NewQueueManager(timeout time.Duration, maxDepth int) *QueueManager {
	return NewQueueManagerWithAdmissionPolicy(timeout, maxDepth, nil)
}

func NewQueueManagerWithAdmissionPolicy(timeout time.Duration, maxDepth int, policy QueueAdmissionPolicy) *QueueManager {
	if timeout <= 0 {
		timeout = defaultQueueTimeoutMs * time.Millisecond
	}
	if maxDepth <= 0 {
		maxDepth = defaultQueueMaxDepth
	}
	return &QueueManager{
		queueDepths:      map[int]int{},
		queueGroupDepths: map[int]map[string]int{},
		queueWaiters:     map[int][]queueWaiterState{},
		poolDepths:       map[string]int{},
		poolGroupDepths:  map[string]map[string]int{},
		poolWaiters:      map[string][]queueWaiterState{},
		rejectReasons:    map[string]int{},
		timeout:          timeout,
		tick:             25 * time.Millisecond,
		maxDepth:         maxDepth,
		admissionPolicy:  policy,
	}
}

func (m *QueueManager) Acquire(ctx context.Context, plan *core.DispatchPlan, channelID int, setting dto.ChannelSettings) QueueAcquireResult {
	return m.AcquireWithOptions(ctx, plan, channelID, setting, QueueAcquireOptions{})
}

func (m *QueueManager) AcquireWithOptions(ctx context.Context, plan *core.DispatchPlan, channelID int, setting dto.ChannelSettings, options QueueAcquireOptions) QueueAcquireResult {
	if m == nil {
		lease, ok := service.TryAcquireChannelConcurrency(channelID, setting)
		if ok {
			return QueueAcquireResult{Lease: lease, Status: QueueAcquireAcquired}
		}
		return QueueAcquireResult{Lease: lease, Status: QueueAcquireRejected}
	}
	lease, ok := service.TryAcquireChannelConcurrency(channelID, setting)
	if ok {
		return QueueAcquireResult{Lease: lease, Status: QueueAcquireAcquired}
	}
	if plan == nil || !plan.QueueEnabled || plan.QueueWaitMs <= 0 {
		return QueueAcquireResult{Lease: lease, Status: QueueAcquireRejected}
	}
	timeout := time.Duration(plan.QueueWaitMs) * time.Millisecond
	if timeout <= 0 {
		timeout = m.timeout
	}
	if timeout > m.timeout {
		timeout = m.timeout
	}
	waiter, entered := m.tryEnterQueue(channelID, options)
	if !entered {
		return QueueAcquireResult{Lease: lease, Status: QueueAcquireRejected}
	}
	defer m.leaveQueue(channelID, waiter)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(m.tick)
	defer ticker.Stop()
	started := time.Now()
	for {
		select {
		case <-ctx.Done():
			return QueueAcquireResult{Lease: lease, Status: QueueAcquireRejected, WaitTime: time.Since(started)}
		case <-timer.C:
			return QueueAcquireResult{Lease: lease, Status: QueueAcquireRejected, WaitTime: time.Since(started)}
		case <-ticker.C:
			if !m.isQueueTurn(channelID, waiter) {
				continue
			}
			nextLease, acquired := service.TryAcquireChannelConcurrency(channelID, setting)
			if acquired {
				return QueueAcquireResult{Lease: nextLease, Status: QueueAcquireQueued, WaitTime: time.Since(started)}
			}
			lease = nextLease
		}
	}
}

func (m *QueueManager) AcquirePoolWithOptions(ctx context.Context, plan *core.DispatchPlan, options QueuePoolAcquireOptions) QueueAcquireResult {
	if options.TryAcquire == nil {
		return QueueAcquireResult{Status: QueueAcquireRejected}
	}
	if m == nil {
		result := options.TryAcquire()
		if result.Status == "" {
			result.Status = QueueAcquireRejected
		}
		return result
	}
	result := options.TryAcquire()
	if result.Status == QueueAcquireAcquired || result.Status == QueueAcquireQueued || result.StopWaiting {
		return result
	}
	if plan == nil || !plan.QueueEnabled || plan.QueueWaitMs <= 0 {
		if result.Status == "" {
			result.Status = QueueAcquireRejected
		}
		return result
	}
	timeout := time.Duration(plan.QueueWaitMs) * time.Millisecond
	if timeout <= 0 {
		timeout = m.timeout
	}
	if timeout > m.timeout {
		timeout = m.timeout
	}
	waiter, entered := m.tryEnterPoolQueue(options)
	if !entered {
		if result.Status == "" {
			result.Status = QueueAcquireRejected
		}
		return result
	}
	defer m.leavePoolQueue(options.PoolKey, waiter)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(m.tick)
	defer ticker.Stop()
	started := time.Now()
	for {
		select {
		case <-ctx.Done():
			result.WaitTime = time.Since(started)
			if result.Status == "" {
				result.Status = QueueAcquireRejected
			}
			return result
		case <-timer.C:
			result.WaitTime = time.Since(started)
			if result.Status == "" {
				result.Status = QueueAcquireRejected
			}
			return result
		case <-ticker.C:
			if !m.isPoolQueueTurn(options.PoolKey, waiter) {
				continue
			}
			next := options.TryAcquire()
			if next.Status == QueueAcquireAcquired || next.Status == QueueAcquireQueued {
				if next.Status == QueueAcquireAcquired {
					next.Status = QueueAcquireQueued
				}
				next.WaitTime = time.Since(started)
				return next
			}
			if next.StopWaiting {
				next.WaitTime = time.Since(started)
				if next.Status == "" {
					next.Status = QueueAcquireRejected
				}
				return next
			}
			if next.Status != "" || next.Lease != nil || next.Plan != nil {
				result = next
			}
		}
	}
}

func (m *QueueManager) Depth(channelID int) int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queueDepths[channelID]
}

func (m *QueueManager) PoolDepth(poolKey string) int {
	if m == nil || poolKey == "" {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.poolDepths[poolKey]
}

func (m *QueueManager) Snapshot() map[int]int {
	if m == nil {
		return map[int]int{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := make(map[int]int, len(m.queueDepths))
	for channelID, depth := range m.queueDepths {
		snapshot[channelID] = depth
	}
	return snapshot
}

func (m *QueueManager) DetailedSnapshot() core.RuntimeQueueSnapshot {
	if m == nil {
		return core.RuntimeQueueSnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().Unix()
	snapshot := core.RuntimeQueueSnapshot{
		UpdatedAt: now,
		Summary: core.RuntimeQueueSummary{
			UpdatedAt:     now,
			QueueCapacity: m.maxDepth,
			TotalCapacity: m.maxDepth,
		},
		Channels: make([]core.RuntimeQueueChannelSnapshot, 0, len(m.queueDepths)),
	}
	for channelID, depth := range m.queueDepths {
		if channelID <= 0 || depth <= 0 {
			continue
		}
		channel := core.RuntimeQueueChannelSnapshot{
			ChannelID:       channelID,
			QueueDepth:      depth,
			QueuedRequests:  depth,
			WaitingRequests: depth,
			QueueCapacity:   m.maxDepth,
			MaxQueueDepth:   m.maxDepth,
		}
		for group, groupDepth := range m.queueGroupDepths[channelID] {
			if groupDepth <= 0 {
				continue
			}
			groupSnapshot := core.RuntimeQueueGroupSnapshot{
				ChannelID:       channelID,
				Group:           group,
				QueueDepth:      groupDepth,
				QueuedRequests:  groupDepth,
				WaitingRequests: groupDepth,
			}
			applyQueuePriorityDepths(&groupSnapshot, m.queueWaiters[channelID])
			channel.Groups = append(channel.Groups, groupSnapshot)
		}
		for _, waiter := range m.queueWaiters[channelID] {
			if waiter.HighPriority {
				channel.HighPriorityDepth++
			} else {
				channel.NormalDepth++
			}
		}
		runtimeKeys := runtimeQueueKeySnapshots(channelID, m.queueWaiters[channelID])
		snapshot.RuntimeKeys = append(snapshot.RuntimeKeys, runtimeKeys...)
		if len(m.queueWaiters[channelID]) == 0 {
			channel.NormalDepth = depth
		}
		channel.HighPriorityCapacity = highPriorityCapacityForPolicy(m.admissionPolicy, m.maxDepth)
		channel.NormalCapacity = normalCapacityForPolicy(m.admissionPolicy, m.maxDepth)
		sortRuntimeQueueGroupSnapshots(channel.Groups)
		snapshot.Channels = append(snapshot.Channels, channel)
		snapshot.Summary.TotalQueued += depth
		snapshot.Summary.TotalDepth += depth
		snapshot.Summary.Waiting += depth
		snapshot.Summary.QueuedRequests += depth
		snapshot.Summary.WaitingRequests += depth
		snapshot.Summary.HighPriorityDepth += channel.HighPriorityDepth
		snapshot.Summary.NormalDepth += channel.NormalDepth
		if depth > snapshot.Summary.MaxQueueDepth {
			snapshot.Summary.MaxQueueDepth = depth
		}
	}
	poolGroups := make([]core.RuntimeQueueGroupSnapshot, 0)
	for poolKey, depth := range m.poolDepths {
		if poolKey == "" || depth <= 0 {
			continue
		}
		for group, groupDepth := range m.poolGroupDepths[poolKey] {
			if groupDepth <= 0 {
				continue
			}
			groupSnapshot := core.RuntimeQueueGroupSnapshot{
				Group:           group,
				QueueDepth:      groupDepth,
				QueuedRequests:  groupDepth,
				WaitingRequests: groupDepth,
			}
			applyQueuePriorityDepths(&groupSnapshot, m.poolWaiters[poolKey])
			poolGroups = append(poolGroups, groupSnapshot)
		}
		for _, waiter := range m.poolWaiters[poolKey] {
			if waiter.HighPriority {
				snapshot.Summary.HighPriorityDepth++
			} else {
				snapshot.Summary.NormalDepth++
			}
		}
		snapshot.RuntimeKeys = append(snapshot.RuntimeKeys, runtimeQueuePoolKeySnapshots(m.poolWaiters[poolKey])...)
		snapshot.Summary.TotalQueued += depth
		snapshot.Summary.TotalDepth += depth
		snapshot.Summary.Waiting += depth
		snapshot.Summary.QueuedRequests += depth
		snapshot.Summary.WaitingRequests += depth
		if depth > snapshot.Summary.MaxQueueDepth {
			snapshot.Summary.MaxQueueDepth = depth
		}
	}
	snapshot.Summary.QueueChannels = len(snapshot.Channels)
	snapshot.Summary.HighPriorityCapacity = highPriorityCapacityForPolicy(m.admissionPolicy, m.maxDepth)
	snapshot.Summary.NormalCapacity = normalCapacityForPolicy(m.admissionPolicy, m.maxDepth)
	snapshot.Groups = aggregateRuntimeQueueGroups(append(aggregateQueueGroups(snapshot.Channels), poolGroups...))
	snapshot.Summary.QueueGroups = len(snapshot.Groups)
	snapshot.RejectReasons = runtimeQueueRejectReasons(m.rejectReasons)
	sortRuntimeQueueChannelSnapshots(snapshot.Channels)
	sortRuntimeQueueKeySnapshots(snapshot.RuntimeKeys)
	return snapshot
}

func (m *QueueManager) tryEnterQueue(channelID int, options QueueAcquireOptions) (queueWaiterState, bool) {
	if m == nil {
		return queueWaiterState{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.queueDepths == nil {
		m.queueDepths = map[int]int{}
	}
	if m.queueGroupDepths == nil {
		m.queueGroupDepths = map[int]map[string]int{}
	}
	if m.queueWaiters == nil {
		m.queueWaiters = map[int][]queueWaiterState{}
	}
	if m.rejectReasons == nil {
		m.rejectReasons = map[string]int{}
	}
	currentDepth := m.queueDepths[channelID]
	currentGroupDepth := m.groupDepthLocked(channelID, options.Group)
	maxDepth := m.maxDepth
	if options.MaxDepth > 0 {
		maxDepth = options.MaxDepth
	}
	if m.admissionPolicy != nil {
		if !m.admissionPolicy.AllowQueue(QueueAdmissionContext{
			ChannelID:         channelID,
			Group:             options.Group,
			Priority:          options.Priority,
			CurrentDepth:      currentDepth,
			CurrentGroupDepth: currentGroupDepth,
			GroupDepths:       m.cloneGroupDepthsLocked(channelID),
			MaxDepth:          maxDepth,
		}) {
			m.rejectReasons[queueRejectReason(options, currentDepth, maxDepth)]++
			return queueWaiterState{}, false
		}
	} else if currentDepth >= maxDepth {
		m.rejectReasons["max_depth_reached"]++
		return queueWaiterState{}, false
	}
	m.queueDepths[channelID]++
	if m.queueGroupDepths[channelID] == nil {
		m.queueGroupDepths[channelID] = map[string]int{}
	}
	m.queueGroupDepths[channelID][options.Group]++
	m.nextWaiterSeq++
	waiter := queueWaiterState{
		Group:        options.Group,
		Priority:     options.Priority,
		HighPriority: m.isHighPriorityLocked(options),
		RuntimeKey:   normalizedQueueRuntimeKey(channelID, options),
		Sequence:     m.nextWaiterSeq,
	}
	m.queueWaiters[channelID] = append(m.queueWaiters[channelID], waiter)
	return waiter, true
}

func (m *QueueManager) tryEnterPoolQueue(options QueuePoolAcquireOptions) (queueWaiterState, bool) {
	if m == nil || options.PoolKey == "" {
		return queueWaiterState{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.poolDepths == nil {
		m.poolDepths = map[string]int{}
	}
	if m.poolGroupDepths == nil {
		m.poolGroupDepths = map[string]map[string]int{}
	}
	if m.poolWaiters == nil {
		m.poolWaiters = map[string][]queueWaiterState{}
	}
	if m.rejectReasons == nil {
		m.rejectReasons = map[string]int{}
	}
	currentDepth := m.poolDepths[options.PoolKey]
	currentGroupDepth := m.poolGroupDepthLocked(options.PoolKey, options.Group)
	maxDepth := m.maxDepth
	if options.MaxDepth > 0 {
		maxDepth = options.MaxDepth
	}
	if m.admissionPolicy != nil {
		if !m.admissionPolicy.AllowQueue(QueueAdmissionContext{
			Group:             options.Group,
			Priority:          options.Priority,
			CurrentDepth:      currentDepth,
			CurrentGroupDepth: currentGroupDepth,
			GroupDepths:       m.clonePoolGroupDepthsLocked(options.PoolKey),
			MaxDepth:          maxDepth,
		}) {
			m.rejectReasons[queueRejectReason(QueueAcquireOptions{
				Group:    options.Group,
				Priority: options.Priority,
			}, currentDepth, maxDepth)]++
			return queueWaiterState{}, false
		}
	} else if currentDepth >= maxDepth {
		m.rejectReasons["max_depth_reached"]++
		return queueWaiterState{}, false
	}
	m.poolDepths[options.PoolKey]++
	if m.poolGroupDepths[options.PoolKey] == nil {
		m.poolGroupDepths[options.PoolKey] = map[string]int{}
	}
	m.poolGroupDepths[options.PoolKey][options.Group]++
	m.nextWaiterSeq++
	waiter := queueWaiterState{
		Group:        options.Group,
		Priority:     options.Priority,
		HighPriority: m.isHighPriorityLocked(QueueAcquireOptions{Group: options.Group, Priority: options.Priority}),
		RuntimeKey:   normalizedPoolQueueRuntimeKey(options),
		Sequence:     m.nextWaiterSeq,
	}
	m.poolWaiters[options.PoolKey] = append(m.poolWaiters[options.PoolKey], waiter)
	return waiter, true
}

func (m *QueueManager) leaveQueue(channelID int, waiter queueWaiterState) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if groups := m.queueGroupDepths[channelID]; groups != nil {
		if groups[waiter.Group] <= 1 {
			delete(groups, waiter.Group)
		} else {
			groups[waiter.Group]--
		}
		if len(groups) == 0 {
			delete(m.queueGroupDepths, channelID)
		}
	}
	if m.queueDepths[channelID] <= 1 {
		delete(m.queueDepths, channelID)
		m.removeQueueWaiterLocked(channelID, waiter)
		return
	}
	m.queueDepths[channelID]--
	m.removeQueueWaiterLocked(channelID, waiter)
}

func (m *QueueManager) leavePoolQueue(poolKey string, waiter queueWaiterState) {
	if m == nil || poolKey == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if groups := m.poolGroupDepths[poolKey]; groups != nil {
		if groups[waiter.Group] <= 1 {
			delete(groups, waiter.Group)
		} else {
			groups[waiter.Group]--
		}
		if len(groups) == 0 {
			delete(m.poolGroupDepths, poolKey)
		}
	}
	if m.poolDepths[poolKey] <= 1 {
		delete(m.poolDepths, poolKey)
		m.removePoolWaiterLocked(poolKey, waiter)
		return
	}
	m.poolDepths[poolKey]--
	m.removePoolWaiterLocked(poolKey, waiter)
}

func (m *QueueManager) groupDepthLocked(channelID int, group string) int {
	if m.queueGroupDepths == nil || m.queueGroupDepths[channelID] == nil {
		return 0
	}
	return m.queueGroupDepths[channelID][group]
}

func (m *QueueManager) poolGroupDepthLocked(poolKey string, group string) int {
	if m.poolGroupDepths == nil || len(m.poolGroupDepths[poolKey]) == 0 {
		return 0
	}
	return m.poolGroupDepths[poolKey][group]
}

func (m *QueueManager) cloneGroupDepthsLocked(channelID int) map[string]int {
	if m.queueGroupDepths == nil || len(m.queueGroupDepths[channelID]) == 0 {
		return nil
	}
	source := m.queueGroupDepths[channelID]
	groupDepths := make(map[string]int, len(source))
	for group, depth := range source {
		groupDepths[group] = depth
	}
	return groupDepths
}

func (m *QueueManager) clonePoolGroupDepthsLocked(poolKey string) map[string]int {
	if m.poolGroupDepths == nil || len(m.poolGroupDepths[poolKey]) == 0 {
		return nil
	}
	source := m.poolGroupDepths[poolKey]
	groupDepths := make(map[string]int, len(source))
	for group, depth := range source {
		groupDepths[group] = depth
	}
	return groupDepths
}

func (m *QueueManager) isHighPriorityLocked(options QueueAcquireOptions) bool {
	if policy, ok := m.admissionPolicy.(*PriorityQueueAdmissionPolicy); ok && policy != nil {
		return policy.isHighPriority(QueueAdmissionContext{
			Group:    options.Group,
			Priority: options.Priority,
		})
	}
	return options.Priority > 0
}

func (m *QueueManager) removeQueueWaiterLocked(channelID int, target queueWaiterState) {
	waiters := m.queueWaiters[channelID]
	if len(waiters) == 0 {
		return
	}
	for index, waiter := range waiters {
		if queueWaiterSameIdentity(waiter, target) {
			m.queueWaiters[channelID] = append(waiters[:index], waiters[index+1:]...)
			if len(m.queueWaiters[channelID]) == 0 {
				delete(m.queueWaiters, channelID)
			}
			return
		}
	}
	m.queueWaiters[channelID] = waiters[1:]
	if len(m.queueWaiters[channelID]) == 0 {
		delete(m.queueWaiters, channelID)
	}
}

func (m *QueueManager) removePoolWaiterLocked(poolKey string, target queueWaiterState) {
	waiters := m.poolWaiters[poolKey]
	if len(waiters) == 0 {
		return
	}
	for index, waiter := range waiters {
		if queueWaiterSameIdentity(waiter, target) {
			m.poolWaiters[poolKey] = append(waiters[:index], waiters[index+1:]...)
			if len(m.poolWaiters[poolKey]) == 0 {
				delete(m.poolWaiters, poolKey)
			}
			return
		}
	}
	m.poolWaiters[poolKey] = waiters[1:]
	if len(m.poolWaiters[poolKey]) == 0 {
		delete(m.poolWaiters, poolKey)
	}
}

func (m *QueueManager) isQueueTurn(channelID int, target queueWaiterState) bool {
	if m == nil {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	waiters := m.queueWaiters[channelID]
	if len(waiters) <= 1 {
		return true
	}
	bestIndex := 0
	for idx := 1; idx < len(waiters); idx++ {
		if queueWaiterPrecedes(waiters[idx], waiters[bestIndex]) {
			bestIndex = idx
		}
	}
	best := waiters[bestIndex]
	return queueWaiterSameIdentity(best, target)
}

func (m *QueueManager) isPoolQueueTurn(poolKey string, target queueWaiterState) bool {
	if m == nil || poolKey == "" {
		return true
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	waiters := m.poolWaiters[poolKey]
	if len(waiters) <= 1 {
		return true
	}
	bestIndex := 0
	for idx := 1; idx < len(waiters); idx++ {
		if queueWaiterPrecedes(waiters[idx], waiters[bestIndex]) {
			bestIndex = idx
		}
	}
	best := waiters[bestIndex]
	return queueWaiterSameIdentity(best, target)
}

func queueWaiterPrecedes(left queueWaiterState, right queueWaiterState) bool {
	if left.Priority != right.Priority {
		return left.Priority > right.Priority
	}
	if left.HighPriority != right.HighPriority {
		return left.HighPriority
	}
	return left.Sequence < right.Sequence
}

func queueWaiterSameIdentity(left queueWaiterState, right queueWaiterState) bool {
	return left.Group == right.Group &&
		left.Priority == right.Priority &&
		left.HighPriority == right.HighPriority &&
		left.RuntimeKey == right.RuntimeKey &&
		left.Sequence == right.Sequence
}

func applyQueuePriorityDepths(group *core.RuntimeQueueGroupSnapshot, waiters []queueWaiterState) {
	if group == nil {
		return
	}
	for _, waiter := range waiters {
		if waiter.Group != group.Group {
			continue
		}
		if waiter.HighPriority {
			group.HighPriorityDepth++
		} else {
			group.NormalDepth++
		}
	}
	if group.HighPriorityDepth == 0 && group.NormalDepth == 0 {
		group.NormalDepth = group.QueueDepth
	}
}

func runtimeQueueKeySnapshots(channelID int, waiters []queueWaiterState) []core.RuntimeQueueKeySnapshot {
	if len(waiters) == 0 {
		return nil
	}
	keyMap := map[core.RuntimeKey]*core.RuntimeQueueKeySnapshot{}
	for _, waiter := range waiters {
		key := waiter.RuntimeKey
		if key.ChannelID <= 0 {
			key.ChannelID = channelID
		}
		target := keyMap[key]
		if target == nil {
			target = &core.RuntimeQueueKeySnapshot{
				RuntimeKey:            key,
				RequestedModel:        key.RequestedModel,
				UpstreamModel:         key.UpstreamModel,
				ChannelID:             key.ChannelID,
				Group:                 key.Group,
				EndpointType:          string(key.EndpointType),
				CapabilityFingerprint: key.CapabilityFingerprint,
			}
			keyMap[key] = target
		}
		target.QueueDepth++
		target.QueuedRequests++
		target.WaitingRequests++
		if waiter.HighPriority {
			target.HighPriorityDepth++
		} else {
			target.NormalDepth++
		}
	}
	out := make([]core.RuntimeQueueKeySnapshot, 0, len(keyMap))
	for _, item := range keyMap {
		out = append(out, *item)
	}
	sortRuntimeQueueKeySnapshots(out)
	return out
}

func runtimeQueuePoolKeySnapshots(waiters []queueWaiterState) []core.RuntimeQueueKeySnapshot {
	if len(waiters) == 0 {
		return nil
	}
	keyMap := map[core.RuntimeKey]*core.RuntimeQueueKeySnapshot{}
	for _, waiter := range waiters {
		key := waiter.RuntimeKey
		key.ChannelID = 0
		target := keyMap[key]
		if target == nil {
			target = &core.RuntimeQueueKeySnapshot{
				RuntimeKey:            key,
				RequestedModel:        key.RequestedModel,
				UpstreamModel:         key.UpstreamModel,
				Group:                 key.Group,
				EndpointType:          string(key.EndpointType),
				CapabilityFingerprint: key.CapabilityFingerprint,
			}
			keyMap[key] = target
		}
		target.QueueDepth++
		target.QueuedRequests++
		target.WaitingRequests++
		if waiter.HighPriority {
			target.HighPriorityDepth++
		} else {
			target.NormalDepth++
		}
	}
	out := make([]core.RuntimeQueueKeySnapshot, 0, len(keyMap))
	for _, item := range keyMap {
		out = append(out, *item)
	}
	sortRuntimeQueueKeySnapshots(out)
	return out
}

func normalizedQueueRuntimeKey(channelID int, options QueueAcquireOptions) core.RuntimeKey {
	key := options.RuntimeKey
	if key.ChannelID <= 0 {
		key.ChannelID = channelID
	}
	if key.Group == "" {
		key.Group = options.Group
	}
	return key
}

func normalizedPoolQueueRuntimeKey(options QueuePoolAcquireOptions) core.RuntimeKey {
	key := options.RuntimeKey
	key.ChannelID = 0
	if key.Group == "" {
		key.Group = options.Group
	}
	return key
}

func aggregateQueueGroups(channels []core.RuntimeQueueChannelSnapshot) []core.RuntimeQueueGroupSnapshot {
	groupMap := map[string]*core.RuntimeQueueGroupSnapshot{}
	for _, channel := range channels {
		for _, group := range channel.Groups {
			key := group.Group
			if key == "" {
				key = "_default"
			}
			target := groupMap[key]
			if target == nil {
				target = &core.RuntimeQueueGroupSnapshot{Group: group.Group}
				groupMap[key] = target
			}
			target.QueueDepth += group.QueueDepth
			target.QueuedRequests += group.QueuedRequests
			target.WaitingRequests += group.WaitingRequests
			target.HighPriorityDepth += group.HighPriorityDepth
			target.NormalDepth += group.NormalDepth
		}
	}
	groups := make([]core.RuntimeQueueGroupSnapshot, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, *group)
	}
	sortRuntimeQueueGroupSnapshots(groups)
	return groups
}

func aggregateRuntimeQueueGroups(groups []core.RuntimeQueueGroupSnapshot) []core.RuntimeQueueGroupSnapshot {
	if len(groups) == 0 {
		return nil
	}
	groupMap := map[string]*core.RuntimeQueueGroupSnapshot{}
	for _, group := range groups {
		key := group.Group
		if key == "" {
			key = "_default"
		}
		target := groupMap[key]
		if target == nil {
			target = &core.RuntimeQueueGroupSnapshot{Group: group.Group}
			groupMap[key] = target
		}
		target.QueueDepth += group.QueueDepth
		target.QueuedRequests += group.QueuedRequests
		target.WaitingRequests += group.WaitingRequests
		target.HighPriorityDepth += group.HighPriorityDepth
		target.NormalDepth += group.NormalDepth
	}
	out := make([]core.RuntimeQueueGroupSnapshot, 0, len(groupMap))
	for _, group := range groupMap {
		out = append(out, *group)
	}
	sortRuntimeQueueGroupSnapshots(out)
	return out
}

func sortRuntimeQueueChannelSnapshots(channels []core.RuntimeQueueChannelSnapshot) {
	sort.SliceStable(channels, func(i, j int) bool {
		if channels[i].QueueDepth != channels[j].QueueDepth {
			return channels[i].QueueDepth > channels[j].QueueDepth
		}
		return channels[i].ChannelID < channels[j].ChannelID
	})
}

func sortRuntimeQueueGroupSnapshots(groups []core.RuntimeQueueGroupSnapshot) {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].QueueDepth != groups[j].QueueDepth {
			return groups[i].QueueDepth > groups[j].QueueDepth
		}
		return groups[i].Group < groups[j].Group
	})
}

func sortRuntimeQueueKeySnapshots(keys []core.RuntimeQueueKeySnapshot) {
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i].QueueDepth != keys[j].QueueDepth {
			return keys[i].QueueDepth > keys[j].QueueDepth
		}
		if keys[i].ChannelID != keys[j].ChannelID {
			return keys[i].ChannelID < keys[j].ChannelID
		}
		if keys[i].RequestedModel != keys[j].RequestedModel {
			return keys[i].RequestedModel < keys[j].RequestedModel
		}
		return keys[i].Group < keys[j].Group
	})
}

func runtimeQueueRejectReasons(counts map[string]int) []core.RuntimeQueueReasonCount {
	if len(counts) == 0 {
		return nil
	}
	reasons := make([]core.RuntimeQueueReasonCount, 0, len(counts))
	for reason, count := range counts {
		if count <= 0 {
			continue
		}
		reasons = append(reasons, core.RuntimeQueueReasonCount{
			Reason: reason,
			Count:  count,
		})
	}
	sort.SliceStable(reasons, func(i, j int) bool {
		if reasons[i].Count != reasons[j].Count {
			return reasons[i].Count > reasons[j].Count
		}
		return reasons[i].Reason < reasons[j].Reason
	})
	return reasons
}

func queueRejectReason(options QueueAcquireOptions, currentDepth int, maxDepth int) string {
	if currentDepth >= maxDepth {
		if options.Priority > 0 {
			return "priority_queue_limit_reached"
		}
		return "max_depth_reached"
	}
	return "admission_policy_rejected"
}

func highPriorityCapacityForPolicy(policy QueueAdmissionPolicy, maxDepth int) int {
	if priorityPolicy, ok := policy.(*PriorityQueueAdmissionPolicy); ok && priorityPolicy != nil {
		return priorityPolicy.highPriorityLimit(maxDepth)
	}
	return 0
}

func normalCapacityForPolicy(policy QueueAdmissionPolicy, maxDepth int) int {
	if priorityPolicy, ok := policy.(*PriorityQueueAdmissionPolicy); ok && priorityPolicy != nil {
		reserved := priorityPolicy.highPriorityReservedDepth(maxDepth)
		if maxDepth-reserved < 0 {
			return 0
		}
		return maxDepth - reserved
	}
	return maxDepth
}
