# M06 Agent 与可视化画布详细设计

> Design ID：DES-M06
> Requirement：[REQ-AIC](../../requirement/006-M06-Agent与可视化画布需求.md)
> 状态：proposed
> 剧本分析业务门禁：[M02](./002-M02-项目组合与内容单元详细设计.md)、[M03](./003-M03-导入与叙事理解详细设计.md)、[M04](./004-M04-生产知识与一致性图谱详细设计.md)、[专题 004](../004-AI视频生产平台剧本基础分析与人物拆解详细设计.md)

## 1. 设计结论

LangGraph 只编排一次 AgentRun 内部推理；业务 Operation、批准和 current 状态固定由 Go Service 与 PostgreSQL 管理。Go M06 对业务变更只拥有 `AgentRun`/`AgentProposal`/`ProposalItem` 的运行与提案信封，不拥有信封内的领域命令语义和接受后写入。信封中的 `target_module + command_type + command_schema_version + payload/hash` 由 Go M03、M04、M05、M07 或 M10 等目标模块定义；用户接受时必须重新鉴权并调用目标 Go 命令端口。

代码上明确分离 `backend/src/agents` 与顶层 `agent/`：前者是 Go M06 业务模块，后者是 Python、无 Kafka/Redis/Elasticsearch/业务写权限且不提供公共 API/Ingress 的内网计算微服务。Agent 文件可以承载完整的剧本、人物和生产需求 **推理编排**，但不能承载权威 **业务编排**；Go M03 拥有 Manifest/Narrative 批准与受控检索，Go M02 拥有物化与顺序，Go M04 拥有人物身份、出场投影和生产需求 current。

M06 另外拥有 `CanvasView` 和 `CanvasLayout` 等展示状态，但不拥有节点代表的领域对象或业务依赖。Agent、列表、表格和画布最终提交同一目标命令，不为画布或 Agent 建第二套业务 API。Go M06 是模块化单体内的逻辑边界，不拆为领域微服务；只有无业务事实的 Python Runtime 是独立内网计算微服务。

## 2. 事实所有权与跨模块契约

### 2.1 M06 拥有与不拥有的事实

| 类别 | M06 拥有 | M06 不拥有 |
| --- | --- | --- |
| Agent 运行 | Run 身份、冻结作用域、graph/contract 版本、状态、result hash、checkpoint 引用 | 业务 Operation 的完成语义、生成 Job、目标对象 current |
| Proposal / Advisory | 可执行提案批次/信封、非可执行说明/建议/validation gap、基线/证据引用、决定和执行回执引用 | `command_payload` 的领域语义、目标对象写入、风险例外和批准规则 |
| 会话 | 受保留策略控制的 instruction/session 索引 | 从会话推断的正式人物、镜头、计划或决定 |
| 画布 | 视图所有者、可见性、过滤器、节点位置/大小/分组、装饰边 | 节点领域内容、真实依赖边、执行顺序、对象删除语义 |

`AgentProposalItem` 的最小信封为：`item_id`、`agent_run_id`、`target_module`、`command_type`、`command_schema_version`、二选一的 `command_payload | command_payload_ref`、`payload_hash`、`payload_size`、`based_on_refs[]`、`conflict_scope_keys[]`、`evidence_refs[]`、`change_summary_ref`、`confidence`、`uncertainty_refs[]`、`conflict_refs[]`、`impact_summary_ref`、`usage_estimate_ref`、`required_capabilities[]`、`external_effect`、`expires_at`。M06 只校验信封结构、大小、引用范围和 hash；大 payload 使用绑定 workspace/run、不可变且内容寻址的受限引用，不进入 Kafka/checkpoint。目标模块版本化并校验 payload，M06 不会对字段做领域级补默认值或迁移。`confidence` 必须注明校准方法/版本，未知时显式为空并给出不确定原因，不能伪造精度；`change_summary_ref` 必须能由目标模块 Preview 展开为字段级 diff。`conflict_scope_keys` 只是供预览和路由使用的声明；接受时必须由目标模块根据 Command 重新计算实际 read/write set，不能信任模型声明。

`AgentAdvisoryItem` 专门表达不能执行的 `explanation | suggestion | validation_gap | schema_change_suggestion | deferred_dependency`，最小字段为 `advisory_id`、`run_id`、稳定 `logical_key`、`generation`、可选 `supersedes_id`、`kind`、`code`、`summary`、`based_on_refs[]`、`evidence_refs[]`、`scope_refs[]`、`recommended_actions[]`。它没有 `target_module/command/payload`，只能查看、以 append-only Resolution 消除/标 stale，或打开人工工作台，不能 Accept。同一 Run 跨 Result sequence 的同 logical key/hash 回读既有项，并为每个包含它的 Result 追加独立 membership；内容变化必须创建 successor generation 并显式 supersede，不能重复制造同一 blocker。Result hash/count 按该 sequence 的有序 membership 计算，不按本次新建 Advisory 行数计算。非可执行结果不得伪装为 AgentProposalItem。

基线分成两层，不共用一个“全局 current hash”完成并发判断：

- `AgentRunRequest.input_snapshot_hash/based_on_versions` 冻结 Run 的只读复现快照，说明模型当时看到了什么；它用于审计、重跑和判断是否应创建 successor Run，不因用户接受同一 Run 的一项提案而改写。
- 每个 `AgentProposalItem.based_on_refs` 只包含该 Command 实际依赖的不可变输入和可变语义键 expected revision；目标模块在 Preview/Accept 时从 payload 重算实际 read/write set，并只对这些键做 CAS。全局 Narrative、CoverageSchema 等真实共同依赖变化会使相关项过期；同一 Proposal 内不相交人物、Mention、ContentUnit slot 或 coverage subject 的写入不会互相过期，相交写入才返回冲突。

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

