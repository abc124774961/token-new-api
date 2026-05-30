package service

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	channelAffinityDiagnosticsDefaultLimit = 5000
	channelAffinityDiagnosticsMaxLimit     = 50000
)

type ChannelAffinityDiagnosticsQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	ModelName      string
	Username       string
	TokenName      string
	Group          string
	ChannelID      int
	RuleName       string
	KeySource      string
	KeyFingerprint string
	Limit          int
}

type ChannelAffinityDiagnosticsResponse struct {
	Summary ChannelAffinityDiagnosticsSummary `json:"summary"`
	Rows    []ChannelAffinityDiagnosticsRow   `json:"rows"`
	Limit   int                               `json:"limit"`
}

type ChannelAffinityDiagnosticsSummary struct {
	TotalLogs                   int64            `json:"total_logs"`
	AffinityLogs                int64            `json:"affinity_logs"`
	NoAffinityLogs              int64            `json:"no_affinity_logs"`
	CacheHits                   int64            `json:"cache_hits"`
	PromptTokens                int64            `json:"prompt_tokens"`
	CachedTokens                int64            `json:"cached_tokens"`
	PromptCacheHitTokens        int64            `json:"prompt_cache_hit_tokens"`
	Retained                    int64            `json:"retained"`
	Broken                      int64            `json:"broken"`
	ChannelSwitches             int64            `json:"channel_switches"`
	AccountSwitches             int64            `json:"account_switches"`
	KeyChanges                  int64            `json:"key_changes"`
	UpstreamNoCachedTokenLogs   int64            `json:"upstream_no_cached_token_logs"`
	DistinctKeys                int              `json:"distinct_keys"`
	CachedTokenRate             float64          `json:"cached_token_rate"`
	HitRate                     float64          `json:"hit_rate"`
	KeySources                  map[string]int64 `json:"key_sources"`
	BreakReasons                map[string]int64 `json:"break_reasons"`
	CachedTokenRateModes        map[string]int64 `json:"cached_token_rate_modes"`
	ScannedLimitMayTruncateData bool             `json:"scanned_limit_may_truncate_data"`
}

type ChannelAffinityDiagnosticsRow struct {
	Key                         string           `json:"key"`
	RuleName                    string           `json:"rule_name"`
	KeySource                   string           `json:"key_source"`
	KeyFingerprint              string           `json:"key_fp"`
	ModelName                   string           `json:"model_name"`
	Group                       string           `json:"group"`
	ChannelID                   int              `json:"channel_id"`
	ChannelName                 string           `json:"channel_name,omitempty"`
	PreferredChannelID          int              `json:"preferred_channel_id,omitempty"`
	SelectedChannelID           int              `json:"selected_channel_id,omitempty"`
	AccountID                   string           `json:"account_id,omitempty"`
	AccountType                 string           `json:"account_type,omitempty"`
	AccountIdentityKey          string           `json:"account_identity_key,omitempty"`
	CredentialIndex             *int             `json:"credential_index,omitempty"`
	ResourceID                  string           `json:"resource_id,omitempty"`
	ResourceType                string           `json:"resource_type,omitempty"`
	ProxyID                     int              `json:"proxy_id,omitempty"`
	Total                       int64            `json:"total"`
	CacheHits                   int64            `json:"cache_hits"`
	HitRate                     float64          `json:"hit_rate"`
	PromptTokens                int64            `json:"prompt_tokens"`
	CachedTokens                int64            `json:"cached_tokens"`
	PromptCacheHitTokens        int64            `json:"prompt_cache_hit_tokens"`
	CachedTokenRate             float64          `json:"cached_token_rate"`
	Retained                    int64            `json:"retained"`
	Broken                      int64            `json:"broken"`
	ChannelSwitches             int64            `json:"channel_switches"`
	UpstreamNoCachedTokenLogs   int64            `json:"upstream_no_cached_token_logs"`
	FirstSeenAt                 int64            `json:"first_seen_at,omitempty"`
	LastSeenAt                  int64            `json:"last_seen_at,omitempty"`
	BreakReasons                map[string]int64 `json:"break_reasons,omitempty"`
	CachedTokenRateMode         string           `json:"cached_token_rate_mode,omitempty"`
	ScannedLimitMayTruncateData bool             `json:"scanned_limit_may_truncate_data,omitempty"`
}

