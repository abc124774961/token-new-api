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

import React from 'react';
import {
  Button,
  Dropdown,
  Space,
  SplitButtonGroup,
  Tag,
  AvatarGroup,
  Avatar,
  Tooltip,
  Progress,
  Popover,
  Typography,
  Input,
  Modal,
} from '@douyinfe/semi-ui';
import {
  Ban,
  Edit3,
  KeyRound,
  MessageSquareText,
  Power,
  ShieldCheck,
  Trash2,
} from 'lucide-react';
import {
  timestamp2string,
  renderGroup,
  renderQuota,
  getModelCategories,
  showError,
} from '../../../helpers';
import {
  IconTreeTriangleDown,
  IconCopy,
  IconEyeOpened,
  IconEyeClosed,
} from '@douyinfe/semi-icons';

// progress color helper
const getProgressColor = (pct) => {
  if (pct === 100) return 'var(--semi-color-success)';
  if (pct <= 10) return 'var(--semi-color-danger)';
  if (pct <= 30) return 'var(--semi-color-warning)';
  return undefined;
};

// Render functions
function renderTimestamp(timestamp) {
  return <span className='ct-token-time'>{timestamp2string(timestamp)}</span>;
}

// Render status column only (no usage)
const getStatusMeta = (status, t) => {
  if (status === 1) {
    return {
      tone: 'enabled',
      label: t('已启用'),
    };
  }
  if (status === 2) {
    return {
      tone: 'disabled',
      label: t('已禁用'),
    };
  }
  if (status === 3) {
    return {
      tone: 'expired',
      label: t('已过期'),
    };
  }
  if (status === 4) {
    return {
      tone: 'exhausted',
      label: t('已耗尽'),
    };
  }
  return {
    tone: 'unknown',
    label: t('未知状态'),
  };
};

const renderName = (text, record, t) => {
  const statusMeta = getStatusMeta(record?.status, t);
  return (
    <div className='ct-token-name-cell'>
      <div className={`ct-token-name-icon is-${statusMeta.tone}`}>
        <KeyRound size={16} />
      </div>
      <div className='ct-token-name-copy'>
        <strong>{text || '-'}</strong>
        <span>
          {t('令牌 ID')} #{record?.id || '-'}
        </span>
      </div>
    </div>
  );
};

const renderStatus = (text, record, t) => {
  const statusMeta = getStatusMeta(text, t);

  return (
    <span className={`ct-token-status-pill is-${statusMeta.tone}`}>
      <span />
      {statusMeta.label}
    </span>
  );
};

// Render group column
const formatGroupRatioValue = (ratio) => {
  const numericRatio = Number(ratio);
  if (!Number.isFinite(numericRatio) || numericRatio <= 0) {
    return null;
  }
  return `${numericRatio.toFixed(4).replace(/0+$/, '').replace(/\.$/, '')}x`;
};

const getGroupBillingInfo = (groupRatioValue) => {
  if (
    groupRatioValue &&
    typeof groupRatioValue === 'object' &&
    !Array.isArray(groupRatioValue)
  ) {
    return {
      ratio: groupRatioValue.ratio,
      dynamicBilling: groupRatioValue.dynamic_billing || null,
    };
  }
  return {
    ratio: groupRatioValue,
    dynamicBilling: null,
  };
};

const getDynamicBillingRatio = (dynamicBilling) => {
  if (!dynamicBilling) {
    return null;
  }
  return formatGroupRatioValue(
    dynamicBilling.current_ratio ||
      dynamicBilling.average_ratio_7d ||
      dynamicBilling.max_ratio_7d ||
      dynamicBilling.min_ratio_7d,
  );
};

const renderGroupRatioTag = (groupInfo, t) => {
  const dynamicRatio = getDynamicBillingRatio(groupInfo.dynamicBilling);
  if (dynamicRatio) {
    return (
      <Tag size='small' color='cyan' shape='circle' type='light'>
        {t('动态倍率')} {dynamicRatio}
      </Tag>
    );
  }

  if (groupInfo.dynamicBilling) {
    return (
      <Tag size='small' color='cyan' shape='circle' type='light'>
        {t('动态倍率计算中')}
      </Tag>
    );
  }

  const staticRatio = formatGroupRatioValue(groupInfo.ratio);
  if (!staticRatio) {
    return null;
  }
  return (
    <Tag size='small' color='green' shape='circle'>
      {staticRatio}
    </Tag>
  );
};

