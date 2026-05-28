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

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Banner,
  Button,
  Checkbox,
  Col,
  Dropdown,
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
import { IconChevronDown, IconDelete, IconPlus } from '@douyinfe/semi-icons';
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
const CIRCUIT_ERROR_TYPES = [
  'stream_interrupted',
  'rate_limit',
  'auth',
  'quota',
  'server_error',
  'upstream_error',
];
const STICKY_FAILURE_POLICY_OPTIONS = ['clear', 'keep'];
const BILLING_RATIO_MODE_OPTIONS = ['static', 'dynamic'];
const PROXY_REUSE_POLICY_OPTIONS = ['warn', 'confirm', 'block'];

const DEFAULT_SETTING = {
  enabled: false,
  runtime_sync_event_subscribe_enabled: false,
  probe_enabled: true,
  probe_interval_seconds: 60,
  probe_worker_count: 2,
  probe_timeout_seconds: 8,
  probe_max_per_tick: 5,
  probe_min_channel_interval_seconds: 300,
  probe_low_score_threshold: 0.62,
  probe_missing_sample_threshold: 3,
  probe_long_no_success_seconds: 1800,
  probe_recovery_successes_required: 2,
  probe_failure_avoidance_priority_enabled: true,
  cost_calculation_enabled: true,
  cost_calculation_interval_seconds: 5,
  cost_calculation_worker_count: 2,
  cost_calculation_batch_size: 100,
  dynamic_billing_enabled: false,
  dynamic_billing_profit_rate: 0.2,
  dynamic_billing_window_samples: 300,
  dynamic_billing_min_samples: 5,
  dynamic_billing_refresh_seconds: 30,
  dynamic_billing_max_age_seconds: 300,
  default_mode: 'off',
  rollout_percent: 0,
  default_strategy: 'balanced',
  sticky_ttl_seconds: 180,
  sticky_keep_score_ratio: 0.85,
  sticky_save_on_select: false,
  sticky_renew_on_success: true,
  sticky_failure_policy: 'clear',
  cache_affinity_enabled: true,
  cache_affinity_keep_score_ratio: 0.75,
  cost_first_sticky_escape_enabled: true,
  cost_first_sticky_escape_cost_ratio: 0.75,
  cost_first_sticky_escape_cache_cost_ratio: 0.55,
  cost_first_sticky_escape_max_speed_score_drop: 0.06,
  cost_first_sticky_escape_cache_max_speed_score_drop: 0.03,
  cost_first_sticky_escape_min_samples: 5,
  cost_first_sticky_escape_success_slack: 0.02,
  cost_first_guard_enabled: true,
  cost_first_guard_multiple: 1.8,
  cost_first_guard_success_advantage: 0.03,
  cost_first_guard_speed_advantage: 0.08,
  queue_enabled: true,
  queue_default_timeout_ms: 2000,
  queue_max_depth_per_channel: 64,
  queue_depth_multiplier: 2,
  queue_high_priority_threshold: 0,
  queue_high_priority_extra_depth: 0,
  queue_high_priority_reserved_depth: 0,
  queue_absolute_max_depth: 0,
  circuit_breaker_enabled: true,
  circuit_failure_threshold: 0.5,
  circuit_min_samples: 10,
  circuit_open_seconds: 30,
  circuit_half_open_probe_count: 3,
  circuit_error_policies: {},
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
  proxy_same_brand_reuse_policy: 'warn',
};

const numberOrDefault = (value, fallback = 0) => {
  const next = Number(value);
  return Number.isFinite(next) ? next : fallback;
};

const formatRatioValue = (value, digits = 4) => {
  const next = Number(value);
  if (!Number.isFinite(next)) {
    return '-';
  }
  return `${parseFloat(next.toFixed(digits))}x`;
};

const formatPricePerMillion = (value) => {
  const next = Number(value);
  if (!Number.isFinite(next) || next <= 0) {
    return '-';
  }
  return `$${parseFloat(next.toFixed(6))}/M`;
};

const formatStaticBillingRatio = (group, ratio, t) => {
  if (String(group || '').trim() === 'auto' && !Number.isFinite(Number(ratio))) {
    return t('按实际分组');
  }
  return formatRatioValue(ratio);
};

const makeSelectOptions = (values, labeler = (value) => value) =>
  (values || []).map((value) => ({ label: labeler(value), value }));

const normalizeCandidateGroups = (value) => {
  if (Array.isArray(value)) {
    return [...new Set(value.map((x) => String(x).trim()).filter(Boolean))];
  }
  return [
    ...new Set(
      String(value || '')
        .split(/[\n,]/)
        .map((x) => x.trim())
        .filter(Boolean),
    ),
  ];
};

const makeGroupOption = (group, info = {}) => ({
  label: group,
  value: group,
  description: info?.desc || group,
  ratio: info?.ratio,
});

const parseGroupOptions = (data) => {
  if (Array.isArray(data)) {
    return data.map((group) => makeGroupOption(group));
  }
  return Object.entries(data || {}).map(([group, info]) =>
    makeGroupOption(group, info),
  );
};

const mergeGroupOptions = (...optionGroups) => {
  const optionMap = new Map();
  for (const options of optionGroups) {
    for (const option of options || []) {
      if (!option?.value) continue;
      optionMap.set(option.value, {
        ...(optionMap.get(option.value) || makeGroupOption(option.value)),
        ...option,
      });
    }
  }
  return Array.from(optionMap.values());
};

