# Lanverse 剧本视觉生产跨服务需求规格

> 状态：VP-D14 已接受（2026-08-31）
>
> 接受依据：产品映射、Owner/事务、失败/恢复三轴独立反例审阅通过；所有条款从未来实施与验收的未通过状态开始，历史 SG-Ixx 证据不抵扣
>
> 正文 SHA-256：dfc9e7360dce9000dd373f7c826f581e7494347411ca1f4ea12c3371dd9241ef
>
> 产品依据：[Lanverse 剧本视觉生产工作台产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md)
>
> 设计依据：[剧本视觉生产工作台与世界观预设设计](../design/0011-剧本视觉生产工作台与世界观预设设计.md)、[项目制作圣经执行框架](../design/3001-项目制作圣经生成执行框架设计.md)、[Agent/Harness](../design/3003-StoryGraph剧本解析Harness与内置Skill设计.md)、[Storyboard Harness](../design/3002-本地-Codex-分镜智能体执行框架设计.md)、[Backend 领域](../design/2002-后端领域模块功能设计.md)、[Generation](../design/2051-通用媒体Provider与Generation执行器设计.md)、[Human Gate](../design/2055-Workflow公共HumanGate命令与恢复设计.md)、[Frontend](../design/1002-前端功能模块设计.md)
>
> Agent 合同：[StoryGraph 剧本解析 Harness 与内置 Skill 需求规格](3003-StoryGraph剧本解析Harness与内置Skill需求规格.md)
>
> 下一文档门：VP-D15。本文被接受前不得据此编写实施 Plan 或代码。

## 1. 目的、范围与验证约定

本文把已接受设计收口为可执行、可追踪、可失败关闭的跨服务合同。范围从完整剧本上传开始，经过结构事实、制作世界、世界观预设、六类视觉参考、逐目标审核、Production Packet 与正式分镜，结束于 Gate 5 接受后的 StoryGraph 与 ShotPlan 正式版本。

当前 MVP 明确不包含付费、成本、配额、视频生成、通用无限画布、多人实时协作和模型市场。既有 Cost、Quota、视频或旧 DAG 画布实现只作为历史事实，不能改变本文范围，也不能抵扣未来验收。

验证类型采用以下固定术语：

- Contract：Go、Python、OpenAPI 与前端生成类型的同一 fixture 正反验证。
- Unit：领域不变量、Canonical Hash、状态迁移和纯函数验证。
- Integration：真实 PostgreSQL、对象存储、Temporal 或 Agent 进程边界验证。
- Journey：从用户动作到正式 Owner 状态的纵向闭环验证。
- Browser：真实浏览器中的响应式、键盘、焦点、错误恢复和可访问性验证。
- Negative：越权、陈旧、重复、乱序、部分成功和未知结果必须失败关闭。

所有 ID 在 VP-D15 中必须恰好映射到一个主实施切片和至少一个验收项；不得以“由整体测试覆盖”代替明确映射。

## 2. 全局架构与唯一写入边界

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-ARC-001 | Browser 只调用 Backend 公共 API；不得直接调用 Agent、Temporal、对象存储写接口或媒体 Provider。 | Architecture + Negative |
| VPR-ARC-002 | Go Backend 是 ProductionWorld、StoryGraph、Review、Workflow、ReferenceAsset、Generation、Asset 与 Storyboard 正式状态的唯一写入者。 | Integration |
| VPR-ARC-003 | PostgreSQL/GORM 保存业务事实，Temporal 保存长流程控制；双方用冻结身份和 Receipt 连接，不复制彼此状态机。 | Integration + Replay |
| VPR-ARC-004 | Agent 只返回严格 Candidate；不得分配正式业务 UUID、确认 Gate、应用 Owner 版本、恢复 Workflow 或调用媒体 Provider。 | Contract + Negative |
| VPR-ARC-005 | 每个业务变更先形成 OwnerVersion，再由 StoryGraph 投影引用；StoryGraph 不得成为角色、形象、场景、道具、交互或视觉资产的第二写入源。 | Unit + Integration |
| VPR-ARC-006 | production 合同按一个实施切片内的 schema、writer、reader、fixture 原子切换；不得长期双写、fallback、latest 补全或静默兼容旧语义。 | Contract + Migration |
| VPR-ARC-007 | Workflow 只编排稳定宏观阶段；Agent shard、候选修订和视觉 Target 不得膨胀为动态 WorkflowDefinition 节点。 | Architecture + Replay |
| VPR-ARC-008 | MVP 生产主链不得依赖 Cost、Quota、Payment、视频生成或通用画布成功；相关服务故障不能阻断本文图片参考与分镜闭环。 | Journey + Fault injection |
| VPR-ARC-009 | 只实现已被当前纵向切片消费的表、接口、目录和抽象；不得预建未来兼容层、微服务或空 Owner。 | Diff audit |

