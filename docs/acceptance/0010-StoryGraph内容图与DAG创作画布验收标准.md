# Lanverse 剧本视觉生产升级验收标准

> 状态：VP-D15 已接受（2026-08-31）；全部实现与验收目标初始未通过
>
> 接受依据：221 个 Requirement 表格条款与 Plan 主切片一一映射，无遗漏、无重复、无历史证据抵扣
>
> 正文 SHA-256：65b4e6e57418432abbcc3b55026775242641beb6bb6c51bd2554318f7b65d90f
>
> Requirement：[跨服务需求规格](../requirement/0010-StoryGraph内容图与DAG创作画布需求规格.md) · [Agent/Harness 需求规格](../requirement/3003-StoryGraph剧本解析Harness与内置Skill需求规格.md)
>
> Plan：[Lanverse 剧本视觉生产升级实施计划](../plan/0010-StoryGraph内容图与DAG创作画布实施计划.md)
>
> 证据基线：所有目标初始未通过；历史 SG-Ixx、旧 Runware/计费/视频/Canvas 或当前未提交 SG-I21 修改均不抵扣。

## 1. 证据口径

只有在对应主切片重新执行并记录真实命令、输入、结果、Owner 事实和失败路径后，Requirement 项才可从 [ ] 改为 [x]。Plan 勾选、代码存在、模型返回 JSON、图片已生成或旧测试曾通过都不能单独构成验收。

每条 Evidence 必须包含：

1. 精确 Git 基线与最终提交；
2. Red 失败证据和 Green 后同一断言通过；
3. 定向 unit/contract/integration/journey/browser 命令与结果；
4. 当时全量 CI 的真实命令、退出状态和必要外部条件；
5. 正式 Owner/Receipt/Hash/Workflow/Asset 的事实对账；
6. 未执行、跳过、使用 mock 或外部条件缺失的明确说明；
7. 与用户既有未提交修改的隔离证明。

同一 Requirement 只归一个主切片；后续回归可追加证据，但不能改变主责任。VP-I15 只做最终复核，不新增主合同。

## 2. Cross-service Requirement Checklist

### 2.1 架构、版本与幂等

- [ ] `VPR-ARC-001`（`VP-I01`）：Browser 只调用 Backend 公共 API；不得直接调用 Agent、Temporal、对象存储写接口或媒体 Provider。 最低证据：Architecture + Negative。
- [ ] `VPR-ARC-002`（`VP-I01`）：Go Backend 是 ProductionWorld、StoryGraph、Review、Workflow、ReferenceAsset、Generation、Asset 与 Storyboard 正式状态的唯一写入者。 最低证据：Integration。
- [ ] `VPR-ARC-003`（`VP-I01`）：PostgreSQL/GORM 保存业务事实，Temporal 保存长流程控制；双方用冻结身份和 Receipt 连接，不复制彼此状态机。 最低证据：Integration + Replay。
- [ ] `VPR-ARC-004`（`VP-I01`）：Agent 只返回严格 Candidate；不得分配正式业务 UUID、确认 Gate、应用 Owner 版本、恢复 Workflow 或调用媒体 Provider。 最低证据：Contract + Negative。
- [ ] `VPR-ARC-005`（`VP-I01`）：每个业务变更先形成 OwnerVersion，再由 StoryGraph 投影引用；StoryGraph 不得成为角色、形象、场景、道具、交互或视觉资产的第二写入源。 最低证据：Unit + Integration。
- [ ] `VPR-ARC-006`（`VP-I01`）：production 合同按一个实施切片内的 schema、writer、reader、fixture 原子切换；不得长期双写、fallback、latest 补全或静默兼容旧语义。最低证据：Contract + GORM Catalog/Schema Sync；本项目不建立独立 migration 事实源。
- [ ] `VPR-ARC-007`（`VP-I01`）：Workflow 只编排稳定宏观阶段；Agent shard、候选修订和视觉 Target 不得膨胀为动态 WorkflowDefinition 节点。 最低证据：Architecture + Replay。
- [ ] `VPR-ARC-008`（`VP-I01`）：MVP 生产主链不得依赖 Cost、Quota、Payment、视频生成或通用画布成功；相关服务故障不能阻断本文图片参考与分镜闭环。 最低证据：Journey + Fault injection。
- [ ] `VPR-ARC-009`（`VP-I01`）：只实现已被当前纵向切片消费的表、接口、目录和抽象；不得预建未来兼容层、微服务或空 Owner。 最低证据：Diff audit。
- [ ] `VPR-COM-001`（`VP-I01`）：OwnerVersionIdentityContract 固定包含 owner_kind、logical_id、version_id、revision、content_hash、created_at；引用必须携带完整身份，禁止 current/latest。 最低证据：Contract。
- [ ] `VPR-COM-002`（`VP-I01`）：每个 logical scope 只有一个线性 Head；发布命令以 expected_head CAS，冲突返回 head_conflict 且不产生部分写入。 最低证据：Integration + Concurrency。
- [ ] `VPR-COM-003`（`VP-I01`）：Canonical Hash 对语义字段执行版本化、排序稳定、UTF-8 与长度明确的确定性编码；Go/Python/TypeScript fixture 必须同值。 最低证据：Cross-language golden。
- [ ] `VPR-COM-004`（`VP-I01`）：业务 OwnerReceipt 证明命令已应用，Workflow EffectReceipt 证明副作用已被流程消费；两层 Receipt 单向引用，不得形成 hash cycle。 最低证据：Unit + Replay。
- [ ] `VPR-COM-005`（`VP-I01`）：rebase 只允许设计明确列出的空语义变化；任何证据、身份、状态、预设、视觉版本或绑定变化都必须重新计算并重新审核。 最低证据：Negative。
- [ ] `VPR-COM-006`（`VP-I01`）：所有命令具有稳定 idempotency_key；同键同输入收敛到同一结果，同键异输入返回 idempotency_conflict。 最低证据：Integration。
- [ ] `VPR-COM-007`（`VP-I01`）：TypedReadSetProof 精确记录计算读取的 OwnerVersionIdentity 集合；应用时逐项验证，缺失或漂移返回 stale_read_set。 最低证据：Unit + Integration。
- [ ] `VPR-COM-008`（`VP-I01`）：audit 字段、数据库自增值、租约到期时间、current head 和运行时授权不进入内容 Hash；其余影响语义的字段不得排除。 最低证据：Golden + Mutation。

### 2.2 文本事实与制作世界

- [ ] `VPR-P0-001`（`VP-I01`）：ScriptSourceVersion 保存原始 Unicode 字节、规范化换行策略、code-point 索引规则和 source_hash；span 不能用字节偏移混充字符偏移。 最低证据：Unicode golden。
- [ ] `VPR-P0-002`（`VP-I01`）：接受新剧本源必须在同一事务发布 SourceVersion 和 SourceHead；失败不得留下孤儿版本或悬空 Head。 最低证据：Integration。
- [ ] `VPR-P0-003`（`VP-I01`）：propose_script_spans 的分段按 source 范围无重叠、无遗漏覆盖；暂时 ID 只在 Candidate 内有效。 最低证据：Property + Adversarial。
- [ ] `VPR-P0-004`（`VP-I01`）：extract_scene_facts 在身份解析前只输出 style-blind 的地点、时间、动作、对白、原始人物提及、原始道具提及和证据 span；不得注入预设或视觉描述。 最低证据：Contract + Injection。
- [ ] `VPR-P0-005`（`VP-I02`）：resolve_identities 必须把全部 raw mention 精确划分为 resolved、ambiguous 或 rejected；不得丢项、重复归属或静默造人。 最低证据：Partition property。
- [ ] `VPR-P0-006`（`VP-I02`）：Gate 1 Subject 固定绑定 SourceVersion、SpanCandidateRevision、SceneFactCandidateRevision 与 IdentityCandidateRevision 的完整身份和 Hash。 最低证据：Contract。
- [ ] `VPR-P0-007`（`VP-I02`）：Gate 1 接受后先原子发布 EpisodeStructureVersion，再独立原子发布 IdentityResolutionVersion；任一步失败都可重放且不得重复应用。 最低证据：Integration + Fault injection。
- [ ] `VPR-P0-008`（`VP-I02`）：EpisodeStructureVersion 与 IdentityResolutionVersion 只能表达结构与身份，不得提前写角色形象、地点规格、道具状态、预设或参考图计划。 最低证据：Negative。
- [ ] `VPR-P0-009`（`VP-I02`）：changes_requested 必须包含 typed issue、证据 span 与允许的修复范围；自由文本不能成为机器执行的唯一输入。 最低证据：Contract。
- [ ] `VPR-P0-010`（`VP-I02`）：P0 可独立形成首个纵向切片：真实剧本输入、Agent Candidate、Gate 1 决策、Owner Apply、Workflow 恢复和查询回读全部可运行。 最低证据：Journey。

- [x] `VPR-WLD-001`（`VP-I03`）：Backend 机械地把身份事实划分为 confirmed、ambiguous、rejected 三个全集分区；Agent 不得决定正式分区。 最低证据：Unit + Property。Go/Python IdentityResolution Contract 均以完整 mention universe 验证 resolved、ambiguous、rejected 精确分区，重复归属、缺项和越界 reuse 被拒绝；Backend Gate 1 Apply 只从冻结分区机械生成正式 resolved/unresolved 映射，并为 new/reuse 身份校验确定性 UUID，Agent Candidate 不直接写正式身份。
- [x] `VPR-WLD-002`（`VP-I03`）：角色、角色形象、地点、道具、道具状态和交互的每项事实必须是 Evidence XOR CreatorDecision，且记录来源、作者与版本。 最低证据：Contract。Go/Python Production Entity Contract 同时接受纯 Evidence 与纯 `user_supplied` CreatorDecision，拒绝两者同时存在、两者都不存在以及 provenance 错配；Evidence 必须来自冻结 SceneFact universe。Gate 2 Apply 后 Basis 进入不可变 ProductionWorldEvidence/AssetState 等正式事实，记录独立 ID、revision、created_by 与 created_at。
- [x] `VPR-WLD-003`（`VP-I03`）：Character、CharacterAppearance、Location、Prop、PropState 使用独立稳定 logical_id 和 version；不得用名称、文件名或提示词充当身份。 最低证据：Integration。Gate 1 对新身份以 project + temporary identity key 确定性生成 UUID；真实 PostgreSQL + Temporal 旅程证明 Production Entity 使用该 UUID 而非名称，正式 Asset 另有独立 UUID，Appearance/LocationState/PropState 以稳定 state_key 绑定独立 AssetState UUID 与 revision。真实 Asset Owner 旅程进一步证明新增状态时既有 Asset ID、State ID 和 revision 保持不变，Head 仅新增成员。
- [x] `VPR-WLD-004`（`VP-I03`）：同一 Character 可有多个 Appearance；服装、年龄阶段、伤妆、湿身、伪装等改变必须形成独立 Appearance，而非覆盖角色锚点。 最低证据：Journey。三场契约旅程以林舟白衣/黑衣两个有序 Appearance State 绑定不同 SceneOccurrence，并用精确 Continuity 解释换装。
- [x] `VPR-WLD-005`（`VP-I03`）：SceneOccurrence 精确绑定 scene、subject_kind、subject_id、appearance_or_state_id、evidence 与顺序；同一场景内的出现不得靠全文搜索推断。 最低证据：Contract。已由 `bind_scene_occurrences` strict Candidate、Backend 双上游冻结读取和真实 Temporal 旅程覆盖；`mentioned_only` 不得提升为实际出现。
- [x] `VPR-WLD-006`（`VP-I03`）：InteractionSpec 精确绑定 actor appearance、prop state、动作、手位/身体接触、相对尺度、朝向、连续性和证据；不得仅保存“人物拿道具”的提示词。 最低证据：Contract + Journey。Go/Python strict Contract 已绑定精确 Occurrence/State，并要求手别、握法、接触点、方向和比例逐字段命中同场 Action Evidence；三场旅程覆盖 hold → give/receive 单视角 → carry。
- [x] `VPR-WLD-007`（`VP-I03`）：ContinuityLedger 对相邻场景记录 appearance、prop state、损坏、污渍、持有关系和位置的进入/离开状态，并能指出冲突证据。 最低证据：Unit + Journey。Candidate Ledger 已逐场覆盖实际 Character/Prop 的 State、holder、Location 与 Evidence；删除跨场 carry 时以无原因瞬移拒绝，交接重复应用、无因状态跳变和双 holder 仍由机械校验拒绝。
- [x] `VPR-WLD-008`（`VP-I03`）：Gate 2 Subject 固定绑定 ProductionWorldCandidate、SceneOccurrenceCandidate、InteractionCandidate、ContinuityCandidate 与全部上游正式版本。 最低证据：Contract + Integration。Backend 已从三个 strict fragment 确定性生成 Bible/Planning/Asset partition roots、expected business key/cross-partition/scope roots；`production.production_world_assembly` 在正式 Workflow 中持久化 aggregate Manifest、不可变 Candidate Revision 与 Head，重放复用同一 Revision，任一 Agent Candidate Head 漂移时整次聚合不落库；Gate 2 Subject 冻结 ProductionWorld Candidate revision、SceneOccurrence revision、同一 Interaction/Continuity revision 的两个独立 projection hash、SourceVersion、StructureIdentitySetVersion、三个预期 Owner Head 与 read-set root，漂移解码失败；真实 PostgreSQL 旅程已持久化唯一 Gate Input 与 HumanTask，HumanTask 精确冻结 aggregate 加三个上游 Candidate ID，重复打开不新增记录；approved ReviewDecision 只能解析为同一 Gate Input 和 aggregate Candidate 组成的严格 Owner Material，解析时再次验证正式 Head 与全候选集合。
- [x] `VPR-WLD-009`（`VP-I03`）：Gate 2 接受由一个命令原子发布 ProductionWorldVersion、SceneOccurrenceVersion 和 ContinuityVersion 三个 Owner family；任一失败全部回滚。 最低证据：Integration。Backend 以单个 `ConfirmProductionWorldCommand` 在同一 GORM 事务中依次调用 Asset、Bible、Planning Owner，发布三组不可变集合版本与 Head、集合 Receipt、CommandReceipt 和 Outbox；强制第二个 Owner 标识冲突时三域写入、Receipt 与 Outbox 全部回滚。真实 PostgreSQL 竞争旅程证明重复命令收敛到同一结果、漂移 Head 被拒绝；真实 Temporal 旅程证明 Gate 2 决议经 Signal/Apply Receipt 恢复原 Run 并完成。
- [x] `VPR-WLD-010`（`VP-I03`）：changes_requested 可只关闭受影响的 scene/entity shard；闭包必须包含其交互与连续性邻接，不得默认重跑全剧。 最低证据：Closure property。四类 operation 的确定性七类闭包、错型/越界/no-op 拒绝已有 Go/Python Contract 覆盖；真实 PostgreSQL + Homebrew Temporal 旅程进一步证明 Interaction 修复只重跑 `reconcile_interaction_continuity`，Entity 与 Scene Binding 节点以 `SKIPPED + reused_from_node_run_id` 复用，生成不同内容哈希的新 Candidate/Gate 后再批准并完成正式 Owner Apply。
- [x] `VPR-WLD-011`（`VP-I03`）：Gate 2 之前不得选择世界观预设、规划参考图、生成媒体、写 StoryGraph 正式版本或生成 Shot。 最低证据：Negative。真实 PostgreSQL + Homebrew Temporal 旅程在 Production World HumanTask 已 `OPEN`、尚未提交 Gate 2 Decision 的准确时点，以 GORM 查询同一 project：GenerationTarget、GenerationIntent、GenerationCandidate、GenerationCandidateSelection、Artifact、StoryGraphVersion/Head、StoryboardDraftSet/Batch/Shot 全部为 0；当前 Workflow 图到 Gate 2 为止不包含视觉、Provider、StoryGraph 或 Shot 节点。

