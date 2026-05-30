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

import React, { useEffect, useRef, useState } from 'react';
import { Button, Modal, Space, Spin, Table, Tag, Typography } from '@douyinfe/semi-ui';
import { IconRefresh } from '@douyinfe/semi-icons';
import { API, renderNumber, showError, timestamp2string } from '../../../../helpers';

const { Text } = Typography;

function pct(value) {
  const n = Number(value || 0);
  if (!Number.isFinite(n) || n <= 0) return '-';
  return `${(n * 100).toFixed(2)}%`;
}

function count(value) {
  const n = Number(value || 0);
  if (!Number.isFinite(n)) return '0';
  return renderNumber(n);
}

function compactBreakReasons(reasons, t) {
  if (!reasons || typeof reasons !== 'object') return '-';
  const entries = Object.entries(reasons)
    .filter(([, v]) => Number(v || 0) > 0)
    .sort((a, b) => Number(b[1] || 0) - Number(a[1] || 0));
  if (entries.length === 0) return '-';
  return entries
    .slice(0, 2)
    .map(([k, v]) => `${t(k)} ${v}`)
    .join(' / ');
}

function channelLabel(row) {
  if (!row?.channel_id) return '-';
  return row.channel_name ? `${row.channel_id} - ${row.channel_name}` : row.channel_id;
}

function accountLabel(row, t) {
  if (row?.account_id) return row.account_id;
  if (row?.account_identity_key) return row.account_identity_key;
  if (row?.credential_index !== undefined && row?.credential_index !== null) {
    return `${t('账号')} #${Number(row.credential_index) + 1}`;
  }
  return '-';
}

