package router_test

import (
	"testing"

	"github.com/QuantumNous/new-api/router"
	classic "github.com/QuantumNous/new-api/web/classic"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayRouterOnlyRegistersRelayRoutes(t *testing.T) {
	t.Parallel()

	engine := gin.New()
	router.SetGatewayRouter(engine)
	routes := routeIndex(engine)

	require.Contains(t, routes, "POST /v1/chat/completions")
	require.Contains(t, routes, "POST /v1/responses")
	require.Contains(t, routes, "POST /v1/responses/compact")
	require.Contains(t, routes, "POST /v1/images/generations")
	require.Contains(t, routes, "POST /v1/images/edits")
	require.Contains(t, routes, "POST /v1/edits")
	require.Contains(t, routes, "POST /v1/images/variations")
	require.Contains(t, routes, "POST /kling/v1/videos/text2video")
	require.Contains(t, routes, "POST /jimeng/")
	require.NotContains(t, routes, "GET /api/user/self")
	require.NotContains(t, routes, "GET /api/status")
}

func TestWebServiceRouterDoesNotRegisterRelayRoutes(t *testing.T) {
	t.Parallel()

	engine := gin.New()
	router.SetWebServiceRouter(engine, classic.ThemeAssets())
	routes := routeIndex(engine)

	require.Contains(t, routes, "GET /api/status")
	require.Contains(t, routes, "GET /api/user/self")
	require.NotContains(t, routes, "POST /v1/chat/completions")
	require.NotContains(t, routes, "POST /v1/images/generations")
}

func TestFullRouterKeepsLegacyCombinedRoutes(t *testing.T) {
	t.Parallel()

	engine := gin.New()
	router.SetRouter(engine, classic.ThemeAssets())
	routes := routeIndex(engine)

	require.Contains(t, routes, "GET /api/status")
	require.Contains(t, routes, "POST /v1/chat/completions")
	require.Contains(t, routes, "POST /v1/images/generations")
}

func routeIndex(engine *gin.Engine) map[string]struct{} {
	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	return routes
}
