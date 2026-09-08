# Harness Agent 设计与实现核查报告

日期：2026-09-08。范围：当前工作区的上传、分集、每集入库/检索、场次及人物分析。用户明确要求先读设计、检查实现，不继续修改代码。本文是核查记录，不是新的已批准 Design 或实现完成声明。

## 1. 结论与设计优先级

**当前实现方向大体遵循已接受架构，但没有正确、完整地接成用户需要的产品闭环。** 有真实的四任务 Harness、Python Workflow/Activity、草案/Attempt/Manifest、Go 源桥及采纳实现；同时存在主入口错接、长稿容量不足、同步事件缺失、正式语义模型不足和部署缺项。不能据此认定“所有 Agent 都没实现”，也不能认为只补环境配置即可完成。

本次优先阅读并对照：

- [0013 架构调整](../design/0013-创作编排与多媒体画布架构调整设计.md)：新 Python 编排/草案与 Go 正式 Owner 的边界、交接和四门；§11–12 的命令与服务启动合同。
- [3004 专业能力](../design/3004-AgentHarness专业能力与创作流程设计.md)：全文，尤其 §4、§6–8、§11 的文本 MVP、源保真、人物/场次、上下文和回执。
- [0015 能力缺口设计](../design/0015-Harness能力缺口评审与创作画布闭环详细设计.md)：能力目录、长稿/身份模型、执行、事件、切流及完成标准；其中历史缺口快照不是当前完成状态。
- [3003 旧 Harness 设计](../design/3003-StoryGraph剧本解析Harness与内置Skill设计.md)：适用范围声明、旧 Stage 切换面、场景事实先于全局身份的约束；新流程所有权由 0013/3004 明确替代，不能照抄旧 Go 编排。
- [0015 Spec](../requirement/0015-Agent执行清单与尝试追踪需求规格.md)、[Plan](../plan/0015-Agent服务实施计划.md)、[Acceptance](0015-Agent执行可追踪性验收记录.md)：已经完成的是特定 Attempt/初始 Manifest 切片，记录明确排除完整模型、事件与浏览器闭环。

3004 和 0015 的部分“尚未注册 Workflow/尚无源桥”文字已落后于当前未提交代码。评审以实际代码定位为准，保留历史验收范围，不把文字中的目标或已勾选局部测试当生产完成。

用户澄清：本轮“任务信息”主要指**人物信息**，不把增加执行面板替代人物建模。

## 2. 正确的既有设计链路

```text
Go 接管原稿并固定 SourceEdition
→ Go 命令/Outbox → Python 持久接受/启动 → Temporal
→ map_manuscript：分集、非正文分类、范围覆盖
→ 平台分集门 + 正式 Episode/ScriptVersion/ID 映射
→ analyze_episode：逐集摘要、场次、节拍、对白、人物/地点/道具提及
→ 平台剧稿结构门 + 正式 Scene/Beat/Dialogue/Mention
→ build_world：全稿身份归并、人物关系、状态事件、资产需求
→ 平台总册门 + 正式 WorldBook 与制作映射
→ direct_scene：按已审阅场次和披露范围生成导演草案
→ 平台导演门 + 正式文本意图
```

每集发布后的检索同步由 Go 正式版本事件驱动，数据库为事实源。Agent 不直接写平台数据库或 ES；ES 不参与决定人物身份或审批。分集已采纳后应可入库/检索，不必等全部导演分镜结束。

## 3. 核查矩阵

状态“符合局部合同”仅指本次代码/定向测试范围；“部分”不表示可上线。

