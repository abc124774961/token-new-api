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

import React, { useEffect, useMemo, useState } from 'react';
import { Button, Modal } from '@douyinfe/semi-ui';
import { ArrowRight, ExternalLink, ServerCog } from 'lucide-react';
import { useTranslation } from 'react-i18next';

const OLD_DOMAIN = 'https://api.codetoken.top';
const NEW_DOMAIN = 'https://api.token-bits.com';
const LEGACY_HOSTNAME = 'api.codetoken.top';
const STORAGE_KEY = 'domain_migration_notice_api_codetoken_top_v1';

const isLegacyDomain = () => {
  if (typeof window === 'undefined') {
    return false;
  }
  return window.location.hostname === LEGACY_HOSTNAME;
};

const getDismissed = () => {
  try {
    return window.localStorage.getItem(STORAGE_KEY) === 'dismissed';
  } catch (error) {
    return false;
  }
};

const setDismissed = () => {
  try {
    window.localStorage.setItem(STORAGE_KEY, 'dismissed');
  } catch (error) {
    // localStorage may be unavailable in strict privacy contexts.
  }
};

const getNewDomainTarget = () => {
  if (typeof window === 'undefined') {
    return `${NEW_DOMAIN}/`;
  }
  const { pathname, search, hash } = window.location;
  return `${NEW_DOMAIN}${pathname || '/'}${search || ''}${hash || ''}`;
};

const DomainMigrationNotice = () => {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(false);

  const shouldShow = useMemo(() => isLegacyDomain(), []);

  useEffect(() => {
    if (shouldShow && !getDismissed()) {
      setVisible(true);
    }
  }, [shouldShow]);

  const handleClose = () => {
    setDismissed();
    setVisible(false);
  };

  const handleGoNewDomain = () => {
    setDismissed();
    window.location.assign(getNewDomainTarget());
  };

  if (!shouldShow) {
    return null;
  }

  return (
    <Modal
      className='ct-domain-migration-modal'
      visible={visible}
      onCancel={handleClose}
      width={560}
      centered
      maskClosable={false}
      title={
        <div className='ct-domain-migration-title'>
          <span className='ct-domain-migration-icon'>
            <ServerCog size={18} />
          </span>
          <span>{t('服务域名迁移通知')}</span>
        </div>
      }
      footer={
        <div className='ct-domain-migration-footer'>
          <Button theme='borderless' onClick={handleClose}>
            {t('我知道了')}
          </Button>
          <Button
            theme='solid'
            type='primary'
            icon={<ExternalLink size={15} />}
            onClick={handleGoNewDomain}
          >
            {t('前往新域名')}
          </Button>
        </div>
      }
    >
      <div className='ct-domain-migration-body'>
        <p>
          {t(
            '当前访问的是旧域名 {{oldDomain}}。服务已迁移至 {{newDomain}}，后续请以新域名为准。',
            {
              oldDomain: OLD_DOMAIN,
              newDomain: NEW_DOMAIN,
            },
          )}
        </p>
        <p>
          {t(
            '请尽快更新浏览器书签、客户端 Base URL 和相关集成配置，避免后续访问或调用受影响。',
          )}
        </p>
        <div className='ct-domain-migration-route' aria-label={t('域名迁移')}>
          <div className='ct-domain-migration-domain ct-domain-migration-domain--old'>
            <span>{t('当前域名')}</span>
            <strong>{OLD_DOMAIN}</strong>
          </div>
          <span className='ct-domain-migration-arrow'>
            <ArrowRight size={18} />
          </span>
          <div className='ct-domain-migration-domain ct-domain-migration-domain--new'>
            <span>{t('新域名')}</span>
            <strong>{NEW_DOMAIN}</strong>
          </div>
        </div>
        <div className='ct-domain-migration-tip'>
          {t('如果你的客户端配置了完整 API 地址，请将域名部分替换为新域名。')}
        </div>
      </div>
    </Modal>
  );
};

export default DomainMigrationNotice;
