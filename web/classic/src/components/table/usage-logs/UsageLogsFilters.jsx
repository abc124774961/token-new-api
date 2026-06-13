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
import { Button, Form } from '@douyinfe/semi-ui';
import { IconSearch } from '@douyinfe/semi-icons';
import {
  PiArrowClockwiseDuotone,
  PiColumnsDuotone,
  PiMagnifyingGlassDuotone,
} from 'react-icons/pi';

import { DATE_RANGE_PRESETS } from '../../../constants/console.constants';
import {
  adminDangerousOperationPermissions,
  adminMenuPermissions,
  adminOperationPermissions,
} from '../../../apps/admin-console/permissions/adminPermissions.config';

const adminAuditPermissionOptions = [
  ...adminMenuPermissions,
  ...adminDangerousOperationPermissions.map((item) => ({
    ...item,
    label: item.operation,
    group: item.page,
  })),
  ...adminOperationPermissions.map((item) => ({
    ...item,
    label: item.operation,
  })),
];

const LogsFilters = ({
  formInitValues,
  setFormApi,
  refresh,
  setShowColumnSelector,
  formApi,
  setLogType,
  loading,
  isAdminUser,
  initialLogType,
  groupOptions = [],
  t,
}) => {
  const showAdminAuditFilters = isAdminUser && initialLogType === 3;

  return (
    <Form
      className='ct-usage-logs-filter-form'
      initValues={formInitValues}
      getFormApi={(api) => setFormApi(api)}
      onSubmit={refresh}
      allowEmpty={true}
      autoComplete='off'
      layout='vertical'
      trigger='change'
      stopValidateWithError={false}
    >
      <div className='ct-usage-logs-filter-content'>
        <div className='ct-usage-logs-filter-grid'>
          <div className='ct-usage-logs-filter-field ct-usage-logs-filter-field-time'>
            <span>{t('时间范围')}</span>
            <Form.DatePicker
              field='dateRange'
              className='w-full'
              type='dateTimeRange'
              placeholder={[t('开始时间'), t('结束时间')]}
              showClear
              pure
              size='small'
              presets={DATE_RANGE_PRESETS.map((preset) => ({
                text: t(preset.text),
                start: preset.start(),
                end: preset.end(),
              }))}
            />
          </div>

          <div className='ct-usage-logs-filter-field'>
            <span>{t('令牌名称')}</span>
            <Form.Input
              field='token_name'
              prefix={<IconSearch />}
              placeholder={t('令牌名称')}
              showClear
              pure
              size='small'
            />
          </div>

          <div className='ct-usage-logs-filter-field'>
            <span>{t('模型名称')}</span>
            <Form.Input
              field='model_name'
              prefix={<IconSearch />}
              placeholder={t('模型名称')}
              showClear
              pure
              size='small'
            />
          </div>

          <div className='ct-usage-logs-filter-field'>
            <span>{t('分组')}</span>
            {groupOptions.length > 0 ? (
              <Form.Select
                field='group'
                placeholder={t('分组')}
                optionList={groupOptions}
                showClear
                filter
                pure
                size='small'
              />
            ) : (
              <Form.Input
                field='group'
                prefix={<IconSearch />}
                placeholder={t('分组')}
                showClear
                pure
                size='small'
              />
            )}
          </div>

          <div className='ct-usage-logs-filter-field'>
            <span>{t('Request ID')}</span>
            <Form.Input
              field='request_id'
              prefix={<IconSearch />}
              placeholder={t('Request ID')}
              showClear
              pure
              size='small'
            />
          </div>

          {isAdminUser && (
            <>
              <div className='ct-usage-logs-filter-field'>
                <span>{t('渠道 ID')}</span>
                <Form.Input
                  field='channel'
                  prefix={<IconSearch />}
                  placeholder={t('渠道 ID')}
                  showClear
                  pure
                  size='small'
                />
              </div>
              <div className='ct-usage-logs-filter-field'>
                <span>{t('用户名称')}</span>
                <Form.Input
                  field='username'
                  prefix={<IconSearch />}
                  placeholder={t('用户名称')}
                  showClear
                  pure
                  size='small'
                />
              </div>
            </>
          )}

          {showAdminAuditFilters && (
            <>
              <div className='ct-usage-logs-filter-field'>
                <span>{t('权限点')}</span>
                <Form.Select
                  field='audit_permission'
                  placeholder={t('权限点')}
                  showClear
                  filter
                  pure
                  size='small'
                >
                  {adminAuditPermissionOptions.map((item) => (
                    <Form.Select.Option
                      key={item.permission}
                      value={item.permission}
                    >
                      {`${t(item.group)} / ${t(item.label)}`}
                    </Form.Select.Option>
                  ))}
                </Form.Select>
              </div>
              <div className='ct-usage-logs-filter-field'>
                <span>{t('权限来源')}</span>
                <Form.Select
                  field='audit_source'
                  placeholder={t('权限来源')}
                  showClear
                  pure
                  size='small'
                >
                  <Form.Select.Option value='role_compatibility'>
                    {t('固定角色兼容')}
                  </Form.Select.Option>
                  <Form.Select.Option value='database'>
                    {t('数据库权限')}
                  </Form.Select.Option>
                  <Form.Select.Option value='root'>
                    {t('超级管理员')}
                  </Form.Select.Option>
                </Form.Select>
              </div>
              <div className='ct-usage-logs-filter-field'>
                <span>{t('操作结果')}</span>
                <Form.Select
                  field='audit_result'
                  placeholder={t('操作结果')}
                  showClear
                  pure
                  size='small'
                >
                  <Form.Select.Option value='completed'>
                    {t('完成')}
                  </Form.Select.Option>
                  <Form.Select.Option value='denied'>
                    {t('拒绝')}
                  </Form.Select.Option>
                  <Form.Select.Option value='error'>
                    {t('错误')}
                  </Form.Select.Option>
                  <Form.Select.Option value='http_error'>
                    {t('HTTP 错误')}
                  </Form.Select.Option>
                  <Form.Select.Option value='aborted'>
                    {t('中止')}
                  </Form.Select.Option>
                </Form.Select>
              </div>
              <div className='ct-usage-logs-filter-field'>
                <span>{t('审计操作人')}</span>
                <Form.Input
                  field='audit_operator'
                  prefix={<IconSearch />}
                  placeholder={t('审计操作人')}
                  showClear
                  pure
                  size='small'
                />
              </div>
              <div className='ct-usage-logs-filter-field'>
                <span>{t('目标用户 ID')}</span>
                <Form.Input
                  field='audit_target_user_id'
                  prefix={<IconSearch />}
                  placeholder={t('目标用户 ID')}
                  showClear
                  pure
                  size='small'
                />
              </div>
            </>
          )}
        </div>

        <div className='ct-usage-logs-filter-footer'>
          <div className='ct-usage-logs-filter-type'>
            <span>{t('日志类型')}</span>
            <Form.Select
              field='logType'
              placeholder={t('日志类型')}
              className='ct-usage-logs-log-type-select'
              showClear
              pure
              onChange={() => {
                // 延迟执行搜索，让表单值先更新
                setTimeout(() => {
                  refresh();
                }, 0);
              }}
              size='small'
            >
              <Form.Select.Option value='0'>{t('全部')}</Form.Select.Option>
              <Form.Select.Option value='1'>{t('充值')}</Form.Select.Option>
              <Form.Select.Option value='2'>{t('消费')}</Form.Select.Option>
              <Form.Select.Option value='3'>{t('管理')}</Form.Select.Option>
              <Form.Select.Option value='4'>{t('系统')}</Form.Select.Option>
              <Form.Select.Option value='5'>{t('错误')}</Form.Select.Option>
              <Form.Select.Option value='6'>{t('退款')}</Form.Select.Option>
            </Form.Select>
          </div>

          <div className='ct-usage-logs-filter-buttons'>
            <Button
              className='ct-usage-logs-query-button'
              type='primary'
              htmlType='submit'
              loading={loading}
              size='small'
              icon={<PiMagnifyingGlassDuotone size={15} />}
            >
              {t('查询')}
            </Button>
            <Button
              className='ct-usage-logs-reset-button'
              type='tertiary'
              theme='borderless'
              onClick={() => {
                if (formApi) {
                  formApi.reset();
                  setLogType(initialLogType || 0);
                  setTimeout(() => {
                    refresh();
                  }, 100);
                }
              }}
              size='small'
              icon={<PiArrowClockwiseDuotone size={15} />}
            >
              {t('重置')}
            </Button>
            <Button
              className='ct-usage-logs-column-button'
              type='tertiary'
              theme='borderless'
              onClick={() => setShowColumnSelector(true)}
              size='small'
              icon={<PiColumnsDuotone size={15} />}
            >
              {t('列设置')}
            </Button>
          </div>
        </div>
      </div>
    </Form>
  );
};

export default LogsFilters;
