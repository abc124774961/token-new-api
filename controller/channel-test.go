package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/pkg/channelcapability"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"

	"github.com/gin-gonic/gin"
)

type testResult struct {
	context     *gin.Context
	localErr    error
	newAPIError *types.NewAPIError
	refreshed   bool
	refreshErr  error
}

type channelTestOptions struct {
	CredentialIndex  *int
	AllowProxyBridge bool
}

type accountCapabilityProbeResult struct {
	Index        int                            `json:"index"`
	Capabilities model.ChannelAccountCapability `json:"capabilities"`
	OAuthJSON    bool                           `json:"oauth_json"`
}

func normalizeChannelTestEndpoint(channel *model.Channel, modelName, endpointType string) string {
	normalized := strings.TrimSpace(endpointType)
	if normalized != "" {
		return normalized
	}
	if strings.HasSuffix(modelName, ratio_setting.CompactModelSuffix) {
		return string(constant.EndpointTypeOpenAIResponseCompact)
	}
	if channel != nil && channel.Type == constant.ChannelTypeCodex {
		return string(constant.EndpointTypeOpenAIResponse)
	}
	if channel != nil && channel.Type == constant.ChannelTypeOpenAI {
		otherSettings := channel.GetOtherSettings()
		if otherSettings.UsesResponsesWireAPI() {
			return string(constant.EndpointTypeOpenAIResponse)
		}
	}
	return normalized
}

func resolveChannelTestEndpoint(channel *model.Channel, modelName, endpointType string, options channelTestOptions) string {
	if channelTestUsesOAuthJSONCredential(channel, options) && strings.TrimSpace(endpointType) == "" {
		if capability, ok := channelTestAccountCapability(channel, options); ok {
			if capability.HasResponsesWriteAllowed() {
				endpointType = string(constant.EndpointTypeOpenAIResponse)
			} else if capability.HasResponsesWriteDenied() || capability.HasChatCompletionsWriteAllowed() {
				return string(constant.EndpointTypeOpenAI)
			}
		}
		if endpointType == "" {
			endpointType = string(constant.EndpointTypeOpenAIResponse)
		}
	}
	return normalizeChannelTestEndpoint(channel, modelName, endpointType)
}

func channelTestAccountCapability(channel *model.Channel, options channelTestOptions) (model.ChannelAccountCapability, bool) {
	index, ok := channelTestOAuthJSONCredentialIndex(channel, options)
	if !ok {
		return model.ChannelAccountCapability{}, false
	}
	return channel.ChannelInfo.AccountCapability(index)
}

func channelTestUsesOAuthJSONCredential(channel *model.Channel, options channelTestOptions) bool {
	_, ok := channelTestOAuthJSONCredentialIndex(channel, options)
	return ok
}

func channelTestOAuthJSONCredentialIndex(channel *model.Channel, options channelTestOptions) (int, bool) {
	if channel == nil || (channel.Type != constant.ChannelTypeOpenAI && channel.Type != constant.ChannelTypeCodex) {
		return 0, false
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return 0, false
	}
	if options.CredentialIndex != nil {
		index := *options.CredentialIndex
		if index < 0 || index >= len(keys) {
			return 0, false
		}
		return index, isOAuthJSONAccountKey(keys[index])
	}

	if channel.ChannelInfo.IsMultiKey && len(keys) > 1 {
		return 0, false
	}
	return 0, isOAuthJSONAccountKey(keys[0])
}

func isOAuthJSONAccountKey(raw string) bool {
	oauthKey, err := service.ParseCodexOAuthKey(strings.TrimSpace(raw))
	if err != nil || oauthKey == nil {
		return false
	}
	if strings.TrimSpace(oauthKey.AccountID) == "" {
		return false
	}
	return strings.TrimSpace(oauthKey.AccessToken) != "" || strings.TrimSpace(oauthKey.RefreshToken) != ""
}

func probeChannelAccountCapabilities(c *gin.Context, channel *model.Channel, credentialIndex int, testModel string) (accountCapabilityProbeResult, error) {
	if channel == nil {
		return accountCapabilityProbeResult{}, errors.New("渠道不存在")
	}
	keys := channel.GetKeys()
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return accountCapabilityProbeResult{}, errors.New("账号索引超出范围")
	}
	if channel.Type != constant.ChannelTypeOpenAI && channel.Type != constant.ChannelTypeCodex {
		return accountCapabilityProbeResult{}, errors.New("该渠道暂不支持账号权限检测")
	}
	if isOAuthJSONAccountKey(keys[credentialIndex]) {
		result, err := service.ProbeCodexOAuthAccountCapabilities(c.Request.Context(), channel, credentialIndex, service.CodexCapabilityProbeOptions{})
		if err != nil {
			return accountCapabilityProbeResult{}, err
		}
		if err := saveChannelAccountCapability(channel.Id, credentialIndex, result.Capability); err != nil {
			return accountCapabilityProbeResult{}, err
		}
		return accountCapabilityProbeResult{
			Index:        credentialIndex,
			Capabilities: result.Capability,
			OAuthJSON:    result.OAuthJSON,
		}, nil
	}

	options := channelTestOptions{CredentialIndex: &credentialIndex}
	capability := model.ChannelAccountCapability{
		CheckedTime: common.GetTimestamp(),
	}
	if existing, ok := channel.ChannelInfo.AccountCapability(credentialIndex); ok {
		capability = existing
		capability.CheckedTime = common.GetTimestamp()
	}

	messages := make([]string, 0, 3)
	lastEndpoint := ""

	channel, responsesResult := runChannelCapabilityProbeTest(c, channel, testModel, string(constant.EndpointTypeOpenAIResponse), false, options)
	capability.ResponsesWrite = channelCapabilityValueFromProbe(responsesResult)
	if probeMessage := channelCapabilityProbeMessage("Responses", responsesResult); probeMessage != "" {
		messages = append(messages, probeMessage)
		lastEndpoint = string(constant.EndpointTypeOpenAIResponse)
	}

	if capability.ResponsesWrite != nil && *capability.ResponsesWrite {
		var compactResult testResult
		channel, compactResult = runChannelCapabilityProbeTest(c, channel, testModel, string(constant.EndpointTypeOpenAIResponseCompact), false, options)
		capability.ResponsesCompactWrite = channelCapabilityValueFromProbe(compactResult)
		if probeMessage := channelCapabilityProbeMessage("Compact", compactResult); probeMessage != "" {
			messages = append(messages, probeMessage)
			lastEndpoint = string(constant.EndpointTypeOpenAIResponseCompact)
		}
	} else if capability.ResponsesWrite != nil && !*capability.ResponsesWrite {
		capability.ResponsesCompactWrite = lo.ToPtr(false)
	}

	if channel.Type == constant.ChannelTypeOpenAI {
		_, chatResult := runChannelCapabilityProbeTest(c, channel, testModel, string(constant.EndpointTypeOpenAI), false, options)
		capability.ChatCompletionsWrite = channelCapabilityValueFromProbe(chatResult)
		if probeMessage := channelCapabilityProbeMessage("Chat", chatResult); probeMessage != "" {
			messages = append(messages, probeMessage)
			lastEndpoint = string(constant.EndpointTypeOpenAI)
		}
	} else {
		capability.ChatCompletionsWrite = lo.ToPtr(false)
	}

	if len(messages) == 0 {
		capability.LastMessage = "权限检测完成"
		capability.LastEndpoint = ""
	} else {
		capability.LastMessage = strings.Join(messages, "；")
		capability.LastEndpoint = lastEndpoint
	}

	if err := saveChannelAccountCapability(channel.Id, credentialIndex, capability); err != nil {
		return accountCapabilityProbeResult{}, err
	}
	return accountCapabilityProbeResult{
		Index:        credentialIndex,
		Capabilities: capability,
		OAuthJSON:    isOAuthJSONAccountKey(keys[credentialIndex]),
	}, nil
}

