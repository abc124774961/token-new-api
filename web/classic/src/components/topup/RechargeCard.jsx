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

import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Typography,
  Card,
  Button,
  Banner,
  Form,
  Spin,
  Tooltip,
  Tag,
  Badge,
} from '@douyinfe/semi-ui';
import { SiAlipay, SiWechat, SiStripe } from 'react-icons/si';
import {
  ArrowRight,
  BarChart2,
  CheckCircle2,
  CreditCard,
  FileText,
  LockKeyhole,
  Pencil,
  Receipt,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  TrendingUp,
  Wallet,
  Zap,
} from 'lucide-react';
import { IconGift } from '@douyinfe/semi-icons';
import {
  API,
  timestamp2string,
} from '../../helpers';
import { getCurrencyConfig } from '../../helpers/render';

const { Text } = Typography;

const PAYMENT_METHOD_MAP = {
  stripe: 'Stripe',
  creem: 'Creem',
  waffo: 'Waffo',
  waffo_pancake: 'Waffo Pancake',
  alipay: '支付宝',
  wxpay: '微信',
};

const renderPayMethodIcon = (payMethod) => {
  if (payMethod.type === 'alipay') {
    return <SiAlipay size={24} color='#1677FF' />;
  }
  if (payMethod.type === 'wxpay') {
    return <SiWechat size={24} color='#07C160' />;
  }
  if (payMethod.type === 'stripe') {
    return <SiStripe size={24} color='#635BFF' />;
  }
  if (payMethod.icon) {
    return (
      <img
        src={payMethod.icon}
        alt={payMethod.name}
        style={{
          width: 24,
          height: 24,
          objectFit: 'contain',
        }}
      />
    );
  }
  return (
    <CreditCard
      size={24}
      color={payMethod.color || 'var(--semi-color-text-2)'}
    />
  );
};

