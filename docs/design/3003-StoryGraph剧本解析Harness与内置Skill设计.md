# StoryGraph 剧本解析 Harness 与内置 Skill 设计

- 状态：已接受（`SG-D02`）
- 日期：2026-08-26
- 接受日期：2026-08-27
- 系统设计：[StoryGraph 内容图与 DAG 创作画布设计](0010-StoryGraph内容图与DAG创作画布设计.md)
- 冻结现状输入（非规范前置）：[项目制作圣经生成执行框架设计](3001-项目制作圣经生成执行框架设计.md) · [本地 Codex 分镜智能体执行框架设计](3002-本地-Codex-分镜智能体执行框架设计.md)；本设计唯一规范前置是已接受的 0010
- 运行边界：[后端服务架构](2001-后端服务架构.md) · [剧本到分镜 MVP 垂直切片设计](0009-剧本到分镜MVP垂直切片设计.md)
- 后续门禁：本次接受只解锁 `SG-D03`；在 `SG-D20/SG-D21` 的统一 Plan 与全未勾选 Acceptance 建立前，不移动目录、不修改 Execution Policy、不新增 Invocation Kind，也不建立兼容入口

## 设计结论

最终目标是把项目自有剧本分析能力从仓库根目录 `.agents/skills` 收口到 Agent 应用自己的一个版本化 StoryGraph Skill Bundle：

```text
agent/skills/build-storygraph/SKILL.md
```

这个目标通过两次各自完整、原子的切换实现：第一个任务只把现有 8 个 Skill 按原名、原字节迁到 `agent/skills` 并一次性切换 Loader/Docker；第二个任务才把 8 个过渡入口收口为 `build-storygraph/SKILL.md` 和 References，并一次性切换 Stage 调用者。任一提交运行时都只有一条路径，不存在双读 fallback。

最终的 `SKILL.md` 是 Bundle 唯一入口和总边界；各阶段详细规则放在它显式引用的 `references/` 中，由 Harness 按阶段只注入需要的文件。最终运行时不再扫描根目录 `.agents`，不保留旧路径 fallback，也不保留当前没有消费者的 `agents/openai.yaml`。

Agent 内实现 `StoryGraphHarness`，负责 Codex Candidate Schema、按阶段装载 Skill、有界模型调用、候选规范化和局部 Review/Repair；Go Backend 拥有 Invocation Envelope、Stage 枚举、AgentDefinitionManifest、Execution Policy、预算和全部正式业务写入。已有 Workflow Compiler 把宏观 Stage/Human Gate 编译为稳定 WorkflowDefinition Node，Temporal 只解释这张正式执行 DAG；Stage 内动态 shard 是对应 NodeRun 下的 Backend-owned AgentInvocation 子任务，不成为隐藏的第二张执行图。

```text
WorkflowDefinition / Temporal
  稳定宏观 Node、依赖、Human Gate、NodeRun/恢复
                    ↓ signed stage invocation
Backend Agent Owner
  Stage/Shard、Envelope、Policy、Invocation、SQL 事实
                    ↓ strict wire contract
Agent StoryGraphHarness
  Skill guidance、Codex CLI、Candidate schema、candidate-only
                    ↓ candidate + hash
Backend
  重验 Wire/Evidence/DAG → 人工审核 → Owner Command → StoryGraph Compiler
```

首版不使用 LangGraph 再编排一套完整流程。Temporal 已是唯一跨步骤持久工作流引擎；Agent 内每个 `stage + shard_key + input_hash` 实例保持有界和可重放，普通 Skill Registry 足够。只有未来出现一个 Invocation 内确实无法用清晰代码表达的有限推理分支，并有独立验收后，才重新评估 LangGraph。

## 当前事实与缺口

当前项目有 8 个 Skill：Bible 的 extract/reconcile/review，以及 Storyboard 的 analyze/plan/draft/review/repair。真实运行却只有：

```text
Production Bible: extract → reconcile → review
Storyboard:       draft-shots（单次调用）
```

现状存在以下缺口：

- Skill Loader 固定读取 `.agents/skills/{name}`；Docker 也复制根目录 `.agents`；
- `agents/openai.yaml` 没有任何运行时消费者；
- Storyboard 的 analyze/plan/review/repair Skill 没有进入生产调用链；
- Production Bible 没有真正按上下文窗口切块，集号识别也不能覆盖完整 60 集格式；
- 当前 AgentInvocation 只在整次调用前后持久化，无法从 evidence/reconcile/review 中间阶段恢复；
- 分集和场景解析主要是 Backend 行级规则，不具备带证据的深度剧情分析；
- Storyboard 输入资产固定为空，无法验证角色形象和场景一致性；
- 只有 Skill Version 字符串，没有验证实际 Markdown 内容的 Bundle Hash；
- 当前 `storyboard_draft` 只返回 Shot 列表，缺少可审核、带证据且可路由到唯一 Owner 的 Shot Intent/关系候选；Agent 未来仍不返回可直写正式图的 Fragment/Patch。

这些事实用于确定迁移和实现影响，不作为“已经实现”的证据。

## 目标

- 用一个项目内置 Skill Bundle 支撑全文证据、Bible、分集、场景、节拍、实体、关系、分镜和审核全流程。
- 每个 AI stage instance 都有 Backend-owned Stage Key、Shard Key、输入 Hash、结果 Hash 和独立 Candidate Schema。
- Backend 持久化每个 stage instance、shard 与聚合结果，Workflow/Temporal 按正式宏观 Node 恢复、重试和局部重跑，不把完整剧本塞进一个超长 Invocation。
- 所有输出都是 Candidate 或受冻结边界约束的 CandidateRepairPatch；正式 ID、版本、关系、角色卡和 Shot 由 Backend 分配和写入。
- 让角色/场景 occurrence、AssetState（产品显示为 Appearance Variant）和精确视觉版本成为分镜输入，而不是自由文本或空资产数组。
- 启动时验证 Bundle 完整性和 Hash，运行中不依赖外部 Skill 仓库、用户级 Codex Skill 或工作区文件读取。

