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

import React, { useMemo, useState } from 'react';
import { Button, SideSheet, Skeleton, Tag } from '@douyinfe/semi-ui';
import {
  PiArrowUpRightDuotone,
  PiCalendarCheckDuotone,
  PiCreditCardDuotone,
  PiGaugeDuotone,
  PiListMagnifyingGlassDuotone,
  PiPackageDuotone,
  PiWalletDuotone,
} from 'react-icons/pi';
import { renderQuota } from '../../helpers';
import { formatSubscriptionResetPeriod } from '../../helpers/subscriptionFormat';

const formatDate = (timestamp, t) => {
  if (!timestamp) return t('暂无');
  return new Date(timestamp * 1000).toLocaleDateString();
};

const getRemainingDays = (subscription) => {
  if (!subscription?.end_time) return 0;
  const remaining = subscription.end_time - Date.now() / 1000;
  return Math.max(0, Math.ceil(remaining / 86400));
};

const getSubscriptionUsage = (subscription) => {
  const total = Number(subscription?.amount_total || 0);
  const used = Math.max(0, Number(subscription?.amount_used || 0));
  const hasUnlimited = total <= 0;
  const boundedUsed = hasUnlimited ? used : Math.min(used, total);
  const remain = hasUnlimited ? 0 : Math.max(0, total - used);
  const percent = hasUnlimited
    ? 100
    : total > 0
      ? Math.min(100, Math.round((boundedUsed / total) * 100))
      : 0;

  return {
    total,
    used,
    boundedUsed,
    remain,
    percent,
    hasUnlimited,
    isDepleted: !hasUnlimited && total > 0 && used >= total,
  };
};

const getProgressTone = ({ hasActive, hasUnlimited, percent }) => {
  if (!hasActive) return 'idle';
  if (hasUnlimited) return 'complete';
  if (percent >= 90) return 'danger';
  if (percent >= 70) return 'warning';
  if (percent >= 40) return 'notice';
  return 'healthy';
};

const getSubscriptionStatus = (subscription, usage, t) => {
  const now = Date.now() / 1000;
  const status = subscription?.status || '';
  const isExpired = subscription?.end_time > 0 && subscription.end_time < now;

  if (status === 'cancelled') {
    return { label: t('已作废'), color: 'grey' };
  }
  if (isExpired || status === 'expired') {
    return { label: t('已过期'), color: 'grey' };
  }
  if (usage?.isDepleted) {
    return { label: t('已耗尽'), color: 'red' };
  }
  if (status === 'active') {
    return { label: t('生效'), color: 'green' };
  }
  return { label: status || t('未知'), color: 'grey' };
};

