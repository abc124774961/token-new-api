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
import { Button, Spin, Tag } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import {
  AlertCircle,
  Loader2,
  RefreshCw,
  Server,
  Settings,
  WifiOff,
} from 'lucide-react';
import ConsolePageShell from '../layout/ConsolePageShell';

const DeploymentGuardState = ({
  tone = 'warning',
  icon,
  eyebrow,
  title,
  subtitle,
  badge,
  children,
  actions,
}) => (
  <ConsolePageShell
    className='ct-deployment-guard-page'
    eyebrow={eyebrow}
    title={title}
    subtitle={subtitle}
    badge={badge}
  >
    <section className={`ct-deployment-guard-card is-${tone}`}>
      <div className='ct-deployment-guard-icon'>{icon}</div>
      <div className='ct-deployment-guard-copy'>{children}</div>
      {actions && <div className='ct-deployment-guard-actions'>{actions}</div>}
    </section>
  </ConsolePageShell>
);

const DeploymentAccessGuard = ({
  children,
  loading,
  isEnabled,
  connectionLoading,
  connectionOk,
  connectionError,
  onRetry,
}) => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const handleGoToSettings = () => {
    navigate('/admin/settings?tab=model-deployment');
  };

  if (loading) {
    return (
      <DeploymentGuardState
        tone='loading'
        eyebrow={t('模型与路由')}
        title={t('模型部署')}
        subtitle={t('正在加载部署服务配置。')}
        badge={
          <Tag color='blue' shape='circle' type='light'>
            {t('初始化')}
          </Tag>
        }
        icon={
          <span className='ct-deployment-guard-spinner'>
            <Spin size='middle' />
          </span>
        }
      >
        <strong>{t('加载设置中...')}</strong>
        <p>{t('正在读取 io.net 部署开关和连接配置。')}</p>
      </DeploymentGuardState>
    );
  }

  if (!isEnabled) {
    return (
      <DeploymentGuardState
        tone='warning'
        eyebrow={t('模型与路由')}
        title={t('模型部署服务未启用')}
        subtitle={t('访问模型部署功能需要先启用 io.net 部署服务。')}
        badge={
          <Tag color='orange' shape='circle' type='light'>
            {t('需要配置')}
          </Tag>
        }
        icon={<AlertCircle size={34} />}
        actions={
          <Button
            type='primary'
            theme='solid'
            icon={<Settings size={16} />}
            onClick={handleGoToSettings}
          >
            {t('前往设置页面')}
          </Button>
        }
      >
        <strong>{t('需要配置的项目')}</strong>
        <ul>
          <li>
            <Server size={14} />
            {t('启用 io.net 部署开关')}
          </li>
          <li>
            <Settings size={14} />
            {t('配置有效的 io.net API Key')}
          </li>
        </ul>
        <p>{t('配置完成后刷新页面即可使用模型部署功能')}</p>
      </DeploymentGuardState>
    );
  }

  if (connectionLoading || (connectionOk === null && !connectionError)) {
    return (
      <DeploymentGuardState
        tone='loading'
        eyebrow={t('模型与路由')}
        title={t('正在检查 io.net 连接...')}
        subtitle={t('正在验证部署服务连接状态，请稍候。')}
        badge={
          <Tag color='cyan' shape='circle' type='light'>
            {t('连接检测')}
          </Tag>
        }
        icon={<Loader2 className='is-spinning' size={34} />}
      >
        <strong>{t('连接检测中')}</strong>
        <p>{t('检查完成后会自动进入模型部署工作台。')}</p>
      </DeploymentGuardState>
    );
  }

  if (connectionOk === false) {
    const isExpired = connectionError?.type === 'expired';
    const title = isExpired ? t('接口密钥已过期') : t('无法连接 io.net');
    const description = isExpired
      ? t('当前 API 密钥已过期，请在设置中更新。')
      : t('当前配置无法连接到 io.net。');
    const detail = connectionError?.message || '';

    return (
      <DeploymentGuardState
        tone='danger'
        eyebrow={t('模型与路由')}
        title={title}
        subtitle={description}
        badge={
          <Tag color='red' shape='circle' type='light'>
            {t('连接异常')}
          </Tag>
        }
        icon={<WifiOff size={34} />}
        actions={
          <>
            <Button
              type='primary'
              theme='solid'
              icon={<Settings size={16} />}
              onClick={handleGoToSettings}
            >
              {t('前往设置')}
            </Button>
            {onRetry ? (
              <Button
                type='tertiary'
                icon={<RefreshCw size={16} />}
                onClick={onRetry}
              >
                {t('重试连接')}
              </Button>
            ) : null}
          </>
        }
      >
        <strong>{t('部署服务暂不可用')}</strong>
        {detail ? <p>{detail}</p> : <p>{t('请检查 API Key 和网络连接。')}</p>}
      </DeploymentGuardState>
    );
  }

  return children;
};

export default DeploymentAccessGuard;