func probePlatformAccountCapabilities(c *gin.Context, channel *model.Channel, credentialIndex int, testModel string) (accountCapabilityProbeResult, error) {
	if channel == nil {
		return accountCapabilityProbeResult{}, errors.New("渠道不存在")
	}
	keys := channel.GetKeys()
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return accountCapabilityProbeResult{}, errors.New("账号索引超出范围")
	}
	if channel.Type != constant.ChannelTypeOpenAI && channel.Type != constant.ChannelTypeCodex {
		return accountCapabilityProbeResult{}, errors.New("该渠道暂不支持 Platform API 诊断")
	}
	if isOAuthJSONAccountKey(keys[credentialIndex]) {
		result, err := service.ProbeCodexOAuthPlatformCapabilities(c.Request.Context(), channel, credentialIndex)
		if err != nil {
			return accountCapabilityProbeResult{}, err
		}
		if err := saveChannelAccountCapability(channel.Id, credentialIndex, result.Capability); err != nil {
			return accountCapabilityProbeResult{}, err
		}
		return accountCapabilityProbeResult{
			Index:        credentialIndex,
			Capabilities: result.Capability,
			OAuthJSON:    result.OAuthJSON,
		}, nil
	}
	options := channelTestOptions{CredentialIndex: &credentialIndex}
	capability := model.ChannelAccountCapability{
		CheckedTime:            common.GetTimestamp(),
		CapabilityProbeSurface: "platform_api",
	}
	if existing, ok := channel.ChannelInfo.AccountCapability(credentialIndex); ok {
		capability = existing
		capability.CheckedTime = common.GetTimestamp()
		capability.CapabilityProbeSurface = "platform_api"
	}

	messages := make([]string, 0, 3)
	_, chatResult := runChannelCapabilityProbeTest(c, channel, testModel, string(constant.EndpointTypeOpenAI), false, options)
	capability.PlatformChatCompletionsWrite = channelCapabilityValueFromProbe(chatResult)
	capability.ChatCompletionsWrite = capability.PlatformChatCompletionsWrite
	if probeMessage := channelCapabilityProbeMessage("Platform Chat", chatResult); probeMessage != "" {
		messages = append(messages, probeMessage)
	}

	channel, responsesResult := runChannelCapabilityProbeTest(c, channel, testModel, string(constant.EndpointTypeOpenAIResponse), false, options)
	capability.PlatformResponsesWrite = channelCapabilityValueFromProbe(responsesResult)
	if channel.Type != constant.ChannelTypeCodex {
		capability.ResponsesWrite = capability.PlatformResponsesWrite
	}
	if probeMessage := channelCapabilityProbeMessage("Platform Responses", responsesResult); probeMessage != "" {
		messages = append(messages, probeMessage)
	}

	_, compactResult := runChannelCapabilityProbeTest(c, channel, testModel, string(constant.EndpointTypeOpenAIResponseCompact), false, options)
	capability.PlatformResponsesCompactWrite = channelCapabilityValueFromProbe(compactResult)
	if channel.Type != constant.ChannelTypeCodex {
		capability.ResponsesCompactWrite = capability.PlatformResponsesCompactWrite
	}
	if probeMessage := channelCapabilityProbeMessage("Platform Compact", compactResult); probeMessage != "" {
		messages = append(messages, probeMessage)
	}

	capability.LastEndpoint = string(constant.EndpointTypeOpenAI)
	rawMessage := strings.Join(messages, "；")
	rawLower := strings.ToLower(rawMessage)
	if len(messages) == 0 {
		capability.LastMessage = "Platform API 诊断完成"
	} else {
		capability.LastMessage = service.SummarizePlatformAPIDiagnosticMessages(messages)
		if strings.Contains(rawLower, "insufficient_quota") || strings.Contains(rawLower, "exceeded your current quota") {
			capability.CapabilityClassification = channelcapability.ClassificationPlatformQuotaInsufficient
		} else if strings.Contains(rawLower, "api.responses.write") || strings.Contains(rawLower, "missing scopes") || strings.Contains(rawLower, "insufficient permissions") {
			capability.CapabilityClassification = channelcapability.ClassificationPlatformResponsesScopeMiss
		}
	}
	return accountCapabilityProbeResult{
		Index:        credentialIndex,
		Capabilities: capability,
		OAuthJSON:    isOAuthJSONAccountKey(keys[credentialIndex]),
	}, nil
}

func runChannelCapabilityProbeTest(c *gin.Context, channel *model.Channel, testModel string, endpointType string, isStream bool, options channelTestOptions) (*model.Channel, testResult) {
	result := testChannel(channel, testModel, endpointType, isStream, options)
	if result.newAPIError == nil || !channelTestUsesOAuthJSONCredential(channel, options) || !shouldRefreshOAuthJSONAccountAfterChannelTest(result) {
		return channel, result
	}
	refreshedChannel, refreshErr := refreshOAuthJSONAccountAfterChannelTest(c, channel, options)
	if refreshErr != nil {
		result.refreshErr = refreshErr
		return channel, result
	}
	refreshedResult := testChannel(refreshedChannel, testModel, endpointType, isStream, options)
	refreshedResult.refreshed = true
	return refreshedChannel, refreshedResult
}

func channelCapabilityValueFromProbe(result testResult) *bool {
	if result.localErr == nil && result.newAPIError == nil {
		return lo.ToPtr(true)
	}
	if isCapabilityDeniedOrUnsupportedTestResult(result) {
		return lo.ToPtr(false)
	}
	return nil
}

func channelCapabilityProbeMessage(label string, result testResult) string {
	if result.localErr == nil && result.newAPIError == nil {
		return ""
	}
	message := friendlyChannelTestErrorMessage(result)
	if strings.TrimSpace(message) == "" {
		message = "检测失败"
	}
	return label + ": " + message
}

func isMissingResponsesScopeTestResult(result testResult) bool {
	lower := strings.ToLower(channelTestErrorRawText(result))
	return strings.Contains(lower, "api.responses.write") ||
		strings.Contains(lower, "missing scopes") ||
		strings.Contains(lower, "insufficient permissions")
}

