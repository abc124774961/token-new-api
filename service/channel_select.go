package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	"github.com/QuantumNous/new-api/pkg/codexauth"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

type RetryParam struct {
	Ctx                    *gin.Context
	TokenGroup             string
	ModelName              string
	EndpointType           constant.EndpointType
	RequiresCodexImageTool bool
	Retry                  *int
	ExtraRetries           *int
	resetNextTry           bool
}

type channelAvoidanceEntry struct {
	until                 time.Time
	reason                string
	failureCount          int
	probeRecoveryRequired bool
}

var channelFailureAvoidance sync.Map
var channelTimeoutDegradeEvents sync.Map
var channelRuntimeFailureAvoidance sync.Map
var channelRuntimeTimeoutDegradeEvents sync.Map
var channelOverloadRecoveryEvents sync.Map
var channelRuntimeOverloadRecoveryEvents sync.Map

const (
	channelFailureAvoidancePauseDuration = 30 * time.Minute
	channelFailureAvoidanceStepSeconds   = 8
	ChannelTimeoutRecoveryReason         = "timeout_recovery"
	ChannelOverloadRecoveryReason        = "overload_recovery"
)

type ChannelFailureAvoidanceStatus struct {
	Active                bool   `json:"active"`
	Reason                string `json:"reason,omitempty"`
	Until                 int64  `json:"until,omitempty"`
	RemainingSec          int64  `json:"remaining_seconds,omitempty"`
	FailureCount          int    `json:"failure_count,omitempty"`
	ProbeRecoveryRequired bool   `json:"probe_recovery_required,omitempty"`
}

type ChannelFailureAvoidanceRecord struct {
	Active                bool
	Reason                string
	Until                 time.Time
	Remaining             time.Duration
	FailureCount          int
	ShouldPause           bool
	ProbeRecoveryRequired bool
}

type ChannelFailureAvoidanceContext struct {
	ChannelName  string
	ChannelType  int
	Group        string
	ModelName    string
	RequestId    string
	ErrorType    string
	ErrorCode    string
	StatusCode   int
	AttemptIndex int
	FinalFailure bool
	UsedChannels string
	Message      string
	Metadata     string
}

type ChannelPerformanceAvoidanceContext struct {
	ChannelName  string
	ChannelType  int
	Group        string
	ModelName    string
	RequestId    string
	AttemptIndex int
	TTFTMs       int64
	DurationMs   int64
}

type ChannelTimeoutDegradeConfig struct {
	Enabled               bool
	Window                time.Duration
	MinSamples            int
	Threshold             float64
	Consecutive           int
	RecoveryProbeRequired bool
}

type ChannelOverloadRecoveryConfig struct {
	Enabled     bool
	Window      time.Duration
	MinSamples  int
	Consecutive int
}

type channelTimeoutEvent struct {
	at      time.Time
	kind    string
	timeout bool
}

type channelTimeoutDegradeState struct {
	mu          sync.Mutex
	events      []channelTimeoutEvent
	consecutive int
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) GetExtraRetries() int {
	if p.ExtraRetries == nil {
		return 0
	}
	return *p.ExtraRetries
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) SetExtraRetries(retry int) {
	p.ExtraRetries = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.ExtraRetries != nil && *p.ExtraRetries > 0 {
		*p.ExtraRetries--
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func (p *RetryParam) AllowExtraRetry(count int) {
	if count <= 0 {
		return
	}
	current := p.GetExtraRetries()
	p.SetExtraRetries(current + count)
}

func (p *RetryParam) HasBudget(maxRetry int) bool {
	return p.GetRetry() <= maxRetry || p.GetExtraRetries() > 0
}

func getUsedChannelSet(ctx *gin.Context) map[int]struct{} {
	usedChannelSet := make(map[int]struct{})
	if ctx == nil {
		return usedChannelSet
	}
	for _, channelIDStr := range ctx.GetStringSlice("use_channel") {
		channelID, err := strconv.Atoi(channelIDStr)
		if err != nil {
			continue
		}
		usedChannelSet[channelID] = struct{}{}
	}
	return usedChannelSet
}

func getBalanceSkippedChannelSet(ctx *gin.Context) map[int]struct{} {
	skippedChannelSet := make(map[int]struct{})
	if ctx == nil {
		return skippedChannelSet
	}
	channelIDs, ok := common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelBalanceSkipped)
	if !ok {
		return skippedChannelSet
	}
	for _, channelID := range channelIDs {
		if channelID > 0 {
			skippedChannelSet[channelID] = struct{}{}
		}
	}
	return skippedChannelSet
}

func MarkChannelBalanceSkipped(ctx *gin.Context, channelID int) {
	if ctx == nil || channelID <= 0 {
		return
	}
	channelIDs, _ := common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelBalanceSkipped)
	for _, existing := range channelIDs {
		if existing == channelID {
			return
		}
	}
	channelIDs = append(channelIDs, channelID)
	common.SetContextKey(ctx, constant.ContextKeyChannelBalanceSkipped, channelIDs)
}

func MarkChannelRuntimeBalanceSkipped(ctx *gin.Context, identity ChannelRuntimeIdentity) {
	if ctx == nil {
		return
	}
	identity = identity.Normalize()
	if !identity.Valid() {
		return
	}
	if !identity.HasAccountScope() {
		MarkChannelBalanceSkipped(ctx, identity.ChannelID)
		return
	}
	identities, _ := common.GetContextKeyType[[]ChannelRuntimeIdentity](ctx, constant.ContextKeyChannelRuntimeBalanceSkipped)
	for _, existing := range identities {
		if existing.Normalize().AccountScope() == identity.AccountScope() {
			return
		}
	}
	identities = append(identities, identity.AccountScope())
	common.SetContextKey(ctx, constant.ContextKeyChannelRuntimeBalanceSkipped, identities)
}

func IsChannelBalanceSkipped(ctx *gin.Context, channelID int) bool {
	if ctx == nil || channelID <= 0 {
		return false
	}
	_, ok := getBalanceSkippedChannelSet(ctx)[channelID]
	return ok
}

func IsChannelRuntimeBalanceSkipped(ctx *gin.Context, identity ChannelRuntimeIdentity) bool {
	if ctx == nil {
		return false
	}
	identity = identity.Normalize()
	if !identity.Valid() {
		return false
	}
	if !identity.HasAccountScope() {
		return IsChannelBalanceSkipped(ctx, identity.ChannelID)
	}
	identities, ok := common.GetContextKeyType[[]ChannelRuntimeIdentity](ctx, constant.ContextKeyChannelRuntimeBalanceSkipped)
	if !ok {
		return false
	}
	accountScope := identity.AccountScope()
	for _, existing := range identities {
		if existing.Normalize().AccountScope() == accountScope {
			return true
		}
	}
	return false
}

