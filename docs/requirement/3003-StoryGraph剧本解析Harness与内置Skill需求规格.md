# StoryGraph 剧本解析 Harness 与内置 Skill 需求规格

> 状态：VP-D14 已接受（2026-08-31）
>
> 接受依据：产品映射、Agent/Backend Owner、失败/恢复三轴独立反例审阅通过；所有条款从未来实施与验收的未通过状态开始，历史 Agent 证据不抵扣
>
> 正文 SHA-256：7ac8243bc1648a75ddb701b6e1dadfae1fcd4d0c5bfc54701f3eeea1c93dbbb6
>
> 设计依据：[StoryGraph Harness 与内置 Skill 设计](../design/3003-StoryGraph剧本解析Harness与内置Skill设计.md)
>
> 跨服务依据：[Lanverse 剧本视觉生产跨服务需求规格](0010-StoryGraph内容图与DAG创作画布需求规格.md)
>
> 产品范围：[Lanverse 剧本视觉生产工作台产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md)
>
> 下一文档门：VP-D15。本文只定义 Agent/Harness 可测合同，不建立第二份产品愿景或实施计划。

## 1. 范围、术语与边界

本文固定成熟 Skill 的吸收供应链、单一内置 Bundle、StageVariantKeyProduction、storygraph-stage-wire-production、十三个 Stage、Candidate Revision、Shard、Review/Repair、Vision 与发布恢复合同。

Backend 拥有 Definition、Release、Control、Invocation、Lease、Result、Candidate Head 和所有正式业务写入；Agent Runtime 只消费冻结输入并返回严格 Candidate。Agent 不得接收媒体 Provider 密钥，不调用图片或视频 Provider，不写数据库、对象存储、Kafka、Elasticsearch 或 Temporal。

验证术语与跨服务 Requirement 一致。所有 VPA ID 在 VP-D15 中必须恰好分配到一个主实施切片和至少一个初始未勾选验收项。

## 2. Runtime 与正式状态隔离

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-BND-001 | 最终运行入口唯一为 agent/skills/build-storygraph/SKILL.md；Runtime 不从用户目录、网络或当前工作区动态发现其他 Skill。 | Path + Container negative |
| VPA-BND-002 | Go Backend 唯一拥有 StageDefinition、StageRelease、ControlHead、CandidateStageSet、Invocation/Attempt/Result、ShardManifest、CandidateRevision/Head。 | Architecture + DB |
| VPA-BND-003 | Agent 成功只产生 CandidateArtifact 与诊断，不得 Confirm/Apply、分配正式业务 UUID、推进 Gate、恢复 Workflow 或发布 OwnerVersion。 | Journey + Zero-write |
| VPA-BND-004 | Agent Runtime 不包含 ORM、业务 Repository、Temporal、对象存储、Kafka、Elasticsearch、Provider client 或公共业务 HTTP route。 | Dependency + Network scan |
| VPA-BND-005 | Agent input 不含 Secret、Provider Endpoint、私有签名 URL、图片/视频字节；Vision Stage 只接收 Backend 颁发的受限媒体读取能力与稳定 Asset ref。 | Contract + Secret scan |
| VPA-BND-006 | Stage shard 挂到既有 WorkflowRun/NodeRun；Runtime 不建立与 Temporal 重复的 checkpoint 状态机。 | Integration + Replay |

## 3. 成熟 Skill 吸收、Bundle 与发布供应链

### 3.1 外部 Skill 吸收

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-SUP-001 | 每个外部 Skill 先建立 SourceInventory，记录来源 URL/commit、抓取时间、作者、版本、文件 Hash、许可、NOTICE、预期能力和审核人。 | Inventory audit |
| VPA-SUP-002 | 许可不明确、禁止再分发、包含凭据、隐式联网、指令注入、越权工具或不可追溯来源的 Skill 必须 quarantined，不能进入改写队列。 | Adversarial review |
| VPA-SUP-003 | 通过初审的材料分类为 adopt、rewrite、reference-only 或 reject；禁止原样复制外部运行时、提示词或工具声明进入生产 Bundle。 | Mapping review |
| VPA-SUP-004 | rewrite 必须映射到七个能力之一：parse-script-structure、build-production-bible、map-scene-continuity、resolve-visual-foundation、design-reference-assets、review-production、direct-storyboard。 | Capability matrix |
| VPA-SUP-005 | 每个改写通过 golden、adversarial、回归和边界 eval，再以 CandidateStageSet 进入 shadow；独立 reviewer 签署后才可批准。 | Eval + Signature |
| VPA-SUP-006 | 运行时 Bundle 不下载外部 Skill、不联网搜索“最新最佳实践”、不按来源项目结构加载文件；吸收结果必须是仓库内审计过的重写资产。 | Network + Path negative |