## 3. 公共版本、Hash、Receipt 与幂等合同

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-COM-001 | OwnerVersionIdentityContract 固定包含 owner_kind、logical_id、version_id、revision、content_hash、created_at；引用必须携带完整身份，禁止 current/latest。 | Contract |
| VPR-COM-002 | 每个 logical scope 只有一个线性 Head；发布命令以 expected_head CAS，冲突返回 head_conflict 且不产生部分写入。 | Integration + Concurrency |
| VPR-COM-003 | Canonical Hash 对语义字段执行版本化、排序稳定、UTF-8 与长度明确的确定性编码；Go/Python/TypeScript fixture 必须同值。 | Cross-language golden |
| VPR-COM-004 | 业务 OwnerReceipt 证明命令已应用，Workflow EffectReceipt 证明副作用已被流程消费；两层 Receipt 单向引用，不得形成 hash cycle。 | Unit + Replay |
| VPR-COM-005 | rebase 只允许设计明确列出的空语义变化；任何证据、身份、状态、预设、视觉版本或绑定变化都必须重新计算并重新审核。 | Negative |
| VPR-COM-006 | 所有命令具有稳定 idempotency_key；同键同输入收敛到同一结果，同键异输入返回 idempotency_conflict。 | Integration |
| VPR-COM-007 | TypedReadSetProof 精确记录计算读取的 OwnerVersionIdentity 集合；应用时逐项验证，缺失或漂移返回 stale_read_set。 | Unit + Integration |
| VPR-COM-008 | audit 字段、数据库自增值、租约到期时间、current head 和运行时授权不进入内容 Hash；其余影响语义的字段不得排除。 | Golden + Mutation |

## 4. P0：剧本结构事实与 Gate 1

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-P0-001 | ScriptSourceVersion 保存原始 Unicode 字节、规范化换行策略、code-point 索引规则和 source_hash；span 不能用字节偏移混充字符偏移。 | Unicode golden |
| VPR-P0-002 | 接受新剧本源必须在同一事务发布 SourceVersion 和 SourceHead；失败不得留下孤儿版本或悬空 Head。 | Integration |
| VPR-P0-003 | propose_script_spans 的分段按 source 范围无重叠、无遗漏覆盖；暂时 ID 只在 Candidate 内有效。 | Property + Adversarial |
| VPR-P0-004 | extract_scene_facts 在身份解析前只输出 style-blind 的地点、时间、动作、对白、原始人物提及、原始道具提及和证据 span；不得注入预设或视觉描述。 | Contract + Injection |
| VPR-P0-005 | resolve_identities 必须把全部 raw mention 精确划分为 resolved、ambiguous 或 rejected；不得丢项、重复归属或静默造人。 | Partition property |
| VPR-P0-006 | Gate 1 Subject 固定绑定 SourceVersion、SpanCandidateRevision、SceneFactCandidateRevision 与 IdentityCandidateRevision 的完整身份和 Hash。 | Contract |
| VPR-P0-007 | Gate 1 接受后先原子发布 EpisodeStructureVersion，再独立原子发布 IdentityResolutionVersion；任一步失败都可重放且不得重复应用。 | Integration + Fault injection |
| VPR-P0-008 | EpisodeStructureVersion 与 IdentityResolutionVersion 只能表达结构与身份，不得提前写角色形象、地点规格、道具状态、预设或参考图计划。 | Negative |
| VPR-P0-009 | changes_requested 必须包含 typed issue、证据 span 与允许的修复范围；自由文本不能成为机器执行的唯一输入。 | Contract |
| VPR-P0-010 | P0 可独立形成首个纵向切片：真实剧本输入、Agent Candidate、Gate 1 决策、Owner Apply、Workflow 恢复和查询回读全部可运行。 | Journey |

