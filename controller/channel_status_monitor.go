package controller

import (
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	modelgatewayobservability "github.com/QuantumNous/new-api/pkg/modelgateway/observability"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

const (
	channelStatusMonitorDefaultHours = 24
	channelStatusMonitorMaxHours     = 24 * 30
	channelStatusMonitorRecentLimit  = 60
	channelStatusMonitorLogLimit     = 5000
	channelStatusMonitorCacheTTL     = 60 * time.Second
	channelStatusMonitorErrorTTL     = 60 * time.Second
	channelStatusMonitorPartialTTL   = 15 * time.Second
	channelStatusMonitorColdWait     = 900 * time.Millisecond
	channelStatusMonitorRuntimeLimit = 8
)

var channelStatusMonitorCache = struct {
	sync.Mutex
	items    map[int]channelStatusMonitorCacheItem
	inFlight map[int]chan struct{}
}{
	items:    make(map[int]channelStatusMonitorCacheItem),
	inFlight: make(map[int]chan struct{}),
}

type channelStatusMonitorCacheItem struct {
	result    ChannelStatusMonitorResponse
	expiresAt time.Time
}

type ChannelStatusMonitorItem struct {
	ID                     int                                           `json:"id"`
	Name                   string                                        `json:"name"`
	Type                   int                                           `json:"type"`
	Status                 int                                           `json:"status"`
	Group                  string                                        `json:"group"`
	Models                 string                                        `json:"models"`
	Priority               int64                                         `json:"priority"`
	ResponseTime           int                                           `json:"response_time"`
	TestTime               int64                                         `json:"test_time"`
	CreatedTime            int64                                         `json:"created_time"`
	Enabled                bool                                          `json:"enabled"`
	ActiveConcurrency      int                                           `json:"active_concurrency"`
	MaxConcurrency         int                                           `json:"max_concurrency"`
	ConcurrencyCeiling     int                                           `json:"concurrency_ceiling"`
	ConcurrencyCooldown    int64                                         `json:"concurrency_cooldown_remaining_seconds,omitempty"`
	ConcurrencyReason      string                                        `json:"concurrency_reason,omitempty"`
	FailureAvoidance       int64                                         `json:"failure_avoidance_remaining_seconds,omitempty"`
	FailureReason          string                                        `json:"failure_reason,omitempty"`
	PauseType              string                                        `json:"pause_type,omitempty"`
	PauseReason            string                                        `json:"pause_reason,omitempty"`
	PauseUntil             int64                                         `json:"pause_until,omitempty"`
	PauseRemaining         int64                                         `json:"pause_remaining_seconds,omitempty"`
	LastRequestAt          int64                                         `json:"last_request_at,omitempty"`
	LastSuccessAt          int64                                         `json:"last_success_at,omitempty"`
	LastFailureAt          int64                                         `json:"last_failure_at,omitempty"`
	RecentRequests         int64                                         `json:"recent_requests"`
	RecentSuccesses        int64                                         `json:"recent_successes"`
	RecentFailures         int64                                         `json:"recent_failures"`
	RecentHealthProbes     int64                                         `json:"recent_health_probes"`
	RecentError429         int64                                         `json:"recent_error_429"`
	RecentError5xx         int64                                         `json:"recent_error_5xx"`
	RecentErrorTimeout     int64                                         `json:"recent_error_timeout"`
	RecentClientAborted    int64                                         `json:"recent_client_aborted"`
	RecentRecovered        int64                                         `json:"recent_recovered"`
	RecentEmptyOutputs     int64                                         `json:"recent_empty_outputs"`
	RecentExperienceIssues int64                                         `json:"recent_experience_issues"`
	RecentBalanceErrors    int64                                         `json:"recent_balance_errors"`
	RecentErrorRateLimit   int64                                         `json:"recent_error_rate_limit"`
	RecentStreamErrors     int64                                         `json:"recent_stream_errors"`
	RecentAvgLatencyMs     int64                                         `json:"recent_avg_latency_ms"`
	RecentAvgFirstRespMs   int64                                         `json:"recent_avg_first_response_ms"`
	RecentAvgOutputTokens  int64                                         `json:"recent_avg_output_tokens"`
	RecentTotalTokens      int64                                         `json:"recent_total_tokens"`
	SuccessRate            float64                                       `json:"success_rate"`
	HealthScore            int                                           `json:"health_score"`
	HealthState            string                                        `json:"health_state"`
	Runtime                *ChannelStatusMonitorRuntimeStats             `json:"runtime,omitempty"`
	RuntimeItems           []modelgatewayobservability.RuntimeStatusItem `json:"runtime_items,omitempty"`
}

type ChannelStatusMonitorGroup struct {
	Group                  string                            `json:"group"`
	GroupRatio             float64                           `json:"group_ratio"`
	TotalChannels          int                               `json:"total_channels"`
	EnabledChannels        int                               `json:"enabled_channels"`
	DisabledChannels       int                               `json:"disabled_channels"`
	BusyChannels           int                               `json:"busy_channels"`
	CooldownChannels       int                               `json:"cooldown_channels"`
	BadChannels            int                               `json:"bad_channels"`
	HealthyChannels        int                               `json:"healthy_channels"`
	RecentRequests         int64                             `json:"recent_requests"`
	RecentSuccesses        int64                             `json:"recent_successes"`
	RecentFailures         int64                             `json:"recent_failures"`
	RecentHealthProbes     int64                             `json:"recent_health_probes"`
	RecentError429         int64                             `json:"recent_error_429"`
	RecentError5xx         int64                             `json:"recent_error_5xx"`
	RecentErrorTimeout     int64                             `json:"recent_error_timeout"`
	RecentClientAborted    int64                             `json:"recent_client_aborted"`
	RecentRecovered        int64                             `json:"recent_recovered"`
	RecentEmptyOutputs     int64                             `json:"recent_empty_outputs"`
	RecentExperienceIssues int64                             `json:"recent_experience_issues"`
	SuccessRate            float64                           `json:"success_rate"`
	AvgLatencyMs           int64                             `json:"avg_latency_ms"`
	AvgTTFTMs              int64                             `json:"avg_ttft_ms"`
	RecentStatus           []string                          `json:"recent_status"`
	RecentStatusSource     string                            `json:"recent_status_source,omitempty"`
	Channels               []*ChannelStatusMonitorItem       `json:"channels"`
	Runtime                *ChannelStatusMonitorRuntimeStats `json:"runtime,omitempty"`
}

type ChannelStatusMonitorSummary struct {
	WindowHours            int                               `json:"window_hours"`
	TotalGroups            int                               `json:"total_groups"`
	TotalChannels          int                               `json:"total_channels"`
	EnabledChannels        int                               `json:"enabled_channels"`
	DisabledChannels       int                               `json:"disabled_channels"`
	BusyChannels           int                               `json:"busy_channels"`
	CooldownChannels       int                               `json:"cooldown_channels"`
	BadChannels            int                               `json:"bad_channels"`
	HealthyChannels        int                               `json:"healthy_channels"`
	RecentRequests         int64                             `json:"recent_requests"`
	RecentSuccesses        int64                             `json:"recent_successes"`
	RecentFailures         int64                             `json:"recent_failures"`
	RecentHealthProbes     int64                             `json:"recent_health_probes"`
	RecentError429         int64                             `json:"recent_error_429"`
	RecentError5xx         int64                             `json:"recent_error_5xx"`
	RecentErrorTimeout     int64                             `json:"recent_error_timeout"`
	RecentClientAborted    int64                             `json:"recent_client_aborted"`
	RecentRecovered        int64                             `json:"recent_recovered"`
	RecentEmptyOutputs     int64                             `json:"recent_empty_outputs"`
	RecentExperienceIssues int64                             `json:"recent_experience_issues"`
	SuccessRate            float64                           `json:"success_rate"`
	AvgLatencyMs           int64                             `json:"avg_latency_ms"`
	AvgTTFTMs              int64                             `json:"avg_ttft_ms"`
	Runtime                *ChannelStatusMonitorRuntimeStats `json:"runtime,omitempty"`
}

type ChannelStatusMonitorRuntimeStats struct {
	RuntimeKeys                 int     `json:"runtime_keys"`
	Channels                    int     `json:"channels"`
	AvailableRuntimeKeys        int     `json:"available_runtime_keys"`
	HealthyRuntimeKeys          int     `json:"healthy_runtime_keys"`
	RiskRuntimeKeys             int     `json:"risk_runtime_keys"`
	CircuitOpenRuntimeKeys      int     `json:"circuit_open_runtime_keys"`
	CircuitHalfOpenRuntimeKeys  int     `json:"circuit_half_open_runtime_keys"`
	CooldownRuntimeKeys         int     `json:"cooldown_runtime_keys"`
	FailureAvoidanceRuntimeKeys int     `json:"failure_avoidance_runtime_keys"`
	HighPressureRuntimeKeys     int     `json:"high_pressure_runtime_keys"`
	ConfigIsolatedRuntimeKeys   int     `json:"config_isolated_runtime_keys"`
	AvgScore                    float64 `json:"avg_score"`
	AvgRoutingScore             float64 `json:"avg_routing_score"`
	AvgSuccessRate              float64 `json:"avg_success_rate"`
	AvgTTFTMs                   float64 `json:"avg_ttft_ms"`
	AvgDurationMs               float64 `json:"avg_duration_ms"`
	AvgCostItemScore            float64 `json:"avg_cost_item_score"`
	AvgCostRatio                float64 `json:"avg_cost_ratio"`
	CostPricingMode             string  `json:"cost_pricing_mode,omitempty"`
	AvgGroupPriorityRatio       float64 `json:"avg_group_priority_ratio"`
	AvgEmptyOutputRate          float64 `json:"avg_empty_output_rate"`
	AvgExperienceIssueRate      float64 `json:"avg_experience_issue_rate"`
	SampleCount                 int     `json:"sample_count"`
	RealSampleCount30m          int     `json:"real_sample_count_30m"`
	ActiveConcurrency           int     `json:"active_concurrency"`
	MaxConcurrency              int     `json:"max_concurrency"`
	QueueDepth                  int     `json:"queue_depth"`
	QueueCapacity               int     `json:"queue_capacity"`
	FirstBytePending            int     `json:"first_byte_pending"`
	SlowFirstBytePending        int     `json:"slow_first_byte_pending"`
	OldestFirstByteWaitMs       float64 `json:"oldest_first_byte_wait_ms"`
	LastRealAttemptAt           int64   `json:"last_real_attempt_at,omitempty"`
	LastRealSuccessAt           int64   `json:"last_real_success_at,omitempty"`
	LastRealFailureAt           int64   `json:"last_real_failure_at,omitempty"`
	LastProbeAt                 int64   `json:"last_probe_at,omitempty"`
	LastProbeSuccessAt          int64   `json:"last_probe_success_at,omitempty"`
	HealthStatus                string  `json:"health_status"`
}

type ChannelStatusMonitorResponse struct {
	Summary ChannelStatusMonitorSummary  `json:"summary"`
	Groups  []*ChannelStatusMonitorGroup `json:"groups"`
	Partial bool                         `json:"partial,omitempty"`
}

func GetChannelStatusMonitor(c *gin.Context) {
	windowHours := channelStatusMonitorDefaultHours
	if raw := c.Query("hours"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			windowHours = parsed
		}
	}

	result, err := buildChannelStatusMonitorCached(windowHours)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func buildChannelStatusMonitorCached(windowHours int) (ChannelStatusMonitorResponse, error) {
	normalizedHours := normalizeChannelStatusMonitorWindowHours(windowHours)
	now := time.Now()
	channelStatusMonitorCache.Lock()
	if cached, ok := channelStatusMonitorCache.items[normalizedHours]; ok && now.Before(cached.expiresAt) {
		result := cached.result
		channelStatusMonitorCache.Unlock()
		return result, nil
	}
	if cached, ok := channelStatusMonitorCache.items[normalizedHours]; ok {
		if _, refreshing := channelStatusMonitorCache.inFlight[normalizedHours]; !refreshing {
			done := make(chan struct{})
			channelStatusMonitorCache.inFlight[normalizedHours] = done
			go refreshChannelStatusMonitorCache(normalizedHours, done)
		}
		result := cached.result
		channelStatusMonitorCache.Unlock()
		return result, nil
	}
	if done, ok := channelStatusMonitorCache.inFlight[normalizedHours]; ok {
		channelStatusMonitorCache.Unlock()
		return waitForChannelStatusMonitorCache(normalizedHours, done)
	}
	done := make(chan struct{})
	channelStatusMonitorCache.inFlight[normalizedHours] = done
	channelStatusMonitorCache.Unlock()

	go refreshChannelStatusMonitorCache(normalizedHours, done)
	return waitForChannelStatusMonitorCache(normalizedHours, done)
}

func refreshChannelStatusMonitorCache(windowHours int, done chan struct{}) {
	result, err := buildChannelStatusMonitor(windowHours)
	finishChannelStatusMonitorCacheRefresh(windowHours, done, result, err)
}

func waitForChannelStatusMonitorCache(windowHours int, done chan struct{}) (ChannelStatusMonitorResponse, error) {
	select {
	case <-done:
		channelStatusMonitorCache.Lock()
		if cached, ok := channelStatusMonitorCache.items[windowHours]; ok {
			result := cached.result
			channelStatusMonitorCache.Unlock()
			return result, nil
		}
		channelStatusMonitorCache.Unlock()
	case <-time.After(channelStatusMonitorColdWait):
		return getOrBuildChannelStatusMonitorPartial(windowHours)
	}
	return getOrBuildChannelStatusMonitorPartial(windowHours)
}

func finishChannelStatusMonitorCacheRefresh(windowHours int, done chan struct{}, result ChannelStatusMonitorResponse, err error) {
	cacheTTL := channelStatusMonitorCacheTTL
	if err != nil {
		if partial, fallbackErr := buildChannelStatusMonitorPartial(windowHours); fallbackErr == nil {
			result = partial
			err = nil
			cacheTTL = channelStatusMonitorErrorTTL
		}
	}
	defer close(done)
	channelStatusMonitorCache.Lock()
	defer channelStatusMonitorCache.Unlock()
	delete(channelStatusMonitorCache.inFlight, windowHours)
	if err != nil {
		return
	}
	channelStatusMonitorCache.items[windowHours] = channelStatusMonitorCacheItem{
		result:    result,
		expiresAt: time.Now().Add(cacheTTL),
	}
}

func normalizeChannelStatusMonitorWindowHours(windowHours int) int {
	if windowHours <= 0 {
		return channelStatusMonitorDefaultHours
	}
	if windowHours > channelStatusMonitorMaxHours {
		return channelStatusMonitorMaxHours
	}
	return windowHours
}

func buildChannelStatusMonitor(windowHours int) (ChannelStatusMonitorResponse, error) {
	return buildChannelStatusMonitorWithLogLimit(windowHours, channelStatusMonitorLogLimit)
}

func buildChannelStatusMonitorBase(windowHours int) (ChannelStatusMonitorResponse, error) {
	return buildChannelStatusMonitorFromRows(windowHours, nil, nil)
}

func buildChannelStatusMonitorPartial(windowHours int) (ChannelStatusMonitorResponse, error) {
	result, err := buildChannelStatusMonitorBase(windowHours)
	if err != nil {
		return result, err
	}
	result.Partial = true
	applyChannelStatusMonitorRuntimeStatus(&result, buildChannelStatusMonitorRuntimeStatus())
	return result, nil
}

func getOrBuildChannelStatusMonitorPartial(windowHours int) (ChannelStatusMonitorResponse, error) {
	now := time.Now()
	channelStatusMonitorCache.Lock()
	if cached, ok := channelStatusMonitorCache.items[windowHours]; ok && now.Before(cached.expiresAt) {
		result := cached.result
		channelStatusMonitorCache.Unlock()
		return result, nil
	}
	channelStatusMonitorCache.Unlock()

	result, err := buildChannelStatusMonitorPartial(windowHours)
	if err != nil {
		return result, err
	}

	channelStatusMonitorCache.Lock()
	defer channelStatusMonitorCache.Unlock()
	if cached, ok := channelStatusMonitorCache.items[windowHours]; ok && time.Now().Before(cached.expiresAt) && !cached.result.Partial {
		return cached.result, nil
	}
	channelStatusMonitorCache.items[windowHours] = channelStatusMonitorCacheItem{
		result:    result,
		expiresAt: time.Now().Add(channelStatusMonitorPartialTTL),
	}
	return result, nil
}

func buildChannelStatusMonitorWithLogLimit(windowHours int, logLimit int) (ChannelStatusMonitorResponse, error) {
	windowHours = normalizeChannelStatusMonitorWindowHours(windowHours)
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		return ChannelStatusMonitorResponse{}, err
	}
	groupNames := make([]string, 0, len(channels))
	for _, channel := range channels {
		if shouldIncludeChannelInStatusMonitor(channel) {
			groupNames = append(groupNames, channelMonitorGroups(channel)...)
		}
	}

	startTs := time.Now().Add(-time.Duration(windowHours) * time.Hour).Unix()
	userRequestLimit := logLimit
	if userRequestLimit <= 0 {
		userRequestLimit = channelStatusMonitorLogLimit
	}
	userRequestRows, err := model.GetChannelStatusMonitorUserRequests(startTs, groupNames, userRequestLimit)
	if err != nil {
		return ChannelStatusMonitorResponse{}, err
	}
	recentUserRequestRows, err := model.GetChannelStatusMonitorRecentUserRequestsByGroups(groupNames, channelStatusMonitorRecentLimit)
	if err != nil {
		recentUserRequestRows = nil
	}
	result := buildChannelStatusMonitorFromRowsWithChannelsAndUserRequests(windowHours, channels, nil, nil, userRequestRows, recentUserRequestRows)
	applyChannelStatusMonitorRuntimeStatus(&result, buildChannelStatusMonitorRuntimeStatus())
	return result, nil
}

