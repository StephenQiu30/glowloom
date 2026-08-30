# Workflow 公共 Human Gate 命令与恢复设计

- 状态：已接受设计
- 接受记录：`VP-D11`（2026-08-30）；五 Gate 产品语义、Subject/Decision Wire、Effect/Resume 恢复三轴隔离反例评审通过（最终正文评审 SHA-256 `a9bc69626403cb400c8a463f7c2bfc06e719fd95abfd435dde5fddef5630161c`）
- 历史事实：旧版曾于 `SG-D16` 接受通用 HumanTask/Decision/Resume 骨架，并同步过 Bible-first、Storyboard Intent/Detail、Reference Asset、Shot Frame/Video 路由；这些历史实现和 Evidence 保留，但旧 Subject/Owner 路由不抵扣当前五 Gate 目标
- 已接受前置：[视觉生产工作台设计](0011-剧本视觉生产工作台与世界观预设设计.md) · [Production Bible 设计](3001-项目制作圣经生成执行框架设计.md) · [Storyboard Harness 设计](3002-本地-Codex-分镜智能体执行框架设计.md) · [Agent Harness 与 Skill 设计](3003-StoryGraph剧本解析Harness与内置Skill设计.md) · [后端领域设计](2002-后端领域模块功能设计.md) · [Generation 执行器设计](2051-通用媒体Provider与Generation执行器设计.md)
- 历史派生：[StoryGraph 产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md) · [需求规格](../requirement/0010-StoryGraph内容图与DAG创作画布需求规格.md) · [唯一实施计划](../plan/0010-StoryGraph内容图与DAG创作画布实施计划.md)；继续冻结，等待 `VP-D13`–`VP-D15`
- 下一设计门：[前端功能模块设计](1002-前端功能模块设计.md)（`VP-D12`）

## 1. 结论

Guided Studio 对用户只呈现五个稳定的决策 Gate：

```text
Gate 1  Structure & Identity
Gate 2  Bible & Continuity
Gate 3  Visual Foundation & Scope
Gate 4  Reference Selection
Gate 5  Storyboard
```

Backend 内部可以有多个 Agent Stage、Target、Wave、Owner Command 和 checkpoint，但不能把它们暴露为新的用户 Gate，也不能把五个 Gate 压成一个“已审核”状态。每次人工处理必须形成三段可独立查询、恢复和审计的事实：

```text
ReviewDecision recorded
  → Gate Effect applied | not_required | conflict
  → Workflow Resume completed | unknown | conflict
```

- `review` 唯一拥有 HumanTask、Claim 和不可变 ReviewDecision；
- 业务 Owner 唯一拥有正式 Version/Head/Command Receipt；
- `workflow` 唯一拥有 Gate Input、Effect Coordination、Resume Intent/Receipt 和 Temporal Signal；
- Frontend、Agent、Handler、Kafka 和 Temporal Workflow Definition 都不能绕过这些 Owner。

五个 Gate 的正向效果固定为：

| Gate | 正向 Decision | 正式 Effect |
|---|---|---|
| Gate 1 | `approved` | `production/project` 先确认 Episode 生命周期，`production/bible` 再发布 StructureIdentitySetVersion |
| Gate 2 | `approved` | `production/bible + production/planning + asset` 同事务发布 Confirmed Production World |
| Gate 3 | `approved` | `preset + production/reference` 同事务发布 Effective Snapshots 与 ApprovedReferencePlanVersion |
| Gate 4 | `selected` | `generation` 发布 CandidateSelection，随后 `asset` 或 `asset + production/reference` 发布 exact Result |
| Gate 5 | `approved` | `production/storyboard` 按 Scene 原子发布 formal Shot set 与 ShotProductionBindingVersion |

`rejected|changes_requested` 不产生正向 Owner Version，但仍以同一 ReviewDecision 恢复 Workflow 的拒绝或 repair 分支。Decision 已提交后，任何超时、进程退出或网络未知都只能以同一个 Decision ID 对账；不得创建第二 Decision、重做已完成 Effect、发送不同 Resume payload 或启动第二 Workflow。

## 2. 问题、范围与非目标

### 2.1 要解决的问题

旧 Human Gate 设计与新视觉生产链存在六个错位：

1. Subject 仍围绕 `production_bible_candidate`、`episode_plan_candidate`、Storyboard Intent/Detail 和 Shot 媒体，不能表达新的五 Gate；
2. Gate 1 实际有两个有序 Owner 事务，Gate 2/3/5 各有不同原子边界，不能用一个泛化 `OwnerApplier` 隐藏；
3. Gate 4 是多 Target、多波次、多任务的用户阶段，不是一张“Reference 全部批准”任务；
4. CandidateSelection 不是 AssetVersion/Reference Binding，选择成功后仍需精确 Owner Apply；
5. Task、Decision、Owner Apply 和 Workflow Resume 容易在 UI/接口中被合并成一个状态；
6. Decision 后的进程退出、Owner 响应未知、Temporal Signal unknown 和并发 resume 需要逐段恢复证据。

### 2.2 本文范围

- 五个用户 Gate 与内部 Task/Effect/checkpoint 的映射；
- immutable Workflow Gate Input、HumanTask、Decision 和 gate-specific strict union；
- Claim/Renew/Release/Expire 与租户授权；
- 显式 Effect Plan、Step Intent/Receipt、Gate Effect Bundle Receipt；
- Decision → Owner Effect → Temporal Resume 的幂等、并发和未知恢复；
- 公共 Command/Query/HTTP 语义和稳定失败码；
- Gate 4 多任务聚合、局部阻塞和 checkpoint 边界；
- stale、影响闭包、安全、可观测性与后续验收门。

### 2.3 非目标

本文不定义：

- 前端页面布局、交互动效、批量操作 UI 或通知中心；
- 任意 Gate 插件、反射式 Service Locator、通用 Saga DSL 或跨服务事务框架；
- 自动批准、模型替用户决策、按置信度跳过 Gate；
- 在 HumanTask 复制完整剧本、Candidate、图片 bytes、QC 结果或 Owner Snapshot；
- 用 Kafka 发送业务 Command/Temporal Signal，或用日志恢复业务状态；
- 修改、撤回、覆盖或删除 ReviewDecision；
- 让 Human Decision 覆盖 Schema/Evidence/Coverage/rights/deterministic QC blocker；
- 为旧 Bible/Intent/Detail/Shot Media Subject 建兼容 fallback。

