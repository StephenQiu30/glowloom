# M06 Agent 与可视化画布详细设计

> Design ID：DES-M06
> Requirement：[REQ-AIC](../../requirement/006-M06-Agent与可视化画布需求.md)
> 状态：proposed

## 1. 设计结论

LangGraph 只编排一次 AgentRun 内部推理；业务 Operation、批准和 current 状态仍在 Python 应用层与 PostgreSQL。M06 对业务变更只拥有 `AgentRun`/`AgentProposal`/`ProposalItem` 的运行与提案信封，不拥有信封内的领域命令 Schema、语义校验和接受后写入。信封中的 `target_module + command_type + command_schema_version + payload/hash` 由 M03、M04、M05、M07 或 M10 等目标模块定义；用户接受时必须重新鉴权并调用目标模块公开命令端口。

M06 另外拥有 `CanvasView` 和 `CanvasLayout` 等展示状态，但不拥有节点代表的领域对象或业务依赖。Agent、列表、表格和画布最终提交同一目标命令，不为画布或 Agent 建第二套业务 API。本模块是模块化单体内的逻辑边界，不拆为独立微服务。

## 2. 事实所有权与跨模块契约

### 2.1 M06 拥有与不拥有的事实

| 类别 | M06 拥有 | M06 不拥有 |
| --- | --- | --- |
| Agent 运行 | Run 身份、冻结作用域、graph/contract 版本、状态、result hash、checkpoint 引用 | 业务 Operation 的完成语义、生成 Job、目标对象 current |
| Proposal | 提案批次、项目信封、基线引用、证据引用、决定和执行回执引用 | `command_payload` 的领域语义、目标对象写入、风险例外和批准规则 |
| 会话 | 受保留策略控制的 instruction/session 索引 | 从会话推断的正式人物、镜头、计划或决定 |
| 画布 | 视图所有者、可见性、过滤器、节点位置/大小/分组、装饰边 | 节点领域内容、真实依赖边、执行顺序、对象删除语义 |

`AgentProposalItem` 的最小信封为：`item_id`、`agent_run_id`、`target_module`、`command_type`、`command_schema_version`、`command_payload`、`payload_hash`、`based_on_refs[]`、`evidence_refs[]`、`impact_summary_ref`、`usage_estimate_ref`、`required_capabilities[]`、`external_effect`、`expires_at`。M06 只校验信封结构、大小、引用范围和 hash；目标模块版本化并校验 `command_payload`，M06 不会对字段做领域级补默认值或迁移。

### 2.2 跨模块端口

| 依赖模块 | M06 使用的公开契约 | 一致性与失败规则 |
| --- | --- | --- |
| M01 | `AuthorizeAgentRun`、`AuthorizeCommand`、actor/capability 查询 | Run 创建和每个 ProposalItem 决定都重新鉴权；权限撤销后不使用旧会话权限 |
| M02 | workspace/project/scope 权威引用 | Run 作用域在调用模型前冻结；项目归档阻止新 Run |
| M03/M04/M05/M07/M10 | 版本化 Query 端口、Command Schema Registry、`ExecuteProposedCommand` | 只读查询返回最小字段；接受时目标模块以 expected revision 和领域门禁决定成败 |
| M04 | 可重建依赖投影查询 | 画布真实边只读；投影滞后必须显示 `projection_as_of` |
| M08 | `CreateOperationForAgentRun`、Operation 查询、Outbox/Task 结果端口 | Operation 是用户运行状态；LangGraph checkpoint 不得推断产品完成 |
| M11 | `AuthorizeUsageCap`、`RecordAgentUsage` | 模型/Tool 实际用量与 GenerationJob 分开归因；超限停止未调用节点 |
| M14 | `EvaluateDataEgress`、数据保留与审计端口 | 治理结论和数据快照 hash 写入 Run；高风险门禁不可由 Tool 降级 |

M06 不跨模块写表。`ExecuteProposedCommand` 返回 `target_result_ref`、`target_revision`、`idempotency_replayed`、`error_code`、`recovery_actions[]`；M06 只追加记录决定与回执，不复制目标对象正文。

