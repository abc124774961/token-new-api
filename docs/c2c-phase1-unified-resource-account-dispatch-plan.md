# C2C 一期：统一资源模型、账号级调度与评分底座

本文是总需求文档中“一期：统一资源模型、账号级调度与评分底座”的独立实施方案。总需求文档见：[C2C 供应渠道账号平台与智能调度解耦方案](./c2c-supplier-account-plan.md)。

一期先把现有渠道管理升级为统一资源模型，并完成账号级智能调度、评分、探活和候选索引底座。C2C 不在这一期作为业务闭环上线，但一期产物必须能支撑二期 C2C 直接接入。

## 核心设计决策

- 一期先做平台自营资源底座，不做供应商注册、供应商授权、收益、提现和结算闭环。
- 现有 `model.Channel` 不废弃，一期作为执行绑定和管理员入口；调度视角通过兼容适配升级为 `SupplyChannel`。
- 一个渠道下的单 key、多 key、OAuth JSON、token-key 都要展开成独立 `AccountIdentity`，不能再藏在渠道内部随机/轮询。
- 账号身份和资源绑定解耦：`AccountIdentity` 表示上游账号是谁，`ResourceAccountBinding` 表示账号挂在哪个资源/渠道下。
- 一期导入去重可使用 `channel_id + brand + credential_subject_fingerprint`，但账号身份本身使用 `provider + brand + credential_subject_fingerprint`。
- 智能调度直接选择账号和 `CredentialRef`，relay 执行必须使用 `DispatchPlan` 中的 credential。
- 评分体系复用现有最新评分引擎，只把 `RuntimeKey` 和候选对象下沉到账号维度。
- 成本分只表示平台/上游运营成本，由运营成本资料和相关候选集合后台物化；用户侧分组倍率、模型倍率、套餐、折扣不进入成本分。
- 请求链路只读取候选索引和物化评分，不全量扫描账号，不现场重算全量评分。

## 一期需求

业务需求：

- 现有渠道管理继续作为管理员主入口，但底层要升级为可扩展的资源/账号体系。
- 渠道列表需要新增“账号管理”入口，进入某个渠道下的账号管理列表页。
- Codex/OpenAI 类型渠道需要支持多账号批量导入、追加导入、去重、导入结果回显和账号管理列表。
- 一个渠道下的单 key、多 key、OAuth JSON、token-key 等都要被抽象成可解释、可评分、可探活、可隔离的账号。
- 一期导入去重按“渠道 + 品牌 + 账号凭证身份”确定，同一个渠道同一个品牌下不能重复创建同一份账号凭证；长期账号身份不绑定到渠道。
- 运行时账号选择不再使用渠道内部随机/轮询，统一由新的智能调度选择。
- 调度详情、评分变更记录、健康检测记录需要能解释到账号维度。

技术需求：

- 现有 `model.Channel` 不能废弃，需要通过兼容适配接入 `SupplyChannel`。
- 现有 `Channel.Key`、`ChannelInfo`、多 key 状态、OAuth/JSON auth、token-key 等凭证形态要统一进入 `AccountIdentity + CredentialRef`。
- Codex 多账号导入不能只塞进一个 channel key 字符串里，需要拆成多个稳定 `AccountIdentity`。
- `account_identity_key` 按 `provider + brand + credential_subject_fingerprint` 生成，表示上游账号身份本身；一期导入去重可临时使用 `channel_id + brand + credential_subject_fingerprint` 作为渠道绑定唯一键。
- 二期不把账号唯一键升级成 `resource_id + brand + credential_subject_fingerprint`，而是新增资源账号绑定关系，用 `resource_id + account_id` 或独立 `resource_account_binding_id` 表达账号挂在哪个供应渠道下。
- Codex/OAuth JSON 凭证的身份指纹必须来自稳定账号主体，不能来自会刷新的 access token、过期时间或原始 JSON 字符串整体。
- 现有评分引擎、探活、运行态快照、评分事件要复用同一套链路，只扩展 runtime key 粒度。
- relay 必须使用调度计划里选中的 credential，不能在执行阶段再次调用随机/轮询选 key。
- 请求链路不能全量展开账号，必须使用有界候选索引和后台物化评分。

非目标：

- 不做供应商注册、供应商侧 OAuth 授权流程、收益、提现、结算闭环；一期只支持管理员导入现有 Codex/OpenAI JSON/OAuth 凭证。
- 不做 C2C 外部供应商资源正式接入，只为二期预留 `supplier_owned`。
- 不改变 C 端用户 API、模型列表、账单展示。
- 不暴露 raw key、完整凭证、供应商身份给普通终端用户。

## 一期目标

产品目标：

- 管理员能从“渠道”自然过渡到“渠道账号”管理，不丢失现有操作习惯。
- 管理员能在 Codex/OpenAI 渠道下批量导入多个账号凭证，并清楚看到新增、重复、无效、更新的结果。
- 管理员能看到某个渠道下有哪些账号、账号状态如何、最近运行是否健康。
- 管理员能理解智能调度为什么选中某个账号、为什么过滤某个账号。

技术目标：

- 建立 `ResourceRef / SupplyChannel / AccountIdentity / CredentialRef` 统一抽象。
- 将现有平台渠道作为 `platform_owned` 资源接入。
- 将单 key、多 key、OAuth JSON、token-key 统一转换成账号候选。
- 建立 Codex 多账号导入解析、唯一性识别、凭证归档和账号索引刷新链路。
- 将 `RuntimeKey`、`RuntimeSnapshot`、`ScoreEvent` 扩展到账号维度。
- 通过 `CredentialResolver` 和 `RelayCredentialInjector` 固定执行凭证。
- 让运营成本和能力配置可继承渠道/模型/账号维度，分组只作为策略权重，质量和健康指标独立到账号维度。

