package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	// Import oauth package to register providers via init()
	_ "github.com/QuantumNous/new-api/oauth"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	SetRealtimeRouter(router)

	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.RouteTag("api"))
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", controller.PostSetup)
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/public/home/status", controller.GetPublicHomeStatus)
		apiRouter.GET("/public/subscription/plans", controller.GetPublicSubscriptionPlans)
		apiRouter.GET("/pricing", middleware.TryUserAuth(), controller.GetPricing)
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.TryUserAuth())
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary)
			perfMetricsRoute.GET("", controller.GetPerfMetrics)
		}
		apiRouter.GET("/rankings", controller.GetRankings)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.GET("/oauth/state", middleware.CriticalRateLimit(), controller.GenerateOAuthCode)
		apiRouter.POST("/oauth/email/bind", middleware.CriticalRateLimit(), controller.EmailBind)
		// Non-standard OAuth (WeChat, Telegram) - keep original routes
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), controller.TelegramLogin)
		apiRouter.GET("/oauth/telegram/bind", middleware.CriticalRateLimit(), controller.TelegramBind)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO) - unified route
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", controller.CreemWebhook)
		apiRouter.POST("/waffo/webhook", controller.WaffoWebhook)
		//apiRouter.POST("/waffo-pancake/webhook", controller.WaffoPancakeWebhook)
		apiRouter.POST("/internal/token-account-automation/credential", controller.TokenAccountAutomationCredentialWriteback)
		apiRouter.GET("/internal/token-account-automation/proxies", controller.ListTokenAccountAutomationProxies)
		apiRouter.GET("/internal/token-account-automation/account-profile", controller.TokenAccountAutomationAccountProfile)
		apiRouter.GET("/internal/electron-browser/accounts", controller.ListElectronBrowserAccounts)
		apiRouter.GET("/internal/token-account-automation/account-pools/invalid", controller.TokenAccountAutomationListInvalidAccounts)
		apiRouter.POST("/internal/token-account-automation/account-pools/invalid/archive", controller.TokenAccountAutomationArchiveInvalidAccount)
		apiRouter.POST("/internal/token-account-automation/account-pools/invalid/:id/reauthorize", controller.TokenAccountAutomationReauthorizeInvalidAccount)
		apiRouter.POST("/internal/token-account-automation/account-pools/discarded/archive", controller.TokenAccountAutomationArchiveDiscardedAccount)

		// Universal secure verification routes
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.UniversalVerify)

		adminPermissionRoute := apiRouter.Group("/admin/permissions")
		adminPermissionRoute.Use(middleware.AdminAuth())
		{
			adminPermissionRoute.GET("/config", controller.GetAdminPermissionConfig)
			adminPermissionRoute.GET("/self", controller.GetAdminSelfPermissions)
			adminPermissionRoute.GET("/roles", middleware.RequireAdminPermission(middleware.AdminPermissionSystemRolesRead), controller.ListAdminPermissionRoles)
			adminPermissionRoute.POST("/roles/sync-templates", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemRolesUpdate), controller.SyncAdminPermissionRoleTemplates)
			adminPermissionRoute.POST("/roles", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemRolesUpdate), controller.CreateAdminPermissionRole)
			adminPermissionRoute.PUT("/roles/:id", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemRolesUpdate), controller.UpdateAdminPermissionRole)
			adminPermissionRoute.DELETE("/roles/:id", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemRolesUpdate), controller.DisableAdminPermissionRole)
			adminPermissionRoute.GET("/users/:id", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemRolesUpdate), controller.GetAdminUserPermissionAssignment)
			adminPermissionRoute.PUT("/users/:id", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemRolesUpdate), controller.UpdateAdminUserPermissionAssignment)
		}

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/register", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Register)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), controller.Verify2FALogin)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.GET("/logout", controller.Logout)
			userRoute.POST("/epay/notify", controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.UserAuth())
			{
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", controller.UpdateSelf)
				selfRoute.DELETE("/self", controller.DeleteSelf)
				selfRoute.GET("/token", controller.GenerateAccessToken)
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/aff/dashboard", controller.GetAffDashboard)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)
				//selfRoute.POST("/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
				//selfRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPancakePay)
				selfRoute.POST("/aff_transfer", controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", controller.Setup2FA)
				selfRoute.POST("/2fa/enable", controller.Enable2FA)
				selfRoute.POST("/2fa/disable", controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin)

				// Custom OAuth bindings
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSettlementComplete), controller.AdminCompleteTopUp)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)
				adminRoute.DELETE("/:id/oauth/bindings/:provider_id", middleware.RequireAdminPermission(middleware.AdminPermissionUserUserDanger), controller.UnbindCustomOAuthByAdmin)
				adminRoute.DELETE("/:id/bindings/:binding_type", middleware.RequireAdminPermission(middleware.AdminPermissionUserUserDanger), controller.AdminClearUserBinding)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", middleware.RequireAdminPermission(middleware.AdminPermissionUserUserDanger), controller.CreateUser)
				adminRoute.POST("/manage", middleware.RequireAdminPermission(middleware.AdminPermissionUserUserDanger), controller.ManageUser)
				adminRoute.PUT("/", middleware.RequireAdminPermission(middleware.AdminPermissionUserUserDanger), controller.UpdateUser)
				adminRoute.DELETE("/:id", middleware.RequireAdminPermission(middleware.AdminPermissionUserUserDanger), controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", middleware.RequireAdminPermission(middleware.AdminPermissionUserUserDanger), controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", middleware.RequireAdminPermission(middleware.AdminPermissionUserUserDanger), controller.AdminDisable2FA)
			}
		}

		// Subscription billing (plans, purchase, admin management)
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/plans", controller.GetSubscriptionPlans)
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
			subscriptionRoute.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)
			subscriptionRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)
			subscriptionRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSubscriptionUpdate), controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSubscriptionUpdate), controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSubscriptionUpdate), controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.POST("/bind", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSubscriptionUpdate), controller.AdminBindSubscription)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSubscriptionUpdate), controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSubscriptionUpdate), controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialSubscriptionUpdate), controller.AdminDeleteUserSubscription)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", controller.SubscriptionEpayReturn)
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.PUT("/", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemSettingsUpdate), controller.UpdateOption)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", middleware.RequireRootAdminPermission(middleware.AdminPermissionModelRoutePolicyDanger), controller.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", middleware.RequireRootAdminPermission(middleware.AdminPermissionModelRatioUpdate), controller.ResetModelRatio)
			optionRoute.POST("/migrate_console_setting", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemSettingsUpdate), controller.MigrateConsoleSetting) // 用于迁移检测的旧键，下个版本会删除
		}

		// Custom OAuth provider management (root only)
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.POST("/discovery", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemSettingsUpdate), controller.FetchCustomOAuthDiscovery)
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)
			customOAuthRoute.POST("/", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemSettingsUpdate), controller.CreateCustomOAuthProvider)
			customOAuthRoute.PUT("/:id", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemSettingsUpdate), controller.UpdateCustomOAuthProvider)
			customOAuthRoute.DELETE("/:id", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemSettingsUpdate), controller.DeleteCustomOAuthProvider)
		}
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemPerformanceDanger), controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemPerformanceDanger), controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemPerformanceDanger), controller.ForceGC)
			performanceRoute.GET("/logs", controller.GetLogFiles)
			performanceRoute.DELETE("/logs", middleware.RequireRootAdminPermission(middleware.AdminPermissionSystemPerformanceDanger), controller.CleanupLogFiles)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", middleware.RequireRootAdminPermission(middleware.AdminPermissionModelRatioUpdate), controller.FetchUpstreamRatios)
			ratioSyncRoute.POST("/channel_cost/apply", middleware.RequireRootAdminPermission(middleware.AdminPermissionModelRatioUpdate), controller.ApplyUpstreamChannelCostSync)
		}
		apiRouter.GET("/channel/status_monitor", middleware.UserAuth(), controller.GetChannelStatusMonitor)
		apiRouter.GET("/channel/balance_monitor", middleware.AdminAuth(), controller.GetChannelBalanceMonitor)
		apiRouter.GET("/channel/balance_monitor/events", middleware.AdminAuth(), controller.GetChannelBalanceMonitorEvents)
		apiRouter.POST("/channel/balance_monitor/refresh", middleware.AdminAuth(), middleware.RequireAdminPermission(middleware.AdminPermissionChannelHealthExecute), controller.RefreshChannelBalanceMonitor)
		channelRoute := apiRouter.Group("/channel")
		channelRoute.Use(middleware.AdminAuth())
		{
			channelRoute.GET("/", controller.GetAllChannels)
			channelRoute.GET("/search", controller.SearchChannels)
			channelRoute.GET("/models", controller.ChannelListModels)
			channelRoute.GET("/models_enabled", controller.EnabledListModels)
			channelRoute.GET("/group_summary", controller.GetChannelGroupSummary)
			channelRoute.GET("/codex-environments", controller.ListCodexApplicationEnvironments)
			channelRoute.GET("/accounts", controller.ListAllChannelAccounts)
			channelRoute.POST("/accounts/auth-recovery/sync", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.SyncChannelAccountAuthRecovery)
			channelRoute.GET("/account-pools/invalid", controller.ListChannelInvalidAccountPool)
			channelRoute.GET("/account-pools/discarded", controller.ListChannelDiscardedAccountPool)
			channelRoute.POST("/account-pools/invalid/archive", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.ArchiveChannelAccountsToInvalidPool)
			channelRoute.POST("/account-pools/discarded/archive", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.ArchiveChannelAccountsToDiscardedPool)
			channelRoute.POST("/account-pools/invalid/:id/restore", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.RestoreChannelInvalidAccount)
			channelRoute.POST("/account-pools/invalid/:id/reauthorize", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.ReauthorizeChannelInvalidAccount)
			channelRoute.POST("/account-pools/invalid/:id/discard", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.DiscardChannelInvalidAccount)
			channelRoute.DELETE("/account-pools/invalid/:id", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.DeleteChannelInvalidAccountPoolItem)
			channelRoute.DELETE("/account-pools/discarded/:id", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.DeleteChannelDiscardedAccountPoolItem)
			channelRoute.POST("/upstream_cost_quote", controller.GetUnsavedChannelUpstreamCostQuote)
			channelRoute.GET("/:id/upstream_cost_quote", controller.GetChannelUpstreamCostQuote)
			channelRoute.GET("/:id/upstream_cost_profiles", controller.ListChannelUpstreamCostProfiles)
			channelRoute.POST("/:id/upstream_cost_profiles", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.SaveChannelUpstreamCostProfile)
			channelRoute.PUT("/:id/upstream_cost_profiles", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.SaveChannelUpstreamCostProfile)
			channelRoute.POST("/:id/upstream_cost_recalculate", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelDanger), controller.RecalculateChannelUpstreamCost)
			channelRoute.DELETE("/:id/upstream_cost_profiles/:profile_id", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelDanger), controller.DeleteChannelUpstreamCostProfile)
			channelRoute.GET("/:id/accounts", controller.ListChannelAccounts)
			channelRoute.POST("/:id/accounts", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.UpdateChannelAccountsStatus)
			channelRoute.PUT("/:id/accounts", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.ImportChannelAccounts)
			channelRoute.DELETE("/:id/accounts", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.DeleteChannelAccounts)
			channelRoute.POST("/:id/account-proxies", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.UpdateChannelAccountsProxy)
			channelRoute.PUT("/:id/accounts/:credential_index", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.UpdateChannelAccountCredential)
			channelRoute.GET("/:id/accounts/:credential_index/requests", controller.ListChannelAccountRecentRequests)
			channelRoute.GET("/:id/accounts/:credential_index/requests/:request_id/reconcile", controller.GetChannelAccountRequestReconcile)
			channelRoute.POST("/:id/accounts/:credential_index/refresh-attribution", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.RefreshChannelAccountUsageAttribution)
			channelRoute.POST("/:id/accounts/:credential_index/status", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.UpdateChannelAccountStatus)
			channelRoute.POST("/:id/accounts/:credential_index/proxy", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.UpdateChannelAccountProxy)
			channelRoute.GET("/:id", controller.GetChannel)
			channelRoute.POST("/:id/key", middleware.RootAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.SecureVerificationRequired(), controller.GetChannelKey)
			channelRoute.GET("/test", controller.TestAllChannels)
			channelRoute.GET("/test/:id", controller.TestChannel)
			channelRoute.GET("/update_balance", controller.UpdateAllChannelsBalance)
			channelRoute.GET("/update_balance/:id", controller.UpdateChannelBalance)
			channelRoute.POST("/", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.AddChannel)
			channelRoute.PUT("/", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.UpdateChannel)
			channelRoute.DELETE("/disabled", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelDanger), controller.DeleteDisabledChannel)
			channelRoute.POST("/tag/disabled", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.DisableTagChannels)
			channelRoute.POST("/tag/enabled", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.EnableTagChannels)
			channelRoute.PUT("/tag", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.EditTagChannels)
			channelRoute.DELETE("/:id", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelDanger), controller.DeleteChannel)
			channelRoute.POST("/batch", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelDanger), controller.DeleteChannelBatch)
			channelRoute.POST("/fix", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.FixChannelsAbilities)
			channelRoute.GET("/fetch_models/:id", controller.FetchUpstreamModels)
			channelRoute.POST("/fetch_models", middleware.RootAuth(), middleware.RequireRootAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.FetchModels)
			channelRoute.POST("/codex/image_generation_tool/probe", middleware.RootAuth(), middleware.RequireRootAdminPermission(middleware.AdminPermissionChannelHealthExecute), controller.ProbeUnsavedChannelCodexImageGenerationTool)
			channelRoute.POST("/:id/codex/image_generation_tool/probe", middleware.RequireAdminPermission(middleware.AdminPermissionChannelHealthExecute), controller.ProbeChannelCodexImageGenerationTool)
			channelRoute.POST("/codex/oauth/start", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.StartCodexOAuth)
			channelRoute.POST("/codex/oauth/complete", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.CompleteCodexOAuth)
			channelRoute.POST("/:id/codex/oauth/start", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.StartCodexOAuthForChannel)
			channelRoute.POST("/:id/codex/oauth/complete", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.CompleteCodexOAuthForChannel)
			channelRoute.POST("/:id/codex/refresh", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.RefreshCodexChannelCredential)
			channelRoute.GET("/:id/codex/usage", controller.GetCodexChannelUsage)
			channelRoute.POST("/:id/clear_failure_avoidance", middleware.RequireAdminPermission(middleware.AdminPermissionChannelHealthExecute), controller.ClearChannelFailureAvoidance)
			channelRoute.POST("/:id/recover_health", middleware.RequireAdminPermission(middleware.AdminPermissionChannelHealthExecute), controller.RecoverChannelHealth)
			channelRoute.POST("/ollama/pull", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.OllamaPullModel)
			channelRoute.POST("/ollama/pull/stream", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.OllamaPullModelStream)
			channelRoute.DELETE("/ollama/delete", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelDanger), controller.OllamaDeleteModel)
			channelRoute.GET("/ollama/version/:id", controller.OllamaVersion)
			channelRoute.POST("/batch/tag", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.BatchSetChannelTag)
			channelRoute.GET("/tag/models", controller.GetTagModels)
			channelRoute.POST("/copy/:id", middleware.RequireAdminPermission(middleware.AdminPermissionChannelChannelUpdate), controller.CopyChannel)
			channelRoute.POST("/multi_key/manage", middleware.RequireAdminPermission(middleware.AdminPermissionChannelAccountDanger), controller.ManageMultiKeys)
			channelRoute.POST("/upstream_updates/apply", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.ApplyChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/apply_all", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.ApplyAllChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/detect", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.DetectChannelUpstreamModelUpdates)
			channelRoute.POST("/upstream_updates/detect_all", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.DetectAllChannelUpstreamModelUpdates)
		}
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.GET("/:id/models", controller.GetTokenModels)
			tokenRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKey)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/batch/keys", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKeysBatch)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthReadOnly())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialRedemptionUpdate), controller.AddRedemption)
			redemptionRoute.PUT("/", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialRedemptionUpdate), controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialRedemptionUpdate), controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialRedemptionUpdate), controller.DeleteRedemption)
		}
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.DELETE("/", middleware.AdminAuth(), middleware.RequireAdminPermission(middleware.AdminPermissionSystemPerformanceDanger), controller.DeleteHistoryLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/channel_affinity_diagnostics", middleware.AdminAuth(), controller.GetChannelAffinityDiagnostics)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/users", middleware.AdminAuth(), controller.GetQuotaDatesByUser)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)

		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		modelGatewayRoute := apiRouter.Group("/model_gateway")
		modelGatewayRoute.Use(middleware.AdminAuth())
		{
			modelGatewayRoute.GET("/config", controller.GetModelGatewayConfig)
			modelGatewayRoute.PUT("/config", middleware.RequireAdminPermission(middleware.AdminPermissionModelRoutePolicyDanger), controller.UpdateModelGatewayConfig)
			modelGatewayRoute.PATCH("/config/probe", middleware.RequireAdminPermission(middleware.AdminPermissionModelRoutePolicyDanger), controller.UpdateModelGatewayProbeConfig)
			modelGatewayRoute.POST("/config/reset", middleware.RequireAdminPermission(middleware.AdminPermissionModelRoutePolicyDanger), controller.ResetModelGatewayConfig)
			modelGatewayRoute.POST("/dynamic_billing/confirm", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.ConfirmModelGatewayDynamicBilling)
			modelGatewayRoute.GET("/channels/:id/score_boosts", controller.GetModelGatewayChannelScoreBoosts)
			modelGatewayRoute.PATCH("/channels/:id/score_boosts", middleware.RequireAdminPermission(middleware.AdminPermissionModelGatewayUpdate), controller.UpdateModelGatewayChannelScoreBoosts)
			modelGatewayRoute.GET("/proxies", controller.ListModelGatewayProxies)
			modelGatewayRoute.POST("/proxies", middleware.RequireAdminPermission(middleware.AdminPermissionChannelProxyUpdate), controller.CreateModelGatewayProxy)
			modelGatewayRoute.PUT("/proxies/:proxy_id", middleware.RequireAdminPermission(middleware.AdminPermissionChannelProxyUpdate), controller.UpdateModelGatewayProxy)
			modelGatewayRoute.POST("/proxies/:proxy_id/detect", middleware.RequireAdminPermission(middleware.AdminPermissionChannelProxyUpdate), controller.DetectModelGatewayProxyGeo)
			modelGatewayRoute.GET("/observability/summary", controller.GetModelGatewayObservabilitySummary)
			modelGatewayRoute.GET("/observability/trends/export", controller.ExportModelGatewayObservabilityTrends)
			modelGatewayRoute.GET("/observability/runtime", controller.GetModelGatewayRuntimeStatus)
			modelGatewayRoute.POST("/observability/runtime/clear_circuit", middleware.RequireAdminPermission(middleware.AdminPermissionChannelHealthExecute), controller.ClearModelGatewayRuntimeCircuit)
			modelGatewayRoute.POST("/observability/runtime/clear_client_empty_output_avoidance", middleware.RequireAdminPermission(middleware.AdminPermissionChannelHealthExecute), controller.ClearModelGatewayClientEmptyOutputAvoidance)
			modelGatewayRoute.GET("/observability/health-check/queue", controller.GetModelGatewayHealthCheckQueue)
			modelGatewayRoute.POST("/observability/health-check/probe", middleware.RequireAdminPermission(middleware.AdminPermissionChannelHealthExecute), controller.RunModelGatewayHealthCheckProbe)
			modelGatewayRoute.GET("/observability/score-history", controller.GetModelGatewayScoreHistory)
			modelGatewayRoute.GET("/observability/score-events", controller.GetModelGatewayScoreEvents)
			modelGatewayRoute.GET("/score-events", controller.GetModelGatewayScoreEvents)
			modelGatewayRoute.GET("/observability/sticky", controller.GetModelGatewayStickyStore)
			modelGatewayRoute.DELETE("/observability/sticky", middleware.RequireAdminPermission(middleware.AdminPermissionModelRoutePolicyDanger), controller.ClearModelGatewayStickyStore)
			modelGatewayRoute.DELETE("/observability/sticky/:key_id", middleware.RequireAdminPermission(middleware.AdminPermissionModelRoutePolicyDanger), controller.ClearModelGatewayStickyStoreEntry)
			modelGatewayRoute.GET("/profit_monitor/config", controller.GetModelGatewayProfitMonitorConfig)
			modelGatewayRoute.PATCH("/profit_monitor/config", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.UpdateModelGatewayProfitMonitorConfig)
			modelGatewayRoute.GET("/profit_monitor/summary", controller.GetModelGatewayProfitMonitorSummary)
			modelGatewayRoute.GET("/profit_monitor/traffic", controller.GetModelGatewayProfitMonitorTraffic)
			modelGatewayRoute.GET("/profit_monitor/recommendations", controller.ListModelGatewayProfitMonitorRecommendations)
			modelGatewayRoute.POST("/profit_monitor/recommendations", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.CreateModelGatewayProfitMonitorRecommendation)
			modelGatewayRoute.PATCH("/profit_monitor/recommendations/:id/decision", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.UpdateModelGatewayProfitMonitorRecommendationDecision)
			modelGatewayRoute.GET("/profit_monitor/canary_tasks", controller.ListModelGatewayProfitMonitorCanaryTasks)
			modelGatewayRoute.POST("/profit_monitor/canary_tasks", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.CreateModelGatewayProfitMonitorCanaryTask)
			modelGatewayRoute.PATCH("/profit_monitor/canary_tasks/:id", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.UpdateModelGatewayProfitMonitorCanaryTask)
			modelGatewayRoute.GET("/profit_monitor/resources", controller.ListModelGatewayProfitMonitorResources)
			modelGatewayRoute.POST("/profit_monitor/resources", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.CreateModelGatewayProfitMonitorResource)
			modelGatewayRoute.PATCH("/profit_monitor/resources/:id", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.UpdateModelGatewayProfitMonitorResource)
			modelGatewayRoute.DELETE("/profit_monitor/resources/:id", middleware.RequireAdminPermission(middleware.AdminPermissionCommercialProfitUpdate), controller.DeleteModelGatewayProfitMonitorResource)
			modelGatewayRoute.GET("/replay/export", controller.ExportModelGatewayReplay)
			modelGatewayRoute.GET("/replay/export/batch", controller.ExportModelGatewayReplayBatch)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", middleware.RequireAdminPermission(middleware.AdminPermissionModelGatewayUpdate), controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", middleware.RequireAdminPermission(middleware.AdminPermissionModelGatewayUpdate), controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", middleware.RequireAdminPermission(middleware.AdminPermissionModelGatewayUpdate), controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("/", middleware.AdminAuth(), controller.GetAllTask)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.CreateVendorMeta)
			vendorRoute.PUT("/", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.SyncUpstreamModels)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.CreateModelMeta)
			modelsRoute.PUT("/", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", middleware.RequireAdminPermission(middleware.AdminPermissionModelModelUpdate), controller.DeleteModelMeta)
		}

		// Deployments (model deployment management)
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)
			deploymentsRoute.POST("/settings/test-connection", middleware.RequireAdminPermission(middleware.AdminPermissionModelDeploymentUpdate), controller.TestIoNetConnection)
			deploymentsRoute.GET("/", controller.GetAllDeployments)
			deploymentsRoute.GET("/search", controller.SearchDeployments)
			deploymentsRoute.POST("/test-connection", middleware.RequireAdminPermission(middleware.AdminPermissionModelDeploymentUpdate), controller.TestIoNetConnection)
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)
			deploymentsRoute.GET("/locations", controller.GetLocations)
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas)
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability)
			deploymentsRoute.POST("/", middleware.RequireAdminPermission(middleware.AdminPermissionModelDeploymentUpdate), controller.CreateDeployment)

			deploymentsRoute.GET("/:id", controller.GetDeployment)
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails)
			deploymentsRoute.PUT("/:id", middleware.RequireAdminPermission(middleware.AdminPermissionModelDeploymentUpdate), controller.UpdateDeployment)
			deploymentsRoute.PUT("/:id/name", middleware.RequireAdminPermission(middleware.AdminPermissionModelDeploymentUpdate), controller.UpdateDeploymentName)
			deploymentsRoute.POST("/:id/extend", middleware.RequireAdminPermission(middleware.AdminPermissionModelDeploymentUpdate), controller.ExtendDeployment)
			deploymentsRoute.DELETE("/:id", middleware.RequireAdminPermission(middleware.AdminPermissionModelDeploymentUpdate), controller.DeleteDeployment)
		}
	}
}

func SetRealtimeRouter(router *gin.Engine) {
	realtimeRouter := router.Group("/api")
	realtimeRouter.Use(middleware.RouteTag("api"))
	realtimeRouter.Use(middleware.GlobalAPIRateLimit())
	realtimeRouter.GET("/realtime/ws", middleware.RealtimeAdminAuth(), controller.RealtimeWebSocket)
}