## 3. AgentRun 与提案契约

`AgentRunRequest` 在首次模型调用前冻结 `workspace_id`、`project_id`、`scope_refs[]`、`actor_ref`、`based_on_versions[]`、`allowed_tools[]`、`model_profile_revision`、`usage_cap`、`governance_evaluation_id`、`graph_id/version`、`input_contract_version`、`input_snapshot_hash`、`trace_id`。Runtime State 只保存小型引用与节点结果引用，不复制剧本或媒体正文。

`AgentRunResult` 必须包含 `run_id`、`sequence`、`graph_version`、`result_contract_version`、`input_snapshot_hash`、`result_hash`、`proposal_items[]`、`evidence_refs[]`、`usage`、`partial_errors[]`、`checkpoint_ref`。Worker 只通过结果端口提交；非法 Schema、run/version 不匹配、越界引用或重复 sequence 在业务副作用之前拒绝。

LangGraph 节点固定约束为：

```text
load_authorized_context
  → deterministic_checks
  → model_reasoning
  → validate_structured_output
  → enrich_evidence_and_impact
  → persist_result
```

Tool 只读且使用限定 run、scope、tool、expiry 的短期 capability token。所有写入意图只能变成 ProposalItem；图节点不能调用领域写 API、对象存储凭据或数据库。

## 4. 功能分解

| 能力 | 命令或查询 | 输入 | 输出 | 业务规则 | 失败恢复 | 切片 |
| --- | --- | --- | --- | --- | --- | --- |
| 创建受限 Run | `StartAgentRun`、`GetAgentRun` | 作用域、基线引用、Skill/graph 版本、Tool 白名单、用量上限 | AgentRun + Operation 引用 | 调模型前冻结输入/Agent 资源/M14 结论；相同业务幂等键回读原 Run | 模型不可用时 Run 失败但手工工作台可用；可以新 Run 重跑 | B |
| 运行进度与恢复 | `CancelAgentRun`、`ResumeAgentRun`、`ListAgentRuns` | run/revision、checkpoint sequence、取消原因 | 节点进度、覆盖率、部分错误、恢复动作 | checkpoint 只恢复 Run；终态 Run 不原地重启 | 租约过期后继续同一 Run/sequence；无安全 checkpoint 则以新 Run 重跑 | B |
| 提交结构化结果 | `CommitAgentRunResult` | run/version、sequence、input/result hash、Proposal envelopes | Run 结果、Proposal 批次、逐项错误 | Worker 结果只能追加；相同 run/sequence/result hash 幂等；不匹配为冲突 | 非法输出保留诊断且零领域副作用；部分 Tool 失败保留成功项 | B |
| 审阅逐项提案 | `ListProposalItems`、`PreviewProposalItem` | proposal/item、当前 actor、当前基线 | 说明/建议/提案/外部动作分类、证据、影响、资源、权限 | 必须展示目标模块预览；不把说明性回复展示为已执行 | 基线变化标记 expired，可重算或手工迁移，不删历史 | B |
| 决定与执行提案 | `Accept/AcceptWithChanges/Reject/DeferProposalItem` | item revision、可选的修改后目标 payload、决定幂等键 | ProposalDecision + 目标模块 result ref | M06 记决定信封；目标模块重新鉴权/校验/写入；修改后接受使用同一目标 Schema | 重复接受回读原结果；版本冲突返回 current 与重算入口；一项失败不阻断其他项 | B |
| 最小关系接管视图 | `GetDependencyTakeoverView`、`OpenAuthoritativeEditor` | project/scope、object filters | 实体—镜头—参考—候选—问题节点、真实边、projection as-of | 真实边从 M04 投影读取；编辑打开目标模块表单 | 投影滞后/连接中断时显示只读与回源动作 | B |
| 画布布局 | `SaveCanvasView`、`UpdateCanvasLayout`、`RemoveLayoutItem` | view revision、节点 refs、布局、可见性 | 个人/团队视图与新 revision | 删除节点只删布局项；装饰边不进入影响分析 | 布局冲突返回 current；断线保留本地草稿但不伪装已保存 | F |
| 批量接管 | `PreviewBatchCommand`、`SubmitBatchCommand` | 框选对象、目标 command、失败策略 | 目标/跳过/冲突清单、影响/资源、逐项结果 | 预览 hash 与提交绑定；仍逐项调用目标模块，不进行跨外部副作用的伪原子事务 | 部分失败显示安全重试项和人工处理项 | F |

