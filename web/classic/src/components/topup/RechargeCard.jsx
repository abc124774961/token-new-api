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

import React, { useRef } from 'react';
import {
  Avatar,
  Typography,
  Card,
  Button,
  Banner,
  Skeleton,
  Form,
  Spin,
  Tooltip,
  Tag,
} from '@douyinfe/semi-ui';
import { SiAlipay, SiWechat, SiStripe } from 'react-icons/si';
import {
  CreditCard,
  Coins,
  Wallet,
  BarChart2,
  TrendingUp,
  Receipt,
  Pencil,
  CheckCircle2,
  Sparkles,
} from 'lucide-react';
import { IconGift } from '@douyinfe/semi-icons';
import { useMinimumLoadingTime } from '../../hooks/common/useMinimumLoadingTime';
import { getCurrencyConfig } from '../../helpers/render';

const { Text } = Typography;

const renderPayMethodIcon = (payMethod) => {
  if (payMethod.type === 'alipay') {
    return <SiAlipay size={18} color='#1677FF' />;
  }
  if (payMethod.type === 'wxpay') {
    return <SiWechat size={18} color='#07C160' />;
  }
  if (payMethod.type === 'stripe') {
    return <SiStripe size={18} color='#635BFF' />;
  }
  if (payMethod.icon) {
    return (
      <img
        src={payMethod.icon}
        alt={payMethod.name}
        style={{
          width: 18,
          height: 18,
          objectFit: 'contain',
        }}
      />
    );
  }
  return (
    <CreditCard
      size={18}
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
  const redeemFormApiRef = useRef(null);
  const showAmountSkeleton = useMinimumLoadingTime(amountLoading);
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

  return (
    <Card className='ct-topup-panel ct-topup-recharge-panel'>
      <div className='ct-topup-panel-head'>
        <div className='ct-topup-title-wrap'>
          <Avatar size='small' className='ct-topup-icon ct-topup-icon-blue'>
            <CreditCard size={16} />
          </Avatar>
          <div>
            <div className='ct-topup-panel-kicker'>{t('额度充值')}</div>
            <Typography.Text className='ct-topup-panel-title'>
              {t('账户充值')}
            </Typography.Text>
            <div className='ct-topup-panel-subtitle'>
              {t('多种充值方式，安全便捷')}
            </div>
          </div>
        </div>
        <Button
          icon={<Receipt size={15} />}
          theme='light'
          type='tertiary'
          onClick={onOpenHistory}
          className='ct-topup-panel-action'
        >
          {t('账单')}
        </Button>
      </div>

      <div className='ct-topup-balance-strip'>
        <RechargeStat
          icon={Wallet}
          label={t('当前余额')}
          value={renderQuota(userState?.user?.quota)}
          tone='teal'
        />
        <RechargeStat
          icon={TrendingUp}
          label={t('历史消耗')}
          value={renderQuota(userState?.user?.used_quota)}
          tone='blue'
        />
        <RechargeStat
          icon={BarChart2}
          label={t('请求次数')}
          value={userState?.user?.request_count || 0}
          tone='green'
        />
      </div>

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
                      <span>{t('选择充值额度')}</span>
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
                  <div className='ct-topup-preset-grid'>
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
                            <span className='ct-topup-preset-main'>
                              <Coins size={16} />
                              <strong>
                                {formatLargeNumber(displayValue)} {symbol}
                              </strong>
                              {hasDiscount && (
                                <Tag color='green' size='small' shape='circle'>
                                  {t('折').includes('off')
                                    ? (
                                        (1 - parseFloat(discount)) *
                                        100
                                      ).toFixed(1)
                                    : (discount * 10).toFixed(1)}
                                  {t('折')}
                                </Tag>
                              )}
                            </span>
                            <span className='ct-topup-preset-sub'>
                              {t('实付')} {symbol}
                              {displayActualPay.toFixed(2)}
                              {hasDiscount
                                ? ` · ${t('节省')} ${symbol}${displaySave.toFixed(2)}`
                                : ` · ${t('节省')} ${symbol}0.00`}
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
                      onClick={() => setSelectedPreset(null)}
                      role='button'
                      tabIndex={0}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter' || event.key === ' ') {
                          event.preventDefault();
                          setSelectedPreset(null);
                        }
                      }}
                    >
                      <span className='ct-topup-preset-main'>
                        <Pencil size={16} />
                        <strong>{t('自定义数量')}</strong>
                        <Tag color='cyan' size='small' shape='circle'>
                          {t('手动输入')}
                        </Tag>
                      </span>
                      <span className='ct-topup-preset-sub'>
                        {t('最低')} {renderQuotaWithAmount(minTopUp)}
                      </span>
                      <div
                        className='ct-topup-custom-input'
                        onClick={(event) => event.stopPropagation()}
                      >
                        <Form.InputNumber
                          field='topUpCount'
                          noLabel
                          disabled={!amountOptionsAvailable}
                          placeholder={
                            t('充值数量，最低 ') +
                            renderQuotaWithAmount(minTopUp)
                          }
                          value={topUpCount}
                          min={minTopUp}
                          max={999999999}
                          step={1}
                          precision={0}
                          onFocus={() => setSelectedPreset(null)}
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
                            value ? parseInt(value.replace(/[^\d]/g, '')) : 0
                          }
                          style={{ width: '100%' }}
                        />
                      </div>
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

              {(enableOnlineTopUp ||
                enableStripeTopUp ||
                enableWaffoTopUp ||
                enableWaffoPancakeTopUp) && (
                <div className='ct-topup-payment-stage'>
                  <div className='ct-topup-payment-stage-head'>
                    <div className='ct-topup-section-label'>
                      <span className='ct-topup-step-badge'>2</span>
                      <span>{t('选择支付方式')}</span>
                    </div>
                    <Skeleton
                      loading={showAmountSkeleton}
                      active
                      placeholder={
                        <Skeleton.Title
                          style={{
                            width: 120,
                            height: 22,
                            borderRadius: 999,
                          }}
                        />
                      }
                    >
                      <div className='ct-topup-pay-summary'>
                        <Sparkles size={14} />
                        <span>{t('实付金额：')}</span>
                        <strong>{renderAmount()}</strong>
                      </div>
                    </Skeleton>
                  </div>

                  {regularPayMethods.length > 0 && (
                    <div className='ct-topup-payment-methods'>
                      {regularPayMethods.map((payMethod) => {
                        const minTopupVal = Number(payMethod.min_topup) || 0;
                        const isStripe = payMethod.type === 'stripe';
                        const isWaffo =
                          typeof payMethod.type === 'string' &&
                          payMethod.type.startsWith('waffo:');
                        const isWaffoPancake =
                          payMethod.type === 'waffo_pancake';
                        const disabled =
                          (!enableOnlineTopUp &&
                            !isStripe &&
                            !isWaffo &&
                            !isWaffoPancake) ||
                          (!enableStripeTopUp && isStripe) ||
                          (!enableWaffoTopUp && isWaffo) ||
                          (!enableWaffoPancakeTopUp && isWaffoPancake) ||
                          minTopupVal > Number(topUpCount || 0);

                        const buttonEl = (
                          <Button
                            key={payMethod.type}
                            theme='outline'
                            type='tertiary'
                            onClick={() => preTopUp(payMethod.type)}
                            disabled={disabled}
                            loading={
                              paymentLoading && payWay === payMethod.type
                            }
                            icon={renderPayMethodIcon(payMethod)}
                            className='ct-topup-payment-button'
                          >
                            <span>{payMethod.name}</span>
                            <em>{t('立即支付')}</em>
                          </Button>
                        );

                        return disabled &&
                          minTopupVal > Number(topUpCount || 0) ? (
                          <Tooltip
                            content={
                              t('此支付方式最低充值金额为') + ' ' + minTopupVal
                            }
                            key={payMethod.type}
                          >
                            {buttonEl}
                          </Tooltip>
                        ) : (
                          <React.Fragment key={payMethod.type}>
                            {buttonEl}
                          </React.Fragment>
                        );
                      })}
                    </div>
                  )}
                </div>
              )}

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
          <Banner
            type='info'
            description={t(
              '管理员未开启在线充值功能，请联系管理员开启或使用兑换码充值。',
            )}
            className='ct-topup-inline-banner'
            closeIcon={null}
          />
        )}
      </div>

      <div className='ct-topup-redeem-shell'>
        <div className='ct-topup-redeem-title'>
          <IconGift />
          <Text strong>{t('兑换码充值')}</Text>
        </div>
        <Form
          getFormApi={(api) => (redeemFormApiRef.current = api)}
          initValues={{ redemptionCode: redemptionCode }}
        >
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
      </div>
    </Card>
  );
};

export default RechargeCard;
