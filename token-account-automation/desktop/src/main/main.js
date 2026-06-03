import { app, BrowserWindow, ipcMain } from 'electron';
import { existsSync, readFileSync } from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { AutomationClient } from './automationClient.js';
import { AuthExecutor } from './authExecutor.js';
import { BrowserSessionManager } from './browserSessionManager.js';
import { SecureStore } from './store.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

let mainWindow;
let store;
let automation;
let browserSessions;
let executor;

const isDev = !app.isPackaged;
const debugEnabled = process.env.AUTOMATION_DESKTOP_DEBUG === '1' || process.env.ELECTRON_ENABLE_LOGGING;

loadEnvFile(process.env.AUTOMATION_ENV_FILE);

process.on('uncaughtException', (error) => {
  console.error('[desktop] uncaught exception:', error);
});

process.on('unhandledRejection', (error) => {
  console.error('[desktop] unhandled rejection:', error);
});

debugLog('registering app ready handler');
app.whenReady().then(async () => {
  debugLog('app ready');
  await bootstrap();
  debugLog('bootstrap complete');
}).catch((error) => {
  console.error('[desktop] bootstrap failed:', error);
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow();
  }
});

async function bootstrap() {
  debugLog('loading secure store');
  store = new SecureStore();
  await store.load();
  debugLog('initializing services');
  automation = new AutomationClient(() => store.snapshot().settings);
  browserSessions = new BrowserSessionManager(() => mainWindow, emit);
  executor = new AuthExecutor({ store, automation, browserSessions, notify: emit });
  createWindow();
  registerIPC();
}

function createWindow() {
  debugLog('creating BrowserWindow');
  mainWindow = new BrowserWindow({
    width: 1680,
    height: 1040,
    minWidth: 1280,
    minHeight: 760,
    show: false,
    autoHideMenuBar: true,
    title: 'Token Account Automation Desktop',
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      sandbox: true,
      nodeIntegration: false,
      contextIsolation: true,
    },
  });
  mainWindow.once('ready-to-show', () => {
    debugLog('window ready-to-show');
    if (!mainWindow.isDestroyed()) {
      mainWindow.maximize();
      mainWindow.show();
      mainWindow.focus();
    }
  });
  mainWindow.webContents.on('did-finish-load', () => debugLog('renderer did-finish-load'));
  mainWindow.webContents.on('did-fail-load', (_, code, description, validatedURL) => {
    console.error('[desktop] renderer did-fail-load:', code, description, validatedURL);
  });
  mainWindow.on('resize', () => browserSessions?.layoutAll());
  if (
    isDev &&
    process.env.AUTOMATION_DESKTOP_LOAD_DIST !== '1' &&
    process.env.AUTOMATION_DESKTOP_RENDERER_URL
  ) {
    const rendererUrl = process.env.AUTOMATION_DESKTOP_RENDERER_URL;
    debugLog(`loading dev renderer ${rendererUrl}`);
    mainWindow.loadURL(rendererUrl);
  } else {
    debugLog('loading packaged renderer');
    mainWindow.loadFile(path.join(__dirname, '../../dist/index.html'));
  }
}

function registerIPC() {
  ipcMain.handle('app:get-state', async () => ({
    store: store.snapshot(),
    executor: executor.status(),
  }));
  ipcMain.handle('settings:update', async (_, settings) => {
    await store.updateSettings(settings);
    return { store: store.snapshot(), executor: executor.status() };
  });
  ipcMain.handle('automation:overview', async () => automation.overview());
  ipcMain.handle('automation:action-templates', async () => automation.actionTemplates());
  ipcMain.handle('automation:accounts', async (_, params) => automation.accounts(params));
  ipcMain.handle('automation:invalid-accounts', async (_, params) => automation.invalidAccounts(params));
  ipcMain.handle('automation:job-detail', async (_, jobId) => automation.jobDetail(jobId));
  ipcMain.handle('automation:retry-job', async (_, jobId, payload) => automation.retryJob(jobId, payload));
  ipcMain.handle('automation:resume-job', async (_, jobId, payload) => automation.resumeJob(jobId, payload));
  ipcMain.handle('automation:cancel-job', async (_, jobId, payload) => automation.cancelJob(jobId, payload));
  ipcMain.handle('automation:archive-job-account', async (_, jobId, pool, payload) => automation.archiveJobAccount(jobId, pool, payload));
  ipcMain.handle('automation:enqueue-account-action', async (_, jobId, action, payload) => automation.enqueueAccountAction(jobId, action, payload));
  ipcMain.handle('automation:reauthorize-invalid-account', async (_, poolId, payload) => automation.reauthorizeInvalidAccount(poolId, payload));
  ipcMain.handle('automation:sync-proxies', async (_, params) => {
    const result = await automation.syncProxies(params);
    const storeSnapshot = await store.mergeRemoteProxies(result.items || []);
    return { result, store: storeSnapshot };
  });
  ipcMain.handle('diagnostics:get', async () => collectDiagnostics());
  ipcMain.handle('executor:start', async () => executor.start());
  ipcMain.handle('executor:stop', async () => executor.stop());
  ipcMain.handle('proxy:upsert', async (_, proxy) => store.upsertProxy(proxy));
  ipcMain.handle('proxy:toggle', async (_, id, enabled) => store.toggleProxy(id, enabled));
  ipcMain.handle('proxy:delete', async (_, id) => store.deleteProxy(id));
  ipcMain.handle('browser:clear-session', async (_, jobId) => browserSessions.clearSession(jobId));
  ipcMain.handle('browser:clear-account-session', async (_, targetRef) => browserSessions.clearAccountSession(targetRef));
}

