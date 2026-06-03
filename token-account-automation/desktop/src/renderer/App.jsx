import { useEffect, useMemo, useState } from 'react';
import {
  Badge,
  Button,
  Card,
  Checkbox,
  Form,
  Input,
  Layout,
  Modal,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Toast,
  Typography,
} from '@douyinfe/semi-ui';
import {
  IconDelete,
  IconPause,
  IconPlay,
  IconPlus,
  IconRefresh,
  IconRestoreStroked,
  IconSave,
} from '@douyinfe/semi-icons';

const { Header, Content } = Layout;
const { Text } = Typography;
const ACTIVE_STATUSES = ['PENDING', 'LEASED', 'RUNNING', 'WAITING_HUMAN'];
const TERMINAL_STATUSES = ['SUCCESS', 'FAILED', 'CANCELED', 'EXPIRED'];

export default function App() {
  const [state, setState] = useState(null);
  const [overview, setOverview] = useState(null);
  const [accounts, setAccounts] = useState({ items: [], total: 0 });
  const [invalidPool, setInvalidPool] = useState({ items: [], total: 0 });
  const [actionTemplates, setActionTemplates] = useState({ items: [], total: 0 });
  const [events, setEvents] = useState([]);
  const [taskStatus, setTaskStatus] = useState('ACTIVE');
  const [taskKeyword, setTaskKeyword] = useState('');
  const [invalidKeyword, setInvalidKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [proxySyncing, setProxySyncing] = useState(false);
  const [proxySyncError, setProxySyncError] = useState('');
  const [diagnostics, setDiagnostics] = useState(null);
  const [diagnosticsLoading, setDiagnosticsLoading] = useState(false);
  const [proxyModal, setProxyModal] = useState(false);
  const [detailVisible, setDetailVisible] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [jobDetail, setJobDetail] = useState(null);
  const [actionLoading, setActionLoading] = useState('');
  const [retryProxyId, setRetryProxyId] = useState('');
  const [retryClearSession, setRetryClearSession] = useState(true);
  const [selectedRowKeys, setSelectedRowKeys] = useState([]);
  const [selectedRows, setSelectedRows] = useState([]);
  const [bulkModal, setBulkModal] = useState(false);
  const [bulkAction, setBulkAction] = useState('');
  const [bulkProxyId, setBulkProxyId] = useState('');
  const [bulkClearSession, setBulkClearSession] = useState(true);
  const [activeTab, setActiveTab] = useState('tasks');

  const settings = state?.store?.settings || {};
  const proxies = state?.store?.proxies || [];
  const proxyHealth = state?.store?.proxyHealth || {};
  const sortedProxies = useMemo(() => sortProxiesByHealth(proxies), [proxies]);
  const executor = state?.executor || { running: false, active: [] };
  const settingsFormKey = useMemo(
    () => [
      settings.automationBaseUrl || '',
      settings.desktopToken ? 'token-set' : 'token-empty',
      settings.workerId || '',
      settings.maxConcurrency || '',
      settings.callbackPort || '',
    ].join('|'),
    [
      settings.automationBaseUrl,
      settings.desktopToken,
      settings.workerId,
      settings.maxConcurrency,
      settings.callbackPort,
    ],
  );

  useEffect(() => {
    bootstrap();
    return window.desktopAutomation.onEvent((event, payload) => {
      setEvents((items) => [{ event, payload, at: Date.now() }, ...items].slice(0, 80));
      window.desktopAutomation.getState().then(setState).catch(() => {});
      if (event.includes('success') || event.includes('error')) {
        refreshData();
      }
    });
  }, []);

  useEffect(() => {
    if (!executor.running) {
      return undefined;
    }
    const timer = window.setInterval(() => refreshData(), 5000);
    return () => window.clearInterval(timer);
  }, [executor.running, taskKeyword, taskStatus, invalidKeyword]);

  async function bootstrap() {
    const next = await window.desktopAutomation.getState();
    setState(next);
    await refreshData();
    await refreshDiagnostics();
  }

  async function refreshData() {
    setLoading(true);
    try {
      const [nextOverview, nextAccounts, nextInvalidPool, nextActionTemplates] = await Promise.all([
        window.desktopAutomation.overview().catch(() => null),
        loadAccountItems(taskStatus, taskKeyword.trim()),
        loadInvalidPoolItems(invalidKeyword.trim()),
        window.desktopAutomation.actionTemplates().catch(() => ({ items: [], total: 0 })),
      ]);
      setOverview(nextOverview);
      setAccounts(nextAccounts);
      setInvalidPool(nextInvalidPool);
      setActionTemplates(nextActionTemplates);
      pruneSelection(nextAccounts.items || []);
    } finally {
      setLoading(false);
    }
  }

  async function saveSettings(values) {
    const next = await window.desktopAutomation.updateSettings(values);
    setState(next);
    Toast.success('配置已保存');
  }

  async function startExecutor() {
    try {
      const next = await window.desktopAutomation.startExecutor();
      setState((prev) => ({ ...prev, executor: next }));
      Toast.success('执行器已启动');
    } catch (error) {
      Toast.error(error.message);
    }
  }

  async function stopExecutor() {
    const next = await window.desktopAutomation.stopExecutor();
    setState((prev) => ({ ...prev, executor: next }));
  }

  async function syncProxies() {
    setProxySyncing(true);
    setProxySyncError('');
    try {
      const payload = await window.desktopAutomation.syncProxies({ enabled_only: true });
      setState((prev) => ({ ...prev, store: payload.store }));
      const total = payload.result?.total ?? payload.result?.items?.length ?? 0;
      Toast.success(`已同步 ${total} 个主站代理`);
    } catch (error) {
      const message = explainProxySyncError(error.message);
      setProxySyncError(message);
      Toast.error(message);
    } finally {
      setProxySyncing(false);
    }
  }

  async function refreshDiagnostics() {
    setDiagnosticsLoading(true);
    try {
      setDiagnostics(await window.desktopAutomation.getDiagnostics());
    } catch (error) {
      Toast.error(error.message);
    } finally {
      setDiagnosticsLoading(false);
    }
  }

  async function openJobDetail(jobId) {
    if (!jobId) {
      return;
    }
    setDetailVisible(true);
    setDetailLoading(true);
    try {
      const detail = await window.desktopAutomation.jobDetail(jobId);
      setJobDetail(detail);
      setRetryProxyId(parseJSON(detail?.job?.input_json)?.preferred_proxy_id || '');
      setRetryClearSession(parseJSON(detail?.job?.input_json)?.clear_session !== false);
    } catch (error) {
      Toast.error(error.message);
    } finally {
      setDetailLoading(false);
    }
  }

  async function refreshJobDetail() {
    if (jobDetail?.job?.job_id) {
      await openJobDetail(jobDetail.job.job_id);
    }
  }

  async function runJobAction(action, handler) {
    setActionLoading(action);
    try {
      await handler();
      Toast.success('操作已提交');
      await Promise.all([refreshData(), refreshJobDetail()]);
    } catch (error) {
      Toast.error(error.message);
    } finally {
      setActionLoading('');
    }
  }

  async function retrySelectedJob() {
    const jobId = jobDetail?.job?.job_id;
    if (!jobId) {
      return;
    }
    await runJobAction('retry', () => window.desktopAutomation.retryJob(jobId, {
      preferred_proxy_id: retryProxyId || undefined,
      clear_session: retryClearSession,
      reason: 'desktop operator retry',
    }));
  }

  async function resumeSelectedJob() {
    const jobId = jobDetail?.job?.job_id;
    if (!jobId) {
      return;
    }
    await runJobAction('resume', () => window.desktopAutomation.resumeJob(jobId, {
      reason: 'desktop operator completed required action',
    }));
  }

  async function cancelSelectedJob() {
    const jobId = jobDetail?.job?.job_id;
    if (!jobId || !window.confirm('确认取消这个授权任务？')) {
      return;
    }
    await runJobAction('cancel', () => window.desktopAutomation.cancelJob(jobId, {
      reason: 'desktop operator canceled',
    }));
  }

  async function clearSelectedAccountSession() {
    const targetRef = jobDetail?.job?.target_ref;
    if (!targetRef) {
      Toast.warning('当前任务没有 target_ref，无法定位本地账号会话');
      return;
    }
    await runJobAction('clear-session', async () => {
      const result = await window.desktopAutomation.clearAccountSession(targetRef);
      Toast.success(`已清理 ${result?.partition || targetRef}`);
    });
  }

  async function archiveSelectedAccount(pool) {
    const jobId = jobDetail?.job?.job_id;
    const locator = jobDetail?.locator;
    if (!jobId || !locator?.channel_id || locator.credential_index === undefined) {
      Toast.warning('当前任务缺少渠道账号定位，无法移入账号池');
      return;
    }
    const isDiscarded = pool === 'discarded';
    if (isDiscarded && !window.confirm('确认将这个账号移入废弃池？废弃池账号不会再参与调度。')) {
      return;
    }
    await runJobAction(`archive-${pool}`, () => window.desktopAutomation.archiveJobAccount(jobId, pool, {
      reason: isDiscarded ? 'desktop_operator_discarded' : 'desktop_operator_invalid',
      note: `desktop job ${jobId}`,
    }));
  }

  async function enqueueSelectedAccountAction(action) {
    const jobId = jobDetail?.job?.job_id;
    const targetRef = jobDetail?.job?.target_ref;
    if (!jobId || !targetRef) {
      Toast.warning('当前任务缺少 target_ref，无法投递账号诊断任务');
      return;
    }
    const label = action === 'profile-verify' ? '资料校验' : '账号探活';
    await runJobAction(action, async () => {
      const result = await window.desktopAutomation.enqueueAccountAction(jobId, action, {
        preferred_proxy_id: retryProxyId || undefined,
        clear_session: retryClearSession,
        reason: `desktop operator ${action}`,
      });
      Toast.success(`已投递${label}任务：${result?.job?.job_id || ''}`);
    });
  }

  async function reauthorizeInvalidPoolItem(item) {
    if (!item?.id) {
      return;
    }
    if (!window.confirm('确认恢复该账号并投递重新授权任务？')) {
      return;
    }
    setActionLoading(`reauthorize-${item.id}`);
    try {
      await window.desktopAutomation.reauthorizeInvalidAccount(item.id, {
        reason: 'desktop_pool_reauthorize',
      });
      Toast.success('已投递重新授权任务');
      await refreshData();
    } catch (error) {
      Toast.error(error.message);
    } finally {
      setActionLoading('');
    }
  }

  function pruneSelection(items) {
    const visible = new Set(items.map((item) => item.job?.job_id).filter(Boolean));
    setSelectedRowKeys((keys) => keys.filter((key) => visible.has(key)));
    setSelectedRows((rows) => rows.filter((row) => visible.has(row.job?.job_id)));
  }

  function openBulkModal(action) {
    if (selectedRows.length === 0) {
      Toast.warning('请先选择任务');
      return;
    }
    setBulkAction(action);
    setBulkProxyId('');
    setBulkClearSession(true);
    setBulkModal(true);
  }

  async function runBulkAction() {
    const action = bulkAction;
    const rows = selectedRows.slice();
    if (action === 'archive-discarded' && !window.confirm('确认批量移入废弃池？废弃池账号不会再参与调度。')) {
      return;
    }
    setActionLoading(`bulk-${action}`);
    let success = 0;
    const failures = [];
    try {
      for (const row of rows) {
        const job = row.job || {};
        try {
          if (action === 'retry') {
            if (!TERMINAL_STATUSES.includes(job.status)) {
              continue;
            }
            await window.desktopAutomation.retryJob(job.job_id, {
              preferred_proxy_id: bulkProxyId || undefined,
              clear_session: bulkClearSession,
              reason: 'desktop bulk retry',
            });
          } else if (action === 'cancel') {
            if (!ACTIVE_STATUSES.includes(job.status)) {
              continue;
            }
            await window.desktopAutomation.cancelJob(job.job_id, { reason: 'desktop bulk cancel' });
          } else if (action === 'resume') {
            if (job.status !== 'WAITING_HUMAN') {
              continue;
            }
            await window.desktopAutomation.resumeJob(job.job_id, { reason: 'desktop bulk resume' });
          } else if (action === 'clear-session') {
            if (!job.target_ref) {
              continue;
            }
            await window.desktopAutomation.clearAccountSession(job.target_ref);
          } else if (action === 'archive-invalid' || action === 'archive-discarded') {
            if (!row.locator?.channel_id || row.locator?.credential_index === undefined) {
              continue;
            }
            await window.desktopAutomation.archiveJobAccount(
              job.job_id,
              action === 'archive-discarded' ? 'discarded' : 'invalid',
              {
                reason: action === 'archive-discarded' ? 'desktop_bulk_discarded' : 'desktop_bulk_invalid',
                note: `desktop bulk ${action}`,
              },
            );
          }
          success += 1;
        } catch (error) {
          failures.push(`${job.job_id || '-'}: ${error.message}`);
        }
      }
      if (failures.length > 0) {
        Toast.warning(`完成 ${success} 个，失败 ${failures.length} 个`);
      } else {
        Toast.success(`完成 ${success} 个任务`);
      }
      setSelectedRowKeys([]);
      setSelectedRows([]);
      setBulkModal(false);
      await refreshData();
    } finally {
      setActionLoading('');
    }
  }

  const columns = useMemo(() => [
    {
      title: '任务',
      dataIndex: 'job',
      width: 210,
      render: (_, row) => (
        <div>
          <Text strong>{row.job?.job_id}</Text>
          <div className="muted mono">{row.job?.target_ref || '-'}</div>
        </div>
      ),
    },
    {
      title: '渠道账号',
      width: 120,
      render: (_, row) => (
        <div>
          <Text>#{row.locator?.channel_id || '-'}</Text>
          <div className="muted">index {row.locator?.credential_index ?? '-'}</div>
        </div>
      ),
    },
    {
      title: '状态',
      width: 130,
      render: (_, row) => <StatusTag status={row.job?.status} />,
    },
    {
      title: '阶段',
      width: 170,
      render: (_, row) => (
        <div>
          <Text>{row.latest_event?.stage || row.latest_event?.event_type || '-'}</Text>
          <div className="muted clamp">{row.latest_event?.message || row.job?.sanitized_error || ''}</div>
        </div>
      ),
    },
    {
      title: '尝试',
      width: 86,
      render: (_, row) => `${row.job?.attempt_count || 0}/${row.job?.max_attempts || 0}`,
    },
    {
      title: '更新时间',
      width: 150,
      render: (_, row) => formatTime(row.job?.updated_at),
    },
    {
      title: '操作',
      width: 96,
      fixed: 'right',
      render: (_, row) => (
        <Button size="small" onClick={() => openJobDetail(row.job?.job_id)}>详情</Button>
      ),
    },
  ], []);

  return (
    <Layout className="app-shell">
      <Header className="topbar ops-topbar">
        <div className="brand-block">
          <h1>授权恢复运营台</h1>
          <div className="brand-subtitle">
            <Tag color="cyan">desktop_session</Tag>
            <span>账号授权、失效池、代理资源统一处理</span>
          </div>
        </div>
        <div className="topbar-status">
          <OpsStatusCard
            label="执行器"
            value={executor.running ? '运行中' : '已暂停'}
            ok={executor.running}
            meta={`活跃 ${executor.active?.length || 0}`}
          />
          <OpsStatusCard
            label="主站回调"
            value={diagnostics?.gateway?.proxies?.ok ? '正常' : '异常'}
            ok={diagnostics?.gateway?.proxies?.ok}
            meta={diagHTTPStatus(diagnostics?.gateway?.proxies)}
          />
          <OpsStatusCard
            label="代理"
            value={`${proxyHealth.available || 0}/${proxyHealth.total || proxies.length}`}
            ok={(proxyHealth.available || 0) > 0}
            meta={`冷却 ${proxyHealth.cooling || 0}`}
          />
        </div>
        <Space>
          <Badge count={executor.active?.length || 0} type="primary">
            <Button icon={<IconRefresh />} loading={loading} onClick={refreshData}>刷新</Button>
          </Badge>
          {executor.running ? (
            <Button icon={<IconPause />} onClick={stopExecutor}>暂停执行</Button>
          ) : (
            <Button theme="solid" icon={<IconPlay />} onClick={startExecutor}>启动执行</Button>
          )}
        </Space>
      </Header>
      <Content className="content ops-content">
        <div className="metrics ops-metrics">
          <Metric label="待处理" value={overview?.stats?.status_counts?.PENDING || 0} tone="blue" />
          <Metric label="运行中" value={(overview?.stats?.status_counts?.LEASED || 0) + (overview?.stats?.status_counts?.RUNNING || 0)} tone="green" />
          <Metric label="待人工" value={overview?.stats?.status_counts?.WAITING_HUMAN || 0} tone="orange" />
          <Metric label="失效池" value={invalidPool.total || 0} tone="red" />
          <Metric label="代理可用" value={proxyHealth.available || 0} tone="cyan" />
          <Metric label="失败任务" value={overview?.stats?.status_counts?.FAILED || 0} tone="red" />
        </div>

        <Tabs
          className="workspace-tabs"
          type="button"
          activeKey={activeTab}
          onChange={setActiveTab}
          keepDOM={false}
        >
          <Tabs.TabPane tab={`任务队列 ${accounts.total || 0}`} itemKey="tasks">
            <Card className="panel workspace-card" bodyStyle={{ padding: 0 }}>
              <div className="account-toolbar task-toolbar">
                <div className="toolbar-title">
                  <Text strong>任务队列</Text>
                  <span className="muted">按任务状态和账号关键词快速定位，详情内处理重跑、清会话和归档。</span>
                </div>
                <Input
                  placeholder="搜索任务、账号、错误"
                  value={taskKeyword}
                  onChange={setTaskKeyword}
                  onEnterPress={refreshData}
                />
                <Select
                  placeholder="任务状态"
                  value={taskStatus}
                  onChange={setTaskStatus}
                  style={{ width: 154 }}
                >
                  <Select.Option value="ACTIVE">活跃任务</Select.Option>
                  <Select.Option value="">全部历史</Select.Option>
                  <Select.Option value="PENDING">待处理</Select.Option>
                  <Select.Option value="LEASED">已领取</Select.Option>
                  <Select.Option value="RUNNING">运行中</Select.Option>
                  <Select.Option value="WAITING_HUMAN">待人工</Select.Option>
                  <Select.Option value="SUCCESS">成功</Select.Option>
                  <Select.Option value="FAILED">失败</Select.Option>
                </Select>
                <Button icon={<IconRefresh />} loading={loading} onClick={refreshData}>刷新</Button>
              </div>
              <BulkToolbar
                selectedRows={selectedRows}
                actionLoading={actionLoading}
                onClear={() => {
                  setSelectedRowKeys([]);
                  setSelectedRows([]);
                }}
                onOpenBulk={openBulkModal}
              />
              <Table
                rowKey={(row) => row.job?.job_id}
                size="small"
                columns={columns}
                dataSource={accounts.items || []}
                rowSelection={{
                  selectedRowKeys,
                  onChange: (keys, rows) => {
                    setSelectedRowKeys(keys || []);
                    setSelectedRows(rows || []);
                  },
                }}
                pagination={false}
                loading={loading}
                scroll={{ y: 'calc(100vh - 310px)', x: 980 }}
              />
            </Card>
          </Tabs.TabPane>

          <Tabs.TabPane tab={`失效账号 ${invalidPool.total || 0}`} itemKey="invalid">
            <InvalidPoolPanel
              items={invalidPool.items || []}
              total={invalidPool.total || 0}
              message={invalidPool.message || ''}
              unavailable={invalidPool.unavailable}
              keyword={invalidKeyword}
              loading={loading}
              actionLoading={actionLoading}
              onKeywordChange={setInvalidKeyword}
              onRefresh={refreshData}
              onReauthorize={reauthorizeInvalidPoolItem}
            />
          </Tabs.TabPane>

          <Tabs.TabPane tab={`代理资源 ${proxyHealth.available || 0}/${proxyHealth.total || proxies.length}`} itemKey="proxies">
            <ProxyOperationsPanel
              proxies={proxies}
              sortedProxies={sortedProxies}
              proxyHealth={proxyHealth}
              proxySync={state?.store?.proxySync}
              syncError={proxySyncError}
              syncing={proxySyncing}
              onSync={syncProxies}
              onAdd={() => setProxyModal(true)}
              onToggle={async (proxy) => {
                const next = await window.desktopAutomation.toggleProxy(proxy.id, proxy.enabled === false);
                setState((prev) => ({ ...prev, store: next }));
              }}
              onDelete={async (proxy) => {
                const next = await window.desktopAutomation.deleteProxy(proxy.id);
                setState((prev) => ({ ...prev, store: next }));
              }}
            />
          </Tabs.TabPane>

          <Tabs.TabPane tab="自动化能力" itemKey="capabilities">
            <ActionTemplatesPanel
              templates={actionTemplates.items || []}
              statusCounts={actionTemplates.status_counts || {}}
            />
          </Tabs.TabPane>

          <Tabs.TabPane tab="配置与诊断" itemKey="settings">
            <div className="ops-grid">
              <SettingsPanel
                settingsFormKey={settingsFormKey}
                settings={settings}
                onSave={saveSettings}
              />
              <DiagnosticsPanel
                diagnostics={diagnostics}
                settings={settings}
                loading={diagnosticsLoading}
                onRefresh={refreshDiagnostics}
              />
              <EventsPanel events={events} />
            </div>
          </Tabs.TabPane>
        </Tabs>
      </Content>
      <ProxyModal
        visible={proxyModal}
        onCancel={() => setProxyModal(false)}
        onSave={async (values) => {
          const next = await window.desktopAutomation.upsertProxy(values);
          setState((prev) => ({ ...prev, store: next }));
          setProxyModal(false);
        }}
      />
      <JobDetailModal
        visible={detailVisible}
        detail={jobDetail}
        loading={detailLoading}
        actionLoading={actionLoading}
        proxies={sortedProxies}
        retryProxyId={retryProxyId}
        retryClearSession={retryClearSession}
        onRetryProxyChange={setRetryProxyId}
        onRetryClearSessionChange={setRetryClearSession}
        onCancel={() => setDetailVisible(false)}
        onRefresh={refreshJobDetail}
        onRetry={retrySelectedJob}
        onResume={resumeSelectedJob}
        onCancelJob={cancelSelectedJob}
        onClearSession={clearSelectedAccountSession}
        onProbe={() => enqueueSelectedAccountAction('probe')}
        onProfileVerify={() => enqueueSelectedAccountAction('profile-verify')}
        onArchiveInvalid={() => archiveSelectedAccount('invalid')}
        onArchiveDiscarded={() => archiveSelectedAccount('discarded')}
      />
      <BulkActionModal
        visible={bulkModal}
        action={bulkAction}
        actionLoading={actionLoading}
        selectedRows={selectedRows}
        proxies={sortedProxies}
        proxyId={bulkProxyId}
        clearSession={bulkClearSession}
        onProxyChange={setBulkProxyId}
        onClearSessionChange={setBulkClearSession}
        onCancel={() => setBulkModal(false)}
        onSubmit={runBulkAction}
      />
    </Layout>
  );
}

async function loadAccountItems(status, keyword) {
  if (status !== 'ACTIVE') {
    return window.desktopAutomation.listAccounts({
      page: 1,
      page_size: 50,
      status,
      keyword,
    }).catch(() => ({ items: [], total: 0 }));
  }
  const pages = await Promise.all(ACTIVE_STATUSES.map((activeStatus) => (
    window.desktopAutomation.listAccounts({
      page: 1,
      page_size: 50,
      status: activeStatus,
      keyword,
    }).catch(() => ({ items: [], total: 0 }))
  )));
  const items = pages
    .flatMap((page) => page.items || [])
    .sort((left, right) => Number(right.job?.updated_at || 0) - Number(left.job?.updated_at || 0))
    .slice(0, 50);
  const total = pages.reduce((sum, page) => sum + Number(page.total || 0), 0);
  return { items, total, page: 1, page_size: 50 };
}

async function loadInvalidPoolItems(keyword) {
  return window.desktopAutomation.listInvalidAccounts({
    page: 1,
    page_size: 20,
    keyword,
  }).catch(() => ({ items: [], total: 0, page: 1, page_size: 20 }));
}

function Metric({ label, value, tone = 'blue' }) {
  return (
    <div className={`metric metric-${tone}`}>
      <div className="metric-label">{label}</div>
      <div className="metric-value">{value}</div>
    </div>
  );
}

function OpsStatusCard({ label, value, meta, ok }) {
  return (
    <div className={`ops-status-card ${ok ? 'is-ok' : 'is-bad'}`}>
      <span>{label}</span>
      <strong>{value}</strong>
      <em>{meta || '-'}</em>
    </div>
  );
}

function SettingsPanel({ settingsFormKey, settings, onSave }) {
  return (
    <Card className="panel ops-panel" title="连接配置">
      <Form key={settingsFormKey} initValues={settings} onSubmit={onSave} labelPosition="top">
        <div className="settings-grid">
          <Form.Input field="automationBaseUrl" label="Automation URL" />
          <Form.Input field="desktopToken" label="Desktop Token" mode="password" />
          <Form.Input field="workerId" label="Worker ID" />
          <Form.InputNumber field="maxConcurrency" label="并发" min={1} max={8} />
          <Form.InputNumber field="callbackPort" label="回调端口" min={1024} max={65535} />
          <div className="settings-actions">
            <Button htmlType="submit" theme="solid" icon={<IconSave />}>保存配置</Button>
          </div>
        </div>
      </Form>
    </Card>
  );
}

function DiagnosticsPanel({ diagnostics, settings, loading, onRefresh }) {
  return (
    <Card
      className="panel ops-panel"
      title="本地环境"
      headerExtraContent={<Button icon={<IconRefresh />} loading={loading} onClick={onRefresh}>刷新诊断</Button>}
    >
      <div className="diagnostics diagnostics-wide">
        <DiagRow label="自动化服务" value={diagnostics?.health?.ok ? '正常' : diagnostics?.health?.message || '未检测'} ok={diagnostics?.health?.ok} />
        <DiagRow label="桌面接口" value={diagnostics?.overview?.ok ? '正常' : diagnostics?.overview?.message || '未检测'} ok={diagnostics?.overview?.ok} />
        <DiagRow label="失效池转发" value={diagHTTPStatus(diagnostics?.desktopInvalidPool)} ok={diagnostics?.desktopInvalidPool?.ok} />
        <DiagRow label="主站代理" value={diagHTTPStatus(diagnostics?.gateway?.proxies)} ok={diagnostics?.gateway?.proxies?.ok} />
        <DiagRow label="主站失效池" value={diagHTTPStatus(diagnostics?.gateway?.invalidPool)} ok={diagnostics?.gateway?.invalidPool?.ok} />
        <DiagRow label="回调端口" value={diagPortStatus(diagnostics?.callbackPortStatus, diagnostics?.callbackPort || settings.callbackPort)} ok={diagnostics?.callbackPortStatus?.ok} />
        <DiagRow label="环境文件" value={diagnostics?.envFile || '-'} ok={diagnostics?.envFileExists} />
        <DiagRow label="网关回调" value={diagnostics?.gatewayCallbackUrl || '-'} />
        <DiagRow label="回调 Token" value={diagnostics?.gatewayCallbackTokenConfigured ? '已配置' : '未配置'} ok={diagnostics?.gatewayCallbackTokenConfigured} />
        <DiagRow label="系统" value={[diagnostics?.platform, diagnostics?.arch].filter(Boolean).join(' / ') || '-'} />
        <DiagRow label="Electron" value={diagnostics?.electronVersion || '-'} />
      </div>
    </Card>
  );
}

function ProxyOperationsPanel({
  proxies,
  sortedProxies,
  proxyHealth,
  proxySync,
  syncError,
  syncing,
  onSync,
  onAdd,
  onToggle,
  onDelete,
}) {
  return (
    <Card
      className="panel workspace-card proxy-workspace"
      title="代理资源"
      headerExtraContent={(
        <Space spacing={8}>
          <Button icon={<IconRefresh />} loading={syncing} onClick={onSync}>同步主站代理</Button>
          <Button theme="solid" icon={<IconPlus />} onClick={onAdd}>添加本地代理</Button>
        </Space>
      )}
    >
      {syncError ? <div className="proxy-sync-alert">{syncError}</div> : null}
      <div className="proxy-summary">
        <Metric label="代理总数" value={proxyHealth.total || proxies.length} tone="blue" />
        <Metric label="可用" value={proxyHealth.available || 0} tone="green" />
        <Metric label="冷却" value={proxyHealth.cooling || 0} tone="orange" />
        <Metric label="停用" value={proxyHealth.disabled || 0} tone="red" />
        <div className="proxy-sync-card">
          <span>最近同步</span>
          <strong>{formatTime(proxySync?.lastSyncedAt)}</strong>
          <em>导入 {proxySync?.imported || 0} · 更新 {proxySync?.updated || 0}</em>
        </div>
      </div>
      <div className="proxy-list proxy-list-wide">
        {sortedProxies.map((proxy) => (
          <div className="proxy-item" key={proxy.id}>
            <div>
              <div className="proxy-title">
                <Text strong>{proxy.name || proxy.id}</Text>
                <ProxyStatusTag proxy={proxy} />
                <Tag color={proxy.enabled === false ? 'grey' : proxy.source === 'main_gateway' ? 'cyan' : 'blue'}>
                  {proxy.enabled === false ? '停用' : proxy.source === 'main_gateway' ? '主站' : '本地'}
                </Tag>
              </div>
              <div className="muted mono clamp">{proxy.maskedAddress || proxy.proxyRules}</div>
              <div className="proxy-health-line">
                <span>健康 {proxy.healthScore ?? 0}</span>
                <span className="muted clamp">{proxy.healthReason || '-'}</span>
              </div>
              {proxy.region || proxy.exitIp ? (
                <div className="muted clamp">{[proxy.region, proxy.exitIp].filter(Boolean).join(' · ')}</div>
              ) : null}
              {proxy.lastError ? <div className="danger clamp">{proxy.lastError}</div> : null}
              {proxy.cooldownUntil ? <div className="warning clamp">冷却至 {formatTime(proxy.cooldownUntil)}</div> : null}
            </div>
            <div className="proxy-actions">
              <Button size="small" onClick={() => onToggle(proxy)}>
                {proxy.enabled === false ? '启用' : '停用'}
              </Button>
              <Button
                type="danger"
                size="small"
                icon={<IconDelete />}
                onClick={() => onDelete(proxy)}
              />
            </div>
          </div>
        ))}
        {proxies.length === 0 ? (
          <div className="proxy-empty-state">
            <Text strong>暂无代理资源</Text>
            <span>可以先同步主站代理；如果提示 404，说明主站容器还没发布或重启到当前代码。</span>
            <Space>
              <Button icon={<IconRefresh />} loading={syncing} onClick={onSync}>重新同步</Button>
              <Button theme="solid" icon={<IconPlus />} onClick={onAdd}>添加本地代理</Button>
            </Space>
          </div>
        ) : null}
      </div>
    </Card>
  );
}

function EventsPanel({ events }) {
  return (
    <Card className="panel ops-panel event-panel-wide" title="事件">
      {events.map((item) => (
        <div className="event" key={`${item.at}-${item.event}`}>
          <Text>{item.event}</Text>
          <div className="muted">{formatTime(Math.floor(item.at / 1000))}</div>
          <div className="mono clamp">{JSON.stringify(item.payload || {})}</div>
        </div>
      ))}
      {events.length === 0 ? <div className="empty-proxy">暂无本地事件</div> : null}
    </Card>
  );
}

function ActionTemplatesPanel({ templates, statusCounts }) {
  const categoryOrder = ['授权', '运维', '账号池', '诊断', '安全'];
  const groups = categoryOrder
    .map((category) => ({
      category,
      items: templates.filter((item) => item.category === category),
    }))
    .filter((group) => group.items.length > 0);
  const otherItems = templates.filter((item) => !categoryOrder.includes(item.category));
  if (otherItems.length > 0) {
    groups.push({ category: '其他', items: otherItems });
  }
  return (
    <Card className="panel workspace-card action-panel" title="自动化能力">
      <div className="action-summary">
        <div>
          <strong>{statusCounts.ready || 0}</strong>
          <span>已接入</span>
        </div>
        <div>
          <strong>{statusCounts.partial || 0}</strong>
          <span>部分</span>
        </div>
        <div>
          <strong>{statusCounts.planned || 0}</strong>
          <span>规划</span>
        </div>
      </div>
      <div className="action-template-list">
        {groups.map((group) => (
          <div className="action-group" key={group.category}>
            <div className="action-group-title">{group.category}</div>
            {group.items.map((item) => (
              <div className="action-template-item" key={item.key}>
                <div className="action-template-head">
                  <Text strong>{item.title}</Text>
                  <ActionTemplateStatus item={item} />
                </div>
                <div className="muted clamp">{item.product_value || item.description || '-'}</div>
                <div className="action-template-tags">
                  {item.task_type ? <Tag color="blue">{item.task_type}</Tag> : null}
                  {item.executor_type ? <Tag color="cyan">{item.executor_type}</Tag> : null}
                  {!item.task_type && item.operation_type ? <Tag color="blue">{item.operation_type}</Tag> : null}
                  {item.requires_locator ? <Tag color="orange">需定位</Tag> : null}
                  {item.requires_proxy ? <Tag color="cyan">需代理</Tag> : null}
                  {item.danger ? <Tag color="red">谨慎</Tag> : null}
                </div>
                <div className="action-template-foot">
                  <span className="mono clamp">{item.entry_point || '-'}</span>
                  <span className={item.enabled ? 'ok' : item.implemented ? 'warning' : 'muted'}>
                    {item.tech_status || '-'}
                  </span>
                </div>
              </div>
            ))}
          </div>
        ))}
        {templates.length === 0 ? <div className="empty-proxy">暂无能力模板</div> : null}
      </div>
    </Card>
  );
}

function ActionTemplateStatus({ item }) {
  let color = 'grey';
  let label = '规划中';
  if (item.status === 'partial') {
    color = 'orange';
    label = '部分接入';
  } else if (item.implemented && item.enabled) {
    color = 'green';
    label = '可用';
  } else if (item.implemented && !item.enabled) {
    color = 'orange';
    label = '待配置';
  }
  return <Tag color={color}>{label}</Tag>;
}

function InvalidPoolPanel({
  items,
  total,
  message,
  unavailable,
  keyword,
  loading,
  actionLoading,
  onKeywordChange,
  onRefresh,
  onReauthorize,
}) {
  const columns = [
    {
      title: '失效账号',
      width: 220,
      render: (_, row) => (
        <div>
          <Text strong>{row.account_id || row.credential_masked || `#${row.id}`}</Text>
          <div className="muted mono clamp">{row.account_identity_key || row.credential_short || '-'}</div>
        </div>
      ),
    },
    {
      title: '来源渠道',
      width: 170,
      render: (_, row) => (
        <div>
          <Text>#{row.channel_id} {row.channel_name || ''}</Text>
          <div className="muted">原 index {row.credential_index ?? '-'}</div>
        </div>
      ),
    },
    {
      title: '类型',
      width: 150,
      render: (_, row) => (
        <Space spacing={4}>
          {row.provider ? <Tag color="cyan">{row.provider}</Tag> : null}
          {row.account_type ? <Tag color="blue">{row.account_type}</Tag> : null}
        </Space>
      ),
    },
    {
      title: '原因',
      render: (_, row) => (
        <div>
          <Text>{row.reason || '-'}</Text>
          <div className="muted clamp">{row.note || ''}</div>
        </div>
      ),
    },
    {
      title: '归档时间',
      width: 160,
      render: (_, row) => formatTime(row.archived_at),
    },
    {
      title: '操作',
      width: 110,
      fixed: 'right',
      render: (_, row) => (
        <Button
          size="small"
          theme="solid"
          loading={actionLoading === `reauthorize-${row.id}`}
          onClick={() => onReauthorize(row)}
        >
          重新授权
        </Button>
      ),
    },
  ];
  return (
    <Card className="panel invalid-pool-panel" bodyStyle={{ padding: 0 }}>
      <div className="account-toolbar invalid-toolbar">
        <div>
          <Text strong>失效账号池</Text>
          <span className="muted"> 共 {total} 个，可恢复后重新授权</span>
        </div>
        <Input
          placeholder="搜索账号、渠道、原因"
          value={keyword}
          onChange={onKeywordChange}
          onEnterPress={onRefresh}
        />
        <Button icon={<IconRefresh />} loading={loading} onClick={onRefresh}>刷新</Button>
      </div>
      {unavailable || message ? (
        <div className={unavailable ? 'pool-warning' : 'sync-meta'}>{message}</div>
      ) : null}
      <Table
        rowKey={(row) => row.id}
        size="small"
        columns={columns}
        dataSource={items}
        pagination={false}
        loading={loading}
        scroll={{ y: 'calc(100vh - 310px)', x: 980 }}
      />
    </Card>
  );
}

function BulkToolbar({ selectedRows, actionLoading, onClear, onOpenBulk }) {
  const selectedCount = selectedRows.length;
  const terminalCount = selectedRows.filter((row) => TERMINAL_STATUSES.includes(row.job?.status)).length;
  const activeCount = selectedRows.filter((row) => ACTIVE_STATUSES.includes(row.job?.status)).length;
  const waitingCount = selectedRows.filter((row) => row.job?.status === 'WAITING_HUMAN').length;
  const clearableCount = selectedRows.filter((row) => row.job?.target_ref).length;
  const locatableCount = selectedRows.filter((row) => row.locator?.channel_id && row.locator?.credential_index !== undefined).length;
  if (selectedCount === 0) {
    return null;
  }
  return (
    <div className="bulk-toolbar">
      <div>
        <Text strong>已选择 {selectedCount} 个任务</Text>
        <span className="muted"> 活跃 {activeCount} · 终态 {terminalCount} · 待人工 {waitingCount}</span>
      </div>
      <Space spacing={6}>
        <Button size="small" disabled={terminalCount === 0} loading={actionLoading === 'bulk-retry'} onClick={() => onOpenBulk('retry')}>
          批量重跑
        </Button>
        <Button size="small" disabled={waitingCount === 0} loading={actionLoading === 'bulk-resume'} onClick={() => onOpenBulk('resume')}>
          批量恢复
        </Button>
        <Button size="small" disabled={clearableCount === 0} loading={actionLoading === 'bulk-clear-session'} onClick={() => onOpenBulk('clear-session')}>
          清理会话
        </Button>
        <Button size="small" disabled={locatableCount === 0} loading={actionLoading === 'bulk-archive-invalid'} onClick={() => onOpenBulk('archive-invalid')}>
          移入失效池
        </Button>
        <Button size="small" type="danger" disabled={locatableCount === 0} loading={actionLoading === 'bulk-archive-discarded'} onClick={() => onOpenBulk('archive-discarded')}>
          移入废弃池
        </Button>
        <Button size="small" type="danger" disabled={activeCount === 0} loading={actionLoading === 'bulk-cancel'} onClick={() => onOpenBulk('cancel')}>
          批量取消
        </Button>
        <Button size="small" onClick={onClear}>取消选择</Button>
      </Space>
    </div>
  );
}

function BulkActionModal({
  visible,
  action,
  actionLoading,
  selectedRows,
  proxies,
  proxyId,
  clearSession,
  onProxyChange,
  onClearSessionChange,
  onCancel,
  onSubmit,
}) {
  const enabledProxies = proxies.filter((proxy) => proxy.enabled !== false);
  const title = {
    retry: '批量重跑',
    resume: '批量恢复',
    cancel: '批量取消',
    'clear-session': '批量清理会话',
    'archive-invalid': '批量移入失效池',
    'archive-discarded': '批量移入废弃池',
  }[action] || '批量操作';
  const affected = selectedRows.filter((row) => {
    const status = row.job?.status;
    if (action === 'retry') {
      return TERMINAL_STATUSES.includes(status);
    }
    if (action === 'resume') {
      return status === 'WAITING_HUMAN';
    }
    if (action === 'cancel') {
      return ACTIVE_STATUSES.includes(status);
    }
    if (action === 'clear-session') {
      return Boolean(row.job?.target_ref);
    }
    if (action === 'archive-invalid' || action === 'archive-discarded') {
      return Boolean(row.locator?.channel_id && row.locator?.credential_index !== undefined);
    }
    return false;
  });
  return (
    <Modal
      title={title}
      visible={visible}
      onCancel={onCancel}
      onOk={onSubmit}
      okText="执行"
      confirmLoading={actionLoading === `bulk-${action}`}
    >
      <div className="bulk-modal">
        <div className="bulk-summary">
          已选择 {selectedRows.length} 个任务，本次可处理 {affected.length} 个。
        </div>
        {action === 'retry' ? (
          <>
            <Select
              value={proxyId}
              onChange={onProxyChange}
              placeholder="自动选择代理"
              style={{ width: '100%' }}
              showClear
            >
              {enabledProxies.map((proxy) => (
                <Select.Option value={proxy.id} key={proxy.id}>
                  <ProxySelectOption proxy={proxy} />
                </Select.Option>
              ))}
            </Select>
            <Checkbox checked={clearSession} onChange={(event) => onClearSessionChange(event.target.checked)}>
              重跑前清理本地浏览器会话
            </Checkbox>
          </>
        ) : null}
        {action === 'archive-invalid' || action === 'archive-discarded' ? (
          <div className="warning">
            {action === 'archive-discarded'
              ? '废弃池账号不会再参与调度，请确认这些任务定位到的是不可恢复账号。'
              : '失效池用于可修复账号，归档后原渠道账号会被移除。'}
          </div>
        ) : null}
        <div className="bulk-preview">
          {affected.slice(0, 8).map((row) => (
            <div className="bulk-preview-row" key={row.job?.job_id}>
              <span className="mono clamp">{row.job?.job_id}</span>
              <StatusTag status={row.job?.status} />
            </div>
          ))}
          {affected.length > 8 ? <div className="muted">还有 {affected.length - 8} 个任务...</div> : null}
        </div>
      </div>
    </Modal>
  );
}

function DiagRow({ label, value, ok }) {
  return (
    <div className="diag-row">
      <span>{label}</span>
      <span className={ok === undefined ? 'muted clamp' : ok ? 'ok clamp' : 'danger clamp'}>{value}</span>
    </div>
  );
}

function diagHTTPStatus(result) {
  if (!result) {
    return '未检测';
  }
  if (result.ok) {
    return result.status ? `正常 ${result.status}` : '正常';
  }
  const message = result.message || '异常';
  return result.status ? `${result.status} ${message}` : message;
}

function diagPortStatus(result, port) {
  if (!result) {
    return port ? `${port} 未检测` : '未检测';
  }
  const prefix = port ? `${port} ` : '';
  if (result.status === 'listening') {
    return `${prefix}客户端监听中`;
  }
  if (result.status === 'available') {
    return `${prefix}可用`;
  }
  if (result.status === 'busy') {
    return `${prefix}被占用`;
  }
  return `${prefix}${result.message || result.status || '-'}`;
}

function explainProxySyncError(message = '') {
  const text = String(message || '').trim();
  if (text.includes('status=404') || text.includes('api route not found')) {
    return '主站代理同步接口不存在：请发布或重启主站到包含 token-account-automation callback 路由的版本。';
  }
  if (text.includes('401') || text.includes('invalid token')) {
    return '主站代理同步鉴权失败：请检查主站和客户端的 callback token 是否一致。';
  }
  if (text.includes('gateway callback is not configured')) {
    return '主站回调地址或 token 未配置，请在 .env.dev/.env.pro 中配置 AUTOMATION_GATEWAY_CALLBACK_URL 和 AUTOMATION_GATEWAY_CALLBACK_TOKEN。';
  }
  return text || '主站代理同步失败';
}

function StatusTag({ status }) {
  const color = {
    PENDING: 'blue',
    LEASED: 'cyan',
    RUNNING: 'green',
    WAITING_HUMAN: 'orange',
    SUCCESS: 'green',
    FAILED: 'red',
    CANCELED: 'grey',
    EXPIRED: 'grey',
  }[status] || 'grey';
  return <Tag color={color}>{status || '-'}</Tag>;
}

function ProxyStatusTag({ proxy }) {
  const status = proxy?.proxyStatus || (proxy?.enabled === false ? 'disabled' : 'available');
  const color = {
    available: 'green',
    cooling: 'orange',
    disabled: 'grey',
  }[status] || 'grey';
  const label = {
    available: '可用',
    cooling: '冷却',
    disabled: '停用',
  }[status] || status;
  return <Tag color={color}>{label}</Tag>;
}

function ProxySelectOption({ proxy }) {
  return (
    <div className="proxy-select-option">
      <span>{proxy.name || proxy.id}</span>
      <span className="muted">分 {proxy.healthScore ?? 0} · {proxy.healthReason || '-'}</span>
    </div>
  );
}

function sortProxiesByHealth(proxies = []) {
  return proxies.slice().sort((left, right) => {
    const leftStatus = left.proxyStatus || (left.enabled === false ? 'disabled' : 'available');
    const rightStatus = right.proxyStatus || (right.enabled === false ? 'disabled' : 'available');
    const order = { available: 0, cooling: 1, disabled: 2 };
    if ((order[leftStatus] ?? 9) !== (order[rightStatus] ?? 9)) {
      return (order[leftStatus] ?? 9) - (order[rightStatus] ?? 9);
    }
    if (Number(right.healthScore || 0) !== Number(left.healthScore || 0)) {
      return Number(right.healthScore || 0) - Number(left.healthScore || 0);
    }
    return String(left.name || left.id).localeCompare(String(right.name || right.id));
  });
}

function ProxyModal({ visible, onCancel, onSave }) {
  return (
    <Modal title="添加本地代理" visible={visible} onCancel={onCancel} footer={null}>
      <Form onSubmit={onSave} labelPosition="top">
        <Form.Input field="name" label="名称" />
        <Form.Input
          field="proxyRules"
          label="代理规则"
          placeholder="socks5://127.0.0.1:1080"
          rules={[{ required: true, message: '请输入代理规则' }]}
        />
        <Space>
          <Button onClick={onCancel}>取消</Button>
          <Button htmlType="submit" theme="solid">保存</Button>
        </Space>
      </Form>
    </Modal>
  );
}

function JobDetailModal({
  visible,
  detail,
  loading,
  actionLoading,
  proxies,
  retryProxyId,
  retryClearSession,
  onRetryProxyChange,
  onRetryClearSessionChange,
  onCancel,
  onRefresh,
  onRetry,
  onResume,
  onCancelJob,
  onClearSession,
  onProbe,
  onProfileVerify,
  onArchiveInvalid,
  onArchiveDiscarded,
}) {
  const job = detail?.job || {};
  const input = parseJSON(job.input_json);
  const result = parseJSON(job.result_json);
  const isTerminal = ['SUCCESS', 'FAILED', 'CANCELED', 'EXPIRED'].includes(job.status);
  const isActive = ['PENDING', 'LEASED', 'RUNNING', 'WAITING_HUMAN'].includes(job.status);
  const isWaitingHuman = job.status === 'WAITING_HUMAN';
  const enabledProxies = proxies.filter((proxy) => proxy.enabled !== false);
  const recommendations = accountRecommendations(detail);
  const hasLocator = Boolean(detail?.locator?.channel_id && detail?.locator?.credential_index !== undefined);

  return (
    <Modal
      title="任务详情"
      visible={visible}
      onCancel={onCancel}
      footer={null}
      width={1080}
      bodyStyle={{ padding: 0 }}
    >
      <div className="job-detail">
        {loading ? (
          <div className="detail-loading">加载中...</div>
        ) : (
          <>
            <div className="detail-hero">
              <div>
                <div className="detail-title">
                  <Text strong>{job.job_id || '-'}</Text>
                  <StatusTag status={job.status} />
                </div>
                <div className="muted mono clamp">{job.target_ref || '-'}</div>
              </div>
              <Space wrap>
                <Button icon={<IconRefresh />} loading={loading} onClick={onRefresh}>刷新</Button>
                <Button icon={<IconRestoreStroked />} loading={actionLoading === 'clear-session'} onClick={onClearSession}>
                  清理会话
                </Button>
                <Button disabled={!job.target_ref} loading={actionLoading === 'probe'} onClick={onProbe}>
                  探活
                </Button>
                <Button disabled={!job.target_ref} loading={actionLoading === 'profile-verify'} onClick={onProfileVerify}>
                  资料校验
                </Button>
                <Button disabled={!hasLocator} loading={actionLoading === 'archive-invalid'} onClick={onArchiveInvalid}>
                  移入失效池
                </Button>
                <Button type="danger" disabled={!hasLocator} loading={actionLoading === 'archive-discarded'} onClick={onArchiveDiscarded}>
                  移入废弃池
                </Button>
                <Button disabled={!isWaitingHuman} loading={actionLoading === 'resume'} onClick={onResume}>
                  恢复
                </Button>
                <Button type="danger" disabled={!isActive} loading={actionLoading === 'cancel'} onClick={onCancelJob}>
                  取消
                </Button>
              </Space>
            </div>

            <div className="detail-grid">
              <InfoCard title="任务信息">
                <InfoRow label="类型" value={job.task_type || '-'} />
                <InfoRow label="执行器" value={job.executor_type || '-'} />
                <InfoRow label="尝试" value={`${job.attempt_count || 0}/${job.max_attempts || 0}`} />
                <InfoRow label="优先级" value={job.priority ?? '-'} />
                <InfoRow label="创建" value={formatTime(job.created_at)} />
                <InfoRow label="更新" value={formatTime(job.updated_at)} />
                <InfoRow label="开始" value={formatTime(job.started_at)} />
                <InfoRow label="完成" value={formatTime(job.finished_at)} />
              </InfoCard>

              <InfoCard title="账号定位">
                <InfoRow label="渠道" value={detail?.locator?.channel_id ? `#${detail.locator.channel_id}` : '-'} />
                <InfoRow label="索引" value={detail?.locator?.credential_index ?? '-'} />
                <InfoRow label="Provider" value={detail?.target?.provider || '-'} />
                <InfoRow label="账号" value={detail?.target?.display_name || detail?.target?.subject_key || '-'} />
                <InfoRow label="状态" value={detail?.target?.status || '-'} />
                <InfoRow label="绑定" value={detail?.locator?.external_ref || '-'} mono />
              </InfoCard>

              <InfoCard title="重跑策略">
                <div className="retry-controls">
                  <Select
                    value={retryProxyId}
                    onChange={onRetryProxyChange}
                    placeholder="自动选择代理"
                    style={{ width: '100%' }}
                    showClear
                  >
	                    {enabledProxies.map((proxy) => (
	                      <Select.Option value={proxy.id} key={proxy.id}>
	                        <ProxySelectOption proxy={proxy} />
	                      </Select.Option>
	                    ))}
                  </Select>
                  <Checkbox checked={retryClearSession} onChange={(event) => onRetryClearSessionChange(event.target.checked)}>
                    重跑前清理本地浏览器会话
                  </Checkbox>
                  <Button theme="solid" disabled={!isTerminal} loading={actionLoading === 'retry'} onClick={onRetry}>
                    指定代理重跑
                  </Button>
                  {!isTerminal ? <div className="muted">只有成功、失败、取消或过期任务可以直接重跑。</div> : null}
                </div>
              </InfoCard>
            </div>

            <div className="detail-section">
              <div className="section-title">账号画像与处置建议</div>
              <div className="profile-grid">
                <InfoRow label="身份" value={accountIdentity(detail)} mono />
                <InfoRow label="最近错误" value={latestError(detail)} />
                <InfoRow label="最近事件" value={detail?.events?.[0]?.event_type || '-'} />
                <InfoRow label="建议动作" value={recommendations.primary} />
              </div>
              <div className="recommendations">
                {recommendations.items.map((item) => (
                  <Tag color={item.color} key={item.text}>{item.text}</Tag>
                ))}
              </div>
            </div>

            {job.sanitized_error ? (
              <div className="detail-error">{job.error_code ? `${job.error_code}: ` : ''}{job.sanitized_error}</div>
            ) : null}

            <div className="detail-section">
              <div className="section-title">尝试记录</div>
              <Table
                rowKey={(row) => row.id}
                size="small"
                pagination={false}
                dataSource={detail?.attempts || []}
                columns={[
                  { title: '#', dataIndex: 'attempt_no', width: 60 },
                  { title: 'Worker', dataIndex: 'worker_id', width: 180 },
                  { title: '阶段', dataIndex: 'stage', width: 150 },
                  { title: '状态', dataIndex: 'status', width: 120, render: (status) => <StatusTag status={status} /> },
                  { title: '开始', dataIndex: 'started_at', width: 160, render: formatTime },
                  { title: '结束', dataIndex: 'finished_at', width: 160, render: formatTime },
                  { title: '错误', dataIndex: 'sanitized_error', render: (value) => <span className="danger clamp">{value || '-'}</span> },
                ]}
              />
            </div>

            <div className="detail-section">
              <div className="section-title">事件时间线</div>
              <div className="timeline">
                {(detail?.events || []).map((event) => (
                  <div className="timeline-item" key={event.id}>
                    <div className="timeline-dot" />
                    <div className="timeline-body">
                      <div className="timeline-head">
                        <Text strong>{event.event_type}</Text>
                        <StatusTag status={event.status} />
                        <span className="muted">{formatTime(event.created_at)}</span>
                      </div>
                      <div className="muted">{event.stage || '-'}</div>
                      {event.message ? <div>{event.message}</div> : null}
                      {event.data_json ? <pre>{prettyJSON(event.data_json)}</pre> : null}
                    </div>
                  </div>
                ))}
                {(detail?.events || []).length === 0 ? <div className="empty-proxy">暂无事件</div> : null}
              </div>
            </div>

            <div className="detail-grid two">
              <InfoCard title="输入">
                <pre>{prettyJSON(input)}</pre>
              </InfoCard>
              <InfoCard title="结果">
                <pre>{prettyJSON(result)}</pre>
              </InfoCard>
            </div>
          </>
        )}
      </div>
    </Modal>
  );
}

function InfoCard({ title, children }) {
  return (
    <div className="info-card">
      <div className="section-title">{title}</div>
      {children}
    </div>
  );
}

function InfoRow({ label, value, mono }) {
  return (
    <div className="info-row">
      <span>{label}</span>
      <span className={mono ? 'mono clamp' : 'clamp'}>{value}</span>
    </div>
  );
}

function accountIdentity(detail) {
  const target = detail?.target || {};
  const locator = detail?.locator || {};
  const parts = [
    locator.channel_id ? `channel#${locator.channel_id}` : '',
    locator.credential_index !== undefined ? `index#${locator.credential_index}` : '',
    target.provider,
    target.display_name || target.subject_key,
  ].filter(Boolean);
  return parts.join(' / ') || detail?.job?.target_ref || '-';
}

function latestError(detail) {
  const job = detail?.job || {};
  if (job.sanitized_error) {
    return job.error_code ? `${job.error_code}: ${job.sanitized_error}` : job.sanitized_error;
  }
  const failed = (detail?.events || []).find((event) => event.event_type === 'failed' || event.status === 'FAILED');
  return failed?.message || '-';
}

function accountRecommendations(detail) {
  const job = detail?.job || {};
  const error = latestError(detail).toLowerCase();
  const items = [];
  let primary = '观察任务状态';
  if (job.status === 'WAITING_HUMAN') {
    primary = '人工完成登录后恢复任务';
    items.push({ text: '人工接管', color: 'orange' }, { text: '完成后恢复', color: 'cyan' });
  } else if (TERMINAL_STATUSES.includes(job.status)) {
    primary = '清理会话后换代理重跑';
    items.push({ text: '可重跑', color: 'blue' }, { text: '建议清会话', color: 'cyan' });
  } else if (ACTIVE_STATUSES.includes(job.status)) {
    primary = '等待执行器处理';
    items.push({ text: '活跃队列', color: 'green' });
  }
  if (error.includes('proxy') || error.includes('network') || error.includes('timeout')) {
    primary = '换代理并清理会话后重跑';
    items.push({ text: '代理/网络异常', color: 'red' });
  }
  if (error.includes('captcha') || error.includes('risk') || error.includes('verify')) {
    primary = '需要人工验证或人工登录';
    items.push({ text: '风控/验证', color: 'orange' });
  }
  if (error.includes('invalid') || error.includes('revoked') || error.includes('unauthorized')) {
    primary = '授权失效，重新登录后写回凭据';
    items.push({ text: '授权失效', color: 'red' });
  }
  if (items.length === 0) {
    items.push({ text: '暂无异常标签', color: 'grey' });
  }
  return { primary, items };
}

function formatTime(value) {
  if (!value) {
    return '-';
  }
  return new Date(value * 1000).toLocaleString();
}

function parseJSON(value) {
  if (!value) {
    return {};
  }
  if (typeof value === 'object') {
    return value;
  }
  try {
    return JSON.parse(value);
  } catch {
    return {};
  }
}

function prettyJSON(value) {
  const parsed = typeof value === 'string' ? parseJSON(value) : value;
  if (!parsed || (typeof parsed === 'object' && Object.keys(parsed).length === 0)) {
    return '-';
  }
  return JSON.stringify(parsed, null, 2);
}
