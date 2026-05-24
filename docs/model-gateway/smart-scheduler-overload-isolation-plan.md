# 智能调度体系优化方案

## 目标

本方案只处理当前真实数据暴露出的明确问题：

- 429 过载信号不再作为渠道质量失败。
- 401/403 配置错误渠道不再持续参与调度和高频探活。
- Sticky 与队列只处理临时容量问题，不粘住不可用渠道。
- 修复自动恢复、空失败分类、观测刷新 429 等数据可信问题。

本轮不做以下改动：

- 不重构评分公式。
- 不调整分组权重。
- 不修改 `auto` 当前策略。
- 不拆分真实样本与探活样本。

兼容边界：

- 智能调度默认关闭，只对已开启分组生效，未开启分组继续走旧流程。
- 新增字段与观测指标只做兼容扩展，旧生产版本不读取时不影响启动、调度和回退。
- 429、401/403、队列三类行为只修正智能调度链路，旧生产链路保持原兼容行为。

## 1. 429 过载跳过

### 目标

429 不再被当成渠道失败，只表示“该渠道当前满了，本次请求换下一个”。

关键口径：429 只作用于当前请求内候选 skip，不降权、不冷却、不熔断。

### 识别范围

统一归类为 `overload_skip`：

- HTTP 429
- `Too many pending requests`
- `concurrency limit exceeded`
- 语义明确为并发或容量限制的 `rate_limit`

### 请求内行为

候选渠道返回 429 时：

- 当前请求跳过该候选渠道。
- 不记录为失败质量样本。
- 继续尝试下一个候选渠道。

如果全部候选都是 429：

- 队列开启且未满：进入短队列。
- 队列关闭、队列满或队列超时：返回用户侧 429。

### 长期评分行为

`overload_skip` 不参与长期质量评分：

- 不计入渠道失败率。
- 不降低成功率、速度分、体验分。
- 不触发冷却。
- 不触发熔断。
- 不写入长期失败样本。
- 不触发渠道禁用、配置隔离或 sticky failure。

### 工程观测

工程视图单独展示：

- `overload_skip_count`
- `last_overload_at`
- `overload_channels`

## 2. 401/403 配置错误隔离

### 目标

401/403 这类明确配置错误不应持续探活、污染日志和候选池。

关键口径：401/403 是配置隔离信号，不是临时容量问题，也不应进入队列等待。

### 识别范围

统一归类为 `auth_config_error`：

- HTTP 401
- HTTP 403
- `Invalid API key`
- `permission denied`
- `model not allowed`
- `provider account forbidden`

### 隔离粒度

隔离粒度为：

```text
channel_id + requested_model + selected_group + endpoint_type
```

这样不会因为某个模型不可用，把同渠道其他模型全部隔离。

### 触发规则

同一隔离粒度下：

```text
连续 2 次 auth_config_error => 进入配置错误隔离
```

### 隔离行为

隔离期间：

- 不参与用户请求调度。
- 默认不参与高频探活。
- 最多每 30 分钟进行一次低频验证。

### 解除规则

满足任一条件即可解除：

- 管理员修改渠道配置。
- 管理员手动解除隔离。
- 低频验证连续成功。

### 建议字段

建议新增到 `model_gateway_runtime_snapshots`：

- `config_error_isolated`
- `isolation_reason`
- `isolation_until`
- `auth_config_error_count`
- `last_auth_config_error_at`

新增字段只做兼容扩展，旧版本不读取这些字段，不影响旧服务。

## 3. Sticky 修正

### 目标

保留用户短期渠道粘性，但不能粘住不可用、隔离或饱和渠道。

### 保留 Sticky 的条件

只有在以下条件都满足时保留 sticky：

- 渠道正常。
- 未配置错误隔离。
- 未熔断。
- 未饱和。
- 本次没有明确过载。

### 必须打破 Sticky 的情况

以下情况必须打破 sticky：

- `config_error_isolated = true`
- `circuit_open = true`
- 渠道禁用。
- 渠道余额不足。
- `active_concurrency >= effective_limit` 且存在其他可用候选。

### 429 对 Sticky 的影响

429 只影响当前请求：

- 不清除 sticky。
- 不写 sticky failure。
- 不降低 sticky 渠道长期评分。

也就是本次满了先换，下次还可以继续尝试原 sticky 渠道。

## 4. 队列逻辑细化

### 目标

队列只作为临时容量不足时的兜底，不让用户排在明确不可用渠道后面。

关键口径：队列只处理“等一等可能恢复”的临时容量，不处理配置、权限、余额、能力不匹配等不可用问题。

### 进入队列条件

只有当全部候选都因以下原因暂时不可服务时，才允许进入队列：

- `overload_skip`
- `concurrency_saturated`

### 不进入队列的情况

以下情况不进入队列：

- 401/403 配置错误。
- 渠道禁用。
- 渠道熔断。
- 渠道余额不足。
- 模型能力不匹配。

这些属于不可用，不是等一等即可恢复。

### 队列结果

