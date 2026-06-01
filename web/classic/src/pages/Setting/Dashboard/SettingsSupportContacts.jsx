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

import React, { useContext, useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  Divider,
  Empty,
  Form,
  Modal,
  Space,
  Switch,
  Table,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import {
  ArrowDown,
  ArrowUp,
  BookOpen,
  Copy,
  Edit,
  ExternalLink,
  Eye,
  Hash,
  Headphones,
  LifeBuoy,
  Link as LinkIcon,
  Mail,
  MessageCircle,
  Plus,
  QrCode,
  Save,
  Send,
  Trash2,
  Users,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import {
  API,
  copy,
  setStatusData,
  showError,
  showSuccess,
} from '../../../helpers';
import { StatusContext } from '../../../context/Status';

const { Text } = Typography;

const defaultContactForm = {
  type: 'custom',
  title: '',
  description: '',
  value: '',
  url: '',
  qrcode: '',
  priority: 10,
  enabled: true,
};

const contactTypeIconMap = {
  telegram: Send,
  email: Mail,
  wechat: MessageCircle,
  qq: Hash,
  discord: Users,
  docs: BookOpen,
  custom: LinkIcon,
};

const contactTypeColorMap = {
  telegram: 'blue',
  email: 'green',
  wechat: 'teal',
  qq: 'cyan',
  discord: 'purple',
  docs: 'light-blue',
  custom: 'grey',
};

const contactValue = (contact) =>
  contact?.value || contact?.url || contact?.qrcode || '';

const sortContacts = (contacts) =>
  [...contacts].sort((a, b) => {
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

const applyOrderPriorities = (contacts) =>
  contacts.map((contact, index) => ({
    ...contact,
    priority: (index + 1) * 10,
  }));

const normalizeContacts = (contacts) =>
  sortContacts(
    contacts.map((contact, index) => ({
      id: contact.id || index + 1,
      type: contact.type || 'custom',
      title: contact.title || '',
      description: contact.description || '',
      value: contact.value || '',
      url: contact.url || '',
      qrcode: contact.qrcode || '',
      priority:
        contact.priority === undefined || contact.priority === null
          ? (index + 1) * 10
          : Number(contact.priority),
      enabled: contact.enabled !== false,
    })),
  );

const ContactTypeIcon = ({ type, size = 15 }) => {
  const Icon = contactTypeIconMap[type] || Headphones;
  return <Icon size={size} />;
};

const SettingsSupportContacts = ({ options, refresh }) => {
  const { t } = useTranslation();
  const [, statusDispatch] = useContext(StatusContext);
  const contactFormApiRef = useRef(null);
  const [contacts, setContacts] = useState([]);
  const [panelEnabled, setPanelEnabled] = useState(true);
  const [hasChanges, setHasChanges] = useState(false);
  const [loading, setLoading] = useState(false);
  const [modalLoading, setModalLoading] = useState(false);
  const [showContactModal, setShowContactModal] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const [showPreviewModal, setShowPreviewModal] = useState(false);
  const [editingContact, setEditingContact] = useState(null);
  const [deletingContact, setDeletingContact] = useState(null);
  const [previewContact, setPreviewContact] = useState(null);
  const [contactForm, setContactForm] = useState(defaultContactForm);

  const typeOptions = useMemo(
    () => [
      { value: 'telegram', label: 'Telegram' },
      { value: 'email', label: t('邮箱') },
      { value: 'wechat', label: t('微信') },
      { value: 'qq', label: 'QQ' },
      { value: 'discord', label: 'Discord' },
      { value: 'docs', label: t('文档') },
      { value: 'custom', label: t('自定义') },
    ],
    [t],
  );

  const typeLabelMap = useMemo(
    () =>
      Object.fromEntries(
        typeOptions.map((option) => [option.value, option.label]),
      ),
    [typeOptions],
  );

  const orderedContacts = useMemo(() => sortContacts(contacts), [contacts]);

  const refreshPublicStatus = async () => {
    const res = await API.get('/api/status');
    const { success, data, message } = res.data;
    if (!success) {
      throw new Error(message || t('刷新失败'));
    }
    statusDispatch({ type: 'set', payload: data });
    setStatusData(data);
  };

  const updateOption = async (
    key,
    value,
    successText,
    { refreshOptions = true, refreshStatus = false } = {},
  ) => {
    const res = await API.put('/api/option/', {
      key,
      value,
    });
    const { success, message } = res.data;
    if (!success) {
      throw new Error(message || t('设置保存失败'));
    }
    if (successText) {
      showSuccess(successText);
    }
    if (refreshOptions) {
      await refresh?.();
    }
    if (refreshStatus) {
      await refreshPublicStatus();
    }
    return true;
  };

  const persistContacts = async (nextContacts, successText) => {
    const normalizedContacts = sortContacts(nextContacts);
    await updateOption(
      'console_setting.support_contacts',
      JSON.stringify(normalizedContacts),
      successText,
      { refreshStatus: true },
    );
    setContacts(normalizedContacts);
    setHasChanges(false);
  };

  const submitContacts = async () => {
    try {
      setLoading(true);
      await persistContacts(orderedContacts, t('客服联系方式已更新'));
    } catch (error) {
      console.error('客服联系方式更新失败', error);
      showError(error.message || t('客服联系方式更新失败'));
    } finally {
      setLoading(false);
    }
  };

  const handleToggleEnabled = async (checked) => {
    try {
      await updateOption(
        'console_setting.support_contacts_enabled',
        checked ? 'true' : 'false',
        t('设置已保存'),
        { refreshStatus: true },
      );
      setPanelEnabled(checked);
    } catch (err) {
      showError(err.message || t('设置保存失败'));
    }
  };

  const handleAddContact = () => {
    setEditingContact(null);
    setContactForm({
      ...defaultContactForm,
      priority: (contacts.length + 1) * 10,
    });
    setShowContactModal(true);
  };

  const handleEditContact = (contact) => {
    setEditingContact(contact);
    setContactForm({
      ...defaultContactForm,
      ...contact,
      enabled: contact.enabled !== false,
    });
    setShowContactModal(true);
  };

  const handleDeleteContact = (contact) => {
    setDeletingContact(contact);
    setShowDeleteModal(true);
  };

  const handlePreviewContact = (contact) => {
    setPreviewContact(contact);
    setShowPreviewModal(true);
  };

  const handleCopyContact = async (contact) => {
    const value = contactValue(contact);
    if (!value) {
      return;
    }
    if (await copy(value)) {
      showSuccess(t('已复制到剪贴板'));
    } else {
      showError(t('无法复制到剪贴板，请手动复制'));
    }
  };

  const handleOpenContact = (contact) => {
    if (contact.url) {
      window.open(contact.url, '_blank', 'noopener,noreferrer');
    }
  };

  const handleMoveContact = async (contact, direction) => {
    const current = sortContacts(contacts);
    const index = current.findIndex((item) => item.id === contact.id);
    const targetIndex = index + direction;
    if (index < 0 || targetIndex < 0 || targetIndex >= current.length) {
      return;
    }
    const next = [...current];
    [next[index], next[targetIndex]] = [next[targetIndex], next[index]];
    try {
      setLoading(true);
      await persistContacts(
        applyOrderPriorities(next),
        t('客服联系方式已更新'),
      );
    } catch (error) {
      console.error('客服联系方式排序保存失败', error);
      showError(error.message || t('客服联系方式更新失败'));
    } finally {
      setLoading(false);
    }
  };

  const confirmDeleteContact = async () => {
    if (deletingContact) {
      try {
        setLoading(true);
        await persistContacts(
          contacts.filter((contact) => contact.id !== deletingContact.id),
          t('客服联系方式已更新'),
        );
      } catch (error) {
        console.error('客服联系方式删除保存失败', error);
        showError(error.message || t('客服联系方式更新失败'));
        return;
      } finally {
        setLoading(false);
      }
    }
    setDeletingContact(null);
    setShowDeleteModal(false);
  };

  const handleSaveContact = async () => {
    const formValues = {
      ...contactForm,
      ...(contactFormApiRef.current?.getValues?.() || {}),
    };
    const priorityValue = Number(formValues.priority);
    const normalizedForm = {
      ...formValues,
      type: formValues.type || 'custom',
      title: (formValues.title || '').trim(),
      description: (formValues.description || '').trim(),
      value: (formValues.value || '').trim(),
      url: (formValues.url || '').trim(),
      qrcode: (formValues.qrcode || '').trim(),
      priority: Number.isFinite(priorityValue)
        ? priorityValue
        : (contacts.length + 1) * 10,
      enabled: formValues.enabled !== false,
    };

    if (!normalizedForm.title || !normalizedForm.type) {
      showError(t('请填写联系方式标题和类型'));
      return;
    }
    if (!contactValue(normalizedForm)) {
      showError(t('请至少填写值、链接或二维码之一'));
      return;
    }

    try {
      setModalLoading(true);
      let nextContacts;
      if (editingContact) {
        nextContacts = contacts.map((contact) =>
          contact.id === editingContact.id
            ? { ...contact, ...normalizedForm }
            : contact,
        );
      } else {
        const newId = Math.max(...contacts.map((contact) => contact.id), 0) + 1;
        nextContacts = [...contacts, { id: newId, ...normalizedForm }];
      }
      await persistContacts(nextContacts, t('客服联系方式已更新'));
      setShowContactModal(false);
    } catch (error) {
      console.error('客服联系方式保存失败', error);
      showError(error.message || t('客服联系方式更新失败'));
    } finally {
      setModalLoading(false);
    }
  };

  const parseContacts = (contactsStr) => {
    if (!contactsStr) {
      setContacts([]);
      return;
    }
    try {
      const parsed = JSON.parse(contactsStr);
      setContacts(Array.isArray(parsed) ? normalizeContacts(parsed) : []);
    } catch (error) {
      console.error('解析客服联系方式失败:', error);
      setContacts([]);
    }
  };

  useEffect(() => {
    if (options['console_setting.support_contacts'] !== undefined) {
      parseContacts(options['console_setting.support_contacts']);
    }
  }, [options['console_setting.support_contacts']]);

  useEffect(() => {
    const enabledStr = options['console_setting.support_contacts_enabled'];
    setPanelEnabled(
      enabledStr === undefined
        ? true
        : enabledStr === 'true' || enabledStr === true,
    );
  }, [options['console_setting.support_contacts_enabled']]);

  useEffect(() => {
    if (showContactModal) {
      window.setTimeout(() => {
        contactFormApiRef.current?.setValues?.(contactForm);
      }, 0);
    }
  }, [showContactModal, editingContact?.id]);

  const columns = [
    {
      title: t('渠道'),
      dataIndex: 'type',
      width: 160,
      render: (type) => (
        <Tag
          color={contactTypeColorMap[type] || 'grey'}
          prefixIcon={<ContactTypeIcon type={type} />}
          shape='circle'
        >
          {typeLabelMap[type] || typeLabelMap.custom}
        </Tag>
      ),
    },
    {
      title: t('标题'),
      dataIndex: 'title',
      render: (title) => (
        <Text strong ellipsis={{ showTooltip: true }}>
          {title}
        </Text>
      ),
    },
    {
      title: t('联系方式'),
      dataIndex: 'value',
      render: (_, record) => (
        <Tooltip content={contactValue(record) || '-'}>
          <Text
            code
            ellipsis={{ showTooltip: false }}
            style={{ maxWidth: 260 }}
          >
            {contactValue(record) || '-'}
          </Text>
        </Tooltip>
      ),
    },
    {
      title: t('排序'),
      dataIndex: 'priority',
      width: 140,
      render: (_, record, index) => (
        <Space>
          <Button
            icon={<ArrowUp size={14} />}
            size='small'
            theme='light'
            type='tertiary'
            disabled={index === 0}
            onClick={() => handleMoveContact(record, -1)}
          />
          <Button
            icon={<ArrowDown size={14} />}
            size='small'
            theme='light'
            type='tertiary'
            disabled={index === orderedContacts.length - 1}
            onClick={() => handleMoveContact(record, 1)}
          />
        </Space>
      ),
    },
    {
      title: t('状态'),
      dataIndex: 'enabled',
      width: 110,
      render: (enabled) => (
        <Tag color={enabled === false ? 'grey' : 'green'} shape='circle'>
          {enabled === false ? t('已禁用') : t('已启用')}
        </Tag>
      ),
    },
    {
      title: t('操作'),
      fixed: 'right',
      width: 260,
      render: (_, record) => (
        <Space>
          <Button
            icon={<Eye size={14} />}
            theme='light'
            type='tertiary'
            size='small'
            onClick={() => handlePreviewContact(record)}
          >
            {t('预览')}
          </Button>
          <Button
            icon={<Copy size={14} />}
            theme='light'
            type='tertiary'
            size='small'
            onClick={() => handleCopyContact(record)}
          >
            {t('复制')}
          </Button>
          {record.url && (
            <Button
              icon={<ExternalLink size={14} />}
              theme='light'
              type='primary'
              size='small'
              onClick={() => handleOpenContact(record)}
            />
          )}
          <Button
            icon={<Edit size={14} />}
            theme='light'
            type='tertiary'
            size='small'
            onClick={() => handleEditContact(record)}
          />
          <Button
            icon={<Trash2 size={14} />}
            type='danger'
            theme='light'
            size='small'
            onClick={() => handleDeleteContact(record)}
          />
        </Space>
      ),
    },
  ];

  const renderHeader = () => (
    <div className='flex flex-col w-full'>
      <div className='mb-2'>
        <div className='flex items-center text-teal-600'>
          <LifeBuoy size={16} className='mr-2' />
          <Text>
            {t(
              '客服联系方式管理，可以配置多个公开联系方式用于全站展示（最多20个）',
            )}
          </Text>
        </div>
      </div>

      <Divider margin='12px' />

      <div className='flex flex-col md:flex-row justify-between items-center gap-4 w-full'>
        <div className='flex flex-wrap gap-2 w-full md:w-auto order-2 md:order-1'>
          <Button
            theme='light'
            type='primary'
            icon={<Plus size={14} />}
            className='w-full md:w-auto'
            onClick={handleAddContact}
          >
            {t('添加联系方式')}
          </Button>
          <Button
            icon={<Save size={14} />}
            onClick={submitContacts}
            loading={loading}
            disabled={!hasChanges}
            type='secondary'
            className='w-full md:w-auto'
          >
            {t('保存设置')}
          </Button>
        </div>

        <div className='order-1 md:order-2 flex items-center gap-2'>
          <Switch checked={panelEnabled} onChange={handleToggleEnabled} />
          <Text>{panelEnabled ? t('已启用') : t('已禁用')}</Text>
        </div>
      </div>
    </div>
  );

  return (
    <>
      <Form.Section text={renderHeader()}>
        <Table
          columns={columns}
          dataSource={orderedContacts}
          rowKey='id'
          scroll={{ x: 'max-content' }}
          pagination={false}
          size='middle'
          loading={loading}
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              description={t('暂无客服联系方式')}
              style={{ padding: 30 }}
            />
          }
          className='overflow-hidden'
        />
      </Form.Section>

      <Modal
        title={editingContact ? t('编辑联系方式') : t('添加联系方式')}
        visible={showContactModal}
        onOk={handleSaveContact}
        onCancel={() => setShowContactModal(false)}
        okText={t('保存')}
        cancelText={t('取消')}
        confirmLoading={modalLoading}
      >
        <Form
          layout='vertical'
          initValues={contactForm}
          key={editingContact ? editingContact.id : 'new'}
          getFormApi={(api) => (contactFormApiRef.current = api)}
        >
          <Form.Select
            field='type'
            label={t('渠道类型')}
            optionList={typeOptions}
            rules={[{ required: true, message: t('请选择渠道类型') }]}
            onChange={(value) =>
              setContactForm({ ...contactForm, type: value })
            }
          />
          <Form.Input
            field='title'
            label={t('标题')}
            placeholder={t('如：工作日在线客服')}
            rules={[{ required: true, message: t('请输入标题') }]}
            onChange={(value) =>
              setContactForm({ ...contactForm, title: value })
            }
          />
          <Form.TextArea
            field='description'
            label={t('说明')}
            placeholder={t('如：工作日 10:00-19:00 响应')}
            autosize
            onChange={(value) =>
              setContactForm({ ...contactForm, description: value })
            }
          />
          <Form.Input
            field='value'
            label={t('展示值')}
            placeholder={t('如：support@example.com 或 @username')}
            onChange={(value) =>
              setContactForm({ ...contactForm, value: value })
            }
          />
          <Form.Input
            field='url'
            label={t('跳转链接')}
            placeholder='https://example.com/support'
            onChange={(value) => setContactForm({ ...contactForm, url: value })}
          />
          <Form.Input
            field='qrcode'
            label={t('二维码图片地址')}
            placeholder='https://example.com/qrcode.png'
            onChange={(value) =>
              setContactForm({ ...contactForm, qrcode: value })
            }
          />
          <Form.InputNumber
            field='priority'
            label={t('排序权重')}
            min={0}
            step={10}
            onChange={(value) =>
              setContactForm({ ...contactForm, priority: Number(value) || 0 })
            }
          />
          <div className='flex items-center gap-2 mt-2'>
            <Switch
              checked={contactForm.enabled}
              onChange={(checked) =>
                setContactForm({ ...contactForm, enabled: checked })
              }
            />
            <Text>{contactForm.enabled ? t('已启用') : t('已禁用')}</Text>
          </div>
        </Form>
      </Modal>

      <Modal
        title={t('预览联系方式')}
        visible={showPreviewModal}
        onCancel={() => setShowPreviewModal(false)}
        footer={null}
        width={520}
      >
        {previewContact && (
          <div className='flex flex-col gap-4'>
            <div className='flex items-center gap-3'>
              <Tag
                color={contactTypeColorMap[previewContact.type] || 'grey'}
                prefixIcon={<ContactTypeIcon type={previewContact.type} />}
                shape='circle'
              >
                {typeLabelMap[previewContact.type] || typeLabelMap.custom}
              </Tag>
              <Text strong>{previewContact.title}</Text>
            </div>
            {previewContact.description && (
              <Text type='tertiary'>{previewContact.description}</Text>
            )}
            {contactValue(previewContact) && (
              <Text code copyable>
                {contactValue(previewContact)}
              </Text>
            )}
            {previewContact.qrcode && (
              <div className='flex flex-col gap-2'>
                <Text strong>
                  <QrCode size={14} className='inline mr-1' />
                  {t('二维码')}
                </Text>
                <img
                  src={previewContact.qrcode}
                  alt={previewContact.title}
                  style={{
                    width: 140,
                    height: 140,
                    objectFit: 'cover',
                    borderRadius: 12,
                  }}
                />
              </div>
            )}
          </div>
        )}
      </Modal>

      <Modal
        title={t('确认删除')}
        visible={showDeleteModal}
        onOk={confirmDeleteContact}
        onCancel={() => {
          setShowDeleteModal(false);
          setDeletingContact(null);
        }}
        okText={t('确认删除')}
        cancelText={t('取消')}
        type='warning'
        okButtonProps={{
          type: 'danger',
          theme: 'solid',
        }}
      >
        <Text>{t('确定要删除此联系方式吗？')}</Text>
      </Modal>
    </>
  );
};

export default SettingsSupportContacts;