## 非目标

- 不建立通用 Agent 平台、动态插件市场、任意 Tool Registry 或第二套 Workflow Engine。
- 不让 Codex 访问 Shell、Web、浏览器、文件写入、数据库、对象存储或 Backend 业务写接口。
- 不让一个巨大 Prompt 同时完成全文理解、分集、场景、分镜、审核和图片生成。
- 不在 Agent 内生成正式 UUID、持久 Checkpoint、Human Decision、AssetVersion 或 StoryGraphVersion。
- 不让文本 Harness 直接调用图片/视频 Provider；视觉资产仍由 Generation/Asset Workflow 生产。
- 不为旧 `.agents/skills`、旧 Skill 名称或旧 Invocation Kind 建兼容 fallback。

## 目标目录与所有权

目录只在对应实现切片有真实消费者时建立：

```text
agent/
├── skills/
│   └── build-storygraph/
│       ├── SKILL.md
│       └── references/
│           ├── source-evidence.md
│           ├── story-analysis.md
│           ├── episode-segmentation.md
│           ├── entity-reconciliation.md
│           ├── scene-structure.md
│           ├── storyboard-table.md
│           ├── shot-detail.md
│           ├── continuity-review.md
│           └── visual-identity.md
├── app/modules/storygraph/
│   ├── candidate_schemas.py
│   ├── skill_registry.py
│   ├── harness.py
│   └── gates.py
└── tests/
    ├── unit/test_storygraph_harness.py
    ├── contract/test_storygraph_wire.py
    └── integration/test_storygraph_codex.py
```

职责：

| 位置 | 职责 | 禁止承担 |
|---|---|---|
| `agent/skills/build-storygraph` | 原始项目指导、阶段规则、边界和许可证追溯 | Python 代码、Schema 副本、业务状态 |
| `candidate_schemas.py` | Codex Candidate 输出的 Pydantic 严格联合类型和临时 JSON Schema | Invocation Envelope、Stage/Policy/预算的第二定义 |
| `skill_registry.py` | 接受 Backend Stage Key 后映射到 Skill Reference 与 Candidate Schema | Stage 枚举、调用预算、动态插件发现 |
| `harness.py` | 调用 Codex、装载指定 Reference、执行有限修正、返回 Candidate | Temporal、持久化、人工批准 |
| `gates.py` | Agent 侧快速 Schema/引用/覆盖校验 | 正式 StoryGraph 发布裁决 |
| `agent/tests/{unit,contract,integration}` | Bundle、跨语言合同、阶段、失败与真实 Codex 合约测试 | 与生产代码混放的测试 |

Backend 对应能力必须位于 `backend/internal/production/storygraph`、`backend/internal/agent` 和 `backend/internal/workflow`；Agent 不承担后端服务职责。

## 一个 Skill Bundle 的组织方式

### `SKILL.md`

唯一入口只保存全阶段都成立的规则：

- candidate-only；
- 只使用冻结输入和明确 Evidence；
- 不创建正式 ID 或声称已持久化；
- 不调用任何 Tool/网络/文件；
- 输出严格遵循 Harness Schema；
- 不补写来源未支持的剧情、关系、外观或镜头；
- Asset identity、AssetState、EffectiveStyleSnapshot、Artifact 四轴分离；
- 所有关系输出必须可转换为 StoryGraph DAG 或 Claim Node。

### `references/`

每个 Reference 只描述一个阶段的创作和质量规则。Harness 必须显式映射并只注入当前 Stage 所需文件，禁止像现状一样递归拼接 Bundle 中全部 Markdown。

例如：

| Stage | 注入 Reference |
|---|---|
| `extract_source_evidence` | `source-evidence.md` |
| `analyze_story` | `story-analysis.md`、`entity-reconciliation.md` |
| `reconcile_story` | `entity-reconciliation.md`、`story-analysis.md` |
| `segment_episodes` | `episode-segmentation.md` |
| `analyze_episode` | `scene-structure.md`、`visual-identity.md` |
| `reconcile_episode` | `scene-structure.md`、`continuity-review.md` |
| `draft_storyboard` | `storyboard-table.md`、`visual-identity.md` |
| `detail_shots` | `shot-detail.md`、`visual-identity.md` |
| `review_storygraph` | `continuity-review.md`、当前目标阶段 Reference |
| `repair_candidate` | `continuity-review.md`、只读 Issue 与允许修改的 Candidate 边界 |

不保存生成出来的 JSON Schema 文件；Pydantic Model 是 **Agent 内 Codex Candidate 输出 Schema** 的唯一代码事实，Codex CLI 在调用时读取临时生成的 Schema。Invocation Envelope、Stage 枚举和 Execution Policy 的唯一 Owner 是 Go Backend；Python 只做严格消费校验，不能扩展字段或放宽预算。Go/Python 通过同一组版本化 canonical wire fixture、成功/失败 fixture 和拒绝 fixture 防止漂移，不再各自维护一份 Definition Manifest。

## Bundle 版本和完整性

Backend `AgentDefinitionManifest` / Execution Policy 必须同时冻结：

```text
definition_version
prompt_version
skill_bundle_version
skill_bundle_hash
output_schema_version
model_capability
allowed_tools=[]
max_model_calls
max_execution_seconds
```

