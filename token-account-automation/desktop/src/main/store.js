import { app, safeStorage } from 'electron';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';

const STORE_VERSION = 1;

export class SecureStore {
  constructor(filename = 'desktop-store.json') {
    this.filepath = path.join(app.getPath('userData'), filename);
    this.data = {
      version: STORE_VERSION,
      settings: {
        automationBaseUrl: '',
        desktopToken: '',
        workerId: 'desktop-client-1',
        maxConcurrency: 3,
        callbackPort: 1455,
      },
      proxies: [],
    };
  }

  async load() {
    try {
      const raw = await readFile(this.filepath, 'utf8');
      const payload = JSON.parse(raw);
      if (payload.encrypted && payload.value) {
        const decrypted = safeStorage.decryptString(Buffer.from(payload.value, 'base64'));
        this.data = { ...this.data, ...JSON.parse(decrypted) };
        this.applyEnvDefaults();
        return this.snapshot();
      }
      if (payload && typeof payload === 'object') {
        this.data = { ...this.data, ...payload };
      }
    } catch {
      this.applyEnvDefaults();
      await this.save();
      return this.snapshot();
    }
    this.applyEnvDefaults();
    return this.snapshot();
  }

  async save() {
    await mkdir(path.dirname(this.filepath), { recursive: true });
    const json = JSON.stringify(this.data);
    const payload = safeStorage.isEncryptionAvailable()
      ? { encrypted: true, value: safeStorage.encryptString(json).toString('base64') }
      : { encrypted: false, ...this.data };
    await writeFile(this.filepath, JSON.stringify(payload, null, 2));
    return this.snapshot();
  }

  snapshot() {
    const copy = JSON.parse(JSON.stringify(this.data));
    const now = Math.floor(Date.now() / 1000);
    copy.proxies = (copy.proxies || []).map((proxy) => enrichProxy(proxy, now));
    copy.proxyHealth = summarizeProxyHealth(copy.proxies);
    return copy;
  }

  async updateSettings(next) {
    this.data.settings = {
      ...this.data.settings,
      ...sanitizeSettings(next),
    };
    return this.save();
  }

  async upsertProxy(proxy) {
    const normalized = normalizeProxy(proxy);
    if (!normalized.id) {
      normalized.id = `local-${Date.now()}`;
    }
    const index = this.data.proxies.findIndex((item) => item.id === normalized.id);
    if (index >= 0) {
      this.data.proxies[index] = { ...this.data.proxies[index], ...normalized };
    } else {
      this.data.proxies.push(normalized);
    }
    return this.save();
  }

  async mergeRemoteProxies(proxies = []) {
    let imported = 0;
    let updated = 0;
    const now = Math.floor(Date.now() / 1000);
    for (const proxy of proxies) {
      const normalized = normalizeRemoteProxy(proxy, now);
      if (!normalized.id || !normalized.proxyRules) {
        continue;
      }
      const index = this.data.proxies.findIndex((item) => item.id === normalized.id);
      if (index >= 0) {
        const existing = this.data.proxies[index];
        const localFailureIsNewer = Number(existing.lastFailureAt || 0) > Number(normalized.lastFailureAt || 0);
        this.data.proxies[index] = {
          ...existing,
          ...normalized,
          enabled: existing.enabled !== false && normalized.enabled !== false,
          failureCount: localFailureIsNewer ? Number(existing.failureCount || 0) : Number(normalized.failureCount || 0),
          lastFailureAt: localFailureIsNewer ? Number(existing.lastFailureAt || 0) : Number(normalized.lastFailureAt || 0),
          lastError: localFailureIsNewer ? String(existing.lastError || '') : '',
          cooldownUntil: localFailureIsNewer ? Number(existing.cooldownUntil || 0) : Number(normalized.cooldownUntil || 0),
          successCount: Math.max(Number(existing.successCount || 0), Number(normalized.successCount || 0)),
          lastSuccessAt: Math.max(Number(existing.lastSuccessAt || 0), Number(normalized.lastSuccessAt || 0)),
          useCount: Math.max(Number(existing.useCount || 0), Number(normalized.useCount || 0)),
        };
        updated += 1;
      } else {
        this.data.proxies.push(normalized);
        imported += 1;
      }
    }
    this.data.proxySync = {
      lastSyncedAt: now,
      imported,
      updated,
      total: proxies.length,
    };
    await this.save();
    return this.snapshot();
  }

  async toggleProxy(id, enabled) {
    const proxy = this.data.proxies.find((item) => item.id === id);
    if (!proxy) {
      return this.snapshot();
    }
    proxy.enabled = enabled !== false;
    return this.save();
  }

  async deleteProxy(id) {
    this.data.proxies = this.data.proxies.filter((item) => item.id !== id);
    return this.save();
  }

