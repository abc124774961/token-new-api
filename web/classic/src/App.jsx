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

import React, { lazy, Suspense, useContext, useMemo } from 'react';
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useParams,
} from 'react-router-dom';
import Loading from './components/common/ui/Loading';
import { AuthRedirect, PrivateRoute } from './helpers';
import { StatusContext } from './context/Status';
import SetupCheck from './components/layout/SetupCheck';
import {
  isPricingAuthRequired,
  parseHeaderNavModulesConfig,
} from './constants/header-nav.constants';
import { renderAdminRouteElements } from './apps/admin-console/routes/adminRoutes';

const Home = lazy(() => import('./pages/Home'));
const IntegrationDocs = lazy(() => import('./pages/IntegrationDocs'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const ChannelStatus = lazy(() => import('./pages/ChannelStatus'));
const About = lazy(() => import('./pages/About'));
const UserAgreement = lazy(() => import('./pages/UserAgreement'));
const PrivacyPolicy = lazy(() => import('./pages/PrivacyPolicy'));
const RegisterForm = lazy(() => import('./components/auth/RegisterForm'));
const LoginForm = lazy(() => import('./components/auth/LoginForm'));
const PasswordResetForm = lazy(
  () => import('./components/auth/PasswordResetForm'),
);
const PasswordResetConfirm = lazy(
  () => import('./components/auth/PasswordResetConfirm'),
);
const OAuth2Callback = lazy(() => import('./components/auth/OAuth2Callback'));
const PersonalSetting = lazy(
  () => import('./components/settings/PersonalSetting'),
);
const NotFound = lazy(() => import('./pages/NotFound'));
const Forbidden = lazy(() => import('./pages/Forbidden'));
const Token = lazy(() => import('./pages/Token'));
const TopUp = lazy(() => import('./pages/TopUp'));
const Log = lazy(() => import('./pages/Log'));
const Chat = lazy(() => import('./pages/Chat'));
const Chat2Link = lazy(() => import('./pages/Chat2Link'));
const Midjourney = lazy(() => import('./pages/Midjourney'));
const Pricing = lazy(() => import('./pages/Pricing'));
const Task = lazy(() => import('./pages/Task'));
const Playground = lazy(() => import('./pages/Playground'));
const Setup = lazy(() => import('./pages/Setup'));
const UserConsoleLayout = lazy(
  () => import('./apps/user-console/layout/UserConsoleLayout'),
);

function DynamicOAuth2Callback() {
  const { provider } = useParams();
  return <OAuth2Callback type={provider} />;
}

function UserConsoleRoute({ children }) {
  return (
    <PrivateRoute>
      <UserConsoleLayout>{children}</UserConsoleLayout>
    </PrivateRoute>
  );
}

function App() {
  const location = useLocation();
  const [statusState] = useContext(StatusContext);

  // 获取模型广场权限配置
  const pricingRequireAuth = useMemo(() => {
    return isPricingAuthRequired(
      parseHeaderNavModulesConfig(statusState?.status?.HeaderNavModules),
    );
  }, [statusState?.status?.HeaderNavModules]);

  return (
    <SetupCheck>
      <Suspense fallback={<Loading></Loading>}>
        <Routes>
          <Route
            path='/'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Home />
              </Suspense>
            }
          />
          <Route
            path='/setup'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Setup />
              </Suspense>
            }
          />
          <Route
            path='/integration-docs'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <IntegrationDocs />
              </Suspense>
            }
          />
          <Route path='/forbidden' element={<Forbidden />} />
          {renderAdminRouteElements()}
          <Route
            path='/console/channel-status'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <ChannelStatus />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console/token'
            element={
              <UserConsoleRoute>
                <Token />
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console/playground'
            element={
              <UserConsoleRoute>
                <Playground />
              </UserConsoleRoute>
            }
          />
          <Route
            path='/user/reset'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <PasswordResetConfirm />
              </Suspense>
            }
          />
          <Route
            path='/login'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <AuthRedirect>
                  <LoginForm />
                </AuthRedirect>
              </Suspense>
            }
          />
          <Route
            path='/register'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <AuthRedirect>
                  <RegisterForm />
                </AuthRedirect>
              </Suspense>
            }
          />
          <Route
            path='/reset'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <PasswordResetForm />
              </Suspense>
            }
          />
          <Route
            path='/oauth/github'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <OAuth2Callback type='github'></OAuth2Callback>
              </Suspense>
            }
          />
          <Route
            path='/oauth/discord'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <OAuth2Callback type='discord'></OAuth2Callback>
              </Suspense>
            }
          />
          <Route
            path='/oauth/oidc'
            element={
              <Suspense fallback={<Loading></Loading>}>
                <OAuth2Callback type='oidc'></OAuth2Callback>
              </Suspense>
            }
          />
          <Route
            path='/oauth/linuxdo'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <OAuth2Callback type='linuxdo'></OAuth2Callback>
              </Suspense>
            }
          />
          <Route
            path='/oauth/:provider'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <DynamicOAuth2Callback />
              </Suspense>
            }
          />
          <Route
            path='/console/personal'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <PersonalSetting />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console/topup'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Navigate
                    to={{
                      pathname: '/console/recharge',
                      search: location.search,
                    }}
                    replace
                  />
                </Suspense>
              </PrivateRoute>
            }
          />
          <Route
            path='/console/affiliate'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <TopUp view='affiliate' />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console/recharge'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <TopUp view='recharge' />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console/subscription-plans'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <TopUp view='subscription' />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console/log'
            element={
              <UserConsoleRoute>
                <Log />
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Dashboard />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console/midjourney'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Midjourney />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          <Route
            path='/console/task'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Task />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          <Route
            path='/pricing'
            element={
              pricingRequireAuth ? (
                <PrivateRoute>
                  <Suspense
                    fallback={<Loading></Loading>}
                    key={location.pathname}
                  >
                    <Pricing />
                  </Suspense>
                </PrivateRoute>
              ) : (
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Pricing />
                </Suspense>
              )
            }
          />
          <Route
            path='/about'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <About />
              </Suspense>
            }
          />
          <Route
            path='/user-agreement'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <UserAgreement />
              </Suspense>
            }
          />
          <Route
            path='/privacy-policy'
            element={
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <PrivacyPolicy />
              </Suspense>
            }
          />
          <Route
            path='/console/chat/:id?'
            element={
              <UserConsoleRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Chat />
                </Suspense>
              </UserConsoleRoute>
            }
          />
          {/* 方便使用chat2link直接跳转聊天... */}
          <Route
            path='/chat2link'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Chat2Link />
                </Suspense>
              </PrivateRoute>
            }
          />
          <Route path='*' element={<NotFound />} />
        </Routes>
      </Suspense>
    </SetupCheck>
  );
}

export default App;
