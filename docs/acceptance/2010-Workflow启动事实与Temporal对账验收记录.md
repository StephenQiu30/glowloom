# Workflow 启动事实与 Temporal 对账验收记录

- 状态：Workflow 启动事实、基础投影与真实 Temporal Starter 切片通过；完整运行时尚未完成
- 日期：2026-08-25
- Design：[系统总体架构](../design/0003-系统总体架构.md)、[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[平台 V1 需求规格](../requirement/0001-平台V1需求规格.md)、[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 确定性编译验收记录](2009-Workflow确定性编译验收记录.md)

## 验收范围

本记录验收 Backend 从不可变 AuthoringRevision 编译后，创建 WorkflowRun、NodeRun Projection、Start Intent/Receipt，并以稳定身份调用真实 Temporal 服务及处理 `Started / AlreadyStarted / Unknown` 的最小闭环。它不宣称公共 Run API、Temporal Worker、Control/Signal、Episode/Shot Workflow、HumanTask、Node Cache、局部重跑、取消/恢复或 Replay 已实现。

Workflow 业务事实继续只由 `backend/internal/workflow` 的 GORM Adapter 写入 PostgreSQL。Temporal History 是工作流 Timer、Retry、Signal 与 Activity 的运行权威，其私有持久化不构成第二套 Backend 业务 SQL 事实源，也不向 Agent 暴露。

## 实现证据

| 契约 | 结果 |
|---|---|
| 启动前编译 | Start 只接受不可变 AuthoringRevision ID，并先调用唯一 Compiler；传入 Draft ID 失败关闭 |
| 稳定身份 | Run、NodeRun、Start Intent 与 Temporal Workflow ID 均由冻结业务输入确定，同幂等键重放复用原身份 |
| 业务投影 | 首次启动在同一事务内准备 WorkflowRun、全部 NodeRun Projection 与 Start Intent；外部调用不占用数据库事务 |
| 结果回写 | 每次外部启动尝试独立记录 Start Receipt，并在事务内更新 Intent 与 Run 投影 |
| 已启动对账 | Temporal Memo 保存 `lanverse_input_hash`；`AlreadyStarted` 通过 Describe 读取并比对，匹配才进入 `RUNNING` |
| 冲突与未知 | 已存在 Workflow 的输入 Hash 不匹配进入 `NEEDS_ATTENTION/start_conflict`；无法确认结果进入 `NEEDS_ATTENTION/start_unknown`，不伪报成功 |
| 未知恢复 | 相同幂等键以原 Run、Workflow ID 和输入 Hash 重试；观察到匹配的 `AlreadyStarted` 后收敛为 `RUNNING` |
| SDK 边界 | 使用官方 `go.temporal.io/sdk`；Application 只依赖 Starter Port，不导入 Temporal SDK 或 GORM |
| 部署边界 | 本地 Compose 使用固定镜像摘要和持久数据卷；生产配置只接受显式外部 Temporal 地址 |

## 真实验证

1. `cd backend && go test ./... && go vet ./...`：通过，包括架构边界、Compiler、Start Application 与配置测试。
2. 临时 PostgreSQL 16.15 容器运行 `TestWorkflowStartPersistsRunNodeProjectionAndReconcilesUnknownOutcome`：通过；产生 4 个 Run、36 个 NodeRun Projection、4 个 Start Intent 和 5 个 Start Receipt，Unknown 使用同一身份恢复。
3. 固定摘要 `temporalio/temporal@sha256:cf86707827fac99e4d1c4a47dc11b105382d796199c7bd41fb3213fb0471628e` 的临时真实 Temporal 服务运行 `TestTemporalStarterPersistsStableWorkflowIdentityAndInputHash`：通过；首次 Start、重复 `AlreadyStarted` 与不匹配请求的实际 Memo 读取均符合契约。
4. 两个真实组件测试以 `go test ./tests/workflow -run '^(TestWorkflowStartPersistsRunNodeProjectionAndReconcilesUnknownOutcome|TestTemporalStarterPersistsStableWorkflowIdentityAndInputHash)$' -count=1 -p 1 -v` 复验：通过；临时容器随后自动删除。
5. 开发与生产 Compose 覆盖均执行 `docker compose ... config --quiet`：通过；生产验证使用隔离占位变量，没有读取或输出真实凭据。

## Requirement 状态

- `PLT-FR-003`：Revision → Compiler → Run 内部链已建立，但公共 Start Run API 和前端入口尚未闭环。
- `BE-MOD-005`：WorkflowRun/NodeRun 基础投影已完成；Node Cache、完整查询投影与局部运行范围未完成，因此主需求保持未完成。
- `BE-JRN-003`：稳定 Temporal Workflow ID、Start Intent/Receipt、`AlreadyStarted / Unknown` 对账已完成；Control/Signal、Episode/Shot Workflow 与 Replay 未完成，因此 Journey 保持未完成。
- `BE-RUN-002`：已证明 Start 外部调用不持有数据库事务；Activity、Signal 和 Callback 的同类边界仍待 Worker 切片验证。

## 残余风险与下一切片

- 当前 Temporal Starter 可以创建并对账 Execution，但尚无注册对应 Workflow Type 的 Worker；不得把 Temporal 中的待处理 Execution 解释为业务执行已完成。
- Composition Root 和公共 API 尚未装配 StartService；下一切片必须先完成可 Replay 的最小 Episode Workflow/Worker，再开放 Start Run 用户入口，避免生产请求进入无人消费的 Task Queue。
- Control/Signal 必须沿用 Intent/Receipt 与 Unknown 对账语义；不得由 HTTP Handler 直接调用 Temporal 或修改 GORM Model。
- 最终 `agent-browser` 验收仍只在全部开发、真实组件旅程和自动化回归完成后执行。
