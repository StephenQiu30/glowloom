# 通用媒体 Provider 与 Generation 执行器设计

> 2026-09-07 新流程边界已接受：本文件关于新创作编排、Agent 无存储、仅只读画布的旧限制，按 [0013 架构](0013-创作编排与多媒体画布架构调整设计.md)、[3004 Harness](3004-AgentHarness专业能力与创作流程设计.md)、[1003 画布](1003-多媒体创作画布与运行可视化设计.md) 的明确替代范围调整。Go 保留正式 Writer、审批/采纳、供应商发送权与财务；新流程由 Python 编排并拥有草案，画布布局由 Go Authoring 独立保存。本文的旧运行恢复合同及未受影响的业务约束继续有效，历史通过记录不抵扣新目标。

- 状态：已接受设计
- 接受记录：`VP-D10`（2026-08-30）；产品主链、Target/Wire、Owner/恢复三轴隔离反例评审通过（最终正文评审 SHA-256 `e1d9448341fa5e5cf7bede4468f662140ad53c7c25eca5083eab3296feabc74f`）
- 历史事实：本文旧版曾于 2026-08-29 接受通用 Provider 配置、Secret、ProviderCall 和四类媒体 Adapter 的 Platform Complete 目标；这些事实保留，但其模型枚举、计费、`shot_frame`、`shot_video` 与多 Provider 广度不再作为当前视觉生产 MVP 的完成门
- 已接受前置：[完整设计基线](0001-AI短剧制作平台完整设计基线.md) · [系统总体架构](0003-系统总体架构.md) · [StoryGraph 内容图设计](0010-StoryGraph内容图与DAG创作画布设计.md) · [视觉生产工作台设计](0011-剧本视觉生产工作台与世界观预设设计.md) · [后端领域模块设计](2002-后端领域模块功能设计.md) · [Agent Harness 与 Skill 设计](3003-StoryGraph剧本解析Harness与内置Skill设计.md)
- 历史派生：[StoryGraph 产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md) · [需求规格](../requirement/0010-StoryGraph内容图与DAG创作画布需求规格.md) · [唯一实施计划](../plan/0010-StoryGraph内容图与DAG创作画布实施计划.md)；继续冻结，等待 `VP-D13`–`VP-D15`
- 下一设计门：[Workflow 公共 Human Gate 命令与恢复设计](2055-Workflow公共HumanGate命令与恢复设计.md)（`VP-D11`）

## 1. 结论

当前 MVP 的 Generation 服务不是“输入 Prompt，返回 URL”，也不是以价格、支付或 Provider 数量为中心的模型网关。它是 `ApprovedReferencePlanVersion` 的受控执行器，必须把剧本拆解结果转成六类可审核、可选择、可发布的视觉结果：

```text
ApprovedReferencePlanVersion + exact ReferencePlanTarget
  → compile_reference_brief Agent Candidate
  → Backend Provider-neutral GenerationTargetProduction
  → exact GenerationExecutionSnapshotContract
  → ProviderCall(s) + immutable staged media
  → CandidateBundle(s)
  → deterministic QC + vision_review_candidate_production
  → Human CandidateSelection
  → exact Owner Apply
      ├── base target        → AssetVersion + Artifact/Rendition
      └── composition target → selected Artifact + ReferenceBindingVersion
```

六类 Target 与唯一正式结果固定为：

| Target kind | 波次 | 唯一正式结果 |
|---|---|---|
| `character_identity_anchor` | A | `AssetVersion(purpose=character_identity_anchor)` |
| `location_board` | A | `AssetVersion(purpose=location_board)` |
| `prop_sheet` | A | `AssetVersion(purpose=prop_sheet)` |
| `character_appearance` | B | `AssetVersion(purpose=character_appearance)`，绑定同 Identity 的精确 anchor AssetVersion |
| `scene_composition` | C | selected Composition Artifact + `SceneReferenceBindingVersion` |
| `interaction_composition` | C | selected Composition Artifact + `InteractionReferenceBindingVersion` |

`ApprovedReferencePlanVersion` 不包含 Provider、模型、价格、Prompt、Candidate 或“当前资产”。`GenerationTargetProduction` 也先保持 Provider-neutral；只有 Target 通过业务、依赖、Brief、Style/Policy 和输出合同校验后，执行准备阶段才冻结具体 Provider Binding/Profile/Credential 与请求编译器版本。任何阶段都不得回到旧 `approved_storyboard_intents`、`reference_asset` 泛化 Target 或 `needs_asset` 反向触发路径。

当前 MVP 必须证明六类 Target 都可通过同一条真实图片执行路径完成候选、审查、选择和 Owner Apply。多 Provider 广度、动态计费、付费产品、Shot 图片/视频与自动模型路由继续是 Platform Complete 目标，不阻塞本闭环，也不得以空 Adapter 或占位媒体抵扣未来完成度。

## 2. 问题、范围与非目标

### 2.1 要解决的问题

旧 Generation 设计存在五个根本错位：

1. 参考生成从 Storyboard Intent 或宽泛 `reference_asset` 发起，无法证明它履约哪一个已批准 Reference Target；
2. 一张 composite reference sheet 同时承担人物身份、换装、地点和道具，无法表达独立版本、依赖与局部重做；
3. Prompt、Provider 参数与业务事实混在一起，切换模型可能静默改变已批准目标；
4. Candidate、临时媒体和正式 Artifact/AssetVersion 的 Owner 边界不清，Provider 成功容易被误报为资产发布成功；
5. 计费与 Provider 广度占据 MVP 主路径，但没有先证明“人物多形象—道具交互—场景组合—分镜消费”的产品价值。

### 2.2 本文范围

- 六类 `GenerationTargetProduction` 严格联合和输出槽位合同；
- `ApprovedReferencePlanVersion`、Reference Brief、Target、执行快照的逐层绑定；
- 身份锚点优先、Appearance 精确依赖、组合参考精确基础依赖；
- Provider-neutral Target 与 Provider-specific Execution Snapshot 的边界；
- ProviderCall、Staged Media、CandidateBundle、确定性 QC、Vision Review 和 Selection；
- base/composition 两类 Owner Apply、Gate 4 checkpoint 与失败关闭；
- 幂等、并发、重启、outcome unknown、stale 和局部重做；
- 当前 MVP 与 Platform Complete 的明确完成门。

### 2.3 非目标

本文不定义：

- 价格、支付、账单、动态汇率、Provider 抓价或面向用户的付费墙；
- Shot Frame、Shot Video、音频、口型、运动控制或成片渲染；
- 自动 Provider fallback、质量竞价、模型市场或任意 Base URL/Header 代理；
- 前端页面、HumanTask lease/恢复状态机、真实表名或 HTTP 路径；
- 为每个风格复制一套 Generation 代码或把 Preset Prompt 当作业务事实；
- 让 Agent、Provider、Workflow、StoryGraph 或浏览器直接写 AssetVersion/Reference Binding。

现有 Cost/Quota 代码可以作为历史基础设施保留，但当前 MVP 的 Reference Generation 不得因缺少 PriceQuote、付费账户或账单配置而阻断。若部署需要防滥用，只允许使用版本化的运维调用上限、字节上限和并发上限；它们是执行安全策略，不是产品支付模型，也不进入 Reference Plan。

## 3. Owner 与职责边界

| 模块 | 唯一拥有 | 明确不拥有 |
|---|---|---|
| `production/reference` | Approved Plan/Target、Scene/Interaction Reference Binding | Candidate、Provider、基础 AssetVersion |
| `preset` | EffectiveStyleSnapshot、EffectivePolicySnapshot、Project Preset Binding | Target 业务身份、Provider Secret |
| `agent` | Reference Brief Candidate、Vision Review Candidate 及其 Invocation/Attempt/Release fence | Provider Call、Selection、正式资产 |
| `generation` | GenerationTarget、Execution Snapshot、Provider Job/Call、Staged Media、CandidateBundle/Set、CandidateSelection | Artifact readiness、AssetVersion、Reference Binding |
| `asset` | Artifact/Rendition/Readiness/Rights/Lineage、AssetVersion、composition artifact membership | Reference Plan、Generation Candidate、HumanTask |
| `review` | HumanTask、lease、ReviewDecision | Candidate 内容和 Owner Apply |
| `workflow` | Run/NodeRun、等待、恢复、resume receipt | Target 语义、Provider 结果判断、正式内容 |

关键边界：

- Generation Candidate 引用 `GenerationStagedMediaObject`；它不是正式 Artifact。
- Human CandidateSelection 是“选择哪个 CandidateBundle”的不可变事实；它不等于发布。
- Owner Apply 才把选中 Staged Media 按 content digest 晋升为 `asset` 拥有的 Artifact/Rendition，并发布 AssetVersion 或 Reference Binding。
- Provider Adapter 只返回调用观察与媒体字节；不能创建 CandidateSelection、Artifact、AssetVersion 或 Binding。
- StoryGraph 只在 Owner Apply 后投影正式 Result；GenerationTarget、Provider 配置和未选 Candidate 不进入 `storygraph-production` 权威内容图。

## 4. 从 Reference Plan 到可执行 Target

### 4.1 唯一来源与激活条件

Target Builder 只接受当前 Project 唯一 active `ApprovedReferencePlanVersion` 内的精确 `ReferencePlanTargetVersionRef`。必须同时证明：

