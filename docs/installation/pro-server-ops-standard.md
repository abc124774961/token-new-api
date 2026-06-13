# Pro 服务器部署与运维标准

最后更新：2026-06-14

本文沉淀当前生产服务器部署、配置基线和近期遇到的故障处理经验。后续处理 `api.tokenbits.net`、gateway、数据库恢复、磁盘清理、统计数据缺失、无可用渠道等问题时，先按本文检查，再做代码或运维变更。

## 适用范围

- 生产服务器：`153.75.90.233`
- 主项目目录：`/www/wwwroot/token-new-api`
- 生产域名：`api.tokenbits.net`
- 生产 Compose：`docker-compose.pro.yml`
- 生产环境文件：`.env.pro`
- 发布方式：服务器 Git 拉取源码并在服务器构建镜像

相关基础文档：

- [服务器 Git 拉取发布流程](server-git-deploy.md)
- [本地镜像打包发布](local-image-deploy.md)
- [同域名拆分服务 Nginx 示例](../split-services-nginx.example.conf)
- [ModelGateway Runtime Sync CI And Ops Guide](../modelgateway-runtime-sync-ci.md)
- [ModelGateway Typed Circuit Policy Ops Guide](../modelgateway-circuit-policy-ops.md)

## 安全边界

- 不把 API token、数据库密码、`.env.pro` 完整内容写入文档、聊天记录、提交信息或公开日志。
- 排查线上问题时，命令里统一使用 `<API_KEY>`、`<SQL_DSN>`、`<PASSWORD>` 这类占位符。
- 不直接修改生产数据库数据来绕过调度问题；优先修代码、刷新缓存、恢复配置，再重建容器。
- 清理磁盘前先确认清理项，禁止删除 `.env.pro`、`.env.pro.*`、`data/`、数据库备份、正在使用的 Docker volume。
- 数据库恢复、批量删除、日志清理必须先做备份或确认保留窗口。

## 标准服务拓扑

生产 Compose 拆成 Web 与 Gateway 两个主服务：

| 服务 | 容器名 | 端口 | 职责 |
| --- | --- | --- | --- |
| `new-api-web` | `token-new-api-web-pro` | `WEB_APP_PORT`，默认 `3000` | 主站页面、管理端、配置保存、后台任务、数据库迁移 |
| `new-api-gateway` | `token-new-api-gateway-pro` | `GATEWAY_APP_PORT`，默认 `3001` | OpenAI 兼容 API、模型请求转发、智能调度 |
| `mailpit` | `token-new-api-pro-mailpit` | 默认 `1026/8026` | 邮件测试/收件箱 |

生产环境原则：

- Web 是 `master` 节点，Gateway 是 `slave` 节点。
- Web 默认允许 DB migration：`WEB_SKIP_DB_AUTO_MIGRATE=false`。
- Gateway 默认禁止 DB migration：`GATEWAY_SKIP_DB_AUTO_MIGRATE=true`。
- Web 负责批处理和管理端配置落库：`WEB_BATCH_UPDATE_ENABLED=true`。
- Gateway 不跑批处理：`GATEWAY_BATCH_UPDATE_ENABLED=false`。
- 容器访问宿主机 MySQL/Redis 时使用 `host.docker.internal`，Compose 中必须保留 `extra_hosts: ["host.docker.internal:host-gateway"]`。
- `SQL_DSN` 必须是单行，不能换行。
- 如果使用 Redis，Web 与 Gateway 必须连接同一个 Redis，否则跨进程运行态和缓存同步会不一致。

## Nginx 路由标准

`api.tokenbits.net` 采用同域名拆分：

- API/Gateway 路径转发到 `127.0.0.1:3001`。
- 主站和管理端转发到 `127.0.0.1:3000`。

Gateway 路径至少包含：

```nginx
location ^~ /v1/ {
    proxy_pass http://127.0.0.1:3001;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 900s;
    proxy_send_timeout 900s;
    proxy_buffering off;
}
```

