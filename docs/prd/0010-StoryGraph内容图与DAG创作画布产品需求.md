# StoryGraph 内容图与 DAG 创作画布产品需求

> 状态：已接受产品范围（`SG-D17`，2026-08-27）；不代表 Requirement、Plan、代码或验收已完成
>
> 设计依据：[StoryGraph 内容图与 DAG 创作画布设计](../design/0010-StoryGraph内容图与DAG创作画布设计.md)
>
> 相关设计：[StoryGraph Harness](../design/3003-StoryGraph剧本解析Harness与内置Skill设计.md) · [公共 Human Gate](../design/2055-Workflow公共HumanGate命令与恢复设计.md) · [前端功能模块](../design/1002-前端功能模块设计.md)

## 1. 产品结论

Lanverse StoryGraph MVP 让创作者把一份拥有合法使用权的完整剧本，逐步转换为可审核、可追溯、可恢复的剧集、场景、叙事节拍、角色/地点状态、分镜与视觉资产关系图。产品不再把 Storyboard 当作全部故事数据的终点；Storyboard 保留分镜生产职责，StoryGraph 负责连接完整内容和生产血缘。

用户最终可以回答并验证四类问题：

1. 这段剧情、角色状态、地点状态或镜头来自原稿哪里？
2. 同一角色跨集、跨造型是否仍是一个身份，并使用了哪个精确视觉版本？
3. 修改一个场景、状态或资产会影响哪些分镜和生成结果？
4. 某次 AI 生成、人工审核和 Workflow 运行是否真正写入了正确的正式业务事实？

MVP 的价值是证明“真实原稿 → 正式 StoryGraph → 视觉一致性 → 可浏览/可追踪”的最小生产闭环。它不承诺完整视频、声音、合成或多人协作平台。

## 2. 用户与核心任务

| 用户 | 核心任务 | 可交付结果 |
|---|---|---|
| 编剧/剧本统筹 | 检查分集、场景、节拍、人物关系、伏笔和连续性 | 每条正式关系可反查原稿 Evidence 和 Owner Version |
| 导演/分镜师 | 把已确认剧情拆成可生产 Shot，并理解上下游影响 | 可审核 Shot Intent、正式 Shot 和精确生产 Binding |
| 角色/资产负责人 | 维护一个角色/地点身份下的多个剧情状态和视觉版本 | 角色卡/地点卡、三视图 reference sheet、版本与使用范围 |
| 制片人/审核者 | 在付费生成和正式写入前审核候选 | 冻结 Subject、不可变 Decision、Owner Receipt 与恢复状态 |
| 运行维护者 | 定位异步任务、Provider、索引和日志问题 | Workflow/Receipt/Trace 可对账，重启与未知结果可恢复 |

## 3. 用户问题

当前已实现的剧本到分镜纵向切片把事实分散在 Bible Candidate、Episode Structure、Storyboard JSON、Shot、Artifact 和 Workflow Projection 中，产生以下产品风险：

- 同一角色或地点可能在不同剧集被重复识别，缺少统一卡片和状态演进；
- Storyboard 只引用叙事单元，无法证明 Shot 使用了哪个角色形象、地点状态和视觉版本；
- AI 候选、人工 Decision、正式 Owner Apply 和 Workflow 恢复容易在 UI 中被混成一个“完成”；
- 用户无法从剧本证据追到 Shot/Artifact，也无法可靠计算修改影响；
- 当前 Agent Skill 分散且职责重叠，难以形成可版本化、可恢复的整剧解析 Harness；
- 剧本/StoryGraph 检索和运行日志缺少真实异步投影链，排障依赖数据库或进程输出；
- 现有前端没有真正的 StoryGraph Canvas，不能按 Episode/Scene 直观浏览关系。

## 4. MVP 用户旅程

### 4.1 完整原稿到 Core StoryGraph