func buildChannelStatusMonitorFromRows(windowHours int, logRows []model.ChannelStatusMonitorLogRow, recentLogRows []model.ChannelStatusMonitorRecentLogRow) (ChannelStatusMonitorResponse, error) {
	windowHours = normalizeChannelStatusMonitorWindowHours(windowHours)
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		return ChannelStatusMonitorResponse{}, err
	}
	return buildChannelStatusMonitorFromRowsWithChannels(windowHours, channels, logRows, recentLogRows), nil
}

func buildChannelStatusMonitorFromRowsWithChannels(windowHours int, channels []*model.Channel, logRows []model.ChannelStatusMonitorLogRow, recentLogRows []model.ChannelStatusMonitorRecentLogRow) ChannelStatusMonitorResponse {
	return buildChannelStatusMonitorFromRowsWithChannelsAndUserRequests(windowHours, channels, logRows, recentLogRows, nil, nil)
}

func buildChannelStatusMonitorFromRowsWithChannelsAndUserRequests(windowHours int, channels []*model.Channel, logRows []model.ChannelStatusMonitorLogRow, recentLogRows []model.ChannelStatusMonitorRecentLogRow, userRequestRows []model.ModelGatewayUserRequestSummary, recentUserRequestRows []model.ModelGatewayUserRequestSummary) ChannelStatusMonitorResponse {
	logStats := buildChannelMonitorLogStats(logRows)
	userRequestStats := buildChannelMonitorUserRequestStats(userRequestRows)
	recentUserStatusByGroup := buildChannelMonitorRecentUserRequestStatus(recentUserRequestRows, channelStatusMonitorRecentLimit)
	groupMap := map[string]*ChannelStatusMonitorGroup{}
	summary := ChannelStatusMonitorSummary{WindowHours: windowHours}
	for _, channel := range channels {
		if !shouldIncludeChannelInStatusMonitor(channel) {
			continue
		}
		summary.TotalChannels++

		item := buildChannelStatusMonitorItem(channel, channelMonitorPreferredStats(userRequestStats.totalByChannel[channel.Id], logStats.totalByChannel[channel.Id]))
		if item.Enabled {
			summary.EnabledChannels++
		} else {
			summary.DisabledChannels++
		}
		if item.ActiveConcurrency > 0 {
			summary.BusyChannels++
		}
		if item.FailureAvoidance > 0 {
			summary.CooldownChannels++
		}
		if item.HealthState == "healthy" {
			summary.HealthyChannels++
		} else {
			summary.BadChannels++
		}

		for _, groupName := range channelMonitorGroups(channel) {
			group := groupMap[groupName]
			if group == nil {
				group = &ChannelStatusMonitorGroup{
					Group:      groupName,
					GroupRatio: ratio_setting.GetGroupRatio(groupName),
				}
				groupMap[groupName] = group
			}
			groupItem := buildChannelStatusMonitorItem(channel, channelMonitorPreferredStats(userRequestStats.byChannelGroup[channel.Id][groupName], logStats.byChannelGroup[channel.Id][groupName]))
			groupItem.Group = groupName
			group.Channels = append(group.Channels, groupItem)
			group.TotalChannels++
			if groupItem.Enabled {
				group.EnabledChannels++
			} else {
				group.DisabledChannels++
			}
			if groupItem.ActiveConcurrency > 0 {
				group.BusyChannels++
			}
			if groupItem.FailureAvoidance > 0 {
				group.CooldownChannels++
			}
			if groupItem.HealthState == "healthy" {
				group.HealthyChannels++
			} else {
				group.BadChannels++
			}
		}
	}

	groups := make([]*ChannelStatusMonitorGroup, 0, len(groupMap))
	for _, group := range groupMap {
		if groupStat := userRequestStats.byGroup[group.Group]; groupStat != nil {
			group.RecentRequests = groupStat.requests
			group.RecentSuccesses = groupStat.successes
			group.RecentFailures = groupStat.failures
			group.RecentHealthProbes = groupStat.healthProbes
			group.RecentError429 = groupStat.error429
			group.RecentError5xx = groupStat.error5xx
			group.RecentErrorTimeout = groupStat.errorTimeout
			group.RecentClientAborted = groupStat.clientAborted
			group.RecentRecovered = groupStat.recovered
			group.RecentEmptyOutputs = groupStat.emptyOutputs
			group.RecentExperienceIssues = groupStat.experienceIssues
			group.AvgLatencyMs = avgInt64(groupStat.latencySum, groupStat.latencyCount)
			group.AvgTTFTMs = avgInt64(groupStat.firstRespSum, groupStat.firstRespCount)
		}
		if group.RecentRequests > 0 {
			group.SuccessRate = channelMonitorUserSuccessRate(group.RecentSuccesses, group.RecentRequests, group.RecentClientAborted, group.RecentHealthProbes)
		}
		if len(group.Channels) > 0 {
			sort.Slice(group.Channels, func(i, j int) bool {
				if group.Channels[i].HealthScore == group.Channels[j].HealthScore {
					return group.Channels[i].RecentRequests > group.Channels[j].RecentRequests
				}
				return group.Channels[i].HealthScore < group.Channels[j].HealthScore
			})
		}
		if statuses := recentUserStatusByGroup[group.Group]; len(statuses) > 0 {
			group.RecentStatus = statuses
			group.RecentStatusSource = "user_requests"
		}
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].RecentRequests == groups[j].RecentRequests {
			return groups[i].Group < groups[j].Group
		}
		return groups[i].RecentRequests > groups[j].RecentRequests
	})
	summary.TotalGroups = len(groups)
	if overallStat := userRequestStats.overall; overallStat != nil {
		summary.RecentRequests = overallStat.requests
		summary.RecentSuccesses = overallStat.successes
		summary.RecentFailures = overallStat.failures
		summary.RecentHealthProbes = overallStat.healthProbes
		summary.RecentError429 = overallStat.error429
		summary.RecentError5xx = overallStat.error5xx
		summary.RecentErrorTimeout = overallStat.errorTimeout
		summary.RecentClientAborted = overallStat.clientAborted
		summary.RecentRecovered = overallStat.recovered
		summary.RecentEmptyOutputs = overallStat.emptyOutputs
		summary.RecentExperienceIssues = overallStat.experienceIssues
		summary.AvgLatencyMs = avgInt64(overallStat.latencySum, overallStat.latencyCount)
		summary.AvgTTFTMs = avgInt64(overallStat.firstRespSum, overallStat.firstRespCount)
	}
	if summary.RecentRequests > 0 {
		summary.SuccessRate = channelMonitorUserSuccessRate(summary.RecentSuccesses, summary.RecentRequests, summary.RecentClientAborted, summary.RecentHealthProbes)
	}

	return ChannelStatusMonitorResponse{
		Summary: summary,
		Groups:  groups,
	}
}

