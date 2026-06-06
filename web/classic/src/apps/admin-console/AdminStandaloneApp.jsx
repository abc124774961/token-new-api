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

import React, { lazy, Suspense, useContext, useEffect } from 'react';
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useParams,
} from 'react-router-dom';
import { ToastContainer } from 'react-toastify';
import Loading from '../../components/common/ui/Loading';
import ErrorBoundary from '../../components/common/ErrorBoundary';
import SetupCheck from '../../components/layout/SetupCheck';
import { AuthRedirect } from '../../helpers';
import {
  API,
  getLogo,
  getSystemName,
  loadStoredUserData,
  setStatusData,
  showError,
} from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { normalizeLanguage } from '../../i18n/language';
import { useTranslation } from 'react-i18next';
import { renderAdminRouteElements } from './routes/adminRoutes';

const Setup = lazy(() => import('../../pages/Setup'));
const NotFound = lazy(() => import('../../pages/NotFound'));
const Forbidden = lazy(() => import('../../pages/Forbidden'));
const RegisterForm = lazy(() => import('../../components/auth/RegisterForm'));
const LoginForm = lazy(() => import('../../components/auth/LoginForm'));
const PasswordResetForm = lazy(
  () => import('../../components/auth/PasswordResetForm'),
);
const PasswordResetConfirm = lazy(
  () => import('../../components/auth/PasswordResetConfirm'),
);
const OAuth2Callback = lazy(
  () => import('../../components/auth/OAuth2Callback'),
);

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

function AdminRuntimeBootstrap() {
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState, statusDispatch] = useContext(StatusContext);
  const { i18n } = useTranslation();
  const location = useLocation();
  const suppressStatusError = [
    '/login',
    '/register',
    '/reset',
    '/user/reset',
    '/setup',
  ].includes(location.pathname);

  useEffect(() => {
    loadStoredUserData(userDispatch).catch(console.error);
    applyDocumentBranding(getSystemName(), getLogo());
  }, [userDispatch]);

  useEffect(() => {
    const loadStatus = async () => {
      try {
        const res = await API.get('/api/status', {
          skipErrorHandler: suppressStatusError,
        });
        const { success, data } = res.data;
        if (success) {
          statusDispatch({ type: 'set', payload: data });
          setStatusData(data);
          applyDocumentBranding(data.system_name, data.logo);
        } else if (!suppressStatusError) {
          showError('Unable to connect to server');
        }
      } catch (error) {
        if (!suppressStatusError) {
          showError('Failed to load status');
        }
      }
    };

    loadStatus().catch(console.error);
  }, [statusDispatch, suppressStatusError]);

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

  return null;
}

function DynamicOAuth2Callback() {
  const { provider } = useParams();
  return <OAuth2Callback type={provider} />;
}

export default function AdminStandaloneApp() {
  return (
    <SetupCheck>
      <AdminRuntimeBootstrap />
      <ErrorBoundary>
        <Suspense fallback={<Loading />}>
          <Routes>
            <Route
              path='/'
              element={<Navigate to='/admin/overview' replace />}
            />
            <Route
              path='/setup'
              element={
                <Suspense fallback={<Loading />}>
                  <Setup />
                </Suspense>
              }
            />
            <Route path='/forbidden' element={<Forbidden />} />
            <Route
              path='/login'
              element={
                <AuthRedirect to='/admin/overview'>
                  <LoginForm />
                </AuthRedirect>
              }
            />
            <Route
              path='/register'
              element={
                <AuthRedirect to='/admin/overview'>
                  <RegisterForm />
                </AuthRedirect>
              }
            />
            <Route path='/reset' element={<PasswordResetForm />} />
            <Route path='/user/reset' element={<PasswordResetConfirm />} />
            <Route
              path='/oauth/github'
              element={<OAuth2Callback type='github' />}
            />
            <Route
              path='/oauth/discord'
              element={<OAuth2Callback type='discord' />}
            />
            <Route
              path='/oauth/oidc'
              element={<OAuth2Callback type='oidc' />}
            />
            <Route
              path='/oauth/linuxdo'
              element={<OAuth2Callback type='linuxdo' />}
            />
            <Route
              path='/oauth/:provider'
              element={<DynamicOAuth2Callback />}
            />

            {renderAdminRouteElements()}
            <Route path='*' element={<NotFound />} />
          </Routes>
        </Suspense>
      </ErrorBoundary>
      <ToastContainer />
    </SetupCheck>
  );
}
