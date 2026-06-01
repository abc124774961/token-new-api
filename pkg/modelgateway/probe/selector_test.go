package probe

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	modelgatewayprovider "github.com/QuantumNous/new-api/pkg/modelgateway/provider"
	"github.com/QuantumNous/new-api/pkg/modelgateway/scheduler"
	"github.com/QuantumNous/new-api/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProbeSelectorSkipsWhenNoRecentRealTraffic(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:               key,
		SampleCount:       5,
		SuccessRate:       0.2,
		LastRealAttemptAt: time.Now().Unix(),
	})
	seedProbeSelectorChannel(t, db, 1, "low-score", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestProbeSelectorSelectsLowScoreRuntimeWithRecentTraffic(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-low-score", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "low-score", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        8,
		SuccessRate:        0.4,
		LastRealAttemptAt:  now.Unix(),
		RealSampleCount30m: 2,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second, LowScoreThreshold: 0.7})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, reasonLowScore, candidates[0].Reason)
	require.Equal(t, key.RequestedModel, candidates[0].Key.RequestedModel)
	require.Equal(t, key.UpstreamModel, candidates[0].Key.UpstreamModel)
	require.Equal(t, key.ChannelID, candidates[0].Key.ChannelID)
	require.Equal(t, key.Group, candidates[0].Key.Group)
	require.Equal(t, key.EndpointType, candidates[0].Key.EndpointType)
	require.Contains(t, candidates[0].Key.CapabilityFingerprint, modelgatewayprovider.ProfileStandardOpenAICompatible)
	require.Contains(t, candidates[0].Key.CapabilityFingerprint, modelgatewayprovider.ProxyModeNative)
	require.Equal(t, "default", candidates[0].Group)
}

func TestProbeSelectorTimeoutRecoveryHasPriorityAndRequiredSamples(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-timeout-recovery", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 2, "timeout-recovery", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                  key,
		SampleCount:          8,
		SuccessRate:          0.99,
		FailureAvoidance:     true,
		ProbeRecoveryPending: true,
		ProbeTriggerReason:   reasonTimeoutRecovery,
		LastRealAttemptAt:    now.Unix(),
		RealSampleCount30m:   2,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{
		MinChannelInterval:               time.Second,
		RecoverySuccessesRequired:        2,
		TimeoutRecoverySuccessesRequired: 3,
		FailureAvoidancePriorityEnabled:  true,
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, reasonTimeoutRecovery, candidates[0].Reason)

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.True(t, snapshot.ProbeRecoveryPending)
	require.Equal(t, 3, snapshot.ProbeRecoveryRequired)
}

func TestProbeSelectorDoesNotSelectLowScoreWhenUnifiedHealthAboveThreshold(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-healthy-score", "gpt-5.4", "codex-plus", "codex-plus", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "healthy-score", "codex-plus", "gpt-5.4", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-5.4", UpstreamModel: "gpt-5.4", ChannelID: channel.Id, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        10,
		SuccessRate:        0.8,
		ProbeTriggerReason: reasonLowScore,
		LastRealAttemptAt:  now.Unix(),
		LastRealSuccessAt:  now.Unix(),
		RealSampleCount30m: 2,
		LastProbeSuccessAt: now.Unix(),
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{
		MinChannelInterval:     time.Second,
		LowScoreThreshold:      0.62,
		LongNoSuccessThreshold: 30 * time.Minute,
	})
	require.NoError(t, err)
	require.Empty(t, candidates)

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, 0.8, snapshot.SuccessRate)
}

