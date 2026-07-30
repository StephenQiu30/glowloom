# Lanverse

Lanverse 是面向 AI 短剧生产的模块化单体应用。当前工程从可验证的纵向切片逐步交付，后端使用 FastAPI，前端使用 Next.js App Router、shadcn/ui 与 Radix UI。

## 本地环境

- Python 3.11.15、标准 venv/pip（PyCharm Project venv）
- Node.js 22.23.1、npm 10.9.8
- PostgreSQL 18.4、Redis 8.8.1、RabbitMQ 4.3.4（本机 Homebrew 服务）
- MinIO `RELEASE.2025-09-07T16-13-09Z`（Docker 容器）
- Docker 29.6.2、Compose 5.3.1（仅容器化运行或本机缺少基础设施时需要）

后端以 `backend/.venv` 作为 PyCharm 项目解释器，依赖由 pip 按提交的 requirements 锁安装；前端基于 Vercel 官方 `create-next-app@16.2.12` 生成的 TypeScript/App Router/src 模板。所有依赖均安装在项目目录，不修改系统 Python 或全局 npm。首次执行：

```bash
make setup
make minio-up
make db-init
make dev-api
make dev-frontend
```

API 默认位于 `http://127.0.0.1:8000`，Web 默认位于 `http://127.0.0.1:3000`。完整质量门禁使用 `make check`；浏览器验收需显式执行 `make e2e-install` 与 `make e2e`。

## 配置

首次使用时只需执行 `cp .env.example .env`，后端、前端、OpenAPI 生成和两份 Compose 均从根目录 `.env` 获取配置；不得在 `backend/` 或 `frontend/` 中创建第二份环境文件。`CONTAINER_*` 变量只解决业务容器访问宿主机服务的地址差异，前端只会收到明确列出的 `NEXT_PUBLIC_*` 公共变量，不会继承后端 secret。

当前默认开发拓扑固定为：本机 PostgreSQL、Redis、RabbitMQ + Docker MinIO。只启动对象存储使用 `make minio-up`；只有本机基础设施缺失或需要隔离复现时才执行 `make env-up` 启动完整容器环境，需要容器化业务进程时执行 `make services-up`。根目录 `docker-compose-env.yml` 只负责环境，`docker-compose.yml` 只负责服务。后端 `server` 容器通过一个受监督入口统一运行 API、Scheduler 和 I/O Worker，前端由 `web` 容器运行；真实凭据、媒体、日志和数据不得提交。

Compose 容器统一使用 `lanverse-<模块>` 的稳定名称，例如 `lanverse-server`、`lanverse-web` 和 `lanverse-postgres`，避免默认名称中的实例序号；当前本地拓扑仅支持每个模块运行一个容器实例。

产品、架构、PRD 与执行计划分别位于 `docs/requirement`、`docs/design`、`docs/prd` 和 `docs/plan`。
