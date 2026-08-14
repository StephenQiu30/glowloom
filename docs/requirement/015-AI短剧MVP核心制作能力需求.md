# REQ-015 AI 短剧 MVP 核心制作能力需求

- 状态：proposed（整份文档继续保留演进边界；截至 2026-08-14，MVP-A 11/11 PT 已由 Acceptance 028～038 接受）
- 输入：[平台总体需求](./001-平台总体需求概述.md)、[项目模块需求](./003-项目模块需求.md)、[剧本模块需求](./arrived/006-剧本模块需求.md)、[资产模块需求](./arrived/007-资产模块需求.md)、[分镜模块需求](./arrived/008-分镜模块需求.md)
- 设计：[DES-011 核心生产模块缺口与目标设计](../design/011-AI短剧核心生产模块缺口与目标设计.md)、[DES-012 MVP 核心模块拆分与实施范围](../design/012-AI短剧MVP核心模块拆分与实施范围.md)
- 下游：[PRD-012 AI 短剧 MVP 核心制作产品任务](../prd/012-AI短剧MVP核心制作产品任务.md)

## 1. 需求结论

当前 MVP 的用户结果固定为：创作者能把一份 3–5 集的 UTF-8 中文整剧原稿导入项目，确认分集和单集改写，建立角色/场景/道具的剧情状态，生成并人工接受分镜草案，证明所有必拍内容已有镜头去向，最后导出可复现的分镜制作包。

本需求只增加现有 S0～S3 纵向闭环缺少的能力，不重新定义或重新验收已经接受的 Project、Episode、ScriptVersion、Asset/AssetVersion、Shot/ShotSpecVersion、媒体、授权和任务事实。AI 结果始终是候选；没有人工确认时，不得改变正式 Episode、ScriptVersion、Asset、Shot 或 current 指针。

MVP-A 是本需求的当前范围。MVP-B 的真实图片生成继续复用生产模块与 Provider 控制面，必须在真实账号门禁关闭后单独接受；视频、时间线和成片不属于本需求。

## 2. 业务实体需求

| 编号 | 实体 | 需求 |
| --- | --- | --- |
| MVP-ENT-001 | ScriptDocument | 必须表示项目级整部剧本的稳定身份，属于唯一 Workspace/Project；不得把整剧正文直接塞入 Project 可变字段。 |
| MVP-ENT-002 | DocumentRevision | 必须是 ScriptDocument 的不可变原稿修订，固定 UTF-8 正文或受控文档媒体、内容哈希、字符数、来源和创建者。 |
| MVP-ENT-003 | NarrativeBlock | 必须表示格式体检后可用于分集的稳定块，固定类型、顺序和原稿字符范围；块集合必须能解释全部未排除正文。 |
| MVP-ENT-004 | EpisodePlan | 必须固定一个 DocumentRevision、分集策略、目标时长、边界、预计时长、理由、置信度和 revision；确认前不得创建正式 Episode。 |
| MVP-ENT-005 | ImportCommit | 必须记录一次确认方案向 Episode、ScriptSource 和原始 ScriptVersion 的幂等批量物化结果，并能证明整批成功或整批失败。 |
| MVP-ENT-006 | AdaptationRun | 必须固定输入 ScriptVersion、改写约束、Prompt/模型/Schema 快照、状态和输出候选；不得覆盖原稿或自动发布。 |
| MVP-ENT-007 | NarrativeUnitVersion | 必须以稳定 NarrativeUnit 身份和不可变版本表示场标题、动作、对白或旁白，固定来源范围、顺序和当前 ScriptVersion。 |
| MVP-ENT-008 | AssetState | 必须在稳定 Asset 下表示剧情状态，固定名称、描述、启用状态、revision 和 current AssetVersion；不得用名称代替身份。 |
| MVP-ENT-009 | StoryboardDraftBatch | 必须固定 ScriptVersion、NarrativeUnitVersion、AssetState/Version 和生成约束，保存 AI 镜头草案及人工决定；Apply 前不写正式 Shot。 |
| MVP-ENT-010 | ShotNarrativeReference | 必须固定一个 ShotSpecVersion 与一个 NarrativeUnitVersion 的多对多覆盖边，保存 channel、role、segment、origin 和创建者；不得用镜号或字符位置作长期外键。 |
| MVP-ENT-011 | CoverageDecision | 必须只追加记录批准省略、撤销省略或批准创作性镜头的 actor、reason、evidence 和 expected coverage hash。 |
| MVP-ENT-012 | CoverageReport | 必须由当前 NarrativeUnitVersion、ShotSpecVersion、ShotNarrativeReference 和 CoverageDecision 派生，固定输入哈希并给出 covered、approved_omitted、uncovered、orphan 和 stale。 |
| MVP-ENT-013 | StoryboardExportManifest | 必须固定一次导出的 Script/Narrative/AssetState/AssetVersion/ShotSpec/Coverage 和文件引用；后续 current 变化不得篡改历史 Manifest。 |

## 3. 功能性需求

