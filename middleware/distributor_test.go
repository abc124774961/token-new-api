package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	"github.com/QuantumNous/new-api/pkg/modelgateway/policy"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newResponsesCapabilityTestContext(body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(body)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx
}

func setupDistributorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func withDistributorMemoryCache(t *testing.T, enabled bool) {
	t.Helper()
	original := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = enabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = original
	})
}

func TestResponsesRequestHasImageGenerationToolDetectsDeclaredTools(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tools": [{"type": "image_generation"}]
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolDetectsToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": {"type": "image_generation"}
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolDetectsStringToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": "image_generation"
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolDetectsNestedToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": {
			"type": "allowed_tools",
			"tools": [{"type": "image_generation"}]
		}
	}`)

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresCodexSkillIntent(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"input": [
			{
				"role": "user",
				"content": [
					{"type": "input_text", "text": "[$imagegen](/Users/frode.luo/.codex/skills/.system/imagegen/SKILL.md) 风景"}
				]
			}
		],
		"tool_choice": "auto"
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresOtherTools(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tools": [{"type": "web_search_preview"}],
		"tool_choice": "auto"
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasPreviousResponseIDDetectsField(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"previous_response_id": "resp_123"
	}`)

	require.True(t, responsesRequestHasPreviousResponseID(ctx))
}

func TestResponsesRequestHasPreviousResponseIDIgnoresBlankField(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"previous_response_id": " "
	}`)

	require.False(t, responsesRequestHasPreviousResponseID(ctx))
}

func TestRelaxUnsupportedCodexImageToolRequirementStripsToolWhenNoImageToolCandidate(t *testing.T) {
	db := setupDistributorTestDB(t)
	withDistributorMemoryCache(t, true)

	plainChannel := &model.Channel{
		Id:            301,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "plain-codex-plus",
		Key:           "sk-plain",
		Status:        common.ChannelStatusEnabled,
		Group:         "codex-plus",
		Models:        "gpt-5.4",
		OtherSettings: `{"codex_compatibility_mode":true}`,
	}
	require.NoError(t, db.Create(plainChannel).Error)
	require.NoError(t, plainChannel.AddAbilities(nil))
	model.InitChannelCache()

	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.4",
		"input": "hello",
		"tools": [{"type": "image_generation"}, {"type": "web_search_preview"}],
		"tool_choice": {"type": "allowed_tools", "tools": [{"type": "image_generation"}, {"type": "web_search_preview"}]}
	}`)
	modelRequest := &ModelRequest{
		Model:                  "gpt-5.4",
		EndpointType:           constant.EndpointTypeOpenAIResponse,
		RequiresCodexImageTool: true,
	}

	relaxUnsupportedCodexImageToolRequirementIfNeeded(ctx, modelRequest, "codex-plus-vip3")

	require.False(t, modelRequest.RequiresCodexImageTool)
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyRequiresCodexImageTool))

	var req dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalBodyReusable(ctx, &req))
	require.False(t, req.HasTool(dto.BuildInToolImageGeneration))
	require.True(t, req.HasTool("web_search_preview"))
}

