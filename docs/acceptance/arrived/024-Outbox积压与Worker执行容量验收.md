# Outbox 积压与 Worker 执行容量验收

- 状态：accepted（仅 PT-MQ-004/PT-OBS-002 当前 backlog/inflight/prefetch 增量；PT-MQ-004/PT-OBS-002 整体保持 in_progress）
- 日期：2026-08-04
- 验收基线：`main@f22bdbd` 工作树；本实现与证据在同一后续提交进入 `main`
- 对应需求：[消息队列与异步投递需求](../../requirement/011-消息队列与异步投递需求.md)、[日志与可观测性需求](../../requirement/012-日志与可观测性需求.md)
- 对应设计：[消息队列详细设计](../../design/模块设计/012-消息队列详细设计.md)、[日志与可观测性详细设计](../../design/模块设计/013-日志与可观测性详细设计.md)
- 对应产品任务：[PT-MQ-004/PT-OBS-002 当前增量](../../prd/009-剪辑交付与平台保障PRD任务.md)
- 对应工程计划：[DEV-S4-02/DEV-S6-03 当前局部增量](../../plan/000-MVP全栈实施总计划.md)
- 验收边界：接受 PostgreSQL Outbox backlog/age、Worker inflight/capacity 四项低基数 Gauge 和 I/O=4、Media=2 的真实 RabbitMQ prefetch 证据；不接受 D-007 未量化的积压阈值、API 降级拒绝、RabbitMQ ready/unacked 正式指标、独立进程 scrape/readiness、告警或 PT-MQ-004/PT-OBS-002 整体

## 1. 验收结论

1. 新增不可变 `OutboxBacklog(routing_key,state,count,oldest_created_at)` 投影，直接从 PostgreSQL 聚合 `pending/claimed/manual_attention`；RabbitMQ 队列深度、内存任务或日志不参与业务 backlog 判断。
2. 四项 Gauge 固定为 `lanverse_outbox_events(queue,state)`、`lanverse_outbox_oldest_age_seconds(queue,state)`、`lanverse_worker_inflight(queue)` 与 `lanverse_worker_capacity(queue)`。queue 只有 `lanverse.io/lanverse.media/unregistered`，state 只有 `pending/claimed/manual_attention`；未知 routing key 收敛为固定 sentinel。
3. 每次 Outbox snapshot 先把固定 queue × state 全矩阵置零，再写当前 PostgreSQL 聚合，证明 backlog 清空不会遗留旧 Gauge。最老年龄使用 `created_at` 而不是未来 `available_at`，负时钟差归零。
4. Publisher 通过 `finally` best-effort 刷新 snapshot。主动注入 PostgreSQL 观测查询失败后，publish confirm 仍返回 1，Outbox 仍为 published；Prometheus Gauge 主动抛错时也不改变业务 handler 返回。
5. I/O/Media Worker 的 async handler 外层统一以 `try/finally` 记录 inflight，覆盖非法消息、业务处理及 ack/nack，完成、异常和取消路径均归零。capacity 直接复用现有 `IO_WORKER_MAX_IN_FLIGHT=4`、`MEDIA_WORKER_MAX_IN_FLIGHT=2`，与运行时 `prefetch_count` 没有第二套配置。
6. 真实隔离 RabbitMQ vhost 分别发送 5 条 I/O 和 3 条 Media 持久消息：前 4/2 条 handler 被阻塞时并发数不再增长，独立检查 channel 看到第 5/3 条仍 ready；释放后全部 manual ack 且 ready=0。

## 2. Red → Green 证据

| 阶段 | 命令与真实结果 |
| --- | --- |
| 指标契约 Red | 首次运行新增容量观测与 Outbox 集成测试时在收集阶段失败：`ImportError: cannot import name 'observe_outbox_backlog'`，证明原消息指标没有 backlog/inflight/capacity 契约。 |
| PostgreSQL/进程 Green | 快照聚合、矩阵清零、最老年龄、未知 queue、双 handler 并发、finally 归零、Gauge/查询故障隔离及 Publisher/Worker 回归合计 11 passed；Pyright 0 errors/0 warnings，Ruff 通过。 |
| Broker 契约校准 | 消费 channel 上重复 declare 的计数不能作为独立 ready 证据；测试改用同一隔离 vhost 的独立检查 channel。没有因为首轮断言失败而放宽 handler 并发、ready=1 或最终 ready=0 条件。 |
| Broker Green | `make contract-rabbitmq` 为 4 passed；两类 `capacity+1` 契约、持久信封和真实 Worker Inbox/Task/ack 均通过。 |

## 3. 真实恢复与业务兼容证据

- `make contract-scheduler-stack` 为 1 passed：RabbitMQ 暂停/恢复后，Schedule/Fire/Task/Outbox 与发布结果继续收敛，Publisher 的 finally 指标刷新没有替代持久事实。
- `make contract-media-stack` 为 4 passed：真实 PostgreSQL、隔离 RabbitMQ、本机 MinIO 和 ffprobe 的上传探测、清理、迁移/回滚/退役保持原业务结果；Media Worker inflight 包装未改变 ack/nack。
- 测试只使用专用非默认 vhost 和随机测试 ID；没有连接默认 `/` vhost、清理未知队列、读取/输出本地 `.env` 或新增 RabbitMQ Management 依赖/容器。

## 4. 全量发布门禁

- `make check` 全绿：Ruff、Pyright 0 errors/0 warnings、ESLint、TypeScript、后端 253 passed/21 个显式外部或性能开关 skipped、前端 17 文件/56 tests、`pip check`、Next.js 16.2.12 生产构建与 development/production Compose config 均通过。RabbitMQ、Scheduler 和 Media 默认 skip 已由上述真实命令补证。
- `DEEPSEEK_API_KEY='' ARK_API_KEY='' LANVERSE_E2E_BACKEND_PORT=8002 LANVERSE_E2E_FRONTEND_PORT=3001 make e2e` 为 9/9（1.1 分钟）。DeepSeek 未配置路径和 Seedream/Seedance 不可用事实保持 fail closed，费用保持零事实。
- 本增量没有 Ollama、Ark SDK、模拟 Provider 成功、新容器、新队列、DLQ、Management HTTP 客户端或阶段代号式文件名。

## 5. 未接受项与残余风险

D-007 的 S4/S6 backlog 数量、年龄、恢复时限和告警窗口仍未量化，因此当前 Gauge 只提供事实，不把猜测默认值接入高成本提交拒绝或 `/readyz`。PT-MQ-004 仍缺断连角色 readiness、独立 I/O/Media 资源隔离容量基线和正式积压降级；PT-OBS-002/003 仍缺 RabbitMQ ready/unacked 正式采集、独立 Worker scrape、阈值、告警与 runbook。

默认 `app.server` 单进程能聚合两个 Worker 的 Gauge；若生产拆成多进程/多副本，单进程 Gauge 不能直接代表集群总 inflight。届时必须选用每角色受控 scrape 或正式 Prometheus multiprocess/聚合设计并取得真实部署证据，不能由本 Acceptance 外推。
