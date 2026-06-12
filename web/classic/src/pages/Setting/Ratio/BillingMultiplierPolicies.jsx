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
import {
  Banner,
  Button,
  Form,
  Modal,
  Popconfirm,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconDelete,
  IconEdit,
  IconPlus,
  IconRefresh,
} from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import { ADMIN_PERMISSION_KEYS } from '../../../apps/admin-console/permissions/adminPermissions.config';
import {
  AdminPermissionButton,
  useAdminActionPermission,
} from '../../../apps/admin-console/permissions/AdminPermissionAction';

const { Text } = Typography;

const DEFAULT_POLICY = {
  name: '',
  enabled: true,
  priority: 0,
  scope_type: 'global',
  scope_value: '',
  using_groups: '',
  models: '',
  mode: 'multiply',
  multiplier: 1,
  start_at: 0,
  end_at: 0,
  description: '',
};

const listToJsonText = (value) => {
  if (!value) return '';
  if (Array.isArray(value)) return JSON.stringify(value);
  const text = String(value).trim();
  if (!text) return '';
  try {
    const parsed = JSON.parse(text);
    if (Array.isArray(parsed)) return JSON.stringify(parsed);
  } catch {
    return JSON.stringify(
      text
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean),
    );
  }
  return text;
};

const renderListText = (value) => {
  if (!value) return '-';
  try {
    const parsed = JSON.parse(value);
    if (Array.isArray(parsed) && parsed.length > 0) return parsed.join(', ');
  } catch {
    return value;
  }
  return '-';
};

