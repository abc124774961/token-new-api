package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newKeepAliveTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	return c, recorder
}

func TestDownstreamKeepAliveWritesJSONWhitespaceChunk(t *testing.T) {
	c, recorder := newKeepAliveTestContext()
	src := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
	}
	src.Header.Set("Content-Type", "application/json")
	src.Header.Set("Content-Length", "128")

	keepAlive := &DownstreamKeepAlive{
		c:     c,
		src:   src,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
		mutex: &sync.Mutex{},
	}
	go keepAlive.run(10*time.Millisecond, []byte("\n"))
	defer keepAlive.Stop()

	require.Eventually(t, func() bool {
		return common.GetContextKeyInt(c, constant.ContextKeyRelayDownstreamKeepAliveCount) > 0
	}, time.Second, 10*time.Millisecond)
	require.NotEmpty(t, recorder.Body.String())
	require.Empty(t, strings.Trim(recorder.Body.String(), "\n"))
	require.Empty(t, recorder.Header().Get("Content-Length"))
	require.Equal(t, "chunked", recorder.Header().Get("Transfer-Encoding"))
	require.Equal(t, "no", recorder.Header().Get("X-Accel-Buffering"))
	require.True(t, common.GetContextKeyBool(c, constant.ContextKeyRelayClientReceivedStarted))
}

func TestIOCopyBytesWithJSONKeepAliveSkipsBinaryResponse(t *testing.T) {
	c, recorder := newKeepAliveTestContext()
	src := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
	}
	src.Header.Set("Content-Type", "audio/mpeg")
	data := []byte{1, 2, 3}

	IOCopyBytesWithJSONKeepAliveGracefully(c, src, data)

	require.Equal(t, data, recorder.Body.Bytes())
	require.Equal(t, "3", recorder.Header().Get("Content-Length"))
	require.Empty(t, recorder.Header().Get("Transfer-Encoding"))
	require.Zero(t, common.GetContextKeyInt(c, constant.ContextKeyRelayDownstreamKeepAliveCount))
}

func TestDownstreamKeepAliveIntervalDefaultsAndBounds(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previous := settings.DownstreamKeepaliveIntervalSeconds
	t.Cleanup(func() {
		settings.DownstreamKeepaliveIntervalSeconds = previous
	})

	tests := []struct {
		name       string
		configured int
		want       time.Duration
	}{
		{
			name:       "default",
			configured: 0,
			want:       15 * time.Second,
		},
		{
			name:       "minimum",
			configured: 1,
			want:       5 * time.Second,
		},
		{
			name:       "configured",
			configured: 20,
			want:       20 * time.Second,
		},
		{
			name:       "cdn safe maximum",
			configured: 55,
			want:       25 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings.DownstreamKeepaliveIntervalSeconds = tt.configured
			require.Equal(t, tt.want, DownstreamKeepAliveInterval())
		})
	}
}
