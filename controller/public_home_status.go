package controller

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	publicHomeStatusDefaultDays = 30
	publicHomeStatusMaxDays     = 30
	publicHomeStatusCacheTTL    = 2 * time.Minute
	publicHomeStatusErrorTTL    = 30 * time.Second
)

var publicHomeStatusCache = struct {
	sync.Mutex
	items    map[int]publicHomeStatusCacheItem
	inFlight map[int]chan struct{}
}{
	items:    make(map[int]publicHomeStatusCacheItem),
	inFlight: make(map[int]chan struct{}),
}

type publicHomeStatusCacheItem struct {
	result    PublicHomeStatusResponse
	expiresAt time.Time
}

type PublicHomeStatusSummary struct {
	Days            int     `json:"days"`
	SuccessRate     float64 `json:"success_rate"`
	AvgLatencyMs    int64   `json:"avg_latency_ms"`
	Requests        int64   `json:"requests"`
	EnabledChannels int     `json:"enabled_channels"`
	HealthyChannels int     `json:"healthy_channels"`
	ProtectedEvents int64   `json:"protected_events"`
}

type PublicHomeStatusDaily struct {
	Date            string  `json:"date"`
	Requests        int64   `json:"requests"`
	SuccessRate     float64 `json:"success_rate"`
	AvgLatencyMs    int64   `json:"avg_latency_ms"`
	ProtectedEvents int64   `json:"protected_events"`
}

type PublicHomeStatusResponse struct {
	Summary   PublicHomeStatusSummary `json:"summary"`
	Daily     []PublicHomeStatusDaily `json:"daily"`
	Groups    []PublicHomeStatusGroup `json:"groups"`
	UpdatedAt int64                   `json:"updated_at"`
	Partial   bool                    `json:"partial,omitempty"`
}

type PublicHomeStatusGroup struct {
	Key     string                    `json:"key"`
	Name    string                    `json:"name"`
	Summary PublicHomeStatusSummary   `json:"summary"`
	Daily   []PublicHomeStatusDaily   `json:"daily"`
	States  PublicHomeStatusGroupMeta `json:"states"`
}

type PublicHomeStatusGroupMeta struct {
	Healthy  int `json:"healthy"`
	Cooling  int `json:"cooling"`
	Standby  int `json:"standby"`
	Channels int `json:"channels"`
}

func GetPublicHomeStatus(c *gin.Context) {
	days := publicHomeStatusDefaultDays
	if raw := c.Query("days"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			days = parsed
		}
	}

	result, err := buildPublicHomeStatusCached(normalizePublicHomeStatusDays(days))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func normalizePublicHomeStatusDays(days int) int {
	if days <= 7 {
		return 7
	}
	if days > publicHomeStatusMaxDays {
		return publicHomeStatusMaxDays
	}
	return publicHomeStatusDefaultDays
}

func buildPublicHomeStatusCached(days int) (PublicHomeStatusResponse, error) {
	days = normalizePublicHomeStatusDays(days)
	now := time.Now()

	publicHomeStatusCache.Lock()
	if cached, ok := publicHomeStatusCache.items[days]; ok && now.Before(cached.expiresAt) {
		result := cached.result
		publicHomeStatusCache.Unlock()
		return result, nil
	}
	if done, ok := publicHomeStatusCache.inFlight[days]; ok {
		publicHomeStatusCache.Unlock()
		<-done
		publicHomeStatusCache.Lock()
		if cached, ok := publicHomeStatusCache.items[days]; ok {
			result := cached.result
			publicHomeStatusCache.Unlock()
			return result, nil
		}
		publicHomeStatusCache.Unlock()
		return buildPublicHomeStatusEmpty(days, true), nil
	}
	done := make(chan struct{})
	publicHomeStatusCache.inFlight[days] = done
	publicHomeStatusCache.Unlock()

	result, err := buildPublicHomeStatus(days)
	cacheTTL := publicHomeStatusCacheTTL
	if err != nil {
		publicHomeStatusCache.Lock()
		if cached, ok := publicHomeStatusCache.items[days]; ok {
			result = cached.result
			result.Partial = true
			err = nil
		} else {
			result = buildPublicHomeStatusEmpty(days, true)
			err = nil
		}
		publicHomeStatusCache.Unlock()
		cacheTTL = publicHomeStatusErrorTTL
	}

	publicHomeStatusCache.Lock()
	publicHomeStatusCache.items[days] = publicHomeStatusCacheItem{
		result:    result,
		expiresAt: time.Now().Add(cacheTTL),
	}
	delete(publicHomeStatusCache.inFlight, days)
	close(done)
	publicHomeStatusCache.Unlock()

	return result, err
}

