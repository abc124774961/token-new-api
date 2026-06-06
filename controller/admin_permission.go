package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type adminPermissionRoleRequest struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Code        string   `json:"code"`
	Description string   `json:"description"`
	Status      int      `json:"status"`
	SortOrder   int      `json:"sort_order"`
	Permissions []string `json:"permissions"`
}

type adminUserPermissionAssignmentRequest struct {
	RoleIds          []int    `json:"role_ids"`
	AllowPermissions []string `json:"allow_permissions"`
	DenyPermissions  []string `json:"deny_permissions"`
}

func GetAdminPermissionConfig(c *gin.Context) {
	roles, err := model.ListAdminRolesWithPermissions()
	if err != nil && !errors.Is(err, gorm.ErrInvalidDB) {
		common.ApiError(c, err)
		return
	}
	catalog := middleware.GetAdminPermissionCatalog()
	common.ApiSuccess(c, gin.H{
		"mode":                            model.AdminPermissionSourceRoleCompatibility,
		"role_templates":                  catalog.RoleTemplates,
		"menu_permissions":                catalog.MenuPermissions,
		"dangerous_operation_permissions": catalog.DangerousOperationPermissions,
		"operation_permissions":           catalog.OperationPermissions,
		"stored_roles":                    roles,
	})
}

func GetAdminSelfPermissions(c *gin.Context) {
	role := c.GetInt("role")
	common.ApiSuccess(c, adminPermissionUserFields(c.GetInt("id"), role))
}

func ListAdminPermissionRoles(c *gin.Context) {
	roles, err := model.ListAdminRolesWithPermissions()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"roles": roles,
	})
}

func SyncAdminPermissionRoleTemplates(c *gin.Context) {
	seeds := buildAdminPermissionRoleSeeds()
	roles, err := model.SyncAdminBuiltinRoles(seeds)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	middleware.SetAdminAuditSummary(c, "operation", "sync_admin_role_templates")
	middleware.SetAdminAuditSummary(c, "role_count", len(seeds))
	middleware.SetAdminAuditSummary(c, "permission_count", countAdminPermissionRoleSeedPermissions(seeds))
	common.ApiSuccess(c, gin.H{
		"roles": roles,
	})
}

