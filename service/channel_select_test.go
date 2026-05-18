package service

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func initChannelSelectColumnNames(t *testing.T) {
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

func setupChannelSelectTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	initChannelSelectColumnNames(t)

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

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

func withChannelSelectMemoryCache(t *testing.T, enabled bool) {
	t.Helper()

	original := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = enabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = original
	})
}

func withAutoGroupsForTest(t *testing.T, groups []string) {
	t.Helper()

	original := setting.AutoGroups2JsonString()
	payload, err := common.Marshal(groups)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(string(payload)))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(original))
	})
}

func seedChannelSelectChannel(t *testing.T, db *gorm.DB, id int, group string, modelName string, priority int64, weight uint) {
	t.Helper()

	seedChannelSelectChannelWithOptions(t, db, id, group, modelName, priority, weight, constant.ChannelTypeOpenAI, "")
}

func seedChannelSelectChannelWithOptions(t *testing.T, db *gorm.DB, id int, group string, modelName string, priority int64, weight uint, channelType int, otherSettings string) {
	t.Helper()

	channel := &model.Channel{
		Id:            id,
		Type:          channelType,
		Name:          fmt.Sprintf("channel-%d", id),
		Key:           fmt.Sprintf("sk-%d", id),
		Status:        common.ChannelStatusEnabled,
		Group:         group,
		Models:        modelName,
		Weight:        &weight,
		Priority:      &priority,
		CreatedTime:   int64(id),
		OtherSettings: otherSettings,
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
}

func newRetryContext(usedChannels ...string) *gin.Context {
	ctx, _ := gin.CreateTestContext(nil)
	if len(usedChannels) > 0 {
		ctx.Set("use_channel", usedChannels)
	}
	return ctx
}

func TestCacheGetRandomSatisfiedChannelPrefersUnusedPeerChannel(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannel(t, db, 101, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 102, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 103, "default", "gpt-5.5", 5, 100)
	model.InitChannelCache()

	param := &RetryParam{
		Ctx:        newRetryContext("101"),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(1),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 102, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelSkipsFullConcurrencyChannel(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	seedChannelSelectChannel(t, db, 111, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 112, "default", "gpt-5.5", 10, 100)
	limitSetting := `{"max_concurrency":1}`
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 111).Update("setting", limitSetting).Error)
	model.InitChannelCache()

	lease, ok := TryAcquireChannelConcurrency(111, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, ok)
	defer lease.Release()

	param := &RetryParam{
		Ctx:        newRetryContext(),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 112, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelKeepsImageApiSeparateFromCodexTool(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannelWithOptions(t, db, 121, "default", "gpt-image-2", 10, 100, constant.ChannelTypeOpenAI, "")
	seedChannelSelectChannelWithOptions(t, db, 122, "default", "gpt-image-2", 10, 100, constant.ChannelTypeOpenAI, `{"codex_compatibility_mode":true,"codex_image_generation_tool_supported":true}`)
	model.InitChannelCache()

	param := &RetryParam{
		Ctx:          newRetryContext(),
		TokenGroup:   "default",
		ModelName:    "gpt-image-2",
		EndpointType: constant.EndpointTypeImageGeneration,
		Retry:        common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Contains(t, []int{121, 122}, channel.Id)

	param.RequiresCodexImageTool = true
	param.Ctx = newRetryContext()
	channel, group, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 122, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelAutoMovesToNextGroupAfterCurrentGroupExhausted(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	withAutoGroupsForTest(t, []string{"default", "vip"})

	seedChannelSelectChannel(t, db, 201, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 202, "vip", "gpt-5.5", 10, 100)
	model.InitChannelCache()

	ctx := newRetryContext("201")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(1),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, "vip", group)
	require.Equal(t, 202, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelFallsBackToUsedChannelForSingleGroup(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, false)

	seedChannelSelectChannel(t, db, 301, "default", "gpt-5.5", 10, 100)

	param := &RetryParam{
		Ctx:        newRetryContext("301"),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 301, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelAvoidsRecentlyFailedChannel(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, false)

	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	seedChannelSelectChannel(t, db, 351, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 352, "default", "gpt-5.5", 10, 100)
	RecordChannelFailureAvoidance(351, "do_request_failed")

	param := &RetryParam{
		Ctx:        newRetryContext(),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 352, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelFallsBackWhenAllChannelsAvoided(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, false)

	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	seedChannelSelectChannel(t, db, 361, "default", "gpt-5.5", 10, 100)
	RecordChannelFailureAvoidance(361, "do_request_failed")

	param := &RetryParam{
		Ctx:        newRetryContext(),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 361, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelTriesNextAutoGroupWhenCurrentAvoided(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, false)
	withAutoGroupsForTest(t, []string{"default", "vip"})

	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 45
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	seedChannelSelectChannel(t, db, 371, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 372, "vip", "gpt-5.5", 10, 100)
	RecordChannelFailureAvoidance(371, "do_request_failed")

	ctx := newRetryContext()
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "vip", group)
	require.NotNil(t, channel)
	require.Equal(t, 372, channel.Id)
}

func TestRecordChannelFailureAvoidanceExtendsDurationForRepeatedFailures(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 10
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	firstRecord := RecordChannelFailureAvoidance(381, "do_request_failed")
	first, ok := getChannelFailureAvoidanceForTest(381)
	require.True(t, ok)
	require.NotNil(t, firstRecord)
	require.Equal(t, 1, first.failureCount)
	require.False(t, firstRecord.ShouldPause)

	secondRecord := RecordChannelFailureAvoidance(381, "do_request_failed")
	second, ok := getChannelFailureAvoidanceForTest(381)
	require.True(t, ok)
	require.NotNil(t, secondRecord)
	require.Equal(t, 2, second.failureCount)
	require.Greater(t, second.until.Sub(time.Now()), 15*time.Second)
	require.Greater(t, second.until, first.until)
	require.False(t, secondRecord.ShouldPause)
}

func TestRecordChannelFailureAvoidanceEscalatesToPauseAfterRepeatedFailures(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 10
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	var record *ChannelFailureAvoidanceRecord
	for i := 0; i < channelFailureAvoidancePauseFailures; i++ {
		record = RecordChannelFailureAvoidance(382, "do_request_failed")
	}

	require.NotNil(t, record)
	require.True(t, record.ShouldPause)
	require.Equal(t, channelFailureAvoidancePauseFailures, record.FailureCount)
	require.InDelta(t, channelFailureAvoidancePauseDuration.Seconds(), record.Remaining.Seconds(), 1)
}

func TestRetryParamIncreaseRetryConsumesExtraRetriesFirst(t *testing.T) {
	param := &RetryParam{
		Retry:        common.GetPointer(0),
		ExtraRetries: common.GetPointer(2),
	}

	param.IncreaseRetry()
	require.Equal(t, 0, param.GetRetry())
	require.Equal(t, 1, param.GetExtraRetries())

	param.IncreaseRetry()
	require.Equal(t, 0, param.GetRetry())
	require.Equal(t, 0, param.GetExtraRetries())

	param.IncreaseRetry()
	require.Equal(t, 1, param.GetRetry())
	require.Equal(t, 0, param.GetExtraRetries())
}

func TestCacheGetRandomSatisfiedChannelForceSkipsCurrentAutoGroup(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	withAutoGroupsForTest(t, []string{"default", "vip"})

	seedChannelSelectChannel(t, db, 401, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 402, "vip", "gpt-5.5", 10, 100)
	model.InitChannelCache()

	ctx := newRetryContext("401")
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyForceNextAutoGroup, true)

	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "auto",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, "vip", group)
	require.Equal(t, 402, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelRequiresCodexImageToolSkipsHigherPriorityPlainChannel(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannelWithOptions(t, db, 431, "default", "gpt-5.5", 10, 100, constant.ChannelTypeOpenAI, `{"codex_compatibility_mode":true}`)
	seedChannelSelectChannelWithOptions(t, db, 432, "default", "gpt-5.5", 1, 100, constant.ChannelTypeOpenAI, `{"codex_compatibility_mode":true,"codex_image_generation_tool_supported":true}`)
	model.InitChannelCache()

	normalParam := &RetryParam{
		Ctx:          newRetryContext(),
		TokenGroup:   "default",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAIResponse,
		Retry:        common.GetPointer(0),
	}
	normalChannel, normalGroup, err := CacheGetRandomSatisfiedChannel(normalParam)
	require.NoError(t, err)
	require.Equal(t, "default", normalGroup)
	require.NotNil(t, normalChannel)
	require.Equal(t, 431, normalChannel.Id)

	imageToolParam := &RetryParam{
		Ctx:                    newRetryContext(),
		TokenGroup:             "default",
		ModelName:              "gpt-5.5",
		EndpointType:           constant.EndpointTypeOpenAIResponse,
		RequiresCodexImageTool: true,
		Retry:                  common.GetPointer(0),
	}
	imageToolChannel, imageToolGroup, err := CacheGetRandomSatisfiedChannel(imageToolParam)
	require.NoError(t, err)
	require.Equal(t, "default", imageToolGroup)
	require.NotNil(t, imageToolChannel)
	require.Equal(t, 432, imageToolChannel.Id)
}

func TestCacheGetRandomSatisfiedChannelSeesReEnabledChannelImmediately(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannel(t, db, 451, "default", "gpt-5.5", 10, 100)
	model.InitChannelCache()
	require.True(t, model.UpdateChannelStatus(451, "", common.ChannelStatusAutoDisabled, "temporary failure"))

	disabledParam := &RetryParam{
		Ctx:        newRetryContext(),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	disabledChannel, _, err := CacheGetRandomSatisfiedChannel(disabledParam)
	require.NoError(t, err)
	require.Nil(t, disabledChannel)

	require.True(t, model.UpdateChannelStatus(451, "", common.ChannelStatusEnabled, ""))
	enabledParam := &RetryParam{
		Ctx:        newRetryContext(),
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}
	enabledChannel, group, err := CacheGetRandomSatisfiedChannel(enabledParam)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, enabledChannel)
	require.Equal(t, 451, enabledChannel.Id)
}
