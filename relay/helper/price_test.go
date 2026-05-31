package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestApplySelectedGroupRatioUsesActualSelectedGroupForAutoBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"auto":1,"codex-plus":0.1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "auto")

	info := &relaycommon.RelayInfo{
		TokenGroup:  "auto",
		UserGroup:   "default",
		UsingGroup:  "auto",
		PriceData:   types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
		Request:     nil,
		RequestId:   "req-auto-codex-plus",
		StartTime:   time.Now(),
		RelayFormat: types.RelayFormatOpenAI,
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			GroupRatio:                1,
			EstimatedQuotaBeforeGroup: 1000,
			EstimatedQuotaAfterGroup:  1000,
		},
	}

	groupRatioInfo := ApplySelectedGroupRatio(ctx, info, "codex-plus")

	require.Equal(t, "codex-plus", info.UsingGroup)
	require.Equal(t, "codex-plus", common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))
	require.Equal(t, "codex-plus", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	require.Equal(t, 0.1, groupRatioInfo.GroupRatio)
	require.Equal(t, 0.1, info.PriceData.GroupRatioInfo.GroupRatio)
	require.Equal(t, 0.1, info.TieredBillingSnapshot.GroupRatio)
	require.Equal(t, 100, info.TieredBillingSnapshot.EstimatedQuotaAfterGroup)
}

