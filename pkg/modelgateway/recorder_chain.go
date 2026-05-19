package modelgateway

import (
	"context"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
)

type ExecutionRecorderChain struct {
	recorders []core.ExecutionRecorder
}

func NewExecutionRecorderChain(recorders ...core.ExecutionRecorder) *ExecutionRecorderChain {
	chain := &ExecutionRecorderChain{}
	for _, recorder := range recorders {
		if recorder != nil {
			chain.recorders = append(chain.recorders, recorder)
		}
	}
	return chain
}

func (c *ExecutionRecorderChain) Record(ctx context.Context, record core.DispatchRecord) {
	if c == nil {
		return
	}
	for _, recorder := range c.recorders {
		recorder.Record(ctx, record)
	}
}

func (c *ExecutionRecorderChain) Report(ctx context.Context, result core.AttemptResult) {
	if c == nil {
		return
	}
	for _, recorder := range c.recorders {
		recorder.Report(ctx, result)
	}
}

var _ core.ExecutionRecorder = (*ExecutionRecorderChain)(nil)
