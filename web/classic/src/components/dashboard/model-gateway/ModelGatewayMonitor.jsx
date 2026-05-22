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
  Modal,
  Select,
  SideSheet,
  Skeleton,
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
const USER_REQUEST_PAGE_SIZE = 20;
const TOP_N = 10;
const STICKY_STORE_LIMIT = 100;
const WINDOW_OPTIONS = [1, 6, 24, 72, 168];
const DEFAULT_TREND_BUCKET = 'auto';
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
    case 'explore_baseline':
      return t('探索基线');
    case 'success':
    case 'success_score':
      return t('成功分');
    case 'score_speed_factor':
    case 'speed':
      return t('速度因子');
    case 'speed_score':
      return t('动态速度分');
    case 'load':
    case 'load_score':
      return t('路由负载分');
    case 'routing_total':
      return t('调度分');
    case 'ttft_pending':
      return t('首包等待');
    case 'cost':
    case 'cost_score':
      return t('成本分');
    case 'group':
    case 'group_score':
      return t('分组分');
    case 'experience':
    case 'experience_score':
      return t('体验分');
    case 'confidence_samples':
      return t('样本置信');
    default:
      return normalized || t('未知指标');
  }
}

function scoreMetricDescription(key, t) {
  const normalized = String(key || '').trim();
  switch (normalized) {
    case 'explore_baseline':
      return t('暂无真实样本时仅用于调度探索，不代表渠道真实速度或成功率');
    case 'success':
    case 'success_score':
      return t('近期成功率越高，这项越高');
    case 'score_speed_factor':
    case 'speed':
      return t('用于本次评分的速度因子，会叠加样本置信和首包惩罚');
    case 'speed_score':
      return t('近期成功样本的首包或总耗时去头去尾均值换算分');
    case 'load':
    case 'load_score':
      return t('当前并发远低于上限时影响很小，接近满载才会明显降分');
    case 'routing_total':
      return t('用于调度选择，会叠加当前压力和排队状态');
    case 'ttft_pending':
      return t('仅影响调度避让，不计入渠道健康评分');
    case 'cost':
    case 'cost_score':
      return t('成本倍率越高，这项越低');
    case 'group':
    case 'group_score':
      return t('分组权重越高，这项越高');
    case 'experience':
    case 'experience_score':
      return t('空输出或体验异常越多，这项越低');
    case 'confidence_samples':
      return t('历史样本越多，动态评分越可信');
    default:
      return '';
  }
}

