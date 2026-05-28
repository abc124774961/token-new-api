package cost

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCalculatePreciseBreakdownIgnoresUserBillingRatios(t *testing.T) {
	profile := &model.ModelGatewayChannelCostProfile{
		ChannelID:            7,
		UpstreamModel:        "gpt-5.5",
		InputPerMillion:      2,
		OutputPerMillion:     10,
		CacheReadPerMillion:  0.2,
		CacheWritePerMillion: 3,
		AudioInputPerMillion: 4,
		Source:               SourceManual,
	}
	usage := UsageSnapshot{
		RequestID:          "req-cost",
		ChannelID:          7,
		UpstreamModel:      "gpt-5.5",
		PromptTokens:       1000,
		CompletionTokens:   200,
		CacheReadTokens:    300,
		CacheWriteTokens:   100,
		AudioInputTokens:   50,
		UsageSemantic:      "openai",
		WebSearchCallCount: 1,
	}
	profile.ToolPricesJSON = `{"web_search":0.01}`

	result := Calculate(usage, profile)

	require.Equal(t, SourceManual, result.Source)
	require.Equal(t, AccuracyPrecise, result.Accuracy)
	require.Equal(t, 550, result.Breakdown.BaseInputTokens)
	require.Equal(t, 200, result.Breakdown.BaseOutputTokens)
	require.InEpsilon(t, 0.0011, result.Breakdown.Input.Amount, 0.000001)
	require.InEpsilon(t, 0.002, result.Breakdown.Output.Amount, 0.000001)
	require.InEpsilon(t, 0.00006, result.Breakdown.CacheRead.Amount, 0.000001)
	require.InEpsilon(t, 0.0003, result.Breakdown.CacheWrite.Amount, 0.000001)
	require.InEpsilon(t, 0.0002, result.Breakdown.AudioInput.Amount, 0.000001)
	require.InEpsilon(t, 0.01, result.Breakdown.Tools["web_search"].Amount, 0.000001)
	require.InEpsilon(t, 0.01366, result.Total, 0.000001)
}

func TestCalculateDoesNotSubtractClaudeCacheFromBaseInput(t *testing.T) {
	profile := &model.ModelGatewayChannelCostProfile{
		ChannelID:              8,
		UpstreamModel:          "claude-sonnet",
		InputPerMillion:        3,
		OutputPerMillion:       15,
		CacheReadPerMillion:    0.3,
		CacheWrite5mPerMillion: 3.75,
		CacheWrite1hPerMillion: 6,
	}
	usage := UsageSnapshot{
		ChannelID:          8,
		UpstreamModel:      "claude-sonnet",
		PromptTokens:       1000,
		CompletionTokens:   100,
		CacheReadTokens:    500,
		CacheWriteTokens5m: 100,
		CacheWriteTokens1h: 50,
		UsageSemantic:      "anthropic",
	}

	result := Calculate(usage, profile)

	require.Equal(t, 1000, result.Breakdown.BaseInputTokens)
	require.InEpsilon(t, 0.003, result.Breakdown.Input.Amount, 0.000001)
	require.InEpsilon(t, 0.00015, result.Breakdown.CacheRead.Amount, 0.000001)
	require.InEpsilon(t, 0.000375, result.Breakdown.CacheWrite5m.Amount, 0.000001)
	require.InEpsilon(t, 0.0003, result.Breakdown.CacheWrite1h.Amount, 0.000001)
}

func TestCalculateMissingWhenNoProfile(t *testing.T) {
	usage := UsageSnapshot{
		PromptTokens:     100,
		CompletionTokens: 50,
	}

	missing := Calculate(usage, nil)
	require.Equal(t, SourceMissing, missing.Source)
	require.Equal(t, AccuracyMissing, missing.Accuracy)
	require.Zero(t, missing.Total)
}

