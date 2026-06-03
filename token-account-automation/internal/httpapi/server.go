package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/token-account-automation/internal/config"
	"github.com/QuantumNous/new-api/token-account-automation/internal/gateway"
	"github.com/QuantumNous/new-api/token-account-automation/internal/jsonx"
	"github.com/QuantumNous/new-api/token-account-automation/internal/model"
	"github.com/QuantumNous/new-api/token-account-automation/internal/queue"
	"github.com/QuantumNous/new-api/token-account-automation/internal/secret"
	"gorm.io/gorm"
)

type Server struct {
	cfg     config.Config
	queue   *queue.Service
	secrets *secret.Service
	gateway *gateway.Client
	mux     *http.ServeMux
}

type createJobRequest struct {
	ParentJobID    string         `json:"parent_job_id,omitempty"`
	TaskType       string         `json:"task_type"`
	ExecutorType   string         `json:"executor_type"`
	Priority       int            `json:"priority,omitempty"`
	RunAfter       int64          `json:"run_after,omitempty"`
	MaxAttempts    int            `json:"max_attempts,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	TargetRef      string         `json:"target_ref,omitempty"`
	Input          map[string]any `json:"input,omitempty"`
}

type claimRequest struct {
	ExecutorType string `json:"executor_type"`
	WorkerID     string `json:"worker_id"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
	Limit        int    `json:"limit,omitempty"`
}

type heartbeatRequest struct {
	WorkerID     string `json:"worker_id"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
}

type stageRequest struct {
	WorkerID string         `json:"worker_id"`
	Stage    string         `json:"stage"`
	Message  string         `json:"message,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type succeedRequest struct {
	WorkerID string         `json:"worker_id"`
	Result   map[string]any `json:"result,omitempty"`
}

type credentialSucceedRequest struct {
	WorkerID  string         `json:"worker_id"`
	Value     any            `json:"value"`
	ExpiresAt int64          `json:"expires_at,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type failRequest struct {
	WorkerID          string `json:"worker_id"`
	ErrorCode         string `json:"error_code,omitempty"`
	Error             string `json:"error,omitempty"`
	RetryAfterSeconds int64  `json:"retry_after_seconds,omitempty"`
}

type waitingHumanRequest struct {
	WorkerID string         `json:"worker_id"`
	Reason   string         `json:"reason,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
}

type retryRequest struct {
	RunAfter int64 `json:"run_after,omitempty"`
}

type resumeRequest struct {
	RunAfter int64  `json:"run_after,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type cancelRequest struct {
	Reason string `json:"reason,omitempty"`
}

type desktopRetryRequest struct {
	RunAfter         int64  `json:"run_after,omitempty"`
	PreferredProxyID string `json:"preferred_proxy_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ClearSession     bool   `json:"clear_session,omitempty"`
}

type desktopArchiveAccountRequest struct {
	Reason string `json:"reason,omitempty"`
	Note   string `json:"note,omitempty"`
}

type desktopAccountActionRequest struct {
	RunAfter         int64  `json:"run_after,omitempty"`
	PreferredProxyID string `json:"preferred_proxy_id,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ClearSession     bool   `json:"clear_session,omitempty"`
}

type desktopActionTemplate struct {
	Key               string `json:"key"`
	Title             string `json:"title"`
	Category          string `json:"category"`
	Description       string `json:"description"`
	OperationType     string `json:"operation_type"`
	TaskType          string `json:"task_type,omitempty"`
	ExecutorType      string `json:"executor_type,omitempty"`
	EntryPoint        string `json:"entry_point"`
	Status            string `json:"status"`
	Implemented       bool   `json:"implemented"`
	Enabled           bool   `json:"enabled"`
	RequiresTargetRef bool   `json:"requires_target_ref,omitempty"`
	RequiresLocator   bool   `json:"requires_locator,omitempty"`
	RequiresProxy     bool   `json:"requires_proxy,omitempty"`
	Danger            bool   `json:"danger,omitempty"`
	NextStatus        string `json:"next_status,omitempty"`
	ProductValue      string `json:"product_value,omitempty"`
	TechStatus        string `json:"tech_status,omitempty"`
	OpsNote           string `json:"ops_note,omitempty"`
}

type createSecretRequest struct {
	SecretType string `json:"secret_type"`
	ScopeRef   string `json:"scope_ref,omitempty"`
	Value      any    `json:"value"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
}

