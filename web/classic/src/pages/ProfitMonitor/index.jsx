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

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Empty,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Spin,
  Switch,
  Table,
  Tabs,
  Tag,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  AlertTriangle,
  BarChart3,
  Copy,
  DollarSign,
  Eye,
  Network,
  PackageCheck,
  Plus,
  RefreshCw,
  Save,
  Settings,
  Server,
  Sparkles,
  Trash2,
  TrendingUp,
} from 'lucide-react';
import {
  API,
  copy,
  showError,
  showSuccess,
  timestamp2string,
} from '../../helpers';
import './profit-monitor.css';

const { Text } = Typography;

const WINDOW_OPTIONS = ['24h', '7d', '30d'];
const DIMENSION_OPTIONS = ['group', 'channel', 'model', 'proxy'];

const RESOURCE_TYPES = [
  'account_pool',
  'proxy',
  'server',
  'bandwidth',
  'other',
];

const SCOPE_TYPES = ['global', 'channel', 'group', 'model'];
const ALLOCATION_MODES = ['revenue', 'request', 'global'];
const DECISION_STATUS_OPTIONS = ['pending', 'canary', 'adopted', 'ignored'];
const CANARY_STATUS_OPTIONS = ['planned', 'running', 'completed', 'canceled'];
const CANARY_WATCH_METRIC_OPTIONS = [
  'gross_margin',
  'success_rate',
  'traffic_cost_usd',
  'resource_cost_usd',
  'revenue_gap_usd',
  'request_count',
  'upstream_cost_usd',
  'operating_cost_usd',
];

const DEFAULT_CONFIG = {
  enabled: true,
  server_daily_cost_usd: 0,
  traffic_cost_per_gb_usd: 0,
  traffic_estimation_enabled: false,
  traffic_estimated_bytes_per_token: 0,
  resource_cost_enabled: true,
  target_profit_rate: 0.2,
  dynamic_ratio_min_limit: 0,
  dynamic_ratio_max_limit: 0,
  dynamic_ratio_recommendation_mode: 'observe',
};

const DEFAULT_RESOURCE_FORM = {
  name: '',
  resource_type: 'account_pool',
  scope_type: 'global',
  scope_id: 0,
  scope_key: '',
  amount_usd: 0,
  period_seconds: 86400,
  amortize_start_at: 0,
  amortize_end_at: 0,
  loss_amount_usd: 0,
  loss_recorded_at: 0,
  allocation_mode: 'revenue',
  enabled: true,
  remark: '',
};

const DEFAULT_CANARY_FORM = {
  recommendation_id: 0,
  title: '',
  status: 'planned',
  scope_type: 'group',
  scope_id: 0,
  scope_key: '',
  baseline_revenue_multiplier: 0,
  planned_revenue_multiplier: 0,
  recommended_revenue_multiplier: 0,
  planned_start_at: 0,
  planned_end_at: 0,
  actual_start_at: 0,
  actual_end_at: 0,
  observation_window_seconds: 7200,
  watch_metrics: [
    'gross_margin',
    'success_rate',
    'traffic_cost_usd',
    'resource_cost_usd',
    'revenue_gap_usd',
    'request_count',
  ],
  result_summary: '',
};

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function numberOrDefault(value, fallback = 0) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : fallback;
}