func TestCalculateDerivesDefaultSystemProfileWhenNoChannelRule(t *testing.T) {
	ratio_setting.InitRatioSettings()
	usage := UsageSnapshot{
		RequestID:        "req-default-system-ratio",
		ChannelID:        7,
		UpstreamModel:    "gpt-4o",
		PromptTokens:     1000,
		CompletionTokens: 100,
		CacheReadTokens:  200,
	}

	result := Calculate(usage, DefaultSystemRatioProfile(7))
	expected := Calculate(usage, &model.ModelGatewayChannelCostProfile{
		ChannelID:             7,
		UpstreamModel:         "*",
		Source:                SourceSystemRatio,
		TokenMultiplier:       1,
		InputCostMultiplier:   1,
		OutputCostMultiplier:  1,
		CacheReadMultiplier:   1,
		CacheWriteMultiplier:  1,
		RequestCostMultiplier: 1,
		RechargeMultiplier:    1,
	})

	require.Equal(t, SourceSystemRatio, result.Source)
	require.Equal(t, "estimated", result.Accuracy)
	require.Greater(t, result.Total, float64(0))
	require.InEpsilon(t, expected.Breakdown.Input.Amount, result.Breakdown.Input.Amount, 0.000001)
	require.InEpsilon(t, expected.Breakdown.Output.Amount, result.Breakdown.Output.Amount, 0.000001)
	require.InEpsilon(t, expected.Breakdown.CacheRead.Amount, result.Breakdown.CacheRead.Amount, 0.000001)
	require.InEpsilon(t, expected.Total, result.Total, 0.000001)
}

func TestCalculateDerivesSystemRatioProfileByActualModel(t *testing.T) {
	ratio_setting.InitRatioSettings()
	usage := UsageSnapshot{
		RequestID:        "req-system-ratio",
		ChannelID:        7,
		UpstreamModel:    "gpt-4o",
		PromptTokens:     1000,
		CompletionTokens: 100,
		CacheReadTokens:  200,
	}
	profile := &model.ModelGatewayChannelCostProfile{
		ChannelID:          7,
		UpstreamModel:      "*",
		Source:             SourceSystemRatio,
		CostCoefficient:    0.5,
		TokenMultiplier:    0.05,
		RechargeMultiplier: 0.8,
	}

	result := Calculate(usage, profile)

	require.Equal(t, SourceSystemRatio, result.Source)
	require.Equal(t, "estimated", result.Accuracy)
	require.InEpsilon(t, 0.5, result.Breakdown.CostCoefficient, 0.000001)
	require.InEpsilon(t, 0.05, result.Breakdown.FeeMultiplier, 0.000001)
	require.InEpsilon(t, 0.03125, result.Breakdown.TokenMultiplier, 0.000001)
	require.InEpsilon(t, 0.0000625, result.Breakdown.Input.Amount, 0.000001)
	require.InEpsilon(t, 0.00003125, result.Breakdown.Output.Amount, 0.000001)
	require.InEpsilon(t, 0.0000078125, result.Breakdown.CacheRead.Amount, 0.000001)
	require.InEpsilon(t, 0.0001015625, result.Total, 0.000001)
}

