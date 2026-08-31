# Lanverse 剧本视觉生产升级验收标准

> 状态：VP-D15 已接受（2026-08-31）；全部实现与验收目标初始未通过
>
> 接受依据：221 个 Requirement 表格条款与 Plan 主切片一一映射，无遗漏、无重复、无历史证据抵扣
>
> 正文 SHA-256：02f7ea2e8cc815c64cfb8835f07fcd245722a7e227a53e6182c17c4a18b4eb43
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
- [ ] `VPR-ARC-006`（`VP-I01`）：production 合同按一个实施切片内的 schema、writer、reader、fixture 原子切换；不得长期双写、fallback、latest 补全或静默兼容旧语义。 最低证据：Contract + Migration。
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

- [ ] `VPR-WLD-001`（`VP-I03`）：Backend 机械地把身份事实划分为 confirmed、ambiguous、rejected 三个全集分区；Agent 不得决定正式分区。 最低证据：Unit + Property。
- [ ] `VPR-WLD-002`（`VP-I03`）：角色、角色形象、地点、道具、道具状态和交互的每项事实必须是 Evidence XOR CreatorDecision，且记录来源、作者与版本。 最低证据：Contract。
- [ ] `VPR-WLD-003`（`VP-I03`）：Character、CharacterAppearance、Location、Prop、PropState 使用独立稳定 logical_id 和 version；不得用名称、文件名或提示词充当身份。 最低证据：Integration。
- [ ] `VPR-WLD-004`（`VP-I03`）：同一 Character 可有多个 Appearance；服装、年龄阶段、伤妆、湿身、伪装等改变必须形成独立 Appearance，而非覆盖角色锚点。 最低证据：Journey。
- [ ] `VPR-WLD-005`（`VP-I03`）：SceneOccurrence 精确绑定 scene、subject_kind、subject_id、appearance_or_state_id、evidence 与顺序；同一场景内的出现不得靠全文搜索推断。 最低证据：Contract。
- [ ] `VPR-WLD-006`（`VP-I03`）：InteractionSpec 精确绑定 actor appearance、prop state、动作、手位/身体接触、相对尺度、朝向、连续性和证据；不得仅保存“人物拿道具”的提示词。 最低证据：Contract + Journey。
- [ ] `VPR-WLD-007`（`VP-I03`）：ContinuityLedger 对相邻场景记录 appearance、prop state、损坏、污渍、持有关系和位置的进入/离开状态，并能指出冲突证据。 最低证据：Unit + Journey。
- [ ] `VPR-WLD-008`（`VP-I03`）：Gate 2 Subject 固定绑定 ProductionWorldCandidate、SceneOccurrenceCandidate、InteractionCandidate、ContinuityCandidate 与全部上游正式版本。 最低证据：Contract。
- [ ] `VPR-WLD-009`（`VP-I03`）：Gate 2 接受由一个命令原子发布 ProductionWorldVersion、SceneOccurrenceVersion 和 ContinuityVersion 三个 Owner family；任一失败全部回滚。 最低证据：Integration。
- [ ] `VPR-WLD-010`（`VP-I03`）：changes_requested 可只关闭受影响的 scene/entity shard；闭包必须包含其交互与连续性邻接，不得默认重跑全剧。 最低证据：Closure property。
- [ ] `VPR-WLD-011`（`VP-I03`）：Gate 2 之前不得选择世界观预设、规划参考图、生成媒体、写 StoryGraph 正式版本或生成 Shot。 最低证据：Negative。

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
- [ ] `VPA-P0-004`（`VP-I03`）：ProductionWorldCandidate 严格区分 Character、CharacterAppearance、Location、Prop、PropState，并为每项给出 Evidence XOR CreatorDecision 提案。 最低证据：Schema。
- [ ] `VPA-P0-005`（`VP-I03`）：SceneOccurrenceCandidate 对 scene/subject/appearance_or_state/evidence/ordering 精确绑定，不以名称或 fuzzy search 代替身份。 最低证据：Contract。
- [ ] `VPA-P0-006`（`VP-I03`）：InteractionContinuityCandidate 同时输出人物—道具几何、持有/接触、相对尺度/朝向与跨场 appearance/prop state ledger。 最低证据：Journey。
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