1. Project activation head 与 Plan scope head 指向同一 exact Plan；
2. Target 属于该 Plan，业务键、Target kind、coverage、依赖和 fulfillment 与 Plan hash 一致；
3. `fulfillment=not_generated` 时返回 `reference_target_not_executable`，不创建 Brief、Target、Execution、Job 或 Staged Media；
4. `required|optional` 均可生成，但 optional 未选择不阻断 Gate 4；
5. Target 已被同一 Plan 的正式结果履约时，普通 Start 返回 `reference_target_already_fulfilled`；显式重新生成必须有新的 `ReferenceGenerationAuthorizationContract(kind=regenerate_candidates)` 和递增 generation round。

不得用 Target kind、显示名、Scene ID 或相似资产在项目中搜索替代项；不存在 `current/latest/default asset` 读取。

### 4.2 Target Builder 两阶段输出

业务 Target 与执行配置分离：

```text
BuildReferenceGenerationTargetCommand
  → GenerationTargetProduction（Provider-neutral）

PrepareReferenceGenerationExecutionCommand
  → GenerationExecutionSnapshotContract（exact Provider-specific）
```

`GenerationTargetProduction`：

```text
GenerationTargetProduction
├── target_id / revision / content_hash
├── workspace_id / project_id
├── approved_reference_plan_version_ref
├── reference_plan_target_ref / target_business_key
├── target_kind / fulfillment
├── generation_round
├── generation_authorization_ref
├── reference_brief_candidate_revision_ref
├── effective_style_snapshot_ref
├── effective_policy_snapshot_ref
├── source_payload: ReferenceGenerationSourceProduction strict union
├── dependency_asset_version_refs[]
├── dependency_root_hash
├── output_contract: ReferenceOutputContractProduction
├── target_read_set_root
└── created_by / created_at
```

Target content hash 排除 `created_by/created_at` 审计字段，但覆盖其余完整字段。Target Builder 先在同一个可重复读快照内重算 `TargetReadSetProduction`，再以 expected Head CAS 发布 Target；相同命令与相同 canonical input 返回同一 Target。任何 Plan、Brief、Style、Policy、source 或 dependency 漂移都必须创建新 Target，不原位改写。

`GenerationTargetHeadProduction` 以 `(workspace, project, approved plan ref, reference plan target ref)` 为键，只用 expected revision CAS 指向最高已授权 generation round；`GenerationExecutionHeadContract` 以 Target ref 为键；`GenerationCandidateSelectionHeadContract` 以 Target ref 为键。三个 Head 都只是并发索引，不复制 Target、Execution 或 Selection 内容，也不提供“查最新资产”能力。

首次基础 Target 发布复用 `gen_targets`，存储类别为语义化 `reference_plan`，Payload 为完整 `GenerationTargetProduction`；Source/Policy 列分别保留 exact Reference Plan Target 与 Effective Policy refs，不转换成旧 `approved_storyboard_intents`。新增 Generation Reference Target Head 只保存 scope、精确 Plan/Target ID、当前 Generation Target ID/Hash 和轮次，首次 expected revision 必须为 0。授权、accepted Brief、当前来源和 Preset capability 在同一 Backend 事务内重验，再编译输出合同、dependency/read-set roots，原子写 Target、Head 与 Command Receipt。重复命令先重验事实，再核对原 Target 和 Head；新 key 不得绕过首次 Head CAS 创建第二轮。此入口不准备 Provider Execution，不创建媒体或假 AssetVersion；重新生成按独立授权后续实施。

生成授权与执行授权分离：

```text
ReferenceGenerationAuthorizationContract
├── kind = initial_generation | regenerate_candidates
├── approved_reference_plan_version_ref / reference_plan_target_ref
├── requested_candidate_bundle_count
├── base_generation_target_ref? / base_candidate_set_ref?
├── reason_code
├── human_action_ref / membership_token_version
└── content_hash / authorized_by / authorized_at

ReferenceExecutionAuthorizationContract
├── kind = initial_execution | retry_before_dispatch | switch_provider
├── generation_target_ref
├── previous_execution_ref?
├── selected_project_provider_binding_version_ref
├── reason_code / unresolved_call_acknowledgements[]
├── human_action_ref / membership_token_version
└── content_hash / authorized_by / authorized_at
```

`generation_round=1` 必须绑定 `initial_generation` 用户动作、Gate 3 已批准 Target 与有效 Brief；round 大于 1 必须绑定 `regenerate_candidates`，并覆盖 reason、base Target/CandidateSet refs 和授权 scope。Temporal Activity 重投、Provider retry、Provider 切换或媒体下载重试都不能递增 generation round；它们只可能在同一 Target 下创建新的、显式授权的 Execution Snapshot。

首次生成授权先由 Backend `generation` 应用服务写入既有 Command Receipt（operation=`generation.reference.authorize_initial`），不增加授权专用表；`human_action_ref` 为该不可变回执 UUID，Result 保存完整授权合同。合同绑定同 scope 的 exact Plan/Target OwnerRef、1–4 个候选 Bundle、操作者与会员 Token Version，`kind/reason_code` 均为 `initial_generation`，两个 base refs 均为空。内容 Hash 使用 Production Canonical JSON，并将自身 Hash 置空；时间统一为 UTC 微秒，便于持久化后逐字段重验。

应用服务在同一事务中锁定并重验 Workspace/Membership/用户 Token/Project，消费 Agent Owner 的 accepted Brief exact read，比较命令中的 Plan/Target/Brief revision/hash 后写回执。重复 key 必须重验当前权限和事实；相同输入返回同一回执，输入漂移拒绝。授权只记录用户生成意图，不证明已履约、不递增 generation round、不启动 Workflow/Provider、不消耗成本额度；后续 Target Builder 必须在发布事务内再次验证该回执与 expected Head。当前写入口只支持首次生成，重新生成必须等正式 base Target/CandidateSet 事实具备后按上面的独立授权分支实施，不以首次授权兼容重试或重新生成，也不提前增加独立前端入口。

### 4.3 Reference Brief fence

每个可执行 Target 必须绑定恰 1 个 `reference_brief_candidate_production` 成功 Candidate Revision：

- Candidate target ref、kind、business key、source refs、dependency refs、Style/Policy refs 与正在构建的 Target 逐字节相等；
- Candidate 所在 Stage Release 未 quarantine/revoke，Invocation Outcome、Attempt result、Candidate Head 和 review/repair chain 均有效；
- Candidate 未被上游变更标 stale；
- Brief 包含 source-vs-design slots、positive/negative instructions、rights/provenance requirements、输出视图角色和 QC rubric refs；
- Brief 不是自由 Prompt。Provider Request Compiler 只能把这个结构化 Brief 映射为对应模型请求，不能补人物、换道具、换状态或改写世界事实。

无 Brief、旧 Brief、Target 不匹配或自由文本 Prompt 均在 Provider Binding、远程调用和媒体创建前失败关闭。

消费 Brief 时，Backend `agent` Owner 提供 exact Candidate Revision ID + Revision Hash 的只读入口，在调用方事务内重验当前 Input facts，再读取精确来源 Invocation、Result、Attempt、Dispatch Authorization、Release Control 与 Candidate Head。禁止按最新 Attempt 替代 Candidate 的来源 Attempt；Result/Candidate 内容 Hash 与 Revision Hash 均重新计算，Result 必须为 accepted、Attempt 必须 completed，当前 Control 必须仍 approved 且完整 fence 一致。Coverage Query 与后续 Target Builder 复用此入口；普通 Query 的 accepted 展示也不能绕过这些验证。此入口不创造审批或 repair 事实，当前 Reference Brief 只有原始 accepted Revision，未实现的 repair 不能通过伪造 Revision 序号获得资格。

## 5. 六类 Target strict union

`ReferenceGenerationSourceProduction` 只允许以下六个分支；所有 OwnerRef 都包含 `owner_kind/version_family/owner_logical_id/revision/content_hash/fragment_key?`，数组按 canonical key 排序去重，`additionalProperties=false`。

基础来源编译先覆盖无 AssetVersion 依赖的人物身份锚点、地点板和道具表。Backend 在首次生成授权事务中复用当前 Production World 的 Owner 重验查询，要求其 Owner Set Hash 与已批准 Plan 一致，再从机械 StoryGraph 投影解析 exact Identity/Specification/State、唯一 Production Binding，以及指定 Scene 范围内该身份状态的完整 Occurrence 集合。投影只用于关系解析，不成为第二事实源；缺失、重复、跨 scope、Hash 或关系漂移均在写授权回执前失败。来源输出保留 Brief 的类型专属约束和独立视图角色，使用 canonical JSON 内容 Hash；回执输入身份同时绑定来源 Hash 与 World Owner Set Hash，幂等重放也重新验证。依赖型来源不得以空 AssetVersion 或旧 Storyboard 输入替代；正式 Target 发布仍须在自身写事务内重新读取和编译。

### 5.1 Character Identity Anchor

```text
CharacterIdentityAnchorSourceProduction
├── identity_ref
├── character_specification_ref
├── identity_anchor_asset_state_ref
├── production_binding_ref
├── occurrence_refs[]
├── identity_invariant_slots[]
└── required_view_roles = [front, profile, back]
```

- 一个 active Plan 内每个实际出现的 Character Identity 恰 1 个 anchor Target；
- anchor State 是 Gate 3 审核选定的视觉基础，不因换装、伤势或伪装 State 重复建立身份；
- 身份不变量至少覆盖脸部结构、体型/比例、永久标记和不可由 Appearance 改写的核心特征；
- 输出为一个 CandidateBundle 的三个独立必需视图槽位，不要求把三视图拼成一张 composite sheet。