async function collectDiagnostics() {
  const settings = store.snapshot().settings || {};
  const automationBaseUrl = String(settings.automationBaseUrl || '').replace(/\/+$/, '');
  const gatewayCallbackUrl = String(process.env.AUTOMATION_GATEWAY_CALLBACK_URL || '').replace(/\/+$/, '');
  const gatewayCallbackToken = process.env.AUTOMATION_GATEWAY_CALLBACK_TOKEN || process.env.TOKEN_ACCOUNT_AUTOMATION_CALLBACK_TOKEN || '';
  const envFile = resolveEnvFilePath(process.env.AUTOMATION_ENV_FILE || '');
  const [
    health,
    overview,
    desktopInvalidPool,
    callbackPortStatus,
    gatewayProxies,
    gatewayInvalidPool,
  ] = await Promise.all([
    fetchJSON(`${automationBaseUrl}/health`, settings.desktopToken),
    fetchJSON(`${automationBaseUrl}/api/desktop/overview`, settings.desktopToken),
    fetchJSON(`${automationBaseUrl}/api/desktop/account-pools/invalid?page=1&page_size=1`, settings.desktopToken),
    checkCallbackPort(Number(settings.callbackPort || 0)),
    gatewayCallbackUrl && gatewayCallbackToken
      ? fetchJSON(`${gatewayCallbackUrl}/api/internal/token-account-automation/proxies?enabled_only=true`, gatewayCallbackToken)
      : Promise.resolve({ ok: false, status: 0, message: gatewayCallbackUrl ? 'callback token is not configured' : 'gateway callback url is not configured' }),
    gatewayCallbackUrl && gatewayCallbackToken
      ? fetchJSON(`${gatewayCallbackUrl}/api/internal/token-account-automation/account-pools/invalid?page=1&page_size=1`, gatewayCallbackToken)
      : Promise.resolve({ ok: false, status: 0, message: gatewayCallbackUrl ? 'callback token is not configured' : 'gateway callback url is not configured' }),
  ]);
  return {
    appVersion: app.getVersion(),
    electronVersion: process.versions.electron,
    chromeVersion: process.versions.chrome,
    nodeVersion: process.versions.node,
    platform: process.platform,
    arch: process.arch,
    hostname: os.hostname(),
    automationBaseUrl,
    callbackPort: settings.callbackPort,
    callbackPortStatus,
    maxConcurrency: settings.maxConcurrency,
    envFile,
    envFileExists: envFile ? existsSync(envFile) : false,
    gatewayCallbackUrl,
    gatewayCallbackTokenConfigured: Boolean(gatewayCallbackToken),
    browserLoginExecutor: process.env.AUTOMATION_BROWSER_LOGIN_EXECUTOR || '',
    health,
    overview,
    desktopInvalidPool,
    gateway: {
      proxies: gatewayProxies,
      invalidPool: gatewayInvalidPool,
    },
  };
}

async function fetchJSON(url, token) {
  if (!url || !url.startsWith('http')) {
    return { ok: false, status: 0, message: 'url is not configured' };
  }
  try {
    const response = await fetch(url, {
      headers: token ? { Authorization: `Bearer ${token}`, Accept: 'application/json' } : { Accept: 'application/json' },
    });
    const payload = await response.json().catch(() => ({}));
    return {
      ok: response.ok && payload.success !== false,
      status: response.status,
      message: payload.message || '',
      data: payload.data || payload,
    };
  } catch (error) {
    return { ok: false, status: 0, message: error.message };
  }
}

function checkCallbackPort(port) {
  if (!port) {
    return Promise.resolve({ ok: false, status: 'missing', message: 'callback port is not configured' });
  }
  if (executor?.callbackServer?.server && executor.callbackServer.port === port) {
    return Promise.resolve({ ok: true, status: 'listening', message: 'desktop callback server is listening' });
  }
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once('error', (error) => {
      resolve({ ok: false, status: 'busy', message: error.message });
    });
    server.once('listening', () => {
      server.close(() => resolve({ ok: true, status: 'available', message: 'port is available' }));
    });
    server.listen(port, '127.0.0.1');
  });
}

function emit(event, payload) {
  if (mainWindow && !mainWindow.isDestroyed()) {
    mainWindow.webContents.send('main:event', event, payload);
  }
}

function debugLog(...args) {
  if (debugEnabled) {
    console.log('[desktop]', ...args);
  }
}

function loadEnvFile(filepath) {
  if (!filepath) {
    return;
  }
  const resolved = resolveEnvFilePath(filepath);
  if (!existsSync(resolved)) {
    return;
  }
  const lines = readFileSync(resolved, 'utf8').split(/\r?\n/);
  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!line || line.startsWith('#')) {
      continue;
    }
    const normalized = line.startsWith('export ') ? line.slice(7).trim() : line;
    const index = normalized.indexOf('=');
    if (index <= 0) {
      continue;
    }
    const key = normalized.slice(0, index).trim();
    let value = normalized.slice(index + 1).trim();
    if (process.env[key]) {
      continue;
    }
    if (
      (value.startsWith('"') && value.endsWith('"')) ||
      (value.startsWith("'") && value.endsWith("'"))
    ) {
      value = value.slice(1, -1);
    }
    value = value.replace(/\$\{([A-Za-z_][A-Za-z0-9_]*)\}/g, (_, name) => process.env[name] || '');
    process.env[key] = value;
  }
}

function resolveEnvFilePath(filepath) {
  if (!filepath) {
    return '';
  }
  return path.resolve(__dirname, filepath);
}
