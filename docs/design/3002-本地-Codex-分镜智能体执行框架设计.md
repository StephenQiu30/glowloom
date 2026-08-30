# 本地 Codex 分镜智能体执行框架设计

- 状态：已接受设计
- 历史事实：旧版曾于 `SG-D12` 接受 Draft → `needs_asset` → 参考资产 → Detail 链；该顺序由本文整体取代，旧运行只允许按精确 v1 Wire/Bundle/Runtime 完成历史重放
- 接受记录：`VP-D08`（2026-08-30）；产品主链、Wire/Schema、Owner/恢复三轴隔离反例评审均通过（最终正文评审 SHA-256 `8d40b29a8309cf86fc364a72f552b7da7951aa26f75c0b78bc79e851a13cd860`）
- 已接受前置：[剧本视觉生产工作台与世界观预设设计](0011-剧本视觉生产工作台与世界观预设设计.md) · [StoryGraph 内容图与 DAG 创作画布设计](0010-StoryGraph内容图与DAG创作画布设计.md) · [项目制作圣经生成执行框架设计](3001-项目制作圣经生成执行框架设计.md) · [StoryGraph 剧本解析 Harness 与内置 Skill 设计](3003-StoryGraph剧本解析Harness与内置Skill设计.md)
- 历史派生：[产品需求](../prd/3002-本地-Codex-分镜智能体产品需求.md) · [需求规格](../requirement/3002-本地-Codex-分镜智能体执行框架需求规格.md) · [实施计划](../plan/3002-本地-Codex-分镜智能体执行框架实施计划.md) · [验收标准](../acceptance/3002-本地-Codex-分镜智能体执行框架验收标准.md)；全部继续冻结，分别等待 `VP-D13`–`VP-D15` 的统一文档链
- 下一设计门：[后端领域模块功能设计](2002-后端领域模块功能设计.md)（`VP-D09`）

## 1. 结论

Storyboard 不再承担“发现缺什么资产”的职责。一个 Scene 只有在 Gate 4 已完成、全部 required 基础与组合参考均已选择、v2 p2 StoryGraph 已发布并能构建完整 `SceneProductionPacket` 后，才可进入分镜。

```text
Gate 4 complete
→ Backend 验证 scene readiness
→ v2 p2 StoryGraph + exact Owner Collections
→ SceneProductionPacket + 内容定址执行物化 + 只读媒体附件
→ direct_storyboard（单 Scene、vision、单 Candidate）
→ Backend normalizer 生成精确 ShotProductionBindingCandidate
→ deterministic storyboard gates
→ review_candidate(profile=storyboard)
→ repair_candidate(profile=storyboard，必要且有界)
→ Gate 5 Human Review
→ Storyboard Owner 原子 Apply
→ formal Shot + ShotProductionBindingVersion
→ v2 p3 StoryGraph
```

`Shot Intent` 与 `Shot Detail` 仍是产品上可分别阅读的两层，但不再是两个 Stage、两个资产门或两个正式 Candidate Set。`direct_storyboard` 一次输出完整镜头提案；Backend 在同一 normalized `storyboard_candidate_v2` 中形成 Intent、Detail 与机械绑定。这样不会在 Intent 之后反向生成参考资产，也不会让 Detail 使用另一批“当前资产”。

本设计固定以下不可变边界：

1. Agent 只消费一个完整 Packet 及其冻结资源和媒体，不接收项目资产列表、搜索能力或 Provider 工具；
2. Agent 只提出镜头结构、叙事表达、视听语言和 Packet 内 Occurrence/Interaction 的镜头表现，不选择 AssetVersion；
3. Backend 才能把 Occurrence/Interaction 映射到 Packet 内精确 AssetVersion、ReferenceBinding、Artifact/Rendition 和 view role；
4. Candidate、ReviewIssue 和 CandidateRepairPatch 都不是正式 Shot；Gate 5 后只有 Storyboard Owner 可以原子发布正式 Shot 与生产输入绑定；
5. 新 v2 Schema 不存在 `needs_asset`、Draft 资产门、付费门或“先占位后补资产”的合法分支。

本文全部伪 Schema 均为 `additionalProperties=false`；除标记 `?` 或显式 `| null` 的字段外，列出字段全部必需。所有 `*_hash` 使用小写 SHA-256、项目 Canonical JSON 和各自 contract domain separator，对排除自身 hash 的完整对象计算；所有联合都必须以 `kind`/`ref_kind` 等判别字段展开为 `oneOf`，不得实现为任意 map。

## 2. 问题、目标与非目标

### 2.1 旧链路为何必须删除

旧 Draft/Detail 设计存在四个结构性问题：

- Storyboard Draft 在 Production World 尚未完成视觉生产准备时定义 `needs_asset`，把资产规划责任错误地下推给镜头阶段；
- 用户先审镜头意图、系统再准备人物/地点/道具，镜头可能建立在尚未确认的身份、形象和交互上；
- Detail 重新读取资产后，与 Draft 之间形成第二套隐含版本选择，容易混用当前指针或替换版本；
- 计费前门被放在产品主链中心，但当前 MVP 真正需要证明的是剧本拆解、视觉一致性与可生产交接，而不是收费时机。

旧实现、旧测试或旧 Acceptance 只能作为历史事实，不能证明本文的新 Packet-first 链路已经完成。

### 2.2 当前目标

本文只解决 P3 Storyboard 的 Agent/Harness 执行边界：

- Scene readiness 与 Packet 构建完成门；
- `direct_storyboard/default` 的严格 v2 输入、输出、normalizer、review 与 repair；
- Intent/Detail 的同一 Candidate 结构；
- Gate 5 前后的职责、恢复、局部失效和迁移；
- 至少两个相邻真实 Scene 分别从精确视觉生产包生成可审核分镜，并共同验证跨场状态连续性的完成条件。

### 2.3 非目标

本文不定义：

