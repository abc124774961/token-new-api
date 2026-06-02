import http from 'node:http';
import crypto from 'node:crypto';
import { mkdir } from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { chromium } from 'playwright';

const CODEX_OAUTH_CLIENT_ID = 'app_EMoamEEZ73f0CkXaXp7hrann';
const CODEX_OAUTH_AUTHORIZE_URL = 'https://auth.openai.com/oauth/authorize';
const CODEX_OAUTH_TOKEN_URL = 'https://auth.openai.com/oauth/token';
const CODEX_OAUTH_SCOPE = 'openid profile email offline_access';

const cfg = {
  baseUrl: env('AUTOMATION_BASE_URL', 'http://127.0.0.1:8091').replace(/\/+$/, ''),
  workerToken: env('AUTOMATION_WORKER_TOKEN', ''),
  workerId: env('BROWSER_WORKER_ID', 'browser-worker-1'),
  pollIntervalMs: envInt('BROWSER_POLL_INTERVAL_SECONDS', 2) * 1000,
  callbackPort: envInt('BROWSER_CALLBACK_PORT', 1455),
  loginTimeoutMs: envInt('BROWSER_LOGIN_TIMEOUT_SECONDS', 900) * 1000,
  headless: env('BROWSER_HEADLESS', 'true') !== 'false',
  profileDir: env('BROWSER_PROFILE_DIR', ''),
};

if (!cfg.workerToken) {
  throw new Error('AUTOMATION_WORKER_TOKEN is required');
}

await main();

async function main() {
  console.log(`browser worker ${cfg.workerId} polling ${cfg.baseUrl}`);
  for (;;) {
    try {
      const claim = await claimJob();
      if (!claim?.job) {
        await sleep(cfg.pollIntervalMs);
        continue;
      }
      await handleJob(claim.job);
    } catch (error) {
      console.error(`browser worker loop error: ${safeError(error)}`);
      await sleep(cfg.pollIntervalMs);
    }
  }
}

async function claimJob() {
  const data = await apiPost('/internal/jobs/claim', {
    executor_type: 'browser_playwright',
    worker_id: cfg.workerId,
    lease_seconds: Math.max(60, Math.floor(cfg.loginTimeoutMs / 1000) + 30),
    limit: 5,
  });
  return data;
}

async function handleJob(job) {
  if (job.task_type !== 'auth_browser_login') {
    await failJob(job.job_id, 'unsupported_task', `unsupported task_type=${job.task_type}`, 0);
    return;
  }
  await handleAuthBrowserLogin(job);
}

async function handleAuthBrowserLogin(job) {
  let browser;
  let context;
  let callback;
  try {
    await stage(job.job_id, 'oauth_prepare', 'preparing browser oauth flow', { target_ref: job.target_ref });
    const flow = createAuthorizationFlow(cfg.callbackPort);
    callback = await waitForOAuthCallback(cfg.callbackPort, flow.state, cfg.loginTimeoutMs);

    const browserSession = await openBrowserSession(job);
    browser = browserSession.browser;
    context = browserSession.context;
    const page = await context.newPage();
    await page.goto(flow.authorizeUrl, { waitUntil: 'domcontentloaded' });
    await stage(job.job_id, 'oauth_opened', 'oauth authorize page opened', {
      callback_port: cfg.callbackPort,
      headless: cfg.headless,
      persistent_profile: browserSession.persistent,
      profile_key: browserSession.profileKey,
    });

    const callbackResult = await callback.promise;
    if (callbackResult.error) {
      throw new Error(`oauth callback error: ${callbackResult.error}`);
    }
    const tokens = await exchangeAuthorizationCode(callbackResult.code, flow.verifier, flow.redirectURI);
    await apiPost(`/internal/jobs/${job.job_id}/succeed-credential`, {
      worker_id: cfg.workerId,
      expires_at: tokens.expires_at,
      value: {
        access_token: tokens.access_token,
        refresh_token: tokens.refresh_token,
        expires_at: tokens.expires_at,
        provider: 'codex',
        target_ref: job.target_ref,
        type: 'codex',
      },
      metadata: {
        auth_method: 'browser_playwright',
        callback_port: cfg.callbackPort,
      },
    });
  } catch (error) {
    if (error instanceof WaitingHumanError) {
      await waitingHuman(job.job_id, error.message, {
        auth_method: 'browser_playwright',
        callback_port: cfg.callbackPort,
        headless: cfg.headless,
      });
      return;
    }
    await failJob(job.job_id, 'browser_login_failed', safeError(error), 60);
  } finally {
    if (callback) {
      await callback.close();
    }
    if (context) {
      await context.close();
    }
    if (browser) {
      await browser.close();
    }
  }
}

async function openBrowserSession(job) {
  const launchOptions = {
    headless: cfg.headless,
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  };
  const profileKey = safePathSegment(job.target_ref || job.job_id);
  if (cfg.profileDir) {
    const userDataDir = path.join(cfg.profileDir, profileKey);
    await mkdir(userDataDir, { recursive: true });
    const context = await chromium.launchPersistentContext(userDataDir, launchOptions);
    return { context, browser: null, persistent: true, profileKey };
  }
  const browser = await chromium.launch(launchOptions);
  const context = await browser.newContext();
  return { context, browser, persistent: false, profileKey: '' };
}