## 5. 状态、并发与幂等

### 5.1 AgentRun 状态

| 当前状态 | 允许命令/证据 | 下一状态 | 不变量 |
| --- | --- | --- | --- |
| `queued` | Worker 领取 / 用户取消 | `running` / `cancelled` | 作用域、输入 hash、graph/contract 版本不再改 |
| `running` | 节点 checkpoint、部分结果、需人工输入、取消 | `waiting_user` / `partial` / `completed` / `failed` / `cancelled` | 节点 sequence 单调；结果只追加 |
| `waiting_user` | 授权输入、超时、取消 | `running` / `failed` / `cancelled` | 不保持数据库事务或 Worker 租约 |
| `partial` | 结果端口确认覆盖缺口 | 终态 | 已有 Proposal 仍可决定，未执行项明确标记 |
| `completed` / `failed` / `cancelled` | 新证据只可写诊断 | 终态 | 需重跑时创建新 Run，不原地复活 |
| 非终态 | 基线已被新批准版替代 | `superseded` | 已产生结果保留，Proposal 转 expired |

### 5.2 ProposalItem 状态

`pending → accepted | accepted_with_changes | rejected | deferred | expired`。`deferred` 可在未过期且基线未变时回到 `pending`；其他终态不原地修改。接受与目标写入由顶层用例在同一短事务中编排：目标模块先验证并写事实，M06 追加决定/回执，两者通过公开端口而不是跨表 SQL 完成。

### 5.3 幂等键与冲突

- Run 创建业务幂等范围：`workspace + actor + request_key + skill + input_snapshot_hash`；同键不同 hash 返回 `idempotency_conflict`。
- Worker 结果唯一：`agent_run_id + sequence`，再比较 `result_hash`；同 sequence 不同 hash 进入隔离和审计。
- Proposal 接受唯一：`proposal_item_id + decision_generation`；传给目标模块的命令幂等键从 `proposal_item_id + target_module + payload_hash` 稳定派生。
- 接受前重新读取 `based_on_refs[]`；任一 expected revision 不符返回 `proposal_baseline_expired`，不自动把 payload 迁移到新版。
- 画布布局使用 view revision 乐观锁；布局冲突不影响领域命令。

## 6. 页面与可观察状态

| 页面/区域 | 权威查询 | 必须显示的状态 | 可执行动作 |
| --- | --- | --- | --- |
| Agent 任务抽屉 | AgentRun + Operation | queued/running/waiting_user/partial/终态、当前节点、覆盖率、最近进展、用量/上限、部分错误 | 取消、继续人工输入、安全重跑、转手工工作台 |
| 提案审阅台 | Proposal + 目标模块预览 | 说明/建议/提案/外部动作分类、pending/终态/expired、基线差异、证据、影响、资源和权限 | 逐项接受、修改后接受、拒绝、暂缓、重算 |
| 最小关系接管视图 | M04 依赖投影 + 各模块批量 Query | 节点版本、blocked/unknown/stale、投影 as-of、权限禁用原因 | 选中、跳转权威表单、对受影响对象创建提案 |
| 完整画布 | CanvasView/Layout + 各模块 Query | 个人/团队可见性、布局修订、连接状态、未保存冲突 | 布局、缩放、分组、删除布局项、批量预览/提交 |

页面不从 RabbitMQ delivery、Worker 进程或 checkpoint 推断完成。可观测指标包括 Run 排队/节点耗时、Tool 失败率、checkpoint 恢复率、提案接受/修改/拒绝/过期率、非法输出拒绝数、人工接管次数和关闭 Agent 后任务完成率；日志只记录 ID、hash、版本和短摘要。