1. 用户选择已提交的完整 DocumentRevision；
2. Backend 创建正式 Workflow Definition/Run/NodeRun，再调用本地 Codex StoryGraph Harness；
3. Harness 分阶段提取 Evidence、Bible/Claim、Episode、Scene/Dialogue/Beat/Occurrence Candidate；
4. 确定性 Gate 和有界 Repair 处理结构、引用、证据与覆盖问题；
5. 用户通过公共 Human Gate 审核冻结 Candidate；
6. Backend 各 Owner 原子 Confirm/Apply，并保存 Receipt；
7. StoryGraph Compiler 只读取已确认 Owner 事实，发布不可变 Core StoryGraphVersion；
8. 用户可以按版本、Episode/Scene、实体和上下游影响查询。

### 4.2 角色/地点视觉一致性到正式 Shot

1. 用户在角色卡/地点卡查看唯一身份、Specification、AssetState 和缺失视觉资产；
2. Storyboard Draft 从正式 Scene/Beat/Occurrence 与非空 Specification/State 生成 Shot Intent；
3. Intent Gate 先冻结视觉需求；缺资产时明确 `needs_asset`，不创建 Shot 或 Provider Job；
4. 系统为至少一个角色生成 composite `reference_sheet` 候选，覆盖 front/profile/back；
5. 用户审核并选择唯一 READY 候选，Backend 发布精确 AssetVersion；
6. `detail_shots` 只消费精确 READY AssetVersion；
7. 用户审核后，Storyboard Owner 原子创建正式 Shot 和完整 ShotProductionBindingVersion；
8. StoryGraph Compiler 发布包含 Shot/Binding 的新版本；Shot Frame 生成结果写入独立 ShotImageBindingVersion。

### 4.3 Story Lens 与运行追踪

1. 用户从项目、Episode、角色/地点卡、审核任务或 Run 深链进入 Story Lens；
2. 首版按 Episode/Scene/Entity/Impact 有界加载，不一次展开完整项目；
3. 用户选择节点，在 Inspector 查看 Owner Ref、Evidence、Version/Hash、上下游与版本 Diff；
4. Story Lens 与 Workflow Lens 保持两套 ID/Query；内容节点不能冒充运行节点；
5. 只读 Lens 通过后，用户可提交少量类型化 Domain Intent；Backend 路由真实 Owner Command 并重编译新版本，前端不能写 Graph JSON。

### 4.4 搜索、异步投影与日志诊断

1. Backend 业务事务提交 Owner 事实与 Outbox；
2. Kafka 异步驱动 Script/StoryGraph Elasticsearch 投影和业务事件消费者；
3. 用户通过 Backend Search 查询剧本片段、角色、地点、Scene、Claim 或 Shot，并深链到精确 Owner/StoryGraphVersion；
4. 结构化日志通过 Kafka 进入 ELK，维护者按 trace/run/node/task/job/receipt ID 关联诊断；
5. Kafka、Elasticsearch 或 ELK 故障不得回写或覆盖 PostgreSQL/Temporal 的业务事实；恢复和重放收敛到同一投影。

## 5. MVP 范围

### 5.1 内容与版本

- SourceEvidence、ProductionBibleVersion、Episode、Scene、Dialogue、NarrativeBeat、Occurrence、Claim；
- Asset Identity、Character/Location SpecificationVersion、AssetState、ProductionBinding；
- 不可变 StoryGraphVersion、线性 Head、稳定 Node/Edge Key、Canonical Hash 和 DAG 校验；
- Current/Version/Lens Query、Version Diff、上下游追踪和影响闭包；
- Storyboard Candidate 与正式 Shot 生命周期保持独立，不做改名或双写兼容。

### 5.2 Agent 与审核

- 唯一 `agent/skills/build-storygraph` Skill Bundle 与 Backend-owned Harness；
- 本地 Codex CLI 作为开发阶段真实 AI 调用；Agent 只返回严格 Candidate/Patch；
- Stage/Shard/Candidate Revision、Head CAS、Evidence、确定性 Gate、有界 Review/Repair；
- 项目 HumanTask 列表/详情、Claim/Renew/Release、Decision 和同 Decision ID Resume；
- UI 明确区分 Task、Decision、Owner Apply、Workflow Resume。