### 2.3 Preset、参考计划与图片生产

- [ ] `VPR-PRE-001`（`VP-I05`）：MVP 内置 4–6 个不可变、版本化、可追溯许可与 NOTICE 的 WorldPreset；用户只选择，不在主链编辑任意风格 DSL。 最低证据：Inventory + License。
- [ ] `VPR-PRE-002`（`VP-I05`）：P0 SceneFact、Identity 与 ProductionWorld 提取完全 style-blind；WorldPreset 只能从 Gate 2 正式结果之后参与。 最低证据：Mutation。
- [ ] `VPR-PRE-003`（`VP-I05`）：WorldPreset 同时包含 fidelity invariant 与 world adaptation rule；改编风格不得改变角色身份、剧情事实、持有关系和场景连续性。 最低证据：Contract + Adversarial。
- [ ] `VPR-PRE-004`（`VP-I05`）：ReferenceTargetKind 严格只允许 character_anchor、character_appearance、location、prop、interaction、scene_composition 六类。 最低证据：Schema。
- [ ] `VPR-PRE-005`（`VP-I05`）：plan_reference_assets 先从正式制作世界机械计算 expected target set，再由 Agent 提出规格；Agent 不得删除必需 Target。 最低证据：Set equality。
- [ ] `VPR-PRE-006`（`VP-I05`）：Gate 3 Subject 固定绑定 WorldPresetRelease、VisualFoundationCandidate、ReferencePlanCandidate、expected target set 和 Gate 2 正式版本。 最低证据：Contract。
- [ ] `VPR-PRE-007`（`VP-I05`）：Gate 3 接受在一个命令中原子发布 VisualFoundationVersion 和 ReferencePlanVersion；Preset 选择作为前者内容的一部分冻结。 最低证据：Integration。
- [ ] `VPR-PRE-008`（`VP-I05`）：没有可用图片生成能力时允许保存 Draft Plan，但 Gate 3 不显示 approve；不得在 Gate 3 调用 Provider 或生成图片。 最低证据：Capability negative。
- [ ] `VPR-PRE-009`（`VP-I05`）：切换 Preset 只使视觉基础、参考计划、下游 Brief/Bundle/Selection/Packet/Storyboard 过期；不得使 ScriptSource、SceneFact、Identity 或 ProductionWorld 过期。 最低证据：Impact closure。
- [ ] `VPR-REF-001`（`VP-I06`）：ReferenceTarget 是创作意图和依赖身份；GenerationExecution 是一次执行事实。两者不得共享状态枚举或让重试创建新 Target。 最低证据：Domain unit。
- [ ] `VPR-REF-002`（`VP-I06`）：每个 Brief 必须绑定 ReferencePlanVersion、VisualFoundationVersion、目标 OwnerVersion、dependency selection、StageRelease 和 typed read set；任一漂移后不能执行。 最低证据：Fence。
- [ ] `VPR-REF-003`（`VP-I06`）：character_anchor 必须请求 front、profile、back 三视图，并冻结体型、面部、发型、比例和不可变身份特征。 最低证据：Contract + Media。
- [ ] `VPR-REF-004`（`VP-I06`）：character_appearance 必须请求 front、profile、back 三视图，且继承 character_anchor 身份，只改变已批准的服装/年龄/伤妆等状态。 最低证据：Contract + Vision。
- [ ] `VPR-REF-005`（`VP-I06`）：location 必须请求 empty_establishing、spatial_orientation、material_scale_detail，不能用带主角的气氛图替代空间基底。 最低证据：Contract + Media。
- [ ] `VPR-REF-006`（`VP-I06`）：prop 必须请求 front、side、back、state_detail；多状态道具必须保持身份并展示明确状态差异。 最低证据：Contract + Vision。
- [ ] `VPR-REF-007`（`VP-I06`）：interaction 必须请求 interaction_master，绑定已选 appearance、prop/state、姿势、握持、接触点、相对尺度和朝向。 最低证据：Contract + Vision。
- [ ] `VPR-REF-008`（`VP-I06`）：scene_composition 必须请求 composition_master，绑定场景发生的已选角色形象、地点、道具、交互和连续性状态。 最低证据：Contract + Vision。
- [ ] `VPR-REF-009`（`VP-I06`）：Target dependency graph 必须无环：anchor/location/prop base 先于 appearance/interaction，全部所需 base 先于 scene_composition。 最低证据：DAG property。
- [ ] `VPR-GEN-001`（`VP-I07`）：MVP 至少有一条真实图片 Provider 路径可生成六类 Target；不得因未接支付或成本模块而阻断。 最低证据：Real-provider journey。
- [ ] `VPR-GEN-002`（`VP-I07`）：ProviderCall 以唯一 submission_token 获得一次发送权；发送后断联进入 outcome_unknown，必须先对账，禁止盲重试。 最低证据：Fault injection。
- [ ] `VPR-GEN-003`（`VP-I07`）：Provider 输出先进入 staging；校验媒体类型、尺寸、Hash、恶意内容和目标身份后才可提升为 AssetVersion。 最低证据：Integration + Security。
- [ ] `VPR-GEN-004`（`VP-I07`）：ReferenceBundle 由共享 frozen execution set 与 view-role member 构成，Hash 不包含 VisionReview；VisionReview 引用 Bundle，禁止循环。 最低证据：Hash golden。
- [ ] `VPR-GEN-005`（`VP-I07`）：CandidateBundle 状态严格为 generated、deterministic_rejected、vision_reviewed、eligible、selected、superseded；非法跃迁拒绝。 最低证据：State-machine。
- [ ] `VPR-GEN-006`（`VP-I07`）：deterministic QC 只检查文件、尺寸、数量、Hash、角色覆盖和依赖完整性；语义一致性由独立 Vision Review 负责。 最低证据：Unit。
- [ ] `VPR-GEN-007`（`VP-I07`）：Vision Review 至少输出 identity、view_role、state、interaction_geometry、style_fidelity 五类 typed issue 和证据区域；不得选择候选。 最低证据：Vision contract。
- [ ] `VPR-GEN-008`（`VP-I07`）：warning 必须被用户逐项确认后 Bundle 才可 eligible；error 必须修复或重新生成，不能通过自由文本豁免。 最低证据：Journey。
- [ ] `VPR-GEN-009`（`VP-I07`）：选择以完整 Bundle 为单位，不能拼接不同执行的 front/profile/back 冒充一致三视图。 最低证据：Negative。
- [ ] `VPR-GEN-010`（`VP-I08`）：选择命令先写 SelectionReceipt，再由独立 Owner Apply 消费；重复信号收敛，失败可恢复。 最低证据：Replay。
- [ ] `VPR-GEN-011`（`VP-I08`）：base 目标选择后更新 ReferenceAssetSelectionVersion；只使其依赖 Target 和场景组合过期。 最低证据：Impact。
- [ ] `VPR-GEN-012`（`VP-I09`）：interaction 与 scene_composition 选择应用到对应场景范围，不得覆盖无关场景或全项目 Head。 最低证据：Scope integration。
- [ ] `VPR-GEN-013`（`VP-I08`）：Gate 4 是逐 Target HumanTask 与 checkpoint 聚合，不存在一个全局“全部视觉通过”任务；每个 Target 均保留独立决定和恢复边界。 最低证据：Workflow journey。

### 2.4 Human Gate、分镜、Query 与前端

- [ ] `VPR-GAT-001`（`VP-I02`）：HumanTaskInput 固定 gate_key、subject union、input_hash、impact summary、evidence refs、allowed decisions 与 workflow identity。 最低证据：Contract。
- [ ] `VPR-GAT-002`（`VP-I02`）：gate_key 只允许 structure_identity、production_world、visual_plan、reference_target、storyboard 五个值。 最低证据：Schema。
- [ ] `VPR-GAT-003`（`VP-I02`）：Decision 只允许 approved、changes_requested、rejected、not_required；每个 Gate 声明允许子集和 typed payload。 最低证据：Schema + Negative。
- [ ] `VPR-GAT-004`（`VP-I02`）：claim/renew/release 使用 lease token 和 expected revision；过期、越权或旧 token 不得提交决定。 最低证据：Concurrency。
- [ ] `VPR-GAT-005`（`VP-I02`）：提交 Decision 前重算 input_hash；任何 Subject 或依赖 Head 漂移返回 stale_subject 且不应用。 最低证据：Integration。
- [ ] `VPR-GAT-006`（`VP-I02`）：每种 accepted decision 先产生显式 EffectPlan，列出 Owner 命令、expected heads、read set 和恢复目标；不得用 if/else 隐式猜测。 最低证据：Contract。
- [ ] `VPR-GAT-007`（`VP-I08`）：not_required 只能用于 expected-set 机械证明为非必需的 Target，并记录 NegativeRequirementProof；不能手工跳过必需项。 最低证据：Negative。
- [ ] `VPR-GAT-008`（`VP-I02`）：DecisionReceipt、EffectReceipt 与 WorkflowResumeReceipt 分层持久；崩溃后用同一 Decision 继续，不能要求用户再次点击。 最低证据：Replay。
- [ ] `VPR-GAT-009`（`VP-I02`）：HumanTask、Decision 和 Effect 状态枚举互不混用；查询层可聚合展示但不能覆盖 Owner 状态。 最低证据：Contract。
- [ ] `VPR-GAT-010`（`VP-I02`）：服务端从成员资格与项目角色计算 allowed_actions；前端隐藏按钮不能替代授权。 最低证据：Security。
- [ ] `VPR-STB-001`（`VP-I10`）：Scene Coverage 只有在该场景 expected Target 全部 selected 或有合法 not_required proof 后才为 reference_ready。 最低证据：Coverage property。
- [ ] `VPR-STB-002`（`VP-I10`）：ProductionPacketVersion 对每场景冻结文本事实、角色 appearance、地点、道具状态、交互、连续性、已选 AssetVersion 与视觉基础；禁止 latest。 最低证据：Contract。
- [ ] `VPR-STB-003`（`VP-I10`）：direct_storyboard 只能读取 ProductionPacket、StageRelease 和显式创作约束；不得联网搜索、补全最新状态或调用媒体 Provider。 最低证据：Agent negative。
- [ ] `VPR-STB-004`（`VP-I10`）：Shot Candidate 使用 intent union 加 detail，而不是自由 needs_asset；每个 Shot 必须绑定 scene、source span、主体、空间、动作和连续性。 最低证据：Schema。
- [ ] `VPR-STB-005`（`VP-I10`）：Backend Binding normalizer 把 Agent 临时引用精确解析为 Packet 内 OwnerVersion/AssetVersion；歧义或缺失必须拒绝。 最低证据：Integration。
- [ ] `VPR-STB-006`（`VP-I10`）：ShotPlan 编译、图校验与顺序校验为确定性 Backend 逻辑，不委托 Agent 决定正式合法性。 最低证据：Unit。
- [ ] `VPR-STB-007`（`VP-I10`）：review_candidate/repair_candidate 只返回 typed issue 与允许 schema 内的新 Candidate；不得直接 patch 正式 ShotPlan。 最低证据：Contract。
- [ ] `VPR-STB-008`（`VP-I10`）：Gate 5 接受以 scene scope 原子发布 ShotPlanVersion 与 StoryGraphVersion；单场失败不污染其他场景。 最低证据：Integration。
- [ ] `VPR-STB-009`（`VP-I10`）：ShotPlan 应用前再次验证 StoryGraph read set、Packet、Reference Selection 与 Continuity Head；陈旧返回 stale_read_set。 最低证据：Fault injection。
- [ ] `VPR-STB-010`（`VP-I10`）：Workflow Compiler 只消费已发布 OwnerVersion 与 Gate EffectReceipt，不能从 Agent Candidate 或前端缓存编译执行计划。 最低证据：Architecture。
- [ ] `VPR-QRY-001`（`VP-I11`）：Backend 至少提供 ProjectProductionSummary、GateTimeline、ProductionWorldDetail、ReferenceCoverageMatrix、ReferenceTargetDetail、ScenePacketDetail、StoryboardDetail 与 ImpactPreview 八个 typed Query。 最低证据：OpenAPI contract。
- [ ] `VPR-QRY-002`（`VP-I11`）：readiness、coverage、stale reason、allowed action 与时间线由 Backend 计算；前端不得从多个请求自行拼状态机。 最低证据：Contract。
- [ ] `VPR-QRY-003`（`VP-I04`）：StoryGraphVersion 是 OwnerVersion 和关系的不可变投影；节点携带 owner identity，边携带 typed evidence/continuity/dependency，不复制 Owner 内容。 最低证据：Integration。
- [ ] `VPR-QRY-004`（`VP-I04`）：DAG 投影必须无环、可追溯到 Source span，并能从场景反查角色形象、地点、道具、交互、资产和 Shot。 最低证据：Graph property。
- [ ] `VPR-QRY-005`（`VP-I04`）：Graph 与 DAG 只作为有界只读关系 Lens；MVP 不提供任意拖拽改写正式 Owner 的通用画布。 最低证据：Browser + Negative。
- [ ] `VPR-FE-001`（`VP-I11`）：主入口为项目级 /production；旧页面只可链接进入，不能形成第二套制作状态。 最低证据：Router test。
- [ ] `VPR-FE-002`（`VP-I11`）：前端 API 类型来自已提交 OpenAPI 的生成产物并由 RTK Query 管理服务状态；禁止手写重复 DTO。 最低证据：CI。
- [ ] `VPR-FE-003`（`VP-I11`）：project_id、episode_id、scene_id、gate_key、target_id 进入 URL；刷新、后退、深链保持同一工作上下文。 最低证据：Browser。
- [ ] `VPR-FE-004`（`VP-I11`）：Gate 1 同屏展示原文证据、场景切分和身份歧义，支持逐问题 changes_requested。 最低证据：Browser。
- [ ] `VPR-FE-005`（`VP-I11`）：Gate 2 以角色/形象、地点、道具/状态、场景出现、交互和连续性六个视图审核制作世界。 最低证据：Browser。
- [ ] `VPR-FE-006`（`VP-I11`）：Gate 3 先选 4–6 个 Preset，再展示 expected Target 覆盖与六类计划；缺生成能力时解释为何不可批准。 最低证据：Browser。
- [ ] `VPR-FE-007`（`VP-I11`）：Gate 4 用 Target 卡片和完整 Bundle 对比视图展示 view role、依赖、QC、Vision issue、历史与逐项决定。 最低证据：Browser。
- [ ] `VPR-FE-008`（`VP-I11`）：Gate 5 同屏展示 Packet 证据、Shot 列表、绑定、连续性和 StoryGraph/DAG 只读 Lens。 最低证据：Browser。
- [ ] `VPR-FE-009`（`VP-I11`）：任意变化先打开 Impact Drawer，展示失效对象、保留对象、重跑范围和不可逆副作用，再提交命令。 最低证据：Browser + Contract。
- [ ] `VPR-FE-010`（`VP-I11`）：Inbox 聚合待我处理、即将过期、changes requested、provider_unknown 和失败任务；Provider Settings 不进入项目主旅程。 最低证据：Browser。
- [ ] `VPR-FE-011`（`VP-I11`）：页面统一展示 draft、running、needs_review、changes_requested、blocked、failed、stale、ready、approved，并保留 Owner 原始状态详情。 最低证据：Visual regression。
- [ ] `VPR-FE-012`（`VP-I11`）：桌面、平板和窄屏均可完成五 Gate；键盘、焦点、对比度、错误摘要和 aria 语义满足 WCAG 2.2 AA 的相关条款。 最低证据：Browser + Accessibility。

### 2.5 非功能、旅程与指标

