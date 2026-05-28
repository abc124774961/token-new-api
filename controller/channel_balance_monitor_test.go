package controller

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelBalanceMonitorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ChannelBalanceMonitorEvent{}, &model.Option{}))

	oldDB := model.DB
	oldOptionMap := common.OptionMap
	model.DB = db
	common.OptionMap = make(map[string]string)
	t.Cleanup(func() {
		model.DB = oldDB
		common.OptionMap = oldOptionMap
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestChooseTrustedRatioValueRequiresAllTrustedSourcesToMoveTogether(t *testing.T) {
	value, source, ok, reason := chooseTrustedRatioValue(dto.DifferenceItem{
		Upstreams: map[string]any{
			"official":   2.5,
			"models.dev": "same",
		},
		Confidence: map[string]bool{
			"official":   true,
			"models.dev": true,
		},
	})

	require.False(t, ok)
	require.Nil(t, value)
	require.Empty(t, source)
	require.Equal(t, "可信上游值与当前配置不一致", reason)
}

func TestApplyTrustedRatioDifferencesAppliesOnlyTrustedConsistentValue(t *testing.T) {
	setupChannelBalanceMonitorTestDB(t)
	ratio_setting.InitRatioSettings()
	restore := operation_setting.SetMonitorSettingForTest(operation_setting.MonitorSetting{
		ChannelBalanceWarningThreshold:       10,
		ChannelBalanceMonitorRetentionDays:   30,
		ChannelRatioSyncTrustedAutoApply:     true,
		ChannelBalanceMonitorIntervalMinutes: 10,
		ChannelRatioSyncIntervalMinutes:      60,
	})
	defer restore()

	applied, conflicts := applyTrustedRatioDifferences(map[string]map[string]dto.DifferenceItem{
		"gpt-test": {
			"model_ratio": {
				Current: 1.0,
				Upstreams: map[string]any{
					"models.dev": 2.0,
					"official":   2.0,
				},
				Confidence: map[string]bool{
					"models.dev": true,
					"official":   true,
				},
			},
		},
	}, true)

	require.Equal(t, 1, applied)
	require.Equal(t, 0, conflicts)
	modelRatio, ok, _ := ratio_setting.GetModelRatio("gpt-test")
	require.True(t, ok)
	require.Equal(t, 2.0, modelRatio)
}

func TestApplyTrustedRatioDifferencesRecordsConflictForMixedCurrentAndChangedSources(t *testing.T) {
	db := setupChannelBalanceMonitorTestDB(t)
	ratio_setting.InitRatioSettings()
	restore := operation_setting.SetMonitorSettingForTest(operation_setting.MonitorSetting{
		ChannelBalanceWarningThreshold:       10,
		ChannelBalanceMonitorRetentionDays:   30,
		ChannelRatioSyncTrustedAutoApply:     true,
		ChannelBalanceMonitorIntervalMinutes: 10,
		ChannelRatioSyncIntervalMinutes:      60,
	})
	defer restore()

	applied, conflicts := applyTrustedRatioDifferences(map[string]map[string]dto.DifferenceItem{
		"gpt-test": {
			"model_ratio": {
				Current: 1.0,
				Upstreams: map[string]any{
					"models.dev": 2.0,
					"official":   "same",
				},
				Confidence: map[string]bool{
					"models.dev": true,
					"official":   true,
				},
			},
		},
	}, true)

	require.Equal(t, 0, applied)
	require.Equal(t, 1, conflicts)
	var event model.ChannelBalanceMonitorEvent
	require.NoError(t, db.Where("event_type = ?", channelBalanceMonitorEventRatioConflict).First(&event).Error)
	require.Equal(t, "可信上游值与当前配置不一致", event.Error)
}

func TestConvertSub2APIAvailableChannelsToRatioDataSupportsWrappedResponse(t *testing.T) {
	body := `{
		"code": 0,
		"message": "success",
		"data": [
			{
				"name": "Codex",
				"platforms": [
					{
						"platform": "openai",
						"supported_models": [
							{
								"name": "gpt-5.5",
								"pricing": {
									"billing_mode": "token",
									"input_price": 0.000002,
									"output_price": 0.000012,
									"cache_write_price": 0.000004,
									"cache_read_price": 0.0000005
								}
							}
						]
					}
				]
			}
		]
	}`

	converted, err := convertSub2APIAvailableChannelsToRatioData(strings.NewReader(body))

	require.NoError(t, err)
	require.Equal(t, 1.0, converted["model_ratio"].(map[string]any)["gpt-5.5"])
	require.Equal(t, 6.0, converted["completion_ratio"].(map[string]any)["gpt-5.5"])
	require.Equal(t, 0.25, converted["cache_ratio"].(map[string]any)["gpt-5.5"])
	require.Equal(t, 2.0, converted["create_cache_ratio"].(map[string]any)["gpt-5.5"])
}

func TestConvertSub2APIAvailableChannelsToRatioDataSupportsBareArrayAndIntervals(t *testing.T) {
	body := `[
		{
			"name": "Codex",
			"platforms": [
				{
					"platform": "openai",
					"supported_models": [
						{
							"name": "codex-mini",
							"pricing": {
								"billing_mode": "token",
								"intervals": [
									{
										"min_tokens": 0,
										"input_price": 0.000001,
										"output_price": 0.000003,
										"per_request_price": 0.02
									}
								]
							}
						}
					]
				}
			]
		}
	]`

	converted, err := convertSub2APIAvailableChannelsToRatioData(strings.NewReader(body))

	require.NoError(t, err)
	require.Equal(t, 0.5, converted["model_ratio"].(map[string]any)["codex-mini"])
	require.Equal(t, 3.0, converted["completion_ratio"].(map[string]any)["codex-mini"])
	require.Equal(t, 0.02, converted["model_price"].(map[string]any)["codex-mini"])
}

func TestBuildChannelBalanceMonitorRatioSummaryUsesMappedPricingModel(t *testing.T) {
	ratio_setting.InitRatioSettings()
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldCompletionRatio := ratio_setting.CompletionRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		_ = ratio_setting.UpdateModelRatioByJSONString(oldModelRatio)
		_ = ratio_setting.UpdateCompletionRatioByJSONString(oldCompletionRatio)
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio)
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"upstream-codex":2}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"upstream-codex":4}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.3}`))
	mapping := `{"codex-plus":"upstream-codex"}`
	channel := &model.Channel{
		Models:       "codex-plus",
		Group:        "codex-plus",
		ModelMapping: &mapping,
	}

	summary := buildChannelBalanceMonitorRatioSummary(channel)

	require.Equal(t, 0.3, summary.GroupRatio)
	require.Equal(t, 1, summary.ModelCount)
	require.Len(t, summary.Models, 1)
	require.Equal(t, "codex-plus", summary.Models[0].Model)
	require.Equal(t, "upstream-codex", summary.Models[0].PricingModel)
	require.Equal(t, 2.0, summary.Models[0].ModelRatio)
	require.Equal(t, 4.0, summary.Models[0].CompletionRatio)
}
