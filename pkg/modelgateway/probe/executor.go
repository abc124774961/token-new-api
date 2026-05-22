package probe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type ProbeExecutor struct {
	timeout time.Duration
	billing *ProbeBillingRecorder
}

func NewProbeExecutor(timeout time.Duration, billing *ProbeBillingRecorder) *ProbeExecutor {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if billing == nil {
		billing = NewProbeBillingRecorder()
	}
	return &ProbeExecutor{
		timeout: timeout,
		billing: billing,
	}
}

func (e *ProbeExecutor) Execute(ctx context.Context, candidate ProbeCandidate) ProbeRunResult {
	startedAt := time.Now()
	probeID := newProbeID()
	result := ProbeRunResult{
		ProbeID:    probeID,
		Reason:     candidate.Reason,
		Channel:    candidate.Channel,
		Model:      candidate.Model,
		Group:      candidate.Group,
		RuntimeKey: candidate.Key,
		StartedAt:  startedAt,
	}
	if candidate.Channel == nil {
		result.Err = errors.New("probe channel is nil")
		return result
	}
	if strings.TrimSpace(candidate.Model) == "" {
		result.Err = errors.New("probe model is empty")
		return result
	}

	runCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	out := e.execute(runCtx, result)
	if out.Err == nil && runCtx.Err() != nil && !out.Success {
		out.Duration = time.Since(startedAt)
		out.Err = runCtx.Err()
		out.StatusCode = http.StatusGatewayTimeout
		out.NewAPIError = types.NewErrorWithStatusCode(runCtx.Err(), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)
	}
	return out
}

func (e *ProbeExecutor) execute(ctx context.Context, result ProbeRunResult) ProbeRunResult {
	channel := result.Channel
	endpointType := endpointTypeForProbe(channel, result.Model)
	requestPath := requestPathForEndpoint(endpointType)
	request := buildProbeRequest(result.Model, endpointType)
	if request == nil {
		result.Err = fmt.Errorf("model %s is not supported for health probe", result.Model)
		return result
	}

	c, recorder := newProbeGinContext(ctx, result.ProbeID, requestPath)
	if err := writeRootContext(c, result.ProbeID, result.Group); err != nil {
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeGetChannelFailed)
		return result
	}

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, result.Model)
	if newAPIError != nil {
		result.Context = c
		result.NewAPIError = newAPIError
		result.Err = newAPIError
		result.StatusCode = newAPIError.StatusCode
		return result
	}
	c.Request.Header.Set("Content-Type", "application/json")
	relayFormat := relayFormatForEndpoint(endpointType)
	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)
	if err != nil {
		result.Context = c
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return result
	}
	info.IsChannelTest = true
	info.InitChannelMeta(c)
	if err = attachProbeBillingRequestInput(info, request); err != nil {
		result.Context = c
		result.RelayInfo = info
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeJsonMarshalFailed)
		return result
	}
	if err = helper.ModelMappedHelper(c, info, request); err != nil {
		result.Context = c
		result.RelayInfo = info
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeChannelModelMappedError)
		return result
	}
	request.SetModelName(info.UpstreamModelName)

	apiType, _ := common.ChannelType2APIType(channel.Type)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		apiType != constant.APITypeOpenAI &&
		apiType != constant.APITypeCodex {
		err = fmt.Errorf("responses compaction probe only supports openai/codex channels, got api type %d", apiType)
		result.Context = c
		result.RelayInfo = info
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeInvalidApiType)
		return result
	}
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		err = fmt.Errorf("invalid api type: %d, adaptor is nil", apiType)
		result.Context = c
		result.RelayInfo = info
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeInvalidApiType)
		return result
	}

	priceData, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	if err != nil {
		result.Context = c
		result.RelayInfo = info
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
		return result
	}
	adaptor.Init(info)

	converted, err := convertProbeRequest(c, adaptor, info, request)
	if err != nil {
		result.Context = c
		result.RelayInfo = info
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeConvertRequestFailed)
		return result
	}
	payload, err := common.Marshal(converted)
	if err != nil {
		result.Context = c
		result.RelayInfo = info
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeJsonMarshalFailed)
		return result
	}
	if len(info.ParamOverride) > 0 {
		payload, err = relaycommon.ApplyParamOverrideWithRelayInfo(payload, info)
		if err != nil {
			if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
				result.Context = c
				result.RelayInfo = info
				result.Err = fixedErr
				result.NewAPIError = relaycommon.NewAPIErrorFromParamOverride(fixedErr)
				return result
			}
			result.Context = c
			result.RelayInfo = info
			result.Err = err
			result.NewAPIError = types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid)
			return result
		}
	}

	c.Request.Body = io.NopCloser(bytes.NewReader(payload))
	respAny, err := adaptor.DoRequest(c, info, bytes.NewReader(payload))
	if err != nil {
		result.Context = c
		result.RelayInfo = info
		result.Err = err
		result.NewAPIError = ensureNewAPIError(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
		return result
	}
	httpResp, _ := respAny.(*http.Response)
	if httpResp != nil && httpResp.StatusCode != http.StatusOK {
		newAPIError := service.RelayErrorHandler(c.Request.Context(), httpResp, true)
		result.Context = c
		result.RelayInfo = info
		result.Err = newAPIError
		result.NewAPIError = types.NewOpenAIError(newAPIError, types.ErrorCodeBadResponseStatusCode, httpResp.StatusCode)
		result.StatusCode = httpResp.StatusCode
		return result
	}
	usageAny, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		result.Context = c
		result.RelayInfo = info
		result.Err = newAPIError
		result.NewAPIError = newAPIError
		result.StatusCode = newAPIError.StatusCode
		return result
	}

	usage := coerceProbeUsage(usageAny, info.GetEstimatePromptTokens())
	info.SetEstimatePromptTokens(usage.PromptTokens)
	result.Duration = time.Since(result.StartedAt)
	result.TTFT = probeTTFT(info)
	result.Context = c
	result.RelayInfo = info
	result.Usage = usage
	result.PriceData = priceData
	result.StatusCode = http.StatusOK
	result.Success = true

	if e.billing != nil {
		if quota, err := e.billing.RecordSuccess(c, result, priceData, usage); err != nil {
			common.SysLog(fmt.Sprintf("model gateway probe billing failed: probe_id=%s channel_id=%d error=%v", result.ProbeID, channel.Id, err))
		} else {
			result.Quota = quota
		}
	}
	_ = recorder.Result().Body.Close()
	return result
}

