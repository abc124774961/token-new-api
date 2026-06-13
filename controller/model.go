package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel/ai360"
	"github.com/QuantumNous/new-api/relay/channel/lingyiwanwu"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	"github.com/QuantumNous/new-api/relay/channel/moonshot"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// https://platform.openai.com/docs/api-reference/models/list

var openAIModels []dto.OpenAIModels
var openAIModelsMap map[string]dto.OpenAIModels
var channelId2Models map[int][]string

func init() {
	// https://platform.openai.com/docs/models/model-endpoint-compatibility
	for i := 0; i < constant.APITypeDummy; i++ {
		if i == constant.APITypeAIProxyLibrary {
			continue
		}
		adaptor := relay.GetAdaptor(i)
		channelName := adaptor.GetChannelName()
		modelNames := adaptor.GetModelList()
		for _, modelName := range modelNames {
			openAIModels = append(openAIModels, dto.OpenAIModels{
				Id:      modelName,
				Object:  "model",
				Created: 1626777600,
				OwnedBy: channelName,
			})
		}
	}
	for _, modelName := range ai360.ModelList {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: ai360.ChannelName,
		})
	}
	for _, modelName := range moonshot.ModelList {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: moonshot.ChannelName,
		})
	}
	for _, modelName := range lingyiwanwu.ModelList {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: lingyiwanwu.ChannelName,
		})
	}
	for _, modelName := range minimax.ModelList {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: minimax.ChannelName,
		})
	}
	for modelName, _ := range constant.MidjourneyModel2Action {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: "midjourney",
		})
	}
	openAIModelsMap = make(map[string]dto.OpenAIModels)
	for _, aiModel := range openAIModels {
		openAIModelsMap[aiModel.Id] = aiModel
	}
	channelId2Models = make(map[int][]string)
	for i := 1; i <= constant.ChannelTypeDummy; i++ {
		apiType, success := common.ChannelType2APIType(i)
		if !success || apiType == constant.APITypeAIProxyLibrary {
			continue
		}
		meta := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: i,
		}}
		adaptor := relay.GetAdaptor(apiType)
		adaptor.Init(meta)
		channelId2Models[i] = adaptor.GetModelList()
	}
	openAIModels = lo.UniqBy(openAIModels, func(m dto.OpenAIModels) string {
		return m.Id
	})
}

func ListModels(c *gin.Context, modelType int) {
	acceptUnsetRatioModel := resolveAcceptUnsetRatioModel(c)

	userOpenAiModels := getVisibleOpenAIModels(c, acceptUnsetRatioModel)
	if shouldReturnCodexModels(c, modelType) {
		codexModels := buildCodexModels(userOpenAiModels)
		codexResponse := buildCodexModelsResponse(c, codexModels)
		logCodexModelsTrace(c, codexResponse, "list")
		setCodexModelResponseHeaders(c)
		c.JSON(200, codexResponse)
		return
	}

	switch modelType {
	case constant.ChannelTypeAnthropic:
		useranthropicModels := make([]dto.AnthropicModel, len(userOpenAiModels))
		for i, model := range userOpenAiModels {
			useranthropicModels[i] = dto.AnthropicModel{
				ID:          model.Id,
				CreatedAt:   time.Unix(int64(model.Created), 0).UTC().Format(time.RFC3339),
				DisplayName: model.Id,
				Type:        "model",
			}
		}
		c.JSON(200, gin.H{
			"data":     useranthropicModels,
			"first_id": useranthropicModels[0].ID,
			"has_more": false,
			"last_id":  useranthropicModels[len(useranthropicModels)-1].ID,
		})
	case constant.ChannelTypeGemini:
		userGeminiModels := make([]dto.GeminiModel, len(userOpenAiModels))
		for i, model := range userOpenAiModels {
			userGeminiModels[i] = dto.GeminiModel{
				Name:        model.Id,
				DisplayName: model.Id,
			}
		}
		c.JSON(200, gin.H{
			"models":        userGeminiModels,
			"nextPageToken": nil,
		})
	default:
		c.JSON(200, gin.H{
			"success": true,
			"data":    userOpenAiModels,
			"object":  "list",
		})
	}
}

