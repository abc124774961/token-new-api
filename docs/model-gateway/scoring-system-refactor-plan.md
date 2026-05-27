# 智能调度评分体系实现方案

## 1. 背景与目标

当前智能调度评分存在几个核心问题：

- 调度、探活、运行时状态、评分历史等入口存在不同评分路径，容易出现同一渠道在不同页面分数不一致。
- `health_score`、`experience_score`、动态速度分等综合分概念过多，用户难以判断分数变化来自哪一项真实数据。
- 成本优先、性能优先、稳定优先存在继续分叉实现的风险，后续维护成本高。
- 请求结束后的评分变化缺少可追踪记录，无法明确一次请求让哪个渠道因为什么加分或扣分。

本方案目标：

- 统一评分入口：调度、探活、运行时状态、评分历史、请求详情全部走同一个评分服务。
- 扁平化评分：取消健康分、体验分、动态速度分等打包分值，所有参与总分的内容都作为一级评分项。
- 请求后评分可追踪：每个上游 attempt 结束后生成评分变化事件。
- 分组策略参数化：成本优先、性能优先、稳定优先只通过策略模板调整权重和参数，不复制 scorer。
- 降低热路径成本：候选集、成本基线、runtime snapshot、评分项定义尽量复用。

## 2. 总体架构

新增统一评分服务：

```text
CandidateScoringService
```

统一链路：

```text
Build Candidate Set
-> Build ScoringContext
-> Batch Enrich Runtime Snapshots
-> Resolve Cost Baseline Once
-> Apply StrategyProfile
-> Generate ScoreItems
-> Calculate Score
-> Select Candidate / Generate Explanation
```

所有入口必须复用这条链路：

- `DefaultSmartChannelSelector.Select`
- `DefaultSmartChannelSelector.ScoreCandidate`
- `ProbeScheduler`
- `RuntimeStatusService.applyScore`
- 评分历史 runtime-current 构建逻辑
- 请求详情中的评分变化展示
- 健康检查记录中的评分解释

`WeightedScoreCalculator` 可以保留为内部公式实现，但不允许 runtime、probe、history 等模块直接调用。

## 3. 核心数据结构

### 3.1 ScoringContext

用于一次评分请求的上下文，避免重复构建候选范围和成本基线。

```text
ScoringContext
- requested_model
- endpoint_type
- candidate_groups
- auto_mode
- strategy
- capability_filter
- cost_baseline_scope
- now
- explain_enabled
```

### 3.2 StrategyProfile

策略模板只控制权重和公式参数，不复制评分逻辑。

```text
StrategyProfile
- strategy
- score_item_weights
- formula_params
- missing_sample_policy
- cost_emphasis
- pressure_policy
- minimum_guard_weights
```

内置模板：

- `balanced`
- `speed_first`
- `cost_first`
- `stability_first`

### 3.3 ScoreItemDefinition

每个一级评分项由统一定义生成。

```text
ScoreItemDefinition
- key
- name
- category
- raw_value_resolver
- score_formula
- default_weight
- missing_policy
- formula_label
```

### 3.4 ScoreItem

前端展示、评分解释和变化追踪都使用同一结构。

```text
ScoreItem
- key
- name
- category
- raw_value
- window
- score
- weight
- weighted_score
- previous_score
- delta
- sample_count
- missing_reason
- formula
- reason
```

### 3.5 ScoreEvaluation

一次候选评分结果。

```text
ScoreEvaluation
- score_total
- routing_score_total
- score_items
- score_breakdown
- routing_score_breakdown
- state_tags
- cost_reference_missing
- context
```

兼容规则：

- `score_breakdown` 由 `score_items` 生成。
- `score_total` 为基础总分。
- `routing_score_total` 在基础总分上叠加实时压力和状态门控。

### 3.6 请求后评分相关结构

```text
ScoreSampleDecision
- score_sample
- real_user_metric
- dynamic_price_sample
- circuit_sample
- probe_recovery_sample
- skip_reason
```

```text
ScoreAdjustment
- before_total
- after_total
- delta
- sample_decision
- changed_items
- context
```