func getAvoidedChannelSet() map[int]struct{} {
	avoided := make(map[int]struct{})
	if !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return avoided
	}
	now := time.Now()
	channelFailureAvoidance.Range(func(key, value any) bool {
		channelID, ok := key.(int)
		if !ok {
			channelFailureAvoidance.Delete(key)
			return true
		}
		entry, ok := value.(channelAvoidanceEntry)
		if !ok {
			channelFailureAvoidance.Delete(key)
			return true
		}
		if !entry.until.After(now) && !entry.probeRecoveryRequired {
			return true
		}
		avoided[channelID] = struct{}{}
		return true
	})
	return avoided
}

func getTimeoutRecoveryChannelSet() map[int]struct{} {
	recovering := make(map[int]struct{})
	if !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return recovering
	}
	channelFailureAvoidance.Range(func(key, value any) bool {
		channelID, ok := key.(int)
		if !ok {
			channelFailureAvoidance.Delete(key)
			return true
		}
		entry, ok := value.(channelAvoidanceEntry)
		if !ok {
			channelFailureAvoidance.Delete(key)
			return true
		}
		if entry.probeRecoveryRequired || IsTimeoutRecoveryReason(entry.reason) {
			recovering[channelID] = struct{}{}
		}
		return true
	})
	return recovering
}

func getBalanceInsufficientChannelSet() map[int]struct{} {
	skipped := make(map[int]struct{})
	now := time.Now()
	channelBalanceInsufficientRuntime.Range(func(key, value any) bool {
		channelID, ok := key.(int)
		if !ok {
			channelBalanceInsufficientRuntime.Delete(key)
			return true
		}
		until, ok := value.(time.Time)
		if !ok || !until.After(now) {
			channelBalanceInsufficientRuntime.Delete(channelID)
			return true
		}
		skipped[channelID] = struct{}{}
		return true
	})
	return skipped
}

func getSelectionSkippedChannelSet(ctx *gin.Context) map[int]struct{} {
	skippedChannelSet := make(map[int]struct{})
	if ctx == nil {
		return skippedChannelSet
	}
	channelIDs, ok := common.GetContextKeyType[[]int](ctx, constant.ContextKeyChannelSelectionSkipped)
	if !ok {
		return skippedChannelSet
	}
	for _, channelID := range channelIDs {
		if channelID > 0 {
			skippedChannelSet[channelID] = struct{}{}
		}
	}
	return skippedChannelSet
}

func GetChannelFailureAvoidanceStatus(channelID int) *ChannelFailureAvoidanceStatus {
	if channelID <= 0 || !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return nil
	}
	value, ok := channelFailureAvoidance.Load(channelID)
	if !ok {
		return nil
	}
	entry, ok := value.(channelAvoidanceEntry)
	if !ok {
		channelFailureAvoidance.Delete(channelID)
		return nil
	}
	now := time.Now()
	if !entry.until.After(now) && !entry.probeRecoveryRequired {
		return nil
	}
	remaining := int64(0)
	if entry.until.After(now) {
		remaining = int64(entry.until.Sub(now).Seconds())
	}
	return &ChannelFailureAvoidanceStatus{
		Active:                true,
		Reason:                entry.reason,
		Until:                 entry.until.Unix(),
		RemainingSec:          remaining,
		FailureCount:          entry.failureCount,
		ProbeRecoveryRequired: entry.probeRecoveryRequired,
	}
}

func GetChannelRuntimeFailureAvoidanceStatus(identity ChannelRuntimeIdentity) *ChannelFailureAvoidanceStatus {
	identity = identity.Normalize()
	if !identity.Valid() {
		return nil
	}
	if identity.HasAccountScope() {
		return getChannelRuntimeFailureAvoidanceStatus(identity)
	}
	return GetChannelFailureAvoidanceStatus(identity.ChannelID)
}

func getChannelRuntimeFailureAvoidanceStatus(identity ChannelRuntimeIdentity) *ChannelFailureAvoidanceStatus {
	if !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return nil
	}
	value, ok := channelRuntimeFailureAvoidance.Load(identity.Normalize())
	if !ok {
		return nil
	}
	entry, ok := value.(channelAvoidanceEntry)
	if !ok {
		channelRuntimeFailureAvoidance.Delete(identity.Normalize())
		return nil
	}
	now := time.Now()
	if !entry.until.After(now) && !entry.probeRecoveryRequired {
		channelRuntimeFailureAvoidance.Delete(identity.Normalize())
		return nil
	}
	remaining := int64(0)
	if entry.until.After(now) {
		remaining = int64(entry.until.Sub(now).Seconds())
	}
	return &ChannelFailureAvoidanceStatus{
		Active:                true,
		Reason:                entry.reason,
		Until:                 entry.until.Unix(),
		RemainingSec:          remaining,
		FailureCount:          entry.failureCount,
		ProbeRecoveryRequired: entry.probeRecoveryRequired,
	}
}

func RecordChannelTimeoutDegradeSuccess(channelID int, config ChannelTimeoutDegradeConfig) {
	recordChannelTimeoutDegradeSample(channelID, "success", false, config, nil)
}

func RecordChannelRuntimeTimeoutDegradeSuccess(identity ChannelRuntimeIdentity, config ChannelTimeoutDegradeConfig) {
	identity = identity.Normalize()
	if !identity.Valid() {
		return
	}
	if !identity.HasAccountScope() {
		RecordChannelTimeoutDegradeSuccess(identity.ChannelID, config)
		return
	}
	recordChannelRuntimeTimeoutDegradeSample(identity, "success", false, config, nil)
}

func RecordChannelTimeoutDegradeSample(channelID int, kind string, config ChannelTimeoutDegradeConfig, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	return recordChannelTimeoutDegradeSample(channelID, kind, true, config, failureContext)
}

func RecordChannelRuntimeTimeoutDegradeSample(identity ChannelRuntimeIdentity, kind string, config ChannelTimeoutDegradeConfig, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	identity = identity.Normalize()
	if !identity.Valid() {
		return nil
	}
	if !identity.HasAccountScope() {
		return RecordChannelTimeoutDegradeSample(identity.ChannelID, kind, config, failureContext)
	}
	return recordChannelRuntimeTimeoutDegradeSample(identity, kind, true, config, failureContext)
}

func RecordChannelOverloadRecoverySuccess(channelID int, config ChannelOverloadRecoveryConfig) {
	recordChannelOverloadRecoverySample(channelID, "success", false, config, nil)
}

func RecordChannelRuntimeOverloadRecoverySuccess(identity ChannelRuntimeIdentity, config ChannelOverloadRecoveryConfig) {
	identity = identity.Normalize()
	if !identity.Valid() {
		return
	}
	if !identity.HasAccountScope() {
		RecordChannelOverloadRecoverySuccess(identity.ChannelID, config)
		return
	}
	recordChannelRuntimeOverloadRecoverySample(identity, "success", false, config, nil)
}

func RecordChannelOverloadRecoverySample(channelID int, kind string, config ChannelOverloadRecoveryConfig, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	return recordChannelOverloadRecoverySample(channelID, kind, true, config, failureContext)
}

