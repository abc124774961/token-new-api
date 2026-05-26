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
import { API, getCurrencyConfig, renderQuota } from '../../helpers';
import { StatusContext } from '../../context/Status';
import { useActualTheme } from '../../context/Theme';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import NoticeModal from '../../components/layout/NoticeModal';
import {
  formatSubscriptionDuration,
  formatSubscriptionResetPeriod,
} from '../../helpers/subscriptionFormat';

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

const formatPlanPrice = (amount) => {
  const { symbol, rate } = getCurrencyConfig();
  const number = Number(amount || 0) * rate;
  const digits = Number.isInteger(number) ? 0 : 2;
  return `${symbol} ${number.toFixed(digits)}`;
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
    const baseHeight = 660;
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
      violet: '#6366f1',
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

    const lerp = (start, end, amount) => start + (end - start) * amount;

    const cubicPoint = (start, cp1, cp2, end, progress) => {
      const t = Math.min(Math.max(progress, 0), 1);
      const mt = 1 - t;
      return {
        x:
          mt * mt * mt * start.x +
          3 * mt * mt * t * cp1.x +
          3 * mt * t * t * cp2.x +
          t * t * t * end.x,
        y:
          mt * mt * mt * start.y +
          3 * mt * mt * t * cp1.y +
          3 * mt * t * t * cp2.y +
          t * t * t * end.y,
      };
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
      drawCard(x, y, 138, 94, {
        radius: 12,
        fill: 'rgba(255,255,255,0.9)',
      });
      fillText(label, x + 18, y + 28, {
        size: 12,
        weight: 850,
        fill: '#718197',
      });
      fillText(value, x + 18, y + 61, {
        size: 22,
        weight: 950,
        fill: color.teal,
        family: 'mono',
      });

      if (type === 'bars') {
        const bars = [15, 22, 27, 18, 31, 24, 34, 28, 36];
        bars.forEach((bar, index) => {
          const bx = x + 20 + index * 8;
          const by = y + 82 - bar;
          drawRoundRect(ctx, bx, by, 5, bar, 3);
          ctx.fillStyle = color.teal;
          ctx.fill();
        });
        return;
      }

      ctx.save();
      ctx.beginPath();
      const points = [
        [20, 78],
        [33, 75],
        [45, 79],
        [58, 70],
        [70, 77],
        [84, 68],
        [101, 75],
        [119, 68],
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

    const drawHeaderMetric = (x, y, label, value, tone = color.teal) => {
      drawRoundRect(ctx, x, y, 124, 46, 12);
      ctx.fillStyle = 'rgba(255,255,255,0.78)';
      ctx.fill();
      ctx.strokeStyle = 'rgba(15,23,42,0.07)';
      ctx.lineWidth = 1;
      ctx.stroke();
      fillText(label, x + 14, y + 17, {
        size: 10,
        weight: 900,
        fill: color.faint,
      });
      fillText(value, x + 14, y + 37, {
        size: 15,
        weight: 950,
        fill: tone,
        family: 'mono',
      });
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
        height: pillHeight = 36,
        size = 12,
      } = options;
      setFont(size, 900);
      const w = pillWidth || Math.max(82, ctx.measureText(text).width + 28);
      const h = pillHeight;
      drawRoundRect(ctx, x, y, w, h, 9);
      ctx.fillStyle = fill;
      ctx.fill();
      ctx.strokeStyle = stroke;
      ctx.lineWidth = 1;
      ctx.stroke();
      fillText(text, x + w / 2, y + h / 2 + size * 0.36, {
        size,
        weight: 900,
        fill: textColor,
        align: 'center',
      });
    };

    const drawBezierLane = ({
      start,
      cp1,
      cp2,
      end,
      progress,
      stroke,
      glow,
      width: lineWidth = 2,
      dash = [],
      packets = 1,
      active = true,
    }) => {
      ctx.save();
      ctx.beginPath();
      ctx.moveTo(start.x, start.y);
      ctx.bezierCurveTo(cp1.x, cp1.y, cp2.x, cp2.y, end.x, end.y);
      ctx.strokeStyle = stroke;
      ctx.lineWidth = lineWidth;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      ctx.setLineDash(dash);
      ctx.stroke();
      ctx.setLineDash([]);

      if (active) {
        Array.from({ length: packets }).forEach((_, index) => {
          const packetProgress = (progress + index * 0.36) % 1;
          const point = cubicPoint(start, cp1, cp2, end, packetProgress);
          const tail = cubicPoint(
            start,
            cp1,
            cp2,
            end,
            Math.max(0, packetProgress - 0.055),
          );
          const tailGradient = ctx.createLinearGradient(tail.x, tail.y, point.x, point.y);
          tailGradient.addColorStop(0, 'rgba(255,255,255,0)');
          tailGradient.addColorStop(1, glow);
          ctx.beginPath();
          ctx.moveTo(tail.x, tail.y);
          ctx.lineTo(point.x, point.y);
          ctx.strokeStyle = tailGradient;
          ctx.lineWidth = lineWidth + 5;
          ctx.stroke();

          const halo = ctx.createRadialGradient(point.x, point.y, 1, point.x, point.y, 18);
          halo.addColorStop(0, glow);
          halo.addColorStop(1, 'rgba(255,255,255,0)');
          ctx.fillStyle = halo;
          ctx.beginPath();
          ctx.arc(point.x, point.y, 18, 0, Math.PI * 2);
          ctx.fill();
          ctx.fillStyle = glow;
          ctx.beginPath();
          ctx.arc(point.x, point.y, 4.2, 0, Math.PI * 2);
          ctx.fill();
        });
      }
      ctx.restore();
    };

    const drawIncidentPill = (x, y, time) => {
      const pulse = prefersReducedMotion ? 0.5 : (Math.sin(time * 3.2) + 1) / 2;
      drawRoundRect(ctx, x, y, 264, 44, 14);
      ctx.fillStyle = `rgba(254,242,242,${0.76 + pulse * 0.12})`;
      ctx.fill();
      ctx.strokeStyle = `rgba(239,68,68,${0.14 + pulse * 0.08})`;
      ctx.lineWidth = 1;
      ctx.stroke();

      ctx.fillStyle = `rgba(239,68,68,${0.14 + pulse * 0.12})`;
      ctx.beginPath();
      ctx.arc(x + 22, y + 22, 11, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = color.red;
      ctx.beginPath();
      ctx.arc(x + 22, y + 22, 4.8, 0, Math.PI * 2);
      ctx.fill();

      fillText(t('故障接管场景'), x + 42, y + 18, {
        size: 10,
        weight: 950,
        fill: color.red,
      });
      fillText(t('上游 502 已检测'), x + 42, y + 36, {
        size: 12,
        weight: 930,
        fill: color.ink,
      });
      drawPill(x + 154, y + 9, t('用户无感知恢复'), {
        fill: 'rgba(255,255,255,0.78)',
        stroke: 'rgba(239,68,68,0.12)',
        textColor: color.green,
        width: 98,
        height: 26,
        size: 9,
      });
    };

    const drawTopBar = (time) => {
      drawCard(38, 30, 1404, 84, {
        radius: 18,
        fill: 'rgba(255,255,255,0.82)',
        border: 'rgba(13,156,165,0.12)',
      });

      const pulse = prefersReducedMotion ? 0.55 : (Math.sin(time * 3) + 1) / 2;
      ctx.fillStyle = `rgba(22,163,74,${0.16 + pulse * 0.18})`;
      ctx.beginPath();
      ctx.arc(72, 72, 13, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = color.green;
      ctx.beginPath();
      ctx.arc(72, 72, 5, 0, Math.PI * 2);
      ctx.fill();

      fillText(t('LIVE ROUTING'), 96, 64, {
        size: 11,
        weight: 950,
        fill: color.tealDark,
        family: 'mono',
      });
      fillText(t('实时路由'), 96, 88, {
        size: 17,
        weight: 950,
      });

      fillText(t('请求编号'), 244, 64, {
        size: 10,
        weight: 900,
        fill: color.faint,
      });
      fillText('#RQ-82A7', 244, 88, {
        size: 15,
        weight: 950,
        fill: color.ink,
        family: 'mono',
      });

      fillText(t('当前链路'), 374, 64, {
        size: 10,
        weight: 900,
        fill: color.faint,
      });
      fillText('Codex -> Gateway -> #7 -> Stream', 374, 88, {
        size: 14,
        weight: 900,
        fill: '#334155',
        family: 'mono',
      });

      drawIncidentPill(686, 50, time);
      drawHeaderMetric(986, 49, t('成功率'), successRate, color.green);
      drawHeaderMetric(1124, 49, t('平均延迟'), avgLatency, color.blue);
      drawHeaderMetric(1262, 49, t('健康渠道'), channelText, color.teal);
    };

    const drawGatewayCore = (x, y, time) => {
      const pulse = prefersReducedMotion ? 0.5 : (Math.sin(time * 2.2) + 1) / 2;
      const sweep = prefersReducedMotion ? 0.75 : time * 1.2;

      ctx.save();
      const glow = ctx.createRadialGradient(x, y, 20, x, y, 150);
      glow.addColorStop(0, 'rgba(35,199,207,0.28)');
      glow.addColorStop(0.45, 'rgba(35,199,207,0.09)');
      glow.addColorStop(1, 'rgba(35,199,207,0)');
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(x, y, 150, 0, Math.PI * 2);
      ctx.fill();

      for (let index = 0; index < 28; index += 1) {
        const angle = (Math.PI * 2 * index) / 28;
        const inner = 108 + (index % 3 === 0 ? 0 : 5);
        const outer = 118;
        ctx.beginPath();
        ctx.moveTo(x + Math.cos(angle) * inner, y + Math.sin(angle) * inner);
        ctx.lineTo(x + Math.cos(angle) * outer, y + Math.sin(angle) * outer);
        ctx.strokeStyle = index % 4 === 0 ? 'rgba(13,156,165,0.28)' : 'rgba(100,116,139,0.14)';
        ctx.lineWidth = 2;
        ctx.stroke();
      }

      const rings = [
        { radius: 102, color: 'rgba(13,156,165,0.18)', width: 1.5 },
        { radius: 78, color: 'rgba(59,130,246,0.18)', width: 1.5 },
        { radius: 54, color: 'rgba(22,163,74,0.18)', width: 1.5 },
      ];
      rings.forEach((ring) => {
        ctx.beginPath();
        ctx.arc(x, y, ring.radius, 0, Math.PI * 2);
        ctx.strokeStyle = ring.color;
        ctx.lineWidth = ring.width;
        ctx.stroke();
      });

      [
        { radius: 92, start: 0.16, size: 0.44, tone: color.teal },
        { radius: 70, start: 0.56, size: 0.36, tone: color.blue },
        { radius: 46, start: 0.88, size: 0.42, tone: color.green },
      ].forEach((arc, index) => {
        const startAngle = sweep * (index % 2 === 0 ? 1 : -1) + Math.PI * 2 * arc.start;
        ctx.beginPath();
        ctx.arc(x, y, arc.radius, startAngle, startAngle + Math.PI * arc.size);
        ctx.strokeStyle = arc.tone;
        ctx.lineWidth = index === 0 ? 6 : 4;
        ctx.lineCap = 'round';
        ctx.stroke();
      });

      const scanAngle = sweep + 0.8;
      const scanGradient = ctx.createLinearGradient(
        x,
        y,
        x + Math.cos(scanAngle) * 112,
        y + Math.sin(scanAngle) * 112,
      );
      scanGradient.addColorStop(0, 'rgba(35,199,207,0.34)');
      scanGradient.addColorStop(1, 'rgba(35,199,207,0)');
      ctx.beginPath();
      ctx.moveTo(x, y);
      ctx.lineTo(x + Math.cos(scanAngle - 0.24) * 112, y + Math.sin(scanAngle - 0.24) * 112);
      ctx.arc(x, y, 112, scanAngle - 0.24, scanAngle + 0.08);
      ctx.closePath();
      ctx.fillStyle = scanGradient;
      ctx.fill();

      const coreGradient = ctx.createRadialGradient(x - 18, y - 24, 8, x, y, 62);
      coreGradient.addColorStop(0, '#ffffff');
      coreGradient.addColorStop(0.46, '#ffffff');
      coreGradient.addColorStop(0.48, '#d7fbff');
      coreGradient.addColorStop(1, color.teal);
      ctx.fillStyle = coreGradient;
      ctx.beginPath();
      ctx.arc(x, y, 54 + pulse * 3, 0, Math.PI * 2);
      ctx.fill();
      ctx.strokeStyle = 'rgba(255,255,255,0.84)';
      ctx.lineWidth = 8;
      ctx.beginPath();
      ctx.arc(x, y, 30, 0, Math.PI * 2);
      ctx.stroke();

      fillText('Gateway', x, y - 8, {
        size: 15,
        weight: 950,
        fill: color.ink,
        align: 'center',
        family: 'mono',
      });
      fillText(`${t('评分')} 98.7`, x, y + 17, {
        size: 11,
        weight: 900,
        fill: color.tealDark,
        align: 'center',
        family: 'mono',
      });
      fillText(`${t('链路')} OK`, x, y + 38, {
        size: 10,
        weight: 900,
        fill: color.green,
        align: 'center',
        family: 'mono',
      });

      fillText(t('智能网关'), x, y + 126, {
        size: 15,
        weight: 950,
        align: 'center',
      });
      fillText(t('通道评分'), x, y + 149, {
        size: 12,
        weight: 840,
        fill: color.muted,
        align: 'center',
      });
      ctx.restore();
    };

    const drawChannel = (channel, x, y, index, time) => {
      const statusColor =
        channel.tone === 'failed'
          ? color.red
          : channel.tone === 'cooling'
            ? color.amber
            : color.green;
      const isSelected = Boolean(channel.selected);
      const isPrimary = isSelected || channel.tone === 'failed';
      const pulse = prefersReducedMotion ? 0.4 : (Math.sin(time * 2.8 + index) + 1) / 2;

      ctx.save();
      ctx.globalAlpha = isPrimary ? 1 : 0.44;

      if (isSelected || channel.tone === 'failed') {
        ctx.save();
        const halo = ctx.createLinearGradient(x - 10, y, x + 318, y + 88);
        if (isSelected) {
          halo.addColorStop(0, 'rgba(13,156,165,0.18)');
          halo.addColorStop(0.54, 'rgba(34,197,94,0.12)');
          halo.addColorStop(1, 'rgba(34,197,94,0)');
        } else {
          halo.addColorStop(0, 'rgba(239,68,68,0)');
          halo.addColorStop(0.52, `rgba(239,68,68,${0.1 + pulse * 0.08})`);
          halo.addColorStop(1, 'rgba(239,68,68,0)');
        }
        drawRoundRect(ctx, x - 10, y - 10, 320, 108, 18);
        ctx.fillStyle = halo;
        ctx.fill();
        ctx.restore();
      }

      drawCard(x, y, 300, 88, {
        radius: 12,
        fill:
          channel.tone === 'failed'
            ? 'rgba(255,247,247,0.95)'
            : isSelected
              ? 'rgba(240,253,250,0.98)'
              : 'rgba(255,255,255,0.93)',
        border: isSelected
          ? 'rgba(13,156,165,0.42)'
          : channel.tone === 'failed'
            ? 'rgba(239,68,68,0.26)'
            : 'rgba(15,23,42,0.08)',
      });

      if (isSelected) {
        ctx.save();
        ctx.beginPath();
        ctx.moveTo(x + 4, y + 8);
        ctx.lineTo(x + 4, y + 80);
        ctx.strokeStyle = color.teal;
        ctx.lineWidth = 3;
        ctx.lineCap = 'round';
        ctx.stroke();
        ctx.restore();
      }

      if (channel.tone === 'failed') {
        ctx.save();
        ctx.strokeStyle = `rgba(239,68,68,${0.2 + pulse * 0.22})`;
        ctx.lineWidth = 2;
        ctx.setLineDash([8, 7]);
        drawRoundRect(ctx, x + 4, y + 4, 292, 80, 10);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(x + 242, y + 17);
        ctx.lineTo(x + 308, y + 68);
        ctx.stroke();
        ctx.restore();
      }

      ctx.fillStyle = statusColor;
      ctx.beginPath();
      ctx.arc(x + 22, y + 24, 5.5, 0, Math.PI * 2);
      ctx.fill();
      fillText(channel.name, x + 40, y + 25, {
        size: 14,
        weight: 950,
      });
      if (channel.provider) {
        fillText(channel.provider, x + 40, y + 47, {
          size: 11,
          weight: 850,
          fill: color.muted,
        });
      }
      if (channel.meta) {
        fillText(channel.meta, x + 150, y + 47, {
          size: 10,
          weight: 850,
          fill: channel.tone === 'failed' ? color.red : color.faint,
          family: channel.tone === 'failed' ? 'mono' : undefined,
        });
      }
      drawPill(x + 214, y + 12, channel.badge || channel.status, {
        fill:
          channel.tone === 'failed'
            ? 'rgba(239,68,68,0.09)'
            : channel.tone === 'cooling'
              ? 'rgba(245,158,11,0.11)'
              : 'rgba(22,163,74,0.09)',
        stroke: 'rgba(15,23,42,0.06)',
        textColor: statusColor,
        width: 76,
        height: 26,
        size: 10,
      });

      const metricY = y + 80;
      [
        [t('评分'), channel.score, color.teal],
        [t('延迟'), channel.latency, color.blue],
        [t('成本'), channel.cost, color.green],
      ].forEach(([label, value, tone], metricIndex) => {
        const mx = x + 40 + metricIndex * 80;
        fillText(label, mx, metricY - 14, {
          size: 9,
          weight: 900,
          fill: color.faint,
        });
        fillText(value, mx + 36, metricY - 14, {
          size: 10,
          weight: 950,
          fill: tone,
          family: 'mono',
        });
        drawRoundRect(ctx, mx, metricY - 3, 64, 4, 3);
        ctx.fillStyle = 'rgba(15,23,42,0.06)';
        ctx.fill();
        drawRoundRect(ctx, mx, metricY - 3, lerp(22, 64, metricIndex === 0 ? 0.82 : 0.58), 4, 3);
        ctx.fillStyle = tone;
        ctx.fill();
      });
      ctx.restore();
    };

    const drawSwitchBadge = (x, y, time) => {
      const pulse = prefersReducedMotion ? 0.52 : (Math.sin(time * 3.4) + 1) / 2;
      ctx.save();
      drawRoundRect(ctx, x, y, 154, 50, 15);
      ctx.fillStyle = 'rgba(255,255,255,0.94)';
      ctx.fill();
      ctx.strokeStyle = `rgba(239,68,68,${0.2 + pulse * 0.16})`;
      ctx.lineWidth = 1.4;
      ctx.stroke();

      ctx.beginPath();
      ctx.arc(x + 18, y + 19, 5.6, 0, Math.PI * 2);
      ctx.fillStyle = color.red;
      ctx.fill();
      ctx.beginPath();
      ctx.moveTo(x + 32, y + 19);
      ctx.lineTo(x + 58, y + 19);
      ctx.lineTo(x + 51, y + 14);
      ctx.moveTo(x + 58, y + 19);
      ctx.lineTo(x + 51, y + 24);
      ctx.strokeStyle = `rgba(13,156,165,${0.66 + pulse * 0.22})`;
      ctx.lineWidth = 2.2;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      ctx.stroke();
      ctx.beginPath();
      ctx.arc(x + 75, y + 19, 5.6, 0, Math.PI * 2);
      ctx.fillStyle = color.green;
      ctx.fill();

      fillText(t('502 旁路'), x + 116, y + 18, {
        size: 9,
        weight: 950,
        fill: color.red,
        align: 'center',
      });
      fillText(t('#7 接管'), x + 116, y + 33, {
        size: 10,
        weight: 950,
        fill: color.green,
        align: 'center',
        family: 'mono',
      });
      fillText(`${t('切换耗时')} 186ms`, x + 77, y + 47, {
        size: 8,
        weight: 900,
        fill: color.muted,
        align: 'center',
        family: 'mono',
      });
      ctx.restore();
    };

    const drawHandoffBeam = (time) => {
      const phase = prefersReducedMotion ? 0.6 : (time * 0.42) % 1;
      ctx.save();
      const start = { x: 1308, y: 424 };
      const cp1 = { x: 1318, y: 424 };
      const cp2 = { x: 1320, y: 378 };
      const end = { x: 1332, y: 372 };
      ctx.beginPath();
      ctx.moveTo(start.x, start.y);
      ctx.bezierCurveTo(cp1.x, cp1.y, cp2.x, cp2.y, end.x, end.y);
      ctx.strokeStyle = 'rgba(22,163,74,0.14)';
      ctx.lineWidth = 3.4;
      ctx.lineCap = 'round';
      ctx.stroke();
      ctx.beginPath();
      ctx.moveTo(start.x, start.y);
      ctx.bezierCurveTo(cp1.x, cp1.y, cp2.x, cp2.y, end.x, end.y);
      ctx.strokeStyle = 'rgba(22,163,74,0.66)';
      ctx.lineWidth = 1.6;
      ctx.stroke();

      const point = cubicPoint(start, cp1, cp2, end, phase);
      const halo = ctx.createRadialGradient(point.x, point.y, 1, point.x, point.y, 14);
      halo.addColorStop(0, 'rgba(22,163,74,0.66)');
      halo.addColorStop(1, 'rgba(22,163,74,0)');
      ctx.fillStyle = halo;
      ctx.beginPath();
      ctx.arc(point.x, point.y, 14, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = color.green;
      ctx.beginPath();
      ctx.arc(point.x, point.y, 3.2, 0, Math.PI * 2);
      ctx.fill();

      ctx.restore();
    };

    const drawDecisionTrace = (x, y, time) => {
      drawCard(x, y, 176, 370, {
        radius: 14,
        fill: 'rgba(255,255,255,0.88)',
        border: 'rgba(13,156,165,0.12)',
      });
      fillText(t('决策轨迹'), x + 18, y + 32, {
        size: 14,
        weight: 950,
      });
      fillText(t('实时评估'), x + 18, y + 53, {
        size: 11,
        weight: 850,
        fill: color.muted,
      });

      const steps = [
        { label: t('能力匹配'), code: 'PASS', tone: color.teal },
        { label: t('健康评分'), code: '98.7', tone: color.blue },
        { label: t('规避 502'), code: 'BYPASS', tone: color.red },
        { label: t('接管 #7'), code: '#7', tone: color.green },
        { label: t('SSE 保持'), code: 'ALIVE', tone: color.green },
      ];

      ctx.beginPath();
      ctx.moveTo(x + 31, y + 84);
      ctx.lineTo(x + 31, y + 314);
      ctx.strokeStyle = 'rgba(13,156,165,0.13)';
      ctx.lineWidth = 2;
      ctx.stroke();

      steps.forEach((step, index) => {
        const stepY = y + 88 + index * 54;
        const pulse = prefersReducedMotion ? 0.45 : (Math.sin(time * 2.6 + index * 0.8) + 1) / 2;
        ctx.fillStyle = `rgba(13,156,165,${0.08 + pulse * 0.08})`;
        ctx.beginPath();
        ctx.arc(x + 31, stepY, 13, 0, Math.PI * 2);
        ctx.fill();
        ctx.fillStyle = step.tone;
        ctx.beginPath();
        ctx.arc(x + 31, stepY, 5.2, 0, Math.PI * 2);
        ctx.fill();
        fillText(`0${index + 1}`, x + 54, stepY - 5, {
          size: 10,
          weight: 950,
          fill: color.faint,
          family: 'mono',
        });
        fillText(step.label, x + 54, stepY + 14, {
          size: 12,
          weight: 930,
          fill: color.ink,
        });
        fillText(step.code, x + 138, stepY + 6, {
          size: step.code.length > 6 ? 8 : 10,
          weight: 950,
          fill: step.tone,
          align: 'center',
          family: 'mono',
        });
      });
    };

    const drawOperationMetrics = (time) => {
      drawCard(48, 128, 468, 72, {
        radius: 14,
        fill: 'rgba(255,255,255,0.84)',
        shadow: false,
      });
      fillText(t('实时摘要'), 66, 152, {
        size: 10,
        weight: 950,
        fill: color.tealDark,
        family: 'mono',
      });
      fillText(t('故障接管与用量概览'), 66, 172, {
        size: 12,
        weight: 930,
        fill: color.ink,
      });

      const items = [
        {
          label: t('切换耗时'),
          value: '186ms',
          tone: color.green,
          soft: 'rgba(22,163,74,0.12)',
        },
        {
          label: t('失败旁路次数'),
          value: '12',
          tone: color.red,
          soft: 'rgba(239,68,68,0.12)',
        },
        {
          label: t('可用渠道池'),
          value: channelText,
          tone: color.teal,
          soft: 'rgba(13,156,165,0.12)',
        },
      ];

      items.forEach((item, index) => {
        const x = 250 + index * 104;
        if (index > 0) {
          ctx.beginPath();
          ctx.moveTo(x - 16, 142);
          ctx.lineTo(x - 16, 184);
          ctx.strokeStyle = 'rgba(15,23,42,0.06)';
          ctx.lineWidth = 1;
          ctx.stroke();
        }
        ctx.fillStyle = item.soft;
        ctx.beginPath();
        ctx.arc(x + 30, 150, 11, 0, Math.PI * 2);
        ctx.fill();
        fillText(item.label, x, 158, {
          size: 9,
          weight: 900,
          fill: color.faint,
        });
        fillText(item.value, x, 183, {
          size: 20,
          weight: 950,
          fill: item.tone,
          family: 'mono',
        });
      });
    };

    const drawEndpointCard = ({ x, y, title, subtitle, kind }) => {
      const cardWidth = kind === 'output' ? 138 : 146;
      const cardCenter = x + cardWidth / 2;
      if (kind === 'output') {
        ctx.save();
        const halo = ctx.createRadialGradient(cardCenter, y + 78, 8, cardCenter, y + 78, 118);
        halo.addColorStop(0, 'rgba(22,163,74,0.14)');
        halo.addColorStop(1, 'rgba(22,163,74,0)');
        ctx.fillStyle = halo;
        ctx.fillRect(x - 34, y - 34, 206, 230);
        ctx.restore();
      }
      const cardHeight = kind === 'output' ? 178 : 162;
      drawCard(x, y, cardWidth, cardHeight, {
        radius: 14,
        fill: kind === 'output' ? 'rgba(255,255,255,0.95)' : 'rgba(255,255,255,0.92)',
        border: kind === 'output' ? 'rgba(22,163,74,0.16)' : color.border,
      });
      if (kind === 'client') drawPulseIcon(cardCenter, y + 54);
      else drawMessageIcon(cardCenter, y + 50);

      fillText(title, cardCenter, y + 104, {
        size: 14,
        weight: 950,
        align: 'center',
      });
      fillText(subtitle, cardCenter, y + 127, {
        size: 12,
        weight: 820,
        fill: color.muted,
        align: 'center',
      });
      if (kind === 'output') {
        drawPill(x + 26, y + 140, t('SSE 保持'), {
          fill: 'rgba(34,197,94,0.1)',
          stroke: 'rgba(34,197,94,0.12)',
          textColor: color.green,
          width: 86,
          height: 28,
          size: 10,
        });
      }
    };

    const drawPolicyRail = (time) => {
      const items = [
        t('能力匹配'),
        t('成本权重'),
        t('健康评分'),
        t('失败率降权'),
        t('熔断冷却'),
      ];
      fillText(t('策略参与'), 250, 594, {
        size: 11,
        weight: 900,
        fill: color.faint,
      });
      ctx.save();
      ctx.beginPath();
      ctx.moveTo(250, 623);
      ctx.lineTo(768, 623);
      ctx.strokeStyle = 'rgba(13,156,165,0.12)';
      ctx.lineWidth = 2;
      ctx.stroke();
      ctx.restore();
      items.forEach((item, index) => {
        const x = 250 + index * 108;
        const active = prefersReducedMotion ? index === 2 : Math.floor(time * 1.4) % items.length === index;
        drawPill(x, 606, item, {
          fill: active ? 'rgba(13,156,165,0.11)' : 'rgba(255,255,255,0.82)',
          stroke: active ? 'rgba(13,156,165,0.24)' : 'rgba(15,23,42,0.08)',
          textColor: active ? color.tealDark : '#5c6d83',
          width: 96,
          height: 34,
          size: 11,
        });
        ctx.fillStyle = active ? color.teal : 'rgba(100,116,139,0.26)';
        ctx.beginPath();
        ctx.arc(x + 13, 623, 3.2, 0, Math.PI * 2);
        ctx.fill();
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

      for (let x = 0; x <= baseWidth; x += 40) {
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, baseHeight);
        ctx.strokeStyle = 'rgba(13, 156, 165, 0.045)';
        ctx.lineWidth = 1;
        ctx.stroke();
      }
      for (let y = 0; y <= baseHeight; y += 40) {
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(baseWidth, y);
        ctx.strokeStyle = 'rgba(13, 156, 165, 0.045)';
        ctx.lineWidth = 1;
        ctx.stroke();
      }

      const glow = ctx.createRadialGradient(476, 374, 40, 476, 374, 360);
      glow.addColorStop(0, 'rgba(35, 199, 207, 0.26)');
      glow.addColorStop(0.34, 'rgba(35, 199, 207, 0.08)');
      glow.addColorStop(1, 'rgba(35, 199, 207, 0)');
      ctx.fillStyle = glow;
      ctx.fillRect(96, 92, 804, 530);

      const phase = prefersReducedMotion ? 0.58 : (time * 0.22) % 1;
      drawBezierLane({
        start: { x: 186, y: 388 },
        cp1: { x: 264, y: 366 },
        cp2: { x: 334, y: 368 },
        end: { x: 378, y: 374 },
        progress: phase,
        stroke: 'rgba(59,130,246,0.42)',
        glow: 'rgba(59,130,246,0.95)',
        width: 3,
        packets: 2,
      });

      const channelY = channels.map((_, index) => 132 + index * 88 + 44);
      channelY.forEach((targetY, index) => {
        const tone = channels[index]?.tone;
        const selected = Boolean(channels[index]?.selected);
        drawBezierLane({
          start: { x: 558, y: 374 },
          cp1: { x: 620, y: 304 + index * 8 },
          cp2: { x: 664, y: targetY },
          end: { x: 730, y: targetY },
          progress: (phase + index * 0.16) % 1,
          stroke:
            tone === 'failed'
              ? 'rgba(239,68,68,0.3)'
              : tone === 'cooling'
                ? 'rgba(245,158,11,0.28)'
                : selected
                  ? 'rgba(22,163,74,0.5)'
                  : 'rgba(13,156,165,0.18)',
          glow:
            tone === 'failed'
              ? 'rgba(239,68,68,0.88)'
              : tone === 'cooling'
                ? 'rgba(245,158,11,0.82)'
                : selected
                  ? 'rgba(22,163,74,0.9)'
                  : 'rgba(35,199,207,0.66)',
          width: selected ? 3.2 : tone === 'failed' ? 2 : 2.2,
          dash: tone === 'failed' ? [8, 8] : [],
          packets: selected ? 3 : tone === 'failed' ? 1 : 1,
          active: selected || tone === 'failed' || index === 0,
        });
      });

      drawBezierLane({
        start: { x: 724, y: channelY[2] || 360 },
        cp1: { x: 656, y: 376 },
        cp2: { x: 662, y: 450 },
        end: { x: 724, y: channelY[3] || 446 },
        progress: (phase + 0.34) % 1,
        stroke: 'rgba(239,68,68,0.34)',
        glow: 'rgba(239,68,68,0.9)',
        width: 2,
        dash: [8, 7],
        packets: 1,
      });

      const selectedIndex = Math.max(
        channels.findIndex((channel) => channel.selected),
        0,
      );
      const selectedY = channelY[selectedIndex] || 432;
      drawBezierLane({
        start: { x: 1030, y: selectedY },
        cp1: { x: 1074, y: selectedY + 4 },
        cp2: { x: 1090, y: 422 },
        end: { x: 1108, y: 422 },
        progress: (phase + 0.48) % 1,
        stroke: 'rgba(22,163,74,0.42)',
        glow: 'rgba(22,163,74,0.9)',
        width: 3,
        packets: 2,
      });

      drawTopBar(time);

      drawOperationMetrics(time);

      drawEndpointCard({
        x: 52,
        y: 314,
        title: t('客户端'),
        subtitle: t('请求接入'),
        kind: 'client',
      });

      drawGatewayCore(476, 374, time);

      channels.forEach((channel, index) => {
        drawChannel(channel, 734, 132 + index * 88, index, time);
      });

      drawSwitchBadge(574, 482, time);
      drawDecisionTrace(1128, 150, time);
      drawHandoffBeam(time);

      drawEndpointCard({
        x: 1330,
        y: 286,
        title: t('稳定输出'),
        subtitle: t('流式返回'),
        kind: 'output',
      });

      drawPolicyRail(time);

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
  const [subscriptionPlans, setSubscriptionPlans] = useState([]);

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
        meta: t('候选备用'),
      status: t('健康'),
      tone: 'healthy',
      badge: t('READY'),
      score: '94.8',
      latency: '312ms',
      cost: '1.0x',
    },
      {
        name: 'Channel #2',
        provider: 'Anthropic',
        meta: t('候选备用'),
      status: t('健康'),
      tone: 'healthy',
      badge: t('READY'),
      score: '92.3',
      latency: '421ms',
      cost: '0.8x',
    },
      {
        name: 'Channel #4',
        provider: '',
        meta: 'HTTP 502',
      status: t('失败'),
      tone: 'failed',
      badge: t('502 BYPASS'),
      score: '41.2',
      latency: '502',
      cost: '--',
    },
      {
        name: 'Channel #7',
        provider: 'Google Gemini',
        meta: t('接管中'),
      status: t('健康'),
      tone: 'healthy',
      badge: t('SELECTED'),
      score: '98.7',
      latency: '289ms',
      cost: '0.9x',
      selected: true,
    },
      {
        name: 'Channel #9',
        provider: 'DeepSeek',
        meta: t('熔断冷却 60s'),
      status: t('冷却中'),
      tone: 'cooling',
      badge: t('COOLING'),
      score: '66.4',
      latency: '60s',
      cost: '0.6x',
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

  const realPlanCards = subscriptionPlans
    .map((item) => item?.plan)
    .filter((plan) => plan?.id)
    .slice(0, 3)
    .map((plan, index, list) => {
      const totalAmount = Number(plan?.total_amount || 0);
      const resetLabel = formatSubscriptionResetPeriod(plan, t);
      const durationLabel = formatSubscriptionDuration(plan, t);
      const limit = Number(plan?.max_purchase_per_user || 0);
      const isResetting = resetLabel && resetLabel !== t('不重置');
      const perks = [
        totalAmount > 0
          ? `${renderQuota(totalAmount)} / ${isResetting ? resetLabel : durationLabel}`
          : t('不限额度'),
        `${t('有效期')} ${durationLabel}`,
        isResetting ? `${t('额度重置')} ${resetLabel}` : null,
        plan.upgrade_group ? `${t('升级分组')} ${plan.upgrade_group}` : t('API 全模型调用'),
        limit > 0 ? `${t('限购')} ${limit}` : t('购买套餐后即可享受模型权益'),
      ].filter(Boolean);

      return {
        name: plan.title || t('订阅套餐'),
        subtitle: plan.subtitle || t('购买套餐后即可享受模型权益'),
        price: formatPlanPrice(plan.price_amount),
        period: durationLabel,
        featured: list.length > 1 ? index === 1 : index === 0,
        perks,
      };
    });
  const planCards = realPlanCards;

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

  const loadSubscriptionPlans = async () => {
    const endpoints = ['/api/public/subscription/plans', '/api/subscription/plans'];
    for (const endpoint of endpoints) {
      try {
        const res = await API.get(endpoint, {
          skipErrorHandler: true,
        });
        const { success, data } = res.data || {};
        if (success && Array.isArray(data)) {
          setSubscriptionPlans(data);
          return;
        }
      } catch (error) {
        // Keep trying the next compatible endpoint so old dev backends still work.
      }
    }
    setSubscriptionPlans([]);
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
    loadSubscriptionPlans().then();
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
                <h2>{t('请求失败自动旁路，流式响应稳定返回')}</h2>
                <p>
                  {t(
                    '上游 502、配额异常、超时抖动时，网关自动完成评分、熔断、切换与恢复。',
                  )}
                </p>
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
              <p className='ct-lite-flow-note'>
                {t('适合 Codex、Claude Code、OpenAI SDK 等高频长上下文请求。')}
              </p>
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

              {planCards.length > 0 ? (
                <div className={`ct-lite-plan-grid ct-lite-plan-grid-count-${planCards.length}`}>
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
                        <span>/ {plan.period || t('月')}</span>
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
              ) : (
                <div className='ct-lite-plan-empty'>
                  <strong>{t('暂无可购买套餐')}</strong>
                  <span>{t('请联系管理员配置套餐')}</span>
                  <Link to='/console/subscription-plans'>{t('查看套餐')}</Link>
                </div>
              )}

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
