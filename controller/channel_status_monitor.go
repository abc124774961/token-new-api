package controller

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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
	ID                    int     `json:"id"`
	Name                  string  `json:"name"`
	Type                  int     `json:"type"`
	Status                int     `json:"status"`
	Group                 string  `json:"group"`
	Models                string  `json:"models"`
	Priority              int64   `json:"priority"`
	ResponseTime          int     `json:"response_time"`
	TestTime              int64   `json:"test_time"`
	CreatedTime           int64   `json:"created_time"`
	Enabled               bool    `json:"enabled"`
	ActiveConcurrency     int     `json:"active_concurrency"`
	MaxConcurrency        int     `json:"max_concurrency"`
	ConcurrencyCeiling    int     `json:"concurrency_ceiling"`
	ConcurrencyCooldown   int64   `json:"concurrency_cooldown_remaining_seconds,omitempty"`
	FailureAvoidance      int64   `json:"failure_avoidance_remaining_seconds,omitempty"`
	FailureReason         string  `json:"failure_reason,omitempty"`
	LastRequestAt         int64   `json:"last_request_at,omitempty"`
	LastSuccessAt         int64   `json:"last_success_at,omitempty"`
	LastFailureAt         int64   `json:"last_failure_at,omitempty"`
	RecentRequests        int64   `json:"recent_requests"`
	RecentSuccesses       int64   `json:"recent_successes"`
	RecentFailures        int64   `json:"recent_failures"`
	RecentError429        int64   `json:"recent_error_429"`
	RecentError5xx        int64   `json:"recent_error_5xx"`
	RecentErrorTimeout    int64   `json:"recent_error_timeout"`
	RecentErrorRateLimit  int64   `json:"recent_error_rate_limit"`
	RecentStreamErrors    int64   `json:"recent_stream_errors"`
	RecentAvgLatencyMs    int64   `json:"recent_avg_latency_ms"`
	RecentAvgFirstRespMs  int64   `json:"recent_avg_first_response_ms"`
	RecentAvgOutputTokens int64   `json:"recent_avg_output_tokens"`
	RecentTotalTokens     int64   `json:"recent_total_tokens"`
	SuccessRate           float64 `json:"success_rate"`
	HealthScore           int     `json:"health_score"`
	HealthState           string  `json:"health_state"`
}

type ChannelStatusMonitorGroup struct {
	Group              string                      `json:"group"`
	TotalChannels      int                         `json:"total_channels"`
	EnabledChannels    int                         `json:"enabled_channels"`
	DisabledChannels   int                         `json:"disabled_channels"`
	BusyChannels       int                         `json:"busy_channels"`
	CooldownChannels   int                         `json:"cooldown_channels"`
	BadChannels        int                         `json:"bad_channels"`
	HealthyChannels    int                         `json:"healthy_channels"`
	RecentRequests     int64                       `json:"recent_requests"`
	RecentSuccesses    int64                       `json:"recent_successes"`
	RecentFailures     int64                       `json:"recent_failures"`
	RecentError429     int64                       `json:"recent_error_429"`
	RecentError5xx     int64                       `json:"recent_error_5xx"`
	RecentErrorTimeout int64                       `json:"recent_error_timeout"`
	SuccessRate        float64                     `json:"success_rate"`
	AvgLatencyMs       int64                       `json:"avg_latency_ms"`
	RecentStatus       []string                    `json:"recent_status"`
	Channels           []*ChannelStatusMonitorItem `json:"channels"`
}

