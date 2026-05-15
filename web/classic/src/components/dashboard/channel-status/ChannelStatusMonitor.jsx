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

function formatPercent(value) {
  if (!Number.isFinite(Number(value))) return '0.00%';
  return `${Number(value).toFixed(2)}%`;
}

function formatSuccessRate(value, requests) {
  return Number(requests) > 0 ? formatPercent(value) : '--';
}

function formatLatency(value) {
  const latency = Number(value) || 0;
  if (latency <= 0) return '--';
  if (latency >= 1000) return `${(latency / 1000).toFixed(2)}s`;
  return `${Math.round(latency)}ms`;
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

function getGroupHealth(group) {
  if (!group || group.enabled_channels === 0) {
    return 'critical';
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
    Number(group.cooldown_channels) >= Number(group.enabled_channels) &&
    Number(group.enabled_channels) > 0
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
  if (summary.recent_requests > 0 && summary.success_rate < 99) {
    return { state: 'degraded', label: 'DEGRADED' };
  }
  if (
    summary.cooldown_channels >= summary.enabled_channels &&
    summary.enabled_channels > 0
  ) {
    return { state: 'degraded', label: 'DEGRADED' };
  }
  return { state: 'operational', label: 'OPERATIONAL' };
}

function isGroupAvailable(group) {
  if (!group || Number(group.enabled_channels) <= 0) {
    return false;
  }
  if (
    Number(group.cooldown_channels) >= Number(group.enabled_channels) &&
    Number(group.enabled_channels) > 0
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
  getEnabledChannels(group).forEach((channel) => {
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
  return Math.max(
    Number(channel?.concurrency_cooldown_remaining_seconds) || 0,
    Number(channel?.failure_avoidance_remaining_seconds) || 0,
  );
}

function getEnabledChannels(group) {
  return (group?.channels || []).filter(
    (channel) => channel?.enabled !== false,
  );
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
    default:
      return t('错误');
  }
}

function buildFallbackRecentStatus(group) {
  const total = Math.min(60, Math.max(Number(group?.recent_requests) || 0, 0));
  if (total <= 0) return [];
  const failures = [
    ...Array.from(
      { length: Number(group?.recent_error_429) || 0 },
      () => 'rate_limit',
    ),
    ...Array.from(
      { length: Number(group?.recent_error_5xx) || 0 },
      () => 'server_error',
    ),
    ...Array.from(
      { length: Number(group?.recent_error_timeout) || 0 },
      () => 'timeout',
    ),
  ];
  while (failures.length < Number(group?.recent_failures || 0)) {
    failures.push('error');
  }
  const status = Array.from({ length: total }, () => 'success');
  failures.slice(0, total).forEach((failure, index) => {
    status[Math.max(0, total - failures.length + index)] = failure;
  });
  return status;
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

  return (
    <div className='min-w-[220px]'>
      <div className='flex flex-wrap items-center gap-2'>
        <span className='font-semibold text-semi-color-text-0'>
          {channel.name}
        </span>
        <Typography.Text type='secondary' size='small'>
          #{channel.id}
        </Typography.Text>
        <HealthTag state={channel.health_state} />
        {!channel.enabled && (
          <Tag color='grey' shape='circle' size='small'>
            {t('禁用')}
          </Tag>
        )}
        {cooldown > 0 && (
          <Tooltip
            content={
              channel.failure_reason ||
              `${t('冷却中')} ${formatDuration(cooldown, t)}`
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
      : buildFallbackRecentStatus(group);

  return (
    <div className='ct-channel-monitor-history'>
      <div className='ct-channel-monitor-history-head'>
        <span>{t('近 60 次请求')}</span>
        <span>
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
          {t('暂无近期请求记录')}
        </div>
      )}
    </div>
  );
}

function GroupPanel({ group, windowDays }) {
  const { t } = useTranslation();
  const [detailsVisible, setDetailsVisible] = useState(false);
  const health = getGroupHealth(group);
  const enabledChannels = useMemo(() => getEnabledChannels(group), [group]);
  const modelPreview = useMemo(() => getGroupModelPreview(group), [group]);
  const dominantType = getDominantChannelType(enabledChannels);
  const dominantTypeMeta = getChannelTypeMeta(dominantType);
  const healthMeta = getHealthMeta(health, t);
  const canViewChannelDetails = isAdmin();
  const recentRequests = Number(group.recent_requests) || 0;
  const channelHealthPercent =
    group.total_channels > 0
      ? Math.round((group.healthy_channels / group.total_channels) * 100)
      : 0;
  const availabilityPercent = Math.max(
    0,
    Math.min(recentRequests > 0 ? Number(group.success_rate) || 0 : 0, 100),
  );

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
        title: t('健康评分'),
        dataIndex: 'health_score',
        width: 110,
        sorter: (a, b) => a.health_score - b.health_score,
        render: (value, record) => (
          <Typography.Text
            strong
            type={record.health_state === 'critical' ? 'danger' : 'primary'}
          >
            {formatNumber(value)}
          </Typography.Text>
        ),
      },
      {
        title: t('成功率'),
        dataIndex: 'success_rate',
        width: 110,
        sorter: (a, b) => a.success_rate - b.success_rate,
        render: (value) => formatPercent(value),
      },
      {
        title: t('请求数'),
        dataIndex: 'recent_requests',
        width: 120,
        sorter: (a, b) => a.recent_requests - b.recent_requests,
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
        title: t('延迟'),
        dataIndex: 'recent_avg_latency_ms',
        width: 120,
        sorter: (a, b) =>
          (a.recent_avg_latency_ms || a.response_time || 0) -
          (b.recent_avg_latency_ms || b.response_time || 0),
        render: (_, record) => (
          <div>
            <div className='font-mono font-semibold'>
              {formatLatency(
                record.recent_avg_latency_ms || record.response_time,
              )}
            </div>
            <div className='text-xs text-semi-color-text-2'>
              {t('首包')} {formatLatency(record.recent_avg_first_response_ms)}
            </div>
          </div>
        ),
      },
      {
        title: t('并发'),
        dataIndex: 'active_concurrency',
        width: 110,
        render: (_, record) => (
          <div>
            <div className='font-mono font-semibold'>
              {record.max_concurrency > 0
                ? `${record.active_concurrency}/${record.max_concurrency}`
                : formatNumber(record.active_concurrency)}
            </div>
            {record.concurrency_ceiling > 0 && (
              <div className='text-xs text-semi-color-text-2'>
                {t('上限')} {record.concurrency_ceiling}
              </div>
            )}
          </div>
        ),
      },
      {
        title: `429 / 5xx / ${t('超时')}`,
        dataIndex: 'recent_error_429',
        width: 140,
        render: (_, record) => (
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
        title: t('模型数'),
        dataIndex: 'models',
        width: 110,
        render: (value) => formatNumber(getChannelModelCount(value)),
      },
      {
        title: t('最近请求'),
        dataIndex: 'last_request_at',
        width: 170,
        render: (value) => (
          <Typography.Text type='secondary' size='small'>
            {formatTimestamp(value)}
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
              <strong>{formatNumber(group.enabled_channels)}</strong>
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
                  <span
                    className='ct-channel-monitor-model'
                    title={modelPreview.label}
                  >
                    {modelPreview.label}
                  </span>
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
                <span>{t('对话延迟')}</span>
              </div>
              <div className='ct-channel-monitor-metric-value'>
                {formatLatency(group.avg_latency_ms)}
              </div>
            </div>
            <div className='ct-channel-monitor-metric-box'>
              <div className='ct-channel-monitor-metric-label'>
                <Network size={16} />
                <span>{t('端点 PING')}</span>
              </div>
              <div className='ct-channel-monitor-metric-value'>
                {formatLatency(
                  Math.round(
                    enabledChannels.reduce(
                      (sum, channel) =>
                        sum + (Number(channel.response_time) || 0),
                      0,
                    ) / Math.max(enabledChannels.length, 1),
                  ),
                )}
              </div>
            </div>
          </div>

          <div className='ct-channel-monitor-availability'>
            <div className='ct-channel-monitor-availability-label'>
              {t('可用性')} · {windowDays} {t('天')}
            </div>
            <div className='ct-channel-monitor-availability-value'>
              {formatSuccessRate(group.success_rate, group.recent_requests)}
            </div>
            <div className='ct-channel-monitor-availability-sub'>
              + {formatNumber(modelPreview.count)} {t('模型')}
            </div>
            <div className='ct-channel-monitor-availability-track'>
              <span style={{ width: `${availabilityPercent}%` }} />
            </div>
          </div>

          <div className='ct-channel-monitor-quick-grid'>
            <div>
              <span>{t('请求数')}</span>
              <strong>{formatNumber(group.recent_requests)}</strong>
            </div>
            <div>
              <span>429 / 5xx / {t('超时')}</span>
              <strong>
                {formatNumber(group.recent_error_429)} /{' '}
                {formatNumber(group.recent_error_5xx)} /{' '}
                {formatNumber(group.recent_error_timeout)}
              </strong>
            </div>
            <div>
              <span>
                {t('冷却')} / {t('忙碌')}
              </span>
              <strong>
                {formatNumber(group.cooldown_channels)} /{' '}
                {formatNumber(group.busy_channels)}
              </strong>
            </div>
            <div>
              <span>{t('健康渠道占比')}</span>
              <strong>{channelHealthPercent}%</strong>
            </div>
          </div>

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
            {t('渠道数')} {group.enabled_channels}
          </Tag>
          <Tag shape='circle'>
            {t('成功率')}{' '}
            {formatSuccessRate(group.success_rate, group.recent_requests)}
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
          dataSource={enabledChannels}
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
          label={t('分组可用')}
          value={formatPercent(availableGroupPercent)}
          detail={`${formatNumber(availableGroups)} / ${formatNumber(groups.length)} ${t('可用分组')} · ${formatNumber(unhealthyChannels)} ${t('需关注渠道')}`}
          tone={availableGroups === groups.length ? 'success' : 'warning'}
        />
        <SummaryMetric
          icon={Activity}
          label={t('最终成功率')}
          value={formatSuccessRate(
            summary.success_rate,
            summary.recent_requests,
          )}
          detail={
            isPartial
              ? t('统计刷新中，当前显示基础渠道状态')
              : `${formatNumber(summary.recent_requests)} / ${windowDays}${t('天')} ${t('请求数')}`
          }
          tone={
            summary.success_rate >= 99 || summary.recent_requests === 0
              ? 'success'
              : 'warning'
          }
        />
        <SummaryMetric
          icon={AlertTriangle}
          label={t('最终失败')}
          value={formatNumber(summary.recent_failures)}
          detail={`429 ${formatNumber(summary.recent_error_429)} / 5xx ${formatNumber(summary.recent_error_5xx)}`}
          tone={
            summary.recent_requests > 0 && summary.success_rate < 99
              ? 'danger'
              : 'success'
          }
        />
        <SummaryMetric
          icon={Gauge}
          label={t('活跃负载')}
          value={formatNumber(summary.busy_channels)}
          detail={`${formatLatency(summary.avg_latency_ms)} ${t('平均延迟')} / ${formatNumber(summary.cooldown_channels)} ${t('冷却')}`}
          tone={summary.cooldown_channels > 0 ? 'warning' : 'default'}
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
    const timer = window.setInterval(() => {
      setRefreshCountdown((value) =>
        value <= 1 ? REFRESH_INTERVAL_SECONDS : value - 1,
      );
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  const statusTip = useMemo(
    () => t('根据真实请求最终结果、限流错误、冷却状态与并发配置计算健康状态'),
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
