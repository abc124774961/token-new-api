package service

import (
	"encoding/base64"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func appendRequestPath(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if other == nil {
		return
	}
	if ctx != nil && ctx.Request != nil && ctx.Request.URL != nil {
		if path := ctx.Request.URL.Path; path != "" {
			other["request_path"] = path
			return
		}
	}
	if relayInfo != nil && relayInfo.RequestURLPath != "" {
		path := relayInfo.RequestURLPath
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}
		other["request_path"] = path
	}
}

func GenerateTextOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64, userGroupRatio float64) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_ratio"] = modelRatio
	other["group_ratio"] = groupRatio
	other["completion_ratio"] = completionRatio
	other["cache_tokens"] = cacheTokens
	other["cache_ratio"] = cacheRatio
	other["model_price"] = modelPrice
	other["user_group_ratio"] = userGroupRatio
	other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
	if reasoningEffort := relayInfo.LogReasoningEffort(); reasoningEffort != "" {
		other["reasoning_effort"] = reasoningEffort
		if relayInfo.ReasoningEffort != "" && relayInfo.ReasoningEffort != reasoningEffort {
			other["upstream_reasoning_effort"] = relayInfo.ReasoningEffort
		}
	}
	appendModelTraceInfo(relayInfo, other)
	if relayInfo.ChannelMeta != nil && relayInfo.IsModelMapped {
		other["is_model_mapped"] = true
		other["request_model_name"] = relayInfo.LogModelName()
		if relayInfo.OriginModelName != relayInfo.LogModelName() {
			other["billing_model_name"] = relayInfo.OriginModelName
		}
		other["upstream_model_name"] = relayInfo.UpstreamModelName
	}

	isSystemPromptOverwritten := common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride)
	if isSystemPromptOverwritten {
		other["is_system_prompt_overwritten"] = true
	}

	adminInfo := make(map[string]interface{})
	adminInfo["use_channel"] = ctx.GetStringSlice("use_channel")
	isMultiKey := common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey)
	if isMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex)
	}
	AppendSelectedChannelAccountAdminInfo(ctx, adminInfo)

	isLocalCountTokens := common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens)
	if isLocalCountTokens {
		adminInfo["local_count_tokens"] = isLocalCountTokens
	}

	AppendChannelAffinityAdminInfo(ctx, adminInfo)
	appendClientRequestTraceInfo(relayInfo, adminInfo)
	appendChannelFailureTrace(ctx, adminInfo)

	other["admin_info"] = adminInfo
	appendRequestPath(ctx, relayInfo, other)
	appendRequestConversionChain(relayInfo, other)
	appendFinalRequestFormat(relayInfo, other)
	appendBillingInfo(relayInfo, other)
	appendParamOverrideInfo(relayInfo, other)
	appendStreamStatus(relayInfo, other)
	return other
}

func appendChannelFailureTrace(ctx *gin.Context, adminInfo map[string]interface{}) {
	if ctx == nil || adminInfo == nil {
		return
	}
	trace, ok := common.GetContextKeyType[[]map[string]interface{}](ctx, constant.ContextKeyChannelFailureTrace)
	if !ok || len(trace) == 0 {
		return
	}
	adminInfo["channel_failures"] = trace
}

func AppendSelectedChannelAccountAdminInfo(ctx *gin.Context, adminInfo map[string]interface{}) {
	if ctx == nil || adminInfo == nil {
		return
	}
	setString := func(name string, key constant.ContextKey) {
		value := strings.TrimSpace(common.GetContextKeyString(ctx, key))
		if value != "" {
			adminInfo[name] = value
		}
	}
	setString("account_uid", constant.ContextKeyChannelAccountCredentialUID)
	setString("account_label", constant.ContextKeyChannelAccountCredentialLabel)
	setString("account_id", constant.ContextKeyChannelAccountID)
	setString("account_identity_key", constant.ContextKeyChannelAccountIdentityKey)
	setString("account_unique_key", constant.ContextKeyChannelAccountUniqueKey)
	setString("account_type", constant.ContextKeyChannelAccountType)
	setString("account_brand", constant.ContextKeyChannelAccountBrand)
	setString("account_provider", constant.ContextKeyChannelAccountProvider)
	setString("credential_subject_fingerprint", constant.ContextKeyChannelAccountCredentialSubjectFP)
	setString("credential_fingerprint", constant.ContextKeyChannelAccountCredentialFP)
}

func appendModelTraceInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	requestModelName := relayInfo.LogModelName()
	if requestModelName != "" {
		other["request_model_name"] = requestModelName
	}
	if relayInfo.ContextModelName != "" && relayInfo.ContextModelName != requestModelName {
		other["context_model_name"] = relayInfo.ContextModelName
	}
	if relayInfo.OriginModelName != "" && relayInfo.OriginModelName != requestModelName {
		other["billing_model_name"] = relayInfo.OriginModelName
	}
	if relayInfo.ChannelMeta != nil && relayInfo.UpstreamModelName != "" {
		other["upstream_model_name"] = relayInfo.UpstreamModelName
	}
	if relayInfo.ResponseModelName != "" {
		other["upstream_response_model_name"] = relayInfo.ResponseModelName
	}
	if relayInfo.DownstreamModelName != "" {
		other["downstream_model_name"] = relayInfo.DownstreamModelName
	}
	if relayInfo.ChannelMeta != nil && relayInfo.IsModelMapped {
		other["is_model_mapped"] = true
	}
}

func appendClientRequestTraceInfo(relayInfo *relaycommon.RelayInfo, adminInfo map[string]interface{}) {
	if relayInfo == nil || adminInfo == nil || len(relayInfo.RequestHeaders) == 0 {
		return
	}
	traceHeaders := make(map[string]string)
	codexLikeClient := false
	metadataHeaderPresent := false
	for key, value := range relayInfo.RequestHeaders {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		normalizedValue := strings.TrimSpace(value)
		if normalizedKey == "" || normalizedValue == "" || isSensitiveHeaderName(normalizedKey) {
			continue
		}
		if strings.Contains(normalizedKey, "codex") || strings.Contains(strings.ToLower(normalizedValue), "codex") {
			codexLikeClient = true
		}
		if strings.Contains(normalizedKey, "metadata") || strings.Contains(normalizedKey, "meta") {
			metadataHeaderPresent = true
		}
		if isSafeClientTraceHeader(normalizedKey) {
			traceHeaders[normalizedKey] = truncateClientTraceValue(normalizedValue)
		}
	}
	if len(traceHeaders) == 0 && !codexLikeClient && !metadataHeaderPresent {
		return
	}
	clientTrace := map[string]interface{}{}
	if len(traceHeaders) > 0 {
		clientTrace["headers"] = traceHeaders
	}
	if codexLikeClient {
		clientTrace["codex_like_client"] = true
	}
	if metadataHeaderPresent {
		clientTrace["metadata_header_present"] = true
	}
	adminInfo["client_request"] = clientTrace
}

func isSafeClientTraceHeader(key string) bool {
	if key == "user-agent" || key == "content-type" || key == "openai-beta" {
		return true
	}
	return strings.HasPrefix(key, "x-stainless-") ||
		strings.HasPrefix(key, "x-codex-") ||
		strings.HasPrefix(key, "codex-")
}

func isSensitiveHeaderName(key string) bool {
	for _, marker := range []string{"authorization", "api-key", "apikey", "token", "cookie", "secret", "key"} {
		if strings.Contains(key, marker) {
			return true
		}
	}
	return false
}

func truncateClientTraceValue(value string) string {
	const maxClientTraceValueLen = 240
	if len(value) <= maxClientTraceValueLen {
		return value
	}
	return value[:maxClientTraceValueLen] + "...(truncated)"
}

func appendParamOverrideInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || len(relayInfo.ParamOverrideAudit) == 0 {
		return
	}
	other["po"] = relayInfo.ParamOverrideAudit
}

func appendStreamStatus(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || !relayInfo.IsStream || relayInfo.StreamStatus == nil {
		return
	}
	ss := relayInfo.StreamStatus
	status := "ok"
	if ss.EndReason == relaycommon.StreamEndReasonClientGone {
		status = "client_gone"
	} else if !ss.IsNormalEnd() || ss.HasErrors() {
		status = "error"
	}
	streamInfo := map[string]interface{}{
		"status":     status,
		"end_reason": string(ss.EndReason),
	}
	if ss.EndError != nil {
		streamInfo["end_error"] = ss.EndError.Error()
	}
	if ss.ErrorCount > 0 {
		streamInfo["error_count"] = ss.ErrorCount
		messages := make([]string, 0, len(ss.Errors))
		for _, e := range ss.Errors {
			messages = append(messages, e.Message)
		}
		streamInfo["errors"] = messages
	}
	other["stream_status"] = streamInfo
}

func appendBillingInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if snapshot := relayInfo.DynamicBilling; snapshot != nil {
		other["dynamic_billing_applied"] = snapshot.Applied
		other["dynamic_billing_group"] = snapshot.Group
		other["dynamic_billing_static_group_ratio"] = snapshot.StaticGroupRatio
		if snapshot.CostSource != "" {
			other["dynamic_billing_cost_source"] = snapshot.CostSource
		}
		if snapshot.ApplyMode != "" {
			other["dynamic_billing_apply_mode"] = snapshot.ApplyMode
		}
		if snapshot.ApplyReason != "" {
			other["dynamic_billing_apply_reason"] = snapshot.ApplyReason
		}
		if snapshot.OperatingCostUSD > 0 {
			other["dynamic_billing_operating_cost_usd"] = snapshot.OperatingCostUSD
		}
		if snapshot.RequiredRevenueUSD > 0 {
			other["dynamic_billing_required_revenue_usd"] = snapshot.RequiredRevenueUSD
		}
		if snapshot.BaseQuotaAtRatio1 > 0 {
			other["dynamic_billing_base_quota_at_ratio_1"] = snapshot.BaseQuotaAtRatio1
		}
		if snapshot.CostMultiplier > 0 {
			other["dynamic_billing_cost_multiplier"] = snapshot.CostMultiplier
		}
		if snapshot.TargetRatio > 0 {
			other["dynamic_billing_target_ratio"] = snapshot.TargetRatio
		}
		if snapshot.EffectiveRatio > 0 {
			other["dynamic_billing_effective_ratio"] = snapshot.EffectiveRatio
		}
		if snapshot.Clamped {
			other["dynamic_billing_clamped"] = true
		}
		if snapshot.PendingManualConfirm {
			other["dynamic_billing_pending_manual_confirm"] = true
		}
		if snapshot.Applied {
			other["billing_mode"] = "model_gateway_dynamic"
			other["billing_source_detail"] = "dynamic_group_ratio"
			other["dynamic_billing_ratio"] = snapshot.DynamicRatio
			if snapshot.PricePerM > 0 {
				other["dynamic_billing_price_per_m"] = snapshot.PricePerM
			}
			other["dynamic_profit_rate"] = snapshot.ProfitRate
			other["dynamic_billing_sample_count"] = snapshot.SampleCount
			other["dynamic_billing_calculated_at"] = snapshot.CalculatedAt
			other["dynamic_billing_window_start"] = snapshot.WindowStart
			other["dynamic_billing_window_end"] = snapshot.WindowEnd
			if snapshot.RequestCount > 0 {
				other["dynamic_billing_request_count"] = snapshot.RequestCount
			}
			if snapshot.SuccessRequestCount > 0 {
				other["dynamic_billing_success_request_count"] = snapshot.SuccessRequestCount
			}
			if snapshot.TotalTokens > 0 {
				other["dynamic_billing_total_tokens"] = snapshot.TotalTokens
			}
		} else if snapshot.FallbackReason != "" {
			other["dynamic_billing_fallback"] = true
			other["dynamic_fallback_reason"] = snapshot.FallbackReason
		}
	}
	// billing_source: "wallet", "subscription", or "subscription_wallet"
	if relayInfo.BillingSource != "" {
		other["billing_source"] = relayInfo.BillingSource
	}
	if relayInfo.UserSetting.BillingPreference != "" {
		other["billing_preference"] = relayInfo.UserSetting.BillingPreference
	}
	if relayInfo.BillingSource == BillingSourceSubscription || relayInfo.BillingSource == BillingSourceSubscriptionWallet {
		if relayInfo.SubscriptionId != 0 {
			other["subscription_id"] = relayInfo.SubscriptionId
		}
		if relayInfo.SubscriptionPreConsumed > 0 {
			other["subscription_pre_consumed"] = relayInfo.SubscriptionPreConsumed
		}
		// post_delta: settlement delta applied after actual usage is known (can be negative for refund)
		if relayInfo.SubscriptionPostDelta != 0 {
			other["subscription_post_delta"] = relayInfo.SubscriptionPostDelta
		}
		if relayInfo.SubscriptionPlanId != 0 {
			other["subscription_plan_id"] = relayInfo.SubscriptionPlanId
		}
		if relayInfo.SubscriptionPlanTitle != "" {
			other["subscription_plan_title"] = relayInfo.SubscriptionPlanTitle
		}
		// Compute "this request" subscription consumed + remaining
		consumed := relayInfo.SubscriptionPreConsumed + relayInfo.SubscriptionPostDelta
		usedFinal := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		if consumed < 0 {
			consumed = 0
		}
		if usedFinal < 0 {
			usedFinal = 0
		}
		if relayInfo.SubscriptionAmountTotal > 0 {
			remain := relayInfo.SubscriptionAmountTotal - usedFinal
			if remain < 0 {
				remain = 0
			}
			other["subscription_total"] = relayInfo.SubscriptionAmountTotal
			other["subscription_used"] = usedFinal
			other["subscription_remain"] = remain
		}
		if consumed > 0 {
			other["subscription_consumed"] = consumed
		}
		if relayInfo.BillingSource == BillingSourceSubscriptionWallet {
			other["wallet_quota_deducted"] = relayInfo.WalletConsumed
		} else {
			// Wallet quota is not deducted when billed from subscription.
			other["wallet_quota_deducted"] = 0
		}
	}
}

