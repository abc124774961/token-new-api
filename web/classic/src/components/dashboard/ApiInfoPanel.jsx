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
import { Avatar, Button, Tag } from '@douyinfe/semi-ui';
import {
  PiArrowSquareOutDuotone,
  PiCopyDuotone,
  PiGaugeDuotone,
  PiHardDrivesDuotone,
} from 'react-icons/pi';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import DashboardEmptyState from './DashboardEmptyState';

const ApiInfoPanel = ({
  apiInfoData,
  handleCopyUrl,
  handleSpeedTest,
  t,
}) => {
  return (
    <section className='ct-command-side-panel ct-command-api-panel'>
      <div className='ct-command-panel-head ct-command-panel-head-compact'>
        <div className='ct-command-panel-title-group'>
          <div className='ct-command-panel-icon ct-command-panel-icon-api'>
            <PiHardDrivesDuotone size={20} />
          </div>
          <h3 className='ct-command-panel-title'>{t('API信息')}</h3>
        </div>
        <Tag shape='circle' color='cyan' className='ct-command-count-tag'>
          {apiInfoData.length}
        </Tag>
      </div>
      <ScrollableContainer maxHeight='30rem'>
        {apiInfoData.length > 0 ? (
          apiInfoData.map((api) => (
            <div key={api.id} className='ct-command-api-item'>
              <div className='ct-command-api-topline'>
                <Avatar
                  size='extra-small'
                  color={api.color}
                  className='ct-command-api-avatar'
                >
                  {api.route.substring(0, 2)}
                </Avatar>
                <div className='ct-command-api-route'>{api.route}</div>
                <div className='ct-command-api-actions'>
                  <Button
                    icon={<PiGaugeDuotone size={15} />}
                    size='small'
                    type='tertiary'
                    theme='borderless'
                    className='ct-command-mini-button'
                    onClick={() => handleSpeedTest(api.url)}
                  >
                    {t('测速')}
                  </Button>
                  <Button
                    icon={<PiArrowSquareOutDuotone size={15} />}
                    size='small'
                    type='tertiary'
                    theme='borderless'
                    className='ct-command-mini-button'
                    onClick={() =>
                      window.open(api.url, '_blank', 'noopener,noreferrer')
                    }
                  >
                    {t('跳转')}
                  </Button>
                </div>
              </div>
              <div className='ct-command-api-url-row'>
                <span
                  className='ct-command-api-url'
                  onClick={() => handleCopyUrl(api.url)}
                >
                  {api.url}
                </span>
                <PiCopyDuotone
                  size={16}
                  className='ct-command-api-copy'
                  onClick={() => handleCopyUrl(api.url)}
                />
              </div>
              <div className='ct-command-api-desc'>{api.description}</div>
            </div>
          ))
        ) : (
          <div className='ct-command-empty-wrap'>
            <DashboardEmptyState
              title={t('暂无API信息')}
              description={t('请联系管理员在系统设置中配置API信息')}
            />
          </div>
        )}
      </ScrollableContainer>
    </section>
  );
};

export default ApiInfoPanel;