func TestRelaxUnsupportedResponsesPreviousIDRequirementStripsFieldWhenNoCandidate(t *testing.T) {
	db := setupDistributorTestDB(t)
	withDistributorMemoryCache(t, true)

	plainChannel := &model.Channel{
		Id:            304,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "plain-responses",
		Key:           "sk-plain",
		Status:        common.ChannelStatusEnabled,
		Group:         "codex-plus",
		Models:        "gpt-5.4",
		OtherSettings: `{"wire_api":"responses"}`,
	}
	require.NoError(t, db.Create(plainChannel).Error)
	require.NoError(t, plainChannel.AddAbilities(nil))
	model.InitChannelCache()

	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.4",
		"input": "hello",
		"previous_response_id": "resp_stale"
	}`)
	common.SetContextKey(ctx, constant.ContextKeyResponsesPreviousID, true)
	modelRequest := &ModelRequest{
		Model:                       "gpt-5.4",
		EndpointType:                constant.EndpointTypeOpenAIResponse,
		RequiresResponsesPreviousID: true,
	}

	relaxUnsupportedResponsesPreviousIDRequirementIfNeeded(ctx, modelRequest, "codex-plus")

	require.False(t, modelRequest.RequiresResponsesPreviousID)
	require.False(t, common.GetContextKeyBool(ctx, constant.ContextKeyResponsesPreviousID))

	var req dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalBodyReusable(ctx, &req))
	require.Empty(t, req.PreviousResponseID)
}

func TestRelaxUnsupportedResponsesPreviousIDRequirementKeepsFieldWhenCandidateExists(t *testing.T) {
	db := setupDistributorTestDB(t)
	withDistributorMemoryCache(t, true)

	supportedChannel := &model.Channel{
		Id:            305,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "previous-id-responses",
		Key:           "sk-supported",
		Status:        common.ChannelStatusEnabled,
		Group:         "codex-plus",
		Models:        "gpt-5.4",
		OtherSettings: `{"wire_api":"responses","support_responses_previous_id":true}`,
	}
	require.NoError(t, db.Create(supportedChannel).Error)
	require.NoError(t, supportedChannel.AddAbilities(nil))
	model.InitChannelCache()

	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.4",
		"input": "hello",
		"previous_response_id": "resp_keep"
	}`)
	common.SetContextKey(ctx, constant.ContextKeyResponsesPreviousID, true)
	modelRequest := &ModelRequest{
		Model:                       "gpt-5.4",
		EndpointType:                constant.EndpointTypeOpenAIResponse,
		RequiresResponsesPreviousID: true,
	}

	relaxUnsupportedResponsesPreviousIDRequirementIfNeeded(ctx, modelRequest, "codex-plus")

	require.True(t, modelRequest.RequiresResponsesPreviousID)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyResponsesPreviousID))

	var req dto.OpenAIResponsesRequest
	require.NoError(t, common.UnmarshalBodyReusable(ctx, &req))
	require.Equal(t, "resp_keep", req.PreviousResponseID)
}

func TestRelaxUnsupportedCodexImageToolRequirementKeepsToolWhenImageToolCandidateExists(t *testing.T) {
	db := setupDistributorTestDB(t)
	withDistributorMemoryCache(t, true)

	imageChannel := &model.Channel{
		Id:            302,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "image-codex-plus",
		Key:           "sk-image",
		Status:        common.ChannelStatusEnabled,
		Group:         "codex-plus",
		Models:        "gpt-5.4",
		OtherSettings: `{"codex_compatibility_mode":true,"codex_image_generation_tool_supported":true}`,
	}
	require.NoError(t, db.Create(imageChannel).Error)
	require.NoError(t, imageChannel.AddAbilities(nil))
	model.InitChannelCache()

	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.4",
		"input": "hello",
		"tools": [{"type": "image_generation"}]
	}`)
	modelRequest := &ModelRequest{
		Model:                  "gpt-5.4",
		EndpointType:           constant.EndpointTypeOpenAIResponse,
		RequiresCodexImageTool: true,
	}

	relaxUnsupportedCodexImageToolRequirementIfNeeded(ctx, modelRequest, "codex-plus-vip3")

	require.True(t, modelRequest.RequiresCodexImageTool)
	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestRelaxUnsupportedCodexImageToolRequirementKeepsForcedImageToolChoice(t *testing.T) {
	db := setupDistributorTestDB(t)
	withDistributorMemoryCache(t, true)

	plainChannel := &model.Channel{
		Id:            303,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "plain-codex-plus",
		Key:           "sk-plain",
		Status:        common.ChannelStatusEnabled,
		Group:         "codex-plus",
		Models:        "gpt-5.4",
		OtherSettings: `{"codex_compatibility_mode":true}`,
	}
	require.NoError(t, db.Create(plainChannel).Error)
	require.NoError(t, plainChannel.AddAbilities(nil))
	model.InitChannelCache()

	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.4",
		"input": "hello",
		"tools": [{"type": "image_generation"}],
		"tool_choice": {"type": "image_generation"}
	}`)
	modelRequest := &ModelRequest{
		Model:                  "gpt-5.4",
		EndpointType:           constant.EndpointTypeOpenAIResponse,
		RequiresCodexImageTool: true,
	}

	relaxUnsupportedCodexImageToolRequirementIfNeeded(ctx, modelRequest, "codex-plus-vip3")

	require.True(t, modelRequest.RequiresCodexImageTool)
	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresPlainImagePromptWithoutKeywordHit(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"input": [
			{
				"role": "user",
				"content": [
					{"type": "input_text", "text": "生成一张风景图"}
				]
			}
		],
		"tool_choice": "auto"
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestApplySelectedGroupContextUsesSelectedGroupForAutoRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "auto")

	applySelectedGroupContext(ctx, "auto", "codex-plus")

	require.Equal(t, "codex-plus", common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))
	require.Equal(t, "codex-plus", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestApplySelectedGroupContextKeepsTokenGroupForFallbackRouting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "codex-plus-特惠")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "codex-plus-特惠")

	applySelectedGroupContext(ctx, "codex-plus-特惠", "codex-plus")

	require.Equal(t, "codex-plus-特惠", common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))
	require.Empty(t, common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestApplySelectedGroupContextUsesSelectedGroupWithoutTokenGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	applySelectedGroupContext(ctx, "codex-plus-特惠", "codex-plus")

	require.Equal(t, "codex-plus", common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))
}

