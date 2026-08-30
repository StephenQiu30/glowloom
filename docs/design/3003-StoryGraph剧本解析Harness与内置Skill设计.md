# StoryGraph 剧本解析 Harness 与内置 Skill 设计

- 状态：已接受设计
- 历史事实：旧版曾于 `SG-D02` 接受并完成单 `build-storygraph` Bundle、Candidate Revision 与 Review/Repair 切片；其 Bible-first / Storyboard-first Stage 顺序由本次设计取代
- 接受记录：`VP-D07`（2026-08-30）；剧本/视觉生产 DAG、Skill Release 供应链与 Wire/Schema/恢复三路独立评审均通过（最终正文评审 SHA-256 `5df594553f66f64df0608fa7fe6d580f052ac65802e61410d3e804ec2dd7eac9`）
- 已接受规范前置：[系统总体架构](0003-系统总体架构.md) · [领域语言与模块命名规范](0006-领域语言与模块命名规范.md) · [StoryGraph 内容图与 DAG 创作画布设计](0010-StoryGraph内容图与DAG创作画布设计.md) · [剧本视觉生产工作台与世界观预设设计](0011-剧本视觉生产工作台与世界观预设设计.md) · [项目制作圣经生成执行框架设计](3001-项目制作圣经生成执行框架设计.md)
- 下一设计门：[本地 Codex 分镜智能体执行框架设计](3002-本地-Codex-分镜智能体执行框架设计.md)（`VP-D08`）
- 历史 Agent Requirement：[StoryGraph 剧本解析 Harness 与内置 Skill 需求规格](../requirement/3003-StoryGraph剧本解析Harness与内置Skill需求规格.md)；继续冻结，待 `VP-D14` 重写

## 结论

Lanverse 保留一个项目内置、版本化、不可变的 `build-storygraph` Skill Release，但彻底重排它的 Stage DAG：先 Scene Span 与 style-blind Scene Fact，再全剧 Identity Resolution；Gate 1 后构建 Production World，Gate 2 后才允许 Preset/Visual Foundation、参考资产、视觉审查和 Storyboard。

```text
Backend source acceptance / SourceSpanIndexVersion
→ propose_script_spans
→ extract_scene_facts
→ resolve_identities
→ review_candidate(profile=structure_identity)
→ Gate 1（非 Agent Stage）
   ├─ production/project checkpoint
   └─ StructureIdentitySetVersion
→ derive_production_entities
→ bind_scene_occurrences
→ reconcile_interaction_continuity
→ Backend assemble ProductionWorldCandidate
→ review_candidate(profile=production_world)
→ repair_candidate（有界、必要时）
→ Gate 2（非 Agent Stage）
   └─ Bible + Planning + Asset Owner Apply
→ resolve_visual_foundation
→ plan_reference_assets
→ review_candidate(profile=visual_foundation_scope)
→ Gate 3（非 Agent Stage）
→ compile_reference_brief（先基础 Target，后组合 Target）
→ Backend Generation / Artifact READY
→ review_reference_artifact
→ Human CandidateSelection / Gate 4（非 Agent Stage）
→ storygraph-production + SceneProductionPacket
→ direct_storyboard
→ review_candidate(profile=storyboard)
→ Gate 5（非 Agent Stage）
```

`storygraph-stage-wire-production` 是全新、严格判别的 Wire；它不兼容复用旧 Stage payload。旧 legacy Invocation 只由精确旧 runtime image/bundle hash 完成历史重放，新任务在切换点后只能创建 production Invocation，不双写、不 fallback、不把旧 Stage 名伪装成新语义。

Agent 永远只返回 Candidate、ReviewIssue 或 CandidateRepairPatch。Human Gate、CandidateSelection、Owner Apply、Provider 调用、Artifact 发布、StoryGraph 编译和 SceneProductionPacket 构建都属于 Go Backend 或对应业务 Owner，不是 Skill Stage。

## 1. 问题、范围与非目标

旧 Harness 已实现单 Bundle、严格结构化输出、不可变 Candidate Revision 和局部 Review/Repair，但业务顺序仍是 `analyze_story → Bible Confirm/Asset 物化 → segment/analyze_episode → draft_storyboard → needs_asset`。这会让场景事实无法纠正全局身份，让 Storyboard 反向启动参考资产，也没有 Preset、Reference Plan、Interaction Composition 和 Vision Review 的真实输入合同。

本文固定：

- `storygraph-stage-wire-production` 的 Release、Envelope、Stage 联合类型与 Hash 身份；
- 从剧本结构到 Gate 5 的 Agent Stage DAG，以及每个 Stage 的 runtime class、输入、Candidate 和分片边界；
- P0 Candidate 与 `3001` 的 Span、SceneFact、Identity、ProductionWorld 三分区一一映射；
- Preset 与 style-blind Stage 的强隔离、世界观预设的视觉阶段注入规则；
- 角色三视图、形象变体、地点板、道具板、Scene/Interaction Composition 的 Brief 和 Vision Review Skill；
- 外部成熟 Skill 的设计期吸收、许可证/来源、Eval、Shadow 与不可变发布流程；
- Candidate Revision、Repair、Attempt、unknown、分片与局部恢复语义。

本文不定义五个 Gate 的 HumanTask/Command 记录、五个 P0 Owner 的数据库模型、媒体 Provider API、资产选择写入或 Storyboard 字段细节；分别由 `VP-D09`–`VP-D11` 和 `VP-D08` 固定。本文接受不表示当前 Go/Python Wire、Bundle 或数据库已经修改。

## 2. 当前代码事实与原子切换面

当前仓库事实是：

- 唯一入口已位于 `agent/skills/build-storygraph/SKILL.md`，根目录旧 Skill 路径已退出运行时；
- Bundle 现有九个 Reference，内容主要覆盖 source evidence、旧 story analysis、episode analysis、Storyboard 与 continuity review；
- Python Registry、Pydantic Schema、Go AgentDefinition、Wire validator、GORM Stage check、fixture 与测试共同硬编码旧 Stage 集；
- 当前 runtime class 只有结构化文本，没有内容定址媒体输入和 Vision Review；
- `StageCandidateRevision` 的 invocation/aggregate/repair 三 origin、Head CAS、Shard Manifest、staleness 和 Bible review loop 已有可复用实现；
- 旧 `draft_storyboard`、Generation reference target builder 和 Storyboard Workflow 仍以 `needs_asset` 为旧顺序接口；
- 当前工作区另有暂停、未提交的 `SG-I21` 增量，本设计不修改、不回滚，也不把它算作 production 完成证据。

实现切换必须同时覆盖下列真实消费者，不能只改 Markdown：

```text
Backend AgentDefinition / Wire validator / database Stage allowlist
Python Invocation schema / Candidate schema / Registry / Harness
build-storygraph SKILL.md / References / Release hash
WorkflowDefinition node contracts / stage callers
canonical fixtures / unit / contract / integration / golden tests
```

旧 legacy 的以下 Stage 不进入 production Definition：`extract_source_evidence`、`analyze_story`、`reconcile_story`、`segment_episodes`、`analyze_episode`、`reconcile_episode`、`draft_storyboard`、`detail_shots`、`review_storygraph` 及旧 `repair_candidate` payload。历史数据保持可读；新 production 不建别名或双读入口。

## 3. 一个固定 Skill Release，七个能力路由

运行时仍只有一个显式入口：

```text
agent/skills/build-storygraph/
├── SKILL.md
├── references/             # 领域事实、不变量与边界
│   ├── source-structure.md
│   ├── scene-facts.md
│   ├── identity-resolution.md
│   ├── production-world.md
│   ├── scene-continuity.md
│   ├── visual-foundation.md
│   └── storyboard-direction.md
├── recipes/                # 精确 Target 到 Brief 的 typed 过程
│   ├── reference-plan.md
│   └── reference-design.md
├── rubrics/                # 与生成 Recipe 独立的审查标准
│   ├── production-review.md
│   ├── vision-review.md
│   └── candidate-repair.md
└── eval/failure-cases/      # 仅离线评测，运行 Prompt 不注入
```

