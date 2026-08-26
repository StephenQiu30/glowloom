# Generation Provider 提交与结果对账验收记录

- 状态：实现、真实目标旅程与完整本地 Required CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Generation 高成本准备与执行授权](2044-Generation高成本准备与执行授权验收记录.md)

## 验收范围

本记录验收 `BE-MOD-007`/`BE-JRN-004` 的 Provider 持久协调边界：Project 图片 Provider Binding 追加版本、已 Claim Intent 冻结唯一 GenerationRequest/ProviderJob/Request Key、事务外 Submit/Query、accepted/succeeded/failed/unknown 规范化，以及 terminal Provider Receipt 与 Cost/Quota Owner 终态的共享 GORM 事务。

本增量使用受控 Provider Gateway Adapter 验证跨网络边界和故障矩阵，不接入真实云 Provider SDK，不实现 Callback 公网验签、对象下载/自动 Staging、Outbox Worker、Fallback、Candidate 自动登记、公开 REST 或 Workflow 图片节点，因此不代表第一笔真实云 Provider 调用、`BE-MOD-007`、`BE-JRN-004` 或阶段 6 完成。

## 实现事实

1. `GenerationProviderBindingVersion` 以 Project + `generation.image` + Revision 追加发布，只冻结非敏感 Provider Adapter Key、Model Key 与 Credential Reference；Secret 值不进入 SQL、Hash 或 Receipt。旧 Request 始终引用创建时 Binding ID/Revision。
2. `SubmitImageRequest` 在一个 PostgreSQL/GORM 事务中锁定 Intent，重新验证发起人权限、Claim Token/Fencing、短时 Authorization、Input/Units 以及仍为 Reserved 的 Cost/Quota Owner 事实，然后按 Intent 唯一创建不可变 `GenerationRequest` 与 `ProviderJob(DISPATCHING)`。远程 Gateway 只在事务提交后调用。
3. Request Key 固定为内部 Request ID 派生的稳定身份。同一 Intent 并发首次调用只有创建者执行一次 Submit；已存在 `DISPATCHING/RUNNING/UNKNOWN` Job 的调用只能使用原 Request Key Query，不能换 Key 或创建第二个 Request/Job。
4. Gateway transport error 和不完整/非法 Outcome 均收敛为 `UNKNOWN/OUTCOME_UNKNOWN`，Cost/Quota 保持 Reserved。accepted 进入 `RUNNING/SUBMITTED`；UNKNOWN 可被迟到 terminal Outcome 收敛，但已知 Provider Job Key 不能被清空或替换，terminal Job 不再调用远程边界。
5. succeeded 必须有稳定 Provider Event ID、Provider Job Key、实际 Units 和排序 Output References；每个输出冻结 Output Key、私有 Staging Object Key、SHA-256、字节数、媒体类型与宽高。failed 必须有稳定 Event ID/失败码且无输出。Provider + Event 和 Job 各自唯一，重复 Event 不能绑定另一 Job。
6. terminal Outcome 在一个 Generation 外层 GORM 事务中创建唯一 `GenerationProviderResultReceipt`，并调用 Cost `Settle` + Quota `Consume` 或 Cost/Quota `Release`，随后更新 Job/Intent 和 Command Receipt。Quota 或任一 Owner 失败会回滚 Provider Receipt、Cost Ledger/Reservation 与全部 Generation 终态。
7. Binding、Request、Job、ResultReceipt 和扩展 Intent 均进入唯一 GORM Model Catalog；实现没有 Migration 文件、DDL/Raw SQL、第二 ORM、第二数据库、Redis 正确性、跨事务补偿或 Agent 业务事实。

## 真实验收证据

- Red 阶段先执行目标 Generation 测试，明确因 `ProviderOutcome`、`ProviderSubmission`、`GenerationProviderJob`、`NewProviderService` 与 `NewProviderStore` 缺失而编译失败；补齐最小 Domain/Application/GORM 边界后同一测试转绿。
- 全新 PostgreSQL `16.15-alpine` 下，目标 Provider 旅程连续三次通过（累计 7.390 秒），`go test -race` 6.299 秒通过；目标扩展后单次 6.725 秒通过。
- 首次 Submit 返回 UNKNOWN 时双 Reservation 保持 Reserved；同命令重放不调用远程，Reconcile 只 Query 一次并用迟到 succeeded Receipt 将 Cost 结算、Quota 消费和 Job/Intent 终态原子提交。terminal 新幂等键重放不再 Query。
- Project 发布 Binding Revision 2 后，旧 Request 仍引用 Revision 1；新 Request 使用 Revision 2。已冻结 Provider Job Key 的替换被拒绝，另一 Job 复用已有 Provider Event ID 被拒绝，二者均不改变费用或配额。
- 明确 Provider failed 只在持久失败 Receipt 同事务双 Release；transport error 保持 UNKNOWN。注入 Quota 已 Released 后再收到 succeeded，终态事务失败并回滚先执行的 Cost Settle，Provider Receipt 计数为 0。
- 8 路同 Intent 并发调用只产生 1 个 GenerationRequest、1 个 ProviderJob 和 1 次 Submit；其余调用使用同 Request Key Query。旅程清理后 Intent/Request/Job/ProviderReceipt/Workflow Run/Node 计数为 `0|0|0|0|0|0`。
- PostgreSQL Catalog 实际包含 Provider Job 精确状态/终态 CHECK、Request/Job 唯一绑定、Request Key 唯一索引和 Provider Event 唯一索引；禁用 GORM Nested Transaction Savepoint 时 Provider Store 直接拒绝。
- 全新 PostgreSQL + Temporal + MinIO 下执行 Backend `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全绿；Generation 套件 23.976 秒，Workflow 真实集成套件 141.191 秒，未跳过外部服务旅程。
- Agent 使用真实 Python 3.11 临时虚拟环境执行 Ruff check/format、Pyright 与 12 个 Pytest 全绿；Frontend 使用 Node 22.23.2 执行 `npm ci`、OpenAPI 重生成、ESLint、TypeScript、45 个 Vitest 与 Next.js 生产构建全绿，生成客户端无漂移。
- 开发/生产 Compose、仓库卫生、Frontend/Backend/Agent 三类部署镜像和 API、Workflow Worker、Frontend standalone、Agent Candidate Runtime 入口全部通过。

## 边界与残余风险

- 当前 Gateway 是 Backend Application Port，验收使用受控 Adapter 验证 Submit/Query 调用次数和 Outcome；尚无真实云 Provider HTTP/SDK、Credential Resolver、Rate Limit 或 Callback 验签，不能声称已完成第一笔真实图片生成。
- Provider Output Reference 只验证规范化不可变元数据；真实下载、私有 MinIO Staging、Asset `register_staged → validate_ready` 和 Candidate 自动登记尚未串联。
- Reconcile 当前由内部 Application Command 显式触发，尚无 Outbox/Worker 调度、Callback Event 接入或运维入口；UNKNOWN 不会自动释放或盲目重提。
- 当前只支持 Project 最新单图片 Binding，没有 Capability Registry、Model Policy、Fallback 或多 Provider 路由；真实需求出现前不预建这些抽象。
- 远端 `origin/main` 当前为 `ec2bb71`，本地提交 `40197a3` 尚未推送；当前 Provider 任务将在本记录所列完整本地 CI 后独立提交，远端 GitHub Actions 仍须获得推送授权后验证。
- `agent-browser` 按约定只在全部开发完成后执行；当前仍有真实 Provider/Callback/Staging、CandidateSet/Workflow 和单 Shot Workflow 后续任务，本切片不提前调用。
