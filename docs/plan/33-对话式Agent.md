# PLN-33 对话式Agent 执行计划

| 项 | 内容 |
| --- | --- |
| 产品需求 | [PRD-33](../prd/33-对话式Agent.md) |
| 需求规格 / 功能设计 | [REQ-37](../requirement/37-对话式Agent.md) · [DES-39](../design/39-对话式Agent.md) |
| 里程碑 | M5（[PLN-01](01-实施路线与交付计划.md)） |
| BACKLOG Epic | E-37（[BACKLOG](../../BACKLOG.md)） |
| 前置计划 | E-18（[BACKLOG](../../BACKLOG.md)）、E-19（[BACKLOG](../../BACKLOG.md)）、E-21（[BACKLOG](../../BACKLOG.md)）、E-36（[BACKLOG](../../BACKLOG.md)） |
| 联调 | 无 |
| 状态 | 待办 |

## 1. 目标与完成定义

完成即 TC-37-01, TC-37-02, TC-37-03, TC-37-04 通过，[DES-39](../design/39-对话式Agent.md) 的测试设计及非功能要求有证据；依 [PLN-02 §3.3](02-项目管理与变更.md#33-完成定义dod) 完成格式、静态检查、单元/集成、Race、漏洞、契约、演示与追踪矩阵门禁。

## 2. 前置条件

- 前置 Epic：E-18（[BACKLOG](../../BACKLOG.md)）、E-19（[BACKLOG](../../BACKLOG.md)）、E-21（[BACKLOG](../../BACKLOG.md)）、E-36（[BACKLOG](../../BACKLOG.md)）；按 [BACKLOG](../../BACKLOG.md) 对应里程碑的“Epic 实施顺序”执行。
- 底座与外部条件：PLN-01 对应里程碑的命令层、真实依赖与运行环境；测试数据为近景修改提案、10 镜头生成草稿、断线续传和恶意台词。
- 待确认：REQ-37-Q1、DES-39-Q1、DES-39-Q2、DES-39-Q3、DES-39-Q4。确认前不把默认方案写成已批准结论。

## 3. 任务分解

清单范围以 [DES-39](../design/39-对话式Agent.md) §1 和 [BACKLOG](../../BACKLOG.md) E-37 为准；下表任务仅写实施动作及关键约束。

| 编号 | 任务 | 产出（代码目录 / 文件） | 依赖 | 验证方式 |
| --- | --- | --- | --- | --- |
| E-37-01 | 数据与领域模型：按 [DES-39 §2](../design/39-对话式Agent.md#2-数据) 落实表与仓储。 覆盖：`agent.message`（本 Epic 建表）、`agent.proposal`（本 Epic 建表）、`agent.session`（本 Epic 建表）、`agent.run`（本 Epic 建表）、`operation.operation`（E-21 建表，本 Epic 只加列或复用）、`operation.provider_call`（E-24 建表，本 Epic 只加列或复用）。 | `backend/db/migrations/`、`backend/internal/agent/domain/`、`backend/internal/agent/adapter/postgres/` | 前置 Epic | 迁移、权限与真实库集成 |
| E-37-02 | 用例与接口：按 [DES-39](../design/39-对话式Agent.md) 的接口节实现清单端点；写操作经鉴权、幂等、版本、审计与 Outbox。 覆盖：`POST /api/projects/{pid}/agent/sessions`、`GET /api/projects/{pid}/agent/sessions`、`POST /api/projects/{pid}/agent/sessions/{sid}/runs`、`GET /api/projects/{pid}/agent/sessions/{sid}/messages`、`POST /api/agent/proposals/{id}:apply`、`POST /api/agent/proposals/{id}:reject`、`DELETE /api/projects/{pid}/agent/sessions/{sid}`、`GET /api/admin/agent/runs/{run_id}/debug`、`POST /api/projects/{pid}/agent/sessions/{sid}/budget-quotes`、`POST /api/projects/{pid}/agent/sessions/{sid}:close`、`POST /internal/agent/runs/{run_id}/calls`、`PUT /internal/agent/runs/{run_id}/calls/{call_seq}`。 | `backend/internal/agent/application/`、`backend/internal/agent/adapter/http/`、`backend/docs/` | 前置 Epic 与上一任务 | 契约、鉴权与错误路径 |
| E-37-03 | 异步、工作流与事件：按 [DES-39](../design/39-对话式Agent.md) 的异步节落实清单中的任务与事件；验证重试、去重和终态。 覆盖：`OperationWorkflow`、`agent-session-settle`。 | `backend/internal/agent/adapter/workflow/`、`backend/internal/agent/adapter/event/` | 前置 Epic 与上一任务 | 真实依赖、重试与事件回放 |
| E-37-04 | Agent 服务：agent-api 流式执行只读工具、命令提案和生成草稿；守住项目范围。按 Go 与 Python 各自定义的类型做契约测试。 覆盖：无独立设计清单标识。 | `agent/app/api/`、`agent/app/harness/tools.py`、`agent/evals/` | 前置 Epic 与上一任务 | 契约与离线评测 |
| E-37-05 | 前端：按 [DES-39](../design/39-对话式Agent.md) 的界面节完成流式对话、提案差异与生成草稿卡；额度追加与断线续传可见。 覆盖：无独立设计清单标识。 | `frontend/src/features/agent/` | 前置 Epic 与上一任务 | 交互与端到端场景 |
| E-37-06 | 测试与验收：覆盖 [REQ-37](../requirement/37-对话式Agent.md) §3 与本计划 §5 的特有场景。 覆盖：无独立设计清单标识。 | `backend/tests/`、`agent/tests/`、`frontend/tests/` | 前置 Epic 与上一任务 | 验收用例与证据 |

数据访问范围（[DES-02](../design/02-领域与数据模型.md) DDL）：`agent.session`、`agent.proposal`、`agent.run`、`operation.operation`、`operation.provider_call` 有 `project_id`；`agent.message` 经 `session_id` 继承项目范围。

## 4. 实施顺序

1. Red：先为 TC-37-01, TC-37-02, TC-37-03, TC-37-04 所依赖的业务规则、异常与跨边界路径写失败测试；E-37-06 在各任务中持续补齐测试与验收。
2. Green：按 E-37-01 → E-37-02 → E-37-03 落实数据、接口和异步组合，逐项让测试通过。
3. Refactor：保持测试通过，整理边界与错误路径；契约稳定后 E-37-04、E-37-05 可并行，最后合并验收证据。

## 5. 验证与验收

- 质量门禁：按 [PLN-02 §3.3](02-项目管理与变更.md#33-完成定义dod) 完成定义与 [TST-01](../test/01-测试策略.md) 执行；保留真实命令、环境与结果。
- 本功能特有验证：提案失效、越权工具与提示注入评测；会话额度追加、中断结算、断线按序续传和生成草稿二次确认。
- 验收用例：TC-37-01, TC-37-02, TC-37-03, TC-37-04（[TST-02](../test/02-需求追踪矩阵.md)）；数据：近景修改提案、10 镜头生成草稿、断线续传和恶意台词。
- AI 能力评测按 [TST-03](../test/03-AI评测方案.md) 使用对应数据集留证。

## 6. 风险与回退

- 提示注入或越权工具输出诱导写操作：停用有风险工具，保留只读对话；所有提案重新走用户确认和权限校验。
- 运行中断导致额度费用未知：停止新运行并以已确认占用暂估结算，后续按供应商账单只退不补；消息落库后按序续传。

