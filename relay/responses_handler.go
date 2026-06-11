package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	modelgatewayproxy "github.com/QuantumNous/new-api/pkg/modelgateway/proxy"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func responsesRequestFromRelayRequest(request dto.Request) *dto.OpenAIResponsesRequest {
	switch req := request.(type) {
	case *dto.OpenAIResponsesRequest:
		return req
	case *dto.OpenAIResponsesCompactionRequest:
		return &dto.OpenAIResponsesRequest{
			Model:              req.Model,
			Input:              req.Input,
			Instructions:       req.Instructions,
			PreviousResponseID: req.PreviousResponseID,
			Reasoning:          req.Reasoning,
		}
	default:
		return nil
	}
}

func captureResponsesReasoningEffort(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	if info == nil || request == nil || request.Reasoning == nil {
		return
	}
	effort := strings.TrimSpace(request.Reasoning.Effort)
	if effort == "" {
		return
	}
	info.SetRequestReasoningEffort(effort)
}

func applyResponsesCompactBillingModel(info *relaycommon.RelayInfo) (string, func()) {
	if info == nil {
		return "", func() {}
	}
	originModelName := info.OriginModelName
	originPriceData := info.PriceData
	billingModelName := originModelName
	if info.UpstreamModelName != "" {
		billingModelName = ratio_setting.WithCompactModelSuffix(info.UpstreamModelName)
	}
	info.OriginModelName = billingModelName
	return originModelName, func() {
		info.OriginModelName = originModelName
		info.PriceData = originPriceData
	}
}

func applyProxyBridgeRequestMode(info *relaycommon.RelayInfo) func() {
	if info == nil {
		return func() {}
	}
	originalMode := info.RelayMode
	originalPath := info.RequestURLPath
	originalFormat := info.FinalRequestRelayFormat
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.RequestURLPath = "/v1/chat/completions"
	info.FinalRequestRelayFormat = types.RelayFormatOpenAI
	return func() {
		info.RelayMode = originalMode
		info.RequestURLPath = originalPath
		info.FinalRequestRelayFormat = originalFormat
	}
}

