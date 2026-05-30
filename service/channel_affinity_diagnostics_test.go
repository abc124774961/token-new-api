package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelAffinityDiagnosticsLogDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldLogDB := model.LOG_DB
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	model.LOG_DB = db
	t.Cleanup(func() {
		model.LOG_DB = oldLogDB
	})
	return db
}

func setupChannelAffinityDiagnosticsMainDB(t *testing.T) *gorm.DB {
	t.Helper()
	oldDB := model.DB
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "_main?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = oldDB
	})
	return db
}

func TestMarkChannelAffinitySelectionWritesRetainedAndBrokenState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName:           "codex cli trace",
		UsingGroup:         "default",
		ModelName:          "gpt-5",
		RequestPath:        "/v1/responses",
		KeySourceType:      "gjson",
		KeySourcePath:      "prompt_cache_key",
		KeyHint:            "pc-1",
		KeyFingerprint:     "fp123",
		PreferredChannelID: 11,
	})

	MarkChannelAffinitySelection(ctx, ChannelAffinitySelectionInfo{
		SelectedGroup:      "default",
		SelectedChannelID:  22,
		Retained:           false,
		Broken:             true,
		BreakReason:        "score_below_threshold",
		StickySource:       "cache_affinity",
		SelectedReason:     "score_items_sticky_broken",
		AccountID:          "acct-a",
		CredentialIndex:    2,
		HasCredentialIndex: true,
	})
	adminInfo := map[string]interface{}{}
	AppendChannelAffinityAdminInfo(ctx, adminInfo)

	affinity, ok := adminInfo["channel_affinity"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "codex cli trace", affinity["rule_name"])
	require.Equal(t, "prompt_cache_key", affinity["key_source"])
	require.Equal(t, "fp123", affinity["key_fp"])
	require.Equal(t, 11, affinity["preferred_channel_id"])
	require.Equal(t, 22, affinity["selected_channel_id"])
	require.Equal(t, false, affinity["retained"])
	require.Equal(t, true, affinity["broken"])
	require.Equal(t, "score_below_threshold", affinity["break_reason"])
	require.Equal(t, "acct-a", affinity["account_id"])
	require.Equal(t, 2, affinity["credential_index"])
}