const renderGroupOptionItem = (option, t) => (
  <div style={{ minWidth: 240 }}>
    <div
      style={{
        display: 'flex',
        justifyContent: 'space-between',
        gap: 12,
      }}
    >
      <Typography.Text strong>{option.value}</Typography.Text>
      {option.ratio !== undefined && (
        <Typography.Text type='tertiary' size='small'>
          {t('分组倍率')} {option.ratio}
        </Typography.Text>
      )}
    </div>
    {option.description && option.description !== option.value && (
      <Typography.Text type='tertiary' size='small' ellipsis>
        {option.description}
      </Typography.Text>
    )}
  </div>
);

const CandidateGroupsEditor = ({
  value,
  optionList,
  placeholder,
  onChange,
  t,
}) => {
  const selectedGroups = normalizeCandidateGroups(value);
  const selectedSet = new Set(selectedGroups);

  const toggleGroup = (group) => {
    if (!group) return;
    if (selectedSet.has(group)) {
      onChange(selectedGroups.filter((item) => item !== group));
      return;
    }
    onChange([...selectedGroups, group]);
  };

  const removeGroup = (group, event) => {
    event?.stopPropagation?.();
    onChange(selectedGroups.filter((item) => item !== group));
  };

  const clearGroups = (event) => {
    event?.stopPropagation?.();
    onChange([]);
  };

  const dropdownContent = (
    <div
      style={{
        width: 320,
        maxHeight: 280,
        overflowY: 'auto',
        padding: 8,
      }}
      onClick={(event) => event.stopPropagation()}
    >
      {optionList.length ? (
        optionList.map((option) => {
          const group = option.value;
          const checked = selectedSet.has(group);
          return (
            <div
              key={group}
              role='button'
              tabIndex={0}
              style={{
                display: 'flex',
                alignItems: 'flex-start',
                gap: 8,
                padding: '8px 10px',
                borderRadius: 6,
                cursor: 'pointer',
              }}
              onClick={() => toggleGroup(group)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' || event.key === ' ') {
                  event.preventDefault();
                  toggleGroup(group);
                }
              }}
            >
              <Checkbox checked={checked} style={{ pointerEvents: 'none' }} />
              <div style={{ flex: 1, minWidth: 0 }}>
                {renderGroupOptionItem(option, t)}
              </div>
            </div>
          );
        })
      ) : (
        <Typography.Text type='tertiary'>{t('暂无数据')}</Typography.Text>
      )}
    </div>
  );

  return (
    <Dropdown trigger='click' position='bottomLeft' render={dropdownContent}>
      <div
        role='button'
        tabIndex={0}
        style={{
          minHeight: 32,
          width: '100%',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 8,
          padding: '4px 8px',
          borderRadius: 6,
          background: 'var(--semi-color-fill-0)',
          border: '1px solid var(--semi-color-border)',
          cursor: 'pointer',
        }}
      >
        <Space wrap spacing={4} style={{ flex: 1, minWidth: 0 }}>
          {selectedGroups.length ? (
            selectedGroups.map((group) => (
              <Tag
                key={group}
                size='small'
                color='blue'
                closable
                onClose={(_, event) => removeGroup(group, event)}
              >
                {group}
              </Tag>
            ))
          ) : (
            <Typography.Text type='tertiary'>{placeholder}</Typography.Text>
          )}
        </Space>
        <Space spacing={4}>
          {selectedGroups.length ? (
            <Typography.Text
              type='tertiary'
              onClick={clearGroups}
              style={{ cursor: 'pointer' }}
            >
              x
            </Typography.Text>
          ) : null}
          <IconChevronDown size='small' />
        </Space>
      </div>
    </Dropdown>
  );
};

const schedulerModeLabel = (mode, t) => {
  switch (mode) {
    case 'off':
      return t('智能调度模式：off');
    case 'shadow':
      return t('智能调度模式：shadow');
    case 'active':
      return t('智能调度模式：active');
    default:
      return mode;
  }
};

const schedulerStrategyLabel = (strategy, t) => {
  switch (strategy) {
    case 'balanced':
      return t('智能调度策略：balanced');
    case 'speed_first':
      return t('智能调度策略：speed_first');
    case 'cost_first':
      return t('智能调度策略：cost_first');
    case 'stability_first':
      return t('智能调度策略：stability_first');
    default:
      return strategy;
  }
};

const schedulerAutoModeLabel = (mode, t) => {
  switch (mode) {
    case 'auto_sequential':
      return t('智能调度 auto 模式：auto_sequential');
    case 'auto_fusion':
      return t('智能调度 auto 模式：auto_fusion');
    default:
      return mode;
  }
};

const circuitErrorTypeLabel = (type, t) => {
  switch (type) {
    case 'stream_interrupted':
      return t('熔断错误类型：stream_interrupted');
    case 'rate_limit':
      return t('熔断错误类型：rate_limit');
    case 'auth':
      return t('熔断错误类型：auth');
    case 'quota':
      return t('熔断错误类型：quota');
    case 'server_error':
      return t('熔断错误类型：server_error');
    case 'upstream_error':
      return t('熔断错误类型：upstream_error');
    default:
      return type;
  }
};

const stickyFailurePolicyLabel = (policy, t) => {
  switch (policy) {
    case 'clear':
      return t('粘滞失败策略：clear');
    case 'keep':
      return t('粘滞失败策略：keep');
    default:
      return policy;
  }
};

