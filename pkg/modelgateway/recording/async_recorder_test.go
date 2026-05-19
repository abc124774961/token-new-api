package recording

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAsyncExecutionRecorderRecordsDispatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}))
	oldDB := model.DB
	model.DB = db
	defer func() {
		model.DB = oldDB
	}()

	recorder := NewAsyncExecutionRecorder(8)
	recorder.Record(context.Background(), core.DispatchRecord{
		Request: core.DispatchRequest{
			RequestedGroup: "default",
			ModelName:      "gpt-4.1",
			EndpointType:   constant.EndpointTypeOpenAI,
		},
		Policy: core.GroupSmartPolicy{
			Mode:            core.ModeShadow,
			AutoMode:        core.AutoModeSequential,
			Strategy:        core.StrategyBalanced,
			CandidateGroups: []string{"default"},
		},
		Plan: &core.DispatchPlan{
			Channel:         &model.Channel{Id: 2, Name: "smart"},
			SelectedGroup:   "default",
			RuntimeKey:      core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "mimo-v1", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI},
			ProviderProfile: "mimo_codex_chat",
			ProxyMode:       "responses_via_chat",
			ScoreTotal:      0.88,
			ScoreBreakdown:  map[string]float64{"success": 0.9},
			QueueEnabled:    true,
			QueueDepth:      1,
			QueueCapacity:   8,
			StickySource:    "prompt_cache_key",
			StickyKeyFP:     "abc123",
			StickyRetained:  true,
			CacheAffinity:   true,
			SelectedReason:  "weighted_score",
			Candidates: []core.CandidateExplanation{
				{
					ChannelID:       2,
					ChannelName:     "smart",
					Group:           "default",
					UpstreamModel:   "mimo-v1",
					ProviderProfile: "mimo_codex_chat",
					ProxyMode:       "responses_via_chat",
					RuntimeKey:      core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "mimo-v1", ChannelID: 2, Group: "default", EndpointType: constant.EndpointTypeOpenAI},
					Available:       true,
					ScoreTotal:      0.88,
					ScoreBreakdown:  map[string]float64{"success": 0.9},
					Selected:        true,
				},
			},
		},
		Actual:      &model.Channel{Id: 1, Name: "legacy"},
		ActualGroup: "default",
		Shadow:      true,
		RecordedAt:  time.Now(),
	})

	require.Eventually(t, func() bool {
		var count int64
		require.NoError(t, db.Model(&model.ModelExecutionRecord{}).Count(&count).Error)
		return count == 1
	}, time.Second, 10*time.Millisecond)

	var record model.ModelExecutionRecord
	require.NoError(t, db.First(&record).Error)
	require.True(t, record.Shadow)
	require.True(t, record.SmartHandled)
	require.Equal(t, 2, record.ChannelId)
	require.Equal(t, 1, record.ActualChannelId)
	require.Equal(t, "default", record.SelectedGroup)
	require.Contains(t, record.ScoreBreakdown, "success")
	require.Contains(t, record.RequestMeta, "mimo_codex_chat")
	require.Contains(t, record.RequestMeta, "responses_via_chat")
	require.Contains(t, record.RequestMeta, "prompt_cache_key")
	require.Contains(t, record.RequestMeta, "candidate_explanations")
	require.Contains(t, record.RequestMeta, "mimo-v1")
}