## 7. 失败、安全与恢复

| 错误码/场景 | 用户可见结果 | 恢复动作 |
| --- | --- | --- |
| `agent_model_unavailable` | Run failed/partial，手工工作台可用 | 稍后创建新 Run 或手工完成 |
| `agent_tool_partial_failure` | 逐项成功、失败与未执行清单 | 只为失败范围新建 Run |
| `agent_output_invalid` | 保留契约诊断，零领域副作用 | 修正输入、model profile 或 contract 后新建 Run |
| `agent_scope_denied` / `tool_token_expired` | 拒绝节点并审计 | 由用户重新授权，不自动扩大 scope |
| `proposal_baseline_expired` | 显示变化对象和风险 | 重新计算或在目标编辑器手工迁移 |
| `proposal_command_conflict` | 展示目标 current revision，该项未执行 | 刷新预览后重新决定 |
| `canvas_projection_stale` / 实时连接中断 | 只读、显示 as-of/离线 | 回源查询，重连后按版本刷新 |

来源文本一律视为数据，不能改变系统策略和 Tool 白名单。Tool 不获得数据库、对象存储、密钥或跨项目通配凭据。Checkpoint 使用 Agent Runtime 专用存储和凭据，生命周期跟随 Run 和 M14 保留策略。

## 8. 当前实现分类

| 分类 | 当前事实 | 设计处理 |
| --- | --- | --- |
| 已实现 | `backend/app/modules/agents/harness.py` 已有类型化 LangGraph 节点、结构输出验证和候选不直写领域的约束；`agents/contracts.py` 已有最小运行契约 | 保留“只产生候选”原则和现有 Fake model 测试资产 |
| 需改造 | 当前 Agent 由 I/O Worker 内联调用，产物分散在 `Task`、剧本提取候选和分镜草稿；常驻 checkpointer 明确尚未注入 | 用 AgentRun/Proposal 信封统一运行证据，但不迁移目标模块的 payload 所有权；与 M08 Operation/Outbox 对齐 |
| 新增 | 无持久 `agent_run/agent_proposal/agent_proposal_item`、目标命令端口、独立 Agent Worker/checkpoint 索引、最小关系接管页 | 切片 B 仅新增完成 P0 闭环所需的表、端口、Worker 与页面 |
| 暂缓 | 完整自由画布、多人实时布局、团队模板和批量框选 | 保持 Requirement 内切片 F 范围，不为其预建微服务、目录或兼容层 |

`scr_extraction_candidates` 等现有“候选”是 M03 领域对象，不等同 M06 的通用 AgentProposal；迁移时只复用人工决议和幂等模式，不合并事实所有权。

## 9. 逐 ID 验收映射

### 9.1 功能需求

| Requirement ID | 设计落点 | 最小可执行验收 |
| --- | --- | --- |
| AIC-FR-001 | 提案审阅台的四类回复标记 | 契约测试证明说明/建议不产生 command，外部动作始终显示待授权 |
| AIC-FR-002 | ProposalItem 信封的 target/baseline/evidence/impact/permission | Schema 缺任一必需引用时结果端口拒绝 |
| AIC-FR-003 | 逐项 Decision 状态机 | 10 项提案混合接受、修改、拒绝、暂缓后其他项不受影响 |
| AIC-FR-004 | `ExecuteProposedCommand` 目标模块端口 | 权限、revision、幂等或门禁任一不通过时领域事实不变 |
| AIC-FR-005 | Tool 只读、Worker 结果端口和模块边界 | 架构测试拒绝 Agent Runtime 导入领域 repository/数据库写端口 |
| AIC-FR-006 | AgentRun 全状态与 checkpoint 恢复 | 分别演练 waiting_user、partial、cancel、Worker 重启和 baseline superseded |
| AIC-FR-007 | Skill/Tool 白名单 + 目标模块 Command Registry | 剧本、实体、拆镜、阻塞解释、修复草案均只输出允许的类型契约 |
| AIC-FR-008 | `external_effect`/risk + M01/M14 重新鉴权 | 高消耗、外发、风险接受、主选、交付提案无显式授权均不执行 |
| AIC-FR-009 | 最小关系接管视图 | 从人物状态节点找到受影响镜头并打开 M05 修订表单 |
| AIC-FR-010 | 列表/画布/Agent 共用目标命令 | 三个入口对同一 revision 冲突返回同 error code/current revision |
| AIC-FR-011 | CanvasView/Layout 独立表 | 移动、缩放、分组、装饰连线后领域 hash 不变 |
| AIC-FR-012 | 批量 preview/submit 两阶段 | 提交前显示目标、跳过、影响、资源和失败策略，部分失败逐项可见 |
| AIC-FR-013 | view owner/visibility/revision | 个人与团队视图权限负向测试及所有者转移审计 |
| AIC-FR-014 | based_on_refs 重读与 expired 状态 | 引用版本变化后节点/提案显示过期且接受为零副作用 |
| AIC-FR-015 | AgentRunRequest 冻结字段 + M11 实际用量 | 调模型前能还原 scope/input/tool/外发/上限，完成后可查模型与 Tool 用量 |

