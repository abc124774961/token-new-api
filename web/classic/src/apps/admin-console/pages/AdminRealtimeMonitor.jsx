import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Activity,
  Gauge,
  GitBranch,
  RefreshCw,
  ShieldAlert,
  Timer,
} from 'lucide-react';
import { API, timestamp2string } from '../../../helpers';

function formatNumber(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '0';
  return new Intl.NumberFormat('zh-CN', {
    maximumFractionDigits: numeric >= 1000 ? 0 : 1,
  }).format(numeric);
}

function formatPercent(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `${(numeric * 100).toFixed(1)}%`;
}

function formatLatency(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return numeric >= 1000
    ? `${(numeric / 1000).toFixed(2)}s`
    : `${Math.round(numeric)}ms`;
}

function formatTimestamp(value) {
  const numeric = Number(value);
  return numeric > 0 ? timestamp2string(numeric) : '--';
}

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

const MetricCard = ({ icon: Icon, label, value, helper, tone = '' }) => (
  <div className={`aurora-metric-card ${tone}`}>
    <span className='aurora-metric-icon'>
      <Icon size={20} />
    </span>
    <span>
      <span className='aurora-metric-label'>{label}</span>
      <strong className='aurora-metric-value'>{value}</strong>
      <small className='aurora-metric-helper'>{helper}</small>
    </span>
  </div>
);

const AdminRealtimeMonitor = () => {
  const { t } = useTranslation();
  const [runtime, setRuntime] = useState({ summary: {}, items: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [updatedAt, setUpdatedAt] = useState(0);

  const fetchRuntime = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await API.get(
        '/api/model_gateway/observability/runtime',
        {
          disableDuplicate: true,
          skipErrorHandler: true,
        },
      );
      const payload = unwrapApiData(response);
      setRuntime({
        summary: payload.summary || {},
        items: Array.isArray(payload.items) ? payload.items : [],
      });
      setUpdatedAt(Math.floor(Date.now() / 1000));
    } catch (err) {
      setRuntime({ summary: {}, items: [] });
      setError(err?.response?.data?.message || err?.message || t('加载失败'));
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    fetchRuntime();
  }, [fetchRuntime]);

  const summary = runtime.summary || {};
  const riskyItems = useMemo(() => {
    return [...(runtime.items || [])]
      .filter(
        (item) =>
          item.circuit_open ||
          item.cooldown ||
          Number(item.queue_depth || 0) > 0 ||
          Number(item.active_concurrency || 0) > 0 ||
          Number(item.success_rate || 0) < 0.95,
      )
      .sort((a, b) => {
        const queueDelta =
          Number(b.queue_depth || 0) - Number(a.queue_depth || 0);
        if (queueDelta !== 0) return queueDelta;
        return (
          Number(b.active_concurrency || 0) - Number(a.active_concurrency || 0)
        );
      })
      .slice(0, 10);
  }, [runtime.items]);

  return (
    <div className='aurora-admin-page aurora-realtime-monitor-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('运营首页')}</div>
          <h1>{t('实时监控')}</h1>
          <p>
            {t(
              '查看智能网关运行键、队列、并发、熔断和高压渠道，辅助判断当前请求链路是否稳定。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>
              {t('最后刷新')} {formatTimestamp(updatedAt)}
            </span>
            <span>
              {t('运行键')} {formatNumber(summary.runtime_keys)}
            </span>
          </div>
        </div>
        <div className='aurora-overview-status aurora-status-success'>
          <span>{t('运行状态')}</span>
          <strong>{loading ? t('刷新中') : t('在线')}</strong>
          <em>{t('来自 runtime 快照')}</em>
          <button
            className='aurora-refresh-button'
            type='button'
            onClick={fetchRuntime}
            disabled={loading}
          >
            <RefreshCw size={14} className={loading ? 'is-spinning' : ''} />
            {t('刷新')}
          </button>
        </div>
      </section>

      {error && <div className='aurora-inline-error'>{error}</div>}

      <section className='aurora-metric-grid'>
        <MetricCard
          icon={GitBranch}
          label={t('运行键')}
          value={formatNumber(summary.runtime_keys)}
          helper={`${t('渠道')} ${formatNumber(summary.channels)}`}
        />
        <MetricCard
          icon={Activity}
          label={t('活跃并发')}
          value={formatNumber(summary.active_concurrency)}
          helper={`${t('排队请求')} ${formatNumber(summary.queued_requests)}`}
          tone='aurora-tone-success'
        />
        <MetricCard
          icon={ShieldAlert}
          label={t('熔断中')}
          value={formatNumber(summary.circuit_open)}
          helper={`${t('半开')} ${formatNumber(summary.circuit_half_open)}`}
          tone={
            Number(summary.circuit_open || 0) > 0 ? 'aurora-tone-danger' : ''
          }
        />
        <MetricCard
          icon={Gauge}
          label={t('高压渠道')}
          value={formatNumber(summary.high_pressure_channels)}
          helper={`${t('饱和')} ${formatNumber(summary.saturated_channels)}`}
          tone={
            Number(summary.high_pressure_channels || 0) > 0
              ? 'aurora-tone-warning'
              : ''
          }
        />
      </section>

      <section className='aurora-panel'>
        <div className='aurora-section-head'>
          <div>
            <h2>{t('运行快照')}</h2>
            <p>{t('优先展示有队列、并发、熔断或低成功率的运行键。')}</p>
          </div>
        </div>
        <div className='aurora-simple-table-wrap'>
          <table className='aurora-simple-table aurora-runtime-table'>
            <thead>
              <tr>
                <th>{t('渠道')}</th>
                <th>{t('模型')}</th>
                <th>{t('分组')}</th>
                <th>{t('成功率')}</th>
                <th>{t('TTFT')}</th>
                <th>{t('并发')}</th>
                <th>{t('队列')}</th>
                <th>{t('状态')}</th>
              </tr>
            </thead>
            <tbody>
              {riskyItems.map((item, index) => (
                <tr
                  key={`${item.channel_id}-${item.requested_model}-${item.group}-${index}`}
                >
                  <td>
                    <strong>{item.channel_name || '-'}</strong>
                    <small>ID {item.channel_id || '-'}</small>
                  </td>
                  <td>{item.requested_model || item.upstream_model || '-'}</td>
                  <td>{item.group || '-'}</td>
                  <td>{formatPercent(item.success_rate)}</td>
                  <td>{formatLatency(item.ttft_ms)}</td>
                  <td>
                    {formatNumber(item.active_concurrency)} /{' '}
                    {formatNumber(
                      item.effective_concurrency_limit || item.max_concurrency,
                    )}
                  </td>
                  <td>{formatNumber(item.queue_depth)}</td>
                  <td>
                    <span
                      className={`aurora-status-pill ${
                        item.circuit_open || item.cooldown
                          ? 'is-warning'
                          : 'is-success'
                      }`}
                    >
                      <Timer size={13} />
                      {item.circuit_open
                        ? t('熔断')
                        : item.cooldown
                          ? t('冷却')
                          : t('正常')}
                    </span>
                  </td>
                </tr>
              ))}
              {riskyItems.length === 0 && (
                <tr>
                  <td colSpan={8}>
                    <div className='aurora-empty-state'>
                      {loading ? t('加载中') : t('暂无高风险运行键')}
                    </div>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
};

export default AdminRealtimeMonitor;
