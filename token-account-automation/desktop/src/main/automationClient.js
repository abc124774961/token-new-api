export class AutomationClient {
  constructor(getSettings) {
    this.getSettings = getSettings;
  }

  async overview() {
    return this.request('/api/desktop/overview');
  }

  async actionTemplates() {
    return this.request('/api/desktop/action-templates');
  }

  async accounts(params = {}) {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null && value !== '') {
        query.set(key, String(value));
      }
    }
    return this.request(`/api/desktop/accounts?${query.toString()}`);
  }

  async syncProxies(params = {}) {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null && value !== '') {
        query.set(key, String(value));
      }
    }
    return this.request(`/api/desktop/proxies/sync?${query.toString()}`);
  }

  async invalidAccounts(params = {}) {
    const query = new URLSearchParams();
    for (const [key, value] of Object.entries(params)) {
      if (value !== undefined && value !== null && value !== '') {
        query.set(key, String(value));
      }
    }
    try {
      return await this.request(`/api/desktop/account-pools/invalid?${query.toString()}`);
    } catch (error) {
      if (String(error.message || '').includes('status=404')) {
        return {
          items: [],
          total: 0,
          unavailable: true,
          message: '主站失效池接口未更新，发布或重启主站后可用',
        };
      }
      throw error;
    }
  }

  async jobDetail(jobId) {
    return this.request(`/api/desktop/jobs/${encodeURIComponent(jobId)}`);
  }

  async retryJob(jobId, payload = {}) {
    return this.request(`/api/desktop/jobs/${encodeURIComponent(jobId)}/retry`, { method: 'POST', body: payload });
  }

  async resumeJob(jobId, payload = {}) {
    return this.request(`/api/desktop/jobs/${encodeURIComponent(jobId)}/resume`, { method: 'POST', body: payload });
  }

  async cancelJob(jobId, payload = {}) {
    return this.request(`/api/desktop/jobs/${encodeURIComponent(jobId)}/cancel`, { method: 'POST', body: payload });
  }

  async archiveJobAccount(jobId, pool, payload = {}) {
    const action = pool === 'discarded' ? 'archive-discarded' : 'archive-invalid';
    return this.request(`/api/desktop/jobs/${encodeURIComponent(jobId)}/${action}`, { method: 'POST', body: payload });
  }

  async enqueueAccountAction(jobId, action, payload = {}) {
    return this.request(`/api/desktop/jobs/${encodeURIComponent(jobId)}/${encodeURIComponent(action)}`, { method: 'POST', body: payload });
  }

  async reauthorizeInvalidAccount(poolId, payload = {}) {
    return this.request(`/api/desktop/account-pools/invalid/${encodeURIComponent(poolId)}/reauthorize`, { method: 'POST', body: payload });
  }

  async claim() {
    const settings = this.getSettings();
    return this.request('/internal/jobs/claim', {
      method: 'POST',
      body: {
        executor_type: 'desktop_session',
        worker_id: settings.workerId,
        lease_seconds: 960,
        limit: Math.max(1, Number(settings.maxConcurrency || 3)),
      },
    });
  }

  async heartbeat(jobId) {
    const settings = this.getSettings();
    return this.jobAction(jobId, 'heartbeat', { worker_id: settings.workerId, lease_seconds: 960 });
  }

  async stage(jobId, stage, message, data = {}) {
    const settings = this.getSettings();
    return this.jobAction(jobId, 'stage', { worker_id: settings.workerId, stage, message, data });
  }

  async waitingHuman(jobId, reason, data = {}) {
    const settings = this.getSettings();
    return this.jobAction(jobId, 'waiting-human', { worker_id: settings.workerId, reason, data });
  }

  async succeed(jobId, result = {}) {
    const settings = this.getSettings();
    return this.jobAction(jobId, 'succeed', { worker_id: settings.workerId, result });
  }

  async fail(jobId, errorCode, error, retryAfterSeconds = 60) {
    const settings = this.getSettings();
    return this.jobAction(jobId, 'fail', {
      worker_id: settings.workerId,
      error_code: errorCode,
      error,
      retry_after_seconds: retryAfterSeconds,
    });
  }

  async succeedCredential(jobId, value, expiresAt, metadata = {}) {
    const settings = this.getSettings();
    return this.jobAction(jobId, 'succeed-credential', {
      worker_id: settings.workerId,
      value,
      expires_at: expiresAt,
      metadata,
    });
  }

  async jobAction(jobId, action, body) {
    return this.request(`/internal/jobs/${encodeURIComponent(jobId)}/${action}`, { method: 'POST', body });
  }

  async request(route, options = {}) {
    const settings = this.getSettings();
    const baseUrl = String(settings.automationBaseUrl || '').replace(/\/+$/, '');
    const token = String(settings.desktopToken || '').trim();
    if (!baseUrl || !token) {
      throw new Error('automation base url and desktop token are required');
    }
    const response = await fetch(`${baseUrl}${route}`, {
      method: options.method || 'GET',
      headers: {
        Authorization: `Bearer ${token}`,
        Accept: 'application/json',
        'Content-Type': 'application/json',
      },
      body: options.body ? JSON.stringify(options.body) : undefined,
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.success === false) {
      throw new Error(payload.message || `automation request failed status=${response.status}`);
    }
    return payload.data ?? payload;
  }
}
