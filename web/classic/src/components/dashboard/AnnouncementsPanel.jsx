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
import { Tag, Timeline } from '@douyinfe/semi-ui';
import { PiMegaphoneDuotone } from 'react-icons/pi';
import { marked } from 'marked';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import DashboardEmptyState from './DashboardEmptyState';

const AnnouncementsPanel = ({
  announcementData,
  announcementLegendData,
  t,
}) => {
  return (
    <section className='ct-command-feed-panel ct-command-announcement-panel'>
      <div className='ct-command-panel-head'>
        <div className='ct-command-panel-title-group'>
          <div className='ct-command-panel-icon ct-command-panel-icon-notice'>
            <PiMegaphoneDuotone size={20} />
          </div>
          <h3 className='ct-command-panel-title'>
            {t('系统公告')}
            <Tag color='green' shape='circle' className='ct-dashboard-soft-tag'>
              {t('显示最新20条')}
            </Tag>
          </h3>
        </div>
        <div className='ct-command-legend-inline'>
          {announcementLegendData.map((legend, index) => (
            <div key={index} className='ct-command-legend-item'>
              <div
                className='ct-command-legend-dot'
                style={{
                  backgroundColor:
                    legend.color === 'grey'
                      ? '#8b9aa7'
                      : legend.color === 'blue'
                        ? '#3b82f6'
                        : legend.color === 'green'
                          ? '#10b981'
                          : legend.color === 'orange'
                            ? '#f59e0b'
                            : legend.color === 'red'
                              ? '#ef4444'
                              : '#8b9aa7',
                }}
              />
              <span>{legend.label}</span>
            </div>
          ))}
        </div>
      </div>
      <ScrollableContainer maxHeight='26rem'>
        {announcementData.length > 0 ? (
          <Timeline mode='left' className='ct-command-timeline'>
            {announcementData.map((item, idx) => {
              const htmlExtra = item.extra ? marked.parse(item.extra) : '';
              return (
                <Timeline.Item
                  key={idx}
                  type={item.type || 'default'}
                  time={`${item.relative ? item.relative + ' ' : ''}${item.time}`}
                  extra={
                    item.extra ? (
                      <div
                        className='text-xs text-gray-500'
                        dangerouslySetInnerHTML={{ __html: htmlExtra }}
                      />
                    ) : null
                  }
                >
                  <div>
                    <div
                      dangerouslySetInnerHTML={{
                        __html: marked.parse(item.content || ''),
                      }}
                    />
                  </div>
                </Timeline.Item>
              );
            })}
          </Timeline>
        ) : (
          <div className='ct-command-empty-wrap'>
            <DashboardEmptyState
              title={t('暂无系统公告')}
              description={t('请联系管理员在系统设置中配置公告信息')}
            />
          </div>
        )}
      </ScrollableContainer>
    </section>
  );
};

export default AnnouncementsPanel;