`skill_bundle_hash` 由 Bundle 根目录下允许的文本文件按相对路径排序后，对“路径 + 长度 + 原始 UTF-8 字节”做 Canonical SHA-256。启动时 Agent 重新计算并与 Invocation Policy 比较；文件缺失、多余、Hash 漂移或路径逃逸时 fail closed。

Bundle Version 表达人工接受的语义版本，Bundle Hash 证明运行的实际字节。只改 Version 不改内容、只改内容不更新 Backend Policy 都必须被契约测试拒绝。收口任务完成后的 Agent 源码只有 `agent/skills/build-storygraph/SKILL.md` 这个运行入口；每个发布镜像按 digest 冻结它的实际 Bundle 字节。

长流程恢复必须命中原 `skill_bundle_hash`：新部署可以增加新 hash，但所有仍被非终态 AgentInvocation 引用的旧 Agent 镜像 revision 必须继续可路由并可按同一 image digest 重启，直到引用归零后才允许回收。Dispatcher 按 Backend 冻结的 Bundle Hash 选择 runtime revision；找不到精确 hash 就返回 `skill_bundle_unavailable`，不得悄悄使用当前目录、旧名称 fallback 或相近版本。这是可重放性保留，不是业务兼容层。

## Harness Stage Contract

### 公共输入包络

每个 stage instance Invocation 至少包含：

```text
invocation_id
kind = storygraph_stage
wire_schema_version
input_hash
execution_policy
payload {
  stage / shard_key
  workspace_id / project_id
  source_refs[] {
    owner_kind, owner_logical_id,
    owner_version_id, revision, hash
  }
  base_storygraph_version_id / base_storygraph_hash（如已有）
  upstream_candidates[] {
    stage, shard_key,
    candidate_revision_id, candidate_revision_hash,
    source_invocation_id, source_result_hash
  }
  shard_manifest_ref { manifest_id, version, hash }
  shard { kind, key, tree_path, parent_key?, absolute_start?, absolute_end? }
  stage_input
}
```

`input_hash` 不是只对自由 payload 取 Hash。它对以下 Canonical Material 统一计算，不包含 invocation id 或自身：`wire_schema_version`、完整 payload、排序的 source/upstream Candidate Revision refs、Shard Manifest/Tree Path，以及 Execution Policy 中的 definition/prompt/bundle version + bundle hash/output schema/model policy/Codex runtime contract/allowed tools/调用预算/技术 deadline。任一 Guidance、Schema、模型策略、上游 Candidate Revision 或分片计划变化都必须产生新 input hash。

`stage_instance_key = SHA-256("storygraph-stage-v1" + stage + shard_key + shard_manifest_hash + input_hash)`。同一身份只能重放同一个 Invocation/Result，不因 Retry 创建新 Candidate；同一身份出现不同 Result Hash 必须冲突失败，不能挑最后到达者。

Agent 只能使用包络中的值。Payload 不携带数据库凭据、对象存储凭据、浏览器 Session 或任意业务写 Grant。

### Shard Manifest 与重分片

Shard 不是临时内存数组。Backend 为每个可 fan-out 的宏观 NodeRun 保存不可变 `ShardManifest`：

```text
manifest_id / version / parent_manifest_hash / manifest_hash
workflow_run_id / node_run_id / stage / root_input_hash
shards[] {
  shard_key, tree_path, parent_shard_key?, kind,
  logical_range_or_cluster, context_overlap,
  source_hashes[], status=active|superseded
}
coverage_hash
```

初始分片和 reduce tree 的排序、fan-in 与 `tree_path` 都由 Backend 确定性生成。某个 shard 超预算时不能原地改边界：Backend 创建下一 Manifest Version，新增覆盖父 shard 完整逻辑范围的有序子 shard，并在新 Manifest 中把父 shard 标为 superseded；除显式只读 overlap 外，子 shard 必须无缺口、无重复覆盖。旧 Invocation/Result 保留审计但不再进入聚合，子 shard 因新 Manifest Hash 获得新的 stage instance identity。

Story Analysis 的候选输入另外冻结 `candidate_item_range=[start,end)`。Backend 按 Schema 中的稳定字段顺序展开候选条目：Source Evidence 使用 `observations -> review_issues`，Story Analysis 使用 `entities -> world_entries -> claims -> arcs -> review_issues`，Story Reconciliation 使用 `canonical_entities -> canonical_world_entries -> merged_claims -> merged_arcs -> conflicts -> review_issues`。超预算时只允许在该有序条目序列上把父区间确定性二分；两个子区间必须首尾相接且完整覆盖父区间，Stage Input 只携带各自区间对应的合法候选子集，不能携带完整父候选后仅在提示词中要求忽略一部分。单个条目已经不可再分时返回稳定的不可分错误，不进行同输入重试、截断或扩大预算。

重分片只替换失败 leaf/reduce shard 及其到 root 的祖先路径，未受影响的子树 Key、Candidate Revision 与成功回执不变。发布下一版本时，Backend 可以把发布前已经 `succeeded`、Candidate Head 仍为 current 且来源校验通过的未受影响子树作为新树的显式不可变引用；发布后才返回的旧版本结果一律只保留审计。新路径的每个 Invocation 必须引用新 Manifest Hash，聚合仍以新版本的 active tree、精确 Candidate Revision 和 Head 为准。

只有当前 Manifest 的全部 active leaf shard 均有 current Candidate Revision、其来源 Invocation Result 唯一且成功、上游仍为 current，且 coverage/tree gate 通过时，Backend 才能以 expected manifest hash 与 expected Candidate Head Hash CAS 发布一个聚合 Candidate Revision。Manifest 漂移、旧 shard 迟到或任一 leaf 失败/unknown 时不得产出 `node-output-v1`；重复聚合只能重放同一 Aggregate Candidate Revision Hash。

