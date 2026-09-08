# Agent Harness 专业能力与创作流程设计

- 状态：已接受目标（2026-09-07 用户确认按新设计全面推进）；已实施四类专业文本 Harness 任务，生产编排与正式采纳接线尚未完成
- 日期：2026-09-07
- 上位边界：[0013 创作编排与多媒体画布架构调整](0013-创作编排与多媒体画布架构调整设计.md)
- 联动设计：[1003 多媒体创作画布与运行可视化](1003-多媒体创作画布与运行可视化设计.md)
- 既有设计：[3003 StoryGraph Harness](3003-StoryGraph剧本解析Harness与内置Skill设计.md) · [3001 制作圣经](3001-项目制作圣经生成执行框架设计.md) · [3002 分镜 Harness](3002-本地-Codex-分镜智能体执行框架设计.md)

## 1. 结论、问题与范围

Harness 是有界专业任务的执行内核：固定输入、选择已发布 Skill、装配上下文、调用推理/受限工具、验证结果，并把可保存的候选交回可信应用层。Python Temporal 编排长期创作流程；Go 保持正式采纳、供应商操作、权限与账本的唯一边界。三者不共享可变执行状态。

当前 `StoryGraphHarness`、`SceneAnalysisHarness` 已有包摘要校验、严格 Schema、来源校验和单次调用限制，`build-storygraph` 已有多 Stage 指引；新设计需要补齐可组合的专业能力、按场上下文、持久执行预算、草案发布及对画布的检查点协议。不能把拆出多个 Skill 文件当作这些能力已经存在。

本文负责 Task、Skill、上下文、结果检查、候选修订、专业流程和执行描述。平台事务/迁移/事件传输归 0013；节点坐标、分组和 UI 操作归 1003。非目标为自由多 Agent 群聊、模型生成可执行 Workflow、推理进程直接读写数据库、42 个常驻服务，以及一次性实现所有流程。

## 2. 执行边界与调用循环

```mermaid
flowchart TD
  W[Temporal 注册 Activity] --> A[可信应用层领取任务与预算]
  A --> R[校验 Task 与 Skill Release]
  R --> C[冻结输入并装配上下文]
  C --> H[Harness 调用推理或受限只读工具]
  H --> V[结构 来源 引用 范围 覆盖检查]
  V -->|可修复且仍有预算| P[受限修复或补读]
  P --> H
  V -->|有效候选 含待审问题| S[应用层保存草案 输出绑定 Outbox]
  V -->|技术失败或预算耗尽| F[持久失败或待恢复记录]
  S --> G[平台审阅与正式采纳]
```

职责分为四层：

| 层 | 负责 | 不负责 |
| --- | --- | --- |
| Workflow | 阶段顺序、动态集合、依赖、Timer、等待与取消传播 | 直接 I/O、模型调用、数据库 DAG 轮询 |
| Activity/应用层 | 授权核验、任务领取、预算预留、草案事务、平台端口、执行结果对账 | 把模型建议直接认定为正式决定 |
| Harness | 上下文、Skill 选择、推理循环、受限工具、校验与修复分类 | 跨小时审批等待、长期调度、直接采纳或扣费 |
| 推理适配器 | 执行一个受限请求，返回结构化结果与可获得的使用信息 | 获得平台数据库、供应商长期密钥或任意系统工具 |

Harness 返回结构化候选/Issue；应用层完成持久化后才返回可消费的结果引用。`output_writer` 若存在，属于可信应用层端口，数据库凭据不得进入模型可访问的子进程。

## 3. Task、快照与结果合同

