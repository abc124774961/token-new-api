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
	"github.com/QuantumNous/new-api/pkg/channelcapability"
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
	originalDB := model.DB
	originalLogDB := model.LOG_DB

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
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelFailureEvent{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
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

func TestCacheGetRandomSatisfiedChannelSkipsRequestSelectionMarkedChannel(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannel(t, db, 121, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 122, "default", "gpt-5.5", 10, 100)
	model.InitChannelCache()

	ctx := newRetryContext("121")
	MarkChannelSelectionSkipped(ctx, 121)
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Contains(t, []int{121, 122}, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelDoesNotSkipFullConcurrencyChannel(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	seedChannelSelectChannel(t, db, 111, "default", "gpt-5.5", 10, 100)
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
	require.Equal(t, 111, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelSkipsRequestBalanceMarkedChannel(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	seedChannelSelectChannel(t, db, 115, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 116, "default", "gpt-5.5", 10, 100)
	model.InitChannelCache()

	ctx := newRetryContext()
	MarkChannelBalanceSkipped(ctx, 115)
	param := &RetryParam{
		Ctx:        ctx,
		TokenGroup: "default",
		ModelName:  "gpt-5.5",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 116, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelSkipsRuntimeBalanceInsufficientChannel(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	seedChannelSelectChannel(t, db, 117, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 118, "default", "gpt-5.5", 10, 100)
	model.InitChannelCache()

	MarkChannelBalanceInsufficient(117)
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
	require.Equal(t, 118, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelSkipsSingleCodexPrettyJSONAuthError(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	weight := uint(100)
	priority := int64(10)
	blocked := &model.Channel{
		Id:          119,
		Type:        constant.ChannelTypeCodex,
		Name:        "blocked-codex",
		Key:         "{\n  \"access_token\": \"auth\",\n  \"account_id\": \"auth\"\n}",
		Status:      common.ChannelStatusEnabled,
		Group:       "default",
		Models:      "gpt-5.5",
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: 119,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CapabilityClassification: channelcapability.ClassificationAuthError,
				},
			},
		},
	}
	healthy := &model.Channel{
		Id:          120,
		Type:        constant.ChannelTypeCodex,
		Name:        "healthy-codex",
		Key:         `{"access_token":"ok","account_id":"ok"}`,
		Status:      common.ChannelStatusEnabled,
		Group:       "default",
		Models:      "gpt-5.5",
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: 120,
	}
	require.NoError(t, db.Create(blocked).Error)
	require.NoError(t, blocked.AddAbilities(nil))
	require.NoError(t, db.Create(healthy).Error)
	require.NoError(t, healthy.AddAbilities(nil))
	model.InitChannelCache()

	param := &RetryParam{
		Ctx:          newRetryContext(),
		TokenGroup:   "default",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAIResponse,
		Retry:        common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 120, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelSkipsSingleOpenAIAuthErrorCapability(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	weight := uint(100)
	priority := int64(10)
	blocked := &model.Channel{
		Id:          123,
		Type:        constant.ChannelTypeOpenAI,
		Name:        "blocked-openai",
		Key:         "sk-auth",
		Status:      common.ChannelStatusEnabled,
		Group:       "default",
		Models:      "gpt-5.5",
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: 123,
		ChannelInfo: model.ChannelInfo{
			MultiKeyCapabilities: map[int]model.ChannelAccountCapability{
				0: {
					CapabilityClassification: channelcapability.ClassificationAuthError,
				},
			},
		},
	}
	healthy := &model.Channel{
		Id:          124,
		Type:        constant.ChannelTypeOpenAI,
		Name:        "healthy-openai",
		Key:         "sk-ok",
		Status:      common.ChannelStatusEnabled,
		Group:       "default",
		Models:      "gpt-5.5",
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: 124,
	}
	require.NoError(t, db.Create(blocked).Error)
	require.NoError(t, blocked.AddAbilities(nil))
	require.NoError(t, db.Create(healthy).Error)
	require.NoError(t, healthy.AddAbilities(nil))
	model.InitChannelCache()

	param := &RetryParam{
		Ctx:          newRetryContext(),
		TokenGroup:   "default",
		ModelName:    "gpt-5.5",
		EndpointType: constant.EndpointTypeOpenAI,
		Retry:        common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "default", group)
	require.NotNil(t, channel)
	require.Equal(t, 124, channel.Id)
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
	require.Contains(t, []int{121, 122}, channel.Id)
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

func TestCacheGetRandomSatisfiedChannelUsesBaseGroupForDerivedTokenGroup(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, false)

	seedChannelSelectChannel(t, db, 331, "codex-plus", "gpt-5.4", 10, 100)

	param := &RetryParam{
		Ctx:        newRetryContext(),
		TokenGroup: "codex-plus-特惠",
		ModelName:  "gpt-5.4",
		Retry:      common.GetPointer(0),
	}

	channel, group, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.Equal(t, "codex-plus-特惠", group)
	require.NotNil(t, channel)
	require.Equal(t, 331, channel.Id)
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
	common.ChannelFailureAvoidanceTTLSeconds = 6
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
	require.InDelta(t, 6, firstRecord.Remaining.Seconds(), 1)
	require.False(t, firstRecord.ShouldPause)

	secondRecord := RecordChannelFailureAvoidance(381, "do_request_failed")
	second, ok := getChannelFailureAvoidanceForTest(381)
	require.True(t, ok)
	require.NotNil(t, secondRecord)
	require.Equal(t, 2, second.failureCount)
	require.InDelta(t, 14, secondRecord.Remaining.Seconds(), 1)
	require.Greater(t, second.until, first.until)
	require.False(t, secondRecord.ShouldPause)
}

func TestRecordChannelFailureAvoidanceKeepsFailureCountUntilCleared(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 6
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	firstRecord := RecordChannelFailureAvoidance(382, "do_request_failed")
	require.NotNil(t, firstRecord)

	entry, ok := getChannelFailureAvoidanceForTest(382)
	require.True(t, ok)
	entry.until = time.Now().Add(-time.Second)
	channelFailureAvoidance.Store(382, entry)

	require.Nil(t, GetChannelFailureAvoidanceStatus(382))

	secondRecord := RecordChannelFailureAvoidance(382, "do_request_failed")
	require.NotNil(t, secondRecord)
	require.Equal(t, 2, secondRecord.FailureCount)
	require.InDelta(t, 14, secondRecord.Remaining.Seconds(), 1)
	require.False(t, secondRecord.ShouldPause)

	ClearChannelFailureAvoidance(382)

	thirdRecord := RecordChannelFailureAvoidance(382, "do_request_failed")
	require.NotNil(t, thirdRecord)
	require.Equal(t, 1, thirdRecord.FailureCount)
	require.InDelta(t, 6, thirdRecord.Remaining.Seconds(), 1)
}

func TestRecordChannelPerformanceAvoidanceDoesNotPauseChannel(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = int(channelFailureAvoidancePauseDuration.Seconds())
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	record := RecordChannelPerformanceAvoidance(384, "slow_ttft", &ChannelPerformanceAvoidanceContext{
		ChannelName: "dora",
		Group:       "auto",
		ModelName:   "gpt-5.5",
		RequestId:   "req-slow",
		TTFTMs:      41000,
		DurationMs:  42000,
	})

	require.NotNil(t, record)
	require.True(t, record.Active)
	require.False(t, record.ShouldPause)
	require.Equal(t, "slow_ttft", record.Reason)
}

func TestRecordChannelFailureAvoidanceNeverAutoPausesChannel(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = int(channelFailureAvoidancePauseDuration.Seconds())
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	record := RecordChannelFailureAvoidance(386, "upstream_error:502")

	require.NotNil(t, record)
	require.True(t, record.Active)
	require.False(t, record.ShouldPause)
	require.InDelta(t, channelFailureAvoidancePauseDuration.Seconds(), record.Remaining.Seconds(), 1)
}

func TestRecordChannelTimeoutDegradeTriggersProbeRecovery(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 1
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	config := ChannelTimeoutDegradeConfig{
		Enabled:     true,
		Window:      time.Minute,
		MinSamples:  5,
		Threshold:   0.4,
		Consecutive: 3,
	}
	require.Nil(t, RecordChannelTimeoutDegradeSample(385, "first_byte_timeout", config, nil))
	require.Nil(t, RecordChannelTimeoutDegradeSample(385, "total_duration_timeout", config, nil))
	record := RecordChannelTimeoutDegradeSample(385, "stream_timeout", config, nil)
	require.NotNil(t, record)
	require.Equal(t, ChannelTimeoutRecoveryReason, record.Reason)
	require.True(t, record.ProbeRecoveryRequired)

	status := GetChannelFailureAvoidanceStatus(385)
	require.NotNil(t, status)
	require.True(t, status.ProbeRecoveryRequired)
	require.Equal(t, ChannelTimeoutRecoveryReason, status.Reason)

	require.False(t, ClearChannelFailureAvoidanceOnRealSuccess(385))
	require.NotNil(t, GetChannelFailureAvoidanceStatus(385))

	ClearChannelProbeRecoveryAvoidance(385)
	require.Nil(t, GetChannelFailureAvoidanceStatus(385))
}

func TestRecordChannelTimeoutDegradeUsesWindowRate(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 1
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	config := ChannelTimeoutDegradeConfig{
		Enabled:     true,
		Window:      time.Minute,
		MinSamples:  5,
		Threshold:   0.4,
		Consecutive: 4,
	}
	RecordChannelTimeoutDegradeSuccess(386, config)
	RecordChannelTimeoutDegradeSuccess(386, config)
	require.Nil(t, RecordChannelTimeoutDegradeSample(386, "status_504", config, nil))
	require.Nil(t, RecordChannelTimeoutDegradeSample(386, "status_503", config, nil))
	record := RecordChannelTimeoutDegradeSample(386, "stream_timeout", config, nil)
	require.NotNil(t, record)
	require.Equal(t, ChannelTimeoutRecoveryReason, record.Reason)
}

func TestRecordChannelRuntimeOverloadRecoveryRequiresProbe(t *testing.T) {
	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 1
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	identity := ChannelRuntimeIdentity{
		ChannelID:          387,
		CredentialIndex:    2,
		CredentialIndexSet: true,
	}
	config := ChannelOverloadRecoveryConfig{
		Enabled:     true,
		Window:      time.Minute,
		MinSamples:  3,
		Consecutive: 3,
	}
	require.Nil(t, RecordChannelRuntimeOverloadRecoverySample(identity, "http_429", config, nil))
	require.Nil(t, RecordChannelRuntimeOverloadRecoverySample(identity, "http_429", config, nil))
	record := RecordChannelRuntimeOverloadRecoverySample(identity, "http_429", config, nil)
	require.NotNil(t, record)
	require.Equal(t, ChannelOverloadRecoveryReason, record.Reason)
	require.True(t, record.ProbeRecoveryRequired)

	status := GetChannelRuntimeFailureAvoidanceStatus(identity)
	require.NotNil(t, status)
	require.True(t, status.ProbeRecoveryRequired)
	require.Equal(t, ChannelOverloadRecoveryReason, status.Reason)

	require.False(t, ClearChannelRuntimeFailureAvoidanceOnRealSuccess(identity))
	require.NotNil(t, GetChannelRuntimeFailureAvoidanceStatus(identity))

	ClearChannelRuntimeProbeRecoveryAvoidance(identity)
	require.Nil(t, GetChannelRuntimeFailureAvoidanceStatus(identity))
}

func TestRecordChannelFailureAvoidancePersistsEventContext(t *testing.T) {
	db := setupChannelSelectTestDB(t)

	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 10
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	record := RecordChannelFailureAvoidanceWithContext(383, "upstream_rate_limit", &ChannelFailureAvoidanceContext{
		ChannelName:  "codex-pro",
		ChannelType:  1,
		Group:        "vip",
		ModelName:    "gpt-5.5",
		RequestId:    "req-383",
		ErrorType:    "openai_error",
		ErrorCode:    "rate_limit_exceeded",
		StatusCode:   429,
		AttemptIndex: 2,
		FinalFailure: true,
		UsedChannels: "4->2",
		Message:      "rate limit exceeded",
		Metadata:     `{"retry_after":10}`,
	})
	require.NotNil(t, record)

	var event model.ChannelFailureEvent
	require.NoError(t, db.Where("channel_id = ?", 383).First(&event).Error)
	require.Equal(t, model.ChannelFailureEventTypeAvoidance, event.EventType)
	require.Equal(t, "upstream_rate_limit", event.Reason)
	require.Equal(t, "codex-pro", event.ChannelName)
	require.Equal(t, "vip", event.Group)
	require.Equal(t, "gpt-5.5", event.ModelName)
	require.Equal(t, "req-383", event.RequestId)
	require.Equal(t, "openai_error", event.ErrorType)
	require.Equal(t, "rate_limit_exceeded", event.ErrorCode)
	require.Equal(t, 429, event.StatusCode)
	require.Equal(t, 2, event.AttemptIndex)
	require.Equal(t, 1, event.FailureCount)
	require.Equal(t, int64(10), event.RemainingSeconds)
	require.True(t, event.FinalFailure)
	require.Equal(t, "4->2", event.UsedChannels)
	require.False(t, event.AutoPaused)
	require.Equal(t, `{"retry_after":10}`, event.Metadata)
}

func TestRecordChannelFailureAvoidanceTruncatesLongUsedChannels(t *testing.T) {
	db := setupChannelSelectTestDB(t)

	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 10
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	longUsedChannels := strings.Repeat("83->", 120) + "83"
	record := RecordChannelFailureAvoidanceWithContext(384, "timeout_recovery", &ChannelFailureAvoidanceContext{
		UsedChannels: longUsedChannels,
	})
	require.NotNil(t, record)

	var event model.ChannelFailureEvent
	require.NoError(t, db.Where("channel_id = ?", 384).First(&event).Error)
	require.Len(t, event.UsedChannels, 255)
	require.Equal(t, longUsedChannels[:255], event.UsedChannels)
}

func TestRecordChannelRuntimeProbeAvoidanceCoalescesActiveRecords(t *testing.T) {
	db := setupChannelSelectTestDB(t)

	originalEnabled := common.ChannelFailureAvoidanceEnabled
	originalTTL := common.ChannelFailureAvoidanceTTLSeconds
	common.ChannelFailureAvoidanceEnabled = true
	common.ChannelFailureAvoidanceTTLSeconds = 60
	t.Cleanup(func() {
		common.ChannelFailureAvoidanceEnabled = originalEnabled
		common.ChannelFailureAvoidanceTTLSeconds = originalTTL
		clearAllChannelFailureAvoidanceForTest()
	})

	identity := ChannelRuntimeIdentity{
		ChannelID:          385,
		CredentialIndex:    1,
		CredentialIndexSet: true,
	}.Normalize()

	firstRecord := RecordChannelRuntimeAuthConfigRecovery(identity, &ChannelFailureAvoidanceContext{RequestId: "req-385-a"})
	require.NotNil(t, firstRecord)

	secondRecord := RecordChannelRuntimeAuthConfigRecovery(identity, &ChannelFailureAvoidanceContext{RequestId: "req-385-b"})
	require.Nil(t, secondRecord)

	var count int64
	require.NoError(t, db.Model(&model.ChannelFailureEvent{}).Where("channel_id = ?", 385).Count(&count).Error)
	require.Equal(t, int64(1), count)

	entry, ok := channelRuntimeFailureAvoidance.Load(identity)
	require.True(t, ok)
	avoidanceEntry, ok := entry.(channelAvoidanceEntry)
	require.True(t, ok)
	avoidanceEntry.lastRecordedAt = time.Now().Add(-channelProbeAvoidanceRecordMinInterval - time.Second)
	channelRuntimeFailureAvoidance.Store(identity, avoidanceEntry)

	thirdRecord := RecordChannelRuntimeTimeoutRecovery(identity, &ChannelFailureAvoidanceContext{RequestId: "req-385-c"})
	require.NotNil(t, thirdRecord)
	require.NoError(t, db.Model(&model.ChannelFailureEvent{}).Where("channel_id = ?", 385).Count(&count).Error)
	require.Equal(t, int64(2), count)
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

func TestCacheGetRandomSatisfiedChannelRespectsCodexImageToolRequirement(t *testing.T) {
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

func TestCacheGetRandomSatisfiedChannelFallsBackToBaseModelForCodexMiniVariant(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannelWithOptions(t, db, 441, "default", "gpt-5.4", 10, 100, constant.ChannelTypeOpenAI, `{"codex_compatibility_mode":true}`)
	seedChannelSelectChannelWithOptions(t, db, 442, "default", "gpt-5.4", 1, 100, constant.ChannelTypeOpenAI, `{"codex_compatibility_mode":true,"codex_image_generation_tool_supported":true}`)
	model.InitChannelCache()

	normalParam := &RetryParam{
		Ctx:          newRetryContext(),
		TokenGroup:   "default",
		ModelName:    "gpt-5.4-mini",
		EndpointType: constant.EndpointTypeOpenAIResponse,
		Retry:        common.GetPointer(0),
	}
	normalChannel, normalGroup, err := CacheGetRandomSatisfiedChannel(normalParam)
	require.NoError(t, err)
	require.Equal(t, "default", normalGroup)
	require.NotNil(t, normalChannel)
	require.Equal(t, 441, normalChannel.Id)

	imageToolParam := &RetryParam{
		Ctx:                    newRetryContext(),
		TokenGroup:             "default",
		ModelName:              "gpt-5.4-mini",
		EndpointType:           constant.EndpointTypeOpenAIResponse,
		RequiresCodexImageTool: true,
		Retry:                  common.GetPointer(0),
	}
	imageToolChannel, imageToolGroup, err := CacheGetRandomSatisfiedChannel(imageToolParam)
	require.NoError(t, err)
	require.Equal(t, "default", imageToolGroup)
	require.NotNil(t, imageToolChannel)
	require.Equal(t, 442, imageToolChannel.Id)
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
