package testkit

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/replay"
	"gorm.io/gorm"
)

const ReplayArtifactVersion = replay.ArtifactVersion

const (
	ReplayBatchRunExitOK                 = 0
	ReplayBatchRunExitBlockingRegression = 1
)

type ReplayArtifact struct {
	Version   int                     `json:"version"`
	Kind      string                  `json:"kind"`
	Records   []replay.Record         `json:"records"`
	Scenarios []ReplayScenarioFixture `json:"scenarios,omitempty"`
}

type ReplayRecord = replay.Record
type ReplayRequestMeta = replay.RequestMeta
type ReplayCandidateExplanation = replay.CandidateExplanation
type ReplayRuntimeKey = replay.RuntimeKey
type ReplayExportOptions struct {
	IncludeScenarios bool
	StableIDs        bool
}
type ReplayRecordRepository = replay.RecordRepository
type GormReplayRecordRepository = replay.GormRecordRepository

type ReplayScenarioFixture struct {
	Name         string            `json:"name"`
	RecordRef    int               `json:"record_ref"`
	Dispatch     DispatchScenario  `json:"dispatch"`
	ExpectReplay ReplayExpectation `json:"expect_replay"`
}

type ReplayExpectation struct {
	SelectedChannelID     int                           `json:"selected_channel_id,omitempty"`
	SelectedGroup         string                        `json:"selected_group,omitempty"`
	ScoreTotal            float64                       `json:"score_total,omitempty"`
	ScoreBreakdown        map[string]float64            `json:"score_breakdown,omitempty"`
	SelectedReason        string                        `json:"selected_reason,omitempty"`
	StreamInterrupted     bool                          `json:"stream_interrupted,omitempty"`
	CandidateExplanations []replay.CandidateExplanation `json:"candidate_explanations,omitempty"`
}

type ReplayScoreDriftOptions struct {
	ScoreTolerance     float64
	BreakdownTolerance float64
}

type ReplayScoreDriftReport struct {
	ScenarioName       string                      `json:"scenario_name"`
	RecordRef          int                         `json:"record_ref"`
	SelectedStable     bool                        `json:"selected_stable"`
	GroupStable        bool                        `json:"group_stable"`
	ExpectedChannelID  int                         `json:"expected_channel_id,omitempty"`
	ActualChannelID    int                         `json:"actual_channel_id,omitempty"`
	ExpectedGroup      string                      `json:"expected_group,omitempty"`
	ActualGroup        string                      `json:"actual_group,omitempty"`
	ExpectedScoreTotal float64                     `json:"expected_score_total,omitempty"`
	ActualScoreTotal   float64                     `json:"actual_score_total,omitempty"`
	ScoreDelta         float64                     `json:"score_delta,omitempty"`
	ScoreDrifted       bool                        `json:"score_drifted,omitempty"`
	Breakdown          []ReplayScoreBreakdownDrift `json:"breakdown,omitempty"`
}

type ReplayScoreBreakdownDrift struct {
	Key      string  `json:"key"`
	Expected float64 `json:"expected,omitempty"`
	Actual   float64 `json:"actual,omitempty"`
	Delta    float64 `json:"delta,omitempty"`
	Drifted  bool    `json:"drifted,omitempty"`
}

type ReplayBatchRunOptions struct {
	ReplayScoreDriftOptions
}

type ReplayArtifactRunReport struct {
	Name               string                   `json:"name,omitempty"`
	ScenarioCount      int                      `json:"scenario_count"`
	BlockingRegression bool                     `json:"blocking_regression"`
	NonBlockingDrift   bool                     `json:"non_blocking_drift"`
	Errors             []string                 `json:"errors,omitempty"`
	ScoreDriftReports  []ReplayScoreDriftReport `json:"score_drift_reports,omitempty"`
}

type ReplayBatchRunReport struct {
	Artifacts          []ReplayArtifactRunReport `json:"artifacts"`
	ArtifactCount      int                       `json:"artifact_count"`
	ScenarioCount      int                       `json:"scenario_count"`
	BlockingRegression bool                      `json:"blocking_regression"`
	NonBlockingDrift   bool                      `json:"non_blocking_drift"`
}

