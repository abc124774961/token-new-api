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
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Button,
  Checkbox,
  Empty,
  Input,
  InputNumber,
  Modal,
  Select,
  Spin,
  Switch,
  Table,
  Tabs,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Fingerprint,
  Gauge,
  Hash,
  Settings,
  History,
  KeyRound,
  Layers,
  ListChecks,
  RefreshCw,
  Route,
  Search,
  Server,
  ShieldCheck,
  Stethoscope,
  Timer,
  UserRound,
} from 'lucide-react';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import './channel-health-check.css';

const { TabPane } = Tabs;

const HISTORY_HOUR_OPTIONS = [1, 6, 24, 72, 168];
const ALL_STATUSES = 'all';
const PROBE_SCORE_ITEM_OPTIONS = [
  ['completion_rate', '完成率'],
  ['upstream_error_rate', '上游错误'],
  ['ttft_latency', '首包速度'],
  ['duration_latency', '完整耗时'],
  ['first_byte_backlog', '首包积压'],
  ['empty_output_rate', '空输出'],
  ['stream_interrupted_rate', '流中断'],
];
const PROBE_PROMPT_CATEGORY_OPTIONS = [
  ['short', '短文本'],
  ['zh', '中文'],
  ['medium', '中等文本'],
  ['long', '长文本'],
];

const DEFAULT_PROBE_CONFIG = {
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
  probe_recoverable_score_items: PROBE_SCORE_ITEM_OPTIONS.map(([value]) => value),
  probe_skip_recent_real_request_enabled: true,
  probe_recent_real_request_window_seconds: 1800,
  probe_good_baseline_enabled: true,
  probe_good_baseline_min_samples: 3,
  probe_good_baseline_window_seconds: 86400,
  probe_prompt_library_enabled: true,
  probe_prompt_categories: PROBE_PROMPT_CATEGORY_OPTIONS.map(([value]) => value),
};

function numberOrDefault(value, fallback) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : fallback;
}

function normalizeProbeConfig(setting = {}) {
  return {
    ...DEFAULT_PROBE_CONFIG,
    ...setting,
    probe_recoverable_score_items: Array.isArray(
      setting.probe_recoverable_score_items,
    )
        ? setting.probe_recoverable_score_items
        : DEFAULT_PROBE_CONFIG.probe_recoverable_score_items,
    probe_prompt_categories: Array.isArray(setting.probe_prompt_categories)
        ? setting.probe_prompt_categories
        : DEFAULT_PROBE_CONFIG.probe_prompt_categories,
  };
}

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function buildRequestErrorDetail(err, label, t) {
  const status = err?.response?.status;
  const responseMessage =
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    err?.response?.data?.detail;
  const message =
    responseMessage ||
    err?.message ||
    (err?.request ? t('网络连接失败或服务器无响应') : t('请求异常'));
  const url = [err?.config?.baseURL || '', err?.config?.url || '']
    .filter(Boolean)
    .join('');
  const detailParts = [];
  if (status) detailParts.push(`${t('状态码')}: ${status}`);
  if (err?.code) detailParts.push(`${t('错误码')}: ${err.code}`);
  if (url) detailParts.push(url);
  return {
    label,
    message,
    detail: detailParts.join(' · '),
  };
}

function formatNumber(value) {
  return new Intl.NumberFormat().format(Number(value) || 0);
}

function formatPercent(value, digits = 1) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `${(numeric * 100).toFixed(digits)}%`;
}

function formatScore(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return numeric.toFixed(3).replace(/0+$/, '').replace(/\.$/, '');
}

function formatScoreDelta(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || Math.abs(numeric) < 0.0001) return '0';
  const formatted = Math.abs(numeric)
    .toFixed(3)
    .replace(/0+$/, '')
    .replace(/\.$/, '');
  return `${numeric > 0 ? '+' : '-'}${formatted}`;
}

function scoreDeltaTone(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || Math.abs(numeric) < 0.0001) {
    return 'flat';
  }
  return numeric > 0 ? 'up' : 'down';
}

function formatLatency(value) {
  const latency = Number(value || 0);
  if (latency <= 0) return '--';
  if (latency >= 1000) return `${(latency / 1000).toFixed(2)}s`;
  return `${Math.round(latency)}ms`;
}

function formatTimestamp(timestamp) {
  return Number(timestamp || 0) > 0 ? timestamp2string(Number(timestamp)) : '--';
}

function compactIdentity(value, head = 8, tail = 6) {
  const text = String(value || '').trim();
  if (!text) return '';
  if (text.length <= head + tail + 4) return text;
  return `${text.slice(0, head)}...${text.slice(-tail)}`;
}

function normalizedIdentityValue(...values) {
  return values
    .map((value) => String(value || '').trim())
    .find(Boolean) || '';
}

function credentialNumber(value) {
  const index = Number(value);
  if (!Number.isFinite(index) || index < 0) return null;
  return Math.floor(index) + 1;
}

function getRuntimeKey(record) {
  return record?.runtime_key || record?.runtimeKey || {};
}

function getCredentialIdentity(record = {}, fallback = {}) {
  const runtimeKey = getRuntimeKey(fallback);
  const index = credentialNumber(
    record.credential_index ??
      fallback.credential_index ??
      runtimeKey.credential_index,
  );
  const subjectFP = normalizedIdentityValue(
    record.credential_subject_fingerprint,
    fallback.credential_subject_fingerprint,
    runtimeKey.credential_subject_fingerprint,
  );
  const credentialFP = normalizedIdentityValue(
    record.credential_fingerprint,
    fallback.credential_fingerprint,
    runtimeKey.credential_fingerprint,
  );
  return { index, subjectFP, credentialFP };
}

function getChannelIdentity(record = {}, fallback = {}) {
  const runtimeKey = getRuntimeKey(fallback);
  const channelID = Number(
    record.channel_id ||
      record.final_channel_id ||
      fallback.channel_id ||
      runtimeKey.channel_id ||
      0,
  );
  return {
    channelID,
    channelName: normalizedIdentityValue(
      record.channel_name,
      record.final_channel_name,
      fallback.channel_name,
    ),
    accountID: normalizedIdentityValue(
      record.account_id,
      fallback.account_id,
      runtimeKey.account_id,
    ),
    accountType: normalizedIdentityValue(
      record.account_type,
      fallback.account_type,
      runtimeKey.account_type,
    ),
    brand: normalizedIdentityValue(record.brand, fallback.brand, runtimeKey.brand),
    provider: normalizedIdentityValue(
      record.provider,
      fallback.provider,
      runtimeKey.provider,
    ),
    resourceID: normalizedIdentityValue(
      record.resource_id,
      fallback.resource_id,
      runtimeKey.resource_id,
    ),
    resourceType: normalizedIdentityValue(
      record.resource_type,
      fallback.resource_type,
      runtimeKey.resource_type,
    ),
    ...getCredentialIdentity(record, fallback),
  };
}

function formatRelativeTime(timestamp, t) {
  const normalized = Number(timestamp || 0);
  if (normalized <= 0) return '--';
  const diffSeconds = Math.max(0, Math.floor(Date.now() / 1000) - normalized);
  if (diffSeconds < 30) return t('刚刚');
  if (diffSeconds < 3600) {
    return t('{{count}}分钟前', {
      count: Math.max(1, Math.floor(diffSeconds / 60)),
    });
  }
  if (diffSeconds < 86400) {
    return t('{{count}}小时前', {
      count: Math.max(1, Math.floor(diffSeconds / 3600)),
    });
  }
  return t('{{count}}天前', {
    count: Math.max(1, Math.floor(diffSeconds / 86400)),
  });
}