func RecordChannelRuntimeOverloadRecoverySample(identity ChannelRuntimeIdentity, kind string, config ChannelOverloadRecoveryConfig, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	identity = identity.Normalize()
	if !identity.Valid() {
		return nil
	}
	if !identity.HasAccountScope() {
		return RecordChannelOverloadRecoverySample(identity.ChannelID, kind, config, failureContext)
	}
	return recordChannelRuntimeOverloadRecoverySample(identity, kind, true, config, failureContext)
}

func recordChannelTimeoutDegradeSample(channelID int, kind string, timeoutSample bool, config ChannelTimeoutDegradeConfig, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	if channelID <= 0 || !config.Enabled {
		return nil
	}
	if config.Window <= 0 {
		config.Window = 10 * time.Minute
	}
	if config.MinSamples <= 0 {
		config.MinSamples = 5
	}
	if config.Threshold <= 0 {
		config.Threshold = 0.4
	}
	if config.Threshold > 1 {
		config.Threshold = 1
	}
	if config.Consecutive <= 0 {
		config.Consecutive = 3
	}
	stateValue, _ := channelTimeoutDegradeEvents.LoadOrStore(channelID, &channelTimeoutDegradeState{})
	state, ok := stateValue.(*channelTimeoutDegradeState)
	if !ok || state == nil {
		state = &channelTimeoutDegradeState{}
		channelTimeoutDegradeEvents.Store(channelID, state)
	}
	now := time.Now()
	state.mu.Lock()
	cutoff := now.Add(-config.Window)
	events := state.events[:0]
	for _, event := range state.events {
		if event.at.After(cutoff) {
			events = append(events, event)
		}
	}
	events = append(events, channelTimeoutEvent{at: now, kind: strings.TrimSpace(kind), timeout: timeoutSample})
	state.events = events
	if timeoutSample {
		state.consecutive++
	} else {
		state.consecutive = 0
	}
	samples := len(state.events)
	consecutive := state.consecutive
	timeoutCount := 0
	for _, event := range state.events {
		if event.timeout {
			timeoutCount++
		}
	}
	rate := 0.0
	if samples > 0 {
		rate = float64(timeoutCount) / float64(samples)
	}
	triggered := consecutive >= config.Consecutive || (samples >= config.MinSamples && rate >= config.Threshold)
	state.mu.Unlock()
	if !timeoutSample || !triggered {
		return nil
	}
	if failureContext == nil {
		failureContext = &ChannelFailureAvoidanceContext{}
	}
	failureContext.ErrorType = "timeout"
	failureContext.ErrorCode = ChannelTimeoutRecoveryReason
	failureContext.Message = strings.TrimSpace(fmt.Sprintf("%s samples=%d consecutive=%d rate=%.2f", kind, samples, consecutive, rate))
	record := RecordChannelTimeoutRecovery(channelID, failureContext)
	if record != nil {
		common.SysLog(fmt.Sprintf("channel #%d entered timeout recovery: kind=%s samples=%d consecutive=%d rate=%.2f", channelID, kind, samples, consecutive, rate))
	}
	return record
}

func recordChannelRuntimeTimeoutDegradeSample(identity ChannelRuntimeIdentity, kind string, timeoutSample bool, config ChannelTimeoutDegradeConfig, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	identity = identity.Normalize()
	if !identity.Valid() || !identity.HasAccountScope() || !config.Enabled {
		return nil
	}
	if config.Window <= 0 {
		config.Window = 10 * time.Minute
	}
	if config.MinSamples <= 0 {
		config.MinSamples = 5
	}
	if config.Threshold <= 0 {
		config.Threshold = 0.4
	}
	if config.Threshold > 1 {
		config.Threshold = 1
	}
	if config.Consecutive <= 0 {
		config.Consecutive = 3
	}
	stateValue, _ := channelRuntimeTimeoutDegradeEvents.LoadOrStore(identity, &channelTimeoutDegradeState{})
	state, ok := stateValue.(*channelTimeoutDegradeState)
	if !ok || state == nil {
		state = &channelTimeoutDegradeState{}
		channelRuntimeTimeoutDegradeEvents.Store(identity, state)
	}
	now := time.Now()
	state.mu.Lock()
	cutoff := now.Add(-config.Window)
	events := state.events[:0]
	for _, event := range state.events {
		if event.at.After(cutoff) {
			events = append(events, event)
		}
	}
	events = append(events, channelTimeoutEvent{at: now, kind: strings.TrimSpace(kind), timeout: timeoutSample})
	state.events = events
	if timeoutSample {
		state.consecutive++
	} else {
		state.consecutive = 0
	}
	samples := len(state.events)
	consecutive := state.consecutive
	timeoutCount := 0
	for _, event := range state.events {
		if event.timeout {
			timeoutCount++
		}
	}
	rate := 0.0
	if samples > 0 {
		rate = float64(timeoutCount) / float64(samples)
	}
	triggered := consecutive >= config.Consecutive || (samples >= config.MinSamples && rate >= config.Threshold)
	state.mu.Unlock()
	if !timeoutSample || !triggered {
		return nil
	}
	if failureContext == nil {
		failureContext = &ChannelFailureAvoidanceContext{}
	}
	failureContext.ErrorType = "timeout"
	failureContext.ErrorCode = ChannelTimeoutRecoveryReason
	failureContext.Message = strings.TrimSpace(fmt.Sprintf("%s samples=%d consecutive=%d rate=%.2f", kind, samples, consecutive, rate))
	record := RecordChannelRuntimeTimeoutRecovery(identity, failureContext)
	if record != nil {
		common.SysLog(fmt.Sprintf("channel runtime #%d entered timeout recovery: kind=%s samples=%d consecutive=%d rate=%.2f", identity.ChannelID, kind, samples, consecutive, rate))
	}
	return record
}

func recordChannelOverloadRecoverySample(channelID int, kind string, overloadSample bool, config ChannelOverloadRecoveryConfig, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	if channelID <= 0 || !config.Enabled {
		return nil
	}
	config = normalizeChannelOverloadRecoveryConfig(config)
	stateValue, _ := channelOverloadRecoveryEvents.LoadOrStore(channelID, &channelTimeoutDegradeState{})
	state, ok := stateValue.(*channelTimeoutDegradeState)
	if !ok || state == nil {
		state = &channelTimeoutDegradeState{}
		channelOverloadRecoveryEvents.Store(channelID, state)
	}
	now := time.Now()
	samples, overloadCount, consecutive := recordChannelOverloadRecoveryEvent(state, kind, overloadSample, config.Window, now)
	triggered := overloadCount >= config.MinSamples || consecutive >= config.Consecutive
	if !overloadSample || !triggered {
		return nil
	}
	if failureContext == nil {
		failureContext = &ChannelFailureAvoidanceContext{}
	}
	failureContext.ErrorType = "overload"
	failureContext.ErrorCode = ChannelOverloadRecoveryReason
	failureContext.Message = strings.TrimSpace(fmt.Sprintf("%s samples=%d overloads=%d consecutive=%d window=%s", kind, samples, overloadCount, consecutive, config.Window))
	record := RecordChannelOverloadRecovery(channelID, failureContext)
	if record != nil {
		common.SysLog(fmt.Sprintf("channel #%d entered overload recovery: kind=%s samples=%d overloads=%d consecutive=%d window=%s", channelID, kind, samples, overloadCount, consecutive, config.Window))
	}
	return record
}

