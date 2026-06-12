package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	modelgatewayaccount "github.com/QuantumNous/new-api/pkg/modelgateway/account"
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
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.ChannelBalanceMonitorEvent{},
		&model.ModelGatewayChannelCostProfile{},
		&model.Option{},
	))

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

func TestBalanceMonitorDoesNotAutoDisableMultiKeyAccountWhenBalanceEmpty(t *testing.T) {
	db := setupChannelBalanceMonitorTestDB(t)
	channel := model.Channel{
		Id:     6601,
		Name:   "balance-visible",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyStatusList: map[int]int{},
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	reconcileChannelAccountBalanceStatus(&channel, modelgatewayaccount.ChannelAccount{
		ChannelID:       channel.Id,
		CredentialIndex: 0,
	}, 0, 10)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.Empty(t, updated.ChannelInfo.MultiKeyStatusList)
	require.Empty(t, updated.ChannelInfo.MultiKeyDisabledReason)
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

func TestBuildChannelBalanceMonitorRatioSummaryUsesChannelCostProfile(t *testing.T) {
	profile := model.ModelGatewayChannelCostProfile{
		Id:                  7,
		ChannelID:           11,
		PricingMode:         "token",
		CostCoefficient:     0.05,
		TokenMultiplier:     4.1,
		RechargeMultiplier:  2,
		RequestPrice:        0.02,
		Source:              "auto_synced",
		Accuracy:            "estimated",
		UpdatedAt:           1234,
		SyncedAt:            1200,
		InputPerMillion:     2,
		OutputPerMillion:    4,
		InputCostMultiplier: 9,
	}

	summary := buildChannelBalanceMonitorRatioSummary(profile, true)

	require.True(t, summary.Configured)
	require.True(t, summary.PriceConfigured)
	require.Equal(t, "token", summary.PricingMode)
	require.Equal(t, 0.1025, summary.CostMultiplier)
	require.Equal(t, 0.1025, summary.ActualTokenMultiplier)
	require.Equal(t, 0.05, summary.CostCoefficient)
	require.Equal(t, 4.1, summary.TokenMultiplier)
	require.Equal(t, 2.0, summary.RechargeMultiplier)
	require.Equal(t, 0.02, summary.RequestPrice)
	require.Equal(t, 0.01, summary.ActualRequestPrice)
	require.Equal(t, "auto_synced", summary.Source)
	require.Equal(t, int64(1200), summary.SyncedAt)
}

func TestFetchChannelBalanceResultSupportsNewAPITokenUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, []string{"/api/usage/token/", "/api/usage/token"}, r.URL.Path)
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": true,
			"message": "ok",
			"data": {
				"total_available": 1250000,
				"total_used": 250000,
				"unlimited_quota": false
			}
		}`))
	}))
	defer server.Close()

	baseURL := server.URL + "/v1"
	channel := &model.Channel{
		Type:    constant.ChannelTypeCustom,
		Key:     "sk-test",
		BaseURL: &baseURL,
	}

	result, err := fetchChannelBalanceResult(channel)

	require.NoError(t, err)
	require.Equal(t, 2.5, result.Balance)
	require.Equal(t, "new_api_token_usage", result.Source)
	require.Equal(t, "quota", result.RawUnit)
}

func TestFetchChannelBalanceResultSupportsCreditGrants(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dashboard/billing/credit_grants" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"credit_summary","total_available":12.75}`))
	}))
	defer server.Close()

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "sk-test",
		BaseURL: &baseURL,
	}

	result, err := fetchChannelBalanceResult(channel)

	require.NoError(t, err)
	require.Equal(t, 12.75, result.Balance)
	require.Equal(t, "openai_credit_grants", result.Source)
	require.Equal(t, "usd", result.RawUnit)
}

