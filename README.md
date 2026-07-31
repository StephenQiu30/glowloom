# Lanverse

Lanverse 是面向 AI 短剧生产的模块化单体应用。当前工程从可验证的纵向切片逐步交付，后端使用 FastAPI，前端使用 Next.js App Router、shadcn/ui 与 Radix UI。

## 本地环境

- Python 3.11.15、标准 venv/pip（PyCharm Project venv）
- Node.js 22.23.1、npm 10.9.8
- PostgreSQL 18.4、Redis 8.8.1、RabbitMQ 4.3.4（本机 Homebrew 服务）
- MinIO（Docker Compose 使用 `minio/minio:latest`）
- Docker 29.6.2、Compose 5.3.1（仅运行 MinIO、完整容器环境或验证镜像时需要）

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

首次使用时执行 `cp .env.example .env` 创建开发配置；后端、前端、OpenAPI 生成和基础 Docker Compose 均从根目录 `.env` 获取配置。生产部署执行 `cp .env.production.example .env.production`，再填写生产域名、镜像仓库和全部必需 secret。两个模板保持完全一致的变量集合，包含应用连接串、隔离测试数据库、PostgreSQL 初始化参数及其他基础设施配置，只在默认值和密钥策略上区分环境；两个真实环境文件都不得提交，也不得在 `backend/` 或 `frontend/` 中重复创建。前端只会收到明确列出的 `NEXT_PUBLIC_*` 公共变量，这些值会进入浏览器包，不能保存 secret。

本地开发默认使用本机 PostgreSQL、Redis、RabbitMQ，MinIO 可由 `make minio-up` 按需提供；FastAPI、Scheduler、Worker 与 Next.js 继续使用上面的本地命令运行，不要求 Docker。需要快速启动完整开发环境时，根 `docker-compose.yml` 会构建并启动 PostgreSQL、Redis、RabbitMQ、MinIO、后端和前端；生产部署在该事实源上叠加很薄的 `docker-compose.prod.yml`，隐藏四项基础设施的宿主机端口、禁用现场构建并拉取已发布的 server/web 镜像。所有服务都有健康检查，应用只在依赖健康后启动。基础镜像参考 [`StephenQiu30/code-ark`](https://github.com/StephenQiu30/code-ark) 的直接配置方式使用 `latest`，不维护额外版本守卫。

开发环境使用 `make docker-dev-up`，日志和停止命令分别为 `make docker-dev-logs`、`make docker-dev-down`。生产环境在镜像已发布且 `.env.production` 已填写后使用 `make docker-prod-up`，日志和停止命令分别为 `make docker-prod-logs`、`make docker-prod-down`；生产启动固定使用 `--no-build --pull always --wait`。真实凭据、媒体、日志和数据不得提交。

产品、架构、PRD 与执行计划分别位于 `docs/requirement`、`docs/design`、`docs/prd` 和 `docs/plan`。