func buildChannelStatusMonitorRuntimeStatus() modelgatewayobservability.RuntimeStatusResponse {
	return defaultModelGatewayRuntimeStatusService().Build(modelgatewayobservability.RuntimeStatusQuery{
		Limit: modelGatewayRuntimeStatusMaxLimit,
	})
}

type channelStatusRuntimeItemKey struct {
	group     string
	channelID int
}

type channelStatusRuntimeAgg struct {
	stats                         ChannelStatusMonitorRuntimeStats
	channelIDs                    map[int]struct{}
	activeConcurrencyByChannel    map[int]int
	maxConcurrencyByChannel       map[int]int
	queueDepthByChannel           map[int]int
	queueCapacityByChannel        map[int]int
	firstBytePendingByChannel     map[int]int
	slowFirstBytePendingByChannel map[int]int
	oldestFirstByteWaitByChannel  map[int]float64
	scoreSum                      float64
	scoreCount                    int
	routingScoreSum               float64
	routingScoreCount             int
	successRateSum                float64
	successRateCount              int
	ttftSum                       float64
	ttftCount                     int
	durationSum                   float64
	durationCount                 int
	costItemScoreSum              float64
	costItemScoreCount            int
	costRatioSum                  float64
	costRatioCount                int
	groupPriorityRatioSum         float64
	groupPriorityRatioCount       int
	emptyOutputRateSum            float64
	emptyOutputRateCount          int
	experienceIssueRateSum        float64
	experienceIssueRateCount      int
}

func newChannelStatusRuntimeAgg() *channelStatusRuntimeAgg {
	return &channelStatusRuntimeAgg{
		channelIDs:                    map[int]struct{}{},
		activeConcurrencyByChannel:    map[int]int{},
		maxConcurrencyByChannel:       map[int]int{},
		queueDepthByChannel:           map[int]int{},
		queueCapacityByChannel:        map[int]int{},
		firstBytePendingByChannel:     map[int]int{},
		slowFirstBytePendingByChannel: map[int]int{},
		oldestFirstByteWaitByChannel:  map[int]float64{},
	}
}

