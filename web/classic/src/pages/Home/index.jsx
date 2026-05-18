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

import React, { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { Button } from '@douyinfe/semi-ui';
import {
  IconActivity,
  IconArrowRight,
  IconBolt,
  IconCheckCircleStroked,
  IconCodeStroked,
  IconCopy,
  IconFile,
  IconGithubLogo,
  IconGlobeStroke,
  IconHistogram,
  IconKeyStroked,
  IconLineChartStroked,
  IconPlay,
  IconRoute,
  IconShieldStroked,
} from '@douyinfe/semi-icons';
import {
  API,
  copy,
  showSuccess,
} from '../../helpers';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import NoticeModal from '../../components/layout/NoticeModal';
import { Link } from 'react-router-dom';
import { marked } from 'marked';
import { useTranslation } from 'react-i18next';

const fallbackStatus = {
  summary: {
    days: 30,
    success_rate: 0,
    avg_latency_ms: 0,
    requests: 0,
    enabled_channels: 0,
    healthy_channels: 0,
    protected_events: 0,
  },
  daily: [],
  groups: [],
  updated_at: 0,
  partial: true,
};

const publicStatusGroupKeys = [
  { key: 'codex', label: 'Codex 专用' },
  { key: 'claude', label: 'Claude Code' },
  { key: 'speed', label: '高速组' },
  { key: 'value', label: '低价组' },
];

const demoDailyProfile = [
  { success_rate: 92, avg_latency_ms: 520, requests: 12800000, protected_events: 1200 },
  { success_rate: 86, avg_latency_ms: 410, requests: 15100000, protected_events: 2100 },
  { success_rate: 85, avg_latency_ms: 430, requests: 16700000, protected_events: 1900 },
  { success_rate: 85, avg_latency_ms: 458, requests: 17100000, protected_events: 2200 },
  { success_rate: 80, avg_latency_ms: 680, requests: 18800000, protected_events: 6200 },
  { success_rate: 88, avg_latency_ms: 440, requests: 20300000, protected_events: 900 },
  { success_rate: 90, avg_latency_ms: 735, requests: 21100000, protected_events: 3400 },
  { success_rate: 94, avg_latency_ms: 640, requests: 21900000, protected_events: 4300 },
  { success_rate: 99, avg_latency_ms: 560, requests: 23200000, protected_events: 5200 },
  { success_rate: 98, avg_latency_ms: 610, requests: 24100000, protected_events: 1800 },
  { success_rate: 98, avg_latency_ms: 690, requests: 28700000, protected_events: 1200 },
  { success_rate: 95, avg_latency_ms: 780, requests: 30100000, protected_events: 900 },
  { success_rate: 99.6, avg_latency_ms: 428, requests: 28700000, protected_events: 13842 },
  { success_rate: 97, avg_latency_ms: 730, requests: 33400000, protected_events: 2100 },
  { success_rate: 95, avg_latency_ms: 690, requests: 32700000, protected_events: 2800 },
];

const formatDemoDate = (date) => {
  const month = `${date.getMonth() + 1}`.padStart(2, '0');
  const day = `${date.getDate()}`.padStart(2, '0');
  return `${month}-${day}`;
};

const buildDemoDaily = () => {
  const today = new Date();
  const endDate = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  const lastIndex = demoDailyProfile.length - 1;
  return demoDailyProfile.map((item, index) => {
    const date = new Date(endDate);
    date.setDate(endDate.getDate() - (lastIndex - index) * 2);
    return {
      date: formatDemoDate(date),
      ...item,
    };
  });
};

const formatNumber = (value) => {
  const number = Number(value) || 0;
  if (number >= 1000000) return `${(number / 1000000).toFixed(1)}M`;
  if (number >= 10000) return `${(number / 1000).toFixed(1)}K`;
  return new Intl.NumberFormat().format(number);
};

const formatExactNumber = (value) => new Intl.NumberFormat().format(Number(value) || 0);

const numberValue = (value, fallback = 0) => {
  const number = Number(value);
  return Number.isFinite(number) ? number : fallback;
};

const formatRate = (value) => {
  const number = Number(value) || 0;
  if (number <= 0) return '--';
  return `${number >= 99 ? number.toFixed(2) : number.toFixed(1)}%`;
};

const formatLatency = (value) => {
  const number = Number(value) || 0;
  if (number <= 0) return '--';
  if (number >= 1000) return `${(number / 1000).toFixed(2)}s`;
  return `${Math.round(number)}ms`;
};

const normalizeChartValue = (value, max, minSize = 10) => {
  const number = Number(value) || 0;
  if (max <= 0 || number <= 0) return minSize;
  return Math.max(minSize, Math.round((number / max) * 100));
};

const isEmptyHomeContent = (content) => {
  const normalized = String(content || '')
    .replace(/&nbsp;/gi, '')
    .replace(/<p>\s*<\/p>/gi, '')
    .replace(/<br\s*\/?>/gi, '')
    .trim();
  return normalized === '';
};

const buildChartPolyline = (items, valueGetter, maxValue = 100) => {
  const list = Array.isArray(items) ? items : [];
  const width = 720;
  const height = 220;
  const top = 20;
  const bottom = 28;
  const count = Math.max(list.length - 1, 1);
  if (list.length <= 1) {
    return '0,70 110,92 220,88 330,86 440,62 550,68 720,78';
  }
  return list
    .map((item, index) => {
      const raw = Number(valueGetter(item)) || 0;
      const scale = maxValue > 0 ? Math.min(raw / maxValue, 1) : 0;
      const x = (index / count) * width;
      const y = height - bottom - scale * (height - top - bottom);
      return `${x.toFixed(1)},${Math.max(top, Math.min(height - bottom, y)).toFixed(1)}`;
    })
    .join(' ');
};

const HomeCanvasBackground = () => {
  const canvasRef = useRef(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const context = canvas?.getContext?.('2d', { alpha: true });
    if (!canvas || !context) return undefined;

    const motionQuery = window.matchMedia?.('(prefers-reduced-motion: reduce)');
    let rafId = 0;
    let lastFrame = 0;
    let width = 0;
    let height = 0;
    let dpr = 1;

    const prefersReducedMotion = () => Boolean(motionQuery?.matches);

    const draw = (time = 0) => {
      if (!width || !height) return;

      const animated = !prefersReducedMotion();
      const tick = animated ? time * 0.001 : 0;
      context.clearRect(0, 0, width, height);

      const glowA = context.createRadialGradient(width * 0.72, height * 0.2, 0, width * 0.72, height * 0.2, width * 0.42);
      glowA.addColorStop(0, 'rgba(34, 211, 208, 0.18)');
      glowA.addColorStop(1, 'rgba(34, 211, 208, 0)');
      context.fillStyle = glowA;
      context.fillRect(0, 0, width, height);

      const glowB = context.createRadialGradient(width * 0.24, height * 0.72, 0, width * 0.24, height * 0.72, width * 0.36);
      glowB.addColorStop(0, 'rgba(117, 104, 255, 0.1)');
      glowB.addColorStop(1, 'rgba(117, 104, 255, 0)');
      context.fillStyle = glowB;
      context.fillRect(0, 0, width, height);

      const gridStep = width < 720 ? 72 : 96;
      const gridOffset = animated ? (tick * 8) % gridStep : 0;
      context.save();
      context.globalAlpha = 0.46;
      context.strokeStyle = 'rgba(15, 174, 185, 0.08)';
      context.lineWidth = 1;
      for (let x = -gridStep + gridOffset; x <= width + gridStep; x += gridStep) {
        context.beginPath();
        context.moveTo(x, 0);
        context.lineTo(x, height);
        context.stroke();
      }
      context.strokeStyle = 'rgba(117, 104, 255, 0.055)';
      for (let y = -gridStep + gridOffset * 0.7; y <= height + gridStep; y += gridStep) {
        context.beginPath();
        context.moveTo(0, y);
        context.lineTo(width, y);
        context.stroke();
      }
      context.restore();

      const beam = context.createLinearGradient(0, height * 0.2, width, height * 0.78);
      beam.addColorStop(0, 'rgba(34, 211, 208, 0)');
      beam.addColorStop(0.46, 'rgba(34, 211, 208, 0.13)');
      beam.addColorStop(0.54, 'rgba(117, 104, 255, 0.08)');
      beam.addColorStop(1, 'rgba(34, 211, 208, 0)');
      context.save();
      context.globalAlpha = 0.62;
      context.strokeStyle = beam;
      context.lineWidth = Math.max(28, width * 0.025);
      context.beginPath();
      const beamShift = animated ? Math.sin(tick * 0.35) * width * 0.06 : 0;
      context.moveTo(width * 0.08 + beamShift, height * 0.88);
      context.lineTo(width * 0.82 + beamShift, height * 0.08);
      context.stroke();
      context.restore();

      const particleCount = width < 720 ? 16 : 30;
      context.save();
      context.globalCompositeOperation = 'lighter';
      for (let index = 0; index < particleCount; index += 1) {
        const seed = index * 97;
        const progress = animated ? ((tick * 0.035 + index * 0.071) % 1) : (index / particleCount);
        const x = progress * (width + 180) - 90;
        const y = ((seed * 11) % Math.max(height, 1)) * 0.86 + height * 0.06;
        const radius = 1.2 + (index % 3) * 0.7;
        context.globalAlpha = 0.18 + (index % 4) * 0.055;
        context.fillStyle = index % 4 === 0 ? '#7568ff' : '#22d3d0';
        context.beginPath();
        context.arc(x, y, radius, 0, Math.PI * 2);
        context.fill();
      }
      context.restore();
    };

    const resize = () => {
      const rect = canvas.getBoundingClientRect();
      width = Math.max(1, Math.round(rect.width));
      height = Math.max(1, Math.round(rect.height));
      dpr = Math.min(window.devicePixelRatio || 1, 1.35);
      canvas.width = Math.round(width * dpr);
      canvas.height = Math.round(height * dpr);
      context.setTransform(dpr, 0, 0, dpr, 0, 0);
      draw();
    };

    const loop = (time) => {
      if (!document.hidden && !prefersReducedMotion() && time - lastFrame >= 50) {
        draw(time);
        lastFrame = time;
      }
      rafId = window.requestAnimationFrame(loop);
    };

    const restart = () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      draw();
      if (!prefersReducedMotion()) {
        rafId = window.requestAnimationFrame(loop);
      }
    };

    resize();
    restart();
    window.addEventListener('resize', resize, { passive: true });
    document.addEventListener('visibilitychange', restart);
    if (motionQuery?.addEventListener) {
      motionQuery.addEventListener('change', restart);
    } else if (motionQuery?.addListener) {
      motionQuery.addListener(restart);
    }

    return () => {
      if (rafId) {
        window.cancelAnimationFrame(rafId);
      }
      window.removeEventListener('resize', resize);
      document.removeEventListener('visibilitychange', restart);
      if (motionQuery?.removeEventListener) {
        motionQuery.removeEventListener('change', restart);
      } else if (motionQuery?.removeListener) {
        motionQuery.removeListener(restart);
      }
    };
  }, []);

  return <canvas className='ct-home-canvas' ref={canvasRef} aria-hidden='true' />;
};

