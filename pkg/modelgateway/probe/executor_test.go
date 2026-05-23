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

	candidateChannel := seedProbeExecutorChannel(t, db, 1, "candidate-low", "gpt-4.1", 1)
	selectedChannel := seedProbeExecutorChannel(t, db, 2, "selected-high", "gpt-4.1", 10)
	model.InitChannelCache()

	oldInvoker := relayInvoker
	invoked := false
	RegisterRelayInvoker(func(c *gin.Context, relayFormat types.RelayFormat) {
		invoked = true
		require.Equal(t, types.RelayFormatOpenAI, relayFormat)
		require.True(t, common.GetContextKeyBool(c, constant.ContextKeyHealthProbe))
		require.Equal(t, selectedChannel.Id, common.GetContextKeyInt(c, constant.ContextKeyChannelId))

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
				ChannelId:         selectedChannel.Id,
				ChannelType:       selectedChannel.Type,
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
	require.Equal(t, selectedChannel.Id, result.Channel.Id)
	require.Equal(t, selectedChannel.Id, result.RuntimeKey.ChannelID)
	require.Equal(t, "default", result.Group)
	require.Equal(t, constant.EndpointTypeOpenAI, result.RuntimeKey.EndpointType)
	require.Greater(t, result.TTFT, time.Duration(0))
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

func seedProbeExecutorChannel(t *testing.T, db *gorm.DB, id int, name string, modelName string, priority int64) *model.Channel {
	t.Helper()

	weight := uint(100)
	channel := &model.Channel{
		Id:          id,
		Type:        constant.ChannelTypeOpenAI,
		Name:        name,
		Key:         fmt.Sprintf("sk-%d", id),
		Status:      common.ChannelStatusEnabled,
		Group:       "default",
		Models:      modelName,
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: int64(id),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	return channel
}
