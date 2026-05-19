package replay

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestExportArtifactSanitizesModelExecutionRecord(t *testing.T) {
	scoreBreakdown, err := common.Marshal(map[string]float64{"success": 0.8, "speed": 0.7})
	require.NoError(t, err)
	candidateGroups, err := common.Marshal([]string{"vip", "default"})
	require.NoError(t, err)
	requestMeta, err := common.Marshal(RequestMeta{
		OriginalModelName: "mimo-v1",
		UserUsingGroup:    "vip",
		PromptTokens:      1234,
		PreConsumedQuota:  5678,
		CandidateExplanations: []CandidateExplanation{
			{
				ChannelID:       901,
				ChannelName:     "https://api.secret.example.com/v1?key=abc",
				Group:           "default",
				UpstreamModel:   "mimo-v1",
				ProviderProfile: "mimo_codex_chat",
				ProxyMode:       "responses_via_chat",
				RuntimeKey: RuntimeKey{
					RequestedModel:        "mimo-v1",
					UpstreamModel:         "mimo-v1",
					ChannelID:             901,
					Group:                 "default",
					EndpointType:          string(constant.EndpointTypeOpenAIResponse),
					CapabilityFingerprint: "mimo_codex_chat",
				},
				Available:      true,
				ScoreTotal:     0.88,
				ScoreBreakdown: map[string]float64{"success": 0.8, "speed": 0.7},
				Selected:       true,
			},
			{
				ChannelID:       902,
				ChannelName:     "sk-secret-channel",
				Group:           "vip",
				UpstreamModel:   "gpt-5.5",
				ProviderProfile: "openai_codex",
				RuntimeKey: RuntimeKey{
					RequestedModel: "mimo-v1",
					UpstreamModel:  "gpt-5.5",
					ChannelID:      902,
					Group:          "vip",
					EndpointType:   string(constant.EndpointTypeOpenAIResponse),
				},
				Available:    false,
				RejectReason: "concurrency_full",
			},
		},
	})
	require.NoError(t, err)

	artifact, err := ExportArtifact([]model.ModelExecutionRecord{
		{
			CreatedAt:         1710000000,
			RequestId:         "req-secret",
			UserId:            42,
			TokenId:           77,
			RequestedGroup:    "vip",
			SelectedGroup:     "default",
			RequestedModel:    "mimo-v1",
			ChannelId:         901,
			ChannelName:       "https://api.secret.example.com/v1?key=abc",
			EndpointType:      string(constant.EndpointTypeOpenAIResponse),
			PolicyMode:        core.ModeActive,
			Strategy:          core.StrategyBalanced,
			SmartHandled:      true,
			Success:           false,
			StatusCode:        500,
			ErrorCode:         "bad_response",
			DurationMs:        900,
			TTFTMs:            120,
			StreamInterrupted: true,
			ScoreTotal:        0.88,
			ScoreBreakdown:    string(scoreBreakdown),
			CandidateGroups:   string(candidateGroups),
			SelectedReason:    "weighted_score",
			RequestMeta:       string(requestMeta),
		},
	}, ExportOptions{StableIDs: true})

	require.NoError(t, err)
	require.NoError(t, artifact.Validate())
	require.Len(t, artifact.Records, 1)
	record := artifact.Records[0]
	require.Empty(t, record.RequestID)
	require.Zero(t, record.UserID)
	require.Zero(t, record.TokenID)
	require.Zero(t, record.CreatedAt)
	require.Equal(t, 1, record.ChannelID)
	require.NotContains(t, record.ChannelName, "secret.example.com")
	require.Zero(t, record.RequestMeta.PromptTokens)
	require.Zero(t, record.RequestMeta.PreConsumedQuota)
	require.True(t, record.StreamInterrupted)
	require.Empty(t, record.RequestMeta.CandidateExplanations)
	require.Len(t, record.CandidateExplanations, 2)
	require.Equal(t, 1, record.CandidateExplanations[0].ChannelID)
	require.Equal(t, 2, record.CandidateExplanations[1].ChannelID)
	require.NotContains(t, record.CandidateExplanations[0].ChannelName, "secret.example.com")
	require.NotContains(t, record.CandidateExplanations[1].ChannelName, "sk-")
	require.True(t, record.CandidateExplanations[0].Selected)
	require.False(t, record.CandidateExplanations[1].Available)
	require.Equal(t, "concurrency_full", record.CandidateExplanations[1].RejectReason)
	require.Equal(t, 1, record.CandidateExplanations[0].RuntimeKey.ChannelID)
}

func TestArtifactExporterAggregatesByRequestIDAndWritesGolden(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}))
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      20,
		RequestId:      "req-other",
		RequestedGroup: "default",
		RequestedModel: "gpt-5.5",
		ChannelId:      90,
		EndpointType:   string(constant.EndpointTypeOpenAI),
		PolicyMode:     core.ModeActive,
	}).Error)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:      10,
		RequestId:      "req-replay",
		RequestedGroup: "default",
		SelectedGroup:  "default",
		RequestedModel: "gpt-5.5",
		ChannelId:      8,
		EndpointType:   string(constant.EndpointTypeOpenAI),
		PolicyMode:     core.ModeActive,
		Strategy:       core.StrategyBalanced,
		SmartHandled:   true,
	}).Error)

	exporter := NewArtifactExporter(NewGormRecordRepository(db), ExportOptions{StableIDs: true})
	path := filepath.Join(t.TempDir(), "replay", "req-replay.json")
	artifact, err := exporter.WriteGoldenByRequestID("req-replay", path)
	require.NoError(t, err)
	require.Len(t, artifact.Records, 1)
	require.Equal(t, 1, artifact.Records[0].ChannelID)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "\n  \"version\": 1")
	loaded, err := LoadArtifact(path)
	require.NoError(t, err)
	require.Len(t, loaded.Records, 1)
	require.Empty(t, loaded.Records[0].RequestID)
}

func TestArtifactRejectsSensitiveFields(t *testing.T) {
	artifact := &Artifact{
		Version: ArtifactVersion,
		Kind:    "modelgateway_replay",
		Records: []Record{
			{RequestID: "req-should-not-export"},
		},
	}

	require.Error(t, artifact.Validate())
}