func applyChannelStatusMonitorRuntimeStatus(response *ChannelStatusMonitorResponse, runtimeStatus modelgatewayobservability.RuntimeStatusResponse) {
	if response == nil || len(runtimeStatus.Items) == 0 {
		return
	}
	groupByName := make(map[string]*ChannelStatusMonitorGroup, len(response.Groups))
	channelByRuntimeKey := make(map[channelStatusRuntimeItemKey]*ChannelStatusMonitorItem)
	groupAggs := map[string]*channelStatusRuntimeAgg{}
	channelAggs := map[channelStatusRuntimeItemKey]*channelStatusRuntimeAgg{}
	summaryAgg := newChannelStatusRuntimeAgg()

	for _, group := range response.Groups {
		if group == nil {
			continue
		}
		groupName := normalizeChannelGroup(group.Group)
		groupByName[groupName] = group
		for _, channel := range group.Channels {
			if channel == nil {
				continue
			}
			channelByRuntimeKey[channelStatusRuntimeItemKey{group: groupName, channelID: channel.ID}] = channel
		}
	}

	for _, item := range runtimeStatus.Items {
		if item.ChannelID <= 0 {
			continue
		}
		groupName := normalizeChannelGroup(item.Group)
		if _, ok := groupByName[groupName]; !ok {
			continue
		}
		summaryAgg.add(item)
		groupAgg := groupAggs[groupName]
		if groupAgg == nil {
			groupAgg = newChannelStatusRuntimeAgg()
			groupAggs[groupName] = groupAgg
		}
		groupAgg.add(item)

		key := channelStatusRuntimeItemKey{group: groupName, channelID: item.ChannelID}
		channel := channelByRuntimeKey[key]
		if channel == nil {
			continue
		}
		channel.RuntimeItems = append(channel.RuntimeItems, item)
		channelAgg := channelAggs[key]
		if channelAgg == nil {
			channelAgg = newChannelStatusRuntimeAgg()
			channelAggs[key] = channelAgg
		}
		channelAgg.add(item)
	}

	if summary := summaryAgg.finalize(); summary != nil {
		response.Summary.Runtime = summary
	}
	for groupName, agg := range groupAggs {
		if group := groupByName[groupName]; group != nil {
			group.Runtime = agg.finalize()
		}
	}
	for key, agg := range channelAggs {
		channel := channelByRuntimeKey[key]
		if channel == nil {
			continue
		}
		channel.Runtime = agg.finalize()
		if len(channel.RuntimeItems) > channelStatusMonitorRuntimeLimit {
			channel.RuntimeItems = channel.RuntimeItems[:channelStatusMonitorRuntimeLimit]
		}
	}
}

func (a *channelStatusRuntimeAgg) add(item modelgatewayobservability.RuntimeStatusItem) {
	if a == nil {
		return
	}
	a.stats.RuntimeKeys++
	if item.ChannelID > 0 {
		a.channelIDs[item.ChannelID] = struct{}{}
		setMaxInt(a.activeConcurrencyByChannel, item.ChannelID, item.ActiveConcurrency)
		setMaxInt(a.maxConcurrencyByChannel, item.ChannelID, runtimeMonitorConcurrencyLimit(item))
		setMaxInt(a.queueDepthByChannel, item.ChannelID, item.QueueDepth)
		setMaxInt(a.queueCapacityByChannel, item.ChannelID, item.QueueCapacity)
		setMaxInt(a.firstBytePendingByChannel, item.ChannelID, item.FirstBytePending)
		setMaxInt(a.slowFirstBytePendingByChannel, item.ChannelID, item.SlowFirstBytePending)
		setMaxFloat64(a.oldestFirstByteWaitByChannel, item.ChannelID, item.OldestFirstByteWaitMs)
	}
	if runtimeMonitorItemAvailable(item) {
		a.stats.AvailableRuntimeKeys++
	}
	if item.HealthStatus == "healthy" {
		a.stats.HealthyRuntimeKeys++
	}
	if runtimeMonitorItemAtRisk(item) {
		a.stats.RiskRuntimeKeys++
	}
	if item.CircuitOpen || item.CircuitState == "open" {
		a.stats.CircuitOpenRuntimeKeys++
	}
	if item.CircuitState == "half_open" {
		a.stats.CircuitHalfOpenRuntimeKeys++
	}
	if item.Cooldown {
		a.stats.CooldownRuntimeKeys++
	}
	if item.FailureAvoidance {
		a.stats.FailureAvoidanceRuntimeKeys++
	}
	if item.HealthStatus == "high_pressure" || runtimeMonitorPressureRatio(item) >= 0.9 {
		a.stats.HighPressureRuntimeKeys++
	}
	if item.ConfigErrorIsolated {
		a.stats.ConfigIsolatedRuntimeKeys++
	}
	addPositiveAverage(&a.scoreSum, &a.scoreCount, item.ScoreTotal)
	addPositiveAverage(&a.routingScoreSum, &a.routingScoreCount, item.RoutingScoreTotal)
	if item.SampleCount > 0 || item.SuccessRate > 0 {
		a.successRateSum += item.SuccessRate
		a.successRateCount++
	}
	addPositiveAverage(&a.ttftSum, &a.ttftCount, item.TTFTMs)
	addPositiveAverage(&a.durationSum, &a.durationCount, item.DurationMs)
	addPositiveAverage(&a.costItemScoreSum, &a.costItemScoreCount, item.ScoreBreakdown["cost"])
	addPositiveAverage(&a.costRatioSum, &a.costRatioCount, item.CostRatio)
	addPositiveAverage(&a.groupPriorityRatioSum, &a.groupPriorityRatioCount, item.GroupPriorityRatio)
	if item.SampleCount > 0 || item.EmptyOutputRate > 0 {
		a.emptyOutputRateSum += item.EmptyOutputRate
		a.emptyOutputRateCount++
	}
	if item.SampleCount > 0 || item.ExperienceIssueRate > 0 {
		a.experienceIssueRateSum += item.ExperienceIssueRate
		a.experienceIssueRateCount++
	}
	a.stats.SampleCount += item.SampleCount
	a.stats.RealSampleCount30m += item.RealSampleCount30m
	a.stats.LastRealAttemptAt = maxInt64(a.stats.LastRealAttemptAt, item.LastRealAttemptAt)
	a.stats.LastRealSuccessAt = maxInt64(a.stats.LastRealSuccessAt, item.LastRealSuccessAt)
	a.stats.LastRealFailureAt = maxInt64(a.stats.LastRealFailureAt, item.LastRealFailureAt)
	a.stats.LastProbeAt = maxInt64(a.stats.LastProbeAt, item.LastProbeAt)
	a.stats.LastProbeSuccessAt = maxInt64(a.stats.LastProbeSuccessAt, item.LastProbeSuccessAt)
	a.applyCostPricingMode(item.CostPricingMode)
}

func (a *channelStatusRuntimeAgg) finalize() *ChannelStatusMonitorRuntimeStats {
	if a == nil || a.stats.RuntimeKeys <= 0 {
		return nil
	}
	stats := a.stats
	stats.Channels = len(a.channelIDs)
	stats.ActiveConcurrency = sumIntMap(a.activeConcurrencyByChannel)
	stats.MaxConcurrency = sumIntMap(a.maxConcurrencyByChannel)
	stats.QueueDepth = sumIntMap(a.queueDepthByChannel)
	stats.QueueCapacity = sumIntMap(a.queueCapacityByChannel)
	stats.FirstBytePending = sumIntMap(a.firstBytePendingByChannel)
	stats.SlowFirstBytePending = sumIntMap(a.slowFirstBytePendingByChannel)
	stats.OldestFirstByteWaitMs = maxFloat64Map(a.oldestFirstByteWaitByChannel)
	stats.AvgScore = averageRuntimeMonitorValue(a.scoreSum, a.scoreCount)
	stats.AvgRoutingScore = averageRuntimeMonitorValue(a.routingScoreSum, a.routingScoreCount)
	stats.AvgSuccessRate = averageRuntimeMonitorValue(a.successRateSum, a.successRateCount)
	stats.AvgTTFTMs = averageRuntimeMonitorValue(a.ttftSum, a.ttftCount)
	stats.AvgDurationMs = averageRuntimeMonitorValue(a.durationSum, a.durationCount)
	stats.AvgCostItemScore = averageRuntimeMonitorValue(a.costItemScoreSum, a.costItemScoreCount)
	stats.AvgCostRatio = averageRuntimeMonitorValue(a.costRatioSum, a.costRatioCount)
	stats.AvgGroupPriorityRatio = averageRuntimeMonitorValue(a.groupPriorityRatioSum, a.groupPriorityRatioCount)
	stats.AvgEmptyOutputRate = averageRuntimeMonitorValue(a.emptyOutputRateSum, a.emptyOutputRateCount)
	stats.AvgExperienceIssueRate = averageRuntimeMonitorValue(a.experienceIssueRateSum, a.experienceIssueRateCount)
	stats.HealthStatus = channelStatusRuntimeHealthStatus(stats)
	return &stats
}