type channelAffinityDiagnosticsAccumulator struct {
	row              ChannelAffinityDiagnosticsRow
	rateModeCounts   map[string]int64
	credentialIndex  int
	hasCredentialIdx bool
}

func GetChannelAffinityDiagnostics(query ChannelAffinityDiagnosticsQuery) (ChannelAffinityDiagnosticsResponse, error) {
	if model.LOG_DB == nil {
		return ChannelAffinityDiagnosticsResponse{}, errors.New("log database is not initialized")
	}
	limit := normalizeChannelAffinityDiagnosticsLimit(query.Limit)
	logs, truncated, err := queryChannelAffinityDiagnosticsLogs(query, limit)
	if err != nil {
		return ChannelAffinityDiagnosticsResponse{}, err
	}
	channelNames := channelNamesForDiagnostics(logs)

	resp := ChannelAffinityDiagnosticsResponse{
		Summary: ChannelAffinityDiagnosticsSummary{
			KeySources:           map[string]int64{},
			BreakReasons:         map[string]int64{},
			CachedTokenRateModes: map[string]int64{},
		},
		Limit: limit,
	}
	resp.Summary.ScannedLimitMayTruncateData = truncated
	rowsByKey := map[string]*channelAffinityDiagnosticsAccumulator{}
	distinctKeys := map[string]struct{}{}
	lastKeyByScope := map[string]string{}
	lastAccountByKey := map[string]string{}

	for _, log := range logs {
		if log == nil {
			continue
		}
		other := channelAffinityDiagnosticsOtherMap(log.Other)
		affinity := channelAffinityDiagnosticsAffinityInfo(other)
		hasAffinity := len(affinity) > 0
		if !channelAffinityDiagnosticsMatches(query, affinity, hasAffinity) {
			continue
		}

		resp.Summary.TotalLogs++
		promptTokens := int64(log.PromptTokens)
		cachedTokens := channelAffinityDiagnosticsInt64(other, "cache_tokens")
		if cachedTokens == 0 {
			cachedTokens = channelAffinityDiagnosticsInt64(other, "cached_tokens")
		}
		promptCacheHitTokens := channelAffinityDiagnosticsInt64(other, "prompt_cache_hit_tokens")
		cacheHit := cachedTokens > 0 || promptCacheHitTokens > 0
		rateMode := channelAffinityDiagnosticsRateMode(other)

		if !hasAffinity {
			resp.Summary.NoAffinityLogs++
			acc := channelAffinityDiagnosticsRowAccumulator(rowsByKey, ChannelAffinityDiagnosticsRow{
				Key:       "no_affinity_key",
				KeySource: "no_key_or_unmatched",
				ModelName: log.ModelName,
				Group:     log.Group,
				ChannelID: log.ChannelId,
			})
			channelAffinityDiagnosticsApplyLogToRow(acc, log, channelNames, promptTokens, cachedTokens, promptCacheHitTokens, cacheHit, rateMode, "", false, false, 0, 0)
			continue
		}

		resp.Summary.AffinityLogs++
		ruleName := channelAffinityDiagnosticsString(affinity, "rule_name")
		if ruleName == "" {
			ruleName = channelAffinityDiagnosticsString(affinity, "reason")
		}
		keySource := channelAffinityDiagnosticsString(affinity, "key_source")
		if keySource == "" {
			keySource = channelAffinityDiagnosticsString(affinity, "key_path")
		}
		if keySource == "" {
			keySource = channelAffinityDiagnosticsString(affinity, "key_key")
		}
		if keySource == "" {
			keySource = "unknown"
		}
		keyFp := channelAffinityDiagnosticsString(affinity, "key_fp")
		if keyFp != "" {
			distinctKeys[keyFp] = struct{}{}
		}
		resp.Summary.KeySources[keySource]++
		if rateMode != "" {
			resp.Summary.CachedTokenRateModes[rateMode]++
		}

		selectedChannelID := channelAffinityDiagnosticsInt(affinity, "selected_channel_id")
		if selectedChannelID <= 0 {
			selectedChannelID = channelAffinityDiagnosticsInt(affinity, "channel_id")
		}
		if selectedChannelID <= 0 {
			selectedChannelID = log.ChannelId
		}
		preferredChannelID := channelAffinityDiagnosticsInt(affinity, "preferred_channel_id")
		retained := channelAffinityDiagnosticsBool(affinity, "retained")
		broken := channelAffinityDiagnosticsBool(affinity, "broken")
		breakReason := channelAffinityDiagnosticsString(affinity, "break_reason")
		if preferredChannelID > 0 && selectedChannelID > 0 && preferredChannelID != selectedChannelID {
			resp.Summary.ChannelSwitches++
			broken = true
			if breakReason == "" {
				breakReason = "selected_channel_changed"
			}
		}
		if retained {
			resp.Summary.Retained++
		}
		if broken {
			resp.Summary.Broken++
		}
		if breakReason != "" {
			resp.Summary.BreakReasons[breakReason]++
		}
		if cacheHit {
			resp.Summary.CacheHits++
		}
		resp.Summary.PromptTokens += promptTokens
		resp.Summary.CachedTokens += cachedTokens
		resp.Summary.PromptCacheHitTokens += promptCacheHitTokens
		if promptTokens > 0 && !cacheHit {
			resp.Summary.UpstreamNoCachedTokenLogs++
		}

		scope := log.Username + "\n" + log.TokenName + "\n" + log.ModelName + "\n" + log.Group
		if keyFp != "" {
			if prev, ok := lastKeyByScope[scope]; ok && prev != "" && prev != keyFp {
				resp.Summary.KeyChanges++
			}
			lastKeyByScope[scope] = keyFp
		}

		accountLabel := channelAffinityDiagnosticsAccountLabel(affinity)
		if keyFp != "" && accountLabel != "" {
			if prev, ok := lastAccountByKey[keyFp]; ok && prev != "" && prev != accountLabel {
				resp.Summary.AccountSwitches++
			}
			lastAccountByKey[keyFp] = accountLabel
		}

		row := ChannelAffinityDiagnosticsRow{
			RuleName:           ruleName,
			KeySource:          keySource,
			KeyFingerprint:     keyFp,
			ModelName:          log.ModelName,
			Group:              log.Group,
			ChannelID:          selectedChannelID,
			PreferredChannelID: preferredChannelID,
			SelectedChannelID:  selectedChannelID,
			AccountID:          channelAffinityDiagnosticsString(affinity, "account_id"),
			AccountType:        channelAffinityDiagnosticsString(affinity, "account_type"),
			AccountIdentityKey: channelAffinityDiagnosticsString(affinity, "account_identity_key"),
			ResourceID:         channelAffinityDiagnosticsString(affinity, "resource_id"),
			ResourceType:       channelAffinityDiagnosticsString(affinity, "resource_type"),
			ProxyID:            channelAffinityDiagnosticsInt(affinity, "proxy_id"),
		}
		if idx, ok := channelAffinityDiagnosticsIntOK(affinity, "credential_index"); ok {
			row.CredentialIndex = &idx
		}
		acc := channelAffinityDiagnosticsRowAccumulator(rowsByKey, row)
		channelAffinityDiagnosticsApplyLogToRow(acc, log, channelNames, promptTokens, cachedTokens, promptCacheHitTokens, cacheHit, rateMode, breakReason, retained, broken, preferredChannelID, selectedChannelID)
	}

	resp.Summary.DistinctKeys = len(distinctKeys)
	resp.Summary.HitRate = channelAffinityDiagnosticsRatio(resp.Summary.CacheHits, resp.Summary.AffinityLogs)
	resp.Summary.CachedTokenRate = channelAffinityDiagnosticsTokenRate(resp.Summary.CachedTokens, resp.Summary.PromptTokens, resp.Summary.CachedTokenRateModes)

	resp.Rows = make([]ChannelAffinityDiagnosticsRow, 0, len(rowsByKey))
	for _, acc := range rowsByKey {
		row := acc.row
		row.HitRate = channelAffinityDiagnosticsRatio(row.CacheHits, row.Total)
		row.CachedTokenRate = channelAffinityDiagnosticsTokenRate(row.CachedTokens, row.PromptTokens, acc.rateModeCounts)
		row.CachedTokenRateMode = channelAffinityDiagnosticsDominantRateMode(acc.rateModeCounts)
		row.ScannedLimitMayTruncateData = truncated
		if acc.hasCredentialIdx {
			idx := acc.credentialIndex
			row.CredentialIndex = &idx
		}
		resp.Rows = append(resp.Rows, row)
	}
	sort.Slice(resp.Rows, func(i, j int) bool {
		if resp.Rows[i].Total == resp.Rows[j].Total {
			return resp.Rows[i].LastSeenAt > resp.Rows[j].LastSeenAt
		}
		return resp.Rows[i].Total > resp.Rows[j].Total
	})
	return resp, nil
}

