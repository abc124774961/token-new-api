package helper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const RelayAttemptCancelReasonFirstByteTimeout = modelgatewaycore.RelayAttemptCancelReasonFirstByteTimeout

type RelayAttemptControl struct {
	ctx          context.Context
	mu           sync.RWMutex
	cancelReason string
	reasonReady  chan struct{}
	reasonOnce   sync.Once
}

func NewRelayAttemptControl(ctx context.Context) *RelayAttemptControl {
	if ctx == nil {
		ctx = context.Background()
	}
	return &RelayAttemptControl{ctx: ctx, reasonReady: make(chan struct{})}
}

func FlushWriter(c *gin.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("flush panic recovered: %v", r)
		}
	}()

	if c == nil || c.Writer == nil {
		return nil
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return errors.New("streaming error: flusher not found")
	}

	flusher.Flush()
	return nil
}

func SetEventStreamHeaders(c *gin.Context) {
	// 检查是否已经设置过头部
	if _, exists := c.Get("event_stream_headers_set"); exists {
		return
	}

	// 设置标志，表示头部已经设置过
	c.Set("event_stream_headers_set", true)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
}

func MarkRelayResponseStarted(c *gin.Context) {
	if c == nil {
		return
	}
	MarkRelayDownstreamStarted(c)
	service.MarkChannelFirstByteObserved(c)
	common.SetContextKey(c, constant.ContextKeyRelayResponseStarted, true)
}

func MarkRelayDownstreamStarted(c *gin.Context) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyRelayDownstreamStarted, true)
}

func RelayDownstreamStarted(c *gin.Context) bool {
	return common.GetContextKeyBool(c, constant.ContextKeyRelayDownstreamStarted)
}

func SetRelayAttemptControl(c *gin.Context, control *RelayAttemptControl) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyRelayAttemptControl, control)
}

func ClearRelayAttemptControl(c *gin.Context) {
	if c == nil {
		return
	}
	common.SetContextKey(c, constant.ContextKeyRelayAttemptControl, nil)
}

func RelayAttemptControlFromContext(c *gin.Context) (*RelayAttemptControl, bool) {
	if c == nil {
		return nil, false
	}
	control, ok := common.GetContextKeyType[*RelayAttemptControl](c, constant.ContextKeyRelayAttemptControl)
	return control, ok && control != nil
}

func RelayAttemptContext(c *gin.Context) (context.Context, bool) {
	control, ok := RelayAttemptControlFromContext(c)
	if !ok || control == nil || control.ctx == nil {
		return nil, false
	}
	return control.ctx, true
}

func RelayAttemptCancelReasonReady(c *gin.Context) <-chan struct{} {
	control, ok := RelayAttemptControlFromContext(c)
	if !ok || control == nil {
		return nil
	}
	return control.CancelReasonReady()
}

func (control *RelayAttemptControl) SetCancelReason(reason string) {
	if control == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	control.mu.Lock()
	control.cancelReason = reason
	control.mu.Unlock()
	if reason != "" {
		control.reasonOnce.Do(func() {
			close(control.reasonReady)
		})
	}
}

func (control *RelayAttemptControl) CancelReason() string {
	if control == nil {
		return ""
	}
	control.mu.RLock()
	defer control.mu.RUnlock()
	return control.cancelReason
}

func (control *RelayAttemptControl) CancelReasonReady() <-chan struct{} {
	if control == nil {
		return nil
	}
	return control.reasonReady
}

func RelayAttemptCancelReason(c *gin.Context) string {
	control, ok := RelayAttemptControlFromContext(c)
	if !ok {
		return ""
	}
	return control.CancelReason()
}

func RelayAttemptCanceledFor(c *gin.Context, reason string) bool {
	return RelayAttemptCancelReason(c) == strings.TrimSpace(reason)
}

func InternalRelayAttemptError(c *gin.Context) error {
	reason := RelayAttemptCancelReason(c)
	if reason == "" {
		return nil
	}
	return fmt.Errorf("relay attempt canceled: %s", reason)
}

func ClaudeData(c *gin.Context, resp dto.ClaudeResponse) error {
	MarkRelayResponseStarted(c)
	jsonData, err := common.Marshal(resp)
	if err != nil {
		common.SysError("error marshalling stream response: " + err.Error())
	} else {
		c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
		c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonData)})
	}
	_ = FlushWriter(c)
	return nil
}

func ClaudeChunkData(c *gin.Context, resp dto.ClaudeResponse, data string) {
	MarkRelayResponseStarted(c)
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("data: %s\n", data)})
	_ = FlushWriter(c)
}

func ResponseChunkData(c *gin.Context, resp dto.ResponsesStreamResponse, data string) {
	MarkRelayResponseStarted(c)
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("data: %s", data)})
	_ = FlushWriter(c)
}

func StringData(c *gin.Context, str string) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	MarkRelayResponseStarted(c)
	c.Render(-1, common.CustomEvent{Data: "data: " + str})
	return FlushWriter(c)
}

func PingData(c *gin.Context) error {
	if c == nil || c.Writer == nil {
		return errors.New("context or writer is nil")
	}

	if c.Request != nil && c.Request.Context().Err() != nil {
		return fmt.Errorf("request context done: %w", c.Request.Context().Err())
	}

	if ShouldSuppressPreFirstBytePing(c) {
		return nil
	}
	if _, err := c.Writer.Write([]byte(": PING\n\n")); err != nil {
		return fmt.Errorf("write ping data failed: %w", err)
	}
	MarkRelayDownstreamStarted(c)
	return FlushWriter(c)
}

func ShouldSuppressPreFirstBytePing(c *gin.Context) bool {
	if c == nil || RelayDownstreamStarted(c) || common.GetContextKeyBool(c, constant.ContextKeyRelayResponseStarted) {
		return false
	}
	_, ok := RelayAttemptControlFromContext(c)
	return ok
}

func ObjectData(c *gin.Context, object interface{}) error {
	if object == nil {
		return errors.New("object is nil")
	}
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	return StringData(c, string(jsonData))
}

func Done(c *gin.Context) {
	_ = StringData(c, "[DONE]")
}

func WssString(c *gin.Context, ws *websocket.Conn, str string) error {
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", str))
	return ws.WriteMessage(1, []byte(str))
}

func WssObject(c *gin.Context, ws *websocket.Conn, object interface{}) error {
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", jsonData))
	return ws.WriteMessage(1, jsonData)
}

func WssError(c *gin.Context, ws *websocket.Conn, openaiError types.OpenAIError) {
	if ws == nil {
		return
	}
	errorObj := &dto.RealtimeEvent{
		Type:    "error",
		EventId: GetLocalRealtimeID(c),
		Error:   &openaiError,
	}
	_ = WssObject(c, ws, errorObj)
}

func GetResponseID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("chatcmpl-%s", logID)
}

func GetLocalRealtimeID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("evt_%s", logID)
}

func GenerateStartEmptyResponse(id string, createAt int64, model string, systemFingerprint *string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: systemFingerprint,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role:    "assistant",
					Content: common.GetPointer(""),
				},
			},
		},
	}
}

func GenerateStopResponse(id string, createAt int64, model string, finishReason string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				FinishReason: &finishReason,
			},
		},
	}
}

func GenerateFinalUsageResponse(id string, createAt int64, model string, usage dto.Usage) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices:           make([]dto.ChatCompletionsStreamResponseChoice, 0),
		Usage:             &usage,
	}
}
