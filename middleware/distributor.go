package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/codexauth"
	modelgatewaycredential "github.com/QuantumNous/new-api/pkg/modelgateway/credential"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type ModelRequest struct {
	Model                  string                `json:"model"`
	Group                  string                `json:"group,omitempty"`
	EndpointType           constant.EndpointType `json:"-"`
	RequiresCodexImageTool bool                  `json:"-"`
}

type selectedChannelContextOptions struct {
	EndpointType constant.EndpointType
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		distribute(c, func(c *gin.Context) {
			c.Next()
		})
	}
}

func DistributeWithNext(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		distribute(c, next)
	}
}

func distribute(c *gin.Context, next gin.HandlerFunc) {
	var channel *model.Channel
	var selection *modelgatewayintegration.SelectionResult
	channelId, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
	modelRequest, shouldSelectChannel, err := getModelRequest(c)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
		return
	}
	logDistributorRequestTrace(c, modelRequest, shouldSelectChannel)
	logCodexEffectiveModelCapabilityForRequest(c, modelRequest)
	if ok {
		id, err := strconv.Atoi(channelId.(string))
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
			return
		}
		channel, err = model.GetChannelById(id, true)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidChannelId))
			return
		}
		if channel.Status != common.ChannelStatusEnabled {
			abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorChannelDisabled))
			return
		}
	} else {
		// Select a channel for the user
		// check token model mapping
		modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
		if modelLimitEnable {
			s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
			if !ok {
				// token model limit is empty, all models are not allowed
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenNoModelAccess))
				return
			}
			var tokenModelLimit map[string]bool
			tokenModelLimit, ok = s.(map[string]bool)
			if !ok {
				tokenModelLimit = map[string]bool{}
			}
			matchName := ratio_setting.FormatMatchingModelName(modelRequest.Model) // match gpts & thinking-*
			if _, ok := tokenModelLimit[matchName]; !ok {
				abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorTokenModelForbidden, map[string]any{"Model": modelRequest.Model}))
				return
			}
		}

		if shouldSelectChannel {
			if modelRequest.Model == "" {
				abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorModelNameRequired))
				return
			}
			var selectGroup string
			usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
			// check path is /pg/chat/completions
			if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
				playgroundRequest := &dto.PlayGroundRequest{}
				err = common.UnmarshalBodyReusable(c, playgroundRequest)
				if err != nil {
					abortWithOpenAiMessage(c, http.StatusBadRequest, i18n.T(c, i18n.MsgDistributorInvalidPlayground, map[string]any{"Error": err.Error()}))
					return
				}
				if playgroundRequest.Group != "" {
					if !service.GroupInUserUsableGroups(usingGroup, playgroundRequest.Group) && playgroundRequest.Group != usingGroup {
						abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorGroupAccessDenied))
						return
					}
					usingGroup = playgroundRequest.Group
					common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
				}
			}

			var selectionErr *types.NewAPIError
			selection, selectionErr = selectSmartDistributorChannel(c, modelRequest, usingGroup)
			if selectionErr != nil {
				showGroup := usingGroup
				if usingGroup == "auto" {
					showGroup = fmt.Sprintf("auto(%s)", selectGroup)
				}
				message := i18n.T(c, i18n.MsgDistributorGetChannelFailed, map[string]any{"Group": showGroup, "Model": modelRequest.Model, "Error": selectionErr.Error()})
				abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, types.ErrorCodeModelNotFound)
				return
			}
			if selection != nil {
				channel = selection.Channel
				selectGroup = selection.Group
			}
			if setupErr := setupInitialDistributorSmartSelection(c, modelRequest, usingGroup, &channel, &selectGroup, &selection); setupErr != nil {
				abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgDistributorGetChannelFailed, map[string]any{"Group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "Model": modelRequest.Model, "Error": setupErr.Error()}), setupErr.GetErrorCode())
				return
			}

			if channel == nil {
				if preferredChannelID, found := service.GetPreferredChannelByAffinity(c, modelRequest.Model, usingGroup); found {
					preferred, err := model.CacheGetChannel(preferredChannelID)
					if err == nil && preferred != nil {
						if preferred.Status != common.ChannelStatusEnabled {
							service.MarkChannelAffinityBroken(c, "channel_disabled")
							if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
								abortWithOpenAiMessage(c, http.StatusForbidden, i18n.T(c, i18n.MsgDistributorAffinityChannelDisabled))
								return
							}
						} else if !channelSupportsModelRequest(preferred, *modelRequest) {
							service.MarkChannelAffinityBroken(c, "model_not_supported")
							logger.LogDebug(c, "Skipping affinity channel #%d because it does not support endpoint %s for model %s", preferred.Id, modelRequest.EndpointType, modelRequest.Model)
						} else if usingGroup == "auto" {
							userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
							autoGroups := service.GetUserAutoGroup(userGroup)
							for _, g := range autoGroups {
								if model.IsChannelEnabledForGroupModel(g, modelRequest.Model, preferred.Id) {
									selectGroup = g
									common.SetContextKey(c, constant.ContextKeyAutoGroup, g)
									channel = preferred
									service.MarkChannelAffinityUsed(c, g, preferred.Id)
									break
								}
							}
							if channel == nil {
								service.MarkChannelAffinityBroken(c, "group_not_supported")
							}
						} else if model.IsChannelEnabledForGroupModel(usingGroup, modelRequest.Model, preferred.Id) {
							channel = preferred
							selectGroup = usingGroup
							service.MarkChannelAffinityUsed(c, usingGroup, preferred.Id)
						} else {
							service.MarkChannelAffinityBroken(c, "group_not_supported")
						}
					} else {
						service.MarkChannelAffinityBroken(c, "channel_missing")
					}
				}
			}

			if channel == nil {
				selection, selectionErr = modelgatewayintegration.DefaultChannelSelectionWrapper().Select(c, &service.RetryParam{
					Ctx:                    c,
					ModelName:              modelRequest.Model,
					EndpointType:           modelRequest.EndpointType,
					RequiresCodexImageTool: modelRequest.RequiresCodexImageTool,
					TokenGroup:             usingGroup,
					Retry:                  common.GetPointer(0),
				})
				if selectionErr != nil {
					showGroup := usingGroup
					if usingGroup == "auto" {
						showGroup = fmt.Sprintf("auto(%s)", selectGroup)
					}
					message := i18n.T(c, i18n.MsgDistributorGetChannelFailed, map[string]any{"Group": showGroup, "Model": modelRequest.Model, "Error": selectionErr.Error()})
					// 如果错误，但是渠道不为空，说明是数据库一致性问题
					//if channel != nil {
					//	common.SysError(fmt.Sprintf("渠道不存在：%d", channel.Id))
					//	message = "数据库一致性已被破坏，请联系管理员"
					//}
					abortWithOpenAiMessage(c, http.StatusServiceUnavailable, message, types.ErrorCodeModelNotFound)
					return
				}
				if selection != nil {
					channel = selection.Channel
					selectGroup = selection.Group
					if channel != nil {
						service.MarkChannelAffinitySelection(c, service.ChannelAffinitySelectionInfo{
							SelectedGroup:     selectGroup,
							SelectedChannelID: channel.Id,
							Broken:            true,
							StickySource:      "legacy_affinity",
						})
					}
				}
				if channel == nil {
					abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgDistributorNoAvailableChannel, map[string]any{"Group": usingGroup, "Model": modelRequest.Model}), types.ErrorCodeModelNotFound)
					return
				}
			}
			applySelectedGroupContext(c, usingGroup, selectGroup)
		}
	}
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
	logDistributorSelectedChannel(c, channel, modelRequest)
	if setupErr := setupDistributorSelectedChannelIfNeeded(c, channel, modelRequest, selection); setupErr != nil {
		abortWithOpenAiMessage(c, http.StatusServiceUnavailable, i18n.T(c, i18n.MsgDistributorGetChannelFailed, map[string]any{"Group": common.GetContextKeyString(c, constant.ContextKeyUsingGroup), "Model": modelRequest.Model, "Error": setupErr.Error()}), setupErr.GetErrorCode())
		return
	}
	if next != nil {
		next(c)
	}
	if channel != nil && c.Writer != nil && c.Writer.Status() < http.StatusBadRequest {
		service.RecordChannelAffinity(c, channel.Id)
	}
}

