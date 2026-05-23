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
	"strconv"
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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type ProbeExecutor struct {
	timeout time.Duration
}

type RelayInvoker func(*gin.Context, types.RelayFormat)

var relayInvoker RelayInvoker

func RegisterRelayInvoker(invoker RelayInvoker) {
	relayInvoker = invoker
}

func NewProbeExecutor(timeout time.Duration, _ *ProbeBillingRecorder) *ProbeExecutor {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &ProbeExecutor{
		timeout: timeout,
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
		TargetKey:  candidate.Key,
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
	probeEndpointType := probeEndpointType(result.Channel, result.Model, result.RuntimeKey.EndpointType)
	requestPath := requestPathForEndpoint(probeEndpointType, result.Model)
	request := buildProbeRequest(result.Model, probeEndpointType)
	if request == nil {
		result.Err = fmt.Errorf("model %s is not supported for health probe", result.Model)
		return result
	}

	c, recorder := newProbeGinContext(ctx, result.ProbeID, requestPath)
	result.Context = c
	defer common.CleanupBodyStorage(c)
	if err := writeRootContext(c, result.ProbeID, result.Group); err != nil {
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeGetChannelFailed)
		return result
	}

	common.SetContextKey(c, constant.ContextKeyHealthProbe, true)
	common.SetContextKey(c, constant.ContextKeyHealthProbeReason, result.Reason)
	common.SetContextKey(c, constant.ContextKeyHealthProbeRuntimeKey, result.TargetKey)
	common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, strconv.Itoa(result.TargetKey.ChannelID))
	body, err := common.Marshal(request)
	if err != nil {
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeJsonMarshalFailed)
		return result
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	c.Request.Header.Set("Accept", "text/event-stream")
	c.Request.Header.Set("Content-Type", "application/json")

	invoker := relayInvoker
	if invoker == nil {
		err := errors.New("probe relay invoker is not registered")
		result.Err = err
		result.NewAPIError = types.NewError(err, types.ErrorCodeGetChannelFailed)
		return result
	}
	var relayFormat types.RelayFormat
	middleware.DistributeWithNext(func(c *gin.Context) {
		relayFormat = relayFormatForEndpoint(probeEndpointType)
		invoker(c, relayFormat)
	})(c)
	result.Duration = time.Since(result.StartedAt)
	result.StatusCode = recorder.Code
	if result.StatusCode == 0 {
		result.StatusCode = http.StatusOK
	}
	if info, ok := common.GetContextKeyType[*relaycommon.RelayInfo](c, constant.ContextKeyRelayInfo); ok {
		result.RelayInfo = info
		result.TTFT = probeTTFT(info)
		result.PriceData = info.PriceData
		result.Usage = normalizeProbeUsage(nil, info.GetEstimatePromptTokens())
		result.Quota = probeQuotaFromBilling(info)
	}
	if channelID := common.GetContextKeyInt(c, constant.ContextKeyChannelId); channelID > 0 {
		if selected, err := model.CacheGetChannel(channelID); err == nil && selected != nil {
			result.Channel = selected
			result.RuntimeKey.ChannelID = selected.Id
			if result.RuntimeKey.UpstreamModel == "" && result.RelayInfo != nil {
				result.RuntimeKey.UpstreamModel = result.RelayInfo.UpstreamModelName
			}
		}
	}
	if group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup); strings.TrimSpace(group) != "" {
		result.Group = group
		result.RuntimeKey.Group = group
	}
	if relayFormat != "" {
		result.RuntimeKey.EndpointType = endpointTypeForRelayFormat(relayFormat)
	} else if result.RuntimeKey.EndpointType == "" {
		result.RuntimeKey.EndpointType = probeEndpointType
	}
	if c.IsAborted() || result.StatusCode >= http.StatusBadRequest {
		result.Err = fmt.Errorf("probe relay failed with status %d", result.StatusCode)
		result.NewAPIError = types.NewErrorWithStatusCode(result.Err, types.ErrorCodeDoRequestFailed, result.StatusCode)
		return result
	}
	result.Success = true
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
	balanceInsufficient := r.NewAPIError != nil && service.IsBalanceInsufficientError(r.NewAPIError)
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
		Key:                            r.AttemptRuntimeKey(),
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
		BalanceInsufficient:            balanceInsufficient,
		ActiveConcurrency:              activeConcurrency,
		LearnedConcurrencyLimit:        learnedLimit,
		LearnedConcurrencyLimitChanged: learnedChanged,
		IsHealthProbe:                  true,
		ProbeReason:                    r.Reason,
	}
}