## 5. 制作世界、连续性与 Gate 2

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-WLD-001 | Backend 机械地把身份事实划分为 confirmed、ambiguous、rejected 三个全集分区；Agent 不得决定正式分区。 | Unit + Property |
| VPR-WLD-002 | 角色、角色形象、地点、道具、道具状态和交互的每项事实必须是 Evidence XOR CreatorDecision，且记录来源、作者与版本。 | Contract |
| VPR-WLD-003 | Character、CharacterAppearance、Location、Prop、PropState 使用独立稳定 logical_id 和 version；不得用名称、文件名或提示词充当身份。 | Integration |
| VPR-WLD-004 | 同一 Character 可有多个 Appearance；服装、年龄阶段、伤妆、湿身、伪装等改变必须形成独立 Appearance，而非覆盖角色锚点。 | Journey |
| VPR-WLD-005 | SceneOccurrence 精确绑定 scene、subject_kind、subject_id、appearance_or_state_id、evidence 与顺序；同一场景内的出现不得靠全文搜索推断。 | Contract |
| VPR-WLD-006 | InteractionSpec 精确绑定 actor appearance、prop state、动作、手位/身体接触、相对尺度、朝向、连续性和证据；不得仅保存“人物拿道具”的提示词。 | Contract + Journey |
| VPR-WLD-007 | ContinuityLedger 对相邻场景记录 appearance、prop state、损坏、污渍、持有关系和位置的进入/离开状态，并能指出冲突证据。 | Unit + Journey |
| VPR-WLD-008 | Gate 2 Subject 固定绑定 ProductionWorldCandidate、SceneOccurrenceCandidate、InteractionCandidate、ContinuityCandidate 与全部上游正式版本。 | Contract |
| VPR-WLD-009 | Gate 2 接受由一个命令原子发布 ProductionWorldVersion、SceneOccurrenceVersion 和 ContinuityVersion 三个 Owner family；任一失败全部回滚。 | Integration |
| VPR-WLD-010 | changes_requested 可只关闭受影响的 scene/entity shard；闭包必须包含其交互与连续性邻接，不得默认重跑全剧。 | Closure property |
| VPR-WLD-011 | Gate 2 之前不得选择世界观预设、规划参考图、生成媒体、写 StoryGraph 正式版本或生成 Shot。 | Negative |

## 6. 世界观预设、视觉基础、参考计划与 Gate 3

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-PRE-001 | MVP 内置 4–6 个不可变、版本化、可追溯许可与 NOTICE 的 WorldPreset；用户只选择，不在主链编辑任意风格 DSL。 | Inventory + License |
| VPR-PRE-002 | P0 SceneFact、Identity 与 ProductionWorld 提取完全 style-blind；WorldPreset 只能从 Gate 2 正式结果之后参与。 | Mutation |
| VPR-PRE-003 | WorldPreset 同时包含 fidelity invariant 与 world adaptation rule；改编风格不得改变角色身份、剧情事实、持有关系和场景连续性。 | Contract + Adversarial |
| VPR-PRE-004 | ReferenceTargetKind 严格只允许 character_anchor、character_appearance、location、prop、interaction、scene_composition 六类。 | Schema |
| VPR-PRE-005 | plan_reference_assets 先从正式制作世界机械计算 expected target set，再由 Agent 提出规格；Agent 不得删除必需 Target。 | Set equality |
| VPR-PRE-006 | Gate 3 Subject 固定绑定 WorldPresetRelease、VisualFoundationCandidate、ReferencePlanCandidate、expected target set 和 Gate 2 正式版本。 | Contract |
| VPR-PRE-007 | Gate 3 接受在一个命令中原子发布 VisualFoundationVersion 和 ReferencePlanVersion；Preset 选择作为前者内容的一部分冻结。 | Integration |
| VPR-PRE-008 | 没有可用图片生成能力时允许保存 Draft Plan，但 Gate 3 不显示 approve；不得在 Gate 3 调用 Provider 或生成图片。 | Capability negative |
| VPR-PRE-009 | 切换 Preset 只使视觉基础、参考计划、下游 Brief/Bundle/Selection/Packet/Storyboard 过期；不得使 ScriptSource、SceneFact、Identity 或 ProductionWorld 过期。 | Impact closure |

