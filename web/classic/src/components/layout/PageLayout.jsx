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

import HeaderBar from './headerbar';
import { Layout } from '@douyinfe/semi-ui';
import SiderBar from './SiderBar';
import App from '../../App';
import FooterBar from './Footer';
import DomainMigrationNotice from './DomainMigrationNotice';
import { SupportContactsFloatingButton } from './SupportContacts';
import { ToastContainer } from 'react-toastify';
import ErrorBoundary from '../common/ErrorBoundary';
import React, { useContext, useEffect, useState } from 'react';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { useSidebarCollapsed } from '../../hooks/common/useSidebarCollapsed';
import { useTranslation } from 'react-i18next';
import { isUserConsoleShellRoute } from '../../apps/user-console/routes/userConsoleRoutes.config';
import {
  API,
  getLogo,
  getSystemName,
  showError,
  setStatusData,
  loadStoredUserData,
} from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { useLocation } from 'react-router-dom';
import { normalizeLanguage } from '../../i18n/language';
const { Sider, Content, Header } = Layout;

const applyDocumentBranding = (systemName, logo) => {
  if (systemName) {
    document.title = systemName;
  }
  if (logo) {
    const linkElement = document.querySelector("link[rel~='icon']");
    if (linkElement) {
      linkElement.href = logo;
    }
  }
};

