package service

import (
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"

	"github.com/stretchr/testify/require"
)

func TestTryAcquireChannelConcurrencyHonorsConfiguredLimit(t *testing.T) {
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	setting := dto.ChannelSettings{MaxConcurrency: 1}
	first, ok := TryAcquireChannelConcurrency(1001, setting)
	require.True(t, ok)
	require.Equal(t, 1, first.ActiveAtHit())

	second, ok := TryAcquireChannelConcurrency(1001, setting)
	require.False(t, ok)
	require.Equal(t, 1, second.ActiveAtHit())

	first.Release()
	third, ok := TryAcquireChannelConcurrency(1001, setting)
	require.True(t, ok)
	third.Release()
}

func TestTrackChannelConcurrencyIgnoresConfiguredLimit(t *testing.T) {
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	setting := dto.ChannelSettings{MaxConcurrency: 1}
	first := TrackChannelConcurrency(1006, setting)
	require.Equal(t, 1, first.ActiveAtHit())
	second := TrackChannelConcurrency(1006, setting)
	require.Equal(t, 2, second.ActiveAtHit())
	require.Equal(t, 2, GetChannelActiveConcurrency(1006))

	second.Release()
	first.Release()
	require.Zero(t, GetChannelActiveConcurrency(1006))
}

func TestChannelSelectionReservationCountsTowardEffectiveConcurrency(t *testing.T) {
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	ctx := newRetryContext()
	setting := dto.ChannelSettings{MaxConcurrency: 1}

	require.True(t, ReserveChannelSelection(ctx, 1008, setting))
	require.Equal(t, 1, GetChannelSelectionReservations(1008))
	require.Equal(t, 1, GetChannelEffectiveActiveConcurrency(1008))
	require.True(t, IsChannelConcurrencyFull(1008, setting))
	require.False(t, ReserveChannelSelection(ctx, 1008, setting))

	ReleaseChannelSelectionReservation(ctx, 1008)
	require.Equal(t, 0, GetChannelSelectionReservations(1008))
	require.False(t, IsChannelConcurrencyFull(1008, setting))
}

func TestChannelSelectionReservationDoesNotOverReserveConcurrently(t *testing.T) {
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	setting := dto.ChannelSettings{MaxConcurrency: 3}
	var reserved int32
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ReserveChannelSelection(newRetryContext(), 1009, setting) {
				atomic.AddInt32(&reserved, 1)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int32(3), reserved)
	require.Equal(t, 3, GetChannelSelectionReservations(1009))
	require.True(t, IsChannelConcurrencyFull(1009, setting))
}

func TestChannelFirstByteWaitTracksSlowPendingAndReleases(t *testing.T) {
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	lease := BeginChannelFirstByteWait(newRetryContext(), 1014, "req-first-byte", 0)
	require.NotNil(t, lease)
	status := GetChannelFirstBytePendingStatus(1014)
	require.NotNil(t, status)
	require.Equal(t, 1, status.Pending)
	require.Equal(t, 0, status.SlowPending)

	counter := getChannelFirstByteWaitCounter(1014)
	counter.mu.Lock()
	for key := range counter.waiters {
		counter.waiters[key] = time.Now().Add(-9 * time.Second)
	}
	counter.mu.Unlock()

	status = GetChannelFirstBytePendingStatus(1014)
	require.NotNil(t, status)
	require.Equal(t, 1, status.Pending)
	require.Equal(t, 1, status.SlowPending)
	require.GreaterOrEqual(t, status.OldestMs, int64(8000))

	lease.Release()
	require.Nil(t, GetChannelFirstBytePendingStatus(1014))
}