目录只在实现任务有真实 Stage 消费者时创建；不预建空目录、README、示例或未使用 Recipe。`SKILL.md` 保持短小，只承载所有 Stage 共用的 candidate-only、来源、权限和路由规则；条件性领域规则放入对应 Reference，并且 Harness 每次只注入 Stage manifest 明确列出的文件。此结构遵循 progressive disclosure，不递归拼接全部 Markdown。

一个 Release 含七个逻辑能力，不是七个可被 Codex 自动发现的平行 Skill：

| capability key | 负责什么 | 映射 Stage |
|---|---|---|
| `parse-script-structure` | 原稿结构、Scene Fact、原始 mention | `propose_script_spans`、`extract_scene_facts` |
| `build-production-bible` | 全剧身份、Specification/State/World、三 Owner 候选 | `resolve_identities`、`derive_production_entities`、`bind_scene_occurrences` |
| `map-scene-continuity` | Interaction、普通 Continuity、story-time ledger | `reconcile_interaction_continuity` |
| `resolve-visual-foundation` | Preset/typed overrides、世界冲突、Design Gap、Reference Plan | `resolve_visual_foundation`、`plan_reference_assets` |
| `design-reference-assets` | 六类 Target 的 purpose-specific Visual Brief | `compile_reference_brief` |
| `review-production` | 文本候选审查/修复与五类视觉审查建议 | `review_candidate`、`repair_candidate`、`review_reference_artifact` |
| `direct-storyboard` | 从完整 SceneProductionPacket 生成分镜候选 | `direct_storyboard` |

产品可以把 Preset 展示为“风格 Skill 卡”，但运行时不能为都市、仙侠、黏土、赛博等风格复制该 Release。风格差异来自 Backend-owned `PresetVersion`、purpose profile 和 Effective Snapshot；同一个 Release 对不同冻结输入执行。

本 Bundle 由 Harness 显式加载，不使用 Codex 用户目录自动发现，因此不新增无消费者的 `agents/openai.yaml`。未来若真实 UI 需要自动发现，再作为独立设计修改，而不是在本任务预建。

## 4. 外部成熟 Skill 的吸收与供应链

外部 Skill、Prompt、教程或 LibTV 行为只能在设计/发布期被研究，不能在生产 Invocation 中下载、安装、搜索或执行。

```text
source inventory
→ license / provenance / prompt-injection review
→ classify: reference knowledge | typed recipe | review rubric | failure fixture
→ rewrite into Lanverse domain language and schemas
→ file-level copied-vs-rewritten mapping
→ golden + adversarial eval
→ isolated shadow run
→ independent review + signed release acceptance
→ immutable Release/Stage hashes
```

每个来源记录：source URI/repository、精确 commit/tag/content digest、作者、SPDX/license、NOTICE 要求、获取日期、允许用途、引用文件映射和 `project_owned|compatible_copy|concept_rewrite`。未知、冲突或不允许再分发的许可证必须隔离；只吸收概念并重写，不能先做“原字节迁移”再补许可证。只有项目自有或明确兼容许可内容可保持字节复用。

吸收后的四类产物边界：

- Reference：改变模型领域判断的非通用知识，不复制 Schema 或 Backend Policy；
- Recipe：按 Target purpose 编译 Brief 的 typed 决策规则，不含 Provider 密钥、模型 endpoint 或 Preset 实例值；
- Rubric：结构、身份/状态、风格、跨资产、交互/接触五类审查标准，不拥有发布决定；
- Failure fixture：身份漂移、三视图缺面、地点人物污染、道具手污染、双 holder、错误接触等可执行反例。

Release provenance manifest 与 NOTICE digest 进入 `skill_release_hash` 前像。任何来源、许可证结论、Reference、Recipe、Rubric 或 failure fixture 变化都创建新 Release；不覆盖历史字节。Golden/adversarial Eval 输入根、结果根、Shadow 运行证据、隔离/注入扫描结果、构建证明、审批人与发布签名都必须内容定址；失败用例只进 Eval，不进生产 Prompt。LibTV 未公开的编排不成为 Lanverse Stage/Wire 合同；若未来接入，只能作为 Generation Provider Adapter，结果仍进入 Artifact/QC/Human Selection。

## 5. Release、Stage 与资源身份

Backend `AgentDefinitionCoreManifest` 是允许 Stage/Policy 的唯一 pre-release Owner。发布身份必须是无环的内容 DAG，不能让签名又参与被签名 Hash：

```text
StageVariantKeyProduction {
  stage_key
  profile_key
}

AgentDefinitionCoreManifest {
  definition_core_id / definition_core_version
  wire_schema_id / wire_schema_hash
  variant_contracts[] {
    variant_key: StageVariantKeyProduction
    capability_key / lane / runtime_class
    input/output/normalized-candidate contract refs and schema hashes
    normalizer contract ref/hash
    patch-application = none | contract ref/hash
    execution/model/tool policy constraint hashes
  }
  agent_definition_core_hash
}

BundleContentManifest {
  bundle_entrypoint
  bundle_file_manifest[]
  provenance_manifest_hash / notice_hash
  isolation_scan_hash
  bundle_content_hash
}

StageReleaseManifest {
  wire_schema_id = storygraph-stage-wire-production / wire_schema_hash
  agent_definition_core_hash
  variant_key: StageVariantKeyProduction
  capability_key
  lane = style_blind_text | preset_visual | packet_storyboard
  runtime_class = text | vision
  bundle_content_hash
  input_contract_id / input_schema_hash
  output_contract_id / output_schema_hash
  normalized_candidate_contract_id / normalized_candidate_schema_hash
  normalizer_contract_ref / normalizer_contract_hash
  patch_application = none | contract ref/hash
  prompt_compiler_ref / prompt_compiler_hash
  reference_refs[] / recipe_refs[] / rubric_refs[]
  loaded_resource_set_hash
  execution_policy_ref / execution_policy_hash
  model_policy_ref / model_policy_hash / tool_policy
  runtime_image_digest
  stage_release_hash
}

CandidateStageSetManifest {
  agent_definition_core_hash / bundle_content_hash
  ordered_stage_releases[] { variant_key: StageVariantKeyProduction, stage_release_hash }
  policy_validation_contract_hash / policy_validation_proof_root
  candidate_stage_set_hash
}

EvalAttestation {
  eval_contract_id / fixture_set_root / candidate_stage_set_hash
  coverage_contract_hash / expected_variant_set_root / actual_binding_set_root
  case_bindings[] {
    fixture_id / fixture_hash / variant_key / stage_release_hash
    assertion_contract_hash / expected_root
  }
  evaluator_harness_hash / evaluator_runtime_image_digest
  model_policy_ref / model_policy_hash / sampling_policy_hash
  baseline = none | prior_approved_release_ref
  metric_contract_hash / threshold_set_hash / judgement_policy_hash
  case_observed_root / aggregate_observed_root / decision
  eval_attestation_hash
}

ShadowAttestation {
  shadow_contract_id / cohort_definition_hash / input_root / candidate_stage_set_hash
  coverage_contract_hash / expected_variant_set_root / actual_binding_set_root
  stage_variant_refs[] / evaluator_harness_hash / runtime_image_digest
  model_policy_hash / sampling_policy_hash
  baseline = none | prior_approved_release_ref
  metric_contract_hash / threshold_set_hash / judgement_policy_hash
  observed_result_root / diff_root / decision
  shadow_attestation_hash
}

SkillReleaseManifest {
  release_id / release_version
  predecessor = none | prior_approved_release_ref
  agent_definition_core_hash
  bundle_content_hash / candidate_stage_set_hash
  eval_attestation_hash / shadow_attestation_hash / build_attestation_hash
  skill_release_hash
}

SignatureEnvelope {
  release_id / skill_release_hash
  signature_algorithm / key_id / signed_at
  approval_policy_id / approval_policy_hash / reviewer_ids[]
  signed_root = skill_release_hash
  signature / signature_envelope_hash
}

SkillReleaseControlRecord {
  release_id / skill_release_hash / agent_definition_core_hash
  control_revision / previous_control_hash / release_fence
  status = approved | deprecated | quarantined | revoked
  reason_code / decided_at / decided_by
  signature_envelope_hash / decision_evidence_hash
  control_hash
}

SkillReleaseControlHead {
  release_id / control_revision / control_hash / release_fence
}
```