function formatCountdownDuration(seconds, t) {
  const totalSeconds = Math.max(0, Math.floor(Number(seconds) || 0));
  if (totalSeconds <= 0) return t('可立即探测');
  if (totalSeconds < 60) {
    return t('{{seconds}}秒后', { seconds: totalSeconds });
  }
  if (totalSeconds < 3600) {
    return t('{{minutes}}分{{seconds}}秒后', {
      minutes: Math.floor(totalSeconds / 60),
      seconds: totalSeconds % 60,
    });
  }
  if (totalSeconds < 86400) {
    return t('{{hours}}小时{{minutes}}分钟后', {
      hours: Math.floor(totalSeconds / 3600),
      minutes: Math.floor((totalSeconds % 3600) / 60),
    });
  }
  return t('{{days}}天{{hours}}小时后', {
    days: Math.floor(totalSeconds / 86400),
    hours: Math.floor((totalSeconds % 86400) / 3600),
  });
}

function getProbeCountdownSeconds(record, nowTick, generatedAt) {
  const remaining = Number(record?.next_probe_remaining_seconds);
  const generated = Number(generatedAt || 0);
  if (Number.isFinite(remaining) && remaining >= 0 && generated > 0) {
    return Math.max(0, Math.floor(generated + remaining - nowTick));
  }
  const nextProbeAt = Number(record?.next_probe_at || 0);
  if (nextProbeAt > 0) {
    return Math.max(0, Math.floor(nextProbeAt - nowTick));
  }
  if (Number.isFinite(remaining) && remaining > 0) {
    return Math.floor(remaining);
  }
  return 0;
}

function getRuntimeHealthMeta(status, t) {
  switch (status) {
    case 'circuit_open':
      return { color: 'red', label: t('熔断打开') };
    case 'cooldown':
      return { color: 'orange', label: t('冷却') };
    case 'failure_avoidance':
      return { color: 'orange', label: t('恢复中') };
    case 'queued':
      return { color: 'cyan', label: t('队列中') };
    case 'high_pressure':
    case 'saturated':
      return { color: 'orange', label: t('并发压力') };
    case 'degraded':
      return { color: 'orange', label: t('降级') };
    case 'healthy':
      return { color: 'green', label: t('健康') };
    default:
      return { color: 'grey', label: status || t('未知') };
  }
}

function getPriorityMeta(priority, t) {
  if (priority >= 90) return { color: 'red', label: t('高优先级') };
  if (priority >= 65) return { color: 'orange', label: t('中优先级') };
  return { color: 'cyan', label: t('低优先级') };
}

function getReasonMeta(reason, t) {
  const key = typeof reason === 'string' ? reason : reason?.key;
  const severity = typeof reason === 'object' ? reason?.severity : '';
  const severityColorMap = {
    critical: 'red',
    warning: 'orange',
    info: 'cyan',
    neutral: 'grey',
  };
  const color =
    severityColorMap[severity] ||
    {
      config_error: 'red',
      circuit_open: 'red',
      cooldown: 'orange',
      failure_avoidance: 'orange',
      probe_recovery_pending: 'cyan',
      low_score: 'orange',
      missing_samples: 'grey',
      success_rate: 'orange',
      empty_output: 'orange',
      experience_issue: 'orange',
    }[key] ||
    'grey';
  const label =
    {
      config_error: t('配置异常隔离'),
      circuit_open: t('熔断打开'),
      cooldown: t('冷却恢复探测'),
      failure_avoidance: t('近期失败恢复中'),
      probe_recovery_pending: t('恢复确认中'),
      low_score: t('低评分'),
      missing_samples: t('历史样本不足'),
      success_rate: t('成功率偏低'),
      empty_output: t('空输出偏高'),
      experience_issue: t('体验异常偏高'),
    }[key] || key || t('待检查');
  return { key, label, color };
}

function getProbeStatusMeta(record, t) {
  if (record?.final_success || record?.success) {
    return { color: 'green', label: t('成功') };
  }
  if (record?.client_aborted || record?.status === 'client_aborted') {
    return { color: 'grey', label: t('客户端中断') };
  }
  if (record?.status === 'processing') {
    return { color: 'blue', label: t('处理中') };
  }
  return { color: 'orange', label: t('探活异常') };
}

function formatProbeReason(value, t) {
  switch (String(value || '').trim()) {
    case 'missing_samples':
      return t('缺少样本');
    case 'low_score':
      return t('低分恢复探测');
    case 'low_traffic':
      return t('低访问激活探测');
    case 'failure_avoidance':
      return t('近期失败恢复中');
    case 'cooldown':
      return t('冷却恢复探测');
    case 'long_no_success':
      return t('长期未成功');
    case 'circuit_half_open':
      return t('熔断半开');
    case 'sampling':
      return t('常规抽样');
    default:
      return value || t('健康探活');
  }
}

function recordMatchesSearch(record, keyword) {
  const normalized = keyword.trim().toLowerCase();
  if (!normalized) return true;
  const dispatch = record?.dispatch_record || record?.dispatchRecord || {};
  const candidates = [
    ...(Array.isArray(record?.candidate_explanations)
      ? record.candidate_explanations
      : []),
    ...(Array.isArray(dispatch?.candidate_explanations)
      ? dispatch.candidate_explanations
      : []),
    ...(Array.isArray(dispatch?.request_meta?.candidate_explanations)
      ? dispatch.request_meta.candidate_explanations
      : []),
  ];
  const candidateValues = candidates.flatMap((candidate) => {
    const runtimeKey = candidate?.runtime_key || {};
    return [
      candidate?.channel_name,
      candidate?.channel_id ? `#${candidate.channel_id}` : '',
      candidate?.account_id,
      candidate?.account_type,
      candidate?.brand,
      candidate?.provider,
      candidate?.resource_id,
      candidate?.resource_type,
      candidate?.credential_index != null
        ? `#${Number(candidate.credential_index) + 1}`
        : '',
      candidate?.credential_subject_fingerprint,
      candidate?.credential_fingerprint,
      runtimeKey.account_id,
      runtimeKey.credential_index != null
        ? `#${Number(runtimeKey.credential_index) + 1}`
        : '',
      runtimeKey.credential_subject_fingerprint,
      runtimeKey.credential_fingerprint,
    ];
  });
  return [
    record.requested_model,
    record.upstream_model,
    record.group,
    record.endpoint_type,
    record.channel_name,
    record.channel_id ? `#${record.channel_id}` : '',
    record.final_channel_id ? `#${record.final_channel_id}` : '',
    record.final_channel_name,
    record.account_id,
    record.account_type,
    record.brand,
    record.provider,
    record.resource_id,
    record.resource_type,
    record.credential_index != null
      ? `#${Number(record.credential_index) + 1}`
      : '',
    record.credential_subject_fingerprint,
    record.credential_fingerprint,
    record.request_id,
    record.probe_reason,
    record.final_error_category,
    ...candidateValues,
  ]
    .filter(Boolean)
    .some((value) => String(value).toLowerCase().includes(normalized));
}

function historyRecordKey(record) {
  const requestID = String(record?.request_id || '').trim();
  if (requestID) return requestID;
  return `${record?.id || 0}:${record?.completed_at || record?.created_at || 0}`;
}

