# 管理员后台 RBAC 权限点拆分清单

## 目标

在现有 `Role 1 / 10 / 100` 固定角色模型上，平滑演进到菜单、接口和危险操作按钮三级权限控制。

当前已落地的事实：

- `/admin/*` 统一经过 `AdminRoute`。
- `AdminRoute` 已支持可选 `permission` 参数；无权限点数据时继续以 `role >= 10` 兼容历史管理员后台访问。
- 管理员后台 26 个菜单入口已集中映射到 `adminPermissions.config.js`。
- 管理员后台侧栏已支持基于用户权限点动态过滤菜单。
- `AdminRoles` 已从只读矩阵升级为可配置工作台，支持后端数据源状态、角色模板、角色编辑、权限分组、用户角色绑定、允许/拒绝权限覆盖和 root-only 保存操作。
- 10 个危险操作权限点已集中映射到 `adminPermissions.config.js`。
- 8 个常规操作权限点已集中映射到 `adminPermissions.config.js`，菜单权限、危险操作和常规操作合计 44 个权限点。
- 已新增 `AdminPermissionButton`，支持 `requiredPermission`、`dangerPermission` 和 `fallbackTooltip`。
- `结算记录` 与普通充值历史弹窗中的 `补单` 按钮已接入 `admin:commercial:settlement:complete`。
- `渠道管理` 的单行删除、删除所选通道、删除禁用通道已接入 `admin:channel:channel:danger`。
- `账号池管理` 的批量启停、导入账号、归档/恢复、代理绑定、凭证替换、删除账号和删除归档记录已接入 `admin:channel:account:danger`。
- `用户管理` 的启用/禁用、升降级、重置 Passkey、重置 2FA 和注销用户已接入 `admin:user:user:danger`。
- `倍率配置` 的模型定价保存、未设置价格模型保存、旧 JSON 倍率保存/重置、分组倍率保存、上游价格同步和工具价格保存已接入 `admin:model:ratio:update`。
- `路由策略` 的保存智能调度配置和恢复默认配置已接入 `admin:model:route_policy:danger`。
- `性能设置` 的保存性能配置已接入 `admin:system:settings:update`，清理缓存、重置统计、触发 GC 和清理日志已接入 `admin:system:performance:danger`。
- `系统设置` 的服务器地址、Worker、SSRF、登录注册、Passkey、邮箱白名单、SMTP、OIDC、GitHub/Discord/Linux DO/WeChat/Telegram OAuth、Turnstile 和自定义 OAuth 新建/编辑/删除已接入 `admin:system:settings:update`。
- `速率限制设置` 的模型请求速率限制保存已接入 `admin:system:settings:update`。
- `模型相关设置` 的全局模型、Claude、Gemini、Grok 配置保存已接入 `admin:system:settings:update`，渠道亲和规则保存和缓存清理已接入 `admin:model:route_policy:danger`。
- `模型部署设置` 的 io.net 部署启用和 API Key 保存已接入 `admin:system:settings:update`。
- `支付设置` 的通用支付、易支付、Stripe、Creem、Waffo 和 Waffo Pancake 保存已接入 `admin:system:settings:update`。
- `后台任务` 当前前端只提供查询、筛选、预览和列配置，没有发现执行类写操作入口。
- 后端已新增 `RequireAdminPermission(permission)` / `RequireRootAdminPermission(permission)`，首批接口以现有 `Role 10 / 100` 作为兼容权限源，并在请求上下文写入 `admin_permission`。
- 后端接口守卫已覆盖人工补单、用户危险操作、渠道新增/编辑/删除、账号池危险操作、渠道健康执行、代理维护、模型网关调度/熔断/探活、系统设置、自定义 OAuth、倍率同步、性能清理和日志删除。
- 第二批后端接口守卫已覆盖订阅管理、兑换码管理、供应商和模型元数据、模型部署、盈利监控、动态计费确认、预填分组和上游模型更新等写操作。
- 后端审计日志已接入 `RequireAdminPermission`，所有已挂权限点的后台写接口会记录 `LogTypeManage`，并在 `other.admin_info` 写入权限点、结果、操作者、角色、请求 ID、目标路径参数、查询标识、JSON body 字段名、白名单标识字段和耗时。
- 前端权限源识别已避免把 `/api/user/self` 返回的侧栏 `permissions` 对象误判为后台权限点列表，后端未返回精细权限列表时继续按历史角色兼容。
- 后端已提供权限目录只读接口 `/api/admin/permissions/config`，返回角色模板、菜单权限、危险操作权限、常规操作权限和已存储角色。
- 后端已提供当前管理员权限接口 `/api/admin/permissions/self`，来源可为 `role_compatibility`、`database` 或 `root`。
- 后端已新增权限存储模型：`admin_roles`、`admin_role_permissions`、`admin_user_role_bindings`、`admin_user_permission_overrides`。
- 后端已新增 root-only 权限分配接口，覆盖角色模板同步、角色创建/更新/禁用与用户角色/权限覆盖保存。
- `RequireAdminPermission` 已接入数据库权限来源；用户没有存储权限配置时继续按历史角色兼容。
- 后端审计日志已记录权限来源，权限变更接口会附加角色、目标用户、状态前后、权限数量前后和新增/移除数量摘要。
- 人工补单接口已补业务级审计摘要，附加订单号、订单 ID、目标用户、金额、支付方式、支付网关、订单状态前后、完成时间前后、用户额度前后和额度变化。
- 系统设置通用保存入口已补业务级审计摘要，附加 option key、请求值类型、是否敏感、变更前后存在状态、值类型、长度、指纹和布尔/数字安全值；敏感配置不记录明文。
- 性能高危清理接口已补业务级审计摘要，覆盖磁盘缓存清理、性能统计重置、强制 GC、日志文件清理和历史日志删除，记录清理模式、阈值、前后数量/大小、实际删除数、释放空间、失败数和 GC 前后内存状态。
- 盈利监控建议决策接口已补业务级审计摘要，附加决策状态前后、计划倍率前后、人工决策备注、建议范围、风险等级、建议原因、推荐倍率和操作人。
- 登录响应和 `/api/user/self` 已为管理员返回 `admin_permissions`、`admin_permission_mode` 和 `admin_permission_source`。
- 前端已在旧 localStorage 登录态恢复时补取当前管理员权限，避免独立后台入口长期依赖本地角色猜测。
- 前端已在角色保存、模板同步、角色禁用和用户权限绑定保存后强制刷新当前登录态权限，避免管理员调整自己权限后前端状态滞后。
- 审计日志已支持按权限点、权限来源、操作结果、审计操作人和目标用户 ID 筛选，并在管理日志展开区展示权限点、来源、结果、接口路由、耗时、目标对象和审计摘要。
- 已新增 `bun run verify:admin-permissions` 前端权限矩阵校验，覆盖普通用户、普通管理员、专项管理员、超级管理员和被撤权管理员的菜单与路由权限。
- `verify:admin-permissions` 已补强运营管理员、渠道管理员、模型管理员和财务管理员的允许菜单与代表性禁止直达路由验收。
- 已完成首轮 Playwright 页面级验收，覆盖匿名、普通用户、被撤权管理员、渠道管理员和超级管理员：匿名进入 `/login`，普通用户与被撤权管理员进入 `/forbidden`，渠道管理员只能看到渠道运营菜单，超级管理员能看到全量 26 个后台入口。
- 页面级验收截图已留档：`output/rbac/rbac-channel-admin-menu.png`、`output/rbac/rbac-revoked-admin-forbidden.png`。
- 专项角色脚本化验收留档：`output/rbac/rbac-specialist-role-verification.md`。
- 系统设置页签当前通过 `isRoot()` 仅对 `role >= 100` 渲染。
- 管理员后台当前有 6 个一级分组、26 个二级入口。