func TestProbeSelectorLowScoreIgnoresNonRecoverableCostOnlyPenalty(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-cost-policy", "gpt-5.5", "cost-plus", "cost-plus", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "cost-policy", "cost-plus", "gpt-5.5", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-5.5", UpstreamModel: "gpt-5.5", ChannelID: channel.Id, Group: "cost-plus", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        10,
		SuccessRate:        1,
		TTFTMs:             400,
		DurationMs:         1600,
		TokensPerSecond:    70,
		CostRatio:          0.40,
		CostReferenceRatio: 0.10,
		GroupPriorityRatio: 1,
		LastRealAttemptAt:  now.Unix(),
		RealSampleCount30m: 1,
	})

	selector := NewProbeSelector(store, nil).WithPolicyForGroup(func(group string) core.GroupSmartPolicy {
		require.Equal(t, "cost-plus", group)
		return core.GroupSmartPolicy{
			Strategy:        core.StrategyCostFirst,
			AutoMode:        core.AutoModeSequential,
			CandidateGroups: []string{"cost-plus"},
			GroupPriorityRatio: map[string]float64{
				"cost-plus": 1,
			},
		}
	})
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second, LowScoreThreshold: 0.82})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestProbeSelectorLowScoreHonorsConfiguredRecoverableItems(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-configured-items", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "configured-items", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        20,
		SuccessRate:        0.6,
		LastRealAttemptAt:  now.Unix(),
		LastRealSuccessAt:  now.Add(-2 * time.Hour).Unix(),
		RealSampleCount30m: 1,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{
		MinChannelInterval:           time.Second,
		LowScoreThreshold:            0.7,
		RecoverableScoreItems:        []string{ScoreItemTTFTLatency},
		SkipRecentRealRequestEnabled: false,
		GoodBaselineEnabled:          true,
		GoodBaselineMinSamples:       3,
	})
	require.NoError(t, err)
	require.Empty(t, candidates)

	candidates, err = selector.Select(ProbeConfig{
		MinChannelInterval:           time.Second,
		LowScoreThreshold:            0.7,
		RecoverableScoreItems:        []string{ScoreItemCompletionRate},
		SkipRecentRealRequestEnabled: false,
		GoodBaselineEnabled:          true,
		GoodBaselineMinSamples:       3,
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, []string{ScoreItemCompletionRate}, candidates[0].TriggerScoreItems)
}

func TestProbeSelectorFirstByteBacklogTriggersLowScoreRecovery(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-first-byte-backlog", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "first-byte-backlog", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                   key,
		SampleCount:           20,
		SuccessRate:           0.99,
		TTFTMs:                500,
		DurationMs:            1200,
		TokensPerSecond:       60,
		FirstBytePending:      4,
		SlowFirstBytePending:  3,
		OldestFirstByteWaitMs: 20000,
		LastRealAttemptAt:     now.Unix(),
		LastRealSuccessAt:     now.Add(-2 * time.Hour).Unix(),
		RealSampleCount30m:    1,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{
		MinChannelInterval:           time.Second,
		LowScoreThreshold:            1,
		RecoverableScoreItems:        []string{ScoreItemFirstByteBacklog},
		SkipRecentRealRequestEnabled: false,
		GoodBaselineEnabled:          true,
		GoodBaselineMinSamples:       3,
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Contains(t, candidates[0].TriggerScoreItems, ScoreItemFirstByteBacklog)
}

func TestProbeSelectorSkipsWhenRecentRealRequestExistsForRuntime(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-recent-real", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "recent-real", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        20,
		SuccessRate:        0.3,
		LastRealAttemptAt:  now.Unix(),
		LastRealSuccessAt:  now.Add(-2 * time.Hour).Unix(),
		RealSampleCount30m: 1,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{
		MinChannelInterval:           time.Second,
		LowScoreThreshold:            0.7,
		RecoverableScoreItems:        []string{ScoreItemCompletionRate},
		SkipRecentRealRequestEnabled: true,
		RecentRealRequestWindow:      30 * time.Minute,
		GoodBaselineEnabled:          true,
		GoodBaselineMinSamples:       3,
	})
	require.NoError(t, err)
	require.Empty(t, candidates)
	require.True(t, skipRecentRealRequestProbe(ProbeCandidate{Key: key, Channel: channel, Model: "gpt-4.1", Group: "default"}, ProbeConfig{
		SkipRecentRealRequestEnabled: true,
		RecentRealRequestWindow:      30 * time.Minute,
	}, store, now))
	require.False(t, skipRecentRealRequestProbe(ProbeCandidate{Key: key, Channel: channel, Model: "gpt-4.1", Group: "default", Reason: reasonCircuitProbe}, ProbeConfig{
		SkipRecentRealRequestEnabled: true,
		RecentRealRequestWindow:      30 * time.Minute,
	}, store, now))
	require.False(t, skipRecentRealRequestProbe(ProbeCandidate{Key: key, Channel: channel, Model: "gpt-4.1", Group: "default", Reason: reasonTimeoutRecovery}, ProbeConfig{
		SkipRecentRealRequestEnabled: true,
		RecentRealRequestWindow:      30 * time.Minute,
	}, store, now))
}