function getRecordDispatch(record) {
  return record?.dispatch_record || record?.dispatchRecord || null;
}

function getRecordCandidates(record) {
  const dispatch = getRecordDispatch(record);
  const candidates =
    dispatch?.candidate_explanations ||
    dispatch?.request_meta?.candidate_explanations ||
    record?.candidate_explanations ||
    [];
  return Array.isArray(candidates) ? candidates : [];
}

function getSelectedScoreCandidate(record) {
  const candidates = getRecordCandidates(record);
  if (!candidates.length) return null;
  const selected = candidates.find((candidate) => candidate?.selected === true);
  if (selected) return selected;
  const channelID = Number(
    record?.final_channel_id ||
      getRecordDispatch(record)?.actual_channel_id ||
      getRecordDispatch(record)?.channel_id ||
      0,
  );
  if (channelID > 0) {
    return (
      candidates.find(
        (candidate) => Number(candidate?.channel_id || 0) === channelID,
      ) || null
    );
  }
  return null;
}

function numericScore(value) {
  const score = Number(value);
  return Number.isFinite(score) && score > 0 ? score : null;
}

function getHistoryScore(record) {
  const candidate = getSelectedScoreCandidate(record);
  return (
    numericScore(candidate?.score_total) ??
    numericScore(getRecordDispatch(record)?.score_total) ??
    numericScore(record?.score_total)
  );
}

function getHistoryScoreBreakdown(record) {
  const candidate = getSelectedScoreCandidate(record);
  const breakdown =
    candidate?.score_breakdown ||
    getRecordDispatch(record)?.score_breakdown ||
    record?.score_breakdown ||
    {};
  return breakdown && typeof breakdown === 'object' ? breakdown : {};
}

function historyScoreScope(record) {
  const candidate = getSelectedScoreCandidate(record);
  const dispatch = getRecordDispatch(record);
  const runtimeKey = candidate?.runtime_key || {};
  const group =
    candidate?.group ||
    runtimeKey.group ||
    record?.actual_group ||
    dispatch?.actual_group ||
    dispatch?.selected_group ||
    record?.selected_group ||
    record?.requested_group ||
    dispatch?.requested_group ||
    '';
  const channelID = Number(
    candidate?.channel_id ||
      record?.final_channel_id ||
      dispatch?.actual_channel_id ||
      dispatch?.channel_id ||
      record?.channel_id ||
      0,
  );
  return [
    channelID,
    record?.requested_model || dispatch?.requested_model || runtimeKey.requested_model || '',
    candidate?.upstream_model ||
      runtimeKey.upstream_model ||
      record?.upstream_model ||
      dispatch?.upstream_model ||
      '',
    group,
    record?.endpoint_type || dispatch?.endpoint_type || runtimeKey.endpoint_type || '',
  ]
    .map((value) => String(value || '').trim())
    .join('|');
}

function buildHistoryScoreChanges(records) {
  const changes = new Map();
  const previousByScope = new Map();
  const sorted = [...(records || [])].sort((left, right) => {
    const leftTime = Number(left?.completed_at || left?.created_at || 0);
    const rightTime = Number(right?.completed_at || right?.created_at || 0);
    if (leftTime !== rightTime) return leftTime - rightTime;
    return Number(left?.id || 0) - Number(right?.id || 0);
  });

  sorted.forEach((record) => {
    const score = getHistoryScore(record);
    const key = historyRecordKey(record);
    const scope = historyScoreScope(record);
    if (score === null || !scope) {
      changes.set(key, { score: null, delta: 0, hasPrevious: false, metricDeltas: {} });
      return;
    }
    const breakdown = getHistoryScoreBreakdown(record);
    const previous = previousByScope.get(scope);
    const metricDeltas = {};
    if (previous) {
      Object.keys({ ...breakdown, ...previous.breakdown }).forEach((metric) => {
        const value = Number(breakdown?.[metric]);
        const previousValue = Number(previous.breakdown?.[metric]);
        if (Number.isFinite(value) && Number.isFinite(previousValue)) {
          metricDeltas[metric] = value - previousValue;
        }
      });
    }
    changes.set(key, {
      score,
      delta: previous ? score - previous.score : 0,
      hasPrevious: Boolean(previous),
      metricDeltas,
    });
    previousByScope.set(scope, { score, breakdown });
  });

  return changes;
}

function scoreMetricLabel(key, t) {
  switch (String(key || '').trim()) {
    case 'completion_rate':
      return t('完成率');
    case 'upstream_error_rate':
      return t('上游错误');
    case 'ttft_latency':
      return t('首包');
    case 'duration_latency':
      return t('完整耗时');
    case 'throughput':
      return t('吞吐');
    case 'empty_output_rate':
      return t('空输出');
    case 'stream_interrupted_rate':
      return t('流中断');
    case 'concurrency_load':
      return t('并发');
    case 'queue_pressure':
      return t('队列');
    case 'first_byte_backlog':
      return t('首包积压');
    case 'cost':
      return t('成本');
    case 'group_priority':
      return t('分组');
    default:
      return String(key || '').trim() || t('未知');
  }
}

function ProbeScoreItemTags({ items, t }) {
  if (!items?.length) {
    return <Typography.Text type='tertiary'>--</Typography.Text>;
  }
  return (
    <div className='ct-channel-health-tags'>
      {items.slice(0, 4).map((item) => (
        <Tag key={item} color='orange' size='small' type='light'>
          {scoreMetricLabel(item, t)}
        </Tag>
      ))}
      {items.length > 4 && (
        <Tag color='grey' size='small' type='light'>
          +{items.length - 4}
        </Tag>
      )}
    </div>
  );
}

function importantScoreDeltaEntries(delta, t) {
  return Object.entries(delta || {})
    .map(([key, value]) => ({ key, value: Number(value) }))
    .filter((entry) => Number.isFinite(entry.value) && Math.abs(entry.value) >= 0.0001)
    .sort((left, right) => Math.abs(right.value) - Math.abs(left.value))
    .slice(0, 3)
    .map((entry) => ({
      ...entry,
      label: scoreMetricLabel(entry.key, t),
      tone: scoreDeltaTone(entry.value),
    }));
}

function ScoreChangeCell({ scoreChange, t }) {
  if (!scoreChange || scoreChange.score === null) {
    return (
      <div className='ct-channel-health-score-change ct-channel-health-score-change-empty'>
        <strong>--</strong>
        <span>{t('暂无评分')}</span>
      </div>
    );
  }
  const tone = scoreDeltaTone(scoreChange.delta);
  const metricEntries = importantScoreDeltaEntries(scoreChange.metricDeltas, t);
  return (
    <div className='ct-channel-health-score-change'>
      <div className='ct-channel-health-score-change-main'>
        <strong>{formatScore(scoreChange.score)}</strong>
        <span className={`ct-channel-health-score-delta-${tone}`}>
          {scoreChange.hasPrevious ? formatScoreDelta(scoreChange.delta) : t('首次')}
        </span>
      </div>
      {scoreChange.hasPrevious ? (
        metricEntries.length ? (
          <div className='ct-channel-health-score-change-metrics'>
            {metricEntries.map((entry) => (
              <span key={entry.key}>
                {entry.label}
                <em className={`ct-channel-health-score-delta-${entry.tone}`}>
                  {formatScoreDelta(entry.value)}
                </em>
              </span>
            ))}
          </div>
        ) : (
          <span className='ct-channel-health-score-change-note'>
            {t('评分基本稳定')}
          </span>
        )
      ) : (
        <span className='ct-channel-health-score-change-note'>
          {t('暂无上一条对比')}
        </span>
      )}
    </div>
  );
}