### 9.2 非功能需求

| Requirement ID | 设计落点 | 最小可执行验收 |
| --- | --- | --- |
| AIC-NFR-001 | Agent 入口与手工命令解耦 | 禁用 Agent Worker 后 M03—M10 P0 手工命令契约测试继续通过 |
| AIC-NFR-002 | 版本化 result/proposal/target command Schema | 未知版本、多余字段和非法 payload 在写业务事实前拒绝 |
| AIC-NFR-003 | 最小字段 Query、hash 日志、checkpoint 保留 | 日志/事件/审计扫描不含来源正文、Token 或凭据 |
| AIC-NFR-004 | 画布切片 F 容量 Gate | 真实规模节点/边下渲染、内存、键盘可访问性达到签认基线才启用 |
| AIC-NFR-005 | 所有 P0 的列表/表单入口 | 仅键盘、不看颜色且不使用拖拽也能完成全部 P0 场景 |
| AIC-NFR-006 | 提案决定/修改差异指标 | 仪表盘分别统计正确、修改、拒绝和业务结果，不用对话满意度代替 |

### 9.3 验收条件

| Requirement ID | 设计落点 | 自动化/故障演练 |
| --- | --- | --- |
| AC-AIC-001 | 逐项提案审阅 | Fixture 生成 10 个镜头提案，混合接受/修改/拒绝并校验逐项回执 |
| AC-AIC-002 | 目标 CommandPort | 同一 payload 从 Agent 与 M05 表单执行，比较领域结果 hash |
| AC-AIC-003 | 信封/Schema/鉴权/基线/幂等门禁 | 非法、重复、越权、过期四类负向测试校验业务表无变化 |
| AC-AIC-004 | Agent 媒体意图只生成 M07 payload | 测试 Agent 无法创建 M08 Job，只得到待批准 GenerationPlan 草稿 |
| AC-AIC-005 | 手工闭环 | 停止 Agent Worker 后读取已有结构/实体/镜头并继续规划成功 |
| AC-AIC-006 | M04 真实依赖投影 + M05 跳转 | 人物状态变化 Fixture 仅标出真实引用镜头并打开修订 |
| AC-AIC-007 | Layout 删除语义 | 删除画布节点后校验实体/镜头/候选表及引用不变 |
| AC-AIC-008 | 共用目标命令错误契约 | 画布、列表、API 并发修改同一 revision，比较 code/current revision/recovery action |
| AC-AIC-009 | checkpoint + result 幂等 | 在每个 LangGraph 节点强制重启，同 Run 恢复且 ProposalItem 数量/hash 不重复 |

## 10. 验证与交付顺序

切片 B 只实现受限 AgentRun、逐项 Proposal、共用领域命令和最小关系接管视图。测试使用 Fake model/tool 覆盖图路由、非法输出、重复结果、每节点重启、过期基线和关闭 Agent；契约测试保证接受提案与手工命令同结果。切片 F 只在真实规模性能与可访问性 Gate 通过后交付完整画布，不改变前述事实所有权。
