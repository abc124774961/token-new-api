package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayscheduler "github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	relayQueueManagerMu sync.RWMutex
	relayQueueManager   = newRelayQueueManager()
)

const relayStatusClientClosedRequest = 499

func newRelayQueueManager() *modelgatewayscheduler.QueueManager {
	policy := modelgatewayintegration.RuntimePolicySetting()
	var admissionPolicy modelgatewayscheduler.QueueAdmissionPolicy
	if policy.QueueFairness.HighPriorityThreshold > 0 ||
		policy.QueueFairness.HighPriorityExtraDepth > 0 ||
		policy.QueueFairness.HighPriorityReservedDepth > 0 ||
		policy.QueueFairness.AbsoluteMaxDepth > 0 ||
		len(policy.QueueFairness.HighPriorityGroups) > 0 {
		admissionPolicy = modelgatewayscheduler.NewPriorityQueueAdmissionPolicy(modelgatewayscheduler.QueueFairnessOptions{
			HighPriorityGroups:        append([]string(nil), policy.QueueFairness.HighPriorityGroups...),
			HighPriorityThreshold:     policy.QueueFairness.HighPriorityThreshold,
			HighPriorityExtraDepth:    policy.QueueFairness.HighPriorityExtraDepth,
			HighPriorityReservedDepth: policy.QueueFairness.HighPriorityReservedDepth,
			AbsoluteMaxDepth:          policy.QueueFairness.AbsoluteMaxDepth,
		})
	}
	return modelgatewayscheduler.NewQueueManagerWithAdmissionPolicy(
		time.Duration(policy.QueueTimeoutMs)*time.Millisecond,
		policy.QueueMaxDepth,
		admissionPolicy,
	)
}

func resetRelayQueueManager() {
	relayQueueManagerMu.Lock()
	defer relayQueueManagerMu.Unlock()
	relayQueueManager = newRelayQueueManager()
}

func currentRelayQueueManager() *modelgatewayscheduler.QueueManager {
	relayQueueManagerMu.RLock()
	defer relayQueueManagerMu.RUnlock()
	return relayQueueManager
}

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func logRelayRequestTrace(c *gin.Context, info *relaycommon.RelayInfo) {
	if !service.ShouldLogClientRequestTrace(c) {
		return
	}
	trace := map[string]any{
		"stage": "relay_initialized",
	}
	if info != nil {
		trace["relay_format"] = string(info.RelayFormat)
		trace["relay_mode"] = info.RelayMode
		trace["request_model"] = info.RequestModelName
		trace["context_model"] = info.ContextModelName
		trace["origin_model"] = info.OriginModelName
		trace["is_stream"] = info.IsStream
		trace["required_endpoint_type"] = string(requiredEndpointTypeForRelay(info))
		trace["requires_codex_image_tool"] = requiresCodexImageToolForRelay(info)
		if req := responsesRequestForEndpointDetection(info.Request); req != nil {
			trace["responses_tools"] = service.BuildResponsesRequestToolTraceForLog(req)
		}
	}
	trace["client_request"] = service.BuildClientRequestTraceForLog(c)
	logger.LogInfo(c, "relay request trace: "+service.MarshalTraceForLog(trace))
}

func logRelayRetryParamTrace(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) {
	if !service.ShouldLogClientRequestTrace(c) {
		return
	}
	trace := map[string]any{
		"stage": "relay_retry_param",
	}
	if info != nil {
		trace["request_model"] = info.RequestModelName
		trace["context_model"] = info.ContextModelName
		trace["origin_model"] = info.OriginModelName
	}
	if retryParam != nil {
		trace["token_group"] = retryParam.TokenGroup
		trace["model_name"] = retryParam.ModelName
		trace["endpoint_type"] = string(retryParam.EndpointType)
		trace["requires_codex_image_tool"] = retryParam.RequiresCodexImageTool
		trace["retry"] = retryParam.GetRetry()
	}
	logger.LogInfo(c, "relay retry trace: "+service.MarshalTraceForLog(trace))
}

func logRelaySelectedChannelTrace(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam, channel *model.Channel, selectGroup string, locked bool) {
	if !service.ShouldLogClientRequestTrace(c) {
		return
	}
	selectGroup = selectedRelayGroupForTrace(c, info, retryParam, selectGroup)
	trace := map[string]any{
		"stage":          "relay_selected_channel",
		"selected_group": selectGroup,
		"locked_channel": locked,
	}
	if info != nil {
		trace["request_model"] = info.RequestModelName
		trace["context_model"] = info.ContextModelName
		trace["origin_model"] = info.OriginModelName
	}
	modelName := ""
	endpointType := constant.EndpointType("")
	requiresCodexImageTool := false
	if retryParam != nil {
		trace["token_group"] = retryParam.TokenGroup
		trace["retry"] = retryParam.GetRetry()
		modelName = retryParam.ModelName
		endpointType = retryParam.EndpointType
		requiresCodexImageTool = retryParam.RequiresCodexImageTool
		trace["model_name"] = modelName
		trace["endpoint_type"] = string(endpointType)
		trace["requires_codex_image_tool"] = requiresCodexImageTool
	}
	if channel == nil {
		trace["channel"] = nil
	} else {
		traceChannel := channel
		if cachedChannel, err := model.CacheGetChannel(channel.Id); err == nil && cachedChannel != nil {
			traceChannel = cachedChannel
		}
		otherSettings := traceChannel.GetOtherSettings()
		trace["channel"] = map[string]any{
			"id":                               traceChannel.Id,
			"name":                             traceChannel.Name,
			"type":                             traceChannel.Type,
			"supports_endpoint":                service.ChannelSupportsRequiredEndpoint(traceChannel, modelName, endpointType),
			"supports_codex_image_generation":  service.ChannelSupportsCodexImageGenerationTool(traceChannel),
			"supports_required_capabilities":   service.ChannelSupportsRequiredCapabilities(traceChannel, modelName, endpointType, requiresCodexImageTool),
			"codex_compatibility_mode":         otherSettings.CodexCompatibilityMode,
			"codex_supported_tools":            otherSettings.CodexSupportedTools,
			"codex_image_tool_probe_supported": otherSettings.CodexImageGenerationToolSupported,
		}
	}
	logger.LogInfo(c, "relay channel trace: "+service.MarshalTraceForLog(trace))
}

