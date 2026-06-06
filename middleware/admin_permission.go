package middleware

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const adminAuditMaxBodySummaryBytes = 64 << 10

const (
	AdminPermissionOperationsOverviewRead = "admin:operations:overview:read"
	AdminPermissionOperationsRuntimeRead  = "admin:operations:runtime:read"
	AdminPermissionOperationsAlertsRead   = "admin:operations:alerts:read"

	AdminPermissionChannelChannelRead   = "admin:channel:channel:read"
	AdminPermissionChannelChannelDanger = "admin:channel:channel:danger"
	AdminPermissionChannelChannelUpdate = "admin:channel:channel:update"
	AdminPermissionChannelAccountRead   = "admin:channel:account:read"
	AdminPermissionChannelAccountDanger = "admin:channel:account:danger"
	AdminPermissionChannelBalanceRead   = "admin:channel:balance:read"
	AdminPermissionChannelHealthRead    = "admin:channel:health:read"
	AdminPermissionChannelHealthExecute = "admin:channel:health:execute"
	AdminPermissionChannelProxyRead     = "admin:channel:proxy:read"
	AdminPermissionChannelProxyUpdate   = "admin:channel:proxy:update"

	AdminPermissionModelGatewayRead       = "admin:model:gateway:read"
	AdminPermissionModelGatewayUpdate     = "admin:model:gateway:update"
	AdminPermissionModelModelRead         = "admin:model:model:read"
	AdminPermissionModelModelUpdate       = "admin:model:model:update"
	AdminPermissionModelDeploymentRead    = "admin:model:deployment:read"
	AdminPermissionModelDeploymentUpdate  = "admin:model:deployment:update"
	AdminPermissionModelRoutePolicyRead   = "admin:model:route_policy:read"
	AdminPermissionModelRoutePolicyDanger = "admin:model:route_policy:danger"
	AdminPermissionModelRatioRead         = "admin:model:ratio:read"
	AdminPermissionModelRatioUpdate       = "admin:model:ratio:update"

	AdminPermissionCommercialProfitRead         = "admin:commercial:profit:read"
	AdminPermissionCommercialProfitUpdate       = "admin:commercial:profit:update"
	AdminPermissionCommercialSubscriptionRead   = "admin:commercial:subscription:read"
	AdminPermissionCommercialSubscriptionUpdate = "admin:commercial:subscription:update"
	AdminPermissionCommercialRedemptionRead     = "admin:commercial:redemption:read"
	AdminPermissionCommercialRedemptionUpdate   = "admin:commercial:redemption:update"
	AdminPermissionCommercialSettlementRead     = "admin:commercial:settlement:read"
	AdminPermissionCommercialSettlementComplete = "admin:commercial:settlement:complete"
	AdminPermissionCommercialConsumptionRead    = "admin:commercial:consumption:read"

	AdminPermissionUserUserRead    = "admin:user:user:read"
	AdminPermissionUserUserDanger  = "admin:user:user:danger"
	AdminPermissionUserSegmentRead = "admin:user:segment:read"
	AdminPermissionUserRiskRead    = "admin:user:risk:read"
	AdminPermissionUserRebateRead  = "admin:user:rebate:read"

	AdminPermissionSystemSettingsRead      = "admin:system:settings:read"
	AdminPermissionSystemSettingsUpdate    = "admin:system:settings:update"
	AdminPermissionSystemRolesRead         = "admin:system:roles:read"
	AdminPermissionSystemRolesUpdate       = "admin:system:roles:update"
	AdminPermissionSystemAuditRead         = "admin:system:audit:read"
	AdminPermissionSystemTaskRead          = "admin:system:task:read"
	AdminPermissionSystemPerformanceDanger = "admin:system:performance:danger"
)

const adminPermissionModeRoleCompatibility = "role_compatibility"

type AdminPermissionRoleTemplate struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Domains     []string `json:"domains"`
}

type AdminMenuPermissionCatalogItem struct {
	Group         string `json:"group"`
	Label         string `json:"label"`
	Path          string `json:"path"`
	Permission    string `json:"permission"`
	DefaultRole   string `json:"default_role"`
	Priority      string `json:"priority"`
	LegacyMinRole int    `json:"legacy_min_role"`
}

