package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	modelgatewaycore "github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newRelayRetryContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx
}

func TestRelayTotalDurationWatchdogAppliesOnlySmartStreamingNonSpecificRequests(t *testing.T) {
	setting := scheduler_setting.DefaultSetting()
	setting.RelayTotalTimeoutEnabled = true
	setting.RelayTotalTimeoutSeconds = 180
	restore := scheduler_setting.SetSettingForTest(setting)
	t.Cleanup(restore)

	ctx := newRelayRetryContext()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plan := &modelgatewaycore.DispatchPlan{PolicyMode: modelgatewaycore.ModeActive}
	info := &relaycommon.RelayInfo{
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		StartTime:   time.Now(),
		RelayFormat: types.RelayFormatOpenAI,
	}

	require.True(t, relayTotalDurationWatchdogApplies(ctx, info, plan))
	require.Equal(t, 180*time.Second, relayTotalDurationTimeout(ctx, info, plan))

	ctx.Set("specific_channel_id", "1")
	require.False(t, relayTotalDurationWatchdogApplies(ctx, info, plan))

	ctx = newRelayRetryContext()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plan.IsHealthProbe = true
	require.False(t, relayTotalDurationWatchdogApplies(ctx, info, plan))

	plan.IsHealthProbe = false
	info.IsStream = false
	require.False(t, relayTotalDurationWatchdogApplies(ctx, info, plan))

	info.IsStream = true
	setting.RelayTotalTimeoutEnabled = false
	restoreDisabled := scheduler_setting.SetSettingForTest(setting)
	defer restoreDisabled()
	require.False(t, relayTotalDurationWatchdogApplies(ctx, info, plan))
}

func TestRelayTotalDurationWatchdogCanCancelOnlyBeforeOutput(t *testing.T) {
	ctx := newRelayRetryContext()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{StartTime: time.Now()}

	require.True(t, relayTotalDurationWatchdogCanCancel(ctx, info))

	info.ForceSetFirstResponseTime()
	require.False(t, relayTotalDurationWatchdogCanCancel(ctx, info))
	require.True(t, relayTotalDurationAfterOutput(info, 181*time.Second, 180*time.Second))

	info = &relaycommon.RelayInfo{StartTime: time.Now()}
	helper.MarkRelayDownstreamStarted(ctx)
	require.True(t, relayTotalDurationWatchdogCanCancel(ctx, info))

	common.SetContextKey(ctx, constant.ContextKeyRelayResponseStarted, true)
	require.False(t, relayTotalDurationWatchdogCanCancel(ctx, info))
}

func TestRelayRuntimeIdentityFallsBackToChannelScopeWithoutSelectedPlan(t *testing.T) {
	ctx := newRelayRetryContext()
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 9021)
	common.SetContextKey(ctx, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(ctx, constant.ContextKeyChannelMultiKeyIndex, 3)
	ctx.Set("original_model", "gpt-5.5")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")

	identity := relayRuntimeIdentity(ctx, 9021)

	require.Equal(t, 9021, identity.ChannelID)
	require.False(t, identity.HasAccountScope())
	require.False(t, identity.CredentialIndexSet)
}

func TestRelayRuntimeIdentityPrefersSelectedPlanAccountScope(t *testing.T) {
	ctx := newRelayRetryContext()
	plan := &modelgatewaycore.DispatchPlan{
		RuntimeKey: modelgatewaycore.RuntimeKey{
			ChannelID:           9022,
			RequestedModel:      "gpt-5.5",
			Group:               "default",
			EndpointType:        constant.EndpointTypeOpenAI,
			AccountID:           "acct-1",
			CredentialIndex:     1,
			CredentialSubjectFP: "subject-1",
			CredentialFP:        "credential-1",
		},
		CredentialRef: modelgatewaycore.CredentialRef{
			ResourceID:                   "platform:channel:9022",
			AccountID:                    "acct-1",
			CredentialIndex:              1,
			CredentialSubjectFingerprint: "subject-1",
			CredentialFingerprint:        "credential-1",
		},
	}
	modelgatewayintegration.SetSelectedPlan(ctx, plan)

	identity := relayRuntimeIdentity(ctx, 9022)

	require.Equal(t, 9022, identity.ChannelID)
	require.True(t, identity.HasAccountScope())
	require.True(t, identity.CredentialIndexSet)
	require.Equal(t, "acct-1", identity.AccountID)
	require.Equal(t, 1, identity.CredentialIndex)
}