目标 Go 模块通过公开只读 `CommandSchemaRegistry` 发布唯一的 `CommandSchemaDescriptor(target_module, command_type, version, json_schema, schema_hash)`、golden fixture 与 `ValidateAndPreviewProposedCommand` 端口；当前描述文件归档在 `backend/contracts/agent/commands/`。M06 创建 Run 时解析并冻结 descriptor/hash；Python Runtime 只接收进程边界中的 JSON Schema descriptor，不导入 Go Service、database row 或内部类型。`agent/.../skills/<skill>/contracts.py` 只定义候选、中间 State、Advisory 和 Result 映射，不得复制 `CreateEpisodeBreakdownDraft`、`ResolveMentionGroup` 等目标 Command DTO。Harness 按冻结 Schema 校验模型输出，目标 Go 模块仍是 canonical Schema、fixture 和语义校验的唯一所有者；架构测试比较 registry/hash 并禁止 Skill 自建同名 Command 类型。

### 2.3 Agent Proposal 与模块原生 Proposal 不双写

`script_analysis` 节点中的 `emit_*_proposal_items` 一律表示 **创建 M06 `AgentProposalItem`**，其 payload 直接是目标模块已经发布的版本化 Command；ResultPort 只提交 M06 Run/Proposal 信封，不先在 M03/M04 再创建一份同义 Proposal。用户接受后，M06 调用 `ExecuteProposedCommand`，目标模块只保存命令产生的领域 Revision/Decision，M06 通过 `target_result_ref` 记录回执。

M03 自有的 `NarrativeProposal/ProposalItem/DecideProposalItem` 继续服务 ImportRun、确定性解析或模块原生候选，不与 M06 的 ID、状态机或决定记录复用。两类候选可以在前端聚合展示，但必须标记 `origin=module_native | agent`，并分别回到自身权威命令；不得为一个 Agent 结果同时创建 M06 和 M03 Proposal，也不得用 M03 Proposal 状态推断 AgentProposal 已接受。若目标模块需要保留来源，只保存只读 `origin_ref=agent_proposal_item_id`，不复制 M06 信封或状态。

切片 B 不实现 Proposal 间 future-ID 替换或隐式依赖 DAG。每个可执行 payload 只能引用 AgentRun 启动时已经存在的权威 ID，或使用目标模块发布的自包含原子 Command 在同一事务内创建并消费新 ID。breakdown stage 产生一个 `CreateEpisodeBreakdownDraft` aggregate item；narrative stage 按稳定 ContentUnit 产生 `ApplyNarrativeContentUnitDraft` item，并在 M03 私有 building revision 中独立落片，全部处理后由 M03 `FinalizeNarrativeDraftImport` 校验并切换为可见 draft。knowledge stage 只允许 M04 `ResolveMentionGroup` 和 `DecideCoverageWithRequirements` 等自包含 Command。任何必须依赖另一 item 执行结果的建议都写为 `AgentAdvisoryItem(kind=deferred_dependency)`，待权威结果存在后创建 successor Run，不把临时 ID 塞进 payload。

## 3. AgentRun、Harness、Skill 与提案契约

### 3.1 四段责任

| 层次 | 目标位置 | 责任 | 输出/事实 |
| --- | --- | --- | --- |
| Go M06 Service | `backend/src/agents/service.go` | 创建/取消 Run、冻结授权输入、记录 Proposal/Decision、重新鉴权并转交目标命令 | AgentRun、Proposal、Decision、Operation 引用 |
| Python 通用 Harness | 私有 `agent/src/main.py` FastAPI 计算微服务 | 经内部 HTTP 接受 AgentRun、graph/contract 版本、节点调度、Tool policy、预算/超时、checkpoint、恢复和结构校验 | 版本化 AgentRunResult；无 Kafka/Redis/Elasticsearch/公共 API，不拥有领域事实 |
| Python 具体 Skill | `agent/src/*.py`（需要时新增直接文件） | State、Node、Prompt、确定性合并、覆盖校验和提案映射 | 带 evidence/baseline/failed scope 的 ProposalItem |
| Go 目标模块 Service | M03/M04/M05/M07/M10 `backend/src/<module>/service.go` | 权限、不变量、expected revision、幂等、短事务、Outbox 与事实写入 | 目标领域 Result 与 revision；M02 仅消费 M03 approved Manifest，不接受 Agent Proposal |

目标文件结构如下；只在切片 B 的真实用例落地时创建，不能先生成空 Skill：

```text
backend/
  contracts/agent/                # Go↔Python 唯一当前 JSON Schema
    commands/
  src/agents/                     # Go M06 业务事实；Model/Service/Repository/Controller 同包
    model.go
    service.go
    repository.go
    controller.go

agent/src/                       # 私有 Python Service；无 Kafka/Redis/Elasticsearch、公共 API、业务数据库或通用 MinIO 权限
  main.py                         # FastAPI/Uvicorn；仅 start/get/cancel/health
  contracts.py                    # 仅候选/中间 State/Advisory；禁止复制目标 Command DTO（按需创建）
  harness.py                      # 按需创建的 Harness/Skill 实现
```

### 3.2 Run 与 Result 信封

`AgentRunRequest` 在首次模型调用前冻结 `workspace_id`、`project_id`、统一用户/服务发起者 `initiator_principal_ref`、可选 `actor_ref`、执行 `service_identity_ref`、`scope_refs[]`、`based_on_versions[]`、`allowed_tools[]`、`allowed_command_schema_descriptors[]/set_hash`、`model_profile_revision`、`usage_cap`、`governance_evaluation_id`、`skill_id/version/stage/stage_generation`、可选 `parent_workflow_ref`、`root_operation_id`、该 Run 的 `operation_id`、`graph_id/version`、`input_contract_version`、`result_contract_version`、`input_snapshot_ref/hash`、`trace_id`、`request_idempotency_key/request_hash`。request hash 覆盖除 trace/时间外全部不可变语义字段。Runtime State 只保存小型引用与节点结果引用，不复制剧本或媒体正文。

