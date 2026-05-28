package probe

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayprovider "github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/pkg/modelgateway/recording"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildProbeRequestUsesStreaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	openAIRequest, ok := buildProbeRequest("gpt-4.1", constant.EndpointTypeOpenAI).(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, openAIRequest.Stream)
	require.True(t, *openAIRequest.Stream)
	require.NotNil(t, openAIRequest.StreamOptions)
	require.True(t, openAIRequest.StreamOptions.IncludeUsage)
	openAICtx, _ := newProbeGinContext(context.Background(), "probe-stream-openai", requestPathForEndpoint(constant.EndpointTypeOpenAI, "gpt-4.1"))
	require.True(t, openAIRequest.IsStream(openAICtx))

	responsesRequest, ok := buildProbeRequest("gpt-4.1", constant.EndpointTypeOpenAIResponse).(*dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.NotNil(t, responsesRequest.Stream)
	require.True(t, *responsesRequest.Stream)
	require.NotNil(t, responsesRequest.StreamOptions)
	require.True(t, responsesRequest.StreamOptions.IncludeUsage)
	responsesCtx, _ := newProbeGinContext(context.Background(), "probe-stream-responses", requestPathForEndpoint(constant.EndpointTypeOpenAIResponse, "gpt-4.1"))
	require.True(t, responsesRequest.IsStream(responsesCtx))

	claudeRequest, ok := buildProbeRequest("claude-3-5-sonnet", constant.EndpointTypeAnthropic).(*dto.ClaudeRequest)
	require.True(t, ok)
	require.NotNil(t, claudeRequest.Stream)
	require.True(t, *claudeRequest.Stream)
	claudeCtx, _ := newProbeGinContext(context.Background(), "probe-stream-claude", requestPathForEndpoint(constant.EndpointTypeAnthropic, "claude-3-5-sonnet"))
	require.True(t, claudeRequest.IsStream(claudeCtx))

	geminiPath := requestPathForEndpoint(constant.EndpointTypeGemini, "gemini-1.5-pro")
	require.Equal(t, "/v1beta/models/gemini-1.5-pro:streamGenerateContent", geminiPath)
	geminiRequest, ok := buildProbeRequest("gemini-1.5-pro", constant.EndpointTypeGemini).(*dto.GeminiChatRequest)
	require.True(t, ok)
	geminiCtx, _ := newProbeGinContext(context.Background(), "probe-stream-gemini", geminiPath)
	require.True(t, geminiRequest.IsStream(geminiCtx))

	require.Equal(
		t,
		constant.EndpointTypeOpenAIResponse,
		probeEndpointType(&model.Channel{Type: constant.ChannelTypeOpenAI}, "gpt-4.1", constant.EndpointTypeOpenAIResponseCompact),
	)
}

func TestProbeEndpointTypeHonorsExplicitCodexResponsesProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := dto.ChannelOtherSettings{
		ProviderProfile: modelgatewayprovider.ProfileOpenAICodex,
		ProxyProfile:    modelgatewayprovider.ProxyModeNativeResponses,
	}
	settingsBytes, err := common.Marshal(settings)
	require.NoError(t, err)
	channel := &model.Channel{
		Type:          constant.ChannelTypeOpenAI,
		OtherSettings: string(settingsBytes),
	}

	require.Equal(t, constant.EndpointTypeOpenAIResponse, endpointTypeForProbe(channel, "gpt-5.5"))
	require.Equal(t, constant.EndpointTypeOpenAIResponse, probeEndpointType(channel, "gpt-5.5", constant.EndpointTypeOpenAI))

	request, ok := buildProbeRequest("gpt-5.5", probeEndpointType(channel, "gpt-5.5", constant.EndpointTypeOpenAI)).(*dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.NotNil(t, request.Stream)
	require.True(t, *request.Stream)

	ctx, _ := newProbeGinContext(context.Background(), "probe-explicit-codex", requestPathForEndpoint(constant.EndpointTypeOpenAIResponse, "gpt-5.5"))
	common.SetContextKey(ctx, constant.ContextKeyChannelOtherSetting, settings)
	info := relaycommon.GenRelayInfoOpenAI(ctx, request)
	info.RelayMode = 0
	info.RequestURLPath = "/v1/responses"
	info.InitChannelMeta(ctx)
	require.True(t, request.IsStream(ctx))
}