## 3. Owner 与单向依赖

```text
Workflow Gate Node
  → WorkflowHumanGateInputV2
  → review.HumanTask
  → review.ReviewDecisionV2
  → workflow.GateEffectCoordinationV2
      → explicit gate-specific Owner Command(s)
      → GateEffectBundleReceiptV2
  → workflow.WorkflowResumeIntentV2
      → Temporal Signal / History reconcile
      → WorkflowResumeReceiptV2
```

单向 Hash/引用规则：

1. Gate Input 不引用 Task、Decision、Effect 或 Resume；
2. HumanTask 引用 Gate Input；Decision 引用 Task 和 Gate Input；
3. Owner Version 可以记录 ReviewDecision audit ref，但不引用 Effect Bundle/Resume；
4. Effect Receipt 引用 Decision 和 Owner Receipt；Resume Envelope 引用 Effect Receipt；
5. Workflow Node output 最后引用 Resume Receipt，不反向进入上游任何 content hash；
6. 任一实现若需要 Candidate ↔ Decision、Owner Version ↔ Receipt 或 Effect ↔ Resume 双向 hash，Schema 设计即失败。

Review Decision、Owner Effect 和 Temporal Resume 是多个明确事务/网络边界。可靠性来自稳定 identity、Owner Command 幂等、Receipt 和 History 对账，不伪装成跨边界数据库事务。

## 4. Workflow Gate Input 与 HumanTask

### 4.1 WorkflowHumanGateInputV2

Workflow 在打开 Task 前先持久化 immutable Gate Input：

```text
WorkflowHumanGateInputV2
├── gate_input_id / workflow_run_ref / node_run_ref
├── workflow_gate_wait_snapshot_ref / wait_token_hash
├── gate_key = gate_1_structure_identity |
│              gate_2_bible_continuity |
│              gate_3_visual_foundation_scope |
│              gate_4_reference_selection |
│              gate_5_storyboard
├── gate_instance_key
├── workspace_id / project_id / scope_keys[]
├── subject_type
├── subject_payload: HumanGateSubjectV2 strict union
├── subject_content_hash
├── subject_read_set_root
├── candidate_revision_refs[] / candidate_set_ref?
├── review_issue_refs[] / deterministic_gate_result_refs[]
├── allowed_decisions[]
├── decision_payload_contract_ref
├── effect_plan_contract_ref / effect_plan_hash
├── renderer_contract_ref
├── created_by_node_attempt_ref
├── content_hash
└── created_at
```

`additionalProperties=false`；除 `?` 外字段必需。`gate_instance_key` 固定为：

| Gate | instance key |
|---|---|
| 1 | `gate-1:<project>:<structure-candidate-head>` |
| 2 | `gate-2:<project>:<production-world-candidate-root>` |
| 3 | `gate-3:<project>:<visual-foundation-plan-candidate-root>` |
| 4 | `gate-4:<approved-plan-ref>:<reference-target-ref>:<generation-round>:<candidate-set-ref>` |
| 5 | `gate-5:<scene-owner-logical-id>:<storyboard-candidate-head>` |

Key 的每个占位都使用 exact ref 的 canonical logical/version identity，不使用显示名、数组序号、页面 URL 或“当前”。同一 Gate Input content hash 重放返回同一记录；同一 instance key 异参冲突。

`HumanGateSubjectRefV2` 统一指向 Gate Input 内嵌 Subject：`{gate_input_id, subject_type, subject_revision=1, subject_hash}`。Task/Decision 不允许有时直接指 Candidate、有时指 CandidateSet 或 Owner Snapshot；具体 Candidate/Set/Owner refs 只位于对应 strict union payload。Gate Input 顶层 `candidate_revision_refs[]/candidate_set_ref?/subject_read_set_root` 是索引前像，Backend normalizer 必须证明它们与 payload 中同类 refs/root 逐字节全等，不能形成第二份可漂移 Subject。

Workflow 在 Gate Input 之前创建 `WorkflowHumanGateWaitSnapshotV2`，只冻结 workflow execution chain、NodeRun waiting revision、gate key/instance、wait token hash 和 resume contract，不引用 Gate Input/Task/Decision。Gate Input 的 `node_run_ref` 必须逐字节等于该 waiting revision；NodeRun 后续记录 Gate Input/Task/Resume 的新状态不能反向改变 Wait Snapshot 或 Gate Input hash。实际 wait token 只在 Workflow/Backend 内部保存，Task/Frontend 只见 hash/ref。

### 4.2 HumanTask

HumanTask 只冻结审核入口，不复制 Subject 内容：

```text
HumanTaskV2
├── task_id / workspace_id / project_id
├── workflow_gate_input_ref / gate_input_hash
├── gate_key / gate_instance_key / subject_type
├── subject_ref / subject_revision / subject_hash
├── scope_keys[] / ordered_candidate_keys[]
├── allowed_decisions[] / rubric_policy_ref
├── status = OPEN | CLAIMED | COMPLETED | CANCELLED | STALE
├── task_revision
├── claim_owner_ref? / claim_token_hash? / claim_expires_at?
├── stale_reason_ref?
└── created_at / updated_at
```

Subject ref 永远是上一节的 `HumanGateSubjectRefV2`；Agent Candidate Revision、CandidateSet 和 Owner Snapshot 只作为该 Subject payload 的内容定址依赖。Task 的 mutable claim/status revision 不进入 Subject hash。一个 Gate Input 最多创建一个 Task；Task stale 后同一旧 Input 不得重新开 Task，必须先生成新 Gate Input。

Task Query 通过 renderer contract 调用对应 typed View；不得把大 Candidate JSON、剧本文本、媒体 bytes 或 Provider 响应塞进 Task 行。

### 4.3 Task 开放前置

只有同时满足以下条件才打开 Task：