// ReplayBatchRunCIReport is the stable JSON shape for replay batch runner output.
// BlockingRegressions counts artifact reports with blocking regressions; any count
// above zero maps to ExitCode 1. NonBlockingDrifts counts artifact reports with
// score-only drift and keeps ExitCode 0.
type ReplayBatchRunCIReport struct {
	Status              string                    `json:"status"`
	ExitCode            int                       `json:"exit_code"`
	ArtifactCount       int                       `json:"artifact_count"`
	ScenarioCount       int                       `json:"scenario_count"`
	BlockingRegressions int                       `json:"blocking_regressions"`
	NonBlockingDrifts   int                       `json:"non_blocking_drifts"`
	Artifacts           []ReplayArtifactRunReport `json:"artifacts"`
}

func (r ReplayScoreDriftReport) HasDrift() bool {
	if !r.SelectedStable || !r.GroupStable || r.ScoreDrifted {
		return true
	}
	for _, item := range r.Breakdown {
		if item.Drifted {
			return true
		}
	}
	return false
}

func ReplayScoreDrifted(reports []ReplayScoreDriftReport) bool {
	for _, report := range reports {
		if report.HasDrift() {
			return true
		}
	}
	return false
}

func replayScoreOnlyDrifted(reports []ReplayScoreDriftReport) bool {
	for _, report := range reports {
		if !report.SelectedStable || !report.GroupStable {
			continue
		}
		if report.ScoreDrifted {
			return true
		}
		for _, item := range report.Breakdown {
			if item.Drifted {
				return true
			}
		}
	}
	return false
}

func (r ReplayBatchRunReport) HasBlockingRegression() bool {
	return r.BlockingRegression || r.BlockingRegressionCount() > 0
}

func (r ReplayBatchRunReport) HasNonBlockingDrift() bool {
	return r.NonBlockingDrift || r.NonBlockingDriftCount() > 0
}

func (r ReplayBatchRunReport) BlockingRegressionCount() int {
	count := 0
	for _, artifact := range r.Artifacts {
		if artifact.BlockingRegression {
			count++
		}
	}
	if count == 0 && r.BlockingRegression {
		return 1
	}
	return count
}

func (r ReplayBatchRunReport) NonBlockingDriftCount() int {
	count := 0
	for _, artifact := range r.Artifacts {
		if artifact.NonBlockingDrift {
			count++
		}
	}
	if count == 0 && r.NonBlockingDrift {
		return 1
	}
	return count
}

func (r ReplayBatchRunReport) ExitCode() int {
	if r.HasBlockingRegression() {
		return ReplayBatchRunExitBlockingRegression
	}
	return ReplayBatchRunExitOK
}

func (r ReplayBatchRunReport) Status() string {
	if r.HasBlockingRegression() {
		return "failed"
	}
	if r.HasNonBlockingDrift() {
		return "drifted"
	}
	return "passed"
}

func (r ReplayBatchRunReport) CIReport() ReplayBatchRunCIReport {
	artifactCount := r.ArtifactCount
	if artifactCount == 0 {
		artifactCount = len(r.Artifacts)
	}
	scenarioCount := r.ScenarioCount
	if scenarioCount == 0 {
		for _, artifact := range r.Artifacts {
			scenarioCount += artifact.ScenarioCount
		}
	}
	return ReplayBatchRunCIReport{
		Status:              r.Status(),
		ExitCode:            r.ExitCode(),
		ArtifactCount:       artifactCount,
		ScenarioCount:       scenarioCount,
		BlockingRegressions: r.BlockingRegressionCount(),
		NonBlockingDrifts:   r.NonBlockingDriftCount(),
		Artifacts:           append([]ReplayArtifactRunReport(nil), r.Artifacts...),
	}
}

func (r ReplayBatchRunReport) MarshalCIReport() ([]byte, error) {
	return common.MarshalIndent(r.CIReport(), "", "  ")
}

func (r ReplayBatchRunReport) CLISummary() string {
	report := r.CIReport()
	return fmt.Sprintf(
		"replay batch: status=%s exit_code=%d artifacts=%d scenarios=%d blocking_regressions=%d non_blocking_drifts=%d",
		report.Status,
		report.ExitCode,
		report.ArtifactCount,
		report.ScenarioCount,
		report.BlockingRegressions,
		report.NonBlockingDrifts,
	)
}