- 队列等待成功：重新选择候选渠道。
- 队列超时：返回用户侧 429。
- 队列满：返回用户侧 429。

队列等待时间沿用现有配置，不扩大默认等待时间。

## 5. 记录修复

### 目标

让观测数据可解释、可追踪、可用于后续调度评估。

### 新增字段

`model_execution_records` 增加：

- `error_category`

失败 attempt 必须写入该字段。

### 分类规则

`error_category` 可取值：

- `overload_skip`
- `auth_config_error`
- `server_error`
- `stream_interrupted`
- `client_aborted`
- `upstream_error`
- `unknown`

无法识别时统一写 `unknown`。

### 自动恢复修复

同一个 `request_id`：

```text
前置 attempt 失败 + 后续 attempt 成功 => recovered = true
```

### 空失败修复

当前存在大量不可解释失败：

```text
status_code = 0
error_code = ''
error_type = ''
```

后续必须至少写入：

```text
error_category = unknown
```

工程视图单独展示 `unknown`，便于继续补充分类。

## 6. 观测页面与接口

### 用户请求视图

只展示最终用户体验：

- 用户成功率
- 最终失败数
- 自动恢复数
- 平均首包
- P95 首包
- 平均总耗时
- P95 总耗时

不展示内部 429、候选渠道、探活失败。

### 工程视图

展示内部调度细节：

- `overload_skip_count`
- `auth_config_error_count`
- `config_error_isolated`
- `isolation_reason`
- `isolation_until`
- `queue_wait_count`
- `unknown_error_count`
- `candidate_skip_reason`

### 观测接口防 429

前端：

- 自动刷新使用 single-flight。
- 切换筛选时取消旧请求。
- 刷新按钮防抖。
- 倒计时刷新不并发触发。

后端：

- summary 接口增加 2 秒短缓存。
- 相同 query 参数复用结果。

## 7. 本地开启与验证注意事项

本地验证建议按灰度方式开启，避免误判旧链路行为：

1. 在系统设置中开启智能调度总开关 `enabled=true`。
2. 只给验证分组配置 `shadow` 或 `active`，未配置分组继续走旧生产兼容流程。
3. 压测前确认验证分组的队列、熔断、探活配置与目标场景一致；如果只验证 429 skip，可优先固定候选池和并发上限。
4. 观测验证优先看工程视图和接口字段，不把用户请求视图里的最终成功率误解为内部 attempt 失败率。
5. 本地或单节点 Redis 不稳定时，可关闭运行态 Redis 同步，只验证本进程调度行为；跨节点一致性另行验证。

重点检查：

- 429 attempt 只产生 `overload_skip`，不产生冷却、熔断、禁用、长期失败样本。
- 401/403 连续命中后按 `channel_id + requested_model + selected_group + endpoint_type` 隔离。
- 队列只在候选全部为 `overload_skip` 或 `concurrency_saturated` 时进入。
- 未开启智能调度的分组仍走旧流程，不依赖新增字段。

压测工具：

```bash
# 100 并发流式
MODEL_GATEWAY_BENCH_BASE_URL=http://localhost:3001/v1 \
MODEL_GATEWAY_BENCH_API_KEY=本地测试令牌 \
MODEL_GATEWAY_BENCH_MODEL=gpt-5.5 \
MODEL_GATEWAY_BENCH_REPORT=tmp/bench/modelgateway-stream100-report.json \
  node scripts/modelgateway-load-bench.mjs --scenario stream100 --endpoint chat --run

# 200 请求，单批 20 并发，每批间隔 20 秒
MODEL_GATEWAY_BENCH_BASE_URL=http://localhost:3001/v1 \
MODEL_GATEWAY_BENCH_API_KEY=本地测试令牌 \
MODEL_GATEWAY_BENCH_MODEL=gpt-5.5 \
MODEL_GATEWAY_BENCH_REPORT=tmp/bench/modelgateway-batch200-report.json \
  node scripts/modelgateway-load-bench.mjs --scenario batch200 --endpoint chat --run
```

可选环境变量：

- `MODEL_GATEWAY_BENCH_TOTAL`：总请求数，默认 `stream100=100`、`batch200=200`。
- `MODEL_GATEWAY_BENCH_BATCH_SIZE`：单批并发数，默认 `stream100=100`、`batch200=20`。
- `MODEL_GATEWAY_BENCH_BATCH_INTERVAL_MS`：分批间隔，默认 `20000`。
- `MODEL_GATEWAY_BENCH_PROMPT`、`MODEL_GATEWAY_BENCH_MAX_TOKENS`：请求内容与最大输出。
- `MODEL_GATEWAY_BENCH_REPORT`：JSON 报告路径，报告包含状态码、错误分类、request id、TTFT 和总耗时分布。
- `MODEL_GATEWAY_BENCH_INCLUDE_ERROR_SAMPLE=1`：失败请求写入前 4KB 响应样本，便于确认 429/auth/config/stream 中断原因。

仍保留轻量脚本 `scripts/modelgateway-loadtest.mjs`，用于快速本地冒烟：

