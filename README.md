# Lanverse

Lanverse 是一个以前后端分离方式实现的剧本解析与分镜生产服务。当前唯一优先主线是：上传剧本，通过受控 Skill Harness 解析剧集、场景、人物及人物出场集数，再按剧集生成可编辑的分镜表。

## 架构边界

- `backend/`：Python 3.11 + FastAPI 模块化单体，持有业务事务、Skill Harness、剧本解析和分镜能力。
- `frontend/`：Next.js App Router + TypeScript；API 客户端由 FastAPI OpenAPI 契约生成。
- `docs/`：按 `requirement → design → prd → plan → acceptance` 记录长期有效的产品与工程事实。

项目不包含 Go 后端、独立 Agent 服务、独立 Admin 服务或邀请功能。Harness/Skill 是 FastAPI 进程内的受控执行能力，不是另一套业务服务。

## 本机开发

项目不要求 Docker，直接复用本机 Python、PostgreSQL、Redis、RabbitMQ、MinIO 和 Node.js 环境。

```bash
cp .env.example .env
cd backend
uv venv --python 3.11 .venv
uv pip install -e '.[dev]'
set -a; source ../.env; set +a
.venv/bin/python -m app.initialize_database
.venv/bin/python -m app.server
```

另开终端启动前端：

```bash
cd frontend
npm ci
OPENAPI_SCHEMA_URL=../backend/openapi.json npm run openapi2ts
npm run dev
```

默认地址：

- 前端：`http://127.0.0.1:8123`
- FastAPI：`http://127.0.0.1:8686`
- 健康检查：`http://127.0.0.1:8686/healthz`
- OpenAPI：`http://127.0.0.1:8686/openapi.json`

## 契约与验证

修改 FastAPI 路由或响应模型后，重新生成 OpenAPI 和前端客户端：

```bash
cd backend
.venv/bin/python -m scripts.export_openapi
cd ../frontend
OPENAPI_SCHEMA_URL=../backend/openapi.json npm run openapi2ts
```

本机质量检查：

```bash
cd backend
.venv/bin/ruff check app tests scripts
.venv/bin/ruff format --check app tests scripts
.venv/bin/pyright app tests scripts
.venv/bin/python -m pytest tests/unit tests/contract tests/architecture -q

cd ../frontend
npm run lint
npm run typecheck
npm run test
npm run build
```

数据库集成测试只连接 `.env` 中显式配置、名称以 `_test` 结尾的 `TEST_DATABASE_URL`，不会自动创建容器。