```text
ScoreEvent
- trace_id
- request_id
- attempt_index
- channel_id
- requested_model
- upstream_model
- group
- endpoint_type
- is_health_probe
- strategy
- auto_mode
- before_total
- after_total
- delta
- sample_decision_json
- changed_items_json
- context_json
- created_at
```

## 4. 一级评分项

最终公式：

```text
总分 = Σ(一级评分项得分 × 一级评分项权重)
```

所有参与总分的内容都必须是一级项。UI 可以分类折叠，但后端不再打包成健康分、体验分、动态速度分。

### 4.1 动态样本项

#### completion_rate

- 名称：完成率分
- 原始数据：完成样本数 / 评分样本总数
- 样本来源：真实上游 attempt + 探活 attempt
- 典型公式：`completed / total`
- 说明：衡量渠道能否正常完成请求。

#### upstream_error_rate

- 名称：上游错误率分
- 原始数据：可归因上游错误数 / 评分样本总数
- 典型公式：`1 - upstream_error_rate`
- 包含：上游 5xx、429、连接失败、首包前 responses failed、超时等。

#### ttft_latency

- 名称：首包速度分
- 原始数据：TTFT P50/P90
- 推荐公式：以 P90 为主，`inverse_latency_score(ttft_p90, 800ms, 20000ms)`
- 说明：首包前失败不写入 TTFT 样本，只进入错误率。

#### duration_latency

- 名称：完整耗时分
- 原始数据：Duration P50/P90
- 推荐公式：`inverse_latency_score(duration_p90, 3000ms, 90000ms)`
- 说明：中途流中断不作为正常完整耗时样本。

#### throughput

- 名称：吞吐速度分
- 原始数据：tokens per second P50/P90
- 推荐公式：`throughput_score(tps_p50, 5, 80)`
- 缺失 token 数据时标记 `sample_missing`。

#### empty_output_rate

- 名称：空输出率分
- 原始数据：空输出次数 / 成功响应样本数
- 典型公式：`1 - empty_output_rate`
- 说明：空输出独立扣分，不再塞进体验分。

#### stream_interrupted_rate

- 名称：流中断率分
- 原始数据：中途断流次数 / 流式样本数
- 典型公式：`1 - stream_interrupted_rate`
- 说明：中途断流同时影响完成率，不触发中途切换渠道。

### 4.2 实时压力项

#### concurrency_load

- 名称：并发负载分
- 原始数据：active concurrency / effective concurrency limit
- 说明：只反映当前调度压力，不写入长期样本。

#### queue_pressure

- 名称：队列压力分
- 原始数据：queue depth、estimated queue wait
- 说明：只影响当前路由选择。

#### first_byte_backlog

- 名称：首包积压分
- 原始数据：first byte pending、slow first byte pending、oldest first byte wait
- 说明：避免持续把请求打到首包卡住的渠道。

### 4.3 固定公式项

#### cost

- 名称：成本分
- 原始数据：当前渠道成本、候选范围内最低成本
- 默认公式：`min_cost / current_cost`
- 成本优先公式：`pow(min_cost / current_cost, 1.35)`
- 成本基线缺失时跳过该项并标记 `cost_reference_missing=true`。

#### group_priority

- 名称：分组分
- 原始数据：分组倍率或分组优先级配置
- 说明：体现运营配置，不和健康数据混在一起。

## 5. 策略模板

### 5.1 balanced

均衡策略，完成率、错误率、速度、成本、负载都保留中等权重。

### 5.2 speed_first

性能优先：

- 提高 `ttft_latency`、`duration_latency`、`throughput`、`concurrency_load`、`queue_pressure` 权重。
- `cost` 只保留低权重 tie-breaker。
- 压力项优先影响 `routing_score_total`。

### 5.3 cost_first

成本优先：

- 提高 `cost` 权重。
- 成本公式使用强化参数：`pow(min_cost / current_cost, 1.35)`。
- 保留 `completion_rate`、`upstream_error_rate`、`stream_interrupted_rate` 最低保护权重，避免低价坏渠道被打满分。

### 5.4 stability_first

稳定优先：

- 提高 `completion_rate`、`upstream_error_rate`、`stream_interrupted_rate` 权重。
- 降低成本权重。
- 慢速但稳定的渠道不应被速度项过度惩罚。

## 6. 分组与成本基线

成本基线只在 `CandidateScoringService` 中计算。

