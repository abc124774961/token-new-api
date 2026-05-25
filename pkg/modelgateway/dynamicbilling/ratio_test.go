package dynamicbilling

import (
	"fmt"
	"testing"
	"time"

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

func newDynamicBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.ModelGatewayRequestCostSummary{}))
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