const renderGroupColumn = (text, record, t, groupRatios = {}) => {
  const normalizedGroup = text || record?.group || 'auto';
  const groupInfo = getGroupBillingInfo(groupRatios[normalizedGroup]);
  const ratioTag = renderGroupRatioTag(groupInfo, t);
  const groupTag =
    normalizedGroup === 'auto' ? (
      <Tooltip
        content={t(
          '当前分组为 auto，会自动选择最优分组，当一个组不可用时自动降级到下一个组（熔断机制）',
        )}
        position='top'
      >
        <Tag color='white' shape='circle'>
          {t('auto')}
          {record && record.cross_group_retry ? `(${t('跨分组')})` : ''}
        </Tag>
      </Tooltip>
    ) : (
      renderGroup(normalizedGroup)
    );

  return (
    <span className='flex items-center gap-1 flex-wrap'>
      {groupTag}
      {ratioTag}
    </span>
  );
};

// Render token key column with show/hide and copy functionality
const renderTokenKey = (
  text,
  record,
  showKeys,
  resolvedTokenKeys,
  loadingTokenKeys,
  toggleTokenVisibility,
  copyTokenKey,
  copyTokenConnectionString,
  t,
) => {
  const revealed = !!showKeys[record.id];
  const loading = !!loadingTokenKeys[record.id];
  const keyValue =
    revealed && resolvedTokenKeys[record.id]
      ? resolvedTokenKeys[record.id]
      : record.key || '';
  const displayedKey = keyValue ? `sk-${keyValue}` : '';

  return (
    <div className='ct-token-key-cell'>
      <Input
        readOnly
        value={displayedKey}
        size='small'
        className='ct-token-key-input'
        suffix={
          <div className='ct-token-key-actions'>
            <Button
              theme='borderless'
              size='small'
              type='tertiary'
              icon={revealed ? <IconEyeClosed /> : <IconEyeOpened />}
              loading={loading}
              aria-label='toggle token visibility'
              onClick={async (e) => {
                e.stopPropagation();
                await toggleTokenVisibility(record);
              }}
            />
            <Dropdown
              trigger='click'
              position='bottomRight'
              clickToHide
              menu={[
                {
                  node: 'item',
                  name: t('复制密钥'),
                  onClick: () => copyTokenKey(record),
                },
                {
                  node: 'item',
                  name: t('复制连接信息'),
                  onClick: () => copyTokenConnectionString(record),
                },
              ]}
            >
              <Button
                theme='borderless'
                size='small'
                type='tertiary'
                icon={<IconCopy />}
                loading={loading}
                aria-label='copy token key'
                onClick={async (e) => {
                  e.stopPropagation();
                }}
              />
            </Dropdown>
          </div>
        }
      />
    </div>
  );
};

// Render model limits column
const renderModelLimits = (text, record, t) => {
  if (record.model_limits_enabled && text) {
    const models = text.split(',').filter(Boolean);
    const categories = getModelCategories(t);

    const vendorAvatars = [];
    const matchedModels = new Set();
    Object.entries(categories).forEach(([key, category]) => {
      if (key === 'all') return;
      if (!category.icon || !category.filter) return;
      const vendorModels = models.filter((m) =>
        category.filter({ model_name: m }),
      );
      if (vendorModels.length > 0) {
        vendorAvatars.push(
          <Tooltip
            key={key}
            content={vendorModels.join(', ')}
            position='top'
            showArrow
          >
            <Avatar
              size='extra-extra-small'
              alt={category.label}
              color='transparent'
            >
              {category.icon}
            </Avatar>
          </Tooltip>,
        );
        vendorModels.forEach((m) => matchedModels.add(m));
      }
    });

    const unmatchedModels = models.filter((m) => !matchedModels.has(m));
    if (unmatchedModels.length > 0) {
      vendorAvatars.push(
        <Tooltip
          key='unknown'
          content={unmatchedModels.join(', ')}
          position='top'
          showArrow
        >
          <Avatar size='extra-extra-small' alt='unknown'>
            {t('其他')}
          </Avatar>
        </Tooltip>,
      );
    }

    return (
      <div className='ct-token-model-limit'>
        <AvatarGroup size='extra-extra-small'>{vendorAvatars}</AvatarGroup>
        <span>{t('{{count}} 个模型', { count: models.length })}</span>
      </div>
    );
  } else {
    return (
      <Tag color='white' shape='circle' className='ct-token-soft-tag'>
        {t('无限制')}
      </Tag>
    );
  }
};

