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
import { Banner } from '@douyinfe/semi-ui';
import { IconAlertTriangle } from '@douyinfe/semi-icons';
import {
  AlertTriangle,
  CircleDollarSign,
  Gauge,
  Network,
  Route,
  ShieldCheck,
} from 'lucide-react';
import CardPro from '../../common/ui/CardPro';
import ChannelsTable from './ChannelsTable';
import ChannelsActions from './ChannelsActions';
import ChannelsFilters from './ChannelsFilters';
import ChannelsTabs from './ChannelsTabs';
import { useChannelsData } from '../../../hooks/channels/useChannelsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import BatchTagModal from './modals/BatchTagModal';
import ModelTestModal from './modals/ModelTestModal';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import EditChannelModal from './modals/EditChannelModal';
import EditTagModal from './modals/EditTagModal';
import MultiKeyManageModal from './modals/MultiKeyManageModal';
import ChannelUpstreamUpdateModal from './modals/ChannelUpstreamUpdateModal';
import ChannelGroupSummaryModal from './modals/ChannelGroupSummaryModal';
import { createCardProPagination } from '../../../helpers/utils';
import { CHANNEL_OPTIONS } from '../../../constants';
import {
  isBalanceInsufficientChannel,
  isRecoverableHealthChannel,
} from './channelHealthUtils';

function parseJsonObject(value) {
  if (!value) return {};
  if (typeof value === 'object') return value;
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (error) {
    return {};
  }
}

function flattenChannels(channels = []) {
  const rows = [];
  channels.forEach((channel) => {
    if (Array.isArray(channel.children)) {
      channel.children.forEach((child) => rows.push(child));
      return;
    }
    rows.push(channel);
  });
  return rows;
}

function isBalanceRiskChannel(channel) {
  return isBalanceInsufficientChannel(channel);
}

function isCircuitRiskChannel(channel) {
  return isRecoverableHealthChannel(channel) && !isBalanceRiskChannel(channel);
}

function isPassThroughChannel(channel) {
  const setting = parseJsonObject(channel?.setting);
  return setting?.pass_through_body_enabled === true;
}

function formatNumber(value) {
  return new Intl.NumberFormat('zh-CN').format(Number(value) || 0);
}

function getStatusFilterLabel(statusFilter, t) {
  if (statusFilter === 'disabled') return t('已禁用');
  if (statusFilter === 'all') return t('全部含禁用');
  return t('已启用');
}

function getTypeFilterLabel(activeTypeKey, t) {
  if (!activeTypeKey || activeTypeKey === 'all') return t('全部类型');
  const option = CHANNEL_OPTIONS.find(
    (item) => String(item.value) === String(activeTypeKey),
  );
  return option?.label || activeTypeKey;
}

const AdminChannelMetric = ({
  icon: Icon,
  label,
  value,
  helper,
  tone = 'info',
}) => (
  <article
    className={`ct-admin-channel-metric ct-admin-channel-metric-${tone}`}
  >
    <span className='ct-admin-channel-metric-icon'>
      <Icon size={18} />
    </span>
    <div>
      <div className='ct-admin-channel-metric-label'>{label}</div>
      <div className='ct-admin-channel-metric-value'>{value}</div>
      <div className='ct-admin-channel-metric-helper'>{helper}</div>
    </div>
  </article>
);