func isCapabilityDeniedOrUnsupportedTestResult(result testResult) bool {
	if isMissingResponsesScopeTestResult(result) {
		return true
	}
	lower := strings.ToLower(channelTestErrorRawText(result))
	for _, needle := range []string{
		"unsupported",
		"not supported",
		"404",
		"not found",
		"model_not_found",
		"does not exist",
		"invalid endpoint",
		"unknown url",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func saveChannelAccountCapability(channelID int, credentialIndex int, capability model.ChannelAccountCapability) error {
	lock := model.GetChannelPollingLock(channelID)
	lock.Lock()
	defer lock.Unlock()

	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return err
	}
	keys := channel.GetKeys()
	if credentialIndex < 0 || credentialIndex >= len(keys) {
		return errors.New("账号索引超出范围")
	}
	if channel.ChannelInfo.MultiKeyCapabilities == nil {
		channel.ChannelInfo.MultiKeyCapabilities = make(map[int]model.ChannelAccountCapability)
	}
	channel.ChannelInfo.MultiKeyCapabilities[credentialIndex] = capability
	if err := channel.SaveChannelInfo(); err != nil {
		return err
	}
	model.InitChannelCache()
	return nil
}

func testChannel(channel *model.Channel, testModel string, endpointType string, isStream bool, options ...channelTestOptions) testResult {
	tik := time.Now()
	var unsupportedTestChannelTypes = []int{
		constant.ChannelTypeMidjourney,
		constant.ChannelTypeMidjourneyPlus,
		constant.ChannelTypeSunoAPI,
		constant.ChannelTypeKling,
		constant.ChannelTypeJimeng,
		constant.ChannelTypeDoubaoVideo,
		constant.ChannelTypeVidu,
	}
	if lo.Contains(unsupportedTestChannelTypes, channel.Type) {
		channelTypeName := constant.GetChannelTypeName(channel.Type)
		return testResult{
			localErr: fmt.Errorf("%s channel test is not supported", channelTypeName),
		}
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	testModel = strings.TrimSpace(testModel)
	if testModel == "" {
		if channel.TestModel != nil && *channel.TestModel != "" {
			testModel = strings.TrimSpace(*channel.TestModel)
		} else {
			models := channel.GetModels()
			if len(models) > 0 {
				testModel = strings.TrimSpace(models[0])
			}
			if testModel == "" {
				testModel = "gpt-4o-mini"
			}
		}
	}

	testOptions := firstChannelTestOptions(options)
	endpointType = resolveChannelTestEndpoint(channel, testModel, endpointType, testOptions)

	requestPath := "/v1/chat/completions"

	// 如果指定了端点类型，使用指定的端点类型
	if endpointType != "" {
		if endpointInfo, ok := common.GetDefaultEndpointInfo(constant.EndpointType(endpointType)); ok {
			requestPath = endpointInfo.Path
		}
	} else {
		// 如果没有指定端点类型，使用原有的自动检测逻辑

		if strings.Contains(strings.ToLower(testModel), "rerank") {
			requestPath = "/v1/rerank"
		}

		// 先判断是否为 Embedding 模型
		if strings.Contains(strings.ToLower(testModel), "embedding") ||
			strings.HasPrefix(testModel, "m3e") || // m3e 系列模型
			strings.Contains(testModel, "bge-") || // bge 系列模型
			strings.Contains(testModel, "embed") ||
			channel.Type == constant.ChannelTypeMokaAI { // 其他 embedding 模型
			requestPath = "/v1/embeddings" // 修改请求路径
		}

		// VolcEngine 图像生成模型
		if channel.Type == constant.ChannelTypeVolcEngine && strings.Contains(testModel, "seedream") {
			requestPath = "/v1/images/generations"
		}

		// responses-only models
		if strings.Contains(strings.ToLower(testModel), "codex") {
			requestPath = "/v1/responses"
		}

		// responses compaction models (must use /v1/responses/compact)
		if strings.HasSuffix(testModel, ratio_setting.CompactModelSuffix) {
			requestPath = "/v1/responses/compact"
		}
	}
	if strings.HasPrefix(requestPath, "/v1/responses/compact") {
		testModel = ratio_setting.WithCompactModelSuffix(testModel)
	}

	c.Request = &http.Request{
		Method: "POST",
		URL:    &url.URL{Path: requestPath}, // 使用动态路径
		Body:   nil,
		Header: make(http.Header),
	}

	cache, err := model.GetUserCache(1)
	if err != nil {
		return testResult{
			localErr:    err,
			newAPIError: nil,
		}
	}
	cache.WriteContext(c)
	c.Set("id", 1)

	//c.Request.Header.Set("Authorization", "Bearer "+channel.Key)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("channel", channel.Type)
	c.Set("base_url", channel.GetBaseURL())
	group, _ := model.GetUserGroup(1, false)
	c.Set("group", group)

	newAPIError := middleware.SetupContextForSelectedChannelWithEndpoint(
		c,
		channel,
		testModel,
		constant.EndpointType(endpointType),
		buildChannelTestSelection(channel, testOptions),
	)
	if newAPIError != nil {
		return testResult{
			context:     c,
			localErr:    newAPIError,
			newAPIError: newAPIError,
		}
	}

	// Determine relay format based on endpoint type or request path
	var relayFormat types.RelayFormat
	if endpointType != "" {
		// 根据指定的端点类型设置 relayFormat
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeOpenAI:
			relayFormat = types.RelayFormatOpenAI
		case constant.EndpointTypeOpenAIResponse:
			relayFormat = types.RelayFormatOpenAIResponses
		case constant.EndpointTypeOpenAIResponseCompact:
			relayFormat = types.RelayFormatOpenAIResponsesCompaction
		case constant.EndpointTypeAnthropic:
			relayFormat = types.RelayFormatClaude
		case constant.EndpointTypeGemini:
			relayFormat = types.RelayFormatGemini
		case constant.EndpointTypeJinaRerank:
			relayFormat = types.RelayFormatRerank
		case constant.EndpointTypeImageGeneration, constant.EndpointTypeImageEdit:
			relayFormat = types.RelayFormatOpenAIImage
		case constant.EndpointTypeEmbeddings:
			relayFormat = types.RelayFormatEmbedding
		default:
			relayFormat = types.RelayFormatOpenAI
		}
	} else {
		// 根据请求路径自动检测
		relayFormat = types.RelayFormatOpenAI
		if c.Request.URL.Path == "/v1/embeddings" {
			relayFormat = types.RelayFormatEmbedding
		}
		if c.Request.URL.Path == "/v1/images/generations" {
			relayFormat = types.RelayFormatOpenAIImage
		}
		if c.Request.URL.Path == "/v1/messages" {
			relayFormat = types.RelayFormatClaude
		}
		if strings.Contains(c.Request.URL.Path, "/v1beta/models") {
			relayFormat = types.RelayFormatGemini
		}
		if c.Request.URL.Path == "/v1/rerank" || c.Request.URL.Path == "/rerank" {
			relayFormat = types.RelayFormatRerank
		}
		if c.Request.URL.Path == "/v1/responses" {
			relayFormat = types.RelayFormatOpenAIResponses
		}
		if strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact") {
			relayFormat = types.RelayFormatOpenAIResponsesCompaction
		}
	}

	request := buildTestRequest(testModel, endpointType, channel, isStream)

	info, err := relaycommon.GenRelayInfo(c, relayFormat, request, nil)

	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeGenRelayInfoFailed),
		}
	}

	info.IsChannelTest = true
	info.InitChannelMeta(c)

	if capability, ok := channelTestAccountCapability(channel, testOptions); ok {
		common.SysLog(fmt.Sprintf(
			"channel test route decision: channel_id=%d credential_index=%d responses_write=%s chat_completions_write=%s compact_write=%s allow_proxy_bridge=%t relay_mode=%d endpoint_type=%s request_path=%s",
			channel.Id,
			lo.FromPtrOr(testOptions.CredentialIndex, -1),
			channelTestCapabilityFlagString(capability.ResponsesWrite),
			channelTestCapabilityFlagString(capability.ChatCompletionsWrite),
			channelTestCapabilityFlagString(capability.ResponsesCompactWrite),
			testOptions.AllowProxyBridge,
			info.RelayMode,
			endpointType,
			requestPath,
		))
	}

	err = attachTestBillingRequestInput(info, request)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeJsonMarshalFailed),
		}
	}

	if testOptions.AllowProxyBridge &&
		(info.RelayMode == relayconstant.RelayModeResponses || info.RelayMode == relayconstant.RelayModeResponsesCompact) {
		jsonData, err := common.Marshal(request)
		if err != nil {
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewError(err, types.ErrorCodeJsonMarshalFailed),
			}
		}
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))

		common.SysLog(fmt.Sprintf("testing channel %d with model %s , info %+v ", channel.Id, testModel, info.ToString()))
		newAPIError := relay.ResponsesHelper(c, info)
		if newAPIError != nil {
			return testResult{
				context:     c,
				localErr:    newAPIError,
				newAPIError: newAPIError,
			}
		}

		result := w.Result()
		respBody, err := readTestResponseBody(result.Body, isStream)
		if err != nil {
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError),
			}
		}
		if bodyErr := validateTestResponseBody(respBody, isStream); bodyErr != nil {
			return testResult{
				context:     c,
				localErr:    bodyErr,
				newAPIError: types.NewOpenAIError(bodyErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
			}
		}

		common.SysLog(fmt.Sprintf("testing channel #%d, response: \n%s", channel.Id, string(respBody)))
		return testResult{
			context:     c,
			localErr:    nil,
			newAPIError: nil,
		}
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeChannelModelMappedError),
		}
	}

	testModel = info.UpstreamModelName
	// 更新请求中的模型名称
	request.SetModelName(testModel)

	apiType, _ := common.ChannelType2APIType(channel.Type)
	if info.RelayMode == relayconstant.RelayModeResponsesCompact &&
		apiType != constant.APITypeOpenAI &&
		apiType != constant.APITypeCodex {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("responses compaction test only supports openai/codex channels, got api type %d", apiType),
			newAPIError: types.NewError(fmt.Errorf("unsupported api type: %d", apiType), types.ErrorCodeInvalidApiType),
		}
	}
	adaptor := relay.GetAdaptor(apiType)
	if adaptor == nil {
		return testResult{
			context:     c,
			localErr:    fmt.Errorf("invalid api type: %d, adaptor is nil", apiType),
			newAPIError: types.NewError(fmt.Errorf("invalid api type: %d, adaptor is nil", apiType), types.ErrorCodeInvalidApiType),
		}
	}

	//// 创建一个用于日志的 info 副本，移除 ApiKey
	//logInfo := info
	//logInfo.ApiKey = ""
	common.SysLog(fmt.Sprintf("testing channel %d with model %s , info %+v ", channel.Id, testModel, info.ToString()))

	priceData, err := helper.ModelPriceHelper(c, info, 0, request.GetTokenCountMeta())
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest)),
		}
	}

	adaptor.Init(info)

	var convertedRequest any
	// 根据 RelayMode 选择正确的转换函数
	switch info.RelayMode {
	case relayconstant.RelayModeEmbeddings:
		// Embedding 请求 - request 已经是正确的类型
		if embeddingReq, ok := request.(*dto.EmbeddingRequest); ok {
			convertedRequest, err = adaptor.ConvertEmbeddingRequest(c, info, *embeddingReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid embedding request type"),
				newAPIError: types.NewError(errors.New("invalid embedding request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeImagesGenerations:
		// 图像生成请求 - request 已经是正确的类型
		if imageReq, ok := request.(*dto.ImageRequest); ok {
			convertedRequest, err = adaptor.ConvertImageRequest(c, info, *imageReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid image request type"),
				newAPIError: types.NewError(errors.New("invalid image request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeRerank:
		// Rerank 请求 - request 已经是正确的类型
		if rerankReq, ok := request.(*dto.RerankRequest); ok {
			convertedRequest, err = adaptor.ConvertRerankRequest(c, info.RelayMode, *rerankReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid rerank request type"),
				newAPIError: types.NewError(errors.New("invalid rerank request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeResponses:
		// Response 请求 - request 已经是正确的类型
		if responseReq, ok := request.(*dto.OpenAIResponsesRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, *responseReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid response request type"),
				newAPIError: types.NewError(errors.New("invalid response request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	case relayconstant.RelayModeResponsesCompact:
		// Response compaction request - convert to OpenAIResponsesRequest before adapting
		switch req := request.(type) {
		case *dto.OpenAIResponsesCompactionRequest:
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
				Model:              req.Model,
				Input:              req.Input,
				Instructions:       req.Instructions,
				PreviousResponseID: req.PreviousResponseID,
			})
		case *dto.OpenAIResponsesRequest:
			convertedRequest, err = adaptor.ConvertOpenAIResponsesRequest(c, info, *req)
		default:
			return testResult{
				context:     c,
				localErr:    errors.New("invalid response compaction request type"),
				newAPIError: types.NewError(errors.New("invalid response compaction request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	default:
		// Chat/Completion 等其他请求类型
		if generalReq, ok := request.(*dto.GeneralOpenAIRequest); ok {
			convertedRequest, err = adaptor.ConvertOpenAIRequest(c, info, generalReq)
		} else {
			return testResult{
				context:     c,
				localErr:    errors.New("invalid general request type"),
				newAPIError: types.NewError(errors.New("invalid general request type"), types.ErrorCodeConvertRequestFailed),
			}
		}
	}

	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
		}
	}
	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewError(err, types.ErrorCodeJsonMarshalFailed),
		}
	}

	//jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
	//if err != nil {
	//	return testResult{
	//		context:     c,
	//		localErr:    err,
	//		newAPIError: types.NewError(err, types.ErrorCodeConvertRequestFailed),
	//	}
	//}

	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			if fixedErr, ok := relaycommon.AsParamOverrideReturnError(err); ok {
				return testResult{
					context:     c,
					localErr:    fixedErr,
					newAPIError: relaycommon.NewAPIErrorFromParamOverride(fixedErr),
				}
			}
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid),
			}
		}
	}

	requestBody := bytes.NewBuffer(jsonData)
	c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
		}
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			err := service.RelayErrorHandler(c.Request.Context(), httpResp, true)
			common.SysError(fmt.Sprintf(
				"channel test bad response: channel_id=%d name=%s type=%d model=%s endpoint_type=%s status=%d err=%v",
				channel.Id,
				channel.Name,
				channel.Type,
				testModel,
				endpointType,
				httpResp.StatusCode,
				err,
			))
			return testResult{
				context:     c,
				localErr:    err,
				newAPIError: types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError),
			}
		}
	}
	usageA, respErr := adaptor.DoResponse(c, httpResp, info)
	if respErr != nil {
		return testResult{
			context:     c,
			localErr:    respErr,
			newAPIError: respErr,
		}
	}
	usage, usageErr := coerceTestUsage(usageA, isStream, info.GetEstimatePromptTokens())
	if usageErr != nil {
		return testResult{
			context:     c,
			localErr:    usageErr,
			newAPIError: types.NewOpenAIError(usageErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
		}
	}
	result := w.Result()
	respBody, err := readTestResponseBody(result.Body, isStream)
	if err != nil {
		return testResult{
			context:     c,
			localErr:    err,
			newAPIError: types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError),
		}
	}
	if bodyErr := validateTestResponseBody(respBody, isStream); bodyErr != nil {
		return testResult{
			context:     c,
			localErr:    bodyErr,
			newAPIError: types.NewOpenAIError(bodyErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError),
		}
	}
	info.SetEstimatePromptTokens(usage.PromptTokens)

	quota, tieredResult := settleTestQuota(info, priceData, usage)
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	consumedTime := float64(milliseconds) / 1000.0
	other := buildTestLogOther(c, info, priceData, usage, tieredResult)
	model.RecordConsumeLog(c, 1, model.RecordConsumeLogParams{
		ChannelId:        channel.Id,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        info.OriginModelName,
		TokenName:        "模型测试",
		Quota:            quota,
		Content:          "模型测试",
		UseTimeSeconds:   int(consumedTime),
		IsStream:         info.IsStream,
		Group:            info.UsingGroup,
		Other:            other,
	})
	common.SysLog(fmt.Sprintf("testing channel #%d, response: \n%s", channel.Id, string(respBody)))
	return testResult{
		context:     c,
		localErr:    nil,
		newAPIError: nil,
	}
}

func firstChannelTestOptions(options []channelTestOptions) channelTestOptions {
	if len(options) == 0 {
		return channelTestOptions{}
	}
	return options[0]
}

func channelTestCapabilityFlagString(value *bool) string {
	if value == nil {
		return "nil"
	}
	if *value {
		return "true"
	}
	return "false"
}

func buildChannelTestSelection(channel *model.Channel, options channelTestOptions) *modelgatewayintegration.SelectionResult {
	if channel == nil || options.CredentialIndex == nil {
		return nil
	}
	return &modelgatewayintegration.SelectionResult{
		Channel: channel,
		Group:   channel.Group,
		Plan: &modelgatewaycore.DispatchPlan{
			Channel:       channel,
			SelectedGroup: channel.Group,
			CredentialRef: modelgatewaycore.CredentialRef{
				ResourceID:      fmt.Sprintf("platform:channel:%d", channel.Id),
				CredentialIndex: *options.CredentialIndex,
				Resolver:        "channel_key",
			},
		},
		SmartHandled: true,
	}
}

func attachTestBillingRequestInput(info *relaycommon.RelayInfo, request dto.Request) error {
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

func settleTestQuota(info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage) (int, *billingexpr.TieredResult) {
	if usage != nil && info != nil && info.TieredBillingSnapshot != nil {
		isClaudeUsageSemantic := usage.UsageSemantic == "anthropic" || info.GetFinalRequestRelayFormat() == types.RelayFormatClaude
		usedVars := billingexpr.UsedVars(info.TieredBillingSnapshot.ExprString)
		if ok, quota, result := service.TryTieredSettle(info, service.BuildTieredTokenParams(usage, isClaudeUsageSemantic, usedVars)); ok {
			return quota, result
		}
	}

	quota := 0
	if !priceData.UsePrice {
		quota = usage.PromptTokens + int(math.Round(float64(usage.CompletionTokens)*priceData.CompletionRatio))
		quota = int(math.Round(float64(quota) * priceData.ModelRatio))
		if priceData.ModelRatio != 0 && quota <= 0 {
			quota = 1
		}
		return quota, nil
	}

	return int(priceData.ModelPrice * common.QuotaPerUnit), nil
}

func buildTestLogOther(c *gin.Context, info *relaycommon.RelayInfo, priceData types.PriceData, usage *dto.Usage, tieredResult *billingexpr.TieredResult) map[string]interface{} {
	other := service.GenerateTextOtherInfo(c, info, priceData.ModelRatio, priceData.GroupRatioInfo.GroupRatio, priceData.CompletionRatio,
		usage.PromptTokensDetails.CachedTokens, priceData.CacheRatio, priceData.ModelPrice, priceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		service.InjectTieredBillingInfo(other, info, tieredResult)
	}
	return other
}

func coerceTestUsage(usageAny any, isStream bool, estimatePromptTokens int) (*dto.Usage, error) {
	switch u := usageAny.(type) {
	case *dto.Usage:
		return u, nil
	case dto.Usage:
		return &u, nil
	case nil:
		if !isStream {
			return nil, errors.New("usage is nil")
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	default:
		if !isStream {
			return nil, fmt.Errorf("invalid usage type: %T", usageAny)
		}
		usage := &dto.Usage{
			PromptTokens: estimatePromptTokens,
		}
		usage.TotalTokens = usage.PromptTokens
		return usage, nil
	}
}

func readTestResponseBody(body io.ReadCloser, isStream bool) ([]byte, error) {
	defer func() { _ = body.Close() }()
	const maxStreamLogBytes = 8 << 10
	if isStream {
		return io.ReadAll(io.LimitReader(body, maxStreamLogBytes))
	}
	return io.ReadAll(body)
}

func detectErrorFromTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return nil
	}
	if message := detectErrorMessageFromJSONBytes(b); message != "" {
		return fmt.Errorf("upstream error: %s", message)
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}
		if message := detectErrorMessageFromJSONBytes(payload); message != "" {
			return fmt.Errorf("upstream error: %s", message)
		}
	}

	return nil
}

func validateStreamTestResponseBody(respBody []byte) error {
	b := bytes.TrimSpace(respBody)
	if len(b) == 0 {
		return errors.New("stream response body is empty")
	}

	for _, line := range bytes.Split(b, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
			continue
		}

		return nil
	}

	return errors.New("stream response body does not contain a valid stream event")
}

func validateTestResponseBody(respBody []byte, isStream bool) error {
	if bodyErr := detectErrorFromTestResponseBody(respBody); bodyErr != nil {
		return bodyErr
	}
	if isStream {
		return validateStreamTestResponseBody(respBody)
	}
	return nil
}

func shouldUseStreamForAutomaticChannelTest(channel *model.Channel) bool {
	return channel != nil && channel.Type == constant.ChannelTypeCodex
}

func detectErrorMessageFromJSONBytes(jsonBytes []byte) string {
	if len(jsonBytes) == 0 {
		return ""
	}
	if jsonBytes[0] != '{' && jsonBytes[0] != '[' {
		return ""
	}
	errVal := gjson.GetBytes(jsonBytes, "error")
	if !errVal.Exists() || errVal.Type == gjson.Null {
		return ""
	}

	message := gjson.GetBytes(jsonBytes, "error.message").String()
	if message == "" {
		message = gjson.GetBytes(jsonBytes, "error.error.message").String()
	}
	if message == "" && errVal.Type == gjson.String {
		message = errVal.String()
	}
	if message == "" {
		message = errVal.Raw
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return "upstream returned error payload"
	}
	return message
}

func buildTestRequest(model string, endpointType string, channel *model.Channel, isStream bool) dto.Request {
	testResponsesInput := json.RawMessage(`[{"role":"user","content":"hi"}]`)

	// 根据端点类型构建不同的测试请求
	if endpointType != "" {
		switch constant.EndpointType(endpointType) {
		case constant.EndpointTypeEmbeddings:
			// 返回 EmbeddingRequest
			return &dto.EmbeddingRequest{
				Model: model,
				Input: []any{"hello world"},
			}
		case constant.EndpointTypeImageGeneration:
			// 返回 ImageRequest
			return &dto.ImageRequest{
				Model:  model,
				Prompt: "a cute cat",
				N:      lo.ToPtr(uint(1)),
				Size:   "1024x1024",
			}
		case constant.EndpointTypeImageEdit:
			// 返回 ImageRequest
			return &dto.ImageRequest{
				Model:  model,
				Prompt: "make the image brighter",
				N:      lo.ToPtr(uint(1)),
				Size:   "1024x1024",
				Image:  json.RawMessage(`"data:image/png;base64,iVBORw0KGgo="`),
			}
		case constant.EndpointTypeJinaRerank:
			// 返回 RerankRequest
			return &dto.RerankRequest{
				Model:     model,
				Query:     "What is Deep Learning?",
				Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
				TopN:      lo.ToPtr(2),
			}
		case constant.EndpointTypeOpenAIResponse:
			// 返回 OpenAIResponsesRequest
			return &dto.OpenAIResponsesRequest{
				Model:  model,
				Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
				Stream: lo.ToPtr(isStream),
			}
		case constant.EndpointTypeOpenAIResponseCompact:
			// 返回 OpenAIResponsesCompactionRequest
			return &dto.OpenAIResponsesCompactionRequest{
				Model: model,
				Input: testResponsesInput,
			}
		case constant.EndpointTypeAnthropic, constant.EndpointTypeGemini, constant.EndpointTypeOpenAI:
			// 返回 GeneralOpenAIRequest
			maxTokens := uint(16)
			if constant.EndpointType(endpointType) == constant.EndpointTypeGemini {
				maxTokens = 3000
			}
			req := &dto.GeneralOpenAIRequest{
				Model:  model,
				Stream: lo.ToPtr(isStream),
				Messages: []dto.Message{
					{
						Role:    "user",
						Content: "hi",
					},
				},
				MaxTokens: lo.ToPtr(maxTokens),
			}
			if isStream {
				req.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
			}
			return req
		}
	}

	// 自动检测逻辑（保持原有行为）
	if strings.Contains(strings.ToLower(model), "rerank") {
		return &dto.RerankRequest{
			Model:     model,
			Query:     "What is Deep Learning?",
			Documents: []any{"Deep Learning is a subset of machine learning.", "Machine learning is a field of artificial intelligence."},
			TopN:      lo.ToPtr(2),
		}
	}

	// 先判断是否为 Embedding 模型
	if strings.Contains(strings.ToLower(model), "embedding") ||
		strings.HasPrefix(model, "m3e") ||
		strings.Contains(model, "bge-") {
		// 返回 EmbeddingRequest
		return &dto.EmbeddingRequest{
			Model: model,
			Input: []any{"hello world"},
		}
	}

	// Responses compaction models (must use /v1/responses/compact)
	if strings.HasSuffix(model, ratio_setting.CompactModelSuffix) {
		return &dto.OpenAIResponsesCompactionRequest{
			Model: model,
			Input: testResponsesInput,
		}
	}

	// Responses-only models (e.g. codex series)
	if strings.Contains(strings.ToLower(model), "codex") {
		return &dto.OpenAIResponsesRequest{
			Model:  model,
			Input:  json.RawMessage(`[{"role":"user","content":"hi"}]`),
			Stream: lo.ToPtr(isStream),
		}
	}

	// Chat/Completion 请求 - 返回 GeneralOpenAIRequest
	testRequest := &dto.GeneralOpenAIRequest{
		Model:  model,
		Stream: lo.ToPtr(isStream),
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hi",
			},
		},
	}
	if isStream {
		testRequest.StreamOptions = &dto.StreamOptions{IncludeUsage: true}
	}

	if strings.HasPrefix(model, "o") {
		testRequest.MaxCompletionTokens = lo.ToPtr(uint(16))
	} else if strings.Contains(model, "thinking") {
		if !strings.Contains(model, "claude") {
			testRequest.MaxTokens = lo.ToPtr(uint(50))
		}
	} else if strings.Contains(model, "gemini") {
		testRequest.MaxTokens = lo.ToPtr(uint(3000))
	} else {
		testRequest.MaxTokens = lo.ToPtr(uint(16))
	}

	return testRequest
}

