package service

import (
	"errors"
	"testing"

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