func normalizeChannelAffinityDiagnosticsLimit(limit int) int {
	if limit <= 0 {
		return channelAffinityDiagnosticsDefaultLimit
	}
	if limit > channelAffinityDiagnosticsMaxLimit {
		return channelAffinityDiagnosticsMaxLimit
	}
	return limit
}

func queryChannelAffinityDiagnosticsLogs(query ChannelAffinityDiagnosticsQuery, limit int) ([]*model.Log, bool, error) {
	tx := model.LOG_DB.Model(&model.Log{}).
		Where("logs.type IN ?", []int{model.LogTypeConsume, model.LogTypeError})
	logGroupColumn := "logs." + model.LogGroupColumn()
	if query.StartTimestamp > 0 {
		tx = tx.Where("logs.created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		tx = tx.Where("logs.created_at <= ?", query.EndTimestamp)
	}
	if query.ModelName != "" {
		tx = tx.Where("logs.model_name LIKE ?", query.ModelName)
	}
	if query.Username != "" {
		tx = tx.Where("logs.username = ?", query.Username)
	}
	if query.TokenName != "" {
		tx = tx.Where("logs.token_name = ?", query.TokenName)
	}
	if query.Group != "" {
		tx = tx.Where(logGroupColumn+" = ?", query.Group)
	}
	if query.ChannelID > 0 {
		tx = tx.Where("logs.channel_id = ?", query.ChannelID)
	}

	rows := make([]*model.Log, 0, limit)
	err := tx.
		Select("logs.id, logs.created_at, logs.type, logs.username, logs.token_name, logs.model_name, logs.prompt_tokens, logs.completion_tokens, logs.channel_id, " + logGroupColumn + ", logs.other").
		Order("logs.id asc").
		Limit(limit + 1).
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	truncated := len(rows) > limit
	if truncated {
		rows = rows[:limit]
	}
	return rows, truncated, nil
}

func channelNamesForDiagnostics(logs []*model.Log) map[int]string {
	channelIDs := make([]int, 0)
	seen := map[int]struct{}{}
	addID := func(channelID int) {
		if channelID <= 0 {
			return
		}
		if _, ok := seen[channelID]; ok {
			return
		}
		seen[channelID] = struct{}{}
		channelIDs = append(channelIDs, channelID)
	}
	for _, log := range logs {
		if log == nil {
			continue
		}
		addID(log.ChannelId)
		other := channelAffinityDiagnosticsOtherMap(log.Other)
		affinity := channelAffinityDiagnosticsAffinityInfo(other)
		if len(affinity) > 0 {
			addID(channelAffinityDiagnosticsInt(affinity, "channel_id"))
			addID(channelAffinityDiagnosticsInt(affinity, "preferred_channel_id"))
			addID(channelAffinityDiagnosticsInt(affinity, "selected_channel_id"))
		}
	}
	if len(channelIDs) == 0 || model.DB == nil {
		return nil
	}
	var channels []struct {
		Id   int    `gorm:"column:id"`
		Name string `gorm:"column:name"`
	}
	if err := model.DB.Table("channels").Select("id, name").Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		common.SysLog("failed to load channel affinity diagnostic channel names: " + err.Error())
		return nil
	}
	out := make(map[int]string, len(channels))
	for _, channel := range channels {
		out[channel.Id] = channel.Name
	}
	return out
}

