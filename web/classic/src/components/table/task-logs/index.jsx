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
import { Activity, Columns3, FileText, Rows3 } from 'lucide-react';
import ConsoleTableScaffold from '../../common/ui/ConsoleTableScaffold';
import TaskLogsTable from './TaskLogsTable';
import TaskLogsActions from './TaskLogsActions';
import TaskLogsFilters from './TaskLogsFilters';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import ContentModal from './modals/ContentModal';
import AudioPreviewModal from './modals/AudioPreviewModal';
import { useTaskLogsData } from '../../../hooks/task-logs/useTaskLogsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const TaskLogsPage = () => {
  const taskLogsData = useTaskLogsData();
  const isMobile = useIsMobile();
  const visibleColumnCount = Object.values(
    taskLogsData.visibleColumns || {},
  ).filter(Boolean).length;

  return (
    <>
      {/* Modals */}
      <ColumnSelectorModal {...taskLogsData} />
      <ContentModal {...taskLogsData} isVideo={false} />
      {/* 新增：视频预览弹窗 */}
      <ContentModal
        isModalOpen={taskLogsData.isVideoModalOpen}
        setIsModalOpen={taskLogsData.setIsVideoModalOpen}
        modalContent={taskLogsData.videoUrl}
        isVideo={true}
      />
      <AudioPreviewModal
        isModalOpen={taskLogsData.isAudioModalOpen}
        setIsModalOpen={taskLogsData.setIsAudioModalOpen}
        audioClips={taskLogsData.audioClips}
      />

      <ConsoleTableScaffold
        eyebrow={taskLogsData.t('异步任务')}
        title={taskLogsData.t('任务记录')}
        subtitle={taskLogsData.t(
          '查看异步任务、音视频预览、请求内容和执行状态，帮助定位任务链路问题。',
        )}
        badge={
          <Tag color='orange' shape='circle' type='light'>
            {taskLogsData.t('任务观测')}
          </Tag>
        }
        metrics={[
          {
            key: 'logs',
            label: taskLogsData.t('任务总数'),
            value: taskLogsData.logCount,
            helper: taskLogsData.t('当前筛选范围'),
            tone: 'teal',
            icon: <Activity size={20} />,
          },
          {
            key: 'visible',
            label: taskLogsData.t('当前页'),
            value: taskLogsData.logs.length,
            helper: taskLogsData.t('可见任务记录'),
            tone: 'blue',
            icon: <Rows3 size={20} />,
          },
          {
            key: 'columns',
            label: taskLogsData.t('显示列'),
            value: visibleColumnCount,
            helper: taskLogsData.t('当前表格视图'),
            tone: 'green',
            icon: <Columns3 size={20} />,
          },
          {
            key: 'content',
            label: taskLogsData.t('内容预览'),
            value: taskLogsData.t('文本/媒体'),
            helper: taskLogsData.t('支持弹窗查看'),
            tone: 'amber',
            icon: <FileText size={20} />,
          },
        ]}
        tableTitle={taskLogsData.t('任务清单')}
        tableSubtitle={taskLogsData.t('按时间、状态、模型和内容扫描任务')}
        tableIcon={<Activity size={18} />}
        tableMeta={`${taskLogsData.t('共')} ${taskLogsData.logCount} ${taskLogsData.t('条')}`}
        toolbar={<TaskLogsActions {...taskLogsData} />}
      >
        <div className='ct-console-table-section ct-console-table-filter-panel'>
          <TaskLogsFilters {...taskLogsData} />
        </div>
        <div className='ct-console-table-surface'>
          <TaskLogsTable {...taskLogsData} />
        </div>
        <div className='ct-console-table-pagination'>
          {createCardProPagination({
            currentPage: taskLogsData.activePage,
            pageSize: taskLogsData.pageSize,
            total: taskLogsData.logCount,
            onPageChange: taskLogsData.handlePageChange,
            onPageSizeChange: taskLogsData.handlePageSizeChange,
            isMobile: isMobile,
            t: taskLogsData.t,
          })}
        </div>
      </ConsoleTableScaffold>
    </>
  );
};

export default TaskLogsPage;
