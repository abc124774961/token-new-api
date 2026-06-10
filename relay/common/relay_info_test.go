package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	basecommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoCurrentRequestDurationUsesUpstreamCompletedTime(t *testing.T) {
	startedAt := time.Unix(100, 0)
	info := &RelayInfo{StartTime: startedAt}

	require.True(t, info.SetUpstreamCompletedTime(startedAt.Add(4*time.Second)))
	require.False(t, info.SetUpstreamCompletedTime(startedAt.Add(9*time.Second)))
	require.Equal(t, 4*time.Second, info.UpstreamCompletedDuration())
	require.Equal(t, 4*time.Second, info.CurrentRequestDuration())
}

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoInitChannelMetaAccountProxyOverridesChannelProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}
	basecommon.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	basecommon.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Proxy: "http://channel-proxy:8080",
	})
	basecommon.SetContextKey(ctx, constant.ContextKeyChannelAccountProxyURL, "socks5://account-proxy:1080")

	info := &RelayInfo{}
	info.InitChannelMeta(ctx)

	require.Equal(t, "socks5://account-proxy:1080", info.ChannelSetting.Proxy)
}

func TestRelayInfoInitChannelMetaUsesEffectiveCodexChannelType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := &gin.Context{}
	basecommon.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeCodex)
	basecommon.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, constant.ChannelBaseURLs[constant.ChannelTypeCodex])
	basecommon.SetContextKey(ctx, constant.ContextKeyChannelKey, `{"access_token":"access-a","account_id":"acct-a"}`)

	info := &RelayInfo{}
	info.InitChannelMeta(ctx)

	require.Equal(t, constant.ChannelTypeCodex, info.ChannelType)
	require.Equal(t, constant.APITypeCodex, info.ApiType)
	require.Equal(t, constant.ChannelBaseURLs[constant.ChannelTypeCodex], info.ChannelBaseUrl)
}

func TestApplyChannelForceStreamResponseForResponsesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	request := &dto.OpenAIResponsesRequest{Model: "gpt-5.5"}
	info := &RelayInfo{
		Request:   request,
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{ForceStreamResponse: true},
		},
	}

	require.True(t, info.ApplyChannelForceStreamResponse(ctx))
	require.True(t, info.IsStream)
	require.NotNil(t, request.Stream)
	require.True(t, *request.Stream)
	require.True(t, basecommon.GetContextKeyBool(ctx, constant.ContextKeyRelayForceStreamResponse))
	require.True(t, ctx.GetBool(string(constant.ContextKeyIsStream)))
}

func TestApplyChannelForceStreamResponseForChatRequest(t *testing.T) {
	request := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}
	info := &RelayInfo{
		Request:   request,
		RelayMode: relayconstant.RelayModeChatCompletions,
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{ForceStreamResponse: true},
		},
	}

	require.True(t, info.ApplyChannelForceStreamResponse(nil))
	require.True(t, info.IsStream)
	require.NotNil(t, request.Stream)
	require.True(t, *request.Stream)
}

func TestApplyChannelForceStreamResponseSkipsUnsupportedRequest(t *testing.T) {
	request := &dto.EmbeddingRequest{Model: "text-embedding-3-large"}
	info := &RelayInfo{
		Request:   request,
		RelayMode: relayconstant.RelayModeEmbeddings,
		ChannelMeta: &ChannelMeta{
			ChannelSetting: dto.ChannelSettings{ForceStreamResponse: true},
		},
	}

	require.False(t, info.ApplyChannelForceStreamResponse(nil))
	require.False(t, info.IsStream)
}
