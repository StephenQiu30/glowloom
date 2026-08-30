# StoryGraph 内容图与 DAG 创作画布设计

- 历史状态：已接受（`SG-D01`，2026-08-27）；通用媒体 Provider/Shot 视频关系曾于 2026-08-29 同步
- 视觉生产 Schema 同步：已接受（`VP-D05`，2026-08-30；三路独立反例评审通过）
- 当前已接受上位设计：[完整设计基线](0001-AI短剧制作平台完整设计基线.md) · [系统总体架构](0003-系统总体架构.md) · [领域语言与模块命名规范](0006-领域语言与模块命名规范.md) · [剧本视觉生产工作台与世界观预设设计](0011-剧本视觉生产工作台与世界观预设设计.md)
- 待后续同步的专项设计：[项目制作圣经生成执行框架设计](3001-项目制作圣经生成执行框架设计.md) · [StoryGraph 剧本解析 Harness 与内置 Skill 设计](3003-StoryGraph剧本解析Harness与内置Skill设计.md) · [后端领域模块功能设计](2002-后端领域模块功能设计.md)
- 当前门禁：旧 `SG-Dxx/SG-Ixx` 只保留历史事实；`VP-D05` 已接受并独立提交，当前只解锁 `VP-D06`，PRD、Requirement、Plan 与 Acceptance 等待 `VP-D13`–`VP-D15`

## 设计结论

StoryGraph 是已确认 Owner 事实之间的**下游、不可变、Schema 限定只读投影**，不是业务事实中心，也不是 Scene Fact、Identity Resolution、Production World、Reference Plan 或 Guided Studio 的前置输入。

当前产品顺序固定为：

```text
SceneSpanCandidate
→ SceneFactCandidate（style-blind）
→ IdentityResolutionCandidate
→ Gate 1 用户决议
→ project_episode_lifecycle_confirmed / production/project Owner Apply
→ gate_1_structure_identity / production/bible Owner Apply
→ StructureIdentitySetVersion
→ Production World Candidate
→ Gate 2 / bible + planning + asset Owner Apply
→ Confirmed Production World
→ Preset resolution + ReferencePlanCandidate
→ Gate 3 / preset + production/reference Owner Apply
→ EffectiveStyleSnapshot + ApprovedReferencePlanVersion
→ identity anchor AssetVersion
→ anchor-bound appearance variants + Location/Prop AssetVersion
→ Scene/Interaction Composition + Human Selection
→ SceneReferenceBindingVersion / InteractionReferenceBindingVersion
→ Gate 4
→ production/storygraph 编译精确 StoryGraphVersion
→ SceneProductionPacket
→ Storyboard / Gate 5
```

Candidate、ReviewDecision、CandidateSelection 都不进入正式 StoryGraph。各业务 Owner 先发布精确版本，`production/storygraph` 的 Backend Compiler 只把指定 `schema_id` 明确允许的 Owner 版本投影为 `StoryGraphVersion`，并维护单一线性 `StoryGraphHead`。Guided Studio 读取 Backend typed Query；Story Lens 才主要读取 StoryGraph。

上述主链标出的是“为 SceneProductionPacket 首次强制编译 Graph”的时点。p0/p1 可选发布早期只读投影供 Trace/Diff，但它们不阻断 Owner Apply、不成为 Guided Studio 上游，也不得冒充 production-ready Packet。

当前 Schema 目标是 `storygraph-v2`。既有 `storygraph-v1` 版本保持不可变、可查询、可 Diff，但不能承载 Reference Plan、Reference Binding 或 Interaction 的新约束。v2 不原地改写 v1，也不借兼容字段把新语义塞回旧版本。

## 问题与范围

既有 v1 已建立稳定 Node/Edge Key、精确 OwnerRef、不可变 JSONB Version、线性 Head、Canonical Hash、DAG 校验、Diff 与影响追踪，但它仍反映旧的 Bible-first、Storyboard-first 媒体链：

- Reference 依赖 Storyboard Intent，顺序与当前产品相反；
- 没有 `production/reference` Node/Edge；
- `InteractionClaim` 没有 typed predicate、Occurrence participant 和状态转换投影；
- `generation_target`、Shot 图片/视频 Binding 与多 Provider 广度被误当成当前完成门；
- Compiler 曾被要求从 Evidence 推断正式 Anchor，越过了 Owner Apply；
- Canvas/Graph Patch 与实现队列混入当前产品范围。

本 Design 只解决 StoryGraph v2 的内容 Schema、输入清单、编译边界、v1→v2 线性升级和 SceneProductionPacket 交接。不定义 Agent Stage/Wire、GORM Record/Command/Receipt、GenerationTarget、HumanTask 恢复合同或前端页面。

## 非目标

- 不把 StoryGraph 变成通用知识图谱、EAV、图数据库或第二业务 Writer；
- 不为 `InteractionClaim` 新建 Interaction Node、数据库表、人物—道具直连权威 Edge 或组合 AssetState；
- 不把 Candidate、HumanTask、ReviewDecision、CandidateSelection、Workflow Run、Provider Job/Call 或可变 ArtifactReadiness 投影进内容图；
- 不把 `VisualProductionPackageView`、`SceneProductionPacket`、Character Look、角色卡、地点卡或 ContinuityLedgerView 变成 Graph Node；
- 不在当前 MVP Schema 投影 GenerationTarget、Shot 图片/视频结果 Binding、视频、声音、渲染或多 Provider 配置；
- 不开放通用 Graph Patch、Canvas JSON overwrite、浏览器写库或 Agent 写库；
- 不把 Story Lens、Authoring Canvas 或多人协作作为 P0–P4 完成门。

## 四种图的正式边界

| 图或历史 | 回答的问题 | 唯一 Owner | 权威存储 | 是否可直接执行 |
|---|---|---|---|---|
| `StoryGraphVersion` + `StoryGraphHead` | 当前 Schema 已列入的确认事实如何形成内容依赖，以及 Project 当前发布版本指向 | `production/storygraph` | PostgreSQL/GORM 不可变快照与线性 Head | 否 |
| `AuthoringRevision.Graph` | 用户准备怎样编排生产节点和端口 | `authoring` | PostgreSQL/GORM 不可变 Revision | 否，需编译 |
| `WorkflowDefinitionVersion` | 一次运行的合法执行 DAG 是什么 | `workflow` | PostgreSQL/GORM 不可变 Definition | 是，由 Temporal 解释 |
| Temporal History | Run/Activity/Timer/Signal 实际如何推进 | Temporal | Temporal History | 已执行事实 |

四者不共享 Record、ID、Schema 或 Writer。StoryGraph 可以影响分析，但不直接执行；Workflow 可以执行，但不拥有故事或视觉生产事实。

## Schema 身份与演进

### 固定身份

| 项目 | v1 历史值 | v2 目标值 |
|---|---|---|
| `schema_id` | `storygraph-v1` | `storygraph-v2` |
| Node Key 派生 | `story-node-key-v1` | 保持 `story-node-key-v1` |
| Edge Key 派生 | `story-edge-key-v1` | 保持 `story-edge-key-v1` |
| Graph 发布 | 不可变 Version + 单 Project 线性 Head | 同一条线性 Head，不建 v2 分支 |
| Schema Manifest | 既有 v1 allowlist | 本 Design 的 v2 Node、Edge、Payload、Owner Input Manifest |

`schema_manifest_hash` 必须对以下唯一根对象的 Canonical JSON 计算 SHA-256：

```text
SchemaManifestHashRoot {
  manifest_contract_id: "storygraph-schema-manifest-v1",
  schema_id: "storygraph-v2",
  canonical_json_contract_id: "storygraph-canonical-json-v2",
  node_key_derivation_id: "story-node-key-v1",
  edge_key_derivation_id: "story-edge-key-v1",
  node_definitions[]: NodeDefinition,
  payload_union_definitions[]: PayloadUnionDefinition,
  edge_alias_definitions[]: EdgeAliasDefinition,
  edge_matrix_definitions[]: EdgeMatrixDefinition,
  owner_collection_definitions[]: OwnerCollectionDefinition,
  coverage_rules[]: CoverageRuleDefinition,
  checkpoint_owner_family_definitions[]: CheckpointOwnerFamilyDefinition,
  exclusion_definitions[]: ExclusionDefinition
}
```

上述全部对象都必须 `additionalProperties=false`，所有列出字段 required，`null | T` 必须显式保留 `null`。Manifest 元模型固定为：

```text
NodeDefinition {
  node_type: non-empty NFC String,
  owner_kind: non-empty NFC String,
  allowed_version_families: non-empty NFC String[],
  payload_contract_ids: non-empty NFC String[],
  payload_discriminant_field: null | non-empty NFC String,
  evidence_policy_id: "none" | "source_evidence_root" | "evidence_required" |
                      "evidence_or_creator_decision"
}

PayloadUnionDefinition {
  node_type: non-empty NFC String,
  discriminant_field: null | non-empty NFC String,
  discriminant_value: null | non-empty NFC String,
  payload_contract_id: non-empty NFC String,
  payload_contract_hash: lowercase SHA-256
}

EdgeAliasDefinition {
  alias: non-empty NFC String,
  expanded_node_types: non-empty NFC String[]
}

EdgeMatrixDefinition {
  matrix_id: "edge-type-matrix-v1" | "claim-cardinality-matrix-v1" |
             "materialization-matrix-v1" | "reference-planning-matrix-v1" |
             "reference-target-input-compatibility-matrix-v1" |
             "reference-target-result-matrix-v1" |
             "reference-target-activation-matrix-v1" |
             "reference-binding-matrix-v1",
  row_index: integer in [1, 9007199254740991],
  row_payload: EdgeTypeMatrixRow | ClaimCardinalityMatrixRow |
               MaterializationMatrixRow | ReferencePlanningMatrixRow |
               ReferenceTargetInputCompatibilityMatrixRow |
               ReferenceTargetResultMatrixRow |
               ReferenceTargetActivationMatrixRow | ReferenceBindingMatrixRow,
  projection_rule_id: non-empty NFC String,
  cardinality_rule_id: non-empty NFC String
}

EdgeTypeMatrixRow {
  edge_type,
  allowed_source_to_target,
  qualifier_and_cardinality
}
ClaimCardinalityMatrixRow {
  claim_branch,
  participant_cardinality,
  anchor_cardinality,
  state_cardinality
}
MaterializationMatrixRow {
  source,
  target,
  binding_role,
  target_cardinality
}
ReferencePlanningMatrixRow {
  source,
  reference_role,
  target_payload_field
}
ReferenceTargetInputCompatibilityMatrixRow {
  target_kind,
  target_uniqueness_key,
  expected_target_coverage,
  identity_refs,
  specification_refs,
  state_refs,
  style_refs,
  scene_refs,
  occurrence_refs,
  interaction_refs,
  target_dependencies
}
ReferenceTargetResultMatrixRow {
  target_kind,
  unique_allowed_result
}
ReferenceTargetActivationMatrixRow {
  target_activation_condition,
  required_cardinality,
  optional_cardinality,
  not_generated_cardinality
}
ReferenceBindingMatrixRow {
  target_binding,
  source,
  reference_role,
  target_cardinality
}

OwnerCollectionDefinition {
  minimum_coverage: "p0" | "p1" | "p2" | "p3",
  phase_rank: integer in [0, 3],
  owner_kind: non-empty NFC String,
  version_family: non-empty NFC String,
  scope_kind: "project" | "episode" | "scene" | "reference_plan",
  scope_key_and_collection_cardinality: non-empty NFC String,
  member_completeness: non-empty NFC String,
  scope_key_rule_id: non-empty NFC String,
  member_contract_id: non-empty NFC String,
  cardinality_rule_id: non-empty NFC String,
  empty_collection_policy: "forbidden" | "receipt_bound" |
                           "scope_rebase_conditional"
}

CoverageRuleDefinition {
  coverage_phase: "p0" | "p1" | "p2" | "p3",
  phase_rank: integer in [0, 3],
  scope_set_field: "p0_scope_keys" | "p1_scope_keys" |
                   "p2_scope_keys" | "p3_scope_keys",
  activation_condition: "scope_set_non_empty",
  prerequisite_phases: ("p0" | "p1" | "p2" | "p3")[],
  required_version_families: non-empty NFC String[],
  decision_checkpoint_ids: non-empty NFC String[],
  coverage_rule_id: non-empty NFC String
}

CheckpointOwnerFamilyDefinition {
  decision_checkpoint_id: non-empty NFC String,
  owner_kind: non-empty NFC String,
  version_family: non-empty NFC String
}

ExclusionDefinition {
  exclusion_kind: "node_type" | "compiler_input_kind" | "runtime_kind" |
                  "query_view_kind",
  literal: non-empty NFC String
}
```

Manifest 数组的排序/唯一键固定为：`node_definitions` 按 `node_type`；`payload_union_definitions` 按 `(node_type, discriminant_field ?? "", discriminant_value ?? "", payload_contract_id)`；`edge_alias_definitions` 按 `alias`；`edge_matrix_definitions` 按 `(matrix_id, row_index)`；`owner_collection_definitions` 按 `(phase_rank, owner_kind, version_family, scope_kind)`；`coverage_rules` 按 `phase_rank`；`checkpoint_owner_family_definitions` 按 `(decision_checkpoint_id, owner_kind, version_family)`；`exclusion_definitions` 按 `(exclusion_kind, literal)`。每个排序键在所属数组中唯一。所有嵌套 String 数组按 UTF-8 字节字典序排序去重。

八种 `*MatrixRow` 均为 `additionalProperties=false`，列出的字段全部是 non-empty NFC String；`row_payload` 的实际类型必须与 `matrix_id` 一一对应，不得跨表复用或填充伪字段。字段值逐格取对应 Markdown 表格 Cell 的 GFM inline plain-text：移除 code/emphasis 标记，解析实体与转义，首尾 ASCII whitespace 去除，内部连续 ASCII whitespace/newline 折叠为一个 U+0020，再做 NFC；其余 Unicode 与标点逐字保留。OwnerCollectionDefinition 的 `scope_key_and_collection_cardinality/member_completeness` 分别对应 Owner Collection 表的同名列，CheckpointOwnerFamilyDefinition 的三个字段对应 Checkpoint Owner/Family 表的同名列，它们均使用同一 Cell 规范化。这样 Claim 一行可原样承载 Participant/Anchor/State 三列，Reference activation 也不需要伪造 endpoint/qualifier。

