package dynamicbilling

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestApplyUsesDynamicRatioWhenBaselineReady(t *testing.T) {
	cache := NewRatioCache()
	now := time.Now().Unix()
	cache.Store(map[string]RatioBaseline{
		cacheKey("gpt-test", "codex-plus"): {
			RequestedModel: "gpt-test",
			Group:          "codex-plus",
			Ratio:          0.42,
			SampleCount:    5,
			CalculatedAt:   now,
			WindowStart:    now - 60,
			WindowEnd:      now,
			ProfitRate:     0.2,
		},
	})

	snapshot := Apply(ApplyInput{
		RequestedModel:   "gpt-test",
		Group:            "codex-plus",
		StaticGroupRatio: 0.1,
		Mode:             scheduler_setting.BillingRatioModeDynamic,
		Now:              now,
		Provider:         cache,
		Setting: scheduler_setting.SchedulerSetting{
			DynamicBillingEnabled:       true,
			DynamicBillingProfitRate:    0.2,
			DynamicBillingMinSamples:    2,
			DynamicBillingMaxAgeSeconds: 300,
		},
	})

	require.True(t, snapshot.Applied)
	require.Equal(t, 0.42, snapshot.DynamicRatio)
	require.Equal(t, 0.1, snapshot.StaticGroupRatio)
	require.Empty(t, snapshot.FallbackReason)
}

func TestApplyUsesGroupScopedBaselineAcrossModels(t *testing.T) {
	cache := NewRatioCache()
	now := time.Now().Unix()
	cache.Store(map[string]RatioBaseline{
		cacheKey("gpt-reference", "codex-plus"): {
			RequestedModel: "gpt-reference",
			ReferenceModel: "gpt-reference",
			Group:          "codex-plus",
			Ratio:          0.42,
			SampleCount:    5,
			CalculatedAt:   now,
			WindowStart:    now - 60,
			WindowEnd:      now,
			ProfitRate:     0.2,
		},
	})

	snapshot := Apply(ApplyInput{
		RequestedModel:   "another-model",
		Group:            "codex-plus",
		StaticGroupRatio: 0.1,
		Mode:             scheduler_setting.BillingRatioModeDynamic,
		Now:              now,
		Provider:         cache,
		Setting: scheduler_setting.SchedulerSetting{
			DynamicBillingEnabled:       true,
			DynamicBillingProfitRate:    0.2,
			DynamicBillingMinSamples:    2,
			DynamicBillingMaxAgeSeconds: 300,
		},
	})

	require.True(t, snapshot.Applied)
	require.Equal(t, 0.42, snapshot.DynamicRatio)
	require.Equal(t, 0.1, snapshot.StaticGroupRatio)
}

func TestApplyFallsBackWhenStaticMode(t *testing.T) {
	cache := NewRatioCache()
	cache.Store(map[string]RatioBaseline{
		cacheKey("gpt-test", "codex-plus"): {
			RequestedModel: "gpt-test",
			Group:          "codex-plus",
			Ratio:          0.42,
			SampleCount:    5,
			CalculatedAt:   time.Now().Unix(),
		},
	})

	snapshot := Apply(ApplyInput{
		RequestedModel:   "gpt-test",
		Group:            "codex-plus",
		StaticGroupRatio: 0.1,
		Mode:             scheduler_setting.BillingRatioModeStatic,
		Provider:         cache,
		Setting: scheduler_setting.SchedulerSetting{
			DynamicBillingEnabled:    true,
			DynamicBillingMinSamples: 1,
		},
	})

	require.False(t, snapshot.Applied)
	require.Equal(t, FallbackStaticMode, snapshot.FallbackReason)
}

func TestRatioCacheSnapshotsReturnsCopy(t *testing.T) {
	cache := NewRatioCache()
	cache.Store(map[string]RatioBaseline{
		cacheKey("gpt-test", "codex-plus"): {
			RequestedModel: "gpt-test",
			Group:          "codex-plus",
			Ratio:          0.42,
			SampleCount:    5,
		},
	})

	snapshots := cache.Snapshots()
	require.Len(t, snapshots, 1)
	snapshots[0].Ratio = 9

	baseline, ok := cache.Lookup("gpt-test", "codex-plus")
	require.True(t, ok)
	require.Equal(t, 0.42, baseline.Ratio)
}

