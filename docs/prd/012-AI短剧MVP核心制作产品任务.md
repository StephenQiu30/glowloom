# PRD-012 AI 短剧 MVP 核心制作产品任务

- 状态：active（MVP-A 11/11 PT 已由 Acceptance 028～038 验收并完成产品签字；MVP-B 条件切片尚未激活）
- 日期：2026-08-13
- 输入：[REQ-015 AI 短剧 MVP 核心制作能力](../requirement/015-AI短剧MVP核心制作能力需求.md)、[DES-011 核心生产模块缺口与目标设计](../design/011-AI短剧核心生产模块缺口与目标设计.md)、[DES-012 MVP 核心模块拆分与实施范围](../design/012-AI短剧MVP核心模块拆分与实施范围.md)
- 既有事实：[PRD-008 创作生产模块任务](./008-创作生产模块PRD任务.md)中已接受的 S2/S3 PT
- 输出：11 个不重复既有能力、可独立验收的 `PT-*` 产品任务
- 下游：[PRD-010 追踪矩阵](./010-需求设计与产品任务追踪矩阵.md)、[PLAN-012 AI 短剧 MVP 核心制作执行计划](../plan/012-AI短剧MVP核心制作执行计划.md)

## 1. 产品结果与当前基线

MVP-A 完成后，创作者能在一个 Project 内完成：

> 整剧导入 → 格式体检 → 分集确认 → 批量建立 Episode → 单集改写/发布 → 稳定叙事单元 → 资产剧情状态 → AI 分镜草案 → 多对多覆盖确认 → 固定版本分镜包导出。

当前实现不是从零开始。以下能力保持已接受且不重新计价：

- Project/Episode 身份、顺序、目标时长和生命周期；
- 单集 ScriptSource/ScriptVersion、DeepSeek 结构提取、候选决议和 confirmed Scene/Dialogue；
- 六类 Asset、不可变 AssetVersion、媒体/授权/readiness 和镜头升级预检；
- Shot、不可变 ShotSpecVersion、顺序 CAS、固定 AssetVersion 引用、手工拆合和 readiness；
- Task/Outbox/Worker、MinIO、RabbitMQ、PostgreSQL 和基础审计。

本 PRD 只补齐整剧入口、改写、稳定叙事单元、资产状态、AI 分镜草案、覆盖证明和分镜包。任何 PT accepted 都不能外推为真实图片、视频或成片能力。

## 2. 用户与黄金路径

| 角色 | 当前 MVP 能力 | 不允许 |
| --- | --- | --- |
| owner/editor | 导入自有剧本、编辑分集、发起改写、确认资产状态、编辑/应用分镜和导出 | AI 自动发布；绕过 stale/coverage/rights；覆盖历史版本 |
| viewer | 查看已授权的整剧、分集、版本、资产状态、覆盖和历史导出 | 写入、应用候选、切换 current 或获取未授权正文/媒体 |
| 产品负责人/短剧制作人 | 冻结黄金样本、必拍规则、目标时长和 PT 接受结论 | 用营销案例或单次生成代替可重复验收 |

黄金路径固定为一部 3–5 集 UTF-8 中文剧本，项目上限 10 集；选择一集 60–120 秒、12–24 镜、9:16 做联合验收。显式集标记走确定性拆分；无标记文本只给一个 AI 建议并由人确认。

## 3. 产品任务