## 7. 六类参考 Target、Brief、Generation 与 Vision

### 7.1 Target 与 Brief

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-REF-001 | ReferenceTarget 是创作意图和依赖身份；GenerationExecution 是一次执行事实。两者不得共享状态枚举或让重试创建新 Target。 | Domain unit |
| VPR-REF-002 | 每个 Brief 必须绑定 ReferencePlanVersion、VisualFoundationVersion、目标 OwnerVersion、dependency selection、StageRelease 和 typed read set；任一漂移后不能执行。 | Fence |
| VPR-REF-003 | character_anchor 必须请求 front、profile、back 三视图，并冻结体型、面部、发型、比例和不可变身份特征。 | Contract + Media |
| VPR-REF-004 | character_appearance 必须请求 front、profile、back 三视图，且继承 character_anchor 身份，只改变已批准的服装/年龄/伤妆等状态。 | Contract + Vision |
| VPR-REF-005 | location 必须请求 empty_establishing、spatial_orientation、material_scale_detail，不能用带主角的气氛图替代空间基底。 | Contract + Media |
| VPR-REF-006 | prop 必须请求 front、side、back、state_detail；多状态道具必须保持身份并展示明确状态差异。 | Contract + Vision |
| VPR-REF-007 | interaction 必须请求 interaction_master，绑定已选 appearance、prop/state、姿势、握持、接触点、相对尺度和朝向。 | Contract + Vision |
| VPR-REF-008 | scene_composition 必须请求 composition_master，绑定场景发生的已选角色形象、地点、道具、交互和连续性状态。 | Contract + Vision |
| VPR-REF-009 | Target dependency graph 必须无环：anchor/location/prop base 先于 appearance/interaction，全部所需 base 先于 scene_composition。 | DAG property |

### 7.2 执行、候选、审核与选择

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-GEN-001 | MVP 至少有一条真实图片 Provider 路径可生成六类 Target；不得因未接支付或成本模块而阻断。 | Real-provider journey |
| VPR-GEN-002 | ProviderCall 以唯一 submission_token 获得一次发送权；发送后断联进入 outcome_unknown，必须先对账，禁止盲重试。 | Fault injection |
| VPR-GEN-003 | Provider 输出先进入 staging；校验媒体类型、尺寸、Hash、恶意内容和目标身份后才可提升为 AssetVersion。 | Integration + Security |
| VPR-GEN-004 | ReferenceBundle 由共享 frozen execution set 与 view-role member 构成，Hash 不包含 VisionReview；VisionReview 引用 Bundle，禁止循环。 | Hash golden |
| VPR-GEN-005 | CandidateBundle 状态严格为 generated、deterministic_rejected、vision_reviewed、eligible、selected、superseded；非法跃迁拒绝。 | State-machine |
| VPR-GEN-006 | deterministic QC 只检查文件、尺寸、数量、Hash、角色覆盖和依赖完整性；语义一致性由独立 Vision Review 负责。 | Unit |
| VPR-GEN-007 | Vision Review 至少输出 identity、view_role、state、interaction_geometry、style_fidelity 五类 typed issue 和证据区域；不得选择候选。 | Vision contract |
| VPR-GEN-008 | warning 必须被用户逐项确认后 Bundle 才可 eligible；error 必须修复或重新生成，不能通过自由文本豁免。 | Journey |
| VPR-GEN-009 | 选择以完整 Bundle 为单位，不能拼接不同执行的 front/profile/back 冒充一致三视图。 | Negative |
| VPR-GEN-010 | 选择命令先写 SelectionReceipt，再由独立 Owner Apply 消费；重复信号收敛，失败可恢复。 | Replay |
| VPR-GEN-011 | base 目标选择后更新 ReferenceAssetSelectionVersion；只使其依赖 Target 和场景组合过期。 | Impact |
| VPR-GEN-012 | interaction 与 scene_composition 选择应用到对应场景范围，不得覆盖无关场景或全项目 Head。 | Scope integration |
| VPR-GEN-013 | Gate 4 是逐 Target HumanTask 与 checkpoint 聚合，不存在一个全局“全部视觉通过”任务；每个 Target 均保留独立决定和恢复边界。 | Workflow journey |

