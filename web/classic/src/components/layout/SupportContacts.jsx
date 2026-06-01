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

import React, { useContext, useMemo, useState } from 'react';
import { Button, Modal, Tag, Tooltip, Typography } from '@douyinfe/semi-ui';
import {
  BookOpen,
  Copy,
  ExternalLink,
  Hash,
  Headphones,
  LifeBuoy,
  Link as LinkIcon,
  Mail,
  MessageCircle,
  QrCode,
  Send,
  Users,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'react-router-dom';
import { StatusContext } from '../../context/Status';
import { copy, showError, showSuccess } from '../../helpers';

const { Text } = Typography;

const contactTypeIcons = {
  telegram: Send,
  email: Mail,
  wechat: MessageCircle,
  qq: Hash,
  discord: Users,
  docs: BookOpen,
  custom: LinkIcon,
};

const contactTypeColors = {
  telegram: 'blue',
  email: 'green',
  wechat: 'teal',
  qq: 'cyan',
  discord: 'purple',
  docs: 'light-blue',
  custom: 'grey',
};

const contactTypeLabels = (t) => ({
  telegram: t('Telegram'),
  email: t('邮箱'),
  wechat: t('微信'),
  qq: t('QQ'),
  discord: t('Discord'),
  docs: t('文档'),
  custom: t('自定义'),
});

export const getVisibleSupportContacts = (status) => {
  if (!status?.support_contacts_enabled) {
    return [];
  }
  if (!Array.isArray(status.support_contacts)) {
    return [];
  }
  return status.support_contacts
    .filter((contact) => {
      if (!contact || contact.enabled === false) {
        return false;
      }
      return contact.title && (contact.value || contact.url || contact.qrcode);
    })
    .sort((a, b) => {
      const aPriority = Number.isFinite(Number(a.priority))
        ? Number(a.priority)
        : 0;
      const bPriority = Number.isFinite(Number(b.priority))
        ? Number(b.priority)
        : 0;
      if (aPriority !== bPriority) {
        return aPriority - bPriority;
      }
      return Number(a.id || 0) - Number(b.id || 0);
    });
};

const useSupportContacts = () => {
  const [statusState] = useContext(StatusContext);
  return useMemo(
    () => getVisibleSupportContacts(statusState?.status),
    [statusState?.status],
  );
};

const getContactKey = (contact, index) =>
  `${contact.type || 'custom'}-${contact.id || contact.title || index}`;

const getContactValue = (contact) =>
  contact.value || contact.url || contact.qrcode || '';

const getContactDisplayValue = (contact) => contact.value || contact.url || '';

const openContactURL = (url) => {
  if (!url || typeof window === 'undefined') {
    return;
  }
  window.open(url, '_blank', 'noopener,noreferrer');
};

function ContactIcon({ type, size = 18 }) {
  const Icon = contactTypeIcons[type] || Headphones;
  return <Icon size={size} />;
}

function SupportContactsModal({ visible, contacts, onClose }) {
  const { t } = useTranslation();
  const labels = contactTypeLabels(t);

  const handleCopy = async (contact) => {
    const value = getContactValue(contact);
    if (!value) {
      return;
    }
    if (await copy(value)) {
      showSuccess(t('已复制到剪贴板'));
    } else {
      showError(t('无法复制到剪贴板，请手动复制'));
    }
  };

  return (
    <Modal
      className='ct-support-contacts-modal'
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={760}
      centered
      title={
        <div className='ct-support-modal-title'>
          <span className='ct-support-modal-title-icon'>
            <LifeBuoy size={18} />
          </span>
          <span>{t('联系客服')}</span>
        </div>
      }
    >
      <div className='ct-support-modal-body'>
        <p className='ct-support-modal-lead'>
          {t('请选择合适的渠道联系我们，处理结果会以对应渠道回复。')}
        </p>
        <div className='ct-support-contact-grid'>
          {contacts.map((contact, index) => (
            <div
              className='ct-support-contact-card'
              key={getContactKey(contact, index)}
            >
              <div className='ct-support-contact-card-header'>
                <span className={`ct-support-contact-icon is-${contact.type}`}>
                  <ContactIcon type={contact.type} />
                </span>
                <div className='ct-support-contact-title-wrap'>
                  <Text strong ellipsis={{ showTooltip: true }}>
                    {contact.title}
                  </Text>
                  <Tag
                    color={contactTypeColors[contact.type] || 'grey'}
                    shape='circle'
                    size='small'
                  >
                    {labels[contact.type] || labels.custom}
                  </Tag>
                </div>
              </div>

              {contact.description && (
                <Text
                  className='ct-support-contact-description'
                  type='tertiary'
                >
                  {contact.description}
                </Text>
              )}

              {getContactDisplayValue(contact) && (
                <div className='ct-support-contact-value'>
                  <span>{getContactDisplayValue(contact)}</span>
                </div>
              )}

              {contact.qrcode && (
                <div className='ct-support-contact-qr'>
                  <div className='ct-support-contact-qr-label'>
                    <QrCode size={14} />
                    <span>{t('二维码')}</span>
                  </div>
                  <img src={contact.qrcode} alt={contact.title} />
                </div>
              )}

              <div className='ct-support-contact-actions'>
                {getContactValue(contact) && (
                  <Button
                    icon={<Copy size={14} />}
                    size='small'
                    theme='light'
                    type='tertiary'
                    onClick={() => handleCopy(contact)}
                  >
                    {t('复制')}
                  </Button>
                )}
                {contact.url && (
                  <Button
                    icon={<ExternalLink size={14} />}
                    size='small'
                    theme='solid'
                    type='primary'
                    onClick={() => openContactURL(contact.url)}
                  >
                    {t('打开链接')}
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </Modal>
  );
}

export function SupportContactsFloatingButton() {
  const { t } = useTranslation();
  const location = useLocation();
  const contacts = useSupportContacts();
  const [visible, setVisible] = useState(false);
  const isConsoleRoute = location.pathname.startsWith('/console');
  const floatingClassName = `ct-support-floating-button ${
    isConsoleRoute ? 'is-console' : 'is-edge-tab'
  }`;

  if (contacts.length === 0) {
    return null;
  }

  return (
    <>
      <Tooltip content={t('联系客服')} position='left'>
        <Button
          className={floatingClassName}
          icon={<LifeBuoy size={18} />}
          onClick={() => setVisible(true)}
          theme='solid'
          type='primary'
        >
          {t('联系客服')}
        </Button>
      </Tooltip>
      <SupportContactsModal
        visible={visible}
        contacts={contacts}
        onClose={() => setVisible(false)}
      />
    </>
  );
}

export function SupportContactsFooterSummary() {
  const { t } = useTranslation();
  const contacts = useSupportContacts();
  const [visible, setVisible] = useState(false);

  if (contacts.length === 0) {
    return null;
  }

  const labels = contactTypeLabels(t);
  const footerContacts = contacts.slice(0, 2);

  return (
    <div className='ct-support-footer-summary'>
      <span className='ct-support-footer-label'>
        <LifeBuoy size={14} />
        {t('客服支持')}
      </span>
      <div className='ct-support-footer-actions'>
        {footerContacts.map((contact, index) => (
          <Button
            key={getContactKey(contact, index)}
            className='ct-support-footer-chip'
            icon={<ContactIcon type={contact.type} size={13} />}
            size='small'
            theme='light'
            type='tertiary'
            onClick={() =>
              contact.url ? openContactURL(contact.url) : setVisible(true)
            }
          >
            {contact.title || labels[contact.type] || labels.custom}
          </Button>
        ))}
        <Button
          className='ct-support-footer-chip'
          icon={<Headphones size={13} />}
          size='small'
          theme='light'
          type='primary'
          onClick={() => setVisible(true)}
        >
          {contacts.length > 2 ? t('更多联系方式') : t('查看全部')}
        </Button>
      </div>
      <SupportContactsModal
        visible={visible}
        contacts={contacts}
        onClose={() => setVisible(false)}
      />
    </div>
  );
}