| 编号 | 需求 |
| --- | --- |
| MVP-FR-001 | 创作者必须能在 Project 中粘贴整剧文本或上传 UTF-8 `.txt/.md`；非法编码、空文本、超限、缺号、重复集号和空集必须在物化前给出位置与下一动作。 |
| MVP-FR-002 | 合法显式集标记必须得到确定性集数和边界；同一 DocumentRevision 重复分析必须得到相同结果。 |
| MVP-FR-003 | 无集标记时，系统必须按目标单集时长给出一个可解释的分集建议；建议必须包含边界证据、预计时长和置信度，并明确是 AI 候选。 |
| MVP-FR-004 | 创作者必须能移动边界、拆集、合集、修改标题并确认 EpisodePlan；确认后才能幂等批量物化 Episode 和原始 ScriptVersion。 |
| MVP-FR-005 | 创作者必须能为一个已发布 ScriptVersion 填写改写目标并生成一个改写候选，查看原文/候选差异、编辑候选并显式发布新版本。 |
| MVP-FR-006 | 发布 ScriptVersion 后，系统必须形成场标题、动作、对白和旁白 NarrativeUnitVersion；低置信或非法结构必须允许人工修正。 |
| MVP-FR-007 | 新 ScriptVersion 成为 current 时，引用旧版本的资产提取、分镜覆盖和导出必须变为 stale；MVP 不自动把旧分镜静默迁移到新剧本。 |
| MVP-FR-008 | 创作者必须能在角色、场景和道具 Asset 下创建/编辑/禁用 AssetState，记录其 Episode/NarrativeUnit 出现证据，并为状态选择固定 AssetVersion。 |
| MVP-FR-009 | 资产改名、禁用、状态 current version 切换前必须展示受影响 Episode、Shot、Prompt 和非终态任务；应用后不得删除历史引用或漂移旧 ShotSpec。 |
| MVP-FR-010 | 导演必须能基于固定剧本、叙事单元和资产状态生成 StoryboardDraftBatch，逐镜接受/修改/忽略或整批 Apply；AI 运行不得直接写正式 Shot。 |
| MVP-FR-011 | 导演必须能查看剧本与镜头的双向多对多关系和 CoverageReport；全部必拍单元已覆盖或批准省略、orphan=0、stale=0 后才允许 Episode 分镜 ready。 |
| MVP-FR-012 | 创作者必须能导出固定版本的分镜制作包，至少包含镜号、时长、叙事来源、画面、对白、资产状态/版本、Prompt、readiness、JSON、CSV/HTML 和 Manifest。 |

## 4. 接口需求

| 编号 | 接口能力 | 输入 | 输出与错误 |
| --- | --- | --- | --- |
| MVP-IF-001 | 整剧导入与分析 | ActorContext、Project、text 或 document MediaVersion、幂等键 | ScriptDocument/Revision、分析 Task、格式问题和 next actions；正文不进入日志/消息。 |
| MVP-IF-002 | 分集计划查询与命令 | DocumentRevision、策略/目标时长、边界操作、expected revision | EpisodePlan、预计时长/理由/置信度；边界非法、并发冲突和非当前 revision 使用稳定错误。 |
| MVP-IF-003 | 批量物化与发布 | confirmed EpisodePlan、expected Project/Episode state、幂等键 | ImportCommit 与 Episode/ScriptVersion 引用；任一步失败时零部分 current 更新。 |
| MVP-IF-004 | 改写候选与发布 | ScriptVersion、改写约束、expected current、幂等键 | AdaptationRun、diff、候选和发布结果；结果不明不得盲重发或覆盖原文。 |
| MVP-IF-005 | NarrativeUnit 查询与修正 | ScriptVersion、单元版本、人工修正、expected revision | 有序 NarrativeUnitVersion、来源范围和影响摘要；跨版本/跨空间引用拒绝。 |
| MVP-IF-006 | 资产状态与影响命令 | Asset、State、Occurrence、Version、expected revision/preflight hash | 状态矩阵、出现证据、影响清单和 readiness；跨资产版本或陈旧预检零写入。 |
| MVP-IF-007 | 分镜草案与覆盖命令 | Episode、固定输入版本、DraftDecision、expected order/spec/coverage hash | DraftBatch、Apply diff、正式 Shot 引用和 CoverageReport；未满足依赖时 fail closed。 |
| MVP-IF-008 | 分镜包导出 | Episode、固定 Script/Asset/Shot 版本、coverage hash、幂等键 | Export Manifest、JSON、CSV/HTML 媒体引用或阻断清单；不得读取会漂移的“当前主选”。 |

## 5. 状态与失效语义

