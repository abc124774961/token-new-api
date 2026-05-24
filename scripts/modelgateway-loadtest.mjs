#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';

const DEFAULT_BODY = 'Reply with one short sentence for model gateway load testing.';
const MAX_SAMPLE_BYTES = 4096;

function env(name, fallback = '') {
  return process.env[name] || fallback;
}

function numberEnv(name, fallback) {
  const value = Number(process.env[name]);
  return Number.isFinite(value) && value > 0 ? value : fallback;
}

function boolEnv(name, fallback = false) {
  const value = process.env[name];
  if (value === undefined || value === '') return fallback;
  return ['1', 'true', 'yes', 'on'].includes(String(value).toLowerCase());
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function percentile(values, p) {
  const sorted = values.filter((value) => Number.isFinite(value)).sort((a, b) => a - b);
  if (!sorted.length) return 0;
  const index = Math.min(sorted.length - 1, Math.max(0, Math.ceil(sorted.length * p) - 1));
  return sorted[index];
}

function buildPayload(index, batchName) {
  const model = env('MODEL', 'gpt-5.5');
  const prompt = env('PROMPT', `${DEFAULT_BODY} batch=${batchName} index=${index}`);
  const maxTokens = numberEnv('MAX_TOKENS', 64);
  return {
    model,
    stream: env('STREAM', 'true') !== 'false',
    messages: [{ role: 'user', content: prompt }],
    max_tokens: maxTokens,
  };
}

function baseURL() {
  return env('BASE_URL', 'http://localhost:3001/v1').replace(/\/+$/, '');
}

function firstHeader(headers, names) {
  for (const name of names) {
    const value = headers.get(name);
    if (value) return value;
  }
  return '';
}

function classifyResult(result, bodySample) {
  const haystack = `${result.statusText}\n${result.error}\n${bodySample}`.toLowerCase();
  if (/timeout after/.test(result.error)) return 'timeout';
  if (result.error) return result.status > 0 ? 'stream_interrupted' : 'network_error';
  if (result.status === 429 || /overload_skip|too many pending|concurrency limit|rate.?limit/.test(haystack)) {
    return 'rate_limit_429';
  }
  if (
    result.status === 401 ||
    result.status === 403 ||
    /auth_config_error|invalid api key|permission denied|model not allowed|forbidden|unauthorized/.test(haystack)
  ) {
    return 'auth_config_error';
  }
  if (result.status >= 500) return 'server_error';
  if (result.status >= 400) return 'http_error';
  if (result.status >= 200 && result.status < 300) return 'success';
  return 'unknown';
}

function sanitizeConfig(config) {
  return {
    scenario: config.scenario,
    batch_name: config.batchName,
    base_url: config.baseURL,
    model: config.model,
    stream: config.stream,
    timeout_ms: config.timeoutMs,
    include_error_sample: config.includeErrorSample,
    report_path: config.reportPath,
    auth: config.apiKey ? 'api key set' : 'api key missing',
  };
}

async function runOne(index, batchName) {
  const apiKey = env('API_KEY');
  if (!apiKey) throw new Error('API_KEY is required');
  const startedAt = Date.now();
  const timeoutMs = numberEnv('TIMEOUT_MS', 180000);
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), timeoutMs);
  const result = {
    index,
    batchName,
    status: 0,
    statusText: '',
    ok: false,
    bytes: 0,
    ttftMs: 0,
    totalMs: 0,
    requestId: '',
    errorKind: 'unknown',
    error: '',
  };
  let bodySample = '';

  try {
    const response = await fetch(`${baseURL()}/chat/completions`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${apiKey}`,
        'Content-Type': 'application/json',
        'User-Agent': 'modelgateway-loadtest/1.0',
        'X-Model-Gateway-Loadtest': batchName,
        'X-Model-Gateway-Loadtest-Index': String(index),
      },
      body: JSON.stringify(buildPayload(index, batchName)),
      signal: controller.signal,
    });
    result.status = response.status;
    result.statusText = response.statusText;
    result.requestId = firstHeader(response.headers, [
      'x-oneapi-request-id',
      'x-request-id',
      'x-newapi-request-id',
      'new-api-request-id',
      'x-ratelimit-request-id',
    ]);

    if (!response.body) {
      const text = await response.text();
      result.ttftMs = Date.now() - startedAt;
      result.bytes = Buffer.byteLength(text);
      bodySample = text.slice(0, MAX_SAMPLE_BYTES);
    } else {
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let firstChunk = true;
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        if (firstChunk) {
          result.ttftMs = Date.now() - startedAt;
          firstChunk = false;
        }
        result.bytes += value?.byteLength || 0;
        if (value && bodySample.length < MAX_SAMPLE_BYTES) {
          const text = decoder.decode(value, { stream: true });
          bodySample += text.slice(0, MAX_SAMPLE_BYTES - bodySample.length);
        }
      }
      const rest = decoder.decode();
      if (rest && bodySample.length < MAX_SAMPLE_BYTES) {
        bodySample += rest.slice(0, MAX_SAMPLE_BYTES - bodySample.length);
      }
      if (firstChunk) result.ttftMs = Date.now() - startedAt;
    }
  } catch (error) {
    result.error = error?.name === 'AbortError'
      ? `timeout after ${timeoutMs}ms`
      : error?.message || String(error);
  } finally {
    clearTimeout(timeout);
    result.totalMs = Date.now() - startedAt;
  }
  result.errorKind = classifyResult(result, bodySample);
  result.ok = result.errorKind === 'success';
  if (boolEnv('INCLUDE_ERROR_SAMPLE') && !result.ok && bodySample) {
    result.errorSample = bodySample;
  }
  return result;
}

async function runPool(total, concurrency, batchName, offset = 0) {
  const results = [];
  let cursor = 0;
  const workers = Array.from({ length: concurrency }, async () => {
    for (;;) {
      const current = cursor;
      cursor += 1;
      if (current >= total) return;
      results[current] = await runOne(offset + current, batchName);
    }
  });
  await Promise.all(workers);
  return results;
}

function printSummary(results, batchName) {
  const total = results.length;
  const successes = results.filter((item) => item.ok).length;
  const statusCounts = new Map();
  const errorKindCounts = new Map();
  for (const item of results) {
    statusCounts.set(item.status, (statusCounts.get(item.status) || 0) + 1);
    errorKindCounts.set(item.errorKind, (errorKindCounts.get(item.errorKind) || 0) + 1);
  }
  const ttftValues = results.map((item) => item.ttftMs).filter((value) => value > 0);
  const totalValues = results.map((item) => item.totalMs).filter((value) => value > 0);
  const statusText = [...statusCounts.entries()]
    .sort(([left], [right]) => left - right)
    .map(([status, count]) => `${status}:${count}`)
    .join(' ');
  const errorKindText = [...errorKindCounts.entries()]
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([kind, count]) => `${kind}:${count}`)
    .join(' ');
  console.log(
    [
      `batch=${batchName}`,
      `total=${total}`,
      `success=${successes}`,
      `failure=${total - successes}`,
      `success_rate=${total ? ((successes / total) * 100).toFixed(2) : '0.00'}%`,
      `status=${statusText}`,
      `error_kind=${errorKindText}`,
      `missing_request_id=${results.filter((item) => !item.requestId).length}`,
      `ttft_avg_ms=${average(ttftValues).toFixed(0)}`,
      `ttft_p95_ms=${percentile(ttftValues, 0.95).toFixed(0)}`,
      `total_avg_ms=${average(totalValues).toFixed(0)}`,
      `total_p95_ms=${percentile(totalValues, 0.95).toFixed(0)}`,
    ].join('\t'),
  );
}

function buildSummary(results, batchName) {
  const total = results.length;
  const successes = results.filter((item) => item.ok).length;
  const statusCounts = Object.fromEntries(
    [...results.reduce((map, item) => map.set(String(item.status || 'NO_RESPONSE'), (map.get(String(item.status || 'NO_RESPONSE')) || 0) + 1), new Map()).entries()]
      .sort(([left], [right]) => left.localeCompare(right)),
  );
  const errorKindCounts = Object.fromEntries(
    [...results.reduce((map, item) => map.set(item.errorKind, (map.get(item.errorKind) || 0) + 1), new Map()).entries()]
      .sort(([left], [right]) => left.localeCompare(right)),
  );
  const ttftValues = results.map((item) => item.ttftMs).filter((value) => value > 0);
  const totalValues = results.map((item) => item.totalMs).filter((value) => value > 0);
  return {
    batch_name: batchName,
    total,
    success: successes,
    failure: total - successes,
    success_rate: total ? successes / total : 0,
    status_counts: statusCounts,
    error_kind_counts: errorKindCounts,
    request_ids_missing_count: results.filter((item) => !item.requestId).length,
    ttft_ms: distribution(ttftValues),
    total_ms: distribution(totalValues),
  };
}

function average(values) {
  if (!values.length) return 0;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function distribution(values) {
  return {
    count: values.length,
    min: values.length ? Math.min(...values) : 0,
    avg: average(values),
    p50: percentile(values, 0.5),
    p90: percentile(values, 0.9),
    p95: percentile(values, 0.95),
    max: values.length ? Math.max(...values) : 0,
  };
}

function writeReportIfRequested(config, summary, results) {
  if (!config.reportPath) return;
  const reportPath = path.resolve(config.reportPath);
  fs.mkdirSync(path.dirname(reportPath), { recursive: true });
  fs.writeFileSync(reportPath, `${JSON.stringify({
    kind: 'modelgateway_loadtest_report',
    created_at: new Date().toISOString(),
    config: sanitizeConfig(config),
    summary,
    results,
  }, null, 2)}\n`);
  console.log(`report=${reportPath}`);
}

async function main() {
  const scenario = env('SCENARIO', process.argv[2] || 'burst100');
  const batchName = env('BATCH', `mg-${scenario}-${Date.now()}`);
  const allResults = [];
  const config = {
    scenario,
    batchName,
    baseURL: baseURL(),
    model: env('MODEL', 'gpt-5.5'),
    stream: env('STREAM', 'true') !== 'false',
    timeoutMs: numberEnv('TIMEOUT_MS', 180000),
    includeErrorSample: boolEnv('INCLUDE_ERROR_SAMPLE'),
    reportPath: env('REPORT_PATH'),
    apiKey: env('API_KEY'),
  };

  if (scenario === 'burst100') {
    const total = numberEnv('TOTAL', 100);
    const concurrency = numberEnv('CONCURRENCY', 100);
    allResults.push(...(await runPool(total, concurrency, batchName)));
  } else if (scenario === 'batched200') {
    const total = numberEnv('TOTAL', 200);
    const concurrency = numberEnv('CONCURRENCY', 20);
    const intervalMs = numberEnv('BATCH_INTERVAL_MS', 20000);
    let offset = 0;
    while (offset < total) {
      const size = Math.min(concurrency, total - offset);
      const results = await runPool(size, size, `${batchName}-part-${offset / concurrency + 1}`, offset);
      allResults.push(...results);
      printSummary(results, `${batchName}-part-${offset / concurrency + 1}`);
      offset += size;
      if (offset < total) await sleep(intervalMs);
    }
  } else {
    throw new Error(`unknown SCENARIO ${scenario}; use burst100 or batched200`);
  }

  const summary = buildSummary(allResults, batchName);
  printSummary(allResults, batchName);
  writeReportIfRequested(config, summary, allResults);
  for (const item of allResults) {
    console.log(
      [
        item.index,
        item.status,
        item.ok ? 'ok' : 'fail',
        item.errorKind,
        item.requestId || '-',
        item.ttftMs,
        item.totalMs,
        item.bytes,
        item.error || '-',
      ].join('\t'),
    );
  }
}

main().catch((error) => {
  console.error(error?.message || error);
  process.exitCode = 1;
});
