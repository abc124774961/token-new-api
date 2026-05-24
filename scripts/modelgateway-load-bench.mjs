#!/usr/bin/env node

import fs from 'node:fs/promises';
import path from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';

const DEFAULT_BASE_URL = 'http://localhost:3000/v1';
const DEFAULT_PROMPT = 'Reply with one short sentence for a model gateway streaming benchmark.';
const DEFAULT_TIMEOUT_MS = 180_000;
const DEFAULT_MAX_TOKENS = 64;
const MAX_SAMPLE_BYTES = 4096;

const SCENARIOS = {
  stream100: {
    total: 100,
    batchSize: 100,
    batchIntervalMs: 0,
    description: '100 concurrent streaming requests',
  },
  batch200: {
    total: 200,
    batchSize: 20,
    batchIntervalMs: 20_000,
    description: '200 streaming requests, 20 requests per batch, one batch every 20 seconds',
  },
  custom: {
    total: 1,
    batchSize: 1,
    batchIntervalMs: 0,
    description: 'custom totals from env or flags',
  },
};

function usage() {
  console.log(`Usage:
  node scripts/modelgateway-load-bench.mjs [options]

Default behavior is dry-run only. No HTTP requests are sent unless --run is set.

Scenarios:
  stream100              100 concurrent streaming requests
  batch200               200 requests, 20 concurrent per batch, one batch every 20 seconds
  custom                 Use --total, --batch-size, and --batch-interval-ms

Options:
  --run                  Execute requests. Without this, only prints the plan.
  --dry-run              Force dry-run.
  --scenario <name>      stream100 | batch200 | custom (env: MODEL_GATEWAY_BENCH_SCENARIO)
  --endpoint <style>     chat | responses (env: MODEL_GATEWAY_BENCH_ENDPOINT)
  --base-url <url>       OpenAI-compatible base URL, usually ending in /v1
  --url <url>            Full endpoint URL override
  --model <model>        Model name (env: MODEL_GATEWAY_BENCH_MODEL)
  --total <n>            Total request count for custom or overrides
  --batch-size <n>       Requests launched per batch
  --concurrency <n>      Alias for --batch-size
  --batch-interval-ms <n> Delay between batch starts
  --timeout-ms <n>       Per-request timeout (default: ${DEFAULT_TIMEOUT_MS})
  --max-tokens <n>       max_tokens/max_output_tokens (default: ${DEFAULT_MAX_TOKENS})
  --prompt <text>        Prompt text
  --report <path>        Optional JSON report path
  --allow-remote         Allow --run against non-localhost URLs
  --no-auth              Do not require or send Authorization
  --verbose              Print one line per request result
  -h, --help             Show this help

Environment:
  MODEL_GATEWAY_BENCH_API_KEY          Bearer token. Never hard-code this.
  OPENAI_API_KEY                       Fallback bearer token.
  MODEL_GATEWAY_BENCH_BASE_URL         Default: ${DEFAULT_BASE_URL}
  MODEL_GATEWAY_BENCH_URL              Full endpoint URL override.
  MODEL_GATEWAY_BENCH_MODEL            Required for --run.
  MODEL_GATEWAY_BENCH_ENDPOINT         chat or responses.
  MODEL_GATEWAY_BENCH_EXTRA_BODY_JSON  Shallow-merged JSON object for request body overrides.
  MODEL_GATEWAY_BENCH_HEADERS_JSON     Extra HTTP headers as a JSON object.
  MODEL_GATEWAY_BENCH_ALLOW_REMOTE=1   Required for non-localhost --run unless --allow-remote is used.
  MODEL_GATEWAY_BENCH_NO_AUTH=1        Same as --no-auth.

Examples:
  node scripts/modelgateway-load-bench.mjs --scenario stream100

  MODEL_GATEWAY_BENCH_BASE_URL=http://127.0.0.1:3000/v1 \\
  MODEL_GATEWAY_BENCH_API_KEY=sk-local \\
  MODEL_GATEWAY_BENCH_MODEL=gpt-test \\
    node scripts/modelgateway-load-bench.mjs --scenario batch200 --endpoint chat --run
`);
}

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '-h' || arg === '--help') {
      usage();
      process.exit(0);
    }
    if (arg === '--run') {
      args.run = true;
      continue;
    }
    if (arg === '--dry-run') {
      args.dryRun = true;
      continue;
    }
    if (arg === '--allow-remote') {
      args.allowRemote = true;
      continue;
    }
    if (arg === '--no-auth') {
      args.noAuth = true;
      continue;
    }
    if (arg === '--verbose') {
      args.verbose = true;
      continue;
    }
    const valueFlags = new Set([
      '--scenario',
      '--endpoint',
      '--base-url',
      '--url',
      '--model',
      '--total',
      '--batch-size',
      '--concurrency',
      '--batch-interval-ms',
      '--timeout-ms',
      '--max-tokens',
      '--prompt',
      '--report',
    ]);
    if (valueFlags.has(arg)) {
      if (i + 1 >= argv.length) throw new Error(`${arg} requires a value`);
      args[toCamel(arg.slice(2))] = argv[++i];
      continue;
    }
    throw new Error(`unknown option: ${arg}`);
  }
  return args;
}

