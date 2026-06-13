# 服务器 Git 拉取发布流程

本文记录生产服务器采用 Git 拉取源码、服务器本机构建镜像并重启容器的发布流程。适用于服务器目录已经接入 Git，或需要把现有非 Git 目录改造成 Git 工作区的场景。

当前 pro 服务器的完整运维标准见 [Pro 服务器部署与运维标准](pro-server-ops-standard.md)。本文件只记录 Git 发布动作。

当前生产服务的已知默认值：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `SERVER_HOST` | `153.75.90.233` | 服务器公网 IP |
| `SERVER_USER` | `root` | SSH 登录用户 |
| `APP_DIR` | `/www/wwwroot/token-new-api` | 服务器项目目录 |
| `REPO_URL` | `https://github.com/abc124774961/token-new-api` | Git 远端仓库 |
| `BRANCH` | `main` | 发布分支 |
| `COMPOSE_FILE` | `docker-compose.pro.yml` | 生产 Compose 文件 |
| `ENV_FILE` | `.env.pro` | 生产环境变量文件 |
| `WEB_SERVICE` | `new-api-web` | Web / 管理端 Compose 服务名 |
| `GATEWAY_SERVICE` | `new-api-gateway` | Gateway Compose 服务名 |
| `WEB_CONTAINER` | `token-new-api-web-pro` | Web / 管理端容器名 |
| `GATEWAY_CONTAINER` | `token-new-api-gateway-pro` | Gateway 容器名 |
| `WEB_HEALTH_URL` | `http://127.0.0.1:3000/-/healthz` | Web 本机健康检查地址 |
| `GATEWAY_HEALTH_URL` | `http://127.0.0.1:3001/-/healthz` | Gateway 本机健康检查地址 |

## 保护规则

发布过程中必须保留这些服务器特定文件和目录：

- `.env.pro`
- `.env.pro.*`
- `data/`
- `logs/`
- `logs-web/`
- `logs-gateway/`

不要把 `.env.pro` 内容打印到终端、提交到日志或复制到公开文档。仓库中如果跟踪了 `.env.pro`，服务器工作区必须使用 `git update-index --skip-worktree .env.pro`，避免后续拉取覆盖生产配置。

## 标准发布

本地确认目标代码已经推送到远端：

```bash
git status -sb
git log --oneline -3
git ls-remote origin refs/heads/main
```

服务器执行发布：

```bash
ssh root@153.75.90.233

cd /www/wwwroot/token-new-api
git pull --ff-only
scripts/deploy-pro-split.sh
```

健康检查：

```bash
curl -fsS http://127.0.0.1:3000/-/healthz | grep '"success"[[:space:]]*:[[:space:]]*true'
curl -fsS http://127.0.0.1:3001/-/healthz | grep '"success"[[:space:]]*:[[:space:]]*true'
curl -fsS http://127.0.0.1:3000/api/status | grep '"success"[[:space:]]*:[[:space:]]*true'
docker compose --env-file .env.pro -f docker-compose.pro.yml ps
docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=120 new-api-web
docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=120 new-api-gateway
```

外部域名检查：

```bash
curl -k -fsS --max-time 15 https://api.tokenbits.net/api/status | head -c 220
curl -k -sS --max-time 20 https://api.tokenbits.net/v1/models \
  -H "Authorization: Bearer <API_KEY>" | head -c 500
```

如果服务器公网 `:3000` 或 `:3001` 访问超时，但域名和服务器本机 `127.0.0.1` 正常，通常是防火墙或反向代理访问路径限制，不一定是容器异常。

## 首次接入 Git

仅当服务器项目目录还不是 Git 仓库时使用本节。执行前先确认容器健康：

```bash
ssh root@153.75.90.233 \
  'docker compose --env-file /www/wwwroot/token-new-api/.env.pro -f /www/wwwroot/token-new-api/docker-compose.pro.yml ps; curl -fsS http://127.0.0.1:3000/api/status | head -c 160; echo'
```

创建备份和回滚脚本：

```bash
cd /www/wwwroot/token-new-api
stamp=$(date +%Y%m%d%H%M%S)
backup_root=/root/new-api-git-deploy-backups/$stamp
mkdir -p "$backup_root/preserve"

tar -C /www/wwwroot \
  --exclude=token-new-api/data \
  --exclude=token-new-api/logs \
  --exclude=token-new-api/logs-web \
  --exclude=token-new-api/logs-gateway \
  -czf "$backup_root/token-new-api-source-before-git-$stamp.tar.gz" \
  token-new-api

for path in .env.pro .env.pro.before-docker-gateway .env.pro.before-mysql-restore data logs logs-web logs-gateway; do
  if [ -e "$path" ]; then
    cp -a "$path" "$backup_root/preserve/"
  fi
done

docker image inspect token-new-api-web-pro:latest token-new-api-gateway-pro:latest --format '{{.RepoTags}} {{.Id}}' > "$backup_root/current-image-id.txt" 2>/dev/null || true
docker save token-new-api-web-pro:latest token-new-api-gateway-pro:latest | gzip -c > "$backup_root/token-new-api-pro-current-$stamp.tar.gz"
```