## 角色分层

| 角色       | 当前映射                | 目标职责                                 | 默认能力                 |
| ---------- | ----------------------- | ---------------------------------------- | ------------------------ |
| 普通用户   | `role < 10`             | 使用普通控制台                           | 不进入 `/admin/*`        |
| 运营管理员 | `role >= 10` 的目标细分 | 查看运营首页、处理用户、结算、日志和风险 | 可读为主，少量低风险操作 |
| 渠道管理员 | `role >= 10` 的目标细分 | 管理渠道、账号池、代理、健康检测         | 渠道写操作和探活操作     |
| 模型管理员 | `role >= 10` 的目标细分 | 管理模型、部署、路由策略                 | 模型和路由配置操作       |
| 财务管理员 | `role >= 10` 的目标细分 | 查看盈利、订阅、兑换码、结算             | 商业运营读写和补单复核   |
| 超级管理员 | `role >= 100`           | 系统治理、全局配置、危险操作兜底         | 全部权限                 |

第一阶段不要删除现有数字角色。新增权限点应作为 `role` 的补充，保持历史管理员账号可用。

## 权限命名

建议使用稳定字符串，避免和页面文案绑定：

```text
admin:{domain}:{resource}:{action}
```

示例：

```text
admin:channel:channel:read
admin:channel:channel:update
admin:model:route_policy:update
admin:commercial:settlement:complete
admin:system:settings:update
```

