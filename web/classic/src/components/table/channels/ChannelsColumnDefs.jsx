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

import React from 'react';
import {
  Button,
  Dropdown,
  InputNumber,
  Modal,
  Space,
  SplitButtonGroup,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  timestamp2string,
  renderGroup,
  renderQuota,
  getChannelIcon,
  renderQuotaWithAmount,
  showSuccess,
  showError,
  showInfo,
  API,
  renderChannelCapabilities,
} from '../../../helpers';
import {
  CHANNEL_OPTIONS,
  MODEL_FETCHABLE_CHANNEL_TYPES,
} from '../../../constants';
import { parseUpstreamUpdateMeta } from '../../../hooks/channels/upstreamUpdateUtils';
import {
  IconTreeTriangleDown,
  IconMore,
  IconAlertTriangle,
} from '@douyinfe/semi-icons';
import { FaRandom } from 'react-icons/fa';

// Render functions
const renderType = (type, record = {}, t) => {
  const channelInfo = record?.channel_info;
  let type2label = new Map();
  for (let i = 0; i < CHANNEL_OPTIONS.length; i++) {
    type2label[CHANNEL_OPTIONS[i].value] = CHANNEL_OPTIONS[i];
  }
  type2label[0] = { value: 0, label: t('未知类型'), color: 'grey' };

  let icon = getChannelIcon(type);

  if (channelInfo?.is_multi_key) {
    icon =
      channelInfo?.multi_key_mode === 'random' ? (
        <div className='flex items-center gap-1'>
          <FaRandom className='text-blue-500' />
          {icon}
        </div>
      ) : (
        <div className='flex items-center gap-1'>
          <IconTreeTriangleDown className='text-blue-500' />
          {icon}
        </div>
      );
  }

  const typeTag = (
    <Tag color={type2label[type]?.color} shape='circle' prefixIcon={icon}>
      {type2label[type]?.label}
    </Tag>
  );

  let ionetMeta = null;
  if (record?.other_info) {
    try {
      const parsed = JSON.parse(record.other_info);
      if (parsed && typeof parsed === 'object' && parsed.source === 'ionet') {
        ionetMeta = parsed;
      }
    } catch (error) {
      // ignore invalid metadata
    }
  }

  if (!ionetMeta) {
    return typeTag;
  }

  const handleNavigate = (event) => {
    event?.stopPropagation?.();
    if (!ionetMeta?.deployment_id) {
      return;
    }
    const targetUrl = `/console/deployment?deployment_id=${ionetMeta.deployment_id}`;
    window.open(targetUrl, '_blank', 'noopener');
  };

  return (
    <Space spacing={6}>
      {typeTag}
      <Tooltip
        content={
          <div className='max-w-xs'>
            <div className='text-xs text-gray-600'>
              {t('来源于 IO.NET 部署')}
            </div>
            {ionetMeta?.deployment_id && (
              <div className='text-xs text-gray-500 mt-1'>
                {t('部署 ID')}: {ionetMeta.deployment_id}
              </div>
            )}
          </div>
        }
      >
        <span>
          <Tag
            color='purple'
            type='light'
            className='cursor-pointer'
            onClick={handleNavigate}
          >
            IO.NET
          </Tag>
        </span>
      </Tooltip>
    </Space>
  );
};

const renderTagType = (t) => {
  return (
    <Tag color='light-blue' shape='circle' type='light'>
      {t('标签聚合')}
    </Tag>
  );
};

const renderStatus = (status, channelInfo = undefined, t) => {
  if (channelInfo) {
    if (channelInfo.is_multi_key) {
      let keySize = channelInfo.multi_key_size;
      let enabledKeySize = keySize;
      if (channelInfo.multi_key_status_list) {
        enabledKeySize =
          keySize - Object.keys(channelInfo.multi_key_status_list).length;
      }
      return renderMultiKeyStatus(status, keySize, enabledKeySize, t);
    }
  }
  switch (status) {
    case 1:
      return (
        <Tag color='green' shape='circle'>
          {t('已启用')}
        </Tag>
      );
    case 2:
      return (
        <Tag color='red' shape='circle'>
          {t('已禁用')}
        </Tag>
      );
    case 3:
      return (
        <Tag color='yellow' shape='circle'>
          {t('自动禁用')}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {t('未知状态')}
        </Tag>
      );
  }
};

