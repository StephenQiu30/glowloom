# Workflow 人工任务续租与释放验收记录

- 状态：Backend 内部 HumanTask Renew/Release Lease 切片通过；Expire 由后续独立验收记录证明，公共 API 尚未实现
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 人工信号协调验收记录](2011-Workflow人工信号协调验收记录.md)

## 验收范围

本记录验收已领取 HumanTask 的主动续租和释放：`Claim → Renew → Expired Takeover / Release → Reclaim → Decide`。Review 继续唯一拥有 HumanTask、Claim Lease 与 ReviewDecision；本切片不修改 WorkflowRun、Production 或 Temporal History，也不把尚未实现的显式 Expire Command 和公共 HTTP 路由计入完成范围。

HumanTask 继续使用唯一 GORM Model Catalog 中已有的 ClaimedBy、ClaimToken、ClaimExpiresAt 与 Revision；Renew/Release 复用唯一 PostgreSQL、GORM 行锁和通用 Command Receipt。没有新增 Migration、迁移字段、第二 ORM、第二数据库连接、手写 SQL 或 Agent Writer。

## 实现证据

| 契约 | 结果 |
|---|---|
| Owner 门禁 | Renew/Release 重新校验 Workspace Reviewer 权限、User Token Version 和当前 ClaimedBy |
| Fencing | Renew/Release 必须同时匹配 Task ID、Expected Revision、Claim Token 和未过期 Claim；旧 Token 不能操作后来 Claim |
| 续租 | Renew 保留原 Claim Token，只把期限延长到当前时间加配置 Lease，并单调增加 Task Revision |
| 释放 | Release 在事务中把 Task 恢复为 `OPEN`，清除 ClaimedBy、ClaimToken 和 ClaimExpiresAt，并单调增加 Revision |
| 幂等 | Renew/Release 各使用独立 Operation + Workspace Idempotency Key；同输入重放返回原结果且不增加 Revision |
| 输入漂移 | 同一 Idempotency Key 绑定不同 Actor、Token、Task 或 Revision 时返回冲突 |
| 过期接管 | Renew 后仍以最终 ClaimExpiresAt 为门禁；过期后另一 Reviewer 可领取并获得新 Token，旧 Token 决议失败 |
| 单一事实 | HumanTask 更新和 Command Receipt 在同一 PostgreSQL 事务提交或回滚 |

## 真实验证

1. 全新 PostgreSQL 16.15 运行 `LANVERSE_TEST_DATABASE_URL=... go test ./tests/review -run 'TestHumanTask(PersistsClaimTakeoverAndOneDecision|OwnerRenewsAndReleasesClaimIdempotently)$' -count=1 -p 1 -v`：通过，用时 0.970 秒。
2. GORM 旅程完成 Claim A → Renew A → 过期接管 B → Release B → Reclaim B → Decide；Renew/Release 各只有一个 Command Receipt。
3. 同一 Renew/Release 重放不增加 Revision；其他 Reviewer、过期 Token和释放后的旧 Token 均不能修改当前 Lease。
4. 全新 PostgreSQL 16.15 与固定摘要 Temporal 服务执行 `test -z "$(gofmt -l .)"`、`go vet ./...` 和 `go test -p 1 ./...`：全部通过；Review 包用时 3.282 秒，Workflow 真实组件包用时 16.006 秒。
5. Agent Candidate Runtime 执行 Ruff Check/Format、Pyright 与全量 Pytest：通过，`12 passed`；Frontend 执行 OpenAPI 生成、ESLint、TypeScript、`45` 项 Vitest 和生产构建：全部通过。
6. OpenAPI Drift、Delivery Hygiene、Secret/Data/Report、语言边界和 `git diff --check`：全部通过。

## Requirement 状态

- `BE-MOD-006`：本记录证明 HumanTask、Claim/Renew/Release、过期接管、不可变 ReviewDecision 与 Stale 检查；Expire 见[后续验收](2014-Workflow人工任务过期回收验收记录.md)，公共 API 和跨日浏览器旅程仍未完成，因此主需求保持未完成。
- `BE-APP-002`：Renew/Release 的 HumanTask 与 Receipt 原子提交已有真实 PostgreSQL 证据；其他模块仍按各自 Acceptance 验收。
- `BE-APP-007`：Renew/Release 每次重新验证 Reviewer Membership 与 Token Version；完整公共错误等价矩阵仍未完成。

## 残余风险与下一切片

- 本记录验收时尚无独立 Expire 能力；后续已由[人工任务过期回收验收记录](2014-Workflow人工任务过期回收验收记录.md)补充有界 Expire Sweep 证据。
- Renew/Release 尚未暴露公共 API；需与 Workflow 查询、授权错误映射和 OpenAPI 同一切片装配，不能由 Handler 直接访问 GORM Store。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