`EdgeMatrixDefinition.row_index` 是对应本 Design 八张矩阵的 1-based 正文行号（不计表头）；Alias 必须先用 `EdgeAliasDefinition` 展开为排序去重 Node Type 数组，不对展开结果另行编号。`projection_rule_id/cardinality_rule_id` 分别固定为 `storygraph-v2/<matrix_id>/row-<row_index>-projection-v1` 和 `storygraph-v2/<matrix_id>/row-<row_index>-cardinality-v1`；`scope_key_rule_id/member_contract_id/cardinality_rule_id` 分别固定为 `storygraph-v2/owner-collection/<version_family>-scope-key-v1`、`storygraph-v2/owner-collection/<version_family>-members-v1` 和 `storygraph-v2/owner-collection/<version_family>-cardinality-v1`；`coverage_rule_id` 固定为 `storygraph-v2/coverage/<coverage_phase>-v1`。所有 `<row_index>` 使用无前导 0 的十进制字符串。

本 Design 的 Node/Payload/Edge/Collection/Coverage/Checkpoint Owner/Family/排除表就是上述数组的人类可审阅来源。Node Allowlist 的合并单元格必须先按 `|`、`、` 展开，每个字面 Node Type 恰生成 1 个 `NodeDefinition`；Payload Union 按展开后的 `node_type + discriminant` 恰生成 1 个 Definition。八张 Edge 矩阵、Owner Collection、Coverage、Checkpoint Owner/Family 和 Exclusion 的每一正文行分别恰生成 1 个对应 Definition。`VP-D14` 必须逐项机械化为跨 Go/Python fixture 并冻结实际 hash；fixture/hash 未建立前不得实现或发布 v2。实现不得手填一个与清单无关的常量冒充 Schema Hash。

既有 Key 派生合同不变：

```text
story_node_key = "sgn_" + SHA-256(Canonical JSON({
  schema: "story-node-key-v1",
  node_type,
  owner_kind,
  owner_logical_id,
  fragment_key
}))

edge_key = "sge_" + SHA-256(Canonical JSON({
  schema: "story-edge-key-v1",
  edge_type,
  from_node_key,
  to_node_key,
  qualifier
}))
```

`workspace_id/project_id/owner_version_id/revision/hash` 不进入 Node Key，因为 Graph 已按 Project 隔离，Key 表达逻辑身份；它们必须进入 Node Content Hash 表达精确版本。

### 单 Head 升级

1. v1 Version 永久只读，旧 ID/Hash/查询保持有效；
2. 第一个 v2 Version 从验证过的 Coverage Proof 与精确 Owner Collections 编译，`parent_version_id` 指向当时线性 Head，即使父版本是 v1；
3. CAS 成功后 Head 指向 v2，不复制或改写 v1；
4. 未变 Node Type 沿用 `story-node-key-v1`，相同逻辑 Owner/fragment 的 `story_node_key` 跨 Schema 保持稳定；
5. Cross-schema Diff 将 v2 新 Node/Edge 标为 added，将 v2 排除的旧媒体 Node/Edge 标为 removed，将同 Key 内容变化标为 changed；
6. `schema_rank` 固定为 `storygraph-v1=1 < storygraph-v2=2`；Head 必须保存 `schema_id + schema_rank`，CAS 只允许 `v1→v1`（cutover 前）、恰一次 `v1→v2` 和之后的 `v2→v2`；
7. v2 cutover 成功后，任何 legacy v1 Compile/Publish 即使持有旧 expected Head 也必须被 CAS 以 `schema_downgrade` 拒绝；v1 只保留 Read/Diff/Trace；
8. v2 失败不影响当前 Head；不得建立第二个 v2 Head、v1 child 或兼容双写。

## `storygraph-v2` Schema-scoped Owner Input Manifest

Compiler 只接收经 Backend Owner Query 冻结且可证明完整的精确集合：

```text
CompileStoryGraphVersion(
  schema_id = storygraph-v2,
  verified_coverage_proof,
  exact_owner_collections[],
  expected_graph_head_revision,
  expected_graph_head_content_hash,
  idempotency_key
)
```

唯一 Owner 版本引用合同固定为：

```text
OwnerVersionRef {
  workspace_id: UUID,
  project_id: UUID,
  owner_kind: non-empty NFC String,
  version_family: non-empty NFC String,
  owner_logical_id: non-empty NFC String,
  owner_version_id: UUID,
  owner_revision: integer in [1, 9007199254740991],
  owner_content_hash: lowercase SHA-256
}

OwnerNodeRef extends OwnerVersionRef {
  fragment_key?: non-empty NFC String,
  fragment_content_hash?: lowercase SHA-256
}
```

两个 Ref 都是 `additionalProperties=false`。`workspace_id/project_id` 必须与 Graph scope 一致；`owner_content_hash` 专指 Owner 版本内容，不与投影 Node 的 `content_hash` 复用名义。`fragment_key/fragment_content_hash` 必须同时省略或同时存在；投影聚合片段时二者必须同时存在，不得用 Owner 版本 Hash 代替 fragment 内容身份。`owner_kind/version_family` 还必须是 Schema Manifest 当前 Collection 表的字面组合。

为防止调用方漏传一个 Scene、State、Target 或 Binding，单个 Ref 不得直接组成 Compiler 输入；所有 Ref 必须属于一个 Owner 出具的完整集合证明：

```text
OwnerCollectionRef {
  workspace_id: UUID,
  project_id: UUID,
  owner_kind: non-empty NFC String,
  version_family: non-empty NFC String,
  scope_kind: project | episode | scene | reference_plan,
  scope_key: non-empty NFC String,
  scope_revision: integer in [1, 9007199254740991],
  scope_content_hash: lowercase SHA-256,
  members[]: OwnerVersionRef,
  member_count: integer in [0, 9007199254740991],
  members_hash: lowercase SHA-256,
  collection_root_hash: lowercase SHA-256
}
```

`OwnerCollectionRef` 也是 `additionalProperties=false`；`member_count` 必须等于 `members[]` 实际长度。`members[]` 按 `(owner_kind, version_family, owner_logical_id, owner_version_id)` 升序，不可重复。`members_hash = SHA-256(Canonical JSON(members))`；`collection_root_hash` 对除自身外的整个 Collection 根对象计算。Compiler 必须在同一读事务中调用对应 Owner read adapter 重算 `scope_revision/scope_content_hash/member_count/members_hash/collection_root_hash`；这是 phantom-range 完整性证明，不是新业务 Owner Record。任一根不一致即返回 stale，不自动补读“最新”成员。

以下 `version_family` 是 Schema Manifest 字面量。当前非空 Scope Set 的高阶 Coverage 累计包含低阶全部 Collection；历史高水位 `coverage_phase` 不单独激活已因 Scope Rebase 全量退役的高阶 Collection：

| 最低 Coverage | `version_family` / Owner | `scope_kind` | `scope_key` 与 Collection 基数 | 空集策略 | 成员完整性 |
|---|---|---|---|---|---|
| `p0` | `script_source_set` / `production/script` | `project` | `project:<project_id>`；每 Project 恰 1 Collection | `forbidden` | 精确 1 个已接受 Source Revision + 1 个匹配 SourceSpanIndexVersion；后者作校验输入但不成为 Node |
| `p0` | `project_episode_set` / `production/project` | `project` | `project:<project_id>`；每 Project 恰 1 Collection | `forbidden` | 完整 active Episode 顺序，`1..n` Episode |
| `p0` | `bible_structure_identity_set` / `production/bible` | `project` | `project:<project_id>`；每 Project 恰 1 Collection | `forbidden` | 精确 1 个 Gate 1 `StructureIdentitySetVersion` 完整性根；只作 Proof，不成为 Node |
| `p0` | `bible_production_world_set` / `production/bible` | `project` | `project:<project_id>`；每 Project 恰 1 Collection | `forbidden` | 精确 1 个 Gate 2 `ProductionBibleVersion` 且内嵌精确 StructureIdentitySetVersion Ref；Evidence/Specification/Binding/Rule/跨集 Claim/Arc/Thread 各 `0..n` 且与根一致 |
| `p0` | `planning_scene_set` / `production/planning` | `episode` | `episode:<episode_owner_logical_id>`；每 active Episode 恰 1 Collection | `forbidden` | Scene `1..n`；Dialogue/Beat/Occurrence/Continuity/Causal Claim 各 `0..n` 且集合完整 |
| `p0` | `planning_structure_rebase_set` / `production/planning` | `project` | `project:<project_id>`；每 Project 恰 1 Collection | `scope_rebase_conditional` | 正常编译为 0；Scope Rebase 时恰 1 个已发布不可变 Structure Rebase/Supersession Version，只作 Proof，不成为 Node |
| `p0` | `asset_identity_state_set` / `asset` | `project` | `project:<project_id>`；每 Project 恰 1 Collection | `forbidden` | 生产世界内 AssetIdentity `1..n`，每个身份 AssetState `1..n`；未发布候选为 `0` |
| `p1` | `preset_effective_set` / `preset` | `project` | `project:<project_id>`；每 Project 恰 1 Collection | `forbidden` | 精确 1 个 EffectivePolicySnapshot + 1 个 EffectiveStyleSnapshot |
| `p1` | `reference_plan_set` / `production/reference` | `reference_plan` | `reference-plan:<plan_owner_logical_id>`；p1 非空的 Project 恰 1 个 active Plan 且恰 1 Collection | `forbidden` | 精确 1 个 ApprovedReferencePlanVersion，Target `1..n`，与 Plan fragment 根完全一致 |
| `p1` | `asset_base_reference_set` / `asset` | `reference_plan` | 与 `reference_plan_set` 同 Scope；p1 恰 1 Collection | `receipt_bound` | 已履约基础 Target 的 AssetVersion 和每个版本精确 1 个 selected READY Artifact；基数见下文 phase-aware 表 |
| `p2` | `reference_binding_set` / `production/reference` | `scene` | 对每个 `p2_scope_key` 恰 1 Collection，`scope_key` 即该 Scene Scope Key | `receipt_bound` | 已履约组合 Target 的 Scene/Interaction Binding；基数见下文 phase-aware 表 |
| `p2` | `asset_composition_artifact_set` / `asset` | `scene` | 与 `reference_binding_set` 同 Scope；每 `p2_scope_key` 恰 1 Collection | `receipt_bound` | 每个正式 Scene/Interaction Binding 精确 1 个 selected READY Composition Artifact；不包含未选 Candidate Artifact |
| `p3` | `storyboard_formal_set` / `production/storyboard` | `scene` | 对每个 `p3_scope_key` 恰 1 Collection，`scope_key` 即该 Scene Scope Key | `forbidden` | 完整正式 Shot 集；每个 Shot 精确 1 个 ShotProductionBindingVersion，Draft/Intent Candidate 为 `0` |

Collection 唯一键固定为 `(workspace_id, project_id, owner_kind, version_family, scope_kind, scope_key)`，同一编译输入中不可重复。`scope_key` 必须按上表逐字节派生，Compiler 不按 Collection 成员反向猜测 Scope。上表第 4、5列分别机械生成 `cardinality_rule_id` 和 `empty_collection_policy`；`scope_key_rule_id` 固定为 `storygraph-v2/owner-collection/<version_family>-scope-key-v1`。

Coverage Rule 的累计 Family Set 固定为：

```text
p0_families = [
  asset_identity_state_set, bible_production_world_set,
  bible_structure_identity_set, planning_scene_set,
  planning_structure_rebase_set, project_episode_set, script_source_set
]
p1_families = p0_families + [
  asset_base_reference_set, preset_effective_set, reference_plan_set
]
p2_families = p1_families + [
  asset_composition_artifact_set, reference_binding_set
]
p3_families = p2_families + [storyboard_formal_set]
```

展开 `+` 后每组按 UTF-8 字节字典序排序去重存入 Manifest。`CoverageRuleDefinition` 的四行唯一来源是：

| `coverage_phase` | `phase_rank` | `scope_set_field` | `activation_condition` | `prerequisite_phases` | `required_version_families` | `decision_checkpoint_ids` |
|---|---:|---|---|---|---|---|
| `p0` | 0 | `p0_scope_keys` | `scope_set_non_empty` | `[]` | `p0_families` | `project_episode_lifecycle_confirmed, source_revision_accepted, gate_1_structure_identity, gate_2_bible_continuity` |
| `p1` | 1 | `p1_scope_keys` | `scope_set_non_empty` | `[p0]` | `p1_families` | `gate_3_visual_foundation_scope, gate_4_base_reference_selection` |
| `p2` | 2 | `p2_scope_keys` | `scope_set_non_empty` | `[p0, p1]` | `p2_families` | `gate_4_composition_selection` |
| `p3` | 3 | `p3_scope_keys` | `scope_set_non_empty` | `[p0, p1, p2]` | `p3_families` | `gate_5_storyboard` |

上表方括号/逗号列表展开为 String 数组并按 UTF-8 字节字典序排序去重；`coverage_rule_id` 按上文公式生成。每行只在其 `scope_set_field` 指向的当前 Scope Set 非空时激活；编译所需 Family/Checkpoint 是全部激活行的累计结果，而不是按历史 `coverage_phase` 直接选行。`p0_scope_keys[]` 始终非空；合法 Scope Rebase 使 `p1|p2|p3_scope_keys[]` 全空时，可保留高阶 `coverage_phase` 高水位，但不带入空 Scope 对应的 Plan/Binding/Storyboard Collection。

Receipt 的 Checkpoint 只允许绑定下表 Owner/Family；每个非条件 Collection 只在该行 Checkpoint 发布一次当前不可变 Root，后续 Gate 通过新 Family 表达新事实，不回填旧 Root：

| `decision_checkpoint_id` | `owner_kind` | `version_family` |
|---|---|---|
| `project_episode_lifecycle_confirmed` | `production/project` | `project_episode_set` |
| `source_revision_accepted` | `production/script` | `script_source_set` |
| `gate_1_structure_identity` | `production/bible` | `bible_structure_identity_set` |
| `gate_2_bible_continuity` | `production/bible` | `bible_production_world_set` |
| `gate_2_bible_continuity` | `production/planning` | `planning_scene_set` |
| `gate_2_bible_continuity` | `production/planning` | `planning_structure_rebase_set` |
| `gate_2_bible_continuity` | `asset` | `asset_identity_state_set` |
| `gate_3_visual_foundation_scope` | `preset` | `preset_effective_set` |
| `gate_3_visual_foundation_scope` | `production/reference` | `reference_plan_set` |
| `gate_4_base_reference_selection` | `asset` | `asset_base_reference_set` |
| `gate_4_composition_selection` | `production/reference` | `reference_binding_set` |
| `gate_4_composition_selection` | `asset` | `asset_composition_artifact_set` |
| `gate_5_storyboard` | `production/storyboard` | `storyboard_formal_set` |

表外组合以 `invalid_checkpoint_owner_family` 拒绝。`project_episode_lifecycle_confirmed` 在用户审核 EpisodeSpan/SceneSpan 边界后、`gate_1_structure_identity` Apply 前，由 `production/project` 独立发布 Episode 身份与顺序。它与 Gate 1 共用一次 Guided Studio 决议，但是 Backend 内部的独立 Owner Checkpoint，不是第六个用户 Gate，也不让 `production/project` 写 StructureIdentitySetVersion。因此上传完整多集剧本时可先由候选 EpisodeSpan 提议 Episode，再按用户确认的边界提交 Project Owner，无需在建项时预知集数。Gate 1 的 `bible_structure_identity_set` 和 Gate 2 的 `bible_production_world_set` 必须保持两个 Root；后者只通过内嵌精确 StructureIdentitySetVersion Ref 建立 ancestry，不能要求历史 Gate 1 Receipt 命中 Gate 2 后的新 Root。