`AgentRunResult` 必须包含 `run_id`、`stage/stage_generation`、`sequence`、`graph_version`、`result_contract_version`、`input_snapshot_hash`、`allowed_command_schema_set_hash`、`result_hash`、`proposal_items[]`、`advisory_items[]`、`evidence_refs[]`、`usage`、`partial_errors[]`、`checkpoint_ref`。Agent Service 先把结果持久化到 checkpoint/result store，再由 Go `AgentExecutorPort.GetRun` 拉取并经结果端口提交；非法 Schema、run/stage generation/version 不匹配、越界引用、Command Schema set 漂移或重复 sequence 在业务副作用之前拒绝。

#### 3.2.1 私有微服务调用与单一消息总线

Go M06 只依赖 `AgentExecutorPort.StartRun/GetRun/CancelRun`；唯一首期 adapter 位于 backend，使用 private HTTP/JSON 调用 Python FastAPI/Uvicorn Service。平台唯一 Kafka 集群只把 Agent invoke/poll 作为 `lanverse.tasks.operation` 中的 Go Task 投递给 operation-worker；Python 不安装 Kafka/Redis client。Agent 不注册公共 OpenAPI、公共域名或 Ingress，生产关闭 FastAPI 的 docs/schema 路由；frontend、用户和第三方均不能获得 Agent 地址。

`POST /internal/agent-runs` 仅在 checkpoint store 持久化 `run_id + request_idempotency_key + request_hash + accepted` 后返回；同 key/hash 回读既有 Run，异 hash 冲突。`GET /internal/agent-runs/{run_id}` 返回持久化进度/Result，`POST /internal/agent-runs/{run_id}:cancel` 幂等请求取消。Go 在 Kafka Inbox 接管 Task 并提交 offset 后才调用 start；HTTP 超时只以同 key/hash 重试或 GET 对账。accepted/running 时写 PostgreSQL `waiting_external + available_at`，Scheduler 重投同一 operation Task 后再次查询，不保持长连接、不占 Kafka partition。

网络权限固定为：Agent 只接受 operation-worker 的 mTLS/服务身份，可访问 checkpoint store、模型 allowlist，以及 backend 签发的 run-scoped 只读 Tool、ModelCallGate 和载荷 capability；不得访问公共 Go API、Kafka、Redis、Elasticsearch、业务数据库、通用 MinIO 或媒体 Provider 网络。backend 是消息、跨服务契约、检索、重试、限流和死信的唯一 owner。

通用 Harness 的固定执行外壳为：

```text
validate_run_envelope
  → load_authorized_snapshot_refs
  → execute_versioned_skill_graph
  → validate_result_contract_and_scope
  → persist_result_for_backend_pull
```

Tool 只读且使用限定 run、scope、tool、expiry 的短期 capability token。所有写入意图只能变成 ProposalItem；Python 图节点不能调用领域写 API、业务数据库或通用对象存储凭据。

剧本检索 Tool 固定调用 Go M03 `SearchApprovedNarrative`，capability 绑定 run/workspace/project/exact approved revision set、允许 filter、最大结果数和 expiry。Go 重新鉴权并强制注入租户/修订范围，返回 node/scene/content-unit refs、高亮短片段和 SourceAnchor；Python 不获得 Elasticsearch 地址、索引名、query DSL 或 API key。搜索 stale/unavailable 时 Tool 返回具名 partial error，Agent 不把零结果解释为剧本中不存在该人物/资产。

每次模型调用前，Agent 必须调用 backend 内部 `ModelCallGate`，由 Go 同时执行 `DistributedRateLimiterPort` 的 workspace/model-profile GCRA 与 M11 UsageGate，并返回单次、限 run/model/expiry 的调用 capability。Redis 不可用或 UsageGate 阻止时不调用模型，Run 保持可恢复等待；Python 不直接读取 Redis 计数或 M11 表。

大型 Proposal payload 通过 Go M06 受限 `StageResultPayloadPort` 暂存：Python Agent Service 以 `run/item/request_idempotency_key/request_hash` 请求绑定 `workspace/run/item/content_type/max_bytes/expected_hash/expires_at` 的一次性 MinIO PUT capability，上传并由 Go ResultPort 校验大小/hash 后，M06 才登记不可变 `payload_ref`。capability/request 唯一；提交成功但响应丢失时，`GetStageResultPayloadByCapability` 或重复同 key/hash 必须回读原 ref，异 hash 返回幂等冲突。Python Agent 不获得通用 MinIO、桶级或 root 凭据；唯一 MinIO adapter 位于 Go `backend/src/platform/objectstorage`，引用不进入 Kafka/checkpoint，`CommitAgentRunResult` 只提交 ref/hash。ref 保留期至少覆盖 Proposal 的审阅、过期和执行审计期限；上传失败、取消或未被有效 Result 引用的对象由具名 orphan cleanup Operation 清理，已被 Proposal/Decision 引用的 payload 不按孤儿删除。

对于 `script_analysis`，`parent_workflow_ref` 固定为 M03 `AnalysisRun`，`root_operation_id` 指向该整本分析的根 Operation。breakdown、narrative、knowledge 三个 M06 AgentRun 各自创建一个 `parent_operation_id=root_operation_id` 的 child Operation；child 只表达单 stage 的排队、运行、恢复和结果提交。child 完成后根 Operation 进入 `waiting_user` 并展示对应批准门禁；批准后由业务编排器创建下一 stage 的新 AgentRun/child Operation。Agent Service 无权把根 Operation 标记 completed；只有 M03/M02/M04 的门禁和最终投影都满足后，根 AnalysisRun/Operation 才能完成。

### 3.3 `script_analysis` Skill

首个 Skill 负责整本剧本的拆集、叙事结构、人物和生产需求候选，但使用同一 Skill 定义中的三个独立 AgentRun stage。拆集批准与叙事批准是两个业务门禁，不能藏在一个 Run/checkpoint 内：

1. `breakdown` 只以 SourceRevision 为基线，输出 M03 EpisodeBreakdown Proposal；
2. M03 批准 Manifest、M02 物化 ContentUnit/Order 且 M03 接收 mapping 后，`narrative` 才能按稳定 ContentUnit 输出 M03 Scene/Beat/Mention Proposal；
3. M03 批准 NarrativeRevision 后，`knowledge` 才能以 approved Narrative、Order 和当前 M04 知识为基线输出 M04 Proposal。