func (r ProbeRunResult) AttemptRuntimeKey() core.RuntimeKey {
	if r.TargetKey.ChannelID > 0 {
		key := r.TargetKey
		if key.EndpointType == "" {
			key.EndpointType = r.RuntimeKey.EndpointType
		}
		return key
	}
	return r.RuntimeKey
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
	stream := true
	streamOptions := &dto.StreamOptions{IncludeUsage: true}
	switch endpointType {
	case constant.EndpointTypeOpenAIResponse:
		maxTokens := uint(8)
		return &dto.OpenAIResponsesRequest{
			Model:           modelName,
			Input:           []byte(`[{"role":"user","content":"Reply with exactly the word ok."}]`),
			Stream:          &stream,
			StreamOptions:   streamOptions,
			MaxOutputTokens: &maxTokens,
		}
	case constant.EndpointTypeAnthropic:
		maxTokens := uint(8)
		return &dto.ClaudeRequest{
			Model: modelName,
			Messages: []dto.ClaudeMessage{{
				Role:    "user",
				Content: "Reply with exactly the word ok.",
			}},
			MaxTokens: &maxTokens,
			Stream:    &stream,
		}
	case constant.EndpointTypeGemini:
		maxTokens := uint(128)
		return &dto.GeminiChatRequest{
			Contents: []dto.GeminiChatContent{{
				Role: "user",
				Parts: []dto.GeminiPart{{
					Text: "Reply with exactly the word ok.",
				}},
			}},
			GenerationConfig: dto.GeminiChatGenerationConfig{
				MaxOutputTokens: &maxTokens,
			},
		}
	case constant.EndpointTypeOpenAI:
		maxTokens := uint(8)
		return &dto.GeneralOpenAIRequest{
			Model:         modelName,
			Stream:        &stream,
			StreamOptions: streamOptions,
			Messages: []dto.Message{{
				Role:    "user",
				Content: "Reply with exactly the word ok.",
			}},
			MaxTokens: &maxTokens,
		}
	default:
		return nil
	}
}

func probeEndpointType(channel *model.Channel, modelName string, fallback constant.EndpointType) constant.EndpointType {
	endpointType := fallback
	if endpointType == "" {
		endpointType = endpointTypeForProbe(channel, modelName)
	}
	if endpointType == constant.EndpointTypeOpenAIResponseCompact {
		return constant.EndpointTypeOpenAIResponse
	}
	if endpointType != "" {
		return endpointType
	}
	return constant.EndpointTypeOpenAI
}

func endpointTypeForProbe(channel *model.Channel, modelName string) constant.EndpointType {
	modelName = strings.TrimSpace(modelName)
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return constant.EndpointTypeOpenAIResponse
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

func requestPathForEndpoint(endpointType constant.EndpointType, modelName string) string {
	if endpointType == constant.EndpointTypeGemini {
		return fmt.Sprintf("/v1beta/models/%s:streamGenerateContent", url.PathEscape(strings.TrimSpace(modelName)))
	}
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

func endpointTypeForRelayFormat(relayFormat types.RelayFormat) constant.EndpointType {
	switch relayFormat {
	case types.RelayFormatOpenAIResponses:
		return constant.EndpointTypeOpenAIResponse
	case types.RelayFormatOpenAIResponsesCompaction:
		return constant.EndpointTypeOpenAIResponseCompact
	case types.RelayFormatClaude:
		return constant.EndpointTypeAnthropic
	case types.RelayFormatGemini:
		return constant.EndpointTypeGemini
	default:
		return constant.EndpointTypeOpenAI
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

func probeQuotaFromBilling(info *relaycommon.RelayInfo) int {
	if info == nil || info.Billing == nil {
		return 0
	}
	if quota := info.FinalPreConsumedQuota + int(info.SubscriptionPostDelta); quota > 0 {
		return quota
	}
	return info.Billing.GetPreConsumedQuota()
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