接入 Git 并拉取远端：

```bash
cd /www/wwwroot/token-new-api
app_dir=/www/wwwroot/token-new-api
repo=https://github.com/abc124774961/token-new-api
branch=main
backup_root=/root/new-api-git-deploy-backups/YYYYMMDDHHMMSS

owner_group=$(stat -c "%U:%G" .)
git config --global --add safe.directory "$app_dir" || true

if [ ! -d .git ]; then
  git init
fi

if git remote get-url origin >/dev/null 2>&1; then
  git remote set-url origin "$repo"
else
  git remote add origin "$repo"
fi

GIT_TERMINAL_PROMPT=0 git fetch origin "$branch"
git symbolic-ref HEAD "refs/heads/$branch"
GIT_TERMINAL_PROMPT=0 git reset --hard "origin/$branch"

for path in .env.pro .env.pro.before-docker-gateway .env.pro.before-mysql-restore data logs logs-web logs-gateway; do
  if [ -e "$backup_root/preserve/$path" ]; then
    rm -rf "$path"
    cp -a "$backup_root/preserve/$path" .
  fi
done

git update-index --skip-worktree .env.pro || true
printf ".env.pro\n.env.pro.*\ndata/\nlogs/\nlogs-web/\nlogs-gateway/\n" >> .git/info/exclude
sort -u .git/info/exclude -o .git/info/exclude
git branch --set-upstream-to=origin/main main
chown -R "$owner_group" "$app_dir"

git rev-parse --short HEAD
git status --short
```

注意：`backup_root` 要替换成上一步实际输出的备份目录。首次接入后，确认 `git status --short` 没有意外变更。

## 回滚

每次发布前建议生成回滚脚本。脚本应恢复生产配置和旧镜像，然后重建服务：

```bash
cat > "$backup_root/rollback.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
backup_root="$backup_root"
cd /www/wwwroot/token-new-api

for path in .env.pro .env.pro.before-docker-gateway .env.pro.before-mysql-restore data logs logs-web logs-gateway; do
  if [ -e "\$backup_root/preserve/\$path" ]; then
    rm -rf "\$path"
    cp -a "\$backup_root/preserve/\$path" .
  fi
done

gzip -dc "\$backup_root/token-new-api-pro-current-$stamp.tar.gz" | docker load
docker compose --env-file .env.pro -f docker-compose.pro.yml up -d --no-build --force-recreate new-api-web new-api-gateway

for i in \$(seq 1 45); do
  if curl -fsS http://127.0.0.1:3000/api/status | grep -q '"success"[[:space:]]*:[[:space:]]*true' \
    && curl -fsS http://127.0.0.1:3001/-/healthz | grep -q '"success"[[:space:]]*:[[:space:]]*true'; then
    docker compose --env-file .env.pro -f docker-compose.pro.yml ps new-api-web new-api-gateway
    exit 0
  fi
  sleep 2
done

docker compose --env-file .env.pro -f docker-compose.pro.yml logs --tail=160 new-api-web new-api-gateway >&2
exit 1
EOF

chmod 700 "$backup_root/rollback.sh"
```

出现问题时执行：

```bash
bash /root/new-api-git-deploy-backups/YYYYMMDDHHMMSS/rollback.sh
```

本次发布实际生成过的回滚脚本示例：

```text
/root/new-api-git-deploy-backups/20260601162955/rollback.sh
```

## 发布检查清单

- 远端 `main` 已包含要发布的提交。
- 服务器当前 Web 和 Gateway 容器在发布前是健康状态。
- 已创建 `/root/new-api-git-deploy-backups/<timestamp>/` 备份。
- `.env.pro`、`data/`、`logs/`、`logs-web/`、`logs-gateway/` 已复制到备份目录。
- 新镜像 build 成功后再重建容器。
- `/api/status` 返回 `success: true`。
- Web 与 Gateway Docker 健康状态变成 `healthy`。
- 最近日志没有启动失败、数据库连接失败或配置缺失错误。
- `git ls-files -v .env.pro` 输出以 `S` 开头，表示生产 `.env.pro` 已被 `skip-worktree` 保护。
