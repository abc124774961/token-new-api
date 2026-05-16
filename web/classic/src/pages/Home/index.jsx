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

import React, { useContext, useEffect, useMemo, useState } from 'react';
import { Button, Input, ScrollItem, ScrollList, Typography } from '@douyinfe/semi-ui';
import {
  IconArrowRight,
  IconCheckCircleStroked,
  IconCodeStroked,
  IconCopy,
  IconFile,
  IconGithubLogo,
  IconGlobeStroke,
  IconKeyStroked,
  IconPlay,
  IconRoute,
} from '@douyinfe/semi-icons';
import {
  API,
  copy,
  getLogo,
  getSystemName,
  showError,
  showSuccess,
} from '../../helpers';
import { API_ENDPOINTS } from '../../constants/common.constant';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import NoticeModal from '../../components/layout/NoticeModal';
import { Link } from 'react-router-dom';
import { marked } from 'marked';
import { useTranslation } from 'react-i18next';
import {
  AzureAI,
  Claude,
  Cohere,
  DeepSeek,
  Gemini,
  Grok,
  Hunyuan,
  Midjourney,
  Minimax,
  Moonshot,
  OpenAI,
  Qingyan,
  Qwen,
  Spark,
  Suno,
  Volcengine,
  Wenxin,
  XAI,
  Xinference,
  Zhipu,
} from '@lobehub/icons';

const { Text } = Typography;

const providerIcons = [
  { name: 'OpenAI', icon: <OpenAI size={40} /> },
  { name: 'Claude', icon: <Claude.Color size={40} /> },
  { name: 'Gemini', icon: <Gemini.Color size={40} /> },
  { name: 'DeepSeek', icon: <DeepSeek.Color size={40} /> },
  { name: 'Qwen', icon: <Qwen.Color size={40} /> },
  { name: 'Grok', icon: <Grok size={40} /> },
  { name: 'Moonshot', icon: <Moonshot size={40} /> },
  { name: 'Azure AI', icon: <AzureAI.Color size={40} /> },
  { name: 'Volcengine', icon: <Volcengine.Color size={40} /> },
  { name: 'Zhipu', icon: <Zhipu.Color size={40} /> },
  { name: 'Cohere', icon: <Cohere.Color size={40} /> },
  { name: 'MiniMax', icon: <Minimax.Color size={40} /> },
  { name: 'Wenxin', icon: <Wenxin.Color size={40} /> },
  { name: 'Spark', icon: <Spark.Color size={40} /> },
  { name: 'Hunyuan', icon: <Hunyuan.Color size={40} /> },
  { name: 'Xinference', icon: <Xinference.Color size={40} /> },
  { name: 'Midjourney', icon: <Midjourney size={40} /> },
  { name: 'Suno', icon: <Suno size={40} /> },
  { name: 'Qingyan', icon: <Qingyan.Color size={40} /> },
  { name: 'xAI', icon: <XAI size={40} /> },
];

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

