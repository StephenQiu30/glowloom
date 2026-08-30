# Generation 高成本准备与执行授权验收记录

- 状态：实现、真实目标旅程与完整本地 CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)

## 验收范围

本记录验收 `BE-MOD-007`/`BE-JRN-004` 的高成本执行前置边界：Generation 从真实 Workflow Runtime 冻结事实创建唯一 Intent，在同一 PostgreSQL/GORM 事务内调用 Cost/Quota Owner 完成费用与日配额预留，并提供互斥的 Claim/Cancel 与短时执行授权。

本增量不执行 Provider，不创建 Provider Binding/Job/Callback、CandidateSet、Usage Receipt、Outbox、Settle/Consume 或 Reconcile，也未接入公开 REST 和 Workflow 图片节点，因此不代表 `BE-MOD-007`、`BE-JRN-004` 或阶段 6 完成。

## 实现事实

1. `PrepareImageGeneration` 首次或新幂等键调用必须从 `wrk_runs`/`wrk_node_run_projections` 校验 Workspace/Project、Run/Node 归属、Run 发起人与 Token Version、执行态、`external_ai` 风险等级、Runtime Claim 和 canonical `node-input-canonical` Hash；孤立 UUID、非执行态或 Input 漂移均失败关闭。
2. 每个 Workflow NodeRun 只有一个 `GenerationIntent`。Intent 冻结 Input Hash、Units、发起身份、Cost Estimate/Reservation、Quota Reservation、Owner Receipt 和 Content Hash；相同来源并发或新幂等键只复用 Owner 事实，绑定漂移不会建第二份占用。
3. Generation Coordinator 开启一个外层 GORM 事务，将同一 `*gorm.DB` 绑定给现有 Cost/Quota Application Service；Owner 仍执行自身授权、锁、账本/计数器、状态机和 Command Receipt。GORM 嵌套 Savepoint 被关闭时直接拒绝，不退化为多事务补偿或跨 Owner 写表。
4. `PREPARING` 只用于未提交事务内的 NodeRun 并发串行化；成功后一次性提交 `PREPARED`、Estimate、Cost Reservation/Ledger、Quota Reservation/Counter 与全部 Receipt。Cost、Quota 或 Generation 任一失败时整体回滚，数据库不保留 `PREPARING` 事实。
5. `AcquireExecutionClaim` 与 `CancelPreparedIntent` 锁定同一 Intent。Claim 只从 `PREPARED` 进入 `CLAIMED`，固化 Claimant、不可预测 Token、Fencing Version 和到期时间；Cancel 只从 `PREPARED` 进入 `CANCELLED`，并在同一外层事务中调用 Cost/Quota Owner 释放。两者并发只能一个成功，已 Claim 事实不会被假定为未执行而释放。
6. `ExecutionAuthorization` 从持久 Claim 派生，绑定 Intent/Claim Token、Fencing、Revision、Input Hash、Units、Cost/Quota Reservation 和 Expires At。每次验证重新检查发起人授权与双 Reservation 仍为 Reserved；到期、撤权或任一事实漂移均失败。
7. `gen_intents` 进入唯一 GORM Model Catalog。NodeRun 唯一索引、Claim Token 唯一索引、Workflow/Cost/Quota/Receipt 外键以及精确状态/Revision/Owner 引用 CHECK 已由真实 PostgreSQL 创建；没有 Migration 文件、DDL/Raw SQL、第二数据库、Redis 正确性或 Agent 业务事实。

## 真实验收证据

- Red 阶段先执行 `go test ./tests/generation`，明确因 `NewPreparationService`、`NewPreparationStore`、Intent 状态与 Command 缺失而编译失败；完成最小实现后同一旅程转绿。
- 真实 PostgreSQL `16.15-alpine` 下，目标旅程单次通过，`-count=3` 连续三次通过（累计 8.198 秒）；`go test -race -count=1` 在修正测试共享错误变量后 5.606 秒通过。
- Quota 上限为 1、准备 Units 为 2 时，Intent、Estimate、Cost Reservation/Ledger、Quota Reservation 与四类 Receipt 计数全部为 0；8 路相同准备并发时收敛为唯一 Intent/Owner 事实和同一 Receipt。
- 伪造 NodeRun、Input Hash/Units 漂移、禁用 GORM Savepoint 全部被拒绝；重新计算 Workflow canonical Input 与数据库持久 Hash 一致后才能占用费用/配额。
- Cancel 同时将 Cost/Quota Reservation 转为 Released 且可幂等重放；Claim 产生 Fence=1 的稳定授权，其他 Claimant 不能覆盖，Claimed Intent 不能 Cancel。Claim/Cancel 竞态恰有一个成功。
- 发起人 Token Version 撤销、TTL 到期以及外部注入 Cost Released 事实后，授权验证均失败关闭。旅程结束时已提交 `PREPARING` 计数为 0。
- PostgreSQL Catalog 查询和自动验收确认 `ck_gen_intent_state`、`ck_gen_intent_claim_fields`、NodeRun/Claim 唯一索引及 Workflow/Owner/Receipt 外键实际存在。初次 Schema 审阅发现同一 GORM tag 的多个 CHECK 只生成最后一个，后将表达式合并为可实际生成的状态 CHECK，并把存在性纳入测试。
- Generation 旅程初次污染 Workflow 包的全局计数，精确清理测试自建的 Intent/Node/Run/Snapshot/Definition 后，旅程结束计数为 `0|0|0|0|0`，共享 CI 数据库的 Workflow 套件恢复全绿。
- 全新 PostgreSQL + Temporal + MinIO 下执行 Backend `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全绿；Generation 套件 8.674 秒，Workflow 真实集成套件 78.880 秒，未跳过外部服务旅程。
- Agent 使用真实 Python 3.11 临时虚拟环境执行依赖安装、Ruff check/format、Pyright 与 12 个 Pytest 全绿；Frontend 使用 Node 22.23.2 执行 `npm ci`、OpenAPI 重生成、ESLint、TypeScript、45 个 Vitest 与 Next.js 生产构建全绿，生成客户端无漂移。
- 首次 Frontend `npm ci` 和 Backend 镜像构建分别因网络 `ECONNRESET`/Go Proxy `unexpected EOF` 未通过；在不改依赖、不跳过检查的前提下重试成功。开发/生产 Compose、仓库卫生、三类部署镜像及 API、Workflow Worker、Frontend standalone、Agent Candidate Runtime 入口全部通过。

## 边界与残余风险

- 当前 Preparation/Claim 只是 Backend 内部 Application Interface，尚未接入 Workflow 图片 Node Executor；必须等 Provider Job/Request Key/Receipt 契约完成后才能认定真实外部生成已受联合门禁。
- Claim 后的 Provider 结果仍可能未知；当前正确行为是保留 Cost/Quota Reserved 并等待后续 Receipt/Reconcile，不允许超时自动释放或自动重新 Claim。
- CandidateSet、单 Shot Workflow 和手动选择与本切片已有 Candidate/QC/Selection 事实的串联尚未实现，完整 `BE-JRN-004` 仍未验收。
- 远端 `origin/main` 当前为已通过 GitHub Actions 的 `ec2bb71`；本地提交只有在获得推送并实际运行后才能报告远端绿色。
- `agent-browser` 按约定只在全部开发完成后执行；当前仍有 Provider、CandidateSet/Workflow 和单 Shot Workflow 后续任务，本切片不提前调用。