所有对象 `additionalProperties=false`。除签名前像另行声明外，每个 `*_hash` 都对带 Contract domain separator、排除自身 hash 字段的完整 Canonical JSON 根计算；`signature_envelope_hash` 包含 signature 但排除自身。资源 Ref 至少含 bundle-relative path、byte length 和 lowercase SHA-256，按 UTF-8 path 排序且不可重复。`bundle_file_manifest[]` 覆盖 Release 允许的全部文本资源；`bundle_content_hash` 覆盖除自身外的完整 Bundle manifest。`loaded_resource_set_hash` 只覆盖该 Stage Variant 真正注入的入口、Reference、Recipe 与 Rubric；Failure fixture 不得进入该集合。

`StageVariantKeyProduction` 是唯一键对象，不再并存可分叉的 compound string 或外层重复字段。两个 key 都必须匹配 ASCII `^[a-z][a-z0-9_]{0,63}$`，无 profile 的 Stage 显式使用 `profile_key=default`。任何 Stage Set、Eval/Shadow binding、Invocation 或 Candidate Ref 分支中出现的 Variant Key 都必须与解引用后 `StageReleaseManifest.variant_key` 的 Canonical JSON bytes 完全相等；列表统一按 `(stage_key UTF-8 bytes, profile_key UTF-8 bytes)` 二元组排序。`review_candidate` 和 `repair_candidate` 的 structure/production-world/visual/storyboard profile 因而是不同 Stage Variant，分别冻结 lane、Schema、Rubric 和资源集；P0 Variant 无法加载视觉 Rubric。`stage_release_hash` 对除自身外的完整 Stage manifest 计算，其中已绑定 `bundle_content_hash` 与 `agent_definition_core_hash`。

`AgentDefinitionCoreManifest` 只能引用在任何 Release 之前独立存在的 Wire/Schema/Contract/Policy artifact；Schema 机械拒绝 StageRelease、SkillRelease、Signature、Control 或运行 activation ref。发布后的 Definition → Release 激活绑定只存在 Backend Control/Registry 中，不回写 Core。

`CandidateStageSetManifest` 是被评测与最终发布共用的唯一 Stage 集合根。对每个 ordered row，验证器必须解引用 StageRelease 并证明：其 `wire_schema_id/hash` 与 Core 顶层 Wire 身份逐字节相等；`agent_definition_core_hash` 与 `bundle_content_hash` 分别等于 Stage Set 的根；Variant Key 与 Core row 逐字节相等；capability/lane/runtime/input/output/normalized-candidate/normalizer/patch-application contract 与 Core row 完全相等；execution/model/tool policy 通过 Core 冻结的 policy validator 和 constraints。非 repair Variant 的 patch application 必须是 canonical `none`，repair Variant 则必须是与 target profile 一致的精确 Backend patch contract。`policy_validation_proof_root` 覆盖每个 row 的输入、判定和 validator hash。Stage Set 数量与键集必须等于 Core 完整集合，不可重复、缺少或在批准后追加；任一根/行不等价都不产生 `candidate_stage_set_hash`。

Eval/Shadow 不能只保存“通过”文本。两者都必须绑定同一 `candidate_stage_set_hash`；每个 case/binding 的 Variant Key + StageRelease Hash 必须是该 Stage Set 精确成员。Coverage contract 从 Core/Stage Set 和 fixture/cohort 机械派生应测 Variant 集，`expected_variant_set_root` 必须与 actual bindings 派生的 `actual_binding_set_root` 全等；不允许以同 key 的另一 Stage hash 替换或跳过应测 Variant。Attestation 还必须冻结 evaluator/runtime/model/sampling、baseline、指标、阈值、判定策略和观测根。Baseline 是严格联合：无 baseline 只能写 canonical `{kind:none}`；有 baseline 则写 `{kind:prior_approved_release, release_id, skill_release_hash, signature_envelope_hash, approved_control_hash}`。后者必须在 Eval/Shadow 开始前已 approved，且是候选 Release `predecessor` 链上严格先祖；self/current/future/descendant 或断链 Ref 全部拒绝。Genesis 或批准策略明确无可比基线时才允许 `none`。

`SkillReleaseManifest.candidate_stage_set_hash` 必须逐字节等于 Eval/Shadow 两个 Attestation 的 Stage Set root，且其 Core/Bundle roots 必须等于 Release 自身的 Core/Bundle refs。`predecessor` 使用与 baseline 相同的 `none|prior_approved_release_ref` 联合，发布验证器必须追溯到 genesis 并拒绝循环。`skill_release_hash` 再覆盖除自身外的完整 Release manifest。

`SignatureEnvelope` 在 Release Root 之后创建；签名算法、key id、批准策略和审阅人都被冻结，签名前像是带 domain separator 的 `skill_release_hash`，因而无 hash/signature 环。构建证明必须绑定 Bundle root、Candidate Stage Set root 及其全部 Stage Release roots、Schema/compiler/toolchain 与 runtime image digests。

`SkillReleaseControlRecord` 是不进 Release Hash 的 Backend-owned 不可变追加决策；每个 Release 只有一个 `SkillReleaseControlHead`，更新必须以 expected revision/hash CAS 从前一 Head 追加新 Record。无 Control 的 Release 不可执行；首个 `approved` 必须引用有效 SignatureEnvelope。允许转移只有 `approved→deprecated|quarantined|revoked`、`deprecated→approved|quarantined|revoked`、`quarantined→approved|revoked`；`revoked` 是终态，quarantine 后再批准必须携带新安全/许可证决定证据。`release_fence` 只在进入 `quarantined|revoked` 时单调递增，后续再批准也不回退；普通 deprecated/approved 切换不无故 fence 运行中恢复。

Control 终态复验和写入权只属于 Go Backend，不属于无数据库权限的 Harness。Backend 在 Invocation 创建、Attempt `queued→dispatched`、accepted Outcome CAS 与任何 invocation/aggregate/repair Candidate Revision 创建、以及任何 Candidate Apply/reuse 的状态写事务中，都必须先按统一锁序锁定 current `SkillReleaseControlHead`，并将 expected `control_hash + release_fence` 作为同一条件写/CAS 的谓词；Control 转移事务也锁同一 Head，从而不存在 check-before-write TOCTOU。Invocation-origin Revision 必须与 accepted Outcome CAS 在同一事务创建；aggregate/repair 也不得绕过同一 Control 谓词。若 Apply 先持锁并提交，它是撤销前事实；若 revoke/quarantine 先提交，旧 Apply 必须条件失败。

Backend 完成 dispatch 状态写后签发内容定址 `DispatchAuthorizationProduction`，冻结 invocation/attempt/claim、Stage/Input/Release hashes、`control_hash_at_dispatch`、`release_fence`、期限和 Backend 签名。Harness 只验证该授权、执行冻结输入，并在 Result 原样回传 authorization hash/control/fence；它不查库、不改 Control、不决定 Outcome。只有 `approved` 可创建新 Invocation；`deprecated` 只允许没有 quarantine/revoke 中间祖先的已存 Invocation 精确恢复。Head 进入 `quarantined|revoked` 时立即递增 release fence，运行中 claim 的迟到结果只留审计，不得成为 accepted Outcome；既有但未 Apply 的 Candidate/Outcome 也不得继续 Apply/reuse。若 quarantine 后重新 approved，旧 Attempt/Outcome 仍不自动复活，必须在新 Control Hash 下建新 Invocation。撤销前已提交的 Owner 事实保持可审计有效，不做隐式回滚。任何状态都不允许 fallback 到相近 Release。

新 Release 不自动使已确认 Owner 事实 stale；只有用户/Workflow 显式以新 Release 重跑才产生新 Candidate lineage。

