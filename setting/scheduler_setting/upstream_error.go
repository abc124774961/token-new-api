package scheduler_setting

func UpstreamErrorKinds() []string {
	return []string{
		UpstreamErrorKindBalanceQuota,
		UpstreamErrorKindRateLimit,
		UpstreamErrorKindConcurrencyLimit,
		UpstreamErrorKindCapacityOverload,
		UpstreamErrorKindModelPoolUnavailable,
		UpstreamErrorKindToolEndpointUnavailable,
		UpstreamErrorKindAuthAccount,
		UpstreamErrorKindAccessRegion,
		UpstreamErrorKindRequestLimit,
		UpstreamErrorKindPolicySafety,
		UpstreamErrorKindTransportTimeout,
		UpstreamErrorKindBadResponse,
		UpstreamErrorKindStreamInterrupted,
	}
}

func UpstreamErrorActions() []string {
	return []string{
		UpstreamErrorActionSwitchChannel,
		UpstreamErrorActionStop,
		UpstreamErrorActionRecordOnly,
	}
}

func DefaultUpstreamErrorRules() []UpstreamErrorRule {
	return cloneUpstreamErrorRules([]UpstreamErrorRule{
		{
			ID:          "upstream_balance_quota",
			Enabled:     true,
			Priority:    100,
			Kind:        UpstreamErrorKindBalanceQuota,
			StatusCodes: []int{400, 401, 402, 403, 429, 503},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{
					"insufficient_quota",
					"quota_not_enough",
					"balance_insufficient",
					"upstream_unavailable",
				},
				Type: []string{"billing_error", "insufficient_quota"},
				Message: []string{
					"insufficient quota",
					"exceeded your current quota",
					"check your plan and billing",
					"insufficient account balance",
					"insufficient balance",
					"not enough balance",
					"insufficient credit",
					"insufficient credits",
					"credit balance",
					"quota not enough",
					"quota_not_enough",
					"余额不足",
					"余额不够",
					"账户余额",
				},
				Metadata: []string{
					"insufficient_quota",
					"insufficient account balance",
					"balance_insufficient",
					"quota_not_enough",
				},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 600,
			Description:      "Upstream account balance, credit, or provider-side quota is exhausted.",
		},
		{
			ID:          "upstream_concurrency_limit",
			Enabled:     true,
			Priority:    95,
			Kind:        UpstreamErrorKindConcurrencyLimit,
			StatusCodes: []int{429},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"concurrency_limit", "too_many_pending_requests"},
				Message: []string{
					"concurrency limit",
					"concurrency limit exceeded",
					"too many pending requests",
					"too many concurrent",
					"并发",
				},
				Metadata: []string{"concurrency limit", "too many pending requests"},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 60,
			Description:      "Provider-side concurrency or pending request cap was reached.",
		},
		{
			ID:          "upstream_capacity_overload",
			Enabled:     true,
			Priority:    90,
			Kind:        UpstreamErrorKindCapacityOverload,
			StatusCodes: []int{429, 503, 529},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"overloaded", "capacity", "no_available_channel"},
				Message: []string{
					"overloaded",
					"over capacity",
					"capacity",
					"no available",
					"pool is empty",
					"all models are busy",
					"temporarily unavailable",
					"service unavailable",
					"服务器繁忙",
					"资源不足",
					"池子空",
				},
				Metadata: []string{"overloaded", "capacity", "no available", "pool is empty"},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 600,
			Description:      "The upstream pool or provider capacity is temporarily unavailable.",
		},
		{
			ID:          "upstream_rate_limit",
			Enabled:     true,
			Priority:    80,
			Kind:        UpstreamErrorKindRateLimit,
			StatusCodes: []int{400, 429},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"rate_limit", "rate_limited", "too_many_requests"},
				Type: []string{"rate_limit_error"},
				Message: []string{
					"rate limit",
					"rate_limit",
					"too many requests",
					"quota rate",
					"quota limit",
					"retry after",
					"后重试",
					"限速",
					"速率",
					"配额限制",
					"限速规则",
				},
				Header: []string{"retry_after", "rate_limit", "ratelimit"},
				Metadata: []string{
					"retry_after",
					"rate_limit",
					"ratelimit",
					"too many requests",
				},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 60,
			Description:      "Provider request or token rate limit was reached.",
		},
		{
			ID:          "upstream_model_pool_unavailable",
			Enabled:     true,
			Priority:    70,
			Kind:        UpstreamErrorKindModelPoolUnavailable,
			StatusCodes: []int{400, 404, 429, 503},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"model_not_found", "model_not_supported", "model_unavailable"},
				Message: []string{
					"model not found",
					"model_not_found",
					"model not supported",
					"unsupported model",
					"model unavailable",
					"model does not exist",
					"not have access to the model",
					"model pool",
					"模型不存在",
					"模型不可用",
					"未找到模型",
				},
				Metadata: []string{"model not found", "model_not_found", "model unavailable"},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 300,
			Description:      "The selected upstream account or pool cannot serve the model.",
		},
		{
			ID:          "upstream_tool_endpoint_unavailable",
			Enabled:     true,
			Priority:    65,
			Kind:        UpstreamErrorKindToolEndpointUnavailable,
			StatusCodes: []int{400, 404, 422, 503},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"tool_not_supported", "endpoint_not_found", "unsupported_tool"},
				Message: []string{
					"tool not supported",
					"function calling is not supported",
					"tools are not supported",
					"endpoint not found",
					"unsupported endpoint",
					"responses endpoint",
					"api.responses.write",
					"工具不支持",
				},
				Metadata: []string{"tool not supported", "endpoint not found", "api.responses.write"},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 300,
			Description:      "The upstream endpoint cannot serve the requested tool or API capability.",
		},
		{
			ID:          "upstream_auth_account",
			Enabled:     true,
			Priority:    60,
			Kind:        UpstreamErrorKindAuthAccount,
			StatusCodes: []int{401, 403},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"invalid_api_key", "unauthorized", "permission_denied"},
				Type: []string{"auth", "authentication", "permission"},
				Message: []string{
					"invalid api key",
					"incorrect api key",
					"unauthorized",
					"permission denied",
					"invalid token",
					"expired api key",
					"missing scopes",
					"insufficient permissions",
					"无效的 api key",
					"无权限",
				},
				Metadata: []string{"invalid api key", "missing scopes", "insufficient permissions"},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 1800,
			Description:      "The upstream account credential or account permission is invalid.",
		},
		{
			ID:          "upstream_access_region",
			Enabled:     true,
			Priority:    55,
			Kind:        UpstreamErrorKindAccessRegion,
			StatusCodes: []int{403, 451},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"region_not_supported", "unsupported_country"},
				Message: []string{
					"not available in your region",
					"unsupported country",
					"unsupported region",
					"country",
					"region",
					"geo",
					"区域",
					"地区",
				},
				Metadata: []string{"unsupported country", "unsupported region", "region"},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 1800,
			Description:      "The upstream account or endpoint is restricted by region.",
		},
		{
			ID:          "upstream_request_limit",
			Enabled:     true,
			Priority:    50,
			Kind:        UpstreamErrorKindRequestLimit,
			StatusCodes: []int{400, 413, 422},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"context_length_exceeded", "request_too_large", "max_tokens"},
				Message: []string{
					"context length",
					"maximum context",
					"prompt is too long",
					"request too large",
					"input too large",
					"max tokens",
					"too many images",
					"超过上下文",
					"请求过大",
				},
				Metadata: []string{"context length", "request too large", "max tokens"},
			},
			SchedulerAction: UpstreamErrorActionStop,
			Description:     "The request itself exceeds provider limits and should not be retried blindly.",
		},
		{
			ID:          "upstream_policy_safety",
			Enabled:     true,
			Priority:    45,
			Kind:        UpstreamErrorKindPolicySafety,
			StatusCodes: []int{400, 403, 422},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"content_policy_violation", "safety", "blocked"},
				Type: []string{"policy", "safety"},
				Message: []string{
					"content policy",
					"policy violation",
					"safety",
					"blocked",
					"violates",
					"moderation",
					"安全策略",
					"审核",
				},
				Metadata: []string{"content policy", "policy violation", "safety"},
			},
			SchedulerAction: UpstreamErrorActionStop,
			Description:     "The provider rejected the content for policy or safety reasons.",
		},
		{
			ID:          "upstream_transport_timeout",
			Enabled:     true,
			Priority:    40,
			Kind:        UpstreamErrorKindTransportTimeout,
			StatusCodes: []int{408, 504, 524},
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"timeout", "deadline_exceeded"},
				Message: []string{
					"timeout",
					"timed out",
					"deadline exceeded",
					"context deadline exceeded",
					"i/o timeout",
					"超时",
				},
				Metadata: []string{"timeout", "deadline exceeded"},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 120,
			Description:      "The upstream transport timed out before a usable response.",
		},
		{
			ID:               "upstream_bad_response",
			Enabled:          true,
			Priority:         30,
			Kind:             UpstreamErrorKindBadResponse,
			StatusCodes:      []int{500, 502, 503, 520, 522, 523, 525, 529},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 120,
			Description:      "The provider returned a server-side or malformed upstream response.",
		},
		{
			ID:       "upstream_stream_interrupted",
			Enabled:  true,
			Priority: 20,
			Kind:     UpstreamErrorKindStreamInterrupted,
			Keywords: UpstreamErrorKeywordRule{
				Code: []string{"stream_interrupted", "unexpected_eof"},
				Message: []string{
					"stream interrupted",
					"unexpected eof",
					"incomplete chunk",
					"connection reset",
					"broken pipe",
					"first byte",
					"流中断",
				},
				Metadata: []string{"stream interrupted", "unexpected eof", "connection reset"},
			},
			SchedulerAction:  UpstreamErrorActionSwitchChannel,
			AvoidanceSeconds: 60,
			Description:      "The upstream stream ended before a complete usable output.",
		},
	})
}

func CloneUpstreamErrorRules(rules []UpstreamErrorRule) []UpstreamErrorRule {
	return cloneUpstreamErrorRules(rules)
}

func cloneUpstreamErrorRules(rules []UpstreamErrorRule) []UpstreamErrorRule {
	if rules == nil {
		return nil
	}
	out := make([]UpstreamErrorRule, 0, len(rules))
	for _, rule := range rules {
		rule.StatusCodes = append([]int(nil), rule.StatusCodes...)
		rule.Keywords = cloneUpstreamErrorKeywordRule(rule.Keywords)
		out = append(out, rule)
	}
	return out
}

func cloneUpstreamErrorKeywordRule(keywords UpstreamErrorKeywordRule) UpstreamErrorKeywordRule {
	return UpstreamErrorKeywordRule{
		Code:     append([]string(nil), keywords.Code...),
		Type:     append([]string(nil), keywords.Type...),
		Message:  append([]string(nil), keywords.Message...),
		Metadata: append([]string(nil), keywords.Metadata...),
		Header:   append([]string(nil), keywords.Header...),
	}
}
