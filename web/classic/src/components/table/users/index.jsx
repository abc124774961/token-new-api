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
import { ShieldCheck, Users, UserRoundCheck, WalletCards } from 'lucide-react';
import ConsoleTableScaffold from '../../common/ui/ConsoleTableScaffold';
import UsersTable from './UsersTable';
import UsersActions from './UsersActions';
import UsersFilters from './UsersFilters';
import AddUserModal from './modals/AddUserModal';
import EditUserModal from './modals/EditUserModal';
import { useUsersData } from '../../../hooks/users/useUsersData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import CompactModeToggle from '../../common/ui/CompactModeToggle';

const UsersPage = () => {
  const usersData = useUsersData();
  const isMobile = useIsMobile();

  const {
    // Modal state
    showAddUser,
    showEditUser,
    editingUser,
    setShowAddUser,
    closeAddUser,
    closeEditUser,
    refresh,

    // Form state
    formInitValues,
    setFormApi,
    searchUsers,
    loadUsers,
    activePage,
    pageSize,
    groupOptions,
    loading,
    searching,

    // Description state
    compactMode,
    setCompactMode,
    users,

    // Translation
    t,
  } = usersData;
  const enabledUsers = (users || []).filter((user) => user.status === 1).length;
  const adminUsers = (users || []).filter((user) => user.role >= 10).length;

  return (
    <>
      <AddUserModal
        refresh={refresh}
        visible={showAddUser}
        handleClose={closeAddUser}
      />

      <EditUserModal
        refresh={refresh}
        visible={showEditUser}
        handleClose={closeEditUser}
        editingUser={editingUser}
      />

      <ConsoleTableScaffold
        eyebrow={t('用户运营')}
        title={t('用户管理')}
        subtitle={t(
          '集中查看用户状态、分组、额度和权限，支持快速筛选与账号处置。',
        )}
        badge={
          <Tag color='cyan' shape='circle' type='light'>
            {t('账号资产')}
          </Tag>
        }
        metrics={[
          {
            key: 'users',
            label: t('用户总数'),
            value: usersData.userCount,
            helper: t('当前筛选范围'),
            tone: 'teal',
            icon: <Users size={20} />,
          },
          {
            key: 'visible',
            label: t('当前页'),
            value: users.length,
            helper: t('可见用户记录'),
            tone: 'blue',
            icon: <UserRoundCheck size={20} />,
          },
          {
            key: 'enabled',
            label: t('当前页启用'),
            value: enabledUsers,
            helper: t('可正常访问的账号'),
            tone: 'green',
            icon: <ShieldCheck size={20} />,
          },
          {
            key: 'groups',
            label: t('用户分组'),
            value: groupOptions.length,
            helper: t('可用于筛选和定价'),
            tone: 'amber',
            icon: <WalletCards size={20} />,
          },
        ]}
        tableTitle={t('用户清单')}
        tableSubtitle={
          searching ? t('正在筛选匹配用户') : t('按账号、分组和状态扫描用户')
        }
        tableIcon={<Users size={18} />}
        tableMeta={`${t('管理员')} ${adminUsers}`}
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
            <UsersActions setShowAddUser={setShowAddUser} t={t} />

            <UsersFilters
              formInitValues={formInitValues}
              setFormApi={setFormApi}
              searchUsers={searchUsers}
              loadUsers={loadUsers}
              activePage={activePage}
              pageSize={pageSize}
              groupOptions={groupOptions}
              loading={loading}
              searching={searching}
              t={t}
            />
          </div>
        </div>
        <div className='ct-console-table-surface'>
          <UsersTable {...usersData} />
        </div>
        <div className='ct-console-table-pagination'>
          {createCardProPagination({
            currentPage: usersData.activePage,
            pageSize: usersData.pageSize,
            total: usersData.userCount,
            onPageChange: usersData.handlePageChange,
            onPageSizeChange: usersData.handlePageSizeChange,
            isMobile: isMobile,
            t: usersData.t,
          })}
        </div>
      </ConsoleTableScaffold>
    </>
  );
};

export default UsersPage;
