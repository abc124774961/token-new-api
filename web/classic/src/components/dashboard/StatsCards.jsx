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
import { Avatar, Skeleton, Tag } from '@douyinfe/semi-ui';
import { VChart } from '@visactor/react-vchart';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import DashboardCard from './DashboardCard';

const StatsCards = ({
  groupedStatsData,
  loading,
  getTrendSpec,
  CARD_PROPS,
  CHART_CONFIG,
}) => {
  const navigate = useNavigate();
  const { t } = useTranslation();
  return (
    <div className='mb-4'>
      <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4'>
        {groupedStatsData.map((group, idx) => (
          <DashboardCard
            key={idx}
            {...CARD_PROPS}
            className={`w-full ${group.color || ''}`}
            title={group.title}
            tone={group.tone || 'default'}
          >
            <div className='ct-dashboard-stat-list'>
              {group.items.map((item, itemIdx) => (
                <div
                  key={itemIdx}
                  className='ct-dashboard-stat-row'
                  onClick={item.onClick}
                >
                  <div className='ct-dashboard-stat-main'>
                    <Avatar
                      className='ct-dashboard-stat-avatar'
                      size='small'
                      color={item.avatarColor}
                    >
                      {item.icon}
                    </Avatar>
                    <div>
                      <div className='ct-dashboard-stat-label'>
                        {item.title}
                      </div>
                      <div className='ct-dashboard-stat-value'>
                        <Skeleton
                          loading={loading}
                          active
                          placeholder={
                            <Skeleton.Paragraph
                              active
                              rows={1}
                              style={{
                                width: '65px',
                                height: '24px',
                                marginTop: '4px',
                              }}
                            />
                          }
                        >
                          {item.value}
                        </Skeleton>
                      </div>
                    </div>
                  </div>
                  {item.title === t('当前余额') ? (
                    <Tag
                      color='green'
                      shape='circle'
                      size='large'
                      className='ct-dashboard-action-tag'
                      onClick={(e) => {
                        e.stopPropagation();
                        navigate('/console/topup');
                      }}
                    >
                      {t('充值')}
                    </Tag>
                  ) : (
                    (loading ||
                      (item.trendData && item.trendData.length > 0)) && (
                      <div className='w-24 h-10'>
                        <VChart
                          spec={getTrendSpec(item.trendData, item.trendColor)}
                          option={CHART_CONFIG}
                        />
                      </div>
                    )
                  )}
                </div>
              ))}
            </div>
          </DashboardCard>
        ))}
      </div>
    </div>
  );
};

export default StatsCards;