- 状态：实现与定向验证已通过；最新修改后的 skip-free 全量 CI、独立提交与 Requirement 勾选尚未完成
- Git 基线/提交：基线 `14d43fe1`；最终提交待全量门通过后记录
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
- 语义化命名约束 Green：项目自有 API、索引、Topic、目录、类型、配置键和文档阶段名继续由 `TestProjectContractsUseSemanticNames` 扫描并拒绝任意 `v数字` 发布序号。Codex CLI 上游固定能力键改由语义化 `_CODEX_DELEGATED_AGENT_FEATURE` 封装，项目代码不再把其外部数字版本名当作内部能力名；实际传给上游的契约值未被伪造。官方 Go module 主版本路径、SemVer、GitHub Action、构造器和 API 标识仍只作为不可改写的第三方事实豁免。Backend 架构门及 Agent 定向 14 tests 通过。
- Backend 全表零增量 Green：最新回归注入固定 PostgreSQL、Temporal、本机 Homebrew Kafka 与已运行的 Homebrew MinIO，与 CI 一致执行 `go test -json -count=1 -p 1 ./...` 并把结果直接送入仓库 skip gate。Go 测试未出现失败；5 条 subprocess helper skip 属于 CI 明确允许项，真实基础设施 skip 仅剩 ES/Logstash 4 条。对 `lanverse_test` 全部 95 张正式 SQL 表逐表执行精确 `count(*)`，前后快照 SHA-256 仍均为 `9c0515d83761e87c026deed07e17ab1e5fc9264033d7e7ad05bd00cb32847d94`；未创建替代数据库、Schema 或 migration。
- 全量 CI：当前仍未通过，不能作为完成证据。最新 Backend JSON 共 9 条 skip：5 条 subprocess helper 可由 CI 规则允许，仓库 skip gate 精确拒绝 4 条真实旅程，分别是 Observability/Logstash 1 条、Search/Elasticsearch 3 条；MinIO 相关 5 条已全部真实执行。Agent 当前 Ruff/format/Pyright 零错误、57 passed、5 条显式真实 Codex 旅程 skipped；Frontend、OpenAPI、Compose 渲染、三类镜像顺序构建及镜像运行时契约此前已按当前 CI 命令通过，但未运行会启动隔离依赖的 deployment journey。Elasticsearch `127.0.0.1:9200` 当前关闭，Kibana 不健康，且外部 `es-local-dev`/`logstash-local-dev` 均为 Docker OOM 后 `Exited (137)`；固定库两条历史非法 pending `SeedEvent` 未在本次限定旅程中造成失败，但仍是未授权处置的运维风险。未清库、未创建替代库、未启动或重启环境。最近远端 CI 位于旧基线 `794e3ee8`，不能作为当前通过证据。
- 真实输入/产物/事实对账：输入 `第一场 夜 内\n林舟握住门把。\n第二场 日 外\n林舟离开。`；PostgreSQL 保存同一 SourceVersion/SourceSpanIndex/Head/Receipt、全 code-point 覆盖的两个 ScriptSpan、两个 style-blind SceneFact 及不可变 Invocation/Attempt/Result/Candidate lineage。Agent 无数据库连接，正式 UUID、Head、Receipt 与 Candidate 接受均由 Backend/GORM 写入。
- 未覆盖条件与残余风险：待既有 Elasticsearch 与 Logstash 恢复后，再重跑当前 4 条缺条件旅程和 skip-free Backend 全量门；Kibana 仍不健康，后续 deployment journey 也需要其恢复。Search 只使用正式 `lanverse-script-search` / `lanverse-storygraph-search` 别名，不允许每次测试创建新索引。MinIO 5 条真实旅程和全部 95 张 SQL 表均已实现本次全量零增量；历史两条非法 `SeedEvent` 和 10 个已过期 claim 未授权前不删除。全量 CI 通过前不提交、不进入 `VP-I02`，全部开发完成前不运行 `agent-browser`。

### `VP-I02` — 全剧身份调和、Gate 1 与正式结构

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I03` — 制作世界、人物多形象、道具交互与 Gate 2

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

### `VP-I04` — storygraph-production 制作投影与可追溯 Query

- 状态：未开始
- Git 基线/提交：待记录
- Red 命令与失败：待记录
- Green/定向验证：待记录
- 全量 CI：待记录
- 真实输入/产物/事实对账：待记录
- 未覆盖条件与残余风险：待记录

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