func TestBuildRatioBaselinesUsesActualBilledBaseBeforeGroup(t *testing.T) {
	db := newDynamicBillingTestDB(t)
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-test":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})

	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         "req-dynamic-base",
		UpstreamCostTotal: 0.0006,
		CalculatedAt:      now,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Type:             model.LogTypeConsume,
		RequestId:        "req-dynamic-base",
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		PromptTokens:     100,
		CompletionTokens: 100,
		Quota:            100,
		Other:            `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4}`,
	}).Error)

	baselines, err := BuildRatioBaselines(db, db, scheduler_setting.SchedulerSetting{
		DynamicBillingWindowSamples: 300,
		DynamicBillingProfitRate:    0,
	}, now)

	require.NoError(t, err)
	baseline, ok := baselines[cacheKey("gpt-test", "codex-plus")]
	require.True(t, ok)
	require.InEpsilon(t, 0.3, baseline.Ratio, 0.000001)
	require.InEpsilon(t, 1.2, baseline.PricePerM, 0.000001)
	require.Equal(t, "gpt-test", baseline.ReferenceModel)
}

func TestBuildRatioBaselinesUsesLatestSampleWindow(t *testing.T) {
	db := newDynamicBillingTestDB(t)
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-test":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})

	now := time.Now().Unix()
	writeSample := func(requestID string, calculatedAt int64, cost float64, promptTokens int) {
		require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
			RequestId:         requestID,
			UpstreamCostTotal: cost,
			CalculatedAt:      calculatedAt,
		}).Error)
		require.NoError(t, db.Create(&model.Log{
			Type:             model.LogTypeConsume,
			CreatedAt:        calculatedAt,
			RequestId:        requestID,
			ModelName:        "gpt-test",
			Group:            "codex-plus",
			PromptTokens:     promptTokens,
			CompletionTokens: 100,
			Quota:            100,
			Other:            `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4}`,
		}).Error)
	}

	writeSample("req-old", now-300, 0.0004, 100)
	writeSample("req-new", now-10, 0.0008, 300)

	baselines, err := BuildRatioBaselines(db, db, scheduler_setting.SchedulerSetting{
		DynamicBillingWindowSamples: 1,
		DynamicBillingProfitRate:    0,
	}, now)

	require.NoError(t, err)
	baseline, ok := baselines[cacheKey("gpt-test", "codex-plus")]
	require.True(t, ok)
	require.Equal(t, 1, baseline.SampleCount)
	require.InEpsilon(t, 0.4, baseline.Ratio, 0.000001)
	require.Equal(t, now-10, baseline.WindowStart)
}

func TestBuildRatioBaselinesSkipsInvalidRowsUntilTargetValidSamples(t *testing.T) {
	db := newDynamicBillingTestDB(t)
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-test":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})

	now := time.Now().Unix()
	writeSample := func(requestID string, calculatedAt int64, cost float64, promptTokens int, other string) {
		require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
			RequestId:         requestID,
			UpstreamCostTotal: cost,
			CalculatedAt:      calculatedAt,
		}).Error)
		require.NoError(t, db.Create(&model.Log{
			Type:             model.LogTypeConsume,
			CreatedAt:        calculatedAt,
			RequestId:        requestID,
			ModelName:        "gpt-test",
			Group:            "codex-plus",
			PromptTokens:     promptTokens,
			CompletionTokens: 100,
			Quota:            100,
			Other:            other,
		}).Error)
	}

	writeSample("req-probe", now-5, 0.0002, 120, `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4,"is_health_probe":true}`)
	writeSample("req-empty", now-4, 0.0002, 120, `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4,"empty_output":true}`)
	writeSample("req-valid", now-20, 0.0008, 300, `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4}`)

	baselines, err := BuildRatioBaselines(db, db, scheduler_setting.SchedulerSetting{
		DynamicBillingWindowSamples: 1,
		DynamicBillingProfitRate:    0,
	}, now)

	require.NoError(t, err)
	baseline, ok := baselines[cacheKey("gpt-test", "codex-plus")]
	require.True(t, ok)
	require.Equal(t, 1, baseline.SampleCount)
	require.InEpsilon(t, 0.4, baseline.Ratio, 0.000001)
	require.Equal(t, now-20, baseline.WindowStart)
}