func applySelectedGroupContext(c *gin.Context, requestedGroup, selectedGroup string) {
	if c == nil {
		return
	}
	selectedGroup = strings.TrimSpace(selectedGroup)
	if selectedGroup == "" {
		return
	}
	requestedGroup = strings.TrimSpace(requestedGroup)
	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if requestedGroup == "auto" || tokenGroup == "auto" {
		common.SetContextKey(c, constant.ContextKeyAutoGroup, selectedGroup)
	}
	common.SetContextKey(c, constant.ContextKeyUsingGroup, selectedGroup)
}

func selectSmartDistributorChannel(c *gin.Context, modelRequest *ModelRequest, usingGroup string) (*modelgatewayintegration.SelectionResult, *types.NewAPIError) {
	if modelRequest == nil {
		return nil, nil
	}
	return modelgatewayintegration.DefaultChannelSelectionWrapper().SelectSmartOnly(c, &service.RetryParam{
		Ctx:                    c,
		ModelName:              modelRequest.Model,
		EndpointType:           modelRequest.EndpointType,
		RequiresCodexImageTool: modelRequest.RequiresCodexImageTool,
		TokenGroup:             usingGroup,
		Retry:                  common.GetPointer(0),
	})
}

func setupInitialDistributorSmartSelection(c *gin.Context, modelRequest *ModelRequest, usingGroup string, channel **model.Channel, selectGroup *string, selection **modelgatewayintegration.SelectionResult) *types.NewAPIError {
	if c == nil || modelRequest == nil || channel == nil || selectGroup == nil || selection == nil {
		return nil
	}
	currentSelection := *selection
	if currentSelection == nil || !currentSelection.SmartHandled || currentSelection.Plan == nil || *channel == nil {
		return nil
	}
	var lastSetupErr *types.NewAPIError
	for attempts := 0; attempts < 4; attempts++ {
		applySelectedGroupContext(c, usingGroup, *selectGroup)
		setupErr := SetupContextForSelectedChannelWithEndpoint(c, *channel, modelRequest.Model, modelRequest.EndpointType, currentSelection)
		if setupErr == nil {
			return nil
		}
		lastSetupErr = setupErr
		if !shouldRetryDistributorSmartSetupFailure(c, setupErr) {
			return setupErr
		}
		markDistributorSmartSetupFailure(c, currentSelection)
		if attempts == 3 {
			return lastSetupErr
		}
		nextSelection, selectionErr := selectSmartDistributorChannel(c, modelRequest, usingGroup)
		if selectionErr != nil {
			return selectionErr
		}
		if nextSelection == nil || nextSelection.Channel == nil || !nextSelection.SmartHandled || nextSelection.Plan == nil {
			*channel = nil
			*selection = nil
			return nil
		}
		currentSelection = nextSelection
		*selection = nextSelection
		*channel = nextSelection.Channel
		*selectGroup = nextSelection.Group
	}
	return lastSetupErr
}

