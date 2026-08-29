# StoryGraph 内容图与 DAG 创作画布设计

- 状态：已接受（`SG-D01`）；通用媒体 Provider/Shot 视频关系已同步（2026-08-29）
- 日期：2026-08-26
- 接受日期：2026-08-27
- 上位设计：[系统总体架构](0003-系统总体架构.md) · [领域语言与模块命名规范](0006-领域语言与模块命名规范.md) · [剧本到分镜 MVP 垂直切片设计](0009-剧本到分镜MVP垂直切片设计.md)
- 相关设计：[前端应用架构](1001-前端应用架构.md) · [后端领域模块功能设计](2002-后端领域模块功能设计.md) · [通用媒体 Provider 与 Generation 执行器设计](2051-通用媒体Provider与Generation执行器设计.md) · [项目制作圣经生成执行框架设计](3001-项目制作圣经生成执行框架设计.md) · [StoryGraph 剧本解析 Harness 与内置 Skill 设计](3003-StoryGraph剧本解析Harness与内置Skill设计.md)
- 当前门禁：`SG-D01`–`SG-D16` 的 Design 已接受并完成 2026-08-29 Provider 变更同步；`SG-D17`–`SG-D21` 必须继续按 PRD → Requirement → Agent Contract 复核 → Plan → Acceptance 顺序推进，全部重新接受前不编码

## 设计结论

Lanverse 的内容生产核心从“以 Storyboard 为终点”升级为“以 StoryGraph 为内容关系主干”。Storyboard 不被简单改名或删除，它继续表示分镜候选、审核、正式 Shot、图片/视频绑定和导出生命周期；StoryGraph 负责把剧本证据、分集、场景、叙事节拍、角色、地点、剧情状态、分镜和生产绑定组织成一个可版本化、可追溯、可投影的内容 DAG。

```text
DocumentRevision → ProductionBibleVersion → Episode → Scene → NarrativeBeat → Shot
                                         Scene / Beat + AssetState → Occurrence ─┘

Specification / AssetState / EffectiveStyleSnapshot → Reference GenerationTarget → Artifact → AssetVersion
Specification / AssetState ─┐
Asset / AssetVersion ────────┴→ ProductionBinding（关系声明结果）
Shot + Occurrence + exact AssetVersion → ShotProductionBindingVersion → Frame GenerationTarget → Artifact
Shot + selected frame Artifact → ShotImageBindingVersion → Video GenerationTarget → video Artifact
Shot + selected video Artifact → ShotVideoBindingVersion
```

StoryGraph、Authoring Graph、Workflow Definition 和 Temporal History 必须保持四个不同语义：

1. StoryGraph 表达“故事和生产内容之间是什么关系”。
2. Authoring Graph 表达“用户准备怎样编排生产步骤”。
3. Workflow Definition 表达“本次运行需要按哪些依赖执行”。
4. Temporal History 表达“这次执行实际上发生了什么”。

四者可以使用相同的 DAG 基础概念和 Canvas 交互语言，但不能共用一个万能 Graph Schema、Node ID、数据表或 Writer。

StoryGraph 的所有**权威图边**统一表示上游事实到下游结果的依赖，必须无环。人物互相关系、敌对、亲属、情感变化等天然可能形成语义环，因此不直接持久化为 `character_a → character_b` 的拓扑边，而建成带 subject/object 的 `RelationshipClaim` 节点；角色与来源证据只作为该 Claim 的输入依赖。这样既保留完整语义，也不破坏 DAG。

后端仍是全部正式业务事实的唯一 Writer；PostgreSQL/GORM Model Catalog 仍是唯一 SQL 事实源。StoryGraph 不引入 Neo4j、第二数据库、第二 ORM、Migration 文件、Raw SQL 图查询、浏览器写库或 Agent 写库。

## 问题与设计动机

当前“原稿 → Bible → 分集 → 结构 → Storyboard → Shot”已经证明了纵向流程，但业务关系分散在多个聚合和 JSON 字段中：

- Scene、Dialogue、NarrativeUnit 存放在 Episode Structure 快照内；
- Bible Candidate 已区分稳定实体与阶段状态，但确认后尚未形成真实角色卡和资产状态；
- Storyboard 只持有 NarrativeUnit ID 数组，正式 Shot 尚未保存角色形象、地点状态和视觉版本绑定；
- Storyboard Agent 输入中的资产集合仍为空；
- Authoring Graph 已有端口 DAG、规范化和发布能力，但它描述执行编排，不描述剧情关系；
- Canvas 目前只有目标设计，没有真实前端编辑器、公开 Authoring API 或协作运行时。

如果继续只扩展 Storyboard JSON，角色一致性、跨集伏笔、地点复用、状态连续性、局部重跑影响范围和未来 Canvas 都会依赖隐式约定。如果反过来把所有内容塞入现有 Authoring Graph，又会把故事事实、执行节点和运行历史混为一体。

本设计选择新增明确的 StoryGraph 内容关系边界，同时复用现有 Owner、不可变版本、Canonical Hash、Graph Validation、Human Gate 和 Temporal 能力。

## 目标

- 用一份版本化 StoryGraph 连接完整原稿、分集、场景、节拍、实体 occurrence、分镜和生产资产。
- 每个图节点和关系都能反查唯一 Backend Owner、版本、Hash 和来源证据。
- 让同一套 StoryGraph 同时支持 Guided 业务步骤、Story Canvas、影响分析、局部重跑和生产追踪。
- 将“角色是谁”“角色当前形象”“采用什么画风”“具体生成了哪张图”拆成四个稳定维度。
- 为角色卡、多个形象状态、角色视觉版本和 front/profile/back 三视图建立可落地的最小模型。
- 让 Agent 只返回带证据的业务 Candidate/CandidateRepairPatch，由 Backend 重验并路由真实 Owner；StoryGraph Compiler 只编译已确认 Owner 事实。
- 保留 Storyboard 作为 StoryGraph 的分镜表投影与 Shot 生产能力，而不是建立双写兼容层。

## 非目标

- 不把 StoryGraph 设计成通用知识图谱、图数据库平台或任意 Schema 的 EAV 系统。
- 不在首个切片实现多人实时协作、无限画布全部镜头同时渲染、复杂图查询语言或图市场。
- 不让 StoryGraph 取代 Temporal、Workflow Compiler、Review、Generation、Asset 或 ObjectStore。
- 不在角色状态首版实现服装 × 伤势 × 年龄 × 发型的组合规则引擎。
- 不把三视图生成塞进文本 StoryGraph Harness；图片生成仍由 Generation/Asset Workflow 负责。
- 不自动接受 AI 分集、角色合并、关系判断、分镜或视觉资产。

## 四种图的正式边界