func TestMarkChannelFirstByteObservedReleasesContextLease(t *testing.T) {
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	ctx := newRetryContext()
	lease := BeginChannelFirstByteWait(ctx, 1015, "req-first-byte", 1)
	require.NotNil(t, lease)
	require.NotNil(t, GetChannelFirstBytePendingStatus(1015))

	MarkChannelFirstByteObserved(ctx)
	require.Nil(t, GetChannelFirstBytePendingStatus(1015))
	lease.Release()
	require.Nil(t, GetChannelFirstBytePendingStatus(1015))
}

func TestLearnChannelConcurrencyLimitSavesObservedLimit(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannel(t, db, 1002, "default", "gpt-5.5", 10, 100)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1002).Update("setting", `{"proxy":"http://127.0.0.1:8080","custom_setting":"keep-me"}`).Error)
	model.InitChannelCache()

	err := types.NewOpenAIError(
		errors.New("status_code=429, Too many pending requests, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		429,
	)

	LearnChannelConcurrencyLimit(nil, 1002, 8, err)

	channel, getErr := model.GetChannelById(1002, true)
	require.NoError(t, getErr)
	require.Equal(t, 7, channel.GetSetting().MaxConcurrency)

	var settingMap map[string]any
	require.NoError(t, common.Unmarshal([]byte(*channel.Setting), &settingMap))
	require.Equal(t, "http://127.0.0.1:8080", settingMap["proxy"])
	require.Equal(t, "keep-me", settingMap["custom_setting"])
}

func TestLearnChannelConcurrencyLimitKeepsLowerExistingLimit(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannel(t, db, 1003, "default", "gpt-5.5", 10, 100)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1003).Update("setting", `{"max_concurrency":3}`).Error)

	err := types.NewOpenAIError(
		errors.New("status_code=429, Too many pending requests, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		429,
	)

	LearnChannelConcurrencyLimit(nil, 1003, 8, err)

	channel, getErr := model.GetChannelById(1003, true)
	require.NoError(t, getErr)
	require.Equal(t, 3, channel.GetSetting().MaxConcurrency)
}

func TestLearnChannelConcurrencyLimitIgnoresGenericRateLimit(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)

	seedChannelSelectChannel(t, db, 1010, "default", "gpt-5.5", 10, 100)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1010).Update("setting", `{"max_concurrency":8}`).Error)
	model.InitChannelCache()

	err := types.NewOpenAIError(
		errors.New("status_code=429, rate limit exceeded, retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)

	result := LearnChannelConcurrencyLimitWithResult(nil, 1010, 8, err)
	require.False(t, result.Changed)
	require.Zero(t, result.LearnedLimit)

	channel, getErr := model.GetChannelById(1010, true)
	require.NoError(t, getErr)
	require.Equal(t, 8, channel.GetSetting().MaxConcurrency)
}

func TestShouldDisableChannelIgnoresLocalConcurrencyLimit(t *testing.T) {
	original := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = original })

	err := types.NewErrorWithStatusCode(
		errors.New("channel #1 reached configured max concurrency 1"),
		types.ErrorCodeChannelConcurrencyLimit,
		429,
	)

	require.False(t, ShouldDisableChannel(err))
}

