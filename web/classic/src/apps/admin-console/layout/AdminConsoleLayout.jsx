import React, { useContext, useMemo } from 'react';
import ConsoleShell from '../../../shared/console-ui/components/ConsoleShell';
import { adminConsoleNavGroups } from '../navigation/adminConsoleNav.config';
import { StatusContext } from '../../../context/Status';
import { UserContext } from '../../../context/User';
import { mergeAdminConfig } from '../../../hooks/common/useSidebar';
import {
  getAdminRoutePermission,
  hasAdminPermission,
} from '../permissions/adminPermissions.config';

const AdminConsoleLayout = ({ children }) => {
  const [statusState] = useContext(StatusContext);
  const [userState] = useContext(UserContext);

  const currentUser = useMemo(() => {
    if (userState?.user) {
      return userState.user;
    }

    try {
      return JSON.parse(localStorage.getItem('user')) || {};
    } catch (error) {
      return {};
    }
  }, [userState?.user]);

  const adminModulesConfig = useMemo(() => {
    if (!statusState?.status?.SidebarModulesAdmin) {
      return mergeAdminConfig(null);
    }

    try {
      return mergeAdminConfig(
        JSON.parse(statusState.status.SidebarModulesAdmin),
      );
    } catch (error) {
      return mergeAdminConfig(null);
    }
  }, [statusState?.status?.SidebarModulesAdmin]);

  const navGroups = useMemo(() => {
    return adminConsoleNavGroups
      .map((group) => ({
        ...group,
        items: (group.items || []).filter((item) => {
          if (!item.sidebarSection || !item.sidebarModule) {
            return hasAdminPermission(
              currentUser,
              item.permission || getAdminRoutePermission(item.path),
            );
          }

          return (
            adminModulesConfig[item.sidebarSection]?.enabled &&
            adminModulesConfig[item.sidebarSection]?.[item.sidebarModule] &&
            hasAdminPermission(
              currentUser,
              item.permission || getAdminRoutePermission(item.path),
            )
          );
        }),
      }))
      .filter((group) => group.items.length > 0);
  }, [adminModulesConfig, currentUser]);

  return (
    <ConsoleShell
      variant='admin'
      title='CodeToken AI Admin'
      subtitle='管理员后台'
      baseLabel='管理员后台'
      envLabel='Pro'
      navGroups={navGroups}
    >
      {children}
    </ConsoleShell>
  );
};

export default AdminConsoleLayout;