  nextProxy(preferredProxyId = '', context = {}) {
    const now = Math.floor(Date.now() / 1000);
    preferredProxyId = String(preferredProxyId || '').trim();
    if (preferredProxyId) {
      const preferred = this.data.proxies.find((item) => item.id === preferredProxyId);
      const scored = preferred ? enrichProxy(preferred, now, context) : null;
      if (scored && scored.proxyStatus === 'available') {
        return { ...preferred, ...scored, selectionReason: `operator preferred · ${scored.healthReason}` };
      }
    }
    const available = this.data.proxies
      .map((proxy) => enrichProxy(proxy, now, context))
      .filter((proxy) => proxy.proxyStatus === 'available')
      .sort((left, right) => {
        if (Number(right.healthScore || 0) !== Number(left.healthScore || 0)) {
          return Number(right.healthScore || 0) - Number(left.healthScore || 0);
        }
        return String(left.name || left.id).localeCompare(String(right.name || right.id));
      });
    const selected = available[0];
    return selected ? { ...selected, selectionReason: `auto selected · ${selected.healthReason}` } : null;
  }

  recommendedProxies(limit = 5) {
    const now = Math.floor(Date.now() / 1000);
    return this.data.proxies
      .map((proxy) => enrichProxy(proxy, now))
      .sort((left, right) => {
        if (left.proxyStatus !== right.proxyStatus) {
          return left.proxyStatus === 'available' ? -1 : 1;
        }
        return Number(right.healthScore || 0) - Number(left.healthScore || 0);
      })
      .slice(0, Math.max(1, Number(limit || 5)));
  }

  async markProxyFailure(id, message) {
    const proxy = this.data.proxies.find((item) => item.id === id);
    if (!proxy) {
      return this.snapshot();
    }
    proxy.failureCount = Number(proxy.failureCount || 0) + 1;
    proxy.lastFailureAt = Math.floor(Date.now() / 1000);
    proxy.lastError = String(message || '').slice(0, 300);
    proxy.cooldownUntil = proxy.lastFailureAt + Math.min(3600, 60 * proxy.failureCount);
    return this.save();
  }

  async markProxySuccess(id) {
    const proxy = this.data.proxies.find((item) => item.id === id);
    if (!proxy) {
      return this.snapshot();
    }
    proxy.lastSuccessAt = Math.floor(Date.now() / 1000);
    proxy.successCount = Number(proxy.successCount || 0) + 1;
    proxy.useCount = Number(proxy.useCount || 0) + 1;
    proxy.lastError = '';
    proxy.failureCount = Math.max(0, Number(proxy.failureCount || 0) - 1);
    proxy.cooldownUntil = 0;
    return this.save();
  }

  applyEnvDefaults() {
    const settings = this.data.settings || {};
    const envBaseURL = desktopAutomationBaseURL();
    const currentBaseURL = String(settings.automationBaseUrl || '').trim();
    this.data.settings = {
      ...settings,
      automationBaseUrl: !currentBaseURL || currentBaseURL === 'http://127.0.0.1:8091' ? envBaseURL : currentBaseURL,
      desktopToken: settings.desktopToken || envString('AUTOMATION_DESKTOP_TOKEN', ''),
      workerId: settings.workerId || envString('DESKTOP_WORKER_ID', 'desktop-client-1'),
      maxConcurrency: Number(settings.maxConcurrency || envString('DESKTOP_MAX_CONCURRENCY', '3')),
      callbackPort: Number(settings.callbackPort || envString('BROWSER_CALLBACK_PORT', '1455')),
    };
  }
}

function envString(name, fallback) {
  const value = String(process.env[name] || '').trim();
  return value || fallback;
}

function desktopAutomationBaseURL() {
  const explicit = envString('DESKTOP_AUTOMATION_BASE_URL', '');
  if (explicit) {
    return explicit;
  }
  const listenAddr = envString('AUTOMATION_LISTEN_ADDR', ':18091');
  const port = listenAddr.match(/:(\d+)$/)?.[1] || '18091';
  return `http://127.0.0.1:${port}`;
}

function sanitizeSettings(value = {}) {
  const next = { ...value };
  if (next.automationBaseUrl) {
    next.automationBaseUrl = String(next.automationBaseUrl).replace(/\/+$/, '');
  }
  if (next.maxConcurrency !== undefined) {
    next.maxConcurrency = Math.max(1, Math.min(8, Number(next.maxConcurrency) || 3));
  }
  if (next.callbackPort !== undefined) {
    next.callbackPort = Math.max(1024, Math.min(65535, Number(next.callbackPort) || 1455));
  }
  return next;
}

function normalizeProxy(proxy = {}) {
  return {
    id: String(proxy.id || '').trim(),
    name: String(proxy.name || '').trim(),
    proxyRules: String(proxy.proxyRules || '').trim(),
    enabled: proxy.enabled !== false,
    lastError: String(proxy.lastError || '').trim(),
    failureCount: Number(proxy.failureCount || 0),
    successCount: Number(proxy.successCount || 0),
    useCount: Number(proxy.useCount || 0),
    cooldownUntil: Number(proxy.cooldownUntil || 0),
    lastFailureAt: Number(proxy.lastFailureAt || 0),
    lastSuccessAt: Number(proxy.lastSuccessAt || 0),
  };
}

