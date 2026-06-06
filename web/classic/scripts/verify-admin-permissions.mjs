import {
  ADMIN_LEGACY_ROLE,
  ADMIN_PERMISSION_KEYS,
  ADMIN_ROOT_ROLE,
  adminDangerousOperationPermissions,
  adminMenuPermissions,
  adminOperationPermissions,
  adminTargetRoleTemplates,
  getAdminRoutePermission,
  hasAdminPermission,
} from '../src/apps/admin-console/permissions/adminPermissions.config.js';
import { adminConsoleNavGroups } from '../src/apps/admin-console/navigation/adminConsoleNav.config.js';

const allPermissionItems = [
  ...adminMenuPermissions,
  ...adminDangerousOperationPermissions,
  ...adminOperationPermissions,
];

const allPermissions = [
  ...new Set(allPermissionItems.map((item) => item.permission)),
];
const menuPaths = adminMenuPermissions.map((item) => item.path);
const navItems = adminConsoleNavGroups.flatMap((group) =>
  (group.items || []).map((item) => ({ ...item, group: group.label })),
);

function fail(message) {
  throw new Error(message);
}

function assert(condition, message) {
  if (!condition) {
    fail(message);
  }
}

function assertSetEqual(actual, expected, label) {
  const actualSet = new Set(actual);
  const expectedSet = new Set(expected);
  const missing = [...expectedSet].filter((item) => !actualSet.has(item));
  const extra = [...actualSet].filter((item) => !expectedSet.has(item));
  assert(
    missing.length === 0 && extra.length === 0,
    `${label} mismatch\nmissing: ${missing.join(', ') || '-'}\nextra: ${extra.join(', ') || '-'}`,
  );
}

function permissionsForDefaultRole(roleName) {
  return allPermissionItems
    .filter((item) => item.defaultRole === roleName)
    .map((item) => item.permission);
}

function menuPathsForDefaultRole(roleName) {
  return adminMenuPermissions
    .filter((item) => item.defaultRole === roleName)
    .map((item) => item.path);
}

function visibleNavPaths(user) {
  return navItems
    .filter((item) =>
      hasAdminPermission(
        user,
        item.permission || getAdminRoutePermission(item.path),
      ),
    )
    .map((item) => item.path)
    .sort();
}

function accessibleMenuPaths(user) {
  return adminMenuPermissions
    .filter((item) => hasAdminPermission(user, item.permission))
    .map((item) => item.path)
    .sort();
}

function canAccessRoute(user, path) {
  return hasAdminPermission(user, getAdminRoutePermission(path));
}

function assertAllPermissions(user, expected, label) {
  for (const permission of allPermissions) {
    assert(
      hasAdminPermission(user, permission) === expected,
      `${label}: expected ${permission} to be ${expected ? 'allowed' : 'denied'}`,
    );
  }
}

const specialistRouteExpectations = [
  {
    roleName: '运营管理员',
    allowedPaths: [
      '/admin/overview',
      '/admin/realtime-monitor',
      '/admin/channel-alerts',
      '/admin/users',
      '/admin/user-segments',
      '/admin/risk-records',
      '/admin/invite-rebates',
    ],
    deniedPaths: [
      '/admin/channels',
      '/admin/model-gateway',
      '/admin/profit-monitor',
      '/admin/settings',
    ],
  },
  {
    roleName: '渠道管理员',
    allowedPaths: [
      '/admin/channels',
      '/admin/channel-accounts',
      '/admin/channel-balance-monitor',
      '/admin/channel-health-check',
      '/admin/channel-proxies',
    ],
    deniedPaths: [
      '/admin/overview',
      '/admin/model-gateway',
      '/admin/profit-monitor',
      '/admin/settings',
    ],
  },
  {
    roleName: '模型管理员',
    allowedPaths: [
      '/admin/model-gateway',
      '/admin/models',
      '/admin/deployment',
      '/admin/route-policy',
    ],
    deniedPaths: [
      '/admin/channels',
      '/admin/profit-monitor',
      '/admin/users',
      '/admin/ratio-config',
      '/admin/settings',
    ],
  },
  {
    roleName: '财务管理员',
    allowedPaths: [
      '/admin/profit-monitor',
      '/admin/subscription',
      '/admin/redemption',
      '/admin/settlements',
      '/admin/consumption',
    ],
    deniedPaths: [
      '/admin/channels',
      '/admin/model-gateway',
      '/admin/users',
      '/admin/settings',
    ],
  },
];

function runSpecialistRouteChecks() {
  for (const expectation of specialistRouteExpectations) {
    const permissions = permissionsForDefaultRole(expectation.roleName);
    const user = {
      role: ADMIN_LEGACY_ROLE,
      admin_permissions: permissions,
      admin_permission_source: 'database',
    };

    assertSetEqual(
      menuPathsForDefaultRole(expectation.roleName).sort(),
      expectation.allowedPaths.sort(),
      `${expectation.roleName} expected menu paths`,
    );
    assertSetEqual(
      visibleNavPaths(user),
      expectation.allowedPaths.sort(),
      `${expectation.roleName} visible nav paths`,
    );

    for (const path of expectation.allowedPaths) {
      assert(
        canAccessRoute(user, path),
        `${expectation.roleName}: expected route ${path} to be allowed`,
      );
    }

    for (const path of expectation.deniedPaths) {
      assert(
        !canAccessRoute(user, path),
        `${expectation.roleName}: expected route ${path} to be denied`,
      );
    }
  }
}