type AdminDangerousOperationPermissionCatalogItem struct {
	Page          string `json:"page"`
	Operation     string `json:"operation"`
	Permission    string `json:"permission"`
	DefaultRole   string `json:"default_role"`
	Confirmation  string `json:"confirmation"`
	Priority      string `json:"priority"`
	LegacyMinRole int    `json:"legacy_min_role"`
}

type AdminOperationPermissionCatalogItem struct {
	Group         string `json:"group"`
	Operation     string `json:"operation"`
	Permission    string `json:"permission"`
	DefaultRole   string `json:"default_role"`
	Priority      string `json:"priority"`
	LegacyMinRole int    `json:"legacy_min_role"`
}

type AdminPermissionCatalog struct {
	Mode                          string                                         `json:"mode"`
	RoleTemplates                 []AdminPermissionRoleTemplate                  `json:"role_templates"`
	MenuPermissions               []AdminMenuPermissionCatalogItem               `json:"menu_permissions"`
	DangerousOperationPermissions []AdminDangerousOperationPermissionCatalogItem `json:"dangerous_operation_permissions"`
	OperationPermissions          []AdminOperationPermissionCatalogItem          `json:"operation_permissions"`
}

var adminPermissionRoleTemplates = []AdminPermissionRoleTemplate{
	{
		Key:         "operations_admin",
		Name:        "运营管理员",
		Code:        "Operations",
		Description: "查看运营首页、用户运营、风险记录和结算处理状态。",
		Domains:     []string{"运营首页", "用户运营"},
	},
	{
		Key:         "channel_admin",
		Name:        "渠道管理员",
		Code:        "Channel",
		Description: "维护渠道、账号池、余额监控、健康检测和代理配置。",
		Domains:     []string{"渠道运营"},
	},
	{
		Key:         "model_admin",
		Name:        "模型管理员",
		Code:        "Model",
		Description: "维护智能网关、模型、部署、路由策略和倍率配置。",
		Domains:     []string{"模型与路由"},
	},
	{
		Key:         "commercial_admin",
		Name:        "财务管理员",
		Code:        "Finance",
		Description: "查看盈利、订阅、兑换码、结算记录和消费明细。",
		Domains:     []string{"商业运营"},
	},
	{
		Key:         "root",
		Name:        "超级管理员",
		Code:        "Root",
		Description: "负责系统治理、全局配置、审计日志和高风险操作兜底。",
		Domains:     []string{"系统治理"},
	},
}