### 3.2 单一 Bundle 与渐进披露

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-BDL-001 | 固定目录只包含 SKILL.md、references、recipes、rubrics、eval 与 manifest 允许的资源；路径逃逸、符号链接逃逸、非 UTF-8、缺失或多余文件 fail closed。 | Filesystem adversarial |
| VPA-BDL-002 | SKILL.md 只保存跨阶段不变量、Owner 边界、证据规则和路由；阶段细则放 references，示例放 recipes，评审标准放 rubrics，不在 Python 复制同一指导。 | Structure audit |
| VPA-BDL-003 | 每个 StageRelease 显式列出该 Stage 允许加载的资源；Runtime 只加载入口和该白名单，不递归拼接全部 Markdown。 | Loaded-file golden |
| VPA-BDL-004 | BundleManifest 对相对 POSIX 路径排序并覆盖路径字节、内容长度、原始 UTF-8 内容、输出 schema 和允许工具计算 Canonical SHA-256。 | Go/Python golden |
| VPA-BDL-005 | Bundle hash、任一资源字节、output schema、tool policy 或 version 单独漂移都必须拒绝；不得用当前 Bundle 替代冻结版本。 | Mutation |
| VPA-BDL-006 | 非终态 Invocation 必须路由到精确 bundle_hash 对应的 Agent image digest；找不到返回 skill_bundle_unavailable。 | Rolling deployment |

### 3.3 Definition、Release、Control 与签名

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-REL-001 | StageVariantKeyProduction 精确由 stage_key、profile_key、lane_key、output_schema_version 构成；四字段共同决定变体身份。 | Schema |
| VPA-REL-002 | DefinitionCore 保存变体身份、input/output schema、allowed tools、resource policy、模型能力、预算与不变量，不引用 Release、签名或 Control，避免 hash cycle。 | Hash graph |
| VPA-REL-003 | StageRelease 保存 release_id、definition_hash、bundle_hash、agent_image_digest、model capability、eval attestation、created_at 与 predecessor_release_id。 | Contract |
| VPA-REL-004 | CandidateStageSet 必须对当前生产 Profile 的十三个 StageVariantKey 完整且唯一，并携带完整性 proof 和 policy proof；不能混用未声明 Release。 | Set equality |
| VPA-REL-005 | EvalAttestation 与 ShadowAttestation 绑定同一 CandidateStageSet hash、固定数据集/流量窗口和基线；基线只能是前一 approved set。 | Attestation golden |
| VPA-REL-006 | SkillRelease 在独立 reviewer 签名后引用 CandidateStageSet、Eval、Shadow、provenance 与 license proof；签名不进入被签内容本身。 | Signature |
| VPA-REL-007 | ControlRecord 状态只允许 approved、deprecated、quarantined、revoked；ControlHead 用 expected revision CAS 线性推进。 | State machine + Concurrency |
| VPA-REL-008 | revoked 为终止安全状态；恢复必须创建新 StageRelease 和新审阅，不得把原记录改回 approved。 | Negative |
| VPA-REL-009 | dispatch、accept result、apply candidate 三处分别验证 StageRelease、SkillRelease 和 ControlHead fence；任一已 quarantined/revoked 都失败关闭。 | Race + Fault injection |
| VPA-REL-010 | Release、Signature、Attestation、Control 与 Receipt 的引用方向必须无环，Canonical Hash 排除数据库当前态和运行时租约。 | Graph/hash property |