运营目标：

- 运营能按渠道、账号、模型、分组排查问题。
- 单个账号异常不误伤整个渠道。
- 重复导入、凭证失效、账号认证失败、账号被隔离都能在列表和记录里定位。
- 探活、过滤、评分变化都有账号级记录。
- 成本分、健康分、过滤原因能在调度详情中解释清楚。

## 一期任务点

1. 资源与账号抽象

   - 新增 `ResourceRef`、`SupplyChannel`、`AccountIdentity`、`CredentialRef` 概念。
   - 建立 `ChannelCompatibilityAdapter`，把现有 `model.Channel` 包装成 `platform_owned` 资源。
   - 建立 `CredentialAccountRegistry`，从单 key、多 key、OAuth JSON、token-key 生成稳定账号身份。
   - 定义 `resource_id`、`account_id`、`account_type`、`credential_index`、`credential_subject_fingerprint`、`credential_fingerprint` 规则。
   - 一期导入去重按 `channel_id + brand + credential_subject_fingerprint` 约束，二期通过 `ResourceAccountBinding` 表达资源与账号的绑定关系，不改变账号身份唯一键。

2. Codex 多账号导入与管理

   - Codex/OpenAI 渠道账号页支持批量粘贴或上传多份 JSON auth / OAuth credential。
   - 导入后逐条解析账号主体、品牌、凭证类型、有效性和短指纹。
   - 同一个渠道同一个品牌下，重复凭证只更新或跳过，不创建重复账号。
   - 导入结果返回新增、更新、重复、无效和失败原因，不返回 raw credential。
   - 账号管理列表支持查看、筛选、启用、禁用、删除/归档、手动探活、查看评分变更记录。

3. 候选构建与智能调度

   - 将候选构建从 channel 维度升级为 account 维度。
   - 建立 `AccountCandidateIndex`，按 `requested_model + group + endpoint_type + required_tools` 组织候选。
   - 单次请求默认只取 Top-K + 探索候选，避免全量扫描。
   - 去掉渠道内部随机/轮询作为正式运行逻辑，由智能调度选择账号。
   - retry 支持同执行绑定下切换其他账号。
   - Codex 多账号候选按账号级物化评分进入候选池，不能按导入顺序、随机或轮询选择。

4. 执行凭证链路

   - 建立 `CredentialResolver`，只在 relay 执行前解析 raw credential。
   - `DispatchPlan` 携带 `CredentialRef`。
   - `RelayCredentialInjector` 将 plan credential 写入执行上下文。
   - relay 执行阶段禁止二次 `GetNextEnabledKey()` 覆盖调度结果。
   - shadow 只用于验证和对比，不作为长期运行路径。

5. 评分与探活

   - `RuntimeKey` 增加账号维度。
   - `RuntimeSnapshot`、`model_gateway_runtime_snapshots`、`ModelGatewayScoreEvent` 增加账号字段。
   - 复用现有 `AttemptResult -> RuntimeHealthMonitor -> CandidateScoringService` 链路。
   - 完成率、错误率、首包、耗时、吞吐、空输出、流中断、隔离状态按账号更新。
   - 运营成本、能力匹配继承渠道/模型/账号配置；分组优先级只作为策略权重，不进入成本分。
   - Codex 多账号每个账号独立计算健康分、体验分、成本分、路由分和隔离状态。
   - 用户请求完成后只记录 attempt fact，评分刷新走后台任务；认证失败、凭证不可用等硬状态可即时写入账号状态。
   - 低健康分恢复探测、低访问量激活探测按账号 runtime key 执行。

6. 渠道账号管理页面

   - 渠道列表操作列新增“账号管理”。
   - 新增渠道账号管理列表页。
   - 单 key 渠道也展示默认账号。
   - Codex/OpenAI 渠道账号页提供“导入账号”入口，支持批量新增和追加导入。
   - 列表展示账号序号、品牌、账号类型、凭证类型、短指纹、唯一性状态、状态、禁用原因、最近成功、最近失败、最近探活、当前评分、操作。
   - 初期复用现有多 key 查询、禁用、启用、删除能力，但交互升级为独立页面。

7. 观测与解释

   - 调度详情展示资源类型、渠道 ID、账号序号、短指纹、过滤条件、当前调用工具。
   - 调度详情展示账号唯一键摘要、品牌、凭证类型、账号级评分拆解和命中/过滤原因。
   - 评分变更记录支持按账号查看。
   - 健康检测待检查队列和历史支持账号维度。
   - 兼容历史 channel 级 score event 和 snapshot 查询。

## 一期实施拆解

建议按下面顺序实施，避免只做 UI 或只做数据结构导致调度结果不可解释。

1. 账号抽象与导入底座

   - 定义 `AccountIdentity`、`CredentialRef`、账号主体指纹、凭证版本指纹。
   - 建立 Codex/OpenAI 多账号导入解析器。
   - 实现导入去重、导入结果明细、raw credential 加密存储。
   - 单 key、多 key、OAuth JSON、token-key 统一接入账号 registry。

2. 渠道账号管理页面

   - 渠道列表增加“账号管理”入口。
   - 新增渠道账号列表页，单 key 渠道也展示默认账号。
   - 支持批量导入、追加导入、启用、禁用、删除/归档、手动探活入口。
   - 列表展示品牌、账号类型、凭证类型、短指纹、状态、最近成功/失败/探活、评分摘要。

