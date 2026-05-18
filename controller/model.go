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
	acceptUnsetRatioModel := operation_setting.SelfUseModeEnabled
	if !acceptUnsetRatioModel {
		userId := c.GetInt("id")
		if userId > 0 {
			userSettings, _ := model.GetUserSetting(userId, false)
			if userSettings.AcceptUnsetRatioModel {
				acceptUnsetRatioModel = true
			}
		}
	}

	userOpenAiModels := getVisibleOpenAIModels(c, acceptUnsetRatioModel)
	if shouldReturnCodexModels(c, modelType) {
		c.JSON(200, dto.CodexModelsResponse{
			Models: buildCodexModels(userOpenAiModels),
		})
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
		tokenGroup = "auto"
	}
	if len(groups) == 1 && tokenGroup != "auto" {
		return model.GetGroupAvailableModels(groups[0]), true
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
		tokenGroup = "auto"
	}
	switch tokenGroup {
	case "auto":
		return service.GetUserAutoGroup(userGroup), true
	default:
		return []string{tokenGroup}, true
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
	if groups, ok := resolveVisibleModelGroups(c); ok {
		actualModelByName = model.GetPreferredActualModelsForGroups(groups, modelNames)
	}

	for i := range models {
		models[i].SupportedSessionModes = buildSupportedSessionModes(models[i].Id, models[i].SupportedEndpointTypes)
		models[i].ActualModelReturned = buildActualModelReturned(models[i], actualModelByName[models[i].Id])
		models[i].InputModalities = buildInputModalities(models[i].Id)
		models[i].ExperimentalSupportedTools = buildExperimentalSupportedTools(models[i].Id, models[i].SupportedSessionModes, models)
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

func buildCodexModels(models []dto.OpenAIModels) []dto.CodexModelInfo {
	codexModels := make([]dto.CodexModelInfo, 0, len(models))
	applyPatchToolType := "freeform"
	for idx, modelItem := range models {
		if common.IsImageGenerationModel(modelItem.Id) {
			continue
		}
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
			TruncationPolicy:              dto.CodexTruncationPolicy{Mode: "bytes", Limit: 10000},
			SupportsParallelToolCalls:     true,
			SupportsImageDetailOriginal:   false,
			ContextWindow:                 272000,
			EffectiveContextWindowPercent: 95,
			ExperimentalSupportedTools:    modelItem.ExperimentalSupportedTools,
			InputModalities:               modelItem.InputModalities,
			ActualModelReturned:           modelItem.ActualModelReturned,
			SupportedEndpointTypes:        modelItem.SupportedEndpointTypes,
			SupportedSessionModes:         modelItem.SupportedSessionModes,
		})
	}
	return codexModels
}

func defaultCodexReasoningLevels() []dto.CodexReasoningLevel {
	return []dto.CodexReasoningLevel{
		{Effort: "low", Description: "Fast responses with lighter reasoning"},
		{Effort: "medium", Description: "Balances speed and reasoning depth for everyday tasks"},
		{Effort: "high", Description: "Greater reasoning depth for complex problems"},
		{Effort: "xhigh", Description: "Extra high reasoning depth for complex problems"},
	}
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

func buildExperimentalSupportedTools(modelName string, sessionModes []string, models []dto.OpenAIModels) []string {
	if common.IsImageGenerationModel(modelName) || !containsString(sessionModes, "responses") {
		return nil
	}
	if visibleModelsSupportImageGeneration(models) {
		return []string{dto.BuildInToolImageGeneration}
	}
	return nil
}

func visibleModelsSupportImageGeneration(models []dto.OpenAIModels) bool {
	for _, modelItem := range models {
		if common.IsImageGenerationModel(modelItem.Id) {
			return true
		}
		if endpointTypesContain(modelItem.SupportedEndpointTypes, constant.EndpointTypeImageGeneration) {
			return true
		}
	}
	return false
}

func buildSupportedSessionModes(modelName string, endpointTypes []constant.EndpointType) []string {
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
			modes = append(modes, "image_generation", "image_edit")
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
		case "chat_completions", "responses":
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

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
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
	if aiModel, ok := openAIModelsMap[modelId]; ok {
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
		openAIError := types.OpenAIError{
			Message: fmt.Sprintf("The model '%s' does not exist", modelId),
			Type:    "invalid_request_error",
			Param:   "model",
			Code:    "model_not_found",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
	}
}
