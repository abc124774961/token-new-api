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
import { CopyCheck, Gift, Rows3, Ticket } from 'lucide-react';
import ConsoleTableScaffold from '../../common/ui/ConsoleTableScaffold';
import RedemptionsTable from './RedemptionsTable';
import RedemptionsActions from './RedemptionsActions';
import RedemptionsFilters from './RedemptionsFilters';
import EditRedemptionModal from './modals/EditRedemptionModal';
import { useRedemptionsData } from '../../../hooks/redemptions/useRedemptionsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import CompactModeToggle from '../../common/ui/CompactModeToggle';

const RedemptionsPage = () => {
  const redemptionsData = useRedemptionsData();
  const isMobile = useIsMobile();

  const {
    // Edit state
    showEdit,
    editingRedemption,
    closeEdit,
    refresh,

    // Actions state
    selectedKeys,
    setEditingRedemption,
    setShowEdit,
    batchCopyRedemptions,
    batchDeleteRedemptions,

    // Filters state
    formInitValues,
    setFormApi,
    searchRedemptions,
    loading,
    searching,

    // UI state
    compactMode,
    setCompactMode,
    redemptions,

    // Translation
    t,
  } = redemptionsData;
  const selectedCount = selectedKeys.length;

  return (
    <>
      <EditRedemptionModal
        refresh={refresh}
        editingRedemption={editingRedemption}
        visiable={showEdit}
        handleClose={closeEdit}
      />

      <ConsoleTableScaffold
        eyebrow={t('商业运营')}
        title={t('兑换码管理')}
        subtitle={t(
          '批量生成、筛选、复制和回收兑换码，支撑充值活动和人工补偿流程。',
        )}
        badge={
          <Tag color='orange' shape='circle' type='light'>
            {t('权益发放')}
          </Tag>
        }
        metrics={[
          {
            key: 'codes',
            label: t('兑换码总数'),
            value: redemptionsData.tokenCount,
            helper: t('当前筛选范围'),
            tone: 'teal',
            icon: <Ticket size={20} />,
          },
          {
            key: 'visible',
            label: t('当前页'),
            value: redemptions.length,
            helper: t('可见兑换码记录'),
            tone: 'blue',
            icon: <Rows3 size={20} />,
          },
          {
            key: 'selected',
            label: t('已选择'),
            value: selectedCount,
            helper: t('用于复制或删除'),
            tone: selectedCount > 0 ? 'amber' : 'green',
            icon: <CopyCheck size={20} />,
          },
          {
            key: 'operation',
            label: t('运营用途'),
            value: t('发放'),
            helper: t('充值活动和补偿'),
            tone: 'amber',
            icon: <Gift size={20} />,
          },
        ]}
        tableTitle={t('兑换码清单')}
        tableSubtitle={
          searching
            ? t('正在筛选匹配兑换码')
            : t('按状态、额度和创建信息扫描兑换码')
        }
        tableIcon={<Ticket size={18} />}
        tableMeta={`${t('共')} ${redemptionsData.tokenCount} ${t('条')}`}
        toolbar={
          <CompactModeToggle
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        }
      >
        <div className='ct-console-table-section ct-console-table-filter-panel'>
          <div className='flex flex-col md:flex-row justify-between items-center gap-2 w-full'>
            <RedemptionsActions
              selectedKeys={selectedKeys}
              setEditingRedemption={setEditingRedemption}
              setShowEdit={setShowEdit}
              batchCopyRedemptions={batchCopyRedemptions}
              batchDeleteRedemptions={batchDeleteRedemptions}
              t={t}
            />

            <div className='w-full md:w-full lg:w-auto order-1 md:order-2'>
              <RedemptionsFilters
                formInitValues={formInitValues}
                setFormApi={setFormApi}
                searchRedemptions={searchRedemptions}
                loading={loading}
                searching={searching}
                t={t}
              />
            </div>
          </div>
        </div>
        <div className='ct-console-table-surface'>
          <RedemptionsTable {...redemptionsData} />
        </div>
        <div className='ct-console-table-pagination'>
          {createCardProPagination({
            currentPage: redemptionsData.activePage,
            pageSize: redemptionsData.pageSize,
            total: redemptionsData.tokenCount,
            onPageChange: redemptionsData.handlePageChange,
            onPageSizeChange: redemptionsData.handlePageSizeChange,
            isMobile: isMobile,
            t: redemptionsData.t,
          })}
        </div>
      </ConsoleTableScaffold>
    </>
  );
};

export default RedemptionsPage;
