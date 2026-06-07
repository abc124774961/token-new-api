package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestResetStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		statusCode       int
		statusCodeConfig string
		expectedCode     int
	}{
		{
			name:             "map string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"503"}`,
			expectedCode:     503,
		},
		{
			name:             "map int value",
			statusCode:       429,
			statusCodeConfig: `{"429":503}`,
			expectedCode:     503,
		},
		{
			name:             "skip invalid string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"bad-code"}`,
			expectedCode:     429,
		},
		{
			name:             "skip status code 200",
			statusCode:       200,
			statusCodeConfig: `{"200":503}`,
			expectedCode:     200,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newAPIError := &types.NewAPIError{
				StatusCode: tc.statusCode,
			}
			ResetStatusCode(newAPIError, tc.statusCodeConfig)
			require.Equal(t, tc.expectedCode, newAPIError.StatusCode)
		})
	}
}

func TestRelayErrorHandlerAttachesUpstreamResponseMetadata(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", upstreamResponseSnippetLimit+32)
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Status:     "502 Bad Gateway",
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=utf-8"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}

	err := RelayErrorHandler(context.Background(), resp, false)
	require.NotNil(t, err)
	require.NotEmpty(t, err.Metadata)

	var metadata struct {
		StatusCode    int    `json:"status_code"`
		StatusText    string `json:"status_text"`
		ContentType   string `json:"content_type"`
		BodySnippet   string `json:"body_snippet"`
		BodyLength    int    `json:"body_length"`
		BodyTruncated bool   `json:"body_truncated"`
	}
	require.NoError(t, common.Unmarshal(err.Metadata, &metadata))
	require.Equal(t, http.StatusBadGateway, metadata.StatusCode)
	require.Equal(t, "502 Bad Gateway", metadata.StatusText)
	require.Equal(t, "text/html; charset=utf-8", metadata.ContentType)
	require.Equal(t, len(body), metadata.BodyLength)
	require.True(t, metadata.BodyTruncated)
	require.Len(t, metadata.BodySnippet, upstreamResponseSnippetLimit)
	require.Equal(t, strings.Repeat("x", upstreamResponseSnippetLimit), metadata.BodySnippet)
}

func TestRelayErrorHandlerHidesUpstreamBalanceInsufficientFromClient(t *testing.T) {
	t.Parallel()

	body := `{"error":{"message":"Insufficient account balance","type":"billing_error","code":"INSUFFICIENT_BALANCE"}}`
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}

	err := RelayErrorHandler(context.Background(), resp, false)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeUpstreamUnavailable, err.GetErrorCode())
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.True(t, IsBalanceInsufficientError(err))

	clientErr := err.ToOpenAIError()
	require.Equal(t, upstreamBalanceInsufficientClientMessage, clientErr.Message)
	require.NotContains(t, strings.ToLower(clientErr.Message), "insufficient account balance")
	require.NotContains(t, strings.ToLower(clientErr.Message), "balance")

	var metadata struct {
		StatusCode  int    `json:"status_code"`
		StatusText  string `json:"status_text"`
		BodySnippet string `json:"body_snippet"`
		BodyLength  int    `json:"body_length"`
	}
	require.NoError(t, common.Unmarshal(err.Metadata, &metadata))
	require.Equal(t, http.StatusForbidden, metadata.StatusCode)
	require.Equal(t, "403 Forbidden", metadata.StatusText)
	require.Equal(t, len(body), metadata.BodyLength)
	require.Empty(t, metadata.BodySnippet)
	require.NotContains(t, common.JsonRawMessageToString(err.Metadata), "Insufficient account balance")
}

func TestClientContextLimitErrorDetection(t *testing.T) {
	t.Parallel()

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Requested 200000 tokens exceeds the maximum context length of 128000 tokens.",
		Type:    "invalid_request_error",
		Code:    "context_too_large",
	}, http.StatusBadRequest)

	require.True(t, IsClientContextLimitError(err))
	require.True(t, IsClientContextLimitMessage("context_length_exceeded"))
	require.True(t, IsClientContextLimitMessage("上下文长度超限"))
	require.False(t, IsClientContextLimitMessage("invalid api key"))
}

