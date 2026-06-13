package router

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetRelayRoutesRegistersRootModelsAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	SetRelayRoutes(engine.Group(""))

	routes := make(map[string]struct{})
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	require.Contains(t, routes, "GET /v1/models")
	require.Contains(t, routes, "GET /v1/models/:model")
	require.Contains(t, routes, "GET /models")
	require.Contains(t, routes, "GET /models/:model")
}
