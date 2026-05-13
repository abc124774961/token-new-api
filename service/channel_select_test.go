package service

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

	channel := &model.Channel{
		Id:          id,
		Name:        fmt.Sprintf("channel-%d", id),
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