func (r ProbeRunResult) AttemptResult() core.AttemptResult {
	errCode := ""
	errType := ""
	errMessage := ""
	errorCategory := ""
	statusCode := r.StatusCode
	if r.NewAPIError != nil {
		errCode = string(r.NewAPIError.GetErrorCode())
		errType = string(r.NewAPIError.GetErrorType())
		errMessage = r.NewAPIError.Error()
		if statusCode == 0 {
			statusCode = r.NewAPIError.StatusCode
		}
		errorCategory = probeErrorCategory(r.NewAPIError)
	} else if r.Err != nil {
		errMessage = r.Err.Error()
		errorCategory = "upstream_error"
	}
	channelID := 0
	channelName := ""
	if r.Channel != nil {
		channelID = r.Channel.Id
		channelName = r.Channel.Name
	}
	concurrencyLimited := r.NewAPIError != nil && (service.IsUpstreamConcurrencyLimitError(r.NewAPIError) || r.NewAPIError.GetErrorCode() == types.ErrorCodeChannelConcurrencyLimit)
	activeConcurrency := 0
	learnedLimit := 0
	learnedChanged := false
	if concurrencyLimited && channelID > 0 {
		activeConcurrency = service.GetChannelActiveConcurrency(channelID)
		learned := service.LearnChannelConcurrencyLimitWithResult(r.Context, channelID, activeConcurrency, r.NewAPIError)
		learnedLimit = learned.LearnedLimit
		learnedChanged = learned.Changed
	}
	key := r.RuntimeKey
	if key.ChannelID == 0 {
		key.ChannelID = channelID
	}
	if key.RequestedModel == "" {
		key.RequestedModel = r.Model
	}
	if key.UpstreamModel == "" && r.RelayInfo != nil {
		key.UpstreamModel = r.RelayInfo.UpstreamModelName
	}
	if key.Group == "" {
		key.Group = r.Group
	}
	if key.EndpointType == "" {
		key.EndpointType = constant.EndpointTypeOpenAI
	}
	ttft := r.TTFT
	if ttft <= 0 && r.Success {
		ttft = r.Duration
	}
	return core.AttemptResult{
		Key:                            key,
		RequestID:                      r.ProbeID,
		AttemptIndex:                   0,
		ChannelID:                      channelID,
		ChannelName:                    channelName,
		RequestedGroup:                 r.Group,
		SelectedGroup:                  r.Group,
		ModelName:                      r.Model,
		EndpointType:                   key.EndpointType,
		Success:                        r.Success,
		StatusCode:                     statusCode,
		ErrorCode:                      errCode,
		ErrorType:                      errType,
		ErrorMessage:                   errMessage,
		ErrorCategory:                  errorCategory,
		ObservedAt:                     time.Now(),
		Duration:                       r.Duration,
		TTFT:                           ttft,
		ConcurrencyLimited:             concurrencyLimited,
		ActiveConcurrency:              activeConcurrency,
		LearnedConcurrencyLimit:        learnedLimit,
		LearnedConcurrencyLimitChanged: learnedChanged,
		IsHealthProbe:                  true,
		ProbeReason:                    r.Reason,
	}
}