3. 候选索引与智能调度改造

   - `CandidatePoolBuilder` 从 channel 候选升级为 account 候选。
   - `AccountCandidateIndex` 使用有界候选，不在请求链路全量展开账号。
   - `DispatchPlan` 写入 `ResourceRef + AccountIdentity + CredentialRef`。
   - legacy 随机/轮询只保留迁移兜底，并在调度详情显式标记。

4. relay 凭证固定

   - `RelayCredentialInjector` 将 plan credential 写入执行上下文。
   - provider adapter 不得再次调用 `GetNextEnabledKey()` 覆盖计划凭证。
   - shadow/legacy fallback 只用于迁移期验证，不作为长期正式路径。

5. 账号级评分与探活

   - `RuntimeKey`、`RuntimeSnapshot`、`ModelGatewayScoreEvent` 增加账号字段。
   - 真实请求和探活继续使用同一条评分链路。
   - 成本分改为后台物化，只响应运营成本和相关候选集合变化。
   - 账号 A 的失败、隔离、探活结果只影响账号 A。

6. 观测解释与回归

   - 调度详情展示账号维度、当前调用工具、过滤条件和评分拆解。
   - 评分变更记录支持账号查询。
   - 健康检测队列和历史支持账号维度。
   - 补齐多账号、relay 凭证固定、账号级评分、成本分物化、候选上限测试。

## 一期验收

功能验收：

- 渠道列表存在“账号管理”入口，并能进入对应渠道账号列表页。
- 单 key 渠道展示一个默认账号。
- 多 key 渠道展示多个账号，并能看出启用/禁用状态。
- Codex/OpenAI 渠道支持一次导入多份账号凭证，并能返回新增、重复、无效、更新的明细。
- 同一渠道同一品牌下重复导入同一账号凭证，不会产生重复账号。
- API key、OAuth JSON、token-key 至少能归入统一账号抽象。
- 智能调度选中账号后，relay 使用同一个 credential 执行。
- retry 可以切换到同渠道下另一个可用账号。
- 账号管理列表能按渠道、品牌、账号类型、状态、短指纹、评分状态筛选。

评分验收：

- 同一个渠道下多个账号能生成多个账号级 runtime key。
- Codex 多账号之间的成功率、首包、流中断、空输出、隔离状态互不污染。
- 账号 A 失败只更新账号 A 的 snapshot、score event、隔离状态。
- 探活结果只影响目标账号 runtime key。
- 历史 channel 级 snapshot 只作为冷启动 fallback。
- 评分变更记录能解释账号级分数变化原因。
- 成本分不因单次请求动态变化，只在运营成本配置、账号成本覆盖或相关候选集合变化后刷新。
- 智能调度选择账号时使用账号级物化评分和实时压力修正，不使用随机/轮询。

性能验收：

- 请求链路不全量扫描账号。
- 10 万账号模拟下，单次调度候选数不超过配置上限。
- 不为冷账号预建大量 DB 行。
- 只持久化有样本、探活、隔离或被调度过的账号 snapshot。

兼容验收：

- 现有渠道列表、编辑、启用、禁用、删除等主流程可继续使用。
- 现有多 key 的导入、追加、禁用、启用、删除能力可继续在账号管理页使用。
- Codex 原有 JSON auth / OAuth key 形态可以通过账号 registry 兼容导入。
- 历史 snapshot、score event 不强制迁移，仍可查询。
- 新增 DB 字段兼容 SQLite、MySQL、PostgreSQL。
- 普通终端用户不可见 raw key、完整凭证、账号短指纹、供应商身份。

## 一期现状基线

当前系统已经具备以下能力，一期应在这些能力上增量升级：

- 渠道管理已有单 key、多 key、随机/轮询、key 禁用/启用、key 删除等能力。
- Codex/OpenAI 已有 OAuth、JSON auth、token refresh、proxy、relay 适配等能力，但还没有统一的账号级多凭证导入和智能调度选择能力。
- 智能调度已有 `DispatchRequest`、`DispatchPlan`、`RuntimeKey`、`CandidatePoolBuilder`、`RuntimeSnapshotStore`、`CandidateScoringService`。
- `RuntimeKey` 当前维度为 `requested_model + upstream_model + channel_id + group + endpoint_type + capability_fingerprint`，缺少账号/凭证维度。
- `RuntimeSnapshot` 和 `model_gateway_runtime_snapshots` 已有成功率、首包、耗时、吞吐、空输出、体验异常、真实访问、探活、隔离等字段。
- `ModelGatewayScoreEvent` 已有评分变更记录，但还不能按账号解释变化。
- 探活已经具备近 30 分钟真实流量门槛、低健康分恢复探测、低访问量激活探测、runtime key 限频。
- 当前 relay 前后仍大量依赖 `channel.GetNextEnabledKey()` 和 `ContextKeyChannelKey`，智能调度还没有固定到具体账号/凭证。

一期核心改造不是新增 C2C 页面，而是把现有渠道、多 key、评分、探活升级成可承载 C2C 的统一底座。

## 产品评审

产品目标：

- 管理员仍从现有渠道管理理解系统，不因为一期改造失去原有操作路径。
- Codex/OpenAI 渠道支持批量导入多个账号，并以账号列表方式管理，而不是要求管理员拆成多个渠道。
- 多 key 渠道从“一个渠道里随机/轮询 key”升级为“一个渠道下多个可解释账号资源”。
- 调度详情要能让管理员看懂为什么选中某个账号、为什么过滤某些账号。
- 一期不对普通终端用户暴露供应商、账号、成本、资源来源等信息。

产品范围：