const billingRatioModeLabel = (mode, t) => {
  switch (mode) {
    case 'dynamic':
      return t('动态收费倍率');
    case 'static':
      return t('静态收费倍率');
    default:
      return mode;
  }
};

const proxyReusePolicyLabel = (policy, t) => {
  switch (policy) {
    case 'warn':
      return t('代理复用策略：仅提醒');
    case 'confirm':
      return t('代理复用策略：二次确认');
    case 'block':
      return t('代理复用策略：禁止复用');
    default:
      return policy;
  }
};

const makeCircuitErrorRows = (policies = {}) =>
  CIRCUIT_ERROR_TYPES.map((type) => {
    const policy = policies?.[type] || {};
    return {
      type,
      enabled: !!policies?.[type],
      failure_threshold:
        policy.failure_threshold === undefined
          ? ''
          : String(policy.failure_threshold),
      min_samples:
        policy.min_samples === undefined ? '' : String(policy.min_samples),
      open_seconds:
        policy.open_seconds === undefined ? '' : String(policy.open_seconds),
      half_open_probe_count:
        policy.half_open_probe_count === undefined
          ? ''
          : String(policy.half_open_probe_count),
    };
  });

const circuitErrorRowsToPolicyMap = (rows) => {
  const policies = {};
  for (const row of rows || []) {
    if (!row.enabled) continue;
    policies[row.type] = {
      failure_threshold: numberOrDefault(row.failure_threshold, 0.5),
      min_samples: numberOrDefault(row.min_samples, 1),
      open_seconds: numberOrDefault(row.open_seconds, 30),
      half_open_probe_count: numberOrDefault(row.half_open_probe_count, 1),
    };
  }
  return policies;
};

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
      billing_ratio_mode: policy?.billing_ratio_mode || 'static',
      mode: policy?.mode || 'off',
      strategy: policy?.strategy || 'balanced',
      auto_mode: policy?.auto_mode || 'auto_sequential',
      cross_group_fusion: !!policy?.cross_group_fusion,
      candidate_groups: normalizeCandidateGroups(policy?.candidate_groups),
      cache_affinity_enabled: !!policy?.cache_affinity_enabled,
      queue_enabled: !!policy?.queue_enabled,
      queue_high_priority: !!policy?.queue_high_priority,
      circuit_breaker_enabled: !!policy?.circuit_breaker_enabled,
    }));