## 4. storygraph-stage-wire-production 与执行身份

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-WIR-001 | 公共 Invocation kind 只允许 storygraph_stage，wire_schema_version 固定 storygraph-stage-wire-production；不保留 production_bible、storyboard_draft 或无类型 map union。 | Strict schema |
| VPA-WIR-002 | Invocation 固定 invocation_id、attempt_id、StageVariantKeyProduction、StageRelease identity、SkillRelease identity、Control proof、scope、source refs、upstream refs、shard、payload、input_hash 与执行预算。 | Go/Python fixture |
| VPA-WIR-003 | scope 必须显式包含 workspace、project、episode 以及该 Stage 允许的 scene/entity/target；未知层级和跨项目引用拒绝。 | Negative |
| VPA-WIR-004 | source ref 使用完整 OwnerVersionIdentity；upstream ref 使用 CandidateRevision identity、producer Invocation/result hash；Agent 不得补全 current/latest。 | Mutation |
| VPA-WIR-005 | Stage input 为按 stage_key/profile_key 判别的 strict union，additional properties 默认 false；自由 JSON 只能存在于明确定义的 opaque evidence 字段。 | Schema fuzz |
| VPA-WIR-006 | input_hash 覆盖 wire version、variant、release、bundle、scope、排序 refs、shard manifest、payload 与执行预算；不覆盖 invocation_id、attempt_id、租约或 dispatch authorization。 | Cross-language golden |
| VPA-WIR-007 | dispatch authorization 在 Backend 运行时单独颁发并绑定 invocation、attempt、expiry 和 agent image；不能改变 Candidate 语义 Hash。 | Security |
| VPA-WIR-008 | AttemptResult 只允许 accepted、rejected、outcome_unknown，包含 input_hash、output_hash、diagnostic_hash、release fence 与完成时间；同尝试结果不可覆盖。 | State machine |
| VPA-WIR-009 | Go/Python 必须共用提交到仓库的正例、缺字段、未知字段、排序、Unicode、Hash 漂移和跨项目攻击 fixture。 | CI |
| VPA-WIR-010 | 旧 Wire 在 production 切片中原子移除或明确隔离为历史调用路径；不得 fallback 或自动转换成 production 正式 Candidate。 | Architecture negative |

## 5. 十三个 StageVariant 合同

生产 Profile 固定包含下列十三个 stage_key；除 review_candidate 与 repair_candidate 外 profile_key 为 default，review/repair 使用被评审目标的显式 profile。lane_key 首版为 primary，Vision Stage 仍通过 capability 和受限媒体读取声明隔离。

| stage_key | 输入 | Candidate 输出 | 明确禁止 |
|---|---|---|---|
| propose_script_spans | ScriptSourceVersion | ScriptSpanCandidate | 身份、风格、正式 Scene ID |
| extract_scene_facts | Source + spans | SceneFactCandidate | 视觉预设、身份合并、Owner 写入 |
| resolve_identities | Scene facts + raw mentions | IdentityResolutionCandidate | 静默造人、遗漏 mention |
| derive_production_entities | 正式结构/身份 + scene facts | ProductionWorldCandidate | 预设、参考图、正式 UUID |
| bind_scene_occurrences | Production world candidate + scenes | SceneOccurrenceCandidate | 全文模糊绑定、未证实状态 |
| reconcile_interaction_continuity | occurrences + evidence | InteractionContinuityCandidate | 覆盖冲突、无证据持有关系 |
| review_candidate | profile-bound candidate + rubric | ReviewCandidate | Apply、选择媒体、自由修复 |
| repair_candidate | candidate + typed issues + allowlist | 新 CandidateRevision | 原地 patch、越界字段修改 |
| resolve_visual_foundation | Gate 2 versions + WorldPresetRelease | VisualFoundationCandidate | 改写剧情事实、Provider 调用 |
| plan_reference_assets | foundation + production world | ReferencePlanCandidate | 删除 expected Target、媒体生成 |
| compile_reference_brief | approved plan + exact dependencies | strict ReferenceBriefCandidate | 自由 prompt、Secret、latest |
| review_reference_artifact | Bundle ref + restricted media reads | VisionReviewCandidate | 选择 Bundle、写 Asset、Provider 调用 |
| direct_storyboard | ProductionPacketVersion | StoryboardCandidate | needs_asset、联网搜索、正式 Shot 写入 |

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-STG-001 | CandidateStageSet 对上表十三个 stage_key 完整且无重复；缺一项、额外项或变体碰撞均不能批准。 | Set golden |
| VPA-STG-002 | 每个 Stage 使用独立 strict input/output schema、allowed resource list、model capability 和 max model calls；不能共用万能 Candidate。 | Registry audit |
| VPA-STG-003 | review_candidate 与 repair_candidate 的 profile 必须精确绑定被评审 Stage schema 和 rubric；未知 profile 拒绝。 | Contract |
| VPA-STG-004 | review_reference_artifact 是唯一允许 Vision 能力的 Stage，只能读取 Invocation 授权的稳定媒体引用。 | Capability negative |
| VPA-STG-005 | direct_storyboard 只能在 ProductionPacketVersion reference_ready 后 dispatch；前序 Stage 不能绕过 Packet 直接生成 Shot。 | Fence |