`ExclusionDefinition` 的完整字面 Registry 固定为：

| `exclusion_kind` | `literal` |
|---|---|
| `node_type` | `project` |
| `node_type` | `interaction_claim` |
| `node_type` | `shot_continuity_claim` |
| `node_type` | `generation_target` |
| `node_type` | `shot_image_binding_version` |
| `node_type` | `shot_video_binding_version` |
| `compiler_input_kind` | `candidate` |
| `compiler_input_kind` | `human_task` |
| `compiler_input_kind` | `review_decision` |
| `compiler_input_kind` | `candidate_selection` |
| `compiler_input_kind` | `unselected_artifact` |
| `runtime_kind` | `authoring_revision_graph` |
| `runtime_kind` | `workflow_definition` |
| `runtime_kind` | `temporal_history` |
| `runtime_kind` | `provider_profile` |
| `runtime_kind` | `provider_job` |
| `runtime_kind` | `provider_call` |
| `runtime_kind` | `workflow_run` |
| `runtime_kind` | `workflow_invocation` |
| `query_view_kind` | `scene_production_packet` |
| `query_view_kind` | `continuity_ledger_view` |
| `query_view_kind` | `visual_production_package_view` |

上表是完整集，不从其他“非目标”文本自动添加字面；Node allowlist 仍对所有未知 Node Type 默认拒绝。

`verified_coverage_proof` 不是调用方可自由填写的字符串，而是 Backend 根据 Owner Apply Receipt 和 Collection Root 重算的内容定址证明：

```text
VerifiedCoverageProof {
  coverage_phase: p0 | p1 | p2 | p3,
  coverage_scope_sets: {
    p0_scope_keys[],
    p1_scope_keys[],
    p2_scope_keys[],
    p3_scope_keys[]
  },
  owner_apply_receipt_refs[]: {
    decision_checkpoint_id: "project_episode_lifecycle_confirmed" |
                            "source_revision_accepted" |
                            "gate_1_structure_identity" |
                            "gate_2_bible_continuity" |
                            "gate_3_visual_foundation_scope" |
                            "gate_4_base_reference_selection" |
                            "gate_4_composition_selection" |
                            "gate_5_storyboard",
    receipt_scope_key,
    covered_scope_keys[],
    owner_kind,
    version_family,
    committed_collection_root_hash,
    committed_owner_version_ref: OwnerVersionRef | null,
    receipt_content_hash
  },
  collection_root_hashes[],
  scope_rebase: null | {
    planning_structure_rebase_ref,
    from_graph_version_id,
    from_planning_collection_root_hash,
    to_planning_collection_root_hash,
    reason: source_revision_changed | scene_split | scene_merge | scene_deleted,
    retired_scope_keys[],
    replacement_scope_map[]: {from_scope_key, to_scope_keys[]},
    impact_closure_hash,
    downstream_invalidation_receipt_hash
  },
  coverage_scope_manifest_hash
}
```

Coverage Scope Key 统一使用 `scene:<scene_owner_logical_id>`；`receipt_scope_key` 是 `project:<project_id> | reference-plan:<plan_owner_logical_id> | scene:<scene_owner_logical_id>` 中一个字面引用。Project/Plan 级 Receipt 通过显式 `covered_scope_keys[]` 覆盖多个 Scene，不冒充 Scene Scope Key。Proof 内所有数组的排序和去重合同冻结为：

- `p0_scope_keys[]` 到 `p3_scope_keys[]` 分别按 Scope Key UTF-8 字节字典序升序且不可重复，并满足 `p3 ⊆ p2 ⊆ p1 ⊆ p0`；
- 每个 Receipt 的 `covered_scope_keys[]` 按 Scope Key UTF-8 字节字典序升序且不可重复；`owner_apply_receipt_refs[]` 按 `(decision_checkpoint_id, receipt_scope_key, owner_kind, version_family, committed_collection_root_hash, receipt_content_hash)` 的 UTF-8 字节元组升序，整个元组不可重复；
- `collection_root_hashes[]` 是小写 64 位十六进制 Hash，按 ASCII 升序且不可重复；
- `retired_scope_keys[]` 按 Scope Key UTF-8 字节字典序升序且不可重复；
- `replacement_scope_map[]` 按 `from_scope_key` UTF-8 字节字典序升序且每个 `from_scope_key` 只允许一项，其内 `to_scope_keys[]` 按同一规则排序且不可重复。

`p0_scope_keys[]` 必须为 `1..n`，其他 phase 允许为空；`project_episode_lifecycle_confirmed` 与 `source_revision_accepted` Receipt 的 `covered_scope_keys[]` 必须为空，因为确认 Project/Episode 生命周期或原稿本身都不越权声称后续 Scene Gate 已通过。其他 Receipt 的 `covered_scope_keys[]` 不得为空。

`coverage_scope_manifest_hash` 对 `{workspace_id, project_id, coverage_phase, coverage_scope_sets, owner_apply_receipt_refs, collection_root_hashes, scope_rebase}` 的 Canonical JSON 计算。Receipt 的 `owner_kind + version_family + committed_collection_root_hash` 必须精确命中相应 OwnerCollectionRef，`decision_checkpoint_id + receipt_scope_key + owner_kind + version_family + committed_collection_root_hash` 组合不得重复。`collection_root_hashes[]` 必须恰好等于 `exact_owner_collections[]` 全部 `collection_root_hash` 的去重集。

- `gate_1_structure_identity` 和 `gate_2_bible_continuity` Receipt 的 `covered_scope_keys[]` 并集必须分别精确等于 `p0_scope_keys[]`；`gate_3_visual_foundation_scope` 与 `gate_4_base_reference_selection` 的并集必须分别精确等于 `p1_scope_keys[]`；`gate_4_composition_selection` 和 `gate_5_storyboard` 的并集必须分别精确等于 `p2_scope_keys[]`、`p3_scope_keys[]`，无遗漏也无超出。Gate 4 拆成两个内部 Checkpoint 只为表达基础资产必须先于组合绑定发布，用户侧仍只显示一个 Reference Selection Gate；
- 除下述 Rebase 例外，每个 Coverage 所要求的 OwnerCollectionRef 都必须至少有一个同 `owner_kind + version_family + collection_root_hash` 的 Receipt，一个 Receipt 不得为其他 Owner/family/root 借用；
- 非空 Collection 的 `committed_owner_version_ref` 必须非空且为该 Collection member；合法空 Collection 的该字段必须为 `null`，Receipt 仍通过 `committed_collection_root_hash` 证明 Owner 已明确提交“空集合”，不得凭空伪造 SetVersion；该规则适用于全 `not_generated`/未选 optional 时的 `asset_base_reference_set`，以及无需组合结果时的 `reference_binding_set/asset_composition_artifact_set`；
- 唯一无 Receipt 空集例外是 `scope_rebase=null` 时的 `planning_structure_rebase_set`：它必须为空且不得有对应 Receipt。`scope_rebase!=null` 时该 Collection 必须恰 1 member 且恰有 1 个 `gate_2_bible_continuity` Receipt，其 committed Ref 精确等于 `planning_structure_rebase_ref`；Gate 1 仍只由 `production/bible` 发布 StructureIdentitySetVersion，Planning Rebase 不扩大 Gate 1 Writer；
- Compiler 按 Receipt、Collection 和基数反向推导每个 Scene 的最高 Coverage，仅在推导出的四组 Scope Set 与 Proof 声明完全相同时接受；
- `coverage_phase` 是线性 Head 的历史最高阶高水位，不是“全项目已到该阶段”的布尔值；首个 v2 版本取当前非空 Scope Set 的最高阶，后续取 `max(parent.coverage_phase, current highest non-empty scope phase)`；
- 同一 v2 线性 Head 只能保持或提高 `coverage_phase`；同一 Planning Structure Epoch 内，每个 phase 的 scope set 只能保持或扩大；合法删除/重切分见下文 Scope Rebase，它可以在最高阶 Scope 全被退役时保留高水位；
- 任何下游准入、Target 履约和 Packet 生成都只读精确 Scope Set，绝不只看 `coverage_phase`；
- 上游变更使高阶结果 stale 时，保留当前 Head 并重过 Gate，不发布低阶降级 Head。

`owner_set_hash` 的唯一前像固定为：

```text
OwnerSetHashRoot {
  schema_id: "storygraph-v2",
  schema_manifest_hash,
  verified_coverage_proof: complete canonical object,
  exact_owner_collections[]: complete OwnerCollectionRef objects
}
```

Collection 按 `(owner_kind, version_family, scope_kind, scope_key)` 排序，其内 members 按上文合同排序。重试必须保留完全相同的 `OwnerSetHashRoot`，不得只保留 hash 而丢失 Proof 或 Collection 前像。

### Scope Rebase

同一 Planning Structure Epoch 中 `scope_rebase` 必须为 `null`。只有新的已接受 Source/Planning 结构根真实删除、合并或拆分 Scene 时，才允许用非空 Rebase 替换 Scope Set，并必须同时满足：

1. `from_graph_version_id` 恰为当前 Head，前后 Planning Collection Root 均可重算；
2. `planning_structure_rebase_ref` 必须指向 `production/planning + planning_structure_rebase_set` 已发布的不可变 Owner Version；该 Owner payload 拥有 reason、前后结构根、retired Scope 和 replacement map，Compiler 必须验证 Proof 字节级等价，不得从两个 Root 自行猜测 split/merge 语义；
3. `retired_scope_keys` 精确等于旧集合中已不存在的 Scene，`replacement_scope_map` 精确表达 Owner 已发布的 split/merge 对应；
4. Impact Closure 必须包含所有依赖的 Plan Target、AssetVersion、Reference Binding、Packet、Shot 与搜索投影，且对应 Owner 已提交 invalidation/stale Receipt；
5. 新 Graph 不得保留指向 retired Scope 的 Node/Edge/Binding，Cross-schema/Version Diff 必须显示 removed/replaced；
6. `coverage_phase/schema_rank` 仍不降级；只替换当前真实覆盖 Scope，不伪造已通过的 Gate。

没有上述完整 Proof 的 Scope 缩减一律拒绝；正常新 Scene 只走 Scope 扩展。

所有 Coverage 均禁止 Candidate、ReviewDecision、CandidateSelection、Provider/Profile/Job/Call、Workflow/Invocation、SceneProductionPacket、ContinuityLedgerView 和未选 Artifact 进入 Collection。Owner Apply Receipt 只证明 Coverage，不投影为 Node。只有清单中当前 Schema 可见的 Collection Root 变化才使 Graph stale：`production/reference` 变化不会让 v1 stale，但会让已包含它的 v2 p1/p2/p3 stale。Compiler 发现 Owner Collection 或 Graph Head 漂移时返回 stale/conflict，由上层显式提交新编译请求。

## v2 Node Allowlist

StoryGraph Node 是正式 Owner 事实或其稳定聚合片段的投影，不新建第二业务 Record。

| Node Type | 唯一 `owner_kind` | 允许的 `version_family` | 投影事实 / Payload discriminant |
|---|---|---|---|
| `source_revision` | `production/script` | `script_source_set` | 精确 Document/Script Revision |
| `source_evidence` | `production/bible` | `bible_production_world_set` | 已确认 Evidence Fragment；不是 Candidate Evidence |
| `policy_snapshot` | `preset` | `preset_effective_set` | EffectivePolicySnapshot |
| `effective_style_snapshot` | `preset` | `preset_effective_set` | EffectiveStyleSnapshot |
| `asset_identity` | `asset` | `asset_identity_state_set` | `Asset(kind=character|location|prop)` |
| `character_specification` | `production/bible` | `bible_production_world_set` | CharacterSpecificationVersion |
| `location_specification` | `production/bible` | `bible_production_world_set` | LocationSpecificationVersion |
| `prop_specification` | `production/bible` | `bible_production_world_set` | PropSpecificationVersion |
| `asset_state` | `asset` | `asset_identity_state_set` | 完整 AssetState；Appearance Variant 只是产品名 |
| `production_binding` | `production/bible` | `bible_production_world_set` | Specification/State 与物化 Asset 的 ProductionBinding |
| `world_rule` | `production/bible` | `bible_production_world_set` | 有 Evidence 或明确创作者决定的 WorldRule |
| `story_arc`、`plot_thread` | `production/bible` | `bible_production_world_set` | 已确认全局叙事事实 |
| `relationship_claim`、`foreshadowing_claim`、`payoff_claim` | `production/bible` | `bible_production_world_set` | 带 participant/anchor/scope/evidence 的 Claim |
| `episode` | `production/project` | `project_episode_set` | 已确认 Episode |
| `scene`、`dialogue`、`narrative_beat` | `production/planning` | `planning_scene_set` | 已确认规划片段；Action 是 Beat/Dialogue payload，不新增 Node |
| `occurrence` | `production/planning` | `planning_scene_set` | 精确 AssetState 在 Scene/Beat 的一次出现 |
| `continuity_claim` | `production/planning` | `planning_scene_set` | `claim_type=continuity|interaction` 严格联合类型 |
| `causal_claim` | `production/planning` | `planning_scene_set` | 因果/Goal/Conflict/Turning Point 语义；产品词不新增 Node |
| `artifact` | `asset` | `asset_base_reference_set|asset_composition_artifact_set` | 只投影被正式 AssetVersion/Reference Binding 选择的 READY Artifact |
| `asset_version` | `asset` | `asset_base_reference_set` | `purpose=character_identity_anchor|character_appearance|location_board|prop_sheet` 的已发布基础版本 |
| `approved_reference_plan_version` | `production/reference` | `reference_plan_set` | Gate 3 已批准的完整计划版本 |
| `reference_plan_target` | `production/reference` | `reference_plan_set` | 计划内稳定 fragment；`target_kind=character_identity_anchor|character_appearance|location_board|prop_sheet|scene_composition|interaction_composition`、`fulfillment=required|optional|not_generated`、目标 OwnerNodeRef、排序去重的 `coverage_scope_keys[]` 和 Target 依赖 |
| `scene_reference_binding_version` | `production/reference` | `reference_binding_set` | Scene + 精确基础 AssetVersion + 已选 Composition Artifact |
| `interaction_reference_binding_version` | `production/reference` | `reference_binding_set` | InteractionClaim + Character/Prop AssetVersion + 已选 Composition Artifact |
| `shot` | `production/storyboard` | `storyboard_formal_set` | Gate 5 Owner Apply 后的正式 Shot |
| `shot_production_binding_version` | `production/storyboard` | `storyboard_formal_set` | Shot 的精确 Scene/Occurrence/Style/Asset/Reference 输入集合 |