- 渠道列表仍保留原有入口和主要操作。
- 渠道列表操作列新增“账号管理”，点击跳转到当前渠道的账号管理列表页面。
- Codex/OpenAI 渠道账号管理页新增“导入账号”能力，支持批量粘贴、追加导入、重复识别、导入结果明细。
- 多 key 管理保留现有导入、追加、删除、禁用、启用能力。
- 账号管理列表页面初期承接现有多 key 管理能力，后续扩展账号级评分、探活、过滤、隔离记录。
- 调度详情、评分变更记录、健康检测记录新增账号维度展示。
- 资源来源先只展示 `platform_owned`，二期再出现 `supplier_owned`。

产品不做：

- 不做供应商注册、OAuth 授权、收益页、提现页。
- 不改变 C 端用户 API、模型列表、账单展示。
- 不把账号短指纹、key 序号暴露给普通终端用户。

产品验收：

- 管理员能在调度详情中看到选中的是哪个渠道、哪个 key 序号、哪个短指纹。
- 管理员能从渠道列表进入该渠道的账号管理列表页。
- 管理员能在 Codex/OpenAI 渠道下批量导入多个账号凭证，并能看到每条导入结果。
- 账号管理列表能展示当前渠道下所有账号/key 的序号、品牌、账号类型、凭证类型、短指纹、唯一性状态、启用状态、禁用原因、最近使用状态。
- 管理员能在评分变更记录中看到账号级分数变化原因。
- 管理员能区分“渠道被过滤”和“某个账号被过滤”。
- 管理员不再需要为多 key 渠道选择随机或轮询策略；账号选择由智能调度解释。

## 运营评审

运营目标：

- 一期先服务内部平台自营资源运营，提前验证后续 C2C 供应商资源的管理模型。
- 运营人员需要能定位单个账号异常，而不是只能看到整个渠道异常。
- 账号异常、探活、过滤、评分变化要能形成可追踪记录，减少误判。

运营关注点：

- 多 key 中某一个 key 失败，不能导致整个渠道被错误拉低。
- Codex 多账号里某一个账号认证失败、额度不足、代理异常或频繁中断，不能污染其他账号评分。
- 重复导入应该被明确标记为重复，不能悄悄生成多个同身份账号。
- 探活记录要展示探活原因：低健康分恢复、低访问量激活、手动探活。
- 过滤原因要区分容量、并发、配置隔离、账号不可用、工具不支持、分组不匹配。
- 成本分变化要说明来自后台物化刷新，不是单次用户请求造成。
- 健康检测默认不做全量巡检，只在近 30 分钟有真实流量时激活相关资源。

运营不做：

- 不做供应商结算运营。
- 不做供应商风控处罚。
- 不做提现、对账、发票等财务运营。

运营验收：

- 一个多 key 渠道里禁用某个 key 后，运营能在账号维度看到该 key 不再参与智能调度。
- 一个 Codex 渠道批量导入多个账号后，运营能按品牌、状态、短指纹、最近失败原因定位问题账号。
- 运营可以从渠道列表进入账号管理页，查看单个账号的最近成功、最近失败、最近探活、当前评分和隔离原因。
- 某个账号发生认证失败时，只隔离该账号，并能看到隔离原因。
- 低访问量激活探活不会扫描无关模型、无关分组、无真实流量的资源。
- 调度详情能明确展示当前调用工具和过滤条件，避免误解候选数量。

## 技术评审

技术目标：

- 在不破坏现有渠道管理入口和数据兼容的前提下，引入账号级资源维度。
- 账号体系要支持多种执行身份形态，不能写死为 C2C 供应渠道账号或 OAuth 账号。
- 技术架构要区分供应商登录主体、供应渠道、大模型品牌账号三层，不能把三者混成一个表或一个概念。
- Codex 多账号导入后必须形成多个独立 `AccountIdentity`，并通过唯一键防止重复账号。
- 运行时账号选择统一走新智能调度，做到“调度器选哪个 credential，relay 就用哪个 credential”。
- RuntimeSnapshot、ScoreEvent、探活、候选索引统一使用账号级 RuntimeKey。
- 单次调度只处理有界候选，不能按 10 万账号线性扫描。

## 一期技术架构

一期采用“现有渠道兼容 + 统一资源账号抽象 + 账号级智能调度”的架构。现有 `model.Channel` 不废弃，但从调度视角被包装成 `SupplyChannel`；现有单 key、多 key、token-key、OAuth JSON 等凭证都被包装成 `AccountIdentity + CredentialRef`。

架构分层：

- `ChannelCompatibilityAdapter`：读取现有 `model.Channel`、`ChannelInfo`、`Key`、`Setting`、`OtherSettings`，生成 `SupplyChannel` 和 `AccountIdentity`。
- `CredentialAccountRegistry`：维护账号身份索引，负责从现有渠道 key、多 key、token-key、JSON auth 生成稳定 `account_id` 和 `credential_fingerprint`。
- `CredentialResolver`：只在 relay 执行前用 `CredentialRef` 解析 raw credential，并写入执行上下文。
- `AccountCandidateIndex`：按模型、分组、endpoint、工具能力维护有界候选，不在请求链路扫描所有账号。
- `AccountRuntimeStore`：基于现有 `RuntimeSnapshotStore` 扩展账号维度，保存账号级运行态。
- `AccountScoringBridge`：复用现有 `CandidateScoringService`、`RuntimeHealthMonitor`、`ScoreEventRecorder`，只改变 runtime key 粒度。
- `RelayCredentialInjector`：在智能调度选出账号后，将 plan credential 注入 relay，禁止再次随机/轮询选 key。

一期数据来源：