## 6. P0 解析与制作世界输出

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-P0-001 | ScriptSpanCandidate 用 code-point start/end、source_hash、临时 span_id 和 coverage proof；范围越界、重叠或缺口拒绝。 | Unicode/property |
| VPA-P0-002 | SceneFactCandidate 是 style-blind，保留 raw_character_mentions、raw_prop_mentions、地点、时间、动作、对白和逐字段 evidence spans。 | Golden + Injection |
| VPA-P0-003 | IdentityResolutionCandidate 对 raw mention 做 resolved/ambiguous/rejected 精确分区，输出 confidence、rationale 和 evidence，不产生正式 Character。 | Partition |
| VPA-P0-004 | ProductionWorldCandidate 严格区分 Character、CharacterAppearance、Location、Prop、PropState，并为每项给出 Evidence XOR CreatorDecision 提案。 | Schema |
| VPA-P0-005 | SceneOccurrenceCandidate 对 scene/subject/appearance_or_state/evidence/ordering 精确绑定，不以名称或 fuzzy search 代替身份。 | Contract |
| VPA-P0-006 | InteractionContinuityCandidate 同时输出人物—道具几何、持有/接触、相对尺度/朝向与跨场 appearance/prop state ledger。 | Journey |
| VPA-P0-007 | P0 Candidate 中的所有临时 ID 只能在同一 Candidate graph 内引用；Backend Apply 负责机械分配与返回正式 identity map。 | Integration |

## 7. Preset、六类参考与 Vision 输出

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-VIS-001 | WorldPresetRelease 只在 resolve_visual_foundation 及其下游出现；对同一 P0 输入切换 Preset 不得改变 span、scene fact、identity 或 production entity Candidate hash。 | Metamorphic |
| VPA-VIS-002 | VisualFoundationCandidate 分开输出 fidelity invariants、world adaptations、palette/material/light/camera rules 与 forbidden changes。 | Strict schema |
| VPA-VIS-003 | ReferencePlanCandidate 必须覆盖 Backend 提供的 expected target keys，类型只允许六类；只能补充规格，不能删除、改名或合并 Target。 | Set equality |
| VPA-VIS-004 | ReferenceBriefCandidate 使用六类判别 union 和固定 view roles；只表达 Provider-neutral 视觉要求，不含自由 Provider 参数、密钥或执行命令。 | Schema |
| VPA-VIS-005 | character_anchor 与 character_appearance 输出 front/profile/back；后者显式继承 anchor identity 与批准变化。 | Contract |
| VPA-VIS-006 | location 输出 empty_establishing/spatial_orientation/material_scale_detail；prop 输出 front/side/back/state_detail。 | Contract |
| VPA-VIS-007 | interaction 输出 interaction_master 并绑定 appearance、prop state、动作、手位/接触点、尺度和朝向；scene_composition 输出 composition_master 并绑定全部已选 base。 | Contract |
| VPA-VIS-008 | VisionReviewCandidate 至少含 identity、view_role、state、interaction_geometry、style_fidelity 五类 issue、severity、region/evidence 和 recommendation。 | Vision eval |
| VPA-VIS-009 | Vision Stage 不得返回 selected、approved 或 Owner mutation；Backend 结合 deterministic QC 与 Human Gate 决定 eligibility/selection。 | Negative |