function toCamel(value) {
  return value.replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
}

function env(name, fallback = '') {
  const value = process.env[name];
  return value === undefined || value === '' ? fallback : value;
}

function boolEnv(name) {
  return ['1', 'true', 'yes', 'on'].includes(String(process.env[name] || '').toLowerCase());
}

function positiveInt(value, name) {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }
  return parsed;
}

function nonNegativeInt(value, name) {
  const parsed = Number(value);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error(`${name} must be a non-negative integer`);
  }
  return parsed;
}

function optionalNumber(value, name) {
  if (value === undefined || value === '') return undefined;
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) throw new Error(`${name} must be a number`);
  return parsed;
}

function parseJsonObject(raw, name) {
  if (!raw) return {};
  let parsed;
  try {
    parsed = JSON.parse(raw);
  } catch (error) {
    throw new Error(`${name} is not valid JSON: ${error.message}`);
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(`${name} must be a JSON object`);
  }
  return parsed;
}

function normalizeEndpoint(value) {
  const endpoint = String(value || 'chat').toLowerCase();
  if (['chat', 'chat/completions', 'completions'].includes(endpoint)) return 'chat';
  if (['responses', 'response'].includes(endpoint)) return 'responses';
  throw new Error(`unsupported endpoint style: ${value}`);
}

function endpointPath(endpoint) {
  return endpoint === 'responses' ? '/responses' : '/chat/completions';
}

function joinUrl(baseURL, suffix) {
  return `${String(baseURL).replace(/\/+$/, '')}/${String(suffix).replace(/^\/+/, '')}`;
}

function isLocalURL(url) {
  const parsed = new URL(url);
  const hostname = parsed.hostname.toLowerCase();
  return hostname === 'localhost'
    || hostname.endsWith('.localhost')
    || hostname === '127.0.0.1'
    || hostname === '0.0.0.0'
    || hostname === '::1';
}