动作建议：

| 动作       | 含义                                        |
| ---------- | ------------------------------------------- |
| `read`     | 查看页面、列表、详情、摘要                  |
| `create`   | 新建资源                                    |
| `update`   | 编辑配置或资源                              |
| `delete`   | 删除、归档、清理                            |
| `execute`  | 立即探活、刷新、同步、重算、GC 等执行类动作 |
| `export`   | 导出日志、下载报表                          |
| `complete` | 结算补单、人工确认入账                      |
| `danger`   | 影响全局或不可逆的高风险操作                |

## 菜单权限点

| 分组       | 路由                             | 权限点                               | 默认角色   | 优先级 |
| ---------- | -------------------------------- | ------------------------------------ | ---------- | ------ |
| 运营首页   | `/admin/overview`                | `admin:operations:overview:read`     | 管理员     | P0     |
| 运营首页   | `/admin/realtime-monitor`        | `admin:operations:runtime:read`      | 管理员     | P0     |
| 运营首页   | `/admin/channel-alerts`          | `admin:operations:alerts:read`       | 管理员     | P0     |
| 渠道运营   | `/admin/channels`                | `admin:channel:channel:read`         | 渠道管理员 | P0     |
| 渠道运营   | `/admin/channel-accounts`        | `admin:channel:account:read`         | 渠道管理员 | P0     |
| 渠道运营   | `/admin/channel-balance-monitor` | `admin:channel:balance:read`         | 渠道管理员 | P0     |
| 渠道运营   | `/admin/channel-health-check`    | `admin:channel:health:read`          | 渠道管理员 | P0     |
| 渠道运营   | `/admin/channel-proxies`         | `admin:channel:proxy:read`           | 渠道管理员 | P1     |
| 模型与路由 | `/admin/model-gateway`           | `admin:model:gateway:read`           | 模型管理员 | P0     |
| 模型与路由 | `/admin/models`                  | `admin:model:model:read`             | 模型管理员 | P0     |
| 模型与路由 | `/admin/deployment`              | `admin:model:deployment:read`        | 模型管理员 | P0     |
| 模型与路由 | `/admin/route-policy`            | `admin:model:route_policy:read`      | 模型管理员 | P0     |
| 模型与路由 | `/admin/ratio-config`            | `admin:model:ratio:read`             | 超级管理员 | P0     |
| 商业运营   | `/admin/profit-monitor`          | `admin:commercial:profit:read`       | 财务管理员 | P0     |
| 商业运营   | `/admin/subscription`            | `admin:commercial:subscription:read` | 财务管理员 | P1     |
| 商业运营   | `/admin/redemption`              | `admin:commercial:redemption:read`   | 财务管理员 | P1     |
| 商业运营   | `/admin/settlements`             | `admin:commercial:settlement:read`   | 财务管理员 | P0     |
| 商业运营   | `/admin/consumption`             | `admin:commercial:consumption:read`  | 财务管理员 | P0     |
| 用户运营   | `/admin/users`                   | `admin:user:user:read`               | 运营管理员 | P0     |
| 用户运营   | `/admin/user-segments`           | `admin:user:segment:read`            | 运营管理员 | P1     |
| 用户运营   | `/admin/risk-records`            | `admin:user:risk:read`               | 运营管理员 | P0     |
| 用户运营   | `/admin/invite-rebates`          | `admin:user:rebate:read`             | 运营管理员 | P1     |
| 系统治理   | `/admin/settings`                | `admin:system:settings:read`         | 超级管理员 | P0     |
| 系统治理   | `/admin/roles`                   | `admin:system:roles:read`            | 管理员     | P0     |
| 系统治理   | `/admin/audit-logs`              | `admin:system:audit:read`            | 超级管理员 | P0     |
| 系统治理   | `/admin/background-tasks`        | `admin:system:task:read`             | 超级管理员 | P1     |

## 接口权限点

### 渠道运营