func TestBuildRatioBaselinesProfit24hUsesTokenCostMarkupAndBaseQuota(t *testing.T) {
	db := newDynamicBillingTestDB(t)
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-test":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})

	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         "req-profit-24h",
		UpstreamCostTotal: 0.0006,
		BreakdownJSON:     `{"token_multiplier":0.03}`,
		CalculatedAt:      now,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Type:             model.LogTypeConsume,
		CreatedAt:        now,
		RequestId:        "req-profit-24h",
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		PromptTokens:     100,
		CompletionTokens: 100,
		Quota:            100,
		Other:            `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4}`,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:      "req-profit-24h",
		CreatedAt:      now,
		UpdatedAt:      now,
		CompletedAt:    now,
		RequestedModel: "gpt-test",
		SelectedGroup:  "codex-plus",
		Success:        true,
		TotalTokens:    200,
		Quota:          100,
	}).Error)

	baselines, err := BuildRatioBaselines(db, db, scheduler_setting.SchedulerSetting{
		DynamicBillingCostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
		DynamicBillingApplyMode:          scheduler_setting.DynamicBillingApplyModeAuto,
		DynamicBillingProfitRate:         0.2,
		DynamicBillingProfitWindowHours:  24,
		DynamicBillingMinTokens:          1,
		DynamicBillingMinRequests:        1,
		DynamicBillingMinSuccessRequests: 1,
		DynamicBillingMinRatio:           0.01,
		DynamicBillingMaxRatio:           2,
		DynamicBillingMaxStepChange:      10,
	}, now)

	require.NoError(t, err)
	baseline, ok := baselines[cacheKey("gpt-test", "codex-plus")]
	require.True(t, ok)
	require.Equal(t, scheduler_setting.DynamicBillingCostSourceProfit24h, baseline.CostSource)
	require.Equal(t, int64(1), baseline.RequestCount)
	require.Equal(t, int64(1), baseline.SuccessRequestCount)
	require.InEpsilon(t, 0.03, baseline.CostMultiplier, 0.000001)
	require.InEpsilon(t, 10000, baseline.BaseQuotaAtRatio1, 0.000001)
	require.InEpsilon(t, 0.0006, baseline.OperatingCostUSD, 0.000001)
	require.InEpsilon(t, 0.00072, baseline.RequiredRevenueUSD, 0.000001)
	require.InEpsilon(t, 0.036, baseline.TargetRatio, 0.000001)
	require.InEpsilon(t, 0.036, baseline.Ratio, 0.000001)
	require.Empty(t, baseline.FallbackReason)
}

