# Console Split Migration Plan

## 目标

将当前混合在 `/console/*` 下的普通用户控制台和管理员能力，拆分为两套独立应用壳：

```text
普通用户控制台: /console/*
管理员后台: /admin/*
共享控制台组件: src/shared/console-ui/*
```

迁移后的产品结构需要满足：

- 普通用户控制台只展示接入、调用、日志、费用等用户任务。
- 管理员后台只展示运营、渠道、模型、商业、用户和系统治理任务。
- 个人设置、安全设置、语言设置、退出登录等入口放入右上角用户菜单。
- 两套应用共享统一商业化设计语言，但信息密度和页面目标不同。
- 文件夹层面提前隔离，为后续独立部署保留空间。

## 当前状态

当前路由集中在 `src/App.jsx`，大量页面挂在 `/console/*` 下：

- 普通用户页面：`/console/token`, `/console/playground`, `/console/log`, `/console/recharge` 等。
- 管理员页面：`/console/channel`, `/console/model-gateway`, `/console/profit-monitor`, `/console/setting` 等。
- 权限守卫已有 `PrivateRoute` 和 `AdminRoute`。
- 当前侧栏由 `components/layout/SiderBar.jsx` 统一渲染，普通用户和管理员菜单混在一个侧栏中。
- 当前顶部栏由 `components/layout/headerbar/*` 统一渲染，控制台和门户混合判断依赖 `/console` 前缀。

## 目标目录

```text
src/apps/
  user-console/
    DESIGN_STYLE.md
    layout/
    navigation/
    pages/
    routes/

  admin-console/
    DESIGN_STYLE.md
    layout/
    navigation/
    pages/
    routes/

src/shared/
  console-ui/
    console-ui.css
    components/
    hooks/
```

## 任务分配

### 产品

- 确认普通用户控制台的信息架构。
- 确认管理员后台的信息架构。
- 确认旧路由到新路由的兼容策略。
- 明确哪些管理能力必须只在 `/admin/*` 出现。
- 确认右上角用户菜单的最终入口。

### 运营

- 确认管理员首页需要展示的核心指标。
- 确认渠道预警、结算异常、智能切换等运营数据优先级。
- 确认渠道管理、账号池、余额监控、健康检测的首批迁移顺序。
- 确认普通用户费用中心的充值、订阅、邀请入口顺序。

### UI

- 基于 `Aurora Enterprise Console` 统一两套应用视觉。
- 维护普通用户控制台设计说明：`user-console/DESIGN_STYLE.md`。
- 维护管理员后台设计说明：`admin-console/DESIGN_STYLE.md`。
- 输出共享组件规范：按钮、状态标签、指标卡、筛选栏、表格、导航。
- 检查普通端和管理端是否像同一套商业 SaaS 产品家族。

### 前端

- 建立 `user-console` 和 `admin-console` 应用骨架。
- 建立 `shared/console-ui` 共享视觉 token 和基础组件。
- 将管理员路由逐步从 `/console/*` 迁移到 `/admin/*`。
- 保留旧路由兼容跳转，降低发布风险。
- 补齐新增文案的 i18n。
- 每个阶段运行构建和 i18n 同步验证。

### 后端

- 已进入权限边界收口阶段，后台写接口需要逐步挂载稳定权限点。
- 当前先使用 `Role 10 / 100` 兼容历史后台权限；已补数据库权限来源，后续逐步迁移到角色配置工作台。
- 提供当前管理员权限接口与权限目录只读接口，前端不得继续只依赖本地角色猜测。
- 后台写接口需要记录管理审计日志，至少包含操作者、权限点、结果、目标、耗时和安全摘要。
- 后续如独立部署后台，需要确认 `/admin/*` 前端路由与后端 API 权限策略无冲突。
- 如管理员首页需要新增聚合指标，再设计后端 dashboard API。

## 阶段计划

### 阶段 1：骨架隔离

目标：建立可持续迁移的目录、导航配置和共享设计 token。

交付：

- `src/apps/user-console/navigation/userConsoleNav.config.js`
- `src/apps/admin-console/navigation/adminConsoleNav.config.js`
- `src/apps/user-console/layout/UserConsoleLayout.jsx`
- `src/apps/admin-console/layout/AdminConsoleLayout.jsx`
- `src/shared/console-ui/console-ui.css`
- `src/shared/console-ui/components/*`

验收：

- 普通端和管理端导航配置分开维护。
- 共享视觉变量可以被两套应用复用。
- 不破坏现有 `/console/*` 页面。

### 阶段 2：最小路由闭环

目标：让 `/admin/*` 拥有独立入口，并保留旧路径。

交付：

```text
/admin/channels                -> 渠道管理
/admin/channel-accounts        -> 账号池管理
/admin/channel-balance-monitor -> 渠道余额监控
/admin/model-gateway           -> 智能模型网关
/admin/profit-monitor          -> 盈利监控台
/admin/settings                -> 系统设置
```

旧路由暂时保留：

```text
/console/channel
/console/channel/accounts
/console/channel-balance-monitor
/console/model-gateway
/console/profit-monitor
/console/setting
```

验收：

- `/admin/*` 必须经过 `AdminRoute`。
- 普通用户不能访问 `/admin/*`。
- 管理员可从右上角用户菜单进入管理员后台。

### 阶段 3：普通端瘦身

目标：普通用户侧栏不再出现管理员区域，个人设置进入右上角菜单。

普通端导航：

```text
概览
- 数据看板
- 服务状态

开发接入
- 操练场
- 令牌管理
- 使用日志
- 绘图日志
- 任务日志
- 聊天

费用中心
- 账户充值
- 套餐订阅
- 邀请有奖
```

验收：

- 普通用户侧栏不显示 `管理后台`、`渠道管理`、`模型部署` 等后台项。
- `个人设置` 不出现在侧栏。
- 个人相关入口从右上角用户菜单进入。

### 阶段 4：管理员后台迁移

目标：管理员后台使用独立信息架构。

管理员导航：

```text
运营首页
渠道运营
模型与路由
商业运营
用户运营
系统治理
```

首批迁移优先级：

1. 渠道管理
2. 账号池管理
3. 渠道余额监控
4. 渠道健康检测
5. 智能模型网关
6. 盈利监控台
7. 系统设置

验收：

- 管理员后台不显示普通用户充值、订阅、邀请、令牌等菜单。
- 管理员后台首屏优先呈现运营和风险信息。
- 表格、状态标签、筛选器与普通端共享视觉语言。

