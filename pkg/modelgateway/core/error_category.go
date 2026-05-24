package core

const (
	ErrorCategoryUnknown                  = "unknown"
	ErrorCategoryClientAborted            = "client_aborted"
	ErrorCategoryStreamInterrupted        = "stream_interrupted"
	ErrorCategoryLocalConcurrencyLimit    = "local_concurrency_limit"
	ErrorCategoryUpstreamConcurrencyLimit = "upstream_concurrency_limit"
	ErrorCategoryOverloadSkip             = "overload_skip"
	ErrorCategoryAuthConfigError          = "auth_config_error"
	ErrorCategoryRateLimit                = "upstream_rate_limit"
	ErrorCategoryBalanceOrQuota           = "balance_or_quota"
	ErrorCategoryUnsupportedCapability    = "unsupported_capability"
	ErrorCategoryUpstreamError            = "upstream_error"
	ErrorCategoryServerError              = "server_error"
	ErrorCategoryTimeout                  = "timeout"
)
