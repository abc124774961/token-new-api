package service

import (
	"context"
	"encoding/json"
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
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	initBillingTestColumnNames()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	model.DB = db
	model.LOG_DB = db

	common.UsingSQLite = true
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true

	if err := db.AutoMigrate(
		&model.Task{},
		&model.User{},
		&model.Token{},
		&model.Log{},
		&model.Channel{},
		&model.TopUp{},
		&model.SubscriptionPlan{},
		&model.UserSubscription{},
		&model.SubscriptionPreConsumeRecord{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

func initBillingTestColumnNames() {
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
			if err := os.Setenv("SQL_DSN", originalSQLDSN); err != nil {
				panic("failed to restore SQL_DSN: " + err.Error())
			}
		} else if err := os.Unsetenv("SQL_DSN"); err != nil {
			panic("failed to unset SQL_DSN: " + err.Error())
		}
	}()

	common.IsMasterNode = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll("service_task_billing_init", "/", "_"))
	common.UsingSQLite = false
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	if err := os.Setenv("SQL_DSN", "local"); err != nil {
		panic("failed to set SQL_DSN: " + err.Error())
	}

	if err := model.InitDB(); err != nil {
		panic("failed to initialize model column names: " + err.Error())
	}
	if model.DB != nil {
		sqlDB, err := model.DB.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}
}

// ---------------------------------------------------------------------------
// Seed helpers
// ---------------------------------------------------------------------------

func truncate(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM tasks")
		model.DB.Exec("DELETE FROM users")
		model.DB.Exec("DELETE FROM tokens")
		model.DB.Exec("DELETE FROM logs")
		model.DB.Exec("DELETE FROM channels")
		model.DB.Exec("DELETE FROM top_ups")
		model.DB.Exec("DELETE FROM subscription_plans")
		model.DB.Exec("DELETE FROM user_subscriptions")
		model.DB.Exec("DELETE FROM subscription_pre_consume_records")
	})
}

func seedUser(t *testing.T, id int, quota int) {
	t.Helper()
	user := &model.User{Id: id, Username: "test_user", Quota: quota, Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
}

func seedToken(t *testing.T, id int, userId int, key string, remainQuota int) {
	t.Helper()
	token := &model.Token{
		Id:          id,
		UserId:      userId,
		Key:         key,
		Name:        "test_token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: remainQuota,
		UsedQuota:   0,
	}
	require.NoError(t, model.DB.Create(token).Error)
}

func seedSubscription(t *testing.T, id int, userId int, amountTotal int64, amountUsed int64) {
	t.Helper()
	planId := id
	plan := &model.SubscriptionPlan{
		Id:            planId,
		Title:         "test_plan",
		DurationUnit:  model.SubscriptionDurationMonth,
		DurationValue: 1,
		TotalAmount:   amountTotal,
		Enabled:       true,
	}
	require.NoError(t, model.DB.Create(plan).Error)
	sub := &model.UserSubscription{
		Id:          id,
		UserId:      userId,
		PlanId:      planId,
		AmountTotal: amountTotal,
		AmountUsed:  amountUsed,
		Status:      "active",
		StartTime:   time.Now().Unix(),
		EndTime:     time.Now().Add(30 * 24 * time.Hour).Unix(),
	}
	require.NoError(t, model.DB.Create(sub).Error)
}

func seedChannel(t *testing.T, id int) {
	t.Helper()
	ch := &model.Channel{Id: id, Name: "test_channel", Key: "sk-test", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(ch).Error)
}

func makeTask(userId, channelId, quota, tokenId int, billingSource string, subscriptionId int) *model.Task {
	return &model.Task{
		TaskID:    "task_" + time.Now().Format("150405.000"),
		UserId:    userId,
		ChannelId: channelId,
		Quota:     quota,
		Status:    model.TaskStatus(model.TaskStatusInProgress),
		Group:     "default",
		Data:      json.RawMessage(`{}`),
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
		Properties: model.Properties{
			OriginModelName: "test-model",
		},
		PrivateData: model.TaskPrivateData{
			BillingSource:     billingSource,
			SubscriptionId:    subscriptionId,
			SubscriptionQuota: subscriptionQuotaForTask(quota, billingSource),
			WalletQuota:       walletQuotaForTask(quota, billingSource),
			TokenId:           tokenId,
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      0.02,
				GroupRatio:      1.0,
				OriginModelName: "test-model",
			},
		},
	}
}

