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

import React, { useState } from 'react';
import { Tag } from '@douyinfe/semi-ui';
import { Boxes, Cloud, Columns3, ServerCog } from 'lucide-react';
import ConsoleTableScaffold from '../../common/ui/ConsoleTableScaffold';
import DeploymentsTable from './DeploymentsTable';
import DeploymentsActions from './DeploymentsActions';
import DeploymentsFilters from './DeploymentsFilters';
import EditDeploymentModal from './modals/EditDeploymentModal';
import CreateDeploymentModal from './modals/CreateDeploymentModal';
import ColumnSelectorModal from './modals/ColumnSelectorModal';
import { useDeploymentsData } from '../../../hooks/model-deployments/useDeploymentsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const DeploymentsPage = () => {
  const deploymentsData = useDeploymentsData();
  const isMobile = useIsMobile();

  // Create deployment modal state
  const [showCreateModal, setShowCreateModal] = useState(false);
  const batchOperationsEnabled = false;

  const {
    // Edit state
    showEdit,
    editingDeployment,
    closeEdit,
    refresh,

    // Actions state
    selectedKeys,
    setSelectedKeys,
    setEditingDeployment,
    setShowEdit,
    batchDeleteDeployments,

    // Filters state
    formInitValues,
    setFormApi,
    searchDeployments,
    loading,
    searching,

    // Column visibility
    showColumnSelector,
    setShowColumnSelector,
    visibleColumns,
    setVisibleColumns,
    COLUMN_KEYS,

    // Description state
    compactMode,
    setCompactMode,
    deployments,

    // Translation
    t,
  } = deploymentsData;
  const selectedCount = selectedKeys.length;
  const activeDeployments = (deployments || []).filter((deployment) =>
    ['running', 'active', 'ready'].includes(
      String(deployment.status || '').toLowerCase(),
    ),
  ).length;

  return (
    <>
      {/* Modals */}
      <EditDeploymentModal
        refresh={refresh}
        editingDeployment={editingDeployment}
        visible={showEdit}
        handleClose={closeEdit}
      />

      <CreateDeploymentModal
        visible={showCreateModal}
        onCancel={() => setShowCreateModal(false)}
        onSuccess={refresh}
        t={t}
      />

      <ColumnSelectorModal
        visible={showColumnSelector}
        onCancel={() => setShowColumnSelector(false)}
        visibleColumns={visibleColumns}
        onVisibleColumnsChange={setVisibleColumns}
        columnKeys={COLUMN_KEYS}
        t={t}
      />

      <ConsoleTableScaffold
        eyebrow={t('模型与路由')}
        title={t('模型部署')}
        subtitle={t(
          '管理 io.net 部署实例、连接状态和资源配置，快速定位可用算力与异常部署。',
        )}
        badge={
          <Tag color='purple' shape='circle' type='light'>
            {t('算力资源')}
          </Tag>
        }
        metrics={[
          {
            key: 'deployments',
            label: t('部署总数'),
            value: deploymentsData.deploymentCount,
            helper: t('当前筛选范围'),
            tone: 'teal',
            icon: <Cloud size={20} />,
          },
          {
            key: 'visible',
            label: t('当前页'),
            value: deployments.length,
            helper: t('可见部署记录'),
            tone: 'blue',
            icon: <ServerCog size={20} />,
          },
          {
            key: 'active',
            label: t('当前页活跃'),
            value: activeDeployments,
            helper: t('运行中或可用状态'),
            tone: 'green',
            icon: <Boxes size={20} />,
          },
          {
            key: 'selected',
            label: t('已选择'),
            value: selectedCount,
            helper: t('用于批量操作'),
            tone: selectedCount > 0 ? 'amber' : 'teal',
            icon: <Columns3 size={20} />,
          },
        ]}
        tableTitle={t('部署清单')}
        tableSubtitle={
          searching ? t('正在筛选匹配部署') : t('按状态、容器和资源扫描部署')
        }
        tableIcon={<ServerCog size={18} />}
        tableMeta={`${t('共')} ${deploymentsData.deploymentCount} ${t('条')}`}
      >
        <div className='ct-console-table-section ct-console-table-filter-panel'>
          <div className='flex flex-col md:flex-row justify-between items-center gap-2 w-full'>
            <DeploymentsActions
              selectedKeys={selectedKeys}
              setSelectedKeys={setSelectedKeys}
              setEditingDeployment={setEditingDeployment}
              setShowEdit={setShowEdit}
              batchDeleteDeployments={batchDeleteDeployments}
              batchOperationsEnabled={batchOperationsEnabled}
              compactMode={compactMode}
              setCompactMode={setCompactMode}
              showCreateModal={showCreateModal}
              setShowCreateModal={setShowCreateModal}
              setShowColumnSelector={setShowColumnSelector}
              t={t}
            />
            <DeploymentsFilters
              formInitValues={formInitValues}
              setFormApi={setFormApi}
              searchDeployments={searchDeployments}
              loading={loading}
              searching={searching}
              setShowColumnSelector={setShowColumnSelector}
              t={t}
            />
          </div>
        </div>
        <div className='ct-console-table-surface'>
          <DeploymentsTable
            {...deploymentsData}
            batchOperationsEnabled={batchOperationsEnabled}
          />
        </div>
        <div className='ct-console-table-pagination'>
          {createCardProPagination({
            currentPage: deploymentsData.activePage,
            pageSize: deploymentsData.pageSize,
            total: deploymentsData.deploymentCount,
            onPageChange: deploymentsData.handlePageChange,
            onPageSizeChange: deploymentsData.handlePageSizeChange,
            isMobile: isMobile,
            t: deploymentsData.t,
          })}
        </div>
      </ConsoleTableScaffold>
    </>
  );
};

export default DeploymentsPage;