func TestSetupContextForSelectedChannelAppliesSmartCredentialWithoutPolling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.CryptoSecret = "test-secret"
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := &model.Channel{
		Id:     42,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b\nsk-c",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeyPollingIndex: 2,
		},
	}
	selection := &modelgatewayintegration.SelectionResult{
		Channel:      channel,
		Group:        "default",
		SmartHandled: true,
		Plan: &core.DispatchPlan{
			Channel:       channel,
			SelectedGroup: "default",
			CredentialRef: core.CredentialRef{
				ResourceID:            "platform:channel:42",
				CredentialIndex:       1,
				CredentialFingerprint: common.GenerateHMAC("sk-b"),
				Resolver:              "channel_key",
			},
		},
	}

	apiErr := SetupContextForSelectedChannel(ctx, channel, "gpt-5.4", selection)

	require.Nil(t, apiErr)
	require.Equal(t, "sk-b", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey))
	require.Equal(t, 1, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	require.Equal(t, 2, channel.ChannelInfo.MultiKeyPollingIndex)
}

func TestSetupContextForSelectedChannelAppliesSmartCredentialWithSubjectOnlyRef(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.CryptoSecret = "test-secret"
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := &model.Channel{
		Id:     43,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeyPollingIndex: 0,
		},
	}
	selection := &modelgatewayintegration.SelectionResult{
		Channel:      channel,
		Group:        "default",
		SmartHandled: true,
		Plan: &core.DispatchPlan{
			Channel:       channel,
			SelectedGroup: "default",
			CredentialRef: core.CredentialRef{
				CredentialIndex:              1,
				CredentialSubjectFingerprint: common.GenerateHMAC("subject-b"),
				Resolver:                     "channel_key",
			},
		},
	}

	apiErr := SetupContextForSelectedChannel(ctx, channel, "gpt-5.4", selection)

	require.Nil(t, apiErr)
	require.Equal(t, "sk-b", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey))
	require.Equal(t, 1, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	require.Equal(t, 0, channel.ChannelInfo.MultiKeyPollingIndex)
}