func TestCalculateDerivesToiotoCostFromSystemModelPricing(t *testing.T) {
	ratio_setting.InitRatioSettings()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-5.4":1.25}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-5.4":6}`))
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"gpt-5.4":0.1}`))
	require.NoError(t, ratio_setting.UpdateCreateCacheRatioByJSONString(`{}`))
	t.Cleanup(ratio_setting.InitRatioSettings)

	profile := model.ModelGatewayChannelCostProfile{
		ChannelID:             4,
		UpstreamModel:         "*",
		Source:                SourceSystemRatio,
		CostCoefficient:       0.05,
		TokenMultiplier:       4.1,
		InputCostMultiplier:   4.1,
		OutputCostMultiplier:  4.1,
		CacheReadMultiplier:   4.1,
		CacheWriteMultiplier:  4.1,
		RequestCostMultiplier: 0.2,
		RechargeMultiplier:    2,
	}

	quote := QuoteSystemRatioProfile("gpt-5.4", profile)
	require.Equal(t, "model_ratio", quote.PriceSource)
	require.InEpsilon(t, 0.05, quote.CostCoefficient, 0.000001)
	require.InEpsilon(t, 4.1, quote.FeeMultiplier, 0.000001)
	require.InEpsilon(t, 0.1025, quote.ActualTokenMultiplier, 0.000001)
	require.InEpsilon(t, 0.125, quote.BaseInputPerMillion, 0.000001)
	require.InEpsilon(t, 0.25625, quote.InputPerMillion, 0.000001)
	require.InEpsilon(t, 1.5375, quote.OutputPerMillion, 0.000001)
	require.InEpsilon(t, 0.025625, quote.CacheReadPerMillion, 0.000001)
	require.Zero(t, quote.CacheWritePerMillion)

	result := Calculate(UsageSnapshot{
		RequestID:        "req-toioto-cost",
		ChannelID:        4,
		UpstreamModel:    "gpt-5.4",
		PromptTokens:     2_000_000,
		CompletionTokens: 1_000_000,
		CacheReadTokens:  1_000_000,
	}, &profile)

	require.Equal(t, SourceSystemRatio, result.Source)
	require.Equal(t, "estimated", result.Accuracy)
	require.Equal(t, 1_000_000, result.Breakdown.BaseInputTokens)
	require.InEpsilon(t, 0.05, result.Breakdown.CostCoefficient, 0.000001)
	require.InEpsilon(t, 4.1, result.Breakdown.FeeMultiplier, 0.000001)
	require.InEpsilon(t, 0.1025, result.Breakdown.TokenMultiplier, 0.000001)
	require.InEpsilon(t, 0.25625, result.Breakdown.Input.Amount, 0.000001)
	require.InEpsilon(t, 0.025625, result.Breakdown.CacheRead.Amount, 0.000001)
	require.InEpsilon(t, 1.5375, result.Breakdown.Output.Amount, 0.000001)
	require.InEpsilon(t, 1.819375, result.Total, 0.000001)
}

func TestCostRatioFromSystemDefaultProfileUsesActualModel(t *testing.T) {
	ratio_setting.InitRatioSettings()
	profile := &model.ModelGatewayChannelCostProfile{
		ChannelID:          7,
		UpstreamModel:      "*",
		Source:             SourceSystemRatio,
		TokenMultiplier:    0.05,
		RechargeMultiplier: 0.8,
	}

	ratio, ok := CostRatioFromProfileForModel(profile, "gpt-4o")

	require.True(t, ok)
	expected := blendedTokenReferenceCost(DeriveSystemRatioProfile("gpt-4o", *profile))
	require.InEpsilon(t, expected, ratio, 0.000001)
}

func TestCostRatioFromProfileUsesBlendedTokenReferenceCost(t *testing.T) {
	profile := &model.ModelGatewayChannelCostProfile{
		ChannelID:             8,
		UpstreamModel:         "gpt-5.5",
		Source:                SourceManual,
		InputPerMillion:       0.20,
		OutputPerMillion:      1.20,
		CacheReadPerMillion:   0.02,
		CacheWritePerMillion:  0.25,
		ImageInputPerMillion:  0.30,
		AudioInputPerMillion:  0.40,
		AudioOutputPerMillion: 1.60,
		RequestPrice:          10,
	}

	ratio, ok := CostRatioFromProfileForModel(profile, "gpt-5.5")

	require.True(t, ok)
	require.InEpsilon(t, 0.6515, ratio, 0.0001)
	require.Equal(t, "token", CostPricingModeFromProfileForModel(profile, "gpt-5.5"))
}

func TestCostRatioFromProfileFallsBackToRequestCost(t *testing.T) {
	profile := &model.ModelGatewayChannelCostProfile{
		ChannelID:     8,
		UpstreamModel: "image-model",
		Source:        SourceManual,
		RequestPrice:  0.03,
	}

	ratio, ok := CostRatioFromProfileForModel(profile, "image-model")

	require.True(t, ok)
	require.InEpsilon(t, 0.03, ratio, 0.000001)
	require.Equal(t, "request", CostPricingModeFromProfileForModel(profile, "image-model"))
}

