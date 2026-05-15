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
import { Avatar, Tag, Divider } from '@douyinfe/semi-ui';
import { Server, Gauge, ExternalLink, Copy } from 'lucide-react';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import DashboardCard from './DashboardCard';
import DashboardEmptyState from './DashboardEmptyState';

const ApiInfoPanel = ({
  apiInfoData,
  handleCopyUrl,
  handleSpeedTest,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
  t,
}) => {
  return (
    <DashboardCard
      {...CARD_PROPS}
      className='ct-dashboard-side-card ct-dashboard-api-card'
      tone='info'
      title={
        <div className={`${FLEX_CENTER_GAP2} ct-dashboard-panel-title`}>
          <Server size={16} />
          {t('API信息')}
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <ScrollableContainer maxHeight='24rem'>
        {apiInfoData.length > 0 ? (
          apiInfoData.map((api) => (
            <React.Fragment key={api.id}>
              <div className='ct-dashboard-list-item ct-dashboard-api-item'>
                <div className='ct-dashboard-api-avatar'>
                  <Avatar size='extra-small' color={api.color}>
                    {api.route.substring(0, 2)}
                  </Avatar>
                </div>
                <div className='ct-dashboard-api-content'>
                  <div className='ct-dashboard-api-head'>
                    <span className='ct-dashboard-api-route'>{api.route}</span>
                    <div className='ct-dashboard-api-actions'>
                      <Tag
                        prefixIcon={<Gauge size={12} />}
                        size='small'
                        color='white'
                        shape='circle'
                        onClick={() => handleSpeedTest(api.url)}
                        className='ct-dashboard-mini-action'
                      >
                        {t('测速')}
                      </Tag>
                      <Tag
                        prefixIcon={<ExternalLink size={12} />}
                        size='small'
                        color='white'
                        shape='circle'
                        onClick={() =>
                          window.open(api.url, '_blank', 'noopener,noreferrer')
                        }
                        className='ct-dashboard-mini-action'
                      >
                        {t('跳转')}
                      </Tag>
                    </div>
                  </div>
                  <div className='ct-dashboard-api-url-row'>
                    <span
                      className='ct-dashboard-api-url'
                      onClick={() => handleCopyUrl(api.url)}
                    >
                      {api.url}
                    </span>
                    <Copy
                      size={14}
                      className='flex-shrink-0 text-gray-400 hover:text-semi-color-primary cursor-pointer transition-colors'
                      onClick={() => handleCopyUrl(api.url)}
                    />
                  </div>
                  <div className='ct-dashboard-api-desc'>{api.description}</div>
                </div>
              </div>
              <Divider />
            </React.Fragment>
          ))
        ) : (
          <div className='ct-dashboard-empty-wrap'>
            <DashboardEmptyState
              title={t('暂无API信息')}
              description={t('请联系管理员在系统设置中配置API信息')}
            />
          </div>
        )}
      </ScrollableContainer>
    </DashboardCard>
  );
};

export default ApiInfoPanel;