var adminMenuPermissionCatalog = []AdminMenuPermissionCatalogItem{
	{Group: "运营首页", Label: "经营总览", Path: "/admin/overview", Permission: AdminPermissionOperationsOverviewRead, DefaultRole: "运营管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "运营首页", Label: "实时监控", Path: "/admin/realtime-monitor", Permission: AdminPermissionOperationsRuntimeRead, DefaultRole: "运营管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "运营首页", Label: "渠道预警", Path: "/admin/channel-alerts", Permission: AdminPermissionOperationsAlertsRead, DefaultRole: "运营管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "渠道运营", Label: "渠道管理", Path: "/admin/channels", Permission: AdminPermissionChannelChannelRead, DefaultRole: "渠道管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "渠道运营", Label: "账号池管理", Path: "/admin/channel-accounts", Permission: AdminPermissionChannelAccountRead, DefaultRole: "渠道管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "渠道运营", Label: "渠道余额监控", Path: "/admin/channel-balance-monitor", Permission: AdminPermissionChannelBalanceRead, DefaultRole: "渠道管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "渠道运营", Label: "渠道健康检测", Path: "/admin/channel-health-check", Permission: AdminPermissionChannelHealthRead, DefaultRole: "渠道管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "渠道运营", Label: "代理管理", Path: "/admin/channel-proxies", Permission: AdminPermissionChannelProxyRead, DefaultRole: "渠道管理员", Priority: "P1", LegacyMinRole: common.RoleAdminUser},
	{Group: "模型与路由", Label: "智能模型网关", Path: "/admin/model-gateway", Permission: AdminPermissionModelGatewayRead, DefaultRole: "模型管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "模型与路由", Label: "模型管理", Path: "/admin/models", Permission: AdminPermissionModelModelRead, DefaultRole: "模型管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "模型与路由", Label: "模型部署", Path: "/admin/deployment", Permission: AdminPermissionModelDeploymentRead, DefaultRole: "模型管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "模型与路由", Label: "路由策略", Path: "/admin/route-policy", Permission: AdminPermissionModelRoutePolicyRead, DefaultRole: "模型管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "模型与路由", Label: "倍率配置", Path: "/admin/ratio-config", Permission: AdminPermissionModelRatioRead, DefaultRole: "超级管理员", Priority: "P0", LegacyMinRole: common.RoleRootUser},
	{Group: "商业运营", Label: "盈利监控台", Path: "/admin/profit-monitor", Permission: AdminPermissionCommercialProfitRead, DefaultRole: "财务管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "商业运营", Label: "订阅管理", Path: "/admin/subscription", Permission: AdminPermissionCommercialSubscriptionRead, DefaultRole: "财务管理员", Priority: "P1", LegacyMinRole: common.RoleAdminUser},
	{Group: "商业运营", Label: "兑换码管理", Path: "/admin/redemption", Permission: AdminPermissionCommercialRedemptionRead, DefaultRole: "财务管理员", Priority: "P1", LegacyMinRole: common.RoleAdminUser},
	{Group: "商业运营", Label: "结算记录", Path: "/admin/settlements", Permission: AdminPermissionCommercialSettlementRead, DefaultRole: "财务管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "商业运营", Label: "消费明细", Path: "/admin/consumption", Permission: AdminPermissionCommercialConsumptionRead, DefaultRole: "财务管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "用户运营", Label: "用户管理", Path: "/admin/users", Permission: AdminPermissionUserUserRead, DefaultRole: "运营管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "用户运营", Label: "用户分层", Path: "/admin/user-segments", Permission: AdminPermissionUserSegmentRead, DefaultRole: "运营管理员", Priority: "P1", LegacyMinRole: common.RoleAdminUser},
	{Group: "用户运营", Label: "风控记录", Path: "/admin/risk-records", Permission: AdminPermissionUserRiskRead, DefaultRole: "运营管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "用户运营", Label: "邀请返佣", Path: "/admin/invite-rebates", Permission: AdminPermissionUserRebateRead, DefaultRole: "运营管理员", Priority: "P1", LegacyMinRole: common.RoleAdminUser},
	{Group: "系统治理", Label: "系统设置", Path: "/admin/settings", Permission: AdminPermissionSystemSettingsRead, DefaultRole: "超级管理员", Priority: "P0", LegacyMinRole: common.RoleRootUser},
	{Group: "系统治理", Label: "权限角色", Path: "/admin/roles", Permission: AdminPermissionSystemRolesRead, DefaultRole: "管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "系统治理", Label: "审计日志", Path: "/admin/audit-logs", Permission: AdminPermissionSystemAuditRead, DefaultRole: "超级管理员", Priority: "P0", LegacyMinRole: common.RoleRootUser},
	{Group: "系统治理", Label: "后台任务", Path: "/admin/background-tasks", Permission: AdminPermissionSystemTaskRead, DefaultRole: "超级管理员", Priority: "P1", LegacyMinRole: common.RoleRootUser},
}

var adminDangerousOperationPermissionCatalog = []AdminDangerousOperationPermissionCatalogItem{
	{Page: "渠道管理", Operation: "删除渠道、批量删除、成本重算", Permission: AdminPermissionChannelChannelDanger, DefaultRole: "渠道管理员", Confirmation: "必须", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Page: "账号池管理", Operation: "批量启停、导入、归档、恢复、代理绑定、凭证替换和删除记录", Permission: AdminPermissionChannelAccountDanger, DefaultRole: "渠道管理员", Confirmation: "必须", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Page: "渠道健康检测", Operation: "立即探活、恢复健康、清理熔断", Permission: AdminPermissionChannelHealthExecute, DefaultRole: "渠道管理员", Confirmation: "建议", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Page: "路由策略", Operation: "保存调度配置、恢复默认", Permission: AdminPermissionModelRoutePolicyDanger, DefaultRole: "模型管理员", Confirmation: "必须", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Page: "倍率配置", Operation: "保存/重置模型倍率、分组倍率、上游价格同步和工具价格", Permission: AdminPermissionModelRatioUpdate, DefaultRole: "超级管理员", Confirmation: "必须", Priority: "P0", LegacyMinRole: common.RoleRootUser},
	{Page: "结算记录", Operation: "人工补单", Permission: AdminPermissionCommercialSettlementComplete, DefaultRole: "财务管理员", Confirmation: "必须", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Page: "用户管理", Operation: "禁用、重置安全、变更角色", Permission: AdminPermissionUserUserDanger, DefaultRole: "运营管理员", Confirmation: "必须", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Page: "系统设置", Operation: "保存支付、OAuth、SMTP、限流、性能设置", Permission: AdminPermissionSystemSettingsUpdate, DefaultRole: "超级管理员", Confirmation: "必须", Priority: "P0", LegacyMinRole: common.RoleRootUser},
	{Page: "权限角色", Operation: "创建、更新、禁用角色和分配用户权限", Permission: AdminPermissionSystemRolesUpdate, DefaultRole: "超级管理员", Confirmation: "必须", Priority: "P0", LegacyMinRole: common.RoleRootUser},
	{Page: "性能设置", Operation: "清理缓存、重置统计、触发 GC、删除日志", Permission: AdminPermissionSystemPerformanceDanger, DefaultRole: "超级管理员", Confirmation: "必须", Priority: "P1", LegacyMinRole: common.RoleRootUser},
}

var adminOperationPermissionCatalog = []AdminOperationPermissionCatalogItem{
	{Group: "渠道运营", Operation: "编辑渠道配置、恢复熔断和恢复健康状态", Permission: AdminPermissionChannelChannelUpdate, DefaultRole: "渠道管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "渠道运营", Operation: "创建、编辑和启停代理配置", Permission: AdminPermissionChannelProxyUpdate, DefaultRole: "渠道管理员", Priority: "P1", LegacyMinRole: common.RoleAdminUser},
	{Group: "模型与路由", Operation: "保存智能网关调度和观测配置", Permission: AdminPermissionModelGatewayUpdate, DefaultRole: "模型管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "模型与路由", Operation: "新增、编辑和同步模型配置", Permission: AdminPermissionModelModelUpdate, DefaultRole: "模型管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "模型与路由", Operation: "新增、编辑和发布模型部署", Permission: AdminPermissionModelDeploymentUpdate, DefaultRole: "模型管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "商业运营", Operation: "保存盈利监控和动态倍率建议", Permission: AdminPermissionCommercialProfitUpdate, DefaultRole: "财务管理员", Priority: "P0", LegacyMinRole: common.RoleAdminUser},
	{Group: "商业运营", Operation: "新增、编辑和上下架订阅方案", Permission: AdminPermissionCommercialSubscriptionUpdate, DefaultRole: "财务管理员", Priority: "P1", LegacyMinRole: common.RoleAdminUser},
	{Group: "商业运营", Operation: "新增、编辑和批量生成兑换码", Permission: AdminPermissionCommercialRedemptionUpdate, DefaultRole: "财务管理员", Priority: "P1", LegacyMinRole: common.RoleAdminUser},
}

var adminCompatibilityOperationPermissions = []struct {
	Permission    string
	LegacyMinRole int
}{
	{Permission: AdminPermissionChannelChannelUpdate, LegacyMinRole: common.RoleAdminUser},
	{Permission: AdminPermissionChannelProxyUpdate, LegacyMinRole: common.RoleAdminUser},
	{Permission: AdminPermissionCommercialProfitUpdate, LegacyMinRole: common.RoleAdminUser},
	{Permission: AdminPermissionCommercialRedemptionUpdate, LegacyMinRole: common.RoleAdminUser},
	{Permission: AdminPermissionCommercialSubscriptionUpdate, LegacyMinRole: common.RoleAdminUser},
	{Permission: AdminPermissionModelDeploymentUpdate, LegacyMinRole: common.RoleAdminUser},
	{Permission: AdminPermissionModelGatewayUpdate, LegacyMinRole: common.RoleAdminUser},
	{Permission: AdminPermissionModelModelUpdate, LegacyMinRole: common.RoleAdminUser},
}

func GetAdminPermissionCatalog() AdminPermissionCatalog {
	return AdminPermissionCatalog{
		Mode:                          adminPermissionModeRoleCompatibility,
		RoleTemplates:                 adminPermissionRoleTemplates,
		MenuPermissions:               adminMenuPermissionCatalog,
		DangerousOperationPermissions: adminDangerousOperationPermissionCatalog,
		OperationPermissions:          adminOperationPermissionCatalog,
	}
}

func GetAdminPermissionsForRole(role int) []string {
	if role >= common.RoleRootUser {
		return []string{"*"}
	}
	if role < common.RoleAdminUser {
		return nil
	}

	permissions := make([]string, 0, len(adminMenuPermissionCatalog)+len(adminDangerousOperationPermissionCatalog))
	seen := make(map[string]bool)
	for _, item := range adminMenuPermissionCatalog {
		if role >= item.LegacyMinRole && !seen[item.Permission] {
			permissions = append(permissions, item.Permission)
			seen[item.Permission] = true
		}
	}
	for _, item := range adminDangerousOperationPermissionCatalog {
		if role >= item.LegacyMinRole && !seen[item.Permission] {
			permissions = append(permissions, item.Permission)
			seen[item.Permission] = true
		}
	}
	for _, item := range adminOperationPermissionCatalog {
		if role >= item.LegacyMinRole && !seen[item.Permission] {
			permissions = append(permissions, item.Permission)
			seen[item.Permission] = true
		}
	}
	for _, item := range adminCompatibilityOperationPermissions {
		if role >= item.LegacyMinRole && !seen[item.Permission] {
			permissions = append(permissions, item.Permission)
			seen[item.Permission] = true
		}
	}
	sort.Strings(permissions)
	return permissions
}

// RequireAdminPermission attaches a stable permission point to an admin API route.
// The first stage keeps the existing role model as the compatibility source, while
// giving every guarded endpoint the same permission key used by the admin console.
func RequireAdminPermission(permission string, legacyMinRole ...int) gin.HandlerFunc {
	minRole := common.RoleAdminUser
	if len(legacyMinRole) > 0 {
		minRole = legacyMinRole[0]
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Set("admin_permission", permission)
		if c.GetInt("role") < minRole {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
			})
			recordAdminPermissionAudit(c, permission, minRole, "denied", start)
			c.Abort()
			return
		}
		allowed, source, err := model.CheckAdminPermission(c.GetInt("id"), c.GetInt("role"), permission)
		c.Set("admin_permission_source", source)
		if err != nil {
			common.SysLog("failed to check admin permission: " + err.Error())
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "权限检查失败",
			})
			recordAdminPermissionAudit(c, permission, minRole, "error", start)
			c.Abort()
			return
		}
		if !allowed {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": common.TranslateMessage(c, i18n.MsgAuthInsufficientPrivilege),
			})
			recordAdminPermissionAudit(c, permission, minRole, "denied", start)
			c.Abort()
			return
		}
		summary := adminPermissionAuditSummary(c)
		c.Next()
		summary = mergeAdminPermissionAuditSummary(summary, adminPermissionAuditExtraSummary(c))
		recordAdminPermissionAudit(c, permission, minRole, adminPermissionAuditResult(c), start, summary)
	}
}

func RequireRootAdminPermission(permission string) gin.HandlerFunc {
	return RequireAdminPermission(permission, common.RoleRootUser)
}

func recordAdminPermissionAudit(c *gin.Context, permission string, minRole int, result string, start time.Time, summary ...map[string]interface{}) {
	route := c.FullPath()
	if route == "" {
		route = c.Request.URL.Path
	}
	var auditSummary map[string]interface{}
	if len(summary) > 0 {
		auditSummary = summary[0]
	}
	source := c.GetString("admin_permission_source")
	model.RecordAdminAuditLog(c, model.AdminAuditLogParams{
		Permission: permission,
		Result:     result,
		Method:     c.Request.Method,
		Route:      route,
		Path:       c.Request.URL.Path,
		RequestId:  c.GetString(common.RequestIdKey),
		StatusCode: c.Writer.Status(),
		DurationMs: time.Since(start).Milliseconds(),
		MinRole:    minRole,
		Role:       c.GetInt("role"),
		Target:     adminPermissionAuditTarget(c),
		QueryKeys:  adminPermissionAuditQueryKeys(c),
		Summary:    auditSummary,
		Source:     source,
	})
}

func adminPermissionAuditResult(c *gin.Context) string {
	if len(c.Errors) > 0 {
		return "error"
	}
	if c.IsAborted() {
		return "aborted"
	}
	if c.Writer.Status() >= http.StatusBadRequest {
		return "http_error"
	}
	return "completed"
}

func adminPermissionAuditTarget(c *gin.Context) map[string]string {
	target := make(map[string]string, len(c.Params))
	for _, param := range c.Params {
		target[param.Key] = param.Value
	}
	return target
}

func adminPermissionAuditQueryKeys(c *gin.Context) []string {
	query := c.Request.URL.Query()
	if len(query) == 0 {
		return nil
	}
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func adminPermissionAuditSummary(c *gin.Context) map[string]interface{} {
	summary := make(map[string]interface{})
	if query := adminPermissionAuditQueryIdentifiers(c); len(query) > 0 {
		summary["query_identifiers"] = query
	}

	bodySummary := adminPermissionAuditJSONBodySummary(c)
	for key, value := range bodySummary {
		summary[key] = value
	}
	if len(summary) == 0 {
		return nil
	}
	return summary
}

func SetAdminAuditSummary(c *gin.Context, key string, value interface{}) {
	if c == nil || key == "" {
		return
	}
	summary, _ := c.Get("admin_audit_summary")
	result, _ := summary.(map[string]interface{})
	if result == nil {
		result = make(map[string]interface{})
	}
	result[key] = value
	c.Set("admin_audit_summary", result)
}

func adminPermissionAuditExtraSummary(c *gin.Context) map[string]interface{} {
	value, ok := c.Get("admin_audit_summary")
	if !ok {
		return nil
	}
	summary, _ := value.(map[string]interface{})
	return summary
}

func mergeAdminPermissionAuditSummary(base map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]interface{}, len(extra))
	}
	for key, value := range extra {
		base[key] = value
	}
	return base
}