func subscriptionQuotaForTask(quota int, billingSource string) int {
	if billingSource == BillingSourceSubscription {
		return quota
	}
	return 0
}

func walletQuotaForTask(quota int, billingSource string) int {
	if billingSource == BillingSourceWallet {
		return quota
	}
	return 0
}

// ---------------------------------------------------------------------------
// Read-back helpers
// ---------------------------------------------------------------------------

func getUserQuota(t *testing.T, id int) int {
	t.Helper()
	var user model.User
	require.NoError(t, model.DB.Select("quota").Where("id = ?", id).First(&user).Error)
	return user.Quota
}

func getTokenRemainQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("remain_quota").Where("id = ?", id).First(&token).Error)
	return token.RemainQuota
}

func getTokenUsedQuota(t *testing.T, id int) int {
	t.Helper()
	var token model.Token
	require.NoError(t, model.DB.Select("used_quota").Where("id = ?", id).First(&token).Error)
	return token.UsedQuota
}

func getSubscriptionUsed(t *testing.T, id int) int64 {
	t.Helper()
	var sub model.UserSubscription
	require.NoError(t, model.DB.Select("amount_used").Where("id = ?", id).First(&sub).Error)
	return sub.AmountUsed
}

func getLastLog(t *testing.T) *model.Log {
	t.Helper()
	var log model.Log
	err := model.LOG_DB.Order("id desc").First(&log).Error
	if err != nil {
		return nil
	}
	return &log
}

func countLogs(t *testing.T) int64 {
	t.Helper()
	var count int64
	model.LOG_DB.Model(&model.Log{}).Count(&count)
	return count
}

func makeBillingTestContext(t *testing.T, tokenKey string, tokenRemain int) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	reqID := "req_" + time.Now().Format("150405.000000000")
	ctx.Set(common.RequestIdKey, reqID)
	ctx.Set("token_quota", tokenRemain)
	ctx.Set("username", "test_user")
	common.SetContextKey(ctx, constant.ContextKeyTokenKey, tokenKey)
	common.SetContextKey(ctx, constant.ContextKeyTokenId, 1)
	common.SetContextKey(ctx, constant.ContextKeyTokenUnlimited, false)
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{
		BillingPreference: "subscription_first",
	})
	return ctx
}

func makeBillingRelayInfo(ctx *gin.Context, userID int, tokenID int, tokenKey string) *relaycommon.RelayInfo {
	reqID := ctx.GetString(common.RequestIdKey)
	if reqID == "" {
		reqID = "req_" + time.Now().Format("150405.000000000")
	}
	return &relaycommon.RelayInfo{
		RequestId:       reqID,
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        tokenKey,
		OriginModelName: "test-model",
		IsPlayground:    true,
		UserSetting: dto.UserSetting{
			BillingPreference: "subscription_first",
		},
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
		StartTime: time.Now(),
	}
}

