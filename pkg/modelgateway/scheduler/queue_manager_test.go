package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
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

func TestQueueManagerPoolWaitsForAnyResourceInPool(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8020, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()
	second, acquired := service.TryAcquireChannelConcurrency(8021, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)

	manager := scheduler.NewQueueManager(500*time.Millisecond, 4)
	done := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		done <- manager.AcquirePoolWithOptions(context.Background(), &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, scheduler.QueuePoolAcquireOptions{
			PoolKey: "group=codex-plus|primary=8020,8021",
			Group:   "codex-plus",
			TryAcquire: func() scheduler.QueueAcquireResult {
				for _, channelID := range []int{8020, 8021} {
					lease, ok := service.TryAcquireChannelConcurrency(channelID, dto.ChannelSettings{MaxConcurrency: 1})
					if ok {
						return scheduler.QueueAcquireResult{
							Lease:  lease,
							Status: scheduler.QueueAcquireAcquired,
							Plan: &core.DispatchPlan{
								Channel: &model.Channel{Id: channelID},
							},
						}
					}
				}
				return scheduler.QueueAcquireResult{Status: scheduler.QueueAcquireRejected}
			},
		})
	}()
	require.Eventually(t, func() bool {
		return manager.PoolDepth("group=codex-plus|primary=8020,8021") == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	second.Release()
	result := <-done
	defer result.Lease.Release()

	require.Equal(t, scheduler.QueueAcquireQueued, result.Status)
	require.NotNil(t, result.Lease)
	require.NotNil(t, result.Plan)
	require.NotNil(t, result.Plan.Channel)
	require.Equal(t, 8021, result.Plan.Channel.Id)
	require.Equal(t, 0, manager.PoolDepth("group=codex-plus|primary=8020,8021"))
}

func TestQueueManagerPoolDepthIsIsolatedByGroupPoolKey(t *testing.T) {
	manager := scheduler.NewQueueManager(500*time.Millisecond, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tryReject := func() scheduler.QueueAcquireResult {
		return scheduler.QueueAcquireResult{Status: scheduler.QueueAcquireRejected}
	}
	plan := &core.DispatchPlan{QueueEnabled: true, QueueWaitMs: 400}
	firstDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		firstDone <- manager.AcquirePoolWithOptions(ctx, plan, scheduler.QueuePoolAcquireOptions{
			PoolKey:    "group=A|primary=1,2",
			Group:      "A",
			MaxDepth:   1,
			TryAcquire: tryReject,
		})
	}()
	require.Eventually(t, func() bool {
		return manager.PoolDepth("group=A|primary=1,2") == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	rejected := manager.AcquirePoolWithOptions(context.Background(), plan, scheduler.QueuePoolAcquireOptions{
		PoolKey:    "group=A|primary=1,2",
		Group:      "A",
		MaxDepth:   1,
		TryAcquire: tryReject,
	})
	require.Equal(t, scheduler.QueueAcquireRejected, rejected.Status)

	secondCtx, secondCancel := context.WithCancel(context.Background())
	defer secondCancel()
	secondDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		secondDone <- manager.AcquirePoolWithOptions(secondCtx, plan, scheduler.QueuePoolAcquireOptions{
			PoolKey:    "group=B|primary=1,2",
			Group:      "B",
			MaxDepth:   1,
			TryAcquire: tryReject,
		})
	}()
	require.Eventually(t, func() bool {
		return manager.PoolDepth("group=B|primary=1,2") == 1
	}, 100*time.Millisecond, 10*time.Millisecond)
	snapshot := manager.DetailedSnapshot()
	require.Equal(t, 2, snapshot.Summary.TotalQueued)
	require.Equal(t, 2, snapshot.Summary.QueueGroups)
	require.Len(t, snapshot.Groups, 2)
	require.Len(t, snapshot.RuntimeKeys, 2)

	cancel()
	secondCancel()
	require.Equal(t, scheduler.QueueAcquireRejected, (<-firstDone).Status)
	require.Equal(t, scheduler.QueueAcquireRejected, (<-secondDone).Status)
	require.Equal(t, 0, manager.PoolDepth("group=A|primary=1,2"))
	require.Equal(t, 0, manager.PoolDepth("group=B|primary=1,2"))
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

func TestQueueManagerPriorityPolicyAllowsRetryIntentPriorityExtraCapacity(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8012, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManagerWithAdmissionPolicy(500*time.Millisecond, 1, scheduler.NewPriorityQueueAdmissionPolicy(scheduler.QueueFairnessOptions{
		HighPriorityThreshold:  core.RetryRoutingQueuePriority,
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
		}, 8012, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group: "default",
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8012) == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	retryDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		retryDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8012, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group:    "default",
			Priority: core.RetryRoutingQueuePriority,
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8012) == 2
	}, 100*time.Millisecond, 10*time.Millisecond)

	snapshot := manager.DetailedSnapshot()
	require.Equal(t, 1, snapshot.Summary.HighPriorityDepth)
	require.Equal(t, 1, snapshot.Summary.NormalDepth)

	cancel()
	require.Equal(t, scheduler.QueueAcquireRejected, (<-normalDone).Status)
	require.Equal(t, scheduler.QueueAcquireRejected, (<-retryDone).Status)
	require.Equal(t, 0, manager.Depth(8012))
}

func TestQueueManagerPriorityPolicyLetsRetryIntentAcquireBeforeNormalWaiter(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8013, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)

	manager := scheduler.NewQueueManagerWithAdmissionPolicy(500*time.Millisecond, 2, scheduler.NewPriorityQueueAdmissionPolicy(scheduler.QueueFairnessOptions{
		HighPriorityThreshold: core.RetryRoutingQueuePriority,
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	normalDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		normalDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8013, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group: "default",
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8013) == 1
	}, 100*time.Millisecond, 10*time.Millisecond)

	retryDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		retryDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8013, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{
			Group:    "default",
			Priority: core.RetryRoutingQueuePriority,
		})
	}()
	require.Eventually(t, func() bool {
		return manager.Depth(8013) == 2
	}, 100*time.Millisecond, 10*time.Millisecond)

	first.Release()
	retryResult := <-retryDone
	require.Equal(t, scheduler.QueueAcquireQueued, retryResult.Status)
	require.NotNil(t, retryResult.Lease)
	retryResult.Lease.Release()

	normalResult := <-normalDone
	require.Equal(t, scheduler.QueueAcquireQueued, normalResult.Status)
	require.NotNil(t, normalResult.Lease)
	normalResult.Lease.Release()
	require.Equal(t, 0, manager.Depth(8013))
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

