package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	modelgatewaycredential "github.com/QuantumNous/new-api/pkg/modelgateway/credential"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestBuildChannelTestSelectionLocksCredentialIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	index := 1
	channel := &model.Channel{
		Id:     703,
		Name:   "account-test-lock",
		Key:    "sk-one\nsk-two",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}

	selection := buildChannelTestSelection(channel, channelTestOptions{CredentialIndex: &index})
	require.NotNil(t, selection)
	resolved, apiErr := modelgatewaycredential.ResolveChannelCredential(channel, selection.Plan.CredentialRef)

	require.Nil(t, apiErr)
	modelgatewaycredential.ApplyResolvedCredentialToContext(ctx, resolved)
	require.Equal(t, 1, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	require.Equal(t, "sk-two", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
}

func TestClearChannelBalanceInsufficientFromSuccessfulTestClearsMarkers(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	autoBan := 1
	channel := model.Channel{
		Id:                 701,
		Name:               "balance-ok-after-test",
		Type:               constant.ChannelTypeOpenAI,
		Status:             common.ChannelStatusEnabled,
		AutoBan:            &autoBan,
		Balance:            0,
		BalanceUpdatedTime: common.GetTimestamp(),
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": service.ChannelStatusReasonBalanceInsufficient,
	})
	require.NoError(t, db.Create(&channel).Error)
	service.MarkChannelBalanceInsufficient(channel.Id)

	status, cleared := clearChannelBalanceInsufficientFromSuccessfulTest(&channel, testResult{})

	require.True(t, cleared)
	require.Equal(t, common.ChannelStatusEnabled, status)
	require.False(t, service.IsRuntimeBalanceInsufficientChannelID(channel.Id))

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.False(t, service.IsKnownBalanceInsufficientChannel(updated))
	require.Equal(t, "", service.ChannelStatusReason(updated))
	require.Equal(t, int64(0), updated.BalanceUpdatedTime)
}

func TestClearChannelBalanceInsufficientFromSuccessfulTestResumesBalancePausedChannel(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	autoBan := 1
	channel := model.Channel{
		Id:      702,
		Name:    "balance-paused-ok-after-test",
		Type:    constant.ChannelTypeOpenAI,
		Status:  common.ChannelStatusAutoDisabled,
		AutoBan: &autoBan,
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": service.ChannelStatusReasonBalanceInsufficient,
	})
	require.NoError(t, db.Create(&channel).Error)

	status, cleared := clearChannelBalanceInsufficientFromSuccessfulTest(&channel, testResult{})

	require.True(t, cleared)
	require.Equal(t, common.ChannelStatusEnabled, status)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusEnabled, updated.Status)
	require.False(t, service.IsKnownBalanceInsufficientChannel(updated))
	require.Equal(t, "", service.ChannelStatusReason(updated))
}