```text
stage=breakdown
load_source_snapshot
  → deterministic_precheck_and_chunk
  → extract_episode_boundaries
  → validate_breakdown_coverage
  → emit_agent_proposal_item(command=M03.CreateEpisodeBreakdownDraft)

人工决定 → M03 批准 EpisodeBreakdownManifest
  → M02 原子物化 ContentUnit / OrderRevision
  → M03 接收 temporary key → ContentUnit mapping

stage=narrative（新 AgentRun）
M03.StartNarrativeDraftImport → draft_import_id / private building revision
load_source_manifest_mapping_and_order_snapshot
  → parallel_extract_scene_beat_mentions_by_content_unit
  → merge_and_normalize_narrative_candidates
  → validate_narrative_schema_evidence_coverage
  → emit_agent_proposal_items(command=M03.ApplyNarrativeContentUnitDraft)

人工逐集决定 / 手工补齐 → M03.FinalizeNarrativeDraftImport
  → M03 独立校对并批准 NarrativeRevision

stage=knowledge（新 AgentRun）
load_approved_narrative_and_knowledge_snapshot
  → cluster_character_identity_proposals
  → propose_mention_resolution_and_coverage
  → derive_character_episode_occurrence_evidence
  → suggest_production_requirements
  → validate_knowledge_schema_evidence_coverage
  → emit_agent_proposal_items(commands=M04.ResolveMentionGroup|DecideCoverageWithRequirements)

人工决定 → M04 确定性重建 Occurrence / Inventory / Readiness
```

| stage / 节点 | 输入 | 输出 | 关键约束 |
| --- | --- | --- | --- |
| breakdown / `load_source_snapshot` | SourceRevision 与 capability | 冻结 source refs/hash | 只读授权范围；正文不复制到 checkpoint |
| breakdown / `deterministic_precheck_and_chunk` | 文档结构、页/段/锚点 | 边界候选分片与错误范围 | 确定性规则先行；超限显式分片，不静默截断 |
| breakdown / `extract_episode_boundaries` | 有锚点分片 | EpisodeBreakdown 候选、临时 key、边界证据 | 非空来源唯一覆盖或具名 gap；不创建 ContentUnit ID |
| breakdown / `emit_agent_proposal_item` | 已验证完整边界候选集 | 一个 M06 AgentProposalItem：`M03.CreateEpisodeBreakdownDraft` | 自包含全部候选/coverage/local keys，可修改后接受；不创建 M03 原生 Proposal；Agent 无 M02 target |
| narrative / `load_*_mapping_*` | Source、approved Manifest、M02 mapping/Order、M03 draft_import/building revision | 冻结 narrative snapshot/hash | mapping 必须与 Manifest hash 完全匹配，不能按序号猜 ContentUnit ID；普通查询看不到 building revision |
| narrative / `parallel_extract_*` | 按稳定 ContentUnit/完整场次切分的来源 | NarrativeScene、Beat、Mention 候选 | 每个候选绑定 ContentUnit 与 Anchor；单分片失败不丢成功分片 |
| narrative / `merge_and_normalize_*` | 全部分片结果 | 去重候选、冲突集、GlobalOccurrenceIndex 构建输入和覆盖统计 | 只做 M03 候选归并；正式索引由 M03 从 draft/approved 结构重建，不创建 M04 实体 |
| narrative / `emit_agent_proposal_items` | 已验证叙事候选 | 每个稳定 ContentUnit 一个 M06 AgentProposalItem：`M03.ApplyNarrativeContentUnitDraft` | 每项自包含该集完整 Scene subtree/coverage/local keys；超 inline cap 使用不可变 payload_ref；item 只 CAS 对应 `(draft_import, content_unit)` slot，不以整个 building revision 的每次变化使其他集过期；M03 每集短事务落入私有 building revision，不创建 M03 原生 Proposal |
| knowledge / `load_approved_*` | approved Narrative、ContentUnit/Order、M04 current refs | 冻结只读 knowledge reproducibility snapshot/hash | 快照必须已经存在且可读；M03 Narrative/CoverageSchema 等共同基线变化使相关 Proposal 过期，但该 Run 的快照不是所有 item 共用的全局 CAS token |
| knowledge / `cluster_character_identity_proposals` | approved ProductionElementMention 与知识快照 | ProductionEntity/UnresolvedSubjectRevision 聚类建议 | 不凭名称直接合并人物；不覆盖人工 MentionResolution |
| knowledge / `propose_mention_resolution_and_coverage` | 聚类建议、Mention 与 CoverageSchema | MentionResolution/CoverageDecision 提案 | 缺失、重叠和冲突保持显式，不能默认创建/链接 |
| knowledge / `derive_character_episode_occurrence_evidence` | Mention、Resolution 提案、Order | 非权威 `OccurrenceEvidence` | 不复用 M03 GlobalOccurrenceIndex 名称，不输出 current；未知/未分析/失败不能标成缺席 |
| knowledge / `suggest_production_requirements` | ProductionElementMention、实体建议、NarrativeScene 与 CoverageSchemaRevision | 人物、地点、道具、服化、特效、声音等 ProductionRequirementRevision 提案 | 未支持类型只形成具名 validation gap/schema-change suggestion，不生成目标 Command；Inventory 以 nullable decision + `unassessed_reason` 表达未评估成员 |
| knowledge / `validate_*` | 全部候选、证据与失败 | 合法项、冲突、缺口、`failed_scopes` | 校验 based-on 版本、证据可达、全剧覆盖和 Contract |
| knowledge / `emit_agent_proposal_items` | 已验证知识候选 | 0..N 个 M06 AgentProposalItem：`M04.ResolveMentionGroup` 或 `M04.DecideCoverageWithRequirements` | 每项自包含且无 future ID；只携带实际 Mention/group/entity 或 coverage subject 的 expected refs/conflict keys；M04 重算 read/write set，互不相交项可逐项接受；不创建 M04 原生 Proposal；不输出投影写命令 |