func TestQueueManagerDetailedSnapshotIncludesPriorityGroupsAndRejects(t *testing.T) {
	service.ClearChannelConcurrencyForTest()
	t.Cleanup(service.ClearChannelConcurrencyForTest)

	first, acquired := service.TryAcquireChannelConcurrency(8011, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, acquired)
	defer first.Release()

	manager := scheduler.NewQueueManagerWithAdmissionPolicy(500*time.Millisecond, 2, scheduler.NewPriorityQueueAdmissionPolicy(scheduler.QueueFairnessOptions{
		HighPriorityGroups:        []string{"vip"},
		HighPriorityExtraDepth:    1,
		HighPriorityReservedDepth: 1,
		AbsoluteMaxDepth:          3,
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	normalDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		normalDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8011, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{Group: "default"})
	}()
	require.Eventually(t, func() bool { return manager.Depth(8011) == 1 }, 100*time.Millisecond, 10*time.Millisecond)

	highDone := make(chan scheduler.QueueAcquireResult, 1)
	go func() {
		highDone <- manager.AcquireWithOptions(ctx, &core.DispatchPlan{
			QueueEnabled: true,
			QueueWaitMs:  400,
		}, 8011, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{Group: "vip"})
	}()
	require.Eventually(t, func() bool { return manager.Depth(8011) == 2 }, 100*time.Millisecond, 10*time.Millisecond)

	rejected := manager.AcquireWithOptions(context.Background(), &core.DispatchPlan{
		QueueEnabled: true,
		QueueWaitMs:  400,
	}, 8011, dto.ChannelSettings{MaxConcurrency: 1}, scheduler.QueueAcquireOptions{Group: "default"})
	require.Equal(t, scheduler.QueueAcquireRejected, rejected.Status)

	snapshot := manager.DetailedSnapshot()
	require.Equal(t, 2, snapshot.Summary.TotalQueued)
	require.Equal(t, 1, snapshot.Summary.HighPriorityDepth)
	require.Equal(t, 1, snapshot.Summary.NormalDepth)
	require.Equal(t, 3, snapshot.Summary.HighPriorityCapacity)
	require.Equal(t, 1, snapshot.Summary.NormalCapacity)
	require.Len(t, snapshot.Channels, 1)
	require.Equal(t, 8011, snapshot.Channels[0].ChannelID)
	require.Equal(t, 1, snapshot.Channels[0].HighPriorityDepth)
	require.Equal(t, 1, snapshot.Channels[0].NormalDepth)
	require.Len(t, snapshot.Channels[0].Groups, 2)
	require.Len(t, snapshot.Groups, 2)
	require.NotEmpty(t, snapshot.RejectReasons)
	require.Equal(t, "max_depth_reached", snapshot.RejectReasons[0].Reason)

	cancel()
	require.Equal(t, scheduler.QueueAcquireRejected, (<-normalDone).Status)
	require.Equal(t, scheduler.QueueAcquireRejected, (<-highDone).Status)
	require.Equal(t, 0, manager.Depth(8011))
}
