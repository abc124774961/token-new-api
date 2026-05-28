package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
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

func TestResponsesRequestHasImageGenerationToolIgnoresDeclaredTools(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tools": [{"type": "image_generation"}]
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": {"type": "image_generation"}
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresStringToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": "image_generation"
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresNestedToolChoice(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tool_choice": {
			"type": "allowed_tools",
			"tools": [{"type": "image_generation"}]
		}
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolDetectsCodexSkillIntent(t *testing.T) {
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

	require.True(t, responsesRequestHasImageGenerationTool(ctx))
}

func TestResponsesRequestHasImageGenerationToolIgnoresOtherTools(t *testing.T) {
	ctx := newResponsesCapabilityTestContext(`{
		"model": "gpt-5.5",
		"tools": [{"type": "web_search_preview"}],
		"tool_choice": "auto"
	}`)

	require.False(t, responsesRequestHasImageGenerationTool(ctx))
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