function MetricCard({ label, value, detail, icon, tone = 'default' }) {
  return (
    <div className={`ct-channel-health-metric ct-channel-health-metric-${tone}`}>
      <div className='ct-channel-health-metric-main'>
        <span>{label}</span>
        <strong>{value}</strong>
        <small>{detail}</small>
      </div>
      <div className='ct-channel-health-metric-icon'>{icon}</div>
    </div>
  );
}

function IconBadge({ icon, label, tooltip, tone = 'default', mono = false }) {
  if (!label) return null;
  const badge = (
    <span
      className={`ct-channel-health-icon-badge ct-channel-health-icon-badge-${tone} ${
        mono ? 'ct-channel-health-icon-badge-mono' : ''
      }`}
    >
      {icon}
      <span>{label}</span>
    </span>
  );
  return tooltip ? <Tooltip content={tooltip}>{badge}</Tooltip> : badge;
}

function IdentityTooltip({ identity, t }) {
  const rows = [
    [t('渠道 ID'), identity.channelID ? `#${identity.channelID}` : ''],
    [t('渠道'), identity.channelName],
    [t('账号标识'), identity.accountID],
    [t('账号凭证类型'), identity.accountType],
    [t('凭证序号'), identity.index ? `#${identity.index}` : ''],
    [t('凭证主体指纹'), identity.subjectFP],
    [t('凭证指纹'), identity.credentialFP],
    [t('品牌'), identity.brand],
    [t('供应商'), identity.provider],
    [t('资源'), identity.resourceID],
    [t('资源类型'), identity.resourceType],
  ].filter(([, value]) => String(value || '').trim());
  if (!rows.length) return null;
  return (
    <div className='ct-channel-health-identity-tooltip'>
      {rows.map(([label, value]) => (
        <div key={label}>
          <span>{label}</span>
          <strong>{value}</strong>
        </div>
      ))}
    </div>
  );
}

function ChannelIdentityCell({ record, t, fallback = null }) {
  const identity = getChannelIdentity(record, fallback || {});
  const title =
    identity.channelName ||
    (identity.channelID ? `${t('渠道')} #${identity.channelID}` : '--');
  const detail = [identity.brand, identity.provider].filter(Boolean).join(' / ');
  const tooltip = <IdentityTooltip identity={identity} t={t} />;
  return (
    <div className='ct-channel-health-identity-cell'>
      <div className='ct-channel-health-identity-title'>
        <Tooltip content={tooltip || title}>
          <Typography.Text strong ellipsis={{ showTooltip: false }}>
            {title}
          </Typography.Text>
        </Tooltip>
      </div>
      <div className='ct-channel-health-badge-row'>
        <IconBadge
          icon={<Hash size={12} />}
          label={identity.channelID ? `#${identity.channelID}` : ''}
          tooltip={t('渠道 ID')}
          mono
        />
        <IconBadge
          icon={<KeyRound size={12} />}
          label={identity.index ? `#${identity.index}` : ''}
          tooltip={t('凭证序号')}
          tone='blue'
          mono
        />
        <IconBadge
          icon={<UserRound size={12} />}
          label={compactIdentity(identity.accountID)}
          tooltip={`${t('账号标识')}: ${identity.accountID}`}
          tone='green'
          mono
        />
        <IconBadge
          icon={<Fingerprint size={12} />}
          label={compactIdentity(identity.subjectFP || identity.credentialFP)}
          tooltip={
            identity.subjectFP
              ? `${t('凭证主体指纹')}: ${identity.subjectFP}`
              : identity.credentialFP
                ? `${t('凭证指纹')}: ${identity.credentialFP}`
                : ''
          }
          tone='purple'
          mono
        />
      </div>
      {detail && (
        <div className='ct-channel-health-badge-row'>
          <IconBadge
            icon={<Server size={12} />}
            label={compactIdentity(detail, 10, 4)}
            tooltip={detail}
            tone='grey'
          />
        </div>
      )}
    </div>
  );
}

function ScopeCell({ record, t }) {
  const dispatch = getRecordDispatch(record) || {};
  const candidate = getSelectedScoreCandidate(record) || {};
  const runtimeKey = candidate.runtime_key || {};
  const requestedModel =
    record.requested_model || dispatch.requested_model || runtimeKey.requested_model || '';
  const group =
    record.group ||
    candidate.group ||
    runtimeKey.group ||
    record.actual_group ||
    dispatch.actual_group ||
    dispatch.selected_group ||
    record.selected_group ||
    record.requested_group ||
    dispatch.requested_group ||
    '';
  const upstreamModel =
    record.upstream_model ||
    candidate.upstream_model ||
    runtimeKey.upstream_model ||
    dispatch.upstream_model ||
    '';
  const endpointType =
    record.endpoint_type || dispatch.endpoint_type || runtimeKey.endpoint_type || '';
  return (
    <div className='ct-channel-health-scope-cell'>
      <Typography.Text strong ellipsis={{ showTooltip: true }}>
        {requestedModel || '--'}
      </Typography.Text>
      <div className='ct-channel-health-badge-row'>
        <IconBadge
          icon={<Layers size={12} />}
          label={group}
          tooltip={t('分组')}
          tone='green'
        />
        <IconBadge
          icon={<Route size={12} />}
          label={upstreamModel}
          tooltip={t('上游模型')}
          tone='blue'
        />
        <IconBadge
          icon={<Server size={12} />}
          label={endpointType}
          tooltip={t('端点')}
          tone='grey'
        />
      </div>
    </div>
  );
}

function ScoreCompactCell({ record, t }) {
  return (
    <div className='ct-channel-health-score-stack'>
      <strong>{formatScore(record.score_total)}</strong>
      <div className='ct-channel-health-badge-row'>
        <IconBadge
          icon={<Gauge size={12} />}
          label={formatScore(record.routing_score_total)}
          tooltip={t('本次调度评分')}
          tone='blue'
          mono
        />
        <IconBadge
          icon={<CheckCircle2 size={12} />}
          label={formatScore(record.score_breakdown?.completion_rate)}
          tooltip={t('完成率')}
          tone='green'
          mono
        />
        <IconBadge
          icon={<AlertTriangle size={12} />}
          label={formatPercent(record.empty_output_rate)}
          tooltip={t('空输出率')}
          tone={Number(record.empty_output_rate || 0) > 0 ? 'orange' : 'grey'}
          mono
        />
      </div>
    </div>
  );
}

function SamplesCell({ record, t }) {
  return (
    <div className='ct-channel-health-stack'>
      <div className='ct-channel-health-badge-row'>
        <IconBadge
          icon={<Clock3 size={12} />}
          label={`30m ${formatNumber(record.real_sample_count_30m)}`}
          tooltip={t('近30分钟')}
          tone='blue'
          mono
        />
        <IconBadge
          icon={<ListChecks size={12} />}
          label={formatNumber(record.sample_count)}
          tooltip={t('历史样本')}
          tone='grey'
          mono
        />
      </div>
      <Typography.Text type='tertiary' size='small'>
        {t('最后真实成功')} {formatRelativeTime(record.last_real_success_at, t)}
      </Typography.Text>
    </div>
  );
}