func TestRelayErrorHandlerHidesPlainTextUpstreamBalanceInsufficient(t *testing.T) {
	t.Parallel()

	body := "unexpected status 403 Forbidden: Insufficient account balance"
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}

	err := RelayErrorHandler(context.Background(), resp, false)
	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeUpstreamUnavailable, err.GetErrorCode())
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.True(t, IsBalanceInsufficientError(err))
	require.NotContains(t, common.JsonRawMessageToString(err.Metadata), "Insufficient account balance")
	require.NotContains(t, err.ToOpenAIError().Message, "Insufficient account balance")
}

func TestRelayErrorHandlerKeepsUpstreamBalanceDetailForExplicitChannelTest(t *testing.T) {
	t.Parallel()

	body := `{"error":{"message":"Insufficient account balance","type":"billing_error","code":"INSUFFICIENT_BALANCE"}}`
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Status:     "403 Forbidden",
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	err := RelayErrorHandler(context.Background(), resp, true)
	require.NotNil(t, err)
	require.NotEqual(t, types.ErrorCodeUpstreamUnavailable, err.GetErrorCode())
	require.Contains(t, err.Error(), "Insufficient account balance")
	require.Equal(t, http.StatusForbidden, err.StatusCode)
}

func TestUpstreamOpenAIErrorHidesBalanceInsufficient(t *testing.T) {
	t.Parallel()

	err := UpstreamOpenAIError(types.OpenAIError{
		Message: "Insufficient account balance",
		Type:    "billing_error",
		Code:    "INSUFFICIENT_BALANCE",
	}, http.StatusForbidden)

	require.NotNil(t, err)
	require.Equal(t, types.ErrorCodeUpstreamUnavailable, err.GetErrorCode())
	require.Equal(t, http.StatusServiceUnavailable, err.StatusCode)
	require.True(t, IsBalanceInsufficientError(err))
	require.Equal(t, upstreamBalanceInsufficientClientMessage, err.ToOpenAIError().Message)
	require.NotContains(t, common.JsonRawMessageToString(err.Metadata), "Insufficient account balance")
}

func TestSanitizeClientRelayErrorHidesUnhandledUpstreamBalanceInsufficient(t *testing.T) {
	t.Parallel()

	err := types.NewOpenAIError(
		errors.New("unexpected status 403 Forbidden: Insufficient account balance"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusForbidden,
	)

	sanitized := SanitizeClientRelayError(err)
	require.NotNil(t, sanitized)
	require.Equal(t, types.ErrorCodeUpstreamUnavailable, sanitized.GetErrorCode())
	require.Equal(t, http.StatusServiceUnavailable, sanitized.StatusCode)
	require.Equal(t, upstreamBalanceInsufficientClientMessage, sanitized.ToOpenAIError().Message)
}

func TestSanitizeClientRelayErrorKeepsLocalUserQuotaMessage(t *testing.T) {
	t.Parallel()

	err := types.NewErrorWithStatusCode(
		errors.New("用户额度不足"),
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
		types.ErrOptionWithSkipRetry(),
	)

	require.Same(t, err, SanitizeClientRelayError(err))
}

func TestSanitizeClientRelayErrorHidesRetryableUpstreamUserQuotaCode(t *testing.T) {
	t.Parallel()

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "Insufficient account balance",
		Type:    "insufficient_quota",
		Code:    string(types.ErrorCodeInsufficientUserQuota),
	}, http.StatusForbidden)

	sanitized := SanitizeClientRelayError(err)
	require.NotSame(t, err, sanitized)
	require.Equal(t, types.ErrorCodeUpstreamUnavailable, sanitized.GetErrorCode())
	require.NotContains(t, sanitized.ToOpenAIError().Message, "Insufficient account balance")
}