- 单 key 渠道：生成一个默认账号，`account_type = api_key` 或由渠道类型推导，`credential_index = 0`。
- 多 key 渠道：每个启用 key 生成一个账号；禁用 key 进入账号列表但不进入可调度候选。
- OAuth/JSON auth 渠道：优先从 payload 中提取稳定账号标识；无法提取时使用规范化凭证指纹作为账号 ID 的输入。
- token-key 模式：按可执行 token-key 生成账号，`account_type = token_key`。
- 未来 C2C：二期由 C2C 平台直接提供 `SupplyChannel + AccountIdentity + CredentialRef`，不再依赖 `model.Channel.Key` 拆分。

## Codex 多账号导入方案

一期要先支持现有管理员在 Codex/OpenAI 类型渠道下导入和管理多个账号凭证。这里的“多账号”不是一个渠道里的随机 key 池，而是同一个渠道资源下的多个可执行账号身份。

导入入口：

- 渠道列表操作列进入“账号管理”页面。
- 账号管理页提供“导入账号”按钮。
- 导入方式支持批量粘贴和文件上传，初期以文本粘贴为主。
- 支持 JSON 数组、按行 JSON、单个 JSON auth、已有 Open Codex / Codex OAuth credential 文本。
- 导入模式默认是“只导入新增”，重复账号跳过；可选“更新已有凭证”，用于刷新同账号 credential。

导入解析：

- 后端按条目拆分，逐条解析，不因为某一条失败导致整批失败。
- 每条凭证解析出 `brand`、`provider`、`account_type`、`credential_type`、`credential_subject_fingerprint`、`credential_fingerprint`。
- Codex JSON auth 优先解析稳定账号主体，例如 upstream account id、JWT subject、授权账号标识。
- 如果无法解析稳定主体，但凭证结构合法，则使用规范化后的长期 credential 指纹作为兜底主体指纹，并标记 `subject_source = credential_fallback`。
- access token、expires_at、临时 session 字段不参与主体指纹，避免刷新后同一账号变成新账号。
- refresh token、账号主体、issuer、client id 等长期身份信息可参与不可逆 HMAC 输入，但 raw value 不落日志、不返回前端。

导入结果：

- 每条返回 `row_index`、`status`、`brand`、`account_type`、`short_fingerprint`、`reason`。
- `status` 包括 `created`、`updated`、`duplicate_skipped`、`invalid`、`conflict`。
- `duplicate_skipped` 表示同一渠道同一品牌下已经存在同一账号凭证身份。
- `conflict` 表示主体相同但凭证类型、provider 或关键归属不一致，需要人工确认。
- 前端导入完成后刷新账号列表和候选索引状态，不展示 raw credential。

保存规则：

- 新增账号写入账号注册表和凭证存储，生成 `AccountIdentity` 与 `CredentialRef`。
- 重复账号在“只导入新增”模式下不修改现有凭证、不改变状态、不覆盖备注。
- “更新已有凭证”模式只更新 credential secret 版本和可执行状态，不重置历史评分。
- 删除账号默认建议做归档，保留历史评分事件和 runtime snapshot 查询能力。
- 导入后不立即把所有账号写入 snapshot；只有真实请求、探活、隔离、手动操作后才生成运行态数据。

## 账号唯一性与绑定方案

账号身份和资源绑定需要解耦。一期为了兼容现有渠道管理，导入去重可以先落在渠道作用域；二期 C2C 接入后，不应该把账号唯一键升级成资源作用域，而应该让账号身份独立存在，再通过绑定关系挂到供应渠道、平台渠道或合作方资源下。

核心判断：

- `AccountIdentity` 表示“这个上游账号是谁”，应该由 provider、brand、账号主体指纹决定。
- `ResourceAccountBinding` 表示“这个账号挂在哪个资源/渠道下，以什么策略参与调度”，应该由 resource 和 account 的绑定关系决定。
- `CredentialRef` 表示“这次执行用哪份凭证版本”，可以随 token refresh 或 JSON auth 更新。
- 二期如果用 `resource_id + brand + credential_subject_fingerprint` 作为账号唯一键，会让同一个上游账号在不同资源下变成多个账号身份，不利于风控、审计、代理绑定和后续迁移。

一期兼容键：

```text
phase1_channel_account_binding_key =
  channel_id + ":" +
  brand + ":" +
  credential_subject_fingerprint
```

二期目标模型：

```text
account_identity_key =
  provider + ":" +
  brand + ":" +
  credential_subject_fingerprint

resource_account_binding_key =
  resource_id + ":" +
  account_id
```

字段含义：

- `channel_id`：一期执行绑定，来自现有 `model.Channel.ID`。
- `resource_id`：资源 ID，一期可由 `platform:channel:{channel_id}` 生成，二期切换为 C2C 供应渠道、平台自营资源或合作方资源 ID。
- `brand`：大模型品牌或生态，例如 `codex`、`openai`、`claude`、`gemini`。
- `provider`：凭证和账号来源，例如 `codex`、`openai_oauth`、`xautojs`、`manual_api_key`。
- `account_id`：账号身份 ID，来自 `account_identity_key`，不随绑定资源变化。
- `credential_subject_fingerprint`：账号主体指纹，用于识别“这是不是同一个上游账号”。
- `credential_fingerprint`：凭证版本指纹，用于识别“这份可执行凭证是否变化”。

主体指纹来源优先级：

1. 上游账号稳定 ID，例如 OAuth subject、JWT subject、upstream account id。
2. 授权账号稳定声明，例如 issuer + subject、provider + account id。
3. 规范化长期 credential 指纹，例如去掉 access token、expires_at、临时 session 后的 refresh credential 指纹。
4. 无法解析时标记为 `unresolved_subject`，只能作为单账号兜底，不允许批量静默合并。

一期导入去重处理：

