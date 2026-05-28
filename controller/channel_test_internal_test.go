package controller

import (
	"errors"
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

func TestResolveChannelTestEndpointUsesResponsesForOpenAIOAuthJSON(t *testing.T) {
	index := 1
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI,
		Key:  `sk-normal` + "\n" + `{"access_token":"access-token","refresh_token":"refresh-token","account_id":"account-id"}`,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}

	endpointType := resolveChannelTestEndpoint(channel, "gpt-5.4", "", channelTestOptions{CredentialIndex: &index})

	require.Equal(t, string(constant.EndpointTypeOpenAIResponse), endpointType)
}

func TestIsMissingResponsesScopeTestResult(t *testing.T) {
	require.True(t, isMissingResponsesScopeTestResult(testResult{
		newAPIError: types.NewOpenAIError(
			errors.New("Missing scopes: api.responses.write"),
			types.ErrorCodeBadResponseStatusCode,
			401,
		),
	}))
	require.False(t, isMissingResponsesScopeTestResult(testResult{
		newAPIError: types.NewOpenAIError(
			errors.New("model does not exist"),
			types.ErrorCodeBadResponseStatusCode,
			404,
		),
	}))
}

func TestChannelCapabilityValueFromProbe(t *testing.T) {
	require.True(t, *channelCapabilityValueFromProbe(testResult{}))

	denied := channelCapabilityValueFromProbe(testResult{
		newAPIError: types.NewOpenAIError(
			errors.New("Missing scopes: api.responses.write"),
			types.ErrorCodeBadResponseStatusCode,
			401,
		),
	})
	require.NotNil(t, denied)
	require.False(t, *denied)

	unknown := channelCapabilityValueFromProbe(testResult{
		newAPIError: types.NewOpenAIError(
			errors.New("context deadline exceeded"),
			types.ErrorCodeDoRequestFailed,
			500,
		),
	})
	require.Nil(t, unknown)
}

func TestResolveChannelTestEndpointDoesNotGuessMultiKeyOAuthJSONWithoutSelection(t *testing.T) {
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI,
		Key:  `{"access_token":"access-token","refresh_token":"refresh-token","account_id":"account-id"}` + "\n" + `sk-normal`,
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}

	endpointType := resolveChannelTestEndpoint(channel, "gpt-5.4", "", channelTestOptions{})

	require.Empty(t, endpointType)
}

func TestShouldRefreshOAuthJSONAccountAfterChannelTest(t *testing.T) {
	require.True(t, shouldRefreshOAuthJSONAccountAfterChannelTest(testResult{
		newAPIError: types.WithOpenAIError(types.OpenAIError{
			Message: "Your authentication token has been invalidated. Please try signing in again.",
			Type:    "invalid_request_error",
			Code:    "token_invalidated",
		}, 401),
	}))
	require.True(t, shouldRefreshOAuthJSONAccountAfterChannelTest(testResult{
		newAPIError: types.NewOpenAIError(errors.New("bad response status code 401, message: token_invalidated"), types.ErrorCodeBadResponseStatusCode, 401),
	}))
	require.False(t, shouldRefreshOAuthJSONAccountAfterChannelTest(testResult{
		newAPIError: types.NewOpenAIError(errors.New("invalid api key"), types.ErrorCodeChannelInvalidKey, 401),
	}))
}

func TestFriendlyChannelTestErrorMessageForOAuthRefreshFailure(t *testing.T) {
	message := friendlyChannelTestErrorMessage(testResult{
		newAPIError: types.WithOpenAIError(types.OpenAIError{
			Message: "Your authentication token has been invalidated. Please try signing in again.",
			Type:    "invalid_request_error",
			Code:    "token_invalidated",
		}, 401),
		refreshErr: errors.New("codex oauth refresh failed: status=401"),
	})

	require.Contains(t, message, "账号授权已失效")
	require.Contains(t, message, "重新从 xauto 下载账号数据")
}

func TestFriendlyChannelTestErrorMessageForMissingResponsesScope(t *testing.T) {
	message := friendlyChannelTestErrorMessage(testResult{
		newAPIError: types.NewOpenAIError(
			errors.New("You have insufficient permissions for this operation. Missing scopes: api.responses.write."),
			types.ErrorCodeBadResponseStatusCode,
			401,
		),
	})

	require.Contains(t, message, "账号权限不足")
	require.Contains(t, message, "api.responses.write")
}

func TestFriendlyChannelTestErrorMessageForProxyConnectionRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUpstreamRequestInfo, map[string]interface{}{
		"error":      "socks connect tcp 206.123.156.217:10722->api.openai.com:443: dial tcp 206.123.156.217:10722: connect: connection refused",
		"error_kind": "url_Post",
		"host":       "api.openai.com",
		"path":       "/v1/responses",
	})

	message := friendlyChannelTestErrorMessage(testResult{
		context: ctx,
		newAPIError: types.NewError(
			errors.New("upstream error: do request failed"),
			types.ErrorCodeDoRequestFailed,
		),
	})

	require.Contains(t, message, "代理连接被拒绝")
	require.Contains(t, message, "更换/取消代理")
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