func (a *channelStatusRuntimeAgg) applyCostPricingMode(mode string) {
	if a == nil {
		return
	}
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return
	}
	if a.stats.CostPricingMode == "" {
		a.stats.CostPricingMode = mode
		return
	}
	if a.stats.CostPricingMode != mode {
		a.stats.CostPricingMode = "mixed"
	}
}

func runtimeMonitorItemAvailable(item modelgatewayobservability.RuntimeStatusItem) bool {
	return !item.CircuitOpen &&
		item.CircuitState != "open" &&
		!item.Cooldown &&
		!item.FailureAvoidance &&
		!item.ConfigErrorIsolated
}

func runtimeMonitorItemAtRisk(item modelgatewayobservability.RuntimeStatusItem) bool {
	if !runtimeMonitorItemAvailable(item) {
		return true
	}
	switch item.HealthStatus {
	case "healthy":
		return false
	default:
		return strings.TrimSpace(item.HealthStatus) != ""
	}
}

func runtimeMonitorConcurrencyLimit(item modelgatewayobservability.RuntimeStatusItem) int {
	if item.EffectiveConcurrencyLimit > 0 {
		return item.EffectiveConcurrencyLimit
	}
	if item.LearnedConcurrencyLimit > 0 {
		return item.LearnedConcurrencyLimit
	}
	if item.ConfiguredConcurrencyLimit > 0 {
		return item.ConfiguredConcurrencyLimit
	}
	return item.MaxConcurrency
}

func runtimeMonitorPressureRatio(item modelgatewayobservability.RuntimeStatusItem) float64 {
	limit := runtimeMonitorConcurrencyLimit(item)
	if limit <= 0 {
		return 0
	}
	return float64(item.ActiveConcurrency) / float64(limit)
}

func channelStatusRuntimeHealthStatus(stats ChannelStatusMonitorRuntimeStats) string {
	switch {
	case stats.RuntimeKeys <= 0:
		return ""
	case stats.CircuitOpenRuntimeKeys > 0:
		return "circuit_open"
	case stats.ConfigIsolatedRuntimeKeys > 0:
		return "config_isolated"
	case stats.CooldownRuntimeKeys > 0:
		return "cooldown"
	case stats.FailureAvoidanceRuntimeKeys > 0:
		return "failure_avoidance"
	case stats.HighPressureRuntimeKeys > 0:
		return "high_pressure"
	case stats.RiskRuntimeKeys > 0:
		return "degraded"
	default:
		return "healthy"
	}
}

func addPositiveAverage(sum *float64, count *int, value float64) {
	if sum == nil || count == nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	*sum += value
	*count++
}

func averageRuntimeMonitorValue(sum float64, count int) float64 {
	if count <= 0 || math.IsNaN(sum) || math.IsInf(sum, 0) {
		return 0
	}
	return math.Round((sum/float64(count))*10000) / 10000
}

func setMaxInt(values map[int]int, key int, value int) {
	if key <= 0 || value <= 0 {
		return
	}
	if value > values[key] {
		values[key] = value
	}
}

func setMaxFloat64(values map[int]float64, key int, value float64) {
	if key <= 0 || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return
	}
	if value > values[key] {
		values[key] = value
	}
}