### 公共输出包络

```text
invocation_id
kind = storygraph_stage
wire_schema_version
stage / shard_key
status = succeeded | failed | unknown
candidate_type
candidate（仅 succeeded 非空）
input_hash
result_hash（仅 succeeded 非空）
issues[]
executor { name, version, model }
error { code, summary, retryable }（仅 failed/unknown 非空）
```

成功时 Candidate/Result Hash 必填且 `error=null`；failed/unknown 时 Candidate/Result Hash 必须为空且稳定 Error 必填。Agent Invocation Result 一经保存永远不可修改；同一 stage instance 重投只能重放同一个 Result Hash。Candidate 内只使用来源给定的正式 Ref 或临时 Key。Backend 在 Owner Apply 时分配正式 UUID，并返回 temporary key → owner ref → story node key 的 Receipt 映射。

### Candidate Revision 与 Repair Head

Agent 的 `result_hash` 证明一次不可变 Invocation 输出；可修复业务候选使用 Backend-owned 不可变 `StageCandidateRevision`，两者不能共用“Result”语义：

```text
StageCandidateRevision
├── id / stage_instance_key / revision_no
├── parent_candidate_revision_id / parent_candidate_revision_hash（首版为空）
├── origin_kind = invocation | aggregate | repair
├── invocation_origin { source_invocation_id, source_result_hash }?
├── aggregate_origin { shard_manifest_id, manifest_version, shard_manifest_hash,
│                       sorted_leaf_candidate_revision_refs[] }?
├── repair_origin { repair_invocation_id, repair_result_hash }?
├── candidate / candidate_content_hash / candidate_revision_hash
└── created_at

StageCandidateHead
└── stage_instance_key / current_revision_id / current_candidate_revision_hash / revision
```

三种 origin 是严格互斥联合，恰好一个非空。首次成功 Invocation Result 以 `invocation` origin 原子创建 Candidate Revision 1；Shard 聚合以 `aggregate` origin 创建聚合 Revision，其 leaf refs 按 `(stage_instance_key, shard_key, candidate_revision_id)` 排序且每项冻结 revision hash；Repair Invocation 只返回不可变 Patch Result，Backend 将它应用到 expected current Candidate Revision 后以 `repair` origin 创建 N+1，并 CAS 切换 Candidate Head。`candidate_content_hash` 只证明规范化候选内容；`candidate_revision_hash` 还覆盖 stage instance、revision number、parent revision hash、origin kind、该 origin 的全部 Canonical Material 和 content hash，因而同时证明调用、聚合或修复谱系。原 Manifest/Invocation/Result/Revision 永不覆盖。下游与 Shard 聚合只绑定精确 `candidate_revision_id + candidate_revision_hash`；Head 切换后，引用旧 Revision 的下游 stage instance 才被标记 stale。

`invocation` 与 `aggregate` origin 只允许 `revision_no=1` 且无 parent；`repair` origin 只允许 `revision_no>=2`，必须同时冻结 parent revision id/hash 与成功的 `repair_candidate` Invocation/Result hash。Backend 的修复发布原语只接收上层已经按冻结允许集生成并完成确定性校验的新 Candidate：在一个 GORM/PostgreSQL 事务中锁定 Head，重验 expected revision id/hash/revision、父 Revision 与 Repair Result，拒绝无内容变化，创建不可变 N+1 后以相同 expected 条件 CAS 切换 Head。该原语不自行解释 Patch、不创建 Receipt、也不扫描下游；Patch 允许集、幂等 Receipt、stale closure 和有界重审继续由同一业务 Coordinator 在后续交付单元内完成，避免底层 Adapter 变成第二业务编排层。

### Stage 列表

| Stage | Shard/输入 | Candidate 输出 | 正式应用 Owner |
|---|---|---|---|
| `extract_source_evidence` | 一个绝对区间脚本 Slice、Episode marker hints | Evidence Observation Fragment | Production Bible |
| `analyze_story` | 一个有界剧情区间或实体簇、已校验 Evidence | Story Arc/Entity/State/World/Relationship Claim Fragment | Production Bible |
| `reconcile_story` | 同层有界 Candidate refs/hash；按确定性树逐层 reduce | 规范 Entity Key、跨片段 Claim/Arc 合并 Candidate | Production Bible |
| `segment_episodes` | marker/evidence 的有界索引、目标时长 | Episode Boundary Candidate | Planning |
| `analyze_episode` | 一个 Episode 内的脚本 Slice、scene marker hints、相邻上下文、已确认 Bible Snapshot | Scene、Dialogue、Beat、Occurrence、Continuity Claim Fragment | Planning 的 Episode Structure |
| `reconcile_episode` | 一个 Episode 的有序 Scene Fragment refs/hash | Episode Structure Candidate | Planning |
| `draft_storyboard` | 一个 Scene 的 Beat/Occurrence、Specification/AssetState、EffectiveStyleSnapshot、时长策略 | Storyboard Row/Shot Intent Fragment、资产需求 | Storyboard |
| `detail_shots` | 一个 Scene/Shot batch、已接受 Intent、相邻边界、精确 AssetVersion | 完整 Shot Detail/ShotProductionBinding Candidate | Storyboard |
| `review_storygraph` | 一个 Lens/影响闭包 shard + deterministic gate result | Evidence-scoped Review Issues | Review |
| `repair_candidate` | 目标 Issue、冻结 Candidate Revision hash、允许修改 Key 与 fragment hash | `CandidateRepairPatch` | 无；Backend 创建下一 Candidate Revision 后再审核 |

模型成功永远不自动调用正式应用 Owner。每个业务阶段都必须先通过 Backend 重验；需要人工判断的实体合并、分集边界、关系、结构、分镜和视觉版本继续使用独立 Human Gate。