- 新增时先查 `channel_id + brand + credential_subject_fingerprint`。
- 已存在且 credential 版本相同：返回 `duplicate_skipped`。
- 已存在但 credential 版本不同：根据导入模式返回 `duplicate_skipped` 或更新 credential version。
- 主体相同但 brand/provider 冲突：返回 `conflict`。
- 主体无法解析的条目，不参与跨条目合并，只能生成带告警的兜底账号。

二期绑定处理：

- 新增账号时先按 `provider + brand + credential_subject_fingerprint` 查找或创建 `AccountIdentity`。
- 供应渠道接入时创建 `ResourceAccountBinding`，把 `resource_id` 绑定到已有或新建 `account_id`。
- 同一个 `resource_id + account_id` 只能存在一个有效绑定，避免同一供应渠道重复绑定同一账号。
- 是否允许同一个 `account_id` 绑定多个资源，由 C2C 业务策略控制；默认建议一个上游账号只能被一个供应商有效绑定，平台自营迁移可走显式转移流程。
- 账号凭证刷新只更新 `CredentialRef` 或 credential secret version，不改变 `AccountIdentity`，也不改变已有绑定。

数据库约束建议：

- 一期账号注册表保留 `channel_id`、`resource_id`、`brand`、`account_id`、`credential_subject_fingerprint`、`credential_fingerprint`、`archived`。
- 为兼容 SQLite、MySQL、PostgreSQL，不依赖 partial index。
- 一期唯一索引建议使用 `channel_id + brand + credential_subject_fingerprint + archived`，归档账号用 `archived = true` 保留历史。
- 二期拆出 `account_identities` 和 `resource_account_bindings` 后，账号身份唯一索引用 `provider + brand + credential_subject_fingerprint`，绑定唯一索引用 `resource_id + account_id + archived`。
- `account_id` 使用稳定生成规则，不使用自增 ID 作为 runtime key 的唯一身份。
- raw credential 单独加密存储，只通过 `CredentialRef` 解析。

## 智能调度方案

账号级智能调度的核心原则是：候选构建展开到账号，评分选择落到账号，relay 执行固定到账号凭证。

调度流程：

```text
客户端请求
  -> 解析 requested_model / group / endpoint_type / required_tools
  -> AccountCandidateIndex 取相关账号候选
  -> 过滤禁用、隔离、并发满、能力不匹配、分组不匹配、工具不支持
  -> 合并账号级物化评分、成本分、实时压力修正
  -> 选择 routing_score_total 最高候选
  -> DispatchPlan 写入 ResourceRef + AccountIdentity + CredentialRef
  -> RelayCredentialInjector 注入 plan credential
  -> relay 执行，不再 GetNextEnabledKey
  -> AttemptResult 记录事实，后台更新账号级评分
```

候选构建：

- 候选索引按 `requested_model + group + endpoint_type + required_tools` 组织。
- 索引项存储到账号维度，包含 `channel_id`、`resource_id`、`brand`、`account_id`、`credential_ref`、能力、成本参考、状态。
- 单次请求只取 Top-K 稳定候选和少量探索候选。
- Codex 同渠道多账号全部作为独立候选参与排序，但候选上限控制在配置内。
- 账号被禁用、凭证失效、硬隔离时要即时从可调度索引中剔除或降为不可选。

选择规则：

- 主排序使用现有智能调度的 `routing_score_total`。
- `routing_score_total` 由账号级物化评分、策略权重、实时压力修正、探索权重共同决定。
- 成本分来自后台物化结果；候选集合或运营成本配置变化后刷新，不在单次请求里重算全量账号成本。
- 并发满、队列满、账号硬不可用属于过滤条件，不作为随机换账号。
- retry 时优先选择同模型、同分组、同 endpoint 的其他账号候选；可以同渠道换账号，也可以跨渠道换账号。

执行绑定：

- `DispatchPlan` 是唯一执行凭证来源。
- relay 不允许在执行阶段再根据 channel 配置随机或轮询选 key。
- 如果某个 provider adapter 仍依赖 `ContextKeyChannelKey`，由 `RelayCredentialInjector` 写入 plan credential。
- 如果智能调度关闭，必须走显式 legacy fallback，并在调度详情标记 `legacy_channel_key_selection`。
- legacy fallback 只用于迁移期保底，不作为一期验收能力。

## 评分体系方案

一期不新增第二套账号评分模型，而是把账号作为更细粒度的 `RuntimeKey` 接入现有最新评分引擎。

账号级 runtime key：

```text
runtime_key =
  requested_model +
  upstream_model +
  channel_id +
  resource_id +
  brand +
  account_id +
  credential_subject_fingerprint +
  group +
  endpoint_type +
  capability_fingerprint
```

评分组成：

- 健康分：完成率、上游错误、认证失败、限流、超时、流中断等。
- 体验分：首包延迟、完整耗时、吞吐、空输出率、非空输出体验异常率等。
- 成本分：渠道/账号/模型的参考单位成本物化结果，不因单次请求波动。
- 能力分：模型匹配、endpoint 匹配、工具能力、stream 支持等。
- 策略分：分组优先级、探索权重、运营配置、实时压力修正。

更新链路：

- 用户请求完成后写入 attempt fact 和账号维度摘要。
- 后台评分任务消费 attempt fact，调用现有 `RuntimeHealthMonitor -> CandidateScoringService`。
- 探活请求和真实请求使用同一条评分链路，只是不更新真实访问字段。
- 认证失败、凭证格式错误、账号被上游明确拒绝等硬不可用状态，可以同步写入账号状态和可调度索引。
- 普通失败、慢响应、空输出、流中断走 EWMA 平滑更新，不直接把整个渠道打低。

账号与渠道的关系：

