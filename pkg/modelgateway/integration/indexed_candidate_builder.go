package integration

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/candidateindex"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

const (
	defaultAccountCandidateIndexRefreshInterval = 30 * time.Second
	defaultAccountCandidateShadowLogInterval    = 30 * time.Second
)

type AccountCandidateIndexOptions struct {
	RefreshInterval time.Duration
	LogInterval     time.Duration
	MaxCandidates   int
	ShadowLog       bool
}

type IndexedCandidatePoolBuilder struct {
	index         *candidateindex.CandidateIndex
	maxCandidates int
}

func NewIndexedCandidatePoolBuilder(index *candidateindex.CandidateIndex, maxCandidates int) *IndexedCandidatePoolBuilder {
	if maxCandidates <= 0 {
		maxCandidates = defaultMaxCandidatesPerGroup
	}
	return &IndexedCandidatePoolBuilder{
		index:         index,
		maxCandidates: maxCandidates,
	}
}

func (b *IndexedCandidatePoolBuilder) Build(req *core.DispatchRequest, policy core.GroupSmartPolicy) []core.Candidate {
	if b == nil || b.index == nil || req == nil {
		return nil
	}
	groups := candidateGroupsForRequest(req, policy)
	if len(groups) == 0 {
		return nil
	}
	return b.index.Query(candidateindex.Query{
		Groups:                 groups,
		ModelName:              req.ModelName,
		EndpointType:           req.EndpointType,
		RequiresCodexImageTool: req.RequiresCodexImageTool,
		MaxCandidates:          b.maxCandidates,
	})
}

type AccountCandidateShadowBuilder struct {
	primary        core.CandidatePoolBuilder
	shadow         core.CandidatePoolBuilder
	shadowLog      bool
	logInterval    time.Duration
	lastLogUnixSec atomic.Int64
}

type AccountCandidatePrimaryBuilder struct {
	indexed        core.CandidatePoolBuilder
	fallback       core.CandidatePoolBuilder
	shadowLog      bool
	logInterval    time.Duration
	lastLogUnixSec atomic.Int64
}

func NewAccountCandidateShadowBuilder(primary core.CandidatePoolBuilder, shadow core.CandidatePoolBuilder, options AccountCandidateIndexOptions) *AccountCandidateShadowBuilder {
	return &AccountCandidateShadowBuilder{
		primary:     primary,
		shadow:      shadow,
		shadowLog:   options.ShadowLog,
		logInterval: options.logInterval(),
	}
}

func NewAccountCandidatePrimaryBuilder(indexed core.CandidatePoolBuilder, fallback core.CandidatePoolBuilder, options AccountCandidateIndexOptions) *AccountCandidatePrimaryBuilder {
	return &AccountCandidatePrimaryBuilder{
		indexed:     indexed,
		fallback:    fallback,
		shadowLog:   options.ShadowLog,
		logInterval: options.logInterval(),
	}
}

func (b *AccountCandidateShadowBuilder) Build(req *core.DispatchRequest, policy core.GroupSmartPolicy) []core.Candidate {
	if b == nil || b.primary == nil {
		return nil
	}
	primaryCandidates := b.primary.Build(req, policy)
	if b.shadow == nil || !reserveCandidateComparisonLog(b.shadowLog, b.logInterval, &b.lastLogUnixSec) {
		return primaryCandidates
	}
	shadowCandidates := b.shadow.Build(req, policy)
	logCandidateComparison("model gateway account candidate shadow", req, policy, primaryCandidates, shadowCandidates)
	return primaryCandidates
}

func (b *AccountCandidatePrimaryBuilder) Build(req *core.DispatchRequest, policy core.GroupSmartPolicy) []core.Candidate {
	if b == nil || b.indexed == nil {
		return nil
	}
	indexedCandidates := b.indexed.Build(req, policy)
	if len(indexedCandidates) > 0 {
		if b.fallback != nil && reserveCandidateComparisonLog(b.shadowLog, b.logInterval, &b.lastLogUnixSec) {
			fallbackCandidates := b.fallback.Build(req, policy)
			logCandidateComparison("model gateway account candidate index", req, policy, fallbackCandidates, indexedCandidates)
		}
		return indexedCandidates
	}
	if b.fallback == nil {
		return nil
	}
	fallbackCandidates := b.fallback.Build(req, policy)
	if reserveCandidateComparisonLog(b.shadowLog, b.logInterval, &b.lastLogUnixSec) {
		logCandidateComparison("model gateway account candidate index", req, policy, fallbackCandidates, indexedCandidates)
	}
	return fallbackCandidates
}

func (o AccountCandidateIndexOptions) logInterval() time.Duration {
	if o.LogInterval > 0 {
		return o.LogInterval
	}
	return defaultAccountCandidateShadowLogInterval
}

func reserveCandidateComparisonLog(enabled bool, logInterval time.Duration, lastLogUnixSec *atomic.Int64) bool {
	if !enabled || lastLogUnixSec == nil {
		return false
	}
	now := time.Now().Unix()
	interval := int64(logInterval.Seconds())
	if interval <= 0 {
		interval = int64(defaultAccountCandidateShadowLogInterval.Seconds())
	}
	last := lastLogUnixSec.Load()
	if now-last < interval {
		return false
	}
	return lastLogUnixSec.CompareAndSwap(last, now)
}