func TestCalculateDerivesConfiguredRequestPriceWithoutCostMultiplier(t *testing.T) {
	ratio_setting.InitRatioSettings()
	usage := UsageSnapshot{
		RequestID:     "req-fixed-price",
		ChannelID:     7,
		UpstreamModel: "dall-e-3",
		PromptTokens:  1000,
	}
	profile := &model.ModelGatewayChannelCostProfile{
		ChannelID:             7,
		UpstreamModel:         "*",
		Source:                SourceSystemRatio,
		TokenMultiplier:       0.01,
		RequestPrice:          0.003,
		RequestCostMultiplier: 100,
		RechargeMultiplier:    2,
	}

	result := Calculate(usage, profile)

	require.Equal(t, SourceSystemRatio, result.Source)
	require.Equal(t, "estimated", result.Accuracy)
	require.Zero(t, result.Breakdown.Input.Amount)
	require.Zero(t, result.Breakdown.TokenMultiplier)
	require.InEpsilon(t, 0.0015, result.Breakdown.Request.Amount, 0.000001)
	require.InEpsilon(t, 0.0015, result.Total, 0.000001)
}

func TestCalculateFixedPriceFallsBackToSystemRequestPrice(t *testing.T) {
	ratio_setting.InitRatioSettings()
	usage := UsageSnapshot{
		RequestID:     "req-fixed-price-fallback",
		ChannelID:     7,
		UpstreamModel: "dall-e-3",
		PromptTokens:  1000,
	}
	profile := &model.ModelGatewayChannelCostProfile{
		ChannelID:          7,
		UpstreamModel:      "*",
		Source:             SourceSystemRatio,
		TokenMultiplier:    0.01,
		RechargeMultiplier: 2,
	}

	result := Calculate(usage, profile)

	require.Equal(t, SourceSystemRatio, result.Source)
	require.Equal(t, "estimated", result.Accuracy)
	require.Zero(t, result.Breakdown.Input.Amount)
	require.InEpsilon(t, 0.02, result.Breakdown.Request.Amount, 0.000001)
	require.InEpsilon(t, 0.02, result.Total, 0.000001)
}

func TestUsageSnapshotFromLogIgnoresGroupRatioForUpstreamCost(t *testing.T) {
	other, err := common.Marshal(map[string]any{
		"group_ratio":             0.1,
		"model_ratio":             0.2,
		"completion_ratio":        3,
		"cache_tokens":            20,
		"cache_write_tokens":      5,
		"upstream_model_name":     "provider-model",
		"audio_input_token_count": 9,
		"provider_special_tokens": 7,
	})
	require.NoError(t, err)

	usage := UsageSnapshotFromLog(model.Log{
		RequestId:        "req-log",
		ChannelId:        9,
		ModelName:        "client-model",
		PromptTokens:     100,
		CompletionTokens: 40,
		Other:            string(other),
	})

	require.Equal(t, "provider-model", usage.UpstreamModel)
	require.Equal(t, 20, usage.CacheReadTokens)
	require.Equal(t, 5, usage.CacheWriteTokens)
	require.Equal(t, 9, usage.AudioInputTokens)
	require.Equal(t, float64(7), usage.UnrecognizedUsage["provider_special_tokens"])
}

