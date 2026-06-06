package model

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	AdminPermissionSourceRoleCompatibility = "role_compatibility"
	AdminPermissionSourceDatabase          = "database"
	AdminPermissionSourceRoot              = "root"

	AdminPermissionOverrideAllow = "allow"
	AdminPermissionOverrideDeny  = "deny"
)

type AdminRole struct {
	Id          int    `json:"id"`
	Key         string `json:"key" gorm:"type:varchar(64);uniqueIndex"`
	Name        string `json:"name" gorm:"type:varchar(64);index"`
	Code        string `json:"code" gorm:"type:varchar(32)"`
	Description string `json:"description" gorm:"type:varchar(255)"`
	Status      int    `json:"status" gorm:"type:int;default:1;index"`
	Builtin     int    `json:"builtin" gorm:"type:int;default:0"`
	SortOrder   int    `json:"sort_order" gorm:"type:int;default:0"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type AdminRolePermission struct {
	Id         int    `json:"id"`
	RoleId     int    `json:"role_id" gorm:"index:idx_admin_role_permission,unique"`
	Permission string `json:"permission" gorm:"type:varchar(128);index:idx_admin_role_permission,unique"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
}

type AdminUserRoleBinding struct {
	Id        int   `json:"id"`
	UserId    int   `json:"user_id" gorm:"index:idx_admin_user_role,unique"`
	RoleId    int   `json:"role_id" gorm:"index:idx_admin_user_role,unique"`
	Status    int   `json:"status" gorm:"type:int;default:1;index"`
	CreatedAt int64 `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt int64 `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type AdminUserPermissionOverride struct {
	Id         int    `json:"id"`
	UserId     int    `json:"user_id" gorm:"index:idx_admin_user_permission_override,unique"`
	Permission string `json:"permission" gorm:"type:varchar(128);index:idx_admin_user_permission_override,unique"`
	Effect     string `json:"effect" gorm:"type:varchar(16);default:'allow'"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	UpdatedAt  int64  `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type AdminRoleWithPermissions struct {
	AdminRole
	Permissions []string `json:"permissions"`
}

type AdminPermissionResolution struct {
	Source               string   `json:"source"`
	Permissions          []string `json:"permissions"`
	DeniedPermissions    []string `json:"denied_permissions,omitempty"`
	HasStoredAssignments bool     `json:"has_stored_assignments"`
}

type AdminUserPermissionAssignment struct {
	UserId               int                        `json:"user_id"`
	Source               string                     `json:"source"`
	Roles                []AdminRoleWithPermissions `json:"roles"`
	RoleIds              []int                      `json:"role_ids"`
	AllowPermissions     []string                   `json:"allow_permissions"`
	DenyPermissions      []string                   `json:"deny_permissions"`
	EffectivePermissions []string                   `json:"effective_permissions"`
}

type AdminBuiltinRoleSeed struct {
	Key         string
	Name        string
	Code        string
	Description string
	SortOrder   int
	Permissions []string
}

func ListAdminRolesWithPermissions() ([]AdminRoleWithPermissions, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var roles []AdminRole
	if err := DB.Order("sort_order asc, id asc").Find(&roles).Error; err != nil {
		return nil, err
	}
	return attachAdminRolePermissions(DB, roles)
}

func SyncAdminBuiltinRoles(seeds []AdminBuiltinRoleSeed) ([]AdminRoleWithPermissions, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		for _, seed := range seeds {
			key := strings.TrimSpace(seed.Key)
			name := strings.TrimSpace(seed.Name)
			if key == "" || name == "" {
				continue
			}
			var role AdminRole
			err := tx.Where("key = ?", key).First(&role).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				role = AdminRole{
					Key:         key,
					Name:        name,
					Code:        strings.TrimSpace(seed.Code),
					Description: strings.TrimSpace(seed.Description),
					Status:      common.UserStatusEnabled,
					Builtin:     1,
					SortOrder:   seed.SortOrder,
				}
				if err := tx.Create(&role).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			} else {
				updates := map[string]interface{}{
					"name":        name,
					"code":        strings.TrimSpace(seed.Code),
					"description": strings.TrimSpace(seed.Description),
					"status":      common.UserStatusEnabled,
					"builtin":     1,
					"sort_order":  seed.SortOrder,
				}
				if err := tx.Model(&AdminRole{}).Where("id = ?", role.Id).Updates(updates).Error; err != nil {
					return err
				}
			}
			if err := replaceAdminRolePermissions(tx, role.Id, seed.Permissions); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ListAdminRolesWithPermissions()
}

func GetAdminRoleWithPermissions(id int) (*AdminRoleWithPermissions, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	var role AdminRole
	if err := DB.First(&role, id).Error; err != nil {
		return nil, err
	}
	roles, err := attachAdminRolePermissions(DB, []AdminRole{role})
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &roles[0], nil
}

func UpsertAdminRoleWithPermissions(role AdminRole, permissions []string) (*AdminRoleWithPermissions, error) {
	if DB == nil {
		return nil, gorm.ErrInvalidDB
	}
	permissions = normalizeAdminPermissionList(permissions)
	err := DB.Transaction(func(tx *gorm.DB) error {
		if role.Status == 0 {
			role.Status = common.UserStatusEnabled
		}
		if role.Id > 0 {
			updates := map[string]interface{}{
				"key":         strings.TrimSpace(role.Key),
				"name":        strings.TrimSpace(role.Name),
				"code":        strings.TrimSpace(role.Code),
				"description": strings.TrimSpace(role.Description),
				"status":      role.Status,
				"sort_order":  role.SortOrder,
			}
			if err := tx.Model(&AdminRole{}).Where("id = ?", role.Id).Updates(updates).Error; err != nil {
				return err
			}
		} else {
			role.Key = strings.TrimSpace(role.Key)
			role.Name = strings.TrimSpace(role.Name)
			role.Code = strings.TrimSpace(role.Code)
			role.Description = strings.TrimSpace(role.Description)
			if err := tx.Create(&role).Error; err != nil {
				return err
			}
		}
		return replaceAdminRolePermissions(tx, role.Id, permissions)
	})
	if err != nil {
		return nil, err
	}
	return GetAdminRoleWithPermissions(role.Id)
}

func replaceAdminRolePermissions(tx *gorm.DB, roleId int, permissions []string) error {
	permissions = normalizeAdminPermissionList(permissions)
	if err := tx.Where("role_id = ?", roleId).Delete(&AdminRolePermission{}).Error; err != nil {
		return err
	}
	for _, permission := range permissions {
		if err := tx.Create(&AdminRolePermission{RoleId: roleId, Permission: permission}).Error; err != nil {
			return err
		}
	}
	return nil
}

func DisableAdminRole(id int) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	return DB.Model(&AdminRole{}).Where("id = ?", id).Update("status", common.UserStatusDisabled).Error
}

func GetAdminUserPermissionAssignment(userId int, fallback []string, role int) (*AdminUserPermissionAssignment, error) {
	resolution, err := ResolveAdminPermissions(userId, role, fallback)
	if err != nil {
		return nil, err
	}
	roles, roleIds, err := getAdminUserBoundRoles(userId)
	if err != nil {
		return nil, err
	}
	allows, denies, err := getAdminUserPermissionOverrides(userId)
	if err != nil {
		return nil, err
	}
	return &AdminUserPermissionAssignment{
		UserId:               userId,
		Source:               resolution.Source,
		Roles:                roles,
		RoleIds:              roleIds,
		AllowPermissions:     allows,
		DenyPermissions:      denies,
		EffectivePermissions: resolution.Permissions,
	}, nil
}

func SetAdminUserPermissionAssignment(userId int, roleIds []int, allowPermissions []string, denyPermissions []string) error {
	if DB == nil {
		return gorm.ErrInvalidDB
	}
	roleIds = normalizeAdminRoleIds(roleIds)
	allowPermissions = normalizeAdminPermissionList(allowPermissions)
	denyPermissions = normalizeAdminPermissionList(denyPermissions)
	allowPermissions = subtractDeniedAdminPermissions(allowPermissions, denyPermissions)

	return DB.Transaction(func(tx *gorm.DB) error {
		if len(roleIds) > 0 {
			var count int64
			if err := tx.Model(&AdminRole{}).Where("id IN ? AND status = ?", roleIds, common.UserStatusEnabled).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(roleIds)) {
				return gorm.ErrRecordNotFound
			}
		}
		if err := tx.Where("user_id = ?", userId).Delete(&AdminUserRoleBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userId).Delete(&AdminUserPermissionOverride{}).Error; err != nil {
			return err
		}
		for _, roleId := range roleIds {
			if err := tx.Create(&AdminUserRoleBinding{UserId: userId, RoleId: roleId, Status: common.UserStatusEnabled}).Error; err != nil {
				return err
			}
		}
		for _, permission := range allowPermissions {
			if err := tx.Create(&AdminUserPermissionOverride{UserId: userId, Permission: permission, Effect: AdminPermissionOverrideAllow}).Error; err != nil {
				return err
			}
		}
		for _, permission := range denyPermissions {
			if err := tx.Create(&AdminUserPermissionOverride{UserId: userId, Permission: permission, Effect: AdminPermissionOverrideDeny}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func ResolveAdminPermissions(userId int, role int, fallback []string) (AdminPermissionResolution, error) {
	if role >= common.RoleRootUser {
		return AdminPermissionResolution{
			Source:      AdminPermissionSourceRoot,
			Permissions: []string{"*"},
		}, nil
	}
	if role < common.RoleAdminUser {
		return AdminPermissionResolution{Source: AdminPermissionSourceRoleCompatibility}, nil
	}
	if DB == nil || userId <= 0 {
		return AdminPermissionResolution{
			Source:      AdminPermissionSourceRoleCompatibility,
			Permissions: normalizeAdminPermissionList(fallback),
		}, nil
	}

	hasStored, err := userHasStoredAdminPermissionAssignments(userId)
	if err != nil {
		return AdminPermissionResolution{}, err
	}
	if !hasStored {
		return AdminPermissionResolution{
			Source:      AdminPermissionSourceRoleCompatibility,
			Permissions: normalizeAdminPermissionList(fallback),
		}, nil
	}

	rolePermissions, err := getAdminUserRolePermissions(userId)
	if err != nil {
		return AdminPermissionResolution{}, err
	}
	allows, denies, err := getAdminUserPermissionOverrides(userId)
	if err != nil {
		return AdminPermissionResolution{}, err
	}
	grants := make(map[string]bool)
	for _, permission := range rolePermissions {
		grants[permission] = true
	}
	for _, permission := range allows {
		grants[permission] = true
	}
	for _, permission := range denies {
		delete(grants, permission)
	}
	permissions := make([]string, 0, len(grants))
	for permission := range grants {
		if adminPermissionDenied(denies, permission) {
			continue
		}
		permissions = append(permissions, permission)
	}
	sort.Strings(permissions)
	return AdminPermissionResolution{
		Source:               AdminPermissionSourceDatabase,
		Permissions:          permissions,
		DeniedPermissions:    denies,
		HasStoredAssignments: true,
	}, nil
}

func CheckAdminPermission(userId int, role int, permission string) (bool, string, error) {
	if role >= common.RoleRootUser {
		return true, AdminPermissionSourceRoot, nil
	}
	if role < common.RoleAdminUser {
		return false, AdminPermissionSourceRoleCompatibility, nil
	}
	if DB == nil || userId <= 0 {
		return true, AdminPermissionSourceRoleCompatibility, nil
	}
	hasStored, err := userHasStoredAdminPermissionAssignments(userId)
	if err != nil {
		return false, "", err
	}
	if !hasStored {
		return true, AdminPermissionSourceRoleCompatibility, nil
	}
	resolution, err := ResolveAdminPermissions(userId, role, nil)
	if err != nil {
		return false, "", err
	}
	for _, denied := range resolution.DeniedPermissions {
		if adminPermissionGrantMatches(denied, permission) {
			return false, resolution.Source, nil
		}
	}
	for _, grant := range resolution.Permissions {
		if adminPermissionGrantMatches(grant, permission) {
			return true, resolution.Source, nil
		}
	}
	return false, resolution.Source, nil
}

func attachAdminRolePermissions(tx *gorm.DB, roles []AdminRole) ([]AdminRoleWithPermissions, error) {
	result := make([]AdminRoleWithPermissions, 0, len(roles))
	if len(roles) == 0 {
		return result, nil
	}
	roleIds := make([]int, 0, len(roles))
	for _, role := range roles {
		roleIds = append(roleIds, role.Id)
	}
	var rows []AdminRolePermission
	if err := tx.Where("role_id IN ?", roleIds).Order("permission asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	byRole := make(map[int][]string)
	for _, row := range rows {
		byRole[row.RoleId] = append(byRole[row.RoleId], row.Permission)
	}
	for _, role := range roles {
		result = append(result, AdminRoleWithPermissions{
			AdminRole:   role,
			Permissions: normalizeAdminPermissionList(byRole[role.Id]),
		})
	}
	return result, nil
}

func getAdminUserBoundRoles(userId int) ([]AdminRoleWithPermissions, []int, error) {
	var bindings []AdminUserRoleBinding
	if err := DB.Where("user_id = ? AND status = ?", userId, common.UserStatusEnabled).Order("role_id asc").Find(&bindings).Error; err != nil {
		return nil, nil, err
	}
	if len(bindings) == 0 {
		return nil, nil, nil
	}
	roleIds := make([]int, 0, len(bindings))
	for _, binding := range bindings {
		roleIds = append(roleIds, binding.RoleId)
	}
	var roles []AdminRole
	if err := DB.Where("id IN ? AND status = ?", roleIds, common.UserStatusEnabled).Order("sort_order asc, id asc").Find(&roles).Error; err != nil {
		return nil, nil, err
	}
	withPermissions, err := attachAdminRolePermissions(DB, roles)
	if err != nil {
		return nil, nil, err
	}
	return withPermissions, roleIds, nil
}

func getAdminUserRolePermissions(userId int) ([]string, error) {
	var rows []struct {
		Permission string `gorm:"column:permission"`
	}
	err := DB.Table("admin_role_permissions").
		Select("admin_role_permissions.permission").
		Joins("INNER JOIN admin_user_role_bindings ON admin_user_role_bindings.role_id = admin_role_permissions.role_id").
		Joins("INNER JOIN admin_roles ON admin_roles.id = admin_role_permissions.role_id").
		Where("admin_user_role_bindings.user_id = ? AND admin_user_role_bindings.status = ? AND admin_roles.status = ?", userId, common.UserStatusEnabled, common.UserStatusEnabled).
		Order("admin_role_permissions.permission asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	permissions := make([]string, 0, len(rows))
	for _, row := range rows {
		permissions = append(permissions, row.Permission)
	}
	return normalizeAdminPermissionList(permissions), nil
}

func getAdminUserPermissionOverrides(userId int) ([]string, []string, error) {
	var rows []AdminUserPermissionOverride
	if err := DB.Where("user_id = ?", userId).Order("permission asc").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	allows := make([]string, 0)
	denies := make([]string, 0)
	for _, row := range rows {
		switch strings.ToLower(strings.TrimSpace(row.Effect)) {
		case AdminPermissionOverrideDeny:
			denies = append(denies, row.Permission)
		default:
			allows = append(allows, row.Permission)
		}
	}
	return normalizeAdminPermissionList(allows), normalizeAdminPermissionList(denies), nil
}

func userHasStoredAdminPermissionAssignments(userId int) (bool, error) {
	var count int64
	if err := DB.Model(&AdminUserRoleBinding{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := DB.Model(&AdminUserPermissionOverride{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func normalizeAdminPermissionList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeAdminRoleIds(values []int) []int {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(values))
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func adminPermissionGrantMatches(grant string, permission string) bool {
	grant = strings.TrimSpace(grant)
	permission = strings.TrimSpace(permission)
	if grant == "" || permission == "" {
		return false
	}
	if grant == "*" || grant == permission {
		return true
	}
	grantParts := strings.Split(grant, ":")
	permissionParts := strings.Split(permission, ":")
	if len(grantParts) > len(permissionParts) {
		return false
	}
	for i := range grantParts {
		if grantParts[i] == "*" {
			return true
		}
		if grantParts[i] != permissionParts[i] {
			return false
		}
	}
	return len(grantParts) == len(permissionParts)
}

func subtractDeniedAdminPermissions(allows []string, denies []string) []string {
	if len(allows) == 0 || len(denies) == 0 {
		return allows
	}
	result := make([]string, 0, len(allows))
	for _, permission := range allows {
		if adminPermissionDenied(denies, permission) {
			continue
		}
		result = append(result, permission)
	}
	return normalizeAdminPermissionList(result)
}

func adminPermissionDenied(denies []string, permission string) bool {
	for _, denied := range denies {
		if adminPermissionGrantMatches(denied, permission) {
			return true
		}
	}
	return false
}
