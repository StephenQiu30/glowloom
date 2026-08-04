# generation 协议毒消息终止收敛验收

- 状态：accepted（仅 PT-MQ-003 的已知 generation schema/payload 协议错误增量；PT-MQ-003、DEV-S4-02 与 S4 整体保持 in_progress）
- 日期：2026-08-04
- 验收基线：`main@0909bd4` 工作树；本实现与证据在同一后续提交进入 `main`
- 对应需求：[生产模块需求](../requirement/009-生产模块需求.md)、[消息队列与异步投递需求](../requirement/011-消息队列与异步投递需求.md)
- 对应设计：[生产模块详细设计](../design/模块设计/009-生产模块详细设计.md)、[消息队列详细设计](../design/模块设计/012-消息队列详细设计.md)
- 对应产品任务：[PT-MQ-003 当前协议毒消息增量](../prd/009-剪辑交付与平台保障PRD任务.md)
- 对应工程计划：[DEV-S4-02 当前增量](../plan/000-MVP全栈实施总计划.md)
- 验收边界：只接受可信 `generation.requested` Envelope 中未知 schema/非法 payload 对已知 queued generation Task 的零外部副作用收敛

## 1. 验收结论

1. 未知 schema 和 payload task_id 不匹配在 Attempt 创建前被分类为不可重试协议错误；实测两条路径均为零 GenerationAttempt、零 Provider 调用、零 Candidate/Media/settle。
2. 同一 PostgreSQL 事务把 queued Task 置为 `failed/manual_attention`，保存稳定 `unsupported_message_schema|invalid_message_payload`、`retryable=false`和 `next_action=contact_support`；active Reservation 转 released，追加全额且 `attempt_id=null` 的 release CostEntry 与唯一 `task.failed` Audit。
3. Messaging 在上述业务事实同事务内把 Inbox 置为 `manual_attention`，且 ack 回调从新 session 已能读到全部已提交事实；不存在“先 ack、后释放”窗口。
4. 同 event 重投将 Inbox attempt_count 增加到 2 后回读 duplicate，release、`task.failed` Audit 仍各一条且 Attempt 仍为零。另一 event_id 的同类错误到达已失败 Task 时只写 rejected Inbox，不修改终态或二次释放。
5. Production 只暴露类型化 `GenerationProtocolErrorCode` 和应用用例，不引用 Envelope/Inbox ORM；Messaging 不直接写 Task/Reservation/Cost，跨模块架构守卫未放宽。

## 2. Red → Green 证据

| 阶段 | 命令与真实结果 |
| --- | --- |
| Red | `.venv/bin/python -m pytest tests/integration/test_generation_attempt_execution.py -q` 为 2 failed/4 passed；两个失败均在 ack 回调中读到 Task 仍为 `queued`，精确证明原缺口。 |
| Green | 同文件最终 6/6，覆盖未知 schema、非法 payload、ack 前提交、同 event duplicate、新 event 终态保护、零 Attempt 与单次 release/Audit。 |
| 边界/Typing | delivery/module boundary + generation 定向为 10/10；Ruff 全绿，Pyright 为 0 errors/0 warnings/0 informations。 |
| 真实 Broker | `make contract-rabbitmq` 为 6/6；两条同 event_id 持久消息经真实 `lanverse.io` 队列消费，首条 rejected、次条 duplicate，两者 manual ack 后 ready=0，PostgreSQL 与上述事实对账一致。 |

## 3. 全量与无密钥回归

- `make check` 全绿：Ruff、Pyright 0 errors/0 warnings、ESLint、TypeScript；后端 266 passed/23 个显式外部或性能开关 skipped；前端 17 文件/56 tests；`pip check`、Next.js 16.2.12 生产构建和 development/production Compose config 均通过。
- `DEEPSEEK_API_KEY='' ARK_API_KEY='' LANVERSE_E2E_BACKEND_PORT=8002 LANVERSE_E2E_FRONTEND_PORT=3001 make e2e` 为 9/9（1.1 分钟）。本增量没有读取或需要真实 Key，没有 Ollama、Ark SDK 或 fake Provider 成功。

## 4. 未接受项与剩余风险

无法解析或超限 body 没有可信 event/workspace/task ID，当前仍只使用固定低基数标签拒绝并 ack，不生成 Inbox。未知 event type、业务可重试分类、PostgreSQL available_at/重试上限、告警、运维查询/重放、Provider unknown/query/cancel/reconcile 和新 Attempt sequence 均未接受。Inbox `manual_attention` 已是可查持久事实，但当前没有独立告警或 operator API，不得把本次低基数 rejected 指标冒充为人工处置运维闭环。

D-004 仍 open；本次只证明 Attempt 之前、外部副作用明确为零的协议错误，不能外推到真实 Seedream/Seedance submit 前后崩溃或供应商结果未知场景。
