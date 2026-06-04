package core

const (
	ErrorCategoryUnknown                   = "unknown"
	ErrorCategoryClientAborted             = "client_aborted"
	ErrorCategoryChannelInducedClientAbort = "channel_induced_client_abort"
	ErrorCategoryStreamInterrupted         = "stream_interrupted"
	ErrorCategoryLocalConcurrencyLimit     = "local_concurrency_limit"
	ErrorCategoryUpstreamConcurrencyLimit  = "upstream_concurrency_limit"
	ErrorCategoryOverloadSkip              = "overload_skip"
	ErrorCategoryAuthConfigError           = "auth_config_error"
	ErrorCategoryRateLimit                 = "upstream_rate_limit"
	ErrorCategoryBalanceOrQuota            = "balance_or_quota"
	ErrorCategoryUserQuotaExhausted        = "user_quota_exhausted"
	ErrorCategoryUnsupportedCapability     = "unsupported_capability"
	ErrorCategoryClientRequestError        = "client_request_error"
	ErrorCategoryUpstreamError             = "upstream_error"
	ErrorCategoryServerError               = "server_error"
	ErrorCategoryTimeout                   = "timeout"
	ErrorCategorySchedulerExhausted        = "scheduler_exhausted"
)

const (
	WarningLevelInfo     = "info"
	WarningLevelWarning  = "warning"
	WarningLevelCritical = "critical"

	WarningFlagChannelInducedAbort     = "channel_induced_abort"
	WarningFlagNoEffectiveFirstByte    = "no_effective_first_byte"
	WarningFlagHighAbortRate           = "high_abort_rate"
	WarningFlagTotalTimeoutAfterOutput = "total_duration_timeout_after_output"
)