### 5.2 Character Appearance

```text
CharacterAppearanceSourceProduction
├── identity_ref
├── character_specification_ref
├── appearance_asset_state_ref
├── production_binding_ref
├── occurrence_refs[]
├── identity_anchor_target_ref
├── identity_anchor_asset_version_ref
├── invariant_slots[]
├── variable_slots[]
└── required_view_roles = [front, profile, back]
```

- 必须依赖同 Identity、同 Specification、同 EffectiveStyle 的已选 exact anchor AssetVersion；
- anchor AssetVersion 必须履约 Source 中的 `identity_anchor_target_ref`，不能按“同角色最新资产”替代；
- variable slots 可改变服装、发型、妆容、伤势、污损、伪装和随剧情变化的状态；
- 任一输出改变 identity invariant 时 deterministic/vision QC 失败，不得把新脸发布为 Appearance。

### 5.3 Location Board

```text
LocationBoardSourceProduction
├── location_identity_ref
├── location_specification_ref
├── location_asset_state_ref
├── production_binding_ref
├── occurrence_refs[]
├── topology_constraints[]
├── scale_anchors[]
├── material_slots[]
├── occupancy_policy = empty
└── required_view_roles = [empty_establishing, spatial_orientation, material_scale_detail]
```

Location Board 默认空场，不得擅自放入人物或剧情道具。三个视图共同证明空间方向、入口/出口/关键区域、尺度和材料语言；不能只交付一张氛围图。

### 5.4 Prop Sheet

```text
PropSheetSourceProduction
├── prop_identity_ref
├── prop_specification_ref
├── prop_asset_state_ref
├── production_binding_ref
├── occurrence_refs[]
├── physical_dimensions
├── structural_slots[]
├── state_slots[]
├── content_or_mechanism_slots[]
├── occupancy_policy = no_hands_no_people
└── required_view_roles = [front, side, back, state_detail]
```

Prop Sheet 不出现手、人物或场景；持握、传递、佩戴与使用姿势由 Interaction Composition 表达。打开/关闭、完好/损坏、是否有内容物必须匹配精确 Prop State，不能把多个剧情状态混在一个含糊 Sheet 中。

### 5.5 Scene Composition

```text
SceneCompositionSourceProduction
├── scene_ref
├── occurrence_refs[]
├── interaction_claim_refs[]
├── continuity_claim_refs[]
├── scene_production_closure[]
├── selected_base_asset_version_refs[]
├── composition_purpose
├── spatial_constraints[]
└── required_view_roles = [composition_master]
```

- `selected_base_asset_version_refs[]` 的 Identity/Specification/State 三元组去重集必须与 Scene production closure 精确相等；
- 每个版本必须履约该 Composition Target 的 `depends_on_target_refs[]` 中对应基础 Target；
- Composition 只能组织已选人物形象、地点和道具，不能替换基础资产、增加未出现身份或修正上游连续性；
- 结果是一张可供分镜理解的场景构图参考，不是 Shot、镜头列表或最终画面。

### 5.6 Interaction Composition

```text
InteractionCompositionSourceProduction
├── scene_ref
├── interaction_claim_ref
├── actor_occurrence_ref
├── prop_occurrence_ref
├── counterparty_occurrence_ref?
├── selected_base_asset_version_refs[]
├── hand_side / grip_or_contact_point / orientation
├── body_prop_scale_constraints[]
├── transfer_or_use_state
└── required_view_roles = [interaction_master]
```

- 参与 Occurrence、Identity/State 与 selected base AssetVersions 必须逐项闭合；
- `interaction_master` 必须同时清晰表达全身/主体关系与接触区域，不能只生成手部特写而失去人物形象，也不能只生成站姿而看不清握点；
- actor、counterparty、holder、手别、方向、接触点、比例与使用状态任何一项错误均为 `interaction_contact_mismatch`；
- Interaction Composition 不创建新的 AssetState，不把“拿着道具的人物”永久固化为 Character Appearance。

### 5.7 输出合同

```text
ReferenceOutputContractProduction
├── contract_id = reference-output-production
├── modality = image
├── candidate_bundle_count
├── slots[]
│   ├── slot_key / view_role / required=true
│   ├── allowed_media_types[]
│   ├── aspect_ratio / min_width / min_height / max_bytes
│   └── semantic_requirements[] / qc_rubric_refs[]
├── bundle_completeness = all_required_slots
└── content_hash
```

`candidate_bundle_count` 是用户请求并经 Policy 限制的候选方向数，不是 Provider 自由返回的图片数。每个 Bundle 必须拥有全部 required slot；例如三视图生成 4 个候选方向时，逻辑上是 4 个 Bundle × 3 个槽位。MVP 可以按执行策略串行或受限并行生成槽位，但 CandidateSet 不能把不同人物方向的 front/profile/back 混拼为一个 Bundle。

输出合同先由 `generation` 领域构造器闭合，再由应用层从通过 `ValidateFor(input)` 的 Reference Brief 编译。每个 role 使用相同名称作为 `slot_key`，按 role 字典序固定顺序；输入输出均要求完整且唯一的角色集合，不能用 composite sheet 代替槽位。调用方提供逐槽位媒体/比例/尺寸/字节 Policy，必须恰好覆盖 Brief 的角色集合；构造器复制并规范化数组，拒绝缺失、重复、未知角色与未知 JSON 字段。正向、layout、scale 要求按集合排序去重进入每个 slot，QC 引用按 contract ID 排序且不可丢失；完整 Brief/source/negative 仍由后续 Target 和请求编译器冻结。

首个输出合同的运行上限固定为每 Target 1–4 个 Bundle、每 Bundle 最多 4 个必需槽位、单槽位最多 10 MiB、同 Bundle 最大字节预算合计 32 MiB，以符合已接受的 Vision 媒体预算。只接受 PNG/JPEG、正整数最小尺寸（不超过 8192）与约分后的正整数比例（两边不超过 100）；具体尺寸来自显式 Policy，不推测 Provider 能力。Content Hash 使用现有 Production Canonical JSON，覆盖完整合同且把自身 `content_hash` 置空；解码重验全部字段和 Hash。此编译不证明 Provider 配置、Target Head、Candidate 发布资格或 READY 媒体，后续 Target Builder/Execution 仍承担正式事实与能力验证。

## 6. 依赖 DAG 与执行波次

Backend 从 Plan Target dependency refs 和已发布结果机械生成执行 DAG：

```text
Wave A: character_identity_anchor | location_board | prop_sheet
   ↓ exact selected AssetVersion
Wave B: character_appearance
   ↓ all required selected base AssetVersions
Base Selection Checkpoint
   ↓
Wave C: scene_composition | interaction_composition
   ↓ exact selected Composition Artifact + Binding
Per-Scene Composition Checkpoint
```

规则：

1. Wave A 可按 Target 并行，但同一 Target/round/slot 的远程发送权仍唯一；
2. Appearance 在 anchor AssetVersion 发布前不得编译 Provider Request，anchor CandidateSelection 但未 Apply 也不够；
3. Base checkpoint 只在全部 active required base Target 有正式 AssetVersion 时完成；
4. Composition Target 必须冻结完整 exact base version set；缺一个、多个、错误 State 或旧 Plan 结果都失败；
5. optional Target 可以不生成或不选择；一旦某 required Composition 声明依赖它，该依赖的 fulfillment rank 必须满足 Plan 规则；
6. 一个 Target 失败只阻塞其依赖闭包，不使无依赖的其他 Target 回滚；
7. Gate 4 完成由 D09 的 checkpoint Command/Collection Receipt 证明，不由 Workflow Run 百分比或 Provider Job 状态推断。

### 6.1 风格预设与 Generation 正交

世界观/风格切换不复制 Target Schema、Generation Workflow 或 Provider Adapter。所有 Preset 都通过相同六类 Target 工作：

```text
PresetVersion + typed overrides
  → EffectiveStyleSnapshot + EffectivePolicySnapshot
  → purpose profile + structured Reference Brief
  → same GenerationTargetProduction strict union
  → Provider-specific Request Compiler
```

- Target Builder 重算 Preset capability manifest；目标 kind、required view role、输入图或 QC rubric 不受支持时返回 `preset_capability_missing`，不使用通用 Prompt fallback；
- `faithful|world_adaptation`、世界设计基底、材质/服饰/建筑/道具语言、画面媒介和 negative constraints 全部由 exact Effective Snapshots 与 Brief 冻结；
- Preset 不能改变 Identity、Scene、AssetState、Interaction holder 或 Reference Plan coverage，只能填充已授权 design gap 和视觉表达；
- 相同 Provider/Profile 可以执行多个 Preset，但每个 Execution 都绑定 Target 中的 exact snapshot refs；Adapter 不维护“当前风格”；
- 切换 Preset 发布新 Snapshot/Plan/Brief/Target，并按第 11 节 stale 闭包重做视觉结果；旧 P0 剧本事实不变化。

## 7. Provider-neutral Target 与执行快照

### 7.1 执行准备

`PrepareReferenceGenerationExecutionCommand` 在 Target 发布后解析精确 Project Provider Binding，但不改变 Target：