func resolveAcceptUnsetRatioModel(c *gin.Context) bool {
	if operation_setting.SelfUseModeEnabled {
		return true
	}
	userId := c.GetInt("id")
	if userId <= 0 {
		return false
	}
	userSettings, _ := model.GetUserSetting(userId, false)
	return userSettings.AcceptUnsetRatioModel
}

func getVisibleOpenAIModels(c *gin.Context, acceptUnsetRatioModel bool) []dto.OpenAIModels {
	userOpenAiModels := make([]dto.OpenAIModels, 0)

	modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	if modelLimitEnable {
		s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		var tokenModelLimit map[string]bool
		if ok {
			tokenModelLimit = s.(map[string]bool)
		} else {
			tokenModelLimit = map[string]bool{}
		}
		for allowModel, _ := range tokenModelLimit {
			if !acceptUnsetRatioModel {
				if !helper.HasModelBillingConfig(allowModel) {
					continue
				}
			}
			if oaiModel, ok := openAIModelsMap[allowModel]; ok {
				oaiModel.SupportedEndpointTypes = model.GetModelSupportEndpointTypes(allowModel)
				userOpenAiModels = append(userOpenAiModels, oaiModel)
			} else {
				userOpenAiModels = append(userOpenAiModels, dto.OpenAIModels{
					Id:                     allowModel,
					Object:                 "model",
					Created:                1626777600,
					OwnedBy:                "custom",
					SupportedEndpointTypes: model.GetModelSupportEndpointTypes(allowModel),
				})
			}
		}
		return finalizeVisibleOpenAIModels(c, userOpenAiModels)
	}

	models, ok := resolveVisibleModelNames(c)
	if !ok {
		return userOpenAiModels
	}
	for _, modelName := range models {
		if !acceptUnsetRatioModel {
			if !helper.HasModelBillingConfig(modelName) {
				continue
			}
		}
		if oaiModel, ok := openAIModelsMap[modelName]; ok {
			oaiModel.SupportedEndpointTypes = model.GetModelSupportEndpointTypes(modelName)
			userOpenAiModels = append(userOpenAiModels, oaiModel)
		} else {
			userOpenAiModels = append(userOpenAiModels, dto.OpenAIModels{
				Id:                     modelName,
				Object:                 "model",
				Created:                1626777600,
				OwnedBy:                "custom",
				SupportedEndpointTypes: model.GetModelSupportEndpointTypes(modelName),
			})
		}
	}

	return finalizeVisibleOpenAIModels(c, userOpenAiModels)
}

func resolveVisibleModelNames(c *gin.Context) ([]string, bool) {
	groups, ok := resolveVisibleModelGroups(c)
	if !ok {
		return nil, false
	}

	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if tokenGroup == "" {
		tokenGroup = service.AutoGroupName
	}
	if len(groups) == 1 && tokenGroup != service.AutoGroupName {
		return model.GetAvailableModelsForGroups(service.EffectiveRoutingGroups(groups[0])), true
	}
	return model.GetAvailableModelsForGroups(groups), true
}

func resolveVisibleModelGroups(c *gin.Context) ([]string, bool) {
	userId := c.GetInt("id")
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if userGroup == "" {
		userGroup = c.GetString("group")
	}
	if userGroup == "" && userId > 0 && model.DB != nil {
		var err error
		userGroup, err = model.GetUserGroup(userId, false)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "get user group failed",
			})
			return nil, false
		}
	}

	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if tokenGroup == "" {
		tokenGroup = service.AutoGroupName
	}
	switch tokenGroup {
	case service.AutoGroupName:
		return service.GetUserAutoGroup(userGroup), true
	default:
		return service.EffectiveRoutingGroups(tokenGroup), true
	}
}