Pydantic 严格类型是 Agent Candidate JSON Schema 的代码事实；构建时生成 Canonical Schema artifact/hash并写入 Backend Manifest。Go 拥有 Invocation、Stage、Policy 和正式业务校验，必须用同一组跨语言 success/reject fixtures 验证等价，不能各自放宽字段。Bundle Markdown 不复制 JSON Schema。

## 6. `storygraph-stage-wire-production`

### 6.1 公共 Invocation Envelope

Wire 是按 `variant_key: StageVariantKeyProduction` 严格判别的联合类型，不再提供可自由组合的 `source_refs[]`、`base_storygraph_version_*` 或任意 `stage_input`：

```text
StoryGraphStageInvocationProduction {
  invocation_id
  kind = storygraph_stage
  wire_schema_id = storygraph-stage-wire-production
  workspace_id / project_id
  variant_key: StageVariantKeyProduction
  stage_release_ref {
    release_id / skill_release_hash
    stage_release_hash
    approved_control_hash_at_create / release_fence_at_create
  }
  scope { scope_kind, scope_key, impact_closure_hash }
  execution_policy
  source_inputs
  upstream_candidate_refs[]
  owner_input_refs[]
  shard_manifest_ref
  shard
  media_attachments[]
  stage_payload
  input_hash
}
```

Invocation 输入身份不包含后发 dispatch token。实际 transport 在外层携带 `StoryGraphStageDispatchProduction { invocation, attempt_id, claim_version, dispatch_authorization }`。`DispatchAuthorizationProduction` 先对 invocation/attempt/claim、input/stage/release hash、`control_hash_at_dispatch`、current `release_fence` 与过期时间计算 domain-separated unsigned root，Backend 只签该 root；再对含签名的完整 Envelope 计算 `dispatch_authorization_hash`（排除自身）。Token 不进 `input_hash`，但它的 hash 必须进 Attempt Result，因而无签名或输入循环。

Candidate 引用不得假设所有 Revision 都由 Agent Stage 直接生成：

```text
RevisionPointerProduction { revision_id / revision / revision_hash }

CandidateRevisionRefProduction {
  revision_id / revision / revision_hash
  normalized_candidate_contract_id / normalized_candidate_schema_hash
  candidate_content_hash
  origin = invocation | aggregate | repair
  producer_ref =
    invocation { variant_key, stage_instance_key, accepted_attempt_result_ref/hash }
    | aggregate { node_run_ref, aggregate_contract_id/hash,
                  shard_manifest_ref/hash, ordered_input_roots }
    | repair { repair_variant_key, repair_stage_instance_key,
               accepted_attempt_result_ref/hash,
               parent_revision_pointer, review_candidate_revision_pointer }
}
```

`producer_ref` 按 `origin` 严格判别且 `additionalProperties=false`。Invocation/repair 分支的 Variant Key 必须符合 Stage Release 成员校验；aggregate 分支只能引用 Backend-owned NodeRun/Aggregate Contract/Manifest，不得伪造 Variant 或 stage instance。三个分支都必须与解引用后 Revision 内的 producer union 逐字节相等。

`source_inputs`、`owner_input_refs`、`media_attachments` 和 `stage_payload` 的字段与基数由当前 Stage Variant 固定。上述仅是共用基座的概念超集；生成的 JSON Schema 使用 `oneOf` 展开每个分支，同时对 `variant_key.stage_key` 和 `variant_key.profile_key` 设置 `const`，且要求该 key 与 `stage_release_ref` 解引用后 Manifest 中的 key 逐字节相等。该 Stage Release 还必须是 `skill_release_hash` 解引用后 `candidate_stage_set_hash` 的精确成员，且 Core/Bundle roots 与 Release 完全相等。不存在 untyped map，某分支不允许的整个字段必须缺席。所有 Owner Ref 使用 `0010` 的完整 OwnerVersionRef/OwnerNodeRef 身份；所有 Candidate Ref 使用上述严格联合，并冻结 revision/content/producer 身份。任何 current/latest、裸 UUID、名称搜索或未给 hash 的引用都拒绝。

P0 style-blind Stage 的分支 Schema 根本不含 Preset、EffectiveStyle/ProductionPolicy、Artifact、AssetVersion 或 StoryGraph 字段；多传一个也因 `additionalProperties=false` 失败。通用 `execution_policy` 只是模型/资源/预算执行策略，不是生产风格 Policy。`direct_storyboard` 不读取通用 Owner 列表，只接收一个内容定址 `SceneProductionPacket` 及其冻结媒体附件。

媒体附件不是 URL 或对象存储凭据：Backend transport broker 根据精确 Artifact/Rendition Ref 把有限只读 bytes 注入隔离运行目录，Envelope 保存 attachment purpose、MIME、byte length、content digest、pixel width/height、page/frame count、rights/lineage ref、对应 Owner ref 和排序 key。Harness/runtime 重算 digest 并验证媒体元数据；不一致 fail closed。附件字节不直接进入 JSON Hash，但其完整 manifest/digest 进入 `input_hash`。

### 6.2 Hash 前像

所有 Hash 使用项目 Canonical JSON 与小写 SHA-256，不用字符串拼接或到达顺序：

```text
InvocationInputHashRoot {
  wire_schema_id
  workspace_id / project_id
  variant_key / stage_release_ref
  scope
  execution_policy
  complete stage-discriminated payload excluding invocation_id/input_hash
}

StageInstanceKeyRoot {
  identity_contract_id = storygraph-stage-instance-production
  variant_key
  scope_kind / scope_key
  shard_manifest_hash / shard_key / tree_path
  input_hash
}
```

`input_hash` 不包含 `invocation_id` 和自身；Stage instance key 对上面完整根计算。数组排序规则由 Input Contract 固定：Owner Ref 按完整逻辑 Ref tuple，Candidate Ref 按 `(origin rank: invocation<aggregate<repair, revision_id UTF-8 bytes, revision, revision_hash)`，附件按 purpose/sort key/digest，Issue/业务 key 按 stable key；Scene、Beat、story-time 等有业务顺序的数组保留显式 sequence key，并由 Schema 冻结唯一排序来源。

ShardManifest、Attempt Result、Candidate content、Candidate revision 与 Patch 各自有独立 Contract ID 和 Hash Root；不能只保存 hash 而丢失前像。Go/Python fixture 必须同时验证 Canonical JSON bytes、hash、字段拒绝和数组乱序拒绝。

### 6.3 模型输出与 Attempt Result

模型只生成当前 Stage Variant `output_schema_hash` 指定的 Candidate Body，不得自报 invocation/attempt/claim、status、executor、control 或 hash。Harness 完成严格 Schema 校验、canonicalization 与内容 Hash 后，才包装不可变 Attempt Result：

```text
StoryGraphStageAttemptResultProduction {
  invocation_id / attempt_id / claim_version / release_fence
  wire_schema_id / variant_key / stage_release_hash
  skill_release_hash / release_control_hash_at_dispatch
  dispatch_authorization_hash
  input_hash
  status = succeeded | failed | unknown
  output_contract_id / output_schema_hash
  output_content_hash / candidate
  diagnostics[]
  executor { runtime_class, runtime_image_digest, harness_hash, model_policy_ref }
  error { code, safe_summary, retry_class }
  result_hash
}
```

成功时严格 Candidate、output contract/schema/content hash 与 result hash 非空，error 为空；确定失败时 Candidate/content hash 为空且稳定 error 非空；unknown 时 Candidate/content hash 也为空，它表示该 Attempt 的执行效果无法确认，不是逻辑 Invocation 的终态。`diagnostics[]` 只允许有限的执行/解析诊断，业务 ReviewIssue 必须属于对应 Candidate，不允许在通用 Envelope 里建第二条语义通道。Result Hash 覆盖除自身外的完整对象。用户原文、Prompt、图片 bytes 和凭据不进入日志；日志只保存内容身份、Stage Variant、scope、耗时、token/attempt 计数和安全错误摘要。

