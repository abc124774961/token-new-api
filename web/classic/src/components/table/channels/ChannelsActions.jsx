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
  Dropdown,
  Modal,
  Switch,
  Typography,
  Select,
  Button,
} from '@douyinfe/semi-ui';
import CompactModeToggle from '../../common/ui/CompactModeToggle';
import { AdminPermissionButton } from '../../../apps/admin-console/permissions/AdminPermissionAction';
import { ADMIN_PERMISSION_KEYS } from '../../../apps/admin-console/permissions/adminPermissions.config';
import { RotateCcw } from 'lucide-react';
import {
  flattenChannelRows,
  isRecoverableHealthChannel,
} from './channelHealthUtils';

const ChannelsActions = ({
  enableBatchDelete,
  batchDeleteChannels,
  setShowBatchSetTag,
  testAllChannels,
  fixChannelsAbilities,
  updateAllChannelsBalance,
  recoverVisibleChannelHealth,
  recoveringChannelHealth,
  deleteAllDisabledChannels,
  applyAllUpstreamUpdates,
  detectAllUpstreamUpdates,
  openGroupSummaryModal,
  detectAllUpstreamUpdatesLoading,
  applyAllUpstreamUpdatesLoading,
  compactMode,
  setCompactMode,
  idSort,
  setIdSort,
  setEnableBatchDelete,
  enableTagMode,
  setEnableTagMode,
  statusFilter,
  setStatusFilter,
  getFormValues,
  loadChannels,
  searchChannels,
  activeTypeKey,
  activePage,
  pageSize,
  setActivePage,
  selectedChannels = [],
  channels = [],
  t,
}) => {
  const selectedRecoverableCount = flattenChannelRows(selectedChannels).filter(
    isRecoverableHealthChannel,
  ).length;
  const visibleRecoverableCount = flattenChannelRows(channels).filter(
    isRecoverableHealthChannel,
  ).length;
  const recoverTargetCount =
    selectedChannels.length > 0
      ? selectedRecoverableCount
      : visibleRecoverableCount;

  return (
    <div className='ct-channel-actions flex flex-col gap-2'>
      {/* 第一行：批量操作按钮 + 设置开关 */}
      <div className='flex flex-col md:flex-row justify-between gap-2'>
        {/* 左侧：批量操作按钮 */}
        <div className='ct-channel-action-group flex flex-wrap md:flex-nowrap items-center gap-2 w-full md:w-auto order-2 md:order-1'>
          <AdminPermissionButton
            size='small'
            disabled={!enableBatchDelete}
            type='danger'
            className='w-full md:w-auto'
            dangerPermission={ADMIN_PERMISSION_KEYS.channelChannelDanger}
            fallbackTooltip={t(
              '没有删除渠道权限，请联系渠道管理员或超级管理员。',
            )}
            onClick={() => {
              Modal.confirm({
                title: t('确定是否要删除所选通道？'),
                content: t('此修改将不可逆'),
                onOk: () => batchDeleteChannels(),
              });
            }}
          >
            {t('删除所选通道')}
          </AdminPermissionButton>

          <Button
            size='small'
            disabled={!enableBatchDelete}
            type='tertiary'
            onClick={() => setShowBatchSetTag(true)}
            className='w-full md:w-auto'
          >
            {t('批量设置标签')}
          </Button>

          <Dropdown
            size='small'
            trigger='click'
            render={
              <Dropdown.Menu>
                <Dropdown.Item>
                  <Button
                    size='small'
                    type='tertiary'
                    className='w-full'
                    loading={detectAllUpstreamUpdatesLoading}
                    disabled={detectAllUpstreamUpdatesLoading}
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定？'),
                        content: t('确定要测试所有未手动禁用渠道吗？'),
                        onOk: () => testAllChannels(),
                        size: 'small',
                        centered: true,
                      });
                    }}
                  >
                    {t('测试所有未手动禁用渠道')}
                  </Button>
                </Dropdown.Item>
                <Dropdown.Item>
                  <Button
                    size='small'
                    className='w-full'
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定是否要修复数据库一致性？'),
                        content: t(
                          '进行该操作时，可能导致渠道访问错误，请仅在数据库出现问题时使用',
                        ),
                        onOk: () => fixChannelsAbilities(),
                        size: 'sm',
                        centered: true,
                      });
                    }}
                  >
                    {t('修复数据库一致性')}
                  </Button>
                </Dropdown.Item>
                <Dropdown.Item>
                  <Button
                    size='small'
                    type='secondary'
                    className='w-full'
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定？'),
                        content: t('确定要更新所有已启用通道余额吗？'),
                        onOk: () => updateAllChannelsBalance(),
                        size: 'sm',
                        centered: true,
                      });
                    }}
                  >
                    {t('更新所有已启用通道余额')}
                  </Button>
                </Dropdown.Item>
                <Dropdown.Item>
                  <Button
                    size='small'
                    type='tertiary'
                    className='w-full'
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定？'),
                        content: t(
                          '确定要仅检测全部渠道上游模型更新吗？（不执行新增/删除）',
                        ),
                        onOk: () => detectAllUpstreamUpdates(),
                        size: 'sm',
                        centered: true,
                      });
                    }}
                  >
                    {t('检测全部渠道上游更新')}
                  </Button>
                </Dropdown.Item>
                <Dropdown.Item>
                  <Button
                    size='small'
                    type='primary'
                    className='w-full'
                    loading={applyAllUpstreamUpdatesLoading}
                    disabled={applyAllUpstreamUpdatesLoading}
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定？'),
                        content: t('确定要对全部渠道执行上游模型更新吗？'),
                        onOk: () => applyAllUpstreamUpdates(),
                        size: 'sm',
                        centered: true,
                      });
                    }}
                  >
                    {t('处理全部渠道上游更新')}
                  </Button>
                </Dropdown.Item>
                <Dropdown.Item>
                  <AdminPermissionButton
                    size='small'
                    type='danger'
                    className='w-full'
                    dangerPermission={
                      ADMIN_PERMISSION_KEYS.channelChannelDanger
                    }
                    fallbackTooltip={t(
                      '没有删除渠道权限，请联系渠道管理员或超级管理员。',
                    )}
                    onClick={() => {
                      Modal.confirm({
                        title: t('确定是否要删除禁用通道？'),
                        content: t('此修改将不可逆'),
                        onOk: () => deleteAllDisabledChannels(),
                        size: 'sm',
                        centered: true,
                      });
                    }}
                  >
                    {t('删除禁用通道')}
                  </AdminPermissionButton>
                </Dropdown.Item>
              </Dropdown.Menu>
            }
          >
            <Button
              size='small'
              theme='light'
              type='tertiary'
              className='w-full md:w-auto'
            >
              {t('批量操作')}
            </Button>
          </Dropdown>

          <CompactModeToggle
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />

          <Button
            size='small'
            theme='light'
            type='tertiary'
            className='w-full md:w-auto'
            onClick={openGroupSummaryModal}
          >
            {t('分组列表')}
          </Button>

          <AdminPermissionButton
            size='small'
            theme='light'
            type='tertiary'
            icon={<RotateCcw size={14} />}
            className='w-full md:w-auto'
            loading={recoveringChannelHealth}
            disabled={recoveringChannelHealth || recoverTargetCount === 0}
            requiredPermission={ADMIN_PERMISSION_KEYS.channelHealthExecute}
            fallbackTooltip={t('没有渠道健康执行权限，请联系管理员。')}
            onClick={() => {
              Modal.confirm({
                title: t('确定要恢复 {{count}} 个异常渠道健康状态吗？', {
                  count: recoverTargetCount,
                }),
                content: t(
                  '将清除余额不足、冷却和熔断等运行态标记；未真正恢复的渠道可能会在后续调度中再次进入异常。',
                ),
                onOk: () => recoverVisibleChannelHealth(),
                size: 'small',
                centered: true,
              });
            }}
          >
            {t('一键恢复健康')}
          </AdminPermissionButton>
        </div>

        {/* 右侧：设置开关区域 */}
        <div className='ct-channel-switch-group flex flex-col md:flex-row items-start md:items-center gap-2 w-full md:w-auto order-1 md:order-2'>
          <div className='ct-channel-switch-item flex items-center justify-between w-full md:w-auto'>
            <Typography.Text strong className='ct-channel-action-label mr-2'>
              {t('使用ID排序')}
            </Typography.Text>
            <Switch
              size='small'
              checked={idSort}
              onChange={(v) => {
                localStorage.setItem('id-sort', v + '');
                setIdSort(v);
                const { searchKeyword, searchGroup, searchModel } =
                  getFormValues();
                if (
                  searchKeyword === '' &&
                  searchGroup === '' &&
                  searchModel === ''
                ) {
                  loadChannels(activePage, pageSize, v, enableTagMode);
                } else {
                  searchChannels(
                    enableTagMode,
                    activeTypeKey,
                    statusFilter,
                    activePage,
                    pageSize,
                    v,
                  );
                }
              }}
            />
          </div>

          <div className='ct-channel-switch-item flex items-center justify-between w-full md:w-auto'>
            <Typography.Text strong className='ct-channel-action-label mr-2'>
              {t('开启批量操作')}
            </Typography.Text>
            <Switch
              size='small'
              checked={enableBatchDelete}
              onChange={(v) => {
                localStorage.setItem('enable-batch-delete', v + '');
                setEnableBatchDelete(v);
              }}
            />
          </div>

          <div className='ct-channel-switch-item flex items-center justify-between w-full md:w-auto'>
            <Typography.Text strong className='ct-channel-action-label mr-2'>
              {t('标签聚合模式')}
            </Typography.Text>
            <Switch
              size='small'
              checked={enableTagMode}
              onChange={(v) => {
                localStorage.setItem('enable-tag-mode', v + '');
                setEnableTagMode(v);
                setActivePage(1);
                loadChannels(1, pageSize, idSort, v);
              }}
            />
          </div>

          <div className='ct-channel-switch-item flex items-center justify-between w-full md:w-auto'>
            <Typography.Text strong className='ct-channel-action-label mr-2'>
              {t('状态筛选')}
            </Typography.Text>
            <Select
              size='small'
              value={statusFilter}
              style={{ minWidth: 116 }}
              onChange={(v) => {
                localStorage.setItem('channel-status-filter', v);
                setStatusFilter(v);
                setActivePage(1);
                loadChannels(
                  1,
                  pageSize,
                  idSort,
                  enableTagMode,
                  activeTypeKey,
                  v,
                );
              }}
            >
              <Select.Option value='enabled'>{t('已启用')}</Select.Option>
              <Select.Option value='disabled'>{t('已禁用')}</Select.Option>
              <Select.Option value='all'>{t('全部含禁用')}</Select.Option>
            </Select>
          </div>
        </div>
      </div>
    </div>
  );
};

export default ChannelsActions;