func TestPreConsumeBilling_WalletDeductsTokenAndUserQuota(t *testing.T) {
	truncate(t)

	const userID, tokenID = 201, 1
	const initUserQuota = 10000
	const initTokenRemain = 8000
	const preConsumed = 3000
	const tokenKey = "sk-token-preconsume"

	seedUser(t, userID, initUserQuota)
	seedToken(t, tokenID, userID, tokenKey, initTokenRemain)

	ctx := makeBillingTestContext(t, tokenKey, initTokenRemain)
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{
		BillingPreference: "wallet_only",
	})
	relayInfo := makeBillingRelayInfo(ctx, userID, tokenID, tokenKey)
	relayInfo.IsPlayground = false
	relayInfo.ForcePreConsume = true
	relayInfo.UserSetting = dto.UserSetting{BillingPreference: "wallet_only"}

	apiErr := PreConsumeBilling(ctx, preConsumed, relayInfo)
	require.Nil(t, apiErr)
	require.NotNil(t, relayInfo.Billing)

	assert.Equal(t, BillingSourceWallet, relayInfo.BillingSource)
	assert.Equal(t, preConsumed, relayInfo.FinalPreConsumedQuota)
	assert.Equal(t, initUserQuota-preConsumed, getUserQuota(t, userID))
	assert.Equal(t, initTokenRemain-preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, getTokenUsedQuota(t, tokenID))
}

func TestBillingSession_RefundRestoresWalletAndTokenPreConsume(t *testing.T) {
	truncate(t)

	const userID, tokenID = 202, 1
	const initUserQuota = 12000
	const initTokenRemain = 9000
	const preConsumed = 2500
	const tokenKey = "sk-token-refund"

	seedUser(t, userID, initUserQuota)
	seedToken(t, tokenID, userID, tokenKey, initTokenRemain)

	ctx := makeBillingTestContext(t, tokenKey, initTokenRemain)
	common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{
		BillingPreference: "wallet_only",
	})
	relayInfo := makeBillingRelayInfo(ctx, userID, tokenID, tokenKey)
	relayInfo.IsPlayground = false
	relayInfo.ForcePreConsume = true
	relayInfo.UserSetting = dto.UserSetting{BillingPreference: "wallet_only"}

	apiErr := PreConsumeBilling(ctx, preConsumed, relayInfo)
	require.Nil(t, apiErr)
	require.NotNil(t, relayInfo.Billing)
	require.Equal(t, initUserQuota-preConsumed, getUserQuota(t, userID))
	require.Equal(t, initTokenRemain-preConsumed, getTokenRemainQuota(t, tokenID))
	require.Equal(t, preConsumed, getTokenUsedQuota(t, tokenID))

	relayInfo.Billing.Refund(ctx)

	require.Eventually(t, func() bool {
		return getUserQuota(t, userID) == initUserQuota &&
			getTokenRemainQuota(t, tokenID) == initTokenRemain &&
			getTokenUsedQuota(t, tokenID) == 0
	}, time.Second, 10*time.Millisecond)
}

// ===========================================================================
// RefundTaskQuota tests
// ===========================================================================

func TestRefundTaskQuota_Wallet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 1, 1, 1
	const initQuota, preConsumed = 10000, 3000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-test-key", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "task failed: upstream error")

	// User quota should increase by preConsumed
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Token remain_quota should increase, used_quota should decrease
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, -preConsumed, getTokenUsedQuota(t, tokenID))

	// A refund log should be created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed, log.Quota)
	assert.Equal(t, "test-model", log.ModelName)
}

func TestRefundTaskQuota_Subscription(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 2, 2, 2, 1
	const preConsumed = 2000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-key", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RefundTaskQuota(ctx, task, "subscription task failed")

	// Subscription used should decrease by preConsumed
	assert.Equal(t, subUsed-int64(preConsumed), getSubscriptionUsed(t, subID))

	// Token should also be refunded
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRefundTaskQuota_ZeroQuota(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 3
	seedUser(t, userID, 5000)

	task := makeTask(userID, 0, 0, 0, BillingSourceWallet, 0)

	RefundTaskQuota(ctx, task, "zero quota task")

	// No change to user quota
	assert.Equal(t, 5000, getUserQuota(t, userID))

	// No log created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRefundTaskQuota_NoToken(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 4, 4
	const initQuota, preConsumed = 10000, 1500

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0) // TokenId=0

	RefundTaskQuota(ctx, task, "no token task failed")

	// User quota refunded
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))

	// Log created
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

// ===========================================================================
// RecalculateTaskQuota tests
// ===========================================================================