| 图或历史 | 回答的问题 | 唯一 Owner | 权威存储 | 是否可直接执行 |
|---|---|---|---|---|
| `StoryGraphVersion` | 剧本、实体、场景、节拍、Shot 和资产如何关联 | `production/storygraph` | PostgreSQL/GORM 不可变快照 | 否 |
| `AuthoringRevision.Graph` | 用户选择了哪些生产节点、端口和绑定 | `authoring` | PostgreSQL/GORM 不可变 Revision | 否，需编译 |
| `WorkflowDefinitionVersion` | 一次运行的合法执行 DAG 是什么 | `workflow` | PostgreSQL/GORM 不可变 Definition | 是，由 Temporal 解释 |
| Temporal History | Run/Activity/Timer/Signal 实际如何推进 | Temporal | Temporal History | 已执行事实 |

ID 也必须分离：

- `story_node_key`：跨 StoryGraph 版本稳定的逻辑节点键；
- `owner_ref`：同时包含稳定逻辑身份与精确业务版本的正式引用；
- `authoring_node_id`：Authoring Draft 中的编排节点实例；
- `workflow_node_id` / `node_run_id`：Definition 节点和某次运行实例。

任何 API、日志或 Canvas 选择状态都不得用其中一种 ID 冒充另一种。`owner_ref` 明确拆成稳定的 `owner_logical_id + fragment_key` 与精确的 `owner_version_id + revision + content_hash`；二者不能都叫 `owner_id`。`story_node_key` 必须由 `node_type + owner_kind + owner_logical_id + fragment_key` 确定性派生，不包含 version/revision/hash；只要逻辑 Owner 与片段身份不变，跨版本 Key 就不变。`edge_key` 同理由 edge type、稳定端点 Key 和类型化 qualifier 派生。精确 Owner 版本变化只产生内容 Diff，不得伪装成节点全量删增。

## StoryGraph DAG 语义

### 边方向

所有持久化 StoryGraph Edge 都采用同一个方向：

> 上游证据、约束或资源 → 下游解释、出现、实现或产物。

允许的首版关系如下：

| Edge Type | 示例 | 语义 |
|---|---|---|
| `contains` | Episode → Scene → NarrativeBeat | 结构包含，父级先于子级存在 |
| `derived_from` | SourceEvidence → CharacterSpecification | 下游事实来源于上游证据 |
| `describes_identity` | `asset_identity` → Character/Location/Prop Specification | 规范描述属于唯一资产身份 |
| `has_state` | `asset_identity` → AssetState | 剧情形象状态属于唯一资产身份 |
| `precedes` | Scene A → Scene B、Shot 1 → Shot 2 | 同一序列中的明确先后 |
| `anchors_occurrence` | Scene/NarrativeBeat → Occurrence | 一次出现属于哪个场次或节拍 |
| `instantiates_occurrence` | AssetState(kind=character/location/prop) → Occurrence | 哪个精确剧情状态发生一次出现 |
| `realizes` | NarrativeBeat → Shot | Shot 实现一个来源节拍 |
| `informs` | Occurrence → Shot | Shot 消费一次实际出现及其状态 |
| `constrains` | WorldRule/EffectiveStyleSnapshot → Shot/GenerationTarget | 上游规则约束下游生产 |
| `materializes` | Specification/AssetState/Asset/AssetVersion → ProductionBinding；Specification/AssetState/EffectiveStyleSnapshot → ReferenceGenerationTarget → Artifact → AssetVersion | Binding 是由各参与事实共同指向的关系声明结果；参考生成保持从输入到产物的依赖方向 |
| `binds_input` | Shot/Occurrence/AssetVersion → ShotProductionBindingVersion | Shot 使用哪些精确生产参考版本 |
| `feeds_generation` | ShotProductionBindingVersion → ShotFrameGenerationTarget → Artifact；ShotImageBindingVersion → ShotVideoGenerationTarget → Artifact(video) | 冻结生产输入进入 Shot 画面生成，选中首帧再进入视频生成 |
| `binds_output` | Shot/Artifact(image) → ShotImageBindingVersion；Shot/Artifact(video) → ShotVideoBindingVersion | 正式 Shot 分别选择哪一个图片或视频产物 |
| `supports` | Evidence → Claim | 来源证据支持一条语义主张 |
| `claim_participant` | `asset_identity`/WorldRule → Claim | 主张的 subject/object 等参与者 |
| `claim_anchor` | Episode/Scene/Beat/Occurrence → Claim | 主张成立、变化、伏笔或回收的剧情锚点 |
| `supersedes` | Older Claim → Newer Claim | 新主张在明确有效范围内取代旧主张 |

首版不开放用户自定义 Edge Type。`GenerationTarget` Payload 必须是 `reference_asset|shot_frame|shot_video` 严格联合类型：前者才能发布 AssetVersion；`shot_frame` 只能产生供 ShotImageBindingVersion 选择的图片 Artifact；`shot_video` 必须冻结正式 Shot、精确 ShotProductionBindingVersion、ShotImageBindingVersion、目标时长与视频 Profile，只能产生供 ShotVideoBindingVersion 选择的视频 Artifact。新增类型必须提升 StoryGraph Schema Version，并补齐允许的 source/target 类型、Hash、无环和反向追踪测试。

`materializes` 指向 `production_binding` 时必须带严格 `binding_role=specification|state|asset|asset_version` qualifier，且 source type 必须与 role 匹配；四类参与事实全部指向 Binding Node，Binding 不再反向指向 Asset/AssetVersion。这样 `asset_identity → asset_state` 与物化关系不会形成 `Asset → State → Binding → Asset` 环，Compiler 的类型矩阵也必须拒绝任何 `production_binding → asset_identity|asset_version` 权威边。

### 天然成环的语义关系

人物关系和叙事语义可能天然成环，例如 A 保护 B、B 欺骗 A，或两个角色互为敌手。若直接把它们都做成拓扑 Edge，StoryGraph 就不再是 DAG。

本设计统一使用 Claim Node：

```text
asset_identity(character=A) ── claim_participant ─┐
asset_identity(character=B) ── claim_participant ─┼→ RelationshipClaim
Scene/Beat ───── claim_anchor ─────┤   { subject=A, predicate=protects, object=B,
Evidence ─────── supports ─────────┘     valid_scope, polarity, status }
```

`subject`、`object` 和所有 semantic anchor 保存在严格 Claim Payload 中时，必须同时引用可校验的 `story_node_key`，并与 `claim_participant` / `claim_anchor` 输入边完全一致；不能把影响闭包所需的 Scene、Beat 或 Occurrence 只藏在 Payload。Claim 至少带 `predicate`、`valid_scope`（project/episode/scene/beat 或来源时间区间）、`polarity`、`status=asserted|uncertain|negated` 和可选 `supersedes_claim_ref`。`supersedes` 固定为旧 Claim → 新 Claim，因此“前 3 集信任、第 4 集起不信任”是两个有范围且有演进方向的事实，不是两条永久冲突关系。

Claim 自身也必须有唯一业务 Owner：稳定世界规则、人物关系、跨集 Story Arc/Plot Thread、伏笔与回收归 Production Bible；Episode/Scene 内的因果与连续性归 Planning 的 Episode Structure；Shot 级生产连续性归 Storyboard。StoryGraph Compiler 只读取并编译这些 Owner 事实，绝不创建或修订 Claim。Canvas 可以把 Claim 投影成便于阅读的 A→B 视觉连线，但该语义投影不进入权威拓扑或 Hash。