func writeRootContext(c *gin.Context, probeID string, group string) error {
	root := model.GetRootUser()
	if root == nil || root.Id <= 0 {
		return errors.New("root user not found")
	}
	root.ToBaseUser().WriteContext(c)
	c.Set("id", root.Id)
	common.SetContextKey(c, constant.ContextKeyUserId, root.Id)
	c.Set("username", root.Username)
	c.Set("token_name", TokenName)
	c.Set("token_id", 0)
	c.Set(common.RequestIdKey, probeID)
	if group = strings.TrimSpace(group); group != "" {
		c.Set("group", group)
		common.SetContextKey(c, constant.ContextKeyUsingGroup, group)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, group)
	}
	return nil
}

func newProbeGinContext(ctx context.Context, probeID string, path string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	if ctx == nil {
		ctx = context.Background()
	}
	c.Request = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: path},
		Body:   http.NoBody,
		Header: make(http.Header),
	}
	c.Request = c.Request.WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, probeID)
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	return c, recorder
}

func buildProbeRequest(modelName string, endpointType constant.EndpointType) dto.Request {
	stream := false
	switch endpointType {
	case constant.EndpointTypeOpenAIResponse:
		maxTokens := uint(8)
		return &dto.OpenAIResponsesRequest{
			Model:           modelName,
			Input:           []byte(`[{"role":"user","content":"hi"}]`),
			Stream:          &stream,
			MaxOutputTokens: &maxTokens,
		}
	case constant.EndpointTypeOpenAIResponseCompact:
		return &dto.OpenAIResponsesCompactionRequest{
			Model: modelName,
			Input: []byte(`[{"role":"user","content":"hi"}]`),
		}
	case constant.EndpointTypeOpenAI, constant.EndpointTypeAnthropic, constant.EndpointTypeGemini:
		maxTokens := uint(8)
		if endpointType == constant.EndpointTypeGemini {
			maxTokens = 128
		}
		return &dto.GeneralOpenAIRequest{
			Model:  modelName,
			Stream: &stream,
			Messages: []dto.Message{{
				Role:    "user",
				Content: "hi",
			}},
			MaxTokens: &maxTokens,
		}
	default:
		return nil
	}
}

func endpointTypeForProbe(channel *model.Channel, modelName string) constant.EndpointType {
	modelName = strings.TrimSpace(modelName)
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return constant.EndpointTypeOpenAIResponseCompact
	}
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return constant.EndpointTypeOpenAIResponse
	}
	if strings.Contains(strings.ToLower(modelName), "codex") {
		return constant.EndpointTypeOpenAIResponse
	}
	if channel != nil && channel.Type == constant.ChannelTypeOpenAI {
		otherSettings := channel.GetOtherSettings()
		if otherSettings.UsesResponsesWireAPI() {
			return constant.EndpointTypeOpenAIResponse
		}
	}
	return constant.EndpointTypeOpenAI
}

func requestPathForEndpoint(endpointType constant.EndpointType) string {
	if info, ok := common.GetDefaultEndpointInfo(endpointType); ok && info.Path != "" {
		return info.Path
	}
	return "/v1/chat/completions"
}

func relayFormatForEndpoint(endpointType constant.EndpointType) types.RelayFormat {
	switch endpointType {
	case constant.EndpointTypeOpenAIResponse:
		return types.RelayFormatOpenAIResponses
	case constant.EndpointTypeOpenAIResponseCompact:
		return types.RelayFormatOpenAIResponsesCompaction
	case constant.EndpointTypeAnthropic:
		return types.RelayFormatClaude
	case constant.EndpointTypeGemini:
		return types.RelayFormatGemini
	default:
		return types.RelayFormatOpenAI
	}
}

func convertProbeRequest(c *gin.Context, adaptor interface {
	ConvertOpenAIRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeneralOpenAIRequest) (any, error)
	ConvertOpenAIResponsesRequest(*gin.Context, *relaycommon.RelayInfo, dto.OpenAIResponsesRequest) (any, error)
}, info *relaycommon.RelayInfo, request dto.Request) (any, error) {
	switch info.RelayMode {
	case relayconstant.RelayModeResponses:
		responseReq, ok := request.(*dto.OpenAIResponsesRequest)
		if !ok {
			return nil, errors.New("invalid responses probe request")
		}
		return adaptor.ConvertOpenAIResponsesRequest(c, info, *responseReq)
	case relayconstant.RelayModeResponsesCompact:
		switch req := request.(type) {
		case *dto.OpenAIResponsesCompactionRequest:
			return adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
				Model:              req.Model,
				Input:              req.Input,
				Instructions:       req.Instructions,
				PreviousResponseID: req.PreviousResponseID,
				MaxOutputTokens:    lo.ToPtr(uint(8)),
			})
		case *dto.OpenAIResponsesRequest:
			return adaptor.ConvertOpenAIResponsesRequest(c, info, *req)
		default:
			return nil, errors.New("invalid responses compaction probe request")
		}
	default:
		generalReq, ok := request.(*dto.GeneralOpenAIRequest)
		if !ok {
			return nil, errors.New("invalid openai probe request")
		}
		return adaptor.ConvertOpenAIRequest(c, info, generalReq)
	}
}