- DocumentRevision、NarrativeBlock、NarrativeUnitVersion、ShotSpecVersion、Export Manifest 均不可变；所谓编辑必须创建新版本。
- EpisodePlan 使用 `draft → review_ready → confirmed → materialized`；旧计划可 `superseded`，不能覆盖已发布 ImportCommit。
- AdaptationRun 与 StoryboardDraftBatch 使用异步 Task 的合法状态；结果不明进入 `unknown/manual_attention`，确认无外部副作用前不得新建重复运行。
- AssetState 生命周期为 `enabled ↔ disabled`；Asset/AssetVersion 的 active/archived 和 readiness 仍使用现有模块事实，状态 current version 切换只影响未来入口。
- Coverage 是派生事实。任一 ScriptVersion、NarrativeUnitVersion、ShotSpecVersion、AssetState/Version 或 CoverageDecision 变化时，旧 evaluation hash 必须失效。
- `@图片N/@视频N/@音频N` 只是 Prompt 编译显示别名，不得成为数据库身份或跨版本外键。

## 6. 非功能性需求

| 编号 | 类别 | 需求 |
| --- | --- | --- |
| MVP-NFR-001 | 数据完整性 | NarrativeBlock 切片和 EpisodePlan 正文范围必须 gap=0、overlap=0；split/merge 后对白、动作和 required coverage 必须守恒，禁止截断。 |
| MVP-NFR-002 | 幂等与并发 | 导入、分析、确认、物化、发布、状态切换、草案 Apply、覆盖决定和导出必须同时使用幂等键及 expected revision/current/hash。 |
| MVP-NFR-003 | 租户与权限 | 所有新增事实必须按 Workspace 隔离并使用复合引用；owner/editor 可按现有能力写，viewer 只读，跨空间 ID 不泄露存在性。 |
| MVP-NFR-004 | 不可变与可追溯 | 任一正式 Shot、资产状态版本和导出必须能回溯精确 Document/Script/Narrative/Asset/Shot 版本、actor、时间和来源决定。 |
| MVP-NFR-005 | Fail closed | 格式、依赖、权利、版本、覆盖或导出依赖 unavailable/stale/blocked 时不得默认为 ready，也不得用前端本地推导绕过。 |
| MVP-NFR-006 | 异步恢复 | AI 分集、改写和分镜草案必须复用 Task/Outbox/Worker；刷新、重复消息和 Worker 重启不得重复外部调用或正式 Apply。 |
| MVP-NFR-007 | 性能与容量 | MVP 上限为每项目 10 集、整剧 100,000 code points、每集 20,000 字和单 Episode 120 active Shot；36 镜批量查询 P95 ≤800 ms、120 镜 P95 ≤2 s。 |
| MVP-NFR-008 | 正文与媒体安全 | 剧本文本、Prompt、文档 object key、预签名 URL 和权利材料不得进入普通日志、metric label、消息正文或未授权响应。 |
| MVP-NFR-009 | 数据变更 | 新表和约束必须使用正式 schema migration，在空库、当前 schema 快照和含黄金样本的旧库副本上验证前滚、校验与恢复；不得用 `create_all` 冒充升级。 |
| MVP-NFR-010 | 可访问与契约 | 主要导入、边界编辑、diff、资产状态、分镜覆盖和导出路径必须键盘可完成；前端只消费 OpenAPI 生成 client 和服务端状态。 |

## 7. 失败路径

- 文件上传成功但格式分析失败时，保留安全的 DocumentRevision 和失败 Task；不得创建 Episode。
- EpisodePlan 并发变化、Project/Episode 状态变化或物化任一步失败时，ImportCommit 整批失败或冲突，不能留下半批 current。
- 改写输出无法通过 Schema、长度或事实约束时只形成失败/待人工候选，原稿和 current 不变。
- AssetState 指向另一 Asset 的版本、被禁用、媒体/权利不 ready 时，相关 Shot 和生成入口 blocked。
- StoryboardDraft Apply 发现人工 Shot/order/spec 已变化时，返回 diff 冲突且零正式写入。
- split/merge 无法完整分配对白、动作或覆盖引用时直接拒绝，不允许以前端空数组或截断结果继续。
- Coverage 依赖服务不可用时返回 unavailable；导出必须被阻断并列出 next actions。
- 导出文件写入失败时不得登记成功 Manifest；重复请求只能恢复或回读同一导出任务。

## 8. 非目标与条件范围

- 当前不支持 PDF/DOCX/OCR、超过 10 集的整季自动处理、Top-3 分集方案和跨项目资产库。
- 当前不支持复杂逐单元改写接受/拒绝、自动跨修订镜头迁移、导演字符区间映射和自动审美评分。
- 当前不支持真实视频、TTS、字幕、时间线、MP4、发行、备案、计费、商业套餐和企业协作。
- MVP-B 图片生成不在本 Requirement 的 PT 中重复建设；它必须复用 REQ-009/014 的 Provider、Request、Attempt、Candidate、Selection、MediaLineage 和费用事实。

## 9. 接受边界

本 Requirement 继续以 `proposed` 标记整份文档的演进边界；其中 MVP-A 的 11 个 PT 已由 Acceptance 028～038 逐项 accepted，产品负责人兼短剧制作人/QA 已接受黄金样本和固定版本分镜包。既有 S0～S3 事实保持 accepted；MVP-A accepted 不得外推为真实图片、视频、时间线或成片能力。
