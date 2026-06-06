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

import React, { useContext } from 'react';
import { Banner, Tag } from '@douyinfe/semi-ui';
import { CalendarClock, CreditCard, Power, Rows3 } from 'lucide-react';
import ConsoleTableScaffold from '../../common/ui/ConsoleTableScaffold';
import SubscriptionsTable from './SubscriptionsTable';
import SubscriptionsActions from './SubscriptionsActions';
import AddEditSubscriptionModal from './modals/AddEditSubscriptionModal';
import { useSubscriptionsData } from '../../../hooks/subscriptions/useSubscriptionsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import { StatusContext } from '../../../context/Status';
import CompactModeToggle from '../../common/ui/CompactModeToggle';

const SubscriptionsPage = () => {
  const subscriptionsData = useSubscriptionsData();
  const isMobile = useIsMobile();
  const [statusState] = useContext(StatusContext);
  const enableEpay = !!statusState?.status?.enable_online_topup;

  const {
    showEdit,
    editingPlan,
    sheetPlacement,
    closeEdit,
    refresh,
    openCreate,
    compactMode,
    setCompactMode,
    plans,
    t,
  } = subscriptionsData;
  const enabledPlans = (plans || []).filter((item) => item?.plan?.enabled).length;

  return (
    <>
      <AddEditSubscriptionModal
        visible={showEdit}
        handleClose={closeEdit}
        editingPlan={editingPlan}
        placement={sheetPlacement}
        refresh={refresh}
        t={t}
      />

      <ConsoleTableScaffold
        eyebrow={t('商业运营')}
        title={t('订阅管理')}
        subtitle={t(
          '维护套餐售卖、第三方商品映射和订阅状态，确保用户购买路径清晰可控。',
        )}
        badge={
          <Tag color='blue' shape='circle' type='light'>
            {t('套餐配置')}
          </Tag>
        }
        metrics={[
          {
            key: 'plans',
            label: t('套餐总数'),
            value: subscriptionsData.planCount,
            helper: t('当前配置数量'),
            tone: 'teal',
            icon: <CalendarClock size={20} />,
          },
          {
            key: 'visible',
            label: t('当前页'),
            value: plans.length,
            helper: t('可见套餐记录'),
            tone: 'blue',
            icon: <Rows3 size={20} />,
          },
          {
            key: 'enabled',
            label: t('当前页启用'),
            value: enabledPlans,
            helper: t('可被用户购买'),
            tone: 'green',
            icon: <Power size={20} />,
          },
          {
            key: 'epay',
            label: t('在线支付'),
            value: enableEpay ? t('已启用') : t('未启用'),
            helper: t('影响套餐售卖'),
            tone: enableEpay ? 'green' : 'amber',
            icon: <CreditCard size={20} />,
          },
        ]}
        tableTitle={t('套餐清单')}
        tableSubtitle={t('按周期、额度、售价和支付映射扫描套餐')}
        tableIcon={<CalendarClock size={18} />}
        tableMeta={`${t('共')} ${subscriptionsData.planCount} ${t('条')}`}
        toolbar={
          <CompactModeToggle
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        }
      >
        <div className='ct-console-table-section ct-console-table-filter-panel'>
          <div className='flex flex-col md:flex-row justify-between items-start md:items-center gap-2 w-full'>
            {/* Mobile: actions first; Desktop: actions left */}
            <div className='order-1 md:order-0 w-full md:w-auto'>
              <SubscriptionsActions openCreate={openCreate} t={t} />
            </div>
            <Banner
              type='info'
              description={t('Stripe/Creem 需在第三方平台创建商品并填入 ID')}
              closeIcon={null}
              // Mobile: banner below; Desktop: banner right
              className='!rounded-lg order-2 md:order-1'
              style={{ maxWidth: '100%' }}
            />
          </div>
        </div>
        <div className='ct-console-table-surface'>
          <SubscriptionsTable {...subscriptionsData} enableEpay={enableEpay} />
        </div>
        <div className='ct-console-table-pagination'>
          {createCardProPagination({
            currentPage: subscriptionsData.activePage,
            pageSize: subscriptionsData.pageSize,
            total: subscriptionsData.planCount,
            onPageChange: subscriptionsData.handlePageChange,
            onPageSizeChange: subscriptionsData.handlePageSizeChange,
            isMobile,
            t: subscriptionsData.t,
          })}
        </div>
      </ConsoleTableScaffold>
    </>
  );
};

export default SubscriptionsPage;