- GORM 表、HTTP 路径、Command/Receipt 字段和事务锁序；由 `VP-D09` 固定；
- GenerationTarget、Provider、价格、Quota、Cost 或媒体调用；由 `VP-D10` 同步；
- HumanTask 状态机和五个 Gate 的公共恢复协议；由 `VP-D11` 固定；
- Guided Studio 页面和编辑交互；由 `VP-D12` 固定；
- Shot 图片、视频、声音、口型或最终合成生成；它们在正式 Shot 之后；
- 多 Scene 自动剪辑、镜头素材生成或自动发布；
- 任何新代码、Migration 或 Bundle 文件；必须等 `VP-D15` 接受后从新 Plan 领取。

## 3. 单 Stage 决策

### 3.1 为什么 Intent 与 Detail 同次产生

Intent 表达“为什么拍、拍什么”，Detail 表达“如何拍”。二者分层是审核需要，不代表它们必须分两次模型调用。

若保留两个 Stage，系统仍要在二者之间定义可变输入、第二次版本选择和失败恢复；而 Gate 4 后 Packet 已具备完整生产输入，Detail 不再需要等待资产。因此 v2 只保留：

```text
StageVariantKeyV2 {
  stage_key: "direct_storyboard",
  profile_key: "default"
}
```

该 Stage 使用 `packet_storyboard` lane、`vision` runtime、`storygraph-stage-wire-v2`，每个 Scene 一个 Stage instance。Review 与 Repair 使用 `review_candidate/storyboard` 和 `repair_candidate/storyboard` 两个已在 `3003` 固定的 profile-bound Variant。

### 3.2 一个 Scene 一个一致性单元

MVP 不把单 Scene 再按 Shot Batch 切开。镜头数量、时长、轴线、调度、对话覆盖和场内连续性必须在一次 Candidate 中整体收敛。跨场连续性只通过 Packet 中冻结的 story-time boundary 只读输入表达。

若完整 Scene 超出已批准 runtime 上下文或媒体上限，Backend 返回 `storyboard_packet_budget_exceeded`，场景保持不可 dispatch；不得静默丢附件、截断对白、拆成无全局 reduce 的 Shot shard 或临时放宽预算。未来若要支持超大 Scene，必须新增被评审的 deterministic map/reduce Contract，不在本设计中预建。

## 4. Scene readiness 与 Packet 完成门

### 4.1 可进入分镜的必要条件

Backend 只有在下列条件全部成立时才能构建可 dispatch 的 Packet：

1. `scene_scope_key` 属于 v2 p2 `VerifiedCoverageProof.p2_scope_keys`，Scene/Beat/Dialogue/Evidence 覆盖完整；
2. Scene 中所有实际 Character/Location/Prop Occurrence 都绑定精确 Identity、Specification 与 AssetState；
3. 所有相关 InteractionClaim、手别/握点/方向/比例、Prop 前后状态和故事时间 continuity boundary 已确认；
4. EffectiveStyleSnapshot、EffectivePolicySnapshot 和唯一 active ApprovedReferencePlanVersion 均为精确不可变版本；
5. 该 Scene 的全部 active required 基础 Target 已有且仅有一个已选 READY AssetVersion；
6. required Scene Composition 和每个 required Interaction Composition 已有且仅有一个已选 READY Binding/Artifact；
7. optional Target 的状态必须明确为已选或未选，`not_generated` Target 不得伪造空 Binding；
8. 所有引用的 ArtifactReadiness、Rights 与 Lineage 可重建且未 quarantine/stale；
9. Packet 中 Owner refs、Graph 投影与 Reference Binding 三者逐项等价；
10. 当前 scope 没有 unresolved、non-convergent、未裁决或机械 blocker。

这些条件由 Backend deterministic readiness gate 计算。Agent 不能把缺口解释为创作选择，也不能返回一个带警告的分镜绕过它。

### 4.2 规范 Packet 与执行物化

规范 `SceneProductionPacket` 继续逐字节使用 `0010` 已接受的 `scene-production-packet-v1`，不增加第二个业务 Record，也不把展示文本写回 Graph。其完整对象和 `packet_content_hash` 随 Invocation 冻结。

规范 Packet 使用 Owner/Evidence/Artifact refs 表达权威身份；隔离的 Agent 又不能查询数据库，因此 transport 还需要把这些 exact refs 物化成只读 Stage resources。该物化不是新业务事实，只是 Packet 的可执行编码：

```text
PacketResourceRefV1 {
  resource_key,
  resource_kind = owner_projection | evidence_slice | policy_projection |
                  continuity_boundary | coverage_manifest,
  source =
    { kind = owner_node_ref, owner_node_ref }
    | { kind = evidence_ref, evidence_ref }
    | { kind = derived, derivation_contract_id, derivation_contract_hash,
        source_owner_refs[], source_evidence_refs[] },
  projection_contract_id,
  projection_schema_hash,
  runtime_relative_path,
  byte_length,
  content_digest
}

PacketLocalRefV1 {
  local_key,
  semantic_kind = scene | beat | dialogue | evidence | occurrence | asset_state |
                  claim | asset_version | reference_binding,
  source =
    { kind = owner_node_ref, owner_node_ref }
    | { kind = evidence_ref, evidence_ref },
  local_ref_hash
}

ScenePacketExecutionMaterializationV1 {
  materialization_contract_id = "scene-packet-execution-materialization-v1",
  packet_contract_id,
  packet_content_hash,
  scene_scope_key,
  resource_refs[],
  packet_local_refs[],
  storyboard_coverage_manifest,
  media_attachment_manifest_hash,
  scene_semantic_input_root,
  materialization_hash
}
```

`source` 是按 `kind` 严格判别且 `additionalProperties=false` 的联合，不存在 untyped `owner_or_evidence_ref`。`derived` 只能用于 Coverage/Continuity 等 Backend 机械投影，必须冻结 derivation contract 和完整来源集合，不能生成新业务事实。

