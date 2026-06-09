package app

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/oauth"
	modelgatewaycost "github.com/QuantumNous/new-api/pkg/modelgateway/cost"
	modelgatewaydynamicbilling "github.com/QuantumNous/new-api/pkg/modelgateway/dynamicbilling"
	modelgatewayintegration "github.com/QuantumNous/new-api/pkg/modelgateway/integration"
	modelgatewayrealtime "github.com/QuantumNous/new-api/pkg/modelgateway/observability/realtime"
	modelgatewayprobe "github.com/QuantumNous/new-api/pkg/modelgateway/probe"
	modelgatewaytraffic "github.com/QuantumNous/new-api/pkg/modelgateway/traffic"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	_ "github.com/QuantumNous/new-api/setting/performance_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/scheduler_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	_ "net/http/pprof"
)

type ServiceRole string

const (
	RoleFull    ServiceRole = "full"
	RoleWeb     ServiceRole = "web"
	RoleGateway ServiceRole = "gateway"
)

type RunConfig struct {
	Role   ServiceRole
	Assets router.ThemeAssets
}

func Run(config RunConfig) {
	startTime := time.Now()
	role := normalizeRole(config.Role)

	cleanup, err := InitResources(role)
	if err != nil {
		common.FatalLog("failed to initialize resources: " + err.Error())
		return
	}
	defer cleanup()

	common.SysLog(fmt.Sprintf("New API %s started, service role: %s", common.Version, role))
	if common.DebugEnabled {
		common.SysLog("running in debug mode")
	}

	StartBackgroundProcesses(role)
	StartDiagnostics()

	server := NewServer(role)
	RegisterRoutes(server, role, config.Assets)

	port := os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}

	common.LogStartupSuccess(startTime, port)
	if err := server.Run(":" + port); err != nil {
		common.FatalLog("failed to start HTTP server: " + err.Error())
	}
}

func InitResources(role ServiceRole) (func(), error) {
	if err := godotenv.Load(".env"); err != nil && common.DebugEnabled {
		common.SysLog("No .env file found, using default environment variables. If needed, please create a .env file and set the relevant variables.")
	}

	applyRoleDefaults(role)
	common.InitEnv()
	logger.SetupLogger()

	ratio_setting.InitRatioSettings()
	service.InitHttpClient()
	service.InitTokenEncoders()

	if err := model.InitDB(); err != nil {
		common.FatalLog("failed to initialize database: " + err.Error())
		return nil, err
	}
	modelgatewayrealtime.RegisterDefaultTopic()
	model.CheckSetup()
	model.InitOptionMap()
	common.CleanupOldCacheFiles()
	model.GetPricing()

	if err := model.InitLogDB(); err != nil {
		return nil, err
	}
	if common.GetEnvOrDefaultBool("SKIP_CODEX_APPLICATION_ENVIRONMENT_SYNC", false) {
		common.SysLog("codex application environment sync skipped by SKIP_CODEX_APPLICATION_ENVIRONMENT_SYNC")
	} else if err := model.SyncCodexApplicationEnvironments(); err != nil {
		return nil, err
	}

	if err := common.InitRedisClient(); err != nil {
		return nil, err
	}
	modelgatewayintegration.SyncRuntimeEventSubscriberLifecycle()
	if common.GetEnvOrDefaultBool("SKIP_MODEL_GATEWAY_RUNTIME_WARMUP", false) {
		common.SysLog("model gateway runtime observability warmup skipped by SKIP_MODEL_GATEWAY_RUNTIME_WARMUP")
	} else {
		modelgatewayintegration.WarmupDefaultRuntimeObservability()
	}

	perfmetrics.Init()
	modelgatewaytraffic.Init()
	common.StartSystemMonitor()

	if err := i18n.Init(); err != nil {
		common.SysError("failed to initialize i18n: " + err.Error())
	} else {
		common.SysLog("i18n initialized with languages: " + strings.Join(i18n.SupportedLanguages(), ", "))
	}
	i18n.SetUserLangLoader(model.GetUserLanguage)

	if err := oauth.LoadCustomProviders(); err != nil {
		common.SysError("failed to load custom OAuth providers: " + err.Error())
	}

	return func() {
		modelgatewayintegration.CloseRuntimeEventSubscriber()
		if err := model.CloseDB(); err != nil {
			common.FatalLog("failed to close database: " + err.Error())
		}
	}, nil
}

func NewServer(role ServiceRole) *gin.Engine {
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	server := gin.New()
	server.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		common.SysLog(fmt.Sprintf("panic detected: %v", err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Panic detected, error: %v. Please submit a issue here: https://github.com/Calcium-Ion/new-api", err),
				"type":    "new_api_panic",
			},
		})
	}))
	server.Use(middleware.RequestId())
	server.Use(middleware.PoweredBy())
	server.Use(middleware.I18n())
	middleware.SetUpLogger(server)

	store := cookie.NewStore([]byte(common.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   2592000,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteStrictMode,
	})
	server.Use(sessions.Sessions("session", store))
	server.GET("/-/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"role":    string(role),
		})
	})
	return server
}