Production Bible 在 Episode Planning 之前确认，因此 Bible Candidate/Version 的 `anchor_keys` 只能作为候选阶段的冻结引用，不能伪装成尚未存在的正式 Scene/Beat/Occurrence `story_node_key`。Core StoryGraph Compiler 同时拿到 exact Bible Version 与完整 confirmed Planning Owner Set 后，必须用 Claim 的每条精确 Evidence 与已确认 Scene Evidence 的 Unicode code-point 区间重叠确定正式 `claim_anchor`；全部 Evidence 都必须命中，结果按稳定节点键排序。无匹配、越界、跨来源或无法确定唯一语义输入时编译失败，不能沿用候选 key、猜测场景或创建新的 Owner 事实。

### 规范 Node Type 与唯一 Owner

StoryGraph Node 是 Owner 事实的投影，不为产品别名新建 Record。首版每个规范 Node Type 固定唯一 `owner_kind`：

| Node Type | 唯一 `owner_kind` | 投影的正式事实 |
|---|---|---|
| `source_revision` | `production/script` | DocumentRevision/ScriptVersion |
| `source_evidence` | `production/bible` | 已校验 Evidence Fragment |
| `policy_snapshot`、`effective_style_snapshot` | `preset` | EffectivePolicy/Style Snapshot |
| `asset_identity` | `asset` | `Asset(kind=character|location|prop)`；不存在 AssetIdentity Record |
| `character_specification`、`location_specification`、`prop_specification` | `production/bible` | 对应不可变 SpecificationVersion |
| `asset_state` | `asset` | `AssetState`；Appearance Variant 只是产品别名 |
| `production_binding` | `production/bible` | 现有 `ProductionBinding`；投影精确 Binding ID/revision/hash，不用无 Owner 的普通 Edge 替代 |
| `world_rule`、`story_arc`、`plot_thread`、`relationship_claim`、`foreshadowing_claim`、`payoff_claim` | `production/bible` | Bible 全局叙事事实 |
| `episode` | `production/project` | 已物化 Episode |
| `scene`、`dialogue`、`narrative_beat`、`occurrence`、`continuity_claim`、`causal_claim` | `production/planning` | 已确认 EpisodeStructure 片段 |
| `shot`、`shot_continuity_claim` | `production/storyboard` | Owner Apply 后的正式 Shot/Claim；Draft Row/Shot Intent 不进入正式图 |
| `generation_target` | `generation` | 冻结 Generation Intent/Job Target |
| `artifact`、`asset_version` | `asset` | READY Artifact 与发布 AssetVersion |
| `shot_production_binding_version`、`shot_image_binding_version`、`shot_video_binding_version` | `production/storyboard` | Shot 输入参考集合、生成画面结果与最终视频结果绑定 |

Canvas rank 由合法边经过稳定拓扑排序后计算。Claim 可能只依赖人物，也可能依赖后段 Scene/Beat/Occurrence，因此不能固定在第 2 层。业务 sequence/position/timecode 属于 Owner 内容并进入 Content Hash；Canvas 坐标、折叠和 viewport 只是视图状态，不进入 Hash。合法边仍由类型矩阵与上表 owner_kind 共同放行，不能仅凭展示分组放行。

### 规范化与门禁

Backend 发布 StoryGraphVersion 前必须完成：

1. Node Key、Edge Key、类型和 Owner Ref 唯一且合法，稳定 Key 与逻辑 Owner/端点可重新推导；
2. Owner Ref 的 workspace/project、版本、revision/hash 与当前命令冻结输入一致；
3. Evidence 的 document revision、`normalized_text` Unicode code-point half-open 绝对区间、原文和 hash 可跨 Go/Python 重建；
4. 所有 Edge 符合类型矩阵，不存在 dangling reference、自环或重复关系；
5. 使用稳定 Kahn 拓扑排序拒绝任意环，并以 Node Key 作为并列顺序裁决；
6. `precedes` 与业务 position/timecode 一致；
7. 每个正式 Scene、NarrativeBeat、Shot 和 Claim 都可追溯到至少一个来源或已确认上游；Claim Payload 引用与 participant/anchor 边、有效范围和演进状态一致；
8. Graph 内容按稳定顺序 Canonical JSON 编码；layout、viewport、折叠和颜色不进入 Content Hash；
9. 相同冻结输入和正式 Owner 事实生成相同 `topology_hash` 与 `content_hash`。

现有 Authoring Graph 的端口校验、Canonical Sort、Hash 和 Kahn 算法可以作为实现参考；StoryGraph 不直接复用其端口执行 DTO，因为两者语义不同。

## 数据与事实源

### 最小持久模型

首版只新增一个关系快照聚合：不可变 Version 加一个按项目线性发布的 Head 指针，不预建每种节点和边的通用关系表：

```text
StoryGraphVersion
├── id / workspace_id / project_id / version_no
├── parent_version_id / parent_content_hash
├── source_revision_id / source_revision_hash
├── owner_head_refs JSONB / owner_set_hash
├── schema_version
├── nodes JSONB
├── edges JSONB
├── topology_hash / content_hash
├── status=published
└── created_by / created_at

StoryGraphHead
├── workspace_id / project_id
├── current_version_id / current_content_hash
└── revision
```

`owner_head_refs` 是本次编译实际读取的完整、排序、去重 Owner 输入清单，每项冻结 owner kind、logical id、version id、revision、hash；`owner_set_hash` 对整份清单做 Canonical Hash。预检可以在事务外失败返回，但产生可发布内容的权威读取、完整 Owner Head/phantom 范围扫描、编译、校验、Version 插入和 Head 切换必须位于同一个 GORM `SERIALIZABLE` PostgreSQL 事务；事务先锁定项目的 `StoryGraphHead`，再在同一一致快照读取全部图可见 Owner 事实。这样并发 Owner 提交要么序列化在本次编译之前并被读取，要么序列化在其后并让新 Graph 立即显式 stale，不能形成混合快照。

`version_no = locked_head.revision + 1`，数据库还必须具有 `(workspace_id, project_id, version_no)` 唯一约束；事务以 `expected_current_content_hash + expected_head_revision` CAS 切换 Head。序列化失败、唯一冲突或 CAS 失败整体回滚，并以同一幂等命令从新 Head 重编译；不得保留孤立 Version 或产生父链分叉。上述全部通过 GORM Model/Transaction/Locking 实现，不引入 Raw SQL 或 Migration 文件。

`GetCurrentStoryGraphVersion` 必须返回 `compiled_from=owner_head_refs` 和实时 `stale` 标记。若任一当前 Owner Head 与冻结清单不一致，旧 Graph 仍可读但明确标记过期；它不能被业务 Command 当作当前 Owner 事实，Command 仍需重验精确 Owner revision/hash。

每个 Node 至少包含：

```text
story_node_key
node_type
owner_ref {
  owner_kind,
  owner_logical_id, fragment_key?,
  owner_version_id, owner_revision, owner_hash
}
label / business_position（只保存投影必需且属于业务事实的字段）
evidence_refs[]
payload（由 node_type 决定的严格联合类型）
content_hash
```