func selectedRelayGroupForTrace(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam, selectedGroup string) string {
	if group := strings.TrimSpace(selectedGroup); group != "" && group != "auto" {
		return group
	}
	if info != nil {
		if group := strings.TrimSpace(info.UsingGroup); group != "" && group != "auto" {
			return group
		}
	}
	if group := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUsingGroup)); group != "" && group != "auto" {
		return group
	}
	if group := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyAutoGroup)); group != "" && group != "auto" {
		return group
	}
	if group := strings.TrimSpace(selectedGroup); group != "" {
		return group
	}
	if info != nil && strings.TrimSpace(info.UsingGroup) != "" {
		return strings.TrimSpace(info.UsingGroup)
	}
	if retryParam != nil {
		return strings.TrimSpace(retryParam.TokenGroup)
	}
	return ""
}

func requiredEndpointTypeForRelay(info *relaycommon.RelayInfo) constant.EndpointType {
	if info == nil {
		return ""
	}
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations:
		return constant.EndpointTypeImageGeneration
	case relayconstant.RelayModeImagesEdits:
		return constant.EndpointTypeImageEdit
	case relayconstant.RelayModeResponsesCompact:
		return constant.EndpointTypeOpenAIResponseCompact
	case relayconstant.RelayModeResponses:
		return constant.EndpointTypeOpenAIResponse
	}
	return constant.EndpointTypeOpenAI
}

func requiresCodexImageToolForRelay(info *relaycommon.RelayInfo) bool {
	if info == nil || info.RelayMode != relayconstant.RelayModeResponses {
		return false
	}
	req := responsesRequestForEndpointDetection(info.Request)
	return service.ResponsesRequestRequiresCodexImageGenerationTool(req)
}