function PerformanceCell({ record, t }) {
  return (
    <div className='ct-channel-health-stack'>
      <div className='ct-channel-health-badge-row'>
        <IconBadge
          icon={<CheckCircle2 size={12} />}
          label={record.sample_count > 0 ? formatPercent(record.success_rate) : '--'}
          tooltip={t('完成率')}
          tone='green'
          mono
        />
        <IconBadge
          icon={<Timer size={12} />}
          label={formatLatency(record.ttft_ms)}
          tooltip={t('首包')}
          tone='blue'
          mono
        />
        <IconBadge
          icon={<Gauge size={12} />}
          label={formatLatency(record.duration_ms)}
          tooltip={t('耗时')}
          tone='purple'
          mono
        />
      </div>
      <Typography.Text type='tertiary' size='small'>
        {t('并发')} {formatNumber(record.active_concurrency)}
        {record.max_concurrency > 0
          ? ` / ${formatNumber(record.max_concurrency)}`
          : ''}
      </Typography.Text>
    </div>
  );
}

function reasonIconForKey(key) {
  switch (key) {
    case 'config_error':
    case 'circuit_open':
      return <AlertTriangle size={11} />;
    case 'cooldown':
    case 'failure_avoidance':
    case 'timeout_recovery':
      return <Clock3 size={11} />;
    case 'probe_recovery_pending':
      return <ShieldCheck size={11} />;
    case 'low_score':
      return <Gauge size={11} />;
    case 'missing_samples':
      return <ListChecks size={11} />;
    default:
      return <Activity size={11} />;
  }
}

function ReasonTags({ reasons, t }) {
  if (!reasons?.length) {
    return <Typography.Text type='tertiary'>--</Typography.Text>;
  }
  return (
    <div className='ct-channel-health-tags'>
      {reasons.slice(0, 4).map((reason) => {
        const meta = getReasonMeta(reason, t);
        return (
          <Tag key={meta.key} color={meta.color} size='small' type='light'>
            <span className='ct-channel-health-tag-content'>
              {reasonIconForKey(meta.key)}
              {meta.label}
            </span>
          </Tag>
        );
      })}
      {reasons.length > 4 && (
        <Tag color='grey' size='small' type='light'>
          +{reasons.length - 4}
        </Tag>
      )}
    </div>
  );
}

function ProbeSettingField({ label, children }) {
  return (
    <label className='ct-channel-health-setting-field'>
      <Typography.Text type='secondary' size='small'>
        {label}
      </Typography.Text>
      {children}
    </label>
  );
}

function ProbeSettingsModal({
  visible,
  config,
  loading,
  saving,
  onCancel,
  onChange,
  onSave,
  t,
}) {
  const update = (key, value) => onChange({ ...config, [key]: value });
  const scoreItems = config.probe_recoverable_score_items || [];
  const promptCategories = config.probe_prompt_categories || [];

  return (
    <Modal
      title={t('健康探活设置')}
      visible={visible}
      onCancel={onCancel}
      onOk={onSave}
      confirmLoading={saving}
      okText={t('保存设置')}
      cancelText={t('取消')}
      width={760}
      className='ct-channel-health-settings-modal'
    >
      <Spin spinning={loading}>
        <div className='ct-channel-health-settings'>
          <section>
            <div className='ct-channel-health-settings-title'>{t('基础开关')}</div>
            <div className='ct-channel-health-settings-grid'>
              <ProbeSettingField label={t('启用健康探活')}>
                <Switch
                  checked={config.probe_enabled !== false}
                  onChange={(value) => update('probe_enabled', value)}
                />
              </ProbeSettingField>
              <ProbeSettingField label={t('探活扫描间隔')}>
                <InputNumber
                  min={10}
                  suffix={t('秒')}
                  value={config.probe_interval_seconds}
                  onChange={(value) =>
                    update('probe_interval_seconds', numberOrDefault(value, 60))
                  }
                />
              </ProbeSettingField>
              <ProbeSettingField label={t('每轮最多探活')}>
                <InputNumber
                  min={1}
                  value={config.probe_max_per_tick}
                  onChange={(value) =>
                    update('probe_max_per_tick', numberOrDefault(value, 5))
                  }
                />
              </ProbeSettingField>
              <ProbeSettingField label={t('单渠道最小间隔')}>
                <InputNumber
                  min={10}
                  suffix={t('秒')}
                  value={config.probe_min_channel_interval_seconds}
                  onChange={(value) =>
                    update(
                      'probe_min_channel_interval_seconds',
                      numberOrDefault(value, 300),
                    )
                  }
                />
              </ProbeSettingField>
              <ProbeSettingField label={t('单次探活超时')}>
                <InputNumber
                  min={1}
                  suffix={t('秒')}
                  value={config.probe_timeout_seconds}
                  onChange={(value) =>
                    update('probe_timeout_seconds', numberOrDefault(value, 8))
                  }
                />
              </ProbeSettingField>
            </div>
          </section>

          <section>
            <div className='ct-channel-health-settings-title'>{t('低分触发')}</div>
            <div className='ct-channel-health-settings-grid'>
              <ProbeSettingField label={t('低分阈值')}>
                <InputNumber
                  min={0.01}
                  max={1}
                  step={0.01}
                  value={config.probe_low_score_threshold}
                  onChange={(value) =>
                    update('probe_low_score_threshold', numberOrDefault(value, 0.62))
                  }
                />
              </ProbeSettingField>
            </div>
            <Checkbox.Group
              value={scoreItems}
              onChange={(values) => update('probe_recoverable_score_items', values)}
            >
              <div className='ct-channel-health-checkbox-grid'>
                {PROBE_SCORE_ITEM_OPTIONS.map(([value, label]) => (
                  <Checkbox key={value} value={value}>
                    {t(label)}
                  </Checkbox>
                ))}
              </div>
            </Checkbox.Group>
          </section>

          <section>
            <div className='ct-channel-health-settings-title'>{t('真实请求跳过')}</div>
            <div className='ct-channel-health-settings-grid'>
              <ProbeSettingField label={t('近期已有真实请求则跳过体检')}>
                <Switch
                  checked={config.probe_skip_recent_real_request_enabled !== false}
                  onChange={(value) =>
                    update('probe_skip_recent_real_request_enabled', value)
                  }
                />
              </ProbeSettingField>
              <ProbeSettingField label={t('近期窗口')}>
                <InputNumber
                  min={1}
                  suffix={t('秒')}
                  value={config.probe_recent_real_request_window_seconds}
                  onChange={(value) =>
                    update(
                      'probe_recent_real_request_window_seconds',
                      numberOrDefault(value, 1800),
                    )
                  }
                />
              </ProbeSettingField>
            </div>
          </section>

          <section>
            <div className='ct-channel-health-settings-title'>{t('轻量基线')}</div>
            <div className='ct-channel-health-settings-grid'>
              <ProbeSettingField label={t('只体检平常表现不错的渠道')}>
                <Switch
                  checked={config.probe_good_baseline_enabled !== false}
                  onChange={(value) => update('probe_good_baseline_enabled', value)}
                />
              </ProbeSettingField>
              <ProbeSettingField label={t('最低历史样本数')}>
                <InputNumber
                  min={1}
                  value={config.probe_good_baseline_min_samples}
                  onChange={(value) =>
                    update('probe_good_baseline_min_samples', numberOrDefault(value, 3))
                  }
                />
              </ProbeSettingField>
              <ProbeSettingField label={t('历史成功窗口')}>
                <InputNumber
                  min={1}
                  suffix={t('秒')}
                  value={config.probe_good_baseline_window_seconds}
                  onChange={(value) =>
                    update(
                      'probe_good_baseline_window_seconds',
                      numberOrDefault(value, 86400),
                    )
                  }
                />
              </ProbeSettingField>
            </div>
          </section>

          <section>
            <div className='ct-channel-health-settings-title'>{t('样本库')}</div>
            <div className='ct-channel-health-settings-grid'>
              <ProbeSettingField label={t('启用内置低成本随机样本')}>
                <Switch
                  checked={config.probe_prompt_library_enabled !== false}
                  onChange={(value) => update('probe_prompt_library_enabled', value)}
                />
              </ProbeSettingField>
            </div>
            <Checkbox.Group
              value={promptCategories}
              onChange={(values) => update('probe_prompt_categories', values)}
            >
              <div className='ct-channel-health-checkbox-grid'>
                {PROBE_PROMPT_CATEGORY_OPTIONS.map(([value, label]) => (
                  <Checkbox key={value} value={value}>
                    {t(label)}
                  </Checkbox>
                ))}
              </div>
            </Checkbox.Group>
          </section>
        </div>
      </Spin>
    </Modal>
  );
}