func TestChannel(c *gin.Context) {
	channelId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channel, err := model.CacheGetChannel(channelId)
	if err != nil {
		channel, err = model.GetChannelById(channelId, true)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	//defer func() {
	//	if channel.ChannelInfo.IsMultiKey {
	//		go func() { _ = channel.SaveChannelInfo() }()
	//	}
	//}()
	testModel := c.Query("model")
	endpointType := c.Query("endpoint_type")
	isStream, _ := strconv.ParseBool(c.Query("stream"))
	testOptions, ok := parseChannelTestOptions(c)
	if !ok {
		return
	}
	tik := time.Now()
	result := testChannel(channel, testModel, endpointType, isStream, testOptions)
	if result.newAPIError != nil &&
		channelTestUsesOAuthJSONCredential(channel, testOptions) &&
		shouldRefreshOAuthJSONAccountAfterChannelTest(result) {
		refreshedChannel, refreshErr := refreshOAuthJSONAccountAfterChannelTest(c, channel, testOptions)
		if refreshErr != nil {
			result.refreshErr = refreshErr
			common.SysError(fmt.Sprintf(
				"channel test oauth json refresh failed: channel_id=%d name=%s err=%v",
				channel.Id,
				channel.Name,
				refreshErr,
			))
		} else {
			channel = refreshedChannel
			result = testChannel(channel, testModel, endpointType, isStream, testOptions)
			result.refreshed = true
		}
	}
	if result.localErr != nil {
		shouldMarkBalanceInsufficient := markChannelBalanceInsufficientFromTest(channel, result)
		resp := gin.H{
			"success": false,
			"message": friendlyChannelTestErrorMessage(result),
			"time":    0.0,
		}
		if result.refreshed {
			resp["oauth_refreshed"] = true
		}
		if result.newAPIError != nil {
			resp["error_code"] = result.newAPIError.GetErrorCode()
			if shouldMarkBalanceInsufficient {
				resp["balance_insufficient"] = true
				resp["status_reason"] = service.ChannelStatusReasonBalanceInsufficient
			}
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	tok := time.Now()
	milliseconds := tok.Sub(tik).Milliseconds()
	consumedTime := float64(milliseconds) / 1000.0
	if result.newAPIError != nil {
		go channel.UpdateResponseTime(milliseconds)
		shouldMarkBalanceInsufficient := markChannelBalanceInsufficientFromTest(channel, result)
		resp := gin.H{
			"success":              false,
			"message":              friendlyChannelTestErrorMessage(result),
			"time":                 consumedTime,
			"error_code":           result.newAPIError.GetErrorCode(),
			"balance_insufficient": shouldMarkBalanceInsufficient,
			"status_reason":        service.ChannelStatusReasonBalanceInsufficient,
		}
		if result.refreshed {
			resp["oauth_refreshed"] = true
		}
		c.JSON(http.StatusOK, resp)
		return
	}
	status, balanceInsufficientCleared := clearChannelBalanceInsufficientFromSuccessfulTest(channel, result)
	go channel.UpdateResponseTime(milliseconds)
	resp := gin.H{
		"success":                      true,
		"message":                      "",
		"time":                         consumedTime,
		"status":                       status,
		"balance_insufficient":         false,
		"balance_insufficient_cleared": balanceInsufficientCleared,
	}
	if result.refreshed {
		resp["oauth_refreshed"] = true
	}
	c.JSON(http.StatusOK, resp)
}

func friendlyChannelTestErrorMessage(result testResult) string {
	raw := channelTestErrorRawText(result)
	lower := strings.ToLower(raw)

	if result.refreshErr != nil {
		refreshText := strings.ToLower(result.refreshErr.Error())
		if strings.Contains(refreshText, "status=401") || strings.Contains(refreshText, "unauthorized") {
			return "账号授权已失效，自动刷新也失败了。refresh_token 可能已失效，请重新从 xauto 下载账号数据后导入。"
		}
		return "账号授权已失效，自动刷新失败。请检查账号代理是否可用，或重新从 xauto 下载账号数据后导入。"
	}

	if strings.Contains(lower, "api.responses.write") || strings.Contains(lower, "missing scopes") || strings.Contains(lower, "insufficient permissions") {
		return "账号权限不足，缺少 Responses API 写入权限（api.responses.write）。请重新授权/导入带该权限的账号，或检查 OpenAI 组织和项目角色。"
	}
	if strings.Contains(lower, "insufficient_quota") || strings.Contains(lower, "exceeded your current quota") {
		return "Platform API 额度不足或未开通计费；这不影响 Codex backend 调度。请使用“检测 Codex 能力”确认账号可用性。"
	}
	if strings.Contains(lower, "token_invalidated") || strings.Contains(lower, "authentication token has been invalidated") {
		if result.refreshed {
			return "账号授权已失效，自动刷新后仍不可用。请重新从 xauto 下载账号数据后导入。"
		}
		return "账号授权已失效，请重新从 xauto 下载账号数据后导入。"
	}
	if strings.Contains(lower, "connection refused") {
		return "账号绑定的代理连接被拒绝，代理服务可能没启动、端口未开放或安全组未放行。请更换/取消代理，或检查代理地址和端口。"
	}
	if strings.Contains(lower, "i/o timeout") || strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "client.timeout") || strings.Contains(lower, "timeout") {
		return "请求上游超时，通常是账号代理网络不通或太慢。请检查代理连通性，或更换更稳定的代理。"
	}
	if strings.Contains(lower, "no such host") || strings.Contains(lower, "server misbehaving") {
		return "域名解析失败，通常是代理或服务器 DNS 异常。请检查代理服务器的 DNS 和网络出口。"
	}
	if strings.Contains(lower, "socks connect") || strings.Contains(lower, "proxyconnect") || strings.Contains(lower, "proxy") {
		return "账号代理连接失败。请检查代理协议、地址、端口、账号密码和服务器防火墙，或更换代理后再测试。"
	}
	if strings.Contains(lower, "do request failed") {
		return "请求上游失败，通常是网络或代理问题。请检查该账号绑定的代理是否可用，再重新测试。"
	}

	if strings.TrimSpace(raw) != "" {
		return raw
	}
	return "测试失败，请检查账号凭证、代理和渠道配置后重试。"
}

func channelTestErrorRawText(result testResult) string {
	parts := make([]string, 0, 4)
	if result.localErr != nil {
		parts = append(parts, result.localErr.Error())
	}
	if result.newAPIError != nil {
		parts = append(parts, result.newAPIError.Error(), string(result.newAPIError.GetErrorCode()))
	}
	if result.refreshErr != nil {
		parts = append(parts, result.refreshErr.Error())
	}
	if result.context != nil {
		if upstreamRequest, ok := common.GetContextKeyType[map[string]interface{}](result.context, constant.ContextKeyUpstreamRequestInfo); ok {
			for _, key := range []string{"error", "error_kind", "host", "path"} {
				if value, ok := upstreamRequest[key]; ok {
					parts = append(parts, fmt.Sprintf("%v", value))
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

func shouldRefreshOAuthJSONAccountAfterChannelTest(result testResult) bool {
	if result.newAPIError == nil {
		return false
	}
	if strings.EqualFold(string(result.newAPIError.GetErrorCode()), "token_invalidated") {
		return true
	}
	message := strings.ToLower(result.newAPIError.Error())
	return strings.Contains(message, "token_invalidated") ||
		strings.Contains(message, "authentication token has been invalidated")
}

func refreshOAuthJSONAccountAfterChannelTest(c *gin.Context, channel *model.Channel, options channelTestOptions) (*model.Channel, error) {
	if c == nil || c.Request == nil {
		return nil, errors.New("missing request context")
	}
	credentialIndex, ok := channelTestOAuthJSONCredentialIndex(channel, options)
	if !ok {
		return nil, errors.New("selected credential is not oauth json")
	}
	proxyURL, err := channelTestCredentialProxyURL(channel, credentialIndex)
	if err != nil {
		return nil, err
	}
	_, refreshedChannel, err := service.RefreshCodexAccountCredential(c.Request.Context(), channel.Id, service.CodexAccountCredentialRefreshOptions{
		CredentialIndex: credentialIndex,
		ProxyURL:        proxyURL,
		ResetCaches:     true,
	})
	if err != nil {
		return nil, err
	}
	if refreshedChannel == nil {
		return nil, errors.New("refreshed channel is empty")
	}
	return refreshedChannel, nil
}

func channelTestCredentialProxyURL(channel *model.Channel, credentialIndex int) (string, error) {
	if channel == nil || channel.ChannelInfo.MultiKeyProxyIDs == nil || credentialIndex < 0 {
		return "", nil
	}
	proxyID := channel.ChannelInfo.MultiKeyProxyIDs[credentialIndex]
	if proxyID <= 0 {
		return "", nil
	}
	proxyConfig, err := model.GetModelGatewayProxyByID(proxyID)
	if err != nil {
		return "", fmt.Errorf("credential proxy not found: proxy_id=%d: %w", proxyID, err)
	}
	if proxyConfig == nil || !proxyConfig.Enabled {
		return "", fmt.Errorf("credential proxy disabled: proxy_id=%d", proxyID)
	}
	proxyURL, err := proxyConfig.ProxyURL()
	if err != nil {
		return "", fmt.Errorf("credential proxy invalid: proxy_id=%d: %w", proxyID, err)
	}
	return proxyURL, nil
}

func parseChannelTestOptions(c *gin.Context) (channelTestOptions, bool) {
	value := strings.TrimSpace(c.Query("credential_index"))
	if value == "" {
		return channelTestOptions{}, true
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < 0 {
		common.ApiErrorMsg(c, "账号索引无效")
		return channelTestOptions{}, false
	}
	return channelTestOptions{
		CredentialIndex:  &index,
		AllowProxyBridge: true,
	}, true
}

func markChannelBalanceInsufficientFromTest(channel *model.Channel, result testResult) bool {
	if channel == nil || result.newAPIError == nil {
		return false
	}
	if !service.ShouldDisableChannelForBalance(result.newAPIError) || !channel.GetAutoBan() {
		return false
	}
	usingKey := ""
	if result.context != nil {
		usingKey = common.GetContextKeyString(result.context, constant.ContextKeyChannelKey)
	}
	service.DisableChannelForBalance(*types.NewChannelError(
		channel.Id,
		channel.Type,
		channel.Name,
		channel.ChannelInfo.IsMultiKey,
		usingKey,
		channel.GetAutoBan(),
	))
	return true
}

func clearChannelBalanceInsufficientFromSuccessfulTest(channel *model.Channel, result testResult) (int, bool) {
	if channel == nil {
		return 0, false
	}
	usingKey := ""
	if result.context != nil {
		usingKey = common.GetContextKeyString(result.context, constant.ContextKeyChannelKey)
	}
	return service.ClearChannelBalanceInsufficientAfterSuccess(channel.Id, usingKey, channel.Name)
}

var testAllChannelsLock sync.Mutex
var testAllChannelsRunning bool = false

func testAllChannels(notify bool) error {

	testAllChannelsLock.Lock()
	if testAllChannelsRunning {
		testAllChannelsLock.Unlock()
		return errors.New("测试已在运行中")
	}
	testAllChannelsRunning = true
	testAllChannelsLock.Unlock()
	channels, getChannelErr := model.GetAllChannels(0, 0, true, false)
	if getChannelErr != nil {
		return getChannelErr
	}
	var disableThreshold = int64(common.ChannelDisableThreshold * 1000)
	if disableThreshold == 0 {
		disableThreshold = 10000000 // a impossible value
	}
	gopool.Go(func() {
		// 使用 defer 确保无论如何都会重置运行状态，防止死锁
		defer func() {
			testAllChannelsLock.Lock()
			testAllChannelsRunning = false
			testAllChannelsLock.Unlock()
		}()

		for _, channel := range channels {
			if channel.Status == common.ChannelStatusManuallyDisabled {
				continue
			}
			if service.IsErrorPausedChannel(channel) && !service.ShouldResumeErrorPausedChannel(channel, nil) {
				continue
			}
			isChannelEnabled := channel.Status == common.ChannelStatusEnabled
			tik := time.Now()
			result := testChannel(channel, "", "", shouldUseStreamForAutomaticChannelTest(channel))
			tok := time.Now()
			milliseconds := tok.Sub(tik).Milliseconds()

			shouldBanChannel := false
			newAPIError := result.newAPIError
			// request error disables the channel
			if newAPIError != nil {
				shouldBanChannel = service.ShouldDisableChannelForBalance(newAPIError) || service.ShouldDisableChannel(result.newAPIError)
			}

			// 当错误检查通过，才检查响应时间
			if common.AutomaticDisableChannelEnabled && !shouldBanChannel {
				if milliseconds > disableThreshold {
					err := fmt.Errorf("响应时间 %.2fs 超过阈值 %.2fs", float64(milliseconds)/1000.0, float64(disableThreshold)/1000.0)
					newAPIError = types.NewOpenAIError(err, types.ErrorCodeChannelResponseTimeExceeded, http.StatusRequestTimeout)
					shouldBanChannel = true
				}
			}

			// disable channel
			if isChannelEnabled && shouldBanChannel && channel.GetAutoBan() {
				processChannelError(result.context, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError, true)
			}

			// enable channel
			if !isChannelEnabled &&
				service.ShouldEnableChannel(newAPIError, channel.Status) &&
				!service.IsBalanceInsufficientPausedChannel(channel) &&
				(!service.IsErrorPausedChannel(channel) || service.ShouldResumeErrorPausedChannel(channel, newAPIError)) {
				service.EnableChannel(channel.Id, common.GetContextKeyString(result.context, constant.ContextKeyChannelKey), channel.Name)
			}

			channel.UpdateResponseTime(milliseconds)
			time.Sleep(common.RequestInterval)
		}

		if notify {
			service.NotifyRootUser(dto.NotifyTypeChannelTest, "通道测试完成", "所有通道测试已完成")
		}
	})
	return nil
}

func TestAllChannels(c *gin.Context) {
	err := testAllChannels(true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

var autoTestChannelsOnce sync.Once

func AutomaticallyTestChannels() {
	// 只在Master节点定时测试渠道
	if !common.IsMasterNode {
		return
	}
	autoTestChannelsOnce.Do(func() {
		for {
			if !operation_setting.GetMonitorSetting().AutoTestChannelEnabled {
				time.Sleep(1 * time.Minute)
				continue
			}
			for {
				frequency := operation_setting.GetMonitorSetting().AutoTestChannelMinutes
				time.Sleep(time.Duration(int(math.Round(frequency))) * time.Minute)
				common.SysLog(fmt.Sprintf("automatically test channels with interval %f minutes", frequency))
				common.SysLog("automatically testing all channels")
				_ = testAllChannels(false)
				common.SysLog("automatically channel test finished")
				if !operation_setting.GetMonitorSetting().AutoTestChannelEnabled {
					break
				}
			}
		}
	})
}