function configFromArgs(args) {
  const scenario = String(args.scenario || env('MODEL_GATEWAY_BENCH_SCENARIO', 'stream100')).toLowerCase();
  if (!SCENARIOS[scenario]) throw new Error(`unsupported scenario: ${scenario}`);
  const defaults = SCENARIOS[scenario];
  const endpoint = normalizeEndpoint(args.endpoint || env('MODEL_GATEWAY_BENCH_ENDPOINT', 'chat'));
  const baseURL = args.baseUrl || env('MODEL_GATEWAY_BENCH_BASE_URL', DEFAULT_BASE_URL);
  const requestURL = args.url || env('MODEL_GATEWAY_BENCH_URL') || joinUrl(baseURL, endpointPath(endpoint));
  const apiKey = env('MODEL_GATEWAY_BENCH_API_KEY') || env('OPENAI_API_KEY');
  const noAuth = Boolean(args.noAuth || boolEnv('MODEL_GATEWAY_BENCH_NO_AUTH'));
  const total = positiveInt(args.total || env('MODEL_GATEWAY_BENCH_TOTAL', defaults.total), 'total');
  const batchSize = positiveInt(
    args.batchSize || args.concurrency || env('MODEL_GATEWAY_BENCH_BATCH_SIZE', defaults.batchSize),
    'batch size',
  );
  const batchIntervalMs = nonNegativeInt(
    args.batchIntervalMs || env('MODEL_GATEWAY_BENCH_BATCH_INTERVAL_MS', defaults.batchIntervalMs),
    'batch interval',
  );
  const timeoutMs = positiveInt(args.timeoutMs || env('MODEL_GATEWAY_BENCH_TIMEOUT_MS', DEFAULT_TIMEOUT_MS), 'timeout');
  const maxTokens = positiveInt(args.maxTokens || env('MODEL_GATEWAY_BENCH_MAX_TOKENS', DEFAULT_MAX_TOKENS), 'max tokens');
  const temperature = optionalNumber(env('MODEL_GATEWAY_BENCH_TEMPERATURE'), 'temperature');
  const extraBody = parseJsonObject(env('MODEL_GATEWAY_BENCH_EXTRA_BODY_JSON'), 'MODEL_GATEWAY_BENCH_EXTRA_BODY_JSON');
  const extraHeaders = parseJsonObject(env('MODEL_GATEWAY_BENCH_HEADERS_JSON'), 'MODEL_GATEWAY_BENCH_HEADERS_JSON');
  const run = Boolean(args.run || boolEnv('MODEL_GATEWAY_BENCH_RUN')) && !args.dryRun;

  return {
    scenario,
    scenarioDescription: defaults.description,
    endpoint,
    baseURL,
    requestURL,
    apiKey,
    noAuth,
    model: args.model || env('MODEL_GATEWAY_BENCH_MODEL'),
    total,
    batchSize,
    batchIntervalMs,
    timeoutMs,
    maxTokens,
    temperature,
    prompt: args.prompt || env('MODEL_GATEWAY_BENCH_PROMPT', DEFAULT_PROMPT),
    reportPath: args.report || env('MODEL_GATEWAY_BENCH_REPORT'),
    allowRemote: Boolean(args.allowRemote || boolEnv('MODEL_GATEWAY_BENCH_ALLOW_REMOTE')),
    verbose: Boolean(args.verbose || boolEnv('MODEL_GATEWAY_BENCH_VERBOSE')),
    includeErrorSample: boolEnv('MODEL_GATEWAY_BENCH_INCLUDE_ERROR_SAMPLE'),
    extraBody,
    extraHeaders,
    run,
    runID: `mgb-${new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14)}`,
  };
}

function validateForRun(config) {
  if (typeof fetch !== 'function') {
    throw new Error('global fetch is not available; use Node 18+ or Bun');
  }
  if (!config.model) {
    throw new Error('MODEL_GATEWAY_BENCH_MODEL or --model is required for --run');
  }
  if (!config.noAuth && !config.apiKey) {
    throw new Error('MODEL_GATEWAY_BENCH_API_KEY is required for --run unless --no-auth is set');
  }
  if (!isLocalURL(config.requestURL) && !config.allowRemote) {
    throw new Error('refusing non-localhost --run; set MODEL_GATEWAY_BENCH_ALLOW_REMOTE=1 or pass --allow-remote after explicit approval');
  }
}

function dryRun(config) {
  console.log('DRY RUN: no HTTP requests will be sent.');
  console.log(`scenario=${config.scenario} (${config.scenarioDescription})`);
  console.log(`endpoint=${config.endpoint} url=${config.requestURL}`);
  console.log(`model=${config.model || '<MODEL_GATEWAY_BENCH_MODEL required for --run>'}`);
  console.log(`total=${config.total} batch_size=${config.batchSize} batch_interval_ms=${config.batchIntervalMs}`);
  console.log(`timeout_ms=${config.timeoutMs} max_tokens=${config.maxTokens}`);
  console.log(`auth=${config.noAuth ? 'disabled' : (config.apiKey ? 'api key set' : 'api key missing')}`);
  console.log(`remote_guard=${isLocalURL(config.requestURL) ? 'local URL' : 'remote URL requires MODEL_GATEWAY_BENCH_ALLOW_REMOTE=1'}`);
  console.log('\nAdd --run to execute against a local gateway or mock.');
}

function requestBody(config, index) {
  const prompt = `${config.prompt} request=${index} run=${config.runID}`;
  const common = {
    model: config.model,
    stream: true,
  };
  if (config.temperature !== undefined) common.temperature = config.temperature;
  if (config.endpoint === 'responses') {
    return {
      ...common,
      input: prompt,
      max_output_tokens: config.maxTokens,
      ...config.extraBody,
    };
  }
  return {
    ...common,
    messages: [{ role: 'user', content: prompt }],
    max_tokens: config.maxTokens,
    ...config.extraBody,
  };
}

