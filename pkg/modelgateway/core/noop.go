package core

import "context"

type NoopRecorder struct{}

func (NoopRecorder) Record(ctx context.Context, record DispatchRecord) {}

func (NoopRecorder) Report(ctx context.Context, result AttemptResult) {}