| 能力 | 设计要求 | 当前事实与判断 |
| --- | --- | --- |
| 运行隔离 | 可信 Creation 与受限 Harness 分开 | `app/creation`、`app/modules`、`app/reasoning`、`app/text_contract` 分责；架构测试通过，符合局部边界 |
| 四个文本任务 | 按阶段输入输出，不越人审门 | `modules/text_storyboard/harness.py` 有四种严格模型及各自 reference，符合局部合同 |
| 原稿证据 | 固定 hash、码点、逐字引用、覆盖账 | `text_contract/source.py`、`validation.py` 有实现；28 项定向测试通过，未验证真实模型语义 |
| 专业包发布 | 固定内容/Schema/预算摘要 | `TextSkill.release_hash/guidance` 验证固定 release，符合局部合同 |
| 长稿 | 不截断；有界分块提取/归并或明确失败 | 当前整稿装配、单次请求上限 240000 字节；明确拒绝是正确保护，但缺处理本次长稿的方案 |
| 英文分集 | 保留显式集号 | 新 `inspect_source` 正确识别指定稿 1–60；旧 Go 格式解析识别 0，不能混为同一实现 |
| 编排 | Python Temporal + Activity，固定四门 | `creation/workflow.py`、`activities.py`、`worker.py` 已实现串行逐集/逐场与门查询；当前部署未接通 |
| 持久执行 | reserve/fence/预算/结果事务 | `execution.py` 有冻结、领取、unknown、finish、输出绑定；不能等同完整取消/撤回控制 |
| 执行描述 | 初始 Manifest，再动态实例和多输出 | 初始 Manifest 已实现；`instance_count` 固定 null，尚未形成设计要求的动态 PlanExpansion/多角色输出 |
| 源读取与采纳 | Go 签名源桥、ReviewDecision、Owner 回执 | Go creation/source/proposals 与 Python PlatformClient 已实现；已有合成 PostgreSQL 旅程通过，真实产品链仍阻断 |
| 分集正式保存 | 每集独立正文/版本/来源 | `AcceptTextPlanning` 写 Episode 和 published ScriptVersion，符合局部入库路径；缺同步事件 |
| ES 同步 | 正式数据 + Outbox 同事务，可靠投影 | 旧发布路径有事件，新 TextPlanning 采纳路径没有，未满足闭环 |
| 场景资料 | 场次/地点区分、时空、节拍、对白和提及 | 候选字段较完整，正式 TextSceneFacts 只保存子集；正式地点/状态绑定仍未在新采纳中填充 |
| 人物信息 | 提及→身份→造型/出场/关系/披露/状态 | 有 EntityProposal/TextWorldEntity、关系、事件及证据；完整 CastRecord/Appearance/Disclosure/跨集修订尚缺 |
| Agent 结果事件 | Outbox 发布到 Kafka/平台 Inbox | 有结果行，无对应 Publisher 和消费注册，当前主要靠查询/门接口，未满足事件闭环 |
| 前端主入口 | 新源固定后进入同一新运行 | 原稿卡仍渲染旧制作圣经与确定性分集，新文本创作另设入口；实际交互断开 |

## 4. 按影响排序的发现

### H-01 阻断：指定 60 集稿超过新 Harness 首阶段容量

设计：3004 §4、§11.3；0015 §6.1、§7.2。实现：`agent/app/modules/text_storyboard/harness.py:235` 装入所选全部 SourceBlock，`:244` 固定 240000 字节；map_manuscript 选择全稿，build_world 也装入全稿和全部 analyses。

本次读取已上传不可变修订 `2dd9c0c1-5125-4719-94fd-fd0b825fb95d`，只运行 `inspect_source` 和 `TextHarness.prepare`，未调用模型：139723 字符、3981 块、英文集号 1–60 全部识别；按当前真实装配器生成的上下文为 **438264 字节**，返回 `context_insufficient: source was not truncated`。

结论：即使部署 Creation 并修复入口，这份原稿仍不能进入第一个模型调用。拒绝静默截断符合设计；缺陷是产品没有实现适用于该规模的有界解析。全局阶段还会叠加逐集结果，存在更大的容量风险，但未取得真实 analyses，不能给出该阶段实测数值。不能仅抬高常量就声称长稿治理完成。

### H-02 阻断：当前上传页面仍进入失配的旧链路

设计：0013 §6 切流、3004 §11 四任务。`frontend/src/features/project/script-document-import-card.tsx:487` 仍渲染旧 ProductionBibleWorkspace，确认按钮只固定原稿与格式结果；新 `/creation` 是另一个入口。

旧 `backend/internal/production/bible/application/service.go:162` 将 normalized_text/bible_id/task_id 等作为 analyze_story 输入；严格 Python 旧 Stage 当前要求 evidence 候选与分片范围。上轮真实页面请求先触发旧 CHECK 500；精确修复空任务表约束后，Agent 收到 HTTP 422，未运行模型。新入口同时返回 Creation 未配置。

结论：既有设计已经决定新流程归 Python，不应继续修补旧整稿 Bible-first 主流程并称其为新 SOP。需明确新提交路由与旧历史查询/恢复边界，当前评审不执行切流或删除旧处理器。