`NodeDefinition.evidence_policy_id` 按下表完整 Registry 展开，每个 Node Type 必须恰命中一行：

| `evidence_policy_id` | Node Type 字面集 |
|---|---|
| `none` | `source_revision|policy_snapshot|effective_style_snapshot|production_binding|artifact|asset_version|approved_reference_plan_version|reference_plan_target|scene_reference_binding_version|interaction_reference_binding_version|shot|shot_production_binding_version` |
| `source_evidence_root` | `source_evidence` |
| `evidence_required` | `episode|scene|dialogue|narrative_beat` |
| `evidence_or_creator_decision` | `asset_identity|character_specification|location_specification|prop_specification|asset_state|occurrence|world_rule|story_arc|plot_thread|relationship_claim|foreshadowing_claim|payoff_claim|continuity_claim|causal_claim` |

表内 `|` 是 Node Type 分隔符，展开后按 UTF-8 字节字典序去重；四行并集必须精确等于 Node Allowlist，有遗漏或重复即拒绝 Schema Manifest。

`project` 只作为 Query scope/root，不是 v2 Node。Character Look、角色卡、地点卡、Scene Card、Action、Conflict、Goal、Turning Point 都是上述 Node 的 typed View 或 payload，不得绕过 allowlist 新增 Node。

v2 明确排除历史 `shot_continuity_claim`、`generation_target`、`shot_image_binding_version` 和 `shot_video_binding_version`。未来若需要把 Shot 媒体执行链重新纳入内容图，必须在 `VP-D10` 接受 GenerationTarget 后发布后续 Schema，不能修改 v2 语义。

## v2 Payload Contract Registry

所有 Payload Object 都必须 `additionalProperties=false`，必需字段不可省略，只有本节显式写为 nullable/optional 的字段才可为 `null` 或省略。OwnerNodeRef 数组统一按 `(owner_kind, version_family, owner_logical_id, fragment_key ?? "", owner_version_id)` 的 UTF-8 字节元组升序并去重；省略 `fragment_key` 时唯一使用空字符串作排序哨兵，已存在的 fragment 因合同要求非空而不会碰撞。Scope Key 和字符串 ID 数组按 UTF-8 字节字典序排序并去重。

全部 Payload 共用以下基底：

```text
ProjectionRefPayload {
  payload_contract_id,
  projection_hash
}
```

`projection_hash` 必须精确等于 OwnerNodeRef 的 `fragment_content_hash`；无 fragment 时等于 `owner_content_hash`。以下 Node 使用仅引用 Payload，完整业务字段继续由 Owner typed Query 提供，不在 Graph 复制第二份事实：

本节的 Schema 记法同时是 `VP-D14` 的机械转换合同：未标 `?` 的字段全部 required，`?` 表示 optional，`null | T` 表示 required nullable；未显式标型的单数 `*_ref` 是 `OwnerNodeRef`，`*_refs[]` 是按 OwnerNodeRef 合同键排序去重的数组，`*_key` / `*_id` 是非空 NFC JSON String。枚举只允许字面值，不允许未知值或自由扩展。以下共用类型也是 Manifest `property_schemas` 的字面来源：

```text
StableKey := non-empty NFC JSON String without U+0000..U+001F or U+007F
ScopeKey := StableKey matching /^scene:[^\u0000-\u001f\u007f]+$/
StoryTimeKey := StableKey whose Owner-defined byte order is chronological inside one Project
SequenceKey := StableKey whose Owner-defined byte order is the exact sibling order
DescriptorKey := StableKey matching /^[a-z][a-z0-9_.:-]{0,127}$/
ClaimScope {
  kind: "project" | "episode" | "scene" | "beat" | "source_range",
  owner_logical_id: StableKey
}
StoryTimeRange {start_key: StoryTimeKey, end_key: StoryTimeKey}
PositiveRational {
  numerator: integer in [1, 9007199254740991],
  denominator: integer in [1, 9007199254740991]
}
```

`StoryTimeRange.start_key <= end_key`；`PositiveRational` 必须约分到 `gcd(numerator, denominator)=1`，禁止小数和等价的多种表示。`StoryTimeKey` 只能由 Planning Owner 发布，Compiler 不从 Episode/Scene 标签拼接或重新编号。`SequenceKey` 也只能逐字节投影 Owner 已发布的 order key：Episode 由 `production/project`、Scene/Dialogue/Beat 由 `production/planning`、Reference Target 由 `production/reference`、Shot 由 `production/storyboard` 提供；同父级/同 scope 内必须唯一且严格递增，Compiler 不按 ID、标签或数组到达顺序自行生成。

显式创作者决定使用以下引用合同：

```text
ContentAddressedAuditRef {
  workspace_id,
  project_id,
  audit_owner_kind: "production/bible" | "production/planning" | "asset",
  audit_id,
  audit_revision: integer >= 1,
  audit_content_hash
}
```

`workspace_id/project_id` 必须与 Graph 一致，`audit_owner_kind` 必须等于包含该 Ref 的 Node `owner_kind`，`audit_content_hash` 必须是对应 Owner 不可变审计记录的小写 SHA-256 Content Hash，并被该 Node 的 `owner_content_hash/projection_hash` 二次覆盖。该 Ref 只是业务 Owner Payload 内的内嵌内容定址引用，不是 StoryGraph Node、新 Owner Collection 或 Graph Edge；ReviewDecision 仍不进入 Graph。单个 Ref 无数组排序问题；若未来合同引入数组，必须先发布新 Payload Contract，不得在 v2 临时加字段。

```text
source_revision
source_evidence
policy_snapshot
effective_style_snapshot
episode
scene
dialogue
narrative_beat
artifact
approved_reference_plan_version
```

它们的 `payload_contract_id` 逐个固定为 `storygraph-v2/<node_type>-ref-payload-v1`，Manifest 必须将逐个展开后的字面量列入，不存储通配符。

World Rule 和全局叙事事实必须显式保存无 Evidence 时的创作决定来源：

```text
world_rule | story_arc | plot_thread : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/auditable-bible-fact-payload-v1",
  creator_decision_ref: ContentAddressedAuditRef | null
}
```

这三种 Node 的 Envelope `evidence_refs[]` 非空或 `creator_decision_ref` 非空恰有一项成立；相同 Contract ID 对应一个列出三种 `allowed_node_types` 的 Contract Hash，Manifest 仍分别在三个 Node Definition 中引用该字面 ID/Hash。

可被风格/世界约束的结果共用：

```text
ConstraintRefs {
  world_rule_refs[],
  policy_snapshot_ref,
  effective_style_snapshot_ref
}
```

嵌套 Object/Ref 的 `object_contract_id` 只能使用下列字面 Registry，字段定义就是本 Design 同名代码块；Array Property 不使用此列 ID，只使用后文独立 `array_item_contract_id` Registry：

| 对象 | Contract ID |
|---|---|
| OwnerVersionRef | `storygraph-v2/owner-version-ref-v1` |
| OwnerNodeRef | `storygraph-v2/owner-node-ref-v1` |
| ProjectionRefPayload | `storygraph-v2/projection-ref-payload-base-v1` |
| ContentAddressedAuditRef | `storygraph-v2/content-addressed-audit-ref-v1` |
| ClaimScope | `storygraph-v2/claim-scope-v1` |
| StoryTimeRange | `storygraph-v2/story-time-range-v1` |
| PositiveRational | `storygraph-v2/positive-rational-v1` |
| ConstraintRefs | `storygraph-v2/constraint-refs-v1` |
| Reference Target `target_owner_refs` | `storygraph-v2/reference-target-owner-refs-v1` |
| EvidenceRef | `storygraph-v2/evidence-ref-v1` |
| ArtifactProvenanceRef | `scene-production-packet-v1/artifact-provenance-ref-v1` |

上述嵌套 Contract 同样 `additionalProperties=false`；同名 Contract ID 在 v2 内不得指向不同字段集。

三类 Edge 集必须精确等于该对象；Policy/Style Ref 恰各 1，WorldRule 为 `0..n`。其余 Payload Contract 固定如下：

```text
asset_identity : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/asset-identity-payload-v1",
  asset_kind: "character" | "location" | "prop",
  creator_decision_ref: ContentAddressedAuditRef | null
}

SPECIFICATION : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/specification-payload-v1",
  asset_kind: "character" | "location" | "prop",
  creator_decision_ref: ContentAddressedAuditRef | null
}

asset_state : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/asset-state-payload-v1",
  asset_kind: "character" | "location" | "prop",
  state_key: StableKey,
  story_time_range: null | StoryTimeRange,
  creator_decision_ref: ContentAddressedAuditRef | null
}

production_binding : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/production-binding-payload-v1",
  asset_identity_ref,
  specification_ref,
  state_refs[1..n]
}

occurrence : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/occurrence-payload-v1",
  asset_identity_ref,
  asset_state_ref,
  scene_ref,
  beat_ref: OwnerNodeRef | null,
  creator_decision_ref: ContentAddressedAuditRef | null
}

asset_version : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/asset-version-payload-v1",
  purpose: "character_identity_anchor" | "character_appearance" |
           "location_board" | "prop_sheet",
  asset_identity_ref,
  specification_ref,
  asset_state_ref,
  identity_anchor_asset_version_ref: OwnerNodeRef | null,
  selected_artifact_ref,
  fulfilled_reference_target_ref,
  constraints: ConstraintRefs
}

reference_plan_target : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/reference-plan-target-payload-v1",
  target_kind: "character_identity_anchor" | "character_appearance" |
               "location_board" | "prop_sheet" |
               "scene_composition" | "interaction_composition",
  fulfillment: "required" | "optional" | "not_generated",
  target_owner_refs: {
    identity: OwnerNodeRef[],
    specification: OwnerNodeRef[],
    state: OwnerNodeRef[],
    style: OwnerNodeRef[],
    scene: OwnerNodeRef[],
    occurrence: OwnerNodeRef[],
    interaction: OwnerNodeRef[]
  },
  coverage_scope_keys: ScopeKey[1..n],
  depends_on_target_refs[],
  constraints: ConstraintRefs
}

scene_reference_binding_version : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/scene-reference-binding-payload-v1",
  scene_ref,
  occurrence_refs[],
  interaction_claim_refs[],
  character_asset_version_refs[],
  location_asset_version_ref,
  prop_asset_version_refs[],
  selected_composition_artifact_ref,
  fulfilled_reference_target_ref,
  constraints: ConstraintRefs
}

interaction_reference_binding_version : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/interaction-reference-binding-payload-v1",
  interaction_claim_ref,
  character_asset_version_refs[1..n],
  prop_asset_version_ref,
  selected_composition_artifact_ref,
  fulfilled_reference_target_ref,
  constraints: ConstraintRefs
}

shot : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/shot-payload-v1",
  source_beat_refs[1..n]
}

shot_production_binding_version : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/shot-production-binding-payload-v1",
  shot_ref,
  occurrence_refs[],
  asset_version_refs[],
  scene_reference_binding_refs[],
  interaction_reference_binding_refs[],
  effective_style_snapshot_ref,
  constraints: ConstraintRefs
}
```

`asset_identity|SPECIFICATION|asset_state` 的 Node Envelope `evidence_refs[]` 非空或 `creator_decision_ref` 非空恰有一项成立，因此 Design Gap 补全不会被冒充为原文事实。`asset_version` 只有 `purpose=character_appearance` 时 `identity_anchor_asset_version_ref` 必须非空，其他 purpose 必须为 `null`。Scene/Interaction/Shot Binding 的数组必须与下文 Edge Matrix 一一等价；列表可为空的含义是 Owner 明确发布了空集，不是未加载或“去查最新”。

Bible Claim 和 Causal Claim 共用以下严格合同：

```text
narrative_claim : ProjectionRefPayload + {
  payload_contract_id: "storygraph-v2/narrative-claim-payload-v1",
  claim_series_key: StableKey,
  claim_revision: integer in [1, 9007199254740991],
  predicate: /^[a-z][a-z0-9_]{0,63}$/,
  subject_ref,
  object_ref: OwnerNodeRef | null,
  participant_refs[],
  anchor_refs[1..n],
  valid_scope: ClaimScope,
  story_time_range: null | StoryTimeRange,
  polarity: "positive" | "negative" | "neutral",
  status: "asserted" | "negated",
  creator_decision_ref: ContentAddressedAuditRef | null,
  supersedes_claim_ref: OwnerNodeRef | null
}
```

`relationship_claim|foreshadowing_claim|payoff_claim|causal_claim` 必须使用该 Contract；Node Envelope 的 `evidence_refs[]` 非空或 `creator_decision_ref` 非空恰有一项成立。Participant/Anchor/Supersedes 边集必须与 Payload 一一等价。全部 Claim 的 `claim_revision=1` 当且仅当 `supersedes_claim_ref=null`；`claim_revision>1` 时 Ref 必须非空，并满足下文逐次 `+1` 的单链规则。

Payload Contract 自身的 Hash 元模型固定为：

```text
PropertySchemaDefinition {
  json_pointer: RFC 6901 JSON Pointer,
  json_types: ("array" | "boolean" | "integer" | "null" | "object" | "string")[],
  format: null | non-empty NFC String,
  enum_values: JSON Scalar[],
  pattern: null | non-empty NFC String,
  integer_minimum: null | integer,
  integer_maximum: null | integer,
  min_items: null | integer in [0, 9007199254740991],
  max_items: null | integer in [0, 9007199254740991],
  object_contract_id: null | non-empty NFC String,
  array_item_contract_id: null | non-empty NFC String,
  object_additional_properties: null | false
}

ArraySortRuleDefinition {
  json_pointer: RFC 6901 JSON Pointer,
  sort_mode: "utf8_ascending" | "ascii_ascending" |
             "integer_ascending" | "canonical_json_ascending" |
             "owner_node_ref_tuple_ascending" |
             "evidence_ref_tuple_ascending",
  sort_key_json_pointers: RFC 6901 JSON Pointer[],
  unique: boolean
}

InvariantDefinition {
  invariant_id: non-empty NFC String
}

PayloadContractHashRoot {
  payload_meta_contract_id: "storygraph-payload-contract-meta-v1",
  payload_contract_id: non-empty NFC String,
  allowed_node_types: non-empty NFC String[],
  discriminant_field: null | non-empty NFC String,
  discriminant_value: null | non-empty NFC String,
  required_fields: RFC 6901 JSON Pointer[],
  nullable_fields: RFC 6901 JSON Pointer[],
  property_schemas: PropertySchemaDefinition[],
  array_sort_rules: ArraySortRuleDefinition[],
  invariants: InvariantDefinition[],
  additional_properties: false
}
```