- [ ] `VPR-NFR-001`（`VP-I12`）：剧本文本、Skill、Preset、Provider 输出和用户评论全部视为不可信数据；不能改变系统指令、允许工具或 Owner 边界。 最低证据：Injection suite。
- [ ] `VPR-NFR-002`（`VP-I12`）：Provider Secret 只在 Backend 凭据边界解密；不得进入 Agent input、日志、OpenAPI response、前端缓存或 Hash fixture。 最低证据：Secret scan。
- [ ] `VPR-NFR-003`（`VP-I12`）：私有资产预览使用短时授权或同源代理；持久记录只保存稳定对象身份，不保存签名 URL。 最低证据：Integration。
- [ ] `VPR-NFR-004`（`VP-I12`）：结构化日志至少带 project、workflow、stage、target、invocation、decision 或 provider_call 的适用身份；不得记录完整剧本或密钥。 最低证据：Log contract。
- [ ] `VPR-NFR-005`（`VP-I12`）：跨服务错误至少区分 validation、conflict、stale、unauthorized、unavailable、timeout、outcome_unknown、quarantined 和 internal。 最低证据：OpenAPI + Journey。
- [ ] `VPR-NFR-006`（`VP-I12`）：长流程在 Backend/Temporal 以 checkpoint、heartbeat、retry policy 和 reconciliation 恢复；HTTP 请求时限不能充当流程总时限。 最低证据：Restart + Replay。
- [ ] `VPR-NFR-007`（`VP-I12`）：changes_requested 与上游变化用有界依赖闭包计算；规模随受影响节点和边增长，不得默认扫描或重跑全项目。 最低证据：Performance property。
- [ ] `VPR-NFR-008`（`VP-I12`）：每次失效都给出机器可读 stale_reason、caused_by_identity、affected_scope 和 recommended_action；不得只显示“请重试”。 最低证据：Contract。
- [ ] `VPR-JRN-001`（`VP-I13`）：一份真实完整剧本完成上传、P0 Candidate、Gate 1、Owner Apply 与回读。 最低证据：API/DB/Workflow/Agent evidence。
- [ ] `VPR-JRN-002`（`VP-I13`）：同一角色至少两个 Appearance、一个至少两个 State 的道具、一次人物持道具 Interaction 和跨场 Continuity 通过 Gate 2。 最低证据：Owner versions + UI。
- [ ] `VPR-JRN-003`（`VP-I13`）：用户可在 4–6 个 Preset 中切换，并验证只失效 Gate 3 之后的视觉链。 最低证据：Impact proof。
- [ ] `VPR-JRN-004`（`VP-I13`）：六类 Target 各至少生成一个真实媒体 Bundle；三视图、道具多面/状态、交互和场景构图通过 deterministic 与 Vision 审核。 最低证据：Real assets + reviews。
- [ ] `VPR-JRN-005`（`VP-I13`）：Gate 4 每个必需 Target 均 selected 或有合法 not_required proof，Production-ready Scene Coverage 达 100%。 最低证据：Coverage numerator/denominator。
- [ ] `VPR-JRN-006`（`VP-I13`）：一个 scene_composition changes_requested 只重跑该场景与必要依赖，其他已选 Base Bundle 和已通过场景不失效。 最低证据：Closure diff。
- [ ] `VPR-JRN-007`（`VP-I13`）：Agent 超时、Backend 重启、Temporal replay、Provider outcome_unknown、媒体校验失败和重复 Gate signal 均在原身份上恢复。 最低证据：Fault matrix。
- [ ] `VPR-JRN-008`（`VP-I13`）：Gate 5 接受后可从任一 Shot 反查 Packet、资产、Target、制作世界实体、SceneFact 与原始剧本 span。 最低证据：Reverse trace。
- [ ] `VPR-MET-001`（`VP-I14`）：Production-ready Scene Coverage 的分母是 Gate 2 正式 Scene 集；分子是 expected Target 全部满足且 Packet/Storyboard Gate 已通过的 Scene，由 Backend 计算。 最低证据：Query golden。
- [ ] `VPR-MET-002`（`VP-I14`）：前导指标至少包含 identity resolution coverage、interaction coverage、reference target coverage、bundle pass rate 和 stale closure size。 最低证据：Metrics contract。
- [ ] `VPR-MET-003`（`VP-I14`）：运营指标至少包含各 Gate 首次通过时延、changes_requested 次数、局部重跑比例、outcome_unknown 对账时延和恢复成功率。 最低证据：Observability。
- [ ] `VPR-MET-004`（`VP-I14`）：守护指标为越权写入、跳过必需 Target、盲重试未知 Provider、跨项目污染、Hash/Wire 漂移和未授权 Secret 暴露均为零。 最低证据：Security + Incident query。

## 3. Agent/Harness Requirement Checklist

### 3.1 边界、Skill 供应链、Bundle 与 Release

- [ ] `VPA-BND-001`（`VP-I01`）：最终运行入口唯一为 agent/skills/build-storygraph/SKILL.md；Runtime 不从用户目录、网络或当前工作区动态发现其他 Skill。 最低证据：Path + Container negative。
- [ ] `VPA-BND-002`（`VP-I01`）：Go Backend 唯一拥有 StageDefinition、StageRelease、ControlHead、CandidateStageSet、Invocation/Attempt/Result、ShardManifest、CandidateRevision/Head。 最低证据：Architecture + DB。
- [ ] `VPA-BND-003`（`VP-I01`）：Agent 成功只产生 CandidateArtifact 与诊断，不得 Confirm/Apply、分配正式业务 UUID、推进 Gate、恢复 Workflow 或发布 OwnerVersion。 最低证据：Journey + Zero-write。
- [ ] `VPA-BND-004`（`VP-I01`）：Agent Runtime 不包含 ORM、业务 Repository、Temporal、对象存储、Kafka、Elasticsearch、Provider client 或公共业务 HTTP route。 最低证据：Dependency + Network scan。
- [ ] `VPA-BND-005`（`VP-I01`）：Agent input 不含 Secret、Provider Endpoint、私有签名 URL、图片/视频字节；Vision Stage 只接收 Backend 颁发的受限媒体读取能力与稳定 Asset ref。 最低证据：Contract + Secret scan。
- [ ] `VPA-BND-006`（`VP-I01`）：Stage shard 挂到既有 WorkflowRun/NodeRun；Runtime 不建立与 Temporal 重复的 checkpoint 状态机。 最低证据：Integration + Replay。
- [ ] `VPA-SUP-001`（`VP-I05`）：每个外部 Skill 先建立 SourceInventory，记录来源 URL/commit、抓取时间、作者、版本、文件 Hash、许可、NOTICE、预期能力和审核人。 最低证据：Inventory audit。
- [ ] `VPA-SUP-002`（`VP-I05`）：许可不明确、禁止再分发、包含凭据、隐式联网、指令注入、越权工具或不可追溯来源的 Skill 必须 quarantined，不能进入改写队列。 最低证据：Adversarial review。
- [ ] `VPA-SUP-003`（`VP-I05`）：通过初审的材料分类为 adopt、rewrite、reference-only 或 reject；禁止原样复制外部运行时、提示词或工具声明进入生产 Bundle。 最低证据：Mapping review。
- [ ] `VPA-SUP-004`（`VP-I05`）：rewrite 必须映射到七个能力之一：parse-script-structure、build-production-bible、map-scene-continuity、resolve-visual-foundation、design-reference-assets、review-production、direct-storyboard。 最低证据：Capability matrix。
- [ ] `VPA-SUP-005`（`VP-I05`）：每个改写通过 golden、adversarial、回归和边界 eval，再以 CandidateStageSet 进入 shadow；独立 reviewer 签署后才可批准。 最低证据：Eval + Signature。
- [ ] `VPA-SUP-006`（`VP-I05`）：运行时 Bundle 不下载外部 Skill、不联网搜索“最新最佳实践”、不按来源项目结构加载文件；吸收结果必须是仓库内审计过的重写资产。 最低证据：Network + Path negative。
- [ ] `VPA-BDL-001`（`VP-I01`）：固定目录只包含 SKILL.md、references、recipes、rubrics、eval 与 manifest 允许的资源；路径逃逸、符号链接逃逸、非 UTF-8、缺失或多余文件 fail closed。 最低证据：Filesystem adversarial。
- [ ] `VPA-BDL-002`（`VP-I01`）：SKILL.md 只保存跨阶段不变量、Owner 边界、证据规则和路由；阶段细则放 references，示例放 recipes，评审标准放 rubrics，不在 Python 复制同一指导。 最低证据：Structure audit。
- [ ] `VPA-BDL-003`（`VP-I01`）：每个 StageRelease 显式列出该 Stage 允许加载的资源；Runtime 只加载入口和该白名单，不递归拼接全部 Markdown。 最低证据：Loaded-file golden。
- [ ] `VPA-BDL-004`（`VP-I01`）：BundleManifest 对相对 POSIX 路径排序并覆盖路径字节、内容长度、原始 UTF-8 内容、输出 schema 和允许工具计算 Canonical SHA-256。 最低证据：Go/Python golden。
- [ ] `VPA-BDL-005`（`VP-I01`）：Bundle hash、任一资源字节、output schema、tool policy 或 version 单独漂移都必须拒绝；不得用当前 Bundle 替代冻结版本。 最低证据：Mutation。
- [ ] `VPA-BDL-006`（`VP-I01`）：非终态 Invocation 必须路由到精确 bundle_hash 对应的 Agent image digest；找不到返回 skill_bundle_unavailable。 最低证据：Rolling deployment。
- [ ] `VPA-REL-001`（`VP-I05`）：StageVariantKeyProduction 精确由 stage_key、profile_key、lane_key、output_schema_version 构成；四字段共同决定变体身份。 最低证据：Schema。
- [ ] `VPA-REL-002`（`VP-I05`）：DefinitionCore 保存变体身份、input/output schema、allowed tools、resource policy、模型能力、预算与不变量，不引用 Release、签名或 Control，避免 hash cycle。 最低证据：Hash graph。
- [ ] `VPA-REL-003`（`VP-I05`）：StageRelease 保存 release_id、definition_hash、bundle_hash、agent_image_digest、model capability、eval attestation、created_at 与 predecessor_release_id。 最低证据：Contract。
- [ ] `VPA-REL-004`（`VP-I05`）：CandidateStageSet 必须对当前生产 Profile 的十三个 StageVariantKey 完整且唯一，并携带完整性 proof 和 policy proof；不能混用未声明 Release。 最低证据：Set equality。
- [ ] `VPA-REL-005`（`VP-I05`）：EvalAttestation 与 ShadowAttestation 绑定同一 CandidateStageSet hash、固定数据集/流量窗口和基线；基线只能是前一 approved set。 最低证据：Attestation golden。
- [ ] `VPA-REL-006`（`VP-I05`）：SkillRelease 在独立 reviewer 签名后引用 CandidateStageSet、Eval、Shadow、provenance 与 license proof；签名不进入被签内容本身。 最低证据：Signature。
- [ ] `VPA-REL-007`（`VP-I05`）：ControlRecord 状态只允许 approved、deprecated、quarantined、revoked；ControlHead 用 expected revision CAS 线性推进。 最低证据：State machine + Concurrency。
- [ ] `VPA-REL-008`（`VP-I05`）：revoked 为终止安全状态；恢复必须创建新 StageRelease 和新审阅，不得把原记录改回 approved。 最低证据：Negative。
- [ ] `VPA-REL-009`（`VP-I05`）：dispatch、accept result、apply candidate 三处分别验证 StageRelease、SkillRelease 和 ControlHead fence；任一已 quarantined/revoked 都失败关闭。 最低证据：Race + Fault injection。
- [ ] `VPA-REL-010`（`VP-I05`）：Release、Signature、Attestation、Control 与 Receipt 的引用方向必须无环，Canonical Hash 排除数据库当前态和运行时租约。 最低证据：Graph/hash property。

### 3.2 Wire 与 Stage

- [ ] `VPA-WIR-001`（`VP-I01`）：公共 Invocation kind 只允许 storygraph_stage，wire_schema_version 固定 storygraph-stage-wire-production；不保留 production_bible、storyboard_draft 或无类型 map union。 最低证据：Strict schema。
- [ ] `VPA-WIR-002`（`VP-I01`）：Invocation 固定 invocation_id、attempt_id、StageVariantKeyProduction、StageRelease identity、SkillRelease identity、Control proof、scope、source refs、upstream refs、shard、payload、input_hash 与执行预算。 最低证据：Go/Python fixture。
- [ ] `VPA-WIR-003`（`VP-I01`）：scope 必须显式包含 workspace、project、episode 以及该 Stage 允许的 scene/entity/target；未知层级和跨项目引用拒绝。 最低证据：Negative。
- [ ] `VPA-WIR-004`（`VP-I01`）：source ref 使用完整 OwnerVersionIdentity；upstream ref 使用 CandidateRevision identity、producer Invocation/result hash；Agent 不得补全 current/latest。 最低证据：Mutation。
- [ ] `VPA-WIR-005`（`VP-I01`）：Stage input 为按 stage_key/profile_key 判别的 strict union，additional properties 默认 false；自由 JSON 只能存在于明确定义的 opaque evidence 字段。 最低证据：Schema fuzz。
- [ ] `VPA-WIR-006`（`VP-I01`）：input_hash 覆盖 wire version、variant、release、bundle、scope、排序 refs、shard manifest、payload 与执行预算；不覆盖 invocation_id、attempt_id、租约或 dispatch authorization。 最低证据：Cross-language golden。
- [ ] `VPA-WIR-007`（`VP-I01`）：dispatch authorization 在 Backend 运行时单独颁发并绑定 invocation、attempt、expiry 和 agent image；不能改变 Candidate 语义 Hash。 最低证据：Security。
- [ ] `VPA-WIR-008`（`VP-I01`）：AttemptResult 只允许 accepted、rejected、outcome_unknown，包含 input_hash、output_hash、diagnostic_hash、release fence 与完成时间；同尝试结果不可覆盖。 最低证据：State machine。
- [ ] `VPA-WIR-009`（`VP-I01`）：Go/Python 必须共用提交到仓库的正例、缺字段、未知字段、排序、Unicode、Hash 漂移和跨项目攻击 fixture。 最低证据：CI。
- [ ] `VPA-WIR-010`（`VP-I01`）：旧 Wire 在 production 切片中原子移除或明确隔离为历史调用路径；不得 fallback 或自动转换成 production 正式 Candidate。 最低证据：Architecture negative。
- [ ] `VPA-STG-001`（`VP-I13`）：CandidateStageSet 对上表十三个 stage_key 完整且无重复；缺一项、额外项或变体碰撞均不能批准。 最低证据：Set golden。
- [ ] `VPA-STG-002`（`VP-I02`）：每个 Stage 使用独立 strict input/output schema、allowed resource list、model capability 和 max model calls；不能共用万能 Candidate。 最低证据：Registry audit。
- [ ] `VPA-STG-003`（`VP-I02`）：review_candidate 与 repair_candidate 的 profile 必须精确绑定被评审 Stage schema 和 rubric；未知 profile 拒绝。 最低证据：Contract。
- [ ] `VPA-STG-004`（`VP-I07`）：review_reference_artifact 是唯一允许 Vision 能力的 Stage，只能读取 Invocation 授权的稳定媒体引用。 最低证据：Capability negative。
- [ ] `VPA-STG-005`（`VP-I10`）：direct_storyboard 只能在 ProductionPacketVersion reference_ready 后 dispatch；前序 Stage 不能绕过 Packet 直接生成 Shot。 最低证据：Fence。

### 3.3 P0、视觉与 Storyboard Candidate