func TestDistributeRetriesSmartSetupFailureBeforeRelay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.CryptoSecret = "test-secret"
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	channel := &model.Channel{
		Id:     52,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-disabled\nsk-enabled",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.5",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:         true,
			MultiKeySize:       2,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled},
		},
	}
	keyA := core.RuntimeKey{ChannelID: channel.Id, RequestedModel: "gpt-5.5", Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	keyB := keyA
	keyA.CredentialIndex = 0
	keyA.CredentialFP = common.GenerateHMAC("sk-disabled")
	keyB.CredentialIndex = 1
	keyB.CredentialFP = common.GenerateHMAC("sk-enabled")

	snapshots := scheduler.NewMemoryRuntimeSnapshotStore()
	snapshots.Put(core.RuntimeSnapshot{
		Key:                keyA,
		SuccessRate:        0.99,
		TTFTMs:             120,
		TokensPerSecond:    80,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	snapshots.Put(core.RuntimeSnapshot{
		Key:                keyB,
		SuccessRate:        0.95,
		TTFTMs:             180,
		TokensPerSecond:    60,
		CostRatio:          1,
		GroupPriorityRatio: 1,
		SampleCount:        20,
	})
	selector := scheduler.NewDefaultSmartChannelSelector(
		scheduler.NewStaticCandidatePoolBuilder([]core.Candidate{
			{
				Channel:    channel,
				Group:      "default",
				RuntimeKey: keyA,
				CredentialRef: core.CredentialRef{
					ResourceID:                   "platform:channel:52",
					CredentialIndex:              0,
					CredentialFingerprint:        common.GenerateHMAC("sk-disabled"),
					CredentialSubjectFingerprint: common.GenerateHMAC("sk-disabled"),
					Resolver:                     "channel_key",
				},
			},
			{
				Channel:    channel,
				Group:      "default",
				RuntimeKey: keyB,
				CredentialRef: core.CredentialRef{
					ResourceID:                   "platform:channel:52",
					CredentialIndex:              1,
					CredentialFingerprint:        common.GenerateHMAC("sk-enabled"),
					CredentialSubjectFingerprint: common.GenerateHMAC("sk-enabled"),
					Resolver:                     "channel_key",
				},
			},
		}),
		snapshots,
		scheduler.DefaultScoreWeights(),
	)
	facade := modelgateway.NewSmartDispatchFacade(modelgateway.SmartDispatchDeps{
		PolicyResolver: policy.NewDefaultGroupPolicyResolver(testkit.StaticSettingsProvider{Settings: core.SchedulerSettings{
			Enabled:         true,
			DefaultMode:     core.ModeOff,
			DefaultStrategy: core.StrategyBalanced,
			GroupPolicies: map[string]core.GroupPolicySetting{
				"default": {Mode: core.ModeActive, Strategy: core.StrategyBalanced, AutoMode: core.AutoModeSequential},
			},
		}}),
		AutoResolver: policy.NewDefaultAutoGroupResolver(&testkit.FakeGroupPermissionService{
			AutoGroups: map[string][]string{"default": []string{"default"}},
		}),
		Selector: selector,
		Recorder: &testkit.FakeExecutionRecorder{},
	})
	restoreWrapper := modelgatewayintegration.SetDefaultChannelSelectionWrapperForTest(modelgatewayintegration.NewChannelSelectionWrapper(facade, &testkit.FakeLegacyChannelSelector{}))
	t.Cleanup(restoreWrapper)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","messages":[{"role":"user","content":"hi"}]}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	nextCalled := false

	distribute(ctx, func(c *gin.Context) {
		nextCalled = true
		require.Equal(t, "sk-enabled", common.GetContextKeyString(c, constant.ContextKeyChannelKey))
		require.Equal(t, 1, common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex))
	})

	require.True(t, nextCalled)
	require.False(t, ctx.IsAborted())
	require.Equal(t, 1, service.GetChannelSelectionReservations(channel.Id))
	service.ReleaseChannelSelectionReservations(ctx)
}

func TestSetupContextForSelectedChannelLegacyStillUsesNextEnabledKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := &model.Channel{
		Id:     43,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b\nsk-c",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeyPollingIndex: 1,
		},
	}

	apiErr := SetupContextForSelectedChannel(ctx, channel, "gpt-5.4")

	require.Nil(t, apiErr)
	require.Equal(t, "sk-a", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.Equal(t, 0, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	require.Equal(t, 1, channel.ChannelInfo.MultiKeyPollingIndex)
}

func TestSetupContextForSelectedChannelLegacySkipsAttemptedRuntimeKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := &model.Channel{
		Id:     89,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
		},
	}
	service.MarkChannelRuntimeSelectionSkipped(ctx, service.ChannelRuntimeIdentity{
		ChannelID:          channel.Id,
		CredentialIndex:    0,
		CredentialIndexSet: true,
	})

	apiErr := SetupContextForSelectedChannel(ctx, channel, "gpt-5.4")

	require.Nil(t, apiErr)
	require.Equal(t, "sk-b", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey))
	require.Equal(t, 1, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
}

