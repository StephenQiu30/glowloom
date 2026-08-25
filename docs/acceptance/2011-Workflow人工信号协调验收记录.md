# Workflow 人工信号协调验收记录

- 状态：内部 Human Gate 决议、Apply Receipt 与真实 Temporal Signal 协调切片通过；公共 API 与生产 Worker 尚未装配
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[平台 V1 需求规格](../requirement/0001-平台V1需求规格.md)、[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 启动事实与 Temporal 对账验收记录](2010-Workflow启动事实与Temporal对账验收记录.md)

## 验收范围

本记录验收 `ReviewDecision → Human Gate Apply Receipt → Signal Intent/Receipt → Temporal Signal` 的 Backend 内部协调闭环。Review 只拥有 HumanTask、Claim Lease 与不可变决议；Workflow 负责校验等待中的 Run/Node Revision、写入应用回执和信号事实，并通过官方 Temporal Go SDK 发送一次幂等信号。

PostgreSQL 业务事实仍只有 Backend 的 GORM Model Catalog 一个来源；本切片没有 Migration 文件、迁移字段、手写 SQL、第二 ORM 或 Agent 数据库连接。Temporal History 只承担跨步骤运行事实与 Signal 去重/对账，不成为第二个 Backend 业务数据库。

## 实现证据

| 契约 | 结果 |
|---|---|
| 不可变决议 | HumanTask 领取使用 Claim Token 与到期时间；决议绑定精确 Subject Revision，重复决议返回同一结果 |
| 应用前置 | Signal 准备事务锁定 WorkflowRun 与 NodeRun，只接受同 Workspace、同 Run 且 Revision 匹配的 `WAITING_HUMAN` 节点 |
| 唯一事实 | 每个 ReviewDecision 只有一个 Human Gate Apply Receipt；每个 Workspace Idempotency Key 只有一个 Signal Intent |
| 稳定信号 | `SignalID` 同时写入 PostgreSQL、Temporal `RequestId` 和 Signal Payload，重复调用不产生第二个 History Signal Event |
| 输入对账 | Signal Payload 还原后重新计算完整输入 Hash；相同 SignalID 的匹配输入返回 `already_applied`，漂移输入返回 `conflict` |
| 结果未知 | 外部调用失败且 History 无法确认时写 `unknown` Receipt；后续使用原 Intent、SignalID 与输入 Hash 重试 |
| 投影生效 | Apply Activity 只接受已完成且身份、Decision、Subject Revision 全匹配的 Signal Intent；批准后 Node 进入 `SUCCEEDED`、Run 恢复 `RUNNING`，重复 Apply 不再修改 Revision |
| 运行契约 | Episode Workflow 只接受带 SignalID/SignalIntentID 的合法 Human Gate Signal，并在 Apply Activity 成功后决定继续或终止 |
| Worker 注册 | 同一个 Temporal Client 显式注册 Episode Workflow 和 Load/Execute/Open/Apply/Complete 五个 Runtime Activity；注册入口只接受完整 RuntimeActivities 契约 |
| 依赖边界 | Application 只依赖 WorkflowSignaler Port；Temporal SDK 与 History 解码全部位于 `adapter/temporal` |

## 真实验证

1. 固定摘要 Temporal 服务运行 `LANVERSE_TEST_TEMPORAL_ADDRESS=127.0.0.1:57244 go test ./tests/workflow -run 'Test(TemporalStarter|EpisodeWorkflowCompletes)' -count=1 -v`：通过。首次发送返回 `signaled`，同一输入重放返回 `already_applied`，相同 SignalID 的漂移输入返回 `conflict`；Episode Workflow 完成后 History Replay 通过。
2. 全新 PostgreSQL 16.15 与固定摘要 Temporal 服务同时运行 `go vet ./...`、`go test -p 1 ./...`：通过；`backend/tests/workflow` 包含真实 GORM Signal、Apply 投影、正式 Worker 注册、真实 Temporal Start/Signal 与 Replay，最新复验用时 10.065 秒。
3. Agent 执行 Ruff Check/Format、Pyright 与全量 Pytest：通过，`12 passed`；Agent 继续只运行 Candidate Runtime。
4. Frontend 从 `backend/api/openapi/lanverse-v1.json` 重新生成 Client 后执行 ESLint、TypeScript、Vitest 与生产构建：通过，`16` 个测试文件、`45` 项测试全部成功。
5. OpenAPI 生成目录无 Diff，仓库 Secret/Data/Report 与语言边界卫生检查通过。
6. 全新 PostgreSQL 16.15 运行 `LANVERSE_TEST_DATABASE_URL=... go test ./tests/workflow -run TestRuntimePlanWaitsForCommittedStartAndRestoresCompiledOrder -count=1 -v`：通过；漂移 Decision 被拒绝，批准投影落库，重复 Apply 保持同一终态。
7. 固定摘要 Temporal 服务运行 `LANVERSE_TEST_TEMPORAL_ADDRESS=127.0.0.1:57245 go test ./tests/workflow -run TestEpisodeWorkflowCompletesOnRealTemporalAndReplaysHistory -count=1 -v`：通过；正式 Worker 注册入口消费五类 Activity，Workflow 完成并 Replay 成功。

## Requirement 状态

- `PLT-FR-006`：领取、决议、Signal 的幂等与 Stale/Unknown 恢复已由应用层和真实组件验证；公共 HTTP 入口及浏览器旅程尚未闭环，因此主需求保持未完成。
- `BE-MOD-005`：Episode Workflow、Human Wait、Signal 与 Replay 已建立；Node Cache、Control、Shot Workflow、局部重跑和取消/恢复尚未完成。
- `BE-MOD-006`：HumanTask、Claim、过期接管、ReviewDecision 与 Stale 检查已建立；Renew/Release/显式 Expire 和公共 API 尚未完成。
- `BE-JRN-006`：内部 Apply Receipt 与一次 Temporal Signal 已完成；实际 Worker Composition Root 和面向用户的决议入口尚未装配，因此 Journey 保持未完成。

## 残余风险与下一切片

- `cmd/api` 尚未调用 Worker 注册入口，也未装配真实 Node Executor、Start/Review/Signal Handler；当前能力只能由内部用例调用，不能报告为产品服务可用。
- 拒绝/修改请求后的真实 Run 恢复旅程和崩溃重放仍需随 Worker 装配实现；当前已验证批准终态与漂移 Decision 拒绝。
- 下一任务先实现单一 Temporal Client 驱动的 Worker Composition 与真实 Activity 注册，再开放 Workflow/Review HTTP API；不得先暴露会进入无人消费 Task Queue 的 Start 接口。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
