package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestQueueManagerWaitsForConcurrencyLease(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8001, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	manager := scheduler.NewQueueManager(500*time.Millisecond, 4)
	releaseDone := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		first.Release()
		close(releaseDone)
	}()

	result := manager.Acquire(context.Background(), &core.DispatchPlan{
		QueueEnabled: true,
		QueueWaitMs:  400,
	}, 8001, dto.ChannelSettings{MaxConcurrency: 1})
	defer result.Lease.Release()

	require.Equal(t, scheduler.QueueAcquireQueued, result.Status)
	require.NotNil(t, result.Lease)
	require.GreaterOrEqual(t, result.WaitTime, 40*time.Millisecond)
	<-releaseDone
}

func TestQueueManagerRejectsWithoutQueuePlan(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8002, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManager(100*time.Millisecond, 4)
	result := manager.Acquire(context.Background(), nil, 8002, dto.ChannelSettings{MaxConcurrency: 1})

	require.Equal(t, scheduler.QueueAcquireRejected, result.Status)
}

func TestQueueManagerContextCancellationReleasesDepth(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8003, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManager(500*time.Millisecond, 4)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		close(started)
		done <- manager.Acquire(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8003, dto.ChannelSettings{MaxConcurrency: 1})
	}()
	<-started
	require.Eventually(t, func() bool {
		return manager.Depth(8003) == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	cancel()
	result := <-done

	require.Equal(t, scheduler.QueueAcquireRejected, result.Status)
	require.Equal(t, 0, manager.Depth(8003))
}

func TestQueueManagerTimeoutReleasesDepth(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8004, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManager(30*time.Millisecond, 4)
	result := manager.Acquire(context.Background(), &core.DispatchPlan{
		QueueEnabled: true,
		QueueWaitMs:  25,
	}, 8004, dto.ChannelSettings{MaxConcurrency: 1})

	require.Equal(t, scheduler.QueueAcquireRejected, result.Status)
	require.Equal(t, 0, manager.Depth(8004))
}

func TestQueueManagerRejectsWhenMaxDepthReached(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8005, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManager(500*time.Millisecond, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	done := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		close(started)
		done <- manager.Acquire(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8005, dto.ChannelSettings{MaxConcurrency: 1})
	}()
	<-started
	require.Eventually(t, func() bool {
		return manager.Depth(8005) == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	rejected := manager.Acquire(context.Background(), &core.DispatchPlan{
		QueueEnabled: true,
		QueueWaitMs:  400,
	}, 8005, dto.ChannelSettings{MaxConcurrency: 1})

	require.Equal(t, scheduler.QueueAcquireRejected, rejected.Status)
	require.Equal(t, map[int]int{8005: 1}, manager.Snapshot())

	cancel()
	<-done
	require.Equal(t, 0, manager.Depth(8005))
}

func TestQueueManagerPriorityOptionsDoNotBypassDefaultDepthLimit(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8006, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManager(500*time.Millisecond, 1)
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		close(started)
		done <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8006, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group:    "default",
			Priority: 0,
		})
	}()
	<-started
	require.Eventually(t, func() bool {
		return manager.Depth(8006) == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	rejected := manager.AcquireWithOptions(context.Background(), &core.DispatchPlan{
		QueueEnabled: true,
		QueueWaitMs:  400,
	}, 8006, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
		Group:    "high-priority",
		Priority: 100,
	})

	require.Equal(t, scheduler.QueueAcquireRejected, rejected.Status)
	require.Equal(t, map[int]int{8006: 1}, manager.Snapshot())

	cancel()
	<-done
	require.Equal(t, 0, manager.Depth(8006))
}

func TestQueueManagerAdmissionPolicyReceivesPriorityContext(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8007, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	var seen scheduler.QueueAdmissionContext
	manager := scheduler.NewQueueManagerWithAdmissionPolicy(100*time.Millisecond, 2, scheduler.QueueAdmissionPolicyFunc(func(ctx scheduler.QueueAdmissionContext) bool {
		seen = ctx
		return false
	}))
	result := manager.AcquireWithOptions(context.Background(), &core.DispatchPlan{
		QueueEnabled: true,
		QueueWaitMs:  50,
	}, 8007, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
		Group:    "high-priority",
		Priority: 100,
	})

	require.Equal(t, scheduler.QueueAcquireRejected, result.Status)
	require.Equal(t, scheduler.QueueAdmissionContext{
		ChannelID:         8007,
		Group:             "high-priority",
		Priority:          100,
		CurrentDepth:      0,
		CurrentGroupDepth: 0,
		MaxDepth:          2,
	}, seen)
	require.Equal(t, 0, manager.Depth(8007))
}

