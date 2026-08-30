# 通用媒体 Provider 与 Generation 执行器设计

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
  → Backend Provider-neutral GenerationTargetV2
  → exact GenerationExecutionSnapshotV1
  → ProviderCall(s) + immutable staged media
  → CandidateBundle(s)
  → deterministic QC + vision_review_candidate_v2
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

`ApprovedReferencePlanVersion` 不包含 Provider、模型、价格、Prompt、Candidate 或“当前资产”。`GenerationTargetV2` 也先保持 Provider-neutral；只有 Target 通过业务、依赖、Brief、Style/Policy 和输出合同校验后，执行准备阶段才冻结具体 Provider Binding/Profile/Credential 与请求编译器版本。任何阶段都不得回到旧 `approved_storyboard_intents`、`reference_asset` 泛化 Target 或 `needs_asset` 反向触发路径。

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

- 六类 `GenerationTargetV2` 严格联合和输出槽位合同；
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
- StoryGraph 只在 Owner Apply 后投影正式 Result；GenerationTarget、Provider 配置和未选 Candidate 不进入 `storygraph-v2` 权威内容图。

## 4. 从 Reference Plan 到可执行 Target

### 4.1 唯一来源与激活条件

Target Builder 只接受当前 Project 唯一 active `ApprovedReferencePlanVersion` 内的精确 `ReferencePlanTargetVersionRef`。必须同时证明：

1. Project activation head 与 Plan scope head 指向同一 exact Plan；
2. Target 属于该 Plan，业务键、Target kind、coverage、依赖和 fulfillment 与 Plan hash 一致；
3. `fulfillment=not_generated` 时返回 `reference_target_not_executable`，不创建 Brief、Target、Execution、Job 或 Staged Media；
4. `required|optional` 均可生成，但 optional 未选择不阻断 Gate 4；
5. Target 已被同一 Plan 的正式结果履约时，普通 Start 返回 `reference_target_already_fulfilled`；显式重新生成必须有新的 `ReferenceGenerationAuthorizationV1(kind=regenerate_candidates)` 和递增 generation round。

不得用 Target kind、显示名、Scene ID 或相似资产在项目中搜索替代项；不存在 `current/latest/default asset` 读取。

### 4.2 Target Builder 两阶段输出

业务 Target 与执行配置分离：

```text
BuildReferenceGenerationTargetCommand
  → GenerationTargetV2（Provider-neutral）

PrepareReferenceGenerationExecutionCommand
  → GenerationExecutionSnapshotV1（exact Provider-specific）
```

`GenerationTargetV2`：

```text
GenerationTargetV2
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
├── source_payload: ReferenceGenerationSourceV2 strict union
├── dependency_asset_version_refs[]
├── dependency_root_hash
├── output_contract: ReferenceOutputContractV2
├── target_read_set_root
└── created_by / created_at
```

Target content hash 排除 `created_by/created_at` 审计字段，但覆盖其余完整字段。Target Builder 先在同一个可重复读快照内重算 `TargetReadSetV2`，再以 expected Head CAS 发布 Target；相同命令与相同 canonical input 返回同一 Target。任何 Plan、Brief、Style、Policy、source 或 dependency 漂移都必须创建新 Target，不原位改写。

`GenerationTargetHeadV2` 以 `(workspace, project, approved plan ref, reference plan target ref)` 为键，只用 expected revision CAS 指向最高已授权 generation round；`GenerationExecutionHeadV1` 以 Target ref 为键；`GenerationCandidateSelectionHeadV1` 以 Target ref 为键。三个 Head 都只是并发索引，不复制 Target、Execution 或 Selection 内容，也不提供“查最新资产”能力。

生成授权与执行授权分离：

```text
ReferenceGenerationAuthorizationV1
├── kind = initial_generation | regenerate_candidates
├── approved_reference_plan_version_ref / reference_plan_target_ref
├── requested_candidate_bundle_count
├── base_generation_target_ref? / base_candidate_set_ref?
├── reason_code
├── human_action_ref / membership_token_version
└── content_hash / authorized_by / authorized_at

ReferenceExecutionAuthorizationV1
├── kind = initial_execution | retry_before_dispatch | switch_provider
├── generation_target_ref
├── previous_execution_ref?
├── selected_project_provider_binding_version_ref
├── reason_code / unresolved_call_acknowledgements[]
├── human_action_ref / membership_token_version
└── content_hash / authorized_by / authorized_at
```