同样需要将 `/v1beta/`、`/pg/`、`/mj/`、`/suno/`、`/kling/v1/`、`/jimeng/` 等 Gateway 路径放在 `location /` 前面。`/v1/models` 是标准模型列表接口，必须命中 Gateway。

断流类问题优先检查：

- `proxy_read_timeout` 和 `proxy_send_timeout` 是否至少 `900s`。
- Gateway 路径是否错误落到 Web 容器。
- SSE/流式接口是否关闭了 `proxy_buffering`。
- 负载均衡或 CDN 是否有更短的连接超时。

## 生产环境变量基线

`.env.pro` 必须包含但不能公开的核心项：

```env
SESSION_SECRET=<secret>
CRYPTO_SECRET=<secret>
SQL_DSN=<mysql-or-postgres-dsn>
```

推荐保持的公开配置形态：

```env
APP_PORT=3000
WEB_APP_PORT=3000
GATEWAY_APP_PORT=3001
TZ=Asia/Shanghai

ERROR_LOG_ENABLED=true
WEB_BATCH_UPDATE_ENABLED=true
GATEWAY_BATCH_UPDATE_ENABLED=false
WEB_NODE_TYPE=master
WEB_NODE_NAME=web-1
GATEWAY_NODE_TYPE=slave
GATEWAY_NODE_NAME=gateway-1
WEB_SKIP_DB_AUTO_MIGRATE=false
GATEWAY_SKIP_DB_AUTO_MIGRATE=true
GATEWAY_SKIP_CODEX_APPLICATION_ENVIRONMENT_SYNC=true

RELAY_TIMEOUT=900
STREAMING_TIMEOUT=900
STREAM_SCANNER_MAX_BUFFER_MB=256

CHANNEL_FAILURE_AVOIDANCE_ENABLED=true
CHANNEL_FAILURE_AVOIDANCE_TTL_SECONDS=6
CHANNEL_BALANCE_AUTO_RESUME_ENABLED=true
CHANNEL_BALANCE_RECOVERY_THRESHOLD=0
```

`model_execution_records` 表增长较快时，生产建议启用增量保留清理，避免 CPU 和磁盘被大表拖垮：

```env
MODEL_EXECUTION_RECORD_RETENTION_ENABLED=true
MODEL_EXECUTION_RECORD_RETENTION_HOURS=12
MODEL_EXECUTION_RECORD_CLEANUP_INTERVAL_MINUTES=10
MODEL_EXECUTION_RECORD_CLEANUP_BATCH_SIZE=500
MODEL_EXECUTION_RECORD_CLEANUP_MAX_BATCHES=3
```

如果线上业务需要更长审计保留，先评估表大小、索引、查询成本和备份策略，再调整保留窗口。

## 本地与服务器配置同步

本地可以保留 `.env.pro` 的同名变量，方便复现生产问题，但不得提交真实值。同步时只比对变量名和关键开关，不在终端输出完整值：

```bash
cd /Users/frode.luo/project/token-new-api
awk -F= '/^[A-Za-z_][A-Za-z0-9_]*=/{print $1}' .env.pro | sort > /tmp/local-env-keys.txt

ssh root@153.75.90.233 \
  'cd /www/wwwroot/token-new-api && awk -F= '\''/^[A-Za-z_][A-Za-z0-9_]*=/{print $1}'\'' .env.pro | sort' \
  > /tmp/server-env-keys.txt

diff -u /tmp/local-env-keys.txt /tmp/server-env-keys.txt
```

需要核对开关时，只输出白名单键：

```bash
ssh root@153.75.90.233 '
  cd /www/wwwroot/token-new-api
  grep -E "^(WEB_APP_PORT|GATEWAY_APP_PORT|WEB_NODE_TYPE|GATEWAY_NODE_TYPE|WEB_BATCH_UPDATE_ENABLED|GATEWAY_BATCH_UPDATE_ENABLED|WEB_SKIP_DB_AUTO_MIGRATE|GATEWAY_SKIP_DB_AUTO_MIGRATE|MODEL_EXECUTION_RECORD_RETENTION_ENABLED|MODEL_EXECUTION_RECORD_RETENTION_HOURS)=" .env.pro
'
```

