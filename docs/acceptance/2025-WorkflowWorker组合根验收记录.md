# Workflow Worker 组合根验收记录

- 状态：可部署 `workflow-worker` 与真实 Temporal 单 Script Workflow 切片通过；其他 Production/Agent Executor 尚未接入
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 执行身份与剧本输入节点](2024-Workflow执行身份与剧本输入节点验收记录.md)

## 验收范围

本记录验收 `cmd/workflow-worker → Backend Composition Root → Temporal Worker → Workflow Runtime → Script Application → PostgreSQL Projection` 的最小可部署闭环。API 与 Worker 复用同一 Backend 代码、GORM Model Catalog 和容器镜像；Worker 只通过已声明的 Application/Adapter 依赖执行节点，没有第二数据库、Migration、手写 SQL、自建调度器或 Worker 专用业务实现。

## 实现证据

| 契约 | 结果 |
|---|---|
| 可执行进程 | `backend/cmd/workflow-worker` 校验配置，连接 PostgreSQL，用同一 GORM Model Catalog 同步 Schema，连接 Temporal 并注册正式 Workflow/Activity |
| 模块边界 | `internal/bootstrap` 只组合 Workflow Repository Interface、Script Source 和 Review Application Service；具体 GORM Adapter 仅由 `cmd/workflow-worker` 组合根创建 |
| 共享镜像 | Backend Dockerfile 同时编译 `lanverse-api` 与 `lanverse-workflow-worker`，Compose 用同一 Backend 镜像、不同 Entrypoint 启动两个进程 |
| 运行依赖 | Worker 只需 PostgreSQL 和 Temporal，不伪造 HTTP Health Server；Compose 用依赖健康条件控制启动顺序 |
| 失败路径 | 缺少 `DATABASE_URL`、数据库/Schema Sync/Temporal/Worker 启动失败都终止进程；SIGINT/SIGTERM 停止 Temporal Worker 后关闭连接 |
| CI 防回归 | 必需 Deployment Job 渲染开发/生产 Compose、构建 Backend 镜像，并校验两个二进制均存在 |

## 真实验证

1. Red：真实 Temporal 旅程先调用不存在的 `bootstrap.NewWorkflowRuntime`，原代码编译失败。
2. Green：新旅程在真实 PostgreSQL 中创建单 Script Authoring Revision，用正式 Temporal Client/Worker 完成 `LoadExecutionPlan → ExecuteNode → CompleteRun`；WorkflowRun 与 Script NodeRun 均到达 `SUCCEEDED`，输出绑定与剧本 Revision ID/Hash 一致。
3. 首次全量 CI 真实失败：数据库架构测试拒绝 `internal/bootstrap` 直接导入 GORM。实现没有添加白名单或兼容分支，而是把具体 GORM Adapter 移回 `cmd/workflow-worker` 组合根；架构门禁随后通过。
4. Backend 完整门禁在最终代码快照的全新 PostgreSQL 16.15 空库和固定摘要 Temporal Server 上通过：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`，其中 `backend/tests/workflow` 用时 27.761 秒。
5. 实际 Backend 镜像使用 Go 1.26/Alpine 构建成功，两个二进制均可执行。Worker 容器成功连接宿主 PostgreSQL/Temporal 并注册 Task Queue；发送 SIGTERM 后记录 stopping/stopped 并以退出码 0 结束。无 `DATABASE_URL` 容器按预期退出并报告配置错误。
6. 开发 Compose 与含 Production Overlay 的 Compose 均通过 `docker compose ... config --quiet`；新 Deployment Job 的本地等价命令完整通过。
7. Agent 门禁使用 Python 3.11.15 通过 Ruff、Pyright 和 12 项 Pytest。Frontend 门禁使用官方 Node 22.23.2 容器通过锁定安装、OpenAPI 生成、ESLint、TypeScript、45 项 Vitest 与 Next.js 生产构建。
8. OpenAPI 生成零漂移，Delivery Hygiene 与 `git diff --check` 通过。

## Requirement Checklist

- `BE-MOD-005`：Workflow Runtime 已由独立可部署 Worker 执行，并在真实 Temporal/PostgreSQL 上完成第一个 Owner Node。
- `BE-JRN-003`：Temporal 单 Script Workflow 已从启动到投影终态完成；完整 Episode Graph 仍因其他 Executor 未接入而未完成。

## 残余风险与下一切片

- `workflow-worker` 当前只能完成 `workflow.input.script_revision`；其他非 Human Gate 节点会因未支持 Executor 而失败，这是当前明确边界，不是已通过的全链路。
- API 的 Workflow Start/Query/Control HTTP 边界尚未对外组合，本记录不宣称浏览器可启动 Workflow。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地门禁通过不等于远端绿色；未获准推送前，不宣称远端 CI 已修复。
