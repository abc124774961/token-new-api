import { adminConsoleNavGroups } from '../src/apps/admin-console/navigation/adminConsoleNav.config.js';
import { getAdminRoutePermission } from '../src/apps/admin-console/permissions/adminPermissions.config.js';
import { legacyAdminRedirects } from '../src/apps/admin-console/routes/adminLegacyRedirects.config.js';
import { userConsoleNavGroups } from '../src/apps/user-console/navigation/userConsoleNav.config.js';
import {
  userConsoleHiddenShellRoutes,
  isUserConsoleShellRoute,
  userConsoleShellRoutes,
} from '../src/apps/user-console/routes/userConsoleRoutes.config.js';

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
    `${label} mismatch\nmissing: ${missing.join(', ') || '-'}\nextra: ${
      extra.join(', ') || '-'
    }`,
  );
}

function flattenNav(groups) {
  return groups.flatMap((group) =>
    (group.items || []).map((item) => ({
      ...item,
      groupKey: group.key,
      groupLabel: group.label,
    })),
  );
}

const userItems = flattenNav(userConsoleNavGroups);
const adminItems = flattenNav(adminConsoleNavGroups);

function runUserConsoleChecks() {
  assertSetEqual(
    userConsoleNavGroups.map((group) => group.label),
    ['概览', '开发接入', '费用中心'],
    'user console top-level groups',
  );

  const forbiddenUserLabels = [
    '管理后台',
    '渠道管理',
    '账号池管理',
    '模型部署',
    '盈利监控台',
    '系统设置',
    '个人设置',
  ];
  const userLabels = new Set([
    ...userConsoleNavGroups.map((group) => group.label),
    ...userItems.map((item) => item.label),
  ]);

  for (const label of forbiddenUserLabels) {
    assert(!userLabels.has(label), `user console must not show ${label}`);
  }

  for (const item of userItems) {
    assert(
      item.path.startsWith('/console'),
      `user console route must stay under /console: ${item.path}`,
    );
    assert(
      isUserConsoleShellRoute(item.path),
      `user console nav path is not covered by shell routes: ${item.path}`,
    );
  }

  assertSetEqual(
    userItems.map((item) => item.path).sort(),
    userConsoleShellRoutes
      .filter((path) => !userConsoleHiddenShellRoutes.includes(path))
      .sort(),
    'user console nav paths vs shell routes',
  );

  for (const path of userConsoleHiddenShellRoutes) {
    assert(
      userConsoleShellRoutes.includes(path),
      `hidden user console route must be covered by shell routes: ${path}`,
    );
    assert(
      !userItems.some((item) => item.path === path),
      `hidden user console route must not show in sidebar: ${path}`,
    );
  }
}

function runAdminConsoleChecks() {
  assertSetEqual(
    adminConsoleNavGroups.map((group) => group.label),
    ['运营首页', '渠道运营', '模型与路由', '商业运营', '用户运营', '系统治理'],
    'admin console top-level groups',
  );

  const forbiddenAdminLabels = [
    '数据看板',
    '操练场',
    '令牌管理',
    '账户充值',
    '套餐订阅',
    '邀请有奖',
    '个人设置',
  ];
  const adminLabels = new Set([
    ...adminConsoleNavGroups.map((group) => group.label),
    ...adminItems.map((item) => item.label),
  ]);

  for (const label of forbiddenAdminLabels) {
    assert(!adminLabels.has(label), `admin console must not show ${label}`);
  }

  for (const item of adminItems) {
    assert(
      item.path.startsWith('/admin'),
      `admin console route must stay under /admin: ${item.path}`,
    );
    assert(
      Boolean(getAdminRoutePermission(item.path)),
      `admin console nav path has no permission mapping: ${item.path}`,
    );
  }

  const aliasPairs = adminItems.flatMap((item) =>
    (item.aliases || []).map((alias) => `${alias}->${item.path}`),
  );
  const redirectPairs = legacyAdminRedirects.map(
    ({ from, to }) => `${from}->${to}`,
  );

  assertSetEqual(
    redirectPairs.sort(),
    aliasPairs.sort(),
    'legacy admin aliases vs redirect config',
  );
}

function main() {
  runUserConsoleChecks();
  runAdminConsoleChecks();
  console.log('Console migration verification passed.');
}

main();
