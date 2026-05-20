package main

import (
	"io"
	"os"

	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	return testkit.RunReplayBatchCLI(args, stdout, stderr)
}
