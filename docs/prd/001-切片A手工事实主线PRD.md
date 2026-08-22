# 切片 A：手工事实主线 PRD

> PRD ID：PRD-A
> 上游模块：M01、M02、M03、M04（最小）、M05（Animatic）、M07（Fixture Plan/Item）、M08（Operation/Job/Attempt）、M09（Fixture Candidate）、M11（上限）、M14（来源）
> 状态：proposed

## 1. 用户问题与结果

用户需要在没有 Agent 和真实生成 Provider 的情况下，把一份真实的多集剧本先变成可确认的剧集、每集场景、全剧人物/地点/道具/服装等生产需求和人物×剧集矩阵，再形成可追溯、可编辑、可播放的镜头预演。切片 A 用手工闭环验证领域对象、版本、权限和运行骨架是否正确。

## 2. P0 用户旅程

```text
创建 Workspace/项目/Brief
  → 上传整本 DOCX/Markdown/TXT
  → 查看完整性、剧集边界与原文锚点
  → 手工 split/merge/move/rename 并确认剧集
  → 原子物化稳定 ContentUnit 与顺序
  → 手工校对每集 Scene/Beat/ProductionElementMention
  → 批准 NarrativeRevision
  → 按正文/短语/剧集/场次/人物/地点/生产类型检索 approved 剧本并回跳原文
  → 以默认 published CoverageSchema 建立全剧人物/地点/道具事实与 ProductionRequirementItem
  → 校对人物×剧集矩阵并回跳场次/原文
  → 手工创建并重排不少于 10 个 Shot
  → 用 Fixture 生成占位 Candidate
  → 人工选择分镜参考/视频占位
  → 生成顺序 Animatic
```

## 3. 范围

- 单 Workspace、单项目、一份可拆成不少于 3 个 ContentUnit 的整本来源，基础成员/项目职责；
- Project Brief 版本、来源保全、ParseReport/Anchor、结构表单；
- EpisodeBreakdownRevision/Manifest、ContentUnit 原子物化/OrderRevision、每集 Scene 与全局 occurrence 完整度；
- 人物身份/别名/稳定外观、地点身份、关键道具和最小状态；项目默认 published ProductionCoverageSchemaRevision，人工创建/修订 UnresolvedSubjectRevision、MentionResolution、ProductionRequirementRevision、CoverageSchemaRevision 与 ProductionCoverageDecision 的完整命令；Inventory/ReadinessProjection 与 MediaArtifact 分离；
- 人物×剧集矩阵以正式实体或 M04 权威 revision-bound 未决主体为行；每个 approved character Mention 恰有 entity、active unresolved 或 explicit reject 一个 current 结果，缺失/重叠使投影 incomplete；返回完整基线、failed scopes、resolution coverage counters，区分 resolved/unresolved/conflict/confirmed_absent/not_analyzed，并可回跳 Scene/Anchor；
- approved NarrativeRevision 的中文/中英全文、短语和结构 filter 检索；结果带高亮、SourceAnchor、indexed revision set/checkpoint/as_of/stale，只经 Go backend 鉴权，不索引 draft/Agent Proposal；解析和批准不依赖 Elasticsearch；
- ShotPlan/Order、覆盖检查、谱系、Fixture Candidate/Selection；
- 最小 GenerationPlanItem/Operation/Job/Attempt/Outbox/Worker 恢复、项目资源硬上限、来源权利声明；
- 简单顺序预演，不含真实 Provider、Agent、正式审阅或交付。

Fixture Adapter 必须接收与真实执行同版本的最小任务契约，生成明确标记为 `fixture` 的 Attempt、零外部用量和独立 Candidate。页面或测试不得绕过 M07/M08 直接插入 Candidate，也不得把 Fixture 结果呈现为真实 Provider 成功。

## 4. 页面与关键决定

| 页面 | 用户决定 |
| --- | --- |
| 项目创建 | 目标、时长、画幅、资源上限 |
| 整本导入/拆集 | 每集边界、标题、顺序、忽略范围和物化是否正确；首期可完全手工 |
| 结构校对 | 每集 Scene/Beat/Mention 是否正确，失败范围是否完整披露 |
| 剧本检索 | 查询正文/台词/人物/地点/道具，按剧集/场次/类型过滤并回跳 Anchor；识别 stale/unavailable 与零命中的区别 |
| 全剧实体/生产清单 | 绑定哪个 CoverageSchema；哪个身份/状态/需求可以发布，哪些只是 mentioned_only、不需要生产或仍 unassessed；零 Requirement 引用的 coverage row 仍可见 |
| 人物×剧集矩阵 | 出现、未决、冲突、确认未出现和未分析是否正确并有证据；failed scopes、unassigned/overlap 是否为零 |
| 镜头表 | 每个 Beat 如何覆盖、镜头顺序和锁定 |
| 候选/预演 | 哪个 Fixture 作为当前 purpose 选择 |
| Operation | 重试、恢复或接管 |

## 5. 明确延期

LangGraph、真实图像/视频、质量 AI、外部审片、正式时间线、完整画布、模板/API、PDF/OCR。切片 A 可以有确定性解析，不允许用假 Agent 冒充 B。

## 6. 验收与退出

1. 整本来源形成不少于 3 个已确认剧集并原子物化稳定 ContentUnit；全部非空来源唯一归属或具名忽略，失败重试不重复；
2. 真实来源产生 approved NarrativeRevision，所有 Scene/Beat/ProductionElementMention 可回跳且 partial 不冒充完整；
3. 全剧人物/地点/道具/服装需求可追到集/场/原文，Inventory 绑定不可变 CoverageSchema revision/hash，coverage universe 每项有权威 CoverageDecision 或 nullable decision + unassessed reason，零 Requirement 引用的 not_required/rejected 行仍可读，正式批准时 unassessed=0；人物×剧集矩阵保留 UnresolvedSubjectRevision 行，每个 approved character Mention 恰有一个 current entity/unresolved/reject 结果，缺失/重叠使矩阵 incomplete 并阻止预检；清空投影后行键/成员一致，并区分五类空/未决状态；
4. 用户手工建立至少 10 个有 Beat/实体绑定的 Shot，覆盖缺口可见；
5. Fixture 结果形成独立 Candidate，主选与预览高亮分离；
6. 重排不改变 ContentUnit/Shot/Entity ID，旧矩阵与 Animatic 可重读；
7. 重复命令/Outbox/Worker 重启不产生重复事实；
8. 跨 Workspace 对象/媒体/Worker 负向测试通过；
9. 关闭全部 AI/Provider 后用户仍完成上述旅程。
10. approved 中文/中英剧本可全文/短语/结构筛选并回跳正确 Anchor；搜索只返回当前有权访问的 approved revisions，索引删除后可重建且跨 Workspace 命中为 0，Elastic 中断不影响导入、校对和批准。

未满足任一项，不进入切片 B 的正式实现。
