package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesStreamHandlerTreatsCreatedThenEOFAsError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = false
	setting.PingIntervalSeconds = 10
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.5\",\"created_at\":1}}\n",
		)),
		Header: make(http.Header),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
		IsStream:    true,
	}

	usage, err := OaiResponsesStreamHandler(ctx, info, resp)
	require.Nil(t, usage)
	require.Error(t, err)
	require.Equal(t, http.StatusInternalServerError, err.StatusCode)
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesStreamHandlerUsesDeliveredDeltaWhenCompletedMissing(t *testing.T) {
	service.InitTokenEncoders()
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = false
	setting.PingIntervalSeconds = 10
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.5\",\"created_at\":1}}",
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}",
		}, "\n"))),
		Header: make(http.Header),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	startTime := time.Now()
	info := &relaycommon.RelayInfo{
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
		IsStream:          true,
		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
	}

	usage, err := OaiResponsesStreamHandler(ctx, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Contains(t, recorder.Body.String(), "response.output_text.delta")
}

func TestOaiResponsesStreamHandlerReturnsFailedEventError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = false
	setting.PingIntervalSeconds = 10
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}",
			"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-5.5\",\"created_at\":1,\"error\":{\"message\":\"upstream failed\",\"type\":\"server_error\",\"code\":\"server_error\"}}}",
		}, "\n"))),
		Header: make(http.Header),
	}
	resp.Header.Set("Content-Type", "text/event-stream")

	startTime := time.Now()
	info := &relaycommon.RelayInfo{
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
		IsStream:          true,
		StartTime:         startTime,
		FirstResponseTime: startTime.Add(-time.Second),
	}

	usage, err := OaiResponsesStreamHandler(ctx, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, err)
	require.Equal(t, http.StatusInternalServerError, err.StatusCode)
	require.Contains(t, err.Error(), "upstream failed")
}