每个结果至少携带 `stage`、Run 级 `based_on_versions[]`、逐项 `based_on_refs[]/conflict_scope_keys[]`、`source_anchor_refs[]`、适用剧集/场景、confidence、conflicts、coverage、`failed_scopes[]` 和目标 Command Schema 版本。模型或 Tool 部分失败时 Run 可以是 `partial`，成功项仍可审阅，但 UI 必须同时显示未分析和失败范围。`FinalizeNarrativeDraftImport` 只有在 mapping 内每个 ContentUnit 的 item 已成功应用或由人工补齐、没有 failed scope，且整本 coverage 校验通过时才把 building revision 切换为可见 draft；拒绝/暂缓 Agent item 本身不代表该集可以留空。后一个 Run 不得引用前一个 Run 未接受的 Candidate ID；Manifest、mapping、Order、Narrative 或 item 实际依赖的 M04 基线变化会使相关 Run/Proposal 过期，需要以新 Run 重算，不能在 checkpoint 中偷换基线。

### 3.4 接受后的权威路径

```text
SourceRevision
  → StartAgentRun(stage=breakdown)
  → M06 AgentProposal(M03.CreateEpisodeBreakdownDraft)
  → AcceptProposalItem → M03 创建 breakdown draft
  → M03 独立校对 / ApproveEpisodeBreakdown → approved EpisodeBreakdownManifest
  → M02 原子物化 ContentUnit / OrderRevision → M03 接收 mapping
  → M03 StartNarrativeDraftImport → private building revision
  → StartAgentRun(stage=narrative, based_on=Manifest + mapping + Order)
  → M06 AgentProposalItems(M03.ApplyNarrativeContentUnitDraft)
  → 逐集决定 / 手工补齐 → M03 FinalizeNarrativeDraftImport → narrative draft
  → M03 独立校对 / ApproveNarrativeRevision → approved NarrativeRevision
  → StartAgentRun(stage=knowledge, based_on=approved Narrative + Order + M04 current)
  → M06 AgentProposal(M04 atomic Commands) → 人工逐项决定 → M04 Service 写入批准事实
  → M04 Service 确定性重建 EntityOccurrence / Inventory / Readiness
```

`AcceptProposalItem` 只表示接受 Agent 建议并执行目标 draft/knowledge Command，不等于 `ApproveEpisodeBreakdown` 或 `ApproveNarrativeRevision`；接受建议与批准内容是两个可分离 capability，批准者必须在 M03 工作台重新查看 coverage/conflict 后单独提交。M03 拥有 SourceRevision、EpisodeBreakdownManifest、NarrativeDraftImport、NarrativeScene、Beat、ProductionElementMention、Anchor 和 approved NarrativeRevision；M02 只拥有 ContentUnitMaterialization、稳定 ContentUnit、OrderRevision 与返回给 M03 的 mapping；M04 拥有 ProductionEntity、UnresolvedSubjectRevision、MentionResolution、ProductionRequirementRevision、ProductionRequirementInventoryProjection、RequirementReadinessProjection 和 EntityOccurrenceProjection。人物出场集数不是模型独立批准的字段，而是 M04 根据 approved Narrative、Order、UnresolvedSubjectRevision/MentionResolution 确定性重建的带版本投影。M06 只保留三个 Run 及其 Agent Proposal 信封。切片 A 的手工表单直接调用相同 M03/M04 Command，必须在 Agent Service 完全关闭时仍可完成，并与接受 Agent Proposal 后得到相同领域结果。

## 4. 功能分解

