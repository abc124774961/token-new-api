/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export const ADMIN_LEGACY_ROLE = 10;
export const ADMIN_ROOT_ROLE = 100;

export const ADMIN_PERMISSION_KEYS = {
  operationsOverviewRead: 'admin:operations:overview:read',
  operationsRuntimeRead: 'admin:operations:runtime:read',
  operationsAlertsRead: 'admin:operations:alerts:read',
  channelChannelRead: 'admin:channel:channel:read',
  channelChannelUpdate: 'admin:channel:channel:update',
  channelAccountRead: 'admin:channel:account:read',
  channelBalanceRead: 'admin:channel:balance:read',
  channelHealthRead: 'admin:channel:health:read',
  channelProxyRead: 'admin:channel:proxy:read',
  channelProxyUpdate: 'admin:channel:proxy:update',
  modelGatewayRead: 'admin:model:gateway:read',
  modelGatewayUpdate: 'admin:model:gateway:update',
  modelModelRead: 'admin:model:model:read',
  modelModelUpdate: 'admin:model:model:update',
  modelDeploymentRead: 'admin:model:deployment:read',
  modelDeploymentUpdate: 'admin:model:deployment:update',
  modelRoutePolicyRead: 'admin:model:route_policy:read',
  modelRatioRead: 'admin:model:ratio:read',
  commercialProfitRead: 'admin:commercial:profit:read',
  commercialProfitUpdate: 'admin:commercial:profit:update',
  commercialSubscriptionRead: 'admin:commercial:subscription:read',
  commercialSubscriptionUpdate: 'admin:commercial:subscription:update',
  commercialRedemptionRead: 'admin:commercial:redemption:read',
  commercialRedemptionUpdate: 'admin:commercial:redemption:update',
  commercialSettlementRead: 'admin:commercial:settlement:read',
  commercialSettlementComplete: 'admin:commercial:settlement:complete',
  commercialConsumptionRead: 'admin:commercial:consumption:read',
  userUserRead: 'admin:user:user:read',
  userUserDanger: 'admin:user:user:danger',
  userSegmentRead: 'admin:user:segment:read',
  userRiskRead: 'admin:user:risk:read',
  userRebateRead: 'admin:user:rebate:read',
  systemSettingsRead: 'admin:system:settings:read',
  systemSettingsUpdate: 'admin:system:settings:update',
  systemRolesRead: 'admin:system:roles:read',
  systemRolesUpdate: 'admin:system:roles:update',
  systemAuditRead: 'admin:system:audit:read',
  systemTaskRead: 'admin:system:task:read',
  channelChannelDanger: 'admin:channel:channel:danger',
  channelAccountDanger: 'admin:channel:account:danger',
  channelHealthExecute: 'admin:channel:health:execute',
  modelRoutePolicyDanger: 'admin:model:route_policy:danger',
  modelRatioUpdate: 'admin:model:ratio:update',
  systemPerformanceDanger: 'admin:system:performance:danger',
};

export const adminTargetRoleTemplates = [
  {
    key: 'operations_admin',
    name: '运营管理员',
    code: 'Operations',
    description: '查看运营首页、用户运营、风险记录和结算处理状态。',
    domains: ['运营首页', '用户运营'],
  },
  {
    key: 'channel_admin',
    name: '渠道管理员',
    code: 'Channel',
    description: '维护渠道、账号池、余额监控、健康检测和代理配置。',
    domains: ['渠道运营'],
  },
  {
    key: 'model_admin',
    name: '模型管理员',
    code: 'Model',
    description: '维护智能网关、模型、部署、路由策略和倍率配置。',
    domains: ['模型与路由'],
  },
  {
    key: 'commercial_admin',
    name: '财务管理员',
    code: 'Finance',
    description: '查看盈利、订阅、兑换码、结算记录和消费明细。',
    domains: ['商业运营'],
  },
  {
    key: 'root',
    name: '超级管理员',
    code: 'Root',
    description: '负责系统治理、全局配置、审计日志和高风险操作兜底。',
    domains: ['系统治理'],
  },
];

