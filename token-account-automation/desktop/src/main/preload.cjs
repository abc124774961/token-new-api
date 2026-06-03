const { contextBridge, ipcRenderer } = require('electron');

contextBridge.exposeInMainWorld('desktopAutomation', {
  getState: () => ipcRenderer.invoke('app:get-state'),
  updateSettings: (settings) => ipcRenderer.invoke('settings:update', settings),
  listAccounts: (params) => ipcRenderer.invoke('automation:accounts', params),
  listInvalidAccounts: (params) => ipcRenderer.invoke('automation:invalid-accounts', params),
  jobDetail: (jobId) => ipcRenderer.invoke('automation:job-detail', jobId),
  retryJob: (jobId, payload) => ipcRenderer.invoke('automation:retry-job', jobId, payload),
  resumeJob: (jobId, payload) => ipcRenderer.invoke('automation:resume-job', jobId, payload),
  cancelJob: (jobId, payload) => ipcRenderer.invoke('automation:cancel-job', jobId, payload),
  archiveJobAccount: (jobId, pool, payload) => ipcRenderer.invoke('automation:archive-job-account', jobId, pool, payload),
  enqueueAccountAction: (jobId, action, payload) => ipcRenderer.invoke('automation:enqueue-account-action', jobId, action, payload),
  reauthorizeInvalidAccount: (poolId, payload) => ipcRenderer.invoke('automation:reauthorize-invalid-account', poolId, payload),
  overview: () => ipcRenderer.invoke('automation:overview'),
  actionTemplates: () => ipcRenderer.invoke('automation:action-templates'),
  syncProxies: (params) => ipcRenderer.invoke('automation:sync-proxies', params),
  getDiagnostics: () => ipcRenderer.invoke('diagnostics:get'),
  startExecutor: () => ipcRenderer.invoke('executor:start'),
  stopExecutor: () => ipcRenderer.invoke('executor:stop'),
  upsertProxy: (proxy) => ipcRenderer.invoke('proxy:upsert', proxy),
  toggleProxy: (id, enabled) => ipcRenderer.invoke('proxy:toggle', id, enabled),
  deleteProxy: (id) => ipcRenderer.invoke('proxy:delete', id),
  clearSession: (jobId) => ipcRenderer.invoke('browser:clear-session', jobId),
  clearAccountSession: (targetRef) => ipcRenderer.invoke('browser:clear-account-session', targetRef),
  onEvent: (handler) => {
    const listener = (_, event, payload) => handler(event, payload);
    ipcRenderer.on('main:event', listener);
    return () => ipcRenderer.removeListener('main:event', listener);
  },
});