- Gate Input schema/hash/read set/coverage 已通过 Backend deterministic validation；
- Candidate/Review Stage Outcome 已成功且 Release Control 未 quarantine/revoke；
- Mechanical blocker 已清零；允许人工裁决的 semantic issue 显式存在于 Review View；
- 所需媒体 READY、digest/rights/lineage/QC 满足当前 Gate；
- allowed decisions 与 effect plan 是 Backend 对 gate/subject exhaustive match 的结果；
- 同一 gate instance 既没有 active Task，也没有已提交 Decision。

人工不能看到一个注定无法 Apply 的“伪可审批”任务。`changes_requested` 可以在存在可修 semantic issue 时开放；Schema、hash、rights、租户或 deterministic QC fail 不转成 changes_requested。

## 5. Decision strict union

### 5.1 ReviewDecisionV2

```text
ReviewDecisionV2
├── decision_id / workspace_id / project_id
├── human_task_ref / expected_claimed_task_revision
├── workflow_gate_input_ref / gate_input_hash
├── subject_ref / subject_revision / subject_hash
├── gate_key / gate_instance_key
├── decision_payload: HumanGateDecisionPayloadV2 strict union
├── claim_owner_ref / claim_token_fingerprint
├── membership_token_version
├── idempotency_key / command_hash
├── content_hash
└── decided_by / decided_at
```

公共 discriminant 只有：

```text
approved {
  accepted_semantic_issue_refs[],
  creator_decision_entries[]
}

selected {
  selected_candidate_bundle_ref,
  accepted_warning_issue_refs[],
  acknowledged_not_assessable_issue_refs[]
}

changes_requested {
  issue_refs[],
  change_spec: GateChangeSpecV2 strict union,
  reason_code,
  user_note?
}

rejected {
  reason_code,
  user_note?
}
```

- Gate 1/2/3/5 的最大集合为 `approved|changes_requested|rejected`；
- Gate 4 的最大集合为 `selected|changes_requested|rejected`，不允许 `approved`；
- 实际 `allowed_decisions[]` 是上述最大集合与当前 Subject readiness 的交集：存在可裁决 semantic blocker 时必须移除 `approved|selected`，只开放 changes/reject；新 Candidate 消除 blocker 后才创建可正向决议的新 Gate Input/Task；
- `selected_candidate_bundle_ref` 必须属于 Gate 4 Subject 冻结的 complete CandidateSet；
- warn/not_assessable acknowledgements 必须与所选 Bundle 的 Vision Review issues 精确全等，不得忽略；
- `user_note` 只用于审计，不授权 Patch、Owner 写入、Prompt 或工具调用；
- rejected/approved 不得夹带 candidate selection；selected 不得夹带 change spec。

`CreatorDecisionEntryV1` 是 Decision 内嵌严格对象，至少包含 target Owner/family/logical id/field path、对应 DesignGap/semantic issue ref、typed chosen value、reason code 和 affected scope keys。它不是提前存在的 Owner Ref；正向 Owner Apply 校验其 path/value 在 Subject allowlist 内后，把它内容定址为目标 Owner 的 creator-decision fragment，并从该 fragment 反查 ReviewDecision/path。这样不会出现“Owner 结果尚未发布，Decision 却先引用结果 ID”的前向引用。Gate-specific schema 可以要求该数组为空；自由文本 note 永远不能成为 CreatorDecisionEntry。

### 5.2 GateChangeSpecV2

`changes_requested` 不是自由文本修库。严格分支固定为：

| Gate | 允许的 change spec | 结果 |
|---|---|---|
| 1 | Scene split/merge proposal、Identity merge/split/rebind、resolve/unresolve key refs | Workflow 创建新 structure/identity Candidate；旧 Subject 不改 |
| 2 | State boundary/snapshot、Occurrence rebind、Interaction/Continuity decision、typed creator decision refs | Workflow 触发受限 repair/reconcile，生成新 Candidate |
| 3 | Preset switch、faithful/adaptation、typed override、design gap option、Target fulfillment/scope edit | Workflow 生成新 Foundation/Plan Candidate |
| 4 | `regenerate_candidates|edit_reference_brief|request_upstream_reference_change`，绑定 exact Target/Brief/dependency refs | Workflow 先产生新的上游 Result（如需要），再走 D10 的新 Brief/Target/round；不改旧 CandidateSet/Target dependency |
| 5 | review issue repair、typed shot change、new creative candidate slot | Workflow 走 D08 的严格 repair/aggregate/new sampling path |

每个 change spec 都由 Backend allowlist 验证 target key/path/ref/operation 和影响 scope；无法结构化表达的方向性意见只能成为 `user_note` 并阻断自动 repair，不能直接提升为 Agent Patch 权限。

### 5.3 Decision 提交

Decision Command 在一个 `review` GORM 事务中完成：

1. 重新验证 Workspace membership、Token Version、Task revision/status、claim owner/token/expiry；
2. 重读 Gate Input/Subject exact refs/hash、Candidate Head/Set 和 allowed decision；
3. 运行 decision payload strict schema 与 issue/candidate membership 校验；
4. 创建唯一 immutable ReviewDecision、Review Command Receipt，Task → COMPLETED；
5. 写 Decision Recorded Outbox；
6. 返回 Decision ID，即使后续 Effect/Resume 尚未开始。

同 Task 只能有一个 Decision。相同 idempotency key/input 返回相同 Decision；同 key 异参或并发第二 Decision 冲突。Decision 提交后不因 Effect/Temporal 失败回滚、删除或伪装成“未提交”。

Decision 保存的是事务开始时通过 CAS 的 claimed Task revision；Task → COMPLETED 后产生的新 revision 不回填 Decision。Task 行也不把 Decision content hash 写进其旧 revision；协调 Query 通过唯一 task→decision 关系读取，从而避免 Task ↔ Decision hash 环。

## 6. 五个 Gate 的 Subject 与 Effect Plan

### 6.1 Gate 1 — Structure & Identity

```text
Gate1StructureIdentitySubjectV2
├── document_revision_ref / source_span_index_ref
├── structure_candidate_revision_ref
├── identity_candidate_revision_ref
├── structure_review_candidate_revision_ref
├── identity_review_candidate_revision_ref
├── scene_scope_manifest_ref
├── reserved_episode_and_scene_key_manifest_ref
├── raw_mention_partition_root
├── expected_project_head / expected_bible_structure_head
└── read_set_root
```