func appendRequestConversionChain(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if len(relayInfo.RequestConversionChain) == 0 {
		return
	}
	chain := make([]string, 0, len(relayInfo.RequestConversionChain))
	for _, f := range relayInfo.RequestConversionChain {
		switch f {
		case types.RelayFormatOpenAI:
			chain = append(chain, "OpenAI Compatible")
		case types.RelayFormatClaude:
			chain = append(chain, "Claude Messages")
		case types.RelayFormatGemini:
			chain = append(chain, "Google Gemini")
		case types.RelayFormatOpenAIResponses:
			chain = append(chain, "OpenAI Responses")
		default:
			chain = append(chain, string(f))
		}
	}
	if len(chain) == 0 {
		return
	}
	other["request_conversion"] = chain
}

func appendFinalRequestFormat(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		// claude indicates the final upstream request format is Claude Messages.
		// Frontend log rendering uses this to keep the original Claude input display.
		other["claude"] = true
	}
}

func GenerateWssOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["ws"] = true
	info["audio_input"] = usage.InputTokenDetails.AudioTokens
	info["audio_output"] = usage.OutputTokenDetails.AudioTokens
	info["text_input"] = usage.InputTokenDetails.TextTokens
	info["text_output"] = usage.OutputTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateAudioOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["audio"] = true
	info["audio_input"] = usage.PromptTokensDetails.AudioTokens
	info["audio_output"] = usage.CompletionTokenDetails.AudioTokens
	info["text_input"] = usage.PromptTokensDetails.TextTokens
	info["text_output"] = usage.CompletionTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateClaudeOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64,
	cacheCreationTokens int, cacheCreationRatio float64,
	cacheCreationTokens5m int, cacheCreationRatio5m float64,
	cacheCreationTokens1h int, cacheCreationRatio1h float64,
	modelPrice float64, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, userGroupRatio)
	info["claude"] = true
	info["cache_creation_tokens"] = cacheCreationTokens
	info["cache_creation_ratio"] = cacheCreationRatio
	if cacheCreationTokens5m != 0 {
		info["cache_creation_tokens_5m"] = cacheCreationTokens5m
		info["cache_creation_ratio_5m"] = cacheCreationRatio5m
	}
	if cacheCreationTokens1h != 0 {
		info["cache_creation_tokens_1h"] = cacheCreationTokens1h
		info["cache_creation_ratio_1h"] = cacheCreationRatio1h
	}
	return info
}