func finalizeVisibleOpenAIModels(c *gin.Context, models []dto.OpenAIModels) []dto.OpenAIModels {
	models = sortVisibleModels(models)
	if len(models) == 0 {
		return models
	}

	modelNames := make([]string, 0, len(models))
	for _, item := range models {
		modelNames = append(modelNames, item.Id)
	}

	actualModelByName := make(map[string]string)
	capabilitiesByModel := make(map[string]model.GroupModelCapability)
	if groups, ok := resolveVisibleModelGroups(c); ok {
		actualModelByName = model.GetPreferredActualModelsForGroups(groups, modelNames)
		capabilitiesByModel = model.GetModelCapabilitiesForGroups(groups, modelNames)
	}

	for i := range models {
		modelCodexImageToolSupported := false
		groupCodexImageToolSupported := false
		if capability, ok := capabilitiesByModel[models[i].Id]; ok {
			models[i].SupportedEndpointTypes = capability.SupportedEndpointTypes
			modelCodexImageToolSupported = capability.CodexImageGenerationToolSupported
			groupCodexImageToolSupported = capability.GroupCodexImageGenerationToolSupported
		} else {
			modelCodexImageToolSupported = model.GetModelCodexImageGenerationToolSupported(models[i].Id)
		}
		advertiseCodexImageTool := shouldAdvertiseCodexImageToolForModel(models[i].Id, modelCodexImageToolSupported, groupCodexImageToolSupported)
		models[i].SupportedSessionModes = buildSupportedSessionModes(models[i].Id, models[i].SupportedEndpointTypes, advertiseCodexImageTool)
		models[i].ActualModelReturned = buildActualModelReturned(models[i], actualModelByName[models[i].Id])
		models[i].Capabilities = buildModelCapabilities(models[i].Id, advertiseCodexImageTool)
		models[i].InputModalities = buildInputModalities(models[i].Id)
		models[i].OutputModalities = buildOutputModalities(models[i].Id, models[i].SupportedEndpointTypes, advertiseCodexImageTool)
		models[i].SupportedModalities = buildSupportedModalities(models[i].InputModalities, models[i].OutputModalities)
		models[i].ExperimentalSupportedTools = buildExperimentalSupportedTools(models[i].Id, advertiseCodexImageTool, models[i].ExperimentalSupportedTools)
	}
	return models
}

func shouldReturnCodexModels(c *gin.Context, modelType int) bool {
	if modelType != constant.ChannelTypeOpenAI {
		return false
	}
	if strings.EqualFold(c.Query("format"), "codex") {
		return true
	}
	userAgent := strings.ToLower(c.GetHeader("User-Agent"))
	return strings.Contains(userAgent, "codex")
}

func setCodexModelResponseHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
}

func logCodexModelsTrace(c *gin.Context, response dto.CodexModelsResponse, stage string) {
	if !service.ShouldLogClientRequestTrace(c) {
		return
	}
	trace := map[string]any{
		"stage":                        "codex_models_" + stage,
		"model_count":                  len(response.Models),
		"client_request":               service.BuildClientRequestTraceForLog(c),
		"models":                       summarizeCodexModelsForTrace(response.Models),
		"capabilities":                 response.Capabilities,
		"experimental_supported_tools": response.ExperimentalSupportedTools,
		"input_modalities":             response.InputModalities,
		"output_modalities":            response.OutputModalities,
		"supported_modalities":         response.SupportedModalities,
		"model_provider_capabilities":  response.ModelProviderCapabilities,
	}
	logger.LogInfo(c, "codex models trace: "+service.MarshalTraceForLog(trace))
	logger.LogInfo(c, "codex models response headers: "+service.MarshalTraceForLog(buildCodexModelResponseHeaderLog(c)))
	logger.LogInfo(c, "codex models response capability summary: "+service.MarshalTraceForLog(summarizeCodexModelsResponseForLog(response)))
}