function requestHeaders(config, index, batchIndex) {
  const headers = {
    'content-type': 'application/json',
    'user-agent': 'new-api-modelgateway-load-bench/1.0',
    'x-model-gateway-bench-run': config.runID,
    'x-model-gateway-bench-index': String(index),
    'x-model-gateway-bench-batch': String(batchIndex),
    ...config.extraHeaders,
  };
  if (!config.noAuth && config.apiKey) {
    headers.authorization = `Bearer ${config.apiKey}`;
  }
  return headers;
}

async function runOne(config, index, batchIndex) {
  const startedAtMs = Date.now();
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), config.timeoutMs);
  const result = {
    index,
    batch: batchIndex,
    ok: false,
    status: 0,
    statusText: '',
    requestID: '',
    ttftMs: 0,
    totalMs: 0,
    bytes: 0,
    errorKind: 'unknown',
    errorMessage: '',
    startedAt: new Date(startedAtMs).toISOString(),
    finishedAt: '',
  };

  let bodySample = '';
  try {
    const response = await fetch(config.requestURL, {
      method: 'POST',
      headers: requestHeaders(config, index, batchIndex),
      body: JSON.stringify(requestBody(config, index)),
      signal: controller.signal,
    });
    result.status = response.status;
    result.statusText = response.statusText;
    result.requestID = firstHeader(response.headers, [
      'x-oneapi-request-id',
      'x-request-id',
      'x-newapi-request-id',
      'new-api-request-id',
      'x-ratelimit-request-id',
    ]);
    bodySample = await readBody(response, result, startedAtMs);
  } catch (error) {
    if (error?.name === 'AbortError') {
      result.errorMessage = `timeout after ${config.timeoutMs}ms`;
    } else {
      result.errorMessage = error?.message || String(error);
    }
  } finally {
    clearTimeout(timeout);
    const finishedAtMs = Date.now();
    result.totalMs = result.totalMs || finishedAtMs - startedAtMs;
    result.ttftMs = result.ttftMs || result.totalMs;
    result.finishedAt = new Date(finishedAtMs).toISOString();
  }

  result.errorKind = classifyResult(result, bodySample);
  result.ok = result.errorKind === 'success';
  if (config.includeErrorSample && !result.ok && bodySample) {
    result.errorSample = bodySample;
  }
  return result;
}

function firstHeader(headers, names) {
  for (const name of names) {
    const value = headers.get(name);
    if (value) return value;
  }
  return '';
}

async function readBody(response, result, startedAtMs) {
  if (!response.body || typeof response.body.getReader !== 'function') {
    const text = await response.text();
    result.bytes = Buffer.byteLength(text);
    result.ttftMs = Date.now() - startedAtMs;
    result.totalMs = Date.now() - startedAtMs;
    return text.slice(0, MAX_SAMPLE_BYTES);
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  const streamState = { buffer: '' };
  let sample = '';
  let sawFirstByte = false;
  let sawTokenSignal = false;

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    if (!value || value.byteLength === 0) continue;
    const now = Date.now();
    if (!sawFirstByte) {
      result.ttftMs = now - startedAtMs;
      sawFirstByte = true;
    }
    result.bytes += value.byteLength;
    const text = decoder.decode(value, { stream: true });
    if (sample.length < MAX_SAMPLE_BYTES) {
      sample += text.slice(0, MAX_SAMPLE_BYTES - sample.length);
    }
    if (!sawTokenSignal && hasStreamTokenSignal(text, streamState)) {
      result.ttftMs = now - startedAtMs;
      sawTokenSignal = true;
    }
  }
  const rest = decoder.decode();
  if (rest && sample.length < MAX_SAMPLE_BYTES) {
    sample += rest.slice(0, MAX_SAMPLE_BYTES - sample.length);
  }
  result.totalMs = Date.now() - startedAtMs;
  return sample;
}

function hasStreamTokenSignal(text, state) {
  state.buffer += text;
  const lines = state.buffer.split(/\r?\n/);
  state.buffer = lines.pop() || '';
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    if (!trimmed.startsWith('data:')) {
      return true;
    }
    const payload = trimmed.slice(5).trim();
    if (!payload || payload === '[DONE]') continue;
    if (payloadLooksLikeToken(payload)) return true;
  }
  return false;
}