export const adminMenuPermissions = [
  {
    group: '运营首页',
    label: '经营总览',
    path: '/admin/overview',
    permission: ADMIN_PERMISSION_KEYS.operationsOverviewRead,
    defaultRole: '运营管理员',
    priority: 'P0',
  },
  {
    group: '运营首页',
    label: '实时监控',
    path: '/admin/realtime-monitor',
    permission: ADMIN_PERMISSION_KEYS.operationsRuntimeRead,
    defaultRole: '运营管理员',
    priority: 'P0',
  },
  {
    group: '运营首页',
    label: '渠道预警',
    path: '/admin/channel-alerts',
    permission: ADMIN_PERMISSION_KEYS.operationsAlertsRead,
    defaultRole: '运营管理员',
    priority: 'P0',
  },
  {
    group: '渠道运营',
    label: '渠道管理',
    path: '/admin/channels',
    permission: ADMIN_PERMISSION_KEYS.channelChannelRead,
    defaultRole: '渠道管理员',
    priority: 'P0',
  },
  {
    group: '渠道运营',
    label: '账号池管理',
    path: '/admin/channel-accounts',
    permission: ADMIN_PERMISSION_KEYS.channelAccountRead,
    defaultRole: '渠道管理员',
    priority: 'P0',
  },
  {
    group: '渠道运营',
    label: '渠道余额监控',
    path: '/admin/channel-balance-monitor',
    permission: ADMIN_PERMISSION_KEYS.channelBalanceRead,
    defaultRole: '渠道管理员',
    priority: 'P0',
  },
  {
    group: '渠道运营',
    label: '渠道健康检测',
    path: '/admin/channel-health-check',
    permission: ADMIN_PERMISSION_KEYS.channelHealthRead,
    defaultRole: '渠道管理员',
    priority: 'P0',
  },
  {
    group: '渠道运营',
    label: '代理管理',
    path: '/admin/channel-proxies',
    permission: ADMIN_PERMISSION_KEYS.channelProxyRead,
    defaultRole: '渠道管理员',
    priority: 'P1',
  },
  {
    group: '模型与路由',
    label: '智能模型网关',
    path: '/admin/model-gateway',
    permission: ADMIN_PERMISSION_KEYS.modelGatewayRead,
    defaultRole: '模型管理员',
    priority: 'P0',
  },
  {
    group: '模型与路由',
    label: '智能调度',
    path: '/admin/smart-scheduler',
    permission: ADMIN_PERMISSION_KEYS.modelRoutePolicyRead,
    defaultRole: '模型管理员',
    priority: 'P0',
  },
  {
    group: '模型与路由',
    label: '模型管理',
    path: '/admin/models',
    permission: ADMIN_PERMISSION_KEYS.modelModelRead,
    defaultRole: '模型管理员',
    priority: 'P0',
  },
  {
    group: '模型与路由',
    label: '模型部署',
    path: '/admin/deployment',
    permission: ADMIN_PERMISSION_KEYS.modelDeploymentRead,
    defaultRole: '模型管理员',
    priority: 'P0',
  },
  {
    group: '模型与路由',
    label: '倍率配置',
    path: '/admin/ratio-config',
    permission: ADMIN_PERMISSION_KEYS.modelRatioRead,
    defaultRole: '超级管理员',
    legacyMinRole: ADMIN_ROOT_ROLE,
    priority: 'P0',
  },
  {
    group: '商业运营',
    label: '盈利监控台',
    path: '/admin/profit-monitor',
    permission: ADMIN_PERMISSION_KEYS.commercialProfitRead,
    defaultRole: '财务管理员',
    priority: 'P0',
  },
  {
    group: '商业运营',
    label: '订阅管理',
    path: '/admin/subscription',
    permission: ADMIN_PERMISSION_KEYS.commercialSubscriptionRead,
    defaultRole: '财务管理员',
    priority: 'P1',
  },
  {
    group: '商业运营',
    label: '兑换码管理',
    path: '/admin/redemption',
    permission: ADMIN_PERMISSION_KEYS.commercialRedemptionRead,
    defaultRole: '财务管理员',
    priority: 'P1',
  },
  {
    group: '商业运营',
    label: '结算记录',
    path: '/admin/settlements',
    permission: ADMIN_PERMISSION_KEYS.commercialSettlementRead,
    defaultRole: '财务管理员',
    priority: 'P0',
  },
  {
    group: '商业运营',
    label: '消费明细',
    path: '/admin/consumption',
    permission: ADMIN_PERMISSION_KEYS.commercialConsumptionRead,
    defaultRole: '财务管理员',
    priority: 'P0',
  },
  {
    group: '用户运营',
    label: '用户管理',
    path: '/admin/users',
    permission: ADMIN_PERMISSION_KEYS.userUserRead,
    defaultRole: '运营管理员',
    priority: 'P0',
  },
  {
    group: '用户运营',
    label: '用户分层',
    path: '/admin/user-segments',
    permission: ADMIN_PERMISSION_KEYS.userSegmentRead,
    defaultRole: '运营管理员',
    priority: 'P1',
  },
  {
    group: '用户运营',
    label: '风控记录',
    path: '/admin/risk-records',
    permission: ADMIN_PERMISSION_KEYS.userRiskRead,
    defaultRole: '运营管理员',
    priority: 'P0',
  },
  {
    group: '用户运营',
    label: '邀请返佣',
    path: '/admin/invite-rebates',
    permission: ADMIN_PERMISSION_KEYS.userRebateRead,
    defaultRole: '运营管理员',
    priority: 'P1',
  },
  {
    group: '系统治理',
    label: '系统设置',
    path: '/admin/settings',
    permission: ADMIN_PERMISSION_KEYS.systemSettingsRead,
    defaultRole: '超级管理员',
    legacyMinRole: ADMIN_ROOT_ROLE,
    priority: 'P0',
  },
  {
    group: '系统治理',
    label: '权限角色',
    path: '/admin/roles',
    permission: ADMIN_PERMISSION_KEYS.systemRolesRead,
    defaultRole: '管理员',
    priority: 'P0',
  },
  {
    group: '系统治理',
    label: '审计日志',
    path: '/admin/audit-logs',
    permission: ADMIN_PERMISSION_KEYS.systemAuditRead,
    defaultRole: '超级管理员',
    legacyMinRole: ADMIN_ROOT_ROLE,
    priority: 'P0',
  },
  {
    group: '系统治理',
    label: '后台任务',
    path: '/admin/background-tasks',
    permission: ADMIN_PERMISSION_KEYS.systemTaskRead,
    defaultRole: '超级管理员',
    legacyMinRole: ADMIN_ROOT_ROLE,
    priority: 'P1',
  },
].map((item) => ({
  legacyMinRole: ADMIN_LEGACY_ROLE,
  ...item,
}));

