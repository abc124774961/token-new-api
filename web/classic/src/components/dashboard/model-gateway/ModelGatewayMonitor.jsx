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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { createPortal } from 'react-dom';
import {
  Avatar,
  Banner,
  Button,
  Descriptions,
  Empty,
  Input,
  InputNumber,
  Modal,
  Select,
  SideSheet,
  Skeleton,
  Switch,
  Table,
  Tag,
  TextArea,
  Toast,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import { VChart } from '@visactor/react-vchart';
import {
  Activity,
  Ban,
  Bot,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  CheckCircle2,
  Clock3,
  Copy,
  Download,
  Eye,
  Gauge,
  GitBranch,
  Info,
  Layers3,
  ListTree,
  UserRound,
  Search,
  RadioTower,
  RefreshCw,
  RotateCcw,
  ServerCog,
  SlidersHorizontal,
  Timer,
  Trash2,
  Wrench,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers/api';
import {
  copy,
  getQuotaPerUnit,
  renderQuota,
  showError,
  timestamp2string,
} from '../../../helpers';
import { useModelGatewayObservabilityData } from '../../../hooks/dashboard/useModelGatewayObservabilityData';
import DashboardCard from '../DashboardCard';
import './model-gateway.css';

const DEFAULT_HOURS = 24;
const RECENT_LIMIT = 50;
const DEFAULT_USER_REQUEST_PAGE_SIZE = 50;
const USER_REQUEST_PAGE_SIZE_OPTIONS = [20, 50, 100, 200];
const TOP_N = 10;
const STICKY_STORE_LIMIT = 100;
const WINDOW_OPTIONS = [1, 6, 24, 72, 168];
const DEFAULT_TREND_BUCKET = 'auto';
const RETRY_REASON_FIRST_BYTE_TIMEOUT = 'first_byte_timeout';
const TREND_BUCKET_OPTIONS = [
  { value: 'auto', labelKey: '自动粒度' },
  { value: '300', labelKey: '5 分钟' },
  { value: '900', labelKey: '15 分钟' },
  { value: '1800', labelKey: '30 分钟' },
  { value: '3600', labelKey: '1 小时' },
  { value: '21600', labelKey: '6 小时' },
  { value: '86400', labelKey: '1 天' },
];
const EMPTY_IMAGE_SIZE = { width: 150, height: 150 };
const EMPTY_FILTERS = {
  model: '',
  group: '',
  channel_id: '',
  request_id: '',
  circuit_error_type: '',
};
const VIEW_MODES = {
  USER_REQUESTS: 'user_requests',
  OPERATIONS: 'operations',
  ENGINEERING: 'engineering',
};
const SCORE_BOOST_ITEM_KEYS = [
  'completion_rate',
  'upstream_error_rate',
  'ttft_latency',
  'duration_latency',
  'throughput',
  'empty_output_rate',
  'stream_interrupted_rate',
  'concurrency_load',
  'queue_pressure',
  'first_byte_backlog',
  'cost',
];
const CIRCUIT_ERROR_TYPE_OPTIONS = [
  'stream_interrupted',
  'rate_limit',
  'auth',
  'quota',
  'server_error',
  'upstream_error',
];
const REPLAY_BATCH_LIMIT_OPTIONS = [10, 20, 50, 100, 200];
const SCORE_HISTORY_LIMIT = 50;
const MINI_SPARKLINE_CHART_OPTIONS = { mode: 'desktop-browser' };
const MINI_SPARKLINE_COLORS = {
  success: '#10b981',
  warning: '#f97316',
  danger: '#ef4444',
  default: '#14b8a6',
};
const LATENCY_THRESHOLDS = {
  avgDurationMs: { warning: 30000, danger: 60000 },
  p95DurationMs: { warning: 45000, danger: 90000 },
  ttftMs: { warning: 10000, danger: 20000 },
  p95TtftMs: { warning: 15000, danger: 30000 },
};
const EMPTY_REPLAY_BATCH_FILTERS = {
  hours: DEFAULT_HOURS,
  limit: 20,
  model: '',
  group: '',
  channel_id: '',
  error_type: '',
  success: 'all',
  request_ids: '',
};

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function formatNumber(value) {
  return new Intl.NumberFormat().format(Number(value) || 0);
}

function shortenText(value, maxLength = 18) {
  const text = String(value || '').trim();
  if (!text || text.length <= maxLength) return text;
  if (maxLength <= 8) return `${text.slice(0, maxLength)}...`;
  const head = Math.ceil((maxLength - 3) / 2);
  const tail = Math.floor((maxLength - 3) / 2);
  return `${text.slice(0, head)}...${text.slice(-tail)}`;
}

function formatPercent(value, digits = 1) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `${(numeric * 100).toFixed(digits)}%`;
}

function formatAttemptRate(rate, attempts) {
  if (Number(attempts || 0) <= 0) return '--';
  return formatPercent(rate);
}

function formatLatency(value) {
  const latency = Number(value) || 0;
  if (latency <= 0) return '--';
  if (latency >= 1000) return `${(latency / 1000).toFixed(2)}s`;
  return `${Math.round(latency)}ms`;
}

function formatBytes(value) {
  const bytes = Number(value);
  if (!Number.isFinite(bytes) || bytes <= 0) return '--';
  const units = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  );
  const size = bytes / Math.pow(1024, index);
  return `${size.toFixed(index === 0 ? 0 : 2)} ${units[index]}`;
}

function formatScore(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return numeric.toFixed(3).replace(/0+$/, '').replace(/\.$/, '');
}

function formatScoreWithZero(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric < 0) return '--';
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

function formatScoreDeltaMagnitude(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || Math.abs(numeric) < 0.0001) return '0';
  return Math.abs(numeric).toFixed(3).replace(/0+$/, '').replace(/\.$/, '');
}

function scoreDeltaTone(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || Math.abs(numeric) < 0.0001) {
    return 'neutral';
  }
  return numeric > 0 ? 'positive' : 'negative';
}

function scoreDeltaColor(value) {
  const tone = scoreDeltaTone(value);
  if (tone === 'positive') return 'green';
  if (tone === 'negative') return 'orange';
  return 'grey';
}

function scoreMetricLabel(key, t) {
  const normalized = String(key || '').trim();
  switch (normalized) {
    case 'completion_rate':
      return t('完成率分');
    case 'upstream_error_rate':
      return t('上游错误率分');
    case 'ttft_latency':
      return t('首包速度分');
    case 'duration_latency':
      return t('完整耗时分');
    case 'throughput':
      return t('吞吐速度分');
    case 'empty_output_rate':
      return t('空输出率分');
    case 'stream_interrupted_rate':
      return t('流中断率分');
    case 'concurrency_load':
      return t('并发负载分');
    case 'queue_pressure':
      return t('队列压力分');
    case 'first_byte_backlog':
      return t('首包积压分');
    case 'group_priority':
      return t('分组优先级分');
    case 'routing_total':
      return t('本次调度评分');
    case 'cost':
      return t('成本分');
    case 'recoverable_quality_score':
      return t('质量分');
    default:
      return normalized || t('未知指标');
  }
}

function scoreMetricDescription(key, t) {
  const normalized = String(key || '').trim();
  switch (normalized) {
    case 'completion_rate':
      return t('完成请求占比越高，这项越高');
    case 'upstream_error_rate':
      return t('可归因上游错误越少，这项越高');
    case 'ttft_latency':
      return t('首包越快，这项越高');
    case 'duration_latency':
      return t('完整响应耗时越短，这项越高');
    case 'throughput':
      return t('输出吞吐越高，这项越高');
    case 'empty_output_rate':
      return t('空输出越少，这项越高');
    case 'stream_interrupted_rate':
      return t('流式中断越少，这项越高');
    case 'concurrency_load':
      return t('当前并发越接近上限，这项越低');
    case 'queue_pressure':
      return t('队列越深、等待越久，这项越低');
    case 'first_byte_backlog':
      return t('首包等待积压越多，这项越低');
    case 'group_priority':
      return t('按分组优先级换算的固定公式项');
    case 'routing_total':
      return t('用于调度选择，会叠加当前压力和排队状态');
    case 'cost':
      return t('由参考单位成本换算，成本越低这项越高');
    case 'recoverable_quality_score':
      return t('只由可通过探活修复的质量评分项换算');
    default:
      return '';
  }
}

function scoreMetricEntries(value = {}, delta = {}, t) {
  return Object.entries(value || {})
    .filter(
      ([key, score]) =>
        Number.isFinite(Number(score)) && !scoreMetricIsHidden(key),
    )
    .map(([key, score]) => ({
      key,
      label: scoreMetricLabel(key, t),
      description: scoreMetricDescription(key, t),
      score: Number(score),
      delta: Number(delta?.[key] || 0),
    }))
    .sort((left, right) => {
      const leftDelta = Math.abs(left.delta);
      const rightDelta = Math.abs(right.delta);
      if (leftDelta !== rightDelta) return rightDelta - leftDelta;
      return left.label.localeCompare(right.label);
    });
}

function normalizeScoreItemsForDisplay(items = []) {
  return (Array.isArray(items) ? items : [])
    .map((item) => {
      const deltaAvailable = hasOwnFiniteNumber(item, 'delta');
      const weightedDeltaAvailable = hasOwnFiniteNumber(item, 'weighted_delta');
      return {
        ...item,
        score: Number(item?.score || 0),
        weight: Number(item?.weight || 0),
        weighted_score: Number(item?.weighted_score || 0),
        delta: Number(item?.delta || 0),
        weighted_delta: Number(item?.weighted_delta || 0),
        delta_available: deltaAvailable,
        weighted_delta_available: weightedDeltaAvailable,
        raw_number:
          item?.raw_number === undefined || item?.raw_number === null
            ? null
            : Number(item.raw_number),
        reference_number:
          item?.reference_number === undefined ||
          item?.reference_number === null
            ? null
            : Number(item.reference_number),
        formula_parameters: item?.formula_parameters || {},
        sample_count: Number(item?.sample_count || 0),
      };
    })
    .filter((item) => item.key);
}

function scoreItemByKey(items = [], key) {
  return (
    (Array.isArray(items) ? items : []).find((item) => item.key === key) || null
  );
}

function scoreItemSourceLabel(source, t) {
  const normalized = String(source || '').trim();
  switch (normalized) {
    case 'score_stats_latency':
    case 'score_stats_rate':
      return t('评分窗口');
    case 'runtime_latency_samples':
      return t('运行态样本');
    case 'snapshot_fallback':
      return t('快照回填');
    case 'sample_missing':
      return t('样本缺失');
    case 'config':
      return t('配置');
    case 'realtime':
      return t('实时');
    default:
      return normalized || '--';
  }
}

function formatScoreItemRawValue(item, t) {
  if (!item) return '--';
  if (item.missing_reason) return t(item.missing_reason);
  const rawNumber = Number(item.raw_number);
  if (
    item.raw_number !== null &&
    Number.isFinite(rawNumber) &&
    rawNumber >= 0
  ) {
    const unit = String(item.raw_unit || '').trim();
    if (unit === 'ms') return formatLatency(rawNumber);
    if (unit === 'tps') return `${formatNumber(rawNumber)} tps`;
    if (unit === 'ratio') return formatPercent(rawNumber);
    if (unit === 'per_million_tokens') {
      return `${formatCostUnitPrice(rawNumber)} ${t('/M')}`;
    }
    if (unit === 'request') {
      return `${formatCostUnitPrice(rawNumber)} ${t('/次')}`;
    }
    if (['concurrency', 'pending', 'queue_depth'].includes(unit)) {
      return formatNumber(rawNumber);
    }
    return formatScore(rawNumber);
  }
  return item.raw_value || '--';
}

function formatScoreItemSourceAndWindow(item, t) {
  if (!item) return '--';
  const parts = [];
  if (item.window) parts.push(t(item.window));
  const source = scoreItemSourceLabel(item.source, t);
  if (source && source !== '--') parts.push(source);
  return parts.length ? parts.join(' · ') : '--';
}

function scoreItemDeltaByKey(deltas = []) {
  const out = {};
  for (const item of Array.isArray(deltas) ? deltas : []) {
    const key = item?.key;
    if (!key) continue;
    out[key] = item;
  }
  return out;
}

function hasOwnFiniteNumber(value, key) {
  if (!value || !Object.prototype.hasOwnProperty.call(value, key)) return false;
  return Number.isFinite(Number(value[key]));
}

function scoreItemChangeMeta(delta, item) {
  if (hasOwnFiniteNumber(delta, 'weighted_delta')) {
    return {
      value: Number(delta.weighted_delta),
      kind: 'weighted',
    };
  }
  if (hasOwnFiniteNumber(delta, 'delta')) {
    return {
      value: Number(delta.delta),
      kind: 'raw',
    };
  }
  if (item?.weighted_delta_available) {
    return {
      value: Number(item.weighted_delta),
      kind: 'weighted',
    };
  }
  if (item?.delta_available) {
    return {
      value: Number(item.delta),
      kind: 'raw',
    };
  }
  return {
    value: Number(item?.delta || 0),
    kind: 'raw',
  };
}

function scoreItemDisplayRows(items = [], deltas = [], t) {
  const normalized = normalizeScoreItemsForDisplay(items);
  const deltaByKey = scoreItemDeltaByKey(deltas);
  return normalized.map((item) => {
    const delta = deltaByKey[item.key];
    const change = scoreItemChangeMeta(delta, item);
    return {
      ...item,
      name: scoreItemLabel(item, t),
      delta: Number(delta?.delta ?? item.delta ?? 0),
      weighted_delta: Number(delta?.weighted_delta ?? item.weighted_delta ?? 0),
      change_value: change.value,
      change_kind: change.kind,
      before_score: Number(delta?.before_score ?? item.previous_score ?? 0),
      after_score: Number(delta?.after_score ?? item.score ?? 0),
      before_raw_value: delta?.before_raw_value || '',
      after_raw_value: delta?.after_raw_value || item.raw_value || '',
      before_raw_number:
        delta?.before_raw_number === undefined ||
        delta?.before_raw_number === null
          ? null
          : Number(delta.before_raw_number),
      after_raw_number:
        delta?.after_raw_number === undefined ||
        delta?.after_raw_number === null
          ? item.raw_number
          : Number(delta.after_raw_number),
      raw_unit: delta?.raw_unit || item.raw_unit,
    };
  });
}

function scoreHistorySampleLabel(value, t) {
  const count = Number(value || 0);
  return count > 0 ? formatNumber(count) : t('暂无评分样本');
}

function scoreHistorySampleConfidence(item, t) {
  const count = Number(item?.sample_count || 0);
  if (count >= 100) return { label: t('样本充足'), color: 'green' };
  if (count >= 10) return { label: t('样本一般'), color: 'cyan' };
  if (count > 0) return { label: t('样本偏少'), color: 'orange' };
  return { label: t('样本不足'), color: 'grey' };
}

function scoreHistorySourceMeta(item, t) {
  if (item?.source === 'runtime_current') {
    return { label: t('当前运行态'), color: 'cyan' };
  }
  if (item?.is_health_probe) {
    return { label: t('探活请求'), color: 'blue' };
  }
  if (item?.score_sample_source) {
    return {
      label: formatScoreSampleSource(item.score_sample_source, t),
      color: 'blue',
    };
  }
  if (item?.request_id) {
    return { label: t('真实请求'), color: 'green' };
  }
  return { label: t('评分样本'), color: 'grey' };
}

function scoreHistoryStatusMeta(item, t) {
  if (!item) return { label: t('暂无数据'), color: 'grey' };
  if (item.cost_reference_missing) {
    return { label: t('成本参考缺失'), color: 'orange' };
  }
  if (!item.available) {
    return { label: t('不可调度'), color: 'red' };
  }
  if (item.reject_reason) {
    return {
      label: formatChannelStatusReason(item.reject_reason, t),
      color: 'orange',
    };
  }
  return { label: t('可调度'), color: 'green' };
}

function scoreHistoryContributionEntries(current, history, direction, t) {
  const sign = direction === 'negative' ? -1 : 1;
  const rows = [];
  const scoreItemDeltas = Array.isArray(current?.score_item_deltas)
    ? current.score_item_deltas
    : [];
  for (const item of scoreItemDeltas) {
    const hasWeightedDelta = hasOwnFiniteNumber(item, 'weighted_delta');
    const weightedDelta = Number(item?.weighted_delta);
    const rawDelta = Number(item?.delta);
    const value = hasWeightedDelta ? weightedDelta : rawDelta;
    if (!Number.isFinite(value) || Math.abs(value) < 0.0001) continue;
    if (sign > 0 && value <= 0) continue;
    if (sign < 0 && value >= 0) continue;
    const detail = [
      hasWeightedDelta
        ? t('加权贡献变化，才会影响稳定评分总分')
        : t('原始子项变化，不等于总分直接加减'),
      item.reason ||
        scoreMetricDescription(item.key, t) ||
        [item.before_raw_value, item.after_raw_value]
          .filter(Boolean)
          .join(' → '),
    ].filter(Boolean);
    rows.push({
      key: item.key,
      label: item.name ? t(item.name) : scoreMetricLabel(item.key, t),
      value,
      valueKind: hasWeightedDelta ? 'weighted' : 'raw',
      badge: hasWeightedDelta ? t('贡献变化') : t('原始子项变化'),
      description: detail.join(' · '),
    });
  }
  if (!rows.length && !scoreItemDeltas.length) {
    const sourceDelta =
      current?.score_breakdown_delta || history?.metric_deltas || {};
    for (const [key, rawValue] of Object.entries(sourceDelta)) {
      const value = Number(rawValue || 0);
      if (!Number.isFinite(value) || Math.abs(value) < 0.0001) continue;
      if (sign > 0 && value <= 0) continue;
      if (sign < 0 && value >= 0) continue;
      rows.push({
        key,
        label: scoreMetricLabel(key, t),
        value,
        valueKind: 'raw',
        badge: t('原始子项变化'),
        description: [
          t('历史字段：原始子项变化，不等于总分直接加减'),
          scoreMetricDescription(key, t),
        ]
          .filter(Boolean)
          .join(' · '),
      });
    }
  }
  return rows
    .sort((left, right) => Math.abs(right.value) - Math.abs(left.value))
    .slice(0, 3);
}

function scoreHistoryContributionReasonText(entries = [], t) {
  return entries
    .map((entry) => {
      const suffix =
        entry.valueKind === 'raw' ? `（${t('原始子项变化')}）` : '';
      return `${entry.label}${formatScoreDelta(entry.value)}${suffix}`;
    })
    .join(t('、'));
}

function scoreHistoryRecommendation(current, negativeEntries, t) {
  if (!current) return t('暂无评分记录，暂不判断');
  if (Number(current.sample_count || 0) <= 0) {
    return t('样本不足，暂不判断');
  }
  if (current.cost_reference_missing) {
    return t('建议检查成本配置');
  }
  if (!current.available) {
    return t('建议排查不可调度原因');
  }
  const mainNegative = negativeEntries?.[0]?.key || '';
  if (
    ['upstream_error_rate', 'stream_interrupted_rate'].includes(mainNegative)
  ) {
    return t('建议排查上游错误');
  }
  if (
    ['ttft_latency', 'duration_latency', 'throughput'].includes(mainNegative)
  ) {
    return t('建议关注速度变化');
  }
  if (
    ['concurrency_load', 'queue_pressure', 'first_byte_backlog'].includes(
      mainNegative,
    )
  ) {
    return t('建议观察负载与排队');
  }
  if (mainNegative === 'cost') {
    return t('建议检查成本配置');
  }
  return t('可继续观察');
}

function scoreHistorySummaryTitle(delta, t) {
  const numeric = Number(delta || 0);
  if (!Number.isFinite(numeric) || Math.abs(numeric) < 0.0001) {
    return t('本次评分基本稳定');
  }
  return numeric > 0 ? t('本次评分上升') : t('本次评分下降');
}

function scoreHistoryObjectTitle(history, current, t) {
  const channel =
    history?.channel_name ||
    current?.channel_name ||
    (history?.channel_id ? `#${history.channel_id}` : t('未知渠道'));
  const model =
    current?.requested_model ||
    history?.runtime_key?.requested_model ||
    history?.runtime_key?.upstream_model ||
    '--';
  const group =
    current?.selected_group ||
    current?.requested_group ||
    history?.runtime_key?.group ||
    '--';
  return { channel, model, group };
}

function formatTimestamp(timestamp) {
  return timestamp ? timestamp2string(timestamp) : '--';
}

function formatRelativeTime(timestamp, nowSeconds, t) {
  const normalized = normalizeTimestamp(timestamp);
  if (!normalized) return '--';
  const diffSeconds = Math.max(0, Number(nowSeconds || 0) - normalized);
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

function realtimeStatusMeta(
  connectionState,
  fallbackMode,
  fallbackCountdown,
  t,
) {
  if (fallbackMode) {
    return {
      color: 'orange',
      label: t('已降级轮询：{{seconds}} 秒后', {
        seconds: fallbackCountdown,
      }),
    };
  }
  if (connectionState === 'connected') {
    return { color: 'green', label: t('实时已连接') };
  }
  if (connectionState === 'connecting') {
    return { color: 'blue', label: t('实时连接中...') };
  }
  if (connectionState === 'reconnecting') {
    return { color: 'orange', label: t('实时重连中') };
  }
  return { color: 'grey', label: t('实时未连接') };
}

function dynamicBillingRefreshCountdown(
  connectionState,
  fallbackMode,
  fallbackCountdown,
  refreshSeconds,
) {
  const configured = Math.max(0, Number(refreshSeconds || 0));
  if (configured <= 0) return 0;
  if (fallbackMode) return Math.max(0, Number(fallbackCountdown || 0));
  if (connectionState === 'connected') return configured;
  return configured;
}

function dynamicBillingRemainingSeconds(
  latestCalculatedAt,
  refreshSeconds,
  nowMs,
) {
  const interval = Math.max(0, Number(refreshSeconds || 0));
  if (interval <= 0) return 0;
  const anchor = normalizeTimestamp(latestCalculatedAt);
  if (!anchor) return interval;
  const nowSeconds = Math.max(
    0,
    Math.floor(Number(nowMs || Date.now()) / 1000),
  );
  const elapsed = Math.max(0, nowSeconds - anchor);
  const remainder = interval - (elapsed % interval);
  return remainder > 0 ? remainder : interval;
}

function normalizeTimestamp(value) {
  if (value === undefined || value === null || value === '') return null;
  const numeric = Number(value);
  if (Number.isFinite(numeric)) {
    if (numeric <= 0) return null;
    return numeric > 100000000000 ? Math.floor(numeric / 1000) : numeric;
  }
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : null;
}

function formatBucketTimestamp(value) {
  const timestamp = normalizeTimestamp(value);
  return timestamp ? formatTimestamp(timestamp) : '--';
}

function formatBucketRange(record, compact = true) {
  const start = formatBucketTimestamp(record?.bucket_start);
  const end = formatBucketTimestamp(record?.bucket_end);

  if (start === '--') return end;
  if (end === '--') return start;
  if (!compact) return `${start} - ${end}`;

  if (start.slice(0, 10) === end.slice(0, 10)) {
    return `${start.slice(5, 16)} - ${end.slice(11, 16)}`;
  }
  return `${start.slice(5, 16)} - ${end.slice(5, 16)}`;
}

function splitReplayRequestIds(value) {
  return String(value || '')
    .split(/[,\s]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function buildReplayBatchParams(filters) {
  const requestIds = splitReplayRequestIds(filters.request_ids);
  const limit = Number(filters.limit) || EMPTY_REPLAY_BATCH_FILTERS.limit;
  const params = {
    stable_ids: true,
    limit: Math.min(200, Math.max(1, limit)),
  };

  if (requestIds.length > 0) {
    params.request_ids = requestIds.join(',');
    return params;
  }

  params.hours = Number(filters.hours) || DEFAULT_HOURS;
  if (filters.model?.trim()) params.model = filters.model.trim();
  if (filters.group?.trim()) params.group = filters.group.trim();
  if (String(filters.channel_id || '').trim()) {
    params.channel_id = String(filters.channel_id).trim();
  }
  if (filters.error_type?.trim()) {
    params.error_type = filters.error_type.trim();
  }
  if (filters.success === 'success') params.success = true;
  if (filters.success === 'failure') params.success = false;
  return params;
}

function buildReplayBatchDownloadUrl(filters) {
  const params = new URLSearchParams();
  Object.entries(buildReplayBatchParams(filters)).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, String(value));
    }
  });
  params.set('download', 'true');
  return `/api/model_gateway/replay/export/batch?${params.toString()}`;
}

function formatDurationSeconds(value, t) {
  const seconds = Number(value);
  if (!Number.isFinite(seconds) || seconds < 0) return '--';
  if (seconds < 60) return `${Math.floor(seconds)} ${t('秒')}`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} ${t('分钟')}`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} ${t('小时')}`;
  return `${Math.floor(seconds / 86400)} ${t('天')}`;
}

function formatStickyExpiry(record) {
  const expiresAt = normalizeTimestamp(record?.expires_at || record?.expires);
  return expiresAt ? formatTimestamp(expiresAt) : '--';
}

function formatStickySource(value, t) {
  const normalized = String(value || '').trim();
  if (!normalized) return '--';
  if (normalized === 'user_sticky' || normalized === 'user') {
    return t('用户粘滞');
  }
  if (normalized === 'cache_affinity') {
    return t('缓存亲和');
  }
  return normalized;
}

function formatTechnicalCode(value) {
  const normalized = String(value || '').trim();
  if (!normalized) return '';
  return normalized.replace(/_/g, ' ');
}

function formatCircuitErrorType(value, t) {
  const normalized = normalizeCircuitErrorType(value);
  if (!normalized) return t('未知');
  switch (normalized) {
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
      return normalized;
  }
}

function normalizeCircuitErrorType(value) {
  return String(value || '').trim();
}

function circuitReasonCount(items, type) {
  const normalized = normalizeCircuitErrorType(type);
  if (!normalized || !Array.isArray(items)) return 0;
  return items.reduce((total, item) => {
    if (normalizeCircuitErrorType(item?.reason) !== normalized) return total;
    return total + (Number(item?.count) || 0);
  }, 0);
}

function circuitErrorCountMapValue(map, type) {
  const normalized = normalizeCircuitErrorType(type);
  if (!normalized || !map || typeof map !== 'object') return 0;
  return Number(map[normalized]) || 0;
}

function trendMatchesCircuitError(record, type) {
  const normalized = normalizeCircuitErrorType(type);
  if (!normalized) return true;
  return (
    circuitReasonCount(record?.circuit_error_types, normalized) > 0 ||
    circuitReasonCount(record?.circuit_error_counts, normalized) > 0 ||
    circuitReasonCount(record?.circuit_open_reasons, normalized) > 0
  );
}

function runtimeItemMatchesCircuitError(item, type) {
  const normalized = normalizeCircuitErrorType(type);
  if (!normalized) return true;
  return (
    normalizeCircuitErrorType(item?.circuit_open_reason) === normalized ||
    circuitErrorCountMapValue(item?.circuit_error_counts, normalized) > 0
  );
}

function riskEventMatchesCircuitError(event, type) {
  const normalized = normalizeCircuitErrorType(type);
  if (!normalized) return true;
  if (
    event?.event_type === 'circuit_error_type' ||
    event?.event_type === 'circuit_open_reason'
  ) {
    return normalizeCircuitErrorType(event?.reason) === normalized;
  }
  return (
    normalized === 'stream_interrupted' &&
    (event?.event_type === 'stream_interrupted' ||
      event?.status === 'stream_interrupted')
  );
}

function filterReasonCounts(items, type) {
  const normalized = normalizeCircuitErrorType(type);
  if (!normalized || !Array.isArray(items)) return items;
  return items.filter(
    (item) => normalizeCircuitErrorType(item?.reason) === normalized,
  );
}

function filterRuntimeStatusByCircuitError(runtimeStatus, type) {
  const normalized = normalizeCircuitErrorType(type);
  if (!normalized || !runtimeStatus) return runtimeStatus;
  const items = (runtimeStatus.items || []).filter((item) =>
    runtimeItemMatchesCircuitError(item, normalized),
  );
  const channelIDs = new Set(
    items
      .map((item) => Number(item.channel_id))
      .filter((channelID) => channelID > 0),
  );
  return {
    ...runtimeStatus,
    summary: {
      ...(runtimeStatus.summary || {}),
      runtime_keys: items.length,
      channels: channelIDs.size,
      active_concurrency: items.reduce(
        (total, item) => total + (Number(item.active_concurrency) || 0),
        0,
      ),
      queued_requests: items.reduce(
        (total, item) => total + (Number(item.queue_depth) || 0),
        0,
      ),
      queue_channels: items.filter((item) => Number(item.queue_depth) > 0)
        .length,
      circuit_open: items.filter((item) => item.circuit_open).length,
      circuit_half_open: items.filter(
        (item) => item.circuit_state === 'half_open',
      ).length,
      cooldown_channels: items.filter((item) => item.cooldown).length,
      failure_avoidance_channels: items.filter((item) => item.failure_avoidance)
        .length,
    },
    items,
  };
}

function filterRiskSnapshotByCircuitError(risk, type) {
  const normalized = normalizeCircuitErrorType(type);
  if (!normalized || !risk) return risk;
  return {
    ...risk,
    top_circuit_open_reasons: filterReasonCounts(
      risk.top_circuit_open_reasons,
      normalized,
    ),
    top_circuit_error_types: filterReasonCounts(
      risk.top_circuit_error_types,
      normalized,
    ),
  };
}

function getStickyKeyID(record) {
  return record?.key_id || record?.keyID || '';
}

function clampRate(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return 0;
  return Math.min(1, Math.max(0, numeric));
}

function getSuccessTone(rate, attempts) {
  if (Number(attempts) <= 0) return 'default';
  if (Number(rate) >= 0.98) return 'success';
  if (Number(rate) >= 0.9) return 'warning';
  return 'danger';
}

function getThresholdTone(value, thresholds) {
  const numeric = Number(value || 0);
  if (numeric <= 0) return 'success';
  if (numeric >= thresholds.danger) return 'danger';
  if (numeric >= thresholds.warning) return 'warning';
  return 'success';
}

function isLatencyWarning(value, thresholds) {
  return Number(value || 0) >= thresholds.warning;
}

function isHealthProbeRecord(record) {
  return Boolean(
    record?.is_health_probe || record?.request_meta?.is_health_probe,
  );
}

function getStatusMeta(record, t) {
  if (isHealthProbeRecord(record)) {
    if (record?.success) {
      return { color: 'cyan', label: t('健康探活') };
    }
    return { color: 'orange', label: t('探活异常') };
  }
  if (record?.kind === 'user_request_detail') {
    if (record?.client_aborted) {
      return { color: 'grey', label: t('客户端中断') };
    }
    if (record?.success || record?.final_success) {
      return { color: 'green', label: t('成功') };
    }
    if (record?.stream_interrupted) {
      return { color: 'orange', label: t('流中断') };
    }
    return { color: 'red', label: t('失败') };
  }
  if (isDispatch(record)) {
    return { color: 'blue', label: t('已调度') };
  }
  if (record?.success) {
    return { color: 'green', label: t('成功') };
  }
  if (record?.stream_interrupted) {
    return { color: 'orange', label: t('流中断') };
  }
  return { color: 'red', label: t('失败') };
}

function isUserQuotaExhaustedRecord(record) {
  const category = String(
    record?.error_category || record?.final_error_category || '',
  )
    .trim()
    .toLowerCase();
  const status = String(record?.status || '')
    .trim()
    .toLowerCase();
  const errorCode = String(
    record?.error_code || record?.dispatch_record?.error_code || '',
  )
    .trim()
    .toLowerCase();
  const dispatchCategory = String(record?.dispatch_record?.error_category || '')
    .trim()
    .toLowerCase();
  const balanceMarked =
    record?.balance_insufficient === true ||
    record?.request_meta?.balance_insufficient === true ||
    record?.dispatch_record?.balance_insufficient === true;
  return (
    category === 'user_quota_exhausted' ||
    status === 'user_quota_exhausted' ||
    dispatchCategory === 'user_quota_exhausted' ||
    (errorCode === 'insufficient_user_quota' && !balanceMarked)
  );
}

function isBalanceInsufficientStatus(record) {
  if (isUserQuotaExhaustedRecord(record)) return false;
  const reason = String(record?.status_reason || record?.reject_reason || '')
    .trim()
    .toLowerCase();
  const category = String(
    record?.error_category || record?.final_error_category || '',
  )
    .trim()
    .toLowerCase();
  return (
    record?.balance_insufficient === true ||
    record?.request_meta?.balance_insufficient === true ||
    category === 'balance_or_quota' ||
    reason === 'balance_insufficient' ||
    reason.includes('余额不足')
  );
}

function formatChannelStatusReason(reason, t) {
  if (!reason) return '';
  const normalized = String(reason).trim().toLowerCase();
  if (
    normalized === 'balance_insufficient' ||
    normalized.includes('余额不足')
  ) {
    return t('余额不足');
  }
  if (normalized === 'local_concurrency_full') return t('并发压力');
  if (
    normalized === 'concurrency_full' ||
    normalized === 'learned_concurrency_full' ||
    normalized === 'concurrency_saturated' ||
    normalized === 'cold_start_probe_full'
  ) {
    return t('并发压力');
  }
  if (normalized === 'slow_ttft') {
    return t('首包过慢');
  }
  if (normalized === 'ttft_pending') {
    return t('首包等待降权');
  }
  if (normalized === 'circuit_open') {
    return t('熔断打开');
  }
  if (normalized === 'circuit_half_open') {
    return t('熔断半开');
  }
  if (normalized === 'cooldown') {
    return t('临时冷却');
  }
  if (normalized === 'timeout_recovery') {
    return t('频繁超时降级中');
  }
  if (normalized === 'score_anomaly_fast_probe') {
    return t('分数异常快速恢复');
  }
  if (normalized === 'failure_avoidance') {
    return t('近期失败恢复中');
  }
  if (
    normalized === 'already_failed_in_request' ||
    normalized === 'routing_slot_reserved'
  ) {
    return t('本次请求已尝试失败');
  }
  if (normalized === 'max_depth_reached') {
    return t('排队已满');
  }
  return String(reason);
}

function formatStickyBreakReason(reason, t) {
  if (!reason) return '';
  const normalized = String(reason).trim().toLowerCase();
  if (normalized === 'score_below_threshold') {
    return t('粘滞候选未达保留阈值，改选调度分更高候选');
  }
  if (normalized === 'cost_first_cheaper_higher_score') {
    return t('成本优先发现明显更低成本且调度分更高的候选');
  }
  if (normalized === 'cost_first_cheaper_speed_acceptable') {
    return t('成本优先发现明显更低成本且速度影响可接受的候选');
  }
  return formatChannelStatusReason(reason, t);
}

function formatProbeReason(value, t) {
  const normalized = String(value || '').trim();
  switch (normalized) {
    case 'missing_samples':
      return t('缺少样本');
    case 'low_score':
      return t('低分恢复探测');
    case 'failure_avoidance':
      return t('近期失败恢复中');
    case 'timeout_recovery':
      return t('等待恢复探活');
    case 'score_anomaly_fast_probe':
      return t('分数异常快速恢复');
    case 'cooldown':
      return t('冷却恢复探测');
    case 'long_no_success':
      return t('长期未成功');
    case 'circuit_half_open':
      return t('熔断半开');
    case 'sampling':
      return t('常规抽样');
    default:
      return normalized || t('健康探活');
  }
}

function getProbeReason(record) {
  return record?.probe_reason || record?.request_meta?.probe_reason || '';
}

function formatProbeScope(record, t) {
  if (!record) return '';
  const runtimeKey =
    record.runtime_key || record.request_meta?.runtime_key || {};
  const model =
    record.requested_model ||
    runtimeKey.requested_model ||
    runtimeKey.upstream_model ||
    '';
  const group =
    record.selected_group ||
    record.group ||
    runtimeKey.group ||
    record.requested_group ||
    '';
  const parts = [model, group].filter(Boolean);
  if (!parts.length) return '';
  return t('探活对象：{{scope}}', { scope: parts.join(' / ') });
}

function formatProbeReasonWithScope(record, t) {
  const reason = formatProbeReason(getProbeReason(record), t);
  const scope = formatProbeScope(record, t);
  if (!reason) return scope;
  if (!scope) return reason;
  return `${reason} · ${scope}`;
}

function isDispatch(record) {
  return record?.kind === 'dispatch';
}

function pickDispatchDetailRecord(records) {
  if (!Array.isArray(records) || records.length === 0) return null;
  const withCandidates = records.find(
    (record) =>
      Array.isArray(getCandidateExplanations(record)) &&
      getCandidateExplanations(record).length > 0,
  );
  return (
    withCandidates ||
    records.find((record) => isDispatch(record) && record?.smart_handled) ||
    records.find((record) => isDispatch(record)) ||
    records[0]
  );
}

function pickAttemptDetailRecord(records) {
  if (!Array.isArray(records) || records.length === 0) return null;
  const attempts = userRequestAttemptRecords(records);
  if (!attempts.length) return null;
  const successfulAttempt = [...attempts]
    .reverse()
    .find((record) => record?.success);
  if (successfulAttempt) return successfulAttempt;
  return attempts[attempts.length - 1] || null;
}

function isAttemptClientAborted(attempt) {
  const category = String(attempt?.error_category || '').toLowerCase();
  return (
    attempt?.client_aborted === true ||
    Number(attempt?.status_code || 0) === 499 ||
    category === 'client_aborted' ||
    category === 'channel_induced_client_abort' ||
    category.includes('client_abort') ||
    category.includes('client_gone')
  );
}

function userRequestAttemptRecords(records) {
  if (!Array.isArray(records) || records.length === 0) return [];
  return records
    .filter((record) => !isDispatch(record))
    .sort(
      (left, right) =>
        Number(left?.attempt_index || 0) - Number(right?.attempt_index || 0) ||
        Number(left?.created_at || 0) - Number(right?.created_at || 0),
    );
}

function buildUserRequestDetailRecord(userRequest, records) {
  const dispatch = pickDispatchDetailRecord(records);
  const attempt = pickAttemptDetailRecord(records);
  const attemptRecords = userRequestAttemptRecords(records);
  const base = dispatch || attempt || {};
  const dispatchCandidates =
    dispatch?.candidate_explanations ||
    dispatch?.request_meta?.candidate_explanations ||
    [];
  const baseCandidates =
    base?.candidate_explanations ||
    base?.request_meta?.candidate_explanations ||
    [];
  const candidateExplanations = dispatchCandidates.length
    ? dispatchCandidates
    : baseCandidates;
  const finalChannelId = Number(
    userRequest?.final_channel_id ||
      attempt?.channel_id ||
      base?.channel_id ||
      0,
  );
  const finalChannelName =
    userRequest?.final_channel_name ||
    attempt?.channel_name ||
    base?.channel_name ||
    '';
  const latestAttemptClientAborted = isAttemptClientAborted(attempt);
  const finalStatusCode = latestAttemptClientAborted
    ? attempt?.status_code || 499
    : userRequest?.final_status_code || attempt?.status_code || 0;
  const finalErrorCategory =
    userRequest?.channel_induced_client_abort ||
    attempt?.channel_induced_client_abort ||
    attempt?.request_meta?.channel_induced_client_abort
      ? 'channel_induced_client_abort'
      : latestAttemptClientAborted
        ? 'client_aborted'
        : userRequest?.final_error_category || attempt?.error_category || '';
  return {
    ...base,
    ...attempt,
    kind: 'user_request_detail',
    id: attempt?.id || dispatch?.id || userRequest?.id,
    created_at:
      userRequest?.created_at || dispatch?.created_at || attempt?.created_at,
    completed_at: userRequest?.completed_at || attempt?.created_at,
    request_id: userRequest?.request_id || base?.request_id,
    requested_model:
      userRequest?.requested_model ||
      attempt?.requested_model ||
      dispatch?.requested_model,
    requested_group:
      userRequest?.requested_group ||
      dispatch?.requested_group ||
      attempt?.requested_group,
    selected_group:
      userRequest?.selected_group ||
      dispatch?.selected_group ||
      attempt?.selected_group,
    actual_group:
      userRequest?.actual_group ||
      dispatch?.actual_group ||
      attempt?.actual_group,
    actual_group_ratio:
      userRequest?.actual_group_ratio ||
      dispatch?.actual_group_ratio ||
      attempt?.actual_group_ratio,
    channel_id: finalChannelId,
    channel_name: finalChannelName,
    actual_channel_id: finalChannelId,
    actual_channel_name: finalChannelName,
    upstream_cost_total:
      userRequest?.upstream_cost_total ||
      attempt?.upstream_cost_total ||
      dispatch?.upstream_cost_total,
    upstream_cost_model:
      userRequest?.upstream_cost_model ||
      attempt?.upstream_cost_model ||
      dispatch?.upstream_cost_model,
    upstream_cost_breakdown:
      userRequest?.upstream_cost_breakdown ||
      attempt?.upstream_cost_breakdown ||
      dispatch?.upstream_cost_breakdown,
    upstream_cost_source:
      userRequest?.upstream_cost_source ||
      attempt?.upstream_cost_source ||
      dispatch?.upstream_cost_source,
    upstream_cost_accuracy:
      userRequest?.upstream_cost_accuracy ||
      attempt?.upstream_cost_accuracy ||
      dispatch?.upstream_cost_accuracy,
    is_health_probe:
      userRequest?.is_health_probe === true ||
      attempt?.is_health_probe === true ||
      dispatch?.is_health_probe === true ||
      dispatch?.request_meta?.is_health_probe === true,
    probe_reason:
      userRequest?.probe_reason ||
      attempt?.probe_reason ||
      dispatch?.probe_reason ||
      dispatch?.request_meta?.probe_reason ||
      '',
    success: userRequest?.final_success === true || attempt?.success === true,
    final_success: userRequest?.final_success === true,
    status_code: finalStatusCode,
    final_status_code: finalStatusCode,
    error_category: finalErrorCategory,
    final_error_category: finalErrorCategory,
    warning_level:
      userRequest?.warning_level ||
      attempt?.warning_level ||
      attempt?.request_meta?.warning_level ||
      '',
    warning_flags:
      userRequest?.warning_flags ||
      attempt?.warning_flags ||
      attempt?.request_meta?.warning_flags ||
      [],
    warning_message:
      userRequest?.warning_message ||
      attempt?.warning_message ||
      attempt?.request_meta?.warning_message ||
      '',
    channel_induced_client_abort:
      userRequest?.channel_induced_client_abort === true ||
      attempt?.channel_induced_client_abort === true ||
      attempt?.request_meta?.channel_induced_client_abort === true,
    retry_action: attempt?.retry_action || '',
    retry_reason:
      attempt?.retry_reason ||
      attempt?.request_meta?.retry_reason ||
      base?.retry_reason ||
      base?.request_meta?.retry_reason ||
      '',
    duration_ms: userRequest?.duration_ms || attempt?.duration_ms || 0,
    ttft_ms: userRequest?.ttft_ms || attempt?.ttft_ms || 0,
    client_aborted:
      userRequest?.client_aborted === true ||
      attempt?.client_aborted === true ||
      latestAttemptClientAborted,
    stream_interrupted:
      userRequest?.stream_interrupted === true ||
      attempt?.stream_interrupted === true,
    empty_output:
      userRequest?.empty_output === true || attempt?.empty_output === true,
    experience_issue:
      userRequest?.experience_issue || attempt?.experience_issue || '',
    recovered: userRequest?.recovered === true,
    attempts: userRequest?.attempts || records?.length || 0,
    attempt_records: attemptRecords,
    score_total: dispatch?.score_total || base?.score_total || 0,
    score_breakdown: dispatch?.score_breakdown || base?.score_breakdown,
    candidate_groups: dispatch?.candidate_groups || base?.candidate_groups,
    candidate_explanations: candidateExplanations,
    selected_reason: dispatch?.selected_reason || base?.selected_reason,
    queue_enabled: dispatch?.queue_enabled || base?.queue_enabled,
    queue_wait_ms: dispatch?.queue_wait_ms || base?.queue_wait_ms,
    queue_depth: dispatch?.queue_depth || base?.queue_depth,
    queue_capacity: dispatch?.queue_capacity || base?.queue_capacity,
    sticky_source: dispatch?.sticky_source || base?.sticky_source,
    sticky_retained: dispatch?.sticky_retained || base?.sticky_retained,
    sticky_break: dispatch?.sticky_break || base?.sticky_break,
    cache_affinity: dispatch?.cache_affinity || base?.cache_affinity,
    used_channels: attempt?.used_channels || base?.used_channels,
    request_meta: {
      ...(dispatch?.request_meta || {}),
      ...(attempt?.request_meta || {}),
      ...(attempt?.retry_reason ? { retry_reason: attempt.retry_reason } : {}),
      ...(userRequest?.is_health_probe ? { is_health_probe: true } : {}),
      ...(userRequest?.probe_reason
        ? { probe_reason: userRequest.probe_reason }
        : {}),
      ...(userRequest?.warning_level
        ? { warning_level: userRequest.warning_level }
        : {}),
      ...(Array.isArray(userRequest?.warning_flags)
        ? { warning_flags: userRequest.warning_flags }
        : {}),
      ...(userRequest?.warning_message
        ? { warning_message: userRequest.warning_message }
        : {}),
      ...(userRequest?.channel_induced_client_abort
        ? { channel_induced_client_abort: true }
        : {}),
      candidate_explanations: candidateExplanations,
    },
  };
}

function userRequestDispatchRecord(record) {
  const dispatch = record?.dispatch_record || record?.dispatchRecord || null;
  if (dispatch && typeof dispatch === 'object') return dispatch;
  return null;
}

function userRequestScoreRecord(record) {
  const dispatch = userRequestDispatchRecord(record);
  if (!dispatch) return null;
  const detail = buildUserRequestDetailRecord(record, [dispatch]);
  return {
    ...detail,
    candidate_explanations:
      dispatch?.candidate_explanations ||
      dispatch?.request_meta?.candidate_explanations ||
      detail?.candidate_explanations ||
      [],
  };
}

function selectedUserRequestScoreCandidate(record) {
  const scoreRecord = userRequestScoreRecord(record);
  if (!scoreRecord) return null;
  return findSelectedCandidate(
    scoreRecord,
    getCandidateExplanations(scoreRecord),
  );
}

function scoreHistoryCandidateFromRecord(record) {
  if (!record) return null;
  const selectedCandidate = findSelectedCandidate(
    record,
    getCandidateExplanations(record),
  );
  if (selectedCandidate) {
    return selectedCandidate;
  }
  const channelId = Number(record.actual_channel_id || record.channel_id || 0);
  if (!channelId) return null;
  const requestMetaRuntimeKey =
    record.runtime_key || record.request_meta?.runtime_key || {};
  return {
    channel_id: channelId,
    channel_name: record.actual_channel_name || record.channel_name || '',
    runtime_key: {
      requested_model:
        requestMetaRuntimeKey.requested_model || record.requested_model || '',
      upstream_model:
        requestMetaRuntimeKey.upstream_model ||
        record.upstream_model ||
        record.requested_model ||
        '',
      channel_id: channelId,
      group:
        requestMetaRuntimeKey.group ||
        record.actual_group ||
        record.selected_group ||
        record.requested_group ||
        '',
      endpoint_type:
        requestMetaRuntimeKey.endpoint_type || record.endpoint_type || '',
      capability_fingerprint:
        requestMetaRuntimeKey.capability_fingerprint ||
        record.provider_profile ||
        '',
    },
  };
}

function scoreHistoryChangeForRecord(history, record) {
  const items = Array.isArray(history?.items) ? history.items : [];
  const requestId = String(record?.request_id || '').trim();
  if (requestId) {
    const matchedIndex = items.findIndex(
      (item) => String(item?.request_id || '').trim() === requestId,
    );
    if (matchedIndex >= 0) {
      const item = items[matchedIndex];
      return {
        delta: Number(item?.score_delta || 0),
        hasComparison: matchedIndex + 1 < items.length,
      };
    }
  }
  if (history?.current) {
    return {
      delta: Number(history?.score_delta || history.current?.score_delta || 0),
      hasComparison: Boolean(history?.previous),
    };
  }
  return { delta: 0, hasComparison: false };
}

function SummaryMetric({ icon: Icon, label, value, detail, tone = 'default' }) {
  const colorMap = {
    default: 'blue',
    success: 'green',
    warning: 'orange',
    danger: 'red',
  };
  const dashboardToneMap = {
    default: 'info',
    success: 'uptime',
    warning: 'notice',
    danger: 'notice',
  };

  return (
    <DashboardCard
      className={`ct-model-gateway-metric ct-model-gateway-metric-${tone}`}
      tone={dashboardToneMap[tone] || 'info'}
      bodyStyle={{ height: '100%' }}
    >
      <div className='ct-model-gateway-metric-inner'>
        <div className='ct-model-gateway-metric-copy'>
          <span>{label}</span>
          <strong>{value}</strong>
          <small>{detail}</small>
        </div>
        <Avatar size='small' color={colorMap[tone] || 'blue'}>
          <Icon size={15} />
        </Avatar>
      </div>
    </DashboardCard>
  );
}

function EmptyState({ t }) {
  return (
    <Empty
      image={<IllustrationConstruction style={EMPTY_IMAGE_SIZE} />}
      darkModeImage={<IllustrationConstructionDark style={EMPTY_IMAGE_SIZE} />}
      title={t('暂无智能模型网关观测数据')}
      description={t('当前时间窗口内还没有调度记录')}
    />
  );
}

function MetricSkeleton() {
  return (
    <div className='ct-model-gateway-metric-grid'>
      {Array.from({ length: 6 }).map((_, index) => (
        <Skeleton
          key={index}
          loading
          active
          placeholder={
            <Skeleton.Paragraph
              rows={3}
              style={{ height: 108, borderRadius: 16 }}
            />
          }
        />
      ))}
    </div>
  );
}

function AggregateNameCell({ record, type }) {
  const { t } = useTranslation();
  const icon =
    type === 'model' ? (
      <Bot size={16} />
    ) : type === 'group' ? (
      <Layers3 size={16} />
    ) : type === 'profile' ? (
      <ServerCog size={16} />
    ) : type === 'proxy' ? (
      <GitBranch size={16} />
    ) : (
      <RadioTower size={16} />
    );
  const label = record.name || record.key || t('未知');
  const balanceInsufficient =
    type === 'channel' && isBalanceInsufficientStatus(record);
  const statusReason = formatChannelStatusReason(record?.status_reason, t);

  return (
    <div className='ct-model-gateway-aggregate-name'>
      <Avatar size='extra-small' color={type === 'channel' ? 'cyan' : 'blue'}>
        {icon}
      </Avatar>
      <div className='min-w-0'>
        <div className='ct-model-gateway-aggregate-title-row'>
          <div className='ct-model-gateway-aggregate-title' title={label}>
            {label}
          </div>
          {balanceInsufficient && (
            <Tooltip
              content={statusReason || t('渠道余额不足，已暂停调度')}
              position='top'
            >
              <Tag color='red' size='small' type='light' shape='circle'>
                {t('余额不足')}
              </Tag>
            </Tooltip>
          )}
        </div>
        {type === 'channel' && record.channel_id > 0 && (
          <Typography.Text type='secondary' size='small'>
            #{record.channel_id}
          </Typography.Text>
        )}
        {(type === 'profile' || type === 'proxy') && (
          <Typography.Text type='secondary' size='small'>
            {formatNumber(record.dispatches)} {t('次调度')}
          </Typography.Text>
        )}
      </div>
    </div>
  );
}

function ScoreBreakdown({ value }) {
  const entries = Object.entries(value || {})
    .filter(([, score]) => Number.isFinite(Number(score)))
    .sort((a, b) => Number(b[1]) - Number(a[1]))
    .slice(0, 4);

  if (!entries.length)
    return <Typography.Text type='tertiary'>--</Typography.Text>;

  return (
    <div className='ct-model-gateway-score-list'>
      {entries.map(([key, score]) => (
        <Tag key={key} color='cyan' size='small' shape='circle' type='light'>
          {key}: {formatScore(score)}
        </Tag>
      ))}
    </div>
  );
}

function scoreItemLabel(item, t) {
  const key = item?.key || '';
  const name = item?.name || '';
  if (name) return t(name);
  return scoreMetricLabel(key, t);
}

function ScoreItemsMiniList({ items = [], t, limit = 6 }) {
  const entries = (Array.isArray(items) ? items : [])
    .filter(
      (item) =>
        Number.isFinite(Number(item?.score)) &&
        Number(item?.weight) > 0 &&
        !item?.missing_reason,
    )
    .slice(0, limit);
  if (!entries.length) return null;
  return (
    <>
      <div className='ct-model-gateway-cost-tooltip-divider' />
      {entries.map((item) => (
        <div className='ct-model-gateway-cost-tooltip-row' key={item.key}>
          <span title={item.raw_value || item.formula || ''}>
            {scoreItemLabel(item, t)}
          </span>
          <strong>
            {formatScore(item.score)}
            <small style={{ marginLeft: 6, fontWeight: 500 }}>
              {formatPercent(Number(item.weight || 0))}
            </small>
          </strong>
        </div>
      ))}
    </>
  );
}

function routingScoreItemsForDisplay(items = []) {
  return normalizeScoreItemsForDisplay(items)
    .filter((item) => Number(item.weight) > 0 && !item.missing_reason)
    .sort((left, right) => {
      if (right.weight !== left.weight) return right.weight - left.weight;
      return right.weighted_score - left.weighted_score;
    });
}

function RoutingScoreItemsPanel({ items = [], total, t }) {
  const entries = routingScoreItemsForDisplay(items);
  const hasTotal = Number.isFinite(Number(total)) && Number(total) > 0;
  if (!entries.length && !hasTotal) return null;
  return (
    <div className='ct-model-gateway-routing-score-panel'>
      <div className='ct-model-gateway-routing-score-panel-head'>
        <div>
          <span>{t('本次调度评分构成')}</span>
          <strong>{hasTotal ? formatScore(total) : '--'}</strong>
        </div>
        <Typography.Text type='tertiary' size='small'>
          {t('按本次调度权重计算')}
        </Typography.Text>
      </div>
      {entries.length ? (
        <div className='ct-model-gateway-routing-score-grid'>
          {entries.map((item) => (
            <Tooltip
              key={item.key}
              content={`${scoreItemLabel(item, t)} · ${t('原始数据')}: ${formatScoreItemRawValue(item, t)} · ${t('权重')}: ${formatPercent(item.weight)} · ${t('贡献')}: ${formatScore(item.weighted_score)}`}
            >
              <div
                className={`ct-model-gateway-routing-score-item ct-model-gateway-routing-score-item-${item.key}`}
              >
                <div className='ct-model-gateway-routing-score-item-top'>
                  <span>{scoreItemLabel(item, t)}</span>
                  <strong>{formatScore(item.score)}</strong>
                </div>
                <div className='ct-model-gateway-routing-score-item-meta'>
                  <span>{formatScoreItemRawValue(item, t)}</span>
                  <span>
                    {t('权重')} {formatPercent(item.weight)}
                  </span>
                </div>
                <div className='ct-model-gateway-routing-score-bar'>
                  <i
                    style={{
                      width: `${Math.max(2, Math.min(100, item.score * 100))}%`,
                    }}
                  />
                </div>
                <div className='ct-model-gateway-routing-score-item-foot'>
                  {t('贡献')} {formatScore(item.weighted_score)}
                </div>
              </div>
            </Tooltip>
          ))}
        </div>
      ) : (
        <Typography.Text type='tertiary' size='small'>
          {t('暂无评分拆解')}
        </Typography.Text>
      )}
    </div>
  );
}

function QueueStickyTags({
  record,
  t,
  compact = false,
  showQueue = true,
  showSticky = true,
}) {
  const tagProps = compact ? { size: 'small' } : {};
  const tags = [];
  const queued = Number(record?.queued_dispatches || 0);
  const queueEnabled = Number(record?.queue_enabled_dispatches || 0);
  const queueWait = Number(record?.avg_queue_wait_ms || record?.queue_wait_ms);
  const queueDepth = Number(record?.queue_depth);
  const queueCapacity = Number(record?.queue_capacity);
  const stickyRoutes = Number(record?.sticky_routes || 0);
  const stickyRetained = Number(record?.sticky_retained || 0);
  const stickyBroken = Number(record?.sticky_broken || 0);
  const cacheAffinity = Number(record?.cache_affinity_routes || 0);
  const hasStickyRetainedCount = typeof record?.sticky_retained !== 'boolean';

  if (
    showQueue &&
    (record?.queue_enabled || queueEnabled > 0 || queued > 0 || queueWait > 0)
  ) {
    tags.push(
      <Tag key='queue-wait' color='blue' type='light' {...tagProps}>
        {t('队列等待')} {formatLatency(queueWait)}
      </Tag>,
    );
  }
  if (showQueue && (record?.queue_enabled || queued > 0 || queueEnabled > 0)) {
    tags.push(
      <Tag key='queued' color='cyan' type='light' {...tagProps}>
        {t('已排队')}
        {queued > 0 || queueEnabled > 0
          ? ` ${formatNumber(queued)} / ${formatNumber(queueEnabled)}`
          : ''}
      </Tag>,
    );
  }
  if (showQueue && Number.isFinite(queueDepth) && queueDepth > 0) {
    const capacity =
      Number.isFinite(queueCapacity) && queueCapacity > 0
        ? ` / ${formatNumber(queueCapacity)}`
        : '';
    tags.push(
      <Tag key='queue-depth' color='grey' type='light' {...tagProps}>
        {t('队列深度')} {formatNumber(queueDepth)}
        {capacity}
      </Tag>,
    );
  }
  if (showSticky && record?.sticky_source) {
    tags.push(
      <Tag key='sticky-source' color='blue' type='light' {...tagProps}>
        {t('来源')}: {record.sticky_source}
      </Tag>,
    );
  }
  if (showSticky && stickyRoutes > 0) {
    tags.push(
      <Tag key='sticky-routes' color='cyan' type='light' {...tagProps}>
        {t('粘滞路由')} {formatNumber(stickyRoutes)}
      </Tag>,
    );
  }
  if (showSticky && (record?.sticky_retained || stickyRetained > 0)) {
    tags.push(
      <Tag key='sticky-retained' color='green' type='light' {...tagProps}>
        {t('粘滞保留')}{' '}
        {hasStickyRetainedCount && stickyRetained > 0
          ? formatNumber(stickyRetained)
          : ''}
      </Tag>,
    );
  }
  if (showSticky && (record?.sticky_break || stickyBroken > 0)) {
    const reason =
      typeof record?.sticky_break === 'string' && record.sticky_break
        ? `: ${formatStickyBreakReason(record.sticky_break, t)}`
        : stickyBroken > 0
          ? ` ${formatNumber(stickyBroken)}`
          : '';
    tags.push(
      <Tag key='sticky-broken' color='orange' type='light' {...tagProps}>
        {t('粘滞断开')}
        {reason}
      </Tag>,
    );
  }
  if (showSticky && (record?.cache_affinity || cacheAffinity > 0)) {
    tags.push(
      <Tag key='cache-affinity' color='purple' type='light' {...tagProps}>
        {t('缓存亲和')} {cacheAffinity > 0 ? formatNumber(cacheAffinity) : ''}
      </Tag>,
    );
  }

  if (!tags.length)
    return <Typography.Text type='tertiary'>--</Typography.Text>;

  return <div className='ct-model-gateway-queue-tags'>{tags}</div>;
}

function resourceProtectionPhaseMeta(phase, reason, t) {
  const normalized = String(phase || reason || '').trim();
  switch (normalized) {
    case 'primary_hit':
    case 'primary_resource_available':
      return { color: 'green', label: t('命中主资源') };
    case 'primary_saturated_wait':
    case 'primary_resource_saturated':
      return { color: 'cyan', label: t('主资源满载等待') };
    case 'fallback_after_timeout':
    case 'primary_wait_timeout':
    case 'fallback_after_primary_wait_timeout':
      return { color: 'orange', label: t('等待超时转兜底') };
    case 'primary_failure_fallback':
    case 'primary_resource_failure':
      return { color: 'red', label: t('主资源故障转兜底') };
    case 'no_primary_fallback':
    case 'no_primary_resource_candidate':
      return { color: 'orange', label: t('无主资源转兜底') };
    default:
      return normalized ? { color: 'grey', label: normalized } : null;
  }
}

function resourceProtectionRecordMeta(record, t) {
  const enabled =
    record?.resource_protection_enabled ||
    record?.request_meta?.resource_protection_enabled;
  if (!enabled) return null;
  const phase =
    record?.resource_protection_phase ||
    record?.request_meta?.resource_protection_phase;
  const reason =
    record?.resource_protection_reason ||
    record?.request_meta?.resource_protection_reason;
  return resourceProtectionPhaseMeta(phase, reason, t);
}

function ResourceProtectionAggregateCell({ record, t }) {
  const dispatches = Number(record?.resource_protection_dispatches || 0);
  if (dispatches <= 0) {
    return <Typography.Text type='tertiary'>--</Typography.Text>;
  }
  const waits = Number(record?.resource_protection_primary_waits || 0);
  const timeoutFallbacks = Number(
    record?.resource_protection_wait_timeout_fallbacks || 0,
  );
  const failureFallbacks = Number(
    record?.resource_protection_primary_failure_fallbacks || 0,
  );
  const queueDepth = Number(record?.resource_protection_queue_depth || 0);
  const queueCapacity = Number(record?.resource_protection_queue_capacity || 0);
  const costShare = Number(
    record?.resource_protection_fallback_cost_share || 0,
  );
  return (
    <div className='ct-model-gateway-record-tags'>
      <Tag color='cyan' type='light' size='small'>
        {t('主资源等待')} {formatNumber(waits)}
      </Tag>
      <Tag
        color={timeoutFallbacks > 0 ? 'orange' : 'green'}
        type='light'
        size='small'
      >
        {t('超时兜底')} {formatNumber(timeoutFallbacks)}
      </Tag>
      <Tag
        color={failureFallbacks > 0 ? 'red' : 'green'}
        type='light'
        size='small'
      >
        {t('故障兜底')} {formatNumber(failureFallbacks)}
      </Tag>
      <Tag color='blue' type='light' size='small'>
        {t('平均等待')} {formatLatency(record?.resource_protection_avg_wait_ms)}
      </Tag>
      <Tag color='grey' type='light' size='small'>
        {t('主资源队列')} {formatQueuePair(queueDepth, queueCapacity)}
      </Tag>
      <Tag color={costShare > 0 ? 'orange' : 'green'} type='light' size='small'>
        {t('兜底成本占比')} {costShare > 0 ? formatPercent(costShare) : '--'}
      </Tag>
    </div>
  );
}

function formatAttemptFlowAction(action, t) {
  switch (action) {
    case 'complete':
      return t('请求完成');
    case 'switch_channel':
      return t('切换渠道');
    case 'resource_protection_fallback':
      return t('主资源保护兜底');
    case 'retry':
      return t('继续重试');
    case 'stop':
      return t('停止重试');
    default:
      return action || '--';
  }
}

function getAttemptRetryReason(record) {
  return String(
    record?.retry_reason || record?.request_meta?.retry_reason || '',
  )
    .trim()
    .toLowerCase();
}

function isFirstByteTimeoutAttempt(record) {
  return getAttemptRetryReason(record) === RETRY_REASON_FIRST_BYTE_TIMEOUT;
}

function formatAttemptRetryReason(reason, t) {
  switch (
    String(reason || '')
      .trim()
      .toLowerCase()
  ) {
    case RETRY_REASON_FIRST_BYTE_TIMEOUT:
      return t('首字超时');
    case 'primary_wait_timeout':
      return t('等待超时转兜底');
    default:
      return reason || '';
  }
}

function formatAttemptChannelLabel(record, t) {
  const id = Number(
    record?.actual_channel_id ||
      record?.channel_id ||
      record?.final_channel_id ||
      0,
  );
  const name = stripChannelRatioSuffix(
    record?.actual_channel_name ||
      record?.channel_name ||
      record?.final_channel_name ||
      '',
  );
  if (name && id > 0) return `${name} #${id}`;
  if (name) return name;
  if (id > 0) return `#${id}`;
  return t('未知渠道');
}

function formatAttemptErrorCategory(category, t) {
  switch (category) {
    case 'channel_induced_client_abort':
      return t('渠道诱发中断');
    case 'client_aborted':
      return t('客户端中断');
    case 'upstream_concurrency_limit':
      return t('上游并发受限');
    case 'local_concurrency_limit':
      return t('并发压力');
    case 'upstream_rate_limit':
      return t('上游限速');
    case 'stream_interrupted':
      return t('流中断');
    case 'unsupported_capability':
      return t('能力不匹配');
    case 'balance_or_quota':
      return t('余额或额度');
    case 'timeout':
      return t('超时');
    case 'server_error':
      return t('服务端错误');
    case 'upstream_error':
      return t('上游错误');
    case 'error':
      return t('错误');
    default:
      return category || '--';
  }
}

function getWarningFlags(record) {
  const direct = record?.warning_flags;
  const meta = record?.request_meta?.warning_flags;
  const value = Array.isArray(direct) && direct.length ? direct : meta;
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item || '').trim()).filter(Boolean);
}

function hasModelGatewayWarning(record) {
  return (
    Boolean(record?.channel_induced_client_abort) ||
    Boolean(record?.request_meta?.channel_induced_client_abort) ||
    Boolean(String(record?.warning_level || '').trim()) ||
    Boolean(String(record?.request_meta?.warning_level || '').trim()) ||
    Boolean(String(record?.warning_message || '').trim()) ||
    Boolean(String(record?.request_meta?.warning_message || '').trim()) ||
    getWarningFlags(record).length > 0
  );
}

function warningMessage(record) {
  return String(
    record?.warning_message || record?.request_meta?.warning_message || '',
  ).trim();
}

function modelGatewayWarningLabel(record, t) {
  const flags = getWarningFlags(record);
  if (
    record?.channel_induced_client_abort ||
    record?.request_meta?.channel_induced_client_abort ||
    flags.includes('channel_induced_abort')
  ) {
    return t('渠道诱发中断');
  }
  return t('渠道预警');
}

function modelGatewayWarningContent(record, t) {
  const message = warningMessage(record);
  if (message) return message;
  const flags = getWarningFlags(record);
  if (
    record?.channel_induced_client_abort ||
    record?.request_meta?.channel_induced_client_abort ||
    flags.includes('channel_induced_abort')
  ) {
    return t('客户端在未收到有效下游响应前断开，疑似由渠道流式响应异常诱发');
  }
  return t('该请求命中调度预警，请查看尝试记录与渠道状态');
}

function modelGatewayWarningColor(record) {
  const level = String(
    record?.warning_level || record?.request_meta?.warning_level || '',
  )
    .trim()
    .toLowerCase();
  if (level === 'critical') return 'red';
  return 'orange';
}

function ModelGatewayWarningTag({ record, t, size = 'small' }) {
  if (!hasModelGatewayWarning(record)) return null;
  return (
    <Tooltip content={modelGatewayWarningContent(record, t)}>
      <Tag color={modelGatewayWarningColor(record)} size={size} type='light'>
        {modelGatewayWarningLabel(record, t)}
      </Tag>
    </Tooltip>
  );
}

function flowActionTone(action, record) {
  if (record?.concurrency_limited) return 'orange';
  switch (action) {
    case 'complete':
      return 'green';
    case 'switch_channel':
      return 'orange';
    case 'retry':
      return 'blue';
    case 'stop':
      return 'red';
    default:
      return 'grey';
  }
}

function getUsedChannels(record) {
  const direct = record?.used_channels;
  const meta = record?.request_meta?.used_channels;
  const value = Array.isArray(direct) && direct.length ? direct : meta;
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item)).filter(Boolean);
}

function DispatchFlowTags({ record, t, compact = false }) {
  const tagProps = compact ? { size: 'small' } : {};
  const tags = [];
  const action = record?.retry_action || (record?.will_retry ? 'retry' : '');
  const category = record?.error_category;
  const retryReason = getAttemptRetryReason(record);
  const balanceInsufficient = isBalanceInsufficientStatus(record);
  const activeConcurrency = Number(record?.active_concurrency || 0);
  const configuredLimit = Number(record?.configured_concurrency_limit || 0);
  const learnedLimit = Number(record?.learned_concurrency_limit || 0);
  const usedChannels = getUsedChannels(record);
  const resourceProtectionMeta = resourceProtectionRecordMeta(record, t);

  if (action) {
    tags.push(
      <Tag
        key='action'
        color={flowActionTone(action, record)}
        type='light'
        {...tagProps}
      >
        {formatAttemptFlowAction(action, t)}
      </Tag>,
    );
  }
  if (resourceProtectionMeta) {
    tags.push(
      <Tag
        key='resource-protection'
        color={resourceProtectionMeta.color}
        type='light'
        {...tagProps}
      >
        {resourceProtectionMeta.label}
      </Tag>,
    );
  }
  if (category) {
    tags.push(
      <Tag
        key='category'
        color={category === 'channel_induced_client_abort' ? 'orange' : 'grey'}
        type='light'
        {...tagProps}
      >
        {t('失败分类')}: {formatAttemptErrorCategory(category, t)}
      </Tag>,
    );
  }
  if (hasModelGatewayWarning(record)) {
    tags.push(
      <ModelGatewayWarningTag
        key='warning'
        record={record}
        t={t}
        size={compact ? 'small' : 'default'}
      />,
    );
  }
  if (retryReason) {
    tags.push(
      <Tag
        key='retry-reason'
        color={isFirstByteTimeoutAttempt(record) ? 'red' : 'grey'}
        type='light'
        {...tagProps}
      >
        {formatAttemptRetryReason(retryReason, t)}
      </Tag>,
    );
  }
  if (balanceInsufficient) {
    tags.push(
      <Tag key='balance-insufficient' color='red' type='light' {...tagProps}>
        {t('余额不足')}
      </Tag>,
    );
  }
  if (record?.concurrency_limited) {
    tags.push(
      <Tag key='concurrency-limited' color='orange' type='light' {...tagProps}>
        {t('动态并发')}
      </Tag>,
    );
  }
  if (activeConcurrency > 0 || configuredLimit > 0) {
    tags.push(
      <Tag key='concurrency' color='cyan' type='light' {...tagProps}>
        {t('并发')} {formatNumber(activeConcurrency)}
        {configuredLimit > 0 ? ` / ${formatNumber(configuredLimit)}` : ''}
      </Tag>,
    );
  }
  if (learnedLimit > 0) {
    tags.push(
      <Tag
        key='learned-limit'
        color={record?.learned_concurrency_limit_changed ? 'orange' : 'grey'}
        type='light'
        {...tagProps}
      >
        {t('学习上限')} {formatNumber(learnedLimit)}
      </Tag>,
    );
  }
  if (usedChannels.length > 1) {
    tags.push(
      <Tag key='used-channels' color='blue' type='light' {...tagProps}>
        {t('链路')} {usedChannels.join(' -> ')}
      </Tag>,
    );
  }

  if (!tags.length)
    return <Typography.Text type='tertiary'>--</Typography.Text>;

  return <div className='ct-model-gateway-flow-tags'>{tags}</div>;
}

function attemptRecordStatusMeta(record, t) {
  if (record?.success) {
    return {
      color: 'green',
      label: t('成功'),
      detail: t('最终成功渠道'),
    };
  }
  if (isFirstByteTimeoutAttempt(record)) {
    return {
      color: 'red',
      label: t('首字超时'),
      detail: t('内部切换渠道'),
    };
  }
  if (hasModelGatewayWarning(record)) {
    return {
      color: modelGatewayWarningColor(record),
      label: modelGatewayWarningLabel(record, t),
      detail: modelGatewayWarningContent(record, t),
    };
  }
  if (record?.client_aborted || Number(record?.status_code || 0) === 499) {
    return {
      color: 'grey',
      label: t('客户端中断'),
      detail: t('客户端中断'),
    };
  }
  if (record?.will_retry || record?.retry_action === 'switch_channel') {
    return {
      color: 'orange',
      label: t('智能调度'),
      detail: t('将切换到下一候选'),
    };
  }
  return {
    color: 'red',
    label: t('失败'),
    detail: t('尝试失败'),
  };
}

function SmartDispatchAttemptTimeline({ record, t }) {
  const attempts = Array.isArray(record?.attempt_records)
    ? record.attempt_records
    : [];
  if (!record?.recovered && attempts.length <= 1) return null;
  const displayAttempts = attempts.length
    ? attempts
    : [
        {
          attempt_index: Math.max(0, Number(record?.attempts || 1) - 1),
          channel_id: record?.channel_id,
          channel_name: record?.channel_name,
          success: record?.final_success || record?.success,
          duration_ms: record?.duration_ms,
          ttft_ms: record?.ttft_ms,
        },
      ];

  return (
    <DetailPanel title={t('智能调度记录')}>
      <div className='ct-model-gateway-smart-dispatch'>
        <div className='ct-model-gateway-smart-dispatch-summary'>
          <Info size={15} />
          <span>
            {t('本次请求通过智能调度完成，下面按实际尝试顺序展示渠道记录')}
          </span>
        </div>
        <div className='ct-model-gateway-smart-dispatch-list'>
          {displayAttempts.map((attempt, index) => {
            const meta = attemptRecordStatusMeta(attempt, t);
            const category =
              attempt?.error_category || attempt?.request_meta?.error_category;
            const errorReason = category
              ? formatAttemptErrorCategory(category, t)
              : formatChannelStatusReason(attempt?.status_reason, t);
            const retryReason = getAttemptRetryReason(attempt);
            const flowTags = (
              <DispatchFlowTags record={attempt} t={t} compact />
            );
            return (
              <div
                className='ct-model-gateway-smart-dispatch-item'
                key={`${attempt?.id || attempt?.attempt_index || index}-${
                  attempt?.channel_id || attempt?.actual_channel_id || 'channel'
                }`}
              >
                <div className='ct-model-gateway-smart-dispatch-index'>
                  {formatNumber(Number(attempt?.attempt_index ?? index) + 1)}
                </div>
                <div className='ct-model-gateway-smart-dispatch-main'>
                  <div className='ct-model-gateway-smart-dispatch-head'>
                    <Typography.Text strong>
                      {formatAttemptChannelLabel(attempt, t)}
                    </Typography.Text>
                    <Tag color={meta.color} size='small' type='light'>
                      {meta.label}
                    </Tag>
                  </div>
                  <div className='ct-model-gateway-smart-dispatch-meta'>
                    <span>{formatTimestamp(attempt?.created_at)}</span>
                    <span>
                      {t('尝试耗时')} {formatLatency(attempt?.duration_ms)}
                    </span>
                    <span>
                      {t('尝试首包')} {formatLatency(attempt?.ttft_ms)}
                    </span>
                    {attempt?.status_code ? (
                      <span>HTTP {attempt.status_code}</span>
                    ) : null}
                  </div>
                  <div className='ct-model-gateway-smart-dispatch-tags'>
                    <Tag color={meta.color} size='small' type='light'>
                      {meta.detail}
                    </Tag>
                    {attempt?.retry_action ? (
                      <Tag
                        color={flowActionTone(attempt.retry_action, attempt)}
                        size='small'
                        type='light'
                      >
                        {formatAttemptFlowAction(attempt.retry_action, t)}
                      </Tag>
                    ) : null}
                    {errorReason && errorReason !== '--' ? (
                      <Tag color='grey' size='small' type='light'>
                        {errorReason}
                      </Tag>
                    ) : null}
                    {retryReason ? (
                      <Tag
                        color={
                          isFirstByteTimeoutAttempt(attempt) ? 'red' : 'grey'
                        }
                        size='small'
                        type='light'
                      >
                        {formatAttemptRetryReason(retryReason, t)}
                      </Tag>
                    ) : null}
                    {flowTags}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </DetailPanel>
  );
}

function getRuntimeHealthMeta(status, t) {
  const normalized = status || 'healthy';
  switch (normalized) {
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
      return { color: 'grey', label: normalized };
  }
}

function RuntimeMetricTile({ label, value, detail, tone = 'default' }) {
  return (
    <div
      className={`ct-model-gateway-runtime-metric ct-model-gateway-runtime-metric-${tone}`}
    >
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{detail}</small>
    </div>
  );
}

function latestTrendWithRecords(trends) {
  if (!Array.isArray(trends) || trends.length === 0) return null;
  return (
    [...trends].reverse().find((item) => Number(item?.records) > 0) ||
    trends[trends.length - 1]
  );
}

function affectedAggregateCount(items) {
  if (!Array.isArray(items)) return 0;
  return items.filter((item) => {
    const attempts = Number(item?.attempts || 0);
    if (attempts <= 0) return false;
    return (
      Number(item?.success_rate || 0) < 0.98 ||
      Number(item?.failures || 0) > 0 ||
      Number(item?.stream_interrupted || 0) > 0 ||
      isLatencyWarning(
        item?.avg_duration_ms,
        LATENCY_THRESHOLDS.avgDurationMs,
      ) ||
      Number(item?.avg_queue_wait_ms || 0) >= 500
    );
  }).length;
}

function modelGatewayHealthTone(status) {
  switch (status) {
    case 'operational':
      return 'success';
    case 'degraded':
      return 'warning';
    case 'critical':
      return 'danger';
    default:
      return 'default';
  }
}

function getModelGatewayHealth(data, runtimeStatus) {
  const summary = data?.summary || {};
  const runtimeSummary = runtimeStatus?.summary || {};
  const attempts = Number(summary.attempts || 0);
  const dispatches = Number(summary.dispatches || 0);
  const successRate = attempts > 0 ? Number(summary.success_rate || 0) : 1;
  const streamInterrupted = Number(summary.stream_interrupted || 0);
  const avgDurationMs = Number(summary.avg_duration_ms || 0);
  const avgQueueWaitMs = Number(summary.avg_queue_wait_ms || 0);
  const queuedDispatches = Number(summary.queued_dispatches || 0);
  const circuitOpen = Number(runtimeSummary.circuit_open || 0);
  const cooldownChannels = Number(runtimeSummary.cooldown_channels || 0);
  const highPressureChannels = Number(
    runtimeSummary.high_pressure_channels ||
      runtimeSummary.saturated_channels ||
      0,
  );
  const riskRuntimeKeys = Number(summary.current_risk_runtime_keys || 0);
  const streamRatio = attempts > 0 ? streamInterrupted / attempts : 0;
  const queueRatio =
    Number(summary.queue_enabled_dispatches || 0) > 0
      ? queuedDispatches / Number(summary.queue_enabled_dispatches || 0)
      : 0;

  let score = attempts > 0 || dispatches > 0 ? 100 : 0;
  if (attempts > 0) {
    score -= Math.min(42, Math.max(0, 0.995 - successRate) * 520);
    score -= Math.min(18, streamRatio * 1200);
  }
  if (avgDurationMs >= LATENCY_THRESHOLDS.avgDurationMs.danger) score -= 16;
  else if (avgDurationMs >= LATENCY_THRESHOLDS.avgDurationMs.warning)
    score -= 8;
  if (avgQueueWaitMs >= 1500) score -= 12;
  else if (avgQueueWaitMs >= 500) score -= 7;
  else if (avgQueueWaitMs > 0) score -= 3;
  score -= Math.min(18, circuitOpen * 6 + cooldownChannels * 3);
  score -= Math.min(10, highPressureChannels * 3 + riskRuntimeKeys * 2);
  score -= Math.min(8, queueRatio * 20);
  score = Math.max(0, Math.min(100, Math.round(score)));

  let status = 'operational';
  if (score < 60) status = 'critical';
  else if (score < 86 || circuitOpen > 0) {
    status = 'degraded';
  } else if (score < 94 || cooldownChannels > 0 || queuedDispatches > 0) {
    status = 'watching';
  }

  return {
    status,
    score,
    tone: modelGatewayHealthTone(status),
    affectedModels: affectedAggregateCount(data?.by_model),
    affectedGroups: affectedAggregateCount(data?.by_group),
    affectedChannels:
      affectedAggregateCount(data?.by_channel) + circuitOpen + cooldownChannels,
    circuitOpen,
    cooldownChannels,
    highPressureChannels,
  };
}

function getHealthStatusLabel(status, t) {
  switch (status) {
    case 'operational':
      return t('OPERATIONAL');
    case 'watching':
      return t('WATCHING');
    case 'critical':
      return t('CRITICAL');
    default:
      return t('DEGRADED');
  }
}

function getHealthStatusDescription(health, t) {
  if (health.status === 'operational') {
    return t('调度成功率、响应速度和运行态风险均处于稳定区间');
  }
  if (health.status === 'critical') {
    return t('存在高风险异常，请优先处理熔断、排队或大面积失败');
  }
  if (health.status === 'watching') {
    return t('整体可用，但已有队列、冷却或响应波动需要观察');
  }
  return t('存在影响用户体验的异常，建议先处理高影响渠道和模型');
}

function buildSparkValues(trends, key) {
  if (!Array.isArray(trends)) return [];
  return trends
    .filter((item) => Number(item?.records) > 0)
    .slice(-16)
    .map((item) => Number(item?.[key]) || 0);
}

function buildIncidentSparkValues(incident) {
  const metric = Number(String(incident?.metric || '').replace(/[^\d.-]/g, ''));
  const fallback = Number.isFinite(metric) && metric > 0 ? metric : 1;
  return Array.from({ length: 8 }, (_, index) => {
    const wave = Math.sin(index * 1.35 + fallback * 0.07) * fallback * 0.18;
    const lift = index % 3 === 0 ? fallback * 0.22 : 0;
    return Math.max(0, fallback + wave + lift);
  });
}

function percentileFromValues(values, percentile) {
  const sortedValues = (values || [])
    .map((value) => Number(value))
    .filter((value) => Number.isFinite(value) && value > 0)
    .sort((a, b) => a - b);
  if (!sortedValues.length) return 0;
  const index = Math.min(
    sortedValues.length - 1,
    Math.max(0, Math.ceil(percentile * sortedValues.length) - 1),
  );
  return sortedValues[index];
}

function durationP95FromRecords(records) {
  return percentileFromValues(
    (records || [])
      .filter((record) => !isDispatch(record))
      .map((record) => record.duration_ms),
    0.95,
  );
}

function userRequestSummaryFromData(data) {
  return data?.user_requests?.summary || {};
}

function userRequestHealthTone(status) {
  switch (status) {
    case 'operational':
      return 'success';
    case 'critical':
      return 'danger';
    case 'degraded':
      return 'warning';
    default:
      return 'default';
  }
}

function getUserRequestHealth(data) {
  const summary = userRequestSummaryFromData(data);
  const requests = Number(summary.user_requests || 0);
  const completedRequests = Math.max(
    0,
    requests -
      Number(summary.client_aborted || 0) -
      Number(summary.user_quota_exhausted || 0),
  );
  const successRate =
    completedRequests > 0 ? Number(summary.user_success_rate || 0) : 1;
  const finalFailures = Number(summary.final_failures || 0);
  const recovered = Number(summary.recovered || 0);
  const emptyOutputs = Number(summary.empty_outputs || 0);
  const experienceIssues = Number(summary.experience_issues || 0);
  const p95TtftMs = Number(summary.p95_ttft_ms || 0);
  let score = completedRequests > 0 ? 100 : 0;
  if (completedRequests > 0) {
    score -= Math.min(46, Math.max(0, 0.995 - successRate) * 560);
    score -= Math.min(
      16,
      (finalFailures / Math.max(1, completedRequests)) * 160,
    );
    score -= Math.min(
      18,
      ((emptyOutputs + experienceIssues) / Math.max(1, completedRequests)) *
        120,
    );
  }
  if (p95TtftMs >= LATENCY_THRESHOLDS.p95TtftMs.danger) score -= 16;
  else if (p95TtftMs >= LATENCY_THRESHOLDS.p95TtftMs.warning) score -= 8;
  if (recovered > 0)
    score -= Math.min(8, (recovered / Math.max(1, completedRequests)) * 22);
  score = Math.max(0, Math.min(100, Math.round(score)));

  let status = 'operational';
  if (score < 60 || successRate < 0.9) status = 'critical';
  else if (
    score < 86 ||
    successRate < 0.98 ||
    finalFailures > 0 ||
    emptyOutputs > 0 ||
    experienceIssues > 0
  )
    status = 'degraded';
  else if (score < 94 || recovered > 0) status = 'watching';

  return {
    status,
    tone: userRequestHealthTone(status),
    score,
  };
}

function getUserRequestStatusLabel(status, t) {
  switch (status) {
    case 'operational':
      return t('用户感知稳定');
    case 'watching':
      return t('用户感知观察中');
    case 'critical':
      return t('用户感知异常');
    default:
      return t('用户感知降级');
  }
}

function getUserRequestStatusMeta(record, t) {
  if (record?.is_health_probe || record?.request_meta?.is_health_probe) {
    return record?.final_success || record?.success
      ? { color: 'cyan', label: t('健康探活'), tone: 'probe' }
      : { color: 'orange', label: t('探活异常'), tone: 'probe-warning' };
  }
  if (isUserRequestProcessing(record)) {
    return { color: 'blue', label: t('执行中'), tone: 'processing' };
  }
  if (String(record?.status || '').trim() === 'settling') {
    return { color: 'teal', label: t('费用结算中'), tone: 'settling' };
  }
  if (isUserQuotaExhaustedRecord(record)) {
    return { color: 'grey', label: t('用户额度不足'), tone: 'quota' };
  }
  if (hasModelGatewayWarning(record)) {
    return {
      color: modelGatewayWarningColor(record),
      label: modelGatewayWarningLabel(record, t),
      tone: 'warning',
    };
  }
  if (
    record?.client_aborted ||
    record?.status === 'client_aborted' ||
    record?.final_error_category === 'client_aborted' ||
    Number(record?.final_status_code || 0) === 499
  ) {
    return { color: 'grey', label: t('客户端中断'), tone: 'aborted' };
  }
  if (record?.final_success && isSmartSwitchRecovered(record)) {
    return { color: 'teal', label: t('成功'), tone: 'recovered' };
  }
  if (record?.final_success) {
    if (record?.empty_output || record?.experience_issue) {
      return { color: 'orange', label: t('体验异常'), tone: 'warning' };
    }
    return { color: 'green', label: t('成功'), tone: 'success' };
  }
  return { color: 'red', label: t('最终失败'), tone: 'failed' };
}

function isSmartSwitchRecovered(record) {
  return record?.recovered === true;
}

function isUserRequestProcessing(record) {
  if (!record) return false;
  if (Number(record?.completed_at || 0) > 0) return false;
  const status = String(record?.status || '').trim();
  if (
    [
      'success',
      'failed',
      'health_probe',
      'health_probe_failed',
      'client_aborted',
      'user_quota_exhausted',
      'settling',
    ].includes(status)
  ) {
    return false;
  }
  if (
    record?.final_error_category ||
    Number(record?.final_status_code || 0) > 0 ||
    record?.client_aborted ||
    record?.final_success
  ) {
    return false;
  }
  return status === 'processing' || status === '';
}

function formatUserRequestErrorCategory(category, t) {
  switch (category) {
    case 'channel_induced_client_abort':
      return t('渠道诱发中断');
    case 'rate_limit':
      return t('最终失败类型：rate_limit');
    case 'timeout':
      return t('最终失败类型：timeout');
    case 'upstream_error':
      return t('最终失败类型：upstream_error');
    case 'stream_interrupted':
      return t('最终失败类型：stream_interrupted');
    case 'client_aborted':
      return t('用户主动终止');
    case 'balance_or_quota':
      return t('余额或额度');
    case 'user_quota_exhausted':
      return t('用户额度不足');
    case 'server_error':
      return t('最终失败类型：server_error');
    case 'scheduler_exhausted':
      return t('不可调度');
    default:
      return category || '--';
  }
}

function formatUserRequestExperienceIssue(issue, t) {
  switch (issue) {
    case 'empty_output':
      return t('空输出');
    default:
      return issue || t('体验异常');
  }
}

function userRequestLiveDurationMs(record, nowSeconds) {
  if (!isUserRequestProcessing(record)) {
    return Number(record?.duration_ms || 0);
  }
  return Math.max(
    0,
    (Number(nowSeconds || 0) - Number(record?.created_at || nowSeconds)) * 1000,
  );
}

function userRequestDisplayTime(record, nowSeconds, t) {
  const timestamp = isUserRequestProcessing(record)
    ? record?.created_at
    : record?.completed_at || record?.created_at;
  return formatRelativeTime(timestamp, nowSeconds, t);
}

function isLongFormGenerationRequest(record) {
  const model = String(record?.requested_model || '').toLowerCase();
  const path = String(
    record?.request_path || record?.request_meta?.request_path || '',
  ).toLowerCase();
  return (
    model.includes('image') ||
    model.includes('video') ||
    path.includes('/images/') ||
    path.includes('/videos/')
  );
}

function userRequestProcessingStage(record, durationMs, hasTTFT, t) {
  if (!isUserRequestProcessing(record)) return '';
  if (hasTTFT) return t('上游已响应');
  if (isLongFormGenerationRequest(record)) return t('生成任务等待上游');
  if (durationMs >= 30000) return t('首包等待偏长');
  return t('等待首包');
}

function userRequestStatusCaption(record, meta, processing, hasTTFT, durationMs, t) {
  if (processing) {
    return userRequestProcessingStage(record, durationMs, hasTTFT, t);
  }
  if (meta?.tone === 'settling') return t('上游已完成，费用待结算');
  if (isSmartSwitchRecovered(record)) return t('智能切换后成功');
  if (meta?.tone === 'quota') return t('业务拦截');
  if (meta?.tone === 'aborted') return t('客户端断开');
  if (meta?.tone === 'probe' || meta?.tone === 'probe-warning') {
    return t('探活样本');
  }
  if (meta?.tone === 'warning') return t('需复核');
  if (record?.final_success) return t('已结算');
  return t('需排查');
}

function userRequestDurationCaption(processing, t) {
  return processing ? t('实时计时') : t('端到端耗时');
}

function userRequestTTFTCaption(processing, hasTTFT, t) {
  if (processing && !hasTTFT) return t('未收到首包');
  if (processing && hasTTFT) return t('首包已到');
  return t('首包延迟');
}

function userRequestTimeCaption(record, meta, processing, t) {
  if (processing) return t('开始处理');
  if (meta?.tone === 'aborted') return t('断开时间');
  if (meta?.tone === 'quota') return t('拦截时间');
  if (meta?.tone === 'settling') return t('上游完成时间');
  if (meta?.tone === 'probe' || meta?.tone === 'probe-warning') {
    return t('探活时间');
  }
  return record?.final_success ? t('完成时间') : t('失败时间');
}

function userRequestMatchesQuery(record, query) {
  const normalizedQuery = String(query || '')
    .trim()
    .toLowerCase();
  if (!normalizedQuery) return true;
  return [
    record?.request_id,
    record?.username,
    record?.user_id,
    record?.requested_model,
    record?.requested_group,
    record?.selected_group,
    record?.actual_group,
    record?.final_channel_id,
    record?.final_channel_name,
    record?.final_error_category,
  ]
    .map((value) => String(value || '').toLowerCase())
    .some((value) => value.includes(normalizedQuery));
}

function formatUserRequestUser(record) {
  const username = String(record?.username || '').trim();
  const userID = Number(record?.user_id || 0);
  if (username) {
    return username;
  }
  return userID > 0 ? `用户 #${userID}` : '--';
}

function formatUserRequestUserID(record, t) {
  const userID = Number(record?.user_id || 0);
  return userID > 0 ? `${t('ID')} #${userID}` : t('未知用户');
}

function formatUserRequestChannel(record) {
  const channelId = Number(record?.final_channel_id || 0);
  const channelName = String(record?.final_channel_name || '').trim();
  const displayName = stripChannelRatioSuffix(channelName);
  return displayName || (channelId > 0 ? `#${channelId}` : '--');
}

function formatUserRequestChannelId(record) {
  const channelId = Number(record?.final_channel_id || 0);
  return channelId > 0 ? `#${channelId}` : '';
}

function stripChannelRatioSuffix(channelName) {
  const value = String(channelName || '').trim();
  if (!value) return '';
  const stripped = value
    .replace(/\s*(?:[-_/|:]\s*)?\(?\s*x\s*[0-9]+(?:\.[0-9]+)?\s*\)?$/i, '')
    .replace(/\s*(?:[-_/|:]\s*)?\(?\s*[0-9]+(?:\.[0-9]+)?\s*x\s*\)?$/i, '')
    .trim();
  return stripped || value;
}

function formatUserRequestGroupFlow(record) {
  const requestedGroup = String(record?.requested_group || '').trim();
  const selectedGroup = String(record?.selected_group || '').trim();
  const actualGroup = String(record?.actual_group || '').trim();
  return actualGroup || selectedGroup || requestedGroup || '--';
}

function safeNumber(value, fallback = 0) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : fallback;
}

function formatBillingRatio(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return numeric.toFixed(4).replace(/0+$/, '').replace(/\.$/, '');
}

function formatFixedBillingRatio(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return numeric.toFixed(4);
}

function formatRatioValue(value, digits = 4) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return `${numeric.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}x`;
}

function formatUserRequestGroupRatio(record) {
  const actualGroup = String(record?.actual_group || '').trim();
  const billingGroup = String(record?.billing?.group || '').trim();
  const ratio = Number(
    record?.actual_group_ratio ||
      (actualGroup && billingGroup === actualGroup
        ? record?.billing?.group_ratio
        : 0) ||
      0,
  );
  if (!Number.isFinite(ratio) || ratio <= 0) return '';
  return `${formatBillingRatio(ratio)}x`;
}

function formatUsdCostAmount(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  if (numeric < 0.00000001) {
    return `$${numeric.toExponential(2).replace('e', 'E')}`;
  }
  const digits = numeric < 0.000001 ? 12 : numeric < 0.01 ? 8 : 6;
  return `$${numeric.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatCostUnitPrice(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  const digits = numeric < 0.000001 ? 12 : numeric < 1 ? 6 : 4;
  return `$${numeric.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatCandidateReferenceCost(candidate, t) {
  const cost = Number(candidate?.cost_ratio || 0);
  if (!Number.isFinite(cost) || cost <= 0) return '';
  const mode = String(candidate?.cost_pricing_mode || '').trim();
  const suffix = mode === 'request' ? t('/次') : t('/M');
  return `${formatCostUnitPrice(cost)} ${suffix}`;
}

function formatCostScoreItemSummary(item, t) {
  if (!item || item.missing_reason) return '';
  const current = Number(item.raw_number);
  const reference = Number(item.reference_number);
  const unit = String(item.raw_unit || item.reference_unit || '').trim();
  const suffix = unit === 'request' ? t('/次') : t('/M');
  const currentLabel =
    Number.isFinite(current) && current > 0
      ? `${formatCostUnitPrice(current)} ${suffix}`
      : '--';
  const referenceLabel =
    Number.isFinite(reference) && reference > 0
      ? `${formatCostUnitPrice(reference)} ${suffix}`
      : '--';
  return `${t('当前')} ${currentLabel} · ${t('参考')} ${referenceLabel}`;
}

function formatCostRatio(value) {
  const text = formatBillingRatio(value);
  return text === '--' ? '--' : `${text}x`;
}

function formatDynamicCostRatio(value) {
  const text = formatFixedBillingRatio(value);
  return text === '--' ? '--' : `${text}x`;
}

function formatBillingTokenCount(value) {
  const numeric = Number(value || 0);
  return numeric > 0 ? formatNumber(numeric) : '--';
}

function formatBillingUnitPrice(ratio) {
  const numeric = Number(ratio);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return renderQuota(numeric * 1000, 6);
}

function billingQuotaPerUnit() {
  const quotaPerUnit = Number(getQuotaPerUnit());
  return Number.isFinite(quotaPerUnit) && quotaPerUnit > 0
    ? quotaPerUnit
    : 500000;
}

function billingReferenceQuota(billing) {
  if (!billing) return 0;
  return safeNumber(billing.quota);
}

function dynamicBillingSummaryLabel(billing, t) {
  if (!billing) return '';
  if (billing.dynamic_billing_applied) {
    return [
      formatDynamicCostRatio(
        billing.dynamic_billing_ratio || billing.group_ratio,
      ),
      billing.dynamic_billing_price_per_m > 0
        ? `${formatUsdCostAmount(billing.dynamic_billing_price_per_m)}/M`
        : '',
      dynamicBillingApplyReasonLabel(billing.dynamic_billing_apply_reason, t),
    ]
      .filter(Boolean)
      .join(' · ');
  }
  if (billing.dynamic_billing_fallback) {
    return `${t('回退静态')} ${billing.dynamic_fallback_reason || ''}`.trim();
  }
  return '';
}

function dynamicBillingApplyReasonLabel(reason, t) {
  switch (String(reason || '').trim()) {
    case 'step_change_auto_applied':
      return `${t('自动应用')} · ${t('变化过大')}`;
    case 'manual_mode_auto_applied':
    case 'auto_applied':
      return t('自动应用');
    default:
      return '';
  }
}

function dynamicBillingOverviewFromData(data) {
  const overview = data?.dynamic_billing_overview;
  if (!overview || !Array.isArray(overview.groups)) {
    return { enabled: false, groups: [] };
  }
  return {
    ...overview,
    groups: overview.groups.filter((item) => dynamicBillingDisplayGroup(item)),
  };
}

function dynamicBilling7dOverviewFromData(data) {
  const overview = data?.dynamic_billing_7d_overview;
  if (!overview || !Array.isArray(overview.groups)) {
    return { enabled: false, groups: [] };
  }
  return {
    ...overview,
    groups: overview.groups.filter((item) => dynamicBillingDisplayGroup(item)),
  };
}

function dynamicBillingDisplayGroup(item) {
  if (!item) return '';
  const candidates = [
    item.display_group,
    item.current_target_group,
    ...(Array.isArray(item.target_groups) ? item.target_groups : []),
    item.policy_group,
  ];
  for (const candidate of candidates) {
    const group = String(candidate || '').trim();
    if (!group || group.toLowerCase() === 'auto') continue;
    return group;
  }
  return '';
}

function formatDynamicBillingOverviewStatus(status, t) {
  switch (status) {
    case 'active':
      return t('动态可用');
    case 'expired':
      return t('结果过期');
    case 'global_disabled':
      return t('全局未启用');
    case 'manual_confirm':
    case 'manual_confirm_required':
      return t('自动应用');
    case 'observe_mode':
      return t('仅观测');
    case 'step_change_too_large':
      return t('自动应用');
    case 'insufficient_usage':
      return t('样本不足');
    case 'no_cost_data':
      return t('成本缺失');
    case 'base_quota_missing':
      return t('基础计费缺失');
    case 'traffic_not_ready':
      return t('流量成本未就绪');
    case 'waiting_samples':
    default:
      return t('等待样本');
  }
}

function dynamicBillingOverviewStatusRank(status) {
  switch (status) {
    case 'active':
    case 'manual_confirm':
    case 'manual_confirm_required':
    case 'step_change_too_large':
      return 0;
    case 'observe_mode':
      return 1;
    case 'waiting_samples':
    case 'insufficient_usage':
    case 'no_cost_data':
    case 'base_quota_missing':
    case 'traffic_not_ready':
      return 2;
    case 'expired':
      return 3;
    case 'global_disabled':
    default:
      return 4;
  }
}

function dynamicBillingOverviewStatusClassName(status) {
  switch (status) {
    case 'active':
    case 'manual_confirm':
    case 'manual_confirm_required':
    case 'step_change_too_large':
      return 'is-active';
    case 'observe_mode':
      return 'is-waiting';
    case 'expired':
      return 'is-expired';
    case 'global_disabled':
      return 'is-disabled';
    case 'waiting_samples':
    default:
      return 'is-waiting';
  }
}

function formatDynamicBillingRatioRange(item) {
  if (!item) return '--';
  const minRatio = safeNumber(item.min_ratio);
  const maxRatio = safeNumber(item.max_ratio);
  if (minRatio <= 0 && maxRatio <= 0) return '--';
  if (minRatio > 0 && maxRatio > 0 && Math.abs(minRatio - maxRatio) >= 0.0001) {
    return `${formatDynamicCostRatio(minRatio)}-${formatDynamicCostRatio(maxRatio)}`;
  }
  return formatDynamicCostRatio(maxRatio || minRatio);
}

function formatDynamicBillingRatioCurrent(item) {
  if (!item) return '--';
  const currentRatio = safeNumber(item.current_ratio);
  if (currentRatio > 0) return formatDynamicCostRatio(currentRatio);
  const blendedRatio = safeNumber(item.blended_ratio);
  if (blendedRatio > 0) return formatDynamicCostRatio(blendedRatio);
  return formatDynamicBillingRatioRange(item);
}

function formatDynamicBillingRatioAverage(item) {
  if (!item) return '--';
  const blendedRatio = safeNumber(item.blended_ratio);
  if (blendedRatio > 0) return formatDynamicCostRatio(blendedRatio);
  const averageRatio = safeNumber(item.average_ratio);
  if (averageRatio > 0) return formatDynamicCostRatio(averageRatio);
  return formatDynamicBillingRatioCurrent(item);
}

function formatDynamicBillingPriceRange(item) {
  if (!item) return '--';
  const minPrice = safeNumber(item.min_price_per_m);
  const maxPrice = safeNumber(item.max_price_per_m);
  if (minPrice <= 0 && maxPrice <= 0) return '--';
  if (
    minPrice > 0 &&
    maxPrice > 0 &&
    Math.abs(minPrice - maxPrice) >= 0.000001
  ) {
    return `${formatUsdCostAmount(minPrice)}-${formatUsdCostAmount(maxPrice)}/M`;
  }
  return `${formatUsdCostAmount(maxPrice || minPrice)}/M`;
}

function formatDynamicBillingPriceCompact(item) {
  if (!item) return '--';
  const currentPrice = safeNumber(item.current_price_per_m);
  if (currentPrice > 0) {
    const digits = currentPrice < 0.01 ? 4 : currentPrice < 1 ? 3 : 2;
    return `$${currentPrice.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
  }
  const blendedPrice = safeNumber(item.blended_price_per_m);
  if (blendedPrice > 0) {
    const digits = blendedPrice < 0.01 ? 4 : blendedPrice < 1 ? 3 : 2;
    return `$${blendedPrice.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
  }
  const minPrice = safeNumber(item.min_price_per_m);
  const maxPrice = safeNumber(item.max_price_per_m);
  if (minPrice <= 0 && maxPrice <= 0) return '--';
  const value = maxPrice || minPrice;
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  const digits = numeric < 0.01 ? 4 : numeric < 1 ? 3 : 2;
  return `$${numeric.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
}

function formatDynamicBillingPriceAverage(item) {
  if (!item) return '--';
  const blendedPrice = safeNumber(item.blended_price_per_m);
  if (blendedPrice > 0) {
    const digits = blendedPrice < 0.01 ? 4 : blendedPrice < 1 ? 3 : 2;
    return `$${blendedPrice.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
  }
  const averagePrice = safeNumber(item.average_price_per_m);
  if (averagePrice > 0) {
    const digits = averagePrice < 0.01 ? 4 : averagePrice < 1 ? 3 : 2;
    return `$${averagePrice.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
  }
  return formatDynamicBillingPriceCompact(item);
}

function dynamicBillingCostFactor(overview, item) {
  const upstreamCost = safeNumber(item?.upstream_cost_usd);
  const requiredRevenue = safeNumber(item?.required_revenue_usd);
  if (upstreamCost > 0 && requiredRevenue > 0) {
    return requiredRevenue / upstreamCost;
  }
  const profitRate = Math.min(
    Math.max(safeNumber(overview?.profit_rate), 0),
    0.95,
  );
  return 1 / (1 - profitRate);
}

function formatDynamicBillingCostPriceCompact(item, overview) {
  if (!item) return '--';
  const factor = dynamicBillingCostFactor(overview, item);
  const currentPrice = safeNumber(item.current_price_per_m);
  if (currentPrice > 0) {
    const costPrice = currentPrice / factor;
    const digits = costPrice < 0.01 ? 4 : costPrice < 1 ? 3 : 2;
    return `$${costPrice.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
  }
  const blendedPrice = safeNumber(item.blended_price_per_m);
  const fallbackPrice =
    blendedPrice > 0
      ? blendedPrice
      : safeNumber(item.max_price_per_m || item.min_price_per_m);
  if (fallbackPrice <= 0) return '--';
  const costPrice = fallbackPrice / factor;
  const digits = costPrice < 0.01 ? 4 : costPrice < 1 ? 3 : 2;
  return `$${costPrice.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
}

function formatDynamicBillingCostRatioCurrent(item, overview) {
  if (!item) return '--';
  const costMultiplier = safeNumber(item.cost_multiplier);
  if (costMultiplier > 0) {
    return formatDynamicCostRatio(costMultiplier);
  }
  const factor = dynamicBillingCostFactor(overview, item);
  const currentRatio = safeNumber(item.current_ratio);
  if (currentRatio > 0) {
    return formatDynamicCostRatio(currentRatio / factor);
  }
  const blendedRatio = safeNumber(item.blended_ratio);
  if (blendedRatio > 0) {
    return formatDynamicCostRatio(blendedRatio / factor);
  }
  const fallbackRatio = safeNumber(item.max_ratio || item.min_ratio);
  if (fallbackRatio <= 0) return '--';
  return formatDynamicCostRatio(fallbackRatio / factor);
}

function formatDynamicBillingCostPriceAverage(item, overview) {
  if (!item) return '--';
  const factor = dynamicBillingCostFactor(overview, item);
  const blendedPrice = safeNumber(item.blended_price_per_m);
  if (blendedPrice > 0) {
    const costPrice = blendedPrice / factor;
    const digits = costPrice < 0.01 ? 4 : costPrice < 1 ? 3 : 2;
    return `$${costPrice.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
  }
  const averagePrice = safeNumber(item.average_price_per_m);
  const fallbackPrice =
    averagePrice > 0 ? averagePrice : safeNumber(item.current_price_per_m);
  if (fallbackPrice <= 0)
    return formatDynamicBillingCostPriceCompact(item, overview);
  const costPrice = fallbackPrice / factor;
  const digits = costPrice < 0.01 ? 4 : costPrice < 1 ? 3 : 2;
  return `$${costPrice.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}/M`;
}

function formatDynamicBillingCostRatioAverage(item, overview) {
  if (!item) return '--';
  const costMultiplier = safeNumber(item.cost_multiplier);
  if (costMultiplier > 0) {
    return formatDynamicCostRatio(costMultiplier);
  }
  const factor = dynamicBillingCostFactor(overview, item);
  const blendedRatio = safeNumber(item.blended_ratio);
  if (blendedRatio > 0) {
    return formatDynamicCostRatio(blendedRatio / factor);
  }
  const averageRatio = safeNumber(item.average_ratio);
  if (averageRatio > 0) {
    return formatDynamicCostRatio(averageRatio / factor);
  }
  return formatDynamicBillingCostRatioCurrent(item, overview);
}

function dynamicBillingCostSourceLabel(source, t) {
  switch (String(source || '').trim()) {
    case 'profit_24h':
      return t('24h 盈利经营成本');
    case 'sample_cost':
      return t('近期上游成本样本');
    default:
      return '';
  }
}

function billingPromptTokens(billing) {
  const prompt = safeNumber(billing?.prompt_tokens);
  const cacheRead = safeNumber(billing?.cache_tokens);
  const cacheWrite = safeNumber(
    billing?.cache_write_tokens || billing?.cache_creation_tokens,
  );
  const imageTokens = safeNumber(billing?.image_tokens);
  const audioTokens = safeNumber(billing?.audio_input_token_count);
  return Math.max(
    0,
    prompt - cacheRead - cacheWrite - imageTokens - audioTokens,
  );
}

function billingInputQuota(billing) {
  if (!billing) return 0;
  const modelRatio = safeNumber(billing.model_ratio);
  const groupRatio = safeNumber(billing.group_ratio, 1);
  return billingPromptTokens(billing) * modelRatio * groupRatio;
}

function billingOutputQuota(billing) {
  if (!billing) return 0;
  const modelRatio = safeNumber(billing.model_ratio);
  const groupRatio = safeNumber(billing.group_ratio, 1);
  const completionRatio = safeNumber(billing.completion_ratio, 1);
  return (
    safeNumber(billing.completion_tokens) *
    modelRatio *
    completionRatio *
    groupRatio
  );
}

function billingCacheReadQuota(billing) {
  if (!billing) return 0;
  const modelRatio = safeNumber(billing.model_ratio);
  const groupRatio = safeNumber(billing.group_ratio, 1);
  const cacheRatio = safeNumber(billing.cache_ratio);
  return (
    safeNumber(billing.cache_tokens) * modelRatio * cacheRatio * groupRatio
  );
}

function billingCacheWriteQuota(billing) {
  if (!billing) return 0;
  const modelRatio = safeNumber(billing.model_ratio);
  const groupRatio = safeNumber(billing.group_ratio, 1);
  const cacheTokens = safeNumber(
    billing.cache_write_tokens || billing.cache_creation_tokens,
  );
  if (
    safeNumber(billing.cache_creation_tokens_5m) > 0 ||
    safeNumber(billing.cache_creation_tokens_1h) > 0
  ) {
    return (
      safeNumber(billing.cache_creation_tokens_5m) *
        modelRatio *
        safeNumber(billing.cache_creation_ratio_5m) *
        groupRatio +
      safeNumber(billing.cache_creation_tokens_1h) *
        modelRatio *
        safeNumber(billing.cache_creation_ratio_1h) *
        groupRatio
    );
  }
  return (
    cacheTokens *
    modelRatio *
    safeNumber(billing.cache_creation_ratio) *
    groupRatio
  );
}

function billingToolQuota(billing) {
  if (!billing) return 0;
  const groupRatio = safeNumber(billing.group_ratio, 1);
  const quotaPerUnit = billingQuotaPerUnit();
  const webSearchQuota =
    safeNumber(billing.web_search_call_count) *
    safeNumber(billing.web_search_price) *
    (quotaPerUnit / 1000) *
    groupRatio;
  const fileSearchQuota =
    safeNumber(billing.file_search_call_count) *
    safeNumber(billing.file_search_price) *
    (quotaPerUnit / 1000) *
    groupRatio;
  const imageGenerationQuota =
    safeNumber(billing.image_generation_call_count) *
    safeNumber(billing.image_generation_call_price) *
    quotaPerUnit *
    groupRatio;
  return webSearchQuota + fileSearchQuota + imageGenerationQuota;
}

function billingSourceLabel(source, t) {
  switch (source) {
    case 'subscription':
      return t('订阅抵扣');
    case 'subscription_wallet':
      return t('订阅抵扣 + 钱包补扣');
    case 'wallet':
      return t('钱包扣费');
    default:
      return source || t('标准计费');
  }
}

function billingModeLabel(mode, t) {
  switch (mode) {
    case 'model_gateway_dynamic':
      return t('动态收费');
    case 'tiered_expr':
      return t('阶梯计费');
    default:
      return mode || t('按量计费');
  }
}

function HoverCard({ content, children, className = '' }) {
  const anchorRef = useRef(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ left: 0, top: 0 });

  const updatePosition = useCallback(() => {
    const anchor = anchorRef.current;
    if (!anchor || typeof window === 'undefined') return;
    const rect = anchor.getBoundingClientRect();
    setPosition({
      left: Math.min(
        window.innerWidth - 18,
        Math.max(18, rect.left + rect.width / 2),
      ),
      top: Math.max(18, rect.top - 12),
    });
  }, []);

  const show = useCallback(() => {
    updatePosition();
    setOpen(true);
  }, [updatePosition]);
  const hide = useCallback(() => setOpen(false), []);

  useEffect(() => {
    if (!open) return undefined;
    const handle = () => updatePosition();
    window.addEventListener('scroll', handle, true);
    window.addEventListener('resize', handle);
    return () => {
      window.removeEventListener('scroll', handle, true);
      window.removeEventListener('resize', handle);
    };
  }, [open, updatePosition]);

  return (
    <>
      <div
        ref={anchorRef}
        className={className}
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
        tabIndex={0}
      >
        {children}
      </div>
      {open &&
        createPortal(
          <div
            className='ct-model-gateway-hover-card'
            style={{
              left: position.left,
              top: position.top,
            }}
          >
            {content}
          </div>,
          document.body,
        )}
    </>
  );
}

function UserRequestEventTooltip({ record, meta, processing, durationMs, t }) {
  const attempts = Math.max(1, Number(record?.attempts || 0));
  const hasTTFT = Number(record?.ttft_ms || 0) > 0;
  const firstByteTimeoutAttempts = Array.isArray(record?.attempt_records)
    ? record.attempt_records.filter((attempt) =>
        isFirstByteTimeoutAttempt(attempt),
      ).length
    : 0;
  const rows = [
    [t('状态'), meta?.label || '--'],
    [
      t('事件'),
      processing
        ? t('请求仍在处理中')
        : hasModelGatewayWarning(record)
          ? modelGatewayWarningLabel(record, t)
          : record?.client_aborted
            ? t('用户主动终止')
            : record?.recovered
              ? t('智能调度后成功')
              : record?.final_success
                ? t('请求完成')
                : t('请求失败'),
    ],
    [t('创建时间'), formatTimestamp(record?.created_at)],
    [
      t('完成时间'),
      processing ? t('等待完成') : formatTimestamp(record?.completed_at),
    ],
    [t('尝试次数'), processing ? '--' : formatNumber(attempts)],
    [t('总耗时'), formatLatency(durationMs)],
    [
      t('首包延迟'),
      processing && !hasTTFT ? '--' : formatLatency(record?.ttft_ms),
    ],
  ];

  if (firstByteTimeoutAttempts > 0 && !processing) {
    rows.push([
      t('渠道切换'),
      t('首字超时内部切换 {{count}} 次', {
        count: formatNumber(firstByteTimeoutAttempts),
      }),
    ]);
  }
  if (isSmartSwitchRecovered(record) && !processing) {
    rows.push([
      t('智能切换'),
      t('请求通过智能调度切换到可用渠道后完成'),
    ]);
  }
  if (hasModelGatewayWarning(record) && !processing) {
    rows.push([t('渠道预警'), modelGatewayWarningContent(record, t)]);
  }

  if (
    !record?.final_success &&
    record?.final_error_category &&
    meta?.tone !== 'aborted' &&
    !processing
  ) {
    rows.push([
      t('失败原因'),
      formatUserRequestErrorCategory(record.final_error_category, t),
    ]);
  }
  if (
    record?.final_success &&
    (record?.empty_output || record?.experience_issue) &&
    !processing
  ) {
    rows.push([
      t('体验问题'),
      formatUserRequestExperienceIssue(
        record.experience_issue || (record.empty_output ? 'empty_output' : ''),
        t,
      ),
    ]);
  }

  return (
    <div className='ct-model-gateway-event-tooltip'>
      <div className='ct-model-gateway-event-tooltip-title'>
        {t('事件详情')}
      </div>
      {rows.map(([label, value]) => (
        <div className='ct-model-gateway-event-tooltip-row' key={label}>
          <span>{label}</span>
          <strong>{value || '--'}</strong>
        </div>
      ))}
    </div>
  );
}

function UserRequestCostTooltip({ billing, t }) {
  if (!billing) {
    return (
      <div className='ct-model-gateway-cost-tooltip'>
        <div className='ct-model-gateway-cost-tooltip-title'>
          {t('费用计算参考')}
        </div>
        <p>{t('暂无消费日志，无法展示费用参考')}</p>
      </div>
    );
  }

  const inputQuota = billingInputQuota(billing);
  const outputQuota = billingOutputQuota(billing);
  const cacheReadQuota = billingCacheReadQuota(billing);
  const cacheWriteQuota = billingCacheWriteQuota(billing);
  const toolQuota = billingToolQuota(billing);
  const rows = [
    {
      label: t('输入成本'),
      value: inputQuota > 0 ? renderQuota(inputQuota, 6) : t('见实际扣费'),
      meta: `${formatBillingTokenCount(billingPromptTokens(billing))} ${t(
        'tokens',
      )}`,
    },
    {
      label: t('输出成本'),
      value: outputQuota > 0 ? renderQuota(outputQuota, 6) : t('见实际扣费'),
      meta: `${formatBillingTokenCount(billing.completion_tokens)} ${t(
        'tokens',
      )}`,
    },
    {
      label: t('输入单价'),
      value: formatBillingUnitPrice(billing.model_ratio),
      meta: t('每 1K tokens'),
    },
    {
      label: t('输出单价'),
      value: formatBillingUnitPrice(
        safeNumber(billing.model_ratio) *
          safeNumber(billing.completion_ratio, 1),
      ),
      meta: t('每 1K tokens'),
    },
  ];
  if (safeNumber(billing.cache_tokens) > 0) {
    rows.push({
      label: t('缓存读取成本'),
      value: renderQuota(cacheReadQuota, 6),
      meta: `${formatBillingTokenCount(billing.cache_tokens)} ${t('tokens')}`,
    });
  }
  if (
    safeNumber(billing.cache_write_tokens || billing.cache_creation_tokens) > 0
  ) {
    rows.push({
      label: t('缓存写入成本'),
      value: renderQuota(cacheWriteQuota, 6),
      meta: `${formatBillingTokenCount(
        billing.cache_write_tokens || billing.cache_creation_tokens,
      )} ${t('tokens')}`,
    });
  }
  if (toolQuota > 0) {
    rows.push({
      label: t('工具调用成本'),
      value: renderQuota(toolQuota, 6),
      meta: `${formatNumber(
        safeNumber(billing.web_search_call_count) +
          safeNumber(billing.file_search_call_count) +
          safeNumber(billing.image_generation_call_count),
      )} ${t('次')}`,
    });
  }

  const ratioText = [
    `${t('模型')} ${formatBillingRatio(billing.model_ratio)}`,
    `${t('输出')} ${formatBillingRatio(billing.completion_ratio || 1)}`,
    `${t('分组')} ${formatBillingRatio(billing.group_ratio || 1)}`,
  ].join(' / ');
  const settlementRows = [
    [t('服务档位'), billingModeLabel(billing.billing_mode, t)],
    [t('倍率'), ratioText],
    [
      t('原始'),
      `${formatBillingTokenCount(billing.total_tokens)} ${t('tokens')}`,
    ],
    [t('计费'), billingSourceLabel(billing.billing_source, t)],
  ];
  if (billing.dynamic_billing_applied) {
    settlementRows.splice(2, 0, [
      t('动态收费'),
      [
        `${formatDynamicCostRatio(billing.dynamic_billing_ratio || billing.group_ratio)}`,
        billing.dynamic_billing_price_per_m > 0
          ? `${formatUsdCostAmount(billing.dynamic_billing_price_per_m)}/M`
          : '',
        dynamicBillingApplyReasonLabel(billing.dynamic_billing_apply_reason, t),
      ]
        .filter(Boolean)
        .join(' · '),
    ]);
  } else if (billing.dynamic_billing_fallback) {
    settlementRows.splice(2, 0, [
      t('动态收费'),
      `${t('回退静态')} ${billing.dynamic_fallback_reason || ''}`.trim(),
    ]);
  }
  if (safeNumber(billing.subscription_consumed) > 0) {
    settlementRows.push([
      t('订阅抵扣'),
      renderQuota(billing.subscription_consumed, 6),
    ]);
  }
  if (safeNumber(billing.wallet_quota_deducted) > 0) {
    settlementRows.push([
      t('钱包补扣'),
      renderQuota(billing.wallet_quota_deducted, 6),
    ]);
  }

  return (
    <div className='ct-model-gateway-cost-tooltip'>
      <div className='ct-model-gateway-cost-tooltip-title'>
        {t('费用计算参考')}
      </div>
      {rows.map((row) => (
        <div className='ct-model-gateway-cost-tooltip-row' key={row.label}>
          <span>
            {row.label}
            {row.meta && <em>{row.meta}</em>}
          </span>
          <strong title={t('费用计算参考')}>{row.value}</strong>
        </div>
      ))}
      <div className='ct-model-gateway-cost-tooltip-divider' />
      {settlementRows.map(([label, value]) => (
        <div className='ct-model-gateway-cost-tooltip-row' key={label}>
          <span>{label}</span>
          <strong title={label}>{value || '--'}</strong>
        </div>
      ))}
      <div className='ct-model-gateway-cost-tooltip-divider' />
      <div className='ct-model-gateway-cost-tooltip-total'>
        <span>{t('实际扣费')}</span>
        <strong>{renderQuota(billingReferenceQuota(billing), 6)}</strong>
      </div>
    </div>
  );
}

function upstreamCostSourceLabel(source, accuracy, t) {
  if (source === 'not_applicable' || accuracy === 'not_applicable')
    return t('未发生');
  if (source === 'pending' || accuracy === 'pending') return t('待核算');
  if (source === 'missing' || accuracy === 'missing') return t('未配置');
  if (source === 'auto_synced') return t('自动同步');
  if (source === 'manual') return t('手动规则');
  return source || '--';
}

function upstreamCostStatus(record) {
  if (isUserQuotaExhaustedRecord(record)) {
    return { source: 'not_applicable', accuracy: 'not_applicable', amount: 0 };
  }
  const source = String(record?.upstream_cost_source || '').trim();
  const accuracy = String(record?.upstream_cost_accuracy || '').trim();
  const total = safeNumber(record?.upstream_cost_total);
  if (
    source === 'pending' ||
    accuracy === 'pending' ||
    (!source && total <= 0)
  ) {
    return { source: 'pending', accuracy: 'pending', amount: 0 };
  }
  if (source === 'missing' || accuracy === 'missing') {
    return { source: 'missing', accuracy: 'missing', amount: 0 };
  }
  return {
    source,
    accuracy,
    amount: Number.isFinite(total) && total > 0 ? total : 0,
  };
}

function upstreamCostComponentRows(breakdown, t) {
  const components = [
    ['input', t('输入')],
    ['output', t('输出')],
    ['cache_read', t('缓存读取')],
    ['cache_write', t('缓存写入')],
    ['cache_write_5m', t('缓存写入 5m')],
    ['cache_write_1h', t('缓存写入 1h')],
    ['image_input', t('图片输入')],
    ['image_output', t('图片输出')],
    ['audio_input', t('音频输入')],
    ['audio_output', t('音频输出')],
    ['request', t('按次')],
  ];
  const rows = [];
  components.forEach(([key, label]) => {
    const item = breakdown?.[key];
    const amount = safeNumber(item?.amount);
    const tokens = safeNumber(item?.tokens);
    const count = safeNumber(item?.count);
    const price = safeNumber(item?.price_per_million || item?.unit_price);
    if (amount <= 0 && tokens <= 0 && count <= 0 && price <= 0) return;
    const metaParts = [];
    if (tokens > 0) {
      metaParts.push(`${formatBillingTokenCount(tokens)} ${t('tokens')}`);
    } else if (count > 0) {
      metaParts.push(`${formatNumber(count)} ${t('次')}`);
    }
    if (item?.price_per_million) {
      metaParts.push(
        `${t('推导成本/M')} ${formatCostUnitPrice(item.price_per_million)}`,
      );
    } else if (item?.unit_price) {
      metaParts.push(
        `${t('按次成本')} ${formatCostUnitPrice(item.unit_price)}`,
      );
    }
    rows.push({ label, amount, meta: metaParts.join(' · ') });
  });
  Object.entries(breakdown?.tools || {}).forEach(([name, item]) => {
    const amount = safeNumber(item?.amount);
    const count = safeNumber(item?.count);
    const price = safeNumber(item?.unit_price);
    if (amount <= 0 && count <= 0 && price <= 0) return;
    const metaParts = [];
    if (count > 0) metaParts.push(`${formatNumber(count)} ${t('次')}`);
    if (item?.unit_price) {
      metaParts.push(
        `${t('按次成本')} ${formatCostUnitPrice(item.unit_price)}`,
      );
    }
    rows.push({
      label: name,
      amount,
      meta: metaParts.join(' · '),
    });
  });
  return rows;
}

function upstreamCostSummaryComponentRows(breakdown, t) {
  return upstreamCostComponentRows(breakdown, t).filter(
    (row) => row.amount > 0,
  );
}

function UserRequestUpstreamCostTooltip({ record, t }) {
  const status = upstreamCostStatus(record);
  const breakdown = record?.upstream_cost_breakdown || {};
  const rows = upstreamCostComponentRows(breakdown, t);
  const pricingRows = upstreamCostPricingRows(breakdown, t);
  return (
    <div className='ct-model-gateway-cost-tooltip'>
      <div className='ct-model-gateway-cost-tooltip-title'>
        {t('上游成本明细')}
      </div>
      <div className='ct-model-gateway-cost-tooltip-row'>
        <span>{t('状态')}</span>
        <strong>
          {upstreamCostSourceLabel(status.source, status.accuracy, t)}
        </strong>
      </div>
      <div className='ct-model-gateway-cost-tooltip-row'>
        <span>{t('供应商成本')}</span>
        <strong>
          {status.amount > 0 ? formatUsdCostAmount(status.amount) : '--'}
        </strong>
      </div>
      {pricingRows.length > 0 && (
        <>
          <div className='ct-model-gateway-cost-tooltip-divider' />
          <div className='ct-model-gateway-cost-tooltip-section'>
            {t('渠道成本配置')}
          </div>
          {pricingRows.map(([label, value]) => (
            <div className='ct-model-gateway-cost-tooltip-row' key={label}>
              <span>{label}</span>
              <strong>{value}</strong>
            </div>
          ))}
        </>
      )}
      {rows.length > 0 && (
        <div className='ct-model-gateway-cost-tooltip-divider' />
      )}
      {rows.map((row) => (
        <div className='ct-model-gateway-cost-tooltip-row' key={row.label}>
          <span>
            {row.label}
            {row.meta && <em>{row.meta}</em>}
          </span>
          <strong>
            {row.amount > 0 ? formatUsdCostAmount(row.amount) : '--'}
          </strong>
        </div>
      ))}
    </div>
  );
}

function upstreamCostPricingRows(breakdown, t) {
  const costCoefficient = safeNumber(breakdown?.cost_coefficient);
  const feeMultiplier = safeNumber(breakdown?.fee_multiplier);
  const tokenMultiplier = safeNumber(breakdown?.token_multiplier);
  const rechargeMultiplier = safeNumber(breakdown?.recharge_multiplier);
  return [
    [
      t('成本系数'),
      costCoefficient > 0 ? formatCostRatio(costCoefficient) : '--',
    ],
    [
      t('费用计算倍率'),
      feeMultiplier > 0 ? formatCostRatio(feeMultiplier) : '--',
    ],
    [
      t('充值倍率'),
      rechargeMultiplier > 0 ? formatCostRatio(rechargeMultiplier) : '--',
    ],
    [
      t('1:1 实际成本倍率'),
      tokenMultiplier > 0 ? formatCostRatio(tokenMultiplier) : '--',
    ],
  ].filter(([, value]) => value && value !== '--');
}

function UpstreamCostDetailPanel({ record, t }) {
  const status = upstreamCostStatus(record);
  const breakdown = record?.upstream_cost_breakdown || {};
  const rows = upstreamCostSummaryComponentRows(breakdown, t);
  const pricingRows = upstreamCostPricingRows(breakdown, t);
  const source = upstreamCostSourceLabel(status.source, status.accuracy, t);
  const upstreamModel =
    record?.upstream_cost_model ||
    record?.upstream_model ||
    record?.requested_model ||
    '';
  return (
    <div className='ct-model-gateway-upstream-cost-detail'>
      <div className='ct-model-gateway-upstream-cost-detail-kpis'>
        <div>
          <span>{t('供应商成本')}</span>
          <strong>
            {status.amount > 0 ? formatUsdCostAmount(status.amount) : source}
          </strong>
        </div>
        <div>
          <span>{t('成本来源')}</span>
          <strong>{source}</strong>
        </div>
        <div>
          <span>{t('上游模型')}</span>
          <strong title={upstreamModel}>{upstreamModel || '--'}</strong>
        </div>
      </div>

      {pricingRows.length > 0 && (
        <div className='ct-model-gateway-upstream-cost-config'>
          {pricingRows.map(([label, value]) => (
            <Tag key={label} color='cyan' type='light' shape='circle'>
              {label} {value}
            </Tag>
          ))}
        </div>
      )}

      {rows.length > 0 ? (
        <div className='ct-model-gateway-upstream-cost-breakdown'>
          <div className='ct-model-gateway-upstream-cost-breakdown-head'>
            <span>{t('成本项')}</span>
            <span>{t('本次用量')}</span>
            <span>{t('推导单价')}</span>
            <span>{t('金额')}</span>
          </div>
          {rows.map((row) => {
            const itemLabel = row.label || '--';
            const metaParts = String(row.meta || '').split(' · ');
            return (
              <div
                className='ct-model-gateway-upstream-cost-breakdown-row'
                key={itemLabel}
              >
                <strong>{itemLabel}</strong>
                <span>{metaParts[0] || '--'}</span>
                <span>{metaParts[1] || '--'}</span>
                <strong>
                  {row.amount > 0 ? formatUsdCostAmount(row.amount) : '--'}
                </strong>
              </div>
            );
          })}
        </div>
      ) : (
        <Typography.Text type='tertiary'>
          {status.source === 'pending'
            ? t('成本正在后台异步计算')
            : t('暂无可展示的成本明细')}
        </Typography.Text>
      )}
    </div>
  );
}

function UserRequestCostSummaryTooltip({ record, t }) {
  return (
    <div className='ct-model-gateway-cost-summary-tooltip'>
      <UserRequestUpstreamCostTooltip record={record} t={t} />
      <div className='ct-model-gateway-cost-summary-tooltip-divider' />
      <UserRequestCostTooltip billing={record?.billing} t={t} />
    </div>
  );
}

function UserRequestCostSummaryCell({ record, t }) {
  const status = upstreamCostStatus(record);
  const billing = record?.billing;
  const processing = isUserRequestProcessing(record);
  const channelRatio = safeNumber(
    record?.upstream_cost_breakdown?.token_multiplier,
  );
  const hasCost = status.amount > 0;
  const upstreamRatioLabel =
    channelRatio > 0 ? formatCostRatio(channelRatio) : '--';
  const upstreamLabel = hasCost
    ? formatUsdCostAmount(status.amount)
    : upstreamCostSourceLabel(status.source, status.accuracy, t);
  const upstreamSummaryLabel =
    upstreamRatioLabel !== '--'
      ? `${upstreamLabel} · ${upstreamRatioLabel}`
      : upstreamLabel;
  const billingLabel = billing
    ? renderQuota(billingReferenceQuota(billing), 6)
    : processing || String(record?.status || '').trim() === 'settling'
      ? t('待结算')
      : '--';
  const dynamicBillingLabel = dynamicBillingSummaryLabel(billing, t);

  return (
    <div
      className={`ct-model-gateway-user-request-cost-summary-col ${
        hasCost
          ? ''
          : `ct-model-gateway-user-request-cost-summary-${status.source || 'pending'}`
      }`}
    >
      <HoverCard
        content={<UserRequestCostSummaryTooltip record={record} t={t} />}
        className='ct-model-gateway-user-request-cost-summary-trigger'
      >
        <div className='ct-model-gateway-user-request-cost-summary-line'>
          <span>{t('上游成本')}</span>
          <strong title={t('1:1 实际成本倍率')}>{upstreamSummaryLabel}</strong>
        </div>
        <div className='ct-model-gateway-user-request-cost-summary-line ct-model-gateway-user-request-cost-summary-billing'>
          <span>{t('平台计费')}</span>
          <strong>{billingLabel}</strong>
        </div>
        {dynamicBillingLabel && (
          <div className='ct-model-gateway-user-request-cost-summary-line ct-model-gateway-user-request-cost-summary-dynamic'>
            <span>{t('动态倍率')}</span>
            <strong title={dynamicBillingLabel}>{dynamicBillingLabel}</strong>
          </div>
        )}
      </HoverCard>
    </div>
  );
}

function UserRequestScoreSummaryTooltip({ candidate, scoreRecord, t }) {
  const sampleCount = Number(candidate?.sample_count || 0);
  const scoreItems = normalizeScoreItemsForDisplay(
    candidate?.score_items,
  ).filter((item) => item.weight > 0 && !item.missing_reason);
  const routingScore = getCandidateRoutingScore(candidate);
  return (
    <div className='ct-model-gateway-user-request-score-tooltip'>
      <div className='ct-model-gateway-cost-tooltip-title'>
        {t('当前模型稳定评分')}
      </div>
      <div className='ct-model-gateway-cost-tooltip-row'>
        <span>{t('本次选中模型')}</span>
        <strong
          title={candidate?.upstream_model || scoreRecord?.requested_model}
        >
          {candidate?.upstream_model || scoreRecord?.requested_model || '--'}
        </strong>
      </div>
      <div className='ct-model-gateway-cost-tooltip-row'>
        <span>{t('稳定评分')}</span>
        <strong>
          {formatScore(candidate?.score_total || scoreRecord?.score_total)}
        </strong>
      </div>
      {routingScore !== null && (
        <div className='ct-model-gateway-cost-tooltip-row'>
          <span>{t('本次调度评分')}</span>
          <strong>{formatScore(routingScore)}</strong>
        </div>
      )}
      <div className='ct-model-gateway-cost-tooltip-row'>
        <span>{t('评分样本')}</span>
        <strong>
          {sampleCount > 0 ? formatNumber(sampleCount) : t('暂无评分样本')}
        </strong>
      </div>
      {scoreItems.length > 0 && (
        <div className='ct-model-gateway-cost-tooltip-divider' />
      )}
      <ScoreItemsMiniList items={scoreItems} t={t} />
    </div>
  );
}

function UserRequestScoreSummaryCell({ record, t, onOpenScoreHistory }) {
  const scoreRecord = userRequestScoreRecord(record);
  const candidate = selectedUserRequestScoreCandidate(record);
  const score =
    getCandidateScore(candidate) ?? Number(scoreRecord?.score_total || 0);
  const routingScore = getCandidateRoutingScore(candidate);
  const hasScore = Number.isFinite(score) && score > 0;
  const metricEntries = normalizeScoreItemsForDisplay(candidate?.score_items)
    .filter((item) => item.weight > 0 && !item.missing_reason)
    .slice(0, 3)
    .map((item) => [item.key, scoreItemLabel(item, t), item.score]);
  const canOpenHistory =
    typeof onOpenScoreHistory === 'function' &&
    Boolean(candidate || scoreHistoryCandidateFromRecord(scoreRecord));
  const handleOpenHistory = () => {
    if (!canOpenHistory) return;
    onOpenScoreHistory(
      candidate || scoreHistoryCandidateFromRecord(scoreRecord),
    );
  };

  if (!hasScore) {
    const processing = isUserRequestProcessing(record);
    return (
      <div className='ct-model-gateway-user-request-score-col ct-model-gateway-user-request-score-empty'>
        <strong>--</strong>
        <span>{processing ? t('运行中，暂未评分') : t('未命中评分样本')}</span>
      </div>
    );
  }

  return (
    <div className='ct-model-gateway-user-request-score-col'>
      <HoverCard
        content={
          <UserRequestScoreSummaryTooltip
            candidate={candidate}
            scoreRecord={scoreRecord}
            t={t}
          />
        }
        className={`ct-model-gateway-user-request-score-trigger${
          canOpenHistory ? ' ct-model-gateway-user-request-score-clickable' : ''
        }`}
      >
        <button
          type='button'
          disabled={!canOpenHistory}
          onClick={handleOpenHistory}
          className='ct-model-gateway-user-request-score-main'
          title={canOpenHistory ? t('查看评分变更') : t('当前模型稳定评分')}
        >
          <span>{t('稳定评分')}</span>
          <strong>{formatScore(score)}</strong>
        </button>
        <div className='ct-model-gateway-user-request-score-meta'>
          {routingScore !== null && (
            <em>
              {t('本次调度评分')} {formatScore(routingScore)}
            </em>
          )}
          {metricEntries.map(([key, label, value]) => (
            <small key={key}>
              {label} {formatScore(value)}
            </small>
          ))}
        </div>
      </HoverCard>
    </div>
  );
}

function compareUserRequestsForDisplay(a, b) {
  const leftProcessing = isUserRequestProcessing(a);
  const rightProcessing = isUserRequestProcessing(b);
  if (leftProcessing !== rightProcessing) {
    return leftProcessing ? -1 : 1;
  }
  const leftTime = leftProcessing
    ? normalizeTimestamp(a?.created_at)
    : normalizeTimestamp(a?.completed_at) || normalizeTimestamp(a?.created_at);
  const rightTime = rightProcessing
    ? normalizeTimestamp(b?.created_at)
    : normalizeTimestamp(b?.completed_at) || normalizeTimestamp(b?.created_at);
  const timeDiff = rightTime - leftTime;
  if (timeDiff !== 0) return timeDiff;
  const completedDiff =
    normalizeTimestamp(b?.completed_at) - normalizeTimestamp(a?.completed_at);
  if (completedDiff !== 0) return completedDiff;
  const createdDiff =
    normalizeTimestamp(b?.created_at) - normalizeTimestamp(a?.created_at);
  if (createdDiff !== 0) return createdDiff;
  const idDiff = Number(b?.id || 0) - Number(a?.id || 0);
  if (idDiff !== 0) return idDiff;
  return String(b?.request_id || '').localeCompare(String(a?.request_id || ''));
}

function compactUserRequestId(requestId) {
  const value = String(requestId || '').trim();
  if (!value || value.length <= 22) return value;
  return `${value.slice(0, 12)}...${value.slice(-6)}`;
}

function buildUserRequestSparkValues(trends, key) {
  if (!Array.isArray(trends)) return [];
  return trends
    .filter((item) =>
      key === 'requests'
        ? Number(item?.requests) > 0
        : Number(item?.user_requests || 0) > 0,
    )
    .slice(-16)
    .map((item) => Number(item?.[key]) || 0);
}

function buildUserRequestTrendSpec(trends, t) {
  const trendRows = Array.isArray(trends) ? trends : [];
  const values = trendRows.flatMap((item) => {
    const bucket = formatBucketRange(item);
    const requests = Number(item.requests || 0);
    return [
      {
        bucket,
        metric: t('请求量'),
        value: requests,
        axis: 'count',
      },
      {
        bucket,
        metric: t('用户成功率'),
        value: Number(item.user_success_rate || 0) * 100,
        axis: 'rate',
      },
      {
        bucket,
        metric: t('P95 首包'),
        value: Number(item.p95_ttft_ms || 0),
        axis: 'latency',
      },
    ];
  });

  return {
    type: 'line',
    data: [{ id: 'user-request-trends', values }],
    xField: 'bucket',
    yField: 'value',
    seriesField: 'metric',
    axes: [
      {
        orient: 'bottom',
        label: { visible: false },
        tick: { visible: false },
        domainLine: { visible: false },
      },
      {
        orient: 'left',
        label: { style: { fill: '#64748b', fontSize: 11 } },
        grid: { visible: true, style: { stroke: 'rgba(148, 163, 184, 0.16)' } },
      },
    ],
    color: ['#14b8a6', '#059669', '#f97316'],
    line: {
      style: {
        lineWidth: (datum) => (datum.metric === t('请求量') ? 1.2 : 2.3),
        strokeOpacity: (datum) => (datum.metric === t('请求量') ? 0.45 : 1),
      },
    },
    point: { visible: false },
    legends: {
      visible: true,
      orient: 'top',
      position: 'end',
      item: {
        label: { style: { fill: '#475569', fontSize: 11 } },
      },
    },
    tooltip: {
      visible: true,
    },
    padding: { top: 8, right: 12, bottom: 8, left: 44 },
    background: { fill: 'transparent' },
    animation: false,
  };
}

function MiniSparkline({ values, tone = 'success', variant = 'line' }) {
  const spec = useMemo(() => {
    const normalizedValues = Array.isArray(values)
      ? values
          .map((value) => Number(value))
          .filter((value) => Number.isFinite(value))
      : [];
    const sourcePoints = normalizedValues.length
      ? normalizedValues
      : Array.from({ length: 12 }, () => 0);
    const min = Math.min(...sourcePoints);
    const max = Math.max(...sourcePoints);
    const flat = max === min || max - min < 0.000001;
    const baseline = Math.max(Math.abs(max), Math.abs(min), 1);
    const chartValues = sourcePoints.map((value, index) => ({
      x: index,
      y: flat
        ? baseline + Math.sin(index * 1.7 + 0.35) * baseline * 0.045
        : value,
    }));
    const color = MINI_SPARKLINE_COLORS[tone] || MINI_SPARKLINE_COLORS.default;

    return {
      type: 'line',
      data: [{ id: 'sparkline', values: chartValues }],
      xField: 'x',
      yField: 'y',
      autoFit: true,
      padding: {
        top: 2,
        right: 1,
        bottom: 2,
        left: 1,
      },
      axes: [
        {
          orient: 'bottom',
          visible: false,
        },
        {
          orient: 'left',
          visible: false,
        },
      ],
      legends: {
        visible: false,
      },
      tooltip: {
        visible: false,
      },
      crosshair: {
        visible: false,
      },
      animation: false,
      line: {
        style: {
          stroke: color,
          lineWidth: variant === 'line' ? 2.2 : 2,
          lineCap: 'round',
          lineJoin: 'round',
        },
      },
      point: {
        visible: false,
      },
      background: {
        fill: 'transparent',
      },
    };
  }, [values, tone, variant]);

  return (
    <div
      className={`ct-model-gateway-mini-sparkline ct-model-gateway-mini-sparkline-${tone} ct-model-gateway-mini-sparkline-${variant}`}
      aria-hidden='true'
    >
      <VChart spec={spec} option={MINI_SPARKLINE_CHART_OPTIONS} />
    </div>
  );
}

function OperationKpiCard({
  icon: Icon,
  label,
  value,
  detail,
  tone = 'success',
  sparkValues,
  delta,
}) {
  const colorMap = {
    success: 'green',
    warning: 'orange',
    danger: 'red',
    default: 'blue',
  };

  return (
    <div
      className={`ct-model-gateway-kpi-card ct-model-gateway-kpi-card-${tone}`}
    >
      <div className='ct-model-gateway-kpi-head'>
        <div className='ct-model-gateway-kpi-title'>
          <Avatar size='extra-small' color={colorMap[tone] || 'blue'}>
            <Icon size={14} />
          </Avatar>
          <span>{label}</span>
        </div>
        <div className='ct-model-gateway-kpi-meta' title={detail}>
          <span>{detail}</span>
          {delta && <em>{delta}</em>}
        </div>
      </div>
      <strong>{value}</strong>
      <MiniSparkline values={sparkValues} tone={tone} variant='line' />
    </div>
  );
}

function HealthOverviewCard({ health, summary, trends, t }) {
  const statusLabel = getHealthStatusLabel(health.status, t);
  const scoreTone = health.tone;

  return (
    <DashboardCard
      className={`ct-model-gateway-health-card ct-model-gateway-health-card-${scoreTone}`}
      bodyClassName='ct-model-gateway-health-body'
    >
      <div className='ct-model-gateway-health-main'>
        <div className='ct-model-gateway-health-shield'>
          <Activity size={34} />
        </div>
        <div className='ct-model-gateway-health-head-copy'>
          <div className='ct-model-gateway-health-status'>{statusLabel}</div>
          <div className='ct-model-gateway-health-score-row'>
            <span>{t('综合状态')}</span>
            <strong>{formatNumber(health.score)}</strong>
          </div>
        </div>
      </div>
      <MiniSparkline
        values={buildSparkValues(trends, 'success_rate')}
        tone={
          scoreTone === 'danger'
            ? 'danger'
            : scoreTone === 'warning'
              ? 'warning'
              : 'success'
        }
        variant='line'
      />
      <div className='ct-model-gateway-health-impact-grid'>
        <div>
          <small>{t('影响模型')}</small>
          <strong>{formatNumber(health.affectedModels)}</strong>
          <span>
            {t('失败')} {formatNumber(summary.failures)}
          </span>
        </div>
        <div>
          <small>{t('影响渠道')}</small>
          <strong>{formatNumber(health.affectedChannels)}</strong>
          <span>
            {t('熔断')} {formatNumber(health.circuitOpen)}
          </span>
        </div>
      </div>
      <Typography.Text type='secondary' size='small'>
        {getHealthStatusDescription(health, t)}
      </Typography.Text>
    </DashboardCard>
  );
}

function UserRequestHealthCard({ health, summary, trends, t }) {
  const userRequests = Number(summary.user_requests || 0);
  return (
    <DashboardCard
      className={`ct-model-gateway-user-health ct-model-gateway-user-health-${health.tone}`}
      bodyClassName='ct-model-gateway-user-health-body'
    >
      <div className='ct-model-gateway-user-health-head'>
        <div className='ct-model-gateway-user-health-icon'>
          <Gauge size={34} />
        </div>
        <div>
          <span>{t('用户感知健康')}</span>
          <strong>{formatNumber(health.score)}</strong>
          <small>{getUserRequestStatusLabel(health.status, t)}</small>
        </div>
      </div>
      <MiniSparkline
        values={buildUserRequestSparkValues(trends, 'user_success_rate')}
        tone={
          health.tone === 'danger'
            ? 'danger'
            : health.tone === 'warning'
              ? 'warning'
              : 'success'
        }
      />
      <div className='ct-model-gateway-user-health-metrics'>
        <div>
          <small>{t('请求成功')}</small>
          <strong>{formatNumber(summary.successes)}</strong>
          <span>{formatPercent(summary.user_success_rate)}</span>
        </div>
        <div>
          <small>{t('探活请求')}</small>
          <strong>{formatNumber(summary.health_probes)}</strong>
          <span>{t('不计入最终失败')}</span>
        </div>
        <div>
          <small>{t('最终失败')}</small>
          <strong>{formatNumber(summary.final_failures)}</strong>
          <span>
            {formatNumber(userRequests)} {t('客户端请求')}
          </span>
        </div>
        <div>
          <small>{t('智能调度')}</small>
          <strong>{formatNumber(summary.recovered)}</strong>
          <span>{t('智能调度后成功')}</span>
        </div>
      </div>
    </DashboardCard>
  );
}

function DynamicBillingMiniPanel({
  overview,
  t,
  title,
  countdownEnabled = true,
  averageMode = false,
}) {
  const groups = Array.isArray(overview?.groups) ? overview.groups : [];
  const [countdownNow, setCountdownNow] = useState(() => Date.now());

  useEffect(() => {
    const timer = window.setInterval(() => {
      setCountdownNow(Date.now());
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  if (!groups.length) return null;
  const prioritizedGroups = [...groups].sort((left, right) => {
    const statusDiff =
      dynamicBillingOverviewStatusRank(left?.status) -
      dynamicBillingOverviewStatusRank(right?.status);
    if (statusDiff !== 0) return statusDiff;
    const latestDiff =
      safeNumber(right?.latest_calculated_at) -
      safeNumber(left?.latest_calculated_at);
    if (latestDiff !== 0) return latestDiff;
    return String(left?.policy_group || '').localeCompare(
      String(right?.policy_group || ''),
    );
  });
  const primary = prioritizedGroups[0] || {};
  const priceValue = averageMode
    ? formatDynamicBillingPriceAverage(primary)
    : formatDynamicBillingPriceCompact(primary);
  const ratioValue = averageMode
    ? formatDynamicBillingRatioAverage(primary)
    : formatDynamicBillingRatioCurrent(primary);
  const costPriceValue = averageMode
    ? formatDynamicBillingCostPriceAverage(primary, overview)
    : formatDynamicBillingCostPriceCompact(primary, overview);
  const costRatioValue = averageMode
    ? formatDynamicBillingCostRatioAverage(primary, overview)
    : formatDynamicBillingCostRatioCurrent(primary, overview);
  const costValue =
    costPriceValue !== '--' || costRatioValue !== '--'
      ? [costPriceValue, costRatioValue]
          .filter((item) => item && item !== '--')
          .join(' · ')
      : '--';
  const groupValue = dynamicBillingDisplayGroup(primary);
  const referenceModelValue = String(
    primary?.reference_model || primary?.current_model || '',
  ).trim();
  const priceLabel = averageMode ? t('均价') : t('参考价');
  const costSourceLabel =
    dynamicBillingCostSourceLabel(primary?.cost_source, t) ||
    dynamicBillingCostSourceLabel(overview?.cost_source, t);
  const statusLabel = formatDynamicBillingOverviewStatus(primary?.status, t);
  const upstreamCost = safeNumber(primary?.upstream_cost_usd);
  const requiredRevenue = safeNumber(primary?.required_revenue_usd);
  const costMultiplierRatio = safeNumber(primary?.cost_multiplier);
  const grossMarginMultiplier =
    upstreamCost > 0 && requiredRevenue > 0
      ? requiredRevenue / upstreamCost
      : dynamicBillingCostFactor(overview, primary);
  const targetRatio = safeNumber(
    primary?.effective_ratio || primary?.target_ratio,
  );
  const refreshCountdown = countdownEnabled
    ? dynamicBillingRemainingSeconds(
        primary?.latest_calculated_at,
        overview?.refresh_seconds,
        countdownNow,
      )
    : 0;
  const statusClassName = dynamicBillingOverviewStatusClassName(
    primary?.status,
  );
  const profitDetailRows =
    primary?.cost_source === 'profit_24h'
      ? [
          {
            label: t('上游成本'),
            value:
              upstreamCost > 0
                ? `${formatUsdCostAmount(upstreamCost)}${
                    targetRatio > 0
                      ? ` · ${formatDynamicCostRatio(targetRatio)}`
                      : ''
                  }`
                : '--',
          },
          {
            label: t('成本倍率'),
            value:
              costMultiplierRatio > 0
                ? formatCostRatio(costMultiplierRatio)
                : '--',
          },
          {
            label: t('毛利换算'),
            value:
              grossMarginMultiplier > 0
                ? formatCostRatio(grossMarginMultiplier)
                : '--',
          },
        ]
      : [];
  const costRow = (
    <div className='ct-model-gateway-dynamic-mini-cost-row'>
      <span>{t('成本')}</span>
      <bdi className='ct-model-gateway-dynamic-mini-cost-value'>
        {costValue}
      </bdi>
    </div>
  );
  return (
    <div className='ct-model-gateway-kpi-card ct-model-gateway-kpi-card-default ct-model-gateway-dynamic-mini-card'>
      <div className='ct-model-gateway-kpi-head'>
        <div className='ct-model-gateway-kpi-title'>
          <Avatar size='extra-small' color='blue'>
            <RadioTower size={14} />
          </Avatar>
          <span>{title || t('当前动态倍率')}</span>
        </div>
        <div className='ct-model-gateway-dynamic-mini-head-meta'>
          {groupValue ? (
            <span className='ct-model-gateway-dynamic-mini-chip is-group'>
              {groupValue}
            </span>
          ) : null}
          {referenceModelValue ? (
            <span className='ct-model-gateway-dynamic-mini-chip is-model'>
              {referenceModelValue}
            </span>
          ) : null}
          {costSourceLabel ? (
            <span className='ct-model-gateway-dynamic-mini-chip is-model'>
              {costSourceLabel}
            </span>
          ) : null}
          {refreshCountdown > 0 ? (
            <span className='ct-model-gateway-dynamic-mini-chip is-countdown'>
              {refreshCountdown}s
            </span>
          ) : null}
        </div>
      </div>
      <div className='ct-model-gateway-dynamic-mini-main-row'>
        <strong className='ct-model-gateway-dynamic-mini-price'>
          {ratioValue}
        </strong>
        {priceValue !== '--' ? (
          <div className='ct-model-gateway-dynamic-mini-inline-ratio'>
            <span className='ct-model-gateway-dynamic-mini-inline-ratio-label'>
              {priceLabel}
            </span>
            <bdi className='ct-model-gateway-dynamic-mini-inline-ratio-value'>
              {priceValue}
            </bdi>
          </div>
        ) : null}
      </div>
      <div className='ct-model-gateway-dynamic-mini-foot-row'>
        {profitDetailRows.length > 0 ? (
          <Tooltip
            content={
              <div className='ct-model-gateway-dynamic-mini-tooltip'>
                {profitDetailRows.map((item) => (
                  <div key={item.label}>
                    <span>{item.label}</span>
                    <bdi>{item.value}</bdi>
                  </div>
                ))}
              </div>
            }
          >
            {costRow}
          </Tooltip>
        ) : (
          costRow
        )}
        <span
          className={`ct-model-gateway-dynamic-mini-status ${statusClassName}`}
        >
          {statusLabel}
        </span>
      </div>
    </div>
  );
}

function UserRequestTrendPanel({ trends, t }) {
  const spec = useMemo(() => buildUserRequestTrendSpec(trends, t), [trends, t]);
  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <Activity size={17} />
          {t('客户端请求趋势')}
        </span>
      }
      bodyClassName='ct-model-gateway-user-trend-body'
    >
      {Array.isArray(trends) &&
      trends.some((item) => Number(item?.requests) > 0) ? (
        <VChart spec={spec} option={MINI_SPARKLINE_CHART_OPTIONS} />
      ) : (
        <div className='ct-model-gateway-trend-empty'>
          {t('暂无客户端请求趋势')}
        </div>
      )}
    </DashboardCard>
  );
}

function UserRequestRankPanel({ title, icon: Icon, rows, type, t }) {
  const items = [...(rows || [])]
    .filter((item) => Number(item?.user_requests || 0) > 0)
    .slice(0, 6);
  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <Icon size={17} />
          {title}
        </span>
      }
      bodyClassName='ct-model-gateway-user-rank-body'
    >
      <div className='ct-model-gateway-user-rank-head'>
        <span>{type === 'model' ? t('模型') : t('分组')}</span>
        <span>{t('用户成功率')}</span>
        <span>{t('P95 首包')}</span>
        <span>{t('智能调度')}</span>
        <span>{t('请求量')}</span>
      </div>
      {items.length ? (
        items.map((item) => {
          const rate = clampRate(item.user_success_rate);
          const tone = getSuccessTone(
            item.user_success_rate,
            item.user_requests,
          );
          return (
            <div
              className='ct-model-gateway-user-rank-row'
              key={`${type}-${item.key}`}
            >
              <div className='ct-model-gateway-leaderboard-name'>
                <Avatar
                  size='extra-small'
                  color={type === 'model' ? 'blue' : 'cyan'}
                >
                  {type === 'model' ? <Bot size={13} /> : <Layers3 size={13} />}
                </Avatar>
                <Typography.Text strong ellipsis={{ showTooltip: true }}>
                  {item.key || t('未知')}
                </Typography.Text>
              </div>
              <div className='ct-model-gateway-user-rank-rate'>
                <span
                  className={`ct-model-gateway-leaderboard-rate ct-model-gateway-leaderboard-rate-${tone}`}
                >
                  {formatAttemptRate(
                    item.user_success_rate,
                    item.user_requests,
                  )}
                </span>
                <div className='ct-model-gateway-leaderboard-meter'>
                  <span style={{ width: `${Math.round(rate * 100)}%` }} />
                </div>
              </div>
              <span>{formatLatency(item.p95_ttft_ms)}</span>
              <span>{formatNumber(item.recovered)}</span>
              <span>{formatNumber(item.user_requests || item.requests)}</span>
            </div>
          );
        })
      ) : (
        <Typography.Text type='secondary' size='small'>
          {t('暂无排行数据')}
        </Typography.Text>
      )}
    </DashboardCard>
  );
}

function UserRequestRecentTable({
  records,
  t,
  refreshing,
  onRefresh,
  onOpenDispatchDetail,
  onOpenScoreHistory,
  dispatchDetailLoading,
  pageSize = DEFAULT_USER_REQUEST_PAGE_SIZE,
  onPageSizeChange,
}) {
  const [nowSeconds, setNowSeconds] = useState(() =>
    Math.floor(Date.now() / 1000),
  );
  const [query, setQuery] = useState('');
  const [showHealthProbes, setShowHealthProbes] = useState(false);
  const [page, setPage] = useState(1);
  const [jumpPage, setJumpPage] = useState('1');
  const hasProcessing = useMemo(
    () => (records || []).some((record) => isUserRequestProcessing(record)),
    [records],
  );

  useEffect(() => {
    if (!hasProcessing) return undefined;
    const timer = window.setInterval(() => {
      setNowSeconds(Math.floor(Date.now() / 1000));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [hasProcessing]);

  const filteredItems = useMemo(
    () =>
      [...(records || [])]
        .filter((record) => showHealthProbes || !isHealthProbeRecord(record))
        .filter((record) => userRequestMatchesQuery(record, query))
        .sort(compareUserRequestsForDisplay),
    [query, records, showHealthProbes],
  );
  const totalPages = Math.max(1, Math.ceil(filteredItems.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pageItems = filteredItems.slice(
    (currentPage - 1) * pageSize,
    currentPage * pageSize,
  );

  useEffect(() => {
    setPage(1);
  }, [query, records, showHealthProbes]);

  useEffect(() => {
    setJumpPage(String(currentPage));
  }, [currentPage]);

  const gotoPage = useCallback(
    (nextPage) => {
      setPage(Math.min(totalPages, Math.max(1, nextPage)));
    },
    [totalPages],
  );
  const handlePageSizeChange = useCallback(
    (value) => {
      const nextPageSize = Number(value);
      if (!Number.isFinite(nextPageSize) || nextPageSize <= 0) return;
      onPageSizeChange(nextPageSize);
      setPage(1);
    },
    [onPageSizeChange],
  );
  const submitJumpPage = useCallback(() => {
    const nextPage = Number(jumpPage);
    if (!Number.isFinite(nextPage)) {
      setJumpPage(String(currentPage));
      return;
    }
    gotoPage(nextPage);
  }, [currentPage, gotoPage, jumpPage]);
  const handleCopyRequestId = useCallback(
    async (requestId) => {
      if (!requestId) return;
      const ok = await copy(requestId);
      if (ok) {
        Toast.success(t('复制成功'));
      } else {
        showError(t('复制失败'));
      }
    },
    [t],
  );
  const visiblePages = useMemo(() => {
    const start = Math.max(1, Math.min(currentPage - 2, totalPages - 4));
    return Array.from(
      { length: Math.min(5, totalPages) },
      (_, index) => start + index,
    );
  }, [currentPage, totalPages]);

  return (
    <DashboardCard bodyClassName='ct-model-gateway-user-request-list-body'>
      <div className='ct-model-gateway-user-request-list-head'>
        <div>
          <h3>{t('用户请求实时台账')}</h3>
          <p>
            {t(
              '处理中置顶，已完成按最后完成时间倒序；成本、计费与评分为异步核算口径',
            )}
          </p>
        </div>
        <div className='ct-model-gateway-user-request-list-actions'>
          <label className='ct-model-gateway-user-request-probe-toggle'>
            <Switch
              size='small'
              checked={showHealthProbes}
              onChange={setShowHealthProbes}
            />
            <span>{t('显示探活数据')}</span>
          </label>
          <Input
            value={query}
            onChange={setQuery}
            prefix={<Search size={14} />}
            placeholder={t('搜索请求 ID / 用户 / 模型 / 渠道')}
            className='ct-model-gateway-user-request-search'
            showClear
          />
          <Button
            icon={<RefreshCw size={15} />}
            loading={refreshing}
            onClick={onRefresh}
          >
            {t('刷新')}
          </Button>
        </div>
      </div>

      <div className='ct-model-gateway-user-request-table-wrap'>
        <div className='ct-model-gateway-user-request-table'>
          <div className='ct-model-gateway-user-request-table-head'>
            {[
              { key: 'status', label: t('处置状态') },
              { key: 'user', label: t('用户') },
              { key: 'request', label: t('请求 / 路由') },
              { key: 'score', label: t('调度评分'), hint: true },
              { key: 'cost', label: t('成本 / 计费'), hint: true },
              { key: 'duration', label: t('耗时'), hint: true },
              { key: 'ttft', label: t('首包'), hint: true },
              { key: 'complete', label: t('时间状态') },
              { key: 'action', label: t('诊断') },
            ].map(({ key, label, hint }) => (
              <span key={key}>
                {label}
                {hint && <Info size={13} />}
              </span>
            ))}
          </div>

          {pageItems.length ? (
            <div className='ct-model-gateway-user-request-rows'>
              {pageItems.map((record) => {
                const meta = getUserRequestStatusMeta(record, t);
                const processing = isUserRequestProcessing(record);
                const durationMs = userRequestLiveDurationMs(
                  record,
                  nowSeconds,
                );
                const requestId = record.request_id || '';
                const userLabel = formatUserRequestUser(record);
                const userIDLabel = formatUserRequestUserID(record, t);
                const channelLabel = formatUserRequestChannel(record);
                const channelIdLabel = formatUserRequestChannelId(record);
                const groupFlowLabel = formatUserRequestGroupFlow(record);
                const groupRatioLabel = formatUserRequestGroupRatio(record);
                const hasTTFT = Number(record.ttft_ms || 0) > 0;
                const smartSwitchRecovered = isSmartSwitchRecovered(record);
                const firstByteTimeoutAttempts = Array.isArray(
                  record.attempt_records,
                )
                  ? record.attempt_records.filter((attempt) =>
                      isFirstByteTimeoutAttempt(attempt),
                    ).length
                  : 0;
                const issueLabel = processing
                  ? userRequestProcessingStage(record, durationMs, hasTTFT, t)
                  : firstByteTimeoutAttempts > 0
                    ? t('首字超时切换')
                    : hasModelGatewayWarning(record) && !processing
                      ? modelGatewayWarningLabel(record, t)
                      : !record.final_success &&
                          record.final_error_category &&
                          meta.tone !== 'aborted' &&
                          !processing
                        ? formatUserRequestErrorCategory(
                            record.final_error_category,
                            t,
                          )
                        : record.final_success &&
                            (record.empty_output || record.experience_issue) &&
                            !processing
                          ? formatUserRequestExperienceIssue(
                              record.experience_issue ||
                                (record.empty_output ? 'empty_output' : ''),
                              t,
                            )
                          : '';
                const StatusIcon =
                  meta.tone === 'processing'
                    ? RadioTower
                    : meta.tone === 'aborted' || meta.tone === 'quota'
                      ? Ban
                      : meta.tone === 'failed' || meta.tone === 'probe-warning'
                        ? Info
                        : CheckCircle2;
                const durationTone = getThresholdTone(
                  durationMs,
                  LATENCY_THRESHOLDS.avgDurationMs,
                );
                const ttftTone = getThresholdTone(
                  record.ttft_ms,
                  LATENCY_THRESHOLDS.ttftMs,
                );
                const statusCaption = userRequestStatusCaption(
                  record,
                  meta,
                  processing,
                  hasTTFT,
                  durationMs,
                  t,
                );

                return (
                  <div
                    className={`ct-model-gateway-user-request-row ct-model-gateway-user-request-row-${meta.tone}`}
                    key={requestId || record.id}
                  >
                    <div className='ct-model-gateway-user-request-status-col'>
                      <div className='ct-model-gateway-user-request-status-top-line'>
                        <div
                          className={`ct-model-gateway-user-request-status-pill ct-model-gateway-user-request-status-pill-${meta.tone}`}
                        >
                          <StatusIcon size={13} />
                          <span>{meta.label}</span>
                        </div>
                        {smartSwitchRecovered && (
                          <Tooltip
                            content={t('请求通过智能调度切换到可用渠道后完成')}
                          >
                            <span className='ct-model-gateway-user-request-smart-switch-icon'>
                              <GitBranch size={12} />
                            </span>
                          </Tooltip>
                        )}
                      </div>
                      <small title={statusCaption}>{statusCaption}</small>
                    </div>

                    <div className='ct-model-gateway-user-request-user-col'>
                      <div className='ct-model-gateway-user-request-user-line'>
                        <UserRound size={14} />
                        <Typography.Text
                          ellipsis={{ showTooltip: true }}
                          title={userLabel}
                        >
                          {userLabel}
                        </Typography.Text>
                      </div>
                      <span title={userIDLabel}>{userIDLabel}</span>
                    </div>

                    <div className='ct-model-gateway-user-request-summary-col'>
                      <div className='ct-model-gateway-user-request-id-line'>
                        <Typography.Text
                          ellipsis={{ showTooltip: true }}
                          className='ct-model-gateway-request-id'
                          title={requestId || '--'}
                        >
                          {compactUserRequestId(requestId) || '--'}
                        </Typography.Text>
                        {requestId && (
                          <Tooltip content={t('复制')}>
                            <Button
                              aria-label={t('复制')}
                              className='ct-model-gateway-user-request-copy-btn'
                              icon={<Copy size={13} />}
                              size='small'
                              type='tertiary'
                              onClick={() => handleCopyRequestId(requestId)}
                            />
                          </Tooltip>
                        )}
                      </div>
                      <div className='ct-model-gateway-user-request-route-line'>
                        <strong title={record.requested_model || '--'}>
                          {record.requested_model || '--'}
                        </strong>
                        {(record.is_health_probe ||
                          record.request_meta?.is_health_probe) && (
                          <Tag color='cyan' size='small' type='light'>
                            {t('健康探活')}
                          </Tag>
                        )}
                        {smartSwitchRecovered && (
                          <Tooltip
                            content={t('请求通过智能调度切换到可用渠道后完成')}
                          >
                            <Tag
                              className='ct-model-gateway-user-request-smart-switch-tag'
                              color='cyan'
                              size='small'
                              type='light'
                            >
                              {t('智能切换')}
                            </Tag>
                          </Tooltip>
                        )}
                        <ModelGatewayWarningTag record={record} t={t} />
                        <span title={channelLabel}>{channelLabel}</span>
                        {channelIdLabel && (
                          <code title={t('渠道 ID')}>{channelIdLabel}</code>
                        )}
                      </div>
                      <div className='ct-model-gateway-user-request-group-line'>
                        <span title={groupFlowLabel}>{groupFlowLabel}</span>
                        {groupRatioLabel && (
                          <em title={groupRatioLabel}>{groupRatioLabel}</em>
                        )}
                        {issueLabel && (
                          <small title={issueLabel}>{issueLabel}</small>
                        )}
                        {(record.is_health_probe ||
                          record.request_meta?.is_health_probe) && (
                          <small title={formatProbeReasonWithScope(record, t)}>
                            {formatProbeReasonWithScope(record, t)}
                          </small>
                        )}
                      </div>
                    </div>

                    <UserRequestScoreSummaryCell
                      record={record}
                      t={t}
                      onOpenScoreHistory={onOpenScoreHistory}
                    />

                    <UserRequestCostSummaryCell record={record} t={t} />

                    <div
                      className={`ct-model-gateway-user-request-value-col ct-model-gateway-user-request-value-${durationTone}`}
                    >
                      <strong>{formatLatency(durationMs)}</strong>
                      <span>{userRequestDurationCaption(processing, t)}</span>
                    </div>

                    <div
                      className={`ct-model-gateway-user-request-value-col ct-model-gateway-user-request-value-${ttftTone}`}
                    >
                      <strong>
                        {hasTTFT ? formatLatency(record.ttft_ms) : '--'}
                      </strong>
                      <span>
                        {userRequestTTFTCaption(processing, hasTTFT, t)}
                      </span>
                    </div>

                    <div className='ct-model-gateway-user-request-complete-col'>
                      <HoverCard
                        content={
                          <UserRequestEventTooltip
                            record={record}
                            meta={meta}
                            processing={processing}
                            durationMs={durationMs}
                            t={t}
                          />
                        }
                        className='ct-model-gateway-user-request-complete-trigger'
                      >
                        <span className='ct-model-gateway-user-request-dot' />
                        <strong>
                          {userRequestDisplayTime(record, nowSeconds, t)}
                        </strong>
                      </HoverCard>
                      <span>
                        {userRequestTimeCaption(record, meta, processing, t)}
                      </span>
                    </div>

                    <div className='ct-model-gateway-user-request-action-col'>
                      <Tooltip content={t('查看调度链路与计费明细')}>
                        <Button
                          size='small'
                          type='tertiary'
                          aria-label={t('查看调度链路与计费明细')}
                          icon={<Eye size={14} />}
                          loading={dispatchDetailLoading === requestId}
                          disabled={!requestId}
                          onClick={() => onOpenDispatchDetail?.(record)}
                        >
                          {t('诊断')}
                        </Button>
                      </Tooltip>
                    </div>
                  </div>
                );
              })}
            </div>
          ) : (
            <EmptyState t={t} />
          )}
        </div>
      </div>

      <div className='ct-model-gateway-user-request-pagination'>
        <span>{t('共 {{count}} 条', { count: filteredItems.length })}</span>
        <Select
          value={pageSize}
          onChange={handlePageSizeChange}
          className='ct-model-gateway-user-request-page-size-select'
          size='small'
        >
          {USER_REQUEST_PAGE_SIZE_OPTIONS.map((option) => (
            <Select.Option key={option} value={option}>
              {option} {t('条/页')}
            </Select.Option>
          ))}
        </Select>
        <div className='ct-model-gateway-user-request-page-actions'>
          <Button
            size='small'
            type='tertiary'
            disabled={currentPage <= 1}
            onClick={() => gotoPage(1)}
            icon={<ChevronsLeft size={14} />}
            aria-label={t('第一页')}
          ></Button>
          <Button
            size='small'
            type='tertiary'
            disabled={currentPage <= 1}
            onClick={() => gotoPage(currentPage - 1)}
            icon={<ChevronLeft size={14} />}
            aria-label={t('上一页')}
          ></Button>
          {visiblePages.map((item) => (
            <Button
              key={item}
              size='small'
              type={item === currentPage ? 'primary' : 'tertiary'}
              onClick={() => gotoPage(item)}
            >
              {item}
            </Button>
          ))}
          <Button
            size='small'
            type='tertiary'
            disabled={currentPage >= totalPages}
            onClick={() => gotoPage(currentPage + 1)}
            icon={<ChevronRight size={14} />}
            aria-label={t('下一页')}
          ></Button>
          <Button
            size='small'
            type='tertiary'
            disabled={currentPage >= totalPages}
            onClick={() => gotoPage(totalPages)}
            icon={<ChevronsRight size={14} />}
            aria-label={t('最后一页')}
          ></Button>
        </div>
        <div className='ct-model-gateway-user-request-page-jump'>
          <span>{t('跳至')}</span>
          <Input
            value={jumpPage}
            size='small'
            onChange={setJumpPage}
            onBlur={submitJumpPage}
            onEnterPress={submitJumpPage}
          />
          <span>{t('页')}</span>
        </div>
      </div>
    </DashboardCard>
  );
}

function UserRequestDashboard({
  data,
  t,
  refreshing,
  onRefresh,
  onOpenDispatchDetail,
  onOpenScoreHistory,
  dispatchDetailLoading,
  dynamicRefreshCountdown,
  userRequestPageSize,
  onUserRequestPageSizeChange,
}) {
  const userRequests = data?.user_requests || {};
  const summary = userRequests.summary || {};
  const trends = userRequests.trends || [];
  const health = getUserRequestHealth(data);
  const dynamicBillingOverview = dynamicBillingOverviewFromData(data);
  const dynamicBilling7dOverview = dynamicBilling7dOverviewFromData(data);
  const dynamicBillingOverviewWithCountdown = useMemo(() => {
    if (!dynamicBillingOverview) return dynamicBillingOverview;
    return {
      ...dynamicBillingOverview,
      refresh_seconds: dynamicRefreshCountdown,
    };
  }, [dynamicBillingOverview, dynamicRefreshCountdown]);
  const hasDynamicBilling7dOverview =
    Array.isArray(dynamicBilling7dOverview?.groups) &&
    dynamicBilling7dOverview.groups.length > 0;
  const hasDynamicBillingOverview =
    Array.isArray(dynamicBillingOverviewWithCountdown?.groups) &&
    dynamicBillingOverviewWithCountdown.groups.length > 0;

  return (
    <div className='ct-model-gateway-user-layout'>
      <div className='ct-model-gateway-user-top'>
        <UserRequestHealthCard
          health={health}
          summary={summary}
          trends={trends}
          t={t}
        />
        <div
          className={`ct-model-gateway-user-kpi-grid ${
            hasDynamicBillingOverview || hasDynamicBilling7dOverview
              ? 'ct-model-gateway-user-kpi-grid-with-dynamic'
              : ''
          }`}
        >
          <OperationKpiCard
            icon={CheckCircle2}
            label={t('用户成功率')}
            value={formatAttemptRate(
              summary.user_success_rate,
              summary.user_requests,
            )}
            detail={`${formatNumber(summary.successes)} / ${formatNumber(
              Math.max(
                0,
                Number(summary.user_requests || 0) -
                  Number(summary.client_aborted || 0) -
                  Number(summary.user_quota_exhausted || 0),
              ),
            )}`}
            tone={getSuccessTone(
              summary.user_success_rate,
              summary.user_requests,
            )}
            sparkValues={buildUserRequestSparkValues(
              trends,
              'user_success_rate',
            )}
          />
          <DynamicBillingMiniPanel
            overview={dynamicBillingOverviewWithCountdown}
            t={t}
          />
          {hasDynamicBilling7dOverview ? (
            <DynamicBillingMiniPanel
              overview={dynamicBilling7dOverview}
              t={t}
              title={t('7天动态均价')}
              countdownEnabled={false}
              averageMode
            />
          ) : null}
          <OperationKpiCard
            icon={ListTree}
            label={t('客户端请求数')}
            value={formatNumber(summary.total_requests)}
            detail={`${formatNumber(summary.scanned_requests)} ${t('已扫描')} · ${formatNumber(summary.health_probes)} ${t('探活')}`}
            tone='default'
            sparkValues={buildUserRequestSparkValues(trends, 'requests')}
          />
          <OperationKpiCard
            icon={GitBranch}
            label={t('智能调度')}
            value={formatNumber(summary.recovered)}
            detail={t('智能调度后成功')}
            tone={Number(summary.recovered || 0) > 0 ? 'warning' : 'success'}
            sparkValues={buildUserRequestSparkValues(trends, 'recovered')}
          />
          <OperationKpiCard
            icon={Timer}
            label={t('平均首包')}
            value={formatLatency(summary.avg_ttft_ms)}
            detail={t('首包延迟参考')}
            tone={getThresholdTone(
              summary.avg_ttft_ms,
              LATENCY_THRESHOLDS.ttftMs,
            )}
            sparkValues={buildUserRequestSparkValues(trends, 'avg_ttft_ms')}
          />
          <OperationKpiCard
            icon={Gauge}
            label={t('P95 首包')}
            value={formatLatency(summary.p95_ttft_ms)}
            detail={t('首包延迟参考')}
            tone={getThresholdTone(
              summary.p95_ttft_ms,
              LATENCY_THRESHOLDS.p95TtftMs,
            )}
            sparkValues={buildUserRequestSparkValues(trends, 'p95_ttft_ms')}
          />
          <OperationKpiCard
            icon={Activity}
            label={t('最终失败')}
            value={formatNumber(summary.final_failures)}
            detail={t('隐藏中间调度错误')}
            tone={
              Number(summary.final_failures || 0) > 0 ? 'danger' : 'success'
            }
            sparkValues={buildUserRequestSparkValues(trends, 'final_failures')}
          />
        </div>
      </div>

      <UserRequestTrendPanel trends={trends} t={t} />

      <div className='ct-model-gateway-user-rank-grid'>
        <UserRequestRankPanel
          title={t('模型用户体验排行')}
          icon={Bot}
          rows={userRequests.by_model}
          type='model'
          t={t}
        />
        <UserRequestRankPanel
          title={t('分组用户体验排行')}
          icon={Layers3}
          rows={userRequests.by_group}
          type='group'
          t={t}
        />
      </div>

      <UserRequestRecentTable
        records={userRequests.recent_requests || []}
        t={t}
        refreshing={refreshing}
        onRefresh={onRefresh}
        onOpenDispatchDetail={onOpenDispatchDetail}
        onOpenScoreHistory={onOpenScoreHistory}
        dispatchDetailLoading={dispatchDetailLoading}
        pageSize={userRequestPageSize}
        onPageSizeChange={onUserRequestPageSizeChange}
      />
    </div>
  );
}

function buildOperationalIncidents(data, runtimeStatus, t) {
  const summary = data?.summary || {};
  const runtimeSummary = runtimeStatus?.summary || {};
  const incidents = [];
  const attempts = Number(summary.attempts || 0);
  const successRate = Number(summary.success_rate || 0);
  const failures = Number(summary.failures || 0);
  const streamInterrupted = Number(summary.stream_interrupted || 0);
  const queued = Number(summary.queued_dispatches || 0);
  const avgQueueWaitMs = Number(summary.avg_queue_wait_ms || 0);
  const avgDurationMs = Number(summary.avg_duration_ms || 0);
  const circuitOpen = Number(runtimeSummary.circuit_open || 0);
  const cooldownChannels = Number(runtimeSummary.cooldown_channels || 0);
  const queuedRequests = Number(runtimeSummary.queued_requests || 0);

  if (circuitOpen > 0 || cooldownChannels > 0) {
    incidents.push({
      key: 'circuit',
      type: t('渠道熔断'),
      target: circuitOpen > 0 ? t('渠道') : t('冷却'),
      impact: t('可能导致同模型候选渠道减少'),
      startedAt: '--',
      duration: `${formatNumber(circuitOpen)} / ${formatNumber(cooldownChannels)}`,
      status: circuitOpen > 0 ? t('熔断中') : t('冷却中'),
      metric: `${formatNumber(circuitOpen + cooldownChannels)}`,
      tone: circuitOpen > 0 ? 'danger' : 'warning',
      action: t('查看运行态'),
    });
  }
  if (queued > 0 || queuedRequests > 0 || avgQueueWaitMs > 0) {
    incidents.push({
      key: 'queue',
      type: t('队列积压'),
      target: `${formatNumber(queued || queuedRequests)} ${t('请求')}`,
      impact: `${t('平均等待')} ${formatLatency(avgQueueWaitMs)}`,
      startedAt: '--',
      duration: formatLatency(avgQueueWaitMs),
      status: t('告警中'),
      metric: formatLatency(avgQueueWaitMs),
      tone: avgQueueWaitMs >= 500 ? 'warning' : 'default',
      action: t('查看队列'),
    });
  }
  if (streamInterrupted > 0) {
    incidents.push({
      key: 'stream',
      type: t('流式中断'),
      target: `${formatNumber(streamInterrupted)} ${t('次')}`,
      impact:
        attempts > 0
          ? `${formatPercent(streamInterrupted / attempts)} ${t('占比')}`
          : t('需检查上游稳定性'),
      startedAt: '--',
      duration:
        attempts > 0 ? formatPercent(streamInterrupted / attempts) : '--',
      status: t('异常中'),
      metric: formatNumber(streamInterrupted),
      tone: 'danger',
      action: t('导出 Replay'),
    });
  }
  if (attempts > 0 && successRate < 0.98) {
    incidents.push({
      key: 'success',
      type: t('成功率波动'),
      target: formatPercent(successRate),
      impact: `${formatNumber(failures)} ${t('失败')}`,
      startedAt: '--',
      duration: formatPercent(successRate),
      status: successRate < 0.9 ? t('异常中') : t('告警中'),
      metric: formatPercent(successRate),
      tone: successRate < 0.9 ? 'danger' : 'warning',
      action: t('筛选异常'),
    });
  }
  if (isLatencyWarning(avgDurationMs, LATENCY_THRESHOLDS.avgDurationMs)) {
    incidents.push({
      key: 'latency',
      type: t('响应变慢'),
      target: formatLatency(avgDurationMs),
      impact: t('用户响应速度下降'),
      startedAt: '--',
      duration: formatLatency(avgDurationMs),
      status: t('告警中'),
      metric: formatLatency(avgDurationMs),
      tone: getThresholdTone(avgDurationMs, LATENCY_THRESHOLDS.avgDurationMs),
      action: t('查看慢渠道'),
    });
  }

  if (!incidents.length) {
    incidents.push({
      key: 'healthy',
      type: t('暂无高风险异常'),
      target: t('运行稳定'),
      impact: t('当前窗口未发现需要立即处理的异常'),
      startedAt: '--',
      duration: '--',
      status: t('健康'),
      metric: t('健康'),
      tone: 'success',
      action: t('继续观察'),
    });
  }

  return incidents.slice(0, 5);
}

function IncidentWorkbench({ incidents, t, onReplayBatch }) {
  const incidentBadges = incidents.filter(
    (incident) => incident.key !== 'healthy',
  );

  return (
    <DashboardCard
      title={
        <div className='ct-model-gateway-panel-title-row'>
          <div className='ct-model-gateway-panel-title-group'>
            <span className='ct-model-gateway-panel-title'>
              <ListTree size={17} />
              {t('异常工作台')}
            </span>
            {incidentBadges.length > 0 && (
              <div className='ct-model-gateway-incident-badges'>
                {incidentBadges.slice(0, 4).map((incident) => (
                  <span
                    className={`ct-model-gateway-incident-badge ct-model-gateway-incident-badge-${incident.tone}`}
                    key={`badge-${incident.key}`}
                  >
                    <strong>{incident.metric}</strong>
                    {incident.type}
                  </span>
                ))}
              </div>
            )}
          </div>
          <Button
            size='small'
            type='tertiary'
            icon={<Eye size={14} />}
            onClick={onReplayBatch}
          >
            {t('进入异常分析')}
          </Button>
        </div>
      }
      bodyClassName='ct-model-gateway-incident-body'
    >
      <div className='ct-model-gateway-incident-table'>
        <div className='ct-model-gateway-incident-header'>
          <span>{t('类型')}</span>
          <span>{t('对象')}</span>
          <span>{t('影响')}</span>
          <span>{t('开始时间')}</span>
          <span>{t('持续')}</span>
          <span>{t('状态')}</span>
          <span>{t('操作')}</span>
        </div>
        {incidents.map((incident) => (
          <div
            key={incident.key}
            className={`ct-model-gateway-incident-row ct-model-gateway-incident-row-${incident.tone}`}
          >
            <span className='ct-model-gateway-incident-type'>
              <Tag
                color={
                  incident.tone === 'danger'
                    ? 'red'
                    : incident.tone === 'warning'
                      ? 'orange'
                      : incident.tone === 'success'
                        ? 'green'
                        : 'blue'
                }
                shape='circle'
                type='light'
              >
                {incident.type}
              </Tag>
            </span>
            <span className='ct-model-gateway-ellipsis-cell'>
              {incident.target}
            </span>
            <div className='ct-model-gateway-incident-impact'>
              <span>{incident.impact}</span>
              <MiniSparkline
                values={buildIncidentSparkValues(incident)}
                tone={incident.tone}
                variant='inline'
              />
            </div>
            <span>{incident.startedAt}</span>
            <span>{incident.duration}</span>
            <Tag
              color={
                incident.tone === 'danger'
                  ? 'red'
                  : incident.tone === 'warning'
                    ? 'orange'
                    : 'green'
              }
              size='small'
              type='light'
            >
              {incident.status}
            </Tag>
            <div className='ct-model-gateway-incident-action'>
              <Button
                size='small'
                type='tertiary'
                aria-label={t('查看详情')}
                icon={<Eye size={13} />}
                onClick={onReplayBatch}
              />
              <Button
                size='small'
                type='tertiary'
                aria-label={t('处理建议')}
                icon={<Wrench size={13} />}
                onClick={onReplayBatch}
              />
            </div>
          </div>
        ))}
      </div>
    </DashboardCard>
  );
}

function sortOperationalRows(items) {
  return [...(items || [])]
    .filter((item) => Number(item?.attempts || 0) > 0)
    .sort((a, b) => {
      const aRisk =
        (1 - Number(a.success_rate || 0)) * 10000 +
        Number(a.failures || 0) * 20 +
        Number(a.stream_interrupted || 0) * 25 +
        Number(a.avg_duration_ms || 0) / 100;
      const bRisk =
        (1 - Number(b.success_rate || 0)) * 10000 +
        Number(b.failures || 0) * 20 +
        Number(b.stream_interrupted || 0) * 25 +
        Number(b.avg_duration_ms || 0) / 100;
      return (
        bRisk - aRisk || Number(b.dispatches || 0) - Number(a.dispatches || 0)
      );
    });
}

function PerformanceLeaderboard({ title, icon: Icon, rows, type, t }) {
  const items = sortOperationalRows(rows).slice(0, 5);

  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <Icon size={17} />
          {title}
        </span>
      }
      bodyClassName='ct-model-gateway-leaderboard-body'
    >
      <div className='ct-model-gateway-ops-table'>
        <div className='ct-model-gateway-leaderboard-head'>
          <span>{type === 'channel' ? t('渠道') : t('模型')}</span>
          <span>{t('成功率')}</span>
          <span>{t('平均响应')}</span>
          <span>{t('首包延迟')}</span>
          <span>{type === 'channel' ? t('熔断状态') : t('流中断')}</span>
          <span>QPS</span>
        </div>
        {items.length ? (
          items.map((item) => {
            const successTone = getSuccessTone(
              item.success_rate,
              item.attempts,
            );
            const label =
              item.name ||
              item.key ||
              (item.channel_id ? `#${item.channel_id}` : t('未知'));
            const successPercent = Math.round(
              clampRate(item.success_rate) * 100,
            );
            const streamRate =
              Number(item.attempts || 0) > 0
                ? Number(item.stream_interrupted || 0) /
                  Number(item.attempts || 0)
                : 0;
            const statusTone =
              successTone === 'danger'
                ? 'danger'
                : Number(item.failures || 0) > 0 ||
                    Number(item.stream_interrupted || 0) > 0
                  ? 'warning'
                  : 'success';
            return (
              <div
                className='ct-model-gateway-leaderboard-row'
                key={`${type}-${label}-${item.channel_id || ''}`}
              >
                <div className='ct-model-gateway-leaderboard-name'>
                  <Avatar
                    size='extra-small'
                    color={type === 'channel' ? 'cyan' : 'blue'}
                  >
                    {type === 'channel' ? (
                      <RadioTower size={13} />
                    ) : (
                      <Bot size={13} />
                    )}
                  </Avatar>
                  <div>
                    <Typography.Text strong ellipsis={{ showTooltip: true }}>
                      {label}
                    </Typography.Text>
                  </div>
                </div>
                <div className='ct-model-gateway-leaderboard-metric'>
                  <span
                    className={`ct-model-gateway-leaderboard-rate ct-model-gateway-leaderboard-rate-${successTone}`}
                  >
                    {formatAttemptRate(item.success_rate, item.attempts)}
                  </span>
                  <div className='ct-model-gateway-leaderboard-meter ct-model-gateway-leaderboard-meter-rate'>
                    <span style={{ width: `${successPercent}%` }} />
                  </div>
                </div>
                <div className='ct-model-gateway-leaderboard-metric'>
                  <span>{formatLatency(item.avg_duration_ms)}</span>
                  <MiniSparkline
                    values={[
                      Number(item.avg_duration_ms || 0) * 0.92,
                      Number(item.avg_duration_ms || 0) * 1.04,
                      Number(item.avg_duration_ms || 0) * 0.98,
                      Number(item.avg_duration_ms || 0) * 1.08,
                      Number(item.avg_duration_ms || 0),
                    ]}
                    tone={getThresholdTone(
                      item.avg_duration_ms,
                      LATENCY_THRESHOLDS.avgDurationMs,
                    )}
                    variant='inline'
                  />
                </div>
                <div className='ct-model-gateway-leaderboard-metric'>
                  <span>{formatLatency(item.avg_ttft_ms)}</span>
                  <MiniSparkline
                    values={[
                      Number(item.avg_ttft_ms || 0) * 0.9,
                      Number(item.avg_ttft_ms || 0) * 1.06,
                      Number(item.avg_ttft_ms || 0) * 0.96,
                      Number(item.avg_ttft_ms || 0) * 1.12,
                      Number(item.avg_ttft_ms || 0),
                    ]}
                    tone={getThresholdTone(
                      item.avg_ttft_ms,
                      LATENCY_THRESHOLDS.ttftMs,
                    )}
                    variant='inline'
                  />
                </div>
                {type === 'channel' ? (
                  <Tag
                    color={
                      statusTone === 'danger'
                        ? 'red'
                        : statusTone === 'warning'
                          ? 'orange'
                          : 'green'
                    }
                    size='small'
                    type='light'
                  >
                    {statusTone === 'danger'
                      ? t('异常')
                      : statusTone === 'warning'
                        ? t('告警')
                        : t('正常')}
                  </Tag>
                ) : (
                  <span
                    className={`ct-model-gateway-leaderboard-stream ct-model-gateway-leaderboard-stream-${statusTone}`}
                  >
                    {formatPercent(streamRate)}
                  </span>
                )}
                <span className='ct-model-gateway-leaderboard-qps'>
                  {formatNumber(item.dispatches)}
                </span>
              </div>
            );
          })
        ) : (
          <Typography.Text type='secondary' size='small'>
            {t('暂无排行数据')}
          </Typography.Text>
        )}
      </div>
    </DashboardCard>
  );
}

function DiagnosisMiniTable({
  title,
  count,
  countTone = 'warning',
  columns,
  rows,
  footer,
  onFooterClick,
  t,
}) {
  return (
    <div className='ct-model-gateway-diagnosis-panel'>
      <div className='ct-model-gateway-diagnosis-panel-head'>
        <strong>{title}</strong>
        <Tag
          color={
            countTone === 'danger'
              ? 'red'
              : countTone === 'success'
                ? 'green'
                : 'orange'
          }
          size='small'
          type='light'
        >
          {count}
        </Tag>
      </div>
      <div className='ct-model-gateway-diagnosis-table'>
        <div className='ct-model-gateway-diagnosis-table-head'>
          {columns.map((column) => (
            <span key={column}>{column}</span>
          ))}
        </div>
        {rows.length ? (
          rows.map((row) => (
            <div className='ct-model-gateway-diagnosis-table-row' key={row.key}>
              {row.cells.map((cell, index) => (
                <span key={`${row.key}-${index}`}>{cell}</span>
              ))}
            </div>
          ))
        ) : (
          <div className='ct-model-gateway-diagnosis-empty'>
            {t('暂无数据')}
          </div>
        )}
      </div>
      {footer && (
        <button
          type='button'
          onClick={onFooterClick}
          aria-label={`${title} ${footer}`}
        >
          {footer}
        </button>
      )}
    </div>
  );
}

function EngineeringDiagnosisRail({
  summary,
  runtimeStatus,
  health,
  t,
  onReplayBatch,
}) {
  const runtimeSummary = runtimeStatus?.summary || {};
  const queueDepth = Number(
    runtimeSummary.queued_requests || summary?.queued_dispatches || 0,
  );
  const stickyRoutes = Number(summary?.sticky_routes || 0);
  const stickyRetained = Number(summary?.sticky_retained || 0);
  const stickyRate = stickyRoutes > 0 ? stickyRetained / stickyRoutes : null;

  const circuitRows = (runtimeStatus?.items || [])
    .filter((item) => item.circuit_open || item.circuit_state === 'half_open')
    .slice(0, 2)
    .map((item, index) => ({
      key: `circuit-${index}`,
      cells: [
        item.channel_name || `#${item.channel_id || '--'}`,
        item.circuit_open ? t('熔断中') : t('半开探测'),
        item.circuit_open_until
          ? formatTimestamp(item.circuit_open_until).slice(11, 16)
          : '--',
      ],
    }));
  const queueRows =
    queueDepth > 0
      ? [
          {
            key: 'queue-summary',
            cells: [
              t('运行队列'),
              formatNumber(queueDepth),
              queueDepth > 0 ? t('告警') : t('正常'),
            ],
          },
        ]
      : [];
  const streamRows =
    Number(summary?.stream_interrupted || 0) > 0
      ? [
          {
            key: 'stream-summary',
            cells: [
              t('上游流式'),
              formatPercent(
                Number(summary.stream_interrupted || 0) /
                  Math.max(1, Number(summary.attempts || 0)),
              ),
              t('异常'),
            ],
          },
        ]
      : [];
  const slowModels = sortOperationalRows(
    runtimeStatus?.items?.length ? [] : [],
  ).slice(0, 0);

  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <ServerCog size={17} />
          {t('工程诊断摘要')}
        </span>
      }
      bodyClassName='ct-model-gateway-diagnosis-rail'
    >
      <DiagnosisMiniTable
        title={t('渠道熔断')}
        count={`${formatNumber(runtimeSummary.circuit_open)} ${t('熔断中')}`}
        countTone={
          Number(runtimeSummary.circuit_open || 0) > 0 ? 'danger' : 'success'
        }
        columns={[t('渠道'), t('状态'), t('剩余时间')]}
        rows={circuitRows}
        footer={t('查看全部')}
        onFooterClick={onReplayBatch}
        t={t}
      />
      <DiagnosisMiniTable
        title={t('队列积压')}
        count={`${formatNumber(queueDepth)} ${t('告警')}`}
        countTone={queueDepth > 0 ? 'warning' : 'success'}
        columns={[t('队列'), t('深度'), t('状态')]}
        rows={queueRows}
        footer={t('查看全部')}
        onFooterClick={onReplayBatch}
        t={t}
      />
      <DiagnosisMiniTable
        title={t('流式中断')}
        count={`${formatNumber(summary?.stream_interrupted)} ${t('异常')}`}
        countTone={
          Number(summary?.stream_interrupted || 0) > 0 ? 'danger' : 'success'
        }
        columns={[t('渠道'), t('中断率'), t('状态')]}
        rows={streamRows}
        footer={t('查看全部')}
        onFooterClick={onReplayBatch}
        t={t}
      />
      <DiagnosisMiniTable
        title={t('响应变慢')}
        count={`${formatLatency(summary?.avg_duration_ms)} ${t('平均响应')}`}
        countTone={getThresholdTone(
          summary?.avg_duration_ms,
          LATENCY_THRESHOLDS.avgDurationMs,
        )}
        columns={[t('模型'), t('P95 响应'), t('状态')]}
        rows={slowModels}
        footer={t('查看全部')}
        onFooterClick={onReplayBatch}
        t={t}
      />
      <div className='ct-model-gateway-diagnosis-foot'>
        <div>
          <span>{t('粘滞路由命中')}</span>
          <strong>
            {stickyRate === null ? '--' : formatPercent(stickyRate)}
          </strong>
          <small>{t('较昨日')} +0pp</small>
        </div>
        <div>
          <span>{t('队列深度')}</span>
          <strong>{formatNumber(queueDepth)}</strong>
          <small>
            {t('影响范围')} {formatNumber(health.affectedModels)} /{' '}
            {formatNumber(health.affectedChannels)}
          </small>
        </div>
      </div>
    </DashboardCard>
  );
}

function ViewModeSwitch({ value, onChange, t }) {
  const options = [
    {
      key: VIEW_MODES.USER_REQUESTS,
      icon: CheckCircle2,
      label: t('客户端请求视图'),
    },
    {
      key: VIEW_MODES.OPERATIONS,
      icon: Gauge,
      label: t('运营视图'),
    },
    {
      key: VIEW_MODES.ENGINEERING,
      icon: ServerCog,
      label: t('工程视图'),
    },
  ];

  return (
    <div className='ct-model-gateway-view-switch' role='tablist'>
      {options.map((item) => {
        const Icon = item.icon;
        const active = value === item.key;
        return (
          <button
            key={item.key}
            type='button'
            role='tab'
            aria-selected={active}
            className={active ? 'is-active' : ''}
            onClick={() => onChange(item.key)}
          >
            <Icon size={15} />
            <span>{item.label}</span>
          </button>
        );
      })}
    </div>
  );
}

function EngineeringSummaryDeck({
  data,
  runtimeStatus,
  t,
  onReplayBatch,
  onRefreshSticky,
}) {
  const summary = data?.summary || {};
  const runtimeSummary = runtimeStatus?.summary || {};
  const queue = buildQueuePanelData(data, runtimeStatus);
  const circuitOpen = Number(runtimeSummary.circuit_open || 0);
  const cooldownChannels = Number(runtimeSummary.cooldown_channels || 0);
  const queuedRequests = Number(
    runtimeSummary.queued_requests || queue.totalQueued || 0,
  );
  const stickyRoutes = Number(summary.sticky_routes || 0);
  const stickyRetained = Number(summary.sticky_retained || 0);
  const stickyBroken = Number(summary.sticky_broken || 0);
  const stickyRate = stickyRoutes > 0 ? stickyRetained / stickyRoutes : null;
  const overloadSkipCount = Number(summary.overload_skip_count || 0);
  const authConfigErrorCount = Number(summary.auth_config_error_count || 0);
  const unknownErrorCount = Number(summary.unknown_error_count || 0);
  const configErrorIsolatedCount = Number(
    summary.config_error_isolated_count || 0,
  );
  const queueWaitCount = Number(summary.queue_wait_count || 0);
  const trends = data?.trends || [];
  const latestTrend = latestTrendWithRecords(trends);
  const runtimeUpdatedAt =
    normalizeTimestamp(
      runtimeSummary.updated_at || runtimeStatus?.updated_at,
    ) || null;

  const cards = [
    {
      icon: RadioTower,
      label: t('熔断 / 冷却'),
      value: `${formatNumber(circuitOpen)} / ${formatNumber(cooldownChannels)}`,
      detail: `${formatNumber(runtimeSummary.circuit_half_open)} ${t('半开探测')}`,
      tone:
        circuitOpen > 0
          ? 'danger'
          : cooldownChannels > 0
            ? 'warning'
            : 'success',
    },
    {
      icon: GitBranch,
      label: t('队列深度'),
      value: formatQueuePair(queue.depth, queue.capacity),
      detail: `${formatNumber(queuedRequests)} ${t('等待中')} · ${formatNumber(queue.runtimeKeys)} ${t('运行键')}`,
      tone: queuedRequests > 0 ? 'warning' : 'success',
    },
    {
      icon: Activity,
      label: t('并发 / 运行态'),
      value: formatQueuePair(queue.activeConcurrency, queue.maxConcurrency),
      detail: `${formatNumber(runtimeSummary.runtime_keys)} ${t('运行键')} · ${formatNumber(runtimeSummary.channels)} ${t('渠道')}`,
      tone:
        Number(
          runtimeSummary.high_pressure_channels ||
            runtimeSummary.saturated_channels ||
            0,
        ) > 0
          ? 'warning'
          : 'default',
    },
    {
      icon: ListTree,
      label: t('粘滞 / 缓存亲和'),
      value: stickyRate === null ? '--' : formatPercent(stickyRate),
      detail: `${formatNumber(stickyBroken)} ${t('粘滞断开')} · ${formatNumber(summary.cache_affinity_routes)} ${t('缓存亲和')}`,
      tone: stickyBroken > 0 ? 'warning' : 'success',
    },
    {
      icon: Clock3,
      label: t('Runtime 更新时间'),
      value: runtimeUpdatedAt
        ? formatTimestamp(runtimeUpdatedAt).slice(11)
        : '--',
      detail: runtimeUpdatedAt
        ? formatTimestamp(runtimeUpdatedAt)
        : t('暂无运行态状态数据'),
      tone: runtimeUpdatedAt ? 'success' : 'default',
    },
    {
      icon: RotateCcw,
      label: t('Replay 样本'),
      value: latestTrend
        ? formatNumber(latestTrend.records)
        : formatNumber(summary.total_records),
      detail: t('按当前筛选导出排障样本'),
      tone: 'default',
      action: onReplayBatch,
    },
    {
      icon: RadioTower,
      label: t('过载跳过'),
      value: formatNumber(overloadSkipCount),
      detail: t('仅当前请求换候选，不降权'),
      tone: overloadSkipCount > 0 ? 'warning' : 'success',
    },
    {
      icon: Ban,
      label: t('配置隔离'),
      value: `${formatNumber(configErrorIsolatedCount)} / ${formatNumber(
        authConfigErrorCount,
      )}`,
      detail: t('隔离路由 / 权限配置错误'),
      tone: configErrorIsolatedCount > 0 ? 'danger' : 'success',
    },
    {
      icon: Info,
      label: t('未知失败'),
      value: formatNumber(unknownErrorCount),
      detail: t('需补充分类型的失败样本'),
      tone: unknownErrorCount > 0 ? 'warning' : 'success',
    },
    {
      icon: Timer,
      label: t('队列等待样本'),
      value: formatNumber(queueWaitCount),
      detail: `${formatNumber(summary.queued_dispatches)} ${t('已排队')} · ${formatLatency(summary.avg_queue_wait_ms)}`,
      tone: queueWaitCount > 0 ? 'warning' : 'success',
    },
  ];

  return (
    <div className='ct-model-gateway-engineering-summary'>
      {cards.map((card) => {
        const Icon = card.icon;
        return (
          <div
            className={`ct-model-gateway-engineering-tile ct-model-gateway-engineering-tile-${card.tone}`}
            key={card.label}
          >
            <div className='ct-model-gateway-engineering-tile-head'>
              <Avatar
                size='extra-small'
                color={
                  card.tone === 'danger'
                    ? 'red'
                    : card.tone === 'warning'
                      ? 'orange'
                      : card.tone === 'success'
                        ? 'green'
                        : 'blue'
                }
              >
                <Icon size={13} />
              </Avatar>
              <span>{card.label}</span>
            </div>
            <strong>{card.value}</strong>
            <small>{card.detail}</small>
            {card.action && (
              <Button size='small' type='tertiary' onClick={card.action}>
                {t('导出')}
              </Button>
            )}
          </div>
        );
      })}
      <div className='ct-model-gateway-engineering-actions'>
        <Button
          size='small'
          icon={<RefreshCw size={14} />}
          onClick={onRefreshSticky}
        >
          {t('刷新粘滞')}
        </Button>
        <Button
          size='small'
          type='primary'
          icon={<Download size={14} />}
          onClick={onReplayBatch}
        >
          {t('批量导出 Replay')}
        </Button>
      </div>
    </div>
  );
}

function OperationsDashboard({ data, runtimeStatus, t, onReplayBatch }) {
  const summary = data?.summary || {};
  const trends = data?.trends || [];
  const health = getModelGatewayHealth(data, runtimeStatus);
  const incidents = buildOperationalIncidents(data, runtimeStatus, t);
  const durationP95Ms = durationP95FromRecords(data?.recent_records);

  return (
    <div className='ct-model-gateway-ops-layout'>
      <div className='ct-model-gateway-ops-main'>
        <div className='ct-model-gateway-ops-top-grid'>
          <HealthOverviewCard
            health={health}
            summary={summary}
            trends={trends}
            t={t}
          />
          <div className='ct-model-gateway-kpi-grid'>
            <OperationKpiCard
              icon={CheckCircle2}
              label={t('成功率')}
              value={formatAttemptRate(summary.success_rate, summary.attempts)}
              detail={`${formatNumber(summary.successes)} / ${formatNumber(summary.attempts)} ${t('尝试')}`}
              tone={getSuccessTone(summary.success_rate, summary.attempts)}
              sparkValues={buildSparkValues(trends, 'success_rate')}
            />
            <OperationKpiCard
              icon={Timer}
              label={t('平均响应')}
              value={formatLatency(summary.avg_duration_ms)}
              detail={`${formatNumber(summary.dispatches)} ${t('次调度')}`}
              tone={getThresholdTone(
                summary.avg_duration_ms,
                LATENCY_THRESHOLDS.avgDurationMs,
              )}
              sparkValues={buildSparkValues(trends, 'avg_duration_ms')}
            />
            <OperationKpiCard
              icon={Gauge}
              label={t('P95 响应')}
              value={formatLatency(durationP95Ms)}
              detail={`${formatNumber(summary.total_records)} ${t('条记录')}`}
              tone={getThresholdTone(
                durationP95Ms,
                LATENCY_THRESHOLDS.p95DurationMs,
              )}
              sparkValues={buildSparkValues(trends, 'avg_duration_ms')}
            />
            <OperationKpiCard
              icon={Clock3}
              label={t('首包延迟')}
              value={formatLatency(summary.avg_ttft_ms)}
              detail={t('流式首包平均值')}
              tone={getThresholdTone(
                summary.avg_ttft_ms,
                LATENCY_THRESHOLDS.ttftMs,
              )}
              sparkValues={buildSparkValues(trends, 'avg_ttft_ms')}
            />
            <OperationKpiCard
              icon={GitBranch}
              label={t('队列等待')}
              value={formatLatency(summary.avg_queue_wait_ms)}
              detail={`${formatNumber(summary.queued_dispatches)} ${t('已排队')}`}
              tone={
                Number(summary.avg_queue_wait_ms || 0) > 0
                  ? 'warning'
                  : 'success'
              }
              sparkValues={buildSparkValues(trends, 'avg_queue_wait_ms')}
            />
            <OperationKpiCard
              icon={Activity}
              label={t('流中断')}
              value={formatNumber(summary.stream_interrupted)}
              detail={`${formatNumber(summary.failures)} ${t('失败')}`}
              tone={
                Number(summary.stream_interrupted || 0) > 0
                  ? 'danger'
                  : 'success'
              }
              sparkValues={buildSparkValues(trends, 'stream_interrupted')}
            />
          </div>
        </div>

        <IncidentWorkbench
          incidents={incidents}
          t={t}
          onReplayBatch={onReplayBatch}
        />

        <div className='ct-model-gateway-leaderboard-grid'>
          <PerformanceLeaderboard
            title={t('模型表现')}
            icon={Bot}
            rows={data?.by_model}
            type='model'
            t={t}
          />
          <PerformanceLeaderboard
            title={t('渠道表现')}
            icon={RadioTower}
            rows={data?.by_channel}
            type='channel'
            t={t}
          />
        </div>
      </div>

      <EngineeringDiagnosisRail
        summary={summary}
        runtimeStatus={runtimeStatus}
        health={health}
        t={t}
        onReplayBatch={onReplayBatch}
      />
    </div>
  );
}

function OperationalRecentRecords({
  records,
  t,
  onOpenDetail,
  onExportReplay,
}) {
  const items = (records || [])
    .filter((record) => {
      if (isDispatch(record)) return false;
      return (
        !record.success ||
        record.stream_interrupted ||
        isLatencyWarning(record.duration_ms, LATENCY_THRESHOLDS.avgDurationMs)
      );
    })
    .slice(0, 6);

  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <Gauge size={17} />
          {t('最近异常记录')}
        </span>
      }
      bodyClassName='ct-model-gateway-recent-ops-body'
    >
      <div className='ct-model-gateway-recent-ops-head'>
        <span>{t('时间')}</span>
        <span>{t('类型')}</span>
        <span>{t('对象')}</span>
        <span>{t('影响')}</span>
        <span>{t('持续')}</span>
        <span>{t('状态')}</span>
        <span>{t('操作')}</span>
      </div>
      {items.length ? (
        items.map((record) => {
          const status = getStatusMeta(record, t);
          const type = record.stream_interrupted
            ? t('流式中断')
            : record.is_health_probe || record.request_meta?.is_health_probe
              ? t('健康探活')
              : record.success
                ? t('响应变慢')
                : t('失败');
          const target =
            record.actual_channel_name ||
            record.channel_name ||
            record.requested_model ||
            '--';
          return (
            <div
              className='ct-model-gateway-recent-ops-row'
              key={`${record.id}-${record.kind}-${record.request_id}`}
            >
              <span>{formatTimestamp(record.created_at)}</span>
              <Tag
                color={
                  record.is_health_probe || record.request_meta?.is_health_probe
                    ? 'cyan'
                    : record.stream_interrupted || !record.success
                      ? 'red'
                      : 'orange'
                }
                size='small'
                type='light'
              >
                {type}
              </Tag>
              <Typography.Text ellipsis={{ showTooltip: true }}>
                {target}
              </Typography.Text>
              <span>
                {record.stream_interrupted
                  ? t('需检查上游稳定性')
                  : formatLatency(record.duration_ms)}
              </span>
              <span>{formatLatency(record.duration_ms)}</span>
              <Tag color={status.color} size='small' type='light'>
                {status.label}
              </Tag>
              <div className='ct-model-gateway-recent-ops-actions'>
                <Button
                  size='small'
                  type='tertiary'
                  aria-label={t('查看详情')}
                  icon={<Eye size={13} />}
                  onClick={() => onOpenDetail(record)}
                />
                <Button
                  size='small'
                  type='tertiary'
                  aria-label={t('导出 Replay JSON')}
                  icon={<RotateCcw size={13} />}
                  disabled={!record.request_id}
                  onClick={() => onExportReplay(record.request_id)}
                />
              </div>
            </div>
          );
        })
      ) : (
        <Typography.Text type='secondary' size='small'>
          {t('暂无异常记录')}
        </Typography.Text>
      )}
    </DashboardCard>
  );
}

const QUEUE_SNAPSHOT_KEYS = [
  'queue',
  'queue_status',
  'queue_snapshot',
  'runtime_queue',
  'runtimeQueue',
  'runtime_status_queue',
  'runtimeStatusQueue',
];
const QUEUE_SUMMARY_KEYS = [
  'summary',
  'stats',
  'totals',
  'metrics',
  'overview',
];
const CHANNEL_QUEUE_KEYS = [
  'channels',
  'channel_queues',
  'channelQueues',
  'by_channel',
  'byChannel',
  'per_channel',
  'perChannel',
];
const RUNTIME_QUEUE_KEYS = [
  'runtime_keys',
  'runtimeKeys',
  'runtime_key_queues',
  'runtimeKeyQueues',
  'runtime_items',
  'runtimeItems',
  'by_runtime',
  'byRuntime',
  'by_runtime_key',
  'byRuntimeKey',
  'per_runtime',
  'perRuntime',
  'per_runtime_key',
  'perRuntimeKey',
  'items',
];
const QUEUE_NODE_KEYS = [
  'nodes',
  'queue_nodes',
  'queueNodes',
  'node_queues',
  'nodeQueues',
  'by_node',
  'byNode',
  'per_node',
  'perNode',
];
const QUEUE_DEPTH_KEYS = [
  'queue_depth',
  'depth',
  'current_depth',
  'queued',
  'queued_requests',
  'waiting',
  'waiting_requests',
  'pending',
  'pending_requests',
  'size',
  'length',
  'count',
  'value',
];
const QUEUE_CAPACITY_KEYS = [
  'queue_capacity',
  'capacity',
  'total_capacity',
  'max_capacity',
  'max_queue_depth',
  'max_depth',
  'limit',
  'max',
];
const QUEUE_WAITING_KEYS = [
  'waiting',
  'waiting_requests',
  'queue_waiting',
  'pending',
  'pending_requests',
  'queued',
  'queued_requests',
];
const QUEUE_ACTIVE_KEYS = [
  'active_concurrency',
  'active',
  'running',
  'inflight',
  'in_flight',
];
const QUEUE_MAX_CONCURRENCY_KEYS = [
  'max_concurrency',
  'concurrency_capacity',
  'concurrency_limit',
  'max_active',
];
const QUEUE_RUNNING_KEY_KEYS = [
  'running_keys',
  'runningKeys',
  'active_keys',
  'activeKeys',
  'runtime_keys',
  'runtimeKeys',
  'key_count',
  'keys',
];
const QUEUE_REJECT_KEYS = [
  'reject_reasons',
  'rejection_reasons',
  'admission_reject_reasons',
  'rejected_reasons',
  'rejects',
  'rejections',
];
const QUEUE_COOLDOWN_KEYS = [
  'cooldowns',
  'cooldown_hints',
  'cooldownHints',
  'cooldown_channels',
  'cooldownChannels',
  'cooldown',
];

function isPlainObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function getFirstValue(source, keys) {
  if (!source) return undefined;
  for (const key of keys) {
    const value = source?.[key];
    if (value !== undefined && value !== null && value !== '') return value;
  }
  return undefined;
}

function getFirstNumber(source, keys) {
  const value = getFirstValue(source, keys);
  if (value === undefined || value === null || value === '') return null;
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : null;
}

function getFirstText(source, keys) {
  const value = getFirstValue(source, keys);
  if (value === undefined || value === null || value === '') return '';
  if (typeof value === 'object') return formatRuntimeKey(value);
  return String(value);
}

function getFirstObject(source, keys) {
  const value = getFirstValue(source, keys);
  return isPlainObject(value) ? value : null;
}

function getFirstCollection(source, keys) {
  const value = getFirstValue(source, keys);
  if (Array.isArray(value) || isPlainObject(value)) return value;
  return null;
}

function sumNumbers(rows, key) {
  const total = rows.reduce((sum, row) => {
    const value = row?.[key];
    return Number.isFinite(value) ? sum + value : sum;
  }, 0);
  return total > 0 ? total : null;
}

function firstNumberFromSources(sources, keys) {
  for (const source of sources) {
    const value = getFirstNumber(source, keys);
    if (value !== null) return value;
  }
  return null;
}

function firstTimestampFromSources(sources, keys) {
  for (const source of sources) {
    const value = getFirstValue(source, keys);
    const timestamp = normalizeTimestamp(value);
    if (timestamp) return timestamp;
  }
  return null;
}

function pickQueueSnapshot(data, runtimeStatus) {
  const candidates = [
    ...QUEUE_SNAPSHOT_KEYS.map((key) => runtimeStatus?.[key]),
    ...QUEUE_SNAPSHOT_KEYS.map((key) => runtimeStatus?.summary?.[key]),
    ...QUEUE_SNAPSHOT_KEYS.map((key) => data?.runtime_status?.[key]),
    ...QUEUE_SNAPSHOT_KEYS.map((key) => data?.[key]),
  ];
  return candidates.find(
    (candidate) => Array.isArray(candidate) || isPlainObject(candidate),
  );
}

function queueMapLooksLikeRows(value, kind) {
  if (!isPlainObject(value)) return false;
  return Object.entries(value).some(([key, row]) => {
    if (kind === 'channel' && /^\d+$/.test(key)) return true;
    if (!isPlainObject(row)) {
      return (
        Number.isFinite(Number(row)) &&
        (kind === 'channel'
          ? /^\d+$/.test(key)
          : key.includes('/') || key.includes('|') || key.includes(':'))
      );
    }
    const hasRowIdentity =
      kind === 'channel'
        ? /^\d+$/.test(key) ||
          row.channel_id !== undefined ||
          row.channelId !== undefined
        : row.runtime_key !== undefined ||
          row.runtimeKey !== undefined ||
          row.requested_model !== undefined ||
          row.key !== undefined;
    if (!hasRowIdentity) return false;
    return [
      ...QUEUE_DEPTH_KEYS,
      ...QUEUE_CAPACITY_KEYS,
      ...QUEUE_ACTIVE_KEYS,
      'channel_id',
      'channelId',
      'requested_model',
      'runtime_key',
      'runtimeKey',
    ].some((field) => row[field] !== undefined && row[field] !== null);
  });
}

function normalizeQueueNodeCollection(collection) {
  if (!collection) return [];
  const rows = Array.isArray(collection)
    ? collection.map((value, index) => [String(index), value])
    : Object.entries(collection);

  return rows
    .map(([mapKey, value], index) => {
      const record = isPlainObject(value)
        ? { _map_key: mapKey, ...value }
        : { _map_key: mapKey, queue_depth: value };
      const summary = getFirstObject(record, QUEUE_SUMMARY_KEYS) || {};
      const channelRows = normalizeQueueCollection(
        getFirstCollection(record, CHANNEL_QUEUE_KEYS),
        'channel',
      );
      const runtimeRows = normalizeQueueCollection(
        getFirstCollection(record, RUNTIME_QUEUE_KEYS),
        'runtime',
      );
      const nodeID = getFirstText(record, [
        'node_id',
        'nodeId',
        'id',
        'name',
        'hostname',
        'host',
      ]);
      const nodeName = getFirstText(record, [
        'node_name',
        'nodeName',
        'label',
        'display_name',
      ]);
      const region = getFirstText(record, ['region', 'zone', 'az']);
      const updatedAt = firstTimestampFromSources(
        [record],
        [
          'updated_at',
          'updatedAt',
          'last_seen_at',
          'lastSeenAt',
          'last_update',
          'lastUpdate',
          'timestamp',
        ],
      );

      return {
        id: `node-${mapKey}-${index}`,
        label: nodeName || nodeID || mapKey,
        detail: [nodeID && nodeID !== nodeName ? nodeID : '', region]
          .filter(Boolean)
          .join(' · '),
        depth:
          firstNumberFromSources(
            [summary, record],
            [
              'total_queued',
              'total_depth',
              'queued_requests',
              ...QUEUE_DEPTH_KEYS,
            ],
          ) ?? sumNumbers(channelRows, 'depth'),
        capacity:
          firstNumberFromSources(
            [summary, record],
            ['total_capacity', ...QUEUE_CAPACITY_KEYS],
          ) ?? sumNumbers(channelRows, 'capacity'),
        runningKeys:
          firstNumberFromSources([summary, record], QUEUE_RUNNING_KEY_KEYS) ??
          runtimeRows.length,
        updatedAt,
      };
    })
    .filter((row) =>
      [row.depth, row.capacity, row.runningKeys, row.updatedAt].some((value) =>
        Number.isFinite(value),
      ),
    );
}

function normalizeQueueCollection(collection, kind) {
  if (!collection) return [];
  const rows = Array.isArray(collection)
    ? collection.map((value, index) => [String(index), value])
    : Object.entries(collection);

  return rows
    .map(([mapKey, value], index) => {
      const record = isPlainObject(value)
        ? { _map_key: mapKey, ...value }
        : { _map_key: mapKey, queue_depth: value };
      if (kind === 'channel' && !record.channel_id && /^\d+$/.test(mapKey)) {
        record.channel_id = Number(mapKey);
      }
      const channelId = getFirstText(record, [
        'channel_id',
        'channelId',
        'channel',
      ]);
      const channelName = getFirstText(record, [
        'channel_name',
        'channelName',
        'name',
      ]);
      const model = getFirstText(record, [
        'requested_model',
        'model',
        'runtime_model',
      ]);
      const upstreamModel = getFirstText(record, ['upstream_model']);
      const group = getFirstText(record, ['group', 'requested_group']);
      const endpoint = getFirstText(record, ['endpoint_type', 'endpoint']);
      const runtimeKey = getFirstText(record, [
        'runtime_key',
        'runtimeKey',
        'key',
      ]);
      const label =
        channelName ||
        (kind === 'channel' && channelId ? `#${channelId}` : '') ||
        model ||
        runtimeKey ||
        mapKey;
      const detail = [
        channelId && label !== `#${channelId}` ? `#${channelId}` : '',
        group,
        upstreamModel,
        endpoint,
      ]
        .filter(Boolean)
        .join(' · ');

      return {
        id: `${kind}-${mapKey}-${index}`,
        label,
        detail,
        depth: getFirstNumber(record, QUEUE_DEPTH_KEYS),
        capacity: getFirstNumber(record, QUEUE_CAPACITY_KEYS),
        waiting: getFirstNumber(record, QUEUE_WAITING_KEYS),
        active: getFirstNumber(record, QUEUE_ACTIVE_KEYS),
        maxConcurrency: getFirstNumber(record, QUEUE_MAX_CONCURRENCY_KEYS),
        highPriority: getFirstNumber(record, [
          'high_priority_depth',
          'high_priority_queued',
          'priority_depth',
          'priority_queued',
          'high_priority',
        ]),
        normal: getFirstNumber(record, [
          'normal_depth',
          'normal_queued',
          'standard_depth',
          'standard_queued',
          'normal',
        ]),
        rejectReason: getFirstText(record, [
          'reject_reason',
          'rejection_reason',
          'last_reject_reason',
          'reason',
        ]),
        cooldownReason: getFirstText(record, [
          'cooldown_reason',
          'failure_avoidance_reason',
        ]),
        cooldownSeconds: getFirstNumber(record, [
          'cooldown_remaining_seconds',
          'failure_avoidance_remaining_seconds',
          'cooldown_seconds',
          'cooldown_remaining',
        ]),
      };
    })
    .filter((row) =>
      [
        row.depth,
        row.capacity,
        row.waiting,
        row.active,
        row.maxConcurrency,
        row.highPriority,
        row.normal,
      ].some((value) => Number.isFinite(value) && value > 0),
    );
}

function queueCollectionLooksLikeChannelRows(collection) {
  if (!Array.isArray(collection)) return false;
  return collection.some((row) => {
    if (!isPlainObject(row)) return false;
    return [
      'channel_id',
      'channelId',
      'channel',
      'channel_name',
      'channelName',
    ].some((key) => row[key] !== undefined && row[key] !== null);
  });
}

function normalizeReasonRows(collection) {
  if (!collection) return [];
  const rows = Array.isArray(collection)
    ? collection.map((value, index) => [String(index), value])
    : Object.entries(collection);

  return rows
    .map(([key, value], index) => {
      if (isPlainObject(value)) {
        return {
          id: `reason-${key}-${index}`,
          label:
            getFirstText(value, [
              'reason',
              'status',
              'type',
              'key',
              'name',
              'message',
            ]) || key,
          count:
            getFirstNumber(value, [
              'count',
              'rejected',
              'rejects',
              'value',
              'total',
            ]) || 0,
        };
      }
      return {
        id: `reason-${key}-${index}`,
        label: key,
        count: Number.isFinite(Number(value)) ? Number(value) : 0,
      };
    })
    .filter((row) => row.label);
}

function normalizeCooldownRows(collection, runtimeItems = []) {
  const rows = [];
  const sourceRows = collection
    ? Array.isArray(collection)
      ? collection.map((value, index) => [String(index), value])
      : Object.entries(collection)
    : [];

  for (const [key, value] of sourceRows) {
    const record = isPlainObject(value) ? value : { reason: value };
    rows.push({
      id: `cooldown-${key}`,
      label:
        getFirstText(record, ['channel_name', 'name', 'requested_model']) ||
        (record.channel_id ? `#${record.channel_id}` : key),
      reason:
        getFirstText(record, [
          'reason',
          'cooldown_reason',
          'failure_avoidance_reason',
          'status',
        ]) || (isPlainObject(value) ? '' : String(value || '')),
      seconds: getFirstNumber(record, [
        'cooldown_remaining_seconds',
        'failure_avoidance_remaining_seconds',
        'remaining_seconds',
        'seconds',
      ]),
    });
  }

  for (const item of runtimeItems) {
    if (!item?.cooldown && !item?.failure_avoidance) continue;
    rows.push({
      id: `runtime-cooldown-${item.channel_id || 'channel'}-${
        item.requested_model || 'model'
      }-${item.group || 'group'}`,
      label: item.requested_model || `#${item.channel_id || '--'}`,
      reason:
        item.cooldown_reason ||
        item.failure_avoidance_reason ||
        (item.cooldown ? 'cooldown' : 'failure_avoidance'),
      seconds: Number(
        item.cooldown_remaining_seconds ||
          item.failure_avoidance_remaining_seconds ||
          0,
      ),
      detail: [
        item.channel_id ? `#${item.channel_id}` : '',
        item.group,
        item.upstream_model,
      ]
        .filter(Boolean)
        .join(' · '),
    });
  }

  return rows.filter((row) => row.label || row.reason).slice(0, 6);
}

function normalizePriorityBucket(snapshot, summary, name, fieldPrefix) {
  const source =
    getFirstObject(snapshot, [
      name,
      `${name}_queue`,
      `${fieldPrefix}_queue`,
      `${fieldPrefix}Queue`,
    ]) || {};
  const depth =
    getFirstNumber(source, QUEUE_DEPTH_KEYS) ??
    getFirstNumber(summary, [
      `${fieldPrefix}_depth`,
      `${fieldPrefix}_queued`,
      `${fieldPrefix}_requests`,
      `${fieldPrefix}_waiting`,
    ]);
  const capacity =
    getFirstNumber(source, QUEUE_CAPACITY_KEYS) ??
    getFirstNumber(summary, [
      `${fieldPrefix}_capacity`,
      `${fieldPrefix}_queue_capacity`,
      `${fieldPrefix}_max_depth`,
    ]);
  const waiting =
    getFirstNumber(source, QUEUE_WAITING_KEYS) ??
    getFirstNumber(summary, [
      `${fieldPrefix}_waiting`,
      `${fieldPrefix}_waiting_requests`,
    ]);

  if (![depth, capacity, waiting].some((value) => value !== null)) return null;
  return { depth, capacity, waiting };
}

function buildQueuePanelData(data, runtimeStatus) {
  const snapshot = pickQueueSnapshot(data, runtimeStatus);
  const directSnapshotRows = Array.isArray(snapshot) ? snapshot : null;
  const directSnapshotIsChannelRows =
    queueCollectionLooksLikeChannelRows(directSnapshotRows);
  const snapshotSummary =
    getFirstObject(snapshot, QUEUE_SUMMARY_KEYS) ||
    (isPlainObject(snapshot) ? snapshot : {});
  const runtimeSummary = runtimeStatus?.summary || {};
  const summarySources = [snapshotSummary, snapshot, runtimeSummary];

  const channelRows = normalizeQueueCollection(
    getFirstCollection(snapshot, CHANNEL_QUEUE_KEYS) ||
      (directSnapshotIsChannelRows ? directSnapshotRows : null) ||
      (queueMapLooksLikeRows(snapshot, 'channel') ? snapshot : null),
    'channel',
  );
  const runtimeRows = normalizeQueueCollection(
    getFirstCollection(snapshot, RUNTIME_QUEUE_KEYS) ||
      (!directSnapshotIsChannelRows ? directSnapshotRows : null) ||
      runtimeStatus?.items,
    'runtime',
  );
  const nodeRows = normalizeQueueNodeCollection(
    getFirstCollection(snapshot, QUEUE_NODE_KEYS) ||
      getFirstCollection(snapshotSummary, QUEUE_NODE_KEYS),
  )
    .sort((a, b) => {
      const aPressure =
        a.capacity > 0 && a.depth !== null ? a.depth / a.capacity : 0;
      const bPressure =
        b.capacity > 0 && b.depth !== null ? b.depth / b.capacity : 0;
      return (
        bPressure - aPressure ||
        Number(b.depth || 0) - Number(a.depth || 0) ||
        Number(b.runningKeys || 0) - Number(a.runningKeys || 0)
      );
    })
    .slice(0, 6);
  const occupancyRows = (channelRows.length ? channelRows : runtimeRows)
    .sort((a, b) => {
      const aPressure =
        a.capacity > 0 && a.depth !== null ? a.depth / a.capacity : 0;
      const bPressure =
        b.capacity > 0 && b.depth !== null ? b.depth / b.capacity : 0;
      return (
        bPressure - aPressure ||
        Number(b.depth || 0) - Number(a.depth || 0) ||
        Number(b.active || 0) - Number(a.active || 0)
      );
    })
    .slice(0, 6);

  const totalQueued =
    firstNumberFromSources(summarySources, [
      'total_queued',
      'queued_total',
      'queued_requests',
      'queue_depth',
      'depth',
    ]) ?? sumNumbers(occupancyRows, 'depth');
  const waiting =
    firstNumberFromSources(summarySources, [
      'waiting',
      'waiting_requests',
      'queue_waiting',
      'pending_requests',
      'pending',
    ]) ?? totalQueued;
  const depth =
    firstNumberFromSources(summarySources, [
      'total_depth',
      'queue_depth',
      'depth',
      'queued_requests',
    ]) ?? sumNumbers(occupancyRows, 'depth');
  const capacity =
    firstNumberFromSources(summarySources, [
      'total_capacity',
      'queue_capacity',
      'capacity',
      'max_capacity',
    ]) ?? sumNumbers(occupancyRows, 'capacity');
  const activeConcurrency =
    firstNumberFromSources(summarySources, [
      'active_concurrency',
      'active',
      'running',
      'inflight',
    ]) ?? sumNumbers(occupancyRows, 'active');
  const maxConcurrency =
    firstNumberFromSources(summarySources, [
      'max_concurrency',
      'concurrency_capacity',
      'concurrency_limit',
    ]) ?? sumNumbers(occupancyRows, 'maxConcurrency');
  const queueChannels =
    firstNumberFromSources(summarySources, ['queue_channels', 'channels']) ??
    channelRows.length;
  const runtimeKeys =
    firstNumberFromSources(summarySources, ['runtime_keys', 'runtimeKeys']) ??
    runtimeRows.length;
  const queueNodes =
    firstNumberFromSources(summarySources, [
      'queue_nodes',
      'queueNodes',
      'node_count',
      'nodeCount',
    ]) ?? nodeRows.length;
  const nodeUpdatedAt =
    firstTimestampFromSources(
      [snapshotSummary, snapshot],
      [
        'nodes_updated_at',
        'nodesUpdatedAt',
        'node_updated_at',
        'nodeUpdatedAt',
        'updated_at',
        'updatedAt',
        'last_update',
        'lastUpdate',
      ],
    ) ||
    nodeRows.reduce(
      (latest, row) => Math.max(latest, Number(row.updatedAt || 0)),
      0,
    ) ||
    null;

  const highPriority = normalizePriorityBucket(
    snapshot,
    snapshotSummary,
    'high_priority',
    'high_priority',
  );
  const normalPriority = normalizePriorityBucket(
    snapshot,
    snapshotSummary,
    'normal',
    'normal',
  );
  const rejectRows = normalizeReasonRows(
    getFirstCollection(snapshot, QUEUE_REJECT_KEYS) ||
      getFirstCollection(snapshotSummary, QUEUE_REJECT_KEYS),
  ).slice(0, 6);
  const cooldownRows = normalizeCooldownRows(
    getFirstCollection(snapshot, QUEUE_COOLDOWN_KEYS) ||
      getFirstCollection(snapshotSummary, QUEUE_COOLDOWN_KEYS),
    runtimeStatus?.items || [],
  );

  return {
    source: snapshot ? 'snapshot' : 'runtime',
    totalQueued,
    waiting,
    depth,
    capacity,
    activeConcurrency,
    maxConcurrency,
    queueChannels,
    runtimeKeys,
    queueNodes,
    nodeRows,
    nodeUpdatedAt,
    occupancyKind: channelRows.length ? 'channel' : 'runtime',
    occupancyRows,
    priorityRows: [
      highPriority
        ? { key: 'high', labelKey: '高优先级队列', ...highPriority }
        : null,
      normalPriority
        ? { key: 'normal', labelKey: '普通队列', ...normalPriority }
        : null,
    ].filter(Boolean),
    rejectRows,
    cooldownRows,
    updatedAt: firstTimestampFromSources(
      [runtimeSummary, snapshotSummary],
      ['updated_at', 'updatedAt', 'last_update', 'lastUpdate'],
    ),
  };
}

function formatNumberOrDash(value) {
  return Number.isFinite(value) ? formatNumber(value) : '--';
}

function formatQueuePair(value, capacity) {
  const left = formatNumberOrDash(value);
  const right = formatNumberOrDash(capacity);
  return right === '--' ? left : `${left} / ${right}`;
}

function queuePressureTone(depth, capacity, fallback = 0) {
  const pressure = capacity > 0 && depth !== null ? depth / capacity : fallback;
  if (pressure >= 0.9) return 'danger';
  if (pressure >= 0.65) return 'warning';
  return 'success';
}

function QueueOccupancyRow({ row, t }) {
  const queuePressure =
    row.capacity > 0 && row.depth !== null ? row.depth / row.capacity : 0;
  const concurrencyPressure =
    row.maxConcurrency > 0 && row.active !== null
      ? row.active / row.maxConcurrency
      : 0;
  const pressure = Math.max(queuePressure, concurrencyPressure);
  const width = `${Math.round(Math.min(1, Math.max(0.06, pressure)) * 100)}%`;

  return (
    <div className='ct-model-gateway-queue-runtime-row'>
      <div className='ct-model-gateway-runtime-name'>
        <Typography.Text strong ellipsis={{ showTooltip: true }}>
          {row.label || t('未知')}
        </Typography.Text>
        <Typography.Text type='secondary' size='small'>
          {row.detail || t('运行键')}
        </Typography.Text>
      </div>
      <div className='ct-model-gateway-queue-runtime-row-main'>
        <div className='ct-model-gateway-queue-runtime-meter'>
          <span style={{ width }} />
        </div>
        <div className='ct-model-gateway-record-tags'>
          {(row.depth !== null || row.capacity !== null) && (
            <Tag color='cyan' size='small' type='light'>
              {t('队列深度')} {formatQueuePair(row.depth, row.capacity)}
            </Tag>
          )}
          {row.waiting !== null && (
            <Tag color='blue' size='small' type='light'>
              {t('等待中')} {formatNumber(row.waiting)}
            </Tag>
          )}
          {(row.active !== null || row.maxConcurrency !== null) && (
            <Tag color='grey' size='small' type='light'>
              {t('并发')} {formatQueuePair(row.active, row.maxConcurrency)}
            </Tag>
          )}
          {row.highPriority !== null && (
            <Tag color='orange' size='small' type='light'>
              {t('高优先级队列')} {formatNumber(row.highPriority)}
            </Tag>
          )}
          {row.normal !== null && (
            <Tag color='blue' size='small' type='light'>
              {t('普通队列')} {formatNumber(row.normal)}
            </Tag>
          )}
          {row.rejectReason && (
            <Tag color='red' size='small' type='light'>
              {row.rejectReason}
            </Tag>
          )}
          {row.cooldownReason && (
            <Tag color='orange' size='small' type='light'>
              {row.cooldownReason}
            </Tag>
          )}
        </div>
      </div>
    </div>
  );
}

function QueueNodeRow({ row, t }) {
  const queuePressure =
    row.capacity > 0 && row.depth !== null ? row.depth / row.capacity : 0;
  const width = `${Math.round(Math.min(1, Math.max(0.06, queuePressure)) * 100)}%`;

  return (
    <div className='ct-model-gateway-queue-runtime-row'>
      <div className='ct-model-gateway-runtime-name'>
        <Typography.Text strong ellipsis={{ showTooltip: true }}>
          {row.label || t('未知节点')}
        </Typography.Text>
        <Typography.Text type='secondary' size='small'>
          {row.detail || t('队列节点')}
        </Typography.Text>
      </div>
      <div className='ct-model-gateway-queue-runtime-row-main'>
        <div className='ct-model-gateway-queue-runtime-meter'>
          <span style={{ width }} />
        </div>
        <div className='ct-model-gateway-record-tags'>
          {(row.depth !== null || row.capacity !== null) && (
            <Tag color='cyan' size='small' type='light'>
              {t('队列深度')} {formatQueuePair(row.depth, row.capacity)}
            </Tag>
          )}
          {row.runningKeys !== null && (
            <Tag color='blue' size='small' type='light'>
              {t('运行键数')} {formatNumber(row.runningKeys)}
            </Tag>
          )}
          {row.updatedAt && (
            <Tag color='grey' size='small' type='light'>
              {t('节点更新时间')} {formatTimestamp(row.updatedAt)}
            </Tag>
          )}
        </div>
      </div>
    </div>
  );
}

function QueueRuntimePressurePanel({ data, runtimeStatus, t }) {
  const queue = useMemo(
    () => buildQueuePanelData(data, runtimeStatus),
    [data, runtimeStatus],
  );
  const capacityPressure =
    queue.capacity > 0 && queue.depth !== null
      ? queue.depth / queue.capacity
      : 0;
  const concurrencyPressure =
    queue.maxConcurrency > 0 && queue.activeConcurrency !== null
      ? queue.activeConcurrency / queue.maxConcurrency
      : 0;
  const pressure = Math.max(capacityPressure, concurrencyPressure);
  const pressureTone = queuePressureTone(queue.depth, queue.capacity, pressure);
  const hasHints = queue.rejectRows.length > 0 || queue.cooldownRows.length > 0;

  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <RadioTower size={17} />
          {t('队列运行态 / 并发压力')}
        </span>
      }
      bodyClassName='ct-model-gateway-queue-runtime-body'
    >
      <div className='ct-model-gateway-runtime-metrics'>
        <RuntimeMetricTile
          label={t('排队 / 等待')}
          value={`${formatNumberOrDash(queue.totalQueued)} / ${formatNumberOrDash(
            queue.waiting,
          )}`}
          detail={
            queue.occupancyKind === 'channel'
              ? `${formatNumber(queue.queueChannels)} ${t('队列渠道')}`
              : `${formatNumber(queue.runtimeKeys)} ${t('运行键')}`
          }
          tone={Number(queue.totalQueued) > 0 ? 'warning' : 'success'}
        />
        <RuntimeMetricTile
          label={t('队列容量')}
          value={formatQueuePair(queue.depth, queue.capacity)}
          detail={`${t('容量占用')} ${formatPercent(capacityPressure)}`}
          tone={pressureTone}
        />
        <RuntimeMetricTile
          label={t('并发压力')}
          value={formatQueuePair(queue.activeConcurrency, queue.maxConcurrency)}
          detail={`${t('容量占用')} ${formatPercent(concurrencyPressure)}`}
          tone={queuePressureTone(
            queue.activeConcurrency,
            queue.maxConcurrency,
            concurrencyPressure,
          )}
        />
        <RuntimeMetricTile
          label={t('队列节点')}
          value={formatNumberOrDash(queue.queueNodes)}
          detail={
            queue.nodeUpdatedAt
              ? `${t('节点更新时间')} ${formatTimestamp(queue.nodeUpdatedAt)}`
              : t('节点上报数量')
          }
          tone={queue.queueNodes > 0 ? 'success' : 'default'}
        />
        <RuntimeMetricTile
          label={t('拒绝与冷却')}
          value={formatNumber(
            queue.rejectRows.length + queue.cooldownRows.length,
          )}
          detail={queue.source === 'snapshot' ? t('队列快照') : t('运行态状态')}
          tone={hasHints ? 'warning' : 'success'}
        />
      </div>

      {queue.priorityRows.length > 0 && (
        <div className='ct-model-gateway-queue-priority-grid'>
          {queue.priorityRows.map((item) => (
            <div className='ct-model-gateway-queue-priority' key={item.key}>
              <span>{t(item.labelKey)}</span>
              <strong>{formatQueuePair(item.depth, item.capacity)}</strong>
              <small>
                {t('等待中')} {formatNumberOrDash(item.waiting)}
              </small>
            </div>
          ))}
        </div>
      )}

      <div className='ct-model-gateway-queue-runtime-layout'>
        <div className='ct-model-gateway-queue-section'>
          <div className='ct-model-gateway-queue-section-title'>
            <Typography.Text strong>{t('节点 Top')}</Typography.Text>
            <Tag color='cyan' size='small' type='light'>
              {formatNumberOrDash(queue.queueNodes)} {t('队列节点')}
            </Tag>
          </div>
          {queue.nodeRows.length ? (
            <div className='ct-model-gateway-queue-runtime-list'>
              {queue.nodeRows.map((row) => (
                <QueueNodeRow key={row.id} row={row} t={t} />
              ))}
            </div>
          ) : (
            <Typography.Text type='secondary' size='small'>
              {t('暂无队列节点')}
            </Typography.Text>
          )}
        </div>

        <div className='ct-model-gateway-queue-section'>
          <div className='ct-model-gateway-queue-section-title'>
            <Typography.Text strong>{t('占用明细')}</Typography.Text>
            <Tag color='cyan' size='small' type='light'>
              {queue.occupancyKind === 'channel'
                ? t('渠道占用')
                : t('运行键占用')}
            </Tag>
          </div>
          {queue.occupancyRows.length ? (
            <div className='ct-model-gateway-queue-runtime-list'>
              {queue.occupancyRows.map((row) => (
                <QueueOccupancyRow key={row.id} row={row} t={t} />
              ))}
            </div>
          ) : (
            <Typography.Text type='secondary' size='small'>
              {t('暂无队列占用')}
            </Typography.Text>
          )}
        </div>

        <div className='ct-model-gateway-queue-section'>
          <div className='ct-model-gateway-queue-section-title'>
            <Typography.Text strong>{t('冷却提示')}</Typography.Text>
            <Tag
              color={hasHints ? 'orange' : 'green'}
              size='small'
              type='light'
            >
              {hasHints ? t('需关注渠道') : t('健康')}
            </Tag>
          </div>
          {hasHints ? (
            <div className='ct-model-gateway-queue-hints'>
              {queue.rejectRows.map((item) => (
                <Tag key={item.id} color='red' size='small' type='light'>
                  {t('拒绝原因')}: {item.label}
                  {item.count > 0 ? ` ${formatNumber(item.count)}` : ''}
                </Tag>
              ))}
              {queue.cooldownRows.map((item) => (
                <Tag key={item.id} color='orange' size='small' type='light'>
                  {item.label}
                  {item.reason ? ` · ${item.reason}` : ''}
                  {item.seconds > 0 ? ` · ${formatNumber(item.seconds)}s` : ''}
                </Tag>
              ))}
            </div>
          ) : (
            <Typography.Text type='secondary' size='small'>
              {t('暂无拒绝或冷却提示')}
            </Typography.Text>
          )}
        </div>
      </div>

      {queue.updatedAt > 0 && (
        <Typography.Text type='secondary' size='small'>
          {t('运行态最近更新时间')}: {formatTimestamp(queue.updatedAt)}
        </Typography.Text>
      )}
    </DashboardCard>
  );
}

function getRuntimeRiskWeight(item) {
  if (!item) return 0;
  if (item.health_status === 'circuit_open' || item.circuit_open) return 100;
  if (item.cooldown || item.health_status === 'cooldown') return 80;
  if (item.failure_avoidance || item.health_status === 'failure_avoidance')
    return 70;
  if (item.circuit_state === 'half_open') return 65;
  if (Number(item.queue_depth) > 0 || item.health_status === 'queued')
    return 55;
  if (
    item.health_status === 'high_pressure' ||
    item.health_status === 'saturated'
  )
    return 45;
  if (item.health_status === 'degraded') return 45;
  return 0;
}

function StickyInsightPanel({ summary, t }) {
  const stickyRoutes = Number(summary?.sticky_routes || 0);
  const stickyRetained = Number(summary?.sticky_retained || 0);
  const stickyBroken = Number(summary?.sticky_broken || 0);
  const cacheAffinity = Number(summary?.cache_affinity_routes || 0);
  const queueEnabled = Number(summary?.queue_enabled_dispatches || 0);
  const queued = Number(summary?.queued_dispatches || 0);
  const queuePressure = queueEnabled > 0 ? queued / queueEnabled : 0;
  const retentionRate = stickyRoutes > 0 ? stickyRetained / stickyRoutes : null;
  const stickyTone =
    stickyRoutes <= 0
      ? 'default'
      : retentionRate >= 0.85
        ? 'success'
        : 'warning';
  const queueTone =
    queued <= 0
      ? 'success'
      : queuePressure >= 0.35 || Number(summary?.avg_queue_wait_ms) >= 1000
        ? 'warning'
        : 'default';

  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <GitBranch size={17} />
          {t('粘滞与排队概览')}
        </span>
      }
      bodyClassName='ct-model-gateway-insight-body'
    >
      <div className='ct-model-gateway-runtime-metrics'>
        <RuntimeMetricTile
          label={t('粘滞保留率')}
          value={retentionRate === null ? '--' : formatPercent(retentionRate)}
          detail={`${formatNumber(stickyRetained)} / ${formatNumber(
            stickyRoutes,
          )} ${t('粘滞路由')}`}
          tone={stickyTone}
        />
        <RuntimeMetricTile
          label={t('粘滞打破')}
          value={formatNumber(stickyBroken)}
          detail={`${formatNumber(cacheAffinity)} ${t('缓存亲和')}`}
          tone={stickyBroken > 0 ? 'warning' : 'success'}
        />
        <RuntimeMetricTile
          label={t('队列压力')}
          value={formatPercent(queuePressure)}
          detail={`${formatNumber(queued)} / ${formatNumber(queueEnabled)} ${t(
            '启用队列调度',
          )}`}
          tone={queueTone}
        />
        <RuntimeMetricTile
          label={t('平均等待')}
          value={formatLatency(summary?.avg_queue_wait_ms)}
          detail={`${formatNumber(summary?.queued_dispatches)} ${t('已排队')}`}
          tone={Number(summary?.avg_queue_wait_ms) > 0 ? 'warning' : 'success'}
        />
      </div>
      <div className='ct-model-gateway-record-tags'>
        <Tag color='cyan' type='light'>
          {t('缓存亲和')} {formatNumber(cacheAffinity)}
        </Tag>
        <Tag color={stickyBroken > 0 ? 'orange' : 'green'} type='light'>
          {t('粘滞断开')} {formatNumber(stickyBroken)}
        </Tag>
        <Tag color={queued > 0 ? 'orange' : 'green'} type='light'>
          {t('队列等待')} {formatLatency(summary?.avg_queue_wait_ms)}
        </Tag>
      </div>
    </DashboardCard>
  );
}

function normalizeStickyStorePayload(payload) {
  const items = Array.isArray(payload)
    ? payload
    : Array.isArray(payload?.items)
      ? payload.items
      : [];
  return {
    items,
    total: Number(payload?.total ?? items.length) || items.length,
  };
}

function StickyStorePanel({ refreshToken = 0, t }) {
  const [stickyStore, setStickyStore] = useState({ items: [], total: 0 });
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [clearingKeyID, setClearingKeyID] = useState('');
  const [bulkClearing, setBulkClearing] = useState(false);

  const loadStickyStore = useCallback(
    async (silent = false) => {
      if (silent) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      try {
        const response = await API.get(
          '/api/model_gateway/observability/sticky',
          {
            params: { limit: STICKY_STORE_LIMIT },
            disableDuplicate: true,
            skipErrorHandler: true,
          },
        );
        setStickyStore(normalizeStickyStorePayload(unwrapApiData(response)));
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('加载粘滞存储失败');
        showError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [t],
  );

  useEffect(() => {
    loadStickyStore(false);
  }, [loadStickyStore]);

  useEffect(() => {
    if (refreshToken > 0) {
      loadStickyStore(true);
    }
  }, [loadStickyStore, refreshToken]);

  const clearStickyEntry = useCallback(
    async (keyID) => {
      if (!keyID) return;
      setClearingKeyID(keyID);
      try {
        const response = await API.delete(
          `/api/model_gateway/observability/sticky/${encodeURIComponent(keyID)}`,
          { skipErrorHandler: true },
        );
        const payload = unwrapApiData(response);
        if (payload?.cleared === false) {
          Toast.warning(t('未找到粘滞记录'));
        } else {
          Toast.success(t('粘滞记录已清理'));
        }
        await loadStickyStore(true);
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('清理粘滞记录失败');
        showError(message);
      } finally {
        setClearingKeyID('');
      }
    },
    [loadStickyStore, t],
  );

  const confirmClearStickyEntry = useCallback(
    (record) => {
      const keyID = getStickyKeyID(record);
      if (!keyID) return;
      Modal.confirm({
        title: t('确认清理粘滞记录？'),
        content: (
          <div className='ct-model-gateway-sticky-confirm'>
            <Typography.Text>
              {t('仅清理该 key_id 对应的粘滞路由缓存')}
            </Typography.Text>
            <Typography.Text className='ct-model-gateway-sticky-mono'>
              {keyID}
            </Typography.Text>
          </div>
        ),
        okText: t('确定'),
        cancelText: t('取消'),
        onOk: () => clearStickyEntry(keyID),
      });
    },
    [clearStickyEntry, t],
  );

  const stickyBulkClearOptions = useMemo(() => {
    const map = new Map();
    stickyStore.items.forEach((item) => {
      const group = String(item.group || '').trim();
      const channelID = Number(item.channel_id || 0);
      if (!group || channelID <= 0) return;
      const key = `${group}\n${channelID}`;
      const current = map.get(key) || { group, channelID, count: 0 };
      current.count += 1;
      map.set(key, current);
    });
    return [...map.values()].sort((a, b) => {
      if (a.group !== b.group) return a.group.localeCompare(b.group);
      return a.channelID - b.channelID;
    });
  }, [stickyStore.items]);

  const clearStickyEntries = useCallback(
    async ({ group, channelID }) => {
      setBulkClearing(true);
      try {
        const response = await API.delete(
          '/api/model_gateway/observability/sticky',
          {
            params: { group, channel_id: channelID },
            skipErrorHandler: true,
          },
        );
        const payload = unwrapApiData(response);
        const deleted = Number(payload?.deleted || 0);
        if (deleted > 0) {
          Toast.success(`${t('已批量清理粘滞记录')} ${formatNumber(deleted)}`);
        } else {
          Toast.warning(t('未找到粘滞记录'));
        }
        await loadStickyStore(true);
      } catch (err) {
        const message =
          err?.response?.data?.message ||
          err?.message ||
          t('批量清理粘滞记录失败');
        showError(message);
      } finally {
        setBulkClearing(false);
      }
    },
    [loadStickyStore, t],
  );

  const confirmClearStickyEntries = useCallback(
    (option) => {
      if (!option?.group || !option?.channelID) return;
      Modal.confirm({
        title: t('确认批量清理粘滞记录？'),
        content: (
          <div className='ct-model-gateway-sticky-confirm'>
            <Typography.Text>
              {t('将清理该分组与渠道对应的共享粘滞路由缓存')}
            </Typography.Text>
            <Typography.Text className='ct-model-gateway-sticky-mono'>
              {option.group} / #{option.channelID}
            </Typography.Text>
            <Typography.Text type='secondary'>
              {formatNumber(option.count)} {t('条记录')}
            </Typography.Text>
          </div>
        ),
        okText: t('确定'),
        cancelText: t('取消'),
        onOk: () => clearStickyEntries(option),
      });
    },
    [clearStickyEntries, t],
  );

  const columns = useMemo(
    () => [
      {
        key: 'sticky-key-id',
        title: t('粘滞 Key ID'),
        dataIndex: 'key_id',
        width: 210,
        render: (_, record) => (
          <Typography.Text
            className='ct-model-gateway-sticky-mono'
            ellipsis={{ showTooltip: true }}
          >
            {getStickyKeyID(record) || '--'}
          </Typography.Text>
        ),
      },
      {
        key: 'sticky-source',
        title: t('来源'),
        dataIndex: 'source',
        width: 130,
        render: (_, record) => {
          const source = formatStickySource(
            record.source || record.sticky_source || record.key_source,
            t,
          );
          return source === '--' ? (
            <Typography.Text type='tertiary'>--</Typography.Text>
          ) : (
            <Tag color='cyan' size='small' type='light'>
              {source}
            </Tag>
          );
        },
      },
      {
        key: 'sticky-channel-id',
        title: t('渠道 ID'),
        dataIndex: 'channel_id',
        width: 110,
        render: (value) =>
          value ? (
            <Typography.Text strong>#{value}</Typography.Text>
          ) : (
            <Typography.Text type='tertiary'>--</Typography.Text>
          ),
      },
      {
        key: 'sticky-group',
        title: t('分组'),
        dataIndex: 'group',
        width: 130,
        render: (value) => (
          <Typography.Text ellipsis={{ showTooltip: true }}>
            {value || '--'}
          </Typography.Text>
        ),
      },
      {
        key: 'sticky-key-fingerprint',
        title: t('Key 指纹'),
        dataIndex: 'key_fingerprint',
        width: 180,
        render: (_, record) => (
          <Typography.Text
            className='ct-model-gateway-sticky-mono'
            ellipsis={{ showTooltip: true }}
          >
            {record.key_fingerprint || record.fingerprint || '--'}
          </Typography.Text>
        ),
      },
      {
        key: 'sticky-expires-at',
        title: t('过期于'),
        dataIndex: 'expires_at',
        width: 170,
        render: (_, record) => (
          <Typography.Text type='secondary' size='small'>
            {formatStickyExpiry(record)}
          </Typography.Text>
        ),
      },
      {
        key: 'sticky-ttl-seconds',
        title: t('剩余 TTL'),
        dataIndex: 'ttl_seconds',
        width: 120,
        render: (value, record) => (
          <Tag
            color={Number(value || record.ttl) > 60 ? 'green' : 'orange'}
            size='small'
            type='light'
          >
            {formatDurationSeconds(value ?? record.ttl, t)}
          </Tag>
        ),
      },
      {
        key: 'sticky-actions',
        title: t('操作'),
        dataIndex: 'key_id',
        width: 90,
        fixed: 'right',
        render: (_, record) => {
          const keyID = getStickyKeyID(record);
          return (
            <Tooltip content={t('清理粘滞记录')}>
              <Button
                size='small'
                type='danger'
                theme='borderless'
                icon={<Trash2 size={14} />}
                loading={clearingKeyID === keyID}
                disabled={!keyID}
                onClick={() => confirmClearStickyEntry(record)}
              />
            </Tooltip>
          );
        },
      },
    ],
    [clearingKeyID, confirmClearStickyEntry, t],
  );

  return (
    <DashboardCard
      title={
        <div className='ct-model-gateway-panel-title-row'>
          <span className='ct-model-gateway-panel-title'>
            <GitBranch size={17} />
            {t('粘滞存储')}
          </span>
          <div className='ct-model-gateway-sticky-actions'>
            <Tag color='cyan' size='small' type='light'>
              {formatNumber(stickyStore.total)} {t('条记录')}
            </Tag>
            <Select
              size='small'
              placeholder={t('批量清理')}
              disabled={stickyBulkClearOptions.length === 0 || bulkClearing}
              loading={bulkClearing}
              className='ct-model-gateway-sticky-bulk-select'
              onChange={(value) => {
                const option = stickyBulkClearOptions.find(
                  (item) => `${item.group}\n${item.channelID}` === value,
                );
                confirmClearStickyEntries(option);
              }}
            >
              {stickyBulkClearOptions.map((option) => (
                <Select.Option
                  key={`${option.group}\n${option.channelID}`}
                  value={`${option.group}\n${option.channelID}`}
                >
                  {option.group} / #{option.channelID} ·{' '}
                  {formatNumber(option.count)}
                </Select.Option>
              ))}
            </Select>
            <Button
              size='small'
              type='tertiary'
              icon={<RefreshCw size={14} />}
              loading={refreshing}
              onClick={() => loadStickyStore(true)}
            >
              {t('刷新')}
            </Button>
          </div>
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <Table
        className='ct-model-gateway-sticky-table'
        size='small'
        columns={columns}
        dataSource={stickyStore.items}
        rowKey={(record, index) => getStickyKeyID(record) || `sticky-${index}`}
        loading={loading}
        pagination={
          stickyStore.items.length > 10
            ? { pageSize: 10, size: 'small' }
            : false
        }
        empty={
          <Empty
            image={<IllustrationConstruction style={EMPTY_IMAGE_SIZE} />}
            darkModeImage={
              <IllustrationConstructionDark style={EMPTY_IMAGE_SIZE} />
            }
            title={t('暂无粘滞存储记录')}
          />
        }
        scroll={{ x: 1140 }}
      />
    </DashboardCard>
  );
}

function TrendStack({ value, detail, tone = 'default' }) {
  return (
    <div
      className={`ct-model-gateway-trend-stack ct-model-gateway-trend-stack-${tone}`}
    >
      <Typography.Text strong>{value}</Typography.Text>
      {detail && (
        <Typography.Text type='secondary' size='small'>
          {detail}
        </Typography.Text>
      )}
    </div>
  );
}

function getQueueWaitPercentiles(record) {
  return [
    { label: 'P50', value: record?.queue_wait_p50_ms },
    { label: 'P90', value: record?.queue_wait_p90_ms },
    { label: 'P95', value: record?.queue_wait_p95_ms },
  ];
}

function hasQueueWaitPercentiles(record) {
  return getQueueWaitPercentiles(record).some(({ value }) => {
    const numeric = Number(value);
    return Number.isFinite(numeric) && numeric > 0;
  });
}

function QueueWaitPercentileTags({ record, compact = false }) {
  const percentiles = getQueueWaitPercentiles(record);

  if (!hasQueueWaitPercentiles(record)) {
    return <Typography.Text type='tertiary'>--</Typography.Text>;
  }

  return (
    <div
      className={`ct-model-gateway-queue-percentiles${
        compact ? ' ct-model-gateway-queue-percentiles-compact' : ''
      }`}
    >
      {percentiles.map(({ label, value }) => (
        <span key={label}>
          <small>{label}</small>
          <strong>{formatLatency(value)}</strong>
        </span>
      ))}
    </div>
  );
}

function TrendSuccessCell({ record }) {
  const tone = getSuccessTone(record?.success_rate, record?.attempts);
  const rate = clampRate(record?.success_rate);
  const color =
    tone === 'success' ? 'green' : tone === 'warning' ? 'orange' : 'red';

  return (
    <div className='ct-model-gateway-trend-success'>
      <div className='ct-model-gateway-trend-success-head'>
        <Tag color={color} type='light' shape='circle' size='small'>
          {formatAttemptRate(record?.success_rate, record?.attempts)}
        </Tag>
        <Typography.Text type='secondary' size='small'>
          {formatNumber(record?.successes)} / {formatNumber(record?.attempts)}
        </Typography.Text>
      </div>
      <div
        className={`ct-model-gateway-trend-meter ct-model-gateway-trend-meter-${tone}`}
      >
        <span style={{ width: `${rate * 100}%` }} />
      </div>
    </div>
  );
}

function TrendBarStrip({ rows, t }) {
  const visibleRows = rows.slice(-24);
  if (!visibleRows.length) {
    return (
      <div className='ct-model-gateway-trend-empty'>{t('暂无调度趋势')}</div>
    );
  }

  return (
    <div className='ct-model-gateway-trend-strip'>
      {visibleRows.map((record) => {
        const rate = clampRate(record.success_rate);
        const tone = getSuccessTone(record.success_rate, record.attempts);
        const height = Math.max(8, Math.round(rate * 42));

        return (
          <Tooltip
            key={record._trend_key}
            content={
              <div className='ct-model-gateway-trend-tooltip'>
                <strong>{formatBucketRange(record)}</strong>
                <span>
                  {t('成功率')}: {formatPercent(record.success_rate)}
                </span>
                <span>
                  {t('平均耗时')}: {formatLatency(record.avg_duration_ms)}
                </span>
                <span>
                  {t('队列等待')}: {formatLatency(record.avg_queue_wait_ms)}
                </span>
                {hasQueueWaitPercentiles(record) && (
                  <span>
                    {t('队列等待分位数')}: P50{' '}
                    {formatLatency(record.queue_wait_p50_ms)} · P90{' '}
                    {formatLatency(record.queue_wait_p90_ms)} · P95{' '}
                    {formatLatency(record.queue_wait_p95_ms)}
                  </span>
                )}
              </div>
            }
          >
            <span
              className={`ct-model-gateway-trend-bar ct-model-gateway-trend-bar-${tone}`}
              style={{ height }}
            />
          </Tooltip>
        );
      })}
    </div>
  );
}

function TrendDimensionTags({ title, items, t, type = 'aggregate' }) {
  const visibleItems = Array.isArray(items) ? items.filter(Boolean) : [];
  return (
    <div className='ct-model-gateway-trend-dimension'>
      <Typography.Text type='secondary' size='small'>
        {title}
      </Typography.Text>
      {visibleItems.length ? (
        <div className='ct-model-gateway-record-tags'>
          {visibleItems.map((item) => {
            if (type === 'reason') {
              return (
                <Tag
                  key={item.reason || 'unknown'}
                  color='orange'
                  size='small'
                  type='light'
                >
                  {item.reason || t('未知')}: {formatNumber(item.count)}
                </Tag>
              );
            }
            if (type === 'circuit') {
              return (
                <Tag
                  key={item.reason || 'unknown'}
                  color='red'
                  size='small'
                  type='light'
                >
                  {formatCircuitErrorType(item.reason, t)}:{' '}
                  {formatNumber(item.count)}
                </Tag>
              );
            }
            if (type === 'risk') {
              const meta = getRiskSeverityMeta(item.severity, t);
              return (
                <Tag
                  key={item.reason || 'unknown'}
                  color={meta.color}
                  size='small'
                  type='light'
                >
                  {item.reason || t('未知')}: {formatNumber(item.count)}
                </Tag>
              );
            }
            return (
              <Tag
                key={item.key || 'unknown'}
                color='cyan'
                size='small'
                type='light'
              >
                {item.key || t('未知')} ·{' '}
                {formatAttemptRate(item.success_rate, item.attempts)} ·{' '}
                {formatNumber(item.attempts)} {t('尝试')}
              </Tag>
            );
          })}
        </div>
      ) : (
        <Typography.Text type='tertiary' size='small'>
          --
        </Typography.Text>
      )}
    </div>
  );
}

function TrendExpandedRow({ record, t }) {
  return (
    <div className='ct-model-gateway-trend-expand'>
      <TrendDimensionTags
        title={t('Provider Profile 趋势')}
        items={record?.by_provider_profile}
        t={t}
      />
      <TrendDimensionTags
        title={t('Proxy Mode 趋势')}
        items={record?.by_proxy_mode}
        t={t}
      />
      <div className='ct-model-gateway-trend-dimension'>
        <Typography.Text type='secondary' size='small'>
          {t('队列等待分位数')}
        </Typography.Text>
        <QueueWaitPercentileTags record={record} />
      </div>
      <TrendDimensionTags
        title={t('候选拒绝原因')}
        items={record?.reject_reasons}
        t={t}
        type='reason'
      />
      <TrendDimensionTags
        title={t('熔断打开原因趋势')}
        items={record?.circuit_open_reasons}
        t={t}
        type='circuit'
      />
      <TrendDimensionTags
        title={t('熔断错误类型趋势')}
        items={record?.circuit_error_types}
        t={t}
        type='circuit'
      />
    </div>
  );
}

function DispatchTrendPanel({ trends, t, onExport, circuitErrorType = '' }) {
  const rows = useMemo(
    () =>
      (Array.isArray(trends) ? trends : [])
        .filter((record) => trendMatchesCircuitError(record, circuitErrorType))
        .map((record, index) => ({
          ...record,
          _trend_key: `${record?.bucket_start || 'start'}-${
            record?.bucket_end || 'end'
          }-${index}`,
        })),
    [circuitErrorType, trends],
  );
  const columns = useMemo(
    () => [
      {
        title: t('时间桶'),
        dataIndex: 'bucket_start',
        width: 230,
        render: (_, record) => (
          <div
            className='ct-model-gateway-trend-time'
            title={formatBucketRange(record, false)}
          >
            <span className='ct-model-gateway-trend-range'>
              {formatBucketRange(record)}
            </span>
            <Typography.Text type='secondary' size='small'>
              {formatNumber(record.records)} {t('条记录')} ·{' '}
              {formatNumber(record.dispatches)} {t('次调度')}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('成功率'),
        dataIndex: 'success_rate',
        width: 160,
        render: (_, record) => <TrendSuccessCell record={record} />,
      },
      {
        title: t('平均耗时'),
        dataIndex: 'avg_duration_ms',
        width: 150,
        render: (_, record) => (
          <TrendStack
            value={formatLatency(record.avg_duration_ms)}
            detail={`${t('首包')} ${formatLatency(record.avg_ttft_ms)}`}
          />
        ),
      },
      {
        title: t('平均排队等待'),
        dataIndex: 'avg_queue_wait_ms',
        width: 150,
        render: (value, record) => (
          <TrendStack
            value={formatLatency(value)}
            detail={
              hasQueueWaitPercentiles(record)
                ? `P95 ${formatLatency(record.queue_wait_p95_ms)}`
                : null
            }
            tone={Number(value) > 0 ? 'warning' : 'success'}
          />
        ),
      },
      {
        title: t('队列等待分位数'),
        dataIndex: 'queue_wait_p50_ms',
        width: 210,
        render: (_, record) => (
          <QueueWaitPercentileTags record={record} compact />
        ),
      },
      {
        title: t('队列次数'),
        dataIndex: 'queued_dispatches',
        width: 150,
        render: (_, record) => (
          <TrendStack
            value={`${formatNumber(record.queued_dispatches)} / ${formatNumber(
              record.queue_enabled_dispatches,
            )}`}
            detail={t('已排队')}
            tone={Number(record.queued_dispatches) > 0 ? 'warning' : 'success'}
          />
        ),
      },
      {
        title: `${t('粘滞保留')} / ${t('粘滞断开')}`,
        dataIndex: 'sticky_retained',
        width: 170,
        render: (_, record) => (
          <TrendStack
            value={`${formatNumber(record.sticky_retained)} / ${formatNumber(
              record.sticky_broken,
            )}`}
            detail={`${formatNumber(record.sticky_routes)} ${t(
              '粘滞路由',
            )} · ${formatNumber(record.cache_affinity_routes)} ${t('缓存亲和')}`}
            tone={Number(record.sticky_broken) > 0 ? 'warning' : 'success'}
          />
        ),
      },
      {
        title: t('流中断'),
        dataIndex: 'stream_interrupted',
        width: 110,
        render: (value, record) => (
          <TrendStack
            value={formatNumber(value)}
            detail={`${formatNumber(record.failures)} ${t('失败')}`}
            tone={Number(value) > 0 ? 'danger' : 'success'}
          />
        ),
      },
    ],
    [t],
  );

  return (
    <DashboardCard
      title={
        <div className='ct-model-gateway-panel-title-row'>
          <span className='ct-model-gateway-panel-title'>
            <Activity size={17} />
            {t('调度趋势')}
          </span>
          <Button
            size='small'
            type='tertiary'
            icon={<Download size={14} />}
            onClick={onExport}
          >
            {t('导出趋势')}
          </Button>
        </div>
      }
      bodyClassName='ct-model-gateway-trend-body'
    >
      <TrendBarStrip rows={rows} t={t} />
      <Table
        className='ct-model-gateway-trend-table'
        size='small'
        columns={columns}
        dataSource={rows}
        rowKey='_trend_key'
        pagination={false}
        expandedRowRender={(record) => (
          <TrendExpandedRow record={record} t={t} />
        )}
        empty={
          <div className='ct-model-gateway-trend-empty'>
            {t('暂无调度趋势')}
          </div>
        }
        scroll={{ x: 1330 }}
      />
    </DashboardCard>
  );
}

function RuntimeRiskPanel({ runtimeStatus, t }) {
  const summary = runtimeStatus?.summary || {};
  const riskItems = (runtimeStatus?.items || [])
    .map((item) => ({ item, weight: getRuntimeRiskWeight(item) }))
    .filter(({ weight }) => weight > 0)
    .sort((a, b) => b.weight - a.weight)
    .slice(0, 6)
    .map(({ item }) => item);
  const riskCount =
    Number(summary.circuit_open || 0) +
    Number(summary.circuit_half_open || 0) +
    Number(summary.cooldown_channels || 0) +
    Number(summary.failure_avoidance_channels || 0) +
    Number(summary.high_pressure_channels || summary.saturated_channels || 0);

  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <Gauge size={17} />
          {t('运行态风险概览')}
        </span>
      }
      bodyClassName='ct-model-gateway-insight-body'
    >
      <div className='ct-model-gateway-runtime-metrics'>
        <RuntimeMetricTile
          label={t('风险渠道')}
          value={formatNumber(riskCount)}
          detail={`${formatNumber(summary.channels)} ${t('渠道')}`}
          tone={riskCount > 0 ? 'warning' : 'success'}
        />
        <RuntimeMetricTile
          label={t('熔断打开')}
          value={formatNumber(summary.circuit_open)}
          detail={`${formatNumber(summary.circuit_half_open)} ${t('半开探测')}`}
          tone={summary.circuit_open > 0 ? 'danger' : 'success'}
        />
        <RuntimeMetricTile
          label={t('并发压力')}
          value={formatNumber(
            summary.high_pressure_channels || summary.saturated_channels,
          )}
          detail={`${formatNumber(summary.active_concurrency)} ${t('活跃并发')}`}
          tone={
            Number(
              summary.high_pressure_channels || summary.saturated_channels || 0,
            ) > 0
              ? 'warning'
              : 'success'
          }
        />
        <RuntimeMetricTile
          label={t('冷却隔离')}
          value={formatNumber(summary.cooldown_channels)}
          detail={`${formatNumber(
            summary.probe_recovery_pending_channels,
          )} ${t('恢复中')}`}
          tone={
            summary.cooldown_channels > 0 ||
            summary.probe_recovery_pending_channels > 0
              ? 'warning'
              : 'success'
          }
        />
        <RuntimeMetricTile
          label={t('低分恢复')}
          value={formatNumber(summary.low_score_recovery_channels)}
          detail={`${formatNumber(summary.recently_recovered_channels)} ${t(
            '近期恢复',
          )}`}
          tone={
            Number(summary.low_score_recovery_channels || 0) > 0
              ? 'warning'
              : 'success'
          }
        />
      </div>
      {riskItems.length ? (
        <div className='ct-model-gateway-risk-list'>
          {riskItems.map((item) => {
            const meta = getRuntimeHealthMeta(item.health_status, t);
            const key = `${item.requested_model || 'model'}-${
              item.channel_id || 0
            }-${item.group || 'group'}-${item.endpoint_type || 'endpoint'}`;
            return (
              <div className='ct-model-gateway-risk-row' key={key}>
                <div className='ct-model-gateway-runtime-name'>
                  <Typography.Text strong ellipsis={{ showTooltip: true }}>
                    {item.requested_model || '--'}
                  </Typography.Text>
                  <Typography.Text type='secondary' size='small'>
                    #{item.channel_id || '--'} · {item.group || '--'}
                    {item.upstream_model ? ` · ${item.upstream_model}` : ''}
                  </Typography.Text>
                </div>
                <div className='ct-model-gateway-record-tags'>
                  <Tag color={meta.color} size='small' type='light'>
                    {meta.label}
                  </Tag>
                  {item.circuit_open_reason && (
                    <Tag color='red' size='small' type='light'>
                      {formatCircuitErrorType(item.circuit_open_reason, t)}
                    </Tag>
                  )}
                  {item.queue_depth > 0 && (
                    <Tag color='cyan' size='small' type='light'>
                      {t('队列')} {formatNumber(item.queue_depth)}
                    </Tag>
                  )}
                  {item.active_concurrency > 0 && (
                    <Tag color='grey' size='small' type='light'>
                      {t('并发')} {formatNumber(item.active_concurrency)}
                      {item.max_concurrency > 0
                        ? ` / ${formatNumber(item.max_concurrency)}`
                        : ''}
                    </Tag>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <Typography.Text type='secondary' size='small'>
          {t('暂无高风险运行键')}
        </Typography.Text>
      )}
    </DashboardCard>
  );
}

function getRiskSeverityMeta(severity, t) {
  switch (severity) {
    case 'critical':
      return { color: 'red', label: t('严重') };
    case 'warning':
      return { color: 'orange', label: t('警告') };
    case 'info':
      return { color: 'blue', label: t('信息') };
    default:
      return { color: 'grey', label: severity || t('未知') };
  }
}

function RiskTimelinePanel({ risk, riskTimeline, t, circuitErrorType = '' }) {
  const filteredRisk = filterRiskSnapshotByCircuitError(risk, circuitErrorType);
  const timeline = Array.isArray(riskTimeline)
    ? riskTimeline
    : Array.isArray(filteredRisk?.risk_timeline)
      ? filteredRisk.risk_timeline
      : Array.isArray(filteredRisk?.timeline)
        ? filteredRisk.timeline
        : [];
  const filteredTimeline = timeline.filter((event) =>
    riskEventMatchesCircuitError(event, circuitErrorType),
  );
  const topStatuses = Array.isArray(filteredRisk?.top_risk_statuses)
    ? filteredRisk.top_risk_statuses
    : Array.isArray(filteredRisk?.top_statuses)
      ? filteredRisk.top_statuses
      : [];
  const topRejectReasons = Array.isArray(filteredRisk?.top_reject_reasons)
    ? filteredRisk.top_reject_reasons
    : [];
  const topCircuitOpenReasons = Array.isArray(
    filteredRisk?.top_circuit_open_reasons,
  )
    ? filteredRisk.top_circuit_open_reasons
    : [];
  const topCircuitErrorTypes = Array.isArray(
    filteredRisk?.top_circuit_error_types,
  )
    ? filteredRisk.top_circuit_error_types
    : [];
  const visibleEvents = filteredTimeline.slice(0, 8);
  const riskEventCount = Number(
    filteredRisk?.risk_event_count ||
      filteredRisk?.event_count ||
      filteredTimeline.length ||
      0,
  );
  const statusChanges = Number(
    filteredRisk?.risk_status_changes || filteredRisk?.status_changes || 0,
  );
  const currentRuntimeKeys = Number(
    filteredRisk?.current_risk_runtime_keys ||
      filteredRisk?.current_runtime_keys ||
      0,
  );

  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <ListTree size={17} />
          {t('风险事件线')}
        </span>
      }
      bodyClassName='ct-model-gateway-insight-body'
    >
      <div className='ct-model-gateway-runtime-metrics'>
        <RuntimeMetricTile
          label={t('风险事件')}
          value={formatNumber(riskEventCount)}
          detail={`${formatNumber(statusChanges)} ${t('状态变化')}`}
          tone={riskEventCount > 0 ? 'warning' : 'success'}
        />
        <RuntimeMetricTile
          label={t('当前风险键')}
          value={formatNumber(currentRuntimeKeys)}
          detail={`${formatNumber(topStatuses.length)} ${t('风险状态')}`}
          tone={currentRuntimeKeys > 0 ? 'warning' : 'success'}
        />
        <RuntimeMetricTile
          label={t('拒绝原因')}
          value={formatNumber(topRejectReasons.length)}
          detail={`${formatNumber(visibleEvents.length)} ${t('最近事件')}`}
          tone={topRejectReasons.length > 0 ? 'warning' : 'success'}
        />
      </div>
      <div className='ct-model-gateway-risk-summary-grid'>
        <TrendDimensionTags
          title={t('Top 风险状态')}
          items={topStatuses.map((item) => ({
            reason: item.status,
            count: item.count,
            severity: item.severity,
          }))}
          t={t}
          type='risk'
        />
        <TrendDimensionTags
          title={t('Top 拒绝原因')}
          items={topRejectReasons}
          t={t}
          type='reason'
        />
        <TrendDimensionTags
          title={t('Top 熔断打开原因')}
          items={topCircuitOpenReasons}
          t={t}
          type='circuit'
        />
        <TrendDimensionTags
          title={t('Top 熔断错误类型')}
          items={topCircuitErrorTypes}
          t={t}
          type='circuit'
        />
      </div>
      {visibleEvents.length ? (
        <div className='ct-model-gateway-risk-timeline'>
          {visibleEvents.map((event, index) => {
            const meta = getRiskSeverityMeta(event.severity, t);
            const key = `${event.timestamp || event.bucket_start || 'risk'}-${
              event.event_type || 'event'
            }-${event.status || 'status'}-${index}`;
            const occurredAt = event.timestamp || event.bucket_start;
            return (
              <div className='ct-model-gateway-risk-event' key={key}>
                <Tag color={meta.color} size='small' type='light'>
                  {meta.label}
                </Tag>
                <div className='ct-model-gateway-risk-event-main'>
                  <Typography.Text strong ellipsis={{ showTooltip: true }}>
                    {event.status || event.event_type || t('未知风险')}
                  </Typography.Text>
                  <Typography.Text type='secondary' size='small'>
                    {formatBucketTimestamp(occurredAt)} ·{' '}
                    {event.source || t('未知来源')} ·{' '}
                    {formatNumber(event.count)} {t('次')}
                  </Typography.Text>
                </div>
                <div className='ct-model-gateway-record-tags'>
                  {event.reason && (
                    <Tag color='orange' size='small' type='light'>
                      {event.event_type === 'circuit_error_type' ||
                      event.event_type === 'circuit_open_reason'
                        ? formatCircuitErrorType(event.reason, t)
                        : event.reason}
                    </Tag>
                  )}
                  {event.group && (
                    <Tag color='cyan' size='small' type='light'>
                      {event.group}
                    </Tag>
                  )}
                  {event.channel_id > 0 && (
                    <Tag color='grey' size='small' type='light'>
                      #{event.channel_id}
                    </Tag>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <Typography.Text type='secondary' size='small'>
          {t('暂无风险事件')}
        </Typography.Text>
      )}
    </DashboardCard>
  );
}

function RuntimeStatusPanel({ runtimeStatus, t, circuitErrorType = '' }) {
  const filteredRuntimeStatus = filterRuntimeStatusByCircuitError(
    runtimeStatus,
    circuitErrorType,
  );
  const summary = filteredRuntimeStatus?.summary || {};
  const items = filteredRuntimeStatus?.items || [];
  const columns = useMemo(
    () => [
      {
        title: t('状态'),
        dataIndex: 'health_status',
        width: 120,
        render: (value) => {
          const meta = getRuntimeHealthMeta(value, t);
          return (
            <Tag color={meta.color} type='light' shape='circle'>
              {meta.label}
            </Tag>
          );
        },
      },
      {
        title: t('模型 / 渠道'),
        dataIndex: 'requested_model',
        width: 260,
        render: (_, record) => (
          <div className='ct-model-gateway-runtime-name'>
            <Typography.Text strong ellipsis={{ showTooltip: true }}>
              {record.requested_model || '--'}
            </Typography.Text>
            <Typography.Text type='secondary' size='small'>
              #{record.channel_id || '--'} · {record.group || '--'}
              {record.upstream_model ? ` · ${record.upstream_model}` : ''}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('并发'),
        dataIndex: 'active_concurrency',
        width: 120,
        render: (_, record) => (
          <Typography.Text strong>
            {formatNumber(record.active_concurrency)}
            {record.max_concurrency > 0
              ? ` / ${formatNumber(record.max_concurrency)}`
              : ''}
          </Typography.Text>
        ),
      },
      {
        title: t('队列'),
        dataIndex: 'queue_depth',
        width: 180,
        render: (_, record) => (
          <div className='ct-model-gateway-runtime-stack'>
            <Typography.Text strong>
              {formatNumber(record.queue_depth)}
              {record.queue_capacity > 0
                ? ` / ${formatNumber(record.queue_capacity)}`
                : ''}
            </Typography.Text>
            <Typography.Text type='secondary' size='small'>
              {t('预估等待')} {formatLatency(record.estimated_queue_wait_ms)}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('熔断'),
        dataIndex: 'circuit_state',
        width: 240,
        render: (_, record) => (
          <div className='ct-model-gateway-runtime-stack'>
            <div className='ct-model-gateway-record-tags'>
              <Tag
                color={
                  record.circuit_open
                    ? 'red'
                    : record.circuit_state === 'half_open'
                      ? 'orange'
                      : 'green'
                }
                size='small'
                type='light'
              >
                {record.circuit_state || 'closed'}
              </Tag>
              {record.circuit_open_reason && (
                <Tag color='red' size='small' type='light'>
                  {formatCircuitErrorType(record.circuit_open_reason, t)}
                </Tag>
              )}
            </div>
            <Typography.Text type='secondary' size='small'>
              {record.circuit_open_until
                ? formatTimestamp(record.circuit_open_until)
                : `${t('样本')} ${formatNumber(record.circuit_sample_count)}`}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('熔断错误类型'),
        dataIndex: 'circuit_error_counts',
        width: 240,
        render: (value) => {
          const entries = Object.entries(value || {})
            .filter(([, count]) => Number(count) > 0)
            .sort((a, b) => Number(b[1]) - Number(a[1]))
            .slice(0, 3);
          if (!entries.length) {
            return <Typography.Text type='tertiary'>--</Typography.Text>;
          }
          return (
            <div className='ct-model-gateway-record-tags'>
              {entries.map(([kind, count]) => (
                <Tag key={kind} color='orange' size='small' type='light'>
                  {formatCircuitErrorType(kind, t)} {formatNumber(count)}
                </Tag>
              ))}
            </div>
          );
        },
      },
      {
        title: t('冷却 / 降权'),
        dataIndex: 'cooldown',
        width: 230,
        render: (_, record) => {
          const tags = [];
          if (record.cooldown) {
            tags.push(
              <Tag key='cooldown' color='orange' size='small' type='light'>
                {t('冷却')} {formatNumber(record.cooldown_remaining_seconds)}s
              </Tag>,
            );
          }
          if (record.failure_avoidance) {
            tags.push(
              <Tag key='avoidance' color='amber' size='small' type='light'>
                {t('恢复中')}{' '}
                {formatNumber(record.failure_avoidance_remaining_seconds)}s
              </Tag>,
            );
          }
          if (record.probe_recovery_pending) {
            tags.push(
              <Tag
                key='recovery-progress'
                color='cyan'
                size='small'
                type='light'
              >
                {t('恢复')}{' '}
                {formatNumber(record.probe_recovery_success_count || 0)}/
                {formatNumber(record.probe_recovery_required || 0)}
              </Tag>,
            );
          }
          if (!tags.length) {
            return <Typography.Text type='tertiary'>--</Typography.Text>;
          }
          return <div className='ct-model-gateway-record-tags'>{tags}</div>;
        },
      },
      {
        title: t('性能'),
        dataIndex: 'success_rate',
        width: 220,
        render: (_, record) => (
          <div className='ct-model-gateway-runtime-stack'>
            <Typography.Text strong>
              {formatAttemptRate(record.success_rate, record.attempts)}
            </Typography.Text>
            <Typography.Text type='secondary' size='small'>
              {t('平均耗时')} {formatLatency(record.duration_ms)} · {t('首包')}{' '}
              {formatLatency(record.ttft_ms)}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('倍率'),
        dataIndex: 'cost_ratio',
        width: 150,
        render: (_, record) => (
          <div className='ct-model-gateway-runtime-stack'>
            <Typography.Text>
              {t('成本倍率')} {formatScore(record.cost_ratio)}
            </Typography.Text>
            <Typography.Text type='secondary' size='small'>
              {t('稳定评分')} {formatScore(record.score_total)} ·{' '}
              <Tooltip
                content={t('探活原因按当前模型/分组运行键统计，不代表整个渠道')}
              >
                <span>
                  {formatProbeReason(record.probe_trigger_reason, t) ||
                    t('无探测')}
                </span>
              </Tooltip>
            </Typography.Text>
          </div>
        ),
      },
    ],
    [t],
  );

  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <Activity size={17} />
          {t('运行态状态')}
        </span>
      }
      bodyClassName='ct-model-gateway-runtime-body'
    >
      <div className='ct-model-gateway-runtime-metrics'>
        <RuntimeMetricTile
          label={t('活跃并发')}
          value={formatNumber(summary.active_concurrency)}
          detail={`${formatNumber(summary.channels)} ${t('渠道')}`}
          tone='default'
        />
        <RuntimeMetricTile
          label={t('排队请求')}
          value={formatNumber(summary.queued_requests)}
          detail={`${formatNumber(summary.queue_channels)} ${t('队列渠道')}`}
          tone={summary.queued_requests > 0 ? 'warning' : 'success'}
        />
        <RuntimeMetricTile
          label={t('熔断渠道')}
          value={formatNumber(summary.circuit_open)}
          detail={`${formatNumber(summary.circuit_half_open)} ${t('半开探测')}`}
          tone={summary.circuit_open > 0 ? 'danger' : 'success'}
        />
        <RuntimeMetricTile
          label={t('冷却渠道')}
          value={formatNumber(summary.cooldown_channels)}
          detail={`${formatNumber(
            summary.probe_recovery_pending_channels,
          )} ${t('恢复中')}`}
          tone={
            summary.cooldown_channels > 0 ||
            summary.probe_recovery_pending_channels > 0
              ? 'warning'
              : 'success'
          }
        />
        <RuntimeMetricTile
          label={t('低分恢复')}
          value={formatNumber(summary.low_score_recovery_channels)}
          detail={`${formatNumber(
            summary.recently_recovered_channels,
          )} ${t('近期恢复')}`}
          tone={
            Number(summary.low_score_recovery_channels || 0) > 0
              ? 'warning'
              : 'success'
          }
        />
      </div>
      <Table
        size='small'
        columns={columns}
        dataSource={items}
        rowKey={(record) =>
          `${record.requested_model || 'model'}-${record.channel_id || 0}-${
            record.group || 'group'
          }-${record.endpoint_type || 'endpoint'}`
        }
        pagination={false}
        empty={
          <Empty
            image={<IllustrationConstruction style={EMPTY_IMAGE_SIZE} />}
            darkModeImage={
              <IllustrationConstructionDark style={EMPTY_IMAGE_SIZE} />
            }
            title={t('暂无运行态状态数据')}
          />
        }
        scroll={{ x: 1520 }}
      />
      <Typography.Text type='secondary' size='small'>
        {t('运行态最近更新时间')}: {formatTimestamp(summary.updated_at)}
      </Typography.Text>
    </DashboardCard>
  );
}

function DetailValue({ children }) {
  return (
    <Typography.Text
      ellipsis={{ showTooltip: true }}
      className='ct-model-gateway-detail-value'
    >
      {children || '--'}
    </Typography.Text>
  );
}

function DetailPanel({ title, children, className = '' }) {
  return (
    <section
      className={`ct-model-gateway-detail-panel${
        className ? ` ${className}` : ''
      }`}
    >
      <Typography.Title heading={6}>{title}</Typography.Title>
      {children}
    </section>
  );
}

function isDisplayEmpty(value) {
  return value === undefined || value === null || value === '';
}

function DetailMetricTile({
  label,
  value,
  detail,
  tone = 'default',
  onClick,
  actionLabel,
  actionTone = 'default',
}) {
  const clickable = typeof onClick === 'function';
  const content = (
    <>
      <span>{label}</span>
      <strong>{isDisplayEmpty(value) ? '--' : value}</strong>
      {detail ? <small>{detail}</small> : null}
      {actionLabel ? (
        <em
          className={`ct-model-gateway-detail-metric-action ct-model-gateway-detail-metric-action-${actionTone}`}
        >
          {actionLabel}
        </em>
      ) : null}
    </>
  );
  const className = `ct-model-gateway-detail-metric ct-model-gateway-detail-metric-${tone}${
    clickable ? ' ct-model-gateway-detail-metric-clickable' : ''
  }`;
  if (clickable) {
    return (
      <button type='button' className={className} onClick={onClick}>
        {content}
      </button>
    );
  }
  return <div className={className}>{content}</div>;
}

function ScoreItemsTable({
  items = [],
  deltas = [],
  t,
  compact = false,
  preparedRows = null,
}) {
  const rows = Array.isArray(preparedRows)
    ? preparedRows
    : scoreItemDisplayRows(items, deltas, t);
  if (!rows.length) {
    return <Typography.Text type='tertiary'>--</Typography.Text>;
  }
  return (
    <div
      className={`ct-model-gateway-score-items-table${
        compact ? ' ct-model-gateway-score-items-table-compact' : ''
      }`}
    >
      <div className='ct-model-gateway-score-items-head'>
        <span>{t('评分项')}</span>
        <span>{t('原始数据')}</span>
        <span>{t('窗口')}</span>
        <span>{t('得分')}</span>
        <span>{t('权重')}</span>
        <span>{t('加权贡献')}</span>
        <span>{t('贡献变化')}</span>
        <span>{t('公式 / 原因')}</span>
      </div>
      {rows.map((item) => {
        const changeValue = Number(item.change_value || 0);
        const deltaTone = scoreDeltaTone(changeValue);
        const changeTitle =
          item.change_kind === 'weighted'
            ? t('加权贡献变化，才会影响稳定评分总分')
            : t('原始子项变化，不等于总分直接加减');
        const scoreBoosted =
          item.score_adjusted === true && Number(item.score_boost || 0) > 0;
        const baseScore =
          item.base_score === undefined || item.base_score === null
            ? item.score
            : item.base_score;
        const boostTitle = scoreBoosted
          ? t('基础分 {{base}} + 加成 {{boost}} = 最终分 {{score}}', {
              base: formatScoreWithZero(baseScore),
              boost: formatScoreWithZero(item.score_boost),
              score: formatScoreWithZero(item.score),
            })
          : '';
        return (
          <div className='ct-model-gateway-score-items-row' key={item.key}>
            <span
              className='ct-model-gateway-score-item-name'
              title={item.formula || scoreMetricDescription(item.key, t)}
            >
              <strong>{item.name}</strong>
              {item.category ? <small>{t(item.category)}</small> : null}
              {item.missing_reason ? <em>{t(item.missing_reason)}</em> : null}
            </span>
            <span className='ct-model-gateway-score-item-raw'>
              {formatScoreItemRawValue(item, t)}
            </span>
            <span>{formatScoreItemSourceAndWindow(item, t)}</span>
            <span className='ct-model-gateway-score-item-score'>
              <strong className='ct-model-gateway-score-item-number'>
                {formatScore(item.score)}
              </strong>
              {scoreBoosted && (
                <small
                  className='ct-model-gateway-score-boost-badge'
                  title={boostTitle}
                >
                  {formatScoreWithZero(baseScore)} +{' '}
                  {formatScoreWithZero(item.score_boost)}
                </small>
              )}
            </span>
            <span className='ct-model-gateway-score-item-number'>
              {item.weight > 0 ? formatPercent(item.weight) : '--'}
            </span>
            <strong className='ct-model-gateway-score-item-number'>
              {formatScore(item.weighted_score)}
            </strong>
            <em
              className={`ct-model-gateway-score-delta-${deltaTone}`}
              title={changeTitle}
            >
              {formatScoreDelta(changeValue)}
            </em>
            <span
              className='ct-model-gateway-score-item-formula'
              title={[item.formula, item.reason].filter(Boolean).join(' / ')}
            >
              {item.formula || scoreMetricDescription(item.key, t) || '--'}
              {item.reason ? <small>{item.reason}</small> : null}
            </span>
          </div>
        );
      })}
    </div>
  );
}

function ScoreBreakdownPanel({
  entries = [],
  items = [],
  deltas = [],
  onOpenScoreHistory,
  t,
}) {
  return (
    <div className='ct-model-gateway-score-breakdown-panel'>
      <div className='ct-model-gateway-score-breakdown-head'>
        <div>
          <Typography.Text strong>{t('本次评分拆解')}</Typography.Text>
          <Typography.Text type='secondary' size='small'>
            {t('点击评分查看历史变化和主要变更原因')}
          </Typography.Text>
        </div>
        <Button
          size='small'
          type='tertiary'
          icon={<Activity size={14} />}
          disabled={!onOpenScoreHistory}
          onClick={onOpenScoreHistory}
        >
          {t('查看变更')}
        </Button>
      </div>
      {Array.isArray(items) && items.length ? (
        <ScoreItemsTable items={items} deltas={deltas} t={t} />
      ) : (
        <div className='ct-model-gateway-score-list'>
          {entries.length ? (
            entries.map(([key, value]) => (
              <Tooltip key={key} content={scoreMetricDescription(key, t)}>
                <Tag color='cyan' type='light' shape='circle'>
                  {scoreMetricLabel(key, t)}: {formatScore(value)}
                </Tag>
              </Tooltip>
            ))
          ) : (
            <Typography.Text type='tertiary'>--</Typography.Text>
          )}
        </div>
      )}
    </div>
  );
}

function DetailInfoGrid({ items = [] }) {
  return (
    <div className='ct-model-gateway-detail-info-grid'>
      {items.map((item) => (
        <div key={item.key} className='ct-model-gateway-detail-info-item'>
          <span>{item.label}</span>
          <div>{isDisplayEmpty(item.value) ? '--' : item.value}</div>
        </div>
      ))}
    </div>
  );
}

function buildTimingBreakdown(record) {
  const timing = record?.request_meta?.timing || {};
  const queueWaitMs = Number(
    timing.queue_wait_ms ||
      record?.request_meta?.queue_wait_ms ||
      record?.queue_wait_ms ||
      0,
  );
  const relayToFirstByteMs = Number(timing.relay_to_first_byte_ms || 0);
  const relayTotalMs = Number(timing.relay_total_ms || 0);
  const upstreamResponseHeaderMs = Number(
    timing.upstream_response_header_ms || 0,
  );
  const upstreamFirstEventWaitMs = Number(
    timing.upstream_first_event_wait_ms || 0,
  );
  const requestBodyPrepareMs = Number(timing.request_body_prepare_ms || 0);
  const requestBodyBytes = Number(timing.request_body_bytes || 0);
  const requestBodyStorage = String(timing.request_body_storage || '');
  const requestBodySizeLikelyLatency = Boolean(
    timing.request_body_size_likely_latency,
  );
  const ttftMs = Number(record?.ttft_ms || 0);
  const durationMs = Number(record?.duration_ms || 0);
  const effectiveRelayFirstByteMs =
    relayToFirstByteMs > 0
      ? relayToFirstByteMs
      : ttftMs > 0
        ? Math.max(0, ttftMs - queueWaitMs)
        : 0;
  const effectiveRelayTotalMs =
    relayTotalMs > 0
      ? relayTotalMs
      : durationMs > 0
        ? Math.max(0, durationMs - queueWaitMs)
        : 0;
  const postFirstByteMs =
    Number(timing.post_first_byte_ms || 0) ||
    (effectiveRelayTotalMs > effectiveRelayFirstByteMs
      ? effectiveRelayTotalMs - effectiveRelayFirstByteMs
      : 0);
  const preFirstByteMs =
    Number(timing.pre_first_byte_ms || 0) ||
    (queueWaitMs > 0 || effectiveRelayFirstByteMs > 0
      ? queueWaitMs + effectiveRelayFirstByteMs
      : 0);
  const totalKnownMs =
    queueWaitMs + effectiveRelayFirstByteMs + postFirstByteMs;

  if (
    queueWaitMs <= 0 &&
    effectiveRelayFirstByteMs <= 0 &&
    effectiveRelayTotalMs <= 0 &&
    postFirstByteMs <= 0 &&
    upstreamResponseHeaderMs <= 0 &&
    upstreamFirstEventWaitMs <= 0 &&
    requestBodyPrepareMs <= 0 &&
    requestBodyBytes <= 0
  ) {
    return null;
  }

  return {
    queueWaitMs,
    relayToFirstByteMs: effectiveRelayFirstByteMs,
    relayTotalMs: effectiveRelayTotalMs,
    preFirstByteMs,
    postFirstByteMs,
    totalKnownMs,
    upstreamResponseHeaderMs,
    upstreamFirstEventWaitMs,
    requestBodyPrepareMs,
    requestBodyBytes,
    requestBodyStorage,
    requestBodySizeLikelyLatency,
  };
}

function dominantTimingSegment(timing, t) {
  if (!timing) return '';
  const segments = [
    [t('队列等待'), timing.queueWaitMs],
    [t('转发首包'), timing.relayToFirstByteMs],
    [t('首包后生成'), timing.postFirstByteMs],
  ].filter(([, value]) => Number(value) > 0);
  if (!segments.length) return '';
  segments.sort((a, b) => Number(b[1]) - Number(a[1]));
  return t('主要耗时在 {{segment}}', { segment: segments[0][0] });
}

function TimingBreakdownPanel({ record, t }) {
  const timing = buildTimingBreakdown(record);
  if (!timing) return null;
  const total = Math.max(Number(timing.totalKnownMs || 0), 1);
  const items = [
    {
      key: 'queue',
      label: t('队列等待'),
      value: timing.queueWaitMs,
      detail: t('本地并发队列等待'),
      tone: 'queue',
    },
    {
      key: 'first_byte',
      label: t('转发首包'),
      value: timing.relayToFirstByteMs,
      detail: t('网关转发到上游首包'),
      tone: 'relay',
    },
    {
      key: 'generation',
      label: t('首包后生成'),
      value: timing.postFirstByteMs,
      detail: t('首包之后到完成'),
      tone: 'generation',
    },
  ];

  return (
    <DetailPanel title={t('耗时拆解')}>
      <div className='ct-model-gateway-timing-breakdown'>
        <div className='ct-model-gateway-timing-summary'>
          <Timer size={15} />
          <span>{dominantTimingSegment(timing, t)}</span>
          <Tag color='blue' type='light' shape='circle'>
            {t('首包前')} {formatLatency(timing.preFirstByteMs)}
          </Tag>
        </div>
        <div className='ct-model-gateway-timing-bars'>
          {items.map((item) => {
            const width = `${Math.max(4, (Number(item.value || 0) / total) * 100)}%`;
            return (
              <div
                key={item.key}
                className={`ct-model-gateway-timing-row ct-model-gateway-timing-row-${item.tone}`}
              >
                <div className='ct-model-gateway-timing-label'>
                  <span>{item.label}</span>
                  <strong>{formatLatency(item.value)}</strong>
                </div>
                <div className='ct-model-gateway-timing-track'>
                  <i style={{ width }} />
                </div>
                <small>{item.detail}</small>
              </div>
            );
          })}
        </div>
        {timing.upstreamResponseHeaderMs > 0 ||
        timing.upstreamFirstEventWaitMs > 0 ||
        timing.requestBodyPrepareMs > 0 ||
        timing.requestBodyBytes > 0 ? (
          <div className='ct-model-gateway-timing-detail-grid'>
            {timing.requestBodyPrepareMs > 0 ? (
              <div>
                <span>{t('请求体准备')}</span>
                <strong>{formatLatency(timing.requestBodyPrepareMs)}</strong>
              </div>
            ) : null}
            {timing.upstreamResponseHeaderMs > 0 ? (
              <div>
                <span>{t('上游响应头')}</span>
                <strong>
                  {formatLatency(timing.upstreamResponseHeaderMs)}
                </strong>
              </div>
            ) : null}
            {timing.upstreamFirstEventWaitMs > 0 ? (
              <div>
                <span>{t('首个流事件等待')}</span>
                <strong>
                  {formatLatency(timing.upstreamFirstEventWaitMs)}
                </strong>
              </div>
            ) : null}
            {timing.requestBodyBytes > 0 ? (
              <div>
                <span>{t('上游请求体')}</span>
                <strong>{formatBytes(timing.requestBodyBytes)}</strong>
                <small>
                  {timing.requestBodyStorage === 'disk'
                    ? t('磁盘缓存')
                    : timing.requestBodyStorage === 'memory'
                      ? t('内存缓存')
                      : ''}
                  {timing.requestBodySizeLikelyLatency
                    ? ` · ${t('大请求体可能影响首包')}`
                    : ''}
                </small>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
    </DetailPanel>
  );
}

function DetailAccordion({
  title,
  meta,
  children,
  defaultOpen = false,
  className = '',
}) {
  return (
    <details
      className={`ct-model-gateway-detail-accordion${
        className ? ` ${className}` : ''
      }`}
      open={defaultOpen}
    >
      <summary>
        <span>{title}</span>
        {meta ? <em>{meta}</em> : null}
      </summary>
      <div className='ct-model-gateway-detail-accordion-body'>{children}</div>
    </details>
  );
}

function getCandidateExplanations(record) {
  const topLevel = record?.candidate_explanations;
  const metaLevel = record?.request_meta?.candidate_explanations;
  if (Array.isArray(topLevel) && topLevel.length) return topLevel;
  if (Array.isArray(metaLevel) && metaLevel.length) return metaLevel;
  return [];
}

function normalizeMetaStringList(value) {
  if (Array.isArray(value)) {
    return value.map((item) => String(item || '').trim()).filter(Boolean);
  }
  if (typeof value === 'string') {
    return value
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
}

function getDispatchRequirements(record) {
  const meta = record?.request_meta || {};
  const tools = normalizeMetaStringList(meta?.required_tools).filter(
    (tool) => tool !== 'image_generation',
  );
  const conditions = normalizeMetaStringList(
    meta?.candidate_filter_conditions,
  ).filter((condition) => condition !== 'codex_image_generation_tool');
  return {
    requiresCodexImageTool: false,
    tools,
    conditions,
    visible: tools.length > 0 || conditions.length > 0,
  };
}

function formatDispatchTool(tool, t) {
  const normalized = String(tool || '').trim();
  if (normalized === 'image_generation') return 'image_generation';
  return normalized || t('未知工具');
}

function formatDispatchFilterCondition(condition, t) {
  const normalized = String(condition || '').trim();
  return formatTechnicalCode(normalized) || t('未知过滤条件');
}

function DispatchRequirementNotice({ requirements, t }) {
  if (!requirements?.visible) return null;
  return (
    <div className='ct-model-gateway-dispatch-requirement'>
      <div className='ct-model-gateway-dispatch-requirement-head'>
        <Wrench size={16} />
        <span>{t('本次调用要求')}</span>
      </div>
      <div className='ct-model-gateway-dispatch-requirement-body'>
        {requirements.tools.length ? (
          <div className='ct-model-gateway-dispatch-requirement-row'>
            <span>{t('调度所需工具能力')}</span>
            <div className='ct-model-gateway-record-tags'>
              {requirements.tools.map((tool) => (
                <Tag key={tool} color='purple' type='solid' size='small'>
                  {formatDispatchTool(tool, t)}
                </Tag>
              ))}
            </div>
          </div>
        ) : null}
        {requirements.conditions.length ? (
          <div className='ct-model-gateway-dispatch-requirement-row'>
            <span>{t('过滤条件')}</span>
            <div className='ct-model-gateway-record-tags'>
              {requirements.conditions.map((condition) => (
                <Tag key={condition} color='orange' type='light' size='small'>
                  {formatDispatchFilterCondition(condition, t)}
                </Tag>
              ))}
            </div>
          </div>
        ) : null}
      </div>
      <Typography.Text type='secondary' size='small'>
        {t('不满足本次工具能力要求的渠道不会进入候选列表')}
      </Typography.Text>
    </div>
  );
}

function formatRuntimeKey(runtimeKey) {
  if (!runtimeKey) return '--';
  if (typeof runtimeKey === 'string') return runtimeKey;
  if (typeof runtimeKey !== 'object') return String(runtimeKey);
  return [
    runtimeKey.requested_model,
    runtimeKey.upstream_model,
    runtimeKey.channel_id ? `#${runtimeKey.channel_id}` : '',
    runtimeKey.account_id,
    runtimeKey.credential_subject_fingerprint
      ? `fp:${runtimeKey.credential_subject_fingerprint}`
      : '',
    runtimeKey.group,
    runtimeKey.endpoint_type,
    runtimeKey.capability_fingerprint,
  ]
    .filter(Boolean)
    .join(' / ');
}

function buildRuntimeKeyParams(runtimeKey = {}) {
  const params = {};
  if (runtimeKey.requested_model) {
    params.requested_model = runtimeKey.requested_model;
  }
  if (runtimeKey.upstream_model) {
    params.upstream_model = runtimeKey.upstream_model;
  }
  if (runtimeKey.resource_id) params.resource_id = runtimeKey.resource_id;
  if (runtimeKey.resource_type) params.resource_type = runtimeKey.resource_type;
  if (runtimeKey.account_id) params.account_id = runtimeKey.account_id;
  if (runtimeKey.account_type) params.account_type = runtimeKey.account_type;
  if (runtimeKey.brand) params.brand = runtimeKey.brand;
  if (runtimeKey.provider) params.provider = runtimeKey.provider;
  if (
    runtimeKey.credential_index !== undefined &&
    runtimeKey.credential_index !== null &&
    runtimeKey.credential_index !== ''
  ) {
    params.credential_index = runtimeKey.credential_index;
  }
  if (runtimeKey.credential_subject_fingerprint) {
    params.credential_subject_fingerprint =
      runtimeKey.credential_subject_fingerprint;
  }
  if (runtimeKey.credential_fingerprint) {
    params.credential_fingerprint = runtimeKey.credential_fingerprint;
  }
  if (runtimeKey.group) params.group = runtimeKey.group;
  if (runtimeKey.endpoint_type) params.endpoint_type = runtimeKey.endpoint_type;
  if (runtimeKey.capability_fingerprint) {
    params.capability_fingerprint = runtimeKey.capability_fingerprint;
  }
  return params;
}

function buildScoreHistorySummary(history, current, previous, t) {
  if (!current) return t('暂无评分记录，无法判断当前情况');
  const delta = Number(history?.score_delta || current?.score_delta || 0);
  const previousScore = Number(previous?.score_total || 0);
  if (!previous || previousScore <= 0) {
    return t('当前稳定评分 {{score}}，暂无上一条可对比记录', {
      score: formatScore(current.score_total),
    });
  }
  const important = scoreHistoryContributionEntries(
    current,
    history,
    delta > 0 ? 'positive' : 'negative',
    t,
  );
  if (scoreDeltaTone(delta) === 'neutral') {
    return t('当前稳定评分 {{score}}，相比上一条基本未变化', {
      score: formatScore(current.score_total),
    });
  }
  const reason = important.length
    ? scoreHistoryContributionReasonText(important, t)
    : t('综合指标变化');
  if (delta > 0) {
    return t(
      '当前稳定评分 {{score}}，比上一条提高 {{delta}}，主要因为 {{reason}}',
      {
        score: formatScore(current.score_total),
        delta: formatScoreDeltaMagnitude(delta),
        reason,
      },
    );
  }
  return t(
    '当前稳定评分 {{score}}，比上一条下降 {{delta}}，主要因为 {{reason}}',
    {
      score: formatScore(current.score_total),
      delta: formatScoreDeltaMagnitude(delta),
      reason,
    },
  );
}

function buildScoreHistoryItemReasons(item, t) {
  const reasons = [];
  const sampleCount = Number(item?.sample_count || 0);
  const scoreDelta = Number(item?.score_delta || 0);
  const selectionReason = formatSelectionReason(item?.selected_reason, t);
  const important = scoreHistoryContributionEntries(
    item,
    {},
    scoreDelta > 0 ? 'positive' : 'negative',
    t,
  );
  if (item?.source === 'runtime_current') {
    reasons.push(t('当前运行时动态评分，来自请求完成后的健康样本'));
    if (Number(item?.ttft_ms || 0) > 0) {
      reasons.push(
        t('当前首包延迟 {{latency}}', {
          latency: formatLatency(item.ttft_ms),
        }),
      );
    }
  } else if (item?.selected) {
    reasons.push(
      selectionReason && selectionReason !== '--'
        ? t('本次被最终选择：{{reason}}', { reason: selectionReason })
        : t('本次被最终选择'),
    );
  } else if (item?.available) {
    reasons.push(
      selectionReason && selectionReason !== '--'
        ? t('本次可用但未最终选择，最终调度原因是 {{reason}}', {
            reason: selectionReason,
          })
        : t('本次可用但未最终选择'),
    );
  } else {
    reasons.push(
      t('本次不可用：{{reason}}', {
        reason:
          formatChannelStatusReason(
            item?.reject_reason || item?.status_reason,
            t,
          ) || t('无过滤原因'),
      }),
    );
  }
  if (sampleCount <= 0) {
    reasons.push(t('当前缺少真实历史样本，评分仅作为探索参考'));
  }
  if (important.length) {
    const reason = scoreHistoryContributionReasonText(important, t);
    if (scoreDelta > 0) {
      reasons.push(t('分数提高主要来自 {{reason}}', { reason }));
    } else if (scoreDelta < 0) {
      reasons.push(t('分数下降主要来自 {{reason}}', { reason }));
    }
  } else if (scoreDeltaTone(scoreDelta) === 'neutral') {
    reasons.push(t('与上一条相比评分未变化'));
  }
  return reasons;
}

function getCandidateChannelLabel(candidate, t) {
  const name = String(candidate?.channel_name || '').trim();
  const id = Number(candidate?.channel_id || 0);
  if (name && id > 0) return `${name} #${id}`;
  if (name) return name;
  if (id > 0) return `#${id}`;
  return t('未知');
}

function getCandidateAccountUID(candidate) {
  const explicit = String(candidate?.credential_uid || '').trim();
  if (explicit) return explicit;
  const subject = String(
    candidate?.credential_subject_fingerprint ||
      candidate?.runtime_key?.credential_subject_fingerprint ||
      '',
  ).trim();
  if (subject)
    return `acct-${subject.length <= 8 ? subject : subject.slice(0, 8)}`;
  const credential = String(
    candidate?.credential_fingerprint ||
      candidate?.runtime_key?.credential_fingerprint ||
      '',
  ).trim();
  if (credential) {
    return `acct-${credential.length <= 8 ? credential : credential.slice(0, 8)}`;
  }
  return '';
}

function getCandidateAccountLabel(candidate) {
  const explicit = String(candidate?.credential_label || '').trim();
  if (explicit) return explicit;
  const uid = getCandidateAccountUID(candidate);
  const brand = String(
    candidate?.brand || candidate?.runtime_key?.brand || '',
  ).trim();
  return [brand, uid].filter(Boolean).join('-');
}

function getCandidateScore(candidate) {
  const score = Number(candidate?.score_total);
  return Number.isFinite(score) && score > 0 ? score : null;
}

function getCandidateRoutingScore(candidate) {
  const score = Number(candidate?.routing_score_total);
  return Number.isFinite(score) && score > 0 ? score : null;
}

function getRecordRoutingScore(record) {
  const score = Number(record?.routing_score_total);
  return Number.isFinite(score) && score > 0 ? score : null;
}

function getCandidateSelectionScore(candidate) {
  return getCandidateRoutingScore(candidate) ?? getCandidateScore(candidate);
}

function candidateSortIndex(candidate, candidates) {
  const index = Array.isArray(candidates)
    ? candidates.findIndex((item) => item === candidate)
    : -1;
  return index >= 0 ? index + 1 : 0;
}

function isAvailableCandidate(candidate) {
  return (
    candidate?.available === true && !isBalanceInsufficientStatus(candidate)
  );
}

function findSelectedCandidate(record, candidates) {
  if (!Array.isArray(candidates) || !candidates.length) return null;
  const selected = candidates.find((candidate) => candidate?.selected === true);
  if (selected) return selected;
  const selectedID = Number(
    record?.actual_channel_id || record?.channel_id || 0,
  );
  if (selectedID > 0) {
    return (
      candidates.find(
        (candidate) => Number(candidate?.channel_id || 0) === selectedID,
      ) || null
    );
  }
  return null;
}

function formatSelectionReason(reason, t) {
  const normalized = String(reason || '').trim();
  if (!normalized) return '--';
  if (normalized === 'weighted_score') return t('本次调度分最高');
  if (normalized === 'cost_first_cheaper_higher_score') {
    return t('成本优先发现明显更低成本且调度分更高的候选');
  }
  if (normalized === 'cost_first_cheaper_speed_acceptable') {
    return t('成本优先发现明显更低成本且速度影响可接受的候选');
  }
  if (normalized === 'weighted_score_sticky_broken') {
    return t('粘滞候选未达保留阈值，改选调度分更高候选');
  }
  if (
    normalized === 'user_sticky_retained' ||
    normalized === 'cache_affinity_retained' ||
    normalized.endsWith('_retained')
  ) {
    return t('粘滞路由保留');
  }
  if (normalized === 'ttft_pending') return t('首包等待降权');
  return normalized;
}

function buildStickyBreakText(reason, t) {
  const label = formatStickyBreakReason(reason, t);
  if (!label) return '';
  return t('原粘滞候选未被保留，原因是 {{reason}}。', {
    reason: label,
  });
}

function formatStickyDecisionReason(reason, t) {
  const normalized = String(reason || '').trim();
  if (!normalized) return '';
  const labels = {
    cost_first_cheaper_speed_acceptable: t('成本更低且速度影响可接受'),
    cost_first_sticky_escape_disabled: t('低成本切换已关闭'),
    cost_first_sticky_escape_sticky_cost_missing: t('粘滞候选成本缺失'),
    cost_first_sticky_escape_sticky_samples_insufficient: t('粘滞候选样本不足'),
    cost_first_sticky_escape_sticky_speed_missing: t('粘滞候选速度样本不足'),
    cost_first_sticky_escape_cost_gap_insufficient: t('成本差不足'),
    cost_first_sticky_escape_candidate_samples_insufficient:
      t('低成本候选样本不足'),
    cost_first_sticky_escape_success_guard_failed: t('成功率保护未通过'),
    cost_first_sticky_escape_candidate_speed_missing:
      t('低成本候选速度样本不足'),
    cost_first_sticky_escape_speed_drop_exceeded: t('速度损失超过阈值'),
    cost_first_sticky_escape_no_candidate: t('没有满足条件的低成本候选'),
  };
  return labels[normalized] || formatTechnicalCode(normalized);
}

function buildStickyDecisionMetrics(decision, t) {
  if (!decision || typeof decision !== 'object') return [];
  const costRatio = Number(decision.cost_ratio);
  const speedDelta = Number(decision.speed_score_delta);
  const successDelta =
    Number(decision.candidate_success_rate) -
    Number(decision.sticky_success_rate);
  return [
    {
      key: 'sticky_escape_decision',
      label: t('粘滞判断'),
      value:
        decision.decision === 'switch'
          ? t('已切换')
          : decision.decision === 'retain'
            ? t('已保留')
            : '--',
    },
    {
      key: 'sticky_escape_reason',
      label: t('判断原因'),
      value: formatStickyDecisionReason(decision.reason, t) || '--',
    },
    {
      key: 'sticky_escape_cost',
      label: t('成本差'),
      value: Number.isFinite(costRatio)
        ? t('低 {{percent}}', {
            percent: formatPercent(Math.max(0, 1 - costRatio), 1),
          })
        : '--',
    },
    {
      key: 'sticky_escape_speed',
      label: t('速度影响'),
      value: Number.isFinite(speedDelta)
        ? `${speedDelta >= 0 ? '+' : ''}${speedDelta.toFixed(3)}`
        : '--',
    },
    {
      key: 'sticky_escape_success',
      label: t('成功率差'),
      value: Number.isFinite(successDelta)
        ? `${successDelta >= 0 ? '+' : ''}${formatPercent(successDelta, 1)}`
        : '--',
    },
    {
      key: 'sticky_escape_samples',
      label: t('样本数'),
      value: `${formatNumber(decision.candidate_sample_count)} / ${formatNumber(
        decision.min_samples,
      )}`,
    },
    {
      key: 'sticky_escape_threshold',
      label: t('切换阈值'),
      value: `${formatRatioValue(decision.cost_threshold)} · ${t('速度')} ${formatScore(
        decision.max_speed_score_drop,
      )}`,
    },
  ];
}

function buildSelectionSummaryText(insight, t) {
  const label = insight.selectedLabel || '--';
  if (insight.stickyRetained) {
    const decisionReason = formatStickyDecisionReason(
      insight.stickyDecision?.reason,
      t,
    );
    if (insight.stickyDecision?.decision === 'retain' && decisionReason) {
      return t('选择 {{channel}}：成本优先低成本切换未触发，{{reason}}。', {
        channel: label,
        reason: decisionReason,
      });
    }
    return t('选择 {{channel}}：命中粘滞路由且质量仍满足保留阈值。', {
      channel: label,
    });
  }
  if (insight.stickyBroken) {
    return t(
      '选择 {{channel}}：原粘滞渠道未满足保留条件，已切到当前更优候选。',
      {
        channel: label,
      },
    );
  }
  if (insight.selectedTopTie && insight.topTieCount > 1) {
    return t(
      '选择 {{channel}}：多个候选调度分并列最高，按候选顺序保留先出现渠道。',
      {
        channel: label,
      },
    );
  }
  if (insight.selectedTopTie) {
    return t('选择 {{channel}}：它是当前可用候选里调度分最高的渠道。', {
      channel: label,
    });
  }
  return t('选择 {{channel}}：系统从当前可用候选中完成调度。', {
    channel: label,
  });
}

function buildCandidateDecisionText(candidate, candidates, record, t) {
  if (!candidate) return '';
  const selected = candidate?.selected === true;
  const available = isAvailableCandidate(candidate);
  const channel = getCandidateChannelLabel(candidate, t);
  const rank = candidateSortIndex(candidate, candidates);
  const rankText = rank ? t('候选顺序第 {{rank}} 位', { rank }) : '';
  const candidateSelectionScore = getCandidateSelectionScore(candidate);
  const selectedCandidate = findSelectedCandidate(record, candidates);
  const selectedSelectionScore = getCandidateSelectionScore(selectedCandidate);
  const selectedLabel = selectedCandidate
    ? getCandidateChannelLabel(selectedCandidate, t)
    : '';
  const selectedReason = formatSelectionReason(record?.selected_reason, t);
  const scoreDeltaEpsilon = 0.000001;
  if (selected) {
    if (candidateSelectionScore !== null) {
      return rankText
        ? t(
            '{{channel}} 是本次最终选择，本次调度分 {{score}}，{{rankText}}。',
            {
              channel,
              score: formatScore(candidateSelectionScore),
              rankText,
            },
          )
        : t('{{channel}} 是本次最终选择，本次调度分 {{score}}。', {
            channel,
            score: formatScore(candidateSelectionScore),
          });
    }
    return rankText
      ? t('{{channel}} 是本次最终选择，{{rankText}}。', {
          channel,
          rankText,
        })
      : t('{{channel}} 是本次最终选择。', { channel });
  }
  if (!available) {
    const reason =
      formatChannelStatusReason(candidate?.reject_reason, t) ||
      formatChannelStatusReason(candidate?.status_reason, t) ||
      (isBalanceInsufficientStatus(candidate)
        ? t('余额不足')
        : t('无过滤原因'));
    const normalizedReason = String(candidate?.reject_reason || '')
      .trim()
      .toLowerCase();
    if (
      normalizedReason === 'already_failed_in_request' ||
      normalizedReason === 'routing_slot_reserved'
    ) {
      return t(
        '{{channel}} 本次未参与排序：首字超时或上游错误后内部切换，避免本次请求再次选择同一渠道。',
        { channel },
      );
    }
    return t('{{channel}} 本次未进入可用候选，原因是 {{reason}}。', {
      channel,
      reason,
    });
  }
  if (candidate?.selection_skip_reason === 'concurrency_saturated') {
    const activeConcurrency = Number(candidate?.active_concurrency || 0);
    const effectiveConcurrency = Number(
      candidate?.effective_concurrency_limit ||
        candidate?.max_concurrency ||
        candidate?.learned_concurrency_limit ||
        candidate?.configured_concurrency_limit ||
        0,
    );
    return t(
      '{{channel}} 本次可用但未选中，原因是生效并发已满（{{concurrency}}），系统优先选择仍有余量的候选。',
      {
        channel,
        concurrency:
          effectiveConcurrency > 0
            ? `${formatNumber(activeConcurrency)} / ${formatNumber(
                effectiveConcurrency,
              )}`
            : formatNumber(activeConcurrency),
      },
    );
  }
  if (
    candidate?.selection_skip_reason === 'already_failed_in_request' ||
    candidate?.selection_skip_reason === 'routing_slot_reserved'
  ) {
    return t(
      '{{channel}} 本次不参与排序：首字超时或上游错误后内部切换，避免本次请求再次选择同一渠道。',
      { channel },
    );
  }
  if (candidateSelectionScore !== null && selectedSelectionScore !== null) {
    if (candidateSelectionScore + scoreDeltaEpsilon < selectedSelectionScore) {
      return t(
        '{{channel}} 本次可用但未选中，本次调度分 {{score}} 低于最终选择 {{selectedChannel}} 的 {{selectedScore}}。最终选择依据是 {{reason}}。',
        {
          channel,
          score: formatScore(candidateSelectionScore),
          selectedChannel: selectedLabel || t('已选渠道'),
          selectedScore: formatScore(selectedSelectionScore),
          reason: selectedReason || t('当前策略'),
        },
      );
    }
    if (candidateSelectionScore > selectedSelectionScore + scoreDeltaEpsilon) {
      return t(
        '{{channel}} 本次可用但未选中，虽然本次调度分 {{score}} 更高，但系统仍按 {{reason}} 选择了 {{selectedChannel}}。',
        {
          channel,
          score: formatScore(candidateSelectionScore),
          selectedChannel: selectedLabel || t('已选渠道'),
          reason: selectedReason || t('当前策略'),
        },
      );
    }
    return t(
      '{{channel}} 本次可用但未选中，本次调度分与最终选择 {{selectedChannel}} 基本一致，最终按 {{reason}} 保留。',
      {
        channel,
        selectedChannel: selectedLabel || t('已选渠道'),
        reason: selectedReason || t('当前策略'),
      },
    );
  }
  return t('{{channel}} 本次可用但未选中，最终选择依据是 {{reason}}。', {
    channel,
    reason: selectedReason || t('当前策略'),
  });
}

function buildRejectReasonSummary(candidates, t) {
  const counts = new Map();
  (candidates || []).forEach((candidate) => {
    if (isAvailableCandidate(candidate)) return;
    const reason =
      formatChannelStatusReason(candidate?.reject_reason, t) ||
      formatChannelStatusReason(candidate?.status_reason, t) ||
      (isBalanceInsufficientStatus(candidate)
        ? t('余额不足')
        : t('无过滤原因'));
    counts.set(reason, (counts.get(reason) || 0) + 1);
  });
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 3)
    .map(([reason, count]) => `${reason} ${formatNumber(count)}`);
}

function buildSelectionInsight(record, candidates, t) {
  const selectedCandidate = findSelectedCandidate(record, candidates);
  const selectedIndex = selectedCandidate
    ? (candidates || []).findIndex(
        (candidate) => candidate === selectedCandidate,
      )
    : -1;
  const availableCandidates = (candidates || []).filter(isAvailableCandidate);
  const selectedScore =
    getCandidateSelectionScore(selectedCandidate) ??
    getRecordRoutingScore(record) ??
    getCandidateScore(record);
  const scoredCandidates = availableCandidates.filter(
    (candidate) => getCandidateSelectionScore(candidate) !== null,
  );
  const scores = scoredCandidates.map((candidate) =>
    getCandidateSelectionScore(candidate),
  );
  const maxScore = scores.length ? Math.max(...scores) : null;
  const sameScore = (left, right) =>
    left !== null && right !== null && Math.abs(left - right) < 0.000001;
  const selectedRank =
    selectedScore !== null
      ? scoredCandidates.filter(
          (candidate) =>
            (getCandidateSelectionScore(candidate) || 0) > selectedScore,
        ).length + 1
      : null;
  const topTieCount =
    maxScore !== null
      ? scoredCandidates.filter((candidate) =>
          sameScore(getCandidateSelectionScore(candidate), maxScore),
        ).length
      : 0;
  const selectedTopTie =
    selectedScore !== null &&
    maxScore !== null &&
    sameScore(selectedScore, maxScore);
  const filteredCount = Math.max(
    0,
    Number(candidates?.length || 0) - availableCandidates.length,
  );
  const rawReason = String(record?.selected_reason || '').trim();
  const reasonLabel = formatSelectionReason(rawReason, t);
  const selectedLabel = selectedCandidate
    ? getCandidateChannelLabel(selectedCandidate, t)
    : record?.channel_name ||
      (record?.channel_id ? `#${record.channel_id}` : '--');
  const selectedSamples = Number(selectedCandidate?.sample_count || 0);
  const selectedHasRealSamples = selectedSamples > 0;
  const selectedAllScoreItems = normalizeScoreItemsForDisplay(
    selectedCandidate?.score_items,
  );
  const selectedTtftItem = scoreItemByKey(
    selectedAllScoreItems,
    'ttft_latency',
  );
  const selectedDurationItem = scoreItemByKey(
    selectedAllScoreItems,
    'duration_latency',
  );
  const selectedScoreItems = selectedAllScoreItems.filter(
    (item) => item.weight > 0 && !item.missing_reason,
  );
  const activeConcurrency = Number(
    selectedCandidate?.active_concurrency || record?.active_concurrency || 0,
  );
  const effectiveConcurrency = Number(
    selectedCandidate?.effective_concurrency_limit ||
      selectedCandidate?.max_concurrency ||
      record?.configured_concurrency_limit ||
      0,
  );
  const configuredConcurrency = Number(
    selectedCandidate?.configured_concurrency_limit ||
      record?.configured_concurrency_limit ||
      effectiveConcurrency ||
      0,
  );
  const stickyRetained =
    record?.sticky_retained === true || rawReason.endsWith('_retained');
  const stickyBroken =
    Boolean(record?.sticky_break) ||
    rawReason === 'weighted_score_sticky_broken';
  const stickyBreakText = stickyBroken
    ? buildStickyBreakText(record?.sticky_break, t)
    : '';
  const stickyDecisionMetrics = buildStickyDecisionMetrics(
    record?.sticky_decision || record?.request_meta?.sticky_decision,
    t,
  );
  let explanation = t('根据当前策略从可用候选中完成选择');

  if (selectedTopTie && topTieCount > 1 && rawReason === 'weighted_score') {
    explanation = t('本次调度分并列最高，按候选顺序保留先出现候选');
  } else if (selectedTopTie && rawReason === 'weighted_score') {
    explanation = t('在可用候选中本次调度分最高');
  } else if (stickyRetained) {
    explanation = t('命中粘滞路由并满足保留阈值，优先复用该渠道');
  } else if (stickyBroken) {
    explanation = t('原粘滞候选未满足保留条件，改选当前调度分更优候选');
    if (record?.sticky_break === 'cost_first_cheaper_speed_acceptable') {
      explanation = t('成本优先下命中低成本候选，且速度评分影响在阈值内');
    }
  }

  const primaryMetricKeys = new Set([
    'score',
    'routing_score',
    'latency',
    'duration',
    'rank',
    'concurrency',
  ]);
  const metrics = [
    {
      key: 'score',
      label: selectedHasRealSamples ? t('稳定评分') : t('探索参考'),
      value: selectedHasRealSamples
        ? formatScore(selectedScore)
        : t('暂无评分样本'),
    },
    {
      key: 'routing_score',
      label: t('本次调度评分'),
      value: selectedHasRealSamples
        ? formatScore(getCandidateRoutingScore(selectedCandidate))
        : '--',
    },
    {
      key: 'latency',
      label: t('评分首包'),
      value: formatScoreItemRawValue(selectedTtftItem, t),
    },
    {
      key: 'duration',
      label: t('评分耗时'),
      value: formatScoreItemRawValue(selectedDurationItem, t),
    },
    {
      key: 'rank',
      label: t('候选排名'),
      value:
        selectedRank && availableCandidates.length
          ? `${formatNumber(selectedRank)} / ${formatNumber(
              availableCandidates.length,
            )}`
          : '--',
    },
    {
      key: 'order',
      label: t('候选顺序'),
      value:
        selectedIndex >= 0
          ? `${formatNumber(selectedIndex + 1)} / ${formatNumber(
              candidates?.length || 0,
            )}`
          : '--',
    },
    {
      key: 'tie',
      label: t('并列最高'),
      value:
        selectedTopTie && topTieCount > 1 ? formatNumber(topTieCount) : '--',
    },
    {
      key: 'available',
      label: t('可用候选'),
      value: `${formatNumber(availableCandidates.length)} / ${formatNumber(
        candidates?.length || 0,
      )}`,
    },
    {
      key: 'filtered',
      label: t('过滤候选'),
      value: formatNumber(filteredCount),
    },
    {
      key: 'concurrency',
      label: t('生效并发'),
      value:
        activeConcurrency > 0 || effectiveConcurrency > 0
          ? `${formatNumber(activeConcurrency)} / ${
              effectiveConcurrency > 0
                ? formatNumber(effectiveConcurrency)
                : '--'
            }`
          : '--',
    },
    {
      key: 'sample_source',
      label: t('样本来源'),
      value: formatScoreSampleSource(selectedCandidate?.score_sample_source, t),
    },
    {
      key: 'samples',
      label: t('评分样本'),
      value:
        selectedSamples > 0 ? formatNumber(selectedSamples) : t('暂无评分样本'),
    },
    ...stickyDecisionMetrics,
  ];

  return {
    selectedCandidate,
    selectedLabel,
    reasonLabel,
    rawReason,
    explanation,
    stickyRetained,
    stickyBroken,
    stickyDecision:
      record?.sticky_decision || record?.request_meta?.sticky_decision,
    stickySource: formatStickySource(record?.sticky_source, t),
    stickyBreakText,
    selectedRank,
    selectedTopTie,
    topTieCount,
    summary: '',
    metrics,
    primaryMetrics: metrics.filter((metric) =>
      primaryMetricKeys.has(metric.key),
    ),
    secondaryMetrics: metrics.filter(
      (metric) => !primaryMetricKeys.has(metric.key),
    ),
    scoreEntries: Object.entries(
      selectedCandidate?.score_breakdown || record?.score_breakdown || {},
    ).filter(
      ([key, score]) =>
        Number.isFinite(Number(score)) &&
        scoreEntryIsVisible(key, selectedSamples),
    ),
    scoreItems: selectedScoreItems,
    rejectReasons: buildRejectReasonSummary(candidates, t),
  };
}

function SelectionInsightPanel({ record, candidates, t }) {
  const rawInsight = buildSelectionInsight(record, candidates, t);
  const insight = {
    ...rawInsight,
    summary: buildSelectionSummaryText(rawInsight, t),
  };
  const hasRawCode =
    insight.rawReason &&
    insight.reasonLabel === insight.rawReason &&
    insight.rawReason !== '--';

  return (
    <section>
      <Typography.Title heading={6}>{t('选择依据')}</Typography.Title>
      <div className='ct-model-gateway-selection-insight'>
        <div className='ct-model-gateway-selection-insight-head'>
          <div className='ct-model-gateway-selection-insight-title'>
            <Info size={16} />
            <span>{t('最终选择')}</span>
          </div>
          <Tag color='green' type='solid' shape='circle'>
            {insight.selectedLabel}
          </Tag>
        </div>
        <div className='ct-model-gateway-selection-insight-copy'>
          <Typography.Text strong>{insight.summary}</Typography.Text>
          <Typography.Text type='secondary'>
            {insight.explanation}
          </Typography.Text>
          {insight.stickyBreakText ? (
            <Typography.Text type='secondary'>
              {insight.stickyBreakText}
            </Typography.Text>
          ) : null}
          <div className='ct-model-gateway-selection-insight-chips'>
            <Tag color='cyan' type='light' size='small'>
              {t('选择方式')}: {insight.reasonLabel}
            </Tag>
            {record?.sticky_source ? (
              <Tag color='blue' type='light' size='small'>
                {t('粘滞来源')}: {insight.stickySource}
              </Tag>
            ) : null}
            {hasRawCode ? (
              <Tag color='grey' type='light' size='small'>
                {t('排障码')}: {formatTechnicalCode(insight.rawReason)}
              </Tag>
            ) : null}
          </div>
        </div>
        <div className='ct-model-gateway-selection-insight-grid'>
          {insight.primaryMetrics.map((metric) => (
            <div
              key={metric.key}
              className='ct-model-gateway-selection-insight-item'
            >
              <span>{metric.label}</span>
              <strong>{metric.value}</strong>
            </div>
          ))}
        </div>
        {insight.rejectReasons.length ? (
          <div className='ct-model-gateway-selection-insight-rejects'>
            <Typography.Text type='tertiary' size='small'>
              {t('主要过滤原因')}:
            </Typography.Text>
            <div className='ct-model-gateway-record-tags'>
              {insight.rejectReasons.map((reason) => (
                <Tag key={reason} color='orange' type='light' size='small'>
                  {reason}
                </Tag>
              ))}
            </div>
          </div>
        ) : null}
        {insight.secondaryMetrics.length ||
        insight.scoreItems.length ||
        insight.scoreEntries.length ? (
          <DetailAccordion
            title={t('选择补充指标')}
            meta={t('指标 {{count}} 项', {
              count:
                insight.secondaryMetrics.length +
                (insight.scoreItems.length || insight.scoreEntries.length),
            })}
            className='ct-model-gateway-inline-accordion'
          >
            <div className='ct-model-gateway-selection-insight-grid'>
              {insight.secondaryMetrics.map((metric) => (
                <div
                  key={metric.key}
                  className='ct-model-gateway-selection-insight-item'
                >
                  <span>{metric.label}</span>
                  <strong>{metric.value}</strong>
                </div>
              ))}
            </div>
            {insight.scoreItems.length ? (
              <ScoreItemsTable items={insight.scoreItems} t={t} compact />
            ) : insight.scoreEntries.length ? (
              <div className='ct-model-gateway-score-list'>
                {insight.scoreEntries.map(([key, value]) => (
                  <Tag key={key} color='cyan' type='light' size='small'>
                    {scoreMetricLabel(key, t)}: {formatScore(value)}
                  </Tag>
                ))}
              </div>
            ) : null}
          </DetailAccordion>
        ) : null}
      </div>
    </section>
  );
}

function formatScoreSampleSource(source, t) {
  const normalized = String(source || '').trim();
  if (normalized === 'exact') return t('精确运行样本');
  if (normalized === 'similar') return t('同渠道历史样本');
  if (normalized === 'none') return t('暂无评分样本');
  return normalized || t('暂无评分样本');
}

function scoreEntryIsVisible(key, sampleCount) {
  const normalized = String(key || '').trim();
  return normalized !== '';
}

function scoreMetricIsHidden(key) {
  return String(key || '').trim() === '';
}

function CandidateExplanationCard({
  candidate,
  candidates,
  index,
  record,
  onOpenScoreHistory,
  onRuntimeCircuitCleared,
  t,
}) {
  const [clearingCircuit, setClearingCircuit] = useState(false);
  const [recoveringHealth, setRecoveringHealth] = useState(false);
  const sampleCount = Number(candidate?.sample_count || 0);
  const hasRealSamples = sampleCount > 0;
  const allScoreItems = normalizeScoreItemsForDisplay(candidate?.score_items);
  const allRoutingScoreItems = normalizeScoreItemsForDisplay(
    candidate?.routing_score_items,
  );
  const ttftScoreItem = scoreItemByKey(allScoreItems, 'ttft_latency');
  const durationScoreItem = scoreItemByKey(allScoreItems, 'duration_latency');
  const costScoreItem = scoreItemByKey(allScoreItems, 'cost');
  const scoreEntries = Object.entries(candidate?.score_breakdown || {}).filter(
    ([key, score]) =>
      Number.isFinite(Number(score)) && scoreEntryIsVisible(key, sampleCount),
  );
  const scoreMetricEntries = [
    ...allScoreItems
      .filter((item) => item.weight > 0 && !item.missing_reason)
      .slice(0, 5)
      .map((item) => [item.key, scoreItemLabel(item, t), item.score]),
  ].filter(([, , value]) => Number(value) > 0);
  const routingScore = Number(candidate?.routing_score_total || 0);
  const unavailableReason = String(candidate?.reject_reason || '')
    .trim()
    .toLowerCase();
  const stateTags = Array.isArray(candidate?.state_tags)
    ? candidate.state_tags
    : [];
  const circuitState = String(candidate?.circuit_state || '')
    .trim()
    .toLowerCase();
  const circuitOpen =
    candidate?.circuit_open === true ||
    unavailableReason === 'circuit_open' ||
    stateTags.includes('circuit_open') ||
    circuitState === 'open';
  const circuitHalfOpen =
    circuitState === 'half_open' ||
    candidate?.probe_trigger_reason === 'circuit_half_open';
  const circuitActive = circuitOpen || circuitHalfOpen;
  const circuitOpenReason = String(candidate?.circuit_open_reason || '').trim();
  const circuitOpenUntil = normalizeTimestamp(candidate?.circuit_open_until);
  const circuitProbeUsed = Number(candidate?.circuit_half_open_probe_used || 0);
  const circuitProbeMax = Number(candidate?.circuit_half_open_probe_max || 0);
  const routingScoreItems = allRoutingScoreItems.length
    ? allRoutingScoreItems
    : allScoreItems;
  const available = candidate?.available === true;
  const unavailable = candidate?.available === false;
  const selected = candidate?.selected === true;
  const stickyMatched = candidate?.sticky_matched === true;
  const stickyKnown = typeof candidate?.sticky_matched === 'boolean';
  const balanceInsufficient = isBalanceInsufficientStatus(candidate);
  const statusReason = formatChannelStatusReason(candidate?.status_reason, t);
  const fullChannelLabel = getCandidateChannelLabel(candidate, t);
  const channelLabel =
    String(candidate?.channel_name || '').trim() ||
    (candidate?.channel_id ? `#${candidate.channel_id}` : t('未知'));
  const activeConcurrency = Number(candidate?.active_concurrency || 0);
  const effectiveConcurrency = Number(
    candidate?.effective_concurrency_limit || candidate?.max_concurrency || 0,
  );
  const configuredConcurrency = Number(
    candidate?.configured_concurrency_limit || 0,
  );
  const learnedConcurrency = Number(candidate?.learned_concurrency_limit || 0);
  const displayConcurrencyLimit =
    effectiveConcurrency || learnedConcurrency || configuredConcurrency;
  const firstBytePending = Number(candidate?.first_byte_pending || 0);
  const slowFirstBytePending = Number(candidate?.slow_first_byte_pending || 0);
  const oldestFirstByteWaitMs = Number(
    candidate?.oldest_first_byte_wait_ms || 0,
  );
  const emptyOutputRate = Number(candidate?.empty_output_rate || 0);
  const issueRate = Number(candidate?.experience_issue_rate || 0);
  const accountLabel = getCandidateAccountUID(candidate);
  const credentialLabel =
    getCandidateAccountLabel(candidate) ||
    candidate?.account_id ||
    candidate?.runtime_key?.account_id ||
    candidate?.credential_subject_fingerprint ||
    candidate?.runtime_key?.credential_subject_fingerprint ||
    candidate?.credential_fingerprint ||
    candidate?.runtime_key?.credential_fingerprint ||
    '';
  const poolLevel = candidate?.pool_level || candidate?.runtime_key?.pool_level;
  const referenceCost =
    formatCostScoreItemSummary(costScoreItem, t) ||
    formatCandidateReferenceCost(candidate, t);
  const decisionText = buildCandidateDecisionText(
    candidate,
    candidates,
    record,
    t,
  );
  const timeoutRecovery =
    unavailableReason === 'timeout_recovery' ||
    candidate?.probe_trigger_reason === 'timeout_recovery' ||
    stateTags.includes('timeout_recovery');
  const scoreAnomalyRecovery =
    candidate?.probe_trigger_reason === 'score_anomaly_fast_probe' ||
    candidate?.probe_recovery_phase === 'fast_probe' ||
    candidate?.probe_recovery_phase === 'pending_real_confirmation' ||
    stateTags.includes('score_anomaly_fast_probe');
  const alreadyFailedInRequest =
    unavailableReason === 'already_failed_in_request' ||
    unavailableReason === 'routing_slot_reserved';
  const recoverySuccessCount = Number(
    candidate?.probe_recovery_success_count || 0,
  );
  const recoveryRequired = Number(candidate?.probe_recovery_required || 0);
  const fastRecoveryAttempts = Number(
    candidate?.probe_fast_recovery_attempts || 0,
  );
  const recoverableQualityScore = Number(
    candidate?.recoverable_quality_score || 0,
  );
  const recoverableQualityBaseline = Number(
    candidate?.recoverable_quality_baseline || 0,
  );
  const recoverableQualityDrop = Number(
    candidate?.recoverable_quality_drop_ratio || 0,
  );
  const candidateSummaryMetrics = [
    {
      key: 'ttft',
      label: t('评分首包'),
      value: formatScoreItemRawValue(ttftScoreItem, t),
    },
    {
      key: 'duration',
      label: t('评分耗时'),
      value: formatScoreItemRawValue(durationScoreItem, t),
    },
    {
      key: 'concurrency',
      label: t('生效并发'),
      value:
        activeConcurrency > 0 || displayConcurrencyLimit > 0
          ? `${formatNumber(activeConcurrency)} / ${
              displayConcurrencyLimit > 0
                ? formatNumber(displayConcurrencyLimit)
                : '--'
            }`
          : '--',
    },
    {
      key: 'cost',
      label: t('成本参考'),
      value: referenceCost || '--',
    },
    {
      key: 'routing_score',
      label: t('本次调度评分'),
      value: routingScore > 0 ? formatScore(routingScore) : '--',
    },
  ];

  const clearRuntimeCircuit = async () => {
    const channelID = Number(
      candidate?.channel_id || candidate?.runtime_key?.channel_id || 0,
    );
    if (!channelID || clearingCircuit) return;
    setClearingCircuit(true);
    try {
      const res = await API.post(
        '/api/model_gateway/observability/runtime/clear_circuit',
        {
          channel_id: channelID,
          runtime_key: candidate?.runtime_key || { channel_id: channelID },
          clear_failure_avoidance: true,
        },
      );
      if (res?.data?.success) {
        Toast.success(t('熔断已恢复'));
        onRuntimeCircuitCleared?.();
      } else {
        showError(res?.data?.message || t('恢复熔断失败'));
      }
    } catch (error) {
      showError(error);
    } finally {
      setClearingCircuit(false);
    }
  };

  const recoverChannelHealth = async () => {
    const channelID = Number(
      candidate?.channel_id || candidate?.runtime_key?.channel_id || 0,
    );
    if (!channelID || recoveringHealth) return;
    setRecoveringHealth(true);
    try {
      const res = await API.post(`/api/channel/${channelID}/recover_health`);
      if (res?.data?.success) {
        Toast.success(t('渠道健康状态已恢复'));
        onRuntimeCircuitCleared?.();
      } else {
        showError(res?.data?.message || t('恢复健康失败'));
      }
    } catch (error) {
      showError(error);
    } finally {
      setRecoveringHealth(false);
    }
  };

  return (
    <div
      className={`ct-model-gateway-candidate-card${
        selected ? ' ct-model-gateway-candidate-card-selected' : ''
      }${
        balanceInsufficient
          ? ' ct-model-gateway-candidate-card-balance-warning'
          : ''
      }`}
    >
      <div className='ct-model-gateway-candidate-head'>
        <div className='ct-model-gateway-candidate-title'>
          <span title={fullChannelLabel}>{channelLabel}</span>
          {candidate?.channel_id ? (
            <Typography.Text type='tertiary' size='small'>
              #{candidate.channel_id}
            </Typography.Text>
          ) : null}
        </div>
        <div className='ct-model-gateway-record-tags'>
          {selected && (
            <Tag color='green' type='solid' size='small'>
              {t('最终选择')}
            </Tag>
          )}
          {balanceInsufficient && (
            <Tooltip
              content={statusReason || t('渠道余额不足，已暂停调度')}
              position='top'
            >
              <Tag color='red' type='light' size='small'>
                {t('余额不足')}
              </Tag>
            </Tooltip>
          )}
          {balanceInsufficient ? (
            <Button
              icon={<RotateCcw size={13} />}
              size='small'
              theme='borderless'
              type='tertiary'
              loading={recoveringHealth}
              onClick={recoverChannelHealth}
            >
              {t('恢复健康')}
            </Button>
          ) : null}
          {available && !balanceInsufficient && (
            <Tag color='green' type='light' size='small'>
              {t('可用')}
            </Tag>
          )}
          {(unavailable || balanceInsufficient) && (
            <Tag color='red' type='light' size='small'>
              {t('不可用')}
            </Tag>
          )}
          {alreadyFailedInRequest ? (
            <Tag color='orange' type='light' size='small'>
              {t('不参与本次排序')}
            </Tag>
          ) : null}
          {timeoutRecovery ? (
            <Tag color='orange' type='light' size='small'>
              {t('等待恢复探活')}
            </Tag>
          ) : null}
          {scoreAnomalyRecovery ? (
            <Tag color='orange' type='light' size='small'>
              {candidate?.probe_recovery_phase === 'pending_real_confirmation'
                ? t('待真实请求确认')
                : t('快速校准中')}
            </Tag>
          ) : null}
          {circuitOpen ? (
            <Tag color='red' type='light' size='small'>
              {t('熔断打开')}
            </Tag>
          ) : null}
          {circuitHalfOpen ? (
            <Tag color='orange' type='light' size='small'>
              {t('半开探测')}
            </Tag>
          ) : null}
          {circuitHalfOpen && circuitProbeMax > 0 ? (
            <Tag color='cyan' type='light' size='small'>
              {t('探针 {{count}}/{{required}}', {
                count: circuitProbeUsed,
                required: circuitProbeMax,
              })}
            </Tag>
          ) : null}
          {circuitActive ? (
            <Button
              icon={<RotateCcw size={13} />}
              size='small'
              theme='borderless'
              type='tertiary'
              loading={clearingCircuit}
              onClick={clearRuntimeCircuit}
            >
              {t('恢复')}
            </Button>
          ) : null}
          {timeoutRecovery && recoveryRequired > 0 ? (
            <Tag color='cyan' type='light' size='small'>
              {t('恢复样本 {{count}}/{{required}}', {
                count: recoverySuccessCount,
                required: recoveryRequired,
              })}
            </Tag>
          ) : null}
          {scoreAnomalyRecovery ? (
            <Tag color='cyan' type='light' size='small'>
              {t('快速探活 {{count}}/5', {
                count: fastRecoveryAttempts,
              })}
            </Tag>
          ) : null}
          {!selected && !available && !unavailable && (
            <Tag color='grey' type='light' size='small'>
              #{index + 1}
            </Tag>
          )}
          {poolLevel ? (
            <Tag color='cyan' type='light' size='small'>
              {poolLevel}
            </Tag>
          ) : null}
        </div>
      </div>

      {decisionText ? (
        <div className='ct-model-gateway-candidate-decision'>
          <Info size={13} />
          <span>{decisionText}</span>
        </div>
      ) : null}

      <div className='ct-model-gateway-candidate-meta ct-model-gateway-candidate-meta-summary'>
        <span>
          {t('分组')}: {candidate?.group || '--'}
        </span>
        <span>
          {t('上游模型')}: {candidate?.upstream_model || '--'}
        </span>
        {stickyKnown && (
          <span>
            {t('粘滞匹配')}: {stickyMatched ? t('已匹配') : t('未匹配')}
          </span>
        )}
        {candidate?.brand || candidate?.provider ? (
          <span>
            {t('品牌')}: {candidate?.brand || candidate?.provider}
          </span>
        ) : null}
        {accountLabel ? (
          <span title={accountLabel}>
            {t('账号')}: {shortenText(accountLabel, 18)}
          </span>
        ) : null}
        {credentialLabel ? (
          <span title={credentialLabel}>
            {t('凭证')}: {shortenText(credentialLabel, 18)}
          </span>
        ) : null}
      </div>

      <div className='ct-model-gateway-candidate-dynamic-grid'>
        {candidateSummaryMetrics.map((metric) => (
          <div key={metric.key}>
            <span>{metric.label}</span>
            <strong>{metric.value}</strong>
          </div>
        ))}
      </div>

      <RoutingScoreItemsPanel
        items={routingScoreItems}
        total={routingScore || candidate?.score_total}
        t={t}
      />

      {!available && (
        <Typography.Text
          type={candidate?.reject_reason ? 'danger' : 'tertiary'}
          size='small'
          ellipsis={{ showTooltip: true }}
        >
          {t('过滤原因')}:{' '}
          {formatChannelStatusReason(candidate?.reject_reason, t) ||
            t('无过滤原因')}
        </Typography.Text>
      )}
      {available &&
        !selected &&
        (candidate?.selection_skip_reason ||
          candidate?.reject_reason ||
          candidate?.status_reason) && (
          <Typography.Text
            type='tertiary'
            size='small'
            ellipsis={{ showTooltip: true }}
          >
            {t('未选中原因')}:{' '}
            {formatChannelStatusReason(
              candidate?.selection_skip_reason ||
                candidate?.reject_reason ||
                candidate?.status_reason,
              t,
            ) || t('当前策略')}
          </Typography.Text>
        )}
      <DetailAccordion
        title={t('技术详情')}
        meta={t('运行键、评分与等待状态')}
        className='ct-model-gateway-inline-accordion ct-model-gateway-candidate-tech-accordion'
      >
        <div className='ct-model-gateway-candidate-meta'>
          <span>
            {t('提供商画像')}: {candidate?.provider_profile || '--'}
          </span>
          <span>
            {t('代理模式')}: {candidate?.proxy_mode || '--'}
          </span>
          <span>
            {t('运行键')}: {formatRuntimeKey(candidate?.runtime_key)}
          </span>
        </div>
        <div className='ct-model-gateway-candidate-dynamic-grid'>
          <div>
            <span>{t('评分样本')}</span>
            <strong>
              {sampleCount > 0 ? formatNumber(sampleCount) : t('暂无评分样本')}
            </strong>
          </div>
          <div>
            <span>{t('首包等待')}</span>
            <strong>
              {firstBytePending > 0
                ? `${formatNumber(firstBytePending)}${
                    slowFirstBytePending > 0
                      ? ` / ${formatNumber(slowFirstBytePending)} ${t('慢')}`
                      : ''
                  }`
                : '--'}
            </strong>
          </div>
          <div>
            <span>{t('最长等待')}</span>
            <strong>
              {oldestFirstByteWaitMs > 0
                ? formatLatency(oldestFirstByteWaitMs)
                : '--'}
            </strong>
          </div>
          <div>
            <span>{t('配置上限')}</span>
            <strong>
              {configuredConcurrency > 0
                ? formatNumber(configuredConcurrency)
                : '--'}
            </strong>
          </div>
          <div>
            <span>{t('学习上限')}</span>
            <strong>
              {learnedConcurrency > 0 ? formatNumber(learnedConcurrency) : '--'}
            </strong>
          </div>
          <div>
            <span>{t('样本来源')}</span>
            <strong>
              {formatScoreSampleSource(candidate?.score_sample_source, t)}
            </strong>
          </div>
          <div>
            <span>{t('质量分')}</span>
            <strong>
              {recoverableQualityScore > 0
                ? formatScore(recoverableQualityScore)
                : '--'}
            </strong>
          </div>
          <div>
            <span>{t('质量基线')}</span>
            <strong>
              {recoverableQualityBaseline > 0
                ? formatScore(recoverableQualityBaseline)
                : '--'}
            </strong>
          </div>
          <div>
            <span>{t('相对下降')}</span>
            <strong>
              {recoverableQualityDrop > 0
                ? formatPercent(recoverableQualityDrop)
                : '--'}
            </strong>
          </div>
          <div>
            <span>{t('熔断状态')}</span>
            <strong>
              {circuitHalfOpen
                ? t('半开探测')
                : circuitOpen
                  ? t('熔断打开')
                  : t('正常')}
            </strong>
          </div>
          <div>
            <span>{t('熔断原因')}</span>
            <strong>
              {circuitOpenReason
                ? formatCircuitErrorType(circuitOpenReason, t)
                : '--'}
            </strong>
          </div>
          <div>
            <span>{t('预计恢复')}</span>
            <strong>
              {circuitOpenUntil ? formatTimestamp(circuitOpenUntil) : '--'}
            </strong>
          </div>
          <div>
            <span>{t('半开探针')}</span>
            <strong>
              {circuitProbeMax > 0
                ? `${formatNumber(circuitProbeUsed)} / ${formatNumber(
                    circuitProbeMax,
                  )}`
                : '--'}
            </strong>
          </div>
        </div>
        <div className='ct-model-gateway-candidate-score-row'>
          <Tooltip content={t('查看评分变更记录')}>
            <Tag
              className='ct-model-gateway-score-trigger'
              color='cyan'
              type='light'
              size='small'
              shape='circle'
              onClick={() => onOpenScoreHistory?.(candidate)}
            >
              {hasRealSamples ? t('稳定评分') : t('探索参考')}:{' '}
              {hasRealSamples
                ? formatScore(candidate?.score_total)
                : t('暂无评分样本')}
            </Tag>
          </Tooltip>
          {routingScore > 0 ? (
            <Tooltip
              content={t(
                '本次调度评分会叠加当前并发、排队和首包等待，只用于本次选路',
              )}
            >
              <Tag color='grey' type='light' size='small' shape='circle'>
                {t('本次调度评分')}: {formatScore(routingScore)}
              </Tag>
            </Tooltip>
          ) : null}
          <div className='ct-model-gateway-score-list'>
            {scoreMetricEntries.length ? (
              scoreMetricEntries.map(([key, label, value]) => (
                <Tooltip key={key} content={scoreMetricDescription(key, t)}>
                  <Tag color='cyan' type='light' size='small'>
                    {label}: {formatScore(value)}
                  </Tag>
                </Tooltip>
              ))
            ) : scoreEntries.length ? (
              scoreEntries.map(([key, value]) => (
                <Tag key={key} color='cyan' type='light' size='small'>
                  {scoreMetricLabel(key, t)}: {formatScore(value)}
                </Tag>
              ))
            ) : (
              <Typography.Text type='tertiary' size='small'>
                {t('评分拆解')}: {t('暂无评分样本')}
              </Typography.Text>
            )}
          </div>
        </div>
      </DetailAccordion>
      {(emptyOutputRate > 0 || issueRate > 0) && (
        <div className='ct-model-gateway-candidate-warning-line'>
          <Info size={13} />
          {emptyOutputRate > 0 ? (
            <Tooltip content={t('空输出率单独统计，不再重复计入体验异常率')}>
              <span>
                {t('空输出率')}: {formatPercent(emptyOutputRate)}
              </span>
            </Tooltip>
          ) : null}
          {issueRate > 0 ? (
            <Tooltip
              content={t('体验异常率不包含空输出，只统计非空输出的体验问题')}
            >
              <span>
                {t('体验异常率')}: {formatPercent(issueRate)}
              </span>
            </Tooltip>
          ) : null}
        </div>
      )}
    </div>
  );
}

function RecordDetailDrawer({
  record,
  visible,
  onClose,
  onExportReplay,
  onOpenScoreHistory,
  onRuntimeCircuitCleared,
  scoreHistory,
  scoreHistoryLoading = false,
  t,
}) {
  const requestMeta = record?.request_meta || {};
  const isHealthProbe =
    record?.is_health_probe === true || requestMeta?.is_health_probe === true;
  const probeReason = getProbeReason(record);
  const candidateExplanations = getCandidateExplanations(record);
  const scoreEntries = Object.entries(record?.score_breakdown || {}).filter(
    ([key, value]) =>
      Number.isFinite(Number(value)) && scoreEntryIsVisible(key, 1),
  );
  const metaEntries = Object.entries(requestMeta).filter(
    ([key, value]) =>
      key !== 'candidate_explanations' &&
      value !== '' &&
      value !== undefined &&
      value !== null,
  );
  const status = record ? getStatusMeta(record, t) : null;
  const recordType =
    record?.kind === 'user_request_detail'
      ? t('客户端请求')
      : isDispatch(record)
        ? t('调度')
        : t('尝试');
  const groupRoute = `${record?.requested_group || '--'} -> ${
    record?.selected_group || '--'
  }${record?.actual_group ? ` -> ${record.actual_group}` : ''}`;
  const channelRoute = `#${record?.channel_id || '--'} ${
    record?.channel_name || ''
  }${
    record?.actual_channel_id
      ? ` -> #${record.actual_channel_id} ${record.actual_channel_name || ''}`
      : ''
  }`;
  const selectedCandidate = findSelectedCandidate(
    record,
    candidateExplanations,
  );
  const selectedScoreItems = normalizeScoreItemsForDisplay(
    selectedCandidate?.score_items,
  );
  const selectedChannelLabel = selectedCandidate
    ? getCandidateChannelLabel(selectedCandidate, t)
    : record?.actual_channel_name ||
      record?.channel_name ||
      (record?.channel_id ? `#${record.channel_id}` : '--');
  const availableCandidateCount =
    candidateExplanations.filter(isAvailableCandidate).length;
  const filteredCandidateCount = Math.max(
    0,
    candidateExplanations.length - availableCandidateCount,
  );
  const upstreamStatus = upstreamCostStatus(record);
  const costSource = upstreamCostSourceLabel(
    upstreamStatus.source,
    upstreamStatus.accuracy,
    t,
  );
  const candidateGroups = record?.candidate_groups || [];
  const groupTags = candidateGroups.length ? (
    <div className='ct-model-gateway-record-tags'>
      {candidateGroups.map((group) => (
        <Tag key={group} color='blue' type='light'>
          {group}
        </Tag>
      ))}
    </div>
  ) : (
    <Typography.Text type='tertiary'>--</Typography.Text>
  );
  const policyTags = (
    <div className='ct-model-gateway-record-tags'>
      {record?.policy_mode && <Tag>{record.policy_mode}</Tag>}
      {record?.auto_mode && <Tag>{record.auto_mode}</Tag>}
      {record?.strategy && <Tag>{record.strategy}</Tag>}
      {record?.shadow && <Tag color='purple'>{t('影子')}</Tag>}
      {isHealthProbe && (
        <Tag color='cyan' type='light'>
          {t('健康探活')}
        </Tag>
      )}
    </div>
  );
  const dispatchRequirements = getDispatchRequirements(record);
  const selectionInsight = buildSelectionInsight(
    record,
    candidateExplanations,
    t,
  );
  const selectionSummary = buildSelectionSummaryText(selectionInsight, t);
  const scoreHistoryCandidate = scoreHistoryCandidateFromRecord(record);
  const canOpenScoreHistory =
    typeof onOpenScoreHistory === 'function' && !!scoreHistoryCandidate;
  const handleOpenScoreHistory = useCallback(() => {
    if (!canOpenScoreHistory) return;
    onOpenScoreHistory(scoreHistoryCandidate);
  }, [canOpenScoreHistory, onOpenScoreHistory, scoreHistoryCandidate]);
  const scoreChange = scoreHistoryChangeForRecord(scoreHistory, record);
  const scoreChangeTone =
    scoreHistoryLoading || !scoreChange.hasComparison
      ? 'neutral'
      : scoreDeltaTone(scoreChange.delta);
  const scoreChangeLabel = scoreHistoryLoading
    ? t('变更计算中')
    : scoreChange.hasComparison
      ? `${t('本次变化')} ${formatScoreDelta(scoreChange.delta)}`
      : canOpenScoreHistory
        ? t('暂无对比')
        : undefined;
  const hasErrorDetail = Boolean(
    record?.error_code || record?.error_type || record?.status_code,
  );
  const technicalMeta = [
    t('评分 {{count}} 项', { count: scoreEntries.length }),
    t('元数据 {{count}} 项', { count: metaEntries.length }),
    t('错误 {{count}} 项', { count: hasErrorDetail ? 1 : 0 }),
  ].join(' · ');

  return (
    <SideSheet
      title={t('调度详情')}
      visible={visible}
      onCancel={onClose}
      placement='right'
      width='min(1280px, 96vw)'
      footer={
        <div className='ct-model-gateway-modal-footer'>
          <Button onClick={onClose}>{t('关闭')}</Button>
          <Button
            theme='solid'
            type='primary'
            icon={<RotateCcw size={15} />}
            disabled={!record?.request_id}
            onClick={() => onExportReplay(record?.request_id)}
          >
            {t('导出 Replay JSON')}
          </Button>
        </div>
      }
    >
      {!record ? null : (
        <div className='ct-model-gateway-detail'>
          <div className='ct-model-gateway-detail-hero'>
            <div className='ct-model-gateway-detail-hero-main'>
              <div className='ct-model-gateway-detail-eyebrow'>
                <Tag color={status.color} shape='circle'>
                  {status.label}
                </Tag>
                <span>{recordType}</span>
              </div>
              <div className='ct-model-gateway-detail-title-row'>
                <Typography.Title heading={5}>
                  {record.requested_model || '--'}
                </Typography.Title>
                <Tag color='green' type='light' shape='circle'>
                  {selectedChannelLabel}
                </Tag>
              </div>
              <div className='ct-model-gateway-detail-route'>
                <span title={record.request_id}>{record.request_id}</span>
                <span title={groupRoute}>{groupRoute}</span>
                <span title={channelRoute}>{channelRoute}</span>
              </div>
              <div className='ct-model-gateway-detail-summary'>
                <Info size={14} />
                <span>{selectionSummary}</span>
              </div>
              <DispatchRequirementNotice
                requirements={dispatchRequirements}
                t={t}
              />
            </div>
            <div className='ct-model-gateway-detail-hero-metrics'>
              <DetailMetricTile
                label={t('稳定评分')}
                value={formatScore(record.score_total)}
                detail={t('请求前后运行态评分')}
                tone='score'
                onClick={canOpenScoreHistory ? handleOpenScoreHistory : null}
                actionLabel={scoreChangeLabel}
                actionTone={scoreChangeTone}
              />
              <DetailMetricTile
                label={t('总耗时')}
                value={formatLatency(record.duration_ms)}
                detail={t('用户最终等待')}
              />
              <DetailMetricTile
                label={t('首包')}
                value={formatLatency(record.ttft_ms)}
                detail={t('首次响应')}
              />
              <DetailMetricTile
                label={t('候选渠道')}
                value={`${formatNumber(availableCandidateCount)} / ${formatNumber(
                  candidateExplanations.length,
                )}`}
                detail={
                  filteredCandidateCount > 0
                    ? `${formatNumber(filteredCandidateCount)} ${t('已过滤')}`
                    : dispatchRequirements.visible
                      ? t('工具过滤后候选')
                      : t('全部可用')
                }
              />
            </div>
          </div>

          <div className='ct-model-gateway-detail-two-col'>
            <DetailPanel title={t('请求概览')}>
              <DetailInfoGrid
                items={[
                  {
                    key: 'request_id',
                    label: t('请求 ID'),
                    value: <DetailValue>{record.request_id}</DetailValue>,
                  },
                  {
                    key: 'endpoint_type',
                    label: t('端点类型'),
                    value: <DetailValue>{record.endpoint_type}</DetailValue>,
                  },
                  {
                    key: 'flow',
                    label: t('调度流转'),
                    value: <DispatchFlowTags record={record} t={t} compact />,
                  },
                  {
                    key: 'groups',
                    label: t('候选分组'),
                    value: groupTags,
                  },
                  ...(dispatchRequirements.tools.length
                    ? [
                        {
                          key: 'required_tools',
                          label: t('调度所需工具能力'),
                          value: (
                            <div className='ct-model-gateway-record-tags'>
                              {dispatchRequirements.tools.map((tool) => (
                                <Tag
                                  key={tool}
                                  color='purple'
                                  type='light'
                                  size='small'
                                >
                                  {formatDispatchTool(tool, t)}
                                </Tag>
                              ))}
                            </div>
                          ),
                        },
                      ]
                    : []),
                  ...(dispatchRequirements.conditions.length
                    ? [
                        {
                          key: 'filter_conditions',
                          label: t('过滤条件'),
                          value: (
                            <div className='ct-model-gateway-record-tags'>
                              {dispatchRequirements.conditions.map(
                                (condition) => (
                                  <Tag
                                    key={condition}
                                    color='orange'
                                    type='light'
                                    size='small'
                                  >
                                    {formatDispatchFilterCondition(
                                      condition,
                                      t,
                                    )}
                                  </Tag>
                                ),
                              )}
                            </div>
                          ),
                        },
                      ]
                    : []),
                  ...(isHealthProbe
                    ? [
                        {
                          key: 'probe_reason',
                          label: t('探活原因'),
                          value: (
                            <DetailValue>
                              {formatProbeReasonWithScope(record, t)}
                            </DetailValue>
                          ),
                        },
                      ]
                    : []),
                ]}
              />
            </DetailPanel>

            <DetailPanel title={t('调度状态')}>
              <DetailInfoGrid
                items={[
                  {
                    key: 'queue',
                    label: t('队列等待'),
                    value: (
                      <QueueStickyTags
                        record={record}
                        t={t}
                        compact
                        showSticky={false}
                      />
                    ),
                  },
                  {
                    key: 'sticky',
                    label: t('粘滞路由'),
                    value: (
                      <QueueStickyTags
                        record={record}
                        t={t}
                        compact
                        showQueue={false}
                      />
                    ),
                  },
                  {
                    key: 'policy',
                    label: t('策略'),
                    value: policyTags,
                  },
                  {
                    key: 'cost',
                    label: t('成本状态'),
                    value:
                      upstreamStatus.amount > 0
                        ? formatUsdCostAmount(upstreamStatus.amount)
                        : costSource,
                  },
                ]}
              />
            </DetailPanel>
          </div>

          <SelectionInsightPanel
            record={record}
            candidates={candidateExplanations}
            t={t}
          />

          <SmartDispatchAttemptTimeline record={record} t={t} />

          <DetailPanel title={t('上游成本明细')}>
            <UpstreamCostDetailPanel record={record} t={t} />
          </DetailPanel>

          <TimingBreakdownPanel record={record} t={t} />

          <DetailPanel title={t('候选渠道解释')}>
            {candidateExplanations.length ? (
              <div className='ct-model-gateway-candidate-list'>
                {candidateExplanations.map((candidate, index) => (
                  <CandidateExplanationCard
                    key={`${candidate?.channel_id || 'candidate'}-${
                      formatRuntimeKey(candidate?.runtime_key) || index
                    }`}
                    candidate={candidate}
                    candidates={candidateExplanations}
                    index={index}
                    record={record}
                    onOpenScoreHistory={onOpenScoreHistory}
                    onRuntimeCircuitCleared={onRuntimeCircuitCleared}
                    t={t}
                  />
                ))}
              </div>
            ) : (
              <Typography.Text type='tertiary'>--</Typography.Text>
            )}
          </DetailPanel>

          <DetailAccordion title={t('技术明细')} meta={technicalMeta}>
            <div className='ct-model-gateway-technical-grid'>
              <DetailPanel title={t('评分拆解')}>
                <ScoreBreakdownPanel
                  entries={scoreEntries}
                  items={selectedScoreItems}
                  onOpenScoreHistory={
                    canOpenScoreHistory ? handleOpenScoreHistory : undefined
                  }
                  t={t}
                />
              </DetailPanel>

              <DetailPanel title={t('调度元数据')}>
                {metaEntries.length ? (
                  <Descriptions
                    align='plain'
                    size='small'
                    data={metaEntries.map(([key, value]) => ({
                      key,
                      value: String(value),
                    }))}
                  />
                ) : (
                  <Typography.Text type='tertiary'>--</Typography.Text>
                )}
              </DetailPanel>

              {hasErrorDetail && (
                <DetailPanel title={t('错误信息')}>
                  <Descriptions
                    align='plain'
                    size='small'
                    data={[
                      {
                        key: 'HTTP',
                        value: record.status_code || '--',
                      },
                      {
                        key: t('错误码'),
                        value: record.error_code || '--',
                      },
                      {
                        key: t('错误类型'),
                        value: record.error_type || '--',
                      },
                      {
                        key: t('失败分类'),
                        value: formatAttemptErrorCategory(
                          record.error_category,
                          t,
                        ),
                      },
                      {
                        key: t('错误信息'),
                        value: (
                          <DetailValue>
                            {record.error_message || '--'}
                          </DetailValue>
                        ),
                      },
                    ]}
                  />
                </DetailPanel>
              )}
            </div>
          </DetailAccordion>
        </div>
      )}
    </SideSheet>
  );
}

function normalizeScoreBoostsForSave(boosts = {}) {
  const out = {};
  for (const [key, value] of Object.entries(boosts || {})) {
    const numeric = Number(value);
    if (!SCORE_BOOST_ITEM_KEYS.includes(key)) continue;
    if (!Number.isFinite(numeric) || numeric <= 0) continue;
    out[key] = Math.min(1, Math.max(0, numeric));
  }
  return out;
}

function channelOptionLabel(channel) {
  if (!channel) return '--';
  const id = channel.id || channel.channel_id;
  const name = channel.name || channel.channel_name || '';
  return name ? `${name} (#${id})` : `#${id}`;
}

function scoreBoostItemsForDisplay(allowedItems = [], t) {
  const source =
    Array.isArray(allowedItems) && allowedItems.length
      ? allowedItems
      : SCORE_BOOST_ITEM_KEYS.map((key) => ({ key }));
  return source
    .map((item) => {
      const key = String(item?.key || '').trim();
      if (!SCORE_BOOST_ITEM_KEYS.includes(key)) return null;
      return {
        key,
        name: scoreMetricLabel(key, t),
        description: scoreMetricDescription(key, t),
        category: item?.category || '',
      };
    })
    .filter(Boolean);
}

function ChannelScoreBoostModal({ visible, onCancel, onSaved, t }) {
  const [channelKeyword, setChannelKeyword] = useState('');
  const [channelOptions, setChannelOptions] = useState([]);
  const [selectedChannelId, setSelectedChannelId] = useState(null);
  const [selectedChannelName, setSelectedChannelName] = useState('');
  const [allowedItems, setAllowedItems] = useState([]);
  const [boosts, setBoosts] = useState({});
  const [channelLoading, setChannelLoading] = useState(false);
  const [boostLoading, setBoostLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const latestBoostChannelIdRef = useRef(null);

  const loadChannels = useCallback(
    async (keyword = '') => {
      setChannelLoading(true);
      try {
        const response = await API.get('/api/channel/search', {
          params: {
            keyword: String(keyword || '').trim(),
            page_size: 30,
            p: 1,
            id_sort: true,
          },
          disableDuplicate: true,
          skipErrorHandler: true,
        });
        const payload = unwrapApiData(response);
        setChannelOptions(Array.isArray(payload?.items) ? payload.items : []);
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('加载渠道列表失败');
        showError(message);
      } finally {
        setChannelLoading(false);
      }
    },
    [t],
  );

  const loadBoosts = useCallback(
    async (channelId) => {
      if (!channelId) return;
      const requestChannelId = Number(channelId);
      latestBoostChannelIdRef.current = requestChannelId;
      setBoostLoading(true);
      try {
        const response = await API.get(
          `/api/model_gateway/channels/${requestChannelId}/score_boosts`,
          {
            disableDuplicate: true,
            skipErrorHandler: true,
          },
        );
        if (latestBoostChannelIdRef.current !== requestChannelId) return;
        const payload = unwrapApiData(response);
        setBoosts(payload?.smart_score_boosts || {});
        setAllowedItems(
          Array.isArray(payload?.allowed_score_items)
            ? payload.allowed_score_items
            : [],
        );
        setSelectedChannelName(payload?.channel_name || '');
      } catch (err) {
        const message =
          err?.response?.data?.message ||
          err?.message ||
          t('加载渠道分值加成失败');
        showError(message);
      } finally {
        if (latestBoostChannelIdRef.current === requestChannelId) {
          setBoostLoading(false);
        }
      }
    },
    [t],
  );

  useEffect(() => {
    if (!visible) return;
    setChannelKeyword('');
    setSelectedChannelId(null);
    setSelectedChannelName('');
    setBoosts({});
    setAllowedItems([]);
    latestBoostChannelIdRef.current = null;
    loadChannels('');
  }, [loadChannels, visible]);

  const items = useMemo(
    () => scoreBoostItemsForDisplay(allowedItems, t),
    [allowedItems, t],
  );

  const selectedChannel = useMemo(
    () =>
      channelOptions.find(
        (channel) => Number(channel?.id) === Number(selectedChannelId),
      ),
    [channelOptions, selectedChannelId],
  );

  const updateBoost = useCallback((key, value) => {
    const numeric = Number(value);
    setBoosts((current) => ({
      ...current,
      [key]: Number.isFinite(numeric) ? Math.min(1, Math.max(0, numeric)) : 0,
    }));
  }, []);

  const saveBoosts = useCallback(
    async (nextBoosts = boosts, message = t('渠道分值加成已保存')) => {
      if (!selectedChannelId) {
        Toast.warning(t('请先选择渠道'));
        return;
      }
      setSaving(true);
      try {
        const response = await API.patch(
          `/api/model_gateway/channels/${selectedChannelId}/score_boosts`,
          { smart_score_boosts: normalizeScoreBoostsForSave(nextBoosts) },
          {
            disableDuplicate: true,
            skipErrorHandler: true,
          },
        );
        const payload = unwrapApiData(response);
        setBoosts(payload?.smart_score_boosts || {});
        setAllowedItems(
          Array.isArray(payload?.allowed_score_items)
            ? payload.allowed_score_items
            : allowedItems,
        );
        setSelectedChannelName(payload?.channel_name || selectedChannelName);
        Toast.success(message);
        if (typeof onSaved === 'function') onSaved();
      } catch (err) {
        const errorMessage =
          err?.response?.data?.message ||
          err?.message ||
          t('保存渠道分值加成失败');
        showError(errorMessage);
      } finally {
        setSaving(false);
      }
    },
    [allowedItems, boosts, onSaved, selectedChannelId, selectedChannelName, t],
  );

  const activeBoostCount = Object.values(
    normalizeScoreBoostsForSave(boosts),
  ).length;
  const titleChannel =
    selectedChannelName || channelOptionLabel(selectedChannel);

  return (
    <Modal
      title={t('渠道分值加成')}
      visible={visible}
      onCancel={onCancel}
      width={780}
      footer={
        <div className='ct-model-gateway-score-boost-footer'>
          <Button
            type='danger'
            theme='borderless'
            disabled={!selectedChannelId || saving}
            onClick={() => saveBoosts({}, t('渠道分值加成已清空'))}
          >
            {t('清空加成')}
          </Button>
          <div>
            <Button onClick={onCancel} disabled={saving}>
              {t('取消')}
            </Button>
            <Button
              theme='solid'
              type='primary'
              loading={saving}
              disabled={!selectedChannelId || boostLoading}
              onClick={() => saveBoosts()}
            >
              {t('保存')}
            </Button>
          </div>
        </div>
      }
    >
      <div className='ct-model-gateway-score-boost-modal'>
        <div className='ct-model-gateway-score-boost-search'>
          <Input
            value={channelKeyword}
            onChange={setChannelKeyword}
            onEnterPress={() => loadChannels(channelKeyword)}
            prefix={t('渠道')}
            placeholder={t('搜索渠道名称或 ID')}
          />
          <Button
            icon={<Search size={14} />}
            loading={channelLoading}
            onClick={() => loadChannels(channelKeyword)}
          >
            {t('搜索')}
          </Button>
        </div>
        <Select
          value={selectedChannelId}
          placeholder={t('请选择渠道')}
          loading={channelLoading}
          className='ct-model-gateway-score-boost-channel-select'
          onChange={(value) => {
            const channelId = Number(value);
            setSelectedChannelId(channelId);
            const channel = channelOptions.find(
              (item) => Number(item?.id) === channelId,
            );
            setSelectedChannelName(channelOptionLabel(channel));
            setBoosts({});
            loadBoosts(channelId);
          }}
        >
          {channelOptions.map((channel) => (
            <Select.Option key={channel.id} value={channel.id}>
              {channelOptionLabel(channel)}
            </Select.Option>
          ))}
        </Select>

        <div className='ct-model-gateway-score-boost-summary'>
          <Tag color={selectedChannelId ? 'cyan' : 'grey'} type='light'>
            {selectedChannelId
              ? t('当前渠道：{{channel}}', { channel: titleChannel })
              : t('未选择渠道')}
          </Tag>
          <Tag color={activeBoostCount > 0 ? 'green' : 'grey'} type='light'>
            {t('已配置 {{count}} 项加成', { count: activeBoostCount })}
          </Tag>
        </div>

        <Skeleton
          active
          loading={boostLoading}
          placeholder={<Skeleton.Paragraph rows={7} />}
        >
          <div className='ct-model-gateway-score-boost-table'>
            <div className='ct-model-gateway-score-boost-head'>
              <span>{t('评分项')}</span>
              <span>{t('说明')}</span>
              <span>{t('加成值')}</span>
            </div>
            {items.map((item) => (
              <div className='ct-model-gateway-score-boost-row' key={item.key}>
                <div>
                  <Typography.Text strong>{item.name}</Typography.Text>
                  <small>{item.key}</small>
                </div>
                <Typography.Text type='secondary'>
                  {item.description || '--'}
                </Typography.Text>
                <InputNumber
                  min={0}
                  max={1}
                  step={0.01}
                  precision={2}
                  value={Number(boosts[item.key] || 0)}
                  onChange={(value) => updateBoost(item.key, value)}
                />
              </div>
            ))}
          </div>
        </Skeleton>
      </div>
    </Modal>
  );
}

function ReplayModal({ artifact, loading, visible, onCancel, requestId, t }) {
  const downloadUrl = `/api/model_gateway/replay/export?request_id=${encodeURIComponent(
    requestId || '',
  )}&download=true`;
  const preview = artifact ? JSON.stringify(artifact, null, 2) : '';

  return (
    <Modal
      title={t('Replay 导出')}
      visible={visible}
      onCancel={onCancel}
      footer={
        <div className='ct-model-gateway-modal-footer'>
          <Button onClick={onCancel}>{t('关闭')}</Button>
          <Button
            theme='solid'
            type='primary'
            icon={<Download size={15} />}
            disabled={!requestId}
            onClick={() => window.open(downloadUrl, '_blank', 'noopener')}
          >
            {t('下载 JSON')}
          </Button>
        </div>
      }
      width={860}
    >
      <div className='ct-model-gateway-replay-head'>
        <Typography.Text strong>{requestId || '--'}</Typography.Text>
        {artifact?.count !== undefined && (
          <Tag color='blue' shape='circle'>
            {formatNumber(artifact.count)} {t('条记录')}
          </Tag>
        )}
      </div>
      {loading ? (
        <Skeleton
          active
          loading
          placeholder={<Skeleton.Paragraph rows={8} />}
        />
      ) : (
        <pre className='ct-model-gateway-json-preview'>
          {preview || t('暂无 Replay 预览')}
        </pre>
      )}
    </Modal>
  );
}

function ScoreHistoryModal({ history, loading, visible, onCancel, t }) {
  const items = Array.isArray(history?.items) ? history.items : [];
  const current = history?.current || items[0] || null;
  const previous = history?.previous || items[1] || null;
  const scoreDelta = Number(history?.score_delta || current?.score_delta || 0);
  const positiveEntries = scoreHistoryContributionEntries(
    current,
    history,
    'positive',
    t,
  );
  const negativeEntries = scoreHistoryContributionEntries(
    current,
    history,
    'negative',
    t,
  );
  const summaryText = buildScoreHistorySummary(history, current, previous, t);
  const objectTitle = scoreHistoryObjectTitle(history, current, t);
  const statusMeta = scoreHistoryStatusMeta(current, t);
  const sourceMeta = scoreHistorySourceMeta(current, t);
  const confidenceMeta = scoreHistorySampleConfidence(current, t);
  const recommendation = scoreHistoryRecommendation(
    current,
    negativeEntries,
    t,
  );
  const currentRows = scoreItemDisplayRows(
    current?.score_items,
    current?.score_item_deltas,
    t,
  );
  const [showAllScoreItems, setShowAllScoreItems] = useState(false);
  const visibleCurrentItems = showAllScoreItems
    ? currentRows
    : currentRows.slice(0, 8);

  return (
    <Modal
      title={t('评分解释')}
      visible={visible}
      onCancel={onCancel}
      footer={
        <div className='ct-model-gateway-modal-footer'>
          <Button onClick={onCancel}>{t('关闭')}</Button>
        </div>
      }
      width={1120}
    >
      <div className='ct-model-gateway-score-history'>
        <div className='ct-model-gateway-score-history-head'>
          <div>
            <Typography.Text strong>{objectTitle.channel}</Typography.Text>
            <Typography.Text type='secondary' size='small'>
              {objectTitle.model} / {objectTitle.group}
            </Typography.Text>
          </div>
          <div className='ct-model-gateway-record-tags'>
            <Tag color={statusMeta.color} type='light' shape='circle'>
              {statusMeta.label}
            </Tag>
            <Tag color={sourceMeta.color} type='light' shape='circle'>
              {sourceMeta.label}
            </Tag>
            <Tag color='cyan' type='light' shape='circle'>
              {t('记录数')} {formatNumber(history?.total_matched)}
            </Tag>
            <Tag
              color={scoreDeltaColor(scoreDelta)}
              type='light'
              shape='circle'
            >
              {t('较上一条')} {formatScoreDelta(scoreDelta)}
            </Tag>
            {history?.truncated ? (
              <Tag color='orange' type='light' shape='circle'>
                {t('已截断')}
              </Tag>
            ) : null}
          </div>
        </div>

        <div className='ct-model-gateway-score-history-explain'>
          <div>
            <Info size={16} />
            <span>{scoreHistorySummaryTitle(scoreDelta, t)}</span>
          </div>
          <Typography.Text>{summaryText}</Typography.Text>
          <Typography.Text type='secondary' size='small'>
            {recommendation}
          </Typography.Text>
        </div>

        {current ? (
          <div className='ct-model-gateway-score-history-summary'>
            <div>
              <span>{t('当前稳定评分')}</span>
              <strong>{formatScore(current.score_total)}</strong>
            </div>
            <div>
              <span>{t('上次稳定评分')}</span>
              <strong>{formatScore(previous?.score_total)}</strong>
            </div>
            <div>
              <span>{t('较上一条稳定评分变化')}</span>
              <strong
                className={
                  scoreDelta >= 0
                    ? 'ct-model-gateway-score-delta-positive'
                    : 'ct-model-gateway-score-delta-negative'
                }
              >
                {formatScoreDelta(scoreDelta)}
              </strong>
            </div>
            <div>
              <span>{t('样本可信度')}</span>
              <strong>
                {scoreHistorySampleLabel(current.sample_count, t)}
              </strong>
              <small>
                <Tag color={confidenceMeta.color} size='small' type='light'>
                  {confidenceMeta.label}
                </Tag>
              </small>
            </div>
            <div>
              <span>{t('运营建议')}</span>
              <strong>{recommendation}</strong>
            </div>
          </div>
        ) : null}

        <div className='ct-model-gateway-score-history-contrib-grid'>
          <ScoreHistoryContributionCard
            title={t('主要加分项')}
            emptyText={t('暂无明显加分项')}
            entries={positiveEntries}
            tone='positive'
          />
          <ScoreHistoryContributionCard
            title={t('主要扣分项')}
            emptyText={t('暂无明显扣分项')}
            entries={negativeEntries}
            tone='negative'
          />
        </div>

        {currentRows.length ? (
          <div className='ct-model-gateway-score-history-section'>
            <div className='ct-model-gateway-score-history-section-head'>
              <div>
                <Typography.Text strong>{t('评分明细')}</Typography.Text>
                <Typography.Text type='secondary' size='small'>
                  {t('评分明细优先展示加权贡献，贡献变化才会影响稳定评分')}
                </Typography.Text>
              </div>
              {currentRows.length > 8 ? (
                <Button
                  size='small'
                  type='tertiary'
                  onClick={() => setShowAllScoreItems((value) => !value)}
                >
                  {showAllScoreItems ? t('收起评分项') : t('查看全部评分项')}
                </Button>
              ) : null}
            </div>
            <ScoreItemsTable preparedRows={visibleCurrentItems} t={t} compact />
          </div>
        ) : null}

        {loading ? (
          <Skeleton
            active
            loading
            placeholder={<Skeleton.Paragraph rows={7} />}
          />
        ) : items.length ? (
          <div className='ct-model-gateway-score-history-section'>
            <div className='ct-model-gateway-score-history-section-head'>
              <div>
                <Typography.Text strong>{t('评分事件时间线')}</Typography.Text>
                <Typography.Text type='secondary' size='small'>
                  {t('最近评分变化')}
                </Typography.Text>
              </div>
            </div>
            <div className='ct-model-gateway-score-history-list'>
              {items.map((item) => (
                <ScoreHistoryRecordItem
                  key={`${item.id}-${item.request_id}`}
                  item={item}
                  t={t}
                />
              ))}
            </div>
          </div>
        ) : (
          <Empty description={t('未找到评分变更记录')} />
        )}

        <details className='ct-model-gateway-score-history-debug'>
          <summary>{t('技术详情')}</summary>
          <div>
            <span>{t('Runtime Key')}</span>
            <code>{formatRuntimeKey(history?.runtime_key)}</code>
          </div>
          <div>
            <span>{t('渠道 ID')}</span>
            <code>{history?.channel_id || current?.channel_id || '--'}</code>
          </div>
          <div>
            <span>{t('生成时间')}</span>
            <code>{formatTimestamp(history?.generated_at)}</code>
          </div>
        </details>
      </div>
    </Modal>
  );
}

function ScoreHistoryContributionCard({
  title,
  emptyText,
  entries = [],
  tone,
}) {
  return (
    <div
      className={`ct-model-gateway-score-history-contrib ct-model-gateway-score-history-contrib-${tone}`}
    >
      <Typography.Text strong>{title}</Typography.Text>
      {entries.length ? (
        entries.map((entry) => (
          <div key={entry.key}>
            <span>{entry.label}</span>
            <strong>{formatScoreDelta(entry.value)}</strong>
            {entry.badge ? <small>{entry.badge}</small> : null}
            {entry.description ? <em>{entry.description}</em> : null}
          </div>
        ))
      ) : (
        <Typography.Text type='tertiary' size='small'>
          {emptyText}
        </Typography.Text>
      )}
    </div>
  );
}

function ScoreHistoryRecordItem({ item, t }) {
  const reasons = buildScoreHistoryItemReasons(item, t);
  const metricEntries = scoreMetricEntries(
    item?.score_breakdown,
    item?.score_breakdown_delta,
    t,
  );
  const routingScore = Number(item?.routing_score_total || 0);
  const scoreItemRows = scoreItemDisplayRows(
    item?.score_items,
    item?.score_item_deltas,
    t,
  );
  const showLegacyMetricEntries = metricEntries.length && !scoreItemRows.length;
  const ttftScoreItem = scoreItemByKey(scoreItemRows, 'ttft_latency');
  const durationScoreItem = scoreItemByKey(scoreItemRows, 'duration_latency');
  const routingEntries = normalizeScoreItemsForDisplay(
    item?.routing_score_items,
  ).filter(
    (scoreItem) =>
      ['concurrency_load', 'queue_pressure', 'first_byte_backlog'].includes(
        scoreItem.key,
      ) &&
      Number.isFinite(Number(scoreItem.score)) &&
      scoreItem.weight > 0 &&
      !scoreItem.missing_reason,
  );
  const hasRoutingScore =
    routingScore > 0 &&
    Math.abs(routingScore - Number(item?.score_total || 0)) >= 0.0001;

  return (
    <div className='ct-model-gateway-score-history-item'>
      <div className='ct-model-gateway-score-history-main'>
        <div>
          <Typography.Text strong>
            {t('稳定评分')}: {formatScore(item.score_total)}
          </Typography.Text>
          <Tag
            color={scoreDeltaColor(item.score_delta)}
            type='light'
            size='small'
          >
            {t('较上一条')} {formatScoreDelta(item.score_delta)}
          </Tag>
          {item.selected ? (
            <Tag color='green' type='solid' size='small'>
              {t('最终选择')}
            </Tag>
          ) : null}
          {item.source === 'runtime_current' ? (
            <Tag color='cyan' type='solid' size='small'>
              {t('当前动态')}
            </Tag>
          ) : null}
          {hasRoutingScore ? (
            <Tag color='grey' type='light' size='small'>
              {t('本次调度评分')}: {formatScore(routingScore)}
            </Tag>
          ) : null}
          <Tag
            color={item.available ? 'green' : 'red'}
            type='light'
            size='small'
          >
            {item.available ? t('可用') : t('不可用')}
          </Tag>
        </div>
        <Typography.Text type='secondary' size='small'>
          {formatTimestamp(item.created_at)}
        </Typography.Text>
      </div>

      <div className='ct-model-gateway-score-history-status'>
        <Typography.Text strong>{t('本条情况')}</Typography.Text>
        <div>
          {reasons.map((reason, index) => (
            <Typography.Text
              key={reason}
              type={index === 0 ? undefined : 'secondary'}
            >
              {reason}
            </Typography.Text>
          ))}
        </div>
      </div>

      <div className='ct-model-gateway-score-history-meta'>
        <span>
          {t('请求 ID')}: {item.request_id || '--'}
        </span>
        <span>
          {t('分组')}: {item.requested_group || '--'} →{' '}
          {item.selected_group || '--'}
        </span>
        <span>
          {t('评分首包')}: {formatScoreItemRawValue(ttftScoreItem, t)}
        </span>
        <span>
          {t('评分耗时')}: {formatScoreItemRawValue(durationScoreItem, t)}
        </span>
        <span>
          {t('评分样本')}: {scoreHistorySampleLabel(item.sample_count, t)}
        </span>
      </div>

      {showLegacyMetricEntries ? (
        <div className='ct-model-gateway-score-history-metrics'>
          {metricEntries.slice(0, 8).map((entry) => (
            <div
              key={entry.key}
              title={t('历史字段：原始子项变化，不等于总分直接加减')}
            >
              <span>{entry.label}</span>
              <strong>{formatScore(entry.score)}</strong>
              <em
                className={
                  Number(entry.delta) >= 0
                    ? 'ct-model-gateway-score-delta-positive'
                    : 'ct-model-gateway-score-delta-negative'
                }
              >
                {formatScoreDelta(entry.delta)}
              </em>
              <small>{t('原始子项变化')}</small>
            </div>
          ))}
        </div>
      ) : null}
      {scoreItemRows.length ? (
        <ScoreItemsTable
          items={item.score_items}
          deltas={item.score_item_deltas}
          t={t}
          compact
        />
      ) : null}
      {routingEntries.length ? (
        <div className='ct-model-gateway-candidate-routing-row'>
          <Typography.Text type='tertiary' size='small'>
            {t('实时调度因子')}
          </Typography.Text>
          <Typography.Text type='tertiary' size='small'>
            {t('只影响本次调度评分')}
          </Typography.Text>
          {routingEntries.map((scoreItem) => (
            <Tag key={scoreItem.key} color='grey' type='light' size='small'>
              {scoreItemLabel(scoreItem, t)}: {formatScore(scoreItem.score)}
            </Tag>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ReplayBatchModal({
  artifact,
  filters,
  loading,
  onCancel,
  onDownload,
  onFilterChange,
  onPreview,
  t,
  visible,
}) {
  const manifest = artifact?.manifest || {};
  const items = Array.isArray(manifest.items) ? manifest.items : [];
  const preview = artifact ? JSON.stringify(artifact, null, 2) : '';

  return (
    <Modal
      title={t('Replay 批量导出')}
      visible={visible}
      onCancel={onCancel}
      footer={
        <div className='ct-model-gateway-modal-footer'>
          <Button onClick={onCancel}>{t('关闭')}</Button>
          <Button
            type='tertiary'
            icon={<Download size={15} />}
            onClick={onDownload}
          >
            {t('下载批量 JSON')}
          </Button>
          <Button
            theme='solid'
            type='primary'
            loading={loading}
            icon={<RotateCcw size={15} />}
            onClick={onPreview}
          >
            {t('预览批量 Replay')}
          </Button>
        </div>
      }
      width={980}
    >
      <div className='ct-model-gateway-replay-batch-form'>
        <Select
          value={filters.hours}
          onChange={(value) => onFilterChange('hours', value)}
          prefix={t('时间窗口')}
        >
          {WINDOW_OPTIONS.map((option) => (
            <Select.Option key={option} value={option}>
              {option >= 24
                ? `${Math.round(option / 24)} ${t('天')}`
                : `${option} ${t('小时')}`}
            </Select.Option>
          ))}
        </Select>
        <Select
          value={filters.limit}
          onChange={(value) => onFilterChange('limit', value)}
          prefix={t('限制条数')}
        >
          {REPLAY_BATCH_LIMIT_OPTIONS.map((option) => (
            <Select.Option key={option} value={option}>
              {formatNumber(option)}
            </Select.Option>
          ))}
        </Select>
        <Select
          value={filters.success}
          onChange={(value) => onFilterChange('success', value)}
          prefix={t('状态')}
        >
          <Select.Option value='all'>{t('全部')}</Select.Option>
          <Select.Option value='success'>{t('成功')}</Select.Option>
          <Select.Option value='failure'>{t('失败')}</Select.Option>
        </Select>
        <Input
          value={filters.model}
          onChange={(value) => onFilterChange('model', value)}
          placeholder={t('按模型筛选')}
          prefix={t('模型')}
        />
        <Input
          value={filters.group}
          onChange={(value) => onFilterChange('group', value)}
          placeholder={t('按分组筛选')}
          prefix={t('分组')}
        />
        <Input
          value={filters.channel_id}
          onChange={(value) => onFilterChange('channel_id', value)}
          placeholder={t('按渠道 ID 筛选')}
          prefix={t('渠道 ID')}
        />
        <Input
          value={filters.error_type}
          onChange={(value) => onFilterChange('error_type', value)}
          placeholder={t('按错误类型筛选')}
          prefix={t('错误类型')}
        />
      </div>
      <TextArea
        autosize={{ minRows: 2, maxRows: 5 }}
        className='ct-model-gateway-replay-request-ids'
        value={filters.request_ids}
        onChange={(value) => onFilterChange('request_ids', value)}
        placeholder={t('请求 ID 列表，逗号或换行分隔')}
      />
      <div className='ct-model-gateway-replay-head'>
        <div className='ct-model-gateway-record-tags'>
          <Tag color='blue' shape='circle'>
            {t('Artifacts')} {formatNumber(manifest.artifact_count)}
          </Tag>
          <Tag color='cyan' shape='circle'>
            {t('条记录')} {formatNumber(manifest.record_count)}
          </Tag>
          <Tag
            color={Number(manifest.failed_count) > 0 ? 'orange' : 'green'}
            shape='circle'
          >
            {t('失败项')} {formatNumber(manifest.failed_count)}
          </Tag>
        </div>
        <Typography.Text type='secondary' size='small'>
          {manifest.generated_at
            ? `${t('生成时间')}: ${formatTimestamp(manifest.generated_at)}`
            : t('暂无可导出的 Replay 记录')}
        </Typography.Text>
      </div>
      {items.length > 0 && (
        <div className='ct-model-gateway-replay-batch-items'>
          {items.slice(0, 8).map((item) => (
            <Tag
              key={`${item.request_id}-${item.filename}`}
              color={item.error ? 'orange' : 'green'}
              type='light'
            >
              {item.filename || item.request_id || '--'}
              {item.error
                ? ` · ${item.error}`
                : ` · ${formatNumber(item.record_count)} ${t('条记录')}`}
            </Tag>
          ))}
        </div>
      )}
      {loading ? (
        <Skeleton
          active
          loading
          placeholder={<Skeleton.Paragraph rows={8} />}
        />
      ) : (
        <pre className='ct-model-gateway-json-preview'>
          {preview || t('暂无 Replay 预览')}
        </pre>
      )}
    </Modal>
  );
}

export default function ModelGatewayMonitor() {
  const { t } = useTranslation();
  const [hours, setHours] = useState(DEFAULT_HOURS);
  const [trendBucket, setTrendBucket] = useState(DEFAULT_TREND_BUCKET);
  const [replayVisible, setReplayVisible] = useState(false);
  const [replayLoading, setReplayLoading] = useState(false);
  const [replayRequestId, setReplayRequestId] = useState('');
  const [replayArtifact, setReplayArtifact] = useState(null);
  const [replayBatchVisible, setReplayBatchVisible] = useState(false);
  const [replayBatchLoading, setReplayBatchLoading] = useState(false);
  const [replayBatchFilters, setReplayBatchFilters] = useState(
    EMPTY_REPLAY_BATCH_FILTERS,
  );
  const [replayBatchArtifact, setReplayBatchArtifact] = useState(null);
  const [scoreBoostVisible, setScoreBoostVisible] = useState(false);
  const [scoreHistoryVisible, setScoreHistoryVisible] = useState(false);
  const [scoreHistoryLoading, setScoreHistoryLoading] = useState(false);
  const [scoreHistory, setScoreHistory] = useState(null);
  const [detailScoreHistory, setDetailScoreHistory] = useState(null);
  const [detailScoreHistoryLoading, setDetailScoreHistoryLoading] =
    useState(false);
  const [filters, setFilters] = useState(EMPTY_FILTERS);
  const [appliedFilters, setAppliedFilters] = useState(EMPTY_FILTERS);
  const [detailRecord, setDetailRecord] = useState(null);
  const [dispatchDetailLoading, setDispatchDetailLoading] = useState('');
  const [stickyRefreshToken, setStickyRefreshToken] = useState(0);
  const [viewMode, setViewMode] = useState(VIEW_MODES.USER_REQUESTS);
  const [filtersVisible, setFiltersVisible] = useState(false);
  const [userRequestPageSize, setUserRequestPageSize] = useState(
    DEFAULT_USER_REQUEST_PAGE_SIZE,
  );
  const userRequestRecentLimit = Math.max(RECENT_LIMIT, userRequestPageSize);
  const {
    data,
    loading,
    refreshing,
    error,
    refresh,
    connectionState,
    fallbackMode,
    fallbackCountdown,
  } = useModelGatewayObservabilityData({
    hours,
    trendBucket,
    defaultTrendBucket: DEFAULT_TREND_BUCKET,
    recentLimit: userRequestRecentLimit,
    topN: TOP_N,
    appliedFilters,
    viewMode,
    t,
  });

  const exportReplay = useCallback(
    async (requestId) => {
      if (!requestId) {
        Toast.warning(t('缺少请求 ID'));
        return;
      }
      setReplayRequestId(requestId);
      setReplayArtifact(null);
      setReplayVisible(true);
      setReplayLoading(true);
      try {
        const response = await API.get('/api/model_gateway/replay/export', {
          params: { request_id: requestId, stable_ids: true },
          disableDuplicate: true,
          skipErrorHandler: true,
        });
        setReplayArtifact(unwrapApiData(response));
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('Replay 导出失败');
        showError(message);
      } finally {
        setReplayLoading(false);
      }
    },
    [t],
  );

  const openReplayBatch = useCallback(() => {
    setReplayBatchFilters({
      ...EMPTY_REPLAY_BATCH_FILTERS,
      hours,
      model: appliedFilters.model,
      group: appliedFilters.group,
      channel_id: appliedFilters.channel_id,
      error_type: appliedFilters.circuit_error_type,
      request_ids: appliedFilters.request_id,
    });
    setReplayBatchArtifact(null);
    setReplayBatchVisible(true);
  }, [appliedFilters, hours]);

  const updateReplayBatchFilter = useCallback((key, value) => {
    setReplayBatchFilters((current) => ({ ...current, [key]: value }));
  }, []);

  const previewReplayBatch = useCallback(async () => {
    setReplayBatchLoading(true);
    try {
      const response = await API.get('/api/model_gateway/replay/export/batch', {
        params: buildReplayBatchParams(replayBatchFilters),
        disableDuplicate: true,
        skipErrorHandler: true,
      });
      const payload = unwrapApiData(response);
      setReplayBatchArtifact(payload);
      if (Number(payload?.manifest?.artifact_count || 0) === 0) {
        Toast.warning(t('暂无可导出的 Replay 记录'));
      }
    } catch (err) {
      const message =
        err?.response?.data?.message ||
        err?.message ||
        t('Replay 批量导出失败');
      showError(message);
    } finally {
      setReplayBatchLoading(false);
    }
  }, [replayBatchFilters, t]);

  const downloadReplayBatch = useCallback(() => {
    window.open(
      buildReplayBatchDownloadUrl(replayBatchFilters),
      '_blank',
      'noopener',
    );
  }, [replayBatchFilters]);

  const fetchScoreHistory = useCallback(
    async (candidate) => {
      const channelId = Number(
        candidate?.channel_id || candidate?.runtime_key?.channel_id || 0,
      );
      if (!channelId) {
        throw new Error('missing_channel_id');
      }
      const response = await API.get(
        '/api/model_gateway/observability/score-history',
        {
          params: {
            hours,
            limit: SCORE_HISTORY_LIMIT,
            channel_id: channelId,
            ...buildRuntimeKeyParams(candidate?.runtime_key),
          },
          disableDuplicate: true,
          skipErrorHandler: true,
        },
      );
      return unwrapApiData(response);
    },
    [hours],
  );

  const openScoreHistory = useCallback(
    async (candidate) => {
      const channelId = Number(
        candidate?.channel_id || candidate?.runtime_key?.channel_id || 0,
      );
      if (!channelId) {
        Toast.warning(t('缺少渠道 ID'));
        return;
      }
      setScoreHistory(null);
      setScoreHistoryVisible(true);
      setScoreHistoryLoading(true);
      try {
        const payload = await fetchScoreHistory(candidate);
        setScoreHistory(payload);
        if (!Array.isArray(payload?.items) || payload.items.length === 0) {
          Toast.warning(t('未找到评分变更记录'));
        }
      } catch (err) {
        const message =
          err?.response?.data?.message ||
          err?.message ||
          t('加载评分变更记录失败');
        showError(message);
      } finally {
        setScoreHistoryLoading(false);
      }
    },
    [fetchScoreHistory, t],
  );

  useEffect(() => {
    const candidate = scoreHistoryCandidateFromRecord(detailRecord);
    if (!candidate) {
      setDetailScoreHistory(null);
      setDetailScoreHistoryLoading(false);
      return undefined;
    }
    let cancelled = false;
    setDetailScoreHistory(null);
    setDetailScoreHistoryLoading(true);
    fetchScoreHistory(candidate)
      .then((payload) => {
        if (!cancelled) {
          setDetailScoreHistory(payload);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setDetailScoreHistory(null);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setDetailScoreHistoryLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [detailRecord, fetchScoreHistory]);

  const openUserRequestDispatchDetail = useCallback(
    async (record) => {
      const requestId = record?.request_id;
      if (!requestId) {
        Toast.warning(t('缺少请求 ID'));
        return;
      }
      const fallbackRecords = record?.dispatch_record
        ? [record.dispatch_record]
        : [];
      setDispatchDetailLoading(requestId);
      try {
        const response = await API.get(
          '/api/model_gateway/observability/summary',
          {
            params: {
              hours,
              recent_limit: 200,
              top_n: 1,
              view_mode: VIEW_MODES.ENGINEERING,
              request_id: requestId,
            },
            disableDuplicate: true,
            skipErrorHandler: true,
          },
        );
        const payload = unwrapApiData(response);
        const recentRecords = payload?.recent_records?.length
          ? payload.recent_records
          : fallbackRecords;
        const detail = buildUserRequestDetailRecord(record, recentRecords);
        if (!detail) {
          Toast.warning(t('暂无调度详情'));
          return;
        }
        setDetailRecord(detail);
      } catch (err) {
        if (fallbackRecords.length) {
          setDetailRecord(
            buildUserRequestDetailRecord(record, fallbackRecords),
          );
          return;
        }
        const message =
          err?.response?.data?.message || err?.message || t('加载调度详情失败');
        showError(message);
      } finally {
        setDispatchDetailLoading('');
      }
    },
    [hours, t],
  );

  const exportTrends = useCallback(() => {
    const params = new URLSearchParams();
    params.set('hours', String(hours));
    params.set('download', 'true');
    if (trendBucket !== DEFAULT_TREND_BUCKET) {
      params.set('trend_bucket_seconds', trendBucket);
    }
    if (appliedFilters.model) params.set('model', appliedFilters.model);
    if (appliedFilters.group) params.set('group', appliedFilters.group);
    if (appliedFilters.channel_id) {
      params.set('channel_id', appliedFilters.channel_id);
    }
    if (appliedFilters.request_id) {
      params.set('request_id', appliedFilters.request_id);
    }
    window.open(
      `/api/model_gateway/observability/trends/export?${params.toString()}`,
      '_blank',
      'noopener',
    );
  }, [appliedFilters, hours, trendBucket]);

  const summary = data?.summary || {};
  const userRequestSummary = data?.user_requests?.summary || {};
  const runtimeStatus = data?.runtime_status || {};
  const hasData =
    viewMode === VIEW_MODES.USER_REQUESTS
      ? Number(userRequestSummary.total_requests) > 0
      : Number(summary.total_records) > 0;
  const lastUpdated = summary.end_time
    ? formatTimestamp(summary.end_time)
    : '--';
  const visibleAppliedFilters =
    viewMode === VIEW_MODES.USER_REQUESTS
      ? {
          model: appliedFilters.model,
          group: appliedFilters.group,
          request_id: appliedFilters.request_id,
        }
      : appliedFilters;
  const hasActiveFilters = Object.values(visibleAppliedFilters).some(Boolean);
  const realtimeMeta = realtimeStatusMeta(
    connectionState,
    fallbackMode,
    fallbackCountdown,
    t,
  );
  const dynamicRefreshCountdown = dynamicBillingRefreshCountdown(
    connectionState,
    fallbackMode,
    fallbackCountdown,
    data?.dynamic_billing_overview?.refresh_seconds,
  );

  const updateFilter = useCallback((key, value) => {
    setFilters((current) => ({ ...current, [key]: value }));
  }, []);

  const applyFilters = useCallback(() => {
    setAppliedFilters({
      model: filters.model.trim(),
      group: filters.group.trim(),
      channel_id:
        viewMode === VIEW_MODES.USER_REQUESTS ? '' : filters.channel_id.trim(),
      request_id: filters.request_id.trim(),
      circuit_error_type:
        viewMode === VIEW_MODES.USER_REQUESTS
          ? ''
          : normalizeCircuitErrorType(filters.circuit_error_type),
    });
  }, [filters, viewMode]);

  const resetFilters = useCallback(() => {
    setFilters(EMPTY_FILTERS);
    setAppliedFilters(EMPTY_FILTERS);
  }, []);

  const refreshDashboard = useCallback(() => {
    setStickyRefreshToken((value) => value + 1);
    refresh();
  }, [refresh]);

  const aggregateColumns = useCallback(
    (type) => {
      const columns = [
        {
          key: `${type}-name`,
          title:
            type === 'model'
              ? t('模型')
              : type === 'group'
                ? t('分组')
                : type === 'profile'
                  ? t('Provider Profile')
                  : type === 'proxy'
                    ? t('Proxy Mode')
                    : t('渠道'),
          dataIndex: 'key',
          width: 220,
          render: (_, record) => (
            <AggregateNameCell record={record} type={type} />
          ),
        },
        {
          key: `${type}-dispatches`,
          title: t('调度'),
          dataIndex: 'dispatches',
          width: 100,
          sorter: (a, b) => Number(a.dispatches) - Number(b.dispatches),
          render: (value) => (
            <Typography.Text strong>{formatNumber(value)}</Typography.Text>
          ),
        },
        {
          key: `${type}-success-rate`,
          title: t('成功率'),
          dataIndex: 'success_rate',
          width: 110,
          sorter: (a, b) => Number(a.success_rate) - Number(b.success_rate),
          render: (value, record) => {
            const tone = getSuccessTone(value, record.attempts);
            return (
              <Tag
                color={
                  tone === 'success'
                    ? 'green'
                    : tone === 'warning'
                      ? 'orange'
                      : 'red'
                }
                shape='circle'
                type='light'
              >
                {formatAttemptRate(value, record.attempts)}
              </Tag>
            );
          },
        },
        {
          key: `${type}-avg-duration`,
          title: t('平均耗时'),
          dataIndex: 'avg_duration_ms',
          width: 120,
          sorter: (a, b) =>
            Number(a.avg_duration_ms) - Number(b.avg_duration_ms),
          render: (value) => formatLatency(value),
        },
        {
          key: `${type}-avg-ttft`,
          title: t('首包延迟'),
          dataIndex: 'avg_ttft_ms',
          width: 120,
          sorter: (a, b) => Number(a.avg_ttft_ms) - Number(b.avg_ttft_ms),
          render: (value) => formatLatency(value),
        },
        {
          key: `${type}-stream-interrupted`,
          title: t('流中断'),
          dataIndex: 'stream_interrupted',
          width: 100,
          render: (value) => (
            <Typography.Text type={value > 0 ? 'warning' : 'secondary'}>
              {formatNumber(value)}
            </Typography.Text>
          ),
        },
        {
          key: `${type}-queue-sticky`,
          title: t('队列 / 粘滞'),
          dataIndex: 'avg_queue_wait_ms',
          width: 260,
          render: (_, record) => (
            <QueueStickyTags record={record} t={t} compact />
          ),
        },
        {
          key: `${type}-avg-score`,
          title: t('平均稳定评分'),
          dataIndex: 'avg_score_total',
          width: 110,
          render: (value) => formatScore(value),
        },
        {
          key: `${type}-score-breakdown`,
          title: t('评分拆解'),
          dataIndex: 'score_breakdown',
          width: 240,
          render: (value) => <ScoreBreakdown value={value} />,
        },
      ];
      if (type === 'group') {
        columns.splice(7, 0, {
          key: `${type}-resource-protection`,
          title: t('主资源保护指标'),
          dataIndex: 'resource_protection_dispatches',
          width: 320,
          render: (_, record) => (
            <ResourceProtectionAggregateCell record={record} t={t} />
          ),
        });
      }
      return columns;
    },
    [t],
  );

  const recentColumns = useMemo(
    () => [
      {
        key: 'recent-created-at',
        title: t('时间'),
        dataIndex: 'created_at',
        width: 170,
        render: (value) => (
          <Typography.Text type='secondary' size='small'>
            {formatTimestamp(value)}
          </Typography.Text>
        ),
      },
      {
        key: 'recent-request-id',
        title: t('请求 ID'),
        dataIndex: 'request_id',
        width: 220,
        render: (value, record) => (
          <div className='ct-model-gateway-request-cell'>
            <Typography.Text
              ellipsis={{ showTooltip: true }}
              className='ct-model-gateway-request-id'
            >
              {value || '--'}
            </Typography.Text>
            <div className='ct-model-gateway-record-tags'>
              <Tag color={isDispatch(record) ? 'blue' : 'grey'} size='small'>
                {isDispatch(record) ? t('调度') : t('尝试')}
              </Tag>
              {record.smart_handled && (
                <Tag color='cyan' size='small'>
                  {t('智能处理')}
                </Tag>
              )}
              {record.shadow && (
                <Tag color='purple' size='small'>
                  {t('影子')}
                </Tag>
              )}
              {(record.is_health_probe ||
                record.request_meta?.is_health_probe) && (
                <Tag color='cyan' size='small' type='light'>
                  {t('健康探活')}
                </Tag>
              )}
            </div>
          </div>
        ),
      },
      {
        key: 'recent-model-group',
        title: t('模型 / 分组'),
        dataIndex: 'requested_model',
        width: 230,
        render: (_, record) => (
          <div>
            <Typography.Text strong>
              {record.requested_model || '--'}
            </Typography.Text>
            <div className='text-xs text-semi-color-text-2'>
              {record.requested_group || '--'} → {record.selected_group || '--'}
              {record.actual_group ? ` → ${record.actual_group}` : ''}
            </div>
          </div>
        ),
      },
      {
        key: 'recent-channel',
        title: t('渠道'),
        dataIndex: 'channel_id',
        width: 190,
        render: (_, record) => (
          <div>
            <Typography.Text strong>
              {record.actual_channel_name ||
                record.channel_name ||
                (record.channel_id ? `#${record.channel_id}` : '--')}
            </Typography.Text>
            <div className='text-xs text-semi-color-text-2'>
              #{record.actual_channel_id || record.channel_id || '--'}
            </div>
          </div>
        ),
      },
      {
        key: 'recent-strategy',
        title: t('策略'),
        dataIndex: 'strategy',
        width: 170,
        render: (_, record) => (
          <div className='ct-model-gateway-record-tags'>
            {record.policy_mode && (
              <Tag color='blue' size='small' type='light'>
                {record.policy_mode}
              </Tag>
            )}
            {record.auto_mode && (
              <Tag color='cyan' size='small' type='light'>
                {record.auto_mode}
              </Tag>
            )}
            {record.strategy && (
              <Tag color='grey' size='small' type='light'>
                {record.strategy}
              </Tag>
            )}
          </div>
        ),
      },
      {
        key: 'recent-status',
        title: t('状态'),
        dataIndex: 'success',
        width: 180,
        render: (_, record) => {
          const meta = getStatusMeta(record, t);
          return (
            <div>
              <Tag color={meta.color} shape='circle'>
                {meta.label}
              </Tag>
              {record.retry_action && (
                <div className='text-xs text-semi-color-text-2'>
                  {formatAttemptFlowAction(record.retry_action, t)}
                </div>
              )}
              {record.status_code > 0 && (
                <div className='text-xs text-semi-color-text-2'>
                  HTTP {record.status_code}
                </div>
              )}
              {(record.is_health_probe ||
                record.request_meta?.is_health_probe) && (
                <div className='text-xs text-semi-color-text-2'>
                  {formatProbeReasonWithScope(record, t)}
                </div>
              )}
              {isFirstByteTimeoutAttempt(record) && (
                <div className='text-xs text-semi-color-text-2'>
                  {t('首字超时内部切换')}
                </div>
              )}
            </div>
          );
        },
      },
      {
        key: 'recent-duration',
        title: t('耗时'),
        dataIndex: 'duration_ms',
        width: 130,
        render: (_, record) => (
          <div>
            <Typography.Text strong>
              {formatLatency(record.duration_ms)}
            </Typography.Text>
            <div className='text-xs text-semi-color-text-2'>
              {t('首包')} {formatLatency(record.ttft_ms)}
            </div>
          </div>
        ),
      },
      {
        key: 'recent-score',
        title: t('稳定评分'),
        dataIndex: 'score_total',
        width: 120,
        render: (value) => formatScore(value),
      },
      {
        key: 'recent-queue-sticky',
        title: t('流转 / 队列'),
        dataIndex: 'queue_wait_ms',
        width: 300,
        render: (_, record) => (
          <div className='ct-model-gateway-record-flow-stack'>
            <DispatchFlowTags record={record} t={t} compact />
            <QueueStickyTags record={record} t={t} compact />
          </div>
        ),
      },
      {
        key: 'recent-actions',
        title: t('操作'),
        dataIndex: 'request_id',
        width: 130,
        fixed: 'right',
        render: (value, record) => (
          <div className='ct-model-gateway-row-actions'>
            <Tooltip content={t('调度详情')}>
              <Button
                size='small'
                type='tertiary'
                aria-label={t('调度详情')}
                icon={<Info size={14} />}
                onClick={() => setDetailRecord(record)}
              />
            </Tooltip>
            <Tooltip content={t('导出 Replay JSON')}>
              <Button
                size='small'
                type='tertiary'
                aria-label={t('导出 Replay JSON')}
                icon={<RotateCcw size={14} />}
                disabled={!value}
                onClick={() => exportReplay(value)}
              >
                {t('Replay')}
              </Button>
            </Tooltip>
          </div>
        ),
      },
    ],
    [exportReplay, t],
  );

  return (
    <div className='ct-model-gateway-page'>
      <div className='ct-model-gateway-hero'>
        <div className='ct-model-gateway-title-block'>
          <div className='ct-model-gateway-title-icon'>
            <ServerCog size={24} />
          </div>
          <div>
            <h2>{t('智能模型网关观测')}</h2>
            <p>
              {t('最近更新时间')}: {lastUpdated}
            </p>
          </div>
        </div>
        <ViewModeSwitch value={viewMode} onChange={setViewMode} t={t} />
        <div className='ct-model-gateway-actions'>
          <Button
            icon={<SlidersHorizontal size={15} />}
            type={filtersVisible ? 'primary' : 'tertiary'}
            onClick={() => setFiltersVisible((visible) => !visible)}
          >
            {hasActiveFilters ? t('筛选中') : t('筛选')}
          </Button>
          <Select
            value={hours}
            onChange={setHours}
            className='ct-model-gateway-window-select'
            prefix={t('时间窗口')}
          >
            {WINDOW_OPTIONS.map((option) => (
              <Select.Option key={option} value={option}>
                {option >= 24
                  ? `${Math.round(option / 24)} ${t('天')}`
                  : `${option} ${t('小时')}`}
              </Select.Option>
            ))}
          </Select>
          <Select
            value={trendBucket}
            onChange={setTrendBucket}
            className='ct-model-gateway-window-select'
            prefix={t('趋势粒度')}
          >
            {TREND_BUCKET_OPTIONS.map((option) => (
              <Select.Option key={option.value} value={option.value}>
                {t(option.labelKey)}
              </Select.Option>
            ))}
          </Select>
          <Button
            type='tertiary'
            icon={<Gauge size={15} />}
            onClick={() => setScoreBoostVisible(true)}
          >
            {t('渠道分值加成')}
          </Button>
          <Button
            theme='solid'
            type='primary'
            icon={<RefreshCw size={15} />}
            loading={refreshing}
            onClick={refreshDashboard}
          >
            {t('刷新')}
          </Button>
          <Tag
            color={refreshing ? 'blue' : realtimeMeta.color}
            type='light'
            className='ct-model-gateway-refresh-countdown'
          >
            {refreshing ? t('刷新中') : realtimeMeta.label}
          </Tag>
        </div>
      </div>

      {error && (
        <Banner
          type='danger'
          className='ct-model-gateway-banner'
          description={error}
          closeIcon={null}
        />
      )}

      {filtersVisible && (
        <DashboardCard bodyClassName='ct-model-gateway-filter-body'>
          <Input
            value={filters.model}
            onChange={(value) => updateFilter('model', value)}
            placeholder={t('按模型筛选')}
            prefix={t('模型')}
          />
          <Input
            value={filters.group}
            onChange={(value) => updateFilter('group', value)}
            placeholder={t('按分组筛选')}
            prefix={t('分组')}
          />
          {viewMode !== VIEW_MODES.USER_REQUESTS && (
            <Input
              value={filters.channel_id}
              onChange={(value) => updateFilter('channel_id', value)}
              placeholder={t('按渠道 ID 筛选')}
              prefix={t('渠道')}
            />
          )}
          <Input
            value={filters.request_id}
            onChange={(value) => updateFilter('request_id', value)}
            placeholder={t('按请求 ID 筛选')}
            prefix={t('请求 ID')}
          />
          {viewMode !== VIEW_MODES.USER_REQUESTS && (
            <Select
              value={filters.circuit_error_type}
              onChange={(value) =>
                updateFilter('circuit_error_type', value || '')
              }
              placeholder={t('按错误类型筛选')}
              prefix={t('错误类型')}
              showClear
              className='ct-model-gateway-filter-select'
            >
              {CIRCUIT_ERROR_TYPE_OPTIONS.map((type) => (
                <Select.Option key={type} value={type}>
                  {formatCircuitErrorType(type, t)}
                </Select.Option>
              ))}
            </Select>
          )}
          <div className='ct-model-gateway-filter-actions'>
            <Button type='primary' onClick={applyFilters}>
              {t('应用筛选')}
            </Button>
            <Button onClick={resetFilters} disabled={!hasActiveFilters}>
              {t('重置筛选')}
            </Button>
          </div>
        </DashboardCard>
      )}

      {loading ? (
        <MetricSkeleton />
      ) : viewMode === VIEW_MODES.USER_REQUESTS ? (
        <UserRequestDashboard
          data={data}
          t={t}
          refreshing={refreshing}
          onRefresh={refreshDashboard}
          onOpenDispatchDetail={openUserRequestDispatchDetail}
          onOpenScoreHistory={openScoreHistory}
          dispatchDetailLoading={dispatchDetailLoading}
          dynamicRefreshCountdown={dynamicRefreshCountdown}
          userRequestPageSize={userRequestPageSize}
          onUserRequestPageSizeChange={setUserRequestPageSize}
        />
      ) : viewMode === VIEW_MODES.OPERATIONS ? (
        <OperationsDashboard
          data={data}
          runtimeStatus={runtimeStatus}
          t={t}
          onReplayBatch={openReplayBatch}
        />
      ) : (
        <EngineeringSummaryDeck
          data={data}
          runtimeStatus={runtimeStatus}
          t={t}
          onReplayBatch={openReplayBatch}
          onRefreshSticky={() => setStickyRefreshToken((value) => value + 1)}
        />
      )}

      {!loading && viewMode === VIEW_MODES.ENGINEERING && (
        <div className='ct-model-gateway-section-heading'>
          <div>
            <span>{t('工程诊断详情')}</span>
            <p>
              {t(
                '保留原有运行态、队列、粘滞、风险与 Replay 明细，用于技术排障',
              )}
            </p>
          </div>
        </div>
      )}

      {!loading && viewMode === VIEW_MODES.ENGINEERING && (
        <div className='ct-model-gateway-insight-grid'>
          <StickyInsightPanel summary={summary} t={t} />
          <RuntimeRiskPanel runtimeStatus={runtimeStatus} t={t} />
        </div>
      )}

      {!loading && viewMode === VIEW_MODES.ENGINEERING && (
        <StickyStorePanel refreshToken={stickyRefreshToken} t={t} />
      )}

      {!loading && viewMode === VIEW_MODES.ENGINEERING && (
        <QueueRuntimePressurePanel
          data={data}
          runtimeStatus={runtimeStatus}
          t={t}
        />
      )}

      {!loading && viewMode === VIEW_MODES.ENGINEERING && (
        <RiskTimelinePanel
          risk={data?.risk}
          riskTimeline={data?.risk_timeline || data?.risk_events}
          t={t}
          circuitErrorType={appliedFilters.circuit_error_type}
        />
      )}

      {!loading && viewMode === VIEW_MODES.ENGINEERING && (
        <DispatchTrendPanel
          trends={data?.trends}
          t={t}
          onExport={exportTrends}
          circuitErrorType={appliedFilters.circuit_error_type}
        />
      )}

      {!loading && viewMode === VIEW_MODES.ENGINEERING && (
        <RuntimeStatusPanel
          runtimeStatus={runtimeStatus}
          t={t}
          circuitErrorType={appliedFilters.circuit_error_type}
        />
      )}

      {!loading && !hasData ? (
        <DashboardCard bodyStyle={{ minHeight: 280 }}>
          <EmptyState t={t} />
        </DashboardCard>
      ) : viewMode === VIEW_MODES.USER_REQUESTS ? null : viewMode ===
        VIEW_MODES.ENGINEERING ? (
        <>
          <div className='ct-model-gateway-aggregate-grid'>
            <DashboardCard
              title={
                <span className='ct-model-gateway-panel-title'>
                  <Bot size={17} />
                  {t('按模型聚合')}
                </span>
              }
              bodyStyle={{ padding: 0 }}
            >
              <Table
                size='small'
                columns={aggregateColumns('model')}
                dataSource={data?.by_model || []}
                rowKey='key'
                pagination={false}
                empty={<EmptyState t={t} />}
                scroll={{ x: 1380 }}
              />
            </DashboardCard>
            <DashboardCard
              title={
                <span className='ct-model-gateway-panel-title'>
                  <Layers3 size={17} />
                  {t('按分组聚合')}
                </span>
              }
              bodyStyle={{ padding: 0 }}
            >
              <Table
                size='small'
                columns={aggregateColumns('group')}
                dataSource={data?.by_group || []}
                rowKey='key'
                pagination={false}
                empty={<EmptyState t={t} />}
                scroll={{ x: 1640 }}
              />
            </DashboardCard>
          </div>

          <DashboardCard
            title={
              <span className='ct-model-gateway-panel-title'>
                <RadioTower size={17} />
                {t('按渠道聚合')}
              </span>
            }
            bodyStyle={{ padding: 0 }}
          >
            <Table
              size='small'
              columns={aggregateColumns('channel')}
              dataSource={data?.by_channel || []}
              rowKey={(record) => record.channel_id || record.key}
              pagination={false}
              empty={<EmptyState t={t} />}
              scroll={{ x: 1380 }}
            />
          </DashboardCard>

          <div className='ct-model-gateway-aggregate-grid'>
            <DashboardCard
              title={
                <span className='ct-model-gateway-panel-title'>
                  <ServerCog size={17} />
                  {t('按 Provider Profile 聚合')}
                </span>
              }
              bodyStyle={{ padding: 0 }}
            >
              <Table
                size='small'
                columns={aggregateColumns('profile')}
                dataSource={data?.by_provider_profile || []}
                rowKey='key'
                pagination={false}
                empty={<EmptyState t={t} />}
                scroll={{ x: 1380 }}
              />
            </DashboardCard>
            <DashboardCard
              title={
                <span className='ct-model-gateway-panel-title'>
                  <GitBranch size={17} />
                  {t('按 Proxy Mode 聚合')}
                </span>
              }
              bodyStyle={{ padding: 0 }}
            >
              <Table
                size='small'
                columns={aggregateColumns('proxy')}
                dataSource={data?.by_proxy_mode || []}
                rowKey='key'
                pagination={false}
                empty={<EmptyState t={t} />}
                scroll={{ x: 1380 }}
              />
            </DashboardCard>
          </div>

          <DashboardCard
            title={
              <div className='ct-model-gateway-panel-title-row'>
                <span className='ct-model-gateway-panel-title'>
                  <Gauge size={17} />
                  {t('最近调度记录')}
                </span>
                <Button
                  size='small'
                  type='tertiary'
                  icon={<Download size={14} />}
                  onClick={openReplayBatch}
                >
                  {t('批量导出 Replay JSON')}
                </Button>
              </div>
            }
            bodyStyle={{ padding: 0 }}
          >
            <Table
              size='small'
              columns={recentColumns}
              dataSource={data?.recent_records || []}
              rowKey={(record) => `${record.id}-${record.kind}`}
              pagination={false}
              empty={<EmptyState t={t} />}
              scroll={{ x: 1750 }}
            />
          </DashboardCard>
        </>
      ) : (
        <OperationalRecentRecords
          records={data?.recent_records || []}
          t={t}
          onOpenDetail={setDetailRecord}
          onExportReplay={exportReplay}
        />
      )}

      <ChannelScoreBoostModal
        visible={scoreBoostVisible}
        onCancel={() => setScoreBoostVisible(false)}
        onSaved={refreshDashboard}
        t={t}
      />
      <ReplayModal
        artifact={replayArtifact}
        loading={replayLoading}
        visible={replayVisible}
        onCancel={() => setReplayVisible(false)}
        requestId={replayRequestId}
        t={t}
      />
      <ReplayBatchModal
        artifact={replayBatchArtifact}
        filters={replayBatchFilters}
        loading={replayBatchLoading}
        visible={replayBatchVisible}
        onCancel={() => setReplayBatchVisible(false)}
        onDownload={downloadReplayBatch}
        onFilterChange={updateReplayBatchFilter}
        onPreview={previewReplayBatch}
        t={t}
      />
      <ScoreHistoryModal
        history={scoreHistory}
        loading={scoreHistoryLoading}
        visible={scoreHistoryVisible}
        onCancel={() => setScoreHistoryVisible(false)}
        t={t}
      />
      <RecordDetailDrawer
        record={detailRecord}
        visible={!!detailRecord}
        onClose={() => setDetailRecord(null)}
        onExportReplay={exportReplay}
        onOpenScoreHistory={openScoreHistory}
        onRuntimeCircuitCleared={refreshDashboard}
        scoreHistory={detailScoreHistory}
        scoreHistoryLoading={detailScoreHistoryLoading}
        t={t}
      />
    </div>
  );
}