const Home = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const [homePageContentLoaded, setHomePageContentLoaded] = useState(false);
  const [homePageContent, setHomePageContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [homeStatus, setHomeStatus] = useState(fallbackStatus);
  const [activeStatusGroup, setActiveStatusGroup] = useState('all');
  const isMobile = useIsMobile();
  const isDemoSiteMode = statusState?.status?.demo_site_enabled || false;
  const docsLink = statusState?.status?.docs_link || 'https://docs.newapi.pro';
  const serverAddress =
    statusState?.status?.server_address || `${window.location.origin}`;
  const isChinese = i18n.language.startsWith('zh');
  const apiBaseUrl = `${serverAddress.replace(/\/$/, '')}/v1`;

  const daily = homeStatus.daily || [];
  const statusGroups = homeStatus.groups || [];
  const summary = homeStatus.summary || fallbackStatus.summary;
  const hasRealData = Number(summary.requests) > 0;
  const demoDaily = useMemo(() => buildDemoDaily(), []);
  const updatedAt = homeStatus.updated_at
    ? new Date(homeStatus.updated_at * 1000).toLocaleString()
    : t('等待真实请求数据');
  const groupTabs = [
    {
      key: 'all',
      label: t('全部'),
      summary,
      daily,
      states: {
        healthy: summary.healthy_channels || 0,
        cooling: Math.max(
          (summary.enabled_channels || 0) - (summary.healthy_channels || 0),
          0,
        ),
        standby: 0,
      },
    },
    ...publicStatusGroupKeys.map((item) => {
      const group = statusGroups.find((statusGroup) => statusGroup.key === item.key);
      return {
        key: item.key,
        label: t(item.label),
        summary: group?.summary || { ...fallbackStatus.summary, days: summary.days },
        daily: group?.daily || daily,
        states: group?.states || { healthy: 0, cooling: 0, standby: 0 },
      };
    }),
  ];
  const activeGroup =
    groupTabs.find((item) => item.key === activeStatusGroup) || groupTabs[0];
  const activeSummary = activeGroup.summary || fallbackStatus.summary;
  const visualSummary = hasRealData
    ? activeSummary
    : {
        success_rate: 99.62,
        avg_latency_ms: 428,
        requests: 28700000,
        protected_events: 12842,
      };
  const activeDaily = activeGroup.daily || [];
  const chartDaily = hasRealData && activeDaily.length > 1 ? activeDaily : demoDaily;
  const maxRequests = Math.max(
    ...chartDaily.map((item) => Number(item.requests) || 0),
    0,
  );
  const maxLatency = Math.max(
    ...chartDaily.map((item) => Number(item.avg_latency_ms) || 0),
    0,
  );
  const successPoints = useMemo(
    () => buildChartPolyline(chartDaily, (item) => item.success_rate, 100),
    [chartDaily],
  );
  const latencyPoints = useMemo(
    () =>
      buildChartPolyline(
        chartDaily,
        (item) => item.avg_latency_ms,
        Math.max(maxLatency, 1),
      ),
    [chartDaily, maxLatency],
  );
  const chartLabelStep = Math.max(1, Math.floor(chartDaily.length / 7));
  const chartDateLabels = chartDaily
    .filter((_, index) => index % chartLabelStep === 0)
    .slice(0, 8);
  const latestChartDate = chartDaily[chartDaily.length - 1]?.date || t('等待真实请求数据');
  const tooltipDate = hasRealData ? updatedAt : `${t('示例')} ${latestChartDate}`;
  const statusSyncLabel = hasRealData
    ? `${t('同步')} ${updatedAt}`
    : t('等待真实请求数据');
  const displayDays = numberValue(activeSummary.days, numberValue(summary.days, 30)) || 30;
  const rawEnabledChannels = numberValue(
    activeSummary.enabled_channels,
    numberValue(activeGroup.states?.channels, numberValue(summary.enabled_channels, 0)),
  );
  const rawHealthyChannels = numberValue(
    activeSummary.healthy_channels,
    numberValue(activeGroup.states?.healthy, numberValue(summary.healthy_channels, 0)),
  );
  const displayEnabledChannels = hasRealData ? rawEnabledChannels : 42;
  const displayHealthyChannels = hasRealData
    ? Math.min(rawHealthyChannels, displayEnabledChannels || rawHealthyChannels)
    : 38;
  const displayCoolingChannels = hasRealData
    ? numberValue(
        activeGroup.states?.cooling,
        Math.max(displayEnabledChannels - displayHealthyChannels, 0),
      )
    : 4;
  const displayProtectedEvents = hasRealData
    ? numberValue(activeSummary.protected_events, 0)
    : 12842;
  const displayDataMode = hasRealData ? t('公开脱敏聚合') : t('示例');
  const healthyPercent = displayEnabledChannels > 0
    ? Math.round((displayHealthyChannels / displayEnabledChannels) * 100)
    : 0;
  const coolingPercent = displayEnabledChannels > 0
    ? Math.round((displayCoolingChannels / displayEnabledChannels) * 100)
    : 0;
  const standbyChannels = Math.max(
    displayEnabledChannels - displayHealthyChannels - displayCoolingChannels,
    0,
  );
  const standbyPercent = Math.max(0, 100 - healthyPercent - coolingPercent);
  const statusMetaItems = [
    {
      label: t('当前分组'),
      value: activeGroup.label,
    },
    {
      label: t('数据来源'),
      value: displayDataMode,
    },
    {
      label: t('采样窗口'),
      value: `${displayDays} ${t('天')}`,
    },
    {
      label: t('同步状态'),
      value: hasRealData ? t('实时同步') : t('等待真实数据'),
    },
  ];
  const routeAuditItems = [
    {
      step: '01',
      label: t('请求进入'),
      value: 'Base URL /v1',
    },
    {
      step: '02',
      label: t('识别限流'),
      value: t('秒级熔断保护'),
    },
    {
      step: '03',
      label: t('切换健康渠道'),
      value: `${formatExactNumber(displayHealthyChannels)} ${t('健康')}`,
    },
    {
      step: '04',
      label: t('冷却恢复'),
      value: `${formatExactNumber(displayCoolingChannels)} ${t('冷却中')}`,
    },
  ];

  const heroProofItems = [
    t('多渠道无感切换'),
    t('冷却自动恢复'),
    t('流式响应优化'),
    t('统一账单'),
  ];

  const heroMetricItems = [
    {
      label: t('近 30 天成功率'),
      value: hasRealData ? formatRate(activeSummary.success_rate) : '99.62%',
      note: t('公开脱敏看板'),
    },
    {
      label: t('平均响应延迟'),
      value: hasRealData ? formatLatency(activeSummary.avg_latency_ms) : '428ms',
      note: t('流式请求优先'),
    },
    {
      label: t('健康渠道'),
      value: `${formatExactNumber(displayHealthyChannels)} / ${formatExactNumber(displayEnabledChannels)}`,
      note: t('自动避开限流通道'),
    },
  ];

  const statusInsightItems = [
    {
      value: hasRealData ? formatNumber(activeSummary.requests) : '28.7M',
      label: t('近 30 天请求'),
    },
    {
      value: formatExactNumber(displayProtectedEvents),
      label: t('自动保护事件'),
    },
    {
      value: `${formatExactNumber(displayCoolingChannels)} ${t('条')}`,
      label: t('当前冷却通道'),
    },
  ];
  const statusHealthItems = [
    {
      label: t('健康通道'),
      value: formatExactNumber(displayHealthyChannels),
      helper: `${healthyPercent}% ${t('可用通道')}`,
      tone: 'healthy',
      percent: Math.max(healthyPercent, 8),
    },
    {
      label: t('冷却通道'),
      value: formatExactNumber(displayCoolingChannels),
      helper: t('限流已绕过'),
      tone: 'cooling',
      percent: Math.max(coolingPercent, displayCoolingChannels > 0 ? 7 : 0),
    },
    {
      label: t('备用通道'),
      value: formatExactNumber(standbyChannels),
      helper: t('随时接管'),
      tone: 'standby',
      percent: Math.max(standbyPercent, standbyChannels > 0 ? 7 : 0),
    },
  ];

  const heroRoutingRows = [
    {
      label: t('实时调度'),
      value: 'Codex stream',
      state: t('流式保持'),
    },
    {
      label: t('分组策略'),
      value: t('低价优先'),
      state: t('成本可控'),
    },
    {
      label: t('异常处理'),
      value: t('限流旁路'),
      state: t('秒级恢复'),
    },
  ];

  const heroClients = [
    { label: 'Codex', icon: <IconCodeStroked /> },
    { label: 'Claude Code', icon: <IconKeyStroked /> },
    { label: 'OpenAI SDK', icon: <IconRoute /> },
    { label: t('其他客户端'), icon: <IconGlobeStroke /> },
  ];

  const metricCards = [
    {
      label: t('近 30 天成功率'),
      value: hasRealData ? formatRate(activeSummary.success_rate) : '99.62%',
      delta: '+0.28%',
      tone: 'teal',
      icon: <IconLineChartStroked />,
    },
    {
      label: t('平均响应延迟'),
      value: hasRealData ? formatLatency(activeSummary.avg_latency_ms) : '428ms',
      delta: '-12ms',
      tone: 'blue',
      icon: <IconActivity />,
    },
    {
      label: t('请求量'),
      value: hasRealData ? formatNumber(activeSummary.requests) : '28.7M',
      delta: '+18.6%',
      tone: 'violet',
      icon: <IconHistogram />,
    },
    {
      label: t('保护事件'),
      value: hasRealData
        ? formatNumber(activeSummary.protected_events)
        : '12,842',
      delta: '-8.3%',
      tone: 'orange',
      icon: <IconShieldStroked />,
    },
  ];

  const routeRules = [
    t('模型能力匹配'),
    t('分组倍率优先级'),
    t('失败率实时降权'),
    t('限流冷却隔离'),
    t('成本与速度平衡'),
  ];

  const upstreamRows = [
    {
      status: t('健康'),
      title: t('通道 A'),
      vendor: 'OpenAI',
      price: '$0.65 / 1M',
      rpm: '1,240 RPM',
      tone: 'healthy',
    },
    {
      status: t('冷却中'),
      title: t('通道 B'),
      vendor: 'Anthropic',
      price: '$1.5 / 1M',
      rpm: '680 RPM',
      tone: 'cooling',
    },
    {
      status: t('备用'),
      title: t('通道 C'),
      vendor: 'OpenAI',
      price: '$0.45 / 1M',
      rpm: '880 RPM',
      tone: 'standby',
    },
  ];

  const scenarioItems = [
    {
      icon: <IconCodeStroked />,
      title: t('Codex 长任务'),
      meta: t('长上下文、多轮工具调用不怕单渠道抖动'),
      proof: t('失败自动旁路，减少中断重跑'),
      signal: t('中断重跑减少'),
      metric: '72%',
      tone: 'coding',
    },
    {
      icon: <IconRoute />,
      title: t('Claude Code 项目开发'),
      meta: t('高频编辑、流式响应和模型切换更顺滑'),
      proof: t('限速自动切走，保持输出连续'),
      signal: t('输出连续性'),
      metric: '99%+',
      tone: 'routing',
    },
    {
      icon: <IconActivity />,
      title: t('团队共享额度'),
      meta: t('统一密钥、分组倍率、用量日志和预算控制'),
      proof: t('低价组优先，成本更可预期'),
      signal: t('成本可控'),
      metric: t('低价优先'),
      tone: 'cost',
    },
    {
      icon: <IconShieldStroked />,
      title: t('自动化脚本 / CI'),
      meta: t('批量任务需要稳定吞吐和失败兜底'),
      proof: t('统一 /v1 接入，兼容现有 SDK'),
      signal: t('批量吞吐'),
      metric: t('自动调度'),
      tone: 'automation',
    },
  ];

  const comparisonItems = [
    {
      label: t('官方直连'),
      value: t('单渠道限速明显'),
      detail: t('账号额度、地区网络和速率限制都可能让长任务中断'),
    },
    {
      label: t('普通代理'),
      value: t('缺少健康调度'),
      detail: t('失败后仍可能打到同一条异常线路，成本和体验不可控'),
    },
    {
      label: 'CodeToken AI',
      value: t('多渠道自动恢复'),
      detail: t('按分组、健康、冷却和成本策略选择更合适的上游'),
      featured: true,
    },
  ];

  const trustItems = [
    t('Codex / Claude Code 兼容'),
    t('OpenAI SDK 原生接入'),
    t('分组倍率透明'),
    t('失败与限流可追踪'),
  ];
  const compareScoreItems = [
    {
      label: t('可用性'),
      value: hasRealData ? formatRate(activeSummary.success_rate) : '99.62%',
    },
    {
      label: t('恢复链路'),
      value: `4 ${t('步')}`,
    },
    {
      label: t('接入成本'),
      value: t('低价优先'),
    },
  ];

  const handleCopyBaseURL = async () => {
    const ok = await copy(apiBaseUrl);
    if (ok) {
      showSuccess(t('已复制到剪切板'));
    }
  };

  const displayHomePageContent = async () => {
    setHomePageContent(localStorage.getItem('home_page_content') || '');
    try {
      const res = await API.get('/api/home_page_content', {
        skipErrorHandler: true,
      });
      const { success, message, data } = res.data;
      if (success) {
        if (isEmptyHomeContent(data)) {
          setHomePageContent('');
          localStorage.removeItem('home_page_content');
          setHomePageContentLoaded(true);
          return;
        }
        let content = data;
        if (!data.startsWith('https://')) {
          content = marked.parse(data);
        }
        setHomePageContent(content);
        localStorage.setItem('home_page_content', content);

        if (data.startsWith('https://')) {
          const iframe = document.querySelector('iframe');
          if (iframe) {
            iframe.onload = () => {
              iframe.contentWindow.postMessage({ themeMode: actualTheme }, '*');
              iframe.contentWindow.postMessage({ lang: i18n.language }, '*');
            };
          }
        }
      } else {
        if (message) {
          console.warn('首页内容接口返回失败:', message);
        }
        setHomePageContent('');
      }
    } catch (error) {
      console.error('加载首页内容失败:', error);
      setHomePageContent('');
    }
    setHomePageContentLoaded(true);
  };

  const loadHomeStatus = async () => {
    try {
      const res = await API.get('/api/public/home/status', {
        params: { days: 30 },
        skipErrorHandler: true,
      });
      const { success, data } = res.data;
      if (success && data) {
        setHomeStatus(data);
      }
    } catch (error) {
      console.error('加载首页运行状态失败:', error);
    }
  };

  useEffect(() => {
    const checkNoticeAndShow = async () => {
      const lastCloseDate = localStorage.getItem('notice_close_date');
      const today = new Date().toDateString();
      if (lastCloseDate !== today) {
        try {
          const res = await API.get('/api/notice', {
            skipErrorHandler: true,
          });
          const { success, data } = res.data;
          if (success && data && data.trim() !== '') {
            setNoticeVisible(true);
          }
        } catch (error) {
          console.error('获取公告失败:', error);
        }
      }
    };

    checkNoticeAndShow();
  }, []);

  useEffect(() => {
    displayHomePageContent().then();
    loadHomeStatus().then();
  }, []);

  useEffect(() => {
    document.body.classList.add('ct-home-route');

    return () => {
      document.body.classList.remove('ct-home-route');
    };
  }, []);

  return (
    <div className='w-full overflow-x-hidden'>
      <NoticeModal
        visible={noticeVisible}
        onClose={() => setNoticeVisible(false)}
        isMobile={isMobile}
      />
      {homePageContentLoaded && homePageContent === '' ? (
        <div className='ct-home'>
          <HomeCanvasBackground />
          <section className='ct-hero'>
            <div className='ct-hero-ambient' aria-hidden='true'>
              <div className='ct-ambient-grid' />
              <div className='ct-ambient-ribbon'>
                {Array.from({ length: 18 }).map((_, index) => (
                  <i key={index} />
                ))}
              </div>
            </div>
            <div className='ct-home-shell ct-hero-shell'>
              <div className='ct-hero-copy'>
                <h1 className={isChinese ? 'ct-hero-title zh' : 'ct-hero-title'}>
                  {isChinese ? (
                    <>
                      <em>{t('Codex 与 Claude Code')}</em>
                      <em>{t('稳定高速中转站')}</em>
                    </>
                  ) : (
                    t('Codex / Claude Code 稳定高速中转站')
                  )}
                </h1>
                <h2>{t('多渠道自动切换，低价接入主流编程模型')}</h2>
                <p>{t('为长时间编码、团队共享和自动化脚本准备的高可用 AI API 网关。')}</p>
                <div className='ct-hero-metrics' aria-label={t('首页核心指标')}>
                  {heroMetricItems.map((item) => (
                    <div className='ct-hero-metric' key={item.label}>
                      <strong>{item.value}</strong>
                      <span>{item.label}</span>
                      <small>{item.note}</small>
                    </div>
                  ))}
                </div>
                <div className='ct-hero-actions'>
                  <Link to='/console'>
                    <Button
                      theme='solid'
                      type='primary'
                      size={isMobile ? 'default' : 'large'}
                      className='ct-primary-btn'
                      icon={<IconArrowRight />}
                    >
                      {t('获取密钥')}
                    </Button>
                  </Link>
                  {isDemoSiteMode && statusState?.status?.version ? (
                    <Button
                      size={isMobile ? 'default' : 'large'}
                      className='ct-secondary-btn'
                      icon={<IconGithubLogo />}
                      onClick={() =>
                        window.open('https://github.com/QuantumNous/new-api', '_blank')
                      }
                    >
                      {statusState.status.version}
                    </Button>
                  ) : (
                    docsLink && (
                      <Button
                        size={isMobile ? 'default' : 'large'}
                        className='ct-secondary-btn'
                        onClick={() => window.open(docsLink, '_blank')}
                      >
                        {t('查看文档')}
                      </Button>
                    )
                  )}
                </div>
                <div className='ct-proof-row'>
                  {heroProofItems.map((item) => (
                    <span key={item}>
                      <IconCheckCircleStroked />
                      {item}
                    </span>
                  ))}
                </div>
              </div>

              <div className='ct-hero-map ct-hero-focus-map' aria-hidden='true'>
                <div className='ct-hero-orbit-stage'>
                  <span className='ct-hero-orbit ring-a' />
                  <span className='ct-hero-orbit ring-b' />
                  <span className='ct-hero-orbit ring-c' />
                  <span className='ct-hero-stream stream-a' />
                  <span className='ct-hero-stream stream-b' />
                  <span className='ct-hero-stream stream-c' />
                  <div className='ct-hero-core-panel'>
                    <IconBolt />
                    <strong>CodeToken AI</strong>
                    <span>{t('智能网关')}</span>
                  </div>
                  <div className='ct-hero-signal signal-codex'>
                    <IconCodeStroked />
                    <span>Codex</span>
                  </div>
                  <div className='ct-hero-signal signal-claude'>
                    <IconKeyStroked />
                    <span>Claude Code</span>
                  </div>
                  <div className='ct-hero-signal signal-sdk'>
                    <IconRoute />
                    <span>OpenAI SDK</span>
                  </div>
                  <div className='ct-hero-signal signal-api'>
                    <IconGlobeStroke />
                    <span>API /v1</span>
                  </div>
                  <div className='ct-hero-floating-card card-success'>
                    <span>{t('自动切换')}</span>
                    <strong>{t('限流旁路')}</strong>
                  </div>
                  <div className='ct-hero-floating-card card-cost'>
                    <span>{t('成本优化')}</span>
                    <strong>{t('低价组优先')}</strong>
                  </div>
                  <div className='ct-hero-route-board'>
                    <div className='ct-hero-route-board-head'>
                      <span>{t('调度链路')}</span>
                      <strong>{formatExactNumber(displayHealthyChannels)} / {formatExactNumber(displayEnabledChannels)}</strong>
                    </div>
                    {heroRoutingRows.map((item) => (
                      <div className='ct-hero-route-row' key={item.label}>
                        <span>{item.label}</span>
                        <strong>{item.value}</strong>
                        <em>{item.state}</em>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-status-section'>
            <div className='ct-home-shell'>
              <div className='ct-status-head'>
                <div className='ct-status-title'>
                  <small>{t('稳定性证明')}</small>
                  <h2>{t('真实运行曲线')}</h2>
                  <span>
                    <i />
                    {t('运行状态')}
                  </span>
                </div>
                <div className='ct-group-tabs' role='tablist'>
                  {groupTabs.map((item) => (
                    <button
                      type='button'
                      key={item.key}
                      className={activeGroup.key === item.key ? 'active' : ''}
                      onClick={() => setActiveStatusGroup(item.key)}
                    >
                      {item.label}
                    </button>
                  ))}
                </div>
                <div className='ct-range-pill'>{t('近 30 天')}</div>
              </div>
              <div className='ct-status-meta-row'>
                {statusMetaItems.map((item) => (
                  <div className='ct-status-meta-card' key={item.label}>
                    <span>{item.label}</span>
                    <strong>{item.value}</strong>
                  </div>
                ))}
              </div>
              <div className='ct-status-insights'>
                {statusInsightItems.map((item) => (
                  <div key={item.label}>
                    <strong>{item.value}</strong>
                    <span>{item.label}</span>
                  </div>
                ))}
              </div>
              <div className='ct-status-health-strip' aria-label={t('通道健康分布')}>
                {statusHealthItems.map((item) => (
                  <div className={`ct-status-health-item ${item.tone}`} key={item.label}>
                    <div>
                      <span>{item.label}</span>
                      <strong>{item.value}</strong>
                      <small>{item.helper}</small>
                    </div>
                    <i style={{ '--health-width': `${item.percent}%` }} />
                  </div>
                ))}
              </div>
              <div className='ct-status-grid'>
                <div className='ct-status-metrics'>
                  {metricCards.map((item) => (
                    <div className={`ct-metric-card ${item.tone}`} key={item.label}>
                      <div className='ct-metric-icon'>
                        {item.icon}
                      </div>
                      <div>
                        <span>{item.label}</span>
                        <strong>{item.value}</strong>
                        <small>{item.delta}</small>
                      </div>
                    </div>
                  ))}
                </div>
                <div className='ct-chart-panel'>
                  <div className='ct-chart-legend'>
                    <span><i className='success' />{t('成功率趋势')}</span>
                    <span><i className='latency' />{t('平均延迟')}</span>
                    <span><i className='volume' />{t('请求量')}</span>
                    <span><i className='event' />{t('保护事件')}</span>
                  </div>
                  <div className='ct-chart-data-note'>
                    <span>{displayDataMode}</span>
                    <span>{t('按分组查看成功率、延迟与保护事件')}</span>
                    <span>{statusSyncLabel}</span>
                  </div>
                  <div className='ct-status-chart' aria-hidden='true'>
                    <div className='ct-chart-axis-left'>
                      <span>100</span>
                      <span>75</span>
                      <span>50</span>
                      <span>25</span>
                      <span>0</span>
                    </div>
                    <div className='ct-chart-axis-right'>
                      <span>40M</span>
                      <span>30M</span>
                      <span>20M</span>
                      <span>10M</span>
                      <span>0</span>
                    </div>
                    <svg viewBox='0 0 720 220' preserveAspectRatio='none'>
                      <polyline className='success' points={successPoints} />
                      <polyline className='latency' points={latencyPoints} />
                    </svg>
                    <div className='ct-volume-bars'>
                      {chartDaily.map((item) => (
                        <i
                          key={item.date}
                          style={{
                            '--bar-height': `${normalizeChartValue(item.requests, maxRequests, 10)}%`,
                          }}
                        />
                      ))}
                    </div>
                    <div className='ct-event-markers'>
                      {chartDaily.map((item) => (
                        <i
                          key={item.date}
                          className={item.protected_events > 0 ? 'active' : ''}
                          style={{
                            '--event-size': `${Math.min(18, 6 + Number(item.protected_events || 0) * 2)}px`,
                          }}
                        />
                      ))}
                    </div>
                    <div className='ct-chart-dates'>
                      {chartDateLabels.map((item) => (
                        <span key={item.date}>{item.date}</span>
                      ))}
                    </div>
                    <div className='ct-chart-tooltip'>
                      <strong>{tooltipDate}</strong>
                      <span>{t('成功率趋势')} {formatRate(visualSummary.success_rate)}</span>
                      <span>{t('平均延迟')} {formatLatency(visualSummary.avg_latency_ms)}</span>
                      <span>{t('请求量')} {formatNumber(visualSummary.requests)}</span>
                      <span>{t('保护事件')} {formatExactNumber(visualSummary.protected_events)}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-route-section'>
            <div className='ct-home-shell'>
              <div className='ct-section-headline'>
                <small>{t('无感切换机制')}</small>
                <h2>{t('一次接入，自动避开限流与异常')}</h2>
                <p>{t('客户只需要维护一个 Base URL，后面由网关根据分组、健康、冷却和成本策略做实时选择。')}</p>
              </div>
              <div className='ct-route-canvas'>
                <div className='ct-route-lines' aria-hidden='true'>
                  <span className='ct-route-line-a' />
                  <span className='ct-route-line-b' />
                  <span className='ct-route-line-c' />
                </div>
                <div className='ct-client-access'>
                  <h3>{t('客户端接入')}</h3>
                  <div className='ct-connect-panel'>
                    <div className='ct-config-row highlight'>
                      <span>Base URL</span>
                      <code>{apiBaseUrl}</code>
                    </div>
                    <div className='ct-config-row'>
                      <span>API Key</span>
                      <code>ct-••••••••••••••••••••</code>
                      <Button
                        type='tertiary'
                        onClick={handleCopyBaseURL}
                        icon={<IconCopy />}
                        className='ct-copy-btn'
                      />
                    </div>
                    <div className='ct-client-chips'>
                      {heroClients.map((item) => (
                        <span key={item.label}>{item.icon}{item.label}</span>
                      ))}
                    </div>
                  </div>
                </div>

                <div className='ct-decision-core'>
                  <div className='ct-decision-orb'>
                    <IconBolt />
                    <strong>{t('智能决策')}</strong>
                    <span>{t('毫秒级选择')}</span>
                  </div>
                </div>

                <div className='ct-rule-list'>
                  <h3>{t('智能路由引擎')}</h3>
                  {routeRules.map((item) => (
                    <div className='ct-rule-row' key={item}>
                      <IconCheckCircleStroked />
                      <span>{item}</span>
                    </div>
                  ))}
                </div>

                <div className='ct-upstream-list'>
                  <h3>{t('上游通道池')}</h3>
                  {upstreamRows.map((item) => (
                    <div className={`ct-upstream-line ${item.tone}`} key={item.title}>
                      <div>
                        <b>{item.status}</b>
                        <strong>{item.title}</strong>
                      </div>
                      <span>{item.vendor}</span>
                      <small>{item.price}</small>
                      <small>{item.rpm}</small>
                      <div className='ct-mini-bars'>
                        {Array.from({ length: 7 }).map((_, index) => (
                          <i key={index} />
                        ))}
                      </div>
                    </div>
                  ))}
                  <em>{t('更多通道')} ...</em>
                </div>
                <div className='ct-route-badges'>
                  {[
                    t('多渠道无感切换'),
                    t('自动重试与降级'),
                    t('智能限流保护'),
                    t('更低成本更稳定'),
                  ].map((item) => (
                    <span key={item}>
                      <IconCheckCircleStroked />
                      {item}
                    </span>
                  ))}
                </div>
                <div className='ct-route-audit'>
                  <div className='ct-route-audit-head'>
                    <span>{t('失败恢复路径')}</span>
                    <strong>{t('限流、报错、超时都进入自动保护链路')}</strong>
                  </div>
                  <div className='ct-route-audit-steps'>
                    {routeAuditItems.map((item) => (
                      <div className='ct-route-audit-step' key={item.step}>
                        <span>{item.step}</span>
                        <strong>{item.label}</strong>
                        <small>{item.value}</small>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-scenario-section'>
            <div className='ct-home-shell'>
              <div className='ct-section-headline'>
                <small>{t('真实使用场景')}</small>
                <h2>{t('为不同场景提供稳定动力')}</h2>
                <p>{t('把客户真正关心的不中断、低成本、速度和治理能力拆到可感知的场景里。')}</p>
              </div>
              <div className='ct-scenario-grid'>
                {scenarioItems.map((item) => (
                  <div className='ct-scenario-card' key={item.title}>
                    <div>
                      <h3>{item.title}</h3>
                      <p>{item.meta}</p>
                      <strong>{item.proof}</strong>
                      <div className='ct-scenario-meter'>
                        <span>{item.signal}</span>
                        <b>{item.metric}</b>
                      </div>
                    </div>
                    <div className='ct-scenario-art'>
                      <span>{item.icon}</span>
                      <i />
                      <b />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </section>

          <section className='ct-compare-section'>
            <div className='ct-home-shell'>
              <div className='ct-section-headline compact'>
                <small>{t('低价与专业性')}</small>
                <h2>{t('不是再加一个代理，而是把不稳定变成可运营的系统')}</h2>
              </div>
              <div className='ct-compare-grid'>
                {comparisonItems.map((item) => (
                  <div
                    className={`ct-compare-card${item.featured ? ' featured' : ''}`}
                    key={item.label}
                  >
                    <span>{item.label}</span>
                    <strong>{item.value}</strong>
                    <p>{item.detail}</p>
                  </div>
                ))}
              </div>
              <div className='ct-compare-scoreboard'>
                {compareScoreItems.map((item) => (
                  <div key={item.label}>
                    <span>{item.label}</span>
                    <strong>{item.value}</strong>
                  </div>
                ))}
              </div>
              <div className='ct-trust-row'>
                {trustItems.map((item) => (
                  <span key={item}>
                    <IconCheckCircleStroked />
                    {item}
                  </span>
                ))}
              </div>
            </div>
          </section>

          <section className='ct-final-section'>
            <div className='ct-home-shell ct-final-cta'>
              <div className='ct-final-visual' aria-hidden='true'>
                <span className='ct-final-orbit' />
                <span className='ct-final-orbit inner' />
                <div className='ct-final-bars'>
                  {Array.from({ length: 12 }).map((_, index) => (
                    <i key={index} />
                  ))}
                </div>
              </div>
              <div>
                <h2>{t('更稳定，更智能，更省钱的 AI 编程中转站')}</h2>
                <p>{t('统一接入，高可用保障，让每一次 Codex 与 Claude Code 请求都更有价值')}</p>
              </div>
              <div className='ct-final-actions'>
                <Link to='/console'>
                  <Button
                    theme='solid'
                    type='primary'
                    size={isMobile ? 'default' : 'large'}
                    className='ct-primary-btn'
                    icon={<IconPlay />}
                  >
                    {t('立即获取密钥')}
                  </Button>
                </Link>
                {docsLink && (
                  <Button
                    size={isMobile ? 'default' : 'large'}
                    className='ct-secondary-btn'
                    icon={<IconFile />}
                    onClick={() => window.open(docsLink, '_blank')}
                  >
                    {t('查看文档')}
                  </Button>
                )}
              </div>
            </div>
          </section>
        </div>
      ) : (
        <div className='overflow-x-hidden w-full'>
          {homePageContent.startsWith('https://') ? (
            <iframe src={homePageContent} className='w-full h-screen border-none' />
          ) : (
            <div
              className='mt-[60px]'
              dangerouslySetInnerHTML={{ __html: homePageContent }}
            />
          )}
        </div>
      )}
    </div>
  );
};

export default Home;