func TestSelectedRelayGroupForTracePrefersActualGroupOverAuto(t *testing.T) {
	ctx := newRelayRetryContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "codex-plus")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "codex-plus")
	info := &relaycommon.RelayInfo{
		TokenGroup: "auto",
		UsingGroup: "codex-plus",
	}

	group := selectedRelayGroupForTrace(ctx, info, &service.RetryParam{TokenGroup: "auto"}, "auto")

	require.Equal(t, "codex-plus", group)
}

func initRelayRetryColumnNames(t *testing.T) {
	t.Helper()

	originalIsMasterNode := common.IsMasterNode
	originalSQLitePath := common.SQLitePath
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")
	defer func() {
		common.IsMasterNode = originalIsMasterNode
		common.SQLitePath = originalSQLitePath
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	}()

	common.IsMasterNode = false
	common.SQLitePath = fmt.Sprintf("file:%s_init?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, os.Setenv("SQL_DSN", "local"))

	require.NoError(t, model.InitDB())
	if model.DB != nil {
		sqlDB, err := model.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

func serviceSetupRelayRetryDB(t *testing.T) *gorm.DB {
	t.Helper()
	initRelayRetryColumnNames(t)

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func serviceSeedRelayRetryChannel(t *testing.T, db *gorm.DB, id int, group string, modelName string, priority int64) {
	t.Helper()

	weight := uint(100)
	channel := &model.Channel{
		Id:          id,
		Name:        "channel",
		Key:         "sk-test",
		Status:      common.ChannelStatusEnabled,
		Group:       group,
		Models:      modelName,
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: int64(id),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
}

func TestRequiresCodexImageToolForRelayIgnoresToolChoiceDeclaration(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		Request: &dto.OpenAIResponsesRequest{
			Model:      "gpt-5.5",
			ToolChoice: []byte(`{"type":"image_generation"}`),
		},
	}

	require.False(t, requiresCodexImageToolForRelay(info))
}

func TestRequiresCodexImageToolForRelayDetectsCodexSkillIntent(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		Request: &dto.OpenAIResponsesRequest{
			Model: "gpt-5.5",
			Input: []byte(`[
				{"role":"user","content":[
					{"type":"input_text","text":"[$imagegen](/Users/frode.luo/.codex/skills/.system/imagegen/SKILL.md) 风景"}
				]}
			]`),
			ToolChoice: []byte(`"auto"`),
		},
	}

	require.True(t, requiresCodexImageToolForRelay(info))
}

func withAutoGroupsForRelayTest(t *testing.T, groups []string) {
	t.Helper()

	original := setting.AutoGroups2JsonString()
	payload, err := common.Marshal(groups)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(string(payload)))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(original))
	})
}

func TestShouldRetryAllowsConcurrencyLimitFailoverForAutoGroup(t *testing.T) {
	ctx := newRelayRetryContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "default")
	withAutoGroupsForRelayTest(t, []string{"default", "vip"})
	_ = serviceSetupRelayRetryDB(t)

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("Concurrency limit exceeded for user, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		429,
	)

	require.False(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 0, param.GetExtraRetries())
}

func TestShouldRetryAllowsConcurrencyLimitFailoverWhenAlternativeGroupExists(t *testing.T) {
	ctx := newRelayRetryContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "default")
	withAutoGroupsForRelayTest(t, []string{"default", "vip"})

	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 501, "vip", "gpt-5.5", 10)

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("Concurrency limit exceeded for user, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		429,
	)

	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 1, param.GetExtraRetries())
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyForceNextAutoGroup))
}

func TestShouldRetryAllowsCodexPendingRequestsFailoverWhenAlternativeGroupExists(t *testing.T) {
	ctx := newRelayRetryContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "default")
	withAutoGroupsForRelayTest(t, []string{"default", "vip"})

	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 502, "vip", "gpt-5.5", 10)

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("status_code=429, Too many pending requests, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		429,
	)

	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 1, param.GetExtraRetries())
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyForceNextAutoGroup))
}

func TestShouldRetryAllowsGeneric429FailoverWhenAlternativePeerChannelExists(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 461, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 462, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"461"})
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("rate limit exceeded"),
		types.ErrorCodeBadResponseStatusCode,
		429,
	)

	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 1, param.GetExtraRetries())
	require.Equal(t, "overload_skip", classifyRelayAttemptError(ctx, err))
	require.Equal(t, "switch_channel", retryActionForAttempt(ctx, err, true))
}

func TestOpenAIInsufficientQuota429ClassifiesAsBalanceOrQuota(t *testing.T) {
	ctx := newRelayRetryContext()
	err := types.WithOpenAIError(types.OpenAIError{
		Message: "You exceeded your current quota, please check your plan and billing details.",
		Type:    "insufficient_quota",
		Code:    "insufficient_quota",
	}, http.StatusTooManyRequests)

	require.True(t, service.IsBalanceInsufficientError(err))
	require.Equal(t, modelgatewaycore.ErrorCategoryBalanceOrQuota, classifyRelayAttemptError(ctx, err))
	require.Equal(t, "switch_channel", retryActionForAttempt(ctx, err, true))
}

