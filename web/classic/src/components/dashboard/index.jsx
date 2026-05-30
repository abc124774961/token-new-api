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

import React, { useContext, useEffect } from 'react';
import { getRelativeTime } from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';

import DashboardHeader from './DashboardHeader';
import StatsCards from './StatsCards';
import ChartsPanel from './ChartsPanel';
import ApiInfoPanel from './ApiInfoPanel';
import SubscriptionOverviewPanel from './SubscriptionOverviewPanel';
import AnnouncementsPanel from './AnnouncementsPanel';
import FaqPanel from './FaqPanel';
import UptimePanel from './UptimePanel';
import './dashboard-modern.css';

import { useDashboardData } from '../../hooks/dashboard/useDashboardData';
import { useDashboardStats } from '../../hooks/dashboard/useDashboardStats';
import { useDashboardCharts } from '../../hooks/dashboard/useDashboardCharts';

import {
  CHART_CONFIG,
  ANNOUNCEMENT_LEGEND_DATA,
  UPTIME_STATUS_MAP,
} from '../../constants/dashboard.constants';
import {
  handleCopyUrl,
  handleSpeedTest,
  getUptimeStatusColor,
  getUptimeStatusText,
  renderMonitorList,
} from '../../helpers/dashboard';

const Dashboard = () => {
  // ========== Context ==========
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState, statusDispatch] = useContext(StatusContext);

  // ========== 主要数据管理 ==========
  const dashboardData = useDashboardData(userState, userDispatch, statusState);

  // ========== 图表管理 ==========
  const dashboardCharts = useDashboardCharts(
    dashboardData.dataExportDefaultTime,
    dashboardData.setTrendData,
    dashboardData.setConsumeQuota,
    dashboardData.setTimes,
    dashboardData.setConsumeTokens,
    dashboardData.setPieData,
    dashboardData.setLineData,
    dashboardData.setModelColors,
    dashboardData.t,
  );

  // ========== 统计数据 ==========
  const { groupedStatsData } = useDashboardStats(
    userState,
    dashboardData.consumeQuota,
    dashboardData.consumeTokens,
    dashboardData.times,
    dashboardData.trendData,
    dashboardData.performanceMetrics,
    dashboardData.navigate,
    dashboardData.t,
  );

  // ========== 数据处理 ==========
  const loadUserData = async (overrideInputs, overrideDefaultTime) => {
    if (dashboardData.isAdminUser) {
      const userData = await dashboardData.loadUserQuotaData(overrideInputs);
      if (userData && userData.length > 0) {
        dashboardCharts.updateUserChartData(userData, overrideDefaultTime);
      }
    }
  };

  const initChart = async () => {
    await dashboardData.loadQuotaData().then((data) => {
      if (data && data.length > 0) {
        dashboardCharts.updateChartData(data);
      }
    });
    await loadUserData();
    await Promise.all([
      dashboardData.loadUptimeData(),
      dashboardData.loadSubscriptionData(),
    ]);
  };

  const handleRefresh = async (overrideInputs, overrideDefaultTime) => {
    const data = await dashboardData.refresh(
      overrideInputs,
      overrideDefaultTime,
    );
    if (data && data.length > 0) {
      dashboardCharts.updateChartData(data, overrideDefaultTime);
    }
    await loadUserData(overrideInputs, overrideDefaultTime);
  };

  const handleDateRangeChange = async (rangeValue) => {
    const { inputs: nextInputs, dataExportDefaultTime } =
      dashboardData.handleDateRangeChange(rangeValue);
    await handleRefresh(nextInputs, dataExportDefaultTime);
  };

  const handleUsernameChange = (value) => {
    dashboardData.handleInputChange(value, 'username');
  };

  const handleUsernameClear = async () => {
    const nextInputs = dashboardData.handleInputChange('', 'username');
    await handleRefresh(nextInputs);
  };

  const handleUsernameCommit = async () => {
    await handleRefresh();
  };

  // ========== 数据准备 ==========
  const apiInfoData = statusState?.status?.api_info || [];
  const announcementData = (statusState?.status?.announcements || []).map(
    (item) => {
      const pubDate = item?.publishDate ? new Date(item.publishDate) : null;
      const absoluteTime =
        pubDate && !isNaN(pubDate.getTime())
          ? `${pubDate.getFullYear()}-${String(pubDate.getMonth() + 1).padStart(2, '0')}-${String(pubDate.getDate()).padStart(2, '0')} ${String(pubDate.getHours()).padStart(2, '0')}:${String(pubDate.getMinutes()).padStart(2, '0')}`
          : item?.publishDate || '';
      const relativeTime = getRelativeTime(item.publishDate);
      return {
        ...item,
        time: absoluteTime,
        relative: relativeTime,
      };
    },
  );
  const faqData = statusState?.status?.faq || [];

  const uptimeLegendData = Object.entries(UPTIME_STATUS_MAP).map(
    ([status, info]) => ({
      status: Number(status),
      color: info.color,
      label: dashboardData.t(info.label),
    }),
  );

  // ========== Effects ==========
  useEffect(() => {
    initChart();
  }, []);

  return (
    <div className='ct-dashboard-shell ct-command-dashboard-page h-full'>
      <DashboardHeader
        getGreeting={dashboardData.getGreeting}
        greetingVisible={dashboardData.greetingVisible}
        refresh={handleRefresh}
        loading={dashboardData.loading}
        inputs={dashboardData.inputs}
        dataExportDefaultTime={dashboardData.dataExportDefaultTime}
        timeOptions={dashboardData.timeOptions}
        dateRangePresets={dashboardData.dateRangePresets}
        activeDateRange={dashboardData.activeDateRange}
        onDateRangeChange={handleDateRangeChange}
        onUsernameChange={handleUsernameChange}
        onUsernameCommit={handleUsernameCommit}
        onUsernameClear={handleUsernameClear}
        isAdminUser={dashboardData.isAdminUser}
        performanceMetrics={dashboardData.performanceMetrics}
        t={dashboardData.t}
      />

      <div className='ct-command-dashboard-body'>
        <SubscriptionOverviewPanel
          activeSubscriptions={dashboardData.activeSubscriptions}
          allSubscriptions={dashboardData.allSubscriptions}
          plans={dashboardData.subscriptionPlans}
          billingPreference={dashboardData.billingPreference}
          loading={dashboardData.subscriptionLoading}
          navigate={dashboardData.navigate}
          t={dashboardData.t}
        />

        <StatsCards
          groupedStatsData={groupedStatsData}
          loading={dashboardData.loading}
        />

        <div
          className={`ct-command-dashboard-workbench ${
            dashboardData.hasApiInfoPanel
              ? 'ct-command-dashboard-workbench-has-rail'
              : 'ct-command-dashboard-workbench-full'
          }`}
        >
          <ChartsPanel
            activeChartTab={dashboardData.activeChartTab}
            setActiveChartTab={dashboardData.setActiveChartTab}
            spec_line={dashboardCharts.spec_line}
            spec_model_line={dashboardCharts.spec_model_line}
            spec_pie={dashboardCharts.spec_pie}
            spec_rank_bar={dashboardCharts.spec_rank_bar}
            spec_user_rank={dashboardCharts.spec_user_rank}
            spec_user_trend={dashboardCharts.spec_user_trend}
            isAdminUser={dashboardData.isAdminUser}
            CHART_CONFIG={CHART_CONFIG}
            hasApiInfoPanel={dashboardData.hasApiInfoPanel}
            t={dashboardData.t}
          />

          {dashboardData.hasApiInfoPanel && (
            <ApiInfoPanel
              apiInfoData={apiInfoData}
              handleCopyUrl={(url) => handleCopyUrl(url, dashboardData.t)}
              handleSpeedTest={handleSpeedTest}
              t={dashboardData.t}
            />
          )}
        </div>

        {dashboardData.hasInfoPanels && (
          <div className='ct-command-dashboard-service-grid'>
            {dashboardData.announcementsEnabled && (
              <AnnouncementsPanel
                announcementData={announcementData}
                announcementLegendData={ANNOUNCEMENT_LEGEND_DATA.map(
                  (item) => ({
                    ...item,
                    label: dashboardData.t(item.label),
                  }),
                )}
                t={dashboardData.t}
              />
            )}

            {dashboardData.faqEnabled && (
              <FaqPanel
                faqData={faqData}
                t={dashboardData.t}
              />
            )}

            {dashboardData.uptimeEnabled && (
              <UptimePanel
                uptimeData={dashboardData.uptimeData}
                uptimeLoading={dashboardData.uptimeLoading}
                activeUptimeTab={dashboardData.activeUptimeTab}
                setActiveUptimeTab={dashboardData.setActiveUptimeTab}
                loadUptimeData={dashboardData.loadUptimeData}
                uptimeLegendData={uptimeLegendData}
                renderMonitorList={(monitors) =>
                  renderMonitorList(
                    monitors,
                    (status) =>
                      getUptimeStatusColor(status, UPTIME_STATUS_MAP),
                    (status) =>
                      getUptimeStatusText(
                        status,
                        UPTIME_STATUS_MAP,
                        dashboardData.t,
                      ),
                    dashboardData.t,
                  )
                }
                t={dashboardData.t}
              />
            )}
          </div>
        )}
      </div>
    </div>
  );
};

export default Dashboard;