| ID | 用户结果 | 最小交付边界 | 可执行验收 | 依赖与追踪 |
| --- | --- | --- | --- | --- |
| PT-DAT-004 | 团队可以保留已有数据演进新模型 | Alembic 基线、当前 schema 校验、前滚 revision、备份/恢复说明和旧库副本演练；新环境启动改为 upgrade head | 空库、当前 schema 快照、含黄金样本旧库三条路径通过；结构漂移拒绝；`create_all` 不再冒充非空升级；前滚失败恢复原库 | MVP-NFR-009；DES-012 §10；不替代 PT-DAT-001～003 |
| PT-SCR-006 | 创作者把整部剧本保存并在物化前看懂格式问题 | ScriptDocument、DocumentRevision、NarrativeBlock；text 与 UTF-8 txt/md；确定性集标记解析；格式问题和异步状态 | 显式标记准确率 100%；非法输入有位置/next action；正文 accounted=100%、gap=0、overlap=0；重复分析一致 | MVP-ENT-001～003；MVP-FR-001/002；MVP-IF-001 |
| PT-SCR-007 | 创作者确认“做多少集”并一次性建立各集 | 一个 AI 建议或确定性 EpisodePlan；边界移动/拆合；confirm；ImportCommit 全量物化 Episode/Source/原始 Version | 人工可修边界；预计时长和理由可见；同键重放不重复；任一冲突/失败零半批 Episode/current；原文切片并集守恒 | MVP-ENT-004/005；MVP-FR-003/004；MVP-IF-002/003；PT-SCR-006、PT-PRJ-002 |
| PT-SCR-008 | 创作者在不破坏原稿的情况下改写一集 | AdaptationRun 固定输入/Prompt/模型/Schema；一个候选；原文/候选 diff；编辑并发布新 ScriptVersion | AI 永不覆盖原稿；失败/取消 current 不变；发布使用 expected current；重复运行/发布幂等；正文不进日志/消息 | MVP-ENT-006；MVP-FR-005；MVP-IF-004；PT-SCR-001、PT-PROD-001 |
| PT-SCR-009 | 下游可以稳定引用剧本内容并在改稿后正确失效 | NarrativeUnit/Version；场标题/动作/对白/旁白；来源范围；人工修正；current 切换影响摘要 | 单元顺序/范围合法；必拍单元有稳定 ID；current 切换使旧覆盖/导出 stale；跨版本/跨空间引用拒绝；不自动迁移旧镜头 | MVP-ENT-007；MVP-FR-006/007；MVP-IF-005；PT-SCR-008 |
| PT-AST-006 | 创作者把同一资产的不同剧情状态分开治理 | AssetState、OccurrenceDecision、state current version；角色/场景/道具状态矩阵和 readiness | 状态回溯 Episode/NarrativeUnit；状态只能引用本 Asset Version；current 切换不改旧 ShotSpec；禁用立即阻断未来用途；同名不自动合并 | MVP-ENT-008；MVP-FR-008；MVP-IF-006；PT-SCR-009、PT-AST-002/003 |
| PT-AST-007 | 创作者改名、禁用或换状态版本前看到完整影响 | rename/disable/version preflight+apply；固定影响 hash；Episode/Shot/Prompt/Task 汇总；旧名 alias | 陈旧 hash 零写入；改名零 FK 变化；disable 零历史删除；旧镜头仍固定旧版本；批量影响无按镜 N+1 | MVP-FR-009；MVP-IF-006；PT-AST-006、PT-SBD-006 |
| PT-SBD-007 | 导演拆镜/合镜时不会静默丢对白、动作或覆盖 | 先修现有 split/merge；两端 dialogue/action 分配守恒；拒绝跨 Scene merge 和超上限结果；Transform 记录 omission | split 目标并集等于来源、交集符合规则；merge 不截断；跨 Scene、>8 beats/dialogues、>15 秒拒绝；旧前端默认不再制造空第二镜 | MVP-NFR-001；MVP-FR-011；既有 PT-SBD-004 增量，不重开其已接受范围 |
| PT-SBD-008 | 导演可以审核 AI 分镜方案而不破坏人工镜头 | StoryboardDraftBatch/候选/Decision；固定输入；逐镜修改/忽略；apply preflight/diff；幂等 CAS Apply | Run 对正式 Shot 写入 0；Apply 前显示新增/保留/变化；人工锁定镜头不覆盖；冲突零写入；12–24 镜黄金样本可完成人工应用 | MVP-ENT-009；MVP-FR-010；MVP-IF-007；PT-SCR-009、PT-AST-006、PT-SBD-007 |
| PT-SBD-009 | 导演能证明全部必拍内容都有镜头去向 | ShotNarrativeReference 多对多；CoverageDecision；CoverageReport；双向定位；readiness/ProductionSnapshot 接入 | required 全部 covered 或 approved_omitted；orphan=0、stale=0；依赖 unavailable fail closed；修改剧本/资产/Spec 后旧 hash 失效；36/120 镜性能继续通过 | MVP-ENT-010–012；MVP-FR-011；MVP-IF-007；PT-SCR-009、PT-AST-006、PT-SBD-008 |
| PT-SBD-010 | 制作人能拿走一个不会随 current 漂移的分镜包 | Export Manifest 固定 Script/Narrative/Asset/Shot/Coverage；JSON + CSV/HTML；异步导出和历史下载 | 导出前重新计算 readiness/coverage；失败给 blocker；同键同输入回读；Manifest 可反查全部版本；后续改稿不篡改旧包；文件缺失不登记成功 | MVP-ENT-013；MVP-FR-012；MVP-IF-008；PT-SCR-007、PT-AST-007、PT-SBD-009、PT-MED-002 |

## 4. PT 接受顺序