当前进度：

- `经营总览` 已接入网关观测、健康检测、余额监控和盈利监控摘要。
- `实时监控` 已通过 `AdminRealtimeMonitor` 进入后台专属页面承载，复用 runtime 快照接口展示运行键、并发、排队、熔断和高压渠道。
- `渠道预警` 已通过 `AdminChannelAlerts` 进入后台专属页面承载，复用健康检测队列和余额监控接口汇总渠道探活、隔离、低余额和耗尽账号预警。
- `渠道管理` 已通过 `AdminChannels` 进入后台专属页面承载，并补充运营指标、风险摘要和信息分区。
- `账号池管理` 已通过 `AdminChannelAccounts` 进入后台专属页面承载，复用原有导入、启停、归档、代理、权限检测和调度诊断能力，并增加管理员后台视觉变体。
- `渠道余额监控` 已通过 `AdminChannelBalanceMonitor` 进入后台专属页面承载，复用原有余额监控接口与操作能力，并增加管理员后台视觉变体。
- `渠道健康检测` 已通过 `AdminChannelHealthCheck` 进入后台专属页面承载，复用现有探活队列、检测历史、立即探活和探活配置能力，并增加管理员后台视觉变体。
- `智能模型网关` 已通过 `AdminModelGateway` 进入后台专属页面承载，复用用户请求观测、运营视图、工程诊断、智能切换、评分历史和 Replay 导出能力，并增加管理员后台视觉变体。
- `盈利监控台` 已通过 `AdminProfitMonitor` 进入后台专属页面承载，复用经营利润、分组倍率、资源成本、建议快照和灰度任务能力，并增加管理员后台视觉变体。
- `模型管理` 已通过 `AdminModels` 进入后台专属页面承载，复用模型广场展示、供应商、同步和覆盖能力，并接入后台表格容器样式。
- `模型部署` 已通过 `AdminModelDeployments` 进入后台专属页面承载，复用部署访问守卫、部署表格、日志、续期和配置更新能力，并接入后台表格容器样式。
- `路由策略` 已通过 `AdminRoutePolicy` 进入后台专属页面承载，复用智能模型网关调度设置组件，集中维护候选、兜底、资源保护和动态策略配置。
- `倍率配置` 已通过 `AdminRatioConfig` 进入后台专属页面承载，复用模型定价、分组倍率、上游价格同步和工具调用定价配置能力。
- `用户管理` 已通过 `AdminUsers` 进入后台专属页面承载，复用用户筛选、编辑、升降级、绑定管理、订阅查看和安全重置能力，并接入后台表格容器样式。
- `订阅管理` 已通过 `AdminSubscriptions` 进入后台专属页面承载，复用套餐创建、编辑、价格与第三方商品 ID 管理能力，并接入后台表格容器样式。
- `兑换码管理` 已通过 `AdminRedemptions` 进入后台专属页面承载，复用兑换码生成、复制、筛选和批量删除能力，并接入后台表格容器样式。
- `系统设置` 已通过 `AdminSettings` 进入后台专属页面承载，保留 root-only 管理配置页签，并接入后台设置容器样式。
- `代理管理` 已通过 `AdminChannelProxies` 进入后台专属页面承载，复用代理配置、代理状态和代理维护能力，并增加管理员后台视觉变体。
- `审计日志` 已通过 `AdminAuditLogs` 进入后台专属页面承载，默认聚焦管理日志，便于系统治理追踪后台操作。
- `用户分层` 已通过 `AdminUserSegments` 进入后台专属页面承载，基于用户列表接口抽样展示角色、状态、分组、额度和最近登录分布。
- `权限角色` 已通过 `AdminRoles` 进入后台专属页面承载，并改为读取 `adminPermissions.config.js` 生成菜单权限矩阵，减少权限文档、菜单和页面之间的漂移。
- `结算记录` 已通过 `AdminSettlements` 进入后台专属页面承载，复用全平台充值订单接口，展示支付金额、入账额度、订单状态和 pending 补单入口。
- `消费明细` 已通过 `AdminConsumption` 进入后台专属页面承载，默认聚焦消费日志，便于商业运营追踪扣费、模型计费和渠道费用。
- `风控记录` 已通过 `AdminRiskRecords` 进入后台专属页面承载，默认聚焦错误日志，辅助定位失败、拦截、异常响应和高风险请求。
- `邀请返佣` 已通过 `AdminInviteRebates` 进入后台专属页面承载，基于用户列表接口抽样展示邀请人、邀请人数、历史奖励和待划转额度。
- `后台任务` 已通过 `AdminBackgroundTasks` 进入后台专属页面承载，复用异步任务日志接口，展示任务状态、渠道归属、进度和失败原因。
- 管理员后台导航中的占位入口已完成首轮真实页面承载，下一批进入页面体验收敛、权限边界复核和旧入口访问量观察。

### 阶段 5：旧入口收敛

目标：旧 `/console` 管理员入口逐步跳转或下线。

策略：

- 第一轮：新旧入口并行。
- 第二轮：旧管理入口增加跳转提示。
- 第三轮：旧管理入口跳转到 `/admin/*`。
- 第四轮：删除旧侧栏管理员区域。

验收：

- 已发布版本用户不会因为旧链接失效而中断。
- 文档和导航均指向新后台。
- 埋点或日志确认旧入口访问量低于可下线阈值。

## 验证清单

每个阶段至少执行：

```bash
cd web/classic
bun run i18n:sync
bun run verify:console-migration
bun run verify:admin-permissions
bun run build
bun run build:admin
```

人工验证：

- 普通用户账号看不到管理员入口。
- 管理员账号能进入 `/admin/*`。
- 非管理员访问 `/admin/*` 显示无权限或跳转。
- 顶部栏不出现营销导航。
- 普通端和管理端视觉统一。
- 移动端侧栏抽屉不遮挡内容。

## 风险

- 一次性重写 `PageLayout` 风险高，应先旁路接入新后台。
- 当前 `SiderBar.jsx` 同时承担配置过滤、聊天动态菜单和权限过滤，直接改动容易回归。
- 新增文案较多，需要严格补齐 i18n。
- 管理员后台如果后续独立部署，需要单独处理构建入口和部署路径。

## 当前推荐执行顺序