func RegisterRoutes(server *gin.Engine, role ServiceRole, assets router.ThemeAssets) {
	switch normalizeRole(role) {
	case RoleWeb:
		router.SetWebServiceRouter(server, ApplyAnalytics(assets))
	case RoleGateway:
		router.SetRealtimeRouter(server)
		router.SetGatewayRouter(server)
	default:
		router.SetRouter(server, ApplyAnalytics(assets))
	}
}

func StartBackgroundProcesses(role ServiceRole) {
	startSharedRequestRuntime(role)
	switch normalizeRole(role) {
	case RoleGateway:
		startGatewayBillingRuntime()
	case RoleWeb:
		startManagementRuntime()
	default:
		startManagementRuntime()
	}
	startBatchUpdater()
}

func startGatewayBillingRuntime() {
	modelgatewaycost.SyncDefaultWorkerLifecycle()
	modelgatewaydynamicbilling.LoadDefaultPersistedBaselines(model.DB)
	go func() {
		for {
			setting := scheduler_setting.GetSetting()
			intervalSeconds := setting.DynamicBillingRefreshSeconds
			if intervalSeconds <= 0 {
				intervalSeconds = scheduler_setting.DefaultSetting().DynamicBillingRefreshSeconds
			}
			time.Sleep(time.Duration(intervalSeconds) * time.Second)
			modelgatewaydynamicbilling.LoadDefaultPersistedBaselines(model.DB)
		}
	}()
}

func StartDiagnostics() {
	if os.Getenv("ENABLE_PPROF") == "true" {
		gopool.Go(func() {
			log.Println(http.ListenAndServe("0.0.0.0:8005", nil))
		})
		go common.Monitor()
		common.SysLog("pprof enabled")
	}
	if err := common.StartPyroScope(); err != nil {
		common.SysError(fmt.Sprintf("start pyroscope error : %v", err))
	}
}

func ApplyAnalytics(assets router.ThemeAssets) router.ThemeAssets {
	assets.ClassicIndexPage = injectUmamiAnalytics(assets.ClassicIndexPage)
	assets.ClassicIndexPage = injectGoogleAnalytics(assets.ClassicIndexPage)
	return assets
}

func startSharedRequestRuntime(role ServiceRole) {
	if common.RedisEnabled {
		common.MemoryCacheEnabled = true
	}
	if common.MemoryCacheEnabled {
		common.SysLog("memory cache enabled")
		common.SysLog(fmt.Sprintf("sync frequency: %d seconds", common.SyncFrequency))
		initChannelCacheWithRetry()
		go model.SyncChannelCache(common.SyncFrequency)
	}
	go model.SyncOptions(common.SyncFrequency)
	if role != RoleGateway {
		go model.UpdateQuotaData()
	}
}

func startManagementRuntime() {
	modelgatewayprobe.RegisterRelayInvoker(controller.Relay)
	scheduler_setting.AddChangeHook(func(before scheduler_setting.SchedulerSetting, after scheduler_setting.SchedulerSetting) {
		if probeSchedulerSettingsEqual(before, after) {
			return
		}
		modelgatewayprobe.SyncDefaultProbeSchedulerLifecycle()
	})

	if os.Getenv("CHANNEL_UPDATE_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_UPDATE_FREQUENCY"))
		if err != nil {
			common.FatalLog("failed to parse CHANNEL_UPDATE_FREQUENCY: " + err.Error())
		}
		go controller.AutomaticallyUpdateChannels(frequency)
	}

	go controller.AutomaticallyTestChannels()
	modelgatewayprobe.SyncDefaultProbeSchedulerLifecycle()
	modelgatewaycost.SyncDefaultWorkerLifecycle()
	modelgatewaydynamicbilling.SyncDefaultRefresherLifecycle()
	service.StartCodexCredentialAutoRefreshTask()
	service.StartSubscriptionQuotaResetTask()
	service.StartModelExecutionRecordRetentionTask()
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor {
		a := relay.GetTaskAdaptor(platform)
		if a == nil {
			return nil
		}
		return a
	}
	controller.StartChannelUpstreamModelUpdateTask()
	controller.StartChannelBalanceMonitorTask()

	if common.IsMasterNode && constant.UpdateTask {
		gopool.Go(func() {
			controller.UpdateMidjourneyTaskBulk()
		})
		gopool.Go(func() {
			controller.UpdateTaskBulk()
		})
	}
}

func startBatchUpdater() {
	if os.Getenv("BATCH_UPDATE_ENABLED") == "true" {
		common.BatchUpdateEnabled = true
		common.SysLog("batch update enabled with interval " + strconv.Itoa(common.BatchUpdateInterval) + "s")
		model.InitBatchUpdater()
	}
}

func initChannelCacheWithRetry() {
	defer func() {
		if r := recover(); r != nil {
			common.SysLog(fmt.Sprintf("InitChannelCache panic: %v, retrying once", r))
			_, _, fixErr := model.FixAbility()
			if fixErr != nil {
				common.FatalLog(fmt.Sprintf("InitChannelCache failed: %s", fixErr.Error()))
			}
		}
	}()
	model.InitChannelCache()
}

