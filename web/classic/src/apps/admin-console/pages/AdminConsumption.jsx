import React from 'react';
import UsageLogsTable from '../../../components/table/usage-logs';

const AdminConsumption = () => {
  return (
    <div className='ct-console-content-wrap ct-admin-consumption-page'>
      <UsageLogsTable
        variant='admin'
        initialLogType={2}
        eyebrow='商业运营'
        title='消费明细'
        description='集中查看用户调用消耗、模型计费、渠道费用和请求链路。'
      />
    </div>
  );
};

export default AdminConsumption;