function scoreMetricEntries(value = {}, delta = {}, t) {
  return Object.entries(value || {})
    .filter(([, score]) => Number.isFinite(Number(score)))
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

function importantScoreDeltas(delta = {}, totalDelta = 0, t) {
  const direction = scoreDeltaTone(totalDelta);
  const entries = Object.entries(delta || {})
    .filter(([, value]) => Number.isFinite(Number(value)))
    .map(([key, value]) => ({ key, value: Number(value) }))
    .filter((entry) => Math.abs(entry.value) >= 0.0001);
  const directional = entries.filter((entry) => {
    const tone = scoreDeltaTone(entry.value);
    return direction === 'neutral' || tone === direction;
  });
  const candidates = directional.length ? directional : entries;
  return candidates
    .sort((left, right) => Math.abs(right.value) - Math.abs(left.value))
    .slice(0, 3)
    .map((entry) => ({
      ...entry,
      label: scoreMetricLabel(entry.key, t),
      description: scoreMetricDescription(entry.key, t),
    }));
}

function scoreHistorySampleLabel(value, t) {
  const count = Number(value || 0);
  return count > 0 ? formatNumber(count) : t('暂无真实样本');
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

function getStatusMeta(record, t) {
  if (record?.is_health_probe || record?.request_meta?.is_health_probe) {
    if (record?.success) {
      return { color: 'cyan', label: t('健康探活') };
    }
    return { color: 'orange', label: t('探活异常') };
  }
  if (record?.kind === 'user_request_detail') {
    if (record?.client_aborted) {
      return { color: 'grey', label: t('用户取消') };
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

function isBalanceInsufficientStatus(record) {
  const reason = String(record?.status_reason || record?.reject_reason || '')
    .trim()
    .toLowerCase();
  return (
    record?.balance_insufficient === true ||
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
  if (normalized === 'cooldown') {
    return t('临时冷却');
  }
  if (normalized === 'failure_avoidance') {
    return t('近期失败避让');
  }
  if (normalized === 'max_depth_reached') {
    return t('排队已满');
  }
  return String(reason);
}

function formatProbeReason(value, t) {
  const normalized = String(value || '').trim();
  switch (normalized) {
    case 'missing_samples':
      return t('缺少样本');
    case 'low_score':
      return t('低分恢复');
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

function isDispatch(record) {
  return record?.kind === 'dispatch';
}

function pickDispatchDetailRecord(records) {
  if (!Array.isArray(records) || records.length === 0) return null;
  return (
    records.find((record) => isDispatch(record) && record?.smart_handled) ||
    records.find((record) => isDispatch(record)) ||
    records.find(
      (record) =>
        Array.isArray(getCandidateExplanations(record)) &&
        getCandidateExplanations(record).length > 0,
    ) ||
    records[0]
  );
}

function pickAttemptDetailRecord(records) {
  if (!Array.isArray(records) || records.length === 0) return null;
  return (
    records.find((record) => !isDispatch(record) && record?.success) ||
    records.find((record) => !isDispatch(record)) ||
    null
  );
}

function buildUserRequestDetailRecord(userRequest, records) {
  const dispatch = pickDispatchDetailRecord(records);
  const attempt = pickAttemptDetailRecord(records);
  const base = dispatch || attempt || {};
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
    channel_id: finalChannelId,
    channel_name: finalChannelName,
    actual_channel_id: finalChannelId,
    actual_channel_name: finalChannelName,
    success: userRequest?.final_success === true || attempt?.success === true,
    final_success: userRequest?.final_success === true,
    status_code: userRequest?.final_status_code || attempt?.status_code || 0,
    error_category:
      userRequest?.final_error_category || attempt?.error_category || '',
    duration_ms: userRequest?.duration_ms || attempt?.duration_ms || 0,
    ttft_ms: userRequest?.ttft_ms || attempt?.ttft_ms || 0,
    client_aborted:
      userRequest?.client_aborted === true || attempt?.client_aborted === true,
    stream_interrupted:
      userRequest?.stream_interrupted === true ||
      attempt?.stream_interrupted === true,
    empty_output:
      userRequest?.empty_output === true || attempt?.empty_output === true,
    experience_issue:
      userRequest?.experience_issue || attempt?.experience_issue || '',
    recovered: userRequest?.recovered === true,
    attempts: userRequest?.attempts || records?.length || 0,
    score_total: dispatch?.score_total || base?.score_total || 0,
    score_breakdown: dispatch?.score_breakdown || base?.score_breakdown,
    candidate_groups: dispatch?.candidate_groups || base?.candidate_groups,
    candidate_explanations:
      dispatch?.candidate_explanations ||
      dispatch?.request_meta?.candidate_explanations ||
      base?.candidate_explanations,
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
      candidate_explanations:
        dispatch?.candidate_explanations ||
        dispatch?.request_meta?.candidate_explanations ||
        [],
    },
  };
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
        ? `: ${record.sticky_break}`
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

function formatAttemptFlowAction(action, t) {
  switch (action) {
    case 'complete':
      return t('请求完成');
    case 'switch_channel':
      return t('切换渠道');
    case 'retry':
      return t('继续重试');
    case 'stop':
      return t('停止重试');
    default:
      return action || '--';
  }
}

function formatAttemptErrorCategory(category, t) {
  switch (category) {
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
  const activeConcurrency = Number(record?.active_concurrency || 0);
  const configuredLimit = Number(record?.configured_concurrency_limit || 0);
  const learnedLimit = Number(record?.learned_concurrency_limit || 0);
  const usedChannels = getUsedChannels(record);

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
  if (category) {
    tags.push(
      <Tag key='category' color='grey' type='light' {...tagProps}>
        {t('失败分类')}: {formatAttemptErrorCategory(category, t)}
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

function getRuntimeHealthMeta(status, t) {
  const normalized = status || 'healthy';
  switch (normalized) {
    case 'circuit_open':
      return { color: 'red', label: t('熔断打开') };
    case 'cooldown':
      return { color: 'orange', label: t('冷却') };
    case 'failure_avoidance':
      return { color: 'orange', label: t('失败降权') };
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
  const requests = Number(summary.total_requests || 0);
  const successRate = requests > 0 ? Number(summary.user_success_rate || 0) : 1;
  const finalFailures = Number(summary.final_failures || 0);
  const recovered = Number(summary.recovered || 0);
  const emptyOutputs = Number(summary.empty_outputs || 0);
  const experienceIssues = Number(summary.experience_issues || 0);
  const p95TtftMs = Number(summary.p95_ttft_ms || 0);
  let score = requests > 0 ? 100 : 0;
  if (requests > 0) {
    score -= Math.min(46, Math.max(0, 0.995 - successRate) * 560);
    score -= Math.min(16, (finalFailures / Math.max(1, requests)) * 160);
    score -= Math.min(
      18,
      ((emptyOutputs + experienceIssues) / Math.max(1, requests)) * 120,
    );
  }
  if (p95TtftMs >= LATENCY_THRESHOLDS.p95TtftMs.danger) score -= 16;
  else if (p95TtftMs >= LATENCY_THRESHOLDS.p95TtftMs.warning) score -= 8;
  if (recovered > 0)
    score -= Math.min(8, (recovered / Math.max(1, requests)) * 22);
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
  if (record?.status === 'processing') {
    return { color: 'blue', label: t('处理中'), tone: 'processing' };
  }
  if (
    record?.client_aborted ||
    record?.status === 'client_aborted' ||
    record?.final_error_category === 'client_aborted' ||
    Number(record?.final_status_code || 0) === 499
  ) {
    return { color: 'grey', label: t('用户取消'), tone: 'aborted' };
  }
  if (record?.final_success) {
    if (record?.empty_output || record?.experience_issue) {
      return { color: 'orange', label: t('体验异常'), tone: 'warning' };
    }
    return record?.recovered
      ? { color: 'cyan', label: t('已恢复成功'), tone: 'recovered' }
      : { color: 'green', label: t('成功'), tone: 'success' };
  }
  return { color: 'red', label: t('最终失败'), tone: 'failed' };
}

function isUserRequestProcessing(record) {
  return record?.status === 'processing' || !Number(record?.completed_at || 0);
}

function formatUserRequestErrorCategory(category, t) {
  switch (category) {
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
    case 'server_error':
      return t('最终失败类型：server_error');
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

function userRequestMatchesQuery(record, query) {
  const normalizedQuery = String(query || '')
    .trim()
    .toLowerCase();
  if (!normalizedQuery) return true;
  return [
    record?.request_id,
    record?.requested_model,
    record?.requested_group,
    record?.selected_group,
    record?.final_channel_id,
    record?.final_channel_name,
    record?.final_error_category,
  ]
    .map((value) => String(value || '').toLowerCase())
    .some((value) => value.includes(normalizedQuery));
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
  if (requestedGroup && selectedGroup && requestedGroup !== selectedGroup) {
    return `${requestedGroup}=>${selectedGroup}`;
  }
  return selectedGroup || requestedGroup || '--';
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

function formatUserRequestGroupRatio(record) {
  const ratio = Number(record?.billing?.group_ratio || 0);
  if (!Number.isFinite(ratio) || ratio <= 0) return '';
  return ratio.toFixed(4).replace(/0+$/, '').replace(/\.$/, '');
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
  const rows = [
    [t('状态'), meta?.label || '--'],
    [
      t('事件'),
      processing
        ? t('请求仍在处理中')
        : record?.client_aborted
          ? t('用户主动终止')
          : record?.recovered
            ? t('内部失败后最终成功')
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
    [t('首包延迟'), processing ? '--' : formatLatency(record?.ttft_ms)],
  ];

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

function UserRequestCostCell({ billing, t }) {
  if (!billing) {
    return (
      <div className='ct-model-gateway-user-request-cost-col ct-model-gateway-user-request-cost-empty'>
        <HoverCard
          content={<UserRequestCostTooltip billing={billing} t={t} />}
          className='ct-model-gateway-user-request-cost-trigger'
        >
          <strong>--</strong>
          <span>{t('暂无日志')}</span>
        </HoverCard>
      </div>
    );
  }

  return (
    <div className='ct-model-gateway-user-request-cost-col'>
      <HoverCard
        content={<UserRequestCostTooltip billing={billing} t={t} />}
        className='ct-model-gateway-user-request-cost-trigger'
      >
        <strong>{renderQuota(billingReferenceQuota(billing), 6)}</strong>
        <span>
          {safeNumber(billing.total_tokens) > 0
            ? `${formatBillingTokenCount(billing.total_tokens)} ${t('tokens')}`
            : t('费用参考')}
        </span>
      </HoverCard>
    </div>
  );
}

function userRequestSortTimestamp(record) {
  return (
    normalizeTimestamp(record?.created_at) ||
    normalizeTimestamp(record?.completed_at) ||
    normalizeTimestamp(record?.updated_at) ||
    0
  );
}

function compareUserRequestsForDisplay(a, b) {
  const aProcessing = isUserRequestProcessing(a);
  const bProcessing = isUserRequestProcessing(b);
  if (aProcessing !== bProcessing) {
    return aProcessing ? -1 : 1;
  }
  return userRequestSortTimestamp(b) - userRequestSortTimestamp(a);
}

function buildUserRequestSparkValues(trends, key) {
  if (!Array.isArray(trends)) return [];
  return trends
    .filter((item) => Number(item?.requests) > 0)
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
            <span>{t('健康分')}</span>
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
          <small>{t('最终失败')}</small>
          <strong>{formatNumber(summary.final_failures)}</strong>
          <span>
            {formatNumber(summary.total_requests)} {t('用户请求')}
          </span>
        </div>
        <div>
          <small>{t('自动恢复')}</small>
          <strong>{formatNumber(summary.recovered)}</strong>
          <span>{t('内部重试后成功')}</span>
        </div>
      </div>
    </DashboardCard>
  );
}

function UserRequestTrendPanel({ trends, t }) {
  const spec = useMemo(() => buildUserRequestTrendSpec(trends, t), [trends, t]);
  return (
    <DashboardCard
      title={
        <span className='ct-model-gateway-panel-title'>
          <Activity size={17} />
          {t('用户请求趋势')}
        </span>
      }
      bodyClassName='ct-model-gateway-user-trend-body'
    >
      {Array.isArray(trends) &&
      trends.some((item) => Number(item?.requests) > 0) ? (
        <VChart spec={spec} option={MINI_SPARKLINE_CHART_OPTIONS} />
      ) : (
        <div className='ct-model-gateway-trend-empty'>
          {t('暂无用户请求趋势')}
        </div>
      )}
    </DashboardCard>
  );
}

function UserRequestRankPanel({ title, icon: Icon, rows, type, t }) {
  const items = [...(rows || [])]
    .filter((item) => Number(item?.requests || 0) > 0)
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
        <span>{t('自动恢复')}</span>
        <span>{t('请求量')}</span>
      </div>
      {items.length ? (
        items.map((item) => {
          const rate = clampRate(item.user_success_rate);
          const tone = getSuccessTone(item.user_success_rate, item.requests);
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
                  {formatAttemptRate(item.user_success_rate, item.requests)}
                </span>
                <div className='ct-model-gateway-leaderboard-meter'>
                  <span style={{ width: `${Math.round(rate * 100)}%` }} />
                </div>
              </div>
              <span>{formatLatency(item.p95_ttft_ms)}</span>
              <span>{formatNumber(item.recovered)}</span>
              <span>{formatNumber(item.requests)}</span>
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
  dispatchDetailLoading,
}) {
  const [nowSeconds, setNowSeconds] = useState(() =>
    Math.floor(Date.now() / 1000),
  );
  const [query, setQuery] = useState('');
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
        .filter((record) => userRequestMatchesQuery(record, query))
        .sort(compareUserRequestsForDisplay),
    [query, records],
  );
  const totalPages = Math.max(
    1,
    Math.ceil(filteredItems.length / USER_REQUEST_PAGE_SIZE),
  );
  const currentPage = Math.min(page, totalPages);
  const pageItems = filteredItems.slice(
    (currentPage - 1) * USER_REQUEST_PAGE_SIZE,
    currentPage * USER_REQUEST_PAGE_SIZE,
  );

  useEffect(() => {
    setPage(1);
  }, [query, records]);

  useEffect(() => {
    setJumpPage(String(currentPage));
  }, [currentPage]);

  const gotoPage = useCallback(
    (nextPage) => {
      setPage(Math.min(totalPages, Math.max(1, nextPage)));
    },
    [totalPages],
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
          <h3>{t('最近用户请求')}</h3>
          <p>{t('按时间倒序展示最近的请求记录')}</p>
        </div>
        <div className='ct-model-gateway-user-request-list-actions'>
          <Input
            value={query}
            onChange={setQuery}
            prefix={<Search size={14} />}
            placeholder={t('搜索请求 ID')}
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
              { key: 'status', label: t('状态') },
              { key: 'request', label: t('请求信息') },
              { key: 'group', label: t('渠道信息') },
              { key: 'cost', label: t('费用'), hint: true },
              { key: 'duration', label: t('总耗时'), hint: true },
              { key: 'ttft', label: t('首包'), hint: true },
              { key: 'complete', label: t('完成时间') },
              { key: 'action', label: t('操作') },
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
                const channelLabel = formatUserRequestChannel(record);
                const channelIdLabel = formatUserRequestChannelId(record);
                const groupFlowLabel = formatUserRequestGroupFlow(record);
                const groupRatioLabel = formatUserRequestGroupRatio(record);
                const StatusIcon =
                  meta.tone === 'processing'
                    ? RadioTower
                    : meta.tone === 'aborted'
                      ? Ban
                      : meta.tone === 'failed'
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

                return (
                  <div
                    className={`ct-model-gateway-user-request-row ct-model-gateway-user-request-row-${meta.tone}`}
                    key={requestId || record.id}
                  >
                    <div className='ct-model-gateway-user-request-status-col'>
                      <div
                        className={`ct-model-gateway-user-request-status-pill ct-model-gateway-user-request-status-pill-${meta.tone}`}
                      >
                        <StatusIcon size={13} />
                        <span>{meta.label}</span>
                      </div>
                      <small
                        className={
                          !processing && record.recovered
                            ? ''
                            : 'ct-model-gateway-user-request-hidden-line'
                        }
                      >
                        {t('自动恢复')}
                      </small>
                    </div>

                    <div className='ct-model-gateway-user-request-info-col'>
                      <span>{t('请求 ID')}</span>
                      <div className='ct-model-gateway-user-request-id-line'>
                        <Typography.Text
                          ellipsis={{ showTooltip: true }}
                          className='ct-model-gateway-request-id'
                        >
                          {requestId || '--'}
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
                      {!record.final_success &&
                        record.final_error_category &&
                        meta.tone !== 'aborted' &&
                        !processing && (
                          <small className='ct-model-gateway-user-request-error'>
                            {formatUserRequestErrorCategory(
                              record.final_error_category,
                              t,
                            )}
                          </small>
                        )}
                      {record.final_success &&
                        (record.empty_output || record.experience_issue) &&
                        !processing && (
                          <small className='ct-model-gateway-user-request-error'>
                            {formatUserRequestExperienceIssue(
                              record.experience_issue ||
                                (record.empty_output ? 'empty_output' : ''),
                              t,
                            )}
                          </small>
                        )}
                    </div>

                    <div className='ct-model-gateway-user-request-group-col'>
                      <div
                        className='ct-model-gateway-user-request-channel-primary'
                        title={channelLabel}
                      >
                        <strong>{channelLabel}</strong>
                        {channelIdLabel && (
                          <code title={t('渠道 ID')}>{channelIdLabel}</code>
                        )}
                        {groupRatioLabel && (
                          <em title={t('分组倍率')}>
                            {t('分组倍率')} {groupRatioLabel}
                          </em>
                        )}
                      </div>
                      <div
                        className='ct-model-gateway-user-request-channel-model'
                        title={record.requested_model || '--'}
                      >
                        {record.requested_model || '--'}
                      </div>
                      <div
                        className='ct-model-gateway-user-request-channel-flow'
                        title={groupFlowLabel}
                      >
                        {groupFlowLabel}
                      </div>
                    </div>

                    <UserRequestCostCell billing={record.billing} t={t} />

                    <div
                      className={`ct-model-gateway-user-request-value-col ct-model-gateway-user-request-value-${durationTone}`}
                    >
                      <strong>{formatLatency(durationMs)}</strong>
                      <span>
                        {processing ? t('实时处理中') : t('仅供排查')}
                      </span>
                    </div>

                    <div
                      className={`ct-model-gateway-user-request-value-col ct-model-gateway-user-request-value-${ttftTone}`}
                    >
                      <strong>
                        {processing ? '--' : formatLatency(record.ttft_ms)}
                      </strong>
                      <span>{processing ? t('等待完成') : t('首包延迟')}</span>
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
                        {processing
                          ? t('进行中')
                          : meta.tone === 'aborted'
                            ? t('用户已取消')
                            : record.final_success
                              ? t('请求完成')
                              : t('请求失败')}
                      </span>
                    </div>

                    <div className='ct-model-gateway-user-request-action-col'>
                      <Tooltip content={t('调度详情')}>
                        <Button
                          size='small'
                          type='tertiary'
                          aria-label={t('查看详情')}
                          icon={<Eye size={14} />}
                          loading={dispatchDetailLoading === requestId}
                          disabled={!requestId}
                          onClick={() => onOpenDispatchDetail?.(record)}
                        >
                          {t('查看')}
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
        <div className='ct-model-gateway-user-request-page-size'>
          {USER_REQUEST_PAGE_SIZE} {t('条/页')}
        </div>
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
  dispatchDetailLoading,
}) {
  const userRequests = data?.user_requests || {};
  const summary = userRequests.summary || {};
  const trends = userRequests.trends || [];
  const health = getUserRequestHealth(data);

  return (
    <div className='ct-model-gateway-user-layout'>
      <div className='ct-model-gateway-user-top'>
        <UserRequestHealthCard
          health={health}
          summary={summary}
          trends={trends}
          t={t}
        />
        <div className='ct-model-gateway-user-kpi-grid'>
          <OperationKpiCard
            icon={CheckCircle2}
            label={t('用户成功率')}
            value={formatAttemptRate(
              summary.user_success_rate,
              summary.total_requests,
            )}
            detail={`${formatNumber(summary.successes)} / ${formatNumber(
              summary.total_requests,
            )}`}
            tone={getSuccessTone(
              summary.user_success_rate,
              summary.total_requests,
            )}
            sparkValues={buildUserRequestSparkValues(
              trends,
              'user_success_rate',
            )}
          />
          <OperationKpiCard
            icon={ListTree}
            label={t('用户请求数')}
            value={formatNumber(summary.total_requests)}
            detail={`${formatNumber(summary.scanned_requests)} ${t('已扫描')}`}
            tone='default'
            sparkValues={buildUserRequestSparkValues(trends, 'requests')}
          />
          <OperationKpiCard
            icon={GitBranch}
            label={t('自动恢复')}
            value={formatNumber(summary.recovered)}
            detail={t('内部失败后最终成功')}
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
        dispatchDetailLoading={dispatchDetailLoading}
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
      label: t('用户请求视图'),
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
            summary.failure_avoidance_channels,
          )} ${t('失败降权')}`}
          tone={
            summary.cooldown_channels > 0 ||
            summary.failure_avoidance_channels > 0
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
                {t('失败降权')}{' '}
                {formatNumber(record.failure_avoidance_remaining_seconds)}s
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
              {t('分组倍率')} {formatScore(record.group_priority_ratio)}
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
            summary.failure_avoidance_channels,
          )} ${t('失败降权')}`}
          tone={
            summary.cooldown_channels > 0 ||
            summary.failure_avoidance_channels > 0
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

function getCandidateExplanations(record) {
  const topLevel = record?.candidate_explanations;
  const metaLevel = record?.request_meta?.candidate_explanations;
  if (Array.isArray(topLevel) && topLevel.length) return topLevel;
  if (Array.isArray(metaLevel) && metaLevel.length) return metaLevel;
  return [];
}

function formatRuntimeKey(runtimeKey) {
  if (!runtimeKey) return '--';
  if (typeof runtimeKey === 'string') return runtimeKey;
  if (typeof runtimeKey !== 'object') return String(runtimeKey);
  return [
    runtimeKey.requested_model,
    runtimeKey.upstream_model,
    runtimeKey.channel_id ? `#${runtimeKey.channel_id}` : '',
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
    return t('当前分数 {{score}}，暂无上一条可对比记录', {
      score: formatScore(current.score_total),
    });
  }
  const important = importantScoreDeltas(
    history?.metric_deltas || current?.score_breakdown_delta,
    delta,
    t,
  );
  if (scoreDeltaTone(delta) === 'neutral') {
    return t('当前分数 {{score}}，相比上一条基本未变化', {
      score: formatScore(current.score_total),
    });
  }
  const reason = important.length
    ? important
        .map((item) => `${item.label}${formatScoreDelta(item.value)}`)
        .join(t('、'))
    : t('综合指标变化');
  if (delta > 0) {
    return t(
      '当前分数 {{score}}，比上一条提高 {{delta}}，主要因为 {{reason}}',
      {
        score: formatScore(current.score_total),
        delta: formatScoreDeltaMagnitude(delta),
        reason,
      },
    );
  }
  return t('当前分数 {{score}}，比上一条下降 {{delta}}，主要因为 {{reason}}', {
    score: formatScore(current.score_total),
    delta: formatScoreDeltaMagnitude(delta),
    reason,
  });
}

function buildScoreHistoryItemReasons(item, t) {
  const reasons = [];
  const sampleCount = Number(item?.sample_count || 0);
  const scoreDelta = Number(item?.score_delta || 0);
  const selectionReason = formatSelectionReason(item?.selected_reason, t);
  const important = importantScoreDeltas(
    item?.score_breakdown_delta,
    scoreDelta,
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
    const reason = important
      .map((entry) => `${entry.label}${formatScoreDelta(entry.value)}`)
      .join(t('、'));
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

function getCandidateScore(candidate) {
  const score = Number(candidate?.score_total);
  return Number.isFinite(score) && score > 0 ? score : null;
}

function getCandidateRoutingScore(candidate) {
  const score = Number(candidate?.routing_score_total);
  return Number.isFinite(score) && score > 0 ? score : null;
}

function getCandidateSelectionScore(candidate) {
  return getCandidateRoutingScore(candidate) ?? getCandidateScore(candidate);
}

function getCandidateScoreSpeedFactor(candidate) {
  const score = Number(
    candidate?.score_speed_factor ?? candidate?.score_breakdown?.speed,
  );
  return Number.isFinite(score) && score > 0 ? score : null;
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
  const label = formatChannelStatusReason(reason, t);
  if (!label) return '';
  return t('原粘滞候选未被保留，原因是 {{reason}}。', {
    reason: label,
  });
}

function buildSelectionSummaryText(insight, t) {
  const label = insight.selectedLabel || '--';
  if (insight.stickyRetained) {
    return t('本次最终选择 {{channel}}，因为命中粘滞路由且满足保留阈值。', {
      channel: label,
    });
  }
  if (insight.stickyBroken) {
    return t(
      '本次最终选择 {{channel}}，因为原粘滞候选未满足保留条件，系统改选当前调度分更优的可用候选。',
      {
        channel: label,
      },
    );
  }
  if (insight.selectedTopTie && insight.topTieCount > 1) {
    return t(
      '本次最终选择 {{channel}}，因为多个候选本次调度分并列最高，系统按候选顺序保留先出现的渠道。',
      {
        channel: label,
      },
    );
  }
  if (insight.selectedTopTie) {
    return t(
      '本次最终选择 {{channel}}，因为它在当前可用候选中本次调度分最高。',
      {
        channel: label,
      },
    );
  }
  return t('本次最终选择 {{channel}}，系统根据当前策略从可用候选中完成调度。', {
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
    getCandidateRoutingScore(record) ??
    getCandidateScore(record);
  const selectedHealthScore = getCandidateScore(selectedCandidate);
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
  const selectedTtftMs = Number(selectedCandidate?.ttft_ms || 0);
  const selectedDurationMs = Number(selectedCandidate?.duration_ms || 0);
  const selectedSamples = Number(selectedCandidate?.sample_count || 0);
  const selectedHasRealSamples = selectedSamples > 0;
  const selectedSpeedScore = Number(selectedCandidate?.speed_score || 0);
  const selectedScoreSpeedFactor =
    getCandidateScoreSpeedFactor(selectedCandidate);
  const selectedExperienceScore = Number(
    selectedCandidate?.experience_score || 0,
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
  let explanation = t('根据当前策略从可用候选中完成选择');

  if (selectedTopTie && topTieCount > 1 && rawReason === 'weighted_score') {
    explanation = t('本次调度分并列最高，按候选顺序保留先出现候选');
  } else if (selectedTopTie && rawReason === 'weighted_score') {
    explanation = t('在可用候选中本次调度分最高');
  } else if (stickyRetained) {
    explanation = t('命中粘滞路由并满足保留阈值，优先复用该渠道');
  } else if (stickyBroken) {
    explanation = t('原粘滞候选未满足保留条件，改选当前调度分更优候选');
  }

  return {
    selectedCandidate,
    selectedLabel,
    reasonLabel,
    rawReason,
    explanation,
    stickyRetained,
    stickyBroken,
    stickySource: formatStickySource(record?.sticky_source, t),
    stickyBreakText,
    selectedRank,
    selectedTopTie,
    topTieCount,
    summary: '',
    metrics: [
      {
        key: 'score',
        label: selectedHasRealSamples ? t('本次调度分') : t('探索参考'),
        value: selectedHasRealSamples
          ? formatScore(selectedScore)
          : t('暂无真实样本'),
      },
      {
        key: 'health_score',
        label: selectedHasRealSamples ? t('健康评分') : t('健康评分'),
        value:
          selectedHasRealSamples && selectedHealthScore !== null
            ? formatScore(selectedHealthScore)
            : '--',
      },
      {
        key: 'speed',
        label: t('动态速度分'),
        value:
          selectedHasRealSamples && selectedSpeedScore > 0
            ? formatScore(selectedSpeedScore)
            : t('暂无真实样本'),
      },
      {
        key: 'score_speed_factor',
        label: t('速度因子'),
        value:
          selectedHasRealSamples && selectedScoreSpeedFactor !== null
            ? formatScore(selectedScoreSpeedFactor)
            : '--',
      },
      {
        key: 'latency',
        label: t('动态首包'),
        value: selectedTtftMs > 0 ? formatLatency(selectedTtftMs) : '--',
      },
      {
        key: 'duration',
        label: t('动态耗时'),
        value:
          selectedDurationMs > 0 ? formatLatency(selectedDurationMs) : '--',
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
        value: formatScoreSampleSource(
          selectedCandidate?.score_sample_source,
          t,
        ),
      },
      {
        key: 'samples',
        label: t('评分样本'),
        value:
          selectedSamples > 0
            ? formatNumber(selectedSamples)
            : t('暂无真实样本'),
      },
      {
        key: 'experience',
        label: t('体验分'),
        value:
          selectedHasRealSamples && selectedExperienceScore > 0
            ? formatScore(selectedExperienceScore)
            : '--',
      },
    ],
    scoreEntries: Object.entries(
      selectedCandidate?.score_breakdown || record?.score_breakdown || {},
    ).filter(
      ([key, score]) =>
        Number.isFinite(Number(score)) &&
        scoreEntryIsVisible(key, selectedSamples),
    ),
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
          {insight.metrics.map((metric) => (
            <div
              key={metric.key}
              className='ct-model-gateway-selection-insight-item'
            >
              <span>{metric.label}</span>
              <strong>{metric.value}</strong>
            </div>
          ))}
        </div>
        {insight.scoreEntries.length ? (
          <div className='ct-model-gateway-score-list'>
            {insight.scoreEntries.map(([key, value]) => (
              <Tag key={key} color='cyan' type='light' size='small'>
                {scoreMetricLabel(key, t)}: {formatScore(value)}
              </Tag>
            ))}
          </div>
        ) : null}
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
      </div>
    </section>
  );
}

function formatScoreSampleSource(source, t) {
  const normalized = String(source || '').trim();
  if (normalized === 'exact') return t('精确运行样本');
  if (normalized === 'similar') return t('同渠道历史样本');
  if (normalized === 'none') return t('暂无真实样本');
  return normalized || t('暂无真实样本');
}

function scoreEntryIsVisible(key, sampleCount) {
  const normalized = String(key || '').trim();
  if (
    Number(sampleCount || 0) <= 0 &&
    ['success', 'speed', 'success_score', 'speed_score'].includes(normalized)
  ) {
    return false;
  }
  return normalized !== 'explore_baseline';
}

function CandidateExplanationCard({
  candidate,
  candidates,
  index,
  record,
  onOpenScoreHistory,
  t,
}) {
  const sampleCount = Number(candidate?.sample_count || 0);
  const hasRealSamples = sampleCount > 0;
  const scoreEntries = Object.entries(candidate?.score_breakdown || {}).filter(
    ([key, score]) =>
      Number.isFinite(Number(score)) && scoreEntryIsVisible(key, sampleCount),
  );
  const routingEntries = Object.entries(
    candidate?.routing_score_breakdown || {},
  ).filter(
    ([key, score]) =>
      Number.isFinite(Number(score)) && scoreEntryIsVisible(key, sampleCount),
  );
  const scoreMetricEntries = [
    ['success_score', t('成功'), hasRealSamples ? candidate?.success_score : 0],
    [
      'speed_score',
      t('动态速度分'),
      hasRealSamples ? candidate?.speed_score : 0,
    ],
    [
      'score_speed_factor',
      t('速度因子'),
      hasRealSamples ? getCandidateScoreSpeedFactor(candidate) : 0,
    ],
    ['cost_score', t('成本'), candidate?.cost_score],
    ['group_score', t('分组'), candidate?.group_score],
    [
      'experience_score',
      t('体验'),
      hasRealSamples ? candidate?.experience_score : 0,
    ],
  ].filter(([, , value]) => Number(value) > 0);
  const routingScore = Number(candidate?.routing_score_total || 0);
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
  const ttftMs = Number(candidate?.ttft_ms || 0);
  const durationMs = Number(candidate?.duration_ms || 0);
  const emptyOutputRate = Number(candidate?.empty_output_rate || 0);
  const issueRate = Number(candidate?.experience_issue_rate || 0);
  const decisionText = buildCandidateDecisionText(
    candidate,
    candidates,
    record,
    t,
  );

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
          {!selected && !available && !unavailable && (
            <Tag color='grey' type='light' size='small'>
              #{index + 1}
            </Tag>
          )}
        </div>
      </div>

      {decisionText ? (
        <div className='ct-model-gateway-candidate-decision'>
          <Info size={13} />
          <span>{decisionText}</span>
        </div>
      ) : null}

      <div className='ct-model-gateway-candidate-meta'>
        <span>
          {t('分组')}: {candidate?.group || '--'}
        </span>
        <span>
          {t('上游模型')}: {candidate?.upstream_model || '--'}
        </span>
        <span>
          {t('提供商画像')}: {candidate?.provider_profile || '--'}
        </span>
        <span>
          {t('代理模式')}: {candidate?.proxy_mode || '--'}
        </span>
        <span>
          {t('运行键')}: {formatRuntimeKey(candidate?.runtime_key)}
        </span>
        {stickyKnown && (
          <span>
            {t('粘滞匹配')}: {stickyMatched ? t('已匹配') : t('未匹配')}
          </span>
        )}
      </div>

      <div className='ct-model-gateway-candidate-dynamic-grid'>
        <div>
          <span>{t('动态首包')}</span>
          <strong>{ttftMs > 0 ? formatLatency(ttftMs) : '--'}</strong>
        </div>
        <div>
          <span>{t('动态耗时')}</span>
          <strong>{durationMs > 0 ? formatLatency(durationMs) : '--'}</strong>
        </div>
        <div>
          <span>{t('样本数')}</span>
          <strong>
            {sampleCount > 0 ? formatNumber(sampleCount) : t('暂无真实样本')}
          </strong>
        </div>
        <div>
          <span>{t('生效并发')}</span>
          <strong>
            {activeConcurrency > 0 || displayConcurrencyLimit > 0
              ? `${formatNumber(activeConcurrency)} / ${
                  displayConcurrencyLimit > 0
                    ? formatNumber(displayConcurrencyLimit)
                    : '--'
                }`
              : '--'}
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
            {hasRealSamples ? t('总评分') : t('探索参考')}:{' '}
            {hasRealSamples
              ? formatScore(candidate?.score_total)
              : t('暂无真实样本')}
          </Tag>
        </Tooltip>
        {routingScore > 0 ? (
          <Tooltip
            content={t('调度分会叠加当前并发、排队和首包等待，只用于本次选路')}
          >
            <Tag color='grey' type='light' size='small' shape='circle'>
              {t('调度分')}: {formatScore(routingScore)}
            </Tag>
          </Tooltip>
        ) : null}
        <div className='ct-model-gateway-score-list'>
          {scoreMetricEntries.length ? (
            scoreMetricEntries.map(([key, label, value]) => (
              <Tag key={key} color='cyan' type='light' size='small'>
                {label}: {formatScore(value)}
              </Tag>
            ))
          ) : scoreEntries.length ? (
            scoreEntries.map(([key, value]) => (
              <Tag key={key} color='cyan' type='light' size='small'>
                {key}: {formatScore(value)}
              </Tag>
            ))
          ) : (
            <Typography.Text type='tertiary' size='small'>
              {t('评分拆解')}: {t('暂无真实样本')}
            </Typography.Text>
          )}
        </div>
      </div>
      {routingEntries.length ? (
        <div className='ct-model-gateway-candidate-routing-row'>
          <Typography.Text type='tertiary' size='small'>
            {t('调度因子')}
          </Typography.Text>
          {routingEntries
            .filter(([key]) => ['load', 'ttft_pending'].includes(key))
            .map(([key, value]) => (
              <Tag key={key} color='grey' type='light' size='small'>
                {scoreMetricLabel(key, t)}: {formatScore(value)}
              </Tag>
            ))}
        </div>
      ) : null}

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
      {(emptyOutputRate > 0 || issueRate > 0) && (
        <div className='ct-model-gateway-candidate-warning-line'>
          <Info size={13} />
          <span>
            {t('体验异常率')}:{' '}
            {formatPercent(Math.max(emptyOutputRate, issueRate))}
          </span>
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
  t,
}) {
  const requestMeta = record?.request_meta || {};
  const isHealthProbe =
    record?.is_health_probe === true || requestMeta?.is_health_probe === true;
  const probeReason = getProbeReason(record);
  const candidateExplanations = getCandidateExplanations(record);
  const scoreEntries = Object.entries(record?.score_breakdown || {});
  const metaEntries = Object.entries(requestMeta).filter(
    ([key, value]) =>
      key !== 'candidate_explanations' &&
      value !== '' &&
      value !== undefined &&
      value !== null,
  );
  const status = record ? getStatusMeta(record, t) : null;

  return (
    <SideSheet
      title={t('调度详情')}
      visible={visible}
      onCancel={onClose}
      placement='right'
      width='min(920px, 94vw)'
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
          <Descriptions
            align='plain'
            size='small'
            data={[
              {
                key: t('请求 ID'),
                value: <DetailValue>{record.request_id}</DetailValue>,
              },
              {
                key: t('记录类型'),
                value:
                  record.kind === 'user_request_detail'
                    ? t('用户请求')
                    : isDispatch(record)
                      ? t('调度')
                      : t('尝试'),
              },
              {
                key: t('状态'),
                value: (
                  <Tag color={status.color} shape='circle'>
                    {status.label}
                  </Tag>
                ),
              },
              {
                key: t('请求模型'),
                value: <DetailValue>{record.requested_model}</DetailValue>,
              },
              {
                key: t('端点类型'),
                value: <DetailValue>{record.endpoint_type}</DetailValue>,
              },
              {
                key: t('分组链路'),
                value: (
                  <DetailValue>
                    {`${record.requested_group || '--'} -> ${
                      record.selected_group || '--'
                    }${record.actual_group ? ` -> ${record.actual_group}` : ''}`}
                  </DetailValue>
                ),
              },
              {
                key: t('渠道链路'),
                value: (
                  <DetailValue>
                    {`#${record.channel_id || '--'} ${
                      record.channel_name || ''
                    }${
                      record.actual_channel_id
                        ? ` -> #${record.actual_channel_id} ${
                            record.actual_channel_name || ''
                          }`
                        : ''
                    }`}
                  </DetailValue>
                ),
              },
              {
                key: t('策略'),
                value: (
                  <div className='ct-model-gateway-record-tags'>
                    {record.policy_mode && <Tag>{record.policy_mode}</Tag>}
                    {record.auto_mode && <Tag>{record.auto_mode}</Tag>}
                    {record.strategy && <Tag>{record.strategy}</Tag>}
                    {record.shadow && <Tag color='purple'>{t('影子')}</Tag>}
                    {isHealthProbe && (
                      <Tag color='cyan' type='light'>
                        {t('健康探活')}
                      </Tag>
                    )}
                  </div>
                ),
              },
              ...(isHealthProbe
                ? [
                    {
                      key: t('探活原因'),
                      value: (
                        <DetailValue>
                          {formatProbeReason(probeReason, t)}
                        </DetailValue>
                      ),
                    },
                  ]
                : []),
              {
                key: t('耗时'),
                value: `${formatLatency(record.duration_ms)} / ${t(
                  '首包',
                )} ${formatLatency(record.ttft_ms)}`,
              },
              {
                key: t('评分'),
                value: formatScore(record.score_total),
              },
              {
                key: t('队列等待'),
                value: (
                  <div className='ct-model-gateway-record-tags'>
                    <QueueStickyTags record={record} t={t} showSticky={false} />
                  </div>
                ),
              },
              {
                key: t('调度流转'),
                value: <DispatchFlowTags record={record} t={t} />,
              },
              {
                key: t('粘滞路由'),
                value: (
                  <div className='ct-model-gateway-record-tags'>
                    <QueueStickyTags record={record} t={t} showQueue={false} />
                  </div>
                ),
              },
              {
                key: t('选择原因'),
                value: (
                  <DetailValue>
                    {formatSelectionReason(record.selected_reason, t)}
                  </DetailValue>
                ),
              },
            ]}
          />

          <SelectionInsightPanel
            record={record}
            candidates={candidateExplanations}
            t={t}
          />

          <section>
            <Typography.Title heading={6}>{t('候选分组')}</Typography.Title>
            <div className='ct-model-gateway-record-tags'>
              {(record.candidate_groups || []).length ? (
                record.candidate_groups.map((group) => (
                  <Tag key={group} color='blue' type='light'>
                    {group}
                  </Tag>
                ))
              ) : (
                <Typography.Text type='tertiary'>--</Typography.Text>
              )}
            </div>
          </section>

          <section>
            <Typography.Title heading={6}>{t('候选渠道解释')}</Typography.Title>
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
                    t={t}
                  />
                ))}
              </div>
            ) : (
              <Typography.Text type='tertiary'>--</Typography.Text>
            )}
          </section>

          <section>
            <Typography.Title heading={6}>{t('评分拆解')}</Typography.Title>
            <div className='ct-model-gateway-score-list'>
              {scoreEntries.length ? (
                scoreEntries.map(([key, value]) => (
                  <Tag key={key} color='cyan' type='light' shape='circle'>
                    {key}: {formatScore(value)}
                  </Tag>
                ))
              ) : (
                <Typography.Text type='tertiary'>--</Typography.Text>
              )}
            </div>
          </section>

          <section>
            <Typography.Title heading={6}>{t('调度元数据')}</Typography.Title>
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
          </section>

          {(record.error_code || record.error_type || record.status_code) && (
            <section>
              <Typography.Title heading={6}>{t('错误信息')}</Typography.Title>
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
                    value: formatAttemptErrorCategory(record.error_category, t),
                  },
                  {
                    key: t('错误信息'),
                    value: (
                      <DetailValue>{record.error_message || '--'}</DetailValue>
                    ),
                  },
                ]}
              />
            </section>
          )}
        </div>
      )}
    </SideSheet>
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
  const metricEntries = importantScoreDeltas(
    history?.metric_deltas,
    history?.score_delta,
    t,
  );
  const summaryText = buildScoreHistorySummary(history, current, previous, t);

  return (
    <Modal
      title={t('评分变更记录')}
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
            <Typography.Text strong>
              {history?.channel_name ||
                (history?.channel_id ? `#${history.channel_id}` : '--')}
            </Typography.Text>
            <Typography.Text type='secondary' size='small'>
              {formatRuntimeKey(history?.runtime_key)}
            </Typography.Text>
          </div>
          <div className='ct-model-gateway-record-tags'>
            <Tag color='cyan' type='light' shape='circle'>
              {t('记录数')} {formatNumber(history?.total_matched)}
            </Tag>
            <Tag
              color={scoreDeltaColor(history?.score_delta)}
              type='light'
              shape='circle'
            >
              {t('变化')} {formatScoreDelta(history?.score_delta)}
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
            <span>{t('当前情况')}</span>
          </div>
          <Typography.Text>{summaryText}</Typography.Text>
        </div>

        {current ? (
          <div className='ct-model-gateway-score-history-summary'>
            <div>
              <span>{t('当前分数')}</span>
              <strong>{formatScore(current.score_total)}</strong>
            </div>
            <div>
              <span>{t('上次分数')}</span>
              <strong>{formatScore(previous?.score_total)}</strong>
            </div>
            <div>
              <span>{t('评分变化')}</span>
              <strong
                className={
                  Number(history?.score_delta) >= 0
                    ? 'ct-model-gateway-score-delta-positive'
                    : 'ct-model-gateway-score-delta-negative'
                }
              >
                {formatScoreDelta(history?.score_delta)}
              </strong>
            </div>
            <div>
              <span>{t('样本数')}</span>
              <strong>
                {scoreHistorySampleLabel(current.sample_count, t)}
              </strong>
            </div>
          </div>
        ) : null}

        {metricEntries.length ? (
          <div className='ct-model-gateway-score-history-reasons'>
            <Typography.Text type='secondary' size='small'>
              {t('主要变更原因')}
            </Typography.Text>
            {metricEntries.map((entry) => (
              <div key={entry.key}>
                <span>{entry.label}</span>
                <strong
                  className={
                    Number(entry.value) >= 0
                      ? 'ct-model-gateway-score-delta-positive'
                      : 'ct-model-gateway-score-delta-negative'
                  }
                >
                  {formatScoreDelta(entry.value)}
                </strong>
                {entry.description ? <em>{entry.description}</em> : null}
              </div>
            ))}
          </div>
        ) : null}

        {loading ? (
          <Skeleton
            active
            loading
            placeholder={<Skeleton.Paragraph rows={7} />}
          />
        ) : items.length ? (
          <div className='ct-model-gateway-score-history-list'>
            {items.map((item) => (
              <ScoreHistoryRecordItem
                key={`${item.id}-${item.request_id}`}
                item={item}
                t={t}
              />
            ))}
          </div>
        ) : (
          <Empty description={t('未找到评分变更记录')} />
        )}
      </div>
    </Modal>
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
  const routingEntries = Object.entries(
    item?.routing_score_breakdown || {},
  ).filter(
    ([key, value]) =>
      ['load', 'ttft_pending'].includes(key) && Number.isFinite(Number(value)),
  );

  return (
    <div className='ct-model-gateway-score-history-item'>
      <div className='ct-model-gateway-score-history-main'>
        <div>
          <Typography.Text strong>
            {formatScore(item.score_total)}
          </Typography.Text>
          <Tag
            color={scoreDeltaColor(item.score_delta)}
            type='light'
            size='small'
          >
            {formatScoreDelta(item.score_delta)}
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
          {routingScore > 0 ? (
            <Tag color='grey' type='light' size='small'>
              {t('调度分')}: {formatScore(routingScore)}
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
          {t('首包')}: {formatLatency(item.ttft_ms)}
        </span>
        <span>
          {t('耗时')}: {formatLatency(item.duration_ms)}
        </span>
        <span>
          {t('样本数')}: {scoreHistorySampleLabel(item.sample_count, t)}
        </span>
      </div>

      {metricEntries.length ? (
        <div className='ct-model-gateway-score-history-metrics'>
          {metricEntries.slice(0, 8).map((entry) => (
            <div key={entry.key}>
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
            </div>
          ))}
        </div>
      ) : null}
      {routingEntries.length ? (
        <div className='ct-model-gateway-candidate-routing-row'>
          <Typography.Text type='tertiary' size='small'>
            {t('调度因子')}
          </Typography.Text>
          {routingEntries.map(([key, value]) => (
            <Tag key={key} color='grey' type='light' size='small'>
              {scoreMetricLabel(key, t)}: {formatScore(value)}
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
  const [scoreHistoryVisible, setScoreHistoryVisible] = useState(false);
  const [scoreHistoryLoading, setScoreHistoryLoading] = useState(false);
  const [scoreHistory, setScoreHistory] = useState(null);
  const [filters, setFilters] = useState(EMPTY_FILTERS);
  const [appliedFilters, setAppliedFilters] = useState(EMPTY_FILTERS);
  const [detailRecord, setDetailRecord] = useState(null);
  const [dispatchDetailLoading, setDispatchDetailLoading] = useState('');
  const [stickyRefreshToken, setStickyRefreshToken] = useState(0);
  const [viewMode, setViewMode] = useState(VIEW_MODES.USER_REQUESTS);
  const [filtersVisible, setFiltersVisible] = useState(false);
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
    recentLimit: RECENT_LIMIT,
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
        const payload = unwrapApiData(response);
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
    [hours, t],
  );

  const openUserRequestDispatchDetail = useCallback(
    async (record) => {
      const requestId = record?.request_id;
      if (!requestId) {
        Toast.warning(t('缺少请求 ID'));
        return;
      }
      if (record?.dispatch_record) {
        setDetailRecord(
          buildUserRequestDetailRecord(record, [record.dispatch_record]),
        );
        return;
      }
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
        const recentRecords = payload?.recent_records || [];
        const detail = buildUserRequestDetailRecord(record, recentRecords);
        if (!detail) {
          Toast.warning(t('暂无调度详情'));
          return;
        }
        setDetailRecord(detail);
      } catch (err) {
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
    (type) => [
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
        sorter: (a, b) => Number(a.avg_duration_ms) - Number(b.avg_duration_ms),
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
        title: t('平均评分'),
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
    ],
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
                  {formatProbeReason(getProbeReason(record), t)}
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
        title: t('评分'),
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
          dispatchDetailLoading={dispatchDetailLoading}
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
                scroll={{ x: 1380 }}
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
        t={t}
      />
    </div>
  );
}