`generation_round=1` 必须绑定 `initial_generation` 用户动作、Gate 3 已批准 Target 与有效 Brief；round 大于 1 必须绑定 `regenerate_candidates`，并覆盖 reason、base Target/CandidateSet refs 和授权 scope。Temporal Activity 重投、Provider retry、Provider 切换或媒体下载重试都不能递增 generation round；它们只可能在同一 Target 下创建新的、显式授权的 Execution Snapshot。

### 4.3 Reference Brief fence

每个可执行 Target 必须绑定恰 1 个 `reference_brief_candidate_v2` 成功 Candidate Revision：

- Candidate target ref、kind、business key、source refs、dependency refs、Style/Policy refs 与正在构建的 Target 逐字节相等；
- Candidate 所在 Stage Release 未 quarantine/revoke，Invocation Outcome、Attempt result、Candidate Head 和 review/repair chain 均有效；
- Candidate 未被上游变更标 stale；
- Brief 包含 source-vs-design slots、positive/negative instructions、rights/provenance requirements、输出视图角色和 QC rubric refs；
- Brief 不是自由 Prompt。Provider Request Compiler 只能把这个结构化 Brief 映射为对应模型请求，不能补人物、换道具、换状态或改写世界事实。

无 Brief、旧 Brief、Target 不匹配或自由文本 Prompt 均在 Provider Binding、远程调用和媒体创建前失败关闭。

## 5. 六类 Target strict union

`ReferenceGenerationSourceV2` 只允许以下六个分支；所有 OwnerRef 都包含 `owner_kind/version_family/owner_logical_id/revision/content_hash/fragment_key?`，数组按 canonical key 排序去重，`additionalProperties=false`。

### 5.1 Character Identity Anchor

```text
CharacterIdentityAnchorSourceV2
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
CharacterAppearanceSourceV2
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
LocationBoardSourceV2
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
PropSheetSourceV2
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
SceneCompositionSourceV2
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
InteractionCompositionSourceV2
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
ReferenceOutputContractV2
├── contract_id = reference-output-v2
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
  → same GenerationTargetV2 strict union
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
GenerationExecutionSnapshotV1
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

执行准备必须证明：

- Binding 属于同 Workspace/Project，已启用且 modality/capability 覆盖 Target 全部 slot；
- Model Profile 能逐字节实现 Target 的尺寸、比例、输入图和 slot 数，不得静默裁切、取整、降质量或换模型；
- Request Compiler contract 能将当前 Brief schema 和 Target kind 映射到该 Adapter；
- 所有依赖 Artifact 都 READY、rights/lineage 可用于当前 Provider；
- 同一 Target 通过 `GenerationExecutionHeadV1` 的 expected revision CAS 最多指向一个 active Execution Snapshot。更换 Binding/Profile 必须有新的 `ReferenceExecutionAuthorizationV1`；如果旧 Call 已越发送边界，还必须逐项确认 unresolved/remote invocation 风险。Provider 切换不改变业务 Target hash，但旧 Execution 的 CandidateSet 立即失去 active execution fence；若目标是再生成一批新 Candidate 而不是恢复未完成执行，则必须新 generation round。

无 Provider 配置时返回 `provider_configuration_required`，但 Backend、剧本解析、Gate 1–3、查询和非视觉 Workflow 正常。当前 MVP 只要求至少一条真实、可恢复、可审核的图片执行路径覆盖六类 Target；此前接受的 Seedream、GPT Image、Nano Banana 与 Seedance 广度仍是 Platform Complete，不能用本步声明为已完成。

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

### 7.3 调用身份与状态机

一个 CandidateBundle 的每个 required slot 对应一个确定性 `ProviderCallKey`：

```text
ProviderCallKeyV2
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

`GenerationProviderJobV2` 是一个 Execution Snapshot 的本地聚合，只保存 expected `ProviderCallKeyV2` 完整集及其 root。全部 Call 成功才是 `SUCCEEDED`；至少一个成功且其余全部明确失败是 `PARTIAL_SUCCEEDED`；全部明确失败是 `FAILED`；任一 Call 尚未解决或 `OUTCOME_UNKNOWN` 时 Job 不得假终态。Job 聚合不拥有远程发送权，也不能用“整体重试”创建第二套 Call。

当前 MVP 不建立新的动态 PriceQuote/付费 Reservation 完成门。运行保护只冻结 `OperationalGenerationLimitPolicy`：单 Target 最大 Bundle、单 Bundle 最大 slot、最大输入/输出字节、并发、超时和每日运维调用上限。它不表达货币、用户余额或套餐，且策略缺失时使用代码内安全默认值，不阻断已授权 MVP 旅程。

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