| 能力 | 命令或查询 | 输入 | 输出 | 业务规则 | 失败恢复 | 切片 |
| --- | --- | --- | --- | --- | --- | --- |
| 创建受限 Run | `StartAgentRun`、`GetAgentRun` | 作用域、基线引用、Skill/graph/stage/stage_generation、可选 parent workflow/root Operation、Tool/Command Schema 白名单、输入/结果契约、用量上限 | AgentRun + 自身 Operation 与可选 root 引用 | 调模型前冻结 input snapshot/ref/hash、允许 Command descriptor/set hash、Agent 资源和 M14 结论；有 parent 时本 Run Operation 必须挂 root；相同业务幂等键回读原 Run | 模型不可用时 Run 失败但手工工作台可用；可以用新 stage generation 新建 successor Run | B |
| 运行进度与恢复 | `CancelAgentRun`、`ResumeAgentRun`、`ListAgentRuns` | run/revision、checkpoint sequence、取消原因 | 节点进度、覆盖率、部分错误、恢复动作 | checkpoint 只恢复 Run；终态 Run 不原地重启 | 租约过期后继续同一 Run/sequence；无安全 checkpoint 则以新 Run 重跑 | B |
| 暂存大型结果载荷 | `CreateStageResultPayloadCapability`、`CommitStageResultPayload`、`GetStageResultPayloadByCapability` | workspace/run/item、请求 key/hash、content type、max bytes、expected hash、expiry、流式 bytes | immutable payload_ref/hash/size/retention | capability/request 唯一、短期、限作用域；同 key/hash 或已消费 capability 回读原 ref，异 hash 冲突；服务端校验后登记；Agent 无通用对象存储凭据 | 超限/hash 不符零 Proposal 副作用；响应丢失可安全查询/重放；取消/未引用对象进入 orphan cleanup，已引用对象不清理 | B |
| 提交结构化结果 | `CommitAgentRunResult` | operation-worker 从 Agent Service 拉取的 run/version、sequence、input/result hash、Proposal/Advisory envelopes | Run 结果、Proposal 批次、Advisory 清单、逐项错误 | Service 结果只能追加；先解析 payload/ref 并调用目标模块 `ValidateAndPreviewProposedCommand`，由 Go 生成 canonical field diff、实际 based-on/read/write set 与 hash 后，才在同一 M06 事务持久化不可变 Item；相同 run/sequence/result hash 幂等；Advisory 按 logical key/hash 创建或回读，并始终为当前 Result 追加有序 membership，不进入可接受状态机 | HTTP 超时先 GET 对账；payload/Schema/Preview 非法时保留诊断且零领域副作用；部分 Tool 失败保留成功项和具名 gap | B |
| 审阅逐项提案 | `ListProposalItems`、`ListAdvisoryItems`、`PreviewProposalItem` | proposal/item、当前 actor、当前基线 | 说明/建议/validation gap、可执行提案/外部动作分类、字段级 diff、置信与不确定项、证据、影响、资源、权限 | 必须由目标模块生成字段级 Preview；Advisory 只能查看、消除或打开人工工作台，不把说明性回复展示为已执行 | 实际依赖基线变化才标记 expired，可重算或手工迁移，不删历史 | B |
| 决定与执行提案 | `Accept/AcceptWithChanges/Reject/Defer/UndeferProposalItem` | item hash、expected latest decision generation、可选修改后目标 payload、决定请求幂等键 | append-only ProposalDecision、接受动作的唯一逻辑 ProposalExecution 与逐 attempt Receipt/result ref | M06 以 expected generation 做 CAS 并记录 request key/hash；accept 使用原 payload，accept_with_changes 的有效 payload inline/ref 二选一；reject/defer/undefer 禁止 payload/read-write set 且不执行目标 Command；目标模块重新鉴权/校验/写入 | 同请求键同 payload 回读；同键不同 payload 返回 idempotency conflict；并发不同决定仅一个追加成功；Execution 的 decision/command key 唯一，Receipt 只以 execution/attempt 唯一且所有重试复用同一 command key；一项失败不阻断其他项 | B |
| 处置不可执行建议 | `ResolveAdvisoryItem`、`MarkAdvisoryStale` | advisory item hash、expected latest resolution generation、外部事实 ref、理由、请求 key/hash | append-only AdvisoryResolution 与派生 resolved/stale 状态 | 不能 Accept 或执行 Command；同 request key/hash 回读，expected generation CAS；gap 再现创建 successor Advisory generation，不 reopen/覆盖旧内容 | 外部事实已变返回 stale/current；并发处置只成功一个；历史说明和证据始终可读 | B |
| 最小关系接管视图 | `GetDependencyTakeoverView`、`OpenAuthoritativeEditor` | project/scope、object filters | 实体—镜头—参考—候选—问题节点、真实边、projection as-of | 真实边从 M04 投影读取；编辑打开目标模块表单 | 投影滞后/连接中断时显示只读与回源动作 | B |
| 画布布局 | `SaveCanvasView`、`UpdateCanvasLayout`、`RemoveLayoutItem` | view revision、节点 refs、布局、可见性 | 个人/团队视图与新 revision | 删除节点只删布局项；装饰边不进入影响分析 | 布局冲突返回 current；断线保留本地草稿但不伪装已保存 | F |
| 批量接管 | `PreviewBatchCommand`、`SubmitBatchCommand` | 框选对象、目标 command、失败策略 | 目标/跳过/冲突清单、影响/资源、逐项结果 | 预览 hash 与提交绑定；仍逐项调用目标模块，不进行跨外部副作用的伪原子事务 | 部分失败显示安全重试项和人工处理项 | F |

### 4.1 耐久事件契约

- M06 消费 M03 `narrative.analysis_stage.requested`，按 `analysis_run_id + stage + stage_generation + input_hash` Inbox 幂等创建 AgentRun 与 child Operation；消息只含冻结引用、哈希和 root Operation ID，不含剧本正文。
- M06 在 Run/child Operation 索引提交后发布 `agent.run.created(run_id, skill_id/version/stage/stage_generation, parent_workflow_ref, operation_id, root_operation_id, input_hash)`。
- M06 在 `CommitAgentRunResult` 事务提交后发布 `agent.run.result_committed(run_id, parent_workflow_ref, stage, stage_generation, sequence, status, result_hash, failed_scopes)`；重复 result 不重复发布逻辑事件。
- M03 按 event ID 与 run/sequence 幂等消费上述两个 M06 事件。M06 不发布“根 AnalysisRun 已完成”；取消或 supersede 后的迟到 Result 仍可审计，但不能推进 M03 根流程。

## 5. 状态、并发与幂等

### 5.1 AgentRun 状态

| 当前状态 | 允许命令/证据 | 下一状态 | 不变量 |
| --- | --- | --- | --- |
| `queued` | Agent Service durable accept / 用户取消 | `running` / `cancelled` | 作用域、输入 hash、graph/contract 版本不再改 |
| `running` | 节点 checkpoint、部分结果、需人工输入、取消 | `waiting_user` / `partial` / `completed` / `failed` / `cancelled` | 节点 sequence 单调；结果只追加 |
| `waiting_user` | 授权输入、超时、取消 | `running` / `failed` / `cancelled` | 不保持数据库事务、Kafka consumer 或 Agent HTTP 连接 |
| `partial` | 结果端口确认覆盖缺口 | 终态 | 已有 Proposal 仍可决定，未执行项明确标记 |
| `completed` / `failed` / `cancelled` | 新证据只可写诊断 | 终态 | 需重跑时创建新 Run，不原地复活 |
| 非终态 | 基线已被新批准版替代 | `superseded` | 已产生结果保留，Proposal 转 expired |

### 5.2 ProposalItem 状态

`pending → accepted | accepted_with_changes | rejected | deferred | expired`。`UndeferProposalItem` 在未过期且基线未变时追加 `undefer` Decision，使查询态回到 `pending`；其他终态不原地修改。accept/accept_with_changes 才有有效 payload/read-write set 并执行目标 Command；reject/defer/undefer 不携带 payload。接受与目标写入由顶层用例通过公开端口编排，不允许跨表 SQL：每个接受 Decision 创建一个 `ProposalExecution`，以 decision 和稳定 command key 分别唯一；每次调用目标模块追加 `(execution_id, attempt_no)` 唯一的 Receipt。所有 attempt 复用父 Execution 的 command key，响应丢失后的重试由目标模块回读同一结果；Execution 只允许一个 success/terminal_failure 终局，`(execution_id, terminal_receipt_id)` 只能指向本 Execution 的 Receipt，终局结果引用只从该 Receipt 读取；Receipt 不再对 command key 建唯一约束。

