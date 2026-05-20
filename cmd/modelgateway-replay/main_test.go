package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
	"github.com/stretchr/testify/require"
)

func TestRunReplayBatchCLICommandEntry(t *testing.T) {
	artifact, err := testkit.LoadReplayArtifact(filepath.Join("..", "..", "pkg", "modelgateway", "testdata", "replay", "model_execution_replay.json"))
	require.NoError(t, err)

	root := t.TempDir()
	goldenPath := filepath.Join(root, "golden", "model_execution_replay.json")
	reportPath := filepath.Join(root, "reports", "replay-ci.json")
	require.NoError(t, testkit.WriteReplayArtifact(goldenPath, artifact))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run([]string{
		"-golden", filepath.Dir(goldenPath),
		"-report", reportPath,
		"-score-tolerance", "0.01",
		"-breakdown-tolerance", "0.01",
	}, &stdout, &stderr)
	require.Equal(t, testkit.ReplayBatchRunExitOK, exitCode)
	require.Contains(t, stdout.String(), "replay batch: status=drifted exit_code=0 artifacts=1 scenarios=1")
	require.Empty(t, stderr.String())

	data, err := os.ReadFile(reportPath)
	require.NoError(t, err)
	var report testkit.ReplayBatchRunCIReport
	require.NoError(t, common.Unmarshal(data, &report))
	require.Equal(t, "drifted", report.Status)
	require.Equal(t, testkit.ReplayBatchRunExitOK, report.ExitCode)
	require.Equal(t, 1, report.ArtifactCount)
	require.Equal(t, 1, report.ScenarioCount)
	require.Equal(t, 1, report.NonBlockingDrifts)
}

func TestRunReplayBatchCLICommandEntryRequiresGoldenPath(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := run(nil, &stdout, &stderr)

	require.Equal(t, testkit.ReplayBatchRunExitBlockingRegression, exitCode)
	require.Contains(t, stdout.String(), "replay batch: status=failed exit_code=1")
	require.Contains(t, stdout.String(), "blocking_regressions=1")
	require.Empty(t, stderr.String())
}
