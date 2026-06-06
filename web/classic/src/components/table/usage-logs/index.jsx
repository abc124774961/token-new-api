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
import LogsTable from './UsageLogsTable';
import LogsActions from './UsageLogsActions';
import LogsFilters from './UsageLogsFilters';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import UserInfoModal from './modals/UserInfoModal';
import ChannelAffinityUsageCacheModal from './modals/ChannelAffinityUsageCacheModal';
import ChannelAffinityDiagnosticsModal from './modals/ChannelAffinityDiagnosticsModal';
import ParamOverrideModal from './modals/ParamOverrideModal';
import { useLogsData } from '../../../hooks/usage-logs/useUsageLogsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import { renderQuota } from '../../../helpers';
import {
  PiChartLineUpDuotone,
  PiDatabaseDuotone,
  PiGaugeDuotone,
  PiListMagnifyingGlassDuotone,
  PiReceiptDuotone,
} from 'react-icons/pi';
import './usage-logs.css';

const formatNumber = (value) => {
  const numericValue = Number(value || 0);
  return Number.isFinite(numericValue) ? numericValue.toLocaleString() : '0';
};

const UsageLogMetricCard = ({ icon, label, value, helper, tone }) => (
  <div className={`ct-usage-logs-metric ct-usage-logs-metric-${tone}`}>
    <div className='ct-usage-logs-metric-copy'>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{helper}</small>
    </div>
    <div className='ct-usage-logs-metric-icon'>{icon}</div>
  </div>
);

const adminInsightProfiles = {
  2: [
    ['默认视图', '消费日志', '聚焦扣费和成本字段'],
    ['核对重点', '模型计费', '对照用户消耗与渠道费用'],
    ['处置动作', '展开明细', '查看完整计费过程'],
  ],
  3: [
    ['默认视图', '管理日志', '聚焦后台操作和权限变化'],
    ['核对重点', '操作人', '结合用户、接口和时间排查'],
    ['处置动作', '审计追踪', '保留操作上下文'],
  ],
  5: [
    ['默认视图', '错误日志', '聚焦失败、拦截和异常响应'],
    ['核对重点', '风险用户', '结合用户、渠道和模型定位'],
    ['处置动作', '复测链路', '对异常请求做路由排查'],
  ],
};

const LogsPage = ({
  variant = 'default',
  initialLogType = 0,
  eyebrow,
  title,
  description,
}) => {
  const logsData = useLogsData({ initialLogType });
  const isMobile = useIsMobile();
  const { t } = logsData;
  const isAdminVariant = variant === 'admin';
  const adminInsights =
    adminInsightProfiles[initialLogType] || adminInsightProfiles[2];

  const summaryCards = [
    {
      key: 'quota',
      label: t('消耗额度'),
      value: renderQuota(logsData.stat?.quota || 0),
      helper: t('按当前筛选汇总'),
      tone: 'blue',
      icon: <PiReceiptDuotone size={22} />,
    },
    {
      key: 'rpm',
      label: 'RPM',
      value: formatNumber(logsData.stat?.rpm),
      helper: t('请求速率'),
      tone: 'rose',
      icon: <PiGaugeDuotone size={22} />,
    },
    {
      key: 'tpm',
      label: 'TPM',
      value: formatNumber(logsData.stat?.tpm),
      helper: t('Token 速率'),
      tone: 'cyan',
      icon: <PiChartLineUpDuotone size={22} />,
    },
    {
      key: 'count',
      label: t('日志总数'),
      value: formatNumber(logsData.logCount),
      helper: `${t('本页记录')} ${formatNumber(logsData.logs?.length)}`,
      tone: 'green',
      icon: <PiDatabaseDuotone size={22} />,
    },
  ];

  return (
    <div
      className={`ct-usage-logs-page${
        isAdminVariant ? ' ct-usage-logs-page-admin' : ''
      }`}
    >
      <ColumnSelectorModal {...logsData} />
      <UserInfoModal {...logsData} />
      <ChannelAffinityUsageCacheModal {...logsData} />
      <ChannelAffinityDiagnosticsModal {...logsData} />
      <ParamOverrideModal {...logsData} />

      <section className='ct-usage-logs-hero'>
        <div className='ct-usage-logs-hero-copy'>
          <div className='ct-usage-logs-eyebrow'>
            <PiListMagnifyingGlassDuotone size={16} />
            {t(eyebrow || '请求观测')}
          </div>
          <h1>{t(title || '使用日志')}</h1>
          <p>
            {t(description || '集中查看请求链路、计费消耗、延迟与调度细节。')}
          </p>
        </div>
        <LogsActions {...logsData} />
      </section>

      <section className='ct-usage-logs-summary'>
        {summaryCards.map((card) => (
          <UsageLogMetricCard
            key={card.key}
            icon={card.icon}
            label={card.label}
            value={card.value}
            helper={card.helper}
            tone={card.tone}
          />
        ))}
      </section>

      {isAdminVariant && (
        <section className='ct-usage-logs-admin-strip'>
          {adminInsights.map(([label, value, helper]) => (
            <div className='ct-usage-logs-admin-insight' key={label}>
              <span>{t(label)}</span>
              <strong>{t(value)}</strong>
              <small>{t(helper)}</small>
            </div>
          ))}
        </section>
      )}

      <section className='ct-usage-logs-filter-panel'>
        <div className='ct-usage-logs-section-head'>
          <div>
            <span>{t('筛选条件')}</span>
            <p>{t('时间范围、对象与链路信息')}</p>
          </div>
        </div>
        <LogsFilters {...logsData} />
      </section>

      <section className='ct-usage-logs-table-panel'>
        <div className='ct-usage-logs-table-head'>
          <div>
            <span>{t('请求记录')}</span>
            <p>{t('列表已适配宽表扫描，可展开行查看完整计费过程。')}</p>
          </div>
          <div className='ct-usage-logs-table-meta'>
            <span>{t('当前筛选结果')}</span>
            <strong>{formatNumber(logsData.logCount)}</strong>
          </div>
        </div>
        <LogsTable {...logsData} />
        <div className='ct-usage-logs-pagination'>
          {createCardProPagination({
            currentPage: logsData.activePage,
            pageSize: logsData.pageSize,
            total: logsData.logCount,
            onPageChange: logsData.handlePageChange,
            onPageSizeChange: logsData.handlePageSizeChange,
            isMobile: isMobile,
            t: logsData.t,
          })}
        </div>
      </section>
    </div>
  );
};

export default LogsPage;