func shouldRetryDistributorSmartSetupFailure(c *gin.Context, apiErr *types.NewAPIError) bool {
	if c == nil || apiErr == nil {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	switch apiErr.GetErrorCode() {
	case types.ErrorCodeChannelNoAvailableKey, types.ErrorCodeChannelInvalidKey, types.ErrorCodeGetChannelFailed:
		return true
	default:
		return false
	}
}

func markDistributorSmartSetupFailure(c *gin.Context, selection *modelgatewayintegration.SelectionResult) {
	if c == nil || selection == nil || selection.Channel == nil {
		return
	}
	service.ReleaseChannelSelectionReservation(c, selection.Channel.Id)
	identity := modelgatewayintegration.RuntimeIdentityFromPlan(selection.Plan)
	if !identity.Valid() {
		identity = service.ChannelRuntimeIdentityFromContext(c, selection.Channel.Id)
	}
	service.MarkChannelRuntimeSelectionSkipped(c, identity)
	modelgatewayintegration.RefreshDefaultAccountCandidateIndex()
}

func setupDistributorSelectedChannelIfNeeded(c *gin.Context, channel *model.Channel, modelRequest *ModelRequest, selection *modelgatewayintegration.SelectionResult) *types.NewAPIError {
	if selection != nil && selection.SmartHandled && selection.Plan != nil {
		return nil
	}
	modelName := ""
	endpointType := constant.EndpointType("")
	if modelRequest != nil {
		modelName = modelRequest.Model
		endpointType = modelRequest.EndpointType
	}
	return SetupContextForSelectedChannelWithEndpoint(c, channel, modelName, endpointType, selection)
}

func logCodexEffectiveModelCapabilityForRequest(c *gin.Context, modelRequest *ModelRequest) {
	if c == nil || c.Request == nil || modelRequest == nil || modelRequest.Model == "" {
		return
	}
	if !service.ShouldLogClientRequestTrace(c) {
		return
	}
	if !strings.HasPrefix(c.Request.URL.Path, "/v1/responses") {
		return
	}

	groups := resolveCapabilityLogGroups(c)
	capabilitiesByModel := model.GetModelCapabilitiesForGroups(groups, []string{modelRequest.Model})
	capability := capabilitiesByModel[modelRequest.Model]
	groupCapability := model.GetGroupCapabilities(groups)
	modelCodexImageToolSupported := capability.CodexImageGenerationToolSupported
	groupCodexImageToolSupported := groupCapability.CodexImageGenerationToolSupported || capability.GroupCodexImageGenerationToolSupported
	advertiseCodexImageTool := shouldAdvertiseCodexImageToolForCapabilityLog(modelRequest.Model, modelCodexImageToolSupported, groupCodexImageToolSupported)

	inputModalities := []string{"text"}
	if common.IsOpenAITextModel(modelRequest.Model) || common.IsImageGenerationModel(modelRequest.Model) {
		inputModalities = []string{"text", "image"}
	}
	outputModalities := []string{"text"}
	if common.IsImageGenerationModel(modelRequest.Model) {
		outputModalities = []string{"image"}
	} else if advertiseCodexImageTool {
		outputModalities = []string{"text", "image"}
	}
	tools := []string{}
	capabilities := map[string]bool(nil)
	if advertiseCodexImageTool {
		tools = []string{dto.BuildInToolImageGeneration}
		capabilities = map[string]bool{dto.BuildInToolImageGeneration: true}
	}

	trace := map[string]any{
		"stage":                     "codex_effective_model_capability_for_responses_request",
		"request_model":             modelRequest.Model,
		"groups":                    groups,
		"requires_codex_image_tool": modelRequest.RequiresCodexImageTool,
		"model_codex_image_generation_tool_supported": modelCodexImageToolSupported,
		"group_codex_image_generation_tool_supported": groupCodexImageToolSupported,
		"advertise_codex_image_generation":            advertiseCodexImageTool,
		"response_headers_if_models_endpoint": map[string]string{
			"content-type":  "application/json; charset=utf-8",
			"cache-control": "no-store",
			"pragma":        "no-cache",
		},
		"model_response_summary_if_models_endpoint": map[string]any{
			"slug":                         modelRequest.Model,
			"capabilities":                 capabilities,
			"experimental_supported_tools": tools,
			"input_modalities":             inputModalities,
			"output_modalities":            outputModalities,
			"supported_endpoint_types":     capability.SupportedEndpointTypes,
		},
		"top_level_response_summary_if_models_endpoint": map[string]any{
			"capabilities":                 capabilities,
			"experimental_supported_tools": tools,
			"input_modalities":             inputModalities,
			"output_modalities":            outputModalities,
			"model_provider_capabilities": map[string]bool{
				"namespaceTools":  advertiseCodexImageTool,
				"webSearch":       true,
				"imageGeneration": advertiseCodexImageTool,
			},
		},
	}
	logger.LogInfo(c, "codex effective model capability response data: "+service.MarshalTraceForLog(trace))
}

func resolveCapabilityLogGroups(c *gin.Context) []string {
	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if tokenGroup != "" && tokenGroup != "auto" {
		return []string{tokenGroup}
	}
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if usingGroup != "" && usingGroup != "auto" {
		return []string{usingGroup}
	}
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if userGroup == "" {
		userGroup = c.GetString("group")
	}
	if userGroup == "" {
		return nil
	}
	return service.GetUserAutoGroup(userGroup)
}

func shouldAdvertiseCodexImageToolForCapabilityLog(modelName string, modelCodexImageToolSupported bool, groupCodexImageToolSupported bool) bool {
	if common.IsImageGenerationModel(modelName) {
		return modelCodexImageToolSupported
	}
	if !common.IsOpenAITextModel(modelName) {
		return modelCodexImageToolSupported
	}
	return modelCodexImageToolSupported || groupCodexImageToolSupported
}

func logDistributorRequestTrace(c *gin.Context, modelRequest *ModelRequest, shouldSelectChannel bool) {
	if !service.ShouldLogClientRequestTrace(c) {
		return
	}
	trace := map[string]any{
		"stage":                 "distributor",
		"should_select_channel": shouldSelectChannel,
	}
	if modelRequest != nil {
		trace["model"] = modelRequest.Model
		trace["group"] = modelRequest.Group
		trace["endpoint_type"] = string(modelRequest.EndpointType)
		trace["requires_codex_image_tool"] = modelRequest.RequiresCodexImageTool
	}
	trace["client_request"] = service.BuildClientRequestTraceForLog(c)
	if toolTrace := service.BuildResponsesRequestToolTraceFromContextForLog(c); len(toolTrace) > 0 {
		trace["responses_tools"] = toolTrace
	}
	logger.LogInfo(c, "client request trace: "+service.MarshalTraceForLog(trace))
}

func logDistributorSelectedChannel(c *gin.Context, channel *model.Channel, modelRequest *ModelRequest) {
	if !service.ShouldLogClientRequestTrace(c) {
		return
	}
	trace := map[string]any{
		"stage": "distributor_selected_channel",
	}
	if modelRequest != nil {
		trace["model"] = modelRequest.Model
		trace["endpoint_type"] = string(modelRequest.EndpointType)
		trace["requires_codex_image_tool"] = modelRequest.RequiresCodexImageTool
	}
	if channel == nil {
		trace["channel"] = nil
	} else {
		trace["channel"] = map[string]any{
			"id":                               channel.Id,
			"name":                             channel.Name,
			"type":                             channel.Type,
			"supports_endpoint":                modelRequest == nil || service.ChannelSupportsRequiredEndpoint(channel, modelRequest.Model, modelRequest.EndpointType),
			"supports_codex_image_generation":  service.ChannelSupportsCodexImageGenerationTool(channel),
			"supports_required_capabilities":   modelRequest == nil || service.ChannelSupportsRequiredCapabilities(channel, modelRequest.Model, modelRequest.EndpointType, modelRequest.RequiresCodexImageTool),
			"codex_compatibility_mode":         channel.GetOtherSettings().CodexCompatibilityMode,
			"codex_supported_tools":            channel.GetOtherSettings().CodexSupportedTools,
			"codex_image_tool_probe_supported": channel.GetOtherSettings().CodexImageGenerationToolSupported,
		}
	}
	logger.LogInfo(c, "channel selection trace: "+service.MarshalTraceForLog(trace))
}

// getModelFromRequest 从请求中读取模型信息
// 根据 Content-Type 自动处理：
// - application/json
// - application/x-www-form-urlencoded
// - multipart/form-data
func getModelFromRequest(c *gin.Context) (*ModelRequest, error) {
	var modelRequest ModelRequest
	err := common.UnmarshalBodyReusable(c, &modelRequest)
	if err != nil {
		return nil, errors.New(i18n.T(c, i18n.MsgDistributorInvalidRequest, map[string]any{"Error": err.Error()}))
	}
	return &modelRequest, nil
}

func detectRequiredEndpointType(c *gin.Context) constant.EndpointType {
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/v1/images/generations") {
		return constant.EndpointTypeImageGeneration
	}
	if strings.HasPrefix(path, "/v1/images/edits") {
		return constant.EndpointTypeImageEdit
	}
	if strings.HasPrefix(path, "/v1/responses/compact") {
		return constant.EndpointTypeOpenAIResponseCompact
	}
	if strings.HasPrefix(path, "/v1/responses") {
		return constant.EndpointTypeOpenAIResponse
	}
	return ""
}

