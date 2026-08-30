# Lanverse 剧本视觉生产升级实施计划

> 状态：VP-D15 已接受（2026-08-31）
>
> 接受依据：产品/依赖、合同覆盖、执行/回滚三轴独立反例审阅通过；用户已授权设计接受后直接实施
>
> 正文 SHA-256：6eadf98f607abc3dc42326d034f16d6ec7aae7c160f3e1a685d5f1cd991687a2
>
> 产品依据：[Lanverse 剧本视觉生产工作台产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md)
>
> 跨服务合同：[Lanverse 剧本视觉生产跨服务需求规格](../requirement/0010-StoryGraph内容图与DAG创作画布需求规格.md)
>
> Agent 合同：[StoryGraph 剧本解析 Harness 与内置 Skill 需求规格](../requirement/3003-StoryGraph剧本解析Harness与内置Skill需求规格.md)
>
> 验收入口：[Lanverse 剧本视觉生产升级验收标准](../acceptance/0010-StoryGraph内容图与DAG创作画布验收标准.md)
>
> 本计划被接受前不得编码；接受后只从当前唯一任务领取一个切片。

## 1. 计划结论

升级按 P0–P4 的十四个实现切片和一个最终浏览器验收切片顺序落地。每个切片必须形成用户可观察、服务端可查询、失败可恢复的纵向闭环，而不是先铺满表、接口或抽象层。

`VP-I01` 是唯一开工入口：以真实剧本完成 SourceVersion → ScriptSpanCandidate → style-blind SceneFactCandidate → 持久 Candidate 查询的最小闭环，并只建立这条路径实际消费的 production Wire、Bundle 与 Owner 基础。Interaction、Preset、Generation、Storyboard 和 Guided Studio 不得提前实现。

历史 `SG-I01`–`SG-I20` 只证明旧合同；当前未提交 `SG-I21` 修改属于用户既有工作区事实，实施时必须精确避让或在重叠文件内保留，不能被本计划自动提交、回滚或当作完成证据。

## 2. 当前事实与固定工具链

### 2.1 当前事实

- Backend 已有 production、storygraph、review、workflow、agent、asset、generation 等模块和真实 PostgreSQL/Temporal 基础，但其旧 Bible-first、旧 Stage/Wire、旧媒体/计费合同不能直接满足新 Requirement。
- Agent 已有 candidate_runtime 和单一 build-storygraph 入口，可作为迁移输入；现有 Stage 数、Wire、Bundle 与 Candidate 合同必须按 production 原子演进。
- Frontend 是单 Next.js/npm 应用，已使用 RTK Query/OpenAPI 生成类型；当前 /studio 流程不是新项目级 /production Guided Studio。
- 现有 CI、Docker Compose、真实基础设施和历史测试继续作为回归门，不创建第二套运行栈。

### 2.2 固定工具链

| 边界 | 固定选择 | 本计划限制 |
|---|---|---|
| Backend | Go、GORM Catalog、PostgreSQL、Temporal、现有单 Backend Binary | 不引入第二 ORM、Migration 框架、第二 Workflow 引擎或新微服务 |
| Agent | Python、Pydantic strict schema、单一 build-storygraph Bundle、受限 Codex/模型 broker | 不引入 LangGraph、动态 Skill 下载、Provider client 或业务 Writer |
| Frontend | Next.js、TypeScript、RTK Query、OpenAPI 生成类型、现有组件体系 | 不引入第二数据客户端、monorepo 或通用画布 |
| Media | 现有 Backend Provider/Asset 边界中的一条真实图片路径 | 不把支付、成本、多 Provider 广度或视频设为 MVP 前置 |
| Validation | Go/Python/TypeScript contract fixture、真实 PostgreSQL/Temporal/对象存储、浏览器 | mock 只用于确定性故障注入，不能抵扣最终语义闭环 |

## 3. 每个实现切片的统一执行门

每个 `VP-Ixx` 必须按以下顺序执行，任何一步失败都不得勾选 Plan 或 Acceptance：