| 接口范围                                                      | 权限点                             | 风险 |
| ------------------------------------------------------------- | ---------------------------------- | ---- |
| `GET /api/channel/*`                                          | `admin:channel:channel:read`       | 中   |
| `POST/PUT /api/channel/`                                      | `admin:channel:channel:update`     | 高   |
| `DELETE /api/channel/*`                                       | `admin:channel:channel:delete`     | 高   |
| `POST /api/channel/{id}/recover_health`                       | `admin:channel:health:execute`     | 中   |
| `POST /api/model_gateway/observability/runtime/clear_circuit` | `admin:model:gateway:execute`      | 高   |
| `POST /api/channel/fetch_models`                              | `admin:channel:model_sync:execute` | 中   |
| `POST /api/channel/multi_key/manage`                          | `admin:channel:account:update`     | 高   |
| `POST /api/channel/{id}/upstream_cost_recalculate`            | `admin:channel:cost:execute`       | 高   |

### 模型与路由

| 接口范围                                          | 权限点                            | 风险 |
| ------------------------------------------------- | --------------------------------- | ---- |
| `GET /api/model_gateway/config`                   | `admin:model:route_policy:read`   | 中   |
| `PUT /api/model_gateway/config`                   | `admin:model:route_policy:update` | 高   |
| `POST /api/model_gateway/config/reset`            | `admin:model:route_policy:danger` | 高   |
| `GET /api/model_gateway/observability/*`          | `admin:model:gateway:read`        | 中   |
| `POST /api/ratio_sync/fetch`                      | `admin:model:ratio_sync:execute`  | 中   |
| `PUT /api/option/` 中模型、倍率、工具价格相关 key | `admin:model:ratio:update`        | 高   |
| `POST /api/deployments/settings/test-connection`  | `admin:model:deployment:execute`  | 中   |

### 商业运营

| 接口范围                                  | 权限点                                 | 风险 |
| ----------------------------------------- | -------------------------------------- | ---- |
| `GET /api/model_gateway/profit_monitor/*` | `admin:commercial:profit:read`         | 中   |
| `GET /api/user/topup`                     | `admin:commercial:settlement:read`     | 高   |
| `POST /api/user/topup/complete`           | `admin:commercial:settlement:complete` | 高   |
| `PUT /api/option/` 中支付相关 key         | `admin:system:settings:update`         | 高   |
| `PUT /api/option/` 中套餐、兑换码相关 key | `admin:commercial:settings:update`     | 高   |
| 日志消费查询接口                          | `admin:commercial:consumption:read`    | 中   |

### 用户运营

| 接口范围                         | 权限点                   | 风险 |
| -------------------------------- | ------------------------ | ---- |
| `GET /api/user/*`                | `admin:user:user:read`   | 高   |
| 用户编辑、状态调整、安全重置接口 | `admin:user:user:update` | 高   |
| 风控、错误日志查询接口           | `admin:user:risk:read`   | 中   |
| 邀请和返佣相关查询               | `admin:user:rebate:read` | 中   |

### 系统治理

| 接口范围                                                         | 权限点                             | 风险 |
| ---------------------------------------------------------------- | ---------------------------------- | ---- |
| `PUT /api/option/` 中运营、系统、登录、OAuth、SMTP、公告相关 key | `admin:system:settings:update`     | 高   |
| `GET /api/performance/*`                                         | `admin:system:performance:read`    | 中   |
| `DELETE /api/performance/disk_cache`                             | `admin:system:performance:danger`  | 高   |
| `POST /api/performance/gc`                                       | `admin:system:performance:execute` | 中   |
| `DELETE /api/log/*`                                              | `admin:system:log:delete`          | 高   |
| `POST/PUT/DELETE /api/admin/permissions/roles*`                  | `admin:system:roles:update`        | 高   |
| `PUT /api/admin/permissions/users/*`                             | `admin:system:roles:update`        | 高   |
| 审计日志查询接口                                                 | `admin:system:audit:read`          | 高   |
| 后台任务查询接口                                                 | `admin:system:task:read`           | 中   |

## 危险按钮权限点

