# Workflow 暂停与恢复控制协调验收记录

- 状态：Backend 内部多轮 Pause/Resume、真实 Temporal 安全边界暂停与 PostgreSQL 投影切片通过；公共 API 与生产 Composition Root 尚未装配
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 取消控制协调验收记录](2012-Workflow取消控制协调验收记录.md)

## 验收范围

本记录验收 `Pause/Resume Command → Control Intent → Temporal Control Signal → History Reconcile → Control Receipt → Run Projection → Workflow Safe Boundary` 的 Backend 内部闭环。Pause 允许当前 Activity 完成，但在下一个节点、Human Gate 或 Run Completion 前停止；Resume 使用另一条可对账控制信号继续原 Run，不启动第二个 Temporal Workflow。

本切片复用既有 Control Intent/Receipt 和 WorkflowController Port，不建立 Pause 表、Resume 表或第二套控制服务。PostgreSQL/GORM 仍是唯一业务 SQL 事实源；Temporal History 仍是跨步骤信号和执行顺序权威。仓库没有增加 Migration、迁移字段、手写 SQL、第二 ORM、第二数据库连接或 Agent Writer。

## 实现证据

| 契约 | 结果 |
|---|---|
| 多轮控制身份 | Intent 唯一键和稳定 ID 同时包含 Run、Action 与 Expected Revision；Pause→Resume→Pause 产生三组独立身份，同一命令重放仍返回原 Intent |
| 状态门禁 | Pause 只接受 `RUNNING/RETRYING`，Resume 只接受带完整暂停来源的 `PAUSED`；终态、陈旧 Revision 和并行未决 Control 均拒绝 |
| 暂停来源 | Run 保存 Pause 前 Status 与 ProgressStage；Pause Unknown 后仍保留来源，Resume Applied 后原子恢复并清空暂停来源 |
| 稳定 Temporal Signal | Pause/Resume 使用同一 ControlID 作为 Temporal Request ID，Payload 固定 Run、Action 与 Input Hash；History 重放同 ID 异参返回 Conflict |
| 安全边界 | Episode Workflow 在每个 Node、Human Gate 和 Run Completion 前消费控制信号；活动中的节点可以完成，后续节点在 Resume 前不会启动 |
| 数据库控制栅栏 | Pending/Unknown Pause 或 Cancel 与 Node Claim、Human Gate 和 Run Completion 共用 Run 行锁域；活动节点完成只更新自身，不覆盖 PAUSED/NEEDS_ATTENTION Run |
| 结果未知 | Pause/Resume RPC 结果未知时写 Unknown Receipt 与可执行 Reconcile Action；原 ControlID/Input Hash 对账成功后收敛，不创建新身份 |
| 一次外部事实 | 同一 Pause 或 Resume 重放不重复发送；真实 History 中恰有一条 Pause 和一条 Resume 控制信号 |
| Replay | 真实 Pause→Resume 完整 Episode Workflow History 使用正式 Workflow 注册回放，通过确定性门禁 |
| 依赖边界 | Application 只依赖 WorkflowController；GORM 行锁只在 `adapter/gormdb`，Temporal Signal/History 只在 `adapter/temporal` |

## 真实验证

1. 全新 PostgreSQL 16.15 与固定摘要 Temporal 服务执行 `test -z "$(gofmt -l .)"`、`go vet ./...` 和 `LANVERSE_TEST_DATABASE_URL=... LANVERSE_TEST_TEMPORAL_ADDRESS=... go test -count=1 -p 1 ./...`：全部通过；最终 `backend/tests/workflow` 用时 14.620 秒。
2. `TestPauseResumeControlPersistsRepeatedCyclesAndFencesTheNextNode`：通过。第一节点带 Claim 执行时 Pause 先进入 Unknown，再用原身份完成对账；该节点完成后 Run 仍为 PAUSED，下一节点 Executor 调用数为 0；Resume 恢复原状态，第二轮 Pause 使用新 Intent/ControlID；最终三个 Intent、四个逐次尝试 Receipt 与四次对账调用一一对应。
3. `TestTemporalPauseResumeControlStopsAtTheNextNodeBoundaryAndReplays`：通过。真实 Temporal 在第一节点活动期间接收 Pause，活动完成后 500ms 内第二节点未启动；Resume 后继续并成功终止；History 只有两条控制信号，正式 Replayer 通过。
4. `TestPauseResumeControlReconcilesUnknownWithoutLosingThePausedSource`：通过。Pause 和 Resume 各自先 Unknown 再使用原身份对账，暂停来源始终完整，最终恢复 RUNNING。
5. Agent Candidate Runtime 执行 Ruff Check/Format、Pyright 与全量 Pytest；Frontend 执行 OpenAPI 生成、ESLint、TypeScript、Vitest 与生产构建；OpenAPI Drift、Delivery Hygiene 和 `git diff --check` 均按 Required CI 执行并通过。

## Requirement 状态

- `BE-MOD-005`：Pause/Resume、Run 投影、并发栅栏和失败恢复已有真实组件证据；Node Cache、单 Shot 局部重跑和公共查询仍未完成，因此主需求保持未完成。
- `BE-JRN-003`：Start、Human Gate Signal、Cancel、Pause 与 Resume 均已有 Intent/Receipt 和真实 Temporal 对账；Shot Workflow、公共 Control API 与生产 Composition Root 仍未完成，因此 Journey 保持未完成。
- `BE-APP-004`–`BE-APP-005`：Pause/Resume 已遵守“先 Intent、后 Signal、再 Receipt”，并区分 Applied、Already Applied、Unknown 与 Conflict；其他远程旅程仍分别验收。

## 残余风险与下一切片

- Pause 是安全边界暂停，不抢占已经运行的 Activity；长 Activity 仍需依靠其自身 Heartbeat/取消契约。该语义已由真实 Activity 阻塞测试固定，不能在 UI 中描述为立即终止。
- 当前控制服务只能由 Backend 内部用例调用；在公共查询与生产 Composition Root 装配前不开放公共 Control API，避免用户控制不存在或无人消费的运行。
- Worker 跨进程恢复已由后续验收记录固定；下一切片先细化并实现 Shot Workflow，再进入 Node Cache、输出绑定和单 Shot 局部重跑。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