func handleProxyBridgeResponse(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, bridge *modelgatewayintegration.ProxyBridge, decision modelgatewayintegration.ProxyBridgeDecision) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if info != nil && info.IsStream {
		return handleProxyBridgeStreamResponse(c, info, resp, bridge, decision)
	}
	defer service.CloseResponseBodyGracefully(resp)
	responseBody, err := service.ReadAllWithJSONKeepAlive(c, resp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	result, handled, err := bridge.ConvertChatResponse(c, info, responseBody)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if !handled || result == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("proxy bridge did not handle response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	var response dto.OpenAIResponsesResponse
	if err := common.Unmarshal(result.DownstreamBody, &response); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if response.Usage == nil {
		text := service.ExtractOutputTextFromResponses(&response)
		response.Usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		body, marshalErr := common.Marshal(response)
		if marshalErr != nil {
			return nil, types.NewOpenAIError(marshalErr, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
		}
		result.DownstreamBody = body
	}
	service.IOCopyBytesWithJSONKeepAliveGracefully(c, resp, result.DownstreamBody)
	return response.Usage, nil
}

func handleProxyBridgeStreamResponse(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response, bridge *modelgatewayintegration.ProxyBridge, decision modelgatewayintegration.ProxyBridgeDecision) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	converter, handled, err := bridge.NewChatStreamConverter(c, info)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if !handled || converter == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("proxy bridge did not handle stream response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	var streamErr *types.NewAPIError
	var lastResult *modelgatewayproxy.ConvertStreamResult
	streamSender := newProxyBridgeStreamSender()

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		result, err := converter.Accept(data)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		lastResult = result
		if streamErr = streamSender.Send(c, result); streamErr != nil {
			sr.Stop(streamErr)
			return
		}
		if streamSender.FailedBeforeDelivery() {
			streamErr = streamSender.FailedError()
			if streamErr == nil {
				streamErr = types.NewOpenAIError(fmt.Errorf("proxy bridge stream failed before any usable event was delivered"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			}
			sr.Stop(streamErr)
			return
		}
	})
	if err := helper.InternalRelayAttemptError(c); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeChannelResponseTimeExceeded, http.StatusGatewayTimeout)
	}

	if streamErr != nil {
		return nil, streamErr
	}
	if info != nil && info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone {
		final, err := converter.Finish()
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		lastResult = final
		if streamErr = streamSender.Send(c, final); streamErr != nil {
			return nil, streamErr
		}
	} else if streamSender.DeliveredEventCount() > 0 {
		reason := "unknown"
		if info != nil && info.StreamStatus != nil {
			reason = string(info.StreamStatus.EndReason)
		}
		if info == nil || info.StreamStatus == nil || !info.StreamStatus.IsNormalEnd() {
			common.SetContextKey(c, appconstant.ContextKeyRelayStreamInterrupted, true)
			common.SysLog(fmt.Sprintf("proxy bridge stream interrupted before upstream done: %s", reason))
		} else {
			common.SysLog(fmt.Sprintf("proxy bridge stream ended without upstream done event: %s", reason))
		}
	}
	if streamSender.Failed() {
		common.SetContextKey(c, appconstant.ContextKeyRelayStreamInterrupted, true)
		common.SysLog("proxy bridge stream delivered upstream response.failed event")
	}

	if streamSender.DeliveredEventCount() == 0 {
		return nil, types.NewOpenAIError(fmt.Errorf("proxy bridge stream ended before any usable event was delivered"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	usage := streamSender.Usage()
	if usage.TotalTokens == 0 {
		text := streamSender.UsageFallbackText()
		if lastResult != nil {
			if text == "" {
				text = lastResult.OutputText
			}
			if text == "" {
				text = lastResult.ReasoningText + lastResult.ToolName + lastResult.ToolArguments
			}
		}
		modelName := ""
		promptTokens := 0
		if info != nil {
			modelName = info.UpstreamModelName
			promptTokens = info.GetEstimatePromptTokens()
		}
		usage = service.ResponseText2Usage(c, text, modelName, promptTokens)
	}
	return usage, nil
}

type proxyBridgeStreamSender struct {
	usage                *dto.Usage
	sawResponseCompleted bool
	sawResponseFailed    bool
	failedError          *types.NewAPIError
	deliveredEventCount  int
	bufferedEvents       []proxyBridgeBufferedStreamEvent
	outputText           strings.Builder
	reasoningText        strings.Builder
	toolText             strings.Builder
	toolNamesByItemID    map[string]struct{}
	toolArgsByItemID     map[string]string
}

type proxyBridgeBufferedStreamEvent struct {
	response dto.ResponsesStreamResponse
	data     string
}

func newProxyBridgeStreamSender() *proxyBridgeStreamSender {
	return &proxyBridgeStreamSender{
		usage:             &dto.Usage{},
		toolNamesByItemID: map[string]struct{}{},
		toolArgsByItemID:  map[string]string{},
	}
}

func (s *proxyBridgeStreamSender) Send(c *gin.Context, result *modelgatewayproxy.ConvertStreamResult) *types.NewAPIError {
	if s == nil {
		return nil
	}
	if result == nil {
		return nil
	}
	for _, event := range result.DownstreamEvents {
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(event, &streamResponse); err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if streamResponse.Type == "response.completed" {
			s.sawResponseCompleted = true
			if streamResponse.Response != nil && streamResponse.Response.Usage != nil {
				mergeResponsesStreamUsage(s.usage, streamResponse.Response.Usage)
			}
		}
		if streamResponse.Type == "response.failed" {
			s.sawResponseFailed = true
			s.failedError = newAPIErrorFromResponsesStreamFailure(streamResponse, "proxy bridge stream error: response.failed")
			if streamResponse.Response != nil && streamResponse.Response.Usage != nil {
				mergeResponsesStreamUsage(s.usage, streamResponse.Response.Usage)
			}
		}
		s.captureUsageText(streamResponse)
		if s.FailedBeforeDelivery() {
			continue
		}
		if s.shouldBuffer(streamResponse) {
			s.bufferedEvents = append(s.bufferedEvents, proxyBridgeBufferedStreamEvent{
				response: streamResponse,
				data:     event,
			})
			continue
		}
		s.flushBuffered(c)
		s.send(c, streamResponse, event)
	}
	if result.Usage != nil {
		mergeResponsesStreamUsage(s.usage, result.Usage)
	}
	return nil
}

func (s *proxyBridgeStreamSender) DeliveredEventCount() int {
	if s == nil {
		return 0
	}
	return s.deliveredEventCount
}

func (s *proxyBridgeStreamSender) Failed() bool {
	return s != nil && s.sawResponseFailed
}

func (s *proxyBridgeStreamSender) FailedBeforeDelivery() bool {
	return s != nil && s.sawResponseFailed && s.deliveredEventCount == 0
}

func (s *proxyBridgeStreamSender) FailedError() *types.NewAPIError {
	if s == nil {
		return nil
	}
	return s.failedError
}

func (s *proxyBridgeStreamSender) Usage() *dto.Usage {
	if s == nil || s.usage == nil {
		return &dto.Usage{}
	}
	return s.usage
}

func (s *proxyBridgeStreamSender) UsageFallbackText() string {
	if s == nil {
		return ""
	}
	var text strings.Builder
	appendProxyBridgeUsageTextPart(&text, s.outputText.String())
	appendProxyBridgeUsageTextPart(&text, s.reasoningText.String())
	appendProxyBridgeUsageTextPart(&text, s.toolText.String())
	return text.String()
}

func (s *proxyBridgeStreamSender) captureUsageText(streamResponse dto.ResponsesStreamResponse) {
	if s == nil {
		return
	}
	switch streamResponse.Type {
	case "response.output_text.delta":
		s.outputText.WriteString(streamResponse.Delta)
	case "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		s.reasoningText.WriteString(streamResponse.Delta)
	case "response.function_call_arguments.delta":
		s.appendToolArgumentsDelta(streamResponse.ItemID, streamResponse.Delta)
	case "response.function_call_arguments.done":
		s.appendToolArgumentsSnapshot(streamResponse.ItemID, streamResponse.Delta)
	case dto.ResponsesOutputTypeItemAdded, dto.ResponsesOutputTypeItemDone:
		s.captureToolItemUsageText(streamResponse)
	}
}

func (s *proxyBridgeStreamSender) captureToolItemUsageText(streamResponse dto.ResponsesStreamResponse) {
	if s == nil || streamResponse.Item == nil || streamResponse.Item.Type != "function_call" {
		return
	}
	itemID := strings.TrimSpace(streamResponse.Item.ID)
	if itemID == "" {
		itemID = strings.TrimSpace(streamResponse.ItemID)
	}
	if itemID == "" {
		itemID = strings.TrimSpace(streamResponse.Item.CallId)
	}
	name := strings.TrimSpace(streamResponse.Item.Name)
	if name != "" {
		nameKey := itemID
		if nameKey == "" {
			nameKey = name
		}
		if _, ok := s.toolNamesByItemID[nameKey]; !ok {
			appendProxyBridgeUsageTextPart(&s.toolText, name)
			s.toolNamesByItemID[nameKey] = struct{}{}
		}
	}
	s.appendToolArgumentsSnapshot(itemID, streamResponse.Item.ArgumentsString())
}

func (s *proxyBridgeStreamSender) appendToolArgumentsDelta(itemID string, delta string) {
	if s == nil || delta == "" {
		return
	}
	key := proxyBridgeToolUsageKey(itemID)
	if s.toolArgsByItemID[key] == "" && s.toolText.Len() > 0 {
		s.toolText.WriteByte('\n')
	}
	s.toolText.WriteString(delta)
	s.toolArgsByItemID[key] += delta
}

func (s *proxyBridgeStreamSender) appendToolArgumentsSnapshot(itemID string, args string) {
	if s == nil || args == "" {
		return
	}
	key := proxyBridgeToolUsageKey(itemID)
	prev := s.toolArgsByItemID[key]
	delta := args
	if strings.HasPrefix(args, prev) {
		delta = args[len(prev):]
	}
	if delta == "" {
		return
	}
	if prev == "" && s.toolText.Len() > 0 {
		s.toolText.WriteByte('\n')
	}
	s.toolText.WriteString(delta)
	s.toolArgsByItemID[key] = args
}

func proxyBridgeToolUsageKey(itemID string) string {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return "_unknown"
	}
	return itemID
}

func appendProxyBridgeUsageTextPart(builder *strings.Builder, text string) {
	if builder == nil || strings.TrimSpace(text) == "" {
		return
	}
	if builder.Len() > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString(text)
}

func accountCapabilityPrefersChatCompletions(info *relaycommon.RelayInfo) bool {
	if info == nil || info.ChannelAccountCapability == nil {
		return false
	}
	if info.ChannelAccountCapability.HasResponsesWriteDenied() {
		return true
	}
	return info.ChannelAccountCapability.HasChatCompletionsWriteAllowed()
}

func ensureAccountCapabilityResponsesViaChatPlan(c *gin.Context, info *relaycommon.RelayInfo) func() {
	if !accountCapabilityPrefersChatCompletions(info) {
		return func() {}
	}
	if _, ok := modelgatewayintegration.GetSelectedPlan(c); ok {
		return func() {}
	}
	plan := &core.DispatchPlan{
		SelectedGroup:   firstNonEmptyString(info.UsingGroup, info.TokenGroup),
		ProviderProfile: provider.ProfileStandardOpenAICompatible,
		ProxyMode:       provider.ProxyModeResponsesViaChat,
	}
	if info.ChannelMeta != nil {
		plan.Channel = &model.Channel{
			Id:   info.ChannelId,
			Type: info.ChannelType,
			Name: common.GetContextKeyString(c, appconstant.ContextKeyChannelName),
		}
	}
	modelgatewayintegration.SetSelectedPlan(c, plan)
	return func() {
		modelgatewayintegration.ClearSelectedPlan(c)
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func newAPIErrorFromResponsesStreamFailure(streamResponse dto.ResponsesStreamResponse, fallbackMessage string) *types.NewAPIError {
	if streamResponse.Response != nil {
		if oaiErr := streamResponse.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
			return service.UpstreamOpenAIError(*oaiErr, http.StatusInternalServerError)
		}
	}
	if fallbackMessage == "" {
		fallbackMessage = fmt.Sprintf("responses stream error: %s", streamResponse.Type)
	}
	return types.NewOpenAIError(fmt.Errorf("%s", fallbackMessage), types.ErrorCodeBadResponse, http.StatusInternalServerError)
}

func (s *proxyBridgeStreamSender) shouldBuffer(streamResponse dto.ResponsesStreamResponse) bool {
	if s == nil || s.deliveredEventCount > 0 {
		return false
	}
	switch streamResponse.Type {
	case "response.created", "response.in_progress":
		return true
	default:
		return false
	}
}

func (s *proxyBridgeStreamSender) flushBuffered(c *gin.Context) {
	if s == nil || len(s.bufferedEvents) == 0 {
		return
	}
	for _, event := range s.bufferedEvents {
		s.send(c, event.response, event.data)
	}
	s.bufferedEvents = nil
}

func (s *proxyBridgeStreamSender) send(c *gin.Context, streamResponse dto.ResponsesStreamResponse, data string) {
	if s == nil || data == "" {
		return
	}
	helper.ResponseChunkData(c, streamResponse, data)
	s.deliveredEventCount++
}

func mergeResponsesStreamUsage(dst *dto.Usage, src *dto.Usage) {
	if dst == nil || src == nil {
		return
	}
	if src.PromptTokens != 0 {
		dst.PromptTokens = src.PromptTokens
		if dst.InputTokens == 0 {
			dst.InputTokens = src.PromptTokens
		}
	}
	if src.CompletionTokens != 0 {
		dst.CompletionTokens = src.CompletionTokens
		if dst.OutputTokens == 0 {
			dst.OutputTokens = src.CompletionTokens
		}
	}
	if src.TotalTokens != 0 {
		dst.TotalTokens = src.TotalTokens
	}
	if src.InputTokens != 0 {
		dst.InputTokens = src.InputTokens
		if dst.PromptTokens == 0 {
			dst.PromptTokens = src.InputTokens
		}
	}
	if src.OutputTokens != 0 {
		dst.OutputTokens = src.OutputTokens
		if dst.CompletionTokens == 0 {
			dst.CompletionTokens = src.OutputTokens
		}
	}
	if src.PromptTokensDetails.CachedTokens != 0 {
		dst.PromptTokensDetails.CachedTokens = src.PromptTokensDetails.CachedTokens
	}
	if src.InputTokensDetails != nil {
		if dst.InputTokensDetails == nil {
			details := *src.InputTokensDetails
			dst.InputTokensDetails = &details
		} else {
			mergeResponsesInputTokenDetails(dst.InputTokensDetails, *src.InputTokensDetails)
		}
		mergeResponsesInputTokenDetails(&dst.PromptTokensDetails, *src.InputTokensDetails)
	}
	if src.CompletionTokenDetails.ReasoningTokens != 0 {
		dst.CompletionTokenDetails.ReasoningTokens = src.CompletionTokenDetails.ReasoningTokens
	}
	if src.UsageSemantic != "" {
		dst.UsageSemantic = src.UsageSemantic
	}
	if src.UsageSource != "" {
		dst.UsageSource = src.UsageSource
	}
	if dst.TotalTokens == 0 && (dst.PromptTokens != 0 || dst.CompletionTokens != 0) {
		dst.TotalTokens = dst.PromptTokens + dst.CompletionTokens
	}
}

func mergeResponsesInputTokenDetails(dst *dto.InputTokenDetails, src dto.InputTokenDetails) {
	if dst == nil {
		return
	}
	if src.CachedTokens != 0 {
		dst.CachedTokens = src.CachedTokens
	}
	if src.CachedCreationTokens != 0 {
		dst.CachedCreationTokens = src.CachedCreationTokens
	}
	if src.TextTokens != 0 {
		dst.TextTokens = src.TextTokens
	}
	if src.AudioTokens != 0 {
		dst.AudioTokens = src.AudioTokens
	}
	if src.ImageTokens != 0 {
		dst.ImageTokens = src.ImageTokens
	}
}

func ResponsesHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		switch info.ApiType {
		case appconstant.APITypeOpenAI, appconstant.APITypeCodex:
		default:
			return types.NewErrorWithStatusCode(
				fmt.Errorf("unsupported endpoint %q for api type %d", "/v1/responses/compact", info.ApiType),
				types.ErrorCodeInvalidRequest,
				http.StatusBadRequest,
				types.ErrOptionWithSkipRetry(),
			)
		}
	}

	responsesReq := responsesRequestFromRelayRequest(info.Request)
	if responsesReq == nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("invalid request type, expected dto.OpenAIResponsesRequest or dto.OpenAIResponsesCompactionRequest, got %T", info.Request),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	captureResponsesReasoningEffort(info, responsesReq)

	request, err := common.DeepCopy(responsesReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled {
		info.ResponsesUsageInfo = relaycommon.BuildResponsesUsageInfo(request)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	var requestBody io.Reader
	proxyBridge := modelgatewayintegration.NewProxyBridge(nil)
	restoreCapabilityBridgePlan := ensureAccountCapabilityResponsesViaChatPlan(c, info)
	proxyDecision := proxyBridge.Resolve(c, info)
	if !proxyDecision.Enabled {
		restoreCapabilityBridgePlan()
		restoreCapabilityBridgePlan = func() {}
	}
	defer restoreCapabilityBridgePlan()
	restoreProxyMode := func() {}
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
		requestBody, err = sanitizedOpenAIUpstreamBodyReader(c, info, storage)
		if err != nil {
			return types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
	} else if proxyDecision.Enabled {
		bridgeResult, _, err := proxyBridge.ConvertResponsesRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		if bridgeResult == nil || len(bridgeResult.UpstreamBody) == 0 {
			return types.NewError(fmt.Errorf("proxy bridge produced empty request"), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		bridgeResult.UpstreamBody, err = sanitizeOpenAIUpstreamJSONData(info, bridgeResult.UpstreamBody)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		info.AppendRequestConversion(types.RelayFormatOpenAI)
		restoreProxyMode = applyProxyBridgeRequestMode(info)
		requestBody = bytes.NewBuffer(bridgeResult.UpstreamBody)
	} else {
		convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI Responses API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}
		jsonData, err = sanitizeOpenAIUpstreamJSONData(info, jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		if common.DebugEnabled {
			println("requestBody: ", string(jsonData))
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	restoreProxyMode()
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)

		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	var usage any
	if proxyDecision.Enabled {
		usage, newAPIError = handleProxyBridgeResponse(c, info, httpResp, proxyBridge, proxyDecision)
	} else {
		usage, newAPIError = adaptor.DoResponse(c, httpResp, info)
	}
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	usageDto := usage.(*dto.Usage)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		originModelName, restoreCompactBillingState := applyResponsesCompactBillingModel(info)
		_, err := helper.ModelPriceHelper(c, info, info.GetEstimatePromptTokens(), &types.TokenCountMeta{})
		if err != nil {
			restoreCompactBillingState()
			return types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithSkipRetry(), types.ErrOptionWithStatusCode(http.StatusBadRequest))
		}
		info.OriginModelName = originModelName
		service.PostTextConsumeQuota(c, info, usageDto, nil)

		restoreCompactBillingState()
		return nil
	}

	if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
		service.PostAudioConsumeQuota(c, info, usageDto, "")
	} else {
		service.PostTextConsumeQuota(c, info, usageDto, nil)
	}
	return nil
}
