import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  Activity,
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  CircleDollarSign,
  Database,
  Gauge,
  HeartPulse,
  Network,
  RefreshCw,
  ShieldCheck,
  WalletCards,
} from 'lucide-react';
import { API, timestamp2string } from '../../../helpers';

const OVERVIEW_SOURCE_DEFINITIONS = [
  {
    key: 'observability',
    label: '网关观测',
    request: () =>
      API.get('/api/model_gateway/observability/summary', {
        params: {
          hours: 24,
          recent_limit: 20,
          top_n: 6,
          scan_limit: 5000,
          view_mode: 'user_requests',
        },
        disableDuplicate: true,
        skipErrorHandler: true,
      }),
  },
  {
    key: 'healthQueue',
    label: '健康检测队列',
    request: () =>
      API.get('/api/model_gateway/observability/health-check/queue', {
        params: {
          limit: 20,
          queue_type: 'all',
        },
        disableDuplicate: true,
        skipErrorHandler: true,
      }),
  },
  {
    key: 'balance',
    label: '余额监控',
    request: () =>
      API.get('/api/channel/balance_monitor', {
        disableDuplicate: true,
        skipErrorHandler: true,
      }),
  },
  {
    key: 'profit',
    label: '盈利监控',
    request: () =>
      API.get('/api/model_gateway/profit_monitor/summary', {
        params: {
          window: '24h',
          breakdown_dimension: 'channel',
        },
        disableDuplicate: true,
        skipErrorHandler: true,
      }),
  },
];

const workspaceItems = [
  {
    title: '渠道管理',
    desc: '维护渠道、模型映射、权重、倍率和可用状态。',
    path: '/admin/channels',
    icon: Network,
  },
  {
    title: '账号池管理',
    desc: '管理运行账号、失效账号池和废弃账号池。',
    path: '/admin/channel-accounts',
    icon: Database,
  },
  {
    title: '渠道余额监控',
    desc: '查看渠道余额、账号余额和异常刷新状态。',
    path: '/admin/channel-balance-monitor',
    icon: WalletCards,
  },
  {
    title: '智能模型网关',
    desc: '查看路由、切换记录、用户请求和上游响应。',
    path: '/admin/model-gateway',
    icon: Activity,
  },
];

const progressItems = [
  {
    title: '后台路径闭环',
    status: '已完成',
    desc: '旧管理入口已跳转到 /admin/*，内部管理跳转已收口。',
    done: true,
  },
  {
    title: '经营总览数据接入',
    status: '已完成',
    desc: '总览已接入网关观测、健康检测、余额监控和盈利监控摘要。',
    done: true,
  },
  {
    title: '后台真实页面重构',
    status: '进行中',
    desc: '下一步重构渠道运营和模型路由页面的信息分区。',
    done: false,
  },
  {
    title: '独立部署演练',
    status: '待开始',
    desc: '使用 admin 独立入口在测试环境验证静态资源和登录落点。',
    done: false,
  },
];

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function getErrorMessage(error) {
  return (
    error?.response?.data?.message ||
    error?.response?.data?.error ||
    error?.message ||
    '请求失败'
  );
}

function numberOrZero(value) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

function hasPositiveNumber(value) {
  return Number.isFinite(Number(value)) && Number(value) > 0;
}

function formatNumber(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return new Intl.NumberFormat('zh-CN', {
    maximumFractionDigits: numeric >= 1000 ? 0 : 1,
  }).format(numeric);
}

function formatPercent(value, digits = 1) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `${(numeric * 100).toFixed(digits)}%`;
}

