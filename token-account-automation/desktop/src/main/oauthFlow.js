import crypto from 'node:crypto';
import http from 'node:http';

export const CODEX_OAUTH_CLIENT_ID = 'app_EMoamEEZ73f0CkXaXp7hrann';
export const CODEX_OAUTH_AUTHORIZE_URL = 'https://auth.openai.com/oauth/authorize';
export const CODEX_OAUTH_TOKEN_URL = 'https://auth.openai.com/oauth/token';
export const CODEX_OAUTH_SCOPE = 'openid profile email offline_access';

export function createAuthorizationFlow(callbackPort) {
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

export class OAuthCallbackServer {
  constructor(port) {
    this.port = port;
    this.pending = new Map();
    this.server = null;
  }

  async start() {
    if (this.server) {
      return;
    }
    this.server = http.createServer((req, res) => this.handle(req, res));
    await new Promise((resolve, reject) => {
      this.server.once('error', reject);
      this.server.listen(this.port, '127.0.0.1', resolve);
    });
  }

  waitFor(state, timeoutMs) {
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pending.delete(state);
        reject(new Error('waiting for browser authorization callback'));
      }, timeoutMs);
      this.pending.set(state, {
        resolve: (value) => {
          clearTimeout(timeout);
          resolve(value);
        },
        reject: (error) => {
          clearTimeout(timeout);
          reject(error);
        },
      });
    });
  }

  handle(req, res) {
    const reqURL = new URL(req.url || '/', `http://localhost:${this.port}`);
    if (reqURL.pathname !== '/auth/callback') {
      res.statusCode = 404;
      res.end('not found');
      return;
    }
    const state = reqURL.searchParams.get('state') || '';
    const pending = this.pending.get(state);
    if (!pending) {
      res.statusCode = 400;
      res.end('unknown state');
      return;
    }
    this.pending.delete(state);
    const error = reqURL.searchParams.get('error') || '';
    const code = reqURL.searchParams.get('code') || '';
    res.statusCode = 200;
    res.end('Authorization received. You can return to the desktop client.');
    if (error) {
      pending.reject(new Error(`oauth callback error: ${error}`));
      return;
    }
    pending.resolve({ code });
  }

  async close() {
    if (!this.server) {
      return;
    }
    const server = this.server;
    this.server = null;
    await new Promise((resolve) => server.close(resolve));
  }
}

export async function exchangeAuthorizationCode(fetcher, code, verifier, redirectURI) {
  if (!code) {
    throw new Error('authorization code missing');
  }
  const body = new URLSearchParams();
  body.set('grant_type', 'authorization_code');
  body.set('client_id', CODEX_OAUTH_CLIENT_ID);
  body.set('code', code);
  body.set('code_verifier', verifier);
  body.set('redirect_uri', redirectURI);
  const response = await fetcher(CODEX_OAUTH_TOKEN_URL, {
    method: 'POST',
    headers: {
      Accept: 'application/json',
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body,
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(`oauth token exchange failed status=${response.status} code=${payload.error || ''}`);
  }
  if (!payload.access_token || !payload.refresh_token || !payload.expires_in) {
    throw new Error('oauth token exchange response missing fields');
  }
  return {
    access_token: payload.access_token,
    refresh_token: payload.refresh_token,
    expires_at: Math.floor(Date.now() / 1000) + Number(payload.expires_in),
    provider: 'codex',
    type: 'codex',
  };
}

function base64url(buffer) {
  return buffer.toString('base64').replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
