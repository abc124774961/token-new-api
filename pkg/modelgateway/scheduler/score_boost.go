package scheduler

import (
	"math"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

type ScoreBoostConfig struct {
	SmartScoreBoosts map[string]float64 `json:"smart_score_boosts,omitempty"`
}

type ScoreBoostItem struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

type ScoreBoostService struct{}

var allowedScoreBoostItems = []ScoreBoostItem{
	{Key: scoreItemCompletionRate, Name: "完成率分", Category: scoreCategorySample},
	{Key: scoreItemUpstreamErrorRate, Name: "上游错误率分", Category: scoreCategorySample},
	{Key: scoreItemTTFTLatency, Name: "首包速度分", Category: scoreCategorySample},
	{Key: scoreItemDurationLatency, Name: "完整耗时分", Category: scoreCategorySample},
	{Key: scoreItemThroughput, Name: "吞吐速度分", Category: scoreCategorySample},
	{Key: scoreItemEmptyOutputRate, Name: "空输出率分", Category: scoreCategorySample},
	{Key: scoreItemStreamInterruptedRate, Name: "流中断率分", Category: scoreCategorySample},
	{Key: scoreItemConcurrencyLoad, Name: "并发负载分", Category: scoreCategoryPressure},
	{Key: scoreItemQueuePressure, Name: "队列压力分", Category: scoreCategoryPressure},
	{Key: scoreItemFirstByteBacklog, Name: "首包积压分", Category: scoreCategoryPressure},
	{Key: scoreItemCost, Name: "成本分", Category: scoreCategoryFormula},
}

var allowedScoreBoostItemSet = func() map[string]struct{} {
	out := make(map[string]struct{}, len(allowedScoreBoostItems))
	for _, item := range allowedScoreBoostItems {
		out[item.Key] = struct{}{}
	}
	return out
}()

func NewScoreBoostService() *ScoreBoostService {
	return &ScoreBoostService{}
}

func AllowedScoreBoostItems() []ScoreBoostItem {
	out := make([]ScoreBoostItem, len(allowedScoreBoostItems))
	copy(out, allowedScoreBoostItems)
	return out
}

func IsAllowedScoreBoostItem(key string) bool {
	_, ok := allowedScoreBoostItemSet[strings.TrimSpace(key)]
	return ok
}

func (s *ScoreBoostService) Normalize(boosts map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(boosts))
	for key, value := range boosts {
		key = strings.TrimSpace(key)
		if key == "" || !IsAllowedScoreBoostItem(key) {
			continue
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			continue
		}
		out[key] = round4(clamp01(value))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *ScoreBoostService) BoostsForChannel(channel *model.Channel) map[string]float64 {
	if channel == nil {
		return nil
	}
	return s.Normalize(channel.GetOtherSettings().SmartScoreBoosts)
}

func (s *ScoreBoostService) ApplyScoreBoosts(items []core.ScoreItem, boosts map[string]float64) []core.ScoreItem {
	boosts = s.Normalize(boosts)
	if len(items) == 0 || len(boosts) == 0 {
		return items
	}
	for idx := range items {
		item := &items[idx]
		boost := boosts[item.Key]
		if boost <= 0 || item.MissingReason != "" {
			continue
		}
		baseScore := round4(item.Score)
		item.BaseScore = &baseScore
		item.ScoreBoost = boost
		item.ScoreAdjusted = true
		item.Score = round4(clamp01(item.Score + boost))
		item.WeightedScore = round4(item.Score * item.Weight)
	}
	return items
}