func responsesRequestHasImageGenerationTool(c *gin.Context) bool {
	return false
}

func channelSupportsModelRequest(channel *model.Channel, request ModelRequest) bool {
	if !service.ChannelSupportsRequiredEndpoint(channel, request.Model, request.EndpointType) {
		return false
	}
	return true
}

func getModelRequest(c *gin.Context) (*ModelRequest, bool, error) {
	var modelRequest ModelRequest
	shouldSelectChannel := true
	var err error
	if strings.Contains(c.Request.URL.Path, "/mj/") {
		relayMode := relayconstant.Path2RelayModeMidjourney(c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeMidjourneyTaskFetch ||
			relayMode == relayconstant.RelayModeMidjourneyTaskFetchByCondition ||
			relayMode == relayconstant.RelayModeMidjourneyNotify ||
			relayMode == relayconstant.RelayModeMidjourneyTaskImageSeed {
			shouldSelectChannel = false
		} else {
			midjourneyRequest := dto.MidjourneyRequest{}
			err = common.UnmarshalBodyReusable(c, &midjourneyRequest)
			if err != nil {
				return nil, false, errors.New(i18n.T(c, i18n.MsgDistributorInvalidMidjourney, map[string]any{"Error": err.Error()}))
			}
			midjourneyModel, mjErr, success := service.GetMjRequestModel(relayMode, &midjourneyRequest)
			if mjErr != nil {
				return nil, false, fmt.Errorf("%s", mjErr.Description)
			}
			if midjourneyModel == "" {
				if !success {
					return nil, false, fmt.Errorf("%s", i18n.T(c, i18n.MsgDistributorInvalidParseModel))
				} else {
					// task fetch, task fetch by condition, notify
					shouldSelectChannel = false
				}
			}
			modelRequest.Model = midjourneyModel
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/suno/") {
		relayMode := relayconstant.Path2RelaySuno(c.Request.Method, c.Request.URL.Path)
		if relayMode == relayconstant.RelayModeSunoFetch ||
			relayMode == relayconstant.RelayModeSunoFetchByID {
			shouldSelectChannel = false
		} else {
			modelName := service.CoverTaskActionToModelName(constant.TaskPlatformSuno, c.Param("action"))
			modelRequest.Model = modelName
		}
		c.Set("platform", string(constant.TaskPlatformSuno))
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos/") && strings.HasSuffix(c.Request.URL.Path, "/remix") {
		relayMode := relayconstant.RelayModeVideoSubmit
		c.Set("relay_mode", relayMode)
		shouldSelectChannel = false
	} else if strings.Contains(c.Request.URL.Path, "/v1/videos") {
		//curl https://api.openai.com/v1/videos \
		//  -H "Authorization: Bearer $OPENAI_API_KEY" \
		//  -F "model=sora-2" \
		//  -F "prompt=A calico cat playing a piano on stage"
		//	-F input_reference="@image.jpg"
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			relayMode = relayconstant.RelayModeVideoSubmit
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			if req != nil {
				modelRequest.Model = req.Model
			}
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		c.Set("relay_mode", relayMode)
	} else if strings.Contains(c.Request.URL.Path, "/v1/video/generations") {
		relayMode := relayconstant.RelayModeUnknown
		if c.Request.Method == http.MethodPost {
			req, err := getModelFromRequest(c)
			if err != nil {
				return nil, false, err
			}
			modelRequest.Model = req.Model
			relayMode = relayconstant.RelayModeVideoSubmit
		} else if c.Request.Method == http.MethodGet {
			relayMode = relayconstant.RelayModeVideoFetchByID
			shouldSelectChannel = false
		}
		if _, ok := c.Get("relay_mode"); !ok {
			c.Set("relay_mode", relayMode)
		}
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1beta/models/") || strings.HasPrefix(c.Request.URL.Path, "/v1/models/") {
		// Gemini API 路径处理: /v1beta/models/gemini-2.0-flash:generateContent
		relayMode := relayconstant.RelayModeGemini
		modelName := extractModelNameFromGeminiPath(c.Request.URL.Path)
		if modelName != "" {
			modelRequest.Model = modelName
		}
		c.Set("relay_mode", relayMode)
	} else if !strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") && !strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/realtime") {
		//wss://api.openai.com/v1/realtime?model=gpt-4o-realtime-preview-2024-10-01
		modelRequest.Model = c.Query("model")
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/moderations") {
		if modelRequest.Model == "" {
			modelRequest.Model = "text-moderation-stable"
		}
	}
	if strings.HasSuffix(c.Request.URL.Path, "embeddings") {
		if modelRequest.Model == "" {
			modelRequest.Model = c.Param("model")
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/images/generations") {
		modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "dall-e")
	} else if strings.HasPrefix(c.Request.URL.Path, "/v1/images/edits") {
		//modelRequest.Model = common.GetStringIfEmpty(c.PostForm("model"), "gpt-image-1")
		contentType := c.ContentType()
		if slices.Contains([]string{gin.MIMEPOSTForm, gin.MIMEMultipartPOSTForm}, contentType) {
			req, err := getModelFromRequest(c)
			if err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
		}
	}
	if strings.HasPrefix(c.Request.URL.Path, "/v1/audio") {
		relayMode := relayconstant.RelayModeAudioSpeech
		if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/speech") {

			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "tts-1")
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/translations") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranslation
		} else if strings.HasPrefix(c.Request.URL.Path, "/v1/audio/transcriptions") {
			// 先尝试从请求读取
			if req, err := getModelFromRequest(c); err == nil && req.Model != "" {
				modelRequest.Model = req.Model
			}
			modelRequest.Model = common.GetStringIfEmpty(modelRequest.Model, "whisper-1")
			relayMode = relayconstant.RelayModeAudioTranscription
		}
		c.Set("relay_mode", relayMode)
	}
	if strings.HasPrefix(c.Request.URL.Path, "/pg/chat/completions") {
		// playground chat completions
		req, err := getModelFromRequest(c)
		if err != nil {
			return nil, false, err
		}
		modelRequest.Model = req.Model
		modelRequest.Group = req.Group
		common.SetContextKey(c, constant.ContextKeyTokenGroup, modelRequest.Group)
	}

	if strings.HasPrefix(c.Request.URL.Path, "/v1/responses/compact") && modelRequest.Model != "" {
		modelRequest.Model = ratio_setting.WithCompactModelSuffix(modelRequest.Model)
	}
	modelRequest.EndpointType = detectRequiredEndpointType(c)
	modelRequest.RequiresCodexImageTool = responsesRequestHasImageGenerationTool(c)
	return &modelRequest, shouldSelectChannel, nil
}

func SetupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string, selections ...*modelgatewayintegration.SelectionResult) *types.NewAPIError {
	return setupContextForSelectedChannel(c, channel, modelName, selectedChannelContextOptions{}, selections...)
}

func SetupContextForSelectedChannelWithEndpoint(c *gin.Context, channel *model.Channel, modelName string, endpointType constant.EndpointType, selections ...*modelgatewayintegration.SelectionResult) *types.NewAPIError {
	return setupContextForSelectedChannel(c, channel, modelName, selectedChannelContextOptions{EndpointType: endpointType}, selections...)
}

func setupContextForSelectedChannel(c *gin.Context, channel *model.Channel, modelName string, options selectedChannelContextOptions, selections ...*modelgatewayintegration.SelectionResult) *types.NewAPIError {
	c.Set("original_model", modelName) // for retry
	if channel == nil {
		return types.NewError(errors.New("channel is nil"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyChannelName, channel.Name)
	common.SetContextKey(c, constant.ContextKeyChannelType, channel.Type)
	common.SetContextKey(c, constant.ContextKeyChannelCreateTime, channel.CreatedTime)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, channel.GetSetting())
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, channel.GetOtherSettings())
	paramOverride := channel.GetParamOverride()
	headerOverride := channel.GetHeaderOverride()
	if mergedParam, applied := service.ApplyChannelAffinityOverrideTemplate(c, paramOverride); applied {
		paramOverride = mergedParam
	}
	common.SetContextKey(c, constant.ContextKeyChannelParamOverride, paramOverride)
	common.SetContextKey(c, constant.ContextKeyChannelHeaderOverride, headerOverride)
	if nil != channel.OpenAIOrganization && *channel.OpenAIOrganization != "" {
		common.SetContextKey(c, constant.ContextKeyChannelOrganization, *channel.OpenAIOrganization)
	}
	common.SetContextKey(c, constant.ContextKeyChannelAutoBan, channel.GetAutoBan())
	common.SetContextKey(c, constant.ContextKeyChannelModelMapping, channel.GetModelMapping())
	common.SetContextKey(c, constant.ContextKeyChannelStatusCodeMapping, channel.GetStatusCodeMapping())

	if applied, apiErr := applySelectedPlanCredential(c, channel, firstSelection(selections)); apiErr != nil {
		return apiErr
	} else if !applied {
		key, index, newAPIError := channel.GetNextEnabledKeyForEndpoint(options.EndpointType)
		if newAPIError != nil {
			return newAPIError
		}
		if channel.ChannelInfo.IsMultiKey {
			common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
			common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, index)
		} else {
			// 必须设置为 false，否则在重试到单个 key 的时候会导致日志显示错误
			common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, false)
		}
		// c.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", key))
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		modelgatewaycredential.ApplyChannelCredentialIdentityToContext(c, channel, index, key)
		applySelectedAccountCapability(c, channel, index)
		applyLegacySelectedKeyProxy(c, channel, index)
	} else {
		common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, channel.ChannelInfo.IsMultiKey)
		applySelectedAccountCapability(c, channel, common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex))
	}
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, channel.GetBaseURL())
	applyEffectiveCodexOAuthRelayContext(c, channel, options.EndpointType)

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, false)

	// TODO: api_version统一
	switch channel.Type {
	case constant.ChannelTypeAzure:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeVertexAi:
		c.Set("region", channel.Other)
	case constant.ChannelTypeXunfei:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeGemini:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeAli:
		c.Set("plugin", channel.Other)
	case constant.ChannelCloudflare:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeMokaAI:
		c.Set("api_version", channel.Other)
	case constant.ChannelTypeCoze:
		c.Set("bot_id", channel.Other)
	}
	return nil
}

