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

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { unzipSync, strFromU8 } from 'fflate';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import {
  Banner,
  Button,
  Checkbox,
  Empty,
  Input,
  SideSheet,
  Modal,
  Popconfirm,
  Select,
  Skeleton,
  Space,
  Table,
  Tabs,
  Tag,
  TextArea,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  AlertTriangle,
  BarChart3,
  ArrowLeft,
  BadgeCheck,
  Clock3,
  FileArchive,
  FileText,
  FileUp,
  Fingerprint,
  Gauge,
  KeyRound,
  ListChecks,
  Pencil,
  Plus,
  PlugZap,
  Search,
  Server,
  RefreshCw,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  ToggleLeft,
  ToggleRight,
  UploadCloud,
  UserRoundCog,
  XCircle,
} from 'lucide-react';
import { API, showError, showInfo, showSuccess, timestamp2string } from '../../helpers';
import { renderQuota } from '../../helpers/render';
import ProxyEditorModal from '../../components/model-gateway/ProxyEditorModal';
import './channel-account.css';

const { Text } = Typography;
const CHANNEL_ACCOUNT_IMPORT_FILE_LIMIT = 32 * 1024 * 1024;
const CHANNEL_ACCOUNT_IMPORT_FILE_ACCEPT =
  '.zip,.json,.txt,.ndjson,application/zip,application/json,text/plain';
const XAUTO_NEWAPI_PACKAGE_TYPE = 'newapi-channel-files';
const CHANNEL_ACCOUNT_RECONCILE_CACHE_TTL_MS = 30 * 1000;

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function formatNumber(value) {
  return new Intl.NumberFormat().format(Number(value) || 0);
}

function formatCompactNumber(value) {
  const numeric = Number(value || 0);
  if (Math.abs(numeric) >= 1000000) {
    return `${(numeric / 1000000).toFixed(1).replace(/\.0$/, '')}M`;
  }
  if (Math.abs(numeric) >= 1000) {
    return `${(numeric / 1000).toFixed(1).replace(/\.0$/, '')}K`;
  }
  return formatNumber(numeric);
}

function formatPercent(value, digits = 1) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return '--';
  return `${(numeric * 100).toFixed(digits)}%`;
}

function formatScore(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return '--';
  return numeric.toFixed(3).replace(/0+$/, '').replace(/\.$/, '');
}

function formatLatency(value) {
  const latency = Number(value || 0);
  if (latency <= 0) return '--';
  if (latency >= 1000) return `${(latency / 1000).toFixed(2)}s`;
  return `${Math.round(latency)}ms`;
}

function formatTimestamp(timestamp) {
  return Number(timestamp || 0) > 0
    ? timestamp2string(Number(timestamp))
    : '--';
}

function formatCost(value) {
  const numeric = Number(value || 0);
  if (!Number.isFinite(numeric) || numeric <= 0) return 'US$0.00';
  return `US$${numeric.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}`;
}

function formatQuotaValue(value) {
  const quota = Number(value || 0);
  if (quota <= 0) return renderQuota(0, 2);
  return renderQuota(quota, 2);
}

function shortRequestId(value) {
  const text = String(value || '').trim();
  if (!text) return '--';
  if (text.length <= 18) return text;
  return `${text.slice(0, 10)}...${text.slice(-6)}`;
}

function attemptDisplayIndex(value) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric < 0) return 1;
  return Math.floor(numeric) + 1;
}

function accountDisplayIndex(item) {
  const explicit = Number(item?.account_display_index);
  if (Number.isFinite(explicit) && explicit > 0) return explicit;
  const credentialIndex = Number(item?.credential_index);
  if (Number.isFinite(credentialIndex) && credentialIndex >= 0) {
    return Math.floor(credentialIndex) + 1;
  }
  return '--';
}

function statisticsDiagnosticText(item, t) {
  const diagnostic = item?.statistics_diagnostic || item?.statistics_status || '';
  const labels = {
    health_probe_excluded: t('探活不计入真实统计'),
    missing_account_attribution: t('缺少账号归因'),
    dispatch_record_only: t('仅调度记录'),
    waiting_for_billing: t('等待结算数据'),
    waiting_for_cost: t('等待成本计算'),
    statistics_complete: t('统计完整'),
    health_probe: t('探活不计入真实统计'),
    attribution_missing: t('缺少账号归因'),
    dispatch_only: t('仅调度记录'),
    billing_pending: t('等待结算数据'),
    cost_pending: t('等待成本计算'),
    complete: t('统计完整'),
  };
  return labels[diagnostic] || (item?.statistics_recorded ? t('已有请求状态') : t('未写入统计'));
}

function statisticsDiagnosticColor(item) {
  const status = item?.statistics_status || '';
  if (status === 'complete') return 'green';
  if (status === 'health_probe') return 'cyan';
  if (status === 'billing_pending' || status === 'cost_pending') return 'orange';
  if (status === 'attribution_missing') return 'red';
  if (status === 'dispatch_only') return 'grey';
  return item?.statistics_recorded ? 'green' : 'grey';
}

function reconcileCheckLabel(key, t) {
  const labels = {
    usage_event: t('用量事件'),
    account_match: t('账号匹配'),
    statistics: t('统计状态'),
    user_request: t('最终请求摘要'),
    samples: t('调度/评分样本'),
    cost: t('成本摘要'),
  };
  return labels[key] || key || '--';
}

function reconcileCheckText(check, t) {
  const detailLabels = {
    usage_event_found: t('已找到'),
    usage_event_missing: t('未找到'),
    account_match: t('账号匹配'),
    account_mismatch: t('账号不匹配'),
    statistics_complete: t('统计完整'),
    health_probe_excluded: t('探活不计入真实统计'),
    missing_account_attribution: t('缺少账号归因'),
    dispatch_record_only: t('仅调度记录'),
    waiting_for_billing: t('等待结算数据'),
    waiting_for_cost: t('等待成本计算'),
    user_request_found: t('已找到'),
    user_request_missing: t('未找到'),
    attempt_samples_found: t('已找到'),
    attempt_samples_missing: t('未找到'),
    cost_summary_found: t('已找到'),
    cost_summary_missing: t('等待成本计算'),
  };
  return detailLabels[check?.detail] || check?.detail || check?.status || '--';
}

function reconcileCheckColor(status) {
  if (status === 'ok' || status === 'complete') return 'green';
  if (status === 'warning' || status === 'billing_pending' || status === 'cost_pending') return 'orange';
  if (status === 'missing' || status === 'attribution_missing') return 'red';
  if (status === 'health_probe') return 'cyan';
  return 'grey';
}

function reconcileDiagnosisTitle(key, t) {
  const labels = {
    trace_complete: t('统计链路完整'),
    usage_event_missing_but_samples_exist: t('统计未写入，但存在调度或评分样本'),
    request_trace_missing: t('请求链路缺失'),
    account_mismatch: t('账号不匹配'),
    health_probe_excluded: t('这是探活样本，不计入真实请求统计'),
    account_attribution_missing: t('账号归因缺失，统计可能无法挂到账号'),
    dispatch_only: t('只有调度记录，等待 attempt 或结算写入'),
    billing_pending: t('等待结算数据写入'),
    cost_pending: t('等待成本计算完成'),
    user_request_summary_missing: t('最终请求摘要缺失'),
    request_failed: t('请求最终失败'),
    attempt_samples_missing: t('缺少调度或评分样本'),
    cost_summary_pending: t('等待成本计算完成'),
  };
  return labels[key] || key || '--';
}

function reconcileDiagnosisSuggestion(key, t) {
  const labels = {
    trace_complete: t('这条请求的调度、统计和成本链路已经对齐。'),
    usage_event_missing_but_samples_exist: t('检查 usage event 写入链路，重点看 request_id 归因和异步 recorder。'),
    request_trace_missing: t('检查请求是否经过智能网关，以及 request_id 是否在各阶段传递。'),
    account_mismatch: t('检查本次请求的账号索引与最终写入的 credential_index。'),
    health_probe_excluded: t('无需处理；探活只影响健康状态，不计入真实用量统计。'),
    account_attribution_missing: t('检查账号归因写入，尤其是 account_identity_key 和 credential_fingerprint。'),
    dispatch_only: t('等待 attempt/billing 写入；若长期停留，检查 recorder 或请求中断路径。'),
    billing_pending: t('检查 billing 写入链路和消费日志是否按 request_id 合并。'),
    cost_pending: t('等待异步成本任务，或检查成本 worker 与成本配置。'),
    user_request_summary_missing: t('检查最终请求摘要写入，确认 attempt 是否被标记为最终状态。'),
    request_failed: t('优先查看失败错误、重试链路和上游状态。'),
    attempt_samples_missing: t('检查调度/评分采样是否开启，以及该请求是否绕过智能调度。'),
    cost_summary_pending: t('等待异步成本任务，或检查成本 worker 与成本配置。'),
  };
  return labels[key] || '';
}

function reconcileDiagnosisColor(severity) {
  if (severity === 'ok') return 'green';
  if (severity === 'error') return 'red';
  if (severity === 'warning') return 'orange';
  if (severity === 'info') return 'cyan';
  return 'grey';
}

function pluralCount(value) {
  return Number(value || 0);
}

function buildCredentialTypeOptions(t) {
  return [
    { value: 'auto', label: t('自动识别') },
    { value: 'api_key', label: t('API Key') },
    { value: 'json_auth', label: t('JSON 授权') },
    { value: 'oauth_account', label: t('OAuth 账号') },
    { value: 'token_key', label: t('Token Key') },
    { value: 'session_cookie', label: t('Session Cookie') },
    { value: 'composite', label: t('组合凭证') },
  ];
}

