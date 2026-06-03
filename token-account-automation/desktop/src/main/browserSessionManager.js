import { BrowserWindow, WebContentsView, session } from 'electron';
import crypto from 'node:crypto';

export class BrowserSessionManager {
  constructor(getMainWindow, notify) {
    this.getMainWindow = getMainWindow;
    this.notify = notify;
    this.views = new Map();
  }

  async openAuthView({ job, flow, proxy }) {
    const { session: ses, partition } = await this.prepareAccountSession({ job, proxy });
    const view = new WebContentsView({
      webPreferences: {
        partition,
        sandbox: true,
        nodeIntegration: false,
        contextIsolation: true,
      },
    });
    view.webContents.setWindowOpenHandler(({ url }) => {
      view.webContents.loadURL(url);
      return { action: 'deny' };
    });
    const win = this.getMainWindow();
    if (!win || win.isDestroyed()) {
      throw new Error('main window is not available');
    }
    win.contentView.addChildView(view);
    this.layoutView(view);
    this.views.set(job.job_id, { view, session: ses, partition, proxy });
    view.webContents.on('did-navigate', (_, url) => this.notify?.('browser:navigate', { jobId: job.job_id, url }));
    await view.webContents.loadURL(flow.authorizeUrl);
    return { session: ses, partition };
  }

  async prepareAccountSession({ job, proxy }) {
    const key = partitionKey(job.target_ref || job.job_id);
    const partition = `persist:${key}`;
    const ses = session.fromPartition(partition);
    if (proxy?.proxyRules) {
      await ses.setProxy({ mode: 'fixed_servers', proxyRules: proxy.proxyRules });
      await ses.closeAllConnections();
    }
    return { session: ses, partition, proxy };
  }

  layoutAll() {
    for (const { view } of this.views.values()) {
      this.layoutView(view);
    }
  }

  layoutView(view) {
    const win = this.getMainWindow();
    if (!win || win.isDestroyed()) {
      return;
    }
    const [width, height] = win.getContentSize();
    const panelWidth = Math.max(640, Math.floor(width * 0.58));
    view.setBounds({
      x: width - panelWidth,
      y: 0,
      width: panelWidth,
      height,
    });
  }

  close(jobId) {
    const record = this.views.get(jobId);
    if (!record) {
      return;
    }
    const win = this.getMainWindow();
    if (win && !win.isDestroyed()) {
      win.contentView.removeChildView(record.view);
    }
    record.view.webContents.close();
    this.views.delete(jobId);
  }

  async clearSession(jobId) {
    const record = this.views.get(jobId);
    if (!record) {
      return;
    }
    await record.session.clearStorageData();
    await record.session.clearCache();
  }

  async clearAccountSession(targetRef) {
    const key = partitionKey(targetRef);
    const partition = `persist:${key}`;
    for (const [jobId, record] of this.views.entries()) {
      if (record.partition === partition) {
        this.close(jobId);
      }
    }
    const ses = session.fromPartition(partition);
    await ses.clearStorageData();
    await ses.clearCache();
    await ses.closeAllConnections();
    return { partition };
  }

  fetchForJob(jobId) {
    const record = this.views.get(jobId);
    if (record?.session && typeof record.session.fetch === 'function') {
      return record.session.fetch.bind(record.session);
    }
    return fetch;
  }
}

function partitionKey(value) {
  return `account-${crypto.createHash('sha256').update(String(value || 'unknown')).digest('hex').slice(0, 20)}`;
}
