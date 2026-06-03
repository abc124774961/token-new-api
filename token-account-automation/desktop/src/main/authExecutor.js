import { createAuthorizationFlow, exchangeAuthorizationCode, OAuthCallbackServer } from './oauthFlow.js';

const LOGIN_TIMEOUT_MS = 15 * 60 * 1000;

export class AuthExecutor {
  constructor({ store, automation, browserSessions, notify }) {
    this.store = store;
    this.automation = automation;
    this.browserSessions = browserSessions;
    this.notify = notify;
    this.running = false;
    this.active = new Map();
    this.callbackServer = null;
    this.timer = null;
  }

  status() {
    return {
      running: this.running,
      active: Array.from(this.active.values()).map((item) => item.summary),
    };
  }

  async start() {
    if (this.running) {
      return this.status();
    }
    this.running = true;
    await this.ensureCallbackServer();
    this.loop();
    return this.status();
  }

  async stop() {
    this.running = false;
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    return this.status();
  }

  async ensureCallbackServer() {
    const settings = this.store.snapshot().settings;
    if (this.callbackServer?.port === settings.callbackPort) {
      return;
    }
    if (this.callbackServer) {
      await this.callbackServer.close();
    }
    this.callbackServer = new OAuthCallbackServer(settings.callbackPort);
    await this.callbackServer.start();
  }

  loop() {
    if (!this.running) {
      return;
    }
    this.drain().catch((error) => this.notify?.('executor:error', { message: error.message }));
    this.timer = setTimeout(() => this.loop(), 2500);
  }

  async drain() {
    const settings = this.store.snapshot().settings;
    const capacity = Math.max(1, Number(settings.maxConcurrency || 3)) - this.active.size;
    for (let i = 0; i < capacity; i += 1) {
      const claim = await this.automation.claim().catch((error) => {
        this.notify?.('executor:claim-error', { message: error.message });
        return null;
      });
      if (!claim?.job) {
        return;
      }
      this.handleJob(claim.job).catch((error) => {
        this.notify?.('executor:job-error', { jobId: claim.job.job_id, message: error.message });
      });
    }
  }

  async handleJob(job) {
    const input = parseJobInput(job.input_json);
    const proxy = this.store.nextProxy(input.preferred_proxy_id, { targetRef: job.target_ref });
    const summary = {
      jobId: job.job_id,
      targetRef: job.target_ref,
      status: 'starting',
      proxyId: proxy?.id || '',
      proxyScore: proxy?.healthScore || 0,
    };
    this.active.set(job.job_id, { job, summary });
    const heartbeat = setInterval(() => {
      this.automation.heartbeat(job.job_id).catch(() => {});
    }, 30 * 1000);
    try {
      if (job.task_type === 'account_probe') {
        await this.handleAccountProbe(job, input, proxy, summary);
        return;
      }
      if (job.task_type === 'account_profile_verify') {
        await this.handleAccountProfileVerify(job, input, proxy, summary);
        return;
      }
      if (job.task_type !== 'auth_browser_login') {
        await this.automation.fail(job.job_id, 'unsupported_task', `unsupported desktop task_type=${job.task_type}`, 0);
        summary.status = 'failed';
        return;
      }
      await this.automation.stage(job.job_id, 'desktop_oauth_prepare', 'preparing desktop oauth flow', {
        proxy_id: proxy?.id || '',
        proxy_score: proxy?.healthScore || 0,
        proxy_reason: proxy?.selectionReason || '',
      });
      const settings = this.store.snapshot().settings;
      const flow = createAuthorizationFlow(settings.callbackPort);
      summary.status = 'browser_opened';
      const sessionInfo = await this.browserSessions.openAuthView({ job, flow, proxy });
      if (input.clear_session) {
        await this.browserSessions.clearSession(job.job_id);
      }
      await this.automation.stage(job.job_id, 'desktop_oauth_opened', 'desktop browser opened', {
        partition: sessionInfo.partition,
        proxy_id: proxy?.id || '',
        proxy_name: proxy?.name || '',
        proxy_score: proxy?.healthScore || 0,
        proxy_reason: proxy?.selectionReason || '',
      });
      const callback = await this.callbackServer.waitFor(flow.state, LOGIN_TIMEOUT_MS);
      summary.status = 'token_exchange';
      const tokens = await exchangeAuthorizationCode(
        this.browserSessions.fetchForJob(job.job_id),
        callback.code,
        flow.verifier,
        flow.redirectURI,
      );
      await this.automation.succeedCredential(job.job_id, { ...tokens, target_ref: job.target_ref }, tokens.expires_at, {
        auth_method: 'desktop_session',
        proxy_id: proxy?.id || '',
        proxy_name: proxy?.name || '',
        proxy_score: proxy?.healthScore || 0,
        proxy_reason: proxy?.selectionReason || '',
      });
      if (proxy?.id) {
        await this.store.markProxySuccess(proxy.id);
      }
      summary.status = 'success';
      this.notify?.('executor:job-success', { jobId: job.job_id });
    } catch (error) {
      if (String(error.message || '').includes('authorization callback')) {
        await this.automation.waitingHuman(job.job_id, error.message, {
          auth_method: 'desktop_session',
          proxy_id: proxy?.id || '',
        });
        summary.status = 'waiting_human';
      } else {
        if (proxy?.id) {
          await this.store.markProxyFailure(proxy.id, error.message);
        }
        await this.automation.fail(job.job_id, 'desktop_auth_failed', error.message, 60);
        summary.status = 'failed';
      }
    } finally {
      clearInterval(heartbeat);
      this.browserSessions.close(job.job_id);
      this.active.delete(job.job_id);
    }
  }