func sumIntMap(values map[int]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func maxFloat64Map(values map[int]float64) float64 {
	maxValue := 0.0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return averageRuntimeMonitorValue(maxValue, 1)
}

func shouldIncludeChannelInStatusMonitor(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	return channel.Status == common.ChannelStatusEnabled || service.IsManagedPausedChannel(channel)
}

type channelMonitorLogStats struct {
	requests         int64
	successes        int64
	failures         int64
	healthProbes     int64
	error429         int64
	error5xx         int64
	errorTimeout     int64
	clientAborted    int64
	recovered        int64
	emptyOutputs     int64
	experienceIssues int64
	balanceErrors    int64
	errorRateLimit   int64
	streamErrors     int64
	latencySum       int64
	latencyCount     int64
	firstRespSum     int64
	firstRespCount   int64
	outputTokens     int64
	totalTokens      int64
	lastRequestAt    int64
	lastSuccessAt    int64
	lastFailureAt    int64
}

type channelMonitorLogStatsIndex struct {
	totalByChannel map[int]*channelMonitorLogStats
	byChannelGroup map[int]map[string]*channelMonitorLogStats
	byGroup        map[string]*channelMonitorLogStats
	overall        *channelMonitorLogStats
}

type channelMonitorRequestLog struct {
	id           int
	createdAt    int64
	logType      int
	group        string
	channelId    int
	requestId    string
	status       string
	statusWeight int
	reason       string
	successful   bool
	latencyMs    int64
}

type channelMonitorRequestAgg struct {
	group             string
	requestId         string
	lastId            int
	lastRequestAt     int64
	lastStatus        string
	worstStatus       string
	worstStatusWeight int
	worstReason       string
	success           bool
	latencyMs         int64
}

func buildChannelMonitorLogStats(rows []model.ChannelStatusMonitorLogRow) channelMonitorLogStatsIndex {
	index := channelMonitorLogStatsIndex{
		totalByChannel: make(map[int]*channelMonitorLogStats),
		byChannelGroup: make(map[int]map[string]*channelMonitorLogStats),
		byGroup:        make(map[string]*channelMonitorLogStats),
	}
	requestLogs := make([]channelMonitorRequestLog, 0, len(rows))
	for _, row := range rows {
		if row.ChannelId <= 0 {
			continue
		}
		applyChannelMonitorLogRow(index.totalByChannel, row.ChannelId, row)
		groupName := normalizeChannelGroup(row.Group)
		groupStats := index.byChannelGroup[row.ChannelId]
		if groupStats == nil {
			groupStats = make(map[string]*channelMonitorLogStats)
			index.byChannelGroup[row.ChannelId] = groupStats
		}
		applyChannelMonitorLogRow(groupStats, groupName, row)
		requestLogs = append(requestLogs, buildChannelMonitorRequestLog(row, groupName))
	}
	index.byGroup = buildChannelMonitorGroupRequestStats(requestLogs)
	index.overall = buildChannelMonitorOverallRequestStats(requestLogs)
	return index
}

func buildChannelMonitorRecentStatus(rows []model.ChannelStatusMonitorRecentLogRow, limit int) map[string][]string {
	if limit <= 0 {
		limit = channelStatusMonitorRecentLimit
	}
	result := make(map[string][]string)
	requestLogs := make([]channelMonitorRequestLog, 0, len(rows))
	for _, row := range rows {
		status := monitorLogStatus(row.Type, row.Other, row.Content)
		requestLogs = append(requestLogs, channelMonitorRequestLog{
			id:           row.Id,
			createdAt:    row.CreatedAt,
			logType:      row.Type,
			group:        normalizeChannelGroup(row.Group),
			channelId:    row.ChannelId,
			requestId:    strings.TrimSpace(row.RequestId),
			status:       status,
			statusWeight: monitorLogStatusWeight(status),
			reason:       row.Content,
			successful:   monitorLogIsSuccessful(row.Type, status),
		})
	}
	aggregates := buildChannelMonitorRequestAggregates(requestLogs)
	sort.Slice(aggregates, func(i, j int) bool {
		if aggregates[i].lastRequestAt == aggregates[j].lastRequestAt {
			return aggregates[i].lastId > aggregates[j].lastId
		}
		return aggregates[i].lastRequestAt > aggregates[j].lastRequestAt
	})
	for _, agg := range aggregates {
		if len(result[agg.group]) >= limit {
			continue
		}
		result[agg.group] = append(result[agg.group], channelMonitorRequestStatus(agg))
	}
	for groupName, statuses := range result {
		for left, right := 0, len(statuses)-1; left < right; left, right = left+1, right-1 {
			statuses[left], statuses[right] = statuses[right], statuses[left]
		}
		result[groupName] = statuses
	}
	return result
}

func channelMonitorUniqueUserRequestRows(rows []model.ModelGatewayUserRequestSummary) []model.ModelGatewayUserRequestSummary {
	if len(rows) == 0 {
		return rows
	}
	seen := make(map[string]struct{}, len(rows))
	unique := make([]model.ModelGatewayUserRequestSummary, 0, len(rows))
	for _, row := range rows {
		key := strings.TrimSpace(row.RequestId)
		if key == "" {
			key = "summary:" + strconv.Itoa(row.Id)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, row)
	}
	return unique
}

func buildChannelMonitorRecentUserRequestStatus(rows []model.ModelGatewayUserRequestSummary, limit int) map[string][]string {
	if limit <= 0 {
		limit = channelStatusMonitorRecentLimit
	}
	result := make(map[string][]string)
	grouped := make(map[string][]model.ModelGatewayUserRequestSummary)
	for _, row := range channelMonitorUniqueUserRequestRows(rows) {
		if row.IsHealthProbe {
			continue
		}
		groupName := channelMonitorUserRequestGroup(row)
		if groupName == "" {
			continue
		}
		grouped[groupName] = append(grouped[groupName], row)
	}
	for groupName, groupRows := range grouped {
		sort.Slice(groupRows, func(i, j int) bool {
			if groupRows[i].CompletedAt == groupRows[j].CompletedAt {
				return groupRows[i].Id > groupRows[j].Id
			}
			return groupRows[i].CompletedAt > groupRows[j].CompletedAt
		})
		if len(groupRows) > limit {
			groupRows = groupRows[:limit]
		}
		statuses := make([]string, 0, len(groupRows))
		for i := len(groupRows) - 1; i >= 0; i-- {
			statuses = append(statuses, channelMonitorUserRequestStatus(groupRows[i]))
		}
		result[groupName] = statuses
	}
	return result
}

func buildChannelMonitorUserRequestStats(rows []model.ModelGatewayUserRequestSummary) channelMonitorLogStatsIndex {
	index := channelMonitorLogStatsIndex{
		totalByChannel: make(map[int]*channelMonitorLogStats),
		byChannelGroup: make(map[int]map[string]*channelMonitorLogStats),
		byGroup:        make(map[string]*channelMonitorLogStats),
		overall:        &channelMonitorLogStats{},
	}
	for _, row := range channelMonitorUniqueUserRequestRows(rows) {
		groupName := channelMonitorUserRequestGroup(row)
		if groupName == "" {
			continue
		}
		applyChannelMonitorUserRequestRow(index.overall, row)
		groupStats := index.byGroup[groupName]
		if groupStats == nil {
			groupStats = &channelMonitorLogStats{}
			index.byGroup[groupName] = groupStats
		}
		applyChannelMonitorUserRequestRow(groupStats, row)
		if row.FinalChannelID <= 0 {
			continue
		}
		applyChannelMonitorUserRequestRowForChannel(index.totalByChannel, row.FinalChannelID, row)
		channelGroupStats := index.byChannelGroup[row.FinalChannelID]
		if channelGroupStats == nil {
			channelGroupStats = make(map[string]*channelMonitorLogStats)
			index.byChannelGroup[row.FinalChannelID] = channelGroupStats
		}
		applyChannelMonitorUserRequestRowForChannel(channelGroupStats, groupName, row)
	}
	return index
}

func applyChannelMonitorUserRequestRowForChannel[K comparable](stats map[K]*channelMonitorLogStats, key K, row model.ModelGatewayUserRequestSummary) {
	agg := stats[key]
	if agg == nil {
		agg = &channelMonitorLogStats{}
		stats[key] = agg
	}
	applyChannelMonitorUserRequestRow(agg, row)
}

func applyChannelMonitorUserRequestRow(stats *channelMonitorLogStats, row model.ModelGatewayUserRequestSummary) {
	if stats == nil {
		return
	}
	status := channelMonitorUserRequestStatus(row)
	stats.requests++
	if row.IsHealthProbe {
		stats.healthProbes++
	}
	stats.lastRequestAt = maxInt64(stats.lastRequestAt, row.CompletedAt)
	if row.Recovered && !row.IsHealthProbe {
		stats.recovered++
	}
	if row.EmptyOutput && !row.IsHealthProbe {
		stats.emptyOutputs++
	}
	if strings.TrimSpace(row.ExperienceIssue) != "" && !row.EmptyOutput && !row.IsHealthProbe {
		stats.experienceIssues++
	}
	if row.TTFTMs > 0 {
		stats.firstRespSum += row.TTFTMs
		stats.firstRespCount++
	}
	if row.DurationMs > 0 {
		stats.latencySum += row.DurationMs
		stats.latencyCount++
	}
	if status == "client_aborted" {
		stats.clientAborted++
		return
	}
	if row.FinalSuccess {
		if !row.IsHealthProbe {
			stats.successes++
			stats.lastSuccessAt = maxInt64(stats.lastSuccessAt, row.CompletedAt)
		}
		return
	}
	if row.IsHealthProbe {
		return
	}
	stats.failures++
	stats.lastFailureAt = maxInt64(stats.lastFailureAt, row.CompletedAt)
	applyMonitorStatusToStats(stats, status)
	if status == "stream_interrupted" {
		stats.streamErrors++
	}
	if row.FinalErrorCategory == model.ModelGatewayUserRequestErrorBalanceOrQuota ||
		row.FinalErrorCategory == model.ModelGatewayUserRequestErrorAuthConfig {
		stats.balanceErrors++
	}
}

func channelMonitorUserRequestGroup(row model.ModelGatewayUserRequestSummary) string {
	groupName := strings.TrimSpace(row.SelectedGroup)
	if groupName == "" {
		groupName = strings.TrimSpace(row.RequestedGroup)
	}
	return normalizeChannelGroup(groupName)
}

func channelMonitorUserRequestStatus(row model.ModelGatewayUserRequestSummary) string {
	if row.ClientAborted || strings.TrimSpace(row.FinalErrorCategory) == model.ModelGatewayUserRequestErrorClientAborted || row.FinalStatusCode == relayStatusClientClosedRequest {
		return "client_aborted"
	}
	if row.IsHealthProbe {
		if row.FinalSuccess {
			return "health_probe"
		}
		return "health_probe_failed"
	}
	if row.FinalSuccess {
		if row.EmptyOutput {
			return "empty_output"
		}
		if strings.TrimSpace(row.ExperienceIssue) != "" {
			return "experience_issue"
		}
		return "success"
	}
	switch strings.TrimSpace(row.FinalErrorCategory) {
	case model.ModelGatewayUserRequestErrorRateLimit:
		return "rate_limit"
	case model.ModelGatewayUserRequestErrorTimeout:
		return "timeout"
	case model.ModelGatewayUserRequestErrorServer, model.ModelGatewayUserRequestErrorUpstream:
		return "server_error"
	case model.ModelGatewayUserRequestErrorStreamInterrupted:
		return "stream_interrupted"
	case model.ModelGatewayUserRequestErrorBalanceOrQuota, model.ModelGatewayUserRequestErrorAuthConfig:
		return "error"
	default:
		if row.FinalStatusCode == http.StatusTooManyRequests {
			return "rate_limit"
		}
		if isTimeoutStatus(row.FinalStatusCode) {
			return "timeout"
		}
		if row.FinalStatusCode >= http.StatusInternalServerError {
			return "server_error"
		}
		return "error"
	}
}

func buildChannelMonitorRequestLog(row model.ChannelStatusMonitorLogRow, groupName string) channelMonitorRequestLog {
	other, _ := common.StrToMap(row.Other)
	_, reason := parseMonitorStatusCodeAndReason(other, row.Content)
	status := monitorLogStatus(row.Type, row.Other, row.Content)
	latencyMs := int64(row.UseTime) * 1000
	if latency, ok := parseMonitorInt64(other, "frt"); ok && latency > 0 && (latencyMs <= 0 || latency < latencyMs) {
		latencyMs = latency
	}
	return channelMonitorRequestLog{
		id:           row.Id,
		createdAt:    row.CreatedAt,
		logType:      row.Type,
		group:        groupName,
		channelId:    row.ChannelId,
		requestId:    strings.TrimSpace(row.RequestId),
		status:       status,
		statusWeight: monitorLogStatusWeight(status),
		reason:       reason,
		successful:   monitorLogIsSuccessful(row.Type, status),
		latencyMs:    latencyMs,
	}
}

func buildChannelMonitorGroupRequestStats(logs []channelMonitorRequestLog) map[string]*channelMonitorLogStats {
	result := make(map[string]*channelMonitorLogStats)
	for _, agg := range buildChannelMonitorRequestAggregates(logs) {
		stats := result[agg.group]
		if stats == nil {
			stats = &channelMonitorLogStats{}
			result[agg.group] = stats
		}
		stats.requests++
		stats.lastRequestAt = maxInt64(stats.lastRequestAt, agg.lastRequestAt)
		if agg.success {
			stats.successes++
			stats.lastSuccessAt = maxInt64(stats.lastSuccessAt, agg.lastRequestAt)
		} else {
			stats.failures++
			stats.lastFailureAt = maxInt64(stats.lastFailureAt, agg.lastRequestAt)
			applyMonitorStatusToStats(stats, channelMonitorRequestStatus(agg))
			applyMonitorReasonToStats(stats, agg.worstReason)
		}
		if agg.latencyMs > 0 {
			stats.latencySum += agg.latencyMs
			stats.latencyCount++
		}
	}
	return result
}

func buildChannelMonitorOverallRequestStats(logs []channelMonitorRequestLog) *channelMonitorLogStats {
	stats := &channelMonitorLogStats{}
	for _, agg := range buildChannelMonitorRequestAggregatesWithKey(logs, func(log channelMonitorRequestLog) string {
		if log.requestId != "" {
			return log.requestId
		}
		return fallbackChannelMonitorRequestId(log)
	}) {
		stats.requests++
		stats.lastRequestAt = maxInt64(stats.lastRequestAt, agg.lastRequestAt)
		if agg.success {
			stats.successes++
			stats.lastSuccessAt = maxInt64(stats.lastSuccessAt, agg.lastRequestAt)
		} else {
			stats.failures++
			stats.lastFailureAt = maxInt64(stats.lastFailureAt, agg.lastRequestAt)
			applyMonitorStatusToStats(stats, channelMonitorRequestStatus(agg))
			applyMonitorReasonToStats(stats, agg.worstReason)
		}
		if agg.latencyMs > 0 {
			stats.latencySum += agg.latencyMs
			stats.latencyCount++
		}
	}
	return stats
}

func buildChannelMonitorRequestAggregates(logs []channelMonitorRequestLog) []*channelMonitorRequestAgg {
	return buildChannelMonitorRequestAggregatesWithKey(logs, func(log channelMonitorRequestLog) string {
		groupName := normalizeChannelGroup(log.group)
		requestId := log.requestId
		if requestId == "" {
			requestId = fallbackChannelMonitorRequestId(log)
		}
		return groupName + "\x00" + requestId
	})
}

func buildChannelMonitorRequestAggregatesWithKey(logs []channelMonitorRequestLog, keyFunc func(channelMonitorRequestLog) string) []*channelMonitorRequestAgg {
	aggregates := make(map[string]*channelMonitorRequestAgg)
	for _, log := range logs {
		groupName := normalizeChannelGroup(log.group)
		key := keyFunc(log)
		agg := aggregates[key]
		if agg == nil {
			agg = &channelMonitorRequestAgg{
				group:     groupName,
				requestId: log.requestId,
			}
			aggregates[key] = agg
		}
		if log.createdAt > agg.lastRequestAt || (log.createdAt == agg.lastRequestAt && log.id > agg.lastId) {
			agg.lastRequestAt = log.createdAt
			agg.lastId = log.id
			agg.lastStatus = log.status
		}
		if log.statusWeight > agg.worstStatusWeight {
			agg.worstStatusWeight = log.statusWeight
			agg.worstStatus = log.status
			agg.worstReason = log.reason
		} else if log.statusWeight == agg.worstStatusWeight && agg.worstReason == "" {
			agg.worstReason = log.reason
		}
		if log.successful {
			agg.success = true
			if log.latencyMs > 0 && (agg.latencyMs == 0 || log.createdAt > agg.lastRequestAt || log.id >= agg.lastId) {
				agg.latencyMs = log.latencyMs
			}
		}
	}
	result := make([]*channelMonitorRequestAgg, 0, len(aggregates))
	for _, agg := range aggregates {
		result = append(result, agg)
	}
	return result
}

func fallbackChannelMonitorRequestId(log channelMonitorRequestLog) string {
	if log.id > 0 {
		return "log:" + strconv.Itoa(log.id)
	}
	return "log:" + strconv.FormatInt(log.createdAt, 10) + ":" + strconv.Itoa(log.channelId)
}

func channelMonitorRequestStatus(agg *channelMonitorRequestAgg) string {
	if agg == nil {
		return "error"
	}
	if agg.success {
		return "success"
	}
	if agg.worstStatus != "" {
		return agg.worstStatus
	}
	if agg.lastStatus != "" {
		return agg.lastStatus
	}
	return "error"
}

func monitorLogStatusWeight(status string) int {
	switch status {
	case "success":
		return 0
	case "error":
		return 1
	case "empty_output", "experience_issue", "client_aborted":
		return 2
	case "timeout":
		return 3
	case "stream_interrupted":
		return 4
	case "server_error":
		return 5
	case "rate_limit":
		return 6
	default:
		return 1
	}
}

func applyMonitorStatusToStats(stats *channelMonitorLogStats, status string) {
	if stats == nil {
		return
	}
	switch status {
	case "rate_limit":
		stats.error429++
		stats.errorRateLimit++
	case "server_error":
		stats.error5xx++
	case "timeout":
		stats.errorTimeout++
	case "stream_interrupted":
		stats.streamErrors++
	}
}

func applyMonitorReasonToStats(stats *channelMonitorLogStats, reason string) {
	if stats == nil {
		return
	}
	if service.IsBalanceInsufficientMessage(reason) {
		stats.balanceErrors++
	}
}

func monitorLogStatus(logType int, otherRaw string, content string) string {
	other, _ := common.StrToMap(otherRaw)
	statusCode, _ := parseMonitorStatusCodeAndReason(other, content)
	streamStatus, hasStreamStatus := monitorLogStreamStatus(other)
	if logType == model.LogTypeConsume {
		if statusCode == http.StatusTooManyRequests {
			return "rate_limit"
		}
		if statusCode >= 500 && statusCode <= 599 {
			return "server_error"
		}
		if isTimeoutStatus(statusCode) {
			return "timeout"
		}
		if hasStreamStatus && streamStatus != "success" {
			return streamStatus
		}
		return "success"
	}
	switch {
	case statusCode == http.StatusTooManyRequests:
		return "rate_limit"
	case statusCode >= 500 && statusCode <= 599:
		return "server_error"
	case isTimeoutStatus(statusCode) || strings.Contains(strings.ToLower(content), "timeout") || strings.Contains(content, "超时"):
		return "timeout"
	default:
		return "error"
	}
}

func monitorLogIsSuccessful(logType int, status string) bool {
	return logType == model.LogTypeConsume && status == "success"
}

func monitorLogStreamStatus(other map[string]interface{}) (string, bool) {
	if other == nil {
		return "", false
	}
	raw, ok := other["stream_status"]
	if !ok {
		return "", false
	}
	stream, ok := raw.(map[string]interface{})
	if !ok {
		return "", false
	}
	status, _ := stream["status"].(string)
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" || status == "ok" || status == "client_gone" {
		return "success", true
	}
	endReason, _ := stream["end_reason"].(string)
	switch strings.ToLower(strings.TrimSpace(endReason)) {
	case "timeout", "ping_fail":
		return "timeout", true
	default:
		return "error", true
	}
}

