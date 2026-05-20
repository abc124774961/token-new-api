package testkit

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type ReplayBatchRunCLIOptions struct {
	GoldenPaths []string
	ReportPath  string
	RunOptions  ReplayBatchRunOptions
}

// RunReplayBatchCLI runs model gateway replay goldens for command-line and CI use.
//
// Usage:
//
//	go run ./cmd/modelgateway-replay -golden pkg/modelgateway/testdata/replay -report tmp/replay-ci.json
//	go run ./cmd/modelgateway-replay -golden 'pkg/modelgateway/testdata/replay/*.json' -score-tolerance 0.01
//
// Each -golden value may be a JSON artifact file, directory, or glob. Positional
// args are also treated as golden paths. Directories are scanned recursively for
// .json artifacts, duplicate matches are de-duplicated, and -report writes the
// stable ReplayBatchRunCIReport JSON payload.
func RunReplayBatchCLI(args []string, stdout io.Writer, stderr io.Writer) int {
	var goldenPaths replayCLIPathList
	var reportPath string
	var scoreTolerance float64
	var breakdownTolerance float64

	flags := flag.NewFlagSet("modelgateway-replay", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Var(&goldenPaths, "golden", "replay golden file, directory, or glob; may be repeated")
	flags.StringVar(&reportPath, "report", "", "write replay CI JSON report to path")
	flags.Float64Var(&scoreTolerance, "score-tolerance", 0, "score drift tolerance")
	flags.Float64Var(&breakdownTolerance, "breakdown-tolerance", 0, "score breakdown drift tolerance")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ReplayBatchRunExitOK
		}
		return ReplayBatchRunExitBlockingRegression
	}

	options := ReplayBatchRunCLIOptions{
		GoldenPaths: append([]string(goldenPaths), flags.Args()...),
		ReportPath:  reportPath,
		RunOptions: ReplayBatchRunOptions{
			ReplayScoreDriftOptions: ReplayScoreDriftOptions{
				ScoreTolerance:     scoreTolerance,
				BreakdownTolerance: breakdownTolerance,
			},
		},
	}
	report, err := RunReplayBatchCI(options)
	if err != nil {
		if stderr != nil {
			_, _ = fmt.Fprintf(stderr, "replay batch failed: %s\n", err.Error())
		}
		return ReplayBatchRunExitBlockingRegression
	}
	if stdout != nil {
		_, _ = fmt.Fprintln(stdout, report.CLISummary())
	}
	return report.ExitCode()
}

func RunReplayBatchCI(options ReplayBatchRunCLIOptions) (ReplayBatchRunReport, error) {
	report := RunReplayBatchGoldenPaths(options.GoldenPaths, options.RunOptions)
	if strings.TrimSpace(options.ReportPath) == "" {
		return report, nil
	}
	if err := WriteReplayBatchCIReport(options.ReportPath, report); err != nil {
		return report, err
	}
	return report, nil
}

func RunReplayBatchGoldenPaths(goldenPaths []string, options ReplayBatchRunOptions) ReplayBatchRunReport {
	paths, err := ScanReplayGoldenPaths(goldenPaths)
	if err != nil {
		return replayBatchRunErrorReport("golden_scan", err)
	}
	if len(paths) == 0 {
		return replayBatchRunErrorReport("golden_scan", fmt.Errorf("no replay golden artifacts found"))
	}

	report := ReplayBatchRunReport{
		Artifacts: make([]ReplayArtifactRunReport, 0, len(paths)),
	}
	for _, path := range paths {
		artifact, err := loadReplayGoldenArtifact(path)
		if err != nil {
			report.Artifacts = append(report.Artifacts, ReplayArtifactRunReport{
				Name:               path,
				BlockingRegression: true,
				Errors:             []string{err.Error()},
			})
			report.ArtifactCount++
			report.BlockingRegression = true
			continue
		}
		artifactReport := RunReplayArtifact(path, artifact, options)
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

func ScanReplayGoldenPaths(goldenPaths []string) ([]string, error) {
	if len(goldenPaths) == 0 {
		return nil, fmt.Errorf("at least one replay golden path is required")
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(goldenPaths))
	for _, rawPath := range goldenPaths {
		rawPath = strings.TrimSpace(rawPath)
		if rawPath == "" {
			continue
		}
		paths, err := expandReplayGoldenPath(rawPath)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			clean := filepath.Clean(path)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	sort.Strings(out)
	return out, nil
}

func WriteReplayBatchCIReport(path string, report ReplayBatchRunReport) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("replay CI report path is required")
	}
	data, err := report.MarshalCIReport()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func loadReplayGoldenArtifact(path string) (*ReplayArtifact, error) {
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

type replayCLIPathList []string

func (l *replayCLIPathList) String() string {
	if l == nil {
		return ""
	}
	return strings.Join(*l, ",")
}

func (l *replayCLIPathList) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("golden path is required")
	}
	*l = append(*l, value)
	return nil
}

func expandReplayGoldenPath(path string) ([]string, error) {
	if hasReplayGlobMeta(path) {
		matches, err := filepath.Glob(path)
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no replay golden paths matched %q", path)
		}
		out := make([]string, 0, len(matches))
		for _, match := range matches {
			paths, err := expandReplayGoldenPath(match)
			if err != nil {
				return nil, err
			}
			out = append(out, paths...)
		}
		return out, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	out := make([]string, 0)
	if err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			out = append(out, current)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func hasReplayGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func replayBatchRunErrorReport(name string, err error) ReplayBatchRunReport {
	return ReplayBatchRunReport{
		Artifacts: []ReplayArtifactRunReport{
			{
				Name:               name,
				BlockingRegression: true,
				Errors:             []string{err.Error()},
			},
		},
		ArtifactCount:      1,
		BlockingRegression: true,
	}
}