每个 Edge 至少包含：

```text
edge_key
edge_type
from_node_key
to_node_key
payload（只允许该 edge_type 的附加字段）
content_hash
```

`fragment_key` 用于引用 Owner 聚合内部的稳定片段，例如 `EpisodeStructure + scene_id`，不要求为了图查询把 Scene、Dialogue 或 NarrativeUnit 立即拆成独立 SQL 表。Node Key 派生只使用 `owner_kind + owner_logical_id + fragment_key`；`owner_version_id` / `owner_revision` / `owner_hash` 仍进入 Node Content Hash 和 Owner Set Hash。

### 唯一事实规则

- Episode、Scene、NarrativeUnit、Shot、Review、Generation、Asset 等正式内容仍由各自 Backend Owner 写入。
- `StoryGraphVersion` 是这些 Owner 之间**正式关系的不可变编译事实**，不复制可原地编辑的完整业务对象，也绝不成为 Entity、State、Claim、Shot 或 Asset 的应用 Owner。
- Agent 输出只保存为 Invocation/NodeRun Candidate；Candidate 不等于 StoryGraphVersion。
- Canvas 修改必须提交带 base version/hash 的类型化 Domain Intent；Backend 将每个 Intent 路由到唯一真实 Owner Command，成功后重新编译 StoryGraphVersion。模型产生的未审核 Candidate Repair 与已发布事实修改是两条流程，不能共用一个通用 JSON Patch。
- 不允许 Canvas 直接覆盖 `nodes JSONB`，也不允许 StoryGraph Repository 跨 Owner 更新 Shot、Asset 或 Episode 表。
- PostgreSQL 仍只有一个 `DATABASE_URL`；Version/Head Record 进入唯一 GORM Model Catalog 并由 GORM 同步。
- 不建立 SQL Migration/Checksum/Version 表，不写 Raw SQL 图遍历，不引入第二 ORM 或图数据库。

如果后续某个 Lens 的查询性能成为真实瓶颈，可以新增可重建索引投影；投影不能变成 Command Writer，也不能先于性能证据进入 MVP。

## Storyboard 在新模型中的位置

Storyboard 是 StoryGraph 中从 NarrativeBeat 到 Shot 的一张专业投影，而不是整个故事模型：

```text
StoryGraphVersion
  └─ Storyboard Lens
      ├─ Episode / Scene
      ├─ Formal Shot row
      ├─ Shot number / duration / camera language
      ├─ source coverage
      ├─ Character/Location/Prop Binding
      └─ image/video generation status
```

现有 Storyboard Owner 继续负责：

- Draft Set/Batch；
- 逐镜决议和批次批准；
- 影响预检和原子 Apply；
- 正式 Shot；
- Shot 图片绑定；
- 确定性分镜包导出。

新增 StoryGraph 不建立第二套 Shot Writer。正式 Apply 后由 StoryGraph Compiler 读取新的 Shot/Binding 事实并发布下一 StoryGraphVersion。产品用语可以把“分镜表”作为 StoryGraph 的 Shot Lens，但代码和数据不做旧名兼容双写。

Storyboard Draft 到参考资产生成之间必须存在一个独立 Human Gate，不能因为 Draft “可审核”就直接产生费用。正向决议调用 Storyboard Owner 的 `FreezeIntentSet`：它复用现有 Draft Set/Batch 和统一 Command Receipt，只冻结精确 Set revision/candidate hash、已接受 Shot Intent、视觉需求 hash 与 ReviewDecision ID，不新建第二张状态表、不创建正式 Shot，也不进入 StoryGraphVersion。只有 Owner Receipt 已提交且 Gate 输出了同一份 `approved_storyboard_intents` 引用后，`reference_asset` 才能消费该冻结输入；拒绝、修改、Receipt unknown 或视觉需求漂移都不能启动 Provider。

## 角色卡、形象变体和三视图

### 四个必须分离的维度

| 维度 | 规范事实 | 示例 |
|---|---|---|
| 稳定身份 | `Asset(kind=character)` + `CharacterSpecificationVersion` | 同一个 Aurelia、姓名/别名、稳定锚点、性格、目标和证据 |
| 剧情形象 | `AssetState`，产品显示为 `CharacterAppearanceVariant` | 基础形象、第三集受伤制服、二十集女帝形象 |
| 渲染画风 | `EffectiveStyleSnapshot` | 真人写实、国漫、水墨；不属于人物剧情状态 |
| 具体视觉结果 | `AssetVersion` + lineage artifact refs/view-role metadata | 某状态在某画风下经审核发布的一组参考图 |

### 规范术语映射

| 产品/设计用语 | 唯一正式事实 | StoryGraph 表达 | 禁止新增 |
|---|---|---|---|
| 角色/地点/道具身份 | Asset Owner 的 `Asset(kind=...)` | `asset_identity` Node | `AssetIdentity` Record |
| 角色/地点/道具规格 | Production Bible 的 `*SpecificationVersion` | 对应 `*_specification` Node | 无版本的 Specification 副本 |
| 形象/状态变体 | Asset Owner 的 `AssetState` | `asset_state` Node | `CharacterAppearanceVariant` Record |
| Bible 物化绑定 | Production Bible Owner 的 `ProductionBinding` | `production_binding` Node 及按角色指向它的 `materializes` Edge | 无 Owner Ref 的通用 Binding/Edge Record |
| 画风 | Preset Owner 冻结的 `EffectiveStyleSnapshot` | `effective_style_snapshot` Node | 可变 Style 文本或通用 `StyleSnapshot` 副本 |
| 参考图版本 | Asset Owner 的 `AssetVersion`，其 lineage 保存 Artifact 精确引用和 view-role metadata | `asset_version` + `artifact` Node/Edge | 通用 `ArtifactBinding` Record |
| Character Look | 上述角色 AssetVersion 的只读 typed lens | Canvas/角色卡 View | `CharacterLookVersion` Record/Node |
| 地点卡 | Asset + LocationSpecificationVersion + AssetState + AssetVersion 的只读 View | Location Lens | `LocationCard` Record |

角色卡不是另一个与 Asset 并列的身份表。产品中的“角色卡”由以下权威事实组成：

```text
Character Card View
= Asset(character identity)
+ Current CharacterSpecificationVersion
+ AssetState[]
+ Published AssetVersion[]
+ Claim View[]
+ Evidence / Rights / Lineage
```

`CharacterSpecificationVersion` 至少包含规范名、别名、稳定外貌锚点、年龄印象、人物功能、性格、目标、声音特征、禁改锚点、来源证据和 Bible Version。人物关系不复制进 Specification；角色卡按范围查询 Production Bible/Planning 拥有的 Claim View。不同服装、伤势、伪装、年龄阶段、发型或身份呈现不能创建第二张角色卡。

### Appearance Variant 与 Character Look

首版 `AssetState` 直接表达一个有来源、可生产的完整剧情形象，不实现状态组合引擎：

```text
base
ep03_injured_uniform
ep20_empress_form
```

同一状态在不同画风下可产生不同视觉版本：