## 7. production Stage 责任矩阵

非 Review/Repair Stage 使用 `profile_key=default`：Span 至 Continuity 属于 `style_blind_text`，Visual Foundation/Plan/Brief/Artifact Review 属于 `preset_visual`，Storyboard 属于 `packet_storyboard`。`review_candidate` 的 `structure_identity|production_world|visual_foundation_scope|storyboard` 与 `repair_candidate` 的各 target profile 都是 Manifest-bound `profile_key`，不是运行 payload 可任意切换的字符串。

| Stage | runtime / 分片 | 精确输入 | Candidate 输出 | 禁止输入或行为 |
|---|---|---|---|---|
| `propose_script_spans` | `text`；source window map + ordered reduce | DocumentRevision、SourceSpanIndexVersion、格式诊断、Backend marker hints | `script_structure_proposal_production`：EpisodeSpan/SceneSpan、coverage、边界 issue | Bible、Identity、Preset、Style、目标时长、正式 Episode/Scene ID |
| `extract_scene_facts` | `text`；每个 accepted/proposed SceneSpan | 精确 structure revision、Scene source slices、邻接只读上下文 | `scene_fact_candidate_production`：Dialogue/Beat/Mention/StateClue/InteractionFact | 正式 Identity/State、creative fill、Preset、StoryGraph |
| `resolve_identities` | `text`；mention component map + deterministic reduce | 完整 SceneFact root；增量时可带允许复用的精确旧 Structure identity index | `identity_resolution_candidate_production`：cluster/new/reuse/unresolved 与完整 mention partition | 同名自动合并、Agent UUID、Specification/State、Style |
| `derive_production_entities` | `text`；每 Identity/dependency closure | StructureIdentitySetVersion、内容定址 SceneFacts、Evidence candidates | `production_entity_fragment_candidate_production`：Specification、完整 State、World/跨集 Claim、DesignGap | raw mention 直绑、Preset、Artifact、Scene writer |
| `bind_scene_occurrences` | `text`；每 Scene scope | Structure version、SceneFact、entity/state fragments、稳定 Scene logical ID | `scene_binding_fragment_candidate_production`：Scene/Dialogue/Beat/Occurrence | 自建 Identity/State、mentioned_only 偷升实际 Occurrence |
| `reconcile_interaction_continuity` | `text`；story-time closure map/reduce | State、Occurrence、InteractionFact、相邻 ledger boundary、旧 Claim lineage | `continuity_fragment_candidate_production`：Interaction/普通 Continuity/ledger delta/issues | 自由持物备注、组合 State、双 holder、播放序替故事时间 |
| `review_candidate` | profile-bound `text`；impact closure | 精确目标 Candidate Revision、Backend deterministic gate issues、当前 Variant Rubric | `production_review_candidate_production`：排序 ReviewIssue 与建议 | 输出整体 Gate pass/fail、降级 mechanical blocker、Human Decision |
| `repair_candidate` | profile-bound `text`；单 issue/typed allowlist | 目标 revision/hash、review revision/hash、当前 Variant 允许 keys/fields、只读邻接 | `candidate_repair_patch_production` 严格联合 | 改 Evidence、stable ID、已确认 Owner Ref、Artifact bytes |
| `resolve_visual_foundation` | `vision`；Project/design-gap closure | Confirmed World roots、PresetVersion、typed overrides、合法参考附件 | `visual_foundation_candidate_production`：Style/Policy、冲突、source→design mapping、creative-fill 提案 | 改写 P0、未冻结网络参考、静默世界改编 |
| `plan_reference_assets` | `text`；Project scope + expected-key batches | Confirmed World、Foundation candidate、p1 scopes、Backend expected target keys | `reference_plan_candidate_production`：required/optional/not_generated、依赖和用途 | 从名称猜资产、漏 Target 表达不生成、直接调用 Provider |
| `compile_reference_brief` | `text`；每个 Approved Target | Gate 3 snapshots/plan、精确 Identity/Spec/State/Scene/Occurrence/Interaction、Target contract 要求的全部已选依赖（允许空集时必须显式） | `reference_brief_candidate_production` 六用途严格联合 | 缺少必需已选依赖、当前指针、额外人物/道具、生成图片 |
| `review_reference_artifact` | `vision`；每 Target/Candidate Artifact | Target/Brief、Artifact digest/rights/lineage、Style、精确基础依赖与只读 bytes | `vision_review_candidate_production` 五类审查结果 | CandidateSelection、发布 AssetVersion/Binding、修图或 Provider 调用 |
| `direct_storyboard` | `vision`；每 Scene | 一个精确 SceneProductionPacket 及其内容定址参考附件 | `storyboard_candidate_production`；精确字段由 `VP-D08` 固定 | `needs_asset`、搜索项目资产、触发参考生成、自由 current/latest |

`repair_candidate` 名称可保留，但 production payload/contract 与旧版完全不同；旧 reviewer/repairer 不能接受 production Candidate。视觉 Artifact 不可 Patch：审查失败后由用户拒绝、修改 Brief 或创建新的 Generation Candidate。

## 8. P0 Candidate 与 Backend 规范化

### 8.1 Script Structure

Agent 的 `script_structure_proposal_production` 只使用临时 span keys 和绝对 code-point ranges，不生成 UUID。Backend 在创建 normalized `script_structure_candidate_production` Revision 前，按 Stage Release 冻结的 normalization contract：

- 验证 Episode/Scene coverage、顺序、边界和 Evidence；
- 在 `production/planning` 拥有的 logical ID namespace 中确定性预留 `scene_owner_logical_id`；
- 写入 temporary span key → reserved logical ID 映射；
- 派生 `scene:<scene_owner_logical_id>` scope key；
- 将 normalizer contract id/hash、normalized candidate contract/schema hash、映射和 Agent Attempt Result ref/hash 一并纳入 Candidate Revision hash。

预留不发布正式 Scene、Owner Version 或 Receipt。Gate 1 冻结该映射；Gate 2 Planning Scene 必须逐字节复用。split/merge/delete 产生新 structure revision 和显式 scope rebase，不按标题或数组位置重算旧 ID。其他 Stage 也先以 output Schema 验证模型 Body，再由冻结 normalizer 产生 Revision 内容；当 normalizer 仅做 canonical sort/validation 时，两份内容可逐字节等价，但两个 Contract 身份仍不得隐式混用。

### 8.2 Scene Fact 与 Identity

SceneFact 必须 style-blind，使用 `RawMentionRef`、`StateClue` 和 `InteractionFact`；Candidate Evidence 保留 SourceSpanIndexVersion 与四元组，直到 Gate 2 才由 Bible Owner 物化正式 `source_evidence`。普通 upstream ref 不能替代 Evidence。

Identity 输出必须证明精确 SceneFact root 中每个 RawMentionRef 恰进入一个同 kind cluster 或 unresolved。`reuse` 只能命中输入白名单中的旧 identity key；Agent 不能分配 UUID。Backend deterministic gate 重算 mention universe、cluster membership、type/alias、coverage 和 canonical root。

### 8.3 Production World 三分区

Backend 从三个 Stage fragment 机械组装 `production_world_candidate_production`：

```text
bible partition
  Evidence / Specification / WorldRule / cross-episode Claim / Binding proposal
planning partition
  Scene / Dialogue / Beat / Occurrence / Causal & Continuity Claim
asset partition
  AssetIdentity mapping / complete AssetState timeline
shared proof
  expected business keys / cross-partition ref table
  scope coverage / DesignGap / ContinuityLedgerView / ReviewIssue
```

组装只使用 current Candidate Revision refs 和固定 comparator，不调用模型。任何 fragment 缺失、duplicate key、cross-ref 不可解析或 coverage 不闭合都不产生根 Candidate。

Interaction Candidate 精确镜像 `0010` 的状态机：typed predicate、actor/prop/counterparty Occurrence、Scene/Beat/story-time、holder before/after、PropState before/after、手别/接触/方向/比例、claim revision/supersedes。普通 Continuity 只允许 `state_persists|state_changes`。Ledger 必须机械拒绝双 holder、无因状态跳变、道具瞬移和 give/receive 双重应用。

