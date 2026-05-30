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

import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { API, isAdmin, showError } from '../../helpers';
import { getDashboardDateRangeInputs } from '../../helpers/dashboard';
import {
  DASHBOARD_DATE_RANGE_PRESETS,
  TIME_OPTIONS,
} from '../../constants/dashboard.constants';
import { useIsMobile } from '../common/useIsMobile';
import { useMinimumLoadingTime } from '../common/useMinimumLoadingTime';

const getDefaultTimeForDateRange = (rangeValue) =>
  rangeValue === '7d' || rangeValue === '30d' ? 'day' : 'hour';

export const useDashboardData = (userState, userDispatch, statusState) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const isMobile = useIsMobile();
  const initialized = useRef(false);

  // ========== 基础状态 ==========
  const [loading, setLoading] = useState(false);
  const [greetingVisible, setGreetingVisible] = useState(false);
  const showLoading = useMinimumLoadingTime(loading);

  // ========== 输入状态 ==========
  const defaultDateRange = 'today';
  const [activeDateRange, setActiveDateRange] = useState(defaultDateRange);
  const [inputs, setInputs] = useState({
    username: '',
    token_name: '',
    model_name: '',
    ...getDashboardDateRangeInputs(defaultDateRange),
    channel: '',
    data_export_default_time: '',
  });

  const [dataExportDefaultTime, setDataExportDefaultTime] = useState(
    getDefaultTimeForDateRange(defaultDateRange),
  );

  // ========== 数据状态 ==========
  const [quotaData, setQuotaData] = useState([]);
  const [consumeQuota, setConsumeQuota] = useState(0);
  const [consumeTokens, setConsumeTokens] = useState(0);
  const [times, setTimes] = useState(0);
  const [pieData, setPieData] = useState([{ type: 'null', value: '0' }]);
  const [lineData, setLineData] = useState([]);
  const [modelColors, setModelColors] = useState({});
  const [subscriptionLoading, setSubscriptionLoading] = useState(false);
  const [activeSubscriptions, setActiveSubscriptions] = useState([]);
  const [allSubscriptions, setAllSubscriptions] = useState([]);
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);
  const [billingPreference, setBillingPreference] = useState(
    'subscription_first',
  );

  // ========== 图表状态 ==========
  const [activeChartTab, setActiveChartTab] = useState('1');

  // ========== 趋势数据 ==========
  const [trendData, setTrendData] = useState({
    balance: [],
    usedQuota: [],
    requestCount: [],
    times: [],
    consumeQuota: [],
    tokens: [],
    rpm: [],
    tpm: [],
  });

  // ========== Uptime 数据 ==========
  const [uptimeData, setUptimeData] = useState([]);
  const [uptimeLoading, setUptimeLoading] = useState(false);
  const [activeUptimeTab, setActiveUptimeTab] = useState('');

  // ========== 常量 ==========
  const isAdminUser = isAdmin();

  // ========== Panel enable flags ==========
  const apiInfoEnabled = statusState?.status?.api_info_enabled ?? true;
  const announcementsEnabled =
    statusState?.status?.announcements_enabled ?? true;
  const faqEnabled = statusState?.status?.faq_enabled ?? true;
  const uptimeEnabled = statusState?.status?.uptime_kuma_enabled ?? true;

  const hasApiInfoPanel = apiInfoEnabled;
  const hasInfoPanels = announcementsEnabled || faqEnabled || uptimeEnabled;

  // ========== Memoized Values ==========
  const timeOptions = useMemo(
    () =>
      TIME_OPTIONS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t],
  );

  const dateRangePresets = useMemo(
    () =>
      DASHBOARD_DATE_RANGE_PRESETS.map((option) => ({
        ...option,
        label: t(option.label),
      })),
    [t],
  );

  const performanceMetrics = useMemo(() => {
    const { start_timestamp, end_timestamp } = inputs;
    const timeDiff =
      (Date.parse(end_timestamp) - Date.parse(start_timestamp)) / 60000;
    const avgRPM = isNaN(times / timeDiff)
      ? '0'
      : (times / timeDiff).toFixed(3);
    const avgTPM = isNaN(consumeTokens / timeDiff)
      ? '0'
      : (consumeTokens / timeDiff).toFixed(3);

    return { avgRPM, avgTPM, timeDiff };
  }, [times, consumeTokens, inputs.start_timestamp, inputs.end_timestamp]);

  const getGreeting = useMemo(() => {
    const hours = new Date().getHours();
    let greeting = '';

    if (hours >= 5 && hours < 12) {
      greeting = t('早上好');
    } else if (hours >= 12 && hours < 14) {
      greeting = t('中午好');
    } else if (hours >= 14 && hours < 18) {
      greeting = t('下午好');
    } else {
      greeting = t('晚上好');
    }

    const username = userState?.user?.username || '';
    return `${greeting}，${username} 👋`;
  }, [t, userState?.user?.username]);

  // ========== 回调函数 ==========
  const handleInputChange = useCallback(
    (value, name) => {
      if (name === 'data_export_default_time') {
        setDataExportDefaultTime(value);
        localStorage.setItem('data_export_default_time', value);
        return inputs;
      }

      const nextInputs = { ...inputs, [name]: value ?? '' };
      setInputs(nextInputs);
      if (name === 'start_timestamp' || name === 'end_timestamp') {
        setActiveDateRange('custom');
      }
      return nextInputs;
    },
    [inputs],
  );

  const handleDateRangeChange = useCallback(
    (rangeValue) => {
      const nextDefaultTime = getDefaultTimeForDateRange(rangeValue);
      const nextInputs = {
        ...inputs,
        ...getDashboardDateRangeInputs(rangeValue),
      };
      setInputs(nextInputs);
      setActiveDateRange(rangeValue);
      setDataExportDefaultTime(nextDefaultTime);
      return {
        dataExportDefaultTime: nextDefaultTime,
        inputs: nextInputs,
      };
    },
    [inputs],
  );

  // ========== API 调用函数 ==========
  const loadQuotaData = useCallback(
    async (overrideInputs, overrideDefaultTime) => {
      setLoading(true);
      try {
        let url = '';
        const currentInputs = overrideInputs || inputs;
        const { start_timestamp, end_timestamp, username } = currentInputs;
        let localStartTimestamp = Date.parse(start_timestamp) / 1000;
        let localEndTimestamp = Date.parse(end_timestamp) / 1000;
        const encodedUsername = encodeURIComponent(username || '');
        const currentDefaultTime = overrideDefaultTime || dataExportDefaultTime;

        if (isAdminUser) {
          url = `/api/data/?username=${encodedUsername}&start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&default_time=${currentDefaultTime}`;
        } else {
          url = `/api/data/self/?start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}&default_time=${currentDefaultTime}`;
        }

        const res = await API.get(url);
        const { success, message, data } = res.data;
        if (success) {
          setQuotaData(data);
          if (data.length === 0) {
            data.push({
              count: 0,
              model_name: '无数据',
              quota: 0,
              created_at: Date.now() / 1000,
            });
          }
          data.sort((a, b) => a.created_at - b.created_at);
          return data;
        } else {
          showError(message);
          return [];
        }
      } finally {
        setLoading(false);
      }
    },
    [inputs, dataExportDefaultTime, isAdminUser],
  );

  const loadUptimeData = useCallback(async () => {
    setUptimeLoading(true);
    try {
      const res = await API.get('/api/uptime/status');
      const { success, message, data } = res.data;
      if (success) {
        setUptimeData(data || []);
        if (data && data.length > 0 && !activeUptimeTab) {
          setActiveUptimeTab(data[0].categoryName);
        }
      } else {
        showError(message);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setUptimeLoading(false);
    }
  }, [activeUptimeTab]);

  const loadSubscriptionData = useCallback(async () => {
    setSubscriptionLoading(true);
    try {
      const [plansResult, selfResult] = await Promise.allSettled([
        API.get('/api/subscription/plans', { skipErrorHandler: true }),
        API.get('/api/subscription/self', { skipErrorHandler: true }),
      ]);

      if (
        plansResult.status === 'fulfilled' &&
        plansResult.value?.data?.success
      ) {
        setSubscriptionPlans(plansResult.value.data.data || []);
      } else {
        setSubscriptionPlans([]);
      }

      if (
        selfResult.status === 'fulfilled' &&
        selfResult.value?.data?.success
      ) {
        const payload = selfResult.value.data.data || {};
        setBillingPreference(payload.billing_preference || 'subscription_first');
        setActiveSubscriptions(payload.subscriptions || []);
        setAllSubscriptions(payload.all_subscriptions || []);
      } else {
        setBillingPreference('subscription_first');
        setActiveSubscriptions([]);
        setAllSubscriptions([]);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setSubscriptionLoading(false);
    }
  }, []);

  const loadUserQuotaData = useCallback(
    async (overrideInputs) => {
      if (!isAdminUser) return [];
      try {
        const currentInputs = overrideInputs || inputs;
        const { start_timestamp, end_timestamp } = currentInputs;
        const localStartTimestamp = Date.parse(start_timestamp) / 1000;
        const localEndTimestamp = Date.parse(end_timestamp) / 1000;
        const url = `/api/data/users?start_timestamp=${localStartTimestamp}&end_timestamp=${localEndTimestamp}`;
        const res = await API.get(url);
        const { success, message, data } = res.data;
        if (success) {
          return data || [];
        } else {
          showError(message);
          return [];
        }
      } catch (err) {
        console.error(err);
        return [];
      }
    },
    [inputs, isAdminUser],
  );

  const getUserData = useCallback(async () => {
    let res = await API.get(`/api/user/self`);
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
    } else {
      showError(message);
    }
  }, [userDispatch]);

  const refresh = useCallback(
    async (overrideInputs, overrideDefaultTime) => {
      const data = await loadQuotaData(overrideInputs, overrideDefaultTime);
      await Promise.all([loadUptimeData(), loadSubscriptionData()]);
      return data;
    },
    [loadQuotaData, loadUptimeData, loadSubscriptionData],
  );

  // ========== Effects ==========
  useEffect(() => {
    const timer = setTimeout(() => {
      setGreetingVisible(true);
    }, 100);
    return () => clearTimeout(timer);
  }, []);

  useEffect(() => {
    if (!initialized.current) {
      getUserData();
      initialized.current = true;
    }
  }, [getUserData]);

  return {
    // 基础状态
    loading: showLoading,
    greetingVisible,

    // 输入状态
    inputs,
    dataExportDefaultTime,
    activeDateRange,

    // 数据状态
    quotaData,
    consumeQuota,
    setConsumeQuota,
    consumeTokens,
    setConsumeTokens,
    times,
    setTimes,
    pieData,
    setPieData,
    lineData,
    setLineData,
    modelColors,
    setModelColors,
    subscriptionLoading,
    activeSubscriptions,
    allSubscriptions,
    subscriptionPlans,
    billingPreference,

    // 图表状态
    activeChartTab,
    setActiveChartTab,

    // 趋势数据
    trendData,
    setTrendData,

    // Uptime 数据
    uptimeData,
    uptimeLoading,
    activeUptimeTab,
    setActiveUptimeTab,

    // 计算值
    timeOptions,
    dateRangePresets,
    performanceMetrics,
    getGreeting,
    isAdminUser,
    hasApiInfoPanel,
    hasInfoPanels,
    apiInfoEnabled,
    announcementsEnabled,
    faqEnabled,
    uptimeEnabled,

    // 函数
    handleInputChange,
    handleDateRangeChange,
    loadQuotaData,
    loadUserQuotaData,
    loadUptimeData,
    loadSubscriptionData,
    getUserData,
    refresh,

    // 导航和翻译
    navigate,
    t,
    isMobile,
  };
};