const AdminChannelOperationsOverview = ({ channelsData }) => {
  const {
    channels,
    channelCount,
    statusFilter,
    activeTypeKey,
    enableTagMode,
    globalPassThroughEnabled,
    loading,
    t,
  } = channelsData;
  const rows = flattenChannels(channels);
  const enabledRows = rows.filter((channel) => Number(channel.status) === 1);
  const disabledRows = rows.filter((channel) => Number(channel.status) !== 1);
  const balanceRiskRows = rows.filter(isBalanceRiskChannel);
  const circuitRiskRows = rows.filter(isCircuitRiskChannel);
  const passThroughRows = rows.filter(isPassThroughChannel);
  const costConfiguredRows = rows.filter(
    (channel) => channel?.upstream_cost_display?.configured === true,
  );
  const riskCount =
    disabledRows.length + balanceRiskRows.length + circuitRiskRows.length;
  const currentScope = `${getStatusFilterLabel(statusFilter, t)} · ${getTypeFilterLabel(
    activeTypeKey,
    t,
  )}${enableTagMode ? ` · ${t('标签聚合模式')}` : ''}`;

  const metrics = [
    {
      key: 'total',
      icon: Network,
      label: t('当前筛选渠道'),
      value: loading ? t('加载中') : formatNumber(channelCount),
      helper: currentScope,
      tone: 'info',
    },
    {
      key: 'schedulable',
      icon: Gauge,
      label: t('本页可调度'),
      value: `${formatNumber(enabledRows.length)} / ${formatNumber(rows.length)}`,
      helper: t('按当前页渠道状态估算'),
      tone: disabledRows.length > 0 ? 'warning' : 'success',
    },
    {
      key: 'risk',
      icon: AlertTriangle,
      label: t('本页风险项'),
      value: formatNumber(riskCount),
      helper: `${t('余额')} ${formatNumber(balanceRiskRows.length)} · ${t(
        '熔断',
      )} ${formatNumber(circuitRiskRows.length)}`,
      tone: riskCount > 0 ? 'danger' : 'success',
    },
    {
      key: 'cost',
      icon: CircleDollarSign,
      label: t('成本配置'),
      value: `${formatNumber(costConfiguredRows.length)} / ${formatNumber(
        rows.length,
      )}`,
      helper:
        passThroughRows.length > 0 || globalPassThroughEnabled
          ? `${t('透传风险')} ${formatNumber(
              passThroughRows.length + (globalPassThroughEnabled ? 1 : 0),
            )}`
          : t('当前页成本倍率配置情况'),
      tone:
        passThroughRows.length > 0 || globalPassThroughEnabled
          ? 'warning'
          : 'money',
    },
  ];

  const lanes = [
    {
      key: 'identity',
      icon: Network,
      title: t('渠道信息'),
      desc: t('名称、类型、分组、能力'),
    },
    {
      key: 'routing',
      icon: Route,
      title: t('路由状态'),
      desc: t('优先级、权重、可调度状态'),
    },
    {
      key: 'health',
      icon: ShieldCheck,
      title: t('健康状态'),
      desc: t('启停、响应时间、余额风险'),
    },
    {
      key: 'cost',
      icon: CircleDollarSign,
      title: t('成本结算'),
      desc: t('倍率、已用额度、剩余额度'),
    },
  ];

  return (
    <section className='ct-admin-channel-overview'>
      <div className='ct-admin-channel-hero'>
        <div>
          <div className='ct-admin-channel-kicker'>{t('渠道运营')}</div>
          <h1>{t('渠道管理')}</h1>
          <p>
            {t(
              '后台渠道页已按运营视角整理为渠道信息、路由状态、健康状态和成本结算四个区域，保留原有批量操作与测试能力。',
            )}
          </p>
        </div>
        <div className='ct-admin-channel-status'>
          <span>{t('当前视图')}</span>
          <strong>{currentScope}</strong>
        </div>
      </div>

      <div className='ct-admin-channel-metrics'>
        {metrics.map((metric) => (
          <AdminChannelMetric
            key={metric.key}
            icon={metric.icon}
            label={metric.label}
            value={metric.value}
            helper={metric.helper}
            tone={metric.tone}
          />
        ))}
      </div>

      <div className='ct-admin-channel-lanes'>
        {lanes.map((lane) => {
          const Icon = lane.icon;
          return (
            <div className='ct-admin-channel-lane' key={lane.key}>
              <span>
                <Icon size={16} />
              </span>
              <strong>{lane.title}</strong>
              <small>{lane.desc}</small>
            </div>
          );
        })}
      </div>
    </section>
  );
};

const ChannelsPage = ({ variant = 'default' }) => {
  const channelsData = useChannelsData();
  const isMobile = useIsMobile();
  const isAdminVariant = variant === 'admin';

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...channelsData} />
      <EditTagModal
        visible={channelsData.showEditTag}
        tag={channelsData.editingTag}
        handleClose={() => channelsData.setShowEditTag(false)}
        refresh={channelsData.refresh}
      />
      <EditChannelModal
        refresh={channelsData.refresh}
        visible={channelsData.showEdit}
        handleClose={channelsData.closeEdit}
        editingChannel={channelsData.editingChannel}
      />
      <BatchTagModal {...channelsData} />
      <ModelTestModal {...channelsData} />
      <MultiKeyManageModal
        visible={channelsData.showMultiKeyManageModal}
        onCancel={() => channelsData.setShowMultiKeyManageModal(false)}
        channel={channelsData.currentMultiKeyChannel}
        onRefresh={channelsData.refresh}
      />
      <ChannelUpstreamUpdateModal
        visible={channelsData.showUpstreamUpdateModal}
        addModels={channelsData.upstreamUpdateAddModels}
        removeModels={channelsData.upstreamUpdateRemoveModels}
        preferredTab={channelsData.upstreamUpdatePreferredTab}
        confirmLoading={channelsData.upstreamApplyLoading}
        onConfirm={channelsData.applyUpstreamUpdates}
        onCancel={channelsData.closeUpstreamUpdateModal}
      />
      <ChannelGroupSummaryModal
        visible={channelsData.showGroupSummary}
        onCancel={() => channelsData.setShowGroupSummary(false)}
        data={channelsData.groupSummaryData}
        loading={channelsData.groupSummaryLoading}
        onRefresh={channelsData.fetchChannelGroupSummary}
        t={channelsData.t}
      />

      {/* Main Content */}
      {channelsData.globalPassThroughEnabled ? (
        <Banner
          type='warning'
          closeIcon={null}
          icon={
            <IconAlertTriangle
              size='large'
              style={{ color: 'var(--semi-color-warning)' }}
            />
          }
          description={channelsData.t(
            '已开启全局请求透传：参数覆写、模型重定向、渠道适配等 NewAPI 内置功能将失效，非最佳实践；如因此产生问题，请勿提交 issue 反馈。',
          )}
          style={{ marginBottom: 12 }}
        />
      ) : null}
      {isAdminVariant && (
        <AdminChannelOperationsOverview channelsData={channelsData} />
      )}
      <CardPro
        type='type3'
        tabsArea={<ChannelsTabs {...channelsData} />}
        actionsArea={<ChannelsActions {...channelsData} />}
        searchArea={<ChannelsFilters {...channelsData} />}
        paginationArea={createCardProPagination({
          currentPage: channelsData.activePage,
          pageSize: channelsData.pageSize,
          total: channelsData.channelCount,
          onPageChange: channelsData.handlePageChange,
          onPageSizeChange: channelsData.handlePageSizeChange,
          isMobile: isMobile,
          t: channelsData.t,
        })}
        t={channelsData.t}
      >
        <ChannelsTable {...channelsData} />
      </CardPro>
    </>
  );
};

export default ChannelsPage;