func TestSetupFailureFailoverSkipsBrokenChannelWhenAlternativeExists(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 481, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 482, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	err := types.NewError(errors.New("no enabled keys"), types.ErrorCodeChannelNoAvailableKey)

	require.True(t, prepareModelGatewaySetupFailureRetry(ctx, &model.Channel{Id: 481}, err, param))
	require.Equal(t, 1, param.GetExtraRetries())
	require.Equal(t, modelgatewaycore.ErrorCategoryAuthConfigError, setupFailureErrorCategory(ctx, err))
}

func TestSetupFailureFailoverRejectsSpecificChannel(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 483, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 484, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("specific_channel_id", 483)
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	err := types.NewError(errors.New("credential disabled"), types.ErrorCodeChannelNoAvailableKey)

	require.False(t, prepareModelGatewaySetupFailureRetry(ctx, &model.Channel{Id: 483}, err, param))
	require.Equal(t, 0, param.GetExtraRetries())
}

func TestShouldRetryRejectsGeneric429ForSpecificChannel(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 463, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 464, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("specific_channel_id", 463)
	ctx.Set("use_channel", []string{"463"})
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("rate limit exceeded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)

	require.False(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 0, param.GetExtraRetries())
}

func TestShouldRetryAllowsUpstreamRateLimitWrappedAsBadRequest(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 451, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 452, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"451"})
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("您已达到当前订阅的速率配额限制，请在 42 分 54 秒 后重试，参考限速规则：https://***.io/***/***"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)

	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 1, param.GetExtraRetries())
}

func TestShouldRetryAllowsGenericUpstreamBadRequestFailover(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 471, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 472, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"471"})

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("upstream rejected request for this channel"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)

	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 1, param.GetExtraRetries())
}

func TestShouldRetryRejectsInvalidEncryptedContent(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 473, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 474, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"473"})

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.WithOpenAIError(types.OpenAIError{
		Message: "The encrypted content 我先查一下 could not be verified. Reason: Encrypted content could not be decrypted or parsed.",
		Type:    "openai_error",
		Code:    "invalid_encrypted_content",
	}, http.StatusBadRequest)

	require.False(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 0, param.GetExtraRetries())
	require.False(t, shouldFailoverToAlternativeChannel(ctx, err))
	require.Equal(t, modelgatewaycore.ErrorCategoryClientRequestError, classifyRelayAttemptError(ctx, err))
	require.Equal(t, "stop", retryActionForAttempt(ctx, err, true))
	_, avoid := channelFailureAvoidanceReason(err)
	require.False(t, avoid)
}

func TestShouldRetryRejectsLocalBadRequestWhenNoRetryBudget(t *testing.T) {
	ctx := newRelayRetryContext()

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewErrorWithStatusCode(
		errors.New("invalid request body"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)

	require.False(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 0, param.GetExtraRetries())
}

func TestShouldRetryAllowsServerErrorFailoverWithAlternativePeerChannel(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 601, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 602, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"601"})

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("invalid character 'e' looking for beginning of value"),
		types.ErrorCodeBadResponseBody,
		500,
	)

	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 1, param.GetExtraRetries())
}

func TestShouldRetryRejectsFailoverAfterResponseStarted(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 611, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 612, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"611"})
	common.SetContextKey(ctx, constant.ContextKeyRelayResponseStarted, true)

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("upstream stream ended before response.completed"),
		types.ErrorCodeBadResponse,
		http.StatusInternalServerError,
	)

	require.False(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 0, param.GetExtraRetries())
}

func TestRelayClientAbortClassificationDoesNotRetry(t *testing.T) {
	ctx := newRelayRetryContext()
	baseCtx, cancel := context.WithCancel(context.Background())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(baseCtx)
	cancel()

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	err := types.NewErrorWithStatusCode(
		context.Canceled,
		types.ErrorCodeDoRequestFailed,
		relayStatusClientClosedRequest,
		types.ErrOptionWithSkipRetry(),
	)

	require.True(t, relayClientAborted(ctx, nil, err))
	require.False(t, relayChannelInducedClientAbort(ctx, nil, err))
	require.Equal(t, "client_aborted", classifyRelayAttemptError(ctx, err))
	require.Equal(t, "client_aborted", retryActionForAttempt(ctx, err, false))
	require.False(t, shouldRetry(ctx, err, param, 3))
}

