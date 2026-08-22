# 切片 A：手工事实主线 PRD

> PRD ID：PRD-A
> 状态：proposed
> 日期：2026-08-22
> 上游：[PRD-000](./000-首轮试点产品基线PRD.md)、[系统 Requirement](../requirement/000-AI视频生产平台目标需求总览.md)
> 需求范围：M01、M02、M03、M04、M05；M07/M08/M09/M11/M14 的最小手工与 Fixture 范围
> 设计依据：[系统架构](../design/000-AI视频生产平台目标系统架构设计.md)、[接口工作流](../design/003-AI视频生产平台接口工作流与功能实现设计.md)、[剧本基础分析专题](../design/004-AI视频生产平台剧本基础分析与人物拆解详细设计.md)
> 实施与验收：[PLAN-A](../plan/001-切片A手工事实主线实施计划.md)、[Acceptance](../acceptance/README.md)

## 1. 产品决策

切片 A 必须在 Agent 和真实生成 Provider 都关闭时完成可用闭环。用户从一份真实整本剧本出发，确认剧集、每集叙事、全剧人物与生产需求，再手工建立镜头、取得 Fixture Candidate 并播放顺序预演。该切片验证的是事实、版本、权限和恢复语义；后续自动化不得掩盖 A 的缺陷。

## 2. 用户问题与目标角色

整本剧本包含跨集别名、同名人物、地点复用、服装变化和分散道具信息。主创需要知道“分成了哪些集、每集有哪些场景、人物在哪几集出现、需要准备哪些生产资产”，并能逐项回到原文修正。制片人需要确认范围、资源上限和阻塞；操作员需要在解析或 Worker 失败后继续接管。

| 角色 | 在 A 中的主要任务 |
| --- | --- |
| 主创/导演 | 批准拆集与叙事、决议人物/资产、创建镜头、选择 Fixture Candidate |
| 制片人 | 创建项目/Brief、确认项目范围与硬上限、查看阻塞和预演 |
| 制作操作员 | 导入来源、处理结构缺口、恢复任务、准备矩阵与镜头表 |
| 权利负责人 | 声明来源权利，阻止未声明内容进入后续外发 |

## 3. 用户结果与成功指标

| ID | 用户结果 |
| --- | --- |
| PRD-A-O-001 | 整本来源形成无重叠、无缺口或具名忽略的批准剧集 Manifest，并原子物化稳定 ContentUnit。 |
| PRD-A-O-002 | 每集 Scene、Beat、对白、动作与人物/地点/道具/服装 Mention 可校对并回跳原文。 |
| PRD-A-O-003 | 全剧生产清单与人物×剧集矩阵明确区分已决、未决、冲突、未出现和未分析。 |
| PRD-A-O-004 | 用户可以手工建立不少于 10 个可追溯镜头并看到覆盖缺口。 |
| PRD-A-O-005 | Fixture Candidate、Selection 与 Animatic 贯通真实 Operation/Job/Attempt/媒体边界。 |
| PRD-A-O-006 | 失败、重启、重复提交和投影丢失后可以恢复而不重复事实。 |

| 指标 | 口径 |
| --- | --- |
| 来源与拆集覆盖 | 非空来源字符/Anchor 被某集覆盖或具名忽略的比例；批准目标 100%。 |
| 叙事与知识完整度 | failed scopes、unassigned、overlap、unassessed 分别统计；正式预检目标均为 0。 |
| 人工修正 | 拆集、Scene/Beat/Mention、身份和镜头的新增/修改/删除分别统计。 |
| 恢复正确性 | 重复 Command、消息和 Worker 重启产生的重复事实数，目标 0。 |
| 检索正确性 | 金标查询的结果集合、Anchor 正确率、跨 Workspace/未发布命中，越权目标 0。 |

## 4. P0 用户旅程

```text
创建 Workspace/项目/Brief
  → 上传并保全整本 DOCX/Markdown/TXT
  → 检查 ParseReport、来源覆盖和剧集候选
  → split/merge/move/rename/ignore 并批准 Manifest
  → 原子物化 ContentUnit 与 OrderRevision
  → 校对各集 Scene/Beat/Dialogue/Action/Mention
  → 批准 NarrativeRevision
  → 检索 approved 剧本并回跳 SourceAnchor
  → 决议人物身份、地点、道具、服装与生产需求
  → 确认全剧 Inventory 和人物×剧集矩阵
  → 手工建立/重排不少于 10 个 Shot
  → 通过 Fixture Plan/Job 取得 Candidate
  → 人工 Selection 并生成顺序 Animatic
```