function formatFileSize(bytes) {
  const size = Number(bytes || 0);
  if (size >= 1024 * 1024) return `${(size / 1024 / 1024).toFixed(2)} MB`;
  if (size >= 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${size} B`;
}

function uploadedFileInstance(item) {
  return item?.fileInstance || item?.originFileObj || item?.file || null;
}

function uploadedFileName(item, fallback = 'unnamed') {
  return item?.name || uploadedFileInstance(item)?.name || fallback;
}

function uploadedFileSize(item) {
  return Number(item?.size || uploadedFileInstance(item)?.size || 0);
}

function importFileLooksLikeZip(item) {
  return uploadedFileName(item).toLowerCase().endsWith('.zip');
}

function importFileNameLower(item) {
  return uploadedFileName(item).toLowerCase();
}

function parseImportJSON(text) {
  return JSON.parse(String(text || '').trim());
}

function extractCredentialsFromImportLines(text) {
  return String(text || '')
    .replaceAll('\r\n', '\n')
    .split('\n')
    .flatMap((line) => {
      const trimmed = line.trim();
      if (!trimmed) {
        return [];
      }
      if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
        try {
          return extractCredentialsFromImportJSON(
            parseImportJSON(trimmed),
            false,
          );
        } catch (err) {
          return [trimmed];
        }
      }
      return [trimmed];
    });
}

function extractCredentialsFromImportJSON(value, xautoPackage = false) {
  if (typeof value === 'string') {
    const trimmed = value.trim();
    return trimmed ? [trimmed] : [];
  }
  if (Array.isArray(value)) {
    return value.flatMap((item) =>
      extractCredentialsFromImportJSON(item, xautoPackage),
    );
  }
  if (!value || typeof value !== 'object') {
    return [];
  }
  if (typeof value.channel?.key === 'string' && value.channel.key.trim()) {
    return [value.channel.key.trim()];
  }
  if (value.type === XAUTO_NEWAPI_PACKAGE_TYPE) {
    return [];
  }
  if (value.credential_list !== undefined) {
    return extractCredentialsFromImportJSON(
      value.credential_list,
      xautoPackage,
    );
  }
  if (value.accounts !== undefined) {
    return extractCredentialsFromImportJSON(value.accounts, xautoPackage);
  }
  if (value.credentials !== undefined) {
    return extractCredentialsFromImportJSON(value.credentials, xautoPackage);
  }
  if (typeof value.credential === 'string' && value.credential.trim()) {
    return [value.credential.trim()];
  }
  if (typeof value.key === 'string' && value.key.trim()) {
    return [value.key.trim()];
  }
  return xautoPackage ? [] : [JSON.stringify(value)];
}

function isXAutoNewAPIZipEntries(entries) {
  const manifestName = Object.keys(entries || {}).find(
    (name) => name.toLowerCase().split('/').pop() === 'manifest.json',
  );
  if (!manifestName) {
    return false;
  }
  try {
    const manifest = parseImportJSON(strFromU8(entries[manifestName]));
    return (
      String(manifest?.type || '').trim().toLowerCase() ===
      XAUTO_NEWAPI_PACKAGE_TYPE
    );
  } catch (err) {
    return false;
  }
}

async function credentialsFromXAutoZipFile(file) {
  const buffer = await file.arrayBuffer();
  const entries = unzipSync(new Uint8Array(buffer));
  if (!isXAutoNewAPIZipEntries(entries)) {
    return null;
  }
  return Object.entries(entries).flatMap(([name, bytes]) => {
    const baseName = name.toLowerCase().split('/').pop();
    if (
      !baseName ||
      baseName === 'manifest.json' ||
      !baseName.endsWith('.json')
    ) {
      return [];
    }
    return extractCredentialsFromImportJSON(
      parseImportJSON(strFromU8(bytes)),
      true,
    );
  });
}

async function credentialsFromTextImportFile(file, item) {
  const name = importFileNameLower(item);
  if (
    !name.endsWith('.json') &&
    !name.endsWith('.txt') &&
    !name.endsWith('.ndjson')
  ) {
    return null;
  }
  const text = await file.text();
  const trimmed = text.trim();
  if (!trimmed) {
    return [];
  }
  if (name.endsWith('.ndjson')) {
    return extractCredentialsFromImportLines(trimmed);
  }
  if (
    name.endsWith('.json') ||
    trimmed.startsWith('{') ||
    trimmed.startsWith('[')
  ) {
    return extractCredentialsFromImportJSON(parseImportJSON(trimmed), false);
  }
  return extractCredentialsFromImportLines(trimmed);
}

async function credentialsFromImportFile(item) {
  const file = uploadedFileInstance(item);
  if (!file) {
    return null;
  }
  if (importFileLooksLikeZip(item)) {
    return credentialsFromXAutoZipFile(file);
  }
  return credentialsFromTextImportFile(file, item);
}

class ChannelAccountImportFileQueue {
  constructor(items = []) {
    this.items = items;
  }

  static fromFiles(files) {
    return new ChannelAccountImportFileQueue(
      Array.from(files || [])
        .filter(Boolean)
        .map((file) => ({
          uid: `${file.name || 'file'}-${file.size || 0}-${file.lastModified || Date.now()}`,
          name: file.name,
          size: file.size,
          fileInstance: file,
          status: 'success',
        })),
    );
  }

  oversized(maxSize) {
    return this.items.filter((item) => uploadedFileSize(item) > maxSize);
  }

  withinSize(maxSize) {
    return new ChannelAccountImportFileQueue(
      this.items.filter((item) => uploadedFileSize(item) <= maxSize),
    );
  }

  append(queue) {
    const merged = [...this.items];
    const seen = new Set(merged.map((item) => item.uid));
    queue.items.forEach((item) => {
      if (!seen.has(item.uid)) {
        seen.add(item.uid);
        merged.push(item);
      }
    });
    return new ChannelAccountImportFileQueue(merged);
  }
}

class ChannelAccountImportSubmission {
  constructor({ credentials, files, onlyNew }) {
    this.credentials = stringsTrim(credentials);
    this.files = files || [];
    this.onlyNew = Boolean(onlyNew);
  }

  hasInput() {
    return this.credentials.length > 0 || this.files.length > 0;
  }

  async payload() {
    const parsedFiles = await this.parseSupportedFiles();
    if (parsedFiles.parsedCount > 0 && parsedFiles.unparsedFiles.length === 0) {
      return {
        body: {
          credentials: this.credentials,
          credential_list: parsedFiles.credentials,
          only_new: this.onlyNew,
        },
        config: undefined,
      };
    }

    if (this.files.length === 0) {
      return {
        body: {
          credentials: this.credentials,
          only_new: this.onlyNew,
        },
        config: undefined,
      };
    }

    const form = new FormData();
    form.append('credentials', this.credentials);
    form.append('only_new', String(this.onlyNew));
    parsedFiles.credentials.forEach((credential) => {
      form.append('credential_list', credential);
    });
    parsedFiles.unparsedFiles.forEach((item) => {
      const file = uploadedFileInstance(item);
      if (file) {
        form.append('files', file, uploadedFileName(item));
      }
    });
    return {
      body: form,
      config: undefined,
    };
  }

  async parseSupportedFiles() {
    const credentials = [];
    const unparsedFiles = [];
    let parsedCount = 0;
    for (const item of this.files) {
      const parsedCredentials = await credentialsFromImportFile(item);
      if (parsedCredentials === null) {
        unparsedFiles.push(item);
        continue;
      }
      parsedCount++;
      credentials.push(...parsedCredentials);
    }
    return { credentials, parsedCount, unparsedFiles };
  }
}

function stringsTrim(value) {
  return String(value || '').trim();
}

function operationMessage(operation, t, fallback) {
  if (!operation) return fallback;
  if (operation.type === 'proxy') {
    return operation.action === 'clear' ? t('账号代理已解绑') : t('账号代理已绑定');
  }
  if (operation.type === 'credential') {
    return t('账号凭证已更新');
  }
  if (operation.type === 'import') {
    const parts = [
      t('新增 {{total}} 个账号', { total: pluralCount(operation.added) }),
    ];
    if (pluralCount(operation.skipped_existing) > 0) {
      parts.push(
        t('跳过已有 {{total}} 个', {
          total: pluralCount(operation.skipped_existing),
        }),
      );
    }
    if (pluralCount(operation.skipped_duplicate) > 0) {
      parts.push(
        t('跳过重复输入 {{total}} 个', {
          total: pluralCount(operation.skipped_duplicate),
        }),
      );
    }
    if (operation.channel_restored) {
      parts.push(t('渠道已恢复启用'));
    }
    return parts.join(t('、'));
  }
  if (operation.type === 'delete') {
    const parts = [
      t('已删除 {{total}} 个账号', {
        total: pluralCount(operation.deleted || operation.affected),
      }),
    ];
    if (operation.channel_disabled) {
      parts.push(t('渠道已自动禁用'));
    }
    if (operation.channel_restored) {
      parts.push(t('渠道已恢复启用'));
    }
    return parts.join(t('、'));
  }
  if (operation.type === 'status') {
    const changed = pluralCount(operation.affected);
    const parts = [
      operation.action === 'enable'
        ? t('已启用 {{total}} 个账号', { total: changed })
        : t('已禁用 {{total}} 个账号', { total: changed }),
    ];
    if (operation.channel_disabled) {
      parts.push(t('渠道已自动禁用'));
    }
    if (operation.channel_restored) {
      parts.push(t('渠道已恢复启用'));
    }
    return parts.join(t('、'));
  }
  if (operation.type === 'proxy') {
    const changed = pluralCount(operation.affected);
    return operation.action === 'bind'
      ? t('已设置 {{total}} 个账号代理', { total: changed })
      : t('已解绑 {{total}} 个账号代理', { total: changed });
  }
  return fallback;
}

function findAccountItem(payload, fallbackRecord) {
  const index = Number(fallbackRecord?.credential_index);
  const channelID = Number(fallbackRecord?.channel_id);
  const items = Array.isArray(payload?.items) ? payload.items : [];
  return (
    items.find(
      (item) =>
        Number(item?.credential_index) === index &&
        (!channelID || Number(item?.channel_id) === channelID),
    ) ||
    items.find((item) => Number(item?.credential_index) === index) ||
    fallbackRecord
  );
}

function runtimeKeyLabel(runtimeKey, t) {
  if (!runtimeKey) return '--';
  const parts = [
    runtimeKey.requested_model,
    runtimeKey.group,
    runtimeKey.endpoint_type,
    runtimeKey.upstream_model &&
    runtimeKey.upstream_model !== runtimeKey.requested_model
      ? runtimeKey.upstream_model
      : '',
  ].filter(Boolean);
  return parts.length > 0 ? parts.join(' / ') : t('渠道级快照');
}

function runtimeCapabilitySummary(value) {
  const raw = String(value || '').trim();
  if (!raw) return '';
  const parts = [];
  if (raw.includes('openai_codex')) parts.push('openai_codex');
  if (raw.includes('native_responses')) parts.push('native');
  if (raw.includes('"responses_compact":true')) parts.push('compact');
  if (raw.includes('"codex_image_tool":true')) parts.push('image');
  if (raw.includes('"codex_image_tool":false')) parts.push('no-image');
  if (parts.length > 0) return parts.join(' / ');
  return raw.length > 18 ? `${raw.slice(0, 18)}...` : raw;
}

function healthTagMeta(status, t) {
  switch (status) {
    case 'circuit_open':
      return { color: 'red', label: t('熔断打开') };
    case 'cooldown':
      return { color: 'orange', label: t('冷却') };
    case 'failure_avoidance':
      return { color: 'orange', label: t('恢复中') };
    case 'queued':
      return { color: 'cyan', label: t('队列中') };
    case 'high_pressure':
    case 'saturated':
      return { color: 'orange', label: t('并发压力') };
    case 'degraded':
      return { color: 'orange', label: t('降级') };
    case 'healthy':
      return { color: 'green', label: t('健康') };
    default:
      return { color: 'grey', label: status || t('暂无评分') };
  }
}

function statusTag(record, t) {
  if (record?.key_enabled) {
    return (
      <Tag color='green' type='light' shape='circle'>
        {t('已启用')}
      </Tag>
    );
  }
  return (
    <Tooltip content={record?.disabled_reason || t('未启用')}>
      <Tag color='red' type='light' shape='circle'>
        {t('已禁用')}
      </Tag>
    </Tooltip>
  );
}

function proxyLabel(proxy, t) {
  if (!proxy) return t('未绑定代理');
  return proxy.name || proxy.masked_address || `Proxy #${proxy.id}`;
}

function capabilityTag(label, value) {
  if (value === true) {
    return (
      <Tag color='green' type='light' shape='circle' size='small'>
        {label}
      </Tag>
    );
  }
  if (value === false) {
    return (
      <Tag color='red' type='light' shape='circle' size='small'>
        {label}
      </Tag>
    );
  }
  return (
    <Tag color='grey' type='light' shape='circle' size='small'>
      {label}
    </Tag>
  );
}

function classificationMeta(value, t) {
  switch (value) {
    case 'codex_compact_available':
      return { color: 'green', label: t('Codex/Compact 可用') };
    case 'codex_backend_available':
      return { color: 'green', label: t('Codex 可用') };
    case 'stream_only':
      return { color: 'cyan', label: t('仅支持流式') };
    case 'platform_quota_insufficient':
      return { color: 'orange', label: t('Platform 余额不足') };
    case 'platform_responses_scope_missing':
      return { color: 'orange', label: t('Platform Responses 权限不足') };
    case 'account_usage_limited':
      return { color: 'orange', label: t('账号用量限制中') };
    case 'proxy_error':
      return { color: 'red', label: t('代理异常') };
    case 'auth_error':
      return { color: 'red', label: t('授权异常') };
    case 'region_error':
      return { color: 'red', label: t('区域异常') };
    case 'unknown':
      return { color: 'grey', label: t('未知') };
    default:
      return { color: 'grey', label: value || t('未分类') };
  }
}

function schedulingReasonMeta(value, t) {
  switch (value) {
    case 'schedulable':
      return { color: 'green', label: t('可参与调度') };
    case 'account_disabled':
      return { color: 'red', label: t('账号已禁用') };
    case 'account_usage_limited':
      return { color: 'orange', label: t('账号用量限制中') };
    case 'proxy_error':
      return { color: 'red', label: t('代理异常') };
    case 'codex_stream_unavailable':
      return { color: 'red', label: t('Codex Stream 不可用') };
    case 'codex_compact_unavailable':
      return { color: 'orange', label: t('Compact 不可用') };
    case 'auth_error':
      return { color: 'red', label: t('授权异常') };
    case 'config_error_isolated':
      return { color: 'red', label: t('配置异常隔离') };
    case 'probe_recovery_pending':
      return { color: 'orange', label: t('等待恢复探活') };
    case 'concurrency_full':
      return { color: 'orange', label: t('并发已满') };
    case 'queue_full':
      return { color: 'orange', label: t('队列已满') };
    case 'no_score_sample':
      return { color: 'grey', label: t('暂无评分样本') };
    case 'no_runtime_snapshot':
      return { color: 'grey', label: t('暂无运行态') };
    case 'proxy_unavailable':
      return { color: 'red', label: t('代理不可用') };
    case 'proxy_disabled':
      return { color: 'orange', label: t('代理未启用') };
    default:
      return { color: 'grey', label: value || t('未知原因') };
  }
}

function effectiveCapabilityClassification(capabilities) {
  if (!capabilities) return '';
  if (
    capabilities.usage_limit_status === 'limited' &&
    (!capabilities.usage_limit_expires_at ||
      capabilities.usage_limit_expires_at > Math.floor(Date.now() / 1000))
  ) {
    return 'account_usage_limited';
  }
  if (capabilities.codex_backend_responses_stream_write === true) {
    return capabilities.codex_backend_compact_write === true
      ? 'codex_compact_available'
      : 'codex_backend_available';
  }
  if (capabilities.proxy_last_error) {
    return 'proxy_error';
  }
  return capabilities.capability_classification || '';
}

function CapabilitiesCell({ capabilities, t }) {
  if (!capabilities || !capabilities.checked_time) {
    return <Text type='tertiary'>{t('未检测')}</Text>;
  }

  const classification = classificationMeta(
    effectiveCapabilityClassification(capabilities),
    t,
  );
  const proxyParts = [
    capabilities.proxy_id ? `Proxy #${capabilities.proxy_id}` : '',
    capabilities.proxy_exit_ip || '',
    capabilities.proxy_region || '',
  ].filter(Boolean);
  const content = (
    <div className='ct-channel-account-capability-tip'>
      <div>
        {t('检测时间')}: {timestamp2string(capabilities.checked_time)}
      </div>
      {capabilities.capability_probe_surface ? (
        <div>
          {t('检测口径')}: {capabilities.capability_probe_surface}
        </div>
      ) : null}
      {proxyParts.length > 0 ? (
        <div>
          {t('代理出口')}: {proxyParts.join(' / ')}
        </div>
      ) : null}
      {capabilities.proxy_last_error ? (
        <div>
          {t('代理错误')}: {capabilities.proxy_last_error}
        </div>
      ) : null}
      {capabilities.last_endpoint ? (
        <div>
          {t('最后检测端点')}: {capabilities.last_endpoint}
        </div>
      ) : null}
      {capabilities.last_message ? (
        <div>{capabilities.last_message}</div>
      ) : null}
      {capabilities.usage_limit_status === 'limited' ? (
        <>
          <div>
            {t('限流')}: {capabilities.usage_limit_reason || t('账号用量限制中')}
          </div>
          {capabilities.usage_limit_expires_at ? (
            <div>
              {t('预计恢复')}: {timestamp2string(capabilities.usage_limit_expires_at)}
            </div>
          ) : null}
          {capabilities.usage_limit_message ? (
            <div>{capabilities.usage_limit_message}</div>
          ) : null}
        </>
      ) : null}
    </div>
  );

  return (
    <Tooltip content={content}>
      <div className='ct-channel-account-capability-stack'>
        <Tag color={classification.color} type='light' shape='circle' size='small'>
          {classification.label}
        </Tag>
        <div className='ct-channel-account-capability-group'>
          <span>{t('Codex')}</span>
          <div className='ct-channel-account-capability-tags'>
            {capabilityTag('Stream', capabilities.codex_backend_responses_stream_write)}
            {capabilityTag('Compact', capabilities.codex_backend_compact_write)}
            {capabilityTag('Stream Only', capabilities.codex_backend_requires_stream)}
          </div>
        </div>
        <div className='ct-channel-account-capability-group'>
          <span>{t('Platform')}</span>
          <div className='ct-channel-account-capability-tags'>
            {capabilityTag('Chat', capabilities.platform_chat_completions_write)}
            {capabilityTag('Responses', capabilities.platform_responses_write)}
            {capabilityTag('Compact', capabilities.platform_responses_compact_write)}
          </div>
        </div>
      </div>
    </Tooltip>
  );
}

function proxyAddress(proxy) {
  if (!proxy) return '';
  return proxy.masked_address || proxy.address || '';
}

function normalizeBrand(value) {
  return String(value || '').trim().toLowerCase();
}

function proxyReuseRisks(proxy) {
  return Array.isArray(proxy?.reuse_risks) ? proxy.reuse_risks : [];
}

function proxyHasReuseRisk(proxy) {
  return proxyReuseRisks(proxy).length > 0;
}

function proxyBindingRisk(proxy, record) {
  if (!proxy || !record?.account_identity) return null;
  const brand = normalizeBrand(record.account_identity.brand);
  const subject = record.account_identity.credential_subject_fingerprint || '';
  const usages = Array.isArray(proxy.brand_usage) ? proxy.brand_usage : [];
  const sameBrandUsages = usages.filter((usage) => {
    const usageBrand = normalizeBrand(usage.brand || usage.provider);
    if (!brand || usageBrand !== brand) return false;
    if (!subject) return true;
    return usage.credential_subject_fingerprint !== subject;
  });
  if (sameBrandUsages.length === 0) return null;
  const distinctSubjects = new Set(
    sameBrandUsages
      .map((usage) => usage.credential_subject_fingerprint)
      .filter(Boolean),
  );
  return {
    brand: record.account_identity.brand || record.account_identity.provider,
    usageCount: sameBrandUsages.length,
    distinctSubjectCount: distinctSubjects.size || sameBrandUsages.length,
  };
}

function reuseRiskText(risk, t) {
  if (!risk) return '';
  return t('同品牌 {{brand}} 已有 {{total}} 个账号使用该代理', {
    brand: risk.brand || risk.provider || t('未知品牌'),
    total: Number(risk.distinct_subject_count || risk.distinctSubjectCount || risk.account_count || risk.usageCount || 0),
  });
}

function proxyReusePolicyLabel(policy, t) {
  switch (policy) {
    case 'confirm':
      return t('代理复用策略：二次确认');
    case 'block':
      return t('代理复用策略：禁止复用');
    case 'warn':
    default:
      return t('代理复用策略：仅提醒');
  }
}

function isProxyReuseConfirmRequiredMessage(message) {
  return String(message || '').includes('请确认后继续绑定');
}

function accountIsCodexOAuth(record) {
  const identity = record?.account_identity || {};
  const resource = record?.resource_ref || {};
  const values = [
    identity.account_type,
    identity.brand,
    identity.provider,
    resource.brand,
    resource.provider,
  ]
    .map((value) => String(value || '').toLowerCase())
    .join(' ');
  return values.includes('oauth') && values.includes('codex');
}

function summarizeAccountCapabilityError(message, t) {
  const raw = String(message || '').trim();
  const lower = raw.toLowerCase();
  if (
    lower.includes('insufficient_quota') ||
    lower.includes('exceeded your current quota')
  ) {
    return t('Platform API 额度不足或未开通计费；这不影响 Codex backend 调度。');
  }
  if (
    lower.includes('api.responses.write') ||
    lower.includes('missing scopes') ||
    lower.includes('insufficient permissions')
  ) {
    return t('Platform Responses API 权限不足；这不影响 Codex backend 调度。');
  }
  if (raw.length > 260) {
    return `${raw.slice(0, 260)}...`;
  }
  return raw;
}

function ProxyCell({ record, t, onOpenProxy, onOpenProxyEdit }) {
  const proxy = record?.proxy;
  if (!proxy) {
    return (
      <div className='ct-channel-account-proxy-cell'>
        <Tag color='grey' type='light' shape='circle'>
          {t('未绑定代理')}
        </Tag>
        <Button
          size='small'
          type='tertiary'
          theme='borderless'
          icon={<PlugZap size={14} />}
          onClick={() => onOpenProxy(record)}
          aria-label={t('绑定代理')}
        />
      </div>
    );
  }
  return (
    <div className='ct-channel-account-proxy-cell'>
      <button
        type='button'
        className='ct-channel-account-proxy-main ct-channel-account-proxy-edit-trigger'
        onClick={() => onOpenProxyEdit?.(proxy, record)}
        aria-label={t('编辑代理')}
      >
        <Tooltip
          content={
            <div className='ct-channel-account-proxy-tip'>
              <div>{proxyLabel(proxy, t)}</div>
              <div>{proxyAddress(proxy) || '--'}</div>
            </div>
          }
        >
          <Tag
            color={proxy.enabled ? 'cyan' : 'red'}
            type='light'
            shape='circle'
            prefixIcon={<Server size={12} />}
          >
            {proxyLabel(proxy, t)}
          </Tag>
        </Tooltip>
        {proxyHasReuseRisk(proxy) ? (
          <Tooltip content={reuseRiskText(proxyReuseRisks(proxy)[0], t)}>
            <Tag
              color='orange'
              type='light'
              shape='circle'
              prefixIcon={<AlertTriangle size={12} />}
            >
              {t('同品牌复用风险')}
            </Tag>
          </Tooltip>
        ) : null}
        <Text type='tertiary' ellipsis={{ showTooltip: true }}>
          {proxyAddress(proxy) || '--'}
        </Text>
      </button>
      <Button
        size='small'
        type='tertiary'
        theme='borderless'
        icon={<PlugZap size={14} />}
        onClick={() => onOpenProxy(record)}
        aria-label={t('修改代理')}
      />
    </div>
  );
}

function metricClass(score) {
  const value = Number(score || 0);
  if (value >= 0.85) return 'ct-channel-account-score-good';
  if (value > 0 && value < 0.65) return 'ct-channel-account-score-risk';
  return '';
}

function ScoreSummary({ score, t }) {
  if (!score) {
    return (
      <div className='ct-channel-account-score-empty'>
        <span>{t('暂无评分样本')}</span>
      </div>
    );
  }
  const meta = healthTagMeta(score.health_status, t);
  return (
    <div className='ct-channel-account-score-block'>
      <div className='ct-channel-account-score-head'>
        <span className={metricClass(score.score_total)}>
          {formatScore(score.score_total)}
        </span>
        <Tag color={meta.color} size='small' type='light' shape='circle'>
          {meta.label}
        </Tag>
      </div>
      <div className='ct-channel-account-score-grid'>
        <span>{t('成功率')}</span>
        <strong>{formatPercent(score.success_rate)}</strong>
        <span>{t('首包')}</span>
        <strong>{formatLatency(score.ttft_ms)}</strong>
        <span>{t('样本')}</span>
        <strong>{formatNumber(score.sample_count)}</strong>
        <span>{t('成本分')}</span>
        <strong>{formatScore(score.cost_item_score)}</strong>
      </div>
    </div>
  );
}

function accountStatsWindow(record, key) {
  return record?.stats?.[key] || {};
}

function AccountStatsBlock({ stats, t }) {
  const today = stats?.today || {};
  if (!today.requests) {
    return <Text type='tertiary'>{t('无统计数据')}</Text>;
  }
  return (
    <div className='ct-channel-account-stat-cell'>
      <div>
        <span>{t('请求')}</span>
        <strong>{formatCompactNumber(today.requests)}</strong>
      </div>
      <div>
        <span>Token</span>
        <strong>{formatCompactNumber(today.total_tokens)}</strong>
      </div>
      <div>
        <span>{t('账号成本')}</span>
        <strong className='ct-channel-account-money'>
          {formatCost(today.upstream_cost_total)}
        </strong>
      </div>
      <div>
        <span>{t('用户扣费')}</span>
        <strong>{formatQuotaValue(today.quota)}</strong>
      </div>
      <div>
        <span>{t('成功率')}</span>
        <strong>{formatPercent(today.success_rate)}</strong>
      </div>
      <div>
        <span>{t('超时率')}</span>
        <strong>{formatPercent(today.timeout_rate)}</strong>
      </div>
    </div>
  );
}

function WindowUsagePill({ label, window, t }) {
  const requests = Number(window?.requests || 0);
  const successRate = Number(window?.success_rate || 0);
  const errorLabel = window?.top_error_category || t('无异常');
  return (
    <div className='ct-channel-account-window-row'>
      <Tag color={label === '5h' ? 'violet' : 'teal'} type='light'>
        {label}
      </Tag>
      <div className='ct-channel-account-window-bar'>
        <span style={{ width: `${Math.min(100, Math.max(0, successRate * 100))}%` }} />
      </div>
      <strong>{formatPercent(successRate, 0)}</strong>
      <span>{formatCompactNumber(requests)} req</span>
      <span>{formatCompactNumber(window?.total_tokens || 0)}</span>
      <span>{formatQuotaValue(window?.quota || 0)}</span>
      <Text type='tertiary' ellipsis={{ showTooltip: true }}>
        {errorLabel}
      </Text>
    </div>
  );
}

function UsageWindowsBlock({ stats, t }) {
  return (
    <div className='ct-channel-account-window-stack'>
      <WindowUsagePill label='5h' window={stats?.last_5h || {}} t={t} />
      <WindowUsagePill label='7d' window={stats?.last_7d || {}} t={t} />
    </div>
  );
}

function usageLimitActive(capabilities) {
  return (
    capabilities?.usage_limit_status === 'limited' &&
    (!capabilities.usage_limit_expires_at ||
      capabilities.usage_limit_expires_at > Math.floor(Date.now() / 1000))
  );
}

function AccountUsageLimitTag({ record, t }) {
  const capabilities = record?.capabilities || {};
  const scheduling = record?.scheduling || {};
  const blockingReasons = Array.isArray(scheduling.blocking_reasons)
    ? scheduling.blocking_reasons
    : [];
  const active =
    usageLimitActive(capabilities) ||
    blockingReasons.includes('account_usage_limited');
  if (!active) {
    return null;
  }

  const recoveryAt =
    scheduling.recovery_at || capabilities.usage_limit_expires_at || 0;
  const recoverySource =
    scheduling.recovery_source || capabilities.usage_limit_reset_source || '';
  const reason =
    capabilities.usage_limit_reason ||
    scheduling.detail ||
    capabilities.usage_limit_message ||
    t('账号用量限制中');
  const content = (
    <div className='ct-channel-account-capability-tip'>
      <div>
        {t('限流')}: {reason}
      </div>
      {capabilities.usage_limit_message &&
      capabilities.usage_limit_message !== reason ? (
        <div>{capabilities.usage_limit_message}</div>
      ) : null}
      {recoveryAt ? (
        <div>
          {t('预计恢复')}: {formatTimestamp(recoveryAt)}
        </div>
      ) : null}
      {recoverySource ? (
        <div>
          {t('恢复来源')}: {recoverySource}
        </div>
      ) : null}
    </div>
  );

  return (
    <Tooltip content={content}>
      <Tag
        color='orange'
        type='light'
        shape='circle'
        size='small'
        prefixIcon={<AlertTriangle size={12} />}
      >
        {t('账号用量限制中')}
      </Tag>
    </Tooltip>
  );
}

function AccountDiagnosisBlock({ record, t }) {
  const capabilities = record?.capabilities || {};
  const score = record?.score || {};
  const probeState = record?.stats?.probe_recovery_state || {};
  const limited = usageLimitActive(capabilities);
  const disabled = record && !record.key_enabled;
  const probePending = Boolean(score.probe_recovery_pending || probeState.pending);
  const fallbackBlockingReasons = [
    disabled ? 'account_disabled' : '',
    limited ? 'account_usage_limited' : '',
    probePending ? 'probe_recovery_pending' : '',
  ].filter(Boolean);
  const scheduling = record?.scheduling || {
    schedulable: fallbackBlockingReasons.length === 0,
    primary_reason: fallbackBlockingReasons[0] || 'schedulable',
    blocking_reasons: fallbackBlockingReasons,
    warning_reasons: score.sample_count === 0 ? ['no_score_sample'] : [],
    recovery_at: capabilities.usage_limit_expires_at,
    recovery_source: capabilities.usage_limit_reset_source,
    probe_recovery_pending: probePending,
    probe_recovery_successes:
      score.probe_recovery_success_count ?? probeState.success_count ?? 0,
    probe_recovery_required: score.probe_recovery_required ?? probeState.required ?? 0,
    active_concurrency: score.active_concurrency,
    effective_concurrency_limit: score.effective_concurrency_limit,
    queue_depth: score.queue_depth,
    queue_capacity: score.queue_capacity,
    detail: capabilities.usage_limit_message || score.probe_trigger_reason,
  };
  const blockingReasons = Array.isArray(scheduling.blocking_reasons)
    ? scheduling.blocking_reasons
    : [];
  const warningReasons = Array.isArray(scheduling.warning_reasons)
    ? scheduling.warning_reasons
    : [];
  const primaryMeta = schedulingReasonMeta(
    scheduling.primary_reason || 'schedulable',
    t,
  );
  const conclusion = !scheduling.schedulable
    ? t('账号不可调度')
    : warningReasons.length > 0
      ? t('可调度但有提醒')
      : t('可参与调度');
  const probeCurrent =
    scheduling.probe_recovery_successes ??
    score.probe_recovery_success_count ??
    probeState.success_count ??
    0;
  const probeRequired =
    scheduling.probe_recovery_required ??
    score.probe_recovery_required ??
    probeState.required ??
    0;
  const activeConcurrency =
    scheduling.active_concurrency ?? score.active_concurrency ?? 0;
  const concurrencyCap =
    scheduling.effective_concurrency_limit ??
    score.effective_concurrency_limit ??
    0;
  const queueDepth = scheduling.queue_depth ?? score.queue_depth ?? 0;
  const queueCapacity = scheduling.queue_capacity ?? score.queue_capacity ?? 0;
  const classification = classificationMeta(
    scheduling.capability_classification ||
      effectiveCapabilityClassification(capabilities),
    t,
  );
  const recoveryAt =
    scheduling.recovery_at || capabilities.usage_limit_expires_at || 0;
  const recoverySource =
    scheduling.recovery_source || capabilities.usage_limit_reset_source || '';
  return (
    <div className='ct-channel-account-diagnosis'>
      <div className='ct-channel-account-diagnosis-card'>
        <span>{t('调度解释')}</span>
        <div className='ct-channel-account-reason-line'>
          <Tag color={primaryMeta.color} type='light' shape='circle'>
            {primaryMeta.label}
          </Tag>
          <strong>{conclusion}</strong>
        </div>
        <small>
          {scheduling.detail ||
          (score.health_status
            ? `${t('健康状态')}: ${healthTagMeta(score.health_status, t).label}`
            : t('暂无运行态'))}
        </small>
      </div>
      <div className='ct-channel-account-diagnosis-card'>
        <span>{t('阻塞与提醒')}</span>
        <div className='ct-channel-account-reason-tags'>
          {blockingReasons.length === 0 && warningReasons.length === 0 ? (
            <Tag color='green' size='small' type='light' shape='circle'>
              {t('无阻塞')}
            </Tag>
          ) : null}
          {blockingReasons.map((reason) => {
            const meta = schedulingReasonMeta(reason, t);
            return (
              <Tag
                color={meta.color}
                key={`blocking-${reason}`}
                size='small'
                type='light'
                shape='circle'
              >
                {meta.label}
              </Tag>
            );
          })}
          {warningReasons.map((reason) => {
            const meta = schedulingReasonMeta(reason, t);
            return (
              <Tag
                color={meta.color}
                key={`warning-${reason}`}
                size='small'
                type='light'
                shape='circle'
              >
                {meta.label}
              </Tag>
            );
          })}
        </div>
        <small>
          {classification.label
            ? `${t('能力分类')}: ${classification.label}`
            : t('能力不受 Platform API 失败影响')}
        </small>
      </div>
      <div className='ct-channel-account-diagnosis-card'>
        <span>{t('并发与队列')}</span>
        <strong>
          {t('并发')} {formatNumber(activeConcurrency)} /{' '}
          {concurrencyCap > 0 ? formatNumber(concurrencyCap) : t('不限')}
        </strong>
        <small>
          {t('队列')} {formatNumber(queueDepth)} /{' '}
          {queueCapacity > 0 ? formatNumber(queueCapacity) : t('不限')}
        </small>
      </div>
      <div className='ct-channel-account-diagnosis-card'>
        <span>{t('恢复与限制')}</span>
        <strong>
          {scheduling.probe_recovery_pending
            ? `${formatNumber(probeCurrent)} / ${formatNumber(probeRequired)}`
            : recoveryAt
              ? formatTimestamp(recoveryAt)
              : t('无需恢复探活')}
        </strong>
        <small>{score.probe_trigger_reason || probeState.reason || '--'}</small>
        {recoverySource ? (
          <small>
            {t('恢复来源')}: {recoverySource}
          </small>
        ) : null}
      </div>
      <div className='ct-channel-account-diagnosis-card'>
        <span>{t('最近样本')}</span>
        <strong>
          {formatNumber(score.real_sample_count_30m || 0)} /{' '}
          {formatNumber(score.sample_count || 0)}
        </strong>
        <small>
          {score.last_real_attempt_at
            ? `${t('最后真实请求')}: ${formatTimestamp(score.last_real_attempt_at)}`
            : t('暂无真实样本')}
        </small>
      </div>
    </div>
  );
}

function DispatchHealthBlock({ record, t }) {
  const score = record?.score;
  if (!score) {
    return (
      <div className='ct-channel-account-dispatch-cell'>
        <Text type='tertiary'>{t('暂无运行态')}</Text>
        <AccountUsageLimitTag record={record} t={t} />
      </div>
    );
  }
  const meta = healthTagMeta(score.health_status, t);
  return (
    <div className='ct-channel-account-dispatch-cell'>
      <div className='ct-channel-account-dispatch-head'>
        <Tag color={meta.color} type='light' shape='circle'>
          {meta.label}
        </Tag>
        <strong>{formatScore(score.score_total)}</strong>
      </div>
      <div className='ct-channel-account-dispatch-grid'>
        <span>{t('并发')}</span>
        <strong>
          {formatNumber(score.active_concurrency || 0)} /{' '}
          {formatNumber(score.effective_concurrency_limit || 0)}
        </strong>
        <span>{t('队列')}</span>
        <strong>
          {formatNumber(score.queue_depth || 0)} /{' '}
          {formatNumber(score.queue_capacity || 0)}
        </strong>
      </div>
      {score.probe_recovery_pending ? (
        <Tag color='orange' size='small' type='light' shape='circle'>
          {t('恢复样本 {{current}}/{{required}}', {
            current: score.probe_recovery_success_count || 0,
            required: score.probe_recovery_required || 0,
          })}
        </Tag>
      ) : null}
      <AccountUsageLimitTag record={record} t={t} />
    </div>
  );
}

function RuntimeKeysCell({ record, t }) {
  const keys = record?.runtime_keys || [];
  if (keys.length === 0) {
    return <Text type='tertiary'>{t('暂无运行态')}</Text>;
  }
  return (
    <div className='ct-channel-account-runtime-list'>
      {keys.map((item, index) => {
        const meta = healthTagMeta(item.health_status, t);
        const runtimeKey = item.runtime_key || {};
        const capability = runtimeCapabilitySummary(
          runtimeKey.capability_fingerprint,
        );
        return (
          <div
            className='ct-channel-account-runtime-item'
            key={`${runtimeKey.requested_model || 'channel'}-${runtimeKey.group || 'default'}-${runtimeKey.endpoint_type || 'endpoint'}-${runtimeKey.capability_fingerprint || index}`}
          >
            <div>
              <div className='ct-channel-account-runtime-title'>
                {runtimeKeyLabel(runtimeKey, t)}
              </div>
              <div className='ct-channel-account-runtime-meta'>
                {t('成功率')} {formatPercent(item.success_rate)} · {t('首包')}{' '}
                {formatLatency(item.ttft_ms)} · {t('样本')}{' '}
                {formatNumber(item.sample_count)}
              </div>
              {capability ? (
                <Tooltip content={runtimeKey.capability_fingerprint}>
                  <div className='ct-channel-account-runtime-detail'>
                    {t('能力')} {capability}
                  </div>
                </Tooltip>
              ) : null}
            </div>
            <Tag color={meta.color} size='small' type='light' shape='circle'>
              {formatScore(item.score_total)}
            </Tag>
          </div>
        );
      })}
    </div>
  );
}

function ProxyBindingEditor({
  t,
  currentProxy,
  proxyReusePolicy,
  createProxyInline,
  setCreateProxyInline,
  selectedProxyID,
  setSelectedProxyID,
  proxiesLoading,
  proxies,
  loadProxies,
  selectedProxyRisk,
  proxyForm,
  setProxyForm,
}) {
  const proxyExistingChecked = !createProxyInline;
  const proxyOptions = useMemo(() => {
    const options = [];
    const seen = new Set();
    if (currentProxy?.id) {
      options.push(currentProxy);
      seen.add(Number(currentProxy.id));
    }
    (proxies || []).forEach((proxy) => {
      const proxyID = Number(proxy?.id || 0);
      if (proxyID > 0 && !seen.has(proxyID)) {
        options.push(proxy);
        seen.add(proxyID);
      }
    });
    return options;
  }, [currentProxy, proxies]);

  return (
    <div className='ct-channel-account-proxy-editor'>
      <Banner
        type={proxyReusePolicy === 'block' ? 'warning' : 'info'}
        closeIcon={null}
        fullMode={false}
        description={`${proxyReusePolicyLabel(proxyReusePolicy, t)} · ${t('同品牌不同账号共用同一代理时按该策略处理')}`}
      />

      <div className='ct-channel-account-proxy-mode'>
        <Checkbox
          checked={proxyExistingChecked}
          onChange={(event) => {
            const checked = event.target.checked;
            setCreateProxyInline(!checked);
          }}
        >
          {t('选择已有代理')}
        </Checkbox>
        <Checkbox
          checked={createProxyInline}
          onChange={(event) => setCreateProxyInline(event.target.checked)}
        >
          {t('新增 SOCKS5 代理')}
        </Checkbox>
      </div>

      {!createProxyInline ? (
        <div className='ct-channel-account-proxy-existing'>
          <Select
            value={selectedProxyID}
            onChange={(value) => setSelectedProxyID(Number(value || 0))}
            loading={proxiesLoading}
            filter
            style={{ width: '100%' }}
            placeholder={t('选择代理，可留空解绑')}
          >
            <Select.Option value={0}>{t('不使用代理')}</Select.Option>
            {proxyOptions.map((proxy) => (
              <Select.Option key={proxy.id} value={proxy.id}>
                {proxyLabel(proxy, t)} · {proxyAddress(proxy) || '--'}
              </Select.Option>
            ))}
          </Select>
          <Button
            type='tertiary'
            theme='borderless'
            icon={<RefreshCw size={14} />}
            loading={proxiesLoading}
            onClick={loadProxies}
          >
            {t('刷新代理')}
          </Button>
          {selectedProxyRisk ? (
            <Banner
              type='warning'
              closeIcon={null}
              fullMode={false}
              description={reuseRiskText(selectedProxyRisk, t)}
            />
          ) : null}
        </div>
      ) : (
        <div className='ct-channel-account-proxy-form'>
          <Input
            value={proxyForm.name}
            onChange={(value) =>
              setProxyForm((prev) => ({ ...prev, name: value }))
            }
            placeholder={t('代理名称（可选）')}
          />
          <div className='ct-channel-account-proxy-row'>
            <Select
              value={proxyForm.protocol}
              onChange={(value) =>
                setProxyForm((prev) => ({ ...prev, protocol: value }))
              }
              className='ct-channel-account-proxy-protocol'
            >
              <Select.Option value='socks5'>SOCKS5</Select.Option>
              <Select.Option value='socks5h'>SOCKS5H</Select.Option>
              <Select.Option value='http'>HTTP</Select.Option>
              <Select.Option value='https'>HTTPS</Select.Option>
            </Select>
            <Input
              value={proxyForm.address}
              onChange={(value) =>
                setProxyForm((prev) => ({ ...prev, address: value }))
              }
              placeholder='127.0.0.1:1080'
            />
          </div>
          <div className='ct-channel-account-proxy-row'>
            <Input
              value={proxyForm.username}
              onChange={(value) =>
                setProxyForm((prev) => ({ ...prev, username: value }))
              }
              placeholder={t('代理用户名（可选）')}
            />
            <Input
              type='password'
              value={proxyForm.password}
              onChange={(value) =>
                setProxyForm((prev) => ({ ...prev, password: value }))
              }
              placeholder={t('代理密码（可选）')}
            />
          </div>
          <Input
            value={proxyForm.remark}
            onChange={(value) =>
              setProxyForm((prev) => ({ ...prev, remark: value }))
            }
            placeholder={t('备注（可选）')}
          />
        </div>
      )}
      <Text type='tertiary' size='small'>
        {t('代理会作为独立资源记录使用品牌和账号，后续可用于避免同品牌重复使用同一出口')}
      </Text>
    </div>
  );
}

function buildColumns(
  t,
  onToggleStatus,
  onDeleteAccount,
  onOpenEdit,
  onOpenProxy,
  onOpenProxyEdit,
  onTestAccount,
  onProbeCapability,
  onDiagnosePlatformCapability,
  statusLoadingKey,
  testingAccountKey,
  capabilityLoadingKey,
) {
  return [
    {
      title: t('账号'),
      dataIndex: 'credential_index',
      key: 'credential_index',
      sorter: true,
      width: 280,
      render: (_, record) => {
        const identity = record?.account_identity || {};
        return (
        <div className='ct-channel-account-identity'>
          <div className='ct-channel-account-avatar'>
            <UserRoundCog size={17} />
          </div>
          <div>
            <div className='ct-channel-account-name'>
              {identity.display_name ||
                `${t('账号')} #${record.credential_index + 1}`}
              {statusTag(record, t)}
              <AccountUsageLimitTag record={record} t={t} />
            </div>
            <div className='ct-channel-account-sub'>
              {t('凭证序号')} #{record.credential_index + 1}
            </div>
            {record.disabled_reason ? (
              <div className='ct-channel-account-warning'>
                {record.disabled_reason}
              </div>
            ) : null}
          </div>
        </div>
        );
      },
    },
    {
      title: t('品牌与凭证'),
      dataIndex: 'resource_ref',
      width: 260,
      render: (resource = {}, record) => {
        const identity = record.account_identity || {};
        return (
          <div className='ct-channel-account-meta-stack'>
            <Space spacing={6}>
              <Tag color='cyan' type='light' shape='circle'>
                {identity.brand || resource.brand || '--'}
              </Tag>
              <Tag color='blue' type='light' shape='circle'>
                {identity.account_type || '--'}
              </Tag>
            </Space>
            <div className='ct-channel-account-fp'>
              <Fingerprint size={13} />
              <span>{record.subject_short || '--'}</span>
              <span>/</span>
              <KeyRound size={13} />
              <span>{record.credential_short || '--'}</span>
            </div>
            <Text type='tertiary' ellipsis={{ showTooltip: true }}>
              {identity.account_id || '--'}
            </Text>
          </div>
        );
      },
    },
    {
      title: t('代理'),
      dataIndex: 'proxy',
      width: 250,
      render: (_, record) => (
        <ProxyCell
          record={record}
          t={t}
          onOpenProxy={onOpenProxy}
          onOpenProxyEdit={onOpenProxyEdit}
        />
      ),
    },
    {
      title: t('当前评分'),
      dataIndex: 'score',
      key: 'score',
      sorter: true,
      width: 240,
      render: (score) => <ScoreSummary score={score} t={t} />,
    },
    {
      title: t('运行键'),
      dataIndex: 'runtime_keys',
      width: 380,
      render: (_, record) => <RuntimeKeysCell record={record} t={t} />,
    },
    {
      title: t('账号能力'),
      dataIndex: 'capabilities',
      width: 320,
      render: (capabilities) => (
        <CapabilitiesCell capabilities={capabilities} t={t} />
      ),
    },
    {
      title: t('最近活动'),
      dataIndex: 'recent_activity',
      key: 'recent_activity',
      sorter: true,
      width: 260,
      render: (_, record) => {
        const score = record.score;
        if (!score) return <Text type='tertiary'>{t('暂无真实样本')}</Text>;
        return (
          <div className='ct-channel-account-time-grid'>
            <span>{t('最后真实请求')}</span>
            <strong>{formatTimestamp(score.last_real_attempt_at)}</strong>
            <span>{t('最后成功')}</span>
            <strong>{formatTimestamp(score.last_real_success_at)}</strong>
            <span>{t('最后探测')}</span>
            <strong>{formatTimestamp(score.last_probe_at)}</strong>
          </div>
        );
      },
    },
    {
      title: t('操作'),
      dataIndex: 'operation',
      width: 330,
      fixed: 'right',
      render: (_, record) => {
        const action = record?.key_enabled ? 'disable' : 'enable';
        const loadingKey = `${record.channel_id}-${record.credential_index}`;
        const loading = statusLoadingKey === loadingKey;
        const testing = testingAccountKey === loadingKey;
        const probing = capabilityLoadingKey === loadingKey;
        const platformDiagnosing = capabilityLoadingKey === `${loadingKey}:platform`;
        return (
          <Space className='ct-channel-account-operation' spacing={6}>
            <Popconfirm
              title={
                action === 'disable'
                  ? t('确定禁用该账号？')
                  : t('确定启用该账号？')
              }
              content={
                action === 'disable'
                  ? t('禁用后该账号不会参与智能调度')
                  : t('启用后该账号可重新参与智能调度')
              }
              onConfirm={() => onToggleStatus(record)}
            >
              <Button
                size='small'
                type={action === 'disable' ? 'warning' : 'primary'}
                theme={action === 'disable' ? 'light' : 'solid'}
                loading={loading}
                icon={
                  action === 'disable' ? (
                    <ToggleLeft size={14} />
                  ) : (
                    <ToggleRight size={14} />
                  )
                }
              >
                {action === 'disable' ? t('禁用账号') : t('启用账号')}
              </Button>
            </Popconfirm>
            <Tooltip content={t('测试账号')}>
              <Button
                size='small'
                type='tertiary'
                theme='light'
                icon={<Activity size={14} />}
                loading={testing}
                disabled={!record?.key_enabled}
                onClick={() => onTestAccount(record)}
                aria-label={t('测试账号')}
              >
                {t('测试')}
              </Button>
            </Tooltip>
            <Tooltip content={t('检测 Codex 能力')}>
              <Button
                size='small'
                type='tertiary'
                theme='borderless'
                icon={<Search size={14} />}
                loading={probing}
                onClick={() => onProbeCapability(record)}
                aria-label={t('检测 Codex 能力')}
              />
            </Tooltip>
            <Tooltip content={t('Platform API 诊断')}>
              <Button
                size='small'
                type='tertiary'
                theme='borderless'
                icon={<PlugZap size={14} />}
                loading={platformDiagnosing}
                onClick={() => onDiagnosePlatformCapability(record)}
                aria-label={t('Platform API 诊断')}
              />
            </Tooltip>
            <Tooltip content={t('编辑账号')}>
              <Button
                size='small'
                type='tertiary'
                theme='borderless'
                icon={<Pencil size={14} />}
                loading={loading}
                onClick={() => onOpenEdit(record)}
                aria-label={t('编辑账号')}
              />
            </Tooltip>
            <Popconfirm
              title={t('确定删除该账号？')}
              content={t('删除后该凭证将从渠道中移除，此操作不可撤销')}
              onConfirm={() => onDeleteAccount(record)}
            >
              <Button
                size='small'
                type='danger'
                theme='borderless'
                icon={<Trash2 size={14} />}
                loading={loading}
                aria-label={t('删除账号')}
              />
            </Popconfirm>
          </Space>
        );
      },
    },
  ];
}

function buildStatsColumns(
  t,
  onToggleStatus,
  onOpenDetail,
  statusLoadingKey,
) {
  return [
    {
      title: t('状态'),
      dataIndex: 'credential_index',
      key: 'credential_index',
      width: 270,
      render: (_, record) => {
        const identity = record?.account_identity || {};
        return (
        <div className='ct-channel-account-identity'>
          <div className='ct-channel-account-avatar'>
            <BarChart3 size={17} />
          </div>
          <div>
            <div className='ct-channel-account-name'>
              {identity.display_name ||
                `${t('账号')} #${record.credential_index + 1}`}
              {statusTag(record, t)}
              <AccountUsageLimitTag record={record} t={t} />
            </div>
            <div className='ct-channel-account-sub'>
              {identity.brand || record?.resource_ref?.brand || '--'} ·{' '}
              {t('凭证序号')} #{record.credential_index + 1}
            </div>
            {record.disabled_reason ? (
              <div className='ct-channel-account-warning'>
                {record.disabled_reason}
              </div>
            ) : null}
          </div>
        </div>
        );
      },
    },
    {
      title: t('调度'),
      dataIndex: 'score',
      width: 230,
      render: (_, record) => <DispatchHealthBlock record={record} t={t} />,
    },
    {
      title: t('今日统计'),
      dataIndex: 'today_requests',
      key: 'today_requests',
      sorter: true,
      width: 260,
      render: (_, record) => <AccountStatsBlock stats={record?.stats} t={t} />,
    },
    {
      title: t('用量窗口'),
      dataIndex: 'last_7d_requests',
      key: 'last_7d_requests',
      sorter: true,
      width: 470,
      render: (_, record) => <UsageWindowsBlock stats={record?.stats} t={t} />,
    },
    {
      title: t('最近活跃'),
      dataIndex: 'last_active_at',
      key: 'last_active_at',
      sorter: true,
      width: 190,
      render: (_, record) => {
        const stats = record?.stats || {};
        return (
        <div className='ct-channel-account-time-grid'>
          <span>{t('请求')}</span>
          <strong>{formatTimestamp(stats?.last_active_at)}</strong>
          <span>{t('主要异常')}</span>
          <strong>{stats?.main_error_category || t('无异常')}</strong>
        </div>
        );
      },
    },
    {
      title: t('操作'),
      dataIndex: 'operation',
      width: 210,
      fixed: 'right',
      render: (_, record) => {
        const action = record?.key_enabled ? 'disable' : 'enable';
        const loadingKey = `${record.channel_id}-${record.credential_index}`;
        return (
          <Space spacing={6}>
            <Popconfirm
              title={
                action === 'disable'
                  ? t('确定禁用该账号？')
                  : t('确定启用该账号？')
              }
              content={
                action === 'disable'
                  ? t('禁用后该账号不会参与智能调度')
                  : t('启用后该账号可重新参与智能调度')
              }
              onConfirm={() => onToggleStatus(record)}
            >
              <Button
                size='small'
                type={action === 'disable' ? 'warning' : 'primary'}
                theme='light'
                loading={statusLoadingKey === loadingKey}
                icon={
                  action === 'disable' ? (
                    <ToggleLeft size={14} />
                  ) : (
                    <ToggleRight size={14} />
                  )
                }
              >
                {action === 'disable' ? t('禁用') : t('启用')}
              </Button>
            </Popconfirm>
            <Button
              size='small'
              type='tertiary'
              theme='light'
              icon={<FileText size={14} />}
              onClick={() => onOpenDetail(record)}
            >
              {t('详情')}
            </Button>
          </Space>
        );
      },
    },
  ];
}

function DetailStatWindow({ title, window, t }) {
  return (
    <div className='ct-channel-account-detail-window'>
      <div className='ct-channel-account-detail-window-title'>{title}</div>
      <div className='ct-channel-account-detail-grid'>
        <span>{t('请求')}</span>
        <strong>{formatNumber(window?.requests)}</strong>
        <span>Token</span>
        <strong>{formatCompactNumber(window?.total_tokens)}</strong>
        <span>{t('成功率')}</span>
        <strong>{formatPercent(window?.success_rate)}</strong>
        <span>{t('账号成本')}</span>
        <strong>{formatCost(window?.upstream_cost_total)}</strong>
        <span>{t('用户扣费')}</span>
        <strong>{formatQuotaValue(window?.quota)}</strong>
        <span>{t('平均首包')}</span>
        <strong>{formatLatency(window?.avg_ttft_ms)}</strong>
        <span>{t('平均耗时')}</span>
        <strong>{formatLatency(window?.avg_duration_ms)}</strong>
        <span>{t('主要异常')}</span>
        <strong>{window?.top_error_category || t('无异常')}</strong>
      </div>
    </div>
  );
}

function RequestReconcileModal({ visible, data, loading, onClose, t }) {
  const usage = data?.usage_event || {};
  const userRequest = data?.user_request || {};
  const cost = data?.cost_summary || {};
  const executionRecords = Array.isArray(data?.execution_records)
    ? data.execution_records
    : [];
  const scoreEvents = Array.isArray(data?.score_events) ? data.score_events : [];
  const checks = Array.isArray(data?.checks) ? data.checks : [];
  const diagnoses = Array.isArray(data?.diagnoses) ? data.diagnoses : [];

  return (
    <Modal
      title={t('请求链路对账')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={760}
      bodyStyle={{ padding: 0 }}
    >
      <div className='ct-channel-account-reconcile'>
        {loading ? (
          <Skeleton placeholder={<Skeleton.Paragraph rows={6} />} loading active />
        ) : !data ? (
          <Empty title={t('暂无请求记录')} />
        ) : (
          <>
            <div className='ct-channel-account-reconcile-head'>
              <div>
                <span>{t('请求 ID')}</span>
                <Tooltip content={data.request_id || '--'}>
                  <strong>{shortRequestId(data.request_id)}</strong>
                </Tooltip>
              </div>
              <Tag color='blue'>
                {`${t('渠道')} #${data.channel_id || '--'} / ${t('账号')} #${data.account_display_index || '--'}`}
              </Tag>
            </div>
            {diagnoses.length > 0 ? (
              <div className='ct-channel-account-reconcile-diagnoses'>
                <div className='ct-channel-account-detail-title'>{t('诊断结论')}</div>
                {diagnoses.map((diagnosis) => (
                  <div key={`${diagnosis.key}-${diagnosis.severity}`}>
                    <Tag size='small' color={reconcileDiagnosisColor(diagnosis.severity)}>
                      {reconcileDiagnosisTitle(diagnosis.key, t)}
                    </Tag>
                    <span>{reconcileDiagnosisSuggestion(diagnosis.key, t)}</span>
                  </div>
                ))}
              </div>
            ) : null}
            <div className='ct-channel-account-reconcile-checks'>
              {checks.map((check) => (
                <div key={`${check.key}-${check.status}`}>
                  <span>{reconcileCheckLabel(check.key, t)}</span>
                  <Tag size='small' color={reconcileCheckColor(check.status)}>
                    {reconcileCheckText(check, t)}
                  </Tag>
                </div>
              ))}
            </div>
            <div className='ct-channel-account-reconcile-section'>
              <div className='ct-channel-account-detail-title'>{t('用量事件')}</div>
              <div className='ct-channel-account-detail-grid'>
                <span>{t('统计状态')}</span>
                <strong>{usage.statistics_diagnostic ? statisticsDiagnosticText(usage, t) : t('未找到')}</strong>
                <span>{t('请求类型')}</span>
                <strong>{usage.is_health_probe ? t('探活样本') : t('真实请求')}</strong>
                <span>Token</span>
                <strong>{formatCompactNumber(usage.total_tokens)}</strong>
                <span>{t('用户扣费')}</span>
                <strong>{formatQuotaValue(usage.quota)}</strong>
                <span>{t('首包/耗时')}</span>
                <strong>{formatLatency(usage.ttft_ms)} / {formatLatency(usage.duration_ms)}</strong>
                <span>{t('完成时间')}</span>
                <strong>{formatTimestamp(usage.completed_at)}</strong>
              </div>
            </div>
            <div className='ct-channel-account-reconcile-section'>
              <div className='ct-channel-account-detail-title'>{t('最终请求摘要')}</div>
              <div className='ct-channel-account-detail-grid'>
                <span>{t('结果')}</span>
                <strong>
                  {data.user_request
                    ? userRequest.final_success
                      ? t('成功')
                      : userRequest.final_error_category || userRequest.final_status_code || t('失败')
                    : t('未找到')}
                </strong>
                <span>{t('尝试次数')}</span>
                <strong>{formatNumber(userRequest.attempts)}</strong>
                <span>{t('最终渠道')}</span>
                <strong>{userRequest.final_channel_id ? `#${userRequest.final_channel_id}` : '--'}</strong>
                <span>{t('恢复成功')}</span>
                <strong>{userRequest.recovered ? t('是') : t('否')}</strong>
              </div>
            </div>
            <div className='ct-channel-account-reconcile-section'>
              <div className='ct-channel-account-detail-title'>{t('Attempt 样本')}</div>
              {executionRecords.length === 0 ? (
                <Empty title={t('未找到')} />
              ) : (
                <div className='ct-channel-account-reconcile-list'>
                  {executionRecords.map((item) => (
                    <div key={`${item.attempt_index}-${item.created_at}`}>
                      <span>
                        {t('第 {{index}} 次尝试', {
                          index: attemptDisplayIndex(item.attempt_index),
                        })}
                      </span>
                      <Tag size='small' color={item.success ? 'green' : 'red'}>
                        {item.success ? t('成功') : item.error_category || item.status_code || t('失败')}
                      </Tag>
                      <span>{formatLatency(item.ttft_ms)} / {formatLatency(item.duration_ms)}</span>
                      <span>{item.selected_reason || '--'}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <div className='ct-channel-account-reconcile-section'>
              <div className='ct-channel-account-detail-title'>{t('评分样本')}</div>
              {scoreEvents.length === 0 ? (
                <Empty title={t('未找到')} />
              ) : (
                <div className='ct-channel-account-reconcile-list'>
                  {scoreEvents.map((item) => (
                    <div key={`${item.attempt_index}-${item.created_at}`}>
                      <span>
                        {`${t('账号')} #${item.account_display_index || '--'}`}
                      </span>
                      <Tag size='small' color='teal'>
                        {formatScore(item.after_total)}
                      </Tag>
                      <span>{item.switch_reason || item.failure_scope || '--'}</span>
                      <span>{item.requested_model || '--'}</span>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <div className='ct-channel-account-reconcile-section'>
              <div className='ct-channel-account-detail-title'>{t('成本摘要')}</div>
              <div className='ct-channel-account-detail-grid'>
                <span>{t('模型')}</span>
                <strong>{cost.upstream_model || usage.requested_model || '--'}</strong>
                <span>{t('账号成本')}</span>
                <strong>{formatCost(cost.upstream_cost_total || usage.upstream_cost_total)}</strong>
                <span>{t('来源')}</span>
                <strong>{cost.cost_source || '--'}</strong>
                <span>{t('精度')}</span>
                <strong>{cost.cost_accuracy || '--'}</strong>
              </div>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}

function RecentRequestsBlock({ visible, record, onReload, t }) {
  const [items, setItems] = useState([]);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [refreshResult, setRefreshResult] = useState(null);
  const [reconcileVisible, setReconcileVisible] = useState(false);
  const [reconcileLoading, setReconcileLoading] = useState(false);
  const [reconcileData, setReconcileData] = useState(null);
  const reconcileCacheRef = useRef(new Map());
  const channelID = Number(record?.channel_id || 0);
  const credentialIndex = Number(record?.credential_index ?? -1);
  const recentSummary = useMemo(() => {
    const summary = {
      real: 0,
      probes: 0,
      errors: 0,
      rateLimited: 0,
      timeout: 0,
      latestError: null,
    };
    items.forEach((item) => {
      if (item.is_health_probe) summary.probes += 1;
      else summary.real += 1;
      if (!item.success) {
        summary.errors += 1;
        if (!summary.latestError) summary.latestError = item;
      }
      const errorText = `${item.error_category || ''} ${item.status_code || ''}`.toLowerCase();
      if (item.status_code === 429 || errorText.includes('rate_limit')) {
        summary.rateLimited += 1;
      }
      if (
        [408, 504, 524].includes(Number(item.status_code || 0)) ||
        errorText.includes('timeout')
      ) {
        summary.timeout += 1;
      }
    });
    return summary;
  }, [items]);

  const loadRecentRequests = useCallback(async () => {
    if (!visible || channelID <= 0 || credentialIndex < 0) {
      setItems([]);
      return;
    }
    setLoading(true);
    try {
      const response = await API.get(
        `/api/channel/${channelID}/accounts/${credentialIndex}/requests`,
        { disableDuplicate: true },
      );
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('请求异常'));
      }
      const payload = unwrapApiData(response);
      setItems(Array.isArray(payload?.items) ? payload.items : []);
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('请求异常');
      showError(message);
    } finally {
      setLoading(false);
    }
  }, [channelID, credentialIndex, t, visible]);

  useEffect(() => {
    loadRecentRequests();
  }, [loadRecentRequests]);

  const refreshAttribution = useCallback(async () => {
    if (channelID <= 0 || credentialIndex < 0) return;
    setRefreshing(true);
    try {
      const response = await API.post(
        `/api/channel/${channelID}/accounts/${credentialIndex}/refresh-attribution`,
        {},
      );
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('操作失败'));
      }
      const payload = unwrapApiData(response);
      setItems(Array.isArray(payload?.items) ? payload.items : []);
      setRefreshResult(payload?.refresh_result || null);
      reconcileCacheRef.current.clear();
      showSuccess(t('归因已刷新'));
      onReload?.();
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('操作失败');
      showError(message);
    } finally {
      setRefreshing(false);
    }
  }, [channelID, credentialIndex, onReload, t]);

  const openReconcile = useCallback(
    async (item) => {
      const requestID = String(item?.request_id || '').trim();
      if (channelID <= 0 || credentialIndex < 0 || !requestID) return;
      const cacheKey = `${channelID}:${credentialIndex}:${requestID}`;
      const cached = reconcileCacheRef.current.get(cacheKey);
      if (
        cached &&
        Date.now() - Number(cached.cachedAt || 0) <
          CHANNEL_ACCOUNT_RECONCILE_CACHE_TTL_MS
      ) {
        setReconcileVisible(true);
        setReconcileLoading(false);
        setReconcileData(cached.data);
        return;
      }
      setReconcileVisible(true);
      setReconcileLoading(true);
      setReconcileData(null);
      try {
        const response = await API.get(
          `/api/channel/${channelID}/accounts/${credentialIndex}/requests/${encodeURIComponent(requestID)}/reconcile`,
          { disableDuplicate: true },
        );
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('请求异常'));
        }
        const payload = unwrapApiData(response);
        reconcileCacheRef.current.set(cacheKey, {
          cachedAt: Date.now(),
          data: payload,
        });
        if (reconcileCacheRef.current.size > 50) {
          const firstKey = reconcileCacheRef.current.keys().next().value;
          if (firstKey) reconcileCacheRef.current.delete(firstKey);
        }
        setReconcileData(payload);
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('请求异常');
        showError(message);
        setReconcileVisible(false);
      } finally {
        setReconcileLoading(false);
      }
    },
    [channelID, credentialIndex, t],
  );

  return (
    <div className='ct-channel-account-recent'>
      <div className='ct-channel-account-recent-head'>
        <div>
          <div className='ct-channel-account-detail-title'>{t('最近10条请求')}</div>
          {refreshResult ? (
            <p>
              {t('已处理')} {formatNumber(refreshResult.scanned)} · {t('已更新')}{' '}
              {formatNumber(refreshResult.updated)}
            </p>
          ) : (
            <p>{t('仅重算最近6小时的统计归因')}</p>
          )}
        </div>
        <Button
          size='small'
          theme='light'
          type='primary'
          icon={<RefreshCw size={14} />}
          loading={refreshing}
          onClick={refreshAttribution}
        >
          {t('刷新统计归因')}
        </Button>
      </div>
      {loading ? (
        <Skeleton placeholder={<Skeleton.Paragraph rows={3} />} loading active />
      ) : items.length === 0 ? (
        <Empty title={t('暂无请求记录')} />
      ) : (
        <>
          <div className='ct-channel-account-recent-summary'>
            <div>
              <span>{t('真实请求')}</span>
              <strong>{formatNumber(recentSummary.real)}</strong>
            </div>
            <div>
              <span>{t('探活样本')}</span>
              <strong>{formatNumber(recentSummary.probes)}</strong>
            </div>
            <div>
              <span>{t('限流次数')}</span>
              <strong>{formatNumber(recentSummary.rateLimited)}</strong>
            </div>
            <div>
              <span>{t('超时次数')}</span>
              <strong>{formatNumber(recentSummary.timeout)}</strong>
            </div>
            <div className='ct-channel-account-recent-summary-wide'>
              <span>{t('最近异常')}</span>
              <strong>
                {recentSummary.latestError
                  ? recentSummary.latestError.error_category ||
                    recentSummary.latestError.status_code ||
                    t('失败')
                  : t('无异常')}
              </strong>
            </div>
          </div>
          <div className='ct-channel-account-recent-list'>
            {items.map((item) => (
              <div
                className='ct-channel-account-recent-item'
                key={item.request_id || `${item.completed_at}-${item.status_code}`}
              >
                <div className='ct-channel-account-recent-main'>
                  <Tooltip content={item.request_id || '--'}>
                    <strong>{shortRequestId(item.request_id)}</strong>
                  </Tooltip>
                  <span>{item.requested_model || '--'}</span>
                </div>
                <div className='ct-channel-account-recent-tags'>
                  <Tag size='small' color={item.attempt_index > 0 ? 'orange' : 'grey'}>
                    {t('第 {{index}} 次尝试', {
                      index: attemptDisplayIndex(item.attempt_index),
                    })}
                  </Tag>
                  <Tag size='small' color={item.is_health_probe ? 'cyan' : 'green'}>
                    {item.is_health_probe ? t('探活样本') : t('真实请求')}
                  </Tag>
                  <Tag size='small' color='blue'>
                    {`${t('渠道')} #${item.channel_id || '--'} / ${t('账号')} #${accountDisplayIndex(item)}`}
                  </Tag>
                  <Tag size='small' color={item.success ? 'green' : 'red'}>
                    {item.success ? t('成功') : item.error_category || item.status_code || t('失败')}
                  </Tag>
                  <Tag
                    size='small'
                    color={item.statistics_recorded ? 'green' : 'grey'}
                  >
                    {item.statistics_recorded ? t('写入统计') : t('未写入统计')}
                  </Tag>
                  <Tooltip
                    content={`${t('统计状态')}: ${statisticsDiagnosticText(item, t)}`}
                  >
                    <Tag size='small' color={statisticsDiagnosticColor(item)}>
                      {statisticsDiagnosticText(item, t)}
                    </Tag>
                  </Tooltip>
                  <Tag
                    size='small'
                    color={item.attribution_complete ? 'green' : 'orange'}
                  >
                    {item.attribution_complete ? t('归因完整') : t('归因缺失')}
                  </Tag>
                </div>
                <div className='ct-channel-account-recent-meta'>
                  <span>{formatTimestamp(item.completed_at)}</span>
                  <span>{formatLatency(item.ttft_ms)} / {formatLatency(item.duration_ms)}</span>
                  <span>{formatCompactNumber(item.total_tokens)} Token</span>
                  <span>{formatQuotaValue(item.quota)}</span>
                  <Tooltip content={t('请求链路对账')}>
                    <Button
                      size='small'
                      theme='borderless'
                      type='tertiary'
                      icon={<ListChecks size={14} />}
                      onClick={() => openReconcile(item)}
                    />
                  </Tooltip>
                </div>
              </div>
            ))}
          </div>
        </>
      )}
      <RequestReconcileModal
        visible={reconcileVisible}
        data={reconcileData}
        loading={reconcileLoading}
        onClose={() => setReconcileVisible(false)}
        t={t}
      />
    </div>
  );
}

function AccountDetailSideSheet({ visible, record, onClose, onReload, t }) {
  const identity = record?.account_identity || {};
  const stats = record?.stats || {};
  return (
    <SideSheet
      title={t('账号详情')}
      visible={visible}
      width={560}
      onCancel={onClose}
      bodyStyle={{ padding: 0 }}
    >
      <div className='ct-channel-account-detail-sheet'>
        <div className='ct-channel-account-detail-head'>
          <div>
            <h3>
              {identity.display_name ||
                `${t('账号')} #${Number(record?.credential_index || 0) + 1}`}
            </h3>
            <p>
              {identity.brand || record?.resource_ref?.brand || '--'} ·{' '}
              {identity.account_type || '--'}
            </p>
          </div>
          {record ? statusTag(record, t) : null}
        </div>
        <div className='ct-channel-account-detail-section'>
          <div className='ct-channel-account-detail-title'>{t('账号诊断')}</div>
          <AccountDiagnosisBlock record={record} t={t} />
        </div>
        <div className='ct-channel-account-detail-section'>
          <div className='ct-channel-account-detail-title'>{t('凭证身份')}</div>
          <div className='ct-channel-account-detail-grid'>
            <span>{t('凭证序号')}</span>
            <strong>#{Number(record?.credential_index || 0) + 1}</strong>
            <span>{t('账号标识')}</span>
            <strong>{identity.account_id || '--'}</strong>
            <span>{t('主体指纹')}</span>
            <strong>{record?.subject_short || '--'}</strong>
            <span>{t('凭证指纹')}</span>
            <strong>{record?.credential_short || '--'}</strong>
            <span>{t('代理')}</span>
            <strong>{proxyLabel(record?.proxy, t)}</strong>
          </div>
        </div>
        <div className='ct-channel-account-detail-section'>
          <div className='ct-channel-account-detail-title'>{t('调度健康')}</div>
          <DispatchHealthBlock record={record} t={t} />
        </div>
        <div className='ct-channel-account-detail-section'>
          <div className='ct-channel-account-detail-title'>{t('用量统计')}</div>
          <div className='ct-channel-account-detail-windows'>
            <DetailStatWindow title={t('今日统计')} window={stats.today || {}} t={t} />
            <DetailStatWindow title={t('近5小时')} window={stats.last_5h || {}} t={t} />
            <DetailStatWindow title={t('近7天')} window={stats.last_7d || {}} t={t} />
          </div>
        </div>
        <div className='ct-channel-account-detail-section'>
          <RecentRequestsBlock
            visible={visible}
            record={record}
            onReload={onReload}
            t={t}
          />
        </div>
        <div className='ct-channel-account-detail-section'>
          <div className='ct-channel-account-detail-title'>{t('运行键')}</div>
          <RuntimeKeysCell record={record} t={t} />
        </div>
        <div className='ct-channel-account-detail-section'>
          <div className='ct-channel-account-detail-title'>{t('账号权限')}</div>
          <CapabilitiesCell capabilities={record?.capabilities} t={t} />
        </div>
      </div>
    </SideSheet>
  );
}

function ChannelAccount() {
  const { t } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const importFileInputRef = useRef(null);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [statusLoadingKey, setStatusLoadingKey] = useState('');
  const [testingAccountKey, setTestingAccountKey] = useState('');
  const [capabilityLoadingKey, setCapabilityLoadingKey] = useState('');
  const [capabilityBatchLoading, setCapabilityBatchLoading] = useState(false);
  const [batchLoading, setBatchLoading] = useState(false);
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [keyword, setKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [view, setViewState] = useState(
    searchParams.get('view') === 'stats' ? 'stats' : 'manage',
  );
  const [page, setPage] = useState(
    Math.max(1, Number.parseInt(searchParams.get('page') || '1', 10) || 1),
  );
  const [pageSize, setPageSize] = useState(
    Math.max(1, Number.parseInt(searchParams.get('page_size') || '20', 10) || 20),
  );
  const [sortConfig, setSortConfig] = useState({ sort: '', order: '' });
  const [detailRecord, setDetailRecord] = useState(null);
  const setView = useCallback((nextView) => {
    const normalized = nextView === 'stats' ? 'stats' : 'manage';
    setViewState(normalized);
    setSortConfig({ sort: '', order: '' });
    setPage(1);
  }, []);
  const [importVisible, setImportVisible] = useState(false);
  const [importCredentials, setImportCredentials] = useState('');
  const [importActiveTab, setImportActiveTab] = useState('file');
  const [importFileList, setImportFileList] = useState([]);
  const [importDragActive, setImportDragActive] = useState(false);
  const [importOnlyNew, setImportOnlyNew] = useState(true);
  const [importLoading, setImportLoading] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [editRecord, setEditRecord] = useState(null);
  const [editCredentialType, setEditCredentialType] = useState('auto');
  const [editCredential, setEditCredential] = useState('');
  const [editLoading, setEditLoading] = useState(false);
  const [proxyVisible, setProxyVisible] = useState(false);
  const [batchProxyVisible, setBatchProxyVisible] = useState(false);
  const [proxyRecord, setProxyRecord] = useState(null);
  const [editingProxy, setEditingProxy] = useState(null);
  const [proxyEditorVisible, setProxyEditorVisible] = useState(false);
  const [proxies, setProxies] = useState([]);
  const [proxyReusePolicy, setProxyReusePolicy] = useState('warn');
  const [proxiesLoading, setProxiesLoading] = useState(false);
  const [proxySaving, setProxySaving] = useState(false);
  const [selectedProxyID, setSelectedProxyID] = useState(0);
  const [createProxyInline, setCreateProxyInline] = useState(false);
  const accountCredentialTypeOptions = useMemo(
    () => buildCredentialTypeOptions(t),
    [t],
  );
  const [proxyForm, setProxyForm] = useState({
    name: '',
    protocol: 'socks5',
    address: '',
    username: '',
    password: '',
    remark: '',
  });
  const selectedIndexes = useMemo(
    () =>
      selectedRowKeys
        .map((key) => Number(String(key).split('-')[1]))
        .filter((value) => Number.isInteger(value) && value >= 0),
    [selectedRowKeys],
  );

  const resetProxyEditorState = useCallback((record = null) => {
    setSelectedProxyID(Number(record?.proxy?.id || 0));
    setCreateProxyInline(false);
    setProxyForm({
      name: '',
      protocol: 'socks5',
      address: '',
      username: '',
      password: '',
      remark: '',
    });
  }, []);

  const loadAccounts = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await API.get(`/api/channel/${id}/accounts`, {
        params: {
          view,
          page,
          page_size: pageSize,
          keyword: keyword.trim(),
          status: statusFilter,
          sort: sortConfig.sort,
          order: sortConfig.order,
        },
        disableDuplicate: true,
      });
      const payload = unwrapApiData(response);
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('请求异常'));
      }
      setData(payload);
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('请求异常');
      setError(message);
      showError(message);
    } finally {
      setLoading(false);
    }
  }, [id, keyword, page, pageSize, sortConfig.order, sortConfig.sort, statusFilter, t, view]);

  useEffect(() => {
    const next = new URLSearchParams(searchParams);
    next.set('view', view);
    if (page > 1) next.set('page', String(page));
    else next.delete('page');
    if (pageSize !== 20) next.set('page_size', String(pageSize));
    else next.delete('page_size');
    if (next.toString() !== searchParams.toString()) {
      setSearchParams(next, { replace: true });
    }
  }, [page, pageSize, searchParams, setSearchParams, view]);

  useEffect(() => {
    const timer = setTimeout(() => {
      loadAccounts();
    }, 250);
    return () => clearTimeout(timer);
  }, [loadAccounts]);

  const loadProxies = useCallback(async () => {
    setProxiesLoading(true);
    try {
      const response = await API.get('/api/model_gateway/proxies', {
        disableDuplicate: true,
      });
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('请求异常'));
      }
      const payload = unwrapApiData(response);
      setProxies(Array.isArray(payload) ? payload : []);
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('请求异常');
      showError(message);
    } finally {
      setProxiesLoading(false);
    }
  }, [t]);

  const loadSchedulerConfig = useCallback(async () => {
    try {
      const response = await API.get('/api/model_gateway/config', {
        disableDuplicate: true,
      });
      if (response?.data?.success === false) {
        return;
      }
      const payload = unwrapApiData(response);
      setProxyReusePolicy(
        payload?.setting?.proxy_same_brand_reuse_policy || 'warn',
      );
    } catch (err) {
      setProxyReusePolicy('warn');
    }
  }, []);

  useEffect(() => {
    setSelectedRowKeys([]);
  }, [id, keyword, page, pageSize, statusFilter, view]);

  const toggleAccountStatus = useCallback(
    async (record) => {
      const enabled = !record?.key_enabled;
      const loadingKey = `${record.channel_id}-${record.credential_index}`;
      setStatusLoadingKey(loadingKey);
      try {
        const response = await API.post(
          `/api/channel/${id}/accounts/${record.credential_index}/status`,
          {
            enabled,
            reason: enabled ? '' : 'manual_disabled',
          },
        );
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('操作失败'));
        }
        const payload = unwrapApiData(response);
        await loadAccounts();
        showSuccess(
          operationMessage(
            payload.operation,
            t,
            enabled ? t('账号已启用') : t('账号已禁用'),
          ),
        );
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('操作失败');
        showError(message);
      } finally {
        setStatusLoadingKey('');
      }
    },
    [id, loadAccounts, t],
  );

  const batchUpdateAccountStatus = useCallback(
    async (enabled) => {
      if (selectedIndexes.length === 0) {
        showError(t('请先选择账号'));
        return;
      }
      setBatchLoading(true);
      try {
        const response = await API.post(`/api/channel/${id}/accounts`, {
          enabled,
          reason: enabled ? '' : 'manual_disabled',
          credential_indexes: selectedIndexes,
        });
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('操作失败'));
        }
        const payload = unwrapApiData(response);
        await loadAccounts();
        setSelectedRowKeys([]);
        showSuccess(
          operationMessage(
            payload.operation,
            t,
            enabled
              ? t('已批量启用 {{total}} 个账号', {
                  total: selectedIndexes.length,
                })
              : t('已批量禁用 {{total}} 个账号', {
                  total: selectedIndexes.length,
                }),
          ),
        );
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('操作失败');
        showError(message);
      } finally {
        setBatchLoading(false);
      }
    },
    [id, loadAccounts, selectedIndexes, t],
  );

  const testAccount = useCallback(
    async (record) => {
      if (!record?.key_enabled) {
        showError(t('请先启用账号'));
        return;
      }
      const loadingKey = `${record.channel_id}-${record.credential_index}`;
      if (accountIsCodexOAuth(record)) {
        setTestingAccountKey(loadingKey);
        setCapabilityLoadingKey(loadingKey);
        try {
          const response = await API.post('/api/channel/multi_key/manage', {
            channel_id: Number(id),
            action: 'probe_key_capabilities',
            key_index: record.credential_index,
          });
          if (response?.data?.success === false) {
            throw new Error(response?.data?.message || t('账号权限检测失败'));
          }
          const capabilityMessage =
            response?.data?.data?.capabilities?.last_message ||
            response?.data?.message ||
            t('账号权限检测完成');
          showSuccess(summarizeAccountCapabilityError(capabilityMessage, t));
          await loadAccounts();
        } catch (err) {
          const message =
            err?.response?.data?.message ||
            err?.message ||
            t('账号权限检测失败');
          showError(summarizeAccountCapabilityError(message, t));
        } finally {
          setTestingAccountKey('');
          setCapabilityLoadingKey('');
        }
        return;
      }
      setTestingAccountKey(loadingKey);
      try {
        const response = await API.get(`/api/channel/test/${id}`, {
          params: {
            credential_index: record.credential_index,
          },
          disableDuplicate: true,
        });
        const payload = response?.data || {};
        if (!payload.success) {
          throw new Error(payload.message || t('测试失败'));
        }
        showInfo(
          t('账号测试成功，耗时 {{time}} 秒', {
            time: Number(payload.time || 0).toFixed(2),
          }),
        );
        loadAccounts();
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('测试失败');
        showError(summarizeAccountCapabilityError(message, t));
      } finally {
        setTestingAccountKey('');
      }
    },
    [id, loadAccounts, t],
  );

  const probeAccountCapability = useCallback(
    async (record) => {
      const loadingKey = `${record.channel_id}-${record.credential_index}`;
      setCapabilityLoadingKey(loadingKey);
      try {
        const response = await API.post('/api/channel/multi_key/manage', {
          channel_id: Number(id),
          action: 'probe_key_capabilities',
          key_index: record.credential_index,
        });
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('账号权限检测失败'));
        }
        const capabilityMessage =
          response?.data?.data?.capabilities?.last_message ||
          response?.data?.message ||
          t('账号权限检测完成');
        showSuccess(summarizeAccountCapabilityError(capabilityMessage, t));
        await loadAccounts();
      } catch (err) {
        const message =
          err?.response?.data?.message ||
          err?.message ||
          t('账号权限检测失败');
        showError(summarizeAccountCapabilityError(message, t));
      } finally {
        setCapabilityLoadingKey('');
      }
    },
    [id, loadAccounts, t],
  );

  const diagnosePlatformCapability = useCallback(
    async (record) => {
      const loadingKey = `${record.channel_id}-${record.credential_index}:platform`;
      setCapabilityLoadingKey(loadingKey);
      try {
        const response = await API.post('/api/channel/multi_key/manage', {
          channel_id: Number(id),
          action: 'diagnose_platform_key_capabilities',
          key_index: record.credential_index,
        });
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('Platform API 诊断失败'));
        }
        const capabilityMessage =
          response?.data?.data?.capabilities?.last_message ||
          response?.data?.message ||
          t('Platform API 诊断完成');
        showInfo(summarizeAccountCapabilityError(capabilityMessage, t));
        await loadAccounts();
      } catch (err) {
        const message =
          err?.response?.data?.message ||
          err?.message ||
          t('Platform API 诊断失败');
        showError(summarizeAccountCapabilityError(message, t));
      } finally {
        setCapabilityLoadingKey('');
      }
    },
    [id, loadAccounts, t],
  );

  const probeAllAccountCapabilities = useCallback(async () => {
    setCapabilityBatchLoading(true);
    try {
      const response = await API.post('/api/channel/multi_key/manage', {
        channel_id: Number(id),
        action: 'probe_all_key_capabilities',
      });
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('账号权限检测失败'));
      }
      showSuccess(response?.data?.message || t('账号权限检测完成'));
      await loadAccounts();
    } catch (err) {
      const message =
        err?.response?.data?.message ||
        err?.message ||
        t('账号权限检测失败');
      showError(message);
    } finally {
      setCapabilityBatchLoading(false);
    }
  }, [id, loadAccounts, t]);

  const deleteAccounts = useCallback(
    async (indexes) => {
      const normalizedIndexes = [...new Set(indexes)]
        .map((value) => Number(value))
        .filter((value) => Number.isInteger(value) && value >= 0);
      if (normalizedIndexes.length === 0) {
        showError(t('请先选择账号'));
        return;
      }
      setDeleteLoading(true);
      try {
        const response = await API.delete(`/api/channel/${id}/accounts`, {
          data: {
            credential_indexes: normalizedIndexes,
          },
        });
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('操作失败'));
        }
        const payload = unwrapApiData(response);
        await loadAccounts();
        setSelectedRowKeys([]);
        showSuccess(
          operationMessage(
            payload.operation,
            t,
            t('已删除 {{total}} 个账号', { total: normalizedIndexes.length }),
          ),
        );
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('操作失败');
        showError(message);
      } finally {
        setDeleteLoading(false);
      }
    },
    [id, loadAccounts, t],
  );

  const deleteSingleAccount = useCallback(
    (record) => deleteAccounts([record.credential_index]),
    [deleteAccounts],
  );

  const batchDeleteAccounts = useCallback(() => {
    return deleteAccounts(selectedIndexes);
  }, [deleteAccounts, selectedIndexes]);

  const resetImportModal = useCallback(() => {
    setImportVisible(false);
    setImportCredentials('');
    setImportFileList([]);
    setImportActiveTab('file');
    setImportDragActive(false);
    if (importFileInputRef.current) {
      importFileInputRef.current.value = '';
    }
  }, []);

  const appendImportFiles = useCallback(
    (files) => {
      const incomingQueue = ChannelAccountImportFileQueue.fromFiles(files);
      const oversizedFiles = incomingQueue.oversized(
        CHANNEL_ACCOUNT_IMPORT_FILE_LIMIT,
      );
      if (oversizedFiles.length > 0) {
        showError(
          t('文件过大：{{name}}', {
            name: uploadedFileName(oversizedFiles[0], t('未命名文件')),
          }),
        );
      }
      const validQueue = incomingQueue.withinSize(
        CHANNEL_ACCOUNT_IMPORT_FILE_LIMIT,
      );
      if (validQueue.items.length === 0) {
        return;
      }
      setImportFileList((prev) =>
        new ChannelAccountImportFileQueue(prev).append(validQueue).items,
      );
    },
    [t],
  );

  const handleImportFileInputChange = useCallback(
    (event) => {
      appendImportFiles(event.target.files);
      event.target.value = '';
    },
    [appendImportFiles],
  );

  const handleImportDrop = useCallback(
    (event) => {
      event.preventDefault();
      event.stopPropagation();
      setImportDragActive(false);
      appendImportFiles(event.dataTransfer?.files);
    },
    [appendImportFiles],
  );

  const openImportFilePicker = useCallback(() => {
    importFileInputRef.current?.click();
  }, []);

  const removeImportFile = useCallback((uid) => {
    setImportFileList((prev) => prev.filter((item) => item.uid !== uid));
  }, []);

  const importAccounts = useCallback(async () => {
    const submission = new ChannelAccountImportSubmission({
      credentials: importCredentials,
      files: importFileList,
      onlyNew: importOnlyNew,
    });
    if (!submission.hasInput()) {
      showError(t('请先输入账号凭证'));
      return;
    }
    setImportLoading(true);
    try {
      const { body, config } = await submission.payload();
      const response = await API.put(
        `/api/channel/${id}/accounts`,
        body,
        config,
      );
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('导入失败'));
      }
      const payload = unwrapApiData(response);
      await loadAccounts();
      setSelectedRowKeys([]);
      resetImportModal();
      showSuccess(operationMessage(payload.operation, t, t('导入成功')));
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('导入失败');
      showError(message);
    } finally {
      setImportLoading(false);
    }
  }, [
    id,
    importCredentials,
    importFileList,
    importOnlyNew,
    loadAccounts,
    resetImportModal,
    t,
  ]);

  const openEditModal = useCallback(
    (record) => {
      setEditRecord(record);
      setProxyRecord(record);
      setEditCredentialType(record?.account_identity?.account_type || 'auto');
      setEditCredential('');
      resetProxyEditorState(record);
      setEditVisible(true);
      loadProxies();
      loadSchedulerConfig();
    },
    [loadProxies, loadSchedulerConfig, resetProxyEditorState],
  );

  const closeEditModal = useCallback(() => {
    setEditVisible(false);
    setEditRecord(null);
    setProxyRecord(null);
    setEditCredentialType('auto');
    setEditCredential('');
    resetProxyEditorState();
  }, [resetProxyEditorState]);

  const openProxyModal = useCallback(
    (record) => {
      setProxyRecord(record);
      resetProxyEditorState(record);
      setProxyVisible(true);
      loadProxies();
      loadSchedulerConfig();
    },
    [loadProxies, loadSchedulerConfig, resetProxyEditorState],
  );

  const openBatchProxyModal = useCallback(() => {
    if (selectedIndexes.length === 0) {
      showError(t('请先选择账号'));
      return;
    }
    setProxyRecord(null);
    resetProxyEditorState();
    setBatchProxyVisible(true);
    loadProxies();
    loadSchedulerConfig();
  }, [
    loadProxies,
    loadSchedulerConfig,
    resetProxyEditorState,
    selectedIndexes.length,
    t,
  ]);

  const closeProxyModal = useCallback(() => {
    setProxyVisible(false);
    setProxyRecord(null);
    resetProxyEditorState();
  }, [resetProxyEditorState]);

  const closeBatchProxyModal = useCallback(() => {
    setBatchProxyVisible(false);
    resetProxyEditorState();
  }, [resetProxyEditorState]);

  const openProxyEditModal = useCallback(
    (proxy) => {
      if (!proxy?.id) return;
      setEditingProxy(proxy);
      setProxyEditorVisible(true);
    },
    [],
  );

  const closeProxyEditModal = useCallback(() => {
    setProxyEditorVisible(false);
    setEditingProxy(null);
  }, []);

  const handleProxyEdited = useCallback(async () => {
    await Promise.all([loadAccounts(), loadProxies()]);
  }, [loadAccounts, loadProxies]);

  const selectedProxy = useMemo(
    () => proxies.find((proxy) => Number(proxy.id) === Number(selectedProxyID)),
    [proxies, selectedProxyID],
  );
  const selectedProxyRisk = useMemo(
    () =>
      !createProxyInline && Number(selectedProxyID || 0) > 0
        ? proxyBindingRisk(selectedProxy, proxyRecord)
        : null,
    [createProxyInline, proxyRecord, selectedProxy, selectedProxyID],
  );

  const proxyBindingChanged = useCallback(
    (record = proxyRecord) =>
      Boolean(record) &&
      (createProxyInline ||
        Number(selectedProxyID || 0) !== Number(record?.proxy?.id || 0)),
    [createProxyInline, proxyRecord, selectedProxyID],
  );

  const createOrResolveProxyID = useCallback(async () => {
    let proxyID = Number(selectedProxyID || 0);
    if (createProxyInline) {
      const address = proxyForm.address.trim();
      if (!address) {
        throw new Error(t('请填写代理地址'));
      }
      const createResponse = await API.post('/api/model_gateway/proxies', {
        ...proxyForm,
        enabled: true,
      });
      if (createResponse?.data?.success === false) {
        throw new Error(createResponse?.data?.message || t('创建代理失败'));
      }
      const created = unwrapApiData(createResponse);
      proxyID = Number(created?.id || 0);
    }
    return proxyID;
  }, [createProxyInline, proxyForm, selectedProxyID, t]);

  const submitProxyBinding = useCallback(async (record, allowReuseRisk = false) => {
    if (!record) return null;
    const proxyID = await createOrResolveProxyID();
    const response = await API.post(
      `/api/channel/${id}/accounts/${record.credential_index}/proxy`,
      {
        proxy_id: proxyID,
        allow_reuse_risk: allowReuseRisk,
      },
    );
    if (response?.data?.success === false) {
      throw new Error(response?.data?.message || t('操作失败'));
    }
    return {
      payload: unwrapApiData(response),
      proxyID,
    };
  }, [createOrResolveProxyID, id, t]);

  const submitBatchProxyBinding = useCallback(async (allowReuseRisk = false) => {
    const proxyID = await createOrResolveProxyID();
    const response = await API.post(`/api/channel/${id}/account-proxies`, {
      proxy_id: proxyID,
      credential_indexes: selectedIndexes,
      allow_reuse_risk: allowReuseRisk,
    });
    if (response?.data?.success === false) {
      throw new Error(response?.data?.message || t('操作失败'));
    }
    return {
      payload: unwrapApiData(response),
      proxyID,
    };
  }, [createOrResolveProxyID, id, selectedIndexes, t]);

  const saveAccountCredential = useCallback(async (allowReuseRisk = false) => {
    if (!editRecord) return;
    const confirmedReuse = allowReuseRisk === true;
    const credential = editCredential.trim();
    const shouldUpdateCredential = credential.length > 0;
    const shouldUpdateProxy = proxyBindingChanged(editRecord);
    if (!shouldUpdateCredential && !shouldUpdateProxy) {
      closeEditModal();
      return;
    }
    if (
      shouldUpdateProxy &&
      !confirmedReuse &&
      !createProxyInline &&
      proxyReusePolicy === 'confirm' &&
      selectedProxyRisk
    ) {
      Modal.confirm({
        title: t('确认同品牌代理复用'),
        content: reuseRiskText(selectedProxyRisk, t),
        okText: t('确认绑定'),
        cancelText: t('取消'),
        onOk: () => saveAccountCredential(true),
      });
      return;
    }
    setEditLoading(true);
    try {
      let payload = null;
      const messages = [];
      if (shouldUpdateCredential) {
        const response = await API.put(
          `/api/channel/${id}/accounts/${editRecord.credential_index}`,
          {
            credential,
            credential_type: editCredentialType,
          },
        );
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('保存失败'));
        }
        payload = unwrapApiData(response);
        messages.push(operationMessage(payload.operation, t, t('账号凭证已更新')));
      }
      if (shouldUpdateProxy) {
        const bindingRecord = findAccountItem(payload, editRecord);
        const result = await submitProxyBinding(bindingRecord, confirmedReuse);
        payload = result?.payload || payload;
        messages.push(
          operationMessage(
            result?.payload?.operation,
            t,
            Number(result?.proxyID || 0) > 0
              ? t('账号代理已绑定')
              : t('账号代理已解绑'),
          ),
        );
      }
      if (payload) await loadAccounts();
      closeEditModal();
      showSuccess(messages.filter(Boolean).join(t('、')) || t('保存成功'));
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('保存失败');
      if (
        !confirmedReuse &&
        shouldUpdateProxy &&
        !createProxyInline &&
        isProxyReuseConfirmRequiredMessage(message)
      ) {
        Modal.confirm({
          title: t('确认同品牌代理复用'),
          content: selectedProxyRisk
            ? reuseRiskText(selectedProxyRisk, t)
            : message,
          okText: t('确认绑定'),
          cancelText: t('取消'),
          onOk: () => saveAccountCredential(true),
        });
        return;
      }
      showError(message);
    } finally {
      setEditLoading(false);
    }
  }, [
    closeEditModal,
    createProxyInline,
    editCredential,
    editCredentialType,
    editRecord,
    id,
    proxyBindingChanged,
    loadAccounts,
    proxyReusePolicy,
    selectedProxyRisk,
    submitProxyBinding,
    t,
  ]);

  const saveProxyBindingRequest = useCallback(async (allowReuseRisk = false) => {
    if (!proxyRecord) return;
    setProxySaving(true);
    try {
      const result = await submitProxyBinding(proxyRecord, allowReuseRisk);
      const payload = result?.payload;
      await loadAccounts();
      closeProxyModal();
      showSuccess(
        operationMessage(
          payload?.operation,
          t,
          Number(result?.proxyID || 0) > 0
            ? t('账号代理已绑定')
            : t('账号代理已解绑'),
        ),
      );
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('操作失败');
      if (
        !allowReuseRisk &&
        !createProxyInline &&
        isProxyReuseConfirmRequiredMessage(message)
      ) {
        Modal.confirm({
          title: t('确认同品牌代理复用'),
          content: selectedProxyRisk
            ? reuseRiskText(selectedProxyRisk, t)
            : message,
          okText: t('确认绑定'),
          cancelText: t('取消'),
          onOk: () => saveProxyBindingRequest(true),
        });
        return;
      }
      showError(message);
    } finally {
      setProxySaving(false);
    }
  }, [
    closeProxyModal,
    createProxyInline,
    loadAccounts,
    proxyRecord,
    selectedProxyRisk,
    submitProxyBinding,
    t,
  ]);

  const saveProxyBinding = useCallback(async () => {
    if (
      !createProxyInline &&
      proxyReusePolicy === 'confirm' &&
      selectedProxyRisk
    ) {
      Modal.confirm({
        title: t('确认同品牌代理复用'),
        content: reuseRiskText(selectedProxyRisk, t),
        okText: t('确认绑定'),
        cancelText: t('取消'),
        onOk: () => saveProxyBindingRequest(true),
      });
      return;
    }
    await saveProxyBindingRequest(false);
  }, [
    createProxyInline,
    proxyReusePolicy,
    saveProxyBindingRequest,
    selectedProxyRisk,
    t,
  ]);

  const saveBatchProxyBindingRequest = useCallback(async (allowReuseRisk = false) => {
    if (selectedIndexes.length === 0) {
      showError(t('请先选择账号'));
      return;
    }
    setProxySaving(true);
    try {
      const result = await submitBatchProxyBinding(allowReuseRisk);
      const payload = result?.payload;
      await loadAccounts();
      setSelectedRowKeys([]);
      closeBatchProxyModal();
      showSuccess(
        operationMessage(
          payload?.operation,
          t,
          Number(result?.proxyID || 0) > 0
            ? t('已设置 {{total}} 个账号代理', {
                total: selectedIndexes.length,
              })
            : t('已解绑 {{total}} 个账号代理', {
                total: selectedIndexes.length,
              }),
        ),
      );
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('操作失败');
      if (
        !allowReuseRisk &&
        !createProxyInline &&
        isProxyReuseConfirmRequiredMessage(message)
      ) {
        Modal.confirm({
          title: t('确认同品牌代理复用'),
          content: message,
          okText: t('确认绑定'),
          cancelText: t('取消'),
          onOk: () => saveBatchProxyBindingRequest(true),
        });
        return;
      }
      showError(message);
    } finally {
      setProxySaving(false);
    }
  }, [
    closeBatchProxyModal,
    createProxyInline,
    loadAccounts,
    selectedIndexes,
    submitBatchProxyBinding,
    t,
  ]);

  const saveBatchProxyBinding = useCallback(async () => {
    await saveBatchProxyBindingRequest(false);
  }, [saveBatchProxyBindingRequest]);

  const columns = useMemo(
    () =>
      buildColumns(
        t,
        toggleAccountStatus,
        deleteSingleAccount,
        openEditModal,
        openProxyModal,
        openProxyEditModal,
        testAccount,
        probeAccountCapability,
        diagnosePlatformCapability,
        statusLoadingKey,
        testingAccountKey,
        capabilityLoadingKey,
      ),
    [
      t,
      toggleAccountStatus,
      deleteSingleAccount,
      openEditModal,
      openProxyModal,
      openProxyEditModal,
      testAccount,
      probeAccountCapability,
      diagnosePlatformCapability,
      statusLoadingKey,
      testingAccountKey,
      capabilityLoadingKey,
    ],
  );
  const statsColumns = useMemo(
    () =>
      buildStatsColumns(
        t,
        toggleAccountStatus,
        setDetailRecord,
        statusLoadingKey,
      ),
    [t, toggleAccountStatus, statusLoadingKey],
  );
  const items = data?.items || [];
  const selectedCount = selectedRowKeys.length;
  const isStatsView = view === 'stats';
  const summary = data?.summary || {};
  const tableColumns = isStatsView ? statsColumns : columns;
  const tableScrollX = isStatsView ? 1660 : 2420;
  const tablePagination = useMemo(
    () => ({
      currentPage: data?.page || page,
      pageSize: data?.page_size || pageSize,
      total: data?.filtered_total ?? data?.total ?? 0,
      showSizeChanger: true,
      pageSizeOpts: [20, 50, 100],
      onPageChange: (nextPage) => setPage(nextPage),
      onPageSizeChange: (nextSize) => {
        setPageSize(nextSize);
        setPage(1);
      },
    }),
    [data?.filtered_total, data?.page, data?.page_size, data?.total, page, pageSize],
  );
  const handleTableChange = useCallback((changeInfo = {}) => {
    const sorter = changeInfo?.sorter || {};
    const sortOrder = sorter.sortOrder || sorter.order || sorter.sorter?.sortOrder || '';
    const sortKey =
      sorter.dataIndex ||
      sorter.key ||
      sorter.columnKey ||
      sorter.sorter?.dataIndex ||
      sorter.sorter?.key ||
      '';
    if (!sortOrder) {
      setSortConfig({ sort: '', order: '' });
      return;
    }
    setSortConfig({
      sort: String(sortKey || ''),
      order: String(sortOrder).toLowerCase().includes('asc') ? 'asc' : 'desc',
    });
    setPage(1);
  }, []);
  const metricCards = isStatsView
    ? [
        {
          icon: <KeyRound size={18} />,
          label: t('账号总数'),
          value: formatNumber(data?.total),
          detail: t('当前渠道可识别凭证'),
        },
        {
          icon: <BarChart3 size={18} />,
          label: t('今日请求'),
          value: formatCompactNumber(summary.today?.requests),
          detail: t('默认排除健康探活'),
        },
        {
          icon: <Gauge size={18} />,
          label: t('近5小时请求'),
          value: formatCompactNumber(summary.last_5h?.requests),
          detail: `${t('成功率')} ${formatPercent(summary.last_5h?.success_rate)}`,
        },
        {
          icon: <Clock3 size={18} />,
          label: t('近7天扣费'),
          value: formatQuotaValue(summary.last_7d?.quota),
          detail: `${t('账号成本')} ${formatCost(summary.last_7d?.upstream_cost_total)}`,
        },
      ]
    : [
        {
          icon: <KeyRound size={18} />,
          label: t('账号总数'),
          value: formatNumber(data?.total),
          detail: t('当前渠道可识别凭证'),
        },
        {
          icon: <BadgeCheck size={18} />,
          label: t('启用账号'),
          value: formatNumber(data?.enabled),
          detail: t('可参与智能调度'),
        },
        {
          icon: <Gauge size={18} />,
          label: t('已有评分'),
          value: formatNumber(summary.scored),
          detail: t('当前页运行态快照'),
        },
        {
          icon: <Clock3 size={18} />,
          label: t('近30分钟真实样本'),
          value: formatNumber(summary.real_sample_count_30m),
          detail: t('当前页评分样本'),
        },
      ];
  const rowSelection = useMemo(
    () => ({
      selectedRowKeys,
      onChange: (keys) => setSelectedRowKeys(keys),
    }),
    [selectedRowKeys],
  );
  return (
    <div className='ct-console-content-wrap'>
      <div className='ct-channel-account-page'>
        <div className='ct-channel-account-hero'>
          <div className='ct-channel-account-title-block'>
            <div className='ct-channel-account-title-icon'>
              <ShieldCheck size={22} />
            </div>
            <div>
              <div className='ct-channel-account-eyebrow'>
                {t('渠道账号管理')}
              </div>
              <h2>
                {data?.channel_name || t('渠道')} #{data?.channel_id || id}
              </h2>
              <p>{t('渠道账号以渠道、品牌、凭证主体形成唯一调度身份')}</p>
            </div>
          </div>
          <Space className='ct-channel-account-actions' spacing={8}>
            <Button
              icon={<ArrowLeft size={16} />}
              type='tertiary'
              onClick={() => navigate('/console/channel')}
            >
              {t('返回渠道列表')}
            </Button>
            <Button
              icon={<Plus size={15} />}
              type='primary'
              theme='light'
              onClick={() => setImportVisible(true)}
            >
              {t('导入账号')}
            </Button>
            <Button
              icon={<RefreshCw size={15} />}
              type='primary'
              theme='solid'
              loading={loading}
              onClick={loadAccounts}
            >
              {t('刷新')}
            </Button>
          </Space>
        </div>

        <div className='ct-channel-account-viewbar'>
          <Tabs
            type='button'
            activeKey={view}
            onChange={(key) => setView(key)}
          >
            <Tabs.TabPane
              itemKey='manage'
              tab={
                <span className='ct-channel-account-view-tab'>
                  <SlidersHorizontal size={14} />
                  {t('管理视图')}
                </span>
              }
            />
            <Tabs.TabPane
              itemKey='stats'
              tab={
                <span className='ct-channel-account-view-tab'>
                  <BarChart3 size={14} />
                  {t('统计视图')}
                </span>
              }
            />
          </Tabs>
          <Text type='tertiary'>
            {isStatsView
              ? t('统计从上线后开始累计，默认排除健康探活')
              : t('Codex 能力和 Platform API 诊断分开展示，Platform 失败不影响 Codex 调度')}
          </Text>
        </div>

        {error ? (
          <Banner
            type='danger'
            closeIcon={null}
            description={
              <span className='ct-channel-account-error'>{error}</span>
            }
          />
        ) : null}

        <div className='ct-channel-account-metric-grid'>
          {metricCards.map((card) => (
            <MetricCard
              key={card.label}
              icon={card.icon}
              label={card.label}
              value={card.value}
              detail={card.detail}
            />
          ))}
        </div>

        <div className='ct-channel-account-table-wrap'>
          <div className='ct-channel-account-toolbar'>
            <div className='ct-channel-account-filter-group'>
              <Input
                prefix={<Search size={14} />}
                value={keyword}
                onChange={(value) => {
                  setKeyword(value);
                  setPage(1);
                }}
                placeholder={t('搜索账号、品牌、凭证或运行键')}
                className='ct-channel-account-search'
              />
              <Select
                value={statusFilter}
                onChange={(value) => {
                  setStatusFilter(value);
                  setPage(1);
                }}
                prefix={t('状态')}
                className='ct-channel-account-status-select'
              >
                <Select.Option value='all'>{t('全部')}</Select.Option>
                <Select.Option value='enabled'>{t('已启用')}</Select.Option>
                <Select.Option value='disabled'>{t('已禁用')}</Select.Option>
              </Select>
            </div>
            <Space
              className='ct-channel-account-batch-actions'
              spacing={8}
              style={{ display: isStatsView ? 'none' : undefined }}
            >
              <Text type='tertiary'>
                {t('已选 {{total}} 个账号', { total: selectedCount })}
              </Text>
              <Popconfirm
                title={t('确定启用所选账号？')}
                content={t('启用后这些账号可重新参与智能调度')}
                onConfirm={() => batchUpdateAccountStatus(true)}
                disabled={selectedCount === 0}
              >
                <Button
                  size='small'
                  icon={<ToggleRight size={14} />}
                  loading={batchLoading}
                  disabled={selectedCount === 0}
                >
                  {t('批量启用')}
                </Button>
              </Popconfirm>
              <Popconfirm
                title={t('确定禁用所选账号？')}
                content={t('禁用后这些账号不会参与智能调度')}
                onConfirm={() => batchUpdateAccountStatus(false)}
                disabled={selectedCount === 0}
              >
                <Button
                  size='small'
                  type='warning'
                  theme='light'
                  icon={<ToggleLeft size={14} />}
                  loading={batchLoading}
                  disabled={selectedCount === 0}
                >
                  {t('批量禁用')}
                </Button>
              </Popconfirm>
              <Button
                size='small'
                type='primary'
                theme='light'
                icon={<Search size={14} />}
                loading={capabilityBatchLoading}
                disabled={items.length === 0}
                onClick={probeAllAccountCapabilities}
              >
                {t('检测全部权限')}
              </Button>
              <Button
                size='small'
                icon={<PlugZap size={14} />}
                loading={proxySaving && batchProxyVisible}
                disabled={selectedCount === 0}
                onClick={openBatchProxyModal}
              >
                {t('批量设置代理')}
              </Button>
              <Popconfirm
                title={t('确定删除所选账号？')}
                content={t('删除后这些凭证将从渠道中移除，此操作不可撤销')}
                onConfirm={batchDeleteAccounts}
                disabled={selectedCount === 0}
              >
                <Button
                  size='small'
                  type='danger'
                  theme='light'
                  icon={<Trash2 size={14} />}
                  loading={deleteLoading}
                  disabled={selectedCount === 0}
                >
                  {t('批量删除')}
                </Button>
              </Popconfirm>
            </Space>
          </div>
          {loading && !data ? (
            <Skeleton active placeholder={<Skeleton.Paragraph rows={8} />} />
          ) : (
            <Table
              size='small'
              columns={tableColumns}
              dataSource={items}
              rowKey={(record) =>
                `${record.channel_id}-${record.credential_index}`
              }
              rowSelection={isStatsView ? undefined : rowSelection}
              pagination={tablePagination}
              onChange={handleTableChange}
              empty={<Empty description={t('暂无账号数据')} />}
              scroll={{ x: tableScrollX }}
              loading={loading}
            />
          )}
        </div>
        <AccountDetailSideSheet
          visible={Boolean(detailRecord)}
          record={detailRecord}
          t={t}
          onClose={() => setDetailRecord(null)}
          onReload={loadAccounts}
        />
        <Modal
          title={t('编辑账号')}
          visible={editVisible}
          width={720}
          okText={t('保存')}
          cancelText={t('取消')}
          confirmLoading={editLoading}
          onOk={saveAccountCredential}
          onCancel={closeEditModal}
        >
          <div className='ct-channel-account-edit-modal'>
            <div className='ct-channel-account-edit-target'>
              <div>
                <Text strong>
                  {editRecord?.account_identity?.display_name ||
                    `${t('账号')} #${Number(editRecord?.credential_index || 0) + 1}`}
                </Text>
                <div>
                  <Text type='tertiary'>
                    {t('凭证序号')} #{Number(editRecord?.credential_index || 0) + 1}
                  </Text>
                </div>
              </div>
              <Space spacing={6}>
                <Tag color='cyan' type='light' shape='circle'>
                  {editRecord?.account_identity?.brand ||
                    editRecord?.resource_ref?.brand ||
                    '--'}
                </Tag>
                <Tag color='blue' type='light' shape='circle'>
                  {editRecord?.account_identity?.account_type || '--'}
                </Tag>
              </Space>
            </div>
            <Banner
              type='info'
              closeIcon={null}
              fullMode={false}
              description={t(
                '为保护密钥安全，列表不会回显完整凭证；凭证留空时只保存代理设置，填写新凭证后会替换当前账号。',
              )}
            />
            <div className='ct-channel-account-edit-form'>
              <label className='ct-channel-account-edit-label'>
                <span>{t('账号凭证类型')}</span>
                <Select
                  value={editCredentialType}
                  onChange={setEditCredentialType}
                  style={{ width: '100%' }}
                >
                  {accountCredentialTypeOptions.map((option) => (
                    <Select.Option key={option.value} value={option.value}>
                      {option.label}
                    </Select.Option>
                  ))}
                </Select>
              </label>
              <label className='ct-channel-account-edit-label'>
                <span>{t('账号凭证')}</span>
                <TextArea
                  value={editCredential}
                  onChange={setEditCredential}
                  autosize={{ minRows: 7, maxRows: 14 }}
                  placeholder={t('留空则不修改凭证；粘贴新凭证后会替换当前凭证')}
                  showClear
                />
              </label>
              <Text type='tertiary' size='small'>
                {t('JSON 类型会在保存前压缩为单行，并只在列表展示账号类型和短指纹。')}
              </Text>
            </div>
            <div className='ct-channel-account-edit-section'>
              <div className='ct-channel-account-edit-section-title'>
                <PlugZap size={15} />
                <span>{t('账号代理')}</span>
                {editRecord?.proxy ? (
                  <Tag color='cyan' type='light' shape='circle'>
                    {proxyLabel(editRecord.proxy, t)}
                  </Tag>
                ) : (
                  <Tag color='grey' type='light' shape='circle'>
                    {t('未绑定代理')}
                  </Tag>
                )}
              </div>
              <ProxyBindingEditor
                t={t}
                currentProxy={editRecord?.proxy}
                proxyReusePolicy={proxyReusePolicy}
                createProxyInline={createProxyInline}
                setCreateProxyInline={setCreateProxyInline}
                selectedProxyID={selectedProxyID}
                setSelectedProxyID={setSelectedProxyID}
                proxiesLoading={proxiesLoading}
                proxies={proxies}
                loadProxies={loadProxies}
                selectedProxyRisk={selectedProxyRisk}
                proxyForm={proxyForm}
                setProxyForm={setProxyForm}
              />
            </div>
          </div>
        </Modal>
        <Modal
          title={t('账号代理')}
          visible={proxyVisible}
          width={720}
          okText={t('保存')}
          cancelText={t('取消')}
          confirmLoading={proxySaving}
          onOk={saveProxyBinding}
          onCancel={closeProxyModal}
        >
          <div className='ct-channel-account-proxy-modal'>
            <div className='ct-channel-account-proxy-target'>
              <div>
                <Text strong>
                  {proxyRecord?.account_identity?.display_name ||
                    `${t('账号')} #${Number(proxyRecord?.credential_index || 0) + 1}`}
                </Text>
                <div>
                  <Text type='tertiary'>
                    {t('凭证序号')} #{Number(proxyRecord?.credential_index || 0) + 1}
                  </Text>
                </div>
              </div>
              {proxyRecord?.proxy ? (
                <Tag color='cyan' type='light' shape='circle'>
                  {proxyLabel(proxyRecord.proxy, t)}
                </Tag>
              ) : (
                <Tag color='grey' type='light' shape='circle'>
                  {t('未绑定代理')}
                </Tag>
              )}
            </div>
            <ProxyBindingEditor
              t={t}
              currentProxy={proxyRecord?.proxy}
              proxyReusePolicy={proxyReusePolicy}
              createProxyInline={createProxyInline}
              setCreateProxyInline={setCreateProxyInline}
              selectedProxyID={selectedProxyID}
              setSelectedProxyID={setSelectedProxyID}
              proxiesLoading={proxiesLoading}
              proxies={proxies}
              loadProxies={loadProxies}
              selectedProxyRisk={selectedProxyRisk}
              proxyForm={proxyForm}
              setProxyForm={setProxyForm}
            />
          </div>
        </Modal>
        <Modal
          title={t('批量设置代理')}
          visible={batchProxyVisible}
          width={720}
          okText={t('保存')}
          cancelText={t('取消')}
          confirmLoading={proxySaving}
          onOk={saveBatchProxyBinding}
          onCancel={closeBatchProxyModal}
        >
          <div className='ct-channel-account-proxy-modal'>
            <div className='ct-channel-account-proxy-target'>
              <div>
                <Text strong>
                  {t('已选 {{total}} 个账号', { total: selectedCount })}
                </Text>
                <div>
                  <Text type='tertiary'>
                    {t('保存后会将同一个代理应用到这些账号')}
                  </Text>
                </div>
              </div>
              <Tag color='cyan' type='light' shape='circle'>
                {t('批量操作')}
              </Tag>
            </div>
            <ProxyBindingEditor
              t={t}
              currentProxy={null}
              proxyReusePolicy={proxyReusePolicy}
              createProxyInline={createProxyInline}
              setCreateProxyInline={setCreateProxyInline}
              selectedProxyID={selectedProxyID}
              setSelectedProxyID={setSelectedProxyID}
              proxiesLoading={proxiesLoading}
              proxies={proxies}
              loadProxies={loadProxies}
              selectedProxyRisk={null}
              proxyForm={proxyForm}
              setProxyForm={setProxyForm}
            />
          </div>
        </Modal>
        <Modal
          title={t('导入账号')}
          visible={importVisible}
          width={760}
          okText={t('导入')}
          cancelText={t('取消')}
          confirmLoading={importLoading}
          onOk={importAccounts}
          onCancel={resetImportModal}
        >
          <div className='ct-channel-account-import-modal'>
            <div className='ct-channel-account-import-overview'>
              <div className='ct-channel-account-import-overview-item'>
                <FileArchive size={16} />
                <span>{t('XAutoJS newapi ZIP')}</span>
              </div>
              <div className='ct-channel-account-import-overview-item'>
                <FileText size={16} />
                <span>{t('JSON / TXT / NDJSON')}</span>
              </div>
              <div className='ct-channel-account-import-overview-item'>
                <ListChecks size={16} />
                <span>{t('粘贴多行凭证')}</span>
              </div>
            </div>
            <Tabs
              type='button'
              activeKey={importActiveTab}
              onChange={setImportActiveTab}
              keepDOM
            >
              <Tabs.TabPane
                itemKey='file'
                tab={
                  <span className='ct-channel-account-import-tab'>
                    <FileUp size={14} />
                    {t('文件导入')}
                  </span>
                }
              >
                <button
                  type='button'
                  className={`ct-channel-account-import-dropzone ${
                    importDragActive
                      ? 'ct-channel-account-import-dropzone-active'
                      : ''
                  }`}
                  onClick={openImportFilePicker}
                  onDragEnter={(event) => {
                    event.preventDefault();
                    setImportDragActive(true);
                  }}
                  onDragOver={(event) => {
                    event.preventDefault();
                    event.dataTransfer.dropEffect = 'copy';
                    setImportDragActive(true);
                  }}
                  onDragLeave={(event) => {
                    event.preventDefault();
                    if (!event.currentTarget.contains(event.relatedTarget)) {
                      setImportDragActive(false);
                    }
                  }}
                  onDrop={handleImportDrop}
                >
                  <input
                    ref={importFileInputRef}
                    type='file'
                    accept={CHANNEL_ACCOUNT_IMPORT_FILE_ACCEPT}
                    multiple
                    className='ct-channel-account-import-file-input'
                    onChange={handleImportFileInputChange}
                  />
                  <UploadCloud size={22} />
                  <span>{t('上传 xauto 导出包或账号文件')}</span>
                  <small>
                    {t(
                      '支持 .zip、.json、.txt、.ndjson；ZIP 内会自动识别 xauto newapi 导出结构',
                    )}
                  </small>
                </button>
                <div className='ct-channel-account-import-file-list'>
                  {importFileList.length > 0 ? (
                    importFileList.map((item) => (
                      <div
                        className='ct-channel-account-import-file'
                        key={item.uid}
                      >
                        <div className='ct-channel-account-import-file-main'>
                          <FileText size={15} />
                          <span>{uploadedFileName(item, t('未命名文件'))}</span>
                          <Tag size='small'>
                            {formatFileSize(uploadedFileSize(item))}
                          </Tag>
                        </div>
                        <Button
                          aria-label={t('移除文件')}
                          icon={<XCircle size={15} />}
                          size='small'
                          theme='borderless'
                          type='tertiary'
                          onClick={() => removeImportFile(item.uid)}
                        />
                      </div>
                    ))
                  ) : (
                    <div className='ct-channel-account-import-empty'>
                      {t('还没有选择文件')}
                    </div>
                  )}
                </div>
              </Tabs.TabPane>
              <Tabs.TabPane
                itemKey='paste'
                tab={
                  <span className='ct-channel-account-import-tab'>
                    <KeyRound size={14} />
                    {t('粘贴导入')}
                  </span>
                }
              >
                <TextArea
                  value={importCredentials}
                  onChange={setImportCredentials}
                  autosize={{ minRows: 8, maxRows: 14 }}
                  placeholder={t('每行一个账号凭证，也支持 JSON 对象或 JSON 数组')}
                  showClear
                />
              </Tabs.TabPane>
            </Tabs>
            <Checkbox
              checked={importOnlyNew}
              onChange={(event) => setImportOnlyNew(event.target.checked)}
            >
              {t('只导入新增账号')}
            </Checkbox>
            <Text type='tertiary' size='small'>
              {t('可同时上传文件并粘贴凭证；导入后会追加到当前渠道账号池，不会在列表中展示完整凭证')}
            </Text>
          </div>
        </Modal>
        <ProxyEditorModal
          visible={proxyEditorVisible}
          proxy={editingProxy}
          onCancel={closeProxyEditModal}
          onSaved={handleProxyEdited}
        />
      </div>
    </div>
  );
}

function MetricCard({ icon, label, value, detail }) {
  return (
    <div className='ct-channel-account-metric'>
      <div>
        <div className='ct-channel-account-metric-label'>{label}</div>
        <div className='ct-channel-account-metric-value'>{value}</div>
        <div className='ct-channel-account-metric-detail'>{detail}</div>
      </div>
      <div className='ct-channel-account-metric-icon'>{icon}</div>
    </div>
  );
}

export default ChannelAccount;
