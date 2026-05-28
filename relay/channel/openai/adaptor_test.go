package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetRequestURLUsesResponsesWireAPI(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeResponses,
		RequestURLPath: "/v1/responses",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: "https://ylscode.com/codex",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				WireAPI: "responses",
			},
		},
	}

	got, err := adaptor.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	want := "https://ylscode.com/codex/responses"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

func TestGetRequestURLUsesResponsesWireAPIForCompact(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeResponsesCompact,
		RequestURLPath: "/v1/responses/compact",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: "https://ylscode.com/codex",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				WireAPI: "responses",
			},
		},
	}

	got, err := adaptor.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	want := "https://ylscode.com/codex/responses/compact"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

func TestGetRequestURLUsesCustomWireAPIPath(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeResponses,
		RequestURLPath: "/v1/responses",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: "https://ylscode.com/codex",
			ChannelOtherSettings: dto.ChannelOtherSettings{
				WireAPI: "/backend-api/codex/responses",
			},
		},
	}

	got, err := adaptor.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	want := "https://ylscode.com/codex/backend-api/codex/responses"
	if got != want {
		t.Fatalf("GetRequestURL() = %q, want %q", got, want)
	}
}

func TestSetupRequestHeaderUsesOAuthJSONCredentialForOpenAIChannel(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	header := http.Header{}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ApiKey:      `{"access_token":"access-token","account_id":"account-id","refresh_token":"refresh-token"}`,
		},
	}

	err := adaptor.SetupRequestHeader(ctx, &header, info)

	require.NoError(t, err)
	require.Equal(t, "Bearer access-token", header.Get("Authorization"))
	require.Equal(t, "account-id", header.Get("chatgpt-account-id"))
	require.Equal(t, "responses=experimental", header.Get("OpenAI-Beta"))
	require.Equal(t, "codex_cli_rs", header.Get("originator"))
}

func TestConvertOpenAIResponsesRequestTracksUpstreamSuffixEffort(t *testing.T) {
	t.Parallel()

	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{
		RequestModelName:       "gpt-5.5",
		OriginModelName:        "gpt-5.5",
		RequestReasoningEffort: "xhigh",
		ReasoningEffort:        "xhigh",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.4-medium",
		},
	}
	req := dto.OpenAIResponsesRequest{
		Model: "gpt-5.4-medium",
	}

	converted, err := adaptor.ConvertOpenAIResponsesRequest(nil, info, req)

	require.NoError(t, err)
	convertedReq := converted.(dto.OpenAIResponsesRequest)
	require.Equal(t, "gpt-5.4", convertedReq.Model)
	require.NotNil(t, convertedReq.Reasoning)
	require.Equal(t, "medium", convertedReq.Reasoning.Effort)
	require.Equal(t, "gpt-5.4", info.UpstreamModelName)
	require.Equal(t, "medium", info.ReasoningEffort)
	require.Equal(t, "xhigh", info.RequestReasoningEffort)
}

func TestNormalizeOpenAIResponseModelUsesRequestedModel(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RequestModelName: "gpt-5.5",
		OriginModelName:  "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.4",
			IsModelMapped:     true,
		},
	}
	resp := &dto.OpenAITextResponse{Model: "gpt-5.4"}

	normalizeOpenAITextResponseModel(info, resp)

	require.Equal(t, "gpt-5.5", resp.Model)
	require.Equal(t, "gpt-5.4", info.ResponseModelName)
	require.Equal(t, "gpt-5.5", info.DownstreamModelName)
}

func TestNormalizeOpenAIStreamResponseModelUsesRequestedModel(t *testing.T) {
	t.Parallel()

	info := &relaycommon.RelayInfo{
		RequestModelName: "gpt-5.5",
		OriginModelName:  "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.4",
			IsModelMapped:     true,
		},
	}
	resp := &dto.ChatCompletionsStreamResponse{Model: "gpt-5.4"}

	normalizeOpenAIStreamResponseModel(info, resp)

	require.Equal(t, "gpt-5.5", resp.Model)
	require.Equal(t, "gpt-5.4", info.ResponseModelName)
	require.Equal(t, "gpt-5.5", info.DownstreamModelName)
}