type AccountCandidateIndexRuntime struct {
	index     *candidateindex.CandidateIndex
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	refreshMu sync.Mutex
	startOnce sync.Once
	closeOnce sync.Once
}

func NewAccountCandidateIndexRuntime(index *candidateindex.CandidateIndex, options AccountCandidateIndexOptions) *AccountCandidateIndexRuntime {
	if index == nil {
		index = candidateindex.New(nil, nil)
	}
	interval := options.RefreshInterval
	if interval <= 0 {
		interval = defaultAccountCandidateIndexRefreshInterval
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &AccountCandidateIndexRuntime{
		index:    index,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (r *AccountCandidateIndexRuntime) Index() *candidateindex.CandidateIndex {
	if r == nil {
		return nil
	}
	return r.index
}

func (r *AccountCandidateIndexRuntime) Start() {
	if r == nil {
		return
	}
	r.startOnce.Do(func() {
		r.refresh()
		go r.loop()
	})
}

func (r *AccountCandidateIndexRuntime) Close() {
	if r == nil {
		return
	}
	r.closeOnce.Do(func() {
		r.cancel()
	})
}

func (r *AccountCandidateIndexRuntime) Refresh() {
	if r == nil {
		return
	}
	r.refresh()
}

func (r *AccountCandidateIndexRuntime) loop() {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.ctx.Done():
			return
		case <-ticker.C:
			r.refresh()
		}
	}
}

func (r *AccountCandidateIndexRuntime) refresh() {
	if r == nil || r.index == nil {
		return
	}
	r.refreshMu.Lock()
	defer r.refreshMu.Unlock()
	if model.DB == nil {
		common.SysLog("model gateway account candidate index refresh skipped: database not initialized")
		return
	}
	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("model gateway account candidate index refresh failed: %v", err))
		return
	}
	stats := r.index.Rebuild(channels)
	common.SysLog(fmt.Sprintf("model gateway account candidate index refreshed: channels=%d accounts=%d candidates=%d groups=%d group_models=%d disabled_keys=%d version=%d",
		stats.Channels,
		stats.Accounts,
		stats.Candidates,
		stats.Groups,
		stats.GroupModels,
		stats.DisabledKeys,
		stats.Version,
	))
}

func logCandidateComparison(prefix string, req *core.DispatchRequest, policy core.GroupSmartPolicy, primary []core.Candidate, indexed []core.Candidate) {
	groups := candidateGroupsForRequest(req, policy)
	primaryChannels := uniqueChannelIDs(primary)
	indexedChannels := uniqueChannelIDs(indexed)
	accountCount := uniqueAccountCount(indexed)
	common.SysLog(fmt.Sprintf(
		"%s: model=%s groups=%s endpoint=%s primary_candidates=%d primary_channels=%d indexed_candidates=%d indexed_channels=%d indexed_accounts=%d missing_channels=%s extra_channels=%s",
		prefix,
		req.ModelName,
		strings.Join(groups, ","),
		req.EndpointType,
		len(primary),
		len(primaryChannels),
		len(indexed),
		len(indexedChannels),
		accountCount,
		joinIntSample(diffInts(primaryChannels, indexedChannels), 8),
		joinIntSample(diffInts(indexedChannels, primaryChannels), 8),
	))
}

func uniqueChannelIDs(candidates []core.Candidate) []int {
	if len(candidates) == 0 {
		return nil
	}
	set := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.RuntimeKey.ChannelID > 0 {
			set[candidate.RuntimeKey.ChannelID] = struct{}{}
			continue
		}
		if candidate.Channel != nil && candidate.Channel.Id > 0 {
			set[candidate.Channel.Id] = struct{}{}
		}
	}
	out := make([]int, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

func uniqueAccountCount(candidates []core.Candidate) int {
	if len(candidates) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.RuntimeKey.AccountID == "" {
			continue
		}
		set[candidate.RuntimeKey.AccountID] = struct{}{}
	}
	return len(set)
}

func diffInts(left []int, right []int) []int {
	if len(left) == 0 {
		return nil
	}
	rightSet := make(map[int]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	out := make([]int, 0)
	for _, value := range left {
		if _, ok := rightSet[value]; !ok {
			out = append(out, value)
		}
	}
	return out
}

func joinIntSample(values []int, maxItems int) string {
	if len(values) == 0 {
		return "-"
	}
	if maxItems <= 0 || maxItems > len(values) {
		maxItems = len(values)
	}
	parts := make([]string, 0, maxItems)
	for _, value := range values[:maxItems] {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	if len(values) > maxItems {
		parts = append(parts, fmt.Sprintf("+%d", len(values)-maxItems))
	}
	return strings.Join(parts, ",")
}

var _ core.CandidatePoolBuilder = (*IndexedCandidatePoolBuilder)(nil)
var _ core.CandidatePoolBuilder = (*AccountCandidateShadowBuilder)(nil)
var _ core.CandidatePoolBuilder = (*AccountCandidatePrimaryBuilder)(nil)
