package testkit

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"gorm.io/gorm"
)

type ModelExecutionObservationSummary struct {
	Total             int                                      `json:"total"`
	Successes         int                                      `json:"successes"`
	Failures          int                                      `json:"failures"`
	SuccessRate       float64                                  `json:"success_rate"`
	AvgDurationMs     float64                                  `json:"avg_duration_ms"`
	AvgTTFTMs         float64                                  `json:"avg_ttft_ms"`
	Fallbacks         int                                      `json:"fallbacks"`
	FallbackRate      float64                                  `json:"fallback_rate"`
	StreamInterrupted int                                      `json:"stream_interrupted"`
	ActiveRecords     int                                      `json:"active_records"`
	ShadowRecords     int                                      `json:"shadow_records"`
	ScoreBreakdown    map[string]float64                       `json:"score_breakdown"`
	Models            map[string]ModelExecutionObservationStat `json:"models"`
	Groups            map[string]ModelExecutionObservationStat `json:"groups"`
	Channels          map[int]ModelExecutionObservationStat    `json:"channels"`
}

type ModelExecutionObservationStat struct {
	Total             int     `json:"total"`
	Successes         int     `json:"successes"`
	Failures          int     `json:"failures"`
	SuccessRate       float64 `json:"success_rate"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
	AvgTTFTMs         float64 `json:"avg_ttft_ms"`
	Fallbacks         int     `json:"fallbacks"`
	FallbackRate      float64 `json:"fallback_rate"`
	StreamInterrupted int     `json:"stream_interrupted"`
}

func ModelExecutionObservationRecords() ([]model.ModelExecutionRecord, error) {
	candidateDefaultVIP, err := observationJSON([]string{"default", "vip"})
	if err != nil {
		return nil, err
	}
	candidateVIP, err := observationJSON([]string{"vip"})
	if err != nil {
		return nil, err
	}
	candidateDefaultCanary, err := observationJSON([]string{"default", "canary"})
	if err != nil {
		return nil, err
	}
	candidateResearch, err := observationJSON([]string{"research", "vip"})
	if err != nil {
		return nil, err
	}
	records := []model.ModelExecutionRecord{
		{
			CreatedAt:         1710000001,
			RequestId:         "obs-success-001",
			UserId:            1001,
			TokenId:           2001,
			RequestedGroup:    "default",
			SelectedGroup:     "default",
			ActualGroup:       "default",
			RequestedModel:    "gpt-4.1",
			ChannelId:         101,
			ChannelName:       "openai-default-a",
			ActualChannelId:   101,
			ActualChannelName: "openai-default-a",
			EndpointType:      string(constant.EndpointTypeOpenAI),
			PolicyMode:        core.ModeActive,
			AutoMode:          core.AutoModeFusion,
			Strategy:          core.StrategyBalanced,
			SmartHandled:      true,
			Success:           true,
			StatusCode:        200,
			DurationMs:        800,
			TTFTMs:            120,
			ScoreTotal:        0.93,
			ScoreBreakdown:    mustObservationJSON(map[string]float64{"success": 1, "speed": 0.8, "load": 0.9, "cost": 0.7, "group": 1}),
			CandidateGroups:   candidateDefaultVIP,
			SelectedReason:    "weighted_score",
			RequestMeta:       mustObservationJSON(ReplayRequestMeta{OriginalModelName: "gpt-4.1", UserUsingGroup: "default"}),
		},
		{
			CreatedAt:         1710000002,
			RequestId:         "obs-failure-001",
			AttemptIndex:      1,
			UserId:            1002,
			TokenId:           2002,
			RequestedGroup:    "auto",
			SelectedGroup:     "vip",
			ActualGroup:       "vip",
			RequestedModel:    "gpt-4.1",
			ChannelId:         102,
			ChannelName:       "openai-vip-b",
			ActualChannelId:   102,
			ActualChannelName: "openai-vip-b",
			EndpointType:      string(constant.EndpointTypeOpenAIResponse),
			PolicyMode:        core.ModeActive,
			AutoMode:          core.AutoModeFusion,
			Strategy:          core.StrategyBalanced,
			SmartHandled:      true,
			FallbackUsed:      true,
			Success:           false,
			StatusCode:        502,
			ErrorCode:         "upstream_bad_gateway",
			ErrorType:         "upstream",
			DurationMs:        1600,
			TTFTMs:            260,
			ScoreTotal:        0.62,
			ScoreBreakdown:    mustObservationJSON(map[string]float64{"success": 0.55, "speed": 0.5, "load": 0.6, "cost": 0.8, "group": 0.95}),
			CandidateGroups:   candidateDefaultVIP,
			SelectedReason:    "weighted_score_fallback",
			RequestMeta:       mustObservationJSON(ReplayRequestMeta{OriginalModelName: "gpt-4.1", UserUsingGroup: "vip"}),
		},
		{
			CreatedAt:         1710000003,
			RequestId:         "obs-stream-001",
			UserId:            1003,
			TokenId:           2003,
			RequestedGroup:    "vip",
			SelectedGroup:     "vip",
			ActualGroup:       "vip",
			RequestedModel:    "claude-3.7-sonnet",
			ChannelId:         103,
			ChannelName:       "anthropic-vip-a",
			ActualChannelId:   103,
			ActualChannelName: "anthropic-vip-a",
			EndpointType:      string(constant.EndpointTypeAnthropic),
			PolicyMode:        core.ModeActive,
			AutoMode:          core.AutoModeSequential,
			Strategy:          core.StrategyStabilityFirst,
			SmartHandled:      true,
			FallbackUsed:      true,
			Success:           false,
			StatusCode:        499,
			ErrorCode:         "stream_eof",
			ErrorType:         "stream",
			DurationMs:        2400,
			TTFTMs:            180,
			StreamInterrupted: true,
			ScoreTotal:        0.67,
			ScoreBreakdown:    mustObservationJSON(map[string]float64{"success": 0.4, "speed": 0.65, "load": 0.55, "cost": 0.75, "group": 1}),
			CandidateGroups:   candidateVIP,
			SelectedReason:    "weighted_score_stream_retry",
			RequestMeta:       mustObservationJSON(ReplayRequestMeta{OriginalModelName: "claude-3.7-sonnet", UserUsingGroup: "vip"}),
		},
		{
			CreatedAt:         1710000004,
			RequestId:         "obs-shadow-001",
			UserId:            1004,
			TokenId:           2004,
			RequestedGroup:    "default",
			SelectedGroup:     "canary",
			ActualGroup:       "default",
			RequestedModel:    "gpt-4.1",
			ChannelId:         104,
			ChannelName:       "openai-canary-shadow",
			ActualChannelId:   101,
			ActualChannelName: "openai-default-a",
			EndpointType:      string(constant.EndpointTypeOpenAI),
			PolicyMode:        core.ModeShadow,
			AutoMode:          core.AutoModeFusion,
			Strategy:          core.StrategySpeedFirst,
			Shadow:            true,
			SmartHandled:      true,
			Success:           true,
			StatusCode:        200,
			DurationMs:        700,
			TTFTMs:            100,
			ScoreTotal:        0.96,
			ScoreBreakdown:    mustObservationJSON(map[string]float64{"success": 0.96, "speed": 0.9, "load": 0.85, "cost": 0.6, "group": 0.7}),
			CandidateGroups:   candidateDefaultCanary,
			SelectedReason:    "weighted_score_shadow",
			RequestMeta:       mustObservationJSON(ReplayRequestMeta{OriginalModelName: "gpt-4.1", UserUsingGroup: "default"}),
		},
		{
			CreatedAt:         1710000005,
			RequestId:         "obs-success-002",
			UserId:            1005,
			TokenId:           2005,
			RequestedGroup:    "research",
			SelectedGroup:     "research",
			ActualGroup:       "research",
			RequestedModel:    "gemini-2.5-pro",
			ChannelId:         105,
			ChannelName:       "gemini-research-a",
			ActualChannelId:   105,
			ActualChannelName: "gemini-research-a",
			EndpointType:      string(constant.EndpointTypeGemini),
			PolicyMode:        core.ModeActive,
			AutoMode:          core.AutoModeFusion,
			Strategy:          core.StrategyCostFirst,
			SmartHandled:      true,
			Success:           true,
			StatusCode:        200,
			DurationMs:        1100,
			TTFTMs:            140,
			ScoreTotal:        0.89,
			ScoreBreakdown:    mustObservationJSON(map[string]float64{"success": 0.98, "speed": 0.75, "load": 0.7, "cost": 0.65, "group": 0.9}),
			CandidateGroups:   candidateResearch,
			SelectedReason:    "weighted_score",
			RequestMeta:       mustObservationJSON(ReplayRequestMeta{OriginalModelName: "gemini-2.5-pro", UserUsingGroup: "research"}),
		},
		{
			CreatedAt:         1710000006,
			RequestId:         "obs-failure-002",
			UserId:            1006,
			TokenId:           2006,
			RequestedGroup:    "default",
			SelectedGroup:     "default",
			ActualGroup:       "default",
			RequestedModel:    "claude-3.7-sonnet",
			ChannelId:         101,
			ChannelName:       "openai-default-a",
			ActualChannelId:   101,
			ActualChannelName: "openai-default-a",
			EndpointType:      string(constant.EndpointTypeOpenAI),
			PolicyMode:        core.ModeActive,
			AutoMode:          core.AutoModeFusion,
			Strategy:          core.StrategyBalanced,
			SmartHandled:      true,
			Success:           false,
			StatusCode:        429,
			ErrorCode:         "rate_limit",
			ErrorType:         "upstream",
			DurationMs:        900,
			ScoreTotal:        0.58,
			ScoreBreakdown:    mustObservationJSON(map[string]float64{"success": 0.35, "speed": 0.45, "load": 0.5, "cost": 0.95, "group": 1}),
			CandidateGroups:   candidateDefaultVIP,
			SelectedReason:    "weighted_score_rate_limit",
			RequestMeta:       mustObservationJSON(ReplayRequestMeta{OriginalModelName: "claude-3.7-sonnet", UserUsingGroup: "default"}),
		},
	}
	return records, nil
}

func SeedModelExecutionObservationRecords(db *gorm.DB) ([]model.ModelExecutionRecord, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	records, err := ModelExecutionObservationRecords()
	if err != nil {
		return nil, err
	}
	if err := db.Create(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func SummarizeModelExecutionObservationRecords(records []model.ModelExecutionRecord) (ModelExecutionObservationSummary, error) {
	overall := newObservationAccumulator()
	models := map[string]*observationAccumulator{}
	groups := map[string]*observationAccumulator{}
	channels := map[int]*observationAccumulator{}
	scoreSums := map[string]float64{}
	scoreCounts := map[string]int{}

	for _, record := range records {
		overall.add(record)
		if record.PolicyMode == core.ModeActive {
			overall.activeRecords++
		}
		if record.Shadow || record.PolicyMode == core.ModeShadow {
			overall.shadowRecords++
		}
		if record.RequestedModel != "" {
			observationBucket(models, record.RequestedModel).add(record)
		}
		group := record.SelectedGroup
		if group == "" {
			group = record.RequestedGroup
		}
		if group != "" {
			observationBucket(groups, group).add(record)
		}
		if record.ChannelId > 0 {
			observationChannelBucket(channels, record.ChannelId).add(record)
		}

		scoreBreakdown, err := parseObservationScoreBreakdown(record.ScoreBreakdown)
		if err != nil {
			return ModelExecutionObservationSummary{}, fmt.Errorf("record %s score_breakdown: %w", record.RequestId, err)
		}
		for key, value := range scoreBreakdown {
			scoreSums[key] += value
			scoreCounts[key]++
		}
	}

	summary := overall.summary()
	return ModelExecutionObservationSummary{
		Total:             summary.Total,
		Successes:         summary.Successes,
		Failures:          summary.Failures,
		SuccessRate:       summary.SuccessRate,
		AvgDurationMs:     summary.AvgDurationMs,
		AvgTTFTMs:         summary.AvgTTFTMs,
		Fallbacks:         summary.Fallbacks,
		FallbackRate:      summary.FallbackRate,
		StreamInterrupted: summary.StreamInterrupted,
		ActiveRecords:     overall.activeRecords,
		ShadowRecords:     overall.shadowRecords,
		ScoreBreakdown:    averageObservationScores(scoreSums, scoreCounts),
		Models:            finalizeObservationBuckets(models),
		Groups:            finalizeObservationBuckets(groups),
		Channels:          finalizeObservationChannelBuckets(channels),
	}, nil
}

type observationAccumulator struct {
	total             int
	successes         int
	durationSum       int64
	durationCount     int
	ttftSum           int64
	ttftCount         int
	fallbacks         int
	streamInterrupted int
	activeRecords     int
	shadowRecords     int
}

func newObservationAccumulator() *observationAccumulator {
	return &observationAccumulator{}
}

func (a *observationAccumulator) add(record model.ModelExecutionRecord) {
	a.total++
	if record.Success {
		a.successes++
	}
	if record.DurationMs > 0 {
		a.durationSum += record.DurationMs
		a.durationCount++
	}
	if record.TTFTMs > 0 {
		a.ttftSum += record.TTFTMs
		a.ttftCount++
	}
	if record.FallbackUsed {
		a.fallbacks++
	}
	if record.StreamInterrupted {
		a.streamInterrupted++
	}
}

func (a *observationAccumulator) summary() ModelExecutionObservationStat {
	if a == nil {
		return ModelExecutionObservationStat{}
	}
	return ModelExecutionObservationStat{
		Total:             a.total,
		Successes:         a.successes,
		Failures:          a.total - a.successes,
		SuccessRate:       observationRatio(a.successes, a.total),
		AvgDurationMs:     observationAverage(a.durationSum, a.durationCount),
		AvgTTFTMs:         observationAverage(a.ttftSum, a.ttftCount),
		Fallbacks:         a.fallbacks,
		FallbackRate:      observationRatio(a.fallbacks, a.total),
		StreamInterrupted: a.streamInterrupted,
	}
}

func observationBucket(buckets map[string]*observationAccumulator, key string) *observationAccumulator {
	bucket := buckets[key]
	if bucket == nil {
		bucket = newObservationAccumulator()
		buckets[key] = bucket
	}
	return bucket
}

func observationChannelBucket(buckets map[int]*observationAccumulator, key int) *observationAccumulator {
	bucket := buckets[key]
	if bucket == nil {
		bucket = newObservationAccumulator()
		buckets[key] = bucket
	}
	return bucket
}

func finalizeObservationBuckets(buckets map[string]*observationAccumulator) map[string]ModelExecutionObservationStat {
	out := make(map[string]ModelExecutionObservationStat, len(buckets))
	for key, bucket := range buckets {
		out[key] = bucket.summary()
	}
	return out
}

func finalizeObservationChannelBuckets(buckets map[int]*observationAccumulator) map[int]ModelExecutionObservationStat {
	out := make(map[int]ModelExecutionObservationStat, len(buckets))
	for key, bucket := range buckets {
		out[key] = bucket.summary()
	}
	return out
}

func averageObservationScores(sums map[string]float64, counts map[string]int) map[string]float64 {
	out := make(map[string]float64, len(sums))
	for key, sum := range sums {
		out[key] = sum / float64(counts[key])
	}
	return out
}

func observationRatio(value int, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(value) / float64(total)
}

func observationAverage(sum int64, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}

func parseObservationScoreBreakdown(raw string) (map[string]float64, error) {
	if raw == "" {
		return nil, nil
	}
	var out map[string]float64
	if err := common.UnmarshalJsonStr(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func observationJSON(v any) (string, error) {
	data, err := common.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func mustObservationJSON(v any) string {
	data, err := observationJSON(v)
	if err != nil {
		panic(err)
	}
	return data
}