const ChannelAffinityDiagnosticsModal = ({
  t,
  showChannelAffinityDiagnosticsModal,
  setShowChannelAffinityDiagnosticsModal,
  getChannelAffinityDiagnosticsParams,
}) => {
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState(null);
  const requestSeqRef = useRef(0);
  const paramsRef = useRef(null);

  const load = async (nextParams) => {
    const requestParams =
      nextParams || paramsRef.current || getChannelAffinityDiagnosticsParams?.() || {};
    paramsRef.current = requestParams;
    const reqSeq = (requestSeqRef.current += 1);
    setLoading(true);
    try {
      const res = await API.get('/api/log/channel_affinity_diagnostics', {
        params: requestParams,
        disableDuplicate: true,
      });
      if (reqSeq !== requestSeqRef.current) return;
      const { success, message, data: payload } = res.data || {};
      if (!success) {
        setData(null);
        showError(t(message || '请求失败'));
        return;
      }
      setData(payload || null);
    } catch (e) {
      if (reqSeq !== requestSeqRef.current) return;
      setData(null);
      showError(t('请求失败'));
    } finally {
      if (reqSeq === requestSeqRef.current) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    if (!showChannelAffinityDiagnosticsModal) {
      requestSeqRef.current += 1;
      paramsRef.current = null;
      setLoading(false);
      setData(null);
      return;
    }
    const requestParams = getChannelAffinityDiagnosticsParams?.() || {};
    paramsRef.current = requestParams;
    load(requestParams);
  }, [showChannelAffinityDiagnosticsModal]);

  const summary = data?.summary || {};
  const rows = Array.isArray(data?.rows) ? data.rows : [];

  const columns = [
    {
      title: t('渠道'),
      dataIndex: 'channel_id',
      width: 150,
      render: (_, row) => channelLabel(row),
    },
    {
      title: t('账号'),
      dataIndex: 'account_id',
      width: 150,
      render: (_, row) => accountLabel(row, t),
    },
    {
      title: t('模型'),
      dataIndex: 'model_name',
      width: 130,
      render: (text) => text || '-',
    },
    {
      title: t('分组'),
      dataIndex: 'group',
      width: 110,
      render: (text) => text || '-',
    },
    {
      title: t('Key 来源'),
      dataIndex: 'key_source',
      width: 140,
      render: (text) => text || '-',
    },
    {
      title: t('Key 指纹'),
      dataIndex: 'key_fp',
      width: 100,
      render: (text) => text ? `#${text}` : '-',
    },
    {
      title: t('请求命中率'),
      dataIndex: 'hit_rate',
      width: 120,
      render: (_, row) => `${count(row.cache_hits)}/${count(row.total)} (${pct(row.hit_rate)})`,
    },
    {
      title: t('缓存 Token 占比'),
      dataIndex: 'cached_token_rate',
      width: 130,
      render: (_, row) => pct(row.cached_token_rate),
    },
    {
      title: t('统计口径'),
      dataIndex: 'cached_token_rate_mode',
      width: 160,
      render: (text) => text ? t(text) : '-',
    },
    {
      title: t('缓存 Tokens'),
      dataIndex: 'cached_tokens',
      width: 120,
      render: (text) => count(text),
    },
    {
      title: t('Prompt Tokens'),
      dataIndex: 'prompt_tokens',
      width: 120,
      render: (text) => count(text),
    },
    {
      title: t('亲和状态'),
      dataIndex: 'retained',
      width: 130,
      render: (_, row) => (
        <Space spacing={4}>
          {row.retained > 0 ? <Tag color='green'>{t('保留')} {row.retained}</Tag> : null}
          {row.broken > 0 ? <Tag color='red'>{t('打断')} {row.broken}</Tag> : null}
          {!row.retained && !row.broken ? '-' : null}
        </Space>
      ),
    },
    {
      title: t('打断原因'),
      dataIndex: 'break_reasons',
      width: 180,
      render: (value) => compactBreakReasons(value, t),
    },
    {
      title: t('最近一次'),
      dataIndex: 'last_seen_at',
      width: 160,
      render: (value) => value ? timestamp2string(value) : '-',
    },
  ];

  return (
    <Modal
      title={t('缓存亲和诊断')}
      visible={showChannelAffinityDiagnosticsModal}
      onCancel={() => setShowChannelAffinityDiagnosticsModal(false)}
      footer={null}
      centered
      closable
      maskClosable
      width='80vw'
      bodyStyle={{ maxHeight: '70vh', overflow: 'hidden' }}
    >
      <div style={{ padding: 16, height: '70vh', display: 'flex', flexDirection: 'column', gap: 12 }}>
        <div className='flex flex-col lg:flex-row lg:items-center justify-between gap-2'>
          <Space wrap>
            <Tag color='blue'>{t('亲和请求')}: {count(summary.affinity_logs)}</Tag>
            <Tag color='green'>{t('缓存命中')}: {count(summary.cache_hits)} ({pct(summary.hit_rate)})</Tag>
            <Tag color='cyan'>{t('缓存 Token 占比')}: {pct(summary.cached_token_rate)}</Tag>
            <Tag color='orange'>{t('渠道切换')}: {count(summary.channel_switches)}</Tag>
            <Tag color='purple'>{t('账号切换')}: {count(summary.account_switches)}</Tag>
            <Tag color='yellow'>{t('上游未返回缓存')}: {count(summary.upstream_no_cached_token_logs)}</Tag>
            <Tag color='grey'>{t('无亲和 Key')}: {count(summary.no_affinity_logs)}</Tag>
          </Space>
          <Button icon={<IconRefresh />} onClick={() => load()} loading={loading}>
            {t('刷新诊断')}
          </Button>
        </div>
        <Text type='tertiary' size='small'>
          {t('缓存读来自上游 usage，不是本地缓存；这里按日志聚合 prompt_cache_key / previous_response_id 的亲和表现。')}
          {summary.scanned_limit_may_truncate_data ? ` ${t('扫描上限可能截断了结果')}` : ''}
        </Text>
        <Spin spinning={loading} style={{ flex: 1, minHeight: 0 }}>
          <Table
            size='small'
            columns={columns}
            dataSource={rows}
            rowKey={(record) => record.key}
            pagination={false}
            empty={t('暂无诊断数据')}
            scroll={{ x: 1920, y: 420 }}
          />
        </Spin>
      </div>
    </Modal>
  );
};

export default ChannelAffinityDiagnosticsModal;