func TestRecalculate_PositiveDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 10, 10, 10
	const initQuota, preConsumed = 10000, 2000
	const actualQuota = 3000 // under-charged by 1000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-pos", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should decrease by the delta (1000 additional charge)
	assert.Equal(t, initQuota-(actualQuota-preConsumed), getUserQuota(t, userID))

	// Token should also be charged the delta
	assert.Equal(t, tokenRemain-(actualQuota-preConsumed), getTokenRemainQuota(t, tokenID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Consume (additional charge)
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
	assert.Equal(t, actualQuota-preConsumed, log.Quota)
}

func TestRecalculate_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 11, 11, 11
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged by 2000
	const tokenRemain = 5000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-recalc-neg", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, actualQuota, "adaptor adjustment")

	// User quota should increase by abs(delta) = 2000 (refund overpayment)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))

	// Token should be refunded the difference
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	// task.Quota updated
	assert.Equal(t, actualQuota, task.Quota)

	// Log type should be Refund
	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
	assert.Equal(t, preConsumed-actualQuota, log.Quota)
}

func TestRecalculate_ZeroDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 12
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, preConsumed, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, preConsumed, "exact match")

	// No change to user quota
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No log created (delta is zero)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_ActualQuotaZero(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID = 13
	const initQuota = 10000

	seedUser(t, userID, initQuota)

	task := makeTask(userID, 0, 5000, 0, BillingSourceWallet, 0)

	RecalculateTaskQuota(ctx, task, 0, "zero actual")

	// No change (early return)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(0), countLogs(t))
}