### 5.3 视觉资产与分镜

- 一个角色身份可有多个剧情 AssetState 和多个画风下的 AssetVersion；
- 角色卡、地点卡和 Scene View 是只读组合，不创建 Card Record；
- `reference_asset` 与 `shot_frame` 两种严格 GenerationTarget；
- Runware 首个真实图片 Provider、Cost/Quota、UNKNOWN 对账、Staging、Artifact Readiness、QC 和 CandidateSelection；
- composite reference sheet 的 front/profile/back coverage；
- ShotProductionBindingVersion 冻结精确视觉输入，ShotImageBindingVersion 保存最终画面输出。

### 5.4 Workflow、消息与检索

- Temporal 是唯一跨步骤持久工作流引擎；
- Backend 是唯一业务 Writer，使用单一 PostgreSQL/GORM Model Catalog；
- Kafka 用于 Outbox 后的异步解耦、Script/StoryGraph Search Projection 和结构化日志传输；
- Elasticsearch 用于剧本与 StoryGraph 检索，索引可从 PostgreSQL 事实重建；
- ELK 用于日志收集、检索和运行诊断，不成为业务状态源；
- Provider/Workflow/Consumer 重试、重复投递、结果未知和服务重启可对账。

### 5.5 Frontend

- 当前 npm/Next.js 单应用和 RTK Query，不迁移 monorepo；
- 项目摘要、角色/地点卡、公共 Review Workbench、StoryGraph Search；
- React Flow + Dagre 的单人只读 Story Lens，按 Episode/Scene 有界加载；
- Story Lens 与 Workflow Lens 严格分离；
- 只读里程碑之后的类型化 Domain Intent，不提供通用 JSON 写图；
- 所有前端测试位于 `frontend/tests`。

## 6. 发布门

### Gate A：Core StoryGraph

- 至少两集真实剧本完成 Evidence → Bible/Identity/State → Episode/Scene/Beat/Occurrence/Claim → Core StoryGraphVersion；
- 所有正式节点/边可反查唯一 Owner Ref、Version/Hash 和 Evidence；
- DAG、稳定 Key、线性发布、Diff、影响闭包与恢复通过；
- Candidate、HumanTask 和 StoryGraph 正式事实无混写。

### Gate B：Visual Consistency

- 同一角色跨至少两集和两个 AssetState 仍只有一个身份；
- 至少一个角色 AssetVersion 来自单一 READY composite reference sheet，并完整覆盖 front/profile/back；
- 至少一个 Scene 使用正确 LocationState；
- 正式 Shot 绑定精确 AssetVersion，能反查 Occurrence/State/Style/Artifact lineage；
- reference/shot frame 的费用、配额、Provider UNKNOWN、QC、Selection 与两种 Binding 可恢复。

### Gate C：Canvas、Search 与诊断

- Story Lens 按 Episode/Scene 有界展示、深链、Diff 和影响闭包，首版无写入入口；
- 类型化 Domain Intent 只能通过真实 Owner Command 产生新 StoryGraphVersion；
- 剧本/StoryGraph Search 可返回可追溯结果；索引重复消费和重建结果一致；
- Kafka/ELK 故障不改变业务事实，trace 可关联 Workflow、Review、Provider 和 Receipt。

### Gate D：完整原稿与最终验收

- 完整原稿所有分集完成机器统计，代表集完成人工细查；两集小样本不替代全量；
- Backend、Agent、Frontend、Compose、镜像、OpenAPI、数据/Secret 卫生和当前真实 CI 全部通过；
- 每个完整 `SG-Ixx` 任务已有独立 Git 提交和真实验收证据；
- 最后才使用 `agent-browser` 执行 Web Journey，并核对浏览器、API、PostgreSQL Owner 事实、Temporal 与 Artifact lineage 一致。

## 7. 产品成功标准