### 5.3 幂等键与冲突

- Run 创建业务幂等范围：`workspace + initiator_principal_ref + request_idempotency_key`；规范化 request hash 覆盖 project/scope、Skill/stage generation、parent/root/run Operation、graph/input/result contract、input snapshot、Tool/Command Schema set、model/usage/governance 等不可变语义字段；同键同 hash 回读、同键异 hash 返回 `idempotency_conflict`，service-only Run 不制造假 actor。
- Agent Service 结果唯一：`agent_run_id + sequence`，再比较 `result_hash`；同 sequence 不同 hash 进入隔离和审计。Advisory 使用 Run 内稳定 logical key/generation：跨 result sequence 的同 key/hash 回读既有 Item，但每个 Result 都追加 `(run, result_sequence, advisory_id, ordinal)` membership；membership 不复制 hash，result hash 按 ordinal 读取其所指不可变 Advisory.item_hash 计算，内容变化显式 supersede。Resolution 以请求 key/hash + expected generation 做 CAS，外部事实消除 gap 或使其 stale 时只追加记录，不覆盖原文。
- Proposal 决定以 `(proposal_item_id, decision_generation)` 单调追加，并以 `(proposal_item_id, actor_id, request_idempotency_key)` 唯一；请求携带 expected latest generation，同键同 request hash 回读、同键不同 hash 冲突、并发不同决定只允许一个 CAS 成功。传给目标模块的命令幂等键从 `proposal_item_id + accepted decision generation + target_module + effective_payload_hash` 稳定派生，并在唯一逻辑 Execution 上保存；所有 attempt Receipt 复用该 key。
- Run 的 `input_snapshot_hash` 只用于复现和审计，不直接作为每项接受的全局 CAS token。
- `CommitAgentRunResult` 落 Item 前调用目标模块 Preview，从模型 payload 计算并持久化 canonical read/write set/hash；模型声明仅用于差异诊断，不能成为 CAS 权威。
- 接受前由目标模块根据 Command payload 再次重算实际 read/write set，与落 Item 时的 canonical set/hash 比较并读取 item 对应的 expected current；任一实际依赖不符返回 `proposal_baseline_expired`，不自动把 payload 迁移到新版。同一 Run 内其他不相交 item 的成功写入不得导致本项过期。
- 画布布局使用 view revision 乐观锁；布局冲突不影响领域命令。

## 6. 页面与可观察状态

| 页面/区域 | 权威查询 | 必须显示的状态 | 可执行动作 |
| --- | --- | --- | --- |
| Agent 任务抽屉 | AgentRun + Operation | queued/running/waiting_user/partial/终态、当前节点、覆盖率、最近进展、用量/上限、部分错误 | 取消、继续人工输入、安全重跑、转手工工作台 |
| 提案审阅台 | Proposal + 目标模块预览 | 说明/建议/提案/外部动作分类、pending/终态/expired、基线差异、证据、影响、资源和权限 | 逐项接受、修改后接受、拒绝、暂缓、重算 |
| 最小关系接管视图 | M04 依赖投影 + 各模块批量 Query | 节点版本、blocked/unknown/stale、投影 as-of、权限禁用原因 | 选中、跳转权威表单、对受影响对象创建提案 |
| 完整画布 | CanvasView/Layout + 各模块 Query | 个人/团队可见性、布局修订、连接状态、未保存冲突 | 布局、缩放、分组、删除布局项、批量预览/提交 |

页面不从 Kafka delivery/offset、Agent 进程或 checkpoint 推断完成。可观测指标包括 Run 排队/节点耗时、private HTTP start/get/cancel 延迟与超时、ModelCallGate 限流/额度拒绝、Tool 失败率、checkpoint 恢复率、提案接受/修改/拒绝/过期率、非法输出拒绝数、人工接管次数和关闭 Agent 后任务完成率；日志只记录 ID、hash、版本和短摘要。

## 7. 失败、安全与恢复

| 错误码/场景 | 用户可见结果 | 恢复动作 |
| --- | --- | --- |
| `agent_model_unavailable` | Run failed/partial，手工工作台可用 | 稍后创建新 Run 或手工完成 |
| `agent_service_unavailable` / private HTTP timeout | Operation/Run 保持 queued/waiting_external，不显示完成 | 同 key/hash 重试 start 或 GET 对账；不创建第二 Run，不占 Kafka partition |
| `agent_tool_partial_failure` | 逐项成功、失败与未执行清单 | 只为失败范围新建 Run |
| `agent_output_invalid` | 保留契约诊断，零领域副作用 | 修正输入、model profile 或 contract 后新建 Run |
| `agent_scope_denied` / `tool_token_expired` | 拒绝节点并审计 | 由用户重新授权，不自动扩大 scope |
| `proposal_baseline_expired` | 显示变化对象和风险 | 重新计算或在目标编辑器手工迁移 |
| `proposal_command_conflict` | 展示目标 current revision，该项未执行 | 刷新预览后重新决定 |
| `canvas_projection_stale` / 实时连接中断 | 只读、显示 as-of/离线 | 回源查询，重连后按版本刷新 |

来源文本一律视为数据，不能改变系统策略和 Tool 白名单。Tool 不获得数据库、对象存储、密钥或跨项目通配凭据。Checkpoint 使用 Agent Runtime 专用存储和凭据，生命周期跟随 Run 和 M14 保留策略。

## 8. 逐 ID 验收映射

### 8.1 功能需求