func TestBuildProbeRequestUsesPromptLibraryCategory(t *testing.T) {
	openAIRequest, ok := buildProbeRequestWithCategory("gpt-4.1", constant.EndpointTypeOpenAI, PromptCategoryZH).(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Len(t, openAIRequest.Messages, 1)
	require.Contains(t, openAIRequest.Messages[0].Content, "正常")

	longRequest, ok := buildProbeRequestWithCategory("gpt-4.1", constant.EndpointTypeOpenAIResponse, PromptCategoryLong).(*dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Contains(t, string(longRequest.Input), "diagnostic text")
	require.NotNil(t, longRequest.MaxOutputTokens)
	require.Equal(t, uint(32), *longRequest.MaxOutputTokens)

	fallbackRequest, ok := buildProbeRequestWithCategory("gpt-4.1", constant.EndpointTypeOpenAI, "bad").(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "Reply with exactly: ok", fallbackRequest.Messages[0].Content)
}

func TestProbePromptCategoryNormalizationFallsBackToShort(t *testing.T) {
	require.Equal(t, []string{PromptCategoryShort, PromptCategoryLong}, NormalizePromptCategories([]string{"long", "bad", "short"}))
	require.Equal(t, []string{PromptCategoryShort}, NormalizePromptCategories([]string{"bad"}))
	candidate := ProbeCandidate{Model: "gpt-4.1", Group: "default", PromptCategories: []string{"bad"}}
	require.Equal(t, PromptCategoryShort, selectProbePromptCategory(candidate))
}

func TestProbeExecutorUsesNormalDistributorSelection(t *testing.T) {
	db := setupProbeExecutorTestDB(t)
	restoreSchedulerSetting := scheduler_setting.SetSettingForTest(scheduler_setting.DefaultSetting())
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(func() {
		modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
		restoreSchedulerSetting()
	})

	root := model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100000,
	}
	require.NoError(t, db.Create(&root).Error)

	candidateChannel := seedProbeExecutorChannel(t, db, 1, "candidate-low", "default", "gpt-4.1", 1)
	_ = seedProbeExecutorChannel(t, db, 2, "selected-high", "default", "gpt-4.1", 10)
	model.InitChannelCache()

	oldInvoker := relayInvoker
	invoked := false
	RegisterRelayInvoker(func(c *gin.Context, relayFormat types.RelayFormat) {
		invoked = true
		require.Equal(t, types.RelayFormatOpenAI, relayFormat)
		require.True(t, common.GetContextKeyBool(c, constant.ContextKeyHealthProbe))
		require.Equal(t, candidateChannel.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))

		start := time.Now().Add(-50 * time.Millisecond)
		info := &relaycommon.RelayInfo{
			RequestId:         c.GetString(common.RequestIdKey),
			UserId:            root.Id,
			UsingGroup:        "default",
			UserGroup:         "default",
			RequestModelName:  "gpt-4.1",
			OriginModelName:   "gpt-4.1",
			ContextModelName:  "gpt-4.1",
			StartTime:         start,
			FirstResponseTime: start.Add(12 * time.Millisecond),
			IsStream:          true,
			IsChannelTest:     true,
			RelayFormat:       types.RelayFormatOpenAI,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelId:         candidateChannel.Id,
				ChannelType:       candidateChannel.Type,
				UpstreamModelName: "gpt-4.1",
			},
			PriceData: types.PriceData{
				ModelRatio:      1,
				CompletionRatio: 1,
				GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
			},
		}
		info.SetEstimatePromptTokens(3)
		common.SetContextKey(c, constant.ContextKeyRelayInfo, info)
		c.Status(http.StatusOK)
	})
	t.Cleanup(func() {
		RegisterRelayInvoker(oldInvoker)
	})

	result := NewProbeExecutor(time.Second, nil).Execute(context.Background(), ProbeCandidate{
		Channel: candidateChannel,
		Model:   "gpt-4.1",
		Group:   "default",
		Key: core.RuntimeKey{
			RequestedModel: "gpt-4.1",
			UpstreamModel:  "gpt-4.1",
			ChannelID:      candidateChannel.Id,
			Group:          "default",
			EndpointType:   constant.EndpointTypeOpenAI,
		},
		Reason: reasonNoSamples,
	})

	require.True(t, invoked)
	require.NoError(t, result.Err)
	require.True(t, result.Success)
	require.NotNil(t, result.Channel)
	require.Equal(t, candidateChannel.Id, result.Channel.Id)
	require.Equal(t, candidateChannel.Id, result.RuntimeKey.ChannelID)
	require.Equal(t, "default", result.Group)
	require.Equal(t, constant.EndpointTypeOpenAI, result.RuntimeKey.EndpointType)
	require.Greater(t, result.TTFT, time.Duration(0))
}

