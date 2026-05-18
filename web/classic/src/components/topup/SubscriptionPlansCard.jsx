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
  Database,
  Gauge,
  History,
  Layers3,
  PackageCheck,
  PieChart,
  ShieldCheck,
  ShoppingCart,
  Wallet,
} from 'lucide-react';
import SubscriptionPurchaseModal from './modals/SubscriptionPurchaseModal';
import {
  formatSubscriptionDuration,
  formatSubscriptionResetPeriod,
} from '../../helpers/subscriptionFormat';

const { Text } = Typography;

const SubscriptionStat = ({ icon: Icon, label, value, hint, tone }) => (
  <div className={`ct-topup-stat-tile ct-topup-stat-tile-${tone}`}>
    <div className='ct-topup-stat-icon'>
      <Icon size={17} />
    </div>
    <div className='ct-topup-stat-copy'>
      <span>{label}</span>
      <strong>
        {value}
        {hint && <em>{hint}</em>}
      </strong>
    </div>
  </div>
);

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

const SubscriptionPlansCard = ({
  t,
  loading = false,
  plans = [],
  payMethods = [],
  enableOnlineTopUp = false,
  enableStripeTopUp = false,
  enableCreemTopUp = false,
  billingPreference,
  activeSubscriptions = [],
  allSubscriptions = [],
}) => {
  const [open, setOpen] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [paying, setPaying] = useState(false);
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('');

  const epayMethods = useMemo(() => getEpayMethods(payMethods), [payMethods]);
  const currency = getCurrencyConfig();

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
  const effectiveBillingPreference =
    displayBillingPreference ||
    (hasActiveSubscription ? 'subscription_first' : 'wallet_first');
  const billingPreferenceLabelMap = {
    subscription_first: t('订阅优先'),
    wallet_first: t('钱包优先'),
    subscription_only: t('仅订阅'),
    wallet_only: t('仅钱包'),
  };
  const displayBillingPreferenceLabel =
    billingPreferenceLabelMap[effectiveBillingPreference] ||
    effectiveBillingPreference;

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

  const planMetaMap = useMemo(() => {
    const map = new Map();
    (plans || []).forEach((p) => {
      const plan = p?.plan;
      if (!plan?.id) return;
      map.set(plan.id, plan);
    });
    return map;
  }, [plans]);

  const getPlanPurchaseCount = (planId) =>
    planPurchaseCountMap.get(planId) || 0;

  const renderQuotaLabel = (value, unlimited = false) =>
    unlimited ? t('不限') : renderQuota(value);

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

  const formatDate = (timestamp) => {
    if (!timestamp) return t('暂无');
    return new Date(timestamp * 1000).toLocaleDateString();
  };

  const formatCurrencyAmount = (amount) => {
    const converted = Number(amount || 0) * currency.rate;
    return `${currency.symbol}${converted.toFixed(
      Number.isInteger(converted) ? 0 : 2,
    )}`;
  };

  const getPlanCycleLabel = (plan) => {
    const resetPeriod = formatSubscriptionResetPeriod(plan, t);
    if (resetPeriod && resetPeriod !== t('不重置')) return resetPeriod;
    return formatSubscriptionDuration(plan, t);
  };

  const getSubscriptionPriceLabel = (sub) => {
    const plan = planMetaMap.get(sub?.subscription?.plan_id);
    if (!plan) return '';
    return formatCurrencyAmount(plan.price_amount || 0);
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
  const currentTag =
    hasActiveSubscription && currentTitle !== t('未开通套餐')
      ? t('生效')
      : null;
  const remainDays = primaryActiveSubscription
    ? getRemainingDays(primaryActiveSubscription)
    : 0;
  const progressWidth = activeQuotaStats.hasUnlimited
    ? '100%'
    : `${activeQuotaStats.percent}%`;
  const totalPlanQuotaLabel = renderQuotaLabel(
    activeQuotaStats.total,
    activeQuotaStats.hasUnlimited,
  );
  const remainQuotaLabel = renderQuotaLabel(
    activeQuotaStats.remain,
    activeQuotaStats.hasUnlimited,
  );
  const usedQuotaLabel = renderQuota(activeQuotaStats.used);
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
    const price = Number(plan?.price_amount || 0);
    const convertedPrice = price * currency.rate;
    const displayPrice = convertedPrice.toFixed(
      Number.isInteger(convertedPrice) ? 0 : 2,
    );
    const isCurrent = currentPlanId && plan?.id === currentPlanId;
    const isRecommended = isCurrent || (index === 1 && plans.length > 1);
    const limit = Number(plan?.max_purchase_per_user || 0);
    const limitLabel = limit > 0 ? `${t('限购')} ${limit}` : null;
    const totalLabel =
      totalAmount > 0
        ? `${renderQuota(totalAmount)} / ${getPlanCycleLabel(plan)}`
        : t('不限额度');
    const resetLabel =
      formatSubscriptionResetPeriod(plan, t) === t('不重置')
        ? null
        : formatSubscriptionResetPeriod(plan, t);
    const planBenefits = [
      t('API 全模型调用'),
      `${t('有效期')} ${formatSubscriptionDuration(plan, t)}`,
      resetLabel ? `${t('额度重置')} ${resetLabel}` : null,
      limitLabel,
      resetLabel
        ? t('支持周期性重置套餐权益额度')
        : t('购买套餐后即可享受模型权益'),
    ].filter(Boolean);
    const count = getPlanPurchaseCount(plan?.id);
    const reached = limit > 0 && count >= limit;
    const targetUsers = plan?.upgrade_group || t('API 全模型调用');

    return (
      <Card
        key={plan?.id || index}
        className={`ct-topup-plan-card ${
          isCurrent ? 'ct-topup-plan-card-current' : ''
        } ${isRecommended ? 'ct-topup-plan-card-recommended' : ''}`}
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
              <Tag
                color={isCurrent ? 'green' : 'amber'}
                shape='circle'
                className='ct-topup-plan-ribbon'
              >
                {isCurrent ? t('当前套餐') : t('推荐')}
              </Tag>
            )}
          </div>

          <div className='ct-topup-plan-price'>
            <span>{currency.symbol}</span>
            <strong>{displayPrice}</strong>
            <em>/{t('月')}</em>
          </div>

          <div className='ct-topup-plan-summary'>
            <div className='ct-topup-plan-quota'>
              <Gauge size={15} />
              <span>{t('额度')}</span>
              <strong>{totalLabel}</strong>
            </div>
            <div className='ct-topup-plan-quota'>
              <Layers3 size={15} />
              <span>{t('适用')}</span>
              <strong>{targetUsers}</strong>
            </div>
          </div>

          <div className='ct-topup-plan-benefits'>
            <strong className='ct-topup-plan-benefits-title'>
              {t('套餐权益')}
            </strong>
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
              theme='outline'
              type='primary'
              block
              disabled={reached}
              onClick={() => {
                if (!reached) openBuy(p);
              }}
              className='ct-topup-plan-action-button'
            >
              {reached ? t('已达上限') : t('立即订阅')}
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
          {disableSubscriptionPreference && isSubscriptionPreference && (
            <Text type='tertiary' size='small' className='ct-topup-page-note'>
              {t('已保存偏好为')}
              {billingPreference === 'subscription_only'
                ? t('仅用订阅')
                : t('优先订阅')}
              {t('，当前无生效订阅，将自动使用钱包')}
            </Text>
          )}

          <Card className='ct-topup-panel ct-topup-subscription-overview'>
            <div className='ct-topup-current-band'>
              <div className='ct-topup-current-band-main'>
                <div>
                  <span>{t('当前套餐')}</span>
                  <div className='ct-topup-current-title-line'>
                    <strong>{currentTitle}</strong>
                    {currentTag && (
                      <Tag color='green' shape='circle'>
                        {currentTag}
                      </Tag>
                    )}
                  </div>
                </div>
              </div>
              <div className='ct-topup-current-days'>
                <span>{t('剩余')}</span>
                <div>
                  <strong>{remainDays}</strong>
                  <span>{t('天')}</span>
                </div>
                <em>
                  {hasActiveSubscription
                    ? `${t('有效期')} ${formatDate(subscription?.start_time)} - ${formatDate(subscription?.end_time)}`
                    : t('购买套餐后即可享受模型权益')}
                </em>
              </div>
              <div className='ct-topup-current-band-progress'>
                <div className='ct-topup-current-progress-head'>
                  <span>{t('订阅额度使用')}</span>
                  <strong>
                    {activeQuotaStats.hasUnlimited
                      ? t('不限')
                      : `${activeQuotaStats.percent}%`}
                  </strong>
                </div>
                <div className='ct-topup-current-progress-track'>
                  <span style={{ width: progressWidth }} />
                </div>
                {activeQuotaStats.hasUnlimited && (
                  <em>{t('当前套餐额度不限')}</em>
                )}
              </div>
            </div>

            <div className='ct-topup-balance-strip ct-topup-balance-strip-wide'>
              <SubscriptionStat
                icon={Gauge}
                label={t('订阅剩余额度')}
                value={remainQuotaLabel}
                tone='teal'
              />
              <SubscriptionStat
                icon={PieChart}
                label={t('订阅已用额度')}
                value={usedQuotaLabel}
                tone='blue'
              />
              <SubscriptionStat
                icon={Database}
                label={t('订阅总额度')}
                value={totalPlanQuotaLabel}
                tone='purple'
              />
              <SubscriptionStat
                icon={ShieldCheck}
                label={t('扣费策略')}
                value={displayBillingPreferenceLabel}
                tone='green'
              />
            </div>
          </Card>

          <div className='ct-topup-subscription-main-grid'>
            <Card className='ct-topup-panel ct-topup-plan-board'>
              <div className='ct-topup-plan-grid ct-topup-plan-grid-three'>
                {plans.length > 0 ? (
                  plans.map(renderPlanCard)
                ) : (
                  renderEmptyPlanPanel()
                )}
              </div>
            </Card>

            <div className='ct-topup-subscription-side'>
              <Card className='ct-topup-panel ct-topup-subscription-timeline'>
                <div className='ct-topup-section-heading ct-topup-timeline-head'>
                  <div>
                    <strong>{t('订阅记录')}</strong>
                    <span>{t('查看最近购买和生效状态')}</span>
                  </div>
                  {hasAnySubscription && (
                    <Button theme='borderless' type='tertiary' size='small'>
                      {t('查看全部')}
                    </Button>
                  )}
                </div>
                <div className='ct-topup-timeline-list'>
                  {hasAnySubscription ? (
                    allSubscriptions.slice(0, 5).map((sub, index) => {
                      const item = sub?.subscription;
                      const isActive =
                        item?.status === 'active' &&
                        (!item?.end_time || item.end_time > Date.now() / 1000);
                      return (
                        <div
                          className={`ct-topup-timeline-item ${
                            isActive ? 'ct-topup-timeline-item-active' : ''
                          }`}
                          key={item?.id || index}
                        >
                          <span>
                            {isActive ? (
                              <CheckCircle2 size={14} />
                            ) : (
                              <Clock3 size={14} />
                            )}
                          </span>
                          <div>
                            <div className='ct-topup-timeline-title'>
                              <strong>{getSubscriptionTitle(sub)}</strong>
                              {isActive && (
                                <Tag color='green' shape='circle' size='small'>
                                  {t('生效')}
                                </Tag>
                              )}
                            </div>
                            <em>{formatTime(item?.start_time)}</em>
                          </div>
                          <strong className='ct-topup-timeline-price'>
                            {getSubscriptionPriceLabel(sub)}
                          </strong>
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
                    <span>{t('订阅与钱包的消费顺序')}</span>
                  </div>
                </div>
                <div className='ct-topup-rule-copy'>
                  <strong>{t('订阅额度优先扣费')}</strong>
                  <p>
                    {t(
                      '模型调用产生费用后，会优先使用订阅额度。订阅额度不足时，再按账户钱包余额补扣。',
                    )}
                  </p>
                </div>
                <div className='ct-topup-billing-flow'>
                  <div className='ct-topup-billing-flow-cost'>
                    <span>
                      <ShoppingCart size={15} />
                    </span>
                    <strong>{t('产生费用')}</strong>
                  </div>
                  <ArrowRight size={14} />
                  <div className='ct-topup-billing-flow-subscription'>
                    <span>
                      <ShieldCheck size={15} />
                    </span>
                    <strong>{t('订阅额度优先')}</strong>
                  </div>
                  <ArrowRight size={14} />
                  <div className='ct-topup-billing-flow-wallet'>
                    <span>
                      <Wallet size={15} />
                    </span>
                    <strong>{t('钱包补扣')}</strong>
                  </div>
                </div>
                <div className='ct-topup-rule-list'>
                  <div>
                    <CheckCircle2 size={14} />
                    <span>
                      {t('优先消耗订阅套餐额度，避免钱包余额被提前扣减。')}
                    </span>
                  </div>
                  <div>
                    <CheckCircle2 size={14} />
                    <span>
                      {t('订阅额度不足时，剩余费用会继续从钱包余额扣除。')}
                    </span>
                  </div>
                  <div>
                    <CheckCircle2 size={14} />
                    <span>
                      {t('切换扣费偏好后，新请求会按新的顺序进行扣费。')}
                    </span>
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