func TestProfileCacheUsesDefaultRuleAndLatestEffectiveVersion(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelGatewayChannelCostProfile{}))

	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ModelGatewayChannelCostProfile{
		ChannelID:        7,
		UpstreamModel:    "*",
		InputPerMillion:  1,
		OutputPerMillion: 2,
		EffectiveTime:    now - 60,
		Version:          1,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayChannelCostProfile{
		ChannelID:        7,
		UpstreamModel:    "gpt-5.5",
		InputPerMillion:  3,
		OutputPerMillion: 5,
		EffectiveTime:    now - 30,
		Version:          1,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayChannelCostProfile{
		ChannelID:        7,
		UpstreamModel:    "gpt-5.5",
		InputPerMillion:  4,
		OutputPerMillion: 6,
		EffectiveTime:    now - 30,
		Version:          2,
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayChannelCostProfile{
		ChannelID:        7,
		UpstreamModel:    "gpt-5.5",
		InputPerMillion:  100,
		OutputPerMillion: 100,
		EffectiveTime:    now + 3600,
		Version:          3,
	}).Error)

	cache := &ProfileCache{}
	require.NoError(t, cache.Refresh(context.Background()))

	exact := cache.Lookup(7, "gpt-5.5")
	require.NotNil(t, exact)
	require.Equal(t, "*", exact.UpstreamModel)
	require.Equal(t, 1.0, exact.InputPerMillion)
	require.Equal(t, 1, exact.Version)

	defaultRule := cache.Lookup(7, "unconfigured-model")
	require.NotNil(t, defaultRule)
	require.Equal(t, "*", defaultRule.UpstreamModel)
	require.Equal(t, 1.0, defaultRule.InputPerMillion)
}

func TestProfileCacheStoreDoesNotSuppressFullRefresh(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelGatewayChannelCostProfile{}))

	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.ModelGatewayChannelCostProfile{
		ChannelID:       18,
		UpstreamModel:   "*",
		Source:          SourceSystemRatio,
		CostCoefficient: 0.0549,
		TokenMultiplier: 1,
		EffectiveTime:   now - 60,
		Version:         1,
	}).Error)

	cache := &ProfileCache{}
	cache.Store(model.ModelGatewayChannelCostProfile{
		ChannelID:       12,
		UpstreamModel:   "*",
		Source:          SourceSystemRatio,
		CostCoefficient: 0.08,
		TokenMultiplier: 1,
	})

	require.False(t, cache.loaded())
	require.Nil(t, cache.Lookup(18, "gpt-5.5"))
	require.NoError(t, cache.Refresh(context.Background()))

	profile := cache.Lookup(18, "gpt-5.5")
	require.NotNil(t, profile)
	require.InEpsilon(t, 0.0549, profile.CostCoefficient, 0.000001)
}

func TestWorkerLoadsPendingConsumeLogsIncrementally(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Log{},
		&model.ModelGatewayChannelCostProfile{},
		&model.ModelGatewayRequestCostSummary{},
	))

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&[]model.Log{
		{
			Type:      model.LogTypeConsume,
			RequestId: "req-existing",
		},
		{
			Type:      model.LogTypeConsume,
			RequestId: "req-bootstrap",
		},
	}).Error)
	require.NoError(t, db.Create(&model.ModelGatewayRequestCostSummary{
		RequestId:         "req-existing",
		UpstreamCostTotal: 0.01,
		CostSource:        SourceSystemRatio,
		CostAccuracy:      "estimated",
		CalculatedAt:      common.GetTimestamp(),
	}).Error)

	worker := NewWorker(WorkerConfig{Enabled: true, Batch: 10})
	pending, scannedThroughID, err := worker.loadPendingConsumeLogs(10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "req-bootstrap", pending[0].RequestId)
	require.Equal(t, 2, scannedThroughID)
	worker.advanceCursor(scannedThroughID)

	require.NoError(t, db.Create(&[]model.Log{
		{
			Type:      model.LogTypeConsume,
			RequestId: "req-next",
		},
		{
			Type:      model.LogTypeError,
			RequestId: "req-error",
		},
	}).Error)

	pending, scannedThroughID, err = worker.loadPendingConsumeLogs(10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "req-next", pending[0].RequestId)
	require.Equal(t, 3, scannedThroughID)
	worker.advanceCursor(scannedThroughID)

	pending, scannedThroughID, err = worker.loadPendingConsumeLogs(10)
	require.NoError(t, err)
	require.Empty(t, pending)
	require.Equal(t, 3, scannedThroughID)
}

