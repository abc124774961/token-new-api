# Model Gateway Retry Routing Intents

## 背景

首包超时通常说明当前上游通道已经不适合继续承接这一次流式请求。普通重试只会重新走一次渠道选择，而重试意图会把“这次切换是为了从首包超时里恢复”显式传给调度器，让下一次选择更偏向低 TTFT 和高成功率的通道。

当前支持的意图：

- `first_byte_recovery`：由首包超时触发的切换恢复。

## 触发条件

首包超时不会无条件改变后续路由，只有同时满足以下条件时才写入意图：

- 当前尝试命中首包超时；
- `shouldRetry` 判定仍有可切换的候选通道；
- 本次重试动作是 `switch_channel`；
- 当前失败通道已通过 `MarkChannelSelectionSkipped` 加入本次请求的跳过集合。

这样不会扩大原有重试预算，也不会把刚超时的通道再次选回来。

## 路由评分

调度器会在路由评分项里追加 `retry_intent_recovery`，仅参与 `RoutingScoreTotal`，用于下一次实际通道选择。该项综合：

- 完成率/成功率；
- 首包延迟 TTFT，低 TTFT 得分更高；
- 首包等待积压，积压越低越优先。

该评分项不会修改成本展示和模型计费，只影响这一次恢复性切换的路由倾向。

## 队列优先级

`first_byte_recovery` 会把计划的 `QueuePriority` 提升到 `RetryRoutingQueuePriority`。队列侧做两件事：

- 默认给高优先级重试保留 1 个额外等待深度，避免普通队列满时恢复重试直接被拒绝；
- 同一通道队列内按优先级尝试获取并发，优先级相同再按入队顺序，避免高优先级重试只是“排进去但不先跑”。

如果运维已配置队列公平性，现有配置会保留；只会把高优先级阈值向 `RetryRoutingQueuePriority` 兼容，确保恢复重试能被识别为高优先级。

## 记录字段

执行记录的 `request_meta` 会写入：

- `retry_routing_intent`：意图内容，包括失败通道、触发原因、策略和队列优先级；
- `retry_intent_applied`：本次计划是否应用了重试意图；
- `retry_queue_priority_boost`：是否启用了队列优先级提升；
- `queue_priority`：最终队列优先级；
- 候选解释中的 `retry_intent_applied`、`retry_intent_reason` 和 `retry_intent_recovery` 分项。

这些字段用于后续排查“为什么切到了这个通道”和评估恢复切换的收益。

## 防循环边界

重试意图不是新的重试循环，它只影响下一次选择：

- 不增加额外重试次数，只复用已有 failover 预算；
- 失败通道进入本次请求跳过集合；
- 成功选出新的智能调度计划后立即清除 gin context 上的意图；
- 如果没有可切换通道，`shouldRetry` 返回 false，不会设置意图；
- 队列优先级只影响等待顺序和有限的额外深度，不会绕过通道并发上限。

后续如果新增类似需求，应复用 `RetryRoutingIntent`，明确触发条件、评分项、记录字段和消费时机，避免把一次性恢复信号扩展成长期路由偏置。