1. 应用目录、共享设计 token、普通端和管理端导航配置已经完成。
2. `/admin/*` 最小路由闭环、旧后台路径兼容跳转和右上角管理员入口已经完成；旧入口映射已抽成 `adminLegacyRedirects.config.js`，并由 `verify:console-migration` 校验。
3. 管理员后台 26 个导航入口已经完成首轮真实页面承载，当前不再使用后台占位路由。
4. RBAC 第一阶段已经完成：菜单权限点集中配置、`AdminRoute` 支持可选 `permission`、后台侧栏可按权限点过滤、权限角色页从配置生成矩阵。
5. 权限来源接口已进入闭环：后端提供权限目录、当前管理员权限、角色配置和用户覆盖接口，前端登录态和运行时会读取 `admin_permissions`。
6. 真实权限存储首版已完成：角色、角色权限、用户角色绑定、用户权限覆盖四类表已接入迁移，root-only 修改接口已挂权限守卫。
7. 同步推进页面体验收敛：统一嵌入式设置页、表格密度、状态标签、筛选区和详情抽屉。
8. 页面级迁移稳定后，再拆分侧栏配置字段和独立部署配置，避免过早改后端配置模型。
9. 最后收敛旧 `/console/*` 后台入口，从兼容跳转逐步过渡到下线。

## 后续迁移路线

### 路线 A：管理员后台业务页真实迁移

目标：让管理员进入 `/admin/*` 后看到完整的后台工作流，而不是普通控制台页面套壳。

优先级：

1. 运营首页：补齐实时风险、渠道异常、智能切换、结算异常、利润趋势。
2. 渠道运营：重构渠道管理、账号池管理、余额监控、健康检测、代理管理。
3. 模型与路由：重构智能模型网关、模型管理、模型部署、路由策略、倍率配置。
4. 商业运营：重构盈利监控、订阅管理、兑换码管理、结算记录、消费明细。
5. 用户运营：重构用户管理、用户分层、风控记录、邀请返佣。
6. 系统治理：重构系统设置、权限角色、审计日志、后台任务。

执行规则：

- 新增或迁移后台页面时，路由只维护在 `admin-console/routes/adminRoutes.jsx`。
- 导航只维护在 `admin-console/navigation/adminConsoleNav.config.js`。
- 旧 `/console/*` 后台路径只做兼容跳转，不再承载新的后台页面实现。
- 管理员后台页面不出现充值、订阅、邀请、令牌等普通用户任务。

### 路线 B：普通用户控制台收敛

目标：普通用户只看到接入、调用、日志、费用中心，不再看到后台管理心智。

优先级：

1. 完成剩余普通端页面的新壳适配。
2. 将个人资料、安全设置、通知偏好、语言设置都沉入右上角用户菜单。
3. 检查普通侧栏配置，只保留概览、开发接入、费用中心。
4. 普通端页面使用更低信息密度，减少后台运营类指标露出。

执行规则：

- 普通端路由集中维护在 `user-console/routes`。
- 普通端导航集中维护在 `user-console/navigation/userConsoleNav.config.js`。
- 普通端页面可以复用 `shared/console-ui`，但不复用后台专用指标卡和风险表格。

### 路线 C：配置与部署拆分

目标：从文件夹、构建、配置三个层面为独立部署管理员后台做准备。

优先级：

1. 保持当前 `SidebarModulesAdmin` 兼容，不立刻改后端字段。
2. 待页面迁移稳定后，评估新增 `UserConsoleModules` 与 `AdminConsoleModules`。
3. 使用 `admin.html`、`admin-console/entry.jsx`、`build:admin` 进行独立后台构建。
4. 在部署环境验证 `/admin/*` history fallback、静态资源路径和登录后落点。
5. 再决定是否拆成独立域名或独立部署流水线。

执行规则：

- 默认合并入口和独立后台入口共用 `adminRoutes.jsx`。
- 共享样式放在 `shared/console-ui`，避免普通端和管理端复制两套视觉 token。
- 独立部署前只做前端入口拆分，不提前改动后端 API 权限模型。

## 执行状态

### 已完成

