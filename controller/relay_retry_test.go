package controller

import (
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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
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

func TestShouldRetryRejectsGeneric429WhenNoRetryBudget(t *testing.T) {
	ctx := newRelayRetryContext()
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := &service.RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	err := types.NewOpenAIError(
		errors.New("rate limit exceeded"),
		types.ErrorCodeBadResponseStatusCode,
		429,
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
}

func TestProcessChannelErrorRecordsTemporaryAvoidanceForBadGateway(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
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