`PacketLocalRefV1.source` 同样是严格判别联合；`semantic_kind=evidence` 必须使用 Evidence 分支，其余 kind 必须使用 OwnerNodeRef 分支。`packet_local_refs[]` 为 Packet 内每个可被模型引用的对象分配短而确定性的 local key；key 由 `packet_content_hash + semantic_kind + source` 的冻结合同派生。它只是本次输入的别名，Backend 可从完整 Manifest 唯一反解，不是 Owner logical ID，也不能跨 Packet 复用。附件继续使用独立 attachment key，不塞进该联合。

`direct_storyboard` 分支的 `stage_payload` 只包含完整 Packet 与完整 Execution Materialization；共用 Wire 的 `source_inputs`、`upstream_candidate_refs` 和 `owner_input_refs` 必须缺席。Resource 文件由 Harness 按 Manifest 注入临时只读目录，模型只能读列出的文件。任意 ref 未物化、重复物化、Projection Hash 不符或出现 Packet 外 Owner ref 都会使输入失败。

`scene_semantic_input_root` 的前像明确排除 `packet_content_hash`、StoryGraph Version/Head/Graph content hash、runtime path 和 Packet-local alias bytes；它先把所有 local keys 反解为 exact refs，再覆盖全部 Scene 业务 exact refs、资源内容 digest、Policy、Coverage 语义、媒体 exact refs/digest 和 derivation contract。这样无关 scope 造成的 Graph 版本变化不会伪装成本 Scene 变化，而任何本 Scene 内容、版本或字节变化都会改变 root。该 root 只用于 Backend 判定“Graph 因其他 scope 更新但本 Scene 语义完全未变”，不能替代 `packet_content_hash`、跨 Packet 搜索或 Agent input identity。

### 4.3 Storyboard coverage manifest

Backend 从 Packet 的 exact Owner closure 和 EffectivePolicy 机械编译：

```text
StoryboardCoverageManifestV1 {
  contract_id = "storyboard-coverage-manifest-v1",
  packet_content_hash,
  expected_beat_keys[],
  expected_dialogue_keys[],
  expected_action_evidence_keys[],
  expected_occurrence_keys[],
  expected_interaction_claim_keys[],
  coverage_requirements[],
  story_time_boundary,
  target_scene_duration_ms,
  allowed_duration_tolerance_ms,
  allowed_shot_count_range,
  aspect_ratio,
  effective_policy_snapshot_ref,
  manifest_hash
}
```

每条 `coverage_requirement` 使用有限枚举，例如 `must_visualize`、`must_hear`、`may_be_offscreen`、`must_preserve_order`、`must_show_state_transition`；它只能由已确认事实和 Policy 派生，不能把 Backend 的创作偏好冒充原文事实。Manifest 中的 keys 必须在 `packet_local_refs[]` 唯一解析为对应 semantic kind 和 exact ref。Evidence Slice 的文本、Dialogue/Beat/Claim 的有界投影和 story-time sequence key 都通过 resource refs 提供，绝不让模型按标题或数组到达顺序猜测。

所有 `resource_refs[]` 按 `resource_key`、`packet_local_refs[]` 按 `local_key`、Coverage keys 按其业务 sequence key 或合同指定 UTF-8 key 排序且唯一；Hash 使用项目 Canonical JSON 与 contract domain separator 并排除自身字段。`runtime_relative_path` 必须是无 `..`、无反斜线、无绝对前缀的 NFC 相对路径，Harness 拒绝符号链接、重复路径和 Manifest 外文件。Go/Python 必须共享相同 success/reject fixtures。

## 5. 视觉附件合同

### 5.1 附件覆盖

Backend media broker 根据 Packet 的 `artifact_provenance_refs` 和 Reference Binding 精确注入下列只读 rendition：

- Character identity anchor；
- 当前 Scene 实际 Character Appearance 的 front/profile/back 或 Plan 要求的 view roles；
- Location board；
- Scene 中实际使用的 Prop sheet；
- required 或已选 optional Scene Composition；
- Scene 内 required 或已选 optional Interaction Composition。

```text
StoryboardMediaAttachmentManifestV1 {
  contract_id = "storyboard-media-attachment-manifest-v1",
  packet_content_hash,
  entries[],
  target_coverage[],
  manifest_hash
}

StoryboardMediaAttachmentEntryV1 {
  attachment_key,
  purpose,
  view_role,
  owner_ref,
  asset_version_ref,
  reference_binding_ref?,
  artifact_ref,
  rendition_ref,
  mime_type,
  pixel_width,
  pixel_height,
  page_or_frame_count,
  byte_length,
  content_digest,
  rights_content_hash,
  lineage_content_hash,
  sort_key
}
```

`target_coverage[]` 对 Packet 中每个 active Target 明确记录 required/optional/not_generated、selected 状态、要求的 view roles 和对应 attachment keys。每个附件必须带精确 Owner/AssetVersion/Binding/Artifact/Rendition refs；Binding 只在该 Target 存在正式 Binding 时出现。`attachment_key` 由上述身份的 Canonical JSON 根确定性派生，不使用文件名或 URL。Entries 按 `(purpose, sort_key, content_digest)` 排序且 key 唯一，Coverage 按 Target business key 排序；Hash 规则与 Packet Materialization 一致。

### 5.2 fail-closed 规则

- required Target 的所需 view role 缺失：Packet 不可 dispatch；
- optional 未选择或 `not_generated`：Manifest 显式无附件，不视为缺失；
- 同一 Attachment key 对应不同 digest、Artifact 非 READY、Rights/Lineage 不等价：拒绝；
- 媒体总量超限：返回预算失败，不自动抽样；
- Agent 输出只能引用输入 Manifest 中的 attachment key，不能输出 URL、路径、base64 或新 Artifact ref；每 Shot 的 used keys 只需覆盖该 Shot 实际可见/表现对象所需的视觉依据，不要求重复列出 Scene 输入的全部三视图；
- 附件 bytes 不进入 JSON，但完整 Manifest/digest 进入 `input_hash`，Harness 在执行前重算。