正向 Effect Plan 是两个有序、独立事务：

1. `ConfirmProjectEpisodeLifecycleCommand` → Project Episode refs + `project_episode_lifecycle_confirmed` Receipt；
2. `ConfirmStructureIdentitySetCommand` → StructureIdentitySetVersion + `gate_1_structure_identity` Receipt。

第二步必须引用第一步 exact Receipt。第一步成功、第二步响应失败时保留 Project 事实；resume 精确复用第一步 Receipt，只重放第二步同 idempotency key。两个步骤共同属于同一个用户 Gate 1，不创建第六个 Task/Decision。

Scene/Identity split/merge/rebind 必须先产生新 Candidate/Task；approved Apply 只复制冻结 Candidate 的 exact mapping，不在事务中解释用户自由文本。

### 6.2 Gate 2 — Bible & Continuity

```text
Gate2BibleContinuitySubjectV2
├── structure_identity_set_version_ref
├── production_world_candidate_root
├── bible_partition_candidate_ref
├── planning_partition_candidate_refs[]
├── asset_partition_candidate_ref
├── reconcile_manifest_ref / continuity_gate_result_ref
├── expected_business_key_roots[] / scope_closure_root
├── expected_bible_planning_asset_heads[]
└── read_set_root
```

approved 只调用一个 `ConfirmProductionWorldCommand`。它在同一 PostgreSQL/GORM 事务中由 `production/bible + production/planning + asset` 校验并发布各自 Version/Head、每个真实 Collection Receipt、一个 Command Receipt 和 Outbox。任一 partition、Evidence/creator decision、Occurrence/Interaction/Continuity 或 expected Head 失败整体回滚。

Gate 2 不创建 Preset、Reference Plan、AssetVersion、Generation Target、StoryGraph 或 Shot。partial scope 由 Subject 的 scope closure 明确；未在 closure 内的 Scene 不被暗中发布。

### 6.3 Gate 3 — Visual Foundation & Scope

```text
Gate3VisualFoundationScopeSubjectV2
├── confirmed_production_world_read_set_root
├── preset_version_ref / project_preset_binding_candidate_ref
├── effective_style_candidate_revision_ref
├── effective_policy_candidate_revision_ref
├── reference_plan_candidate_revision_ref
├── design_gap_decision_manifest_ref
├── world_adaptation_mapping_refs[]
├── backend_expected_reference_target_key_root
├── expected_preset_and_reference_heads[]
└── read_set_root
```

approved 调用 `ConfirmVisualFoundationAndReferencePlanCommand`，在同一事务中：

- `preset` 发布 Project Binding、EffectiveStyleSnapshot、EffectivePolicySnapshot；
- `production/reference` 发布唯一 active ApprovedReferencePlanVersion 与六类 Target；
- Backend 重算 expected Target business key 集与 Candidate Plan 全等；
- source fact → world adaptation mapping 和 typed override 完整进入审计；
- 分别写 Owner Collection/Command Receipts 与 Outbox。

Gate 3 不调用 Provider、不生成图片、不选择 Candidate。Preset 不支持 Target kind/view role 时必须在 Gate Input/Review View 中形成 `preset_capability_missing` semantic blocker；Task 可以开放 changes_requested/rejected，但 `allowed_decisions[]` 必须移除 approved。用户通过 changes_requested 改 Preset/Scope并取得新 Candidate/Task，不能 approved 后静默 fallback。

### 6.4 Gate 4 — Reference Selection

Gate 4 是一个用户阶段，不是一个全项目 HumanTask。每个 exact Reference Target/generation round/CandidateSet 打开一个任务：

```text
Gate4ReferenceSelectionSubjectV2
├── approved_reference_plan_version_ref
├── reference_plan_target_ref / target_business_key
├── generation_target_ref / generation_execution_ref
├── reference_candidate_bundle_input_refs[]
├── candidate_bundle_refs[] / candidate_set_ref
├── deterministic_qc_result_refs[]
├── bundle_vision_review_candidate_refs[]
├── dependency_asset_version_refs[] / dependency_root_hash
├── target_and_execution_head_fence
├── result_owner_expected_heads[]
└── read_set_root
```

selected 的 Effect Plan 固定为两个阶段：

1. `generation.SelectReferenceCandidate` 发布 `GenerationCandidateSelectionV2`；
2. 按 Target kind 显式分派：
   - base 四类 → `PublishSelectedBaseReferenceResultCommand`，由 `asset` 发布 Artifact/Rendition + AssetVersion；
   - Scene/Interaction Composition → `PublishSelectedCompositionResultCommand`，由 `asset + production/reference` 同事务发布 selected Composition Artifact + ReferenceBindingVersion。

第二步必须消费第一步 exact Selection Receipt。Selection 已提交、Result Apply 响应未知时 resume 不改选、不创建第二 Selection，只以同 Owner Command key 对账/重放。

Identity anchor、Location、Prop、Appearance 与 Composition 的依赖波次由 D10 执行 DAG决定。一个选择 Task 完成只代表一个 Target Result 已发布；Gate 4 用户阶段完成由：

- `CompleteBaseReferenceSelectionCheckpointCommand`；
- 各 Scene 的 `CompleteSceneCompositionSelectionCheckpointCommand`；
- 全部 active required Target coverage；

共同机械证明。checkpoint 是 Workflow barrier/Owner Command，不创建额外 HumanTask 或“批准全部”Decision。optional 未选和 not_generated 按 Plan 处理，不生成假 Result。

changes_requested 可以要求重生成、编辑 Brief 或请求更换上游参考，但只恢复 Workflow 的新上游 Result→Brief→Target 分支；不得把另一个 AssetVersion塞进当前 Target dependency，Coordinator 也不直接调用 Provider。rejected 保留 Target blocked；required Target 未解决时 Gate 4 不能完成。

### 6.5 Gate 5 — Storyboard

```text
Gate5StoryboardSubjectV2
├── scene_production_packet_ref / packet_hash
├── packet_execution_materialization_ref
├── storyboard_candidate_revision_ref / candidate_head_ref
├── storyboard_review_candidate_revision_ref
├── storyboard_deterministic_gate_result_ref
├── scene_semantic_input_root
├── skill_release_control_refs[]
├── expected_storyboard_head
├── gate5_graph_check_input_ref
└── read_set_root
```

