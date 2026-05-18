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

import React, { useMemo } from 'react';
import { Button, Empty, Modal, Space, Table, Tag, Typography } from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { Layers, Network, RefreshCw, ShieldCheck } from 'lucide-react';
import { CHANNEL_OPTIONS } from '../../../../constants';
import {
  getChannelIcon,
  renderChannelCapabilities,
  renderGroup,
} from '../../../../helpers';

const EMPTY_IMAGE_SIZE = { width: 150, height: 150 };

const formatNumber = (value) =>
  new Intl.NumberFormat().format(Number(value) || 0);

const getChannelTypeMeta = (type) => {
  const numericType = Number(type);
  return (
    CHANNEL_OPTIONS.find((item) => item.value === numericType) || {
      value: numericType,
      label: String(type),
      color: 'grey',
    }
  );
};

const MetricTile = ({ icon: Icon, label, value, detail }) => (
  <div className='rounded-lg border border-slate-100 bg-white/80 px-4 py-3 shadow-[0_8px_20px_rgba(15,23,42,0.04)]'>
    <div className='flex items-center justify-between gap-3'>
      <div>
        <div className='text-[12px] font-semibold text-slate-500'>{label}</div>
        <div className='mt-1 font-mono text-2xl font-black text-slate-900'>
          {value}
        </div>
      </div>
      <span className='inline-flex h-9 w-9 items-center justify-center rounded-lg bg-cyan-50 text-teal-700'>
        <Icon size={18} />
      </span>
    </div>
    {detail ? (
      <div className='mt-2 text-[12px] font-medium text-slate-500'>{detail}</div>
    ) : null}
  </div>
);