func recordChannelRuntimeOverloadRecoverySample(identity ChannelRuntimeIdentity, kind string, overloadSample bool, config ChannelOverloadRecoveryConfig, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	identity = identity.Normalize()
	if !identity.Valid() || !identity.HasAccountScope() || !config.Enabled {
		return nil
	}
	config = normalizeChannelOverloadRecoveryConfig(config)
	stateValue, _ := channelRuntimeOverloadRecoveryEvents.LoadOrStore(identity, &channelTimeoutDegradeState{})
	state, ok := stateValue.(*channelTimeoutDegradeState)
	if !ok || state == nil {
		state = &channelTimeoutDegradeState{}
		channelRuntimeOverloadRecoveryEvents.Store(identity, state)
	}
	now := time.Now()
	samples, overloadCount, consecutive := recordChannelOverloadRecoveryEvent(state, kind, overloadSample, config.Window, now)
	triggered := overloadCount >= config.MinSamples || consecutive >= config.Consecutive
	if !overloadSample || !triggered {
		return nil
	}
	if failureContext == nil {
		failureContext = &ChannelFailureAvoidanceContext{}
	}
	failureContext.ErrorType = "overload"
	failureContext.ErrorCode = ChannelOverloadRecoveryReason
	failureContext.Message = strings.TrimSpace(fmt.Sprintf("%s samples=%d overloads=%d consecutive=%d window=%s", kind, samples, overloadCount, consecutive, config.Window))
	record := RecordChannelRuntimeOverloadRecovery(identity, failureContext)
	if record != nil {
		common.SysLog(fmt.Sprintf("channel runtime #%d entered overload recovery: kind=%s samples=%d overloads=%d consecutive=%d window=%s", identity.ChannelID, kind, samples, overloadCount, consecutive, config.Window))
	}
	return record
}

func normalizeChannelOverloadRecoveryConfig(config ChannelOverloadRecoveryConfig) ChannelOverloadRecoveryConfig {
	if config.Window <= 0 {
		config.Window = time.Minute
	}
	if config.MinSamples <= 0 {
		config.MinSamples = 3
	}
	if config.Consecutive <= 0 {
		config.Consecutive = config.MinSamples
	}
	return config
}

func recordChannelOverloadRecoveryEvent(state *channelTimeoutDegradeState, kind string, overloadSample bool, window time.Duration, now time.Time) (int, int, int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	cutoff := now.Add(-window)
	events := state.events[:0]
	for _, event := range state.events {
		if event.at.After(cutoff) {
			events = append(events, event)
		}
	}
	events = append(events, channelTimeoutEvent{at: now, kind: strings.TrimSpace(kind), timeout: overloadSample})
	state.events = events
	if overloadSample {
		state.consecutive++
	} else {
		state.consecutive = 0
	}
	overloadCount := 0
	for _, event := range state.events {
		if event.timeout {
			overloadCount++
		}
	}
	return len(state.events), overloadCount, state.consecutive
}

func mergeChannelSets(sets ...map[int]struct{}) map[int]struct{} {
	merged := make(map[int]struct{})
	for _, set := range sets {
		for channelID := range set {
			merged[channelID] = struct{}{}
		}
	}
	return merged
}

func subtractChannelSet(set map[int]struct{}, excluded map[int]struct{}) map[int]struct{} {
	result := make(map[int]struct{}, len(set))
	for channelID := range set {
		if _, skip := excluded[channelID]; skip {
			continue
		}
		result[channelID] = struct{}{}
	}
	return result
}

func RecordChannelFailureAvoidance(channelID int, reason string) *ChannelFailureAvoidanceRecord {
	return RecordChannelFailureAvoidanceWithContext(channelID, reason, nil)
}

func RecordChannelPerformanceAvoidance(channelID int, reason string, performanceContext *ChannelPerformanceAvoidanceContext) *ChannelFailureAvoidanceRecord {
	if performanceContext == nil {
		return recordChannelAvoidance(channelID, reason, nil, false)
	}
	failureContext := &ChannelFailureAvoidanceContext{
		ChannelName:  performanceContext.ChannelName,
		ChannelType:  performanceContext.ChannelType,
		Group:        performanceContext.Group,
		ModelName:    performanceContext.ModelName,
		RequestId:    performanceContext.RequestId,
		ErrorType:    "performance",
		ErrorCode:    reason,
		StatusCode:   0,
		AttemptIndex: performanceContext.AttemptIndex,
		FinalFailure: false,
		Message:      fmt.Sprintf("ttft=%dms duration=%dms", performanceContext.TTFTMs, performanceContext.DurationMs),
	}
	return recordChannelAvoidance(channelID, reason, failureContext, false)
}

func RecordChannelFailureAvoidanceWithContext(channelID int, reason string, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	return recordChannelAvoidance(channelID, reason, failureContext, true)
}

func RecordChannelRuntimeFailureAvoidanceWithContext(identity ChannelRuntimeIdentity, reason string, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	identity = identity.Normalize()
	if !identity.Valid() {
		return nil
	}
	if !identity.HasAccountScope() {
		return RecordChannelFailureAvoidanceWithContext(identity.ChannelID, reason, failureContext)
	}
	return recordChannelRuntimeAvoidance(identity, reason, failureContext, true, false)
}

func RecordChannelOverloadRecovery(channelID int, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	if failureContext != nil {
		if strings.TrimSpace(failureContext.ErrorType) == "" {
			failureContext.ErrorType = "overload"
		}
		failureContext.ErrorCode = ChannelOverloadRecoveryReason
	}
	return recordChannelAvoidanceWithProbeRecovery(channelID, ChannelOverloadRecoveryReason, failureContext, true, true)
}

func RecordChannelRuntimeOverloadRecovery(identity ChannelRuntimeIdentity, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	identity = identity.Normalize()
	if !identity.Valid() {
		return nil
	}
	if !identity.HasAccountScope() {
		return RecordChannelOverloadRecovery(identity.ChannelID, failureContext)
	}
	if failureContext != nil {
		if strings.TrimSpace(failureContext.ErrorType) == "" {
			failureContext.ErrorType = "overload"
		}
		failureContext.ErrorCode = ChannelOverloadRecoveryReason
	}
	return recordChannelRuntimeAvoidance(identity, ChannelOverloadRecoveryReason, failureContext, true, true)
}

func recordChannelAvoidance(channelID int, reason string, failureContext *ChannelFailureAvoidanceContext, allowPause bool) *ChannelFailureAvoidanceRecord {
	return recordChannelAvoidanceWithProbeRecovery(channelID, reason, failureContext, allowPause, false)
}

