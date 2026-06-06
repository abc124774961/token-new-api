package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAdminChannelAuditControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Channel{},
		&model.Ability{},
		&model.Log{},
	))
	model.DB = db
	model.LOG_DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})
	return db
}

func adminChannelAuditControllerTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 100)
		c.Set("username", "channel-admin")
		c.Set("role", common.RoleRootUser)
		c.Set("group", "default")
		c.Set(common.RequestIdKey, "admin-channel-audit-test")
		c.Next()
	})
	router.POST("/api/channel/batch", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelDanger), DeleteChannelBatch)
	router.POST("/api/channel/batch/tag", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), BatchSetChannelTag)
	return router
}

func createAdminAuditTestChannel(t *testing.T, db *gorm.DB, channel model.Channel) model.Channel {
	t.Helper()
	if channel.Type == 0 {
		channel.Type = constant.ChannelTypeOpenAI
	}
	if channel.Key == "" {
		channel.Key = "sk-test"
	}
	if channel.Models == "" {
		channel.Models = "gpt-test"
	}
	if channel.Group == "" {
		channel.Group = "default"
	}
	require.NoError(t, db.Create(&channel).Error)
	return channel
}

func TestDeleteChannelBatchAuditSummaryIncludesTargetStats(t *testing.T) {
	db := setupAdminChannelAuditControllerTestDB(t)
	tagA := "batch-a"
	tagB := "batch-b"
	first := createAdminAuditTestChannel(t, db, model.Channel{
		Name:   "delete-enabled",
		Status: common.ChannelStatusEnabled,
		Tag:    &tagA,
	})
	second := createAdminAuditTestChannel(t, db, model.Channel{
		Name:   "delete-manual-disabled",
		Status: common.ChannelStatusManuallyDisabled,
		Tag:    &tagB,
	})
	kept := createAdminAuditTestChannel(t, db, model.Channel{
		Name:   "keep-auto-disabled",
		Status: common.ChannelStatusAutoDisabled,
		Tag:    &tagB,
	})

	router := adminChannelAuditControllerTestRouter()
	recorder := httptest.NewRecorder()
	body := `{"ids":[` + strconv.Itoa(first.Id) + `,` + strconv.Itoa(second.Id) + `,` + strconv.Itoa(first.Id) + `,0]}`
	request := httptest.NewRequest(http.MethodPost, "/api/channel/batch", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])

	var remaining int64
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", kept.Id).Count(&remaining).Error)
	require.Equal(t, int64(1), remaining)
	require.NoError(t, db.Model(&model.Channel{}).Where("id IN ?", []int{first.Id, second.Id}).Count(&remaining).Error)
	require.Equal(t, int64(0), remaining)

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "delete_channel_batch", summary["operation"])
	require.Equal(t, float64(4), summary["requested_count"])
	require.Equal(t, float64(2), summary["requested_unique_count"])
	require.Equal(t, float64(1), summary["invalid_id_count"])
	require.Equal(t, float64(2), summary["target_channel_count_before"])
	require.Equal(t, float64(1), summary["target_enabled_count_before"])
	require.Equal(t, float64(1), summary["target_manual_disabled_count_before"])
	require.Equal(t, float64(0), summary["target_auto_disabled_count_before"])
	require.Equal(t, float64(2), summary["target_tagged_count_before"])
	require.Equal(t, float64(2), summary["target_distinct_tag_count_before"])
}

func TestBatchSetChannelTagAuditSummaryIncludesBeforeAfterStats(t *testing.T) {
	db := setupAdminChannelAuditControllerTestDB(t)
	oldTag := "old-tag"
	newTag := "new-tag"
	first := createAdminAuditTestChannel(t, db, model.Channel{
		Name:   "tag-enabled",
		Status: common.ChannelStatusEnabled,
		Tag:    &oldTag,
	})
	second := createAdminAuditTestChannel(t, db, model.Channel{
		Name:   "tag-auto-disabled",
		Status: common.ChannelStatusAutoDisabled,
	})

	router := adminChannelAuditControllerTestRouter()
	recorder := httptest.NewRecorder()
	body := `{"ids":[` + strconv.Itoa(first.Id) + `,` + strconv.Itoa(second.Id) + `],"tag":"` + newTag + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/channel/batch/tag", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload), recorder.Body.String())
	require.Equal(t, true, payload["success"])

	var channels []model.Channel
	require.NoError(t, db.Where("id IN ?", []int{first.Id, second.Id}).Find(&channels).Error)
	require.Len(t, channels, 2)
	for _, channel := range channels {
		require.NotNil(t, channel.Tag)
		require.Equal(t, newTag, *channel.Tag)
	}

	summary := latestAdminPermissionAuditSummary(t, db)
	require.Equal(t, "batch_set_channel_tag", summary["operation"])
	require.Equal(t, float64(2), summary["requested_count"])
	require.Equal(t, float64(2), summary["requested_unique_count"])
	require.Equal(t, float64(0), summary["invalid_id_count"])
	require.Equal(t, false, summary["tag_after_is_nil"])
	require.Equal(t, newTag, summary["tag_after"])
	require.Equal(t, float64(2), summary["target_channel_count_before"])
	require.Equal(t, float64(1), summary["target_enabled_count_before"])
	require.Equal(t, float64(1), summary["target_auto_disabled_count_before"])
	require.Equal(t, float64(1), summary["target_tagged_count_before"])
	require.Equal(t, float64(1), summary["target_untagged_count_before"])
	require.Equal(t, float64(1), summary["target_distinct_tag_count_before"])
	require.Equal(t, float64(2), summary["target_channel_count_after"])
	require.Equal(t, float64(2), summary["target_tagged_count_after"])
	require.Equal(t, float64(0), summary["target_untagged_count_after"])
	require.Equal(t, float64(1), summary["target_distinct_tag_count_after"])
}