func GenerateMjOtherInfo(relayInfo *relaycommon.RelayInfo, priceData types.PriceData) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_price"] = priceData.ModelPrice
	other["group_ratio"] = priceData.GroupRatioInfo.GroupRatio
	if priceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = priceData.GroupRatioInfo.GroupSpecialRatio
	}
	if guard := priceData.FixedPriceMarginGuard; guard != nil && guard.Applied {
		other["fixed_price_margin_guard"] = true
		other["fixed_price_margin_guard_original_group_ratio"] = guard.OriginalGroupRatio
		other["fixed_price_margin_guard_group_ratio"] = guard.ProtectedGroupRatio
		other["fixed_price_margin_guard_cost_usd"] = guard.CostUSD
		other["fixed_price_margin_guard_target_margin"] = guard.TargetMargin
		other["fixed_price_margin_guard_min_revenue_usd"] = guard.MinRevenueUSD
		other["fixed_price_margin_guard_profile_id"] = guard.ProfileID
		other["fixed_price_margin_guard_profile_model"] = guard.ProfileModel
		other["fixed_price_margin_guard_profile_source"] = guard.ProfileSource
		other["fixed_price_margin_guard_profile_accuracy"] = guard.ProfileAccuracy
	}
	appendRequestPath(nil, relayInfo, other)
	return other
}

func annotateHealthProbeLog(ctx *gin.Context, other map[string]interface{}) {
	if ctx == nil || other == nil || !common.GetContextKeyBool(ctx, constant.ContextKeyHealthProbe) {
		return
	}
	other["is_health_probe"] = true
	other["billing_source"] = "model_gateway_probe"
	other["source"] = "health_probe"
	if probeID := strings.TrimSpace(common.GetContextKeyString(ctx, common.RequestIdKey)); probeID != "" {
		other["probe_id"] = probeID
	}
	if reason := strings.TrimSpace(common.GetContextKeyString(ctx, constant.ContextKeyHealthProbeReason)); reason != "" {
		other["probe_reason"] = reason
	}
	if key, ok := common.GetContextKey(ctx, constant.ContextKeyHealthProbeRuntimeKey); ok {
		other["runtime_key"] = key
	}
}

// InjectTieredBillingInfo overlays tiered billing fields onto an existing
// module-specific other map. Call this after GenerateTextOtherInfo /
// GenerateClaudeOtherInfo / etc. when the request used tiered_expr billing.
func InjectTieredBillingInfo(other map[string]interface{}, relayInfo *relaycommon.RelayInfo, result *billingexpr.TieredResult) {
	if relayInfo == nil || other == nil {
		return
	}
	snap := relayInfo.TieredBillingSnapshot
	if snap == nil {
		return
	}
	other["billing_mode"] = "tiered_expr"
	other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
	if result != nil {
		other["matched_tier"] = result.MatchedTier
	}
}
