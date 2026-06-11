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

import React, { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tabs,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  AlertTriangle,
  Copy,
  Gauge,
  Plus,
  RadioTower,
  RefreshCw,
  RotateCcw,
  Save,
  ServerCrash,
  ShieldCheck,
  SlidersHorizontal,
} from 'lucide-react';
import { API, showError, showSuccess, showWarning } from '../../../helpers';
import SettingsModelGatewayScheduler from '../../../pages/Setting/Operation/SettingsModelGatewayScheduler';
import { ADMIN_PERMISSION_KEYS } from '../permissions/adminPermissions.config';
import { AdminPermissionButton } from '../permissions/AdminPermissionAction';

const VALID_TABS = [
  'policy',
  'upstream-errors',
  'avoidance',
  'probe',
  'dynamic',
];
const DEFAULT_RULE_VERSION = 1;
const COMMON_STATUS_CODES = [
  400, 401, 402, 403, 404, 408, 413, 422, 429, 451, 500, 502, 503, 504, 520,
  522, 523, 524, 525, 529,
];
const KEYWORD_FIELDS = ['code', 'type', 'message', 'metadata', 'header'];

const DEFAULT_SETTING_PATCH = {
  upstream_error_classification_enabled: true,
  upstream_error_rule_version: DEFAULT_RULE_VERSION,
  upstream_error_rules: [],
  observability_performance_mode_enabled: true,
  observability_diagnostic_level: 'errors_only',
  observability_client_request_trace_enabled: false,
  observability_score_event_enabled: false,
  observability_candidate_detail_enabled: false,
};

function arrayValue(value) {
  return Array.isArray(value) ? value : [];
}

function normalizeKeywords(keywords = {}) {
  return KEYWORD_FIELDS.reduce((acc, field) => {
    acc[field] = arrayValue(keywords?.[field]).map(String).filter(Boolean);
    return acc;
  }, {});
}

function normalizeRule(rule = {}, index = 0) {
  return {
    id: String(rule.id || `rule_${index + 1}`),
    enabled: rule.enabled !== false,
    priority: Number.isFinite(Number(rule.priority))
      ? Number(rule.priority)
      : 0,
    kind: String(rule.kind || 'rate_limit'),
    status_codes: arrayValue(rule.status_codes)
      .map((value) => Number(value))
      .filter(
        (value) => Number.isInteger(value) && value >= 100 && value <= 599,
      ),
    keywords: normalizeKeywords(rule.keywords),
    scheduler_action: String(rule.scheduler_action || 'switch_channel'),
    avoidance_seconds: Number.isFinite(Number(rule.avoidance_seconds))
      ? Math.max(0, Number(rule.avoidance_seconds))
      : 0,
    description: String(rule.description || ''),
  };
}

function normalizeRules(rules) {
  return arrayValue(rules).map((rule, index) => normalizeRule(rule, index));
}