```text
Character Look（只读 typed lens）
= Published AssetVersion(kind=character_reference)
  whose inputs are CharacterSpecificationVersion + AssetState + EffectiveStyleSnapshot
  and whose lineage records Generation/Review provenance
```

Character Look 只是对唯一 `AssetVersion` 的只读 typed lens/ref：不创建 `CharacterLookVersion` Record，不发布第二个版本号，也不在同一 StoryGraphVersion 中用同一个 owner_ref 再投影一个 `character_look` 节点。Shot 必须绑定精确 AssetVersion，不读取“当前最新”或依赖自由文本角色描述。

### 三视图

MVP 复用现有“单次 CandidateSelection 只选择一个 Candidate”的基数：一个可发布的角色参考 AssetVersion 必须选择一个 READY 的 composite `reference_sheet` Artifact，并由其 Binding 元数据完整覆盖三个语义视图角色：

- `front`
- `profile`
- `back`

可选扩展为 `three_quarter`、`portrait` 和 `expression_sheet`。Provider 返回的拼版必须在 Binding 元数据中声明每个 view role 的区域或独立派生 Artifact；不能只保存一个无语义 URL。只有未来真实 Provider 证明必须跨 1..N Artifact 组合时，才新增 `LookCandidateGroup` 并让一次 CandidateSelection 选择整个组；首版不让一次选择隐式挑中三张无聚合关系的图。

发布角色 Look 的硬门：

1. CharacterSpecificationVersion、AssetState 和 EffectiveStyleSnapshot 均已冻结；
2. 被选 composite Candidate 的 view-role coverage 恰好包含 front/profile/back，且共享同一 Look 输入 Hash；
3. reference_sheet Artifact 为 READY 且 Rights/Lineage 完整；
4. 身份锚点、服装、比例、色彩和画风一致性 QC 有明确结果；
5. 用户完成唯一 CandidateSelection；
6. 发布后不原地替换 Artifact，修改产生下一 AssetVersion。

### 地点卡、地点状态与场景视图

Scene 是一次具体剧情场次，Location 是可跨场次复用的地点身份。地点卡和角色卡一样只是只读派生视图，不建立 `LocationCard` Record：

```text
Location Card View
= Asset(kind=location)
+ Current LocationSpecificationVersion
+ AssetState[]             # 昼夜、天气、损坏、布置等
+ Published AssetVersion[] # 精确画风下已发布参考图
+ Evidence / Rights / Lineage
```

Location Specification 至少表达空间布局、入口出口、关键区域、尺度、固定视觉锚点、材质和光照约束。Scene/Beat 与精确 Location State 共同产生 Location Occurrence；Shot 再绑定用于生产的 Location AssetVersion。产品若显示“场景卡”，它只能是 `Scene + LocationState + Occurrence` 的派生视图，不拥有独立事实。不能把 `Scene` 和 `Location` 合并，也不能让每一集重复创建同一地点。

### Occurrence 与 Production Binding

`Occurrence` 表达“某身份/状态在某个 Scene 或 NarrativeBeat 中实际出现”。现有上位设计中的 `ProductionBinding` 保留为 Production Bible 发布时“Specification/State 与物化 Asset/AssetVersion”的正式关系；本设计新增且只用于 Shot 输入的是 Storyboard Owner 的 `ShotProductionBindingVersion`，表达“某个精确 Shot revision 使用哪一组已发布视觉版本”。两者不能复用名称、Record 或 Writer。

```text
Specification / AssetState ─┐
Asset / AssetVersion ────────┴→ ProductionBinding

Scene / NarrativeBeat ────────────┐
AssetState(character/location/prop) ┼→ Occurrence → Shot
NarrativeBeat ────────────────────┘             │
                                                ├─┐
Occurrence + exact AssetVersion ────────────────┘ ├→ ShotProductionBindingVersion → GenerationTarget
Shot + selected frame Artifact ──────────────────→ ShotImageBindingVersion
ShotImageBindingVersion + ShotProductionBindingVersion → GenerationTarget(kind=shot_video) → Artifact(video)
Shot + selected video Artifact ──────────────────→ ShotVideoBindingVersion
```

Occurrence 必须带来源证据和剧情状态。每个 `ShotProductionBindingVersion` 是不可变完整集合，冻结 `shot_id + shot_revision + version_no + parent_version_id + entries[] + content_hash`；`entries` 按 `(occurrence_ref, asset_role)` 排序且不可重复，每项包含精确 AssetVersion Ref、所需 view roles 和 lineage hash。一个 Shot revision 同时只有一个 current Binding Version；修改必须以 expected current version/hash 发布下一完整集合，不能逐资产覆盖或由多个 Writer 追加。

发布门禁必须证明：每个已审核 Shot Intent 要求的视觉 Occurrence 恰好有对应 Entry；Entry 的 AssetVersion 为 READY，其 lineage 指回与 Occurrence 相同的 Asset（StoryGraph 中投影为 `asset_identity`）和 AssetState、兼容的 EffectiveStyleSnapshot，并覆盖该 Shot 要求的 view roles；未知、重复、跨身份、跨剧情状态、错误画风或“最新版本”引用全部拒绝。

四个 Binding 名称不能混用：

| 名称 | 语义 | 唯一 Owner |
|---|---|---|
| `ProductionBinding` | Bible Specification/State 与物化 Asset/AssetVersion 的既有发布关系 | Production Bible |
| `ShotProductionBindingVersion` | Shot 的完整不可变生产输入集合：精确 Character/Location/Prop AssetVersion | Storyboard |
| `ShotImageBindingVersion` | 现有输出结果绑定：Shot 选择的生成 frame Artifact | Storyboard |
| `ShotVideoBindingVersion` | 视频输出结果绑定：Shot 选择通过 Video QC 的视频 Artifact，并冻结 Target/Selection/首帧/时长/媒体元数据 | Storyboard |

StoryGraph 只投影四种 Owner Binding 事实，不复制 Writer；`ProductionBinding` 必须以自己的 `production_binding` Node 携带 Owner Ref，Specification、AssetState、Asset 和 AssetVersion 分别通过带 `binding_role` 的入向 `materializes` Edge 指向该节点，因而可精确反查 Binding ID/revision/hash且不引入权威环。Shot 输入 Binding 既不能借用 Bible materialization Binding，也不能借用图片/视频结果 Binding 来保存；媒体 Provider Connection/Credential/Profile/Job/Call 仍不进入 StoryGraph 权威节点。

## 剧本深度解析与 StoryGraph 内容范围

StoryGraph 不只保存层级拆分，还允许保存有证据的深层叙事节点：

- Episode boundary 与标题；
- Scene、Dialogue、Action 和 NarrativeBeat；
- Story Arc、Plot Thread、Conflict、Goal 和 Turning Point；
- Character/Location/Prop Occurrence；
- RelationshipClaim、ContinuityClaim、Foreshadowing/Payoff Claim；
- Owner Apply 后的正式 Shot 及其 purpose、镜头语言、声画意图和时长；Storyboard Row/Shot Intent 候选不进入正式图；
- Character Look typed lens、Location AssetVersion 投影、ShotProductionBindingVersion、ShotImageBindingVersion、ShotVideoBindingVersion 和 Artifact lineage。