func TestChannelInducedClientAbortCanSwitchChannel(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 641, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 642, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("use_channel", []string{"641"})
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonInternalFirstByteTimeout, errors.New("channel induced stream stall"))
	err := types.NewErrorWithStatusCode(
		context.Canceled,
		types.ErrorCodeDoRequestFailed,
		relayStatusClientClosedRequest,
	)

	require.True(t, relayClientAborted(ctx, info, err))
	require.True(t, relayChannelInducedClientAbort(ctx, info, err))
	require.Equal(t, modelgatewaycore.ErrorCategoryChannelInducedClientAbort, classifyRelayAttemptError(ctx, err))
	willRetry := shouldRetry(ctx, err, param, 0)
	require.True(t, willRetry)
	require.Equal(t, "switch_channel", retryActionForAttempt(ctx, err, willRetry))
	require.True(t, setChannelInducedRetryRoutingIntentIfNeeded(ctx, &model.Channel{Id: 641, Name: "stalled-channel"}, 0, "switch_channel"))
	intent, ok := modelgatewaycore.GetRetryRoutingIntent(ctx)
	require.True(t, ok)
	require.Equal(t, modelgatewaycore.RelayAttemptCancelReasonChannelInducedClientAbort, intent.Reason)
	require.Equal(t, 641, intent.FailedChannelID)
	require.Equal(t, modelgatewaycore.RetryRoutingQueuePriority, intent.QueuePriority)
}

func TestRelayClientAbortIgnoresCanceledContextAfterNormalStreamEnd(t *testing.T) {
	ctx := newRelayRetryContext()
	baseCtx, cancel := context.WithCancel(context.Background())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(baseCtx)
	cancel()

	info := &relaycommon.RelayInfo{StreamStatus: relaycommon.NewStreamStatus()}
	info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonEOF, nil)

	require.False(t, relayClientAborted(ctx, info, nil))
}

func TestShouldRetryAllowsFirstByteTimeoutFailoverWithAlternativePeerChannel(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 621, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 622, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"621"})
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	err := newRelayFirstByteTimeoutError(relayFirstByteTimeout)

	require.Equal(t, modelgatewaycore.ErrorCategoryTimeout, classifyRelayAttemptError(ctx, err))
	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, "switch_channel", retryActionForAttempt(ctx, err, true))
	require.False(t, relayClientAborted(ctx, nil, err))
}

func TestFirstByteRetryIntentIsOnlySetWhenSwitchWillRetry(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 631, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 632, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"631"})
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	service.MarkChannelSelectionSkipped(ctx, 631)

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	err := newRelayFirstByteTimeoutError(relayFirstByteTimeout)

	willRetry := shouldRetry(ctx, err, param, 0)
	require.True(t, willRetry)
	require.Equal(t, "switch_channel", retryActionForAttempt(ctx, err, willRetry))
	require.True(t, setFirstByteRetryRoutingIntentIfNeeded(ctx, &model.Channel{Id: 631, Name: "slow-first-byte"}, 0, true, willRetry, "switch_channel"))
	intent, ok := modelgatewaycore.GetRetryRoutingIntent(ctx)
	require.True(t, ok)
	require.Equal(t, modelgatewaycore.RelayAttemptCancelReasonFirstByteTimeout, intent.Reason)
	require.Equal(t, 631, intent.FailedChannelID)
	require.Equal(t, modelgatewaycore.RetryRoutingQueuePriority, intent.QueuePriority)

	service.MarkChannelSelectionSkipped(ctx, 632)
	err = newRelayFirstByteTimeoutError(relayFirstByteTimeout)
	willRetry = shouldRetry(ctx, err, param, 0)
	require.False(t, willRetry)
	require.Equal(t, "stop", retryActionForAttempt(ctx, err, willRetry))
	modelgatewaycore.ClearRetryRoutingIntent(ctx)
	require.False(t, setFirstByteRetryRoutingIntentIfNeeded(ctx, &model.Channel{Id: 632, Name: "other"}, 1, true, willRetry, "stop"))
	_, ok = modelgatewaycore.GetRetryRoutingIntent(ctx)
	require.False(t, ok)
}

func TestShouldRetryRejectsFirstByteTimeoutWithoutAlternativeOrBudget(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 623, "default", "gpt-5.5", 10)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"623"})
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	err := newRelayFirstByteTimeoutError(relayFirstByteTimeout)

	require.Equal(t, modelgatewaycore.ErrorCategoryTimeout, classifyRelayAttemptError(ctx, err))
	require.False(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 0, param.GetExtraRetries())
	require.Equal(t, "stop", retryActionForAttempt(ctx, err, false))
}