func TestRefreshChannelBalanceMonitorAccountRecordsBalanceSource(t *testing.T) {
	db := setupChannelBalanceMonitorTestDB(t)
	restore := operation_setting.SetMonitorSettingForTest(operation_setting.MonitorSetting{
		ChannelBalanceWarningThreshold:       10,
		ChannelBalanceMonitorRetentionDays:   30,
		ChannelBalanceMonitorIntervalMinutes: 10,
		ChannelRatioSyncIntervalMinutes:      60,
	})
	defer restore()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" && r.URL.Path != "/api/usage/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"quota":1500000,"used_quota":0}}`))
	}))
	defer server.Close()

	baseURL := server.URL
	channel := model.Channel{
		Type:    constant.ChannelTypeCustom,
		Key:     "sk-test",
		Name:    "new-api",
		Group:   "default",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(&channel).Error)
	ref := buildChannelBalanceMonitorAccountRefs([]*model.Channel{&channel})[0]

	require.NoError(t, refreshChannelBalanceMonitorAccount(ref))

	var event model.ChannelBalanceMonitorEvent
	require.NoError(t, db.Where("event_type = ?", channelBalanceMonitorEventBalanceLow).First(&event).Error)
	require.Equal(t, 3.0, event.Balance)
	details := map[string]any{}
	require.NoError(t, common.UnmarshalJsonStr(event.Details, &details))
	require.Equal(t, "new_api_token_usage", details["balance_source"])
	require.Equal(t, "quota", details["balance_raw_unit"])
}

func TestApplyUpstreamChannelCostSyncUpdatesDefaultProfileAndRecordsEvent(t *testing.T) {
	db := setupChannelBalanceMonitorTestDB(t)
	channel := model.Channel{
		Type:   1,
		Key:    "sk-test",
		Name:   "sub2",
		Group:  "codex-plus",
		Models: "codex-mini",
		Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, db.Create(&channel).Error)

	applied, conflicts := applyUpstreamChannelCostSync(map[string]map[string]dto.DifferenceItem{
		upstreamChannelCostField: {
			strconv.Itoa(channel.Id): {
				Current: 1.0,
				Upstreams: map[string]any{
					"sub2": UpstreamChannelCostSyncItem{
						CostMultiplier:        0.06,
						UpstreamCostSource:    "sub2",
						UpstreamCostChannel:   "codex-plus",
						UpstreamCostFieldPath: "platforms[0].groups[0].rate_multiplier",
					},
				},
				Confidence: map[string]bool{"sub2": true},
			},
		},
	}, true)

	require.Equal(t, 1, applied)
	require.Equal(t, 0, conflicts)
	var profile model.ModelGatewayChannelCostProfile
	require.NoError(t, db.Where("channel_id = ? AND upstream_model = ?", channel.Id, defaultChannelCostModel).First(&profile).Error)
	require.Equal(t, 0.06, profile.CostCoefficient)
	require.Equal(t, 1.0, profile.TokenMultiplier)
	require.Equal(t, 1.0, profile.RechargeMultiplier)
	require.Equal(t, "auto_synced", profile.Source)
	var event model.ChannelBalanceMonitorEvent
	require.NoError(t, db.Where("event_type = ?", channelBalanceMonitorEventCostApplied).First(&event).Error)
	require.Equal(t, upstreamChannelCostField, event.Field)
	require.Equal(t, strconv.Itoa(channel.Id), event.ModelName)
}

func TestExtractUpstreamChannelCostMultiplierFromSub2GroupRate(t *testing.T) {
	group := "codex-plus"
	channel := &model.Channel{
		Id:     42,
		Name:   "aiwano-plus-0.06",
		Group:  group,
		Models: "codex-mini",
	}
	chItem := dto.UpstreamDTO{
		ID:       42,
		Name:     "aiwano-plus-0.06",
		Endpoint: sub2APIEndpoint,
	}
	body := []byte(`[
		{
			"name": "aiwano-plus-0.06",
			"platforms": [
				{
					"platform": "openai",
					"groups": [
						{"name": "default", "rate_multiplier": 1},
						{"name": "codex-plus", "rate_multiplier": 0.06}
					],
					"supported_models": [{"name": "codex-mini"}]
				}
			]
		}
	]`)

	item, ok := extractUpstreamChannelCostSyncItem(body, chItem, channel)

	require.True(t, ok)
	require.Equal(t, 42, item.ChannelID)
	require.Equal(t, 0.06, item.CostMultiplier)
	require.Equal(t, "aiwano-plus-0.06", item.UpstreamCostChannel)
	require.Contains(t, item.UpstreamCostFieldPath, "rate_multiplier")
}