三视图是角色基础参考的结构化 view-role coverage，不要求所有 Provider 把三视图拼成一张图。若所选 Artifact 是组合板，也必须由已发布 Rendition 元数据证明 front/profile/back 区域与 digest，不能靠文件名猜测。

## 6. `direct_storyboard` Agent 输出

### 6.1 模型只返回 Proposal Body

模型输出合同为 `storyboard-proposal-body-v2`。Attempt Result、Candidate Revision、Hash、Owner ref 和正式 ID 均由 Harness/Backend 包装：

`3003` 的 Stage Release row 必须分别冻结 `output_contract=storyboard-proposal-body-v2`、`normalized_candidate_contract=storyboard-candidate-v2` 与 `normalizer_contract=storyboard-normalizer-v2`；三者不是同一个 Schema，也不得把模型 Body 直接登记为 normalized Candidate。

```text
StoryboardProposalBodyV2 {
  proposal_contract_id = "storyboard-proposal-body-v2",
  packet_content_hash,
  scene_scope_key,
  proposed_shots[],
  scene_continuity_summary
}

ProposedShotV2 {
  proposed_shot_key,
  sequence_index,
  intent,
  detail,
  visible_occurrence_keys[],
  depicted_interaction_claim_keys[],
  used_attachment_keys[]
}
```

`proposed_shot_key` 只允许 `shot_1`–`shot_9999`，且 `sequence_index` 从 1 连续递增。它是一次 Proposal 内的临时关联键，不是 Shot logical ID。数组顺序仍以显式 sequence index 为准，不能以 JSON 到达顺序或 key 字典序替代。

### 6.2 Intent 层

每个 `intent` 至少包含：

- `purpose_code` 与简短导演意图；
- 精确 Scene/Beat/Dialogue/Evidence/Claim refs；
- 建议时长毫秒；
- action、dialogue delivery、sound、performance；
- 叙事信息、情绪转折与观众注意点；
- continuity in/out 与上一/下一 Shot 的意图关系；
- offscreen/on-screen 的显式处理。

Intent 使用 `packet_local_refs[]` 中的 scene/beat/dialogue/evidence/claim local keys；它不能回显或改写 OwnerRef/EvidenceRef。Backend normalizer 才把 key 恢复为 Packet 内 exact refs。模型不计算 Proposal Hash；Harness 对通过 Schema 的 Body 计算 `proposal_body_content_hash` 并写入 Attempt Result。Intent 不能增加 Scene 外角色、地点、道具、事件或对白。

### 6.3 Detail 层

每个 `detail` 至少包含：

- shot size、camera angle/height、lens family、focus/depth；
- camera movement 的起止、速度语义与稳定方式；
- composition、blocking、screen direction、eyeline 与轴线策略；
- first/key/last frame intent；
- lighting/color 的“如何应用当前 EffectiveStyle”说明，而不是新建画风；
- Character 表演、Prop 接触与 Location 空间关系；
- 声画进入/退出点与相邻镜头转接；
- 可枚举 risk codes。

Detail 不包含 Provider prompt、生成参数、AssetVersion 选择、自由 URL、正式 timecode 或正式 Shot ID。正式 timecode 由 Owner Apply 根据已批准 duration 和 sequence 机械计算。

### 6.4 Agent 允许与禁止的选择

Agent 可以决定：

- 一个 Beat 如何拆成若干镜头；
- 对白在画内或画外呈现；
- Packet local keys 指向的哪些实际 Occurrence 在某 Shot 可见；
- Packet local key 指向的哪个 InteractionClaim 在某 Shot 被表现；
- 使用哪一个已注入的 view attachment 帮助描述构图；
- 镜头语言、表演、调度与声音设计。

Agent 不可以决定：

- 某 Occurrence 对应哪个 Identity/State/AssetVersion；
- 使用哪一个 Scene/Interaction Binding 或替换 Artifact；
- required 资产是否可以跳过；
- 新增角色、地点、道具、状态、Interaction 或原文事件；
- 读取项目资产库、寻找“当前/最新”版本、触发媒体生成或发布任何资产；
- 返回 `needs_asset`、空 binding、占位 UUID 或降级为纯文字参考。

## 7. Backend normalizer 与精确绑定

### 7.1 normalized candidate

Harness 先验证 Proposal Schema，Backend 再使用冻结的 `storyboard-normalizer-v2` 产生：

```text
StoryboardCandidateV2 {
  candidate_contract_id = "storyboard-candidate-v2",
  packet_ref { packet_contract_id, packet_content_hash, scene_scope_key },
  stage_release_ref,
  execution_materialization_hash,
  temporary_shot_set_key,
  shots[],
  coverage_proof,
  duration_summary,
  continuity_summary,
  candidate_content_hash
}

StoryboardShotCandidateV2 {
  temporary_shot_key,
  sequence_index,
  intent,
  detail,
  production_binding_candidate,
  shot_content_hash
}

ShotProductionBindingCandidateV2 {
  scene_ref,
  occurrence_bindings[],
  interaction_bindings[],
  location_asset_version_ref,
  scene_reference_binding_ref,
  attachment_bindings[],
  binding_content_hash
}
```

`temporary_shot_set_key` 由 `stage_instance_key` 的冻结 key contract 派生；初始 `temporary_shot_key` 由 set key + `proposed_shot_key` 派生，sequence 不进入 key。它们只在该 Candidate lineage 内稳定。Normalizer 保存 Proposal key → temporary key 映射，模型不能伪造正式 Owner key。

Candidate、Shot 和 Binding 的 `*_content_hash` 都使用各自 contract domain separator，对排除自身 hash 字段的完整 Canonical JSON 计算。Normalizer 完成并创建不可变 Candidate Revision 后，Backend 才运行 deterministic gates；后发 Gate Issue 不进入 Candidate Hash。

### 7.2 绑定必须机械产生

Normalizer 先按完整 `packet_local_refs[]` 把 Proposal local keys 唯一恢复为 exact refs，再根据 `visible_occurrence_keys[]` 做唯一映射：