- 已建立 `user-console` 与 `admin-console` 应用目录。
- 已沉淀两套设计风格文档。
- 已建立 `shared/console-ui` 共享视觉 token 与 `ConsoleShell`。
- 已建立普通端和管理端导航配置。
- 已接入 `/admin/*` 管理员后台路由。
- 已将右上角用户菜单接入 `进入管理员后台`。
- 已将普通 `/console` 侧栏瘦身为 `概览 / 开发接入 / 费用中心`。
- 已从普通 `/console` 侧栏移除 `个人设置` 和管理员区域。
- 已将旧 `/console` 管理员入口重定向到 `/admin/*`。
- 已将管理页面内部跳转收口到 `/admin/*`，包括渠道账号、模型部署和系统设置入口。
- 已建立 `/admin/overview` 经营总览首版，并将 `/admin` 与右上角管理员入口指向经营总览。
- 已将 `/admin/overview` 从静态占位升级为数据驱动总览，接入网关观测、健康检测队列、渠道余额监控和盈利监控四类现有摘要 API。
- 已为经营总览增加数据源健康区域，单个接口失败时按模块降级展示，不阻塞整个后台首页。
- 已新增 `admin-console/pages/AdminChannels.jsx`，`/admin/channels` 已切换为管理员后台专用渠道页入口。
- 已为渠道管理页增加运营视角头部、当前筛选指标、风险摘要和表格信息分区说明，保留原渠道表格的批量操作、测试、余额刷新和列设置能力。
- 已补齐经营总览新增文案的 7 个前端语言包翻译。
- 已将普通控制台核心路径接入 `UserConsoleLayout`，外层旧 Header/Sider/Footer 在这些路径上自动退让。
- 已将新控制台壳的右上角用户信息改为可操作菜单，承接个人设置、令牌、充值、管理员后台和退出登录入口。
- 已将聊天 iframe 页面接入 `UserConsoleLayout`，并从 `localStorage.chats` 动态生成普通端聊天导航入口。
- 已将普通端新导航接入用户侧栏配置过滤，个人配置页不再展示后台模块。
- 已将管理端新导航接入全局侧栏配置过滤，已有配置项的后台模块会按 `SidebarModulesAdmin` 控制显示。
- 已将全局侧栏配置页按 `普通用户控制台 / 管理员后台` 两套信息架构重排，展示层不再暴露旧的 `个人设置` 侧栏模块。
- 已将管理员后台新信息架构中的占位入口纳入 `SidebarModulesAdmin`，包括运营首页、路由策略、倍率配置、结算记录、消费明细、用户分层、风控记录、邀请返佣、权限角色、审计日志和后台任务。
- 已将结算记录、消费明细、风控记录、邀请返佣和后台任务从占位入口升级为真实后台页面，继续压缩管理端空页面比例。
- 已补齐当前前端扫描到的缺失 i18n key，覆盖 `zh-CN / zh-TW / en / fr / ru / ja / vi` 七个语言包。
- 已新增管理员后台独立入口基础设施：`admin.html`、`admin-console/entry.jsx`、`AdminStandaloneApp.jsx`、`build:admin` 和 `dev:admin`。
- 已抽取 `admin-console/routes/adminRoutes.jsx`，默认合并入口和管理员独立入口共用同一套后台路由，避免后续页面迁移时双份维护。
- 已新增 `admin-console/permissions/adminPermissions.config.js`，集中维护 26 个后台菜单权限点、目标角色模板和兼容角色策略。
- 已在 `adminPermissions.config.js` 增加 10 个危险操作权限点，单独覆盖补单、倍率、路由策略、系统设置、权限角色、用户角色和删除清理等高风险动作。
- 已让 `AdminRoute` 支持可选 `permission`，在后端尚未返回权限点时继续按 `Role 10` 兼容历史管理员。
- 已让管理员后台侧栏支持基于用户权限点动态过滤菜单。
- 已让 `AdminRoles` 从权限配置生成菜单权限矩阵，权限清单不再由页面硬编码维护。
- 已新增 `AdminPermissionButton`，支持 `requiredPermission`、`dangerPermission` 和 `fallbackTooltip`，并将 `结算记录` 和普通充值历史弹窗的 `补单` 按钮接入 `admin:commercial:settlement:complete`。
- 已将 `渠道管理` 的单行删除、删除所选通道、删除禁用通道接入 `admin:channel:channel:danger`；无权限时不触发删除请求，并展示清晰原因。
- 已将 `账号池管理` 的批量启停、导入账号、归档/恢复、代理绑定、凭证替换、删除账号和删除归档记录接入 `admin:channel:account:danger`；工具栏、行操作和保存函数均会阻断无权限操作。
- 已将 `用户管理` 的启用/禁用、升降级、重置 Passkey、重置 2FA 和注销用户接入 `admin:user:user:danger`；表格按钮、下拉危险动作和确认弹窗均会阻断无权限操作。
- 已将 `倍率配置` 的模型定价保存、未设置价格模型保存、旧 JSON 倍率保存/重置、分组倍率保存、上游价格同步和工具价格保存接入 `admin:model:ratio:update`；主按钮、确认弹窗和保存函数均会阻断无权限写入。
- 已将 `路由策略` 的保存智能调度配置和恢复默认配置接入 `admin:model:route_policy:danger`；保存按钮、恢复默认按钮和确认回调均会阻断无权限写入。
- 已将 `性能设置` 的保存性能配置接入 `admin:system:settings:update`，清理缓存、重置统计、触发 GC 和清理日志接入 `admin:system:performance:danger`；按钮、确认弹窗和执行函数均会阻断无权限写入或执行。
- 已将 `系统设置` 的服务器地址、Worker、SSRF、登录注册、Passkey、邮箱白名单、SMTP、OIDC、GitHub/Discord/Linux DO/WeChat/Telegram OAuth、Turnstile 和自定义 OAuth 新建/编辑/删除接入 `admin:system:settings:update`；保存按钮、开关即时保存、删除确认和提交函数均会阻断无权限写入。
- 已将 `速率限制设置` 的模型请求速率限制保存接入 `admin:system:settings:update`；保存按钮和提交函数均会阻断无权限写入。
- 已将 `模型相关设置` 的全局模型、Claude、Gemini、Grok 配置保存接入 `admin:system:settings:update`，并将渠道亲和规则保存和亲和缓存清理接入 `admin:model:route_policy:danger`。
- 已将 `模型部署设置` 的 io.net 部署启用和 API Key 保存接入 `admin:system:settings:update`；连接测试保留为非持久执行辅助动作。
- 已将 `支付设置` 的通用支付、易支付、Stripe、Creem、Waffo 和 Waffo Pancake 保存接入 `admin:system:settings:update`；保存按钮和提交函数均会阻断无权限写入。
- 已复核 `后台任务` 前端页面，当前只包含任务查询、筛选、预览和列配置，暂无执行类写操作按钮。
- 已新增后端 `RequireAdminPermission(permission)` 与 `RequireRootAdminPermission(permission)`，首版保持 `Role 10 / 100` 兼容，同时把权限点写入请求上下文。
- 已新增后端权限目录与当前管理员权限接口：`/api/admin/permissions/config` 和 `/api/admin/permissions/self`。
- 已新增后端权限存储模型：`admin_roles`、`admin_role_permissions`、`admin_user_role_bindings`、`admin_user_permission_overrides`。
- 已新增 root-only 权限分配接口，覆盖角色创建/更新/禁用与用户角色/权限覆盖保存。
- 已让登录响应和 `/api/user/self` 为管理员返回 `admin_permissions`、`admin_permission_mode` 和 `admin_permission_source`。
- 已让 `RequireAdminPermission` 优先使用数据库权限来源；用户没有存储权限配置时继续按 `Role 10 / 100` 兼容。
- 已让前端运行时对旧登录态进行后台权限补水，管理员进入合并入口或独立后台入口时会自动合并当前权限。
- 已将 root-only 菜单与危险操作的兼容角色从普通管理员收口到超级管理员，包括倍率配置、系统设置、权限角色、审计日志、后台任务、倍率写入、系统写入和性能清理。
- 已将首批高风险后端接口接入权限守卫，覆盖人工补单、用户危险操作、渠道删除、账号池危险操作、渠道健康执行、模型网关调度/熔断/探活、系统设置、自定义 OAuth、倍率同步、性能清理和日志删除。
- 已将第二批后台写接口接入权限守卫，覆盖订阅、兑换码、供应商、模型元数据、模型部署、盈利监控、代理维护、动态计费确认、预填分组、渠道维护和上游模型更新。
- 已将后台审计日志接入权限中间件，已挂权限点的后台写接口会记录 `LogTypeManage`，并写入权限点、执行结果、操作者、角色、请求 ID、目标路径参数、查询标识、JSON body 字段名、白名单标识字段和耗时。
- 已修正后台权限源识别，避免 `/api/user/self` 的侧栏 `permissions` 对象被误判为精细后台权限列表。
- 已将权限角色工作台的角色保存、模板同步、角色禁用和用户权限绑定保存接入当前登录态权限强制刷新，避免管理员调整自己权限后前端状态滞后。
- 已为审计日志增加权限点、权限来源、操作结果、审计操作人和目标用户 ID 筛选，并在管理日志展开区展示接口路由、耗时、目标对象和审计摘要。