const RechargeStat = ({ icon: Icon, label, value, tone }) => (
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

const getPaymentTone = (type = '') => {
  if (type === 'alipay') return 'alipay';
  if (type === 'wxpay') return 'wechat';
  if (type === 'stripe') return 'stripe';
  if (type === 'redeem') return 'redeem';
  return 'generic';
};

const RechargeCard = ({
  t,
  enableOnlineTopUp,
  enableStripeTopUp,
  enableCreemTopUp,
  creemProducts,
  creemPreTopUp,
  presetAmounts,
  selectedPreset,
  selectPresetAmount,
  formatLargeNumber,
  priceRatio,
  topUpCount,
  minTopUp,
  renderQuotaWithAmount,
  getAmount,
  setTopUpCount,
  setSelectedPreset,
  renderAmount,
  amount,
  amountLoading,
  payMethods,
  preTopUp,
  paymentLoading,
  payWay,
  redemptionCode,
  setRedemptionCode,
  topUp,
  isSubmitting,
  topUpLink,
  openTopUpLink,
  userState,
  renderQuota,
  statusLoading,
  topupInfo,
  onOpenHistory,
  enableWaffoTopUp,
  enableWaffoPancakeTopUp,
}) => {
  const onlineFormApiRef = useRef(null);
  const redeemShellRef = useRef(null);
  const [selectedPayment, setSelectedPayment] = useState('');
  const [recentTopups, setRecentTopups] = useState([]);
  const [recentLoading, setRecentLoading] = useState(false);
  const regularPayMethods = payMethods || [];
  const hasOnlineTopUp =
    enableOnlineTopUp ||
    enableStripeTopUp ||
    enableCreemTopUp ||
    enableWaffoTopUp ||
    enableWaffoPancakeTopUp;
  const showPresetAmounts =
    (enableOnlineTopUp ||
      enableStripeTopUp ||
      enableWaffoTopUp ||
      enableWaffoPancakeTopUp) &&
    presetAmounts.length > 0;
  const selectedPresetValue = presetAmounts.some(
    (preset) => preset.value === selectedPreset,
  )
    ? selectedPreset
    : null;
  const isCustomAmount = !selectedPresetValue;
  const amountOptionsAvailable =
    enableOnlineTopUp ||
    enableStripeTopUp ||
    enableWaffoTopUp ||
    enableWaffoPancakeTopUp;

  const selectedMethod = useMemo(
    () => regularPayMethods.find((method) => method.type === selectedPayment),
    [regularPayMethods, selectedPayment],
  );

  const selectedDiscount =
    topupInfo?.discount?.[topUpCount] ||
    presetAmounts.find((preset) => preset.value === selectedPresetValue)
      ?.discount ||
    1.0;
  const originalPayAmount = Number(topUpCount || 0) * Number(priceRatio || 1);
  const actualPayAmount = Number(amount || 0);
  const discountSavings =
    actualPayAmount > 0
      ? Math.max(
          0,
          Number.isFinite(originalPayAmount - actualPayAmount)
            ? originalPayAmount - actualPayAmount
            : 0,
        )
      : 0;

  useEffect(() => {
    if (!statusLoading && !hasOnlineTopUp && selectedPayment !== 'redeem') {
      setSelectedPayment('redeem');
      return;
    }
    if (!selectedPayment && regularPayMethods.length > 0) {
      setSelectedPayment(regularPayMethods[0].type);
      return;
    }
    if (
      selectedPayment &&
      selectedPayment !== 'redeem' &&
      !regularPayMethods.some((method) => method.type === selectedPayment)
    ) {
      setSelectedPayment(regularPayMethods[0]?.type || '');
    }
  }, [hasOnlineTopUp, regularPayMethods, selectedPayment, statusLoading]);

  const loadRecentTopups = async () => {
    setRecentLoading(true);
    try {
      const res = await API.get('/api/user/topup/self?p=1&page_size=4', {
        skipErrorHandler: true,
      });
      if (res.data?.success) {
        setRecentTopups(res.data.data?.items || []);
      }
    } catch (e) {
      setRecentTopups([]);
    } finally {
      setRecentLoading(false);
    }
  };

  useEffect(() => {
    loadRecentTopups();
  }, []);

  const selectCustomAmount = () => {
    setSelectedPreset(null);
    onlineFormApiRef.current?.setValue('topUpCount', topUpCount);
  };

  const handlePay = () => {
    if (selectedPayment === 'redeem') {
      topUp();
      return;
    }
    if (selectedPayment) {
      preTopUp(selectedPayment);
    }
  };

  const getMethodDisabled = (payMethod) => {
    const minTopupVal = Number(payMethod.min_topup) || 0;
    const isStripe = payMethod.type === 'stripe';
    const isWaffo =
      typeof payMethod.type === 'string' && payMethod.type.startsWith('waffo:');
    const isWaffoPancake = payMethod.type === 'waffo_pancake';
    return (
      (!enableOnlineTopUp && !isStripe && !isWaffo && !isWaffoPancake) ||
      (!enableStripeTopUp && isStripe) ||
      (!enableWaffoTopUp && isWaffo) ||
      (!enableWaffoPancakeTopUp && isWaffoPancake) ||
      minTopupVal > Number(topUpCount || 0)
    );
  };

  const renderPaymentButton = (payMethod) => {
    const minTopupVal = Number(payMethod.min_topup) || 0;
    const disabled = getMethodDisabled(payMethod);
    const buttonEl = (
      <button
        key={payMethod.type}
        type='button'
        disabled={disabled}
        onClick={() => !disabled && setSelectedPayment(payMethod.type)}
        className={`ct-topup-payment-choice ct-topup-payment-choice-${getPaymentTone(
          payMethod.type,
        )} ${selectedPayment === payMethod.type ? 'ct-topup-payment-choice-active' : ''}`}
      >
        <span>{renderPayMethodIcon(payMethod)}</span>
        <strong>{payMethod.name}</strong>
        <em>
          {disabled && minTopupVal > Number(topUpCount || 0)
            ? `${t('最低')} ${minTopupVal}`
            : t('可用')}
        </em>
      </button>
    );

    return disabled && minTopupVal > Number(topUpCount || 0) ? (
      <Tooltip
        content={t('此支付方式最低充值金额为') + ' ' + minTopupVal}
        key={payMethod.type}
      >
        {buttonEl}
      </Tooltip>
    ) : (
      buttonEl
    );
  };

  const renderStatus = (status) => {
    const statusMap = {
      success: { type: 'success', label: t('成功') },
      pending: { type: 'warning', label: t('待支付') },
      failed: { type: 'danger', label: t('失败') },
      expired: { type: 'danger', label: t('已过期') },
    };
    const config = statusMap[status] || { type: 'primary', label: status || '-' };
    return (
      <span className='ct-topup-status-dot'>
        <Badge dot type={config.type} />
        {config.label}
      </span>
    );
  };

  const renderRedeemForm = () => (
    <Form initValues={{ redemptionCode: redemptionCode }}>
      <Form.Input
        field='redemptionCode'
        noLabel={true}
        placeholder={t('请输入兑换码')}
        value={redemptionCode}
        onChange={(value) => setRedemptionCode(value)}
        prefix={<IconGift />}
        suffix={
          <Button
            type='primary'
            theme='solid'
            onClick={topUp}
            loading={isSubmitting}
            className='ct-topup-primary-button'
          >
            {t('兑换额度')}
          </Button>
        }
        showClear
        style={{ width: '100%' }}
        extraText={
          topUpLink && (
            <Text type='tertiary'>
              {t('在找兑换码？')}
              <Text
                type='secondary'
                underline
                className='cursor-pointer'
                onClick={openTopUpLink}
              >
                {t('购买兑换码')}
              </Text>
            </Text>
          )
        }
      />
    </Form>
  );

  return (
    <div className='ct-topup-recharge-page'>
      <div className='ct-topup-page-head'>
        <div>
          <div className='ct-topup-page-title-row'>
            <Typography.Title heading={2} className='ct-topup-page-title'>
              {t('账户充值')}
            </Typography.Title>
            <Tag
              color={hasOnlineTopUp ? 'green' : 'amber'}
              shape='circle'
              prefixIcon={<ShieldCheck size={12} />}
            >
              {hasOnlineTopUp ? t('在线支付可用') : t('在线支付未开启')}
            </Tag>
          </div>
          <Text type='tertiary' className='ct-topup-page-subtitle'>
            {t('选择充值金额与支付方式，确认后进入安全支付流程')}
          </Text>
        </div>
        <div className='ct-topup-page-actions'>
          <Button
            icon={<Receipt size={15} />}
            theme='light'
            type='tertiary'
            onClick={onOpenHistory}
            className='ct-topup-panel-action'
          >
            {t('账单')}
          </Button>
          <Button
            icon={<RefreshCw size={15} />}
            theme='light'
            type='tertiary'
            onClick={loadRecentTopups}
            loading={recentLoading}
            className='ct-topup-panel-action'
          >
            {t('刷新')}
          </Button>
        </div>
      </div>

      <div className='ct-topup-balance-strip ct-topup-balance-strip-wide'>
        <RechargeStat
          icon={Wallet}
          label={t('当前余额')}
          value={renderQuota(userState?.user?.quota || 0)}
          tone='teal'
        />
        <RechargeStat
          icon={TrendingUp}
          label={t('历史消耗')}
          value={renderQuota(userState?.user?.used_quota || 0)}
          tone='blue'
        />
        <RechargeStat
          icon={BarChart2}
          label={t('请求次数')}
          value={userState?.user?.request_count || 0}
          tone='green'
        />
        <RechargeStat
          icon={Receipt}
          label={t('本次到账')}
          value={renderQuotaWithAmount(topUpCount || 0)}
          tone='blue'
        />
      </div>

      <div className='ct-topup-checkout-grid'>
        <Card className='ct-topup-panel ct-topup-recharge-panel'>
          <div className='ct-topup-form-shell'>
            {statusLoading ? (
              <div className='ct-topup-loading'>
                <Spin size='large' />
              </div>
            ) : hasOnlineTopUp ? (
              <Form
                getFormApi={(api) => (onlineFormApiRef.current = api)}
                initValues={{ topUpCount: topUpCount }}
              >
                <div className='ct-topup-form-stack'>
                  {amountOptionsAvailable && (
                    <Form.Slot
                      label={
                        <div className='ct-topup-section-label'>
                          <span className='ct-topup-step-badge'>1</span>
                          <span>{t('选择充值金额')}</span>
                          {(() => {
                            const { symbol, rate, type } = getCurrencyConfig();
                            if (type === 'USD') return null;
                            return (
                              <span className='ct-topup-section-hint'>
                                (1 $ = {rate.toFixed(2)} {symbol})
                              </span>
                            );
                          })()}
                        </div>
                      }
                    >
                      <div className='ct-topup-preset-grid ct-topup-preset-grid-checkout'>
                        {showPresetAmounts &&
                          presetAmounts.map((preset, index) => {
                            const discount =
                              preset.discount ||
                              topupInfo?.discount?.[preset.value] ||
                              1.0;
                            const originalPrice = preset.value * priceRatio;
                            const discountedPrice = originalPrice * discount;
                            const hasDiscount = discount < 1.0;
                            const actualPay = discountedPrice;
                            const save = originalPrice - discountedPrice;

                            const { symbol, rate, type } = getCurrencyConfig();
                            const statusStr = localStorage.getItem('status');
                            let usdRate = 7;
                            try {
                              if (statusStr) {
                                const s = JSON.parse(statusStr);
                                usdRate = s?.usd_exchange_rate || 7;
                              }
                            } catch (e) {}

                            let displayValue = preset.value;
                            let displayActualPay = actualPay;
                            let displaySave = save;

                            if (type === 'USD') {
                              displayActualPay = actualPay / usdRate;
                              displaySave = save / usdRate;
                            } else if (type === 'CNY') {
                              displayValue = preset.value * usdRate;
                            } else if (type === 'CUSTOM') {
                              displayValue = preset.value * rate;
                              displayActualPay = (actualPay / usdRate) * rate;
                              displaySave = (save / usdRate) * rate;
                            }

                            return (
                              <button
                                key={index}
                                type='button'
                                className={`ct-topup-preset ${
                                  selectedPresetValue === preset.value
                                    ? 'ct-topup-preset-active'
                                    : ''
                                }`}
                                onClick={() => {
                                  selectPresetAmount(preset);
                                  onlineFormApiRef.current?.setValue(
                                    'topUpCount',
                                    preset.value,
                                  );
                                }}
                              >
                                {hasDiscount && (
                                  <Tag
                                    color='amber'
                                    size='small'
                                    shape='circle'
                                    className='ct-topup-preset-ribbon'
                                  >
                                    {t('推荐')}
                                  </Tag>
                                )}
                                <span className='ct-topup-preset-main'>
                                  <strong>
                                    {formatLargeNumber(displayValue)} {symbol}
                                  </strong>
                                </span>
                                <span className='ct-topup-preset-sub'>
                                  {t('实付')} {symbol}
                                  {displayActualPay.toFixed(2)}
                                  {hasDiscount
                                    ? ` · ${t('节省')} ${symbol}${displaySave.toFixed(2)}`
                                    : ''}
                                </span>
                                {selectedPresetValue === preset.value && (
                                  <CheckCircle2
                                    size={17}
                                    className='ct-topup-preset-check'
                                  />
                                )}
                              </button>
                            );
                          })}

                        <div
                          className={`ct-topup-preset ct-topup-custom-preset ${
                            isCustomAmount ? 'ct-topup-preset-active' : ''
                          }`}
                          onClick={selectCustomAmount}
                          role='button'
                          tabIndex={0}
                          onKeyDown={(event) => {
                            if (event.key === 'Enter' || event.key === ' ') {
                              event.preventDefault();
                              selectCustomAmount();
                            }
                          }}
                        >
                          <span className='ct-topup-preset-main'>
                            <Pencil size={16} />
                            <strong>{t('自定义金额')}</strong>
                          </span>
                          <div
                            className='ct-topup-custom-input'
                            onClick={(event) => event.stopPropagation()}
                          >
                            <Form.InputNumber
                              field='topUpCount'
                              noLabel
                              disabled={!amountOptionsAvailable}
                              placeholder={t('输入金额')}
                              value={topUpCount}
                              min={minTopUp}
                              max={999999999}
                              step={1}
                              precision={0}
                              onFocus={selectCustomAmount}
                              onChange={async (value) => {
                                if (value && value >= 1) {
                                  setTopUpCount(value);
                                  setSelectedPreset(null);
                                  await getAmount(value);
                                }
                              }}
                              onBlur={(e) => {
                                const value = parseInt(e.target.value);
                                if (!value || value < minTopUp) {
                                  setTopUpCount(minTopUp);
                                  getAmount(minTopUp);
                                }
                              }}
                              formatter={(value) => (value ? `${value}` : '')}
                              parser={(value) =>
                                value
                                  ? parseInt(value.replace(/[^\d]/g, ''))
                                  : 0
                              }
                              style={{ width: '100%' }}
                            />
                          </div>
                          <span className='ct-topup-preset-sub'>
                            {t('最低')} {renderQuotaWithAmount(minTopUp)}
                          </span>
                          {isCustomAmount && (
                            <CheckCircle2
                              size={17}
                              className='ct-topup-preset-check'
                            />
                          )}
                        </div>
                      </div>
                    </Form.Slot>
                  )}

                  <div className='ct-topup-payment-stage'>
                    <div className='ct-topup-section-label'>
                      <span className='ct-topup-step-badge'>2</span>
                      <span>{t('选择支付方式')}</span>
                    </div>
                    <div className='ct-topup-payment-choice-grid'>
                      {regularPayMethods.map(renderPaymentButton)}
                      <button
                        type='button'
                        onClick={() => {
                          setSelectedPayment('redeem');
                          redeemShellRef.current?.scrollIntoView({
                            behavior: 'smooth',
                            block: 'nearest',
                          });
                        }}
                        className={`ct-topup-payment-choice ct-topup-payment-choice-redeem ${
                          selectedPayment === 'redeem'
                            ? 'ct-topup-payment-choice-active'
                            : ''
                        }`}
                      >
                        <span>
                          <IconGift />
                        </span>
                        <strong>{t('兑换码')}</strong>
                        <em>{t('输入兑换')}</em>
                      </button>
                    </div>
                  </div>

                  {enableCreemTopUp && creemProducts.length > 0 && (
                    <Form.Slot label={t('Creem 充值')}>
                      <div className='ct-topup-creem-grid'>
                        {creemProducts.map((product, index) => (
                          <button
                            key={index}
                            type='button'
                            onClick={() => creemPreTopUp(product)}
                            className='ct-topup-creem-product'
                          >
                            <strong>{product.name}</strong>
                            <span>
                              {t('充值额度')}: {product.quota}
                            </span>
                            <em>
                              {product.currency === 'EUR' ? '€' : '$'}
                              {product.price}
                            </em>
                          </button>
                        ))}
                      </div>
                    </Form.Slot>
                  )}
                </div>
              </Form>
            ) : (
              <div className='ct-topup-offline-checkout'>
                <Banner
                  type='info'
                  description={t(
                    '管理员未开启在线充值功能，请联系管理员开启或使用兑换码充值。',
                  )}
                  className='ct-topup-inline-banner'
                  closeIcon={null}
                />
                <div className='ct-topup-section-label'>
                  <span className='ct-topup-step-badge'>1</span>
                  <span>{t('兑换码充值')}</span>
                </div>
                <div className='ct-topup-redeem-main-card'>
                  <div className='ct-topup-redeem-main-copy'>
                    <span>
                      <IconGift />
                    </span>
                    <div>
                      <strong>{t('兑换码充值')}</strong>
                      <em>
                        {t(
                          '管理员未开启在线充值功能，请联系管理员开启或使用兑换码充值。',
                        )}
                      </em>
                    </div>
                  </div>
                  {renderRedeemForm()}
                </div>
              </div>
            )}
          </div>
        </Card>

        <Card className='ct-topup-panel ct-topup-order-card'>
          <div className='ct-topup-section-heading'>
            <div>
              <strong>{t('订单确认')}</strong>
              <span>{t('确认金额与支付方式后发起支付')}</span>
            </div>
            <LockKeyhole size={18} />
          </div>
          <div className='ct-topup-order-lines'>
            <div>
              <span>{t('选择金额')}</span>
              <strong>{renderQuotaWithAmount(topUpCount || 0)}</strong>
            </div>
            <div>
              <span>{t('优惠金额')}</span>
              <strong className='ct-topup-order-save'>
                {renderQuotaWithAmount(discountSavings)}
              </strong>
            </div>
            <div>
              <span>{t('实付金额')}</span>
              <strong>{renderAmount()}</strong>
            </div>
          </div>
          <div className='ct-topup-order-arrival'>
            <span>{t('到账金额')}</span>
            <strong>{renderQuotaWithAmount(topUpCount || 0)}</strong>
            <em>
              {selectedDiscount < 1
                ? `${t('已应用折扣')} ${(selectedDiscount * 10).toFixed(1)}${t('折')}`
                : t('当前无折扣')}
            </em>
          </div>
          <div className='ct-topup-order-method'>
            <span>{t('支付方式')}</span>
            <strong>
              {selectedPayment === 'redeem'
                ? t('兑换码')
                : selectedMethod?.name || t('请选择支付方式')}
            </strong>
          </div>
          <Button
            type='primary'
            theme='solid'
            block
            loading={
              selectedPayment === 'redeem'
                ? isSubmitting
                : paymentLoading && payWay === selectedPayment
            }
            disabled={
              !selectedPayment ||
              (selectedPayment !== 'redeem' &&
                (!selectedMethod || getMethodDisabled(selectedMethod)))
            }
            onClick={handlePay}
            icon={selectedPayment === 'redeem' ? <IconGift /> : <Zap size={15} />}
            className='ct-topup-primary-button ct-topup-pay-submit'
          >
            {selectedPayment === 'redeem'
              ? t('兑换额度')
              : `${t('立即支付')} ${renderAmount()}`}
          </Button>
          <Text type='tertiary' size='small' className='ct-topup-order-note'>
            {t('支付即表示您已阅读并同意服务条款')}
          </Text>
        </Card>
      </div>

      <div className='ct-topup-recharge-bottom-grid'>
        <Card className='ct-topup-panel ct-topup-record-panel'>
          <div className='ct-topup-section-heading'>
            <div>
              <strong>{t('充值记录')}</strong>
              <span>{t('展示最近 30 天的充值与订阅支付记录')}</span>
            </div>
            <Button
              theme='borderless'
              type='tertiary'
              onClick={onOpenHistory}
              icon={<ArrowRight size={14} />}
              iconPosition='right'
            >
              {t('查看全部')}
            </Button>
          </div>
          <div className='ct-topup-simple-table'>
            <div className='ct-topup-simple-table-head ct-topup-recharge-table-head'>
              <span>{t('订单号')}</span>
              <span>{t('支付方式')}</span>
              <span>{t('金额')}</span>
              <span>{t('状态')}</span>
              <span>{t('时间')}</span>
            </div>
            {recentTopups.length > 0 ? (
              recentTopups.map((record) => (
                <div
                  className='ct-topup-simple-table-row ct-topup-recharge-table-row'
                  key={record.id}
                >
                  <span>{record.trade_no || '-'}</span>
                  <span>
                    {t(PAYMENT_METHOD_MAP[record.payment_method] || record.payment_method || '-')}
                  </span>
                  <span>{Number(record.money || 0).toFixed(2)}</span>
                  <span>{renderStatus(record.status)}</span>
                  <span>{timestamp2string(record.create_time)}</span>
                </div>
              ))
            ) : (
              <div className='ct-topup-empty-row'>
                <Receipt size={18} />
                <span>{recentLoading ? t('加载中') : t('暂无充值记录')}</span>
              </div>
            )}
          </div>
        </Card>

        <div className='ct-topup-recharge-side-stack'>
          <Card
            className='ct-topup-panel ct-topup-redeem-shell'
            ref={redeemShellRef}
          >
            <div className='ct-topup-redeem-title'>
              <IconGift />
              <Text strong>{t('兑换码充值')}</Text>
            </div>
            {renderRedeemForm()}
          </Card>

          <Card className='ct-topup-panel ct-topup-pay-notes'>
            <div className='ct-topup-section-heading'>
              <div>
                <strong>{t('支付说明')}</strong>
                <span>{t('支付成功后额度将自动入账')}</span>
              </div>
            </div>
            <div className='ct-topup-note-grid'>
              {[
                { icon: ShieldCheck, title: t('安全可靠'), desc: t('多重支付防护') },
                { icon: Zap, title: t('即时到账'), desc: t('支付成功后自动入账') },
                { icon: Sparkles, title: t('赠送优惠'), desc: t('充值可享受额外优惠') },
                { icon: FileText, title: t('可开发票'), desc: t('支持开具普通发票') },
              ].map((item) => {
                const Icon = item.icon;
                return (
                  <div className='ct-topup-note-item' key={item.title}>
                    <Icon size={18} />
                    <strong>{item.title}</strong>
                    <span>{item.desc}</span>
                  </div>
                );
              })}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
};

export default RechargeCard;