func TestBuildRatioBaselinesUsesRequestCostUsageWeightedMultiplier(t *testing.T) {
	db := newDynamicBillingTestDB(t)
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-test":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})

	now := time.Now().Unix()
	writeSample := func(requestID string, cost float64, multiplier float64, quota int) {
		require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
			RequestId:         requestID,
			UpstreamCostTotal: cost,
			BreakdownJSON:     fmt.Sprintf(`{"token_multiplier":%f}`, multiplier),
			CalculatedAt:      now,
		}).Error)
		require.NoError(t, db.Create(&model.Log{
			Type:             model.LogTypeConsume,
			CreatedAt:        now,
			RequestId:        requestID,
			ModelName:        "gpt-test",
			Group:            "codex-plus",
			PromptTokens:     100,
			CompletionTokens: 100,
			Quota:            quota,
			Other:            `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4}`,
		}).Error)
	}

	writeSample("req-big-cheap", 0.003, 0.03, 100)
	writeSample("req-small-expensive", 0.00001, 0.1, 100)

	baselines, err := BuildRatioBaselines(db, db, scheduler_setting.SchedulerSetting{
		DynamicBillingWindowSamples: 300,
		DynamicBillingProfitRate:    0.01,
	}, now)

	require.NoError(t, err)
	baseline, ok := baselines[cacheKey("gpt-test", "codex-plus")]
	require.True(t, ok)
	expectedBaseQuota := 0.003/0.03*common.QuotaPerUnit + 0.00001/0.1*common.QuotaPerUnit
	expectedCostMultiplier := (0.03*(0.003/0.03*common.QuotaPerUnit) + 0.1*(0.00001/0.1*common.QuotaPerUnit)) / expectedBaseQuota
	require.InEpsilon(t, expectedBaseQuota, baseline.BaseQuotaAtRatio1, 0.000001)
	require.InEpsilon(t, expectedCostMultiplier, baseline.CostMultiplier, 0.000001)
	require.InEpsilon(t, expectedCostMultiplier*1.01, baseline.Ratio, 0.000001)
	require.Greater(t, math.Abs(baseline.Ratio-(0.03+0.1)/2*1.01), 0.01)
}

func TestBuildRatioBaselinesProfit24hUsesEstimatedTrafficCost(t *testing.T) {
	db := newDynamicBillingTestDB(t)
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-test":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})
	setProfit24hMonitorConfigForTest(t, profit24hMonitorConfig{
		TrafficCostPerGBUSD:           1,
		TrafficEstimationEnabled:      true,
		TrafficEstimatedBytesPerToken: 1024,
		ResourceCostEnabled:           true,
	})

	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         "req-profit-traffic",
		UpstreamCostTotal: 0.0006,
		CalculatedAt:      now,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Type:             model.LogTypeConsume,
		CreatedAt:        now,
		RequestId:        "req-profit-traffic",
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		PromptTokens:     100,
		CompletionTokens: 100,
		Quota:            100,
		Other:            `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4}`,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:      "req-profit-traffic",
		CreatedAt:      now,
		UpdatedAt:      now,
		CompletedAt:    now,
		RequestedModel: "gpt-test",
		SelectedGroup:  "codex-plus",
		Success:        true,
		TotalTokens:    200,
		Quota:          100,
	}).Error)

	baselines, err := BuildRatioBaselines(db, db, scheduler_setting.SchedulerSetting{
		DynamicBillingCostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
		DynamicBillingApplyMode:          scheduler_setting.DynamicBillingApplyModeAuto,
		DynamicBillingProfitRate:         0.2,
		DynamicBillingProfitWindowHours:  24,
		DynamicBillingMinTokens:          1,
		DynamicBillingMinRequests:        1,
		DynamicBillingMinSuccessRequests: 1,
		DynamicBillingMinRatio:           0.01,
		DynamicBillingMaxRatio:           2,
		DynamicBillingMaxStepChange:      10,
	}, now)

	require.NoError(t, err)
	baseline, ok := baselines[cacheKey("gpt-test", "codex-plus")]
	require.True(t, ok)
	require.True(t, baseline.TrafficDataReady)
	require.True(t, baseline.TrafficEstimated)
	require.InEpsilon(t, float64(200*1024)/1024/1024/1024, baseline.TrafficCostUSD, 0.000001)
	require.InEpsilon(t, 0.0006+baseline.TrafficCostUSD, baseline.OperatingCostUSD, 0.000001)
	require.Empty(t, baseline.FallbackReason)
}