const renderMultiKeyStatus = (status, keySize, enabledKeySize, t) => {
  switch (status) {
    case 1:
      return (
        <Tag color='green' shape='circle'>
          {t('已启用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    case 2:
      return (
        <Tag color='red' shape='circle'>
          {t('已禁用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    case 3:
      return (
        <Tag color='yellow' shape='circle'>
          {t('自动禁用')} {enabledKeySize}/{keySize}
        </Tag>
      );
    default:
      return (
        <Tag color='grey' shape='circle'>
          {t('未知状态')} {enabledKeySize}/{keySize}
        </Tag>
      );
  }
};

const parseChannelOtherInfo = (otherInfo) => {
  if (!otherInfo) return {};
  if (typeof otherInfo === 'object') return otherInfo;
  try {
    const parsed = JSON.parse(otherInfo);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (error) {
    return {};
  }
};

const formatPauseDuration = (seconds, t) => {
  const total = Math.max(0, Math.ceil(Number(seconds) || 0));
  if (total <= 0) return '';
  if (total < 60) return `${total}${t('秒')}`;
  const minutes = Math.floor(total / 60);
  const remainingSeconds = total % 60;
  if (minutes < 60) {
    return remainingSeconds > 0
      ? `${minutes}${t('分钟')} ${remainingSeconds}${t('秒')}`
      : `${minutes}${t('分钟')}`;
  }
  const hours = Math.floor(minutes / 60);
  const remainingMinutes = minutes % 60;
  return remainingMinutes > 0
    ? `${hours}${t('小时')} ${remainingMinutes}${t('分钟')}`
    : `${hours}${t('小时')}`;
};

const getPauseRemainingSeconds = (pauseStatus, nowSeconds) => {
  if (!pauseStatus?.active) return 0;
  if (pauseStatus.until) {
    return Math.max(0, Number(pauseStatus.until) - nowSeconds);
  }
  return Math.max(0, Number(pauseStatus.remaining_seconds) || 0);
};

const renderStatusReason = (reason, t) => {
  if (!reason) return '';
  const normalized = String(reason).trim().toLowerCase();
  if (
    normalized === 'balance_insufficient' ||
    normalized.includes('余额不足')
  ) {
    return t('余额不足');
  }
  if (normalized === 'all keys are disabled') {
    return t('账号池为空或全部账号已禁用');
  }
  return reason;
};

const isBalanceInsufficientChannel = (record) => {
  if (record?.balance_insufficient === true) return true;
  const statusReason = String(
    record?.status_reason ||
      parseChannelOtherInfo(record?.other_info).status_reason ||
      '',
  )
    .trim()
    .toLowerCase();
  return (
    statusReason === 'balance_insufficient' || statusReason.includes('余额不足')
  );
};

const ChannelStatusCell = ({ record, t, refresh }) => {
  const [nowSeconds, setNowSeconds] = React.useState(() =>
    Math.floor(Date.now() / 1000),
  );
  const [clearingCircuit, setClearingCircuit] = React.useState(false);
  const failureAvoidance = record?.failure_avoidance;
  const concurrencyCooldown = record?.concurrency_cooldown;
  const circuitRemaining = getPauseRemainingSeconds(
    failureAvoidance,
    nowSeconds,
  );
  const cooldownRemaining = getPauseRemainingSeconds(
    concurrencyCooldown,
    nowSeconds,
  );
  const circuitActive =
    failureAvoidance?.active === true && circuitRemaining > 0;
  const runtimeCircuit = record?.runtime_circuit || {};
  const runtimeCircuitOpen = Number(runtimeCircuit.open_runtime_keys || 0);
  const runtimeCircuitHalfOpen = Number(
    runtimeCircuit.half_open_runtime_keys || 0,
  );
  const runtimeCircuitOpenActive = runtimeCircuitOpen > 0;
  const runtimeCircuitHalfOpenActive = runtimeCircuitHalfOpen > 0;
  const runtimeCircuitActive =
    runtimeCircuitOpenActive || runtimeCircuitHalfOpenActive;
  const anyCircuitActive = circuitActive || runtimeCircuitActive;
  const cooldownActive =
    concurrencyCooldown?.active === true && cooldownRemaining > 0;

  React.useEffect(() => {
    if (!anyCircuitActive && !cooldownActive) return undefined;
    const timer = window.setInterval(() => {
      setNowSeconds(Math.floor(Date.now() / 1000));
    }, 1000);
    return () => window.clearInterval(timer);
  }, [anyCircuitActive, cooldownActive]);

  const otherInfo = parseChannelOtherInfo(record?.other_info);
  const reason = renderStatusReason(
    concurrencyCooldown?.reason ||
      failureAvoidance?.reason ||
      otherInfo.status_reason,
    t,
  );
  const statusTime = otherInfo.status_time
    ? timestamp2string(otherInfo.status_time)
    : '';
  const cooldownLabel = formatPauseDuration(cooldownRemaining, t);
  const circuitLabel = formatPauseDuration(circuitRemaining, t);
  const cooldownUntil = concurrencyCooldown?.until
    ? timestamp2string(concurrencyCooldown.until)
    : '';
  const circuitUntil = failureAvoidance?.until
    ? timestamp2string(failureAvoidance.until)
    : '';

  const clearCircuit = async (event) => {
    event.stopPropagation();
    if (clearingCircuit || !record?.id) return;
    setClearingCircuit(true);
    try {
      const res = await API.post(
        '/api/model_gateway/observability/runtime/clear_circuit',
        {
          channel_id: record.id,
          clear_failure_avoidance: true,
        },
      );
      if (res?.data?.success) {
        showSuccess(t('熔断已恢复'));
        refresh?.();
      } else {
        showError(res?.data?.message || t('恢复熔断失败'));
      }
    } catch (error) {
      showError(error);
    } finally {
      setClearingCircuit(false);
    }
  };

  const statusNode = isBalanceInsufficientChannel(record) ? (
    <Tag color='red' shape='circle'>
      {t('余额不足')}
    </Tag>
  ) : cooldownActive || anyCircuitActive ? (
    <Space spacing={4} wrap>
      {cooldownActive && (
        <Tag color='yellow' shape='circle'>
          {t('冷却中')} {cooldownLabel}
        </Tag>
      )}
      {circuitActive && (
        <Tag color='orange' shape='circle'>
          {t('熔断中')} {circuitLabel}
        </Tag>
      )}
      {runtimeCircuitOpenActive && (
        <Tag color='red' shape='circle'>
          {t('运行态熔断 {{count}}', { count: runtimeCircuitOpen })}
        </Tag>
      )}
      {runtimeCircuitHalfOpenActive && (
        <Tag color='cyan' shape='circle'>
          {t('半开探测')} {runtimeCircuitHalfOpen}
        </Tag>
      )}
      {anyCircuitActive && (
        <Button
          size='small'
          type='tertiary'
          loading={clearingCircuit}
          onClick={clearCircuit}
        >
          {t('解除')}
        </Button>
      )}
    </Space>
  ) : (
    renderStatus(record?.status, record?.channel_info, t)
  );

  const tooltipLines = [];
  if (cooldownActive) {
    tooltipLines.push(`${t('冷却剩余')}: ${cooldownLabel}`);
  }
  if (cooldownUntil) {
    tooltipLines.push(`${t('冷却到')}: ${cooldownUntil}`);
  }
  if (circuitActive) {
    tooltipLines.push(`${t('熔断剩余')}: ${circuitLabel}`);
  }
  if (circuitUntil) {
    tooltipLines.push(`${t('熔断到')}: ${circuitUntil}`);
  }
  if (runtimeCircuitOpen > 0) {
    tooltipLines.push(`${t('运行态熔断')}: ${runtimeCircuitOpen}`);
  }
  if (runtimeCircuitHalfOpen > 0) {
    tooltipLines.push(`${t('半开探测')}: ${runtimeCircuitHalfOpen}`);
  }
  if (reason) {
    tooltipLines.push(`${t('原因：')}${reason}`);
  }
  if (statusTime) {
    tooltipLines.push(`${t('时间：')}${statusTime}`);
  }
  if (concurrencyCooldown?.failure_count) {
    tooltipLines.push(
      `${t('冷却失败次数')}: ${concurrencyCooldown.failure_count}`,
    );
  }
  if (failureAvoidance?.failure_count) {
    tooltipLines.push(`${t('熔断次数')}: ${failureAvoidance.failure_count}`);
  }
  if (concurrencyCooldown?.success_streak) {
    tooltipLines.push(
      `${t('恢复连续成功')}: ${concurrencyCooldown.success_streak}`,
    );
  }

  if (!tooltipLines.length) return statusNode;

  return (
    <Tooltip
      content={
        <div className='flex flex-col gap-1 max-w-xs'>
          {tooltipLines.map((line) => (
            <div key={line}>{line}</div>
          ))}
        </div>
      }
    >
      <div className='inline-flex'>{statusNode}</div>
    </Tooltip>
  );
};

const renderResponseTime = (responseTime, t) => {
  let time = responseTime / 1000;
  time = time.toFixed(2) + t(' 秒');
  if (responseTime === 0) {
    return (
      <Tag color='grey' shape='circle'>
        {t('未测试')}
      </Tag>
    );
  } else if (responseTime <= 1000) {
    return (
      <Tag color='green' shape='circle'>
        {time}
      </Tag>
    );
  } else if (responseTime <= 3000) {
    return (
      <Tag color='lime' shape='circle'>
        {time}
      </Tag>
    );
  } else if (responseTime <= 5000) {
    return (
      <Tag color='yellow' shape='circle'>
        {time}
      </Tag>
    );
  } else {
    return (
      <Tag color='red' shape='circle'>
        {time}
      </Tag>
    );
  }
};

const isRequestPassThroughEnabled = (record) => {
  if (!record || record.children !== undefined) {
    return false;
  }
  const settingValue = record.setting;
  if (!settingValue) {
    return false;
  }
  if (typeof settingValue === 'object') {
    return settingValue.pass_through_body_enabled === true;
  }
  if (typeof settingValue !== 'string') {
    return false;
  }
  try {
    const parsed = JSON.parse(settingValue);
    return parsed?.pass_through_body_enabled === true;
  } catch (error) {
    return false;
  }
};

const getUpstreamUpdateMeta = (record) => {
  const supported =
    !!record &&
    record.children === undefined &&
    MODEL_FETCHABLE_CHANNEL_TYPES.has(record.type);
  if (!record || record.children !== undefined) {
    return {
      supported: false,
      enabled: false,
      pendingAddModels: [],
      pendingRemoveModels: [],
    };
  }
  const parsed =
    record?.upstreamUpdateMeta && typeof record.upstreamUpdateMeta === 'object'
      ? record.upstreamUpdateMeta
      : parseUpstreamUpdateMeta(record?.settings);
  return {
    supported,
    enabled: parsed?.enabled === true,
    pendingAddModels: Array.isArray(parsed?.pendingAddModels)
      ? parsed.pendingAddModels
      : [],
    pendingRemoveModels: Array.isArray(parsed?.pendingRemoveModels)
      ? parsed.pendingRemoveModels
      : [],
  };
};

const trimFixedNumber = (value, precision) => {
  const numberValue = Number(value || 0);
  if (!Number.isFinite(numberValue) || numberValue <= 0) {
    return '0';
  }
  return numberValue.toFixed(precision).replace(/0+$/, '').replace(/\.$/, '');
};

const formatChannelCostRatio = (value) => {
  const numberValue = Number(value || 0);
  if (!Number.isFinite(numberValue) || numberValue <= 0) {
    return '0x';
  }
  return `${trimFixedNumber(numberValue, numberValue < 0.01 ? 6 : 4)}x`;
};

const formatChannelCostPrice = (value) => {
  const numberValue = Number(value || 0);
  if (!Number.isFinite(numberValue) || numberValue <= 0) {
    return '$0';
  }
  return `$${trimFixedNumber(numberValue, numberValue < 1 ? 6 : 4)}`;
};

const getPrimaryChannelCostRatio = (display) => {
  if (!display) return 0;
  const candidates = [
    display.actual_token_multiplier,
    display.cost_coefficient,
    display.fee_multiplier,
    display.token_multiplier,
    display.base_cost_multiplier,
    display.recharge_multiplier,
  ];
  for (const value of candidates) {
    const numberValue = Number(value || 0);
    if (Number.isFinite(numberValue) && numberValue > 0) {
      return numberValue;
    }
  }
  return 0;
};

const buildChannelCostPriceRows = (display, t) => {
  if (!display) return [];
  if (display.pricing_mode === 'request') {
    return [
      {
        key: 'request',
        label: t('按次'),
        value: display.request_price,
        suffix: '',
      },
    ].filter((item) => Number(item.value || 0) > 0);
  }
  return [
    {
      key: 'input',
      label: t('输入'),
      shortLabel: 'I',
      value: display.input_per_million,
    },
    {
      key: 'output',
      label: t('输出'),
      shortLabel: 'O',
      value: display.output_per_million,
    },
    {
      key: 'cache_read',
      label: t('缓存读'),
      shortLabel: 'C',
      value: display.cache_read_per_million,
    },
    {
      key: 'cache_write',
      label: t('缓存写'),
      shortLabel: 'W',
      value: display.cache_write_per_million,
    },
  ].filter((item) => Number(item.value || 0) > 0);
};

const renderChannelCostDisplay = (record, t) => {
  if (record?.children !== undefined) {
    return <Typography.Text type='tertiary'>-</Typography.Text>;
  }
  const display = record?.upstream_cost_display;
  if (!display?.configured) {
    return (
      <Tag color='grey' type='light' size='small' shape='circle'>
        {t('未配置')}
      </Tag>
    );
  }

  const rows = buildChannelCostPriceRows(display, t);
  const hasPrices = display.price_configured && rows.length > 0;
  const primaryRatio = getPrimaryChannelCostRatio(display);
  const ratioText = formatChannelCostRatio(primaryRatio);

  const tooltipContent = (
    <div className='flex flex-col gap-1 min-w-[180px] text-xs'>
      {display.model ? (
        <div>
          {t('模型')}: {display.model}
        </div>
      ) : null}
      {display.pricing_model && display.pricing_model !== display.model ? (
        <div>
          {t('定价模型')}: {display.pricing_model}
        </div>
      ) : null}
      {display.pricing_mode === 'request' ? (
        <div>
          {t('1:1 实际成本倍率')}: {ratioText}
        </div>
      ) : (
        <>
          <div>
            {t('成本系数')}: {formatChannelCostRatio(display.cost_coefficient)}
          </div>
          <div>
            {t('费用计算倍率')}:{' '}
            {formatChannelCostRatio(display.fee_multiplier)}
          </div>
          <div>
            {t('1:1 实际成本倍率')}: {ratioText}
          </div>
        </>
      )}
      {rows.map((row) => (
        <div key={row.key}>
          {row.label}: {formatChannelCostPrice(row.value)}
          {row.suffix === '' ? '' : '/M'}
        </div>
      ))}
      {!hasPrices ? <div>{t('模型定价未配置')}</div> : null}
    </div>
  );

  return (
    <Tooltip content={tooltipContent} position='topLeft'>
      <div
        className={`ct-channel-cost-ratio ${hasPrices ? '' : 'ct-channel-cost-ratio-muted'}`}
      >
        <span className='ct-channel-cost-ratio-badge'>1:1</span>
        <span className='ct-channel-cost-ratio-value'>{ratioText}</span>
      </div>
    </Tooltip>
  );
};

export const getChannelsColumns = ({
  t,
  COLUMN_KEYS,
  updateChannelBalance,
  manageChannel,
  manageTag,
  submitTagEdit,
  testChannel,
  setCurrentTestChannel,
  setShowModelTestModal,
  setEditingChannel,
  setShowEdit,
  setShowEditTag,
  setEditingTag,
  copySelectedChannel,
  refresh,
  activePage,
  channels,
  checkOllamaVersion,
  setShowMultiKeyManageModal,
  setCurrentMultiKeyChannel,
  openUpstreamUpdateModal,
  detectChannelUpstreamUpdates,
}) => {
  return [
    {
      key: COLUMN_KEYS.ID,
      title: t('ID'),
      dataIndex: 'id',
    },
    {
      key: COLUMN_KEYS.NAME,
      title: t('名称'),
      dataIndex: 'name',
      render: (text, record, index) => {
        const passThroughEnabled = isRequestPassThroughEnabled(record);
        const upstreamUpdateMeta = getUpstreamUpdateMeta(record);
        const pendingAddCount = upstreamUpdateMeta.pendingAddModels.length;
        const pendingRemoveCount =
          upstreamUpdateMeta.pendingRemoveModels.length;
        const showUpstreamUpdateTag =
          upstreamUpdateMeta.supported &&
          upstreamUpdateMeta.enabled &&
          (pendingAddCount > 0 || pendingRemoveCount > 0);
        const balanceInsufficient = isBalanceInsufficientChannel(record);
        const nameNode =
          record.remark && record.remark.trim() !== '' ? (
            <Tooltip
              content={
                <div className='flex flex-col gap-2 max-w-xs'>
                  <div className='text-sm'>{record.remark}</div>
                  <Button
                    size='small'
                    type='primary'
                    theme='outline'
                    onClick={(e) => {
                      e.stopPropagation();
                      navigator.clipboard
                        .writeText(record.remark)
                        .then(() => {
                          showSuccess(t('复制成功'));
                        })
                        .catch(() => {
                          showError(t('复制失败'));
                        });
                    }}
                  >
                    {t('复制')}
                  </Button>
                </div>
              }
              trigger='hover'
              position='topLeft'
            >
              <span>{text}</span>
            </Tooltip>
          ) : (
            <span>{text}</span>
          );

        if (
          !passThroughEnabled &&
          !showUpstreamUpdateTag &&
          !balanceInsufficient
        ) {
          return nameNode;
        }

        return (
          <Space spacing={6} align='center'>
            {nameNode}
            {passThroughEnabled && (
              <Tooltip
                content={t(
                  '该渠道已开启请求透传：参数覆写、模型重定向、渠道适配等 NewAPI 内置功能将失效，非最佳实践；如因此产生问题，请勿提交 issue 反馈。',
                )}
                trigger='hover'
                position='topLeft'
              >
                <span className='inline-flex items-center'>
                  <IconAlertTriangle
                    style={{ color: 'var(--semi-color-warning)' }}
                  />
                </span>
              </Tooltip>
            )}
            {balanceInsufficient && (
              <Tooltip content={t('渠道余额不足，已暂停调度')} position='top'>
                <Tag color='red' type='light' size='small' shape='circle'>
                  {t('余额不足')}
                </Tag>
              </Tooltip>
            )}
            {showUpstreamUpdateTag && (
              <Space spacing={4} align='center'>
                {pendingAddCount > 0 ? (
                  <Tooltip content={t('点击处理新增模型')} position='top'>
                    <Tag
                      color='green'
                      type='light'
                      size='small'
                      shape='circle'
                      className='cursor-pointer transition-all duration-150 hover:opacity-85 hover:-translate-y-px active:scale-95'
                      onClick={(e) => {
                        e.stopPropagation();
                        openUpstreamUpdateModal(
                          record,
                          upstreamUpdateMeta.pendingAddModels,
                          upstreamUpdateMeta.pendingRemoveModels,
                          'add',
                        );
                      }}
                    >
                      +{pendingAddCount}
                    </Tag>
                  </Tooltip>
                ) : null}
                {pendingRemoveCount > 0 ? (
                  <Tooltip content={t('点击处理删除模型')} position='top'>
                    <Tag
                      color='red'
                      type='light'
                      size='small'
                      shape='circle'
                      className='cursor-pointer transition-all duration-150 hover:opacity-85 hover:-translate-y-px active:scale-95'
                      onClick={(e) => {
                        e.stopPropagation();
                        openUpstreamUpdateModal(
                          record,
                          upstreamUpdateMeta.pendingAddModels,
                          upstreamUpdateMeta.pendingRemoveModels,
                          'remove',
                        );
                      }}
                    >
                      -{pendingRemoveCount}
                    </Tag>
                  </Tooltip>
                ) : null}
              </Space>
            )}
          </Space>
        );
      },
    },
    {
      key: COLUMN_KEYS.GROUP,
      title: t('分组'),
      dataIndex: 'group',
      render: (text, record, index) => (
        <div>
          <Space spacing={2}>
            {text
              ?.split(',')
              .sort((a, b) => {
                if (a === 'default') return -1;
                if (b === 'default') return 1;
                return a.localeCompare(b);
              })
              .map((item, index) => renderGroup(item))}
          </Space>
        </div>
      ),
    },
    {
      key: COLUMN_KEYS.COST,
      title: t('倍率'),
      dataIndex: 'upstream_cost_display',
      width: 108,
      render: (text, record) => renderChannelCostDisplay(record, t),
    },
    {
      key: COLUMN_KEYS.TYPE,
      title: t('类型'),
      dataIndex: 'type',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return <>{renderType(text, record, t)}</>;
        } else {
          return <>{renderTagType(t)}</>;
        }
      },
    },
    {
      key: COLUMN_KEYS.CAPABILITIES,
      title: t('能力'),
      dataIndex: 'capabilities',
      width: 190,
      render: (text, record, index) => renderChannelCapabilities(record, t),
    },
    {
      key: COLUMN_KEYS.STATUS,
      title: t('状态'),
      dataIndex: 'status',
      render: (text, record, index) => (
        <ChannelStatusCell record={record} t={t} refresh={refresh} />
      ),
    },
    {
      key: COLUMN_KEYS.RESPONSE_TIME,
      title: t('响应时间'),
      dataIndex: 'response_time',
      render: (text, record, index) => <div>{renderResponseTime(text, t)}</div>,
    },
    {
      key: COLUMN_KEYS.BALANCE,
      title: t('已用/剩余'),
      dataIndex: 'expired_time',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <Space spacing={1}>
                <Tooltip content={t('已用额度')}>
                  <Tag color='white' type='ghost' shape='circle'>
                    {renderQuota(record.used_quota)}
                  </Tag>
                </Tooltip>
                <Tooltip
                  content={
                    record.type === 57
                      ? t('查看 Codex 帐号信息与用量')
                      : t('剩余额度') +
                        ': ' +
                        renderQuotaWithAmount(record.balance) +
                        t('，点击更新')
                  }
                >
                  <Tag
                    color={record.type === 57 ? 'light-blue' : 'white'}
                    type={record.type === 57 ? 'light' : 'ghost'}
                    shape='circle'
                    className={record.type === 57 ? 'cursor-pointer' : ''}
                    onClick={() => updateChannelBalance(record)}
                  >
                    {record.type === 57
                      ? t('帐号信息')
                      : renderQuotaWithAmount(record.balance)}
                  </Tag>
                </Tooltip>
              </Space>
            </div>
          );
        } else {
          return (
            <Tooltip content={t('已用额度')}>
              <Tag color='white' type='ghost' shape='circle'>
                {renderQuota(record.used_quota)}
              </Tag>
            </Tooltip>
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.PRIORITY,
      title: t('优先级'),
      dataIndex: 'priority',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <InputNumber
                style={{ width: 70 }}
                name='priority'
                onBlur={(e) => {
                  manageChannel(record.id, 'priority', record, e.target.value);
                }}
                keepFocus={true}
                innerButtons
                defaultValue={record.priority}
                min={-999}
                size='small'
              />
            </div>
          );
        } else {
          return (
            <InputNumber
              style={{ width: 70 }}
              name='priority'
              keepFocus={true}
              onBlur={(e) => {
                Modal.warning({
                  title: t('修改子渠道优先级'),
                  content:
                    t('确定要修改所有子渠道优先级为 ') +
                    e.target.value +
                    t(' 吗？'),
                  onOk: () => {
                    if (e.target.value === '') {
                      return;
                    }
                    submitTagEdit('priority', {
                      tag: record.key,
                      priority: e.target.value,
                    });
                  },
                });
              }}
              innerButtons
              defaultValue={record.priority}
              min={-999}
              size='small'
            />
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.WEIGHT,
      title: t('权重'),
      dataIndex: 'weight',
      render: (text, record, index) => {
        if (record.children === undefined) {
          return (
            <div>
              <InputNumber
                style={{ width: 70 }}
                name='weight'
                onBlur={(e) => {
                  manageChannel(record.id, 'weight', record, e.target.value);
                }}
                keepFocus={true}
                innerButtons
                defaultValue={record.weight}
                min={0}
                size='small'
              />
            </div>
          );
        } else {
          return (
            <InputNumber
              style={{ width: 70 }}
              name='weight'
              keepFocus={true}
              onBlur={(e) => {
                Modal.warning({
                  title: t('修改子渠道权重'),
                  content:
                    t('确定要修改所有子渠道权重为 ') +
                    e.target.value +
                    t(' 吗？'),
                  onOk: () => {
                    if (e.target.value === '') {
                      return;
                    }
                    submitTagEdit('weight', {
                      tag: record.key,
                      weight: e.target.value,
                    });
                  },
                });
              }}
              innerButtons
              defaultValue={record.weight}
              min={-999}
              size='small'
            />
          );
        }
      },
    },
    {
      key: COLUMN_KEYS.OPERATE,
      title: '',
      dataIndex: 'operate',
      fixed: 'right',
      render: (text, record, index) => {
        if (record.children === undefined) {
          const upstreamUpdateMeta = getUpstreamUpdateMeta(record);
          const moreMenuItems = [
            {
              node: 'item',
              name: t('账号管理'),
              type: 'tertiary',
              onClick: () => {
                window.location.href = `/console/channel/accounts?channel_id=${record.id}`;
              },
            },
            {
              node: 'item',
              name: t('删除'),
              type: 'danger',
              onClick: () => {
                Modal.confirm({
                  title: t('确定是否要删除此渠道？'),
                  content: t('此修改将不可逆'),
                  onOk: () => {
                    (async () => {
                      await manageChannel(record.id, 'delete', record);
                      await refresh();
                      setTimeout(() => {
                        if (channels.length === 0 && activePage > 1) {
                          refresh(activePage - 1);
                        }
                      }, 100);
                    })();
                  },
                });
              },
            },
            {
              node: 'item',
              name: t('复制'),
              type: 'tertiary',
              onClick: () => {
                Modal.confirm({
                  title: t('确定是否要复制此渠道？'),
                  content: t('复制渠道的所有信息'),
                  onOk: () => copySelectedChannel(record),
                });
              },
            },
          ];

          if (upstreamUpdateMeta.supported) {
            moreMenuItems.push({
              node: 'item',
              name: t('仅检测上游模型更新'),
              type: 'tertiary',
              onClick: () => {
                detectChannelUpstreamUpdates(record);
              },
            });
            moreMenuItems.push({
              node: 'item',
              name: t('处理上游模型更新'),
              type: 'tertiary',
              onClick: () => {
                if (!upstreamUpdateMeta.enabled) {
                  showInfo(t('该渠道未开启上游模型更新检测'));
                  return;
                }
                if (
                  upstreamUpdateMeta.pendingAddModels.length === 0 &&
                  upstreamUpdateMeta.pendingRemoveModels.length === 0
                ) {
                  showInfo(t('该渠道暂无可处理的上游模型更新'));
                  return;
                }
                openUpstreamUpdateModal(
                  record,
                  upstreamUpdateMeta.pendingAddModels,
                  upstreamUpdateMeta.pendingRemoveModels,
                  upstreamUpdateMeta.pendingAddModels.length > 0
                    ? 'add'
                    : 'remove',
                );
              },
            });
          }

          if (record.type === 4) {
            moreMenuItems.unshift({
              node: 'item',
              name: t('测活'),
              type: 'tertiary',
              onClick: () => checkOllamaVersion(record),
            });
          }

          return (
            <Space wrap>
              <SplitButtonGroup
                className='overflow-hidden'
                aria-label={t('测试单个渠道操作项目组')}
              >
                <Button
                  size='small'
                  type='tertiary'
                  onClick={() => testChannel(record, '')}
                >
                  {t('测试')}
                </Button>
                <Button
                  size='small'
                  type='tertiary'
                  icon={<IconTreeTriangleDown />}
                  onClick={() => {
                    setCurrentTestChannel(record);
                    setShowModelTestModal(true);
                  }}
                />
              </SplitButtonGroup>

              {record.status === 1 ? (
                <Button
                  type='danger'
                  size='small'
                  onClick={() => manageChannel(record.id, 'disable', record)}
                >
                  {t('禁用')}
                </Button>
              ) : (
                <Button
                  size='small'
                  onClick={() => manageChannel(record.id, 'enable', record)}
                >
                  {t('启用')}
                </Button>
              )}

              {record.channel_info?.is_multi_key ? (
                <SplitButtonGroup aria-label={t('多密钥渠道操作项目组')}>
                  <Button
                    type='tertiary'
                    size='small'
                    onClick={() => {
                      setEditingChannel(record);
                      setShowEdit(true);
                    }}
                  >
                    {t('编辑')}
                  </Button>
                  <Dropdown
                    trigger='click'
                    position='bottomRight'
                    menu={[
                      {
                        node: 'item',
                        name: t('多密钥管理'),
                        onClick: () => {
                          setCurrentMultiKeyChannel(record);
                          setShowMultiKeyManageModal(true);
                        },
                      },
                    ]}
                  >
                    <Button
                      type='tertiary'
                      size='small'
                      icon={<IconTreeTriangleDown />}
                    />
                  </Dropdown>
                </SplitButtonGroup>
              ) : (
                <Button
                  type='tertiary'
                  size='small'
                  onClick={() => {
                    setEditingChannel(record);
                    setShowEdit(true);
                  }}
                >
                  {t('编辑')}
                </Button>
              )}

              <Dropdown
                trigger='click'
                position='bottomRight'
                menu={moreMenuItems}
              >
                <Button icon={<IconMore />} type='tertiary' size='small' />
              </Dropdown>
            </Space>
          );
        } else {
          // 标签操作按钮
          return (
            <Space wrap>
              <Button
                type='tertiary'
                size='small'
                onClick={() => manageTag(record.key, 'enable')}
              >
                {t('启用全部')}
              </Button>
              <Button
                type='tertiary'
                size='small'
                onClick={() => manageTag(record.key, 'disable')}
              >
                {t('禁用全部')}
              </Button>
              <Button
                type='tertiary'
                size='small'
                onClick={() => {
                  setShowEditTag(true);
                  setEditingTag(record.key);
                }}
              >
                {t('编辑')}
              </Button>
            </Space>
          );
        }
      },
    },
  ];
};
