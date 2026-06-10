package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSessionRoutingKeyFromGinIgnoresUserMetadata(t *testing.T) {
	ctx := newSessionRoutingTestContext(`{"metadata":{"user_id":"user-1001","userId":"user-1001"}}`)

	require.Empty(t, SessionRoutingKeyFromGin(ctx))
}

func TestSessionRoutingKeyFromGinUsesSessionSignal(t *testing.T) {
	ctx := newSessionRoutingTestContext(`{"metadata":{"conversation_id":"conv-1001"}}`)

	key := SessionRoutingKeyFromGin(ctx)
	require.NotEmpty(t, key)
	require.True(t, strings.HasPrefix(key, "session:body.metadata.conversation_id:"))
}

func newSessionRoutingTestContext(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	return ctx
}