approved 调用 `ApplyStoryboardCandidateCommand`，由 `production/storyboard` 在一个 Scene 事务中：

- 重验 Candidate/Decision/Release、Packet、媒体、Binding 与 `Gate5GraphCheckV1`；
- 创建完整 formal Shot set；
- 每 Shot 创建恰 1 个 ShotProductionBindingVersion；
- 保存 temporary→formal mapping、Command Receipt 和 Outbox；
- Owner 事务后独立触发 StoryGraph Compiler。

其他 Scene 使全局 Graph Head 前进时，只允许 D09 的严格 Scene semantic revalidation；本 Scene 任一输入变化都令旧 Decision Effect conflict。`changes_requested` 只进入 D08 的 typed repair/new candidate path，不在 Apply 中编辑 Shot。

### 6.6 负向 Decision

`rejected|changes_requested` 的 Effect Plan 为空，但仍发布 `GateEffectBundleReceiptV2(disposition=not_required)`，其中冻结 gate branch、Decision ref/hash、change spec/reason hash 和预期 Resume output。它不能创建任何业务 Owner Version、Collection Receipt、CandidateSelection、Generation Target 或 Provider Call。

Workflow Definition 只根据这个 immutable branch：

- rejected → 结束当前 scope 或返回上游；
- changes_requested → 进入受限 repair/reconcile/new candidate；
- selected/approved → 只在 Effect Bundle completed 后进入下游。

浏览器不能直接选择下一个 Node、触发 Agent/Provider 或伪造 not_required Receipt。

## 7. Effect Coordination 与 Receipt

### 7.1 显式 Effect Plan Registry

Composition Root 注册五个 gate key 的编译时 exhaustive plan，不提供数据库动态 executor、字符串反射或 fallback：

```text
gate_1_structure_identity       → gate1-effect-plan-v2
gate_2_bible_continuity        → gate2-effect-plan-v2
gate_3_visual_foundation_scope → gate3-effect-plan-v2
gate_4_reference_selection     → gate4-effect-plan-v2
gate_5_storyboard              → gate5-effect-plan-v2
```

Plan contract 冻结允许 decision、步骤顺序、Owner Command contract、input projector、result normalizer 和 Gate output schema hash。Task 打开时的 `effect_plan_hash` 必须与 Decision 后 Composition Root 当前已加载 release 相等；Plan 被 quarantine/revoke 或 hash 漂移时不能用新逻辑解释旧 Decision，必须由精确旧 release 恢复或明确 conflict。

`GateEffectPlanReleaseV2` 是 Backend 编译发布物的签名 Manifest，包含 plan contract/hash、input/output schema hash、五个显式 projector/applier factory refs 和 control status。部署必须保留仍有 OPEN Task、recorded Decision 或 pending/unknown Coordination 引用的旧 Release；新 Release 只供新 Gate Input 使用。它不是数据库脚本插件，不能从 Task payload 下载并执行任意代码。

### 7.2 Coordination identity

```text
GateEffectCoordinationV2
├── coordination_id
├── coordination_key = human-gate-effect:<review_decision_id>
├── decision_ref / decision_hash
├── workflow_gate_input_ref / gate_input_hash
├── effect_plan_contract_ref / effect_plan_hash
├── expected_step_keys[] / expected_step_root
├── state = pending | applying | completed | conflict
├── conflict_code?
└── created_at / updated_at

GateEffectStepIntentV2
├── coordination_ref / step_key / step_index
├── owner_command_contract_ref
├── owner_command_idempotency_key
├── owner_command_hash
├── depends_on_step_receipt_refs[]
├── state = pending | dispatched | completed | conflict
└── created_at / updated_at

GateEffectBundleReceiptV2
├── coordination_ref / decision_ref
├── disposition = applied | not_required
├── ordered_step_receipt_refs[]
├── canonical_gate_output
├── gate_output_hash
├── content_hash
└── completed_at
```

Step idempotency key 固定为 `human-gate-effect:<decision_id>:<step_key>`。Coordinator 在调用 Owner 前持久化 Intent；Owner Command 事务成功但响应丢失时，resume 按 key/hash查询同一 Command Receipt，不能把 `dispatched` 超时重置为新命令。

Effect Bundle 只在所有 required step 已有精确 Receipt 后创建。Gate 1 的两个 step 有序；Gate 4 的 Selection→Result 有序；Gate 2/3/5 各一个原子 step。`not_required` 的 expected step 集为空。

### 7.3 Gate Output

canonical Gate Output 只包含下游需要的 exact refs/hash：

| Gate | Output |
|---|---|
| 1 | Project Episode Receipt + StructureIdentitySetVersion/Receipt |
| 2 | Bible/Planning/Asset Owner roots + Collection/Command Receipts |
| 3 | Effective Style/Policy + Approved Plan/Target root + Receipts |
| 4 | CandidateSelection + AssetVersion 或 ReferenceBinding + Artifact refs/Receipts |
| 5 | formal Shot set + ShotProductionBinding set + Command Receipt |
| negative | Decision branch + change spec/reason hash |

Gate Output 不含完整 Candidate、剧本文本、图片 bytes、Prompt、Secret、临时 URL、搜索结果或“最新版本”别名。

## 8. Workflow Resume 与 Temporal 恢复

### 8.1 Resume envelope

```text
HumanGateResumeEnvelopeV2
├── signal_id = human-gate-resume:<review_decision_id>
├── workflow_run_ref / node_run_ref / wait_token
├── gate_key / gate_instance_key
├── workflow_gate_input_ref / gate_input_hash
├── human_task_ref / review_decision_ref / decision_hash
├── gate_effect_bundle_receipt_ref / effect_bundle_hash
├── gate_output_hash
├── branch = approved | selected | rejected | changes_requested
├── resume_contract_ref
└── envelope_hash
```

`wait_token` 由等待 Node 创建，绑定 Workflow execution chain、NodeRun revision 和 Gate Input，不由浏览器传入。Resume Envelope 从已提交事实确定性构建；相同 Decision 永远得到相同 signal id/envelope hash。

### 8.2 Resume Intent/Receipt

