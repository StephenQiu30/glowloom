# Workflow 执行身份与剧本输入节点验收记录

- 状态：WorkflowRun 执行身份与 `workflow.input.script_revision` Production Executor 切片通过；生产 Worker 组合根仍待后续切片
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 节点输入冻结](2019-Workflow节点输入冻结验收记录.md) · [Workflow 节点输出绑定](2018-Workflow节点输出绑定验收记录.md)

## 验收范围

本记录验收 `Workflow Start Actor → WorkflowRun 身份事实 → Node Claim → NodeExecutorCommand → Script Application 授权读取 → script_revision node-output-v1` 的最小闭环。Backend 是执行投影和 Script 业务事实的唯一 Writer，PostgreSQL/GORM Model Catalog 是唯一 SQL 事实源。本切片没有 Migration、手写 SQL、默认 Token Version、兼容回退、第二 ORM 或 Repository 旁路。

## 实现证据

| 契约 | 结果 |
|---|---|
| 启动身份 | Workflow Start 必须接收有效 User ID 和 Token Version，并把 User ID/Token Version 与 Workspace/Project 一起持久化到 WorkflowRun |
| 幂等边界 | 相同 Start Idempotency Key 重放时，持久 Run 的 Initiator User ID 或 Token Version 与当前启动身份不同则拒绝收敛 |
| Claim 传递 | Runtime 仅从已提交 WorkflowRun 生成 Workspace ID、Project ID、Initiator User ID 和 Initiator Token Version；Temporal Payload 不能自报授权上下文 |
| 正式 Owner 访问 | `workflow.input.script_revision` 通过 Script Application Service `GetRevision` 重新鉴权，没有直接访问 Script Repository |
| 不可变性 | Executor 逐项比对 Workspace、Project、Revision ID、Version 和 Normalized Hash，任一漂移都拒绝输出 |
| 撤销语义 | Owner Application 用当前 UserAccount 核对启动时 Token Version；账户 Token Version 提升后，尚未执行节点 fail closed |
| 输出契约 | 成功读取后只返回编译期望的 `script:script_revision` canonical `node-output-v1` |

## 真实验证

1. Red：首先在 Runtime Service 与真实 PostgreSQL 测试中声明运行身份和 Production Executor；原实现因缺少 `NewNodeExecutor` 与 Claim/Command 身份字段而编译失败。接入 Script Application 后，真实数据库用例又暴露 `Invalid credentials`，证明仅传 User ID 不足以落地授权。
2. Green：WorkflowRun 新增受 GORM CHECK 约束的 `initiator_token_version >= 1`，Start、GORM Mapper、Claim、Runtime 和 Executor 完整传递；无默认值或旧行兼容路径。
3. 真实 PostgreSQL 聚焦旅程通过：Start 投影保存启动身份，GORM Claim 将它传给 Executor，Script Application 读取不可变 Revision 并输出正式端口绑定；Revision Version 漂移和 Token Version 撤销的负向用例通过。
4. Backend 完整门禁在全新 PostgreSQL 16.15 空库和固定摘要 Temporal Server 上通过：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`，其中 `backend/tests/workflow` 用时 25.622 秒。
5. Agent 门禁使用 Python 3.11.15 和锁定开发依赖：Ruff 检查/格式、Pyright 无错误，Pytest 12 项通过。
6. Frontend 门禁在官方 `node:22-bookworm-slim` 临时容器的 Node 22.23.2 上从 `npm ci` 开始通过：OpenAPI 生成、ESLint、TypeScript、Vitest 45 项和 Next.js 生产构建均成功。
7. OpenAPI 客户端生成零漂移，Delivery Hygiene 与 `git diff --check` 通过；未跟踪凭据、数据、日志或测试报告。

## Requirement Checklist

- `BE-MOD-005`：Workflow 节点执行身份已成为持久 Run 事实，第一个真实 Script Owner Executor 已落地；其他 Production/Agent Executor 尚未实现。
- `BE-JRN-003`：单节点活动的授权输入已冻结，但生产 Worker 组合根和全部节点并未因本记录而视为完成。

## 残余风险与下一切片

- 仓库尚无可部署的 `workflow-worker` Composition Root；下一切片必须把 Workflow Store、Temporal Worker、Review Opener 和本 Executor 装配成真实进程，并通过 Temporal 真服务的单 Script Workflow。
- 当前 Executor 仅支持 `workflow.input.script_revision`；全图运行到下一个未支持节点时必须明确失败，不允许空结果或伪成功。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地门禁通过不等于远端绿色；未获准推送前，不宣称远端 CI 已修复。
