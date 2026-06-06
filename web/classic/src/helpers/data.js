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

import { API } from './api';
import {
  ADMIN_LEGACY_ROLE,
  hasAdminPermissionSource,
} from '../apps/admin-console/permissions/adminPermissions.config';

export function setStatusData(data) {
  localStorage.setItem('status', JSON.stringify(data));
  localStorage.setItem('system_name', data.system_name);
  localStorage.setItem('logo', data.logo);
  localStorage.setItem('footer_html', data.footer_html);
  localStorage.setItem('quota_per_unit', data.quota_per_unit);
  // 兼容：保留旧字段，同时写入新的额度展示类型
  localStorage.setItem('display_in_currency', data.display_in_currency);
  localStorage.setItem('quota_display_type', data.quota_display_type || 'USD');
  localStorage.setItem('enable_drawing', data.enable_drawing);
  localStorage.setItem('enable_task', data.enable_task);
  localStorage.setItem('enable_data_export', data.enable_data_export);
  localStorage.setItem('chats', JSON.stringify(data.chats));
  localStorage.setItem(
    'data_export_default_time',
    data.data_export_default_time,
  );
  localStorage.setItem(
    'default_collapse_sidebar',
    data.default_collapse_sidebar,
  );
  localStorage.setItem('mj_notify_enabled', data.mj_notify_enabled);
  if (data.chat_link) {
    // localStorage.setItem('chat_link', data.chat_link);
  } else {
    localStorage.removeItem('chat_link');
  }
  if (data.chat_link2) {
    // localStorage.setItem('chat_link2', data.chat_link2);
  } else {
    localStorage.removeItem('chat_link2');
  }
  if (data.docs_link) {
    localStorage.setItem('docs_link', data.docs_link);
  } else {
    localStorage.removeItem('docs_link');
  }
}

export function setUserData(data) {
  localStorage.setItem('user', JSON.stringify(data));
}

export async function hydrateAdminPermissionData(user, options = {}) {
  const { force = false } = options;
  const role = Number(user?.role || 0);
  if (
    !user ||
    role < ADMIN_LEGACY_ROLE ||
    (!force && hasAdminPermissionSource(user))
  ) {
    return user;
  }

  try {
    const res = await API.get('/api/admin/permissions/self', {
      disableDuplicate: true,
      skipErrorHandler: true,
    });
    const { success, data } = res.data || {};
    if (!success || !data) {
      return user;
    }
    const nextUser = {
      ...user,
      admin_permissions: data.admin_permissions,
      admin_permission_mode: data.admin_permission_mode || data.mode,
      admin_permission_source: data.admin_permission_source || data.source,
    };
    setUserData(nextUser);
    return nextUser;
  } catch (error) {
    return user;
  }
}

export async function refreshStoredAdminPermissionData(userDispatch) {
  const rawUser = localStorage.getItem('user');
  if (!rawUser) {
    return undefined;
  }

  const user = JSON.parse(rawUser);
  const nextUser = await hydrateAdminPermissionData(user, { force: true });
  if (nextUser !== user && userDispatch) {
    userDispatch({ type: 'login', payload: nextUser });
  }
  return nextUser;
}

export async function loadStoredUserData(userDispatch) {
  const rawUser = localStorage.getItem('user');
  if (!rawUser) {
    return undefined;
  }

  const user = JSON.parse(rawUser);
  userDispatch({ type: 'login', payload: user });
  const hydratedUser = await hydrateAdminPermissionData(user);
  if (hydratedUser !== user) {
    userDispatch({ type: 'login', payload: hydratedUser });
  }
  return hydratedUser;
}
