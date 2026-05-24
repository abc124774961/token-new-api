# 模型网关动态收费方案

## 背景

智能模型网关已经具备调度、观测和异步上游成本补算能力。下一步需要新增动态收费能力：平台只配置一个全局利润率，系统根据近期真实上游成本自动推导用户收费，同时不影响请求热路径性能。

## 评审结论

- 方案方向通过，采用 `真实上游成本异步补算 + 动态基准后台聚合 + 热路径只读缓存` 的总体架构。
- 动态收费只对 `policy_mode=active` 且 `smart_handled=true` 的真实智能调度请求生效。
- 动态收费命中后，直接作为最终用户价格，完全替代现有 `group_ratio`、`user_group_ratio` 和 `tiered_expr` 口径。
- 动态基准未命中、未就绪、过期或缓存异常时，统一回退到现有静态计费，并明确记录回退原因。
- 价格基准必须从真实上游成本的分项 breakdown 反推出可复用单价，不能只按历史总成本均值直接收费。

## 产品口径

- 动态收费仅对以下请求生效：
  - `policy_mode=active`
  - `smart_handled=true`
  - 已确定 `selected_group`
- 以下请求不参与动态收费，也不进入动态样本：
  - `shadow` 模式请求
  - 探活请求
  - 回放、测试、系统内部请求
  - 非智能链路请求
- 利润率采用加价率口径：
  - `sell = cost x (1 + profit_rate)`
- 动态收费命中后，直接作为最终用户价格，不再叠加：
  - `group_ratio`
  - `user_group_ratio`
  - `tiered_expr`
- 动态收费未命中时，完整回退现有静态计费逻辑，用户请求不报错、不阻塞。

## 运营要求

- 后台提供全局动态收费开关。
- 后台提供全局利润率配置。
- 后台提供样本窗口、最小样本数、刷新周期和价格波动护栏配置。
- 后台展示当前 baseline 状态：
  - 是否 ready
  - 最近刷新时间
  - 样本数
  - 当前动态价格每 M / 按次价格
  - 当前命中率 / 回退率
- 后台必须支持查看回退原因：
  - `not_ready`
  - `insufficient_samples`
  - `cache_not_loaded`
  - `baseline_expired`
  - `missing_key`
  - `worker_error`
- 后台必须支持价格波动护栏：
  - 单次最大涨幅
  - 单次最大跌幅

## 技术方案

### 1. 总体架构

采用三层结构：

- `request sample`：每个真实请求只标准化一次动态收费样本。
- `rollup`：按时间桶增量累计金额与量。
- `baseline`：按窗口折叠 rollup，生成最终动态计费基准并刷新内存缓存。

请求热路径只读取 `baseline cache`，不扫描历史日志，不实时重算上游成本。

### 2. 样本来源

动态收费样本来自现有异步上游成本汇总 `model_gateway_request_cost_summaries`，并结合 `breakdown_json` 与 usage 组件生成可复用计费样本。

禁止直接按 `upstream_cost_total` 做历史均价收费，必须从 breakdown 中拆出输入、输出、缓存、图片、音频、工具或按次价格等分项单价。

### 3. 聚合维度

统一 key：

`billing_key = requested_model + billing_group`

其中：

`billing_group = consume log.group > selected_group > requested_group`

热路径命中时，使用已确定的 `selected_group` 查询同口径 key，避免历史聚合口径与实时命中口径不一致。

### 4. 计费模式

动态收费模式采用确定性规则，不允许按近期样本自动切换：

- 固定按次价格模型：`request` 模式
- 其他智能网关文本模型：`token` 模式
- `tiered_expr` 模型在智能网关请求中也统一按 `token` 模式动态收费

### 5. 后台模块

新增 `pkg/modelgateway/dynamicbilling`，建议拆分：

- `SampleBuilder`
- `RollupWorker`
- `BaselineRefresher`
- `BaselineCache`
- `BaselineGuardrail`

职责分别为：

- 从真实成功请求中抽取标准化样本
- 增量写入时间桶聚合
- 定时生成 baseline
- 将 baseline 装载到内存缓存
- 在刷新时应用涨跌幅护栏

### 6. 预聚合存储

新增三层数据：

#### `model_gateway_dynamic_billing_request_samples`

- 每个 `request_id` 唯一
- 保存标准化后的动态收费样本
- 用于去重与审计

#### `model_gateway_dynamic_billing_rollups`

- 按 `billing_key + pricing_mode + bucket_start` 聚合金额与量
- worker 只增量处理新增 request sample

#### `model_gateway_dynamic_billing_baselines`

- `billing_key`
- `requested_model`
- `billing_group`
- `pricing_mode`
- token 模式的分项单价字段
- request 模式的 `request_price`
- `sample_count`
- `window_start`
- `window_end`
- `profit_rate`
- `ready`
- `fallback_only`
- `baseline_version`
- `calculated_at`

