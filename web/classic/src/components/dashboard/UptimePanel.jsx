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
import { Button, Spin, Tabs, TabPane, Tag } from '@douyinfe/semi-ui';
import { PiArrowsClockwiseDuotone, PiGaugeDuotone } from 'react-icons/pi';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import DashboardEmptyState from './DashboardEmptyState';

const UptimePanel = ({
  uptimeData,
  uptimeLoading,
  activeUptimeTab,
  setActiveUptimeTab,
  loadUptimeData,
  uptimeLegendData,
  renderMonitorList,
  t,
}) => {
  return (
    <section className='ct-command-feed-panel ct-command-uptime-panel'>
      <div className='ct-command-panel-head ct-command-panel-head-compact'>
        <div className='ct-command-panel-title-group'>
          <div className='ct-command-panel-icon ct-command-panel-icon-uptime'>
            <PiGaugeDuotone size={20} />
          </div>
          <h3 className='ct-command-panel-title'>{t('服务可用性')}</h3>
        </div>
        <Button
          icon={<PiArrowsClockwiseDuotone size={16} />}
          onClick={loadUptimeData}
          loading={uptimeLoading}
          size='small'
          theme='borderless'
          type='tertiary'
          className='ct-command-panel-action'
        />
      </div>
      <div className='ct-command-uptime-body'>
        <Spin spinning={uptimeLoading}>
          {uptimeData.length > 0 ? (
            uptimeData.length === 1 ? (
              <ScrollableContainer maxHeight='23rem'>
                {renderMonitorList(uptimeData[0].monitors)}
              </ScrollableContainer>
            ) : (
              <Tabs
                type='card'
                collapsible
                activeKey={activeUptimeTab}
                onChange={setActiveUptimeTab}
                size='small'
                className='ct-command-uptime-tabs'
              >
                {uptimeData.map((group, groupIdx) => (
                  <TabPane
                    tab={
                      <span className='flex items-center gap-2'>
                        <PiGaugeDuotone size={16} />
                        {group.categoryName}
                        <Tag
                          color={
                            activeUptimeTab === group.categoryName
                              ? 'red'
                              : 'grey'
                          }
                          size='small'
                          shape='circle'
                        >
                          {group.monitors ? group.monitors.length : 0}
                        </Tag>
                      </span>
                    }
                    itemKey={group.categoryName}
                    key={groupIdx}
                  >
                    <ScrollableContainer maxHeight='20.5rem'>
                      {renderMonitorList(group.monitors)}
                    </ScrollableContainer>
                  </TabPane>
                ))}
              </Tabs>
            )
          ) : (
            <div className='ct-command-empty-wrap'>
              <DashboardEmptyState
                title={t('暂无监控数据')}
                description={t('请联系管理员在系统设置中配置Uptime')}
              />
            </div>
          )}
        </Spin>
      </div>

      {uptimeData.length > 0 && (
        <div className='ct-command-legend-bar'>
          <div className='ct-command-legend-inline justify-center'>
            {uptimeLegendData.map((legend, index) => (
              <div key={index} className='ct-command-legend-item'>
                <div
                  className='ct-command-legend-dot'
                  style={{ backgroundColor: legend.color }}
                />
                <span>{legend.label}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </section>
  );
};

export default UptimePanel;
