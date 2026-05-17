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
import {
  Badge,
  Button,
  Card,
  Skeleton,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess, renderQuota } from '../../helpers';
import { getCurrencyConfig } from '../../helpers/render';
import {
  ArrowRight,
  CheckCircle2,
  Clock3,
  Gauge,
  History,
  PackageCheck,
  RefreshCw,
  ShieldCheck,
  ShoppingCart,
  Wallet,
  WalletCards,
} from 'lucide-react';
import SubscriptionPurchaseModal from './modals/SubscriptionPurchaseModal';
import {
  formatSubscriptionDuration,
  formatSubscriptionResetPeriod,
} from '../../helpers/subscriptionFormat';

const { Text } = Typography;

function getEpayMethods(payMethods = []) {
  return (payMethods || []).filter(
    (m) => m?.type && m.type !== 'stripe' && m.type !== 'creem',
  );
}

function submitEpayForm({ url, params }) {
  const form = document.createElement('form');
  form.action = url;
  form.method = 'POST';
  const isSafari =
    navigator.userAgent.indexOf('Safari') > -1 &&
    navigator.userAgent.indexOf('Chrome') < 1;
  if (!isSafari) form.target = '_blank';
  Object.keys(params || {}).forEach((key) => {
    const input = document.createElement('input');
    input.type = 'hidden';
    input.name = key;
    input.value = params[key];
    form.appendChild(input);
  });
  document.body.appendChild(form);
  form.submit();
  document.body.removeChild(form);
}

const SubscriptionMetric = ({ icon: Icon, label, value, tone }) => (
  <div className={`ct-topup-stat-tile ct-topup-stat-tile-${tone}`}>
    <div className='ct-topup-stat-copy'>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
    <div className='ct-topup-stat-icon'>
      <Icon size={16} />
    </div>
  </div>
);

