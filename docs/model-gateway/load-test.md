# 智能模型网关压测工具说明

本说明用于准备真实请求验收压测，但脚本默认只 dry-run，不会发送任何 HTTP 请求。真实执行必须显式加 `--run`；如果目标不是 localhost，还必须额外设置 `MODEL_GATEWAY_BENCH_ALLOW_REMOTE=1` 或 `--allow-remote`，避免误压远端服务。

## 脚本

- `scripts/modelgateway-load-bench.mjs`：OpenAI compatible 流式压测脚本，支持 `chat/completions` 与 `responses` 两种请求风格。
- `tmp/bench/modelgateway_bench_mock.mjs`：本地 mock，只用于验证脚本统计逻辑，不访问真实上游。

脚本只从环境变量或命令行读取 token，不包含任何硬编码 token。推荐 token 使用：

```bash
export MODEL_GATEWAY_BENCH_API_KEY='sk-...'
```

也兼容读取 `OPENAI_API_KEY` 作为兜底。

## 验收场景

### 场景 A：100 并发流式

Dry-run：

```bash
MODEL_GATEWAY_BENCH_MODEL='your-model' \
node scripts/modelgateway-load-bench.mjs \
  --scenario stream100 \
  --endpoint chat
```

本地真实执行：

```bash
MODEL_GATEWAY_BENCH_BASE_URL='http://127.0.0.1:3000/v1' \
MODEL_GATEWAY_BENCH_API_KEY='sk-local-or-real-local-token' \
MODEL_GATEWAY_BENCH_MODEL='your-model' \
MODEL_GATEWAY_BENCH_REPORT='tmp/bench/modelgateway-stream100-report.json' \
node scripts/modelgateway-load-bench.mjs \
  --scenario stream100 \
  --endpoint chat \
  --run
```

### 场景 B：200 个请求，每批并发 20，每隔 20 秒一批

Dry-run：

```bash
MODEL_GATEWAY_BENCH_MODEL='your-model' \
node scripts/modelgateway-load-bench.mjs \
  --scenario batch200 \
  --endpoint chat
```

本地真实执行：

```bash
MODEL_GATEWAY_BENCH_BASE_URL='http://127.0.0.1:3000/v1' \
MODEL_GATEWAY_BENCH_API_KEY='sk-local-or-real-local-token' \
MODEL_GATEWAY_BENCH_MODEL='your-model' \
MODEL_GATEWAY_BENCH_REPORT='tmp/bench/modelgateway-batch200-report.json' \
node scripts/modelgateway-load-bench.mjs \
  --scenario batch200 \
  --endpoint chat \
  --run
```

`batch200` 按批次启动时间间隔调度：第 1 批立即启动，第 2 批在约 20 秒后启动，以此类推。若单批请求超过 20 秒仍未结束，批次可能短暂重叠。

## Responses 风格

将 `--endpoint chat` 改为 `--endpoint responses` 即可。脚本会自动切换：

- `POST /v1/chat/completions`：`messages` + `max_tokens`
- `POST /v1/responses`：`input` + `max_output_tokens`

示例：

```bash
MODEL_GATEWAY_BENCH_MODEL='your-model' \
node scripts/modelgateway-load-bench.mjs \
  --scenario stream100 \
  --endpoint responses
```

## 本地 mock 验证

启动 mock：

```bash
node tmp/bench/modelgateway_bench_mock.mjs \
  --port 3118 \
  --mode mixed
```

另一个终端执行小规模真实请求，验证 `200/429/401`、TTFT 和总耗时统计：

```bash
MODEL_GATEWAY_BENCH_BASE_URL='http://127.0.0.1:3118/v1' \
MODEL_GATEWAY_BENCH_API_KEY='sk-local' \
MODEL_GATEWAY_BENCH_MODEL='mock-model' \
node scripts/modelgateway-load-bench.mjs \
  --scenario custom \
  --total 6 \
  --batch-size 3 \
  --batch-interval-ms 100 \
  --endpoint chat \
  --run
```

Responses mock 验证：

```bash
MODEL_GATEWAY_BENCH_BASE_URL='http://127.0.0.1:3118/v1' \
MODEL_GATEWAY_BENCH_API_KEY='sk-local' \
MODEL_GATEWAY_BENCH_MODEL='mock-model' \
node scripts/modelgateway-load-bench.mjs \
  --scenario custom \
  --total 3 \
  --batch-size 3 \
  --endpoint responses \
  --run
```

## 常用参数

| 参数 | 环境变量 | 说明 |
| --- | --- | --- |
| `--scenario` | `MODEL_GATEWAY_BENCH_SCENARIO` | `stream100`、`batch200` 或 `custom` |
| `--endpoint` | `MODEL_GATEWAY_BENCH_ENDPOINT` | `chat` 或 `responses` |
| `--base-url` | `MODEL_GATEWAY_BENCH_BASE_URL` | 默认 `http://localhost:3000/v1` |
| `--url` | `MODEL_GATEWAY_BENCH_URL` | 完整 endpoint URL 覆盖 |
| `--model` | `MODEL_GATEWAY_BENCH_MODEL` | 实际请求模型，`--run` 时必填 |
| `--total` | `MODEL_GATEWAY_BENCH_TOTAL` | 总请求数 |
| `--batch-size` | `MODEL_GATEWAY_BENCH_BATCH_SIZE` | 每批启动请求数 |
| `--batch-interval-ms` | `MODEL_GATEWAY_BENCH_BATCH_INTERVAL_MS` | 批次启动间隔 |
| `--timeout-ms` | `MODEL_GATEWAY_BENCH_TIMEOUT_MS` | 单请求超时 |
| `--max-tokens` | `MODEL_GATEWAY_BENCH_MAX_TOKENS` | `max_tokens` 或 `max_output_tokens` |
| `--report` | `MODEL_GATEWAY_BENCH_REPORT` | 可选 JSON 报告路径 |
| `--allow-remote` | `MODEL_GATEWAY_BENCH_ALLOW_REMOTE=1` | 明确允许非 localhost 压测 |
| `--no-auth` | `MODEL_GATEWAY_BENCH_NO_AUTH=1` | 不发送 Authorization |

如需补充供应商或网关参数，可用 `MODEL_GATEWAY_BENCH_EXTRA_BODY_JSON` 做浅合并：

```bash
MODEL_GATEWAY_BENCH_EXTRA_BODY_JSON='{"temperature":0,"stream_options":{"include_usage":true}}'
```

如需额外 header，可用：

```bash
MODEL_GATEWAY_BENCH_HEADERS_JSON='{"X-Test-Group":"model-gateway"}'
```

## 输出指标

控制台 summary 包含：

- `success_rate`：2xx 且流读取完成的成功率。
- `status_counts`：HTTP 状态码统计，网络错误记为 `NO_RESPONSE`。
- `error_kind_counts`：`success`、`rate_limit_429`、`auth_config_error`、`server_error`、`http_error`、`timeout`、`network_error`、`stream_interrupted` 等。
- `rate_limit_429`：HTTP 429 或 overload/rate-limit 语义错误数量。
- `auth_config_error`：HTTP 401/403 或认证/权限/模型配置错误语义数量。
- `ttft_ms_success`：成功请求的首个有效流式数据时间，输出 `min/avg/p50/p90/p95/max`。
- `total_ms_all`：所有请求总耗时分布。

JSON 报告不会写入 API key，只记录脱敏后的配置、summary 和逐请求统计。