func buildCodexModelResponseHeaderLog(c *gin.Context) map[string]any {
	if c == nil {
		return map[string]any{}
	}
	return map[string]any{
		"content_type":  "application/json; charset=utf-8",
		"cache_control": c.Writer.Header().Get("Cache-Control"),
		"pragma":        c.Writer.Header().Get("Pragma"),
	}
}

func summarizeCodexModelsResponseForLog(response dto.CodexModelsResponse) map[string]any {
	return map[string]any{
		"model_count":                  len(response.Models),
		"capabilities":                 response.Capabilities,
		"experimental_supported_tools": response.ExperimentalSupportedTools,
		"input_modalities":             response.InputModalities,
		"output_modalities":            response.OutputModalities,
		"supported_modalities":         response.SupportedModalities,
		"model_provider_capabilities":  response.ModelProviderCapabilities,
		"models":                       summarizeCodexModelsForTrace(response.Models),
	}
}

func summarizeCodexModelsForTrace(models []dto.CodexModelInfo) []map[string]any {
	items := make([]map[string]any, 0, len(models))
	for _, item := range models {
		items = append(items, map[string]any{
			"slug":                         item.Slug,
			"experimental_supported_tools": item.ExperimentalSupportedTools,
			"capabilities":                 item.Capabilities,
			"supported_session_modes":      item.SupportedSessionModes,
			"actual_model_returned":        item.ActualModelReturned,
			"input_modalities":             item.InputModalities,
			"output_modalities":            item.OutputModalities,
			"supported_endpoint_types":     item.SupportedEndpointTypes,
		})
	}
	return items
}

func buildCodexModels(models []dto.OpenAIModels) []dto.CodexModelInfo {
	codexModels := make([]dto.CodexModelInfo, 0, len(models))
	applyPatchToolType := "freeform"
	for idx, modelItem := range models {
		codexModels = append(codexModels, dto.CodexModelInfo{
			Slug:                          modelItem.Id,
			DisplayName:                   modelItem.Id,
			Description:                   nil,
			DefaultReasoningLevel:         "medium",
			SupportedReasoningLevels:      defaultCodexReasoningLevels(),
			ShellType:                     "shell_command",
			Visibility:                    "list",
			SupportedInAPI:                true,
			Priority:                      idx + 1,
			AvailabilityNux:               nil,
			Upgrade:                       nil,
			BaseInstructions:              "",
			SupportsReasoningSummaries:    true,
			DefaultReasoningSummary:       "auto",
			SupportVerbosity:              true,
			DefaultVerbosity:              "low",
			ApplyPatchToolType:            &applyPatchToolType,
			WebSearchToolType:             "text",
			SupportsSearchTool:            common.IsOpenAITextModel(modelItem.Id),
			TruncationPolicy:              dto.CodexTruncationPolicy{Mode: "bytes", Limit: 10000},
			SupportsParallelToolCalls:     true,
			SupportsImageDetailOriginal:   false,
			ContextWindow:                 272000,
			EffectiveContextWindowPercent: 95,
			ExperimentalSupportedTools:    nonNilStringSlice(modelItem.ExperimentalSupportedTools),
			InputModalities:               modelItem.InputModalities,
			OutputModalities:              modelItem.OutputModalities,
			SupportedModalities:           modelItem.SupportedModalities,
			Capabilities:                  modelItem.Capabilities,
			ActualModelReturned:           modelItem.ActualModelReturned,
			SupportedEndpointTypes:        modelItem.SupportedEndpointTypes,
			SupportedSessionModes:         modelItem.SupportedSessionModes,
		})
	}
	return codexModels
}