上述对象全部 `additionalProperties=false`，未命中的元数据字段必须使用显式 `null` 或空数组，不得省略。`allowed_node_types/json_types/required_fields/nullable_fields` 与 `sort_key_json_pointers` 按 UTF-8 字节字典序排序去重；`enum_values` 按元素 Canonical JSON 字节排序去重；`property_schemas` 按 `json_pointer`、`array_sort_rules` 按 `json_pointer`、`invariants` 按 `invariant_id` 排序，各排序键唯一。`required_fields` 与 `nullable_fields` 必须是 `property_schemas[].json_pointer` 的子集；每个 Array Property 必须恰有 1 条 Sort Rule，即使业务上有序也必须冻结其唯一排序来源。

正文类型记法到 `PropertySchemaDefinition` 的唯一编码规则固定为：

1. Payload Root 使用 JSON Pointer `""`，其 `object_contract_id=payload_contract_id`；每个字段/嵌套字段各生成 1 个 RFC 6901 Pointer；`ProjectionRefPayload + {...}` 先展平基底字段，不保存 inheritance 标记；
2. `T` 的 `json_types` 只包含 T 的 JSON type；`null | T` 同时包含 `null` 和 T；`?` 只决定该 Pointer 不进入 `required_fields`，不把 `null` 自动加入 `json_types`；`nullable_fields` 恰好等于 `json_types` 含 `null` 的 Pointer 集；
3. 单值字面/枚举写入 `enum_values`，无枚举时为 `[]`；Regex 写入 `pattern`，否则为 `null`；整数边界只在 Integer Property 上保存，其他类型的 `integer_minimum/integer_maximum` 都为 `null`；
4. 除 Root 外，Object Property 的 `object_contract_id` 必须来自本节嵌套 Contract Registry，`object_additional_properties=false`；非 Object 两字段都为 `null`。Array Property 的 `array_item_contract_id` 必须来自下表；非 Array 为 `null`；
5. 无显式基数的 `T[]` 编码为 `min_items=0, max_items=null`；`T[1..n]` 为 `1/null`；其他数字边界逐字面写入。非 Array 的 `min_items/max_items` 都为 `null`；
6. Scalar 的 `format` 只能使用下表字面；枚举、带 `pattern` 字符串、Object、Array 和 Integer 的 `format=null`。

| 正文类型 | `format` |
|---|---|
| non-empty NFC String | `nfc-nonempty` |
| UUID | `uuid-lowercase-hyphenated` |
| lowercase SHA-256 / `*_hash` | `sha256-lowercase-hex` |
| StableKey | `stable-key-v1` |
| ScopeKey | `scene-scope-key-v1` |
| StoryTimeKey | `story-time-key-v1` |
| SequenceKey | `sequence-key-v1` |
| DescriptorKey | `descriptor-key-v1` |
| `story_node_key` | `story-node-key-v1` |
| `edge_key` | `story-edge-key-v1` |
| UTC timestamp | `rfc3339-utc-canonical-v1` |

| Array Item | `array_item_contract_id` |
|---|---|
| OwnerNodeRef | `storygraph-v2/array-item/owner-node-ref-v1` |
| EvidenceRef | `storygraph-v2/array-item/evidence-ref-v1` |
| ScopeKey | `storygraph-v2/array-item/scene-scope-key-v1` |
| StableKey / non-empty NFC String | `storygraph-v2/array-item/nfc-string-v1` |
| lowercase SHA-256 | `storygraph-v2/array-item/sha256-v1` |
| 其他已登记 Object | `<object_contract_id>/array-item-v1` |

所有 Payload Array 在 v2 都是去重集，因此 `ArraySortRuleDefinition.unique=true`。OwnerNodeRef/EvidenceRef 数组分别使用已定义的 tuple sort mode 且 `sort_key_json_pointers=[]`；Scalar 数组使用对应 UTF-8/ASCII/Integer sort mode 且 `sort_key_json_pointers=[""]`；其他 Object 数组使用 `canonical_json_ascending` 和本 Design 显式的 key Pointer，不得同时使用两种 sort mode。

Invariant ID Registry 是下表的字面集，它们分别指向本节相应合同后的规范段落/矩阵；同一 ID 在 v2 不得改义，语义变更必须新 Contract/Schema：

| 适用 Contract | `invariant_id` 字面量 |
|---|---|
| 全部 Payload | `storygraph-v2/invariant/projection-hash-equals-owner-v1` |
| ref-only Payload | `storygraph-v2/invariant/ref-payload-no-business-copy-v1` |
| AssetIdentity/Specification/AssetState/Occurrence/Auditable Bible Fact/Narrative/Continuity | `storygraph-v2/invariant/evidence-xor-creator-decision-v1` |
| AssetIdentity/Specification/AssetState | `storygraph-v2/invariant/asset-kind-and-state-range-v1` |
| ProductionBinding | `storygraph-v2/invariant/production-binding-edge-equivalence-v1` |
| Occurrence | `storygraph-v2/invariant/occurrence-edge-equivalence-v1` |
| AssetVersion | `storygraph-v2/invariant/asset-version-anchor-target-artifact-constraint-v1` |
| ReferencePlanTarget | `storygraph-v2/invariant/reference-target-scope-dependency-constraint-v1` |
| SceneReferenceBinding | `storygraph-v2/invariant/scene-reference-binding-edge-equivalence-v1` |
| InteractionReferenceBinding | `storygraph-v2/invariant/interaction-reference-binding-edge-equivalence-v1` |
| Shot | `storygraph-v2/invariant/shot-source-beat-equivalence-v1` |
| ShotProductionBinding | `storygraph-v2/invariant/shot-binding-edge-equivalence-v1` |
| Narrative Claim | `storygraph-v2/invariant/narrative-claim-edge-equivalence-v1` |
| Interaction Claim | `storygraph-v2/invariant/interaction-predicate-state-machine-v1` |
| Continuity Claim | `storygraph-v2/invariant/continuity-state-transition-v1` |
| 全部 Claim | `storygraph-v2/invariant/claim-supersession-chain-v1` |

Schema Manifest 的 `payload_union_definitions[]` 必须为每个展开 Node Type 保存 `payload_contract_id + payload_contract_hash`。`VP-D14` 只能把本节机械转为 Schema/fixture 并计算 Hash，不得自行增删字段或修改 nullable/排序/不变量规则。

Evidence 的唯一 Graph 存储位置是 Node Envelope 的 `evidence_refs[]`；Payload 禁止再出现 Evidence 数组。`source_evidence` 自身的 Envelope Evidence 必须为空，其 Owner fragment 中的 `(document_revision_id, absolute_start, absolute_end, text_hash)` 必须对应恰好一条 `source_revision → source_evidence` 边。该四元组在同一 Project 的 `source_evidence` Node 集中必须全局唯一；若两个不同 Owner fragment/Node Key 拥有相同四元组，Compiler 必须以 `duplicate_evidence_identity` 拒绝，不得任选其一。对其他 Node，`derived_from` 只映射非 Claim 事实，`supports` 只映射 Claim；二者必须将 Envelope 中每个 Evidence Ref 精确映射到同内容身份的唯一 `source_evidence` Node，无多无少，同一 Claim 不得同时生成两种 Evidence Edge。使用 `creator_decision_ref` 时 Envelope Evidence 必须为空，两种出处不得同时存在。除下文 `derived_from|supports` 目标外，其他 Node Type 的 `evidence_refs[]` 必须为空，不得用无边 Evidence 改变 Node Hash。

`occurrence` 同样必须在非空 Envelope `evidence_refs[]` 与非空 `creator_decision_ref` 之间恰选一。后者只允许 Gate 2 用户明确将 `mentioned_only` 提升为实际视觉出现时使用，审计 Ref 必须属于 `production/planning`，并被该 Occurrence Owner Version 的 Content Hash 覆盖；这是显式创作决定，不冒充原文 Evidence。Scene/Beat/Identity/State Ref 和后续 Target 覆盖规则与 Evidence-backed Occurrence 完全相同。

## `continuity_claim` 的 Interaction 分支

`InteractionClaim` 不新增 Node Type。它只允许投影为：

```text
continuity_claim {
  payload_contract_id: "storygraph-v2/continuity-interaction-payload-v1",
  projection_hash,
  claim_type: "interaction",
  claim_series_key: StableKey,
  claim_revision: integer in [1, 9007199254740991],
  predicate: "hold" | "carry" | "wear" | "use" | "give" |
             "receive" | "place" | "drop" | "open" | "break",
  actor_occurrence_refs[1..n],
  prop_occurrence_ref,
  counterparty_occurrence_ref?,
  scene_ref,
  beat_ref?,
  valid_scope: ClaimScope,
  story_time: StoryTimeKey,
  status: "asserted",
  hand: "left" | "right" | "both" | "unspecified",
  grip_type?: DescriptorKey,
  contact_point?: DescriptorKey,
  direction?: DescriptorKey,
  relative_scale?: PositiveRational,
  holder_before: Character OwnerNodeRef | null,
  holder_after: Character OwnerNodeRef | null,
  prop_state_before: Prop AssetState OwnerNodeRef,
  prop_state_after: Prop AssetState OwnerNodeRef,
  creator_decision_ref: ContentAddressedAuditRef | null,
  supersedes_claim_ref: OwnerNodeRef | null
}
```

Owner Claim 保存精确 participant/anchor/AssetState OwnerNodeRef，Compiler 只把这些 Ref 映射为稳定 Story Node Key 和 Edge；Payload 不允许要求 Owner 反向保存 Graph Key。

Interaction 硬约束：

1. 恰有一个 `prop` participant，必须指向 Prop Occurrence；
2. 至少一个 `actor` Character Occurrence；`give/receive` 需要一个 `counterparty` Character Occurrence；
3. 恰有一个 Scene anchor，可有 Beat anchor，并必须同时锚定 Character/Prop Occurrence；
4. `holder_before/holder_after` 字段必须存在，值只能为 `null` 或已列入 participant 的 Character；
5. `prop_state_before/after` 必须引用同一 Prop 的精确 AssetState；无状态改变时允许两者相同，有变化时必须由 typed delta 或 Evidence 证明；
6. 手别、握法、接触点、方向、相对比例只在 Evidence 或显式用户决定存在时填写，不能由 Preset 或常识补写；
7. Payload 中 participant、anchor、state 与 Graph Edge 必须一一一致；
8. `claim_type` 与 `predicate` 分离，`claim_type` 不得冒充动作词；
9. 跨场长期所有权或人物关系仍使用 RelationshipClaim，不滥用 InteractionClaim。

Predicate 的持有和道具状态转换必须遵守以下矩阵；`actor` / `counterparty` 均指精确 Character Occurrence 对应的 Character，`H` 表示 `null` 或同一已列入 participant：

| Predicate | 必需人物角色 | `holder_before → holder_after` | PropState 规则 |
|---|---|---|---|
| `hold` | actor | `null|actor → actor` | 可相同；从 `null` 开始时，前一时点必须无 holder |
| `carry` | actor | `actor → actor` | 可相同；不隐式表达取得或交接 |
| `wear` | actor | `null|actor → actor` | 开始穿戴时 before/after 必须不同；持续穿戴可相同 |
| `use` | actor | `H → H`，`H` 为 `null` 或同一 participant | 可相同或有 Evidence 的精确变化；不隐式转移 holder |
| `give` | actor + counterparty | `actor → counterparty` | 可相同或有 Evidence 的精确变化 |
| `receive` | actor + counterparty | `counterparty → actor` | 可相同或有 Evidence 的精确变化 |
| `place` | actor | `actor → null` | before/after 必须不同并包含放置位置/姿态 delta |
| `drop` | actor | `actor → null` | before/after 必须不同并包含掉落位置/状态 delta |
| `open` | actor | `H → H` | before/after 必须不同并包含 `closed → open` delta |
| `break` | actor | `H → H` | before/after 必须不同并包含完整度下降/broken delta |

`give` 与 `receive` 是同一类交接事件的两种叙事视角；同一 Prop、Anchor 和 story time 只能发布一个规范 Claim，typed Query 可按角色反向展示，不得重复应用转换。按 story time 派生的持有账本必须保证同一物理 Prop 在任一时点最多一个 holder；只有已建立不同 AssetIdentity 的复制品才能例外。任何必需转换缺失、前后账本不连续、双 holder 或 PropState delta 冲突都必须在 Gate 2 保持 `unresolved` 并阻断对应 Owner Apply；Compiler 只校验和投影已发布 Claim，不能补猜转换。

## `continuity_claim` 的普通 Continuity 分支

普通连续性不使用自由文本 predicate，v2 只接受以下严格 payload：

```text
continuity_claim {
  payload_contract_id: "storygraph-v2/continuity-state-payload-v1",
  projection_hash,
  claim_type: "continuity",
  claim_series_key: StableKey,
  claim_revision: integer in [1, 9007199254740991],
  predicate: "state_persists" | "state_changes",
  subject: AssetIdentity OwnerNodeRef,
  state_before: AssetState OwnerNodeRef,
  state_after: AssetState OwnerNodeRef,
  anchor_start: Episode | Scene | Beat | Occurrence OwnerNodeRef,
  anchor_end: Episode | Scene | Beat | Occurrence OwnerNodeRef,
  valid_scope: ClaimScope,
  story_time_start: StoryTimeKey,
  story_time_end: StoryTimeKey,
  status: "asserted" | "negated",
  creator_decision_ref: ContentAddressedAuditRef | null,
  supersedes_claim_ref: OwnerNodeRef | null
}
```

该分支必须恰有一个 `subject`，前后 State 属于该身份，故事时间不逆序，起止 Anchor 在有效范围内。`state_persists` 要求前后为同一 AssetState；`state_changes` 要求两者不同且有 Evidence 或显式创作者决定说明 delta。正式 v2 不接受 `uncertain` Continuity；未决不确定项保留在 Candidate/Review Issue，并在 Gate 2 阻断对应作用域。

Graph 必须为该 payload 投影恰好一条 `claim_participant(participant_role=subject)`、一条 `claim_state(state_role=before)`、一条 `claim_state(state_role=after)`、一条 `claim_anchor(anchor_role=scope_start)` 和一条 `claim_anchor(anchor_role=scope_end)`；即使起止 Ref 相同，两条不同 qualifier 的 Anchor Edge 仍必须存在。

两个 Continuity 分支都必须满足“Node Envelope `evidence_refs[]` 非空”与“`creator_decision_ref` 非空”恰好一项；其 `supersedes_claim_ref` 也必须与 `supersedes` 入边精确一一等价。

Manifest 对 `continuity_claim` 必须展开两个互斥的 `payload_union_definitions`：`claim_type=interaction` 唯一映射 `storygraph-v2/continuity-interaction-payload-v1`，`claim_type=continuity` 唯一映射 `storygraph-v2/continuity-state-payload-v1`。两个 Contract 各自生成独立 `payload_contract_hash`，不共用 union 虚拟 Hash，不允许第三个 discriminant。