func channelAffinityDiagnosticsMatches(query ChannelAffinityDiagnosticsQuery, affinity map[string]interface{}, hasAffinity bool) bool {
	if !hasAffinity {
		return strings.TrimSpace(query.RuleName) == "" &&
			strings.TrimSpace(query.KeySource) == "" &&
			strings.TrimSpace(query.KeyFingerprint) == ""
	}
	if query.RuleName != "" {
		ruleName := channelAffinityDiagnosticsString(affinity, "rule_name")
		if ruleName == "" {
			ruleName = channelAffinityDiagnosticsString(affinity, "reason")
		}
		if ruleName != query.RuleName {
			return false
		}
	}
	if query.KeySource != "" {
		keySource := channelAffinityDiagnosticsString(affinity, "key_source")
		if keySource == "" {
			keySource = channelAffinityDiagnosticsString(affinity, "key_path")
		}
		if keySource != query.KeySource {
			return false
		}
	}
	if query.KeyFingerprint != "" && channelAffinityDiagnosticsString(affinity, "key_fp") != query.KeyFingerprint {
		return false
	}
	return true
}

func channelAffinityDiagnosticsOtherMap(raw string) map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := common.UnmarshalJsonStr(raw, &out); err != nil || out == nil {
		return map[string]interface{}{}
	}
	return out
}