## 全流程编排

### WorkflowDefinition/Temporal 宏观流程

```text
normalize source (Backend deterministic)
→ fan-out extract_source_evidence by immutable absolute slices
→ validate evidence
→ bounded analyze_story map
→ deterministic tree reconcile_story
→ review_storygraph(scope=bible)
→ Human Gate: Bible / identity / state / claim
→ Backend Owner: materialize Character/Location Asset + Specification + AssetState
→ segment_episodes
→ Human Gate: episode plan
→ materialize Episode and published ScriptVersion
→ fan-out bounded analyze_episode + reconcile_episode
→ Human Gate: scene / beat / occurrence batch
→ compile Core StoryGraphVersion
→ fan-out draft_storyboard by scene
→ Human Gate: shot intent / visual requirements → Storyboard Owner FreezeIntentSet Receipt
→ Backend Generation: character/location composite reference sheets
→ Artifact READY/QC → Human CandidateSelection → publish AssetVersion
→ fan-out detail_shots by scene/shot batch with exact AssetVersion
→ review_storygraph(scope=storyboard)
→ bounded repair_candidate（必要时）
→ Backend apply CandidateRepairPatch / create next Candidate Revision / CAS Head
→ deterministic gates + review affected closure（直到无 blocker 或预算耗尽）
→ Human Gate: storyboard batch
→ Owner Apply
→ compile next StoryGraphVersion
```

WorkflowDefinition 必须显式包含上述宏观业务节点及 Human Gate，每个节点产生稳定 NodeRun；Temporal 保存这些节点的等待、Retry、Timer 和 Signal。每个宏观 Node 内的 Shard Manifest、AgentInvocation、Candidate Ref、Owner Receipt 和恢复状态由 Backend PostgreSQL 保存并全部挂到该 NodeRun，不能动态伪造 Workflow Node/NodeRun。节点只有在当前 Manifest 全部 active shard 成功、旧 Manifest 输出被排除、聚合 Candidate Revision Hash 已 CAS 固化后才输出 `node-output-v1`；Agent 不保存跨请求 Checkpoint。

### 为什么不做一个超长 Invocation

完整剧本可能持续数十分钟或数小时。把所有阶段放进一个 Agent HTTP 请求会导致：

- 中间结果无法由 Backend 持久化和单独恢复；
- 任何后段失败都可能重跑全文；
- 600/900 秒 Invocation 上限与完整任务冲突；
- Human Gate 无法插入；
- 一个 Skill Prompt 会承载不相关规则并膨胀上下文。

因此一个 `stage + shard_key + input_hash` 实例对应一个独立 AgentInvocation。完整流程的“一个 Harness”指统一 Skill Registry、Candidate Schema、Bundle 和调用协议，不是一个进程内巨型模型调用，也不是“一个 Stage 名称全剧只调用一次”。

## 全文切片与证据坐标

原稿 Normalize、格式检测和不可变 DocumentRevision 仍由 Backend 负责。Backend 以段落/场景/显式 Episode marker 为优先边界生成确定性 Slice：

- 每个 Slice 固定 `DocumentRevision.normalized_text` 上的 Unicode code-point absolute half-open range `[start,end)`；Go/Python 不使用 UTF-8 byte offset 或 UTF-16 code unit 混算；
- 边界需要上下文时只增加显式 overlap，并保留原始绝对坐标；
- Evidence 必须逐字等于 DocumentRevision 对应区间；
- 相同区间 Evidence 以 range + text hash 去重；
- Slice 大小由接受后的 Requirement 固定，不按模型临时猜测；
- 不用模型返回的 chunk-local offset 直接写正式 Evidence；
- 60 集 marker 解析不能再局限于中文一至十，格式体检必须覆盖阿拉伯数字、中文数字和既有脚本样本。

Episode segmentation 优先消费显式 marker 和确定性体检；AI 只对缺失、歧义或用户要求重构的边界生成 Candidate，不能覆盖已确认来源。

`analyze_story` 也不能接收“全部 Evidence”。Backend 先按剧情区间/实体簇建立有界 map shard，再让 `reconcile_story` 只消费同层 Candidate 的规范 Key、Candidate Revision Hash、必要 Evidence Ref 和冲突摘要，按固定 fan-in 构造确定性 reduce tree。任何 map/reduce 输入超预算都必须重新分片并产生新的 Shard Manifest，禁止截断输入或临时扩大上下文。`review_storygraph` 使用相同原则按 Lens/影响闭包分片；`detail_shots` 按 Scene/Shot batch fan-out，并显式携带前后相邻边界，最后另做跨集连续性 Review。

## 深度剧本分析

`analyze_story` 和 `analyze_episode` 允许提取以下有证据 Candidate：

- 角色、地点、道具、服装、声音、视觉主题和别名；
- 稳定身份与 AssetState（Appearance Variant）；
- 角色目标、冲突、关系主张、阵营和阶段变化；
- Story Arc、Plot Thread、Turning Point、Hook、Foreshadowing/Payoff；
- Scene、Dialogue、Action、NarrativeBeat 和情绪/信息/权力变化；
- Character/Location/Prop Occurrence；
- 连续性输入/输出、道具持有、空间方向和未决风险。

每项必须引用 Evidence 或已确认上游 Key。Relationship、因果、伏笔等使用 Claim Candidate，包含 subject/object/anchor refs、valid scope、polarity、status 和可选 supersedes；Backend 先通过对应 Production Bible/Planning Owner 应用，经确认后才由 Compiler 投影为 Claim Node。StoryGraph 不是 Claim Owner，Agent 也不允许返回可能使 DAG 成环的任意持久 Edge。