function formatUsd(value, digits = 4) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  const sign = numeric < 0 ? '-' : '';
  const abs = Math.abs(numeric);
  return `${sign}$${abs.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatPercent(value, digits = 1) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `${(numeric * 100).toFixed(digits)}%`;
}

function formatMultiplier(value, digits = 2) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return `${numeric.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '')}x`;
}

function formatRatioNumber(value, digits = 4) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return numeric.toFixed(digits).replace(/0+$/, '').replace(/\.$/, '');
}

function formatRatioValue(value, digits = 4) {
  const text = formatRatioNumber(value, digits);
  return text === '--' ? '--' : `${text}x`;
}

function formatDynamicRatioLimitRange(recommendation, t) {
  const min = Number(recommendation?.dynamic_ratio_limit_min || 0);
  const max = Number(recommendation?.dynamic_ratio_limit_max || 0);
  const minText = formatRatioNumber(min);
  const maxText = formatRatioNumber(max);
  if (min > 0 && max > 0) {
    return t('{{min}}x - {{max}}x', { min: minText, max: maxText });
  }
  if (min > 0) {
    return t('下限 {{ratio}}x', { ratio: minText });
  }
  if (max > 0) {
    return t('上限 {{ratio}}x', { ratio: maxText });
  }
  return t('未额外限制');
}

function formatDynamicRatioLimitResult(recommendation, t) {
  if (!recommendation?.dynamic_ratio_limit_applied) {
    return t('未触发限制');
  }
  switch (recommendation?.dynamic_ratio_limit_reason) {
    case 'min_limit':
      return t('已按下限限制');
    case 'max_limit':
      return t('已按上限限制');
    default:
      return t('已应用倍率限制');
  }
}

function formatProfitMarkupFormula(targetRate, markupMultiplier) {
  const markupText = formatMultiplier(markupMultiplier);
  if (markupText === '--') return '--';
  const rateText = formatPercent(targetRate);
  if (rateText === '--') return markupText;
  return `1 / (1 - ${rateText}) = ${markupText}`;
}

function resolveCostMarkupMultiplier(source, fallbackSummary) {
  const direct = Number(source?.cost_markup_multiplier || 0);
  if (Number.isFinite(direct) && direct > 0) return direct;
  const requiredRevenue = Number(source?.required_revenue_usd || 0);
  const upstreamCost = Number(
    source?.upstream_cost_usd || fallbackSummary?.upstream_cost_usd || 0,
  );
  if (
    Number.isFinite(requiredRevenue) &&
    Number.isFinite(upstreamCost) &&
    requiredRevenue > 0 &&
    upstreamCost > 0
  ) {
    return requiredRevenue / upstreamCost;
  }
  return 0;
}

function resolveCostMultiplier(source) {
  const direct = Number(source?.cost_multiplier || 0);
  if (Number.isFinite(direct) && direct > 0) return direct;
  const suggestedRatio = Number(source?.suggested_dynamic_ratio || 0);
  const markup = resolveCostMarkupMultiplier(source);
  if (
    Number.isFinite(suggestedRatio) &&
    Number.isFinite(markup) &&
    suggestedRatio > 0 &&
    markup > 0
  ) {
    return suggestedRatio / markup;
  }
  return 0;
}

function formatShare(value) {
  return formatPercent(value, 1);
}

function formatNumber(value) {
  return new Intl.NumberFormat().format(Number(value) || 0);
}

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let cursor = bytes;
  let unitIndex = 0;
  while (cursor >= 1024 && unitIndex < units.length - 1) {
    cursor /= 1024;
    unitIndex += 1;
  }
  return `${cursor >= 10 || unitIndex === 0 ? cursor.toFixed(0) : cursor.toFixed(2)} ${units[unitIndex]}`;
}

function formatTimestamp(value) {
  const timestamp = Number(value || 0);
  return timestamp > 0 ? timestamp2string(timestamp) : '--';
}

function formatPeriod(seconds, t) {
  const total = Number(seconds || 0);
  if (total === 86400) return t('每天');
  if (total === 604800) return t('每周');
  if (total === 2592000) return t('每30天');
  if (total > 0) return t('{{seconds}}秒', { seconds: total });
  return '--';
}

function labelForResourceType(value, t) {
  const labels = {
    account_pool: t('账号池'),
    proxy: t('代理资源'),
    server: t('服务器'),
    bandwidth: t('流量带宽'),
    other: t('其他资源'),
  };
  return labels[value] || value || '--';
}

function labelForScope(value, t) {
  const labels = {
    global: t('全局'),
    channel: t('渠道'),
    group: t('分组'),
    model: t('模型'),
  };
  return labels[value] || value || '--';
}

function labelForAllocation(value, t) {
  const labels = {
    revenue: t('按收入分摊'),
    request: t('按请求数分摊'),
    global: t('全局成本'),
  };
  return labels[value] || value || '--';
}

function labelForDimension(value, t) {
  const labels = {
    group: t('按分组'),
    channel: t('按渠道'),
    model: t('按模型'),
    proxy: t('按代理'),
  };
  return labels[value] || value || '--';
}

function labelForRecommendationRisk(value, t) {
  const labels = {
    low: t('低风险'),
    medium: t('中风险'),
    high: t('高风险'),
    insufficient_data: t('样本不足'),
  };
  return labels[value] || value || '--';
}

function colorForRecommendationRisk(value) {
  if (value === 'high') return 'red';
  if (value === 'medium') return 'orange';
  if (value === 'low') return 'green';
  return 'grey';
}

function labelForRecommendationDecisionStatus(value, t) {
  const labels = {
    pending: t('待处理'),
    canary: t('灰度中'),
    adopted: t('已采纳'),
    ignored: t('已忽略'),
  };
  return labels[value] || labels.pending;
}

function colorForRecommendationDecisionStatus(value) {
  if (value === 'adopted') return 'green';
  if (value === 'canary') return 'blue';
  if (value === 'ignored') return 'grey';
  return 'orange';
}

function labelForCanaryStatus(value, t) {
  const labels = {
    planned: t('计划中'),
    running: t('执行中'),
    completed: t('已完成'),
    canceled: t('已取消'),
  };
  return labels[value] || labels.planned;
}

function colorForCanaryStatus(value) {
  if (value === 'running') return 'blue';
  if (value === 'completed') return 'green';
  if (value === 'canceled') return 'grey';
  return 'orange';
}

function labelForCanaryWatchMetric(value, t) {
  const labels = {
    gross_margin: t('毛利率'),
    success_rate: t('完成率'),
    traffic_cost_usd: t('流量成本'),
    resource_cost_usd: t('资源成本'),
    revenue_gap_usd: t('收入缺口'),
    request_count: t('请求数'),
    upstream_cost_usd: t('上游成本'),
    operating_cost_usd: t('总经营成本'),
  };
  return labels[value] || value || '--';
}

function normalizeWatchMetrics(values) {
  const items = Array.isArray(values)
    ? values
    : String(values || '')
        .split(/[\n,，]/)
        .map((item) => item.trim());
  const allowed = new Set(CANARY_WATCH_METRIC_OPTIONS);
  const seen = new Set();
  return items.filter((item) => {
    if (!allowed.has(item) || seen.has(item)) return false;
    seen.add(item);
    return true;
  });
}

function canaryDefaultWatchMetrics() {
  return [...DEFAULT_CANARY_FORM.watch_metrics];
}

function labelForRecommendationReason(value, t) {
  switch (value) {
    case 'recommendation_disabled':
      return t('动态倍率建议已关闭，仅保留快照。');
    case 'no_cost_data':
      return t('当前窗口缺少成本数据，请先补充上游、流量或资源成本。');
    case 'insufficient_revenue_data':
      return t('样本不足或收入数据缺失，建议先累积真实请求后再调整。');
    case 'insufficient_sample':
      return t(
        '样本不足：至少需要 20 次真实请求、5 次成功请求和 1000 token 后再生成可信建议。',
      );
    case 'target_covered':
      return t('当前毛利率已覆盖目标毛利率，建议保持观察。');
    case 'high_gap':
      return t(
        '当前毛利率明显低于目标，请优先检查成本来源，再逐步上调动态倍率。',
      );
    case 'traffic_estimated':
      return t(
        '当前毛利率低于目标，且流量成本仍未完全来自真实字节，建议补齐流量数据后小幅灰度调整。',
      );
    case 'below_target':
      return t('当前毛利率低于目标，建议小幅上调并观察请求量变化。');
    case 'ok':
      return t('建议已生成，后台动态倍率会按配置自动应用并保留记录。');
    default:
      return value ? t(value) : '--';
  }
}

function labelForRecommendationConstraint(value, t) {
  switch (value) {
    case 'snapshot_only':
      return t('仅保存快照，不会自动改写线上倍率。');
    case 'insufficient_data_no_direct_adjust':
      return t('数据不足时不能直接调整倍率。');
    case 'billing_expression_remains_source':
      return t('计费表达式仍是线上计费唯一依据。');
    default:
      return value ? t(value) : '--';
  }
}

function labelForRecommendationAction(value, t) {
  switch (value) {
    case 'gray_raise_dynamic_ratio':
      return t('参考建议收入倍率进行小流量灰度，不要一次性全量调整。');
    case 'keep_observing':
      return t('暂不需要上调动态倍率，继续观察真实请求利润。');
    case 'complete_real_traffic_data':
      return t('补齐真实入站和出站流量字节后重新生成建议。');
    case 'add_resource_cost_ledger':
      return t('如存在账号池采购、代理或封号损耗，请先录入资源成本台账。');
    case 'check_cost_anomalies':
      return t('先排查异常上游成本、失败请求和资源损耗，再调整倍率。');
    default:
      return value ? t(value) : '--';
  }
}

function parseJsonObject(value) {
  if (!value) return {};
  if (typeof value === 'object' && !Array.isArray(value)) return value;
  if (typeof value !== 'string') return {};
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
      ? parsed
      : {};
  } catch {
    return {};
  }
}

function stringifyJson(value) {
  return JSON.stringify(value || {}, null, 2);
}

function metricTone(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return 'neutral';
  if (numeric < 0) return 'danger';
  if (numeric > 0) return 'success';
  return 'neutral';
}

function MetricCard({ icon: Icon, label, value, detail, tone = 'neutral' }) {
  return (
    <div className={`ct-profit-metric ct-profit-metric-${tone}`}>
      <div>
        <div className='ct-profit-metric-label'>{label}</div>
        <div className='ct-profit-metric-value'>{value}</div>
        {detail && <div className='ct-profit-metric-detail'>{detail}</div>}
      </div>
      <div className='ct-profit-metric-icon'>
        <Icon size={18} />
      </div>
    </div>
  );
}

function ConfigModal({ visible, config, saving, onCancel, onSave, t }) {
  const [form, setForm] = useState(DEFAULT_CONFIG);

  useEffect(() => {
    if (visible) {
      setForm({ ...DEFAULT_CONFIG, ...(config || {}) });
    }
  }, [visible, config]);

  const update = (key, value) => setForm((prev) => ({ ...prev, [key]: value }));

  return (
    <Modal
      title={t('盈利监控设置')}
      visible={visible}
      onCancel={onCancel}
      onOk={() => onSave(form)}
      confirmLoading={saving}
      okText={t('保存设置')}
      cancelText={t('取消')}
      width={720}
      className='ct-profit-modal'
    >
      <div className='ct-profit-form-grid'>
        <label className='ct-profit-field'>
          <span>{t('启用盈利监控')}</span>
          <Switch
            checked={form.enabled !== false}
            onChange={(value) => update('enabled', value)}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('计入资源成本')}</span>
          <Switch
            checked={form.resource_cost_enabled !== false}
            onChange={(value) => update('resource_cost_enabled', value)}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('服务器每天平摊成本')}</span>
          <InputNumber
            min={0}
            step={0.01}
            prefix='$'
            value={form.server_daily_cost_usd}
            onChange={(value) =>
              update('server_daily_cost_usd', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('目标毛利率')}</span>
          <InputNumber
            min={0}
            max={95}
            step={0.01}
            suffix='%'
            value={Number(form.target_profit_rate || 0) * 100}
            onChange={(value) =>
              update('target_profit_rate', numberOrDefault(value) / 100)
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('建议倍率下限')}</span>
          <InputNumber
            min={0}
            max={100}
            step={0.0001}
            suffix='x'
            value={form.dynamic_ratio_min_limit}
            onChange={(value) =>
              update('dynamic_ratio_min_limit', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('建议倍率上限')}</span>
          <InputNumber
            min={0}
            max={100}
            step={0.0001}
            suffix='x'
            value={form.dynamic_ratio_max_limit}
            onChange={(value) =>
              update('dynamic_ratio_max_limit', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('流量每 GB 成本')}</span>
          <InputNumber
            min={0}
            step={0.01}
            prefix='$'
            value={form.traffic_cost_per_gb_usd}
            onChange={(value) =>
              update('traffic_cost_per_gb_usd', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('启用流量估算')}</span>
          <Switch
            checked={form.traffic_estimation_enabled === true}
            onChange={(value) => update('traffic_estimation_enabled', value)}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('每 token 估算字节')}</span>
          <InputNumber
            min={0}
            step={1}
            value={form.traffic_estimated_bytes_per_token}
            onChange={(value) =>
              update(
                'traffic_estimated_bytes_per_token',
                numberOrDefault(value),
              )
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('动态倍率建议模式')}</span>
          <Select
            value={form.dynamic_ratio_recommendation_mode}
            onChange={(value) =>
              update('dynamic_ratio_recommendation_mode', value)
            }
            optionList={[
              { label: t('关闭'), value: 'off' },
              { label: t('观察建议'), value: 'observe' },
              { label: t('AI建议'), value: 'ai' },
            ]}
          />
        </label>
      </div>
      <div className='ct-profit-help'>
        <AlertTriangle size={15} />
        <span>
          {t(
            '经营成本会进入后台动态倍率计算；启用动态计费后，请求链路只读取内存倍率快照。',
          )}{' '}
          {t('0 表示不额外限制，最终仍会受后台全局倍率上下限保护。')}
        </span>
      </div>
    </Modal>
  );
}

function ResourceModal({ visible, resource, saving, onCancel, onSave, t }) {
  const [form, setForm] = useState(DEFAULT_RESOURCE_FORM);

  useEffect(() => {
    if (visible) {
      setForm({ ...DEFAULT_RESOURCE_FORM, ...(resource || {}) });
    }
  }, [visible, resource]);

  const update = (key, value) => setForm((prev) => ({ ...prev, [key]: value }));

  return (
    <Modal
      title={resource?.id ? t('编辑资源成本') : t('新增资源成本')}
      visible={visible}
      onCancel={onCancel}
      onOk={() => onSave(form)}
      confirmLoading={saving}
      okText={t('保存')}
      cancelText={t('取消')}
      width={760}
      className='ct-profit-modal'
    >
      <div className='ct-profit-form-grid'>
        <label className='ct-profit-field ct-profit-field-wide'>
          <span>{t('资源名称')}</span>
          <Input
            value={form.name}
            placeholder={t('例如：Codex 账号池 5 月采购')}
            onChange={(value) => update('name', value)}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('资源类型')}</span>
          <Select
            value={form.resource_type}
            onChange={(value) => update('resource_type', value)}
            optionList={RESOURCE_TYPES.map((value) => ({
              label: labelForResourceType(value, t),
              value,
            }))}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('启用')}</span>
          <Switch
            checked={form.enabled !== false}
            onChange={(value) => update('enabled', value)}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('成本金额')}</span>
          <InputNumber
            min={0}
            step={0.01}
            prefix='$'
            value={form.amount_usd}
            onChange={(value) => update('amount_usd', numberOrDefault(value))}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('摊销周期秒数')}</span>
          <InputNumber
            min={1}
            step={3600}
            value={form.period_seconds}
            onChange={(value) =>
              update('period_seconds', numberOrDefault(value, 86400))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('摊销开始时间戳')}</span>
          <InputNumber
            min={0}
            step={60}
            value={form.amortize_start_at}
            onChange={(value) =>
              update('amortize_start_at', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('摊销结束时间戳')}</span>
          <InputNumber
            min={0}
            step={60}
            value={form.amortize_end_at}
            onChange={(value) =>
              update('amortize_end_at', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('损耗金额')}</span>
          <InputNumber
            min={0}
            step={0.01}
            prefix='$'
            value={form.loss_amount_usd}
            onChange={(value) =>
              update('loss_amount_usd', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('损耗记录时间戳')}</span>
          <InputNumber
            min={0}
            step={60}
            value={form.loss_recorded_at}
            onChange={(value) =>
              update('loss_recorded_at', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('作用范围')}</span>
          <Select
            value={form.scope_type}
            onChange={(value) => update('scope_type', value)}
            optionList={SCOPE_TYPES.map((value) => ({
              label: labelForScope(value, t),
              value,
            }))}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('范围 ID')}</span>
          <InputNumber
            min={0}
            value={form.scope_id}
            onChange={(value) => update('scope_id', numberOrDefault(value))}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('范围标识')}</span>
          <Input
            value={form.scope_key}
            placeholder={t('分组名或模型名，可留空')}
            onChange={(value) => update('scope_key', value)}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('分摊方式')}</span>
          <Select
            value={form.allocation_mode}
            onChange={(value) => update('allocation_mode', value)}
            optionList={ALLOCATION_MODES.map((value) => ({
              label: labelForAllocation(value, t),
              value,
            }))}
          />
        </label>
        <label className='ct-profit-field ct-profit-field-wide'>
          <span>{t('备注')}</span>
          <Input
            value={form.remark}
            placeholder={t('采购批次、供应商、账号损耗原因等')}
            onChange={(value) => update('remark', value)}
          />
        </label>
      </div>
    </Modal>
  );
}

function RecommendationDetailModal({
  visible,
  snapshot,
  decisionSaving,
  onCancel,
  onCopy,
  onCreateCanaryTask,
  onDecisionSave,
  t,
}) {
  const safeSnapshot = snapshot || {};
  const [decisionStatus, setDecisionStatus] = useState('pending');
  const [plannedRevenueMultiplier, setPlannedRevenueMultiplier] = useState(0);
  const [decisionRemark, setDecisionRemark] = useState('');
  const inputPayload = useMemo(
    () => parseJsonObject(safeSnapshot.input_json),
    [safeSnapshot.input_json],
  );
  const recommendationPayload = useMemo(
    () => parseJsonObject(safeSnapshot.recommendation_json),
    [safeSnapshot.recommendation_json],
  );
  const constraintCodes = Array.isArray(recommendationPayload.constraint_codes)
    ? recommendationPayload.constraint_codes
    : [];
  const actionCodes = Array.isArray(
    recommendationPayload.suggested_action_codes,
  )
    ? recommendationPayload.suggested_action_codes
    : [];
  const inputText = stringifyJson(inputPayload);
  const recommendationText = stringifyJson(recommendationPayload);

  useEffect(() => {
    if (!visible) return;
    const status = safeSnapshot.decision_status || 'pending';
    const planned = Number(safeSnapshot.planned_revenue_multiplier || 0);
    const recommended = Number(
      safeSnapshot.recommended_revenue_multiplier || 0,
    );
    setDecisionStatus(status);
    setPlannedRevenueMultiplier(
      planned > 0
        ? planned
        : recommended > 0
          ? Number(recommended.toFixed(2))
          : 0,
    );
    setDecisionRemark(safeSnapshot.decision_remark || '');
  }, [
    visible,
    safeSnapshot.decision_remark,
    safeSnapshot.decision_status,
    safeSnapshot.planned_revenue_multiplier,
    safeSnapshot.recommended_revenue_multiplier,
  ]);

  const saveDecision = () => {
    onDecisionSave(safeSnapshot.id, {
      decision_status: decisionStatus,
      planned_revenue_multiplier: numberOrDefault(plannedRevenueMultiplier),
      decision_remark: decisionRemark,
    });
  };

  return (
    <Modal
      title={t('建议快照详情')}
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width={920}
      className='ct-profit-modal'
    >
      <div className='ct-profit-detail-grid'>
        <div>
          <span>{t('快照编号')}</span>
          <strong>#{safeSnapshot.id || '--'}</strong>
        </div>
        <div>
          <span>{t('生成窗口')}</span>
          <strong>
            {safeSnapshot.window || '--'} ·{' '}
            {labelForDimension(safeSnapshot.dimension, t)}
          </strong>
        </div>
        <div>
          <span>{t('当前毛利率')}</span>
          <strong>{formatPercent(safeSnapshot.current_margin)}</strong>
        </div>
        <div>
          <span>{t('成本倍率')}</span>
          <strong>{formatMultiplier(resolveCostMultiplier(safeSnapshot), 4)}</strong>
        </div>
        <div>
          <span>{t('目标毛利换算')}</span>
          <strong>
            {formatProfitMarkupFormula(
              safeSnapshot.target_profit_rate,
              resolveCostMarkupMultiplier(safeSnapshot),
            )}
          </strong>
        </div>
        <div>
          <span>{t('建议收入倍率')}</span>
          <strong>
            {Number(safeSnapshot.recommended_revenue_multiplier) > 0
              ? formatMultiplier(safeSnapshot.recommended_revenue_multiplier)
              : '--'}
          </strong>
        </div>
        <div>
          <span>{t('风险等级')}</span>
          <strong>
            {labelForRecommendationRisk(safeSnapshot.risk_level, t)}
          </strong>
        </div>
        <div>
          <span>{t('置信度')}</span>
          <strong>{formatPercent(safeSnapshot.confidence)}</strong>
        </div>
      </div>

      <div className='ct-profit-detail-section'>
        <h3>{t('建议摘要')}</h3>
        <p>{labelForRecommendationReason(safeSnapshot.reason, t)}</p>
      </div>

      <div className='ct-profit-detail-section'>
        <h3>{t('约束')}</h3>
        <div className='ct-profit-chip-list'>
          {constraintCodes.length > 0 ? (
            constraintCodes.map((code) => (
              <Tag key={code} color='teal' type='light'>
                {labelForRecommendationConstraint(code, t)}
              </Tag>
            ))
          ) : (
            <Tag type='light'>{t('暂无数据')}</Tag>
          )}
        </div>
      </div>

      <div className='ct-profit-detail-section'>
        <h3>{t('建议动作')}</h3>
        <div className='ct-profit-chip-list'>
          {actionCodes.length > 0 ? (
            actionCodes.map((code) => (
              <Tag key={code} color='blue' type='light'>
                {labelForRecommendationAction(code, t)}
              </Tag>
            ))
          ) : (
            <Tag type='light'>{t('暂无数据')}</Tag>
          )}
        </div>
      </div>

      <div className='ct-profit-decision-panel'>
        <div className='ct-profit-decision-head'>
          <div>
            <h3>{t('运营决策')}</h3>
            <p>{t('只记录运营决策，不会自动调整线上倍率。')}</p>
          </div>
          <Tag
            color={colorForRecommendationDecisionStatus(decisionStatus)}
            type='light'
          >
            {labelForRecommendationDecisionStatus(decisionStatus, t)}
          </Tag>
        </div>
        <div className='ct-profit-decision-status-row'>
          {DECISION_STATUS_OPTIONS.map((status) => (
            <Button
              key={status}
              size='small'
              theme={decisionStatus === status ? 'solid' : 'light'}
              type={decisionStatus === status ? 'primary' : 'tertiary'}
              onClick={() => setDecisionStatus(status)}
            >
              {labelForRecommendationDecisionStatus(status, t)}
            </Button>
          ))}
        </div>
        <div className='ct-profit-form-grid ct-profit-decision-form'>
          <label className='ct-profit-field'>
            <span>{t('计划收入倍率')}</span>
            <InputNumber
              min={0}
              max={100}
              step={0.01}
              suffix='x'
              value={plannedRevenueMultiplier}
              onChange={(value) =>
                setPlannedRevenueMultiplier(numberOrDefault(value))
              }
            />
          </label>
          <label className='ct-profit-field'>
            <span>{t('决策状态')}</span>
            <Select
              value={decisionStatus}
              onChange={setDecisionStatus}
              optionList={DECISION_STATUS_OPTIONS.map((status) => ({
                label: labelForRecommendationDecisionStatus(status, t),
                value: status,
              }))}
            />
          </label>
          <label className='ct-profit-field ct-profit-field-wide'>
            <span>{t('决策备注')}</span>
            <TextArea
              autosize={{ minRows: 2, maxRows: 5 }}
              value={decisionRemark}
              placeholder={t('例如：先对 default 分组小流量灰度 2 小时')}
              onChange={setDecisionRemark}
            />
          </label>
        </div>
        {(safeSnapshot.decision_operator_name ||
          safeSnapshot.decision_updated_at > 0) && (
          <div className='ct-profit-decision-meta'>
            <span>
              {t('操作人')}: {safeSnapshot.decision_operator_name || '--'}
            </span>
            <span>
              {t('决策时间')}:{' '}
              {formatTimestamp(safeSnapshot.decision_updated_at)}
            </span>
          </div>
        )}
        <div className='ct-profit-decision-actions'>
          <Button
            icon={<Plus size={15} />}
            disabled={!safeSnapshot.id}
            onClick={() => onCreateCanaryTask(safeSnapshot)}
          >
            {t('创建灰度任务')}
          </Button>
          <Button
            type='primary'
            icon={<Save size={15} />}
            loading={decisionSaving}
            disabled={!safeSnapshot.id}
            onClick={saveDecision}
          >
            {t('保存决策')}
          </Button>
        </div>
      </div>

      <div className='ct-profit-json-actions'>
        <Button icon={<Copy size={15} />} onClick={() => onCopy(inputText)}>
          {t('复制输入快照')}
        </Button>
        <Button
          icon={<Copy size={15} />}
          onClick={() => onCopy(recommendationText)}
        >
          {t('复制 AI 建议包')}
        </Button>
      </div>

      <div className='ct-profit-json-grid'>
        <div>
          <div className='ct-profit-json-title'>{t('输入快照')}</div>
          <pre>{inputText}</pre>
        </div>
        <div>
          <div className='ct-profit-json-title'>{t('AI 建议包')}</div>
          <pre>{recommendationText}</pre>
        </div>
      </div>
    </Modal>
  );
}

function CanaryTaskModal({ visible, task, saving, onCancel, onSave, t }) {
  const [form, setForm] = useState(DEFAULT_CANARY_FORM);

  useEffect(() => {
    if (visible) {
      setForm({ ...DEFAULT_CANARY_FORM, ...(task || {}) });
    }
  }, [visible, task]);

  const update = (key, value) => setForm((prev) => ({ ...prev, [key]: value }));

  const submit = () => {
    onSave({
      ...form,
      watch_metrics: normalizeWatchMetrics(form.watch_metrics),
    });
  };

  return (
    <Modal
      title={task?.id ? t('编辑灰度任务') : t('新增灰度任务')}
      visible={visible}
      onCancel={onCancel}
      onOk={submit}
      confirmLoading={saving}
      okText={t('保存')}
      cancelText={t('取消')}
      width={820}
      className='ct-profit-modal'
    >
      <div className='ct-profit-form-grid'>
        <label className='ct-profit-field ct-profit-field-wide'>
          <span>{t('任务标题')}</span>
          <Input
            value={form.title}
            placeholder={t('例如：default 分组倍率灰度观察')}
            onChange={(value) => update('title', value)}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('任务状态')}</span>
          <Select
            value={form.status}
            onChange={(value) => update('status', value)}
            optionList={CANARY_STATUS_OPTIONS.map((value) => ({
              label: labelForCanaryStatus(value, t),
              value,
            }))}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('关联快照')}</span>
          <InputNumber
            min={0}
            value={form.recommendation_id}
            onChange={(value) =>
              update('recommendation_id', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('灰度范围')}</span>
          <Select
            value={form.scope_type}
            onChange={(value) => update('scope_type', value)}
            optionList={SCOPE_TYPES.map((value) => ({
              label: labelForScope(value, t),
              value,
            }))}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('范围 ID')}</span>
          <InputNumber
            min={0}
            value={form.scope_id}
            onChange={(value) => update('scope_id', numberOrDefault(value))}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('范围标识')}</span>
          <Input
            value={form.scope_key}
            placeholder={t('分组名或模型名，可留空')}
            onChange={(value) => update('scope_key', value)}
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('观察窗口秒数')}</span>
          <InputNumber
            min={60}
            step={300}
            value={form.observation_window_seconds}
            onChange={(value) =>
              update('observation_window_seconds', numberOrDefault(value, 7200))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('基线收入倍率')}</span>
          <InputNumber
            min={0}
            max={100}
            step={0.01}
            suffix='x'
            value={form.baseline_revenue_multiplier}
            onChange={(value) =>
              update('baseline_revenue_multiplier', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('计划收入倍率')}</span>
          <InputNumber
            min={0}
            max={100}
            step={0.01}
            suffix='x'
            value={form.planned_revenue_multiplier}
            onChange={(value) =>
              update('planned_revenue_multiplier', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('建议收入倍率')}</span>
          <InputNumber
            min={0}
            max={100}
            step={0.01}
            suffix='x'
            value={form.recommended_revenue_multiplier}
            onChange={(value) =>
              update('recommended_revenue_multiplier', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('计划开始时间戳')}</span>
          <InputNumber
            min={0}
            step={60}
            value={form.planned_start_at}
            onChange={(value) =>
              update('planned_start_at', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field'>
          <span>{t('计划结束时间戳')}</span>
          <InputNumber
            min={0}
            step={60}
            value={form.planned_end_at}
            onChange={(value) =>
              update('planned_end_at', numberOrDefault(value))
            }
          />
        </label>
        <label className='ct-profit-field ct-profit-field-wide'>
          <span>{t('观察指标')}</span>
          <Select
            multiple
            value={normalizeWatchMetrics(form.watch_metrics)}
            onChange={(value) =>
              update('watch_metrics', normalizeWatchMetrics(value))
            }
            optionList={CANARY_WATCH_METRIC_OPTIONS.map((value) => ({
              label: `${labelForCanaryWatchMetric(value, t)} · ${value}`,
              value,
            }))}
            placeholder={t('选择灰度观察指标')}
            style={{ width: '100%' }}
          />
        </label>
        <label className='ct-profit-field ct-profit-field-wide'>
          <span>{t('复盘结论')}</span>
          <TextArea
            autosize={{ minRows: 2, maxRows: 5 }}
            value={form.result_summary}
            placeholder={t('记录灰度结果、是否扩大范围、是否回滚等')}
            onChange={(value) => update('result_summary', value)}
          />
        </label>
      </div>
      <div className='ct-profit-help compact'>
        <Save size={15} />
        <span>{t('灰度任务仅用于追踪，不会自动修改计费配置。')}</span>
      </div>
    </Modal>
  );
}

export default function ProfitMonitor() {
  const { t } = useTranslation();
  const [windowKey, setWindowKey] = useState('24h');
  const [dimension, setDimension] = useState('group');
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [resourceSaving, setResourceSaving] = useState(false);
  const [recommendationGenerating, setRecommendationGenerating] =
    useState(false);
  const [decisionSaving, setDecisionSaving] = useState(false);
  const [canarySaving, setCanarySaving] = useState(false);
  const [configVisible, setConfigVisible] = useState(false);
  const [resourceModalVisible, setResourceModalVisible] = useState(false);
  const [canaryModalVisible, setCanaryModalVisible] = useState(false);
  const [editingResource, setEditingResource] = useState(null);
  const [editingCanaryTask, setEditingCanaryTask] = useState(null);
  const [selectedRecommendation, setSelectedRecommendation] = useState(null);
  const [recommendations, setRecommendations] = useState([]);
  const [canaryTasks, setCanaryTasks] = useState([]);
  const [data, setData] = useState({
    config: DEFAULT_CONFIG,
    summary: {},
    breakdown: [],
    resources: { items: [] },
    recommendation: {},
  });
  const [trafficData, setTrafficData] = useState({
    summary: {},
    breakdown: [],
    series: [],
  });

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [
        summaryResponse,
        trafficResponse,
        recommendationResponse,
        canaryTaskResponse,
      ] = await Promise.all([
        API.get('/api/model_gateway/profit_monitor/summary', {
          params: { window: windowKey, dimension },
          disableDuplicate: true,
        }),
        API.get('/api/model_gateway/profit_monitor/traffic', {
          params: { window: windowKey, dimension },
          disableDuplicate: true,
        }),
        API.get('/api/model_gateway/profit_monitor/recommendations', {
          params: { limit: 8, window: windowKey, dimension },
          disableDuplicate: true,
        }),
        API.get('/api/model_gateway/profit_monitor/canary_tasks', {
          params: { limit: 8 },
          disableDuplicate: true,
        }),
      ]);
      const payload = unwrapApiData(summaryResponse);
      const trafficPayload = unwrapApiData(trafficResponse);
      const recommendationPayload = unwrapApiData(recommendationResponse);
      const canaryTaskPayload = unwrapApiData(canaryTaskResponse);
      setData({
        config: { ...DEFAULT_CONFIG, ...(payload.config || {}) },
        summary: payload.summary || {},
        breakdown: payload.breakdown || [],
        resources: payload.resources || { items: [] },
        recommendation: payload.recommendation || {},
        window: payload.window,
        dimension: payload.dimension,
        start_timestamp: payload.start_timestamp,
        end_timestamp: payload.end_timestamp,
      });
      setTrafficData({
        summary: trafficPayload.summary || {},
        breakdown: trafficPayload.breakdown || [],
        series: trafficPayload.series || [],
        window: trafficPayload.window,
        dimension: trafficPayload.dimension,
      });
      setRecommendations(
        Array.isArray(recommendationPayload) ? recommendationPayload : [],
      );
      setCanaryTasks(Array.isArray(canaryTaskPayload) ? canaryTaskPayload : []);
    } catch (error) {
      showError(t('获取盈利监控数据失败：') + (error.message || ''));
    } finally {
      setLoading(false);
    }
  }, [dimension, t, windowKey]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const summary = data.summary || {};
  const resources = data.resources || { items: [] };
  const recommendation = data.recommendation || {};
  const costMultiplier = resolveCostMultiplier(recommendation);
  const profitMarkupMultiplier = resolveCostMarkupMultiplier(
    recommendation,
    summary,
  );
  const trafficSummary = trafficData.summary || {};

  const metricCards = useMemo(
    () => [
      {
        icon: DollarSign,
        label: t('收入'),
        value: formatUsd(summary.revenue_usd, 4),
        detail: t('{{count}} 次真实请求', {
          count: formatNumber(summary.requests),
        }),
        tone: 'success',
      },
      {
        icon: PackageCheck,
        label: t('总经营成本'),
        value: formatUsd(summary.operating_cost_usd, 4),
        detail: `${t('上游')} ${formatUsd(summary.upstream_cost_usd, 4)} · ${t(
          '资源',
        )} ${formatUsd(
          Number(summary.resource_amortized_cost_usd || 0) +
            Number(summary.resource_loss_cost_usd || 0),
          4,
        )}`,
        tone: 'warning',
      },
      {
        icon: Network,
        label: t('流量成本'),
        value: formatUsd(summary.traffic_cost_usd, 4),
        detail: summary.traffic_estimated
          ? t('估算 {{bytes}}', {
              bytes: formatBytes(summary.traffic_bytes),
            })
          : t('入站 {{inbound}} · 出站 {{outbound}}', {
              inbound: formatBytes(summary.traffic_request_bytes),
              outbound: formatBytes(summary.traffic_response_bytes),
            }),
        tone: summary.traffic_estimated ? 'warning' : 'info',
      },
      {
        icon: TrendingUp,
        label: t('毛利'),
        value: formatUsd(summary.profit_usd, 4),
        detail: t('毛利率 {{rate}}', {
          rate: formatPercent(summary.gross_margin),
        }),
        tone: metricTone(summary.profit_usd),
      },
      {
        icon: Server,
        label: t('日摊与资源'),
        value: formatUsd(
          Number(summary.server_cost_usd || 0) +
            Number(summary.resource_amortized_cost_usd || 0) +
            Number(summary.resource_loss_cost_usd || 0),
          4,
        ),
        detail: t('服务器 {{server}} · 损耗 {{loss}}', {
          server: formatUsd(summary.server_cost_usd, 4),
          loss: formatUsd(summary.resource_loss_cost_usd, 4),
        }),
        tone: 'neutral',
      },
      {
        icon: Activity,
        label: t('完成率'),
        value: formatPercent(summary.success_rate),
        detail: t('{{success}} / {{total}} 成功', {
          success: formatNumber(summary.success_requests),
          total: formatNumber(summary.requests),
        }),
        tone: 'neutral',
      },
      {
        icon: BarChart3,
        label: t('建议最低收入'),
        value: formatUsd(recommendation.required_revenue_usd, 4),
        detail: recommendation.can_recommend
          ? `${t('目标毛利率')} ${formatPercent(
              recommendation.target_profit_rate,
            )}`
          : t('样本不足，暂不建议'),
        tone: 'info',
      },
      {
        icon: Sparkles,
        label: t('建议动态倍率'),
        value:
          Number(recommendation.suggested_dynamic_ratio || 0) > 0
            ? formatRatioValue(recommendation.suggested_dynamic_ratio)
            : '--',
        detail: recommendation.dynamic_ratio_limit_applied
          ? `${t('原始 {{raw}}x', {
              raw: formatRatioNumber(recommendation.suggested_dynamic_ratio_raw),
            })} · ${formatDynamicRatioLimitResult(recommendation, t)}`
          : Number(recommendation.current_effective_dynamic_ratio || 0) > 0
            ? t('当前生效 {{ratio}}x', {
                ratio: formatRatioNumber(
                  recommendation.current_effective_dynamic_ratio,
                ),
              })
            : t('尚未应用到动态倍率'),
        tone: recommendation.dynamic_billing_applied ? 'success' : 'info',
      },
    ],
    [profitMarkupMultiplier, recommendation, summary, t],
  );

  const trafficMetricCards = useMemo(
    () => [
      {
        icon: Network,
        label: t('总流量'),
        value: formatBytes(trafficSummary.total_bytes),
        detail: t('{{count}} 次流量请求', {
          count: formatNumber(trafficSummary.request_count),
        }),
        tone: trafficSummary.data_ready ? 'info' : 'neutral',
      },
      {
        icon: Activity,
        label: t('入站流量'),
        value: formatBytes(trafficSummary.request_bytes),
        detail: t('客户端请求体和请求头估算'),
        tone: 'neutral',
      },
      {
        icon: BarChart3,
        label: t('出站流量'),
        value: formatBytes(trafficSummary.response_bytes),
        detail: t('实际写回客户端的响应字节'),
        tone: 'neutral',
      },
      {
        icon: DollarSign,
        label: t('流量成本'),
        value: formatUsd(trafficSummary.traffic_cost_usd, 4),
        detail: t('每 GB {{cost}}', {
          cost: formatUsd(trafficSummary.traffic_cost_per_gb, 4),
        }),
        tone: 'warning',
      },
    ],
    [t, trafficSummary],
  );

  const saveConfig = async (nextConfig) => {
    setSaving(true);
    try {
      await API.patch('/api/model_gateway/profit_monitor/config', nextConfig);
      showSuccess(t('盈利监控设置已保存'));
      setConfigVisible(false);
      await fetchData();
    } catch (error) {
      showError(t('保存失败：') + (error.message || ''));
    } finally {
      setSaving(false);
    }
  };

  const generateRecommendationSnapshot = async () => {
    setRecommendationGenerating(true);
    try {
      await API.post(
        '/api/model_gateway/profit_monitor/recommendations',
        null,
        {
          params: { window: windowKey, dimension },
          disableDuplicate: true,
        },
      );
      showSuccess(t('动态倍率建议快照已生成'));
      await fetchData();
    } catch (error) {
      showError(t('生成建议快照失败：') + (error.message || ''));
    } finally {
      setRecommendationGenerating(false);
    }
  };

  const copyRecommendationPayload = async (text) => {
    if (await copy(text)) {
      showSuccess(t('已复制到剪贴板'));
    } else {
      showError(t('复制失败，请手动复制'));
    }
  };

  const saveRecommendationDecision = async (id, payload) => {
    if (!id) return;
    setDecisionSaving(true);
    try {
      const response = await API.patch(
        `/api/model_gateway/profit_monitor/recommendations/${id}/decision`,
        payload,
      );
      const updated = unwrapApiData(response);
      if (updated?.id) {
        setRecommendations((prev) =>
          prev.map((item) => (item.id === updated.id ? updated : item)),
        );
        setSelectedRecommendation(updated);
      }
      showSuccess(t('建议快照决策已保存'));
      await fetchData();
    } catch (error) {
      showError(t('保存决策失败：') + (error.message || ''));
    } finally {
      setDecisionSaving(false);
    }
  };

  const openCanaryTaskModal = (source = null) => {
    if (source?.watch_metrics || source?.status) {
      setEditingCanaryTask({
        ...DEFAULT_CANARY_FORM,
        ...source,
        watch_metrics: normalizeWatchMetrics(source.watch_metrics),
      });
      setCanaryModalVisible(true);
      return;
    }

    const snapshot = source || {};
    const scopeType = SCOPE_TYPES.includes(snapshot.dimension)
      ? snapshot.dimension
      : 'global';
    const recommendedMultiplier = numberOrDefault(
      snapshot.recommended_revenue_multiplier,
    );
    const plannedMultiplier = numberOrDefault(
      snapshot.planned_revenue_multiplier,
      recommendedMultiplier,
    );
    const now = Math.floor(Date.now() / 1000);
    setEditingCanaryTask({
      ...DEFAULT_CANARY_FORM,
      recommendation_id: numberOrDefault(snapshot.id),
      title: snapshot.id
        ? t('盈利建议 #{{id}} 灰度任务', { id: snapshot.id })
        : '',
      scope_type: scopeType,
      baseline_revenue_multiplier: 0,
      planned_revenue_multiplier:
        plannedMultiplier > 0 ? plannedMultiplier : recommendedMultiplier,
      recommended_revenue_multiplier: recommendedMultiplier,
      planned_start_at: now,
      planned_end_at: now + DEFAULT_CANARY_FORM.observation_window_seconds,
      watch_metrics: canaryDefaultWatchMetrics(),
    });
    setCanaryModalVisible(true);
  };

  const closeCanaryTaskModal = () => {
    setEditingCanaryTask(null);
    setCanaryModalVisible(false);
  };

  const saveCanaryTask = async (form) => {
    if (!String(form.title || '').trim()) {
      showError(t('灰度任务标题不能为空'));
      return;
    }
    setCanarySaving(true);
    try {
      const payload = {
        ...form,
        watch_metrics: normalizeWatchMetrics(form.watch_metrics),
      };
      if (editingCanaryTask?.id) {
        await API.patch(
          `/api/model_gateway/profit_monitor/canary_tasks/${editingCanaryTask.id}`,
          payload,
        );
      } else {
        await API.post(
          '/api/model_gateway/profit_monitor/canary_tasks',
          payload,
        );
      }
      showSuccess(t('灰度任务已保存'));
      closeCanaryTaskModal();
      await fetchData();
    } catch (error) {
      showError(t('保存灰度任务失败：') + (error.message || ''));
    } finally {
      setCanarySaving(false);
    }
  };

  const openResourceModal = (resource = null) => {
    setEditingResource(resource);
    setResourceModalVisible(true);
  };

  const closeResourceModal = () => {
    setEditingResource(null);
    setResourceModalVisible(false);
  };

  const saveResource = async (form) => {
    if (!String(form.name || '').trim()) {
      showError(t('资源名称不能为空'));
      return;
    }
    setResourceSaving(true);
    try {
      if (editingResource?.id) {
        await API.patch(
          `/api/model_gateway/profit_monitor/resources/${editingResource.id}`,
          form,
        );
      } else {
        await API.post('/api/model_gateway/profit_monitor/resources', form);
      }
      showSuccess(t('资源成本已保存'));
      closeResourceModal();
      await fetchData();
    } catch (error) {
      showError(t('保存失败：') + (error.message || ''));
    } finally {
      setResourceSaving(false);
    }
  };

  const deleteResource = (resource) => {
    Modal.confirm({
      title: t('删除资源成本'),
      content: t('删除后该资源不再参与后续经营成本统计，确定继续吗？'),
      okText: t('删除'),
      cancelText: t('取消'),
      okButtonProps: { type: 'danger' },
      onOk: async () => {
        try {
          await API.delete(
            `/api/model_gateway/profit_monitor/resources/${resource.id}`,
          );
          showSuccess(t('资源成本已删除'));
          await fetchData();
        } catch (error) {
          showError(t('删除失败：') + (error.message || ''));
        }
      },
    });
  };

  const breakdownColumns = [
    {
      title: t('维度'),
      dataIndex: 'dimension_name',
      width: 220,
      render: (value, record) => (
        <div className='ct-profit-name-cell'>
          <strong>{value || record.dimension_key || '--'}</strong>
          <span>
            {dimension === 'channel' && record.dimension_id
              ? `#${record.dimension_id}`
              : record.dimension_key || ''}
          </span>
        </div>
      ),
    },
    {
      title: t('请求数'),
      dataIndex: 'requests',
      render: (value) => formatNumber(value),
    },
    {
      title: t('完成率'),
      dataIndex: 'success_rate',
      render: (value) => formatPercent(value),
    },
    {
      title: t('收入'),
      dataIndex: 'revenue_usd',
      render: (value) => formatUsd(value, 4),
    },
    {
      title: t('上游成本'),
      dataIndex: 'upstream_cost_usd',
      render: (value) => formatUsd(value, 4),
    },
    {
      title: t('流量成本'),
      dataIndex: 'traffic_cost_usd',
      render: (value, record) => (
        <div className='ct-profit-name-cell'>
          <strong>{formatUsd(value, 4)}</strong>
          <span>{formatBytes(record.traffic_bytes)}</span>
        </div>
      ),
    },
    {
      title: t('分摊经营成本'),
      dataIndex: 'allocated_operating_cost_usd',
      render: (value) => formatUsd(value, 4),
    },
    {
      title: t('毛利'),
      dataIndex: 'profit_usd',
      render: (value) => (
        <Text type={Number(value) < 0 ? 'danger' : 'success'}>
          {formatUsd(value, 4)}
        </Text>
      ),
    },
    {
      title: t('毛利率'),
      dataIndex: 'gross_margin',
      render: (value) => formatPercent(value),
    },
  ];

  const trafficBreakdownColumns = [
    {
      title: t('维度'),
      dataIndex: 'dimension_name',
      width: 220,
      render: (value, record) => (
        <div className='ct-profit-name-cell'>
          <strong>{value || record.dimension_key || '--'}</strong>
          <span>{record.dimension_key || ''}</span>
        </div>
      ),
    },
    {
      title: t('流量请求'),
      dataIndex: 'request_count',
      render: (value) => formatNumber(value),
    },
    {
      title: t('入站流量'),
      dataIndex: 'request_bytes',
      render: (value) => formatBytes(value),
    },
    {
      title: t('出站流量'),
      dataIndex: 'response_bytes',
      render: (value) => formatBytes(value),
    },
    {
      title: t('总流量'),
      dataIndex: 'total_bytes',
      render: (value) => formatBytes(value),
    },
    {
      title: t('占比'),
      dataIndex: 'share',
      render: (value) => formatShare(value),
    },
    {
      title: t('流量成本'),
      dataIndex: 'traffic_cost_usd',
      render: (value) => formatUsd(value, 4),
    },
  ];

  const trafficSeriesColumns = [
    {
      title: t('采集时间'),
      dataIndex: 'bucket_ts',
      width: 180,
      render: (value) => formatTimestamp(value),
    },
    {
      title: t('流量请求'),
      dataIndex: 'request_count',
      render: (value) => formatNumber(value),
    },
    {
      title: t('入站流量'),
      dataIndex: 'request_bytes',
      render: (value) => formatBytes(value),
    },
    {
      title: t('出站流量'),
      dataIndex: 'response_bytes',
      render: (value) => formatBytes(value),
    },
    {
      title: t('总流量'),
      dataIndex: 'total_bytes',
      render: (value) => formatBytes(value),
    },
    {
      title: t('流量成本'),
      dataIndex: 'traffic_cost_usd',
      render: (value) => formatUsd(value, 4),
    },
  ];

  const recommendationColumns = [
    {
      title: t('生成时间'),
      dataIndex: 'created_at',
      width: 160,
      render: (value) => formatTimestamp(value),
    },
    {
      title: t('窗口'),
      dataIndex: 'window',
      width: 80,
      render: (value) => value || '--',
    },
    {
      title: t('维度'),
      dataIndex: 'dimension',
      width: 100,
      render: (value) => labelForDimension(value, t),
    },
    {
      title: t('当前毛利率'),
      dataIndex: 'current_margin',
      render: (value) => formatPercent(value),
    },
    {
      title: t('目标毛利率'),
      dataIndex: 'target_profit_rate',
      render: (value) => formatPercent(value),
    },
    {
      title: t('成本倍率'),
      dataIndex: 'cost_multiplier',
      width: 120,
      render: (value, record) =>
        formatMultiplier(
          resolveCostMultiplier({
            ...record,
            cost_multiplier: value,
          }),
          4,
        ),
    },
    {
      title: t('目标毛利换算'),
      dataIndex: 'cost_markup_multiplier',
      width: 170,
      render: (value, record) =>
        formatProfitMarkupFormula(
          record.target_profit_rate,
          resolveCostMarkupMultiplier({
            ...record,
            cost_markup_multiplier: value,
          }),
        ),
    },
    {
      title: t('建议收入倍率'),
      dataIndex: 'recommended_revenue_multiplier',
      render: (value) =>
        Number(value) > 0 ? formatMultiplier(value) : '--',
    },
    {
      title: t('每百万 token 最低收入'),
      dataIndex: 'recommended_floor_per_m_token_usd',
      render: (value) => formatUsd(value, 4),
    },
    {
      title: t('风险'),
      dataIndex: 'risk_level',
      render: (value) => (
        <Tag color={colorForRecommendationRisk(value)} type='light'>
          {labelForRecommendationRisk(value, t)}
        </Tag>
      ),
    },
    {
      title: t('决策'),
      dataIndex: 'decision_status',
      width: 140,
      render: (value, record) => (
        <div className='ct-profit-name-cell'>
          <Tag color={colorForRecommendationDecisionStatus(value)} type='light'>
            {labelForRecommendationDecisionStatus(value, t)}
          </Tag>
          {Number(record.planned_revenue_multiplier) > 0 && (
            <span>
              {t('计划 {{ratio}}x', {
                ratio: Number(record.planned_revenue_multiplier).toFixed(2),
              })}
            </span>
          )}
        </div>
      ),
    },
    {
      title: t('置信度'),
      dataIndex: 'confidence',
      render: (value) => formatPercent(value),
    },
    {
      title: t('建议原因'),
      dataIndex: 'reason',
      width: 260,
      render: (value) => <span>{labelForRecommendationReason(value, t)}</span>,
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      fixed: 'right',
      render: (_, record) => (
        <Button
          size='small'
          icon={<Eye size={14} />}
          onClick={() => setSelectedRecommendation(record)}
        >
          {t('查看')}
        </Button>
      ),
    },
  ];

  const canaryTaskColumns = [
    {
      title: t('灰度任务'),
      dataIndex: 'title',
      width: 240,
      render: (value, record) => (
        <div className='ct-profit-name-cell'>
          <strong>{value || '--'}</strong>
          <span>
            {record.recommendation_id
              ? t('关联快照 #{{id}}', { id: record.recommendation_id })
              : t('未关联建议快照')}
          </span>
        </div>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      width: 100,
      render: (value) => (
        <Tag color={colorForCanaryStatus(value)} type='light'>
          {labelForCanaryStatus(value, t)}
        </Tag>
      ),
    },
    {
      title: t('范围'),
      dataIndex: 'scope_type',
      width: 160,
      render: (value, record) => (
        <span>
          {labelForScope(value, t)}
          {record.scope_key ? ` · ${record.scope_key}` : ''}
          {record.scope_id ? ` #${record.scope_id}` : ''}
        </span>
      ),
    },
    {
      title: t('计划倍率'),
      dataIndex: 'planned_revenue_multiplier',
      width: 120,
      render: (value, record) => (
        <div className='ct-profit-name-cell'>
          <strong>
            {Number(value) > 0 ? `${Number(value).toFixed(2)}x` : '--'}
          </strong>
          {Number(record.recommended_revenue_multiplier) > 0 && (
            <span>
              {t('建议 {{ratio}}x', {
                ratio: Number(record.recommended_revenue_multiplier).toFixed(2),
              })}
            </span>
          )}
        </div>
      ),
    },
    {
      title: t('计划窗口'),
      dataIndex: 'planned_start_at',
      width: 260,
      render: (_, record) => (
        <div className='ct-profit-name-cell'>
          <strong>{formatTimestamp(record.planned_start_at)}</strong>
          <span>{formatTimestamp(record.planned_end_at)}</span>
        </div>
      ),
    },
    {
      title: t('观察指标'),
      dataIndex: 'watch_metrics',
      width: 260,
      render: (value) => (
        <div className='ct-profit-chip-list ct-profit-chip-list-tight'>
          {normalizeWatchMetrics(value).map((item) => (
            <Tag key={item} type='light' color='teal'>
              {labelForCanaryWatchMetric(item, t)}
            </Tag>
          ))}
        </div>
      ),
    },
    {
      title: t('复盘结论'),
      dataIndex: 'result_summary',
      width: 220,
      render: (value) => value || '--',
    },
    {
      title: t('更新时间'),
      dataIndex: 'updated_at',
      width: 160,
      render: (value) => formatTimestamp(value),
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      fixed: 'right',
      width: 100,
      render: (_, record) => (
        <Button size='small' onClick={() => openCanaryTaskModal(record)}>
          {t('编辑')}
        </Button>
      ),
    },
  ];

  const resourceColumns = [
    {
      title: t('资源'),
      dataIndex: 'name',
      width: 220,
      render: (value, record) => (
        <div className='ct-profit-name-cell'>
          <strong>{value}</strong>
          <span>
            {record.remark || labelForResourceType(record.resource_type, t)}
          </span>
        </div>
      ),
    },
    {
      title: t('类型'),
      dataIndex: 'resource_type',
      render: (value) => (
        <Tag color='cyan' type='light'>
          {labelForResourceType(value, t)}
        </Tag>
      ),
    },
    {
      title: t('范围'),
      dataIndex: 'scope_type',
      render: (value, record) => (
        <span>
          {labelForScope(value, t)}
          {record.scope_key ? ` · ${record.scope_key}` : ''}
          {record.scope_id ? ` #${record.scope_id}` : ''}
        </span>
      ),
    },
    {
      title: t('成本'),
      dataIndex: 'amount_usd',
      render: (value, record) => (
        <span>
          {formatUsd(value, 4)} / {formatPeriod(record.period_seconds, t)}
        </span>
      ),
    },
    {
      title: t('窗口摊销'),
      dataIndex: 'window_cost_usd',
      render: (value) => formatUsd(value, 4),
    },
    {
      title: t('窗口损耗'),
      dataIndex: 'window_loss_usd',
      render: (value) => formatUsd(value, 4),
    },
    {
      title: t('状态'),
      dataIndex: 'enabled',
      render: (value) =>
        value !== false ? (
          <Tag color='green' type='light'>
            {t('启用')}
          </Tag>
        ) : (
          <Tag color='grey' type='light'>
            {t('停用')}
          </Tag>
        ),
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button size='small' onClick={() => openResourceModal(record)}>
            {t('编辑')}
          </Button>
          <Button
            size='small'
            type='danger'
            icon={<Trash2 size={14} />}
            onClick={() => deleteResource(record)}
          />
        </Space>
      ),
    },
  ];

  return (
    <div className='ct-profit-page'>
      <div className='ct-profit-header'>
        <div>
          <div className='ct-profit-eyebrow'>{t('经营分析')}</div>
          <h1>{t('盈利监控台')}</h1>
          <p>
            {t('按真实请求收入、上游成本、服务器日摊和资源损耗汇总经营利润。')}
          </p>
          <div className='ct-profit-window'>
            <span>
              {formatTimestamp(data.start_timestamp)} -{' '}
              {formatTimestamp(data.end_timestamp)}
            </span>
            {summary.traffic_data_ready && !summary.traffic_estimated ? (
              <Tag color='green' type='light'>
                {t('已接入真实流量字节')}
              </Tag>
            ) : summary.traffic_estimated ? (
              <Tag color='orange' type='light'>
                {t('流量为估算值')}
              </Tag>
            ) : (
              <Tag color='grey' type='light'>
                {t('未接入真实流量字节')}
              </Tag>
            )}
          </div>
        </div>
        <div className='ct-profit-actions'>
          <Select
            value={windowKey}
            onChange={setWindowKey}
            optionList={WINDOW_OPTIONS.map((value) => ({
              label: value,
              value,
            }))}
          />
          <Select
            value={dimension}
            onChange={setDimension}
            optionList={DIMENSION_OPTIONS.map((value) => ({
              label: labelForDimension(value, t),
              value,
            }))}
          />
          <Button
            icon={<RefreshCw size={16} />}
            onClick={fetchData}
            loading={loading}
          >
            {t('刷新')}
          </Button>
          <Button
            icon={<Settings size={16} />}
            onClick={() => setConfigVisible(true)}
          >
            {t('设置')}
          </Button>
        </div>
      </div>

      <Spin spinning={loading}>
        <div className='ct-profit-metrics'>
          {metricCards.map((card) => (
            <MetricCard key={card.label} {...card} />
          ))}
        </div>

        <Tabs type='button' className='ct-profit-tabs' keepDOM={false}>
          <Tabs.TabPane tab={t('利润总览')} itemKey='overview'>
            <div className='ct-profit-panels'>
              <section className='ct-profit-panel ct-profit-panel-wide'>
                <div className='ct-profit-panel-head'>
                  <div>
                    <h2>{t('经营拆解')}</h2>
                    <p>
                      {t('分摊成本用于定位哪些分组、渠道或模型正在吞噬利润。')}
                    </p>
                  </div>
                  <Tag color='teal' type='light'>
                    {t('真实请求，不含健康探活')}
                  </Tag>
                </div>
                <Table
                  size='small'
                  columns={breakdownColumns}
                  dataSource={data.breakdown || []}
                  rowKey={(record, index) =>
                    `${record.dimension_id}-${record.dimension_key}-${index}`
                  }
                  pagination={{ pageSize: 10 }}
                  empty={<Empty description={t('暂无经营拆解数据')} />}
                  scroll={{ x: 1080 }}
                />
              </section>

              <section className='ct-profit-panel'>
                <div className='ct-profit-panel-head'>
                  <div>
                    <h2>{t('动态倍率建议')}</h2>
                    <p>{t('基于当前窗口总经营成本和目标毛利率计算。')}</p>
                  </div>
                  <Button
                    type='primary'
                    icon={<Sparkles size={16} />}
                    loading={recommendationGenerating}
                    onClick={generateRecommendationSnapshot}
                  >
                    {t('生成建议快照')}
                  </Button>
                </div>
                <div className='ct-profit-recommendation'>
                  <div>
                    <span>{t('目标毛利率')}</span>
                    <strong>
                      {formatPercent(recommendation.target_profit_rate)}
                    </strong>
                  </div>
                  <div>
                    <span>{t('收入缺口')}</span>
                    <strong>
                      {formatUsd(recommendation.revenue_gap_usd, 4)}
                    </strong>
                  </div>
                  <div>
                    <span>{t('成本倍率')}</span>
                    <strong>{formatMultiplier(costMultiplier, 4)}</strong>
                  </div>
                  <div>
                    <span>{t('目标毛利换算')}</span>
                    <strong>
                      {formatProfitMarkupFormula(
                        recommendation.target_profit_rate,
                        profitMarkupMultiplier,
                      )}
                    </strong>
                  </div>
                  <div>
                    <span>{t('每百万 token 最低收入')}</span>
                    <strong>
                      {formatUsd(
                        recommendation.recommended_floor_per_m_token_usd,
                        4,
                      )}
                    </strong>
                  </div>
                  <div>
                    <span>{t('每百万基础计费额度最低收入')}</span>
                    <strong>
                      {formatUsd(
                        recommendation.minimum_revenue_per_m_base_quota_usd,
                        4,
                      )}
                    </strong>
                  </div>
                  <div>
                    <span>{t('建议收入倍率')}</span>
                    <strong>
                      {recommendation.can_recommend
                        ? formatMultiplier(
                            recommendation.recommended_revenue_multiplier,
                          )
                        : '--'}
                    </strong>
                  </div>
                  <div>
                    <span>{t('建议动态倍率')}</span>
                    <strong>
                      {Number(recommendation.suggested_dynamic_ratio || 0) > 0
                        ? formatRatioValue(recommendation.suggested_dynamic_ratio)
                        : '--'}
                    </strong>
                  </div>
                  <div>
                    <span>{t('原始建议动态倍率')}</span>
                    <strong>
                      {Number(recommendation.suggested_dynamic_ratio_raw || 0) >
                      0
                        ? formatRatioValue(
                            recommendation.suggested_dynamic_ratio_raw,
                          )
                        : '--'}
                    </strong>
                  </div>
                  <div>
                    <span>{t('倍率限制')}</span>
                    <strong>
                      {formatDynamicRatioLimitRange(recommendation, t)}
                    </strong>
                  </div>
                  <div>
                    <span>{t('限制结果')}</span>
                    <strong>
                      {formatDynamicRatioLimitResult(recommendation, t)}
                    </strong>
                  </div>
                  <div>
                    <span>{t('当前生效动态倍率')}</span>
                    <strong>
                      {Number(
                        recommendation.current_effective_dynamic_ratio || 0,
                      ) > 0
                        ? formatRatioValue(
                            recommendation.current_effective_dynamic_ratio,
                          )
                        : '--'}
                    </strong>
                  </div>
                </div>
                <div className='ct-profit-help compact'>
                  <Save size={15} />
                  <span>{t('生成建议只保存快照，不会自动调整线上倍率。')}</span>
                </div>
                <div className='ct-profit-snapshot-list'>
                  <div className='ct-profit-subhead'>
                    <strong>{t('建议快照记录')}</strong>
                    <span>
                      {t('保留最近生成的建议，方便人工复盘和后续接入 AI。')}
                    </span>
                  </div>
                  <Table
                    size='small'
                    columns={recommendationColumns}
                    dataSource={recommendations}
                    rowKey='id'
                    pagination={false}
                    empty={<Empty description={t('暂无建议快照')} />}
                    scroll={{ x: 1240 }}
                  />
                </div>
              </section>
            </div>

            <section className='ct-profit-panel'>
              <div className='ct-profit-panel-head'>
                <div>
                  <h2>{t('灰度任务台账')}</h2>
                  <p>
                    {t(
                      '把建议快照变成可跟踪的灰度任务，记录计划、观察指标和复盘结论。',
                    )}
                  </p>
                </div>
                <Button
                  type='primary'
                  icon={<Plus size={16} />}
                  onClick={() => openCanaryTaskModal()}
                >
                  {t('新增灰度任务')}
                </Button>
              </div>
              <Table
                size='small'
                columns={canaryTaskColumns}
                dataSource={canaryTasks}
                rowKey='id'
                pagination={{ pageSize: 8 }}
                empty={<Empty description={t('暂无灰度任务')} />}
                scroll={{ x: 1480 }}
              />
            </section>

            <section className='ct-profit-panel'>
              <div className='ct-profit-panel-head'>
                <div>
                  <h2>{t('资源成本台账')}</h2>
                  <p>
                    {t(
                      '账号池采购、封号损耗、代理和服务器等资源成本独立记录。',
                    )}
                  </p>
                </div>
                <Button
                  type='primary'
                  icon={<Plus size={16} />}
                  onClick={() => openResourceModal()}
                >
                  {t('新增资源成本')}
                </Button>
              </div>
              <Table
                size='small'
                columns={resourceColumns}
                dataSource={resources.items || []}
                rowKey='id'
                pagination={{ pageSize: 8 }}
                empty={<Empty description={t('暂无资源成本')} />}
                scroll={{ x: 1120 }}
              />
            </section>
          </Tabs.TabPane>

          <Tabs.TabPane tab={t('流量统计')} itemKey='traffic'>
            <div className='ct-profit-metrics ct-profit-traffic-metrics'>
              {trafficMetricCards.map((card) => (
                <MetricCard key={card.label} {...card} />
              ))}
            </div>
            <div className='ct-profit-panels'>
              <section className='ct-profit-panel ct-profit-panel-wide'>
                <div className='ct-profit-panel-head'>
                  <div>
                    <h2>{t('真实流量明细')}</h2>
                    <p>{t('按所选维度拆分真实入站、出站和成本。')}</p>
                  </div>
                  <Tag color='green' type='light'>
                    {trafficSummary.data_ready
                      ? t('真实流量')
                      : t('暂无真实流量')}
                  </Tag>
                </div>
                <Table
                  size='small'
                  columns={trafficBreakdownColumns}
                  dataSource={trafficData.breakdown || []}
                  rowKey={(record, index) =>
                    `${record.dimension_id}-${record.dimension_key}-${index}`
                  }
                  pagination={{ pageSize: 10 }}
                  empty={<Empty description={t('暂无流量拆解数据')} />}
                  scroll={{ x: 980 }}
                />
              </section>

              <section className='ct-profit-panel'>
                <div className='ct-profit-panel-head'>
                  <div>
                    <h2>{t('小时趋势')}</h2>
                    <p>{t('按采集桶展示真实请求字节变化。')}</p>
                  </div>
                </div>
                <Table
                  size='small'
                  columns={trafficSeriesColumns}
                  dataSource={trafficData.series || []}
                  rowKey='bucket_ts'
                  pagination={{ pageSize: 8 }}
                  empty={<Empty description={t('暂无流量趋势数据')} />}
                  scroll={{ x: 780 }}
                />
              </section>
            </div>
          </Tabs.TabPane>
        </Tabs>
      </Spin>

      <ConfigModal
        visible={configVisible}
        config={data.config}
        saving={saving}
        onCancel={() => setConfigVisible(false)}
        onSave={saveConfig}
        t={t}
      />
      <ResourceModal
        visible={resourceModalVisible}
        resource={editingResource}
        saving={resourceSaving}
        onCancel={closeResourceModal}
        onSave={saveResource}
        t={t}
      />
      <CanaryTaskModal
        visible={canaryModalVisible}
        task={editingCanaryTask}
        saving={canarySaving}
        onCancel={closeCanaryTaskModal}
        onSave={saveCanaryTask}
        t={t}
      />
      <RecommendationDetailModal
        visible={!!selectedRecommendation}
        snapshot={selectedRecommendation}
        decisionSaving={decisionSaving}
        onCancel={() => setSelectedRecommendation(null)}
        onCopy={copyRecommendationPayload}
        onCreateCanaryTask={openCanaryTaskModal}
        onDecisionSave={saveRecommendationDecision}
        t={t}
      />
    </div>
  );
}
