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
import { Tag } from '@douyinfe/semi-ui';
import { Columns3, ImageIcon, Palette, Rows3 } from 'lucide-react';
import ConsoleTableScaffold from '../../common/ui/ConsoleTableScaffold';
import MjLogsTable from './MjLogsTable';
import MjLogsActions from './MjLogsActions';
import MjLogsFilters from './MjLogsFilters';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import ContentModal from './modals/ContentModal';
import { useMjLogsData } from '../../../hooks/mj-logs/useMjLogsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const MjLogsPage = () => {
  const mjLogsData = useMjLogsData();
  const isMobile = useIsMobile();
  const visibleColumnCount = Object.values(
    mjLogsData.visibleColumns || {},
  ).filter(Boolean).length;

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...mjLogsData} />
      <ContentModal {...mjLogsData} />

      <ConsoleTableScaffold
        eyebrow={mjLogsData.t('图像任务')}
        title={mjLogsData.t('Midjourney 任务记录')}
        subtitle={mjLogsData.t(
          '查看绘图任务、回调结果、提示词和图片内容，辅助定位任务链路问题。',
        )}
        badge={
          <Tag color='purple' shape='circle' type='light'>
            {mjLogsData.t('绘图观测')}
          </Tag>
        }
        metrics={[
          {
            key: 'logs',
            label: mjLogsData.t('任务总数'),
            value: mjLogsData.logCount,
            helper: mjLogsData.t('当前筛选范围'),
            tone: 'teal',
            icon: <Palette size={20} />,
          },
          {
            key: 'visible',
            label: mjLogsData.t('当前页'),
            value: mjLogsData.logs.length,
            helper: mjLogsData.t('可见任务记录'),
            tone: 'blue',
            icon: <Rows3 size={20} />,
          },
          {
            key: 'columns',
            label: mjLogsData.t('显示列'),
            value: visibleColumnCount,
            helper: mjLogsData.t('当前表格视图'),
            tone: 'green',
            icon: <Columns3 size={20} />,
          },
          {
            key: 'media',
            label: mjLogsData.t('内容预览'),
            value: mjLogsData.t('图片'),
            helper: mjLogsData.t('支持弹窗查看'),
            tone: 'amber',
            icon: <ImageIcon size={20} />,
          },
        ]}
        tableTitle={mjLogsData.t('任务清单')}
        tableSubtitle={mjLogsData.t('按时间、状态、模型和提示词扫描绘图任务')}
        tableIcon={<Palette size={18} />}
        tableMeta={`${mjLogsData.t('共')} ${mjLogsData.logCount} ${mjLogsData.t('条')}`}
        toolbar={<MjLogsActions {...mjLogsData} />}
      >
        <div className='ct-console-table-section ct-console-table-filter-panel'>
          <MjLogsFilters {...mjLogsData} />
        </div>
        <div className='ct-console-table-surface'>
          <MjLogsTable {...mjLogsData} />
        </div>
        <div className='ct-console-table-pagination'>
          {createCardProPagination({
            currentPage: mjLogsData.activePage,
            pageSize: mjLogsData.pageSize,
            total: mjLogsData.logCount,
            onPageChange: mjLogsData.handlePageChange,
            onPageSizeChange: mjLogsData.handlePageSizeChange,
            isMobile: isMobile,
            t: mjLogsData.t,
          })}
        </div>
      </ConsoleTableScaffold>
    </>
  );
};

export default MjLogsPage;