function parseKeywordText(value) {
  return String(value || '')
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function keywordText(values) {
  return arrayValue(values).join('\n');
}

function statusSelectValue(values) {
  return arrayValue(values).map((value) => String(value));
}

function parseStatusValues(values) {
  const items = Array.isArray(values) ? values : values ? [values] : [];
  return [
    ...new Set(
      items
        .map((value) => Number(value))
        .filter(
          (value) => Number.isInteger(value) && value >= 100 && value <= 599,
        ),
    ),
  ].sort((a, b) => a - b);
}

function kindLabel(kind, t) {
  const labels = {
    balance_quota: t('余额或额度限制'),
    rate_limit: t('速率限制'),
    concurrency_limit: t('并发限制'),
    capacity_overload: t('容量过载'),
    model_pool_unavailable: t('模型池不可用'),
    tool_endpoint_unavailable: t('工具端点不可用'),
    auth_account: t('账号鉴权问题'),
    access_region: t('地域访问限制'),
    request_limit: t('请求规格限制'),
    policy_safety: t('内容安全限制'),
    transport_timeout: t('传输超时'),
    bad_response: t('上游响应异常'),
    stream_interrupted: t('流中断'),
  };
  return labels[kind] || kind;
}

function actionLabel(action, t) {
  const labels = {
    switch_channel: t('切换渠道'),
    stop: t('停止请求'),
    record_only: t('仅记录'),
  };
  return labels[action] || action;
}

function nextRuleID(rules) {
  const existing = new Set(arrayValue(rules).map((rule) => rule.id));
  let index = rules.length + 1;
  let id = `custom_rule_${index}`;
  while (existing.has(id)) {
    index += 1;
    id = `custom_rule_${index}`;
  }
  return id;
}

function ruleKeywordCount(rule) {
  return KEYWORD_FIELDS.reduce(
    (sum, field) => sum + arrayValue(rule?.keywords?.[field]).length,
    0,
  );
}

function diagnosticLevelLabel(level, t) {
  switch (level) {
    case 'full':
      return t('完整诊断');
    case 'minimal':
      return t('最小诊断');
    default:
      return t('只保留错误');
  }
}

function SmartSchedulerSummary({ setting, t }) {
  const cards = [
    {
      icon: <SlidersHorizontal size={15} />,
      label: t('当前调度模式'),
      value: setting?.default_mode || 'off',
      tone: setting?.default_mode === 'active' ? 'green' : 'grey',
    },
    {
      icon: <Activity size={15} />,
      label: t('智能调度'),
      value: setting?.enabled ? t('已启用') : t('未启用'),
      tone: setting?.enabled ? 'green' : 'grey',
    },
    {
      icon: <Gauge size={15} />,
      label: t('当前策略'),
      value: setting?.default_strategy || 'balanced',
      tone: 'blue',
    },
    {
      icon: <RefreshCw size={15} />,
      label: t('规则热更新'),
      value: setting?.upstream_error_classification_enabled
        ? `v${setting?.upstream_error_rule_version || DEFAULT_RULE_VERSION}`
        : t('未启用'),
      tone: setting?.upstream_error_classification_enabled ? 'green' : 'grey',
    },
    {
      icon: <RadioTower size={15} />,
      label: t('观测模式'),
      value: setting?.observability_performance_mode_enabled
        ? diagnosticLevelLabel(setting?.observability_diagnostic_level, t)
        : t('完整诊断'),
      tone: setting?.observability_performance_mode_enabled ? 'cyan' : 'orange',
    },
  ];

  return (
    <section className='aurora-source-grid'>
      {cards.map((card) => (
        <div className='aurora-source-item' key={card.label}>
          <span className='aurora-source-state'>
            {card.icon}
            {card.label}
          </span>
          <strong>{card.value}</strong>
          <Tag color={card.tone} size='small' type='light'>
            {card.label}
          </Tag>
        </div>
      ))}
    </section>
  );
}

function SchedulerPlaceholder({ icon, title, value, description }) {
  return (
    <section className='aurora-panel ct-admin-settings-embed'>
      <div className='ct-admin-embed-head'>
        <div className='ct-admin-embed-title'>
          <span className='ct-admin-embed-icon'>{icon}</span>
          <span>
            <strong>{title}</strong>
            <p>{description}</p>
          </span>
        </div>
        <span className='ct-admin-embed-badge'>{value}</span>
      </div>
    </section>
  );
}

function UpstreamErrorRulesPanel() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [setting, setSetting] = useState(DEFAULT_SETTING_PATCH);
  const [rules, setRules] = useState([]);
  const [defaults, setDefaults] = useState([]);
  const [kinds, setKinds] = useState([]);
  const [actions, setActions] = useState([]);
  const [keywordEditor, setKeywordEditor] = useState(null);

  const loadConfig = async () => {
    try {
      setLoading(true);
      const res = await API.get('/api/model_gateway/config', {
        disableDuplicate: true,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(t(message || '加载智能调度配置失败'));
        return;
      }
      const nextSetting = {
        ...DEFAULT_SETTING_PATCH,
        ...(data?.setting || {}),
      };
      setSetting(nextSetting);
      setRules(normalizeRules(nextSetting.upstream_error_rules));
      setDefaults(normalizeRules(data?.upstream_error_rule_defaults));
      setKinds(arrayValue(data?.upstream_error_kinds));
      setActions(arrayValue(data?.upstream_error_actions));
    } catch (error) {
      showError(t('加载智能调度配置失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfig();
  }, []);

  const kindOptions = useMemo(
    () =>
      (kinds.length ? kinds : ['rate_limit']).map((kind) => ({
        value: kind,
        label: kindLabel(kind, t),
      })),
    [kinds, t],
  );

  const actionOptions = useMemo(
    () =>
      (actions.length
        ? actions
        : ['switch_channel', 'stop', 'record_only']
      ).map((action) => ({
        value: action,
        label: actionLabel(action, t),
      })),
    [actions, t],
  );

  const updateRule = (id, patch) => {
    setRules((current) =>
      current.map((rule) => (rule.id === id ? { ...rule, ...patch } : rule)),
    );
  };

  const addRule = () => {
    setRules((current) => [
      ...current,
      normalizeRule({
        id: nextRuleID(current),
        enabled: true,
        priority: 10,
        kind: 'rate_limit',
        status_codes: [429],
        scheduler_action: 'switch_channel',
        avoidance_seconds: 60,
        description: t('自定义上游错误规则'),
      }),
    ]);
  };

  const duplicateRule = (rule) => {
    setRules((current) => [
      ...current,
      {
        ...normalizeRule(rule),
        id: nextRuleID(current),
        description: rule.description || t('复制规则'),
      },
    ]);
  };

  const disableRule = (rule) => {
    updateRule(rule.id, { enabled: false });
  };

  const saveConfig = async () => {
    try {
      setSaving(true);
      const payload = {
        ...setting,
        upstream_error_classification_enabled:
          setting.upstream_error_classification_enabled !== false,
        upstream_error_rule_version: DEFAULT_RULE_VERSION,
        upstream_error_rules: normalizeRules(rules),
      };
      const res = await API.put('/api/model_gateway/config', payload);
      const { success, message, data } = res.data;
      if (!success) {
        showError(t(message || '保存失败，请重试'));
        return;
      }
      const nextSetting = {
        ...DEFAULT_SETTING_PATCH,
        ...(data?.setting || payload),
      };
      setSetting(nextSetting);
      setRules(normalizeRules(nextSetting.upstream_error_rules));
      showSuccess(t('保存成功'));
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setSaving(false);
    }
  };

  const restoreDefaults = () => {
    Modal.confirm({
      title: t('恢复默认规则'),
      content: t('将上游错误分类规则恢复为系统默认规则，并立即保存。'),
      okText: t('确认'),
      cancelText: t('取消'),
      onOk: async () => {
        try {
          setSaving(true);
          const res = await API.post(
            '/api/model_gateway/config/reset',
            {},
            { params: { scope: 'upstream_error_rules' } },
          );
          const { success, message, data } = res.data;
          if (!success) {
            showError(t(message || '恢复默认规则失败'));
            return;
          }
          const nextSetting = {
            ...DEFAULT_SETTING_PATCH,
            ...(data?.setting || {}),
          };
          setSetting(nextSetting);
          setRules(normalizeRules(nextSetting.upstream_error_rules));
          setDefaults(normalizeRules(data?.upstream_error_rule_defaults));
          showSuccess(t('已恢复默认规则'));
        } catch (error) {
          showError(t('恢复默认规则失败'));
        } finally {
          setSaving(false);
        }
      },
    });
  };

  const openKeywordEditor = (rule) => {
    setKeywordEditor({
      ruleId: rule.id,
      keywords: normalizeKeywords(rule.keywords),
    });
  };

  const saveKeywordEditor = () => {
    if (!keywordEditor?.ruleId) return;
    updateRule(keywordEditor.ruleId, {
      keywords: normalizeKeywords(keywordEditor.keywords),
    });
    setKeywordEditor(null);
  };

  const columns = [
    {
      title: t('启用'),
      dataIndex: 'enabled',
      width: 70,
      render: (_, row) => (
        <Switch
          size='small'
          checked={row.enabled !== false}
          onChange={(checked) => updateRule(row.id, { enabled: checked })}
        />
      ),
    },
    {
      title: t('ID'),
      dataIndex: 'id',
      width: 180,
      render: (value, row) => (
        <Input
          size='small'
          value={value}
          onChange={(next) => updateRule(row.id, { id: next })}
        />
      ),
    },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      width: 95,
      render: (value, row) => (
        <InputNumber
          size='small'
          min={0}
          max={10000}
          value={value}
          onChange={(next) =>
            updateRule(row.id, { priority: Number(next) || 0 })
          }
        />
      ),
    },
    {
      title: 'kind',
      dataIndex: 'kind',
      width: 175,
      render: (value, row) => (
        <Select
          size='small'
          value={value}
          style={{ width: '100%' }}
          onChange={(next) => updateRule(row.id, { kind: next })}
        >
          {kindOptions.map((option) => (
            <Select.Option key={option.value} value={option.value}>
              {option.label}
            </Select.Option>
          ))}
        </Select>
      ),
    },
    {
      title: t('状态码'),
      dataIndex: 'status_codes',
      width: 220,
      render: (value, row) => (
        <Select
          multiple
          allowCreate
          filter
          size='small'
          value={statusSelectValue(value)}
          style={{ width: '100%' }}
          onChange={(next) =>
            updateRule(row.id, { status_codes: parseStatusValues(next) })
          }
        >
          {COMMON_STATUS_CODES.map((code) => (
            <Select.Option key={code} value={String(code)}>
              HTTP {code}
            </Select.Option>
          ))}
        </Select>
      ),
    },
    {
      title: t('关键词'),
      dataIndex: 'keywords',
      width: 170,
      render: (_, row) => (
        <Button size='small' onClick={() => openKeywordEditor(row)}>
          {t('编辑关键词')} ({ruleKeywordCount(row)})
        </Button>
      ),
    },
    {
      title: t('调度动作'),
      dataIndex: 'scheduler_action',
      width: 140,
      render: (value, row) => (
        <Select
          size='small'
          value={value}
          style={{ width: '100%' }}
          onChange={(next) => updateRule(row.id, { scheduler_action: next })}
        >
          {actionOptions.map((option) => (
            <Select.Option key={option.value} value={option.value}>
              {option.label}
            </Select.Option>
          ))}
        </Select>
      ),
    },
    {
      title: t('避让秒数'),
      dataIndex: 'avoidance_seconds',
      width: 105,
      render: (value, row) => (
        <InputNumber
          size='small'
          min={0}
          max={86400}
          value={value}
          onChange={(next) =>
            updateRule(row.id, { avoidance_seconds: Number(next) || 0 })
          }
        />
      ),
    },
    {
      title: t('说明'),
      dataIndex: 'description',
      width: 260,
      render: (value, row) => (
        <Input
          size='small'
          value={value}
          onChange={(next) => updateRule(row.id, { description: next })}
        />
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'actions',
      width: 150,
      fixed: 'right',
      render: (_, row) => (
        <Space spacing={6}>
          <Button
            icon={<Copy size={14} />}
            size='small'
            onClick={() => duplicateRule(row)}
          />
          <Button size='small' onClick={() => disableRule(row)}>
            {t('禁用')}
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <section className='aurora-panel ct-admin-settings-embed ct-smart-scheduler-errors'>
      <div className='ct-admin-embed-head'>
        <div className='ct-admin-embed-title'>
          <span className='ct-admin-embed-icon'>
            <ServerCrash size={16} />
          </span>
          <span>
            <strong>{t('上游错误分类')}</strong>
            <p>
              {t(
                '把上游余额、限速、容量、鉴权等限制类返回转成调度信号，优先提升请求成功率。',
              )}
            </p>
          </span>
        </div>
        <Tag
          color={
            setting.upstream_error_classification_enabled ? 'green' : 'grey'
          }
          type='light'
        >
          {setting.upstream_error_classification_enabled
            ? t('规则热更新')
            : t('未启用')}
        </Tag>
      </div>
      <div className='ct-admin-embed-body'>
        <Spin spinning={loading}>
          <div className='ct-smart-scheduler-toolbar'>
            <Space wrap>
              <Switch
                checked={
                  setting.upstream_error_classification_enabled !== false
                }
                onChange={(checked) =>
                  setSetting((current) => ({
                    ...current,
                    upstream_error_classification_enabled: checked,
                  }))
                }
              />
              <Typography.Text strong>{t('启用上游错误分类')}</Typography.Text>
              <Tag color='blue' type='light'>
                {t('规则版本')} v
                {setting.upstream_error_rule_version || DEFAULT_RULE_VERSION}
              </Tag>
              <Tag color='grey' type='light'>
                {t('规则数量')} {rules.length}
              </Tag>
            </Space>
            <Space wrap>
              <Button icon={<RefreshCw size={14} />} onClick={loadConfig}>
                {t('刷新配置')}
              </Button>
              <Button icon={<Plus size={14} />} onClick={addRule}>
                {t('新建规则')}
              </Button>
              <Button
                icon={<RotateCcw size={14} />}
                onClick={restoreDefaults}
                loading={saving}
              >
                {t('恢复默认规则')}
              </Button>
              <AdminPermissionButton
                dangerPermission={ADMIN_PERMISSION_KEYS.modelRoutePolicyDanger}
                icon={<Save size={14} />}
                loading={saving}
                type='primary'
                onClick={saveConfig}
              >
                {t('保存配置')}
              </AdminPermissionButton>
            </Space>
          </div>

          <Banner
            type='info'
            closeIcon={null}
            description={t(
              '一级错误分类由后端策略推导，前端只维护匹配条件和调度动作；规则只影响已进入上游返回处理的错误。',
            )}
          />

          <Table
            columns={columns}
            dataSource={rules}
            rowKey='id'
            pagination={false}
            scroll={{ x: 1560 }}
            size='small'
            style={{ marginTop: 14 }}
          />
        </Spin>
      </div>

      <Modal
        title={t('编辑关键词')}
        visible={Boolean(keywordEditor)}
        onCancel={() => setKeywordEditor(null)}
        onOk={saveKeywordEditor}
        okText={t('保存')}
        cancelText={t('取消')}
        width={720}
      >
        {keywordEditor ? (
          <div className='ct-smart-scheduler-keyword-grid'>
            {KEYWORD_FIELDS.map((field) => (
              <label key={field} className='ct-smart-scheduler-keyword-field'>
                <span>{field}</span>
                <TextArea
                  autosize={{ minRows: 3, maxRows: 6 }}
                  value={keywordText(keywordEditor.keywords[field])}
                  onChange={(value) =>
                    setKeywordEditor((current) => ({
                      ...current,
                      keywords: {
                        ...current.keywords,
                        [field]: parseKeywordText(value),
                      },
                    }))
                  }
                />
              </label>
            ))}
          </div>
        ) : null}
      </Modal>
    </section>
  );
}

const AdminSmartScheduler = () => {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const tabParam = searchParams.get('tab') || 'policy';
  const activeTab = VALID_TABS.includes(tabParam) ? tabParam : 'policy';
  const [summarySetting, setSummarySetting] = useState(DEFAULT_SETTING_PATCH);

  useEffect(() => {
    if (activeTab !== tabParam) {
      const next = new URLSearchParams(searchParams);
      next.set('tab', activeTab);
      setSearchParams(next, { replace: true });
    }
  }, [activeTab, tabParam, searchParams, setSearchParams]);

  useEffect(() => {
    let mounted = true;
    API.get('/api/model_gateway/config', { disableDuplicate: true })
      .then((res) => {
        if (!mounted || !res.data?.success) return;
        setSummarySetting({
          ...DEFAULT_SETTING_PATCH,
          ...(res.data?.data?.setting || {}),
        });
      })
      .catch(() => {});
    return () => {
      mounted = false;
    };
  }, []);

  const changeTab = (nextTab) => {
    const next = new URLSearchParams(searchParams);
    next.set('tab', nextTab);
    setSearchParams(next, { replace: false });
  };

  return (
    <div className='aurora-admin-page aurora-smart-scheduler-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('模型与路由')}</div>
          <h1>{t('智能调度')}</h1>
          <p>
            {t(
              '集中维护调度策略、上游错误分类、熔断避让、探测恢复和动态策略。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>{t('配置来源')} /api/model_gateway/config</span>
            <span>{t('复用路由策略权限')}</span>
          </div>
        </div>
        <div className='aurora-overview-status aurora-status-warning'>
          <span>{t('配置级别')}</span>
          <strong>{t('全局')}</strong>
          <em>{t('配置变更会影响实时调度')}</em>
        </div>
      </section>

      <SmartSchedulerSummary setting={summarySetting} t={t} />

      <section className='aurora-panel ct-admin-settings-embed ct-smart-scheduler-tabs'>
        <Tabs
          type='button'
          activeKey={activeTab}
          onChange={changeTab}
          keepDOM={false}
        >
          <Tabs.TabPane tab={t('调度策略')} itemKey='policy'>
            <SettingsModelGatewayScheduler />
          </Tabs.TabPane>
          <Tabs.TabPane tab={t('上游错误分类')} itemKey='upstream-errors'>
            <UpstreamErrorRulesPanel />
          </Tabs.TabPane>
          <Tabs.TabPane tab={t('熔断与避让')} itemKey='avoidance'>
            <SchedulerPlaceholder
              icon={<ShieldCheck size={16} />}
              title={t('熔断与避让')}
              value={t('v1 摘要')}
              description={t(
                '当前由调度策略中的熔断、失败避让、冷却与队列保护配置承载。',
              )}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab={t('探测恢复')} itemKey='probe'>
            <SchedulerPlaceholder
              icon={<RadioTower size={16} />}
              title={t('探测恢复')}
              value={t('探测配置')}
              description={t(
                '当前由调度策略中的健康探测、恢复探测和探测队列配置承载。',
              )}
            />
          </Tabs.TabPane>
          <Tabs.TabPane tab={t('动态策略')} itemKey='dynamic'>
            <SchedulerPlaceholder
              icon={<AlertTriangle size={16} />}
              title={t('动态策略')}
              value={t('策略集合')}
              description={t(
                '当前由动态倍率、成本优先、渠道优先级择优和粘滞路由配置承载。',
              )}
            />
          </Tabs.TabPane>
        </Tabs>
      </section>
    </div>
  );
};

export default AdminSmartScheduler;
