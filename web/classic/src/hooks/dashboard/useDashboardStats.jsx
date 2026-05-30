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

import { useMemo } from 'react';
import {
  PiChartBarDuotone,
  PiClockCounterClockwiseDuotone,
  PiCoinsDuotone,
  PiGaugeDuotone,
  PiLightningDuotone,
  PiMoneyDuotone,
  PiPaperPlaneTiltDuotone,
  PiPulseDuotone,
  PiTextTDuotone,
  PiTimerDuotone,
  PiWalletDuotone,
  PiWaveformDuotone,
} from 'react-icons/pi';
import { renderQuota } from '../../helpers';
import { createSectionTitle } from '../../helpers/dashboard';

export const useDashboardStats = (
  userState,
  consumeQuota,
  consumeTokens,
  times,
  trendData,
  performanceMetrics,
  navigate,
  t,
) => {
  const groupedStatsData = useMemo(
    () => [
      {
        title: createSectionTitle(PiWalletDuotone, t('账户数据')),
        tone: 'info',
        items: [
          {
            title: t('当前余额'),
            value: renderQuota(userState?.user?.quota),
            icon: <PiMoneyDuotone size={19} />,
            avatarColor: 'teal',
            iconTone: 'teal',
            trendData: [],
            trendColor: '#7cc7bd',
          },
          {
            title: t('历史消耗'),
            value: renderQuota(userState?.user?.used_quota),
            icon: <PiClockCounterClockwiseDuotone size={19} />,
            avatarColor: 'blue',
            iconTone: 'blue',
            trendData: [],
            trendColor: '#93b7e8',
          },
        ],
      },
      {
        title: createSectionTitle(PiPulseDuotone, t('使用统计')),
        tone: 'analysis',
        items: [
          {
            title: t('请求次数'),
            value: userState.user?.request_count,
            icon: <PiPaperPlaneTiltDuotone size={19} />,
            avatarColor: 'green',
            iconTone: 'green',
            trendData: [],
            trendColor: '#7dcaa6',
          },
          {
            title: t('统计次数'),
            value: times,
            icon: <PiWaveformDuotone size={19} />,
            avatarColor: 'cyan',
            iconTone: 'cyan',
            trendData: trendData.times,
            trendColor: '#7cc7d8',
          },
        ],
      },
      {
        title: createSectionTitle(PiLightningDuotone, t('资源消耗')),
        tone: 'notice',
        items: [
          {
            title: t('统计额度'),
            value: renderQuota(consumeQuota),
            icon: <PiCoinsDuotone size={19} />,
            avatarColor: 'yellow',
            iconTone: 'amber',
            trendData: trendData.consumeQuota,
            trendColor: '#dfb467',
          },
          {
            title: t('统计Tokens'),
            value: isNaN(consumeTokens) ? 0 : consumeTokens.toLocaleString(),
            icon: <PiTextTDuotone size={19} />,
            avatarColor: 'indigo',
            iconTone: 'violet',
            trendData: trendData.tokens,
            trendColor: '#baa5e8',
          },
        ],
      },
      {
        title: createSectionTitle(PiGaugeDuotone, t('性能指标')),
        tone: 'faq',
        items: [
          {
            title: t('平均RPM'),
            value: performanceMetrics.avgRPM,
            icon: <PiTimerDuotone size={19} />,
            avatarColor: 'indigo',
            iconTone: 'indigo',
            trendData: trendData.rpm,
            trendColor: '#a5b4fc',
          },
          {
            title: t('平均TPM'),
            value: performanceMetrics.avgTPM,
            icon: <PiChartBarDuotone size={19} />,
            avatarColor: 'orange',
            iconTone: 'orange',
            trendData: trendData.tpm,
            trendColor: '#dfa17a',
          },
        ],
      },
    ],
    [
      userState?.user?.quota,
      userState?.user?.used_quota,
      userState?.user?.request_count,
      times,
      consumeQuota,
      consumeTokens,
      trendData,
      performanceMetrics,
      navigate,
      t,
    ],
  );

  return {
    groupedStatsData,
  };
};