func TestProbeSelectorGoodBaselineRequiresRecentHistoricalSuccess(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequestForChannel(t, db, "req-baseline-traffic", "gpt-4.1", "default", "default", 2, now.Unix(), true)
	channel := seedProbeSelectorChannel(t, db, 1, "baseline-window", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        20,
		SuccessRate:        0.6,
		LastRealAttemptAt:  now.Unix(),
		LastRealSuccessAt:  now.Add(-48 * time.Hour).Unix(),
		RealSampleCount30m: 1,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{
		MinChannelInterval:           time.Second,
		LowScoreThreshold:            0.7,
		RecoverableScoreItems:        []string{ScoreItemCompletionRate},
		SkipRecentRealRequestEnabled: false,
		GoodBaselineEnabled:          true,
		GoodBaselineMinSamples:       3,
		GoodBaselineWindow:           24 * time.Hour,
	})
	require.NoError(t, err)
	require.Empty(t, candidates)

	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        20,
		SuccessRate:        0.6,
		LastRealAttemptAt:  now.Unix(),
		LastRealSuccessAt:  now.Add(-time.Hour).Unix(),
		RealSampleCount30m: 1,
	})
	candidates, err = selector.Select(ProbeConfig{
		MinChannelInterval:           time.Second,
		LowScoreThreshold:            0.7,
		RecoverableScoreItems:        []string{ScoreItemCompletionRate},
		SkipRecentRealRequestEnabled: false,
		GoodBaselineEnabled:          true,
		GoodBaselineMinSamples:       3,
		GoodBaselineWindow:           24 * time.Hour,
	})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, reasonLowScore, candidates[0].Reason)
}

func TestProbeSelectorDoesNotUseLongNoSuccessWhenChannelRecentlySucceeded(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	channel := seedProbeSelectorChannel(t, db, 1, "recent-success", "default", "gpt-4.1,gpt-5.4", 1)
	seedProbeSelectorRecentRequestForChannel(t, db, "req-succeeded", "gpt-4.1", "default", "default", channel.Id, now.Add(-time.Minute).Unix(), true)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-5.4", UpstreamModel: "gpt-5.4", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        8,
		SuccessRate:        0.75,
		LastRealAttemptAt:  now.Add(-2 * time.Hour).Unix(),
		LastRealFailureAt:  now.Add(-2 * time.Hour).Unix(),
		RealSampleCount30m: 0,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{
		MinChannelInterval:     time.Second,
		LowScoreThreshold:      0.62,
		LongNoSuccessThreshold: 30 * time.Minute,
	})
	require.NoError(t, err)
	require.NotContains(t, probeCandidateReasons(candidates), reasonLongNoSuccess)
}