- Character Occurrence → exact AssetState → Packet 中该 Appearance 的 exact Character AssetVersion；
- Prop Occurrence → exact Prop State → exact Prop AssetVersion；
- Scene → exact Location AssetVersion；
- Scene Composition → Packet 中 exact SceneReferenceBindingVersion；只有 Plan 明确为未选 optional/`not_generated` 时才可为 `null`；
- depicted InteractionClaim → exact Character/Prop Occurrence、AssetVersion，以及按 Plan 必需/已选时非空、未选 optional/`not_generated` 时为 `null` 的 InteractionReferenceBindingVersion；
- attachment key → exact Artifact/Rendition/view role。

Agent Body 不携带这些 AssetVersion/Binding 选择，因此不存在模型把正确 Occurrence 绑定到另一人物形象或另一道具版本的合法路径。出现零个或多个匹配、Binding 与 Claim participant 不等价、State 错配或 view role 不满足时，Normalizer 失败，不创建 Candidate Revision。

`ShotProductionBindingCandidateV2` 只是正式 Binding 的候选前像。Gate 5 后 Storyboard Owner 必须从它逐字节复制业务引用并分配正式版本身份，不能在 Apply 时重新查询“最新资产”。

## 8. deterministic storyboard gates

Backend 对每个 normalized Candidate Revision 至少执行以下可重算 Gate，并另存严格结果：

```text
StoryboardDeterministicGateResultV1 {
  contract_id = "storyboard-deterministic-gate-result-v1",
  gate_contract_id,
  gate_contract_hash,
  candidate_revision_pointer,
  issues[],
  issue_root,
  result_hash
}
```

`result_hash` 覆盖除自身外的完整对象；Issue 指向精确 Candidate Revision 和 temporary shot/path，因此没有 Candidate ↔ Issue Hash 环。Gate Result 不修改 Candidate，也不拥有 Human 决议。

| Gate | 机械条件 | 失败结果 |
|---|---|---|
| Packet identity | Packet/Materialization/StageRelease/Input Hash 全等 | 技术失败，不进入 Review |
| Source coverage | expected Beat/Dialogue/Action Evidence 按 requirement 完整覆盖且不越界 | blocker |
| Scene isolation | 所有 refs/attachments 均属于 Packet，未出现 Scene 外事实 | blocker |
| Shot structure | 临时 key 唯一、序号连续、时长为正、shot count 合法 | blocker |
| Duration | 总时长与 target/tolerance 一致，Dialogue 不被不可能时长截断 | blocker |
| Occurrence | must_visualize 项至少出现一次；offscreen 处理符合 Policy | blocker |
| Interaction | required Interaction 至少由一 Shot 精确表现，participant/手别/状态一致 | blocker |
| Binding | 每个可见 occurrence 与 depicted interaction 得到唯一 exact binding | 技术失败，不创建 Candidate Revision |
| Attachment | Invocation Manifest 已完整覆盖 Target/view roles；Shot used keys 存在且与该 Shot 的 occurrence/interaction binding 相容 | blocker |
| Continuity | story-time boundary、状态 in/out、screen direction/axis 硬约束不冲突 | blocker |
| Style/Policy | 未引入新风格或违反禁用镜头语言、画幅和内容 Policy | blocker |
| Forbidden fields | 无 URL、Provider、current/latest、`needs_asset`、正式 ID | Schema 失败 |

Mechanical blocker 不能被模型 Reviewer 降级。Gate 输出排序 issue，不拥有 Human 决议。

## 9. Storyboard Review

`review_candidate/profile=storyboard` 是 text runtime，因为视觉附件已由 `direct_storyboard` 用于构图；Reviewer 评估的是冻结 Packet 投影、normalized Candidate 和 deterministic issues，不重新选图或做第二次 Vision Selection。

Reviewer 只能输出 `production_review_candidate_v2` 中 Evidence/Owner-ref scoped 的 ReviewIssue，覆盖：

1. 叙事清晰度与 Beat 转折；
2. 对白、动作和表演可拍性；
3. 镜头节奏、重复信息与时长合理性；
4. 轴线、视线、调度和场内/跨场连续性；
5. 已冻结视觉基底在镜头 Detail 中的应用；
6. Interaction 接触、Prop 状态和人物多形象是否被正确表现；
7. 首/关键/末帧意图是否足以进入后续 Shot 媒体生产。

Reviewer 不输出整体 pass/fail，不新增事实、不改资产、不把审美意见写成 deterministic blocker。无合法 Evidence/Owner ref、命中 Candidate 外 path 或引用已 stale Revision 的 Issue 被拒绝。

## 10. 有界 Repair

### 10.1 允许的 typed operation

`repair_candidate/profile=storyboard` 只针对一个冻结 Issue 和一个精确 Candidate Head 返回严格联合 Patch：

- `retime_shot`：调整一个或多个 duration，仍满足总预算；
- `replace_intent_field`：修改 purpose/action/dialogue delivery/sound/performance/continuity note；
- `replace_camera_detail`：修改景别、角度、焦段族、运动、构图、调度、首/关键/末帧；
- `associate_or_redistribute_packet_ref`：把 Coverage Manifest 中已存在的 Packet local key 关联到一个 Shot，或在现有 Shot 间重分配；不能创建 key、改 exact ref 或删除 required 总覆盖；
- `split_shot`：把一个 Shot 的既有 exact refs 确定性分配给两个 Shot；
- `merge_adjacent_shots`：只合并相邻 Shot，并保留两者 exact ref 并集；
- `move_adjacent_shot`：只在 issue allowlist 指定范围内调整相邻顺序；
- `replace_visibility_or_depiction`：只使用 Packet local key 指向的 exact Occurrence/Interaction ref，随后由 normalizer 重建绑定。

