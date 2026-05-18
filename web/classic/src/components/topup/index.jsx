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

import React, { useEffect, useState, useContext, useRef } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  API,
  showError,
  showInfo,
  showSuccess,
  renderQuota,
  renderQuotaWithAmount,
  copy,
  getQuotaPerUnit,
} from '../../helpers';
import { Modal, Toast, Button, Tag } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import {
  Copy,
  Receipt,
  ReceiptText,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react';

import ConsolePageShell from '../layout/ConsolePageShell';
import RechargeCard from './RechargeCard';
import InvitationCard from './InvitationCard';
import SubscriptionPlansCard from './SubscriptionPlansCard';
import TransferModal from './modals/TransferModal';
import PaymentConfirmModal from './modals/PaymentConfirmModal';
import TopupHistoryModal from './modals/TopupHistoryModal';

const AFFILIATE_PAGE_SIZE = 10;

const isVisibleRechargePayMethod = (method) => {
  if (!method?.name || !method?.type) return false;
  const normalizedType = String(method.type).toLowerCase();
  const normalizedName = String(method.name).trim();
  return !normalizedType.startsWith('custom') && !normalizedName.startsWith('自定义');
};

const TopUp = ({ view = 'all' }) => {
  const { t } = useTranslation();
  const [searchParams, setSearchParams] = useSearchParams();
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const normalizedView = ['affiliate', 'recharge', 'subscription'].includes(
    view,
  )
    ? view
    : 'all';
  const showAffiliate =
    normalizedView === 'all' || normalizedView === 'affiliate';
  const showRecharge =
    normalizedView === 'all' || normalizedView === 'recharge';
  const showSubscription =
    normalizedView === 'all' || normalizedView === 'subscription';
  const isSplitView = normalizedView !== 'all';

  const [redemptionCode, setRedemptionCode] = useState('');
  const [amount, setAmount] = useState(0.0);
  const [minTopUp, setMinTopUp] = useState(statusState?.status?.min_topup || 1);
  const [topUpCount, setTopUpCount] = useState(
    statusState?.status?.min_topup || 1,
  );
  const [topUpLink, setTopUpLink] = useState('');
  const [enableOnlineTopUp, setEnableOnlineTopUp] = useState(
    statusState?.status?.enable_online_topup || false,
  );
  const [priceRatio, setPriceRatio] = useState(statusState?.status?.price || 1);

  const [enableStripeTopUp, setEnableStripeTopUp] = useState(
    statusState?.status?.enable_stripe_topup || false,
  );
  const [statusLoading, setStatusLoading] = useState(true);

  // Creem 相关状态
  const [creemProducts, setCreemProducts] = useState([]);
  const [enableCreemTopUp, setEnableCreemTopUp] = useState(false);
  const [creemOpen, setCreemOpen] = useState(false);
  const [selectedCreemProduct, setSelectedCreemProduct] = useState(null);

  // Waffo 相关状态
  const [enableWaffoTopUp, setEnableWaffoTopUp] = useState(false);
  const [waffoPayMethods, setWaffoPayMethods] = useState([]);
  const [waffoMinTopUp, setWaffoMinTopUp] = useState(1);
  const [enableWaffoPancakeTopUp, setEnableWaffoPancakeTopUp] = useState(false);
  const [waffoPancakeMinTopUp, setWaffoPancakeMinTopUp] = useState(1);

  const [isSubmitting, setIsSubmitting] = useState(false);
  const [open, setOpen] = useState(false);
  const [payWay, setPayWay] = useState('');
  const [amountLoading, setAmountLoading] = useState(false);
  const [paymentLoading, setPaymentLoading] = useState(false);
  const [confirmLoading, setConfirmLoading] = useState(false);
  const [payMethods, setPayMethods] = useState([]);

  const affFetchedRef = useRef(false);
  const recentTopupsReloadRef = useRef(null);

  // 邀请相关状态
  const [affLink, setAffLink] = useState('');
  const [openTransfer, setOpenTransfer] = useState(false);
  const [transferAmount, setTransferAmount] = useState(0);
  const [affiliateDashboard, setAffiliateDashboard] = useState(null);
  const [affiliateLoading, setAffiliateLoading] = useState(false);
  const [affiliatePage, setAffiliatePage] = useState(1);

  // 账单Modal状态
  const [openHistory, setOpenHistory] = useState(false);
  const [rechargeHeaderRefreshing, setRechargeHeaderRefreshing] = useState(false);
  const [subscriptionHeaderRefreshing, setSubscriptionHeaderRefreshing] =
    useState(false);

  // 订阅相关
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const [subscriptionLoading, setSubscriptionLoading] = useState(true);
  const [billingPreference, setBillingPreference] =
    useState('subscription_first');
  const [activeSubscriptions, setActiveSubscriptions] = useState([]);
  const [allSubscriptions, setAllSubscriptions] = useState([]);

  // 预设充值额度选项
  const [presetAmounts, setPresetAmounts] = useState([]);
  const [selectedPreset, setSelectedPreset] = useState(null);

  // 充值配置信息
  const [topupInfo, setTopupInfo] = useState({
    amount_options: [],
    discount: {},
  });

  const confirmPayMethods = [
    ...payMethods,
    ...waffoPayMethods.map((method, index) => ({
      ...method,
      type: `waffo:${index}`,
      min_topup: waffoMinTopUp,
      color: method.color || 'rgba(var(--semi-primary-5), 1)',
    })),
  ];

  const getPayMethodConfig = (payment) =>
    confirmPayMethods.find((method) => method.type === payment);

  const hasOnlineTopUp =
    enableOnlineTopUp ||
    enableStripeTopUp ||
    enableCreemTopUp ||
    enableWaffoTopUp ||
    enableWaffoPancakeTopUp;

  const getPaymentMinTopUp = (payment) => {
    const configuredMinTopUp = Number(getPayMethodConfig(payment)?.min_topup);
    return Number.isFinite(configuredMinTopUp) && configuredMinTopUp > 0
      ? configuredMinTopUp
      : minTopUp;
  };

  const requestAmountByPayment = async (payment, value) => {
    if (payment === 'stripe') {
      return getStripeAmount(value);
    }
    if (payment === 'waffo_pancake') {
      return getWaffoPancakeAmount(value);
    }
    if (typeof payment === 'string' && payment.startsWith('waffo:')) {
      return getWaffoAmount(value);
    }
    return getAmount(value);
  };

  const topUp = async () => {
    if (redemptionCode === '') {
      showInfo(t('请输入兑换码！'));
      return;
    }
    setIsSubmitting(true);
    try {
      const res = await API.post('/api/user/topup', {
        key: redemptionCode,
      });
      const { success, message, data } = res.data;
      if (success) {
        showSuccess(t('兑换成功！'));
        Modal.success({
          title: t('兑换成功！'),
          content: t('成功兑换额度：') + renderQuota(data),
          centered: true,
        });
        if (userState.user) {
          const updatedUser = {
            ...userState.user,
            quota: userState.user.quota + data,
          };
          userDispatch({ type: 'login', payload: updatedUser });
        }
        setRedemptionCode('');
      } else {
        showError(message);
      }
    } catch (err) {
      showError(t('请求失败'));
    } finally {
      setIsSubmitting(false);
    }
  };

  const openTopUpLink = () => {
    if (!topUpLink) {
      showError(t('超级管理员未设置充值链接！'));
      return;
    }
    window.open(topUpLink, '_blank');
  };

  const preTopUp = async (payment) => {
    if (payment === 'stripe') {
      if (!enableStripeTopUp) {
        showError(t('管理员未开启Stripe充值！'));
        return;
      }
    } else if (payment === 'waffo_pancake') {
      if (!enableWaffoPancakeTopUp) {
        showError(t('管理员未开启 Waffo Pancake 充值！'));
        return;
      }
    } else if (payment.startsWith('waffo:')) {
      if (!enableWaffoTopUp) {
        showError(t('管理员未开启 Waffo 充值！'));
        return;
      }
    } else {
      if (!enableOnlineTopUp) {
        showError(t('管理员未开启在线充值！'));
        return;
      }
    }

    setPayWay(payment);
    setPaymentLoading(true);
    try {
      const selectedMinTopUp = getPaymentMinTopUp(payment);
      await requestAmountByPayment(payment);

      if (topUpCount < selectedMinTopUp) {
        showError(t('充值数量不能小于') + selectedMinTopUp);
        return;
      }
      setOpen(true);
    } catch (error) {
      showError(t('获取金额失败'));
    } finally {
      setPaymentLoading(false);
    }
  };

  const onlineTopUp = async () => {
    if (payWay === 'waffo_pancake') {
      setConfirmLoading(true);
      try {
        await waffoPancakeTopUp();
      } finally {
        setOpen(false);
        setConfirmLoading(false);
      }
      return;
    }

    if (payWay.startsWith('waffo:')) {
      const payMethodIndex = Number(payWay.split(':')[1]);
      setConfirmLoading(true);
      try {
        await waffoTopUp(Number.isFinite(payMethodIndex) ? payMethodIndex : 0);
      } finally {
        setOpen(false);
        setConfirmLoading(false);
      }
      return;
    }

    if (payWay === 'stripe') {
      // Stripe 支付处理
      if (amount === 0) {
        await getStripeAmount();
      }
    } else {
      // 普通支付处理
      if (amount === 0) {
        await getAmount();
      }
    }

    if (topUpCount < minTopUp) {
      showError('充值数量不能小于' + minTopUp);
      return;
    }
    setConfirmLoading(true);
    try {
      let res;
      if (payWay === 'stripe') {
        // Stripe 支付请求
        res = await API.post('/api/user/stripe/pay', {
          amount: parseInt(topUpCount),
          payment_method: 'stripe',
        });
      } else {
        // 普通支付请求
        res = await API.post('/api/user/pay', {
          amount: parseInt(topUpCount),
          payment_method: payWay,
        });
      }

      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          if (payWay === 'stripe') {
            // Stripe 支付回调处理
            window.open(data.pay_link, '_blank');
          } else {
            // 普通支付表单提交
            let params = data;
            let url = res.data.url;
            let form = document.createElement('form');
            form.action = url;
            form.method = 'POST';
            let isSafari =
              navigator.userAgent.indexOf('Safari') > -1 &&
              navigator.userAgent.indexOf('Chrome') < 1;
            if (!isSafari) {
              form.target = '_blank';
            }
            for (let key in params) {
              let input = document.createElement('input');
              input.type = 'hidden';
              input.name = key;
              input.value = params[key];
              form.appendChild(input);
            }
            document.body.appendChild(form);
            form.submit();
            document.body.removeChild(form);
          }
        } else {
          const errorMsg =
            typeof data === 'string' ? data : message || t('支付失败');
          showError(errorMsg);
        }
      } else {
        showError(res);
      }
    } catch (err) {
      showError(t('支付请求失败'));
    } finally {
      setOpen(false);
      setConfirmLoading(false);
    }
  };

  const creemPreTopUp = async (product) => {
    if (!enableCreemTopUp) {
      showError(t('管理员未开启 Creem 充值！'));
      return;
    }
    setSelectedCreemProduct(product);
    setCreemOpen(true);
  };

  const onlineCreemTopUp = async () => {
    if (!selectedCreemProduct) {
      showError(t('请选择产品'));
      return;
    }
    // Validate product has required fields
    if (!selectedCreemProduct.productId) {
      showError(t('产品配置错误，请联系管理员'));
      return;
    }
    setConfirmLoading(true);
    try {
      const res = await API.post('/api/user/creem/pay', {
        product_id: selectedCreemProduct.productId,
        payment_method: 'creem',
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          processCreemCallback(data);
        } else {
          const errorMsg =
            typeof data === 'string' ? data : message || t('支付失败');
          showError(errorMsg);
        }
      } else {
        showError(res);
      }
    } catch (err) {
      showError(t('支付请求失败'));
    } finally {
      setCreemOpen(false);
      setConfirmLoading(false);
    }
  };

  const waffoTopUp = async (payMethodIndex) => {
    try {
      if (topUpCount < waffoMinTopUp) {
        showError(t('充值数量不能小于') + waffoMinTopUp);
        return;
      }
      setPaymentLoading(true);
      const requestBody = {
        amount: parseInt(topUpCount),
      };
      if (payMethodIndex != null) {
        requestBody.pay_method_index = payMethodIndex;
      }
      const res = await API.post('/api/user/waffo/pay', requestBody);
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success' && data?.payment_url) {
          window.open(data.payment_url, '_blank');
        } else {
          showError(data || t('支付请求失败'));
        }
      } else {
        showError(res);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaymentLoading(false);
    }
  };

  const getWaffoAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post(
        '/api/user/waffo/amount',
        {
          amount: parseInt(value),
        },
        { skipErrorHandler: true },
      );
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({ content: '错误：' + data, id: 'getAmount' });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // amount fetch failed silently
    } finally {
      setAmountLoading(false);
    }
  };

  const waffoPancakeTopUp = async () => {
    const minTopUpValue = Number(waffoPancakeMinTopUp || 1);
    if (topUpCount < minTopUpValue) {
      showError(t('充值数量不能小于') + minTopUpValue);
      return;
    }

    setPaymentLoading(true);
    try {
      const res = await API.post('/api/user/waffo-pancake/pay', {
        amount: parseInt(topUpCount),
      });
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          const checkoutUrl = data?.checkout_url || '';
          if (checkoutUrl) {
            window.open(checkoutUrl, '_blank');
          } else {
            showError(t('支付请求失败'));
          }
        } else {
          const errorMsg =
            typeof data === 'string' ? data : message || t('支付请求失败');
          showError(errorMsg);
        }
      } else {
        showError(res);
      }
    } catch (e) {
      showError(t('支付请求失败'));
    } finally {
      setPaymentLoading(false);
    }
  };

  const getWaffoPancakeAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post(
        '/api/user/waffo-pancake/amount',
        {
          amount: parseInt(value),
        },
        { skipErrorHandler: true },
      );
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({ content: '错误：' + data, id: 'getAmount' });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // amount fetch failed silently
    } finally {
      setAmountLoading(false);
    }
  };

  const processCreemCallback = (data) => {
    // 与 Stripe 保持一致的实现方式
    window.open(data.checkout_url, '_blank');
  };

  const getUserQuota = async () => {
    try {
      let res = await API.get(`/api/user/self`, { skipErrorHandler: true });
      const { success, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
      }
    } catch (e) {
      // Keep the page usable with cached user state when the local API is unavailable.
    }
  };

  const getSubscriptionPlans = async () => {
    setSubscriptionLoading(true);
    try {
      const res = await API.get('/api/subscription/plans', {
        skipErrorHandler: true,
      });
      if (res.data?.success) {
        setSubscriptionPlans(res.data.data || []);
      }
    } catch (e) {
      setSubscriptionPlans([]);
    } finally {
      setSubscriptionLoading(false);
    }
  };

  const getSubscriptionSelf = async () => {
    try {
      const res = await API.get('/api/subscription/self', {
        skipErrorHandler: true,
      });
      if (res.data?.success) {
        setBillingPreference(
          res.data.data?.billing_preference || 'subscription_first',
        );
        // Active subscriptions
        const activeSubs = res.data.data?.subscriptions || [];
        setActiveSubscriptions(activeSubs);
        // All subscriptions (including expired)
        const allSubs = res.data.data?.all_subscriptions || [];
        setAllSubscriptions(allSubs);
      }
    } catch (e) {
      // ignore
    }
  };

  const updateBillingPreference = async (pref) => {
    const previousPref = billingPreference;
    setBillingPreference(pref);
    try {
      const res = await API.put('/api/subscription/self/preference', {
        billing_preference: pref,
      });
      if (res.data?.success) {
        showSuccess(t('更新成功'));
        const normalizedPref =
          res.data?.data?.billing_preference || pref || previousPref;
        setBillingPreference(normalizedPref);
      } else {
        showError(res.data?.message || t('更新失败'));
        setBillingPreference(previousPref);
      }
    } catch (e) {
      showError(t('请求失败'));
      setBillingPreference(previousPref);
    }
  };

  // 获取充值配置信息
  const getTopupInfo = async () => {
    try {
      const res = await API.get('/api/user/topup/info', {
        skipErrorHandler: true,
      });
      const { message, data, success } = res.data;
      if (success) {
        const amountOptions = Array.isArray(data.amount_options)
          ? data.amount_options
          : [];
        const discountConfig = data.discount || {};
        let minTopUpValue = 1;

        setTopupInfo({
          amount_options: amountOptions,
          discount: discountConfig,
        });

        // 处理支付方式
        let payMethods = data.pay_methods || [];
        try {
          if (typeof payMethods === 'string') {
            payMethods = JSON.parse(payMethods);
          }
          if (payMethods && payMethods.length > 0) {
            // 检查name和type是否为空
            payMethods = payMethods.filter(isVisibleRechargePayMethod);
            // 如果没有color，则设置默认颜色
            payMethods = payMethods.map((method) => {
              // 规范化最小充值数
              const normalizedMinTopup = Number(method.min_topup);
              method.min_topup = Number.isFinite(normalizedMinTopup)
                ? normalizedMinTopup
                : 0;

              // Stripe 的最小充值从后端字段回填
              if (
                method.type === 'stripe' &&
                (!method.min_topup || method.min_topup <= 0)
              ) {
                const stripeMin = Number(data.stripe_min_topup);
                if (Number.isFinite(stripeMin)) {
                  method.min_topup = stripeMin;
                }
              }

              if (!method.color) {
                if (method.type === 'alipay') {
                  method.color = 'rgba(var(--semi-blue-5), 1)';
                } else if (method.type === 'wxpay') {
                  method.color = 'rgba(var(--semi-green-5), 1)';
                } else if (method.type === 'stripe') {
                  method.color = 'rgba(var(--semi-purple-5), 1)';
                } else {
                  method.color = 'rgba(var(--semi-primary-5), 1)';
                }
              }
              return method;
            });
          } else {
            payMethods = [];
          }

          // 如果启用了 Stripe 支付，添加到支付方法列表
          // 这个逻辑现在由后端处理，如果 Stripe 启用，后端会在 pay_methods 中包含它

          setPayMethods(payMethods);
          const enableStripeTopUp = data.enable_stripe_topup || false;
          const enableOnlineTopUp = data.enable_online_topup || false;
          const enableCreemTopUp = data.enable_creem_topup || false;
          const enableWaffoTopUp = data.enable_waffo_topup || false;
          const enableWaffoPancakeTopUp =
            data.enable_waffo_pancake_topup || false;
          minTopUpValue = enableOnlineTopUp
            ? data.min_topup
            : enableStripeTopUp
              ? data.stripe_min_topup
              : enableWaffoTopUp
                ? data.waffo_min_topup
                : enableWaffoPancakeTopUp
                  ? data.waffo_pancake_min_topup
                  : 1;
          setEnableOnlineTopUp(enableOnlineTopUp);
          setEnableStripeTopUp(enableStripeTopUp);
          setEnableCreemTopUp(enableCreemTopUp);
          setEnableWaffoTopUp(enableWaffoTopUp);
          setWaffoPayMethods(data.waffo_pay_methods || []);
          setWaffoMinTopUp(data.waffo_min_topup || 1);
          setEnableWaffoPancakeTopUp(enableWaffoPancakeTopUp);
          setWaffoPancakeMinTopUp(data.waffo_pancake_min_topup || 1);
          setMinTopUp(minTopUpValue);
          setTopUpCount(minTopUpValue);
          setTopUpLink(data.topup_link || '');

          // 设置 Creem 产品
          try {
            const products = JSON.parse(data.creem_products || '[]');
            setCreemProducts(products);
          } catch (e) {
            setCreemProducts([]);
          }

          // 如果没有自定义充值数量选项，根据最小充值金额生成预设充值额度选项
          if (amountOptions.length === 0) {
            const generatedPresets = generatePresetAmounts(minTopUpValue);
            setPresetAmounts(generatedPresets);
            setSelectedPreset(
              generatedPresets.some((preset) => preset.value === minTopUpValue)
                ? minTopUpValue
                : null,
            );
          }

          // 初始化显示实付金额
          getAmount(minTopUpValue);
        } catch (e) {
          setPayMethods([]);
        }

        // 如果有自定义充值数量选项，使用它们替换默认的预设选项
        if (amountOptions.length > 0) {
          const customPresets = amountOptions.map((amount) => ({
            value: amount,
            discount: discountConfig[amount] || 1.0,
          }));
          setPresetAmounts(customPresets);
          setSelectedPreset(
            customPresets.some((preset) => preset.value === minTopUpValue)
              ? minTopUpValue
              : null,
          );
        }
      } else {
        // Render the configured empty state instead of interrupting the page with a toast.
      }
    } catch (error) {
      // Render the configured empty state instead of interrupting the page with a toast.
    } finally {
      setStatusLoading(false);
    }
  };

  // 获取邀请链接
  const getAffLink = async () => {
    try {
      const res = await API.get('/api/user/aff', { skipErrorHandler: true });
      const { success, data } = res.data;
      if (success) {
        let link = `${window.location.origin}/register?aff=${data}`;
        setAffLink(link);
      }
    } catch (e) {
      setAffLink('');
    }
  };

  const getAffiliateDashboard = async (page = affiliatePage) => {
    if (!showAffiliate) return;
    setAffiliateLoading(true);
    try {
      const res = await API.get('/api/user/aff/dashboard', {
        params: {
          p: page,
          page_size: AFFILIATE_PAGE_SIZE,
        },
        skipErrorHandler: true,
        disableDuplicate: true,
      });
      const { success, data } = res.data;
      if (success) {
        setAffiliateDashboard(data);
      }
    } catch (e) {
      setAffiliateDashboard((prev) => prev || null);
    } finally {
      setAffiliateLoading(false);
    }
  };

  const handleAffiliatePageChange = (page) => {
    const nextPage = Math.max(1, page);
    if (nextPage === affiliatePage) return;
    setAffiliatePage(nextPage);
  };

  const refreshAffiliateDashboard = () => {
    getAffiliateDashboard(affiliatePage).then();
  };

  // 划转邀请额度
  const transfer = async () => {
    if (transferAmount < getQuotaPerUnit()) {
      showError(t('划转金额最低为') + ' ' + renderQuota(getQuotaPerUnit()));
      return;
    }
    const res = await API.post(`/api/user/aff_transfer`, {
      quota: transferAmount,
    });
    const { success, message } = res.data;
    if (success) {
      showSuccess(message);
      setOpenTransfer(false);
      getUserQuota().then();
      getAffiliateDashboard(affiliatePage).then();
    } else {
      showError(message);
    }
  };

  // 复制邀请链接
  const handleAffLinkClick = async () => {
    const fallbackCode = userState?.user?.aff_code;
    const link =
      affLink ||
      (fallbackCode
        ? `${window.location.origin}/register?aff=${fallbackCode}`
        : `${window.location.origin}/register`);
    await copy(link);
    showSuccess(t('邀请链接已复制到剪切板'));
  };

  // URL 参数自动打开账单弹窗（支付回跳时触发）
  useEffect(() => {
    if (searchParams.get('show_history') === 'true') {
      setOpenHistory(true);
      searchParams.delete('show_history');
      setSearchParams(searchParams, { replace: true });
    }
  }, []);

  useEffect(() => {
    // 始终获取最新用户数据，确保余额等统计信息准确
    getUserQuota().then();
    setTransferAmount(getQuotaPerUnit());
  }, []);

  useEffect(() => {
    const routeClass = 'ct-topup-affiliate-route';
    if (normalizedView === 'affiliate') {
      document.body.classList.add(routeClass);
    } else {
      document.body.classList.remove(routeClass);
    }
    return () => document.body.classList.remove(routeClass);
  }, [normalizedView]);

  useEffect(() => {
    if (!showAffiliate || affFetchedRef.current) return;
    affFetchedRef.current = true;
    getAffLink().then();
  }, [showAffiliate]);

  useEffect(() => {
    if (!showAffiliate) return;
    getAffiliateDashboard(affiliatePage).then();
  }, [showAffiliate, affiliatePage]);

  // 在 statusState 可用时获取充值信息
  useEffect(() => {
    if (showRecharge || showSubscription) {
      getTopupInfo().then();
    }
    if (showSubscription) {
      getSubscriptionPlans().then();
      getSubscriptionSelf().then();
    }
  }, [showRecharge, showSubscription]);

  useEffect(() => {
    if (statusState?.status) {
      // const minTopUpValue = statusState.status.min_topup || 1;
      // setMinTopUp(minTopUpValue);
      // setTopUpCount(minTopUpValue);
      setPriceRatio(statusState.status.price || 1);

      setStatusLoading(false);
    }
  }, [statusState?.status]);

  const renderAmount = (value = amount) => {
    const numericValue = Number(value || 0);
    return `${Number.isFinite(numericValue) ? numericValue.toFixed(2) : value} ${t('元')}`;
  };

  const getAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post(
        '/api/user/amount',
        {
          amount: parseFloat(value),
        },
        { skipErrorHandler: true },
      );
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({ content: '错误：' + data, id: 'getAmount' });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // amount fetch failed silently
    }
    setAmountLoading(false);
  };

  const getStripeAmount = async (value) => {
    if (value === undefined) {
      value = topUpCount;
    }
    setAmountLoading(true);
    try {
      const res = await API.post(
        '/api/user/stripe/amount',
        {
          amount: parseFloat(value),
        },
        { skipErrorHandler: true },
      );
      if (res !== undefined) {
        const { message, data } = res.data;
        if (message === 'success') {
          setAmount(parseFloat(data));
        } else {
          setAmount(0);
          Toast.error({ content: '错误：' + data, id: 'getAmount' });
        }
      } else {
        showError(res);
      }
    } catch (err) {
      // amount fetch failed silently
    } finally {
      setAmountLoading(false);
    }
  };

  const handleCancel = () => {
    setOpen(false);
  };

  const handleTransferCancel = () => {
    setOpenTransfer(false);
  };

  const handleOpenHistory = () => {
    setOpenHistory(true);
  };

  const handleHistoryCancel = () => {
    setOpenHistory(false);
  };

  const handleRechargeHeaderRefresh = async () => {
    setRechargeHeaderRefreshing(true);
    try {
      if (typeof recentTopupsReloadRef.current === 'function') {
        await recentTopupsReloadRef.current();
      } else {
        await getTopupInfo();
      }
    } finally {
      setRechargeHeaderRefreshing(false);
    }
  };

  const handleSubscriptionHeaderRefresh = async () => {
    setSubscriptionHeaderRefreshing(true);
    try {
      await getSubscriptionSelf();
    } finally {
      setSubscriptionHeaderRefreshing(false);
    }
  };

  const handleCreemCancel = () => {
    setCreemOpen(false);
    setSelectedCreemProduct(null);
  };

  // 选择预设充值额度
  const selectPresetAmount = (preset) => {
    setTopUpCount(preset.value);
    setSelectedPreset(preset.value);

    // 计算实际支付金额，考虑折扣
    const discount = preset.discount || topupInfo.discount[preset.value] || 1.0;
    const discountedAmount = preset.value * priceRatio * discount;
    setAmount(discountedAmount);
  };

  // 格式化大数字显示
  const formatLargeNumber = (num) => {
    return num.toString();
  };

  // 根据最小充值金额生成预设充值额度选项
  const generatePresetAmounts = (minAmount) => {
    const multipliers = [1, 5, 10, 30, 50, 100, 300, 500];
    return multipliers.map((multiplier) => ({
      value: minAmount * multiplier,
    }));
  };

  const affiliateSummary = affiliateDashboard?.summary || {};
  const affiliateCanTransfer =
    (affiliateSummary.aff_quota ?? userState?.user?.aff_quota) &&
    (affiliateSummary.aff_quota ?? userState?.user?.aff_quota) > 0;
  const hasActiveSubscription = activeSubscriptions.length > 0;
  const subscriptionPreferenceDisabled = !hasActiveSubscription;
  const isSubscriptionPreference =
    billingPreference === 'subscription_first' ||
    billingPreference === 'subscription_only';
  const effectiveBillingPreference =
    subscriptionPreferenceDisabled && isSubscriptionPreference
      ? 'wallet_first'
      : billingPreference ||
        (hasActiveSubscription ? 'subscription_first' : 'wallet_first');

  const scrollToAffiliateRecords = () => {
    document
      .querySelector('.ct-topup-record-panel')
      ?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  const renderSubscriptionHeaderActions = () => (
    <div className='ct-topup-subscription-toolbar'>
      <div className='ct-topup-pref-switch'>
        <Button
          className={`ct-topup-pref-option ${
            effectiveBillingPreference === 'subscription_first'
              ? 'ct-topup-pref-option-active'
              : ''
          }`}
          type={
            effectiveBillingPreference === 'subscription_first'
              ? 'primary'
              : 'tertiary'
          }
          theme={
            effectiveBillingPreference === 'subscription_first'
              ? 'solid'
              : 'light'
          }
          disabled={subscriptionPreferenceDisabled}
          onClick={() => updateBillingPreference('subscription_first')}
        >
          {t('订阅优先')}
        </Button>
        <Button
          className={`ct-topup-pref-option ${
            effectiveBillingPreference === 'wallet_first'
              ? 'ct-topup-pref-option-active'
              : ''
          }`}
          type={
            effectiveBillingPreference === 'wallet_first'
              ? 'primary'
              : 'tertiary'
          }
          theme={
            effectiveBillingPreference === 'wallet_first' ? 'solid' : 'light'
          }
          onClick={() => updateBillingPreference('wallet_first')}
        >
          {t('钱包优先')}
        </Button>
      </div>
      <Button
        icon={
          <RefreshCw
            size={15}
            className={subscriptionHeaderRefreshing ? 'animate-spin' : ''}
          />
        }
        theme='light'
        type='tertiary'
        onClick={handleSubscriptionHeaderRefresh}
        loading={subscriptionHeaderRefreshing}
        className='ct-topup-panel-action'
      >
        {t('刷新')}
      </Button>
    </div>
  );

  const pageHeader = {
    affiliate: {
      eyebrow: t('增长中心'),
      title: t('邀请有奖'),
      subtitle: t('分享专属邀请链接，好友注册并消费后奖励自动计入邀请余额'),
      badge: (
        <Tag
          color={affiliateCanTransfer ? 'green' : 'cyan'}
          shape='circle'
          prefixIcon={<ShieldCheck size={12} />}
        >
          {t('收益可划转')}
        </Tag>
      ),
      actions: (
        <>
          <Button
            type='primary'
            theme='solid'
            onClick={handleAffLinkClick}
            icon={<Copy size={14} />}
            className='ct-topup-primary-button'
          >
            {t('复制链接')}
          </Button>
          <Button
            theme='light'
            type='tertiary'
            onClick={scrollToAffiliateRecords}
            icon={<ReceiptText size={14} />}
            className='ct-topup-panel-action'
          >
            {t('邀请记录')}
          </Button>
          <Button
            theme='borderless'
            type='tertiary'
            onClick={refreshAffiliateDashboard}
            loading={affiliateLoading}
            icon={<RefreshCw size={14} />}
            className='ct-topup-panel-action'
            aria-label={t('刷新数据')}
          >
            {t('刷新数据')}
          </Button>
        </>
      ),
    },
    recharge: {
      eyebrow: t('账户中心'),
      title: t('账户充值'),
      subtitle: t('选择充值金额与支付方式，确认后进入安全支付流程'),
      badge: (
        <Tag
          color={hasOnlineTopUp ? 'green' : 'amber'}
          shape='circle'
          prefixIcon={<ShieldCheck size={12} />}
        >
          {hasOnlineTopUp ? t('在线支付可用') : t('在线支付未开启')}
        </Tag>
      ),
      actions: (
        <>
          <Button
            icon={<Receipt size={15} />}
            theme='light'
            type='tertiary'
            onClick={handleOpenHistory}
            className='ct-topup-panel-action'
          >
            {t('账单')}
          </Button>
          <Button
            icon={
              <RefreshCw
                size={15}
                className={rechargeHeaderRefreshing ? 'animate-spin' : ''}
              />
            }
            theme='light'
            type='tertiary'
            onClick={handleRechargeHeaderRefresh}
            loading={rechargeHeaderRefreshing}
            className='ct-topup-panel-action'
          >
            {t('刷新')}
          </Button>
        </>
      ),
    },
    subscription: {
      eyebrow: t('订阅中心'),
      title: t('套餐订阅'),
      subtitle: t('按套餐优先级管理长期额度，适合稳定高频使用场景'),
      badge: (
        <Tag
          color={hasActiveSubscription ? 'green' : 'cyan'}
          shape='circle'
          prefixIcon={<ShieldCheck size={12} />}
        >
          {hasActiveSubscription ? t('订阅生效中') : t('可购买套餐')}
        </Tag>
      ),
      actions: renderSubscriptionHeaderActions(),
    },
    all: {
      eyebrow: t('账户中心'),
      title: t('充值与订阅'),
      subtitle: t('管理邀请奖励、账户充值与订阅套餐'),
    },
  }[normalizedView];

  return (
    <ConsolePageShell
      className={`ct-topup-page ct-topup-page-${normalizedView}`}
      bodyClassName={`ct-topup-page-body ct-topup-page-body-${normalizedView}`}
      eyebrow={pageHeader.eyebrow}
      title={pageHeader.title}
      subtitle={pageHeader.subtitle}
      badge={pageHeader.badge}
      actions={pageHeader.actions}
    >
      {/* 划转模态框 */}
      <TransferModal
        t={t}
        openTransfer={openTransfer}
        transfer={transfer}
        handleTransferCancel={handleTransferCancel}
        userState={userState}
        renderQuota={renderQuota}
        getQuotaPerUnit={getQuotaPerUnit}
        transferAmount={transferAmount}
        setTransferAmount={setTransferAmount}
      />

      {/* 充值确认模态框 */}
      <PaymentConfirmModal
        t={t}
        open={open}
        onlineTopUp={onlineTopUp}
        handleCancel={handleCancel}
        confirmLoading={confirmLoading}
        topUpCount={topUpCount}
        renderQuotaWithAmount={renderQuotaWithAmount}
        amountLoading={amountLoading}
        renderAmount={renderAmount}
        payWay={payWay}
        payMethods={confirmPayMethods}
        amountNumber={amount}
        discountRate={topupInfo?.discount?.[topUpCount] || 1.0}
      />

      {/* 充值账单模态框 */}
      <TopupHistoryModal
        visible={openHistory}
        onCancel={handleHistoryCancel}
        t={t}
      />

      {/* Creem 充值确认模态框 */}
      <Modal
        title={t('确定要充值 $')}
        visible={creemOpen}
        onOk={onlineCreemTopUp}
        onCancel={handleCreemCancel}
        maskClosable={false}
        size='small'
        centered
        confirmLoading={confirmLoading}
      >
        {selectedCreemProduct && (
          <>
            <p>
              {t('产品名称')}：{selectedCreemProduct.name}
            </p>
            <p>
              {t('价格')}：{selectedCreemProduct.currency === 'EUR' ? '€' : '$'}
              {selectedCreemProduct.price}
            </p>
            <p>
              {t('充值额度')}：{selectedCreemProduct.quota}
            </p>
            <p>{t('是否确认充值？')}</p>
          </>
        )}
      </Modal>

      {/* 主布局区域 */}
      <div
        className={`ct-topup-flow ${
          isSplitView ? `ct-topup-flow-${normalizedView}` : ''
        }`}
      >
        {showAffiliate && (
          <InvitationCard
            t={t}
            userState={userState}
            renderQuota={renderQuota}
            setOpenTransfer={setOpenTransfer}
            affLink={affLink}
            affiliateDashboard={affiliateDashboard}
            affiliateLoading={affiliateLoading}
            affiliatePage={affiliatePage}
            onAffiliatePageChange={handleAffiliatePageChange}
            handleAffLinkClick={handleAffLinkClick}
          />
        )}

        {(showRecharge || showSubscription) && (
          <div
            className={`ct-topup-product-grid ${
              showRecharge !== showSubscription
                ? 'ct-topup-product-grid-single'
                : ''
            } ${
              normalizedView === 'subscription'
                ? 'ct-topup-product-grid-subscription'
                : ''
            }`}
          >
            {showRecharge && (
              <RechargeCard
                t={t}
                enableOnlineTopUp={enableOnlineTopUp}
                enableStripeTopUp={enableStripeTopUp}
                enableCreemTopUp={enableCreemTopUp}
                creemProducts={creemProducts}
                creemPreTopUp={creemPreTopUp}
                enableWaffoTopUp={enableWaffoTopUp}
                enableWaffoPancakeTopUp={enableWaffoPancakeTopUp}
                presetAmounts={presetAmounts}
                selectedPreset={selectedPreset}
                selectPresetAmount={selectPresetAmount}
                formatLargeNumber={formatLargeNumber}
                priceRatio={priceRatio}
                topUpCount={topUpCount}
                minTopUp={minTopUp}
                renderQuotaWithAmount={renderQuotaWithAmount}
                getAmount={getAmount}
                requestAmountByPayment={requestAmountByPayment}
                setTopUpCount={setTopUpCount}
                setSelectedPreset={setSelectedPreset}
                renderAmount={renderAmount}
                amount={amount}
                amountLoading={amountLoading}
                payMethods={confirmPayMethods}
                preTopUp={preTopUp}
                paymentLoading={paymentLoading}
                payWay={payWay}
                redemptionCode={redemptionCode}
                setRedemptionCode={setRedemptionCode}
                topUp={topUp}
                isSubmitting={isSubmitting}
                topUpLink={topUpLink}
                openTopUpLink={openTopUpLink}
                userState={userState}
                renderQuota={renderQuota}
                statusLoading={statusLoading}
                topupInfo={topupInfo}
                onOpenHistory={handleOpenHistory}
                onRecentTopupsReloadReady={(reload) => {
                  recentTopupsReloadRef.current = reload;
                }}
              />
            )}
            {showSubscription && (
              <SubscriptionPlansCard
                t={t}
                loading={subscriptionLoading}
                plans={subscriptionPlans}
                payMethods={payMethods}
                enableOnlineTopUp={enableOnlineTopUp}
                enableStripeTopUp={enableStripeTopUp}
                enableCreemTopUp={enableCreemTopUp}
                billingPreference={billingPreference}
                onChangeBillingPreference={updateBillingPreference}
                activeSubscriptions={activeSubscriptions}
                allSubscriptions={allSubscriptions}
                reloadSubscriptionSelf={getSubscriptionSelf}
                withCard={true}
                className='ct-topup-panel ct-topup-subscription-panel'
              />
            )}
          </div>
        )}
      </div>
    </ConsolePageShell>
  );
};

export default TopUp;