### 下一步

- 将管理员页面内容按 `Aurora Enterprise Console` 组件体系继续收敛，下一步处理页面细节一致性、专项角色回归、业务级审计摘要，以及旧入口访问量观察。
- 将权限角色工作台从首版可配置推进到可验收状态，重点补专项角色测试、被撤权管理员落点和窄屏体验。
- 完成普通控制台剩余页面适配，重点检查旧布局是否仍在新壳路径上出现。
- 保持当前侧栏配置字段兼容，先从展示层拆分普通端配置和管理端配置。
- 使用 `build:admin` 和 `dev:admin` 持续验证管理员后台独立入口。
- 进入部署前，只在本地和测试环境验证；不发布到 pro，除非明确要求。

## 接下来改造与迁移方案

### 第 1 阶段：权限边界收口

目标：先把后台危险写操作从 UI、函数和后端接口三个层面收口，避免管理员后台独立后权限语义漂移。

任务：

- 前端继续复核新增后台任务执行类按钮；当前任务日志页暂无写操作可接入。
- 后端已补 `RequireAdminPermission(permission)`，首批保护补单、倍率、路由策略、系统设置、用户角色、渠道删除、账号池危险操作、健康执行和删除清理接口。
- 后端第二批接口守卫已补订阅、兑换码、供应商、模型元数据、部署、利润监控、代理维护、动态计费确认和上游模型更新写操作。
- 后端已补权限来源只读接口和数据库权限来源；当前无存储配置时按 `Role 10 / 100` 生成 `admin_permissions`，有用户绑定/覆盖时使用数据库来源。
- 审计日志已记录操作者、权限点、请求 ID、目标路径参数、查询标识、JSON body 字段名、白名单标识字段、执行结果和耗时。
- 权限角色 UI 工作台已支持角色模板、角色编辑、权限分组、用户角色绑定和允许/拒绝覆盖；权限变更保存后会刷新当前登录态权限。
- 审计日志已增加权限点、权限来源、操作结果、操作人和目标用户 ID 筛选，并在展开区展示目标对象和审计摘要。
- 下一步继续补专项角色回归和业务级变更摘要，优先扩展其他人工决策接口。
- 测试继续补齐普通用户、只读管理员、专项管理员和超级管理员四类账号路径。

验收：

- 无权限按钮不发请求。
- 直调接口也会被后端拒绝。
- 危险操作都有确认、提示和审计记录。

### 第 2 阶段：管理员后台体验统一

目标：把已经迁入 `/admin/*` 的页面从“旧页面套壳”推进到统一后台产品体验。

任务：

- 统一表格密度、筛选栏、状态标签、风险摘要、详情抽屉和批量操作区。
- 渠道运营优先收敛渠道管理、账号池、余额监控、健康检测和代理管理。
- 模型与路由优先收敛智能模型网关、路由策略、倍率配置和模型部署。
- 商业运营优先收敛盈利监控、结算记录、消费明细、订阅和兑换码。

验收：

- 管理员后台不出现普通用户充值、令牌、邀请等心智。
- 同一类操作在不同页面有一致按钮位置、状态展示和错误反馈。
- 首页到详情、列表到操作的路径清晰，不依赖旧 `/console/*` 页面入口。

### 第 3 阶段：普通控制台最终瘦身

目标：普通用户控制台只保留接入、调用、日志和费用中心。

任务：

- 检查 `/console/*` 剩余页面是否仍被旧 `PageLayout` 包裹。
- 个人资料、安全设置、通知偏好、语言和退出登录统一沉入右上角用户菜单。
- 普通端侧栏只保留概览、开发接入、费用中心和动态聊天入口。
- 普通端页面降低运营密度，避免展示后台风险指标。

验收：

- 普通用户看不到 `管理后台`、渠道运营、模型与路由、系统治理等入口。
- 普通用户流程从首页到令牌、操练场、日志、充值/套餐完整闭环。

### 第 4 阶段：独立构建与部署演练

目标：在不发布 pro 的前提下，把管理员后台独立部署路径验证清楚。

任务：

- 使用 `dev:admin` 验证管理员独立入口。
- 使用 `build:admin` 生成独立后台产物。
- 验证 `/admin/*` history fallback、静态资源路径、登录后落点和权限守卫。
- 评估是否需要独立域名、独立 CDN 缓存和单独发布流水线。

验收：

- 合并入口和独立入口共用同一套路由配置。
- 独立后台构建不引入普通端导航和营销头部。
- 测试环境验证完成后，再决定是否进入 pro 发布流程。

## 近期执行拆解

### Sprint 1：后台路径闭环

状态：前端已完成，待产品和运营确认旧入口提示策略。

责任分配：

- 产品：确认旧管理入口跳转策略，确认是否需要在旧入口展示迁移提示。
- 运营：提供首批需要在后台首页展示的异常指标优先级。
- UI：确认后台导航和表格区域是否沿用当前 `Aurora Enterprise Console` 视觉。
- 前端：完成 `/admin/*` 真实页面接入、旧链接替换和构建验证。
- 后端：确认现有管理 API 权限不依赖 `/console/*` 前端路径。

交付：

- `/admin/channels`、`/admin/channel-accounts`、`/admin/channel-balance-monitor`、`/admin/model-gateway`、`/admin/settings` 可从后台内互相跳转。
- 旧 `/console/*` 管理入口保留兼容跳转。
- 普通端不出现管理员侧栏项。

### Sprint 2：管理员经营总览

状态：前端聚合首版已完成，待真实管理员登录态视觉验收和后端聚合 API 评估。

责任分配：

- 产品：定义经营总览的信息层级，区分实时风险、渠道运营、商业结果。
- 运营：给出渠道预警、结算异常、智能切换异常、余额预警的展示优先级。
- UI：输出经营总览卡片、风险列表、趋势图、快捷操作的页面规范。
- 前端：实现 `/admin/overview`，优先复用 `shared/console-ui`，并保持模块级降级能力。
- 后端：如现有接口性能不足或查询太分散，再补充后台聚合指标 API。

交付：

- 管理员进入后台默认能看到运营风险和商业指标的聚合摘要。
- 页面不展示普通用户充值、令牌、邀请等入口。
- 指标空态、加载态、异常态完整。
- 总览优先复用现有 admin API；后续如需要降低首页请求数，再替换为单个后台聚合 API。