### 8.2 CandidateBundle

```text
ReferenceCandidateBundleInputV1
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

GenerationCandidateBundleV2
├── candidate_bundle_id / generation_target_ref / execution_ref
├── candidate_bundle_index
├── bundle_input_ref / bundle_input_hash
├── bundle_vision_review_candidate_revision_ref
├── dependency_root_hash
├── content_hash
└── created_at

GenerationCandidateSetV2
├── candidate_set_id / target_ref / execution_ref
├── ordered_candidate_bundle_refs[]
├── failed_or_unknown_slot_refs[]
├── generation_completion_state
├── content_hash
└── created_at
```

构造顺序严格单向：`Call/Staged/per-slot QC → canonical slot_set_root → bundle deterministic QC → ReferenceCandidateBundleInputV1 → Vision Invocation/Candidate → GenerationCandidateBundleV2 → CandidateSet`。Bundle deterministic QC Result 的 input root 必须等于 `slot_set_root`，不得引用 Bundle Input；Vision Candidate 的 input hash 覆盖 bundle input ref/hash，但 Bundle Input 不引用 Vision，因此两处都不存在内容哈希环。

`generation_completion_state` 固定为 `complete|partial_explicit_failure|outcome_unknown`。Bundle Input 必须来自同一 Target、round、Execution Snapshot、bundle index 和 dependency root。任何必需 slot 缺失、unknown 或跨 Bundle 混合都不得标为 complete。部分明确失败可以展示诊断，但只有 complete 且 deterministic QC passed 的 Bundle Input 能进入 Vision Review；最终 CandidateBundle 还必须绑定成功且 fence 有效的 bundle-level Vision Candidate。任一 Call outcome unknown 时 CandidateSet 只能用于对账，不开放选择。

### 8.3 两层审查

确定性 QC 与 Agent Vision Review 不得合并：

| 层 | 负责 | 必查项 | 是否可由人覆盖 |
|---|---|---|---|
| Deterministic QC | Backend | digest、解码、媒体/尺寸/比例、slot 完整、输入/输出身份、重复图片、rights policy、恶意内容 | 否 |
| Vision Review | `review_reference_artifact` Skill | 视图语义、Identity/State、Style、跨资产一致性、Interaction 接触 | `warn/not_assessable` 可显式确认；`fail` 不可直接选择 |

Vision Review 一次读取同一 complete `ReferenceCandidateBundleInputV1` 的全部 required Staged bytes，以及 exact Target、Brief、Style/Policy 和 dependency Assets，输出一个 bundle-level `vision_review_candidate_v2`；Issue 可指向单一 slot 或跨 slot 区域。这样 front/profile/back 的脸型、体型、服装和标记一致性可以整体审查，而不是三次彼此失联的单图判断。它不能修图、生成新媒体或选择 Bundle。任一 required rubric 为 `fail` 时 Bundle 不可选择；`not_assessable` 必须在 CandidateSelection 中逐项确认风险，不能默认为 pass。

### 8.4 CandidateSelection

```text
GenerationCandidateSelectionV2
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

同一 Target round 使用 `GenerationCandidateSelectionHeadV1` expected revision CAS，最多指向一个 active Selection。重复同值选择幂等；改选必须创建新的 ReviewDecision 和 Selection revision，并且尚未被 Owner Apply 消费。Selection 会 pin 所选 Staged Media 到 Owner Apply 或显式废弃完成，普通 retention 不能提前清理。Owner Apply 完成后不能原位改选，必须新 generation round 或显式发布新的结果 Version。

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
TargetReadSetV2
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
ExecutionReadSetV1
├── generation_target_ref / target_read_set_root
├── generation_execution_head_expected_revision
├── project_provider_binding_version_ref
├── provider_connection_version_ref / provider_credential_version_ref
├── provider_model_profile_version_ref
├── adapter_registry_release_hash / request_compiler_contract_ref
├── operational_limit_policy_ref
└── root_hash
```

`GenerationExecutionSnapshotV1.execution_read_set_root` 必须等于上表 canonical root；准备、恢复和 CandidateSet assemble 均重算，而不是把 Snapshot 自报 hash 当作证明。

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

`VP-D15` 接受前不修改代码。后续 Plan 必须按 Red → Green → Refactor 拆出可独立验收的垂直切片，不能先建空 Generation v2 框架。实现至少机械证明：

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