func TestApplyProfit24hLegacyManualModeAutoApplies(t *testing.T) {
	cache := NewRatioCache()
	now := time.Now().Unix()
	cache.Store(map[string]RatioBaseline{
		cacheKey("gpt-test", "codex-plus"): {
			RequestedModel:       "gpt-test",
			Group:                "codex-plus",
			Ratio:                0.42,
			SampleCount:          5,
			RequestCount:         20,
			SuccessRequestCount:  5,
			TotalTokens:          1000,
			BaseQuotaAtRatio1:    1000,
			TrafficDataReady:     true,
			CalculatedAt:         now,
			CostSource:           scheduler_setting.DynamicBillingCostSourceProfit24h,
			ApplyMode:            scheduler_setting.DynamicBillingApplyModeManual,
			PendingManualConfirm: true,
			FallbackReason:       FallbackManualConfirm,
		},
	})

	snapshot := Apply(ApplyInput{
		RequestedModel:   "gpt-test",
		Group:            "codex-plus",
		StaticGroupRatio: 0.1,
		Mode:             scheduler_setting.BillingRatioModeDynamic,
		Now:              now,
		Provider:         cache,
		Setting: scheduler_setting.SchedulerSetting{
			DynamicBillingEnabled:            true,
			DynamicBillingCostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
			DynamicBillingApplyMode:          scheduler_setting.DynamicBillingApplyModeManual,
			DynamicBillingMinSamples:         1,
			DynamicBillingMaxAgeSeconds:      300,
			DynamicBillingMinRequests:        20,
			DynamicBillingMinSuccessRequests: 5,
			DynamicBillingMinTokens:          1000,
		},
	})

	require.True(t, snapshot.Applied)
	require.Empty(t, snapshot.FallbackReason)
	require.False(t, snapshot.PendingManualConfirm)
	require.Equal(t, ApplyReasonManualModeAutoApplied, snapshot.ApplyReason)
	require.Equal(t, scheduler_setting.DynamicBillingCostSourceProfit24h, snapshot.CostSource)
}

func TestApplyProfit24hManualModeAppliesAfterConfirmation(t *testing.T) {
	cache := NewRatioCache()
	now := time.Now().Unix()
	cache.Store(map[string]RatioBaseline{
		cacheKey("gpt-test", "codex-plus"): {
			RequestedModel:      "gpt-test",
			Group:               "codex-plus",
			Ratio:               0.42,
			SampleCount:         5,
			RequestCount:        20,
			SuccessRequestCount: 5,
			TotalTokens:         1000,
			BaseQuotaAtRatio1:   1000,
			TrafficDataReady:    true,
			CalculatedAt:        now,
			CostSource:          scheduler_setting.DynamicBillingCostSourceProfit24h,
			ApplyMode:           scheduler_setting.DynamicBillingApplyModeManual,
		},
	})

	snapshot := Apply(ApplyInput{
		RequestedModel:   "gpt-test",
		Group:            "codex-plus",
		StaticGroupRatio: 0.1,
		Mode:             scheduler_setting.BillingRatioModeDynamic,
		Now:              now,
		Provider:         cache,
		Setting: scheduler_setting.SchedulerSetting{
			DynamicBillingEnabled:            true,
			DynamicBillingCostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
			DynamicBillingApplyMode:          scheduler_setting.DynamicBillingApplyModeManual,
			DynamicBillingMinSamples:         1,
			DynamicBillingMaxAgeSeconds:      300,
			DynamicBillingMinRequests:        20,
			DynamicBillingMinSuccessRequests: 5,
			DynamicBillingMinTokens:          1000,
		},
	})

	require.True(t, snapshot.Applied)
	require.Empty(t, snapshot.FallbackReason)
	require.False(t, snapshot.PendingManualConfirm)
	require.Equal(t, ApplyReasonManualModeAutoApplied, snapshot.ApplyReason)
	require.Equal(t, 0.42, snapshot.DynamicRatio)
}