参考范围：

```text
requested_model
endpoint_type
candidate_groups
capability_filter
auto_mode
```

### 6.1 auto_sequential

- 当前分组先构建候选集。
- 成本基线只在当前分组候选中取最低成本。
- 当前分组不可用或跨组重试时，再切到下一组重新构建候选集。

### 6.2 auto_fusion

- 多个候选分组合并成一个候选集。
- 成本基线在融合候选集中取最低成本。
- `group_priority` 体现运营优先级，避免低成本分组无条件压过高优先级分组。

### 6.3 成本基线缺失

缺失时：

- 不返回假 `0.5`。
- `cost_reference_missing=true`。
- 跳过 `cost` 项。
- 有效权重重新归一化。

单次请求不会改变成本分，只有成本配置或候选范围变化才改变成本分。

## 7. 请求后评分调整

新增：

```text
ScoreAdjustmentService
ScoreSampleClassifier
```

每个上游 attempt 结束后统一处理：

```text
Normalize RuntimeKey
-> Compute Before ScoreItems
-> Classify Sample
-> Update ScoreStats
-> Compute After ScoreItems
-> Generate ScoreEvent
-> Persist Attempt Association
```

### 7.1 样本分类

进入评分样本：

- 真实上游 attempt 成功或失败。
- 探活成功或失败。
- 首包前上游失败。
- 中途流中断。
- 空输出。
- 可归因上游错误。

不进入评分样本：

- 用户主动取消。
- 余额不足。
- 本地并发限制直接拒绝。
- 未进入智能调度链路的失败。
- 配置错误隔离期间跳过。

探活规则：

- 探活样本进入评分，用于恢复渠道。
- 探活样本不进入用户真实成功率。
- 探活样本不进入动态价格统计。

重试规则：

- 按每个渠道 attempt 独立调分。
- 后续渠道成功不会回滚前面失败渠道的扣分。

### 7.2 ScoreAdjustment 示例

```json
{
  "before_total": 0.8123,
  "after_total": 0.7981,
  "delta": -0.0142,
  "sample_decision": {
    "score_sample": true,
    "real_user_metric": true,
    "dynamic_price_sample": false,
    "reason": "stream_interrupted"
  },
  "items": [
    {
      "key": "stream_interrupted_rate",
      "name": "流中断率分",
      "before_score": 0.96,
      "after_score": 0.91,
      "delta": -0.05,
      "weight": 0.06,
      "weighted_delta": -0.003,
      "before_raw_value": "1/25",
      "after_raw_value": "2/26",
      "reason": "本次请求中途流中断"
    }
  ]
}
```

## 8. 存储设计

### 8.1 Runtime Snapshot

`model_gateway_runtime_snapshots` 增加：

```text
score_stats_json TEXT
```

JSON schema：

```json
{
  "version": 1,
  "samples": 0,
  "rates": {
    "completion": {"success": 0, "total": 0, "ewma": 0},
    "upstream_error": {"count": 0, "total": 0, "ewma": 0},
    "empty_output": {"count": 0, "total": 0, "ewma": 0},
    "stream_interrupted": {"count": 0, "total": 0, "ewma": 0}
  },
  "latency": {
    "ttft_ms": [],
    "duration_ms": [],
    "tokens_per_second": []
  }
}
```

要求：

- latency ring buffer 最多保留 64 条。
- 计数项保留窗口计数和 EWMA。
- JSON 字段使用 TEXT，兼容 SQLite、MySQL、PostgreSQL。
- JSON 编解码必须使用 `common.Marshal / common.Unmarshal`。

旧字段继续兼容回填，但新版评分不再依赖：

- `SuccessScore`
- `SpeedScore`
- `ExperienceScore`
- `HealthScoreAverage`

### 8.2 Score Event

新增表：

```text
model_gateway_score_events
```

字段：

```text
id
trace_id
request_id
attempt_index
channel_id
requested_model
upstream_model
group
endpoint_type
is_health_probe
strategy
auto_mode
before_total
after_total
delta
sample_decision_json
changed_items_json
context_json
created_at
```

要求：

- 一次 attempt 最多一条 event。
- 异步写入，不阻塞用户请求。
- 只保存变化项和关键项，避免日志过大。
- JSON 字段使用 TEXT，兼容 SQLite、MySQL、PostgreSQL。
- JSON 编解码必须使用 `common.Marshal / common.Unmarshal`。

