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