export const adminMenuPermissionByPath = Object.fromEntries(
  adminMenuPermissions.map((item) => [item.path, item]),
);

export const adminMenuPermissionByKey = Object.fromEntries(
  adminMenuPermissions.map((item) => [item.permission, item]),
);

export const adminDangerousOperationPermissions = [
  {
    page: '渠道管理',
    operation: '删除渠道、批量删除、成本重算',
    permission: ADMIN_PERMISSION_KEYS.channelChannelDanger,
    defaultRole: '渠道管理员',
    confirmation: '必须',
    priority: 'P0',
  },
  {
    page: '账号池管理',
    operation: '批量启停、导入、归档、恢复、代理绑定、凭证替换和删除记录',
    permission: ADMIN_PERMISSION_KEYS.channelAccountDanger,
    defaultRole: '渠道管理员',
    confirmation: '必须',
    priority: 'P0',
  },
  {
    page: '渠道健康检测',
    operation: '立即探活、恢复健康、清理熔断',
    permission: ADMIN_PERMISSION_KEYS.channelHealthExecute,
    defaultRole: '渠道管理员',
    confirmation: '建议',
    priority: 'P0',
  },
  {
    page: '智能调度',
    operation: '保存调度配置、恢复默认、保存上游错误分类规则',
    permission: ADMIN_PERMISSION_KEYS.modelRoutePolicyDanger,
    defaultRole: '模型管理员',
    confirmation: '必须',
    priority: 'P0',
  },
  {
    page: '倍率配置',
    operation: '保存/重置模型倍率、分组倍率、上游价格同步和工具价格',
    permission: ADMIN_PERMISSION_KEYS.modelRatioUpdate,
    defaultRole: '超级管理员',
    confirmation: '必须',
    legacyMinRole: ADMIN_ROOT_ROLE,
    priority: 'P0',
  },
  {
    page: '结算记录',
    operation: '人工补单',
    permission: ADMIN_PERMISSION_KEYS.commercialSettlementComplete,
    defaultRole: '财务管理员',
    confirmation: '必须',
    priority: 'P0',
  },
  {
    page: '用户管理',
    operation: '禁用、重置安全、变更角色',
    permission: ADMIN_PERMISSION_KEYS.userUserDanger,
    defaultRole: '运营管理员',
    confirmation: '必须',
    priority: 'P0',
  },
  {
    page: '系统设置',
    operation: '保存支付、OAuth、SMTP、限流、性能设置',
    permission: ADMIN_PERMISSION_KEYS.systemSettingsUpdate,
    defaultRole: '超级管理员',
    confirmation: '必须',
    legacyMinRole: ADMIN_ROOT_ROLE,
    priority: 'P0',
  },
  {
    page: '权限角色',
    operation: '创建、更新、禁用角色和分配用户权限',
    permission: ADMIN_PERMISSION_KEYS.systemRolesUpdate,
    defaultRole: '超级管理员',
    confirmation: '必须',
    legacyMinRole: ADMIN_ROOT_ROLE,
    priority: 'P0',
  },
  {
    page: '性能设置',
    operation: '清理缓存、重置统计、触发 GC、删除日志',
    permission: ADMIN_PERMISSION_KEYS.systemPerformanceDanger,
    defaultRole: '超级管理员',
    confirmation: '必须',
    legacyMinRole: ADMIN_ROOT_ROLE,
    priority: 'P1',
  },
].map((item) => ({
  legacyMinRole: ADMIN_LEGACY_ROLE,
  ...item,
}));

