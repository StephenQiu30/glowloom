# Quota 图片生成日配额验收记录

- 状态：实现、真实目标旅程与完整本地 CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)

## 验收范围

本记录验收 `BE-JRN-004` 的 Quota 前置边界：Workspace Owner 为 Project 设置单一 `generation.image` UTC 日配额 Policy；Owner/Editor 使用稳定 Source 建立唯一 Reservation；Quota 在 PostgreSQL/GORM 事务内锁定 Policy 和当天 Counter，并用显式 Consume/Release 维持 Reserved、Consumed、Released 生命周期及 Owner CommandReceipt。

本增量不包含 Cost、GenerationIntent、Cost/Quota 共享准备事务、Execution Claim/Authorization、Provider/Model/Price、Submission/Callback、Outbox、Unknown Reconcile Worker、公共 HTTP 或前端管理页面，因此不代表 `BE-JRN-004` 或阶段 6 已完成。

## 实现事实

1. Quota 只支持 `generation.image` 与 `UTC_DAY`；窗口由 Backend 的服务端时钟计算，Policy 以 Expected Revision 更新，只有当前 Workspace Owner 可以创建或修改。
2. Reserve 在一个 GORM/PostgreSQL 事务中按 Policy → Counter 顺序加锁，冻结 Policy Revision、Limit、窗口、Source 和 Units，并原子写唯一 Reservation 与 `quota.reservation.reserve` CommandReceipt。
3. 同一 Policy、窗口、Source Type 与 Source ID 只有一条 Reservation；同一命令或同一 Source 重投收敛为原事实，Units、Policy Revision、Source 或命令输入漂移均失败关闭。
4. Consume/Release 按 Policy → Counter → Reservation 的一致锁序更新状态与计数并写各自 Receipt。结果未知时没有自动动作，Reservation 保持 `RESERVED`；已消费和已释放之间禁止相互转换。
5. 每次预留或转换前均从当前窗口的 Reservation 事实重算 Reserved/Consumed 数量，并与 Counter 对账；Counter、Reservation 绑定哈希、终态时间或不可变事实漂移时拒绝继续。
6. `QuotaPolicy`、`QuotaCounter`、`QuotaReservation` 进入唯一 GORM Model Catalog。实现没有 Migration 文件、手写 SQL、Redis Counter、第二数据库、兼容字段或双写分支，PostgreSQL 是唯一 Quota 事实源。

## 真实验收证据

- Red 阶段先执行 `go test ./tests/quota`，明确因 `internal/quota/adapter/gormdb` 尚不存在而编译失败；完成最小实现后同一目标测试转绿。
- 目标旅程使用 PostgreSQL `16.15-alpine`。Owner 建立 limit=3 的 Policy 后，8 路同 Source 并发预留收敛为同一 Reservation；同一 Source 重投、命令回放及 Units 漂移分别按契约处理。
- 下一 UTC 日对 8 个不同 Source 各预留 2 Units、日上限 6；并发结果精确为 3 次成功和 5 次 `quota_exceeded`，Counter 最终为 Reserved=6、Consumed=0，没有超限。
- Policy 降至当前用量以下、陈旧 Expected Revision、Viewer 写入、撤销 Token、Counter 漂移、Reservation 绑定哈希漂移均失败关闭；Owner/Editor 的权限边界按当前身份重新验证。
- Consume 与 Release 的重放没有第二次计数；已消费后 Release、已释放后 Consume 均被拒绝；未发出终结命令的未知结果保持 Reserved。
- 提交前审计新增“次日更新 Policy 后释放上一窗口未知 Reservation”用例，修复前真实返回 `Quota counter facts have drifted`；实现拆分当前 Counter 与历史 Counter 校验后转绿。当前窗口仍绑定最新 Policy Revision/Limit，历史窗口按自身冻结事实终结，当前日和上一日 Counter 均保持守恒。
- 目标用例连续三次执行 `go test -count=3 -run TestDailyImageQuotaReservationLifecycle ./tests/quota`，全部通过。测试过程中真实发现 GORM `ON CONFLICT DO NOTHING` 在冲突路径会改写传入记录、继而令后续查询使用零值；实现改为分离创建记录与持久化查询目标后通过并发验收，未跳过断言或增加兼容分支。
- 最终提交前复跑时，前两次分别因误用临时数据库密码和数据库名而在连接阶段失败，尚未进入业务断言；改为读取本任务容器的实际 `POSTGRES_USER/POSTGRES_PASSWORD/POSTGRES_DB` 且不输出值后，同一目标旅程以退出码 0 完成，实际运行约 2.4 秒。
- 修复后首次完整复跑误用了上一轮已有事实的数据库，Quota 通过，但 Authoring 全表计数与 Review 固定测试邮箱分别因旧事实失败；该结果未计作 CI 通过，也未通过清库或兼容分支掩盖。随后按 GitHub Actions 契约启动全新固定版本依赖重跑。
- 全新 PostgreSQL + Temporal + MinIO 下，`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 均以退出码 0 完成；Quota 目标包实际运行约 2.6 秒，Workflow 套件实际运行约 68.5 秒，外部依赖旅程未跳过。
- Agent 的 Ruff check/format、Pyright 零错误与 12 个 Pytest 通过；Frontend OpenAPI 重生成、ESLint、TypeScript、45 个 Vitest 和 Next.js 生产构建通过，生成 Client 无漂移。
- 开发/生产 Compose 与仓库卫生检查通过；Frontend、Backend、Agent 三类部署镜像从当前源码重新构建，并分别通过 standalone、API/Workflow Worker 双二进制和私有 Candidate Runtime 入口断言。

## 边界与残余风险

- Quota 当前是 Backend 内部 Application Port，尚未进入 Generation 高成本准备协调器；没有真实 GenerationIntent 或 Provider Job 消费该配额，不能宣称图片 Provider 调用已经受控。
- Policy 当前只覆盖图片生成单位，不含价格、币种、预算、Rate Limit、动态指标注册或管理 UI；新增这些能力前必须先解决现有 Project Budget 字段与 Cost Owner 的唯一事实归属。
- Unknown 结果按契约保留 Reservation，但 Reconcile Worker 尚未实现；在对账能力交付前不能自动释放或重新提交。
- 远端 `origin/main` 仍停留在已通过 GitHub Actions 的 `2f6e066`；本地后续提交只有在获得推送并实际运行后才能报告远端绿色。
- `agent-browser` 按约定只在全部开发完成后执行；当前仍有 Cost、GenerationIntent、Provider、CandidateSet/Workflow 和单 Shot Workflow 后续任务，因此本切片不提前调用。