- 账号 A 的失败只更新账号 A 的 runtime snapshot、score event、隔离状态。
- 同渠道下账号 B 不因为账号 A 的单次失败扣分。
- 渠道级 base URL 错误、渠道配置错误、全局代理错误等可作为渠道级故障影响该渠道下相关账号。
- 新账号冷启动可读取同 channel、同模型、同分组的历史 channel 级 snapshot 作为初始参考。
- 一旦账号有真实样本或探活样本，账号级 snapshot 优先。

成本评分：

- 成本分使用后台物化，不在用户请求链路里现场重算。
- 成本分只表示平台/上游运营成本，不表示 C 端用户扣费成本。
- 成本分固定输入只包括渠道/账号/上游模型的运营成本资料，例如输入 token 成本、输出 token 成本、缓存成本、按次成本、账号级成本覆盖。
- 分组倍率、模型倍率、用户折扣、套餐价格、销售侧计费策略都不参与成本分。
- 当同批相关候选集合变化、运营成本配置变化、账号成本覆盖变化时，后台刷新相关候选成本分。
- 因此成本分可以随着“相关渠道/账号集合”变化而变化，但不是因为某一次用户请求成功或失败而变化。
- 调度详情需要展示“成本分”和“参考成本”，避免把评分值误认为真实扣费。

一期推荐字段：

```go
type ResourceRef struct {
    ResourceID         string
    ResourceType       string // platform_owned / supplier_owned / partner_owned
    ExecutionBindingID int    // current channel_id in phase 1
    Provider           string
    Brand              string
}

type AccountIdentity struct {
    AccountID                    string
    AccountType                  string // api_key / oauth_account / json_auth / token_key / composite
    Brand                        string
    Provider                     string
    CredentialIndex              int
    CredentialSubjectFingerprint string
    CredentialFingerprint        string
    AccountIdentityKey           string
    AccountUniqueKey             string
    DisplayName                  string
    Status                       string
}

type CredentialRef struct {
    ResourceID                    string
    AccountID                     string
    CredentialIndex               int
    CredentialSubjectFingerprint  string
    CredentialFingerprint         string
    Resolver                      string
}
```

字段稳定性：

- `resource_id` 一期可按 `platform:channel:{channel_id}` 生成，二期 C2C 使用独立资源 ID。
- `account_id` 应按 `provider + brand + credential_subject_fingerprint` 生成或映射，不使用可变 access token，也不绑定到某一个 resource。
- `account_identity_key` 表示账号身份唯一性，二期继续沿用，不升级为 resource 作用域。
- `account_unique_key` 一期可按 `channel_id + brand + credential_subject_fingerprint` 生成，用于现有渠道下的导入去重和管理列表唯一性；二期由 `ResourceAccountBinding` 替代这个渠道作用域绑定键。
- `credential_subject_fingerprint` 表示账号主体身份，刷新 credential 后应保持稳定。
- `credential_fingerprint` 必须使用不可逆 HMAC/SHA256，短展示只取前 6-8 位。
- `credential_fingerprint` 表示凭证版本，凭证刷新后允许变化，但不应导致账号变成新账号。
- `credential_index` 可以用于 UI 排序和现有 multi-key 兼容，但不能作为长期唯一身份。

核心接口调整：

- `RuntimeKey` 增加 `resource_id`、`account_id`、`account_type`、`credential_index`、`credential_fingerprint`。
- `DispatchPlan` 增加 `ResourceRef`、`AccountIdentity`、`CredentialRef` 或等价字段。
- `Candidate` 增加资源来源、执行绑定、账号身份、credential 引用。
- `CandidateExplanation`、观测 API、评分变更 API 增加账号短指纹和资源类型。
- 新增渠道账号列表接口，按 `channel_id` 返回账号/key 列表、状态、短指纹、评分摘要和最近探活/请求时间。
- `model_gateway_runtime_snapshots`、`model_gateway_score_events` 增加账号维度字段，历史空值兼容。

现有代码改造点：

- `pkg/modelgateway/core.RuntimeKey` 增加账号字段，并更新 normalize/hash/JSON 持久化逻辑。
- `pkg/modelgateway/core.DispatchPlan`、`Candidate`、`CandidateExplanation` 增加 `ResourceRef`、`AccountIdentity`、`CredentialRef`。
- `pkg/modelgateway/integration.ModelCandidatePoolBuilder` 从按 channel 生成候选，改为通过账号 registry 展开账号候选。
- `pkg/modelgateway/integration.ChannelSelectionWrapper` 去掉长期依赖 legacy key 选择的路径；迁移期 fallback 必须显式标记和告警。
- `middleware/distributor` 和 relay 上下文从 plan credential 注入 `ContextKeyChannelKey`，避免执行阶段再调用 `GetNextEnabledKey()`。
- `model_gateway_runtime_snapshots`、`model_gateway_score_events` 添加 `resource_id/account_id/account_type/credential_index/credential_fingerprint`，保留 `channel_id` 作为执行绑定。
- 观测接口和前端调度详情同步展示账号维度，并支持按账号查询评分事件。

账号抽象要求：

- `SupplierUser` 只负责登录、权限、结算归属，不参与 runtime 评分。
- `SupplyChannel` 是供应商名下的资源容器，可关联多个大模型品牌账号。
- `AccountIdentity` 是渠道下的大模型品牌账号，才是 runtime 评分、探活、隔离的对象。
- 一期先从现有渠道 key 生成 `AccountIdentity`，单 key 渠道也生成默认账号。
- 多 key 渠道的每一条启用 key 都是一个账号身份，而不是渠道内部随机 key。
- token-key 模式也归入账号体系，账号类型可标记为 `token_key`。
- OAuth、JSON auth、API key、token-key 的差异由 credential resolver 处理，调度评分层只看统一账号字段。
- `account_id` 必须稳定，不能因为 access token refresh、key 展示脱敏、顺序缓存重建而变化。
- `credential_fingerprint` 必须不可逆，且只用于识别、评分、审计和短展示。