## 5. 功能范围

| ID | P0 功能要求 | 关键 Requirement |
| --- | --- | --- |
| PRD-A-FR-001 | 创建单 Workspace、成员职责、Project 与版本化 Brief，并隔离所有查询、写入、Worker 和媒体访问。 | IAM-FR-001/003—010；PRJ-FR-001—007 |
| PRD-A-FR-002 | 接纳整本来源，保存不可变原件、hash、ParseReport、失败范围和 SourceAnchor。 | NAR-FR-001—005；GOV-FR-001 |
| PRD-A-FR-003 | 创建、修订和批准 EpisodeBreakdown；支持 split/merge/move/rename/reorder/ignore，并一次物化 ContentUnit。 | NAR-FR-013；PRJ-FR-011 |
| PRD-A-FR-004 | 在不依赖 Agent 的情况下创建/编辑/删除每集结构与 ProductionElementMention，批准 NarrativeRevision。 | NAR-FR-006—014 |
| PRD-A-FR-005 | 对 approved Narrative 提供正文、短语和结构筛选、高亮与 SourceAnchor 回跳；索引 stale/unavailable 与零结果分离。 | NAR-FR-015；NAR-NFR-008 |
| PRD-A-FR-006 | 手工完成 MentionResolution、UnresolvedSubjectRevision、人物/地点/道具/服装事实和范围状态。 | KNO-FR-001—007/010/012/016 |
| PRD-A-FR-007 | 以不可变 CoverageSchema/Requirement/Decision 建立全剧生产清单、Readiness 与人物×剧集矩阵。 | KNO-FR-017；KNO-NFR-007 |
| PRD-A-FR-008 | 手工创建、拆分、合并、锁定和重排 Shot，绑定 Beat、实体状态与覆盖。 | SHT-FR-001—009/011—012 |
| PRD-A-FR-009 | 通过 M07 Fixture Plan、M08 Operation/Job/Attempt 和 M09 Candidate/Selection 形成明确标记的零外部用量预演。 | PLN-FR-001—003/006—009/013；EXE-FR-001—012；MED-FR-001—010 |
| PRD-A-FR-010 | 显示项目硬上限、来源权利、审计、错误恢复和下一动作。 | USG-FR-001—003/007/009；GOV-FR-001/006/007/010 |

NAR-FR-015 原为模块 P1；本 PRD 将“approved 私有剧本检索”提升为 A 的 P0 验收项，只提升这里定义的正文/短语/结构过滤、权限、回跳、重建和 stale 语义，不提前交付公共 ToC 搜索或向量检索。

## 6. 页面与关键决定

| 页面/工作台 | 用户必须看到 | 用户决定 |
| --- | --- | --- |
| 项目与 Brief | 当前版本、目标集数/时长/画幅、硬上限、阻塞 | 创建或修订 Brief |
| 整本导入/拆集 | 原文、Anchor、覆盖、冲突、忽略范围、完整度 | 修订并批准 Manifest |
| 叙事校对 | 每集结构、Mention、来源证据、failed scope | 创建/修订并批准 Narrative |
| 剧本检索 | query/filter、命中高亮、Anchor、revision basis、stale | 回源核对，不在搜索页改写事实 |
| 生产清单 | CoverageSchema、Requirement、Decision、readiness、unassessed | 决议相关性与生产需求 |
| 人物矩阵 | entity/unresolved 行、集/场/Anchor、五类空状态、完整度 | 合并、拆分、关联或暂缓身份 |
| 镜头表 | Beat/实体绑定、覆盖、谱系、顺序和锁定 | 新建/修订/拆合/重排 Shot |
| 候选与预演 | fixture 标识、Attempt、Selection purpose、顺序 | 选择当前用途 Candidate |
| Operation 中心 | 权威状态、最近证据、错误和恢复动作 | 重试、取消、接管或等待 |

## 7. 业务规则与失败体验