func TestModelGatewayAttemptFailureScopeClassifiesClientAbort(t *testing.T) {
	result := modelgatewaycore.AttemptResult{
		Success:       false,
		ClientAborted: true,
		ErrorCategory: modelgatewaycore.ErrorCategoryClientAborted,
		RetryAction:   "client_aborted",
	}

	require.Equal(t, modelgatewaycore.FailureScopeClient, modelGatewayAttemptFailureScope(result))
	require.Empty(t, modelGatewayAttemptSwitchReason(result))
}

func TestModelGatewayAttemptFailureScopeClassifiesFirstByteSwitchAsAccount(t *testing.T) {
	result := modelgatewaycore.AttemptResult{
		Success:       false,
		WillRetry:     true,
		RetryAction:   "switch_channel",
		RetryReason:   modelgatewaycore.RelayAttemptCancelReasonFirstByteTimeout,
		ErrorCategory: modelgatewaycore.ErrorCategoryTimeout,
	}

	require.Equal(t, modelgatewaycore.FailureScopeAccount, modelGatewayAttemptFailureScope(result))
	require.Equal(t, modelgatewaycore.RelayAttemptCancelReasonFirstByteTimeout, modelGatewayAttemptSwitchReason(result))
}

func TestModelGatewayAttemptFailureScopeClassifiesResourceFailures(t *testing.T) {
	for _, category := range []string{
		modelgatewaycore.ErrorCategoryBalanceOrQuota,
		modelgatewaycore.ErrorCategoryAuthConfigError,
		modelgatewaycore.ErrorCategoryUnsupportedCapability,
	} {
		result := modelgatewaycore.AttemptResult{
			Success:       false,
			ErrorCategory: category,
		}
		require.Equal(t, modelgatewaycore.FailureScopeResource, modelGatewayAttemptFailureScope(result))
	}
}

func TestModelGatewayAttemptFailureScopeClassifiesLocalConcurrencyAsSystem(t *testing.T) {
	result := modelgatewaycore.AttemptResult{
		Success:            false,
		ConcurrencyLimited: true,
		ErrorCategory:      modelgatewaycore.ErrorCategoryLocalConcurrencyLimit,
	}

	require.Equal(t, modelgatewaycore.FailureScopeSystem, modelGatewayAttemptFailureScope(result))
}

func TestRelayFirstByteWatchdogAppliesOnlySafeStreamingSmartRequests(t *testing.T) {
	ctx := newRelayRetryContext()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plan := &modelgatewaycore.DispatchPlan{PolicyMode: modelgatewaycore.ModeActive}
	info := &relaycommon.RelayInfo{
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		StartTime:   time.Now(),
		RelayFormat: types.RelayFormatOpenAI,
	}

	require.True(t, relayFirstByteWatchdogApplies(ctx, info, plan))

	info.RelayMode = relayconstant.RelayModeAudioSpeech
	require.False(t, relayFirstByteWatchdogApplies(ctx, info, plan))

	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.IsStream = false
	require.False(t, relayFirstByteWatchdogApplies(ctx, info, plan))

	info.IsStream = true
	info.IsChannelTest = true
	require.False(t, relayFirstByteWatchdogApplies(ctx, info, plan))

	info.IsChannelTest = false
	plan.IsHealthProbe = true
	require.False(t, relayFirstByteWatchdogApplies(ctx, info, plan))

	plan.IsHealthProbe = false
	plan.PolicyMode = modelgatewaycore.ModeShadow
	require.False(t, relayFirstByteWatchdogApplies(ctx, info, plan))

	plan.PolicyMode = modelgatewaycore.ModeActive
	helper.MarkRelayDownstreamStarted(ctx)
	require.False(t, relayFirstByteWatchdogApplies(ctx, info, plan))
}

func TestRelayFirstByteWatchdogCanCancelRequiresNoDownstreamWrite(t *testing.T) {
	ctx := newRelayRetryContext()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	start := time.Now()
	info := &relaycommon.RelayInfo{
		StartTime:         start,
		FirstResponseTime: start.Add(-time.Second),
	}

	require.True(t, relayFirstByteWatchdogCanCancel(ctx, info))

	helper.MarkRelayResponseStarted(ctx)
	require.False(t, relayFirstByteWatchdogCanCancel(ctx, info))

	ctx = newRelayRetryContext()
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Status(http.StatusOK)
	ctx.Writer.WriteHeaderNow()
	require.False(t, relayFirstByteWatchdogCanCancel(ctx, info))
}

func TestRelayRequestContextCanceledDetectsQueueWaitAbort(t *testing.T) {
	ctx := newRelayRetryContext()
	baseCtx, cancel := context.WithCancel(context.Background())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(baseCtx)
	cancel()

	require.True(t, relayRequestContextCanceled(ctx))
}