```text
WorkflowResumeIntentV2
├── signal_id / envelope_hash
├── workflow_run_ref / node_run_ref / wait_token
├── state = prepared | sending | unknown | completed | conflict
├── send_attempt_count / last_error_code?
└── created_at / updated_at

WorkflowResumeReceiptV2
├── signal_id / envelope_hash
├── temporal_execution_ref
├── applied_node_run_ref / applied_history_event_ref
├── outcome = signaled | already_applied
├── content_hash
└── completed_at
```

处理顺序：

1. Effect Bundle completed/not_required 后，在 GORM 事务中插入或同值读取 Resume Intent；
2. 事务外向 Temporal 发送 Envelope；
3. Workflow handler 先验证 wait token/gate input/effect hash，再按 signal id 去重；
4. 首次合法 Envelope 推进等待 Node 并在 History 记录 applied identity；
5. Backend 收到确定响应后写 Resume Receipt；响应丢失时 Intent → unknown；
6. resume/recovery worker 只发送同一 Envelope并查询 History；已存在同 signal id/hash时写 `already_applied` Receipt；
7. 同 signal id 异 hash、错误 wait token、错误 Node/Gate 或已由不同 Decision 推进时 conflict。

Temporal Activity retry、API 重试、Worker 重启和 continue-as-new 必须保留 signal dedupe identity；continue-as-new 在 Workflow state 中转移已应用 signal root或可验证 compact receipt，不能清空后接受旧 Signal 第二次。

### 8.3 三阶段状态

| Decision | Effect | Resume | 用户语义 | 唯一动作 |
|---|---|---|---|---|
| 无 | — | — | 待审核 | claim/decide |
| recorded | pending/applying | pending | 决议已记录，业务尚未应用 | 同 Decision resume |
| recorded | completed/not_required | pending/unknown | 业务结果已确定，工作流恢复中 | 同 Envelope 对账 |
| recorded | conflict | pending | 冻结 Subject/Owner baseline/plan 漂移 | 旧 Decision 只读；新 Gate Input/Task |
| recorded | completed/not_required | conflict | Signal identity/input/wait token 冲突 | 人工诊断；不建第二 Decision/Workflow |
| recorded | completed/not_required | completed | 工作流已继续 | 重取 NodeRun 与下游 Owner View |

只有 Resume Receipt completed 且 NodeRun/下游输出可重读时，UI 才显示“已继续”。Decision recorded 不能显示 Gate completed；Gate 4 单 Target Resume completed 也不能显示整个 Reference Selection stage completed。

## 9. Claim、授权与并发

### 9.1 Claim 生命周期

```text
ClaimHumanTask(expected_task_revision, idempotency_key)
RenewHumanTaskClaim(expected_task_revision, claim_token, idempotency_key)
ReleaseHumanTaskClaim(expected_task_revision, claim_token, idempotency_key)
```

- Claim token 只返回当前 claim owner，数据库只保存安全 hash/fingerprint；
- token 不进 URL、列表、日志、Trace、Metric、Decision content、Temporal History 或 Outbox；
- lease 到期可由其他授权成员 claim，但旧 token/revision 永久失效；
- renew/release/decide 都校验 Workspace/Project membership、Token Version、claim owner、token、expiry 和 expected revision；
- 相同 key/相同 command hash 幂等，同 key 异参冲突；
- Decision 事务与 expire/renew 并发时只有一个 expected revision 成功。

### 9.2 Scope 与防枚举

所有 Task/Gate Input/Subject/Candidate/Decision/Owner Ref/Run/Node 必须属于同 Workspace/Project/scope closure。任一 cross-scope ref 在进入 Effect Projector 前拒绝。无权与不存在按平台防枚举策略返回相同外观；Viewer 只读，具备项目编辑权的成员才能 claim/decide，Gate-specific Owner role policy 可进一步收紧。

Actor 身份、Membership/Token Version 进入 Decision/Command audit，但 Access Token/Claim Token 不进入 content hash或业务版本。

## 10. 公共 Command、Query 与 HTTP 语义

### 10.1 Query

语义 Query 固定为：

```text
ListProjectHumanTasks(project_ref, status?, gate_key?, subject_type?, cursor, limit)
GetHumanTask(task_ref)
GetHumanTaskSubjectView(task_ref)
GetHumanGateCoordination(decision_ref)
GetHumanGateTimeline(gate_instance_key)
```

列表稳定排序 `created_at DESC, task_id DESC`；cursor 绑定筛选 hash。Task Detail 返回冻结 refs、allowed decisions、Claim 摘要、Subject View handle 和三阶段协调状态，不嵌入完整大对象。

Subject View 必须按 Gate 调用 D09 typed Query：

| Gate | View |
|---|---|
| 1 | `GetGate1ReviewView` |
| 2 | `GetGate2ProductionWorldReviewView` |
| 3 | `GetVisualFoundationReviewView` + expected Target matrix |
| 4 | exact Candidate comparison + `GetReferenceCoverageMatrixView` |
| 5 | `GetGate5StoryboardReviewView` |

View result 再重算当前 read set，并明确 `ready|stale|blocked`；它不静默换成新 Subject。旧 Task 可只读展示历史 Subject，Decision 按冻结 hash验证。

### 10.2 Commands

```text
POST /api/v1/human-tasks/{task_id}/claims
POST /api/v1/human-tasks/{task_id}/claim-renewals
POST /api/v1/human-tasks/{task_id}/claim-releases
POST /api/v1/human-tasks/{task_id}/decisions
POST /api/v1/review-decisions/{decision_id}/resume
```

Decision Request 只接受：

```text
claim_token
expected_task_revision
expected_gate_input_hash
expected_subject_revision / expected_subject_hash
decision_payload: HumanGateDecisionPayloadV2
idempotency_key
```

不接受客户端自报 Workspace、Project、Run、Node、wait token、effect plan、Owner command、candidate list、Output refs 或 signal identity。Handler 只做认证、strict JSON 和 Application 调用，不含 GORM、Owner switch 或 Temporal Client。

Decision 处理可同步推进 Effect/Resume，但响应必须分层：