Agent 产生的深层分析起初都是候选，必须保留 EvidenceRef、置信/歧义和 Review Issue；只有经 Human Gate 和真实 Owner Apply 物化的事实才能被 Compiler 编入正式 StoryGraphVersion。模型不得用“常见套路”补写剧本中不存在的关系、动机、伏笔或形象。

## Guided 与 Canvas

### 两种 Canvas Lens，不是两套事实

Canvas Studio 增加两个明确 Lens：

| Lens | 数据 | 用户任务 |
|---|---|---|
| Story Lens | StoryGraphVersion 和待审 Graph Patch | 浏览/审核剧集、场景、关系、角色状态、分镜和资产影响 |
| Workflow Lens | AuthoringDraft/Revision 和 Workflow Projection | 编排节点、端口、Human Gate、运行、Dirty 和局部重跑 |

两者复用缩放、选择、MiniMap、Inspector、快捷键、Diff 和运行覆盖层等前端基础能力，但请求不同 Backend Contract，也不共享 Node ID。Guided 页面继续以业务表单和分步流程调用相同 Owner Command；Canvas 不是第三个 Writer。

### Story Lens 分层展示

首版不把整部剧所有节点一次画在无限画布上。用户按 Lens 和范围加载子图：

- Outline：Project → Episode → Scene；
- Narrative：Scene → Beat/Dialogue → Shot；
- Entity：Character/Location/Prop → State → Occurrence/Claim；
- Production：Shot → Binding → AssetVersion/Artifact；
- Impact：选中节点的上游证据和下游影响闭包。

默认只展开一集或一个 Scene；折叠、坐标、颜色、Viewport 和临时筛选不进入 StoryGraph Hash。

### 开源复用

