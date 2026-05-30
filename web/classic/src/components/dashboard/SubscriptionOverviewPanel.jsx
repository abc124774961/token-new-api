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

import React, { useMemo } from 'react';
import { Button, Skeleton, Tag } from '@douyinfe/semi-ui';
import {
  PiArrowUpRightDuotone,
  PiCalendarCheckDuotone,
  PiCreditCardDuotone,
  PiGaugeDuotone,
  PiPackageDuotone,
  PiWalletDuotone,
} from 'react-icons/pi';
import { renderQuota } from '../../helpers';

const formatDate = (timestamp, t) => {
  if (!timestamp) return t('暂无');
  return new Date(timestamp * 1000).toLocaleDateString();
};

const getRemainingDays = (subscription) => {
  if (!subscription?.end_time) return 0;
  const remaining = subscription.end_time - Date.now() / 1000;
  return Math.max(0, Math.ceil(remaining / 86400));
};

const getProgressTone = ({ hasActive, hasUnlimited, percent }) => {
  if (!hasActive) return 'idle';
  if (hasUnlimited) return 'complete';
  if (percent >= 90) return 'danger';
  if (percent >= 70) return 'warning';
  if (percent >= 40) return 'notice';
  return 'healthy';
};

const SubscriptionOverviewPanel = ({
  activeSubscriptions = [],
  allSubscriptions = [],
  plans = [],
  billingPreference,
  loading,
  navigate,
  t,
}) => {
  const overview = useMemo(() => {
    const activeList = activeSubscriptions || [];
    const historyList = allSubscriptions || [];
    const planMap = new Map();
    (plans || []).forEach((item) => {
      const plan = item?.plan;
      if (plan?.id) {
        planMap.set(plan.id, plan);
      }
    });

    const primary = activeList?.[0]?.subscription || null;
    const planTitle = primary?.plan_id
      ? planMap.get(primary.plan_id)?.title
      : '';
    const title = primary
      ? planTitle || `${t('订阅')} #${primary.id}`
      : t('未开通套餐');

    let total = 0;
    let used = 0;
    let hasUnlimited = false;

    activeList.forEach((item) => {
      const subscription = item?.subscription;
      const amountTotal = Number(subscription?.amount_total || 0);
      const amountUsed = Number(subscription?.amount_used || 0);

      if (amountTotal <= 0) {
        hasUnlimited = true;
        used += amountUsed;
        return;
      }

      total += amountTotal;
      used += Math.min(amountUsed, amountTotal);
    });

    const remain = Math.max(0, total - used);
    const percent =
      total > 0 && !hasUnlimited
        ? Math.min(100, Math.round((used / total) * 100))
        : hasUnlimited
          ? 100
          : 0;
    const hasActive = activeList.length > 0;
    const isSubscriptionPreference =
      billingPreference === 'subscription_first' ||
      billingPreference === 'subscription_only';
    const effectiveBillingPreference =
      !hasActive && isSubscriptionPreference
        ? 'wallet_first'
        : billingPreference ||
          (hasActive ? 'subscription_first' : 'wallet_first');
    const billingPreferenceLabelMap = {
      subscription_first: t('订阅优先'),
      wallet_first: t('钱包优先'),
      subscription_only: t('仅订阅'),
      wallet_only: t('仅钱包'),
    };

    return {
      hasActive,
      title,
      primary,
      percent,
      progressTone: getProgressTone({ hasActive, hasUnlimited, percent }),
      remain,
      total,
      used,
      hasUnlimited,
      activeCount: activeList.length,
      historyCount: historyList.length,
      remainingDays: getRemainingDays(primary),
      billingPreferenceLabel:
        billingPreferenceLabelMap[effectiveBillingPreference] ||
        effectiveBillingPreference,
    };
  }, [activeSubscriptions, allSubscriptions, plans, billingPreference, t]);

  const metricItems = [
    {
      icon: <PiWalletDuotone size={18} />,
      label: t('剩余额度'),
      value: !overview.hasActive
        ? t('暂无')
        : overview.hasUnlimited
          ? t('无限额度')
          : renderQuota(overview.remain),
      tone: 'green',
    },
    {
      icon: <PiGaugeDuotone size={18} />,
      label: t('已用额度'),
      value: overview.hasActive ? renderQuota(overview.used) : t('暂无'),
      tone: 'cyan',
    },
    {
      icon: <PiCalendarCheckDuotone size={18} />,
      label: t('有效期'),
      value: overview.hasActive
        ? `${overview.remainingDays} ${t('天')}`
        : t('暂无'),
      tone: 'amber',
    },
    {
      icon: <PiCreditCardDuotone size={18} />,
      label: t('扣费策略'),
      value: overview.billingPreferenceLabel,
      tone: 'violet',
    },
  ];

  return (
    <section className='ct-command-subscription-panel'>
      <div className='ct-command-subscription-main'>
        <div className='ct-command-panel-title-group'>
          <span className='ct-command-panel-icon ct-command-panel-icon-subscription'>
            <PiPackageDuotone size={20} />
          </span>
          <div>
            <div className='ct-command-panel-kicker'>{t('个人套餐')}</div>
            <h3 className='ct-command-panel-title'>{t('当前套餐')}</h3>
          </div>
        </div>

        <div className='ct-command-subscription-plan'>
          <Skeleton
            loading={loading}
            active
            placeholder={<Skeleton.Title style={{ width: 220, height: 30 }} />}
          >
            <div className='ct-command-subscription-title-row'>
              <strong className='ct-command-tone-blue'>{overview.title}</strong>
              <Tag
                color={overview.hasActive ? 'green' : 'grey'}
                shape='circle'
                size='large'
                className='ct-command-subscription-status'
              >
                {overview.hasActive ? t('订阅生效中') : t('未开通套餐')}
              </Tag>
              {overview.activeCount > 1 && (
                <Tag
                  color='cyan'
                  shape='circle'
                  size='large'
                  className='ct-command-subscription-status'
                >
                  +{overview.activeCount - 1}
                </Tag>
              )}
            </div>
          </Skeleton>
          <Skeleton
            loading={loading}
            active
            placeholder={
              <Skeleton.Paragraph
                rows={1}
                style={{ width: 360, marginTop: 8 }}
              />
            }
          >
            <div className='ct-command-subscription-meta'>
              {overview.hasActive
                ? `${t('有效期')} ${formatDate(
                    overview.primary?.start_time,
                    t,
                  )} - ${formatDate(overview.primary?.end_time, t)}`
                : t('查看套餐')}
              {overview.historyCount > 0 && (
                <span>
                  {t('订阅')} {overview.historyCount}
                </span>
              )}
            </div>
          </Skeleton>

          <div
            className={`ct-command-subscription-progress ct-command-subscription-progress-${overview.progressTone}`}
          >
            <span
              className={`ct-command-subscription-progress-bar ct-command-subscription-progress-bar-${overview.progressTone}`}
              style={{ width: `${overview.percent}%` }}
            />
          </div>
        </div>

        <Button
          type='tertiary'
          icon={<PiArrowUpRightDuotone size={16} />}
          iconPosition='right'
          onClick={() => navigate?.('/console/subscription-plans')}
          className='ct-command-subscription-action'
        >
          {overview.hasActive ? t('订阅中心') : t('查看套餐')}
        </Button>
      </div>

      <div className='ct-command-subscription-metrics'>
        {metricItems.map((item) => (
          <div
            key={item.label}
            className={`ct-command-subscription-metric ct-command-subscription-metric-${item.tone}`}
          >
            <span className='ct-command-subscription-metric-icon'>
              {item.icon}
            </span>
            <span className='ct-command-subscription-metric-copy'>
              <span>{item.label}</span>
              <Skeleton
                loading={loading}
                active
                placeholder={
                  <Skeleton.Title style={{ width: 76, height: 20 }} />
                }
              >
                <strong className={`ct-command-tone-${item.tone}`}>
                  {item.value}
                </strong>
              </Skeleton>
            </span>
          </div>
        ))}
      </div>
    </section>
  );
};

export default SubscriptionOverviewPanel;
