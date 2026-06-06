package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdminPermissionResolutionUsesDatabaseAssignments(t *testing.T) {
	oldDB := DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AdminRole{}, &AdminRolePermission{}, &AdminUserRoleBinding{}, &AdminUserPermissionOverride{}))
	DB = db
	t.Cleanup(func() {
		DB = oldDB
	})

	fallback := []string{"admin:legacy:read"}
	resolution, err := ResolveAdminPermissions(7, common.RoleAdminUser, fallback)
	require.NoError(t, err)
	require.Equal(t, AdminPermissionSourceRoleCompatibility, resolution.Source)
	require.Equal(t, []string{"admin:legacy:read"}, resolution.Permissions)

	role, err := UpsertAdminRoleWithPermissions(AdminRole{
		Key:  "channel-admin",
		Name: "渠道管理员",
	}, []string{"admin:channel:*", "admin:system:roles:read"})
	require.NoError(t, err)
	require.NotZero(t, role.Id)

	require.NoError(t, SetAdminUserPermissionAssignment(7, []int{role.Id}, []string{"admin:commercial:settlement:read"}, []string{"admin:system:roles:read"}))

	resolution, err = ResolveAdminPermissions(7, common.RoleAdminUser, fallback)
	require.NoError(t, err)
	require.Equal(t, AdminPermissionSourceDatabase, resolution.Source)
	require.Contains(t, resolution.Permissions, "admin:channel:*")
	require.Contains(t, resolution.Permissions, "admin:commercial:settlement:read")
	require.NotContains(t, resolution.Permissions, "admin:system:roles:read")
	require.Contains(t, resolution.DeniedPermissions, "admin:system:roles:read")

	allowed, source, err := CheckAdminPermission(7, common.RoleAdminUser, "admin:channel:channel:update")
	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, AdminPermissionSourceDatabase, source)

	allowed, _, err = CheckAdminPermission(7, common.RoleAdminUser, "admin:system:roles:read")
	require.NoError(t, err)
	require.False(t, allowed)

	rootResolution, err := ResolveAdminPermissions(1, common.RoleRootUser, nil)
	require.NoError(t, err)
	require.Equal(t, AdminPermissionSourceRoot, rootResolution.Source)
	require.Equal(t, []string{"*"}, rootResolution.Permissions)
}

func TestSyncAdminBuiltinRolesCreatesAndRefreshesRoles(t *testing.T) {
	oldDB := DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&AdminRole{}, &AdminRolePermission{}))
	DB = db
	t.Cleanup(func() {
		DB = oldDB
	})

	roles, err := SyncAdminBuiltinRoles([]AdminBuiltinRoleSeed{
		{
			Key:         "channel_admin",
			Name:        "渠道管理员",
			Code:        "Channel",
			Description: "维护渠道",
			SortOrder:   20,
			Permissions: []string{"admin:channel:channel:read", "admin:channel:channel:update"},
		},
	})
	require.NoError(t, err)
	require.Len(t, roles, 1)
	require.Equal(t, "channel_admin", roles[0].Key)
	require.Equal(t, 1, roles[0].Builtin)
	require.Equal(t, common.UserStatusEnabled, roles[0].Status)
	require.Equal(t, []string{"admin:channel:channel:read", "admin:channel:channel:update"}, roles[0].Permissions)

	roles, err = SyncAdminBuiltinRoles([]AdminBuiltinRoleSeed{
		{
			Key:         "channel_admin",
			Name:        "渠道管理员",
			Code:        "Channel",
			Description: "维护渠道和账号池",
			SortOrder:   10,
			Permissions: []string{"admin:channel:account:read"},
		},
	})
	require.NoError(t, err)
	require.Len(t, roles, 1)
	require.Equal(t, "维护渠道和账号池", roles[0].Description)
	require.Equal(t, 10, roles[0].SortOrder)
	require.Equal(t, []string{"admin:channel:account:read"}, roles[0].Permissions)
}
