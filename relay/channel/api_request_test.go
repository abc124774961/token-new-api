package channel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok)
}

func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"])
}

func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value",
			"x-runtime": "runtime-only",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
				"X-Legacy": "legacy-only",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"])
	require.Equal(t, "runtime-only", headers["x-runtime"])
	_, exists := headers["x-legacy"]
	require.False(t, exists)
}

func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("X-Feature-Flag", "feature-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "feature-123", headers["x-feature-flag"])

	_, hasTraceID := headers["x-trace-id"]
	require.False(t, hasTraceID)
	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding)
}

func TestSanitizeUpstreamRequestHeadersDropsInternalAndClientTraceHeaders(t *testing.T) {
	t.Parallel()

	headers := http.Header{}
	headers.Set("Authorization", "Bearer upstream-key")
	headers.Set("chatgpt-account-id", "acct-1")
	headers.Set("OpenAI-Beta", "responses=v1")
	headers.Set("User-Agent", "codex-cli-test")
	headers.Set("originator", "codex_cli_rs")
	headers.Set("X-Codex-Beta-Features", "terminal_resize_reflow")
	headers.Set("Session_id", "client-session")
	headers.Set("X-Codex-Turn-Metadata", `{"thread_id":"thread-1"}`)
	headers.Set("X-Codex-Trace", "trace-1")
	headers.Set("X-Forwarded-For", "203.0.113.1")
	headers.Set("X-Real-IP", "203.0.113.1")
	headers.Set("CF-Connecting-IP", "203.0.113.1")
	headers.Set("X-New-Api-User", "user-1")
	headers.Set("X-User-Email", "user@example.com")
	headers.Set("X-Trace-Id", "trace-2")

	sanitizeUpstreamRequestHeaders(headers)

	require.Equal(t, "Bearer upstream-key", headers.Get("Authorization"))
	require.Equal(t, "acct-1", headers.Get("chatgpt-account-id"))
	require.Equal(t, "responses=v1", headers.Get("OpenAI-Beta"))
	require.Equal(t, "codex-cli-test", headers.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", headers.Get("originator"))
	require.Equal(t, "terminal_resize_reflow", headers.Get("X-Codex-Beta-Features"))
	require.Empty(t, headers.Get("Session_id"))
	require.Empty(t, headers.Get("X-Codex-Turn-Metadata"))
	require.Empty(t, headers.Get("X-Codex-Trace"))
	require.Empty(t, headers.Get("X-Forwarded-For"))
	require.Empty(t, headers.Get("X-Real-IP"))
	require.Empty(t, headers.Get("CF-Connecting-IP"))
	require.Empty(t, headers.Get("X-New-Api-User"))
	require.Empty(t, headers.Get("X-User-Email"))
	require.Empty(t, headers.Get("X-Trace-Id"))
}

func TestProcessHeaderOverride_OAuthJSONSkipsManagedAuthOverrides(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ApiKey:      `{"access_token":"access-token","account_id":"account-id","refresh_token":"refresh-token"}`,
			HeadersOverride: map[string]any{
				"Authorization":      "Bearer {api_key}",
				"chatgpt-account-id": "wrong-account-id",
				"User-Agent":         "Codex CLI",
			},
		},
	}

	headers, err := processHeaderOverride(info, ctx)

	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["user-agent"])
	_, hasAuthorization := headers["authorization"]
	require.False(t, hasAuthorization)
	_, hasAccountID := headers["chatgpt-account-id"]
	require.False(t, hasAccountID)
}

func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI")
	ctx.Request.Header.Set("Session_id", "sess-123")

	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"},
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"])

	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"])
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists)

	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator"))
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features"))
}

func TestApplyDefaultUpstreamHeadersCodexSetsUserAgent(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	req := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeCodex,
		},
	}

	applyDefaultUpstreamHeaders(ctx, req, info)

	require.Equal(t, "codex_cli_rs/0.0.0", req.Header.Get("User-Agent"))
	require.Equal(t, "codex_cli_rs", req.Header.Get("originator"))
}

func TestApplyDefaultUpstreamHeadersCodexKeepsRuntimeUserAgent(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	req := httptest.NewRequest(http.MethodPost, "https://example.com/backend-api/codex/responses", nil)
	req.Header.Set("User-Agent", "codex-cli-test")
	req.Header.Set("originator", "Codex CLI")
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeCodex,
		},
	}

	applyDefaultUpstreamHeaders(ctx, req, info)

	require.Equal(t, "codex-cli-test", req.Header.Get("User-Agent"))
	require.Equal(t, "Codex CLI", req.Header.Get("originator"))
}

func TestApplyDefaultUpstreamHeadersCodexLikeResponsesUsesClientUserAgent(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("User-Agent", "codex-cli/0.45.0")
	ctx.Request.Header.Set("Originator", "Codex CLI")

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		UsingGroup: "codex-plus",
		RelayMode:  relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
		},
	}

	applyDefaultUpstreamHeaders(ctx, req, info)

	require.Equal(t, "codex-cli/0.45.0", req.Header.Get("User-Agent"))
	require.Equal(t, "Codex CLI", req.Header.Get("originator"))
}