func probeSchedulerSettingsEqual(before scheduler_setting.SchedulerSetting, after scheduler_setting.SchedulerSetting) bool {
	return before.ProbeEnabled == after.ProbeEnabled &&
		before.ProbeIntervalSeconds == after.ProbeIntervalSeconds &&
		before.ProbeWorkerCount == after.ProbeWorkerCount &&
		before.ProbeTimeoutSeconds == after.ProbeTimeoutSeconds &&
		before.ProbeMaxPerTick == after.ProbeMaxPerTick &&
		before.ProbeMinChannelIntervalSeconds == after.ProbeMinChannelIntervalSeconds &&
		before.ProbeLowScoreThreshold == after.ProbeLowScoreThreshold &&
		before.ProbeMissingSampleThreshold == after.ProbeMissingSampleThreshold &&
		before.ProbeLongNoSuccessSeconds == after.ProbeLongNoSuccessSeconds &&
		before.ProbeRecoverySuccessesRequired == after.ProbeRecoverySuccessesRequired &&
		before.ProbeFailureAvoidancePriorityEnabled == after.ProbeFailureAvoidancePriorityEnabled &&
		strings.Join(before.ProbeRecoverableScoreItems, ",") == strings.Join(after.ProbeRecoverableScoreItems, ",") &&
		before.ProbeSkipRecentRealRequestEnabled == after.ProbeSkipRecentRealRequestEnabled &&
		before.ProbeRecentRealRequestWindowSeconds == after.ProbeRecentRealRequestWindowSeconds &&
		before.ProbeGoodBaselineEnabled == after.ProbeGoodBaselineEnabled &&
		before.ProbeGoodBaselineMinSamples == after.ProbeGoodBaselineMinSamples &&
		before.ProbeGoodBaselineWindowSeconds == after.ProbeGoodBaselineWindowSeconds &&
		before.ProbePromptLibraryEnabled == after.ProbePromptLibraryEnabled &&
		strings.Join(before.ProbePromptCategories, ",") == strings.Join(after.ProbePromptCategories, ",")
}

func injectUmamiAnalytics(indexPage []byte) []byte {
	analyticsInjectBuilder := &strings.Builder{}
	if os.Getenv("UMAMI_WEBSITE_ID") != "" {
		umamiSiteID := os.Getenv("UMAMI_WEBSITE_ID")
		umamiScriptURL := os.Getenv("UMAMI_SCRIPT_URL")
		if umamiScriptURL == "" {
			umamiScriptURL = "https://analytics.umami.is/script.js"
		}
		analyticsInjectBuilder.WriteString("<script defer src=\"")
		analyticsInjectBuilder.WriteString(umamiScriptURL)
		analyticsInjectBuilder.WriteString("\" data-website-id=\"")
		analyticsInjectBuilder.WriteString(umamiSiteID)
		analyticsInjectBuilder.WriteString("\"></script>")
	}
	analyticsInjectBuilder.WriteString("<!--Umami QuantumNous-->\n")
	return bytes.ReplaceAll(indexPage, []byte("<!--umami-->\n"), []byte(analyticsInjectBuilder.String()))
}

func injectGoogleAnalytics(indexPage []byte) []byte {
	analyticsInjectBuilder := &strings.Builder{}
	if os.Getenv("GOOGLE_ANALYTICS_ID") != "" {
		gaID := os.Getenv("GOOGLE_ANALYTICS_ID")
		analyticsInjectBuilder.WriteString("<script async src=\"https://www.googletagmanager.com/gtag/js?id=")
		analyticsInjectBuilder.WriteString(gaID)
		analyticsInjectBuilder.WriteString("\"></script>")
		analyticsInjectBuilder.WriteString("<script>")
		analyticsInjectBuilder.WriteString("window.dataLayer = window.dataLayer || [];")
		analyticsInjectBuilder.WriteString("function gtag(){dataLayer.push(arguments);}")
		analyticsInjectBuilder.WriteString("gtag('js', new Date());")
		analyticsInjectBuilder.WriteString("gtag('config', '")
		analyticsInjectBuilder.WriteString(gaID)
		analyticsInjectBuilder.WriteString("');")
		analyticsInjectBuilder.WriteString("</script>")
	}
	analyticsInjectBuilder.WriteString("<!--Google Analytics QuantumNous-->\n")
	return bytes.ReplaceAll(indexPage, []byte("<!--Google Analytics-->\n"), []byte(analyticsInjectBuilder.String()))
}

func applyRoleDefaults(role ServiceRole) {
	if normalizeRole(role) != RoleGateway {
		return
	}
	setDefaultEnv("SKIP_DB_AUTO_MIGRATE", "true")
	setDefaultEnv("SKIP_CODEX_APPLICATION_ENVIRONMENT_SYNC", "true")
	setDefaultEnv("BATCH_UPDATE_ENABLED", "false")
	setDefaultEnv("NODE_TYPE", "slave")
}

func setDefaultEnv(key string, value string) {
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, value)
	}
}

func normalizeRole(role ServiceRole) ServiceRole {
	switch role {
	case RoleWeb, RoleGateway, RoleFull:
		return role
	default:
		return RoleFull
	}
}