func New(cfg config.Config, queueService *queue.Service, secretService *secret.Service) *Server {
	s := &Server{cfg: cfg, queue: queueService, secrets: secretService, gateway: gateway.New(cfg.GatewayCallbackURL, cfg.GatewayCallbackToken, cfg.GatewayTimeoutSecs), mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("/health", s.health)
	s.mux.HandleFunc("/operator", s.operatorPage)
	s.mux.HandleFunc("/operator/", s.operatorPage)
	s.mux.HandleFunc("/api/jobs", s.apiJobs)
	s.mux.HandleFunc("/api/jobs/", s.apiJobByID)
	s.mux.HandleFunc("/api/operator/waiting-human", s.apiOperatorWaitingHuman)
	s.mux.HandleFunc("/api/stats/jobs", s.apiJobStats)
	s.mux.HandleFunc("/api/secrets", s.apiSecrets)
	s.mux.HandleFunc("/api/desktop/overview", s.apiDesktopOverview)
	s.mux.HandleFunc("/api/desktop/accounts", s.apiDesktopAccounts)
	s.mux.HandleFunc("/api/desktop/action-templates", s.apiDesktopActionTemplates)
	s.mux.HandleFunc("/api/desktop/account-pools/invalid", s.apiDesktopInvalidAccountPool)
	s.mux.HandleFunc("/api/desktop/account-pools/invalid/", s.apiDesktopInvalidAccountPool)
	s.mux.HandleFunc("/api/desktop/jobs/", s.apiDesktopJobByID)
	s.mux.HandleFunc("/api/desktop/proxies/sync", s.apiDesktopProxySync)
	s.mux.HandleFunc("/api/events/account-auth-invalid", s.apiAccountAuthInvalid)
	s.mux.HandleFunc("/internal/jobs/claim", s.internalClaim)
	s.mux.HandleFunc("/internal/jobs/", s.internalJobByID)
}

func (s *Server) apiSecrets(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req createSecretRequest
	if err := jsonx.Decode(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	value, err := secretValueToString(req.Value)
	if err != nil {
		writeError(w, err)
		return
	}
	record, err := s.secrets.Create(r.Context(), secret.CreateRequest{
		SecretType: req.SecretType,
		ScopeRef:   req.ScopeRef,
		Value:      value,
		ExpiresAt:  req.ExpiresAt,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"secret": record}})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "service": "token-account-automation"})
}

func (s *Server) apiJobs(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		jobs, total, err := s.queue.ListJobs(r.Context(), queue.JobFilter{
			TaskType:     r.URL.Query().Get("task_type"),
			ExecutorType: r.URL.Query().Get("executor_type"),
			Status:       r.URL.Query().Get("status"),
			TargetRef:    r.URL.Query().Get("target_ref"),
			Keyword:      r.URL.Query().Get("keyword"),
			Page:         page,
			PageSize:     pageSize,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"items": jobs, "total": total}})
	case http.MethodPost:
		var req createJobRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		job, created, err := s.queue.CreateJob(r.Context(), queue.CreateJobRequest{
			ParentJobID:    req.ParentJobID,
			TaskType:       req.TaskType,
			ExecutorType:   req.ExecutorType,
			Priority:       req.Priority,
			RunAfter:       req.RunAfter,
			MaxAttempts:    req.MaxAttempts,
			IdempotencyKey: req.IdempotencyKey,
			TargetRef:      req.TargetRef,
			Input:          req.Input,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job": job, "created": created}})
	default:
		writeMethodNotAllowed(w)
	}
}

