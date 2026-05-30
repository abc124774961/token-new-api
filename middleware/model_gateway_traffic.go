package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	modelgatewaytraffic "github.com/QuantumNous/new-api/pkg/modelgateway/traffic"
	"github.com/gin-gonic/gin"
)

type modelGatewayTrafficResponseWriter struct {
	gin.ResponseWriter
	responseBytes int64
}

func (w *modelGatewayTrafficResponseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	if n > 0 {
		w.responseBytes += int64(n)
	}
	return n, err
}

func (w *modelGatewayTrafficResponseWriter) WriteString(data string) (int, error) {
	n, err := w.ResponseWriter.WriteString(data)
	if n > 0 {
		w.responseBytes += int64(n)
	}
	return n, err
}

func ModelGatewayTrafficRecorder() gin.HandlerFunc {
	return func(c *gin.Context) {
		writer := &modelGatewayTrafficResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		requestBytes := estimateModelGatewayRequestBytes(c.Request)

		c.Next()

		if common.GetContextKeyBool(c, constant.ContextKeyHealthProbe) {
			return
		}
		channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
		if channelID <= 0 {
			return
		}
		modelName := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyOriginalModel))
		if modelName == "" {
			modelName = strings.TrimSpace(c.GetString("original_model"))
		}
		if modelName == "" {
			return
		}
		group := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
		if group == "" || group == "auto" {
			group = strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyTokenGroup))
		}
		modelgatewaytraffic.Record(modelgatewaytraffic.Sample{
			ModelName:     modelName,
			Group:         group,
			ChannelID:     channelID,
			ProxyID:       common.GetContextKeyInt(c, constant.ContextKeyChannelAccountProxyID),
			RequestBytes:  requestBytes,
			ResponseBytes: writer.responseBytes,
		})
	}
}

func estimateModelGatewayRequestBytes(request *http.Request) int64 {
	if request == nil {
		return 0
	}
	total := int64(0)
	if request.ContentLength > 0 {
		total += request.ContentLength
	}
	if request.Method != "" {
		total += int64(len(request.Method))
	}
	if request.URL != nil {
		total += int64(len(request.URL.RequestURI()))
	}
	if request.Proto != "" {
		total += int64(len(request.Proto))
	}
	for key, values := range request.Header {
		total += int64(len(key) + 4)
		for _, value := range values {
			total += int64(len(value))
		}
	}
	return total
}
