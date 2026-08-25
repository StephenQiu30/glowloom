# Workflow 持久任务恢复验收记录

- 状态：持久任务恢复切片通过；完整 Workflow 尚未完成
- 日期：2026-08-25
- Design：[后端服务架构](../design/2001-后端服务架构.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端运行架构需求规格](../requirement/2001-后端运行架构需求规格.md)
- Plan：[后端运行架构实施计划](../plan/2001-后端运行架构实施计划.md)

## 验收范围

本记录只验收目标 Workflow 设计的第一个可靠执行切片：当前 Production Bible 与 Storyboard Agent Invocation 的有界 Lease、过期回收和 Claim Fencing。它不把轻量 Worker 计为完整 Temporal Workflow，也不宣称 Compiler、Definition、Run/Node、HumanTask 或 Signal 已实现。

数据仍只有一个事实源：`AgentInvocation` 的 `claim_version` 与 `lease_expires_at` 由 `backend/internal/platform/database/model` 中唯一 GORM Catalog 定义和同步。仓库没有增加 Migration 文件、迁移账本、第二数据库 DSN、第二 ORM 或 Agent 数据库连接。

## 实现证据

| 契约 | 结果 |
|---|---|
| 有界配置 | `AGENT_CLAIM_LEASE_SECONDS` 默认 1800 秒，可通过 Backend 配置覆盖，非正整数 fail closed |
| 原子领取 | PostgreSQL 行锁与 `SKIP LOCKED` 领取 `queued` 或 Lease 已过期的 `running` Invocation |
| 稳定身份 | 过期回收复用原 Invocation ID，不创建第二业务请求 |
| Fencing | 每次领取原子递增 `claim_version`；完成/失败必须同时匹配当前 Version 且 Lease 未过期 |
| 迟到结果 | 旧 Claim 的完成结果返回未应用，不修改当前 Invocation、业务聚合或 WorkflowTask |
| 正常终态 | 当前 Claim 成功后写入 `succeeded` 并清除 Lease；原有 Candidate → 人工 Apply 门禁保持不变 |
| Resume 栅栏 | Production Bible 从失败态 Resume 时保留原 Claim Version；下一次领取只递增不重置 |
| 进程重启 | Worker 在 Agent HTTP 调用中被强杀后，新进程等待 Lease 过期并以同一 Invocation ID 恢复 |

## 真实验证

1. `cd backend && go test ./... && go vet ./...`：通过。
2. 独立临时 PostgreSQL 运行 `go test ./internal/production/storyboard/adapter/gormdb ./tests/workflow -count=1 -p 1 -v`：通过。
3. `TestPostgreSQLStoryboardingJourney`：通过，证明正常 Storyboard Candidate、审核、Apply 与导出链未回归。
4. `TestExpiredInvocationIsReclaimedAndStaleResultIsFenced`：通过，证明未过期不能重复领取、过期后相同 ID 从 Claim 1 递增到 Claim 2、Claim 1 迟到结果被拒绝、Claim 2 成功提交并清除 Lease。
5. `TestBibleResumePreservesClaimFenceAndRejectsLateResult`：通过，证明 Resume 保留 Claim 7、重新领取递增到 Claim 8、Claim 7 的迟到结果被拒绝且 Claim 8 可正常提交。
6. `TestBibleWorkerReclaimsInvocationAfterProcessRestart`：通过；真实 OS 子进程使用现有 Bible Worker、HTTP Agent Client 与 PostgreSQL。第一个进程在 Agent 调用期间被强杀，第二个进程在测试用 500ms Lease 过期后复用同一 Invocation ID，将 Claim 8 递增到 Claim 9 并只提交一个 Candidate。

## Requirement 状态

- [x] `BE-RUN-004`：Repository 持久 Lease、过期 `running` 回收、迟到结果 Fencing 与真实 Worker 进程重启均已通过。
- [x] `BE-DAT-006`：原子 Claim Version、Lease、完成 Fencing 与 Bible Resume 后旧结果负向测试均已通过。

## 残余风险与下一切片

- 当前单进程 Worker 不续租；默认 30 分钟 Lease 必须长于正常单次 Agent 调用。调用超过 Lease 时结果会被安全拒绝，但会产生一次无效外部执行。
- 当前证据不要求增加 Heartbeat：尚无横向扩容、单次 Agent 调用超过 30 分钟或数小时分阶段执行的真实数据；出现这些事实后再按设计引入续租。
- 下一切片进入 Workflow Compiler 的真实前置，先固定最小 Authoring Revision 与 Node Catalog，不预建空 Temporal 层或第二执行引擎。
- Temporal、Workflow Compiler、Definition Version、Run/Node Projection、HumanTask、Signal Receipt、局部重跑和 Node Cache 仍由目标阶段计划控制，均未实现。
