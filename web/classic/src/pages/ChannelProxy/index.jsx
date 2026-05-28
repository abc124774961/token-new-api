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
  Banner,
  Button,
  Empty,
  Input,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Clock3,
  EyeOff,
  Fingerprint,
  KeyRound,
  ListChecks,
  Network,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Server,
  ShieldCheck,
} from 'lucide-react';
import { API, showError, showSuccess, timestamp2string } from '../../helpers';
import './channel-proxy.css';

const { Text } = Typography;

const emptyProxyForm = {
  name: '',
  protocol: 'socks5',
  address: '',
  username: '',
  password: '',
  enabled: true,
  remark: '',
};

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function formatNumber(value) {
  return new Intl.NumberFormat().format(Number(value) || 0);
}

function formatTimestamp(timestamp) {
  return Number(timestamp || 0) > 0
    ? timestamp2string(Number(timestamp))
    : '--';
}

function protocolLabel(protocol) {
  return String(protocol || 'socks5').toUpperCase();
}

function proxyAddress(proxy) {
  return proxy?.masked_address || proxy?.address || '';
}

function proxyReuseRisks(proxy) {
  return Array.isArray(proxy?.reuse_risks) ? proxy.reuse_risks : [];
}

function reuseRiskText(risk, t) {
  if (!risk) return '';
  return t('同品牌 {{brand}} 已有 {{total}} 个账号使用该代理', {
    brand: risk.brand || risk.provider || t('未知品牌'),
    total: Number(risk.distinct_subject_count || risk.account_count || 0),
  });
}

function usageIdentity(usage, t) {
  const parts = [
    usage?.brand || usage?.provider || t('未知品牌'),
    usage?.account_id ? `${t('账号')} ${usage.account_id}` : '',
    usage?.credential_index != null
      ? `${t('凭证序号')} #${Number(usage.credential_index) + 1}`
      : '',
  ].filter(Boolean);
  return parts.join(' · ');
}

function proxySearchText(proxy) {
  const usages = Array.isArray(proxy?.brand_usage) ? proxy.brand_usage : [];
  return [
    proxy?.id,
    proxy?.name,
    proxy?.protocol,
    proxyAddress(proxy),
    proxy?.username,
    proxy?.remark,
    ...usages.flatMap((usage) => [
      usage?.brand,
      usage?.provider,
      usage?.channel_id,
      usage?.account_id,
      usage?.credential_subject_fingerprint,
      usage?.credential_index,
    ]),
  ]
    .filter((value) => value !== undefined && value !== null && value !== '')
    .join(' ')
    .toLowerCase();
}

function statusTag(proxy, t) {
  if (!proxy?.enabled) {
    return (
      <Tag color='grey' type='light' shape='circle'>
        {t('已禁用')}
      </Tag>
    );
  }
  if (Number(proxy?.failure_count || 0) > 0 && Number(proxy?.last_failure_at || 0) > Number(proxy?.last_success_at || 0)) {
    return (
      <Tag color='orange' type='light' shape='circle'>
        {t('最近失败')}
      </Tag>
    );
  }
  return (
    <Tag color='green' type='light' shape='circle'>
      {t('可用')}
    </Tag>
  );
}