func recordChannelRuntimeAvoidance(identity ChannelRuntimeIdentity, reason string, failureContext *ChannelFailureAvoidanceContext, allowPause bool, probeRecoveryRequired bool) *ChannelFailureAvoidanceRecord {
	if !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return nil
	}
	identity = identity.Normalize()
	if !identity.Valid() || !identity.HasAccountScope() {
		return nil
	}
	now := time.Now()
	baseTTL := time.Duration(common.ChannelFailureAvoidanceTTLSeconds) * time.Second
	failureCount := 1
	if value, ok := channelRuntimeFailureAvoidance.Load(identity); ok {
		if entry, ok := value.(channelAvoidanceEntry); ok {
			failureCount = entry.failureCount + 1
			if entry.probeRecoveryRequired && !probeRecoveryRequired {
				probeRecoveryRequired = true
				reason = entry.reason
			}
		}
	}
	ttl := baseTTL
	if failureCount > 1 {
		ttl += time.Duration((failureCount-1)*channelFailureAvoidanceStepSeconds) * time.Second
	}
	until := now.Add(ttl)
	remaining := until.Sub(now)
	shouldPause := allowPause && remaining >= channelFailureAvoidancePauseDuration
	if shouldPause {
		until = now.Add(channelFailureAvoidancePauseDuration)
		remaining = channelFailureAvoidancePauseDuration
	}
	channelRuntimeFailureAvoidance.Store(identity, channelAvoidanceEntry{
		until:                 until,
		reason:                reason,
		failureCount:          failureCount,
		probeRecoveryRequired: probeRecoveryRequired,
	})
	common.SysLog(fmt.Sprintf("channel runtime #%d temporarily cooled for %s until %s after %d errors: %s", identity.ChannelID, remaining, until.Format(time.RFC3339), failureCount, reason))
	recordChannelFailureAvoidanceEvent(identity.ChannelID, reason, until, remaining, failureCount, shouldPause, failureContext)
	return &ChannelFailureAvoidanceRecord{
		Active:                true,
		Reason:                reason,
		Until:                 until,
		Remaining:             remaining,
		FailureCount:          failureCount,
		ShouldPause:           shouldPause,
		ProbeRecoveryRequired: probeRecoveryRequired,
	}
}

func recordChannelAvoidanceWithProbeRecovery(channelID int, reason string, failureContext *ChannelFailureAvoidanceContext, allowPause bool, probeRecoveryRequired bool) *ChannelFailureAvoidanceRecord {
	if channelID <= 0 || !common.ChannelFailureAvoidanceEnabled || common.ChannelFailureAvoidanceTTLSeconds <= 0 {
		return nil
	}
	now := time.Now()
	baseTTL := time.Duration(common.ChannelFailureAvoidanceTTLSeconds) * time.Second
	failureCount := 1
	if value, ok := channelFailureAvoidance.Load(channelID); ok {
		if entry, ok := value.(channelAvoidanceEntry); ok {
			failureCount = entry.failureCount + 1
			if entry.probeRecoveryRequired && !probeRecoveryRequired {
				probeRecoveryRequired = true
				reason = entry.reason
			}
		}
	}
	ttl := baseTTL
	if failureCount > 1 {
		ttl += time.Duration((failureCount-1)*channelFailureAvoidanceStepSeconds) * time.Second
	}
	until := now.Add(ttl)
	remaining := until.Sub(now)
	shouldPause := allowPause && remaining >= channelFailureAvoidancePauseDuration
	if shouldPause {
		until = now.Add(channelFailureAvoidancePauseDuration)
		remaining = channelFailureAvoidancePauseDuration
	}
	channelFailureAvoidance.Store(channelID, channelAvoidanceEntry{
		until:                 until,
		reason:                reason,
		failureCount:          failureCount,
		probeRecoveryRequired: probeRecoveryRequired,
	})
	common.SysLog(fmt.Sprintf("channel #%d temporarily cooled for %s until %s after %d errors: %s", channelID, remaining, until.Format(time.RFC3339), failureCount, reason))
	recordChannelFailureAvoidanceEvent(channelID, reason, until, remaining, failureCount, shouldPause, failureContext)
	return &ChannelFailureAvoidanceRecord{
		Active:                true,
		Reason:                reason,
		Until:                 until,
		Remaining:             remaining,
		FailureCount:          failureCount,
		ShouldPause:           shouldPause,
		ProbeRecoveryRequired: probeRecoveryRequired,
	}
}

func recordChannelFailureAvoidanceEvent(channelID int, reason string, until time.Time, remaining time.Duration, failureCount int, shouldPause bool, failureContext *ChannelFailureAvoidanceContext) {
	params := model.RecordChannelFailureEventParams{
		ChannelId:        channelID,
		EventType:        model.ChannelFailureEventTypeAvoidance,
		Reason:           reason,
		FailureCount:     failureCount,
		RemainingSeconds: int64(remaining.Seconds()),
		Until:            until.Unix(),
		AutoPaused:       shouldPause,
	}
	if shouldPause {
		params.EventType = model.ChannelFailureEventTypePaused
	}
	if failureContext != nil {
		params.ChannelName = failureContext.ChannelName
		params.ChannelType = failureContext.ChannelType
		params.Group = failureContext.Group
		params.ModelName = failureContext.ModelName
		params.RequestId = failureContext.RequestId
		params.ErrorType = failureContext.ErrorType
		params.ErrorCode = failureContext.ErrorCode
		params.StatusCode = failureContext.StatusCode
		params.AttemptIndex = failureContext.AttemptIndex
		params.FinalFailure = failureContext.FinalFailure
		params.UsedChannels = failureContext.UsedChannels
		params.Message = failureContext.Message
		params.Metadata = failureContext.Metadata
	}
	model.RecordChannelFailureEvent(params)
}

func ClearChannelFailureAvoidance(channelID int) {
	if channelID <= 0 {
		return
	}
	channelFailureAvoidance.Delete(channelID)
	channelTimeoutDegradeEvents.Delete(channelID)
	channelOverloadRecoveryEvents.Delete(channelID)
	channelRuntimeFailureAvoidance.Range(func(key, value any) bool {
		identity, ok := key.(ChannelRuntimeIdentity)
		if ok && identity.ChannelID == channelID {
			channelRuntimeFailureAvoidance.Delete(key)
		}
		return true
	})
	channelRuntimeTimeoutDegradeEvents.Range(func(key, value any) bool {
		identity, ok := key.(ChannelRuntimeIdentity)
		if ok && identity.ChannelID == channelID {
			channelRuntimeTimeoutDegradeEvents.Delete(key)
		}
		return true
	})
	channelRuntimeOverloadRecoveryEvents.Range(func(key, value any) bool {
		identity, ok := key.(ChannelRuntimeIdentity)
		if ok && identity.ChannelID == channelID {
			channelRuntimeOverloadRecoveryEvents.Delete(key)
		}
		return true
	})
}