func TestWorkerReprocessesLowQualityCostSummaries(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Log{},
		&model.ModelGatewayChannelCostProfile{},
		&model.ModelGatewayRequestCostSummary{},
	))

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&[]model.Log{
		{Type: model.LogTypeConsume, RequestId: "req-missing"},
		{Type: model.LogTypeConsume, RequestId: "req-manual-zero"},
		{Type: model.LogTypeConsume, RequestId: "req-good"},
	}).Error)
	require.NoError(t, db.Create(&[]model.ModelGatewayRequestCostSummary{
		{
			RequestId:    "req-missing",
			CostSource:   SourceMissing,
			CostAccuracy: AccuracyMissing,
		},
		{
			RequestId:         "req-manual-zero",
			UpstreamCostTotal: 0,
			CostSource:        SourceManual,
			CostAccuracy:      "estimated",
		},
		{
			RequestId:         "req-good",
			UpstreamCostTotal: 0.01,
			CostSource:        SourceSystemRatio,
			CostAccuracy:      "estimated",
		},
	}).Error)

	worker := NewWorker(WorkerConfig{Enabled: true, Batch: 10})
	pending, scannedThroughID, err := worker.loadPendingConsumeLogs(10)
	require.NoError(t, err)
	require.Equal(t, 3, scannedThroughID)
	require.Len(t, pending, 2)
	require.Equal(t, "req-missing", pending[0].RequestId)
	require.Equal(t, "req-manual-zero", pending[1].RequestId)
}

func TestWorkerReprocessesDefaultSystemRatioOneXCostSummaries(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Log{},
		&model.ModelGatewayChannelCostProfile{},
		&model.ModelGatewayRequestCostSummary{},
	))

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	staleBreakdown, err := common.Marshal(Breakdown{
		Currency:           "USD",
		CostCoefficient:    1,
		FeeMultiplier:      1,
		TokenMultiplier:    1,
		RechargeMultiplier: 1,
		Input:              BreakdownComponent{Tokens: 1000, PricePerMillion: 2, Amount: 0.002},
	})
	require.NoError(t, err)
	goodBreakdown, err := common.Marshal(Breakdown{
		Currency:           "USD",
		CostCoefficient:    0.0549,
		FeeMultiplier:      1,
		TokenMultiplier:    0.0549,
		RechargeMultiplier: 1,
		Input:              BreakdownComponent{Tokens: 1000, PricePerMillion: 0.1098, Amount: 0.0001098},
	})
	require.NoError(t, err)

	require.NoError(t, db.Create(&[]model.Log{
		{Type: model.LogTypeConsume, RequestId: "req-stale-default"},
		{Type: model.LogTypeConsume, RequestId: "req-good-cost"},
	}).Error)
	require.NoError(t, db.Create(&[]model.ModelGatewayRequestCostSummary{
		{
			RequestId:         "req-stale-default",
			UpstreamCostTotal: 0.002,
			BreakdownJSON:     string(staleBreakdown),
			CostSource:        SourceSystemRatio,
			CostAccuracy:      "estimated",
			CalculatedAt:      common.GetTimestamp(),
		},
		{
			RequestId:         "req-good-cost",
			UpstreamCostTotal: 0.0001098,
			BreakdownJSON:     string(goodBreakdown),
			CostSource:        SourceSystemRatio,
			CostAccuracy:      "estimated",
			CalculatedAt:      common.GetTimestamp(),
		},
	}).Error)

	worker := NewWorker(WorkerConfig{Enabled: true, Batch: 10})
	pending, scannedThroughID, err := worker.loadPendingConsumeLogs(10)
	require.NoError(t, err)
	require.Equal(t, 2, scannedThroughID)
	require.Len(t, pending, 1)
	require.Equal(t, "req-stale-default", pending[0].RequestId)
}