## 8. 五个 Human Gate 的公共合同

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-GAT-001 | HumanTaskInput 固定 gate_key、subject union、input_hash、impact summary、evidence refs、allowed decisions 与 workflow identity。 | Contract |
| VPR-GAT-002 | gate_key 只允许 structure_identity、production_world、visual_plan、reference_target、storyboard 五个值。 | Schema |
| VPR-GAT-003 | Decision 只允许 approved、changes_requested、rejected、not_required；每个 Gate 声明允许子集和 typed payload。 | Schema + Negative |
| VPR-GAT-004 | claim/renew/release 使用 lease token 和 expected revision；过期、越权或旧 token 不得提交决定。 | Concurrency |
| VPR-GAT-005 | 提交 Decision 前重算 input_hash；任何 Subject 或依赖 Head 漂移返回 stale_subject 且不应用。 | Integration |
| VPR-GAT-006 | 每种 accepted decision 先产生显式 EffectPlan，列出 Owner 命令、expected heads、read set 和恢复目标；不得用 if/else 隐式猜测。 | Contract |
| VPR-GAT-007 | not_required 只能用于 expected-set 机械证明为非必需的 Target，并记录 NegativeRequirementProof；不能手工跳过必需项。 | Negative |
| VPR-GAT-008 | DecisionReceipt、EffectReceipt 与 WorkflowResumeReceipt 分层持久；崩溃后用同一 Decision 继续，不能要求用户再次点击。 | Replay |
| VPR-GAT-009 | HumanTask、Decision 和 Effect 状态枚举互不混用；查询层可聚合展示但不能覆盖 Owner 状态。 | Contract |
| VPR-GAT-010 | 服务端从成员资格与项目角色计算 allowed_actions；前端隐藏按钮不能替代授权。 | Security |

## 9. Production Packet、正式分镜与 Gate 5

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-STB-001 | Scene Coverage 只有在该场景 expected Target 全部 selected 或有合法 not_required proof 后才为 reference_ready。 | Coverage property |
| VPR-STB-002 | ProductionPacketVersion 对每场景冻结文本事实、角色 appearance、地点、道具状态、交互、连续性、已选 AssetVersion 与视觉基础；禁止 latest。 | Contract |
| VPR-STB-003 | direct_storyboard 只能读取 ProductionPacket、StageRelease 和显式创作约束；不得联网搜索、补全最新状态或调用媒体 Provider。 | Agent negative |
| VPR-STB-004 | Shot Candidate 使用 intent union 加 detail，而不是自由 needs_asset；每个 Shot 必须绑定 scene、source span、主体、空间、动作和连续性。 | Schema |
| VPR-STB-005 | Backend Binding normalizer 把 Agent 临时引用精确解析为 Packet 内 OwnerVersion/AssetVersion；歧义或缺失必须拒绝。 | Integration |
| VPR-STB-006 | ShotPlan 编译、图校验与顺序校验为确定性 Backend 逻辑，不委托 Agent 决定正式合法性。 | Unit |
| VPR-STB-007 | review_candidate/repair_candidate 只返回 typed issue 与允许 schema 内的新 Candidate；不得直接 patch 正式 ShotPlan。 | Contract |
| VPR-STB-008 | Gate 5 接受以 scene scope 原子发布 ShotPlanVersion 与 StoryGraphVersion；单场失败不污染其他场景。 | Integration |
| VPR-STB-009 | ShotPlan 应用前再次验证 StoryGraph read set、Packet、Reference Selection 与 Continuity Head；陈旧返回 stale_read_set。 | Fault injection |
| VPR-STB-010 | Workflow Compiler 只消费已发布 OwnerVersion 与 Gate EffectReceipt，不能从 Agent Candidate 或前端缓存编译执行计划。 | Architecture |