func firstSelection(selections []*modelgatewayintegration.SelectionResult) *modelgatewayintegration.SelectionResult {
	if len(selections) == 0 {
		return nil
	}
	return selections[0]
}

func applySelectedPlanCredential(c *gin.Context, channel *model.Channel, selection *modelgatewayintegration.SelectionResult) (bool, *types.NewAPIError) {
	if c == nil || channel == nil || selection == nil || !selection.SmartHandled || selection.Plan == nil {
		return false, nil
	}
	ref := selection.Plan.CredentialRef
	if ref.ResourceID == "" &&
		ref.AccountID == "" &&
		ref.CredentialSubjectFingerprint == "" &&
		ref.CredentialFingerprint == "" &&
		ref.Resolver == "" &&
		ref.CredentialIndex == 0 {
		return false, nil
	}
	resolved, apiErr := modelgatewaycredential.ResolveChannelCredential(channel, ref)
	if apiErr != nil {
		return false, apiErr
	}
	modelgatewaycredential.ApplyResolvedCredentialToContext(c, resolved)
	return true, nil
}

func applySelectedAccountCapability(c *gin.Context, channel *model.Channel, credentialIndex int) {
	if c == nil || channel == nil || credentialIndex < 0 {
		return
	}
	if capability, ok := channel.ChannelInfo.AccountCapability(credentialIndex); ok {
		common.SetContextKey(c, constant.ContextKeyChannelAccountCapability, capability)
	}
}

