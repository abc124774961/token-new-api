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

import React, { lazy } from 'react';
import { Navigate, Route, useLocation, useParams } from 'react-router-dom';
import { AdminRoute } from '../../../helpers';
import AdminConsoleLayout from '../layout/AdminConsoleLayout';
import { getAdminRoutePermission } from '../permissions/adminPermissions.config';
import { legacyAdminRedirects } from './adminLegacyRedirects.config';

const AdminOverview = lazy(() => import('../pages/AdminOverview'));
const AdminChannels = lazy(() => import('../pages/AdminChannels'));
const AdminChannelAccounts = lazy(
  () => import('../pages/AdminChannelAccounts'),
);
const AdminChannelBalanceMonitor = lazy(
  () => import('../pages/AdminChannelBalanceMonitor'),
);
const AdminChannelHealthCheck = lazy(
  () => import('../pages/AdminChannelHealthCheck'),
);
const AdminChannelProxies = lazy(() => import('../pages/AdminChannelProxies'));
const AdminModelGateway = lazy(() => import('../pages/AdminModelGateway'));
const AdminModels = lazy(() => import('../pages/AdminModels'));
const AdminModelDeployments = lazy(
  () => import('../pages/AdminModelDeployments'),
);
const AdminProfitMonitor = lazy(() => import('../pages/AdminProfitMonitor'));
const AdminSubscriptions = lazy(() => import('../pages/AdminSubscriptions'));
const AdminRedemptions = lazy(() => import('../pages/AdminRedemptions'));
const AdminSettlements = lazy(() => import('../pages/AdminSettlements'));
const AdminConsumption = lazy(() => import('../pages/AdminConsumption'));
const AdminUsers = lazy(() => import('../pages/AdminUsers'));
const AdminUserSegments = lazy(() => import('../pages/AdminUserSegments'));
const AdminRiskRecords = lazy(() => import('../pages/AdminRiskRecords'));
const AdminInviteRebates = lazy(() => import('../pages/AdminInviteRebates'));
const AdminSettings = lazy(() => import('../pages/AdminSettings'));
const AdminRoles = lazy(() => import('../pages/AdminRoles'));
const AdminAuditLogs = lazy(() => import('../pages/AdminAuditLogs'));
const AdminBackgroundTasks = lazy(
  () => import('../pages/AdminBackgroundTasks'),
);
const AdminRealtimeMonitor = lazy(
  () => import('../pages/AdminRealtimeMonitor'),
);
const AdminChannelAlerts = lazy(() => import('../pages/AdminChannelAlerts'));
const AdminSmartScheduler = lazy(() => import('../pages/AdminSmartScheduler'));
const AdminRatioConfig = lazy(() => import('../pages/AdminRatioConfig'));

function AdminChannelAccountLegacyRedirect() {
  const { id } = useParams();
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  if (id && !params.has('channel_id')) {
    params.set('channel_id', id);
  }

  const search = params.toString();
  return (
    <Navigate
      to={{
        pathname: '/admin/channel-accounts',
        search: search ? `?${search}` : '',
      }}
      replace
    />
  );
}

function LegacyAdminRedirect({ to }) {
  const location = useLocation();
  return (
    <Navigate
      to={{
        pathname: to,
        search: location.search,
      }}
      replace
    />
  );
}

function RoutePolicyLegacyRedirect() {
  const location = useLocation();
  const params = new URLSearchParams(location.search);
  params.set('tab', 'policy');
  return (
    <Navigate
      to={{
        pathname: '/admin/smart-scheduler',
        search: `?${params.toString()}`,
      }}
      replace
    />
  );
}

function AdminConsoleRoute({ children }) {
  const location = useLocation();
  const permission = getAdminRoutePermission(location.pathname);

  return (
    <AdminRoute permission={permission}>
      <AdminConsoleLayout>{children}</AdminConsoleLayout>
    </AdminRoute>
  );
}

export function renderAdminRouteElements() {
  return (
    <>
      <Route
        path='/admin'
        element={<Navigate to='/admin/overview' replace />}
      />
      <Route
        path='/admin/overview'
        element={
          <AdminConsoleRoute>
            <AdminOverview />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/realtime-monitor'
        element={
          <AdminConsoleRoute>
            <AdminRealtimeMonitor />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/channel-alerts'
        element={
          <AdminConsoleRoute>
            <AdminChannelAlerts />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/channels'
        element={
          <AdminConsoleRoute>
            <AdminChannels />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/channel-accounts'
        element={
          <AdminConsoleRoute>
            <AdminChannelAccounts />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/channel/:id/accounts'
        element={
          <AdminConsoleRoute>
            <AdminChannelAccountLegacyRedirect />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/channel-balance-monitor'
        element={
          <AdminConsoleRoute>
            <AdminChannelBalanceMonitor />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/channel-health-check'
        element={
          <AdminConsoleRoute>
            <AdminChannelHealthCheck />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/channel-proxies'
        element={
          <AdminConsoleRoute>
            <AdminChannelProxies />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/model-gateway'
        element={
          <AdminConsoleRoute>
            <AdminModelGateway />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/models'
        element={
          <AdminConsoleRoute>
            <AdminModels />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/deployment'
        element={
          <AdminConsoleRoute>
            <AdminModelDeployments />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/route-policy'
        element={<RoutePolicyLegacyRedirect />}
      />
      <Route
        path='/admin/smart-scheduler'
        element={
          <AdminConsoleRoute>
            <AdminSmartScheduler />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/ratio-config'
        element={
          <AdminConsoleRoute>
            <AdminRatioConfig />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/profit-monitor'
        element={
          <AdminConsoleRoute>
            <AdminProfitMonitor />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/subscription'
        element={
          <AdminConsoleRoute>
            <AdminSubscriptions />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/redemption'
        element={
          <AdminConsoleRoute>
            <AdminRedemptions />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/settlements'
        element={
          <AdminConsoleRoute>
            <AdminSettlements />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/consumption'
        element={
          <AdminConsoleRoute>
            <AdminConsumption />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/users'
        element={
          <AdminConsoleRoute>
            <AdminUsers />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/user-segments'
        element={
          <AdminConsoleRoute>
            <AdminUserSegments />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/risk-records'
        element={
          <AdminConsoleRoute>
            <AdminRiskRecords />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/invite-rebates'
        element={
          <AdminConsoleRoute>
            <AdminInviteRebates />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/settings'
        element={
          <AdminConsoleRoute>
            <AdminSettings />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/roles'
        element={
          <AdminConsoleRoute>
            <AdminRoles />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/audit-logs'
        element={
          <AdminConsoleRoute>
            <AdminAuditLogs />
          </AdminConsoleRoute>
        }
      />
      <Route
        path='/admin/background-tasks'
        element={
          <AdminConsoleRoute>
            <AdminBackgroundTasks />
          </AdminConsoleRoute>
        }
      />

      {legacyAdminRedirects.map(({ from, to }) => (
        <Route
          element={<LegacyAdminRedirect to={to} />}
          key={from}
          path={from}
        />
      ))}
      <Route
        path='/console/channel/:id/accounts'
        element={<AdminChannelAccountLegacyRedirect />}
      />
    </>
  );
}