func TestProbeSelectorDoesNotUseLongNoSuccessWhenSameRuntimeRecentlySucceededInSummary(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	channel := seedProbeSelectorChannel(t, db, 1, "summary-success", "default", "gpt-5.4", 1)
	seedProbeSelectorRecentRequestForChannel(t, db, "req-same-key-succeeded", "gpt-5.4", "default", "default", channel.Id, now.Add(-time.Minute).Unix(), true)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-5.4", UpstreamModel: "gpt-5.4", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        5,
		SuccessRate:        0.8,
		LastRealAttemptAt:  now.Add(-2 * time.Hour).Unix(),
		RealSampleCount30m: 0,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{
		MinChannelInterval:     time.Second,
		LowScoreThreshold:      0.62,
		LongNoSuccessThreshold: 30 * time.Minute,
	})
	require.NoError(t, err)
	require.NotContains(t, probeCandidateReasons(candidates), reasonLongNoSuccess)
}

func TestProbeSelectorDoesNotSelectLowTrafficForHealthyRecentScope(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-traffic", "gpt-4.1", "codex-pro", "codex-pro", now.Unix())
	_ = seedProbeSelectorChannel(t, db, 1, "target", "codex-pro", "gpt-4.1", 1)
	_ = seedProbeSelectorChannel(t, db, 2, "other-model", "codex-pro", "gpt-5.5", 1)
	_ = seedProbeSelectorChannel(t, db, 3, "other-group", "codex-plus", "gpt-4.1", 1)
	model.InitChannelCache()

	selector := NewProbeSelector(scheduler.NewMemoryRuntimeSnapshotStore(), nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestProbeSelectorDoesNotUseLowTrafficForCodexRuntimeKeyActivation(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-codex-runtime-key", "gpt-5.4", "codex-plus", "codex-plus", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 4, "toioto", "codex-plus", "gpt-5.4", 1)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("type", constant.ChannelTypeCodex).Error)
	channel.Type = constant.ChannelTypeCodex
	model.InitChannelCache()

	selector := NewProbeSelector(scheduler.NewMemoryRuntimeSnapshotStore(), nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second})
	require.NoError(t, err)
	require.Empty(t, candidates)

	key := probeRuntimeKeyForChannel(channel, "gpt-5.4", "codex-plus", endpointTypeForProbe(channel, "gpt-5.4"), core.RuntimeKey{})
	require.Equal(t, constant.EndpointTypeOpenAIResponse, key.EndpointType)
	require.Contains(t, key.CapabilityFingerprint, modelgatewayprovider.ProfileOpenAICodex)
	require.Contains(t, key.CapabilityFingerprint, modelgatewayprovider.ProxyModeNativeResponses)
}

func TestProbeSelectorCollapsesCandidatesToSingleModelPerChannel(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-channel-one-model-a", "gpt-4.1", "codex-plus", "codex-plus", now.Unix())
	seedProbeSelectorRecentRequest(t, db, "req-channel-one-model-b", "gpt-5.5", "codex-plus", "codex-plus", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "multi-model-channel", "codex-plus", "gpt-4.1,gpt-5.5", 1)
	testModel := "gpt-5.5"
	channel.TestModel = &testModel
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("test_model", testModel).Error)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	for _, modelName := range []string{"gpt-4.1", "gpt-5.5"} {
		key := core.RuntimeKey{RequestedModel: modelName, UpstreamModel: modelName, ChannelID: channel.Id, Group: "codex-plus", EndpointType: constant.EndpointTypeOpenAI}
		store.Put(core.RuntimeSnapshot{
			Key:                key,
			SampleCount:        8,
			SuccessRate:        0.3,
			LastRealAttemptAt:  now.Unix(),
			RealSampleCount30m: 1,
		})
	}

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second})
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, channel.Id, candidates[0].Channel.Id)
	require.Equal(t, reasonLowScore, candidates[0].Reason)
	require.Equal(t, "gpt-5.5", candidates[0].Model)
	require.Equal(t, "codex-plus", candidates[0].Group)
}