不要用 `cat .env.pro`。

## 标准发布流程

发布前本地确认：

```bash
git status --short
git log --oneline -3
```

服务器发布：

```bash
ssh root@153.75.90.233
cd /www/wwwroot/token-new-api
git status --short
git pull --ff-only
scripts/deploy-pro-split.sh
```

只改 Gateway 后端链路时，可以只发布 Gateway：

```bash
cd /www/wwwroot/token-new-api
git pull --ff-only
scripts/deploy-pro-split.sh --gateway
```

只改管理端或后台任务时，可以只发布 Web：

```bash
cd /www/wwwroot/token-new-api
git pull --ff-only
scripts/deploy-pro-split.sh --web
```

发布后检查：

```bash
docker compose --env-file .env.pro -f docker-compose.pro.yml ps
curl -fsS http://127.0.0.1:3000/-/healthz
curl -fsS http://127.0.0.1:3001/-/healthz
curl -fsS http://127.0.0.1:3000/api/status
docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=160 new-api-web
docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=160 new-api-gateway
```

外部检查：

```bash
curl -k -fsS --max-time 15 https://api.tokenbits.net/api/status | head -c 220
curl -k -sS --max-time 20 https://api.tokenbits.net/v1/models \
  -H "Authorization: Bearer <API_KEY>" | head -c 500
```

## 回滚标准

发布前应保留：

- 当前 Git commit。
- `.env.pro`、`.env.pro.*`、`data/`、`logs-web/`、`logs-gateway/`。
- 当前镜像或可重新部署的旧镜像包。

已有 Git 发布回滚流程见 [服务器 Git 拉取发布流程](server-git-deploy.md)。遇到 Gateway 立即不可用时，优先回滚 Gateway 服务，不要先动数据库：

```bash
cd /www/wwwroot/token-new-api
git log --oneline -5
git checkout <last-good-commit>
scripts/deploy-pro-split.sh --gateway --no-pull
```

如果回滚后确认恢复，再决定是否对问题提交做代码修复。

## Gateway 无可用渠道处理标准

典型现象：

- 客户请求返回无可用渠道。
- 后台新增或恢复渠道后仍不生效。
- 重启 Gateway 容器后恢复。
- 明明上游 token 可用，但系统仍认为不可用或模型不存在。

处理原则：

- 不通过手工改生产数据绕过。
- 普通请求失败不能无条件清理隔离或限流。
- 明确恢复动作，包含启用账号、替换凭证、导入新账号、OAuth 刷新成功、健康恢复，才清理对应运行态阻塞。
- 配置变化后必须刷新 routing cache、account candidate index、cost baseline cache，必要时重置 proxy client cache。
- 请求即将返回 503 前允许做限频自愈刷新并重试一次，避免必须重启容器。

排查命令：

```bash
cd /www/wwwroot/token-new-api
docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=300 new-api-gateway \
  | grep -Ei "no available|无可用|refresh|candidate|channel|account|isolation|avoidance|model not found|模型"
```

检查 API 模型列表：

```bash
curl -k -sS -i --max-time 20 https://api.tokenbits.net/v1/models \
  -H "Authorization: Bearer <API_KEY>"
```

对比本机 Gateway：

```bash
curl -sS -i --max-time 20 http://127.0.0.1:3001/v1/models \
  -H "Authorization: Bearer <API_KEY>"
```

判断：

- 外部失败、本机成功：优先查 Nginx/CDN/防火墙/域名路由。
- 本机 Gateway 失败、Web 正常：查 Gateway 缓存、调度、账号候选和 runtime 阻塞。
- Web 和 Gateway 都失败：查 DB、Redis、环境变量、迁移和服务启动日志。

