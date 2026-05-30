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
import { Button, Input, Tag } from '@douyinfe/semi-ui';
import {
  PiArrowsClockwiseDuotone,
  PiClockCountdownDuotone,
  PiPulseDuotone,
  PiShieldCheckDuotone,
  PiSquaresFourDuotone,
  PiUserCircleDuotone,
} from 'react-icons/pi';

const DashboardHeader = ({
  getGreeting,
  greetingVisible,
  refresh,
  loading,
  inputs,
  dataExportDefaultTime,
  timeOptions = [],
  dateRangePresets = [],
  activeDateRange,
  onDateRangeChange,
  onUsernameChange,
  onUsernameCommit,
  onUsernameClear,
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
    <div className='ct-command-hero'>
      <div className='ct-command-hero-main'>
        <div className='ct-command-hero-copy'>
          <div className='ct-command-eyebrow'>
            <PiSquaresFourDuotone size={15} />
            {t('控制台工作区')}
          </div>
          <h2
            className='ct-command-greeting'
            style={{ opacity: greetingVisible ? 1 : 0 }}
          >
            {getGreeting}
          </h2>
        </div>
        <div className='ct-command-hero-meta'>
          <Tag shape='circle' prefixIcon={<PiPulseDuotone size={13} />}>
            {t('运行状态')}
          </Tag>
          <Tag shape='circle' prefixIcon={<PiShieldCheckDuotone size={13} />}>
            {isAdminUser ? t('管理员') : t('用户')}
          </Tag>
          <Tag
            shape='circle'
            prefixIcon={<PiClockCountdownDuotone size={13} />}
          >
            {t('时间粒度')} · {activeTimeLabel}
          </Tag>
          <Tag shape='circle'>
            RPM {formatPerformanceValue(performanceMetrics?.avgRPM)} · TPM{' '}
            {formatPerformanceValue(performanceMetrics?.avgTPM)}
          </Tag>
        </div>
      </div>
      <div className='ct-command-filter-toolbar'>
        <div className='ct-command-range-control' aria-label={t('时间范围')}>
          {dateRangePresets.map((option) => {
            const active = option.value === activeDateRange;
            return (
              <Button
                key={option.value}
                type='tertiary'
                onClick={() => onDateRangeChange?.(option.value)}
                disabled={loading}
                className={`ct-command-range-button ${
                  active ? 'ct-command-range-button-active' : ''
                }`}
              >
                {option.label}
              </Button>
            );
          })}
        </div>
        {isAdminUser && (
          <div className='ct-command-username-filter'>
            <span className='ct-command-filter-label'>{t('用户名')}:</span>
            <Input
              value={inputs?.username || ''}
              onChange={onUsernameChange}
              onEnterPress={onUsernameCommit}
              onBlur={onUsernameCommit}
              onClear={onUsernameClear}
              showClear
              prefix={<PiUserCircleDuotone size={15} />}
              placeholder={t('用户名')}
              className='ct-command-username-input'
            />
          </div>
        )}
        <Button
          type='tertiary'
          icon={<PiArrowsClockwiseDuotone size={17} />}
          onClick={() => refresh?.()}
          loading={loading}
          className='ct-command-refresh-button'
        >
          {t('刷新')}
        </Button>
      </div>
    </div>
  );
};

export default DashboardHeader;