1. 开工检查：确认 Git 根、分支、工作区；列出本切片精确文件白名单和需避让的用户修改。
2. Red：先新增能证明目标合同当前失败的 unit/contract/integration/journey 测试；不能先写测试时在 Evidence 中说明并提供最接近的可执行失败证据。
3. Green：只实现本切片纵向闭环和当前真实消费者；不预建后续 Owner、兼容层、目录或接口。
4. Refactor：去除重复 DTO、空转发层、旧 fallback 和无消费者依赖，保持模块职责与依赖方向。
5. 定向验证：执行本切片全部正例、负例、并发、重启或浏览器验证。
6. 全量门：执行当时仓库要求的 Go、Agent、Frontend、OpenAPI、Compose/Image、architecture、hygiene 与真实基础设施 CI。
7. 证据：在 Acceptance 对应 ID 与切片 Evidence Log 中记录真实命令、输入、结果、时间和残余风险；不得复制历史结果。
8. 提交：检查 staged diff 只含本切片和对应验收证据，以中文规范标题独立提交；不推送、不建 PR。

## 4. 唯一实施队列

### P0：文本制作世界

#### VP-I01 — Scene Fact 生产契约 首个纵向切片

目标：真实中文剧本在不受风格污染的前提下形成可重读 SourceVersion、全覆盖 ScriptSpanCandidate 与 SceneFactCandidate。

- [ ] Red：Unicode code-point、全覆盖、未知字段、注入文本、Hash 漂移、Agent 越权和零正式业务写入失败测试。
- [ ] 建立当前路径需要的 OwnerVersionIdentity、Canonical Hash、production Invocation/Result、Bundle allowlist 和 Candidate persistence 最小合同。
- [ ] 实现 SourceVersion 原子发布、propose_script_spans、extract_scene_facts、strict Candidate 接受与 typed Query 回读。
- [ ] 用至少一份真实多场中文剧本执行 Agent，不以 mock JSON 抵扣语义闭环。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

停止点：如果 production Wire 不能在 Go/Python fixture 同值，或 Source span 无法逐字回读，不进入 VP-I02。

#### VP-I02 — 全剧身份调和、Gate 1 与正式结构

目标：全部 raw mention 得到精确分区，用户在 Gate 1 审核结构与身份，Backend 以同一 Decision 恢复并发布正式结构/身份版本。

- [ ] Red：同名、别名、漏 mention、重复分区、stale subject、租约丢失、并发 Decision、Apply 后崩溃与重复 Resume。
- [ ] 实现 resolve_identities、Candidate Revision/Head、typed review/repair 和有界修复。
- [ ] 实现 structure_identity HumanTask、EffectPlan、两步 Owner Apply、Receipt 与 Workflow resume。
- [ ] 前端先复用现有工作台提供最小可审核 Gate 1，不提前建设完整 Guided Studio。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

#### VP-I03 — 制作世界、人物多形象、道具交互与 Gate 2

目标：正式形成 Character/Appearance、Location、Prop/State、SceneOccurrence、Interaction 与 Continuity，并在 Gate 2 原子应用。

- [ ] Red：同一角色两形象、同一道具两状态、人物持道具几何、交接、无原因瞬移、证据/创作者决议互斥和局部闭包。
- [ ] 实现 derive_production_entities、bind_scene_occurrences、reconcile_interaction_continuity 三阶段 strict Candidate。
- [ ] 实现 ProductionWorld、Occurrence、Interaction/Continuity 的版本、Head、read set 与三 Owner family 原子 Apply。
- [ ] 实现 Gate 2 六视图最小审核与 changes_requested 受影响闭包。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

#### VP-I04 — storygraph-production 制作投影与可追溯 Query

目标：把已发布 OwnerVersion 投影为无环 storygraph-production，并提供有界关系、反查和影响查询。

- [ ] Red：Owner 污染源、环、跨项目 ref、current/latest、Head CAS、事务中断、无界查询和 Search 故障。
- [ ] 实现 OwnerVersion-first Compiler、StoryGraphVersion/Head、typed node/edge、Receipt 与原子发布。
- [ ] 实现 DAG、上游/下游、反向证据、版本 diff 和 ImpactPreview 有界 Query。
- [ ] 证明 Query 零写入且不依赖 Elasticsearch 正常。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

### P1：视觉身份与基础参考

#### VP-I05 — Skill 供应链、4–6 Preset、视觉基础与 Gate 3

目标：建立可审计 Skill/Release 供应链和首批策展 Preset，使用户从正式制作世界选择视觉基底并批准 Reference Plan。