function formatUsd(value, digits = 4) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `$${numeric.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatLatency(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  if (numeric >= 1000) return `${(numeric / 1000).toFixed(2)}s`;
  return `${Math.round(numeric)}ms`;
}

function formatTimestamp(timestamp) {
  const numeric = Number(timestamp);
  return numeric > 0 ? timestamp2string(numeric) : '--';
}

function buildSourceDetail(key, data, t) {
  switch (key) {
    case 'observability': {
      const summary = data?.user_requests?.summary || data?.summary || {};
      return `${t('用户请求')} ${formatNumber(summary.user_requests || summary.total_requests)} · ${t(
        '最终失败',
      )} ${formatNumber(summary.final_failures)}`;
    }
    case 'healthQueue': {
      const summary = data?.summary || {};
      return `${t('待检测')} ${formatNumber(summary.pending_count)} · ${t(
        '运行键',
      )} ${formatNumber(summary.runtime_keys)}`;
    }
    case 'balance': {
      const summary = data?.summary || {};
      return `${t('低余额')} ${formatNumber(summary.low_balance_accounts)} · ${t(
        '耗尽账号',
      )} ${formatNumber(summary.empty_accounts)}`;
    }
    case 'profit': {
      const summary = data?.summary || {};
      return `${t('利润')} ${formatUsd(summary.profit_usd)} · ${t('毛利率')} ${formatPercent(
        summary.gross_margin,
      )}`;
    }
    default:
      return t('摘要已返回');
  }
}

function buildMetricCards(data, t) {
  const observabilitySummary = data.observability?.summary || {};
  const userRequestSummary = data.observability?.user_requests?.summary || {};
  const healthSummary = data.healthQueue?.summary || {};
  const balanceSummary = data.balance?.summary || {};
  const profitSummary = data.profit?.summary || {};
  const dynamicRatioSummary = data.profit?.dynamic_ratio_summary || {};

  const requests =
    userRequestSummary.user_requests ||
    profitSummary.requests ||
    observabilitySummary.dispatches ||
    observabilitySummary.attempts;
  const successRate = hasPositiveNumber(userRequestSummary.user_success_rate)
    ? userRequestSummary.user_success_rate
    : hasPositiveNumber(profitSummary.success_rate)
      ? profitSummary.success_rate
      : observabilitySummary.success_rate;
  const channelRiskCount =
    numberOrZero(healthSummary.pending_count) +
    numberOrZero(healthSummary.isolated_count) +
    numberOrZero(healthSummary.score_anomaly_count) +
    numberOrZero(balanceSummary.low_balance_accounts) +
    numberOrZero(balanceSummary.empty_accounts) +
    numberOrZero(balanceSummary.error_accounts);
  const revenueGap = numberOrZero(dynamicRatioSummary.revenue_gap_usd);

  return [
    {
      key: 'requests',
      label: '24h 用户请求',
      value: formatNumber(requests),
      helper: `${t('已扫描')} ${formatNumber(userRequestSummary.scanned_requests || observabilitySummary.scanned_records)} · ${t(
        '首包 P95',
      )} ${formatLatency(userRequestSummary.p95_ttft_ms)}`,
      tone: 'info',
      icon: Activity,
    },
    {
      key: 'success-rate',
      label: '最终成功率',
      value: formatPercent(successRate),
      helper: `${t('成功')} ${formatNumber(userRequestSummary.successes || profitSummary.success_requests)} · ${t(
        '最终失败',
      )} ${formatNumber(userRequestSummary.final_failures)}`,
      tone: numberOrZero(successRate) >= 0.98 ? 'success' : 'warning',
      icon: Gauge,
    },
    {
      key: 'channel-risk',
      label: '渠道风险',
      value: formatNumber(channelRiskCount),
      helper: `${t('待检测')} ${formatNumber(healthSummary.pending_count)} · ${t(
        '低余额',
      )} ${formatNumber(balanceSummary.low_balance_accounts)}`,
      tone: channelRiskCount > 0 ? 'warning' : 'success',
      icon: AlertTriangle,
    },
    {
      key: 'profit',
      label: '24h 利润',
      value: formatUsd(profitSummary.profit_usd),
      helper:
        revenueGap > 0
          ? `${t('收入缺口')} ${formatUsd(revenueGap)}`
          : `${t('收入')} ${formatUsd(profitSummary.revenue_usd)} · ${t('成本')} ${formatUsd(
              profitSummary.operating_cost_usd,
            )}`,
      tone: numberOrZero(profitSummary.profit_usd) < 0 ? 'danger' : 'money',
      icon: CircleDollarSign,
    },
  ];
}

function buildRiskItems(data, t) {
  const observabilitySummary = data.observability?.summary || {};
  const userRequestSummary = data.observability?.user_requests?.summary || {};
  const healthSummary = data.healthQueue?.summary || {};
  const balanceSummary = data.balance?.summary || {};
  const profitSummary = data.profit?.summary || {};
  const dynamicRatioSummary = data.profit?.dynamic_ratio_summary || {};

  const channelAlertCount =
    numberOrZero(healthSummary.pending_count) +
    numberOrZero(healthSummary.isolated_count) +
    numberOrZero(healthSummary.score_anomaly_count);
  const balanceAlertCount =
    numberOrZero(balanceSummary.low_balance_accounts) +
    numberOrZero(balanceSummary.empty_accounts) +
    numberOrZero(balanceSummary.error_accounts);
  const failoverRiskCount =
    numberOrZero(observabilitySummary.fallback_used) +
    numberOrZero(
      observabilitySummary.resource_protection_primary_failure_fallbacks,
    ) +
    numberOrZero(userRequestSummary.recovered);
  const settlementRiskCount =
    numberOrZero(dynamicRatioSummary.pending_manual_confirm_groups) +
    numberOrZero(dynamicRatioSummary.clamped_groups) +
    (numberOrZero(profitSummary.profit_usd) < 0 ? 1 : 0);

  return [
    {
      title: '渠道预警',
      desc: `${t('待检测')} ${formatNumber(healthSummary.pending_count)} · ${t(
        '隔离中',
      )} ${formatNumber(healthSummary.isolated_count)} · ${t('快速校准')} ${formatNumber(
        healthSummary.score_anomaly_count,
      )}`,
      status:
        channelAlertCount > 0
          ? `${formatNumber(channelAlertCount)} ${t('项待处理')}`
          : t('正常'),
      action: '查看预警',
      path: '/admin/channel-alerts',
      icon: AlertTriangle,
      tone: channelAlertCount > 0 ? 'warning' : 'success',
    },
    {
      title: '结算与利润异常',
      desc: `${t('待确认分组')} ${formatNumber(
        dynamicRatioSummary.pending_manual_confirm_groups,
      )} · ${t('倍率触顶')} ${formatNumber(dynamicRatioSummary.clamped_groups)} · ${t(
        '利润',
      )} ${formatUsd(profitSummary.profit_usd)}`,
      status:
        settlementRiskCount > 0
          ? `${formatNumber(settlementRiskCount)} ${t('项待处理')}`
          : t('正常'),
      action: '查看结算',
      path: '/admin/profit-monitor',
      icon: ShieldCheck,
      tone: settlementRiskCount > 0 ? 'danger' : 'success',
    },
    {
      title: '智能切换异常',
      desc: `${t('兜底切换')} ${formatNumber(observabilitySummary.fallback_used)} · ${t(
        '故障兜底',
      )} ${formatNumber(
        observabilitySummary.resource_protection_primary_failure_fallbacks,
      )} · ${t('已恢复')} ${formatNumber(userRequestSummary.recovered)}`,
      status:
        failoverRiskCount > 0
          ? `${formatNumber(failoverRiskCount)} ${t('条记录')}`
          : t('暂无异常'),
      action: '查看网关',
      path: '/admin/model-gateway',
      icon: HeartPulse,
      tone: failoverRiskCount > 0 ? 'info' : 'success',
    },
    {
      title: '余额预警',
      desc: `${t('低余额账号')} ${formatNumber(balanceSummary.low_balance_accounts)} · ${t(
        '耗尽账号',
      )} ${formatNumber(balanceSummary.empty_accounts)} · ${t('异常账号')} ${formatNumber(
        balanceSummary.error_accounts,
      )}`,
      status:
        balanceAlertCount > 0
          ? `${formatNumber(balanceAlertCount)} ${t('个账号')}`
          : t('正常'),
      action: '查看余额',
      path: '/admin/channel-balance-monitor',
      icon: WalletCards,
      tone: balanceAlertCount > 0 ? 'warning' : 'success',
    },
  ];
}

const AdminOverview = () => {
  const { t } = useTranslation();
  const [overviewState, setOverviewState] = useState({
    loading: true,
    refreshing: false,
    data: {},
    errors: {},
    updatedAt: 0,
  });

  const loadOverview = useCallback(async (silent = false) => {
    setOverviewState((current) => ({
      ...current,
      loading: !silent,
      refreshing: silent,
      errors: silent ? current.errors : {},
    }));

    const results = await Promise.allSettled(
      OVERVIEW_SOURCE_DEFINITIONS.map((source) => source.request()),
    );
    const nextData = {};
    const nextErrors = {};

    results.forEach((result, index) => {
      const source = OVERVIEW_SOURCE_DEFINITIONS[index];
      if (result.status === 'fulfilled') {
        nextData[source.key] = unwrapApiData(result.value);
        return;
      }
      nextErrors[source.key] = getErrorMessage(result.reason);
    });

    setOverviewState({
      loading: false,
      refreshing: false,
      data: nextData,
      errors: nextErrors,
      updatedAt: Math.floor(Date.now() / 1000),
    });
  }, []);

  useEffect(() => {
    loadOverview(false);
  }, [loadOverview]);

  const sourceStates = useMemo(
    () =>
      OVERVIEW_SOURCE_DEFINITIONS.map((source) => {
        const hasData = Object.prototype.hasOwnProperty.call(
          overviewState.data,
          source.key,
        );
        const error = overviewState.errors[source.key];
        return {
          ...source,
          ok: hasData && !error,
          error,
          detail: error
            ? error
            : hasData
              ? buildSourceDetail(source.key, overviewState.data[source.key], t)
              : t('等待返回'),
        };
      }),
    [overviewState.data, overviewState.errors, t],
  );
  const connectedSources = sourceStates.filter((source) => source.ok).length;
  const failedSources = sourceStates.filter((source) => source.error).length;
  const metricCards = useMemo(
    () => buildMetricCards(overviewState.data, t),
    [overviewState.data, t],
  );
  const riskItems = useMemo(
    () => buildRiskItems(overviewState.data, t),
    [overviewState.data, t],
  );
  const overviewStatus = failedSources
    ? {
        label: '数据源',
        value: `${connectedSources}/${sourceStates.length}`,
        helper: '部分接口失败',
        tone: 'warning',
      }
    : {
        label: '数据源',
        value: `${connectedSources}/${sourceStates.length}`,
        helper: connectedSources ? '摘要已更新' : '等待返回',
        tone: connectedSources ? 'success' : 'info',
      };

  return (
    <div className='aurora-overview'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('管理员运营中枢')}</div>
          <h1>{t('经营总览')}</h1>
          <p>
            {t(
              '总览已接入网关观测、健康检测、余额监控和盈利监控摘要，用于第一时间定位请求、渠道、结算和利润风险。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>{t('统计窗口')} 24h</span>
            <span>
              {t('最后刷新')} {formatTimestamp(overviewState.updatedAt)}
            </span>
          </div>
        </div>
        <div
          className={`aurora-overview-status aurora-status-${overviewStatus.tone}`}
        >
          <span>{t(overviewStatus.label)}</span>
          <strong>{overviewStatus.value}</strong>
          <em>{t(overviewStatus.helper)}</em>
          <button
            className='aurora-refresh-button'
            type='button'
            disabled={overviewState.loading || overviewState.refreshing}
            onClick={() => loadOverview(true)}
          >
            <RefreshCw
              size={15}
              className={overviewState.refreshing ? 'is-spinning' : ''}
            />
            {overviewState.refreshing ? t('刷新中') : t('刷新')}
          </button>
        </div>
      </section>

      <section className='aurora-metric-grid'>
        {metricCards.map((card) => {
          const Icon = card.icon;
          return (
            <article
              className={`aurora-metric-card aurora-tone-${card.tone}`}
              key={card.key}
            >
              <div className='aurora-metric-icon'>
                <Icon size={20} />
              </div>
              <div>
                <div className='aurora-metric-label'>{t(card.label)}</div>
                <div className='aurora-metric-value'>
                  {overviewState.loading ? t('加载中') : card.value}
                </div>
                <div className='aurora-metric-helper'>{card.helper}</div>
              </div>
            </article>
          );
        })}
      </section>

      <div className='aurora-overview-grid'>
        <section className='aurora-panel aurora-panel-main'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('实时风险')}</h2>
              <p>
                {t(
                  '优先处理影响请求成功率、扣费准确性和渠道连续可用性的事项。',
                )}
              </p>
            </div>
          </div>

          <div className='aurora-risk-list'>
            {riskItems.map((item) => {
              const Icon = item.icon;
              return (
                <Link
                  className={`aurora-risk-item aurora-tone-${item.tone}`}
                  key={item.title}
                  to={item.path}
                >
                  <span className='aurora-risk-icon'>
                    <Icon size={18} />
                  </span>
                  <span className='aurora-risk-body'>
                    <span className='aurora-risk-title'>
                      {t(item.title)}
                      <em>{item.status}</em>
                    </span>
                    <span className='aurora-risk-desc'>{item.desc}</span>
                  </span>
                  <span className='aurora-risk-action'>
                    {t(item.action)}
                    <ArrowRight size={15} />
                  </span>
                </Link>
              );
            })}
          </div>
        </section>

        <section className='aurora-panel'>
          <div className='aurora-section-head'>
            <div>
              <h2>{t('后台工作台')}</h2>
              <p>
                {t('把管理员高频入口集中在这里，减少从普通控制台来回跳转。')}
              </p>
            </div>
          </div>

          <div className='aurora-workspace-list'>
            {workspaceItems.map((item) => {
              const Icon = item.icon;
              return (
                <Link
                  className='aurora-workspace-item'
                  key={item.title}
                  to={item.path}
                >
                  <span className='aurora-workspace-icon'>
                    <Icon size={18} />
                  </span>
                  <span>
                    <strong>{t(item.title)}</strong>
                    <small>{t(item.desc)}</small>
                  </span>
                  <em>{t('进入')}</em>
                </Link>
              );
            })}
          </div>
        </section>
      </div>

      <section className='aurora-panel aurora-source-panel'>
        <div className='aurora-section-head'>
          <div>
            <h2>{t('数据源健康')}</h2>
            <p>
              {t('总览页按模块降级展示，单个接口失败不会阻塞整个后台首页。')}
            </p>
          </div>
        </div>
        <div className='aurora-source-grid'>
          {sourceStates.map((source) => (
            <article
              className={`aurora-source-item ${source.ok ? 'is-ok' : 'is-error'}`}
              key={source.key}
            >
              <span className='aurora-source-state'>
                {source.ok ? (
                  <CheckCircle2 size={16} />
                ) : (
                  <AlertCircle size={16} />
                )}
                {source.ok ? t('已接入') : t('需检查')}
              </span>
              <strong>{t(source.label)}</strong>
              <small>{source.detail}</small>
            </article>
          ))}
        </div>
      </section>

      <section className='aurora-panel aurora-progress-panel'>
        <div className='aurora-section-head'>
          <div>
            <h2>{t('迁移进度')}</h2>
          </div>
        </div>
        <div className='aurora-progress-list'>
          {progressItems.map((item) => (
            <article
              className={`aurora-progress-item ${item.done ? 'is-done' : ''}`}
              key={item.title}
            >
              <span className='aurora-progress-dot' />
              <div>
                <div className='aurora-progress-title'>
                  {t(item.title)}
                  <em>{t(item.status)}</em>
                </div>
                <p>{t(item.desc)}</p>
              </div>
            </article>
          ))}
        </div>
      </section>
    </div>
  );
};

export default AdminOverview;