func buildPublicHomeStatus(days int) (PublicHomeStatusResponse, error) {
	days = normalizePublicHomeStatusDays(days)
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		return PublicHomeStatusResponse{}, err
	}

	channelIds := make([]int, 0, len(channels))
	enabledChannels := 0
	healthyChannels := 0
	channelGroups := make(map[string]map[int]bool)
	groupStates := make(map[string]*PublicHomeStatusGroupMeta)
	for _, channel := range channels {
		if !shouldIncludeChannelInStatusMonitor(channel) {
			continue
		}
		channelIds = append(channelIds, channel.Id)
		if channel.Status == common.ChannelStatusEnabled {
			enabledChannels++
		}
		healthState := channelHealthState(channel, nil)
		if healthState == "healthy" {
			healthyChannels++
		}
		for _, groupKey := range publicHomeStatusGroupKeysForChannel(channel) {
			if channelGroups[groupKey] == nil {
				channelGroups[groupKey] = make(map[int]bool)
			}
			channelGroups[groupKey][channel.Id] = true
			if groupStates[groupKey] == nil {
				groupStates[groupKey] = &PublicHomeStatusGroupMeta{}
			}
			applyPublicHomeStatusChannelState(groupStates[groupKey], healthState)
		}
	}

	startTs := startOfPublicHomeStatusWindow(days).Unix()
	rows, err := model.GetPublicHomeStatusLogs(startTs, channelIds)
	if err != nil {
		return PublicHomeStatusResponse{}, err
	}

	result := buildPublicHomeStatusFromRows(days, rows)
	result.Summary.EnabledChannels = enabledChannels
	result.Summary.HealthyChannels = healthyChannels
	applyPublicHomeStatusGroupChannelSummaries(&result, channelGroups, groupStates)
	result.UpdatedAt = time.Now().Unix()
	return result, nil
}

func startOfPublicHomeStatusWindow(days int) time.Time {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start.AddDate(0, 0, -normalizePublicHomeStatusDays(days)+1)
}

func buildPublicHomeStatusEmpty(days int, partial bool) PublicHomeStatusResponse {
	result := buildPublicHomeStatusFromRows(days, nil)
	result.UpdatedAt = time.Now().Unix()
	result.Partial = partial
	return result
}

func buildPublicHomeStatusFromRows(days int, rows []model.ChannelStatusMonitorLogRow) PublicHomeStatusResponse {
	days = normalizePublicHomeStatusDays(days)
	start := startOfPublicHomeStatusWindow(days)
	buckets, bucketByDate := newPublicHomeStatusBuckets(days, start)

	requests := buildChannelMonitorRequestAggregates(publicHomeStatusRequestLogs(rows))
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].lastRequestAt == requests[j].lastRequestAt {
			return requests[i].lastId < requests[j].lastId
		}
		return requests[i].lastRequestAt < requests[j].lastRequestAt
	})

	overall := &publicHomeStatusBucket{}
	for _, request := range requests {
		if request == nil || request.lastRequestAt <= 0 {
			continue
		}
		date := time.Unix(request.lastRequestAt, 0).Format("2006-01-02")
		bucket := bucketByDate[date]
		if bucket == nil {
			continue
		}
		applyPublicHomeStatusRequest(bucket, request)
		applyPublicHomeStatusRequest(overall, request)
	}

	groups := buildPublicHomeStatusGroups(days, start, requests)
	daily := make([]PublicHomeStatusDaily, 0, len(buckets))
	for _, bucket := range buckets {
		daily = append(daily, PublicHomeStatusDaily{
			Date:            bucket.date,
			Requests:        bucket.requests,
			SuccessRate:     successRate(bucket.successes, bucket.requests),
			AvgLatencyMs:    avgInt64(bucket.latencySum, bucket.latencyCount),
			ProtectedEvents: bucket.protectedEvents,
		})
	}

	return PublicHomeStatusResponse{
		Summary: PublicHomeStatusSummary{
			Days:            days,
			SuccessRate:     successRate(overall.successes, overall.requests),
			AvgLatencyMs:    avgInt64(overall.latencySum, overall.latencyCount),
			Requests:        overall.requests,
			ProtectedEvents: overall.protectedEvents,
		},
		Daily:     daily,
		Groups:    groups,
		UpdatedAt: time.Now().Unix(),
	}
}

type publicHomeStatusBucket struct {
	date            string
	requests        int64
	successes       int64
	latencySum      int64
	latencyCount    int64
	protectedEvents int64
}

func publicHomeStatusRequestLogs(rows []model.ChannelStatusMonitorLogRow) []channelMonitorRequestLog {
	requestLogs := make([]channelMonitorRequestLog, 0, len(rows))
	for _, row := range rows {
		groupName := normalizeChannelGroup(row.Group)
		if groupName == "" {
			groupName = "default"
		}
		requestLogs = append(requestLogs, buildChannelMonitorRequestLog(row, groupName))
	}
	return requestLogs
}

func applyPublicHomeStatusRequest(bucket *publicHomeStatusBucket, request *channelMonitorRequestAgg) {
	if bucket == nil || request == nil {
		return
	}
	bucket.requests++
	if request.success {
		bucket.successes++
	}
	if request.latencyMs > 0 {
		bucket.latencySum += request.latencyMs
		bucket.latencyCount++
	}
	if request.worstStatus != "" && request.worstStatus != "success" {
		bucket.protectedEvents++
	}
}

