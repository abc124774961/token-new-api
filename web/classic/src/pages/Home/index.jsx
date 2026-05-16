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

import React, { useContext, useEffect, useState } from 'react';
import {
  Button,
  Typography,
  Input,
  ScrollList,
  ScrollItem,
} from '@douyinfe/semi-ui';
import {
  API,
  showError,
  copy,
  showSuccess,
  getSystemName,
  getLogo,
} from '../../helpers';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import { API_ENDPOINTS } from '../../constants/common.constant';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { marked } from 'marked';
import { useTranslation } from 'react-i18next';
import {
  IconGithubLogo,
  IconPlay,
  IconFile,
  IconCopy,
  IconShieldStroked,
  IconRoute,
  IconBolt,
  IconActivity,
  IconCloud,
  IconLockStroked,
  IconServerStroked,
  IconCodeStroked,
  IconGlobeStroke,
  IconKeyStroked,
  IconCheckCircleStroked,
  IconLightningStroked,
  IconArrowRight,
} from '@douyinfe/semi-icons';
import { Link } from 'react-router-dom';
import NoticeModal from '../../components/layout/NoticeModal';
import {
  Moonshot,
  OpenAI,
  XAI,
  Zhipu,
  Volcengine,
  Cohere,
  Claude,
  Gemini,
  Suno,
  Minimax,
  Wenxin,
  Spark,
  Qingyan,
  DeepSeek,
  Qwen,
  Midjourney,
  Grok,
  AzureAI,
  Hunyuan,
  Xinference,
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

const Home = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const [homePageContentLoaded, setHomePageContentLoaded] = useState(false);
  const [homePageContent, setHomePageContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
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

  const heroStats = [
    {
      value: '99.9%',
      label: t('稳定性目标'),
      note: t('多渠道池与健康调度共同保障'),
    },
    {
      value: '40+',
      label: t('模型与渠道生态'),
      note: t('覆盖主流编程、对话与多模态模型'),
    },
    {
      value: '1 min',
      label: t('分钟级接入'),
      note: t('替换 Base URL 与 Key 即可迁移'),
    },
  ];

  const trustItems = [
    {
      icon: <IconShieldStroked />,
      title: t('超级稳定'),
      description: t('渠道限流、异常和冷却由网关侧感知，自动避开不健康线路。'),
    },
    {
      icon: <IconRoute />,
      title: t('无感切换'),
      description: t(
        '按模型、分组、优先级和可用状态路由，请求失败时快速切到备用渠道。',
      ),
    },
    {
      icon: <IconBolt />,
      title: t('速度优先'),
      description: t(
        '就近选择低延迟渠道，流式输出保持顺滑，减少等待和重试时间。',
      ),
    },
    {
      icon: <IconActivity />,
      title: t('低价透明'),
      description: t('多组倍率、缓存计费和明细日志可见，团队成本更容易控制。'),
    },
  ];

  const reliabilityStats = [
    {
      label: t('近 30 天成功率'),
      value: '99.92%',
      trend: t('稳定高位'),
    },
    {
      label: t('P95 流式首包'),
      value: '186ms',
      trend: t('快速响应'),
    },
    {
      label: t('自动切换事件'),
      value: '128',
      trend: t('用户无感处理'),
    },
  ];

  const successBars = [94, 98, 96, 99, 97, 100, 98, 95, 99, 97, 100, 98];
  const latencyPoints = [58, 46, 52, 35, 42, 30, 38, 32, 44, 34, 28, 36];

  const featureItems = [
    {
      icon: <IconCodeStroked />,
      title: t('Codex / Claude Code 专属工作流'),
      description: t(
        '兼容 Responses、Chat Completions 与 Claude Messages，适配 CLI、IDE 和自动化脚本。',
      ),
    },
    {
      icon: <IconCloud />,
      title: t('多供应商聚合'),
      description: t(
        '统一接入 OpenAI、Claude、Gemini、DeepSeek、Qwen 等模型，按业务选择最合适的线路。',
      ),
    },
    {
      icon: <IconLockStroked />,
      title: t('企业级令牌治理'),
      description: t(
        '按用户、分组、配额、过期时间和调用权限管理密钥，降低泄露与滥用风险。',
      ),
    },
    {
      icon: <IconServerStroked />,
      title: t('可观测运营面板'),
      description: t(
        '渠道状态、冷却倒计时、调用日志、倍率和订阅信息集中展示，问题定位更快。',
      ),
    },
  ];

  const routeSteps = [
    {
      title: t('开发工具发起请求'),
      description: t(
        'Codex、Claude Code 或 OpenAI SDK 继续使用熟悉的接口格式。',
      ),
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

  const integrationItems = [
    {
      label: 'Base URL',
      value: apiBaseUrl,
    },
    {
      label: 'Auth',
      value: 'Bearer sk-...',
    },
    {
      label: 'Wire API',
      value: 'responses / chat completions',
    },
  ];

  const healthRows = [
    {
      provider: 'codex-main',
      status: t('运行中'),
      latency: '186 ms',
      load: '72%',
    },
    {
      provider: 'claude-sonnet',
      status: t('备用就绪'),
      latency: '214 ms',
      load: '48%',
    },
    {
      provider: 'deepseek-fast',
      status: t('低价线路'),
      latency: '132 ms',
      load: '64%',
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

        // 如果内容是 URL，则发送主题模式
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
                <h1
                  className={isChinese ? 'ct-hero-title zh' : 'ct-hero-title'}
                >
                  {isChinese ? (
                    <>
                      <em>Codex / Claude Code</em>
                      <em>{t('稳定高速中转站')}</em>
                    </>
                  ) : (
                    t('Codex / Claude Code 稳定高速中转站')
                  )}
                  <span>{t('多渠道无感切换，低价接入主流编程模型')}</span>
                </h1>
                <p className='ct-hero-desc'>
                  {t(
                    '为高频 AI 编程场景打造的统一 API 入口。自动调度多家上游渠道，降低单渠道限流、波动和成本压力，让 Codex、Claude Code、OpenAI SDK 与常见客户端稳定跑起来。',
                  )}
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
                        window.open(
                          'https://github.com/QuantumNous/new-api',
                          '_blank',
                        )
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
                    <span>{t('实时调度中控')}</span>
                    <strong>{t('Operational Gateway')}</strong>
                  </div>
                  <b>{t('运行稳定')}</b>
                </div>
                <div className='ct-route-map' aria-hidden='true'>
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
                    <span>{t('低延迟渠道')}</span>
                  </div>
                </div>
                <div className='ct-ops-metrics'>
                  <div>
                    <span>P95</span>
                    <strong>186ms</strong>
                    <small>{t('当前示例延迟')}</small>
                  </div>
                  <div>
                    <span>{t('切换')}</span>
                    <strong>&lt; 1s</strong>
                    <small>{t('故障快速转移')}</small>
                  </div>
                  <div>
                    <span>{t('成本')}</span>
                    <strong>-35%</strong>
                    <small>{t('按低价分组优化')}</small>
                  </div>
                </div>
                <div className='ct-health-list'>
                  {healthRows.map((row) => (
                    <div className='ct-health-row' key={row.provider}>
                      <div>
                        <i />
                        <strong>{row.provider}</strong>
                      </div>
                      <span>{row.status}</span>
                      <code>{row.latency}</code>
                      <small>{row.load}</small>
                    </div>
                  ))}
                </div>
                <div className='ct-connect-panel'>
                  <div className='ct-panel-header'>
                    <span>{t('接入配置')}</span>
                    <b>{t('OpenAI / Claude Compatible')}</b>
                  </div>
                  <div className='ct-base-input'>
                    <Input
                      readonly
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
                              cycled={true}
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
                  <div className='ct-code-preview'>
                    <div>model_provider = "custom"</div>
                    <div>wire_api = "responses"</div>
                    <div>{`base_url = "${apiBaseUrl}"`}</div>
                  </div>
                </div>
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
              <Text className='ct-section-kicker'>{t('稳定运行')}</Text>
              <h2>{t('用近期运行曲线把稳定性讲清楚')}</h2>
              <p>
                {t(
                  '客户最关心的是能不能持续可用。首页直接展示成功率、延迟和自动切换趋势，让服务质量一眼可见。',
                )}
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
                  <b>{t('健康')}</b>
                </div>
                <div className='ct-success-bars' aria-hidden='true'>
                  {successBars.map((height, index) => (
                    <i
                      key={`${height}-${index}`}
                      style={{ '--bar-height': `${height}%` }}
                    />
                  ))}
                </div>
              </div>
              <div className='ct-latency-card'>
                <div className='ct-chart-head'>
                  <div>
                    <span>{t('延迟走势')}</span>
                    <strong>P95 Latency</strong>
                  </div>
                  <b>{t('低延迟')}</b>
                </div>
                <div className='ct-latency-line' aria-hidden='true'>
                  {latencyPoints.map((top, index) => (
                    <i
                      key={`${top}-${index}`}
                      style={{
                        left: `${(index / (latencyPoints.length - 1)) * 100}%`,
                        top: `${top}%`,
                      }}
                    />
                  ))}
                  <svg viewBox='0 0 220 92' preserveAspectRatio='none'>
                    <polyline points='0,54 20,42 40,48 60,32 80,38 100,28 120,35 140,30 160,41 180,31 200,26 220,33' />
                  </svg>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-section'>
            <div className='ct-section-head'>
              <Text className='ct-section-kicker'>{t('无感切换')}</Text>
              <h2>{t('把上游波动挡在你的开发工具之外')}</h2>
              <p>
                {t(
                  '一次请求进入网关后，会根据模型、分组倍率、渠道健康和限流状态做动态决策。用户侧仍然是同一个 Base URL，同一把 Key，同一套客户端配置。',
                )}
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
              <Text className='ct-section-kicker'>
                {t('兼容常用开发者工具')}
              </Text>
              <h2>{t('Codex、Claude Code 和 OpenAI 生态即插即用')}</h2>
              <p>
                {t(
                  '适合个人高频开发、团队共享额度、脚本自动化、IDE 助手和多模型调度场景。',
                )}
              </p>
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
              <p>
                {t(
                  '按需求选择速度、价格和模型能力，不再被单个供应商的额度、地区或稳定性限制。',
                )}
              </p>
            </div>
            <div className='ct-provider-grid'>
              {providerIcons.map((provider) => (
                <div className='ct-provider-item' key={provider.name}>
                  {provider.icon}
                  <span>{provider.name}</span>
                </div>
              ))}
              <div className='ct-provider-item ct-provider-more'>
                <Typography.Text className='!text-2xl font-bold'>
                  40+
                </Typography.Text>
                <span>{t('更多渠道')}</span>
              </div>
            </div>
          </section>

          <section className='ct-section'>
            <div className='ct-section-head'>
              <Text className='ct-section-kicker'>{t('专业控制台')}</Text>
              <h2>{t('不是简单转发，而是一套可运营的 API 网关')}</h2>
              <p>
                {t(
                  '从接入、计费、监控到故障处理都在一个控制台完成，适合长期稳定运营和团队协作。',
                )}
              </p>
            </div>
            <div className='ct-feature-grid'>
              {featureItems.map((item) => (
                <div className='ct-feature-card' key={item.title}>
                  <div className='ct-icon-chip'>{item.icon}</div>
                  <h3>{item.title}</h3>
                  <p>{item.description}</p>
                </div>
              ))}
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
              <p>
                {t(
                  '创建令牌后，将 Base URL 设置为本站地址，剩下的模型、渠道、价格和故障切换交给网关处理。',
                )}
              </p>
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
            <iframe
              src={homePageContent}
              className='w-full h-screen border-none'
            />
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
