# 渠道内多账号智能调度方案

## Summary

本方案将智能调度从“渠道级”扩展到“渠道 + 账号级”，同时继续复用最新评分引擎的同一套 `ScoreItem` 体系。核心目标是让多账号聚合渠道里的每个账号都拥有独立的健康状态、评分、熔断和恢复能力，但请求链路不遍历全量账号、不现场重算复杂评分。

整体策略：

- 评分对象：从 `model + group + endpoint + channel` 扩展为 `model + group + endpoint + channel + credential`。
- 评分体系：完全复用现有 `CandidateScoringService`、`ScoreItem`、`RuntimeSnapshot`、`ScoreEvent`。
- 请求调度：只读取后台物化好的账号分数，并做少量实时压力修正。
- 请求结束：异步更新账号级 snapshot 和当前分数。
- 硬不可用：认证失败、余额不足、手动禁用等必须当场标记，不等待异步评分。

## Design Principles

1. 不新增第二套账号评分体系

   账号不是一套新评分模型，而是更细粒度的 `RuntimeKey`。最新评分引擎里的完成率、上游错误率、首包速度、完整耗时、空输出率、流中断率、并发负载、队列压力、首包积压、成本分、分组分全部继续复用。

2. 请求链路不做大规模计算

   在 1000 个渠道、10 万账号的情况下，调度不能每次展开并评分所有账号。单次请求只从候选索引里拿有界候选，例如 Top-K 64 个账号 + 少量探索账号。

3. 评分异步物化

   请求结束后通过异步任务更新账号级 runtime snapshot，并调用最新评分引擎生成当前分数。调度阶段优先读物化结果。

4. 硬状态即时生效

   账号不可用、认证失败、余额不足、手动禁用、上游明确拒绝等状态需要立即写入账号状态索引，马上影响后续调度。

5. 渠道级能力仍保留

   渠道级并发上限、Base URL 故障、渠道禁用、分组/模型能力、成本配置仍然有效。账号级能力是在渠道级约束下进一步选择具体账号。

## Runtime Key

现有 runtime key：

```text
requested_model + upstream_model + channel_id + group + endpoint_type + capability_fingerprint
```

扩展为：

```text
requested_model
+ upstream_model
+ channel_id
+ group
+ endpoint_type
+ capability_fingerprint
+ credential_index
+ credential_fingerprint
```

新增字段：

```go
type RuntimeKey struct {
    RequestedModel        string
    UpstreamModel         string
    ChannelID             int
    Group                 string
    EndpointType          constant.EndpointType
    CapabilityFingerprint string
    CredentialIndex       int
    CredentialFingerprint string
}
```

账号指纹规则：

- OAuth JSON：优先使用 `account_id` 生成不可逆 HMAC/SHA256 指纹，避免 access token 刷新后评分丢失。
- 普通 API Key：使用 normalized raw key 生成不可逆 HMAC/SHA256 指纹。
- UI 和日志只展示账号序号与短指纹，例如 `账号 #3 / fp: ab12cd`。
- 原始 key 只能保存在内存态 `CredentialRef.RawKey`，并使用 `json:"-"`，不能进入日志、API 响应、回放导出或 score event。

## Data Structures

新增内存态账号引用：

```go
type CredentialRef struct {
    Index       int    `json:"credential_index,omitempty"`
    Fingerprint string `json:"credential_fingerprint,omitempty"`
    RawKey      string `json:"-"`
    Status      string `json:"status,omitempty"`
    Source      string `json:"source,omitempty"`
}
```

扩展候选与计划：

```go
type Candidate struct {
    Channel                *model.Channel
    Group                  string
    UpstreamModel          string
    ProviderProfile        string
    ProxyMode              string
    RequiresCodexImageTool bool
    RuntimeKey             RuntimeKey
    Credential             *CredentialRef
}

type DispatchPlan struct {
    Channel     *model.Channel
    RuntimeKey  RuntimeKey
    Credential  *CredentialRef
    ScoreTotal  float64
    // existing fields remain unchanged
}
```

持久化扩展：

- `model_gateway_runtime_snapshots`
  - `credential_index`
  - `credential_fingerprint`
  - `score_total`
  - `routing_score_total`
  - `score_breakdown_json`
  - `routing_score_breakdown_json`
  - `score_items_json`