| Requirement ID | 设计落点 | 最小可执行验收 |
| --- | --- | --- |
| AIC-FR-001 | 提案审阅台的四类回复标记 | 契约测试证明说明/建议不产生 command，外部动作始终显示待授权 |
| AIC-FR-002 | ProposalItem 信封的 target/baseline/evidence/impact/permission | Schema 缺任一必需引用时结果端口拒绝 |
| AIC-FR-003 | 逐项 Decision 状态机 | 10 项提案混合接受、修改、拒绝、暂缓后其他项不受影响 |
| AIC-FR-004 | `ExecuteProposedCommand` 目标模块端口 | 权限、revision、幂等或门禁任一不通过时领域事实不变 |
| AIC-FR-005 | Tool 只读、Service 结果端口和模块边界 | 架构测试拒绝 Agent Runtime 导入 Kafka/Redis/Elasticsearch client、领域 repository/数据库写端口；剧本检索只经 run-scoped backend Tool |
| AIC-FR-006 | AgentRun 全状态与 checkpoint 恢复 | 分别演练 waiting_user、partial、cancel、Agent Service 重启和 baseline superseded |
| AIC-FR-007 | Skill/Tool 白名单 + 目标模块 Command Registry | 剧本、实体、拆镜、阻塞解释、修复草案均只输出允许的类型契约 |
| AIC-FR-008 | `external_effect`/risk + M01/M14 重新鉴权 | 高消耗、外发、风险接受、主选、交付提案无显式授权均不执行 |
| AIC-FR-009 | 最小关系接管视图 | 从人物状态节点找到受影响镜头并打开 M05 修订表单 |
| AIC-FR-010 | 列表/画布/Agent 共用目标命令 | 三个入口对同一 revision 冲突返回同 error code/current revision |
| AIC-FR-011 | CanvasView/Layout 独立表 | 移动、缩放、分组、装饰连线后领域 hash 不变 |
| AIC-FR-012 | 批量 preview/submit 两阶段 | 提交前显示目标、跳过、影响、资源和失败策略，部分失败逐项可见 |
| AIC-FR-013 | view owner/visibility/revision | 个人与团队视图权限负向测试及所有者转移审计 |
| AIC-FR-014 | based_on_refs 重读与 expired 状态 | 引用版本变化后节点/提案显示过期且接受为零副作用 |
| AIC-FR-015 | AgentRunRequest 冻结字段 + M11 实际用量 | 调模型前能还原 scope/input/tool/外发/上限，完成后可查模型与 Tool 用量 |

### 8.2 非功能需求

| Requirement ID | 设计落点 | 最小可执行验收 |
| --- | --- | --- |
| AIC-NFR-001 | Agent 入口与手工命令解耦 | 禁用 Agent Service 后 M03—M10 P0 手工命令契约测试继续通过 |
| AIC-NFR-002 | 版本化 result/proposal/target command Schema | 未知版本、多余字段和非法 payload 在写业务事实前拒绝 |
| AIC-NFR-003 | 最小字段 Query、hash 日志、checkpoint 保留 | 日志/事件/审计扫描不含来源正文、Token 或凭据 |
| AIC-NFR-004 | 画布切片 F 容量 Gate | 真实规模节点/边下渲染、内存、键盘可访问性达到签认基线才启用 |
| AIC-NFR-005 | 所有 P0 的列表/表单入口 | 仅键盘、不看颜色且不使用拖拽也能完成全部 P0 场景 |
| AIC-NFR-006 | 提案决定/修改差异指标 | 仪表盘分别统计正确、修改、拒绝和业务结果，不用对话满意度代替 |

### 8.3 验收条件

| Requirement ID | 设计落点 | 自动化/故障演练 |
| --- | --- | --- |
| AC-AIC-001 | 逐项提案审阅 | Fixture 生成 10 个镜头提案，混合接受/修改/拒绝并校验逐项回执 |
| AC-AIC-002 | 目标 CommandPort | 同一 payload 从 Agent 与 M05 表单执行，比较领域结果 hash |
| AC-AIC-003 | 信封/Schema/鉴权/基线/幂等门禁 | 非法、重复、越权、过期四类负向测试校验业务表无变化 |
| AC-AIC-004 | Agent 媒体意图只生成 M07 payload | 测试 Agent 无法创建 M08 Job，只得到待批准 GenerationPlan 草稿 |
| AC-AIC-005 | 手工闭环 | 停止 Agent Service 后读取已有结构/实体/镜头并继续规划成功 |
| AC-AIC-006 | M04 真实依赖投影 + M05 跳转 | 人物状态变化 Fixture 仅标出真实引用镜头并打开修订 |
| AC-AIC-007 | Layout 删除语义 | 删除画布节点后校验实体/镜头/候选表及引用不变 |
| AC-AIC-008 | 共用目标命令错误契约 | 画布、列表、API 并发修改同一 revision，比较 code/current revision/recovery action |
| AC-AIC-009 | checkpoint + result 幂等 | 在每个 LangGraph 节点强制重启，同 Run 恢复且 ProposalItem 数量/hash 不重复 |

## 9. 验证与交付顺序

切片 B 先用 `script_analysis` 做唯一 P0 Skill：以一份包含跨集人物别名、同名歧义、空场景、失败分片和人物/地点/道具需求的固定整本剧本验证 `Breakdown Run → M03 Manifest 批准 → M02 物化/mapping → Narrative Run → M03 Narrative 批准 → Knowledge Run → M04 决议 → 确定性投影`。测试使用 Fake model/tool 覆盖 fan-out/fan-in、非法输出、重复结果、每节点重启、三个 stage 各自的过期基线、根/child Operation 关联、未知不等于缺席、覆盖缺口和关闭 Agent；额外验证 Narrative Run 不能在 mapping 前启动、Knowledge Run 不能读取未批准 Narrative、任一 item 不能引用 future ID，且 Agent 无法提交 EntityOccurrence current。契约测试保证手工命令与接受 AgentProposal 得到相同领域结果。切片 B 同时只交付受限 AgentRun、AgentProposal、共用领域命令和最小关系接管视图，不预建其他 Skill。切片 F 只在真实规模性能与可访问性 Gate 通过后交付完整画布，不改变前述事实所有权。