func TestShouldRetryAllowsServerErrorFailoverToNextAutoGroup(t *testing.T) {
	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"701"})
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyAutoGroup, "default")
	withAutoGroupsForRelayTest(t, []string{"default", "vip"})

	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 701, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 702, "vip", "gpt-5.5", 10)

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("invalid character 'e' looking for beginning of value"),
		types.ErrorCodeBadResponseBody,
		500,
	)

	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, 1, param.GetExtraRetries())
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyForceNextAutoGroup))
}

func TestProcessChannelErrorSkipsPersistingRetriableIntermediateFailure(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	t.Cleanup(func() {
		service.ClearChannelFailureAvoidance(2)
	})

	ctx := newRelayRetryContext()
	ctx.Set("id", 1)
	ctx.Set("username", "tester")
	ctx.Set("token_name", "default")
	ctx.Set("original_model", "gpt-5.5")
	ctx.Set("token_id", 5)
	ctx.Set("group", "auto")
	ctx.Set("channel_id", 2)
	ctx.Set("channel_name", "channel-2")
	ctx.Set("channel_type", 1)
	ctx.Set("use_channel", []string{"2"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now())

	err := types.NewOpenAIError(
		errors.New("Concurrency limit exceeded for user, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		429,
	)
	processChannelError(ctx, *types.NewChannelError(2, 1, "channel-2", false, "", false), err, false)

	var count int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&count).Error)
	require.Equal(t, int64(0), count)
	require.Nil(t, service.GetChannelFailureAvoidanceStatus(2))
}

func TestProcessChannelErrorSkipsFailureAvoidanceForOverload429(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		service.ClearChannelFailureAvoidance(916)
		service.ClearChannelConcurrencyForTest()
	})

	err := types.NewOpenAIError(
		errors.New("rate limit exceeded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)
	ctx := newRelayRetryContext()
	processChannelError(ctx, *types.NewChannelError(916, 1, "channel-916", false, "", true), err, false)

	require.Equal(t, "overload_skip", classifyRelayAttemptError(ctx, err))
	require.Nil(t, service.GetChannelFailureAvoidanceStatus(916))
	require.Nil(t, service.GetChannelConcurrencyCooldownStatus(916))
	require.False(t, service.IsRuntimeBalanceInsufficientChannelID(916))
}

func TestProcessChannelErrorRecordsTemporaryAvoidanceForBadGateway(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 6
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(902)
	})

	err := types.NewOpenAIError(
		errors.New("bad response status code 502"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadGateway,
	)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(902, 1, "channel-902", false, "", false), err, false)

	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 902, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 903, "default", "gpt-5.5", 10)

	param := &service.RetryParam{
		Ctx:        newRelayRetryContext(),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	channel, _, selectErr := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, selectErr)
	require.NotNil(t, channel)
	require.Equal(t, 903, channel.Id)
}

func TestProcessChannelErrorRecordsTemporaryAvoidanceForWrappedRateLimit(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 6
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(904)
	})

	err := types.NewOpenAIError(
		errors.New("status_code=400, 您已达到当前订阅的速率配额限制，请在 42 分 13 秒 后重试"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(904, 1, "channel-904", false, "", false), err, false)

	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 904, "default", "gpt-5.5", 10)
	serviceSeedRelayRetryChannel(t, db, 905, "default", "gpt-5.5", 10)

	param := &service.RetryParam{
		Ctx:        newRelayRetryContext(),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	channel, _, selectErr := service.CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, selectErr)
	require.NotNil(t, channel)
	require.Equal(t, 905, channel.Id)
}

func TestProcessChannelErrorRecordsTemporaryAvoidanceForGenericUpstreamBadRequest(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(906)
	})

	err := types.NewOpenAIError(
		errors.New("upstream rejected request for this channel"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(906, 1, "channel-906", false, "", false), err, false)

	status := service.GetChannelFailureAvoidanceStatus(906)
	require.NotNil(t, status)
	require.True(t, status.Active)
	require.Equal(t, "upstream_error:400:bad_response_status_code", status.Reason)
	require.Equal(t, 1, status.FailureCount)
}

func TestProcessChannelErrorExtendsTemporaryAvoidanceForRepeatedUpstreamErrors(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 6
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(908)
	})

	err := types.NewOpenAIError(
		errors.New("upstream rejected request for this channel"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(908, 1, "channel-908", false, "", false), err, false)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(908, 1, "channel-908", false, "", false), err, false)

	status := service.GetChannelFailureAvoidanceStatus(908)
	require.NotNil(t, status)
	require.True(t, status.Active)
	require.Equal(t, "upstream_error:400:bad_response_status_code", status.Reason)
	require.Equal(t, 2, status.FailureCount)
	require.Greater(t, time.Unix(status.Until, 0).Sub(time.Now()), 12*time.Second)
}