func ClearChannelRuntimeFailureAvoidance(identity ChannelRuntimeIdentity) {
	identity = identity.Normalize()
	if !identity.Valid() {
		return
	}
	if !identity.HasAccountScope() {
		ClearChannelFailureAvoidance(identity.ChannelID)
		return
	}
	channelRuntimeFailureAvoidance.Delete(identity)
	channelRuntimeTimeoutDegradeEvents.Delete(identity)
	channelRuntimeOverloadRecoveryEvents.Delete(identity)
}

func ClearChannelRuntimeFailureAvoidanceForAccountIndex(channelID int, credentialIndex int) {
	if channelID <= 0 || credentialIndex < 0 {
		return
	}
	channelRuntimeFailureAvoidance.Range(func(key, value any) bool {
		identity, ok := key.(ChannelRuntimeIdentity)
		if ok && identity.ChannelID == channelID && identity.CredentialIndexSet && identity.CredentialIndex == credentialIndex {
			channelRuntimeFailureAvoidance.Delete(key)
		}
		return true
	})
	channelRuntimeTimeoutDegradeEvents.Range(func(key, value any) bool {
		identity, ok := key.(ChannelRuntimeIdentity)
		if ok && identity.ChannelID == channelID && identity.CredentialIndexSet && identity.CredentialIndex == credentialIndex {
			channelRuntimeTimeoutDegradeEvents.Delete(key)
		}
		return true
	})
	channelRuntimeOverloadRecoveryEvents.Range(func(key, value any) bool {
		identity, ok := key.(ChannelRuntimeIdentity)
		if ok && identity.ChannelID == channelID && identity.CredentialIndexSet && identity.CredentialIndex == credentialIndex {
			channelRuntimeOverloadRecoveryEvents.Delete(key)
		}
		return true
	})
}

func ClearChannelFailureAvoidanceOnRealSuccess(channelID int) bool {
	if channelID <= 0 {
		return false
	}
	value, ok := channelFailureAvoidance.Load(channelID)
	if !ok {
		return false
	}
	entry, ok := value.(channelAvoidanceEntry)
	if !ok {
		channelFailureAvoidance.Delete(channelID)
		return true
	}
	if entry.probeRecoveryRequired || IsTimeoutRecoveryReason(entry.reason) {
		return false
	}
	channelFailureAvoidance.Delete(channelID)
	return true
}

func ClearChannelRuntimeFailureAvoidanceOnRealSuccess(identity ChannelRuntimeIdentity) bool {
	identity = identity.Normalize()
	if !identity.Valid() {
		return false
	}
	if !identity.HasAccountScope() {
		return ClearChannelFailureAvoidanceOnRealSuccess(identity.ChannelID)
	}
	value, ok := channelRuntimeFailureAvoidance.Load(identity)
	if !ok {
		return false
	}
	entry, ok := value.(channelAvoidanceEntry)
	if !ok {
		channelRuntimeFailureAvoidance.Delete(identity)
		return true
	}
	if entry.probeRecoveryRequired || IsTimeoutRecoveryReason(entry.reason) {
		return false
	}
	channelRuntimeFailureAvoidance.Delete(identity)
	return true
}

func ClearChannelProbeRecoveryAvoidance(channelID int) {
	ClearChannelFailureAvoidance(channelID)
}

func ClearChannelRuntimeProbeRecoveryAvoidance(identity ChannelRuntimeIdentity) {
	ClearChannelRuntimeFailureAvoidance(identity)
}

func IsTimeoutRecoveryReason(reason string) bool {
	return strings.TrimSpace(reason) == ChannelTimeoutRecoveryReason
}

func IsProbeRecoveryReason(reason string) bool {
	switch strings.TrimSpace(reason) {
	case ChannelTimeoutRecoveryReason, ChannelOverloadRecoveryReason:
		return true
	default:
		return false
	}
}

func RecordChannelTimeoutRecovery(channelID int, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	if failureContext != nil {
		if strings.TrimSpace(failureContext.ErrorType) == "" {
			failureContext.ErrorType = "timeout"
		}
		failureContext.ErrorCode = ChannelTimeoutRecoveryReason
	}
	return recordChannelAvoidanceWithProbeRecovery(channelID, ChannelTimeoutRecoveryReason, failureContext, true, true)
}

func RecordChannelRuntimeTimeoutRecovery(identity ChannelRuntimeIdentity, failureContext *ChannelFailureAvoidanceContext) *ChannelFailureAvoidanceRecord {
	identity = identity.Normalize()
	if !identity.Valid() {
		return nil
	}
	if !identity.HasAccountScope() {
		return RecordChannelTimeoutRecovery(identity.ChannelID, failureContext)
	}
	if failureContext != nil {
		if strings.TrimSpace(failureContext.ErrorType) == "" {
			failureContext.ErrorType = "timeout"
		}
		failureContext.ErrorCode = ChannelTimeoutRecoveryReason
	}
	return recordChannelRuntimeAvoidance(identity, ChannelTimeoutRecoveryReason, failureContext, true, true)
}

func getChannelFailureAvoidanceForTest(channelID int) (channelAvoidanceEntry, bool) {
	value, ok := channelFailureAvoidance.Load(channelID)
	if !ok {
		return channelAvoidanceEntry{}, false
	}
	entry, ok := value.(channelAvoidanceEntry)
	return entry, ok
}

func clearAllChannelFailureAvoidanceForTest() {
	channelFailureAvoidance.Range(func(key, value any) bool {
		channelFailureAvoidance.Delete(key)
		return true
	})
	channelTimeoutDegradeEvents.Range(func(key, value any) bool {
		channelTimeoutDegradeEvents.Delete(key)
		return true
	})
	channelOverloadRecoveryEvents.Range(func(key, value any) bool {
		channelOverloadRecoveryEvents.Delete(key)
		return true
	})
	channelRuntimeFailureAvoidance.Range(func(key, value any) bool {
		channelRuntimeFailureAvoidance.Delete(key)
		return true
	})
	channelRuntimeTimeoutDegradeEvents.Range(func(key, value any) bool {
		channelRuntimeTimeoutDegradeEvents.Delete(key)
		return true
	})
	channelRuntimeOverloadRecoveryEvents.Range(func(key, value any) bool {
		channelRuntimeOverloadRecoveryEvents.Delete(key)
		return true
	})
}

func ChannelSupportsRequiredEndpoint(channel *model.Channel, modelName string, endpointType constant.EndpointType) bool {
	if endpointType == "" {
		return true
	}
	if channel == nil {
		return false
	}
	return channelcapability.SupportsEndpoint(channel.Type, modelName, channel.GetOtherSettings(), endpointType)
}