```bash
API_KEY=本地测试令牌 BASE_URL=http://localhost:3001/v1 MODEL=gpt-5.5 REPORT_PATH=tmp/bench/modelgateway-light-report.json \
  node scripts/modelgateway-loadtest.mjs burst100
```

## 8. 开发顺序

1. 增加 `error_category` 字段和错误分类函数。
2. 实现 429 `overload_skip`，不降权、不冷却、不熔断。
3. 实现 401/403 配置错误隔离。
4. 修正 sticky 和队列判断。
5. 修复 `recovered` 和 `unknown` 错误记录。
6. 补充观测接口字段。
7. 前端观测刷新防并发。
8. 跑 100 并发、200 分批流式压测验证。

## 9. 验收标准

- 429 不再触发渠道冷却。
- 429 不再降低渠道成功率。
- 429 不再触发渠道熔断、禁用或配置隔离。
- 401/403 渠道不会持续高频探活。
- 401/403 隔离不误伤同渠道其它模型、分组或 endpoint。
- 队列只承接临时容量不足，不承接配置错误、余额不足、禁用、熔断和能力不匹配。
- 失败后成功的请求 `recovered = true`。
- 失败 attempt 都有 `error_category`。
- 观测页面自动刷新不再打出 summary 429。
- 100 并发流式成功率保持稳定。
- 200 分批流式成功率不低于当前水平。
- 未开启智能调度的分组和旧生产兼容链路不受新增字段影响。

## 10. 当前实施进度

### 已完成

后端调度主链路：

- 后端新增统一错误分类常量，核心链路支持 `overload_skip`、`auth_config_error`、`unknown`。
- Relay 主链路已将 429 作为当前请求跳过信号，支持换候选，不进入 failure avoidance、冷却、禁用或熔断，也不写入长期失败样本。
- Relay 主链路已将 401/403 写入配置错误隔离，连续 2 次按 `channel_id + requested_model + selected_group + endpoint_type` 隔离。
- 调度器运行时快照、候选解释、运行时 enrich 已支持配置错误隔离字段。
- 默认选择器已把 `config_error_isolated` 候选剔除，并自然打破 sticky。
- 探活选择器已跳过快照隔离、渠道配置隔离和内存隔离路由。
- 熔断器和健康监控已跳过 429 overload 样本，不计分、不熔断。
- 执行记录 `model_execution_records` 已新增 `error_category`，失败 attempt 可持久化分类。
- Runtime snapshot 已新增隔离字段并支持 flush/restore。
- Sticky 生命周期测试修正为稳定 session key，避免成功续期阶段重新解析失败。

已完成的基础回归：

```text
go test ./pkg/modelgateway/...
go test ./controller -run 'Relay|Retry|ChannelError|ConfigIsolation|Overload'
go test ./service -run 'ConfigIsolation|ConcurrencyLimitDoesNotCreateCooldown|UpstreamPendingConcurrencyDoesNotCreateCooldown'
```

### 已完成的工程化补充

- 观测页面已补 single-flight、旧请求取消、手动刷新防抖，倒计时刷新不会并发堆积。
- 工程视图已展示 `overload_skip_count`、`auth_config_error_count`、`unknown_error_count`、`config_error_isolated_count`、`queue_wait_count`。
- 观测接口 summary/trend/aggregate 已输出工程指标，候选解释已透出配置隔离原因、隔离截止时间和 auth 错误次数。
- 已新增 `scripts/modelgateway-loadtest.mjs`，支持 100 并发流式与 200 分批流式压测。
- 已对轻量压测脚本补充 request id、timeout、错误分类、失败响应样本和 JSON report；真实验收优先使用 `scripts/modelgateway-load-bench.mjs`。

已补充回归：

- 前端观测页切换筛选、手动刷新、倒计时刷新不会并发打 `/api/model_gateway/observability/summary`。
- 工程视图可以看到 `overload_skip_count`、`auth_config_error_count`、隔离状态、队列等待和 unknown 分类。
- 同一 query 参数的 summary 短缓存不会串筛选条件，也不会隐藏最新手动刷新结果。
- 后端聚合测试覆盖 overload/auth/unknown/queue/isolation 指标。

### 待压测

真实压测暂未完成，待按真实测试令牌执行：

- 100 并发流式：验证成功率、首包、总耗时、429 skip 数、冷却/熔断未误触发。
- 200 分批流式：验证成功率不低于当前水平，队列只承接临时容量不足。
- 401/403 配置错误场景：验证隔离后不进入用户请求调度，不高频探活，低频验证或配置变更后可解除。
- 旧生产兼容场景：关闭智能调度或未配置分组时，确认仍走旧流程，新增字段为空或缺失不影响请求。

### 剩余验收

- 前端刷新防并发与 summary 短缓存落地并验证。
- 工程视图聚合指标落地并能区分用户体验指标与内部调度指标。
- 压测报告补齐 100 并发、200 分批、401/403 隔离、旧链路兼容四组结果。
- 根据压测结果确认是否需要调整队列默认深度或探活低频验证间隔；若无证据，本轮不调整默认策略。
