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

import React, { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Banner,
  Input,
  Modal,
  Select,
  Switch,
  TextArea,
  Typography,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../helpers';
import './ProxyEditorModal.css';

const { Text } = Typography;

const supportedProxyProtocols = new Set(['socks5', 'socks5h', 'http', 'https']);

class ProxyStringParser {
  constructor(defaultProtocol = 'socks5') {
    this.defaultProtocol = defaultProtocol;
  }

  parse(rawText) {
    const candidates = this.extractCandidates(rawText);
    for (const candidate of candidates) {
      const parsed =
        this.parseInlineAuth(candidate) ||
        this.parseUrl(candidate) ||
        this.parseSeparated(candidate);
      if (parsed) {
        return parsed;
      }
    }
    return null;
  }

  extractCandidates(rawText) {
    const text = String(rawText || '').trim();
    if (!text) {
      return [];
    }
    const candidates = new Set();
    const addCandidate = (value) => {
      const normalized = this.stripWrapper(value);
      if (normalized) {
        candidates.add(normalized);
      }
    };

    addCandidate(text);
    text
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter(Boolean)
      .forEach((line) => {
        addCandidate(line);
        line
          .split(/[\s;]+/)
          .map((part) => part.trim())
          .filter(Boolean)
          .forEach(addCandidate);
      });

    const proxyLikeMatches = text.match(
      /(?:https?|socks5h?|socks):\/\/[^\s'",;]+|[^\s'",;@]+:[^\s'",;@]+@[^\s'",;]+:\d{1,5}|[^\s'",;:]+:\d{1,5}:[^\s'",;:]+:[^\s'",;]+/gi,
    );
    (proxyLikeMatches || []).forEach(addCandidate);

    return [...candidates];
  }

  parseInlineAuth(candidate) {
    const source = this.extractProtocol(candidate);
    if (!source || !source.rest.includes('@')) {
      return null;
    }
    const atIndex = source.rest.lastIndexOf('@');
    const credentialPart = source.rest.slice(0, atIndex);
    const endpoint = this.cleanEndpoint(source.rest.slice(atIndex + 1));
    if (!credentialPart || !endpoint) {
      return null;
    }
    const separatorIndex = credentialPart.indexOf(':');
    const username =
      separatorIndex >= 0
        ? credentialPart.slice(0, separatorIndex)
        : credentialPart;
    const password =
      separatorIndex >= 0 ? credentialPart.slice(separatorIndex + 1) : '';

    return this.buildResult({
      protocol: source.protocol,
      address: endpoint,
      username: this.safeDecode(username),
      password: this.safeDecode(password),
    });
  }

  parseUrl(candidate) {
    const source = this.extractProtocol(candidate);
    if (!source) {
      return null;
    }
    try {
      const parsed = new URL(`${source.protocol}://${source.rest}`);
      if (!parsed.host) {
        return null;
      }
      return this.buildResult({
        protocol: source.protocol,
        address: parsed.host,
        username: this.safeDecode(parsed.username),
        password: this.safeDecode(parsed.password),
      });
    } catch {
      return null;
    }
  }

  parseSeparated(candidate) {
    const source = this.extractProtocol(candidate);
    if (!source || source.rest.includes('@')) {
      return null;
    }
    const endpoint = this.cleanEndpoint(source.rest);
    const separators = ['|', ',', '\t', ':'];
    for (const separator of separators) {
      if (!endpoint.includes(separator)) {
        continue;
      }
      const parts = endpoint
        .split(separator)
        .map((part) => this.stripWrapper(part))
        .filter(Boolean);
      const parsedParts = this.parseOrderedParts(parts, source.protocol);
      if (parsedParts) {
        return parsedParts;
      }
    }
    return null;
  }

  parseOrderedParts(parts, protocol) {
    if (parts.length < 4) {
      return null;
    }
    if (this.isLikelyPort(parts[1]) && this.isLikelyHost(parts[0])) {
      return this.buildResult({
        protocol,
        address: `${parts[0]}:${parts[1]}`,
        username: parts[2],
        password: parts.slice(3).join(':'),
      });
    }
    const last = parts.length - 1;
    if (this.isLikelyPort(parts[last]) && this.isLikelyHost(parts[last - 1])) {
      return this.buildResult({
        protocol,
        address: `${parts[last - 1]}:${parts[last]}`,
        username: parts[0],
        password: parts.slice(1, last - 1).join(':'),
      });
    }
    return null;
  }

  extractProtocol(candidate) {
    const cleaned = this.stripWrapper(candidate);
    const protocolMatch = cleaned.match(/^([a-z][a-z0-9+.-]*):\/\//i);
    if (!protocolMatch) {
      return {
        protocol: this.defaultProtocol,
        rest: cleaned,
      };
    }
    const protocol = this.normalizeProtocol(protocolMatch[1]);
    if (!protocol) {
      return null;
    }
    return {
      protocol,
      rest: cleaned.slice(protocolMatch[0].length),
    };
  }

  normalizeProtocol(protocol) {
    const normalized = String(protocol || '')
      .trim()
      .toLowerCase()
      .replace(/:$/, '');
    if (normalized === 'socks') {
      return 'socks5';
    }
    return supportedProxyProtocols.has(normalized) ? normalized : null;
  }

  cleanEndpoint(value) {
    return this.stripWrapper(value).split(/[/?#]/)[0].replace(/\/+$/, '');
  }

  stripWrapper(value) {
    return String(value || '')
      .trim()
      .replace(/^[\s"'`({]+/, '')
      .replace(/[\s"'`)};,]+$/, '');
  }

  safeDecode(value) {
    try {
      return decodeURIComponent(value || '');
    } catch {
      return value || '';
    }
  }

  isLikelyHost(value) {
    return Boolean(
      value &&
        (value === 'localhost' ||
          value.includes('.') ||
          value.includes(':') ||
          /^[a-z0-9-]+$/i.test(value)),
    );
  }

  isLikelyPort(value) {
    const port = Number(value);
    return Number.isInteger(port) && port > 0 && port <= 65535;
  }

  hasAddressPort(protocol, address) {
    try {
      const parsed = new URL(`${protocol}://${address}`);
      return Boolean(parsed.host && parsed.port);
    } catch {
      return false;
    }
  }

  buildResult(result) {
    const address = this.cleanEndpoint(result.address);
    if (!result.protocol || !address || !this.hasAddressPort(result.protocol, address)) {
      return null;
    }
    return {
      protocol: result.protocol,
      address,
      username: result.username || '',
      password: result.password || '',
    };
  }
}

const proxyStringParser = new ProxyStringParser();

const emptyProxyForm = {
  name: '',
  protocol: 'socks5',
  address: '',
  username: '',
  password: '',
  enabled: true,
  remark: '',
};

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function proxyAddress(proxy) {
  return proxy?.masked_address || proxy?.address || '';
}

function defaultProxyNameFromParsed(parsed) {
  if (!parsed?.protocol || !parsed?.address) {
    return '';
  }
  return `${parsed.protocol}://${parsed.address}`;
}

function ProxyEditorModal({ visible, proxy, onCancel, onSaved }) {
  const { t } = useTranslation();
  const [saving, setSaving] = useState(false);
  const [form, setForm] = useState(emptyProxyForm);
  const [quickProxyText, setQuickProxyText] = useState('');
  const [quickFillStatus, setQuickFillStatus] = useState(null);
  const [autoProxyName, setAutoProxyName] = useState('');
  const isEditing = Boolean(proxy?.id);

  useEffect(() => {
    if (!visible) return;
    setForm({
      name: proxy?.name || '',
      protocol: proxy?.protocol || 'socks5',
      address: '',
      username: proxy?.username || '',
      password: '',
      enabled: proxy?.enabled !== false,
      remark: proxy?.remark || '',
    });
    setQuickProxyText('');
    setQuickFillStatus(null);
    setAutoProxyName('');
  }, [proxy, visible]);

  const applyParsedProxy = useCallback((parsed) => {
    const generatedName = defaultProxyNameFromParsed(parsed);
    setForm((prev) => ({
      ...prev,
      protocol: parsed.protocol,
      address: parsed.address,
      username: parsed.username,
      password: parsed.password,
      name:
        prev.name.trim() && prev.name !== autoProxyName
          ? prev.name
          : generatedName,
    }));
    setAutoProxyName(generatedName);
  }, [autoProxyName]);

  const parseQuickProxyText = useCallback((value, showInvalid = false) => {
    const text = String(value || '').trim();
    if (!text) {
      setQuickFillStatus(null);
      return;
    }
    const parsed = proxyStringParser.parse(text);
    if (parsed) {
      applyParsedProxy(parsed);
      setQuickFillStatus('success');
      return;
    }
    setQuickFillStatus(showInvalid ? 'warning' : null);
  }, [applyParsedProxy]);

  const handleQuickProxyTextChange = useCallback(
    (value) => {
      setQuickProxyText(value);
      parseQuickProxyText(value);
    },
    [parseQuickProxyText],
  );

  const handleProxyAddressPaste = useCallback(
    (event) => {
      const clipboardText = event?.clipboardData?.getData('text');
      const parsed = proxyStringParser.parse(clipboardText);
      if (!parsed) {
        return;
      }
      event.preventDefault();
      setQuickProxyText(clipboardText);
      applyParsedProxy(parsed);
      setQuickFillStatus('success');
    },
    [applyParsedProxy],
  );

  const saveProxy = useCallback(async () => {
    if (!isEditing && !form.address.trim()) {
      showError(t('请填写代理地址'));
      return;
    }
    setSaving(true);
    try {
      const payload = {
        ...form,
        name: form.name.trim(),
        address: form.address.trim(),
        username: form.username.trim(),
        remark: form.remark.trim(),
      };
      const response = isEditing
        ? await API.put(`/api/model_gateway/proxies/${proxy.id}`, payload)
        : await API.post('/api/model_gateway/proxies', payload);
      if (response?.data?.success === false) {
        throw new Error(response?.data?.message || t('保存失败'));
      }
      const savedProxy = unwrapApiData(response);
      showSuccess(isEditing ? t('代理已更新') : t('代理已创建'));
      onCancel?.();
      await onSaved?.(savedProxy);
    } catch (err) {
      const message =
        err?.response?.data?.message || err?.message || t('保存失败');
      showError(message);
    } finally {
      setSaving(false);
    }
  }, [form, isEditing, onCancel, onSaved, proxy, t]);

  return (
    <Modal
      title={isEditing ? t('编辑代理') : t('新增代理')}
      visible={visible}
      width={680}
      okText={t('保存')}
      cancelText={t('取消')}
      confirmLoading={saving}
      onOk={saveProxy}
      onCancel={onCancel}
    >
      <div className='ct-proxy-editor-form'>
        {isEditing ? (
          <Banner
            type='info'
            closeIcon={null}
            description={t('编辑时代理地址和密码留空会保留原值，列表不会展示完整密码')}
          />
        ) : null}
        <div className='ct-proxy-editor-quick-fill'>
          <TextArea
            autosize={{ minRows: 2, maxRows: 4 }}
            value={quickProxyText}
            onChange={handleQuickProxyTextChange}
            onBlur={() => parseQuickProxyText(quickProxyText, true)}
            placeholder={t('粘贴代理字符串自动填充')}
          />
          <Text
            size='small'
            className={`ct-proxy-editor-quick-fill-status${
              quickFillStatus ? ` is-${quickFillStatus}` : ''
            }`}
          >
            {quickFillStatus === 'success'
              ? t('已自动填充代理字段')
              : quickFillStatus === 'warning'
                ? t('未识别出可用代理格式，请检查是否包含地址和端口')
                : t(
                    '支持格式：user:pass@host:port、host:port:user:pass、protocol://user:pass@host:port',
                  )}
          </Text>
        </div>
        <Input
          value={form.name}
          onChange={(value) => setForm((prev) => ({ ...prev, name: value }))}
          placeholder={t('代理名称（可选）')}
        />
        <div className='ct-proxy-editor-form-row'>
          <Select
            value={form.protocol}
            onChange={(value) =>
              setForm((prev) => ({ ...prev, protocol: value }))
            }
            className='ct-proxy-editor-protocol-select'
          >
            <Select.Option value='socks5'>SOCKS5</Select.Option>
            <Select.Option value='socks5h'>SOCKS5H</Select.Option>
            <Select.Option value='http'>HTTP</Select.Option>
            <Select.Option value='https'>HTTPS</Select.Option>
          </Select>
          <Input
            value={form.address}
            onChange={(value) =>
              setForm((prev) => ({ ...prev, address: value }))
            }
            onPaste={handleProxyAddressPaste}
            placeholder={
              isEditing
                ? t('留空保持原地址，当前：{{address}}', {
                    address: proxyAddress(proxy) || '--',
                  })
                : '127.0.0.1:1080'
            }
          />
        </div>
        <div className='ct-proxy-editor-form-row'>
          <Input
            value={form.username}
            onChange={(value) =>
              setForm((prev) => ({ ...prev, username: value }))
            }
            placeholder={t('代理用户名（可选）')}
          />
          <Input
            type='password'
            value={form.password}
            onChange={(value) =>
              setForm((prev) => ({ ...prev, password: value }))
            }
            placeholder={
              proxy?.password_set
                ? t('留空保持原密码')
                : t('代理密码（可选）')
            }
          />
        </div>
        <Input
          value={form.remark}
          onChange={(value) =>
            setForm((prev) => ({ ...prev, remark: value }))
          }
          placeholder={t('备注（可选）')}
        />
        <div className='ct-proxy-editor-form-enabled'>
          <Text>{t('启用代理')}</Text>
          <Switch
            checked={form.enabled}
            onChange={(checked) =>
              setForm((prev) => ({ ...prev, enabled: checked }))
            }
          />
        </div>
      </div>
    </Modal>
  );
}

export default ProxyEditorModal;