当前验证：

- `bun run i18n:sync` 通过。
- 静态 i18n key 扫描通过，当前扫描到的 `t()` 文案均存在于七个前端语言包。
- `bun run build` 通过。
- 无登录态访问 `/admin/overview` 会进入登录页，管理员路由守卫仍生效。
- 受本地 browser 安全策略限制，暂未在注入登录态场景下截图验收；需要使用真实管理员登录态继续视觉检查。

### Sprint 3：普通控制台视觉迁移

状态：进行中。

责任分配：

- 产品：确认普通用户控制台仅保留接入、调用、日志、费用任务。
- 运营：确认充值、套餐、邀请在费用中心的顺序和默认曝光。
- UI：确认普通端和管理端同属一套商业 SaaS 设计语言，但密度不同。
- 前端：将 `/console/*` 接入 `UserConsoleLayout`，并逐步迁移核心页面。

交付：

- `/console/token`、`/console/playground`、`/console/log` 使用新普通端布局。
- `/console`、`/console/channel-status`、`/console/midjourney`、`/console/task`、`/console/recharge`、`/console/subscription-plans`、`/console/affiliate` 使用新普通端布局。
- 个人设置、安全设置、语言和退出登录都在右上角用户菜单中完成。
- 普通用户无法通过导航进入后台页面。

当前验证：

- 普通控制台新壳路径集中在 `user-console/routes/userConsoleRoutes.config.js`。
- `PageLayout` 在新壳路径上关闭旧 Header/Sider/Footer，避免双导航。
- 新控制台壳右上角用户菜单已可进入个人设置，并保留令牌、充值、管理员后台和退出入口。
- 聊天页已接入新壳，iframe 高度由新控制台内容区控制，聊天入口由本地聊天配置动态生成。
- 普通端导航项通过旧配置 key 兼容过滤：`chat / console / personal`。
- 个人侧栏配置只保留普通用户模块；后台模块、智能模型网关和个人设置入口不再出现在个人侧栏配置里。

### Sprint 3.5：侧栏模块配置拆分

状态：前端首轮已完成，后台占位模块开关已补齐，待运营确认各占位模块的实际页面迁移优先级。

责任分配：

- 产品：确认普通用户可自定义的导航范围只包含概览、开发接入、费用中心。
- 运营：确认管理后台全局模块开关是否需要覆盖新占位模块，如结算记录、风控记录、审计日志。
- UI：确认配置页是否需要按新信息架构重排为普通端和管理端两类。
- 前端：将新普通端、管理端导航接入配置过滤，并保持旧配置字段兼容。
- 后端：后续如需要更彻底拆分，可新增独立配置字段，例如 `UserConsoleModules` 与 `AdminConsoleModules`。

交付：

- 普通端新壳导航受用户 `sidebar_modules` 和全局 `SidebarModulesAdmin` 共同约束。
- 个人侧栏配置页不再出现后台模块。
- 管理后台新壳导航受全局 `SidebarModulesAdmin` 约束。
- 全局侧栏配置页已按 `普通用户控制台 / 管理员后台` 分组展示：
  - 普通用户控制台：概览、开发接入、费用中心。
  - 管理员后台：运营首页、渠道运营、模型与路由、商业运营、用户运营、系统治理。
- 全局侧栏配置页继续保存到 `SidebarModulesAdmin`，保持旧配置字段兼容。
- 管理员后台 26 个导航项均已绑定 `sidebarSection/sidebarModule`，新旧真实页面和占位页面都可被全局模块开关控制。
- `个人设置` 已从全局侧栏配置展示中移除，统一放入右上角用户信息菜单。
- 暂不改后端字段，避免影响已有配置数据。

当前验证：

- 后台导航配置完整性检查通过：26 个管理员导航项均有配置映射，默认配置无缺失 key。
- `bun run i18n:sync` 通过。
- 自定义缺失 key 扫描通过，当前静态扫描到的 `t()` 文案均存在于七个前端语言包。
- `bun run build` 通过。

### Sprint 4：独立部署准备

状态：前端基础设施已完成，待接入独立部署环境验证。

责任分配：

- 产品：确认是否需要后台独立域名、独立菜单、独立登录后落点。
- UI：确认独立后台的品牌栏、登录态、空态是否与普通端保持一致。
- 前端：拆分 `user-console/routes` 与 `admin-console/routes`，评估 Vite 多入口。
- 后端：确认管理 API、静态资源路径、权限策略与独立部署兼容。

交付：

- 管理端路由、导航、页面、样式在文件夹层面独立维护。
- 保留共享组件，不复制业务页面。
- 独立部署前无需修改核心业务页面。
- `admin.html` 作为管理端独立 HTML 入口。
- `src/apps/admin-console/entry.jsx` 作为管理端独立 React 入口。
- `src/apps/admin-console/AdminStandaloneApp.jsx` 仅挂载管理端页面、认证基础页和旧后台路径跳转，不挂普通用户控制台和门户页面。
- `src/apps/admin-console/routes/adminRoutes.jsx` 作为后台路由唯一维护点，合并版 `App.jsx` 与独立后台 `AdminStandaloneApp.jsx` 共同复用。
- `bun run build:admin` 输出 `dist-admin/index.html`，用于后续管理端独立部署验证。
- `bun run dev:admin` 可用 3004 端口调试管理端入口。

当前验证：

- `bun run i18n:sync` 通过。
- 自定义缺失 key 扫描通过，当前静态扫描到的 `t()` 文案均存在于七个前端语言包。
- `bun run build` 通过，默认合并版仍输出 `dist/index.html`。
- `bun run build:admin` 通过，并生成 `dist-admin/admin.html` 与 `dist-admin/index.html`。
- `bun run dev:admin` 通过，浏览器访问 `/admin/overview` 会加载 `src/apps/admin-console/entry.jsx`，未登录状态正常跳转并渲染登录页。
- 2026-06-06 本轮路由集中化后复测：`bun run build` 和 `bun run build:admin` 均通过。

### Sprint 5：后台真实页面重构

状态：首轮页面承载已完成，进入体验收敛和权限复核。

责任分配：

- 产品：复核 26 个后台入口的首屏主任务，确认低频能力是否进入详情抽屉或二级操作。
- 运营：确认实时监控、渠道预警、结算记录、消费明细、风控记录的默认筛选和处理优先级。
- UI：统一后台表格密度、状态标签、筛选器、批量操作、详情抽屉和嵌入式设置页样式。
- 前端：继续收敛 `admin-console/pages` 页面体验，减少旧页面直接套壳痕迹，保留稳定业务能力。
- 后端：评估经营总览、实时监控和风险中心是否需要聚合 API，降低首页和监控页请求分散度。