function payloadLooksLikeToken(payload) {
  if (!payload.startsWith('{')) return true;
  try {
    const parsed = JSON.parse(payload);
    if (Array.isArray(parsed?.choices)) {
      return parsed.choices.some((choice) => {
        const delta = choice?.delta || choice?.message || {};
        return nonEmptyString(delta.content)
          || nonEmptyString(delta.reasoning_content)
          || nonEmptyString(choice?.text);
      });
    }
    if (nonEmptyString(parsed?.delta)) return true;
    if (nonEmptyString(parsed?.text)) return true;
    if (nonEmptyString(parsed?.output_text)) return true;
    if (String(parsed?.type || '').includes('output_text.delta') && nonEmptyString(parsed?.delta)) return true;
    return false;
  } catch {
    return true;
  }
}

function nonEmptyString(value) {
  return typeof value === 'string' && value.trim() !== '';
}

function classifyResult(result, bodySample) {
  const haystack = `${result.statusText}\n${result.errorMessage}\n${bodySample}`.toLowerCase();
  if (result.errorMessage.includes('timeout after')) return 'timeout';
  if (result.errorMessage) return result.status > 0 ? 'stream_interrupted' : 'network_error';
  if (result.status === 429 || /too many pending|concurrency limit|rate.?limit|overload_skip/.test(haystack)) {
    return 'rate_limit_429';
  }
  if (result.status === 401
    || result.status === 403
    || /auth_config_error|invalid api key|permission denied|model not allowed|forbidden|unauthorized/.test(haystack)) {
    return 'auth_config_error';
  }
  if (result.status >= 500) return 'server_error';
  if (result.status >= 400) return 'http_error';
  if (result.status >= 200 && result.status < 300) return 'success';
  return 'unknown';
}

async function runScenario(config) {
  validateForRun(config);
  const startedAtMs = Date.now();
  const tasks = [];
  const totalBatches = Math.ceil(config.total / config.batchSize);
  console.log(`RUN: scenario=${config.scenario} endpoint=${config.endpoint} url=${config.requestURL}`);
  console.log(`run_id=${config.runID} total=${config.total} batch_size=${config.batchSize} batch_interval_ms=${config.batchIntervalMs}`);

  for (let batchIndex = 0; batchIndex < totalBatches; batchIndex += 1) {
    const batchStartOffset = batchIndex * config.batchIntervalMs;
    const waitMs = startedAtMs + batchStartOffset - Date.now();
    if (waitMs > 0) await sleep(waitMs);
    const startIndex = batchIndex * config.batchSize;
    const endIndex = Math.min(config.total, startIndex + config.batchSize);
    console.log(`[batch ${batchIndex + 1}/${totalBatches}] launch indexes ${startIndex}..${endIndex - 1}`);
    for (let index = startIndex; index < endIndex; index += 1) {
      tasks.push(runOne(config, index, batchIndex));
    }
  }

  const settled = await Promise.allSettled(tasks);
  const results = settled.map((item, index) => {
    if (item.status === 'fulfilled') return item.value;
    return {
      index,
      batch: -1,
      ok: false,
      status: 0,
      statusText: '',
      requestID: '',
      ttftMs: 0,
      totalMs: 0,
      bytes: 0,
      errorKind: 'network_error',
      errorMessage: item.reason?.message || String(item.reason),
      startedAt: '',
      finishedAt: '',
    };
  }).sort((a, b) => a.index - b.index);

  const summary = summarize(config, results, Date.now() - startedAtMs);
  if (config.verbose) printResults(results);
  printSummary(summary);
  if (config.reportPath) await writeReport(config, summary, results);
}

function summarize(config, results, wallMs) {
  const statusCounts = countBy(results, (result) => (result.status ? String(result.status) : 'NO_RESPONSE'));
  const errorKindCounts = countBy(results, (result) => result.errorKind);
  const successes = results.filter((result) => result.ok);
  const ttftValues = successes.map((result) => result.ttftMs).filter(Number.isFinite);
  const totalValues = results.map((result) => result.totalMs).filter(Number.isFinite);
  return {
    scenario: config.scenario,
    endpoint: config.endpoint,
    request_url: config.requestURL,
    run_id: config.runID,
    total: results.length,
    success: successes.length,
    failure: results.length - successes.length,
    success_rate: results.length ? successes.length / results.length : 0,
    wall_ms: wallMs,
    status_counts: statusCounts,
    error_kind_counts: errorKindCounts,
    rate_limit_429_count: errorKindCounts.rate_limit_429 || 0,
    auth_config_error_count: errorKindCounts.auth_config_error || 0,
    ttft_ms_success: distribution(ttftValues),
    total_ms_all: distribution(totalValues),
  };
}

