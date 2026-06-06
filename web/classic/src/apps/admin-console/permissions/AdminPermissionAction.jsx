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

import React, { useContext, useMemo } from 'react';
import { Button, Tooltip } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { UserContext } from '../../../context/User';
import { hasAdminPermission } from './adminPermissions.config';

function getLocalUser() {
  try {
    return JSON.parse(localStorage.getItem('user')) || {};
  } catch (error) {
    return {};
  }
}

function useCurrentUser() {
  const userContext = useContext(UserContext);
  const contextUser = Array.isArray(userContext)
    ? userContext[0]?.user
    : userContext?.state?.user;

  return useMemo(() => contextUser || getLocalUser(), [contextUser]);
}

export function useAdminActionPermission(permission, options) {
  const user = useCurrentUser();
  return hasAdminPermission(user, permission, options);
}

export const AdminPermissionButton = ({
  requiredPermission,
  dangerPermission,
  fallbackTooltip,
  disabled,
  children,
  onClick,
  tooltipPosition = 'top',
  ...buttonProps
}) => {
  const { t } = useTranslation();
  const permission = dangerPermission || requiredPermission;
  const allowed = useAdminActionPermission(permission);
  const isBlocked = Boolean(permission) && !allowed;
  const disabledReason =
    fallbackTooltip || t('当前账号没有执行该操作的权限，请联系超级管理员。');

  const button = (
    <Button
      {...buttonProps}
      disabled={disabled || isBlocked}
      onClick={isBlocked ? undefined : onClick}
    >
      {children}
    </Button>
  );

  if (!isBlocked) {
    return button;
  }

  return (
    <Tooltip content={disabledReason} position={tooltipPosition}>
      <span className='aurora-permission-action-disabled'>{button}</span>
    </Tooltip>
  );
};