## 10. Query、DAG 投影与前端工作台

### 10.1 Query 与投影

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-QRY-001 | Backend 至少提供 ProjectProductionSummary、GateTimeline、ProductionWorldDetail、ReferenceCoverageMatrix、ReferenceTargetDetail、ScenePacketDetail、StoryboardDetail 与 ImpactPreview 八个 typed Query。 | OpenAPI contract |
| VPR-QRY-002 | readiness、coverage、stale reason、allowed action 与时间线由 Backend 计算；前端不得从多个请求自行拼状态机。 | Contract |
| VPR-QRY-003 | StoryGraphVersion 是 OwnerVersion 和关系的不可变投影；节点携带 owner identity，边携带 typed evidence/continuity/dependency，不复制 Owner 内容。 | Integration |
| VPR-QRY-004 | DAG 投影必须无环、可追溯到 Source span，并能从场景反查角色形象、地点、道具、交互、资产和 Shot。 | Graph property |
| VPR-QRY-005 | Graph 与 DAG 只作为有界只读关系 Lens；MVP 不提供任意拖拽改写正式 Owner 的通用画布。 | Browser + Negative |

### 10.2 Guided Studio

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-FE-001 | 主入口为项目级 /production；旧页面只可链接进入，不能形成第二套制作状态。 | Router test |
| VPR-FE-002 | 前端 API 类型来自已提交 OpenAPI 的生成产物并由 RTK Query 管理服务状态；禁止手写重复 DTO。 | CI |
| VPR-FE-003 | project_id、episode_id、scene_id、gate_key、target_id 进入 URL；刷新、后退、深链保持同一工作上下文。 | Browser |
| VPR-FE-004 | Gate 1 同屏展示原文证据、场景切分和身份歧义，支持逐问题 changes_requested。 | Browser |
| VPR-FE-005 | Gate 2 以角色/形象、地点、道具/状态、场景出现、交互和连续性六个视图审核制作世界。 | Browser |
| VPR-FE-006 | Gate 3 先选 4–6 个 Preset，再展示 expected Target 覆盖与六类计划；缺生成能力时解释为何不可批准。 | Browser |
| VPR-FE-007 | Gate 4 用 Target 卡片和完整 Bundle 对比视图展示 view role、依赖、QC、Vision issue、历史与逐项决定。 | Browser |
| VPR-FE-008 | Gate 5 同屏展示 Packet 证据、Shot 列表、绑定、连续性和 StoryGraph/DAG 只读 Lens。 | Browser |
| VPR-FE-009 | 任意变化先打开 Impact Drawer，展示失效对象、保留对象、重跑范围和不可逆副作用，再提交命令。 | Browser + Contract |
| VPR-FE-010 | Inbox 聚合待我处理、即将过期、changes requested、provider_unknown 和失败任务；Provider Settings 不进入项目主旅程。 | Browser |
| VPR-FE-011 | 页面统一展示 draft、running、needs_review、changes_requested、blocked、failed、stale、ready、approved，并保留 Owner 原始状态详情。 | Visual regression |
| VPR-FE-012 | 桌面、平板和窄屏均可完成五 Gate；键盘、焦点、对比度、错误摘要和 aria 语义满足 WCAG 2.2 AA 的相关条款。 | Browser + Accessibility |

## 11. 非功能、安全与恢复

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPR-NFR-001 | 剧本文本、Skill、Preset、Provider 输出和用户评论全部视为不可信数据；不能改变系统指令、允许工具或 Owner 边界。 | Injection suite |
| VPR-NFR-002 | Provider Secret 只在 Backend 凭据边界解密；不得进入 Agent input、日志、OpenAPI response、前端缓存或 Hash fixture。 | Secret scan |
| VPR-NFR-003 | 私有资产预览使用短时授权或同源代理；持久记录只保存稳定对象身份，不保存签名 URL。 | Integration |
| VPR-NFR-004 | 结构化日志至少带 project、workflow、stage、target、invocation、decision 或 provider_call 的适用身份；不得记录完整剧本或密钥。 | Log contract |
| VPR-NFR-005 | 跨服务错误至少区分 validation、conflict、stale、unauthorized、unavailable、timeout、outcome_unknown、quarantined 和 internal。 | OpenAPI + Journey |
| VPR-NFR-006 | 长流程在 Backend/Temporal 以 checkpoint、heartbeat、retry policy 和 reconciliation 恢复；HTTP 请求时限不能充当流程总时限。 | Restart + Replay |
| VPR-NFR-007 | changes_requested 与上游变化用有界依赖闭包计算；规模随受影响节点和边增长，不得默认扫描或重跑全项目。 | Performance property |
| VPR-NFR-008 | 每次失效都给出机器可读 stale_reason、caused_by_identity、affected_scope 和 recommended_action；不得只显示“请重试”。 | Contract |

