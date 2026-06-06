import React from 'react';
import UsageLogsTable from '../../../components/table/usage-logs';

const AdminRiskRecords = () => {
  return (
    <div className='ct-console-content-wrap ct-admin-risk-records-page'>
      <UsageLogsTable
        variant='admin'
        initialLogType={5}
        eyebrow='用户运营'
        title='风控记录'
        description='追踪失败、拦截、异常响应和高风险请求，辅助运营定位风险用户与异常渠道。'
      />
    </div>
  );
};

export default AdminRiskRecords;
