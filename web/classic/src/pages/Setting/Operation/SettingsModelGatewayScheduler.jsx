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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Col,
  Form,
  Input,
  Modal,
  Row,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDelete, IconPlus } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess, showWarning } from '../../../helpers';

const MODE_OPTIONS = ['off', 'shadow', 'active'];
const STRATEGY_OPTIONS = [
  'balanced',
  'speed_first',
  'cost_first',
  'stability_first',
];
const AUTO_MODE_OPTIONS = ['auto_sequential', 'auto_fusion'];

const DEFAULT_SETTING = {
  enabled: false,
  default_mode: 'off',
  rollout_percent: 0,
  default_strategy: 'balanced',
  sticky_ttl_seconds: 180,
  sticky_keep_score_ratio: 0.85,
  cache_affinity_enabled: true,
  cache_affinity_keep_score_ratio: 0.75,
  queue_enabled: true,
  queue_default_timeout_ms: 2000,
  queue_max_depth_per_channel: 64,
  queue_depth_multiplier: 2,
  circuit_breaker_enabled: true,
  circuit_failure_threshold: 0.5,
  circuit_min_samples: 10,
  circuit_open_seconds: 30,
  circuit_half_open_probe_count: 3,
  cooldown_max_seconds: 600,
  success_weight: 0.32,
  speed_weight: 0.28,
  load_weight: 0.2,
  cost_weight: 0.15,
  group_weight: 0.05,
  group_priority_ratio: {},
  group_policies: {},
  failure_fast_window_seconds: 60,
  failure_main_window_seconds: 300,
  failure_fallback_window_seconds: 1800,
};

const numberOrDefault = (value, fallback = 0) => {
  const next = Number(value);
  return Number.isFinite(next) ? next : fallback;
};

const makeSelectOptions = (values) =>
  (values || []).map((value) => ({ label: value, value }));

const normalizeSetting = (setting) => ({
  ...DEFAULT_SETTING,
  ...(setting || {}),
  group_priority_ratio: setting?.group_priority_ratio || {},
  group_policies: setting?.group_policies || {},
});

const policyMapToRows = (policies = {}, priorities = {}) =>
  Object.entries(policies)
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([group, policy], index) => ({
      id: `${group}-${index}`,
      group,
      priority_ratio:
        priorities[group] === undefined ? '' : String(priorities[group]),
      mode: policy?.mode || 'off',
      strategy: policy?.strategy || 'balanced',
      auto_mode: policy?.auto_mode || 'auto_sequential',
      cross_group_fusion: !!policy?.cross_group_fusion,
      candidate_groups_text: (policy?.candidate_groups || []).join('\n'),
      cache_affinity_enabled: !!policy?.cache_affinity_enabled,
      queue_enabled: !!policy?.queue_enabled,
      circuit_breaker_enabled: !!policy?.circuit_breaker_enabled,
    }));

const rowsToPolicyMaps = (rows) => {
  const policies = {};
  const priorities = {};
  for (const row of rows || []) {
    const group = (row.group || '').trim();
    if (!group) continue;
    const candidateGroups = (row.candidate_groups_text || '')
      .split('\n')
      .map((x) => x.trim())
      .filter(Boolean);
    policies[group] = {
      mode: row.mode || 'off',
      strategy: row.strategy || 'balanced',
      auto_mode: row.auto_mode || 'auto_sequential',
      cross_group_fusion: !!row.cross_group_fusion,
      candidate_groups: [...new Set(candidateGroups)],
      cache_affinity_enabled: !!row.cache_affinity_enabled,
      queue_enabled: !!row.queue_enabled,
      circuit_breaker_enabled: !!row.circuit_breaker_enabled,
    };
    const ratio = Number(row.priority_ratio);
    if (Number.isFinite(ratio) && ratio > 0) {
      priorities[group] = ratio;
    }
  }
  return { policies, priorities };
};