Agent 只能提出 `needs_creator_decision` 或 DesignGap；不能构造 ContentAddressedAuditRef。Gate 2 用户决定由目标 Owner 内容定址并满足 `evidence XOR creator decision`。

## 9. 宏观 Workflow、Gate 与正式输出

### 9.1 P0 Text Production World

```text
source_revision_accepted Receipt
→ Structure Stage Node(s)
→ SceneFact Stage Node(s)
→ Identity Stage Node
→ deterministic gates + review_candidate
→ Human Gate 1
→ project_episode_lifecycle_confirmed Receipt
→ gate_1_structure_identity Receipt + StructureIdentitySetVersion
→ Production Entity / Scene Binding / Continuity Node(s)
→ deterministic aggregate + fixed-point reconcile
→ review_candidate / bounded repair
→ Human Gate 2
→ gate_2_bible_continuity per-Collection Receipts
→ Confirmed Production World typed view
→ optional p0 StoryGraph compile only when all required Collections exist
```

Human Gate 可审核成功 Candidate 中的 semantic blocker/unresolved。只有 Wire、Schema、Evidence 坐标、coverage、hash 或 reference 完整性等技术错误才使 Stage 无 Candidate 失败；语义歧义必须以成功 Candidate + scoped ReviewIssue 到达用户，不能因“有 blocker”而让 Node 永远到不了 Human Gate。

Gate 1 和 Gate 2 不是 Agent Stage。Gate 1 的 Project checkpoint 先独立提交，随后 Bible Structure Apply；Gate 2 由 Bible/Planning/Asset 三 Owner 原子协调，并按每个 Collection Root 各出 Receipt。Stage output 只携带精确 Candidate/Review refs；Receipt/Owner refs 由 Backend 返回，StoryNodeKey 只能由后续 Compiler 派生。

### 9.2 P1/P2 Visual Production

```text
Confirmed Production World
→ resolve_visual_foundation
→ plan_reference_assets
→ deterministic expected-target coverage + review_candidate
→ Gate 3 Owner Apply
→ base wave A（可并行）:
   Character identity-anchor Brief
   │  → Generation / READY / Vision Review / Human Selection
   │  → published identity-anchor AssetVersion
   Location board + Prop sheet Briefs
      → Generation / READY / Vision Review / Human Selection
→ base wave B:
   Character appearance Brief
   → 必须消费同 Identity 已选 identity-anchor AssetVersion
   → Generation / READY / Vision Review / Human Selection
→ base-selection barrier
→ composition wave:
   Scene Composition + Interaction Composition Briefs
   → 消费全部精确已选基础 AssetVersion
   → Generation / READY / Vision Review / Human Selection
→ Gate 4 complete
```

Backend 决定 Target expected business keys、两遍 Character anchor/appearance 算法、依赖 rank、required/optional/not_generated 和 Result 等价；Agent 只为精确 Target 编译 Brief。一个 Target 一个 Stage instance。Identity anchor、Location 和 Prop 的 dependency contract 可为显式空集；Appearance 必须依赖同 Identity 已选 anchor，组合 Target 必须依赖已选精确基础 AssetVersion。每个 Selection 都可局部发布依赖版本，但只有所有 required Binding 闭合才完成 Gate 4；`not_generated` 不创建 Brief 或 Provider Job。

### 9.3 P3 Storyboard

Gate 4 后 Backend 从一致 Owner set 编译 production p2 StoryGraph，再构建 `SceneProductionPacket`。`direct_storyboard` 只能读取该 Packet，不能把缺失参考变成 `needs_asset`，不能访问项目资产库或反向触发 Generation。其 Intent/Detail、review/repair 与 Gate 5 精确合同由 `VP-D08` 接着固定。

## 10. Style/Preset 强隔离

Stage allowlist 固定：

| 输入 | P0 span/fact/identity/world/continuity | Visual foundation/reference | Storyboard |
|---|---|---|---|
| Document/SourceSpan/Evidence | 必需或按 Stage 精确给定 | 只经 Confirmed World Ref 追溯 | 只经 Packet 追溯 |
| PresetVersion/typed override | 禁止 | 必需或按视觉 Stage 给定 | 只经 Effective Snapshot/Packet |
| EffectiveStyle/Policy | 禁止 | Gate 3 后必需 | Packet 必需 |
| Artifact/AssetVersion | 禁止 | 只在审查/组合 Target 给定 | Packet 精确给定 |
| StoryGraphVersion | 禁止 | 禁止作为事实输入 | 只允许 Packet 内绑定版本 |

Backend 在构造 input hash 前执行 style-isolation gate；P0 payload 或 attachment 中出现任何 Preset/Style/Artifact/StoryGraph 字段，返回 `style_input_forbidden`。切换 Preset 不改变 P0 Invocation/Candidate input hash，也不 stale SceneFact、Identity、Specification、State、Occurrence 或 Interaction。

`faithful` 模式不得改写已确认世界事实；`world_adaptation` 的每个 source fact → design mapping、语义不变量和影响 scope 都进入 Visual Foundation Candidate 并等待 Gate 3 决议。Preset capability manifest 不支持某 Target kind 时以 `preset_capability_missing` 阻塞，不使用通用 Prompt 静默回退。

## 11. Reference Brief 与 Vision Review

`reference_brief_candidate_production` 是按 `target_kind` 判别的严格联合：

| target kind | Brief 必须固定 | 关键禁止 |
|---|---|---|
| `character_identity_anchor` | Identity、Specification、Gate 3 选定 anchor State、front/profile/back、身份不变量 | 换装 State 重复造身份锚点 |
| `character_appearance` | 同一 Identity、anchor dependency、精确 State、三视图与可变槽位 | 改脸、体型、永久标记 |
| `location_board` | Location/Specification/State、拓扑、材质、尺度、空场策略 | 未要求人物污染 |
| `prop_sheet` | Prop/Specification/State、结构、比例、开闭/损坏/内容物 | 手、人、场景污染 |
| `scene_composition` | 精确 Scene closure、全部 Occurrence/Interaction、已选基础版本、构图目的 | 额外身份/道具、替代基础资产 |
| `interaction_composition` | 精确 InteractionClaim、participant Occurrence、手别/握点/方向/比例、已选基础版本 | 组合 AssetState、错误接触或 holder |

视觉 Stage 不输出自由 Prompt 即完成。Brief 至少含 target/ref/constraint identity、source-vs-design slots、positive/negative instructions、output view roles、layout/scale、dependency refs、rights/provenance requirements、QC rubric refs、content hash。Backend 再把 Brief 编译为 Provider-specific request；Provider Adapter 不是 Skill。

`vision_review_candidate_production` 对每项给 `pass|warn|fail|not_assessable`、证据区域/视图、issue code、置信度和建议，覆盖：

1. 结构/视图完整性；
2. Identity/State 一致性；
3. Style/Preset 合规性；
4. 跨资产一致性；
5. Interaction/接触正确性。

Vision Reviewer 不能发布、选择、修改 Artifact 或降低 Backend deterministic QC。只有 Human CandidateSelection 能发布 AssetVersion/Reference Binding；审查 `not_assessable` 不等于通过。

## 12. Shard、Coverage 与固定点

Backend 为每个可 fan-out NodeRun 发布不可变 `ShardManifestProduction`。Manifest Hash Root 至少覆盖 workflow/node、stage release、root input hash、candidate contract、partition rule、active/superseded shards、ordered reduce tree、coverage proof 和 parent manifest hash。

