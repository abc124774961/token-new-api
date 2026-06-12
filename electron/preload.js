const { contextBridge, ipcRenderer } = require('electron');

// 获取数据目录路径（用于显示给用户）
// 优先使用主进程设置的真实路径，如果没有则回退到手动拼接
function getDataDirPath() {
  // 如果主进程已设置真实路径，直接使用
  if (process.env.ELECTRON_DATA_DIR) {
    return process.env.ELECTRON_DATA_DIR;
  }
}

contextBridge.exposeInMainWorld('electron', {
  isElectron: true,
  version: process.versions.electron,
  platform: process.platform,
  versions: process.versions,
  dataDir: getDataDirPath()
});

contextBridge.exposeInMainWorld('electronBrowser', {
  listAccounts: (filters) => ipcRenderer.invoke('browser-workspace:list-accounts', filters || {}),
  openAccount: (payload) => ipcRenderer.invoke('browser-workspace:open-account', payload || {}),
  activateTab: (tabId) => ipcRenderer.invoke('browser-workspace:activate-tab', tabId),
  closeTab: (tabId) => ipcRenderer.invoke('browser-workspace:close-tab', tabId),
  navigate: (payload) => ipcRenderer.invoke('browser-workspace:navigate', payload || {}),
  reload: (tabId) => ipcRenderer.invoke('browser-workspace:reload', tabId),
  back: (tabId) => ipcRenderer.invoke('browser-workspace:back', tabId),
  forward: (tabId) => ipcRenderer.invoke('browser-workspace:forward', tabId),
  setLayout: (bounds) => ipcRenderer.invoke('browser-workspace:set-layout', bounds || {}),
  getTabs: () => ipcRenderer.invoke('browser-workspace:get-tabs'),
  openAdmin: () => ipcRenderer.invoke('browser-workspace:open-admin'),
  onTabsUpdated: (callback) => {
    const listener = (_event, snapshot) => callback(snapshot);
    ipcRenderer.on('browser-workspace:tabs-updated', listener);
    return () => ipcRenderer.removeListener('browser-workspace:tabs-updated', listener);
  }
});