- [ ] Red：许可缺失、NOTICE 缺失、路径逃逸、注入、Bundle 漂移、Release 撤销、十三 Stage 集不完整、Preset 污染 P0 和非法 Gate 3 批准。
- [ ] 把成熟 Skill 吸收流程固化为 SourceInventory → review → rewrite → eval → shadow → signature；不复制未授权运行资产。
- [ ] 上架 4–6 个不可变、版本化、可追溯许可的 PresetRelease。
- [ ] 实现 resolve_visual_foundation、plan_reference_assets、expected target set、Gate 3 与原子发布。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

#### VP-I06 — 六类 Target 与 Provider-neutral Brief

目标：从已批准 ReferencePlan 生成六类 strict Target/Brief 和无环依赖计划，不执行媒体。

- [ ] Red：缺 Target、错误 view role、跨身份依赖、自由 Provider 参数、Secret、latest 和依赖环。
- [ ] 实现 character_anchor、character_appearance、location、prop、interaction、scene_composition 六类 Target。
- [ ] 实现 compile_reference_brief strict union、三视图/多面/状态/组合角色和冻结 read set。
- [ ] 提供 ReferenceCoverageMatrix 与 Target detail Query。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

#### VP-I07 — 图片执行、Bundle、确定性 QC 与 Vision Review

目标：至少一条真实图片路径把 strict Brief 变成 staging Asset、完整 Bundle、确定性 QC 和独立 VisionReviewCandidate。

- [ ] Red：唯一发送权、发送后断联、outcome_unknown、恶意媒体、错误尺寸、缺 view role、Bundle/Vision Hash 环和 Vision 越权选择。
- [ ] 实现 Target/Execution 分层、ProviderCall、staging、AssetVersion 提升与完整 Bundle 状态机。
- [ ] 实现 deterministic QC 与 review_reference_artifact 五类 typed issue。
- [ ] 用真实媒体验证六类 schema 中至少一个基础 Target 和一个缺陷样本。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

#### VP-I08 — Gate 4 基础 Bundle 选择与 checkpoint

目标：以完整 Bundle 为单位逐 Target 审核基础角色、形象、地点和道具，选择后安全推进依赖。

- [ ] Red：跨执行拼三视图、warning 未确认、error 豁免、非法 not_required、重复 Decision、Selection 后崩溃和并发选择。
- [ ] 实现 reference_target per-target HumanTask、NegativeRequirementProof、SelectionReceipt 与 Base Owner Apply。
- [ ] 实现 checkpoint 聚合、依赖解锁和精确 stale closure；不存在全局 Gate 4 approve。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

### P2：交互与场景组合

#### VP-I09 — Interaction/Scene Composition 与局部失效

目标：精确消费已选 base Bundle，生成人物—道具交互图和场景组合图，并只应用到对应场景。

- [ ] Red：错误 appearance/prop state、手位/尺度/朝向漂移、混用预设、组合失败覆盖 base、跨场景污染和局部重跑扩大。
- [ ] 完成 interaction 与 scene_composition 真实生成、Vision Review、逐 Target 选择与场景级 Owner Apply。
- [ ] 证明一个 scene_composition changes_requested 不使无关 base 和已通过场景过期。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

### P3：Packet-first 正式分镜

#### VP-I10 — Production Packet、direct_storyboard 与 Gate 5

目标：reference_ready 场景编译不可变 ProductionPacket，Agent 只基于 Packet 生成分镜，Backend 精确绑定并原子发布 ShotPlan/StoryGraph。

- [ ] Red：缺必需 Target、latest、needs_asset、歧义 local key、旧 Packet、连续性漂移、非法 repair、Gate 5 Apply 中断和重复 Resume。
- [ ] 实现 Scene Coverage、ProductionPacketVersion 与 direct_storyboard strict Stage。
- [ ] 实现 Backend Binding normalizer、确定性 Shot/Graph 校验、review/repair 和 scene-scope 原子 Apply。
- [ ] 完成 Gate 5、反向追踪和 Workflow Compiler 正式输入门。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

#### VP-I11 — 项目级 Guided Studio 与 typed Query

目标：以 /production 交付可完成五 Gate 的项目级工作台，服务端拥有 readiness、timeline 和 allowed actions。