func buildCodexModelsResponse(c *gin.Context, codexModels []dto.CodexModelInfo) dto.CodexModelsResponse {
	response := dto.CodexModelsResponse{Models: codexModels}

	imageGenerationSupported := false
	if groups, ok := resolveVisibleModelGroups(c); ok {
		imageGenerationSupported = model.GetGroupCapabilities(groups).CodexImageGenerationToolSupported
	}
	for _, item := range codexModels {
		imageGenerationSupported = imageGenerationSupported || item.Capabilities[dto.BuildInToolImageGeneration]
	}
	if !imageGenerationSupported {
		return response
	}

	response.Capabilities = map[string]bool{dto.BuildInToolImageGeneration: true}
	response.ExperimentalSupportedTools = []string{dto.BuildInToolImageGeneration}
	response.InputModalities = []string{"text", "image"}
	response.OutputModalities = []string{"text", "image"}
	response.SupportedModalities = []string{"text", "image"}
	response.ModelProviderCapabilities = &dto.CodexModelProviderCapabilities{
		NamespaceTools:  true,
		WebSearch:       true,
		ImageGeneration: true,
	}
	return response
}

func defaultCodexReasoningLevels() []dto.CodexReasoningLevel {
	return []dto.CodexReasoningLevel{
		{Effort: "low", Description: "Fast responses with lighter reasoning"},
		{Effort: "medium", Description: "Balances speed and reasoning depth for everyday tasks"},
		{Effort: "high", Description: "Greater reasoning depth for complex problems"},
		{Effort: "xhigh", Description: "Extra high reasoning depth for complex problems"},
	}
}