const SubscriptionPlansCard = ({
  t,
  loading = false,
  plans = [],
  payMethods = [],
  enableOnlineTopUp = false,
  enableStripeTopUp = false,
  enableCreemTopUp = false,
  billingPreference,
  onChangeBillingPreference,
  activeSubscriptions = [],
  allSubscriptions = [],
  reloadSubscriptionSelf,
}) => {
  const [open, setOpen] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [paying, setPaying] = useState(false);
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('');
  const [refreshing, setRefreshing] = useState(false);

  const epayMethods = useMemo(() => getEpayMethods(payMethods), [payMethods]);

  const openBuy = (p) => {
    setSelectedPlan(p);
    setSelectedEpayMethod(epayMethods?.[0]?.type || '');
    setOpen(true);
  };

  const closeBuy = () => {
    setOpen(false);
    setSelectedPlan(null);
    setPaying(false);
  };

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      await reloadSubscriptionSelf?.();
    } finally {
      setRefreshing(false);
    }
  };

  const payStripe = async () => {
    if (!selectedPlan?.plan?.stripe_price_id) {
      showError(t('该套餐未配置 Stripe'));
      return;
    }
    setPaying(true);
    try {
      const res = await API.post('/api/subscription/stripe/pay', {
        plan_id: selectedPlan.plan.id,
      });
      if (res.data?.message === 'success') {
        window.open(res.data.data?.pay_link, '_blank');
        showSuccess(t('已打开支付页面'));
        closeBuy();
      } else {
        const errorMsg =
          typeof res.data?.data === 'string'
            ? res.data.data
            : res.data?.message || t('支付失败');
        showError(errorMsg);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaying(false);
    }
  };

  const payCreem = async () => {
    if (!selectedPlan?.plan?.creem_product_id) {
      showError(t('该套餐未配置 Creem'));
      return;
    }
    setPaying(true);
    try {
      const res = await API.post('/api/subscription/creem/pay', {
        plan_id: selectedPlan.plan.id,
      });
      if (res.data?.message === 'success') {
        window.open(res.data.data?.checkout_url, '_blank');
        showSuccess(t('已打开支付页面'));
        closeBuy();
      } else {
        const errorMsg =
          typeof res.data?.data === 'string'
            ? res.data.data
            : res.data?.message || t('支付失败');
        showError(errorMsg);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaying(false);
    }
  };

  const payEpay = async () => {
    if (!selectedEpayMethod) {
      showError(t('请选择支付方式'));
      return;
    }
    setPaying(true);
    try {
      const res = await API.post('/api/subscription/epay/pay', {
        plan_id: selectedPlan.plan.id,
        payment_method: selectedEpayMethod,
      });
      if (res.data?.message === 'success') {
        submitEpayForm({ url: res.data.url, params: res.data.data });
        showSuccess(t('已发起支付'));
        closeBuy();
      } else {
        const errorMsg =
          typeof res.data?.data === 'string'
            ? res.data.data
            : res.data?.message || t('支付失败');
        showError(errorMsg);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaying(false);
    }
  };

  const hasActiveSubscription = activeSubscriptions.length > 0;
  const hasAnySubscription = allSubscriptions.length > 0;
  const primaryActiveSubscription = hasActiveSubscription
    ? activeSubscriptions[0]
    : null;
  const disableSubscriptionPreference = !hasActiveSubscription;
  const isSubscriptionPreference =
    billingPreference === 'subscription_first' ||
    billingPreference === 'subscription_only';
  const displayBillingPreference =
    disableSubscriptionPreference && isSubscriptionPreference
      ? 'wallet_first'
      : billingPreference;
  const billingPreferenceLabelMap = {
    subscription_first: t('优先订阅'),
    wallet_first: t('优先钱包'),
    subscription_only: t('仅用订阅'),
    wallet_only: t('仅用钱包'),
  };
  const displayBillingPreferenceLabel =
    billingPreferenceLabelMap[displayBillingPreference] ||
    displayBillingPreference;

  const planPurchaseCountMap = useMemo(() => {
    const map = new Map();
    (allSubscriptions || []).forEach((sub) => {
      const planId = sub?.subscription?.plan_id;
      if (!planId) return;
      map.set(planId, (map.get(planId) || 0) + 1);
    });
    return map;
  }, [allSubscriptions]);

  const planTitleMap = useMemo(() => {
    const map = new Map();
    (plans || []).forEach((p) => {
      const plan = p?.plan;
      if (!plan?.id) return;
      map.set(plan.id, plan.title || '');
    });
    return map;
  }, [plans]);

  const getPlanPurchaseCount = (planId) =>
    planPurchaseCountMap.get(planId) || 0;

  const getSubscriptionTitle = (sub) => {
    const subscription = sub?.subscription;
    const planTitle = planTitleMap.get(subscription?.plan_id) || '';
    if (planTitle) return planTitle;
    return subscription?.id
      ? `${t('订阅')} #${subscription.id}`
      : t('订阅套餐');
  };

  const formatTime = (timestamp) => {
    if (!timestamp) return t('暂无');
    return new Date(timestamp * 1000).toLocaleString();
  };

  const getRemainingDays = (sub) => {
    if (!sub?.subscription?.end_time) return 0;
    const now = Date.now() / 1000;
    const remaining = sub.subscription.end_time - now;
    return Math.max(0, Math.ceil(remaining / 86400));
  };

  const activeQuotaStats = useMemo(() => {
    let total = 0;
    let used = 0;
    let hasUnlimited = false;

    (activeSubscriptions || []).forEach((sub) => {
      const subscription = sub?.subscription;
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
        : 0;

    return { total, used, remain, percent, hasUnlimited };
  }, [activeSubscriptions]);

  const subscription = primaryActiveSubscription?.subscription;
  const currentPlanId = subscription?.plan_id;
  const currentTitle = hasActiveSubscription
    ? getSubscriptionTitle(primaryActiveSubscription)
    : t('未开通套餐');
  const remainDays = primaryActiveSubscription
    ? getRemainingDays(primaryActiveSubscription)
    : 0;
  const progressWidth = activeQuotaStats.hasUnlimited
    ? '100%'
    : `${activeQuotaStats.percent}%`;

  const renderLoading = () => (
    <div className='ct-topup-subscription-page'>
      <Card className='ct-topup-panel'>
        <Skeleton.Title style={{ width: 220, height: 28 }} />
        <Skeleton.Paragraph rows={4} />
      </Card>
      <div className='ct-topup-plan-grid ct-topup-plan-grid-three'>
        {[1, 2, 3].map((i) => (
          <Card key={i} className='ct-topup-plan-card' bodyStyle={{ padding: 18 }}>
            <Skeleton.Title style={{ width: '65%', height: 24 }} />
            <Skeleton.Paragraph rows={4} />
            <Skeleton.Button style={{ marginTop: 16, width: '100%' }} />
          </Card>
        ))}
      </div>
    </div>
  );

  const renderPlanCard = (p, index) => {
    const plan = p?.plan;
    const totalAmount = Number(plan?.total_amount || 0);
    const { symbol, rate } = getCurrencyConfig();
    const price = Number(plan?.price_amount || 0);
    const convertedPrice = price * rate;
    const displayPrice = convertedPrice.toFixed(
      Number.isInteger(convertedPrice) ? 0 : 2,
    );
    const isCurrent = currentPlanId && plan?.id === currentPlanId;
    const isRecommended = isCurrent || (index === 1 && plans.length > 1);
    const limit = Number(plan?.max_purchase_per_user || 0);
    const limitLabel = limit > 0 ? `${t('限购')} ${limit}` : null;
    const totalLabel =
      totalAmount > 0
        ? `${renderQuota(totalAmount)} / ${t('周期')}`
        : t('不限额度');
    const resetLabel =
      formatSubscriptionResetPeriod(plan, t) === t('不重置')
        ? null
        : formatSubscriptionResetPeriod(plan, t);
    const planBenefits = [
      `${t('套餐额度')} ${totalLabel}`,
      `${t('有效期')} ${formatSubscriptionDuration(plan, t)}`,
      resetLabel ? `${t('额度重置')} ${resetLabel}` : null,
      plan?.upgrade_group ? `${t('升级分组')} ${plan.upgrade_group}` : null,
      limitLabel,
    ].filter(Boolean);
    const count = getPlanPurchaseCount(plan?.id);
    const reached = limit > 0 && count >= limit;

    return (
      <Card
        key={plan?.id || index}
        className={`ct-topup-plan-card ${isCurrent ? 'ct-topup-plan-card-current' : ''}`}
        bodyStyle={{ padding: 0 }}
      >
        <div className='ct-topup-plan-card-inner'>
          <div className='ct-topup-plan-head'>
            <div>
              <Typography.Title heading={4} className='ct-topup-plan-title'>
                {plan?.title || t('订阅套餐')}
              </Typography.Title>
              {plan?.subtitle && (
                <Text type='tertiary' className='ct-topup-plan-subtitle'>
                  {plan.subtitle}
                </Text>
              )}
            </div>
            {isRecommended && (
              <Tag color={isCurrent ? 'green' : 'amber'} shape='circle'>
                {isCurrent ? t('当前套餐') : t('推荐')}
              </Tag>
            )}
          </div>

          <div className='ct-topup-plan-price'>
            <span>{symbol}</span>
            <strong>{displayPrice}</strong>
            <em>/{t('月')}</em>
          </div>

          <div className='ct-topup-plan-quota'>
            <Gauge size={15} />
            <span>{t('额度')}</span>
            <strong>{totalLabel}</strong>
          </div>

          <div className='ct-topup-plan-benefits'>
            {planBenefits.map((benefit) => (
              <div key={benefit} className='ct-topup-plan-benefit-line'>
                <CheckCircle2 size={14} />
                <span>{benefit}</span>
              </div>
            ))}
          </div>

          <Tooltip
            content={
              reached ? `${t('已达到购买上限')} (${count}/${limit})` : ''
            }
            position='top'
          >
            <Button
              theme={isCurrent ? 'solid' : 'outline'}
              type='primary'
              block
              disabled={reached}
              onClick={() => {
                if (!reached) openBuy(p);
              }}
              className={isCurrent ? 'ct-topup-primary-button' : ''}
            >
              {isCurrent ? t('当前套餐') : reached ? t('已达上限') : t('立即订阅')}
            </Button>
          </Tooltip>
        </div>
      </Card>
    );
  };

  const renderEmptyPlanPanel = () => (
    <Card className='ct-topup-panel ct-topup-empty-plan-card'>
      <div className='ct-topup-empty-plan-layout'>
        <div className='ct-topup-empty-plan-icon'>
          <PackageCheck size={26} />
        </div>
        <div className='ct-topup-empty-plan-copy'>
          <strong>{t('暂无可购买套餐')}</strong>
          <span>{t('请联系管理员配置套餐')}</span>
        </div>
        <div className='ct-topup-empty-plan-preview'>
          {[100, 200, 500].map((quota, index) => (
            <div key={quota} className='ct-topup-empty-plan-preview-card'>
              <span>{quota}</span>
              <strong>{t('套餐')}</strong>
              <em>{index === 1 ? t('推荐') : t('待配置')}</em>
            </div>
          ))}
        </div>
      </div>
    </Card>
  );

  return (
    <>
      {loading ? (
        renderLoading()
      ) : (
        <div className='ct-topup-subscription-page'>
          <div className='ct-topup-page-head'>
            <div>
              <div className='ct-topup-page-title-row'>
                <Typography.Title heading={2} className='ct-topup-page-title'>
                  {t('套餐订阅')}
                </Typography.Title>
                <Tag
                  color='green'
                  shape='circle'
                  prefixIcon={<ShieldCheck size={12} />}
                >
                  {t('订阅额度优先')}
                </Tag>
              </div>
              <Text type='tertiary' className='ct-topup-page-subtitle'>
                {t('订阅用户优先扣订阅额度，不足时再由钱包补扣')}
              </Text>
            </div>
            <div className='ct-topup-page-actions'>
              <div className='ct-topup-pref-switch'>
                <Button
                  type={
                    displayBillingPreference === 'subscription_first'
                      ? 'primary'
                      : 'tertiary'
                  }
                  theme={
                    displayBillingPreference === 'subscription_first'
                      ? 'solid'
                      : 'light'
                  }
                  disabled={disableSubscriptionPreference}
                  onClick={() => onChangeBillingPreference?.('subscription_first')}
                >
                  {t('订阅优先')}
                </Button>
                <Button
                  type={
                    displayBillingPreference === 'wallet_first'
                      ? 'primary'
                      : 'tertiary'
                  }
                  theme={
                    displayBillingPreference === 'wallet_first'
                      ? 'solid'
                      : 'light'
                  }
                  onClick={() => onChangeBillingPreference?.('wallet_first')}
                >
                  {t('钱包优先')}
                </Button>
              </div>
              <Button
                icon={
                  <RefreshCw
                    size={15}
                    className={refreshing ? 'animate-spin' : ''}
                  />
                }
                theme='light'
                type='tertiary'
                onClick={handleRefresh}
                loading={refreshing}
                className='ct-topup-panel-action'
              >
                {t('刷新')}
              </Button>
            </div>
          </div>

          {disableSubscriptionPreference && isSubscriptionPreference && (
            <Text type='tertiary' size='small'>
              {t('已保存偏好为')}
              {billingPreference === 'subscription_only'
                ? t('仅用订阅')
                : t('优先订阅')}
              {t('，当前无生效订阅，将自动使用钱包')}
            </Text>
          )}

          <Card className='ct-topup-panel ct-topup-current-band'>
            <div className='ct-topup-current-band-main'>
              <div>
                <span>{t('当前套餐')}</span>
                <strong>{currentTitle}</strong>
                <em>
                  {hasActiveSubscription
                    ? `${t('有效期')} ${formatTime(subscription?.start_time)} - ${formatTime(subscription?.end_time)}`
                    : t('购买套餐后即可享受模型权益')}
                </em>
              </div>
              <div className='ct-topup-current-days'>
                <span>{t('剩余')}</span>
                <strong>{remainDays}</strong>
                <span>{t('天')}</span>
              </div>
            </div>
            <div className='ct-topup-current-band-progress'>
              <div className='ct-topup-current-progress-head'>
                <span>
                  {activeQuotaStats.hasUnlimited
                    ? t('不限额度')
                    : `${renderQuota(activeQuotaStats.used)} / ${renderQuota(activeQuotaStats.total)}`}
                </span>
                <strong>
                  {activeQuotaStats.hasUnlimited
                    ? t('不限')
                    : `${activeQuotaStats.percent}%`}
                </strong>
              </div>
              <div className='ct-topup-current-progress-track'>
                <span style={{ width: progressWidth }} />
              </div>
              <em>
                {activeQuotaStats.hasUnlimited
                  ? t('当前套餐额度不限')
                  : `${t('剩余')} ${renderQuota(activeQuotaStats.remain)}`}
              </em>
            </div>
            <div className='ct-topup-current-band-rule'>
              <ShieldCheck size={17} />
              <div>
                <strong>{t('订阅额度优先扣费')}</strong>
                <span>{t('产生费用时优先使用订阅额度，不足自动从钱包补扣')}</span>
              </div>
            </div>
          </Card>

          <div className='ct-topup-balance-strip ct-topup-balance-strip-wide'>
            <SubscriptionMetric
              icon={Wallet}
              label={t('剩余额度')}
              value={
                activeQuotaStats.hasUnlimited
                  ? t('不限')
                  : renderQuota(activeQuotaStats.remain)
              }
              tone='teal'
            />
            <SubscriptionMetric
              icon={Gauge}
              label={t('已用额度')}
              value={renderQuota(activeQuotaStats.used)}
              tone='blue'
            />
            <SubscriptionMetric
              icon={PackageCheck}
              label={t('总额度')}
              value={
                activeQuotaStats.hasUnlimited
                  ? t('不限')
                  : renderQuota(activeQuotaStats.total)
              }
              tone='green'
            />
            <SubscriptionMetric
              icon={WalletCards}
              label={t('扣费偏好')}
              value={displayBillingPreferenceLabel}
              tone='blue'
            />
          </div>

          <div className='ct-topup-subscription-main-grid'>
            <div className='ct-topup-plan-grid ct-topup-plan-grid-three'>
              {plans.length > 0 ? (
                plans.map(renderPlanCard)
              ) : (
                renderEmptyPlanPanel()
              )}
            </div>

            <div className='ct-topup-subscription-side'>
              <Card className='ct-topup-panel ct-topup-subscription-timeline'>
                <div className='ct-topup-section-heading'>
                  <div>
                    <strong>{t('订阅记录')}</strong>
                    <span>{t('展示最近的套餐续订与变更')}</span>
                  </div>
                  <Tag shape='circle'>
                    {allSubscriptions.length} {t('条')}
                  </Tag>
                </div>
                <div className='ct-topup-timeline-list'>
                  {hasAnySubscription ? (
                    allSubscriptions.slice(0, 5).map((sub, index) => {
                      const item = sub?.subscription;
                      const isActive =
                        item?.status === 'active' &&
                        (!item?.end_time || item.end_time > Date.now() / 1000);
                      return (
                        <div className='ct-topup-timeline-item' key={item?.id || index}>
                          <span>
                            {isActive ? (
                              <CheckCircle2 size={14} />
                            ) : (
                              <Clock3 size={14} />
                            )}
                          </span>
                          <div>
                            <strong>{getSubscriptionTitle(sub)}</strong>
                            <em>{formatTime(item?.start_time)}</em>
                          </div>
                          <Tag
                            color={isActive ? 'green' : 'grey'}
                            shape='circle'
                            size='small'
                          >
                            {isActive ? t('生效') : t('历史')}
                          </Tag>
                        </div>
                      );
                    })
                  ) : (
                    <div className='ct-topup-empty-row'>
                      <History size={18} />
                      <span>{t('暂无订阅记录')}</span>
                    </div>
                  )}
                </div>
              </Card>

              <Card className='ct-topup-panel ct-topup-billing-rule-card'>
                <div className='ct-topup-section-heading'>
                  <div>
                    <strong>{t('扣费规则')}</strong>
                    <span>{t('订阅优先时自动选择最合适的资金来源')}</span>
                  </div>
                </div>
                <div className='ct-topup-billing-flow'>
                  <div>
                    <ShoppingCart size={18} />
                    <span>{t('产生费用')}</span>
                  </div>
                  <ArrowRight size={16} />
                  <div>
                    <ShieldCheck size={18} />
                    <span>{t('订阅额度优先')}</span>
                  </div>
                  <ArrowRight size={16} />
                  <div>
                    <Wallet size={18} />
                    <span>{t('不足钱包补扣')}</span>
                  </div>
                </div>
                <div className='ct-topup-rule-list'>
                  <div>
                    <CheckCircle2 size={15} />
                    <span>{t('订阅额度足够时不会扣减钱包余额')}</span>
                  </div>
                  <div>
                    <CheckCircle2 size={15} />
                    <span>{t('订阅额度不足时仅补扣差额')}</span>
                  </div>
                  <div>
                    <CheckCircle2 size={15} />
                    <span>{t('可在页面右上角切换扣费偏好')}</span>
                  </div>
                </div>
              </Card>
            </div>
          </div>
        </div>
      )}

      <SubscriptionPurchaseModal
        t={t}
        visible={open}
        onCancel={closeBuy}
        selectedPlan={selectedPlan}
        paying={paying}
        selectedEpayMethod={selectedEpayMethod}
        setSelectedEpayMethod={setSelectedEpayMethod}
        epayMethods={epayMethods}
        enableOnlineTopUp={enableOnlineTopUp}
        enableStripeTopUp={enableStripeTopUp}
        enableCreemTopUp={enableCreemTopUp}
        purchaseLimitInfo={
          selectedPlan?.plan?.id
            ? {
                limit: Number(selectedPlan?.plan?.max_purchase_per_user || 0),
                count: getPlanPurchaseCount(selectedPlan?.plan?.id),
              }
            : null
        }
        onPayStripe={payStripe}
        onPayCreem={payCreem}
        onPayEpay={payEpay}
      />
    </>
  );
};

export default SubscriptionPlansCard;