func TestProcessChannelErrorSkipsTemporaryAvoidanceForLocalBadRequest(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(910)
	})

	err := types.NewErrorWithStatusCode(
		errors.New("invalid request body"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(910, 1, "channel-910", false, "", false), err, false)

	require.Nil(t, service.GetChannelFailureAvoidanceStatus(910))
}

func TestProcessChannelErrorSkipsTemporaryAvoidanceForLocalConcurrencyLimit(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(912)
	})

	err := types.NewErrorWithStatusCode(
		errors.New("channel #912 reached configured max concurrency 1"),
		types.ErrorCodeChannelConcurrencyLimit,
		http.StatusTooManyRequests,
	)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(912, 1, "channel-912", false, "", false), err, false)

	require.Nil(t, service.GetChannelFailureAvoidanceStatus(912))
}

func TestTraceChannelFailureMarksConcurrencyLimitedWithoutCooldown(t *testing.T) {
	ctx := newRelayRetryContext()
	err := types.NewErrorWithStatusCode(
		errors.New("channel #912 reached configured max concurrency 1"),
		types.ErrorCodeChannelConcurrencyLimit,
		http.StatusTooManyRequests,
	)

	traceChannelFailure(ctx, *types.NewChannelError(912, 1, "channel-912", false, "", false), err, false)

	trace, ok := common.GetContextKeyType[[]map[string]interface{}](ctx, constant.ContextKeyChannelFailureTrace)
	require.True(t, ok)
	require.Len(t, trace, 1)
	require.Equal(t, true, trace[0]["concurrency_limited"])
	require.Equal(t, "switch_channel", trace[0]["retry_action"])
	require.NotContains(t, trace[0], "concurrency_cooldown")
}

func TestTraceChannelFailureAddsOverloadCategoryAndSwitchAction(t *testing.T) {
	ctx := newRelayRetryContext()
	err := types.NewOpenAIError(
		errors.New("rate limit exceeded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)

	traceChannelFailure(ctx, *types.NewChannelError(917, 1, "channel-917", false, "", false), err, false)

	trace, ok := common.GetContextKeyType[[]map[string]interface{}](ctx, constant.ContextKeyChannelFailureTrace)
	require.True(t, ok)
	require.Len(t, trace, 1)
	require.Equal(t, "overload_skip", trace[0]["error_category"])
	require.Equal(t, "switch_channel", trace[0]["retry_action"])
	require.NotContains(t, trace[0], "temporary_avoidance_reason")
}

func TestProcessChannelErrorSkipsTemporaryAvoidanceForUpstreamConcurrencyBusy(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(913)
	})

	err := types.NewOpenAIError(
		errors.New("Concurrency limit exceeded for user, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(913, 1, "channel-913", false, "", false), err, false)

	require.Nil(t, service.GetChannelFailureAvoidanceStatus(913))
}

func TestProcessChannelErrorSkipsTemporaryAvoidanceForBalanceInsufficient(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		service.ClearChannelFailureAvoidance(914)
	})

	err := types.NewOpenAIError(
		errors.New("insufficient balance"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	processChannelError(newRelayRetryContext(), *types.NewChannelError(914, 1, "channel-914", false, "", false), err, false)

	require.Nil(t, service.GetChannelFailureAvoidanceStatus(914))
}

func TestShouldRetrySwitchesChannelForSanitizedUpstreamBalanceInsufficient(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	serviceSeedRelayRetryChannel(t, db, 920, "default", "gpt-5.5", 0)
	serviceSeedRelayRetryChannel(t, db, 921, "default", "gpt-5.5", 0)

	ctx := newRelayRetryContext()
	ctx.Set("use_channel", []string{"920"})
	service.MarkChannelBalanceSkipped(ctx, 920)
	service.MarkChannelBalanceInsufficient(920)
	t.Cleanup(func() {
		service.ClearChannelBalanceInsufficient(920)
		service.ClearChannelBalanceInsufficient(921)
	})

	retry := 0
	extraRetries := 0
	param := &service.RetryParam{
		Ctx:          ctx,
		TokenGroup:   "default",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAI,
		Retry:        &retry,
		ExtraRetries: &extraRetries,
	}
	err := types.NewOpenAIError(
		errors.New("上游渠道暂不可用，请稍后重试"),
		types.ErrorCodeUpstreamUnavailable,
		http.StatusServiceUnavailable,
	)

	require.True(t, service.IsBalanceInsufficientError(err))
	require.True(t, shouldRetry(ctx, err, param, 0))
	require.Equal(t, "switch_channel", retryActionForAttempt(ctx, err, true))
	require.Equal(t, 1, param.GetExtraRetries())
}

func TestProcessChannelErrorUpdatesBalanceReasonWhenAlreadyAutoDisabled(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		service.ClearChannelConcurrencyForTest()
	})
	autoBan := 1
	channel := model.Channel{
		Id:      909,
		Name:    "already-disabled",
		Type:    constant.ChannelTypeOpenAI,
		Status:  common.ChannelStatusAutoDisabled,
		AutoBan: &autoBan,
	}
	channel.SetOtherInfo(map[string]interface{}{
		"status_reason": "temporary failure",
	})
	require.NoError(t, db.Create(&channel).Error)

	processChannelError(
		newRelayRetryContext(),
		*types.NewChannelError(channel.Id, channel.Type, channel.Name, false, "", true),
		types.NewOpenAIError(errors.New("insufficient balance"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests),
		true,
	)

	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.True(t, service.IsBalanceInsufficientPausedChannel(updated))
	require.Equal(t, service.ChannelStatusReasonBalanceInsufficient, service.ChannelStatusReason(updated))
}

func TestProcessChannelErrorMarksBalanceInsufficientSynchronously(t *testing.T) {
	db := serviceSetupRelayRetryDB(t)
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		service.ClearChannelConcurrencyForTest()
	})
	autoBan := 1
	channel := model.Channel{
		Id:      910,
		Name:    "balance-fast-skip",
		Type:    constant.ChannelTypeOpenAI,
		Status:  common.ChannelStatusEnabled,
		AutoBan: &autoBan,
	}
	require.NoError(t, db.Create(&channel).Error)

	processChannelError(
		newRelayRetryContext(),
		*types.NewChannelError(channel.Id, channel.Type, channel.Name, false, "", true),
		types.NewOpenAIError(errors.New("insufficient account balance"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden),
		false,
	)

	require.True(t, service.IsRuntimeBalanceInsufficientChannelID(channel.Id))
	updated, err := model.GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.True(t, service.IsBalanceInsufficientPausedChannel(updated))
}