Patch 不使用任意 JSON Pointer。每个操作都有 typed target、expected fragment hash、Issue authorization root 和完整 replacement body；Backend 以 Candidate Head CAS 应用，产生下一 `StageCandidateRevision(origin=repair)`，重跑 normalizer、全部 deterministic gates 与 Review。

### 10.2 永远不可修的字段

Repair 不能：

- 修改 Packet/Materialization/StageRelease/Policy/Expected Coverage；
- 修改 Evidence 文本、range 或 hash，只能重新关联输入中原样 ref；
- 修改 Identity、Specification、AssetState、Occurrence 或 InteractionClaim 内容；
- 选择或替换 AssetVersion、ReferenceBinding、Artifact/Rendition；
- 添加 Packet 外事实、删除 required coverage 或把 on-screen 需求降为 offscreen；
- 修改已确认 Owner 事实或正式 StoryGraph；
- 生成媒体、改图或发布 CandidateSelection。

结构性 Patch 中，retime/field replace/visibility change/move 保留既有 temporary shot key；split child key 由 parent key + repair revision + child ordinal 派生，merge key 由有序 parent keys + repair revision 派生，并保存 old key → new key lineage。超过冻结轮次、无法在允许集内修复、fixed point 不收敛或仍有 blocker 时停在 `needs_review`，不自动放宽规则。

## 11. Gate 5 与 Storyboard Owner Apply

### 11.1 Human Review subject

Gate 5 的 Subject 是精确 `StoryboardCandidateRevision`，必须同时展示：

- Packet/Scene/Stage Release 身份；
- Intent 与 Detail；
- 来源覆盖、时长和连续性；
- 每 Shot 的 Occurrence/Interaction 与精确生产绑定；
- deterministic issues、ReviewIssues 和 Repair lineage；
- 上游 stale/impact 状态。

用户可以批准、拒绝或要求修改。要求修改有两条受限入口：选择当前 ReviewCandidate 中可修的精确 Issue；或在 UI 提交有限枚举的 typed change（目标 Shot、允许字段/操作、期望值和原因）。Backend 对后一种请求执行同一 path/ref/allowlist 校验，并用冻结 `storyboard-human-change-review-aggregate-v1` 创建 `human_typed_change` ReviewIssue Candidate Revision。该 Revision 的 producer 必须使用 `origin=aggregate`，精确绑定当前 Gate 5 NodeRun、单 issue ShardManifest、目标 Candidate/Decision/allowlist ordered input roots 和 aggregate contract/hash，不伪造 Agent StageVariant。Repair 输入仍必须携带该 review revision/hash；自由文本不能直接变成 Patch 权限。不适合 typed Repair 的方向性变化改为新创作候选。

若用户要求完全不同的镜头方案，则以新的冻结 sampling seed 创建同 Packet 的新 `direct_storyboard` Invocation/Candidate slot。它是新的创作候选，不是技术重试，也不覆盖旧 Candidate。可选候选数和模型预算由 ExecutionPolicy 冻结，不引入计费产品门。不同 candidate slot 各自有 Candidate Head，Gate 5 只能批准其中一个精确 Revision；其他候选保持可审计但不能随批准候选一起 Apply。

### 11.2 原子 Apply

批准后 Storyboard Owner 在一个事务中：

1. 锁定 Candidate Head、ReviewDecision subject 与待写 formal storyboard baseline；
2. 重验 Skill Release Control、Packet、scene semantic input、Owner refs、Artifact readiness 和 Binding；
3. 为 temporary shot set/shot keys 分配正式 Owner logical/version identity；
4. 创建完整 Scene formal Shot set；
5. 为每个 Shot 创建且仅创建一个 `ShotProductionBindingVersion`；
6. 保存 temporary → formal mapping、Candidate/Packet/Decision/Release provenance 与 Command Receipt；
7. 在同一事务的 Outbox 中保存 committed event；
8. 触发 Compiler 从最新一致 Owner Collections 发布 v2 p3 StoryGraph。

整个 Scene Apply 只有一个 Command Receipt；每个 Shot 恰有一个生产绑定版本。任一 Shot、Binding、映射、Outbox 或 Receipt 写入失败时整 Scene 回滚，不允许部分镜头成为正式事实。重复同 idempotency key 只返回同一 Receipt；不同命令争用同一 baseline 必须整体冲突。Compiler 在 Owner 事务提交后运行；编译失败使 Graph 保持旧 Head 并重试/告警，不回滚或删除已提交的 Storyboard Owner 事实。

`ShotImageBindingVersion`、`ShotVideoBindingVersion` 或后续输出不能代替 `ShotProductionBindingVersion`。前者绑定生成结果，后者冻结生成前的业务输入。

### 11.3 与 Graph Head 并发

Packet 绑定一个精确含当前 Scene p2 coverage 的 v2 Graph，但其他 Scene 的无关 Owner 更新或 p3 发布可能先产生新 Graph Head。Apply 不应仅因全局 Head ID 变化就迫使用户重审：

- Backend 必须在事务内从当前 v2 Graph（该 Scene 仍具 p2 coverage）重算该 Scene 的 `scene_semantic_input_root`；
- 若该 root、全部 Packet refs、Policy、Coverage、媒体 digest 和 Binding 逐字节未变，可记录 `gate5_scene_revalidation_proof` 并把已批准 Candidate 应用到当前 Head；
- 若任一当前 Scene 输入变化，则返回 `storyboard_input_stale`，禁止套用旧 Decision；
- Revalidation 只消除无关 scope 的 Graph 版本漂移，不允许替换 Packet 内任何引用。

具体 Proof Record、锁序与 Compiler CAS 由 `VP-D09` 固定。

## 12. 局部失效与重算

