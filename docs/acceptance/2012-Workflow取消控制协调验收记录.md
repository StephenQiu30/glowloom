# Workflow 取消控制协调验收记录

- 状态：Backend 内部取消控制、真实 Temporal 对账与 PostgreSQL 投影切片通过；公共 API 与生产 Composition Root 尚未装配
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 人工信号协调验收记录](2011-Workflow人工信号协调验收记录.md)

## 验收范围

本记录验收 `Cancel Command → Control Intent → Temporal Cancel Request → History Reconcile → Control Receipt → Run/Node Projection` 的 Backend 内部协调闭环。它只完成设计中已经明确的取消动作，不把尚未实现的 Pause/Resume、局部重跑、公共 HTTP API 或生产 Worker 装配计入本切片。

PostgreSQL 仍是唯一业务 SQL 事实源，`WorkflowControlIntent` 与 `WorkflowControlReceipt` 只在 Backend 唯一 GORM Model Catalog 定义和同步。仓库没有增加 Migration、迁移字段、手写 SQL、第二 ORM、第二数据库连接或 Agent Writer；Temporal History 只承担跨步骤取消运行事实与结果对账。

## 实现证据

| 契约 | 结果 |
|---|---|
| Revision 门禁 | 首次准备在事务中锁定 WorkflowRun，只接受同 Workspace、可取消状态和精确 Expected Revision |
| 唯一 Intent | 每个 Run + Action + Expected Revision 只有一个 Control Intent；Workspace Idempotency Key 不能绑定漂移输入；该身份同时支持后续多轮 Pause/Resume |
| 稳定身份 | 同一 Intent 派生稳定 ControlID；Temporal `RequestCancelWorkflowExecution.RequestId` 使用该 ID，未知结果重试不换身份 |
| History 对账 | Temporal Cancel Reason 保存 ControlID + Input Hash；相同控制重放返回 `already_applied`，相同 ID 的漂移输入返回 `conflict` |
| 结果未知 | RPC 失败且 History 无法确认时写 `unknown` Receipt，Run 进入 `NEEDS_ATTENTION/cancel_unknown`，下一次用原 Intent 对账 |
| 取消终态 | 只在 Temporal History 确认 Workflow 已取消后将 Intent 置为 `completed`，Run 置为 `CANCELLED` |
| Claim 竞态 | Run、Control Intent、Receipt 与全部非终态 NodeRun 在一个 PostgreSQL 事务收敛；活动 Claim Token 同时清除，迟到节点结果被状态和 Fencing 拒绝 |
| 一次外部事实 | 重复取消只产生一个 Temporal Cancel Requested History Event；每次 Backend 尝试保留独立 Receipt |
| Replay | 真实取消后的完整 Episode Workflow History 使用正式 Workflow 注册进行 Replay，通过确定性门禁 |
| 依赖边界 | Application 只依赖 WorkflowController Port；Temporal API、History 读取和原因解码只存在于 `adapter/temporal` |

## 真实验证

1. 全新 PostgreSQL 16.15 与固定摘要 Temporal 服务执行 `test -z "$(gofmt -l .)"`、`go vet ./...` 和 `LANVERSE_TEST_DATABASE_URL=... LANVERSE_TEST_TEMPORAL_ADDRESS=... go test -p 1 ./...`：全部通过；最终 `backend/tests/workflow` 复验用时 12.319 秒。
2. PostgreSQL 旅程先让 Node 获得 Claim 并阻塞，再注入 `unknown`，最后使用同一 ControlID/Input Hash 返回 `already_applied`：最终只有一个 Intent、两个 Receipt，Run 与全部未完成 NodeRun 均为 `CANCELLED`；取消前 Claim 的迟到成功被拒绝，取消后的新 Executor 调用次数为 0。
3. Temporal 旅程等待真实 Episode Workflow 已进入 Human Gate 后请求取消：首次返回 `applied`，重放返回 `already_applied`，漂移 WorkflowRunID 返回 `conflict`；History 中恰有一个 Cancel Requested Event，终态为 Canceled，正式 Replayer 通过，单项复验用时 0.537 秒。
4. Agent Candidate Runtime 执行 Ruff Check/Format、Pyright 与全量 Pytest：通过，`12 passed`。
5. Frontend 从 Backend OpenAPI 重新生成 Client 后执行 ESLint、TypeScript、Vitest 与生产构建：通过，`16` 个测试文件、`45` 项测试全部成功；生成目录无 Diff。
6. Delivery Hygiene、`git diff --check`、Secret/Data/Report 扫描和 Go/Python 项目边界检查：通过。

## Requirement 状态

- `BE-MOD-005`：取消竞态、History 对账和 Run/Node 取消投影已有真实组件证据；Node Cache、Pause/Resume、单 Shot 局部重跑、跨日恢复和公共查询仍未完成，因此主需求保持未完成。
- `BE-JRN-003`：Start、Signal 和 Cancel 均已有 Intent/Receipt 与真实 Temporal 对账；Pause/Resume、Shot Workflow、公共 Control API 和生产 Composition Root 仍未完成，因此 Journey 保持未完成。
- `BE-APP-004`–`BE-APP-005`：取消外部调用已遵守“先 Intent、后调用、再 Receipt”并区分已知结果与 Unknown；其他远程旅程仍分别验收。

## 残余风险与下一切片

- 当前取消服务只能由 Backend 内部用例调用；在真实 Node Executor 和 Worker Composition Root 就绪前不开放公共 Control API，避免用户控制不存在或无人消费的运行。
- Cancel Requested 但 Temporal 尚未进入 Canceled 时，Backend 使用 `PAUSED/cancel_requested` 并要求继续 Reconcile；本次真实旅程直接收敛到 Canceled，尚需在长 Activity 故障注入中覆盖该中间态。
- Pause/Resume Control Signal 与 Workflow 内部暂停门禁已在[独立验收记录](2015-Workflow暂停与恢复控制协调验收记录.md)中完成；下一切片进入 Shot Workflow 与恢复。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