func adminPermissionAuditQueryIdentifiers(c *gin.Context) map[string]interface{} {
	values := c.Request.URL.Query()
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]interface{})
	for key, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if !adminPermissionAuditAllowedIdentifierKey(normalized) || len(value) == 0 {
			continue
		}
		if scalar, ok := adminPermissionAuditSafeScalar(value[0]); ok {
			result[key] = scalar
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func adminPermissionAuditJSONBodySummary(c *gin.Context) map[string]interface{} {
	if c.Request == nil || c.Request.Body == nil || c.Request.ContentLength == 0 {
		return nil
	}
	contentType := strings.ToLower(c.Request.Header.Get("Content-Type"))
	if !strings.HasPrefix(contentType, "application/json") {
		return nil
	}
	if c.Request.ContentLength < 0 || c.Request.ContentLength > adminAuditMaxBodySummaryBytes {
		return map[string]interface{}{"body_summary": "skipped_large_or_unknown_json_body"}
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return map[string]interface{}{"body_summary": "unavailable"}
	}
	defer func() {
		if _, seekErr := storage.Seek(0, io.SeekStart); seekErr == nil {
			c.Request.Body = io.NopCloser(storage)
		}
	}()

	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return nil
	}
	var parsed map[string]interface{}
	if err := common.Unmarshal(body, &parsed); err != nil || parsed == nil {
		return map[string]interface{}{"body_summary": "non_object_json_body"}
	}

	keys := make([]string, 0, len(parsed))
	identifiers := make(map[string]interface{})
	for key, value := range parsed {
		keys = append(keys, key)
		normalized := strings.ToLower(strings.TrimSpace(key))
		if key == "key" && c.FullPath() == "/api/option/" {
			if scalar, ok := adminPermissionAuditSafeScalar(value); ok {
				identifiers["option_key"] = scalar
			}
			continue
		}
		if !adminPermissionAuditAllowedIdentifierKey(normalized) {
			continue
		}
		if scalar, ok := adminPermissionAuditSafeScalar(value); ok {
			identifiers[key] = scalar
		}
	}
	sort.Strings(keys)

	result := map[string]interface{}{"body_keys": keys}
	if len(identifiers) > 0 {
		result["body_identifiers"] = identifiers
	}
	return result
}

func adminPermissionAuditAllowedIdentifierKey(key string) bool {
	switch key {
	case "id",
		"user_id",
		"target_user_id",
		"channel_id",
		"credential_index",
		"profile_id",
		"provider_id",
		"proxy_id",
		"plan_id",
		"subscription_id",
		"user_subscription_id",
		"trade_no",
		"order_id",
		"model",
		"model_name",
		"upstream_model",
		"name",
		"group",
		"rule_name",
		"target_timestamp":
		return true
	default:
		return false
	}
}

func adminPermissionAuditSafeScalar(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" || len(trimmed) > 128 {
			return nil, false
		}
		return trimmed, true
	case float64, float32, int, int64, int32, uint, uint64, uint32, bool:
		return typed, true
	default:
		return nil, false
	}
}