| 上游变化 | 必须 stale | 不应 stale |
|---|---|---|
| Scene/Beat/Dialogue/Evidence | 当前 Scene Packet、Candidate、Decision | 无关 Scene |
| Identity merge/split | 受影响 Occurrence、参考资产、组合 Binding、Packet、Candidate | 无关 Identity/Scene |
| Character/Location/Prop State | 引用该 State 的参考、Binding、Packet、Candidate | 未引用 State |
| Interaction/Continuity | 对应组合 Binding、相关 Scene Packet/Candidate | 无关 Interaction |
| Preset/Style/Policy | 受影响 Brief、Asset/Binding、Packet/Candidate | P0 SceneFact/Identity |
| AssetVersion/CandidateSelection | 引用该版本的 Binding、Packet/Candidate | 未引用 AssetVersion |
| Scene/Interaction Composition | 对应 Packet/Candidate | 基础角色/地点/道具资产 |
| 其他 Scene 的 Owner/Graph 更新 | 当前 Scene 做 apply-time revalidation | 语义根完全不变的 Candidate/Decision |
| 只修改 Candidate 镜头字段 | 当前 Review/Repair 下游 | Packet 与上游视觉资产 |

Stale Candidate/Decision 永久保留审计，不改 hash、不覆盖。新 Packet 产生新的 Stage instance/input hash；不得把旧 Agent Result 包装成新 Packet 的 Candidate。只有上节严格的 apply-time Graph revalidation 可复用已批准语义，不能跨业务输入复用。

## 13. Invocation、幂等与恢复

`direct_storyboard` 完全继承 `3003` 的 AgentInvocation/Attempt/Outcome、StageInstanceKey、Skill Release Control 和 DispatchAuthorization：

- Stage instance identity 覆盖 Variant、Scene scope、Packet/Materialization/Attachment Manifest、ExecutionPolicy 和 input hash；
- 技术重试只在同 Invocation 的持久 attempt budget 内创建新 Attempt；
- 同一 input 的新创作候选必须使用用户触发的新 frozen sampling seed，因此是新 input hash，不冒充重试；
- 首个满足 claim/release fence、Schema、Hash 和 current Control 的成功 Result 才能成为唯一 Outcome；
- unknown Attempt 先对账，不能生成空 Candidate 或换 seed 重跑；
- late Result、旧 Claim、旧 Candidate Head、quarantined/revoked Release 在 Outcome、Candidate Revision 和 Apply 三处均被 fence；
- Repair 使用 expected Candidate Head Hash CAS；重复 Patch 返回同一 Receipt，不同 Patch 争用同一 Head 时冲突；
- Workflow/worker 重启不重置 model calls、repair rounds、deadline 或 alternative candidate budget。

Agent 进程不保存跨请求 Checkpoint。所有 Packet、Materialization、Attachment Manifest、Invocation、Attempt、Candidate Revision、Review、Decision 和 Owner Receipt 都由 Backend 持久化。

## 14. Runtime 与安全边界

`direct_storyboard` 使用隔离 vision runtime：临时工作目录、只读注入、忽略用户配置、严格 output schema，禁用 Shell、Web、Browser、Plugin、Skill Search、任意文件浏览、数据库、对象存储凭据和业务 API。Codex 网络只用于受控模型调用，不是 Tool 网络。

Runtime 只能加载 `build-storygraph` 当前 Stage Release 声明的：

- 核心 `SKILL.md`；
- storyboard intent/detail recipe；
- continuity/camera/story coverage rubric；
- 当前 Packet resource manifest；
- 当前媒体附件；
- output schema。

不加载其他 Stage Reference、外部 Skill 原文、failure fixture 或项目文件。剧本、用户参考文本和图片元数据均是不可信数据；其中指令不能提升为系统 Guidance。日志只保存 refs/hash、Stage/Attempt、资源计数、token/耗时和安全摘要，不记录原文、图片 bytes、Prompt 或凭据。

## 15. 失败语义

| code | 层级 | 处理 |
|---|---|---|
| `scene_not_production_ready` | Packet build | Gate 4/required coverage 未闭合，不创建 Invocation |
| `scene_packet_contract_invalid` | Packet build/Invocation | Packet refs、Graph、Owner closure 或 hash 不等价，拒绝 |
| `scene_packet_materialization_invalid` | Invocation | Resource set 不完整、越界或 digest 不符，拒绝 |
| `storyboard_media_coverage_incomplete` | Invocation | required Target/view role 无合法附件，拒绝 |
| `storyboard_packet_budget_exceeded` | Invocation | Scene/媒体超当前 runtime 上限，保持 blocked，不截断 |
| `storyboard_proposal_schema_invalid` | Attempt | 有限结构纠正后仍失败，确定失败 |
| `storyboard_unknown_reference` | Normalizer | Proposal 使用 Packet 外 ref/attachment，拒绝 Candidate |
| `storyboard_binding_ambiguous` | Normalizer | Occurrence/Interaction 无唯一 exact binding，拒绝 Candidate |
| `storyboard_source_coverage_invalid` | Candidate | 缺 required source 或越界，blocker |
| `storyboard_duration_invalid` | Candidate | Shot/Scene 时长违反冻结 Policy，blocker |
| `storyboard_continuity_invalid` | Candidate | 状态、Interaction、轴线硬约束冲突，blocker |
| `storyboard_repair_unauthorized` | Repair | operation/path/ref 超 Issue allowlist，拒绝 |
| `storyboard_non_convergent` | Repair | 轮次耗尽仍变化或 blocker，进入人工处理 |
| `storyboard_input_stale` | Gate 5 Apply | 当前 Scene semantic root 变化，旧 Decision 不可用 |
| `storyboard_apply_conflict` | Gate 5 Apply | baseline/Head/Receipt 争用，整体回滚 |
| `skill_release_control_blocked` | 全链 | Release fence 变化，拒绝 Outcome/Candidate/Apply 且不 fallback |
| `attempt_runtime_unknown` | Attempt | 保持 reconciling，先对账同一执行 |

`needs_asset` 不是失败码、Candidate 状态或恢复分支；在 v2 Storyboard Schema 中出现该字段直接属于 Schema invalid。

## 16. 可观测与质量评测

### 16.1 业务指标