### 7. Ready 条件

只有以下条件同时满足，baseline 才允许用于动态收费：

- `sample_count >= dynamic_billing_min_samples`
- `calculated_at` 未超过 `dynamic_billing_max_age_seconds`
- baseline cache 已成功加载

否则统一静态回退，不允许半动态收费。

### 8. 热路径接入

在 `relay/helper/price.go` 增加动态收费入口：

- 仅当当前请求为 active 智能调度请求时尝试读取 `BaselineCache`
- 命中后构造动态计费快照并挂到 `RelayInfo`
- 未命中时直接走现有 `ModelPriceHelper` / `tiered_expr`

动态计费快照必须冻结以下字段，保证预扣费和结算口径一致：

- `dynamic_billing_key`
- `dynamic_pricing_mode`
- `dynamic_profit_rate`
- `dynamic_baseline_version`
- `dynamic_baseline_prices`

结算阶段禁止重新读取最新 baseline。

### 9. 样本排除规则

以下请求不得进入动态收费样本：

- `is_health_probe=true`
- `client_aborted=true`
- `stream_interrupted=true`
- `empty_output=true`
- `experience_issue != ""`
- 余额不足 / 配额不足 / 扣费失败
- `policy_mode=shadow`

### 10. 波动护栏与回退

新增配置：

- `dynamic_billing_max_step_up_ratio`
- `dynamic_billing_max_step_down_ratio`

每次 baseline 刷新时，相对上一版本的价格变动如果超过护栏，则截断到护栏边界，不直接跳变。

若当前窗口样本异常、样本数不足或聚合失败：

- 若上一版 baseline 仍未过期，则继续沿用上一版
- 若上一版 baseline 也过期，则统一静态回退

回退必须写明原因：

- `not_ready`
- `insufficient_samples`
- `cache_not_loaded`
- `baseline_expired`
- `missing_key`
- `worker_error`

## 配置项

在 `scheduler_setting` 中新增：

- `dynamic_billing_enabled`
- `dynamic_billing_profit_rate`
- `dynamic_billing_window_minutes`
- `dynamic_billing_min_samples`
- `dynamic_billing_refresh_seconds`
- `dynamic_billing_bucket_minutes`
- `dynamic_billing_max_age_seconds`
- `dynamic_billing_max_step_up_ratio`
- `dynamic_billing_max_step_down_ratio`

配置保存继续沿用现有 `scheduler_setting.*` option 存储方式。

## 观测与日志

消费日志与观测页补充：

- `billing_mode = "model_gateway_dynamic"`
- `billing_source_detail = "dynamic_baseline"`
- `dynamic_billing_applied`
- `dynamic_billing_fallback`
- `dynamic_fallback_reason`
- `dynamic_billing_key`
- `dynamic_pricing_mode`
- `dynamic_profit_rate`
- `dynamic_baseline_version`

以下界面和导出能力都需要能看出本次请求实际走的是动态收费还是静态回退：

- `/api/model_gateway/observability/summary`
- replay/export
- 详情侧栏

## 验收标准

- 仅 `policy_mode=active` 的智能请求启用动态收费。
- 动态收费命中后，不再叠加旧分组倍率和 `tiered_expr`。
- 动态基准未就绪时，统一静态回退，并记录回退原因。
- 请求热路径无新增历史明细查询。
- 请求内动态基准版本冻结，预扣费与结算一致。
- 单次价格刷新涨跌幅受护栏约束。
- 后台可查看 baseline 状态、样本数、刷新时间和回退原因。

## 测试

- `go test ./relay/helper ./service ./pkg/modelgateway/dynamicbilling ./controller`
- `cd web/classic && bun run i18n:sync`
- `cd web/classic && bun run build`

建议补充测试场景：

- active 智能请求动态收费命中
- shadow 请求不参与动态收费
- baseline 样本不足触发静态回退
- baseline 过期触发静态回退
- 动态收费命中后不叠加 `group_ratio`
- 同一请求预扣费与结算使用同一 `dynamic_baseline_version`
- 涨跌幅护栏生效

## 假设

- “近期收费表现”明确指近期真实上游成本表现，来源于现有异步成本汇总及其 breakdown。
- 动态收费第一阶段只覆盖智能网关 active 请求，不覆盖全站其他请求。
- 动态收费命中后，旧用户分组价格体系完全不参与本次计费。
- 动态基准以 `requested_model + billing_group` 为主维度，避免按 channel 直接把价格抖动传给用户。
- 第一阶段优先保证性能、计费稳定性和可解释性，不做自动模式切换和更复杂的分层利润策略。