func TestRecalculate_Subscription_NegativeDelta(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 14, 14, 14, 2
	const preConsumed = 5000
	const actualQuota = 2000 // over-charged by 3000
	const subTotal, subUsed int64 = 100000, 50000
	const tokenRemain = 8000

	seedUser(t, userID, 0)
	seedToken(t, tokenID, userID, "sk-sub-recalc", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscription, subID)

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription over-charge")

	// Subscription used should decrease by delta (refund 3000)
	assert.Equal(t, subUsed-int64(preConsumed-actualQuota), getSubscriptionUsed(t, subID))

	// Token refunded
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	assert.Equal(t, actualQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRefundTaskQuota_SubscriptionWallet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 15, 15, 15, 3
	const initQuota = 10000
	const preConsumed = 3000
	const subscriptionConsumed = 2000
	const walletConsumed = 1000
	const subTotal, subUsed int64 = 5000, 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-sub-wallet-task", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscriptionWallet, subID)
	task.PrivateData.SubscriptionQuota = subscriptionConsumed
	task.PrivateData.WalletQuota = walletConsumed

	RefundTaskQuota(ctx, task, "subscription wallet task failed")

	assert.Equal(t, initQuota+walletConsumed, getUserQuota(t, userID))
	assert.Equal(t, subUsed-int64(subscriptionConsumed), getSubscriptionUsed(t, subID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 0, task.PrivateData.SubscriptionQuota)
	assert.Equal(t, 0, task.PrivateData.WalletQuota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestRecalculate_SubscriptionWallet_PositiveDeltaUsesSubscriptionThenWallet(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 16, 16, 16, 4
	const initQuota = 10000
	const preConsumed = 3000
	const actualQuota = 4500
	const subscriptionConsumed = 2500
	const walletConsumed = 500
	const subTotal, subUsed int64 = 3500, 2500
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-sub-wallet-recalc", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscriptionWallet, subID)
	task.PrivateData.SubscriptionQuota = subscriptionConsumed
	task.PrivateData.WalletQuota = walletConsumed

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription wallet adjustment")

	assert.Equal(t, int64(subTotal), getSubscriptionUsed(t, subID))
	assert.Equal(t, initQuota-500, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain-(actualQuota-preConsumed), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, actualQuota, task.Quota)
	assert.Equal(t, 3500, task.PrivateData.SubscriptionQuota)
	assert.Equal(t, 1000, task.PrivateData.WalletQuota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeConsume, log.Type)
}

func TestRecalculate_SubscriptionWallet_NegativeDeltaRefundsWalletFirst(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID, subID = 17, 17, 17, 5
	const initQuota = 10000
	const preConsumed = 3000
	const actualQuota = 1200
	const subscriptionConsumed = 2000
	const walletConsumed = 1000
	const subTotal, subUsed int64 = 5000, 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-sub-wallet-refund", tokenRemain)
	seedChannel(t, channelID)
	seedSubscription(t, subID, userID, subTotal, subUsed)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceSubscriptionWallet, subID)
	task.PrivateData.SubscriptionQuota = subscriptionConsumed
	task.PrivateData.WalletQuota = walletConsumed

	RecalculateTaskQuota(ctx, task, actualQuota, "subscription wallet refund")

	assert.Equal(t, initQuota+walletConsumed, getUserQuota(t, userID))
	assert.Equal(t, subUsed-int64(800), getSubscriptionUsed(t, subID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, actualQuota, task.Quota)
	assert.Equal(t, 1200, task.PrivateData.SubscriptionQuota)
	assert.Equal(t, 0, task.PrivateData.WalletQuota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

// ===========================================================================
// CAS + Billing integration tests
// Simulates the flow in updateVideoSingleTask (service/task_polling.go)
// ===========================================================================

// simulatePollBilling reproduces the CAS + billing logic from updateVideoSingleTask.
// It takes a persisted task (already in DB), applies the new status, and performs
// the conditional update + billing exactly as the polling loop does.
func simulatePollBilling(ctx context.Context, task *model.Task, newStatus model.TaskStatus, actualQuota int) {
	snap := task.Snapshot()

	shouldRefund := false
	shouldSettle := false
	quota := task.Quota

	task.Status = newStatus
	switch string(newStatus) {
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		task.FinishTime = 9999
		shouldSettle = true
	case model.TaskStatusFailure:
		task.Progress = "100%"
		task.FinishTime = 9999
		task.FailReason = "upstream error"
		if quota != 0 {
			shouldRefund = true
		}
	default:
		task.Progress = "50%"
	}

	isDone := task.Status == model.TaskStatus(model.TaskStatusSuccess) || task.Status == model.TaskStatus(model.TaskStatusFailure)
	if isDone && snap.Status != task.Status {
		won, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			shouldRefund = false
			shouldSettle = false
		} else if !won {
			shouldRefund = false
			shouldSettle = false
		}
	} else if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	if shouldSettle && actualQuota > 0 {
		RecalculateTaskQuota(ctx, task, actualQuota, "test settle")
	}
	if shouldRefund {
		RefundTaskQuota(ctx, task, task.FailReason)
	}
}

func TestCASGuardedRefund_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 20, 20, 20
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS wins: task in DB should now be FAILURE
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusFailure, reloaded.Status)

	// Refund should have happened
	assert.Equal(t, initQuota+preConsumed, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+preConsumed, getTokenRemainQuota(t, tokenID))

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

func TestCASGuardedRefund_Lose(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 21, 21, 21
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 6000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-refund-lose", tokenRemain)
	seedChannel(t, channelID)

	// Create task with IN_PROGRESS in DB
	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate another process already transitioning to FAILURE
	model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Update("status", model.TaskStatusFailure)

	// Our process still has the old in-memory state (IN_PROGRESS) and tries to transition
	// task.Status is still IN_PROGRESS in the snapshot
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusFailure), 0)

	// CAS lost: user quota should NOT change (no double refund)
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))

	// No billing log should be created
	assert.Equal(t, int64(0), countLogs(t))
}

func TestCASGuardedSettle_Win(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 22, 22, 22
	const initQuota, preConsumed = 10000, 5000
	const actualQuota = 3000 // over-charged, should get partial refund
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-cas-settle-win", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	require.NoError(t, model.DB.Create(task).Error)

	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusSuccess), actualQuota)

	// CAS wins: task should be SUCCESS
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)

	// Settlement should refund the over-charge (5000 - 3000 = 2000 back to user)
	assert.Equal(t, initQuota+(preConsumed-actualQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-actualQuota), getTokenRemainQuota(t, tokenID))

	// task.Quota should be updated to actualQuota
	assert.Equal(t, actualQuota, task.Quota)
}