- Manifest、Narrative、CoverageSchema、Requirement、矩阵 basis、ShotPlan 与 Order 都显式版本化；重排不能更换稳定业务 ID。
- Scene 不得跨 ContentUnit；每个 approved character Mention 恰有一个 current entity、active unresolved 或 explicit reject 结果。
- `confirmed_absent`、`not_analyzed`、`unresolved`、`conflict` 和已出现但无生产需求必须分别表达。
- Fixture Adapter 必须走与真实执行同类的 Plan/Job/Attempt/Candidate 边界，标记 `fixture` 且外部用量为 0；禁止页面或测试直插 Candidate。
- 解析 partial、搜索 stale、投影 incomplete、媒体 quarantined 和 Operation unknown 都不得显示为成功或空结果。
- Agent、真实 Provider、PDF/OCR、完整画布、正式审阅/交付和公共搜索不属于 A。

## 8. 验收标准

| ID | Given / When / Then | 关联验收 |
| --- | --- | --- |
| PRD-A-AC-001 | 给定不少于 3 集且含无标题/冲突标题的授权整本剧本，当用户完成边界修订和批准时，全部非空来源唯一覆盖或具名忽略，并一次物化稳定 ContentUnit；失败重试不重复。 | AC-PRJ-009、AC-NAR-010/011 |
| PRD-A-AC-002 | 给定单集解析失败和用户手工修订，当批准 Narrative 时，成功集可校对，failed scope 明确，任何 Scene/Beat/Mention 均可回跳且 partial 不冒充完整。 | AC-NAR-001—012 |
| PRD-A-AC-003 | 给定跨集别名、两个同名人物和未决主体，当用户完成或暂缓消歧时，矩阵保留稳定 entity/unresolved 行；缺失或重叠 current resolution 会标 incomplete 并阻止预检。 | AC-KNO-001/002/013/014 |
| PRD-A-AC-004 | 给定地点复用、道具、服装及零 Requirement 引用的 coverage subject，当用户完成全剧清单时，每项有 Decision 或具名 unassessed；正式完成要求 unassessed=0，历史可按原 basis 重建。 | AC-KNO-003—007/015/016 |
| PRD-A-AC-005 | 给定 approved 中文/中英叙事，当按台词、别名、地点、道具和结构过滤检索时，结果只含授权 approved revision，返回正确高亮/Anchor；索引删除后结果可重建，跨 Workspace 命中为 0。 | AC-NAR-013、AC-SYS-011 的私有范围 |
| PRD-A-AC-006 | 给定批准叙事，当用户手工建立并重排至少 10 个 Shot 时，每个镜头有来源/Beat/实体绑定，覆盖缺口可见，重排不更换 Shot ID，旧 Animatic 可重读。 | AC-SHT-001—009 |
| PRD-A-AC-007 | 给定同一 Fixture 任务被重复命令、Kafka 重投和 Worker 重启，当流程完成时，只存在一个逻辑 Job、独立 fixture Candidate 与零外部用量；预览高亮不等于 Selection。 | AC-EXE-001/003/004、AC-MED-001—004、AC-USG-003 |
| PRD-A-AC-008 | 给定另一 Workspace 的已知对象、媒体或 Operation ID，当 UI/API/Worker 尝试访问时，访问被拒绝且不泄漏对象；审计保留安全证据。 | AC-IAM-001/002/006/008、AC-SYS-007 |
| PRD-A-AC-009 | 给定 Agent、模型和真实 Provider 全部关闭，当用户执行完整旅程时，仍能得到批准叙事、全剧清单/矩阵、至少 10 个镜头、Fixture Selection 与可播放 Animatic。 | AC-SYS-001/008 |
| PRD-A-AC-010 | 给定 API、Worker、Kafka、MinIO、Redis、Elastic 或 OTel 的受控中断，当恢复或降级时，业务事实不回滚、不重复，用户可见错误与下一动作来自持久化状态。 | SYS-NFR-001—007 |

## 9. 发布 Gate 与待确认

所有 `PRD-A-AC-*`、对应 P0 Requirement AC、OpenAPI/生成客户端一致性、真实 PostgreSQL/RLS、Kafka 重投、MinIO exact version/hash、权限负向和键盘主路径必须通过。未解决的 S0/S1 安全、数据损坏、跨租户或重复副作用缺陷阻止退出。

开发前还需签认：金标整本剧本及权利、目标深度集、默认 CoverageSchema、Fixture 媒体、项目硬上限和可访问性测试人。任一项缺失时可以完成契约/领域测试，但不能把对应真实用户验收报告为通过。