export default function SettingsModelGatewayScheduler() {
  const { t } = useTranslation();
  const refForm = useRef();
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [setting, setSetting] = useState(DEFAULT_SETTING);
  const [defaults, setDefaults] = useState(DEFAULT_SETTING);
  const [policyRows, setPolicyRows] = useState([]);
  const [meta, setMeta] = useState({
    modes: MODE_OPTIONS,
    strategies: STRATEGY_OPTIONS,
    auto_modes: AUTO_MODE_OPTIONS,
  });

  const modeOptions = useMemo(
    () => makeSelectOptions(meta.modes || MODE_OPTIONS),
    [meta.modes],
  );
  const strategyOptions = useMemo(
    () => makeSelectOptions(meta.strategies || STRATEGY_OPTIONS),
    [meta.strategies],
  );
  const autoModeOptions = useMemo(
    () => makeSelectOptions(meta.auto_modes || AUTO_MODE_OPTIONS),
    [meta.auto_modes],
  );

  const loadConfig = async () => {
    try {
      setLoading(true);
      const res = await API.get('/api/model_gateway/config', {
        disableDuplicate: true,
      });
      const { success, message, data } = res.data;
      if (!success) {
        showError(t(message));
        return;
      }
      const next = normalizeSetting(data?.setting);
      setSetting(next);
      setDefaults(normalizeSetting(data?.defaults));
      setMeta({
        modes: data?.modes || MODE_OPTIONS,
        strategies: data?.strategies || STRATEGY_OPTIONS,
        auto_modes: data?.auto_modes || AUTO_MODE_OPTIONS,
      });
      setPolicyRows(
        policyMapToRows(next.group_policies, next.group_priority_ratio),
      );
      refForm.current?.setValues(next);
    } catch (error) {
      showError(t('加载智能调度配置失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfig();
  }, []);

  const updateSetting = (field, value) => {
    setSetting((current) => ({ ...current, [field]: value }));
  };

  const updatePolicyRow = (id, patch) => {
    setPolicyRows((rows) =>
      rows.map((row) => (row.id === id ? { ...row, ...patch } : row)),
    );
  };

  const addPolicyRow = () => {
    setPolicyRows((rows) => [
      ...rows,
      {
        id: `new-${Date.now()}`,
        group: '',
        priority_ratio: '',
        mode: setting.default_mode || 'off',
        strategy: setting.default_strategy || 'balanced',
        auto_mode: 'auto_fusion',
        cross_group_fusion: false,
        candidate_groups_text: '',
        cache_affinity_enabled: setting.cache_affinity_enabled,
        queue_enabled: setting.queue_enabled,
        circuit_breaker_enabled: setting.circuit_breaker_enabled,
      },
    ]);
  };

  const removePolicyRow = (id) => {
    setPolicyRows((rows) => rows.filter((row) => row.id !== id));
  };

  const buildPayload = () => {
    const { policies, priorities } = rowsToPolicyMaps(policyRows);
    return {
      ...setting,
      rollout_percent: numberOrDefault(setting.rollout_percent),
      sticky_ttl_seconds: numberOrDefault(setting.sticky_ttl_seconds, 180),
      sticky_keep_score_ratio: numberOrDefault(
        setting.sticky_keep_score_ratio,
        0.85,
      ),
      cache_affinity_keep_score_ratio: numberOrDefault(
        setting.cache_affinity_keep_score_ratio,
        0.75,
      ),
      queue_default_timeout_ms: numberOrDefault(
        setting.queue_default_timeout_ms,
        2000,
      ),
      queue_max_depth_per_channel: numberOrDefault(
        setting.queue_max_depth_per_channel,
        64,
      ),
      queue_depth_multiplier: numberOrDefault(
        setting.queue_depth_multiplier,
        2,
      ),
      circuit_failure_threshold: numberOrDefault(
        setting.circuit_failure_threshold,
        0.5,
      ),
      circuit_min_samples: numberOrDefault(setting.circuit_min_samples, 10),
      circuit_open_seconds: numberOrDefault(setting.circuit_open_seconds, 30),
      circuit_half_open_probe_count: numberOrDefault(
        setting.circuit_half_open_probe_count,
        3,
      ),
      cooldown_max_seconds: numberOrDefault(setting.cooldown_max_seconds, 600),
      success_weight: numberOrDefault(setting.success_weight, 0.32),
      speed_weight: numberOrDefault(setting.speed_weight, 0.28),
      load_weight: numberOrDefault(setting.load_weight, 0.2),
      cost_weight: numberOrDefault(setting.cost_weight, 0.15),
      group_weight: numberOrDefault(setting.group_weight, 0.05),
      failure_fast_window_seconds: numberOrDefault(
        setting.failure_fast_window_seconds,
        60,
      ),
      failure_main_window_seconds: numberOrDefault(
        setting.failure_main_window_seconds,
        300,
      ),
      failure_fallback_window_seconds: numberOrDefault(
        setting.failure_fallback_window_seconds,
        1800,
      ),
      group_priority_ratio: priorities,
      group_policies: policies,
    };
  };

  const saveConfig = async () => {
    const groupNames = policyRows
      .map((row) => (row.group || '').trim())
      .filter(Boolean);
    if (new Set(groupNames).size !== groupNames.length) {
      showError(t('分组策略中存在重复分组'));
      return;
    }
    try {
      setSaving(true);
      const res = await API.put('/api/model_gateway/config', buildPayload());
      const { success, message, data } = res.data;
      if (!success) {
        showError(t(message));
        return;
      }
      showSuccess(t('保存成功'));
      const next = normalizeSetting(data?.setting);
      setSetting(next);
      setPolicyRows(
        policyMapToRows(next.group_policies, next.group_priority_ratio),
      );
      refForm.current?.setValues(next);
    } catch (error) {
      showError(t('保存失败，请重试'));
    } finally {
      setSaving(false);
    }
  };

  const resetConfig = () => {
    Modal.confirm({
      title: t('恢复智能调度默认配置'),
      content: t('将覆盖当前智能调度配置，并保留其它系统设置不变。'),
      onOk: async () => {
        try {
          setSaving(true);
          const res = await API.post('/api/model_gateway/config/reset');
          const { success, message, data } = res.data;
          if (!success) {
            showError(t(message));
            return;
          }
          showSuccess(t('已恢复默认配置'));
          const next = normalizeSetting(data?.setting || defaults);
          setSetting(next);
          setPolicyRows(
            policyMapToRows(next.group_policies, next.group_priority_ratio),
          );
          refForm.current?.setValues(next);
        } catch (error) {
          showError(t('恢复默认配置失败'));
        } finally {
          setSaving(false);
        }
      },
    });
  };

  const policyColumns = [
    {
      title: t('分组'),
      dataIndex: 'group',
      width: 150,
      render: (_, row) => (
        <Input
          value={row.group}
          placeholder='vip'
          onChange={(value) => updatePolicyRow(row.id, { group: value })}
        />
      ),
    },
    {
      title: t('模式'),
      dataIndex: 'mode',
      width: 140,
      render: (_, row) => (
        <Select
          value={row.mode}
          optionList={modeOptions}
          onChange={(value) => updatePolicyRow(row.id, { mode: value })}
        />
      ),
    },
    {
      title: t('策略'),
      dataIndex: 'strategy',
      width: 170,
      render: (_, row) => (
        <Select
          value={row.strategy}
          optionList={strategyOptions}
          onChange={(value) => updatePolicyRow(row.id, { strategy: value })}
        />
      ),
    },
    {
      title: t('auto 模式'),
      dataIndex: 'auto_mode',
      width: 170,
      render: (_, row) => (
        <Select
          value={row.auto_mode}
          optionList={autoModeOptions}
          onChange={(value) => updatePolicyRow(row.id, { auto_mode: value })}
        />
      ),
    },
    {
      title: t('跨分组融合'),
      dataIndex: 'cross_group_fusion',
      width: 120,
      render: (_, row) => (
        <Switch
          checked={row.cross_group_fusion}
          onChange={(value) =>
            updatePolicyRow(row.id, { cross_group_fusion: value })
          }
        />
      ),
    },
    {
      title: t('候选分组'),
      dataIndex: 'candidate_groups_text',
      width: 220,
      render: (_, row) => (
        <Input
          value={(row.candidate_groups_text || '').replace(/\n/g, ', ')}
          placeholder='default, vip'
          onChange={(value) =>
            updatePolicyRow(row.id, {
              candidate_groups_text: value
                .split(',')
                .map((x) => x.trim())
                .filter(Boolean)
                .join('\n'),
            })
          }
        />
      ),
    },
    {
      title: t('分组倍率'),
      dataIndex: 'priority_ratio',
      width: 120,
      render: (_, row) => (
        <Input
          value={row.priority_ratio}
          placeholder='1.0'
          onChange={(value) =>
            updatePolicyRow(row.id, { priority_ratio: value })
          }
        />
      ),
    },
    {
      title: t('能力'),
      dataIndex: 'capabilities',
      width: 240,
      render: (_, row) => (
        <Space wrap>
          <Tag
            color={row.cache_affinity_enabled ? 'cyan' : 'grey'}
            onClick={() =>
              updatePolicyRow(row.id, {
                cache_affinity_enabled: !row.cache_affinity_enabled,
              })
            }
          >
            {t('缓存亲和')}
          </Tag>
          <Tag
            color={row.queue_enabled ? 'blue' : 'grey'}
            onClick={() =>
              updatePolicyRow(row.id, { queue_enabled: !row.queue_enabled })
            }
          >
            {t('队列')}
          </Tag>
          <Tag
            color={row.circuit_breaker_enabled ? 'orange' : 'grey'}
            onClick={() =>
              updatePolicyRow(row.id, {
                circuit_breaker_enabled: !row.circuit_breaker_enabled,
              })
            }
          >
            {t('熔断')}
          </Tag>
        </Space>
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'action',
      width: 90,
      fixed: 'right',
      render: (_, row) => (
        <Button
          type='danger'
          theme='borderless'
          icon={<IconDelete />}
          onClick={() => removePolicyRow(row.id)}
        />
      ),
    },
  ];

  return (
    <Spin spinning={loading || saving}>
      <Form
        values={setting}
        getFormApi={(formApi) => (refForm.current = formApi)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('智能调度配置')}>
          <Banner
            type='info'
            fullMode={false}
            closeIcon={null}
            title={t('按分组开启智能调度')}
            description={t(
              '默认保持关闭；只有配置为 shadow 或 active 的分组会进入智能调度逻辑。',
            )}
            style={{ marginBottom: 16 }}
          />
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8}>
              <Form.Switch
                field='enabled'
                label={t('启用智能调度配置')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) => updateSetting('enabled', value)}
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.Select
                field='default_mode'
                label={t('默认模式')}
                optionList={modeOptions}
                onChange={(value) => updateSetting('default_mode', value)}
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                field='rollout_percent'
                label={t('灰度比例')}
                min={0}
                max={100}
                suffix='%'
                onChange={(value) =>
                  updateSetting('rollout_percent', numberOrDefault(value))
                }
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8}>
              <Form.Select
                field='default_strategy'
                label={t('默认策略')}
                optionList={strategyOptions}
                onChange={(value) => updateSetting('default_strategy', value)}
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.Switch
                field='cache_affinity_enabled'
                label={t('缓存亲和')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting('cache_affinity_enabled', value)
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.Switch
                field='queue_enabled'
                label={t('队列等待')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) => updateSetting('queue_enabled', value)}
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8}>
              <Form.Switch
                field='circuit_breaker_enabled'
                label={t('熔断')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting('circuit_breaker_enabled', value)
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                field='sticky_ttl_seconds'
                label={t('粘滞 TTL')}
                min={1}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting('sticky_ttl_seconds', numberOrDefault(value))
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                field='sticky_keep_score_ratio'
                label={t('粘滞保留阈值')}
                min={0.01}
                max={1}
                step={0.01}
                onChange={(value) =>
                  updateSetting(
                    'sticky_keep_score_ratio',
                    numberOrDefault(value, 0.85),
                  )
                }
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='queue_default_timeout_ms'
                label={t('队列等待超时')}
                min={1}
                suffix='ms'
                onChange={(value) =>
                  updateSetting(
                    'queue_default_timeout_ms',
                    numberOrDefault(value, 2000),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='queue_max_depth_per_channel'
                label={t('单渠道最大队列')}
                min={1}
                onChange={(value) =>
                  updateSetting(
                    'queue_max_depth_per_channel',
                    numberOrDefault(value, 64),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='circuit_failure_threshold'
                label={t('熔断失败率阈值')}
                min={0.01}
                max={1}
                step={0.01}
                onChange={(value) =>
                  updateSetting(
                    'circuit_failure_threshold',
                    numberOrDefault(value, 0.5),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='circuit_open_seconds'
                label={t('熔断打开时间')}
                min={1}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting(
                    'circuit_open_seconds',
                    numberOrDefault(value, 30),
                  )
                }
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='success_weight'
                label={t('成功率权重')}
                min={0}
                step={0.01}
                onChange={(value) =>
                  updateSetting('success_weight', numberOrDefault(value))
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='speed_weight'
                label={t('速度权重')}
                min={0}
                step={0.01}
                onChange={(value) =>
                  updateSetting('speed_weight', numberOrDefault(value))
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='load_weight'
                label={t('负载权重')}
                min={0}
                step={0.01}
                onChange={(value) =>
                  updateSetting('load_weight', numberOrDefault(value))
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='cost_weight'
                label={t('成本权重')}
                min={0}
                step={0.01}
                onChange={(value) =>
                  updateSetting('cost_weight', numberOrDefault(value))
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='group_weight'
                label={t('分组权重')}
                min={0}
                step={0.01}
                onChange={(value) =>
                  updateSetting('group_weight', numberOrDefault(value))
                }
              />
            </Col>
          </Row>
        </Form.Section>
      </Form>

      <div style={{ marginTop: 8, marginBottom: 12 }}>
        <Space>
          <Button type='primary' onClick={saveConfig} loading={saving}>
            {t('保存智能调度配置')}
          </Button>
          <Button onClick={loadConfig}>{t('刷新配置')}</Button>
          <Button type='danger' theme='borderless' onClick={resetConfig}>
            {t('恢复默认配置')}
          </Button>
        </Space>
      </div>

      <div style={{ marginTop: 12 }}>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: 8,
            gap: 12,
          }}
        >
          <Typography.Title heading={6} style={{ margin: 0 }}>
            {t('分组智能调度策略')}
          </Typography.Title>
          <Button icon={<IconPlus />} onClick={addPolicyRow}>
            {t('新增分组策略')}
          </Button>
        </div>
        <Table
          size='small'
          columns={policyColumns}
          dataSource={policyRows}
          rowKey='id'
          pagination={false}
          empty={
            <div style={{ padding: 24, color: 'var(--semi-color-text-2)' }}>
              {t('暂无分组智能调度策略')}
            </div>
          }
          scroll={{ x: 1520 }}
        />
      </div>
    </Spin>
  );
}