func channelAffinityDiagnosticsAffinityInfo(other map[string]interface{}) map[string]interface{} {
	adminInfo := channelAffinityDiagnosticsMap(other["admin_info"])
	if len(adminInfo) == 0 {
		return nil
	}
	affinity := channelAffinityDiagnosticsMap(adminInfo["channel_affinity"])
	if len(affinity) == 0 {
		return nil
	}
	return affinity
}

func channelAffinityDiagnosticsMap(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	case string:
		out := map[string]interface{}{}
		if err := common.UnmarshalJsonStr(typed, &out); err != nil || out == nil {
			return nil
		}
		return out
	default:
		return nil
	}
}

func channelAffinityDiagnosticsRowAccumulator(rows map[string]*channelAffinityDiagnosticsAccumulator, row ChannelAffinityDiagnosticsRow) *channelAffinityDiagnosticsAccumulator {
	key := strings.Join([]string{
		row.RuleName,
		row.KeySource,
		row.KeyFingerprint,
		row.ModelName,
		row.Group,
		strconv.Itoa(row.ChannelID),
		row.AccountID,
		row.AccountIdentityKey,
		channelAffinityDiagnosticsCredentialKey(row.CredentialIndex),
	}, "\n")
	if key == "" {
		key = "unknown"
	}
	if acc, ok := rows[key]; ok {
		return acc
	}
	row.Key = key
	acc := &channelAffinityDiagnosticsAccumulator{
		row:            row,
		rateModeCounts: map[string]int64{},
	}
	if row.CredentialIndex != nil {
		acc.credentialIndex = *row.CredentialIndex
		acc.hasCredentialIdx = true
	}
	rows[key] = acc
	return acc
}

