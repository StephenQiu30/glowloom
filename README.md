# Lanverse

Lanverse 是面向 AI 短剧生产的模块化单体应用。后端使用 FastAPI，前端使用 Next.js App Router、shadcn/ui 与 Radix UI。

## 本地开发

本地模式直接复用 Homebrew 安装的 PostgreSQL 18.4、Redis 8.8.1、RabbitMQ 4.3.4 和 MinIO；应用使用 Python 3.11.15、Node.js 22.23.1 与 npm 10.9.8。Docker 不是本地开发前置。

首次安装项目依赖：

```bash
cp .env.example .env
python3.11 -m venv backend/.venv
backend/.venv/bin/python -m pip install 'pip==26.1.2'
backend/.venv/bin/python -m pip install --requirement backend/requirements-dev.txt
backend/.venv/bin/python -m pip check
cd frontend && npm ci
```

启动本机 PostgreSQL、Redis 和 RabbitMQ：

```bash
brew services start postgresql@18
brew services start redis
brew services start rabbitmq
```

MinIO 不由项目启动或管理，直接复用本机通过 Homebrew 安装并已运行的实例。当前应用连接 `127.0.0.1:9000`，Console 位于 `127.0.0.1:9001`；确认端口即可：

```bash
lsof -nP -iTCP:9000 -sTCP:LISTEN
```

`.env` 中的 `MINIO_ACCESS_KEY` 和 `MINIO_SECRET_KEY` 必须与该实例一致。项目不会重启、停止或修改这项本机服务。

初始化数据库并启动完整后端角色：

```bash
cd backend
.venv/bin/python -m app.initialize_database
.venv/bin/python -m app.server
```

另开终端启动前端：

```bash
cd frontend
npm run dev
```

Web、API 文档和依赖就绪状态分别位于：

- `http://127.0.0.1:8123`
- `http://127.0.0.1:8686/docs`
- `http://127.0.0.1:8686/readyz`

## Docker 一键启动

Docker Compose 保留完整六服务环境，包含 PostgreSQL、Redis、RabbitMQ、MinIO、server 和 web：

```bash
docker compose up -d --build --wait
```

查看日志与停止环境：

```bash
docker compose logs --follow
docker compose down
```

为避免占用本机 MinIO 的 `9000/9001`，Compose 将自己的 MinIO 发布到宿主机 `9100/9101`；容器内部仍通过 `minio:9000` 通信。两套环境可以并存。

生产环境在镜像已发布且 `.env.production` 已填写后执行：

```bash
docker compose \
  --env-file .env.production \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  up -d --no-build --pull always --wait
```

## 验证

后端：

```bash
cd backend
.venv/bin/ruff check app tests
.venv/bin/pyright
.venv/bin/python -m pytest
.venv/bin/python -m pip check
```

前端：

```bash
cd frontend
npm run lint
npm run typecheck
npm run test
npm run build
```

Compose 配置：

```bash
docker compose --env-file .env.example config >/dev/null
```

需要真实外部依赖的契约通过对应 `LANVERSE_RUN_*` 环境变量显式开启；CI 中保留了 Redis、RabbitMQ、MinIO、ffprobe、Scheduler、媒体栈和浏览器的完整原生命令作为可执行事实源。

产品、架构、PRD 与执行计划分别位于 `docs/requirement`、`docs/design`、`docs/prd` 和 `docs/plan`。真实凭据、媒体、日志和本地数据不得提交。