~~~mermaid
flowchart TD
    D["PT-DAT-004 migration"] --> I["PT-SCR-006 整剧导入"]
    D --> R["PT-SCR-008 改写候选"]
    I --> P["PT-SCR-007 分集物化"]
    R --> N["PT-SCR-009 NarrativeUnit"]
    N --> A["PT-AST-006 资产状态"]
    A --> X["PT-AST-007 资产影响"]
    N --> S["PT-SBD-008 分镜草案"]
    F["PT-SBD-007 拆合守恒"] --> S
    A --> S
    S --> C["PT-SBD-009 Coverage"]
    X --> C
    P --> E["PT-SBD-010 分镜包"]
    C --> E
~~~

PT-SCR-006/007 与 PT-SCR-008/009 可以在数据库基线后分两条流推进；PT-AST-006/007 与 PT-SBD-007/008 可在 NarrativeUnit 最小契约冻结后并行；PT-SBD-009 是联合收口点，PT-SBD-010 是 MVP-A 唯一最终接受出口。

## 5. 横向验收矩阵

| 场景 | 必须覆盖的 PT | 退出条件 |
| --- | --- | --- |
| 确定性整剧 | SCR-006/007 | 5 集标记、缺号、重复、空集、全角/中文数字、Emoji；边界 100% 可重复 |
| 无标记建议 | SCR-007 | 一个建议有理由/时长/置信度；用户可改；不自动确认 |
| 改写与失效 | SCR-008/009、SBD-009/010 | 原稿不变；新 current 后旧 Coverage/Export stale；历史仍可读 |
| 状态资产 | AST-006/007 | 常服/受伤、日景/夜景、完好/破损；固定版本和影响可解释 |
| 分镜守恒 | SBD-007/009 | 正反打、反应镜头、蒙太奇、拆/合/重排后 required coverage 不丢失 |
| AI 草案 | SBD-008 | 候选不写正式 Shot；Apply diff/CAS/幂等；人工锁定不覆盖 |
| 导出恢复 | DAT-004、SBD-010 | 旧库升级、导出失败、重试、历史 Manifest 和字节完整性 |
| 权限与安全 | 全部 | Workspace 复合引用、owner/editor/viewer、正文/Prompt/object key/URL 脱敏 |

## 6. MVP-A 完成定义

- 11 个 PT 均有独立真实 Acceptance，且没有用既有 S0～S3 证据提前代替新增能力；
- 一部 3–5 集黄金剧从整剧导入到分镜包导出完整执行两次，第二次包含并发、Worker 重启和至少一个下游失效场景；
- 所有必拍 NarrativeUnit `covered` 或 `approved_omitted`，未解释遗漏=0、orphan=0、stale=0；
- 导出 Manifest 能反查固定 Document/Script/Narrative/AssetState/AssetVersion/ShotSpec/Coverage；
- schema migration 在空库、当前 schema 和含样本旧库副本通过；
- Ruff、Pyright、Pytest、前端 lint/typecheck/test/build、OpenAPI 漂移、固定性能和浏览器主路径全绿；
- 没有 P0/P1 数据丢失、越权、幂等、并发或静默漂移缺陷。

## 7. MVP-B 条件切片

MVP-B 不在本 PRD 创建第二套 Provider/Candidate 任务。进入真实图片生成前必须：

1. MVP-A accepted；
2. D-004 对目标图片模型的账号、权限、参数、额度和真实账单证据关闭；
3. PLAN-011 的 Provider Connection/Binding 相关 PT 达到目标图片用途所需状态；
4. 重新修订 PRD-008/PLAN-000，把现有同时耦合 Image/Video 的 S4 拆成“图片先接受、视频后接受”，而不是让视频阻塞 MVP-B；
5. 复用唯一 PromptRevision、GenerationRequest、Task/Attempt、Candidate/Selection、MediaLineage 和 CostEntry；AssetState 只通过类型化 target 和 promotion adapter 接入。

在上述变更接受前，PLAN-012 只允许做不调用 Provider 的接口兼容性检查，不允许创建假图片成功、第二套 Candidate 表或资产模块私有生成任务。

## 8. 非目标

- 不重开或重写已接受的 PT-SCR-001～005、PT-AST-001～005 和 PT-SBD-001～006；新增任务只追加缺口。
- 不包含 Top-3 分集、50 集规模、PDF/DOCX/OCR、跨项目资产库、导演标注自动映射和复杂跨修订迁移。
- 不包含视频、TTS、字幕、时间线、MP4、商业计费、客户协作、备案发行或公开上线。

## 9. 状态与接受规则

产品负责人已明确启动并接受 MVP-A。PT-DAT-004、PT-SCR-006～009、PT-AST-006/007、PT-SBD-007～010 已由 Acceptance 028～038 分别 accepted；2026-08-14，产品负责人兼短剧制作人/QA 同步接受原创黄金样本的主观内容质量。MVP-A accepted 不等于发布型 MVP 已完成，真实图片、视频、时间线与成片仍以各自 PRD、Plan 和独立 Acceptance 为准。