func TestBuildRatioBaselinesProfit24hManualAndLargeStepAutoApplyWithReason(t *testing.T) {
	db := newDynamicBillingTestDB(t)
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-test":2}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.ModelGatewayDynamicBillingBaseline{
		BillingGroup:       "codex-plus",
		ReferenceModel:     "gpt-test",
		Ratio:              0.01,
		ReferencePricePerM: 0.04,
		SampleCount:        5,
		CostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
		ApplyMode:          scheduler_setting.DynamicBillingApplyModeManual,
		CalculatedAt:       now - 60,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         "req-profit-confirm",
		UpstreamCostTotal: 0.0006,
		BreakdownJSON:     `{"token_multiplier":0.03}`,
		CalculatedAt:      now,
	}).Error)
	require.NoError(t, db.Create(&model.Log{
		Type:             model.LogTypeConsume,
		CreatedAt:        now,
		RequestId:        "req-profit-confirm",
		ModelName:        "gpt-test",
		Group:            "codex-plus",
		PromptTokens:     100,
		CompletionTokens: 100,
		Quota:            100,
		Other:            `{"group_ratio":0.1,"model_ratio":2,"completion_ratio":4}`,
	}).Error)
	require.NoError(t, db.Create(&model.ChannelAccountUsageEvent{
		RequestId:      "req-profit-confirm",
		CreatedAt:      now,
		UpdatedAt:      now,
		CompletedAt:    now,
		RequestedModel: "gpt-test",
		SelectedGroup:  "codex-plus",
		Success:        true,
		TotalTokens:    200,
		Quota:          100,
	}).Error)
	setting := scheduler_setting.SchedulerSetting{
		DynamicBillingCostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
		DynamicBillingApplyMode:          scheduler_setting.DynamicBillingApplyModeManual,
		DynamicBillingProfitRate:         0.2,
		DynamicBillingProfitWindowHours:  24,
		DynamicBillingMinTokens:          1,
		DynamicBillingMinRequests:        1,
		DynamicBillingMinSuccessRequests: 1,
		DynamicBillingMinRatio:           0.01,
		DynamicBillingMaxRatio:           2,
		DynamicBillingMaxStepChange:      0.3,
	}

	baselines, err := BuildRatioBaselines(db, db, setting, now)
	require.NoError(t, err)
	baseline, ok := baselines[cacheKey("gpt-test", "codex-plus")]
	require.True(t, ok)
	require.False(t, baseline.PendingManualConfirm)
	require.Empty(t, baseline.FallbackReason)
	require.Equal(t, ApplyReasonStepChangeAutoApplied, baseline.ApplyReason)
	require.InEpsilon(t, baseline.EffectiveRatio, baseline.Ratio, 0.000001)
}

func TestApplyProfit24hInsufficientUsageDoesNotApply(t *testing.T) {
	cache := NewRatioCache()
	now := time.Now().Unix()
	cache.Store(map[string]RatioBaseline{
		cacheKey("gpt-test", "codex-plus"): {
			RequestedModel:      "gpt-test",
			Group:               "codex-plus",
			Ratio:               0.42,
			SampleCount:         5,
			RequestCount:        3,
			SuccessRequestCount: 2,
			TotalTokens:         900,
			BaseQuotaAtRatio1:   1000,
			TrafficDataReady:    true,
			CalculatedAt:        now,
			CostSource:          scheduler_setting.DynamicBillingCostSourceProfit24h,
			ApplyMode:           scheduler_setting.DynamicBillingApplyModeAuto,
		},
	})

	snapshot := Apply(ApplyInput{
		RequestedModel:   "gpt-test",
		Group:            "codex-plus",
		StaticGroupRatio: 0.1,
		Mode:             scheduler_setting.BillingRatioModeDynamic,
		Now:              now,
		Provider:         cache,
		Setting: scheduler_setting.SchedulerSetting{
			DynamicBillingEnabled:            true,
			DynamicBillingCostSource:         scheduler_setting.DynamicBillingCostSourceProfit24h,
			DynamicBillingApplyMode:          scheduler_setting.DynamicBillingApplyModeAuto,
			DynamicBillingMinSamples:         1,
			DynamicBillingMaxAgeSeconds:      300,
			DynamicBillingMinRequests:        20,
			DynamicBillingMinSuccessRequests: 5,
			DynamicBillingMinTokens:          1000,
		},
	})

	require.False(t, snapshot.Applied)
	require.Equal(t, FallbackInsufficientUsage, snapshot.FallbackReason)
	require.Equal(t, 0.1, snapshot.StaticGroupRatio)
	require.Equal(t, 0.42, snapshot.DynamicRatio)
}