func CreateAdminPermissionRole(c *gin.Context) {
	var req adminPermissionRoleRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	role, err := saveAdminPermissionRole(0, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	setAdminRoleAuditSummary(c, "create_admin_role", nil, role)
	common.ApiSuccess(c, role)
}

func UpdateAdminPermissionRole(c *gin.Context) {
	roleId, ok := adminPermissionPathInt(c, "id")
	if !ok {
		return
	}
	var req adminPermissionRoleRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	before, err := model.GetAdminRoleWithPermissions(roleId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	role, err := saveAdminPermissionRole(roleId, req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	setAdminRoleAuditSummary(c, "update_admin_role", before, role)
	common.ApiSuccess(c, role)
}

func DisableAdminPermissionRole(c *gin.Context) {
	roleId, ok := adminPermissionPathInt(c, "id")
	if !ok {
		return
	}
	before, err := model.GetAdminRoleWithPermissions(roleId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DisableAdminRole(roleId); err != nil {
		common.ApiError(c, err)
		return
	}
	after := *before
	after.Status = common.UserStatusDisabled
	setAdminRoleAuditSummary(c, "disable_admin_role", before, &after)
	common.ApiSuccess(c, nil)
}

func GetAdminUserPermissionAssignment(c *gin.Context) {
	userId, ok := adminPermissionPathInt(c, "id")
	if !ok {
		return
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	assignment, err := model.GetAdminUserPermissionAssignment(userId, middleware.GetAdminPermissionsForRole(user.Role), user.Role)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, assignment)
}

func UpdateAdminUserPermissionAssignment(c *gin.Context) {
	userId, ok := adminPermissionPathInt(c, "id")
	if !ok {
		return
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	before, err := model.GetAdminUserPermissionAssignment(userId, middleware.GetAdminPermissionsForRole(user.Role), user.Role)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req adminUserPermissionAssignmentRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetAdminUserPermissionAssignment(userId, req.RoleIds, req.AllowPermissions, req.DenyPermissions); err != nil {
		common.ApiError(c, err)
		return
	}
	after, err := model.GetAdminUserPermissionAssignment(userId, middleware.GetAdminPermissionsForRole(user.Role), user.Role)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	setAdminUserAssignmentAuditSummary(c, userId, before, after)
	common.ApiSuccess(c, nil)
}

func adminPermissionUserFields(userId int, role int) gin.H {
	fallback := middleware.GetAdminPermissionsForRole(role)
	resolution, err := model.ResolveAdminPermissions(userId, role, fallback)
	if err != nil {
		common.SysLog("failed to resolve admin permissions: " + err.Error())
		resolution = model.AdminPermissionResolution{
			Source:      model.AdminPermissionSourceRoleCompatibility,
			Permissions: fallback,
		}
	}
	return gin.H{
		"mode":                     resolution.Source,
		"source":                   resolution.Source,
		"role":                     role,
		"admin_permissions":        resolution.Permissions,
		"denied_admin_permissions": resolution.DeniedPermissions,
		"admin_permission_mode":    resolution.Source,
		"admin_permission_source":  resolution.Source,
	}
}

func appendAdminPermissionUserFields(data map[string]any, userId int, role int) {
	if role < common.RoleAdminUser {
		return
	}
	for key, value := range adminPermissionUserFields(userId, role) {
		data[key] = value
	}
}

func saveAdminPermissionRole(roleId int, req adminPermissionRoleRequest) (*model.AdminRoleWithPermissions, error) {
	role := model.AdminRole{
		Id:          roleId,
		Key:         strings.TrimSpace(req.Key),
		Name:        strings.TrimSpace(req.Name),
		Code:        strings.TrimSpace(req.Code),
		Description: strings.TrimSpace(req.Description),
		Status:      req.Status,
		SortOrder:   req.SortOrder,
	}
	if role.Key == "" || role.Name == "" {
		return nil, errors.New("角色 key 和名称不能为空")
	}
	return model.UpsertAdminRoleWithPermissions(role, req.Permissions)
}

func buildAdminPermissionRoleSeeds() []model.AdminBuiltinRoleSeed {
	catalog := middleware.GetAdminPermissionCatalog()
	type permissionRolePair struct {
		permission  string
		defaultRole string
	}
	pairs := make([]permissionRolePair, 0, len(catalog.MenuPermissions)+len(catalog.DangerousOperationPermissions)+len(catalog.OperationPermissions))
	allPermissions := make([]string, 0, len(pairs))
	appendPair := func(permission string, defaultRole string) {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			return
		}
		pairs = append(pairs, permissionRolePair{
			permission:  permission,
			defaultRole: strings.TrimSpace(defaultRole),
		})
		allPermissions = append(allPermissions, permission)
	}
	for _, item := range catalog.MenuPermissions {
		appendPair(item.Permission, item.DefaultRole)
	}
	for _, item := range catalog.DangerousOperationPermissions {
		appendPair(item.Permission, item.DefaultRole)
	}
	for _, item := range catalog.OperationPermissions {
		appendPair(item.Permission, item.DefaultRole)
	}

	seeds := make([]model.AdminBuiltinRoleSeed, 0, len(catalog.RoleTemplates))
	for index, template := range catalog.RoleTemplates {
		permissions := make([]string, 0)
		if template.Key == "root" {
			permissions = allPermissions
		} else {
			for _, pair := range pairs {
				if pair.defaultRole == template.Name {
					permissions = append(permissions, pair.permission)
				}
			}
		}
		seeds = append(seeds, model.AdminBuiltinRoleSeed{
			Key:         template.Key,
			Name:        template.Name,
			Code:        template.Code,
			Description: template.Description,
			SortOrder:   (index + 1) * 10,
			Permissions: permissions,
		})
	}
	return seeds
}

func countAdminPermissionRoleSeedPermissions(seeds []model.AdminBuiltinRoleSeed) int {
	count := 0
	for _, seed := range seeds {
		count += len(seed.Permissions)
	}
	return count
}

func setAdminRoleAuditSummary(c *gin.Context, operation string, before *model.AdminRoleWithPermissions, after *model.AdminRoleWithPermissions) {
	middleware.SetAdminAuditSummary(c, "operation", operation)
	if after != nil {
		middleware.SetAdminAuditSummary(c, "role_id", after.Id)
		middleware.SetAdminAuditSummary(c, "role_key", after.Key)
		middleware.SetAdminAuditSummary(c, "role_name", after.Name)
		middleware.SetAdminAuditSummary(c, "status_after", after.Status)
		middleware.SetAdminAuditSummary(c, "permission_count_after", len(after.Permissions))
	}
	if before != nil {
		middleware.SetAdminAuditSummary(c, "status_before", before.Status)
		middleware.SetAdminAuditSummary(c, "permission_count_before", len(before.Permissions))
	}
	added, removed := adminPermissionStringSetDelta(adminRolePermissions(before), adminRolePermissions(after))
	middleware.SetAdminAuditSummary(c, "permission_added_count", added)
	middleware.SetAdminAuditSummary(c, "permission_removed_count", removed)
}

func setAdminUserAssignmentAuditSummary(c *gin.Context, userId int, before *model.AdminUserPermissionAssignment, after *model.AdminUserPermissionAssignment) {
	middleware.SetAdminAuditSummary(c, "operation", "update_admin_user_permissions")
	middleware.SetAdminAuditSummary(c, "target_user_id", userId)
	middleware.SetAdminAuditSummary(c, "role_count_before", len(adminAssignmentRoleIds(before)))
	middleware.SetAdminAuditSummary(c, "role_count_after", len(adminAssignmentRoleIds(after)))
	middleware.SetAdminAuditSummary(c, "allow_permission_count_before", len(adminAssignmentAllowPermissions(before)))
	middleware.SetAdminAuditSummary(c, "allow_permission_count_after", len(adminAssignmentAllowPermissions(after)))
	middleware.SetAdminAuditSummary(c, "deny_permission_count_before", len(adminAssignmentDenyPermissions(before)))
	middleware.SetAdminAuditSummary(c, "deny_permission_count_after", len(adminAssignmentDenyPermissions(after)))
	middleware.SetAdminAuditSummary(c, "effective_permission_count_before", len(adminAssignmentEffectivePermissions(before)))
	middleware.SetAdminAuditSummary(c, "effective_permission_count_after", len(adminAssignmentEffectivePermissions(after)))

	addedRoles, removedRoles := adminPermissionIntSetDelta(adminAssignmentRoleIds(before), adminAssignmentRoleIds(after))
	addedAllows, removedAllows := adminPermissionStringSetDelta(adminAssignmentAllowPermissions(before), adminAssignmentAllowPermissions(after))
	addedDenies, removedDenies := adminPermissionStringSetDelta(adminAssignmentDenyPermissions(before), adminAssignmentDenyPermissions(after))
	addedEffective, removedEffective := adminPermissionStringSetDelta(adminAssignmentEffectivePermissions(before), adminAssignmentEffectivePermissions(after))
	middleware.SetAdminAuditSummary(c, "role_added_count", addedRoles)
	middleware.SetAdminAuditSummary(c, "role_removed_count", removedRoles)
	middleware.SetAdminAuditSummary(c, "allow_permission_added_count", addedAllows)
	middleware.SetAdminAuditSummary(c, "allow_permission_removed_count", removedAllows)
	middleware.SetAdminAuditSummary(c, "deny_permission_added_count", addedDenies)
	middleware.SetAdminAuditSummary(c, "deny_permission_removed_count", removedDenies)
	middleware.SetAdminAuditSummary(c, "effective_permission_added_count", addedEffective)
	middleware.SetAdminAuditSummary(c, "effective_permission_removed_count", removedEffective)
}

func adminRolePermissions(role *model.AdminRoleWithPermissions) []string {
	if role == nil {
		return nil
	}
	return role.Permissions
}

func adminAssignmentRoleIds(assignment *model.AdminUserPermissionAssignment) []int {
	if assignment == nil {
		return nil
	}
	return assignment.RoleIds
}

func adminAssignmentAllowPermissions(assignment *model.AdminUserPermissionAssignment) []string {
	if assignment == nil {
		return nil
	}
	return assignment.AllowPermissions
}

func adminAssignmentDenyPermissions(assignment *model.AdminUserPermissionAssignment) []string {
	if assignment == nil {
		return nil
	}
	return assignment.DenyPermissions
}

func adminAssignmentEffectivePermissions(assignment *model.AdminUserPermissionAssignment) []string {
	if assignment == nil {
		return nil
	}
	return assignment.EffectivePermissions
}

func adminPermissionStringSetDelta(before []string, after []string) (int, int) {
	beforeSet := make(map[string]bool, len(before))
	afterSet := make(map[string]bool, len(after))
	for _, value := range before {
		value = strings.TrimSpace(value)
		if value != "" {
			beforeSet[value] = true
		}
	}
	for _, value := range after {
		value = strings.TrimSpace(value)
		if value != "" {
			afterSet[value] = true
		}
	}
	return adminPermissionSetDelta(beforeSet, afterSet)
}

func adminPermissionIntSetDelta(before []int, after []int) (int, int) {
	beforeSet := make(map[int]bool, len(before))
	afterSet := make(map[int]bool, len(after))
	for _, value := range before {
		beforeSet[value] = true
	}
	for _, value := range after {
		afterSet[value] = true
	}
	added := 0
	for value := range afterSet {
		if !beforeSet[value] {
			added++
		}
	}
	removed := 0
	for value := range beforeSet {
		if !afterSet[value] {
			removed++
		}
	}
	return added, removed
}

func adminPermissionSetDelta(before map[string]bool, after map[string]bool) (int, int) {
	added := 0
	for value := range after {
		if !before[value] {
			added++
		}
	}
	removed := 0
	for value := range before {
		if !after[value] {
			removed++
		}
	}
	return added, removed
}

func adminPermissionPathInt(c *gin.Context, key string) (int, bool) {
	value, err := strconv.Atoi(c.Param(key))
	if err != nil || value <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return 0, false
	}
	return value, true
}