| 页面         | 操作                                                     | 权限点                                 | 二次确认 |
| ------------ | -------------------------------------------------------- | -------------------------------------- | -------- |
| 渠道管理     | 删除渠道、批量删除、成本重算                             | `admin:channel:channel:danger`         | 必须     |
| 账号池管理   | 批量启停、导入、归档、恢复、代理绑定、凭证替换和删除记录 | `admin:channel:account:danger`         | 必须     |
| 渠道健康检测 | 立即探活、恢复健康、清理熔断                             | `admin:channel:health:execute`         | 建议     |
| 路由策略     | 保存调度配置、恢复默认、保存渠道亲和规则、清理亲和缓存   | `admin:model:route_policy:danger`      | 必须     |
| 倍率配置     | 保存/重置模型倍率、分组倍率、上游价格同步和工具价格      | `admin:model:ratio:update`             | 必须     |
| 结算记录     | 人工补单                                                 | `admin:commercial:settlement:complete` | 必须     |
| 用户管理     | 禁用、重置安全、变更角色                                 | `admin:user:user:danger`               | 必须     |
| 系统设置     | 保存支付、OAuth、SMTP、限流、模型、部署、性能设置        | `admin:system:settings:update`         | 必须     |
| 权限角色     | 创建、更新、禁用角色和分配用户权限                       | `admin:system:roles:update`            | 必须     |
| 性能设置     | 清理缓存、重置统计、触发 GC、删除日志                    | `admin:system:performance:danger`      | 必须     |

## 前端落地顺序

1. 已完成：建立 `adminPermissions.config.js`，集中维护权限点、菜单映射和目标角色模板。
2. 已完成：为 `AdminRoute` 增加可选 `permission` 参数，保持无权限点数据时继续使用 `role >= 10` 兼容。
3. 已完成：导航渲染基于 `hasAdminPermission(permission)` 过滤菜单。
4. 已完成：`AdminRoles` 改为读取权限配置生成菜单权限矩阵，避免文档、页面和代码三份漂移。
5. 已完成：新增 `AdminPermissionButton`，支持 `requiredPermission`、`dangerPermission`、`fallbackTooltip`。
6. 已完成：高风险操作按钮接入权限点，已覆盖 `补单`、渠道删除/批量删除、账号池管理、用户管理、倍率配置、路由策略、渠道亲和规则、性能设置高危操作、系统设置、限流、模型、部署和支付 root-only 写操作。
7. 已完成：后端返回当前用户权限点列表；前端在登录态恢复时会同步更新 `admin_permissions`。
8. 已完成：`AdminRoles` 从只读矩阵升级为可配置工作台，接入后端角色、模板同步和用户覆盖接口。
9. 已完成：前端复核后台任务是否新增执行类入口；当前任务日志页暂无写操作可接入。
10. 已完成：权限变更保存、模板同步和用户绑定保存后刷新当前登录态权限，避免管理员调整自己权限后前端状态滞后。
11. 已完成：补齐前端权限矩阵回归脚本，覆盖普通用户、普通管理员、专项管理员、超级管理员和被撤权管理员。
12. 已完成：首轮真实页面验收覆盖匿名、普通用户、被撤权管理员、渠道管理员和超级管理员，并确认被撤权管理员直达后台时进入 `/forbidden`。
13. 已完成：专项角色脚本化验收补强，明确运营管理员、渠道管理员、模型管理员和财务管理员的允许菜单与禁止直达路由。
14. 下一步：补齐运营管理员、模型管理员和财务管理员三个专项角色的真实页面截图，确认各自菜单和直达页面落点；本轮浏览器安全策略拒绝本地测试登录态存储注入，需要真实测试账号或后端测试登录态继续。

## 后端落地顺序

