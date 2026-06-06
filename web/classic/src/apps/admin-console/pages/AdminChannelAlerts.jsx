import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  AlertTriangle,
  HeartPulse,
  RefreshCw,
  ShieldCheck,
  WalletCards,
  Zap,
} from 'lucide-react';
import { API, timestamp2string } from '../../../helpers';

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function formatNumber(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '0';
  return new Intl.NumberFormat('zh-CN').format(numeric);
}

function formatTimestamp(value) {
  const numeric = Number(value);
  return numeric > 0 ? timestamp2string(numeric) : '--';
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

const AdminChannelAlerts = () => {
  const { t } = useTranslation();
  const [data, setData] = useState({
    health: { summary: {}, items: [], reason_counts: [] },
    balance: { summary: {}, accounts: [] },
  });
  const [loading, setLoading] = useState(false);
  const [errors, setErrors] = useState({});
  const [updatedAt, setUpdatedAt] = useState(0);

  const fetchAlerts = useCallback(async () => {
    setLoading(true);
    const nextErrors = {};
    const [healthResult, balanceResult] = await Promise.allSettled([
      API.get('/api/model_gateway/observability/health-check/queue', {
        params: { limit: 50, queue_type: 'all' },
        disableDuplicate: true,
        skipErrorHandler: true,
      }),
      API.get('/api/channel/balance_monitor', {
        disableDuplicate: true,
        skipErrorHandler: true,
      }),
    ]);

    const nextData = {
      health: { summary: {}, items: [], reason_counts: [] },
      balance: { summary: {}, accounts: [] },
    };

    if (healthResult.status === 'fulfilled') {
      const payload = unwrapApiData(healthResult.value);
      nextData.health = {
        summary: payload.summary || {},
        items: Array.isArray(payload.items) ? payload.items : [],
        reason_counts: Array.isArray(payload.reason_counts)
          ? payload.reason_counts
          : [],
      };
    } else {
      nextErrors.health =
        healthResult.reason?.response?.data?.message ||
        healthResult.reason?.message ||
        t('加载失败');
    }

    if (balanceResult.status === 'fulfilled') {
      const payload = unwrapApiData(balanceResult.value);
      nextData.balance = {
        summary: payload.summary || {},
        accounts: Array.isArray(payload.accounts) ? payload.accounts : [],
      };
    } else {
      nextErrors.balance =
        balanceResult.reason?.response?.data?.message ||
        balanceResult.reason?.message ||
        t('加载失败');
    }

    setData(nextData);
    setErrors(nextErrors);
    setUpdatedAt(Math.floor(Date.now() / 1000));
    setLoading(false);
  }, [t]);

  useEffect(() => {
    fetchAlerts();
  }, [fetchAlerts]);

  const healthSummary = data.health.summary || {};
  const balanceSummary = data.balance.summary || {};
  const alertTotal =
    Number(healthSummary.pending_count || 0) +
    Number(healthSummary.isolated_count || 0) +
    Number(healthSummary.score_anomaly_count || 0) +
    Number(balanceSummary.low_balance_accounts || 0) +
    Number(balanceSummary.empty_accounts || 0) +
    Number(balanceSummary.error_accounts || 0);

  const balanceAlerts = useMemo(() => {
    return (data.balance.accounts || [])
      .filter(
        (account) =>
          Number(account.low_balance_accounts || 0) > 0 ||
          Number(account.empty_accounts || 0) > 0 ||
          Number(account.error_accounts || 0) > 0 ||
          account.status === 'low' ||
          account.status === 'empty' ||
          account.status === 'error',
      )
      .slice(0, 8);
  }, [data.balance.accounts]);

  return (
    <div className='aurora-admin-page aurora-channel-alerts-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('运营首页')}</div>
          <h1>{t('渠道预警')}</h1>
          <p>
            {t(
              '集中查看健康检测队列、低分渠道、隔离渠道和余额异常，帮助运营优先处理影响可用性的渠道。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>
              {t('最后刷新')} {formatTimestamp(updatedAt)}
            </span>
            <span>
              {t('预警总数')} {formatNumber(alertTotal)}
            </span>
          </div>
        </div>
        <div
          className={`aurora-overview-status ${
            alertTotal > 0 ? 'aurora-status-warning' : 'aurora-status-success'
          }`}
        >
          <span>{t('当前预警')}</span>
          <strong>{formatNumber(alertTotal)}</strong>
          <em>{alertTotal > 0 ? t('需要处理') : t('正常')}</em>
          <button
            className='aurora-refresh-button'
            type='button'
            onClick={fetchAlerts}
            disabled={loading}
          >
            <RefreshCw size={14} className={loading ? 'is-spinning' : ''} />
            {t('刷新')}
          </button>
        </div>
      </section>

      {Object.values(errors).map((error) => (
        <div className='aurora-inline-error' key={error}>
          {error}
        </div>
      ))}

      <section className='aurora-metric-grid'>
        <MetricCard
          icon={HeartPulse}
          label={t('待检测')}
          value={formatNumber(healthSummary.pending_count)}
          helper={`${t('返回')} ${formatNumber(healthSummary.returned_count)}`}
          tone={
            Number(healthSummary.pending_count || 0) > 0
              ? 'aurora-tone-warning'
              : ''
          }
        />
        <MetricCard
          icon={ShieldCheck}
          label={t('隔离中')}
          value={formatNumber(healthSummary.isolated_count)}
          helper={`${t('快速校准')} ${formatNumber(healthSummary.score_anomaly_count)}`}
          tone={
            Number(healthSummary.isolated_count || 0) > 0
              ? 'aurora-tone-danger'
              : ''
          }
        />
        <MetricCard
          icon={WalletCards}
          label={t('低余额')}
          value={formatNumber(balanceSummary.low_balance_accounts)}
          helper={`${t('耗尽账号')} ${formatNumber(balanceSummary.empty_accounts)}`}
          tone={
            Number(balanceSummary.low_balance_accounts || 0) > 0
              ? 'aurora-tone-warning'
              : ''
          }
        />
        <MetricCard
          icon={Zap}
          label={t('运行队列')}
          value={formatNumber(healthSummary.queued_requests)}
          helper={`${t('活跃并发')} ${formatNumber(healthSummary.active_concurrency)}`}
        />
      </section>

      <section className='aurora-overview-grid'>
        <div className='aurora-panel aurora-panel-main'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('健康检测队列')}</h2>
              <p>{t('按优先级展示当前需要探活或恢复校准的渠道。')}</p>
            </div>
            <Link
              className='aurora-risk-action'
              to='/admin/channel-health-check'
            >
              {t('查看健康检测')}
            </Link>
          </div>
          <div className='aurora-simple-table-wrap'>
            <table className='aurora-simple-table'>
              <thead>
                <tr>
                  <th>{t('渠道')}</th>
                  <th>{t('模型')}</th>
                  <th>{t('分组')}</th>
                  <th>{t('评分')}</th>
                  <th>{t('原因')}</th>
                </tr>
              </thead>
              <tbody>
                {(data.health.items || []).slice(0, 8).map((item, index) => (
                  <tr key={item.row_key || `${item.channel_id}-${index}`}>
                    <td>
                      <strong>{item.channel_name || '-'}</strong>
                      <small>ID {item.channel_id || '-'}</small>
                    </td>
                    <td>
                      {item.requested_model || item.upstream_model || '-'}
                    </td>
                    <td>{item.group || '-'}</td>
                    <td>{formatNumber(item.score_total)}</td>
                    <td>
                      {(item.reasons || [])
                        .map((reason) => reason.label || reason.key)
                        .filter(Boolean)
                        .join(' / ') || '-'}
                    </td>
                  </tr>
                ))}
                {(data.health.items || []).length === 0 && (
                  <tr>
                    <td colSpan={5}>
                      <div className='aurora-empty-state'>
                        {loading ? t('加载中') : t('暂无健康检测预警')}
                      </div>
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>
        </div>

        <div className='aurora-panel'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('余额预警')}</h2>
              <p>{t('低余额、耗尽或余额刷新异常账号。')}</p>
            </div>
            <Link
              className='aurora-risk-action'
              to='/admin/channel-balance-monitor'
            >
              {t('查看余额监控')}
            </Link>
          </div>
          <div className='aurora-risk-list'>
            {balanceAlerts.map((account, index) => (
              <div
                className='aurora-risk-item aurora-tone-warning'
                key={`${account.channel_id}-${index}`}
              >
                <span className='aurora-risk-icon'>
                  <AlertTriangle size={18} />
                </span>
                <span className='aurora-risk-body'>
                  <span className='aurora-risk-title'>
                    {account.channel_name ||
                      account.account_display_name ||
                      '-'}
                    <em>ID {account.channel_id || '-'}</em>
                  </span>
                  <span className='aurora-risk-desc'>
                    {t('低余额')} {formatNumber(account.low_balance_accounts)} ·{' '}
                    {t('耗尽')} {formatNumber(account.empty_accounts)} ·{' '}
                    {t('异常')} {formatNumber(account.error_accounts)}
                  </span>
                </span>
              </div>
            ))}
            {balanceAlerts.length === 0 && (
              <div className='aurora-empty-state'>
                {loading ? t('加载中') : t('暂无余额预警')}
              </div>
            )}
          </div>
        </div>
      </section>
    </div>
  );
};

export default AdminChannelAlerts;