func TestNonTerminalUpdate_NoBilling(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, channelID = 23, 23
	const initQuota, preConsumed = 10000, 3000

	seedUser(t, userID, initQuota)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, 0, BillingSourceWallet, 0)
	task.Status = model.TaskStatus(model.TaskStatusInProgress)
	task.Progress = "20%"
	require.NoError(t, model.DB.Create(task).Error)

	// Simulate a non-terminal poll update (still IN_PROGRESS, progress changed)
	simulatePollBilling(ctx, task, model.TaskStatus(model.TaskStatusInProgress), 0)

	// User quota should NOT change
	assert.Equal(t, initQuota, getUserQuota(t, userID))

	// No billing log
	assert.Equal(t, int64(0), countLogs(t))

	// Task progress should be updated in DB
	var reloaded model.Task
	require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
	assert.Equal(t, "50%", reloaded.Progress)
}

// ===========================================================================
// Mock adaptor for settleTaskBillingOnComplete tests
// ===========================================================================

type mockAdaptor struct {
	adjustReturn int
}

func (m *mockAdaptor) Init(_ *relaycommon.RelayInfo) {}
func (m *mockAdaptor) FetchTask(string, string, map[string]any, string) (*http.Response, error) {
	return nil, nil
}
func (m *mockAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) { return nil, nil }
func (m *mockAdaptor) AdjustBillingOnComplete(_ *model.Task, _ *relaycommon.TaskInfo) int {
	return m.adjustReturn
}

// ===========================================================================
// PerCallBilling tests — settleTaskBillingOnComplete
// ===========================================================================

func TestSettle_PerCallBilling_SkipsAdaptorAdjust(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 30, 30, 30
	const initQuota, preConsumed = 10000, 5000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-adaptor", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 2000}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no adjustment despite adaptor returning 2000
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_PerCallBilling_SkipsTotalTokens(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 31, 31, 31
	const initQuota, preConsumed = 10000, 4000
	const tokenRemain = 7000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-percall-tokens", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PerCallBilling = true

	adaptor := &mockAdaptor{adjustReturn: 0}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, TotalTokens: 9999}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Per-call: no recalculation by tokens
	assert.Equal(t, initQuota, getUserQuota(t, userID))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, preConsumed, task.Quota)
	assert.Equal(t, int64(0), countLogs(t))
}

func TestSettle_NonPerCall_AdaptorAdjustWorks(t *testing.T) {
	truncate(t)
	ctx := context.Background()

	const userID, tokenID, channelID = 32, 32, 32
	const initQuota, preConsumed = 10000, 5000
	const adaptorQuota = 3000
	const tokenRemain = 8000

	seedUser(t, userID, initQuota)
	seedToken(t, tokenID, userID, "sk-nonpercall-adj", tokenRemain)
	seedChannel(t, channelID)

	task := makeTask(userID, channelID, preConsumed, tokenID, BillingSourceWallet, 0)
	// PerCallBilling defaults to false

	adaptor := &mockAdaptor{adjustReturn: adaptorQuota}
	taskResult := &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}

	settleTaskBillingOnComplete(ctx, adaptor, task, taskResult)

	// Non-per-call: adaptor adjustment applies (refund 2000)
	assert.Equal(t, initQuota+(preConsumed-adaptorQuota), getUserQuota(t, userID))
	assert.Equal(t, tokenRemain+(preConsumed-adaptorQuota), getTokenRemainQuota(t, tokenID))
	assert.Equal(t, adaptorQuota, task.Quota)

	log := getLastLog(t)
	require.NotNil(t, log)
	assert.Equal(t, model.LogTypeRefund, log.Type)
}

