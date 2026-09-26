# DES-39 对话式 Agent

| 项 | 内容 |
| --- | --- |
| 对应需求 | [REQ-37 对话式 Agent](../requirement/37-对话式Agent.md)：AGT-01 对话式 Agent；AGT-02 会话管理；AGT-03 Agent 执行可见性；CNV-07 AI 画布助手 |
| 优先级 | MVP（首个版本，PRD-01 P20、P27） |
| 里程碑 | M5（[PLN-01](../plan/01-实施路线与交付计划.md)） |
| 页面 | Agent 面板（全局右侧抽屉）、管理员调试面板 |
| 基础设计 | DES-05 §7；DES-02 §5.13；DES-07 §12 |
| 依赖功能设计 | [DES-21](21-分镜生成与镜头编辑.md)、[DES-22](22-生成模式与参考组合.md)、[DES-24](24-报价与二次确认.md)、[DES-38](38-画布功能.md) |
| 状态 | 草案（待评审，2026-09-25） |

本文把 [REQ-37](../requirement/37-对话式Agent.md) 拆解为实现设计。需求目标、业务规则（文中 R1、R2… 指 REQ-37 §2 的规则）与验收标准以 REQ-37 为准；表结构以 [DES-02](02-领域与数据模型.md)、接口签名以 [DES-03](03-接口设计.md)、工作流与生成操作以 [DES-04](04-工作流与生成操作.md) 为准，本文说明该功能用到哪些部分、如何组合。设计发现需求问题时先改需求（[文档规则](../README.md#文档规则) 2）。

## 1. 设计清单

| 类别 | 内容 |
| --- | --- |
| 数据表 | `agent.message`、`agent.proposal`、`agent.session`、`agent.run`、`operation.operation`（额度）、`operation.provider_call` |
| 接口 | `POST /api/projects/{pid}/agent/sessions`<br>`GET /api/projects/{pid}/agent/sessions`<br>`POST /api/projects/{pid}/agent/sessions/{sid}/runs`<br>`GET /api/projects/{pid}/agent/sessions/{sid}/messages`<br>`POST /api/agent/proposals/{id}:apply`<br>`POST /api/agent/proposals/{id}:reject`<br>`DELETE /api/projects/{pid}/agent/sessions/{sid}`<br>`GET /api/admin/agent/runs/{run_id}/debug`<br>`POST /api/projects/{pid}/agent/sessions/{sid}/budget-quotes`<br>`POST /api/projects/{pid}/agent/sessions/{sid}:close`<br>`POST /internal/agent/runs/{run_id}/calls`<br>`PUT /internal/agent/runs/{run_id}/calls/{call_seq}` |
| 工作流与任务 | `OperationWorkflow`、`agent-session-settle` |
| 领域事件 | 无 |
| 测试用例 | TC-37-xx（[TST-02](../test/02-需求追踪矩阵.md)） |

## 2. 数据

`agent.session`（`last_seq`、`current_operation_id`、`status`）、`agent.run`（每次运行占用的额度与实际费用）、`agent.message`（`seq`、`role`、`event_type`、`content`、`run_id`）、`agent.proposal`（`kind`、`commands`、`diff`、`operation_ids`、`status`）；见 DES-02 §5.13。

## 3. 接口

```http
POST /api/projects/{pid}/agent/sessions                  { "title": "", "budget_micros": 2000000 }
    → 201 { "session_id": "…", "quote": { "operation_id": "…", "quote_micros": 2000000 } }；用户经 `POST /api/operations/{id}:confirm` 确认后会话才可运行（DES-04 §8.7）
POST /api/projects/{pid}/agent/sessions/{sid}/budget-quotes { "budget_micros": 2000000 }   → 追加额度报价
POST /api/projects/{pid}/agent/sessions/{sid}:close                                      → 结束会话并结算
GET  /api/projects/{pid}/agent/sessions
POST /api/projects/{pid}/agent/sessions/{sid}/runs       （SSE，AG-UI 事件流）{ "message": "把第 3 场的镜头都改成近景", "context": { "page": "storyboard", "episode_id": "…" } }
GET  /api/projects/{pid}/agent/sessions/{sid}/messages?after_seq=120
POST /api/agent/proposals/{id}:apply                     → 服务端校验提案属于当前用户会话，按用户身份逐条执行（等同用户调用），返回逐条结果
POST /api/agent/proposals/{id}:reject
DELETE /api/projects/{pid}/agent/sessions/{sid}
GET  /api/admin/agent/runs/{run_id}/debug                （管理员）
```

提案应用由服务端代为逐条执行是为保证“与用户手动调用等价”且原子地记录结果；执行身份为点击应用的用户，审计中带 `via_agent_proposal_id`。

AG-UI 事件映射见 DES-01 §7.4；自定义事件 `canvas_commands`、`proposal`、`operation_draft`。

## 4. 异步与工作流

对话运行不使用 Temporal（交互式、短时），`agent-api` 直接流式执行。每次运行前 Go 在当前额度内登记一条 `agent.run` 并占用 `run_cap`，作为 Harness Budget 传入；每一次模型调用在发起前登记一条 `provider_call`（带 `request_key`），返回后立即写入实际用量与费用；运行结束只汇总 `agent.run.cost_micros`；中断的运行由 `agent-session-settle` 判为 abandoned。额度的确认、追加、结算规则以 DES-04 §8.7 为准（本段为摘要）。生成草稿确认后走 OperationWorkflow。

## 5. 事件与实时

AG-UI over SSE；提案应用产生的业务事件照常发布。

## 6. 界面

CopilotKit 面板：流式消息（Streamdown）、步骤与工具调用折叠卡、提案差异卡（应用 / 拒绝）、生成草稿卡（报价确认组件）；会话列表；管理员调试抽屉（上下文、事件流）。

## 7. 权限与审计

制作者；审计：`agent.proposal_applied`、`agent.proposal_rejected`，业务命令审计带提案 ID。

## 8. 非功能要求

PERF-11（首个流式片段 ≤ 3 秒）；SEC-09（提示注入用例集）。

## 9. 测试设计

提案失效与重放；工具范围越权测试；提示注入评测集（TST-03）。

## 10. 待确认

| 编号 | 问题 | 默认方案（确认前按此实施） | 确认时机 |
| --- | --- | --- | --- |
| DES-39-Q1 | CopilotKit 与 ag-ui-protocol 的许可（原 REQ-37-Q2）。 | MVP 开发前核实；不满足要求时前端改用自研面板直接消费 AG-UI 事件流。 | M5 开始前 |
| DES-39-Q2 | 对话式 Agent 使用哪个 LLM？ | 经 Model Router 选择，P0 评测后确定（TST-03）；不同 Skill 可配置不同模型。 | P0 结束时 |
| DES-39-Q3 | 浏览器断线后，服务端的 Agent 运行是否继续？ | 继续运行并逐条落库；重连后按 `after_seq` 续传，不重复执行。 | M5 开始前 |
| DES-39-Q4 | 会话与消息保留多久？ | 保留 90 天；删除会话为软删除，30 天后清理。 | M5 实施中 |

需求层面的待确认问题见 [REQ-37](../requirement/37-对话式Agent.md) 的“待确认”。