func responsesRequestForEndpointDetection(request dto.Request) *dto.OpenAIResponsesRequest {
	switch req := request.(type) {
	case *dto.OpenAIResponsesRequest:
		return req
	default:
		return nil
	}
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	defer service.ReleaseChannelSelectionReservations(c)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("relay error: %s", newAPIError.Error()))
			if relayResponseAlreadyStarted(c) {
				return
			}
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}
	common.SetContextKey(c, constant.ContextKeyRelayInfo, relayInfo)
	if common.GetContextKeyBool(c, constant.ContextKeyHealthProbe) {
		relayInfo.IsChannelTest = true
	}
	logger.LogInfo(c, fmt.Sprintf(
		"relay model trace initialized: format=%s, request_model=%s, context_model=%s, origin_model=%s, path=%s",
		relayFormat,
		relayInfo.RequestModelName,
		relayInfo.ContextModelName,
		relayInfo.OriginModelName,
		c.Request.URL.Path,
	))
	logRelayRequestTrace(c, relayInfo)

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if priceData.FreeModel {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, priceData.QuotaToPreConsume, relayInfo)
		if newAPIError != nil {
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if relayInfo.Billing != nil {
				relayInfo.Billing.Refund(c)
			}
			service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:                    c,
		TokenGroup:             relayInfo.TokenGroup,
		ModelName:              relayInfo.OriginModelName,
		EndpointType:           requiredEndpointTypeForRelay(relayInfo),
		RequiresCodexImageTool: requiresCodexImageToolForRelay(relayInfo),
		Retry:                  common.GetPointer(0),
	}
	logRelayRetryParamTrace(c, relayInfo, retryParam)
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil
	var finalAttemptReported bool
	var lastModelGatewayPlan *modelgatewaycore.DispatchPlan
	var lastModelGatewayChannel *model.Channel
	var lastConcurrencyLimitError *types.NewAPIError
	var lastConcurrencyLimitChannel *model.Channel
	var lastConcurrencyLimitPlan *modelgatewaycore.DispatchPlan
	var lastConcurrencyLimitLease *service.ChannelConcurrencyLease

	for ; retryParam.HasBudget(common.RetryTimes); retryParam.IncreaseRetry() {
		relayInfo.RetryIndex = relayAttemptIndex(c)
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			if channelErr.GetErrorCode() == types.ErrorCodeChannelConcurrencyLimit {
				lastConcurrencyLimitError = channelErr
				if canFailover, forceNextAutoGroup := service.GetConcurrencyLimitFailoverPlan(retryParam); canFailover {
					if forceNextAutoGroup {
						common.SetContextKey(c, constant.ContextKeyForceNextAutoGroup, true)
					}
					retryParam.AllowExtraRetry(1)
					continue
				}
			}
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			break
		}
		channelSetting := channel.GetSetting()
		if contextSetting, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting); ok {
			channelSetting = contextSetting
		}
		plan := selectedModelGatewayPlan(c)
		if plan != nil {
			lastModelGatewayPlan = plan
			lastModelGatewayChannel = channel
		}
		concurrencyResult := currentRelayQueueManager().AcquireWithOptions(c.Request.Context(), plan, channel.Id, channelSetting, relayQueueAcquireOptions(plan))
		queueWait := concurrencyResult.WaitTime
		service.ReleaseChannelSelectionReservation(c, channel.Id)
		concurrencyLease := concurrencyResult.Lease
		if concurrencyResult.Status == modelgatewayscheduler.QueueAcquireRejected {
			clientAbort := relayRequestContextCanceled(c) || relayClientAborted(c, relayInfo, nil)
			limit := 0
			active := 0
			if concurrencyLease != nil {
				limit = concurrencyLease.Limit
				active = concurrencyLease.ActiveAtHit()
			}
			if clientAbort {
				newAPIError = types.NewErrorWithStatusCode(context.Canceled, types.ErrorCodeDoRequestFailed, relayStatusClientClosedRequest, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(
					fmt.Errorf("channel #%d reached configured max concurrency %d", channel.Id, limit),
					types.ErrorCodeChannelConcurrencyLimit,
					http.StatusTooManyRequests,
				)
			}
			lastConcurrencyLimitError = newAPIError
			lastConcurrencyLimitChannel = channel
			lastConcurrencyLimitPlan = plan
			lastConcurrencyLimitLease = concurrencyLease
			if relayInfo.ChannelMeta == nil {
				relayInfo.InitChannelMeta(c)
			}
			addUsedChannel(c, channel.Id)
			willRetry := false
			if !clientAbort {
				service.MarkChannelSelectionSkipped(c, channel.Id)
				var forceNextAutoGroup bool
				willRetry, forceNextAutoGroup = service.GetConcurrencyLimitFailoverPlan(retryParam)
				if willRetry {
					if forceNextAutoGroup {
						common.SetContextKey(c, constant.ContextKeyForceNextAutoGroup, true)
					}
					retryParam.AllowExtraRetry(1)
				}
				traceChannelFailure(c, *newChannelErrorFromSelectedChannel(c, channel), newAPIError, !willRetry)
			}
			reportModelGatewayAttempt(c, relayInfo, retryParam, channel, newAPIError, time.Since(relayInfo.StartTime), modelGatewayAttemptFlow{
				ErrorCategory:              lo.Ternary(clientAbort, modelgatewaycore.ErrorCategoryClientAborted, modelgatewaycore.ErrorCategoryLocalConcurrencyLimit),
				RetryAction:                lo.Ternary(clientAbort, "client_aborted", lo.Ternary(willRetry, "switch_channel", "stop")),
				WillRetry:                  willRetry,
				ClientAborted:              clientAbort,
				ConcurrencyLimited:         !clientAbort,
				ConfiguredConcurrencyLimit: limit,
				ActiveConcurrency:          active,
				UsedChannels:               append([]string(nil), c.GetStringSlice("use_channel")...),
				QueueWait:                  queueWait,
				RelayTotal:                 time.Since(relayInfo.StartTime),
			})
			if clientAbort || !willRetry {
				finalAttemptReported = true
				break
			}
			continue
		}
		firstByteLease := service.BeginChannelFirstByteWait(c, channel.Id, relayInfo.RequestId, relayInfo.RetryIndex)

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			firstByteLease.Release()
			concurrencyLease.Release()
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		relayStart := time.Now()
		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}
		relayTotal := time.Since(relayStart)
		relayToFirstByte := time.Duration(0)
		if relayInfo.HasSendResponse() && relayInfo.FirstResponseTime.After(relayStart) {
			relayToFirstByte = relayInfo.FirstResponseTime.Sub(relayStart)
		}
		firstByteLease.Release()

		if newAPIError == nil {
			concurrencyLease.Release()
			if relayClientAborted(c, relayInfo, nil) {
				reportModelGatewayAttempt(c, relayInfo, retryParam, channel, types.NewErrorWithStatusCode(
					context.Canceled,
					types.ErrorCodeDoRequestFailed,
					relayStatusClientClosedRequest,
					types.ErrOptionWithSkipRetry(),
				), time.Since(relayInfo.StartTime), modelGatewayAttemptFlow{
					ErrorCategory:    modelgatewaycore.ErrorCategoryClientAborted,
					RetryAction:      "client_aborted",
					ClientAborted:    true,
					UsedChannels:     append([]string(nil), c.GetStringSlice("use_channel")...),
					QueueWait:        queueWait,
					RelayToFirstByte: relayToFirstByte,
					RelayTotal:       relayTotal,
				})
				finalAttemptReported = true
				return
			}
			relayInfo.LastError = nil
			service.ClearChannelFailureAvoidance(channel.Id)
			recordRelayChannelConfigSuccess(c, channel.Id, relayInfo, retryParam)
			service.RecordChannelConcurrencySuccess(channel.Id)
			reportModelGatewayAttempt(c, relayInfo, retryParam, channel, nil, time.Since(relayInfo.StartTime), modelGatewayAttemptFlow{
				RetryAction:      "complete",
				QueueWait:        queueWait,
				RelayToFirstByte: relayToFirstByte,
				RelayTotal:       relayTotal,
			})
			finalAttemptReported = true
			return
		}

		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError
		clientAbort := relayClientAborted(c, relayInfo, newAPIError)
		errorCategory := classifyRelayAttemptError(c, newAPIError)
		overloadSkip := errorCategory == modelgatewaycore.ErrorCategoryOverloadSkip
		flow := modelGatewayAttemptFlow{
			ErrorCategory:      errorCategory,
			ConcurrencyLimited: overloadSkip || service.IsUpstreamConcurrencyLimitError(newAPIError),
			ClientAborted:      clientAbort,
		}
		if (overloadSkip || service.IsUpstreamConcurrencyLimitError(newAPIError)) && !clientAbort {
			service.MarkChannelSelectionSkipped(c, channel.Id)
		}
		if service.IsUpstreamConcurrencyLimitError(newAPIError) && !clientAbort {
			flow.ActiveConcurrency = concurrencyLease.CurrentActive()
			if concurrencyLease.Limit > 0 {
				flow.ConfiguredConcurrencyLimit = concurrencyLease.Limit
			}
			learned := service.LearnChannelConcurrencyLimitWithResult(c, channel.Id, flow.ActiveConcurrency, newAPIError)
			flow.LearnedConcurrencyLimit = learned.LearnedLimit
			flow.LearnedConcurrencyLimitChanged = learned.Changed
			if flow.ConfiguredConcurrencyLimit <= 0 && learned.PreviousLimit > 0 {
				flow.ConfiguredConcurrencyLimit = learned.PreviousLimit
			}
		}
		if service.IsBalanceInsufficientError(newAPIError) && !clientAbort {
			service.MarkChannelBalanceSkipped(c, channel.Id)
			service.MarkChannelBalanceInsufficient(channel.Id)
			flow.BalanceInsufficient = true
		}
		concurrencyLease.Release()

		willRetry := shouldRetry(c, newAPIError, retryParam, common.RetryTimes-retryParam.GetRetry()) && !clientAbort
		flow.WillRetry = willRetry
		flow.RetryAction = retryActionForAttempt(c, newAPIError, willRetry)
		if !clientAbort {
			processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError, !willRetry)
		}
		flow.UsedChannels = append([]string(nil), c.GetStringSlice("use_channel")...)
		flow.QueueWait = queueWait
		flow.RelayToFirstByte = relayToFirstByte
		flow.RelayTotal = relayTotal
		reportModelGatewayAttempt(c, relayInfo, retryParam, channel, newAPIError, time.Since(relayInfo.StartTime), flow)
		if !willRetry {
			finalAttemptReported = true
		}

		if !willRetry {
			break
		}
	}

	if lastConcurrencyLimitError != nil && !finalAttemptReported && (newAPIError == nil || newAPIError.GetErrorCode() == types.ErrorCodeGetChannelFailed) {
		newAPIError = lastConcurrencyLimitError
		if lastConcurrencyLimitChannel != nil {
			if lastConcurrencyLimitPlan != nil {
				modelgatewayintegration.SetSelectedPlan(c, lastConcurrencyLimitPlan)
			}
			limit := 0
			active := 0
			if lastConcurrencyLimitLease != nil {
				limit = lastConcurrencyLimitLease.Limit
				active = lastConcurrencyLimitLease.ActiveAtHit()
			}
			addUsedChannel(c, lastConcurrencyLimitChannel.Id)
			traceChannelFailure(c, *newChannelErrorFromSelectedChannel(c, lastConcurrencyLimitChannel), newAPIError, true)
			reportModelGatewayAttempt(c, relayInfo, retryParam, lastConcurrencyLimitChannel, newAPIError, time.Since(relayInfo.StartTime), modelGatewayAttemptFlow{
				ErrorCategory:              modelgatewaycore.ErrorCategoryLocalConcurrencyLimit,
				RetryAction:                "stop",
				ConcurrencyLimited:         true,
				ConfiguredConcurrencyLimit: limit,
				ActiveConcurrency:          active,
				UsedChannels:               append([]string(nil), c.GetStringSlice("use_channel")...),
			})
			finalAttemptReported = true
		}
	}

	reportModelGatewayClientAbortIfNeeded(c, relayInfo, retryParam, lastModelGatewayPlan, lastModelGatewayChannel, finalAttemptReported)

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if newAPIError != nil {
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func relayAttemptIndex(c *gin.Context) int {
	if c == nil {
		return 0
	}
	return len(c.GetStringSlice("use_channel"))
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		channel := &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}
		logRelaySelectedChannelTrace(c, info, retryParam, channel, info.TokenGroup, true)
		return channel, nil
	}
	selection, selectionErr := modelgatewayintegration.DefaultChannelSelectionWrapper().Select(c, retryParam)

	if selectionErr != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", retryParam.TokenGroup, info.OriginModelName, selectionErr.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	var channel *model.Channel
	selectGroup := retryParam.TokenGroup
	if selection != nil {
		channel = selection.Channel
		selectGroup = selection.Group
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（retry）", selectGroup, info.OriginModelName), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}

	helper.ApplySelectedGroupRatio(c, info, selectGroup)
	if apiErr := reserveSelectedGroupBilling(c, info); apiErr != nil {
		return nil, apiErr
	}
	logRelaySelectedChannelTrace(c, info, retryParam, channel, selectGroup, false)
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

func reserveSelectedGroupBilling(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	if info == nil || info.Billing == nil || info.PriceData.FreeModel {
		return nil
	}
	targetQuota := selectedGroupPreConsumeTarget(info)
	if targetQuota <= info.Billing.GetPreConsumedQuota() {
		return nil
	}
	if err := info.Billing.Reserve(targetQuota); err != nil {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("selected group billing reserve failed: %w", err),
			types.ErrorCodeInsufficientUserQuota,
			http.StatusForbidden,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	info.PriceData.QuotaToPreConsume = targetQuota
	return nil
}

func selectedGroupPreConsumeTarget(info *relaycommon.RelayInfo) int {
	if info == nil {
		return 0
	}
	if snap := info.TieredBillingSnapshot; snap != nil {
		return snap.EstimatedQuotaAfterGroup
	}
	priceData := info.PriceData
	return priceData.QuotaToPreConsume
}

func selectedModelGatewayPlan(c *gin.Context) *modelgatewaycore.DispatchPlan {
	plan, _ := modelgatewayintegration.GetSelectedPlan(c)
	return plan
}

func relayQueueAcquireOptions(plan *modelgatewaycore.DispatchPlan) modelgatewayscheduler.QueueAcquireOptions {
	if plan == nil {
		return modelgatewayscheduler.QueueAcquireOptions{}
	}
	return modelgatewayscheduler.QueueAcquireOptions{
		Group:      plan.SelectedGroup,
		Priority:   plan.QueuePriority,
		RuntimeKey: plan.RuntimeKey,
	}
}

func newChannelErrorFromSelectedChannel(c *gin.Context, channel *model.Channel) *types.ChannelError {
	if channel == nil {
		return types.NewChannelError(0, 0, "", false, "", false)
	}
	return types.NewChannelError(
		channel.Id,
		channel.Type,
		channel.Name,
		channel.ChannelInfo.IsMultiKey,
		common.GetContextKeyString(c, constant.ContextKeyChannelKey),
		channel.GetAutoBan(),
	)
}

type modelGatewayAttemptFlow struct {
	ErrorCategory                  string
	RetryAction                    string
	WillRetry                      bool
	ClientAborted                  bool
	ConcurrencyLimited             bool
	BalanceInsufficient            bool
	EmptyOutput                    bool
	ExperienceIssue                string
	ActiveConcurrency              int
	ConfiguredConcurrencyLimit     int
	LearnedConcurrencyLimit        int
	LearnedConcurrencyLimitChanged bool
	UsedChannels                   []string
	QueueWait                      time.Duration
	RelayToFirstByte               time.Duration
	RelayTotal                     time.Duration
}

func reportModelGatewayAttempt(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam, channel *model.Channel, apiErr *types.NewAPIError, duration time.Duration, flow modelGatewayAttemptFlow) {
	if c == nil || info == nil || channel == nil {
		return
	}
	plan := selectedModelGatewayPlan(c)
	if plan == nil {
		return
	}
	wrapper := modelgatewayintegration.DefaultChannelSelectionWrapper()
	if wrapper == nil || wrapper.Facade == nil {
		return
	}
	selectedGroup := info.UsingGroup
	if plan != nil && plan.SelectedGroup != "" {
		selectedGroup = plan.SelectedGroup
	}
	if selectedGroup == "" && retryParam != nil {
		selectedGroup = retryParam.TokenGroup
	}
	modelName := info.OriginModelName
	if retryParam != nil && retryParam.ModelName != "" {
		modelName = retryParam.ModelName
	}
	result := &modelgatewaycore.AttemptResult{
		RequestID:         info.RequestId,
		UserID:            info.UserId,
		TokenID:           info.TokenId,
		AttemptIndex:      info.RetryIndex,
		ChannelID:         channel.Id,
		ChannelName:       channel.Name,
		RequestedGroup:    info.TokenGroup,
		SelectedGroup:     selectedGroup,
		ModelName:         modelName,
		EndpointType:      requiredEndpointTypeForRelay(info),
		Success:           apiErr == nil,
		Duration:          duration,
		TTFT:              relayTTFT(info),
		QueueWait:         flow.QueueWait,
		RelayToFirstByte:  flow.RelayToFirstByte,
		RelayTotal:        flow.RelayTotal,
		StreamInterrupted: flow.ClientAborted || relayStreamInterrupted(c) || (apiErr != nil && relayResponseAlreadyStarted(c)),
	}
	if plan != nil {
		result.Key = plan.RuntimeKey
		if result.RequestedGroup == "" {
			result.RequestedGroup = plan.RequestedGroup
		}
	}
	if result.StreamInterrupted {
		result.Success = false
	}
	if apiErr != nil {
		result.StatusCode = apiErr.StatusCode
		result.ErrorCode = string(apiErr.GetErrorCode())
		result.ErrorType = string(apiErr.GetErrorType())
		result.ErrorMessage = apiErr.MaskSensitiveError()
	}
	result.ErrorCategory = flow.ErrorCategory
	result.WillRetry = flow.WillRetry
	result.RetryAction = flow.RetryAction
	result.ClientAborted = flow.ClientAborted
	result.ConcurrencyLimited = flow.ConcurrencyLimited
	result.BalanceInsufficient = flow.BalanceInsufficient
	result.EmptyOutput = flow.EmptyOutput || relayEmptyOutput(c)
	result.ExperienceIssue = relayExperienceIssue(c, flow.ExperienceIssue)
	result.ActiveConcurrency = flow.ActiveConcurrency
	result.ConfiguredConcurrencyLimit = flow.ConfiguredConcurrencyLimit
	result.LearnedConcurrencyLimit = flow.LearnedConcurrencyLimit
	result.LearnedConcurrencyLimitChanged = flow.LearnedConcurrencyLimitChanged
	result.UsedChannels = append([]string(nil), flow.UsedChannels...)
	wrapper.Facade.Report(c, result)
}

func relayEmptyOutput(c *gin.Context) bool {
	return common.GetContextKeyBool(c, constant.ContextKeyRelayEmptyOutput)
}

func relayExperienceIssue(c *gin.Context, fallback string) string {
	if value := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyRelayExperienceIssue)); value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
}