## 8. Production Packet 与 Storyboard 输出

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-STB-001 | direct_storyboard 输入冻结每场景 ProductionPacket：source facts、appearance、location、prop state、interaction、continuity、selected assets 和 visual foundation。 | Fixture |
| VPA-STB-002 | StoryboardCandidate 使用 intent 判别 union 与 typed detail；不得出现 needs_asset、current/latest、Provider、搜索或未绑定自然语言实体名。 | Schema + Negative |
| VPA-STB-003 | 每个 Shot Candidate 携带临时 shot_id、scene ref、source span、主体、动作、构图、镜头、时长、连续性与 Packet 内 binding refs。 | Contract |
| VPA-STB-004 | Agent binding 只引用 Packet local key；Backend normalizer 独立解析为正式 OwnerVersion/AssetVersion，歧义时拒绝而非猜测。 | Integration |
| VPA-STB-005 | review_candidate 对 Storyboard 只输出 typed issue；repair_candidate 只能按 allowlist 生成新完整 CandidateArtifact。 | Repair negative |

## 9. Shard、Candidate Revision、Review 与 Repair

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-CAN-001 | ShardManifestProduction 不可变，包含 manifest_hash、scope universe、shard key、coverage、dependency closure 和 fixed-point proof；同阶段所有 shard 无重叠且完整覆盖。 | Property |
| VPA-CAN-002 | CandidateRevision 不可变，包含 revision_id、stage variant、shard、input_hash、output_hash、producer union、parent revision 和 status。 | Contract |
| VPA-CAN-003 | producer union 明确区分 Agent Attempt、Backend mechanical、Human correction；三者字段不可混用。 | Strict union |
| VPA-CAN-004 | 每个 stage_instance_key 只有一个 CandidateHead，更新使用 expected revision CAS；并发 repair 只能一个成功。 | Concurrency |
| VPA-CAN-005 | repair 输入必须是 typed issue、field-path allowlist、原 Candidate 与全部冻结 refs；输出完整新 Artifact，不接受 JSON Patch 或原地修改。 | Negative |
| VPA-CAN-006 | repair 后重新运行 schema、invariant、review 与 affected closure；未受影响 shard 保留原 Revision，不默认全剧重跑。 | Closure |
| VPA-CAN-007 | source、owner、release、manifest 或 upstream candidate 漂移使当前 Candidate stale；stale 只能重算或明确 canonical-empty rebase。 | Mutation |
| VPA-CAN-008 | Candidate rejected、stale、quarantined 后不能 Apply；历史 Artifact 仍可审计但不能成为 latest 输入。 | Fence |

## 10. Runtime、失败、恢复与安全

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-RUN-001 | Invocation 与 Attempt 分离；同 Invocation 可有多个 Attempt，但每次只允许一个有效 lease，成功 Result 收敛到同一 input_hash。 | Restart |
| VPA-RUN-002 | 超时或进程中断若无法证明未执行，Attempt 进入 outcome_unknown；Backend 先按 invocation/attempt/result identity 对账再决定重试。 | Fault injection |
| VPA-RUN-003 | dispatch 前、Result 接受前、Candidate Apply 前均重验 release/control/input fence；运行中撤销 Release 不能被旧结果绕过。 | Race |
| VPA-RUN-004 | Text broker 与 Vision broker 能力分离；除受限媒体读取外 allowed_tools 为空，Stage 不能自行打开网络、shell 或文件系统。 | Sandbox |
| VPA-RUN-005 | 每次 Attempt 使用独立临时目录和显式文件白名单，完成后可回收；不得读取项目无关文件、用户目录或凭据。 | Filesystem attack |
| VPA-RUN-006 | 剧本、Skill 引用、用户评论和媒体元数据全部按 untrusted data 处理；提示注入不能更改 Stage、工具、输出 schema 或 Owner 边界。 | Adversarial |
| VPA-RUN-007 | max model calls、单调用 deadline、总执行 deadline 和输出大小预算由 StageRelease 冻结；超限返回 typed error，不截断成合法 Candidate。 | Budget |
| VPA-RUN-008 | 错误至少区分 invalid_input、schema_mismatch、bundle_unavailable、release_blocked、lease_lost、timeout、outcome_unknown、model_unavailable、media_unavailable、internal。 | Error fixture |
| VPA-RUN-009 | HTTP 时限不充当 Workflow 总时限；Backend/Temporal 用心跳、retry policy、reconciliation 和持久 Receipt 恢复。 | Replay |