func TestQueueManagerPriorityPolicyAllowsHighPriorityExtraCapacity(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8008, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManagerWithAdmissionPolicy(500*time.Millisecond, 1, scheduler.NewPriorityQueueAdmissionPolicy(scheduler.QueueFairnessOptions{
		HighPriorityGroups:     []string{"high-priority"},
		HighPriorityExtraDepth: 1,
		AbsoluteMaxDepth:       2,
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	normalDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		normalDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8008, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group: "default",
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8008) == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	highDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		highDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8008, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group: "high-priority",
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8008) == 2
	}, 100*time.Millisecond, 10*time.Millisecond)
	require.Equal(t, map[int]int{8008: 2}, manager.Snapshot())

	cancel()
	require.Equal(t, scheduler.QueueAcquireRejected, (<-normalDone).Status)
	require.Equal(t, scheduler.QueueAcquireRejected, (<-highDone).Status)
	require.Equal(t, 0, manager.Depth(8008))
}

func TestQueueManagerPriorityPolicyEnforcesAbsoluteLimit(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8009, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManagerWithAdmissionPolicy(500*time.Millisecond, 1, scheduler.NewPriorityQueueAdmissionPolicy(scheduler.QueueFairnessOptions{
		HighPriorityGroups:     []string{"high-priority"},
		HighPriorityExtraDepth: 3,
		AbsoluteMaxDepth:       2,
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	normalDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		normalDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8009, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group: "default",
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8009) == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	highDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		highDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8009, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group: "high-priority",
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8009) == 2
	}, 100*time.Millisecond, 10*time.Millisecond)

	rejected := manager.AcquireWithOptions(context.Background(), &core.DispatchPlan{
		QueueEnabled: true,
		QueueWaitMs:  400,
	}, 8009, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
		Group: "high-priority",
	})

	require.Equal(t, scheduler.QueueAcquireRejected, rejected.Status)
	require.Equal(t, map[int]int{8009: 2}, manager.Snapshot())

	cancel()
	require.Equal(t, scheduler.QueueAcquireRejected, (<-normalDone).Status)
	require.Equal(t, scheduler.QueueAcquireRejected, (<-highDone).Status)
	require.Equal(t, 0, manager.Depth(8009))
}

func TestQueueManagerPriorityPolicyReservesCapacityFromNormalGroups(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8010, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManagerWithAdmissionPolicy(500*time.Millisecond, 2, scheduler.NewPriorityQueueAdmissionPolicy(scheduler.QueueFairnessOptions{
		HighPriorityGroups:        []string{"high-priority"},
		HighPriorityReservedDepth: 1,
		AbsoluteMaxDepth:          2,
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	normalDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		normalDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8010, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group: "default",
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8010) == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	rejectedNormal := manager.AcquireWithOptions(context.Background(), &core.DispatchPlan{
		QueueEnabled: true,
		QueueWaitMs:  400,
	}, 8010, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
		Group: "default",
	})
	require.Equal(t, scheduler.QueueAcquireRejected, rejectedNormal.Status)
	require.Equal(t, map[int]int{8010: 1}, manager.Snapshot())

	highDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		highDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8010, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group: "high-priority",
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8010) == 2
	}, 100*time.Millisecond, 10*time.Millisecond)

	cancel()
	require.Equal(t, scheduler.QueueAcquireRejected, (<-normalDone).Status)
	require.Equal(t, scheduler.QueueAcquireRejected, (<-highDone).Status)
	require.Equal(t, 0, manager.Depth(8010))
}
