# Workflow 人工任务过期回收验收记录

- 状态：Backend 内部 HumanTask Expire Sweep 切片通过；生产调度与公共查询尚未装配
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 人工任务续租与释放验收记录](2013-Workflow人工任务续租与释放验收记录.md)

## 验收范围

本记录验收 Backend 内部有界 `ExpireClaims`：扫描已经超过 ClaimExpiresAt 的 `CLAIMED` HumanTask，将其恢复为 `OPEN` 并清除 Lease 身份，使任务可被后续 Reviewer 重新领取。它不创建第二个任务、不替代已有过期 Claim 原子接管，也不把生产定时调度、公共 Query 或浏览器旅程计入本切片。

Expire 复用唯一 GORM HumanTask 模型与 PostgreSQL 事务，没有增加状态字段、Migration、迁移元数据、第二 ORM、第二数据库连接、手写 SQL 或 Agent Writer。该操作不调用外部服务；重复执行依靠到期状态谓词、行锁和 Revision 收敛，不建立远程结果 Intent/Receipt。

## 实现证据

| 契约 | 结果 |
|---|---|
| 有界批次 | Application 只接受 `1..500` 的 Limit，防止一次事务无界锁定任务 |
| 到期谓词 | 只选择 `CLAIMED`、ClaimExpiresAt 非空且小于等于当前时间的任务；有效 Claim 保持不变 |
| 并发领取 | GORM 使用 `FOR UPDATE SKIP LOCKED` 和稳定的到期时间/ID 顺序，多个 Sweep 不等待或重复处理同一行 |
| Fencing | 每个被锁定任务仍使用当前 Revision 更新；状态、Owner、Token、Expiry 和 Revision 在同一事务收敛 |
| 回收终态 | 到期任务恢复为 `OPEN`，清除 ClaimedBy、ClaimToken 和 ClaimExpiresAt，Revision 单调增加一次 |
| 幂等重放 | 首次 Sweep 返回实际回收数；相同数据库状态再次执行返回 0，不重复增加 Revision |
| Claim 竞态 | Sweep 与新 Claim 共享 HumanTask 行锁；先完成的一方递增 Revision，另一方必须重新读取后重试 |

## 真实验证

1. 全新 PostgreSQL 16.15 运行 `LANVERSE_TEST_DATABASE_URL=... go test ./tests/review -run 'TestHumanTask(PersistsClaimTakeoverAndOneDecision|ExpireSweepReopensExpiredClaimOnce)$' -count=1 -p 1 -v`：通过，用时 0.904 秒。
2. 同库同时存在一个过期 Claim 和一个有效 Claim；两个并发 Sweep 的回收计数总和严格为 1，只将过期任务恢复为 `OPEN`，有效任务的 Status、Token 与 Revision 不变。
3. 第二次 Sweep 返回 0；单元测试同时覆盖未到期 Sweep 返回 0 和过期后 Revision 只增加一次。
4. 全新 PostgreSQL 16.15 与固定摘要 Temporal 服务执行 `test -z "$(gofmt -l .)"`、`go vet ./...` 和 `go test -p 1 ./...`：全部通过；Review 包用时 2.585 秒，Workflow 真实组件包用时 15.958 秒。
5. Agent Candidate Runtime 执行 Ruff Check/Format、Pyright 与全量 Pytest：通过，`12 passed`；Frontend 执行 OpenAPI 生成、ESLint、TypeScript、`45` 项 Vitest 和生产构建：全部通过。
6. OpenAPI Drift、Delivery Hygiene、Secret/Data/Report、语言边界和 `git diff --check`：全部通过。

## Requirement 状态

- `BE-MOD-006`：HumanTask Claim/Renew/Release/Expire、过期接管、不可变 Decision 与 Stale 门禁均已有内部应用和真实 PostgreSQL 证据；公共 API、跨日运行与浏览器验收仍未完成，因此主需求保持未完成。
- `BE-APP-002`：Expire 对 HumanTask 的状态、Lease 身份与 Revision 在一个 PostgreSQL 事务更新；没有跨进程部分提交。

## 残余风险与下一切片

- `ExpireClaims` 尚未由生产 Worker 定时调用；Composition Root 装配时必须使用有界间隔和批次，不新增常驻 Binary 或第二调度系统。
- 维护 Sweep 尚未写入 Audit Event；公共运维可见性随 Workflow Composition/Observability 切片实现，不在本任务预建新事件框架。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