## 11. Eval、Shadow 与 CI

| ID | 必须满足的合同 | 最低验证 |
|---|---|---|
| VPA-EVL-001 | skill-creator 的结构校验用于仓库内 Bundle，但其通过只证明格式，不替代业务 eval、许可、安全和发布审阅。 | CI |
| VPA-EVL-002 | CI 检查 provenance、license/NOTICE、文件 allowlist、Canonical Hash、十三 Stage 完整性、Wire strictness 与跨语言 fixture。 | CI |
| VPA-EVL-003 | golden dataset 覆盖中文 Unicode、多集多场、同名人物、多 Appearance、多状态道具、人物持道具、跨场连续性和六类 Target。 | Dataset audit |
| VPA-EVL-004 | adversarial dataset 覆盖 prompt injection、伪造 system 文本、路径逃逸、恶意媒体元数据、跨项目 ref、latest 补全和超预算输出。 | Security CI |
| VPA-EVL-005 | Vision eval 使用固定真实图片样本和人工标注，分别评估五类 issue；不能只 mock 图片读取或只验证 JSON 可解析。 | Vision benchmark |
| VPA-EVL-006 | Shadow 在不写正式 Owner 的条件下运行完整 CandidateStageSet，与前一 approved set 对比质量、错误、时延和局部闭包。 | Shadow evidence |
| VPA-EVL-007 | forward test 在新 Release 批准后验证新 Invocation 使用新 set、在途 Invocation 保持冻结旧 set、revoked set 三道 fence 均拒绝。 | Deployment integration |
| VPA-EVL-008 | 最终端到端验收至少一次使用真实 Agent/Codex 执行关键 Stage；mock 只可用于确定性故障注入，不能抵扣最终语义闭环。 | Real-agent journey |

## 12. Agent 端到端旅程

| ID | 必须满足的旅程 | 完成证据 |
|---|---|---|
| VPA-JRN-001 | 真实剧本依次运行 spans、scene facts、identity，产生严格 Candidate，经 Backend Gate 1 Apply 后可追溯原 span。 | Agent/DB/Workflow |
| VPA-JRN-002 | production entities、occurrences、interaction continuity 识别两种形象、两种道具状态和持有交互，经 Gate 2 原子应用。 | Candidate + Owner versions |
| VPA-JRN-003 | 同一正式 P0 输入在两个 Preset 下保持事实 Candidate hash 不变，仅 visual foundation 和下游变化。 | Metamorphic evidence |
| VPA-JRN-004 | 六类 ReferenceBrief 通过 strict schema；Vision 对真实 Bundle 发现至少一个注入的 identity 或 geometry 缺陷且不自行选择。 | Brief/Vision evidence |
| VPA-JRN-005 | ProductionPacket 驱动 direct_storyboard，Backend normalizer 精确绑定，review/repair 形成新 Revision，经 Gate 5 原子应用。 | Full journey |
| VPA-JRN-006 | Agent 崩溃、租约丢失、bundle 缺失、release 撤销、outcome_unknown 和重复 Result 均在冻结身份上恢复或失败关闭。 | Fault matrix |

## 13. VP-D14 文档完成门

- [x] VPA-BND-001 至 VPA-JRN-006 的表格条款全部拥有唯一 ID、明确 Owner 和最低验证。
- [x] 十三个 Stage 与已接受 Agent Design 完全一致，六类 Target 和五类 Vision issue 与跨服务 Requirement 一致。
- [x] 外部成熟 Skill 只能通过来源、许可、安全、改写、eval、shadow、独立签名供应链进入生产。
- [x] Wire、Definition、Bundle、Release、Attestation、Control、Candidate 和 Receipt 的 Hash 引用无环。
- [x] 产品映射、Agent/Backend Owner、失败/恢复三次独立反例审阅完成。
- [x] 每个 VPA 条款都具备可供 VP-D15 分配唯一主实施切片和初始未勾选验收项的粒度。
- [x] 正文 SHA-256 在接受时写回文首，且覆盖从第一个正文标题到文件末尾。