已交付：

- 运营首页：`/admin/overview`、`/admin/realtime-monitor`、`/admin/channel-alerts`。
- 渠道运营：`/admin/channels`、`/admin/channel-accounts`、`/admin/channel-balance-monitor`、`/admin/channel-health-check`、`/admin/channel-proxies`。
- 模型与路由：`/admin/model-gateway`、`/admin/models`、`/admin/deployment`、`/admin/route-policy`、`/admin/ratio-config`。
- 商业运营：`/admin/profit-monitor`、`/admin/subscription`、`/admin/redemption`、`/admin/settlements`、`/admin/consumption`。
- 用户运营：`/admin/users`、`/admin/user-segments`、`/admin/risk-records`、`/admin/invite-rebates`。
- 系统治理：`/admin/settings`、`/admin/roles`、`/admin/audit-logs`、`/admin/background-tasks`。
- `AdminRoutePolicy` 与 `AdminRatioConfig` 已增加后台原生配置工作台头部和嵌入式设置组件隔离容器。
- `AdminConsumption`、`AdminRiskRecords`、`AdminAuditLogs` 共用的日志后台变体已增加运营提示条，并统一管理员表格行高和指标卡密度。
- `AdminSettlements` 已增加支付轨道、入账对象、订单状态和补单动作分区，订单表格完成行高固定、数字列和状态列区分。
- `AdminRoles` 已增加权限执行状态表，明确后台路由守卫、系统设置 root-only、用户运营处置和后续 RBAC 拆分责任。
- `AdminSettings` 已从直接套用旧设置页升级为系统治理页，增加全局配置影响范围、当前权限状态和非超级管理员 root-only 提示。
- 已新增 `admin-console/RBAC_PERMISSION_PLAN.md`，将下一阶段菜单、接口和危险操作按钮的权限点拆分、落地顺序和任务分配固化为评审清单。
- 已新增 `AdminPermissionButton`，并将补单按钮接入危险操作权限 `admin:commercial:settlement:complete`；无权限时按钮禁用并显示原因。
- `渠道管理` 已将单行删除、删除所选通道、删除禁用通道接入 `admin:channel:channel:danger`。
- `账号池管理` 已将批量启停、导入、归档/恢复、代理绑定、凭证替换和删除记录接入 `admin:channel:account:danger`，无权限时不打开危险确认或不执行保存请求。
- `用户管理` 已将启用/禁用、升降级、重置安全和注销接入 `admin:user:user:danger`，无权限时不打开危险确认或不执行确认请求。
- `AdminRoles` 已展示菜单权限矩阵和危险操作矩阵，便于产品、运营、后端按同一份权限点评审。

当前验证：

- 管理员导航 26 个 `path` 均能在 `adminRoutes.jsx` 找到对应路由。
- `AdminPlaceholder` 已不再参与后台路由，后台导航中没有 `模块迁移中` 占位入口。
- 未登录访问新增后台入口会进入 `/login`，不出现 404 或白屏。
- 新增后台文案已补齐 `zh-CN / zh-TW / en / fr / ru / ja / vi` 七个语言包。
- 构建和 i18n 检查作为每轮改造后的硬门槛继续保留。
- 审计日志筛选新增了 controller 专项测试，覆盖权限点、权限来源、操作结果、操作人和目标用户 ID 组合过滤。
- 前端新增 `verify:admin-permissions` 权限矩阵脚本，覆盖普通用户、普通管理员、专项管理员、超级管理员和被撤权管理员的菜单与路由权限。
- `verify:admin-permissions` 已补强运营管理员、渠道管理员、模型管理员和财务管理员的允许菜单与代表性禁止直达路由验收。
- 前端新增 `verify:console-migration` 迁移校验脚本，覆盖普通端导航边界、管理端导航边界、用户壳路由覆盖和旧 `/console/*` 后台别名重定向映射。
- Playwright 真实登录态验收已覆盖匿名、普通用户、被撤权管理员、渠道管理员和超级管理员：匿名进入 `/login`，普通用户与被撤权管理员进入 `/forbidden`，渠道管理员只显示渠道运营菜单，超级管理员显示全量后台菜单。
- 页面验收截图已留档：
  - `/Users/frode.luo/project/token-new-api/output/rbac/rbac-channel-admin-menu.png`
  - `/Users/frode.luo/project/token-new-api/output/rbac/rbac-revoked-admin-forbidden.png`
- 专项角色脚本化验收留档：`/Users/frode.luo/project/token-new-api/output/rbac/rbac-specialist-role-verification.md`。

下一批任务：

- 基于 `RBAC_PERMISSION_PLAN.md` 与产品、运营、后端确认角色模板、危险操作和接口守卫优先级。
- 继续补模型管理员、财务管理员和运营管理员三个专项角色的真实页面截图，形成全角色验收留档；本轮浏览器安全策略拒绝本地测试登录态存储注入，需使用真实测试账号或后端测试登录态继续。
- 继续补关键后台写接口的业务级审计摘要，优先扩展其他人工决策接口。
- 继续收敛旧业务组件的内层表单和抽屉样式，重点处理保存按钮、危险操作和空态提示。
- 使用真实管理员登录态完成桌面宽屏视觉验收，补充截图留档。

### Sprint 5.5：RBAC 权限来源与存储迁移

状态：数据库权限来源、内置角色模板同步、权限角色 UI 工作台、权限变更后登录态刷新、审计日志筛选和前端权限矩阵脚本首版已完成。

责任分配：

- 产品：确认角色模板是否采用 `运营管理员 / 渠道管理员 / 模型管理员 / 财务管理员 / 超级管理员` 作为首批可配置角色。
- 运营：确认各角色默认拥有的菜单、危险操作和审计查看范围，尤其是补单、用户处置、渠道删除、倍率与系统设置。
- UI：继续评审权限角色工作台的密度、分组、空态、无权限禁用态和窄屏表现。
- 前端：已将 `AdminRoles` 接入后端权限目录、角色模板同步、角色保存和用户覆盖接口，并保留本地配置作为降级兜底。
- 后端：已完成跨 SQLite / MySQL / PostgreSQL 的权限存储和角色模板同步，下一步补权限变更审计查询和专项角色覆盖测试。

已交付：

