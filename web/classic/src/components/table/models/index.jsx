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

import React, { useState } from 'react';
import { Banner, Button, Modal, Tag } from '@douyinfe/semi-ui';
import { IconAlertTriangle, IconClose } from '@douyinfe/semi-icons';
import { Boxes, Database, Layers3, Store } from 'lucide-react';
import ConsoleTableScaffold from '../../common/ui/ConsoleTableScaffold';
import ModelsTable from './ModelsTable';
import ModelsActions from './ModelsActions';
import ModelsFilters from './ModelsFilters';
import ModelsTabs from './ModelsTabs';
import EditModelModal from './modals/EditModelModal';
import EditVendorModal from './modals/EditVendorModal';
import { useModelsData } from '../../../hooks/models/useModelsData';
import { useIsMobile } from '../../../hooks/common/useIsMobile';
import { createCardProPagination } from '../../../helpers/utils';

const MARKETPLACE_DISPLAY_NOTICE_STORAGE_KEY =
  'models_marketplace_display_notice_dismissed';

const ModelsPage = () => {
  const modelsData = useModelsData();
  const isMobile = useIsMobile();

  const {
    // Edit state
    showEdit,
    editingModel,
    closeEdit,
    refresh,

    // Actions state
    selectedKeys,
    setSelectedKeys,
    setEditingModel,
    setShowEdit,
    batchDeleteModels,

    // Filters state
    formInitValues,
    setFormApi,
    searchModels,
    loading,
    searching,

    // Description state
    compactMode,
    setCompactMode,

    // Vendor state
    vendors,
    showAddVendor,
    setShowAddVendor,
    showEditVendor,
    setShowEditVendor,
    editingVendor,
    setEditingVendor,
    loadVendors,

    // Translation
    t,
  } = modelsData;
  const visibleCount = modelsData.models?.length || 0;
  const selectedCount = selectedKeys.length;

  const [showMarketplaceDisplayNotice, setShowMarketplaceDisplayNotice] =
    useState(() => {
      try {
        return (
          localStorage.getItem(MARKETPLACE_DISPLAY_NOTICE_STORAGE_KEY) !== '1'
        );
      } catch (_) {
        return true;
      }
    });

  const confirmCloseMarketplaceDisplayNotice = () => {
    Modal.confirm({
      title: t('确认关闭提示'),
      content: t(
        '关闭后将不再显示此提示（仅对当前浏览器生效）。确定要关闭吗？',
      ),
      okText: t('关闭提示'),
      cancelText: t('取消'),
      okButtonProps: {
        type: 'danger',
      },
      onOk: () => {
        try {
          localStorage.setItem(MARKETPLACE_DISPLAY_NOTICE_STORAGE_KEY, '1');
        } catch (_) {}
        setShowMarketplaceDisplayNotice(false);
      },
    });
  };

  return (
    <>
      <EditModelModal
        refresh={refresh}
        editingModel={editingModel}
        visiable={showEdit}
        handleClose={closeEdit}
      />

      <EditVendorModal
        visible={showAddVendor || showEditVendor}
        handleClose={() => {
          setShowAddVendor(false);
          setShowEditVendor(false);
          setEditingVendor({ id: undefined });
        }}
        editingVendor={showEditVendor ? editingVendor : { id: undefined }}
        refresh={() => {
          loadVendors();
          refresh();
        }}
      />

      <ConsoleTableScaffold
        eyebrow={t('模型与路由')}
        title={t('模型管理')}
        subtitle={t(
          '维护模型广场展示、供应商归属和上游同步状态，调用路由仍以渠道配置为准。',
        )}
        badge={
          <Tag color='teal' shape='circle' type='light'>
            {t('展示配置')}
          </Tag>
        }
        metrics={[
          {
            key: 'models',
            label: t('模型总数'),
            value: modelsData.modelCount,
            helper: t('当前筛选范围'),
            tone: 'teal',
            icon: <Layers3 size={20} />,
          },
          {
            key: 'page',
            label: t('当前页'),
            value: visibleCount,
            helper: t('可见模型记录'),
            tone: 'blue',
            icon: <Database size={20} />,
          },
          {
            key: 'vendors',
            label: t('供应商'),
            value: vendors.length,
            helper: t('已加载供应商'),
            tone: 'green',
            icon: <Store size={20} />,
          },
          {
            key: 'selected',
            label: t('已选择'),
            value: selectedCount,
            helper: t('用于批量操作'),
            tone: selectedCount > 0 ? 'amber' : 'teal',
            icon: <Boxes size={20} />,
          },
        ]}
        tabs={<ModelsTabs {...modelsData} />}
        tableTitle={t('模型清单')}
        tableSubtitle={
          searching ? t('正在筛选匹配模型') : t('按模型、供应商和展示状态扫描')
        }
        tableIcon={<Layers3 size={18} />}
        tableMeta={`${t('共')} ${modelsData.modelCount} ${t('条')}`}
        toolbar={
          <>
            <ModelsActions
              selectedKeys={selectedKeys}
              setSelectedKeys={setSelectedKeys}
              setEditingModel={setEditingModel}
              setShowEdit={setShowEdit}
              batchDeleteModels={batchDeleteModels}
              syncing={modelsData.syncing}
              syncUpstream={modelsData.syncUpstream}
              previewing={modelsData.previewing}
              previewUpstreamDiff={modelsData.previewUpstreamDiff}
              applyUpstreamOverwrite={modelsData.applyUpstreamOverwrite}
              compactMode={compactMode}
              setCompactMode={setCompactMode}
              t={t}
            />
          </>
        }
      >
        {showMarketplaceDisplayNotice ? (
          <div className='ct-console-table-section'>
            <div style={{ position: 'relative' }}>
              <Banner
                type='warning'
                closeIcon={null}
                icon={
                  <IconAlertTriangle
                    size='large'
                    style={{ color: 'var(--semi-color-warning)' }}
                  />
                }
                description={t(
                  '提示：此处配置仅用于控制「模型广场」对用户的展示效果，不会影响模型的实际调用与路由。若需配置真实调用行为，请前往「渠道管理」进行设置。',
                )}
                style={{ marginBottom: 0 }}
              />
              <Button
                theme='borderless'
                size='small'
                type='tertiary'
                icon={<IconClose aria-hidden={true} />}
                onClick={confirmCloseMarketplaceDisplayNotice}
                style={{ position: 'absolute', top: 8, right: 8 }}
                aria-label={t('关闭')}
              />
            </div>
          </div>
        ) : null}
        <div className='ct-console-table-section ct-console-table-filter-panel'>
          <ModelsFilters
            formInitValues={formInitValues}
            setFormApi={setFormApi}
            searchModels={searchModels}
            loading={loading}
            searching={searching}
            t={t}
          />
        </div>
        <div className='ct-console-table-surface'>
          <ModelsTable {...modelsData} />
        </div>
        <div className='ct-console-table-pagination'>
          {createCardProPagination({
            currentPage: modelsData.activePage,
            pageSize: modelsData.pageSize,
            total: modelsData.modelCount,
            onPageChange: modelsData.handlePageChange,
            onPageSizeChange: modelsData.handlePageSizeChange,
            isMobile: isMobile,
            t: modelsData.t,
          })}
        </div>
      </ConsoleTableScaffold>
    </>
  );
};

export default ModelsPage;