export const adminDangerousOperationPermissionByKey = Object.fromEntries(
  adminDangerousOperationPermissions.map((item) => [item.permission, item]),
);

export const adminOperationPermissions = [
  {
    group: '渠道运营',
    operation: '编辑渠道配置、恢复熔断和恢复健康状态',
    permission: ADMIN_PERMISSION_KEYS.channelChannelUpdate,
    defaultRole: '渠道管理员',
    priority: 'P0',
  },
  {
    group: '渠道运营',
    operation: '创建、编辑和启停代理配置',
    permission: ADMIN_PERMISSION_KEYS.channelProxyUpdate,
    defaultRole: '渠道管理员',
    priority: 'P1',
  },
  {
    group: '模型与路由',
    operation: '保存智能网关调度和观测配置',
    permission: ADMIN_PERMISSION_KEYS.modelGatewayUpdate,
    defaultRole: '模型管理员',
    priority: 'P0',
  },
  {
    group: '模型与路由',
    operation: '新增、编辑和同步模型配置',
    permission: ADMIN_PERMISSION_KEYS.modelModelUpdate,
    defaultRole: '模型管理员',
    priority: 'P0',
  },
  {
    group: '模型与路由',
    operation: '新增、编辑和发布模型部署',
    permission: ADMIN_PERMISSION_KEYS.modelDeploymentUpdate,
    defaultRole: '模型管理员',
    priority: 'P0',
  },
  {
    group: '商业运营',
    operation: '保存盈利监控和动态倍率建议',
    permission: ADMIN_PERMISSION_KEYS.commercialProfitUpdate,
    defaultRole: '财务管理员',
    priority: 'P0',
  },
  {
    group: '商业运营',
    operation: '新增、编辑和上下架订阅方案',
    permission: ADMIN_PERMISSION_KEYS.commercialSubscriptionUpdate,
    defaultRole: '财务管理员',
    priority: 'P1',
  },
  {
    group: '商业运营',
    operation: '新增、编辑和批量生成兑换码',
    permission: ADMIN_PERMISSION_KEYS.commercialRedemptionUpdate,
    defaultRole: '财务管理员',
    priority: 'P1',
  },
].map((item) => ({
  legacyMinRole: ADMIN_LEGACY_ROLE,
  ...item,
}));