const PageLayout = () => {
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState, statusDispatch] = useContext(StatusContext);
  const isMobile = useIsMobile();
  const [collapsed, , setCollapsed] = useSidebarCollapsed();
  const [drawerOpen, setDrawerOpen] = useState(false);
  const { i18n } = useTranslation();
  const location = useLocation();

  const cardProPages = [
    '/console/channel',
    '/console/channel/accounts',
    '/console/channel-status',
    '/console/log',
    '/console/redemption',
    '/console/user',
    '/console/token',
    '/console/midjourney',
    '/console/task',
    '/console/models',
    '/pricing',
  ];

  const shouldHideFooter = cardProPages.includes(location.pathname);

  const shouldInnerPadding =
    location.pathname.includes('/console') &&
    !location.pathname.startsWith('/console/chat') &&
    location.pathname !== '/console/playground';

  const isConsoleRoute = location.pathname.startsWith('/console');
  const isAdminRoute = location.pathname.startsWith('/admin');
  const isUserShellRoute = isUserConsoleShellRoute(location.pathname);
  const isAuroraShellRoute = isAdminRoute || isUserShellRoute;
  const isHomeRoute = location.pathname === '/';
  const isAuthRoute =
    ['/login', '/register', '/reset', '/user/reset'].includes(
      location.pathname,
    ) || location.pathname.startsWith('/oauth/');
  const isPublicRoute = [
    '/',
    '/pricing',
    '/integration-docs',
    '/about',
    '/user-agreement',
    '/privacy-policy',
  ].includes(location.pathname);
  const isAuroraSiteRoute = isAuthRoute || isPublicRoute;
  const isTopupRoute = [
    '/console/affiliate',
    '/console/recharge',
    '/console/subscription-plans',
  ].includes(location.pathname);
  const isPaymentGrowthRoute = [
    '/console/recharge',
    '/console/subscription-plans',
  ].includes(location.pathname);
  const paymentGrowthRouteClassMap = {
    '/console/affiliate': 'ct-topup-affiliate-route',
    '/console/recharge': 'ct-topup-recharge-route',
    '/console/subscription-plans': 'ct-topup-subscription-route',
  };
  const showSider =
    isConsoleRoute && !isAuroraShellRoute && (!isMobile || drawerOpen);

  useEffect(() => {
    const routeClasses = Object.values(paymentGrowthRouteClassMap);
    document.body.classList.toggle('ct-aurora-shell-route', isAuroraShellRoute);
    document.body.classList.toggle('ct-aurora-site-route', isAuroraSiteRoute);
    document.body.classList.toggle('ct-aurora-auth-route', isAuthRoute);
    document.body.classList.toggle(
      'ct-aurora-public-route',
      isPublicRoute && !isAuthRoute,
    );
    document.body.classList.toggle(
      'ct-aurora-pricing-route',
      location.pathname === '/pricing',
    );
    document.body.classList.toggle(
      'ct-aurora-docs-route',
      location.pathname === '/integration-docs',
    );
    document.body.classList.toggle(
      'ct-aurora-policy-route',
      ['/about', '/user-agreement', '/privacy-policy'].includes(
        location.pathname,
      ),
    );
    document.body.classList.toggle(
      'ct-payment-growth-route',
      isPaymentGrowthRoute,
    );
    routeClasses.forEach((className) => {
      document.body.classList.toggle(
        className,
        className === paymentGrowthRouteClassMap[location.pathname],
      );
    });

    return () => {
      document.body.classList.remove('ct-aurora-shell-route');
      document.body.classList.remove('ct-aurora-site-route');
      document.body.classList.remove('ct-aurora-auth-route');
      document.body.classList.remove('ct-aurora-public-route');
      document.body.classList.remove('ct-aurora-pricing-route');
      document.body.classList.remove('ct-aurora-docs-route');
      document.body.classList.remove('ct-aurora-policy-route');
      document.body.classList.remove('ct-payment-growth-route');
      routeClasses.forEach((className) => {
        document.body.classList.remove(className);
      });
    };
  }, [
    isAuroraShellRoute,
    isAuroraSiteRoute,
    isAuthRoute,
    isPublicRoute,
    isPaymentGrowthRoute,
    location.pathname,
  ]);

  useEffect(() => {
    if (isMobile && drawerOpen && collapsed) {
      setCollapsed(false);
    }
  }, [isMobile, drawerOpen, collapsed, setCollapsed]);

  const loadUser = async () => {
    await loadStoredUserData(userDispatch);
  };

  const loadStatus = async () => {
    try {
      const res = await API.get('/api/status', {
        skipErrorHandler: isHomeRoute || isTopupRoute,
      });
      const { success, data } = res.data;
      if (success) {
        statusDispatch({ type: 'set', payload: data });
        setStatusData(data);
        applyDocumentBranding(data.system_name, data.logo);
      } else if (!isHomeRoute && !isTopupRoute) {
        showError('Unable to connect to server');
      }
    } catch (error) {
      if (!isHomeRoute && !isTopupRoute) {
        showError('Failed to load status');
      }
    }
  };

  useEffect(() => {
    loadUser().catch(console.error);
    loadStatus().catch(console.error);
    applyDocumentBranding(getSystemName(), getLogo());
  }, []);

  useEffect(() => {
    applyDocumentBranding(
      statusState?.status?.system_name || getSystemName(),
      statusState?.status?.logo || getLogo(),
    );
  }, [statusState?.status?.logo, statusState?.status?.system_name]);

  useEffect(() => {
    let preferredLang;

    if (userState?.user?.setting) {
      try {
        const settings = JSON.parse(userState.user.setting);
        preferredLang = normalizeLanguage(settings.language);
      } catch (e) {
        // Ignore parse errors
      }
    }

    if (!preferredLang) {
      const savedLang = localStorage.getItem('i18nextLng');
      if (savedLang) {
        preferredLang = normalizeLanguage(savedLang);
      }
    }

    if (preferredLang) {
      localStorage.setItem('i18nextLng', preferredLang);
      if (preferredLang !== i18n.language) {
        i18n.changeLanguage(preferredLang);
      }
    }
  }, [i18n, userState?.user?.setting]);

  return (
    <Layout
      className='app-layout'
      style={{
        display: 'flex',
        flexDirection: 'column',
        overflow: isMobile ? 'visible' : 'hidden',
      }}
    >
      {!isAuroraShellRoute && (
        <Header
          style={{
            padding: 0,
            height: 'auto',
            lineHeight: 'normal',
            position: 'fixed',
            width: '100%',
            top: 0,
            zIndex: 100,
          }}
        >
          <HeaderBar
            onMobileMenuToggle={() => setDrawerOpen((prev) => !prev)}
            drawerOpen={drawerOpen}
          />
        </Header>
      )}
      <Layout
        style={{
          overflow: isMobile
            ? 'visible'
            : isAuroraShellRoute
              ? 'hidden'
              : 'auto',
          display: 'flex',
          flexDirection: 'column',
          minHeight: 0,
        }}
      >
        {showSider && (
          <Sider
            className='app-sider'
            style={{
              position: 'fixed',
              left: 0,
              top: '64px',
              zIndex: 99,
              border: 'none',
              paddingRight: '0',
              width: 'var(--sidebar-current-width)',
            }}
          >
            <SiderBar
              onNavigate={() => {
                if (isMobile) setDrawerOpen(false);
              }}
            />
          </Sider>
        )}
        <Layout
          style={{
            marginLeft: isMobile
              ? '0'
              : showSider
                ? 'var(--sidebar-current-width)'
                : '0',
            paddingTop: isConsoleRoute && !isAuroraShellRoute ? '64px' : '0',
            flex: isAuroraShellRoute ? '1 1 0' : '1 1 auto',
            minHeight: 0,
            display: 'flex',
            flexDirection: 'column',
          }}
        >
          <Content
            style={{
              flex: isAuroraShellRoute ? '1 1 0' : '1 0 auto',
              minHeight: 0,
              overflowY: isMobile ? 'visible' : 'hidden',
              WebkitOverflowScrolling: 'touch',
              padding:
                shouldInnerPadding && !isAuroraShellRoute
                  ? isMobile
                    ? '5px'
                    : '24px'
                  : '0',
              position: 'relative',
            }}
          >
            <ErrorBoundary>
              <App />
            </ErrorBoundary>
          </Content>
          {!isAuroraShellRoute && !shouldHideFooter && (
            <Layout.Footer
              style={{
                flex: '0 0 auto',
                width: '100%',
              }}
            >
              <FooterBar />
            </Layout.Footer>
          )}
        </Layout>
      </Layout>
      <ToastContainer />
      <DomainMigrationNotice />
      <SupportContactsFloatingButton />
    </Layout>
  );
};

export default PageLayout;