func TestPersistBaselinesPreservesProfit24hFields(t *testing.T) {
	db := newDynamicBillingTestDB(t)
	now := time.Now().Unix()
	value := RatioBaseline{
		RequestedModel:       "gpt-test",
		ReferenceModel:       "gpt-test",
		Group:                "codex-plus",
		Ratio:                0.42,
		PricePerM:            1.68,
		SampleCount:          5,
		ModelCount:           1,
		CalculatedAt:         now,
		WindowStart:          now - 86400,
		WindowEnd:            now,
		ProfitRate:           0.2,
		CostSource:           scheduler_setting.DynamicBillingCostSourceProfit24h,
		ApplyMode:            scheduler_setting.DynamicBillingApplyModeManual,
		ApplyReason:          ApplyReasonManualModeAutoApplied,
		OperatingCostUSD:     0.001,
		RequiredRevenueUSD:   0.00125,
		BaseQuotaAtRatio1:    1500,
		CostMultiplier:       0.031,
		TargetRatio:          0.5,
		EffectiveRatio:       0.42,
		Clamped:              true,
		PendingManualConfirm: true,
		FallbackReason:       FallbackManualConfirm,
		RequestCount:         20,
		SuccessRequestCount:  5,
		TotalTokens:          1000,
		TrafficCostUSD:       0.0001,
		TrafficEstimated:     true,
		TrafficDataReady:     true,
		ServerCostUSD:        0.0002,
		ResourceCostUSD:      0.0003,
		UpstreamCostUSD:      0.0004,
	}
	require.NoError(t, persistBaselines(db, map[string]RatioBaseline{cacheKey("gpt-test", "codex-plus"): value}))

	loaded := loadPersistedBaselines(db)
	baseline, ok := loaded[cacheKey("gpt-test", "codex-plus")]
	require.True(t, ok)
	require.Equal(t, scheduler_setting.DynamicBillingCostSourceProfit24h, baseline.CostSource)
	require.Equal(t, scheduler_setting.DynamicBillingApplyModeManual, baseline.ApplyMode)
	require.Equal(t, ApplyReasonManualModeAutoApplied, baseline.ApplyReason)
	require.Equal(t, FallbackManualConfirm, baseline.FallbackReason)
	require.True(t, baseline.PendingManualConfirm)
	require.True(t, baseline.Clamped)
	require.InEpsilon(t, 0.00125, baseline.RequiredRevenueUSD, 0.000001)
	require.InEpsilon(t, 0.031, baseline.CostMultiplier, 0.000001)
	require.Equal(t, int64(20), baseline.RequestCount)
	require.True(t, baseline.TrafficEstimated)
	require.True(t, baseline.TrafficDataReady)
}

func newDynamicBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Log{},
		&model.ModelGatewayRequestCostSummary{},
		&model.ChannelAccountUsageEvent{},
		&model.ModelGatewayDynamicBillingBaseline{},
		&model.ModelGatewayProfitResourceCost{},
		&model.ModelGatewayTrafficMetric{},
		&model.Option{},
	))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func replaceOptionMapForDynamicBillingTest() func() {
	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	return func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	}
}

func setProfit24hMonitorConfigForTest(t *testing.T, config profit24hMonitorConfig) {
	t.Helper()
	payload, err := common.Marshal(config)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	oldOptionMap := common.OptionMap
	nextOptionMap := make(map[string]string, len(oldOptionMap)+1)
	for key, value := range oldOptionMap {
		nextOptionMap[key] = value
	}
	nextOptionMap[profitMonitorConfigOptionKey] = string(payload)
	common.OptionMap = nextOptionMap
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = oldOptionMap
		common.OptionMapRWMutex.Unlock()
	})
}
