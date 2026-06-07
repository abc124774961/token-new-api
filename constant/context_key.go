package constant

type ContextKey string

const (
	ContextKeyTokenCountMeta  ContextKey = "token_count_meta"
	ContextKeyPromptTokens    ContextKey = "prompt_tokens"
	ContextKeyEstimatedTokens ContextKey = "estimated_tokens"

	ContextKeyOriginalModel    ContextKey = "original_model"
	ContextKeyRequestStartTime ContextKey = "request_start_time"

	/* token related keys */
	ContextKeyTokenUnlimited         ContextKey = "token_unlimited_quota"
	ContextKeyTokenKey               ContextKey = "token_key"
	ContextKeyTokenId                ContextKey = "token_id"
	ContextKeyTokenGroup             ContextKey = "token_group"
	ContextKeyTokenSpecificChannelId ContextKey = "specific_channel_id"
	ContextKeyTokenModelLimitEnabled ContextKey = "token_model_limit_enabled"
	ContextKeyTokenModelLimit        ContextKey = "token_model_limit"
	ContextKeyTokenCrossGroupRetry   ContextKey = "token_cross_group_retry"

	/* channel related keys */
	ContextKeyChannelId                         ContextKey = "channel_id"
	ContextKeyChannelName                       ContextKey = "channel_name"
	ContextKeyChannelCreateTime                 ContextKey = "channel_create_time"
	ContextKeyChannelBaseUrl                    ContextKey = "base_url"
	ContextKeyChannelType                       ContextKey = "channel_type"
	ContextKeyChannelSetting                    ContextKey = "channel_setting"
	ContextKeyChannelOtherSetting               ContextKey = "channel_other_setting"
	ContextKeyChannelParamOverride              ContextKey = "param_override"
	ContextKeyChannelHeaderOverride             ContextKey = "header_override"
	ContextKeyChannelOrganization               ContextKey = "channel_organization"
	ContextKeyChannelAutoBan                    ContextKey = "auto_ban"
	ContextKeyChannelModelMapping               ContextKey = "model_mapping"
	ContextKeyChannelStatusCodeMapping          ContextKey = "status_code_mapping"
	ContextKeyChannelIsMultiKey                 ContextKey = "channel_is_multi_key"
	ContextKeyChannelMultiKeyIndex              ContextKey = "channel_multi_key_index"
	ContextKeyChannelKey                        ContextKey = "channel_key"
	ContextKeyChannelAccountCapability          ContextKey = "channel_account_capability"
	ContextKeyChannelAccountProxyID             ContextKey = "channel_account_proxy_id"
	ContextKeyChannelAccountProxyURL            ContextKey = "channel_account_proxy_url"
	ContextKeyChannelAccountCodexEnvironmentID  ContextKey = "channel_account_codex_environment_id"
	ContextKeyChannelAccountID                  ContextKey = "channel_account_id"
	ContextKeyChannelAccountIdentityKey         ContextKey = "channel_account_identity_key"
	ContextKeyChannelAccountUniqueKey           ContextKey = "channel_account_unique_key"
	ContextKeyChannelAccountType                ContextKey = "channel_account_type"
	ContextKeyChannelAccountBrand               ContextKey = "channel_account_brand"
	ContextKeyChannelAccountProvider            ContextKey = "channel_account_provider"
	ContextKeyChannelAccountCredentialSubjectFP ContextKey = "channel_account_credential_subject_fingerprint"
	ContextKeyChannelAccountCredentialFP        ContextKey = "channel_account_credential_fingerprint"
	ContextKeyChannelAccountCredentialUID       ContextKey = "channel_account_credential_uid"
	ContextKeyChannelAccountCredentialLabel     ContextKey = "channel_account_credential_label"
	ContextKeyUpstreamRequestInfo               ContextKey = "upstream_request_info"
	ContextKeyUpstreamResponseHeaderMs          ContextKey = "upstream_response_header_ms"
	ContextKeyChannelFailureTrace               ContextKey = "channel_failure_trace"
	ContextKeyChannelBalanceSkipped             ContextKey = "channel_balance_skipped"
	ContextKeyChannelRuntimeBalanceSkipped      ContextKey = "channel_runtime_balance_skipped"
	ContextKeyChannelSelectionReserved          ContextKey = "channel_selection_reserved"
	ContextKeyChannelSelectionSkipped           ContextKey = "channel_selection_skipped"
	ContextKeyChannelRuntimeSelectionSkipped    ContextKey = "channel_runtime_selection_skipped"
	ContextKeyChannelRuntimeAttempted           ContextKey = "channel_runtime_attempted"
	ContextKeyChannelFirstByteWaitLease         ContextKey = "channel_first_byte_wait_lease"
	ContextKeyChannelConcurrencyLease           ContextKey = "channel_concurrency_lease"
	ContextKeyRelayAttemptControl               ContextKey = "relay_attempt_control"
	ContextKeyRelayDownstreamStarted            ContextKey = "relay_downstream_started"
	ContextKeyRelayDownstreamWriteStatus        ContextKey = "relay_downstream_write_status"
	ContextKeyRelayDownstreamKeepAliveCount     ContextKey = "relay_downstream_keepalive_count"
	ContextKeyRelayJSONKeepAliveStarted         ContextKey = "relay_json_keepalive_started"
	ContextKeyRelayClientReceivedStarted        ContextKey = "relay_client_received_started"
	ContextKeyRelayFinalClassification          ContextKey = "relay_final_classification"
	ContextKeyRelayUpstreamStatus               ContextKey = "relay_upstream_status"
	ContextKeyRelayResponseStarted              ContextKey = "relay_response_started"
	ContextKeyRelayStreamInterrupted            ContextKey = "relay_stream_interrupted"
	ContextKeyRelayEmptyOutput                  ContextKey = "relay_empty_output"
	ContextKeyRelayExperienceIssue              ContextKey = "relay_experience_issue"
	ContextKeyRelayFinalPromptTokens            ContextKey = "relay_final_prompt_tokens"
	ContextKeyRelayFinalCompletionTokens        ContextKey = "relay_final_completion_tokens"
	ContextKeyRelayInfo                         ContextKey = "relay_info"
	ContextKeyProviderSurface                   ContextKey = "provider_surface"
	ContextKeyCapabilityClassification          ContextKey = "capability_classification"
	ContextKeyUsageEstimated                    ContextKey = "usage_estimated"
	ContextKeyCodexUpstreamStreamForced         ContextKey = "codex_upstream_stream_forced"
	ContextKeyHealthProbe                       ContextKey = "health_probe"
	ContextKeyHealthProbeReason                 ContextKey = "health_probe_reason"
	ContextKeyHealthProbeRuntimeKey             ContextKey = "health_probe_runtime_key"
	ContextKeyResponsesPreviousID               ContextKey = "responses_previous_id_required"

	ContextKeyAutoGroup           ContextKey = "auto_group"
	ContextKeyAutoGroupIndex      ContextKey = "auto_group_index"
	ContextKeyAutoGroupRetryIndex ContextKey = "auto_group_retry_index"
	ContextKeyForceNextAutoGroup  ContextKey = "force_next_auto_group"

	/* user related keys */
	ContextKeyUserId      ContextKey = "id"
	ContextKeyUserSetting ContextKey = "user_setting"
	ContextKeyUserQuota   ContextKey = "user_quota"
	ContextKeyUserStatus  ContextKey = "user_status"
	ContextKeyUserEmail   ContextKey = "user_email"
	ContextKeyUserGroup   ContextKey = "user_group"
	ContextKeyUsingGroup  ContextKey = "group"
	ContextKeyUserName    ContextKey = "username"

	ContextKeyLocalCountTokens ContextKey = "local_count_tokens"

	ContextKeySystemPromptOverride ContextKey = "system_prompt_override"

	// ContextKeyFileSourcesToCleanup stores file sources that need cleanup when request ends
	ContextKeyFileSourcesToCleanup ContextKey = "file_sources_to_cleanup"

	// ContextKeyAdminRejectReason stores an admin-only reject/block reason extracted from upstream responses.
	// It is not returned to end users, but can be persisted into consume/error logs for debugging.
	ContextKeyAdminRejectReason ContextKey = "admin_reject_reason"

	// ContextKeyLanguage stores the user's language preference for i18n
	ContextKeyLanguage ContextKey = "language"
	ContextKeyIsStream ContextKey = "is_stream"
)
