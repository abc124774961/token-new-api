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
  Button,
  Empty,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock3,
  CreditCard,
  Eye,
  Layers,
  Pause,
  Play,
  RefreshCw,
  Route,
  ShieldAlert,
  Zap,
} from 'lucide-react';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import './channel-balance-monitor.css';

const { Text } = Typography;

const REFRESH_INTERVAL_MS = 60 * 1000;

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function formatTimestamp(timestamp) {
  return Number(timestamp || 0) > 0 ? timestamp2string(Number(timestamp)) : '--';
}

function formatBalance(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `$${numeric.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatRatio(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `${numeric.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}x`;
}

function formatCurrencyValue(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `$${numeric.toFixed(6).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function ratioRangeText(min, max, formatValue = formatRatio) {
  const minValue = Number(min);
  const maxValue = Number(max);
  if (!Number.isFinite(minValue) || !Number.isFinite(maxValue) || (minValue === 0 && maxValue === 0)) {
    return '--';
  }
  if (minValue === maxValue) return formatValue(minValue);
  return `${formatValue(minValue)}-${formatValue(maxValue)}`;
}

function RatioSummaryView({ summary, t }) {
  const ratioSummary = summary || {};
  const models = Array.isArray(ratioSummary.models) ? ratioSummary.models : [];
  const modelCount = Number(ratioSummary.model_count || 0);
  const hasModelRatio = Number(ratioSummary.model_ratio_max || 0) > 0;
  const modelText = hasModelRatio
    ? ratioRangeText(ratioSummary.model_ratio_min, ratioSummary.model_ratio_max)
    : t('按次');
  const tooltip = (
    <div className='cbm-ratio-tooltip'>
      <div>{t('分组倍率')}：{formatRatio(ratioSummary.group_ratio || 1)}</div>
      <div>{t('模型倍率')}：{hasModelRatio ? modelText : t('按次')}</div>
      {Number(ratioSummary.completion_ratio_max || 0) > 0 && (
        <div>{t('输出倍率')}：{ratioRangeText(ratioSummary.completion_ratio_min, ratioSummary.completion_ratio_max)}</div>
      )}
      {models.map((item) => (
        <div className='cbm-ratio-tooltip-row' key={`${item.model}-${item.pricing_model || ''}`}>
          <span>{item.model}</span>
          <span>
            {item.use_price
              ? formatCurrencyValue(item.model_price)
              : formatRatio(item.model_ratio)}
            {Number(item.completion_ratio || 0) > 0 ? ` / ${t('输出')} ${formatRatio(item.completion_ratio)}` : ''}
          </span>
        </div>
      ))}
      {modelCount > models.length && (
        <div>{t('共 {{count}} 个模型', { count: modelCount })}</div>
      )}
    </div>
  );

  return (
    <Tooltip content={tooltip} position='topLeft'>
      <div className='cbm-ratio-cell'>
        <div className='cbm-ratio-main'>
          <span>{t('分组')} {formatRatio(ratioSummary.group_ratio || 1)}</span>
          <span>{modelText}</span>
        </div>
        <div className='cbm-ratio-detail'>
          {modelCount > 0
            ? `${modelCount} ${t('个模型')}`
            : t('无倍率数据')}
        </div>
      </div>
    </Tooltip>
  );
}

function statusMeta(status, t) {
  switch (status) {
    case 'ok':
      return { color: 'green', label: t('正常') };
    case 'low':
      return { color: 'orange', label: t('低余额') };
    case 'empty':
      return { color: 'red', label: t('余额耗尽') };
    case 'unsupported':
      return { color: 'grey', label: t('不支持查询') };
    case 'error':
      return { color: 'red', label: t('刷新失败') };
    default:
      return { color: 'blue', label: t('未知') };
  }
}

function eventMeta(eventType, t) {
  switch (eventType) {
    case 'balance_low':
      return { color: 'orange', label: t('低余额') };
    case 'balance_empty':
      return { color: 'red', label: t('余额耗尽') };
    case 'balance_recovered':
      return { color: 'green', label: t('余额恢复') };
    case 'refresh_failed':
      return { color: 'red', label: t('刷新失败') };
    case 'unsupported':
      return { color: 'grey', label: t('不支持查询') };
    case 'ratio_applied':
      return { color: 'green', label: t('倍率已应用') };
    case 'ratio_conflict':
      return { color: 'orange', label: t('倍率冲突') };
    case 'ratio_failed':
      return { color: 'red', label: t('倍率同步失败') };
    default:
      return { color: 'blue', label: eventType || t('事件') };
  }
}

function MetricCard({ icon: Icon, label, value, detail, tone = 'teal' }) {
  return (
    <div className={`cbm-metric cbm-metric-${tone}`}>
      <div>
        <div className='cbm-metric-label'>{label}</div>
        <div className='cbm-metric-value'>{value}</div>
        {detail && <div className='cbm-metric-detail'>{detail}</div>}
      </div>
      <div className='cbm-metric-icon'>
        <Icon size={18} />
      </div>
    </div>
  );
}

export default function ChannelBalanceMonitor() {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [refreshingMode, setRefreshingMode] = useState('');
  const [data, setData] = useState({
    summary: {},
    accounts: [],
    channels: [],
    events: [],
    settings: {},
  });

  const fetchData = useCallback(async ({ silent = false } = {}) => {
    if (!silent) setLoading(true);
    try {
      const response = await API.get('/api/channel/balance_monitor');
      const payload = unwrapApiData(response);
      setData({
        summary: payload.summary || {},
        accounts: payload.accounts || [],
        channels: payload.channels || [],
        events: payload.events || [],
        settings: payload.settings || {},
      });
    } catch (error) {
      showError(t('获取渠道余额监控失败：') + (error.message || ''));
    } finally {
      if (!silent) setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchData();
    const timer = window.setInterval(() => fetchData({ silent: true }), REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [fetchData]);

  const refreshMonitor = async (mode, extra = {}, loadingKey = mode) => {
    setRefreshingMode(loadingKey);
    try {
      const response = await API.post('/api/channel/balance_monitor/refresh', {
        mode,
        ...extra,
      });
      const payload = unwrapApiData(response);
      if (payload.monitor) {
        setData(payload.monitor);
      } else {
        await fetchData({ silent: true });
      }
      showSuccess(t('刷新完成'));
    } catch (error) {
      showError(t('刷新失败：') + (error.message || ''));
    } finally {
      setRefreshingMode('');
    }
  };

  const updateAccountStatus = async (record, enabled) => {
    const loadingKey = `status-${record.channel_id}-${record.credential_index}`;
    setRefreshingMode(loadingKey);
    try {
      const response = await API.post(
        `/api/channel/${record.channel_id}/accounts/${record.credential_index}/status`,
        {
          enabled,
          reason: enabled ? '' : 'manual_disabled',
        },
      );
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('操作失败'));
      }
      await fetchData({ silent: true });
      showSuccess(enabled ? t('账号已启用') : t('账号已禁用'));
    } catch (error) {
      showError((enabled ? t('启用账号失败：') : t('禁用账号失败：')) + (error.message || ''));
    } finally {
      setRefreshingMode('');
    }
  };

  const summary = data.summary || {};
  const settings = data.settings || {};

  const accountColumns = useMemo(() => [
    {
      title: t('账号'),
      dataIndex: 'account_id',
      render: (_, record) => (
        <div className='cbm-account-cell'>
          <Text strong ellipsis={{ showTooltip: true }}>
            {record.account_display_name || record.account_id || `#${record.credential_index + 1}`}
          </Text>
          <Text type='tertiary' size='small' ellipsis={{ showTooltip: true }}>
            {record.account_id || record.account_identity_key || '--'}
          </Text>
        </div>
      ),
    },
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      render: (_, record) => (
        <div className='cbm-account-cell'>
          <Text strong>{record.channel_name}</Text>
          <Text type='tertiary' size='small'>
            #{record.channel_id} · {record.channel_type_name || record.channel_type} · {record.group || 'default'}
          </Text>
        </div>
      ),
    },
    {
      title: t('凭证'),
      dataIndex: 'credential_index',
      width: 92,
      render: (value, record) => (
        <Tag color={record.is_multi_key ? 'teal' : 'blue'}>
          {record.is_multi_key ? `#${Number(value) + 1}` : t('单账号')}
        </Tag>
      ),
    },
    {
      title: t('余额'),
      dataIndex: 'balance',
      width: 110,
      render: (value) => <Text strong>{formatBalance(value)}</Text>,
    },
    {
      title: t('倍率'),
      dataIndex: 'ratio_summary',
      width: 170,
      render: (value) => <RatioSummaryView summary={value} t={t} />,
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 120,
      render: (status, record) => {
        const meta = statusMeta(status, t);
        return (
          <Tooltip content={record.last_error || record.disabled_reason || ''}>
            <Tag color={meta.color}>{meta.label}</Tag>
          </Tooltip>
        );
      },
    },
    {
      title: t('影响范围'),
      dataIndex: 'affected_channels',
      width: 190,
      render: (value, record) => {
        const labels = Array.isArray(value) && value.length > 0
          ? value
          : [`${record.channel_name}(#${record.channel_id})`];
        return (
          <Tooltip content={labels.join(' / ')}>
            <Text type='tertiary' ellipsis={{ showTooltip: false }}>
              {labels.length > 1 ? `${labels[0]} +${labels.length - 1}` : labels[0]}
            </Text>
          </Tooltip>
        );
      },
    },
    {
      title: t('最近刷新'),
      dataIndex: 'last_updated_time',
      width: 160,
      render: (value) => <Text type='tertiary'>{formatTimestamp(value)}</Text>,
    },
    {
      title: t('操作'),
      dataIndex: 'action',
      width: 185,
      render: (_, record) => (
        <Space>
          <Tooltip content={t('刷新单账号')}>
            <Button
              icon={<RefreshCw size={14} />}
              size='small'
              loading={refreshingMode === `account-${record.channel_id}-${record.credential_index}`}
              onClick={() =>
                refreshMonitor(
                  'balance',
                  {
                    channel_id: record.channel_id,
                    credential_index: record.credential_index,
                  },
                  `account-${record.channel_id}-${record.credential_index}`,
                )
              }
            />
          </Tooltip>
          <Tooltip content={record.key_enabled ? t('暂停账号') : t('恢复账号')}>
            <Button
              icon={record.key_enabled ? <Pause size={14} /> : <Play size={14} />}
              size='small'
              loading={refreshingMode === `status-${record.channel_id}-${record.credential_index}`}
              onClick={() => updateAccountStatus(record, !record.key_enabled)}
            />
          </Tooltip>
          <Tooltip content={t('查看影响渠道')}>
            <Button
              icon={<Eye size={14} />}
              size='small'
              onClick={() => {
                window.location.href = `/console/channel/${record.channel_id}/accounts`;
              }}
            />
          </Tooltip>
        </Space>
      ),
    },
  ], [fetchData, refreshingMode, t]);

  const channelColumns = useMemo(() => [
    {
      title: t('渠道'),
      dataIndex: 'channel_name',
      render: (_, record) => (
        <div className='cbm-account-cell'>
          <Text strong>{record.channel_name}</Text>
          <Text type='tertiary' size='small'>
            #{record.channel_id} · {record.channel_type_name || record.channel_type}
          </Text>
        </div>
      ),
    },
    {
      title: t('账号健康'),
      dataIndex: 'account_total',
      render: (_, record) => (
        <Space wrap>
          <Tag color='green'>{t('可用')} {record.enabled_accounts}</Tag>
          <Tag color='orange'>{t('低余额')} {record.low_balance_accounts}</Tag>
          <Tag color='red'>{t('耗尽')} {record.empty_accounts}</Tag>
          <Tag color='grey'>{t('不支持')} {record.unsupported_accounts}</Tag>
        </Space>
      ),
    },
    {
      title: t('倍率'),
      dataIndex: 'ratio_summary',
      width: 170,
      render: (value) => <RatioSummaryView summary={value} t={t} />,
    },
    {
      title: t('聚合状态'),
      dataIndex: 'aggregate_status',
      width: 120,
      render: (status) => {
        const meta = statusMeta(status, t);
        return <Tag color={meta.color}>{meta.label}</Tag>;
      },
    },
  ], [t]);

  return (
    <div className='cbm-page'>
      <div className='cbm-hero'>
        <div className='cbm-title-block'>
          <div className='cbm-title-icon'>
            <CreditCard size={22} />
          </div>
          <div>
            <div className='cbm-eyebrow'>{t('账号余额运营')}</div>
            <h2>{t('渠道余额监控')}</h2>
            <p>{t('按账号监控余额、按渠道聚合影响，并记录倍率同步事件')}</p>
          </div>
        </div>
        <Space wrap className='cbm-actions'>
          <Button
            icon={<RefreshCw size={15} />}
            loading={refreshingMode === 'balance'}
            onClick={() => refreshMonitor('balance')}
          >
            {t('刷新余额')}
          </Button>
          <Button
            icon={<Zap size={15} />}
            loading={refreshingMode === 'ratio'}
            onClick={() => refreshMonitor('ratio')}
          >
            {t('同步倍率')}
          </Button>
          <Button
            type='primary'
            icon={<Activity size={15} />}
            loading={refreshingMode === 'all'}
            onClick={() => refreshMonitor('all')}
          >
            {t('全部刷新')}
          </Button>
        </Space>
      </div>

      <div className='cbm-metric-grid'>
        <MetricCard icon={CreditCard} label={t('账号总数')} value={summary.account_total || 0} detail={`${t('阈值')} ${formatBalance(settings.warning_threshold)}`} />
        <MetricCard icon={AlertTriangle} label={t('低余额账号')} value={summary.low_balance_accounts || 0} detail={t('需要运营关注')} tone='orange' />
        <MetricCard icon={ShieldAlert} label={t('耗尽账号')} value={summary.empty_accounts || 0} detail={`${t('受影响渠道')} ${summary.affected_channels || 0}`} tone='red' />
        <MetricCard icon={Zap} label={t('倍率自动应用')} value={summary.ratio_auto_applied || 0} detail={`${t('冲突')} ${summary.ratio_conflicts || 0}`} tone='green' />
        <MetricCard icon={Clock3} label={t('最后同步')} value={formatTimestamp(summary.last_sync_time)} detail={settings.enabled ? t('定时监控已开启') : t('定时监控未开启')} tone='blue' />
      </div>

      <div className='cbm-grid'>
        <section className='cbm-panel cbm-panel-main'>
          <div className='cbm-panel-header'>
            <div>
              <h3>{t('账号余额')}</h3>
              <p>{t('充值、告警和停用均按账号维度处理')}</p>
            </div>
          </div>
          <Table
            loading={loading}
            columns={accountColumns}
            dataSource={data.accounts || []}
            rowKey={(record) => `${record.channel_id}-${record.credential_index}-${record.account_id}`}
            pagination={{ pageSize: 10 }}
            empty={<Empty description={t('暂无账号余额数据')} />}
          />
        </section>

        <section className='cbm-panel'>
          <div className='cbm-panel-header'>
            <div>
              <h3>{t('渠道聚合')}</h3>
              <p>{t('查看账号余额对渠道可用性的影响')}</p>
            </div>
          </div>
          <Table
            columns={channelColumns}
            dataSource={data.channels || []}
            rowKey='channel_id'
            pagination={false}
            size='small'
            empty={<Empty description={t('暂无渠道聚合数据')} />}
          />
        </section>
      </div>

      <section className='cbm-panel'>
        <div className='cbm-panel-header'>
          <div>
            <h3>{t('最近事件')}</h3>
            <p>{t('余额告警、账号恢复和倍率同步结果')}</p>
          </div>
        </div>
        <div className='cbm-event-list'>
          {(data.events || []).map((event) => {
            const meta = eventMeta(event.event_type, t);
            return (
              <div className='cbm-event' key={event.id}>
                <div className='cbm-event-icon'>
                  {event.scope === 'ratio' ? <Route size={15} /> : <Layers size={15} />}
                </div>
                <div className='cbm-event-body'>
                  <Space wrap>
                    <Tag color={meta.color}>{meta.label}</Tag>
                    <Text strong>{event.channel_name || event.model_name || event.field || t('系统事件')}</Text>
                    {event.account_id && <Text type='tertiary'>{event.account_id}</Text>}
                  </Space>
                  <Text type='tertiary' size='small'>
                    {formatTimestamp(event.created_time)}
                    {event.error ? ` · ${event.error}` : ''}
                    {event.balance ? ` · ${formatBalance(event.balance)}` : ''}
                  </Text>
                </div>
                {event.auto_applied && (
                  <Tag color='green' prefixIcon={<CheckCircle2 size={12} />}>
                    {t('已应用')}
                  </Tag>
                )}
              </div>
            );
          })}
          {(data.events || []).length === 0 && (
            <Empty description={t('暂无监控事件')} />
          )}
        </div>
      </section>
    </div>
  );
}
