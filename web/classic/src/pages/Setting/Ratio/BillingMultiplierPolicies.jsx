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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Banner,
  Button,
  Empty,
  Form,
  Input,
  InputNumber,
  Modal,
  Popconfirm,
  Select,
  Space,
  Spin,
  Switch,
  TabPane,
  Tabs,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Boxes,
  ClipboardList,
  Edit3,
  Link2,
  PackageCheck,
  Percent,
  PlayCircle,
  Plus,
  RefreshCw,
  Search,
  Settings2,
  Trash2,
  UserRound,
  UsersRound,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import { ADMIN_PERMISSION_KEYS } from '../../../apps/admin-console/permissions/adminPermissions.config';
import {
  AdminPermissionButton,
  useAdminActionPermission,
} from '../../../apps/admin-console/permissions/AdminPermissionAction';
import './billing-multiplier-policies.css';

const { Text } = Typography;

const TARGET_TYPES = ['global', 'user', 'user_group', 'subscription_plan', 'using_group'];
const GROUP_PRICE_MODES = ['multiply', 'override', 'min', 'max'];

const DEFAULT_POLICY = {
  name: '',
  enabled: true,
  priority: 0,
  scope_type: 'global',
  scope_value: '',
  scope_id: 0,
  scope_key: '',
  scope_name: '',
  using_groups: [],
  group_multipliers: [],
  group_prices: [],
  targets: [{ target_type: 'global', enabled: true }],
  models: [],
  mode: 'multiply',
  multiplier: 1,
  start_at: 0,
  end_at: 0,
  description: '',
  preview_user_id: '',
  preview_user_group: '',
  preview_using_group: '',
  preview_model_name: '',
  preview_subscription_plan_id: '',
  preview_base_group_ratio: 1,
};

const SCOPE_ICON = {
  global: Settings2,
  user: UserRound,
  user_group: UsersRound,
  subscription_plan: PackageCheck,
  using_group: Boxes,
};

const parseListValue = (value) => {
  if (!value) return [];
  if (Array.isArray(value)) return value.filter(Boolean);
  const text = String(value).trim();
  if (!text) return [];
  try {
    const parsed = JSON.parse(text);
    if (Array.isArray(parsed)) return parsed.filter(Boolean);
  } catch {
    // Legacy comma separated values.
  }
  return text
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
};

const listToJsonText = (value) => {
  const values = parseListValue(value);
  return values.length > 0 ? JSON.stringify(values) : '';
};