前端页面要求：

- 渠道列表操作列新增“账号管理”按钮，建议使用图标按钮加 tooltip，避免操作列变宽。
- 路由建议为 `/console/channel/:id/accounts` 或 `/console/channel/accounts?channel_id=:id`，页面标题展示渠道名称和 ID。
- 页面主体是账号列表，不再用弹窗承载主流程。
- 初期可复用现有 `MultiKeyManageModal` 的状态查询、禁用、启用、删除能力，但交互升级为独立页面。
- 单 key 渠道也允许进入账号管理页，展示默认账号 `credential_index = 0`。
- 账号列表至少展示：账号序号、短指纹、状态、禁用原因、最近成功、最近失败、最近探活、当前评分、操作。
- 后续二期 C2C 接入后，同一页面可展示资源来源 `platform_owned/supplier_owned`，但一期只显示 `platform_owned`。

执行链路要求：

- 单 key 渠道生成一个默认 credential：`credential_index = 0`。
- 多 key 渠道按启用 key 展开多个 credential 候选。
- token-key 模式按可执行 token-key 展开账号候选，和 API key/OAuth 账号使用同一调度路径。
- raw key 只能在执行前解析，不进入日志、score event、API 响应。
- relay 使用 plan credential，跳过二次 `GetNextEnabledKey()`。
- shadow 模式只用于验证和对比，不作为长期运行路径。
- 原有随机/轮询字段只保留为历史配置兼容和迁移展示，不再作为新账号体系的正式调度逻辑。
- 如果智能调度关闭或无法生成 plan，应走明确的兼容兜底并记录告警；该兜底只用于迁移期保底，不作为新账号体系能力验收项。

评分链路要求：

- 账号级 runtime key 是第一优先评分对象。
- 历史 channel 级 snapshot 可作为账号冷启动 fallback。
- 运营成本、能力匹配等仍可继承渠道/模型/账号配置；分组优先级属于策略权重，不能混入成本分。
- 成本分继续使用后台物化，不因单次请求动态变化；当渠道成本、账号成本覆盖、候选集合变化时刷新相关账号候选。
- C 端用户计费规则，包括分组倍率、用户倍率、套餐价格、销售折扣，不得触发成本分刷新。
- 完成率、错误率、首包、耗时、吞吐、空输出、流中断、隔离状态必须按账号独立更新。
- 账号 A 的失败只更新账号 A 的 snapshot、score event、隔离状态。
- 渠道级配置错误、base URL 故障等仍可影响整个渠道。
- 探活和真实请求使用同一 `AttemptResult -> RuntimeHealthMonitor -> CandidateScoringService` 链路。
- 探活成功或失败只写目标账号 runtime key，不能回写成 channel 级样本。

评分兼容规则：

- 新账号没有样本时，先使用同 channel、同模型、同分组的历史 channel 级 snapshot 作为冷启动参考。
- 一旦账号产生真实样本或探活样本，账号级 snapshot 优先。
- 历史 channel 级 score event 保持可查，不强制迁移。
- 账号级 score event 必须能解释本次变化来自 completion、upstream_error、ttft、duration、throughput、empty_output、stream_interrupted、concurrency、queue、first_byte_backlog、cost、group_priority 中的哪些项。

性能要求：

- 不为冷账号预建大量 DB 行。
- 只持久化有真实样本、探活样本、隔离状态或被调度过的账号 snapshot。
- 候选索引按 `requested_model + group + endpoint_type + required_tools` 组织。
- 单次请求默认 Top-K 64 + 探索 4-8。
- 调度详情展示上限保留 32，避免接口体积膨胀。

兼容与迁移：

- 新字段需要兼容 SQLite、MySQL、PostgreSQL。
- 历史 runtime key hash 不强制重算。
- 旧 score event 保持可查询，只是没有账号维度。
- 现有 `channel_id` 继续保留为执行绑定，不作为未来资源主键。
- 现有 multi-key 的 `multi_key_mode` 只作为历史展示字段，不再参与新智能调度决策。
- 现有 key 禁用/启用状态迁移为账号状态；删除 key 等价于删除或归档对应账号身份。
- 若账号 registry 无法解析某类凭证，必须退化成单账号候选并记录 `unsupported_account_expansion`，不能静默跳过整个渠道。
- 所有 JSON 编解码继续使用 `common.*` 包装函数。

技术验收：

- 单 key、多 key、shadow、active、智能调度兜底都有回归测试。
- 账号级 snapshot/event 能按 `credential_fingerprint` 查询。
- relay 不会覆盖 plan credential。
- 探活结果只影响目标账号级 runtime key。
- 10 万账号候选构建压测不出现全量扫描。

## 一期关键风险与决策

风险：

- 如果只扩展展示、不固定 relay credential，智能调度账号级评分会失真。
- 如果每次请求现场展开所有渠道账号，大账号池会出现性能问题。
- 如果账号维度和 channel 维度混写，评分变更记录会难以解释。
- 如果 raw key 进入日志或 score event，会产生安全风险。

决策：

- 一期必须先改执行链路，确保 selected credential 不被二次随机覆盖。
- 一期去掉渠道内部随机/轮询作为正式运行逻辑，账号选择统一基于新的智能调度。
- 一期候选索引先服务 `platform_owned`，二期 C2C 直接复用。
- 一期只做账号短指纹展示，不展示 raw key。
- 一期不引入供应商业务字段到调度引擎，供应商归属留到二期资源 provider 注入。