- production-ready Scene 中 Packet 构建成功率；
- `direct_storyboard` 首次 Schema/Normalizer 成功率；
- Source/Dialogue/Interaction coverage blocker 数；
- Storyboard 首次 Human approve rate；
- 每 Scene Repair 轮次、候选替代次数和人工退回原因；
- 每 Shot 正式 Binding 完整率；
- stale Candidate 被 Apply 的数量，目标必须为 0；
- 角色形象错绑、Prop 状态错绑、Interaction 组合错绑数量，目标必须为 0。

### 16.2 Release eval fixtures

`direct_storyboard/default`、`review_candidate/storyboard` 与 `repair_candidate/storyboard` 必须进入 `3003` 的同一 CandidateStageSet Eval/Shadow：

1. 真实剧本的至少两个相邻 Scene，包含两名角色、多 Beat/Dialogue、一个人物跨 Scene 的不同形象状态；
2. 一个 Location、一个有前后状态的 Prop、明确手别/握点 Interaction；
3. identity anchor、appearance 三视图、Location/Prop board、Scene/Interaction Composition 的真实只读附件；
4. faithful 与 world_adaptation 两种 Preset 各一套 Packet，P0 事实相同而镜头视觉应用不同；
5. 故意缺 attachment、错 State、错 Interaction participant、stale Binding 和 prompt injection 的拒绝样本；
6. split/merge/retime/continuity Repair 的成功与越权样本；
7. 不给 Reviewer 预期答案的独立 forward test，以实际 coverage、binding、continuity 和可拍性产物评分。

评测不能只搜索输出关键词；必须由 deterministic validator 重算 Packet/Binding/Coverage，再由盲审 rubric 评估镜头质量。新 Release 不自动替换生产 Control，必须满足 `3003` 的 Eval、Shadow、Signature 与 Control 合同。

## 17. 原子迁移

实现阶段必须按新总 Plan 形成以下可验证闭环；本文不提前分配实施编号：

1. 先建立 v2 Packet execution materialization、media manifest、proposal/candidate/normalizer 跨 Go/Python fixture 与拒绝测试；
2. 实现 `direct_storyboard/default` 和严格 vision transport，生产 Registry 暂不激活；
3. 实现 storyboard review/repair profile、deterministic gates 和 Candidate Head CAS；
4. 实现 Gate 5 Owner Apply、formal Shot/Binding 与 p3 Compiler 输入；
5. 在一次 cutover 中使新 Workflow 只创建 Packet-first `direct_storyboard`；
6. 删除新建路径上的 `draft_storyboard`、`detail_shots`、`approved_storyboard_intents`、`needs_asset` 与 Storyboard→Generation caller；
7. 旧 v1 非终态 Invocation 只用精确旧 Wire/Bundle/Runtime 完成或显式终止；v2 Worker 不领取，v1 引用归零后独立删除旧 Schema/Reference/fallback。

不得在过渡期让一个新 Scene 同时产生 Draft/Detail 与 direct Candidate，不得把 v2 字段塞入旧 optional payload，也不得先删旧 runtime 使历史 Invocation 无法恢复。

## 18. 验收边界

未来 Acceptance 至少必须证明：

1. Gate 4 未完成、required 资产/组合 Binding 缺失或 stale 时，系统不能创建 `direct_storyboard` Invocation；
2. Packet、Execution Materialization、Coverage 和 Attachment Manifest 可跨 Go/Python 重算 Canonical JSON/hash，乱序、漏项、越界和 digest 错误全部拒绝；
3. 一个 Scene 一次 `direct_storyboard` 同时形成完整 Intent/Detail，没有 Draft/Detail 中间资产门；
4. v2 Schema 中不存在 `needs_asset`、Provider、current/latest、URL、正式 Shot ID 或 Agent 自选 AssetVersion 字段；
5. Agent 只能输出 Packet local keys 与 Attachment keys，Backend normalizer 唯一恢复 Occurrence/Interaction exact refs，并映射精确 Appearance/Location/Prop AssetVersion 与 Scene/Interaction Binding；
6. 真实角色 identity anchor + 多 Appearance 三视图、Location、Prop、人物拿 Prop 的组合参考能够进入对应镜头且不串身份/状态；
7. Source/Dialogue/Action/Occurrence/Interaction、时长、轴线和 story-time boundary 的 deterministic gates 可重算；
8. ReviewIssue 有 Evidence/Owner ref，Repair 只能执行 typed allowlist，split/merge 后 ref 与 Binding 不丢失，越权 Patch 被拒绝；
9. Gate 5 一次事务创建完整 formal Shot set、每 Shot 恰一个 ShotProductionBindingVersion、整个 Scene Apply 恰一个 Command Receipt，失败不留部分事实；
10. 其他 Scene 的 Graph 更新只触发严格 scene revalidation；本 Scene 任一语义输入变化会拒绝旧 Decision；
11. technical retry、alternative candidate、Attempt unknown、late Result、Repair CAS、Release quarantine/revoke 和 Apply conflict 各有独立恢复证据；
12. 新 run 只有 Packet-first v2 路径，历史 v1 无 fallback 或伪装升级；
13. Agent 从未读取项目资产库、调用 Generation/Provider、写数据库、发布 Asset/Shot/Graph 或持久化本地 Checkpoint；
14. 至少两个相邻真实 Scene 从 Evidence、Identity/State、三视图/道具/组合参考到 formal Shot、Binding 和 v2 p3 Graph 可全链反查，并证明人物形象、Prop 前后状态和 story-time boundary 连续。

## 19. 完成边界与下一门

`VP-D08` 只接受 Storyboard Packet-first Stage、Candidate、Review/Repair、Gate 5 边界和迁移策略，不修改代码，也不宣称旧实现满足新目标。

通过评审与独立提交后只解锁 `VP-D09`：在 Backend 领域设计中固定五个 P0 Owner、Reference/Storyboard Record、Command、Receipt、typed Query、Packet Materialization、Gate 5 revalidation 和 Compiler 的真实持久化/事务边界。`VP-D15` 接受前仍不得开始新实现。