func TestSetupContextForSelectedChannelLegacyAppliesAccountProxy(t *testing.T) {
	db := setupDistributorProxyTestDB(t)
	require.NoError(t, db.Create(&model.ModelGatewayProxy{
		ID:       88,
		Name:     "legacy proxy",
		Protocol: "socks5",
		Address:  "127.0.0.1:1080",
		Enabled:  true,
	}).Error)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := &model.Channel{
		Id:     44,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:       true,
			MultiKeyProxyIDs: map[int]int{0: 88},
		},
	}

	apiErr := SetupContextForSelectedChannel(ctx, channel, "gpt-5.4")

	require.Nil(t, apiErr)
	require.Equal(t, "sk-a", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.Equal(t, 88, common.GetContextKeyInt(ctx, constant.ContextKeyChannelAccountProxyID))
	require.Equal(t, "socks5://127.0.0.1:1080", common.GetContextKeyString(ctx, constant.ContextKeyChannelAccountProxyURL))
}

func TestSetupContextForSelectedChannelAppliesAccountCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	denied := false
	allowed := true
	channel := &model.Channel{
		Id:     45,
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-a\nsk-b",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-4o",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true,
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					ResponsesWrite:       &denied,
					ChatCompletionsWrite: &allowed,
				},
			},
		},
	}

	apiErr := SetupContextForSelectedChannel(ctx, channel, "gpt-4o")
	capability, ok := common.GetContextKeyType[model.ChannelAccountCapability](ctx, constant.ContextKeyChannelAccountCapability)

	require.Nil(t, apiErr)
	require.True(t, ok)
	require.True(t, capability.HasResponsesWriteDenied())
	require.True(t, capability.HasChatCompletionsWriteAllowed())
}

func TestSetupContextForSelectedChannelUsesEndpointSpecificCodexCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	streamAllowed := true
	compactDenied := false
	compactAllowed := true
	channel := &model.Channel{
		Id:     46,
		Type:   constant.ChannelTypeCodex,
		Key:    "oauth-a\noauth-b",
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:           true,
			MultiKeyMode:         constant.MultiKeyModeRandom,
			MultiKeyPollingIndex: 0,
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
					CodexBackendCompactWrite:         &compactDenied,
				},
				1: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
					CodexBackendCompactWrite:         &compactAllowed,
				},
			},
		},
	}

	apiErr := SetupContextForSelectedChannelWithEndpoint(ctx, channel, "gpt-5.4", constant.EndpointTypeOpenAIResponseCompact)

	require.Nil(t, apiErr)
	require.Equal(t, "oauth-b", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	require.Equal(t, 1, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
}

func TestSetupContextForSelectedOpenAIOAuthJSONResponsesUsesCodexBackend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	streamAllowed := true
	channel := &model.Channel{
		Id:     47,
		Type:   constant.ChannelTypeOpenAI,
		Key:    `{"access_token":"access-a","account_id":"acct-a"}`,
		Status: common.ChannelStatusEnabled,
		Group:  "default",
		Models: "gpt-5.4",
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CodexBackendResponsesStreamWrite: &streamAllowed,
				},
			},
		},
	}

	apiErr := SetupContextForSelectedChannelWithEndpoint(ctx, channel, "gpt-5.4", constant.EndpointTypeOpenAIResponse)

	require.Nil(t, apiErr)
	require.Equal(t, constant.ChannelTypeCodex, common.GetContextKeyInt(ctx, constant.ContextKeyChannelType))
	require.Equal(t, constant.ChannelBaseURLs[constant.ChannelTypeCodex], common.GetContextKeyString(ctx, constant.ContextKeyChannelBaseUrl))
	require.Equal(t, "codex_backend", common.GetContextKeyString(ctx, constant.ContextKeyProviderSurface))
	require.Equal(t, "codex_backend_available", common.GetContextKeyString(ctx, constant.ContextKeyCapabilityClassification))
	require.JSONEq(t, `{"access_token":"access-a","account_id":"acct-a"}`, common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
}

func setupDistributorProxyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelGatewayProxy{}, &model.ModelGatewayProxyUsage{}))
	oldDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
