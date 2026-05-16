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
  IconActivity,
  IconArrowRight,
  IconBolt,
  IconCheckCircleStroked,
  IconCloud,
  IconCodeStroked,
  IconCopy,
  IconFile,
  IconGithubLogo,
  IconGlobeStroke,
  IconKeyStroked,
  IconLightningStroked,
  IconLockStroked,
  IconPlay,
  IconRoute,
  IconServerStroked,
  IconShieldStroked,
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

const Home = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const [homePageContentLoaded, setHomePageContentLoaded] = useState(false);
  const [homePageContent, setHomePageContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [homeStatus, setHomeStatus] = useState(fallbackStatus);
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
  const summary = homeStatus.summary || fallbackStatus.summary;
  const maxRequests = Math.max(...daily.map((item) => Number(item.requests) || 0), 0);
  const maxLatency = Math.max(...daily.map((item) => Number(item.avg_latency_ms) || 0), 0);
  const hasRealData = Number(summary.requests) > 0;
  const updatedAt = homeStatus.updated_at
    ? new Date(homeStatus.updated_at * 1000).toLocaleString()
    : t('等待真实请求数据');

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

  const trustItems = [
    {
      icon: <IconShieldStroked />,
      title: t('超级稳定'),
      description: t('渠道限流、异常和冷却由网关侧感知，自动避开不健康线路。'),
    },
    {
      icon: <IconBolt />,
      title: t('速度优先'),
      description: t('就近选择低延迟渠道，流式输出保持顺滑，减少等待和重试时间。'),
    },
    {
      icon: <IconActivity />,
      title: t('低价透明'),
      description: t('多组倍率、缓存计费和明细日志可见，团队成本更容易控制。'),
    },
    {
      icon: <IconLockStroked />,
      title: t('企业级令牌治理'),
      description: t('按用户、分组、配额、过期时间和调用权限管理密钥，降低泄露与滥用风险。'),
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
      label: t('总请求量'),
      value: formatNumber(summary.requests),
      trend: t('近 30 天真实聚合'),
    },
    {
      label: t('保护事件'),
      value: formatNumber(summary.protected_events),
      trend: t('限流、超时与异常保护'),
    },
    {
      label: t('健康渠道'),
      value: `${formatNumber(summary.healthy_channels)} / ${formatNumber(summary.enabled_channels)}`,
      trend: t('公开脱敏汇总'),
    },
  ];

  const latencyPoints = useMemo(() => {
    const points = daily.map((item, index) => {
      const count = Math.max(daily.length - 1, 1);
      const x = (index / count) * 220;
      const latency = Number(item.avg_latency_ms) || 0;
      const y = 84 - (normalizeChartValue(latency, maxLatency, 8) / 100) * 66;
      return `${x.toFixed(1)},${Math.max(14, Math.min(84, y)).toFixed(1)}`;
    });
    if (points.length > 1) return points.join(' ');
    return '0,72 55,58 110,66 165,44 220,52';
  }, [daily, maxLatency]);

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
          <section className='ct-hero'>
            <div className='ct-hero-shell'>
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
                  {t('为高频 AI 编程场景打造的统一 API 入口。自动调度多家上游渠道，降低单渠道限流、波动和成本压力，让 Codex、Claude Code、OpenAI SDK 与常见客户端稳定跑起来。')}
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

              <div className='ct-ops-panel'>
                <div className='ct-ops-topbar'>
                  <div>
                    <span>{t('真实运行状态')}</span>
                    <strong>{t('公开脱敏聚合')}</strong>
                  </div>
                  <b>{homeStatus.partial ? t('部分数据') : t('实时更新')}</b>
                </div>
                <div className='ct-scenario-board' aria-hidden='true'>
                  <div className='ct-workspace-window'>
                    <div className='ct-window-dots'>
                      <i />
                      <i />
                      <i />
                    </div>
                    <code>codex --model gpt-5.5 --base-url {apiBaseUrl}</code>
                    <span>{t('长任务编码会话保持在线')}</span>
                  </div>
                  <div className='ct-route-map'>
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
                    <div className='ct-route-node active'>
                      <IconLightningStroked />
                      <span>{t('健康渠道')}</span>
                    </div>
                  </div>
                </div>
                <div className='ct-ops-metrics'>
                  <div>
                    <span>{t('成功率')}</span>
                    <strong>{formatRate(summary.success_rate)}</strong>
                    <small>{t('按请求最终结果聚合')}</small>
                  </div>
                  <div>
                    <span>{t('平均延迟')}</span>
                    <strong>{formatLatency(summary.avg_latency_ms)}</strong>
                    <small>{t('真实调用日志')}</small>
                  </div>
                  <div>
                    <span>{t('保护事件')}</span>
                    <strong>{formatNumber(summary.protected_events)}</strong>
                    <small>{t('限流与异常已处理')}</small>
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
          </section>

          <section className='ct-section ct-scenarios-section'>
            <div className='ct-section-head centered'>
              <Text className='ct-section-kicker'>{t('真实使用场景')}</Text>
              <h2>{t('为 AI 编程高频、长链路、多人共享而设计')}</h2>
              <p>
                {t('首页不只展示功能，而是把客户会遇到的限流、断流、成本和团队治理问题放到同一条接入链路里解决。')}
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
          </section>

          <section className='ct-section'>
            <div className='ct-section-head'>
              <Text className='ct-section-kicker'>{t('稳定运行')}</Text>
              <h2>{t('用真实运行曲线把稳定性讲清楚')}</h2>
              <p>
                {t('公开首页只展示脱敏后的全站汇总：成功率、延迟、请求量和保护事件，不暴露渠道、模型和错误原因。')}
              </p>
            </div>
            <div className='ct-reliability-panel'>
              <div className='ct-reliability-summary'>
                {reliabilityStats.map((item) => (
                  <div className='ct-reliability-stat' key={item.label}>
                    <span>{item.label}</span>
                    <strong>{item.value}</strong>
                    <small>{item.trend}</small>
                  </div>
                ))}
              </div>
              <div className='ct-reliability-chart'>
                <div className='ct-chart-head'>
                  <div>
                    <span>{t('成功率趋势')}</span>
                    <strong>{t('近 30 天')}</strong>
                  </div>
                  <b>{hasRealData ? t('健康') : t('初始化中')}</b>
                </div>
                <div className='ct-success-bars' aria-hidden='true'>
                  {daily.map((item) => (
                    <i
                      key={item.date}
                      style={{
                        '--bar-height': `${normalizeChartValue(item.success_rate, 100, 16)}%`,
                      }}
                    >
                      <span>{formatRate(item.success_rate)}</span>
                    </i>
                  ))}
                </div>
              </div>
              <div className='ct-latency-card'>
                <div className='ct-chart-head'>
                  <div>
                    <span>{t('延迟走势')}</span>
                    <strong>{t('平均延迟')}</strong>
                  </div>
                  <b>{t('低延迟')}</b>
                </div>
                <div className='ct-latency-line' aria-hidden='true'>
                  <svg viewBox='0 0 220 92' preserveAspectRatio='none'>
                    <polyline points={latencyPoints} />
                  </svg>
                  <div className='ct-request-heat'>
                    {daily.map((item) => (
                      <i
                        key={item.date}
                        style={{
                          '--heat-opacity':
                            normalizeChartValue(item.requests, maxRequests, 8) / 100,
                        }}
                      />
                    ))}
                  </div>
                </div>
              </div>
              <div className='ct-chart-foot'>
                <span>{t('最后更新')}</span>
                <strong>{updatedAt}</strong>
              </div>
            </div>
          </section>

          <section className='ct-section ct-trust-section'>
            <div className='ct-trust-grid'>
              {trustItems.map((item) => (
                <div className='ct-trust-card' key={item.title}>
                  <div className='ct-icon-chip'>{item.icon}</div>
                  <h3>{item.title}</h3>
                  <p>{item.description}</p>
                </div>
              ))}
            </div>
          </section>

          <section className='ct-section'>
            <div className='ct-section-head'>
              <Text className='ct-section-kicker'>{t('无感切换')}</Text>
              <h2>{t('把上游波动挡在你的开发工具之外')}</h2>
              <p>
                {t('一次请求进入网关后，会根据模型、分组倍率、渠道健康和限流状态做动态决策。用户侧仍然是同一个 Base URL，同一把 Key，同一套客户端配置。')}
              </p>
            </div>
            <div className='ct-switch-grid'>
              {routeSteps.map((item, index) => (
                <div className='ct-switch-step' key={item.title}>
                  <span>{String(index + 1).padStart(2, '0')}</span>
                  <h3>{item.title}</h3>
                  <p>{item.description}</p>
                  {index < routeSteps.length - 1 && <IconArrowRight />}
                </div>
              ))}
            </div>
          </section>

          <section className='ct-section'>
            <div className='ct-section-head'>
              <Text className='ct-section-kicker'>{t('兼容常用开发者工具')}</Text>
              <h2>{t('Codex、Claude Code 和 OpenAI 生态即插即用')}</h2>
              <p>{t('适合个人高频开发、团队共享额度、脚本自动化、IDE 助手和多模型调度场景。')}</p>
            </div>
            <div className='ct-client-grid'>
              {clientItems.map((item) => (
                <div className='ct-client-item' key={item}>
                  <IconCheckCircleStroked />
                  {item}
                </div>
              ))}
            </div>
          </section>

          <section className='ct-section'>
            <div className='ct-section-head'>
              <Text className='ct-section-kicker'>{t('模型渠道')}</Text>
              <h2>{t('一个入口聚合主流 AI 供应商')}</h2>
              <p>{t('按需求选择速度、价格和模型能力，不再被单个供应商的额度、地区或稳定性限制。')}</p>
            </div>
            <div className='ct-provider-grid'>
              {providerIcons.map((provider) => (
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
          </section>

          <section className='ct-section'>
            <div className='ct-section-head'>
              <Text className='ct-section-kicker'>{t('专业控制台')}</Text>
              <h2>{t('不是简单转发，而是一套可运营的 API 网关')}</h2>
              <p>{t('从接入、计费、监控到故障处理都在一个控制台完成，适合长期稳定运营和团队协作。')}</p>
            </div>
            <div className='ct-console-band'>
              <div>
                <Text className='ct-section-kicker'>{t('控制台能力')}</Text>
                <h3>{t('给管理员和团队成员都留出清晰边界')}</h3>
              </div>
              <div className='ct-console-tags'>
                {consoleItems.map((item) => (
                  <span key={item}>
                    <IconGlobeStroke />
                    {item}
                  </span>
                ))}
              </div>
            </div>
          </section>

          <section className='ct-final-cta'>
            <div>
              <Text className='ct-section-kicker'>{t('开始使用')}</Text>
              <h2>{t('把 Codex 和 Claude Code 切到更稳的 API 入口')}</h2>
              <p>{t('创建令牌后，将 Base URL 设置为本站地址，剩下的模型、渠道、价格和故障切换交给网关处理。')}</p>
            </div>
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
