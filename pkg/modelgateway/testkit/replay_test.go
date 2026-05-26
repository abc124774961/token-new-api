package testkit

import (
	"bytes"
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

func TestReplayExportSanitizesModelExecutionRecord(t *testing.T) {
	scoreBreakdown, err := common.Marshal(map[string]float64{"success": 0.8, "speed": 0.7})
	require.NoError(t, err)
	candidateGroups, err := common.Marshal([]string{"vip", "default"})
	require.NoError(t, err)
	requestMeta, err := common.Marshal(ReplayRequestMeta{
		OriginalModelName: "mimo-v1",
		UserUsingGroup:    "vip",
		PromptTokens:      1234,
		PreConsumedQuota:  5678,
		CandidateExplanations: []ReplayCandidateExplanation{
			{
				ChannelID:       901,
				ChannelName:     "https://api.secret.example.com/v1?key=abc",
				Group:           "default",
				UpstreamModel:   "mimo-v1",
				ProviderProfile: "mimo_codex_chat",
				ProxyMode:       "responses_via_chat",
				RuntimeKey: ReplayRuntimeKey{
					RequestedModel: "mimo-v1",
					UpstreamModel:  "mimo-v1",
					ChannelID:      901,
					Group:          "default",
					EndpointType:   string(constant.EndpointTypeOpenAIResponse),
				},
				Available:      true,
				ScoreTotal:     0.88,
				ScoreBreakdown: map[string]float64{"success": 0.8, "speed": 0.7},
				Selected:       true,
			},
			{
				ChannelID:     902,
				ChannelName:   "sk-secret-channel",
				Group:         "vip",
				UpstreamModel: "gpt-5.5",
				RuntimeKey: ReplayRuntimeKey{
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

	artifact, err := ExportReplayArtifact([]model.ModelExecutionRecord{
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
			EndpointType:      "openai-response",
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
	}, ReplayExportOptions{IncludeScenarios: true, StableIDs: true})

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
	require.Len(t, record.CandidateExplanations, 2)
	require.Empty(t, record.RequestMeta.CandidateExplanations)
	require.Equal(t, 1, record.CandidateExplanations[0].ChannelID)
	require.NotContains(t, record.CandidateExplanations[0].ChannelName, "secret.example.com")
	require.False(t, record.CandidateExplanations[1].Available)
	require.Equal(t, "concurrency_full", record.CandidateExplanations[1].RejectReason)
	require.Len(t, artifact.Scenarios, 1)
	require.Equal(t, 1, artifact.Scenarios[0].ExpectReplay.SelectedChannelID)
	require.True(t, artifact.Scenarios[0].ExpectReplay.StreamInterrupted)
	require.Len(t, artifact.Scenarios[0].ExpectReplay.CandidateExplanations, 2)
	require.Len(t, artifact.Scenarios[0].Dispatch.Expected.Candidates, 2)
	require.Equal(t, 1, artifact.Scenarios[0].Dispatch.Expected.Candidates[0].ChannelID)
}

func TestReplayArtifactLoadsFixtureAndRunsScenarios(t *testing.T) {
	artifact, err := LoadReplayArtifact(filepath.Join("..", "testdata", "replay", "model_execution_replay.json"))
	require.NoError(t, err)

	scenarios, err := ReplayScenarios(artifact)
	require.NoError(t, err)
	require.Len(t, scenarios, 1)

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.Name, func(t *testing.T) {
			runDispatchScenarioObject(t, &scenario)
		})
	}
}

func TestReplayScoreDriftReportSeparatesScoreChangeFromSelectionRegression(t *testing.T) {
	artifact, err := LoadReplayArtifact(filepath.Join("..", "testdata", "replay", "model_execution_replay.json"))
	require.NoError(t, err)
	require.NotEmpty(t, artifact.Records)
	artifact.Records[0].ScoreTotal = 0.42
	artifact.Records[0].ScoreBreakdown = map[string]float64{
		"success": 0.1,
		"speed":   0.1,
		"load":    0.1,
		"cost":    0.1,
		"group":   0.1,
	}

	reports, err := EvaluateReplayScoreDrift(artifact, ReplayScoreDriftOptions{
		ScoreTolerance:     0.01,
		BreakdownTolerance: 0.01,
	})
	require.NoError(t, err)
	require.Len(t, reports, 1)
	report := reports[0]
	require.True(t, report.SelectedStable)
	require.True(t, report.GroupStable)
	require.Equal(t, 101, report.ExpectedChannelID)
	require.Equal(t, 101, report.ActualChannelID)
	require.NotZero(t, report.ActualScoreTotal)
	require.True(t, report.ScoreDrifted)
	require.True(t, ReplayScoreDrifted(reports))

	breakdownDrifted := false
	for _, item := range report.Breakdown {
		if item.Drifted {
			breakdownDrifted = true
			break
		}
	}
	require.True(t, breakdownDrifted)
}

func TestReplayScoreDriftReportCanBeUsedAsExactGolden(t *testing.T) {
	artifact, err := LoadReplayArtifact(filepath.Join("..", "testdata", "replay", "model_execution_replay.json"))
	require.NoError(t, err)
	reports, err := EvaluateReplayScoreDrift(artifact, ReplayScoreDriftOptions{})
	require.NoError(t, err)
	require.Len(t, reports, 1)
	require.NotZero(t, reports[0].ActualScoreTotal)

	artifact.Records[0].ScoreTotal = reports[0].ActualScoreTotal
	artifact.Records[0].ScoreBreakdown = map[string]float64{}
	for _, item := range reports[0].Breakdown {
		artifact.Records[0].ScoreBreakdown[item.Key] = item.Actual
	}
	reports, err = EvaluateReplayScoreDrift(artifact, ReplayScoreDriftOptions{
		ScoreTolerance:     0.0001,
		BreakdownTolerance: 0.0001,
	})
	require.NoError(t, err)
	require.False(t, ReplayScoreDrifted(reports))
}

func TestRunReplayArtifactsSeparatesRegressionFromDrift(t *testing.T) {
	artifact, err := LoadReplayArtifact(filepath.Join("..", "testdata", "replay", "model_execution_replay.json"))
	require.NoError(t, err)

	report := RunReplayArtifacts(map[string]*ReplayArtifact{
		"model_execution_replay": artifact,
	}, ReplayBatchRunOptions{
		ReplayScoreDriftOptions: ReplayScoreDriftOptions{
			ScoreTolerance:     0.01,
			BreakdownTolerance: 0.01,
		},
	})
	require.Equal(t, 1, report.ArtifactCount)
	require.Equal(t, 1, report.ScenarioCount)
	require.False(t, report.HasBlockingRegression())
	require.True(t, report.HasNonBlockingDrift())
	require.Len(t, report.Artifacts, 1)
	require.Empty(t, report.Artifacts[0].Errors)
	require.True(t, report.Artifacts[0].NonBlockingDrift)
	require.Equal(t, ReplayBatchRunExitOK, report.ExitCode())
	require.Equal(t, "drifted", report.Status())
	require.Equal(t, 0, report.BlockingRegressionCount())
	require.Equal(t, 1, report.NonBlockingDriftCount())
	require.Equal(t, "replay batch: status=drifted exit_code=0 artifacts=1 scenarios=1 blocking_regressions=0 non_blocking_drifts=1", report.CLISummary())
}

func TestRunReplayArtifactReportsBlockingRegression(t *testing.T) {
	artifact, err := LoadReplayArtifact(filepath.Join("..", "testdata", "replay", "model_execution_replay.json"))
	require.NoError(t, err)
	scenario, ok := ToDispatchScenario(artifact.Records[0], "bad_expected_channel")
	require.True(t, ok)
	scenario.Expected.SelectedChannelID = 999
	artifact.Scenarios = []ReplayScenarioFixture{
		{
			Name:      "bad_expected_channel",
			RecordRef: 0,
			Dispatch:  scenario,
			ExpectReplay: ReplayExpectation{
				SelectedChannelID: 999,
				SelectedGroup:     scenario.Expected.SelectedGroup,
			},
		},
	}

	report := RunReplayArtifact("bad", artifact, ReplayBatchRunOptions{})
	require.True(t, report.BlockingRegression)
	require.False(t, report.NonBlockingDrift)
	require.NotEmpty(t, report.Errors)
	require.Contains(t, report.Errors[0], "selected channel mismatch")
}

func TestReplayBatchRunReportCIJSONUsesBlockingOnlyExitCode(t *testing.T) {
	report := ReplayBatchRunReport{
		Artifacts: []ReplayArtifactRunReport{
			{
				Name:               "blocking",
				ScenarioCount:      2,
				BlockingRegression: true,
				Errors:             []string{"selected channel mismatch"},
			},
			{
				Name:             "drift",
				ScenarioCount:    1,
				NonBlockingDrift: true,
			},
		},
		ArtifactCount:      2,
		ScenarioCount:      3,
		BlockingRegression: true,
		NonBlockingDrift:   true,
	}

	require.Equal(t, ReplayBatchRunExitBlockingRegression, report.ExitCode())
	require.Equal(t, "failed", report.Status())
	require.Equal(t, 1, report.BlockingRegressionCount())
	require.Equal(t, 1, report.NonBlockingDriftCount())

	data, err := report.MarshalCIReport()
	require.NoError(t, err)
	require.Contains(t, string(data), `"exit_code": 1`)
	require.Contains(t, string(data), `"blocking_regressions": 1`)
	require.Contains(t, string(data), `"non_blocking_drifts": 1`)

	var payload ReplayBatchRunCIReport
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "failed", payload.Status)
	require.Equal(t, ReplayBatchRunExitBlockingRegression, payload.ExitCode)
	require.Equal(t, 2, payload.ArtifactCount)
	require.Equal(t, 3, payload.ScenarioCount)
	require.Len(t, payload.Artifacts, 2)
}

func TestReplayBatchRunReportCIJSONKeepsScoreOnlyDriftNonBlocking(t *testing.T) {
	report := ReplayBatchRunReport{
		Artifacts: []ReplayArtifactRunReport{
			{
				Name:             "score_drift",
				ScenarioCount:    1,
				NonBlockingDrift: true,
			},
		},
		NonBlockingDrift: true,
	}

	ciReport := report.CIReport()
	require.Equal(t, "drifted", ciReport.Status)
	require.Equal(t, ReplayBatchRunExitOK, ciReport.ExitCode)
	require.Equal(t, 1, ciReport.ArtifactCount)
	require.Equal(t, 1, ciReport.ScenarioCount)
	require.Equal(t, 0, ciReport.BlockingRegressions)
	require.Equal(t, 1, ciReport.NonBlockingDrifts)
	require.Equal(t, "replay batch: status=drifted exit_code=0 artifacts=1 scenarios=1 blocking_regressions=0 non_blocking_drifts=1", report.CLISummary())
}

func TestReplayBatchRunCIScansGoldenPathsAndWritesReport(t *testing.T) {
	artifact, err := LoadReplayArtifact(filepath.Join("..", "testdata", "replay", "model_execution_replay.json"))
	require.NoError(t, err)

	root := t.TempDir()
	firstPath := filepath.Join(root, "model_execution_replay.json")
	secondPath := filepath.Join(root, "nested", "copy.json")
	require.NoError(t, WriteReplayArtifact(firstPath, artifact))
	require.NoError(t, WriteReplayArtifact(secondPath, artifact))

	scanned, err := ScanReplayGoldenPaths([]string{root, filepath.Join(root, "nested", "*.json"), firstPath})
	require.NoError(t, err)
	require.Equal(t, []string{firstPath, secondPath}, scanned)

	reportPath := filepath.Join(root, "reports", "replay-ci.json")
	report, err := RunReplayBatchCI(ReplayBatchRunCLIOptions{
		GoldenPaths: []string{root},
		ReportPath:  reportPath,
		RunOptions: ReplayBatchRunOptions{
			ReplayScoreDriftOptions: ReplayScoreDriftOptions{
				ScoreTolerance:     0.01,
				BreakdownTolerance: 0.01,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, ReplayBatchRunExitOK, report.ExitCode())
	require.Equal(t, "drifted", report.Status())
	require.Equal(t, 2, report.ArtifactCount)
	require.Equal(t, 2, report.ScenarioCount)
	require.Equal(t, 2, report.NonBlockingDriftCount())

	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	var payload ReplayBatchRunCIReport
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "drifted", payload.Status)
	require.Equal(t, ReplayBatchRunExitOK, payload.ExitCode)
	require.Equal(t, 2, payload.ArtifactCount)
	require.Equal(t, 2, payload.ScenarioCount)
	require.Equal(t, 2, payload.NonBlockingDrifts)
	require.Len(t, payload.Artifacts, 2)
}

func TestReplayBatchRunCLIUsesBlockingExitCode(t *testing.T) {
	artifact, err := LoadReplayArtifact(filepath.Join("..", "testdata", "replay", "model_execution_replay.json"))
	require.NoError(t, err)
	scenario, ok := ToDispatchScenario(artifact.Records[0], "blocking")
	require.True(t, ok)
	scenario.Expected.SelectedChannelID = 999
	artifact.Scenarios = []ReplayScenarioFixture{
		{
			Name:      "blocking",
			RecordRef: 0,
			Dispatch:  scenario,
			ExpectReplay: ReplayExpectation{
				SelectedChannelID: 999,
				SelectedGroup:     scenario.Expected.SelectedGroup,
			},
		},
	}

	root := t.TempDir()
	goldenPath := filepath.Join(root, "blocking.json")
	reportPath := filepath.Join(root, "report.json")
	require.NoError(t, WriteReplayArtifact(goldenPath, artifact))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := RunReplayBatchCLI([]string{
		"-golden", goldenPath,
		"-report", reportPath,
	}, &stdout, &stderr)
	require.Equal(t, ReplayBatchRunExitBlockingRegression, exitCode)
	require.Contains(t, stdout.String(), "status=failed")
	require.Empty(t, stderr.String())

	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	var payload ReplayBatchRunCIReport
	require.NoError(t, common.Unmarshal(data, &payload))
	require.Equal(t, "failed", payload.Status)
	require.Equal(t, ReplayBatchRunExitBlockingRegression, payload.ExitCode)
	require.Equal(t, 1, payload.BlockingRegressions)
	require.Len(t, payload.Artifacts, 1)
	require.Contains(t, payload.Artifacts[0].Errors[0], "selected channel mismatch")
}

func TestReplayArtifactRejectsSensitiveFields(t *testing.T) {
	artifact := &ReplayArtifact{
		Version: ReplayArtifactVersion,
		Kind:    "modelgateway_replay",
		Records: []ReplayRecord{
			{
				RequestID:      "req-should-not-export",
				RequestedGroup: "default",
			},
		},
	}

	require.Error(t, artifact.Validate())
}

func TestReplayArtifactRejectsInvalidScenarioRef(t *testing.T) {
	artifact := &ReplayArtifact{
		Version: ReplayArtifactVersion,
		Kind:    "modelgateway_replay",
		Records: []ReplayRecord{
			{RequestedGroup: "default", RequestedModel: "gpt-5.5", ChannelID: 1},
		},
		Scenarios: []ReplayScenarioFixture{
			{
				Name:      "bad_ref",
				RecordRef: 99,
				Dispatch:  DispatchScenario{Name: "bad_ref"},
			},
		},
	}

	require.Error(t, artifact.Validate())
}

func TestReplayExporterAggregatesByRequestID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))

	scoreBreakdown, err := common.Marshal(map[string]float64{"success": 0.92, "speed": 0.7})
	require.NoError(t, err)
	candidateGroups, err := common.Marshal([]string{"default", "vip"})
	require.NoError(t, err)
	requestMeta, err := common.Marshal(ReplayRequestMeta{
		OriginalModelName: "mimo-v1",
		UserUsingGroup:    "vip",
		PromptTokens:      999,
		PreConsumedQuota:  888,
		CandidateExplanations: []ReplayCandidateExplanation{
			{
				ChannelID:       101,
				ChannelName:     "https://upstream.example.com/v1?api_key=secret",
				Group:           "vip",
				UpstreamModel:   "mimo-v1",
				ProviderProfile: "mimo_codex_chat",
				ProxyMode:       "responses_via_chat",
				RuntimeKey: ReplayRuntimeKey{
					RequestedModel: "mimo-v1",
					UpstreamModel:  "mimo-v1",
					ChannelID:      101,
					Group:          "vip",
					EndpointType:   string(constant.EndpointTypeOpenAIResponse),
				},
				Available:      true,
				ScoreTotal:     0.81,
				ScoreBreakdown: map[string]float64{"success": 0.92, "speed": 0.7, "cost": 1, "group": 1},
				Selected:       true,
			},
			{
				ChannelID:     202,
				ChannelName:   "fallback",
				Group:         "default",
				UpstreamModel: "gpt-5.5",
				RuntimeKey: ReplayRuntimeKey{
					RequestedModel: "mimo-v1",
					UpstreamModel:  "gpt-5.5",
					ChannelID:      202,
					Group:          "default",
					EndpointType:   string(constant.EndpointTypeOpenAIResponse),
				},
				Available:    false,
				RejectReason: "concurrency_full",
			},
		},
	})
	require.NoError(t, err)

	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:       20,
		RequestId:       "req-other",
		RequestedGroup:  "default",
		RequestedModel:  "gpt-5.5",
		ChannelId:       90,
		EndpointType:    string(constant.EndpointTypeOpenAI),
		PolicyMode:      core.ModeActive,
		CandidateGroups: string(candidateGroups),
	}).Error)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:       10,
		RequestId:       "req-replay",
		RequestedGroup:  "auto",
		SelectedGroup:   "vip",
		RequestedModel:  "mimo-v1",
		ChannelId:       101,
		ChannelName:     "https://upstream.example.com/v1?api_key=secret",
		EndpointType:    string(constant.EndpointTypeOpenAIResponse),
		PolicyMode:      core.ModeActive,
		AutoMode:        core.AutoModeFusion,
		Strategy:        core.StrategyBalanced,
		SmartHandled:    true,
		ScoreTotal:      0.81,
		ScoreBreakdown:  string(scoreBreakdown),
		CandidateGroups: string(candidateGroups),
		RequestMeta:     string(requestMeta),
	}).Error)
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:         30,
		RequestId:         "req-replay",
		AttemptIndex:      1,
		RequestedGroup:    "auto",
		SelectedGroup:     "vip",
		RequestedModel:    "mimo-v1",
		ChannelId:         101,
		EndpointType:      string(constant.EndpointTypeOpenAIResponse),
		Success:           false,
		StatusCode:        502,
		ErrorCode:         "upstream_eof",
		ErrorType:         "stream",
		DurationMs:        1800,
		TTFTMs:            120,
		StreamInterrupted: true,
	}).Error)

	exporter := NewReplayArtifactExporter(
		NewGormReplayRecordRepository(db),
		ReplayExportOptions{IncludeScenarios: true, StableIDs: true},
	)
	artifact, err := exporter.ExportByRequestID("req-replay")
	require.NoError(t, err)
	require.NoError(t, artifact.Validate())
	require.Len(t, artifact.Records, 2)
	require.Equal(t, "auto", artifact.Records[0].RequestedGroup)
	require.Equal(t, 1, artifact.Records[0].ChannelID)
	require.Empty(t, artifact.Records[0].RequestID)
	require.Zero(t, artifact.Records[0].RequestMeta.PromptTokens)
	require.NotContains(t, artifact.Records[0].ChannelName, "api_key")
	require.Len(t, artifact.Records[0].CandidateExplanations, 2)
	require.False(t, artifact.Records[0].CandidateExplanations[1].Available)
	require.Equal(t, "concurrency_full", artifact.Records[0].CandidateExplanations[1].RejectReason)
	require.True(t, artifact.Records[1].StreamInterrupted)
	require.Len(t, artifact.Scenarios, 2)
	require.Len(t, artifact.Scenarios[0].Dispatch.Channels, 2)
	require.Len(t, artifact.Scenarios[0].Dispatch.Expected.Candidates, 2)
	require.Equal(t, 1, artifact.Scenarios[0].Dispatch.Expected.SelectedChannelID)
}

