import React from 'react';
import UsageLogsTable from '../../../components/table/usage-logs';

const AdminAuditLogs = () => {
  return (
    <div className='ct-console-content-wrap ct-admin-audit-logs-page'>
      <UsageLogsTable
        variant='admin'
        initialLogType={3}
        eyebrow='系统治理'
        title='审计日志'
        description='后台操作审计和追踪'
      />
    </div>
  );
};

export default AdminAuditLogs;