func classifyRelayAttemptError(c *gin.Context, apiErr *types.NewAPIError) string {
	if apiErr == nil {
		return ""
	}
	if relayClientAborted(c, nil, apiErr) {
		return modelgatewaycore.ErrorCategoryClientAborted
	}
	if relayStreamInterrupted(c) || relayResponseAlreadyStarted(c) {
		return modelgatewaycore.ErrorCategoryStreamInterrupted
	}
	if apiErr.GetErrorCode() == types.ErrorCodeChannelConcurrencyLimit {
		return modelgatewaycore.ErrorCategoryLocalConcurrencyLimit
	}
	if service.IsBalanceInsufficientError(apiErr) {
		return modelgatewaycore.ErrorCategoryBalanceOrQuota
	}
	if isRelayOverloadSkipError(apiErr) {
		return modelgatewaycore.ErrorCategoryOverloadSkip
	}
	if isUpstreamRateLimitLikeError(apiErr) {
		return modelgatewaycore.ErrorCategoryRateLimit
	}
	if isRelayAuthConfigError(apiErr) {
		return modelgatewaycore.ErrorCategoryAuthConfigError
	}
	if shouldFailoverOnUnsupportedCapability(c, apiErr) {
		return modelgatewaycore.ErrorCategoryUnsupportedCapability
	}
	if apiErr.StatusCode >= http.StatusInternalServerError {
		return modelgatewaycore.ErrorCategoryServerError
	}
	if isUpstreamFailoverCandidate(apiErr) {
		return modelgatewaycore.ErrorCategoryUpstreamError
	}
	return modelgatewaycore.ErrorCategoryUnknown
}