以下标准必须由 Requirement/Acceptance 映射和真实运行证据计算，不能仅由文档宣称：

- 正式 StoryGraph 中无无 Owner、无精确版本/hash 或违反边类型矩阵的节点/边；
- 权威 StoryGraph DAG 环数量为 `0`；
- 未经 Human Gate/Owner Apply 进入正式 StoryGraph 或创建正式 Shot 的 Candidate 数为 `0`；
- 未绑定精确 READY AssetVersion 的正式 Shot 数为 `0`；
- 角色 composite reference sheet 缺 front/profile/back 仍被发布的数量为 `0`；
- Provider UNKNOWN 被盲目重新提交、重复扣费或重复占用配额的次数为 `0`；
- 同一幂等输入产生多个 Owner Receipt、StoryGraphVersion、Decision、Selection 或 Binding 的次数为 `0`；
- Search 重建后无法反查 PostgreSQL Owner/StoryGraphVersion 的结果数为 `0`；
- Agent、Frontend、Kafka Consumer、Search/ELK 对业务 Owner 表的写入次数为 `0`；
- 完整原稿 coverage、引用完整性和代表集人工审阅均有可重复报告，无未说明跳过项。

产品不预先承诺模型主观质量通过率或大图性能数字；这些阈值必须在 Requirement 中基于选定样本、环境和用户任务定义，并在 Acceptance 记录真实测量。

## 8. 非目标

- 完整视频/动作、配音、字幕、合成与最终成片；
- 多人实时 Canvas、Yjs/Hocuspocus、Presence、评论或离线合并；
- 通用知识图谱、图数据库、任意 EAV Schema、图查询语言或图插件市场；
- 通用 Agent 平台、动态 Tool Registry、无限 Repair 或 Agent 业务写库；
- 多 Provider 自动路由、Fallback、模型市场、复杂语义图片 QC；
- 地点/道具参考图全覆盖，除非真实 Shot Gate 证明首个闭环必需；
- 角色状态组合规则引擎、自动角色近似合并或自由文本“最新资产”绑定；
- 微服务拆分、多地域、多租户企业运营后台或 Kafka Command Bus；
- 为旧 Storyboard/静态 Provider Job/旧 Skill 建兼容双写或 fallback。

## 9. 约束与风险

- AI 分析有非确定性；Evidence、Review Issue、Human Gate 和 Owner Apply 不能被模型置信度替代；
- 完整剧本可能存在跨集边界、别名、隐含状态和矛盾，必须暴露歧义而非猜测合并；
- 本地 Codex 登录、Runware 凭据/额度、Temporal、Kafka、Elasticsearch、ELK、PostgreSQL 和对象存储是不同验收外部条件；缺失时只能记录对应未验收范围；
- Kafka/Elasticsearch/ELK 增加运行复杂度，但用户已明确其异步、检索和日志价值；MVP 只实现真实消费者，不搭建通用平台；
- 图片语义一致性不能完全由确定性 QC 证明，发布仍需要人工选择；
- 两集样本用于契约和恢复开发，不能替代完整原稿的最终效果验收；
- 本地绿色不等于远端 CI 绿色，任何失败检查都必须真实修复而非跳过。

## 10. 文档与实施门禁

本 PRD 是 StoryGraph 唯一产品范围来源；`3003` 只派生 Agent Contract Requirement，不创建第二份 Agent 产品愿景。下一步由 `SG-D18` 建立跨 Backend/Frontend/Workflow/Asset/Kafka/Search 可测契约，由 `SG-D19` 补 Agent 专项契约，再由 `SG-D20` 原样引用 `SG-I01`–`SG-I28` 建立唯一 Plan，`SG-D21` 创建初始全未勾选 Acceptance。

在 `SG-D21` 接受前不编码。编码后每个完整任务必须先 Red → Green → Refactor，再通过真实局部和当前全量 CI并独立 Git 提交；所有实现和非浏览器验收完成后才运行 `agent-browser`。