func TestProcessChannelErrorRecordsConfigIsolationAfterTwoAuthFailures(t *testing.T) {
	t.Cleanup(func() {
		service.ClearChannelConfigIsolation(service.NewChannelConfigIsolationKey(918, "gpt-5.5", "default", constant.EndpointTypeOpenAI))
		service.ClearChannelFailureAvoidance(918)
	})

	ctx := newRelayRetryContext()
	ctx.Set("original_model", "gpt-5.5")
	ctx.Set("group", "default")
	err := types.NewOpenAIError(
		errors.New("invalid API key"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusUnauthorized,
	)
	key := service.NewChannelConfigIsolationKey(918, "gpt-5.5", "default", constant.EndpointTypeOpenAI)

	processChannelError(ctx, *types.NewChannelError(918, 1, "channel-918", false, "", true), err, false)
	status := service.GetChannelConfigIsolationStatus(key)
	require.NotNil(t, status)
	require.False(t, status.Active)
	require.Equal(t, 1, status.FailureCount)
	require.Nil(t, service.GetChannelFailureAvoidanceStatus(918))

	processChannelError(ctx, *types.NewChannelError(918, 1, "channel-918", false, "", true), err, false)
	status = service.GetChannelConfigIsolationStatus(key)
	require.NotNil(t, status)
	require.True(t, status.Active)
	require.Equal(t, 2, status.FailureCount)
	require.Equal(t, "auth_config_error", classifyRelayAttemptError(ctx, err))
	require.Nil(t, service.GetChannelFailureAvoidanceStatus(918))
}

func TestRelayChannelConfigSuccessClearsIsolation(t *testing.T) {
	t.Cleanup(func() {
		service.ClearChannelConfigIsolation(service.NewChannelConfigIsolationKey(919, "gpt-5.5", "default", constant.EndpointTypeOpenAI))
	})

	ctx := newRelayRetryContext()
	ctx.Set("original_model", "gpt-5.5")
	ctx.Set("group", "default")
	key := service.NewChannelConfigIsolationKey(919, "gpt-5.5", "default", constant.EndpointTypeOpenAI)
	service.RecordChannelConfigAuthError(key, "401")
	service.RecordChannelConfigAuthError(key, "403")
	require.True(t, service.IsChannelConfigIsolated(key))

	recordRelayChannelConfigSuccess(ctx, 919, nil, &service.RetryParam{
		TokenGroup:   "default",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAI,
	})

	require.Nil(t, service.GetChannelConfigIsolationStatus(key))
}