## v2 Edge Type Matrix

所有权威 Edge 统一保持“上游证据/约束/输入 → 下游解释/需求/结果”，并通过稳定端点 Key、Edge Type 和严格 qualifier 派生 `edge_key`。下表别名只是 Schema fixture 的可机械展开 macro，不得作为存储值：

| 冻结别名 | 精确 Node Type 展开 |
|---|---|
| `SPECIFICATION` | `character_specification|location_specification|prop_specification` |
| `BIBLE_CLAIM` | `relationship_claim|foreshadowing_claim|payoff_claim` |
| `ANY_CLAIM` | `relationship_claim|foreshadowing_claim|payoff_claim|continuity_claim|causal_claim` |
| `STRUCTURE_ANCHOR` | `episode|scene|narrative_beat|occurrence` |
| `REFERENCE_BINDING` | `scene_reference_binding_version|interaction_reference_binding_version` |
| `REFERENCE_RESULT` | `asset_version|scene_reference_binding_version|interaction_reference_binding_version` |
| `CONSTRAINT_TARGET` | `reference_plan_target|asset_version|scene_reference_binding_version|interaction_reference_binding_version|shot_production_binding_version` |

| Edge Type | 允许的 Source → Target | 必需 qualifier / cardinality |
|---|---|---|
| `contains` | `episode → scene`；`scene → dialogue|narrative_beat` | Parent 必须逐字节等于 Target Owner 已发布的 parent ref，`sequence_key: SequenceKey` 逐字节等于 Target 在该 Parent 下的 Owner order key；每个子节点恰有 1 个结构父级 |
| `derived_from` | `source_revision → source_evidence`；`source_evidence → asset_identity|SPECIFICATION|asset_state|world_rule|story_arc|plot_thread|episode|scene|dialogue|narrative_beat|occurrence` | 无；每个 Evidence 恰 1 条 Source Revision 入边，其他目标入边集必须精确等于 Node Envelope Evidence 映射；Identity/Specification/State/WorldRule/Arc/Thread/Occurrence 有显式创作者决定时可为 0，Episode/Scene/Dialogue/Beat 必须为 `1..n` |
| `describes_identity` | `asset_identity → SPECIFICATION` | 无；每个 Specification 恰有 1 条，Asset kind 与 Specification Type 匹配 |
| `has_state` | `asset_identity → asset_state` | 无；每个 State 恰有 1 条，kind 匹配 |
| `precedes` | `episode|scene|narrative_beat|shot` 各自同类型相邻节点 | 只连接同 Parent/同 Scene scope 按 Owner `SequenceKey` 排序后的相邻项；`sequence_key: SequenceKey` 逐字节等于 Target（后一项）的 Owner order key，非首项恰有 1 条入边，非末项恰有 1 条出边 |
| `anchors_occurrence` | `scene|narrative_beat → occurrence` | `anchor_role=scene|beat`；入边精确等于 Occurrence payload 的 `scene_ref + beat_ref`，Scene 恰 1、Beat `0..1` |
| `instantiates_occurrence` | `asset_state → occurrence` | 无；入边精确等于 Occurrence payload 的 `asset_state_ref`，恰 1 |
| `supports` | `source_evidence → ANY_CLAIM` | 无；入边集精确等于 Claim Node Envelope Evidence 映射；显式创作者决定可为 0 |
| `claim_participant` | `asset_identity|occurrence → ANY_CLAIM` | `participant_role=subject|object|participant|actor|prop|counterparty|holder_before|holder_after`；分支基数见下表 |
| `claim_anchor` | `STRUCTURE_ANCHOR → ANY_CLAIM` | `anchor_role=episode|scene|beat|character_occurrence|prop_occurrence|scope_start|scope_end`；分支基数见下表 |
| `claim_state` | `asset_state → continuity_claim` | `state_role=before|after|prop_before|prop_after`；分支基数见下表 |
| `supersedes` | 同 Node Type、同 Claim family/discriminant 与同 `claim_series_key` 的 Older Claim → Newer Claim | 无；每个 Newer Claim 的入边精确等于 payload `supersedes_claim_ref`（`0..1`），两个 Claim 的 `owner_logical_id/story_node_key` 必须不同、valid scope 兼容，且 Newer `claim_revision = Older claim_revision + 1`；同一 logical id 的 Owner Version 替换只表现为 Node content changed，不生成自环；`continuity` 与 `interaction` 绝不得互相 supersede |
| `constrains` | `world_rule|policy_snapshot|effective_style_snapshot → CONSTRAINT_TARGET` | `constraint_role=world|policy|style`；source 与 role 一一对应，边集等于 target payload 的精确 constraint refs |
| `materializes` | 见下方 Materialization Matrix | `binding_role=asset|specification|state|identity_anchor|artifact`；不接受其他组合 |
| `contains_reference_target` | `approved_reference_plan_version → reference_plan_target` | `sequence_key: SequenceKey` 逐字节等于 Target 在 Plan 内的 Owner order key；Target 恰有 1 个 Plan 父级 |
| `depends_on_reference_target` | `reference_plan_target → reference_plan_target` | 无；边集等于 Target payload 依赖且 Plan 内无环 |
| `plans_reference` | 见下方 Reference Planning Matrix | `reference_role=identity|specification|state|style|scene|occurrence|interaction` |
| `fulfills_reference_target` | `reference_plan_target → REFERENCE_RESULT` | 无；target/result 类型与 phase-aware 基数见下表 |
| `binds_reference_input` | 见下方 Reference Binding Matrix | `reference_role=scene|occurrence|interaction|character_asset|location_asset|prop_asset` |
| `binds_reference_output` | `artifact → REFERENCE_BINDING` | 无；每个 Binding 恰有 1 个 selected READY Composition Artifact |
| `realizes` | `narrative_beat → shot` | 无；每个 Shot 入边 `1..n`，精确等于 Shot payload source refs |
| `informs` | `occurrence|scene_reference_binding_version|interaction_reference_binding_version → shot_production_binding_version` | `informs_role=occurrence|scene_reference|interaction_reference`；source 与 role 一一对应且同 Scene scope，三类入边分别精确等于 Binding payload 的 `occurrence_refs[]|scene_reference_binding_refs[]|interaction_reference_binding_refs[]` |
| `binds_input` | `shot|occurrence|asset_version|REFERENCE_BINDING|effective_style_snapshot → shot_production_binding_version` | `binding_role=shot|occurrence|asset_version|scene_reference|interaction_reference|style`；source 与 role 一一对应，边集等于 target payload |

Claim 分支基数固定为：

| Claim 分支 | Participant | Anchor | State |
|---|---|---|---|
| `BIBLE_CLAIM|causal_claim` | `subject` 恰 1，`object` 为 `0..1`，`participant` 为 `0..n`；精确等于 payload | `episode|scene|beat` 合计 `1..n`；精确等于 payload | 0 |
| `continuity_claim(claim_type=continuity)` | `asset_identity → claim`，`subject` 恰 1 | `scope_start` 恰 1 + `scope_end` 恰 1 | `before` 恰 1 + `after` 恰 1 |
| `continuity_claim(claim_type=interaction)` | Character Occurrence `actor` `1..n`；Prop Occurrence `prop` 恰 1；`give|receive` 的 Character Occurrence `counterparty` 恰 1，其他为 0；非空 holder 以 AssetIdentity 投影对应 role 恰 1 | Scene `scene` 恰 1，Beat `beat` `0..1`，Character Occurrence `character_occurrence` `1..n`，Prop Occurrence `prop_occurrence` 恰 1 | `prop_before` 恰 1 + `prop_after` 恰 1 |

Materialization Matrix 固定为：

| Source | Target | `binding_role` | Target 基数 |
|---|---|---|---|
| `asset_identity` | `production_binding` | `asset` | 恰 1 |
| `SPECIFICATION` | `production_binding` | `specification` | 恰 1，kind 匹配 |
| `asset_state` | `production_binding` | `state` | `1..n`，精确等于 Binding payload |
| `asset_identity` | `asset_version` | `asset` | 恰 1 |
| `SPECIFICATION` | `asset_version` | `specification` | 恰 1，kind/purpose 匹配 |
| `asset_state` | `asset_version` | `state` | 恰 1 |
| `asset_version(purpose=character_identity_anchor)` | `asset_version(purpose=character_appearance)` | `identity_anchor` | Character appearance 恰 1，其他 purpose 为 0 |
| `artifact` | `asset_version` | `artifact` | 恰 1 selected READY Artifact |

`describes_identity` 和 `has_state` 不做同 kind 猜测：每个 `production_binding` 的 `asset_identity_ref + specification_ref + state_refs[]` 必须分别唯一投影一条 `asset_identity → SPECIFICATION` 和对每个 State 的 `asset_identity → asset_state`；Graph 中的这两类 Edge 必须恰好等于全部 ProductionBinding payload 的去重并集。每个 Specification/State 必须被恰好一个同 Asset kind 的 ProductionBinding 覆盖，否则拒绝发布。

Reference Planning Matrix 固定为：

| Source | `reference_role` | Target payload 对应字段 |
|---|---|---|
| `asset_identity` | `identity` | `target_owner_refs.identity[]` |
| `SPECIFICATION` | `specification` | `target_owner_refs.specification[]` |
| `asset_state` | `state` | `target_owner_refs.state[]` |
| `effective_style_snapshot` | `style` | `target_owner_refs.style[]` |
| `scene` | `scene` | `target_owner_refs.scene[]` |
| `occurrence` | `occurrence` | `target_owner_refs.occurrence[]` |
| `continuity_claim(claim_type=interaction)` | `interaction` | `target_owner_refs.interaction[]` |

每个 Target 的 Planning Edge 去重集必须精确等于 payload，不允许 Compiler 自动扩展相似身份或同 Scene 资产。Reference Target 的输入兼容与业务唯一矩阵固定为：

| `target_kind` | `target_uniqueness_key` | `expected_target_coverage` | `identity_refs` | `specification_refs` | `state_refs` | `style_refs` | `scene_refs` | `occurrence_refs` | `interaction_refs` | `target_dependencies` |
|---|---|---|---|---|---|---|---|---|---|---|
| `character_identity_anchor` | `(target_kind, identity[0])` 的 logical ref key | 第一遍：`p1_scope_keys[]` 完整 Occurrence 并集中每个 Character Identity 恰 1；Scope 精确为该 Character 实际出现的 p1 Scene 集 | 恰 1 个 Character | 恰 1 个描述该 Character 的 CharacterSpecification | 恰 1 个属于该 Character 且由 Gate 3 审核选定的身份锚点 AssetState | 恰 1，等于 `constraints.effective_style_snapshot_ref` | `1..n`，与 `coverage_scope_keys[]` 逐一对应 | `0..n`，均为该 Character/锚点 State 在 `scene[]` 中的 Occurrence | 0 | 0 |
| `character_appearance` | `(target_kind, identity[0], specification[0], state[0])` 的 logical ref key | 第二遍：先读该 Character 唯一 Anchor Target 的 State，再为每个实际出现且不等于该 State 的 Character Identity/Specification/State 三元组生成恰 1；Scope 精确为该三元组出现的 p1 Scene 集 | 恰 1 个 Character | 恰 1 个描述该 Character 的 CharacterSpecification | 恰 1 个属于该 Character 的 AssetState | 恰 1，等于 `constraints.effective_style_snapshot_ref` | `1..n`，与 `coverage_scope_keys[]` 逐一对应 | `0..n`，均为该 Character/State 在 `scene[]` 中的 Occurrence | 0 | 恰 1 个同 Character/Specification/Style 且覆盖当前 Scope 的 `character_identity_anchor` Target，且依赖 fulfillment rank 不低于本 Target |
| `location_board` | `(target_kind, identity[0], specification[0], state[0])` 的 logical ref key | `p1_scope_keys[]` 内每个实际出现的 Location Identity/Specification/State 三元组恰 1；Scope 精确为该三元组出现的 p1 Scene 集 | 恰 1 个 Location | 恰 1 个描述该 Location 的 LocationSpecification | 恰 1 个属于该 Location 的 AssetState | 恰 1，等于 `constraints.effective_style_snapshot_ref` | `1..n`，与 `coverage_scope_keys[]` 逐一对应 | `0..n`，均为该 Location/State 在 `scene[]` 中的 Occurrence | 0 | 0 |
| `prop_sheet` | `(target_kind, identity[0], specification[0], state[0])` 的 logical ref key | `p1_scope_keys[]` 内每个实际出现的 Prop Identity/Specification/State 三元组恰 1；Scope 精确为该三元组出现的 p1 Scene 集 | 恰 1 个 Prop | 恰 1 个描述该 Prop 的 PropSpecification | 恰 1 个属于该 Prop 的 AssetState | 恰 1，等于 `constraints.effective_style_snapshot_ref` | `1..n`，与 `coverage_scope_keys[]` 逐一对应 | `0..n`，均为该 Prop/State 在 `scene[]` 中的 Occurrence | 0 | 0 |
| `scene_composition` | `(target_kind, scene[0])` 的 logical ref key | 每个 `p1_scope_key` 恰 1；不制作也必须显式保存 `fulfillment=not_generated` | `1..n`，精确等于 Scene production closure 的 Identity 去重集 | `1..n`，精确等于该 closure 的 Specification 去重集 | `1..n`，精确等于该 closure 的 AssetState 去重集 | 恰 1，等于 `constraints.effective_style_snapshot_ref` | 恰 1，且对应唯一 `coverage_scope_keys[0]` | 精确等于该 Scene 的完整 Occurrence 集 | 精确等于锚定该 Scene 的 InteractionClaim 集 | `not_generated` 为 0；否则 `1..n`，恰为 Scene production closure 每个三元组对应的基础 Target 去重集，每个依赖 fulfillment rank 不低于本 Target |
| `interaction_composition` | `(target_kind, interaction[0])` 的 logical ref key | `p1_scope_keys[]` 内每个已确认 InteractionClaim 恰 1；不制作也必须显式保存 `fulfillment=not_generated` | `2..n`，精确等于 actor/counterparty/prop 的 Identity 去重集 | `2..n`，精确等于这些参与者的 Specification 去重集 | `2..n`，精确等于参与 Occurrence 的 AssetState 去重集 | 恰 1，等于 `constraints.effective_style_snapshot_ref` | 恰 1，等于 InteractionClaim 的 Scene，且对应唯一 `coverage_scope_keys[0]` | `2..n`，精确等于 InteractionClaim 的 actor/prop/counterparty Occurrence 去重集 | 恰 1 个 `claim_type=interaction` 的 ContinuityClaim | `not_generated` 为 0；否则 `2..n`，恰为参与三元组对应的基础 Target 去重集，每个依赖 fulfillment rank 不低于本 Target |