- [ ] Red：刷新/深链丢状态、前端拼状态机、手写 DTO、越权按钮、窄屏阻断、键盘/焦点错误和失败伪成功。
- [ ] 提交新 Query 的 OpenAPI 并重新生成 RTK Query 类型；不得手写重复 DTO。
- [ ] 实现项目摘要、五 Gate 时间线、六视图制作世界、Preset/覆盖矩阵、Bundle 对比、Packet/Storyboard 和只读 DAG Lens。
- [ ] 实现 Impact Drawer、Inbox、统一状态、响应式与 WCAG 2.2 AA 相关条款。
- [ ] 用真实 Backend 数据完成浏览器阶段验收；最终全旅程仍留到 VP-I15。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

### P4：完整剧本、发布与硬化

#### VP-I12 — 恢复、安全、可观测与有界闭包

目标：对 Agent、Temporal、Provider、Gate、Asset 和 StoryGraph 的崩溃窗口完成同身份恢复，并封闭注入、Secret 和越权风险。

- [ ] Red：Agent/Backend 重启、租约丢失、Release 运行中撤销、Provider unknown、迟到结果、重复 Result、Secret 泄漏、路径/媒体注入和超预算。
- [ ] 完成 Attempt/lease/reconciliation、三道 Release fence、typed error、heartbeat/replay 和 stale reason。
- [ ] 完成结构化脱敏日志、短时资产预览、受限 Vision media read 与有界影响闭包。
- [ ] 定向故障矩阵、全量 CI、Acceptance Evidence 与独立提交完成。

#### VP-I13 — 十三 Stage 正式 Release、Eval 与完整剧本真实媒体矩阵

目标：以完整 CandidateStageSet 批准十三 Stage，完成 golden/adversarial/Vision eval、shadow、4–6 Preset 和完整剧本真实图片闭环。

- [ ] 十三 StageVariantKey 完整唯一，Definition/Bundle/Release/Attestation/Control/Signature 引用无环。
- [ ] golden 数据覆盖同名、两 Appearance、两 PropState、Interaction、Continuity、六类 Target 和中文 Unicode。
- [ ] adversarial、真实 Vision、shadow、forward/revocation 和真实 Agent/Codex eval 通过。
- [ ] 一份完整真实剧本达到 Production-ready Scene Coverage 100%，六类 Target 均有真实 Bundle 和反向追踪。
- [ ] 定向验证、全量 CI、Acceptance Evidence 与独立提交完成。

#### VP-I14 — 指标、性能与发布候选全量门

目标：固定覆盖率分子/分母、效率/恢复/守护指标和规模基线，形成无功能新增的发布候选。

- [ ] Backend Query 对 Production-ready Scene Coverage 与五类前导指标给出确定性计算。
- [ ] 记录 Gate 时延、局部重跑比例、unknown 对账时延、恢复成功率和闭包规模。
- [ ] 完整原稿规模下无默认全项目重跑、无未授权写入/跳过必需 Target/盲重试/跨项目污染/Secret 暴露。
- [ ] 全量真实 CI、镜像、Compose、OpenAPI、hygiene 和未提交用户修改隔离检查通过并独立提交。

#### VP-I15 — 最终浏览器验收与事实对账

目标：在 VP-I14 已提交后，用真实浏览器完成最终用户旅程并对账全链事实；本切片不新增实现。

- [ ] 从上传完整剧本到 Gate 5 完成，覆盖两 Appearance、两 PropState、持道具 Interaction、Preset、六类 Bundle、局部修改与正式 Storyboard。
- [ ] 浏览器、API、PostgreSQL、Temporal、Agent Invocation/Result、ProviderCall、AssetVersion、SelectionReceipt、StoryGraph/ShotPlan 逐项对账。
- [ ] 桌面/平板/窄屏、键盘、焦点、错误恢复、深链和可访问性检查通过。
- [ ] 只回填最终 Acceptance Evidence、运行文档检查并独立提交；失败则回到所属实现切片，不在本切片临时修代码。

## 5. Requirement 唯一主切片映射

下表是主责任映射；实现可在更早切片搭建被真实消费的最小部分，但只有主切片负责完成该合同和写入验收证据。范围互不重叠，VP-I15 只复核、不拥有新合同。

