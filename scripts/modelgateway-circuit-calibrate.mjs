#!/usr/bin/env node

import fs from 'node:fs/promises';
import path from 'node:path';

const TARGET_TYPES = ['rate_limit', 'server_error', 'stream_interrupted'];
const DEFAULTS = {
  rate_limit: {
    failure_threshold: 0.6,
    min_samples: 5,
    open_seconds: 20,
    half_open_probe_count: 2,
  },
  server_error: {
    failure_threshold: 0.5,
    min_samples: 10,
    open_seconds: 30,
    half_open_probe_count: 3,
  },
  stream_interrupted: {
    failure_threshold: 0.4,
    min_samples: 5,
    open_seconds: 60,
    half_open_probe_count: 1,
  },
};

const LIMITS = {
  rate_limit: { minThreshold: 0.35, maxThreshold: 0.75, minOpen: 15, maxOpen: 90 },
  server_error: { minThreshold: 0.35, maxThreshold: 0.75, minOpen: 20, maxOpen: 120 },
  stream_interrupted: { minThreshold: 0.25, maxThreshold: 0.6, minOpen: 45, maxOpen: 180 },
};

function usage() {
  console.log(`Usage:
  node scripts/modelgateway-circuit-calibrate.mjs <trend-export.json...>

Options:
  --min-attempts <n>       Minimum group attempts for normal confidence (default: 50)
  --min-error-count <n>    Minimum error count before recommending overrides (default: 3)
  --format json|markdown   Output format (default: markdown)

Examples:
  node scripts/modelgateway-circuit-calibrate.mjs tmp/codex-pro-trends.json
  node scripts/modelgateway-circuit-calibrate.mjs tmp/*-trends.json --format json
`);
}

function parseArgs(argv) {
  const args = {
    files: [],
    minAttempts: 50,
    minErrorCount: 3,
    format: 'markdown',
  };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--help' || arg === '-h') {
      usage();
      process.exit(0);
    }
    if (arg === '--min-attempts') {
      args.minAttempts = Number(argv[++i]);
      continue;
    }
    if (arg === '--min-error-count') {
      args.minErrorCount = Number(argv[++i]);
      continue;
    }
    if (arg === '--format') {
      args.format = String(argv[++i] || '').trim();
      continue;
    }
    args.files.push(arg);
  }
  if (!['json', 'markdown'].includes(args.format)) {
    throw new Error(`unsupported --format: ${args.format}`);
  }
  if (!Number.isFinite(args.minAttempts) || args.minAttempts < 1) {
    throw new Error('--min-attempts must be a positive number');
  }
  if (!Number.isFinite(args.minErrorCount) || args.minErrorCount < 1) {
    throw new Error('--min-error-count must be a positive number');
  }
  if (!args.files.length) {
    usage();
    throw new Error('at least one trend export JSON file is required');
  }
  return args;
}

function unwrapExport(value) {
  if (value?.data?.kind === 'modelgateway_trends_export') return value.data;
  if (value?.data?.data?.kind === 'modelgateway_trends_export') return value.data.data;
  if (value?.kind === 'modelgateway_trends_export') return value;
  if (Array.isArray(value?.trends)) return value;
  throw new Error('not a model gateway trend export payload');
}

function numberValue(value) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

function countFromReasons(rows, reason) {
  if (!Array.isArray(rows)) return 0;
  return rows.reduce((sum, item) => {
    const key = String(item?.reason || item?.type || item?.key || '').trim();
    if (key !== reason) return sum;
    return sum + numberValue(item?.count || item?.value || item?.total);
  }, 0);
}

function percentile(values, p) {
  const sorted = values.filter((v) => Number.isFinite(v)).sort((a, b) => a - b);
  if (!sorted.length) return 0;
  const index = Math.ceil((p / 100) * sorted.length) - 1;
  return sorted[Math.max(0, Math.min(sorted.length - 1, index))];
}

function clamp(value, min, max) {
  return Math.min(max, Math.max(min, value));
}

function roundThreshold(value) {
  return Number(clamp(Math.round(value * 20) / 20, 0.05, 1).toFixed(2));
}

