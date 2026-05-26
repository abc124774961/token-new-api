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

import React, { useContext, useEffect, useRef, useState } from 'react';
import { Button } from '@douyinfe/semi-ui';
import { IconLock, IconSafe } from '@douyinfe/semi-icons';
import {
  Activity,
  Box,
  Check,
  FileSearch,
  LockKeyhole,
  Route,
  Shield,
  Shuffle,
} from 'lucide-react';
import { Link } from 'react-router-dom';
import { marked } from 'marked';
import { useTranslation } from 'react-i18next';
import { API } from '../../helpers';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import NoticeModal from '../../components/layout/NoticeModal';

const fallbackStatus = {
  summary: {
    success_rate: 0,
    avg_latency_ms: 0,
    enabled_channels: 0,
    healthy_channels: 0,
  },
  updated_at: 0,
};

const isEmptyHomeContent = (content) => {
  const normalized = String(content || '')
    .replace(/&nbsp;/gi, '')
    .replace(/<p>\s*<\/p>/gi, '')
    .replace(/<br\s*\/?>/gi, '')
    .trim();
  return normalized === '';
};

const numberValue = (value, fallback = 0) => {
  const number = Number(value);
  return Number.isFinite(number) ? number : fallback;
};

const formatRate = (value) => {
  const number = Number(value) || 0;
  if (number <= 0) return '99.62%';
  return `${number >= 99 ? number.toFixed(2) : number.toFixed(1)}%`;
};

const formatLatency = (value) => {
  const number = Number(value) || 0;
  if (number <= 0) return '428ms';
  if (number >= 1000) return `${(number / 1000).toFixed(2)}s`;
  return `${Math.round(number)}ms`;
};

const Sparkline = () => (
  <svg viewBox='0 0 96 28' aria-hidden='true'>
    <polyline points='2,20 12,18 21,21 31,13 40,18 50,12 60,17 70,11 80,16 94,10' />
  </svg>
);

const MiniBars = ({ count = 9 }) => (
  <span className='ct-lite-bars' aria-hidden='true'>
    {Array.from({ length: count }).map((_, index) => (
      <i key={index} />
    ))}
  </span>
);

const MetricTile = ({ label, value, type = 'line' }) => (
  <div className='ct-lite-metric-tile'>
    <span>{label}</span>
    <strong>{value}</strong>
    {type === 'bars' ? <MiniBars /> : <Sparkline />}
  </div>
);

const drawRoundRect = (ctx, x, y, width, height, radius) => {
  const r = Math.min(radius, width / 2, height / 2);
  ctx.beginPath();
  ctx.moveTo(x + r, y);
  ctx.arcTo(x + width, y, x + width, y + height, r);
  ctx.arcTo(x + width, y + height, x, y + height, r);
  ctx.arcTo(x, y + height, x, y, r);
  ctx.arcTo(x, y, x + width, y, r);
  ctx.closePath();
};