const rowsToPolicyMaps = (rows) => {
  const policies = {};
  const priorities = {};
  for (const row of rows || []) {
    const group = (row.group || '').trim();
    if (!group) continue;
    const candidateGroups = normalizeCandidateGroups(
      row.candidate_groups ?? row.candidate_groups_text,
    );
    policies[group] = {
      mode: row.mode || 'off',
      strategy: row.strategy || 'balanced',
      auto_mode: row.auto_mode || 'auto_sequential',
      cross_group_fusion: !!row.cross_group_fusion,
      candidate_groups: [...new Set(candidateGroups)],
      billing_ratio_mode: row.billing_ratio_mode || 'static',
      cache_affinity_enabled: !!row.cache_affinity_enabled,
      queue_enabled: !!row.queue_enabled,
      queue_high_priority: !!row.queue_high_priority,
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
  const [circuitErrorRows, setCircuitErrorRows] = useState(
    makeCircuitErrorRows(DEFAULT_SETTING.circuit_error_policies),
  );
  const [meta, setMeta] = useState({
    modes: MODE_OPTIONS,
    strategies: STRATEGY_OPTIONS,
    auto_modes: AUTO_MODE_OPTIONS,
  });
  const [groupOptions, setGroupOptions] = useState([]);
  const [dynamicBillingBaselines, setDynamicBillingBaselines] = useState([]);

  const modeOptions = useMemo(
    () =>
      makeSelectOptions(meta.modes || MODE_OPTIONS, (value) =>
        schedulerModeLabel(value, t),
      ),
    [meta.modes, t],
  );
  const strategyOptions = useMemo(
    () =>
      makeSelectOptions(meta.strategies || STRATEGY_OPTIONS, (value) =>
        schedulerStrategyLabel(value, t),
      ),
    [meta.strategies, t],
  );
  const autoModeOptions = useMemo(
    () =>
      makeSelectOptions(meta.auto_modes || AUTO_MODE_OPTIONS, (value) =>
        schedulerAutoModeLabel(value, t),
      ),
    [meta.auto_modes, t],
  );
  const stickyFailurePolicyOptions = useMemo(
    () =>
      STICKY_FAILURE_POLICY_OPTIONS.map((value) => ({
        value,
        label: stickyFailurePolicyLabel(value, t),
      })),
    [t],
  );
  const billingRatioModeOptions = useMemo(
    () =>
      BILLING_RATIO_MODE_OPTIONS.map((value) => ({
        value,
        label: billingRatioModeLabel(value, t),
      })),
    [t],
  );
  const proxyReusePolicyOptions = useMemo(
    () =>
      PROXY_REUSE_POLICY_OPTIONS.map((value) => ({
        value,
        label: proxyReusePolicyLabel(value, t),
      })),
    [t],
  );
  const candidateGroupOptions = useMemo(() => {
    const optionMap = new Map();
    for (const option of groupOptions || []) {
      if (!option?.value) continue;
      optionMap.set(option.value, option);
    }
    for (const row of policyRows || []) {
      const group = String(row.group || '').trim();
      if (group && !optionMap.has(group)) {
        optionMap.set(group, makeGroupOption(group));
      }
      for (const group of normalizeCandidateGroups(row.candidate_groups)) {
        if (!optionMap.has(group)) {
          optionMap.set(group, makeGroupOption(group));
        }
      }
    }
    return Array.from(optionMap.values());
  }, [groupOptions, policyRows]);
  const groupRatioMap = useMemo(() => {
    const ratioMap = new Map();
    for (const option of candidateGroupOptions || []) {
      if (!option?.value) continue;
      const ratio = Number(option.ratio);
      if (Number.isFinite(ratio)) {
        ratioMap.set(option.value, ratio);
      }
    }
    return ratioMap;
  }, [candidateGroupOptions]);
  const dynamicBillingByGroup = useMemo(() => {
    const result = new Map();
    for (const item of dynamicBillingBaselines || []) {
      const group = String(item?.group || '').trim();
      const ratio = Number(item?.ratio);
      if (!group || !Number.isFinite(ratio) || ratio <= 0) continue;
      const pricePerM = Number(item?.price_per_m);
      result.set(group, {
        ratio,
        pricePerM: Number.isFinite(pricePerM) && pricePerM > 0 ? pricePerM : 0,
        sampleCount: Number(item?.sample_count) || 0,
        modelCount: Number(item?.model_count) || 0,
        referenceModel: String(
          item?.reference_model || item?.requested_model || '',
        ).trim(),
        latestCalculatedAt: Number(item?.calculated_at) || 0,
        group,
      });
    }
    return result;
  }, [dynamicBillingBaselines]);

  const resolveDynamicBillingInfo = useCallback(
    (row) => {
      if (!row) return null;
      const directGroup = String(row.group || '').trim();
      const candidates = normalizeCandidateGroups(row.candidate_groups);
      const searchGroups = [
        directGroup,
        ...candidates.filter((group) => group !== directGroup),
      ];
      let best = null;
      for (const group of searchGroups) {
        const info = dynamicBillingByGroup.get(group);
        if (!info) continue;
        if (
          !best ||
          Number(info.latestCalculatedAt || 0) >
            Number(best.latestCalculatedAt || 0) ||
          (Number(info.latestCalculatedAt || 0) ===
            Number(best.latestCalculatedAt || 0) &&
            Number(info.sampleCount || 0) > Number(best.sampleCount || 0))
        ) {
          best = info;
        }
      }
      return best;
    },
    [dynamicBillingByGroup],
  );

  const loadGroups = async () => {
    try {
      const [adminRes, userRes] = await Promise.all([
        API.get('/api/group/', {
          disableDuplicate: true,
        }),
        API.get('/api/user/self/groups', {
          disableDuplicate: true,
        }).catch(() => undefined),
      ]);
      const { success, data, message } = adminRes.data;
      if (!success) {
        showWarning(t(message || '加载分组失败'));
        return;
      }
      const adminOptions = parseGroupOptions(data);
      const userOptions = userRes?.data?.success
        ? parseGroupOptions(userRes.data.data)
        : [];
      setGroupOptions(mergeGroupOptions(adminOptions, userOptions));
    } catch (error) {
      showWarning(t('加载分组失败'));
    }
  };

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
      setDynamicBillingBaselines(data?.dynamic_billing_baselines || []);
      setPolicyRows(
        policyMapToRows(next.group_policies, next.group_priority_ratio),
      );
      setCircuitErrorRows(makeCircuitErrorRows(next.circuit_error_policies));
      refForm.current?.setValues(next);
    } catch (error) {
      showError(t('加载智能调度配置失败'));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfig();
    loadGroups();
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
        candidate_groups: [],
        billing_ratio_mode: 'static',
        cache_affinity_enabled: setting.cache_affinity_enabled,
        queue_enabled: setting.queue_enabled,
        queue_high_priority: false,
        circuit_breaker_enabled: setting.circuit_breaker_enabled,
      },
    ]);
  };

  const removePolicyRow = (id) => {
    setPolicyRows((rows) => rows.filter((row) => row.id !== id));
  };

  const updateCircuitErrorRow = (type, patch) => {
    setCircuitErrorRows((rows) =>
      rows.map((row) => (row.type === type ? { ...row, ...patch } : row)),
    );
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
      sticky_failure_policy: setting.sticky_failure_policy || 'clear',
      cache_affinity_keep_score_ratio: numberOrDefault(
        setting.cache_affinity_keep_score_ratio,
        0.75,
      ),
      cost_first_sticky_escape_enabled:
        setting.cost_first_sticky_escape_enabled !== false,
      cost_first_sticky_escape_cost_ratio: numberOrDefault(
        setting.cost_first_sticky_escape_cost_ratio,
        0.75,
      ),
      cost_first_sticky_escape_cache_cost_ratio: numberOrDefault(
        setting.cost_first_sticky_escape_cache_cost_ratio,
        0.55,
      ),
      cost_first_sticky_escape_max_speed_score_drop: numberOrDefault(
        setting.cost_first_sticky_escape_max_speed_score_drop,
        0.06,
      ),
      cost_first_sticky_escape_cache_max_speed_score_drop: numberOrDefault(
        setting.cost_first_sticky_escape_cache_max_speed_score_drop,
        0.03,
      ),
      cost_first_sticky_escape_min_samples: numberOrDefault(
        setting.cost_first_sticky_escape_min_samples,
        5,
      ),
      cost_first_sticky_escape_success_slack: numberOrDefault(
        setting.cost_first_sticky_escape_success_slack,
        0.02,
      ),
      cost_first_guard_enabled: setting.cost_first_guard_enabled !== false,
      cost_first_guard_multiple: numberOrDefault(
        setting.cost_first_guard_multiple,
        1.8,
      ),
      cost_first_guard_success_advantage: numberOrDefault(
        setting.cost_first_guard_success_advantage,
        0.03,
      ),
      cost_first_guard_speed_advantage: numberOrDefault(
        setting.cost_first_guard_speed_advantage,
        0.08,
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
      queue_high_priority_threshold: numberOrDefault(
        setting.queue_high_priority_threshold,
      ),
      queue_high_priority_extra_depth: numberOrDefault(
        setting.queue_high_priority_extra_depth,
      ),
      queue_high_priority_reserved_depth: numberOrDefault(
        setting.queue_high_priority_reserved_depth,
      ),
      queue_absolute_max_depth: numberOrDefault(
        setting.queue_absolute_max_depth,
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
      probe_interval_seconds: numberOrDefault(
        setting.probe_interval_seconds,
        60,
      ),
      probe_worker_count: numberOrDefault(setting.probe_worker_count, 2),
      probe_timeout_seconds: numberOrDefault(setting.probe_timeout_seconds, 8),
      probe_max_per_tick: numberOrDefault(setting.probe_max_per_tick, 5),
      probe_min_channel_interval_seconds: numberOrDefault(
        setting.probe_min_channel_interval_seconds,
        300,
      ),
      probe_low_score_threshold: numberOrDefault(
        setting.probe_low_score_threshold,
        0.62,
      ),
      probe_missing_sample_threshold: numberOrDefault(
        setting.probe_missing_sample_threshold,
        3,
      ),
      probe_long_no_success_seconds: numberOrDefault(
        setting.probe_long_no_success_seconds,
        1800,
      ),
      probe_recovery_successes_required: numberOrDefault(
        setting.probe_recovery_successes_required,
        2,
      ),
      probe_failure_avoidance_priority_enabled:
        setting.probe_failure_avoidance_priority_enabled !== false,
      cost_calculation_interval_seconds: numberOrDefault(
        setting.cost_calculation_interval_seconds,
        5,
      ),
      cost_calculation_worker_count: numberOrDefault(
        setting.cost_calculation_worker_count,
        2,
      ),
      cost_calculation_batch_size: numberOrDefault(
        setting.cost_calculation_batch_size,
        100,
      ),
      dynamic_billing_profit_rate: numberOrDefault(
        setting.dynamic_billing_profit_rate,
        0.2,
      ),
      dynamic_billing_window_samples: numberOrDefault(
        setting.dynamic_billing_window_samples,
        300,
      ),
      dynamic_billing_min_samples: numberOrDefault(
        setting.dynamic_billing_min_samples,
        5,
      ),
      dynamic_billing_refresh_seconds: numberOrDefault(
        setting.dynamic_billing_refresh_seconds,
        30,
      ),
      dynamic_billing_max_age_seconds: numberOrDefault(
        setting.dynamic_billing_max_age_seconds,
        300,
      ),
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
      proxy_same_brand_reuse_policy:
        setting.proxy_same_brand_reuse_policy || 'warn',
      group_priority_ratio: priorities,
      group_policies: policies,
      circuit_error_policies: circuitErrorRowsToPolicyMap(circuitErrorRows),
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
      setDynamicBillingBaselines(data?.dynamic_billing_baselines || []);
      setPolicyRows(
        policyMapToRows(next.group_policies, next.group_priority_ratio),
      );
      setCircuitErrorRows(makeCircuitErrorRows(next.circuit_error_policies));
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
          setDynamicBillingBaselines(data?.dynamic_billing_baselines || []);
          setPolicyRows(
            policyMapToRows(next.group_policies, next.group_priority_ratio),
          );
          setCircuitErrorRows(
            makeCircuitErrorRows(next.circuit_error_policies),
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
        <Select
          value={row.group || undefined}
          optionList={candidateGroupOptions}
          placeholder={t('请选择分组')}
          filter
          allowCreate
          showClear
          style={{ width: '100%' }}
          onChange={(value) =>
            updatePolicyRow(row.id, { group: String(value || '') })
          }
          renderSelectedItem={(optionNode) => optionNode?.value || ''}
          renderOptionItem={(option) => renderGroupOptionItem(option, t)}
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
      dataIndex: 'candidate_groups',
      width: 330,
      render: (_, row) => (
        <CandidateGroupsEditor
          value={normalizeCandidateGroups(row.candidate_groups)}
          optionList={candidateGroupOptions}
          placeholder={t('请选择候选分组')}
          onChange={(value) =>
            updatePolicyRow(row.id, {
              candidate_groups: normalizeCandidateGroups(value),
            })
          }
          t={t}
        />
      ),
    },
    {
      title: t('调度权重'),
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
      title: t('收费倍率'),
      dataIndex: 'billing_ratio_mode',
      width: 220,
      render: (_, row) => {
        const mode = row.billing_ratio_mode || 'static';
        const staticRatio = groupRatioMap.get(row.group);
        const staticRatioText = formatStaticBillingRatio(
          row.group,
          staticRatio,
          t,
        );
        const dynamicInfo = resolveDynamicBillingInfo(row);
        const hasDynamic = !!dynamicInfo;
        const dynamicRatioText = hasDynamic
          ? formatRatioValue(dynamicInfo.ratio)
          : '-';
        const dynamicPriceText =
          hasDynamic && dynamicInfo.pricePerM > 0
            ? formatPricePerMillion(dynamicInfo.pricePerM)
            : '';
        return (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            <Select
              value={mode}
              optionList={billingRatioModeOptions}
              onChange={(value) =>
                updatePolicyRow(row.id, {
                  billing_ratio_mode: value || 'static',
                })
              }
            />
            {mode === 'dynamic' ? (
              <Space spacing={6} wrap>
                <Tag
                  color={
                    !setting.dynamic_billing_enabled
                      ? 'grey'
                      : hasDynamic
                        ? 'green'
                        : 'amber'
                  }
                >
                  {!setting.dynamic_billing_enabled
                    ? t('未启用')
                    : hasDynamic
                      ? t('动态可用')
                      : t('等待样本')}
                </Tag>
                {hasDynamic ? (
                  <Typography.Text type='tertiary' size='small'>
                    {dynamicInfo.group && dynamicInfo.group !== row.group
                      ? `${dynamicInfo.group} · `
                      : ''}
                    {dynamicRatioText}
                    {dynamicPriceText ? ` · ${dynamicPriceText}` : ''}
                    {dynamicInfo.referenceModel
                      ? ` · ${dynamicInfo.referenceModel}`
                      : ''}
                    {dynamicInfo.sampleCount
                      ? ` · ${dynamicInfo.sampleCount}${t('个样本')}`
                      : ''}
                  </Typography.Text>
                ) : (
                  <Typography.Text type='tertiary' size='small'>
                    {t('回退静态')} {staticRatioText}
                  </Typography.Text>
                )}
              </Space>
            ) : (
              <Typography.Text type='tertiary' size='small'>
                {t('静态分组倍率')} {staticRatioText}
              </Typography.Text>
            )}
          </div>
        );
      },
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
            color={row.queue_high_priority ? 'red' : 'grey'}
            onClick={() =>
              updatePolicyRow(row.id, {
                queue_high_priority: !row.queue_high_priority,
              })
            }
          >
            {t('高优先级队列')}
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

  const circuitErrorColumns = [
    {
      title: t('错误类型'),
      dataIndex: 'type',
      width: 170,
      render: (_, row) => circuitErrorTypeLabel(row.type, t),
    },
    {
      title: t('启用'),
      dataIndex: 'enabled',
      width: 90,
      render: (_, row) => (
        <Switch
          checked={row.enabled}
          onChange={(value) =>
            updateCircuitErrorRow(row.type, { enabled: value })
          }
        />
      ),
    },
    {
      title: t('失败率阈值'),
      dataIndex: 'failure_threshold',
      width: 140,
      render: (_, row) => (
        <Input
          value={row.failure_threshold}
          placeholder={String(setting.circuit_failure_threshold || 0.5)}
          onChange={(value) =>
            updateCircuitErrorRow(row.type, { failure_threshold: value })
          }
        />
      ),
    },
    {
      title: t('最小样本数'),
      dataIndex: 'min_samples',
      width: 130,
      render: (_, row) => (
        <Input
          value={row.min_samples}
          placeholder={String(setting.circuit_min_samples || 10)}
          onChange={(value) =>
            updateCircuitErrorRow(row.type, { min_samples: value })
          }
        />
      ),
    },
    {
      title: t('打开时间'),
      dataIndex: 'open_seconds',
      width: 130,
      render: (_, row) => (
        <Input
          value={row.open_seconds}
          placeholder={String(setting.circuit_open_seconds || 30)}
          onChange={(value) =>
            updateCircuitErrorRow(row.type, { open_seconds: value })
          }
        />
      ),
    },
    {
      title: t('半开探针数'),
      dataIndex: 'half_open_probe_count',
      width: 130,
      render: (_, row) => (
        <Input
          value={row.half_open_probe_count}
          placeholder={String(setting.circuit_half_open_probe_count || 3)}
          onChange={(value) =>
            updateCircuitErrorRow(row.type, { half_open_probe_count: value })
          }
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
              <Form.Switch
                field='runtime_sync_event_subscribe_enabled'
                label={t('运行时同步事件订阅')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting('runtime_sync_event_subscribe_enabled', value)
                }
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
          <Banner
            type='info'
            fullMode={false}
            closeIcon={null}
            title={t('粘滞续期策略')}
            description={t(
              '建议真实请求前保持成功续期开启、失败清理开启；如果上游偶发失败很多，可临时选择失败保留并结合熔断降权。',
            )}
            style={{ marginTop: 8, marginBottom: 16 }}
          />
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8}>
              <Form.Switch
                field='sticky_save_on_select'
                label={t('选中即写入粘滞')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting('sticky_save_on_select', value)
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.Switch
                field='sticky_renew_on_success'
                label={t('成功后续期粘滞')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting('sticky_renew_on_success', value)
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.Select
                field='sticky_failure_policy'
                label={t('失败后粘滞策略')}
                optionList={stickyFailurePolicyOptions}
                onChange={(value) =>
                  updateSetting('sticky_failure_policy', value)
                }
              />
            </Col>
          </Row>
          <Banner
            type='info'
            fullMode={false}
            closeIcon={null}
            title={t('成本优先低成本切换')}
            description={t(
              '仅在成本优先策略下，明显更低成本且速度评分影响可接受的候选才会打破粘滞。',
            )}
            style={{ marginTop: 8, marginBottom: 16 }}
          />
          <Row gutter={16}>
            <Col xs={24} sm={12} md={6}>
              <Form.Switch
                field='cost_first_sticky_escape_enabled'
                label={t('启用低成本切换')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting('cost_first_sticky_escape_enabled', value)
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_first_sticky_escape_cost_ratio'
                label={t('成本切换阈值')}
                min={0.01}
                max={1}
                step={0.01}
                suffix='x'
                onChange={(value) =>
                  updateSetting(
                    'cost_first_sticky_escape_cost_ratio',
                    numberOrDefault(value, 0.75),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_first_sticky_escape_max_speed_score_drop'
                label={t('最大速度分下降')}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) =>
                  updateSetting(
                    'cost_first_sticky_escape_max_speed_score_drop',
                    numberOrDefault(value, 0.06),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_first_sticky_escape_min_samples'
                label={t('切换最小样本')}
                min={1}
                onChange={(value) =>
                  updateSetting(
                    'cost_first_sticky_escape_min_samples',
                    numberOrDefault(value, 5),
                  )
                }
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_first_sticky_escape_cache_cost_ratio'
                label={t('缓存亲和成本阈值')}
                min={0.01}
                max={1}
                step={0.01}
                suffix='x'
                onChange={(value) =>
                  updateSetting(
                    'cost_first_sticky_escape_cache_cost_ratio',
                    numberOrDefault(value, 0.55),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_first_sticky_escape_cache_max_speed_score_drop'
                label={t('缓存亲和速度分下降')}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) =>
                  updateSetting(
                    'cost_first_sticky_escape_cache_max_speed_score_drop',
                    numberOrDefault(value, 0.03),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_first_sticky_escape_success_slack'
                label={t('成功率保护差值')}
                min={0}
                max={1}
                step={0.01}
                onChange={(value) =>
                  updateSetting(
                    'cost_first_sticky_escape_success_slack',
                    numberOrDefault(value, 0.02),
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
                field='queue_depth_multiplier'
                label={t('队列深度倍率')}
                min={1}
                onChange={(value) =>
                  updateSetting(
                    'queue_depth_multiplier',
                    numberOrDefault(value, 2),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='queue_absolute_max_depth'
                label={t('绝对最大队列深度')}
                min={0}
                onChange={(value) =>
                  updateSetting(
                    'queue_absolute_max_depth',
                    numberOrDefault(value),
                  )
                }
              />
            </Col>
          </Row>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                field='queue_high_priority_threshold'
                label={t('高优先级阈值')}
                min={0}
                onChange={(value) =>
                  updateSetting(
                    'queue_high_priority_threshold',
                    numberOrDefault(value),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                field='queue_high_priority_extra_depth'
                label={t('高优先级额外深度')}
                min={0}
                onChange={(value) =>
                  updateSetting(
                    'queue_high_priority_extra_depth',
                    numberOrDefault(value),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.InputNumber
                field='queue_high_priority_reserved_depth'
                label={t('高优先级保留深度')}
                min={0}
                onChange={(value) =>
                  updateSetting(
                    'queue_high_priority_reserved_depth',
                    numberOrDefault(value),
                  )
                }
              />
            </Col>
          </Row>
          <Row gutter={16}>
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
            <Col xs={24} sm={12} md={6}>
              <Form.Select
                field='proxy_same_brand_reuse_policy'
                label={t('代理同品牌复用策略')}
                optionList={proxyReusePolicyOptions}
                onChange={(value) =>
                  updateSetting(
                    'proxy_same_brand_reuse_policy',
                    value || 'warn',
                  )
                }
              />
            </Col>
          </Row>
          <Banner
            type='warning'
            fullMode={false}
            closeIcon={null}
            description={t(
              '仅提醒不拦截绑定；二次确认会在同品牌不同账号共用同一代理时要求确认；禁止复用会直接阻止绑定。',
            )}
            style={{ marginTop: 8, marginBottom: 16 }}
          />
          <Banner
            type='info'
            fullMode={false}
            closeIcon={null}
            title={t('渠道健康探活')}
            description={t(
              '后台低频探测低分、缺样本或长时间未成功的渠道，会回补健康评分，并以探活标记进入用户请求视图与消费日志。',
            )}
            style={{ marginTop: 8, marginBottom: 16 }}
          />
          <Row gutter={16}>
            <Col xs={24} sm={12} md={4}>
              <Form.Switch
                field='probe_enabled'
                label={t('启用健康探活')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) => updateSetting('probe_enabled', value)}
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_interval_seconds'
                label={t('探活扫描间隔')}
                min={10}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting(
                    'probe_interval_seconds',
                    numberOrDefault(value, 60),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_worker_count'
                label={t('探活 Worker 数')}
                min={1}
                onChange={(value) =>
                  updateSetting('probe_worker_count', numberOrDefault(value, 2))
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_timeout_seconds'
                label={t('单次探活超时')}
                min={1}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting(
                    'probe_timeout_seconds',
                    numberOrDefault(value, 8),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_max_per_tick'
                label={t('每轮最多探活')}
                min={1}
                onChange={(value) =>
                  updateSetting('probe_max_per_tick', numberOrDefault(value, 5))
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_min_channel_interval_seconds'
                label={t('单渠道最小间隔')}
                min={10}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting(
                    'probe_min_channel_interval_seconds',
                    numberOrDefault(value, 300),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_low_score_threshold'
                label={t('低分阈值')}
                min={0.01}
                max={1}
                step={0.01}
                onChange={(value) =>
                  updateSetting(
                    'probe_low_score_threshold',
                    numberOrDefault(value, 0.62),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_missing_sample_threshold'
                label={t('低样本阈值')}
                min={1}
                onChange={(value) =>
                  updateSetting(
                    'probe_missing_sample_threshold',
                    numberOrDefault(value, 3),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_long_no_success_seconds'
                label={t('长期未成功')}
                min={1}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting(
                    'probe_long_no_success_seconds',
                    numberOrDefault(value, 1800),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='probe_recovery_successes_required'
                label={t('恢复连续成功')}
                min={1}
                onChange={(value) =>
                  updateSetting(
                    'probe_recovery_successes_required',
                    numberOrDefault(value, 2),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={8}>
              <Form.Switch
                field='probe_failure_avoidance_priority_enabled'
                label={t('避让渠道优先探测')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting(
                    'probe_failure_avoidance_priority_enabled',
                    value,
                  )
                }
              />
            </Col>
          </Row>
          <Banner
            type='info'
            fullMode={false}
            closeIcon={null}
            title={t('上游成本补算')}
            description={t(
              '后台异步读取消费日志并补算供应商成本，只写入成本汇总表；用户请求、首包和流式转发不等待成本计算。',
            )}
            style={{ marginTop: 8, marginBottom: 16 }}
          />
          <Row gutter={16}>
            <Col xs={24} sm={12} md={6}>
              <Form.Switch
                field='cost_calculation_enabled'
                label={t('启用成本补算')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting('cost_calculation_enabled', value)
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_calculation_interval_seconds'
                label={t('成本补算间隔')}
                min={1}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting(
                    'cost_calculation_interval_seconds',
                    numberOrDefault(value, 5),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_calculation_worker_count'
                label={t('成本 Worker 数')}
                min={1}
                onChange={(value) =>
                  updateSetting(
                    'cost_calculation_worker_count',
                    numberOrDefault(value, 2),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={6}>
              <Form.InputNumber
                field='cost_calculation_batch_size'
                label={t('每批补算数量')}
                min={1}
                onChange={(value) =>
                  updateSetting(
                    'cost_calculation_batch_size',
                    numberOrDefault(value, 100),
                  )
                }
              />
            </Col>
          </Row>
          <Banner
            type='info'
            fullMode={false}
            closeIcon={null}
            title={t('动态收费倍率')}
            description={t(
              '分组策略选择动态收费倍率后，请求会按实际选中的分组读取后台动态倍率；未命中时自动回退静态分组倍率。',
            )}
            style={{ marginTop: 8, marginBottom: 16 }}
          />
          <Row gutter={16}>
            <Col xs={24} sm={12} md={4}>
              <Form.Switch
                field='dynamic_billing_enabled'
                label={t('启用动态收费')}
                checkedText='｜'
                uncheckedText='〇'
                onChange={(value) =>
                  updateSetting('dynamic_billing_enabled', value)
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='dynamic_billing_profit_rate'
                label={t('利润率')}
                min={0}
                step={0.01}
                suffix='x'
                onChange={(value) =>
                  updateSetting(
                    'dynamic_billing_profit_rate',
                    numberOrDefault(value, 0.2),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='dynamic_billing_window_samples'
                label={t('后台样本窗口')}
                min={1}
                suffix={t('份')}
                onChange={(value) =>
                  updateSetting(
                    'dynamic_billing_window_samples',
                    numberOrDefault(value, 300),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='dynamic_billing_min_samples'
                label={t('最小样本数')}
                min={1}
                onChange={(value) =>
                  updateSetting(
                    'dynamic_billing_min_samples',
                    numberOrDefault(value, 5),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='dynamic_billing_refresh_seconds'
                label={t('后台刷新间隔')}
                min={1}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting(
                    'dynamic_billing_refresh_seconds',
                    numberOrDefault(value, 30),
                  )
                }
              />
            </Col>
            <Col xs={24} sm={12} md={4}>
              <Form.InputNumber
                field='dynamic_billing_max_age_seconds'
                label={t('最大有效期')}
                min={1}
                suffix={t('秒')}
                onChange={(value) =>
                  updateSetting(
                    'dynamic_billing_max_age_seconds',
                    numberOrDefault(value, 300),
                  )
                }
              />
            </Col>
          </Row>
          <div style={{ marginTop: 8, marginBottom: 16 }}>
            <Banner
              type='info'
              fullMode={false}
              closeIcon={null}
              description={t('请求链路只读取内存动态价格结果，样本窗口按最近样本数由后台任务计算。')}
              style={{ marginBottom: 12 }}
            />
            <Typography.Text strong>{t('按错误类型熔断策略')}</Typography.Text>
            <Banner
              type='warning'
              fullMode={false}
              closeIcon={null}
              description={t(
                '未启用的错误类型会继续使用全局熔断规则；auth 与 quota 只有启用后才参与类型熔断。',
              )}
              style={{ marginTop: 8, marginBottom: 12 }}
            />
            <Table
              size='small'
              rowKey='type'
              pagination={false}
              columns={circuitErrorColumns}
              dataSource={circuitErrorRows}
              scroll={{ x: 790 }}
            />
          </div>
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
          scroll={{ x: 1680 }}
        />
      </div>
    </Spin>
  );
}