function createAuthorizationFlow(callbackPort) {
  const verifier = base64url(crypto.randomBytes(32));
  const challenge = base64url(crypto.createHash('sha256').update(verifier).digest());
  const state = crypto.randomBytes(16).toString('hex');
  const redirectURI = `http://localhost:${callbackPort}/auth/callback`;
  const url = new URL(CODEX_OAUTH_AUTHORIZE_URL);
  url.searchParams.set('response_type', 'code');
  url.searchParams.set('client_id', CODEX_OAUTH_CLIENT_ID);
  url.searchParams.set('redirect_uri', redirectURI);
  url.searchParams.set('scope', CODEX_OAUTH_SCOPE);
  url.searchParams.set('code_challenge', challenge);
  url.searchParams.set('code_challenge_method', 'S256');
  url.searchParams.set('state', state);
  url.searchParams.set('id_token_add_organizations', 'true');
  url.searchParams.set('codex_cli_simplified_flow', 'true');
  url.searchParams.set('originator', 'codex_cli_rs');
  return { verifier, challenge, state, redirectURI, authorizeUrl: url.toString() };
}

async function waitForOAuthCallback(port, expectedState, timeoutMs) {
  let server;
  let timeout;
  const promise = new Promise((resolve, reject) => {
    server = http.createServer((req, res) => {
      const reqURL = new URL(req.url ?? '/', `http://localhost:${port}`);
      if (reqURL.pathname !== '/auth/callback') {
        res.statusCode = 404;
        res.end('not found');
        return;
      }
      const state = reqURL.searchParams.get('state') ?? '';
      if (state !== expectedState) {
        res.statusCode = 400;
        res.end('state mismatch');
        reject(new Error('oauth state mismatch'));
        return;
      }
      const error = reqURL.searchParams.get('error') ?? '';
      const code = reqURL.searchParams.get('code') ?? '';
      res.statusCode = 200;
      res.end('Authorization received. You can close this browser tab.');
      resolve({ code, error });
    });
    server.listen(port, '127.0.0.1', () => {});
    timeout = setTimeout(() => {
      reject(new WaitingHumanError('waiting for browser authorization callback'));
    }, timeoutMs);
  });
  return {
    promise: promise.finally(() => clearTimeout(timeout)),
    close: () => new Promise((resolve) => {
      if (!server) {
        resolve();
        return;
      }
      server.close(() => resolve());
    }),
  };
}

async function exchangeAuthorizationCode(code, verifier, redirectURI) {
  if (!code) {
    throw new Error('authorization code missing');
  }
  const body = new URLSearchParams();
  body.set('grant_type', 'authorization_code');
  body.set('client_id', CODEX_OAUTH_CLIENT_ID);
  body.set('code', code);
  body.set('code_verifier', verifier);
  body.set('redirect_uri', redirectURI);
  const response = await fetch(CODEX_OAUTH_TOKEN_URL, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(`oauth token exchange failed status=${response.status} code=${payload.error ?? ''}`);
  }
  if (!payload.access_token || !payload.refresh_token || !payload.expires_in) {
    throw new Error('oauth token exchange response missing fields');
  }
  return {
    access_token: payload.access_token,
    refresh_token: payload.refresh_token,
    expires_at: Math.floor(Date.now() / 1000) + Number(payload.expires_in),
  };
}

async function stage(jobId, stageName, message, data = {}) {
  await apiPost(`/internal/jobs/${jobId}/stage`, {
    worker_id: cfg.workerId,
    stage: stageName,
    message,
    data,
  });
}

async function waitingHuman(jobId, reason, data = {}) {
  await apiPost(`/internal/jobs/${jobId}/waiting-human`, {
    worker_id: cfg.workerId,
    reason,
    data,
  });
}

async function failJob(jobId, errorCode, error, retryAfterSeconds) {
  await apiPost(`/internal/jobs/${jobId}/fail`, {
    worker_id: cfg.workerId,
    error_code: errorCode,
    error,
    retry_after_seconds: retryAfterSeconds,
  });
}

async function apiPost(path, payload) {
  const response = await fetch(`${cfg.baseUrl}${path}`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${cfg.workerToken}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok || data.success === false) {
    throw new Error(data.message || `automation api failed status=${response.status}`);
  }
  return data.data ?? data;
}

function env(name, fallback) {
  const value = process.env[name];
  return value && value.trim() ? value.trim() : fallback;
}

function envInt(name, fallback) {
  const value = Number.parseInt(env(name, ''), 10);
  return Number.isFinite(value) && value > 0 ? value : fallback;
}

function safePathSegment(value) {
  const sanitized = String(value ?? '')
    .trim()
    .replace(/[^a-zA-Z0-9._-]/g, '_')
    .slice(0, 120);
  return sanitized || 'unknown';
}

function base64url(buffer) {
  return Buffer.from(buffer).toString('base64url');
}

function safeError(error) {
  if (!error) {
    return 'unknown error';
  }
  const message = error instanceof Error ? error.message : String(error);
  return message.slice(0, 1000);
}

class WaitingHumanError extends Error {}