- `model_gateway_score_events`
  - `credential_index`
  - `credential_fingerprint`

历史无账号字段的数据保持兼容，可作为冷启动 fallback 样本。

## Candidate Index

新增账号候选索引 `CredentialCandidateIndex`，按请求路由维度组织：

```text
requested_model + group + endpoint_type + required_tools
```

索引项保存：

```text
channel_id
channel_name
credential_index
credential_fingerprint
materialized_score_total
materialized_routing_score_total
score_items_digest
sample_count
status
last_used_at
last_success_at
last_failure_at
cost_ratio
cost_reference_ratio
group_priority_ratio
```

索引更新来源：

- 渠道新增、编辑、删除。
- 多账号 key 新增、删除、禁用、启用。
- 成本配置变化。
- 分组策略变化。
- 后台评分更新。
- 探活结果更新。
- 认证/余额/硬错误即时状态更新。

索引查询策略：

- 单次请求默认最多返回 Top-K 账号，例如 64。
- 额外加入少量探索账号，例如 4-8 个低样本或近期未访问账号。
- sticky/cache affinity 命中的账号强制加入。
- 重试时排除当前请求已失败的 runtime key。
- 对低样本账号使用冷启动分，不能无限排在末尾。

## Dispatch Flow

请求调度流程：

1. 根据请求生成 `DispatchRequest`。
2. 解析分组策略、候选分组、endpoint、required tools。
3. 从 `CredentialCandidateIndex` 获取有界账号候选。
4. 对候选做轻量实时修正：
   - 渠道/账号启用状态。
   - 账号隔离状态。
   - 渠道并发与选择预占。
   - 队列压力。
   - 首包积压。
   - circuit open / half-open。
   - failure avoidance。
5. 选择 `routing_score_total` 最高的账号。
6. `DispatchPlan` 写入选中的 `CredentialRef`。
7. `SetupContextForSelectedChannel` 如果发现 plan 带有 credential，则直接使用该 raw key。
8. 只有非智能调度或 plan 没有 credential 时，才回退到现有 `GetNextEnabledKey()` 随机/轮询逻辑。

关键行为：

- 智能调度 active 模式下，选中账号必须固定到 relay，不允许再次随机/轮询换 key。
- shadow 模式不改变实际 key 选择，只记录建议账号。
- retry 时允许同一渠道切换到另一个账号，但不能重复选择已经失败的账号 runtime key。

## Async Scoring Flow

请求完成后：

1. Relay 生成 `AttemptResult`，其中 `RuntimeKey` 必须包含账号维度。
2. `AsyncExecutionRecorder` 异步消费结果事件。
3. `RuntimeHealthMonitor` 更新账号级 `RuntimeSnapshot`。
4. 调用 `CandidateScoringService` 按最新评分引擎重新计算：
   - `ScoreTotal`
   - `RoutingScoreTotal`
   - `ScoreBreakdown`
   - `RoutingScoreBreakdown`
   - `ScoreItems`
5. 更新内存 `CredentialCandidateIndex`。
6. 批量或节流持久化 snapshot。
7. 写入 `ModelGatewayScoreEvent`，用于评分变更记录。

当前评分引擎继续使用以下 ScoreItem：

- `completion_rate`：完成率分。
- `upstream_error_rate`：上游错误率分。
- `ttft_latency`：首包速度分。
- `duration_latency`：完整耗时分。
- `throughput`：吞吐速度分。
- `empty_output_rate`：空输出率分。
- `stream_interrupted_rate`：流中断率分。
- `concurrency_load`：并发负载分。
- `queue_pressure`：队列压力分。
- `first_byte_backlog`：首包积压分。
- `cost`：成本分。
- `group_priority`：分组分。

调度时可以复用物化的 sample/formula 分数，只重新叠加 pressure 类动态字段，避免每次请求跑全量评分。

## Immediate Account State

以下状态必须即时生效，不等待后台评分：

- 账号手动禁用：立即从候选索引移除。
- OAuth/API Key 认证失败：立即隔离当前账号。
- account forbidden / model not allowed：立即隔离当前账号。
- 账号余额不足或 quota 不足：立即标记当前账号不可用。
- 明确 key 级限流：当前账号短暂降权或隔离。
- 首包超时触发内部切换：当前账号记录 `first_byte_timeout`，本次请求切换其他账号/渠道。