| 合同 | 必要内容 | 约束 |
| --- | --- | --- |
| TaskEnvelope | task/invocation ID、run/step ID、scope、Skill Release 引用、snapshot_ref、授权引用、budget_ref | 服务端身份验证后派生租户；输入不允许替换工具或动态类路径 |
| InputSnapshot | 正式源版本、输入资源 revision、上游采纳映射、用户锁定、允许输出、模型/Skill/策略摘要 | 不可变；短期令牌不写入快照；副作用前重新鉴权 |
| ContextManifest | 实际装载的片段/资源、hash、裁剪/摘要记录、遗漏项、工具补读证据 | 可以解释“本次看到了什么”；关键原文不得静默截断 |
| AttemptResult | attempt/fence、输入与 release hash、执行状态、候选或错误、usage 观察 | 尚未成为唯一接受结果；迟到成功只能留审计 |
| PersistedOutput | 被接受结果、候选 revision、checks_ref、OutputBinding、usage_ref | 可读取且已提交后才能发布 ready |
| Proposal | 目标类型、base revision、内容或受限 Patch、来源、未决事项、修改范围 | 模型不能分配平台正式 ID，也不能自签通过 |

ResourceRef、RunCommand、AcceptanceReceipt、跨语言 canonical 规则沿用 0013，不重新定义同名身份。

执行状态、审阅、质量、新旧状态与正式采纳分别保存。`succeeded` 表示本次任务产生了符合合同的结果；该结果可以包含 unresolved 或语义 blocker。Schema/来源无效属于技术失败，不能包装成可采纳候选。自动检查通过也不等于平台已采纳。

## 4. 上下文组织与原稿保真

输入按“任务规则 → 当前原文 → 已采纳对象 → 相邻状态与披露约束 → 相关背景 → 示例”装配。原稿、参考内容、用户意图与系统规则使用明确数据边界，附件中的指令不能注册 Skill 或改变工具策略。

长稿先保留完整源和可追溯分块，再按块提取、按类型归并。分块记录原文区间与重叠规则，汇总按来源区间去重；不能先压缩为摘要后宣称已完成全部实体和对白提取。

全局解析覆盖所有已上传原稿；逐场导演只读取当前场、必要上下文、已确认状态及该场允许披露的信息。全局确认“蒙面人是周野”，不意味着早期镜头可显示其脸部。人物身份/造型/出场、地点/地点变体/叙事场次、道具类型/实例/状态分别建模。

SourceRef 固定源版本、块与 Unicode code point 区间、文本 hash；规范化文本保留与原稿的映射。引用错误只能重读或重提案，不能让模型猜 hash。有效来源证明可追溯性，不自动证明模型语义判断正确。

超出上下文预算时，先缩小 scope 或请求已授权片段；可摘要背景，但台词与必需证据不可静默丢弃。仍无法完整处理时返回上下文不足与缺失范围，由 Workflow 重新拆分或进入人工处理。

## 5. Skill 的内容、执行声明和发布

SkillRelease 固定包名/版本、内容摘要、输入输出 Schema、参考文件、执行类型、允许工具、修改范围、修复/用量限制、产物角色与评测版本。`SKILL.md` 表达专业方法；执行权限取发布声明、部署策略、调用授权和本次批准范围的交集，不能由正文覆盖。