function countBy(values, keyFn) {
  return values.reduce((acc, value) => {
    const key = keyFn(value);
    acc[key] = (acc[key] || 0) + 1;
    return acc;
  }, {});
}

function distribution(values) {
  const sorted = values.slice().sort((a, b) => a - b);
  if (!sorted.length) {
    return { count: 0, min: 0, avg: 0, p50: 0, p90: 0, p95: 0, max: 0 };
  }
  const sum = sorted.reduce((acc, value) => acc + value, 0);
  return {
    count: sorted.length,
    min: Math.round(sorted[0]),
    avg: Math.round(sum / sorted.length),
    p50: Math.round(percentile(sorted, 50)),
    p90: Math.round(percentile(sorted, 90)),
    p95: Math.round(percentile(sorted, 95)),
    max: Math.round(sorted[sorted.length - 1]),
  };
}

function percentile(sorted, p) {
  if (!sorted.length) return 0;
  const rank = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, Math.min(sorted.length - 1, rank))];
}

function formatCounts(counts) {
  const entries = Object.entries(counts).sort(([a], [b]) => a.localeCompare(b));
  return entries.length ? entries.map(([key, value]) => `${key}:${value}`).join(', ') : '-';
}

function formatDistribution(stats) {
  if (!stats.count) return 'count=0';
  return `count=${stats.count} min=${stats.min} avg=${stats.avg} p50=${stats.p50} p90=${stats.p90} p95=${stats.p95} max=${stats.max}`;
}

function printResults(results) {
  for (const result of results) {
    console.log([
      `idx=${result.index}`,
      `batch=${result.batch}`,
      `status=${result.status || 'NO_RESPONSE'}`,
      `kind=${result.errorKind}`,
      `ttft_ms=${Math.round(result.ttftMs)}`,
      `total_ms=${Math.round(result.totalMs)}`,
      `bytes=${result.bytes}`,
      `request_id=${result.requestID || '-'}`,
      result.errorMessage ? `error=${result.errorMessage}` : '',
    ].filter(Boolean).join(' '));
  }
}

function printSummary(summary) {
  console.log('\nSummary');
  console.log(`total=${summary.total} success=${summary.success} failure=${summary.failure} success_rate=${(summary.success_rate * 100).toFixed(2)}% wall_ms=${summary.wall_ms}`);
  console.log(`status_counts=${formatCounts(summary.status_counts)}`);
  console.log(`error_kind_counts=${formatCounts(summary.error_kind_counts)}`);
  console.log(`rate_limit_429=${summary.rate_limit_429_count} auth_config_error=${summary.auth_config_error_count}`);
  console.log(`ttft_ms_success ${formatDistribution(summary.ttft_ms_success)}`);
  console.log(`total_ms_all ${formatDistribution(summary.total_ms_all)}`);
}

async function writeReport(config, summary, results) {
  const reportPath = path.resolve(config.reportPath);
  await fs.mkdir(path.dirname(reportPath), { recursive: true });
  const report = {
    kind: 'modelgateway_load_bench_report',
    created_at: new Date().toISOString(),
    config: sanitizedConfig(config),
    summary,
    results,
  };
  await fs.writeFile(reportPath, `${JSON.stringify(report, null, 2)}\n`);
  console.log(`report=${reportPath}`);
}

function sanitizedConfig(config) {
  return {
    scenario: config.scenario,
    endpoint: config.endpoint,
    base_url: config.baseURL,
    request_url: config.requestURL,
    model: config.model,
    total: config.total,
    batch_size: config.batchSize,
    batch_interval_ms: config.batchIntervalMs,
    timeout_ms: config.timeoutMs,
    max_tokens: config.maxTokens,
    stream: true,
    auth: config.noAuth ? 'disabled' : (config.apiKey ? 'api key set' : 'api key missing'),
    allow_remote: config.allowRemote,
    extra_header_keys: Object.keys(config.extraHeaders || {}).sort(),
    extra_body_keys: Object.keys(config.extraBody || {}).sort(),
  };
}

try {
  const args = parseArgs(process.argv.slice(2));
  const config = configFromArgs(args);
  if (!config.run) {
    dryRun(config);
  } else {
    await runScenario(config);
  }
} catch (error) {
  console.error(`modelgateway-load-bench: ${error.message}`);
  process.exit(1);
}