func applyChannelMonitorLogRow[K comparable](stats map[K]*channelMonitorLogStats, key K, row model.ChannelStatusMonitorLogRow) {
	agg := stats[key]
	if agg == nil {
		agg = &channelMonitorLogStats{}
		stats[key] = agg
	}
	if row.CreatedAt > agg.lastRequestAt {
		agg.lastRequestAt = row.CreatedAt
	}
	other, _ := common.StrToMap(row.Other)
	switch row.Type {
	case model.LogTypeConsume:
		agg.requests++
		status := monitorLogStatus(row.Type, row.Other, row.Content)
		if monitorLogIsSuccessful(row.Type, status) {
			agg.successes++
			agg.lastSuccessAt = maxInt64(agg.lastSuccessAt, row.CreatedAt)
		} else {
			agg.failures++
			agg.lastFailureAt = maxInt64(agg.lastFailureAt, row.CreatedAt)
			applyMonitorStatusToStats(agg, status)
			_, reason := parseMonitorStatusCodeAndReason(other, row.Content)
			applyMonitorReasonToStats(agg, reason)
		}
		latencyMs := int64(row.UseTime) * 1000
		if latency, ok := parseMonitorInt64(other, "frt"); ok && latency > 0 {
			agg.firstRespSum += latency
			agg.firstRespCount++
			if latencyMs <= 0 {
				latencyMs = latency
			}
		}
		if latencyMs > 0 {
			agg.latencySum += latencyMs
			agg.latencyCount++
		}
		agg.outputTokens += int64(row.CompletionTokens)
		agg.totalTokens += int64(row.PromptTokens + row.CompletionTokens)
		if streamStatus, ok := monitorLogStreamStatus(other); ok && streamStatus != "success" {
			agg.streamErrors++
		}
	case model.LogTypeError:
		agg.requests++
		agg.failures++
		agg.lastFailureAt = maxInt64(agg.lastFailureAt, row.CreatedAt)
		statusCode, reason := parseMonitorStatusCodeAndReason(other, row.Content)
		applyMonitorReasonToStats(agg, reason)
		switch {
		case statusCode == http.StatusTooManyRequests:
			agg.error429++
			normalizedReason := strings.ToLower(reason)
			if strings.Contains(normalizedReason, "rate limit") ||
				strings.Contains(normalizedReason, "too many") ||
				strings.Contains(normalizedReason, "concurrency") ||
				strings.Contains(normalizedReason, "限速") ||
				strings.Contains(normalizedReason, "并发") {
				agg.errorRateLimit++
			}
		case statusCode >= 500 && statusCode <= 599:
			agg.error5xx++
		case isTimeoutStatus(statusCode) || strings.Contains(strings.ToLower(reason), "timeout") || strings.Contains(reason, "超时"):
			agg.errorTimeout++
		}
	}
}

