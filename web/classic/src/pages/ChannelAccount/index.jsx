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
import { useNavigate, useParams } from 'react-router-dom';
import {
  Banner,
  Button,
  Checkbox,
  Empty,
  Input,
  Modal,
  Popconfirm,
  Select,
  Skeleton,
  Space,
  Table,
  Tag,
  TextArea,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  AlertTriangle,
  ArrowLeft,
  BadgeCheck,
  Clock3,
  Fingerprint,
  Gauge,
  KeyRound,
  Pencil,
  Plus,
  PlugZap,
  Search,
  Server,
  RefreshCw,
  ShieldCheck,
  Trash2,
  ToggleLeft,
  ToggleRight,
  UserRoundCog,
} from 'lucide-react';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import './channel-account.css';

const { Text } = Typography;

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function formatNumber(value) {
  return new Intl.NumberFormat().format(Number(value) || 0);
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
  return fallback;
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

function ProxyCell({ record, t, onOpenProxy }) {
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
      <div className='ct-channel-account-proxy-main'>
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
      </div>
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

function buildColumns(
  t,
  onToggleStatus,
  onDeleteAccount,
  onOpenEdit,
  onOpenProxy,
  statusLoadingKey,
  totalAccounts,
) {
  return [
    {
      title: t('账号'),
      dataIndex: 'account_identity',
      width: 280,
      render: (identity = {}, record) => (
        <div className='ct-channel-account-identity'>
          <div className='ct-channel-account-avatar'>
            <UserRoundCog size={17} />
          </div>
          <div>
            <div className='ct-channel-account-name'>
              {identity.display_name ||
                `${t('账号')} #${record.credential_index + 1}`}
              {statusTag(record, t)}
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
      ),
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
        <ProxyCell record={record} t={t} onOpenProxy={onOpenProxy} />
      ),
    },
    {
      title: t('当前评分'),
      dataIndex: 'score',
      key: 'score_summary',
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
      title: t('最近活动'),
      dataIndex: 'recent_activity',
      key: 'recent_activity',
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
      width: 168,
      fixed: 'right',
      render: (_, record) => {
        const action = record?.key_enabled ? 'disable' : 'enable';
        const loadingKey = `${record.channel_id}-${record.credential_index}`;
        const loading = statusLoadingKey === loadingKey;
        const deleteDisabled = Number(totalAccounts || 0) <= 1;
        return (
          <Space spacing={6}>
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
            <Popconfirm
              title={t('确定删除该账号？')}
              content={
                deleteDisabled
                  ? t('至少需要保留一个账号')
                  : t('删除后该凭证将从渠道中移除，此操作不可撤销')
              }
              onConfirm={() => onDeleteAccount(record)}
              disabled={deleteDisabled}
            >
              <Button
                size='small'
                type='danger'
                theme='borderless'
                icon={<Trash2 size={14} />}
                loading={loading}
                disabled={deleteDisabled}
                aria-label={t('删除账号')}
              />
            </Popconfirm>
          </Space>
        );
      },
    },
  ];
}

function accountSearchText(item) {
  const identity = item?.account_identity || {};
  const resource = item?.resource_ref || {};
  const score = item?.score || {};
  const proxy = item?.proxy || {};
  const runtimeKeys = (item?.runtime_keys || [])
    .map((runtime) => runtimeKeyLabel(runtime.runtime_key, (value) => value))
    .join(' ');
  return [
    identity.display_name,
    identity.account_id,
    identity.account_type,
    identity.brand,
    resource.brand,
    item?.subject_short,
    item?.credential_short,
    item?.disabled_reason,
    proxy.name,
    proxy.masked_address,
    proxy.address,
    score.runtime_key && runtimeKeyLabel(score.runtime_key, (value) => value),
    runtimeKeys,
    item?.credential_index != null ? String(item.credential_index + 1) : '',
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
}

function ChannelAccount() {
  const { t } = useTranslation();
  const { id } = useParams();
  const navigate = useNavigate();
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [statusLoadingKey, setStatusLoadingKey] = useState('');
  const [batchLoading, setBatchLoading] = useState(false);
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [keyword, setKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [importVisible, setImportVisible] = useState(false);
  const [importCredentials, setImportCredentials] = useState('');
  const [importOnlyNew, setImportOnlyNew] = useState(true);
  const [importLoading, setImportLoading] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [editRecord, setEditRecord] = useState(null);
  const [editCredentialType, setEditCredentialType] = useState('auto');
  const [editCredential, setEditCredential] = useState('');
  const [editLoading, setEditLoading] = useState(false);
  const [proxyVisible, setProxyVisible] = useState(false);
  const [proxyRecord, setProxyRecord] = useState(null);
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
  const proxyExistingChecked = !createProxyInline;

  const loadAccounts = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await API.get(`/api/channel/${id}/accounts`, {
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
  }, [id, t]);

  useEffect(() => {
    loadAccounts();
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
  }, [id, keyword, statusFilter]);

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
        setData(payload);
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
    [id, t],
  );

  const batchUpdateAccountStatus = useCallback(
    async (enabled) => {
      const selectedIndexes = selectedRowKeys
        .map((key) => Number(String(key).split('-')[1]))
        .filter((value) => Number.isInteger(value) && value >= 0);
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
        setData(payload);
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
    [id, selectedRowKeys, t],
  );

  const deleteAccounts = useCallback(
    async (indexes) => {
      const normalizedIndexes = [...new Set(indexes)]
        .map((value) => Number(value))
        .filter((value) => Number.isInteger(value) && value >= 0);
      if (normalizedIndexes.length === 0) {
        showError(t('请先选择账号'));
        return;
      }
      if (normalizedIndexes.length >= Number(data?.total || 0)) {
        showError(t('至少需要保留一个账号'));
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
        setData(payload);
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
    [data?.total, id, t],
  );

  const deleteSingleAccount = useCallback(
    (record) => deleteAccounts([record.credential_index]),
    [deleteAccounts],
  );

  const batchDeleteAccounts = useCallback(() => {
    const selectedIndexes = selectedRowKeys
      .map((key) => Number(String(key).split('-')[1]))
      .filter((value) => Number.isInteger(value) && value >= 0);
    return deleteAccounts(selectedIndexes);
  }, [deleteAccounts, selectedRowKeys]);

  const importAccounts = useCallback(async () => {
    const credentials = importCredentials.trim();
    if (!credentials) {
      showError(t('请先输入账号凭证'));
      return;
    }
    setImportLoading(true);
    try {
      const response = await API.put(`/api/channel/${id}/accounts`, {
        credentials,
        only_new: importOnlyNew,
      });
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('导入失败'));
      }
      const payload = unwrapApiData(response);
      setData(payload);
      setSelectedRowKeys([]);
      setImportVisible(false);
      setImportCredentials('');
      showSuccess(operationMessage(payload.operation, t, t('导入成功')));
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('导入失败');
      showError(message);
    } finally {
      setImportLoading(false);
    }
  }, [id, importCredentials, importOnlyNew, t]);

  const openEditModal = useCallback((record) => {
    setEditRecord(record);
    setEditCredentialType(record?.account_identity?.account_type || 'auto');
    setEditCredential('');
    setEditVisible(true);
  }, []);

  const closeEditModal = useCallback(() => {
    setEditVisible(false);
    setEditRecord(null);
    setEditCredentialType('auto');
    setEditCredential('');
  }, []);

  const saveAccountCredential = useCallback(async () => {
    const credential = editCredential.trim();
    if (!editRecord) return;
    if (!credential) {
      showError(t('请填写账号凭证'));
      return;
    }
    setEditLoading(true);
    try {
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
      const payload = unwrapApiData(response);
      setData(payload);
      closeEditModal();
      showSuccess(operationMessage(payload.operation, t, t('账号凭证已更新')));
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('保存失败');
      showError(message);
    } finally {
      setEditLoading(false);
    }
  }, [
    closeEditModal,
    editCredential,
    editCredentialType,
    editRecord,
    id,
    t,
  ]);

  const openProxyModal = useCallback(
    (record) => {
      setProxyRecord(record);
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
      setProxyVisible(true);
      loadProxies();
      loadSchedulerConfig();
    },
    [loadProxies, loadSchedulerConfig],
  );

  const closeProxyModal = useCallback(() => {
    setProxyVisible(false);
    setProxyRecord(null);
    setSelectedProxyID(0);
    setCreateProxyInline(false);
  }, []);

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

  const saveProxyBindingRequest = useCallback(async (allowReuseRisk = false) => {
    if (!proxyRecord) return;
    setProxySaving(true);
    try {
      let proxyID = Number(selectedProxyID || 0);
      if (createProxyInline) {
        const address = proxyForm.address.trim();
        if (!address) {
          showError(t('请填写代理地址'));
          setProxySaving(false);
          return;
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
        allowReuseRisk = false;
      }
      const response = await API.post(
        `/api/channel/${id}/accounts/${proxyRecord.credential_index}/proxy`,
        {
          proxy_id: proxyID,
          allow_reuse_risk: allowReuseRisk,
        },
      );
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('操作失败'));
      }
      const payload = unwrapApiData(response);
      setData(payload);
      closeProxyModal();
      showSuccess(
        operationMessage(
          payload.operation,
          t,
          proxyID > 0 ? t('账号代理已绑定') : t('账号代理已解绑'),
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
    id,
    proxyForm,
    proxyRecord,
    selectedProxyID,
    selectedProxyRisk,
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

  const columns = useMemo(
    () =>
      buildColumns(
        t,
        toggleAccountStatus,
        deleteSingleAccount,
        openEditModal,
        openProxyModal,
        statusLoadingKey,
        data?.total,
      ),
    [
      t,
      toggleAccountStatus,
      deleteSingleAccount,
      openEditModal,
      openProxyModal,
      statusLoadingKey,
      data?.total,
    ],
  );
  const items = data?.items || [];
  const filteredItems = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();
    return items.filter((item) => {
      if (statusFilter === 'enabled' && !item.key_enabled) return false;
      if (statusFilter === 'disabled' && item.key_enabled) return false;
      if (
        normalizedKeyword &&
        !accountSearchText(item).includes(normalizedKeyword)
      ) {
        return false;
      }
      return true;
    });
  }, [items, keyword, statusFilter]);
  const selectedCount = selectedRowKeys.length;
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
          <MetricCard
            icon={<KeyRound size={18} />}
            label={t('账号总数')}
            value={formatNumber(data?.total)}
            detail={t('当前渠道可识别凭证')}
          />
          <MetricCard
            icon={<BadgeCheck size={18} />}
            label={t('启用账号')}
            value={formatNumber(data?.enabled)}
            detail={t('可参与智能调度')}
          />
          <MetricCard
            icon={<Gauge size={18} />}
            label={t('已有评分')}
            value={formatNumber(items.filter((item) => item.score).length)}
            detail={t('来自当前运行态快照')}
          />
          <MetricCard
            icon={<Clock3 size={18} />}
            label={t('近30分钟真实样本')}
            value={formatNumber(
              items.reduce(
                (sum, item) =>
                  sum + Number(item.score?.real_sample_count_30m || 0),
                0,
              ),
            )}
            detail={t('用于激活账号评分')}
          />
        </div>

        <div className='ct-channel-account-table-wrap'>
          <div className='ct-channel-account-toolbar'>
            <div className='ct-channel-account-filter-group'>
              <Input
                prefix={<Search size={14} />}
                value={keyword}
                onChange={setKeyword}
                placeholder={t('搜索账号、品牌、凭证或运行键')}
                className='ct-channel-account-search'
              />
              <Select
                value={statusFilter}
                onChange={setStatusFilter}
                prefix={t('状态')}
                className='ct-channel-account-status-select'
              >
                <Select.Option value='all'>{t('全部')}</Select.Option>
                <Select.Option value='enabled'>{t('已启用')}</Select.Option>
                <Select.Option value='disabled'>{t('已禁用')}</Select.Option>
              </Select>
            </div>
            <Space className='ct-channel-account-batch-actions' spacing={8}>
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
              <Popconfirm
                title={t('确定删除所选账号？')}
                content={
                  selectedCount >= Number(data?.total || 0)
                    ? t('至少需要保留一个账号')
                    : t('删除后这些凭证将从渠道中移除，此操作不可撤销')
                }
                onConfirm={batchDeleteAccounts}
                disabled={
                  selectedCount === 0 ||
                  selectedCount >= Number(data?.total || 0)
                }
              >
                <Button
                  size='small'
                  type='danger'
                  theme='light'
                  icon={<Trash2 size={14} />}
                  loading={deleteLoading}
                  disabled={
                    selectedCount === 0 ||
                    selectedCount >= Number(data?.total || 0)
                  }
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
              columns={columns}
              dataSource={filteredItems}
              rowKey={(record) =>
                `${record.channel_id}-${record.credential_index}`
              }
              rowSelection={rowSelection}
              pagination={{
                pageSize: 12,
                showSizeChanger: true,
                pageSizeOpts: [12, 24, 48],
              }}
              empty={<Empty description={t('暂无账号数据')} />}
              scroll={{ x: 1870 }}
              loading={loading}
            />
          )}
        </div>
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
                '为保护密钥安全，列表不会回显完整凭证；编辑时请粘贴新的完整凭证，保存后替换当前账号。',
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
                  placeholder={t('粘贴新的账号凭证；保存后会替换当前凭证')}
                  showClear
                />
              </label>
              <Text type='tertiary' size='small'>
                {t('JSON 类型会在保存前压缩为单行，并只在列表展示账号类型和短指纹。')}
              </Text>
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
                  {proxies.map((proxy) => (
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
        </Modal>
        <Modal
          title={t('导入账号')}
          visible={importVisible}
          width={620}
          okText={t('导入')}
          cancelText={t('取消')}
          confirmLoading={importLoading}
          onOk={importAccounts}
          onCancel={() => {
            setImportVisible(false);
            setImportCredentials('');
          }}
        >
          <div className='ct-channel-account-import-modal'>
            <TextArea
              value={importCredentials}
              onChange={setImportCredentials}
              autosize={{ minRows: 8, maxRows: 14 }}
              placeholder={t('每行一个账号凭证，支持 API Key 或 JSON 授权')}
              showClear
            />
            <Checkbox
              checked={importOnlyNew}
              onChange={(event) => setImportOnlyNew(event.target.checked)}
            >
              {t('只导入新增账号')}
            </Checkbox>
            <Text type='tertiary' size='small'>
              {t('导入后会追加到当前渠道账号池，不会在列表中展示完整凭证')}
            </Text>
          </div>
        </Modal>
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