func ChannelSupportsRequiredCapabilities(channel *model.Channel, modelName string, endpointType constant.EndpointType, requiresCodexImageTool bool) bool {
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		if !codexChannelHasSchedulableAccount(channel) {
			return false
		}
		if endpointType == constant.EndpointTypeOpenAIResponseCompact && !codexChannelHasCompactCapableAccount(channel) {
			return false
		}
		if endpointType == constant.EndpointTypeOpenAIResponse && !codexChannelHasResponsesCapableAccount(channel) {
			return false
		}
	}
	if channel != nil && channel.Type == constant.ChannelTypeOpenAI && openAICodexOAuthChannelApplies(channel, endpointType) {
		if endpointType == constant.EndpointTypeOpenAIResponseCompact {
			return openAICodexOAuthChannelHasCompactCapableAccount(channel)
		}
		if endpointType == constant.EndpointTypeOpenAIResponse {
			return openAICodexOAuthChannelHasResponsesCapableAccount(channel)
		}
	}
	return ChannelSupportsRequiredEndpoint(channel, modelName, endpointType)
}

func codexChannelHasSchedulableAccount(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	keys := channel.GetKeys()
	if len(keys) == 0 && strings.TrimSpace(channel.Key) != "" {
		keys = []string{channel.Key}
	}
	if len(keys) == 0 {
		return false
	}
	for index := range keys {
		if channel.ChannelInfo.IsMultiKey && !channelKeyEnabledForCapability(channel, index) {
			continue
		}
		capability, ok := channel.ChannelInfo.MultiKeyCapabilities[index]
		if !ok || ChannelAccountCapabilityAllowsScheduling(capability) {
			return true
		}
	}
	return false
}

func codexChannelHasResponsesCapableAccount(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	if len(channel.ChannelInfo.MultiKeyCapabilities) == 0 {
		return true
	}
	for index, capability := range channel.ChannelInfo.MultiKeyCapabilities {
		if channel.ChannelInfo.IsMultiKey && !channelKeyEnabledForCapability(channel, index) {
			continue
		}
		if !ChannelAccountCapabilityAllowsScheduling(capability) {
			continue
		}
		if capability.CodexBackendResponsesStreamWrite == nil || capability.HasCodexBackendResponsesStreamAllowed() {
			return true
		}
	}
	return false
}

func codexChannelHasCompactCapableAccount(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	if len(channel.ChannelInfo.MultiKeyCapabilities) == 0 {
		return true
	}
	for index, capability := range channel.ChannelInfo.MultiKeyCapabilities {
		if channel.ChannelInfo.IsMultiKey && !channelKeyEnabledForCapability(channel, index) {
			continue
		}
		if !ChannelAccountCapabilityAllowsScheduling(capability) {
			continue
		}
		if capability.CodexBackendCompactWrite == nil || capability.HasCodexBackendCompactAllowed() {
			return true
		}
	}
	return false
}

func channelKeyEnabledForCapability(channel *model.Channel, index int) bool {
	if channel == nil || !channel.ChannelInfo.IsMultiKey || channel.ChannelInfo.MultiKeyStatusList == nil {
		return true
	}
	status, ok := channel.ChannelInfo.MultiKeyStatusList[index]
	return !ok || status == common.ChannelStatusEnabled
}

func openAICodexOAuthChannelApplies(channel *model.Channel, endpointType constant.EndpointType) bool {
	if channel == nil || channel.Type != constant.ChannelTypeOpenAI {
		return false
	}
	switch endpointType {
	case constant.EndpointTypeOpenAIResponse, constant.EndpointTypeOpenAIResponseCompact:
	default:
		return false
	}
	for _, key := range channel.GetKeys() {
		if codexauth.IsOAuthJSONCredential(key) {
			return true
		}
	}
	return false
}

func openAICodexOAuthChannelHasResponsesCapableAccount(channel *model.Channel) bool {
	return openAICodexOAuthChannelHasCapableAccount(channel, constant.EndpointTypeOpenAIResponse)
}

func openAICodexOAuthChannelHasCompactCapableAccount(channel *model.Channel) bool {
	return openAICodexOAuthChannelHasCapableAccount(channel, constant.EndpointTypeOpenAIResponseCompact)
}

func openAICodexOAuthChannelHasCapableAccount(channel *model.Channel, endpointType constant.EndpointType) bool {
	if channel == nil {
		return false
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return false
	}
	for index, key := range keys {
		if !codexauth.IsOAuthJSONCredential(key) || !channelKeyEnabledForCapability(channel, index) {
			continue
		}
		capability, ok := channel.ChannelInfo.MultiKeyCapabilities[index]
		if !ok {
			return true
		}
		if !ChannelAccountCapabilityAllowsScheduling(capability) {
			continue
		}
		if endpointType == constant.EndpointTypeOpenAIResponseCompact {
			if capability.CodexBackendCompactWrite == nil || capability.HasCodexBackendCompactAllowed() {
				return true
			}
			continue
		}
		if capability.CodexBackendResponsesStreamWrite == nil || capability.HasCodexBackendResponsesStreamAllowed() {
			return true
		}
	}
	return false
}

func ChannelSupportsCodexImageGenerationTool(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	return channelcapability.SupportsCodexImageGenerationTool(channel.Type, channel.GetOtherSettings())
}

func selectChannelForGroup(ctx *gin.Context, group string, modelName string, endpointType constant.EndpointType, requiresCodexImageTool bool, retry int, allowUsedChannelFallback bool) (*model.Channel, error) {
	excludedChannelIDs := getUsedChannelSet(ctx)
	selectionSkippedChannelIDs := getSelectionSkippedChannelSet(ctx)
	balanceSkippedChannelIDs := getBalanceSkippedChannelSet(ctx)
	avoidedChannelIDs := getAvoidedChannelSet()
	timeoutRecoveryChannelIDs := getTimeoutRecoveryChannelSet()
	balanceInsufficientChannelIDs := getBalanceInsufficientChannelSet()
	excludedWithAvoided := mergeChannelSets(excludedChannelIDs, selectionSkippedChannelIDs, balanceSkippedChannelIDs, balanceInsufficientChannelIDs, avoidedChannelIDs, getChannelConcurrencyCooldownSet())
	channel, err := selectNonFullChannel(group, modelName, endpointType, requiresCodexImageTool, retry, excludedWithAvoided)
	if err != nil {
		return nil, err
	}
	if channel != nil {
		return channel, nil
	}
	if fallbackAvoidedChannelIDs := subtractChannelSet(avoidedChannelIDs, timeoutRecoveryChannelIDs); len(fallbackAvoidedChannelIDs) > 0 && allowUsedChannelFallback {
		// Prefer a temporarily avoided channel over failing the request when no healthy peer exists.
		channel, err = selectNonFullChannel(group, modelName, endpointType, requiresCodexImageTool, retry, mergeChannelSets(excludedChannelIDs, selectionSkippedChannelIDs, balanceSkippedChannelIDs, balanceInsufficientChannelIDs, timeoutRecoveryChannelIDs))
		if err != nil {
			return nil, err
		}
		if channel != nil {
			logger.LogWarn(ctx, "All available channels are temporarily avoided; falling back to an avoided channel")
			return channel, nil
		}
	}
	if !allowUsedChannelFallback || len(excludedChannelIDs) == 0 {
		return channel, nil
	}
	// All peer channels in the current priority/group have been tried. Allow reusing an
	// already-used channel so multi-key channels can continue rotating to another key.
	if fallbackAvoidedChannelIDs := subtractChannelSet(avoidedChannelIDs, timeoutRecoveryChannelIDs); len(fallbackAvoidedChannelIDs) > 0 {
		channel, err = selectNonFullChannel(group, modelName, endpointType, requiresCodexImageTool, retry, mergeChannelSets(fallbackAvoidedChannelIDs, timeoutRecoveryChannelIDs, selectionSkippedChannelIDs, balanceSkippedChannelIDs, balanceInsufficientChannelIDs))
		if err != nil {
			return nil, err
		}
		if channel != nil {
			return channel, nil
		}
	}
	return selectNonFullChannel(group, modelName, endpointType, requiresCodexImageTool, retry, mergeChannelSets(selectionSkippedChannelIDs, balanceSkippedChannelIDs, balanceInsufficientChannelIDs))
}