type ReplayArtifactExporter struct {
	repository ReplayRecordRepository
	options    ReplayExportOptions
}

func NewGormReplayRecordRepository(db *gorm.DB) *GormReplayRecordRepository {
	return replay.NewGormRecordRepository(db)
}

func NewReplayArtifactExporter(repository ReplayRecordRepository, options ReplayExportOptions) *ReplayArtifactExporter {
	return &ReplayArtifactExporter{
		repository: repository,
		options:    options,
	}
}

func (e *ReplayArtifactExporter) ExportByRequestID(requestID string) (*ReplayArtifact, error) {
	if e == nil || e.repository == nil {
		return nil, fmt.Errorf("replay artifact exporter has no repository")
	}
	artifact, err := replay.NewArtifactExporter(e.repository, replay.ExportOptions{StableIDs: e.options.StableIDs}).ExportByRequestID(requestID)
	if err != nil {
		return nil, err
	}
	return BuildReplayArtifact(artifact, e.options)
}

func (e *ReplayArtifactExporter) WriteGoldenByRequestID(requestID string, path string) (*ReplayArtifact, error) {
	artifact, err := e.ExportByRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if err := WriteReplayArtifact(path, artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

func ExportReplayArtifact(records []model.ModelExecutionRecord, options ReplayExportOptions) (*ReplayArtifact, error) {
	artifact, err := replay.ExportArtifact(records, replay.ExportOptions{StableIDs: options.StableIDs})
	if err != nil {
		return nil, err
	}
	return BuildReplayArtifact(artifact, options)
}

func BuildReplayArtifact(artifact *replay.Artifact, options ReplayExportOptions) (*ReplayArtifact, error) {
	if artifact == nil {
		return nil, fmt.Errorf("replay artifact is nil")
	}
	out := &ReplayArtifact{
		Version: artifact.Version,
		Kind:    artifact.Kind,
		Records: append([]replay.Record(nil), artifact.Records...),
	}
	if options.IncludeScenarios {
		for idx, record := range out.Records {
			scenario, ok := ToDispatchScenario(record, fmt.Sprintf("replay_%03d", idx+1))
			if ok {
				out.Scenarios = append(out.Scenarios, ReplayScenarioFixture{
					Name:      fmt.Sprintf("replay_%03d", idx+1),
					RecordRef: idx,
					Dispatch:  scenario,
					ExpectReplay: ReplayExpectation{
						SelectedChannelID:     record.ChannelID,
						SelectedGroup:         record.SelectedGroup,
						ScoreTotal:            record.ScoreTotal,
						ScoreBreakdown:        copyFloatMap(record.ScoreBreakdown),
						SelectedReason:        record.SelectedReason,
						StreamInterrupted:     record.StreamInterrupted,
						CandidateExplanations: append([]replay.CandidateExplanation(nil), record.CandidateExplanations...),
					},
				})
			}
		}
	}
	if err := out.Validate(); err != nil {
		return nil, err
	}
	return out, nil
}

func WriteReplayArtifact(path string, artifact *ReplayArtifact) error {
	if artifact == nil {
		return fmt.Errorf("replay artifact is nil")
	}
	if err := artifact.Validate(); err != nil {
		return err
	}
	data, err := replay.MarshalArtifact(&replay.Artifact{
		Version: artifact.Version,
		Kind:    artifact.Kind,
		Records: artifact.Records,
	})
	if err != nil {
		return err
	}
	data, err = injectScenarios(data, artifact.Scenarios)
	if err != nil {
		return err
	}
	return writeReplayFixture(path, data)
}

func LoadReplayArtifact(path string) (*ReplayArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact ReplayArtifact
	if err := common.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}
	if err := artifact.Validate(); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func (a *ReplayArtifact) Validate() error {
	if a == nil {
		return fmt.Errorf("replay artifact is nil")
	}
	if err := (&replay.Artifact{
		Version: a.Version,
		Kind:    a.Kind,
		Records: a.Records,
	}).Validate(); err != nil {
		return err
	}
	for idx, scenario := range a.Scenarios {
		if scenario.RecordRef < 0 || scenario.RecordRef >= len(a.Records) {
			return fmt.Errorf("scenario %d has invalid record_ref %d", idx, scenario.RecordRef)
		}
		if strings.TrimSpace(scenario.Dispatch.Name) == "" {
			return fmt.Errorf("scenario %d has empty dispatch name", idx)
		}
		if scenario.Dispatch.Expected.Handled && scenario.Dispatch.Expected.SelectedChannelID <= 0 {
			return fmt.Errorf("scenario %d has empty expected selected channel", idx)
		}
	}
	return nil
}

func SanitizeModelExecutionRecord(record model.ModelExecutionRecord, index int, options ReplayExportOptions) (ReplayRecord, error) {
	return replay.SanitizeModelExecutionRecord(record, index, replay.ExportOptions{StableIDs: options.StableIDs})
}

func ToDispatchScenario(r replay.Record, name string) (DispatchScenario, bool) {
	if r.RequestedModel == "" || r.RequestedGroup == "" || r.ChannelID <= 0 {
		return DispatchScenario{}, false
	}
	selectedGroup := firstNonEmptyReplay(r.SelectedGroup, r.RequestedGroup)
	candidateGroups := append([]string(nil), r.CandidateGroups...)
	if len(candidateGroups) == 0 {
		candidateGroups = []string{selectedGroup}
	}
	if !containsString(candidateGroups, selectedGroup) {
		candidateGroups = append(candidateGroups, selectedGroup)
	}
	channels := channelsFromReplayRecord(r, selectedGroup, candidateGroups)
	runtimeSnapshots := runtimeSnapshotsFromReplayRecord(r, selectedGroup)
	scenario := DispatchScenario{
		Name: strings.TrimSpace(name),
		Request: DispatchRequestFixture{
			UserGroup:      firstNonEmptyReplay(r.RequestMeta.UserUsingGroup, r.RequestedGroup),
			RequestedGroup: r.RequestedGroup,
			ModelName:      r.RequestedModel,
		},
		Policy: GroupPolicyFixture{
			Mode:             firstNonEmptyReplay(r.PolicyMode, core.ModeActive),
			Strategy:         firstNonEmptyReplay(r.Strategy, core.StrategyBalanced),
			AutoMode:         r.AutoMode,
			CrossGroupFusion: len(candidateGroups) > 1 && r.RequestedGroup != "auto",
			CandidateGroups:  candidateGroups,
		},
		UsableGroups:     candidateGroups,
		Channels:         channels,
		RuntimeSnapshots: runtimeSnapshots,
		Expected: DispatchExpected{
			Handled:           firstNonEmptyReplay(r.PolicyMode, core.ModeActive) == core.ModeActive,
			SelectedChannelID: r.ChannelID,
			SelectedGroup:     selectedGroup,
			FallbackUsed:      r.FallbackUsed,
			ScoreBreakdown:    r.ScoreBreakdown,
			Candidates:        candidateExpectationsFromReplayRecord(r),
		},
	}
	if scenario.Name == "" {
		scenario.Name = "replay"
	}
	return scenario, true
}

func ReplayScenarios(a *ReplayArtifact) ([]DispatchScenario, error) {
	if a == nil {
		return nil, fmt.Errorf("replay artifact is nil")
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if len(a.Scenarios) > 0 {
		scenarios := make([]DispatchScenario, 0, len(a.Scenarios))
		for _, fixture := range a.Scenarios {
			scenarios = append(scenarios, fixture.Dispatch)
		}
		return scenarios, nil
	}
	scenarios := make([]DispatchScenario, 0, len(a.Records))
	for idx, record := range a.Records {
		scenario, ok := ToDispatchScenario(record, fmt.Sprintf("replay_%03d", idx+1))
		if ok {
			scenarios = append(scenarios, scenario)
		}
	}
	return scenarios, nil
}

func RunReplayArtifact(name string, artifact *ReplayArtifact, options ReplayBatchRunOptions) ReplayArtifactRunReport {
	report := ReplayArtifactRunReport{Name: strings.TrimSpace(name)}
	scenarios, err := ReplayScenarios(artifact)
	if err != nil {
		report.BlockingRegression = true
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	report.ScenarioCount = len(scenarios)
	for idx := range scenarios {
		scenario := scenarios[idx]
		plan, handled, err := ExecuteDispatchScenario(&scenario)
		if err != nil {
			report.BlockingRegression = true
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", scenario.Name, err.Error()))
			continue
		}
		if handled != scenario.Expected.Handled {
			report.BlockingRegression = true
			report.Errors = append(report.Errors, fmt.Sprintf("%s: handled mismatch expected=%t actual=%t", scenario.Name, scenario.Expected.Handled, handled))
			continue
		}
		if !scenario.Expected.Handled {
			if plan != nil {
				report.BlockingRegression = true
				report.Errors = append(report.Errors, fmt.Sprintf("%s: expected no plan", scenario.Name))
			}
			continue
		}
		if plan == nil || plan.Channel == nil {
			report.BlockingRegression = true
			report.Errors = append(report.Errors, fmt.Sprintf("%s: expected selected channel %d but plan is empty", scenario.Name, scenario.Expected.SelectedChannelID))
			continue
		}
		if plan.Channel.Id != scenario.Expected.SelectedChannelID {
			report.BlockingRegression = true
			report.Errors = append(report.Errors, fmt.Sprintf("%s: selected channel mismatch expected=%d actual=%d", scenario.Name, scenario.Expected.SelectedChannelID, plan.Channel.Id))
		}
		if plan.SelectedGroup != scenario.Expected.SelectedGroup {
			report.BlockingRegression = true
			report.Errors = append(report.Errors, fmt.Sprintf("%s: selected group mismatch expected=%s actual=%s", scenario.Name, scenario.Expected.SelectedGroup, plan.SelectedGroup))
		}
	}
	scoreReports, err := EvaluateReplayScoreDrift(artifact, options.ReplayScoreDriftOptions)
	if err != nil {
		report.BlockingRegression = true
		report.Errors = append(report.Errors, err.Error())
		return report
	}
	report.ScoreDriftReports = scoreReports
	report.NonBlockingDrift = replayScoreOnlyDrifted(scoreReports)
	return report
}

func RunReplayArtifacts(artifacts map[string]*ReplayArtifact, options ReplayBatchRunOptions) ReplayBatchRunReport {
	report := ReplayBatchRunReport{
		Artifacts: make([]ReplayArtifactRunReport, 0, len(artifacts)),
	}
	names := make([]string, 0, len(artifacts))
	for name := range artifacts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		artifact := artifacts[name]
		artifactReport := RunReplayArtifact(name, artifact, options)
		report.Artifacts = append(report.Artifacts, artifactReport)
		report.ArtifactCount++
		report.ScenarioCount += artifactReport.ScenarioCount
		if artifactReport.BlockingRegression {
			report.BlockingRegression = true
		}
		if artifactReport.NonBlockingDrift {
			report.NonBlockingDrift = true
		}
	}
	return report
}

func EvaluateReplayScoreDrift(a *ReplayArtifact, options ReplayScoreDriftOptions) ([]ReplayScoreDriftReport, error) {
	if a == nil {
		return nil, fmt.Errorf("replay artifact is nil")
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	scoreTolerance := options.ScoreTolerance
	if scoreTolerance <= 0 {
		scoreTolerance = 0.05
	}
	breakdownTolerance := options.BreakdownTolerance
	if breakdownTolerance <= 0 {
		breakdownTolerance = 0.08
	}
	fixtures := a.Scenarios
	if len(fixtures) == 0 {
		for idx, record := range a.Records {
			scenario, ok := ToDispatchScenario(record, fmt.Sprintf("replay_%03d", idx+1))
			if !ok {
				continue
			}
			fixtures = append(fixtures, ReplayScenarioFixture{
				Name:      scenario.Name,
				RecordRef: idx,
				Dispatch:  scenario,
				ExpectReplay: ReplayExpectation{
					SelectedChannelID:     record.ChannelID,
					SelectedGroup:         record.SelectedGroup,
					ScoreTotal:            record.ScoreTotal,
					ScoreBreakdown:        copyFloatMap(record.ScoreBreakdown),
					SelectedReason:        record.SelectedReason,
					StreamInterrupted:     record.StreamInterrupted,
					CandidateExplanations: append([]replay.CandidateExplanation(nil), record.CandidateExplanations...),
				},
			})
		}
	}
	reports := make([]ReplayScoreDriftReport, 0, len(fixtures))
	for _, fixture := range fixtures {
		plan, handled, err := ExecuteDispatchScenario(&fixture.Dispatch)
		if err != nil {
			return nil, err
		}
		report := ReplayScoreDriftReport{
			ScenarioName:       firstNonEmptyReplay(fixture.Name, fixture.Dispatch.Name),
			RecordRef:          fixture.RecordRef,
			ExpectedChannelID:  fixture.ExpectReplay.SelectedChannelID,
			ExpectedGroup:      fixture.ExpectReplay.SelectedGroup,
			ExpectedScoreTotal: fixture.ExpectReplay.ScoreTotal,
		}
		if handled && plan != nil {
			if plan.Channel != nil {
				report.ActualChannelID = plan.Channel.Id
			}
			report.ActualGroup = plan.SelectedGroup
			report.ActualScoreTotal = plan.ScoreTotal
			report.Breakdown = scoreBreakdownDrift(fixture.ExpectReplay.ScoreBreakdown, plan.ScoreBreakdown, breakdownTolerance)
		}
		report.SelectedStable = report.ExpectedChannelID == 0 || report.ExpectedChannelID == report.ActualChannelID
		report.GroupStable = report.ExpectedGroup == "" || report.ExpectedGroup == report.ActualGroup
		report.ScoreDelta = roundReplayDelta(report.ActualScoreTotal - report.ExpectedScoreTotal)
		report.ScoreDrifted = report.ExpectedScoreTotal > 0 && math.Abs(report.ScoreDelta) > scoreTolerance
		reports = append(reports, report)
	}
	return reports, nil
}

func channelsFromReplayRecord(r replay.Record, selectedGroup string, candidateGroups []string) []ChannelFixture {
	channels := make([]ChannelFixture, 0, len(r.CandidateExplanations)+len(candidateGroups)+1)
	seenChannels := map[int]struct{}{}
	seenGroups := map[string]struct{}{}
	for _, candidate := range r.CandidateExplanations {
		channelID := candidate.ChannelID
		if channelID <= 0 {
			channelID = candidate.RuntimeKey.ChannelID
		}
		if channelID <= 0 {
			continue
		}
		if _, ok := seenChannels[channelID]; ok {
			continue
		}
		group := firstNonEmptyReplay(candidate.Group, candidate.RuntimeKey.Group, selectedGroup)
		channels = append(channels, ChannelFixture{
			ID:    channelID,
			Group: group,
			Name:  firstNonEmptyReplay(candidate.ChannelName, fmt.Sprintf("candidate_channel_%d", channelID)),
		})
		seenChannels[channelID] = struct{}{}
		seenGroups[group] = struct{}{}
	}
	if _, ok := seenChannels[r.ChannelID]; !ok {
		channels = append(channels, ChannelFixture{
			ID:    r.ChannelID,
			Group: selectedGroup,
			Name:  firstNonEmptyReplay(r.ChannelName, fmt.Sprintf("channel_%d", r.ChannelID)),
		})
		seenChannels[r.ChannelID] = struct{}{}
		seenGroups[selectedGroup] = struct{}{}
	}
	for idx, group := range candidateGroups {
		if group == selectedGroup {
			continue
		}
		if _, ok := seenGroups[group]; ok {
			continue
		}
		channelID := replaySyntheticChannelID(r.ChannelID, idx)
		if _, ok := seenChannels[channelID]; ok {
			continue
		}
		channels = append(channels, ChannelFixture{
			ID:    channelID,
			Group: group,
			Name:  fmt.Sprintf("replay_%s_%d", group, channelID),
		})
		seenChannels[channelID] = struct{}{}
		seenGroups[group] = struct{}{}
	}
	return channels
}

func runtimeSnapshotsFromReplayRecord(r replay.Record, selectedGroup string) []RuntimeSnapshotFixture {
	if len(r.CandidateExplanations) == 0 {
		return []RuntimeSnapshotFixture{
			{
				ChannelID:          r.ChannelID,
				Group:              selectedGroup,
				SuccessRate:        replaySuccessRate(r),
				TTFTMs:             float64(r.TTFTMs),
				DurationMs:         float64(r.DurationMs),
				CostRatio:          1,
				GroupPriorityRatio: 1,
				SampleCount:        1,
			},
		}
	}
	snapshots := make([]RuntimeSnapshotFixture, 0, len(r.CandidateExplanations))
	for _, candidate := range r.CandidateExplanations {
		channelID := candidate.ChannelID
		if channelID <= 0 {
			channelID = candidate.RuntimeKey.ChannelID
		}
		if channelID <= 0 {
			continue
		}
		snapshot := RuntimeSnapshotFixture{
			ChannelID:          channelID,
			Group:              firstNonEmptyReplay(candidate.Group, candidate.RuntimeKey.Group, selectedGroup),
			SuccessRate:        replayCandidateMetric(candidate.ScoreBreakdown, "completion_rate", "success", 0.80),
			TTFTMs:             float64(r.TTFTMs),
			DurationMs:         float64(r.DurationMs),
			CostRatio:          replayCandidateCostRatio(candidate.ScoreBreakdown),
			GroupPriorityRatio: replayCandidateMetric(candidate.ScoreBreakdown, "group_priority", "group", 1),
			SampleCount:        1,
		}
		if !candidate.Available {
			switch candidate.RejectReason {
			case "circuit_open":
				snapshot.CircuitOpen = true
			case "cooldown":
				snapshot.Cooldown = true
			case "failure_avoidance":
				snapshot.FailureAvoidance = true
			case "concurrency_full":
				snapshot.ActiveConcurrency = 1
				snapshot.MaxConcurrency = 1
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func candidateExpectationsFromReplayRecord(r replay.Record) []CandidateExpectation {
	if len(r.CandidateExplanations) == 0 {
		return nil
	}
	expectations := make([]CandidateExpectation, 0, len(r.CandidateExplanations))
	for _, candidate := range r.CandidateExplanations {
		channelID := candidate.ChannelID
		if channelID <= 0 {
			channelID = candidate.RuntimeKey.ChannelID
		}
		if channelID <= 0 {
			continue
		}
		available := candidate.Available
		selected := candidate.Selected
		expectations = append(expectations, CandidateExpectation{
			ChannelID:    channelID,
			Available:    &available,
			RejectReason: candidate.RejectReason,
			Selected:     &selected,
		})
	}
	return expectations
}

func replayCandidateMetric(values map[string]float64, primaryKey string, legacyKey string, fallback float64) float64 {
	for _, key := range []string{primaryKey, legacyKey} {
		if value, ok := values[key]; ok && value > 0 {
			return value
		}
	}
	return fallback
}

func replayCandidateCostRatio(values map[string]float64) float64 {
	score := replayCandidateMetric(values, "cost", "", 1)
	if score <= 0 {
		return 1
	}
	return 1 / score
}

func replaySyntheticChannelID(selectedID int, index int) int {
	base := selectedID + 100 + index
	if base <= 0 {
		base = 100 + index
	}
	return base
}

func replaySuccessRate(record replay.Record) float64 {
	return 0.95
}

func scoreBreakdownDrift(expected map[string]float64, actual map[string]float64, tolerance float64) []ReplayScoreBreakdownDrift {
	keys := make([]string, 0, len(expected)+len(actual))
	seen := map[string]struct{}{}
	for key := range expected {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range actual {
		if _, ok := seen[key]; ok {
			continue
		}
		keys = append(keys, key)
	}
	out := make([]ReplayScoreBreakdownDrift, 0, len(keys))
	for _, key := range keys {
		item := ReplayScoreBreakdownDrift{
			Key:      key,
			Expected: expected[key],
			Actual:   actual[key],
		}
		item.Delta = roundReplayDelta(item.Actual - item.Expected)
		item.Drifted = math.Abs(item.Delta) > tolerance
		out = append(out, item)
	}
	return out
}

func roundReplayDelta(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func copyFloatMap(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstNonEmptyReplay(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func injectScenarios(data []byte, scenarios []ReplayScenarioFixture) ([]byte, error) {
	if len(scenarios) == 0 {
		return data, nil
	}
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	payload["scenarios"] = scenarios
	return common.MarshalIndent(payload, "", "  ")
}

func writeReplayFixture(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