- Decision 提交前的校验错误使用 4xx，且没有 Decision ID；
- Decision 已提交、Effect/Resume 未完成时返回 200/202 coordination envelope，必须携带 immutable Decision ID 和阶段状态；
- 不得在 Decision 已提交后返回一个让客户端误以为“全部回滚”的普通 409/500；
- `/resume` 没有业务 Body，只继续同 Decision 缺失阶段，不能替换任何事实。

### 10.3 只读协调响应

```text
HumanGateCoordinationViewV2
├── task_status / task_revision
├── decision_status / decision_ref?
├── effect_status / completed_step_keys[] / owner_receipt_refs[]
├── resume_status / resume_receipt_ref?
├── gate_output_ref? / branch?
├── stage_blocker_code? / recovery_action
└── view_read_set_hash
```

它是 Query View，不回写 HumanTask，也不创建万能 Gate 状态表。

## 11. stale、影响闭包与任务替换

| 变化 | Decision 前 | Decision 后 Effect 前/中 | 已 Apply 后 |
|---|---|---|---|
| Subject Candidate Head 前进 | Task → STALE | Effect conflict | 旧业务结果保留；下游按依赖 stale |
| Source/Owner read set 漂移 | Task → STALE | gate-specific revalidation；不允许则 conflict | 新 Owner Version/Task |
| Skill Release quarantine/revoke | Task blocked/stale | 尚未 Apply 的 Candidate 不可用 | 已发布 Owner 历史保留 |
| Gate 4 Execution/Selection Head 前进 | Task → STALE | Selection/Result effect conflict | 新 Target round/Result Version |
| Gate 5 其他 Scene Graph 更新 | View 提示 Head 变化 | 允许严格 Scene semantic revalidation | Compiler 正常发布新 Graph |
| Membership/Token 撤销 | claim/decide 拒绝 | resume 重新授权调用者，但后台 recovery 可按系统身份只恢复已提交 Decision | 已提交审计不删除 |

Task 在 Decision 前可以 STALE；Decision 后不再改 Task 去套用新 Subject。确定 conflict 必须保留旧 Decision/Intent/Receipts，Workflow 生成新 Candidate/Gate Input/Task。新 Task 不复制旧 Decision 为批准，也不自动 claim。

变化的影响闭包沿 D06/D10/D08 精确 dependency refs 计算。无关 Scene/Target 的任务不应被项目全局更新时间戳误伤；相同显示名、相似图片或全局 Graph Head变化不能替代 scope proof。

## 12. 失败与恢复

| 场景 | 稳定结果 | 恢复 |
|---|---|---|
| Task/Subject hash 漂移 | `review_subject_stale`，无 Decision | 新 Gate Input/Task |
| Claim token/revision/expiry 错误 | `review_claim_conflict` | 重取/重新 claim |
| Decision payload/allowed set 错误 | `review_decision_contract_invalid` | 修正同 Task 请求 |
| 并发两个 Decision | 一个提交，另一个 `review_decision_already_recorded` | 返回已提交 Decision |
| Decision 提交后进程退出 | Decision recorded，Effect pending | `/resume` 或 recovery worker |
| Owner Command 响应丢失 | Step dispatched，结果 unknown | 按同 owner command key/hash查 Receipt/重放 |
| Gate 1 step 1 成功、step 2 失败 | Project Receipt 保留，Effect applying/conflict | 可恢复错误重做 step 2；baseline 漂移则新 Task |
| Gate 2/3/5 Owner transaction 失败 | 无部分 Owner Version/Receipt | 同输入可重试；确定冲突新 Task |
| Gate 4 Selection 成功、Result 响应丢失 | Selection Receipt 保留 | 同 Result command key对账，不改选 |
| Effect plan release/hash 漂移 | `gate_effect_plan_conflict` | 精确旧 release恢复或人工诊断 |
| Signal 已送达、响应丢失 | Resume Intent unknown | 同 Envelope发送 + History 对账 |
| 重复 Signal 同 hash | Temporal `already_applied` | 写 Resume Receipt |
| 同 signal id 异 hash | `workflow_resume_input_conflict` | 人工诊断，不重发/新建 Workflow |
| Worker/API/Frontend 重启 | PostgreSQL/Temporal 事实保留 | 后台扫描 pending/unknown 或用户 resume |
| Kafka/ELK 不可用 | 业务事实照常提交 | 异步补投/恢复日志链路 |

只有确定未提交的本地校验/事务失败可以普通重试；任何已持久化 Decision/Intent 都必须用原 identity。Recovery worker 只扫描明确 pending/unknown Coordination，不推断业务通过、不自动创造用户 Decision。

## 13. Gate 4 聚合状态与局部推进

Gate 4 的用户阶段状态由 `GetReferenceCoverageMatrixView` 和 Owner checkpoint receipts聚合：

```text
reference_selection_stage_status
= target rows(
    planned | generating | awaiting_selection | selected |
    owner_applied | blocked | stale | optional_skipped | not_generated
  )
+ base_checkpoint_status
+ per_scene_composition_checkpoint_status
+ required_coverage_root
```

不建立可写的 `gate_4_approved=true`。规则：

- Wave A 的 anchor/location/prop 任务可并行；
- Appearance 任务只有 exact anchor AssetVersion 发布后才打开；
- Composition 任务只有 complete base closure 发布后才打开；
- 一个 Target rejected/changes_requested 只阻塞其依赖闭包；
- optional target 无 Selection 可以显示 skipped，但 required 不能；
- 一个 Scene checkpoint 完成不要求无关 Scene 同时完成；
- Gate 4 总完成必须满足 active Plan 全部 required coverage 和所有 active Scene checkpoint。

Frontend 所谓“Reference Selection Gate”是这一聚合 View 上的稳定产品阶段；每一行的 HumanTask/Decision/Effect/Resume 仍独立可追溯。

## 14. 安全、日志、事件与保留