- 后端 `middleware/admin_permission.go` 已集中维护完整后台权限目录。
- `/api/admin/permissions/config` 返回角色模板、菜单权限、危险操作权限、常规操作权限和已存储角色。
- `/api/admin/permissions/self` 返回当前管理员的 `admin_permissions`，来源可为 `role_compatibility`、`database` 或 `root`。
- `/api/admin/permissions/roles/sync-templates` 支持 root-only 同步内置运营、渠道、模型、财务和 root 角色模板。
- `/api/admin/permissions/roles` 支持读取、创建、更新和禁用权限角色，写操作仅超级管理员可用。
- `/api/admin/permissions/users/:id` 支持读取和保存用户角色绑定、允许权限和拒绝权限，写操作仅超级管理员可用。
- 前端 `AdminRoles` 已提供后端数据源状态、角色模板列表、角色字段编辑、权限分组勾选、用户角色绑定、允许/拒绝权限覆盖、同步模板和保存确认。
- 登录响应和 `/api/user/self` 已为管理员返回后台权限字段。
- 前端 `loadStoredUserData` 会为旧 localStorage 登录态补取当前后台权限。
- 普通管理员不再获得 root-only 权限，超级管理员继续用 `*` 表示全量权限。
- 后端审计日志会记录权限来源，并可通过 controller 附加角色、目标用户和权限数量摘要。
- 权限角色保存、角色禁用和用户权限绑定保存已补业务级审计摘要，记录状态前后、权限数量前后、角色绑定数量前后以及新增/移除数量，并新增 controller 专项测试覆盖。
- 人工补单已补业务级审计摘要，记录订单号、订单 ID、目标用户、金额、支付方式、支付网关、订单状态前后、完成时间前后、用户额度前后和额度变化，并新增 controller 专项测试覆盖。
- 渠道删除、删除禁用渠道、按 tag 启停和批量改 tag 已补业务级审计摘要，记录请求数量、唯一 ID 数量、无效 ID 数量、目标渠道状态分布、tag 分布和操作前后影响面；批量改 tag 事务内查询已改为同事务读取，避免 SQLite 测试环境写锁等待。
- 系统设置通用保存入口 `PUT /api/option/` 已补业务级审计摘要，记录 option key、请求值类型、是否敏感、变更前后存在状态、值类型、长度、指纹和布尔/数字安全值；敏感配置不记录明文，并新增 controller 专项测试覆盖 before/after 与敏感值脱敏。
- 性能高危清理接口已补业务级审计摘要，覆盖磁盘缓存清理、性能统计重置、强制 GC、日志文件清理和历史日志删除；摘要记录清理模式、阈值、前后数量/大小、实际删除数、释放空间、失败数和 GC 前后内存状态，并新增 controller 专项测试覆盖日志文件清理和历史日志删除。
- 盈利监控建议决策接口已补业务级审计摘要，记录建议 ID、决策状态前后、计划倍率前后、人工决策备注、备注长度/截断状态、建议范围、风险等级、建议原因、推荐倍率和操作人，并新增 controller 专项测试覆盖标准权限审计链路。
- 前端在角色保存、模板同步、角色禁用和用户权限绑定保存后会强制刷新当前登录态权限。
- 审计日志页面已支持按权限来源、权限点、目标用户、操作结果和审计操作人筛选。
- 管理日志展开区已补充权限点、权限来源、操作结果、接口路由、耗时、目标对象和审计摘要。
- `bun run verify:admin-permissions` 已覆盖前端菜单、路由权限点和角色矩阵：普通用户、普通管理员、专项管理员、超级管理员和被撤权管理员。
- Playwright 页面验收已确认匿名、普通用户、被撤权管理员、渠道管理员和超级管理员的实际跳转、菜单和 forbidden 落点。

下一批任务：

- 继续补充其他关键 controller 的业务级变更摘要，优先扩展其他人工决策接口。
- 评审审计筛选区默认条件、字段命名和窄屏布局，确保运营能快速定位危险操作。
- 使用真实登录态继续补页面级验收：运营管理员、模型管理员和财务管理员直达 `/admin/*` 的实际跳转、菜单和空态。

验收：

- 前端菜单过滤、按钮权限和后端接口守卫使用同一份后端权限来源。
- root-only 权限不能被普通管理员授予自己或他人。
- 内置角色模板可由 root 一键同步，首次配置不需要手工创建全部角色。
- 权限变更有审计日志，且不记录敏感字段。
- 权限角色和用户权限绑定变更能在审计摘要中看到 before/after count 与 added/removed count。
- 人工补单能在审计摘要中看到订单状态前后、目标用户和用户额度变化。
- 权限变更后当前登录态权限会刷新，前端菜单和按钮状态不会长期停留在旧权限。
- 审计日志能按权限来源、权限点、目标用户、操作结果和操作人过滤。
- 前端权限矩阵脚本能证明专项角色只进入所属域，被撤权管理员无后台菜单，root-only 菜单不向普通管理员开放。
- 真实页面验收能证明被撤权管理员直达后台会落到 `/forbidden`，渠道管理员不能直达 root-only 系统设置。
- 无数据库权限数据时仍可按 `Role 10 / 100` 兼容旧版本。

### Sprint 6：旧入口收敛与独立部署演练

状态：准备中，当前仍保留兼容跳转，不发布到 pro；静默跳转映射已集中配置并纳入自动校验。

责任分配：

- 产品：确认旧后台入口的提示文案、保留周期和下线阈值。
- 运营：观察旧入口访问量和用户反馈。
- 前端：逐步把旧 `/console/*` 后台入口从静默跳转改为提示跳转，再进入下线准备。
- 后端：验证独立后台静态资源部署、鉴权、CORS、API 网关规则。

交付：

- `adminLegacyRedirects.config.js` 集中维护旧后台入口到 `/admin/*` 的兼容映射。
- `verify:console-migration` 校验普通端与管理端导航隔离、旧入口别名与跳转配置一致。
- 旧后台入口有清晰迁移提示。
- 独立后台构建产物可以在测试环境单独部署。
- 管理员登录后默认落点稳定为 `/admin/overview`。
- 文档、菜单、运营手册都指向 `/admin/*`。

启动条件：

- Sprint 5 视觉收敛通过产品、运营、UI 评审。
- 真实管理员登录态完成 `/admin/*` 主路径验收。
- 测试环境确认 `dist-admin` 的 history fallback、静态资源路径、API 代理和登录后落点。
- 旧 `/console/*` 后台入口访问量和反馈满足下线阈值。