func TestProbeSelectorSkipsConfigErrorIsolatedSnapshot(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-config-isolated", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "config-isolated", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                 key,
		SampleCount:         8,
		SuccessRate:         0.4,
		ConfigErrorIsolated: true,
		IsolationReason:     core.ErrorCategoryAuthConfigError,
		LastRealAttemptAt:   now.Unix(),
		RealSampleCount30m:  2,
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second, LowScoreThreshold: 0.7})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestProbeSelectorSkipsConfigErrorIsolatedChannel(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-channel-config-isolated", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "config-isolated", "default", "gpt-4.1", 1)
	channel.SetOtherInfo(map[string]interface{}{
		"config_error_isolated": true,
		"isolation_reason":      core.ErrorCategoryAuthConfigError,
	})
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("other_info", channel.OtherInfo).Error)
	model.InitChannelCache()

	selector := NewProbeSelector(scheduler.NewMemoryRuntimeSnapshotStore(), nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestProbeSelectorSkipsServiceConfigErrorIsolatedRoute(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-service-config-isolated", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "config-isolated", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	key := service.NewChannelConfigIsolationKey(channel.Id, "gpt-4.1", "default", constant.EndpointTypeOpenAI)
	service.ClearChannelConfigIsolation(key)
	t.Cleanup(func() { service.ClearChannelConfigIsolation(key) })
	service.RecordChannelConfigAuthError(key, core.ErrorCategoryAuthConfigError)
	service.RecordChannelConfigAuthError(key, core.ErrorCategoryAuthConfigError)

	selector := NewProbeSelector(scheduler.NewMemoryRuntimeSnapshotStore(), nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: time.Second})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestProbeSelectorRateLimitsByRuntimeKey(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-rate-limit", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "target", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                key,
		SampleCount:        5,
		SuccessRate:        0.3,
		LastRealAttemptAt:  now.Unix(),
		RealSampleCount30m: 1,
		LastProbeAt:        now.Add(-time.Minute).Unix(),
	})

	selector := NewProbeSelector(store, nil)
	candidates, err := selector.Select(ProbeConfig{MinChannelInterval: 5 * time.Minute, LowScoreThreshold: 0.7})
	require.NoError(t, err)
	require.Empty(t, candidates)
}

func TestProbeSelectorRateLimitsLegacySnapshotAfterCapabilityKeyEnrichment(t *testing.T) {
	db := setupProbeSelectorTestDB(t)
	now := time.Now()
	seedProbeSelectorRecentRequest(t, db, "req-legacy-key-rate-limit", "gpt-4.1", "default", "default", now.Unix())
	channel := seedProbeSelectorChannel(t, db, 1, "legacy-key", "default", "gpt-4.1", 1)
	model.InitChannelCache()

	store := scheduler.NewMemoryRuntimeSnapshotStore()
	legacyKey := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: channel.Id, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	store.Put(core.RuntimeSnapshot{
		Key:                legacyKey,
		SampleCount:        5,
		SuccessRate:        0.3,
		LastRealAttemptAt:  now.Unix(),
		RealSampleCount30m: 1,
	})

	selector := NewProbeSelector(store, nil)
	selector.now = func() time.Time { return now }
	config := ProbeConfig{MinChannelInterval: 5 * time.Minute, LowScoreThreshold: 0.62}
	first, err := selector.Select(config)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.NotEmpty(t, first[0].Key.CapabilityFingerprint)

	second, err := selector.Select(config)
	require.NoError(t, err)
	require.Empty(t, second)

	enriched, ok := store.Get(first[0].Key)
	require.True(t, ok)
	require.Equal(t, now.Unix(), enriched.LastProbeAt)
	require.Equal(t, reasonLowScore, enriched.ProbeTriggerReason)
}