应急重启只作为止血：

```bash
cd /www/wwwroot/token-new-api
docker compose --env-file .env.pro -f docker-compose.pro.yml restart new-api-gateway
```

重启前尽量先保存最近日志：

```bash
stamp=$(date +%Y%m%d%H%M%S)
docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=2000 new-api-gateway \
  > /root/new-api-gateway-incident-$stamp.log
```

## `/v1/models` 标准接口处理标准

标准获取模型接口异常时按顺序检查：

1. 域名是否命中 Gateway：

```bash
curl -k -sS -i https://api.tokenbits.net/v1/models \
  -H "Authorization: Bearer <API_KEY>" | head -n 40
```

2. 本机 Gateway 是否正常：

```bash
curl -sS -i http://127.0.0.1:3001/v1/models \
  -H "Authorization: Bearer <API_KEY>" | head -n 40
```

3. 是否被鉴权或分组过滤：

- `401/403`：查用户 token、分组、过期、余额、权限。
- `404`：查 Nginx 路由或 Gateway 路由注册。
- `200` 但模型少或为空：查用户分组可见模型、渠道启用状态、模型映射、缓存刷新。
- `503`：查无可用渠道、自愈刷新、候选索引、运行态隔离。

4. 查看日志：

```bash
docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=300 new-api-gateway \
  | grep -Ei "models|model list|model not found|permission|group|channel|candidate|cache"
```

## 统计数据缺失处理标准

统计数据突然没有，常见原因：

- `model_execution_records` 清理窗口过短或清理任务误开。
- `LOG_SQL_DSN` 指向错误或日志库连接失败。
- Gateway 与 Web 使用的 DB/Redis 配置不一致。
- 只发布了 Gateway，Web 后台任务或统计查询仍是旧版本。
- 表过大导致查询超时，看起来像没有数据。

检查项：

```bash
cd /www/wwwroot/token-new-api
docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=300 new-api-web \
  | grep -Ei "model execution|cleanup old model execution|log sql|record|统计|timeout"

docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=300 new-api-gateway \
  | grep -Ei "model execution|record|log sql|timeout"
```

如果需要保留最近 12 小时统计，使用增量保留配置，不做一次性大删除。

## CPU 升高与断流处理标准

CPU 突然升高时，先定位来源：

```bash
top -o %CPU
docker stats --no-stream
docker compose --env-file .env.pro -f docker-compose.pro.yml ps
```

如果是 MySQL 高：

```sql
SHOW FULL PROCESSLIST;
SHOW TABLE STATUS LIKE 'model_execution_records';
EXPLAIN SELECT * FROM model_execution_records WHERE created_at > UNIX_TIMESTAMP() - 43200 ORDER BY id DESC LIMIT 50;
```

处理原则：

- 对 `model_execution_records` 做小批量、按时间窗口清理。
- 保留最近 12 小时是当前生产最低建议。
- 避免无条件 `DELETE FROM model_execution_records`。
- 删除前确认备份和业务保留要求。
- 清理期间降低批量大小，避免长事务和锁表。

应用侧建议启用：

```env
MODEL_EXECUTION_RECORD_RETENTION_ENABLED=true
MODEL_EXECUTION_RECORD_RETENTION_HOURS=12
MODEL_EXECUTION_RECORD_CLEANUP_BATCH_SIZE=500
MODEL_EXECUTION_RECORD_CLEANUP_MAX_BATCHES=3
```

断流同时出现时，还要检查 Nginx 超时、Gateway timeout、上游超时、流式 scanner buffer 和 provider 侧失败分类。

## 磁盘清理标准

先看整体：

```bash
df -h
du -xh --max-depth=1 /www/wwwroot | sort -h
du -xh --max-depth=1 /www/wwwroot/token-new-api | sort -h
docker system df
```

清理前必须确认。确认后可以优先处理：