func TestProbeExecutorInjectsSelectedPlanForResponsesViaChatProbe(t *testing.T) {
	db := setupProbeExecutorTestDB(t)
	restoreSchedulerSetting := scheduler_setting.SetSettingForTest(scheduler_setting.DefaultSetting())
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(func() {
		modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
		restoreSchedulerSetting()
	})

	root := model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100000,
	}
	require.NoError(t, db.Create(&root).Error)

	channel := seedProbeExecutorChannel(t, db, 11, "mimo-probe", "default", "mimo-v1", 1)
	settings := channel.GetOtherSettings()
	settings.ProviderProfile = modelgatewayprovider.ProfileMiMoCodexChat
	settings.ProxyProfile = modelgatewayprovider.ProxyModeResponsesViaChat
	settingsBytes, err := common.Marshal(settings)
	require.NoError(t, err)
	channel.OtherSettings = string(settingsBytes)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("settings", channel.OtherSettings).Error)
	model.InitChannelCache()

	oldInvoker := relayInvoker
	RegisterRelayInvoker(func(c *gin.Context, relayFormat types.RelayFormat) {
		require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), relayFormat)
		plan, ok := modelgatewayintegration.GetSelectedPlan(c)
		require.True(t, ok)
		require.NotNil(t, plan)
		require.Equal(t, channel.Id, plan.Channel.Id)
		require.Equal(t, "default", plan.SelectedGroup)
		require.Equal(t, modelgatewayprovider.ProfileMiMoCodexChat, plan.ProviderProfile)
		require.Equal(t, modelgatewayprovider.ProxyModeResponsesViaChat, plan.ProxyMode)
		require.Equal(t, "mimo-v1", plan.RuntimeKey.RequestedModel)
		require.Equal(t, "mimo-v1", plan.RuntimeKey.UpstreamModel)
		require.Equal(t, constant.EndpointTypeOpenAIResponse, plan.RuntimeKey.EndpointType)

		start := time.Now().Add(-50 * time.Millisecond)
		info := &relaycommon.RelayInfo{
			RequestId:         c.GetString(common.RequestIdKey),
			UserId:            root.Id,
			UsingGroup:        "default",
			UserGroup:         "default",
			RequestModelName:  "mimo-v1",
			OriginModelName:   "mimo-v1",
			ContextModelName:  "mimo-v1",
			StartTime:         start,
			FirstResponseTime: start.Add(20 * time.Millisecond),
			IsStream:          true,
			IsChannelTest:     true,
			RelayFormat:       types.RelayFormatOpenAIResponses,
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelId:         channel.Id,
				ChannelType:       channel.Type,
				UpstreamModelName: "mimo-v1",
			},
			PriceData: types.PriceData{
				ModelRatio:      1,
				CompletionRatio: 1,
				GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
			},
		}
		info.SetEstimatePromptTokens(4)
		common.SetContextKey(c, constant.ContextKeyRelayInfo, info)
		c.Status(http.StatusOK)
	})
	t.Cleanup(func() {
		RegisterRelayInvoker(oldInvoker)
	})

	result := NewProbeExecutor(time.Second, nil).Execute(context.Background(), ProbeCandidate{
		Channel: channel,
		Model:   "mimo-v1",
		Group:   "default",
		Key: core.RuntimeKey{
			RequestedModel: "mimo-v1",
			UpstreamModel:  "mimo-v1",
			ChannelID:      channel.Id,
			Group:          "default",
			EndpointType:   constant.EndpointTypeOpenAIResponse,
		},
		Reason: reasonFailureAvoidance,
	})

	require.NoError(t, result.Err)
	require.True(t, result.Success)
	require.Equal(t, constant.EndpointTypeOpenAIResponse, result.RuntimeKey.EndpointType)
}