- [ ] `VPA-P0-001`（`VP-I01`）：ScriptSpanCandidate 用 code-point start/end、source_hash、临时 span_id 和 coverage proof；范围越界、重叠或缺口拒绝。 最低证据：Unicode/property。
- [ ] `VPA-P0-002`（`VP-I01`）：SceneFactCandidate 是 style-blind，保留 raw_character_mentions、raw_prop_mentions、地点、时间、动作、对白和逐字段 evidence spans。 最低证据：Golden + Injection。
- [ ] `VPA-P0-003`（`VP-I02`）：IdentityResolutionCandidate 对 raw mention 做 resolved/ambiguous/rejected 精确分区，输出 confidence、rationale 和 evidence，不产生正式 Character。 最低证据：Partition。
- [x] `VPA-P0-004`（`VP-I03`）：ProductionWorldCandidate 严格区分 Character、CharacterAppearance、Location、Prop、PropState，并为每项给出 Evidence XOR CreatorDecision 提案。 最低证据：Schema。已由 `production_entity_fragment_candidate` strict schema 覆盖；正式发布仍由后续 Gate 2 Owner Apply 完成。
- [x] `VPA-P0-005`（`VP-I03`）：SceneOccurrenceCandidate 对 scene/subject/appearance_or_state/evidence/ordering 精确绑定，不以名称或 fuzzy search 代替身份。 最低证据：Contract。`scene_binding_fragment_candidate` 的 production schema 已精确冻结 Scene、Beat、Dialogue、Subject、Appearance/State、Evidence 和源顺序。
- [x] `VPA-P0-006`（`VP-I03`）：InteractionContinuityCandidate 同时输出人物—道具几何、持有/接触、相对尺度/朝向与跨场 appearance/prop state ledger。 最低证据：Journey。三场真实中文契约旅程已覆盖双角色、林舟双 Appearance、木盒 closed/open 双 State、单次交接、跨场 carry、逐字段几何 Evidence 和防瞬移 Ledger。
- [ ] `VPA-P0-007`（`VP-I03`）：P0 Candidate 中的所有临时 ID 只能在同一 Candidate graph 内引用；Backend Apply 负责机械分配与返回正式 identity map。 最低证据：Integration。
- [ ] `VPA-VIS-001`（`VP-I05`）：WorldPresetRelease 只在 resolve_visual_foundation 及其下游出现；对同一 P0 输入切换 Preset 不得改变 span、scene fact、identity 或 production entity Candidate hash。 最低证据：Metamorphic。
- [ ] `VPA-VIS-002`（`VP-I05`）：VisualFoundationCandidate 分开输出 fidelity invariants、world adaptations、palette/material/light/camera rules 与 forbidden changes。 最低证据：Strict schema。
- [ ] `VPA-VIS-003`（`VP-I05`）：ReferencePlanCandidate 必须覆盖 Backend 提供的 expected target keys，类型只允许六类；只能补充规格，不能删除、改名或合并 Target。 最低证据：Set equality。
- [ ] `VPA-VIS-004`（`VP-I06`）：ReferenceBriefCandidate 使用六类判别 union 和固定 view roles；只表达 Provider-neutral 视觉要求，不含自由 Provider 参数、密钥或执行命令。 最低证据：Schema。
- [ ] `VPA-VIS-005`（`VP-I06`）：character_anchor 与 character_appearance 输出 front/profile/back；后者显式继承 anchor identity 与批准变化。 最低证据：Contract。
- [ ] `VPA-VIS-006`（`VP-I06`）：location 输出 empty_establishing/spatial_orientation/material_scale_detail；prop 输出 front/side/back/state_detail。 最低证据：Contract。
- [ ] `VPA-VIS-007`（`VP-I06`）：interaction 输出 interaction_master 并绑定 appearance、prop state、动作、手位/接触点、尺度和朝向；scene_composition 输出 composition_master 并绑定全部已选 base。 最低证据：Contract。
- [ ] `VPA-VIS-008`（`VP-I07`）：VisionReviewCandidate 至少含 identity、view_role、state、interaction_geometry、style_fidelity 五类 issue、severity、region/evidence 和 recommendation。 最低证据：Vision eval。
- [ ] `VPA-VIS-009`（`VP-I07`）：Vision Stage 不得返回 selected、approved 或 Owner mutation；Backend 结合 deterministic QC 与 Human Gate 决定 eligibility/selection。 最低证据：Negative。
- [ ] `VPA-STB-001`（`VP-I10`）：direct_storyboard 输入冻结每场景 ProductionPacket：source facts、appearance、location、prop state、interaction、continuity、selected assets 和 visual foundation。 最低证据：Fixture。
- [ ] `VPA-STB-002`（`VP-I10`）：StoryboardCandidate 使用 intent 判别 union 与 typed detail；不得出现 needs_asset、current/latest、Provider、搜索或未绑定自然语言实体名。 最低证据：Schema + Negative。
- [ ] `VPA-STB-003`（`VP-I10`）：每个 Shot Candidate 携带临时 shot_id、scene ref、source span、主体、动作、构图、镜头、时长、连续性与 Packet 内 binding refs。 最低证据：Contract。
- [ ] `VPA-STB-004`（`VP-I10`）：Agent binding 只引用 Packet local key；Backend normalizer 独立解析为正式 OwnerVersion/AssetVersion，歧义时拒绝而非猜测。 最低证据：Integration。
- [ ] `VPA-STB-005`（`VP-I10`）：review_candidate 对 Storyboard 只输出 typed issue；repair_candidate 只能按 allowlist 生成新完整 CandidateArtifact。 最低证据：Repair negative。

### 3.4 Candidate、Runtime、Eval 与旅程

- [ ] `VPA-CAN-001`（`VP-I02`）：ShardManifestProduction 不可变，包含 manifest_hash、scope universe、shard key、coverage、dependency closure 和 fixed-point proof；同阶段所有 shard 无重叠且完整覆盖。 最低证据：Property。
- [ ] `VPA-CAN-002`（`VP-I02`）：CandidateRevision 不可变，包含 revision_id、stage variant、shard、input_hash、output_hash、producer union、parent revision 和 status。 最低证据：Contract。
- [ ] `VPA-CAN-003`（`VP-I02`）：producer union 明确区分 Agent Attempt、Backend mechanical、Human correction；三者字段不可混用。 最低证据：Strict union。
- [ ] `VPA-CAN-004`（`VP-I02`）：每个 stage_instance_key 只有一个 CandidateHead，更新使用 expected revision CAS；并发 repair 只能一个成功。 最低证据：Concurrency。
- [ ] `VPA-CAN-005`（`VP-I02`）：repair 输入必须是 typed issue、field-path allowlist、原 Candidate 与全部冻结 refs；输出完整新 Artifact，不接受 JSON Patch 或原地修改。 最低证据：Negative。
- [ ] `VPA-CAN-006`（`VP-I03`）：repair 后重新运行 schema、invariant、review 与 affected closure；未受影响 shard 保留原 Revision，不默认全剧重跑。 最低证据：Closure。
- [ ] `VPA-CAN-007`（`VP-I03`）：source、owner、release、manifest 或 upstream candidate 漂移使当前 Candidate stale；stale 只能重算或明确 canonical-empty rebase。 最低证据：Mutation。
- [ ] `VPA-CAN-008`（`VP-I03`）：Candidate rejected、stale、quarantined 后不能 Apply；历史 Artifact 仍可审计但不能成为 latest 输入。 最低证据：Fence。
- [ ] `VPA-RUN-001`（`VP-I12`）：Invocation 与 Attempt 分离；同 Invocation 可有多个 Attempt，但每次只允许一个有效 lease，成功 Result 收敛到同一 input_hash。 最低证据：Restart。
- [ ] `VPA-RUN-002`（`VP-I12`）：超时或进程中断若无法证明未执行，Attempt 进入 outcome_unknown；Backend 先按 invocation/attempt/result identity 对账再决定重试。 最低证据：Fault injection。
- [ ] `VPA-RUN-003`（`VP-I12`）：dispatch 前、Result 接受前、Candidate Apply 前均重验 release/control/input fence；运行中撤销 Release 不能被旧结果绕过。 最低证据：Race。
- [ ] `VPA-RUN-004`（`VP-I12`）：Text broker 与 Vision broker 能力分离；除受限媒体读取外 allowed_tools 为空，Stage 不能自行打开网络、shell 或文件系统。 最低证据：Sandbox。
- [ ] `VPA-RUN-005`（`VP-I12`）：每次 Attempt 使用独立临时目录和显式文件白名单，完成后可回收；不得读取项目无关文件、用户目录或凭据。 最低证据：Filesystem attack。
- [ ] `VPA-RUN-006`（`VP-I12`）：剧本、Skill 引用、用户评论和媒体元数据全部按 untrusted data 处理；提示注入不能更改 Stage、工具、输出 schema 或 Owner 边界。 最低证据：Adversarial。
- [ ] `VPA-RUN-007`（`VP-I12`）：max model calls、单调用 deadline、总执行 deadline 和输出大小预算由 StageRelease 冻结；超限返回 typed error，不截断成合法 Candidate。 最低证据：Budget。
- [ ] `VPA-RUN-008`（`VP-I12`）：错误至少区分 invalid_input、schema_mismatch、bundle_unavailable、release_blocked、lease_lost、timeout、outcome_unknown、model_unavailable、media_unavailable、internal。 最低证据：Error fixture。
- [ ] `VPA-RUN-009`（`VP-I12`）：HTTP 时限不充当 Workflow 总时限；Backend/Temporal 用心跳、retry policy、reconciliation 和持久 Receipt 恢复。 最低证据：Replay。
- [ ] `VPA-EVL-001`（`VP-I13`）：skill-creator 的结构校验用于仓库内 Bundle，但其通过只证明格式，不替代业务 eval、许可、安全和发布审阅。 最低证据：CI。
- [ ] `VPA-EVL-002`（`VP-I13`）：CI 检查 provenance、license/NOTICE、文件 allowlist、Canonical Hash、十三 Stage 完整性、Wire strictness 与跨语言 fixture。 最低证据：CI。
- [ ] `VPA-EVL-003`（`VP-I13`）：golden dataset 覆盖中文 Unicode、多集多场、同名人物、多 Appearance、多状态道具、人物持道具、跨场连续性和六类 Target。 最低证据：Dataset audit。
- [ ] `VPA-EVL-004`（`VP-I13`）：adversarial dataset 覆盖 prompt injection、伪造 system 文本、路径逃逸、恶意媒体元数据、跨项目 ref、latest 补全和超预算输出。 最低证据：Security CI。
- [ ] `VPA-EVL-005`（`VP-I13`）：Vision eval 使用固定真实图片样本和人工标注，分别评估五类 issue；不能只 mock 图片读取或只验证 JSON 可解析。 最低证据：Vision benchmark。
- [ ] `VPA-EVL-006`（`VP-I13`）：Shadow 在不写正式 Owner 的条件下运行完整 CandidateStageSet，与前一 approved set 对比质量、错误、时延和局部闭包。 最低证据：Shadow evidence。
- [ ] `VPA-EVL-007`（`VP-I13`）：forward test 在新 Release 批准后验证新 Invocation 使用新 set、在途 Invocation 保持冻结旧 set、revoked set 三道 fence 均拒绝。 最低证据：Deployment integration。
- [ ] `VPA-EVL-008`（`VP-I13`）：最终端到端验收至少一次使用真实 Agent/Codex 执行关键 Stage；mock 只可用于确定性故障注入，不能抵扣最终语义闭环。 最低证据：Real-agent journey。
- [ ] `VPA-JRN-001`（`VP-I13`）：真实剧本依次运行 spans、scene facts、identity，产生严格 Candidate，经 Backend Gate 1 Apply 后可追溯原 span。 最低证据：Agent/DB/Workflow。
- [ ] `VPA-JRN-002`（`VP-I13`）：production entities、occurrences、interaction continuity 识别两种形象、两种道具状态和持有交互，经 Gate 2 原子应用。 最低证据：Candidate + Owner versions。
- [ ] `VPA-JRN-003`（`VP-I13`）：同一正式 P0 输入在两个 Preset 下保持事实 Candidate hash 不变，仅 visual foundation 和下游变化。 最低证据：Metamorphic evidence。
- [ ] `VPA-JRN-004`（`VP-I13`）：六类 ReferenceBrief 通过 strict schema；Vision 对真实 Bundle 发现至少一个注入的 identity 或 geometry 缺陷且不自行选择。 最低证据：Brief/Vision evidence。
- [ ] `VPA-JRN-005`（`VP-I13`）：ProductionPacket 驱动 direct_storyboard，Backend normalizer 精确绑定，review/repair 形成新 Revision，经 Gate 5 原子应用。 最低证据：Full journey。
- [ ] `VPA-JRN-006`（`VP-I13`）：Agent 崩溃、租约丢失、bundle 缺失、release 撤销、outcome_unknown 和重复 Result 均在冻结身份上恢复或失败关闭。 最低证据：Fault matrix。

## 4. 实施切片完成 Checklist