func applyEffectiveCodexOAuthRelayContext(c *gin.Context, channel *model.Channel, endpointType constant.EndpointType) {
	if c == nil || channel == nil || channel.Type != constant.ChannelTypeOpenAI {
		return
	}
	switch endpointType {
	case constant.EndpointTypeOpenAIResponse, constant.EndpointTypeOpenAIResponseCompact:
	default:
		return
	}
	if _, ok := codexauth.ParseOAuthJSONCredential(common.GetContextKeyString(c, constant.ContextKeyChannelKey)); !ok {
		return
	}
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeCodex)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, constant.ChannelBaseURLs[constant.ChannelTypeCodex])
	common.SetContextKey(c, constant.ContextKeyProviderSurface, "codex_backend")
	if capability, ok := common.GetContextKeyType[model.ChannelAccountCapability](c, constant.ContextKeyChannelAccountCapability); ok {
		if classification := capability.EffectiveClassification(); classification != "" {
			common.SetContextKey(c, constant.ContextKeyCapabilityClassification, classification)
		}
	}
}

func applyLegacySelectedKeyProxy(c *gin.Context, channel *model.Channel, credentialIndex int) {
	if c == nil || channel == nil || credentialIndex < 0 || channel.ChannelInfo.MultiKeyProxyIDs == nil {
		return
	}
	proxyID := channel.ChannelInfo.MultiKeyProxyIDs[credentialIndex]
	if proxyID <= 0 {
		return
	}
	proxyConfig, err := model.GetModelGatewayProxyByID(proxyID)
	if err != nil || proxyConfig == nil || !proxyConfig.Enabled {
		return
	}
	proxyURL, err := proxyConfig.ProxyURL()
	if err != nil || strings.TrimSpace(proxyURL) == "" {
		return
	}
	common.SetContextKey(c, constant.ContextKeyChannelAccountProxyID, proxyID)
	common.SetContextKey(c, constant.ContextKeyChannelAccountProxyURL, proxyURL)
}

// extractModelNameFromGeminiPath 从 Gemini API URL 路径中提取模型名
// 输入格式: /v1beta/models/gemini-2.0-flash:generateContent
// 输出: gemini-2.0-flash
func extractModelNameFromGeminiPath(path string) string {
	// 查找 "/models/" 的位置
	modelsPrefix := "/models/"
	modelsIndex := strings.Index(path, modelsPrefix)
	if modelsIndex == -1 {
		return ""
	}

	// 从 "/models/" 之后开始提取
	startIndex := modelsIndex + len(modelsPrefix)
	if startIndex >= len(path) {
		return ""
	}

	// 查找 ":" 的位置，模型名在 ":" 之前
	colonIndex := strings.Index(path[startIndex:], ":")
	if colonIndex == -1 {
		// 如果没有找到 ":"，返回从 "/models/" 到路径结尾的部分
		return path[startIndex:]
	}

	// 返回模型名部分
	return path[startIndex : startIndex+colonIndex]
}