const formatNumber = (value) => {
  const number = Number(value) || 0;
  return new Intl.NumberFormat().format(number);
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

const normalizeChartValue = (value, max, minSize = 12) => {
  const number = Number(value) || 0;
  if (max <= 0 || number <= 0) return minSize;
  return Math.max(minSize, Math.round((number / max) * 100));
};

const buildChartPolyline = (items, valueGetter, maxValue = 100) => {
  const list = Array.isArray(items) ? items : [];
  const width = 720;
  const height = 220;
  const top = 22;
  const bottom = 34;
  const count = Math.max(list.length - 1, 1);
  if (list.length <= 1) {
    return '0,170 144,118 288,132 432,82 576,104 720,70';
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

const publicStatusGroupKeys = [
  { key: 'codex', label: 'Codex 专用' },
  { key: 'claude', label: 'Claude Code' },
  { key: 'speed', label: '高速组' },
  { key: 'value', label: '低价组' },
];

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
  const docsLink = statusState?.status?.docs_link || '';
  const serverAddress =
    statusState?.status?.server_address || `${window.location.origin}`;
  const endpointItems = API_ENDPOINTS.map((e) => ({ value: e }));
  const [endpointIndex, setEndpointIndex] = useState(0);
  const isChinese = i18n.language.startsWith('zh');
  const systemName = statusState?.status?.system_name || getSystemName();
  const logo = statusState?.status?.logo || getLogo();
  const apiBaseUrl = `${serverAddress.replace(/\/$/, '')}/v1`;

  const daily = homeStatus.daily || [];
  const statusGroups = homeStatus.groups || [];
  const summary = homeStatus.summary || fallbackStatus.summary;
  const hasRealData = Number(summary.requests) > 0;
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
        channels: summary.enabled_channels || 0,
      },
    },
    ...publicStatusGroupKeys.map((item) => {
      const group = statusGroups.find((statusGroup) => statusGroup.key === item.key);
      return {
        key: item.key,
        label: t(item.label),
        summary: group?.summary || { ...fallbackStatus.summary, days: summary.days },
        daily: group?.daily || daily,
        states: group?.states || {
          healthy: 0,
          cooling: 0,
          standby: 0,
          channels: 0,
        },
      };
    }),
  ];
  const activeGroup =
    groupTabs.find((item) => item.key === activeStatusGroup) || groupTabs[0];
  const activeSummary = activeGroup.summary || fallbackStatus.summary;
  const activeDaily = activeGroup.daily || [];
  const activeStates = activeGroup.states || {};
  const maxRequests = Math.max(
    ...activeDaily.map((item) => Number(item.requests) || 0),
    0,
  );
  const maxLatency = Math.max(
    ...activeDaily.map((item) => Number(item.avg_latency_ms) || 0),
    0,
  );
  const statusChartPoints = useMemo(
    () => buildChartPolyline(activeDaily, (item) => item.success_rate, 100),
    [activeDaily],
  );
  const latencyChartPoints = useMemo(
    () =>
      buildChartPolyline(
        activeDaily,
        (item) => item.avg_latency_ms,
        Math.max(maxLatency, 1),
      ),
    [activeDaily, maxLatency],
  );

  const heroStats = [
    {
      value: hasRealData ? formatRate(summary.success_rate) : t('初始化中'),
      label: t('近 30 天成功率'),
      note: t('按请求最终结果聚合'),
    },
    {
      value: hasRealData ? formatLatency(summary.avg_latency_ms) : '--',
      label: t('平均响应延迟'),
      note: t('来自真实调用日志'),
    },
    {
      value: formatNumber(summary.enabled_channels),
      label: t('可用渠道池'),
      note: t('自动避开冷却与异常线路'),
    },
  ];

  const heroProofItems = [
    t('高可用架构'),
    t('多渠道智能路由'),
    t('企业级安全'),
    t('按量计费更省钱'),
  ];

  const heroClients = [
    { label: 'Codex', icon: <IconCodeStroked /> },
    { label: 'Claude Code', icon: <IconKeyStroked /> },
    { label: 'OpenAI SDK', icon: <IconRoute /> },
    { label: t('其他客户端'), icon: <IconGlobeStroke /> },
  ];

  const heroChannels = [
    {
      status: t('健康'),
      title: t('通道 A'),
      model: 'OpenAI GPT-4o',
      latency: '320ms',
      tone: 'healthy',
    },
    {
      status: t('冷却中'),
      title: t('通道 B'),
      model: 'Anthropic Claude 3.5',
      latency: '1.2s',
      tone: 'cooling',
    },
    {
      status: t('备用'),
      title: t('通道 C'),
      model: 'OpenAI GPT-4o-mini',
      latency: '580ms',
      tone: 'standby',
    },
  ];

  const scenarioItems = [
    {
      icon: <IconCodeStroked />,
      title: t('长时间编码会话'),
      description: t('Codex 或 Claude Code 连续生成、改错和跑测试时，网关持续选择健康线路，减少中途断流。'),
      meta: t('适合 CLI / IDE 自动化'),
    },
    {
      icon: <IconRoute />,
      title: t('上游限流自动转移'),
      description: t('遇到 429、5xx、超时或流式异常时，自动记录保护事件并切换备用渠道，用户侧仍是同一个入口。'),
      meta: t('多渠道无感切换'),
    },
    {
      icon: <IconKeyStroked />,
      title: t('团队共享与成本控制'),
      description: t('通过令牌、分组倍率、配额和调用日志把个人、团队、脚本流量拆清楚，避免成本失控。'),
      meta: t('低价分组优先'),
    },
  ];

  const statusStateItems = [
    {
      label: t('健康'),
      value: activeStates.healthy || 0,
      tone: 'healthy',
    },
    {
      label: t('冷却中'),
      value: activeStates.cooling || 0,
      tone: 'cooling',
    },
    {
      label: t('备用'),
      value: activeStates.standby || 0,
      tone: 'standby',
    },
  ];

  const routeSteps = [
    {
      title: t('开发工具发起请求'),
      description: t('Codex、Claude Code 或 OpenAI SDK 继续使用熟悉的接口格式。'),
    },
    {
      title: t('智能路由判断'),
      description: t('按模型、分组倍率、健康状态和优先级筛选最佳渠道。'),
    },
    {
      title: t('异常自动转移'),
      description: t('遇到限流、熔断或不可用线路时，自动尝试备用渠道。'),
    },
    {
      title: t('统一计费与日志'),
      description: t('输入、缓存、输出和渠道倍率沉淀为可追踪的明细记录。'),
    },
  ];

  const routerRules = [
    t('按模型匹配'),
    t('按分组策略'),
    t('按健康状态'),
    t('按冷却状态'),
    t('按成本最优'),
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

  const routingBadges = [
    t('多渠道无感切换'),
    t('自动重试与降级'),
    t('智能限流保护'),
    t('更低成本更稳定'),
  ];

  const consoleItems = [
    t('令牌与配额'),
    t('分组倍率'),
    t('渠道监控'),
    t('调用日志'),
    t('订阅计费'),
    t('模型价格'),
  ];

  const clientItems = [
    'Codex CLI',
    'Claude Code',
    'OpenAI SDK',
    'Cursor',
    'Cherry Studio',
    'Lobe Chat',
    'OpenCode',
    'ChatBox',
  ];

  const integrationItems = [
    { label: 'Base URL', value: apiBaseUrl },
    { label: 'Auth', value: 'Bearer sk-...' },
    { label: 'Wire API', value: 'responses / chat completions' },
  ];

  const reliabilityStats = [
    {
      label: t('近 30 天成功率'),
      value: formatRate(activeSummary.success_rate),
      trend: activeGroup.label,
    },
    {
      label: t('平均响应延迟'),
      value: formatLatency(activeSummary.avg_latency_ms),
      trend: t('按分组视图聚合'),
    },
    {
      label: t('请求量'),
      value: formatNumber(activeSummary.requests),
      trend: t('近 30 天真实聚合'),
    },
    {
      label: t('保护事件'),
      value: formatNumber(activeSummary.protected_events),
      trend: t('限流、超时与异常保护'),
    },
  ];

  const displayHomePageContent = async () => {
    setHomePageContent(localStorage.getItem('home_page_content') || '');
    try {
      const res = await API.get('/api/home_page_content');
      const { success, message, data } = res.data;
      if (success) {
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
        showError(message);
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

  const handleCopyBaseURL = async () => {
    const ok = await copy(apiBaseUrl);
    if (ok) {
      showSuccess(t('已复制到剪切板'));
    }
  };

  useEffect(() => {
    const checkNoticeAndShow = async () => {
      const lastCloseDate = localStorage.getItem('notice_close_date');
      const today = new Date().toDateString();
      if (lastCloseDate !== today) {
        try {
          const res = await API.get('/api/notice');
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
    const timer = setInterval(() => {
      setEndpointIndex((prev) => (prev + 1) % endpointItems.length);
    }, 3000);
    return () => clearInterval(timer);
  }, [endpointItems.length]);

  return (
    <div className='w-full overflow-x-hidden'>
      <NoticeModal
        visible={noticeVisible}
        onClose={() => setNoticeVisible(false)}
        isMobile={isMobile}
      />
      {homePageContentLoaded && homePageContent === '' ? (
        <div className='ct-home'>
          <section className='ct-screen ct-hero'>
            <div className='ct-home-shell ct-hero-shell'>
              <div className='ct-hero-copy'>
                <div className='ct-brand-pill'>
                  <img src={logo} alt={systemName} />
                  <span>{systemName}</span>
                  <b>{t('Codex 与 Claude Code 高稳定中转站')}</b>
                </div>
                <h1 className={isChinese ? 'ct-hero-title zh' : 'ct-hero-title'}>
                  {isChinese ? (
                    <>
                      <em>{t('Codex / Claude Code')}</em>
                      <em>{t('稳定高速中转站')}</em>
                    </>
                  ) : (
                    t('Codex / Claude Code 稳定高速中转站')
                  )}
                  <span>{t('多渠道无感切换，低价接入主流编程模型')}</span>
                </h1>
                <p className='ct-hero-desc'>
                  {t('统一 Base URL，自动健康路由。面向 Codex、Claude Code 与 OpenAI SDK 的稳定中转入口，把限流、熔断、价格和渠道波动留在网关侧处理。')}
                </p>
                <div className='ct-hero-actions'>
                  <Link to='/console'>
                    <Button
                      theme='solid'
                      type='primary'
                      size={isMobile ? 'default' : 'large'}
                      className='ct-primary-btn'
                      icon={<IconPlay />}
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
                        icon={<IconFile />}
                        onClick={() => window.open(docsLink, '_blank')}
                      >
                        {t('查看文档')}
                      </Button>
                    )
                  )}
                </div>
                <div className='ct-hero-stats'>
                  {heroStats.map((item) => (
                    <div className='ct-hero-stat' key={item.label}>
                      <strong>{item.value}</strong>
                      <span>{item.label}</span>
                      <small>{item.note}</small>
                    </div>
                  ))}
                </div>
              </div>

              <div className='ct-hero-visual'>
                <div className='ct-ops-panel'>
                  <div className='ct-ops-topbar'>
                    <div>
                      <span>{t('运行状态')}</span>
                      <strong>{t('公开脱敏聚合')}</strong>
                    </div>
                    <b>{homeStatus.partial ? t('部分数据') : t('实时更新')}</b>
                  </div>
                  <div className='ct-workspace-window'>
                    <div className='ct-window-dots'>
                      <i />
                      <i />
                      <i />
                    </div>
                    <code>codex --base-url {apiBaseUrl}</code>
                    <span>{t('长任务编码会话保持在线')}</span>
                  </div>
                  <div className='ct-route-map' aria-label={t('多渠道无感切换')}>
                    <div className='ct-route-node active'>
                      <IconCodeStroked />
                      <span>Codex</span>
                    </div>
                    <div className='ct-route-line'>
                      <i />
                    </div>
                    <div className='ct-route-node router'>
                      <IconRoute />
                      <span>{t('智能路由')}</span>
                    </div>
                    <div className='ct-route-line'>
                      <i />
                    </div>
                    <div className='ct-channel-stack'>
                      {statusStateItems.map((item) => (
                        <div className={`ct-channel-pill ${item.tone}`} key={item.label}>
                          <span>{item.label}</span>
                          <strong>{formatNumber(item.value)}</strong>
                        </div>
                      ))}
                    </div>
                  </div>
                  <div className='ct-connect-panel'>
                    <div className='ct-panel-header'>
                      <span>{t('快速接入')}</span>
                      <b>{t('OpenAI / Claude Compatible')}</b>
                    </div>
                    <div className='ct-base-input'>
                      <Input
                        readOnly
                        value={apiBaseUrl}
                        size={isMobile ? 'default' : 'large'}
                        suffix={
                          <div className='ct-input-suffix'>
                            <ScrollList
                              bodyHeight={32}
                              style={{ border: 'unset', boxShadow: 'unset' }}
                            >
                              <ScrollItem
                                mode='wheel'
                                cycled
                                list={endpointItems}
                                selectedIndex={endpointIndex}
                                onSelect={({ index }) => setEndpointIndex(index)}
                              />
                            </ScrollList>
                            <Button
                              type='primary'
                              onClick={handleCopyBaseURL}
                              icon={<IconCopy />}
                              className='ct-copy-btn'
                            />
                          </div>
                        }
                      />
                    </div>
                    <div className='ct-config-list'>
                      {integrationItems.map((item) => (
                        <div className='ct-config-row' key={item.label}>
                          <span>{item.label}</span>
                          <code>{item.value}</code>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-screen ct-reliability-screen'>
            <div className='ct-home-shell'>
              <div className='ct-section-head wide'>
                <Text className='ct-section-kicker'>{t('真实运行曲线')}</Text>
                <h2>{t('按分组查看成功率、延迟与保护事件')}</h2>
                <p>
                  {t('不同分组的渠道池、价格和访问状态并不相同。公开首页只展示匿名聚合视图，不暴露真实渠道、模型和错误原因。')}
                </p>
              </div>
              <div className='ct-reliability-panel'>
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
                <div className='ct-reliability-summary'>
                  {reliabilityStats.map((item) => (
                    <div className='ct-reliability-stat' key={item.label}>
                      <span>{item.label}</span>
                      <strong>{item.value}</strong>
                      <small>{item.trend}</small>
                    </div>
                  ))}
                </div>
                <div className='ct-chart-stage'>
                  <div className='ct-chart-main'>
                    <div className='ct-chart-head'>
                      <div>
                        <span>{activeGroup.label}</span>
                        <strong>{t('近 30 天成功率')}</strong>
                      </div>
                      <b>{hasRealData ? t('健康') : t('初始化中')}</b>
                    </div>
                    <div className='ct-status-chart' aria-hidden='true'>
                      <svg viewBox='0 0 720 220' preserveAspectRatio='none'>
                        <defs>
                          <linearGradient id='ct-success-fill' x1='0' x2='0' y1='0' y2='1'>
                            <stop offset='0%' stopColor='rgba(20, 184, 166, 0.28)' />
                            <stop offset='100%' stopColor='rgba(20, 184, 166, 0)' />
                          </linearGradient>
                        </defs>
                        <polyline className='success' points={statusChartPoints} />
                        <polyline className='latency' points={latencyChartPoints} />
                      </svg>
                      <div className='ct-volume-bars'>
                        {activeDaily.map((item) => (
                          <i
                            key={item.date}
                            style={{
                              '--bar-height': `${normalizeChartValue(item.requests, maxRequests, 10)}%`,
                            }}
                          />
                        ))}
                      </div>
                      <div className='ct-event-markers'>
                        {activeDaily.map((item) => (
                          <i
                            key={item.date}
                            className={item.protected_events > 0 ? 'active' : ''}
                            style={{
                              '--event-size': `${Math.min(18, 6 + Number(item.protected_events || 0) * 2)}px`,
                            }}
                          />
                        ))}
                      </div>
                    </div>
                  </div>
                  <div className='ct-status-aside'>
                    <div className='ct-state-grid'>
                      {statusStateItems.map((item) => (
                        <div className={`ct-state-card ${item.tone}`} key={item.label}>
                          <span>{item.label}</span>
                          <strong>{formatNumber(item.value)}</strong>
                        </div>
                      ))}
                    </div>
                    <div className='ct-chart-legend'>
                      <span><i className='success' />{t('成功率趋势')}</span>
                      <span><i className='latency' />{t('延迟走势')}</span>
                      <span><i className='volume' />{t('请求量')}</span>
                      <span><i className='event' />{t('保护事件')}</span>
                    </div>
                    <div className='ct-chart-foot'>
                      <span>{t('最后更新')}</span>
                      <strong>{updatedAt}</strong>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-screen ct-routing-screen'>
            <div className='ct-home-shell ct-routing-layout'>
              <div className='ct-section-head'>
                <Text className='ct-section-kicker'>{t('多渠道无感切换')}</Text>
                <h2>{t('把上游波动挡在你的开发工具之外')}</h2>
                <p>
                  {t('一次请求进入网关后，会根据模型、分组倍率、渠道健康和限流状态做动态决策。用户侧仍然是同一个 Base URL，同一把 Key，同一套客户端配置。')}
                </p>
              </div>
              <div className='ct-flow-board'>
                {routeSteps.map((item, index) => (
                  <div className='ct-flow-step' key={item.title}>
                    <span>{String(index + 1).padStart(2, '0')}</span>
                    <h3>{item.title}</h3>
                    <p>{item.description}</p>
                    {index < routeSteps.length - 1 && <IconArrowRight />}
                  </div>
                ))}
              </div>
            </div>
          </section>

          <section className='ct-screen ct-scenario-screen'>
            <div className='ct-home-shell'>
              <div className='ct-section-head wide'>
                <Text className='ct-section-kicker'>{t('场景与生态')}</Text>
                <h2>{t('为 AI 编程高频、长链路、多人共享而设计')}</h2>
                <p>
                  {t('少写配置，多看结果。把长会话、限流转移、团队成本和 IDE 自动化放到一条稳定接入链路里。')}
                </p>
              </div>
              <div className='ct-scenario-grid'>
                {scenarioItems.map((item) => (
                  <div className='ct-scenario-card' key={item.title}>
                    <div className='ct-icon-chip'>{item.icon}</div>
                    <span>{item.meta}</span>
                    <h3>{item.title}</h3>
                    <p>{item.description}</p>
                  </div>
                ))}
              </div>
              <div className='ct-ecosystem-band'>
                <div className='ct-client-grid'>
                  {clientItems.map((item) => (
                    <div className='ct-client-item' key={item}>
                      <IconCheckCircleStroked />
                      {item}
                    </div>
                  ))}
                </div>
                <div className='ct-provider-strip'>
                  {providerIcons.slice(0, 10).map((provider) => (
                    <div className='ct-provider-item' key={provider.name}>
                      {provider.icon}
                      <span>{provider.name}</span>
                    </div>
                  ))}
                  <div className='ct-provider-item ct-provider-more'>
                    <Typography.Text className='!text-2xl font-bold'>40+</Typography.Text>
                    <span>{t('更多渠道')}</span>
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-screen ct-final-screen'>
            <div className='ct-home-shell ct-final-cta'>
              <div>
                <Text className='ct-section-kicker'>{t('开始使用')}</Text>
                <h2>{t('把 Codex 和 Claude Code 切到更稳的 API 入口')}</h2>
                <p>{t('创建令牌后，将 Base URL 设置为本站地址，剩下的模型、渠道、价格和故障切换交给网关处理。')}</p>
                <div className='ct-console-tags'>
                  {consoleItems.map((item) => (
                    <span key={item}>
                      <IconGlobeStroke />
                      {item}
                    </span>
                  ))}
                </div>
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
                    {t('进入控制台')}
                  </Button>
                </Link>
                {docsLink && (
                  <Button
                    size={isMobile ? 'default' : 'large'}
                    className='ct-secondary-btn'
                    icon={<IconKeyStroked />}
                    onClick={() => window.open(docsLink, '_blank')}
                  >
                    {t('查看接入文档')}
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
