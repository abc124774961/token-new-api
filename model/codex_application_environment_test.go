package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCodexApplicationEnvironmentModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.MemoryCacheEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&CodexApplicationEnvironment{}, &Channel{}, &Log{}))

	oldDB := DB
	oldLogDB := LOG_DB
	DB = db
	LOG_DB = db
	clearCodexApplicationEnvironmentCacheAll()
	t.Cleanup(func() {
		clearCodexApplicationEnvironmentCacheAll()
		DB = oldDB
		LOG_DB = oldLogDB
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestSeedDefaultCodexApplicationEnvironmentsDoesNotCreateSystemSeeds(t *testing.T) {
	db := setupCodexApplicationEnvironmentModelTestDB(t)

	require.NoError(t, SeedDefaultCodexApplicationEnvironments())

	var count int64
	require.NoError(t, db.Model(&CodexApplicationEnvironment{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestBuildCodexApplicationEnvironmentFromRequestHeadersKeepsOnlyStableRealHeaders(t *testing.T) {
	env, ok := BuildCodexApplicationEnvironmentFromRequestHeaders(map[string]string{
		"User-Agent":             "Codex Desktop/0.135.0-alpha.1 (Mac OS 15.1.0; arm64) unknown (Codex Desktop; 26.527.31326)",
		"Originator":             "codex_cli_rs",
		"X-Codex-Beta-Features":  "terminal_resize_reflow",
		"X-Codex-Turn-Metadata":  `{"session_id":"sess-real","thread_id":"thread-real"}`,
		"X-Codex-Window-Id":      "window-real",
		"Session_id":             "sess-real",
		"Authorization":          "Bearer secret",
		"OpenAI-Beta":            "responses=v1",
		"X-Stainless-Runtime":    "node",
		"X-Stainless-Arch":       "arm64",
		"X-Codex-Trace":          "trace-real",
		"X-Codex-Request-Id":     "request-real",
		"X-Codex-Api-Key-Source": "danger",
	})

	require.True(t, ok)
	require.Equal(t, CodexApplicationEnvironmentRealRequestSource, env.Source)
	require.Equal(t, "Codex Desktop/0.135.0-alpha.1 (Mac OS 15.1.0; arm64) unknown (Codex Desktop; 26.527.31326)", env.UserAgent)
	require.Equal(t, "0.135.0-alpha.1", env.AppVersion)
	require.Equal(t, "Mac OS 15.1.0", env.Platform)
	require.Empty(t, env.SessionID)
	require.Empty(t, env.WindowID)
	require.Empty(t, env.TurnMetadata)

	headers := env.BuildHeaders()
	require.Equal(t, env.UserAgent, headers["User-Agent"])
	require.Equal(t, "codex_cli_rs", headers["originator"])
	require.Equal(t, "terminal_resize_reflow", headers["x-codex-beta-features"])
	require.Equal(t, "responses=v1", headers["openai-beta"])
	require.Equal(t, "node", headers["x-stainless-runtime"])
	require.Equal(t, "arm64", headers["x-stainless-arch"])
	require.NotContains(t, headers, "X-Codex-Turn-Metadata")
	require.NotContains(t, headers, "x-codex-turn-metadata")
	require.NotContains(t, headers, "X-Codex-Window-Id")
	require.NotContains(t, headers, "x-codex-window-id")
	require.NotContains(t, headers, "session_id")
	require.NotContains(t, headers, "x-codex-trace")
	require.NotContains(t, headers, "x-codex-request-id")
	require.NotContains(t, headers, "x-codex-api-key-source")
}

func TestCollectCodexApplicationEnvironmentsFromLogsDedupesStableHeaders(t *testing.T) {
	db := setupCodexApplicationEnvironmentModelTestDB(t)

	insertLog := func(requestID string, headers map[string]string) {
		t.Helper()
		other, err := common.Marshal(map[string]any{
			"admin_info": map[string]any{
				"client_request": map[string]any{
					"headers": headers,
				},
			},
		})
		require.NoError(t, err)
		require.NoError(t, db.Create(&Log{
			Type:      LogTypeConsume,
			RequestId: requestID,
			Other:     string(other),
		}).Error)
	}
	baseHeaders := map[string]string{
		"user-agent":            "Codex Desktop/0.135.0 (Mac OS 15.1.0; arm64)",
		"originator":            "codex_cli_rs",
		"x-codex-beta-features": "terminal_resize_reflow",
		"x-codex-window-id":     "window-one",
	}
	insertLog("req-one", baseHeaders)
	insertLog("req-two", map[string]string{
		"user-agent":             baseHeaders["user-agent"],
		"originator":             baseHeaders["originator"],
		"x-codex-beta-features":  baseHeaders["x-codex-beta-features"],
		"x-codex-turn-metadata":  `{"session_id":"different"}`,
		"x-codex-window-id":      "window-two",
		"x-codex-api-key-source": "danger",
	})

	collected, err := CollectCodexApplicationEnvironmentsFromLogs(10)
	require.NoError(t, err)
	require.Equal(t, 1, collected)

	envs, err := ListRealCodexApplicationEnvironments()
	require.NoError(t, err)
	require.Len(t, envs, 1)
	require.Equal(t, baseHeaders["user-agent"], envs[0].UserAgent)
	require.Empty(t, envs[0].WindowID)
	require.NotContains(t, envs[0].BuildHeaders(), "x-codex-window-id")
}

func TestReconcileCodexApplicationEnvironmentBindingsClearsSystemSeedAndBackfillsRealSamples(t *testing.T) {
	db := setupCodexApplicationEnvironmentModelTestDB(t)
	systemEnv := CodexApplicationEnvironment{
		Name:      "codex-env-001",
		UserAgent: "codex_cli_rs/0.0.0",
		Enabled:   true,
		Source:    CodexApplicationEnvironmentSystemSource,
	}
	realEnvs := []CodexApplicationEnvironment{
		{Name: "codex-real-one", UserAgent: "Codex Desktop/0.135.0", Originator: "codex_cli_rs", Enabled: true, Source: CodexApplicationEnvironmentRealRequestSource},
		{Name: "codex-real-two", UserAgent: "Codex Desktop/0.136.0", Originator: "codex_cli_rs", Enabled: true, Source: CodexApplicationEnvironmentRealRequestSource},
	}
	require.NoError(t, db.Create(&systemEnv).Error)
	require.NoError(t, db.Create(&realEnvs).Error)
	channel := Channel{
		Id:     9301,
		Name:   "codex env reconcile",
		Type:   constant.ChannelTypeCodex,
		Status: common.ChannelStatusEnabled,
		Key: strings.Join([]string{
			`{"access_token":"access-one","refresh_token":"refresh-one","account_id":"acct-one"}`,
			`{"access_token":"access-two","refresh_token":"refresh-two","account_id":"acct-two"}`,
			`{"access_token":"access-three","refresh_token":"refresh-three","account_id":"acct-three"}`,
		}, "\n"),
		ChannelInfo: ChannelInfo{
			IsMultiKey:                  true,
			MultiKeySize:                3,
			MultiKeyCodexEnvironmentIDs: map[int]int{0: systemEnv.Id, 1: systemEnv.Id, 2: systemEnv.Id},
		},
	}
	require.NoError(t, db.Create(&channel).Error)

	require.NoError(t, SyncCodexApplicationEnvironments())

	var disabledSystem CodexApplicationEnvironment
	require.NoError(t, db.First(&disabledSystem, systemEnv.Id).Error)
	require.False(t, disabledSystem.Enabled)

	var updated Channel
	require.NoError(t, db.First(&updated, channel.Id).Error)
	require.Nil(t, updated.ChannelInfo.MultiKeyCodexEnvironmentIDs)
	require.Len(t, updated.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys, 3)
	for index, key := range updated.GetKeys() {
		accountUniqueKey := codexApplicationEnvironmentAccountUniqueKey(&updated, index, key)
		require.NotEmpty(t, accountUniqueKey)
		expected := realEnvs[index%len(realEnvs)].Id
		require.Equal(t, expected, updated.ChannelInfo.MultiKeyCodexEnvironmentAccountUniqueKeys[accountUniqueKey], fmt.Sprintf("account %d should be rebound by unique key", index))
	}
}