func TestProbeAttemptResultUsesDispatchPlanRuntimeKey(t *testing.T) {
	legacyKey := core.RuntimeKey{
		RequestedModel: "gpt-5.4",
		UpstreamModel:  "gpt-5.4",
		ChannelID:      4,
		Group:          "codex-plus",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	planKey := legacyKey
	planKey.EndpointType = constant.EndpointTypeOpenAIResponse
	planKey.CapabilityFingerprint = modelgatewayprovider.ProfileOpenAICodex + "|" + modelgatewayprovider.ProxyModeNativeResponses

	result := ProbeRunResult{
		ProbeID:    "mg_probe_plan_key",
		Reason:     reasonLowScore,
		Channel:    &model.Channel{Id: 4, Name: "toioto"},
		Model:      "gpt-5.4",
		Group:      "codex-plus",
		RuntimeKey: legacyKey,
		TargetKey:  legacyKey,
		Success:    true,
		StatusCode: http.StatusOK,
		Plan: &core.DispatchPlan{
			RuntimeKey: planKey,
		},
	}

	attempt := result.AttemptResult()
	require.Equal(t, planKey, attempt.Key)
	require.Equal(t, constant.EndpointTypeOpenAIResponse, attempt.RuntimeKey().EndpointType)
}

func TestProbeSchedulerDispatchRecordIncludesScoreAndCandidateExplanation(t *testing.T) {
	db := setupProbeExecutorTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}, &model.ModelGatewayUserRequestSummary{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	restoreSchedulerSetting := scheduler_setting.SetSettingForTest(scheduler_setting.DefaultSetting())
	modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
	t.Cleanup(func() {
		modelgatewayintegration.ResetDefaultRuntimeObservabilityDeps()
		restoreSchedulerSetting()
	})

	root := model.User{
		Id:       1,
		Username: "root",
		Role:     common.RoleRootUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100000,
	}
	require.NoError(t, db.Create(&root).Error)
	channel := seedProbeExecutorChannel(t, db, 21, "score-probe", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{
		RequestedModel: "gpt-4.1",
		UpstreamModel:  "gpt-4.1",
		ChannelID:      channel.Id,
		Group:          "default",
		EndpointType:   constant.EndpointTypeOpenAI,
	}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SuccessRate:        0.5,
		SampleCount:        8,
		CostRatio:          0.4,
		CostReferenceRatio: 0.2,
		GroupPriorityRatio: 1,
	})
	recorder := recording.NewAsyncExecutionRecorder(16)
	enricher := scheduler.NewRuntimeSnapshotEnricher(nil, 1500, 8, 2).
		WithCostProfileProvider(fakeProbeCostProfileProvider{ratio: 0.4, ok: true})
	s := NewProbeScheduler(ProbeConfig{Enabled: true}, nil, nil, recorder).
		WithSnapshotStore(store).
		WithRuntimeSnapshotEnricher(enricher)
	plan := s.buildDispatchPlan(ProbeCandidate{
		Channel: channel,
		Model:   "gpt-4.1",
		Group:   "default",
		Key:     key,
		Reason:  reasonLowScore,
	})
	require.NotNil(t, plan)
	require.Greater(t, plan.ScoreTotal, 0.0)
	require.Len(t, plan.Candidates, 1)
	require.True(t, plan.Candidates[0].Selected)
	selector := scheduler.NewDefaultSmartChannelSelector(nil, store, modelgatewayintegration.RuntimePolicySetting().ScoreWeights).
		WithRuntimeSnapshotEnricher(enricher)
	expected := selector.ScoreCandidate(core.Candidate{
		Channel:         channel,
		Group:           plan.SelectedGroup,
		UpstreamModel:   plan.RuntimeKey.UpstreamModel,
		ProviderProfile: plan.ProviderProfile,
		ProxyMode:       plan.ProxyMode,
		RuntimeKey:      plan.RuntimeKey,
	}, probeDispatchPolicy("default"))
	require.InDelta(t, expected.Score.Total, plan.ScoreTotal, 0.0001)
	require.InDelta(t, expected.Score.Breakdown["cost"], plan.Candidates[0].ScoreBreakdown["cost"], 0.0001)
	require.Equal(t, 1.0, plan.Candidates[0].ScoreBreakdown["cost"])
	require.Equal(t, expected.Explanation.ScoreBreakdown, plan.Candidates[0].ScoreBreakdown)
	require.Equal(t, reasonLowScore, plan.ProbeReason)

	result := ProbeRunResult{
		ProbeID:    "mg_probe_score_record",
		Reason:     reasonLowScore,
		Channel:    channel,
		Model:      "gpt-4.1",
		Group:      "default",
		RuntimeKey: key,
		TargetKey:  key,
		StartedAt:  time.Now(),
		Success:    true,
		StatusCode: http.StatusOK,
		Duration:   50 * time.Millisecond,
		TTFT:       10 * time.Millisecond,
		Plan:       plan,
	}
	recorder.Record(context.Background(), result.DispatchRecord())
	recorder.Report(context.Background(), result.AttemptResult())

	require.Eventually(t, func() bool {
		var count int64
		if err := db.Model(&model.ModelExecutionRecord{}).
			Where("request_id = ?", result.ProbeID).
			Count(&count).Error; err != nil {
			return false
		}
		return count >= 2
	}, time.Second, 10*time.Millisecond)

	var dispatch model.ModelExecutionRecord
	require.NoError(t, db.Where("request_id = ? AND smart_handled = ?", result.ProbeID, true).First(&dispatch).Error)
	require.Greater(t, dispatch.ScoreTotal, 0.0)
	require.Contains(t, dispatch.ScoreBreakdown, "completion_rate")
	require.Contains(t, dispatch.RequestMeta, "candidate_explanations")
	require.Contains(t, dispatch.RequestMeta, `"is_health_probe":true`)
	require.Contains(t, dispatch.RequestMeta, `"probe_reason":"low_score"`)

	var summary model.ModelGatewayUserRequestSummary
	require.NoError(t, db.Where("request_id = ?", result.ProbeID).First(&summary).Error)
	require.True(t, summary.FinalSuccess)
}

type fakeProbeCostProfileProvider struct {
	ratio float64
	ok    bool
}

func (p fakeProbeCostProfileProvider) CostRatio(channelID int, upstreamModel string) (float64, bool) {
	return p.ratio, p.ok
}

func setupProbeExecutorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}))

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL

	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		if oldMemoryCache && oldDB != nil {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func seedProbeExecutorChannel(t *testing.T, db *gorm.DB, id int, name string, group string, modelName string, priority int64) *model.Channel {
	t.Helper()

	weight := uint(100)
	channel := &model.Channel{
		Id:          id,
		Type:        constant.ChannelTypeOpenAI,
		Name:        name,
		Key:         fmt.Sprintf("sk-%d", id),
		Status:      common.ChannelStatusEnabled,
		Group:       group,
		Models:      modelName,
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: int64(id),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	return channel
}
