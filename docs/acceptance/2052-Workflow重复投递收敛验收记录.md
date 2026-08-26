# Workflow 重复投递收敛验收记录

- 状态：重复 Start、Activity、Decision 与 Signal 的一次业务效果完成门通过；完整 Workflow 与 Guided MVP 尚未完成
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 人工信号协调](2011-Workflow人工信号协调验收记录.md) · [Generation CandidateSet 与 Workflow 人工选择](2047-GenerationCandidateSet与Workflow人工选择验收记录.md)

## 验收范围

本记录只收敛阶段 5 完成门“重复 Start/Activity/Decision/Signal 不产生重复业务效果”的当前实现证据。它复验同一不可变输入在 HTTP/Application、PostgreSQL/GORM 与真实 Temporal 边界上的身份、回执、投影和 History，而不是增加新的幂等层、兼容字段或第二事实源。

本切片不实现尚待评审的 Runware 图片 Provider、Generation Executor、Episode 动态 `ShotWorkflow × N`、Agent 预算/Tool Allowlist、前端 Workflow 页面、Media/Render 或最终成片，也不据此宣称完整 Workflow 已完成。

## 一次业务效果证据

| 重投边界 | 已复验结果 |
|---|---|
| Start | 相同 Authoring Revision 与幂等键返回同一 WorkflowRun、Start Intent 和 Receipt；已完成 Start 不再次调用 Temporal。UNKNOWN 使用原稳定身份对账；真实 Temporal 对相同 Workflow ID/输入返回 `already_started`，输入漂移可见且不会伪报成功。 |
| Activity | NodeRun 首次失败或持久等待后只使用同一冻结 Input、NodeRun 和业务身份重试；节点成功后的重复调用直接返回原 canonical Output，不再次执行 Owner。Production Bible 持久轮询产生多次 Activity Attempt，但只形成一个 AgentInvocation 和一个 Owner CommandReceipt。 |
| Decision | HumanTask 领取与不可变 ReviewDecision 绑定精确 Task/Subject Revision；同幂等输入重放返回同一 Decision，不同终态输入复用幂等键失败关闭；真实 PostgreSQL 中保持一个 Task 和一个 Decision。 |
| Signal | Generation Selection 首次真实送达 Temporal 后注入响应丢失，PostgreSQL 保存 UNKNOWN；重建 SignalService 后按原 Intent/Signal ID/Hash 从 History 对账。随后八路并发重放返回同一完成事实，只存在一个 Selection、Owner Receipt、Apply Receipt 和 Signal Intent。 |
| Replay | Episode Workflow 的 Human Gate 只产生一个 Temporal Signal Event；同身份重放返回 `already_applied`，漂移输入返回 `conflict`，完整 History 由正式 Workflow 注册成功 Replay。 |

这些路径沿用 Backend Application Port、Owner CommandReceipt、Workflow Intent/Receipt 和 GORM Model Catalog；没有 Migration、DDL、Raw SQL、第二 ORM、Redis 幂等锁或 Temporal 之外的第二工作流引擎。

## 当前真实复验

本次在全新任务专用 PostgreSQL `16.15-alpine`、固定摘要 Temporal Server `1.31.2` 与私有 MinIO 上执行。第一次定向命令未注入测试环境变量，五项用例全部明确 `SKIP`，不计为通过；补齐真实服务地址后原样重跑并只记录非跳过结果。

1. Workflow 组合用例通过，共 `15.921s`：
   - `TestWorkflowStartPersistsRunNodeProjectionAndReconcilesUnknownOutcome`：`3.24s`；
   - `TestRuntimePlanWaitsForCommittedStartAndRestoresCompiledOrder`：`3.00s`；
   - `TestProductionWorkflowWorkerDurablyCompletesBibleCandidate`：`8.20s`；
   - `TestGenerationCandidateSetSelectionPersistsThroughWorkflowSignal`：`0.89s`；
   - `TestEpisodeWorkflowCompletesOnRealTemporalAndReplaysHistory`：`0.20s`。
2. 真实 Temporal Starter 用例 `TestTemporalStarterPersistsStableWorkflowIdentityAndInputHash` 通过，用时 `0.06s`。
3. 真实 PostgreSQL Review 用例 `TestHumanTaskPersistsClaimTakeoverAndOneDecision` 通过，用时 `2.83s`。
4. 本任务只收敛既有行为证据，没有修改核心业务逻辑，因此不存在为本任务制造的 Red。原行为的 Red → Green 证据继续由上述各前置 Acceptance 记录；本记录的完成条件是当前事实在全新真实依赖上仍可复现。

## 完整 Required CI

- Backend 在重新创建的空 PostgreSQL、Temporal 与 MinIO 上通过 `gofmt`、`go vet ./...` 和 `go test -count=1 -p 1 ./...`；Workflow 包用时 `105.140s`，所有依赖用例均实际执行而非跳过。
- Agent 使用明确的 Python `3.11.15` 新建临时 venv，锁定依赖安装后通过 Ruff check/format、Pyright `0 errors` 和 Pytest `12 passed`。首次误用系统 Python `3.9.6` 的旧 pip 导致 editable install 失败；没有修改依赖或增加兼容配置，改用 CI 固定版本后原样重跑通过。
- Frontend 在 `node:22-bookworm` 中执行 `npm ci`、OpenAPI Client 生成、ESLint、TypeScript、Vitest `16 files / 45 tests` 和 Next.js `16.2.12` 生产构建，全部通过；生成 Client 无漂移。
- 开发与生产 Compose 配置均可渲染；Frontend、Backend、Agent 三类镜像构建通过，容器内 API/Workflow Worker、Frontend standalone、Codex CLI/Candidate Runtime 断言通过。
- 文档引用、`git diff --check`、OpenAPI 漂移和仓库 Secret/Data/Report/跨语言边界卫生检查通过。

## Requirement 状态

- `BE-MOD-005`：Compiler、Definition/Input Snapshot、Run/Node Projection、Node Cache 与重复 Activity 完成重放已有证据；真实图片 Generation Executor 与 Episode 动态 Shot 扇出仍未完成。
- `BE-MOD-006`：HumanTask、Lease 与不可变 ReviewDecision 的重复输入收敛已有真实 PostgreSQL 证据。
- `BE-JRN-003`：Start/Signal 稳定身份、UNKNOWN 对账、Temporal Signal 去重与 History Replay 已完成本验收门；完整 Episode/Shot 生产闭环仍受真实图片生成前置约束。
- `BE-JRN-006`：Generation Human Gate 的 Owner Receipt、Apply Receipt、Signal Intent 与生产效果在响应丢失和并发重放下保持唯一。

## 残余风险与下一步

- [Runware 图片 Provider 与 Generation 执行器 Design](../design/2051-Runware图片Provider与Generation执行器设计.md)仍待用户接受；接受前不得开始其 PRD、Requirement、Plan 或编码。
- `Agent 超预算、越权 Tool、无效 Schema 和 Runtime 不可用`这一阶段 5 完成门仍未完成，也不能由本记录推导为通过。
- 当前本地分支比 `origin/main` 更新，最新远端 GitHub Actions 是成功状态，但本任务和最近本地提交尚未获准推送；本记录不声称这些本地提交已有远端 CI 结果。
- `agent-browser` 按约定只在所有开发完成后执行，本切片不提前调用。