| Candidate/Stage | 分片身份与覆盖 |
|---|---|
| Structure | SourceSpanIndex code-point ranges；只读 overlap 不计 coverage |
| SceneFact | `scene:<reserved logical id>`；一个 Scene 的事实不可跨 shard 重复拥有 |
| Identity | RawMentionRef universe；connected component map + 全局 deterministic reduce；每 mention 恰一次 |
| Production Entity | Identity/dependency closure；共享 Claim 扩大闭包 |
| Scene Binding | stable Scene scope；expected SceneFact keys 完整 |
| Continuity | story-time range + ledger boundary snapshot；交互事件只由一个 shard 拥有 |
| Visual Foundation | DesignGap/world-conflict key range；Project root deterministic reduce |
| Reference Plan | Backend expected target business key range；Candidate key 集必须等价 |
| Brief/Visual Review | 一个 Target/Artifact Candidate；不做跨 Target 隐式搜索 |
| Storyboard | 一个 SceneProductionPacket；跨场连续性只读边界显式输入 |

Candidate Contract 为每个数组冻结 canonical item path、business key、排序、空集和 comparator；不再硬编码旧 `entities→world_entries→claims` 顺序。超预算只能发布新 Manifest 并确定性重分片；不能截断、扩大预算或把完整父输入交给子 shard 后靠 Prompt 忽略。

Production World 固定点每轮保存 round、input roots、changed keys、impact closure、ledger boundary、output revision/hash 和 issue root。相同 canonical root 即收敛；最大轮次仍变化时返回成功的 `non_convergent_reconciliation` semantic blocker 给 Human Gate，不选择最后一轮冒充 ready。

局部 scopes 可独立成为 Candidate/Gate 2 Owner facts；Manifest 技术成功不等于 P0 complete。Compiler 仍按 `3001/0010` 要求每个 active Episode 非空完整 Planning Collection、四个 checkpoint 与七 family 中所有实际 Collection Root 的 receipts；`planning_structure_rebase_set` 是条件式 family，正常空 rebase 既无 committed ref 也无 Receipt。任一应存根/回执缺失时 partial View 可继续、p0 Graph 不发布。

## 13. Candidate Revision 与 typed Repair

保留 Backend-owned immutable `StageCandidateRevision` 与 Head CAS：

```text
origin = invocation | aggregate | repair
parent = { kind=none } | { kind=revision, pointer: RevisionPointerProduction }
producer =
  invocation { variant_key, stage_instance_key, accepted_attempt_result_ref/hash }
  | aggregate { node_run_ref, aggregate_contract_id/hash, shard_manifest_ref/hash, ordered_input_roots }
  | repair { repair_variant_key, repair_stage_instance_key, accepted_attempt_result_ref/hash,
             parent_revision_pointer, review_candidate_revision_pointer }
transform =
  invocation { kind=normalizer, contract_ref/hash }
  | aggregate { kind=aggregate, contract_ref/hash }
  | repair { kind=patch_application, contract_ref/hash, authorization_root }
candidate_revision_hash covers
  revision + parent + complete producer discriminated union
  complete transform discriminated union
  normalized candidate contract/schema hash
  candidate content hash
```

Invocation/aggregate origin 只能建 revision 1，common parent 必须是 canonical `{kind:none}`。Repair 建 N+1，common parent 必须是 `{kind:revision}` 且 pointer 与 producer 内 `parent_revision_pointer` 逐字节相等，`revision=parent.revision+1`；Review pointer 和 successful repair Attempt 也是 producer 的必需身份。

Transform 不能自由选择：invocation 必须逐字节等于 producer Variant 所在 StageRelease 的 normalizer；aggregate 必须等于 producer 内 aggregate contract，且 `ordered_input_roots` 必须逐项等于 ShardManifest active leaves/ordered reduce tree 按该 contract comparator 得到的 canonical 顺序；repair 必须等于 repair StageRelease 冻结的 patch-application contract。Repair `authorization_root` 覆盖 target profile、base revision/hash、Review revision/hash、允许 operation/key/field/invariant 集与 repair Attempt input/result hash，必须由 Backend 重算相等才能 Apply。Normalized candidate contract 对 invocation 取 StageRelease 值、对 aggregate 取 aggregate output contract、对 repair 逐字节继承 parent 系列，不得在同一 producer 下分叉。

Backend aggregate 不伪造 Agent Stage Variant，只使用自己的 aggregate contract/Manifest/input roots；Repair 则绑定真实 repair Variant。下游只引用精确 revision id/hash；Head 改变后按 semantic dependency refs 标记影响闭包 stale，不扫描名称或 Graph edge。

`candidate_repair_patch_production` 按 target profile 严格判别：`script_structure|identity_resolution|production_entity|scene_binding|continuity|visual_foundation|reference_plan|reference_brief|storyboard`。每个分支拥有自己的操作 allowlist、key type、base fragment hash 和 invariant set：

- Evidence range/hash、reserved Scene logical ID、已确认 identity key/Owner Ref 和 Artifact bytes 永远不可改；
- Scene split/merge 与 identity merge/split 是 Human typed decision，产生新上游 Candidate，不由通用模型 Patch 偷改；
- State boundary、Occurrence binding、Interaction/Continuity 只能在对应 typed operation 且依赖闭包完整时修改；
- Visual Artifact 不能 Patch；修改 Brief 产生新 Candidate，随后显式重生成；
- Patch 应用后重跑目标 Contract 的全部 deterministic gates 与受影响 Review；无内容变化、越权或旧 Head 一律冲突。

文本 `review_candidate` 模型只返回 Issue，不拥有整体 `pass` 或 Gate 状态。`review_reference_artifact` 可以按每个 Rubric 项输出 `pass|warn|fail|not_assessable`，但该项级判定不等于整体 Gate pass，更不是 CandidateSelection。Mechanical gate blocker 不能被模型降级；Human Decision 也不能绕过 Schema/Evidence/Owner reference 的 fail-closed 条件。

## 14. Invocation、Attempt 与 unknown 恢复

逻辑 `AgentInvocation`、一次执行 `AgentAttempt`、Attempt 观察结果与唯一接受的 Invocation Outcome 必须分离：

```text
AgentInvocation
  immutable input / stage_instance_key / control hash at create / persistent attempt budget
AgentAttempt
  attempt_no / attempt_id / claim_version / release fence / control hash at dispatch / lease
AgentAttemptResult
  immutable succeeded | failed | unknown observation
AgentInvocationOutcome
  accepted_attempt_ref / accepted_result_hash / accepted_control_hash / terminal status
```

同一 Stage Variant identity 的恢复不新建 Invocation，但可以在持久预算内创建新 Attempt；Control Head 跨过 quarantine/revoke 后则必须按新 input/control hash 建新 Invocation。首个同时满足 schema/hash/claim fence/release fence/current Control 的成功 Attempt 才能通过 CAS 成为唯一 accepted outcome；迟到 attempt 即使成功也只留审计，不能覆盖。确定性失败可终结 Invocation；unknown 只终结该 Attempt，Invocation 进入 `reconciling`，不能同时保存一个不可变 unknown Outcome 又把同一对象重新 queued。

恢复分类：

- dispatch 前失败：同 Invocation 创建下一 Attempt；
- 已 dispatch 且有外部 execution id：先对账同一执行，不盲重试；
- 本地 Codex 进程超时/崩溃且无可恢复 side effect：显式 NodeRun Recovery Command 可在预算内创建下一 Attempt；
- post-dispatch outcome 可能有外部副作用：保持 reconciling/人工恢复；Generation Provider 不属于本 Wire，使用其自己的幂等提交合同；
- 每次 Attempt 递增 claim version，旧 Worker 结果因 fencing 不能成为 accepted outcome；
- max attempts/model calls/token/technical deadline 是 Invocation 持久事实，重启和人工恢复不重置。

Stage/Candidate semantic blocker 不使用 failed/unknown；它是成功 Candidate 中的 scoped issue。Release runtime 缺失、Schema 无效、Evidence 不可重建、附件 digest 不符等才是技术失败。

## 15. Runtime 与安全边界

| runtime class | 输入能力 | 工具策略 |
|---|---|---|
| `text` | Envelope 内规范文本、Candidate/Owner refs 的有界只读投影 | `allowed_tools=[]` |
| `vision` | text 能力 + Backend broker 注入的内容定址只读图片 | `allowed_tools=[]` |