function runCatalogChecks() {
  assert(
    adminTargetRoleTemplates.length === 5,
    'expected 5 admin role templates',
  );
  assert(
    adminMenuPermissions.length === 26,
    'expected 26 admin menu permissions',
  );
  assert(
    adminDangerousOperationPermissions.length === 10,
    'expected 10 dangerous operation permissions',
  );
  assert(
    adminOperationPermissions.length === 8,
    'expected 8 operation permissions',
  );
  assert(allPermissions.length === 44, 'expected 44 unique admin permissions');

  assertSetEqual(
    navItems.map((item) => item.path).sort(),
    menuPaths.sort(),
    'admin navigation paths vs menu permission paths',
  );

  for (const item of adminMenuPermissions) {
    assert(
      getAdminRoutePermission(item.path) === item.permission,
      `route permission mismatch for ${item.path}`,
    );
  }

  assert(
    getAdminRoutePermission('/admin/channel/123/accounts') ===
      ADMIN_PERMISSION_KEYS.channelAccountRead,
    'legacy channel account route should resolve to channel account read permission',
  );
}

function runRoleMatrixChecks() {
  const normalUser = { role: 1, admin_permissions: [] };
  assertAllPermissions(normalUser, false, 'normal user');
  assertSetEqual(
    accessibleMenuPaths(normalUser),
    [],
    'normal user accessible menu',
  );
  assertSetEqual(visibleNavPaths(normalUser), [], 'normal user visible nav');

  const revokedAdmin = { role: ADMIN_LEGACY_ROLE, admin_permissions: [] };
  assertAllPermissions(revokedAdmin, false, 'revoked admin');
  assertSetEqual(
    accessibleMenuPaths(revokedAdmin),
    [],
    'revoked admin accessible menu',
  );
  assertSetEqual(
    visibleNavPaths(revokedAdmin),
    [],
    'revoked admin visible nav',
  );

  const legacyAdmin = { role: ADMIN_LEGACY_ROLE };
  const legacyExpectedPaths = adminMenuPermissions
    .filter((item) => item.legacyMinRole <= ADMIN_LEGACY_ROLE)
    .map((item) => item.path)
    .sort();
  assertSetEqual(
    accessibleMenuPaths(legacyAdmin),
    legacyExpectedPaths,
    'legacy role-compatible admin accessible menu',
  );
  assert(
    !hasAdminPermission(legacyAdmin, ADMIN_PERMISSION_KEYS.systemSettingsRead),
    'legacy admin must not read root settings',
  );
  assert(
    !hasAdminPermission(legacyAdmin, ADMIN_PERMISSION_KEYS.systemAuditRead),
    'legacy admin must not read root audit logs',
  );
  assert(
    !hasAdminPermission(legacyAdmin, ADMIN_PERMISSION_KEYS.systemTaskRead),
    'legacy admin must not read root background tasks',
  );
  assert(
    !hasAdminPermission(legacyAdmin, ADMIN_PERMISSION_KEYS.modelRatioUpdate),
    'legacy admin must not update root-only ratio config',
  );

  for (const template of adminTargetRoleTemplates.filter(
    (item) => item.key !== 'root',
  )) {
    const permissions = permissionsForDefaultRole(template.name);
    const user = {
      role: ADMIN_LEGACY_ROLE,
      admin_permissions: permissions,
      admin_permission_source: 'database',
    };
    assertSetEqual(
      accessibleMenuPaths(user),
      menuPathsForDefaultRole(template.name).sort(),
      `${template.name} accessible menu`,
    );
    for (const permission of permissions) {
      assert(
        hasAdminPermission(user, permission),
        `${template.name}: expected own permission ${permission} to be allowed`,
      );
    }
    const deniedPermissions = allPermissions.filter(
      (permission) => !permissions.includes(permission),
    );
    for (const permission of deniedPermissions) {
      assert(
        !hasAdminPermission(user, permission),
        `${template.name}: expected foreign permission ${permission} to be denied`,
      );
    }
  }

  const rootUser = {
    role: ADMIN_ROOT_ROLE,
    admin_permissions: [],
    admin_permission_source: 'root',
  };
  assertAllPermissions(rootUser, true, 'root user');
  assertSetEqual(
    accessibleMenuPaths(rootUser),
    menuPaths.sort(),
    'root accessible menu',
  );
  assertSetEqual(
    visibleNavPaths(rootUser),
    menuPaths.sort(),
    'root visible nav',
  );

  const wildcardChannelAdmin = {
    role: ADMIN_LEGACY_ROLE,
    admin_permissions: ['admin:channel:*'],
    admin_permission_source: 'database',
  };
  assert(
    hasAdminPermission(
      wildcardChannelAdmin,
      ADMIN_PERMISSION_KEYS.channelAccountRead,
    ),
    'channel wildcard should allow channel account read',
  );
  assert(
    hasAdminPermission(
      wildcardChannelAdmin,
      ADMIN_PERMISSION_KEYS.channelChannelDanger,
    ),
    'channel wildcard should allow channel danger permission',
  );
  assert(
    !hasAdminPermission(
      wildcardChannelAdmin,
      ADMIN_PERMISSION_KEYS.modelGatewayRead,
    ),
    'channel wildcard must not allow model gateway read',
  );
}

function main() {
  runCatalogChecks();
  runSpecialistRouteChecks();
  runRoleMatrixChecks();
  console.log('Admin permission matrix verification passed.');
}

main();
