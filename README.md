# Lanverse

Lanverse 当前交付的是“整剧原稿 → 制作圣经 → 分集 → 场景/制作任务 → 分镜 → 确定性导出”的可审核 MVP。

## 当前架构

```text
Browser / Next.js
        ↓
Go lanverse-api ──→ PostgreSQL（唯一 SQL 事实源）
        ├─────────→ MinIO（私有对象字节）
        └─────────→ Python Candidate Runtime ──→ 本机 Codex CLI
```

- `frontend/`：Next.js 创作工作台，只读取服务端事实并提交人工决议。
- `backend/`：唯一公共业务 API 与唯一业务 Writer；认证、项目、剧本、制作圣经、分集、结构、分镜、正式镜头、导出和持久任务都在此实现。
- `agent/`：私有 Candidate Runtime；校验短时 Execution Grant，只执行结构化 Codex Harness，不连接 PostgreSQL/MinIO，不拥有公共业务路由。
- `backend/internal/platform/database/model`：唯一 GORM Model Catalog 与表结构事实源。
- `backend/api/openapi/lanverse-v1.json`：唯一公共 REST 契约源。
- `backend/internal/agent/contract`：Backend ↔ Agent 的版本化调用/结果线协议所有者；`agent/app/candidate_runtime/schemas.py` 以禁止额外字段的 Pydantic 模型校验同一协议。
- `docs/`：Design → PRD/Requirement → Plan → Acceptance 的事实链路。

MVP 不依赖 Redis、Kafka、Temporal、Elasticsearch、多 Go Binary、第二套 ORM 或 Migration 框架。Backend 只接受一个 PostgreSQL `DATABASE_URL`；仓库不保留手写 SQL Schema/Migration、迁移版本字段或 Python SQLAlchemy Writer。

## 文档入口

- [剧本到分镜 MVP 设计](docs/design/0009-剧本到分镜MVP垂直切片设计.md)
- [剧本到分镜 MVP 产品需求](docs/prd/0009-剧本到分镜MVP产品需求.md)
- [剧本到分镜 MVP 需求规格](docs/requirement/0009-剧本到分镜MVP需求规格.md)
- [剧本到分镜 MVP 实施计划](docs/plan/0009-剧本到分镜MVP实施计划.md)
- [剧本到分镜 MVP 验收记录](docs/acceptance/0009-剧本到分镜MVP验收记录.md)
- [后端服务架构](docs/design/2001-后端服务架构.md)

## 本机启动

先按 `.env.example` 准备根目录 `.env`。私有 Agent 需要本机可用且已登录的 Codex CLI：

```bash
cd agent
uv sync --extra dev
AGENT_EXECUTION_SECRET=development-only-agent-execution-secret \
  uv run uvicorn app.candidate_runtime.api:app --host 127.0.0.1 --port 8787
```

另一个终端启动 Frontend、Backend、PostgreSQL 与 MinIO：

```bash
docker compose --env-file .env \
  -f docker-compose.yml \
  -f docker-compose-env.yml \
  up --build -d
```

开发 Compose 中 Backend 通过 `host.docker.internal:8787` 调用私有 Agent。生产环境必须把 `AGENT_URL` 指向私有网络端点，并为 Backend/Agent 注入相同的高强度 `AGENT_EXECUTION_SECRET`；Agent 不接收数据库、JWT 或对象存储凭据。

需要独立部署 Agent 时，从仓库根目录构建，镜像会固定安装 Codex CLI 并带入本项目所需的 Skill Pack：

```bash
docker build --file agent/Dockerfile --tag lanverse/agent-runtime:development .
```

运行镜像时仍须通过运行平台向容器用户 `/home/lanverse/.codex` 提供有效的 Codex 登录配置；镜像不会复制本机凭据。未提供登录配置时健康检查仍可成功，但生成请求会返回可追踪的 Agent 不可用结果。

对象存储区分 Backend 内部地址 `MINIO_ENDPOINT` 与 Browser 可达的 `MINIO_PUBLIC_ENDPOINT`。Bucket 保持私有，Browser 只使用短时预签名 URL。

## 验证

```bash
cd backend && go test ./... && go vet ./...
cd ../agent && uv run --all-extras python -m ruff check app/candidate_runtime tests/candidate_runtime tests/architecture/test_runtime_language_boundaries.py
uv run --all-extras python -m pyright app/candidate_runtime tests/candidate_runtime tests/architecture/test_runtime_language_boundaries.py
uv run --all-extras python -m pytest tests/candidate_runtime tests/architecture/test_runtime_language_boundaries.py
cd ../frontend && npm run typecheck && npm run lint && npm test
```

最终 `agent-browser` 验收只在实现、真实空库/MinIO 全旅程与自动化回归全部完成后执行；当前进度和未决风险以[验收记录](docs/acceptance/0009-剧本到分镜MVP验收记录.md)为准。

本地隔离环境可通过 `.env.example` 的固定验证码完成注册测试；生产环境未接入验证码投递 Provider 前，自助注册不属于可用能力。