// Render IP restrictions column
const renderAllowIps = (text, t) => {
  if (!text || text.trim() === '') {
    return (
      <Tag color='white' shape='circle' className='ct-token-soft-tag'>
        {t('无限制')}
      </Tag>
    );
  }

  const ips = text
    .split('\n')
    .map((ip) => ip.trim())
    .filter(Boolean);

  const displayIps = ips.slice(0, 1);
  const extraCount = ips.length - displayIps.length;

  const ipTags = displayIps.map((ip, idx) => (
    <Tag key={idx} shape='circle'>
      {ip}
    </Tag>
  ));

  if (extraCount > 0) {
    ipTags.push(
      <Tooltip
        key='extra'
        content={ips.slice(1).join(', ')}
        position='top'
        showArrow
      >
        <Tag shape='circle'>{'+' + extraCount}</Tag>
      </Tooltip>,
    );
  }

  return <Space wrap>{ipTags}</Space>;
};

// Render separate quota usage column
const renderQuotaUsage = (text, record, t) => {
  const { Paragraph } = Typography;
  const used = parseInt(record.used_quota) || 0;
  const remain = parseInt(record.remain_quota) || 0;
  const total = used + remain;
  if (record.unlimited_quota) {
    const popoverContent = (
      <div className='text-xs p-2'>
        <Paragraph copyable={{ content: renderQuota(used) }}>
          {t('已用额度')}: {renderQuota(used)}
        </Paragraph>
      </div>
    );
    return (
      <Popover content={popoverContent} position='top'>
        <div className='ct-token-quota is-unlimited'>
          <div>
            <strong>{t('无限额度')}</strong>
            <span>{t('已用 {{quota}}', { quota: renderQuota(used) })}</span>
          </div>
          <ShieldCheck size={15} />
        </div>
      </Popover>
    );
  }
  const percent = total > 0 ? (remain / total) * 100 : 0;
  const popoverContent = (
    <div className='text-xs p-2'>
      <Paragraph copyable={{ content: renderQuota(used) }}>
        {t('已用额度')}: {renderQuota(used)}
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(remain) }}>
        {t('剩余额度')}: {renderQuota(remain)} ({percent.toFixed(0)}%)
      </Paragraph>
      <Paragraph copyable={{ content: renderQuota(total) }}>
        {t('总额度')}: {renderQuota(total)}
      </Paragraph>
    </div>
  );
  return (
    <Popover content={popoverContent} position='top'>
      <div className='ct-token-quota'>
        <div className='ct-token-quota-head'>
          <strong>{renderQuota(remain)}</strong>
          <span>{percent.toFixed(0)}%</span>
        </div>
        <Progress
          percent={percent}
          stroke={getProgressColor(percent)}
          aria-label='quota usage'
          format={() => ''}
          className='ct-token-quota-progress'
        />
        <small>
          {t('总额度')} {renderQuota(total)}
        </small>
      </div>
    </Popover>
  );
};

