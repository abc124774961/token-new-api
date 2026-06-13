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
  Modal,
  Popconfirm,
  Space,
  Spin,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  BadgeCheck,
  Boxes,
  ClipboardList,
  Edit3,
  Link2,
  PackageCheck,
  Percent,
  PlayCircle,
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
  models: [],
  mode: 'multiply',
  multiplier: 1,
  start_at: 0,
  end_at: 0,
  description: '',
  preview_user_id: 0,
  preview_user_group: '',
  preview_using_group: '',
  preview_model_name: '',
  preview_subscription_plan_id: 0,
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
    // Fall back to comma separated legacy input.
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

const formatMultiplier = (value) => `${Number(value || 0).toFixed(6)}x`;

const policyToFormValues = (policy = {}) => {
  const merged = { ...DEFAULT_POLICY, ...policy };
  const scopeID =
    Number(merged.scope_id || 0) > 0
      ? Number(merged.scope_id)
      : Number(merged.scope_value || 0) || 0;
  const scopeKey = merged.scope_key || merged.scope_value || '';
  return {
    ...merged,
    scope_id: scopeID > 0 ? String(scopeID) : '',
    scope_key: scopeKey,
    scope_value: merged.scope_value || '',
    using_groups: parseListValue(merged.using_groups),
    models: parseListValue(merged.models),
    preview_user_id: merged.preview_user_id || '',
    preview_subscription_plan_id: merged.preview_subscription_plan_id || '',
  };
};

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
  const [scopeType, setScopeType] = useState('global');
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [groupOptions, setGroupOptions] = useState([]);
  const [planOptions, setPlanOptions] = useState([]);
  const [userOptions, setUserOptions] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);

  const scopeOptions = useMemo(
    () => [
      { label: t('全局规则'), value: 'global' },
      { label: t('指定用户ID'), value: 'user' },
      { label: t('用户分组 Key'), value: 'user_group' },
      { label: t('订阅套餐ID'), value: 'subscription_plan' },
      { label: t('使用分组 Key'), value: 'using_group' },
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

  const scopeLabelMap = useMemo(
    () => Object.fromEntries(scopeOptions.map((item) => [item.value, item.label])),
    [scopeOptions],
  );

  const modeLabelMap = useMemo(
    () => Object.fromEntries(modeOptions.map((item) => [item.value, item.label])),
    [modeOptions],
  );

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

  const resetScopeFields = (nextScopeType) => {
    setScopeType(nextScopeType);
    setPreview(null);
    formApi?.setValue('scope_id', '');
    formApi?.setValue('scope_key', '');
    formApi?.setValue('scope_value', '');
    formApi?.setValue('scope_name', '');
  };

  const openCreate = () => {
    setEditing(null);
    setPreview(null);
    setScopeType('global');
    setFormSeed(policyToFormValues(DEFAULT_POLICY));
    setModalVisible(true);
  };

  const openEdit = (record) => {
    const values = policyToFormValues(record);
    setEditing(record);
    setPreview(null);
    setScopeType(values.scope_type || 'global');
    setFormSeed(values);
    setModalVisible(true);
  };

  const selectedOptionName = (options, value) => {
    const found = options.find((item) => String(item.value) === String(value));
    return found?.name || found?.label || '';
  };

  const normalizePolicy = (values) => {
    const scope = values.scope_type || 'global';
    let scopeID = Number(values.scope_id || 0) || 0;
    let scopeKey = String(values.scope_key || '').trim();
    let scopeValue = '';
    let scopeName = String(values.scope_name || '').trim();

    if (scope === 'user') {
      scopeValue = scopeID > 0 ? String(scopeID) : '';
      scopeName = scopeName || selectedOptionName(userOptions, scopeID);
      scopeKey = '';
    } else if (scope === 'subscription_plan') {
      scopeValue = scopeID > 0 ? String(scopeID) : '';
      scopeName = scopeName || selectedOptionName(planOptions, scopeID);
      scopeKey = '';
    } else if (scope === 'user_group' || scope === 'using_group') {
      scopeID = 0;
      scopeValue = scopeKey;
      scopeName = scopeName || scopeKey;
    } else {
      scopeID = 0;
      scopeKey = '';
      scopeValue = '';
      scopeName = '';
    }

    return {
      ...DEFAULT_POLICY,
      ...values,
      id: editing?.id || values.id || 0,
      enabled: Boolean(values.enabled),
      priority: Number(values.priority) || 0,
      scope_type: scope,
      scope_value: scopeValue,
      scope_id: scopeID,
      scope_key: scopeKey,
      scope_name: scopeName,
      multiplier: Number(values.multiplier) || 0,
      start_at: Number(values.start_at) || 0,
      end_at: Number(values.end_at) || 0,
      using_groups: listToJsonText(values.using_groups),
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
    const policy = normalizePolicy(values);
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
      const res = await API.delete(
        `/api/billing-multiplier-policies/${record.id}`,
      );
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
    const policy = normalizePolicy(values);
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
    const stable = policies.filter(
      (item) =>
        ['user', 'subscription_plan'].includes(item.scope_type) &&
        Number(item.scope_id || 0) > 0,
    ).length;
    const legacy = policies.filter(
      (item) =>
        ['user', 'subscription_plan'].includes(item.scope_type) &&
        Number(item.scope_id || 0) <= 0 &&
        String(item.scope_value || '').trim() !== '',
    ).length;
    return {
      total: policies.length,
      enabled,
      stable,
      legacy,
    };
  }, [policies]);

  const renderPolicyTarget = (record) => {
    const ScopeIcon = SCOPE_ICON[record.scope_type] || Link2;
    const scopeID = Number(record.scope_id || 0);
    const legacyID = Number(record.scope_value || 0);
    const id = scopeID || legacyID;
    const key = record.scope_key || record.scope_value || '';
    const isIDScope = ['user', 'subscription_plan'].includes(record.scope_type);
    const primary =
      record.scope_type === 'global'
        ? t('全部请求')
        : isIDScope
          ? `#${id || '-'}`
          : key || '-';
    const secondary =
      record.scope_type === 'global'
        ? t('不绑定具体对象')
        : record.scope_name || (isIDScope ? t('未记录名称快照') : t('稳定 Key'));

    return (
      <div className='ct-billing-policy-target'>
        <span className='ct-billing-policy-target-icon'>
          <ScopeIcon size={16} />
        </span>
        <span>
          <strong>{primary}</strong>
          <small>{secondary}</small>
        </span>
      </div>
    );
  };

  const renderListChips = (raw, emptyText) => {
    const values = parseListValue(raw);
    if (values.length === 0) {
      return <Tag type='light'>{emptyText}</Tag>;
    }
    return values.slice(0, 4).map((item) => (
      <Tag key={item} color='teal' type='light'>
        {item}
      </Tag>
    ));
  };

  const renderScopeControl = () => {
    if (scopeType === 'global') {
      return (
        <div className='ct-billing-policy-global-note'>
          <BadgeCheck size={18} />
          <span>{t('全局规则会参与所有请求，请用高优先级和时间窗口控制影响面。')}</span>
        </div>
      );
    }
    if (scopeType === 'user') {
      return (
        <Form.Select
          field='scope_id'
          label={t('关联用户ID')}
          placeholder={t('搜索用户 ID、用户名、显示名称或邮箱')}
          optionList={userOptions}
          filter
          showClear
          remote
          onSearch={searchUsers}
          onChange={(value) => {
            formApi?.setValue('scope_name', selectedOptionName(userOptions, value));
          }}
          rules={[{ required: true, message: t('请选择关联用户ID') }]}
        />
      );
    }
    if (scopeType === 'subscription_plan') {
      return (
        <Form.Select
          field='scope_id'
          label={t('关联订阅套餐ID')}
          placeholder={t('选择订阅套餐 ID')}
          optionList={planOptions}
          filter
          showClear
          onChange={(value) => {
            formApi?.setValue('scope_name', selectedOptionName(planOptions, value));
          }}
          rules={[{ required: true, message: t('请选择订阅套餐ID') }]}
        />
      );
    }
    return (
      <Form.Select
        field='scope_key'
        label={scopeType === 'user_group' ? t('用户分组 Key') : t('使用分组 Key')}
        placeholder={t('选择或输入稳定分组 Key')}
        optionList={groupOptions}
        filter
        allowCreate
        showClear
        onChange={(value) => {
          formApi?.setValue('scope_name', value || '');
        }}
        rules={[{ required: true, message: t('请选择稳定分组 Key') }]}
      />
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
          <h2>{t('会员、用户与分组倍率')}</h2>
          <p>
            {t(
              '规则按稳定 ID 或分组 Key 命中，名称只作为展示快照，避免用户或套餐改名后计费策略失效。',
            )}
          </p>
        </div>
        <div className='ct-billing-policy-actions'>
          <Button
            icon={<RefreshCw size={15} />}
            onClick={fetchPolicies}
            loading={loading}
            theme='borderless'
          >
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
          <span>{t('ID 关联')}</span>
          <strong>{stats.stable}</strong>
          <small>{t('用户和套餐按 ID 命中')}</small>
        </div>
        <div className={stats.legacy > 0 ? 'is-warning' : ''}>
          <span>{t('旧关联')}</span>
          <strong>{stats.legacy}</strong>
          <small>{t('建议编辑后保存为 ID 关联')}</small>
        </div>
      </div>

      <Banner
        type='info'
        description={t(
          '用户与订阅套餐必须使用 ID 关联；用户分组和使用分组使用稳定 Key。优先级越大越先执行，命中结果会写入消费日志快照。',
        )}
      />

      <Spin spinning={loading}>
        {policies.length === 0 ? (
          <Empty
            className='ct-billing-policy-empty'
            description={t('暂无倍率规则')}
          />
        ) : (
          <div className='ct-billing-policy-list'>
            {policies.map((record) => (
              <article className='ct-billing-policy-card' key={record.id}>
                <div className='ct-billing-policy-card-head'>
                  <div>
                    <div className='ct-billing-policy-title-row'>
                      <strong>{record.name}</strong>
                      <Tag
                        color={record.enabled ? 'green' : 'grey'}
                        type='light'
                      >
                        {record.enabled ? t('启用') : t('停用')}
                      </Tag>
                    </div>
                    <p>{record.description || t('未填写备注')}</p>
                  </div>
                  <Space>
                    <Button
                      icon={<Edit3 size={15} />}
                      size='small'
                      theme='borderless'
                      onClick={() => openEdit(record)}
                    />
                    <Popconfirm
                      title={t('确认删除该规则？')}
                      onConfirm={() => deletePolicy(record)}
                    >
                      <Button
                        icon={<Trash2 size={15} />}
                        size='small'
                        type='danger'
                        theme='borderless'
                        disabled={!canManage}
                      />
                    </Popconfirm>
                  </Space>
                </div>

                <div className='ct-billing-policy-body-grid'>
                  {renderPolicyTarget(record)}
                  <div className='ct-billing-policy-effect'>
                    <span>{t('计算策略')}</span>
                    <strong>{modeLabelMap[record.mode] || record.mode}</strong>
                    <small>{formatMultiplier(record.multiplier)}</small>
                  </div>
                  <div className='ct-billing-policy-priority'>
                    <span>{t('优先级')}</span>
                    <strong>{record.priority}</strong>
                    <small>{t('数值越大越先执行')}</small>
                  </div>
                </div>

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
        width={980}
      >
        <Form
          className='ct-billing-policy-form'
          getFormApi={setFormApi}
          initValues={formSeed}
          layout='vertical'
        >
          <section className='ct-billing-policy-form-section'>
            <div className='ct-billing-policy-section-head'>
              <Link2 size={16} />
              <span>
                <strong>{t('对象关联')}</strong>
                <small>{t('用 ID 或稳定 Key 绑定对象，名称只做快照展示。')}</small>
              </span>
            </div>
            <div className='ct-billing-policy-form-grid'>
              <Form.Input
                field='name'
                label={t('规则名称')}
                placeholder={t('例如：VIP2 专属倍率')}
                rules={[{ required: true, message: t('名称不能为空') }]}
              />
              <Form.Select
                field='scope_type'
                label={t('匹配范围')}
                optionList={scopeOptions}
                onChange={resetScopeFields}
              />
              {renderScopeControl()}
              <Form.Input field='scope_name' label={t('名称快照')} disabled />
            </div>
          </section>

          <section className='ct-billing-policy-form-section'>
            <div className='ct-billing-policy-section-head'>
              <Search size={16} />
              <span>
                <strong>{t('命中条件')}</strong>
                <small>{t('不填使用分组或模型时表示该维度不过滤。')}</small>
              </span>
            </div>
            <div className='ct-billing-policy-form-grid'>
              <Form.Select
                field='using_groups'
                label={t('使用分组')}
                placeholder={t('选择可命中的使用分组')}
                optionList={groupOptions}
                multiple
                filter
                allowCreate
                showClear
              />
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
              <Form.InputNumber
                field='start_at'
                label={t('开始时间戳')}
                min={0}
              />
              <Form.InputNumber
                field='end_at'
                label={t('结束时间戳')}
                min={0}
              />
            </div>
          </section>

          <section className='ct-billing-policy-form-section'>
            <div className='ct-billing-policy-section-head'>
              <Percent size={16} />
              <span>
                <strong>{t('计费效果')}</strong>
                <small>{t('保存后只影响后续新请求，历史日志不会自动重算。')}</small>
              </span>
            </div>
            <div className='ct-billing-policy-form-grid'>
              <Form.Switch field='enabled' label={t('启用')} />
              <Form.InputNumber field='priority' label={t('优先级')} step={10} />
              <Form.Select
                field='mode'
                label={t('计算模式')}
                optionList={modeOptions}
              />
              <Form.InputNumber
                field='multiplier'
                label={t('倍率')}
                min={0}
                step={0.01}
                precision={6}
              />
              <Form.TextArea
                field='description'
                label={t('备注')}
                autosize
                className='ct-billing-policy-form-wide'
              />
            </div>
          </section>

          <section className='ct-billing-policy-form-section'>
            <div className='ct-billing-policy-section-head'>
              <PlayCircle size={16} />
              <span>
                <strong>{t('命中预览')}</strong>
                <small>{t('用真实用户 ID 和分组模拟一次扣费倍率链路。')}</small>
              </span>
            </div>
            <div className='ct-billing-policy-form-grid'>
              <Form.Select
                field='preview_user_id'
                label={t('用户ID')}
                placeholder={t('搜索用户用于预览')}
                optionList={userOptions}
                filter
                remote
                showClear
                onSearch={searchUsers}
              />
              <Form.Select
                field='preview_user_group'
                label={t('用户分组')}
                optionList={groupOptions}
                filter
                allowCreate
                showClear
              />
              <Form.Select
                field='preview_using_group'
                label={t('使用分组')}
                optionList={groupOptions}
                filter
                allowCreate
                showClear
              />
              <Form.Select
                field='preview_model_name'
                label={t('模型')}
                placeholder={t('搜索模型用于预览')}
                optionList={modelOptions}
                filter
                remote
                showClear
                onSearch={searchModels}
              />
              <Form.Select
                field='preview_subscription_plan_id'
                label={t('订阅套餐ID')}
                optionList={planOptions}
                filter
                showClear
              />
              <Form.InputNumber
                field='preview_base_group_ratio'
                label={t('基础分组倍率')}
                min={0}
                step={0.01}
              />
            </div>
            <Button
              className='ct-billing-policy-preview-button'
              loading={previewLoading}
              theme='borderless'
              icon={<PlayCircle size={15} />}
              onClick={runPreview}
            >
              {t('计算预览')}
            </Button>
            {preview ? (
              <div className='ct-billing-policy-preview-result'>
                <Tag color={preview.applied ? 'green' : 'grey'} type='light'>
                  {preview.applied ? t('已命中') : t('未命中')}
                </Tag>
                <Text>
                  {t('基础分组倍率')}: {preview.base_group_ratio}
                </Text>
                <Text>
                  {t('最终分组倍率')}: {preview.final_group_ratio}
                </Text>
                <Text>
                  {t('调整倍率')}: {preview.multiplier}
                </Text>
                {Array.isArray(preview.rules) && preview.rules.length > 0 ? (
                  <div className='ct-billing-policy-preview-rules'>
                    {preview.rules.map((rule) => (
                      <Tag key={`${rule.id}-${rule.name}`} color='cyan' type='light'>
                        {rule.name || `#${rule.id}`} ·{' '}
                        {modeLabelMap[rule.mode] || rule.mode} ·{' '}
                        {formatMultiplier(rule.multiplier)}
                      </Tag>
                    ))}
                  </div>
                ) : null}
              </div>
            ) : null}
          </section>
        </Form>
      </Modal>
    </div>
  );
}
