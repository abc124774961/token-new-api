# 本地镜像打包发布

这套流程在本地构建生产 Docker 镜像，然后把镜像压缩包上传到服务器，服务器只执行 `docker load` 和容器重建。适合服务器资源紧张、避免在服务器上 `docker build` 的场景。

脚本不会复制或覆盖服务器上的 `.env.pro`、`data/`、`logs/`。生产配置仍然以服务器 `/www/wwwroot/token-new-api/.env.pro` 为准。

如果采用服务器直接 `git pull` 并在服务器构建镜像，请看 `docs/installation/server-git-deploy.md`。

## 本地文件

- 构建脚本：`scripts/build-pro-image-archive.sh`
- 发布脚本：`scripts/deploy-pro-image-archive.sh`
- 本说明：`docs/installation/local-image-deploy.md`

## 默认配置

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `SERVER_HOST` | `35.224.150.95` | 服务器公网 IP |
| `SERVER_USER` | `root` | SSH 登录用户 |
| `SSH_KEY` | `~/.ssh/gcp-abc124774961` | 本地 SSH 私钥 |
| `SERVER_APP_DIR` | `/www/wwwroot/token-new-api` | 服务器项目目录 |
| `COMPOSE_FILE` | `docker-compose.pro.yml` | 服务器上的 Compose 文件 |
| `ENV_FILE` | `.env.pro` | 服务器上的生产环境文件 |
| `SERVICE` | `new-api` | Compose 服务名，对应 `docker-compose.pro.yml` 中的 `services.new-api` |
| `IMAGE_NAME` | `token-new-api-pro:latest` | 生产镜像名，对应 Compose 中的 `image` |
| `PLATFORM` | `linux/amd64` | 镜像平台，适配当前服务器架构 |
| `HEALTH_URL` | `http://127.0.0.1:3000/api/status` | 容器重建后的本机健康检查地址 |
| `REMOTE_RELEASE_DIR` | `/root/new-api-image-releases` | 服务器镜像包存放目录 |

## 1. 本地构建镜像包

```bash
scripts/build-pro-image-archive.sh
```

脚本会：

- 使用 `Dockerfile.pro.cn` 构建 `linux/amd64` 镜像；
- 打包为 `output/deploy/*.tar.gz`；
- 在最后一行输出镜像包路径。

可按需覆盖默认值：

```bash
IMAGE_NAME=token-new-api-pro:latest \
PLATFORM=linux/amd64 \
OUT_DIR=output/deploy \
scripts/build-pro-image-archive.sh
```

## 2. 上传并重启服务

把上一步输出的路径传给发布脚本：

```bash
ARCHIVE=/absolute/path/to/token-new-api-pro-latest-linux-amd64-YYYYMMDDHHMMSS.tar.gz \
scripts/deploy-pro-image-archive.sh
```

发布脚本会先检查服务器上这些文件是否存在：

```text
/www/wwwroot/token-new-api/.env.pro
/www/wwwroot/token-new-api/docker-compose.pro.yml
```

发布过程不会读取或输出 `.env.pro` 内容，只检查文件存在性。

检查通过后，服务器侧执行：

```bash
docker load
docker compose --env-file .env.pro -f docker-compose.pro.yml up -d --no-build --force-recreate new-api
```

最后等待 `/api/status` 返回成功。如果健康检查失败，脚本会输出最近的服务日志并退出。

## 一行完成

```bash
ARCHIVE="$(scripts/build-pro-image-archive.sh | tail -n 1)" \
  scripts/deploy-pro-image-archive.sh
```

## 自定义服务器参数

```bash
SERVER_HOST=35.224.150.95 \
SERVER_USER=root \
SSH_KEY="$HOME/.ssh/gcp-abc124774961" \
SERVER_APP_DIR=/www/wwwroot/token-new-api \
ARCHIVE=/absolute/path/to/image.tar.gz \
scripts/deploy-pro-image-archive.sh
```

## 回滚

发布脚本只加载新镜像并重建 `new-api` 服务，不会删除旧镜像层。需要回滚时，重新指定旧镜像包执行发布脚本即可：

```bash
ARCHIVE=/absolute/path/to/old-image.tar.gz \
scripts/deploy-pro-image-archive.sh
```

服务器源码、配置和数据仍建议单独备份。当前服务器已有源码备份位置：

```text
/root/new-api-restore-backups/
```