function roundOpenSeconds(value) {
  return Math.round(value / 5) * 5;
}

function bucketErrorCount(trend, type) {
  const fromTypes = countFromReasons(trend?.circuit_error_types, type);
  if (fromTypes > 0) return fromTypes;
  const fromCounts = countFromReasons(trend?.circuit_error_counts, type);
  if (fromCounts > 0) return fromCounts;
  if (type === 'stream_interrupted') return numberValue(trend?.stream_interrupted);
  return 0;
}

function mergeGroup(groups, group, exportPayload, sourceFile) {
  if (!groups.has(group)) {
    groups.set(group, {
      group,
      files: [],
      filters: [],
      trends: [],
    });
  }
  const target = groups.get(group);
  target.files.push(sourceFile);
  target.filters.push(exportPayload.filters || {});
  target.trends.push(...(Array.isArray(exportPayload.trends) ? exportPayload.trends : []));
}

function groupNameForExport(exportPayload, file) {
  const group = String(exportPayload?.filters?.group || '').trim();
  if (group) return group;
  return `all:${path.basename(file)}`;
}

function statsForType(trends, type) {
  const buckets = [];
  let attempts = 0;
  let errorCount = 0;
  let affectedBuckets = 0;
  let maxBucketCount = 0;

  for (const trend of trends) {
    const bucketAttempts = numberValue(trend?.attempts);
    const bucketErrors = bucketErrorCount(trend, type);
    attempts += bucketAttempts;
    errorCount += bucketErrors;
    if (bucketErrors > 0) {
      affectedBuckets += 1;
      maxBucketCount = Math.max(maxBucketCount, bucketErrors);
    }
    if (bucketAttempts > 0) {
      buckets.push({
        attempts: bucketAttempts,
        errors: bucketErrors,
        rate: bucketErrors / bucketAttempts,
      });
    }
  }

  const rates = buckets.map((bucket) => bucket.rate);
  const affectedRates = buckets
    .filter((bucket) => bucket.errors > 0)
    .map((bucket) => bucket.rate);
  return {
    attempts,
    error_count: errorCount,
    affected_buckets: affectedBuckets,
    bucket_count: buckets.length,
    overall_rate: attempts > 0 ? errorCount / attempts : 0,
    max_bucket_rate: rates.length ? Math.max(...rates) : 0,
    p50_bucket_rate: percentile(affectedRates, 50),
    p75_bucket_rate: percentile(affectedRates, 75),
    p90_bucket_rate: percentile(affectedRates, 90),
    max_bucket_count: maxBucketCount,
  };
}

function confidenceFor(stats, args) {
  if (stats.attempts < args.minAttempts || stats.error_count < args.minErrorCount) {
    return 'low';
  }
  if (stats.affected_buckets < 2) {
    return 'medium';
  }
  return 'normal';
}

function recommendPolicy(type, stats, args) {
  const base = DEFAULTS[type];
  const limits = LIMITS[type];
  const confidence = confidenceFor(stats, args);
  const notes = [];

  let failureThreshold = base.failure_threshold;
  let minSamples = base.min_samples;
  let openSeconds = base.open_seconds;
  let halfOpenProbeCount = base.half_open_probe_count;

  if (confidence === 'low') {
    notes.push('sample volume is low; keep the default profile unless this is an active incident');
  } else {
    const severityRate = Math.max(stats.p75_bucket_rate, stats.overall_rate);
    if (severityRate >= base.failure_threshold) {
      failureThreshold = base.failure_threshold;
    } else if (severityRate > 0) {
      failureThreshold = Math.max(limits.minThreshold, Math.min(base.failure_threshold, severityRate + 0.1));
    }

    if (stats.p90_bucket_rate >= base.failure_threshold || stats.affected_buckets >= 3) {
      openSeconds += type === 'stream_interrupted' ? 30 : 15;
    }
    if (type === 'rate_limit' && stats.affected_buckets === 1 && stats.p90_bucket_rate < 0.5) {
      openSeconds = Math.max(limits.minOpen, openSeconds - 10);
      minSamples += 2;
      notes.push('rate limit looks bursty; prefer more samples over longer isolation');
    }
    if (type === 'stream_interrupted') {
      halfOpenProbeCount = 1;
      if (stats.error_count >= args.minErrorCount) {
        minSamples = Math.max(3, Math.min(base.min_samples, stats.max_bucket_count));
      }
    } else if (type === 'server_error' && stats.affected_buckets >= 3) {
      openSeconds += 15;
    }
  }

  if (stats.error_count === 0) {
    notes.push('no events observed for this type');
  }

  return {
    failure_threshold: roundThreshold(clamp(failureThreshold, limits.minThreshold, limits.maxThreshold)),
    min_samples: Math.max(1, Math.round(minSamples)),
    open_seconds: roundOpenSeconds(clamp(openSeconds, limits.minOpen, limits.maxOpen)),
    half_open_probe_count: halfOpenProbeCount,
    confidence,
    notes,
  };
}

