# Workflow 公共 HTTP 闭环验收记录

- 状态：实现、真实目标旅程与完整本地 CI 已通过；远端 GitHub Actions 待获准推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)

## 验收范围

本记录验收 Workflow 对 Browser 可调用的 Backend HTTP 最小闭环：从已发布 Authoring Revision 启动真实 Run，按 Run ID 查询 Run/Node 投影，从指定节点派生局部重跑 Run，并按 expected revision 暂停、恢复或取消。范围包括当前身份与 Workspace 权限复核、Temporal Readiness、OpenAPI 和生成 Client；不增加 SSE、前端运行页面、Generation/Asset API 或第二事实源。

## 实现事实

1. `lanverse-api` 组合既有 Authoring、Workflow Compiler、StartService、ControlService、GORM Store 与官方 Temporal Go SDK Client；API 和 Worker 使用同一 Workflow 领域实现，不复制业务层。
2. `POST /api/v1/workflow-runs`、`GET /api/v1/workflow-runs/{run_id}`、`POST /api/v1/workflow-runs/{run_id}/reruns` 和 `POST /api/v1/workflow-runs/{run_id}/controls` 使用标准 `net/http`、严格 JSON 解码、既有 Validator 与统一 Problem Envelope。
3. 查询每次通过 GORM 复核当前 User Status、Token Version、Workspace Membership，并按不可变 Definition ExecutionOrder 返回 Node 投影；不生成 Receipt 或修改 Run。
4. 启动和局部重跑通过 Authoring Revision 所属 Project 的写权限重新授权；Viewer 只能查询，不能启动、重跑或控制。Control 不接受 Browser 自报 Workspace，并在创建 Intent 的同一 GORM 事务中再次授权、锁定 Run revision。
5. API 启动时建立真实 Temporal Client；`/readyz` 在 PostgreSQL 与对象存储之后调用 Temporal SDK `CheckHealth`，任一必需依赖失败统一返回 `dependency_unavailable`，不会在无法调度 Worker 时报告就绪。
6. OpenAPI 是公开契约事实源，已声明四条 Workflow 路由、请求、Run/Node/Control 响应和 UUID path；Frontend Client 从该文档生成。实现未增加 Migration、手写 SQL Schema、兼容路由或第二 ORM。

## 真实验收证据

- Handler 单元测试先验证 Red，再覆盖启动、查询、局部重跑、三种控制映射、严格校验、认证失败与 Error JSON 对象序列化。
- 真实 PostgreSQL 授权旅程验证 Owner/Viewer 可查询，Viewer 不可启动或控制，跨 Workspace 和失效 Token 不可查询，查询不修改 Run 投影。
- 真实 PostgreSQL 16.15 + Temporal HTTP 旅程使用有效 JWT 启动三节点 Run，查询成功终态，再从 `transform` 派生新 Run；实际执行序列为 `source, transform, export, transform, export`，复用的 `source` 为 `SKIPPED` 且未重跑。
- 同一真实 HTTP 旅程将第三个 Run 推进为 `RETRYING → PAUSED → RETRYING → CANCELLED`；取消首次记录请求，Temporal 终止后使用同幂等键再次调用并对账为 Backend 终态。
- Temporal Client `Ping` 在真实服务上通过；Frontend 连续重新生成 Client 后文件哈希不变，OpenAPI JSON 可解析。
- 全新 PostgreSQL/Temporal 环境执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过，Workflow 外部依赖套件实际运行约 66 秒。
- Agent Ruff check/format、Pyright 零错误与 12 个 Pytest 全部通过；Frontend ESLint、TypeScript、45 个 Vitest 与 Next.js 生产构建全部通过。
- 开发/生产 Compose 均可渲染；Frontend、Backend、Agent 三类镜像重新构建成功，并分别通过 standalone、API/Workflow Worker 双二进制和私有 Candidate Runtime 入口检查。

## 边界与残余风险

- 公共 Workflow API 已可用，但当前 Frontend 尚未实现对应运行页面；Generation/Shot、Asset Owner Receipt 和媒体重跑仍是后续 Workflow 切片，不能因本验收报告为已完成。
- 当前 `main` 未获用户授权推送，远端 GitHub Actions 尚未运行本提交；远端 `Required / CI` 只有在获准推送并实际成功后才能报告绿色。
- `agent-browser` 按约定只在全部开发完成后执行；当前仍有明确后续 Workflow 切片，因此本任务不提前调用。