沿用 [Agent Skills 公开格式](https://github.com/agentskills/agentskills/blob/main/docs/specification.mdx) 的 SKILL.md 与附属资源组织；schemas、execution.yaml、评测格式和 StepDescriptor 是 Lanverse 扩展，不是公开格式自带的执行保障。先复用现有 `build-storygraph` 的真实指引与校验，逐步拆出可独立测试/发布的能力，不批量创建空包。

42 项能力保留原统一设计稿名称，分工如下；这是目标覆盖目录，不代表均已安装或实现。

| 专业域 | 能力目录 | 核心产物 |
| --- | --- | --- |
| 剧作解析（8） | inspect-manuscript、divide-episodes、summarize-episode、segment-scenes、mark-story-beats、extract-dialogue-cues、verify-text-coverage、propose-script-revision | 来源、分集/场次、节拍/对白、覆盖账与改编差异 |
| 全局设定（8） | collect-cast、collect-places、collect-props、resolve-entity-aliases、map-story-relations、trace-continuity、audit-story-facts、compile-asset-needs | 提及、身份提案、关系/状态与制作需求 |
| 视觉开发（5） | design-character-sheet、design-location-sheet、design-prop-sheet、define-visual-style、bind-reference-material | 外观与风格提案、明确用途的参考 |
| 导演分镜（8） | analyze-scene-intent、arrange-blocking、design-shot-coverage、write-shot-briefs、time-dialogue-and-action、review-screen-direction、plan-frame-board、audit-scene-coverage | 场次拆解、调度、覆盖、ShotBrief 和 FrameBoard |
| 镜头制作（5） | prepare-image-instructions、prepare-video-instructions、assess-shot-feasibility、review-generated-take、plan-selective-retake | 配方、可行性、审片问题与重拍提案 |
| 声音交付（4） | plan-voice-performance、align-caption-script、propose-edit-assembly、review-delivery | 配音、字幕、EditPlan 和交付检查 |
| 参考修订（4） | analyze-reference-film、derive-style-playbook、explain-change-impact、replan-affected-work | 拉片研究、风格配置与受限修订方案 |

现有 Stage 与目标 Skill 通过显式迁移表映射，不能用新包名继续返回旧语义 payload。新包发布需要正例、反例、缺证据例、越权例和独立评测；执行固定版本，升级只影响新调用。撤回阻止新执行，旧产物仍可审计；不能用相近包代替找不到的版本。

## 6. 专业结果如何形成

| 步骤 | 模型负责 | 代码/领域负责 | 下游消费 |
| --- | --- | --- | --- |
| 分集分场 | preserve/propose/revise 范围内提出边界和理由 | 首尾覆盖、重复/缺号、空范围、原文映射和采纳 | StoryMap 与稳定 Episode/Scene 映射 |
| 提及与身份 | 基于来源提出 link/merge/split/uncertain | 类型/范围/共现约束、稳定 ID、人工合并与撤销记录 | WorldBook 与各场 Presence |
| 剧情状态 | 从节拍提出状态变化、认知和披露证据 | 分支/时间与 known/unknown/conflicting 处理 | 镜头入口/出口状态 |
| 导演方案 | 叙事目的、站位/视线、镜头覆盖与表演意图 | 必拍节拍/对白映射、引用、时长和硬依赖检查 | SceneBreakdown、ShotBrief、FrameBoard |
| 生成配方 | 在能力范围内提出提示和参考方案 | 硬约束、授权、报价、预留和供应商操作 | RenderRecipe、Take |
| 审片 | 时间/区域定位问题及不确定性 | 解码/时长等技术检查、人工决定与正式选择 | QC Issue 与 TakeSelection |
| 改稿 | 解释影响并在许可集合内重规划 | DependencyIndex 计算字段影响、批准范围和复用验证 | 新修订运行；历史交付保持冻结 |

SourceCheck、ReferenceCheck、ScopeCheck、CoverageCheck 为确定性检查；语义评测保留证据、检查器版本与人工复核。身份、表演或运镜缺少可评价证据时允许 not_evaluable，不能用 Schema 成功推导艺术质量通过。

视觉参考可以按选定集/场需求生成，不必先为全剧所有角色和造型出图；事实解析与实体整理仍覆盖已上传全稿。SceneBreakdown → ShotBrief → RenderRecipe 必须保留中间产物，不能只用一个提示词字段代替导演设计。

## 7. 持久创作流程与人工门

| Flow | 拥有的创作顺序 | 完成依据 |
| --- | --- | --- |
| CreateSeriesFlow | 全稿分析、全局设定、选定剧集制作 | 子流产物与平台正式回执 |
| ReadManuscriptFlow | 原稿检查、分集、逐集/场提取、覆盖与审阅 | StoryMap/场次草案及采纳映射 |
| BuildStoryWorldFlow | 分场提取、全局归并、状态/关系与资产需求 | 固定 WorldBook 及正式采纳 |
| DevelopVisualsFlow | 按需求设计、申请生成、参考审阅 | 选定范围的 VisualLibrary |
| DirectEpisodeFlow | 分场导演、有界汇总与时长/依赖检查 | 整集可审阅导演方案 |
| DirectSceneFlow | 意图、调度、覆盖、ShotBrief、校时、方向与画板 | 来源 → Beat → Shot → Panel 关联 |
| ProduceShotFlow | 已批准配方、关键帧、视频、审片和选择等待 | 候选、QC 与平台选片/绑定回执 |
| ProviderCallFlow | 确保平台 operation 被接收并等待结果 | Go Generation 原操作结果；不直接访问供应商 |
| FinishEpisodeFlow | 声音、字幕、剪辑提案、渲染与交付等待 | Go 冻结的 ReleaseBundle |
| ReviseProductionFlow | 受影响范围重规划、复用与重新审阅 | 新运行关联旧基线与 ChangeSet |
| StudyReferenceFlow | 已授权视频的探测、边界/帧、语义分析、归组与风格提炼 | ReferenceStudy/ReferenceAnalysis、AnalyzedShot、分组与待审 StylePlaybookDraft |

这是职责目录，不要求 11 个 Flow 同时形成代码骨架。纯计算步骤无需创建子 Workflow；需要长期等待、独立取消或有界 fan-out 才建立持久边界。子流并发受租户、推理配额、供应商容量与预算限制；Continue-as-New 需保留逻辑步骤身份和在途处理策略。

参考片的 ReferenceStudy 是专业分析产物，ReferenceAnalysis 是其版本化跨边界索引；AnalyzedShot 表达原片镜头，通过显式 adaptation_link 才能转为待制作 ShotBrief。单帧不足以验证完整运镜，不确定摄影参数保留 unknown。边界修正保留已分析区间，只重抽受影响邻域；StylePlaybookDraft 经过审阅才可入库。

Go 先保存 ReviewDecision，再完成已有显式 Owner Effect，最后可靠通知目标 Python 流程。任一 checkpoint 未完成都不可宣称整项采纳成功。收到拒绝或修改请求时，Workflow 按冻结流程配置进入指定修订分支，不能抹去原结果从全稿重新开始。

## 8. 重试、修复、预算与取消

Invocation 的预算和控制头是持久事实；每次 Attempt 领取时核验剩余额度并取得 fencing token。当前 Harness 内存计数仅约束单进程调用，不能直接充当跨重启预算。新的调用前先登记额度占用，结果返回后按可获得的 usage 对账；未知用量不能当零。

| 情况 | 行为 |
| --- | --- |
| 发送前本地失败 | 同 Invocation 在剩余预算内创建下一 Attempt |
| 执行结果未知 | 先查原执行/已存产物；无可恢复执行时经明确策略重新尝试，可能产生推理费用 |
| Schema 可修复错误 | 在冻结上限内结构修复；不能顺便修改故事或扩大输入范围 |
| 来源错误 | 返回失效片段，补读原文；禁止猜证据 |
| 身份歧义/覆盖缺口 | 保存可审阅问题或限定提案，不随机重试到“通过” |
| 语义修订 | 固定父候选、Issue 与允许字段，应用 typed Patch 后重验，产生新 revision |
| 用户改稿 | 新 snapshot/修订运行，旧结果标 stale；已采用候选在新选择前保持可追溯 |
| Deadline/取消 | 终止并等待本次推理子进程退出，处理残留进程和额度；不占用无限后台任务 |
| 旧 Worker 迟到 | CAS 核验 claim、release/control 与当前 Attempt；失败结果只留审计 |

技术 Attempt deadline、Invocation 累积预算、业务人工等待期限分别定义。暂停不重置预算；重试不得变更原供应商 operation ID。额外创意候选需要新的明确请求与预算。

## 9. 给画布的执行合同

StepDescriptor 与已发布 StepSpec 绑定，至少包含稳定 step_key、业务标题、阶段、scope_kind、progress_unit、output_roles、允许的 renderer 类型和门禁描述。运行前编译固定描述；前端不能根据日志猜阶段，也不各写一份业务步骤清单。

renderer 为受控枚举，不能携带动态组件路径。Descriptor 声明可用操作种类；实际能否执行还由平台当前权限、版本和业务状态决定，不能仅凭前端按钮启用获得授权。

StepInstance 表达一个 scope 的逻辑工作；StepAttempt 表达技术尝试；OutputBinding 使用 step_instance_id + output_role + item_key 绑定持久资源 revision。首次结果填充稳定槽位，重试更新尝试状态，不能重复创建镜头卡。部分结果保存 completeness=partial，不解除正式生产门禁。

运行事件只发送已提交检查点：接受、展开、开始、部分产物、结果就绪、待审、失败和完成。资源/输出事件带可读取引用；工具细节只提供脱敏操作摘要，不暴露隐藏推理、全量原稿或凭据。传输、去重与项目游标沿用 0013；1003 负责投影表现。

进度单位为可验证计数或不确定阶段。动态发现更多集/场时显示范围更新；没有可靠供应商百分比时显示提交时间、最近查询和等待原因。

## 10. 实际落点、兼容与设计验证

保留 `agent/app/modules/storygraph` 的候选与专业校验，复用 `candidate_runtime` 现有传输边界；新可信应用/执行层按需要添加，不能为整张目标架构提前建空目录。代码层区分 task receiver、context builder、skill resolver、reasoning adapter、result checker、repair policy 和持久输出端口，不要求一个职责一个文件或类。

旧 Go-owned Candidate 与旧 Bundle/Wire 保持原身份；新 Agent-owned Proposal 使用新版本合同。接受后同步 3003/3001/3002 的所有权、阶段与恢复条款，保留 style-blind 事实、来源、typed repair、显式 Gate 和调用预算规则，避免两套同名 Candidate Head 同时可写。

设计验证场景：长稿跨块与中文/表情来源；目录/正文集号冲突；别名误合并与撤销；蒙面身份披露；多人手机；闪回道具状态；必拍节拍遗漏；单场失败与局部修订；输出保存后响应丢失；预算跨重启；过期批准；旧 Attempt 迟到；Skill 撤回；视频生成 UNKNOWN。

这些场景是后续合同与验收的输入，尚无本设计的运行通过声明。首个增量聚焦一份合成原稿、一个新解析流程、可采纳草案及恢复证据，再扩展专业能力。

## 11. 文本链实施合同（2026-09-08）

本节细化已接受的文本 MVP。依据为原始规范 `01_AgentHarness_Skill与创作工作流_统一设计稿.md` 的 5–14、18、21 节，以及画布规范的 12–15 节；原件现在位于 Obsidian 的 `Lanverse短剧制作平台/平台设计规范`。原件中的示例、外部 Skill 正文及工具调用示意都是参考资料，不构成用户授权。生产输入仍是 Go 接管后的固定 SourceEdition。

### 11.1 四个可执行任务与阶段屏障

先实现一个独立的 `text-storyboard` 专业包，按任务仅装载对应 reference；保留旧 `build-storygraph` 的文件和摘要。四个任务聚合有共同上下文的能力，不把 42 个目录当完成指标。

| 任务 | 实际专业能力 | 输入与输出 | 下游屏障 |
| --- | --- | --- | --- |
| map_manuscript | inspect-manuscript、divide-episodes | 完整源块 → 分集范围、非正文分类、边界问题 | 分集边界审阅 |
| analyze_episode | summarize-episode、segment-scenes、mark-story-beats、extract-dialogue-cues、collect-cast/places/props | 一集完整原文 → 梗概、场次、节拍、逐字对白、类型化提及、来源覆盖账 | 全集收敛、剧稿结构审阅 |
| build_world | resolve-entity-aliases、map-story-relations、trace-continuity、audit-story-facts、compile-asset-needs | 全稿场次事实 → 身份归并提案、关系、事件账、资产需求 | WorldBook 审阅；未采纳不成为正式总册 |
| direct_scene | analyze-scene-intent、arrange-blocking、design-shot-coverage、write-shot-briefs、time-dialogue-and-action、review-screen-direction、plan-frame-board、audit-scene-coverage | 单场、来源、披露过滤后的设定 → SceneBreakdown、镜头、声音映射、时长区间、文字画板与覆盖检查 | 分镜审阅与 Go 正式采纳 |

这四个入口只产生草案，不授予审批权。Harness 单次只运行一个任务，不在进程内等待人审或自动串过屏障。可信编排先持久保存输出再等待平台回执；验收脚本允许按顺序验证整条候选链，但该行为不能冒充已批准生产流程。

### 11.2 原文、证据与确定性检查

SourceEdition 的“原输入”指 Go 已正式接受的 `NormalizedText`：换行固定为 LF，其余 Unicode code point 序列保持不变，摘要使用对应 `NormalizedHash`。原始上传文件独立保留。Python 收到该固定表示后不再 trim、NFC 或改换行。块以带换行的物理行划分，连续编号和半开字符区间由代码生成；空行也在覆盖账中。源摘要按该表示的 UTF-8 字节计算。模型引用块号及逐字 quote，代码验证匹配后补出绝对偏移及片段摘要；重复短语必须扩大引用或显式指定从零开始的 occurrence，不能自动选第一次。重复对白使用 occurrence，不能把上下文扩进台词正文。

分集和分场通过闭区间块号映射，代码验证所有源块恰好归属一次；非正文、作者注释、未解决片段必须有分类和理由。显式集标题保留原集号，目录中的同名标题单独分类。无标题只能 propose，禁止在 preserve 模式造出集号或自行改编。缺号、重复号、目录/正文疑点进入审阅问题，不补造原文。

场内节拍、对白和提及使用范围内临时键。对白引用与原文逐字一致；未知说话人合法。人物台词属于 claim，不因为它有来源就当客观 fact。实体只能引用已存在、类型一致的提及；每条提及都要归入候选实体或显式 unresolved。正式 UUID 仍由 Go 采纳生成。

连续性保存带时间分支的事件，不维护模型可覆盖的“全剧当前状态”。关系和事件保留事实/推断/提案及叙述/陈述/未知依据；没有证据的推断必须成为 Issue。事件至少有一条本场证据，额外跨场证据只允许复用实体提案已登记的身份归并证据。当前导演上下文保守地只装入本场提及和本场状态证据，隐藏全局 identity key/label；跨场状态需待已审阅披露映射接通。presence 区分画内、画外、仅被提及和未知，分镜不能把仅被提及的人物放进 visible_mentions 或 blocking。每个必拍 Beat、对白和实际可见细节必须被 Shot 引用，每镜有叙事目的、调度、景别/运动、方向、时长上下界及文字 Panel；映射成功只证明结构覆盖，不证明镜头艺术质量。

### 11.3 执行、上下文和失败恢复

任务固定 source hash、上游草案内容、scope、Skill release hash、时限与输出上限。装配器保存实际块列表、上游摘要与遗漏记录。每次只调用一次受限 Codex，不通过随机语义重试消除 unresolved。超出上下文上限必须明确报 context_insufficient，不能静默截断或先压缩全稿。长稿分块归并的第二阶段扩展须保留重叠去重和来源覆盖证据，未实现前不声称无限长稿可用。

推理子进程只继承本地 Codex 认证所需的明确环境白名单，不继承数据库、平台授权和供应商配置。禁用工具、网络搜索与插件；取消或超时必须终止并等待子进程。输出和诊断受字节上限约束；使用量缺失标 unknown。执行结果绑定任务、输入和 release 的摘要，不能跨源版本复用。

Temporal 生产串接沿用 0013 的命令，不引入第二个调度器：读取 Go 冻结源 → 分集草案/平台门 → 逐集解析/平台门 → 全局设定/平台门 → 选定场导演/平台门。逻辑步骤键为 `stage/scope`，Activity 保存草案、OutputBinding 和 Outbox 的事务提交后才返回引用。每个门只接受匹配 run、源版本、候选摘要与正式 Owner Effect 的平台回执；工作流信号只是唤醒，不是授权。失败恢复使用同一步骤输入，复用已保存结果；更换源或改编范围创建新修订。

尚未完成 Go 源读取桥、Agent 草案持久化/预算及平台审阅回执接线时，不在 `lanverse.creation.text-storyboard.production` 上启用自动候选链，也不把 `/readyz` 当作创作工作流就绪证明。

### 11.4 开源参考核验与复用边界

2026-09-08 通过 GitHub 连接核验以下固定提交。只参考组织和评测方法，未复制外部实现、提示词或运行脚本；本包的影视规则按本项目设计编写。

| 一手来源 | 固定提交 / 许可证据 | 采用与不采用 |
| --- | --- | --- |
| [Agent Skills specification](https://github.com/agentskills/agentskills/blob/69ef37e9424c0a7ea9dd2293b559e43ec8176379/docs/specification.mdx) | `69ef37e9424c0a7ea9dd2293b559e43ec8176379`；根 LICENSE 为 Apache-2.0 | SKILL.md + 按需 references；执行授权、Schema、发布摘要仍由 Lanverse 实现 |
| [Anthropic skill-creator](https://github.com/anthropics/skills/blob/41bbe19d1a1a7eaab5e7bb9050a417e5c6cffc8f/skills/skill-creator/SKILL.md) | `41bbe19d1a1a7eaab5e7bb9050a417e5c6cffc8f`；该目录 LICENSE.txt 为 Apache-2.0 | 正反例、断言、质量和用量观测、对照评测；不让生产运行自动改写/安装 Skill |
| [LibTV Skill](https://github.com/libtv-labs/libtv-skills/blob/c609246c1eca69f6bc129bcbb5d64c36734e4a4a/skills/libtv-skill/SKILL.md) | `c609246c1eca69f6bc129bcbb5d64c36734e4a4a`；根 LICENSE 为 MIT | 参考稳定会话/产物引用与进展查询；公开内容是远程服务客户端，不能据此宣称复用了开源剧本解析引擎；不向该服务发送用户原稿 |

### 11.5 验证口径

黄金数据包含钥匙交接、蒙面身份后揭示、裂纹手表和闪回、手机文字及未知录音内容。代码测试覆盖中文/表情偏移、目录与缺集、越界证据、重复覆盖、实体类型冲突、台词误改、未来信息泄露、漏拍对白/节拍、Skill 篡改、取消和上下文耗尽。真实 Codex 评测单独记录实际结果，不把替身模型测试当真实创作质量，也不把本地一份合成剧本成功当全产品验收。

### 11.6 持久执行与源读取的落地约束（2026-09-08）

命令接收库采用追加式、带校验和的迁移；已发布的 command-acceptance 迁移不改写。可信层冻结每次运行的 Skill release 与调用次数上限，固定步骤身份为 run + stage + scope；相同身份改变输入必须冲突。领取步骤在同一数据库事务中先占用一次额度、增加 fence，再发送 Harness 请求。活跃尝试不会被重复请求抢占；超时、断连或进程消失将占用保留为 unknown，不自动重新调用。保存结果必须仍持有未过期 fence，并核验任务/源/Skill/候选摘要。草案、稳定 OutputBinding 与 result_ready Outbox 在同一事务提交；响应丢失后读取原结果，不再调用模型。这里只记推理调用额度，不能冒充实际 token 或货币费用对账。

共享的纯文本合同放在 app/text_contract，包含 Schema、来源索引与确定性检查；可信应用镜像只带合同，不带专业提示词、Codex 或 Harness。任务输入由可信调用方从冻结命令和已有上游草案装配；执行端保存前再次验证来源覆盖和引用。缺少正式门回执时不能通过对执行端直接传递上游 JSON 来越过人工门。

固定源桥属于 Go Creation 应用：机器请求仅携带 run_id 与 command payload_hash，原始 actor、token version、project 和源版本从已存命令读取。每次请求重新检查当前写权限，Script Owner 验证固定版本、全文摘要及 Unicode code point 索引；不跟随当前 head。请求用独立 platform audience 的 HMAC 绑定 HTTP 方法、原始路径、请求体及 60 秒有效期；不接受重定向或调用者指定的源 URL。源输出上限 200 万字符，保持原始码点序列。已有命令接收 ready 状态仍只代表接收能力；完整生产 Workflow 须在四道门与正式 Owner Effect 桥接均完成后启用。