func channelAffinityDiagnosticsApplyLogToRow(acc *channelAffinityDiagnosticsAccumulator, log *model.Log, channelNames map[int]string, promptTokens, cachedTokens, promptCacheHitTokens int64, cacheHit bool, rateMode, breakReason string, retained, broken bool, preferredChannelID, selectedChannelID int) {
	if acc == nil || log == nil {
		return
	}
	row := &acc.row
	row.Total++
	row.PromptTokens += promptTokens
	row.CachedTokens += cachedTokens
	row.PromptCacheHitTokens += promptCacheHitTokens
	if cacheHit {
		row.CacheHits++
	}
	if retained {
		row.Retained++
	}
	if broken {
		row.Broken++
	}
	if preferredChannelID > 0 && selectedChannelID > 0 && preferredChannelID != selectedChannelID {
		row.ChannelSwitches++
	}
	if promptTokens > 0 && !cacheHit {
		row.UpstreamNoCachedTokenLogs++
	}
	if breakReason != "" {
		if row.BreakReasons == nil {
			row.BreakReasons = map[string]int64{}
		}
		row.BreakReasons[breakReason]++
	}
	if rateMode != "" {
		acc.rateModeCounts[rateMode]++
	}
	if row.FirstSeenAt == 0 || log.CreatedAt < row.FirstSeenAt {
		row.FirstSeenAt = log.CreatedAt
	}
	if log.CreatedAt > row.LastSeenAt {
		row.LastSeenAt = log.CreatedAt
	}
	if row.ChannelName == "" {
		row.ChannelName = channelNames[row.ChannelID]
	}
}

func channelAffinityDiagnosticsCredentialKey(idx *int) string {
	if idx == nil {
		return ""
	}
	return strconv.Itoa(*idx)
}

func channelAffinityDiagnosticsAccountLabel(affinity map[string]interface{}) string {
	for _, key := range []string{"account_identity_key", "account_id", "credential_fingerprint", "credential_subject_fingerprint"} {
		if v := channelAffinityDiagnosticsString(affinity, key); v != "" {
			return v
		}
	}
	if idx, ok := channelAffinityDiagnosticsIntOK(affinity, "credential_index"); ok {
		return "credential_index:" + strconv.Itoa(idx)
	}
	return ""
}

func channelAffinityDiagnosticsRateMode(other map[string]interface{}) string {
	mode := channelAffinityDiagnosticsString(other, "cache_token_rate_mode")
	switch mode {
	case cacheTokenRateModeCachedOverPrompt, cacheTokenRateModeCachedOverPromptPlusCached, cacheTokenRateModeMixed:
		return mode
	}
	if channelAffinityDiagnosticsString(other, "usage_semantic") == "anthropic" {
		return cacheTokenRateModeCachedOverPromptPlusCached
	}
	return cacheTokenRateModeCachedOverPrompt
}

func channelAffinityDiagnosticsDominantRateMode(counts map[string]int64) string {
	bestMode := ""
	var bestCount int64
	for mode, count := range counts {
		if count > bestCount || count == bestCount && mode < bestMode {
			bestMode = mode
			bestCount = count
		}
	}
	if len(counts) > 1 {
		return cacheTokenRateModeMixed
	}
	return bestMode
}

func channelAffinityDiagnosticsTokenRate(cachedTokens, promptTokens int64, modes map[string]int64) float64 {
	mode := channelAffinityDiagnosticsDominantRateMode(modes)
	denominator := promptTokens
	if mode == cacheTokenRateModeCachedOverPromptPlusCached {
		denominator += cachedTokens
	}
	return channelAffinityDiagnosticsRatio(cachedTokens, denominator)
}

func channelAffinityDiagnosticsRatio(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func channelAffinityDiagnosticsString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	value, ok := m[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	default:
		return ""
	}
}

func channelAffinityDiagnosticsBool(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	value, ok := m[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		v := strings.TrimSpace(strings.ToLower(typed))
		return v == "true" || v == "1" || v == "yes"
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case int64:
		return typed != 0
	default:
		return false
	}
}

func channelAffinityDiagnosticsInt(m map[string]interface{}, key string) int {
	value, ok := channelAffinityDiagnosticsIntOK(m, key)
	if !ok {
		return 0
	}
	return value
}

func channelAffinityDiagnosticsIntOK(m map[string]interface{}, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	value, ok := m[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case int32:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(typed))
		return i, err == nil
	default:
		return 0, false
	}
}

func channelAffinityDiagnosticsInt64(m map[string]interface{}, key string) int64 {
	if m == nil {
		return 0
	}
	value, ok := m[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return i
		}
	}
	return 0
}