func TestApplySelectedGroupRatioUsesDynamicBillingForSelectedGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"auto":1,"codex-plus":0.1}`))
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:       true,
		DynamicBillingProfitRate:    0.2,
		DynamicBillingMinSamples:    1,
		DynamicBillingMaxAgeSeconds: 300,
	})
	now := time.Now().Unix()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-test\x00codex-plus": {
			RequestedModel: "gpt-test",
			Group:          "codex-plus",
			Ratio:          0.37,
			SampleCount:    3,
			CalculatedAt:   now,
			WindowStart:    now - 60,
			WindowEnd:      now,
			ProfitRate:     0.2,
		},
	})
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		restoreSetting()
		restoreBaselines()
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	modelgatewayintegration.SetSelectedPlan(ctx, &core.DispatchPlan{
		SelectedGroup:    "codex-plus",
		BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		TokenGroup:      "auto",
		UserGroup:       "default",
		UsingGroup:      "auto",
		PriceData:       types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			GroupRatio:                1,
			EstimatedQuotaBeforeGroup: 1000,
			EstimatedQuotaAfterGroup:  1000,
		},
	}

	groupRatioInfo := ApplySelectedGroupRatio(ctx, info, "codex-plus")

	require.Equal(t, 0.37, groupRatioInfo.GroupRatio)
	require.Equal(t, 0.37, info.PriceData.GroupRatioInfo.GroupRatio)
	require.NotNil(t, info.DynamicBilling)
	require.True(t, info.DynamicBilling.Applied)
	require.Equal(t, 0.1, info.DynamicBilling.StaticGroupRatio)
	require.Equal(t, 370, info.TieredBillingSnapshot.EstimatedQuotaAfterGroup)
	require.Equal(t, 370, info.PriceData.QuotaToPreConsume)
	require.Equal(t, 1000.0, info.PriceData.QuotaBeforeGroup)
}

func TestModelPriceHelperUsesDynamicBillingForPreselectedPlan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"codex-plus":0.1}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-test":2}`))
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:       true,
		DynamicBillingProfitRate:    0.2,
		DynamicBillingMinSamples:    1,
		DynamicBillingMaxAgeSeconds: 300,
	})
	now := time.Now().Unix()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-test\x00codex-plus": {
			RequestedModel: "gpt-test",
			Group:          "codex-plus",
			Ratio:          0.37,
			PricePerM:      1.48,
			SampleCount:    3,
			CalculatedAt:   now,
			WindowStart:    now - 60,
			WindowEnd:      now,
			ProfitRate:     0.2,
		},
	})
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		restoreSetting()
		restoreBaselines()
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "codex-plus")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "codex-plus")
	modelgatewayintegration.SetSelectedPlan(ctx, &core.DispatchPlan{
		RequestedGroup:   "codex-plus",
		SelectedGroup:    "codex-plus",
		BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		TokenGroup:      "codex-plus",
		UserGroup:       "default",
		UsingGroup:      "codex-plus",
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.NotNil(t, info.DynamicBilling)
	require.True(t, info.DynamicBilling.Applied)
	require.Equal(t, 0.37, priceData.GroupRatioInfo.GroupRatio)
	require.Equal(t, 0.37, info.PriceData.GroupRatioInfo.GroupRatio)
}

func TestApplySelectedGroupRatioUpdatesPreConsumeForDynamicRatio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"auto":1,"codex-plus":0.1}`))
	restoreSetting := scheduler_setting.SetSettingForTest(scheduler_setting.SchedulerSetting{
		DynamicBillingEnabled:       true,
		DynamicBillingMinSamples:    1,
		DynamicBillingMaxAgeSeconds: 300,
	})
	now := time.Now().Unix()
	restoreBaselines := modelgatewaydynamicbilling.StoreDefaultBaselinesForTest(map[string]modelgatewaydynamicbilling.RatioBaseline{
		"gpt-test\x00codex-plus": {
			RequestedModel: "gpt-test",
			Group:          "codex-plus",
			Ratio:          0.4,
			SampleCount:    5,
			CalculatedAt:   now,
		},
	})
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		restoreSetting()
		restoreBaselines()
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	modelgatewayintegration.SetSelectedPlan(ctx, &core.DispatchPlan{
		SelectedGroup:    "codex-plus",
		BillingRatioMode: scheduler_setting.BillingRatioModeDynamic,
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-test",
		TokenGroup:      "auto",
		UserGroup:       "default",
		UsingGroup:      "auto",
		PriceData: types.PriceData{
			ModelRatio:        2,
			QuotaBeforeGroup:  1000,
			QuotaToPreConsume: 100,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}

	groupRatioInfo := ApplySelectedGroupRatio(ctx, info, "codex-plus")

	require.Equal(t, 0.4, groupRatioInfo.GroupRatio)
	require.NotNil(t, info.DynamicBilling)
	require.True(t, info.DynamicBilling.Applied)
	require.Equal(t, 400, info.PriceData.QuotaToPreConsume)
}

func TestModelPriceHelperPerCallAppliesFixedPriceMarginGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldDB := model.DB
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldModelPrice := ratio_setting.ModelPrice2JSONString()
	model.DB = nil
	modelgatewaycost.RemoveCachedDefaultProfilesForChannel(88)
	modelgatewaycost.StoreCachedDefaultProfile(model.ModelGatewayChannelCostProfile{
		Id:            808,
		ChannelID:     88,
		UpstreamModel: "fixed-asset",
		PricingMode:   "request",
		Source:        modelgatewaycost.SourceManual,
		Accuracy:      modelgatewaycost.AccuracyPrecise,
		RequestPrice:  0.02,
		MetadataJSON:  `{"fixed_price_margin_guard_target_margin":40}`,
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":0.01}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed-asset":0.9}`))
	t.Cleanup(func() {
		model.DB = oldDB
		modelgatewaycost.RemoveCachedDefaultProfilesForChannel(88)
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrice))
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "fixed-asset",
		UserGroup:       "default",
		UsingGroup:      "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         88,
			UpstreamModelName: "fixed-asset",
		},
	}

	priceData, err := ModelPriceHelperPerCall(ctx, info)
	require.NoError(t, err)
	expectedRatio := 0.02 / (1 - modelgatewaycost.FixedPriceMarginGuardDefaultTargetMargin) / 0.9
	require.InEpsilon(t, expectedRatio, priceData.GroupRatioInfo.GroupRatio, 0.000001)
	require.Equal(t, int(0.9*common.QuotaPerUnit*expectedRatio), priceData.Quota)
	require.NotNil(t, priceData.FixedPriceMarginGuard)
	require.True(t, priceData.FixedPriceMarginGuard.Applied)
	require.Equal(t, 808, priceData.FixedPriceMarginGuard.ProfileID)
}