  async handleAccountProbe(job, input, proxy, summary) {
    summary.status = 'probing';
    if (input.clear_session && job.target_ref) {
      await this.browserSessions.clearAccountSession(job.target_ref);
    }
    const sessionInfo = await this.browserSessions.prepareAccountSession({ job, proxy });
    const result = this.diagnosticResult(job, input, proxy, sessionInfo, {
      diagnostic_type: 'account_probe',
      local_session_ready: true,
      proxy_bound: Boolean(proxy?.proxyRules),
      upstream_probe_status: 'not_configured',
      recommendation: proxy?.id ? 'proxy selected; upstream account probe callback pending' : 'no proxy selected; upstream account probe callback pending',
    });
    await this.automation.stage(job.job_id, 'desktop_probe_complete', 'desktop account probe snapshot collected', result);
    await this.automation.succeed(job.job_id, result);
    summary.status = 'success';
    this.notify?.('executor:job-success', { jobId: job.job_id, taskType: job.task_type });
  }

  async handleAccountProfileVerify(job, input, proxy, summary) {
    summary.status = 'profile_verify';
    if (input.clear_session && job.target_ref) {
      await this.browserSessions.clearAccountSession(job.target_ref);
    }
    const sessionInfo = await this.browserSessions.prepareAccountSession({ job, proxy });
    const result = this.diagnosticResult(job, input, proxy, sessionInfo, {
      diagnostic_type: 'account_profile_verify',
      local_profile_snapshot: {
        target_ref: job.target_ref || input.target_ref || '',
        target_provider: input.target_provider || '',
        target_status: input.target_status || '',
        target_display_name: input.target_display_name || '',
        target_subject_key: input.target_subject_key || '',
        channel_id: input.channel_id ?? null,
        credential_index: input.credential_index ?? null,
        external_ref: input.external_ref || '',
      },
      provider_profile_status: 'schema_pending',
      recommendation: 'provider profile callback pending; local desktop account snapshot collected',
    });
    await this.automation.stage(job.job_id, 'desktop_profile_snapshot', 'desktop account profile snapshot collected', result);
    await this.automation.succeed(job.job_id, result);
    summary.status = 'success';
    this.notify?.('executor:job-success', { jobId: job.job_id, taskType: job.task_type });
  }

  diagnosticResult(job, input, proxy, sessionInfo, extra) {
    return {
      ...extra,
      target_ref: job.target_ref || input.target_ref || '',
      source_job_id: input.source_job_id || '',
      reason: input.reason || '',
      gateway_account_profile_status: input.gateway_account_profile_status || '',
      gateway_account_profile_error: input.gateway_account_profile_error || '',
      gateway_account_profile: input.gateway_account_profile || null,
      browser_partition: sessionInfo.partition,
      proxy_id: proxy?.id || '',
      proxy_name: proxy?.name || '',
      proxy_score: proxy?.healthScore || 0,
      proxy_reason: proxy?.selectionReason || proxy?.healthReason || '',
      desktop_versions: {
        electron: process.versions.electron || '',
        chrome: process.versions.chrome || '',
        node: process.versions.node || '',
        platform: process.platform,
        arch: process.arch,
      },
      collected_at: Math.floor(Date.now() / 1000),
    };
  }
}

function parseJobInput(value) {
  if (!value) {
    return {};
  }
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch {
    return {};
  }
}