两类 runtime 都使用临时目录、忽略用户配置、禁用 Shell/Web/Browser/Plugins/Skill Search/任意文件系统浏览、数据库和对象存储凭据。Codex 只看到 Harness 注入的入口、Stage resources、stage payload 和严格 output schema。原稿、用户参考图元数据和外部 Skill 文本都按不可信数据处理，不能把其中指令提升为系统 Guidance。

Agent 不直连媒体 Provider，不写数据库，不发 Owner Command，不访问 StoryGraph Repository。Runtime image 以 digest 冻结。除安全/许可证隔离或撤销外，非终态 Invocation 引用的旧 release/runtime 必须保留可路由，引用归零后才可回收。`quarantined|revoked` 的字节保留用于审计但不再执行；已排队或 reconciling Invocation 转人工决议。找不到精确 image/release 时返回 `skill_release_unavailable`，不使用相近版本。

本地开发可继续使用 Codex CLI 的 ephemeral/read-only/ignore-user-config/strict output-schema 模式；生产实现可以替换执行器，但必须满足同一 StageRelease、媒体 broker、tool deny 与 Wire contract。

## 16. 失败语义

| code | 层级 | 处理 |
|---|---|---|
| `skill_release_invalid` | Invocation | Release/资源/来源 Hash 不匹配，fail closed |
| `skill_release_signature_invalid` | Invocation | Release Root、算法、key 或批准策略无法验签，拒绝 |
| `skill_release_control_blocked` | Invocation/Attempt/Apply | current Control 不允许当前操作，fence 迟到结果且不 fallback |
| `skill_release_unavailable` | Invocation | 精确 runtime 不可路由，保持 reconciling/等待恢复 |
| `dispatch_authorization_invalid` | Attempt | token 签名/期限/输入/claim/control/fence 不等价，Harness 不执行 |
| `stage_contract_invalid` | Attempt | Stage/Input/Output/Runtime 不匹配，不产生 Candidate |
| `style_input_forbidden` | Invocation | P0 输入混入 Preset/Style/Artifact/Graph，拒绝 |
| `source_evidence_invalid` | Attempt | code-point range/text/hash 无法重建，拒绝 |
| `candidate_schema_invalid` | Attempt | 有限结构纠正仍失败，确定失败 |
| `candidate_coverage_invalid` | Aggregate | active leaf/mention/expected key 不完整，不产根 Candidate |
| `upstream_candidate_stale` | Invocation | exact revision 不再 current，标 stale 后按新输入创建新 identity |
| `media_attachment_invalid` | Attempt | MIME/length/digest/Owner ref 不等价，拒绝 vision 执行 |
| `preset_capability_missing` | Candidate | 成功 Candidate 中的 scoped blocker，等待 Gate 3 处理 |
| `non_convergent_reconciliation` | Candidate | 成功 Candidate 中的 scoped blocker，进入人工审核 |
| `attempt_runtime_unknown` | Attempt | 先对账/显式恢复，不伪造空 Candidate |
| `attempt_deadline_exceeded` | Attempt | 不自动放宽预算；按恢复分类处理 |
| `tool_not_allowed` | Attempt | 确定失败并记录安全事件 |

完整 WorkflowRun 不设置截断正常业务的固定墙钟。单 Attempt 技术 deadline、持久重试预算和人工等待分别记录；无论耗时多久，都不能丢弃已成功 shard、Candidate、Decision、Receipt 或 Selection。

## 17. 原子迁移策略

`storygraph-stage-wire-production` 的实施顺序必须形成可验证的小闭环，但正式切换只能一次：

1. 先增加 production Schema/fixture/StageRelease/Attempt 模型和拒绝测试，不让生产 Workflow 创建 production；
2. 建立 production References/Recipe/Rubric 与 provenance inventory，运行 Skill 校验、golden/adversarial/vision eval；
3. 实现 P0 production Stage 与 Backend deterministic aggregate，保持旧 legacy 新建路径仍唯一；
4. 实现 Visual Foundation/Reference/Vision Stage 和 D08 Storyboard Packet Stage；
5. 在一个切换提交中修改 Workflow callers、Definition、Registry、数据库 allowlist 和 feature gate，使新 run 只创建 production；
6. 旧非终态 legacy 仅按原 wire/bundle/runtime digest 完成或显式终止；不得被 production worker 领取；
7. legacy 引用归零后，独立删除旧 Stage Schema/Reference/caller，不保留兼容 fallback。

每步遵循 Red → Green → Refactor、定向测试、真实跨语言 fixture 和相称的全量 CI；不得把 production 字段塞入 legacy optional payload，也不得先改数据库 Stage allowlist后让无消费者值进入生产。

## 18. 验收边界

未来 Acceptance 至少证明：

1. `SKILL.md` frontmatter/name/description 和全部 References 通过 `skill-creator` quick validation，无占位文件、未路由资源或全量递归注入；
2. Bundle、完整唯一 StageVariant 集、Eval/Shadow、Release Root、Signature/Control、input/output/normalizer/prompt/resource hash 可由 Go/Python 重算且跨语言一致；
3. P0 Wire 机械拒绝 Preset/Style/Artifact/StoryGraph，Preset 切换不改变 P0 input hash；
4. 完整剧本按 Span → SceneFact → Identity → Gate 1 → ProductionWorld → Gate 2 运行，RawMention、Scene scope、Owner partition 和 Evidence coverage 完整；
5. 一个角色跨多形象仍一个 Identity；Character/Location/Prop State、Occurrence、Interaction 和 Ledger 可局部修正；
6. 四 checkpoint 与七 family 中所有实际 Collection Root 的 Candidate/Decision/Receipt refs 在 Stage/Node 输出中可精确传递；正常空 `planning_structure_rebase_set` 不伪造 Receipt，Agent 也从不生成 Owner ref 或 StoryNodeKey；
7. 同一 Release 支持至少一个 faithful 与一个 world_adaptation Preset，P0 结果相同、Visual Foundation/Brief 按决议不同；
8. 六类 Reference Target 均产生严格 Brief；Character anchor 必须先生成/审查/人工选中并发布 AssetVersion，appearance 才能编译，组合 Target 只消费精确已选基础版本；
9. Vision Review 对结构、身份/状态、风格、跨资产和接触五类问题给出可复现 issue，且从未自动选择/发布；
10. Storyboard Stage 缺完整 SceneProductionPacket 时 fail closed，不输出 `needs_asset` 或反向启动 Generation；
11. Shard replan、fixed-point、late result、attempt unknown、repair CAS 和局部 stale 均不重复 Owner/Selection 事实；Release quarantine/revoke 能在 dispatch、Outcome CAS 和 Apply/reuse 三处 fence，撤销前已提交 Owner 事实不隐式回滚；
12. 外部 Skill 的来源/许可证/重写映射完整；Eval/Shadow 能从 fixture 和 Stage Release 重算到 runtime/policy/baseline/阈值/观测根与签名接受记录，生产运行无远程安装或网络 Skill 搜索；
13. 使用隔离 fixture 进行独立 forward test：给审阅 Agent 真实剧本、Preset、Prop 交互和参考图输入，不提供预期答案，验证实际产物而非关键词匹配；
14. 旧 legacy 与新 production 无新建双路径，历史 Invocation 只由精确旧 runtime 路由。

## 19. 完成边界与后续门

`VP-D07` 只接受新 Skill Release、Stage DAG、Wire、恢复和外部 Skill 吸收合同，不修改代码。通过独立评审与提交后只解锁：

1. `VP-D08`：固定 `direct_storyboard` 的 SceneProductionPacket、Intent/Detail、Review/Repair 合同；
2. `VP-D09`：固定 AgentInvocation/Attempt、五个 P0 Owner、七 family、Collection/Receipt/typed Query 数据与命令；
3. `VP-D10`–`VP-D12`：同步 Generation、Human Gate 和 Guided Studio；
4. `VP-D13`–`VP-D15`：重写唯一 PRD、Requirement、Plan 与全未通过 Acceptance；
5. 只有 `VP-D15` 已接受后，才从新 Plan 领取第一个 Red → Green 实现切片。