func TestRuntimeHealthMonitorDoesNotUpdateRealAccessForProbe(t *testing.T) {
	store := scheduler.NewMemoryRuntimeSnapshotStore()
	monitor := scheduler.NewRuntimeHealthMonitor(store, nil)
	key := core.RuntimeKey{RequestedModel: "gpt-4.1", UpstreamModel: "gpt-4.1", ChannelID: 1, Group: "default", EndpointType: constant.EndpointTypeOpenAI}
	realAt := time.Now().Add(-time.Minute)
	probeAt := time.Now()

	monitor.Report(nil, core.AttemptResult{
		Key:        key,
		ChannelID:  1,
		ModelName:  "gpt-4.1",
		Success:    false,
		ObservedAt: realAt,
	})
	monitor.Report(nil, core.AttemptResult{
		Key:           key,
		ChannelID:     1,
		ModelName:     "gpt-4.1",
		Success:       true,
		ObservedAt:    probeAt,
		IsHealthProbe: true,
		ProbeReason:   reasonLowTraffic,
	})

	snapshot, ok := store.Get(key)
	require.True(t, ok)
	require.Equal(t, realAt.Unix(), snapshot.LastRealAttemptAt)
	require.Zero(t, snapshot.LastRealSuccessAt)
	require.Equal(t, realAt.Unix(), snapshot.LastRealFailureAt)
	require.Equal(t, probeAt.Unix(), snapshot.LastProbeAt)
	require.Equal(t, probeAt.Unix(), snapshot.LastProbeSuccessAt)
	require.Equal(t, 1, snapshot.RealSampleCount30m)
	require.Equal(t, 2, snapshot.SampleCount)
}

func setupProbeSelectorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ModelGatewayUserRequestSummary{}))

	oldDB := model.DB
	oldMemoryCache := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	oldUsingSQLite := common.UsingSQLite
	oldUsingMySQL := common.UsingMySQL
	oldUsingPostgreSQL := common.UsingPostgreSQL
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = oldDB
		common.MemoryCacheEnabled = oldMemoryCache
		common.RedisEnabled = oldRedisEnabled
		common.UsingSQLite = oldUsingSQLite
		common.UsingMySQL = oldUsingMySQL
		common.UsingPostgreSQL = oldUsingPostgreSQL
		if oldMemoryCache && oldDB != nil {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedProbeSelectorChannel(t *testing.T, db *gorm.DB, id int, name string, group string, modelName string, priority int64) *model.Channel {
	t.Helper()
	weight := uint(100)
	channel := &model.Channel{
		Id:          id,
		Type:        constant.ChannelTypeOpenAI,
		Name:        name,
		Key:         fmt.Sprintf("sk-%d", id),
		Status:      common.ChannelStatusEnabled,
		Group:       group,
		Models:      modelName,
		Weight:      &weight,
		Priority:    &priority,
		CreatedTime: int64(id),
	}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	return channel
}

func seedProbeSelectorRecentRequest(t *testing.T, db *gorm.DB, requestID string, modelName string, requestedGroup string, selectedGroup string, completedAt int64) {
	t.Helper()
	seedProbeSelectorRecentRequestForChannel(t, db, requestID, modelName, requestedGroup, selectedGroup, 1, completedAt, true)
}

func seedProbeSelectorRecentRequestForChannel(t *testing.T, db *gorm.DB, requestID string, modelName string, requestedGroup string, selectedGroup string, channelID int, completedAt int64, success bool) {
	t.Helper()
	require.NoError(t, db.Create(&model.ModelGatewayUserRequestSummary{
		RequestId:      requestID,
		CreatedAt:      completedAt,
		UpdatedAt:      completedAt,
		CompletedAt:    completedAt,
		RequestedModel: modelName,
		RequestedGroup: requestedGroup,
		SelectedGroup:  selectedGroup,
		FinalChannelID: channelID,
		FinalSuccess:   success,
	}).Error)
}

func probeCandidateReasons(candidates []ProbeCandidate) []string {
	reasons := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		reasons = append(reasons, candidate.Reason)
	}
	return reasons
}