- Canvas 节点、边、Handle、选择、缩放和 MiniMap 采用 [`@xyflow/react`](https://github.com/xyflow/xyflow)，不自研图形交互引擎。
- 首版分层自动布局采用 [`@dagrejs/dagre`](https://github.com/dagrejs/dagre)，满足有向图的简单布局；不在没有复杂端口/嵌套布局证据时引入 ELK 或自研布局算法。
- Backend 的类型矩阵、Owner Ref、Evidence、Canonical Hash 和无环门禁属于 Lanverse 业务不变量，不能交给前端布局库裁决。
- 依赖版本只在 Design 接受后的 Requirement/Plan 中固定，并以 lockfile、许可证清单和真实构建验证为准。

## 应用命令与查询边界

本设计只固定意图，不提前冻结 HTTP 路径。接受后 Requirement 至少需要定义：

### Query

- `GetStoryGraphVersion`
- `GetCurrentStoryGraphVersion`
- `GetStoryGraphLens`
- `TraceStoryNodeUpstream`
- `TraceStoryNodeDownstream`
- `DiffStoryGraphVersions`
- `ValidateStoryGraphCandidate`

### Command

- `CompileStoryGraphVersion`
- `ProposeStoryGraphPatch`
- `ApplyStoryGraphPatch`
- `PublishCharacterSpecificationVersion`
- `PublishLocationSpecificationVersion`
- `PublishAssetVersion`
- `PublishShotProductionBindingVersion`

`Propose/ApplyStoryGraphPatch` 是产品层意图名称，不是通用 JSON overwrite 或 StoryGraph 自写接口。它必须携带 base StoryGraph Version/Hash、目标 Owner、类型化 Domain Intent、expected owner revision 和 idempotency key；Backend 通过真实 Owner Application Service 提交业务事实，再以 expected current graph hash 编译并线性发布新 StoryGraphVersion。

## 状态与失败路径

StoryGraphVersion 只有不可变 `published` 状态；旧版本不原地改写。Agent Candidate、HumanTask 和 Owner 聚合继续使用各自状态机，不为 StoryGraph 再复制一套 `queued/running/needs_review`。

必须显式处理：

- Owner Ref 或 Evidence 漂移：拒绝发布，返回过期节点和当前版本；
- Owner Set 或 expected current graph hash 漂移：拒绝 Head 切换，返回 stale 并从新 Head 重编译；
- Candidate 引用未知角色/地点/Scene/Shot：进入 Review Issue，不自动创建近似身份；
- DAG 成环：返回最小可解释环路径，任何入口都不能保存为正式版本；
- Claim 缺少 participant/anchor/evidence、有效范围或 Payload/Edge 不一致：拒绝 Candidate；
- Appearance Variant 把画风当剧情状态：拒绝物化并要求拆分；
- composite reference sheet 缺少 front/profile/back coverage 或 QC 不一致：角色 AssetVersion 保持不可发布；
- Shot Binding 集合不完整、lineage 与 Occurrence 身份/状态/画风/view role 不一致，或读取“最新版本”而非冻结版本：拒绝执行；
- Agent、Canvas 或 Yjs 尝试直接写业务表：权限和架构测试双重拒绝；
- 编译结果未知：以相同幂等键查询 Receipt/Hash，不创建第二版本；
- Canvas 断线或 base hash 冲突：重读当前版本并展示 Patch Diff，不静默覆盖。

## 唯一实施任务队列

正式 Plan 只能在 `SG-D01`–`SG-D19` 全部通过后于 `SG-D20` 派生，代码又必须等 `SG-D21` 的全未勾选 Acceptance Criteria 建立并接受后才能开始。下表先固定不可互换的硬依赖，并是后续 Plan 必须原样引用的唯一 `SG-Ixx` 顺序；任何时刻只实施一项。

| Step | 完整任务 | 硬退出门 |
|---|---|---|
| `SG-I01` | 固定 StoryGraph Schema、稳定 Key、Node/Edge/Claim Owner、Evidence Ref、Canonical Hash、四图边界和跨语言 contract fixture；在首个真实消费者中固定 GORM/PostgreSQL、Temporal、Pydantic/Codex CLI 与 React Flow/Dagre 选型 | Requirement/Acceptance 映射完整，失败 contract 测试先落位，无空工具层、Migration、Raw SQL 或第二 ORM |
| `SG-I02` | 把现有 8 个 Skill 按原名、原字节迁入 `agent/skills`，原子切换 Loader/Docker/独立测试，同一提交删除根目录旧路径 | Guidance 字节等价、单路径、无 fallback，Agent 和全量 CI 通过 |
| `SG-I03` | 实现 StoryGraphVersion/Head、Owner Set 冻结、GORM Record、线性 Compiler/发布；首版只编译已有 Owner 事实 | 独立 PostgreSQL、并发/CAS、无环、Hash、重放和 stale 标记通过 |
| `SG-I04` | 在 `SG-I03` 发布契约上实现 Current/Version/Lens Query、Version Diff、上下游追踪和影响闭包 | Query 仅读、大图有界、相同版本结果确定，全量 CI 通过并已提交 |
| `SG-I05` | 只在 `SG-I04` 完成后建立 Backend-owned Stage Envelope/Policy/Candidate Revision，将 8 个过渡 Skill 收口为 `agent/skills/build-storygraph` Bundle、Stage Reference 和 Bundle Hash，原子删除旧 Skill 名 | 跨语言 fixture、golden、Bundle 完整性、旧 Invocation 精确路由和全量 CI 通过 |
| `SG-I06` | 按已接受 `2055` 完成 HumanTask 列表/详情、Claim/Renew/Release、Decision 和 Resume Backend API，复用已有 Review/Workflow 事实 | Owner Receipt、Signal unknown/recovery、权限、幂等/冲突和 API 重启恢复通过，无第二审核状态机 |
| `SG-I07` | 在 `SG-I06` 真实 API 上交付最小 Review Workbench，显式区分 Task、Decision、Owner Apply 和 Workflow Resume | 刷新、过期 Lease、unknown/conflict、键盘和可访问性自动化通过，不模拟 Backend 成功 |
| `SG-I08` | Definition-first 打通 `extract_source_evidence`：先发布 WorkflowDefinitionVersion 并创建 Run/NodeRun，再在该 NodeRun 下创建 ShardManifest/Invocation/Candidate Revision | Unicode 绝对区间、coverage、重分片、恢复和证据守恒通过 |
| `SG-I09` | Definition-first 接入有界 `analyze_story` map 和确定性 `reconcile_story` tree，产出带证据的 Bible/Claim Candidate Revision | Manifest/leaf 谱系、fan-in、重放、冲突和上游 stale 测试通过 |
| `SG-I10` | 接入 Bible `review_storygraph` 与有界 Candidate Repair，每轮重跑确定性 Gate | Candidate Revision/Head CAS、冻结允许集、修复预算和旧下游 stale 通过 |
| `SG-I11` | 用公共 Human Gate 审批 Bible/identity/state/claim Candidate；正向决议只调用既有 Production Bible `Confirm` Owner Command，固化 confirmed Bible Gate output 与 `production_bible.confirm` Receipt | blocker 未清零不得通过；Confirm 不物化 Asset，Decision/Receipt/Node output 精确绑定并可恢复 |
| `SG-I12` | 只消费 `SG-I11` 的 confirmed Bible 输出，由 Backend Coordinator 在独立命令中原子物化 Character/Location Asset、SpecificationVersion、AssetState 和 ProductionBinding | 幂等 Materialization Receipt、唯一 Owner、单 GORM 事务、失败回滚和反向追踪通过 |
| `SG-I13` | Definition-first 接入 `segment_episodes` Candidate，仅产出有证据的边界/顺序/标题提案 | 全文 coverage、无重叠/缺口、稳定顺序和恢复通过，不创建 Episode |
| `SG-I14` | 完成 Episode Plan Human Gate 与 Backend Owner 原子物化 Episode/Published ScriptVersion | 边界冲突、幂等、全批回滚和 Receipt 验收通过 |
| `SG-I15` | Definition-first 按 Episode Slice 接入 `analyze_episode` 与 `reconcile_episode`，产出 Scene/Dialogue/Beat/Occurrence/Claim Candidate | 只消费已确认 Bible Snapshot，分片、相邻边界、恢复和引用门禁通过 |
| `SG-I16` | 完成 Scene/Beat/Occurrence/Claim Review、Human Gate 和 Planning Owner 全批应用 | 未知身份/状态不得自动创建，整批 Receipt、回滚和反查通过 |
| `SG-I17` | 从已物化 Bible/Episode/Scene/Beat/Occurrence/Claim Owner 事实编译 Core StoryGraphVersion | 多集 DAG、Claim scope、Evidence、Owner Ref、Diff 和影响闭包全链通过 |
| `SG-I18` | 接入 Storyboard Draft，只消费非空正式 Specification/AssetState，产出可审核 Shot Intent 与 `needs_asset` 需求 | Candidate 不进入正式 StoryGraphVersion，缺资产时不得创建 Shot |
| `SG-I19` | 用公共 Human Gate 审批 Shot Intent/visual requirements；Storyboard Owner `FreezeIntentSet` 只冻结 Draft Set revision/hash、已接受 Intent 和视觉需求并返回 Receipt | Gate completed/Receipt/输出可恢复，不创建正式 Shot；拒绝、unknown 或漂移不得产生 Provider Cost/Job |
| `SG-I20` | 直接替换固定 Runware 配置，建立内置 Preset Catalog、Provider Connection/Credential/ModelProfile/Project Binding GORM 事实、加密 Secret Store 与零配置启动语义 | 单 SQL 事实源、API Key 只存密文、root key 缺失只阻塞配置/执行、无 Migration/Raw SQL/兼容入口，全量 CI 通过 |
| `SG-I21` | 建立精确 PriceQuote/Quota、ProviderJob/独立 ProviderCall/Receipt 聚合与 Registry Adapter Port，使用受控 Gateway 验证四候选四 Call、一次发送权、部分失败、unknown 与重启恢复 | 只有首次 `PENDING → DISPATCHING` 可 Submit，终态只结算一次，unknown 不释放预留也不盲目重提 |
| `SG-I22` | 在 `SG-I20`/`SG-I21` 真实 Backend/OpenAPI 上实现 Workspace Provider Settings、Credential 轮换、ModelProfile 和 Project Purpose Binding Web 旅程 | Owner 权限、Secret 只写不读、预设/字段/版本/PriceQuote/零配置状态与刷新恢复通过；无任意 Provider URL |
| `SG-I23` | 只消费 `SG-I19` approved intent，接入火山 Seedream 5.0 Pro+ 的精确 Profile、PriceQuote、真实 `reference_asset` 调用、Staging 和 Candidate | 真实凭据、单 Call 单输出、媒体校验、unknown 边界、重启和 Lineage 通过 |
| `SG-I24` | 在相同 Target/Owner 链接入 OpenAI GPT Image 2 与 snapshot 的精确 Image API 合同 | 独立连接/Profile/Binding、`n=1`、Base64 Staging、费用和真实调用恢复证据通过 |
| `SG-I25` | 在相同 Target/Owner 链接入 Google Nano Banana 2 Lite、2、Pro、Legacy 四个精确 Profile 与两种官方调用协议 | Interactions/Generate Content 不试探回退，四模型分别完成真实调用、Staging、费用和 Lineage |
| `SG-I26` | 完成 composite front/profile/back reference sheet 确定性 QC、公共 Human Gate、单一 CandidateSelection 和 AssetVersion 发布 | 三类图片 Provider 的身份/State/Style/lineage/view-role、Selection/Owner Apply 幂等与失败路径通过 |
| `SG-I27` | 让 `detail_shots` 只消费精确 READY AssetVersion，完成分片 Review 与 Candidate Repair | 精确版本非空，跨 Scene 连续性、修复范围和重审门禁通过 |
| `SG-I28` | 完成 Storyboard Human Gate/Owner Apply，创建正式 Shot 并发布完整 ShotProductionBindingVersion，编译下一 StoryGraphVersion | 全批原子、Binding 完整、精确 Owner Ref、冲突/回滚和重放通过 |
| `SG-I29` | 接入 `shot_frame` Target，在 Seedream/GPT Image/Nano Banana 三类图片 Adapter 上完成动态 Shot、CandidateSet/Selection 和 ShotImageBindingVersion | 精确 AssetVersion 输入、与 ShotProductionBindingVersion 不混用、单 Shot 局部重跑、结果对账和反查通过 |
| `SG-I30` | 实现严格 `shot_video` Target、视频 Artifact 元数据/FFprobe、Video QC、公共 Human Gate、ShotVideoBindingVersion 与 StoryGraph 受控 Edge | 精确首帧/生产 Binding/时长、Selection/Owner Apply、局部重跑、无 Provider URL 和跨 Target 绑定通过 |
| `SG-I31` | 接入 Seedance 2.0、2.0 Fast、2.0 Mini、2.5 精确 Profile/PriceQuote/异步任务协议，完成真实视频生成和同 remote id 恢复 | 四模型分别通过真实创建/查询/重启/Staging/Video QC/Selection/ShotVideoBindingVersion 全链 |
| `SG-I32` | 用 React Flow + Dagre 实现按 Episode/Scene 加载的单人只读 Story Lens，与 Workflow Lens 明确分离并展示图片/视频 Binding | Query、Diff、影响闭包、大图分层加载和无写入入口通过 |
| `SG-I33` | 在只读 Lens 通过后增加类型化 Domain Intent 编辑、Owner Command、重编译和 Patch Diff；Yjs/Hocuspocus 另立设计 | Canvas 无 Graph JSON/SQL 直写，过期 base 冲突和新 Version 反查通过 |
| `SG-I34` | 使用完整原稿执行全量机器统计、代表集人工细查、Provider 故障恢复与全量真实 CI，回填非浏览器最终证据 | 四类 Provider 真实验收和所有代码/全量 CI 完成，该 Acceptance Evidence 已独立提交 |
| `SG-I35` | 只在 `SG-I34` 通过并提交后运行 `agent-browser` Web Journey，回填浏览器/最终 Acceptance 并独立提交 | 浏览器、API、PostgreSQL Owner 事实、Provider Call 与图片/视频 Artifact 系谱一致，无未说明失败 |

每个 `SG-Ixx` 都是一个完整任务：先 Red，再 Green/Refactor；随后运行该任务所需的真实局部 CI 和当前全量 CI，回填该任务 Acceptance Evidence，检查 diff/hygiene，最后独立 Git 提交。任一项未通过都不解锁下一项。Backend 测试进入 `backend/tests/production/storygraph`；Agent 测试按性质进入 `agent/tests/unit`、`agent/tests/contract`、`agent/tests/integration`；Frontend 测试进入 `frontend/tests/unit` 或 `frontend/tests/e2e`，不与生产源码混放。不得用兼容 fallback、跳过检查或模型桩冒充正式闭环。

## 分阶段验收闭环

### A. Core StoryGraph MVP

用一份至少两集的真实剧本证明：原稿 → Evidence → Bible/Asset Specification/State → Episode → Scene/Beat/Occurrence → Core StoryGraphVersion 全链可恢复；每个正式图节点/边可反查唯一 Owner Ref、版本、Hash 和 Evidence；跨版本 Key、无环、Claim scope、线性发布、Diff 与影响闭包通过 Backend 自动化测试。另生成一批可追溯到 Core StoryGraph 节点的已审核 Storyboard Draft/Shot Intent，并明确保持 `needs_asset`；Candidate 不进入正式 StoryGraphVersion，也不伪装成 Shot。

### B. Visual Consistency Milestone

证明同一角色跨两集和两个 AssetState 仍只有一个 Asset/角色卡身份；一个已发布角色参考 AssetVersion 由单一 READY composite reference sheet 覆盖 front/profile/back；至少一个 Scene 绑定正确 Location AssetState；Storyboard Draft 输入 Specification/AssetState 非空，Shot Detail 在资产 READY 后发布完整 ShotProductionBindingVersion，Human Gate/Owner Apply 创建正式 Shot，并编译包含 Shot/Binding 的下一 StoryGraphVersion。正式 Shot 可反查 Occurrence → AssetState → AssetVersion，图片结果落入 ShotImageBindingVersion，视频 Target 消费该精确首帧并最终落入 ShotVideoBindingVersion；Provider 配置/凭据/Job/Call 不成为 StoryGraph 节点。

### C1. Read-only Canvas Milestone

先证明 Story Lens 能按 Episode/Scene 加载、展示 Claim 语义投影、版本 Diff 和影响闭包，不一次渲染全项目；此阶段没有编辑入口，Canvas 只消费 StoryGraph Query。

### C2. Domain Intent 与完整故事验收

再证明 Canvas 类型化 Domain Intent 只能通过真实 Owner Command 修改并触发重编译，不能写 Graph JSON。两集样本只证明契约，完整原稿还必须覆盖全部分集的机器统计和代表集人工细查，并完成四类 Provider 的真实图片/视频旅程。所有自动化、真实 CI 和完整故事链均通过后，最后才使用 `agent-browser` 完成浏览器故事验收。

## 文档派生与实现门禁

本主题的文档评审和同步顺序只由 [文档中心的 `SG-D01`–`SG-D21`](../README.md#当前-storygraph-设计文件推进顺序) 维护，本 Design 不复制第二套步骤。其中 `SG-D01` 必须先单独接受 0010，`SG-D02` 才能单独接受依赖它的 3003；之后按表逐份同步受影响 Design，最后才派生 PRD、Requirement、唯一 Plan 和全未勾选 Acceptance。

0010 PRD 统一拥有用户价值与跨服务范围，3003 不重复创建第二份产品愿景；3003 只派生 Agent 专项 Requirement 条目并被 0010 总 Plan 引用。已验收 `0009` 只增加演进链接，不回写历史范围或证据；旧 `3001/3002` 派生文档在同步前冻结。Acceptance 文档可以在编码前建立标准，但所有 Checklist 初始为 `[ ]`；没有真实执行证据不得勾选。

## 已接受的核心决策

1. StoryGraph 的权威边全部保持 DAG；天然成环的业务语义使用带 participant/anchor 输入边、有效范围和演进状态的 Claim Node 表达。
2. Storyboard 保留为分镜生命周期和 StoryGraph Lens，不做简单改名、删除或双写兼容。
3. 角色稳定身份、剧情形象、渲染画风和生成 Artifact 四轴分离；Character Look 只是 AssetVersion typed lens，MVP 用单一 composite reference sheet 覆盖 front/profile/back。
4. StoryGraph 首版使用 PostgreSQL/GORM 不可变 JSONB Version + 线性 Head 和 Backend 内存拓扑校验，不引入图数据库、Raw SQL 或通用关系表。
5. Canvas 首版采用 React Flow + Dagre 的按 Lens 子图，不提前实现多人协作和全项目无限展开。
6. 媒体生成使用 Backend-owned 通用 Provider 配置与严格 `reference_asset|shot_frame|shot_video` Target；StoryGraph 只投影 Target、Artifact 和四类 Owner Binding，不投影 Provider 配置、凭据、Job 或 Call。