- Claim Token、Access Token、Provider Secret、Prompt、剧本全文、图片 bytes、私有 Artifact URL 不进入日志、Trace、Metric label、Outbox、Temporal Search Attribute 或 Coordination View；
- Decision user_note 做长度/字符/恶意内容约束，只作为审计数据，不直接进入 Agent system prompt；
- Task Subject View 的媒体通过短时只读 preview handle，绑定用户/对象/用途，不进入 Decision identity；
- Owner Command 只从服务器持久事实投影，不能读取客户端额外 JSON；
- HumanTask/Decision/Effect/Resume 的日志只记录低敏 ID、gate key、状态、耗时、错误码和 trace id；
- Kafka 只从 Outbox发布 `HumanTaskOpened|ReviewDecisionRecorded|GateEffectCompleted|WorkflowGateResumed` 投影事件；Consumer 不执行 Decision/Owner/Signal；
- Temporal History 保存 Resume Envelope refs/hash 和 branch，不保存完整 Subject/Candidate/媒体；
- Task/Decision/Receipt 是审计事实不清理；大 Subject/Candidate/媒体遵循各 Owner retention，但已被 Decision/Owner Version 引用的内容必须 pin 或可内容寻址重读。

## 15. 前端契约边界

`VP-D12` 才定义页面，但 Human Gate API 必须支持：

- 五 Gate 时间线与当前 scope/target/scene；
- Task `OPEN|CLAIMED|COMPLETED|CANCELLED|STALE`；
- Decision recorded 的 immutable ID；
- Effect pending/applying/completed/not_required/conflict 与已完成 step；
- Resume pending/unknown/completed/conflict；
- Gate 4 coverage matrix、依赖 blocker 和 checkpoint；
- “决议已记录”“业务效果已提交”“工作流已继续”三种不同文案；
- stale/impact drawer 展示旧 Subject、变化原因、影响闭包和新 Task link。

MVP 使用 Query polling/精确 invalidation 即可；不要求 SSE、WebSocket 或通知中心。未知 subject renderer 只读并禁用 Decision，不能猜测 allowed actions 或 Effect Plan。

## 16. MVP、Platform Complete 与迁移

### 16.1 当前 MVP 必交

1. 五 Gate strict Subject/Decision/Effect/Resume 全链；
2. Gate 1 两步有序恢复、Gate 2/3/5 原子 Owner Apply；
3. Gate 4 每 Target 选择→正式 Result，聚合 checkpoint 不伪造第六 Gate；
4. changes_requested 产生新 Candidate/Task，不改旧 Subject；
5. Decision、Effect、Resume 三状态可独立查询；
6. 进程/Worker/Temporal/Browser 重启后同 Decision 恢复；
7. stale/conflict/unknown 不产生第二业务效果；
8. Backend deterministic blocker 不可由人工越过；
9. 真实多 Scene、多形象、道具 Interaction、Reference Selection 和 Storyboard 全链可追溯。

### 16.2 Platform Complete 保留目标

- 通知、指派、SLA、批量运营、移动审核和多角色审批策略；
- 跨项目 Review Inbox、高级审计导出和合规留存策略；
- 经新 Design 接受的多方会签、审批撤销补偿或外部审批系统连接。

### 16.3 原子迁移

新 Workflow cutover 后只创建五类 v2 Gate Input/Task。旧 Bible-first、Episode/Planning 分裂、Storyboard Intent/Detail、`reference_asset_candidate_set`、Shot Frame/Video Gate 只由精确旧 runtime 恢复或明确终止；v2 Coordinator 不领取、不映射、不 fallback。旧非终态引用归零后按独立任务删除旧 Subject/executor 路由。

迁移不改写历史 HumanTask/Decision/Receipt，不把旧 Decision伪装成新 Gate 通过，不双写 v1/v2 Task。

## 17. 验收门

`VP-D15` 接受前不修改代码。未来 Requirement/Plan/Acceptance 至少必须机械证明：

1. 五个 gate key 与五个 explicit Effect Plan 一一对应；未知 gate/subject/decision/executor fail closed；
2. Gate Input→Task→Decision→Owner Receipt→Effect Bundle→Resume Envelope/Receipt 的 hash 图单向无环；
3. Task 只引用 immutable Subject，Candidate/Head/read set 漂移在 Decision 前 STALE、Decision 后 Effect conflict；
4. Gate 1 第一步成功/第二步崩溃恢复只复用 Project Receipt，不创建第二 Episode/Decision；
5. Gate 2 Bible/Planning/Asset、Gate 3 Preset/Reference、Gate 5 whole Scene 分别按接受事务边界无部分提交；
6. Gate 4 每个 Target 一个 Task，Selection 成功/Result 响应丢失恢复不改选；base 与 composition 正式结果类型精确；
7. Gate 4 overall complete 只能由 base/per-scene checkpoints 和 required coverage 得出；不存在全局批准字段/Task；
8. selected Bundle 属于冻结 CandidateSet、完整 QC/Vision 围栏满足、warn/not_assessable 全确认；
9. approved/selected/changes_requested/rejected strict payload 对额外字段、错误候选、非法 change operation 和自由文本提权全部拒绝；
10. Decision 提交前错误无 Decision；提交后 Effect/Resume 故障仍返回 Decision ID/分层状态；
11. Effect step 在 Owner 响应未知时只用同 command key/hash查 Receipt，不重置 dispatched 或重做已完成 step；
12. Temporal Signal 已应用但响应丢失时，同 Envelope/History 对账得到 already_applied；同 signal id 异 hash 冲突；
13. claim revision/token/expiry、并发接管、Token Version 撤销和跨租户引用全部失败关闭；
14. rejected/changes_requested 无正向 Owner Version/Selection/Provider Call，repair 只消费 typed change spec；
15. Gate 5 其他 Scene Graph Head 前进只在严格 Scene semantic revalidation 等价时允许旧 Decision Apply；
16. Kafka/ELK/Frontend 不可用不改变 Decision/Effect/Resume，日志和 History 无 Claim Token、Secret、Prompt、原文或媒体 bytes；
17. 一个真实多 Scene 剧本依次完成 Gate 1–5，并能从 formal Shot 反查每个 Decision、Effect Receipt、Owner Version、Reference Selection 和原文 Evidence；
18. 全量 CI、真实 PostgreSQL/Temporal 重启、Compose、权限/Secret/日志 hygiene 与浏览器旅程按当时 Plan 通过。

本 Design 通过独立评审和提交后只解锁 `VP-D12`：在前端功能设计中固定 Guided Studio 页面、五 Gate 时间线、Reference coverage matrix、Candidate compare、状态文案与影响抽屉。它不授权提前修改 Workflow、Review、Owner、Temporal、API 或前端代码。