// Render operations column
const renderOperations = (
  text,
  record,
  onOpenLink,
  setEditingToken,
  setShowEdit,
  manageToken,
  refresh,
  t,
) => {
  let chatsArray = [];
  try {
    const raw = localStorage.getItem('chats');
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      for (let i = 0; i < parsed.length; i++) {
        const item = parsed[i];
        const name = Object.keys(item)[0];
        if (!name) continue;
        chatsArray.push({
          node: 'item',
          key: i,
          name,
          value: item[name],
          onClick: () => onOpenLink(name, item[name], record),
        });
      }
    }
  } catch (_) {
    showError(t('聊天链接配置错误，请联系管理员'));
  }

  return (
    <Space wrap className='ct-token-row-actions'>
      <SplitButtonGroup
        className='ct-token-chat-button'
        aria-label={t('项目操作按钮组')}
      >
        <Button
          size='small'
          type='tertiary'
          theme='light'
          icon={<MessageSquareText size={14} />}
          onClick={() => {
            if (chatsArray.length === 0) {
              showError(t('请联系管理员配置聊天链接'));
            } else {
              const first = chatsArray[0];
              onOpenLink(first.name, first.value, record);
            }
          }}
        >
          {t('聊天')}
        </Button>
        <Dropdown trigger='click' position='bottomRight' menu={chatsArray}>
          <Button
            type='tertiary'
            icon={<IconTreeTriangleDown />}
            size='small'
          ></Button>
        </Dropdown>
      </SplitButtonGroup>

      {record.status === 1 ? (
        <Button
          type='danger'
          theme='light'
          size='small'
          icon={<Ban size={14} />}
          onClick={async () => {
            await manageToken(record.id, 'disable', record);
            await refresh();
          }}
        >
          {t('禁用')}
        </Button>
      ) : (
        <Button
          size='small'
          type='primary'
          theme='light'
          icon={<Power size={14} />}
          onClick={async () => {
            await manageToken(record.id, 'enable', record);
            await refresh();
          }}
        >
          {t('启用')}
        </Button>
      )}

      <Button
        type='tertiary'
        theme='light'
        size='small'
        icon={<Edit3 size={14} />}
        onClick={() => {
          setEditingToken(record);
          setShowEdit(true);
        }}
      >
        {t('编辑')}
      </Button>

      <Button
        type='danger'
        theme='borderless'
        size='small'
        icon={<Trash2 size={14} />}
        onClick={() => {
          Modal.confirm({
            title: t('确定是否要删除此令牌？'),
            content: t('此修改将不可逆'),
            onOk: () => {
              (async () => {
                await manageToken(record.id, 'delete', record);
                await refresh();
              })();
            },
          });
        }}
      >
        {t('删除')}
      </Button>
    </Space>
  );
};

export const getTokensColumns = ({
  t,
  showKeys,
  resolvedTokenKeys,
  loadingTokenKeys,
  toggleTokenVisibility,
  copyTokenKey,
  copyTokenConnectionString,
  manageToken,
  onOpenLink,
  setEditingToken,
  setShowEdit,
  refresh,
  groupRatios = {},
}) => {
  return [
    {
      title: t('名称'),
      dataIndex: 'name',
      key: 'name',
      width: 210,
      render: (text, record) => renderName(text, record, t),
    },
    {
      title: t('状态'),
      dataIndex: 'status',
      key: 'status',
      width: 112,
      render: (text, record) => renderStatus(text, record, t),
    },
    {
      title: t('剩余额度/总额度'),
      key: 'quota_usage',
      width: 170,
      render: (text, record) => renderQuotaUsage(text, record, t),
    },
    {
      title: t('分组'),
      dataIndex: 'group',
      key: 'group',
      width: 210,
      render: (text, record) => renderGroupColumn(text, record, t, groupRatios),
    },
    {
      title: t('密钥'),
      key: 'token_key',
      width: 260,
      render: (text, record) =>
        renderTokenKey(
          text,
          record,
          showKeys,
          resolvedTokenKeys,
          loadingTokenKeys,
          toggleTokenVisibility,
          copyTokenKey,
          copyTokenConnectionString,
          t,
        ),
    },
    {
      title: t('可用模型'),
      dataIndex: 'model_limits',
      key: 'model_limits',
      width: 120,
      render: (text, record) => renderModelLimits(text, record, t),
    },
    {
      title: t('IP限制'),
      dataIndex: 'allow_ips',
      key: 'allow_ips',
      width: 120,
      render: (text) => renderAllowIps(text, t),
    },
    {
      title: t('创建时间'),
      dataIndex: 'created_time',
      key: 'created_time',
      width: 170,
      render: (text, record, index) => {
        return <div>{renderTimestamp(text)}</div>;
      },
    },
    {
      title: t('最后使用时间'),
      dataIndex: 'accessed_time',
      key: 'accessed_time',
      width: 170,
      render: (text, record, index) => {
        return <div>{text ? renderTimestamp(text) : '-'}</div>;
      },
    },
    {
      title: t('过期时间'),
      dataIndex: 'expired_time',
      key: 'expired_time',
      width: 150,
      render: (text, record, index) => {
        return (
          <div>
            {record.expired_time === -1 ? (
              <span className='ct-token-time is-forever'>{t('永不过期')}</span>
            ) : (
              renderTimestamp(text)
            )}
          </div>
        );
      },
    },
    {
      title: '',
      dataIndex: 'operate',
      key: 'operate',
      fixed: 'right',
      width: 300,
      render: (text, record, index) =>
        renderOperations(
          text,
          record,
          onOpenLink,
          setEditingToken,
          setShowEdit,
          manageToken,
          refresh,
          t,
        ),
    },
  ];
};