1. Docker build cache：

```bash
docker builder prune
```

2. 未使用镜像和悬空层：

```bash
docker image prune
```

3. 旧发布包和临时归档：

```bash
du -xh --max-depth=1 /root/new-api-image-releases /root/new-api-git-deploy-backups 2>/dev/null | sort -h
```

4. 应用日志：

```bash
du -xh --max-depth=1 /www/wwwroot/token-new-api/logs-web /www/wwwroot/token-new-api/logs-gateway 2>/dev/null | sort -h
```

禁止默认清理：

- `/www/wwwroot/token-new-api/.env.pro`
- `/www/wwwroot/token-new-api/.env.pro.*`
- `/www/wwwroot/token-new-api/data`
- MySQL 数据目录
- 数据库备份目录
- 最近一次可用回滚镜像

## 数据库恢复标准

数据库恢复前：

- 确认备份文件来源和时间。
- 复制备份到服务器固定目录。
- 记录当前 DB 名和连接配置，但不输出密码。
- 先备份当前库或至少导出关键表。
- 恢复期间暂停写入流量或进入维护窗口。

恢复后：

```bash
cd /www/wwwroot/token-new-api
docker compose --env-file .env.pro -f docker-compose.pro.yml restart new-api-web
docker compose --env-file .env.pro -f docker-compose.pro.yml restart new-api-gateway
curl -fsS http://127.0.0.1:3000/api/status
curl -fsS http://127.0.0.1:3001/-/healthz
```

恢复后要重点检查：

- 用户、分组、渠道、账号池是否完整。
- API token 是否存在且分组正确。
- `model_execution_records` 是否过大。
- Web migration 是否成功，Gateway 是否仍禁止 migration。
- 统计页面是否按最新表结构查询。

## 第三方账号池/辅助服务标准

如部署 codex-pool 或 token-account-automation 这类辅助服务：

- 优先使用独立子域名或独立端口，不要覆盖主站 `location /`。
- Nginx 调整后必须验证主站、管理端、Gateway `/v1/models` 都正常。
- 辅助服务 token 只放 `.env.pro` 或服务私有配置，不写入 Git。
- 辅助服务异常不能阻塞主 Gateway 正常请求。
- 上传账号文件前先转换为服务支持的格式，并放到服务约定目录，不混入主项目源码目录。

## 变更后验收清单

每次部署、恢复或配置变更后至少检查：

- `docker compose ps` 中 Web 与 Gateway 均为 running/healthy。
- `http://127.0.0.1:3000/-/healthz` 成功。
- `http://127.0.0.1:3001/-/healthz` 成功。
- `https://api.tokenbits.net/api/status` 成功。
- `https://api.tokenbits.net/v1/models` 使用有效 token 成功。
- 至少一次小模型请求成功。
- 管理端可登录，渠道列表可打开。
- 新增或恢复渠道后，不重启容器也能被 Gateway 看见。
- 日志里没有持续的 DB 连接错误、模型不存在、无可用渠道或 Nginx upstream timeout。

## 故障记录模板

每次线上事故记录：

```text
时间：
影响域名/接口：
现象：
是否影响主站：
是否影响 Gateway：
最近发布 commit：
最近配置变更：
Web 容器状态：
Gateway 容器状态：
CPU/内存/磁盘：
DB 慢查询或大表：
关键日志文件：
采取动作：
是否重启容器：
最终原因：
代码修复：
后续预防：
```

## 后续维护要求

- 服务器 IP、域名、端口、目录、Compose 服务名变化时，先更新本文。
- 新增生产环境变量时，同步更新 `.env.pro.example` 和本文配置基线。
- Gateway 调度、缓存、健康恢复、运行态隔离逻辑变化时，同步更新本文对应故障处理章节。
- 新增清理任务或数据保留策略时，必须写明默认值、风险和回滚方式。
- 涉及生产数据的操作必须保留备份路径和恢复命令。
