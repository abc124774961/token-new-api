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
import { Tabs, TabPane } from '@douyinfe/semi-ui';
import { PiChartBarDuotone, PiChartPieSliceDuotone } from 'react-icons/pi';
import { VChart } from '@visactor/react-vchart';

const ChartsPanel = ({
  activeChartTab,
  setActiveChartTab,
  spec_line,
  spec_model_line,
  spec_pie,
  spec_rank_bar,
  spec_user_rank,
  spec_user_trend,
  isAdminUser,
  CHART_CONFIG,
  hasApiInfoPanel,
  t,
}) => {
  return (
    <section
      className={`ct-command-analysis-panel ${
        hasApiInfoPanel ? 'ct-command-analysis-panel-with-rail' : ''
      }`}
    >
      <div className='ct-command-panel-head'>
        <div className='ct-command-panel-title-group'>
          <div className='ct-command-panel-icon ct-command-panel-icon-analysis'>
            <PiChartBarDuotone size={20} />
          </div>
          <div>
            <div className='ct-command-panel-kicker'>{t('统计状态')}</div>
            <h3 className='ct-command-panel-title'>
              <PiChartPieSliceDuotone size={19} />
              {t('模型数据分析')}
            </h3>
          </div>
        </div>
        <Tabs
          type='button'
          size='small'
          activeKey={activeChartTab}
          onChange={setActiveChartTab}
          className='ct-command-chart-tabs'
        >
          <TabPane tab={<span>{t('消耗分布')}</span>} itemKey='1' />
          <TabPane tab={<span>{t('调用趋势')}</span>} itemKey='2' />
          <TabPane tab={<span>{t('调用次数分布')}</span>} itemKey='3' />
          <TabPane tab={<span>{t('调用次数排行')}</span>} itemKey='4' />
          {isAdminUser && (
            <TabPane tab={<span>{t('用户消耗排行')}</span>} itemKey='5' />
          )}
          {isAdminUser && (
            <TabPane tab={<span>{t('用户消耗趋势')}</span>} itemKey='6' />
          )}
        </Tabs>
      </div>
      <div className='ct-command-chart-stage'>
        {activeChartTab === '1' && (
          <VChart spec={spec_line} option={CHART_CONFIG} />
        )}
        {activeChartTab === '2' && (
          <VChart spec={spec_model_line} option={CHART_CONFIG} />
        )}
        {activeChartTab === '3' && (
          <VChart spec={spec_pie} option={CHART_CONFIG} />
        )}
        {activeChartTab === '4' && (
          <VChart spec={spec_rank_bar} option={CHART_CONFIG} />
        )}
        {activeChartTab === '5' && isAdminUser && (
          <VChart spec={spec_user_rank} option={CHART_CONFIG} />
        )}
        {activeChartTab === '6' && isAdminUser && (
          <VChart spec={spec_user_trend} option={CHART_CONFIG} />
        )}
      </div>
    </section>
  );
};

export default ChartsPanel;