const getSourceLabel = (source, t) => {
  const sourceMap = {
    admin: t('管理'),
    order: t('订单'),
  };
  return sourceMap[source] || source || '-';
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
  const [detailVisible, setDetailVisible] = useState(false);

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
    const activeDetails = activeList
      .map((item, index) => {
        const subscription = item?.subscription || null;
        if (!subscription) return null;

        const plan = subscription.plan_id
          ? planMap.get(subscription.plan_id)
          : null;
        const usage = getSubscriptionUsage(subscription);
        const remainingDays = getRemainingDays(subscription);
        const statusMeta = getSubscriptionStatus(subscription, usage, t);
        const isNearExpiry =
          subscription?.end_time > 0 &&
          subscription.end_time > Date.now() / 1000 &&
          remainingDays <= 7;

        return {
          key:
            subscription.id ||
            `${subscription.plan_id || 'subscription'}-${index}`,
          subscription,
          plan,
          title:
            plan?.title ||
            (subscription.plan_id
              ? `${t('订阅')} #${subscription.plan_id}`
              : `${t('订阅')} #${subscription.id || index + 1}`),
          usage,
          remainingDays,
          statusMeta,
          isNearExpiry,
          order: index + 1,
        };
      })
      .filter(Boolean);

    const primaryDetail = activeDetails[0] || null;
    const title = primaryDetail
      ? activeDetails.length > 1
        ? `${t('生效订阅')} ${activeDetails.length}`
        : primaryDetail.title
      : t('未开通套餐');

    let total = 0;
    let used = 0;
    let hasUnlimited = false;

    activeDetails.forEach((item) => {
      if (item.usage.hasUnlimited) {
        hasUnlimited = true;
        used += item.usage.used;
        return;
      }

      total += item.usage.total;
      used += item.usage.boundedUsed;
    });

    const remain = Math.max(0, total - used);
    const percent =
      total > 0 && !hasUnlimited
        ? Math.min(100, Math.round((used / total) * 100))
        : hasUnlimited
          ? 100
          : 0;
    const hasActive = activeList.length > 0;
    const nearestEndTime = activeDetails.reduce((nearest, item) => {
      const endTime = Number(item.subscription?.end_time || 0);
      if (!endTime) return nearest;
      if (!nearest || endTime < nearest) return endTime;
      return nearest;
    }, 0);
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
      activeDetails,
      nearestEndTime,
      remainingDays: nearestEndTime
        ? getRemainingDays({ end_time: nearestEndTime })
        : getRemainingDays(primary),
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
      label: overview.activeCount > 1 ? t('最近到期') : t('有效期'),
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
    <>
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
              placeholder={
                <Skeleton.Title style={{ width: 220, height: 30 }} />
              }
            >
              <div className='ct-command-subscription-title-row'>
                <strong className='ct-command-tone-blue'>
                  {overview.title}
                </strong>
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
                  ? overview.activeCount > 1
                    ? `${t('最近到期')} ${formatDate(overview.nearestEndTime, t)}`
                    : `${t('有效期')} ${formatDate(
                        overview.primary?.start_time,
                        t,
                      )} - ${formatDate(overview.primary?.end_time, t)}`
                  : t('查看套餐')}
                {overview.historyCount > 0 && (
                  <span>
                    {t('全部订阅')} {overview.historyCount}
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

          <div className='ct-command-subscription-actions'>
            {overview.hasActive && (
              <Button
                type='tertiary'
                icon={<PiListMagnifyingGlassDuotone size={16} />}
                onClick={() => setDetailVisible(true)}
                className='ct-command-subscription-action ct-command-subscription-action-soft'
              >
                {t('查看明细')}
              </Button>
            )}
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

      <SideSheet
        visible={detailVisible}
        placement='right'
        width='min(760px, 100vw)'
        bodyStyle={{ padding: 0 }}
        onCancel={() => setDetailVisible(false)}
        title={
          <div className='ct-command-subscription-sheet-title'>
            <Tag color='cyan' shape='circle'>
              {overview.activeCount}
            </Tag>
            <span>{t('订阅明细')}</span>
          </div>
        }
      >
        <div className='ct-command-subscription-detail'>
          <div className='ct-command-subscription-detail-summary'>
            <div>
              <span>{t('总剩余')}</span>
              <strong>
                {overview.hasUnlimited
                  ? t('无限额度')
                  : renderQuota(overview.remain)}
              </strong>
            </div>
            <div>
              <span>{t('已用额度')}</span>
              <strong>{renderQuota(overview.used)}</strong>
            </div>
            <div>
              <span>{t('扣费策略')}</span>
              <strong>{overview.billingPreferenceLabel}</strong>
            </div>
          </div>

          {overview.activeDetails.length > 0 ? (
            <div className='ct-command-subscription-detail-list'>
              {overview.activeDetails.map((item) => (
                <article
                  key={item.key}
                  className='ct-command-subscription-detail-card'
                >
                  <div className='ct-command-subscription-detail-head'>
                    <div className='ct-command-subscription-detail-title'>
                      <strong>{item.title}</strong>
                      <span>ID #{item.subscription?.id || '-'}</span>
                    </div>
                    <div className='ct-command-subscription-detail-tags'>
                      <Tag color='blue' shape='circle'>
                        {t('顺位')} {item.order}
                      </Tag>
                      <Tag color={item.statusMeta.color} shape='circle'>
                        {item.statusMeta.label}
                      </Tag>
                      {item.isNearExpiry && (
                        <Tag color='orange' shape='circle'>
                          {t('即将到期')}
                        </Tag>
                      )}
                    </div>
                  </div>

                  <div className='ct-command-subscription-detail-quota'>
                    <span>{t('使用进度')}</span>
                    <strong>
                      {item.usage.hasUnlimited
                        ? `${renderQuota(item.usage.used)} / ${t('不限')}`
                        : `${renderQuota(item.usage.boundedUsed)} / ${renderQuota(
                            item.usage.total,
                          )}`}
                    </strong>
                  </div>
                  <div className='ct-command-subscription-detail-progress'>
                    <span style={{ width: `${item.usage.percent}%` }} />
                  </div>

                  <div className='ct-command-subscription-detail-fields'>
                    <div>
                      <span>{t('剩余')}</span>
                      <strong>
                        {item.usage.hasUnlimited
                          ? t('无限额度')
                          : renderQuota(item.usage.remain)}
                      </strong>
                    </div>
                    <div>
                      <span>{t('开始')}</span>
                      <strong>
                        {formatDate(item.subscription?.start_time, t)}
                      </strong>
                    </div>
                    <div>
                      <span>{t('结束')}</span>
                      <strong>
                        {formatDate(item.subscription?.end_time, t)}
                      </strong>
                    </div>
                    <div>
                      <span>{t('来源')}</span>
                      <strong>
                        {getSourceLabel(item.subscription?.source, t)}
                      </strong>
                    </div>
                    <div>
                      <span>{t('重置周期')}</span>
                      <strong>
                        {formatSubscriptionResetPeriod(item.plan, t)}
                      </strong>
                    </div>
                    <div>
                      <span>{t('下次重置')}</span>
                      <strong>
                        {item.subscription?.next_reset_time
                          ? formatDate(item.subscription.next_reset_time, t)
                          : t('不重置')}
                      </strong>
                    </div>
                  </div>
                </article>
              ))}
            </div>
          ) : (
            <div className='ct-command-subscription-detail-empty'>
              {t('暂无生效订阅')}
            </div>
          )}

          <div className='ct-command-subscription-detail-note'>
            {t(
              '多条订阅会按当前订阅扣费顺序依次抵扣，可在订阅中心查看历史记录。',
            )}
          </div>
        </div>
      </SideSheet>
    </>
  );
};

export default SubscriptionOverviewPanel;
