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
import { Button, Tag } from '@douyinfe/semi-ui';
import {
  Clock3,
  RefreshCw,
  ShieldCheck,
  SlidersHorizontal,
} from 'lucide-react';

const DashboardHeader = ({
  getGreeting,
  greetingVisible,
  showSearchModal,
  refresh,
  loading,
  dataExportDefaultTime,
  timeOptions = [],
  isAdminUser,
  performanceMetrics,
  t,
}) => {
  const activeTimeLabel =
    timeOptions.find((option) => option.value === dataExportDefaultTime)
      ?.label || dataExportDefaultTime;
  const formatPerformanceValue = (value) => {
    const numericValue = Number(value);
    if (!Number.isFinite(numericValue)) {
      return value || '0';
    }
    if (Math.abs(numericValue) >= 1000000) {
      return `${(numericValue / 1000000).toFixed(1)}M`;
    }
    if (Math.abs(numericValue) >= 1000) {
      return `${(numericValue / 1000).toFixed(1)}K`;
    }
    return numericValue.toFixed(3).replace(/\.?0+$/, '');
  };

  return (
    <div className='ct-dashboard-hero'>
      <div className='ct-dashboard-hero-main'>
        <div className='ct-dashboard-hero-copy'>
          <div className='ct-dashboard-eyebrow'>{t('数据看板')}</div>
          <h2
            className='ct-dashboard-greeting'
            style={{ opacity: greetingVisible ? 1 : 0 }}
          >
            {getGreeting}
          </h2>
        </div>
        <div className='ct-dashboard-hero-meta'>
          <Tag shape='circle' prefixIcon={<ShieldCheck size={12} />}>
            {isAdminUser ? t('管理员') : t('用户')}
          </Tag>
          <Tag shape='circle' prefixIcon={<Clock3 size={12} />}>
            {t('时间粒度')} · {activeTimeLabel}
          </Tag>
          <Tag shape='circle'>
            RPM {formatPerformanceValue(performanceMetrics?.avgRPM)} · TPM{' '}
            {formatPerformanceValue(performanceMetrics?.avgTPM)}
          </Tag>
        </div>
      </div>
      <div className='ct-dashboard-hero-actions'>
        <Button
          type='tertiary'
          icon={<SlidersHorizontal size={16} />}
          onClick={showSearchModal}
          className='ct-dashboard-icon-button ct-dashboard-icon-button-search'
        >
          {t('筛选')}
        </Button>
        <Button
          type='tertiary'
          icon={<RefreshCw size={16} />}
          onClick={refresh}
          loading={loading}
          className='ct-dashboard-icon-button ct-dashboard-icon-button-refresh'
        >
          {t('刷新')}
        </Button>
      </div>
    </div>
  );
};

export default DashboardHeader;
