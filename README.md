# Lanverse

Lanverse 是前后端分离的 AI 视频生产平台。当前交付切片聚焦“剧本事实主线”：上传整本剧本，异步解析剧集、场景、人物和生产资产，人工批准后将事实写入项目数据。

## 固定架构

- `frontend/`：Next.js App Router、TypeScript、View/ViewModel 分层；API 客户端由 Umi OpenAPI 根据 Swagger 文档生成。
- `admin/`：官方 Ant Design Pro v6 仓库模板的独立管理端，当前只保留登录/注册、管理入口、账号设置、错误页和权限基线，后续按 Lanverse 后端契约接入。
- `backend/`：Go 模块化单体，负责公共 HTTP、PostgreSQL 业务事务、MinIO 对象存储和 Kafka outbox/worker；`backend/cmd/main.go` 是唯一启动入口，使用 `LANVERSE_ROLE` 选择角色。
- `agent/`：Python 私有 Agent 服务，只承载 Harness/Skill 编排，不连接数据库或 Kafka，也不暴露公共 API。
- `docs/`：唯一事实来源，按 `requirement → design → prd → plan → acceptance` 维护。

项目不保留旧 Python 业务后端、RabbitMQ、`/api/v1` 路由或迁移兼容链。数据库由 `backend/schema/current.sql` 初始化为当前结构；不在服务启动时自动改表。

## 本机开发

不使用 Docker，直接复用本机已安装的 PostgreSQL、Kafka、MinIO 和 Node.js 环境。Redis 不是当前剧本解析闭环的依赖。

```bash
cp .env.example .env
brew services start postgresql@18
brew services start kafka
```

确认 PostgreSQL、Kafka、MinIO 已监听 `5432`、`9092`、`9000`，并将 `.env` 中 MinIO 凭据改为本机实例真实值。

初始化当前数据库结构：

```bash
cd backend
go mod download
set -a; source ../.env; set +a
LANVERSE_ROLE=schema-init go run ./cmd
```

启动 Go API 和 Kafka worker（分别使用两个终端）：

```bash
cd backend
set -a; source ../.env; set +a
LANVERSE_ROLE=api go run ./cmd
LANVERSE_ROLE=operation-worker go run ./cmd
```

启动私有 Agent（可选；当前事实解析使用 Go 当前解析器）：

```bash
PYTHONPATH=agent/src uv run --project agent python -m main
```

启动前端：

```bash
cd frontend
npm ci
OPENAPI_SCHEMA_URL=../backend/api/openapi.json npm run openapi2ts
npm run dev
```

启动管理端：

```bash
cd admin
pnpm install --frozen-lockfile
pnpm run dev
```

访问：

- 前端：`http://127.0.0.1:8123`
- 管理端：Umi 默认开发端口（终端输出为准）
- Go API 就绪检查：`http://127.0.0.1:8686/readyz`
- Agent 私有就绪检查：`http://127.0.0.1:8790/readyz`
- Swagger/OpenAPI 源：`backend/api/openapi.json`

## 验证

```bash
cd backend && gofmt -w cmd src && go test ./...
cd ../frontend && npm run lint && npm run typecheck && npm run test && npm run build
cd .. && PYTHONPATH=agent/src uv run --project agent --extra test python -m pytest agent/tests -q
```

剧本解析的验收路径是：前端提交 → Go 创建 revision → MinIO 保存原文 → PostgreSQL outbox → Kafka → operation-worker 解析 → draft → 人工批准 → episode/narrative/entity/production requirement 同事务物化。该路径使用本机真实服务验证，不使用模拟队列或兼容接口。

## 文档与安全

正式设计只位于 `docs/requirement`、`docs/design`、`docs/prd`、`docs/plan` 和 `docs/acceptance`。真实凭据、媒体、日志、数据库和对象存储数据不得提交。