func newPublicHomeStatusBuckets(days int, start time.Time) ([]*publicHomeStatusBucket, map[string]*publicHomeStatusBucket) {
	buckets := make([]*publicHomeStatusBucket, 0, days)
	bucketByDate := make(map[string]*publicHomeStatusBucket, days)
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		bucket := &publicHomeStatusBucket{date: date}
		buckets = append(buckets, bucket)
		bucketByDate[date] = bucket
	}
	return buckets, bucketByDate
}

func buildPublicHomeStatusGroups(days int, start time.Time, requests []*channelMonitorRequestAgg) []PublicHomeStatusGroup {
	groupBuckets := make(map[string]map[string]*publicHomeStatusBucket)
	groupOverall := make(map[string]*publicHomeStatusBucket)
	for _, item := range publicHomeStatusGroups() {
		_, bucketByDate := newPublicHomeStatusBuckets(days, start)
		groupBuckets[item.key] = bucketByDate
		groupOverall[item.key] = &publicHomeStatusBucket{}
	}

	for _, request := range requests {
		groupKey := publicHomeStatusGroupKeyForRequest(request)
		date := time.Unix(request.lastRequestAt, 0).Format("2006-01-02")
		if bucket := groupBuckets[groupKey][date]; bucket != nil {
			applyPublicHomeStatusRequest(bucket, request)
		}
		applyPublicHomeStatusRequest(groupOverall[groupKey], request)
	}

	result := make([]PublicHomeStatusGroup, 0, len(publicHomeStatusGroups()))
	for _, item := range publicHomeStatusGroups() {
		daily := make([]PublicHomeStatusDaily, 0, days)
		buckets, _ := newPublicHomeStatusBuckets(days, start)
		for _, bucket := range buckets {
			if existing := groupBuckets[item.key][bucket.date]; existing != nil {
				bucket = existing
			}
			daily = append(daily, PublicHomeStatusDaily{
				Date:            bucket.date,
				Requests:        bucket.requests,
				SuccessRate:     successRate(bucket.successes, bucket.requests),
				AvgLatencyMs:    avgInt64(bucket.latencySum, bucket.latencyCount),
				ProtectedEvents: bucket.protectedEvents,
			})
		}
		overall := groupOverall[item.key]
		result = append(result, PublicHomeStatusGroup{
			Key:  item.key,
			Name: item.name,
			Summary: PublicHomeStatusSummary{
				Days:            days,
				SuccessRate:     successRate(overall.successes, overall.requests),
				AvgLatencyMs:    avgInt64(overall.latencySum, overall.latencyCount),
				Requests:        overall.requests,
				ProtectedEvents: overall.protectedEvents,
			},
			Daily: daily,
		})
	}
	return result
}

type publicHomeStatusGroupItem struct {
	key  string
	name string
}

func publicHomeStatusGroups() []publicHomeStatusGroupItem {
	return []publicHomeStatusGroupItem{
		{key: "codex", name: "Codex 专用"},
		{key: "claude", name: "Claude Code"},
		{key: "speed", name: "高速组"},
		{key: "value", name: "低价组"},
	}
}

func publicHomeStatusGroupKeyForRequest(request *channelMonitorRequestAgg) string {
	if request == nil {
		return "speed"
	}
	return publicHomeStatusGroupKeyFromName(request.group)
}

func publicHomeStatusGroupKeysForChannel(channel *model.Channel) []string {
	groups := channel.GetGroups()
	if len(groups) == 0 {
		return []string{"speed"}
	}
	seen := make(map[string]bool)
	keys := make([]string, 0, len(groups))
	for _, group := range groups {
		key := publicHomeStatusGroupKeyFromName(group)
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

func publicHomeStatusGroupKeyFromName(group string) string {
	normalized := strings.ToLower(strings.TrimSpace(group))
	switch {
	case strings.Contains(normalized, "codex"):
		return "codex"
	case strings.Contains(normalized, "claude"), normalized == "cc",
		strings.HasPrefix(normalized, "cc-"), strings.HasPrefix(normalized, "cc_"):
		return "claude"
	case strings.Contains(normalized, "low"), strings.Contains(normalized, "cheap"),
		strings.Contains(normalized, "value"), strings.Contains(normalized, "discount"),
		strings.Contains(normalized, "低价"), strings.Contains(normalized, "经济"):
		return "value"
	default:
		return "speed"
	}
}

func applyPublicHomeStatusGroupChannelSummaries(result *PublicHomeStatusResponse, channelGroups map[string]map[int]bool, groupStates map[string]*PublicHomeStatusGroupMeta) {
	if result == nil {
		return
	}
	for index := range result.Groups {
		group := &result.Groups[index]
		channels := channelGroups[group.Key]
		states := groupStates[group.Key]
		if states == nil {
			states = &PublicHomeStatusGroupMeta{}
		}
		states.Channels = len(channels)
		group.States = *states
		group.Summary.EnabledChannels = len(channels)
		group.Summary.HealthyChannels = states.Healthy
	}
}

func applyPublicHomeStatusChannelState(states *PublicHomeStatusGroupMeta, healthState string) {
	if states == nil {
		return
	}
	switch healthState {
	case "healthy":
		states.Healthy++
	case "warning":
		states.Cooling++
	default:
		states.Standby++
	}
}