上表 `logical_ref_key(ref)` 固定为 JSON Array `[ref.owner_kind, ref.version_family, ref.owner_logical_id, ref.fragment_key ?? ""]`，不把显示名、数组到达顺序或版本新旧当作业务身份。每行 `target_uniqueness_key` 中的 Ref 按单元格列出顺序转换，`target_business_key_bytes = Canonical JSON([target_kind, logical_ref_key(ref1), ..., logical_ref_key(refN)])`；四元组内容和外层顺序都不可省略，Key 集按该 Canonical JSON UTF-8 字节字典序排序去重。一个 `ApprovedReferencePlanVersion` 内每个业务唯一键最多一个 Target，因此每个 Character 恰有一个身份锚点，不会因换装或伤势 State 重复生成锚点；重复以 `duplicate_reference_target_business_key` 拒绝。所有基础 Target 的 Identity/Specification/State 必须经精确 `ProductionBinding` 构成合法三元组；每个 Occurrence 的 Identity/State 必须经唯一 active `ProductionBinding` 解析到恰 1 个 Specification，0 个或多个都以 `ambiguous_production_binding` 拒绝。Scene production closure 是该 Scene 完整 Occurrence 所引用的 Identity/State 与该唯一 Specification 的去重并集，且必须包含恰 1 个 Location 三元组。Compiler 不从名称、图像相似度或同 kind 候选中补齐 closure。

正式 Plan 的完整性不以“Owner 说这是完整数组”自证。Compiler 必须从同一 `owner_set_hash` 内的 p0 Scene/Occurrence/Interaction/ProductionBinding 根和 `p1_scope_keys[]`，按上表六行 `expected_target_coverage` 机械生成 `expected_target_business_keys`。该算法固定为两遍：先仅由 p0 实际 Character Occurrence 生成 Identity-only Anchor 预期键并要求每键恰 1 个 Target；再读取该 Target 内由 Gate 3 ReviewDecision 选定、被 ApprovedReferencePlanVersion Content Hash 覆盖的唯一 `state[0]` 作为 `identity_anchor_state_ref`，为其余全部实际 Character State 生成 Appearance 预期键，同时生成 Location/Prop/Scene/Interaction 预期键。Anchor State 是受审视觉基础选择，不冒充 p0 原文事实；唯一键、Target State 合法性和 Plan Content Hash 共同消除 A/B State 二选一的实现自由。Plan 内 Target 的业务唯一键集必须与它逐字节等价，少一个以 `missing_expected_reference_target`、多一个以 `unexpected_reference_target` 拒绝。每个预期 Target 都必须显式保存 `required|optional|not_generated` 之一，不得用“从数组省略”表达不生成。只有 mention 而没有实际 Occurrence 的 `mentioned_only` 不进入预期集，这是其“默认不生成”的精确语义；若用户要求制作，必须先在 Gate 2 以可审计创作决定发布实际视觉 Occurrence，不允许 Gate 3 偷加无上游事实的 Target。

Target 的 `target_owner_refs.style[]` 必须恰等于 `[constraints.effective_style_snapshot_ref]`，Planning Edge 和 `coverage_scope_keys[]` 必须与上表 Scene Ref 的 `scene:<owner_logical_id>` 去重集逐字节等价。`depends_on_target_refs[]` 只能引用同 Plan，fulfillment rank 固定为 `not_generated=0, optional=1, required=2`，任一依赖的 rank 都不得低于依赖者。Character Appearance 依赖的 Anchor Scope 必须是当前 Scope 的超集。Composition Target 为 `not_generated` 时依赖必须为空；否则每个计划三元组必须恰有一个与本 Scope 相交的基础 Target，Character 锚点 State 使用 Anchor Target，其他 Character State 使用 Appearance Target，Location/Prop 分别使用 Board/Sheet Target，依赖集不得多少或替代。任一输入、Scope 或依赖不等价以 `reference_target_input_mismatch` 拒绝。

每个 p1 非空的 Project 必须恰有一个 active `ApprovedReferencePlanVersion`，该 Plan 全部 Target 的 `coverage_scope_keys[]` 去重并集必须精确等于 `p1_scope_keys[]`；不允许按 Scene 并行激活多个 Plan，否则同一 Character 可在不同 Plan 中得到不同身份锚点。项目可通过发布同一 Plan logical id 的新不可变 Version 扩大 Scope 或修改目标，但编译时只能有一个 active Version。所有 active Target、Result 与 Reference Binding 必须属于这一 Plan：Target 由唯一 `contains_reference_target` Parent 证明，Result 由 `fulfilled_reference_target_ref` 反查同一 Parent 证明。任何 p1 Scope 无 Plan、多个 active Plan 或 Plan Scope 并集不等于 `p1_scope_keys[]` 都以 `ambiguous_active_reference_plan` 拒绝；因此 SceneProductionPacket 的单值 `approved_reference_plan_version_ref` 是可机械证明的，不是“选最新”。

Target 与履约结果的类型矩阵固定为：

| `target_kind` | 唯一允许的结果 |
|---|---|
| `character_identity_anchor` | `asset_version(purpose=character_identity_anchor)` |
| `character_appearance` | `asset_version(purpose=character_appearance)` |
| `location_board` | `asset_version(purpose=location_board)` |
| `prop_sheet` | `asset_version(purpose=prop_sheet)` |
| `scene_composition` | `scene_reference_binding_version` |
| `interaction_composition` | `interaction_reference_binding_version` |

`fulfills_reference_target` 不按 Graph 的全局最高 phase 判定，而按每个 Target 的 Scene coverage 判定。基础 Target 允许 `coverage_scope_keys` 为 `1..n`；Scene/Interaction Composition Target 必须恰有 1 个 Scene Scope。

| Target 激活条件 | `required` | `optional` | `not_generated` |
|---|---|---|---|
| 基础 Target：`target.coverage_scope_keys ∩ p1_scope_keys ≠ ∅` | 恰 1 | `0..1` | 0 |
| 组合 Target：唯一 Scope 属于 `p2_scope_keys` | 恰 1 | `0..1` | 0 |
| 不满足上述激活条件 | 0；即使未来 required 也尚未到履约阶段 | 0 | 0 |

`p3_scope_keys` 已是 `p2_scope_keys` 子集，不改变 Reference 履约基数。因此 A Scene 已通过 Gate 4、B Scene 仅通过 Gate 3 时，A 的 required Composition 必须恰 1，B 的结果必须为 0，二者可同时存在于一份 p2 Graph。

每个 `asset_version|scene_reference_binding_version|interaction_reference_binding_version` Result 必须在 payload 中保存恰好一个 `fulfilled_reference_target_ref`，且该 Ref 必须指向同一 `ApprovedReferencePlanVersion`、类型矩阵匹配且已激活的 Target。`fulfills_reference_target` 必须逐字节从 Result 的该字段投影：每个 Result 恰 1 条入边，每个 Target 的出边去重集恰好等于所有反向指向它的 Result，再应用上表 phase-aware 基数。Compiler 不得按 `target_kind/purpose/scene` 相似性配对。

结果不只要“类型相同”，还必须与其 Target 输入逐字节等价：

- 基础 `asset_version` 的 `purpose/asset_identity_ref/specification_ref/asset_state_ref/constraints` 必须分别等于 Target Kind、三个单例 Ref 和 `constraints`；`character_identity_anchor|location_board|prop_sheet` 的 `identity_anchor_asset_version_ref` 必须为 `null`，`character_appearance` 的该 Ref 必须是履约其唯一 Anchor 依赖的精确 AssetVersion；
- `scene_reference_binding_version` 的 `scene_ref/occurrence_refs[]/interaction_claim_refs[]/constraints` 必须等于 Scene Composition Target 的对应 Ref 与约束；Binding 中全部 Character/Location/Prop AssetVersion 的 `(asset_identity_ref, specification_ref, asset_state_ref)` 去重集必须恰等于 Target 的 Scene production closure，且这些 AssetVersion 的 `fulfilled_reference_target_ref` 去重集必须恰等于 `depends_on_target_refs[]`；
- `interaction_reference_binding_version` 的 `interaction_claim_ref/constraints` 必须等于 Interaction Composition Target 的单例 Interaction 与约束；Target 的 Scene/Occurrence 必须精确等于该 Claim 的 Scene 与 actor/prop/counterparty Occurrence，Binding 中 Character/Prop AssetVersion 的三元组去重集必须恰等于 Target 的 Identity/Specification/State 计划三元组，其 `fulfilled_reference_target_ref` 去重集必须恰等于 `depends_on_target_refs[]`。

上述 Result 的 `fulfilled_reference_target_ref` 也必须逐字节等于正在验证的 Target，选中 Artifact 与 `binds_reference_output/materializes(binding_role=artifact)` 仍按原矩阵一一等价。任一 Result 与 Target 的身份、State、Scene、Interaction、依赖或约束不一致以 `reference_target_result_mismatch` 拒绝，不得用已生成的“相似”结果顶替。

Reference Binding Matrix 固定为：

| Target Binding | Source | `reference_role` | Target 基数 |
|---|---|---|---|
| `scene_reference_binding_version` | `scene` | `scene` | 恰 1 |
| 同上 | `occurrence` | `occurrence` | `0..n`，精确等于 Binding payload |
| 同上 | `continuity_claim(claim_type=interaction)` | `interaction` | `0..n`，精确等于 Binding payload |
| 同上 | `asset_version(purpose=character_identity_anchor|character_appearance)` | `character_asset` | `0..n`，精确等于 Scene Occurrence 需求 |
| 同上 | `asset_version(purpose=location_board)` | `location_asset` | 恰 1 |
| 同上 | `asset_version(purpose=prop_sheet)` | `prop_asset` | `0..n`，精确等于 Scene Occurrence 需求 |
| `interaction_reference_binding_version` | `continuity_claim(claim_type=interaction)` | `interaction` | 恰 1 |
| 同上 | `asset_version(purpose=character_identity_anchor|character_appearance)` | `character_asset` | `1..n`，精确等于 actor/counterparty |
| 同上 | `asset_version(purpose=prop_sheet)` | `prop_asset` | 恰 1 |

上表中“精确等于 payload”的含义是：Owner Binding 已保存排序、去重的 OwnerNodeRef 列表，Compiler 必须做一一集合等价校验；它不是自由文本规则。

类型矩阵必须拒绝 Binding → Scene/Claim/AssetVersion 的反向权威边、人物与道具直连 Edge、Binding 互相冒充以及任意自定义 Edge Type。Reference Edge 不能复用 ProductionBinding 或 ShotProductionBinding 保存组合结果。

## Canonical、Hash 与发布门禁

所有 Hash 使用 SHA-256 小写十六进制。`storygraph-canonical-json-v2` 的字节合同固定为“先对所有 String Value/Object Key 做 Unicode NFC，再按 RFC 8785 JSON Canonicalization Scheme 序列化”，并附加以下更严约束：

- 输出必须是 UTF-8，Object Key 按 RFC 8785 的 UTF-16 code-unit 顺序，无 BOM 和多余空白；
- 字符串只转义 `"`、`\` 和 U+0000–U+001F 控制字符；`\\/\<\>\&` 不做 HTML 转义，U+2028/U+2029 不额外转义，`\\u00xx` 使用小写十六进制，非法 surrogate 一律拒绝；
- 数值只允许 `[-9007199254740991, 9007199254740991]` 内的十进制整数，无前导 `+`、无多余前导 0，不允许 float、NaN 或 Infinity；
- UUID 使用小写带连字符格式；Hash 使用 64 位小写十六进制；
- 时间必须重新序列化为 UTC `YYYY-MM-DDTHH:mm:ss[.fraction]Z`，小数秒尾随 0 移除，不允许 `+00:00`；
- Schema 声明 optional 时才省略字段，声明 nullable 时必须保留 `null`；数组在进入 Hash 前按各自合同键排序。

`VP-D14` 必须用包含中文、emoji、`<>&/`、U+2028/U+2029、控制字符、时区输入、整数边界和非法 surrogate 的同一 fixture 验证 Go/Python 成功字节或失败代码完全相同。

每个 v2 Node 必须且只能包含：

```text
story_node_key
node_type
owner_ref {
  workspace_id,
  project_id,
  owner_kind,
  version_family,
  owner_logical_id,
  fragment_key?,
  fragment_content_hash?,
  owner_version_id,
  owner_revision,
  owner_content_hash
}
evidence_refs[]
payload（按 node_type 严格判别）
content_hash
```

v2 不存储 `label` 或 `business_position`；这些可变展示字段由 typed Owner Query 在 Read Model 中按 `owner_ref` 补齐，不进入 Node、Schema Manifest 或 Hash。v1 中既有字段仍随历史 Version 可读，Cross-schema Diff 不将纯展示字段当成 v2 业务变更。`evidence_refs[]` 的元素合同固定为：

```text
EvidenceRef {
  document_revision_id,
  absolute_start: integer in [0, 9007199254740991],
  absolute_end: integer in [1, 9007199254740991],
  text_hash
}
```

`document_revision_id` 是非空 NFC JSON String；区间必须与已接受 `SourceSpanIndexVersion` 一致：使用规范化原文的 Unicode code-point 半开区间，且满足 `absolute_start < absolute_end`。`text_hash` 是对该 code-point slice 重新以 UTF-8 序列化后计算的小写 SHA-256。不得在 UTF-8 byte、UTF-16 code-unit 或 grapheme cluster 之间静默换算；同一 EvidenceRef 不可重复。

Node 的 `content_hash` 精确对以下根对象计算，不包含 `content_hash` 自身：

```text
NodeHashRoot {
  story_node_key,
  node_type,
  owner_ref: OwnerNodeRef,
  evidence_refs[],
  payload
}
```

`evidence_refs[]` 按 `(document_revision_id, absolute_start, absolute_end, text_hash)` 排序。每个 Edge 包含 `edge_key + edge_type + from_node_key + to_node_key + qualifier + content_hash`。`qualifier` 字段始终 required，是下列 `additionalProperties=false` 严格联合中恰好一项：

```text
NoQualifier {}
SequenceQualifier {sequence_key: SequenceKey}
AnchorQualifier {
  anchor_role: "scene" | "beat" | "episode" | "character_occurrence" |
               "prop_occurrence" | "scope_start" | "scope_end"
}
ParticipantQualifier {
  participant_role: "subject" | "object" | "participant" | "actor" |
                    "prop" | "counterparty" | "holder_before" | "holder_after"
}
StateQualifier {state_role: "before" | "after" | "prop_before" | "prop_after"}
ConstraintQualifier {constraint_role: "world" | "policy" | "style"}
BindingQualifier {
  binding_role: "asset" | "specification" | "state" | "identity_anchor" |
                "artifact" | "shot" | "occurrence" | "asset_version" |
                "scene_reference" | "interaction_reference" | "style"
}
ReferenceQualifier {
  reference_role: "identity" | "specification" | "state" | "style" |
                  "scene" | "occurrence" | "interaction" | "character_asset" |
                  "location_asset" | "prop_asset"
}
InformsQualifier {
  informs_role: "occurrence" | "scene_reference" | "interaction_reference"
}
```

`contains|precedes|contains_reference_target` 唯一使用 `SequenceQualifier`；`anchors_occurrence|claim_anchor` 使用 `AnchorQualifier`；`claim_participant` 使用 `ParticipantQualifier`；`claim_state` 使用 `StateQualifier`；`constrains` 使用 `ConstraintQualifier`；`materializes|binds_input` 使用 `BindingQualifier`；`plans_reference|binds_reference_input` 使用 `ReferenceQualifier`；`informs` 使用 `InformsQualifier`；其他 Edge Type 只允许 `NoQualifier`。联合内的 role 仍必须同时满足 Edge Matrix 的端点类型约束；不允许用某个合法 role 绕过端点矩阵。Edge `content_hash` 对以下根对象计算：

```text
EdgeHashRoot {
  edge_key,
  edge_type,
  from_node_key,
  to_node_key,
  qualifier
}
```

Graph 的两个 Hash 根对象不得混用：

```text
TopologyHashRoot {
  schema_id,
  schema_manifest_hash,
  coverage_phase,
  coverage_scope_manifest_hash,
  node_key_derivation_id: "story-node-key-v1",
  edge_key_derivation_id: "story-edge-key-v1",
  nodes[]: {story_node_key, node_type},
  edges[]: {edge_key, edge_type, from_node_key, to_node_key, qualifier}
}