## 12. 必须通过的端到端旅程

| ID | 必须满足的旅程 | 完成证据 |
|---|---|---|
| VPR-JRN-001 | 一份真实完整剧本完成上传、P0 Candidate、Gate 1、Owner Apply 与回读。 | API/DB/Workflow/Agent evidence |
| VPR-JRN-002 | 同一角色至少两个 Appearance、一个至少两个 State 的道具、一次人物持道具 Interaction 和跨场 Continuity 通过 Gate 2。 | Owner versions + UI |
| VPR-JRN-003 | 用户可在 4–6 个 Preset 中切换，并验证只失效 Gate 3 之后的视觉链。 | Impact proof |
| VPR-JRN-004 | 六类 Target 各至少生成一个真实媒体 Bundle；三视图、道具多面/状态、交互和场景构图通过 deterministic 与 Vision 审核。 | Real assets + reviews |
| VPR-JRN-005 | Gate 4 每个必需 Target 均 selected 或有合法 not_required proof，Production-ready Scene Coverage 达 100%。 | Coverage numerator/denominator |
| VPR-JRN-006 | 一个 scene_composition changes_requested 只重跑该场景与必要依赖，其他已选 Base Bundle 和已通过场景不失效。 | Closure diff |
| VPR-JRN-007 | Agent 超时、Backend 重启、Temporal replay、Provider outcome_unknown、媒体校验失败和重复 Gate signal 均在原身份上恢复。 | Fault matrix |
| VPR-JRN-008 | Gate 5 接受后可从任一 Shot 反查 Packet、资产、Target、制作世界实体、SceneFact 与原始剧本 span。 | Reverse trace |

## 13. 产品指标与守护指标

| ID | 必须满足的计算合同 | 最低验证 |
|---|---|---|
| VPR-MET-001 | Production-ready Scene Coverage 的分母是 Gate 2 正式 Scene 集；分子是 expected Target 全部满足且 Packet/Storyboard Gate 已通过的 Scene，由 Backend 计算。 | Query golden |
| VPR-MET-002 | 前导指标至少包含 identity resolution coverage、interaction coverage、reference target coverage、bundle pass rate 和 stale closure size。 | Metrics contract |
| VPR-MET-003 | 运营指标至少包含各 Gate 首次通过时延、changes_requested 次数、局部重跑比例、outcome_unknown 对账时延和恢复成功率。 | Observability |
| VPR-MET-004 | 守护指标为越权写入、跳过必需 Target、盲重试未知 Provider、跨项目污染、Hash/Wire 漂移和未授权 Secret 暴露均为零。 | Security + Incident query |

## 14. VP-D14 文档完成门

- [x] VPR-ARC-001 至 VPR-MET-004 的表格条款全部拥有唯一 ID、明确 Owner 边界和最低验证。
- [x] 五 Gate、六类 Target、ProductionWorld、Interaction、Continuity、Preset、Packet-first Storyboard 与局部恢复均能从 PRD 追踪到合同。
- [x] 本文与 3003 Agent Requirement 的 Stage、Wire、Candidate-only 和媒体边界没有冲突。
- [x] 每个 VPR 条款都具备可供 VP-D15 分配唯一主实施切片和初始未勾选验收项的粒度。
- [x] 产品映射、Owner/事务、失败/恢复三次独立反例审阅完成。
- [x] 正文 SHA-256 在接受时写回文首，且只覆盖从第一个正文标题到文件末尾。