## 分镜表与 Shot 细节

`draft_storyboard` 先从已确认 Scene/Beat、Occurrence、Character/Location Specification、AssetState 和 EffectiveStyleSnapshot 产生分镜表行与资产需求，不要求此时已有视觉 AssetVersion。Backend 随后完成参考图生成、QC、单一 composite CandidateSelection 和 AssetVersion 发布；`detail_shots` 只在所需精确版本 READY 后补全可生产字段。二者不得合并成一次自由生成，缺资产时状态是 `needs_asset`，不能用自由文本或“最新版本”继续。

分镜表行至少包括：

- source beat/scene/episode references；
- shot purpose；
- proposed duration；
- scale、angle、movement、composition；
- action、dialogue、sound 和 performance intent；
- continuity in/out；
- Character/Location/Prop Occurrence；
- required AssetState / view roles；Draft 阶段可为 `needs_asset`，Detail 阶段必须给出精确 AssetVersion；
- first/key/last frame intent；
- risk codes 和 review issues。

Backend 负责全局镜号、连续 timecode、来源覆盖、精确 ID 映射、Hash 和正式 ShotProductionBindingVersion。Agent 不能选择未提供的 AssetVersion，也不能用自由文本代替缺失三视图或地点参考。ShotProductionBindingVersion 是 Storyboard Owner 发布的完整不可变输入集合；生成画面被选中后仍由现有 ShotImageBindingVersion 保存输出结果，两者不能复用 Record 或 Writer。

## 角色卡与视觉资产职责

StoryGraph Harness 只提出：

- Character Specification Candidate；
- AssetState Candidate；
- Location Specification/State Candidate；
- 镜头需要的视图和视觉约束；
- 三视图一致性 Review Issue。

它不生成图片。正式流程为：

```text
confirmed CharacterSpecification + AssetState + EffectiveStyleSnapshot
→ Backend Generation Intent
→ Image Provider composite reference_sheet CandidateSet
→ Artifact Readiness/QC + front/profile/back role coverage
→ Human selects exactly one Candidate
→ published AssetVersion with semantic view-role bindings
→ StoryGraph asset_version / artifact nodes + Character Look typed lens
```

同一角色的不同 AssetState（产品显示为 Appearance Variant）仍引用同一个角色身份；同一 AssetState 在不同 EffectiveStyleSnapshot 下形成不同 Look/AssetVersion，不创建重复角色卡。

## 本地 Codex 执行器

本地开发继续使用 Codex CLI：

- `--ephemeral`；
- `--sandbox read-only`；
- 临时工作目录；
- `--ignore-user-config`；
- 临时生成的 `--output-schema`；
- Tool Allowlist 为空；
- 禁用 Shell/Web/Browser/Plugins/Skill Search/Workspace 工具；
- Model 只来自 Backend 冻结策略或 Candidate Runtime 允许的 CLI 默认值，不读取用户级或项目级 Codex 配置；
- 每个 shard 使用 Backend 冻结的模型调用次数与单次/单 shard 技术 deadline。

Skill Markdown 由 Harness 显式读取并注入，不依赖 Codex 用户目录的自动 Skill 发现。运行时工作目录没有仓库源码，Codex 也不得通过文件工具读取 Skill；实际 Guidance 字节由 Bundle Hash 证明。

完整 Temporal WorkflowRun 不设置业务墙钟上限。单次 Codex call 或单 shard 技术 deadline 耗尽只让该 stage instance 进入可恢复失败/unknown，已成功 shard 和 Human Decision 保持不变；不得把整剧伪装成完成、丢弃 checkpoint 或自动放宽预算。

`execution_deadline_exceeded` 不进入自动领取集合。Story Analysis 宏观 Node 保持 `RETRYING`，Temporal 继续使用持久 Timer 轮询 Backend 事实；有写权限的成员必须通过显式、幂等的 NodeRun Recovery Command 恢复当前 Manifest 中的 deadline 失败。该 Command 只把失败的原 `AgentInvocation` 重新置为 `queued`，不创建新 stage identity、不改 Input/Execution Policy/Manifest、不清除成功 Invocation/Candidate Revision/Decision/Owner Receipt。重新领取时沿用原 Invocation ID 并递增 `claim_version`，旧 Worker 的迟到结果因此不能成为正式效果。若当前 NodeRun 没有唯一可恢复的 deadline 失败、已终态、Manifest 已变化或幂等输入冲突，Command 必须 fail closed。

## 开源与依赖决策

- 继续复用 Pydantic 生成严格 Schema、FastAPI 私有 Runtime、Codex CLI 结构化输出和现有 Canonical Hash/Grant 契约。
- 首版 Skill Registry 使用普通 Python 类型和显式字典，不使用 LangGraph 重复 Temporal 编排；若 `langgraph` 无其他真实消费者，应在实现切片删除该未使用依赖。
- 不引入第二个 Prompt 框架、Provider Registry、数据库驱动、ORM 或外部 Skill checkout。
- 现有外部 Skill 仓库只保留许可证和设计追溯，不在运行时下载、导入或执行。

## 失败语义