function ChannelProxy() {
  const { t } = useTranslation();
  const [proxies, setProxies] = useState([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [keyword, setKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [modalVisible, setModalVisible] = useState(false);
  const [editingProxy, setEditingProxy] = useState(null);
  const [usageProxy, setUsageProxy] = useState(null);
  const [form, setForm] = useState(emptyProxyForm);

  const loadProxies = useCallback(async () => {
    setLoading(true);
    setError('');
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
      setError(message);
      showError(message);
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    loadProxies();
  }, [loadProxies]);

  const openCreateModal = useCallback(() => {
    setEditingProxy(null);
    setForm(emptyProxyForm);
    setModalVisible(true);
  }, []);

  const openEditModal = useCallback((proxy) => {
    setEditingProxy(proxy);
    setForm({
      name: proxy?.name || '',
      protocol: proxy?.protocol || 'socks5',
      address: '',
      username: proxy?.username || '',
      password: '',
      enabled: proxy?.enabled !== false,
      remark: proxy?.remark || '',
    });
    setModalVisible(true);
  }, []);

  const closeModal = useCallback(() => {
    setModalVisible(false);
    setEditingProxy(null);
    setForm(emptyProxyForm);
  }, []);

  const saveProxy = useCallback(async () => {
    const isEditing = Boolean(editingProxy?.id);
    if (!isEditing && !form.address.trim()) {
      showError(t('请填写代理地址'));
      return;
    }
    setSaving(true);
    try {
      const payload = {
        ...form,
        name: form.name.trim(),
        address: form.address.trim(),
        username: form.username.trim(),
        remark: form.remark.trim(),
      };
      const response = isEditing
        ? await API.put(`/api/model_gateway/proxies/${editingProxy.id}`, payload)
        : await API.post('/api/model_gateway/proxies', payload);
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('保存失败'));
      }
      closeModal();
      showSuccess(isEditing ? t('代理已更新') : t('代理已创建'));
      await loadProxies();
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('保存失败');
      showError(message);
    } finally {
      setSaving(false);
    }
  }, [closeModal, editingProxy, form, loadProxies, t]);

  const toggleProxyEnabled = useCallback(
    async (proxy) => {
      const nextEnabled = !proxy?.enabled;
      setSaving(true);
      try {
        const response = await API.put(`/api/model_gateway/proxies/${proxy.id}`, {
          name: proxy.name,
          protocol: proxy.protocol,
          address: '',
          username: proxy.username || '',
          password: '',
          enabled: nextEnabled,
          remark: proxy.remark || '',
        });
        if (response?.data?.success === false) {
          throw new Error(response?.data?.message || t('操作失败'));
        }
        showSuccess(nextEnabled ? t('代理已启用') : t('代理已禁用'));
        await loadProxies();
      } catch (err) {
        const message =
          err?.response?.data?.message || err?.message || t('操作失败');
        showError(message);
      } finally {
        setSaving(false);
      }
    },
    [loadProxies, t],
  );

  const filteredProxies = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();
    return proxies.filter((proxy) => {
      if (statusFilter === 'enabled' && proxy.enabled === false) return false;
      if (statusFilter === 'disabled' && proxy.enabled !== false) return false;
      if (normalizedKeyword && !proxySearchText(proxy).includes(normalizedKeyword)) {
        return false;
      }
      return true;
    });
  }, [keyword, proxies, statusFilter]);

  const metrics = useMemo(() => {
    const enabled = proxies.filter((proxy) => proxy.enabled !== false).length;
    const used = proxies.filter(
      (proxy) =>
        Number(proxy.use_count || 0) > 0 ||
        (Array.isArray(proxy.brand_usage) && proxy.brand_usage.length > 0),
    ).length;
    const brandCount = new Set(
      proxies.flatMap((proxy) =>
        (proxy.brand_usage || []).map((usage) => usage.brand || usage.provider),
      ).filter(Boolean),
    ).size;
    const failed = proxies.filter(
      (proxy) =>
        Number(proxy.failure_count || 0) > 0 &&
        Number(proxy.last_failure_at || 0) > Number(proxy.last_success_at || 0),
    ).length;
    const reuseRisk = proxies.filter(
      (proxy) => proxyReuseRisks(proxy).length > 0,
    ).length;
    return { enabled, used, brandCount, failed, reuseRisk };
  }, [proxies]);

  const columns = useMemo(
    () => [
      {
        title: t('代理资源'),
        dataIndex: 'name',
        width: 330,
        render: (_, record) => (
          <div className='ct-channel-proxy-identity'>
            <div className='ct-channel-proxy-avatar'>
              <Network size={17} />
            </div>
            <div className='ct-channel-proxy-main'>
              <div className='ct-channel-proxy-name'>
                <span title={record.name}>{record.name || `#${record.id}`}</span>
                <Tag color='cyan' type='light' shape='circle'>
                  ID {record.id}
                </Tag>
              </div>
              <div className='ct-channel-proxy-address' title={proxyAddress(record)}>
                {proxyAddress(record) || '--'}
              </div>
              {record.remark ? (
                <div className='ct-channel-proxy-remark' title={record.remark}>
                  {record.remark}
                </div>
              ) : null}
            </div>
          </div>
        ),
      },
      {
        title: t('协议'),
        dataIndex: 'protocol',
        width: 110,
        render: (protocol) => (
          <Tag color='blue' type='light' shape='circle'>
            {protocolLabel(protocol)}
          </Tag>
        ),
      },
      {
        title: t('认证'),
        dataIndex: 'password_set',
        width: 130,
        render: (_, record) => (
          <div className='ct-channel-proxy-auth'>
            {record.username ? (
              <Tag color='teal' type='light' shape='circle'>
                {record.username}
              </Tag>
            ) : null}
            {record.password_set ? (
              <Tooltip content={t('密码已保存，列表不会展示明文')}>
                <Tag color='grey' type='light' shape='circle'>
                  <EyeOff size={12} />
                  {t('密码')}
                </Tag>
              </Tooltip>
            ) : null}
            {!record.username && !record.password_set ? (
              <Text type='tertiary'>--</Text>
            ) : null}
          </div>
        ),
      },
      {
        title: t('状态'),
        dataIndex: 'enabled',
        width: 130,
        render: (_, record) => (
          <Space spacing={8}>
            {statusTag(record, t)}
            <Switch
              size='small'
              checked={record.enabled !== false}
              loading={saving}
              onChange={() => toggleProxyEnabled(record)}
            />
          </Space>
        ),
      },
      {
        title: t('使用情况'),
        dataIndex: 'brand_usage',
        width: 310,
        render: (_, record) => {
          const usages = Array.isArray(record.brand_usage)
            ? record.brand_usage
            : [];
          const risks = proxyReuseRisks(record);
          return (
            <div className='ct-channel-proxy-usage-cell'>
              <div className='ct-channel-proxy-usage-tags'>
                {risks.length > 0 ? (
                  <Tooltip content={reuseRiskText(risks[0], t)}>
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
                {usages.length > 0 ? (
                  usages.slice(0, 3).map((usage, index) => (
                    <Tooltip
                      key={`${usage.channel_id}-${usage.credential_index}-${index}`}
                      content={usageIdentity(usage, t)}
                    >
                      <Tag color='green' type='light' shape='circle'>
                        {usage.brand || usage.provider || t('未知品牌')}
                      </Tag>
                    </Tooltip>
                  ))
                ) : (
                  <Text type='tertiary'>{t('暂无账号绑定')}</Text>
                )}
                {usages.length > 3 ? (
                  <Tag color='grey' type='light' shape='circle'>
                    +{usages.length - 3}
                  </Tag>
                ) : null}
              </div>
              <Button
                size='small'
                type='tertiary'
                theme='borderless'
                icon={<ListChecks size={14} />}
                onClick={() => setUsageProxy(record)}
              >
                {t('使用记录')}
              </Button>
            </div>
          );
        },
      },
      {
        title: t('最近使用'),
        dataIndex: 'last_used_at',
        width: 190,
        render: (_, record) => (
          <div className='ct-channel-proxy-time'>
            <span>{formatTimestamp(record.last_used_at)}</span>
            <span>
              {t('成功')}: {formatTimestamp(record.last_success_at)}
            </span>
          </div>
        ),
      },
      {
        title: t('计数'),
        dataIndex: 'use_count',
        width: 150,
        render: (_, record) => (
          <div className='ct-channel-proxy-counts'>
            <span>
              {t('使用')} {formatNumber(record.use_count)}
            </span>
            <span>
              {t('失败')} {formatNumber(record.failure_count)}
            </span>
          </div>
        ),
      },
      {
        title: t('操作'),
        dataIndex: 'action',
        fixed: 'right',
        width: 120,
        render: (_, record) => (
          <Button
            size='small'
            type='primary'
            theme='light'
            icon={<Pencil size={14} />}
            onClick={() => openEditModal(record)}
          >
            {t('编辑')}
          </Button>
        ),
      },
    ],
    [openEditModal, saving, t, toggleProxyEnabled],
  );

  const usageColumns = useMemo(
    () => [
      {
        title: t('品牌'),
        dataIndex: 'brand',
        width: 130,
        render: (_, record) => record.brand || record.provider || '--',
      },
      {
        title: t('渠道'),
        dataIndex: 'channel_id',
        width: 90,
        render: (value) => (value ? `#${value}` : '--'),
      },
      {
        title: t('账号'),
        dataIndex: 'account_id',
        width: 170,
        render: (_, record) => (
          <div className='ct-channel-proxy-usage-account'>
            <span>{record.account_id || '--'}</span>
            {record.credential_index != null ? (
              <Text type='tertiary'>
                {t('凭证序号')} #{Number(record.credential_index) + 1}
              </Text>
            ) : null}
          </div>
        ),
      },
      {
        title: t('凭证主体指纹'),
        dataIndex: 'credential_subject_fingerprint',
        render: (value) => (
          <span className='ct-channel-proxy-fingerprint'>
            <Fingerprint size={13} />
            {value || '--'}
          </span>
        ),
      },
      {
        title: t('状态'),
        dataIndex: 'last_status',
        width: 120,
        render: (value) => (
          <Tag color={value === 'bound' ? 'cyan' : 'green'} type='light'>
            {value === 'bound' ? t('已绑定') : value || '--'}
          </Tag>
        ),
      },
      {
        title: t('最后使用'),
        dataIndex: 'last_used_at',
        width: 180,
        render: formatTimestamp,
      },
    ],
    [t],
  );

  return (
    <div className='ct-console-content-wrap'>
      <div className='ct-channel-proxy-page'>
        <div className='ct-channel-proxy-hero'>
          <div className='ct-channel-proxy-title-block'>
            <div className='ct-channel-proxy-title-icon'>
              <ShieldCheck size={22} />
            </div>
            <div>
              <div className='ct-channel-proxy-eyebrow'>
                {t('渠道账号代理')}
              </div>
              <h2>{t('代理管理')}</h2>
              <p>{t('独立维护 SOCKS5 代理资源，记录品牌和账号使用情况')}</p>
            </div>
          </div>
          <Space className='ct-channel-proxy-actions' spacing={8}>
            <Button
              icon={<Plus size={15} />}
              type='primary'
              theme='light'
              onClick={openCreateModal}
            >
              {t('新增代理')}
            </Button>
            <Button
              icon={<RefreshCw size={15} />}
              type='primary'
              theme='solid'
              loading={loading}
              onClick={loadProxies}
            >
              {t('刷新')}
            </Button>
          </Space>
        </div>

        {error ? (
          <Banner
            type='danger'
            closeIcon={null}
            description={<span className='ct-channel-proxy-error'>{error}</span>}
          />
        ) : null}

        <div className='ct-channel-proxy-metric-grid'>
          <MetricCard
            icon={<Server size={18} />}
            label={t('代理总数')}
            value={formatNumber(proxies.length)}
            detail={t('独立代理资源')}
          />
          <MetricCard
            icon={<CheckCircle2 size={18} />}
            label={t('启用代理')}
            value={formatNumber(metrics.enabled)}
            detail={t('可参与账号访问')}
          />
          <MetricCard
            icon={<KeyRound size={18} />}
            label={t('已绑定代理')}
            value={formatNumber(metrics.used)}
            detail={t('存在账号使用记录')}
          />
          <MetricCard
            icon={<Activity size={18} />}
            label={t('复用风险')}
            value={formatNumber(metrics.reuseRisk)}
            detail={
              metrics.failed > 0
                ? t('{{total}} 个代理最近失败', { total: metrics.failed })
                : t('同品牌多账号共用出口提醒')
            }
          />
        </div>

        <div className='ct-channel-proxy-table-wrap'>
          <div className='ct-channel-proxy-toolbar'>
            <div className='ct-channel-proxy-filter-group'>
              <Input
                prefix={<Search size={14} />}
                value={keyword}
                onChange={setKeyword}
                placeholder={t('搜索代理、地址、品牌或账号')}
                className='ct-channel-proxy-search'
              />
              <Select
                value={statusFilter}
                onChange={setStatusFilter}
                prefix={t('状态')}
                className='ct-channel-proxy-status-select'
              >
                <Select.Option value='all'>{t('全部')}</Select.Option>
                <Select.Option value='enabled'>{t('已启用')}</Select.Option>
                <Select.Option value='disabled'>{t('已禁用')}</Select.Option>
              </Select>
            </div>
            <Text type='tertiary'>
              {t('共 {{total}} 个代理', { total: filteredProxies.length })}
            </Text>
          </div>
          <Table
            size='small'
            columns={columns}
            dataSource={filteredProxies}
            rowKey='id'
            pagination={{
              pageSize: 12,
              showSizeChanger: true,
              pageSizeOpts: [12, 24, 48],
            }}
            empty={<Empty description={t('暂无代理数据')} />}
            scroll={{ x: 1440 }}
            loading={loading}
          />
        </div>

        <Modal
          title={editingProxy ? t('编辑代理') : t('新增代理')}
          visible={modalVisible}
          width={680}
          okText={t('保存')}
          cancelText={t('取消')}
          confirmLoading={saving}
          onOk={saveProxy}
          onCancel={closeModal}
        >
          <div className='ct-channel-proxy-form'>
            {editingProxy ? (
              <Banner
                type='info'
                closeIcon={null}
                description={t('编辑时代理地址和密码留空会保留原值，列表不会展示完整密码')}
              />
            ) : null}
            <Input
              value={form.name}
              onChange={(value) => setForm((prev) => ({ ...prev, name: value }))}
              placeholder={t('代理名称（可选）')}
            />
            <div className='ct-channel-proxy-form-row'>
              <Select
                value={form.protocol}
                onChange={(value) =>
                  setForm((prev) => ({ ...prev, protocol: value }))
                }
                className='ct-channel-proxy-protocol-select'
              >
                <Select.Option value='socks5'>SOCKS5</Select.Option>
                <Select.Option value='socks5h'>SOCKS5H</Select.Option>
                <Select.Option value='http'>HTTP</Select.Option>
                <Select.Option value='https'>HTTPS</Select.Option>
              </Select>
              <Input
                value={form.address}
                onChange={(value) =>
                  setForm((prev) => ({ ...prev, address: value }))
                }
                placeholder={
                  editingProxy
                    ? t('留空保持原地址，当前：{{address}}', {
                        address: proxyAddress(editingProxy) || '--',
                      })
                    : '127.0.0.1:1080'
                }
              />
            </div>
            <div className='ct-channel-proxy-form-row'>
              <Input
                value={form.username}
                onChange={(value) =>
                  setForm((prev) => ({ ...prev, username: value }))
                }
                placeholder={t('代理用户名（可选）')}
              />
              <Input
                type='password'
                value={form.password}
                onChange={(value) =>
                  setForm((prev) => ({ ...prev, password: value }))
                }
                placeholder={
                  editingProxy?.password_set
                    ? t('留空保持原密码')
                    : t('代理密码（可选）')
                }
              />
            </div>
            <Input
              value={form.remark}
              onChange={(value) =>
                setForm((prev) => ({ ...prev, remark: value }))
              }
              placeholder={t('备注（可选）')}
            />
            <div className='ct-channel-proxy-form-enabled'>
              <Text>{t('启用代理')}</Text>
              <Switch
                checked={form.enabled}
                onChange={(checked) =>
                  setForm((prev) => ({ ...prev, enabled: checked }))
                }
              />
            </div>
          </div>
        </Modal>

        <Modal
          title={t('代理使用记录')}
          visible={Boolean(usageProxy)}
          width={860}
          footer={null}
          onCancel={() => setUsageProxy(null)}
        >
          <div className='ct-channel-proxy-usage-modal'>
            <div className='ct-channel-proxy-usage-head'>
              <div>
                <Text strong>{usageProxy?.name || '--'}</Text>
                <div>
                  <Text type='tertiary'>
                    {proxyAddress(usageProxy)} · {protocolLabel(usageProxy?.protocol)}
                  </Text>
                </div>
              </div>
              {usageProxy?.enabled === false ? (
                <Tag color='grey' type='light' shape='circle'>
                  {t('已禁用')}
                </Tag>
              ) : (
                <Tag color='green' type='light' shape='circle'>
                  {t('可用')}
                </Tag>
              )}
            </div>
            <Table
              size='small'
              columns={usageColumns}
              dataSource={usageProxy?.brand_usage || []}
              rowKey={(record, index) =>
                `${record.channel_id}-${record.credential_index}-${index}`
              }
              pagination={{ pageSize: 8 }}
              empty={<Empty description={t('暂无代理使用记录')} />}
            />
            <div className='ct-channel-proxy-usage-foot'>
              <Clock3 size={14} />
              <span>
                {t('最近使用')}: {formatTimestamp(usageProxy?.last_used_at)}
              </span>
              {Number(usageProxy?.last_failure_at || 0) > Number(usageProxy?.last_success_at || 0) ? (
                <span className='ct-channel-proxy-warning'>
                  <AlertTriangle size={14} />
                  {t('最近失败')}: {formatTimestamp(usageProxy?.last_failure_at)}
                </span>
              ) : null}
            </div>
          </div>
        </Modal>
      </div>
    </div>
  );
}

function MetricCard({ icon, label, value, detail }) {
  return (
    <div className='ct-channel-proxy-metric'>
      <div>
        <div className='ct-channel-proxy-metric-label'>{label}</div>
        <div className='ct-channel-proxy-metric-value'>{value}</div>
        <div className='ct-channel-proxy-metric-detail'>{detail}</div>
      </div>
      <div className='ct-channel-proxy-metric-icon'>{icon}</div>
    </div>
  );
}

export default ChannelProxy;