const GatewayFlowCanvas = ({
  successRate,
  avgLatency,
  channelText,
  channels,
  locale,
  t,
}) => {
  const canvasRef = useRef(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return undefined;

    const ctx = canvas.getContext('2d');
    const prefersReducedMotion = window.matchMedia(
      '(prefers-reduced-motion: reduce)',
    ).matches;
    let animationFrame = 0;
    let width = 0;
    let height = 0;

    const baseWidth = 1480;
    const baseHeight = 560;
    const color = {
      ink: '#0b1b33',
      muted: '#64748b',
      faint: '#8ca0b7',
      teal: '#0d9ca5',
      tealDark: '#087d85',
      cyan: '#23c7cf',
      green: '#16a34a',
      red: '#ef4444',
      amber: '#f59e0b',
      blue: '#3b82f6',
      border: 'rgba(15, 23, 42, 0.08)',
    };

    const resize = () => {
      const rect = canvas.getBoundingClientRect();
      const dpr = Math.min(window.devicePixelRatio || 1, 2);
      width = Math.max(1, rect.width);
      height = Math.max(1, rect.height);
      canvas.width = Math.round(width * dpr);
      canvas.height = Math.round(height * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    };

    const setFont = (size, weight = 800, family = 'sans-serif') => {
      const stack =
        family === 'mono'
          ? 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace'
          : '-apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif';
      ctx.font = `${weight} ${size}px ${stack}`;
      ctx.textBaseline = 'alphabetic';
    };

    const fillText = (text, x, y, options = {}) => {
      const {
        size = 13,
        weight = 800,
        fill = color.ink,
        align = 'left',
        family,
      } = options;
      setFont(size, weight, family);
      ctx.fillStyle = fill;
      ctx.textAlign = align;
      ctx.fillText(String(text), x, y);
    };

    const drawCard = (x, y, w, h, options = {}) => {
      const {
        radius = 10,
        fill = 'rgba(255,255,255,0.92)',
        border = color.border,
        shadow = true,
      } = options;
      ctx.save();
      if (shadow) {
        ctx.shadowColor = 'rgba(15, 42, 71, 0.09)';
        ctx.shadowBlur = 34;
        ctx.shadowOffsetY = 16;
      }
      drawRoundRect(ctx, x, y, w, h, radius);
      ctx.fillStyle = fill;
      ctx.fill();
      ctx.restore();
      ctx.save();
      drawRoundRect(ctx, x, y, w, h, radius);
      ctx.strokeStyle = border;
      ctx.lineWidth = 1;
      ctx.stroke();
      ctx.restore();
    };

    const drawMetric = (x, y, label, value, type = 'line') => {
      drawCard(x, y, 126, 124);
      fillText(label, x + 18, y + 32, {
        size: 12,
        weight: 850,
        fill: '#718197',
      });
      fillText(value, x + 18, y + 68, {
        size: 24,
        weight: 950,
        fill: color.teal,
        family: 'mono',
      });

      if (type === 'bars') {
        const bars = [15, 22, 27, 18, 31, 24, 34, 28, 36];
        bars.forEach((bar, index) => {
          const bx = x + 20 + index * 8;
          const by = y + 98 - bar;
          drawRoundRect(ctx, bx, by, 5, bar, 3);
          ctx.fillStyle = color.teal;
          ctx.fill();
        });
        return;
      }

      ctx.save();
      ctx.beginPath();
      const points = [
        [20, 94],
        [33, 91],
        [45, 95],
        [58, 86],
        [70, 93],
        [84, 84],
        [96, 91],
        [111, 84],
      ];
      points.forEach(([px, py], index) => {
        if (index === 0) ctx.moveTo(x + px, y + py);
        else ctx.lineTo(x + px, y + py);
      });
      ctx.strokeStyle = color.teal;
      ctx.lineWidth = 3;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      ctx.stroke();
      ctx.restore();
    };

    const drawPulseIcon = (x, y) => {
      ctx.save();
      ctx.beginPath();
      ctx.moveTo(x - 20, y + 2);
      ctx.lineTo(x - 10, y + 2);
      ctx.lineTo(x - 4, y - 18);
      ctx.lineTo(x + 8, y + 22);
      ctx.lineTo(x + 14, y + 4);
      ctx.lineTo(x + 24, y + 4);
      ctx.strokeStyle = color.teal;
      ctx.lineWidth = 4;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      ctx.stroke();
      ctx.restore();
    };

    const drawMessageIcon = (x, y) => {
      ctx.save();
      drawRoundRect(ctx, x - 22, y - 22, 44, 34, 6);
      ctx.strokeStyle = color.teal;
      ctx.lineWidth = 4;
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(x - 10, y - 10);
      ctx.lineTo(x + 12, y - 10);
      ctx.moveTo(x - 10, y + 2);
      ctx.lineTo(x + 7, y + 2);
      ctx.stroke();
      ctx.restore();
    };

    const drawPill = (x, y, text, options = {}) => {
      const {
        fill = 'rgba(255,255,255,0.9)',
        stroke = color.border,
        textColor = color.ink,
        width: pillWidth,
      } = options;
      setFont(12, 900);
      const w = pillWidth || Math.max(82, ctx.measureText(text).width + 28);
      const h = 36;
      drawRoundRect(ctx, x, y, w, h, 9);
      ctx.fillStyle = fill;
      ctx.fill();
      ctx.strokeStyle = stroke;
      ctx.lineWidth = 1;
      ctx.stroke();
      fillText(text, x + w / 2, y + 23, {
        size: 12,
        weight: 900,
        fill: textColor,
        align: 'center',
      });
    };

    const drawFlowLine = (x1, y1, x2, y2, progress, stroke, glow) => {
      ctx.save();
      ctx.beginPath();
      ctx.moveTo(x1, y1);
      ctx.lineTo(x2, y2);
      ctx.strokeStyle = stroke;
      ctx.lineWidth = 2;
      ctx.stroke();

      const px = x1 + (x2 - x1) * progress;
      const py = y1 + (y2 - y1) * progress;
      const gradient = ctx.createRadialGradient(px, py, 1, px, py, 16);
      gradient.addColorStop(0, glow);
      gradient.addColorStop(1, 'rgba(255,255,255,0)');
      ctx.fillStyle = gradient;
      ctx.beginPath();
      ctx.arc(px, py, 16, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(px, py, 4, 0, Math.PI * 2);
      ctx.fill();
      ctx.restore();
    };

    const drawChannel = (channel, x, y) => {
      const statusColor =
        channel.tone === 'failed'
          ? color.red
          : channel.tone === 'cooling'
            ? color.amber
            : color.green;
      drawCard(x, y, 320, 70, {
        radius: 8,
        fill: 'rgba(255,255,255,0.93)',
      });
      ctx.fillStyle = statusColor;
      ctx.beginPath();
      ctx.arc(x + 22, y + 35, 5, 0, Math.PI * 2);
      ctx.fill();
      fillText(channel.name, x + 46, y + 29, {
        size: 14,
        weight: 950,
      });
      fillText(channel.meta, x + 46, y + 50, {
        size: 12,
        weight: 800,
        fill: color.muted,
      });
      if (channel.provider) {
        fillText(channel.provider, x + 262, y + 38, {
          size: 12,
          weight: 850,
          fill: '#5f7190',
          align: 'right',
        });
      }
      fillText(channel.status, x + 304, y + 38, {
        size: 12,
        weight: 950,
        fill: statusColor,
        align: 'right',
      });
    };

    const drawScene = (time = 0) => {
      ctx.clearRect(0, 0, width, height);
      ctx.save();
      ctx.scale(width / baseWidth, height / baseHeight);

      const bg = ctx.createLinearGradient(0, 0, baseWidth, baseHeight);
      bg.addColorStop(0, '#ffffff');
      bg.addColorStop(0.52, '#f7fdff');
      bg.addColorStop(1, '#fbfdff');
      ctx.fillStyle = bg;
      ctx.fillRect(0, 0, baseWidth, baseHeight);

      for (let x = 0; x <= baseWidth; x += 42) {
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, baseHeight);
        ctx.strokeStyle = 'rgba(13, 156, 165, 0.045)';
        ctx.lineWidth = 1;
        ctx.stroke();
      }
      for (let y = 0; y <= baseHeight; y += 42) {
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(baseWidth, y);
        ctx.strokeStyle = 'rgba(13, 156, 165, 0.045)';
        ctx.lineWidth = 1;
        ctx.stroke();
      }

      const glow = ctx.createRadialGradient(470, 320, 40, 470, 320, 310);
      glow.addColorStop(0, 'rgba(35, 199, 207, 0.26)');
      glow.addColorStop(0.34, 'rgba(35, 199, 207, 0.08)');
      glow.addColorStop(1, 'rgba(35, 199, 207, 0)');
      ctx.fillStyle = glow;
      ctx.fillRect(120, 40, 760, 500);

      const lineGradient = ctx.createLinearGradient(190, 305, 1280, 305);
      lineGradient.addColorStop(0, 'rgba(59, 130, 246, 0.72)');
      lineGradient.addColorStop(0.35, 'rgba(13, 156, 165, 0.88)');
      lineGradient.addColorStop(0.56, 'rgba(239, 68, 68, 0.62)');
      lineGradient.addColorStop(1, 'rgba(34, 197, 94, 0.7)');

      const phase = prefersReducedMotion ? 0.48 : (time * 0.18) % 1;
      drawFlowLine(190, 305, 465, 305, phase, lineGradient, 'rgba(35, 199, 207, 0.95)');
      drawFlowLine(530, 305, 740, 135, (phase + 0.2) % 1, 'rgba(13,156,165,0.45)', 'rgba(34,197,94,0.9)');
      drawFlowLine(530, 305, 740, 215, (phase + 0.38) % 1, 'rgba(13,156,165,0.45)', 'rgba(34,197,94,0.9)');
      drawFlowLine(530, 305, 740, 295, (phase + 0.55) % 1, 'rgba(239,68,68,0.34)', 'rgba(239,68,68,0.85)');
      drawFlowLine(530, 305, 740, 375, (phase + 0.7) % 1, 'rgba(13,156,165,0.45)', 'rgba(34,197,94,0.9)');
      drawFlowLine(530, 305, 1280, 305, (phase + 0.45) % 1, lineGradient, 'rgba(34,197,94,0.88)');

      drawMetric(48, 48, t('成功率'), successRate);
      drawMetric(200, 48, t('平均延迟'), avgLatency);
      drawMetric(352, 48, t('健康渠道'), channelText, 'bars');

      drawCard(50, 278, 118, 154, { radius: 8 });
      drawPulseIcon(109, 326);
      fillText(t('客户端'), 109, 376, {
        size: 14,
        weight: 950,
        align: 'center',
      });
      fillText(t('请求'), 109, 407, {
        size: 12,
        weight: 820,
        fill: color.muted,
        align: 'center',
      });

      ctx.save();
      const pulse = prefersReducedMotion ? 0.5 : (Math.sin(time * 2) + 1) / 2;
      const coreGradient = ctx.createRadialGradient(470, 305, 12, 470, 305, 84);
      coreGradient.addColorStop(0, '#ffffff');
      coreGradient.addColorStop(0.32, '#ffffff');
      coreGradient.addColorStop(0.33, color.teal);
      coreGradient.addColorStop(0.72, color.cyan);
      coreGradient.addColorStop(1, color.tealDark);
      ctx.fillStyle = `rgba(13,156,165,${0.1 + pulse * 0.06})`;
      ctx.beginPath();
      ctx.arc(470, 305, 76 + pulse * 8, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = coreGradient;
      ctx.beginPath();
      ctx.arc(470, 305, 58, 0, Math.PI * 2);
      ctx.fill();
      ctx.strokeStyle = 'rgba(255,255,255,0.78)';
      ctx.lineWidth = 8;
      ctx.beginPath();
      ctx.arc(470, 305, 34, 0, Math.PI * 2);
      ctx.stroke();
      ctx.restore();
      fillText(t('智能网关'), 470, 392, {
        size: 14,
        weight: 950,
        align: 'center',
      });
      fillText(t('智能评分'), 470, 415, {
        size: 12,
        weight: 820,
        fill: color.muted,
        align: 'center',
      });

      [t('能力匹配'), t('成本权重'), t('健康评分'), t('失败率降权')].forEach(
        (item, index) => {
          drawPill(250 + index * 96, 458, item, {
            fill: 'rgba(255,255,255,0.88)',
            stroke: 'rgba(15,23,42,0.08)',
            textColor: '#5c6d83',
            width: 82,
          });
        },
      );

      channels.forEach((channel, index) => {
        drawChannel(channel, 740, 100 + index * 82);
      });

      drawPill(1160, 344, t('自动切换'), {
        fill: 'rgba(255,255,255,0.95)',
        stroke: 'rgba(239,68,68,0.24)',
        textColor: color.red,
        width: 98,
      });
      fillText('↻', 1176, 367, {
        size: 13,
        weight: 900,
        fill: color.red,
      });

      drawCard(1290, 278, 130, 154, { radius: 8 });
      drawMessageIcon(1355, 322);
      fillText(t('流式响应输出'), 1355, 376, {
        size: 14,
        weight: 950,
        align: 'center',
      });
      fillText(t('流式保持稳定'), 1355, 399, {
        size: 12,
        weight: 820,
        fill: color.muted,
        align: 'center',
      });
      drawPill(1318, 414, `${t('流式保持')} ✓`, {
        fill: 'rgba(34,197,94,0.1)',
        stroke: 'rgba(34,197,94,0.12)',
        textColor: color.green,
        width: 74,
      });

      ctx.restore();
    };

    const render = (time) => {
      drawScene(time / 1000);
      if (!prefersReducedMotion) {
        animationFrame = requestAnimationFrame(render);
      }
    };

    resize();
    const observer = new ResizeObserver(() => {
      resize();
      if (prefersReducedMotion) drawScene(0);
    });
    observer.observe(canvas);
    render(0);

    return () => {
      observer.disconnect();
      cancelAnimationFrame(animationFrame);
    };
  }, [successRate, avgLatency, channelText, channels, locale, t]);

  return (
    <canvas
      ref={canvasRef}
      className='ct-lite-flow-canvas'
      aria-label={t('智能调度与失败切换流程')}
    />
  );
};

const Home = () => {
  const { t, i18n } = useTranslation();
  const [statusState] = useContext(StatusContext);
  const actualTheme = useActualTheme();
  const isMobile = useIsMobile();
  const [homePageContentLoaded, setHomePageContentLoaded] = useState(false);
  const [homePageContent, setHomePageContent] = useState('');
  const [noticeVisible, setNoticeVisible] = useState(false);
  const [homeStatus, setHomeStatus] = useState(fallbackStatus);

  const summary = homeStatus.summary || fallbackStatus.summary;
  const hasRealData = Number(summary.enabled_channels || summary.healthy_channels) > 0;
  const successRate = hasRealData ? formatRate(summary.success_rate) : '99.62%';
  const avgLatency = hasRealData ? formatLatency(summary.avg_latency_ms) : '428ms';
  const enabledChannels = hasRealData ? numberValue(summary.enabled_channels, 42) : 42;
  const healthyChannels = hasRealData
    ? Math.min(numberValue(summary.healthy_channels, 38), enabledChannels || 38)
    : 38;
  const channelText = `${healthyChannels}/${enabledChannels || 42}`;

  const heroHighlights = [
    {
      icon: <IconSafe />,
      title: t('稳定可靠'),
      desc: t('多路自动容灾'),
    },
    {
      icon: <Route size={18} />,
      title: t('智能调度'),
      desc: t('延迟更低，成功率更高'),
    },
    {
      icon: <Activity size={18} />,
      title: t('按量计费'),
      desc: t('透明可追溯'),
    },
    {
      icon: <IconLock />,
      title: t('安全可控'),
      desc: t('令牌与访问控制'),
    },
  ];

  const featureCards = [
    {
      icon: <Shuffle />,
      title: t('多渠道自动切换'),
      desc: t('智能健康检测与延迟评估，自动选择最优通道，保障请求成功率与稳定性。'),
    },
    {
      icon: <Box />,
      title: t('模型与工具能力识别'),
      desc: t('自动识别模型与工具能力，适配请求与参数，减少报错与重试成本。'),
    },
    {
      icon: <Shield />,
      title: t('熔断限流旁路'),
      desc: t('多维熔断与限流策略，异常自动隔离与降级，保护上游服务与整体可用性。'),
    },
    {
      icon: <FileSearch />,
      title: t('统一账单与用量追踪'),
      desc: t('聚合多渠道账单与用量，按模型、分组和倍率计费，账单清晰可追溯。'),
    },
  ];

  const flowChannels = [
    {
      name: 'Channel #1',
      provider: 'OpenAI',
      meta: t('延迟 312ms'),
      status: t('健康'),
      tone: 'healthy',
    },
    {
      name: 'Channel #2',
      provider: 'Anthropic',
      meta: t('延迟 421ms'),
      status: t('健康'),
      tone: 'healthy',
    },
    {
      name: 'Channel #4',
      provider: '',
      meta: 'HTTP 502 / 熔断',
      status: t('失败'),
      tone: 'failed',
    },
    {
      name: 'Channel #7',
      provider: 'Google Gemini',
      meta: t('延迟 289ms'),
      status: t('健康'),
      tone: 'healthy',
    },
    {
      name: 'Channel #9',
      provider: 'DeepSeek',
      meta: t('冷却中 60s'),
      status: t('冷却中'),
      tone: 'cooling',
    },
  ];
  const flowChannelKey = flowChannels
    .map((channel) => `${channel.name}-${channel.meta}-${channel.status}`)
    .join('|');

  const dynamicPriceItems = [
    {
      label: t('输入价格'),
      value: '$2.50 / 1M',
      detail: t('模型价格 × 分组倍率'),
    },
    {
      label: t('输出价格'),
      value: '$15.00 / 1M',
      detail: t('补全与流式输出'),
    },
    {
      label: t('缓存读取价格'),
      value: '$0.25 / 1M',
      detail: t('缓存命中单独展示'),
    },
    {
      label: t('图片生成'),
      value: t('按次 / 按量'),
      detail: t('图片费用独立明细'),
    },
  ];

  const dynamicPriceRules = [
    t('分组倍率实时生效'),
    t('表达式/阶梯计费'),
    t('请求条件调价'),
    t('用量明细可追溯'),
  ];

  const planCards = [
    {
      name: t('入门版'),
      subtitle: t('适合个人开发者与轻量项目'),
      price: '¥ 29',
      perks: [
        '10,000 次请求 / 月',
        t('支持主流模型接入'),
        t('基础自动切换'),
        t('7 天账单与用量追踪'),
        t('社区支持'),
      ],
    },
    {
      name: t('专业版'),
      subtitle: t('适合成长型团队与生产环境'),
      price: '¥ 99',
      featured: true,
      perks: [
        '100,000 次请求 / 月',
        t('高级智能调度与健康检测'),
        t('熔断限流与旁路保护'),
        t('30 天账单与用量追踪'),
        t('优先工单支持'),
      ],
    },
    {
      name: t('团队版'),
      subtitle: t('适合中大型团队与企业级需求'),
      price: '¥ 299',
      perks: [
        '500,000 次请求 / 月',
        t('自定义分组与倍率计费'),
        t('专属通道与更高配额'),
        t('90 天账单与用量追踪'),
        t('专属技术支持'),
      ],
    },
  ];

  const bottomTrust = [
    {
      icon: <LockKeyhole />,
      title: t('数据安全'),
      desc: t('传输加密与访问控制'),
    },
    {
      icon: <Shield />,
      title: t('高可用架构'),
      desc: t('多通道带宽与故障自愈'),
    },
    {
      icon: <Activity />,
      title: t('稳定低延迟'),
      desc: t('全球节点优化路由'),
    },
    {
      icon: <FileSearch />,
      title: t('透明可控'),
      desc: t('明细计费，成本可视'),
    },
  ];

  const displayHomePageContent = async () => {
    const cachedContent = localStorage.getItem('home_page_content') || '';
    setHomePageContent(cachedContent);
    setHomePageContentLoaded(true);

    try {
      const res = await API.get('/api/home_page_content', {
        skipErrorHandler: true,
      });
      const { success, message, data } = res.data;
      if (success) {
        if (isEmptyHomeContent(data)) {
          setHomePageContent('');
          localStorage.removeItem('home_page_content');
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
        setHomePageContent(cachedContent);
      }
    } catch (error) {
      console.error('加载首页内容失败:', error);
      setHomePageContent(cachedContent);
    }
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
      {!homePageContentLoaded ? (
        <div className='ct-home-loading'>{t('加载中...')}</div>
      ) : homePageContent === '' ? (
        <main className='ct-home-lite'>
          <section className='ct-lite-hero'>
            <div className='ct-lite-shell ct-lite-hero-grid'>
              <div className='ct-lite-hero-copy'>
                <h1>{t('统一 AI API 网关，稳定接入多家模型服务')}</h1>
                <p>
                  {t(
                    '多渠道智能调度、熔断旁路、透明计费，让 Codex、Claude Code 与 OpenAI SDK 使用更稳定。',
                  )}
                </p>
                <div className='ct-lite-actions'>
                  <Link to='/console'>
                    <Button theme='solid' type='primary' className='ct-lite-primary'>
                      {t('立即使用')}
                    </Button>
                  </Link>
                  <Link to='/pricing'>
                    <Button theme='outline' type='primary' className='ct-lite-secondary'>
                      {t('查看价格')}
                    </Button>
                  </Link>
                </div>
                <div className='ct-lite-proof-row'>
                  {heroHighlights.map((item) => (
                    <div key={item.title}>
                      <span>{item.icon}</span>
                      <strong>{item.title}</strong>
                      <small>{item.desc}</small>
                    </div>
                  ))}
                </div>
              </div>

              <div className='ct-lite-hero-visual' aria-hidden='true'>
                <MetricTile label={t('平均延迟')} value={avgLatency} />
                <MetricTile label={t('成功率')} value={successRate} />
                <MetricTile label={t('健康渠道')} value={channelText} type='bars' />
                <div className='ct-lite-orb-stage'>
                  <div className='ct-lite-orb-ribbon' />
                  <div className='ct-lite-orb'>
                    <span />
                    <i />
                    <b />
                  </div>
                  <div className='ct-lite-platform'>
                    <span />
                    <i />
                    <b />
                  </div>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-lite-flow-section'>
            <div className='ct-lite-shell'>
              <div className='ct-lite-section-title'>
                <h2>{t('智能调度与失败切换流程')}</h2>
                <p>{t('实时评估、智能路由、自动切换，保障请求稳定、快速、可靠')}</p>
              </div>

              <div className='ct-lite-flow-panel'>
                <GatewayFlowCanvas
                  successRate={successRate}
                  avgLatency={avgLatency}
                  channelText={channelText}
                  channels={flowChannels}
                  locale={`${i18n.language}-${flowChannelKey}`}
                  t={t}
                />
              </div>
            </div>
          </section>

          <section className='ct-lite-features-section'>
            <div className='ct-lite-shell'>
              <div className='ct-lite-section-title'>
                <h2>{t('为稳定而生的智能网关')}</h2>
                <p>{t('四大能力，全面提升接入体验与服务稳定性')}</p>
              </div>

              <div className='ct-lite-feature-grid'>
                {featureCards.map((item) => (
                  <article className='ct-lite-feature-card' key={item.title}>
                    <span>{item.icon}</span>
                    <h3>{item.title}</h3>
                    <p>{item.desc}</p>
                  </article>
                ))}
              </div>

              <div className='ct-lite-pricing-strip'>
                <div>
                  <span>
                    <FileSearch size={34} />
                  </span>
                  <div>
                    <strong>{t('按模型 / 分组 / 倍率计费，缓存与图片明细透明')}</strong>
                    <p>
                      {t(
                        '缓存命中、图片处理、工具调用等费用清晰可见，账单可追溯，成本尽在掌握。',
                      )}
                    </p>
                  </div>
                </div>
                <Link to='/pricing'>
                  <Button theme='outline' type='primary' className='ct-lite-secondary'>
                    {t('查看模型价格')}
                  </Button>
                </Link>
              </div>

              <div className='ct-lite-dynamic-pricing'>
                <div className='ct-lite-dynamic-copy'>
                  <span>{t('动态价格')}</span>
                  <h3>{t('价格根据模型、分组倍率、缓存和图片用量实时计算')}</h3>
                  <p>
                    {t(
                      '同一次请求会拆分输入、输出、缓存读取和图片生成等计费项，命中动态规则时按实际档位结算。',
                    )}
                  </p>
                  <div className='ct-lite-dynamic-rules'>
                    {dynamicPriceRules.map((item) => (
                      <em key={item}>{item}</em>
                    ))}
                  </div>
                </div>

                <div className='ct-lite-dynamic-board'>
                  {dynamicPriceItems.map((item) => (
                    <div className='ct-lite-dynamic-row' key={item.label}>
                      <span>{item.label}</span>
                      <strong>{item.value}</strong>
                      <small>{item.detail}</small>
                    </div>
                  ))}
                  <Link to='/pricing' className='ct-lite-dynamic-link'>
                    {t('查看动态价格')}
                  </Link>
                </div>
              </div>
            </div>
          </section>

          <section className='ct-lite-plans-section'>
            <div className='ct-lite-shell'>
              <div className='ct-lite-section-title'>
                <h2>{t('选择适合你的套餐')}</h2>
                <p>{t('所有套餐均按量计费，可随时升级或取消')}</p>
              </div>

              <div className='ct-lite-plan-grid'>
                {planCards.map((plan) => (
                  <article
                    className={`ct-lite-plan-card${plan.featured ? ' featured' : ''}`}
                    key={plan.name}
                  >
                    {plan.featured && <div className='ct-lite-plan-badge'>{t('最受欢迎')}</div>}
                    <h3>{plan.name}</h3>
                    <p>{plan.subtitle}</p>
                    <div className='ct-lite-price'>
                      <strong>{plan.price}</strong>
                      <span>/ {t('月')}</span>
                    </div>
                    <ul>
                      {plan.perks.map((perk) => (
                        <li key={perk}>
                          <Check size={14} />
                          {perk}
                        </li>
                      ))}
                    </ul>
                    <Link to='/console/subscription-plans'>
                      <Button
                        theme={plan.featured ? 'solid' : 'outline'}
                        type='primary'
                        className={plan.featured ? 'ct-lite-primary' : 'ct-lite-secondary'}
                        block
                      >
                        {t('购买套餐')}
                      </Button>
                    </Link>
                  </article>
                ))}
              </div>

              <div className='ct-lite-trust-row'>
                {bottomTrust.map((item) => (
                  <div key={item.title}>
                    <span>{item.icon}</span>
                    <strong>{item.title}</strong>
                    <small>{item.desc}</small>
                  </div>
                ))}
              </div>
            </div>
          </section>
        </main>
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
