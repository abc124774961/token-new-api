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

import React, { useEffect, useState } from 'react';
import {
  API,
  copy,
  showError,
  showNotice,
  getLogo,
  getSystemName,
} from '../../helpers';
import { useSearchParams, Link } from 'react-router-dom';
import { Button, Card, Form, Typography, Banner } from '@douyinfe/semi-ui';
import { IconMail, IconLock, IconCopy } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';

const { Text, Title } = Typography;

const PasswordResetConfirm = () => {
  const { t } = useTranslation();
  const [inputs, setInputs] = useState({
    email: '',
    token: '',
  });
  const { email, token } = inputs;
  const isValidResetLink = email && token;

  const [loading, setLoading] = useState(false);
  const [disableButton, setDisableButton] = useState(false);
  const [countdown, setCountdown] = useState(30);
  const [newPassword, setNewPassword] = useState('');
  const [searchParams, setSearchParams] = useSearchParams();
  const [formApi, setFormApi] = useState(null);

  const logo = getLogo();
  const systemName = getSystemName();

  useEffect(() => {
    let token = searchParams.get('token');
    let email = searchParams.get('email');
    setInputs({
      token: token || '',
      email: email || '',
    });
    if (formApi) {
      formApi.setValues({
        email: email || '',
        newPassword: newPassword || '',
      });
    }
  }, [searchParams, newPassword, formApi]);

  useEffect(() => {
    let countdownInterval = null;
    if (disableButton && countdown > 0) {
      countdownInterval = setInterval(() => {
        setCountdown(countdown - 1);
      }, 1000);
    } else if (countdown === 0) {
      setDisableButton(false);
      setCountdown(30);
    }
    return () => clearInterval(countdownInterval);
  }, [disableButton, countdown]);

  async function handleSubmit(e) {
    if (!email || !token) {
      showError(t('无效的重置链接，请重新发起密码重置请求'));
      return;
    }
    setDisableButton(true);
    setLoading(true);
    const res = await API.post(`/api/user/reset`, {
      email,
      token,
    });
    const { success, message } = res.data;
    if (success) {
      let password = res.data.data;
      setNewPassword(password);
      await copy(password);
      showNotice(`${t('密码已重置并已复制到剪贴板：')} ${password}`);
    } else {
      showError(message);
    }
    setLoading(false);
  }

  return (
    <div className='ct-auth-page ct-auth-simple-page'>
      <div className='ct-auth-bg-grid' aria-hidden='true' />
      <div className='ct-auth-bg-ribbon' aria-hidden='true' />

      <div className='ct-auth-simple-shell'>
        <Card className='ct-auth-card ct-auth-simple-card'>
          <div className='ct-auth-card-inner'>
            <div className='ct-auth-card-header'>
              <div className='ct-auth-logo-mark'>
                <img src={logo} alt='Logo' />
              </div>
              <div className='ct-auth-card-heading'>
                <Title heading={3} className='ct-auth-card-title'>
                  {t('密码重置确认')}
                </Title>
                <Text className='ct-auth-card-subtitle'>{systemName}</Text>
              </div>
            </div>

            {!isValidResetLink && (
              <Banner
                type='danger'
                description={t('无效的重置链接，请重新发起密码重置请求')}
                className='ct-auth-banner'
                closeIcon={null}
              />
            )}

            <Form
              getFormApi={(api) => setFormApi(api)}
              initValues={{
                email: email || '',
                newPassword: newPassword || '',
              }}
              className='ct-auth-form'
            >
              <Form.Input
                className='ct-auth-form-field'
                field='email'
                label={t('邮箱')}
                name='email'
                disabled={true}
                prefix={<IconMail />}
                placeholder={email ? '' : t('等待获取邮箱信息...')}
              />

              {newPassword && (
                <Form.Input
                  className='ct-auth-form-field'
                  field='newPassword'
                  label={t('新密码')}
                  name='newPassword'
                  disabled={true}
                  prefix={<IconLock />}
                  suffix={
                    <Button
                      className='ct-auth-inline-action'
                      icon={<IconCopy />}
                      type='tertiary'
                      theme='borderless'
                      onClick={async () => {
                        await copy(newPassword);
                        showNotice(`${t('密码已复制到剪贴板：')} ${newPassword}`);
                      }}
                    >
                      {t('复制')}
                    </Button>
                  }
                />
              )}

              <div className='ct-auth-actions'>
                <Button
                  theme='solid'
                  className='ct-auth-button ct-auth-button-primary'
                  type='primary'
                  htmlType='submit'
                  onClick={handleSubmit}
                  loading={loading}
                  disabled={disableButton || newPassword || !isValidResetLink}
                >
                  {newPassword ? t('密码重置完成') : t('确认重置密码')}
                </Button>
              </div>
            </Form>

            <div className='ct-auth-register-line'>
              <Text>
                <Link to='/login' className='ct-auth-link'>
                  {t('返回登录')}
                </Link>
              </Text>
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
};

export default PasswordResetConfirm;
