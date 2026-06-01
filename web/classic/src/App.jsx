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
import { AuthRedirect, PrivateRoute, AdminRoute } from './helpers';
import { StatusContext } from './context/Status';
import SetupCheck from './components/layout/SetupCheck';
import {
  isPricingAuthRequired,
  parseHeaderNavModulesConfig,
} from './constants/header-nav.constants';

const Home = lazy(() => import('./pages/Home'));
const IntegrationDocs = lazy(() => import('./pages/IntegrationDocs'));
const User = lazy(() => import('./pages/User'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const ChannelStatus = lazy(() => import('./pages/ChannelStatus'));
const ModelGateway = lazy(() => import('./pages/ModelGateway'));
const ChannelHealthCheck = lazy(() => import('./pages/ChannelHealthCheck'));
const ProfitMonitor = lazy(() => import('./pages/ProfitMonitor'));
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
const Setting = lazy(() => import('./pages/Setting'));
const Channel = lazy(() => import('./pages/Channel'));
const ChannelAccount = lazy(() => import('./pages/ChannelAccount'));
const ChannelBalanceMonitor = lazy(
  () => import('./pages/ChannelBalanceMonitor'),
);
const ChannelProxy = lazy(() => import('./pages/ChannelProxy'));
const Token = lazy(() => import('./pages/Token'));
const Redemption = lazy(() => import('./pages/Redemption'));
const TopUp = lazy(() => import('./pages/TopUp'));
const Log = lazy(() => import('./pages/Log'));
const Chat = lazy(() => import('./pages/Chat'));
const Chat2Link = lazy(() => import('./pages/Chat2Link'));
const Midjourney = lazy(() => import('./pages/Midjourney'));
const Pricing = lazy(() => import('./pages/Pricing'));
const Task = lazy(() => import('./pages/Task'));
const ModelPage = lazy(() => import('./pages/Model'));
const ModelDeploymentPage = lazy(() => import('./pages/ModelDeployment'));
const Playground = lazy(() => import('./pages/Playground'));
const Subscription = lazy(() => import('./pages/Subscription'));
const Setup = lazy(() => import('./pages/Setup'));

function DynamicOAuth2Callback() {
  const { provider } = useParams();
  return <OAuth2Callback type={provider} />;
}

function ChannelAccountLegacyRedirect() {
  const { id } = useParams();
  return <Navigate to={`/console/channel/accounts?channel_id=${id}`} replace />;
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
          <Route
            path='/console/models'
            element={
              <AdminRoute>
                <ModelPage />
              </AdminRoute>
            }
          />
          <Route
            path='/console/deployment'
            element={
              <AdminRoute>
                <ModelDeploymentPage />
              </AdminRoute>
            }
          />
          <Route
            path='/console/subscription'
            element={
              <AdminRoute>
                <Subscription />
              </AdminRoute>
            }
          />
          <Route
            path='/console/channel'
            element={
              <AdminRoute>
                <Channel />
              </AdminRoute>
            }
          />
          <Route
            path='/console/channel/accounts'
            element={
              <AdminRoute>
                <ChannelAccount />
              </AdminRoute>
            }
          />
          <Route
            path='/console/channel/:id/accounts'
            element={
              <AdminRoute>
                <ChannelAccountLegacyRedirect />
              </AdminRoute>
            }
          />
          <Route
            path='/console/channel-balance-monitor'
            element={
              <AdminRoute>
                <ChannelBalanceMonitor />
              </AdminRoute>
            }
          />
          <Route
            path='/console/channel-proxies'
            element={
              <AdminRoute>
                <ChannelProxy />
              </AdminRoute>
            }
          />
          <Route
            path='/console/channel-status'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <ChannelStatus />
                </Suspense>
              </PrivateRoute>
            }
          />
          <Route
            path='/console/model-gateway'
            element={
              <AdminRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <ModelGateway />
                </Suspense>
              </AdminRoute>
            }
          />
          <Route
            path='/console/channel-health-check'
            element={
              <AdminRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <ChannelHealthCheck />
                </Suspense>
              </AdminRoute>
            }
          />
          <Route
            path='/console/profit-monitor'
            element={
              <AdminRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <ProfitMonitor />
                </Suspense>
              </AdminRoute>
            }
          />
          <Route
            path='/console/token'
            element={
              <PrivateRoute>
                <Token />
              </PrivateRoute>
            }
          />
          <Route
            path='/console/playground'
            element={
              <PrivateRoute>
                <Playground />
              </PrivateRoute>
            }
          />
          <Route
            path='/console/redemption'
            element={
              <AdminRoute>
                <Redemption />
              </AdminRoute>
            }
          />
          <Route
            path='/console/user'
            element={
              <AdminRoute>
                <User />
              </AdminRoute>
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
            path='/console/setting'
            element={
              <AdminRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Setting />
                </Suspense>
              </AdminRoute>
            }
          />
          <Route
            path='/console/personal'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <PersonalSetting />
                </Suspense>
              </PrivateRoute>
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
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <TopUp view='affiliate' />
                </Suspense>
              </PrivateRoute>
            }
          />
          <Route
            path='/console/recharge'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <TopUp view='recharge' />
                </Suspense>
              </PrivateRoute>
            }
          />
          <Route
            path='/console/subscription-plans'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <TopUp view='subscription' />
                </Suspense>
              </PrivateRoute>
            }
          />
          <Route
            path='/console/log'
            element={
              <PrivateRoute>
                <Log />
              </PrivateRoute>
            }
          />
          <Route
            path='/console'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Dashboard />
                </Suspense>
              </PrivateRoute>
            }
          />
          <Route
            path='/console/midjourney'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Midjourney />
                </Suspense>
              </PrivateRoute>
            }
          />
          <Route
            path='/console/task'
            element={
              <PrivateRoute>
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Task />
                </Suspense>
              </PrivateRoute>
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
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Chat />
              </Suspense>
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