func TestReplayExporterWritesGoldenArtifact(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelExecutionRecord{}))
	require.NoError(t, model.EnsureModelExecutionRecordRequestMetaCapacity(db))
	require.NoError(t, db.Create(&model.ModelExecutionRecord{
		CreatedAt:       10,
		RequestId:       "req-golden",
		RequestedGroup:  "default",
		SelectedGroup:   "default",
		RequestedModel:  "gpt-5.5",
		ChannelId:       8,
		EndpointType:    string(constant.EndpointTypeOpenAI),
		PolicyMode:      core.ModeActive,
		Strategy:        core.StrategyBalanced,
		SmartHandled:    true,
		Success:         true,
		CandidateGroups: `["default"]`,
	}).Error)

	exporter := NewReplayArtifactExporter(
		NewGormReplayRecordRepository(db),
		ReplayExportOptions{IncludeScenarios: true},
	)
	path := filepath.Join(t.TempDir(), "replay", "req-golden.json")
	artifact, err := exporter.WriteGoldenByRequestID("req-golden", path)
	require.NoError(t, err)
	require.Len(t, artifact.Records, 1)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "\n  \"version\": 1")
	loaded, err := LoadReplayArtifact(path)
	require.NoError(t, err)
	require.Len(t, loaded.Records, 1)
	require.Empty(t, loaded.Records[0].RequestID)
}