export default function BillingMultiplierPolicies({
  ratioDangerPermissionDenied,
  ensureRatioPermission,
}) {
  const { t } = useTranslation();
  const canManage = useAdminActionPermission(
    ADMIN_PERMISSION_KEYS.modelRatioUpdate,
  );
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [policies, setPolicies] = useState([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [formApi, setFormApi] = useState(null);
  const [preview, setPreview] = useState(null);
  const [previewLoading, setPreviewLoading] = useState(false);

  const scopeOptions = useMemo(
    () => [
      { label: t('全局'), value: 'global' },
      { label: t('指定用户'), value: 'user' },
      { label: t('用户分组'), value: 'user_group' },
      { label: t('订阅套餐'), value: 'subscription_plan' },
      { label: t('使用分组'), value: 'using_group' },
    ],
    [t],
  );

  const modeOptions = useMemo(
    () => [
      { label: t('叠加'), value: 'multiply' },
      { label: t('覆盖'), value: 'override' },
      { label: t('倍率下限'), value: 'min' },
      { label: t('倍率上限'), value: 'max' },
    ],
    [t],
  );

  const fetchPolicies = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/billing-multiplier-policies/');
      if (res.data.success) {
        setPolicies(res.data.data || []);
      } else {
        showError(res.data.message || t('加载失败'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchPolicies();
  }, []);

  const openCreate = () => {
    setEditing(null);
    setPreview(null);
    setModalVisible(true);
    setTimeout(() => formApi?.setValues(DEFAULT_POLICY), 0);
  };

  const openEdit = (record) => {
    setEditing(record);
    setPreview(null);
    setModalVisible(true);
    setTimeout(() => formApi?.setValues({ ...DEFAULT_POLICY, ...record }), 0);
  };

  const normalizePolicy = (values) => ({
    ...DEFAULT_POLICY,
    ...values,
    id: editing?.id || values.id || 0,
    enabled: Boolean(values.enabled),
    priority: Number(values.priority) || 0,
    multiplier: Number(values.multiplier) || 0,
    start_at: Number(values.start_at) || 0,
    end_at: Number(values.end_at) || 0,
    using_groups: listToJsonText(values.using_groups),
    models: listToJsonText(values.models),
  });

  const savePolicy = async () => {
    if (ensureRatioPermission && !ensureRatioPermission()) return;
    if (!formApi) return;
    let values;
    try {
      values = await formApi.validate();
    } catch {
      return;
    }
    const policy = normalizePolicy(values);
    setSaving(true);
    try {
      const url = editing
        ? `/api/billing-multiplier-policies/${editing.id}`
        : '/api/billing-multiplier-policies/';
      const method = editing ? API.put : API.post;
      const res = await method(url, { policy });
      if (res.data.success) {
        showSuccess(t('保存成功'));
        setModalVisible(false);
        fetchPolicies();
      } else {
        showError(res.data.message || t('保存失败'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };

  const deletePolicy = async (record) => {
    if (ensureRatioPermission && !ensureRatioPermission()) return;
    try {
      const res = await API.delete(
        `/api/billing-multiplier-policies/${record.id}`,
      );
      if (res.data.success) {
        showSuccess(t('删除成功'));
        fetchPolicies();
      } else {
        showError(res.data.message || t('删除失败'));
      }
    } catch (error) {
      showError(error.message);
    }
  };

  const runPreview = async () => {
    if (!formApi) return;
    let values;
    try {
      values = await formApi.validate();
    } catch {
      return;
    }
    const policy = normalizePolicy(values);
    setPreviewLoading(true);
    try {
      const res = await API.post('/api/billing-multiplier-policies/preview', {
        user_id: Number(values.preview_user_id) || 0,
        user_group: values.preview_user_group || '',
        using_group: values.preview_using_group || '',
        model_name: values.preview_model_name || '',
        subscription_plan_id: Number(values.preview_subscription_plan_id) || 0,
        base_group_ratio: Number(values.preview_base_group_ratio) || 1,
        policy,
      });
      if (res.data.success) {
        setPreview(res.data.data);
      } else {
        showError(res.data.message || t('预览失败'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setPreviewLoading(false);
    }
  };

  const columns = [
    {
      title: t('名称'),
      dataIndex: 'name',
      render: (text, record) => (
        <Space spacing={6}>
          <Text strong>{text}</Text>
          {record.enabled ? (
            <Tag color='green' size='small'>
              {t('启用')}
            </Tag>
          ) : (
            <Tag color='grey' size='small'>
              {t('停用')}
            </Tag>
          )}
        </Space>
      ),
    },
    {
      title: t('匹配范围'),
      width: 160,
      render: (_, record) => (
        <div>
          <Text>{t(record.scope_type)}</Text>
          {record.scope_value && (
            <Text type='tertiary' size='small' className='block'>
              {record.scope_value}
            </Text>
          )}
        </div>
      ),
    },
    {
      title: t('模式'),
      width: 120,
      render: (_, record) => (
        <Tag color={record.mode === 'override' ? 'orange' : 'blue'}>
          {t(record.mode)}
        </Tag>
      ),
    },
    {
      title: t('倍率'),
      dataIndex: 'multiplier',
      width: 100,
      render: (value) => `${Number(value || 0).toFixed(4)}x`,
    },
    {
      title: t('使用分组'),
      width: 170,
      render: (_, record) => renderListText(record.using_groups),
    },
    {
      title: t('模型'),
      width: 170,
      render: (_, record) => renderListText(record.models),
    },
    {
      title: t('优先级'),
      dataIndex: 'priority',
      width: 90,
    },
    {
      title: t('操作'),
      width: 110,
      render: (_, record) => (
        <Space>
          <Button
            icon={<IconEdit />}
            size='small'
            theme='borderless'
            onClick={() => openEdit(record)}
          />
          <Popconfirm
            title={t('确认删除该规则？')}
            onConfirm={() => deletePolicy(record)}
          >
            <Button
              icon={<IconDelete />}
              size='small'
              type='danger'
              theme='borderless'
              disabled={!canManage}
            />
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <Banner
        type='info'
        description={t(
          '会员、用户、分组倍率会在基础分组倍率之后生效，并写入消费日志快照。',
        )}
        style={{ marginBottom: 16 }}
      />
      <div
        className='flex justify-between items-center'
        style={{ marginBottom: 12 }}
      >
        <Button
          icon={<IconRefresh />}
          onClick={fetchPolicies}
          loading={loading}
        >
          {t('刷新')}
        </Button>
        <AdminPermissionButton
          dangerPermission={ADMIN_PERMISSION_KEYS.modelRatioUpdate}
          fallbackTooltip={ratioDangerPermissionDenied}
          icon={<IconPlus />}
          type='primary'
          theme='solid'
          onClick={openCreate}
        >
          {t('新增规则')}
        </AdminPermissionButton>
      </div>
      <Spin spinning={loading}>
        <Table
          dataSource={policies}
          columns={columns}
          rowKey='id'
          size='small'
          pagination={{ pageSize: 10 }}
        />
      </Spin>

      <Modal
        title={editing ? t('编辑规则') : t('新增规则')}
        visible={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={savePolicy}
        confirmLoading={saving}
        width={760}
      >
        <Form
          getFormApi={setFormApi}
          initValues={DEFAULT_POLICY}
          labelPosition='left'
          labelWidth={120}
        >
          <Form.Input
            field='name'
            label={t('名称')}
            rules={[{ required: true, message: t('名称不能为空') }]}
          />
          <Form.Switch field='enabled' label={t('启用')} />
          <Form.InputNumber field='priority' label={t('优先级')} step={10} />
          <Form.Select
            field='scope_type'
            label={t('匹配范围')}
            optionList={scopeOptions}
          />
          <Form.Input
            field='scope_value'
            label={t('范围值')}
            placeholder={t('全局规则可留空；用户填写ID，套餐填写ID')}
          />
          <Form.Select
            field='mode'
            label={t('计算模式')}
            optionList={modeOptions}
          />
          <Form.InputNumber
            field='multiplier'
            label={t('倍率')}
            min={0}
            step={0.01}
            precision={6}
          />
          <Form.Input
            field='using_groups'
            label={t('使用分组')}
            placeholder='codex-plus,codex-plus-special'
          />
          <Form.Input
            field='models'
            label={t('模型')}
            placeholder='gpt-5.5,gpt-5.4-mini'
          />
          <Form.InputNumber field='start_at' label={t('开始时间戳')} min={0} />
          <Form.InputNumber field='end_at' label={t('结束时间戳')} min={0} />
          <Form.TextArea field='description' label={t('备注')} autosize />

          <div style={{ marginTop: 18, marginBottom: 8 }}>
            <Text strong>{t('预览')}</Text>
          </div>
          <Form.InputNumber
            field='preview_user_id'
            label={t('用户ID')}
            min={0}
          />
          <Form.Input field='preview_user_group' label={t('用户分组')} />
          <Form.Input field='preview_using_group' label={t('使用分组')} />
          <Form.Input field='preview_model_name' label={t('模型')} />
          <Form.InputNumber
            field='preview_subscription_plan_id'
            label={t('订阅套餐ID')}
            min={0}
          />
          <Form.InputNumber
            field='preview_base_group_ratio'
            label={t('基础分组倍率')}
            min={0}
            step={0.01}
            initValue={1}
          />
          <Button
            loading={previewLoading}
            theme='borderless'
            onClick={runPreview}
            style={{ marginLeft: 120 }}
          >
            {t('计算预览')}
          </Button>
          {preview && (
            <div style={{ marginTop: 12, marginLeft: 120 }}>
              <Space wrap>
                <Tag color={preview.applied ? 'green' : 'grey'}>
                  {preview.applied ? t('已命中') : t('未命中')}
                </Tag>
                <Text>
                  {t('基础分组倍率')}: {preview.base_group_ratio}
                </Text>
                <Text>
                  {t('最终分组倍率')}: {preview.final_group_ratio}
                </Text>
                <Text>
                  {t('调整倍率')}: {preview.multiplier}
                </Text>
              </Space>
            </div>
          )}
        </Form>
      </Modal>
    </div>
  );
}