```text
GenerationExecutionSnapshotContract
├── execution_id / revision / content_hash
├── generation_target_ref
├── project_provider_binding_version_ref
├── provider_connection_version_ref
├── provider_credential_version_ref
├── provider_model_profile_version_ref
├── provider_adapter_contract_ref
├── request_compiler_contract_ref
├── provider_capability_snapshot_hash
├── compiled_request_manifest_hash
├── operational_limit_policy_ref
├── execution_read_set_root
├── reference_execution_authorization_ref
└── created_at
```

Secret 明文、Prompt、Provider URL 和响应内容不进入 Snapshot。Credential ref 只用于 Backend 在一次调用前解析短生命周期 Secret；Query 只返回版本/fingerprint，不回显密文或明文。

执行方读取已发布基础 Target 时，使用 Backend Generation 的 exact ID/revision/hash 入口，不以 Payload 解码成功作为执行资格。入口在调用方事务内重验当前操作者写权限、原生成授权人的 Token/权限、唯一发布回执、accepted Brief、当前 World/来源、Preset capability 和 Target Head，按冻结输出策略重编译并比较完整 Target 与回执输入 Hash。原授权人与后续执行操作者可以不同，但不能用后者身份改写前者的授权。读取不创建 Target、回执或执行授权；历史 Target 仍保留，失效时仅拒绝作为当前执行输入。

首次执行授权复用 Command Receipt，operation 为 `generation.reference.authorize_execution_initial`，不新增授权表。合同明确 Workspace/Project、exact Generation Target 与用户选定的 Project Provider Binding ID/revision/hash；`kind/reason_code=initial_execution`、previous Execution 为空、unresolved acknowledgements 为空数组，绑定操作者、Token Version、动作回执 ID 与 UTC 微秒时间，并对完整合同重算 Hash。事务先沿既有 Provider 配置写入锁序锁 Workspace，再消费 Target exact read；精确 Binding 必须属于当前项目且用途为 Reference 图片，关联 Connection/Profile 已启用、内容身份与关系一致，Binding/Connection/Profile/Credential 均仍指向指定版本。不以查询到的新版本替换用户选择。

重复授权 key 仍重验上述事实，回执输入 Hash 绑定 Target read-set 和关联 Provider 版本内容身份；漂移拒绝、同输入返回原回执，失败不留下新授权。缺少选择或配置返回 `provider_configuration_required`，不影响解析与非视觉节点。授权只记录执行意图，不解密 Secret、不调用 Adapter、不预留成本、不创建 Execution/Call；槽位与模型能力、Request Compiler、运行限额及 expected Execution Head 的完整校验仍必须由执行准备在同一事务内消费授权并完成。首次授权不能在执行准备时冒充 retry 或 switch_provider。

执行准备必须证明：

- Binding 属于同 Workspace/Project，已启用且 modality/capability 覆盖 Target 全部 slot；
- Model Profile 能逐字节实现 Target 的尺寸、比例、输入图和 slot 数，不得静默裁切、取整、降质量或换模型；
- Request Compiler contract 能将当前 Brief schema 和 Target kind 映射到该 Adapter；
- 所有依赖 Artifact 都 READY、rights/lineage 可用于当前 Provider；
- 同一 Target 通过 `GenerationExecutionHeadContract` 的 expected revision CAS 最多指向一个 active Execution Snapshot。更换 Binding/Profile 必须有新的 `ReferenceExecutionAuthorizationContract`；如果旧 Call 已越发送边界，还必须逐项确认 unresolved/remote invocation 风险。Provider 切换不改变业务 Target hash，但旧 Execution 的 CandidateSet 立即失去 active execution fence；若目标是再生成一批新 Candidate 而不是恢复未完成执行，则必须新 generation round。

无 Provider 配置时返回 `provider_configuration_required`，但 Backend、剧本解析、Gate 1–3、查询和非视觉 Workflow 正常。当前 MVP 只要求至少一条真实、可恢复、可审核的图片执行路径覆盖六类 Target；此前接受的 Seedream、GPT Image、Nano Banana 与 Seedance 广度仍是 Platform Complete，不能用本步声明为已完成。

首次执行准备只接受 `expected_execution_head_revision=0`。Backend 复用现有 exact Target、accepted Brief、Provider 配置与 Command Receipt 入口，在 Serializable 事务中先锁 Workspace，再重验当前操作者、原执行授权人的权限和原授权回执输入身份。只解析已注册且声明正式 Reference 编译合同的 Factory，不复用旧 GenerationRequest/PriceQuote。编译所有 Bundle/slot 并校验完整清单，冻结 Target read-set、精确 Binding/Connection/Profile、Credential ID/revision/fingerprint、Registry release、Adapter/Compiler/Capability 身份、清单 Hash 与运行限额。默认运行限额为最多 4 Bundle、每 Bundle 4 slot、输入 32 MiB、每输出 10 MiB、并发 2、提交超时 180 秒、每日运维调用 256 次；准备仅冻结限额，实际发送阶段另行原子执行并发/每日额度检查，不把准备当作额度预占。

`gen_reference_executions` 存储严格不可变快照，`gen_reference_execution_heads` 仅以 scoped exact Target 为键保存当前 Execution ID/Hash 和 revision；二者由现有 GORM Catalog 同步，不增加迁移入口。快照、首个 Head 与准备回执必须一次提交，新 key 不能绕过首次 Head；回执失败不能留下孤立快照。相同 key 先重新验证事实，再比较输入 Hash、重编译快照和 Head，不返回失效的准备结果。此入口不用于恢复已越发送边界的历史执行：恢复需要后续专用流程消费冻结 Provider 与 Call 状态，不得借准备重放切换 Provider 或二次 Submit。

首次快照的具体 JSON 将上述冻结输入统一放入 `read_set`，`execution_read_set_root` 对该对象计算 canonical Hash；外层保存 contract ID、execution ID/revision、scope、操作者和 UTC 微秒时间，`content_hash` 覆盖整个快照并将自身置空。`Credential` 使用明确的 fingerprint 字段，不将密文冒充内容 Hash。编译端口及返回类型由 application 定义，OpenAI Factory 直接实现；Registry release Hash 覆盖排序后的注册项与 Reference 编译 descriptor，不建立第二套注册中心。

### 7.2 Adapter 合同

Provider Registry 是进程内只读 Factory 表；数据库只保存不可变连接/Profile/Binding 版本。Adapter 统一的最小端口为：

```text
Compile(execution snapshot, target, brief, slot) → CanonicalProviderRequest
Submit(call identity, canonical request, short-lived credential) → SubmitObservation
Query(remote job identity, short-lived credential) → QueryObservation   // 仅官方支持时
Fetch(output identity) → bounded media stream
Normalize(observation) → typed receipt / staged media metadata
```

Adapter 不能：

- 查询“默认模型”、在多个模型间 fallback 或运行时试探 Transport；
- 修改 Target/Brief、增加候选、隐藏多次远程调用或自动重试可能已送达的 Submit；
- 把 Provider URL 当作长期 Artifact，把 Base64/Prompt/Secret 写入日志或 Receipt；
- 直接访问 Owner Repository、Human Gate 或 StoryGraph Compiler。

模型 ID、参数和能力会变化；具体 Adapter 开发开始时必须重新核验官方一手文档并冻结进 ModelProfile/contract test。本文不把 2026-08-29 的展示名和外部 Model ID 重述成永久事实。

首个基础图片请求编译器固定为 `openai-reference-image-compiler`，消费通过 exact read 的 Target、accepted Brief 与已验证 Profile，不读取 Owner、Secret 或网络。仅支持无图片依赖的人物身份锚点、地点板和道具表；依赖型 Target 不得丢弃参考图后降级为文生图。每个 Bundle/slot 编译一个 `n=1` 的独立请求，显式 PNG、high quality、非流式；尺寸直接使用冻结 slot 的宽高，严格匹配比例，不取整、不裁切、不使用 auto。当前 Profile 必须为启用的 `gpt-image-2` / `openai-image-api-nonstreaming` 且无未支持的 Defaults，不能忽略覆盖参数。

