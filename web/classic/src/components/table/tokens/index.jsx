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

import React, { useEffect, useRef, useState } from 'react';
import {
  Notification,
  Button,
  Space,
  Toast,
  Select,
  Tag,
  Tooltip,
} from '@douyinfe/semi-ui';
import {
  Activity,
  CheckCircle2,
  Database,
  KeyRound,
  Layers3,
  RefreshCw,
  ShieldCheck,
  SlidersHorizontal,
} from 'lucide-react';
import {
  API,
  showError,
  getModelCategories,
  selectFilter,
} from '../../../helpers';
import TokensTable from './TokensTable';
import TokensActions from './TokensActions';
import TokensFilters from './TokensFilters';
import EditTokenModal from './modals/EditTokenModal';
import CCSwitchModal from './modals/CCSwitchModal';
import { useTokensData } from '../../../hooks/tokens/useTokensData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';
import CompactModeToggle from '../../common/ui/CompactModeToggle';
import './tokens.css';

function getTokenStatusStats(tokens) {
  const stats = {
    enabled: 0,
    disabled: 0,
    expired: 0,
    exhausted: 0,
    limitedModels: 0,
    unlimitedQuota: 0,
  };

  for (const token of tokens || []) {
    if (token.status === 1) stats.enabled += 1;
    else if (token.status === 2) stats.disabled += 1;
    else if (token.status === 3) stats.expired += 1;
    else if (token.status === 4) stats.exhausted += 1;

    if (token.model_limits_enabled) stats.limitedModels += 1;
    if (token.unlimited_quota) stats.unlimitedQuota += 1;
  }

  return stats;
}

function TokenMetricCard({ icon: Icon, label, value, detail, tone }) {
  return (
    <div className={`ct-token-metric-card ct-token-metric-${tone || 'teal'}`}>
      <div>
        <span>{label}</span>
        <strong>{value}</strong>
        <small>{detail}</small>
      </div>
      <div className='ct-token-metric-icon'>
        <Icon size={20} />
      </div>
    </div>
  );
}