func buildChannelStatusMonitorItem(channel *model.Channel, logStat *channelMonitorLogStats) *ChannelStatusMonitorItem {
	item := &ChannelStatusMonitorItem{
		ID:                 channel.Id,
		Name:               channel.Name,
		Type:               channel.Type,
		Status:             channel.Status,
		Group:              normalizeChannelGroup(channel.Group),
		Models:             channel.Models,
		Priority:           channel.GetPriority(),
		ResponseTime:       channel.ResponseTime,
		TestTime:           channel.TestTime,
		CreatedTime:        channel.CreatedTime,
		Enabled:            channel.Status == common.ChannelStatusEnabled,
		ActiveConcurrency:  service.GetChannelActiveConcurrency(channel.Id),
		MaxConcurrency:     channel.GetSetting().MaxConcurrency,
		ConcurrencyCeiling: maxConcurrencyCeiling(channel.Setting),
	}

	if cooldown := service.GetChannelConcurrencyCooldownStatus(channel.Id); cooldown != nil {
		item.ConcurrencyCooldown = cooldown.RemainingSec
		item.ConcurrencyReason = cooldown.Reason
	}
	if avoidance := service.GetChannelFailureAvoidanceStatus(channel.Id); avoidance != nil && !service.IsManagedPausedChannel(channel) {
		item.FailureAvoidance = avoidance.RemainingSec
		item.FailureReason = avoidance.Reason
	}
	applyChannelPauseStatus(item, channel)
	if logStat != nil {
		item.RecentRequests = logStat.requests
		item.RecentSuccesses = logStat.successes
		item.RecentFailures = logStat.failures
		item.RecentHealthProbes = logStat.healthProbes
		item.RecentError429 = logStat.error429
		item.RecentError5xx = logStat.error5xx
		item.RecentErrorTimeout = logStat.errorTimeout
		item.RecentClientAborted = logStat.clientAborted
		item.RecentRecovered = logStat.recovered
		item.RecentEmptyOutputs = logStat.emptyOutputs
		item.RecentExperienceIssues = logStat.experienceIssues
		item.RecentBalanceErrors = logStat.balanceErrors
		item.RecentErrorRateLimit = logStat.errorRateLimit
		item.RecentStreamErrors = logStat.streamErrors
		item.RecentAvgLatencyMs = avgInt64(logStat.latencySum, logStat.latencyCount)
		item.RecentAvgFirstRespMs = avgInt64(logStat.firstRespSum, logStat.firstRespCount)
		item.RecentAvgOutputTokens = avgInt64(logStat.outputTokens, logStat.successes)
		item.RecentTotalTokens = logStat.totalTokens
		item.SuccessRate = successRate(logStat.successes, logStat.requests)
		item.LastRequestAt = logStat.lastRequestAt
		item.LastSuccessAt = logStat.lastSuccessAt
		item.LastFailureAt = logStat.lastFailureAt
		item.HealthScore = channelHealthScore(channel, logStat)
		item.HealthState = channelHealthState(channel, logStat)
		return item
	}

	item.HealthScore = channelHealthScore(channel, nil)
	item.HealthState = channelHealthState(channel, nil)
	return item
}

func applyChannelPauseStatus(item *ChannelStatusMonitorItem, channel *model.Channel) {
	if item == nil || channel == nil || channel.Status != common.ChannelStatusAutoDisabled {
		return
	}
	info := channel.GetOtherInfo()
	reason, _ := info["status_reason"].(string)
	if service.IsBalanceInsufficientStatusReason(reason) {
		item.PauseType = service.ChannelStatusReasonBalanceInsufficient
		item.PauseReason = reason
		return
	}
	if service.IsErrorPausedStatusReason(reason) {
		item.PauseType = service.ChannelStatusReasonErrorPaused
		if pauseReason, _ := info["pause_reason"].(string); strings.TrimSpace(pauseReason) != "" {
			item.PauseReason = pauseReason
		} else {
			item.PauseReason = reason
		}
		if until, ok := parseMonitorInt64(info, "pause_until"); ok && until > 0 {
			item.PauseUntil = until
			if remaining := until - time.Now().Unix(); remaining > 0 {
				item.PauseRemaining = remaining
			}
		}
	}
}

func channelMonitorGroups(channel *model.Channel) []string {
	if channel == nil {
		return []string{"default"}
	}
	groups := channel.GetGroups()
	seen := map[string]struct{}{}
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		normalized := normalizeChannelGroup(group)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return []string{"default"}
	}
	return result
}

func normalizeChannelGroup(group string) string {
	group = strings.TrimSpace(group)
	if group == "" {
		return "default"
	}
	return group
}

func avgInt64(sum, count int64) int64 {
	if count <= 0 {
		return 0
	}
	return sum / count
}

func channelMonitorPreferredStats(primary, fallback *channelMonitorLogStats) *channelMonitorLogStats {
	if primary != nil && primary.requests > 0 {
		return primary
	}
	return fallback
}

func channelMonitorUserSuccessRate(successes, requests, clientAborted, healthProbes int64) float64 {
	return successRate(successes, requests-clientAborted-healthProbes)
}

func successRate(successes, requests int64) float64 {
	if requests <= 0 {
		return 0
	}
	return float64(successes) / float64(requests) * 100
}

func parseMonitorInt64(other map[string]interface{}, key string) (int64, bool) {
	if other == nil {
		return 0, false
	}
	v, ok := other[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	case string:
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil {
			return int64(parsed), true
		}
	}
	return 0, false
}

func parseMonitorStatusCode(other map[string]interface{}) (int, bool) {
	if other == nil {
		return 0, false
	}
	v, ok := other["status_code"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func isTimeoutStatus(statusCode int) bool {
	return statusCode == http.StatusRequestTimeout ||
		statusCode == http.StatusGatewayTimeout ||
		statusCode == 524 ||
		statusCode == 529
}

func parseMonitorStatusCodeAndReason(other map[string]interface{}, content string) (int, string) {
	if statusCode, ok := parseMonitorStatusCode(other); ok {
		return statusCode, content
	}
	raw := strings.TrimSpace(content)
	if strings.HasPrefix(strings.ToLower(raw), "status_code=") {
		parts := strings.SplitN(raw, ",", 2)
		if len(parts) == 2 {
			if parsed, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(parts[0], "status_code="))); err == nil {
				return parsed, strings.TrimSpace(parts[1])
			}
		}
	}
	return 0, content
}

func maxInt64(a, b int64) int64 {
	if a >= b {
		return a
	}
	return b
}

func maxConcurrencyCeiling(setting *string) int {
	if setting == nil || strings.TrimSpace(*setting) == "" {
		return 0
	}
	settings := map[string]interface{}{}
	if err := common.Unmarshal([]byte(*setting), &settings); err != nil {
		return 0
	}
	value, ok := settings["max_concurrency_ceiling"]
	if !ok {
		return 0
	}
	switch n := value.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func channelHealthScore(channel *model.Channel, stats *channelMonitorLogStats) int {
	if channel == nil {
		return 0
	}
	if service.IsBalanceInsufficientPausedChannel(channel) {
		return 100
	}
	score := 100
	if channel.Status != common.ChannelStatusEnabled {
		score -= 35
	}
	if stats == nil || stats.requests == 0 {
		if channel.Status == common.ChannelStatusEnabled && channel.ResponseTime <= 0 {
			score -= 10
		}
		if channel.ResponseTime > 4000 {
			score -= 8
		} else if channel.ResponseTime > 2000 {
			score -= 4
		}
		return clampScore(score)
	}
	healthRequests := stats.requests - stats.balanceErrors
	healthFailures := stats.failures - stats.balanceErrors
	if healthRequests <= 0 || healthFailures < 0 {
		healthFailures = 0
	}
	if healthRequests <= 0 {
		if channel.ResponseTime > 4000 {
			score -= 8
		} else if channel.ResponseTime > 2000 {
			score -= 4
		}
		return clampScore(score)
	}
	healthSuccesses := healthRequests - healthFailures
	if healthSuccesses < 0 {
		healthSuccesses = 0
	}
	rate := successRate(healthSuccesses, healthRequests)
	switch {
	case rate < 60:
		score -= 40
	case rate < 85:
		score -= 20
	case rate < 95:
		score -= 8
	}
	nonBalance429 := stats.error429 - stats.balanceErrors
	if nonBalance429 < 0 {
		nonBalance429 = 0
	}
	score -= minInt(nonBalance429*4, 16)
	score -= minInt(stats.error5xx*5, 20)
	score -= minInt(stats.errorTimeout*5, 20)
	score -= minInt(stats.streamErrors*3, 12)
	if service.GetChannelConcurrencyCooldownStatus(channel.Id) != nil {
		score -= 12
	}
	if service.GetChannelFailureAvoidanceStatus(channel.Id) != nil {
		score -= 12
	}
	if channel.ResponseTime > 4000 {
		score -= 8
	} else if channel.ResponseTime > 2000 {
		score -= 4
	}
	return clampScore(score)
}

func channelHealthState(channel *model.Channel, stats *channelMonitorLogStats) string {
	score := channelHealthScore(channel, stats)
	switch {
	case score >= 85:
		return "healthy"
	case score >= 65:
		return "warning"
	default:
		return "critical"
	}
}

func clampScore(v int) int {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	default:
		return v
	}
}

func minInt(a int64, b int) int {
	if int(a) < b {
		return int(a)
	}
	return b
}