type ChannelStatusMonitorSummary struct {
	WindowHours        int     `json:"window_hours"`
	TotalGroups        int     `json:"total_groups"`
	TotalChannels      int     `json:"total_channels"`
	EnabledChannels    int     `json:"enabled_channels"`
	DisabledChannels   int     `json:"disabled_channels"`
	BusyChannels       int     `json:"busy_channels"`
	CooldownChannels   int     `json:"cooldown_channels"`
	BadChannels        int     `json:"bad_channels"`
	HealthyChannels    int     `json:"healthy_channels"`
	RecentRequests     int64   `json:"recent_requests"`
	RecentSuccesses    int64   `json:"recent_successes"`
	RecentFailures     int64   `json:"recent_failures"`
	RecentError429     int64   `json:"recent_error_429"`
	RecentError5xx     int64   `json:"recent_error_5xx"`
	RecentErrorTimeout int64   `json:"recent_error_timeout"`
	SuccessRate        float64 `json:"success_rate"`
	AvgLatencyMs       int64   `json:"avg_latency_ms"`
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
	enabledChannelIds := make([]int, 0, len(channels))
	for _, channel := range channels {
		if channel != nil && channel.Status == common.ChannelStatusEnabled {
			enabledChannelIds = append(enabledChannelIds, channel.Id)
		}
	}

	startTs := time.Now().Add(-time.Duration(windowHours) * time.Hour).Unix()
	logRows, err := model.GetChannelStatusMonitorLogs(startTs, enabledChannelIds, logLimit)
	if err != nil {
		return ChannelStatusMonitorResponse{}, err
	}
	recentLogRows, err := model.GetChannelStatusMonitorRecentLogs(enabledChannelIds, channelStatusMonitorRecentLimit*4)
	if err != nil {
		return ChannelStatusMonitorResponse{}, err
	}
	return buildChannelStatusMonitorFromRowsWithChannels(windowHours, channels, logRows, recentLogRows), nil
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
	logStats := buildChannelMonitorLogStats(logRows)
	recentStatusByGroup := buildChannelMonitorRecentStatus(recentLogRows, channelStatusMonitorRecentLimit)
	groupMap := map[string]*ChannelStatusMonitorGroup{}
	summary := ChannelStatusMonitorSummary{WindowHours: windowHours}
	for _, channel := range channels {
		if channel == nil || channel.Status != common.ChannelStatusEnabled {
			continue
		}
		summary.TotalChannels++

		item := buildChannelStatusMonitorItem(channel, logStats.totalByChannel[channel.Id])
		if item.Enabled {
			summary.EnabledChannels++
		} else {
			summary.DisabledChannels++
		}
		if item.ActiveConcurrency > 0 {
			summary.BusyChannels++
		}
		if item.ConcurrencyCooldown > 0 || item.FailureAvoidance > 0 {
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
				group = &ChannelStatusMonitorGroup{Group: groupName}
				groupMap[groupName] = group
			}
			groupItem := buildChannelStatusMonitorItem(channel, logStats.byChannelGroup[channel.Id][groupName])
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
			if groupItem.ConcurrencyCooldown > 0 || groupItem.FailureAvoidance > 0 {
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
		if groupStat := logStats.byGroup[group.Group]; groupStat != nil {
			group.RecentRequests = groupStat.requests
			group.RecentSuccesses = groupStat.successes
			group.RecentFailures = groupStat.failures
			group.RecentError429 = groupStat.error429
			group.RecentError5xx = groupStat.error5xx
			group.RecentErrorTimeout = groupStat.errorTimeout
			group.AvgLatencyMs = avgInt64(groupStat.latencySum, groupStat.latencyCount)
		}
		if group.RecentRequests > 0 {
			group.SuccessRate = float64(group.RecentSuccesses) / float64(group.RecentRequests) * 100
		}
		if len(group.Channels) > 0 {
			sort.Slice(group.Channels, func(i, j int) bool {
				if group.Channels[i].HealthScore == group.Channels[j].HealthScore {
					return group.Channels[i].RecentRequests > group.Channels[j].RecentRequests
				}
				return group.Channels[i].HealthScore < group.Channels[j].HealthScore
			})
		}
		group.RecentStatus = recentStatusByGroup[group.Group]
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].RecentRequests == groups[j].RecentRequests {
			return groups[i].Group < groups[j].Group
		}
		return groups[i].RecentRequests > groups[j].RecentRequests
	})
	summary.TotalGroups = len(groups)
	if overallStat := logStats.overall; overallStat != nil {
		summary.RecentRequests = overallStat.requests
		summary.RecentSuccesses = overallStat.successes
		summary.RecentFailures = overallStat.failures
		summary.RecentError429 = overallStat.error429
		summary.RecentError5xx = overallStat.error5xx
		summary.RecentErrorTimeout = overallStat.errorTimeout
		summary.AvgLatencyMs = avgInt64(overallStat.latencySum, overallStat.latencyCount)
	}
	if summary.RecentRequests > 0 {
		summary.SuccessRate = float64(summary.RecentSuccesses) / float64(summary.RecentRequests) * 100
	}

	return ChannelStatusMonitorResponse{
		Summary: summary,
		Groups:  groups,
	}
}

type channelMonitorLogStats struct {
	requests       int64
	successes      int64
	failures       int64
	error429       int64
	error5xx       int64
	errorTimeout   int64
	errorRateLimit int64
	streamErrors   int64
	latencySum     int64
	latencyCount   int64
	firstRespSum   int64
	firstRespCount int64
	outputTokens   int64
	totalTokens    int64
	lastRequestAt  int64
	lastSuccessAt  int64
	lastFailureAt  int64
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

func buildChannelMonitorRequestLog(row model.ChannelStatusMonitorLogRow, groupName string) channelMonitorRequestLog {
	other, _ := common.StrToMap(row.Other)
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
	case "timeout":
		return 2
	case "server_error":
		return 3
	case "rate_limit":
		return 4
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
	}
	if avoidance := service.GetChannelFailureAvoidanceStatus(channel.Id); avoidance != nil {
		item.FailureAvoidance = avoidance.RemainingSec
		item.FailureReason = avoidance.Reason
	}
	if logStat != nil {
		item.RecentRequests = logStat.requests
		item.RecentSuccesses = logStat.successes
		item.RecentFailures = logStat.failures
		item.RecentError429 = logStat.error429
		item.RecentError5xx = logStat.error5xx
		item.RecentErrorTimeout = logStat.errorTimeout
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
	score := 100
	if channel.Status != common.ChannelStatusEnabled {
		score -= 35
	}
	if stats == nil || stats.requests == 0 {
		if channel.Status == common.ChannelStatusEnabled {
			score -= 10
		}
		return clampScore(score)
	}
	rate := successRate(stats.successes, stats.requests)
	switch {
	case rate < 60:
		score -= 40
	case rate < 85:
		score -= 20
	case rate < 95:
		score -= 8
	}
	score -= minInt(stats.error429*4, 16)
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
