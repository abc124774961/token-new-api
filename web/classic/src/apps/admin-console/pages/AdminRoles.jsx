import React, {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import {
  AlertTriangle,
  CheckCircle2,
  CircleDollarSign,
  Crown,
  Database,
  KeyRound,
  Lock,
  Network,
  Plus,
  RefreshCw,
  Save,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  UserCheck,
  Users,
} from 'lucide-react';
import {
  Button,
  Checkbox,
  Empty,
  Input,
  InputNumber,
  Modal,
  Select,
  Spin,
  TextArea,
  Toast,
} from '@douyinfe/semi-ui';
import { API, refreshStoredAdminPermissionData } from '../../../helpers';
import { UserContext } from '../../../context/User';
import {
  AdminPermissionButton,
  useAdminActionPermission,
} from '../permissions/AdminPermissionAction';
import {
  ADMIN_LEGACY_ROLE,
  ADMIN_PERMISSION_KEYS,
  ADMIN_ROOT_ROLE,
  adminDangerousOperationPermissions,
  adminMenuPermissions,
  adminOperationPermissions,
  adminTargetRoleTemplates,
} from '../permissions/adminPermissions.config';

const roleIconMap = {
  operations_admin: Users,
  channel_admin: Network,
  model_admin: SlidersHorizontal,
  commercial_admin: CircleDollarSign,
  root: Crown,
};

const roleToneMap = {
  operations_admin: 'is-info',
  channel_admin: 'is-success',
  model_admin: 'is-warning',
  commercial_admin: 'is-info',
  root: 'is-danger',
};

const permissionSourceTone = {
  role_compatibility: 'is-warning',
  database: 'is-success',
  root: 'is-danger',
};

const permissionSourceLabel = {
  role_compatibility: '固定角色兼容',
  database: '数据库权限',
  root: '超级管理员',
};

const ROLE_STATUS_ENABLED = 1;
const ROLE_STATUS_DISABLED = 2;

const localCatalog = {
  role_templates: adminTargetRoleTemplates,
  menu_permissions: adminMenuPermissions,
  dangerous_operation_permissions: adminDangerousOperationPermissions,
  operation_permissions: adminOperationPermissions,
};

function normalizeMenuPermission(item) {
  return {
    group: item.group,
    label: item.label,
    path: item.path,
    permission: item.permission,
    defaultRole: item.default_role ?? item.defaultRole,
    legacyMinRole: item.legacy_min_role ?? item.legacyMinRole,
    priority: item.priority,
    type: 'menu',
  };
}

function normalizeDangerousPermission(item) {
  return {
    group: '危险操作',
    page: item.page,
    label: item.operation,
    operation: item.operation,
    permission: item.permission,
    defaultRole: item.default_role ?? item.defaultRole,
    confirmation: item.confirmation,
    legacyMinRole: item.legacy_min_role ?? item.legacyMinRole,
    priority: item.priority,
    type: 'danger',
  };
}

function normalizeOperationPermission(item) {
  return {
    group: '常规操作',
    page: item.group,
    label: item.operation,
    operation: item.operation,
    permission: item.permission,
    defaultRole: item.default_role ?? item.defaultRole,
    legacyMinRole: item.legacy_min_role ?? item.legacyMinRole,
    priority: item.priority,
    type: 'operation',
  };
}

function normalizeRole(role) {
  return {
    id: Number(role.id || 0),
    key: role.key || '',
    name: role.name || '',
    code: role.code || '',
    description: role.description || '',
    status: Number(role.status || ROLE_STATUS_ENABLED),
    builtin: Number(role.builtin || 0),
    sort_order: Number(role.sort_order || role.sortOrder || 0),
    permissions: Array.isArray(role.permissions) ? role.permissions : [],
  };
}

function permissionTextToList(value) {
  return String(value || '')
    .split(/[\n,，\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function permissionListToText(value) {
  return Array.isArray(value) ? value.join('\n') : '';
}

function dedupe(values) {
  return [...new Set((values || []).filter(Boolean))].sort();
}

function buildTemplatePermissions(template, allPermissions) {
  if (!template) {
    return [];
  }
  if (template.key === 'root') {
    return allPermissions.map((item) => item.permission);
  }
  return allPermissions
    .filter((item) => item.defaultRole === template.name)
    .map((item) => item.permission);
}

function createDraftFromRole(role) {
  const normalized = normalizeRole(role || {});
  return {
    ...normalized,
    permissions: dedupe(normalized.permissions),
  };
}

function createDraftFromTemplate(template, allPermissions, index = 0) {
  return {
    id: 0,
    key: template.key,
    name: template.name,
    code: template.code,
    description: template.description,
    status: ROLE_STATUS_ENABLED,
    builtin: 1,
    sort_order: index * 10,
    permissions: dedupe(buildTemplatePermissions(template, allPermissions)),
  };
}

function groupPermissions(permissions) {
  const groups = new Map();
  for (const item of permissions) {
    const group =
      item.type === 'danger' || item.type === 'operation'
        ? `${item.group} / ${item.page}`
        : item.group;
    if (!groups.has(group)) {
      groups.set(group, []);
    }
    groups.get(group).push(item);
  }
  return [...groups.entries()].map(([group, items]) => ({
    group,
    items,
  }));
}

function buildPayloadFromDraft(draft) {
  return {
    key: draft.key.trim(),
    name: draft.name.trim(),
    code: draft.code.trim(),
    description: draft.description.trim(),
    status: Number(draft.status || ROLE_STATUS_ENABLED),
    sort_order: Number(draft.sort_order || 0),
    permissions: dedupe(draft.permissions),
  };
}

const AdminRoles = () => {
  const { t } = useTranslation();
  const [, userDispatch] = useContext(UserContext);
  const canUpdateRoles = useAdminActionPermission(
    ADMIN_PERMISSION_KEYS.systemRolesUpdate,
  );
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [catalog, setCatalog] = useState(localCatalog);
  const [roles, setRoles] = useState([]);
  const [selectedRoleKey, setSelectedRoleKey] = useState('');
  const [draft, setDraft] = useState(null);
  const [dataSource, setDataSource] = useState('local');
  const [error, setError] = useState('');
  const [assignmentUserId, setAssignmentUserId] = useState(null);
  const [assignment, setAssignment] = useState(null);
  const [assignmentLoading, setAssignmentLoading] = useState(false);
  const [assignmentSaving, setAssignmentSaving] = useState(false);

  const roleTemplates = useMemo(
    () =>
      Array.isArray(catalog.role_templates)
        ? catalog.role_templates
        : localCatalog.role_templates,
    [catalog],
  );

  const permissionRows = useMemo(
    () =>
      (Array.isArray(catalog.menu_permissions)
        ? catalog.menu_permissions
        : localCatalog.menu_permissions
      ).map(normalizeMenuPermission),
    [catalog],
  );

  const dangerousRows = useMemo(
    () =>
      (Array.isArray(catalog.dangerous_operation_permissions)
        ? catalog.dangerous_operation_permissions
        : localCatalog.dangerous_operation_permissions
      ).map(normalizeDangerousPermission),
    [catalog],
  );

  const operationRows = useMemo(
    () =>
      (Array.isArray(catalog.operation_permissions)
        ? catalog.operation_permissions
        : localCatalog.operation_permissions
      ).map(normalizeOperationPermission),
    [catalog],
  );

  const allPermissions = useMemo(() => {
    const byKey = new Map();
    for (const item of [
      ...permissionRows,
      ...dangerousRows,
      ...operationRows,
    ]) {
      byKey.set(item.permission, item);
    }
    return [...byKey.values()].sort((a, b) =>
      `${a.group}-${a.label}`.localeCompare(`${b.group}-${b.label}`),
    );
  }, [dangerousRows, operationRows, permissionRows]);

  const permissionGroups = useMemo(
    () => groupPermissions(allPermissions),
    [allPermissions],
  );

  const roleByKey = useMemo(
    () => Object.fromEntries(roles.map((role) => [role.key, role])),
    [roles],
  );

  const roleOptions = useMemo(
    () =>
      roles
        .filter((role) => role.status === ROLE_STATUS_ENABLED)
        .map((role) => ({
          label: `${role.name} (${role.code || role.key})`,
          value: role.id,
        })),
    [roles],
  );

  const roleMetrics = useMemo(() => {
    const storedTemplateCount = roleTemplates.filter(
      (template) => roleByKey[template.key],
    ).length;
    const effectiveRoleCount = roles.filter(
      (role) => role.status === ROLE_STATUS_ENABLED,
    ).length;
    return {
      storedTemplateCount,
      effectiveRoleCount,
      permissionCount: allPermissions.length,
      dangerCount: dangerousRows.length,
    };
  }, [
    allPermissions.length,
    dangerousRows.length,
    roleByKey,
    roleTemplates,
    roles,
  ]);

  const selectRole = useCallback(
    (key, nextRoles = roles) => {
      const storedRole = nextRoles.find((role) => role.key === key);
      const template = roleTemplates.find((item) => item.key === key);
      const nextDraft = storedRole
        ? createDraftFromRole(storedRole)
        : createDraftFromTemplate(
            template || roleTemplates[0],
            allPermissions,
            Math.max(
              roleTemplates.findIndex((item) => item.key === key),
              0,
            ),
          );
      setSelectedRoleKey(nextDraft.key);
      setDraft(nextDraft);
    },
    [allPermissions, roleTemplates, roles],
  );

  const loadData = useCallback(
    async (preferredRoleKey) => {
      setLoading(true);
      setError('');
      try {
        const [configResponse, rolesResponse] = await Promise.all([
          API.get('/api/admin/permissions/config', {
            disableDuplicate: true,
            skipErrorHandler: true,
          }),
          API.get('/api/admin/permissions/roles', {
            disableDuplicate: true,
            skipErrorHandler: true,
          }),
        ]);
        if (configResponse?.data?.success === false) {
          throw new Error(configResponse?.data?.message || t('加载失败'));
        }
        const configData = configResponse?.data?.data || {};
        const roleData = rolesResponse?.data?.success
          ? rolesResponse?.data?.data
          : configData;
        const nextRoles = Array.isArray(roleData?.roles)
          ? roleData.roles.map(normalizeRole)
          : Array.isArray(configData?.stored_roles)
            ? configData.stored_roles.map(normalizeRole)
            : [];
        setCatalog({
          role_templates:
            configData.role_templates || localCatalog.role_templates,
          menu_permissions:
            configData.menu_permissions || localCatalog.menu_permissions,
          dangerous_operation_permissions:
            configData.dangerous_operation_permissions ||
            localCatalog.dangerous_operation_permissions,
          operation_permissions:
            configData.operation_permissions ||
            localCatalog.operation_permissions,
        });
        setRoles(nextRoles);
        setDataSource('api');
        const key =
          preferredRoleKey ||
          selectedRoleKey ||
          nextRoles[0]?.key ||
          configData.role_templates?.[0]?.key ||
          localCatalog.role_templates[0]?.key;
        if (key) {
          selectRole(key, nextRoles);
        }
      } catch (err) {
        setCatalog(localCatalog);
        setRoles([]);
        setDataSource('local');
        setError(err?.response?.data?.message || err?.message || t('加载失败'));
        const fallbackKey =
          preferredRoleKey ||
          selectedRoleKey ||
          localCatalog.role_templates[0]?.key;
        if (fallbackKey) {
          selectRole(fallbackKey, []);
        }
      } finally {
        setLoading(false);
      }
    },
    [selectRole, selectedRoleKey, t],
  );

  useEffect(() => {
    loadData();
  }, []);

  useEffect(() => {
    if (!draft && roleTemplates.length > 0) {
      selectRole(roleTemplates[0].key);
    }
  }, [draft, roleTemplates, selectRole]);

  const updateDraft = (patch) => {
    setDraft((prev) => ({ ...prev, ...patch }));
  };

  const togglePermission = (permission, checked) => {
    setDraft((prev) => {
      if (!prev) return prev;
      const next = checked
        ? dedupe([...prev.permissions, permission])
        : prev.permissions.filter((item) => item !== permission);
      return { ...prev, permissions: next };
    });
  };

  const setGroupPermissions = (items, checked) => {
    const permissionKeys = items.map((item) => item.permission);
    setDraft((prev) => {
      if (!prev) return prev;
      const next = checked
        ? dedupe([...prev.permissions, ...permissionKeys])
        : prev.permissions.filter((item) => !permissionKeys.includes(item));
      return { ...prev, permissions: next };
    });
  };

  const resetDraftFromTemplate = () => {
    const template = roleTemplates.find((item) => item.key === selectedRoleKey);
    if (!template) {
      return;
    }
    const index = roleTemplates.findIndex(
      (item) => item.key === selectedRoleKey,
    );
    setDraft((prev) => ({
      ...createDraftFromTemplate(template, allPermissions, index),
      id: prev?.id || 0,
      status: prev?.status || ROLE_STATUS_ENABLED,
    }));
  };

  const saveRole = async () => {
    if (!draft) {
      return;
    }
    const payload = buildPayloadFromDraft(draft);
    if (!payload.key || !payload.name) {
      Toast.error({ content: t('角色 key 和名称不能为空') });
      return;
    }
    setSaving(true);
    try {
      const response = draft.id
        ? await API.put(`/api/admin/permissions/roles/${draft.id}`, payload, {
            skipErrorHandler: true,
          })
        : await API.post('/api/admin/permissions/roles', payload, {
            skipErrorHandler: true,
          });
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('保存失败，请重试'));
      }
      Toast.success({ content: t('保存成功') });
      const saved = response?.data?.data;
      await refreshStoredAdminPermissionData(userDispatch);
      await loadData(saved?.key || draft.key);
    } catch (err) {
      Toast.error({
        content:
          err?.response?.data?.message || err?.message || t('保存失败，请重试'),
      });
    } finally {
      setSaving(false);
    }
  };

  const syncRoleTemplates = async () => {
    setSyncing(true);
    try {
      const response = await API.post(
        '/api/admin/permissions/roles/sync-templates',
        {},
        { skipErrorHandler: true },
      );
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('同步失败，请重试'));
      }
      Toast.success({ content: t('内置角色已同步') });
      await refreshStoredAdminPermissionData(userDispatch);
      await loadData(selectedRoleKey);
    } catch (err) {
      Toast.error({
        content:
          err?.response?.data?.message || err?.message || t('同步失败，请重试'),
      });
    } finally {
      setSyncing(false);
    }
  };

  const disableRole = () => {
    if (!draft?.id) {
      return;
    }
    Modal.confirm({
      title: t('确认禁用角色'),
      content: t('禁用后不会删除历史绑定，但该角色不再参与新的权限解析。'),
      onOk: async () => {
        try {
          const response = await API.delete(
            `/api/admin/permissions/roles/${draft.id}`,
            { skipErrorHandler: true },
          );
          if (response?.data?.success === false) {
            throw new Error(response?.data?.message || t('操作失败'));
          }
          Toast.success({ content: t('角色已禁用') });
          await refreshStoredAdminPermissionData(userDispatch);
          await loadData(roleTemplates[0]?.key);
        } catch (err) {
          Toast.error({
            content:
              err?.response?.data?.message || err?.message || t('操作失败'),
          });
        }
      },
    });
  };

  const loadAssignment = async () => {
    const userId = Number(assignmentUserId || 0);
    if (!userId) {
      Toast.warning({ content: t('请输入用户 ID') });
      return;
    }
    setAssignmentLoading(true);
    try {
      const response = await API.get(`/api/admin/permissions/users/${userId}`, {
        disableDuplicate: true,
        skipErrorHandler: true,
      });
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('加载失败'));
      }
      const payload = response?.data?.data || {};
      setAssignment({
        user_id: payload.user_id || userId,
        source: payload.source || 'role_compatibility',
        role_ids: Array.isArray(payload.role_ids) ? payload.role_ids : [],
        allow_permissions: Array.isArray(payload.allow_permissions)
          ? payload.allow_permissions
          : [],
        deny_permissions: Array.isArray(payload.deny_permissions)
          ? payload.deny_permissions
          : [],
        effective_permissions: Array.isArray(payload.effective_permissions)
          ? payload.effective_permissions
          : [],
      });
    } catch (err) {
      Toast.error({
        content: err?.response?.data?.message || err?.message || t('加载失败'),
      });
      setAssignment(null);
    } finally {
      setAssignmentLoading(false);
    }
  };

  const saveAssignment = async () => {
    if (!assignment?.user_id) {
      return;
    }
    const allowText =
      assignment.allowText ??
      permissionListToText(assignment.allow_permissions);
    const denyText =
      assignment.denyText ?? permissionListToText(assignment.deny_permissions);
    setAssignmentSaving(true);
    try {
      const response = await API.put(
        `/api/admin/permissions/users/${assignment.user_id}`,
        {
          role_ids: dedupe(assignment.role_ids).map(Number),
          allow_permissions: permissionTextToList(allowText),
          deny_permissions: permissionTextToList(denyText),
        },
        { skipErrorHandler: true },
      );
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('保存失败，请重试'));
      }
      Toast.success({ content: t('用户权限已保存') });
      await refreshStoredAdminPermissionData(userDispatch);
      await loadAssignment();
    } catch (err) {
      Toast.error({
        content:
          err?.response?.data?.message || err?.message || t('保存失败，请重试'),
      });
    } finally {
      setAssignmentSaving(false);
    }
  };

  const assignmentDraft = useMemo(() => {
    if (!assignment) {
      return null;
    }
    return {
      ...assignment,
      allowText:
        assignment.allowText ??
        permissionListToText(assignment.allow_permissions),
      denyText:
        assignment.denyText ??
        permissionListToText(assignment.deny_permissions),
    };
  }, [assignment]);

  const setAssignmentDraft = (patch) => {
    setAssignment((prev) => {
      const base = prev
        ? {
            ...prev,
            allowText:
              prev.allowText ?? permissionListToText(prev.allow_permissions),
            denyText:
              prev.denyText ?? permissionListToText(prev.deny_permissions),
          }
        : {};
      return { ...base, ...patch };
    });
  };

  return (
    <div className='aurora-admin-page aurora-roles-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('系统治理')}</div>
          <h1>{t('权限角色')}</h1>
          <p>
            {t(
              '配置管理员角色、权限点和用户覆盖规则，把普通管理员与超级管理员边界落到可执行的 RBAC 工作台。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>
              {t('兼容角色')} Role {ADMIN_LEGACY_ROLE} / {ADMIN_ROOT_ROLE}
            </span>
            <span>
              {t('已落库角色')} {roleMetrics.storedTemplateCount}/
              {roleTemplates.length}
            </span>
            <span>
              {t('权限点')} {roleMetrics.permissionCount}
            </span>
            <span>
              {t('危险操作')} {roleMetrics.dangerCount}
            </span>
          </div>
        </div>
        <div
          className={`aurora-overview-status ${
            dataSource === 'api'
              ? 'aurora-status-success'
              : 'aurora-status-warning'
          }`}
        >
          <span>{t('权限模式')}</span>
          <strong>
            {t(dataSource === 'api' ? '接口已接入' : '本地配置兜底')}
          </strong>
          <em>{t('角色、接口和按钮权限统一治理')}</em>
        </div>
      </section>

      {error && (
        <div className='aurora-inline-error'>
          {t('后端权限接口暂不可用，当前展示本地权限目录：')}
          {error}
        </div>
      )}

      <section className='aurora-role-workbench'>
        <div className='aurora-panel aurora-role-sidebar'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('角色模板')}</h2>
              <p>{t('按运营职责拆分后台入口，root 负责高风险兜底。')}</p>
            </div>
            <AdminPermissionButton
              dangerPermission={ADMIN_PERMISSION_KEYS.systemRolesUpdate}
              icon={<RefreshCw size={15} />}
              loading={syncing}
              onClick={syncRoleTemplates}
              size='small'
              theme='light'
              type='primary'
            >
              {t('同步模板')}
            </AdminPermissionButton>
          </div>
          <Spin spinning={loading}>
            <div className='aurora-role-stack'>
              {roleTemplates.map((role, index) => {
                const Icon = roleIconMap[role.key] || ShieldCheck;
                const tone = roleToneMap[role.key] || 'is-info';
                const stored = roleByKey[role.key];
                const permissionCount = stored
                  ? stored.permissions.length
                  : buildTemplatePermissions(role, allPermissions).length;
                const selected = selectedRoleKey === role.key;
                return (
                  <button
                    className={`aurora-role-selector ${selected ? 'is-active' : ''}`}
                    key={role.key}
                    onClick={() => selectRole(role.key)}
                    type='button'
                  >
                    <span className={`aurora-role-card-icon ${tone}`}>
                      <Icon size={20} />
                    </span>
                    <span className='aurora-role-selector-main'>
                      <strong>{t(role.name)}</strong>
                      <small>{t(role.description)}</small>
                      <em>
                        {stored ? t('已落库') : t('模板')}
                        {' · '}
                        {permissionCount}
                        {t('项权限')}
                      </em>
                    </span>
                    {stored ? (
                      <CheckCircle2
                        size={16}
                        className='aurora-role-state-ok'
                      />
                    ) : (
                      <Plus size={16} className='aurora-role-state-muted' />
                    )}
                  </button>
                );
              })}
            </div>
          </Spin>
        </div>

        <div className='aurora-panel aurora-role-editor'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('角色配置')}</h2>
              <p>
                {draft?.id
                  ? t('正在编辑数据库中的管理员角色。')
                  : t('当前为模板草稿，保存后写入数据库并参与权限解析。')}
              </p>
            </div>
            <div className='aurora-role-editor-actions'>
              <Button
                icon={<RefreshCw size={15} />}
                onClick={resetDraftFromTemplate}
                size='small'
                theme='light'
              >
                {t('恢复模板')}
              </Button>
              <AdminPermissionButton
                dangerPermission={ADMIN_PERMISSION_KEYS.systemRolesUpdate}
                disabled={!draft}
                icon={<Save size={15} />}
                loading={saving}
                onClick={saveRole}
                size='small'
                type='primary'
              >
                {draft?.id ? t('保存角色') : t('创建角色')}
              </AdminPermissionButton>
              {draft?.id ? (
                <AdminPermissionButton
                  dangerPermission={ADMIN_PERMISSION_KEYS.systemRolesUpdate}
                  icon={<Trash2 size={15} />}
                  onClick={disableRole}
                  size='small'
                  theme='light'
                  type='danger'
                >
                  {t('禁用')}
                </AdminPermissionButton>
              ) : null}
            </div>
          </div>

          {!draft ? (
            <Empty description={t('暂无角色数据')} />
          ) : (
            <>
              <div className='aurora-role-form-grid'>
                <label>
                  <span>{t('角色名称')}</span>
                  <Input
                    disabled={!canUpdateRoles}
                    value={draft.name}
                    onChange={(value) => updateDraft({ name: value })}
                  />
                </label>
                <label>
                  <span>{t('角色 Key')}</span>
                  <Input
                    disabled={!canUpdateRoles}
                    value={draft.key}
                    onChange={(value) => updateDraft({ key: value })}
                  />
                </label>
                <label>
                  <span>{t('角色代码')}</span>
                  <Input
                    disabled={!canUpdateRoles}
                    value={draft.code}
                    onChange={(value) => updateDraft({ code: value })}
                  />
                </label>
                <label>
                  <span>{t('排序')}</span>
                  <InputNumber
                    disabled={!canUpdateRoles}
                    value={draft.sort_order}
                    onChange={(value) => updateDraft({ sort_order: value })}
                  />
                </label>
                <label>
                  <span>{t('状态')}</span>
                  <Select
                    disabled={!canUpdateRoles}
                    value={draft.status}
                    onChange={(value) => updateDraft({ status: value })}
                  >
                    <Select.Option value={ROLE_STATUS_ENABLED}>
                      {t('启用')}
                    </Select.Option>
                    <Select.Option value={ROLE_STATUS_DISABLED}>
                      {t('禁用')}
                    </Select.Option>
                  </Select>
                </label>
                <label className='is-wide'>
                  <span>{t('说明')}</span>
                  <Input
                    disabled={!canUpdateRoles}
                    value={draft.description}
                    onChange={(value) => updateDraft({ description: value })}
                  />
                </label>
              </div>

              <div className='aurora-permission-editor-head'>
                <div>
                  <strong>{t('权限清单')}</strong>
                  <small>
                    {draft.permissions.length}/{allPermissions.length}{' '}
                    {t('项权限已选择')}
                  </small>
                </div>
              </div>

              <div className='aurora-permission-group-grid'>
                {permissionGroups.map((group) => {
                  const selectedCount = group.items.filter((item) =>
                    draft.permissions.includes(item.permission),
                  ).length;
                  const allChecked = selectedCount === group.items.length;
                  return (
                    <div className='aurora-permission-group' key={group.group}>
                      <div className='aurora-permission-group-head'>
                        <div>
                          <strong>{t(group.group)}</strong>
                          <small>
                            {selectedCount}/{group.items.length}
                          </small>
                        </div>
                        <Button
                          disabled={!canUpdateRoles}
                          onClick={() =>
                            setGroupPermissions(group.items, !allChecked)
                          }
                          size='small'
                          theme='borderless'
                        >
                          {allChecked ? t('清空') : t('全选')}
                        </Button>
                      </div>
                      <div className='aurora-permission-list'>
                        {group.items.map((item) => (
                          <Checkbox
                            checked={draft.permissions.includes(
                              item.permission,
                            )}
                            disabled={!canUpdateRoles}
                            key={item.permission}
                            onChange={(event) =>
                              togglePermission(
                                item.permission,
                                event.target.checked,
                              )
                            }
                          >
                            <span className='aurora-permission-check-label'>
                              <strong>{t(item.label)}</strong>
                              <small>{item.permission}</small>
                            </span>
                          </Checkbox>
                        ))}
                      </div>
                    </div>
                  );
                })}
              </div>
            </>
          )}
        </div>
      </section>

      <section className='aurora-panel aurora-user-assignment-panel'>
        <div className='aurora-section-head'>
          <div>
            <h2>{t('用户权限绑定')}</h2>
            <p>
              {t(
                '按用户 ID 查看和保存角色绑定、允许权限与拒绝权限，拒绝项优先生效。',
              )}
            </p>
          </div>
          <span
            className={`aurora-status-pill ${
              permissionSourceTone[assignmentDraft?.source] || 'is-muted'
            }`}
          >
            {t(permissionSourceLabel[assignmentDraft?.source] || '未加载')}
          </span>
        </div>
        <div className='aurora-assignment-query'>
          <InputNumber
            min={1}
            placeholder={t('用户 ID')}
            value={assignmentUserId}
            onChange={setAssignmentUserId}
          />
          <AdminPermissionButton
            dangerPermission={ADMIN_PERMISSION_KEYS.systemRolesUpdate}
            icon={<Search size={15} />}
            loading={assignmentLoading}
            onClick={loadAssignment}
            type='primary'
          >
            {t('加载用户权限')}
          </AdminPermissionButton>
        </div>

        <Spin spinning={assignmentLoading}>
          {!assignmentDraft ? (
            <div className='aurora-assignment-empty'>
              <UserCheck size={22} />
              <span>{t('输入管理员用户 ID 后加载权限绑定。')}</span>
            </div>
          ) : (
            <div className='aurora-assignment-grid'>
              <div className='aurora-assignment-card'>
                <div className='aurora-assignment-card-head'>
                  <strong>{t('绑定角色')}</strong>
                  <small>
                    {assignmentDraft.role_ids.length}/{roleOptions.length}
                  </small>
                </div>
                {roleOptions.length === 0 ? (
                  <Empty description={t('请先同步或创建角色')} />
                ) : (
                  <Checkbox.Group
                    value={assignmentDraft.role_ids}
                    onChange={(value) =>
                      setAssignmentDraft({ role_ids: value.map(Number) })
                    }
                  >
                    <div className='aurora-assignment-role-list'>
                      {roleOptions.map((role) => (
                        <Checkbox
                          disabled={!canUpdateRoles}
                          key={role.value}
                          value={role.value}
                        >
                          {role.label}
                        </Checkbox>
                      ))}
                    </div>
                  </Checkbox.Group>
                )}
              </div>
              <div className='aurora-assignment-card'>
                <div className='aurora-assignment-card-head'>
                  <strong>{t('允许权限覆盖')}</strong>
                  <small>{t('每行一个权限点')}</small>
                </div>
                <TextArea
                  autosize={{ minRows: 7, maxRows: 12 }}
                  disabled={!canUpdateRoles}
                  placeholder='admin:channel:*'
                  value={assignmentDraft.allowText}
                  onChange={(value) => setAssignmentDraft({ allowText: value })}
                />
              </div>
              <div className='aurora-assignment-card'>
                <div className='aurora-assignment-card-head'>
                  <strong>{t('拒绝权限覆盖')}</strong>
                  <small>{t('优先级最高')}</small>
                </div>
                <TextArea
                  autosize={{ minRows: 7, maxRows: 12 }}
                  disabled={!canUpdateRoles}
                  placeholder='admin:system:roles:update'
                  value={assignmentDraft.denyText}
                  onChange={(value) => setAssignmentDraft({ denyText: value })}
                />
              </div>
              <div className='aurora-assignment-card'>
                <div className='aurora-assignment-card-head'>
                  <strong>{t('生效权限')}</strong>
                  <small>
                    {assignmentDraft.effective_permissions.length} {t('项权限')}
                  </small>
                </div>
                <div className='aurora-effective-permission-list'>
                  {assignmentDraft.effective_permissions.length > 0 ? (
                    assignmentDraft.effective_permissions
                      .slice(0, 18)
                      .map((item) => <span key={item}>{item}</span>)
                  ) : (
                    <em>{t('暂无数据库权限，仍按固定角色兼容。')}</em>
                  )}
                  {assignmentDraft.effective_permissions.length > 18 ? (
                    <em>
                      +{assignmentDraft.effective_permissions.length - 18}{' '}
                      {t('项')}
                    </em>
                  ) : null}
                </div>
              </div>
            </div>
          )}
        </Spin>
        {assignmentDraft ? (
          <div className='aurora-assignment-actions'>
            <AdminPermissionButton
              dangerPermission={ADMIN_PERMISSION_KEYS.systemRolesUpdate}
              icon={<Save size={15} />}
              loading={assignmentSaving}
              onClick={saveAssignment}
              type='primary'
            >
              {t('保存用户权限')}
            </AdminPermissionButton>
          </div>
        ) : null}
      </section>

      <section className='aurora-source-grid'>
        <div className='aurora-source-item is-ok'>
          <span className='aurora-source-state'>
            <Lock size={14} />
            {t('后台入口')}
          </span>
          <strong>{t('路由守卫')}</strong>
          <small>
            {t('/admin/* 统一经过 AdminRoute，未授权账号不会进入管理后台。')}
          </small>
        </div>
        <div className='aurora-source-item is-ok'>
          <span className='aurora-source-state'>
            <Database size={14} />
            {t('权限来源')}
          </span>
          <strong>{t('兼容模式 + 数据库模式')}</strong>
          <small>
            {t(
              '没有数据库绑定时继续按 Role 10/100 兼容，有绑定后按 RBAC 生效。',
            )}
          </small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <AlertTriangle size={14} />
            {t('危险操作')}
          </span>
          <strong>{t('按钮与接口双重守卫')}</strong>
          <small>
            {t('高风险按钮使用独立权限点，后端接口继续记录权限审计。')}
          </small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <KeyRound size={14} />
            {t('迁移方向')}
          </span>
          <strong>{t('后续可独立部署')}</strong>
          <small>
            {t('管理后台已按独立目录推进，后续可继续拆构建和部署链路。')}
          </small>
        </div>
      </section>
    </div>
  );
};

export default AdminRoles;
