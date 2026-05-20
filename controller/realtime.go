package controller

import (
	"github.com/QuantumNous/new-api/pkg/realtime"
	"github.com/gin-gonic/gin"
)

var defaultRealtimeServer = realtime.NewWebSocketServer(realtime.DefaultHub())

func RealtimeWebSocket(c *gin.Context) {
	defaultRealtimeServer.Serve(c)
}
