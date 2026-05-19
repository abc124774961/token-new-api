package scheduler

import (
	"context"
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
	Lease    *service.ChannelConcurrencyLease
	Status   QueueAcquireStatus
	WaitTime time.Duration
}

// QueueAcquireOptions carries optional admission metadata for queued requests.
// The zero value preserves the legacy channel-level queue behavior.
type QueueAcquireOptions struct {
	Group    string
	Priority int
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
	if !m.tryEnterQueue(channelID, options) {
		return QueueAcquireResult{Lease: lease, Status: QueueAcquireRejected}
	}
	defer m.leaveQueue(channelID, options)

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
			nextLease, acquired := service.TryAcquireChannelConcurrency(channelID, setting)
			if acquired {
				return QueueAcquireResult{Lease: nextLease, Status: QueueAcquireQueued, WaitTime: time.Since(started)}
			}
			lease = nextLease
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

func (m *QueueManager) tryEnterQueue(channelID int, options QueueAcquireOptions) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.queueDepths == nil {
		m.queueDepths = map[int]int{}
	}
	if m.queueGroupDepths == nil {
		m.queueGroupDepths = map[int]map[string]int{}
	}
	currentDepth := m.queueDepths[channelID]
	currentGroupDepth := m.groupDepthLocked(channelID, options.Group)
	if m.admissionPolicy != nil {
		if !m.admissionPolicy.AllowQueue(QueueAdmissionContext{
			ChannelID:         channelID,
			Group:             options.Group,
			Priority:          options.Priority,
			CurrentDepth:      currentDepth,
			CurrentGroupDepth: currentGroupDepth,
			GroupDepths:       m.cloneGroupDepthsLocked(channelID),
			MaxDepth:          m.maxDepth,
		}) {
			return false
		}
	} else if currentDepth >= m.maxDepth {
		return false
	}
	m.queueDepths[channelID]++
	if m.queueGroupDepths[channelID] == nil {
		m.queueGroupDepths[channelID] = map[string]int{}
	}
	m.queueGroupDepths[channelID][options.Group]++
	return true
}

func (m *QueueManager) leaveQueue(channelID int, options QueueAcquireOptions) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if groups := m.queueGroupDepths[channelID]; groups != nil {
		if groups[options.Group] <= 1 {
			delete(groups, options.Group)
		} else {
			groups[options.Group]--
		}
		if len(groups) == 0 {
			delete(m.queueGroupDepths, channelID)
		}
	}
	if m.queueDepths[channelID] <= 1 {
		delete(m.queueDepths, channelID)
		return
	}
	m.queueDepths[channelID]--
}

func (m *QueueManager) groupDepthLocked(channelID int, group string) int {
	if m.queueGroupDepths == nil || m.queueGroupDepths[channelID] == nil {
		return 0
	}
	return m.queueGroupDepths[channelID][group]
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