GraphContentHashRoot {
  schema_id,
  schema_manifest_hash,
  coverage_phase,
  coverage_scope_manifest_hash,
  node_key_derivation_id: "story-node-key-v1",
  edge_key_derivation_id: "story-edge-key-v1",
  owner_set_hash,
  nodes[]: complete Node including node content_hash,
  edges[]: complete Edge including edge content_hash
}
```

Node 按 `story_node_key` 排序，Edge 按 `edge_key` 排序。Layout、viewport、颜色、折叠和筛选不进入任何 Hash。`StoryGraphVersion` 必须保存 `workspace_id/project_id/version_no/parent_version_id/parent_content_hash + schema_id/schema_rank/schema_manifest_hash + complete verified_coverage_proof/coverage_phase/coverage_scope_manifest_hash + node_key_derivation_id/edge_key_derivation_id + complete exact_owner_collections/owner_set_hash + topology_hash/content_hash + published_at/created_by`。缺任一身份字段或不能从存储前像离线重算全部 Hash 的 Version 不可发布或成为 Head。

发布前必须验证：

1. Node/Edge Key 可分别按 `story-node-key-v1` / `story-edge-key-v1` 重建，唯一且稳定；
2. OwnerNodeRef 的 workspace/project、family、logical/fragment、version/revision/owner hash 与编译 Collection 一致；
3. Node Type、Owner、Payload discriminant 和 Edge Matrix 完全匹配；
4. Interaction Payload 与 participant/anchor/state Edge 完全一致；
5. Reference Target 的范围、依赖、`fulfillment` 和正式结果满足上文 per-target scope-aware 基数；未进入 `p2_scope_keys` 的 required Composition 尚未履约不会阻断已通过 Gate 4 的其他 Scene；
6. Artifact 只有被已发布 AssetVersion/ReferenceBinding 选择且 READY、Rights/Lineage 完整时才投影；
7. 不存在 dangling ref、自环、重复 Edge 或任意环；稳定 Kahn 排序以 Node Key 裁决并列顺序；
8. 相同 `schema_id + schema_manifest_hash + verified_coverage_proof + owner_set_hash` 生成相同 topology/content hash；
9. Graph 内容使用跨 Go/Python 一致的 Canonical JSON；
10. Compiler 不创建 Identity、Scene、Claim、Asset、Plan、Binding 或 Owner Ref，不通过 Evidence overlap 猜测正式 Anchor。

## Compiler 时序与唯一 Writer

```text
Candidate
→ Backend validation / ReviewDecision
→ corresponding Owner Apply commits immutable Owner Version
→ Backend freezes verified Coverage Proof + exact Owner Collections
→ production/storygraph Compiler validates and projects
→ StoryGraphVersion insert + StoryGraphHead CAS
→ committed event / Story Lens / search projection
```

Compiler 只能保留/引用 Owner 已发布的 participant、anchor、scope、state 和 Evidence。Evidence 区间重叠最多用于机械一致性校验或产生 Review Issue，不能成为 Compiler 发布新语义事实的依据。

Graph Version 与 Head 仍为不可变 Version + 单 Project 线性指针。具体 GORM 字段、事务锁、Command/Receipt、HTTP 路径和重试协议留给 `VP-D09/VP-D14`；架构上必须保证 Version 插入、Head CAS 与 Outbox 事件原子收敛，失败不得留下孤立 Version 或分叉 Head。

## Reference Plan、基础资产与组合 Binding

Reference 顺序不再依赖 Storyboard Intent：

```text
Confirmed Production World
→ Gate 3 / ApprovedReferencePlanVersion
→ Character identity anchor candidate + Human Selection
→ identity anchor AssetVersion
→ anchor-bound Character appearance variants
  + Location boards + Prop sheets
→ Human Selection → base AssetVersion
→ exact base AssetVersion + Scene/Occurrence/InteractionClaim
→ Scene/Interaction Composition Candidate + Human Selection
→ SceneReferenceBindingVersion / InteractionReferenceBindingVersion
→ Gate 4
```

人物身份锚点必须先选中；每个形象变体通过 `materializes(binding_role=identity_anchor)` 精确依赖该锚点，只能改变已声明的 AssetState 内容。角色三视图、形象变体、地点板和道具板使用不同 `asset_version.purpose`，不能用一个自由文本参考图类型混用。

`ReferencePlanTarget` 只表达上文冻结的 `target_kind`、`fulfillment`、目标 OwnerNodeRef、覆盖范围和 Target 依赖，不包含 Provider、价格、路由或运行时 Prompt。精确 GenerationTarget 联合类型留给 `VP-D10`。

Scene Binding 必须绑定精确 Scene、相关 Occurrence、Location/Character/Prop AssetVersion 和已选 Scene Composition Artifact。Interaction Binding 必须绑定 `claim_type=interaction` 的 ContinuityClaim、精确 Character/Prop AssetVersion 和已选 Interaction Composition Artifact。二者都不能借用 `ProductionBinding`、`ShotProductionBindingVersion` 或组合 AssetState。

## SceneProductionPacket

Gate 4 后，系统从与 Reference 结果一致的 v2 p2 VerifiedCoverageProof 与 exact Owner Collections 编译精确 StoryGraphVersion，再由 Backend typed Query 构建 `SceneProductionPacket`。

Packet 是内容定址、只读 Query View，不是 Node、Edge、Owner Record 或 Compiler 输入。它必须使用以下 `additionalProperties=false` 合同独立冻结：

```text
ArtifactProvenanceRef {
  artifact_ref: OwnerNodeRef,
  rights_content_hash: lowercase SHA-256,
  lineage_content_hash: lowercase SHA-256
}

SceneProductionPacket {
  packet_contract_id: "scene-production-packet-v1",
  workspace_id: UUID,
  project_id: UUID,
  scene_scope_key: ScopeKey,
  storygraph_version_id: UUID,
  storygraph_schema_id: "storygraph-v2",
  storygraph_schema_manifest_hash: lowercase SHA-256,
  storygraph_content_hash: lowercase SHA-256,
  storygraph_owner_set_hash: lowercase SHA-256,
  storygraph_coverage_scope_manifest_hash: lowercase SHA-256,
  scene_ref: OwnerNodeRef,
  beat_refs: OwnerNodeRef[],
  dialogue_refs: OwnerNodeRef[],
  occurrence_refs: OwnerNodeRef[],
  asset_state_refs: OwnerNodeRef[],
  claim_refs: OwnerNodeRef[],
  effective_style_snapshot_ref: OwnerNodeRef,
  effective_policy_snapshot_ref: OwnerNodeRef,
  approved_reference_plan_version_ref: OwnerNodeRef,
  character_asset_version_refs: OwnerNodeRef[],
  location_asset_version_ref: OwnerNodeRef,
  prop_asset_version_refs: OwnerNodeRef[],
  scene_reference_binding_ref: OwnerNodeRef | null,
  interaction_reference_binding_refs: OwnerNodeRef[],
  evidence_refs: EvidenceRef[],
  artifact_provenance_refs: ArtifactProvenanceRef[],
  packet_content_hash: lowercase SHA-256
}
```

所有 OwnerNodeRef 数组按本 Design 的 OwnerNodeRef 合同键排序去重，`evidence_refs[]` 按 EvidenceRef 合同排序，`artifact_provenance_refs[]` 按内部 `artifact_ref` 合同键排序去重。`packet_content_hash = SHA-256(Canonical JSON(SceneProductionPacket 除 packet_content_hash))`。每个 Provenance Ref 必须指向 Packet 所引用 AssetVersion/Binding 精确选中的 Artifact，其 Rights/Lineage Hash 必须与 Artifact Owner typed Query 一致；不复制可变权利文本。

Packet 只能为 `scene_scope_key ∈ p2_scope_keys` 构建，且其 Scene 必须精确等于 `scene_ref`。Beat/Dialogue/Occurrence/State/Claim 是该 Scene 的完整 Owner 闭包；Style/Policy/Plan 恰各1个。基础 AssetVersion 集合必须满足该 Scope 的所有 active required Target；`scene_reference_binding_ref` 只有在对应 Target 为 `required` 或已选 `optional` 时非空，`not_generated`/未选 optional 时为 `null`；Interaction Binding 数组按每个 active Target 的同样规则精确等价。

即使 v2 已投影 Reference，Packet 仍必须独立冻结精确 Binding Ref，并校验其与 Graph OwnerRef 一致；不能只保存 Graph Head、Packet ID 或从 Graph/Asset Catalog 搜索“最新”。这不是第二 Writer，而是 Storyboard Invocation 的重放身份。Storyboard Agent/Owner 只消费被冻结的 Packet；Packet 完整对象和 hash 随 Invocation 保存。

## Storyboard、Guided Studio 与 Story Lens

Storyboard 从 P3 开始，只消费 SceneProductionPacket。正式 Shot 与 ShotProductionBindingVersion 在 Gate 5 Owner Apply 后才能进入下一份 v2 p3 Graph；Shot Draft/Intent Candidate 不进图，也不能反向启动基础参考资产生成。

Guided Studio 通过 Backend typed Query 完成五个 Gate、VisualProductionPackageView、Reference coverage 和 Scene readiness，不依赖用户理解 Graph。Story Lens 作为保留平台能力可读取 v1/v2 Version、Diff、Trace 与按 Scene/Episode 的有界子图，但不是当前 MVP 完成门。

Project 是 Query scope/root，不是 Graph Node。Outline 从 Project Query scope 展示 Episode；Action 是 Dialogue/Beat payload；Conflict、Goal、Turning Point 由严格 CausalClaim、StoryArc 或 PlotThread 表达，不创建未列入 Schema 的节点。

当前只保留只读 Query 意图：

- Get current/specified StoryGraph Version；
- Get bounded Story Lens；
- Trace upstream/downstream；
- Diff v1/v2 Versions；
- Get Graph stale/coverage/schema metadata。

`ValidateStoryGraphCandidate`、`ProposeStoryGraphPatch`、`ApplyStoryGraphPatch` 和 Canvas Domain Intent 属于未来 Canvas 设计，不进入当前 Compiler 或 VP-D05 完成门。

## 状态与失败路径

StoryGraphVersion 只有不可变 `published` 状态；旧版本不原地改写。必须显式处理：

- schema-scoped Owner Ref 缺失、重复或漂移：拒绝编译并返回 stale/conflict；
- expected Graph Head 漂移：CAS 失败，不静默换绑 Owner Set；
- Candidate/Review/Provider/Workflow 数据混入：拒绝 Schema 输入；
- Interaction participant/anchor/state/predicate 不完整或 Payload/Edge 不一致：拒绝发布；
- Reference Target 依赖成环、结果类型错误或不满足 per-target scope-aware 基数：只拒绝受影响 Scope；已进入 `p2_scope_keys` 的 required Target 无结果时该 Scene 的 Gate 4 不通过；
- 形象变体未绑定身份锚点、跨身份/State/Style 或修改未声明内容：AssetVersion/Graph 均不得发布；
- Scene/Interaction Binding 缺基础版本、选择未固化、Artifact 非 READY 或 Rights/Lineage 不完整：拒绝发布；
- DAG 成环：返回最小可解释环路径，不保存 Version；
- v2 编译未知：按同一 idempotency/owner-set/head identity 查询结果，不创建第二版本；
- SceneProductionPacket 与 Graph/Binding OwnerRef 不一致：Scene 不得标记 production-ready；
- Canvas 或 Agent 尝试直接写 Graph JSON/业务表：权限与架构测试拒绝。

## P0–P4 激活与验收锚点

| 阶段 | StoryGraph/产品闭环 | 本阶段不要求 |
|---|---|---|
| P0 Text Production Bible | style-blind Scene/Identity Candidate、前两个 Gate、正式 Production World；可发布 v2 p0 Graph，不生成图片 | Preset/Reference、Storyboard、媒体 Provider |
| P1 Visual Identity | Gate 3、Approved Plan、身份锚点、多形象/地点/道具 AssetVersion；发布 v2 p1 Graph | Scene/Interaction Composition、多 Provider 广度 |
| P2 Composition | Scene/Interaction Binding、Gate 4、v2 p2 Graph 与 SceneProductionPacket | Storyboard、视频/声音/成片 |
| P3 Storyboard | Packet → Storyboard/Gate 5，正式 Shot/ShotProductionBinding 后发布 v2 p3 Graph | Shot 图片/视频结果 Binding |
| P4 Full-script Hardening | 完整剧本分片、恢复、影响闭包、stale 防消费、质量基线和 Guided Studio 收敛 | Canvas、Platform Complete 媒体广度 |

当前 MVP 只要求一条真实图片参考资产路径。Shot frame/video、Seedance、FFprobe、视频/声音/渲染、四 Provider 广度和 Canvas 只保留 Platform Complete 历史方向，不抵扣 P0–P4。

## 设计完成边界

本文固定 `storygraph-v2` 的逻辑 Schema Manifest、Owner Input Manifest、Node/Edge/Payload allowlist、Interaction/Reference 约束、v1→v2 线性升级、Compiler 相对时序和 SceneProductionPacket 交接。

本文不授权代码、GORM、OpenAPI、Agent Wire、GenerationTarget 或 Human Gate 状态机变更。`VP-D05` 已经三路独立反例评审通过并按用户的后续自动实施授权接受；下一步只进入 `VP-D06`，当前唯一顺序以[文档中心的视觉生产重排队列](../README.md#当前视觉生产重排推进顺序)为准。