- [ ] `VP-I01`：Scene Fact 生产契约 首个纵向切片；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I02`：全剧身份调和、Gate 1 与正式结构；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I03`：制作世界、人物多形象、道具交互与 Gate 2；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I04`：storygraph-production 制作投影与可追溯 Query；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I05`：Skill 供应链、4–6 Preset、视觉基础与 Gate 3；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I06`：六类 Target 与 Provider-neutral Brief；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I07`：图片执行、Bundle、确定性 QC 与 Vision Review；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I08`：Gate 4 基础 Bundle 选择与 checkpoint；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I09`：Interaction/Scene Composition 与局部失效；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I10`：Production Packet、direct_storyboard 与 Gate 5；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I11`：项目级 Guided Studio 与 typed Query；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I12`：恢复、安全、可观测与有界闭包；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I13`：十三 Stage 正式 Release、Eval 与完整剧本真实媒体矩阵；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I14`：指标、性能与发布候选全量门；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。
- [ ] `VP-I15`：最终浏览器验收与事实对账；Plan 完成门、主 Requirement Evidence、当时全量 CI 与独立提交均已有可复核记录。

## 5. P0–P4 阶段验收

- [ ] P0：真实剧本完成 Gate 1/2，人物多形象、道具状态、Interaction/Continuity 与 storygraph-production Query 均可追溯。
- [ ] P1：4–6 Preset、六类 Target、真实基础 Bundle、Vision Review 和逐 Target 选择通过。
- [ ] P2：Interaction/Scene Composition 精确消费 base，局部失败与修改不污染无关场景。
- [ ] P3：ProductionPacket 驱动正式分镜，Gate 5 与项目级五 Gate Guided Studio 可用。
- [ ] P4：十三 Stage Release、完整剧本、真实媒体、恢复/安全/指标/全量 CI 和最终浏览器对账通过。

## 6. Evidence Log

### `VP-I01` — Scene Fact 生产契约 首个纵向切片

- 状态：实现、定向验证、本机完整语言门与镜像门已通过；远端 Required CI 与 Requirement 勾选尚未完成
- Git 基线/提交：基线 `14d43fe1`；生产闭环提交 `b97628b3`、`e1174013`、`232e9f0e`、`d489298a`；本轮 CI 恢复提交 `5f997fb1`、`e99a1860`、`0bf9ddcf`
- Bundle 根链 Red/Green：`.venv/bin/pytest -q tests/contract/test_scene_analysis_contract.py -k symlinked_parent_escape` 在修复前证明将 `agent` 父目录指向仓库外时，production Bundle 仍会接受仓库外的同名文件集。Green 后 Runtime 在读取前校验 repository→agent→skills→bundle 整条根链、严格解析及仓库包含关系；缺文件、额外文件、非 UTF-8、文件符号链接和父目录符号链接五类反例均 fail closed。Skill 资源字节未改变，冻结 Bundle Hash 仍为 `d096f3d38ff5383d685b2a510cea25985978e294a2a8c46841fa15320eee7b71`；Agent 全量 Ruff/format/Pyright 零错误、56 passed、5 条显式真实 Codex 旅程 skipped。
- Result Hash Red/Green：`go test ./tests/agent -run '^TestSceneAnalysisWireMatchesSharedFixtureAndRejectsMutations$' -count=1` 在 Green 前因 `SceneAnalysisAttemptResult` 缺少 `ComputeResultHash`/`ResultHash` 无法编译；`.venv/bin/pytest -q tests/contract/test_scene_analysis_contract.py -k result_hash` 同时证明缺少完整结果 Hash 的 Result 仍会被 Python 接受。Green 后 Go/Python 对共享 fixture 的完整 Attempt Result 得到同一 `4b99797ab01461fc6a895e98ded59c8b91b70c8a4b263f844dad2812f9038b8c`，完成时间等任一结果字段漂移均被拒绝。Agent API accepted/outcome_unknown 均返回可自校验 `result_hash`；固定 `lanverse_test` 旅程最新 3.510 秒通过，证明 Candidate `source_result_hash` 精确引用持久化 Result Hash 且不再复用 Candidate `output_hash`，前后 Invocation 记录仍为 `48 → 48`。Agent 全量门为 Ruff/format/Pyright 零错误、56 passed、5 条显式真实 Codex 旅程 skipped；Backend `gofmt`/`go vet ./...`/`go test ./... -count=1` 全部通过。
- StageInstanceKey Red/Green：共享 fixture 先原子切换到显式 `identity_contract_id=storygraph-stage-instance-production`、完整 Variant、Scope、Shard Manifest、Shard Key 与 Input Hash 的 Canonical JSON 根。Green 前 Go、Python 和 TypeScript 定向契约均得到旧字符串拼接值 `32465dba5057579e0a2c83e16153958aa0cb2f914092c23673bcc9a568d07682`，与新 golden `dd0fc996bc038cbdd0fc94e6204a992e13e36685f3f93b826803e46759614169` 不同而失败。Green 后三端对同一结构化 preimage 得到同一新值，不保留旧键读取、双算或 fallback；Frontend 定向 1/1、固定 `lanverse_test` 旅程、Backend 全量、Agent 全量以及 Frontend 20 files/56 tests/build 均通过，证明契约边界与正式持久化链路同步切换。
- Source 持久化测试隔离 Red/Green：修复前 `TestAcceptSourcePublishesIndexHeadAndReceiptsAtomically` 与 `TestAcceptSourceHeadCASAllowsOnlyOneConcurrentSuccess` 均直接使用固定库根连接，成功路径会提交随机 Source/Head/Index/Receipt 测试事实。为避免再次故意污染正式 `lanverse_test`，Red 以无 Rollback/精确清理的代码路径和修复前固定库已有 `38` 组 Source Head/Index/Collection Receipt 为可执行前证据，没有再运行旧实现制造垃圾。Green 后单事务旅程使用外层 GORM Rollback；并发 CAS 继续使用真实独立事务，并仅按本次随机 workspace/project/document UUID 与两个 idempotency key 反向清理测试事实。测试移动到 `tests/production/script/adapter/gormdb`，没有给数据库架构门添加例外；`TestDatabaseArchitectureBoundaries` 通过。两条真实 PostgreSQL 旅程 5.068 秒通过，文档、版本、Head、Index、Collection Receipt、Command Receipt 六类数量均保持 `152,161,38,38,38,38 → 152,161,38,38,38,38`；未创建替代数据库、Schema 或迁移。
- Source 幂等冲突 Green：同一固定 PostgreSQL 旅程现在同时证明同 `idempotency_key` 同输入返回完全相同 Source 接受结果，而同键改变 expected Head 输入精确返回 `idempotency_conflict`，不会重复发布 Source/Index/Head/Receipt。`go test ./tests/production/script/adapter/gormdb -count=1 -v` 两条旅程 5.507 秒通过，文档、版本、Head、Index、Collection Receipt 与全库 Command Receipt 数量保持 `152,161,38,38,38,1406 → 152,161,38,38,38,1406`。
- Dispatch Authorization fail-closed Red/Green：`.venv/bin/pytest -q tests/integration/test_scene_analysis_api.py -k malformed_dispatch_authorization` 在修复前以 `a.signature` 触发 Base64 解码错误；旧 `InvalidExecutionGrant` 逃逸出 Scene Analysis 的授权异常边界，ASGI 请求直接抛异常而不是返回 401。Green 后 Scene Analysis 验签只解码一次，并把编码错误转换为语义化 `InvalidSceneAnalysisDispatchAuthorization`；同一 API 断言得到 `401 {"detail":"invalid dispatch authorization"}`。定向 1 passed，Agent fail-fast 完整门 Ruff/format/Pyright 零错误、57 passed、5 条显式真实 Codex 旅程 skipped。
- Bundle Runtime 路由 Red/Green：固定 `lanverse_test` 旅程新增 Runtime Catalog 无可用冻结 Bundle 的分支后，Red 实际返回 `agent_outcome_unknown`，证明 Application 把确定的 `ErrSkillBundleUnavailable` 泛化并丢失 typed error；事务回滚后 Invocation 数仍为 `48 → 48`。Green 后该 Attempt 仍以可重试的 `outcome_unknown` 持久化，但 Result diagnostic/error 与调用错误码均精确为 `skill_bundle_unavailable`，不包含传输细节。`TestSceneAnalysisRuntimeRoutesOnlyExactBundleAndImage` 同时证明精确 Bundle Hash + agent image digest 才发送正式内部 HTTP；image 不一致在网络调用前 fail closed，HTTP 计数保持不变。固定库旅程 3.206 秒、Runtime 路由定向 1 passed。
- Typed Read Set Red/Green：`TestSceneAnalysisWorkflowPersistsTwoStrictCandidatesAndReplays` 在 Green 前将 Agent 返回前的 `DocumentRevision.normalized_hash` 改为另一合法 Hash，实际执行仍返回 `nil` 并发布 Candidate，证明 Candidate Apply 没有重新验证冻结 Source。Green 后 Candidate Apply 在同一 GORM 事务内以共享行锁精确重读 SourceVersion/Document，以及 Scene Fact 的上游 ScriptSpan Candidate、来源 Invocation/Attempt/Result，重新计算 Result、Candidate 内容与 Candidate Revision Hash；Source Hash 漂移和上游 Shard 关系漂移均返回 `stale_read_set`，两条路径 Candidate 写入数均为零。未新增第二套 read-set 字段、表或迁移，Wire 已有 `source_refs`/`upstream_candidates` 就是唯一读集事实。固定 `lanverse_test` 旅程 3.411 秒通过，Invocation 数保持 `48 → 48`。
- Red 命令与失败：`go test ./tests/production/script -run '^TestScriptSourceHTTPRequiresExpectedHeadRevision$' -count=1` 在修复前返回 HTTP 201，证明缺失 expected Head 被 Go 零值错误解释；同一断言修复后返回 422。Go/Python production Wire、Unicode、未知字段、Hash 漂移、跨项目字段、全覆盖与 style 注入反例分别固化在 `backend/tests/agent`、`agent/tests/contract`，不是运行时兼容分支。派发授权 Red 中，Go 因缺少 `IssueSceneAnalysisDispatchAuthorization`/`VerifySceneAnalysisDispatchAuthorization` 无法编译，Agent 两条正式请求因仍要求旧 Header 返回 422；这分别证明 Backend 尚未独立颁发 Attempt 授权、Harness 尚未只接受语义化派发授权。Bundle Red 证明修改未被当前 Stage 加载的已声明资源、manifest 版本或工具策略不会改变旧 hash；Canonical Red 证明 Go 默认把 `<>&` 转义而 Python 保留原字符，形成真实跨语言 Hash 漂移。Bundle/Variant 原子切换后的真实 Codex 首跑在第二阶段调用前因测试输入缺失 lane/output schema 被严格拒绝，未以默认值或兼容补全绕过。
- Green/定向验证：固定 `lanverse_test` 执行 `TestSceneAnalysisWorkflowPersistsTwoStrictCandidatesAndReplays` 最新通过（3.206 秒），覆盖 Source 接受、三节点 DAG、ScriptSpan/SceneFact 持久化、typed Query、Replay、一次传输 `outcome_unknown` 后同 Release 第二 Attempt 成功，以及 Bundle 路由缺失的 typed 可重试结果；同一旅程还对账不可变 `SceneAnalysisDispatchAuthorization` SQL 事实、Attempt claim、授权 Hash/期限与 Result 回传值。派发顺序固定为同一 GORM 事务内 `CreateAttempt → 签发 → CreateDispatchAuthorization`；故障注入证明签发时 Attempt 已对事务可见，但签发失败后 Runtime 调用数仍为零，Invocation/Attempt 全部回滚。整个真实旅程放入外层回滚事务，执行前后 `agt_scene_analysis_invocations` 记录数保持 `48 → 48`，不再污染固定测试库；SQL Adapter 不接触可重放 token。Go 派发授权 Security/Contract 测试通过，证明 invocation、attempt、claim、input/release/control/fence、agent image 与 expiry 绑定且授权不进入 Candidate 语义 Hash；Agent 只接受 `X-Lanverse-Dispatch-Authorization`，accepted/outcome_unknown 六个目标测试通过。Bundle hash 覆盖全部已声明资源、语义化 manifest 身份与预算、输出 Schema 和工具策略，Stage 仍只加载自身白名单引用；未加载资源、版本与工具策略漂移反例均通过。Canonical JSON 已统一 NFC、UTF-8 key 顺序、整数与关闭 HTML 转义；Go/Python/TypeScript 对共享 Unicode/特殊字符、input hash 和 stage identity golden 同值。项目自有任意数字发布序号扫描与 `TestProjectContractsUseSemanticNames` 通过，覆盖业务源码、测试、文档、Agent Skill、Compose 与三类 Dockerfile；直接断言业务 `v数字` 命名必须拒绝，豁免仅限第三方 SemVer、Go module 主版本路径、官方 API/工具固定标识与外部 GitHub Action 引用。Provider Binding 测试中的数字序号变量和幂等键已改为 `bindingAfterProfileEnable` / `provider-binding-publish-after-profile-enable`。Source HTTP 三条测试通过。Agent Ruff/format/Pyright 通过，普通 Pytest 为 57 passed、5 条显式真实 Codex 长旅程 skipped；修正完整四字段 Variant 后，`LANVERSE_TEST_REAL_CODEX=1 .venv/bin/pytest -q tests/integration/test_scene_analysis_real_codex.py` 以中文两场剧本重新通过（1 passed，37.81 秒）。Frontend OpenAPI 重新生成无漂移，ESLint、TypeScript、20 files/56 tests 与 Next.js build 通过。Frontend、Backend、Agent 三类镜像按顺序构建通过，未复现并发构建 OOM；容器内单 Backend Binary、Frontend standalone、Agent 非 root、唯一 Skill Bundle、Codex CLI 与 Bundle 加载断言全部通过，三层 Compose `config --quiet` 通过且未启动环境服务。
- Eventing 固定库隔离 Red/Green：Backend 全量门首次运行时，`TestGORMOutboxInboxRevisionAndDeadLetterState` 从正式固定库领取到历史 pending Outbox，因测试错误假设全库只有自己的事件而失败，并留下 10 个已过期 claim/attempt 副作用。未清理或覆盖这些历史事实。Green 后测试在外层 GORM 回滚事务内创建两个时间顺序最早的合法 Outbox，以 `limit=1` 精确证明首次领取、有效租约期间跳过并领取哨兵、释放后重新领取原事件，不扫描或改写历史无效记录；定向 Eventing package 通过，测试前后全库 Outbox `记录数,attempt 总数,非空 claim 数,published 数` 严格保持 `46,18,10,4 → 46,18,10,4`。
- Kafka/PostgreSQL 测试隔离 Green：两条原本直接提交随机 Workspace/Project/Inbox/Checkpoint/DeadLetter/Outbox 的真实旅程已改为固定库外层 GORM 回滚；Broker 断连恢复用本事务内时间最早的合法 Outbox 与 `batch=1`，只领取本次事件，不扫描历史非法 pending 批次。`LANVERSE_TEST_DATABASE_URL='host=/tmp dbname=lanverse_test sslmode=disable' LANVERSE_TEST_KAFKA_BROKERS='127.0.0.1:9092' go test ./tests/eventing -count=1 -v` 四条真实 Kafka 旅程及完整 Eventing package 全部通过（25.558 秒），Workspace、Project、Outbox、Inbox、Checkpoint、DeadLetter 六类数量保持 `343,333,52,19,9,5 → 343,333,52,19,9,5`。
- Source Evidence/Story Analysis 固定库隔离 Red/Green：全量 Backend 测试定位到 `TestSourceEvidenceAndStoryAnalysisWorkflowRecoverBoundedMapReduce` 每次向固定库提交一整套 Workflow 事实与四条 Outbox。尝试用外层事务回滚的 Red 因旅程主动验证数据库约束失败而使 PostgreSQL 事务进入 aborted 状态，不能作为真实解法。Green 改为测试内按随机 user/workspace/project 精确限定，使用 `tests/platform/adapter/gormdb` 内的 GORM Harness 及正式 Schema Catalog 反向删除本次夹具；不清库、不手写 SQL、不影响其他项目。最新真实 PostgreSQL + Temporal 旅程 49.50 秒通过，Workspace、Project、Outbox 计数严格保持 `343,333,52 → 343,333,52`。
- GORM 测试 Harness/Schema Catalog Red/Green：第一次 Backend 全量回归的真实 Red 为 `TestDatabaseArchitectureBoundaries` 拒绝 Workflow 测试直接导入 GORM。Green 将夹具清理移入测试目录下的 `adapter/gormdb`，未给守卫添加例外；同时第二个 Red 证明 Catalog 原顺序会在 `GenerationIntent` 之前删除被其 `RESTRICT` 引用的 `CommandReceipt`。Green 将 `CommandReceipt` 放到所有依赖者之前，保留正式外键并使 Catalog 的建表/反向清理拓扑一致。架构、语义命名、`go vet ./...` 与真实 Source/Temporal 旅程均通过。
- Workflow 固定库全包隔离 Red/Green：全包回归前的代码审计证明 `TestExpiredInvocationIsReclaimedAndStaleResultIsFenced` 会把固定库全部历史 `draft_storyboard` queued/running Invocation 更新为 failed，Compiler 共用种子、Node Cache 并发测试与跨进程 Bible Worker 又会永久留下 Workspace 和完整 Workflow 事实。Green 将单连接旅程放入外层 GORM 回滚事务，保留 Node Cache 的真实多连接并发并用精确 Workspace Harness 清理，共用 Compiler 种子由 Workflow 测试包按随机 user/workspace/project 三元组统一登记和反向清理，跨进程 Bible Worker 独立登记同一精确范围。首次 Green 还真实暴露 Harness 复用 GORM 查询链使 User 删除继承 Workspace 条件；修复为每次删除使用独立 Session，并对 Workspace/User `RowsAffected` 做严格等值校验。`go test -json -count=1 ./tests/workflow` 在真实 PostgreSQL、Temporal 和本机 Kafka 上为 99 passed、4 条 CI 明确允许的 subprocess helper skipped、0 failed、156.318 秒；仓库 skip verifier 通过。User、Membership、Workspace、Project、Outbox、WorkflowRun、NodeRun、AgentInvocation、SceneAnalysisInvocation 九类事实严格保持 `554,444,343,333,52,150,729,201,48 → 554,444,343,333,52,150,729,201,48`。
- StoryGraph 固定库全包隔离 Green：线性发布、真实多连接 CAS 并发、授权边界和 Query 旅程继续使用正式 GORM Adapter 与固定 `lanverse_test`，并按每次随机 user/workspace/project 及额外 viewer/outsider 身份做精确反向清理；回滚旅程保留原事务验证。`LANVERSE_TEST_DATABASE_URL='host=/tmp dbname=lanverse_test sslmode=disable' go test ./tests/storygraph -count=1 -v` 全部通过（13.194 秒）；User、Membership、Workspace、Project、StoryGraphVersion 与 Outbox 六类事实严格保持 `554,444,343,333,32,52 → 554,444,343,333,32,52`。该证据仅证明旧 StoryGraph 回归的固定库隔离，不抵扣 `VP-I04` 的 storygraph-production 投影实现与验收。
- 其余 Backend 固定库隔离 Green：Review 8 个测试、Authoring 17 个顶层/子测试、Agent 25 个顶层/子测试、Production Bible 24 个测试、Cost 12 个测试和 Quota 3 个测试均在真实 PostgreSQL 上通过；每包的 User/Membership/Workspace/Project 及对应 HumanTask、Authoring、Candidate、Cost、Quota、Receipt 事实运行前后严格不变。真实多连接 CAS、Claim/Expiry、Repair Apply、Budget/Quota 竞争均保留，未用单连接伪造并发。
- Generation 固定库隔离 Red/Green：首轮 Cost 全包真实暴露两个未隔离旅程留下 4 个 Workspace/5 个 Project；修复后共用 Preparation fixture 在一个入口登记 Workspace、用户、项目、Workflow、Provider、Cost、Quota、Generation 和测试专用 NodeCatalog 的精确清理。架构门还真实拒绝过 Generation 测试直接导入 GORM；最终 NodeCatalog 清理收口到 `tests/platform/adapter/gormdb`，未添加守卫例外。原 PostgreSQL/Temporal 全包为 34 passed、3 条 MinIO 条件 skip、1 条 subprocess helper skip、46.613 秒；本轮注入本机既有 MinIO 后，三条 Generation MinIO 旅程已在全量 Backend 门中真实执行，不再产生条件 skip。18 类代表 SQL 事实严格零增量，独立 Generation GORM Adapter 包通过，User/Membership/Workspace/Project/Artifact/Candidate 保持 `554,444,343,333,12,12`。
- 本轮固定库处置：早期 Backend 全量回归由范围外旧测试新增 `56` 个 Workspace、`55` 个 Project 和 `6` 条 Outbox。仅针对本轮新增集合，以 PostgreSQL 事务区间、测试名称白名单、完整 Membership 集合与保留域测试账号四重校验后使用 GORM 精确删除，计数从 `399,388,58` 恢复为 `343,333,52`。Cost 首轮又仅删除本次刚创建且由随机 UUID、测试域账号、完整 Membership 和项目归属同时证明的 4 个 Workspace，计数恢复为全量前基线。这些删除不可恢复；历史夹具、两条非法 `SeedEvent` 与 10 个已过期 claim 未触碰。
- MinIO 真实旅程与精确清理 Green：不读取 `.env`，先验证仓库公开开发凭据被本机 Homebrew MinIO 明确拒绝，再以 MinIO 官方默认凭据连接已运行的 `127.0.0.1:9000`。固定使用 `lanverse-integration-test` 测试桶，不按测试创建新桶；Platform 1、Asset 1、Generation 3 共 5 条真实旅程全部通过。`tests/platform/adapter/minio` 只登记本次随机对象键，逐键删除并在删除后 `StatObject` 断言 404，不扫前缀、不删桶、不触碰历史对象；业务 `objectstore.Client` 未增加测试专用接口。
- 语义化命名约束 Green：项目自有 API、索引、Topic、目录、类型、配置键和文档阶段名继续由 `TestProjectContractsUseSemanticNames` 扫描并拒绝任意 `v数字` 发布序号；覆盖 Backend/Agent/Frontend 业务代码与测试、Backend Observability、Frontend 脚本、项目级配置、Compose、Dockerfile、`.github` 和全部正式文档。Codex CLI 上游固定能力键改由语义化 `_CODEX_DELEGATED_AGENT_FEATURE` 封装，项目代码不再把其外部数字版本名当作内部能力名；实际传给上游的契约值未被伪造。官方 Go module 主版本路径、SemVer、GitHub Action、构造器、npm 包名和 API 标识仍只作为不可改写的第三方事实豁免。Backend 架构门及 Agent 定向 14 tests 通过。
- Go 质量与供应链 Red/Green：本机原始 Go `1.26.5` 执行 `govulncheck v1.7.0` 真实发现 8 条可达漏洞，其中 7 条来自标准库、1 条来自 Temporal 间接使用的 gRPC `v1.79.3`。设计事实与 `go.mod` 将最低安全工具链锁为 `go1.26.6`，gRPC 升至修复版 `v1.82.1` 并接受其最小传递依赖更新；没有升级 Homebrew 或新增 gRPC 业务边界。Green 后 `govulncheck` 为 0 个可达漏洞。`goimports v0.49.0 -l .` 输出为空、`go vet ./...` 通过；`golangci-lint v2.13.2 --new-from-rev=HEAD` 为 0 issues。完整存量扫描另识别 433 条历史 `ST1005`，主要是正式领域名开头的旧错误文本，本切片未以一次性 433 行行为文本改写伪造小提交，该存量不能报告为全仓 lint 通过。Backend 全包 Race Detector 在真实 PostgreSQL、Temporal、Kafka、MinIO 上通过且无竞争，Generation 145.708 秒、StoryGraph 34.686 秒、Workflow 284.390 秒；最终全包普通回归也通过。
- Elasticsearch 客户端 API 迁移 Red/Green：上游已废弃的 `NewBaseClient(Config)` 切换到 `NewBase(Option...)` 后，首个无环境 HTTP 契约 Red 证明 `WithRetry(0)` 实际发出 4 次请求，改变了原有“不自动重试”围栏语义。Green 改用 Elastic Transport 正式 `WithDisableRetry()`，同一测试同时证明 Basic Auth 仍传播且 503 只发出 1 次请求；目标普通测试与 Race 均通过。架构门同步将 `elastic-transport-go` 限定在 `search/adapter/elasticsearch`，不得进入 Domain/Application。
- Workflow 完整包回归与 CI 诊断 Red/Green：远端两次 Backend 失败均精确为正式 Generation 跨进程恢复用例在共享 55 秒总上下文内等待/读取 `execute-node` Activity History 超时，不是整个 Job 超时。同一用例在固定 `lanverse_test` 单独连续 3 次均约 13 秒通过；修正后的当前工作树再次与全部前序 Workflow 用例串行执行，Workflow 完整包 159.163 秒通过，正式 Generation 恢复 13.01 秒、Reference Asset 恢复 12.82 秒、Human Wait 恢复 5.19 秒、Bible Worker 恢复 3.37 秒。根因是该测试把 95 表 GORM Catalog 准备、业务种子与两段 Worker 恢复共用一个累计期限；Green 将准备和恢复分成两个各 40 秒的有界阶段，不改动任何生产 Activity、Provider 或重试时限，修正后目标用例再连续 3 次通过（总计 40.198 秒）。三条共享 Activity 等待路径已在 History 读取或等待超时时附带对应 Worker 子进程输出；输出缓冲由测试包内互斥保护，真实 PostgreSQL + Temporal 目标用例再以 Race Detector 通过（18.029 秒），没有以诊断增强引入并发缺陷。
- Deployment 容器诊断 Red/Green：远端两次失败作业均在三个镜像构建和全部环境容器就绪后，Backend 容器于启动后约 0.55 秒退出 1。拓扑审计确认根因：环境 Compose 只把 PostgreSQL、MinIO、Temporal 和 Kafka 端口发布到 Linux Runner 的 `127.0.0.1`，而 Backend 容器通过 `host.docker.internal` 网关访问；Linux 网关不能访问只绑定宿主回环的发布端口。Red 架构测试同时拒绝四个 host 路由并缺失三个内部服务地址；Green 利用 Backend 已加入的同一 `lanverse-environment` 网络，改为 `postgres`、`minio:9000`、`temporal:7233` 与 Kafka 内部 listener `kafka:19092`，定向架构门通过。当前工作树镜像重新构建通过，并以隔离端口复用本机已有 PostgreSQL、Temporal、MinIO 与 Kafka 真实启动；即使 Elasticsearch 不可用，API 仍 Ready、Workflow Worker 仍注册，证明 ES 不是 API 启动阻断。原 CI 在 `compose up --wait` 失败后立即删除容器，没有保留 Backend 日志；现已改为先输出完整服务状态与 Backend 日志再保持红灯退出，未增加 fallback 或忽略失败。
- Backend 全表零增量 Green：本轮前一次带前后快照的回归注入固定 PostgreSQL、Temporal、本机 Homebrew Kafka 与已运行的 Homebrew MinIO，与 CI 一致执行 `go test -json -count=1 -p 1 ./...` 并把结果直接送入仓库 skip gate。Go 测试未出现失败；5 条 subprocess helper skip 属于 CI 明确允许项，真实基础设施 skip 仅剩 ES/Logstash 4 条。对 `lanverse_test` 全部 95 张正式 SQL 表逐表执行精确 `count(*)`，前后快照 SHA-256 仍均为 `9c0515d83761e87c026deed07e17ab1e5fc9264033d7e7ad05bd00cb32847d94`；未创建替代数据库、Schema 或 migration。随后当前工作树复跑仍由各包 Owner cleanup 与 Workflow `TestMain` 正常退出，开始前未重复生成计数快照，因此不把单边当前计数哈希伪报成第二次零增量证据。
- 全量 CI：当前远端仍未通过，不能作为完成证据。最新远端 run `34248332432` 对应旧提交 `05ae24df`：Backend 原生 Go→Agent 交接超时，Deployment 的 Agent 镜像断言因 shell/Python 引号错误产生 SyntaxError。Red 诊断进一步证明 Temporal Worker 使用 lazy client 会在 Agent lifespan 启动阶段被 SDK 拒绝，环回 HTTP 继承宿主代理且基础命令服务错误使用依赖 Codex 的 `/readyz`。`5f997fb1`、`e99a1860`、`0bf9ddcf` 分别固定健康契约、改用已连接 Temporal 客户端并修复镜像配置/引号，没有延长等待、增加 fallback 或兼容入口。本机复用既有 PostgreSQL、Temporal、MinIO、Homebrew Kafka、Elasticsearch、Kibana 与 Logstash 后，Backend 32 个包全部通过且 skip gate 为 0 个非预期跳过；Agent 为 194 passed、6 个显式真实 Codex 长评测 skipped，原生交接 2 passed；Frontend 69 tests、OpenAPI 无漂移和生产构建通过；三类镜像按顺序构建并通过唯一 Backend 入口、Frontend standalone、Agent 非 root/Codex/Skill Bundle 断言。未启动或重启任何环境服务。当前 HEAD 尚未获准推送，因此没有新的 GitHub Runner 结果。
- 真实输入/产物/事实对账：输入 `第一场 夜 内\n林舟握住门把。\n第二场 日 外\n林舟离开。`；PostgreSQL 保存同一 SourceVersion/SourceSpanIndex/Head/Receipt、全 code-point 覆盖的两个 ScriptSpan、两个 style-blind SceneFact 及不可变 Invocation/Attempt/Result/Candidate lineage。Agent 无数据库连接，正式 UUID、Head、Receipt 与 Candidate 接受均由 Backend/GORM 写入。
- 未覆盖条件与残余风险：现有 Elasticsearch、Logstash 与 Kibana 已恢复健康，本轮 Backend 全量门使用正式 `lanverse-script-search`、`lanverse-storygraph-search`、`lanverse-logs-application` 与 `lanverse-logs-dead-letter` 别名完成真实验证，没有按测试创建业务索引。当前剩余完成门是把领先 `origin/main` 的提交推送后获得当前 HEAD 的 GitHub `Required / CI = success`；在此之前不勾选 `VP-I01`、不进入 `VP-I02`。历史两条非法 `SeedEvent` 和 10 个已过期 claim 未授权前仍不删除；全部开发完成前不运行 `agent-browser`。

### `VP-I02` — 全剧身份调和、Gate 1 与正式结构

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I03` — 制作世界、人物多形象、道具交互与 Gate 2