| 主切片 | Cross-service Requirement | Agent Requirement |
|---|---|---|
| VP-I01 | VPR-ARC-001–009；VPR-COM-001–008；VPR-P0-001–004 | VPA-BND-001–006；VPA-BDL-001–006；VPA-WIR-001–010；VPA-P0-001–002 |
| VP-I02 | VPR-P0-005–010；VPR-GAT-001–006、008–010 | VPA-STG-002–003；VPA-P0-003；VPA-CAN-001–005 |
| VP-I03 | VPR-WLD-001–011 | VPA-P0-004–007；VPA-CAN-006–008 |
| VP-I04 | VPR-QRY-003–005 | — |
| VP-I05 | VPR-PRE-001–009 | VPA-SUP-001–006；VPA-REL-001–010；VPA-VIS-001–003 |
| VP-I06 | VPR-REF-001–009 | VPA-VIS-004–007 |
| VP-I07 | VPR-GEN-001–009 | VPA-STG-004；VPA-VIS-008–009 |
| VP-I08 | VPR-GEN-010–011、013；VPR-GAT-007 | — |
| VP-I09 | VPR-GEN-012 | — |
| VP-I10 | VPR-STB-001–010 | VPA-STG-005；VPA-STB-001–005 |
| VP-I11 | VPR-QRY-001–002；VPR-FE-001–012 | — |
| VP-I12 | VPR-NFR-001–008 | VPA-RUN-001–009 |
| VP-I13 | VPR-JRN-001–008 | VPA-STG-001；VPA-EVL-001–008；VPA-JRN-001–006 |
| VP-I14 | VPR-MET-001–004 | — |
| VP-I15 | 无新增主合同，只复核 VPR-JRN、VPR-FE、VPA-JRN 与所有已记录 Evidence | 无新增主合同 |

## 6. P0–P4 发布门

| 阶段 | 包含切片 | 进入下一阶段前必须满足 |
|---|---|---|
| P0 Text Production World | VP-I01–I04 | 真实剧本完成 Gate 1/2；两 Appearance、两 PropState、Interaction/Continuity 可追溯；storygraph-production 可查询 |
| P1 Visual Identity | VP-I05–I08 | 4–6 Preset 可选；六类计划严格；基础 Bundle 经真实生成、Vision、逐 Target 选择 |
| P2 Composition | VP-I09 | interaction/scene composition 精确消费 base，局部失败不污染其他场景 |
| P3 Storyboard | VP-I10–I11 | Packet-first Gate 5 与项目级五 Gate 工作台可用，前端不直写正式状态 |
| P4 Hardening | VP-I12–I15 | 十三 Stage Release、完整剧本、真实媒体、恢复/安全/指标/浏览器与全量 CI 全部有新证据 |

## 7. 停止、回滚与重叠处理

- 任一切片发现 Design/Requirement 冲突，先停止并修正文档事实，不能用实现暗改合同。
- 数据合同在切片内原子切换；未提交时可通过删除本切片新增未引用事实恢复，但不得回滚用户已有修改或历史提交。
- 对与 `SG-I21` 重叠的 generation/workflow/OpenAPI/Frontend 文件，开工时逐文件确认 diff，保留用户修改；无法安全合并时报告精确冲突而不是覆盖。
- Provider outcome_unknown、Gate Decision 已写但 Effect 未完成、Owner 已写但 Workflow 未恢复等窗口必须按原身份对账，不能通过删除数据库或新建业务事实“恢复”。
- VP-I15 发现问题必须回到唯一所属切片修复、验证、提交后再重新运行最终旅程。

## 8. VP-D15 文档完成门

- [x] 十五个切片顺序、依赖、停止点和 P0–P4 发布门已明确。
- [x] 126 个 VPR 与 95 个 VPA 表格条款恰好映射到一个主实现切片，无遗漏、无重复。
- [x] Acceptance 为每个条款建立初始未勾选项，并使用同一主切片映射。
- [x] 所有实现目标与验收目标 Checklist 初始均为 [ ]，未复制任何 SG-Ixx 通过证据。
- [x] 产品/依赖、合同覆盖、执行/回滚三轴独立反例审阅完成。
- [x] 正文 SHA-256 在接受时写回文首。