默认只影响当前账号，不拖低同渠道其他账号。

渠道级故障仍保留兜底：

- Base URL 连接失败。
- DNS/TLS/代理配置错误。
- 渠道整体禁用。
- 渠道并发上限满。
- 渠道配置错误影响所有账号。

## Probe Flow

探活继续复用正常流程，但目标改成账号级 runtime：

- 低健康分恢复探测：选择低分账号，而不是整个渠道。
- 低访问量激活探测：在近 30 分钟真实流量涉及的模型/分组中，选择低样本账号。
- 探测成功只提升对应账号的 runtime snapshot。
- 探测失败只影响对应账号。
- 系统探测仍不更新真实访问字段。

探测候选也必须走有界 Top-K，不做全账号巡检。

## UI Changes

调度详情：

- 候选卡展示渠道名、渠道 ID、账号序号、账号短指纹。
- 显示 `物化评分` 与 `实时压力修正`，避免误解为每次全量重算。
- 选中记录明确显示使用的账号。

评分变更记录：

- 支持按账号查看分数变化。
- 展示当前账号的 `ScoreItems`、delta、原因。
- 成本分同渠道同模型下通常一致，原因说明为“来自渠道成本配置”。

多账号管理：

- 每个账号展示当前评分、状态、最近成功、最近失败、最近错误原因。
- 支持禁用/启用单账号。
- 支持查看账号级健康检测记录。

健康检测页面：

- 默认按渠道聚合。
- 展开后展示账号级待检查队列与历史。

## Performance Targets

大规模目标：

- 1000 个渠道。
- 10 万账号。
- 单次调度不线性扫描账号。

默认参数：

- 单次候选上限：64。
- 探索账号：4-8。
- 候选解释展示上限：沿用 32。
- 只持久化有样本或被探活过的账号 snapshot。
- 冷账号不预建 DB 行。
- 索引变更异步批量刷新。

调度复杂度目标：

```text
O(logN + K)
```

其中 K 是有界候选数，而不是账号总数。

## Rollout

1. 先扩展 RuntimeKey 与数据结构，保持无账号字段兼容。
2. 实现 CredentialRef 和账号指纹生成。
3. 智能调度候选构建支持多账号展开，但先加候选上限。
4. Relay 固定使用 plan credential，避免二次随机。
5. 异步评分写入账号级 snapshot。
6. 建立 CredentialCandidateIndex。
7. UI 增加账号维度展示。
8. 开启账号级探活。
9. 大账号池压测后再默认启用。

## Test Plan

后端单测：

- 多账号渠道会生成多个账号级 runtime key。
- 账号指纹稳定，不暴露 raw key。
- OAuth token 刷新后 account_id 相同则指纹不变。
- 智能调度选中账号后，Relay 使用同一个 raw key。
- 非智能调度保持原有随机/轮询行为。
- 账号 A 失败只更新账号 A snapshot，不影响账号 B。
- 重试时可以从账号 A 切到同渠道账号 B。
- 认证失败立即隔离当前账号。
- 余额不足立即标记当前账号不可用。
- 10 万账号模拟下，单次调度候选数不超过上限。
- 异步评分能更新 score total、routing score、score items。
- 历史渠道级 snapshot 能作为账号冷启动 fallback。

回归测试：

```bash
go test ./pkg/modelgateway/...
go test ./controller -run 'TestModelGateway|TestObservability|TestConfig'
git diff --check
```

前端验证：

- 调度详情能看出当前候选是哪个账号。
- 评分变更记录能按账号解释变化。
- 多账号管理弹窗能展示账号级评分和状态。
- 健康检测页面能查看账号级待检查队列和历史。

## Assumptions

- 账号级智能调度仅在 smart scheduler active 模式生效。
- shadow 模式只记录建议账号，不改变实际 key。
- 默认只做账号级隔离，不因单个账号异常暂停整个渠道。
- 成本配置仍按渠道/模型维护；同渠道账号默认成本一致。
- 后续如需要账号级成本，可在 `CredentialRef` 或账号配置中扩展成本覆盖字段。
