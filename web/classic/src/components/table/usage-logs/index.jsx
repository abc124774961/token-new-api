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
import { Button } from '@douyinfe/semi-ui';
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
  PiCaretDownDuotone,
  PiCaretUpDuotone,
  PiChartLineUpDuotone,
  PiCoinsDuotone,
  PiDatabaseDuotone,
  PiGaugeDuotone,
  PiListMagnifyingGlassDuotone,
  PiReceiptDuotone,
  PiSlidersHorizontalDuotone,
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
  const [filtersExpanded, setFiltersExpanded] = React.useState(false);

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
      key: 'token',
      label: t('Token 总数'),
      value: formatNumber(logsData.stat?.token),
      helper: t('按当前筛选汇总'),
      tone: 'violet',
      icon: <PiCoinsDuotone size={22} />,
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

      <section className='ct-usage-logs-filter-panel'>
        <div className='ct-usage-logs-section-head'>
          <div>
            <span>{t('筛选条件')}</span>
            <p>{t('时间范围、对象与链路信息')}</p>
          </div>
          <Button
            className='ct-usage-logs-filter-toggle'
            type='tertiary'
            theme='borderless'
            icon={
              filtersExpanded ? (
                <PiCaretUpDuotone size={15} />
              ) : (
                <PiCaretDownDuotone size={15} />
              )
            }
            onClick={() => setFiltersExpanded((expanded) => !expanded)}
          >
            {filtersExpanded ? t('收起筛选') : t('展开筛选')}
          </Button>
        </div>
        <div
          className={`ct-usage-logs-filter-body${
            filtersExpanded ? '' : ' ct-usage-logs-filter-body-collapsed'
          }`}
        >
          <LogsFilters {...logsData} />
        </div>
        {!filtersExpanded ? (
          <div className='ct-usage-logs-filter-collapsed-note'>
            <PiSlidersHorizontalDuotone size={15} />
            <span>{t('筛选已收起，点击展开后可调整时间范围、对象与链路信息。')}</span>
          </div>
        ) : null}
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