### H-03 高：每集正式入库缺少 ES 同步事件

设计：0013 §4.3；0015 §9.1/9.4。`backend/internal/production/planning/adapter/gormdb/text_adoption.go:36` 创建剧集和版本后在 `:52` 更新项目并返回；外层 `creation/adapter/gormdb/proposals.go:242` 保存回执，也没有发布 ScriptVersionPublished。

对照旧 `planning/application/service.go:368` 的 episodeSetEvents，现有事件与搜索消费者可以复用。当前 Search Snapshot 可读取正式 published ScriptVersion，但缺少本次采纳的驱动事件，不能保证自行同步。此结论是完整写入路径静态核查，不是本轮实测 ES 丢失率。

### H-04 高：人物模型只覆盖身份候选，未完成设计中的人物档案

设计：3004 §4/§6/§11.2；0015 §6.2。`agent/app/text_contract/schemas.py` 的 EntityProposal 提供 kind/label/mentions/identity_basis/evidence/uncertainty，`backend/internal/production/bible/domain/text_world.go` 保存对应正式 ID、关系和状态。

这些是真实实现，能表达“不同称呼是否同一人”和基于证据的关系；但新链没有独立的字段级人物档案、Appearance 造型、Presence→Appearance、披露时点/知情主体规则、可撤销 merge/split 正式映射。关系只有 origin/basis，尚无完整有效故事区间和 knowledge owner。年龄、亲属、隐瞒身份、跨集服装和状态不能靠一个 label/自由描述替代。

本稿验收重点应为 Aurelia 的隐瞒身份与第 42 集揭露、Jace/Iris 与未出生孩子区分、Mila 昵称不等于血缘、Lysander 多称谓归并；当前没有模型产物，不能断言这些具体内容已误判。

### H-05 高：正式场次/人物视图未完整承接已生成字段

`text_contract/schemas.py` 的 EpisodeAnalysis 有 summary/conflict/turning_point/ending_hook，Mention 有 visual_details。新 `planning/application/text_adoption.go` 的本地采纳结构未接收这些字段；正式 `TextMention` 无 visual_details，`TextSceneFacts` 只保存场级 summary/time/presentation/mentions。新建 Scene 也未赋值已有 LocationIdentity/LocationState/Occurrences。

完整草案仍保留在提案/任务中，不能称为原始数据永久丢失；问题是正式 Owner 查询与后续制作模型没有完整映射。需逐字段核查什么是正式事实、什么仍是候选；不能仅以“JSON 已存数据库”证明人物/场景业务完整。

### H-06 高：Agent 结果 Outbox 是存储记录，尚无可靠发布闭环

`creation/execution_schema.py:43` 的 creation_result_outbox 只有 event/step/draft/type/created_at；`execution.py:235` 写 result_ready。检索当前 agent/app 未找到该表的发布领取、确认或 Kafka 发布者；现有 Go event registry 只接 ScriptVersionPublished/StoryGraphVersionPublished。

需区分两条缺口：H-03 是 Go 正式剧集→ES；本项是 Agent 草案/运行→平台项目投影。Go→Agent 的命令 Outbox/HTTPS 已有，不可用它抵扣结果方向；轮询能读取结果也不等于完成设计的 Kafka/Inbox/游标合同。

### H-07 高：新流程也会将明确的容量拒绝降级为未知推理

`candidate_runtime/text_storyboard_api.py` 明确返回 HTTP 422/context_insufficient；`creation/activities.py:112` 将所有 Harness 调用异常记为 harness_response_unknown，并保留占用额度。源预检本来没有调用模型，但该分类无法准确表达“发送前/模型前确定失败”。

保守保留未知调用额度是正确的保护，不能简单在异常时退额度；应由合同区分已确认未推理和结果未知，并保留安全错误码。旧 Go Worker 将 422 统一标 retryable=true 是另一条已实测错误，二者不可混淆。

### H-08 中高：全局人物到场景制作映射仍未完成

`TextHarness.directing_context` 主动隐藏全局 key/label，只给当前场提及与事件证据；`execute` 对每个导演结果追加 continuity_mapping_pending blocker。此防泄露措施符合 3004 的过渡合同，但不能说明 WorldResolution 已实现。

Go 支持人工 RiskResolution，不等于代码已经构造通过审阅的跨场状态/披露映射。应保留“人工处理风险”与“完整连续性映射”两种事实；本轮以人物/场景为先，不扩展为媒体制作任务。