1. 保留 `role` 字段，新增权限点来源：角色模板、用户覆盖、系统默认三层。
2. 已完成：在管理 API 中间件加入 `RequireAdminPermission(permission)`，首版用现有 `role` 兼容历史后台访问。
3. 已完成：对补单、系统设置、倍率、路由策略、用户角色、渠道删除、账号池危险操作、健康执行、性能清理和日志删除接口加权限守卫。
4. 已完成：扩展接口守卫到订阅、兑换码、供应商、模型元数据、部署、利润监控、代理维护、动态计费确认和上游模型更新写操作。
5. 已完成：后台审计日志记录权限点、操作者、目标路径参数、查询标识、JSON body 字段名、白名单标识字段、请求 ID、执行结果和耗时。
6. 已完成：提供权限配置只读接口和当前管理员权限接口，前端用于登录态权限来源和后续矩阵渲染。
7. 已完成：新增真实权限存储，拆为角色、角色权限、用户角色绑定、用户权限覆盖四类模型，并挂入 AutoMigrate。
8. 已完成：提供权限变更接口，仅超级管理员可用，覆盖角色保存、角色禁用和用户权限分配。
9. 已完成：`RequireAdminPermission` 接入数据库权限来源；存在用户权限配置时按数据库权限判断，无配置时保留 `Role 10 / 100` 兼容。
10. 已完成：为内置角色模板增加 root-only 初始化/同步能力，减少 root 首次配置成本。
11. 已完成：审计日志查询支持按权限点、权限来源、操作结果、操作人和目标用户 ID 过滤。
12. 已完成：权限角色保存、角色禁用和用户权限绑定保存补充业务级审计摘要，并新增 controller 专项测试覆盖 before/after count 与 added/removed count。
13. 已完成：人工补单补充业务级审计摘要，并新增 controller 专项测试覆盖订单状态和用户额度变化。
14. 已完成：渠道删除、删除禁用渠道、按 tag 启停和批量改 tag 补充业务级审计摘要，并新增 controller 专项测试覆盖批量删除和批量改 tag 的影响面统计。
15. 已完成：系统设置通用保存入口 `PUT /api/option/` 补充业务级审计摘要，并新增 controller 专项测试覆盖 before/after 状态和敏感值脱敏。
16. 已完成：性能高危清理接口补充业务级审计摘要，并新增 controller 专项测试覆盖日志文件清理和历史日志删除结果。
17. 已完成：盈利监控建议决策接口补充业务级审计摘要，并新增 controller 专项测试覆盖标准权限审计链路。
18. 下一步：继续补充其他关键 controller 的业务级变更摘要，优先扩展其他人工决策接口。

## 任务分配

| 角色 | 任务                                                         | 交付                                                                                                                                                                                   |
| ---- | ------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 产品 | 确认运营管理员、渠道管理员、模型管理员、财务管理员的职责边界 | 角色定义和默认权限模板已进入可验证状态，下一步确认被撤权管理员的默认落点                                                                                                               |
| 运营 | 标记哪些按钮属于高风险操作，确认二次确认文案                 | 危险操作清单，下一步确认审计筛选默认条件和重点追踪字段                                                                                                                                 |
| UI   | 输出无权限、只读、危险按钮、二次确认弹窗规范                 | 权限交互规范，下一步检查权限角色工作台、审计日志筛选区和 `/forbidden` 落点的窄屏表现                                                                                                   |
| 前端 | 权限配置、菜单过滤、按钮禁用、页面矩阵生成和角色工作台       | 菜单权限框架、危险按钮权限、root-only 写操作、权限角色工作台、用户权限绑定、本地配置兜底、登录态权限刷新、审计筛选和权限矩阵脚本已完成                                                 |
| 后端 | 权限中间件、权限接口、审计字段、数据迁移                     | 权限中间件、两批后台写接口守卫、审计日志、权限来源接口、真实权限存储、root-only 变更接口、内置角色模板同步、审计过滤、权限变更摘要、人工补单摘要、渠道批量操作摘要、系统设置摘要、性能清理摘要和盈利建议决策摘要已完成，下一步扩展其他 controller 摘要 |
| 测试 | 覆盖角色访问、菜单隐藏、接口拒绝、危险操作确认               | 已补权限目录、模板同步、审计过滤和前端权限矩阵专项测试，下一步补真实登录态端到端验收                                                                                                   |

## 验收标准

- 普通用户访问 `/admin/*` 仍被拒绝。
- 管理员只能看到自己有权限的后台菜单。
- 无权限直达路由显示无权限页，不白屏。
- 无权限按钮不触发请求，并显示清晰原因。
- 高风险操作必须二次确认。
- 后端接口权限和前端菜单权限一致。
- 审计日志能记录权限点和操作结果。
- 审计日志能按权限点、权限来源、操作结果、操作人和目标用户 ID 定位具体后台操作。
- `bun run verify:admin-permissions` 能证明普通用户无后台权限、普通管理员遵循历史兼容、专项管理员只进入所属域、超级管理员全量可见、被撤权管理员无后台菜单。

## 兼容策略

- 第一阶段：没有权限点数据的管理员账号按 `role >= 10` 兼容已有后台访问。
- 第二阶段：对危险操作优先启用权限点守卫。
- 第三阶段：菜单和接口全面接入权限点。
- 第四阶段：允许按用户覆盖权限，同时保留角色模板。