| 失败 | 状态/处理 |
|---|---|
| Bundle/Reference 缺失或 Hash 漂移 | `failed: skill_bundle_invalid`，不可重试到不同内容 |
| 原 Bundle Hash 没有可路由 runtime revision | `unknown: skill_bundle_unavailable`，保留 Invocation 等待精确 revision 恢复 |
| Stage/Schema/Policy 不匹配 | `failed: invocation_policy_invalid` |
| Codex 无法启动或结果未知 | `unknown: runtime_unavailable`，由 Backend 对账/重试 |
| JSON 不符合严格 Schema | 有限结构修正后 `failed: candidate_schema_invalid` |
| Evidence 区间或 Hash 不合法 | `failed: evidence_invalid`，不进入人工候选 |
| 上游 Candidate Revision Hash 不再是 Head | `failed: upstream_candidate_stale` |
| Candidate 形成环或非法类型关系 | Backend `storygraph_invalid`，返回环路径/边 |
| Review Blocker 可定向修复 | 新 `repair_candidate` Invocation，只包含目标作用域与冻结边界 |
| 修复预算耗尽 | 保持 failed/needs_review，不返回伪通过半成品 |
| 单 call/shard Deadline 耗尽 | `failed: execution_deadline_exceeded`，不得自动放宽或清除其他 shard |
| Agent 试图使用 Tool | `failed: tool_not_allowed` |

### Candidate Repair 与已发布事实修改

模型 Repair 只处理尚未发布的 Candidate。`CandidateRepairPatch` 必须冻结 `target_candidate_revision_id/hash`、允许修改的 node/edge/temp keys、base fragment hashes 和邻接只读边界；Repair Invocation 的 Patch Result 保持不可变。Backend 以 expected Candidate Head Hash 做 CAS，应用 Patch 后创建带 parent/repair provenance 的下一 `StageCandidateRevision`，同一 Patch 重投只能返回相同 Receipt，不同 Patch 争用同一 Head 时整体冲突。随后把所有仍引用旧 Candidate Revision Hash 的下游 stage instance 标记 stale 后精确重跑，并对受影响闭包重新执行 deterministic gates 与 `review_storygraph`；只有当前 Shard Manifest 的所有 current Candidate Revision 无 blocker 才能进入 Human Gate，循环次数耗尽则保持 needs_review/failed。模型不能直接 Patch 已发布 StoryGraphVersion。

实施顺序固定为：先完成三类 Revision origin/hash 与原子 Head CAS；再冻结 Bible Review/Repair 输入、允许集和确定性 Gate；随后在同一 Backend 事务中加入 Patch Receipt 与旧下游 stale closure；最后接入 Temporal Node 的有界 review→repair→gate 循环。每一单元都必须独立 Red/Green/真实 PostgreSQL 验证并提交，不能把尚未完成的后续语义伪装进底层 CAS。

Bible Review/Repair 的冻结边界已经落地：Backend 本地 `bible-deterministic-gate-v1` 只根据冻结 Reconciliation Candidate 的 Key/引用结构产生排序 blocker，模型已有的 Conflict/Review Issue 不参与 Gate 计算；`review_storygraph` 输出不再包含任何 Gate/blocker 字段，只能返回属于目标 Candidate Evidence 集合的 Review Issue。`repair_candidate` 同时绑定目标 Candidate Revision 与产生目标 Issue 的 Review Candidate Revision，输入冻结目标 Issue、允许字段、规范片段 Hash、只读邻接和轮次上限；输出 Patch 只能命中该允许集，并按字段契约限定为 `text` 或 `strings` replacement。Bible 允许字段为显式集合，不含 Evidence、Identity Key、对象字段或任何 Graph 写入字段，且 Review/Repair Payload 都拒绝 `base_storygraph_version_id/hash`，因此尚未发布的 Candidate Repair 与已发布 StoryGraph 修改保持两条不相交的 DTO 路径。当前只完成输入、Gate、Harness 输出和 Patch scope 校验，尚未应用 Patch、创建 Receipt、扫描 stale closure 或驱动重审循环。

已发布图上的 Canvas 修改必须经过 Human-approved typed Domain Intent → 对应 Owner Command → 新 Owner revision → StoryGraph recompile。两条路径不共享 Patch DTO，也不允许 Graph Repository 代写 Owner。

## 实施任务映射

