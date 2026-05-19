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
  Toast,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationConstruction,
  IllustrationConstructionDark,
} from '@douyinfe/semi-illustrations';
import {
  Activity,
  Bot,
  CheckCircle2,
  Clock3,
  Download,
  Gauge,
  GitBranch,
  Info,
  Layers3,
  RadioTower,
  RefreshCw,
  RotateCcw,
  ServerCog,
  Timer,
  Zap,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers/api';
import { showError, timestamp2string } from '../../../helpers';
import DashboardCard from '../DashboardCard';
import './model-gateway.css';

const DEFAULT_HOURS = 24;
const RECENT_LIMIT = 50;
const TOP_N = 10;
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

function formatTimestamp(timestamp) {
  return timestamp ? timestamp2string(timestamp) : '--';
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

function getStatusMeta(record, t) {
  if (record?.success) {
    return { color: 'green', label: t('成功') };
  }
  if (record?.stream_interrupted) {
    return { color: 'orange', label: t('流中断') };
  }
  return { color: 'red', label: t('失败') };
}

function isDispatch(record) {
  return record?.kind === 'dispatch';
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

  return (
    <div className='ct-model-gateway-aggregate-name'>
      <Avatar size='extra-small' color={type === 'channel' ? 'cyan' : 'blue'}>
        {icon}
      </Avatar>
      <div className='min-w-0'>
        <div className='ct-model-gateway-aggregate-title' title={label}>
          {label}
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
    case 'saturated':
      return { color: 'red', label: t('并发饱和') };
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

function getRuntimeRiskWeight(item) {
  if (!item) return 0;
  if (item.health_status === 'circuit_open' || item.circuit_open) return 100;
  if (item.health_status === 'saturated') return 90;
  if (item.cooldown || item.health_status === 'cooldown') return 80;
  if (item.failure_avoidance || item.health_status === 'failure_avoidance')
    return 70;
  if (item.circuit_state === 'half_open') return 65;
  if (Number(item.queue_depth) > 0 || item.health_status === 'queued')
    return 55;
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
          {formatPercent(record?.success_rate)}
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
    return <div className='ct-model-gateway-trend-empty'>{t('暂无调度趋势')}</div>;
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
            return (
              <Tag
                key={item.key || 'unknown'}
                color='cyan'
                size='small'
                type='light'
              >
                {item.key || t('未知')} · {formatPercent(item.success_rate)} ·{' '}
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
    </div>
  );
}

function DispatchTrendPanel({ trends, t, onExport }) {
  const rows = useMemo(
    () =>
      (Array.isArray(trends) ? trends : []).map((record, index) => ({
        ...record,
        _trend_key: `${record?.bucket_start || 'start'}-${
          record?.bucket_end || 'end'
        }-${index}`,
      })),
    [trends],
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
        expandedRowRender={(record) => <TrendExpandedRow record={record} t={t} />}
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
    Number(summary.saturated_channels || 0);

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
          label={t('并发饱和')}
          value={formatNumber(summary.saturated_channels)}
          detail={`${formatNumber(summary.active_concurrency)} ${t('活跃并发')}`}
          tone={summary.saturated_channels > 0 ? 'danger' : 'success'}
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

function RuntimeStatusPanel({ runtimeStatus, t }) {
  const summary = runtimeStatus?.summary || {};
  const items = runtimeStatus?.items || [];
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
        width: 200,
        render: (_, record) => (
          <div className='ct-model-gateway-runtime-stack'>
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
            <Typography.Text type='secondary' size='small'>
              {record.circuit_open_until
                ? formatTimestamp(record.circuit_open_until)
                : `${t('样本')} ${formatNumber(record.circuit_sample_count)}`}
            </Typography.Text>
          </div>
        ),
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
              {formatPercent(record.success_rate)}
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
        scroll={{ x: 1480 }}
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

function CandidateExplanationCard({ candidate, index, t }) {
  const scoreEntries = Object.entries(candidate?.score_breakdown || {}).filter(
    ([, score]) => Number.isFinite(Number(score)),
  );
  const available = candidate?.available === true;
  const unavailable = candidate?.available === false;
  const selected = candidate?.selected === true;
  const stickyMatched = candidate?.sticky_matched === true;
  const stickyKnown = typeof candidate?.sticky_matched === 'boolean';
  const channelLabel =
    candidate?.channel_name ||
    (candidate?.channel_id ? `#${candidate.channel_id}` : t('未知'));

  return (
    <div
      className={`ct-model-gateway-candidate-card${
        selected ? ' ct-model-gateway-candidate-card-selected' : ''
      }`}
    >
      <div className='ct-model-gateway-candidate-head'>
        <div className='ct-model-gateway-candidate-title'>
          <span title={channelLabel}>{channelLabel}</span>
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
          {available && (
            <Tag color='green' type='light' size='small'>
              {t('可用')}
            </Tag>
          )}
          {unavailable && (
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

      <div className='ct-model-gateway-candidate-score-row'>
        <Tag color='cyan' type='light' size='small' shape='circle'>
          {t('总评分')}: {formatScore(candidate?.score_total)}
        </Tag>
        <div className='ct-model-gateway-score-list'>
          {scoreEntries.length ? (
            scoreEntries.map(([key, value]) => (
              <Tag key={key} color='cyan' type='light' size='small'>
                {key}: {formatScore(value)}
              </Tag>
            ))
          ) : (
            <Typography.Text type='tertiary' size='small'>
              {t('评分拆解')}: --
            </Typography.Text>
          )}
        </div>
      </div>

      {!available && (
        <Typography.Text
          type={candidate?.reject_reason ? 'danger' : 'tertiary'}
          size='small'
          ellipsis={{ showTooltip: true }}
        >
          {t('过滤原因')}: {candidate?.reject_reason || t('无过滤原因')}
        </Typography.Text>
      )}
    </div>
  );
}

function RecordDetailDrawer({ record, visible, onClose, onExportReplay, t }) {
  const requestMeta = record?.request_meta || {};
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
      width={520}
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
                key: 'request_id',
                value: t('请求 ID'),
                content: <DetailValue>{record.request_id}</DetailValue>,
              },
              {
                key: 'kind',
                value: t('记录类型'),
                content: isDispatch(record) ? t('调度') : t('尝试'),
              },
              {
                key: 'status',
                value: t('状态'),
                content: (
                  <Tag color={status.color} shape='circle'>
                    {status.label}
                  </Tag>
                ),
              },
              {
                key: 'model',
                value: t('请求模型'),
                content: <DetailValue>{record.requested_model}</DetailValue>,
              },
              {
                key: 'endpoint',
                value: t('端点类型'),
                content: <DetailValue>{record.endpoint_type}</DetailValue>,
              },
              {
                key: 'groups',
                value: t('分组链路'),
                content: (
                  <DetailValue>
                    {`${record.requested_group || '--'} -> ${
                      record.selected_group || '--'
                    }${record.actual_group ? ` -> ${record.actual_group}` : ''}`}
                  </DetailValue>
                ),
              },
              {
                key: 'channel',
                value: t('渠道链路'),
                content: (
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
                key: 'policy',
                value: t('策略'),
                content: (
                  <div className='ct-model-gateway-record-tags'>
                    {record.policy_mode && <Tag>{record.policy_mode}</Tag>}
                    {record.auto_mode && <Tag>{record.auto_mode}</Tag>}
                    {record.strategy && <Tag>{record.strategy}</Tag>}
                    {record.shadow && <Tag color='purple'>{t('影子')}</Tag>}
                  </div>
                ),
              },
              {
                key: 'duration',
                value: t('耗时'),
                content: `${formatLatency(record.duration_ms)} / ${t(
                  '首包',
                )} ${formatLatency(record.ttft_ms)}`,
              },
              {
                key: 'score',
                value: t('评分'),
                content: formatScore(record.score_total),
              },
              {
                key: 'queue',
                value: t('队列等待'),
                content: (
                  <div className='ct-model-gateway-record-tags'>
                    <QueueStickyTags record={record} t={t} showSticky={false} />
                  </div>
                ),
              },
              {
                key: 'sticky',
                value: t('粘滞路由'),
                content: (
                  <div className='ct-model-gateway-record-tags'>
                    <QueueStickyTags record={record} t={t} showQueue={false} />
                  </div>
                ),
              },
              {
                key: 'reason',
                value: t('选择原因'),
                content: <DetailValue>{record.selected_reason}</DetailValue>,
              },
            ]}
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
                    index={index}
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
                  value: key,
                  content: String(value),
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
                    key: 'status_code',
                    value: 'HTTP',
                    content: record.status_code || '--',
                  },
                  {
                    key: 'error_code',
                    value: t('错误码'),
                    content: record.error_code || '--',
                  },
                  {
                    key: 'error_type',
                    value: t('错误类型'),
                    content: record.error_type || '--',
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

export default function ModelGatewayMonitor() {
  const { t } = useTranslation();
  const [hours, setHours] = useState(DEFAULT_HOURS);
  const [trendBucket, setTrendBucket] = useState(DEFAULT_TREND_BUCKET);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [replayVisible, setReplayVisible] = useState(false);
  const [replayLoading, setReplayLoading] = useState(false);
  const [replayRequestId, setReplayRequestId] = useState('');
  const [replayArtifact, setReplayArtifact] = useState(null);
  const [filters, setFilters] = useState(EMPTY_FILTERS);
  const [appliedFilters, setAppliedFilters] = useState(EMPTY_FILTERS);
  const [detailRecord, setDetailRecord] = useState(null);

  const loadSummary = useCallback(
    async (silent = false) => {
      if (silent) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      setError('');
      try {
        const response = await API.get(
          '/api/model_gateway/observability/summary',
          {
            params: {
              hours,
              recent_limit: RECENT_LIMIT,
              top_n: TOP_N,
              trend_bucket_seconds:
                trendBucket === DEFAULT_TREND_BUCKET ? undefined : trendBucket,
              model: appliedFilters.model || undefined,
              group: appliedFilters.group || undefined,
              channel_id: appliedFilters.channel_id || undefined,
              request_id: appliedFilters.request_id || undefined,
            },
            disableDuplicate: true,
            skipErrorHandler: true,
          },
        );
        setData(unwrapApiData(response));
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('加载观测数据失败');
        setError(message);
        showError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [appliedFilters, hours, t, trendBucket],
  );

  useEffect(() => {
    loadSummary(false);
  }, [loadSummary]);

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
  const runtimeStatus = data?.runtime_status || {};
  const hasData = Number(summary.total_records) > 0;
  const lastUpdated = summary.end_time
    ? formatTimestamp(summary.end_time)
    : '--';
  const smartRecords = useMemo(
    () =>
      (data?.recent_records || []).filter((record) => record.smart_handled)
        .length,
    [data],
  );
  const hasActiveFilters = Object.values(appliedFilters).some(Boolean);

  const updateFilter = useCallback((key, value) => {
    setFilters((current) => ({ ...current, [key]: value }));
  }, []);

  const applyFilters = useCallback(() => {
    setAppliedFilters({
      model: filters.model.trim(),
      group: filters.group.trim(),
      channel_id: filters.channel_id.trim(),
      request_id: filters.request_id.trim(),
    });
  }, [filters]);

  const resetFilters = useCallback(() => {
    setFilters(EMPTY_FILTERS);
    setAppliedFilters(EMPTY_FILTERS);
  }, []);

  const aggregateColumns = useCallback(
    (type) => [
      {
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
        title: t('调度'),
        dataIndex: 'dispatches',
        width: 100,
        sorter: (a, b) => Number(a.dispatches) - Number(b.dispatches),
        render: (value) => (
          <Typography.Text strong>{formatNumber(value)}</Typography.Text>
        ),
      },
      {
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
              {formatPercent(value)}
            </Tag>
          );
        },
      },
      {
        title: t('平均耗时'),
        dataIndex: 'avg_duration_ms',
        width: 120,
        sorter: (a, b) => Number(a.avg_duration_ms) - Number(b.avg_duration_ms),
        render: (value) => formatLatency(value),
      },
      {
        title: t('首包延迟'),
        dataIndex: 'avg_ttft_ms',
        width: 120,
        sorter: (a, b) => Number(a.avg_ttft_ms) - Number(b.avg_ttft_ms),
        render: (value) => formatLatency(value),
      },
      {
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
        title: t('队列 / 粘滞'),
        dataIndex: 'avg_queue_wait_ms',
        width: 260,
        render: (_, record) => (
          <QueueStickyTags record={record} t={t} compact />
        ),
      },
      {
        title: t('平均评分'),
        dataIndex: 'avg_score_total',
        width: 110,
        render: (value) => formatScore(value),
      },
      {
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
            </div>
          </div>
        ),
      },
      {
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
        title: t('状态'),
        dataIndex: 'success',
        width: 150,
        render: (_, record) => {
          const meta = getStatusMeta(record, t);
          return (
            <div>
              <Tag color={meta.color} shape='circle'>
                {meta.label}
              </Tag>
              {record.status_code > 0 && (
                <div className='text-xs text-semi-color-text-2'>
                  HTTP {record.status_code}
                </div>
              )}
            </div>
          );
        },
      },
      {
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
        title: t('评分'),
        dataIndex: 'score_total',
        width: 120,
        render: (value) => formatScore(value),
      },
      {
        title: t('队列 / 粘滞'),
        dataIndex: 'queue_wait_ms',
        width: 250,
        render: (_, record) => (
          <QueueStickyTags record={record} t={t} compact />
        ),
      },
      {
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
                icon={<Info size={14} />}
                onClick={() => setDetailRecord(record)}
              />
            </Tooltip>
            <Tooltip content={t('导出 Replay JSON')}>
              <Button
                size='small'
                type='tertiary'
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
        <div className='ct-model-gateway-actions'>
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
            onClick={() => loadSummary(true)}
          >
            {t('刷新')}
          </Button>
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

      {summary.truncated && (
        <Banner
          type='warning'
          className='ct-model-gateway-banner'
          description={t('观测记录较多，当前仅展示扫描范围内的聚合结果')}
          closeIcon={null}
        />
      )}

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
        <Input
          value={filters.channel_id}
          onChange={(value) => updateFilter('channel_id', value)}
          placeholder={t('按渠道 ID 筛选')}
          prefix={t('渠道')}
        />
        <Input
          value={filters.request_id}
          onChange={(value) => updateFilter('request_id', value)}
          placeholder={t('按请求 ID 筛选')}
          prefix={t('请求 ID')}
        />
        <div className='ct-model-gateway-filter-actions'>
          <Button type='primary' onClick={applyFilters}>
            {t('应用筛选')}
          </Button>
          <Button onClick={resetFilters} disabled={!hasActiveFilters}>
            {t('重置筛选')}
          </Button>
        </div>
      </DashboardCard>

      {loading ? (
        <MetricSkeleton />
      ) : (
        <div className='ct-model-gateway-metric-grid'>
          <SummaryMetric
            icon={Zap}
            label={t('智能处理')}
            value={formatNumber(smartRecords)}
            detail={`${formatNumber(summary.total_records)} ${t('条记录')}`}
            tone='default'
          />
          <SummaryMetric
            icon={CheckCircle2}
            label={t('调度成功率')}
            value={formatPercent(summary.success_rate)}
            detail={`${formatNumber(summary.successes)} / ${formatNumber(
              summary.attempts,
            )} ${t('尝试')}`}
            tone={getSuccessTone(summary.success_rate, summary.attempts)}
          />
          <SummaryMetric
            icon={Timer}
            label={t('平均耗时')}
            value={formatLatency(summary.avg_duration_ms)}
            detail={`${formatNumber(summary.dispatches)} ${t('次调度')}`}
            tone='success'
          />
          <SummaryMetric
            icon={Clock3}
            label={t('首包延迟')}
            value={formatLatency(summary.avg_ttft_ms)}
            detail={t('流式首包平均值')}
            tone='default'
          />
          <SummaryMetric
            icon={GitBranch}
            label={t('队列等待')}
            value={formatLatency(summary.avg_queue_wait_ms)}
            detail={`${formatNumber(
              summary.queued_dispatches,
            )} / ${formatNumber(summary.queue_enabled_dispatches)} ${t(
              '启用队列调度',
            )}`}
            tone={summary.avg_queue_wait_ms > 0 ? 'warning' : 'success'}
          />
          <SummaryMetric
            icon={Activity}
            label={t('流中断')}
            value={formatNumber(summary.stream_interrupted)}
            detail={`${formatNumber(summary.failures)} ${t('失败')}`}
            tone={summary.stream_interrupted > 0 ? 'danger' : 'success'}
          />
        </div>
      )}

      {!loading && (
        <div className='ct-model-gateway-insight-grid'>
          <StickyInsightPanel summary={summary} t={t} />
          <RuntimeRiskPanel runtimeStatus={runtimeStatus} t={t} />
        </div>
      )}

      {!loading && (
        <DispatchTrendPanel
          trends={data?.trends}
          t={t}
          onExport={exportTrends}
        />
      )}

      {!loading && <RuntimeStatusPanel runtimeStatus={runtimeStatus} t={t} />}

      {!loading && !hasData ? (
        <DashboardCard bodyStyle={{ minHeight: 280 }}>
          <EmptyState t={t} />
        </DashboardCard>
      ) : (
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
              <span className='ct-model-gateway-panel-title'>
                <Gauge size={17} />
                {t('最近调度记录')}
              </span>
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
      )}

      <ReplayModal
        artifact={replayArtifact}
        loading={replayLoading}
        visible={replayVisible}
        onCancel={() => setReplayVisible(false)}
        requestId={replayRequestId}
        t={t}
      />
      <RecordDetailDrawer
        record={detailRecord}
        visible={!!detailRecord}
        onClose={() => setDetailRecord(null)}
        onExportReplay={exportReplay}
        t={t}
      />
    </div>
  );
}
