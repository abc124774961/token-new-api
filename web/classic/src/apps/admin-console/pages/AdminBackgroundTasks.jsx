import React from 'react';
import { useTranslation } from 'react-i18next';
import { Activity, Clock3, ListChecks } from 'lucide-react';
import TaskLogsTable from '../../../components/table/task-logs';

const AdminBackgroundTasks = () => {
  const { t } = useTranslation();

  return (
    <div className='aurora-admin-page aurora-background-tasks-page'>
      <section className='aurora-overview-hero'>
        <div>
          <div className='aurora-overview-kicker'>{t('系统治理')}</div>
          <h1>{t('后台任务')}</h1>
          <p>
            {t(
              '查看异步任务、视频任务和音乐任务的提交状态、渠道归属、执行进度与失败原因。',
            )}
          </p>
          <div className='aurora-overview-meta'>
            <span>{t('数据来源')} /api/task</span>
            <span>{t('管理员视角')}</span>
          </div>
        </div>
        <div className='aurora-overview-status aurora-status-success'>
          <span>{t('任务观测')}</span>
          <strong>{t('实时')}</strong>
          <em>{t('按筛选条件刷新')}</em>
        </div>
      </section>

      <section className='aurora-source-grid'>
        <div className='aurora-source-item is-ok'>
          <span className='aurora-source-state'>
            <ListChecks size={14} />
            {t('任务列表')}
          </span>
          <strong>{t('统一异步任务')}</strong>
          <small>{t('支持任务 ID、渠道 ID 和时间范围筛选。')}</small>
        </div>
        <div className='aurora-source-item is-ok'>
          <span className='aurora-source-state'>
            <Activity size={14} />
            {t('执行进度')}
          </span>
          <strong>{t('状态和进度')}</strong>
          <small>{t('展示队列中、执行中、成功、失败等任务状态。')}</small>
        </div>
        <div className='aurora-source-item'>
          <span className='aurora-source-state'>
            <Clock3 size={14} />
            {t('耗时判断')}
          </span>
          <strong>{t('提交到完成')}</strong>
          <small>{t('辅助排查长时间排队、上游失败和结果回写异常。')}</small>
        </div>
      </section>

      <section className='aurora-panel ct-admin-task-logs-panel'>
        <TaskLogsTable />
      </section>
    </div>
  );
};

export default AdminBackgroundTasks;
