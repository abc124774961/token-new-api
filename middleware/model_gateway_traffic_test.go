package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	modelgatewaytraffic "github.com/QuantumNous/new-api/pkg/modelgateway/traffic"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelGatewayTrafficRecorderRecordsRelayBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	modelgatewaytraffic.ResetForTest()
	t.Cleanup(modelgatewaytraffic.ResetForTest)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyChannelId, 9)
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-test")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyChannelAccountProxyID, 3)
		c.Next()
	})
	router.Use(ModelGatewayTrafficRecorder())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		_, _ = c.Writer.Write([]byte("hello"))
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString("abc"))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)

	summary, err := modelgatewaytraffic.QuerySummary(time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
	require.NoError(t, err)
	require.Equal(t, int64(1), summary.RequestCount)
	require.GreaterOrEqual(t, summary.RequestBytes, int64(3))
	require.Equal(t, int64(5), summary.ResponseBytes)
	require.Equal(t, summary.RequestBytes+summary.ResponseBytes, summary.TotalBytes)
}

func TestModelGatewayTrafficRecorderSkipsHealthProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	modelgatewaytraffic.ResetForTest()
	t.Cleanup(modelgatewaytraffic.ResetForTest)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyHealthProbe, true)
		common.SetContextKey(c, constant.ContextKeyChannelId, 9)
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-test")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		c.Next()
	})
	router.Use(ModelGatewayTrafficRecorder())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		_, _ = c.Writer.Write([]byte("hello"))
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString("abc"))
	router.ServeHTTP(recorder, req)

	summary, err := modelgatewaytraffic.QuerySummary(time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
	require.NoError(t, err)
	require.Equal(t, int64(0), summary.RequestCount)
}