function normalizeRemoteProxy(proxy = {}, now) {
  const id = proxy.id ? `remote-${proxy.id}` : '';
  const region = [proxy.regionCode || proxy.region_code, proxy.city].filter(Boolean).join(' / ');
  return {
    id,
    remoteId: Number(proxy.id || 0),
    source: 'main_gateway',
    name: String(proxy.name || id).trim(),
    proxyRules: String(proxy.proxyRules || proxy.proxy_rules || '').trim(),
    maskedAddress: String(proxy.maskedAddress || proxy.masked_address || '').trim(),
    protocol: String(proxy.protocol || '').trim(),
    enabled: proxy.enabled !== false,
    remark: String(proxy.remark || '').trim(),
    region,
    exitIp: String(proxy.exitIP || proxy.exit_ip || '').trim(),
    geoStatus: String(proxy.geoStatus || proxy.geo_status || '').trim(),
    failureCount: Number(proxy.failureCount || proxy.failure_count || 0),
    successCount: Number(proxy.successCount || proxy.success_count || 0),
    useCount: Number(proxy.useCount || proxy.use_count || 0),
    lastFailureAt: Number(proxy.lastFailureAt || proxy.last_failure_at || 0),
    lastSuccessAt: Number(proxy.lastSuccessAt || proxy.last_success_at || 0),
    lastError: '',
    cooldownUntil: 0,
    syncedAt: now,
  };
}

function enrichProxy(proxy = {}, now = Math.floor(Date.now() / 1000)) {
  const enabled = proxy.enabled !== false;
  const cooldownUntil = Number(proxy.cooldownUntil || 0);
  let status = 'available';
  if (!enabled) {
    status = 'disabled';
  } else if (cooldownUntil > now) {
    status = 'cooling';
  }
  const failureCount = Number(proxy.failureCount || 0);
  const successCount = Number(proxy.successCount || 0);
  const useCount = Number(proxy.useCount || 0);
  const lastSuccessAt = Number(proxy.lastSuccessAt || 0);
  const lastFailureAt = Number(proxy.lastFailureAt || 0);
  let score = enabled ? 80 : 0;
  const reasons = [];
  if (!enabled) {
    reasons.push('disabled');
  }
  if (status === 'cooling') {
    score -= 55;
    reasons.push(`cooling ${formatRelativeSeconds(cooldownUntil - now)}`);
  }
  if (proxy.source === 'main_gateway') {
    score += 6;
    reasons.push('main gateway');
  }
  if (lastSuccessAt > 0) {
    const age = now - lastSuccessAt;
    if (age < 3600) {
      score += 18;
      reasons.push('recent success');
    } else if (age < 86400) {
      score += 10;
      reasons.push('success today');
    } else {
      score += 4;
    }
  }
  if (lastFailureAt > 0) {
    const age = now - lastFailureAt;
    if (age < 1800) {
      score -= 22;
      reasons.push('recent failure');
    } else if (age < 86400) {
      score -= 10;
      reasons.push('failure today');
    }
  }
  if (failureCount > 0) {
    score -= Math.min(45, failureCount * 9);
    reasons.push(`${failureCount} failures`);
  }
  if (successCount > 0) {
    score += Math.min(14, successCount * 2);
  }
  if (useCount > 20) {
    score -= Math.min(8, Math.floor(useCount / 20));
  }
  if (proxy.geoStatus && !['ok', 'success', 'valid'].includes(String(proxy.geoStatus).toLowerCase())) {
    score -= 8;
    reasons.push(`geo ${proxy.geoStatus}`);
  }
  score = Math.max(0, Math.min(100, Math.round(score)));
  if (status !== 'available') {
    score = Math.min(score, status === 'cooling' ? 35 : 0);
  }
  return {
    ...proxy,
    healthScore: score,
    proxyStatus: status,
    healthReason: reasons.slice(0, 4).join(' · ') || 'healthy baseline',
  };
}

function summarizeProxyHealth(proxies = []) {
  const summary = {
    total: proxies.length,
    available: 0,
    cooling: 0,
    disabled: 0,
    bestProxyId: '',
    bestScore: 0,
  };
  for (const proxy of proxies) {
    if (proxy.proxyStatus === 'available') {
      summary.available += 1;
      if (Number(proxy.healthScore || 0) > summary.bestScore) {
        summary.bestScore = Number(proxy.healthScore || 0);
        summary.bestProxyId = proxy.id || '';
      }
    } else if (proxy.proxyStatus === 'cooling') {
      summary.cooling += 1;
    } else {
      summary.disabled += 1;
    }
  }
  return summary;
}

function formatRelativeSeconds(seconds) {
  seconds = Math.max(0, Number(seconds || 0));
  if (seconds >= 3600) {
    return `${Math.ceil(seconds / 3600)}h`;
  }
  if (seconds >= 60) {
    return `${Math.ceil(seconds / 60)}m`;
  }
  return `${Math.ceil(seconds)}s`;
}