## 9. 前端实现

### 9.1 评分详情

评分详情页改为一级项表格：

```text
评分项 / 原始数据 / 窗口 / 得分 / 权重 / 加权贡献 / 变化 / 公式 / 原因
```

默认展示关键项：

- 完成率
- 上游错误率
- 首包速度
- 完整耗时
- 成本
- 并发/队列负载

其他项折叠到详情。

### 9.2 请求详情

增加“本次评分调整”：

- attempt 级评分变化。
- 总分 before / after / delta。
- 一级项变化。
- 探活标签。
- 跳过评分原因。
- 成本参考缺失提示。

### 9.3 运行时详情

增加：

- 最近评分变化时间线。
- 变化最大的一级项。
- 最近探活恢复记录。
- 成本参考缺失提示。

### 9.4 文案清理

移除展示文案：

- 健康分
- 体验分
- 输出质量分
- 动态速度分

## 10. 性能要求

- 候选集每次请求只构建一次。
- 成本基线每个 `ScoringContext` 只解析一次。
- runtime snapshot 批量补齐。
- `ScoreItemDefinition` 全局初始化复用。
- 调度热路径优先计算轻量总分。
- 完整 `score_items` 只在解释、记录、UI 展示时生成。
- runtime status 批量评分复用 scorer 和成本基线。
- 成本基线继续使用缓存，不在热路径查数据库。
- P50/P90 使用固定窗口或 ring buffer，不无限保存样本。

## 11. API 设计

新增或扩展：

```text
GET /api/model_gateway/score_events
GET /api/model_gateway/score_events/:trace_id
```

支持筛选：

```text
channel_id
requested_model
group
is_health_probe
score_item_key
request_id
from
to
```

请求详情接口返回：

```text
score_trace_id
score_adjustment
```

运行时状态和评分历史返回：

```text
score_items
cost_reference_missing
strategy
auto_mode
candidate_groups
```

## 12. 分阶段落地

### 阶段一：统一评分入口

- 引入 `CandidateScoringService`。
- 调度、探活、runtime-current、评分历史全部改为同一入口。
- 修复两套分数和成本分异常。

### 阶段二：扁平一级评分项

- 引入 `ScoreItemDefinition`。
- 新增 `score_items` 输出。
- 前端改为一级项展示。
- 旧字段兼容输出。

### 阶段三：请求后评分追踪

- 引入 `ScoreSampleClassifier`。
- 增加 `score_stats_json`。
- 增加 `model_gateway_score_events`。
- 请求详情展示本次评分调整。

### 阶段四：策略与性能优化

- 成本优先、性能优先、稳定优先全部改为 `StrategyProfile`。
- 完成批量评分、成本基线复用、热路径轻量化。

## 13. 测试计划

后端：

```bash
go test ./pkg/modelgateway/scheduler ./pkg/modelgateway/probe ./pkg/modelgateway/observability ./pkg/modelgateway/recording ./controller -count=1
```

前端：

```bash
cd web/classic && bun run i18n:sync && bun run build
```

必须覆盖：

- 调度、探活、runtime-current、评分历史同分。
- 成本基线缺失不出现 `0.5` 假扣分。
- 探活影响评分但不进入真实成功率和动态价格。
- 每个有效 attempt 生成 score event。
- 请求详情能展示 before / after / delta。
- `cost_first` / `speed_first` 只切换 profile，不复制 scorer。
- `auto_sequential` 成本基线只在当前分组候选内计算。
- `auto_fusion` 成本基线在融合候选集内计算。
- 余额不足不影响评分、熔断、成功率。
- 中途流中断影响流中断率分和完成率分。
- 空输出只影响空输出率分。

## 14. 约束与假设

- UI 可分类折叠一级项，但后端评分不再打包。
- 评分变化按上游 attempt 追踪。
- 探活是评分样本，但不是用户真实请求成功率样本。
- 动态价格只使用真实用户请求样本，不使用探活样本。
- 成本优先不能突破最低可用性保护。
- 性能优先保留成本 tie-breaker。
- 所有 JSON 存储保持跨数据库兼容。
