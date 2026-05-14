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
  const systemName =
    statusState?.status?.system_name || getSystemName();
  const logo = statusState?.status?.logo || getLogo();
  const apiBaseUrl = `${serverAddress.replace(/\/$/, '')}/v1`;

  const clientItems = [
    'Codex CLI',
    'Codex App',
    'OpenAI SDK',
    'Cherry Studio',
    'Lobe Chat',
    'OpenCode',
  ];

  const featureItems = [
    {
      title: t('Codex 模式兼容'),
      description: t('支持 Responses 接口与常见 OpenAI 标准路径，适配 CLI、App 和 IDE 工作流。'),
    },
    {
      title: t('多渠道自动调度'),
      description: t('按分组、模型与可用状态分发请求，减少单个渠道限流对业务的影响。'),
    },
    {
      title: t('用量与密钥管理'),
      description: t('统一管理用户令牌、配额、日志和计费，适合个人与团队长期使用。'),
    },
    {
      title: t('开发者友好接入'),
      description: t('只需替换 Base URL 和 Key，即可继续使用现有 OpenAI 兼容客户端。'),
    },
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

  const displayHomePageContent = async () => {
    setHomePageContent(localStorage.getItem('home_page_content') || '');
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
      setHomePageContent('加载首页内容失败...');
    }
    setHomePageContentLoaded(true);
  };

  const handleCopyBaseURL = async () => {
    const ok = await copy(serverAddress);
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
                  <b>{t('AI 编程模型订阅与 API 服务平台')}</b>
                </div>
                <h1 className={isChinese ? 'ct-hero-title zh' : 'ct-hero-title'}>
                  {systemName}
                  <span>{t('面向 Codex 与 OpenAI 生态的统一 API 网关')}</span>
                </h1>
                <p className='ct-hero-desc'>
                  {t('聚合主流 AI 编程模型，统一密钥、分组、额度与调用日志。Codex CLI、Codex App、OpenAI SDK 和常见客户端只需替换 Base URL 即可接入。')}
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
              </div>

              <div className='ct-connect-panel'>
                <div className='ct-panel-header'>
                  <span>{t('接入配置')}</span>
                  <b>{t('OpenAI Compatible')}</b>
                </div>
                <div className='ct-base-input'>
                  <Input
                    readonly
                    value={serverAddress}
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
          </section>

          <section className='ct-section ct-client-section'>
            <Text className='ct-section-kicker'>{t('兼容常用开发者工具')}</Text>
            <div className='ct-client-grid'>
              {clientItems.map((item) => (
                <div className='ct-client-item' key={item}>
                  {item}
                </div>
              ))}
            </div>
          </section>

          <section className='ct-section'>
            <div className='ct-section-head'>
              <Text className='ct-section-kicker'>{t('模型渠道')}</Text>
              <h2>{t('一个入口接入多家模型供应商')}</h2>
              <p>{t('按模型、分组和可用状态统一调度，适合 Codex、聊天、图像和自动化工作流。')}</p>
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
              <Text className='ct-section-kicker'>{t('核心能力')}</Text>
              <h2>{t('为二次开发和正式使用准备的网关能力')}</h2>
            </div>
            <div className='ct-feature-grid'>
              {featureItems.map((item) => (
                <div className='ct-feature-card' key={item.title}>
                  <h3>{item.title}</h3>
                  <p>{item.description}</p>
                </div>
              ))}
            </div>
          </section>

          <section className='ct-final-cta'>
            <div>
              <Text className='ct-section-kicker'>{t('开始使用')}</Text>
              <h2>{t('把现有客户端切到新的 API 入口')}</h2>
              <p>{t('创建令牌后，将 Base URL 设置为本站地址，模型和渠道调度由网关统一处理。')}</p>
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