func TestShouldDisableChannelIgnoresUpstreamPendingConcurrency(t *testing.T) {
	original := common.AutomaticDisableChannelEnabled
	common.AutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = original })

	err := types.NewOpenAIError(
		errors.New("status_code=429, Too many pending requests, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)

	require.False(t, ShouldDisableChannel(err))
}

func TestConcurrencyLimitDoesNotCreateCooldown(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	seedChannelSelectChannel(t, db, 1004, "default", "gpt-5.5", 10, 100)
	seedChannelSelectChannel(t, db, 1005, "default", "gpt-5.5", 9, 100)
	model.InitChannelCache()

	err := types.NewErrorWithStatusCode(
		errors.New("Too many pending requests"),
		types.ErrorCodeChannelConcurrencyLimit,
		http.StatusTooManyRequests,
	)
	metadata, marshalErr := common.Marshal(map[string]any{"retry_after_seconds": 1})
	require.NoError(t, marshalErr)
	err.Metadata = metadata

	RecordChannelConcurrencyCooldown(1004, err)
	require.Nil(t, GetChannelConcurrencyCooldownStatus(1004))

	channel, errSelect := selectChannelForGroup(newRetryContext(), "default", "gpt-5.5", "", false, 0, true)
	require.NoError(t, errSelect)
	require.NotNil(t, channel)
	require.Contains(t, []int{1004, 1005}, channel.Id)
}

func TestSelectNonFullChannelDoesNotSkipActiveFullChannelWithoutCooldown(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	seedChannelSelectChannel(t, db, 1011, "default", "gpt-5.5", 10, 100)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1011).Update("setting", `{"max_concurrency":1}`).Error)
	seedChannelSelectChannel(t, db, 1012, "default", "gpt-5.5", 9, 100)
	model.InitChannelCache()

	lease, ok := TryAcquireChannelConcurrency(1011, dto.ChannelSettings{MaxConcurrency: 1})
	require.True(t, ok)
	defer lease.Release()

	channel, errSelect := selectChannelForGroup(newRetryContext(), "default", "gpt-5.5", "", false, 0, true)
	require.NoError(t, errSelect)
	require.NotNil(t, channel)
	require.Equal(t, 1011, channel.Id)
	require.Nil(t, GetChannelConcurrencyCooldownStatus(1011))
}

func TestUpstreamPendingConcurrencyDoesNotCreateCooldown(t *testing.T) {
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	err := types.NewOpenAIError(
		errors.New("status_code=429, Too many pending requests, please retry later"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusTooManyRequests,
	)

	require.True(t, IsUpstreamConcurrencyLimitError(err))
	require.Nil(t, GetChannelConcurrencyCooldownStatus(1013))
}

func TestChannelUpdatePreservesMaxConcurrencyCeiling(t *testing.T) {
	db := setupChannelSelectTestDB(t)

	seedChannelSelectChannel(t, db, 1006, "default", "gpt-5.5", 10, 100)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1006).Update("setting", `{"proxy":"http://127.0.0.1:8080","max_concurrency":7,"max_concurrency_ceiling":8}`).Error)

	channel, err := model.GetChannelById(1006, true)
	require.NoError(t, err)
	channel.Setting = common.GetPointer(`{"proxy":"http://127.0.0.1:8081","max_concurrency":6}`)
	require.NoError(t, channel.Update())

	updated, err := model.GetChannelById(1006, true)
	require.NoError(t, err)
	require.Contains(t, *updated.Setting, `"max_concurrency_ceiling":8`)
	require.Contains(t, *updated.Setting, `"proxy":"http://127.0.0.1:8081"`)
}

func TestRecordChannelConcurrencySuccessRecoversLimit(t *testing.T) {
	db := setupChannelSelectTestDB(t)
	withChannelSelectMemoryCache(t, true)
	ClearChannelConcurrencyForTest()
	t.Cleanup(ClearChannelConcurrencyForTest)

	seedChannelSelectChannel(t, db, 1007, "default", "gpt-5.5", 10, 100)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 1007).Update("setting", `{"max_concurrency":7,"max_concurrency_ceiling":8}`).Error)
	model.InitChannelCache()

	state := getChannelConcurrencyControlState(1007)
	state.mu.Lock()
	state.cooldownUntil = time.Now().Add(-time.Second)
	state.reason = "test"
	state.failureCount = 1
	state.successStreak = 0
	state.mu.Unlock()

	RecordChannelConcurrencySuccess(1007)
	RecordChannelConcurrencySuccess(1007)
	RecordChannelConcurrencySuccess(1007)

	channel, err := model.GetChannelById(1007, true)
	require.NoError(t, err)
	require.Equal(t, 8, channel.GetSetting().MaxConcurrency)
	require.Contains(t, *channel.Setting, `"max_concurrency_ceiling":8`)
}
