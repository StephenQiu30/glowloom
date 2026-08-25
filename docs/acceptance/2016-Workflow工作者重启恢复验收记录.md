# Workflow 工作者重启恢复验收记录

- 状态：正式 Temporal Worker 跨进程重启与 Human Wait 恢复切片通过；生产 Composition Root 尚未装配
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 暂停与恢复控制协调验收记录](2015-Workflow暂停与恢复控制协调验收记录.md)

## 验收范围

本记录验收 `Episode Workflow → Human Gate Wait → Worker 进程强制终止 → 零 Worker 期间 Signal → 全新 Worker 进程 → Temporal History Replay → 剩余节点完成` 的恢复闭环。进程恢复直接复用正式 `NewWorker` 注册入口和官方 Temporal Go SDK；不新建内存队列、恢复表、Checkpoint 服务或第二套 Workflow 引擎。

本切片不装配生产启动入口。当前 `cmd/api` 尚无可执行生产节点的 `NodeExecutor` 适配器；在该依赖完成前强行启动 Workflow Worker，会形成“进程在线但节点必然失败”的伪落地。PostgreSQL/GORM 仍是唯一业务 SQL 事实源，Temporal History 仍是跨步骤执行与信号权威；仓库没有增加 Migration、迁移字段、手写 SQL、第二 ORM 或 Agent Writer。

## 实现与验证证据

| 契约 | 结果 |
|---|---|
| 真实进程边界 | 验收测试通过当前 Go 测试二进制启动独立 Worker 子进程，不使用同进程 `Worker.Stop/Start` 模拟重启 |
| 强制中断 | 第一 Worker 在 `prepare` Activity 和 Human Gate Open 已被 Temporal 记录完成后由父进程强制终止 |
| 无 Worker 等待 | 第一进程完全退出、第二进程尚未启动时，Human Gate Signal 仍以稳定 Request ID 写入 Temporal History；首轮可直接 Signaled，也可在短对账超时后返回 Unknown，二者均以原身份收敛为 Already Applied |
| History 恢复 | 第二个全新进程只凭相同 Workflow ID、Task Queue、代码和 Temporal History 恢复等待状态，应用 Decision 后继续 `export` 和 Run Completion |
| 一次 Activity 派发 | History 中 Load、prepare、Open、Apply、export 和 Complete 各只有一次 Start 与 Complete；进程重启没有重新派发已完成 Activity |
| Replay | 完整真实 History 使用正式 Episode Workflow 注册回放并通过确定性门禁，History 中只有一条 Human Gate Signal |
| 成熟组件复用 | 恢复能力由 Temporal 持久 History、Task Queue 和 SDK Worker 提供，Backend 不复制调度、持久等待或 Checkpoint 机制 |
| CI 门禁 | 新剧本读取 `LANVERSE_TEST_TEMPORAL_ADDRESS`，因此进入 Backend Required CI 的固定摘要真实 Temporal 路径，不会在 CI 中被跳过 |

## 真实验证

1. 固定摘要 `temporalio/temporal@sha256:cf86707827fac99e4d1c4a47dc11b105382d796199c7bd41fb3213fb0471628e` 的真实 Temporal 服务执行 `LANVERSE_TEST_TEMPORAL_ADDRESS=... go test ./tests/workflow -run '^TestTemporalWorkerRecoversHumanWaitAfterCrossProcessRestart$' -count=3 -v -timeout=90s`：连续三次通过，用时 16.409 秒；每次均在第一 Worker 完全退出后发送信号，再启动第二 Worker，最终 History 只有一条信号且六类 Activity 各只有一次 Start/Complete。
2. 全新 PostgreSQL 16.15 与同一固定摘要 Temporal 服务执行 `test -z "$(gofmt -l .)"`、`go vet ./...` 和 `LANVERSE_TEST_DATABASE_URL=... LANVERSE_TEST_TEMPORAL_ADDRESS=... go test -count=1 -p 1 ./...`：全部通过；`backend/tests/workflow` 用时 20.536 秒，真实外部依赖测试未跳过。
3. Agent Candidate Runtime 使用 Python 3.11.15 与锁定 dev 依赖执行 Ruff Check/Format、Pyright 和全量 Pytest：全部通过，12 个测试通过；本机旧虚拟环境的 Pyright/Pytest 入口曾指向已删除路径，按锁定版本重装工具入口后真实检查通过，未修改项目代码或降低门禁。
4. Frontend 执行 OpenAPI 生成、ESLint、TypeScript、45 个 Vitest 和 Next.js 生产构建：全部通过；`git diff --exit-code -- frontend/src/api`、Delivery Hygiene 与 `git diff --check` 通过。

## Requirement 状态

- `BE-MOD-005`：Worker 重启与 Human Wait 无进程占用已有真实跨进程证据；Node Cache、Shot Workflow、单 Shot 局部重跑和公共查询仍未完成，因此主需求保持未完成。
- `BE-JRN-003`：Episode Workflow 的 Worker 注册、真实 History 恢复、人工信号和 Replay 已通过；Shot Workflow、公共 Control API 与生产 Composition Root 仍未完成，因此 Journey 保持未完成。
- 阶段 5 完成门：仅勾选“Worker 重启后 Run 可恢复，Human Wait 无需占用 Worker 进程”；单 Shot 局部重跑仍独立保持未完成。

## 残余风险与下一切片

- 当前验证证明 Worker 进程可缺席并恢复等待，不以实际等待一整天延长 CI；跨日耐久性来自同一持久 History 机制，仍需生产 Temporal 的 Retention、可用性、备份和告警配置支撑。
- 当前生产 Composition Root 不能启动 Workflow Worker：正式 `RuntimeService` 需要真实 `NodeExecutor` 与 HumanTask Opener。后续必须先完成这些依赖和启动/关闭/健康检查，再宣称生产 Worker 可用。
- Shot Workflow 目前只有目标名称，尚无被 Design 正式固定的 Shot 输入快照、NodeRun 绑定和 Episode 父子边界；下一切片先补齐可审阅 Design，再按 Red → Green → Refactor 实现，避免空壳 Workflow。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
