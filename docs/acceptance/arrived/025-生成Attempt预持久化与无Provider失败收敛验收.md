# 生成 Attempt 预持久化与无 Provider 失败收敛验收

- 状态：accepted（仅 DEV-S4-02 当前 Attempt `prepared → failed` 无 Provider 增量；PT-PROD-005/006、PT-MQ-003、DEV-S4-02 与 S4 整体保持 in_progress）
- 日期：2026-08-04
- 验收基线：`main@b0486bf` 工作树；本实现与证据在同一后续提交进入 `main`
- 对应需求：[生产模块需求](../../requirement/009-生产模块需求.md)、[消息队列与异步投递需求](../../requirement/011-消息队列与异步投递需求.md)
- 对应设计：[生产模块详细设计](../../design/模块设计/009-生产模块详细设计.md)、[数据库表与数据生命周期详细设计](../../design/模块设计/011-数据库表与数据生命周期详细设计.md)、[消息队列详细设计](../../design/模块设计/012-消息队列详细设计.md)
- 对应产品任务：[PT-PROD-005/006、PT-MQ-003 当前增量](../../prd/009-剪辑交付与平台保障PRD任务.md)
- 对应工程计划：[DEV-S4-02 当前增量](../../plan/000-MVP全栈实施总计划.md)
- 验收边界：接受真实 PostgreSQL Attempt、真实 RabbitMQ generation consumer、发送前持久化、无 Provider 的零外部副作用失败释放和同 Attempt redelivery；不接受任何 Provider submit/query/cancel、unknown/reconcile、Candidate/Media/settle 或 S4 产品结果

## 1. 验收结论

1. 新增 `prod_attempts`，以 `(task_id,workspace_id)` 复合引用 Task，并固定 `(task_id,sequence)`、`provider_request_key`、`(id,workspace_id)` 唯一约束；CostEntry 的非空 `(attempt_id,workspace_id)` 也必须引用同租户 Attempt。真实 PostgreSQL 已拒绝跨 Workspace Attempt、重复 sequence 和重复 provider_request_key。
2. provider_request_key 使用版本前缀、Workspace/Task/sequence/input hash 的 SHA-256，长度固定 64，同输入稳定、sequence 或 input hash 变化时不同；原始 ID 和该 key 不进入普通日志、指标或前端。
3. `generation.requested` 由独立 `lanverse.io.generation.v1` consumer 处理。事务 A 先写 processing Inbox、prepared Attempt、`running/validating` Task 和审计并提交，代码路径中没有 Provider 客户端、网络或对象存储调用。
4. 当前没有 D-004 验证过的图片/视频 Provider。事务 B 因此只执行明确的 fail closed：同一 Attempt 进入 `failed/provider_dispatch_unavailable`，Task 进入 `failed/blocked`，active Reservation 进入 released，追加关联该 Attempt 的全额 release CostEntry，完成 Inbox/Audit，提交后才 manual ack。
5. 注入事务 B 失败后，首条消息 nack/requeue，数据库保留同一 prepared Attempt 和 processing Inbox；redelivery 的 Inbox attempt_count 变为 2，并复用原 attempt_id 收敛。完成后的同 event 再投只回读 completed Inbox，不新增 Attempt、release 或审计。
6. queued 取消先获胜时，后到的 generation 事件直接完成 Inbox 并 ack，Attempt 保持零、release 仍只有 queued 取消产生的一条。Worker 获得 queued Task 时则通过 Task 行锁先进入 running，既有取消接口保持明确冲突，不出现双重释放。

## 2. Red → Green 与边界校准

| 阶段 | 命令与真实结果 |
| --- | --- |
| 事实契约 Red | 首次收集新增集成与 RabbitMQ 契约时失败：`ImportError: cannot import name 'GenerationAttempt' from app.modules.production.models`，证明原系统没有 Attempt 事实。 |
| PostgreSQL/Worker Green | provider_request_key、租户/唯一性、发送前持久化、ack 前提交、重复投递、finalize 回滚恢复和 cancelled winner 定向通过；最终相关 unit/integration/architecture 回归全部通过。 |
| 架构守卫校准 | 首轮 `make check` 为 261 passed/22 skipped 且仅 `test_cross_module_imports_use_public_contracts` 失败，指出 Production 直接引用 Messaging Inbox ORM。实现没有放宽 allowlist，而是把 Envelope/Inbox 归回 Messaging、Task/Attempt/Cost 归回 Production，并通过无框架 `PreparedGenerationAttempt` 公开契约交接；定向 12/12、Ruff、Pyright 随后全绿。 |
| Broker Green | `make contract-rabbitmq` 为 5 passed；新增真实 `io.provider.submit` 持久消息由 Worker 消费，manual ack 后队列 ready=0，数据库已有 failed Attempt、released Reservation、attempt 关联 release 与 completed Inbox。 |

## 3. 恢复与回归证据

- `make contract-scheduler-stack` 为 1 passed，Schedule/Fire/Task/Outbox 的 RabbitMQ 暂停恢复不受新表与 consumer 影响。
- `make contract-media-stack` 为 4 passed，真实 PostgreSQL、RabbitMQ、本机 MinIO 和 ffprobe 的媒体上传、探测、清理及位置迁移/退役保持原结果。
- 最终 `make check` 全绿：Ruff、Pyright 0 errors/0 warnings、ESLint、TypeScript、后端 263 passed/22 个显式外部或性能开关 skipped、前端 17 文件/56 tests、`pip check`、Next.js 16.2.12 生产构建和 development/production Compose config 均通过。RabbitMQ、Scheduler 与 Media 默认 skip 已由上述显式真实命令补证。
- `DEEPSEEK_API_KEY='' ARK_API_KEY='' LANVERSE_E2E_BACKEND_PORT=8002 LANVERSE_E2E_FRONTEND_PORT=3001 make e2e` 为 9/9（1.1 分钟）；空 Key 能力仍明确阻断且费用为零，现有剧本、媒体、资产、分镜和调度闭环未退化。

## 4. 明确未接受项与残余风险

D-004 仍 open，因此当前没有 Ark SDK、Seedream/Seedance submit/query/cancel、provider_task_id、`submitting/accepted/polling/succeeded/unknown` 转换、自动 reconcile、新 Attempt retry、Candidate、Media 或 settle。测试中的 active Capability 只建立合法的本地请求/预占输入；没有 fake Provider 返回，更没有把失败路径冒充生成结果。

未知 schema/非法 generation payload 当前只持久化 rejected Inbox，尚未把 Task/Reservation 推进 manual_attention；Provider 调用一旦真正发生，进程在 submit 前后退出的 unknown 语义也必须用真实账号契约重做，不能从本验收的“明确零外部调用”恢复结论外推。

项目当前仍采用空库/已知表 `create_all`，没有在线迁移框架。新环境会得到完整 Attempt 与 CostEntry 复合外键；既有数据库可创建新 Attempt 表，但 `create_all` 不会给已存在的 CostEntry 补外键。生产存量升级必须先形成单独迁移设计和真实演练，本增量不把全新测试库证据伪装成在线迁移通过。
