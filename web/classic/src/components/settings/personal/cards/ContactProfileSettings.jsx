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

import React, { useMemo, useState } from 'react';
import { Avatar, Button, Card, Form, Typography } from '@douyinfe/semi-ui';
import { Mail } from 'lucide-react';
import { API, showError, showSuccess } from '../../../../helpers';
import { getUserPreferredName } from '../../../../helpers/userDisplay';

const ContactProfileSettings = ({ t, userState, userDispatch }) => {
  const user = userState?.user || {};
  const [saving, setSaving] = useState(false);
  const initValues = useMemo(
    () => ({
      contact_name: user.contact_name || '',
      contact_email: user.contact_email || '',
      contact_qq: user.contact_qq || '',
      contact_other: user.contact_other || '',
    }),
    [user.contact_email, user.contact_name, user.contact_other, user.contact_qq],
  );
  const previewUser = { ...user, ...initValues };

  const submit = async (values) => {
    setSaving(true);
    try {
      const payload = {
        contact_name: values.contact_name || '',
        contact_email: values.contact_email || '',
        contact_qq: values.contact_qq || '',
        contact_other: values.contact_other || '',
      };
      const res = await API.put('/api/user/self', payload);
      if (!res?.data?.success) {
        showError(res?.data?.message || t('保存失败'));
        return;
      }
      const nextUser = { ...user, ...payload };
      userDispatch({ type: 'login', payload: nextUser });
      localStorage.setItem('user', JSON.stringify(nextUser));
      showSuccess(t('联系资料已保存'));
    } catch (error) {
      showError(error?.response?.data?.message || error.message || t('保存失败'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card className='!rounded-2xl shadow-sm border-0'>
      <div className='flex items-center justify-between gap-3 mb-4'>
        <div className='flex items-center min-w-0'>
          <Avatar size='small' color='amber' className='mr-3 shadow-md'>
            <Mail size={16} />
          </Avatar>
          <div className='min-w-0'>
            <Typography.Text className='text-lg font-medium'>
              {t('联系资料')}
            </Typography.Text>
            <div className='text-xs text-gray-600 dark:text-gray-400'>
              {t('设置后相关位置会优先显示联系昵称、QQ 或联系邮箱')}
            </div>
          </div>
        </div>
        <div className='hidden sm:block text-xs text-gray-500 truncate'>
          {t('当前显示')}: {getUserPreferredName(previewUser)}
        </div>
      </div>
      <Form initValues={initValues} onSubmit={submit}>
        <div className='grid grid-cols-1 md:grid-cols-2 gap-x-3'>
          <Form.Input
            field='contact_name'
            label={t('联系昵称')}
            placeholder={t('优先显示的联系昵称')}
            showClear
          />
          <Form.Input
            field='contact_qq'
            label='QQ'
            placeholder={t('请输入 QQ')}
            showClear
          />
          <Form.Input
            field='contact_email'
            label={t('联系邮箱')}
            placeholder={t('请输入联系邮箱')}
            showClear
          />
          <Form.Input
            field='contact_other'
            label={t('其他联系方式')}
            placeholder={t('例如 Telegram、微信或其他备注')}
            showClear
          />
        </div>
        <div className='flex justify-end'>
          <Button htmlType='submit' type='primary' theme='solid' loading={saving}>
            {t('保存')}
          </Button>
        </div>
      </Form>
    </Card>
  );
};

export default ContactProfileSettings;