const ChannelGroupSummaryModal = ({
  visible,
  onCancel,
  data,
  loading,
  onRefresh,
  t,
}) => {
  const groups = data?.groups || [];
  const summary = data?.summary || {};

  const columns = useMemo(
    () => [
      {
        title: t('分组'),
        dataIndex: 'group',
        width: 180,
        fixed: 'left',
        render: (group) => <div className='min-w-[140px]'>{renderGroup(group)}</div>,
      },
      {
        title: t('渠道统计'),
        dataIndex: 'total_channels',
        width: 190,
        render: (_, record) => (
          <div className='flex flex-wrap gap-1'>
            <Tag color='white' type='ghost' shape='circle'>
              {t('总数')} {formatNumber(record.total_channels)}
            </Tag>
            <Tag color='green' type='light' shape='circle'>
              {t('已启用')} {formatNumber(record.enabled_channels)}
            </Tag>
            <Tag color='red' type='light' shape='circle'>
              {t('已禁用')} {formatNumber(record.disabled_channels)}
            </Tag>
            {Number(record.auto_disabled_channels) > 0 ? (
              <Tag color='orange' type='light' shape='circle'>
                {t('自动禁用')} {formatNumber(record.auto_disabled_channels)}
              </Tag>
            ) : null}
          </div>
        ),
      },
      {
        title: t('模型统计'),
        dataIndex: 'enabled_models',
        width: 160,
        render: (_, record) => (
          <div className='flex flex-col gap-1'>
            <Typography.Text strong>
              {formatNumber(record.enabled_models)} /{' '}
              {formatNumber(record.total_models)}
            </Typography.Text>
            <Typography.Text type='secondary' size='small'>
              {t('启用模型 / 全部模型')}
            </Typography.Text>
          </div>
        ),
      },
      {
        title: t('渠道类型'),
        dataIndex: 'type_counts',
        width: 230,
        render: (typeCounts = {}) => (
          <div className='flex max-w-[220px] flex-wrap gap-1'>
            {Object.entries(typeCounts)
              .sort(([, a], [, b]) => Number(b) - Number(a))
              .map(([type, count]) => {
                const meta = getChannelTypeMeta(type);
                return (
                  <Tag
                    key={type}
                    color={meta.color}
                    type='light'
                    shape='circle'
                    prefixIcon={getChannelIcon(Number(type))}
                    size='small'
                  >
                    {meta.label} {formatNumber(count)}
                  </Tag>
                );
              })}
          </div>
        ),
      },
      {
        title: t('分组技能'),
        dataIndex: 'capabilities',
        width: 240,
        render: (capabilities) =>
          renderChannelCapabilities(
            capabilities || [],
            t,
            'inline-flex max-w-[230px] flex-wrap gap-1',
          ),
      },
      {
        title: t('Codex 工具'),
        dataIndex: 'codex_supported_tools',
        width: 180,
        render: (tools = []) => {
          if (!tools.length) {
            return <Typography.Text type='tertiary'>--</Typography.Text>;
          }
          return (
            <div className='flex max-w-[170px] flex-wrap gap-1'>
              {tools.map((tool) => (
                <Tag key={tool} color='violet' type='light' shape='circle' size='small'>
                  {tool}
                </Tag>
              ))}
            </div>
          );
        },
      },
      {
        title: t('模型样例'),
        dataIndex: 'sample_models',
        width: 280,
        render: (models = [], record) => {
          if (!models.length) {
            return <Typography.Text type='tertiary'>--</Typography.Text>;
          }
          const rest = Math.max(0, Number(record.enabled_models) - models.length);
          return (
            <div className='flex max-w-[270px] flex-wrap gap-1'>
              {models.map((model) => (
                <Tag key={model} color='grey' type='light' shape='circle' size='small'>
                  {model}
                </Tag>
              ))}
              {rest > 0 ? (
                <Tag color='white' type='ghost' shape='circle' size='small'>
                  +{formatNumber(rest)}
                </Tag>
              ) : null}
            </div>
          );
        },
      },
    ],
    [t],
  );

  return (
    <Modal
      title={
        <div className='flex items-center gap-2'>
          <Network size={17} />
          <span>{t('渠道分组列表')}</span>
        </div>
      }
      visible={visible}
      onCancel={onCancel}
      width={1280}
      style={{ maxWidth: '96vw' }}
      footer={
        <Space>
          <Button
            icon={<RefreshCw size={15} />}
            theme='light'
            type='tertiary'
            loading={loading}
            onClick={onRefresh}
          >
            {t('刷新')}
          </Button>
          <Button type='primary' theme='solid' onClick={onCancel}>
            {t('关闭')}
          </Button>
        </Space>
      }
    >
      <div className='space-y-4'>
        <div className='grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4'>
          <MetricTile
            icon={Network}
            label={t('分组数')}
            value={formatNumber(summary.total_groups)}
            detail={t('渠道分组覆盖情况')}
          />
          <MetricTile
            icon={ShieldCheck}
            label={t('启用渠道')}
            value={`${formatNumber(summary.enabled_channels)} / ${formatNumber(
              summary.total_channels,
            )}`}
            detail={t('启用渠道 / 全部渠道')}
          />
          <MetricTile
            icon={Layers}
            label={t('启用模型')}
            value={`${formatNumber(summary.enabled_models)} / ${formatNumber(
              summary.total_models,
            )}`}
            detail={t('启用模型 / 全部模型')}
          />
          <MetricTile
            icon={RefreshCw}
            label={t('能力类型')}
            value={formatNumber(Object.keys(summary.capability_counts || {}).length)}
            detail={t('来自能力缓存字段')}
          />
        </div>

        <Table
          columns={columns}
          dataSource={groups}
          rowKey='group'
          loading={loading}
          pagination={false}
          size='small'
          scroll={{ x: 1460, y: 520 }}
          empty={
            <Empty
              title={t('暂无渠道分组')}
              image={<IllustrationNoResult style={EMPTY_IMAGE_SIZE} />}
              darkModeImage={
                <IllustrationNoResultDark style={EMPTY_IMAGE_SIZE} />
              }
            />
          }
        />
      </div>
    </Modal>
  );
};

export default ChannelGroupSummaryModal;