func TestGetChannelAffinityDiagnosticsAggregatesUsageLogs(t *testing.T) {
	db := setupChannelAffinityDiagnosticsLogDB(t)
	now := time.Now().Unix()
	keyFP := fmt.Sprintf("fp-%d", now)

	require.NoError(t, db.Create(&[]model.Log{
		{
			CreatedAt:    now - 20,
			Type:         model.LogTypeConsume,
			Username:     "alice",
			TokenName:    "codex-token",
			ModelName:    "gpt-5.5",
			PromptTokens: 1000,
			ChannelId:    22,
			Group:        "default",
			Other: common.MapToJsonStr(map[string]interface{}{
				"cache_tokens":          500,
				"cache_token_rate_mode": cacheTokenRateModeCachedOverPrompt,
				"admin_info": map[string]interface{}{
					"channel_affinity": map[string]interface{}{
						"rule_name":            "codex cli trace",
						"key_source":           "prompt_cache_key",
						"key_fp":               keyFP,
						"preferred_channel_id": 22,
						"selected_channel_id":  22,
						"retained":             true,
						"broken":               false,
						"account_id":           "acct-a",
						"credential_index":     0,
					},
				},
			}),
		},
		{
			CreatedAt:    now - 10,
			Type:         model.LogTypeConsume,
			Username:     "alice",
			TokenName:    "codex-token",
			ModelName:    "gpt-5.5",
			PromptTokens: 800,
			ChannelId:    23,
			Group:        "default",
			Other: common.MapToJsonStr(map[string]interface{}{
				"cache_token_rate_mode": cacheTokenRateModeCachedOverPrompt,
				"admin_info": map[string]interface{}{
					"channel_affinity": map[string]interface{}{
						"rule_name":            "codex cli trace",
						"key_source":           "prompt_cache_key",
						"key_fp":               keyFP,
						"preferred_channel_id": 22,
						"selected_channel_id":  23,
						"retained":             false,
						"broken":               true,
						"break_reason":         "first_byte_timeout",
						"account_id":           "acct-b",
						"credential_index":     1,
					},
				},
			}),
		},
		{
			CreatedAt:    now - 5,
			Type:         model.LogTypeConsume,
			Username:     "alice",
			TokenName:    "codex-token",
			ModelName:    "gpt-5.5",
			PromptTokens: 300,
			ChannelId:    24,
			Group:        "default",
			Other:        common.MapToJsonStr(map[string]interface{}{}),
		},
	}).Error)

	resp, err := GetChannelAffinityDiagnostics(ChannelAffinityDiagnosticsQuery{
		StartTimestamp: now - 60,
		EndTimestamp:   now,
		ModelName:      "gpt-5.5",
		Group:          "default",
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, resp.Summary.TotalLogs)
	require.EqualValues(t, 2, resp.Summary.AffinityLogs)
	require.EqualValues(t, 1, resp.Summary.NoAffinityLogs)
	require.EqualValues(t, 1, resp.Summary.CacheHits)
	require.EqualValues(t, 500, resp.Summary.CachedTokens)
	require.EqualValues(t, 1800, resp.Summary.PromptTokens)
	require.EqualValues(t, 1, resp.Summary.Retained)
	require.EqualValues(t, 1, resp.Summary.Broken)
	require.EqualValues(t, 1, resp.Summary.ChannelSwitches)
	require.EqualValues(t, 1, resp.Summary.AccountSwitches)
	require.EqualValues(t, 1, resp.Summary.UpstreamNoCachedTokenLogs)
	require.Equal(t, 1, resp.Summary.DistinctKeys)
	require.EqualValues(t, 2, resp.Summary.KeySources["prompt_cache_key"])
	require.EqualValues(t, 1, resp.Summary.BreakReasons["first_byte_timeout"])
	require.InEpsilon(t, float64(1)/2, resp.Summary.HitRate, 0.0001)
	require.InEpsilon(t, float64(500)/1800, resp.Summary.CachedTokenRate, 0.0001)
	require.NotEmpty(t, resp.Rows)
}

func TestGetChannelAffinityDiagnosticsLoadsSelectedChannelName(t *testing.T) {
	logDB := setupChannelAffinityDiagnosticsLogDB(t)
	mainDB := setupChannelAffinityDiagnosticsMainDB(t)
	now := time.Now().Unix()

	require.NoError(t, mainDB.Create(&[]model.Channel{
		{Id: 22, Name: "original-channel"},
		{Id: 23, Name: "selected-channel"},
	}).Error)
	require.NoError(t, logDB.Create(&model.Log{
		CreatedAt:    now,
		Type:         model.LogTypeConsume,
		ModelName:    "gpt-5.5",
		PromptTokens: 100,
		ChannelId:    22,
		Group:        "default",
		Other: common.MapToJsonStr(map[string]interface{}{
			"admin_info": map[string]interface{}{
				"channel_affinity": map[string]interface{}{
					"rule_name":            "codex cli trace",
					"key_source":           "prompt_cache_key",
					"key_fp":               "fp-selected-channel",
					"preferred_channel_id": 22,
					"selected_channel_id":  23,
					"broken":               true,
				},
			},
		}),
	}).Error)

	resp, err := GetChannelAffinityDiagnostics(ChannelAffinityDiagnosticsQuery{
		StartTimestamp: now - 10,
		EndTimestamp:   now + 10,
	})
	require.NoError(t, err)
	require.Len(t, resp.Rows, 1)
	require.Equal(t, 23, resp.Rows[0].ChannelID)
	require.Equal(t, "selected-channel", resp.Rows[0].ChannelName)
}

func TestGetChannelAffinityDiagnosticsFiltersNoAffinityRowsForAffinityFilters(t *testing.T) {
	db := setupChannelAffinityDiagnosticsLogDB(t)
	now := time.Now().Unix()

	require.NoError(t, db.Create(&[]model.Log{
		{
			CreatedAt:    now - 1,
			Type:         model.LogTypeConsume,
			ModelName:    "gpt-5.5",
			PromptTokens: 100,
			ChannelId:    22,
			Group:        "default",
			Other: common.MapToJsonStr(map[string]interface{}{
				"admin_info": map[string]interface{}{
					"channel_affinity": map[string]interface{}{
						"rule_name":  "codex cli trace",
						"key_source": "prompt_cache_key",
						"key_fp":     "fp-filtered",
					},
				},
			}),
		},
		{
			CreatedAt:    now,
			Type:         model.LogTypeConsume,
			ModelName:    "gpt-5.5",
			PromptTokens: 100,
			ChannelId:    23,
			Group:        "default",
			Other:        common.MapToJsonStr(map[string]interface{}{}),
		},
	}).Error)

	resp, err := GetChannelAffinityDiagnostics(ChannelAffinityDiagnosticsQuery{
		StartTimestamp: now - 10,
		EndTimestamp:   now + 10,
		RuleName:       "codex cli trace",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, resp.Summary.TotalLogs)
	require.EqualValues(t, 1, resp.Summary.AffinityLogs)
	require.EqualValues(t, 0, resp.Summary.NoAffinityLogs)
	require.Len(t, resp.Rows, 1)
	require.Equal(t, "fp-filtered", resp.Rows[0].KeyFingerprint)
}

func TestChannelAffinityDiagnosticsStringIgnoresCompositeValues(t *testing.T) {
	value := channelAffinityDiagnosticsString(map[string]interface{}{
		"object": map[string]interface{}{"unexpected": true},
		"array":  []interface{}{"unexpected"},
		"zero":   0,
	}, "object")
	require.Empty(t, value)
	require.Empty(t, channelAffinityDiagnosticsString(map[string]interface{}{
		"array": []interface{}{"unexpected"},
	}, "array"))
	require.Equal(t, "0", channelAffinityDiagnosticsString(map[string]interface{}{
		"zero": 0,
	}, "zero"))
}