// ===========================================================================
// BillingSession subscription-first tests
// ===========================================================================

func TestBillingSession_SubscriptionFirstUsesSubscriptionBeforeWallet(t *testing.T) {
	truncate(t)

	const userID, tokenID = 101, 1
	const initWalletQuota = 10000
	const tokenRemain = 20000
	const preConsumed = 3000
	const actualQuota = 2500

	seedUser(t, userID, initWalletQuota)
	seedToken(t, tokenID, userID, "sk-sub-first", tokenRemain)
	seedSubscription(t, 1, userID, 5000, 0)

	ctx := makeBillingTestContext(t, "sk-sub-first", tokenRemain)
	relayInfo := makeBillingRelayInfo(ctx, userID, tokenID, "sk-sub-first")

	apiErr := PreConsumeBilling(ctx, preConsumed, relayInfo)
	require.Nil(t, apiErr)
	require.NotNil(t, relayInfo.Billing)
	require.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
	require.Equal(t, int64(preConsumed), relayInfo.SubscriptionPreConsumed)
	require.Equal(t, 0, relayInfo.WalletConsumed)

	require.NoError(t, SettleBilling(ctx, relayInfo, actualQuota))

	assert.Equal(t, initWalletQuota, getUserQuota(t, userID))
	assert.Equal(t, int64(actualQuota), getSubscriptionUsed(t, 1))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, int64(actualQuota), relayInfo.SubscriptionAmountUsedAfterPreConsume+relayInfo.SubscriptionPostDelta)

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 1, -1, -1)
	assert.Equal(t, BillingSourceSubscription, other["billing_source"])
	assert.EqualValues(t, 0, other["wallet_quota_deducted"])
	assert.EqualValues(t, actualQuota, other["subscription_consumed"])
}

func TestBillingSession_SubscriptionFirstPartiallyFallsBackToWallet(t *testing.T) {
	truncate(t)

	const userID, tokenID = 102, 1
	const initWalletQuota = 10000
	const tokenRemain = 20000
	const preConsumed = 3000
	const actualQuota = 4200

	seedUser(t, userID, initWalletQuota)
	seedToken(t, tokenID, userID, "sk-sub-wallet", tokenRemain)
	seedSubscription(t, 1, userID, 2500, 0)

	ctx := makeBillingTestContext(t, "sk-sub-wallet", tokenRemain)
	relayInfo := makeBillingRelayInfo(ctx, userID, tokenID, "sk-sub-wallet")

	apiErr := PreConsumeBilling(ctx, preConsumed, relayInfo)
	require.Nil(t, apiErr)
	require.NotNil(t, relayInfo.Billing)
	require.Equal(t, BillingSourceSubscriptionWallet, relayInfo.BillingSource)
	require.Equal(t, int64(2500), relayInfo.SubscriptionPreConsumed)
	require.Equal(t, 500, relayInfo.WalletConsumed)

	require.NoError(t, SettleBilling(ctx, relayInfo, actualQuota))

	assert.Equal(t, initWalletQuota-1700, getUserQuota(t, userID))
	assert.Equal(t, int64(2500), getSubscriptionUsed(t, 1))
	assert.Equal(t, tokenRemain, getTokenRemainQuota(t, tokenID))
	assert.Equal(t, 1700, relayInfo.WalletConsumed)

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 1, -1, -1)
	assert.Equal(t, BillingSourceSubscriptionWallet, other["billing_source"])
	assert.EqualValues(t, 1700, other["wallet_quota_deducted"])
	assert.EqualValues(t, 2500, other["subscription_consumed"])
}

