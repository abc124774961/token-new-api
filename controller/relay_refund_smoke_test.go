package controller

import (
	"bytes"
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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRelayRefundSmokeDB(t *testing.T) *gorm.DB {
	t.Helper()
	initRelayRefundSmokeColumnNames(t)

	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	oldRedisEnabled := common.RedisEnabled
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldBatchUpdateEnabled := common.BatchUpdateEnabled
	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldRetryTimes := common.RetryTimes
	oldCountToken := constant.CountToken
	oldPreConsumedQuota := common.PreConsumedQuota
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldFreeModelPreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
	globalSettings := model_setting.GetGlobalSettings()
	oldPassThroughRequestEnabled := globalSettings.PassThroughRequestEnabled

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	common.RetryTimes = 0
	constant.CountToken = false
	common.PreConsumedQuota = 500
	operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = true
	globalSettings.PassThroughRequestEnabled = false
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"o3-pro":1}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	service.ClearChannelConcurrencyForTest()
	service.ClearChannelFailureAvoidance(301)
	service.InitHttpClient()
	resetRelayQueueManager()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.Token{},
		&model.Channel{},
		&model.Ability{},
		&model.Log{},
		&model.ChannelFailureEvent{},
	))

	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		common.RedisEnabled = oldRedisEnabled
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.BatchUpdateEnabled = oldBatchUpdateEnabled
		common.LogConsumeEnabled = oldLogConsumeEnabled
		common.RetryTimes = oldRetryTimes
		constant.CountToken = oldCountToken
		common.PreConsumedQuota = oldPreConsumedQuota
		operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFreeModelPreConsume
		globalSettings.PassThroughRequestEnabled = oldPassThroughRequestEnabled
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		service.ClearChannelConcurrencyForTest()
		service.ClearChannelFailureAvoidance(301)
		resetRelayQueueManager()
		_ = sqlDB.Close()
	})

	return db
}

func initRelayRefundSmokeColumnNames(t *testing.T) {
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

func seedRelayRefundSmokeData(t *testing.T, db *gorm.DB, baseURL string) {
	t.Helper()
	require.NoError(t, db.Create(&model.User{
		Id:       101,
		Username: "relay_refund_user",
		Role:     common.RoleAdminUser,
		Quota:    1_000_000,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}).Error)
	require.NoError(t, db.Create(&model.Token{
		Id:          201,
		UserId:      101,
		Key:         "token-relay-refund",
		Name:        "relay-refund-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 1_000_000,
		Group:       "default",
	}).Error)
	otherSettings, err := common.Marshal(dto.ChannelOtherSettings{WireAPI: "responses"})
	require.NoError(t, err)
	priority := int64(10)
	weight := uint(100)
	channel := &model.Channel{
		Id:            301,
		Type:          constant.ChannelTypeOpenAI,
		Name:          "relay-refund-openai",
		Key:           "sk-relay-refund",
		Status:        common.ChannelStatusEnabled,
		BaseURL:       &baseURL,
		Models:        "o3-pro",
		Group:         "default",
		Priority:      &priority,
		Weight:        &weight,
		OtherSettings: string(otherSettings),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
}

func newRelayRefundSmokeContext(baseURL string) (*gin.Context, *httptest.ResponseRecorder) {
	requestBody := []byte(`{"model":"o3-pro","input":[{"role":"user","content":"ping"}],"max_output_tokens":1}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(requestBody))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.RequestIdKey, "req-relay-refund")
	ctx.Set("id", 101)
	ctx.Set("username", "relay_refund_user")
	ctx.Set("token_name", "relay-refund-token")
	ctx.Set("token_quota", 1_000_000)
	ctx.Set("specific_channel_id", "301")
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now())
	common.SetContextKey(ctx, constant.ContextKeyUserId, 101)
	common.SetContextKey(ctx, constant.ContextKeyUserName, "relay_refund_user")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyUserQuota, 1_000_000)
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 201)
	common.SetContextKey(ctx, constant.ContextKeyTokenKey, "token-relay-refund")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenUnlimited, false)
	common.SetContextKey(ctx, constant.ContextKeyTokenSpecificChannelId, "301")
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "o3-pro")
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 301)
	common.SetContextKey(ctx, constant.ContextKeyChannelName, "relay-refund-openai")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "sk-relay-refund")
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, baseURL)
	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	common.SetContextKey(ctx, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{WireAPI: "responses"})
	return ctx, recorder
}

func TestRelayResponsesFailureRefundsPreConsumeSmoke(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/responses", r.URL.Path)
		require.Equal(t, "Bearer sk-relay-refund", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"quota exceeded","type":"rate_limit_error","code":"rate_limit_exceeded"}}`))
	}))
	t.Cleanup(upstream.Close)

	db := setupRelayRefundSmokeDB(t)
	seedRelayRefundSmokeData(t, db, upstream.URL)
	ctx, recorder := newRelayRefundSmokeContext(upstream.URL)

	Relay(ctx, types.RelayFormatOpenAIResponses)

	require.Equal(t, http.StatusTooManyRequests, recorder.Code)
	require.Contains(t, recorder.Body.String(), "quota exceeded")
	require.Eventually(t, func() bool {
		var user model.User
		if err := db.Select("quota", "used_quota", "request_count").Where("id = ?", 101).First(&user).Error; err != nil {
			return false
		}
		var token model.Token
		if err := db.Select("remain_quota", "used_quota").Where("id = ?", 201).First(&token).Error; err != nil {
			return false
		}
		return user.Quota == 1_000_000 &&
			user.UsedQuota == 0 &&
			user.RequestCount == 0 &&
			token.RemainQuota == 1_000_000 &&
			token.UsedQuota == 0
	}, time.Second, 10*time.Millisecond)

	var channel model.Channel
	require.NoError(t, db.Select("used_quota").Where("id = ?", 301).First(&channel).Error)
	require.Equal(t, int64(0), channel.UsedQuota)

	var consumeLogCount int64
	require.NoError(t, db.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&consumeLogCount).Error)
	require.Equal(t, int64(0), consumeLogCount)
}