const makeID = () => `${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;

const makeTargetRow = (row = {}) => ({
  _id: row._id || makeID(),
  id: row.id || 0,
  policy_id: row.policy_id || 0,
  target_type: TARGET_TYPES.includes(row.target_type) ? row.target_type : 'global',
  target_id:
    row.target_id === '' || row.target_id === undefined || row.target_id === null
      ? ''
      : String(row.target_id),
  target_key: String(row.target_key || '').trim(),
  target_name: String(row.target_name || '').trim(),
  enabled: row.enabled !== false,
});

const makeGroupPriceRow = (row = {}) => ({
  _id: row._id || makeID(),
  id: row.id || 0,
  policy_id: row.policy_id || 0,
  using_group: String(row.using_group || row.group_key || row.group || '').trim(),
  mode: GROUP_PRICE_MODES.includes(row.mode) ? row.mode : 'override',
  multiplier:
    row.multiplier === '' || row.multiplier === undefined || row.multiplier === null
      ? 1
      : Number(row.multiplier),
  priority: Number(row.priority || 0) || 0,
  enabled: row.enabled !== false,
});

const parseGroupPriceRows = (policy = {}) => {
  const rows = Array.isArray(policy.group_prices) ? policy.group_prices : [];
  if (rows.length > 0) {
    return rows.map(makeGroupPriceRow).filter((item) => item.using_group);
  }
  const legacy = policy.group_multipliers;
  let parsed = legacy;
  if (typeof legacy === 'string') {
    const text = legacy.trim();
    if (!text) return [];
    try {
      parsed = JSON.parse(text);
    } catch {
      return [];
    }
  }
  if (!Array.isArray(parsed)) return [];
  return parsed.map(makeGroupPriceRow).filter((item) => item.using_group);
};

const legacyTargetFromPolicy = (policy = {}) => {
  const scopeType = policy.scope_type || 'global';
  const scopeID =
    Number(policy.scope_id || 0) > 0
      ? Number(policy.scope_id)
      : Number(policy.scope_value || 0) || 0;
  const scopeKey = policy.scope_key || policy.scope_value || '';
  return makeTargetRow({
    target_type: scopeType,
    target_id: scopeID > 0 ? scopeID : '',
    target_key: scopeType === 'user' || scopeType === 'subscription_plan' ? '' : scopeKey,
    target_name: policy.scope_name || '',
    enabled: true,
  });
};

const policyToFormValues = (policy = {}) => {
  const merged = { ...DEFAULT_POLICY, ...policy };
  const targets = Array.isArray(merged.targets) && merged.targets.length > 0
    ? merged.targets.map(makeTargetRow)
    : [legacyTargetFromPolicy(merged)];
  const groupPrices = parseGroupPriceRows(merged);
  return {
    ...merged,
    targets,
    group_prices: groupPrices,
    group_multipliers: groupPrices,
    using_groups: parseListValue(merged.using_groups),
    models: parseListValue(merged.models),
    preview_user_id: merged.preview_user_id || '',
    preview_subscription_plan_id: merged.preview_subscription_plan_id || '',
  };
};

const formatMultiplier = (value) => `${Number(value || 0).toFixed(6)}x`;

export default function BillingMultiplierPolicies({
  ratioDangerPermissionDenied,
  ensureRatioPermission,
}) {
  const { t } = useTranslation();
  const canManage = useAdminActionPermission(
    ADMIN_PERMISSION_KEYS.modelRatioUpdate,
  );
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [policies, setPolicies] = useState([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [formApi, setFormApi] = useState(null);
  const [formSeed, setFormSeed] = useState(policyToFormValues(DEFAULT_POLICY));
  const [targets, setTargets] = useState(policyToFormValues(DEFAULT_POLICY).targets);
  const [groupPrices, setGroupPrices] = useState([]);
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [groupOptions, setGroupOptions] = useState([]);
  const [planOptions, setPlanOptions] = useState([]);
  const [userOptions, setUserOptions] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);

  const targetTypeOptions = useMemo(
    () => [
      { label: t('全局规则'), value: 'global' },
      { label: t('指定用户'), value: 'user' },
      { label: t('用户分组'), value: 'user_group' },
      { label: t('订阅套餐'), value: 'subscription_plan' },
      { label: t('使用分组'), value: 'using_group' },
    ],
    [t],
  );

  const modeOptions = useMemo(
    () => [
      { label: t('叠加倍率'), value: 'multiply' },
      { label: t('覆盖倍率'), value: 'override' },
      { label: t('倍率下限'), value: 'min' },
      { label: t('倍率上限'), value: 'max' },
    ],
    [t],
  );

  const targetTypeLabelMap = useMemo(
    () => Object.fromEntries(targetTypeOptions.map((item) => [item.value, item.label])),
    [targetTypeOptions],
  );

  const modeLabelMap = useMemo(
    () => Object.fromEntries(modeOptions.map((item) => [item.value, item.label])),
    [modeOptions],
  );

  const selectedOptionName = (options, value) => {
    const found = options.find((item) => String(item.value) === String(value));
    return found?.name || found?.label || '';
  };

  const fetchPolicies = useCallback(async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/billing-multiplier-policies/');
      if (res.data.success) {
        setPolicies(res.data.data || []);
      } else {
        showError(res.data.message || t('加载失败'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  }, [t]);

  const loadGroups = useCallback(async () => {
    try {
      const res = await API.get('/api/group/');
      if (res.data?.success && Array.isArray(res.data.data)) {
        setGroupOptions(
          res.data.data.map((group) => ({
            label: group,
            value: group,
          })),
        );
      }
    } catch (error) {
      console.error('failed to load groups', error);
    }
  }, []);

  const loadPlans = useCallback(async () => {
    try {
      const res = await API.get('/api/subscription/admin/plans');
      if (res.data?.success) {
        setPlanOptions(
          (res.data.data || []).map((item) => {
            const plan = item.plan || item;
            return {
              label: `#${plan.id} · ${plan.title}`,
              value: String(plan.id),
              name: plan.title,
              disabled: plan.enabled === false,
            };
          }),
        );
      }
    } catch (error) {
      console.error('failed to load subscription plans', error);
    }
  }, []);

  const searchUsers = useCallback(async (keyword = '') => {
    const encoded = encodeURIComponent(keyword || '');
    const url = keyword
      ? `/api/user/search?keyword=${encoded}&group=&p=1&page_size=20`
      : '/api/user/?p=1&page_size=20';
    try {
      const res = await API.get(url);
      if (res.data?.success) {
        const items = res.data.data?.items || [];
        setUserOptions(
          items.map((user) => ({
            label: `#${user.id} · ${user.display_name || user.username}`,
            value: String(user.id),
            name: user.display_name || user.username,
          })),
        );
      }
    } catch (error) {
      console.error('failed to search users', error);
    }
  }, []);

  const searchModels = useCallback(async (keyword = '') => {
    const encoded = encodeURIComponent(keyword || '');
    const url = keyword
      ? `/api/models/search?keyword=${encoded}&vendor=&p=1&page_size=50`
      : '/api/models/?p=1&page_size=50';
    try {
      const res = await API.get(url);
      if (res.data?.success) {
        const items = res.data.data?.items || [];
        setModelOptions(
          items.map((model) => ({
            label: model.model_name,
            value: model.model_name,
          })),
        );
      }
    } catch (error) {
      console.error('failed to search models', error);
    }
  }, []);

  useEffect(() => {
    fetchPolicies();
    loadGroups();
    loadPlans();
    searchUsers();
    searchModels();
  }, [fetchPolicies, loadGroups, loadPlans, searchModels, searchUsers]);

  useEffect(() => {
    if (modalVisible && formApi) {
      formApi.setValues(formSeed);
    }
  }, [formApi, formSeed, modalVisible]);

  const openCreate = () => {
    const values = policyToFormValues(DEFAULT_POLICY);
    setEditing(null);
    setPreview(null);
    setTargets(values.targets);
    setGroupPrices([]);
    setFormSeed(values);
    setModalVisible(true);
  };

  const openEdit = (record) => {
    const values = policyToFormValues(record);
    setEditing(record);
    setPreview(null);
    setTargets(values.targets);
    setGroupPrices(values.group_prices || []);
    setFormSeed(values);
    setModalVisible(true);
  };

  const addTarget = (type = 'user') => {
    setTargets((rows) => {
      const next = makeTargetRow({ target_type: type, enabled: true });
      if (type !== 'global' && rows.length === 1 && rows[0].target_type === 'global') {
        return [next];
      }
      return [...rows, next];
    });
  };

  const updateTarget = (rowID, patch) => {
    setTargets((rows) =>
      rows.map((row) =>
        row._id === rowID
          ? makeTargetRow({
              ...row,
              ...patch,
              target_id: patch.target_type && patch.target_type !== row.target_type ? '' : row.target_id,
              target_key: patch.target_type && patch.target_type !== row.target_type ? '' : row.target_key,
              target_name: patch.target_type && patch.target_type !== row.target_type ? '' : row.target_name,
            })
          : row,
      ),
    );
  };

  const removeTarget = (rowID) => {
    setTargets((rows) => rows.filter((row) => row._id !== rowID));
  };

  const addGroupPrice = () => {
    setGroupPrices((rows) => [...rows, makeGroupPriceRow({ enabled: true })]);
  };

  const updateGroupPrice = (rowID, patch) => {
    setGroupPrices((rows) =>
      rows.map((row) => (row._id === rowID ? makeGroupPriceRow({ ...row, ...patch }) : row)),
    );
  };

  const removeGroupPrice = (rowID) => {
    setGroupPrices((rows) => rows.filter((row) => row._id !== rowID));
  };

  const normalizeTargets = () => {
    const normalized = [];
    const seen = new Set();
    const activeRows = targets.filter((row) => row.enabled !== false);
    if (activeRows.length > 1 && activeRows.some((row) => row.target_type === 'global')) {
      return { error: t('全局规则不能和其他关联对象同时配置') };
    }
    for (const row of targets) {
      const type = row.target_type || 'global';
      if (!TARGET_TYPES.includes(type)) {
        return { error: t('关联对象类型不正确') };
      }
      let targetID = Number(row.target_id || 0) || 0;
      let targetKey = String(row.target_key || '').trim();
      let targetName = String(row.target_name || '').trim();
      if (type === 'global') {
        targetID = 0;
        targetKey = '';
        targetName = '';
      } else if (type === 'user') {
        if (targetID <= 0) return { error: t('请选择关联用户') };
        targetKey = String(targetID);
        targetName = targetName || selectedOptionName(userOptions, targetID);
      } else if (type === 'subscription_plan') {
        if (targetID <= 0) return { error: t('请选择订阅套餐') };
        targetKey = String(targetID);
        targetName = targetName || selectedOptionName(planOptions, targetID);
      } else {
        if (!targetKey) return { error: t('请选择稳定分组 Key') };
        targetID = 0;
        targetName = targetName || targetKey;
      }
      const seenKey = `${type}:${targetID || targetKey}`.toLowerCase();
      if (seen.has(seenKey)) {
        return { error: t('关联对象不能重复') };
      }
      seen.add(seenKey);
      normalized.push({
        id: Number(row.id || 0) || 0,
        target_type: type,
        target_id: targetID,
        target_key: targetKey,
        target_name: targetName,
        enabled: row.enabled !== false,
      });
    }
    if (normalized.length === 0) {
      return { error: t('请至少添加一个关联对象') };
    }
    return { rows: normalized };
  };

  const normalizeGroupPrices = () => {
    const seen = new Set();
    const normalized = [];
    for (const row of groupPrices) {
      const usingGroup = String(row.using_group || '').trim();
      if (!usingGroup) return { error: t('请先选择分组价格的使用分组') };
      const seenKey = usingGroup.toLowerCase();
      if (seen.has(seenKey)) return { error: t('分组价格不能重复') };
      seen.add(seenKey);
      const multiplier = Number(row.multiplier);
      if (!Number.isFinite(multiplier) || multiplier < 0) {
        return { error: t('分组价格倍率必须大于等于 0') };
      }
      normalized.push({
        id: Number(row.id || 0) || 0,
        using_group: usingGroup,
        group_key: usingGroup,
        mode: GROUP_PRICE_MODES.includes(row.mode) ? row.mode : 'override',
        multiplier,
        priority: Number(row.priority || 0) || 0,
        enabled: row.enabled !== false,
      });
    }
    return { rows: normalized };
  };

  const normalizePolicy = (values) => {
    const targetResult = normalizeTargets();
    if (targetResult.error) throw new Error(targetResult.error);
    const groupPriceResult = normalizeGroupPrices();
    if (groupPriceResult.error) throw new Error(groupPriceResult.error);
    const normalizedTargets = targetResult.rows || [];
    const normalizedGroupPrices = groupPriceResult.rows || [];
    const firstTarget = normalizedTargets[0] || { target_type: 'global' };
    const scopeType = firstTarget.target_type || 'global';
    const scopeID = firstTarget.target_id || 0;
    const scopeKey =
      scopeType === 'user' || scopeType === 'subscription_plan'
        ? ''
        : firstTarget.target_key || '';
    const scopeValue =
      scopeType === 'user' || scopeType === 'subscription_plan'
        ? scopeID > 0 ? String(scopeID) : ''
        : scopeKey;
    const scopeName = firstTarget.target_name || '';
    const name = String(values.name || '').trim() || t('VIP 专属倍率规则');
    return {
      ...DEFAULT_POLICY,
      ...values,
      id: editing?.id || values.id || 0,
      name,
      enabled: Boolean(values.enabled),
      priority: Number(values.priority) || 0,
      scope_type: scopeType,
      scope_value: scopeValue,
      scope_id: scopeID,
      scope_key: scopeKey,
      scope_name: scopeName,
      targets: normalizedTargets,
      group_prices: normalizedGroupPrices,
      group_multipliers:
        normalizedGroupPrices.length > 0 ? JSON.stringify(normalizedGroupPrices) : '',
      using_groups: listToJsonText(normalizedGroupPrices.map((item) => item.using_group)),
      multiplier: Number(values.multiplier) || 0,
      start_at: Number(values.start_at) || 0,
      end_at: Number(values.end_at) || 0,
      models: listToJsonText(values.models),
    };
  };

  const savePolicy = async () => {
    if (ensureRatioPermission && !ensureRatioPermission()) return;
    if (!formApi) return;
    let values;
    try {
      values = await formApi.validate();
    } catch {
      return;
    }
    let policy;
    try {
      policy = normalizePolicy(values);
    } catch (error) {
      showError(error.message);
      return;
    }
    setSaving(true);
    try {
      const url = editing
        ? `/api/billing-multiplier-policies/${editing.id}`
        : '/api/billing-multiplier-policies/';
      const method = editing ? API.put : API.post;
      const res = await method(url, { policy });
      if (res.data.success) {
        showSuccess(t('保存成功'));
        setModalVisible(false);
        fetchPolicies();
      } else {
        showError(res.data.message || t('保存失败'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };

  const deletePolicy = async (record) => {
    if (ensureRatioPermission && !ensureRatioPermission()) return;
    try {
      const res = await API.delete(`/api/billing-multiplier-policies/${record.id}`);
      if (res.data.success) {
        showSuccess(t('删除成功'));
        fetchPolicies();
      } else {
        showError(res.data.message || t('删除失败'));
      }
    } catch (error) {
      showError(error.message);
    }
  };

  const runPreview = async () => {
    if (!formApi) return;
    let values;
    try {
      values = await formApi.validate();
    } catch {
      return;
    }
    let policy;
    try {
      policy = normalizePolicy(values);
    } catch (error) {
      showError(error.message);
      return;
    }
    setPreviewLoading(true);
    try {
      const res = await API.post('/api/billing-multiplier-policies/preview', {
        user_id: Number(values.preview_user_id) || 0,
        user_group: values.preview_user_group || '',
        using_group: values.preview_using_group || '',
        model_name: values.preview_model_name || '',
        subscription_plan_id: Number(values.preview_subscription_plan_id) || 0,
        base_group_ratio: Number(values.preview_base_group_ratio) || 1,
        policy,
      });
      if (res.data.success) {
        setPreview(res.data.data);
      } else {
        showError(res.data.message || t('预览失败'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setPreviewLoading(false);
    }
  };

  const stats = useMemo(() => {
    const enabled = policies.filter((item) => item.enabled).length;
    const targetCount = policies.reduce((sum, item) => sum + Number(item.target_count || item.targets?.length || 0), 0);
    const userTargets = policies.reduce((sum, item) => sum + Number(item.user_target_count || 0), 0);
    const groupPriceCount = policies.reduce((sum, item) => sum + Number(item.group_price_count || item.group_prices?.length || 0), 0);
    return {
      total: policies.length,
      enabled,
      targetCount,
      userTargets,
      groupPriceCount,
    };
  }, [policies]);

  const renderTargetsSummary = (record) => {
    const rows = Array.isArray(record.targets) && record.targets.length > 0
      ? record.targets
      : [legacyTargetFromPolicy(record)];
    return (
      <div className='ct-billing-policy-target-stack'>
        {rows.slice(0, 4).map((target, index) => {
          const Icon = SCOPE_ICON[target.target_type] || Link2;
          const label =
            target.target_type === 'global'
              ? t('全部请求')
              : target.target_name ||
                target.target_key ||
                (target.target_id ? `#${target.target_id}` : '-');
          return (
            <Tag key={`${target.target_type}-${target.target_id}-${target.target_key}-${index}`} color='teal' type='light'>
              <Icon size={13} />
              {targetTypeLabelMap[target.target_type] || target.target_type} · {label}
            </Tag>
          );
        })}
        {rows.length > 4 ? (
          <Tag type='light'>+{rows.length - 4}</Tag>
        ) : null}
      </div>
    );
  };

  const renderListChips = (raw, emptyText) => {
    const values = parseListValue(raw);
    if (values.length === 0) {
      return <Tag type='light'>{emptyText}</Tag>;
    }
    return values.slice(0, 4).map((item) => (
      <Tag key={item} color='cyan' type='light'>
        {item}
      </Tag>
    ));
  };

  const renderTargetEditor = () => (
    <div className='ct-billing-policy-target-editor'>
      <div className='ct-billing-policy-target-toolbar'>
        <Text>{t('一个规则可以关联多个用户、用户分组或订阅套餐。')}</Text>
        <Space>
          <Button icon={<UserRound size={14} />} size='small' theme='borderless' onClick={() => addTarget('user')}>
            {t('添加用户')}
          </Button>
          <Button icon={<UsersRound size={14} />} size='small' theme='borderless' onClick={() => addTarget('user_group')}>
            {t('添加用户分组')}
          </Button>
          <Button icon={<PackageCheck size={14} />} size='small' theme='borderless' onClick={() => addTarget('subscription_plan')}>
            {t('添加套餐')}
          </Button>
          <Button icon={<Boxes size={14} />} size='small' theme='borderless' onClick={() => addTarget('using_group')}>
            {t('添加使用分组')}
          </Button>
        </Space>
      </div>
      <div className='ct-billing-policy-target-list'>
        <div className='ct-billing-policy-target-header'>
          <span>{t('启用')}</span>
          <span>{t('类型')}</span>
          <span>{t('关联对象')}</span>
          <span>{t('名称快照')}</span>
          <span>{t('操作')}</span>
        </div>
        {targets.map((row) => (
          <div className='ct-billing-policy-target-row' key={row._id}>
            <Switch size='small' checked={row.enabled !== false} onChange={(checked) => updateTarget(row._id, { enabled: checked })} />
            <Select
              optionList={targetTypeOptions}
              value={row.target_type}
              onChange={(value) => updateTarget(row._id, { target_type: value || 'global' })}
            />
            {row.target_type === 'user' ? (
              <Select
                placeholder={t('搜索用户')}
                optionList={userOptions}
                value={row.target_id || undefined}
                filter
                remote
                showClear
                onSearch={searchUsers}
                onChange={(value) => updateTarget(row._id, {
                  target_id: value || '',
                  target_name: selectedOptionName(userOptions, value),
                })}
              />
            ) : row.target_type === 'subscription_plan' ? (
              <Select
                placeholder={t('选择订阅套餐')}
                optionList={planOptions}
                value={row.target_id || undefined}
                filter
                showClear
                onChange={(value) => updateTarget(row._id, {
                  target_id: value || '',
                  target_name: selectedOptionName(planOptions, value),
                })}
              />
            ) : row.target_type === 'user_group' || row.target_type === 'using_group' ? (
              <Select
                placeholder={row.target_type === 'using_group' ? t('选择或输入使用分组 Key') : t('选择或输入用户分组 Key')}
                optionList={groupOptions}
                value={row.target_key || undefined}
                filter
                allowCreate
                showClear
                onChange={(value) => updateTarget(row._id, {
                  target_key: value || '',
                  target_name: value || '',
                })}
              />
            ) : (
              <Tag color='green' type='light'>
                {t('全部请求')}
              </Tag>
            )}
            <Input
              value={row.target_name || ''}
              placeholder={t('自动记录展示名')}
              onChange={(value) => updateTarget(row._id, { target_name: value })}
              disabled={row.target_type === 'global'}
            />
            <Button
              icon={<Trash2 size={15} />}
              size='small'
              type='danger'
              theme='borderless'
              disabled={targets.length <= 1}
              onClick={() => removeTarget(row._id)}
            />
          </div>
        ))}
      </div>
    </div>
  );

  const renderGroupPriceEditor = () => {
    if (groupPrices.length === 0) {
      return (
        <div className='ct-billing-policy-group-empty'>
          <span>{t('未配置分组价格，命中后使用默认倍率。')}</span>
          <Button icon={<Plus size={15} />} size='small' theme='borderless' onClick={addGroupPrice}>
            {t('添加分组价格')}
          </Button>
        </div>
      );
    }
    return (
      <div className='ct-billing-policy-group-list'>
        <div className='ct-billing-policy-group-header'>
          <span>{t('启用')}</span>
          <span>{t('使用分组')}</span>
          <span>{t('计算模式')}</span>
          <span>{t('倍率')}</span>
          <span>{t('优先级')}</span>
          <span>{t('操作')}</span>
        </div>
        {groupPrices.map((row) => (
          <div className='ct-billing-policy-group-row' key={row._id}>
            <Switch size='small' checked={row.enabled !== false} onChange={(checked) => updateGroupPrice(row._id, { enabled: checked })} />
            <Select
              placeholder={t('选择使用分组')}
              optionList={groupOptions}
              value={row.using_group || undefined}
              filter
              allowCreate
              showClear
              onChange={(value) => updateGroupPrice(row._id, { using_group: value || '' })}
            />
            <Select
              optionList={modeOptions}
              value={row.mode || 'override'}
              onChange={(value) => updateGroupPrice(row._id, { mode: value || 'override' })}
            />
            <InputNumber min={0} step={0.01} precision={6} value={row.multiplier} onChange={(value) => updateGroupPrice(row._id, { multiplier: value })} />
            <InputNumber step={10} value={row.priority} onChange={(value) => updateGroupPrice(row._id, { priority: value })} />
            <Button icon={<Trash2 size={15} />} size='small' type='danger' theme='borderless' onClick={() => removeGroupPrice(row._id)} />
          </div>
        ))}
        <Button className='ct-billing-policy-add-group' icon={<Plus size={15} />} theme='borderless' onClick={addGroupPrice}>
          {t('添加分组价格')}
        </Button>
      </div>
    );
  };

  return (
    <div className='ct-billing-policy-page'>
      <section className='ct-billing-policy-hero'>
        <div>
          <div className='ct-billing-policy-kicker'>
            <Percent size={15} />
            {t('运营倍率规则')}
          </div>
          <h2>{t('VIP 专属规则与分组价格')}</h2>
          <p>{t('先创建规则，再关联多个用户、用户组或套餐；请求链路按内存索引命中，避免名称变更影响计费。')}</p>
        </div>
        <div className='ct-billing-policy-actions'>
          <Button icon={<RefreshCw size={15} />} onClick={fetchPolicies} loading={loading} theme='borderless'>
            {t('刷新')}
          </Button>
          <AdminPermissionButton
            dangerPermission={ADMIN_PERMISSION_KEYS.modelRatioUpdate}
            fallbackTooltip={ratioDangerPermissionDenied}
            icon={<ClipboardList size={15} />}
            type='primary'
            theme='solid'
            onClick={openCreate}
          >
            {t('新增规则')}
          </AdminPermissionButton>
        </div>
      </section>

      <div className='ct-billing-policy-metrics'>
        <div>
          <span>{t('规则总数')}</span>
          <strong>{stats.total}</strong>
          <small>{t('当前策略库')}</small>
        </div>
        <div>
          <span>{t('已启用')}</span>
          <strong>{stats.enabled}</strong>
          <small>{t('参与实时扣费')}</small>
        </div>
        <div>
          <span>{t('关联对象')}</span>
          <strong>{stats.targetCount}</strong>
          <small>{t('按 ID 或稳定 Key 命中')}</small>
        </div>
        <div>
          <span>{t('分组价格')}</span>
          <strong>{stats.groupPriceCount}</strong>
          <small>{t('使用分组价格矩阵')}</small>
        </div>
      </div>

      <Banner
        type='info'
        description={t('用户与订阅套餐使用 ID 关联；分组使用稳定 Key。保存后会刷新后端缓存，请求链路不会实时扫表。')}
      />

      <Spin spinning={loading}>
        {policies.length === 0 ? (
          <Empty className='ct-billing-policy-empty' description={t('暂无倍率规则')} />
        ) : (
          <div className='ct-billing-policy-list'>
            {policies.map((record) => (
              <article className='ct-billing-policy-card' key={record.id}>
                <div className='ct-billing-policy-card-head'>
                  <div>
                    <div className='ct-billing-policy-title-row'>
                      <strong>{record.name}</strong>
                      <Tag color={record.enabled ? 'green' : 'grey'} type='light'>
                        {record.enabled ? t('启用') : t('停用')}
                      </Tag>
                    </div>
                    <p>{record.description || t('未填写备注')}</p>
                  </div>
                  <Space>
                    <Button icon={<Edit3 size={15} />} size='small' theme='borderless' onClick={() => openEdit(record)} />
                    <Popconfirm title={t('确认删除该规则？')} onConfirm={() => deletePolicy(record)}>
                      <Button icon={<Trash2 size={15} />} size='small' type='danger' theme='borderless' disabled={!canManage} />
                    </Popconfirm>
                  </Space>
                </div>

                <div className='ct-billing-policy-body-grid'>
                  <div className='ct-billing-policy-effect'>
                    <span>{t('关联对象')}</span>
                    <strong>{Number(record.target_count || record.targets?.length || 0)}</strong>
                    <small>{t('用户')} {record.user_target_count || 0} · {t('分组')} {record.group_target_count || 0} · {t('套餐')} {record.plan_target_count || 0}</small>
                  </div>
                  <div className='ct-billing-policy-effect'>
                    <span>{t('默认策略')}</span>
                    <strong>{modeLabelMap[record.mode] || record.mode}</strong>
                    <small>{formatMultiplier(record.multiplier)}</small>
                  </div>
                  <div className='ct-billing-policy-priority'>
                    <span>{t('优先级')}</span>
                    <strong>{record.priority}</strong>
                    <small>{t('数值越大越先执行')}</small>
                  </div>
                </div>

                {renderTargetsSummary(record)}
                <div className='ct-billing-policy-chip-row'>
                  <span>{t('使用分组')}</span>
                  {renderListChips(record.using_groups, t('全部使用分组'))}
                </div>
                <div className='ct-billing-policy-chip-row'>
                  <span>{t('模型')}</span>
                  {renderListChips(record.models, t('全部模型'))}
                </div>
              </article>
            ))}
          </div>
        )}
      </Spin>

      <Modal
        className='ct-billing-policy-modal'
        title={editing ? t('编辑倍率规则') : t('新增倍率规则')}
        visible={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={savePolicy}
        confirmLoading={saving}
        width={1180}
      >
        <Form className='ct-billing-policy-form' getFormApi={setFormApi} initValues={formSeed} layout='vertical'>
          <Tabs type='line'>
            <TabPane tab={t('基础信息')} itemKey='base'>
              <section className='ct-billing-policy-form-section'>
                <div className='ct-billing-policy-section-head'>
                  <Settings2 size={16} />
                  <span>
                    <strong>{t('基础信息')}</strong>
                    <small>{t('规则名称、优先级和默认倍率。')}</small>
                  </span>
                </div>
                <div className='ct-billing-policy-form-grid'>
                  <Form.Input field='name' label={t('规则名称')} placeholder={t('例如：VIP2 专属倍率')} rules={[{ required: true, message: t('名称不能为空') }]} />
                  <Form.Switch field='enabled' label={t('启用')} />
                  <Form.InputNumber field='priority' label={t('优先级')} step={10} />
                  <Form.Select field='mode' label={t('默认计算模式')} optionList={modeOptions} />
                  <Form.InputNumber field='multiplier' label={t('默认倍率')} min={0} step={0.01} precision={6} />
                  <Form.TextArea field='description' label={t('备注')} autosize className='ct-billing-policy-form-wide' />
                </div>
              </section>
            </TabPane>

            <TabPane tab={t('关联对象')} itemKey='targets'>
              <section className='ct-billing-policy-form-section'>
                <div className='ct-billing-policy-section-head'>
                  <Link2 size={16} />
                  <span>
                    <strong>{t('关联对象')}</strong>
                    <small>{t('规则是主体，用户、用户组和套餐只是关联对象。')}</small>
                  </span>
                </div>
                {renderTargetEditor()}
              </section>
            </TabPane>

            <TabPane tab={t('分组价格')} itemKey='group_prices'>
              <section className='ct-billing-policy-form-section'>
                <div className='ct-billing-policy-section-head'>
                  <Boxes size={16} />
                  <span>
                    <strong>{t('分组价格列表')}</strong>
                    <small>{t('一行一个使用分组，命中后优先使用该行倍率。')}</small>
                  </span>
                </div>
                {renderGroupPriceEditor()}
              </section>
            </TabPane>

            <TabPane tab={t('命中条件')} itemKey='conditions'>
              <section className='ct-billing-policy-form-section'>
                <div className='ct-billing-policy-section-head'>
                  <Search size={16} />
                  <span>
                    <strong>{t('命中条件')}</strong>
                    <small>{t('模型和时间用于缩小命中范围。')}</small>
                  </span>
                </div>
                <div className='ct-billing-policy-form-grid'>
                  <Form.Select
                    field='models'
                    label={t('模型')}
                    placeholder={t('搜索并选择模型，留空代表全部模型')}
                    optionList={modelOptions}
                    multiple
                    filter
                    remote
                    allowCreate
                    showClear
                    onSearch={searchModels}
                  />
                  <Form.InputNumber field='start_at' label={t('开始时间戳')} min={0} />
                  <Form.InputNumber field='end_at' label={t('结束时间戳')} min={0} />
                </div>
              </section>
            </TabPane>

            <TabPane tab={t('命中预览')} itemKey='preview'>
              <section className='ct-billing-policy-form-section'>
                <div className='ct-billing-policy-section-head'>
                  <PlayCircle size={16} />
                  <span>
                    <strong>{t('命中预览')}</strong>
                    <small>{t('用真实用户 ID 和分组模拟一次扣费倍率链路。')}</small>
                  </span>
                </div>
                <div className='ct-billing-policy-form-grid'>
                  <Form.Select field='preview_user_id' label={t('用户ID')} placeholder={t('搜索用户用于预览')} optionList={userOptions} filter remote showClear onSearch={searchUsers} />
                  <Form.Select field='preview_user_group' label={t('用户分组')} optionList={groupOptions} filter allowCreate showClear />
                  <Form.Select field='preview_using_group' label={t('使用分组')} optionList={groupOptions} filter allowCreate showClear />
                  <Form.Select field='preview_model_name' label={t('模型')} placeholder={t('搜索模型用于预览')} optionList={modelOptions} filter remote showClear onSearch={searchModels} />
                  <Form.Select field='preview_subscription_plan_id' label={t('订阅套餐ID')} optionList={planOptions} filter showClear />
                  <Form.InputNumber field='preview_base_group_ratio' label={t('基础分组倍率')} min={0} step={0.01} />
                </div>
                <Button className='ct-billing-policy-preview-button' loading={previewLoading} theme='borderless' icon={<PlayCircle size={15} />} onClick={runPreview}>
                  {t('计算预览')}
                </Button>
                {preview ? (
                  <div className='ct-billing-policy-preview-result'>
                    <Tag color={preview.applied ? 'green' : 'grey'} type='light'>
                      {preview.applied ? t('已命中') : t('未命中')}
                    </Tag>
                    <Text>{t('基础分组倍率')}: {preview.base_group_ratio}</Text>
                    <Text>{t('最终分组倍率')}: {preview.final_group_ratio}</Text>
                    <Text>{t('调整倍率')}: {preview.multiplier}</Text>
                    {Array.isArray(preview.rules) && preview.rules.length > 0 ? (
                      <div className='ct-billing-policy-preview-rules'>
                        {preview.rules.map((rule) => (
                          <Tag key={`${rule.id}-${rule.name}`} color='cyan' type='light'>
                            {rule.name || `#${rule.id}`} · {modeLabelMap[rule.mode] || rule.mode} · {formatMultiplier(rule.multiplier)}
                          </Tag>
                        ))}
                      </div>
                    ) : null}
                  </div>
                ) : null}
              </section>
            </TabPane>
          </Tabs>
        </Form>
      </Modal>
    </div>
  );
}