2026-09-12 核验 [OpenAI 官方 GPT Image 2 参数](https://developers.openai.com/api/docs/guides/image-generation#earlier-gpt-image-models)：边长不超过 3840、均为 16 的倍数、长短边比例不超过 3、总像素在 655360–8294400。编译器逐槽位检查这些限制与 PNG Policy；32,000 UTF-8 bytes 为本地 Prompt 安全预算，不声明为供应商上限。Prompt 机械保留 source/design slots、正负约束、layout/scale、rights/provenance 与类型专属 Brief，并明确只生成当前 view role，不把角色列表当作拼版指令。编译结果包含瞬时 canonical 请求字节和逐 Bundle/slot 的请求 Hash；无 Prompt 的 manifest 身份绑定 exact Target/Brief/Profile 和 compiler contract。准备事务仍须重验授权、当前配置/Head、运行限额并持久化 Execution；本编译器不产生发送许可，也不证明生成图片满足语义 QC。

### 7.3 调用身份与状态机

基础图片传输直接消费已编译的 Reference Call、canonical 请求和精确输出 slot，不转换成旧 Intent/GenerationRequest/PriceQuote。发送前重验 Call key、请求 Hash、Bundle/slot、模型参数与本地媒体限额，凭据仅借用当前 invocation 的明文字节。HTTP 只执行一次 POST，不跟随重定向、不设置可重试请求体或把 submission token 当作供应商幂等支持；deadline 覆盖请求、响应读取和私有对象写入。同步接口不提供伪 Query。

传输观察明确区分 `staged`、`output_rejected` 与 `outcome_unknown`，只携带 Call/token、受控原因码和通过校验的媒体元数据；发送后的网络错误、非成功状态或 staging 失败不冒充明确远程失败，也不触发重发。响应严格限制总字节，拒绝重复 JSON key、非唯一输出、URL 替代 Base64、非法 PNG、尾随内容、尺寸或字节超限；不把被拒绝输出提升为 Candidate。通过完整解码的 PNG 才复用既有私有对象存储，key 绑定 Workspace/Project/Execution/Call/token，digest 绑定原始字节。此 Adapter 观察不是持久化 Receipt 或正式 AssetVersion。

后续应用服务必须在同一 Backend invocation 内完成 preflight、领取并提交唯一发送权、调用 Submit、持久化观察；不能将 `should_dispatch` 写进 Temporal 历史，再交给可重投的独立 Submit Activity。Adapter 不自行查数据库或授予发送权，未完成该应用编排和终态 Receipt 前不开放业务执行入口。传输合同测试使用本地 TLS 接口和对象写入断言，不等于真实付费模型或最终浏览器验收。

首次同步执行由 `ReferenceCallExecutionService` 消费上述链路。预检读取 exact Snapshot/Target/完整清单和冻结 Provider，事务结束后解密 invocation 凭据；预检通过后再次完整重验并提交发送权，只有本调用领取成功才执行 Submit。不是 PENDING 的重放只返回已存状态，不解密、不读最新 Provider、不再提交。发送前失败保持 PENDING；领取后 Adapter 明确未尝试 HTTP 的错误记为 `not_sent`，发送后的不确定性记为 `outcome_unknown`，不得自动重新领取。

执行服务使用要求真实 COMMIT 的事务端口，GORM 实现拒绝外层 SQL Transaction/PreparedStmt Transaction，不能将 savepoint 的 release 当作已持久发送权。状态机故障矩阵可以使用受控测试事务替身，但真实发送边界证据必须来自独立已提交的业务事实：本地 TLS 接收端通过另一数据库连接读到 DISPATCHING，执行返回后另一连接读到同一回执，重复执行没有第二次 HTTP。该测试只使用保留测试域的独立 owner，并按精确 scope 清理。

同步 Submit Receipt 内嵌于既有 Call 状态 JSON，与状态和 Hash 同一次 CAS 发布，不新增专用表或第二事实源。回执有独立内容 Hash，绑定 scope、完整 Call、token、冻结输出 slot、观察时间、受控 disposition/reason 和已验证输出；外部引用使用 Call key + Receipt hash。回执只能从空追加一次，同值重放不变、不同值失败关闭。此处 `outcome_unknown` 的 Submit 回执不是远程终态，仍占未解决上限；未来人工对账不得覆写这份原始发送观察。成功或明确输出拒绝/未发送更新为 SUCCEEDED/FAILED，并保留原因分类；每日调用数仍计已领取的发送边界。到期标记与及时观察落库竞争时，同 token 的有效回执可接在 OUTCOME_UNKNOWN 之后，不产生新 Submit。写入失败则保留原 DISPATCHING/OUTCOME_UNKNOWN，重复执行只观察状态，由恢复流程处理，不能因回执缺失重发。

Temporal 通过系统节点 `generation.reference_image_call`（executor 为 `activity.reference_image_call`，cache 为 `never`）消费已存在的执行。节点配置仅含 exact `execution_ref` 与 `call_key`，进入既有不可变 Node Input；不接受 token、Provider 参数或新的生成授权。Worker 装配完整 `Execute` 和只读/到期恢复服务，继续使用既有 ExecuteNode Activity、心跳与 durable polling，不新增 Workflow 类型、服务入口或消息队列。DISPATCHING 重投调用 Expire：未到冻结 deadline 返回 RETRYING，到期返回 NEEDS_ATTENTION；OUTCOME_UNKNOWN 要求人工对账，FAILED 失败关闭，只有带有效 staged 回执的 SUCCEEDED 输出 `reference_call_receipt`。Workflow 通用引用要求 UUID，因此该输出以数据库唯一 submission token 定位、固定回执 revision `1` 和 Receipt hash 标识，回执内容仍绑定完整 Call key，不把 token 当发送许可。执行错误不能携带 Provider 原文进入历史。

该节点是已经授权并准备的 Call 的内部执行能力，不自动授权或重建 Target/Execution。基础波次的完整准备、全部 required slot、媒体 Owner/Bundle/QC 继续按后续链路装配；单节点 SUCCEEDED 仅表示该 Call 的受控传输和回执成功，不表示业务素材已经发布。

已准备 Execution 的整组收集复用 Authoring→Compiler→Start：以 exact `reference_execution` 为冻结输入，将 Job 的完整 canonical Call 集合编译成串行 `generation.reference_call_observation` 节点，最后接 `generation.reference_execution_observation` 汇总。节点通过前一个不可变 Receipt 形成真实 DAG 边，不依赖名称排序模拟依赖；每个 Call 仍有独立 Activity/投影/重试。观察节点的成功含义是“明确结果已持久化”，可以是成功媒体或明确失败，不能作为媒体合格证明；成功媒体继续校验并保留 ready/rejected 事实。未知结果停止后续调用并进入对账，暂时读取或数据库错误保留原 Call 由现有 Activity 重试，不盲目发送。汇总必须重读完整 Job、全部 Call 和输入身份，只接受完整明确结果集合，输出稳定的执行观察引用；部分失败与全部失败仍由 Generation 真实进度表示，不变成媒体成功。

启动收集只消费既有授权/Target/Execution，不创建新生成轮次、配置 Provider 或补齐缺失 Call。Authoring 的冻结输入校验核对持久 Execution 的精确内容身份和项目范围，不使用任意剧本版本占位。启动期间各 Owner 命令使用稳定幂等键，重入复用已创建 Draft/Revision/Run；新的调用方不能借重复启动获得第二次发送权。此步骤不新增 Workflow 类型、消息调度、业务状态表或通用展开框架；它完成一个已准备 Target 的全部 required Call，不替代基础波次准备、六类目标或 Bundle/QC/Selection。

公开启动为 `POST /api/projects/{project_id}/reference-executions/{execution_id}/workflow-runs`，请求仅含 `execution_hash`、`idempotency_key`，返回 202 与 `workflow_run_id/status`，禁止缓存。路径、当前身份、严格 JSON 和各 Owner 的权限检查均生效；完整传输结果继续从既有 Execution 进度查询读取。该入口不提供 Provider 参数、凭据或新一轮生成许可。

一个 CandidateBundle 的每个 required slot 对应一个确定性 `ProviderCallKey`：

```text
ProviderCallKeyProduction
= Canonical JSON([
    execution_ref,
    candidate_bundle_index,
    slot_key,
    compiled_request_hash
])
```

Canonical Provider Request 对每个 Call 固定 `requested_output_count=1`。Provider 意外返回多个输出属于 `provider_output_cardinality_mismatch`：额外字节进入隔离区且任何一个都不能成为该 slot 的 Candidate；Adapter 不自行挑选“第一张”。

```text
ProviderCall
  PENDING
    ├── local preflight failure → FAILED_LOCAL
    └── CAS send right          → DISPATCHING
                                  ├── async remote id → SUBMITTED → RUNNING
                                  ├── success         → SUCCEEDED
                                  ├── explicit fail   → FAILED_REMOTE
                                  └── lost outcome    → OUTCOME_UNKNOWN
```

- 只有首次成功提交 `PENDING → DISPATCHING` 事务的调用路径获得 `should_dispatch=true`；
- `DISPATCHING` 不能因超时、重启或 Activity attempt 号而退回 PENDING；
- 官方返回可查询 remote id 后只能 Query 同一身份，不能 Submit 第二次；
- 同步调用越过发送边界却未落终态时进入 `OUTCOME_UNKNOWN`，必须人工对账或显式新 round；
- 一个 Call 最多一个终态 Receipt 和一个成功 Staged Media identity；重复同值观察幂等，不同值失败关闭；
- Workflow/Temporal 负责定时与恢复，HTTP Client 内不隐藏业务重试循环，Kafka 不调度 Provider。

`GenerationProviderJobProduction` 是一个 Execution Snapshot 的本地聚合，只保存 expected `ProviderCallKeyProduction` 完整集及其 root。全部 Call 成功才是 `SUCCEEDED`；至少一个成功且其余全部明确失败是 `PARTIAL_SUCCEEDED`；全部明确失败是 `FAILED`；任一 Call 尚未解决或 `OUTCOME_UNKNOWN` 时 Job 不得假终态。Job 聚合不拥有远程发送权，也不能用“整体重试”创建第二套 Call。

执行进度是上述事实的只读投影，不新增 Job 状态表或调度入口。`GET /api/projects/{project_id}/reference-executions/{execution_id}` 在同一数据库快照中核验当前读取权限、精确 Execution、Job 全集与全部 Call 状态，按 bundle index/slot key 输出有序身份、状态计数和内容 Hash。全部未发送为 PENDING；存在未知结果优先为 OUTCOME_UNKNOWN；仍有 pending/dispatching 为 RUNNING；只有完整集合均明确结束才能成为上述三个终态。缺项、重复、额外项、跨执行、回执或状态 Hash 漂移均失败关闭，不忽略损坏记录。读取历史执行不依赖最新 Provider、业务 Head 或重新编译，不触发发送、超时转移、媒体读取及正式资产发布。接口禁止 body/query selector，响应 no-store，且不暴露 token、Prompt、Provider 凭据或私有对象路径；SUCCEEDED 只说明全部传输成功，不表示媒体 QC、完整 Bundle、Vision 或 Selection 通过。

首次准备在同一事务发布 Execution/Head、不可变 Job 调用清单、全部 PENDING Call 和准备回执。Call key 是上述 canonical tuple 的 SHA-256；数据库额外唯一约束 `(execution_id, bundle_index, slot_key)`，请求 Hash 变化不能为同一槽位另造一次调用。Job 的 expected keys 按 bundle index、slot key 排序，root 对完整有序 keys 数组计算；Job 引用 Execution，Execution 不反向引用 Job，避免 Hash 环。`gen_reference_provider_jobs` 与 `gen_reference_provider_calls` 由既有 GORM Catalog 管理，不复用绑定旧 Intent/PriceQuote 的调用模型。

准备重放必须重编译完整 manifest，逐项重建 Job/Call 身份并核对数据库清单、scope 和元数据；缺项、多项、请求/槽位/Job root 损坏均拒绝，不补建、不重置已有调用。任何 Call 或回执写入失败回滚整个准备事务。此步骤只建立待发送事实，不授予发送权、不扣减并发/每日额度，也不代替发送后恢复和媒体验收。

当前 MVP 不建立新的动态 PriceQuote/付费 Reservation 完成门。运行保护只冻结 `OperationalGenerationLimitPolicy`：单 Target 最大 Bundle、单 Bundle 最大 slot、最大输入/输出字节、并发、超时和每日运维调用上限。它不表达货币、用户余额或套餐，且策略缺失时使用代码内安全默认值，不阻断已授权 MVP 旅程。

首次发送权由独立 Backend Command 获取，不复用准备回执返回许可。事务先锁 Workspace，重验当前操作者、当前 Target/Brief/来源、原执行授权和精确 Execution/Head/Job；Provider 只读取快照中冻结的版本，不用新的 Binding/Profile 替换进行中的执行。Snapshot 同时冻结准备操作者的 Token Version，以重算原准备回执输入 Hash，不把当前领取者冒充原准备人。使用同一 Registry/Compiler 重编译并核对完整 read-set 和 Call 请求身份后，按 expected Call revision CAS 完成 `PENDING → DISPATCHING`。只有该事务提交成功的调用路径返回 `should_dispatch=true`；重复命令和事务失败均不能再次返回许可。

调用运行状态与不可变身份分开保存：状态内容 Hash 覆盖 Call key、revision、submission token、领取者及其 Token Version、UTC 微秒发送边界时间、冻结 deadline 和 outcome-unknown 时间。submission token 全局唯一，初始 PENDING 没有 token。Workspace 锁内直接统计既有 Call：DISPATCHING 与 OUTCOME_UNKNOWN 均占用未解决调用上限；当日 UTC 时间范围内已越发送边界的记录占每日调用数。超限不改变 Call，不新增额度/计费表，未知结果不会自动释放未解决额度。

deadline 到期后，恢复命令只读取冻结执行、完整 Job/Call 身份和运行状态，验证当前操作者的项目权限及原 submission token；不重读最新 Provider、重编译已过期业务来源或调用 Submit。未到 deadline 保持 DISPATCHING；到期 CAS 标记 OUTCOME_UNKNOWN；重复恢复幂等，永不退回 PENDING。该步骤只落地发送权与丢失结果保护，实际 Provider 传输、成功/失败观察、媒体 staging 与 Temporal 调度按后续实施，不把取得许可当远程调用成功。

## 8. Staged Media、CandidateBundle 与 QC

### 8.1 Generation-owned staged media

Provider 输出立即流式进入私有对象存储并生成：

```text
GenerationStagedMediaObject
├── staged_media_id / workspace_id / project_id
├── provider_call_ref / output_identity
├── object_store_ref
├── sha256 / byte_size / media_type
├── width / height
├── decoding_contract_ref
├── provider_receipt_ref
├── rights_provenance_observation
├── state = quarantined | ready_for_review | rejected
└── content_hash / created_at
```

Provider URL、对象存储临时 URL、原始响应和 Base64 不进入正式事实。下载、解码、MIME、尺寸、字节上限、恶意文件和 digest 校验全部通过后才是 `ready_for_review`。Staged Media 仍由 Generation 拥有，未选择或失败对象按版本化 retention policy 清理；清理不删除 Receipt、digest 和审计身份。

首次 PNG 媒体持久化消费既有成功 Call Receipt，不再次请求 Provider。Backend 在短事务内验证当前操作者权限、exact Execution/Job/Call/Receipt 和冻结输出槽位，登记 `quarantined` 媒体；`staged_media_id` 使用唯一 submission token，Call key 唯一，记录私有存储 profile/bucket/object key、Receipt 引用、digest、尺寸和字节数。记录由 Generation 的 GORM Catalog 管理，不借用 Asset Artifact/Readiness 发布路径。rights provenance 只记录 `not_assessed` 和来源 Receipt，不推断商业授权或许可。

对象读取发生在事务之外，复用私有对象 `ReadVerified` 端口并再次校验实际字节、SHA-256、PNG 完整解码、尺寸、比例和无尾随内容；解码合同以语义名称和内容 Hash 标识。对象存储暂时不可读时保留 quarantined，重试同一媒体身份；校验明确失败则 CAS 为 rejected，不能人工覆盖为 ready。成功 CAS 为 ready_for_review，原始 Receipt 和 Provider 状态不变。完成状态不可原位改写；Bundle/Vision/Owner Apply 仍须重新核对所消费媒体与 Policy，ready 不代表 rights、语义或最终资产通过。

现有 Reference Call Activity 在收到成功 Receipt 后调用该媒体服务，只有媒体 ready 才完成节点；读取或提交后丢失响应可安全重入，不再次 Submit、不生成新媒体身份。节点继续只输出 Receipt 引用，后续 Bundle 按 exact Call 读取 Generation-owned 媒体。此步骤不新增 Workflow 类型或 Provider 参数，不提前发布 CandidateBundle、Selection 或 AssetVersion。

### 8.2 CandidateBundle

```text
ReferenceCandidateBundleInputContract
├── bundle_input_id / generation_target_ref / execution_ref
├── candidate_bundle_index
├── slots[]
│   ├── slot_key / view_role
│   ├── provider_call_ref / staged_media_ref
│   └── deterministic_qc_result_ref
├── slot_set_root
├── bundle_completeness
├── bundle_deterministic_qc_result_ref
├── dependency_root_hash
├── content_hash
└── created_at

GenerationCandidateBundleProduction
├── candidate_bundle_id / generation_target_ref / execution_ref
├── candidate_bundle_index
├── bundle_input_ref / bundle_input_hash
├── bundle_vision_review_candidate_revision_ref
├── dependency_root_hash
├── content_hash
└── created_at

GenerationCandidateSetProduction
├── candidate_set_id / target_ref / execution_ref
├── ordered_candidate_bundle_refs[]
├── failed_or_unknown_slot_refs[]
├── generation_completion_state
├── content_hash
└── created_at
```

构造顺序严格单向：`Call/Staged/per-slot QC → canonical slot_set_root → bundle deterministic QC → ReferenceCandidateBundleInputContract → Vision Invocation/Candidate → GenerationCandidateBundleProduction → CandidateSet`。Bundle deterministic QC Result 的 input root 必须等于 `slot_set_root`，不得引用 Bundle Input；Vision Candidate 的 input hash 覆盖 bundle input ref/hash，但 Bundle Input 不引用 Vision，因此两处都不存在内容哈希环。

`generation_completion_state` 固定为 `complete|partial_explicit_failure|outcome_unknown`。Bundle Input 必须来自同一 Target、round、Execution Snapshot、bundle index 和 dependency root。任何必需 slot 缺失、unknown 或跨 Bundle 混合都不得标为 complete。部分明确失败可以展示诊断，但只有 complete 且 deterministic QC passed 的 Bundle Input 能进入 Vision Review；最终 CandidateBundle 还必须绑定成功且 fence 有效的 bundle-level Vision Candidate。任一 Call outcome unknown 时 CandidateSet 只能用于对账，不开放选择。

### 8.3 两层审查

确定性 QC 与 Agent Vision Review 不得合并：

| 层 | 负责 | 必查项 | 是否可由人覆盖 |
|---|---|---|---|
| Deterministic QC | Backend | digest、解码、媒体/尺寸/比例、slot 完整、输入/输出身份、重复图片、rights policy、恶意内容 | 否 |
| Vision Review | `review_reference_artifact` Skill | 视图语义、Identity/State、Style、跨资产一致性、Interaction 接触 | `warn/not_assessable` 可显式确认；`fail` 不可直接选择 |

Vision Review 一次读取同一 complete `ReferenceCandidateBundleInputContract` 的全部 required Staged bytes，以及 exact Target、Brief、Style/Policy 和 dependency Assets，输出一个 bundle-level `vision_review_candidate_production`；Issue 可指向单一 slot 或跨 slot 区域。这样 front/profile/back 的脸型、体型、服装和标记一致性可以整体审查，而不是三次彼此失联的单图判断。它不能修图、生成新媒体或选择 Bundle。任一 required rubric 为 `fail` 时 Bundle 不可选择；`not_assessable` 必须在 CandidateSelection 中逐项确认风险，不能默认为 pass。

### 8.4 CandidateSelection

```text
GenerationCandidateSelectionProduction
├── selection_id / selection_revision / workspace_id / project_id
├── generation_target_ref / execution_ref / candidate_set_ref
├── selected_candidate_bundle_ref
├── accepted_warning_issue_refs[]
├── acknowledged_not_assessable_issue_refs[]
├── human_review_decision_ref
├── target_head_fence / candidate_set_hash / dependency_root_hash
├── content_hash
└── selected_by / selected_at
```

同一 Target round 使用 `GenerationCandidateSelectionHeadContract` expected revision CAS，最多指向一个 active Selection。重复同值选择幂等；改选必须创建新的 ReviewDecision 和 Selection revision，并且尚未被 Owner Apply 消费。Selection 会 pin 所选 Staged Media 到 Owner Apply 或显式废弃完成，普通 retention 不能提前清理。Owner Apply 完成后不能原位改选，必须新 generation round 或显式发布新的结果 Version。

## 9. Owner Apply 与 Gate 4

### 9.1 基础结果发布

`PublishSelectedBaseReferenceResultCommand` 只接受前四类 Target。协调事务必须：

1. 锁定 active Plan/Target、Selection、Asset scope head 和相关 Owner heads；
2. 重算 Target/Brief/Execution/CandidateSet/Selection/Style/Policy/dependency fences；
3. 验证所选 Bundle 所有 required slot 的 Staged bytes 仍存在、digest 相等、QC/Vision 条件满足；
4. 由 `asset` 将每个 slot 晋升为不可变 Artifact/Rendition，保留 provider/generation lineage，但不复制 Provider URL；
5. 发布唯一 purpose 对应的 AssetVersion，写 `fulfilled_reference_target_ref`、exact Identity/Specification/State、Style/Policy 和 Selection；
6. Appearance 额外写 exact `identity_anchor_asset_version_ref`；其他 purpose 必须为 null；
7. 更新 base membership/head，写 Owner Command Receipt/Outbox；required closure 尚未完成不冒充 Gate 4 checkpoint。

若同一 Plan Target 已有 active Result，显式重新生成的 Apply 必须 CAS 对应 Asset Head，发布新的 AssetVersion revision 并让 active membership 只指向新版本；旧版本保留历史和 lineage，但不能与新版本同时作为该 Target 的 active fulfillment。

三视图/地点板/道具板通过结构化 Rendition `view_role` 覆盖表达；不得只把 composite sheet URL 写进 AssetVersion。若 Provider 原生只返回拼板，Normalizer 必须在 Generation 阶段按版本化布局合同产生可验证的独立 slot，无法可靠分离则该 Adapter/Profile 不具备此 Target capability。

### 9.2 组合结果发布

`PublishSelectedCompositionResultCommand` 只接受 Scene/Interaction Composition，在一个数据库事务中协调 `asset` 与 `production/reference`：

- `asset` 将选中 `composition_master` 晋升为 READY Composition Artifact 并写 scene-scoped composition membership；
- `production/reference` 发布 `SceneReferenceBindingVersion` 或 `InteractionReferenceBindingVersion`；
- Binding 冻结 selected Composition Artifact、全部 exact base AssetVersions、fulfilled Target、Scene/Occurrence/Interaction/Continuity refs、Style/Policy 与 Selection；
- Binding 的 base version closure 必须与 Target source/dependency root 逐字节等价；
- Interaction Binding 必须逐字节验证 Target/Claim 中的 holder、hand、contact、orientation、scale 结构约束，并验证 Vision Review 确实针对所选 bytes 覆盖这些 rubric、Human Decision 未选择 `fail`；Backend 不声称自行理解像素；
- 组合结果不创建 AssetIdentity、AssetState 或 AssetVersion，不把人物拿道具的姿势污染成角色永久外观。

同一 Scene/Interaction Target 的重新生成以 expected Binding Head CAS 发布新 Binding revision，并原子替换 scene-scoped active composition membership；旧 Artifact/Binding 保留历史，但 Compiler 对该 Target 只能看到一个 active Result。

任一 Owner 校验失败整体回滚。Provider 成功、Candidate complete、Vision Review pass、Human Selection 和 Artifact 晋升均不单独等于 Reference Binding 已发布。

### 9.3 Checkpoint

- `CompleteBaseReferenceSelectionCheckpointCommand`：全部 active required base Target 有精确 AssetVersion 才成功；
- `CompleteSceneCompositionSelectionCheckpointCommand`：按 Scene 验证 required Scene/Interaction Composition Binding、composition artifact set 与 dependency closure；
- Gate 4 总完成：全部计划内 active Scene scope checkpoint 已完成，且 required target coverage 精确闭合。

Checkpoint 使用 D09 的 Collection Receipt，不写一张“gate passed=true”捷径表。optional 未选择和 `not_generated` 是 Plan 中的显式状态，不生成假 Result。

## 10. 命令、Query 与接口边界

本文固定语义命令，不固定 HTTP 路径：

| Command / Query | 输入 | 输出/副作用 |
|---|---|---|
| `BuildReferenceGenerationTarget` | exact Plan/Target/Brief/Style/Policy/deps/authorization | immutable Provider-neutral Target |
| `PrepareReferenceGenerationExecution` | Target + exact Project Binding | immutable Execution Snapshot / ProviderCalls |
| `ClaimProviderCallDispatch` | Call expected revision | 唯一 `should_dispatch` fence |
| `RecordProviderCallObservation` | Call/Adapter contract/typed observation | immutable Receipt/state transition |
| `MaterializeStagedMedia` | successful Call/output identity | private staged bytes + digest |
| `BuildReferenceCandidateBundleInput` | Target/Execution/same-index slots/QC | immutable acyclic Bundle Input manifest |
| `AssembleReferenceCandidateSet` | Bundle Inputs + bundle-level Vision Candidate refs | immutable CandidateBundle/Set |
| `SelectReferenceCandidate` | Human ReviewDecision + complete Bundle | immutable Selection |
| `GetReferenceGenerationStatus` | Project/Plan/Target refs | typed state，不返回 Secret/临时 URL |
| `GetReferenceCandidateComparison` | exact CandidateSet | ordered slots、review issues、safe preview handles |

Owner Apply 命令属于 `asset`/`production/reference` 协调器，不属于 Generation API。Agent 只能通过 Stage Invocation 获得 Brief/Vision Review输入；不得暴露 `publish_asset` 或任意 SQL Tool。浏览器只调用 Backend，不能直连 Provider 或对象存储私有地址。

所有 Command 绑定 Workspace、Project、Membership/Token Version、command idempotency key、expected revision/hash。所有 Query 先授权再返回防枚举错误；preview handle 短时、只读、绑定用户/对象/用途，不作为 Artifact identity。

## 11. Hash、Read Set、Fence 与 stale

### 11.1 Target read set

```text
TargetReadSetProduction
├── project_reference_plan_activation_head_ref
├── reference_plan_scope_head_ref
├── reference_plan_target_ref
├── generation_target_head_expected_revision
├── reference_brief_candidate_head_ref
├── effective_style_head_ref / effective_policy_head_ref
├── source_owner_head_refs[]
├── dependency_asset_version_refs[]
├── asset_readiness_refs[]
├── skill_release_control_refs[]
└── root_hash
```

Execution Snapshot 再增加 Provider Binding/Profile/Connection/Credential heads 与 Adapter Registry release hash。Bundle Input 增加 Call/Receipt/Staged/QC roots，CandidateBundle 再增加 Vision Review root，CandidateSet 聚合全部 Bundle roots。Selection 增加 Human ReviewDecision fence。Owner Apply 在短事务内重读这些 exact roots；不能只验证 Target content hash 而忽略其所属 active Plan 或控制面。

```text
ExecutionReadSetContract
├── generation_target_ref / target_read_set_root
├── generation_execution_head_expected_revision
├── project_provider_binding_version_ref
├── provider_connection_version_ref / provider_credential_version_ref
├── provider_model_profile_version_ref
├── adapter_registry_release_hash / request_compiler_contract_ref
├── operational_limit_policy_ref
└── root_hash
```

`GenerationExecutionSnapshotContract.execution_read_set_root` 必须等于上表 canonical root；准备、恢复和 CandidateSet assemble 均重算，而不是把 Snapshot 自报 hash 当作证明。

### 11.2 stale 分类

| 变化 | 影响 | 不影响 |
|---|---|---|
| Plan/Target fulfillment、business key、coverage 或依赖变化 | 旧未 Apply Target/Execution/Candidate/Selection stale | 历史 Receipt/Staged digest |
| Brief Revision 变化 | 对应 Target 下游 stale | 其他 Target |
| Effective Style/Policy 变化 | 全部视觉 Target 下游 stale | p0 Scene Fact/Identity/Occurrence |
| Anchor AssetVersion 变化 | 同 Identity Appearance + 依赖它的 Composition stale | 无关地点/道具 |
| 任一 base AssetVersion 变化 | 引用它的 Scene/Interaction Composition stale | 其他 Scene closure |
| Provider Binding/Profile 变化 | 新 Execution 使用新版本；进行中旧执行保持冻结 | 已发布业务 Result |
| Skill Release quarantine/revoke | 尚未 Apply 的相关 Brief/Vision Candidate 不可继续 | 已提交 Owner 历史版本，除非另有撤销流程 |
| Human Selection 改变 | 尚未 Apply 的结果改用新 revision | 已 Apply 结果不原位覆盖 |

已发布旧 Result 继续保留审计，但若不再履约 active Plan，Compiler/Packet Query 不把它当当前结果。失效通过新 active Plan/Head 和 Result coverage 判定，不删除历史版本。

### 11.3 锁序

跨模块命令沿用 D09 固定锁序：

```text
Workspace/Membership
→ Project activation / Plan heads
→ Preset heads
→ Reference heads
→ Asset heads
→ Generation Target/Execution/Call/Candidate/Selection
→ ReviewDecision
→ Owner Receipt/Outbox
```

远程调用、媒体下载、解码、Vision Agent Invocation 和对象存储上传均在数据库事务外。事务只认已持久化的不可变观察和 digest。

## 12. 失败与恢复

| 场景 | 稳定结果 | 恢复动作 |
|---|---|---|
| Target `not_generated` | `reference_target_not_executable`，零副作用 | 修改 Plan 需新 Gate 3 Version |
| Appearance anchor 未发布 | `reference_dependency_not_fulfilled` | 先完成 exact anchor |
| Composition base closure 缺失/多余 | `reference_dependency_closure_mismatch` | 完成或修正对应基础 Result |
| Brief stale/Release revoked | `reference_brief_fence_failed` | 新 Invocation/Brief/Target |
| Provider 未配置 | `provider_configuration_required` | 配置一条可用图片 Binding，不影响其他流程 |
| Capability 不支持 slot/输入 | `provider_capability_mismatch` | 显式换 Binding 并新 round，不静默降级 |
| 本地编译失败 | Call `FAILED_LOCAL`，未越发送边界 | 输入不变的暂态基础设施故障可用新 Execution Authorization；业务输入错误需新 Target |
| Submit 已越边界、结果丢失 | `OUTCOME_UNKNOWN` | 有 remote id 则 Query；否则人工对账/显式新 round |
| 某 required slot 失败 | Bundle incomplete | 其他 Bundle 可继续；同 Bundle 不拼接别的方向 |
| Staged 下载中断 | Receipt 保留，媒体未 ready | 按同 output identity/digest expectation 重试下载 |
| deterministic QC fail | Staged rejected | 不能人工覆盖；显式新 generation round |
| Vision Review fail | Bundle 不可选择 | 修 Brief/依赖并新 round |
| Review warn/not_assessable | 可见风险 | Selection 必须逐项确认 |
| Selection 与 Head 并发漂移 | `candidate_selection_stale` | 重读 CandidateSet/Review 后新 Decision |
| Owner Apply fence 漂移 | 全事务回滚 | 新 Target/Selection 或重新确认依赖 |
| Worker/Temporal 重启 | 不创建第二 Target/Call | 从 PostgreSQL + Temporal History 恢复 |
| 用户取消 | 停止未派发 Call/后续节点 | 已越发送边界保留对账，不谎报远端已取消 |

失败码属于版本化合同，不把 Provider 原始错误、Prompt、Secret 或 HTTP body直接返回前端。Provider-specific error 规范化后保留诊断 ref；可重试只针对明确未送达或官方可查询的操作，不对 outcome unknown 自动重提。

## 13. 安全、权利与保留策略

- Secret 只存在于 TLS Request、Backend 短生命周期解密缓冲区与当前 Adapter Authorization；不进入 Target、Temporal History、日志、Trace、Metric label、Outbox、Candidate 或 Artifact metadata；
- Provider Host/Region 由编译 allowlist/Connection preset 决定，不提供任意 URL、Header 或反向代理；
- 输入 Asset 使用 Backend 受控上传或短期签名传输，不向 Provider暴露 MinIO 私有地址；
- 每次下载和重定向都校验 scheme/host/DNS/IP/端口/大小/MIME/解码结果，禁止 SSRF 与压缩炸弹；
- Brief/Target 必须携带 rights/provenance requirements，Execution 前校验依赖 Artifact 的授权适用范围；
- 未选 Staged Media 按 policy 到期清理，Receipt、hash、选择和正式 Artifact 不随临时对象清理丢失；
- safe preview 必须水印/限时/只读，并遵守 Workspace 授权；
- Agent Vision 输入使用内容定址附件，不通过公开 URL 或自然语言中的 URL 取图。

## 14. 可观测性与完成度

每个 Target 的状态由确定性聚合 View 计算，不另建可变“进度事实”：

```text
planned
→ brief_ready
→ target_ready
→ executing
→ candidates_ready
→ awaiting_selection
→ selected
→ owner_applied
→ checkpoint_counted
```

状态允许 `blocked(code/scope)`、`failed(code/scope)`、`outcome_unknown(call_ref)`，但不能把 Candidate 数量或 Workflow 百分比映射为 Gate 通过。关键 Metrics 只使用低基数维度：target kind、stage、provider key、status、failure code；不得使用 Workspace ID、Prompt、人物名、Secret 或 Artifact URL 作为 label。

MVP 产品指标直接支持 Production-ready Scene Coverage：

- required base target fulfilled ratio；
- required scene/interaction composition fulfilled ratio；
- identity-anchor→appearance dependency closure；
- Interaction contact failure/rework ratio；
- selected-to-owner-applied latency；
- outcome unknown 与重复发送防护计数。

费用、收入、毛利和 Provider 单价不是本阶段产品成功指标。

## 15. MVP 与 Platform Complete

### 15.1 当前 MVP 必交

1. 六类 strict Target 全部可从同一 active Approved Plan 构建；
2. identity anchor、location、prop 可先行，Appearance 精确消费 anchor，Composition 精确消费全部基础版本；
3. 至少一条真实图片 Provider path 能覆盖所有 Target/output slot，不返回占位图；
4. CandidateBundle 保持视图方向一致，QC/Vision/Selection/Owner Apply 边界可验证；
5. 人物持道具的 Interaction Composition 能证明 holder、手别、握点、方向和比例；
6. base 发布为 AssetVersion，composition 发布为 Artifact + Reference Binding，且都履约 exact Plan Target；
7. 局部失败、重生成、并发、重启和 outcome unknown 不重复远程发送或发布错误结果；
8. 剧本解析、Gate 1–3 和查询在零 Provider 配置时仍可工作；
9. 不新增付费/PriceQuote 产品门，不以 Provider 数量替代视觉闭环；
10. Gate 4 后的 SceneProductionPacket 只消费已发布 exact Result。

### 15.2 Platform Complete 保留目标

- Seedream、GPT Image、Nano Banana 等多图片 Adapter 的精确 Profile 与真实旅程；
- Seedance、Shot Frame/Video、音频和最终媒体 Production Binding；
- Provider 管理 UI 的完整广度、模型能力目录和连接验证；
- 账单、价格、配额商品化和对账中心；
- 经新 Design 接受的 Provider 选择策略、批量优化与可证明 fallback。

Platform Complete 目标不得通过当前 `SG-I21` 未提交增量、旧 Runware Evidence、假 Adapter、目录占位或受控 Gateway 报告为完成。

## 16. 实施约束与验收门

`VP-D15` 接受前不修改代码。后续 Plan 必须按 Red → Green → Refactor 拆出可独立验收的垂直切片，不能先建空 Generation 多候选执行契约 框架。实现至少机械证明：

1. `approved_storyboard_intents`、`needs_asset` 和泛化 `reference_asset` 不能创建 Reference GenerationTarget；
2. 六类 strict union 对缺字段、额外字段、错误 OwnerRef、错误 Style/Policy 和错误 dependency 全部失败；
3. 同一 Plan 每个 Character 恰一 anchor，两个 Appearance State 共用同一 exact anchor AssetVersion；
4. 三视图/地点板/道具板各 required slot 完整，跨 Bundle 拼接被拒绝；
5. Scene closure 多/少一个 base AssetVersion 都无法执行或 Apply；
6. 人物—道具 interaction 的 holder/hand/contact/orientation/scale 任一漂移被拒绝；
7. Provider Binding 在 Target 之后冻结，切换 Provider 不改变业务 Target hash；
8. 同一 Call 只有一次 `PENDING → DISPATCHING` 发送权，重启/并发/Activity 重投不产生第二次 Submit；
9. outcome unknown 无 remote id 时不自动重提，有 remote id 时只 Query 同一任务；
10. Provider URL、Secret、Prompt、Base64 和原始响应不进入正式事实或日志；
11. deterministic QC fail 不可人工覆盖，Vision fail 不可选择，warn/not_assessable 必须显式确认；
12. CandidateSelection 不能直接创建 AssetVersion/Binding，Provider/Agent/Workflow 也不能越权写 Owner；
13. base/composition Owner Apply 的 Receipt、Head、Artifact/Rendition 与 fulfilled Target 可逐字节反查；
14. required Target 未闭合时 Gate 4 checkpoint 不成立，optional/not_generated 不生成假 Result；
15. 零 Provider 配置时非视觉流程通过，真实图片路径配置后六类 Target 端到端通过；
16. 全量 CI、空库重复启动、Compose、对象存储、日志/Secret hygiene 和重启恢复按当时真实环境通过。

本 Design 通过独立评审和提交后只解锁 `VP-D11`：把五个用户 Gate 映射为具体 HumanTask Subject、Decision、lease、恢复和 Owner Apply fence。它不授权提前修改 Generation、Workflow、Agent、Asset、Reference、OpenAPI 或前端代码。