func retryActionForAttempt(c *gin.Context, apiErr *types.NewAPIError, willRetry bool) string {
	if apiErr == nil {
		return "complete"
	}
	if relayClientAborted(c, nil, apiErr) {
		return "client_aborted"
	}
	if !willRetry {
		return "stop"
	}
	if isRelayOverloadSkipError(apiErr) ||
		service.IsUpstreamConcurrencyLimitError(apiErr) ||
		apiErr.GetErrorCode() == types.ErrorCodeChannelConcurrencyLimit ||
		shouldFailoverToAlternativeChannel(c, apiErr) {
		return "switch_channel"
	}
	return "retry"
}

func relayTTFT(info *relaycommon.RelayInfo) time.Duration {
	if info == nil || !info.HasSendResponse() {
		return 0
	}
	return info.FirstResponseTime.Sub(info.StartTime)
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryParam *service.RetryParam, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if relayResponseAlreadyStarted(c) {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryParam != nil && shouldFailoverToAlternativeChannel(c, openaiErr) {
		canFailover, forceNextAutoGroup := service.GetChannelFailoverPlan(retryParam)
		if !canFailover {
			return false
		}
		retryParam.AllowExtraRetry(1)
		if forceNextAutoGroup {
			common.SetContextKey(c, constant.ContextKeyForceNextAutoGroup, true)
		}
		return true
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func relayResponseAlreadyStarted(c *gin.Context) bool {
	return common.GetContextKeyBool(c, constant.ContextKeyRelayResponseStarted)
}

func relayStreamInterrupted(c *gin.Context) bool {
	return common.GetContextKeyBool(c, constant.ContextKeyRelayStreamInterrupted)
}

func relayClientAborted(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) bool {
	if info != nil && info.StreamStatus != nil && info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone {
		return true
	}
	return apiErr != nil && errors.Is(apiErr, context.Canceled)
}

func relayRequestContextCanceled(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	err := c.Request.Context().Err()
	return errors.Is(err, context.Canceled)
}

func reportModelGatewayClientAbortIfNeeded(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam, plan *modelgatewaycore.DispatchPlan, channel *model.Channel, finalAlreadyReported bool) {
	if finalAlreadyReported || !relayClientAborted(c, info, nil) {
		return
	}
	if plan == nil {
		return
	}
	if channel == nil {
		channel = plan.Channel
	}
	if channel == nil {
		return
	}
	modelgatewayintegration.SetSelectedPlan(c, plan)
	reportModelGatewayAttempt(c, info, retryParam, channel, types.NewErrorWithStatusCode(
		context.Canceled,
		types.ErrorCodeDoRequestFailed,
		relayStatusClientClosedRequest,
		types.ErrOptionWithSkipRetry(),
	), time.Since(info.StartTime), modelGatewayAttemptFlow{
		ErrorCategory: modelgatewaycore.ErrorCategoryClientAborted,
		RetryAction:   "client_aborted",
		ClientAborted: true,
		UsedChannels:  append([]string(nil), c.GetStringSlice("use_channel")...),
	})
}

func shouldFailoverToAlternativeChannel(c *gin.Context, openaiErr *types.NewAPIError) bool {
	if openaiErr == nil {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if shouldFailoverOnConcurrencyLimit(c, openaiErr) {
		return true
	}
	if shouldFailoverOnUnsupportedCapability(c, openaiErr) {
		return true
	}
	if isUpstreamRateLimitLikeError(openaiErr) {
		return true
	}
	if isUpstreamFailoverCandidate(openaiErr) {
		return true
	}
	code := openaiErr.StatusCode
	if code < 100 || code > 599 {
		return true
	}
	if code >= http.StatusInternalServerError {
		return true
	}
	switch openaiErr.GetErrorCode() {
	case types.ErrorCodeDoRequestFailed,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody:
		return true
	default:
		return false
	}
}

func shouldFailoverOnUnsupportedCapability(c *gin.Context, openaiErr *types.NewAPIError) bool {
	if openaiErr == nil {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	switch openaiErr.StatusCode {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusForbidden, http.StatusUnprocessableEntity:
	default:
		return false
	}
	message := strings.ToLower(openaiErr.Error())
	return strings.Contains(message, "unknown parameter") ||
		strings.Contains(message, "unsupported parameter") ||
		strings.Contains(message, "unsupported tool") ||
		strings.Contains(message, "invalid tool") ||
		strings.Contains(message, "tool") && strings.Contains(message, "not supported") ||
		strings.Contains(message, "image_generation") && strings.Contains(message, "not supported") ||
		strings.Contains(message, "model") && strings.Contains(message, "disabled") ||
		strings.Contains(message, "model_not_found") ||
		strings.Contains(message, "does not exist") ||
		strings.Contains(message, "unsupported endpoint")
}

func shouldFailoverOnConcurrencyLimit(c *gin.Context, openaiErr *types.NewAPIError) bool {
	if openaiErr == nil || openaiErr.StatusCode != http.StatusTooManyRequests {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	return true
}

func isConcurrencyBusyError(openaiErr *types.NewAPIError) bool {
	if openaiErr == nil || openaiErr.StatusCode != http.StatusTooManyRequests {
		return false
	}
	message := strings.ToLower(openaiErr.Error())
	return strings.Contains(message, "concurrency limit exceeded for user") ||
		strings.Contains(message, "too many pending requests")
}

func isUpstreamFailoverCandidate(openaiErr *types.NewAPIError) bool {
	if openaiErr == nil {
		return false
	}
	code := openaiErr.StatusCode
	if code >= http.StatusOK && code < http.StatusMultipleChoices {
		return false
	}
	if openaiErr.GetErrorType() == types.ErrorTypeOpenAIError {
		return true
	}
	switch openaiErr.GetErrorCode() {
	case types.ErrorCodeDoRequestFailed,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponseStatusCode,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody,
		types.ErrorCodeEmptyResponse,
		types.ErrorCodeModelNotFound:
		return true
	default:
		return false
	}
}

func isUpstreamRateLimitLikeError(openaiErr *types.NewAPIError) bool {
	if openaiErr == nil {
		return false
	}
	if openaiErr.GetErrorCode() != types.ErrorCodeBadResponseStatusCode &&
		openaiErr.GetErrorType() != types.ErrorTypeOpenAIError {
		return false
	}
	code := openaiErr.StatusCode
	if code != http.StatusBadRequest && code != http.StatusTooManyRequests {
		return false
	}
	message := strings.ToLower(openaiErr.Error())
	return strings.Contains(message, "rate limit") ||
		strings.Contains(message, "rate_limit") ||
		strings.Contains(message, "quota rate") ||
		strings.Contains(message, "quota limit") ||
		strings.Contains(message, "too many requests") ||
		strings.Contains(message, "retry after") ||
		strings.Contains(message, "后重试") ||
		strings.Contains(message, "限速") ||
		strings.Contains(message, "速率") ||
		strings.Contains(message, "配额限制") ||
		strings.Contains(message, "限速规则")
}

func isRelayOverloadSkipError(apiErr *types.NewAPIError) bool {
	return apiErr != nil &&
		apiErr.StatusCode == http.StatusTooManyRequests &&
		apiErr.GetErrorCode() != types.ErrorCodeChannelConcurrencyLimit
}

func isRelayAuthConfigError(apiErr *types.NewAPIError) bool {
	if apiErr == nil || service.IsBalanceInsufficientError(apiErr) {
		return false
	}
	if apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden {
		return true
	}
	message := strings.ToLower(apiErr.Error())
	return strings.Contains(message, "invalid api key") ||
		strings.Contains(message, "invalid_key") ||
		strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "forbidden") ||
		strings.Contains(message, "access denied") ||
		strings.Contains(message, "access_denied") ||
		strings.Contains(message, "permission denied") ||
		strings.Contains(message, "permission_denied") ||
		strings.Contains(message, "model not allowed") ||
		strings.Contains(message, "provider account forbidden")
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, persistLog bool) {
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	errorCategory := classifyRelayAttemptError(c, err)
	if errorCategory == modelgatewaycore.ErrorCategoryAuthConfigError {
		recordRelayChannelConfigAuthError(c, channelError.ChannelId, err)
	}
	traceChannelFailure(c, channelError, err, persistLog)
	if errorCategory == modelgatewaycore.ErrorCategoryOverloadSkip {
		return
	}
	if service.ShouldDisableChannelForBalance(err) && channelError.AutoBan {
		service.DisableChannelForBalance(channelError)
	}
	if errorCategory != modelgatewaycore.ErrorCategoryAuthConfigError {
		if reason, ok := channelFailureAvoidanceReason(err); ok {
			if avoidance := service.RecordChannelFailureAvoidanceWithContext(channelError.ChannelId, reason, buildChannelFailureAvoidanceContext(c, channelError, err, persistLog)); avoidance != nil && avoidance.ShouldPause {
				gopool.Go(func() {
					service.PauseChannelForError(channelError, avoidance.Until, avoidance.Reason)
				})
			}
		}
		// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
		// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
		if service.ShouldDisableChannel(err) && channelError.AutoBan {
			gopool.Go(func() {
				service.DisableChannel(channelError, err.ErrorWithStatusCode())
			})
		}
	}

	if persistLog && constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := currentRelayLogGroup(c)
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		if upstreamRequest, ok := common.GetContextKey(c, constant.ContextKeyUpstreamRequestInfo); ok {
			other["upstream_request"] = upstreamRequest
		}
		if len(err.Metadata) > 0 {
			other["error_metadata"] = common.JsonRawMessageToString(err.Metadata)
		}
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		appendChannelFailureTraceAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
	}

}

func buildChannelFailureAvoidanceContext(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, finalFailure bool) *service.ChannelFailureAvoidanceContext {
	if err == nil {
		return nil
	}
	failureContext := &service.ChannelFailureAvoidanceContext{
		ChannelName:  channelError.ChannelName,
		ChannelType:  channelError.ChannelType,
		Group:        currentRelayLogGroup(c),
		ErrorType:    string(err.GetErrorType()),
		ErrorCode:    string(err.GetErrorCode()),
		StatusCode:   err.StatusCode,
		FinalFailure: finalFailure,
		Message:      err.MaskSensitiveError(),
	}
	if c != nil {
		failureContext.ModelName = c.GetString("original_model")
		failureContext.RequestId = c.GetString(common.RequestIdKey)
		usedChannels := c.GetStringSlice("use_channel")
		failureContext.AttemptIndex = len(usedChannels)
		failureContext.UsedChannels = strings.Join(usedChannels, "->")
	}
	if len(err.Metadata) > 0 {
		failureContext.Metadata = common.JsonRawMessageToString(err.Metadata)
	}
	return failureContext
}

func relayChannelConfigIsolationKey(c *gin.Context, channelID int, info *relaycommon.RelayInfo, retryParam *service.RetryParam) service.ChannelConfigIsolationKey {
	modelName := ""
	if retryParam != nil {
		modelName = strings.TrimSpace(retryParam.ModelName)
	}
	if modelName == "" && info != nil {
		modelName = strings.TrimSpace(info.OriginModelName)
	}
	if modelName == "" && c != nil {
		modelName = strings.TrimSpace(c.GetString("original_model"))
	}

	selectedGroup := ""
	endpointType := constant.EndpointType("")
	if plan := selectedModelGatewayPlan(c); plan != nil {
		selectedGroup = strings.TrimSpace(plan.SelectedGroup)
		endpointType = plan.RuntimeKey.EndpointType
	}
	if selectedGroup == "" && info != nil {
		selectedGroup = strings.TrimSpace(info.UsingGroup)
	}
	if selectedGroup == "" && c != nil {
		selectedGroup = strings.TrimSpace(currentRelayLogGroup(c))
	}
	if selectedGroup == "" && retryParam != nil {
		selectedGroup = strings.TrimSpace(retryParam.TokenGroup)
	}
	if endpointType == "" && retryParam != nil {
		endpointType = retryParam.EndpointType
	}
	if endpointType == "" && info != nil {
		endpointType = requiredEndpointTypeForRelay(info)
	}

	return service.NewChannelConfigIsolationKey(channelID, modelName, selectedGroup, endpointType)
}

func recordRelayChannelConfigAuthError(c *gin.Context, channelID int, err *types.NewAPIError) {
	if channelID <= 0 || err == nil {
		return
	}
	key := relayChannelConfigIsolationKey(c, channelID, nil, nil)
	service.RecordChannelConfigAuthError(key, err.MaskSensitiveError())
}

func recordRelayChannelConfigSuccess(c *gin.Context, channelID int, info *relaycommon.RelayInfo, retryParam *service.RetryParam) {
	if channelID <= 0 {
		return
	}
	key := relayChannelConfigIsolationKey(c, channelID, info, retryParam)
	service.RecordChannelConfigSuccess(key)
}

func appendChannelFailureTraceAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	trace, ok := common.GetContextKeyType[[]map[string]interface{}](c, constant.ContextKeyChannelFailureTrace)
	if !ok || len(trace) == 0 {
		return
	}
	adminInfo["channel_failures"] = trace
}

func traceChannelFailure(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, finalFailure bool) {
	if c == nil || err == nil {
		return
	}
	item := map[string]interface{}{
		"channel_id":     channelError.ChannelId,
		"channel_name":   channelError.ChannelName,
		"channel_type":   channelError.ChannelType,
		"status_code":    err.StatusCode,
		"error_type":     err.GetErrorType(),
		"error_code":     err.GetErrorCode(),
		"error_category": classifyRelayAttemptError(c, err),
		"message":        err.MaskSensitiveError(),
		"final_failure":  finalFailure,
	}
	if item["error_category"] != modelgatewaycore.ErrorCategoryOverloadSkip {
		if reason, ok := channelFailureAvoidanceReason(err); ok {
			item["temporary_avoidance_reason"] = reason
		}
	}
	if item["error_category"] == modelgatewaycore.ErrorCategoryOverloadSkip {
		item["retry_action"] = "switch_channel"
	}
	if service.IsUpstreamConcurrencyLimitError(err) || err.GetErrorCode() == types.ErrorCodeChannelConcurrencyLimit {
		item["concurrency_limited"] = true
		item["retry_action"] = "switch_channel"
	}
	if len(err.Metadata) > 0 {
		item["error_metadata"] = common.JsonRawMessageToString(err.Metadata)
	}
	trace, _ := common.GetContextKeyType[[]map[string]interface{}](c, constant.ContextKeyChannelFailureTrace)
	trace = append(trace, item)
	common.SetContextKey(c, constant.ContextKeyChannelFailureTrace, trace)
}

func currentRelayLogGroup(c *gin.Context) string {
	group := c.GetString("group")
	if autoGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); autoGroup != "" {
		return autoGroup
	}
	return group
}

func shouldTemporarilyAvoidChannel(err *types.NewAPIError) bool {
	_, ok := channelFailureAvoidanceReason(err)
	return ok
}

func channelFailureAvoidanceReason(err *types.NewAPIError) (string, bool) {
	if err == nil || types.IsSkipRetryError(err) {
		return "", false
	}
	if service.IsBalanceInsufficientError(err) {
		return "", false
	}
	if isConcurrencyBusyError(err) {
		return "", false
	}
	if isUpstreamRateLimitLikeError(err) {
		return "upstream_rate_limit", true
	}
	if isUpstreamFailoverCandidate(err) {
		return formatUpstreamFailureAvoidanceReason(err), true
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeDoRequestFailed,
		types.ErrorCodeReadResponseBodyFailed,
		types.ErrorCodeBadResponse,
		types.ErrorCodeBadResponseBody:
		return formatUpstreamFailureAvoidanceReason(err), true
	case types.ErrorCodeBadResponseStatusCode:
		if err.StatusCode == http.StatusBadGateway ||
			err.StatusCode == http.StatusServiceUnavailable ||
			err.StatusCode == http.StatusGatewayTimeout ||
			err.StatusCode == 524 ||
			err.StatusCode == 529 {
			return formatUpstreamFailureAvoidanceReason(err), true
		}
	default:
	}
	return "", false
}

func formatUpstreamFailureAvoidanceReason(err *types.NewAPIError) string {
	code := normalizeFailureAvoidancePart(string(err.GetErrorCode()))
	if err.StatusCode >= 100 && err.StatusCode <= 599 {
		return fmt.Sprintf("upstream_error:%d:%s", err.StatusCode, code)
	}
	return fmt.Sprintf("upstream_error:%s", code)
}

func normalizeFailureAvoidancePart(value string) string {
	value = strings.ToLower(value)
	var builder strings.Builder
	for _, r := range value {
		if builder.Len() >= 80 {
			break
		}
		if (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '_' ||
			r == '-' ||
			r == ':' ||
			r == '.' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte('_')
	}
	result := strings.Trim(builder.String(), "._:-")
	if result == "" {
		return "unknown"
	}
	return result
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			relayInfo.Billing.Refund(c)
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:                    c,
		TokenGroup:             relayInfo.TokenGroup,
		ModelName:              relayInfo.OriginModelName,
		EndpointType:           requiredEndpointTypeForRelay(relayInfo),
		RequiresCodexImageTool: requiresCodexImageToolForRelay(relayInfo),
		Retry:                  common.GetPointer(0),
	}

	for ; retryParam.GetRetry() <= common.RetryTimes; retryParam.IncreaseRetry() {
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
					taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					break
				}
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				break
			}
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			break
		}

		if !taskErr.LocalError {
			willRetry := shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry())
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode),
				!willRetry)
			if !willRetry {
				break
			}
			continue
		}

		if !shouldRetryTaskRelay(c, channel.Id, taskErr, common.RetryTimes-retryParam.GetRetry()) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		if settleErr := service.SettleBilling(c, relayInfo, result.Quota); settleErr != nil {
			common.SysError("settle task billing error: " + settleErr.Error())
		}
		service.LogTaskConsumption(c, relayInfo)

		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		task.PrivateData.BillingSource = relayInfo.BillingSource
		task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
		task.PrivateData.SubscriptionQuota = int(relayInfo.SubscriptionPreConsumed + relayInfo.SubscriptionPostDelta)
		if task.PrivateData.SubscriptionQuota < 0 {
			task.PrivateData.SubscriptionQuota = 0
		}
		task.PrivateData.WalletQuota = relayInfo.WalletConsumed
		task.PrivateData.TokenId = relayInfo.TokenId
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:        relayInfo.PriceData.ModelPrice,
			GroupRatio:        relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:        relayInfo.PriceData.ModelRatio,
			OtherRatios:       relayInfo.PriceData.OtherRatios,
			OriginModelName:   relayInfo.OriginModelName,
			PerCallBilling:    common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
			BillingPreference: common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference),
		}
		task.Quota = result.Quota
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}