- 状态：进行中；三阶段 strict Candidate、ProductionWorld 确定性组装、Gate 2 冻结六视图审核输入、`approved|changes_requested|rejected` 决议、局部 repair Run、再次审核及三个 Owner family 原子 Apply 已形成真实 Temporal 闭环，`VPR-WLD-001`–`011` 已全部关闭。`VP-I03` 仍待当前提交的远端全量 CI 与真实 Codex CLI 语义质量验证，不以合同测试代替模型输出质量。
- Git 基线/提交：基线 `b7e5b67e`；ProductionWorld 集合证明 `05088bfd`、三域原子确认 `e1e83bf4`、人工确认接线 `15573e1d`、真实 Temporal Gate 2 恢复 `7e66478b`；六视图 Review Detail 证据随本记录所在提交交付。
- Red 命令与失败：前三阶段的 schema/contract Red 已记录；本轮真实 Gate 2 repair 首先因 Review clone 把合法的 `issue_refs: []` 降为 `nil` 而以 `invalid Production World change request issue references` 拒绝，修复空集合语义后又因 repair base 错读聚合候选表而出现 `Workflow resource not found`。两项均由真实 PostgreSQL + Temporal 路径发现并修复，未通过放宽校验或伪造测试绕过。
- Green/定向验证：`TestSceneAnalysisWorkflowPersistsStructureIdentityReviewAndReplays` 在真实 PostgreSQL 下验证 Gate 2 Signal → 三域事务 → Apply Receipt、强制冲突全回滚、幂等重放及冻结 Candidate 的六视图查询；`TestProductionWorldReviewDetailHasSixTypedViews` 验证角色/形象、地点、道具/状态、场景出现、交互和连续性恰为六个 typed 视图，空视图也保持空数组而非缺失；Review HTTP 与 OpenAPI 测试证明公开详情不返回内部可变状态且生成客户端拥有精确字段。`TestSceneAnalysisGatesAndBoundedRepairsResumeRealTemporalWorkflow` 在本机 PostgreSQL + Homebrew Temporal 下通过，验证 Gate 1 修复后继续生产、Gate 2 Interaction typed ChangeRequest、唯一根节点重跑、未受影响阶段复用、新 Gate 2 冻结、批准、`ConfirmProductionWorld` CommandReceipt 与最终 `SUCCEEDED`；同一旅程在 Gate 2 打开时验证全部视觉/生成/StoryGraph/Storyboard 正式表对当前 project 为零，并在 Apply 后核对 UUID 身份与独立 Asset/State 版本。`TestProductionWorldAssetOwnerPublishesOneIdentityWithMultipleStatesInCallerTransaction` 在真实 PostgreSQL 下证明新增 Appearance/State 不覆盖既有身份和状态版本。
- 全量质量门禁：当前切片通过 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`；真实 PostgreSQL + Homebrew Temporal 旅程以 `go test -race -v ./tests/workflow -run '^TestSceneAnalysisGatesAndBoundedRepairsResumeRealTemporalWorkflow$' -count=1` 明确执行通过且未跳过。此处只记录当前功能切片的真实性门禁，不代表整个 SOP 或 `VP-I03` 已完成。
- 真实输入/产物/事实对账：十一节点 Workflow 依次形成 Source、Span、Scene Fact、Identity、Structure/Identity Review 与 Gate 1、Production Entity、Scene Binding、Interaction/Continuity、ProductionWorld 和 Gate 2；Gate 2 只消费冻结的 aggregate/upstream Candidate 与正式 Head，并由 Backend 唯一写入 ProductionWorldVersion、SceneOccurrenceVersion、ContinuityVersion 及各自 Head/Receipt。Agent 仍只写候选。
- 局部闭包阶段证据：`TestProductionWorldRepairClosureStopsAfterDirectContinuityNeighbour` 证明从 Scene 1 的 Occurrence 修复只包含 Scene 1、直接 Interaction 和 Scene 1→2 Continuity 邻接，不把 Scene 2→3 的下一跳 Scene/Occurrence/Continuity/Ledger 带入；operation-specific 与乱序输入测试分别覆盖四类 typed root、错误 key/type 拒绝和输出确定性。真实 Temporal 旅程已补足 repair Run 执行证据。
- 冻结 inventory 阶段证据：Gate 2 Contract 从同一 ProductionWorldCandidate 机械列出 Entity/State、Scene/Occurrence、Interaction、Continuity 四组 target；target 被添加到冻结 Candidate 之外时 Gate Input 解码失败。公开 Review Detail 与 OpenAPI 仅返回这份服务器 inventory；恢复链路接通后 Gate 固定开放 `approved|changes_requested|rejected`。
- ChangeRequest 合同阶段证据：`TestProductionWorldChangeRequestMatchesFrozenTargetEvidenceAndClosure` 证明 request 必须命中冻结 target、operation-specific reason、至少一条闭包内 Evidence 和服务器完整闭包，扩大 scope、伪造 Evidence、错型 target 或以 `user_note` 替代 Evidence 均被拒绝；`TestProductionWorldRepairRootFollowsTheTypedOperation` 证明 root 只能落到三个既有制作阶段之一。Backend/Agent 共享的 `ProductionWorldRepairDirective` 冻结每个执行阶段的精确 base Candidate、闭包和证据，内置 Skill 要求返回完整 Candidate 且不得改动闭包外内容；Review API 已通过同一 typed payload 接入。
- 闭包保持阶段证据：`TestProductionWorldRepairCandidatePreservesContentOutsideAuthorizedClosure` 与 Python 同名参数化测试覆盖 Entity、Occurrence、Interaction、Continuity 四种操作，证明授权对象及直接依赖可改变，而未选实体、Dialogue/Beat、StoryTime、Interaction 和其他业务项保持；Agent Harness 输出后和 Backend AttemptResult 接受前都会机械拒绝越界、业务 key 集漂移与 no-op。真实 Temporal 旅程证明该检查作用于实际 repair Candidate，并形成不同内容哈希的新 Gate。
- 决议持久化与路由阶段证据：Review Store 对 `production_world_gate_input` 重新读取同一 PostgreSQL Gate、aggregate Candidate 与六视图后才接受 typed ChangeRequest；DecisionReader 不把它误解为 Structure/Identity 请求，Coordinator 只创建一个 repair Run，StartService 使用四种 operation 的固定 root 映射。运行时从原 Workflow 同阶段读取精确 Agent Candidate，通过 GORM 组装 directive；Interaction 修复只执行 `reconcile_interaction_continuity`，上游 Production Entity 与 Scene Binding 显式复用。
- 未覆盖条件与残余风险：尚未执行真实 Codex CLI 语义质量验证；当前网页仍缺少把 Gate 2 六视图 target、服务器闭包和 Evidence 组装为 typed ChangeRequest 的完整交互，`VP-I11` 的六视图工作台与 `VPR-FE-005` 仍未完成。最终 `agent-browser` 验收按约定待全部开发完成后执行。

### `VP-I04` — storygraph-production 制作投影与可追溯 Query

- 状态：进行中；Gate 2 正式 Owner 读取、Temporal DAG 的 P0 制作投影、原子发布、有界 production Query、production Node 合同门、Schema/Payload Hash Root 元合同、跨 Go/Python Production Canonical JSON、完整 Registry fixture/冻结 Hash，以及 Schema/Coverage 编译前像与离线重放校验均已接通；P0 Bible Fact/Causal Claim、完整 production Edge vocabulary、基础 AssetVersion、Plan 内六类 Reference Target 与 Scene/Interaction Reference Binding 静态关系合同已收口，后续 expected target coverage、Shot 等 Schema rank invariant 尚未完成
- Git 基线/提交：Compiler 输入合同 `06358b09`；原子发布见 `506edd53`；DAG 接入见 `89aad81b`；production Query 见 `6d4d9ccd`；Node 合同门见 `ff26d1c0`；Evidence 关系门见 `651653bf`；身份/状态/Occurrence 关系门见 `6e6a06a0`；引用投影门见 `4ef4d21e`；结构顺序门见 `8e6faacf`；连续性状态门见 `e74e14fd`；人物道具交互门见 `e7f43431`；Bible Narrative Claim 见 `eea0891d`；Schema/Payload Hash Root 元合同见 `1eee32dc`；Canonical JSON 修正见 `backend/internal/platform/canonical` 与 `agent/app/protocol/canonical.py`
- Red 命令与失败：Production Owner 合同测试先拒绝缺集合、跨 Project、`current/latest`、Candidate 污染、Collection root 漂移和节点越界；发布测试固定重复幂等键输入漂移与 production→legacy 降级必须零写入；Node 合同 Red 首先因正式 Payload Contract resolver 不存在而编译失败，随后固定错误 Owner Family、被排除节点、错误 Payload Contract、projection hash 漂移、缺失/越界 Evidence 和未知 continuity discriminant 必须拒绝；Evidence 关系 Red 证明旧实现会接受缺失 Scene 证据边、Source 绕过 Evidence 直连 Episode 和无 Source Root 的孤立 Evidence；身份关系 Red 证明旧实现会接受 Binding 缺/重复 State Ref、未知 Payload 字段、状态 kind 漂移、缺失 materialization，以及 Occurrence 指向 Episode 冒充 Scene 或缺失 Scene anchor；引用投影 Red 证明旧实现会接受 production Node 复制显示名、`business_position` 和 ref-only Payload 内的 Owner 摘要；结构顺序 Red 证明旧实现会接受 Scene 无父级、重复结构位置、缺失相邻 Scene/Beat 顺序和伪造 target SequenceKey，端点矩阵还会拒绝合法 Episode/Beat 顺序；Continuity Red 证明旧实现会接受缺失/未知主体字段、`state_persists` 冒充状态变化，以及缺失 subject/state/anchor Edge；Interaction Red 证明旧实现会接受缺失 actor、未知业务字段、错误 Prop Occurrence、失效 holder 转换，以及缺失 Prop participant/Character anchor Edge。
- Green/定向验证：`go test ./tests/storygraph -count=1` 通过；真实 PostgreSQL `TestSceneAnalysisWorkflowPersistsStructureIdentityReviewAndReplays` 通过，证明 Gate 2 后同一角色的多 Appearance/State 仍收敛到唯一 ProductionBinding，每个实际 Occurrence 精确解析到该身份、状态与 Scene，普通 Continuity 保存精确主体、前后 State、跨场 Anchor 与 story time，Interaction 保存 Character/Prop Occurrence、holder 转换、Prop 前后 State、Scene/Beat 与几何字段，且全部 Payload/Edge 双向等价；同一三场旅程逐节点经过 production 合同门，证明 Episode/Scene/Dialogue/Beat 不复制 Owner 显示内容，并从正式 Owner 顺序生成 Scene/Beat 相邻链。该真实数据库旅程还在首次发布后改变正式 Asset Head 的 Collection Root，再以新的幂等键提交旧 Production World Receipt；Compiler 返回 `invalid_owner_snapshot`，StoryGraphVersion 数量保持不变。真实 Homebrew Temporal `TestSceneAnalysisGatesAndBoundedRepairsResumeRealTemporalWorkflow` 通过，证明 Gate 2 Interaction repair 后 `reconcile_interaction_continuity` 的新 Candidate 能经人工审核、正式 Owner Apply、production StoryGraph 编译与发布后完成 Run。production 节点同时满足 Owner/Family、Payload Contract、projection hash、Evidence policy 以及 Envelope→唯一 Evidence Node→Source Revision 的精确边等价；production Query 继续验证 current Version 非误报 stale、Scene depth=4 Impact 和 Claim depth=2 upstream trace 均有界且可反查 Evidence/Identity/State/Occurrence/Claim/Binding。
- Bible Narrative Claim 阶段证据：Go/Python Candidate 合同均拒绝旧的无角色主体数组、缺失 narrative、非唯一 subject、未知正式身份、未知结构 Anchor 与漂移 Scope；Production Bible Owner 以 GORM 保存 typed participants 和 narrative，并要求 revision 2 起逐次引用同 Claim type/series 的前一版本。StoryGraph 发布前 strict relation 门拒绝非法 predicate、Owner 正文复制及 participant/anchor Edge 漂移。真实 PostgreSQL 旅程生成并落库 Relationship Claim，随后机械投影出 `relationship_claim`、`claim_participant` 与 `claim_anchor`；真实 Homebrew Temporal Gate 2/repair/发布旅程继续通过。两条真实旅程均复用固定 `lanverse_test` 与本机服务，未创建替代数据库、migration 或环境容器。
- Schema/Payload Hash Root 元合同阶段证据：Red 先因严格 Decode API 不存在而编译失败；补齐后继续以负例证明旧实现会接受缺失 required nullable 字段、未知字段、嵌套数组乱序、Matrix row 跨联合类型、派生 Rule ID 漂移、Node 声明的 Payload 未被 Union 覆盖、Payload 根合同漂移、多基础类型、Registry 外 format、Array format、Nullable Set 漂移和 Array 缺失唯一排序来源。Green 后同一语义的 JSON Object 字段重排得到相同 Canonical SHA-256；公开 API 名称和注释明确该元合同只验证 Hash Root 形状，不在完整 Registry fixture 建立前授权 production 发布。
- Production Canonical JSON 阶段证据：共享 fixture 的 Red 证明旧 Go `encoding/json` 路径无法满足非 BMP/BMP key 的 RFC 8785 UTF-16 排序且会额外转义 U+2028/U+2029，旧 Python `sort_keys=True` 同样按 Unicode code point 而非 UTF-16 排序；Go 还缺少稳定错误码，Python 缺少保留重复键的 Raw JSON 入口。Green 后两端对 NFC 中文/emoji/`<>&/`/U+2028/U+2029/控制字符、安全整数边界产生相同 Canonical bytes 与 SHA-256，并以同一错误码拒绝浮点、指数整数、越界整数、原始/转义/归一化重复键、孤立 surrogate、非法 UTF-8 和尾随文档。Go 只引入 Apache-2.0 的 `github.com/gowebpki/jcs v1.0.1` 负责 RFC 8785 序列化；项目自身仍在调用前执行严格 Unicode、唯一键、NFC 和整数边界门。
- 完整 Registry 阶段证据：Red 首先因 `BuildProductionSchemaRegistry` 不存在而编译失败，共享 fixture 测试随后因文件不存在失败；Green 后 Backend 从既有 production Node 声明和 Design Registry 机械构建 31 个 Node、32 条 Payload Union、25 个唯一 Payload Contract、7 个 Edge Alias、65 条 Matrix、13 个 Owner Collection、4 条 Coverage、13 个 Checkpoint 与 22 条 Exclusion。每个 Payload 的固定字段、nullable、嵌套 Ref、数组基数/排序与 invariant 先经严格元合同解码，再以 Production Canonical bytes 实算 Hash；Manifest 只引用这些实算 Hash，并得到 Schema Hash `26b73dff931d3edb543b1160a9376c6d019b5244e13eb34c6d32a5418bc8e126`。Go 对声明与共享 fixture 做全量一致性校验，Python 独立重算 25 个 Payload Hash 和最终 Schema Hash，并按 Design 规定的 GFM inline plain-text 规范逐格核对 65 条 Matrix 与 13 个 Owner Collection 单元格；未读取 Markdown 充当运行时 Registry，也未保留手填 Hash 或旧 Hash 兼容路径。
- Owner Collection Root 合同阶段证据：Red 先因共享合同包不存在而编译失败；Green 后 `internal/platform/ownercollection` 以 Production Canonical JSON 固定 NFC、UUID、安全整数、精确 8 字段 `OwnerVersionRef`、四元组排序去重、Scope Hash、裸成员数组 Hash 和排除自身的完整 Collection Root Hash。StoryGraph production Compiler 已实际调用该合同，内部 `created_at` 不再进入编译前像；固定向量、非法跨 Scope/family、非 NFC、越界修订、重复成员和 StoryGraph wire 闭集测试均通过，定向 Race Detector 通过。该阶段尚未证明各正式 Owner Writer/Receipt 已全部切换同一 Root。
- Script Source Writer Root 阶段证据：Red 先因 `BuildSourceCollectionRef` 不存在而编译失败；Green 后 `production/script` 从同一接受事务内的 DocumentRevision、SourceSpanIndexVersion 与 Head revision 构造共享 `script_source_set`，Receipt 保存精确 8 字段成员前像，不再写入 `created_at` 或自定义 Collection Schema 包装。Compiler 在同一共享锁事务中重读该 Receipt、Source Head 与两个正式版本并重算成员、Scope 和 Collection Root，任一不等价返回 `invalid_owner_snapshot`。无服务单元/全量 Backend 门通过；固定 PostgreSQL 的 Source 持久化测试与完整 Gate 1/2 → StoryGraph 发布旅程通过，三组定向测试均以 Race Detector 复验；未启动或重启环境。
- Project Episode Writer Root 阶段证据：Red 先因不可变 `EpisodeOwnerVersion`、Scope Membership/Head 与 Collection Receipt 合同不存在而编译失败；Green 后 Gate 1 第一条 Owner 命令在同一 GORM 事务写入线性 Episode Version、逐 scope revision 的完整 active Membership、CAS Scope Head、正式 `project_episode_lifecycle_confirmed / project_episode_set` Receipt 与独立 Command Receipt。Collection 成员以稳定 Episode ID 为 logical id、以不可变 Episode Version ID/hash 为精确版本引用，`covered_scope_keys=[]`，不再把可变 Episode ID 或 Script 内容 hash 冒充 Owner Version。Bible Gate 1、Production World Gate 2 与 StoryGraph Compiler 均重读正式 Receipt/Head/Membership/Version 并用共享算法重算全等；固定 PostgreSQL 测试证明首发、幂等重放、失败全事务回滚、下一 revision 父链和未变化 Version 跨 Membership 复用，完整 Gate 1/2 → StoryGraph 旅程还证明篡改 Project Episode Head 后返回 `invalid_owner_snapshot` 且零新增 StoryGraphVersion。两条真实旅程及 Race Detector、Backend 同 CI 边界均通过；未启动或重启环境，未新增 migration 文件。
- Structure Identity Writer Root 阶段证据：Red 先因 `BuildStructureIdentityCollection`、完整 Scope Head 与 Collection Receipt 构造器不存在而编译失败；Green 复用既有不可变 `StructureIdentitySetVersion`，以 Project ID 为 logical id、Version ID/hash/revision 为精确成员，只通过共享 `ownercollection` 计算 `bible_structure_identity_set` 的 Scope/Member/Collection Root。Gate 1 第二条 Owner 命令在同一 GORM 事务写 Version、完整 Head、正式 Collection Receipt、独立 Command Receipt、Audit 与 Outbox；公开 Query、Gate 1 后续 CAS、Gate 2 正式读集、Production World 确认和 StoryGraph Compiler 均从版本与 Receipt 前像重算并全等校验。固定 PostgreSQL 首发/幂等/回滚旅程和完整 Gate 1/2 → StoryGraph 旅程通过；后者篡改 Structure Receipt Root 后返回 `invalid_owner_snapshot` 且零新增 StoryGraphVersion。两条旅程及本机 Homebrew Temporal 有界 repair/恢复旅程均通过 Race Detector；期间真实发现测试清理顺序违反 `ProjectEpisodeVersion → EpisodeScriptVersion` 的 `RESTRICT` 外键，修正唯一 GORM Catalog 顺序后复跑及精确遗留 fixture 清理均成功。未启动或重启环境，未增加 migration、替代数据库或兼容双读。
- Production World Bible Writer Root 阶段证据：Red 因 `BuildProductionWorldBibleCollection` 与 Head 完整 VersionRef 不存在而编译失败；Green 让既有不可变 `ProductionWorldBibleVersion` 以 Project ID 为 logical id、精确 Version ID/hash/revision 调用共享 `ownercollection`，Head 持久化 Scope、成员前像与同一 Collection Root。Gate 2 Coordinator 创建 `production_world_confirmed / bible_production_world_set` Receipt 前重算 Version 与 Head 全等；StoryGraph Compiler 在发布事务内重读正式 Version、Head 和 Gate 2 Receipt，分别重算共享 Root 与 Head hash，拒绝自报 hash 或非精确成员。固定 PostgreSQL 完整 Gate 1/2 → StoryGraph 旅程与本机 Homebrew Temporal repair/恢复旅程均通过 Race Detector；未启动或重启环境，未增加 migration、替代数据库或兼容双读。
- Production World Planning Writer Root 阶段证据：Red 因 `BuildProductionWorldPlanningCollection` 与 Head 完整 VersionRef 不存在而编译失败；Green 复用既有不可变 Scene/Dialogue/Beat/Occurrence/Claim Fact 和 Episode Membership/Head，以业务键为 logical id、Fact ID/hash/revision 为精确成员，按每个 active Episode 调用共享 `ownercollection` 计算 `planning_scene_set`。Head 的既有 JSON 列直接持久化完整 VersionRef；Gate 2 Coordinator 从本次正式 Fact 重建 Collection 后才签逐 Episode Receipt，StoryGraph Compiler 在发布事务内重读全部正式 Fact，重建 Collection 与 Head hash 并对账 Receipt，拒绝自报 Root、缺成员或 Head 漂移。领域合同、Planning/StoryGraph/Bible 相关包、固定 PostgreSQL 完整 Gate 1/2 → StoryGraph 旅程与本机 Homebrew Temporal repair/恢复旅程均通过，其中两条真实旅程使用 Race Detector；未启动或重启环境，未增加 migration、替代数据库或兼容双读。
- Asset Identity/State Writer Root 阶段证据：Red 因 `BuildIdentityStateCollection`、Head 完整 VersionRef 和身份加状态的精确成员计数不存在而编译失败；Green 复用既有不可变 `Asset`、`AssetState`、Membership 与 Scope Head，让每个 AssetIdentity 和每个 AssetState 都以各自 logical id、Version ID/hash/revision 进入共享 `asset_identity_state_set`，同时保留 Membership 只表达 State 属于 Identity 的关系。Head 的既有 JSON 列直接持久化完整 VersionRef，`member_count` 精确等于 Identity + State 版本数；Gate 2 Coordinator 从本次正式 Version 重建 Collection 后才签 Receipt，StoryGraph Compiler 在发布事务内重读并验证全部 Asset/State Version，重建 Collection 与 Head hash 后对账 Receipt。固定 PostgreSQL Asset Owner 首发/推进/幂等/回滚旅程、完整 Gate 1/2 → StoryGraph 旅程与本机 Homebrew Temporal repair/恢复旅程均通过 Race Detector；未启动或重启环境，未增加 migration、替代数据库或兼容双读。
- Verified Coverage 与 Schema 编译前像阶段证据：Red 首先因完整 Coverage Scope/Receipt/Schema API 不存在而编译失败，并固定缺 Receipt、Collection Root 错配、Gate 覆盖缺口、Committed Owner Version 越界、未知持久化字段和 Schema Manifest Hash 漂移必须拒绝。Green 后 Backend 从 `source_revision_accepted`、`project_episode_lifecycle_confirmed`、`gate_1_structure_identity` 和 `gate_2_bible_continuity` 正式 Receipt 构造 P0 Proof；Source/Project Receipt 不越权声明 Scene Coverage，Gate 1/Gate 2 Receipt 并集分别精确等于 P0 Scene Scope，六个非空 Collection 各有命中正式成员的 Receipt，合法空 `planning_structure_rebase_set` Root 进入根集合但没有伪造 Receipt。`schema_id/schema_rank/schema_manifest_hash`、完整 Proof、Coverage Hash、稳定 Node/Edge Key 派生 ID 与七个精确 Collection 写入既有 `compilation_input` JSON；OwnerSet、Topology 与 Graph Content Hash 均纳入这些身份。GORM 保存前和读取后都闭集解码并离线重算完整前像，固定 PostgreSQL Gate 1/2 → StoryGraph 旅程证明首次发布、数据库读取、幂等回放和 Query 使用同一可重算 Version；未新增数据库列、migration、兼容双读或替代事实源。
- P0 Bible Fact 与 Causal Claim 运行时合同阶段证据：Red 证明 `world_rule|story_arc|plot_thread` 会接受 Manifest `additionalProperties=false` 明确排除的 Owner 正文和不完整 `creator_decision_ref`，`causal_claim` 会接受未知字段并在缺少 Anchor Edge 时发布。Green 后三种 Bible Fact 只接受 `ProjectionRefPayload + creator_decision_ref`，Evidence 与合法内容定址创作者决定继续严格 XOR；Causal Claim 纳入 Narrative Claim 的同一字段、角色、锚点和 supersedes 双向等价门。`go test ./tests/storygraph -count=1` 通过；固定 PostgreSQL Gate 1/2 → StoryGraph 旅程及本机 Homebrew Temporal repair/恢复旅程均以 Race Detector 通过，未启动或重启环境、未增加数据库字段或 migration。
- Production Edge vocabulary 阶段证据：Red 因 `contains_reference_target`、`depends_on_reference_target`、`plans_reference`、`fulfills_reference_target`、`binds_reference_input`、`binds_reference_output` 及 Constraint/Reference/Informs qualifier 在领域合同中不存在而编译失败。Green 后 Graph 合同可表示 Manifest 全部 23 种 Edge，production 专用端点门逐项约束 source、target 与 role 的一致性，并明确拒绝 production 排除的 legacy `feeds_generation|binds_output`、反向权威边和合法 role 放错端点。`go test ./tests/storygraph -count=1` 与 `go vet ./internal/storygraph/... ./tests/storygraph` 通过；本切片不冒充尚未实现的高阶 payload 基数与跨节点集合等价。
- AssetVersion 关系合同阶段证据：Red 证明旧 Canonicalize 会接受 AssetVersion 未知字段、Purpose/Identity kind 漂移、身份锚点缺失或误用、Composition Artifact 冒充基础素材、Target 输入漂移、`not_generated` Target 产生结果，以及缺失 Artifact/Target/Style 权威边。Green 后 Backend 严格解码 `asset_version` Payload，要求 Identity/Specification/State 命中同一 ProductionBinding、Artifact 属于 `asset_base_reference_set`、Purpose 与基础 Target 的三个单例输入及 Constraint 全等，并从 Payload 精确投影 `materializes`、`fulfills_reference_target` 和 `constrains`。Character Appearance 还必须引用同角色/Specification/Style、不同已声明 State 的 Identity Anchor AssetVersion，且其 Target 唯一依赖该 Anchor 的履约 Target。`go test ./tests/storygraph -count=1`、`go vet ./internal/storygraph/... ./tests/storygraph` 与 `go test -race ./tests/storygraph -run '^TestProductionAssetVersion' -count=1` 通过；READY/Rights/Lineage 仍由后续正式 Asset Owner typed Query 验证，本切片不冒充完整 Reference Plan Coverage 与 phase-aware 履约基数。
- Reference Target 静态合同阶段证据：Red 证明旧 Canonicalize 会接受 Target 未知/缺失字段、重复或漂移 Scene Scope、六类输入矩阵错配、缺少唯一 Plan Parent、Planning/Constraint Edge 缺失、Payload 自依赖而无 Edge、同 Plan 重复业务键，以及 required Appearance 依赖 optional Anchor。Green 后 Backend 严格解码 Target 与七组排序去重 OwnerNodeRef，要求 `coverage_scope_keys[]` 精确等于 Scene Ref 集，Style 与 Constraint 全等；基础 Target 命中合法 ProductionBinding，Composition 的每个 Identity/Specification/State 均参与声明 Binding，全部 Occurrence/Interaction 留在声明 Scene 闭包。业务键按 Design 的 logical ref key 和 Production Canonical JSON 计算，同 Plan 禁止重复；Appearance 只依赖同角色/Specification/Style、不同 State、Scope 超集且 fulfillment rank 不低于自身的 Anchor，Composition 的非 `not_generated` Target 只能依赖同 Plan 基础 Target。六类合法 Target fixture、全 StoryGraph、`go vet` 与定向 Race Detector 均通过；正式 Plan Owner order key、从 P0/P1 Coverage 机械推导 expected target set、Composition 完整闭包逐字节等价与 phase-aware Result 基数仍属后续发布切片。
- Scene Reference Binding 静态合同阶段证据：Red 证明旧 Canonicalize 会接受 Binding 未知字段、Occurrence 或 Character Asset 闭包缺失、基础 Artifact 冒充 Composition 输出、Interaction Target 冒充 Scene Target，以及 Scene 输入、Composition 输出或 Target 履约边缺失。Green 后 Backend 严格解码 Binding 的全部数组，要求 Scene/Occurrence/Interaction/Constraint 与 `scene_composition` Target 逐字节一致；Character/Location/Prop AssetVersion purpose 必须匹配，其 Identity/Specification/State 去重集精确等于 Target production closure，其 `fulfilled_reference_target_ref` 去重集精确等于 Target dependencies。唯一选中 Artifact 必须属于 `asset_composition_artifact_set`，`binds_reference_input|output`、`fulfills_reference_target` 与 `constrains` Edge 精确等于 Payload。全 StoryGraph、`go vet` 与 Scene Binding/Target/AssetVersion 定向 Race Detector 通过；Artifact READY/Rights/Lineage 和 phase-aware Result 基数仍由后续正式 Owner/Coverage 发布门验证。
- Interaction Reference Binding 静态合同阶段证据：Red 证明旧 Canonicalize 会接受 Binding 未知字段、Character Asset 闭包缺失、Character Asset 冒充 Prop、基础 Artifact 冒充 Composition 输出、Scene Target 冒充 Interaction Target，以及 Interaction 输入、Composition 输出或 Target 履约边缺失。Green 后 Backend 严格解码 Binding，要求唯一 `claim_type=interaction` Claim 与 `interaction_composition` Target、Scene、actor/prop/counterparty Occurrence 和 Constraint 精确闭合；另以一个独立合法但混入 Location 参与者的 Target fixture 证明 Binding 仍会因超出 Claim 闭包而拒绝。Character/Prop AssetVersion purpose 和 Identity/State 必须逐一匹配参与 Occurrence，其 Identity/Specification/State 与履约基础 Target 去重集必须等于 Composition Target；Composition Artifact、`binds_reference_input|output`、`fulfills_reference_target` 与 `constrains` Edge 必须精确等于 Payload。全 StoryGraph、`go vet` 与 Scene/Interaction Binding、Target、AssetVersion 定向 Race Detector 通过；Artifact READY/Rights/Lineage 和 phase-aware Result 基数仍由后续正式 Owner/Coverage 发布门验证。
- 全量 CI：当前实现通过 `go test ./tests/architecture -count=1`、`go vet ./...`、`env -u LANVERSE_TEST_DATABASE_URL -u LANVERSE_TEST_TEMPORAL_ADDRESS go test -count=1 ./...`；固定 `lanverse_test` 与本机 Homebrew Temporal `127.0.0.1:7233` 的 `TestSourceEvidenceAndStoryAnalysisWorkflowRecoverBoundedMapReduce` 真实执行通过（58.369 秒），未以空环境跳过；Schema/Payload、Platform Canonical、AssetVersion 与 Reference Target 定向测试另通过 Race Detector。Agent 通过 Ruff lint/format、Pyright 和全量 Pytest `204 passed, 60 skipped`，跳过项均为明确要求隔离 PostgreSQL、本机 Temporal 或真实 Codex CLI 的环境旅程，未冒充通过；Frontend OpenAPI 无漂移、ESLint、TypeScript、25 files/73 tests 与生产构建通过。本机未安装 `goimports`、`golangci-lint` 与 `govulncheck`，未将其误报为已执行。该结果只作为当前 SOP 切片质量门，不抵扣未实现的 VP-I04 条目。
- 真实输入/产物/事实对账：Compiler 只以 Gate 2 `production_world.confirm` CommandReceipt 和其中正式 Collection Receipt 为入口，通过正式 GORM Model 重读 Source、Episode、StructureIdentity、ProductionWorld、Asset/State 与 Planning Scene facts；Workflow 节点只消费 Gate 2 的 `production_world_owner_set`，以 CommandReceipt ID 派生跨 Attempt 稳定幂等键；同一事务落 StoryGraphVersion、线性 Head、CommandReceipt 和 Outbox，Version 的 `compilation_input` 保存可重建的 Schema、Coverage 与 exact OwnerCollections，回放从 PostgreSQL 返回同一前像；新的 Gate 2 Receipt 可沿 production Head 追加，旧编译入口不能降级覆盖；未创建替代数据库、Schema 或 migration，也未启动/重启环境服务。
- 未覆盖条件与残余风险：当前合同门已覆盖完整 Node allowlist、Owner/Family、Payload Contract/discriminant、projection hash、Evidence policy、Evidence Edge、ref-only Payload/no-copy Envelope、Episode/Scene/Beat 结构顺序、P0 身份/Specification/State/Binding/Occurrence、Auditable Bible Fact、Bible/Causal Narrative Claim、普通 Continuity/Interaction 字段、状态机与关系等价、production 全量 Edge vocabulary/端点角色、基础 AssetVersion Payload/Target/Constraint/Edge 等价、六类 Reference Target 的静态输入矩阵/业务唯一键/Planning/Dependency Edge、Scene/Interaction Reference Binding 的 Target/Claim/Asset/Artifact/Edge 等价、发布事务内七类 Owner Head 漂移复验、完整 Schema/Payload Registry Hash Root、共享 Owner Collection Root 算法、全部 P0 正式 Owner Writer/Receipt 对账、完整 P0 `VerifiedCoverageProof` 和底层 RFC 8785 Canonical bytes；Reference Plan 从正式 P1 Coverage 推导的完整 Target 集、Owner order key、Composition 逐字节完整闭包和 phase-aware 履约基数，以及 Shot 的 Payload 闭集与跨节点集合等价仍未完成。当前投影和 Query 是 P0 关系闭环，不宣称 VP-I04、完整 VP-D14 或完整 SOP 已验收。按约定，`agent-browser` 仅在全部开发完成后执行。

### `VP-I05` — Skill 供应链、4–6 Preset、视觉基础与 Gate 3

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I06` — 六类 Target 与 Provider-neutral Brief

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I07` — 图片执行、Bundle、确定性 QC 与 Vision Review

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I08` — Gate 4 基础 Bundle 选择与 checkpoint

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I09` — Interaction/Scene Composition 与局部失效

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I10` — Production Packet、direct_storyboard 与 Gate 5

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I11` — 项目级 Guided Studio 与 typed Query

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I12` — 恢复、安全、可观测与有界闭包

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I13` — 十三 Stage 正式 Release、Eval 与完整剧本真实媒体矩阵

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I14` — 指标、性能与发布候选全量门

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I15` — 最终浏览器验收与事实对账

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

## 7. 最终发布门

- [ ] 126 个 VPR 与 95 个 VPA 表格条款均有同一主切片的新 Evidence，无遗漏、无重复。
- [ ] VP-I01–VP-I15 全部完成，且每个实现切片有独立提交；VP-I15 发生在 VP-I14 提交之后。
- [ ] 至少一份完整真实剧本、4–6 个 Preset、六类 Target、两 Appearance、两 PropState 和一次持道具 Interaction 走通。
- [ ] Production-ready Scene Coverage 由 Backend 计算为 100%，分子/分母和每个不满足原因可查询。
- [ ] Agent、Backend、Temporal、Provider、Gate 和 Asset 关键崩溃窗口完成同身份恢复或明确失败关闭。
- [ ] 越权写入、跳过必需 Target、盲重试 outcome_unknown、跨项目污染、Hash/Wire 漂移和 Secret 暴露均为零。
- [ ] Go、Agent、Frontend、OpenAPI、真实基础设施、镜像、Compose、architecture、hygiene 和最终浏览器旅程均有最新通过证据。
- [ ] 最终 git status 逐项说明用户既有修改；无本目标未提交文件、缓存、日志、凭据或生成产物。