func hasAlternativeChannelInGroup(ctx *gin.Context, group string, modelName string, endpointType constant.EndpointType, requiresCodexImageTool bool, retry int) bool {
	channel, err := selectNonFullChannel(group, modelName, endpointType, requiresCodexImageTool, retry, mergeChannelSets(getUsedChannelSet(ctx), getSelectionSkippedChannelSet(ctx), getBalanceSkippedChannelSet(ctx), getBalanceInsufficientChannelSet(), getAvoidedChannelSet(), getChannelConcurrencyCooldownSet()))
	return err == nil && channel != nil
}

func selectNonFullChannel(group string, modelName string, endpointType constant.EndpointType, requiresCodexImageTool bool, retry int, excludedChannelIDs map[int]struct{}) (*model.Channel, error) {
	excluded := mergeChannelSets(excludedChannelIDs, getChannelConcurrencyCooldownSet())
	for {
		channel, err := model.GetRandomSatisfiedChannel(group, modelName, retry, excluded)
		if err != nil || channel == nil {
			return channel, err
		}
		if !ChannelSupportsRequiredCapabilities(channel, modelName, endpointType, requiresCodexImageTool) {
			excluded[channel.Id] = struct{}{}
			continue
		}
		return channel, nil
	}
}

func resolveAutoGroupCursor(ctx *gin.Context, autoGroups []string) (string, int) {
	currentGroup := common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup)
	if currentGroup != "" {
		for i, group := range autoGroups {
			if group == currentGroup {
				return currentGroup, i
			}
		}
	}
	if index, exists := common.GetContextKey(ctx, constant.ContextKeyAutoGroupIndex); exists {
		if idx, ok := index.(int); ok && idx >= 0 && idx < len(autoGroups) {
			return autoGroups[idx], idx
		}
	}
	if len(autoGroups) == 0 {
		return "", 0
	}
	return autoGroups[0], 0
}

func GetChannelFailoverPlan(param *RetryParam) (bool, bool) {
	if param == nil || param.Ctx == nil {
		return false, false
	}
	if param.TokenGroup != "auto" {
		return hasAlternativeChannelInGroup(param.Ctx, param.TokenGroup, param.ModelName, param.EndpointType, param.RequiresCodexImageTool, param.GetRetry()), false
	}
	autoGroups := GetUserAutoGroup(common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup))
	if len(autoGroups) == 0 {
		return false, false
	}
	currentGroup, currentIndex := resolveAutoGroupCursor(param.Ctx, autoGroups)
	if currentGroup != "" && hasAlternativeChannelInGroup(param.Ctx, currentGroup, param.ModelName, param.EndpointType, param.RequiresCodexImageTool, param.GetRetry()) {
		return true, false
	}
	for i := currentIndex + 1; i < len(autoGroups); i++ {
		if hasAlternativeChannelInGroup(param.Ctx, autoGroups[i], param.ModelName, param.EndpointType, param.RequiresCodexImageTool, 0) {
			return true, true
		}
	}
	return false, false
}

func GetConcurrencyLimitFailoverPlan(param *RetryParam) (bool, bool) {
	return GetChannelFailoverPlan(param)
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// It prefers untried channels within the current group first, and only moves to lower priorities
// or the next auto-group when the current candidate set is exhausted.
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		if len(setting.GetAutoGroups()) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}
		autoGroups := GetUserAutoGroup(userGroup)

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			priorityRetry := param.GetRetry()
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)
			forceNextAutoGroup := common.GetContextKeyBool(param.Ctx, constant.ContextKeyForceNextAutoGroup)
			if forceNextAutoGroup {
				if channel, selectErr := selectChannelForGroup(param.Ctx, autoGroup, param.ModelName, param.EndpointType, param.RequiresCodexImageTool, priorityRetry, false); selectErr != nil {
					return nil, autoGroup, selectErr
				} else if channel != nil {
					common.SetContextKey(param.Ctx, constant.ContextKeyForceNextAutoGroup, false)
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
					selectGroup = autoGroup
					logger.LogDebug(param.Ctx, "Using remaining peer channel in auto group %s before crossing groups", autoGroup)
					return channel, selectGroup, nil
				}
				common.SetContextKey(param.Ctx, constant.ContextKeyForceNextAutoGroup, false)
				logger.LogDebug(param.Ctx, "Force skipping auto group %s due to upstream concurrency limit failover", autoGroup)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				param.SetRetry(0)
				continue
			}

			channel, err = selectChannelForGroup(param.Ctx, autoGroup, param.ModelName, param.EndpointType, param.RequiresCodexImageTool, priorityRetry, false)
			if err != nil {
				return nil, autoGroup, err
			}
			if channel == nil {
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				param.SetRetry(0)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			if crossGroupRetry {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
		if channel == nil && len(getAvoidedChannelSet()) > 0 {
			usedChannelIDs := getUsedChannelSet(param.Ctx)
			selectionSkippedChannelIDs := getSelectionSkippedChannelSet(param.Ctx)
			balanceSkippedChannelIDs := getBalanceSkippedChannelSet(param.Ctx)
			balanceInsufficientChannelIDs := getBalanceInsufficientChannelSet()
			for i := startGroupIndex; i < len(autoGroups); i++ {
				autoGroup := autoGroups[i]
				channel, err = selectNonFullChannel(autoGroup, param.ModelName, param.EndpointType, param.RequiresCodexImageTool, param.GetRetry(), mergeChannelSets(usedChannelIDs, selectionSkippedChannelIDs, balanceSkippedChannelIDs, balanceInsufficientChannelIDs, getChannelConcurrencyCooldownSet()))
				if err != nil {
					return nil, autoGroup, err
				}
				if channel != nil {
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
					selectGroup = autoGroup
					logger.LogWarn(param.Ctx, "All auto-group candidates are temporarily avoided; falling back to an avoided channel")
					break
				}
			}
		}
	} else {
		channel, err = selectChannelForGroup(param.Ctx, param.TokenGroup, param.ModelName, param.EndpointType, param.RequiresCodexImageTool, param.GetRetry(), true)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}