### H-09 中：Manifest/Attempt 完成范围小于完整 SOP 控制

初始 Manifest 固定四阶段、门、源与 release/call_limit；Attempt/fence/unknown 已实现。`manifest.py` 的 instance_count 固定 null，`execution_schema.py` 输出绑定限制 candidate/primary；未找到新流程的完整 PlanExpansion、多产物版本、RunControl/ReleaseControl 围栏。

这些不是对已完成 MF-01～08 切片的否定；其 Spec 明确排除动态展开等工作。应将其标为目标未完成，不能用增加四行阶段描述替代多集可追踪产物。

### H-10 运行阻断：部署与设计版本缺少联合准入

上轮真实运行：Creation URL/Secret 未配置，只有 Candidate Harness 就绪；Go AgentInvocation CHECK 仍保留旧版协议标识；event worker 因 ES EOF 重试，API readiness 仍正常。

当前代码有真实 Worker，不等于部署了匹配队列的 Worker。需要按 0015 §15.2 分开核验 source_bridge/proposal_store/review_apply/workflow_worker/event_projection；数据库 AutoMigrate 不是旧约束升级保证。当前评审不继续修改配置或数据库。

## 5. 验证与证据边界

| 本轮检查 | 结果 | 能证明/不能证明 |
| --- | --- | --- |
| 当前源码与设计逐项比对 | 上述路径可定位 | 实现合同/缺字段/缺调用；不能证明推理质量 |
| 真实已上传原稿 → inspect_source/prepare | 60 个集号；438264 > 240000，明确拒绝 | 实际原稿容量阻断；不调用 Codex，不改变任务 |
| agent 下 `.venv/bin/python -m pytest -q tests/unit/test_text_storyboard.py tests/unit/test_text_storyboard_pipeline.py tests/integration/test_text_storyboard_api.py tests/architecture/test_agent_module_dependencies.py` | **28 passed** | 严格文本合同、合成候选链、HTTP 与模块边界；不是模型或完整 Temporal 验收 |
| backend 下现有 `TestCreationProposalReviewAdoptionPersistsFormalOwnerAndRejectsStaleDecisions`，本地 lanverse_test PostgreSQL | **通过**，6.554s | 现有合成 Owner 采纳旅程；该测试原本未断言同步事件，不能证明 ES |
| 上轮电脑实测 | 上传成功；旧链 500→422；新入口未配置 | 同会话已有运行证据，未伪称本轮重新启动模型 |

本轮曾准备补充 Outbox 断言，但在用户要求先审阅后已精确撤回；没有留下生产代码或测试修改。上述 PostgreSQL 测试运行的是原有断言，不是新增断言的 Red/Green。未执行全量 Go 门禁、真实 Temporal/Kafka/ES、真实模型或新的浏览器验收；只运行与本次核查相关的现有测试。

原稿预检安全摘要保存于本机 `Documents/Codex/2026-09-08/lanverse-script-validation/harness-context-audit.json`；完整原稿不进入 Git。

## 6. 审阅后的建议顺序（未开始实现）

1. 对齐当前有效设计与最新实现状态，明确新上传进入 Creation、旧入口如何保留历史；不再扩展错误的整稿旧协议。
2. 先制定适配真实 60 集稿的有界分集和全局归并方案，保留原文覆盖和跨块身份证据，再验证容量。
3. 列出人物/场景字段从 Python 候选 → Go 正式 Owner → 查询/检索的映射表，明确缺失、候选与正式状态。
4. 联合核对分集采纳事务及 Go→ES、Agent→平台两条事件链，分别定义可恢复检查点。
5. 在明确上述缺口后更新已有 Design 的具体变更，再细化 PRD/Spec/Plan/Acceptance，最后实施并使用该原稿复测。

当前不新增通用 AI 网关、第二个调度器、更多常驻 Agent 或空 Skill 目录。42 项能力和 11 类 Flow 是目标地图，不是该文本切片应机械创建的文件数量。

## 7. 工作区说明

main 原有修改及删除保留；本轮只新增本核查文档。先前临时写出的 0017 Design/Spec 草案已移至仓库外“未采用的闭环草案”，不作为已接受设计或编码依据。没有提交、推送、发布或继续迁移业务数据库。残留工作区逐项列表保存在本机证据目录的 git-status-review-final.txt。