func (s *Server) apiJobByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	jobID, action, ok := splitIDAction(strings.TrimPrefix(r.URL.Path, "/api/jobs/"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if action == "" && r.Method == http.MethodGet {
		detail, err := s.queue.GetDetail(r.Context(), jobID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": detail})
		return
	}
	if action == "retry" && r.Method == http.MethodPost {
		var req retryRequest
		_ = jsonx.Decode(r.Body, &req)
		job, err := s.queue.Retry(r.Context(), jobID, req.RunAfter)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": job})
		return
	}
	if action == "resume" && r.Method == http.MethodPost {
		var req resumeRequest
		_ = jsonx.Decode(r.Body, &req)
		job, err := s.queue.ResumeWaitingHuman(r.Context(), jobID, req.RunAfter, req.Reason)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": job})
		return
	}
	if action == "cancel" && r.Method == http.MethodPost {
		var req cancelRequest
		_ = jsonx.Decode(r.Body, &req)
		if err := s.queue.Cancel(r.Context(), jobID, req.Reason); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job_id": jobID, "status": "CANCELED"}})
		return
	}
	http.NotFound(w, r)
}

func (s *Server) apiOperatorWaitingHuman(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	result, err := s.queue.ListWaitingHuman(r.Context(), queue.JobFilter{
		TaskType:     r.URL.Query().Get("task_type"),
		ExecutorType: r.URL.Query().Get("executor_type"),
		TargetRef:    r.URL.Query().Get("target_ref"),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func (s *Server) apiJobStats(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	stats, err := s.queue.JobStats(r.Context(), queue.JobFilter{
		TaskType:     r.URL.Query().Get("task_type"),
		ExecutorType: r.URL.Query().Get("executor_type"),
		Status:       r.URL.Query().Get("status"),
		TargetRef:    r.URL.Query().Get("target_ref"),
		Keyword:      r.URL.Query().Get("keyword"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": stats})
}

func (s *Server) apiDesktopOverview(w http.ResponseWriter, r *http.Request) {
	if !s.requireDesktopReadToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	stats, err := s.queue.JobStats(r.Context(), queue.JobFilter{ExecutorType: model.ExecutorDesktopSession})
	if err != nil {
		writeError(w, err)
		return
	}
	waiting, err := s.queue.ListWaitingHuman(r.Context(), queue.JobFilter{
		ExecutorType: model.ExecutorDesktopSession,
		Page:         1,
		PageSize:     5,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{
		"executor_type": model.ExecutorDesktopSession,
		"stats":         stats,
		"waiting_human": waiting,
	}})
}

func (s *Server) apiDesktopActionTemplates(w http.ResponseWriter, r *http.Request) {
	if !s.requireDesktopReadToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	items := s.desktopActionTemplates()
	categoryCounts := make(map[string]int)
	statusCounts := make(map[string]int)
	for _, item := range items {
		categoryCounts[item.Category]++
		statusCounts[item.Status]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{
		"items":           items,
		"total":           len(items),
		"category_counts": categoryCounts,
		"status_counts":   statusCounts,
	}})
}

func (s *Server) desktopActionTemplates() []desktopActionTemplate {
	gatewayEnabled := s != nil && s.gateway != nil && s.gateway.Enabled()
	return []desktopActionTemplate{
		{
			Key:               "auth_recover",
			Title:             "授权恢复编排",
			Category:          "授权",
			Description:       "接收主站授权异常事件，优先刷新 token，必要时转入桌面网页登录。",
			OperationType:     "job",
			TaskType:          model.TaskAuthRecover,
			ExecutorType:      model.ExecutorInternalAPI,
			EntryPoint:        "POST /api/events/account-auth-invalid",
			Status:            "ready",
			Implemented:       true,
			Enabled:           true,
			RequiresTargetRef: true,
			NextStatus:        model.JobStatusPending,
			ProductValue:      "把账号失效从人工排查变成标准恢复队列。",
			TechStatus:        "已接入 internal_api 编排器",
			OpsNote:           "主站检测到 token/auth 错误后自动投递。",
		},
		{
			Key:               "auth_token_refresh",
			Title:             "Refresh Token 刷新",
			Category:          "授权",
			Description:       "读取加密 refresh token，尝试换取新 credential 并写回主站渠道账号。",
			OperationType:     "job",
			TaskType:          model.TaskAuthTokenRefresh,
			ExecutorType:      model.ExecutorInternalAPI,
			EntryPoint:        "internal_api executor",
			Status:            "ready",
			Implemented:       true,
			Enabled:           true,
			RequiresTargetRef: true,
			NextStatus:        model.JobStatusSuccess,
			ProductValue:      "无感恢复可刷新账号，减少桌面登录压力。",
			TechStatus:        "已接入 OAuth refresh 与 credential callback",
			OpsNote:           "缺少 refresh_token 或刷新失败时会 fallback 到网页登录任务。",
		},
		{
			Key:               "auth_browser_login",
			Title:             "网页登录授权",
			Category:          "授权",
			Description:       "用 Electron 独立浏览器会话打开账号授权页，支持按账号隔离代理和 Cookie。",
			OperationType:     "job",
			TaskType:          model.TaskAuthBrowserLogin,
			ExecutorType:      model.ExecutorDesktopSession,
			EntryPoint:        "GET /internal/jobs/claim",
			Status:            "ready",
			Implemented:       true,
			Enabled:           true,
			RequiresTargetRef: true,
			RequiresProxy:     true,
			NextStatus:        model.JobStatusWaitingHuman,
			ProductValue:      "覆盖需要人工登录、验证码、SSO 或风控确认的账号。",
			TechStatus:        "已接入桌面执行器、回调端口和独立 session partition",
			OpsNote:           "遇到平台保护时停在待人工，运营完成后恢复任务。",
		},
		{
			Key:               "retry_with_proxy",
			Title:             "指定代理重跑",
			Category:          "运维",
			Description:       "对失败或终态任务指定代理、可选清理本地会话后重新排队。",
			OperationType:     "desktop_job_action",
			EntryPoint:        "POST /api/desktop/jobs/{job_id}/retry",
			Status:            "ready",
			Implemented:       true,
			Enabled:           true,
			RequiresTargetRef: true,
			RequiresProxy:     true,
			NextStatus:        model.JobStatusPending,
			ProductValue:      "快速验证是否代理、会话污染或临时风控导致失败。",
			TechStatus:        "已接入单任务和批量任务操作",
			OpsNote:           "代理下拉会按本地健康分排序。",
		},
		{
			Key:               "clear_local_session",
			Title:             "清理本地会话",
			Category:          "运维",
			Description:       "清除指定账号的本地浏览器 partition，避免 Cookie 或 storage 污染重试。",
			OperationType:     "local_ipc",
			EntryPoint:        "browser:clear-account-session",
			Status:            "ready",
			Implemented:       true,
			Enabled:           true,
			RequiresTargetRef: true,
			ProductValue:      "降低账号间串状态、旧登录态和授权页缓存带来的误判。",
			TechStatus:        "已接入 Electron 主进程 BrowserSessionManager",
			OpsNote:           "建议在同账号多次登录失败后先清理再重跑。",
		},
		{
			Key:           "sync_gateway_proxies",
			Title:         "同步主站代理",
			Category:      "运维",
			Description:   "从主站内部接口拉取代理资源，保存到本地加密配置并参与自动选择。",
			OperationType: "gateway_callback",
			EntryPoint:    "GET /api/desktop/proxies/sync",
			Status:        "ready",
			Implemented:   true,
			Enabled:       gatewayEnabled,
			RequiresProxy: false,
			ProductValue:  "让桌面登录和主站代理池保持同一套运营资源。",
			TechStatus:    "已接入 callback token 转发和本地健康评分",
			OpsNote:       "若显示 404，通常是主站未发布最新内部代理接口。",
		},
		{
			Key:               "archive_invalid_pool",
			Title:             "移入失效池",
			Category:          "账号池",
			Description:       "把任务绑定的渠道账号归档到主站失效池，保留后续修复和重新授权入口。",
			OperationType:     "gateway_callback",
			EntryPoint:        "POST /api/desktop/jobs/{job_id}/archive-invalid",
			Status:            "ready",
			Implemented:       true,
			Enabled:           gatewayEnabled,
			RequiresTargetRef: true,
			RequiresLocator:   true,
			Danger:            true,
			ProductValue:      "把可恢复账号从运行池剥离，避免继续污染调度。",
			TechStatus:        "已接入主站失效池 callback 和任务事件审计",
			OpsNote:           "归档会移除原渠道账号，需确认任务 locator 正确。",
		},
		{
			Key:               "archive_discarded_pool",
			Title:             "移入废弃池",
			Category:          "账号池",
			Description:       "把不可恢复账号归档到废弃池，后续不再参与调度。",
			OperationType:     "gateway_callback",
			EntryPoint:        "POST /api/desktop/jobs/{job_id}/archive-discarded",
			Status:            "ready",
			Implemented:       true,
			Enabled:           gatewayEnabled,
			RequiresTargetRef: true,
			RequiresLocator:   true,
			Danger:            true,
			ProductValue:      "把明确不可用账号沉淀为审计资产，不再反复重试。",
			TechStatus:        "已接入主站废弃池 callback 和任务事件审计",
			OpsNote:           "废弃池操作更强，建议只对确认不可恢复账号使用。",
		},
		{
			Key:           "reauthorize_invalid_pool",
			Title:         "失效池重新授权",
			Category:      "账号池",
			Description:   "从失效账号池恢复账号并投递新的桌面授权任务。",
			OperationType: "gateway_callback",
			EntryPoint:    "POST /api/desktop/account-pools/invalid/{id}/reauthorize",
			Status:        "ready",
			Implemented:   true,
			Enabled:       gatewayEnabled,
			ProductValue:  "让失效账号形成恢复闭环，而不是只做静态归档。",
			TechStatus:    "已接入主站 restore + enqueue auth recovery",
			OpsNote:       "主站接口未更新时客户端会显示 404 发布提示。",
		},
		{
			Key:               "account_probe",
			Title:             "账号可用性探活",
			Category:          "诊断",
			Description:       "投递桌面诊断任务，校验目标定位、本地会话、代理选择和后续上游探活前置条件。",
			OperationType:     "desktop_job_action",
			TaskType:          model.TaskAccountProbe,
			ExecutorType:      model.ExecutorDesktopSession,
			EntryPoint:        "POST /api/desktop/jobs/{job_id}/probe",
			Status:            "partial",
			Implemented:       true,
			Enabled:           true,
			RequiresTargetRef: true,
			RequiresProxy:     true,
			ProductValue:      "在调度前发现账号不可用，减少线上请求失败。",
			TechStatus:        "已接入桌面本地诊断任务，上游账号探活 callback 待接",
			OpsNote:           "当前先验证账号定位、浏览器分区和代理绑定，后续扩展到真实模型探活。",
		},
		{
			Key:               "profile_verify",
			Title:             "账号资料校验",
			Category:          "诊断",
			Description:       "投递桌面资料校验任务，沉淀账号定位、客户端环境和本地授权资料快照。",
			OperationType:     "desktop_job_action",
			TaskType:          model.TaskAccountProfileVerify,
			ExecutorType:      model.ExecutorDesktopSession,
			EntryPoint:        "POST /api/desktop/jobs/{job_id}/profile-verify",
			Status:            "partial",
			Implemented:       true,
			Enabled:           true,
			RequiresTargetRef: true,
			RequiresProxy:     true,
			ProductValue:      "补齐账号运营基础数据，支撑授权、过期和成本判断。",
			TechStatus:        "已接入桌面本地资料快照任务，provider profile schema 待接",
			OpsNote:           "可与重新授权成功后的回调联动执行，后续写回主站账号画像。",
		},
		{
			Key:               "security_handoff",
			Title:             "二次验证人工接管",
			Category:          "安全",
			Description:       "把密码、二次验证、验证码、SSO 和风控确认沉淀为标准待人工事件。",
			OperationType:     "human_handoff",
			EntryPoint:        "POST /internal/jobs/{job_id}/waiting-human",
			Status:            "partial",
			Implemented:       true,
			Enabled:           true,
			RequiresTargetRef: true,
			ProductValue:      "让运营知道任务卡在哪里，以及接管后如何恢复。",
			TechStatus:        "等待人工状态已接入，细分事件码待扩展",
			OpsNote:           "不绕过验证码或平台保护，只辅助合法人工授权。",
		},
	}
}

func (s *Server) apiDesktopAccounts(w http.ResponseWriter, r *http.Request) {
	if !s.requireDesktopReadToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	result, err := s.queue.ListJobItems(r.Context(), queue.JobFilter{
		TaskType:     r.URL.Query().Get("task_type"),
		ExecutorType: model.ExecutorDesktopSession,
		Status:       r.URL.Query().Get("status"),
		TargetRef:    r.URL.Query().Get("target_ref"),
		Keyword:      r.URL.Query().Get("keyword"),
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func (s *Server) apiDesktopJobByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireDesktopReadToken(w, r) {
		return
	}
	jobID, action, ok := splitIDAction(strings.TrimPrefix(r.URL.Path, "/api/desktop/jobs/"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	detail, err := s.queue.GetDetail(r.Context(), jobID)
	if err != nil {
		writeError(w, err)
		return
	}
	if detail.Job.ExecutorType != model.ExecutorDesktopSession {
		writeForbidden(w, "desktop token can only operate desktop_session jobs")
		return
	}
	if action == "" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": detail})
		return
	}
	if action == "retry" && r.Method == http.MethodPost {
		var req desktopRetryRequest
		_ = jsonx.Decode(r.Body, &req)
		patch := map[string]any{}
		if strings.TrimSpace(req.PreferredProxyID) != "" {
			patch["preferred_proxy_id"] = strings.TrimSpace(req.PreferredProxyID)
		}
		if req.ClearSession {
			patch["clear_session"] = true
		}
		if strings.TrimSpace(req.Reason) != "" {
			patch["retry_reason"] = strings.TrimSpace(req.Reason)
		}
		if len(patch) > 0 {
			if _, err := s.queue.UpdateJobInput(r.Context(), jobID, patch); err != nil {
				writeError(w, err)
				return
			}
		}
		job, err := s.queue.Retry(r.Context(), jobID, req.RunAfter)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": job})
		return
	}
	if action == "resume" && r.Method == http.MethodPost {
		var req resumeRequest
		_ = jsonx.Decode(r.Body, &req)
		job, err := s.queue.ResumeWaitingHuman(r.Context(), jobID, req.RunAfter, req.Reason)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": job})
		return
	}
	if action == "cancel" && r.Method == http.MethodPost {
		var req cancelRequest
		_ = jsonx.Decode(r.Body, &req)
		if err := s.queue.Cancel(r.Context(), jobID, req.Reason); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job_id": jobID, "status": "CANCELED"}})
		return
	}
	if (action == "probe" || action == "profile-verify") && r.Method == http.MethodPost {
		var req desktopAccountActionRequest
		_ = jsonx.Decode(r.Body, &req)
		result, err := s.enqueueDesktopAccountAction(r, detail, action, req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
		return
	}
	if (action == "archive-invalid" || action == "archive-discarded") && r.Method == http.MethodPost {
		var req desktopArchiveAccountRequest
		_ = jsonx.Decode(r.Body, &req)
		result, err := s.archiveDesktopJobAccount(r, detail, action, req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
		return
	}
	http.NotFound(w, r)
}

func (s *Server) enqueueDesktopAccountAction(r *http.Request, detail *queue.JobDetail, action string, req desktopAccountActionRequest) (map[string]any, error) {
	if detail == nil {
		return nil, errors.New("job detail is required")
	}
	targetRef := strings.TrimSpace(detail.Job.TargetRef)
	if targetRef == "" {
		return nil, errors.New("target_ref is required")
	}
	taskType := model.TaskAccountProbe
	defaultReason := "desktop_operator_probe"
	actionName := "probe"
	if action == "profile-verify" {
		taskType = model.TaskAccountProfileVerify
		defaultReason = "desktop_operator_profile_verify"
		actionName = "profile_verify"
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = defaultReason
	}
	input := map[string]any{
		"action":        actionName,
		"source_job_id": detail.Job.JobID,
		"target_ref":    targetRef,
		"reason":        reason,
	}
	if strings.TrimSpace(req.PreferredProxyID) != "" {
		input["preferred_proxy_id"] = strings.TrimSpace(req.PreferredProxyID)
	}
	if req.ClearSession {
		input["clear_session"] = true
	}
	channelID := 0
	credentialIndex := -1
	if detail.Locator != nil {
		channelID = detail.Locator.ChannelID
		credentialIndex = detail.Locator.CredentialIndex
		input["channel_id"] = detail.Locator.ChannelID
		input["credential_index"] = detail.Locator.CredentialIndex
		input["external_ref"] = detail.Locator.ExternalRef
	} else if resolvedChannelID, resolvedCredentialIndex, locatorErr := s.channelAccountLocator(r, targetRef); locatorErr == nil {
		channelID = resolvedChannelID
		credentialIndex = resolvedCredentialIndex
		input["channel_id"] = resolvedChannelID
		input["credential_index"] = resolvedCredentialIndex
	}
	if detail.Target != nil {
		input["target_provider"] = detail.Target.Provider
		input["target_status"] = detail.Target.Status
		input["target_display_name"] = detail.Target.DisplayName
		input["target_subject_key"] = detail.Target.SubjectKey
	}
	if s.gateway != nil && s.gateway.Enabled() && channelID > 0 && credentialIndex >= 0 {
		if profile, profileErr := s.gateway.GetAccountProfile(r.Context(), channelID, credentialIndex); profileErr == nil && profile != nil {
			input["gateway_account_profile"] = profile
			input["gateway_account_profile_status"] = "ok"
		} else if profileErr != nil {
			input["gateway_account_profile_status"] = "unavailable"
			input["gateway_account_profile_error"] = queue.Sanitize(profileErr.Error())
		}
	}
	job, created, err := s.queue.CreateJob(r.Context(), queue.CreateJobRequest{
		ParentJobID:  detail.Job.JobID,
		TaskType:     taskType,
		ExecutorType: model.ExecutorDesktopSession,
		Priority:     detail.Job.Priority,
		RunAfter:     req.RunAfter,
		MaxAttempts:  1,
		TargetRef:    targetRef,
		Input:        input,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"action":  actionName,
		"created": created,
		"job":     job,
	}, nil
}

func (s *Server) archiveDesktopJobAccount(r *http.Request, detail *queue.JobDetail, action string, req desktopArchiveAccountRequest) (map[string]any, error) {
	if s == nil || s.gateway == nil || !s.gateway.Enabled() {
		return nil, errors.New("gateway callback is not configured")
	}
	if detail == nil {
		return nil, errors.New("job detail is required")
	}
	channelID, credentialIndex, err := s.channelAccountLocator(r, detail.Job.TargetRef)
	if err != nil {
		return nil, err
	}
	pool := "invalid"
	reason := strings.TrimSpace(req.Reason)
	note := strings.TrimSpace(req.Note)
	if reason == "" {
		reason = "desktop_operator_invalid"
	}
	if note == "" {
		note = "desktop job " + detail.Job.JobID
	}
	archiveReq := gateway.AccountPoolArchiveRequest{
		Targets: []gateway.ChannelAccountArchiveTarget{{
			ChannelID:       channelID,
			CredentialIndex: credentialIndex,
		}},
		Reason:      reason,
		Note:        note,
		SourceJobID: detail.Job.JobID,
	}
	var archiveResult *gateway.AccountPoolArchiveResult
	if action == "archive-discarded" {
		pool = "discarded"
		if strings.TrimSpace(req.Reason) == "" {
			archiveReq.Reason = "desktop_operator_discarded"
		}
		archiveResult, err = s.gateway.ArchiveAccountToDiscardedPool(r.Context(), archiveReq)
	} else {
		archiveResult, err = s.gateway.ArchiveAccountToInvalidPool(r.Context(), archiveReq)
	}
	if err != nil {
		return nil, err
	}
	var operation *gateway.AccountPoolArchiveOperation
	if archiveResult != nil {
		operation = archiveResult.Operation
	}
	eventData := map[string]any{
		"pool":             pool,
		"channel_id":       channelID,
		"credential_index": credentialIndex,
		"reason":           archiveReq.Reason,
		"note":             archiveReq.Note,
		"operation":        operation,
	}
	if err := s.queue.RecordEvent(r.Context(), detail.Job.JobID, "account_pool_archived", pool, "channel account archived to "+pool+" pool", eventData); err != nil {
		return nil, err
	}
	return map[string]any{
		"pool":             pool,
		"channel_id":       channelID,
		"credential_index": credentialIndex,
		"operation":        operation,
	}, nil
}

func (s *Server) apiDesktopProxySync(w http.ResponseWriter, r *http.Request) {
	if !s.requireDesktopReadToken(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	enabledOnly := true
	if strings.TrimSpace(r.URL.Query().Get("enabled_only")) != "" {
		enabledOnly = parseBool(r.URL.Query().Get("enabled_only"), true)
	}
	proxies, err := s.gateway.ListDesktopProxies(r.Context(), enabledOnly)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{
		"items":        proxies,
		"total":        len(proxies),
		"enabled_only": enabledOnly,
	}})
}

func (s *Server) apiDesktopInvalidAccountPool(w http.ResponseWriter, r *http.Request) {
	if !s.requireDesktopReadToken(w, r) {
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/api/desktop/account-pools/invalid")
	if suffix == "" || suffix == "/" {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		result, err := s.gateway.ListInvalidAccountPool(r.Context(), gateway.AccountPoolListParams{
			Page:        positiveQueryInt(r, "page", 1),
			PageSize:    positiveQueryInt(r, "page_size", 20),
			Keyword:     r.URL.Query().Get("keyword"),
			ChannelID:   positiveQueryInt(r, "channel_id", 0),
			AccountType: r.URL.Query().Get("account_type"),
			Brand:       r.URL.Query().Get("brand"),
			Provider:    r.URL.Query().Get("provider"),
		})
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
		return
	}
	poolIDText, action, ok := splitIDAction(strings.TrimPrefix(suffix, "/"))
	if !ok || action != "reauthorize" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	poolID, err := strconv.Atoi(poolIDText)
	if err != nil || poolID <= 0 {
		writeError(w, errors.New("pool_id is required"))
		return
	}
	var req gateway.InvalidAccountReauthorizeRequest
	_ = jsonx.Decode(r.Body, &req)
	if strings.TrimSpace(req.Reason) == "" {
		req.Reason = "desktop_pool_reauthorize"
	}
	result, err := s.gateway.ReauthorizeInvalidAccount(r.Context(), poolID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func (s *Server) apiAccountAuthInvalid(w http.ResponseWriter, r *http.Request) {
	if !s.requireAPIToken(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var event queue.AccountAuthInvalidEvent
	if err := jsonx.Decode(r.Body, &event); err != nil {
		writeError(w, err)
		return
	}
	job, created, err := s.queue.EnqueueAuthInvalid(r.Context(), event)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job": job, "created": created}})
}

func (s *Server) internalClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req claimRequest
	if err := jsonx.Decode(r.Body, &req); err != nil {
		writeError(w, err)
		return
	}
	tokenKind := s.requestTokenKind(r)
	if tokenKind == tokenKindNone || tokenKind == tokenKindAPI {
		writeUnauthorized(w, "invalid token")
		return
	}
	if tokenKind == tokenKindDesktop && strings.ToLower(strings.TrimSpace(req.ExecutorType)) != model.ExecutorDesktopSession {
		writeForbidden(w, "desktop token can only claim desktop_session jobs")
		return
	}
	result, err := s.queue.Claim(r.Context(), queue.ClaimRequest{
		ExecutorType: req.ExecutorType,
		WorkerID:     req.WorkerID,
		LeaseSeconds: req.LeaseSeconds,
		Limit:        req.Limit,
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"job": nil}})
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func (s *Server) internalJobByID(w http.ResponseWriter, r *http.Request) {
	jobID, action, ok := splitIDAction(strings.TrimPrefix(r.URL.Path, "/internal/jobs/"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	if !s.requireInternalJobToken(w, r, jobID) {
		return
	}
	switch action {
	case "heartbeat":
		var req heartbeatRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.Heartbeat(r.Context(), jobID, req.WorkerID, req.LeaseSeconds))
	case "stage":
		var req stageRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.ReportStage(r.Context(), jobID, req.WorkerID, req.Stage, req.Message, req.Data))
	case "succeed":
		var req succeedRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.Succeed(r.Context(), jobID, req.WorkerID, req.Result))
	case "succeed-credential":
		var req credentialSucceedRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		s.succeedCredential(w, r, jobID, req)
	case "fail":
		var req failRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.Fail(r.Context(), jobID, req.WorkerID, req.ErrorCode, req.Error, req.RetryAfterSeconds))
	case "waiting-human":
		var req waitingHumanRequest
		if err := jsonx.Decode(r.Body, &req); err != nil {
			writeError(w, err)
			return
		}
		writeResult(w, s.queue.WaitingHuman(r.Context(), jobID, req.WorkerID, req.Reason, req.Data))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) succeedCredential(w http.ResponseWriter, r *http.Request, jobID string, req credentialSucceedRequest) {
	if err := s.queue.ReportStage(r.Context(), jobID, req.WorkerID, "credential_received", "credential received", nil); err != nil {
		writeError(w, err)
		return
	}
	detail, err := s.queue.GetDetail(r.Context(), jobID)
	if err != nil {
		writeError(w, err)
		return
	}
	value, err := secretValueToString(req.Value)
	if err != nil {
		writeError(w, err)
		return
	}
	record, err := s.secrets.Create(r.Context(), secret.CreateRequest{
		SecretType: secret.SecretTypeCodexOAuth,
		ScopeRef:   detail.Job.TargetRef,
		Value:      value,
		ExpiresAt:  req.ExpiresAt,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	_ = s.secrets.LinkJobSecret(r.Context(), jobID, record.SecretRef, "credential")
	if err := s.writeCredentialToGateway(r, detail.Job.TargetRef, jobID, req.Value, record.SecretRef, record.Fingerprint, req.ExpiresAt, req.Metadata); err != nil {
		writeError(w, err)
		return
	}
	result := map[string]any{
		"credential_secret_ref": record.SecretRef,
		"fingerprint":           record.Fingerprint,
		"expires_at":            record.ExpiresAt,
		"target_ref":            detail.Job.TargetRef,
	}
	if req.Metadata != nil {
		result["metadata"] = req.Metadata
	}
	if err := s.queue.Succeed(r.Context(), jobID, req.WorkerID, result); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": result})
}

func (s *Server) writeCredentialToGateway(r *http.Request, targetRef string, jobID string, value any, secretRef string, fingerprint string, expiresAt int64, metadata map[string]any) error {
	if s == nil || s.gateway == nil || !s.gateway.Enabled() {
		return nil
	}
	channelID, credentialIndex, err := s.channelAccountLocator(r, targetRef)
	if err != nil {
		return err
	}
	if metadata == nil {
		metadata = make(map[string]any)
	}
	if expiresAt > 0 {
		metadata["expires_at"] = expiresAt
	}
	_, err = s.gateway.WriteCredential(r.Context(), gateway.CredentialWritebackRequest{
		ChannelID:       channelID,
		CredentialIndex: credentialIndex,
		CredentialType:  "oauth_account",
		Credential:      value,
		SourceJobID:     jobID,
		SecretRef:       secretRef,
		Fingerprint:     fingerprint,
		Metadata:        metadata,
	})
	return err
}

func (s *Server) channelAccountLocator(r *http.Request, targetRef string) (int, int, error) {
	targetRef = strings.TrimSpace(targetRef)
	if targetRef == "" {
		return 0, -1, errors.New("target_ref is required")
	}
	if channelID, credentialIndex, ok := queue.ParseChannelAccountExternalRef(targetRef); ok {
		return channelID, credentialIndex, nil
	}
	locator, err := s.queue.ChannelAccountLocatorForTarget(r.Context(), targetRef)
	if err == nil && locator != nil {
		return locator.ChannelID, locator.CredentialIndex, nil
	}
	return 0, -1, errors.New("channel account binding not found")
}

func (s *Server) requireAPIToken(w http.ResponseWriter, r *http.Request) bool {
	return requireToken(w, r, s.cfg.APIToken, "api token is not configured")
}

func (s *Server) requireWorkerToken(w http.ResponseWriter, r *http.Request) bool {
	return requireToken(w, r, s.cfg.WorkerToken, "worker token is not configured")
}

func (s *Server) requireDesktopReadToken(w http.ResponseWriter, r *http.Request) bool {
	kind := s.requestTokenKind(r)
	if kind == tokenKindAPI || kind == tokenKindDesktop {
		return true
	}
	writeUnauthorized(w, "invalid token")
	return false
}

func (s *Server) requireInternalJobToken(w http.ResponseWriter, r *http.Request, jobID string) bool {
	kind := s.requestTokenKind(r)
	if kind == tokenKindWorker {
		return true
	}
	if kind != tokenKindDesktop {
		writeUnauthorized(w, "invalid token")
		return false
	}
	detail, err := s.queue.GetDetail(r.Context(), jobID)
	if err != nil {
		writeError(w, err)
		return false
	}
	if detail.Job.ExecutorType != model.ExecutorDesktopSession {
		writeForbidden(w, "desktop token can only operate desktop_session jobs")
		return false
	}
	return true
}

const (
	tokenKindNone    = ""
	tokenKindAPI     = "api"
	tokenKindWorker  = "worker"
	tokenKindDesktop = "desktop"
)

func (s *Server) requestTokenKind(r *http.Request) string {
	actual := bearerToken(r)
	if actual == "" {
		return tokenKindNone
	}
	if tokenMatches(actual, s.cfg.WorkerToken) {
		return tokenKindWorker
	}
	if tokenMatches(actual, s.cfg.DesktopToken) {
		return tokenKindDesktop
	}
	if tokenMatches(actual, s.cfg.APIToken) {
		return tokenKindAPI
	}
	return tokenKindNone
}

func requireToken(w http.ResponseWriter, r *http.Request, expected string, missingMessage string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": missingMessage})
		return false
	}
	if !tokenMatches(bearerToken(r), expected) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "invalid token"})
		return false
	}
	return true
}

func bearerToken(r *http.Request) string {
	actual := strings.TrimSpace(r.Header.Get("Authorization"))
	actual = strings.TrimPrefix(actual, "Bearer ")
	if actual == "" {
		actual = strings.TrimSpace(r.Header.Get("X-Automation-Token"))
	}
	return actual
}

func tokenMatches(actual string, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	return actual != "" && expected != "" && subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) == 1
}

func splitIDAction(value string) (string, string, bool) {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", true
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	return "", "", false
}

func writeResult(w http.ResponseWriter, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": map[string]any{"ok": true}})
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, gorm.ErrRecordNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]any{"success": false, "message": err.Error()})
}

func writeUnauthorized(w http.ResponseWriter, message string) {
	if strings.TrimSpace(message) == "" {
		message = "unauthorized"
	}
	writeJSON(w, http.StatusUnauthorized, map[string]any{"success": false, "message": message})
}

func writeForbidden(w http.ResponseWriter, message string) {
	if strings.TrimSpace(message) == "" {
		message = "forbidden"
	}
	writeJSON(w, http.StatusForbidden, map[string]any{"success": false, "message": message})
}

func parseBool(value string, fallback bool) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func positiveQueryInt(r *http.Request, key string, fallback int) int {
	if r == nil {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "method not allowed"})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	data, err := jsonx.Marshal(payload)
	if err != nil {
		status = http.StatusInternalServerError
		data = []byte(`{"success":false,"message":"failed to encode response"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func secretValueToString(value any) (string, error) {
	if value == nil {
		return "", errors.New("value is required")
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return "", errors.New("value is required")
		}
		return text, nil
	}
	data, err := jsonx.Marshal(value)
	if err != nil {
		return "", err
	}
	text := strings.TrimSpace(string(data))
	if text == "" || text == "null" {
		return "", errors.New("value is required")
	}
	return text, nil
}