全局代码顺序只由 [0010 的 `SG-I01`–`SG-I28`](0010-StoryGraph内容图与DAG创作画布设计.md#唯一实施任务队列) 维护。本节只映射 Agent/Harness 有直接职责的步骤，不重新编号、不改变顺序，也不能把 Compiler 未完成时的 Candidate 宣称为 StoryGraph。

| 全局 Step | Agent/Harness 在该步的唯一职责 | 必须等待的 Backend/业务门 |
|---|---|---|
| `SG-I01` | 固定 Pydantic Candidate/Repair 模型与跨语言 fixture，不建运行目录 | StoryGraph 契约和 Owner 边界已进入 Requirement/Acceptance |
| `SG-I02` | 把 8 个 Skill 原字节迁到 `agent/skills`，切 Loader/Docker，删根路径且无 fallback | 只做行为保持迁移，不改 Guidance |
| `SG-I03`–`SG-I04` | 无新 Harness 能力；仅保持 `SG-I02` 运行契约 | 必须等 Compiler/Publish 和 Query/Diff 分别通过 CI 并提交 |
| `SG-I05` | 接入 Backend-owned Envelope/Policy/Candidate Revision，建立唯一 `build-storygraph` Bundle、Registry、Reference 映射与 Hash，原子删除 8 个过渡名 | `SG-I04` 已完成；Guidance 变更必须走 golden/contract 验收 |
| `SG-I06`–`SG-I07` | 不建第二 Human Gate 状态机；仅保持 Candidate-only 边界 | 公共 API 与 Review Workbench 由 Backend/Frontend 分两个任务闭环 |
| `SG-I08` | 实现 `extract_source_evidence` | WorkflowDefinition/Run/NodeRun 先存在；ShardManifest/Invocation 只挂在该 NodeRun |
| `SG-I09` | 实现有界 `analyze_story` map 与 `reconcile_story` tree | 只消费 current Evidence Candidate Revision，聚合谱系完整 |
| `SG-I10` | 实现 Bible `review_storygraph` 和有界 Repair | 修复后必须重跑确定性 Gate 与受影响审核 |
| `SG-I11` | 只向 Backend 交付当前 Bible Candidate Revision，无 Human Gate 或 Writer | Backend 只确认 immutable ProductionBibleVersion 并完成 Gate，不在本步物化 Asset |
| `SG-I12` | 无物化 Writer；只允许重放 `SG-I11` 已冻结 Candidate/confirmed Bible 引用 | Backend Coordinator 独立物化 Asset/Specification/State/ProductionBinding |
| `SG-I13` | 实现 `segment_episodes` Candidate | Planning Owner Apply 前不创建 Episode |
| `SG-I14` | 无 Episode Writer，只重放已冻结 Candidate Revision | Episode/ScriptVersion 由 Backend Owner 原子物化 |
| `SG-I15` | 实现 `analyze_episode` 与 `reconcile_episode` Candidate | 只消费 confirmed Bible 与已物化 Episode Slice |
| `SG-I16`–`SG-I17` | 无 Planning Writer 或 Graph Writer | Planning Owner 应用后才由 Compiler 发布 Core StoryGraph |
| `SG-I18` | 实现 `draft_storyboard`，输入 Specification/AssetState 非空，输出 Shot Intent/`needs_asset` | 缺 AssetVersion 时禁止创建正式 Shot |
| `SG-I19` | 无 Human Gate 或 Storyboard Writer；只重放冻结 Draft Candidate | Backend `FreezeIntentSet` 完成 Gate 后才允许产生 Provider 费用 |
| `SG-I20`–`SG-I21` | 仅为参考资产提供受限规则或候选，不保存 Provider 结果、图片或选择 | Provider/QC/CandidateSelection/AssetVersion 由 Backend 分两个任务完成 |
| `SG-I22` | 实现 `detail_shots`、Storyboard Review 和 Candidate Repair，只消费 READY 精确 AssetVersion | 修复后重审；不创建 Shot 或 Binding |
| `SG-I23`–`SG-I24` | 无正式 Writer；只重放已冻结 Detail Candidate | Human Gate/Owner Apply/两类 Binding 与 shot frame 生成由 Backend 分步完成 |
| `SG-I25`–`SG-I26` | 无新 Agent 能力 | Canvas 只走 StoryGraph Query 或 Backend Owner Domain Intent |
| `SG-I27` | 参与完整原稿全链、恢复和真实 Codex 验收 | 全量自动化与 CI 通过并独立提交 |
| `SG-I28` | 无新 Agent 实现 | `agent-browser` 只读验收已完成的真实 Web Journey |

每个任务必须先有失败测试，再实现，再重构；测试按性质进入独立 `agent/tests/unit`、`agent/tests/contract`、`agent/tests/integration`，Backend/Frontend 测试继续进入各自现有分类目录。任务通过真实 CI 后先回填该任务 Acceptance Evidence、检查 diff/hygiene，再立即提交；不能积累成一个大提交。

## 验收边界

本设计未来的 Acceptance 至少要证明：

- 仓库和镜像中不存在根 `.agents/skills` 或旧路径 fallback；
- `agent/skills/build-storygraph/SKILL.md` 是唯一入口，Stage 只加载声明的 Reference；
- Bundle 字节变化会导致 Hash 契约失败；
- 非终态 Invocation 在新部署后仍精确路由原 Bundle Hash；缺失时显式等待/失败而不换用新 Guidance；
- 完整原稿能按 Unicode code-point 绝对区间切片、树形 reconcile 并恢复，不重复已完成 stage instance；
- 同一 Harness 完成 Bible、分集、Scene/Beat、Occurrence、Storyboard/Shot 和 Review/Repair 候选；
- Agent 从未直接创建正式 Episode、Scene、角色卡、StoryGraphVersion 或 Shot；
- Backend 可重验 Evidence、Owner Ref、DAG 和 Hash，并在 Human Gate 后应用；
- 一个角色跨多形象仍只有一个身份，并可绑定覆盖 front/profile/back 的精确角色 AssetVersion；
- Storyboard Draft 的 Specification/State 输入非空，Detail 的精确 AssetVersion 输入非空，正式 Shot 不丢失 ShotProductionBindingVersion；
- Candidate Repair 只修改冻结允许集，创建下一 Candidate Revision 并切换 Head 后能使旧下游精确 stale；已发布修改只能走 Owner Domain Intent；
- 失败、unknown、单 shard deadline、重投、恢复和局部修复均有独立自动化证据，完整 WorkflowRun 不因固定墙钟被截断；
- 最终浏览器验收只在全部开发和真实 CI 完成后使用 `agent-browser`。

## 待用户接受的核心决策

1. 最终运行入口固定为 `agent/skills/build-storygraph`，不保留根路径回退或 `agents/openai.yaml`；实施先做 8 个 Skill 的字节等价迁移，再用独立完整任务原子收口单 Bundle，运行时从不双路径。
2. 一个核心 `SKILL.md` + 按阶段 Reference 构成 Bundle；不是 8 个可被任意发现的平行 Skill。
3. WorkflowDefinition/Temporal 只编排稳定宏观节点；Backend 在对应 NodeRun 下持久化 stage shard，Agent Harness 不引入第二个持久 Workflow Engine。
4. 一个 `stage + shard key + input hash` 实例对应一个可持久 AgentInvocation；全局分析使用有界 map/确定性 tree reduce，避免完整剧本巨型调用并支持精确恢复。
5. Agent 只返回 Candidate/CandidateRepairPatch；Backend 分配正式 ID、校验 DAG、主持 Human Gate，并通过唯一 Owner 写入 PostgreSQL/GORM 事实。
