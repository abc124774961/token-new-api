package probe

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProbeBillingRecorderRecordsRootConsumeLog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Log{}))
	oldDB := model.DB
	oldLogDB := model.LOG_DB
	oldLogConsume := common.LogConsumeEnabled
	oldRedisEnabled := common.RedisEnabled
	model.DB = db
	model.LOG_DB = db
	common.LogConsumeEnabled = true
	common.RedisEnabled = false
	defer func() {
		model.DB = oldDB
		model.LOG_DB = oldLogDB
		common.LogConsumeEnabled = oldLogConsume
		common.RedisEnabled = oldRedisEnabled
	}()

	root := model.User{Id: 1, Username: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 1000}
	channel := model.Channel{Id: 10, Name: "probe-channel", Status: common.ChannelStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&root).Error)
	require.NoError(t, db.Create(&channel).Error)

	c, _ := newProbeGinContext(context.Background(), "probe-log-1", "/v1/chat/completions")
	require.NoError(t, writeRootContext(c, "probe-log-1", "default"))
	info := &relaycommon.RelayInfo{
		RequestId:           "probe-log-1",
		UserId:              1,
		UsingGroup:          "default",
		UserGroup:           "default",
		RequestModelName:    "gpt-4.1",
		OriginModelName:     "gpt-4.1",
		ContextModelName:    "gpt-4.1",
		StartTime:           time.Now().Add(-200 * time.Millisecond),
		FirstResponseTime:   time.Now().Add(-120 * time.Millisecond),
		RelayFormat:         types.RelayFormatOpenAI,
		RequestHeaders:      map[string]string{},
		IsChannelTest:       true,
		ChannelMeta:         &relaycommon.ChannelMeta{ChannelId: channel.Id, UpstreamModelName: "gpt-4.1"},
		PriceData:           types.PriceData{ModelRatio: 1, CompletionRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
		BillingRequestInput: nil,
	}
	result := ProbeRunResult{
		ProbeID:    "probe-log-1",
		Reason:     reasonLowScore,
		Channel:    &channel,
		Model:      "gpt-4.1",
		Group:      "default",
		RuntimeKey: core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default"},
		Context:    c,
		RelayInfo:  info,
		Success:    true,
		Duration:   200 * time.Millisecond,
		TTFT:       80 * time.Millisecond,
	}
	usage := &dto.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}

	quota, err := NewProbeBillingRecorder().RecordSuccess(c, result, info.PriceData, usage)
	require.NoError(t, err)
	require.Equal(t, 5, quota)

	var updatedRoot model.User
	require.NoError(t, db.First(&updatedRoot, "id = ?", root.Id).Error)
	require.Equal(t, 995, updatedRoot.Quota)
	require.Equal(t, 5, updatedRoot.UsedQuota)
	require.Equal(t, 1, updatedRoot.RequestCount)

	var updatedChannel model.Channel
	require.NoError(t, db.First(&updatedChannel, "id = ?", channel.Id).Error)
	require.Equal(t, int64(5), updatedChannel.UsedQuota)

	var log model.Log
	require.NoError(t, db.First(&log, "request_id = ?", "probe-log-1").Error)
	require.Equal(t, model.LogTypeConsume, log.Type)
	require.Equal(t, TokenName, log.TokenName)
	require.Equal(t, 0, log.TokenId)
	require.Equal(t, 5, log.Quota)
	require.Contains(t, log.Other, `"is_health_probe":true`)
	require.Contains(t, log.Other, `"billing_source":"model_gateway_probe"`)
	require.Contains(t, log.Other, `"probe_reason":"low_score"`)
}

func TestProbeAttemptResultLearnsUpstreamConcurrencyLimit(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, db.Exec("CREATE TABLE channels (id integer primary key, name text, status integer, `group` text, models text, setting text)").Error)
	oldDB := model.DB
	oldMemoryCache := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = false
	service.ClearChannelConcurrencyForTest()
	defer func() {
		service.ClearChannelConcurrencyForTest()
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCache
	}()

	channel := model.Channel{Id: 11, Name: "probe-busy", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-4.1"}
	require.NoError(t, db.Exec("INSERT INTO channels (id, name, status, `group`, models, setting) VALUES (?, ?, ?, ?, ?, ?)",
		channel.Id, channel.Name, channel.Status, channel.Group, channel.Models, `{"custom_setting":"keep-me"}`).Error)

	leases := make([]*service.ChannelConcurrencyLease, 0, 4)
	for i := 0; i < 4; i++ {
		leases = append(leases, service.TrackChannelConcurrency(channel.Id, dto.ChannelSettings{}))
	}
	defer func() {
		for _, lease := range leases {
			lease.Release()
		}
	}()

	result := ProbeRunResult{
		ProbeID: "probe-concurrency-1",
		Reason:  reasonLowScore,
		Channel: &channel,
		Model:   "gpt-4.1",
		Group:   "default",
		NewAPIError: types.NewOpenAIError(
			errors.New("status_code=429, Too many pending requests, please retry later"),
			types.ErrorCodeBadResponseStatusCode,
			http.StatusTooManyRequests,
		),
		StatusCode: http.StatusTooManyRequests,
	}
	attempt := result.AttemptResult()
	require.True(t, attempt.ConcurrencyLimited)
	require.Equal(t, core.ErrorCategoryOverloadSkip, attempt.ErrorCategory)
	require.Equal(t, 4, attempt.ActiveConcurrency)
	require.Equal(t, 3, attempt.LearnedConcurrencyLimit)
	require.True(t, attempt.LearnedConcurrencyLimitChanged)

	updated, getErr := model.GetChannelById(channel.Id, true)
	require.NoError(t, getErr)
	require.Equal(t, 3, updated.GetSetting().MaxConcurrency)
	require.Contains(t, *updated.Setting, `"custom_setting":"keep-me"`)
}

func TestProbeErrorCategoryMapsOverloadAndAuthConfig(t *testing.T) {
	require.Equal(t, core.ErrorCategoryOverloadSkip, probeErrorCategory(types.NewErrorWithStatusCode(
		errors.New("too many requests"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)))
	require.Equal(t, core.ErrorCategoryAuthConfigError, probeErrorCategory(types.NewErrorWithStatusCode(
		errors.New("invalid api key"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusUnauthorized,
	)))
	require.Equal(t, core.ErrorCategoryAuthConfigError, probeErrorCategory(types.NewErrorWithStatusCode(
		errors.New("permission denied"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusForbidden,
	)))
}
