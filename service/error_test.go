package service

import (
	"context"
	"testing"
	"io"
	"net/http"
	"strings"

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
