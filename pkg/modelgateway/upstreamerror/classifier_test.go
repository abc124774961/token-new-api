package upstreamerror

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelgateway/core"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestDefaultClassifierClassifiesRateLimit(t *testing.T) {
	classifier := DefaultRuleCompiler().Compile(true, scheduler_setting.DefaultUpstreamErrorRules())

	result := classifier.Classify(Signal{
		StatusCode: 429,
		Message:    "Rate limit exceeded, please retry later",
		Metadata:   `{"retry_after_seconds":60}`,
	})

	require.True(t, result.Matched)
	require.Equal(t, scheduler_setting.UpstreamErrorKindRateLimit, result.Kind)
	require.Equal(t, "upstream_rate_limit", result.MatchedRuleID)
	require.Equal(t, core.ErrorCategoryRateLimit, result.ErrorCategory)
	require.Equal(t, scheduler_setting.UpstreamErrorActionSwitchChannel, result.SchedulerAction)
	require.Equal(t, 60, result.AvoidanceSeconds)
	require.Equal(t, 60, result.RetryAfterSeconds)
}

func TestDefaultClassifierPrioritizesBalanceQuotaOverRateLimit(t *testing.T) {
	classifier := DefaultRuleCompiler().Compile(true, scheduler_setting.DefaultUpstreamErrorRules())

	result := classifier.Classify(Signal{
		StatusCode: 429,
		ErrorCode:  "insufficient_quota",
		Message:    "rate limit exceeded because the account has insufficient quota",
	})

	require.True(t, result.Matched)
	require.Equal(t, scheduler_setting.UpstreamErrorKindBalanceQuota, result.Kind)
	require.Equal(t, "upstream_balance_quota", result.MatchedRuleID)
	require.Equal(t, core.ErrorCategoryBalanceOrQuota, result.ErrorCategory)
	require.Equal(t, scheduler_setting.UpstreamErrorActionSwitchChannel, result.SchedulerAction)
	require.Equal(t, 600, result.AvoidanceSeconds)
}

func TestClassifierDisabledDoesNotMatch(t *testing.T) {
	classifier := DefaultRuleCompiler().Compile(false, scheduler_setting.DefaultUpstreamErrorRules())

	result := classifier.Classify(Signal{
		StatusCode: 429,
		Message:    "rate limit exceeded",
	})

	require.False(t, result.Matched)
}

func TestDefaultClassifierStopsRequestLimitAndPolicySafety(t *testing.T) {
	classifier := DefaultRuleCompiler().Compile(true, scheduler_setting.DefaultUpstreamErrorRules())

	requestLimit := classifier.Classify(Signal{
		StatusCode: 400,
		Message:    "maximum context length exceeded",
	})
	require.True(t, requestLimit.Matched)
	require.Equal(t, scheduler_setting.UpstreamErrorKindRequestLimit, requestLimit.Kind)
	require.Equal(t, core.ErrorCategoryClientRequestError, requestLimit.ErrorCategory)
	require.Equal(t, scheduler_setting.UpstreamErrorActionStop, requestLimit.SchedulerAction)
	require.True(t, ShouldStop(requestLimit))

	policySafety := classifier.Classify(Signal{
		StatusCode: 403,
		Message:    "content policy violation",
	})
	require.True(t, policySafety.Matched)
	require.Equal(t, scheduler_setting.UpstreamErrorKindPolicySafety, policySafety.Kind)
	require.Equal(t, core.ErrorCategoryClientRequestError, policySafety.ErrorCategory)
	require.Equal(t, scheduler_setting.UpstreamErrorActionStop, policySafety.SchedulerAction)
	require.True(t, ShouldStop(policySafety))
}

func TestClassifierManagerReloadUsesCompiledRules(t *testing.T) {
	manager := NewClassifierManager(DefaultRuleCompiler())
	setting := scheduler_setting.DefaultSetting()
	setting.UpstreamErrorClassificationEnabled = true
	setting.UpstreamErrorRules = []scheduler_setting.UpstreamErrorRule{
		{
			ID:               "custom_balance",
			Enabled:          true,
			Priority:         10,
			Kind:             scheduler_setting.UpstreamErrorKindBalanceQuota,
			StatusCodes:      []int{402},
			SchedulerAction:  scheduler_setting.UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 33,
		},
	}
	manager.Reload(setting)

	result := manager.Classify(Signal{StatusCode: 402})
	require.True(t, result.Matched)
	require.Equal(t, "custom_balance", result.MatchedRuleID)
	require.Equal(t, scheduler_setting.UpstreamErrorKindBalanceQuota, result.Kind)
	require.Equal(t, 33, result.AvoidanceSeconds)

	setting.UpstreamErrorClassificationEnabled = false
	manager.Reload(setting)
	result = manager.Classify(Signal{StatusCode: 402})
	require.False(t, result.Matched)
}

func TestClassifyAPIErrorSkipsLocalSemanticErrors(t *testing.T) {
	localConcurrency := types.NewError(
		errors.New("local channel queue is full"),
		types.ErrorCodeChannelConcurrencyLimit,
		types.ErrOptionWithStatusCode(429),
	)
	require.False(t, ClassifyAPIError(localConcurrency).Matched)

	userQuota := types.NewError(
		errors.New("insufficient user quota"),
		types.ErrorCodeInsufficientUserQuota,
		types.ErrOptionWithSkipRetry(),
		types.ErrOptionWithStatusCode(429),
	)
	require.False(t, ClassifyAPIError(userQuota).Matched)
}