export const adminOperationPermissionByKey = Object.fromEntries(
  adminOperationPermissions.map((item) => [item.permission, item]),
);

export const adminRoutePermissionPatterns = [
  {
    pattern: /^\/admin\/route-policy$/,
    permission: ADMIN_PERMISSION_KEYS.modelRoutePolicyRead,
  },
  {
    pattern: /^\/admin\/channel\/[^/]+\/accounts$/,
    permission: ADMIN_PERMISSION_KEYS.channelAccountRead,
  },
];

export function getAdminRoutePermission(pathname) {
  if (adminMenuPermissionByPath[pathname]) {
    return adminMenuPermissionByPath[pathname].permission;
  }

  return adminRoutePermissionPatterns.find((item) =>
    item.pattern.test(pathname),
  )?.permission;
}

function parsePermissionValue(value) {
  if (!value) return [];
  if (Array.isArray(value)) return value.filter(Boolean);
  if (typeof value === 'string') {
    const trimmed = value.trim();
    if (!trimmed) return [];
    if (trimmed.startsWith('[')) {
      try {
        const parsed = JSON.parse(trimmed);
        return Array.isArray(parsed) ? parsed.filter(Boolean) : [];
      } catch (error) {
        return [];
      }
    }
    return trimmed
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

export function getAdminUserPermissionList(user) {
  return [
    ...parsePermissionValue(
      Array.isArray(user?.permissions) || typeof user?.permissions === 'string'
        ? user?.permissions
        : undefined,
    ),
    ...parsePermissionValue(user?.admin_permissions),
    ...parsePermissionValue(user?.adminPermissions),
  ];
}

export function hasAdminPermissionSource(user) {
  const legacyPermissions = user?.permissions;
  return (
    user &&
    (Object.prototype.hasOwnProperty.call(user, 'admin_permissions') ||
      Array.isArray(legacyPermissions) ||
      typeof legacyPermissions === 'string' ||
      Object.prototype.hasOwnProperty.call(user, 'adminPermissions'))
  );
}

function matchesPermissionGrant(grant, permission) {
  if (!grant || !permission) return false;
  if (grant === '*' || grant === permission) return true;

  const permissionParts = permission.split(':');
  const wildcardGrants = [
    `${permissionParts[0]}:*`,
    `${permissionParts[0]}:${permissionParts[1]}:*`,
    `${permissionParts[0]}:${permissionParts[1]}:${permissionParts[2]}:*`,
  ];

  return wildcardGrants.includes(grant);
}

export function hasAdminPermission(user, permission, options = {}) {
  const role = Number(user?.role || 0);
  if (role >= ADMIN_ROOT_ROLE) return true;
  if (!permission) return role >= ADMIN_LEGACY_ROLE;

  const permissionConfig = adminMenuPermissionByKey[permission];
  const dangerousOperationConfig =
    adminDangerousOperationPermissionByKey[permission];
  const operationConfig = adminOperationPermissionByKey[permission];
  const legacyMinRole =
    options.legacyMinRole ??
    permissionConfig?.legacyMinRole ??
    dangerousOperationConfig?.legacyMinRole ??
    operationConfig?.legacyMinRole ??
    ADMIN_LEGACY_ROLE;

  if (role < legacyMinRole) return false;

  if (hasAdminPermissionSource(user)) {
    return getAdminUserPermissionList(user).some((grant) =>
      matchesPermissionGrant(grant, permission),
    );
  }

  return role >= legacyMinRole;
}