func attachProbeBillingRequestInput(info *relaycommon.RelayInfo, request dto.Request) error {
	if info == nil {
		return nil
	}
	input, err := helper.BuildBillingExprRequestInputFromRequest(request, info.RequestHeaders)
	if err != nil {
		return err
	}
	info.BillingRequestInput = &input
	return nil
}

func coerceProbeUsage(value any, estimatePromptTokens int) *dto.Usage {
	switch usage := value.(type) {
	case *dto.Usage:
		return normalizeProbeUsage(usage, estimatePromptTokens)
	case dto.Usage:
		return normalizeProbeUsage(&usage, estimatePromptTokens)
	default:
		return normalizeProbeUsage(nil, estimatePromptTokens)
	}
}

func normalizeProbeUsage(usage *dto.Usage, estimatePromptTokens int) *dto.Usage {
	if usage == nil {
		usage = &dto.Usage{}
	}
	if usage.PromptTokens <= 0 {
		if estimatePromptTokens > 0 {
			usage.PromptTokens = estimatePromptTokens
		} else {
			usage.PromptTokens = 1
		}
	}
	if usage.CompletionTokens < 0 {
		usage.CompletionTokens = 0
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.TotalTokens <= 0 {
		usage.TotalTokens = 1
	}
	return usage
}

func settleProbeQuota(info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage) (int, *billingexpr.TieredResult) {
	if usage != nil && info != nil && info.TieredBillingSnapshot != nil {
		isClaudeUsageSemantic := usage.UsageSemantic == "anthropic" || info.GetFinalRequestRelayFormat() == types.RelayFormatClaude
		usedVars := billingexpr.UsedVars(info.TieredBillingSnapshot.ExprString)
		if ok, quota, result := service.TryTieredSettle(info, service.BuildTieredTokenParams(usage, isClaudeUsageSemantic, usedVars)); ok {
			if quota <= 0 && priceData.ModelRatio != 0 {
				quota = 1
			}
			return quota, result
		}
	}
	if usage == nil {
		usage = normalizeProbeUsage(nil, 1)
	}
	if priceData.UsePrice {
		quota := int(priceData.ModelPrice * common.QuotaPerUnit * priceData.GroupRatioInfo.GroupRatio)
		if quota <= 0 && priceData.ModelPrice > 0 && priceData.GroupRatioInfo.GroupRatio > 0 {
			quota = 1
		}
		return quota, nil
	}
	quota := usage.PromptTokens + int(math.Round(float64(usage.CompletionTokens)*priceData.CompletionRatio))
	quota = int(math.Round(float64(quota) * priceData.ModelRatio * priceData.GroupRatioInfo.GroupRatio))
	if priceData.ModelRatio != 0 && priceData.GroupRatioInfo.GroupRatio != 0 && quota <= 0 {
		quota = 1
	}
	return quota, nil
}

func ensureNewAPIError(err error, code types.ErrorCode, status int) *types.NewAPIError {
	var newAPIError *types.NewAPIError
	if errors.As(err, &newAPIError) {
		return newAPIError
	}
	return types.NewErrorWithStatusCode(err, code, status)
}

func probeTTFT(info *relaycommon.RelayInfo) time.Duration {
	if info == nil || !info.HasSendResponse() {
		return 0
	}
	return info.FirstResponseTime.Sub(info.StartTime)
}

func probeErrorCategory(apiErr *types.NewAPIError) string {
	if apiErr == nil {
		return ""
	}
	if service.IsUpstreamConcurrencyLimitError(apiErr) || apiErr.GetErrorCode() == types.ErrorCodeChannelConcurrencyLimit {
		return "upstream_concurrency_limit"
	}
	if service.IsBalanceInsufficientError(apiErr) {
		return "balance_or_quota"
	}
	if apiErr.StatusCode == http.StatusTooManyRequests {
		return "rate_limit"
	}
	if apiErr.StatusCode == http.StatusGatewayTimeout || errors.Is(apiErr, context.DeadlineExceeded) {
		return "timeout"
	}
	if apiErr.StatusCode >= http.StatusInternalServerError {
		return "server_error"
	}
	return "upstream_error"
}

func newProbeID() string {
	return probeIDPrefix + common.GetTimeString() + common.GetRandomString(8)
}

var _ = scheduler.CircuitErrorConcurrencyLimit
