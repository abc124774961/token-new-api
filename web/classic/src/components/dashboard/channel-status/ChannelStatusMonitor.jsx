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
import {
  Avatar,
  Button,
  Empty,
  Modal,
  Skeleton,
  Table,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import {
  Activity,
  AlertTriangle,
  Eye,
  Gauge,
  HeartPulse,
  Info,
  List,
  Network,
  RadioTower,
  RefreshCw,
  ShieldCheck,
  Timer,
  Zap,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  API,
  getChannelIcon,
  isAdmin,
  showError,
  timestamp2string,
} from '../../../helpers';
import { CHANNEL_OPTIONS } from '../../../constants';
import DashboardCard from '../DashboardCard';

const REFRESH_INTERVAL_MS = 60 * 1000;
const REFRESH_INTERVAL_SECONDS = REFRESH_INTERVAL_MS / 1000;
const DEFAULT_MONITOR_WINDOW_DAYS = 7;
const MONITOR_WINDOW_OPTIONS = [7, 15, 30];
const EMPTY_IMAGE_SIZE = { width: 150, height: 150 };

function formatNumber(value) {
  return new Intl.NumberFormat().format(Number(value) || 0);
}

function formatRatio(value) {
  const ratio = Number(value);
  if (!Number.isFinite(ratio)) return '1x';
  const text = Number.isInteger(ratio)
    ? ratio.toFixed(0)
    : ratio.toFixed(2).replace(/0+$/, '').replace(/\.$/, '');
  return `${text}x`;
}

function formatPercent(value) {
  if (!Number.isFinite(Number(value))) return '0.00%';
  return `${Number(value).toFixed(2)}%`;
}

function formatRatePercent(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '0.00%';
  return formatPercent(numeric * 100);
}

function clampRate(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return 0;
  return Math.min(1, Math.max(0, numeric));
}

function getRuntimeOutputStabilityRate(runtime) {
  const emptyOutputRate = clampRate(runtime?.avg_empty_output_rate);
  const experienceIssueRate = clampRate(runtime?.avg_experience_issue_rate);
  return Math.max(0, 1 - Math.min(1, emptyOutputRate + experienceIssueRate));
}

function getRuntimeCompleteResponseRate(runtime) {
  return 1 - clampRate(runtime?.avg_empty_output_rate);
}

function getRuntimeNormalExperienceRate(runtime) {
  return 1 - clampRate(runtime?.avg_experience_issue_rate);
}

function formatSuccessRate(value, requests) {
  return Number(requests) > 0 ? formatPercent(value) : '--';
}

function getClientSuccessPercent(item) {
  const requests =
    (Number(item?.recent_requests) || 0) -
    (Number(item?.recent_client_aborted) || 0);
  const fallback = Number(item?.success_rate);
  if (Number.isFinite(fallback) && fallback > 0) {
    return Math.max(0, Math.min(fallback, 100));
  }
  if (requests <= 0) return null;
  return Math.max(
    0,
    Math.min(((Number(item?.recent_successes) || 0) / requests) * 100, 100),
  );
}

function formatClientSuccessRate(item) {
  const percent = getClientSuccessPercent(item);
  return percent === null ? '--' : formatPercent(percent);
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

function formatCostUnitPrice(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  const digits = numeric < 0.000001 ? 12 : numeric < 1 ? 6 : 4;
  return `$${numeric.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatReferenceCost(runtime, t) {
  const cost = Number(runtime?.avg_cost_ratio || 0);
  if (!Number.isFinite(cost) || cost <= 0) return '';
  const mode = String(runtime?.cost_pricing_mode || '').trim();
  if (mode === 'mixed') {
    return `${formatCostUnitPrice(cost)} ${t('混合')}`;
  }
  const suffix = mode === 'request' ? t('/次') : t('/M');
  return `${formatCostUnitPrice(cost)} ${suffix}`;
}

function formatTimestamp(timestamp) {
  return timestamp ? timestamp2string(timestamp) : '--';
}

function formatDuration(seconds, t) {
  const total = Number(seconds) || 0;
  if (total <= 0) return '';
  if (total >= 60) {
    return `${Math.ceil(total / 60)}${t('分钟')}`;
  }
  return `${total}${t('秒')}`;
}

function getHealthMeta(state, t) {
  switch (state) {
    case 'healthy':
      return { color: 'green', label: t('正常') };
    case 'warning':
      return { color: 'orange', label: t('警告') };
    default:
      return { color: 'red', label: t('严重') };
  }
}

function hasRuntime(group) {
  return Number(group?.runtime?.runtime_keys || 0) > 0;
}

function getGroupHealth(group) {
  if (!group || group.enabled_channels === 0) {
    return 'critical';
  }
  if (hasRuntime(group)) {
    const runtime = group.runtime;
    if (
      Number(runtime.available_runtime_keys || 0) <= 0 ||
      runtime.health_status === 'circuit_open' ||
      runtime.health_status === 'config_isolated'
    ) {
      return 'critical';
    }
    if (
      Number(runtime.risk_runtime_keys || 0) > 0 ||
      ['cooldown', 'failure_avoidance', 'high_pressure', 'degraded'].includes(
        runtime.health_status,
      )
    ) {
      return 'warning';
    }
    return 'healthy';
  }
  const recentRequests = Number(group.recent_requests) || 0;
  const successRate = Number(group.success_rate) || 0;
  if (recentRequests > 0 && successRate < 95) {
    return 'critical';
  }
  if (recentRequests > 0 && successRate < 99) {
    return 'warning';
  }
  if (
    recentRequests <= 0 &&
    Number(group.healthy_channels) <= 0 &&
    Number(group.enabled_channels) > 0
  ) {
    return 'warning';
  }
  if (
    Number(group.cooldown_channels) >= Number(group.total_channels) &&
    Number(group.total_channels) > 0
  ) {
    return 'warning';
  }
  return 'healthy';
}

function getOverallStatus(data, error) {
  if (error && !data) {
    return { state: 'offline', label: 'OFFLINE' };
  }
  if (data?.partial) {
    return { state: 'unknown', label: 'SYNCING' };
  }
  const summary = data?.summary;
  if (!summary || summary.total_channels <= 0) {
    return { state: 'unknown', label: 'NO DATA' };
  }
  if (summary.enabled_channels <= 0) {
    return { state: 'offline', label: 'OFFLINE' };
  }
  if (Number(summary.runtime?.runtime_keys || 0) > 0) {
    if (Number(summary.runtime.available_runtime_keys || 0) <= 0) {
      return { state: 'offline', label: 'OFFLINE' };
    }
    if (
      Number(summary.runtime.risk_runtime_keys || 0) > 0 ||
      Number(summary.runtime.queue_depth || 0) > 0
    ) {
      return { state: 'degraded', label: 'DEGRADED' };
    }
    return { state: 'operational', label: 'OPERATIONAL' };
  }
  if (summary.recent_requests > 0 && summary.success_rate < 99) {
    return { state: 'degraded', label: 'DEGRADED' };
  }
  if (
    summary.cooldown_channels >= summary.total_channels &&
    summary.total_channels > 0
  ) {
    return { state: 'degraded', label: 'DEGRADED' };
  }
  return { state: 'operational', label: 'OPERATIONAL' };
}

function isGroupAvailable(group) {
  if (!group || Number(group.enabled_channels) <= 0) {
    return false;
  }
  if (hasRuntime(group)) {
    return Number(group.runtime?.available_runtime_keys || 0) > 0;
  }
  if (
    Number(group.cooldown_channels) >= Number(group.total_channels) &&
    Number(group.total_channels) > 0
  ) {
    return false;
  }
  const recentRequests = Number(group.recent_requests) || 0;
  return recentRequests <= 0 || Number(group.success_rate) >= 99;
}

function getChannelModelPreview(models) {
  const list = String(models || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean);
  if (!list.length) return '--';
  if (list.length <= 2) return list.join(', ');
  return `${list.slice(0, 2).join(', ')} +${list.length - 2}`;
}

function getChannelModelCount(models) {
  return String(models || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean).length;
}

function getGroupModelPreview(group) {
  const models = new Set();
  getVisibleChannels(group).forEach((channel) => {
    String(channel.models || '')
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean)
      .forEach((model) => models.add(model));
  });
  const list = [...models];
  if (!list.length) return { label: '--', count: 0 };
  if (list.length <= 2) return { label: list.join(', '), count: list.length };
  return {
    label: `${list.slice(0, 2).join(', ')} +${list.length - 2}`,
    count: list.length,
  };
}

function getChannelTypeMeta(type) {
  return CHANNEL_OPTIONS.find((option) => option.value === type);
}

function getDominantChannelType(channels) {
  const counts = new Map();
  channels.forEach((channel) => {
    const current = counts.get(channel.type) || {
      type: channel.type,
      count: 0,
    };
    current.count += 1;
    counts.set(channel.type, current);
  });
  return [...counts.values()].sort((a, b) => b.count - a.count)[0]?.type;
}

function getChannelCooldown(channel) {
  return Number(channel?.failure_avoidance_remaining_seconds) || 0;
}

function getRuntimeStatusMeta(status, t) {
  switch (status) {
    case 'healthy':
      return { color: 'green', label: t('健康') };
    case 'circuit_open':
      return { color: 'red', label: t('熔断') };
    case 'config_isolated':
      return { color: 'red', label: t('配置隔离') };
    case 'cooldown':
      return { color: 'orange', label: t('冷却') };
    case 'failure_avoidance':
      return { color: 'orange', label: t('避险') };
    case 'high_pressure':
      return { color: 'orange', label: t('高压') };
    case 'queued':
      return { color: 'blue', label: t('排队') };
    case 'degraded':
      return { color: 'orange', label: t('降级') };
    default:
      return { color: 'grey', label: t('暂无运行态') };
  }
}

function getChannelPauseMeta(channel, t) {
  if (channel?.pause_type === 'balance_insufficient') {
    return {
      color: 'yellow',
      label: t('余额暂停'),
      reason: t('余额不足'),
      remaining: 0,
    };
  }
  if (channel?.pause_type === 'error_paused') {
    return {
      color: 'orange',
      label: t('暂停中'),
      reason: channel.pause_reason || t('错误暂停'),
      remaining: Number(channel.pause_remaining_seconds) || 0,
    };
  }
  return null;
}

function getVisibleChannels(group) {
  return group?.channels || [];
}

function getStatusLabel(status, t) {
  switch (status) {
    case 'success':
      return t('成功');
    case 'rate_limit':
      return '429';
    case 'server_error':
      return '5xx';
    case 'timeout':
      return t('超时');
    case 'empty_output':
      return t('响应为空');
    case 'experience_issue':
      return t('体验波动');
    case 'stream_interrupted':
      return t('流中断');
    case 'client_aborted':
      return t('客户端中断');
    default:
      return t('错误');
  }
}

function getRecentStatusSourceLabel(source, t) {
  switch (source) {
    case 'user_requests':
      return t('客户端请求');
    default:
      return '';
  }
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
      className={`ct-channel-monitor-summary-card ct-channel-monitor-summary-card-${tone}`}
      tone={dashboardToneMap[tone] || 'default'}
      bodyStyle={{ height: '100%' }}
    >
      <div className='flex items-start justify-between gap-3'>
        <div className='min-w-0'>
          <div className='text-xs font-semibold text-semi-color-text-2'>
            {label}
          </div>
          <div className='mt-2 font-mono text-2xl font-black tabular-nums text-semi-color-text-0'>
            {value}
          </div>
          <div className='mt-1 truncate text-xs text-semi-color-text-2'>
            {detail}
          </div>
        </div>
        <Avatar size='small' color={colorMap[tone] || 'blue'}>
          <Icon size={15} />
        </Avatar>
      </div>
    </DashboardCard>
  );
}

function SummarySkeleton() {
  return (
    <div className='grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4 mb-4'>
      {Array.from({ length: 4 }).map((_, index) => (
        <Skeleton
          key={index}
          loading
          active
          placeholder={
            <Skeleton.Paragraph
              rows={3}
              style={{ height: 96, borderRadius: 16 }}
            />
          }
        />
      ))}
    </div>
  );
}

function HealthTag({ state }) {
  const { t } = useTranslation();
  const meta = getHealthMeta(state, t);
  return (
    <Tag color={meta.color} shape='circle' size='small'>
      {meta.label}
    </Tag>
  );
}

function ChannelNameCell({ channel }) {
  const { t } = useTranslation();
  const cooldown = getChannelCooldown(channel);
  const pause = getChannelPauseMeta(channel, t);
  const runtimeMeta = getRuntimeStatusMeta(channel?.runtime?.health_status, t);

  return (
    <div className='min-w-[220px]'>
      <div className='flex flex-wrap items-center gap-2'>
        <span className='font-semibold text-semi-color-text-0'>
          {channel.name}
        </span>
        <Typography.Text type='secondary' size='small'>
          #{channel.id}
        </Typography.Text>
        {channel.runtime ? (
          <Tag color={runtimeMeta.color} shape='circle' size='small'>
            {runtimeMeta.label}
          </Tag>
        ) : (
          <HealthTag state={channel.health_state} />
        )}
        {!channel.enabled && !pause && (
          <Tag color='grey' shape='circle' size='small'>
            {t('禁用')}
          </Tag>
        )}
        {pause && (
          <Tooltip
            content={
              <div className='flex flex-col gap-1'>
                <span>
                  {t('原因：')}
                  {pause.reason}
                </span>
                {pause.remaining > 0 && (
                  <span>
                    {t('暂停剩余')}: {formatDuration(pause.remaining, t)}
                  </span>
                )}
              </div>
            }
          >
            <Tag color={pause.color} shape='circle' size='small'>
              {pause.label}
              {pause.remaining > 0
                ? ` ${formatDuration(pause.remaining, t)}`
                : ''}
            </Tag>
          </Tooltip>
        )}
        {cooldown > 0 && (
          <Tooltip
            content={
              <div className='flex flex-col gap-1'>
                <span>
                  {t('冷却剩余')}: {formatDuration(cooldown, t)}
                </span>
                {channel.failure_reason && (
                  <span>
                    {t('原因：')}
                    {channel.failure_reason}
                  </span>
                )}
              </div>
            }
          >
            <Tag color='orange' shape='circle' size='small'>
              {t('冷却中')} {formatDuration(cooldown, t)}
            </Tag>
          </Tooltip>
        )}
      </div>
      <div className='mt-1 truncate text-xs text-semi-color-text-2'>
        {getChannelModelPreview(channel.models)}
      </div>
    </div>
  );
}

function ChannelTypeBadge({ type }) {
  const meta = getChannelTypeMeta(type);
  return (
    <Tag
      color={meta?.color || 'grey'}
      shape='circle'
      type='light'
      prefixIcon={getChannelIcon(type)}
    >
      {meta?.label || type || '--'}
    </Tag>
  );
}

function StatusHistoryBar({ group }) {
  const { t } = useTranslation();
  const statuses =
    Array.isArray(group.recent_status) && group.recent_status.length > 0
      ? group.recent_status
      : [];
  const sourceLabel = getRecentStatusSourceLabel(
    group.recent_status_source,
    t,
  );

  return (
    <div className='ct-channel-monitor-history'>
      <div className='ct-channel-monitor-history-head'>
        <span>{t('近 60 次客户端请求记录')}</span>
        <span>
          {sourceLabel ? `${sourceLabel} · ` : ''}
          {REFRESH_INTERVAL_SECONDS} {t('秒')} {t('自动刷新')}
        </span>
      </div>
      {statuses.length > 0 ? (
        <>
          <div className='ct-channel-monitor-bars'>
            {statuses.slice(-60).map((status, index) => (
              <Tooltip
                key={`${status}-${index}`}
                content={getStatusLabel(status, t)}
              >
                <span
                  className={`ct-channel-monitor-bar ct-channel-monitor-bar-${status}`}
                />
              </Tooltip>
            ))}
          </div>
          <div className='ct-channel-monitor-history-foot'>
            <span>PAST</span>
            <span>NOW</span>
          </div>
        </>
      ) : (
        <div className='ct-channel-monitor-no-history'>
          {t('暂无近期客户端请求记录')}
        </div>
      )}
    </div>
  );
}

function GroupPanel({ group, windowDays }) {
  const { t } = useTranslation();
  const [detailsVisible, setDetailsVisible] = useState(false);
  const health = getGroupHealth(group);
  const visibleChannels = useMemo(() => getVisibleChannels(group), [group]);
  const modelPreview = useMemo(() => getGroupModelPreview(group), [group]);
  const dominantType = getDominantChannelType(visibleChannels);
  const dominantTypeMeta = getChannelTypeMeta(dominantType);
  const healthMeta = getHealthMeta(health, t);
  const canViewChannelDetails = isAdmin();
  const runtime = group.runtime || {};
  const hasSmartRuntime = hasRuntime(group);
  const recentRequests = Number(group.recent_requests) || 0;
  const clientAborted = Number(group.recent_client_aborted) || 0;
  const effectiveRequests = Math.max(recentRequests - clientAborted, 0);
  const channelHealthPercent =
    group.total_channels > 0
      ? Math.round((group.healthy_channels / group.total_channels) * 100)
      : 0;
  const clientSuccessPercent = getClientSuccessPercent(group);
  const availabilityPercent = clientSuccessPercent ?? 0;
  const runtimeAvailabilityText = formatClientSuccessRate(group);
  const runtimeReferenceCost = formatReferenceCost(runtime, t);

  const columns = useMemo(
    () => [
      {
        title: t('渠道'),
        dataIndex: 'name',
        width: 300,
        render: (_, record) => <ChannelNameCell channel={record} />,
      },
      {
        title: t('类型'),
        dataIndex: 'type',
        width: 150,
        render: (value) => <ChannelTypeBadge type={value} />,
      },
      {
        title: t('运行态'),
        dataIndex: 'runtime',
        width: 110,
        sorter: (a, b) =>
          Number(a.runtime?.runtime_keys || 0) -
          Number(b.runtime?.runtime_keys || 0),
        render: (_, record) => {
          const runtimeMeta = getRuntimeStatusMeta(
            record.runtime?.health_status,
            t,
          );
          return record.runtime ? (
            <div>
              <Tag color={runtimeMeta.color} type='light' size='small'>
                {runtimeMeta.label}
              </Tag>
              <div className='mt-1 text-xs text-semi-color-text-2'>
                {formatNumber(record.runtime.available_runtime_keys)} /{' '}
                {formatNumber(record.runtime.runtime_keys)}
              </div>
            </div>
          ) : (
            <Typography.Text
              strong
              type={record.health_state === 'critical' ? 'danger' : 'primary'}
            >
              {formatNumber(record.health_score)}
            </Typography.Text>
          );
        },
      },
      {
        title: t('调度分'),
        dataIndex: 'runtime',
        width: 130,
        sorter: (a, b) =>
          Number(a.runtime?.avg_routing_score || a.runtime?.avg_score || 0) -
          Number(b.runtime?.avg_routing_score || b.runtime?.avg_score || 0),
        render: (_, record) =>
          record.runtime ? (
            <div>
              <div className='font-mono font-semibold'>
                {formatScore(
                  record.runtime.avg_routing_score || record.runtime.avg_score,
                )}
              </div>
              <div className='text-xs text-semi-color-text-2'>
                {t('健康分')} {formatScore(record.runtime.avg_score)}
              </div>
            </div>
          ) : (
            formatPercent(record.success_rate)
          ),
      },
      {
        title: t('客户端请求'),
        dataIndex: 'runtime',
        width: 120,
        sorter: (a, b) =>
          Number(a.recent_requests || 0) - Number(b.recent_requests || 0),
        render: (_, record) => (
          <div>
            <div className='font-mono font-semibold'>
              {formatNumber(record.recent_requests)}
            </div>
            <div className='text-xs text-semi-color-text-2'>
              {formatNumber(record.recent_successes)} /{' '}
              {formatNumber(record.recent_failures)}
            </div>
          </div>
        ),
      },
      {
        title: t('首包 / 耗时'),
        dataIndex: 'runtime',
        width: 120,
        sorter: (a, b) =>
          (a.runtime?.avg_ttft_ms ||
            a.recent_avg_first_response_ms ||
            a.response_time ||
            0) -
          (b.runtime?.avg_ttft_ms ||
            b.recent_avg_first_response_ms ||
            b.response_time ||
            0),
        render: (_, record) => (
          <div>
            <div className='font-mono font-semibold'>
              {formatLatency(
                record.runtime?.avg_ttft_ms ||
                  record.recent_avg_first_response_ms ||
                  record.response_time,
              )}
            </div>
            <div className='text-xs text-semi-color-text-2'>
              {t('耗时')}{' '}
              {formatLatency(
                record.runtime?.avg_duration_ms || record.recent_avg_latency_ms,
              )}
            </div>
          </div>
        ),
      },
      {
        title: t('并发 / 队列'),
        dataIndex: 'active_concurrency',
        width: 110,
        render: (_, record) => (
          <div>
            <div className='font-mono font-semibold'>
              {record.runtime
                ? record.runtime.max_concurrency > 0
                  ? `${record.runtime.active_concurrency}/${record.runtime.max_concurrency}`
                  : formatNumber(record.runtime.active_concurrency)
                : record.max_concurrency > 0
                  ? `${record.active_concurrency}/${record.max_concurrency}`
                  : formatNumber(record.active_concurrency)}
            </div>
            {record.runtime ? (
              <div className='text-xs text-semi-color-text-2'>
                {t('队列')} {formatNumber(record.runtime.queue_depth)}
              </div>
            ) : record.concurrency_ceiling > 0 ? (
              <div className='text-xs text-semi-color-text-2'>
                {t('上限')} {record.concurrency_ceiling}
              </div>
            ) : null}
          </div>
        ),
      },
      {
        title: t('成本 / 体验'),
        dataIndex: 'runtime',
        width: 140,
        render: (_, record) =>
          record.runtime ? (
            <div>
              <div className='font-mono font-semibold'>
                {formatScore(record.runtime.avg_cost_score)} /{' '}
                {formatScore(record.runtime.avg_experience_score)}
              </div>
              <div className='text-xs text-semi-color-text-2'>
                {formatReferenceCost(record.runtime, t) || t('暂无参考成本')}
              </div>
            </div>
          ) : (
            <Typography.Text
              type={record.recent_failures > 0 ? 'danger' : 'secondary'}
              className='font-mono'
            >
              {formatNumber(record.recent_error_429)} /{' '}
              {formatNumber(record.recent_error_5xx)} /{' '}
              {formatNumber(record.recent_error_timeout)}
            </Typography.Text>
          ),
      },
      {
        title: t('响应稳定性'),
        dataIndex: 'runtime',
        width: 155,
        render: (_, record) =>
          record.runtime ? (
            <Tooltip
              content={`${t('完整响应')} ${formatRatePercent(getRuntimeCompleteResponseRate(record.runtime))} · ${t('体验正常')} ${formatRatePercent(getRuntimeNormalExperienceRate(record.runtime))}`}
            >
              <div>
                <div className='font-mono font-semibold'>
                  {formatRatePercent(getRuntimeOutputStabilityRate(record.runtime))}
                </div>
                <div className='text-xs text-semi-color-text-2'>
                  {t('完整响应 / 体验正常')}
                </div>
              </div>
            </Tooltip>
          ) : (
            formatNumber(getChannelModelCount(record.models))
          ),
      },
      {
        title: t('客户端请求时间'),
        dataIndex: 'runtime',
        width: 190,
        render: (_, record) => (
          <Typography.Text type='secondary' size='small'>
            <div>
              {formatTimestamp(record.last_request_at)}
            </div>
            <div className='text-xs text-semi-color-text-2'>
              {t('成功')} {formatTimestamp(record.last_success_at)}
            </div>
          </Typography.Text>
        ),
      },
    ],
    [t],
  );

  return (
    <>
      <DashboardCard
        className={`ct-channel-monitor-card ct-channel-monitor-card-${health}`}
        tone={health === 'healthy' ? 'uptime' : 'notice'}
        bodyStyle={{ padding: 0 }}
      >
        {canViewChannelDetails && (
          <div className='ct-channel-monitor-card-toolbar'>
            <div className='ct-channel-monitor-toolbar-count'>
              <strong>{formatNumber(group.total_channels)}</strong>
              <span>{t('渠道')}</span>
            </div>
            <Tooltip content={t('点击查看渠道明细')}>
              <Button
                type='tertiary'
                icon={<Eye size={15} />}
                onClick={() => setDetailsVisible(true)}
                className='ct-channel-monitor-detail-button'
              >
                {t('查看渠道')}
              </Button>
            </Tooltip>
          </div>
        )}
        <div className='ct-channel-monitor-inner'>
          <div className='ct-channel-monitor-top'>
            <div className='ct-channel-monitor-identity'>
              <div className='ct-channel-monitor-logo'>
                {getChannelIcon(dominantType) || <RadioTower size={30} />}
              </div>
              <div className='ct-channel-monitor-title-block'>
                <div className='ct-channel-monitor-title-row'>
                  <h3 title={group.group}>
                    {String(group.group).toUpperCase()}
                  </h3>
                </div>
                <div className='ct-channel-monitor-tags'>
                  <span className='ct-channel-monitor-pill-primary'>
                    {dominantTypeMeta?.label || t('渠道分组')}
                  </span>
                  <span className='ct-channel-monitor-ratio'>
                    {t('分组倍率')} {formatRatio(group.group_ratio)}
                  </span>
                  <span
                    className='ct-channel-monitor-model'
                    title={modelPreview.label}
                  >
                    {modelPreview.label}
                  </span>
                  {hasSmartRuntime && (
                    <span className='ct-channel-monitor-pill-muted'>
                      {t('运行态')} {formatNumber(runtime.runtime_keys)}
                    </span>
                  )}
                </div>
              </div>
            </div>
            <div className='ct-channel-monitor-actions'>
              <Tag
                color={healthMeta.color}
                shape='circle'
                className='ct-channel-monitor-health-tag'
              >
                {healthMeta.label}
              </Tag>
            </div>
          </div>

          <div className='ct-channel-monitor-metrics'>
            <div className='ct-channel-monitor-metric-box'>
              <div className='ct-channel-monitor-metric-label'>
                <Timer size={16} />
                <span>{hasSmartRuntime ? t('平均首包') : t('对话延迟')}</span>
              </div>
              <div className='ct-channel-monitor-metric-value'>
                {formatLatency(
                  hasSmartRuntime
                    ? group.avg_ttft_ms || runtime.avg_ttft_ms
                    : group.avg_latency_ms,
                )}
              </div>
            </div>
            <div className='ct-channel-monitor-metric-box'>
              <div className='ct-channel-monitor-metric-label'>
                <Network size={16} />
                <span>{hasSmartRuntime ? t('调度分') : t('端点 PING')}</span>
              </div>
              <div className='ct-channel-monitor-metric-value'>
                {hasSmartRuntime
                  ? formatScore(runtime.avg_routing_score || runtime.avg_score)
                  : formatLatency(
                      Math.round(
                        visibleChannels.reduce(
                          (sum, channel) =>
                            sum + (Number(channel.response_time) || 0),
                          0,
                        ) / Math.max(visibleChannels.length, 1),
                      ),
                    )}
              </div>
            </div>
          </div>

          <div className='ct-channel-monitor-availability'>
            <div className='ct-channel-monitor-availability-label'>
              {t('客户端成功率')} · {windowDays} {t('天')}
            </div>
            <div className='ct-channel-monitor-availability-value'>
              {runtimeAvailabilityText}
            </div>
            <div className='ct-channel-monitor-availability-sub'>
              {hasSmartRuntime
                ? `${t('有效请求')} ${formatNumber(effectiveRequests)} · ${t('最终成功')} ${formatNumber(group.recent_successes)} · ${t('自动恢复')} ${formatNumber(group.recent_recovered)}`
                : `+ ${formatNumber(modelPreview.count)} ${t('模型')}`}
            </div>
            <div className='ct-channel-monitor-availability-track'>
              <span style={{ width: `${availabilityPercent}%` }} />
            </div>
          </div>

          <div className='ct-channel-monitor-quick-grid'>
            <div>
              <span>{hasSmartRuntime ? t('客户端请求') : t('请求数')}</span>
              <strong>{formatNumber(group.recent_requests)}</strong>
            </div>
            <div>
              <span>
                {hasSmartRuntime
                  ? t('并发 / 队列')
                  : `429 / 5xx / ${t('超时')}`}
              </span>
              <strong>
                {hasSmartRuntime
                  ? `${formatNumber(runtime.active_concurrency)} / ${formatNumber(runtime.queue_depth)}`
                  : `${formatNumber(group.recent_error_429)} / ${formatNumber(group.recent_error_5xx)} / ${formatNumber(group.recent_error_timeout)}`}
              </strong>
            </div>
            <div>
              <span>
                {hasSmartRuntime
                  ? t('自动恢复 / 最终失败')
                  : `${t('错误冷却')} / ${t('忙碌')}`}
              </span>
              <strong>
                {hasSmartRuntime
                  ? `${formatNumber(group.recent_recovered)} / ${formatNumber(group.recent_failures)}`
                  : `${formatNumber(group.cooldown_channels)} / ${formatNumber(group.busy_channels)}`}
              </strong>
            </div>
            <div>
              <span>
                {hasSmartRuntime ? t('输出稳定性') : t('健康渠道占比')}
              </span>
              <strong>
                {hasSmartRuntime
                  ? formatRatePercent(getRuntimeOutputStabilityRate(runtime))
                  : `${channelHealthPercent}%`}
              </strong>
            </div>
          </div>

          {hasSmartRuntime && runtimeReferenceCost && (
            <div className='ct-channel-monitor-runtime-note'>
              <Tooltip
                content={t(
                  '参考成本来自渠道上游成本配置，用于换算成本分，不代表本次实际消费',
                )}
              >
                <span>
                  {t('参考成本')}: {runtimeReferenceCost}
                </span>
              </Tooltip>
            </div>
          )}

          <StatusHistoryBar group={group} />
        </div>
      </DashboardCard>
      <Modal
        title={
          <div className='ct-channel-monitor-modal-title'>
            <List size={17} />
            <span>
              {group.group} · {t('渠道明细')}
            </span>
          </div>
        }
        visible={detailsVisible}
        onCancel={() => setDetailsVisible(false)}
        width={1180}
        style={{ maxWidth: '96vw' }}
        footer={
          <Button
            type='primary'
            theme='solid'
            onClick={() => setDetailsVisible(false)}
          >
            {t('关闭')}
          </Button>
        }
      >
        <div className='ct-channel-monitor-modal-summary'>
          <Tag color={healthMeta.color} shape='circle'>
            {healthMeta.label}
          </Tag>
          <Tag shape='circle'>
            {t('渠道数')} {group.total_channels}
          </Tag>
          <Tag shape='circle'>
            {t('成功率')} {formatClientSuccessRate(group)}
          </Tag>
          <Tag
            color={group.recent_failures > 0 ? 'red' : 'green'}
            shape='circle'
          >
            {t('错误数')} {formatNumber(group.recent_failures)}
          </Tag>
        </div>
        <Table
          columns={columns}
          dataSource={visibleChannels}
          rowKey='id'
          pagination={false}
          size='small'
          scroll={{ x: 1330, y: 520 }}
          empty={
            <Empty
              title={t('该分组暂无渠道')}
              image={<IllustrationConstruction style={EMPTY_IMAGE_SIZE} />}
              darkModeImage={
                <IllustrationConstructionDark style={EMPTY_IMAGE_SIZE} />
              }
            />
          }
        />
      </Modal>
    </>
  );
}

function ChannelStatusContent({ data, windowDays }) {
  const { t } = useTranslation();
  const summary = data?.summary || {};
  const runtime = summary.runtime || {};
  const hasSmartRuntime = Number(runtime.runtime_keys || 0) > 0;
  const groups = data?.groups || [];
  const availableGroups = groups.filter(isGroupAvailable).length;
  const availableGroupPercent =
    groups.length > 0 ? (availableGroups / groups.length) * 100 : 0;
  const isPartial = Boolean(data?.partial);
  const unhealthyChannels = Math.max(
    Number(summary.total_channels) - Number(summary.healthy_channels),
    0,
  );

  return (
    <>
      <div className='ct-channel-monitor-summary-grid grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4 mb-4'>
        <SummaryMetric
          icon={HeartPulse}
          label={hasSmartRuntime ? t('智能运行态') : t('分组可用')}
          value={
            hasSmartRuntime
              ? `${formatNumber(runtime.available_runtime_keys)} / ${formatNumber(runtime.runtime_keys)}`
              : formatPercent(availableGroupPercent)
          }
          detail={
            hasSmartRuntime
              ? `${formatNumber(runtime.channels)} ${t('渠道')} · ${formatNumber(runtime.risk_runtime_keys)} ${t('风险运行态')}`
              : `${formatNumber(availableGroups)} / ${formatNumber(groups.length)} ${t('可用分组')} · ${formatNumber(unhealthyChannels)} ${t('需关注渠道')}`
          }
          tone={
            hasSmartRuntime
              ? runtime.risk_runtime_keys > 0
                ? 'warning'
                : 'success'
              : availableGroups === groups.length
                ? 'success'
                : 'warning'
          }
        />
        <SummaryMetric
          icon={Activity}
          label={t('客户端请求成功率')}
          value={formatClientSuccessRate(summary)}
          detail={
            isPartial
              ? t('统计刷新中，当前显示基础渠道状态')
              : `${formatNumber(summary.recent_requests)} / ${windowDays}${t('天')} ${t('客户端请求')}`
          }
          tone={
            (getClientSuccessPercent(summary) ?? 100) >= 99
              ? 'success'
              : 'warning'
          }
        />
        <SummaryMetric
          icon={AlertTriangle}
          label={hasSmartRuntime ? t('风险运行态') : t('最终失败')}
          value={
            hasSmartRuntime
              ? formatNumber(runtime.risk_runtime_keys)
              : formatNumber(summary.recent_failures)
          }
          detail={
            hasSmartRuntime
              ? `${t('熔断')} ${formatNumber(runtime.circuit_open_runtime_keys)} / ${t('冷却')} ${formatNumber(runtime.cooldown_runtime_keys)}`
              : `429 ${formatNumber(summary.recent_error_429)} / 5xx ${formatNumber(summary.recent_error_5xx)}`
          }
          tone={
            hasSmartRuntime
              ? runtime.risk_runtime_keys > 0
                ? 'danger'
                : 'success'
              : (getClientSuccessPercent(summary) ?? 100) < 99
                ? 'danger'
                : 'success'
          }
        />
        <SummaryMetric
          icon={Gauge}
          label={hasSmartRuntime ? t('活跃队列') : t('活跃负载')}
          value={
            hasSmartRuntime
              ? `${formatNumber(runtime.active_concurrency)} / ${formatNumber(runtime.queue_depth)}`
              : formatNumber(summary.busy_channels)
          }
          detail={
            hasSmartRuntime
              ? `${t('并发')} ${formatNumber(runtime.active_concurrency)} / ${t('排队')} ${formatNumber(runtime.queue_depth)}`
              : `${formatLatency(summary.avg_latency_ms)} ${t('平均延迟')} / ${formatNumber(summary.cooldown_channels)} ${t('错误冷却')}`
          }
          tone={
            hasSmartRuntime
              ? runtime.queue_depth > 0
                ? 'warning'
                : 'default'
              : summary.cooldown_channels > 0
                ? 'warning'
                : 'default'
          }
        />
      </div>

      {groups.length > 0 ? (
        <div className='ct-channel-monitor-grid'>
          {groups.map((group) => (
            <GroupPanel
              key={group.group}
              group={group}
              windowDays={windowDays}
            />
          ))}
        </div>
      ) : (
        <DashboardCard bodyStyle={{ minHeight: 260 }}>
          <div className='ct-dashboard-empty'>
            <Empty
              title={t('暂无渠道状态数据')}
              image={<IllustrationConstruction style={EMPTY_IMAGE_SIZE} />}
              darkModeImage={
                <IllustrationConstructionDark style={EMPTY_IMAGE_SIZE} />
              }
            />
          </div>
        </DashboardCard>
      )}
    </>
  );
}

const ChannelStatusMonitor = () => {
  const { t } = useTranslation();
  const [windowDays, setWindowDays] = useState(DEFAULT_MONITOR_WINDOW_DAYS);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState(null);
  const [lastUpdated, setLastUpdated] = useState(0);
  const [refreshCountdown, setRefreshCountdown] = useState(
    REFRESH_INTERVAL_SECONDS,
  );
  const inFlightWindowsRef = useRef(new Set());
  const latestWindowDaysRef = useRef(windowDays);

  useEffect(() => {
    latestWindowDaysRef.current = windowDays;
  }, [windowDays]);

  const loadData = useCallback(
    async ({ silent = false } = {}) => {
      const requestWindowDays = windowDays;
      if (inFlightWindowsRef.current.has(requestWindowDays)) {
        return;
      }
      inFlightWindowsRef.current.add(requestWindowDays);
      if (silent) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      try {
        const res = await API.get('/api/channel/status_monitor', {
          params: { hours: windowDays * 24 },
          disableDuplicate: true,
          skipErrorHandler: true,
        });
        const { success, message, data } = res.data;
        if (success) {
          if (latestWindowDaysRef.current === requestWindowDays) {
            setData(data);
            setError(null);
            setLastUpdated(Math.floor(Date.now() / 1000));
            setRefreshCountdown(REFRESH_INTERVAL_SECONDS);
          }
        } else {
          throw new Error(message || t('渠道状态加载失败'));
        }
      } catch (err) {
        if (latestWindowDaysRef.current === requestWindowDays) {
          setError(err);
        }
        if (!silent && latestWindowDaysRef.current === requestWindowDays) {
          showError(err.message || t('渠道状态加载失败'));
        }
      } finally {
        inFlightWindowsRef.current.delete(requestWindowDays);
        if (latestWindowDaysRef.current === requestWindowDays) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    },
    [t, windowDays],
  );

  useEffect(() => {
    loadData();
    const timer = window.setInterval(() => {
      loadData({ silent: true });
    }, REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [loadData]);

  useEffect(() => {
    if (!data?.partial || loading || refreshing) {
      return undefined;
    }
    const timer = window.setTimeout(() => {
      loadData({ silent: true });
    }, 2500);
    return () => window.clearTimeout(timer);
  }, [data?.partial, loadData, loading, refreshing]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setRefreshCountdown((value) =>
        value <= 1 ? REFRESH_INTERVAL_SECONDS : value - 1,
      );
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  const statusTip = useMemo(
    () => t('根据真实请求最终结果、限流错误与错误冷却计算健康状态'),
    [t],
  );
  const overallStatus = useMemo(
    () => getOverallStatus(data, error),
    [data, error],
  );

  return (
    <div className='ct-dashboard-shell h-full'>
      <div className='ct-dashboard-hero'>
        <div className='ct-dashboard-hero-copy'>
          <div className='ct-dashboard-eyebrow'>{t('当前路由状态')}</div>
          <h2 className='ct-dashboard-greeting' style={{ opacity: 1 }}>
            {t('渠道状态监控')}
          </h2>
          <div className='ct-channel-monitor-hero-meta mt-2 flex flex-wrap items-center gap-2 text-xs text-semi-color-text-2'>
            <Tooltip content={statusTip}>
              <span className='inline-flex items-center gap-1'>
                <Info size={13} />
                {t('按分组监控渠道访问状态')}
              </span>
            </Tooltip>
            <Tag shape='circle'>
              <Timer size={12} className='mr-1' />
              {windowDays} {t('天')} {t('窗口')}
            </Tag>
            {lastUpdated > 0 && (
              <Tag shape='circle'>
                {t('最后更新')} {formatTimestamp(lastUpdated)}
              </Tag>
            )}
            {data?.partial && (
              <Tag color='orange' shape='circle'>
                {t('统计刷新中')}
              </Tag>
            )}
          </div>
        </div>
        <div className='ct-channel-monitor-toolbar'>
          <div className='ct-channel-monitor-window-tabs'>
            {MONITOR_WINDOW_OPTIONS.map((days) => (
              <Button
                key={days}
                type={windowDays === days ? 'primary' : 'tertiary'}
                theme={windowDays === days ? 'solid' : 'borderless'}
                onClick={() => setWindowDays(days)}
                className='ct-channel-monitor-window-button'
              >
                {days} {t('天')}
              </Button>
            ))}
          </div>
          <Tag
            shape='circle'
            className={`ct-channel-monitor-overall ct-channel-monitor-overall-${overallStatus.state}`}
          >
            <span className='ct-channel-monitor-overall-dot' />
            {overallStatus.label}
          </Tag>
          <Button
            type='tertiary'
            icon={<RefreshCw size={16} />}
            onClick={() => {
              setRefreshCountdown(REFRESH_INTERVAL_SECONDS);
              loadData({ silent: true });
            }}
            loading={loading || refreshing}
            className='ct-channel-monitor-toolbar-button'
          />
          <Button
            type='tertiary'
            icon={<Timer size={14} />}
            className='ct-channel-monitor-auto-refresh'
            disabled
          >
            {t('自动刷新')}: {refreshCountdown}s
          </Button>
        </div>
      </div>

      {loading ? (
        <>
          <SummarySkeleton />
          <Skeleton
            loading
            active
            placeholder={<Skeleton.Paragraph rows={8} />}
          />
        </>
      ) : error && !data ? (
        <DashboardCard bodyStyle={{ minHeight: 320 }}>
          <div className='ct-dashboard-empty'>
            <Empty
              title={t('渠道状态加载失败')}
              image={<Zap size={40} />}
              description={error.message}
            />
          </div>
        </DashboardCard>
      ) : data ? (
        <ChannelStatusContent data={data} windowDays={windowDays} />
      ) : (
        <DashboardCard bodyStyle={{ minHeight: 320 }}>
          <div className='ct-dashboard-empty'>
            <Empty
              title={t('暂无渠道状态数据')}
              image={<ShieldCheck size={40} />}
            />
          </div>
        </DashboardCard>
      )}
    </div>
  );
};

export default ChannelStatusMonitor;
