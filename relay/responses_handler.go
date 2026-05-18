package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	appconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

var imageGenerationIntentPattern = regexp.MustCompile(`(?i)((生成|画|绘制|出图|作图|创建|制作|设计|generate|create|draw|make|render|illustrate).{0,160}(图片|图像|照片|海报|插画|图标|image|picture|photo|poster|illustration|logo|icon)|(图片|图像|照片|海报|插画|图标|image|picture|photo|poster|illustration|logo|icon).{0,160}(生成|画|绘制|出图|作图|创建|制作|设计|generate|create|draw|make|render|illustrate)|(画|绘制|生成|创建|制作|设计).{0,20}(一张|一个|一幅|一份))`)

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

func maybeInjectImageGenerationTool(request *dto.OpenAIResponsesRequest) (bool, error) {
	if request == nil || request.HasTool(dto.BuildInToolImageGeneration) {
		return false, nil
	}
	if !shouldInjectImageGenerationTool(request) {
		return false, nil
	}
	if err := request.AddTool(map[string]any{
		"type":          dto.BuildInToolImageGeneration,
		"output_format": "png",
	}); err != nil {
		return false, err
	}
	if err := request.SetToolChoice(map[string]any{
		"type": dto.BuildInToolImageGeneration,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func shouldInjectImageGenerationTool(request *dto.OpenAIResponsesRequest) bool {
	if request == nil || common.IsImageGenerationModel(request.Model) {
		return false
	}
	text := responsesImageIntentText(request)
	return imageGenerationIntentPattern.MatchString(text)
}

func responsesImageIntentText(request *dto.OpenAIResponsesRequest) string {
	if request == nil {
		return ""
	}
	parts := make([]string, 0)
	parts = appendRawJSONStrings(parts, request.Instructions)
	parts = appendRawJSONStrings(parts, request.Text)
	parts = appendRawJSONStrings(parts, request.Prompt)
	parts = appendRawJSONStrings(parts, request.Input)
	for _, input := range request.ParseInput() {
		if input.Text != "" {
			parts = append(parts, input.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func appendRawJSONStrings(parts []string, raw []byte) []string {
	if len(raw) == 0 {
		return parts
	}

	var value any
	if err := common.Unmarshal(raw, &value); err != nil {
		return appendNonBlankString(parts, string(raw))
	}
	return appendJSONStrings(parts, value)
}

func appendJSONStrings(parts []string, value any) []string {
	switch typed := value.(type) {
	case string:
		return appendNonBlankString(parts, typed)
	case []any:
		for _, item := range typed {
			parts = appendJSONStrings(parts, item)
		}
	case map[string]any:
		for _, key := range []string{"input", "instructions", "prompt", "content", "text"} {
			if nested, ok := typed[key]; ok {
				parts = appendJSONStrings(parts, nested)
			}
		}
	}
	return parts
}

func appendNonBlankString(parts []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return parts
	}
	return append(parts, value)
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
	imageGenerationToolInjected, err := maybeInjectImageGenerationTool(request)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to inject image generation tool: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if imageGenerationToolInjected ||
		(!model_setting.GetGlobalSettings().PassThroughRequestEnabled && !info.ChannelSetting.PassThroughBodyEnabled) {
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
	if (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) && !imageGenerationToolInjected {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewError(err, types.ErrorCodeReadRequestBodyFailed, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.ReaderOnly(storage)
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

		if common.DebugEnabled {
			println("requestBody: ", string(jsonData))
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
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

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
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