function analyzeGroup(group, trends, args) {
  const result = {
    group,
    bucket_count: trends.length,
    policy: {},
    metrics: {},
  };

  for (const type of TARGET_TYPES) {
    const stats = statsForType(trends, type);
    result.metrics[type] = stats;
    result.policy[type] = recommendPolicy(type, stats, args);
  }

  return result;
}

async function readExports(files) {
  const groups = new Map();
  for (const file of files) {
    const raw = await fs.readFile(file, 'utf8');
    const payload = unwrapExport(JSON.parse(raw));
    mergeGroup(groups, groupNameForExport(payload, file), payload, file);
  }
  return [...groups.values()];
}

function asConfigSnippet(analysis) {
  const policies = {};
  for (const type of TARGET_TYPES) {
    const policy = analysis.policy[type];
    if (policy.confidence === 'low' && analysis.metrics[type].error_count === 0) {
      continue;
    }
    policies[type] = {
      failure_threshold: policy.failure_threshold,
      min_samples: policy.min_samples,
      open_seconds: policy.open_seconds,
      half_open_probe_count: policy.half_open_probe_count,
    };
  }
  return { circuit_error_policies: policies };
}

function formatPercent(value) {
  return `${(numberValue(value) * 100).toFixed(1)}%`;
}

function printMarkdown(analyses) {
  console.log('# ModelGateway Circuit Calibration Report\n');
  for (const analysis of analyses) {
    console.log(`## Group: ${analysis.group}\n`);
    console.log('| Error type | Attempts | Events | Affected buckets | Overall rate | P90 bucket rate | Recommendation | Confidence |');
    console.log('| --- | ---: | ---: | ---: | ---: | ---: | --- | --- |');
    for (const type of TARGET_TYPES) {
      const stats = analysis.metrics[type];
      const policy = analysis.policy[type];
      console.log(
        `| \`${type}\` | ${stats.attempts} | ${stats.error_count} | ${stats.affected_buckets}/${stats.bucket_count} | ${formatPercent(stats.overall_rate)} | ${formatPercent(stats.p90_bucket_rate)} | threshold ${policy.failure_threshold}, min ${policy.min_samples}, open ${policy.open_seconds}s, probes ${policy.half_open_probe_count} | ${policy.confidence} |`,
      );
      for (const note of policy.notes) {
        console.log(`\n> ${type}: ${note}`);
      }
    }
    console.log('\nSuggested config snippet:\n');
    console.log('```json');
    console.log(JSON.stringify(asConfigSnippet(analysis), null, 2));
    console.log('```\n');
  }
}

function printJson(analyses) {
  console.log(JSON.stringify({
    kind: 'modelgateway_circuit_calibration',
    target_error_types: TARGET_TYPES,
    groups: analyses.map((analysis) => ({
      ...analysis,
      config_snippet: asConfigSnippet(analysis),
    })),
  }, null, 2));
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const groups = await readExports(args.files);
  const analyses = groups.map((group) => analyzeGroup(group.group, group.trends, args));
  if (args.format === 'json') {
    printJson(analyses);
  } else {
    printMarkdown(analyses);
  }
}

main().catch((error) => {
  console.error(`modelgateway-circuit-calibrate: ${error.message}`);
  process.exitCode = 1;
});