func nonNilStringSlice(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

func shouldAdvertiseCodexImageToolForModel(modelName string, modelCodexImageToolSupported bool, groupCodexImageToolSupported bool) bool {
	if common.IsImageGenerationModel(modelName) {
		return modelCodexImageToolSupported
	}
	if !common.IsOpenAITextModel(modelName) {
		return modelCodexImageToolSupported
	}
	return modelCodexImageToolSupported || groupCodexImageToolSupported
}

func buildModelCapabilities(modelName string, codexImageToolSupported bool) map[string]bool {
	if !codexImageToolSupported {
		return nil
	}
	return map[string]bool{
		dto.BuildInToolImageGeneration: true,
	}
}

func buildExperimentalSupportedTools(modelName string, codexImageToolSupported bool, tools []string) []string {
	result := nonNilStringSlice(tools)
	if codexImageToolSupported {
		result = appendUniqueString(result, dto.BuildInToolImageGeneration)
	}
	return result
}

func buildInputModalities(modelName string) []string {
	if common.IsImageGenerationModel(modelName) {
		return []string{"text", "image"}
	}
	if common.IsOpenAITextModel(modelName) {
		return []string{"text", "image"}
	}
	return []string{"text"}
}

func buildOutputModalities(modelName string, endpointTypes []constant.EndpointType, codexImageToolSupported bool) []string {
	if common.IsImageGenerationModel(modelName) &&
		endpointTypesContain(endpointTypes, constant.EndpointTypeImageGeneration) {
		return []string{"image"}
	}
	if codexImageToolSupported {
		return []string{"text", "image"}
	}
	return []string{"text"}
}

func buildSupportedModalities(inputModalities []string, outputModalities []string) []string {
	modalities := make([]string, 0, len(inputModalities)+len(outputModalities))
	for _, modality := range inputModalities {
		modalities = appendUniqueString(modalities, modality)
	}
	for _, modality := range outputModalities {
		modalities = appendUniqueString(modalities, modality)
	}
	return modalities
}

func buildSupportedSessionModes(modelName string, endpointTypes []constant.EndpointType, codexImageToolSupported bool) []string {
	if common.IsImageGenerationModel(modelName) {
		modes := make([]string, 0, 2)
		for _, endpointType := range endpointTypes {
			switch endpointType {
			case constant.EndpointTypeImageGeneration:
				modes = appendUniqueString(modes, "image_generation")
			case constant.EndpointTypeImageEdit:
				modes = appendUniqueString(modes, "image_edit")
			}
		}
		if len(modes) == 0 {
			return []string{}
		}
		return modes
	}

	modes := make([]string, 0, 2)
	if endpointTypesContain(endpointTypes, constant.EndpointTypeOpenAI) {
		modes = append(modes, "chat_completions")
	}
	if endpointTypesContain(endpointTypes, constant.EndpointTypeOpenAIResponse) ||
		endpointTypesContain(endpointTypes, constant.EndpointTypeOpenAIResponseCompact) {
		modes = append(modes, "responses")
	}
	if codexImageToolSupported {
		modes = appendUniqueString(modes, "image_generation")
	}
	if len(modes) == 0 && common.IsOpenAITextModel(modelName) {
		if common.IsOpenAIResponseOnlyModel(modelName) {
			return []string{"responses"}
		}
		return []string{"chat_completions"}
	}
	return modes
}

func buildActualModelReturned(model dto.OpenAIModels, actualModel string) map[string]string {
	if common.IsImageGenerationModel(model.Id) {
		return nil
	}
	actualModel = common.GetStringIfEmpty(actualModel, model.Id)
	actualModelReturned := make(map[string]string)
	for _, mode := range model.SupportedSessionModes {
		switch mode {
		case "chat_completions", "responses", "image_generation":
			actualModelReturned[mode] = actualModel
		}
	}
	if len(actualModelReturned) == 0 {
		return nil
	}
	return actualModelReturned
}

func endpointTypesContain(endpointTypes []constant.EndpointType, target constant.EndpointType) bool {
	for _, endpointType := range endpointTypes {
		if endpointType == target {
			return true
		}
	}
	return false
}

func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func sortVisibleModels(models []dto.OpenAIModels) []dto.OpenAIModels {
	sort.Slice(models, func(i, j int) bool {
		return models[i].Id < models[j].Id
	})
	return models
}

func ChannelListModels(c *gin.Context) {
	c.JSON(200, gin.H{
		"success": true,
		"data":    openAIModels,
	})
}

func DashboardListModels(c *gin.Context) {
	c.JSON(200, gin.H{
		"success": true,
		"data":    channelId2Models,
	})
}

func EnabledListModels(c *gin.Context) {
	c.JSON(200, gin.H{
		"success": true,
		"data":    model.GetEnabledModels(),
	})
}

func RetrieveModel(c *gin.Context, modelType int) {
	modelId := c.Param("model")
	if aiModel, ok := getVisibleOpenAIModel(c, modelId, resolveAcceptUnsetRatioModel(c)); ok {
		if shouldReturnCodexModels(c, modelType) {
			codexModels := buildCodexModels([]dto.OpenAIModels{aiModel})
			codexResponse := buildCodexModelsResponse(c, codexModels)
			logCodexModelsTrace(c, codexResponse, "retrieve")
			setCodexModelResponseHeaders(c)
			c.JSON(200, codexModels[0])
			return
		}
		switch modelType {
		case constant.ChannelTypeAnthropic:
			c.JSON(200, dto.AnthropicModel{
				ID:          aiModel.Id,
				CreatedAt:   time.Unix(int64(aiModel.Created), 0).UTC().Format(time.RFC3339),
				DisplayName: aiModel.Id,
				Type:        "model",
			})
		default:
			c.JSON(200, aiModel)
		}
	} else {
		c.JSON(200, gin.H{"error": modelNotFoundError(modelId)})
	}
}

func getVisibleOpenAIModel(c *gin.Context, modelId string, acceptUnsetRatioModel bool) (dto.OpenAIModels, bool) {
	for _, item := range getVisibleOpenAIModels(c, acceptUnsetRatioModel) {
		if item.Id == modelId {
			return item, true
		}
	}
	return dto.OpenAIModels{}, false
}

func modelNotFoundError(modelId string) types.OpenAIError {
	return types.OpenAIError{
		Message: fmt.Sprintf("The model '%s' does not exist", modelId),
		Type:    "invalid_request_error",
		Param:   "model",
		Code:    "model_not_found",
	}
}
