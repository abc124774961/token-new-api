package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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
