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
import { Collapse } from '@douyinfe/semi-ui';
import { HelpCircle } from 'lucide-react';
import { IconPlus, IconMinus } from '@douyinfe/semi-icons';
import { marked } from 'marked';
import ScrollableContainer from '../common/ui/ScrollableContainer';
import DashboardCard from './DashboardCard';
import DashboardEmptyState from './DashboardEmptyState';

const FaqPanel = ({
  faqData,
  CARD_PROPS,
  FLEX_CENTER_GAP2,
  ILLUSTRATION_SIZE,
  t,
}) => {
  return (
    <DashboardCard
      {...CARD_PROPS}
      className='lg:col-span-1'
      tone='faq'
      title={
        <div className={`${FLEX_CENTER_GAP2} ct-dashboard-panel-title`}>
          <HelpCircle size={16} />
          {t('常见问答')}
        </div>
      }
      bodyStyle={{ padding: 0 }}
    >
      <ScrollableContainer maxHeight='24rem'>
        {faqData.length > 0 ? (
          <Collapse
            accordion
            expandIcon={<IconPlus />}
            collapseIcon={<IconMinus />}
          >
            {faqData.map((item, index) => (
              <Collapse.Panel
                key={index}
                header={item.question}
                itemKey={index.toString()}
              >
                <div
                  dangerouslySetInnerHTML={{
                    __html: marked.parse(item.answer || ''),
                  }}
                />
              </Collapse.Panel>
            ))}
          </Collapse>
        ) : (
          <div className='flex justify-center items-center py-8'>
            <DashboardEmptyState
              title={t('暂无常见问答')}
              description={t('请联系管理员在系统设置中配置常见问答')}
            />
          </div>
        )}
      </ScrollableContainer>
    </DashboardCard>
  );
};

export default FaqPanel;