function ChannelHealthCheck() {
  const { t } = useTranslation();
  const [queueData, setQueueData] = useState(null);
  const [historyData, setHistoryData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [historyHours, setHistoryHours] = useState(24);
  const [statusFilter, setStatusFilter] = useState(ALL_STATUSES);
  const [keyword, setKeyword] = useState('');
  const [settingsVisible, setSettingsVisible] = useState(false);
  const [settingsLoading, setSettingsLoading] = useState(false);
  const [settingsSaving, setSettingsSaving] = useState(false);
  const [probeConfig, setProbeConfig] = useState(DEFAULT_PROBE_CONFIG);
  const [nowTick, setNowTick] = useState(() => Math.floor(Date.now() / 1000));
  const [filters, setFilters] = useState({
    model: '',
    group: '',
    channel_id: '',
  });
  const [appliedFilters, setAppliedFilters] = useState(filters);

  const loadData = useCallback(
    async (silent = false) => {
      if (silent) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      setError('');
      const commonParams = {
        model: appliedFilters.model || undefined,
        group: appliedFilters.group || undefined,
        channel_id: appliedFilters.channel_id || undefined,
      };
      const requests = [
        {
          key: 'queue',
          label: t('待检查队列'),
          request: API.get('/api/model_gateway/observability/health-check/queue', {
            params: {
              ...commonParams,
              limit: 1000,
              queue_type: statusFilter,
              _t: Date.now(),
            },
            disableDuplicate: true,
            skipErrorHandler: true,
          }),
        },
        {
          key: 'history',
          label: t('检测历史'),
          request: API.get('/api/model_gateway/observability/summary', {
            params: {
              ...commonParams,
              hours: historyHours,
              recent_limit: 200,
              scan_limit: 5000,
              view_mode: 'user_requests',
              health_probe_only: true,
              lite: true,
              include_dispatch: true,
            },
            disableDuplicate: true,
            skipErrorHandler: true,
          }),
        },
      ];

      try {
        const results = await Promise.allSettled(requests.map((item) => item.request));
        const failures = [];

        results.forEach((result, index) => {
          const requestMeta = requests[index];
          if (result.status === 'fulfilled') {
            const data = unwrapApiData(result.value);
            if (requestMeta.key === 'queue') {
              setQueueData({
                ...data,
                local_generated_at: Math.floor(Date.now() / 1000),
              });
            } else {
              setHistoryData(data);
            }
            return;
          }
          failures.push(buildRequestErrorDetail(result.reason, requestMeta.label, t));
        });

        if (failures.length > 0) {
          const message = failures
            .map((failure) =>
              [failure.label, failure.message, failure.detail]
                .filter(Boolean)
                .join(': '),
            )
            .join('\n');
          setError(`${t('部分健康检测数据加载失败')}\n${message}`);
          console.error('[ChannelHealthCheck] load failed', failures);
        }
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [appliedFilters, historyHours, statusFilter, t],
  );

  useEffect(() => {
    loadData();
  }, [loadData]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setNowTick(Math.floor(Date.now() / 1000));
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  const loadProbeConfig = useCallback(async () => {
    setSettingsLoading(true);
    try {
      const res = await API.get('/api/model_gateway/config', {
        disableDuplicate: true,
      });
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(t(message || '加载健康探活设置失败'));
        return;
      }
      setProbeConfig(normalizeProbeConfig(data?.setting));
    } catch (err) {
      showError(t('加载健康探活设置失败'));
    } finally {
      setSettingsLoading(false);
    }
  }, [t]);

  const openSettings = useCallback(() => {
    setSettingsVisible(true);
    loadProbeConfig();
  }, [loadProbeConfig]);

  const saveProbeConfig = useCallback(async () => {
    try {
      setSettingsSaving(true);
      const res = await API.patch('/api/model_gateway/config/probe', probeConfig);
      const { success, message, data } = res.data || {};
      if (!success) {
        showError(t(message || '保存健康探活设置失败'));
        return;
      }
      setProbeConfig(normalizeProbeConfig(data?.setting));
      setSettingsVisible(false);
      showSuccess(t('设置已保存，探活调度已更新'));
      loadData(true);
    } catch (err) {
      showError(t('保存健康探活设置失败'));
    } finally {
      setSettingsSaving(false);
    }
  }, [loadData, probeConfig, t]);

  const pendingRows = queueData?.items || [];
  const visiblePendingRows = useMemo(
    () =>
      pendingRows.filter((record) => recordMatchesSearch(record, keyword)),
    [keyword, pendingRows],
  );
  const rawHistoryRecords = useMemo(
    () => historyData?.user_requests?.recent_requests || [],
    [historyData],
  );
  const historyRecords = useMemo(
    () =>
      rawHistoryRecords.filter((record) =>
        recordMatchesSearch(record, keyword),
      ),
    [keyword, rawHistoryRecords],
  );
  const historyScoreChanges = useMemo(
    () => buildHistoryScoreChanges(rawHistoryRecords),
    [rawHistoryRecords],
  );
  const historySuccessCount = historyRecords.filter(
    (record) => record.final_success || record.success,
  ).length;
  const lowScoreCount = queueData?.summary?.low_score_count || 0;
  const recoveryCount = queueData?.summary?.recovery_count || 0;
  const isolatedCount = queueData?.summary?.isolated_count || 0;
  const enabledScoreLabels = (probeConfig.probe_recoverable_score_items || [])
    .map((item) => scoreMetricLabel(item, t))
    .filter(Boolean)
    .join(' / ');
  const probeEnabledText =
    probeConfig.probe_enabled !== false ? t('已启用') : t('未启用');
  const activeFilterCount = Object.values(appliedFilters).filter(Boolean).length;
  const lastUpdated = formatTimestamp(queueData?.summary?.updated_at);
  const queueGeneratedAt = Number(
    queueData?.local_generated_at || queueData?.generated_at || 0,
  );

  const applyFilters = () => {
    setAppliedFilters({
      model: filters.model.trim(),
      group: filters.group.trim(),
      channel_id: filters.channel_id.trim(),
    });
  };

  const resetFilters = () => {
    const next = { model: '', group: '', channel_id: '' };
    setFilters(next);
    setAppliedFilters(next);
    setKeyword('');
    setStatusFilter(ALL_STATUSES);
  };

  const pendingColumns = useMemo(
    () => [
      {
        title: t('优先级'),
        dataIndex: 'priority',
        width: 105,
        render: (value) => {
          const meta = getPriorityMeta(value, t);
          return (
            <Tag color={meta.color} type='light' shape='circle'>
              {meta.label}
            </Tag>
          );
        },
      },
      {
        title: t('模型 / 分组'),
        dataIndex: 'requested_model',
        width: 230,
        render: (_, record) => <ScopeCell record={record} t={t} />,
      },
      {
        title: `${t('渠道')} / ${t('账号')}`,
        dataIndex: 'channel_id',
        width: 300,
        render: (_, record) => <ChannelIdentityCell record={record} t={t} />,
      },
      {
        title: t('状态'),
        dataIndex: 'health_status',
        width: 110,
        render: (value) => {
          const meta = getRuntimeHealthMeta(value, t);
          return (
            <Tag color={meta.color} size='small' type='light'>
              {meta.label}
            </Tag>
          );
        },
      },
      {
        title: t('检查原因'),
        dataIndex: 'reasons',
        width: 270,
        render: (reasons, record) => (
          <Tooltip
            content={
              <div className='ct-channel-health-tooltip'>
                <div>{t('后端根据当前运行态和探活配置生成待检查项')}</div>
                <div>
                  {t('当前探活原因')}: {formatProbeReason(record.probe_trigger_reason, t)}
                </div>
                {record.probe_skip_reason === 'recent_real_request' && (
                  <div>{t('已有真实请求，跳过体检')}</div>
                )}
              </div>
            }
          >
            <div className='ct-channel-health-stack'>
              <ReasonTags reasons={reasons} t={t} />
              <ProbeScoreItemTags items={record.probe_trigger_score_items} t={t} />
              {record.probe_skip_reason === 'recent_real_request' && (
                <Tag color='green' size='small' type='light'>
                  {t('已有真实请求，跳过体检')}
                </Tag>
              )}
            </div>
          </Tooltip>
        ),
      },
      {
        title: t('当前稳定评分'),
        dataIndex: 'score_total',
        width: 170,
        render: (_, record) => <ScoreCompactCell record={record} t={t} />,
      },
      {
        title: t('真实样本'),
        dataIndex: 'real_sample_count_30m',
        width: 170,
        render: (_, record) => <SamplesCell record={record} t={t} />,
      },
      {
        title: t('探活状态'),
        dataIndex: 'last_probe_at',
        width: 230,
        render: (_, record) => {
          const countdownSeconds = getProbeCountdownSeconds(
            record,
            nowTick,
            queueGeneratedAt,
          );
          const countdownReady = countdownSeconds <= 0;
          return (
            <div className='ct-channel-health-stack'>
              <div
                className={`ct-channel-health-countdown ${
                  countdownReady ? 'ct-channel-health-countdown-ready' : ''
                }`}
              >
                <Clock3 size={12} />
                <span>{t('下一次探测')}</span>
                <strong>{formatCountdownDuration(countdownSeconds, t)}</strong>
              </div>
              <Typography.Text>
                {t('上次探活')} {formatRelativeTime(record.last_probe_at, t)}
              </Typography.Text>
              <Typography.Text type='secondary' size='small'>
                {t('上次成功')} {formatRelativeTime(record.last_probe_success_at, t)}
              </Typography.Text>
              {record.probe_recovery_pending && (
                <Tag color='cyan' size='small' type='light'>
                  {t('恢复')} {formatNumber(record.probe_recovery_success_count)}/
                  {formatNumber(record.probe_recovery_required)}
                </Tag>
              )}
            </div>
          );
        },
      },
      {
        title: t('性能'),
        dataIndex: 'success_rate',
        width: 190,
        render: (_, record) => <PerformanceCell record={record} t={t} />,
      },
    ],
    [nowTick, queueGeneratedAt, t],
  );

  const historyColumns = useMemo(
    () => [
      {
        title: t('结果'),
        dataIndex: 'status',
        width: 120,
        render: (_, record) => {
          const meta = getProbeStatusMeta(record, t);
          return (
            <Tag color={meta.color} type='light' shape='circle'>
              {meta.label}
            </Tag>
          );
        },
      },
      {
        title: t('完成时间'),
        dataIndex: 'completed_at',
        width: 180,
        render: (value) => (
          <div className='ct-channel-health-stack'>
            <Typography.Text>{formatTimestamp(value)}</Typography.Text>
            <Typography.Text type='tertiary' size='small'>
              {formatRelativeTime(value, t)}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('检测范围'),
        dataIndex: 'requested_model',
        width: 230,
        render: (_, record) => <ScopeCell record={record} t={t} />,
      },
      {
        title: `${t('渠道')} / ${t('账号')}`,
        dataIndex: 'final_channel_id',
        width: 300,
        render: (_, record) => (
          <ChannelIdentityCell
            record={record}
            fallback={getSelectedScoreCandidate(record)}
            t={t}
          />
        ),
      },
      {
        title: t('检测原因'),
        dataIndex: 'probe_reason',
        width: 170,
        render: (value) => (
          <Tag color='cyan' size='small' type='light'>
            {formatProbeReason(value, t)}
          </Tag>
        ),
      },
      {
        title: t('耗时 / 首包'),
        dataIndex: 'duration_ms',
        width: 170,
        render: (_, record) => (
          <div className='ct-channel-health-stack'>
            <Typography.Text strong>{formatLatency(record.duration_ms)}</Typography.Text>
            <Typography.Text type='secondary' size='small'>
              {t('首包')} {formatLatency(record.ttft_ms)}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('评分变化'),
        dataIndex: 'score_change',
        width: 230,
        render: (_, record) => (
          <ScoreChangeCell
            scoreChange={historyScoreChanges.get(historyRecordKey(record))}
            t={t}
          />
        ),
      },
      {
        title: t('异常信息'),
        dataIndex: 'final_error_category',
        width: 220,
        render: (_, record) => {
          const tags = [];
          if (record.final_error_category) {
            tags.push(
              <Tag key='error' color='orange' size='small' type='light'>
                {record.final_error_category}
              </Tag>,
            );
          }
          if (record.empty_output) {
            tags.push(
              <Tag key='empty' color='red' size='small' type='light'>
                {t('空输出')}
              </Tag>,
            );
          }
          if (record.experience_issue) {
            tags.push(
              <Tag key='experience' color='orange' size='small' type='light'>
                {record.experience_issue}
              </Tag>,
            );
          }
          return tags.length ? (
            <div className='ct-channel-health-tags'>{tags}</div>
          ) : (
            <Typography.Text type='tertiary'>--</Typography.Text>
          );
        },
      },
      {
        title: t('请求ID'),
        dataIndex: 'request_id',
        width: 240,
        render: (value) => (
          <Typography.Text code ellipsis={{ showTooltip: true }}>
            {value || '--'}
          </Typography.Text>
        ),
      },
    ],
    [historyScoreChanges, t],
  );

  return (
    <div className='ct-console-content-wrap'>
      <div className='ct-channel-health-page'>
        <div className='ct-channel-health-hero'>
          <div className='ct-channel-health-title-block'>
            <div className='ct-channel-health-title-icon'>
              <Stethoscope size={24} />
            </div>
            <div>
              <div className='ct-channel-health-eyebrow'>{t('管理员')}</div>
              <h2>{t('渠道健康检测')}</h2>
              <p>
                {t('最近更新时间')}: {lastUpdated}
              </p>
            </div>
          </div>
          <div className='ct-channel-health-actions'>
            <Button
              icon={<Settings size={15} />}
              onClick={openSettings}
            >
              {t('设置')}
            </Button>
            <Select
              value={historyHours}
              onChange={(value) => setHistoryHours(Number(value) || 24)}
              className='ct-channel-health-window-select'
              prefix={t('历史窗口')}
            >
              {HISTORY_HOUR_OPTIONS.map((option) => (
                <Select.Option key={option} value={option}>
                  {option >= 24
                    ? `${Math.round(option / 24)} ${t('天')}`
                    : `${option} ${t('小时')}`}
                </Select.Option>
              ))}
            </Select>
            <Button
              theme='solid'
              type='primary'
              icon={<RefreshCw size={15} />}
              loading={refreshing}
              onClick={() => loadData(true)}
            >
              {t('刷新')}
            </Button>
          </div>
        </div>

        {error && (
          <Banner
            type='danger'
            className='ct-channel-health-banner'
            description={
              <span className='ct-channel-health-error-text'>{error}</span>
            }
            closeIcon={null}
          />
        )}

        <Banner
          type='info'
          className='ct-channel-health-banner'
          description={`${t(
            '待检查队列由后端根据当前运行态和探活配置生成，检测历史来自实际健康探活请求记录。',
          )} ${t('当前探活')}: ${probeEnabledText} · ${t('当前启用触发分值')}: ${enabledScoreLabels || '--'}`}
          closeIcon={null}
        />

        <div className='ct-channel-health-metric-grid'>
          <MetricCard
            label={t('待检查队列')}
            value={formatNumber(queueData?.summary?.pending_count)}
            detail={`${formatNumber(lowScoreCount)} ${t('低评分')} · ${formatNumber(
              recoveryCount,
            )} ${t('恢复中')}`}
            icon={<ListChecks size={18} />}
            tone={Number(queueData?.summary?.pending_count || 0) > 0 ? 'warning' : 'success'}
          />
          <MetricCard
            label={t('恢复确认中')}
            value={formatNumber(recoveryCount)}
            detail={`${formatNumber(isolatedCount)} ${t('隔离或冷却')}`}
            icon={<ShieldCheck size={18} />}
            tone={recoveryCount > 0 ? 'info' : 'success'}
          />
          <MetricCard
            label={t('检测历史')}
            value={formatNumber(historyRecords.length)}
            detail={`${formatNumber(historySuccessCount)} ${t('成功')} · ${formatPercent(
              historyRecords.length ? historySuccessCount / historyRecords.length : 0,
            )}`}
            icon={<History size={18} />}
            tone='default'
          />
          <MetricCard
            label={t('运行键')}
            value={formatNumber(queueData?.summary?.runtime_keys)}
            detail={`${formatNumber(queueData?.summary?.channels)} ${t('渠道')} · ${formatNumber(
              queueData?.summary?.active_concurrency,
            )} ${t('活跃并发')}`}
            icon={<Activity size={18} />}
            tone='default'
          />
        </div>

        <div className='ct-channel-health-filter-panel'>
          <Input
            value={keyword}
            onChange={setKeyword}
            prefix={<Search size={14} />}
            placeholder={`${t('搜索模型、分组、渠道或请求ID')} / ${t('账号凭证')}`}
            className='ct-channel-health-search'
          />
          <Input
            value={filters.model}
            onChange={(value) => setFilters((prev) => ({ ...prev, model: value }))}
            placeholder={t('按模型筛选')}
            prefix={t('模型')}
          />
          <Input
            value={filters.group}
            onChange={(value) => setFilters((prev) => ({ ...prev, group: value }))}
            placeholder={t('按分组筛选')}
            prefix={t('分组')}
          />
          <Input
            value={filters.channel_id}
            onChange={(value) =>
              setFilters((prev) => ({ ...prev, channel_id: value }))
            }
            placeholder={t('按渠道ID筛选')}
            prefix={t('渠道 ID')}
          />
          <Select
            value={statusFilter}
            onChange={setStatusFilter}
            className='ct-channel-health-status-select'
            prefix={t('队列类型')}
          >
            <Select.Option value={ALL_STATUSES}>{t('全部')}</Select.Option>
            <Select.Option value='low_score'>{t('低评分')}</Select.Option>
            <Select.Option value='recovery'>{t('恢复中')}</Select.Option>
            <Select.Option value='isolated'>{t('隔离或冷却')}</Select.Option>
          </Select>
          <div className='ct-channel-health-filter-actions'>
            <Button type='primary' onClick={applyFilters}>
              {activeFilterCount > 0 ? t('筛选中') : t('筛选')}
            </Button>
            <Button type='tertiary' onClick={resetFilters}>
              {t('重置')}
            </Button>
          </div>
        </div>

        <Spin spinning={loading}>
          <div className='ct-channel-health-panel'>
            <Tabs type='button' keepDOM={false}>
              <TabPane
                itemKey='queue'
                tab={
                  <span className='ct-channel-health-tab'>
                    <ListChecks size={15} />
                    {t('待检查队列')}
                    <Tag color='orange' size='small' type='light'>
                      {formatNumber(visiblePendingRows.length)}
                    </Tag>
                  </span>
                }
              >
                <Table
                  size='small'
                  columns={pendingColumns}
                  dataSource={visiblePendingRows}
                  rowKey='row_key'
                  pagination={{
                    pageSize: 12,
                    showSizeChanger: true,
                    pageSizeOpts: [12, 24, 48],
                  }}
                  empty={<Empty description={t('暂无待检查渠道')} />}
                  scroll={{ x: 1775 }}
                />
              </TabPane>
              <TabPane
                itemKey='history'
                tab={
                  <span className='ct-channel-health-tab'>
                    <Clock3 size={15} />
                    {t('检测历史')}
                    <Tag color='cyan' size='small' type='light'>
                      {formatNumber(historyRecords.length)}
                    </Tag>
                  </span>
                }
              >
                <Table
                  size='small'
                  columns={historyColumns}
                  dataSource={historyRecords}
                  rowKey={(record) =>
                    record.request_id || `${record.id}-${record.completed_at}`
                  }
                  pagination={{
                    pageSize: 12,
                    showSizeChanger: true,
                    pageSizeOpts: [12, 24, 48],
                  }}
                  empty={<Empty description={t('暂无健康检测历史')} />}
                  scroll={{ x: 1880 }}
                />
              </TabPane>
            </Tabs>
          </div>
        </Spin>

        <div className='ct-channel-health-footnote'>
          <AlertTriangle size={14} />
          <span>
            {t(
              '系统探活只在近30分钟存在真实客户端请求时运行，且范围限定在相关模型和分组。',
            )}
          </span>
          <CheckCircle2 size={14} />
          <span>
            {t('探活结果会更新评分，但不会增加真实访问样本计数。')}
          </span>
        </div>
        <ProbeSettingsModal
          visible={settingsVisible}
          config={probeConfig}
          loading={settingsLoading}
          saving={settingsSaving}
          onCancel={() => setSettingsVisible(false)}
          onChange={setProbeConfig}
          onSave={saveProbeConfig}
          t={t}
        />
      </div>
    </div>
  );
}

export default ChannelHealthCheck;