func TestBillingSession_SubscriptionFirstPostSettleUsesRemainingSubscriptionThenWallet(t *testing.T) {
	truncate(t)

	const userID, tokenID = 103, 1
	const initWalletQuota = 10000
	const tokenRemain = 20000
	const preConsumed = 2000
	const actualQuota = 3500

	seedUser(t, userID, initWalletQuota)
	seedToken(t, tokenID, userID, "sk-sub-post-wallet", tokenRemain)
	seedSubscription(t, 1, userID, 3000, 0)

	ctx := makeBillingTestContext(t, "sk-sub-post-wallet", tokenRemain)
	relayInfo := makeBillingRelayInfo(ctx, userID, tokenID, "sk-sub-post-wallet")

	apiErr := PreConsumeBilling(ctx, preConsumed, relayInfo)
	require.Nil(t, apiErr)
	require.NotNil(t, relayInfo.Billing)
	require.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
	require.Equal(t, int64(preConsumed), relayInfo.SubscriptionPreConsumed)
	require.Equal(t, 0, relayInfo.WalletConsumed)

	require.NoError(t, SettleBilling(ctx, relayInfo, actualQuota))

	assert.Equal(t, initWalletQuota-500, getUserQuota(t, userID))
	assert.Equal(t, int64(3000), getSubscriptionUsed(t, 1))
	assert.Equal(t, 500, relayInfo.WalletConsumed)

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 1, -1, -1)
	assert.Equal(t, BillingSourceSubscriptionWallet, other["billing_source"])
	assert.EqualValues(t, 500, other["wallet_quota_deducted"])
	assert.EqualValues(t, 3000, other["subscription_consumed"])
}

func TestBillingSession_SubscriptionFirstPartialChoosesFullCoverSubscription(t *testing.T) {
	truncate(t)

	const userID, tokenID = 104, 1
	const initWalletQuota = 10000
	const tokenRemain = 20000
	const preConsumed = 3000

	seedUser(t, userID, initWalletQuota)
	seedToken(t, tokenID, userID, "sk-sub-full-cover", tokenRemain)
	seedSubscription(t, 1, userID, 1000, 0)
	seedSubscription(t, 2, userID, 5000, 0)

	ctx := makeBillingTestContext(t, "sk-sub-full-cover", tokenRemain)
	relayInfo := makeBillingRelayInfo(ctx, userID, tokenID, "sk-sub-full-cover")

	apiErr := PreConsumeBilling(ctx, preConsumed, relayInfo)
	require.Nil(t, apiErr)
	require.NotNil(t, relayInfo.Billing)
	require.Equal(t, BillingSourceSubscription, relayInfo.BillingSource)
	require.Equal(t, 0, relayInfo.WalletConsumed)
	require.Equal(t, int64(0), getSubscriptionUsed(t, 1))
	require.Equal(t, int64(preConsumed), getSubscriptionUsed(t, 2))
	assert.Equal(t, initWalletQuota, getUserQuota(t, userID))
}

func TestBillingSession_SubscriptionFirstPartialChoosesLargestAvailableSubscription(t *testing.T) {
	truncate(t)

	const userID, tokenID = 105, 1
	const initWalletQuota = 10000
	const tokenRemain = 20000
	const preConsumed = 4000

	seedUser(t, userID, initWalletQuota)
	seedToken(t, tokenID, userID, "sk-sub-largest", tokenRemain)
	seedSubscription(t, 1, userID, 1000, 0)
	seedSubscription(t, 2, userID, 2500, 0)

	ctx := makeBillingTestContext(t, "sk-sub-largest", tokenRemain)
	relayInfo := makeBillingRelayInfo(ctx, userID, tokenID, "sk-sub-largest")

	apiErr := PreConsumeBilling(ctx, preConsumed, relayInfo)
	require.Nil(t, apiErr)
	require.NotNil(t, relayInfo.Billing)
	require.Equal(t, BillingSourceSubscriptionWallet, relayInfo.BillingSource)
	require.Equal(t, int64(2500), relayInfo.SubscriptionPreConsumed)
	require.Equal(t, 1500, relayInfo.WalletConsumed)
	require.Equal(t, int64(0), getSubscriptionUsed(t, 1))
	require.Equal(t, int64(2500), getSubscriptionUsed(t, 2))
	assert.Equal(t, initWalletQuota-1500, getUserQuota(t, userID))
}
