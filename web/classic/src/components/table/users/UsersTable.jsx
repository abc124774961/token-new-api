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

import React, { useMemo, useState } from 'react';
import { Empty } from '@douyinfe/semi-ui';
import CardTable from '../../common/ui/CardTable';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { getUsersColumns } from './UsersColumnDefs';
import PromoteUserModal from './modals/PromoteUserModal';
import DemoteUserModal from './modals/DemoteUserModal';
import EnableDisableUserModal from './modals/EnableDisableUserModal';
import DeleteUserModal from './modals/DeleteUserModal';
import ResetPasskeyModal from './modals/ResetPasskeyModal';
import ResetTwoFAModal from './modals/ResetTwoFAModal';
import UserSubscriptionsModal from './modals/UserSubscriptionsModal';
import { showWarning } from '../../../helpers';
import { ADMIN_PERMISSION_KEYS } from '../../../apps/admin-console/permissions/adminPermissions.config';
import { useAdminActionPermission } from '../../../apps/admin-console/permissions/AdminPermissionAction';

const UsersTable = (usersData) => {
  const {
    users,
    loading,
    activePage,
    pageSize,
    userCount,
    compactMode,
    handlePageChange,
    handlePageSizeChange,
    handleRow,
    setEditingUser,
    setShowEditUser,
    manageUser,
    refresh,
    resetUserPasskey,
    resetUserTwoFA,
    t,
  } = usersData;

  // Modal states
  const [showPromoteModal, setShowPromoteModal] = useState(false);
  const [showDemoteModal, setShowDemoteModal] = useState(false);
  const [showEnableDisableModal, setShowEnableDisableModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [modalUser, setModalUser] = useState(null);
  const [enableDisableAction, setEnableDisableAction] = useState('');
  const [showResetPasskeyModal, setShowResetPasskeyModal] = useState(false);
  const [showResetTwoFAModal, setShowResetTwoFAModal] = useState(false);
  const [showUserSubscriptionsModal, setShowUserSubscriptionsModal] =
    useState(false);
  const canManageUserDanger = useAdminActionPermission(
    ADMIN_PERMISSION_KEYS.userUserDanger,
  );
  const userDangerPermissionDenied = t(
    '没有用户高危操作权限，请联系运营管理员或超级管理员。',
  );

  // Modal handlers
  const showPromoteUserModal = (user) => {
    setModalUser(user);
    setShowPromoteModal(true);
  };

  const showDemoteUserModal = (user) => {
    setModalUser(user);
    setShowDemoteModal(true);
  };

  const showEnableDisableUserModal = (user, action) => {
    setModalUser(user);
    setEnableDisableAction(action);
    setShowEnableDisableModal(true);
  };

  const showDeleteUserModal = (user) => {
    setModalUser(user);
    setShowDeleteModal(true);
  };

  const showResetPasskeyUserModal = (user) => {
    setModalUser(user);
    setShowResetPasskeyModal(true);
  };

  const showResetTwoFAUserModal = (user) => {
    setModalUser(user);
    setShowResetTwoFAModal(true);
  };

  const showUserSubscriptionsUserModal = (user) => {
    setModalUser(user);
    setShowUserSubscriptionsModal(true);
  };

  // Modal confirm handlers
  const handlePromoteConfirm = () => {
    if (!canManageUserDanger) {
      showWarning(userDangerPermissionDenied);
      return;
    }
    manageUser(modalUser.id, 'promote', modalUser);
    setShowPromoteModal(false);
  };

  const handleDemoteConfirm = () => {
    if (!canManageUserDanger) {
      showWarning(userDangerPermissionDenied);
      return;
    }
    manageUser(modalUser.id, 'demote', modalUser);
    setShowDemoteModal(false);
  };

  const handleEnableDisableConfirm = () => {
    if (!canManageUserDanger) {
      showWarning(userDangerPermissionDenied);
      return;
    }
    manageUser(modalUser.id, enableDisableAction, modalUser);
    setShowEnableDisableModal(false);
  };

  const handleResetPasskeyConfirm = async () => {
    if (!canManageUserDanger) {
      showWarning(userDangerPermissionDenied);
      return;
    }
    await resetUserPasskey(modalUser);
    setShowResetPasskeyModal(false);
  };

  const handleResetTwoFAConfirm = async () => {
    if (!canManageUserDanger) {
      showWarning(userDangerPermissionDenied);
      return;
    }
    await resetUserTwoFA(modalUser);
    setShowResetTwoFAModal(false);
  };

  // Get all columns
  const columns = useMemo(() => {
    return getUsersColumns({
      t,
      setEditingUser,
      setShowEditUser,
      showPromoteModal: showPromoteUserModal,
      showDemoteModal: showDemoteUserModal,
      showEnableDisableModal: showEnableDisableUserModal,
      showDeleteModal: showDeleteUserModal,
      showResetPasskeyModal: showResetPasskeyUserModal,
      showResetTwoFAModal: showResetTwoFAUserModal,
      showUserSubscriptionsModal: showUserSubscriptionsUserModal,
      canManageUserDanger,
    });
  }, [
    t,
    setEditingUser,
    setShowEditUser,
    showPromoteUserModal,
    showDemoteUserModal,
    showEnableDisableUserModal,
    showDeleteUserModal,
    showResetPasskeyUserModal,
    showResetTwoFAUserModal,
    showUserSubscriptionsUserModal,
    canManageUserDanger,
  ]);

  // Handle compact mode by removing fixed positioning
  const tableColumns = useMemo(() => {
    return compactMode
      ? columns.map((col) => {
          if (col.dataIndex === 'operate') {
            const { fixed, ...rest } = col;
            return rest;
          }
          return col;
        })
      : columns;
  }, [compactMode, columns]);

  return (
    <>
      <CardTable
        columns={tableColumns}
        dataSource={users}
        scroll={compactMode ? undefined : { x: 'max-content' }}
        pagination={{
          currentPage: activePage,
          pageSize: pageSize,
          total: userCount,
          pageSizeOpts: [10, 20, 50, 100],
          showSizeChanger: true,
          onPageSizeChange: handlePageSizeChange,
          onPageChange: handlePageChange,
        }}
        hidePagination={true}
        loading={loading}
        onRow={handleRow}
        empty={
          <Empty
            image={<IllustrationNoResult style={{ width: 150, height: 150 }} />}
            darkModeImage={
              <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
            }
            description={t('搜索无结果')}
            style={{ padding: 30 }}
          />
        }
        className='overflow-hidden'
        size='middle'
      />

      {/* Modal components */}
      <PromoteUserModal
        visible={showPromoteModal}
        onCancel={() => setShowPromoteModal(false)}
        onConfirm={handlePromoteConfirm}
        user={modalUser}
        t={t}
      />

      <DemoteUserModal
        visible={showDemoteModal}
        onCancel={() => setShowDemoteModal(false)}
        onConfirm={handleDemoteConfirm}
        user={modalUser}
        t={t}
      />

      <EnableDisableUserModal
        visible={showEnableDisableModal}
        onCancel={() => setShowEnableDisableModal(false)}
        onConfirm={handleEnableDisableConfirm}
        user={modalUser}
        action={enableDisableAction}
        t={t}
      />

      <DeleteUserModal
        visible={showDeleteModal}
        onCancel={() => setShowDeleteModal(false)}
        user={modalUser}
        users={users}
        activePage={activePage}
        refresh={refresh}
        manageUser={manageUser}
        canManageUserDanger={canManageUserDanger}
        t={t}
      />

      <ResetPasskeyModal
        visible={showResetPasskeyModal}
        onCancel={() => setShowResetPasskeyModal(false)}
        onConfirm={handleResetPasskeyConfirm}
        user={modalUser}
        t={t}
      />

      <ResetTwoFAModal
        visible={showResetTwoFAModal}
        onCancel={() => setShowResetTwoFAModal(false)}
        onConfirm={handleResetTwoFAConfirm}
        user={modalUser}
        t={t}
      />

      <UserSubscriptionsModal
        visible={showUserSubscriptionsModal}
        onCancel={() => setShowUserSubscriptionsModal(false)}
        user={modalUser}
        t={t}
        onSuccess={() => refresh?.()}
      />
    </>
  );
};

export default UsersTable;
