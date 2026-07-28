# Lanverse

Lanverse 是面向 AI 短剧生产的模块化单体应用。当前工程从可验证的纵向切片逐步交付，后端使用 FastAPI，前端使用 Next.js App Router、shadcn/ui 与 Radix UI。

## 本地环境

- Python 3.11.15、uv 0.11.32
- Node.js 22.23.1、npm 10.9.8
- PostgreSQL 18.4、Redis 8.8.0
- Docker 29.6.2、Compose 5.3.1（仅 RabbitMQ/MinIO 契约测试需要）

所有依赖均安装在项目目录，不修改系统 Python 或全局 npm。首次执行：

```bash
make setup
cd frontend && npm run dev
```

Web 默认位于 `http://127.0.0.1:3000`。当前工程基线质量门禁使用 `make check`；API 与端到端命令将在对应纵向任务实现时加入。

## 配置

从 `backend/.env.example`、`frontend/.env.example` 和 `deploy/.env.example` 复制本地配置。真实凭据、媒体、日志和数据不得提交。

产品、架构、PRD 与执行计划分别位于 `docs/requirement`、`docs/design`、`docs/prd` 和 `docs/plan`。