function TokensPage() {
  // Define the function first, then pass it into the hook to avoid TDZ errors
  const openFluentNotificationRef = useRef(null);
  const openCCSwitchModalRef = useRef(null);
  const tokensData = useTokensData(
    (key) => openFluentNotificationRef.current?.(key),
    (tokenId, key) => openCCSwitchModalRef.current?.(tokenId, key),
  );
  const isMobile = useIsMobile();
  const latestRef = useRef({
    tokens: [],
    selectedKeys: [],
    t: (k) => k,
    selectedModel: '',
    prefillKey: '',
    fetchTokenKey: async () => '',
  });
  const [modelOptions, setModelOptions] = useState([]);
  const [selectedModel, setSelectedModel] = useState('');
  const [fluentNoticeOpen, setFluentNoticeOpen] = useState(false);
  const [prefillKey, setPrefillKey] = useState('');
  const [ccSwitchVisible, setCCSwitchVisible] = useState(false);
  const [ccSwitchTokenId, setCCSwitchTokenId] = useState(null);
  const [ccSwitchKey, setCCSwitchKey] = useState('');

  // Keep latest data for handlers inside notifications
  useEffect(() => {
    latestRef.current = {
      tokens: tokensData.tokens,
      selectedKeys: tokensData.selectedKeys,
      t: tokensData.t,
      selectedModel,
      prefillKey,
      fetchTokenKey: tokensData.fetchTokenKey,
    };
  }, [
    tokensData.tokens,
    tokensData.selectedKeys,
    tokensData.t,
    selectedModel,
    prefillKey,
    tokensData.fetchTokenKey,
  ]);

  const loadModels = async () => {
    try {
      const res = await API.get('/api/user/models');
      const { success, message, data } = res.data || {};
      if (success) {
        const categories = getModelCategories(tokensData.t);
        const options = (data || []).map((model) => {
          let icon = null;
          for (const [key, category] of Object.entries(categories)) {
            if (key !== 'all' && category.filter({ model_name: model })) {
              icon = category.icon;
              break;
            }
          }
          return {
            label: (
              <span className='flex items-center gap-1'>
                {icon}
                {model}
              </span>
            ),
            value: model,
          };
        });
        setModelOptions(options);
      } else {
        showError(tokensData.t(message));
      }
    } catch (e) {
      showError(e.message || 'Failed to load models');
    }
  };

  function openFluentNotification(key) {
    const { t } = latestRef.current;
    const SUPPRESS_KEY = 'fluent_notify_suppressed';
    if (modelOptions.length === 0) {
      // fire-and-forget; a later effect will refresh the notice content
      loadModels();
    }
    if (!key && localStorage.getItem(SUPPRESS_KEY) === '1') return;
    const container = document.getElementById('fluent-new-api-container');
    if (!container) {
      Toast.warning(t('未检测到 FluentRead（流畅阅读），请确认扩展已启用'));
      return;
    }
    setPrefillKey(key || '');
    setFluentNoticeOpen(true);
    Notification.info({
      id: 'fluent-detected',
      title: t('检测到 FluentRead（流畅阅读）'),
      content: (
        <div>
          <div style={{ marginBottom: 8 }}>
            {key
              ? t('请选择模型。')
              : t('选择模型后可一键填充当前选中令牌（或本页第一个令牌）。')}
          </div>
          <div style={{ marginBottom: 8 }}>
            <Select
              placeholder={t('请选择模型')}
              optionList={modelOptions}
              onChange={setSelectedModel}
              filter={selectFilter}
              style={{ width: 320 }}
              showClear
              searchable
              emptyContent={t('暂无数据')}
            />
          </div>
          <Space>
            <Button
              theme='solid'
              type='primary'
              onClick={handlePrefillToFluent}
            >
              {t('一键填充到 FluentRead')}
            </Button>
            {!key && (
              <Button
                type='warning'
                onClick={() => {
                  localStorage.setItem(SUPPRESS_KEY, '1');
                  Notification.close('fluent-detected');
                  Toast.info(t('已关闭后续提醒'));
                }}
              >
                {t('不再提醒')}
              </Button>
            )}
            <Button
              type='tertiary'
              onClick={() => Notification.close('fluent-detected')}
            >
              {t('关闭')}
            </Button>
          </Space>
        </div>
      ),
      duration: 0,
    });
  }
  // assign after definition so hook callback can call it safely
  openFluentNotificationRef.current = openFluentNotification;

  function openCCSwitchModal(tokenId, key) {
    setCCSwitchTokenId(tokenId || null);
    setCCSwitchKey(key || '');
    setCCSwitchVisible(true);
  }
  openCCSwitchModalRef.current = openCCSwitchModal;

  // Prefill to Fluent handler
  const handlePrefillToFluent = async () => {
    const {
      tokens,
      selectedKeys,
      t,
      selectedModel: chosenModel,
      prefillKey: overrideKey,
      fetchTokenKey,
    } = latestRef.current;
    const container = document.getElementById('fluent-new-api-container');
    if (!container) {
      Toast.error(t('未检测到 Fluent 容器'));
      return;
    }

    if (!chosenModel) {
      Toast.warning(t('请选择模型'));
      return;
    }

    let status = localStorage.getItem('status');
    let serverAddress = '';
    if (status) {
      try {
        status = JSON.parse(status);
        serverAddress = status.server_address || '';
      } catch (_) {}
    }
    if (!serverAddress) serverAddress = window.location.origin;

    let apiKeyToUse = '';
    if (overrideKey) {
      apiKeyToUse = 'sk-' + overrideKey;
    } else {
      const token =
        selectedKeys && selectedKeys.length === 1
          ? selectedKeys[0]
          : tokens && tokens.length > 0
            ? tokens[0]
            : null;
      if (!token) {
        Toast.warning(t('没有可用令牌用于填充'));
        return;
      }
      try {
        apiKeyToUse = 'sk-' + (await fetchTokenKey(token));
      } catch (_) {
        return;
      }
    }

    const payload = {
      id: 'new-api',
      baseUrl: serverAddress,
      apiKey: apiKeyToUse,
      model: chosenModel,
    };

    container.dispatchEvent(
      new CustomEvent('fluent:prefill', { detail: payload }),
    );
    Toast.success(t('已发送到 Fluent'));
    Notification.close('fluent-detected');
  };

  // Show notification when Fluent container is available
  useEffect(() => {
    const onAppeared = () => {
      openFluentNotification();
    };
    const onRemoved = () => {
      setFluentNoticeOpen(false);
      Notification.close('fluent-detected');
    };

    window.addEventListener('fluent-container:appeared', onAppeared);
    window.addEventListener('fluent-container:removed', onRemoved);
    return () => {
      window.removeEventListener('fluent-container:appeared', onAppeared);
      window.removeEventListener('fluent-container:removed', onRemoved);
    };
  }, []);

  // When modelOptions or language changes while the notice is open, refresh the content
  useEffect(() => {
    if (fluentNoticeOpen) {
      openFluentNotification();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [modelOptions, selectedModel, tokensData.t, fluentNoticeOpen]);

  useEffect(() => {
    const selector = '#fluent-new-api-container';
    const root = document.body || document.documentElement;

    const existing = document.querySelector(selector);
    if (existing) {
      console.log('Fluent container detected (initial):', existing);
      window.dispatchEvent(
        new CustomEvent('fluent-container:appeared', { detail: existing }),
      );
    }

    const isOrContainsTarget = (node) => {
      if (!(node && node.nodeType === 1)) return false;
      if (node.id === 'fluent-new-api-container') return true;
      return (
        typeof node.querySelector === 'function' &&
        !!node.querySelector(selector)
      );
    };

    const observer = new MutationObserver((mutations) => {
      for (const m of mutations) {
        // appeared
        for (const added of m.addedNodes) {
          if (isOrContainsTarget(added)) {
            const el = document.querySelector(selector);
            if (el) {
              console.log('Fluent container appeared:', el);
              window.dispatchEvent(
                new CustomEvent('fluent-container:appeared', { detail: el }),
              );
            }
            break;
          }
        }
        // removed
        for (const removed of m.removedNodes) {
          if (isOrContainsTarget(removed)) {
            const elNow = document.querySelector(selector);
            if (!elNow) {
              console.log('Fluent container removed');
              window.dispatchEvent(new CustomEvent('fluent-container:removed'));
            }
            break;
          }
        }
      }
    });

    observer.observe(root, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, []);

  const {
    // Edit state
    showEdit,
    editingToken,
    closeEdit,
    refresh,

    // Actions state
    selectedKeys,
    setEditingToken,
    setShowEdit,
    batchCopyTokens,
    batchDeleteTokens,

    // Filters state
    formInitValues,
    setFormApi,
    searchTokens,
    loading,
    searching,

    // Description state
    compactMode,
    setCompactMode,
    tokens,
    tokenCount,
    pageSize,
    activePage,

    // Translation
    t,
  } = tokensData;

  const tokenStats = getTokenStatusStats(tokensData.tokens);
  const selectedCount = selectedKeys.length;
  const pageStart = tokenCount > 0 ? (activePage - 1) * pageSize + 1 : 0;
  const pageEnd =
    tokenCount > 0 ? Math.min(activePage * pageSize, tokenCount) : 0;
  const disabledLikeCount =
    tokenStats.disabled + tokenStats.expired + tokenStats.exhausted;
  const selectedLabel =
    selectedCount > 0
      ? t('已选择 {{count}} 个令牌', { count: selectedCount })
      : t('未选择令牌');

  return (
    <div className='ct-token-page ct-console-page-shell'>
      <EditTokenModal
        refresh={refresh}
        editingToken={editingToken}
        visiable={showEdit}
        handleClose={closeEdit}
      />

      <CCSwitchModal
        visible={ccSwitchVisible}
        onClose={() => setCCSwitchVisible(false)}
        tokenKey={ccSwitchKey}
        tokenId={ccSwitchTokenId}
      />

      <div className='ct-console-page-header ct-token-hero'>
        <div className='ct-console-page-heading'>
          <div className='ct-console-page-eyebrow'>{t('API 访问控制台')}</div>
          <div className='ct-console-page-title-row'>
            <h1 className='ct-console-page-title'>{t('令牌管理')}</h1>
            <Tag color='teal' shape='circle' type='light'>
              {t('安全密钥资产')}
            </Tag>
          </div>
          <p className='ct-console-page-subtitle'>
            {t(
              '统一管理 API 访问令牌、额度、分组和接入方式，快速识别可用状态并完成批量操作。',
            )}
          </p>
        </div>

        <div className='ct-console-page-actions ct-token-hero-actions'>
          <Tooltip content={t('刷新令牌列表')}>
            <Button
              icon={<RefreshCw size={16} />}
              type='tertiary'
              theme='borderless'
              loading={loading}
              className='ct-token-icon-button'
              onClick={() => refresh()}
            />
          </Tooltip>
          <CompactModeToggle
            compactMode={compactMode}
            setCompactMode={setCompactMode}
            t={t}
          />
        </div>
      </div>

      <div className='ct-token-metrics-grid'>
        <TokenMetricCard
          icon={KeyRound}
          tone='teal'
          label={t('令牌总数')}
          value={tokenCount}
          detail={
            tokenCount > 0
              ? t('当前显示 {{start}}-{{end}} 条', {
                  start: pageStart,
                  end: pageEnd,
                })
              : t('暂无令牌')
          }
        />
        <TokenMetricCard
          icon={CheckCircle2}
          tone='green'
          label={t('当前页启用')}
          value={`${tokenStats.enabled}/${tokens.length || 0}`}
          detail={t('禁用或不可用 {{count}} 个', { count: disabledLikeCount })}
        />
        <TokenMetricCard
          icon={ShieldCheck}
          tone='blue'
          label={t('模型限制')}
          value={tokenStats.limitedModels}
          detail={t('无限额度 {{count}} 个', {
            count: tokenStats.unlimitedQuota,
          })}
        />
        <TokenMetricCard
          icon={Layers3}
          tone='amber'
          label={t('批量选择')}
          value={selectedCount}
          detail={selectedLabel}
        />
      </div>

      <div className='ct-token-workbench'>
        <div className='ct-token-workbench-head'>
          <div className='ct-token-workbench-title'>
            <div className='ct-token-workbench-icon'>
              <Database size={18} />
            </div>
            <div>
              <strong>{t('令牌清单')}</strong>
              <span>
                {searching
                  ? t('正在筛选匹配令牌')
                  : t('按状态、额度和接入方式扫描令牌')}
              </span>
            </div>
          </div>

          <div className='ct-token-workbench-status'>
            <Activity size={15} />
            <span>{loading ? t('同步中') : t('已同步')}</span>
          </div>
        </div>

        <div className='ct-token-toolbar'>
          <div className='ct-token-toolbar-actions'>
            <TokensActions
              selectedKeys={selectedKeys}
              setEditingToken={setEditingToken}
              setShowEdit={setShowEdit}
              batchCopyTokens={batchCopyTokens}
              batchDeleteTokens={batchDeleteTokens}
              t={t}
            />
          </div>

          <div className='ct-token-toolbar-search'>
            <TokensFilters
              formInitValues={formInitValues}
              setFormApi={setFormApi}
              searchTokens={searchTokens}
              loading={loading}
              searching={searching}
              t={t}
            />
          </div>
        </div>

        {selectedCount > 0 && (
          <div className='ct-token-selection-bar'>
            <div>
              <SlidersHorizontal size={16} />
              <span>{selectedLabel}</span>
            </div>
            <span>{t('可复制或批量删除所选令牌')}</span>
          </div>
        )}

        <div className='ct-token-table-shell'>
          <TokensTable {...tokensData} />
        </div>

        <div className='ct-token-pagination'>
          {createCardProPagination({
            currentPage: tokensData.activePage,
            pageSize: tokensData.pageSize,
            total: tokensData.tokenCount,
            onPageChange: tokensData.handlePageChange,
            onPageSizeChange: tokensData.handlePageSizeChange,
            isMobile: isMobile,
            t: tokensData.t,
          })}
        </div>
      </div>
    </div>
  );
}

export default TokensPage;
