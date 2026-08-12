# PLAN-012 AI 短剧 MVP 核心制作执行计划

- 状态：active（2026-08-13 用户明确要求开始；DEV-MVPA-01 已完成上游研究、Red 与 migration Green，等待自有旧库备份/恢复演练）
- 日期：2026-08-13
- 代码基线：`main@b6dbce2`（本计划首次提交；每个 DEV 另记录领取时完整 SHA）
- 输入：[PRD-012 AI 短剧 MVP 核心制作产品任务](../prd/012-AI短剧MVP核心制作产品任务.md)
- 上游：[REQ-015](../requirement/015-AI短剧MVP核心制作能力需求.md)、[DES-011](../design/011-AI短剧核心生产模块缺口与目标设计.md)、[DES-012](../design/012-AI短剧MVP核心模块拆分与实施范围.md)、[PLAN-000](./000-MVP全栈实施总计划.md)
- 输出：12 个唯一 `DEV-MVPA-*` 工程任务、71 基准人周、依赖/并行规则、数据迁移、文件边界、Red/Green、验证和 Acceptance Gate

## 1. 执行结论

当前正式计划只激活 **MVP-A：整剧到可信分镜包**。MVP-B 真实图片生成不与本计划并行：它必须等待 MVP-A、目标图片 Provider 真实账号和 Provider 控制面门禁，并先把现有 S4 的图片/视频耦合范围重新拆开。

MVP-A 不是推倒重做 S2/S3，而是在已接受事实上增加四条缺失链路：

1. `ScriptDocument → DocumentRevision → EpisodePlan → ImportCommit → Episode/ScriptVersion`；
2. `ScriptVersion → AdaptationRun → published ScriptVersion → NarrativeUnitVersion`；
3. `Asset → AssetState → AssetVersion → ShotSpecVersion`；
4. `NarrativeUnitVersion ↔ ShotSpecVersion → CoverageReport → StoryboardExportManifest`。

工程任务按纵向用户结果拆分，每个 DEV 同时负责后端事实、迁移、OpenAPI、必要前端、失败恢复和测试，不拆成独立“后端阶段/前端阶段”。AI 只写候选，所有正式 current/apply 都是显式命令。

## 2. 当前代码事实与不可重复工作

| 模块 | 当前可复用事实 | 本计划只补什么 |
| --- | --- | --- |
| projects | Project/Episode、顺序、目标时长、生命周期和 ProductionSnapshot | 批量物化公开命令和整剧来源摘要，不建立第二套 Episode |
| scripts | 单集 Source/Version、hash、CAS、ExtractionBatch/Candidate/Decision、confirmed Scene/Dialogue | 项目级 Document、Plan、改写 Run、NarrativeUnit 和失效传播 |
| assets | 六类 Asset/Version、媒体、Consent、readiness、usage/upgrade | AssetState、Occurrence、state-aware binding 和统一影响预检 |
| storyboards | Shot/Spec、order CAS、AssetVersion 固定引用、split/merge、readiness | 先修守恒，再加 DraftBatch、NarrativeReference、Coverage 和分镜包 |
| production | Task/Outbox/Attempt/预占/取消的部分事实 | MVP-A 只复用异步 Task；不实现图片/video Provider |
| media | 私有 MinIO、上传/探测/版本/位置/临时清理 | `.txt/.md` document purpose 和导出包 MediaVersion |
| frontend | Project 工作台和 Episode 的 script/assets/storyboard/tasks 路由 | 增量整剧向导、改写 diff、状态资产、覆盖栏和导出入口 |

任何任务发现需要修改既有 accepted 不变量时，必须先更新对应 Requirement/Design/PRD，而不是在 `DEV-MVPA-*` 内静默兼容。

## 3. 开始前 Gate

| Gate | 当前状态 | 负责人 | 关闭证据 | 未关闭时允许做什么 |
| --- | --- | --- | --- | --- |
| G-MVPA-001 范围接受 | closed（2026-08-13 用户明确要求执行） | 产品负责人 | 接受 PRD-012 的 MVP-A、11 个 PT、10 集/100k code points 上限和非目标 | 只评审文档，不改业务代码/表 |
| G-MVPA-002 黄金样本 | open | 产品负责人 + 短剧制作人 + QA | 一部自有 3–5 集原稿、单集 60–120 秒/12–24 镜、必拍/允许省略标注和预期分集边界入测试 fixture | 可设计匿名格式语料，不可伪造质量接受 |
| G-MVPA-003 迁移决策 | in_progress（实现已 Green；待自有旧库恢复演练） | 技术负责人 | 接受 Alembic 基线/旧库升级/备份恢复方案，并同步修订 DES-002、MOD-011 与 PLAN-000 的“仅 create_all”旧基线 | 不领取新增业务模型；先在团队自有旧库副本保存真实备份/恢复证据 |
| G-MVPA-004 工作区 | closed（计划编写前已核对） | DEV owner | 每个任务开始前重新运行 `git status --short` 并对白名单；不读/提交本地生成产物 | 保留无关产物；重叠修改时停止 |
| G-MVPA-005 真实依赖 | 当前 S0–S3 已关闭 | QA/工程 | PostgreSQL/RabbitMQ/MinIO/DeepSeek 现有合同回归可运行；新增 AI 分集/改写/分镜只在真实 DeepSeek 授权开关下接受 | 无 Key 可完成纯领域/UI，但 AI PT 保持 blocked；不要求先完成整个 Provider 管理页面 |
| G-MVPA-006 上游证据 | active（逐 DEV 关闭） | DEV owner + Reviewer | 每个 DEV 在 Green 前提交固定 commit、许可证、核心源码/测试、可复用点、失败模式和“不采用”理由；至少覆盖一个领域方案和一个成熟横向方案 | 只允许只读调研、Red 与隔离 spike；不得提交拍脑袋的生产抽象或新依赖 |

2026-08-13 用户明确要求按本计划执行，因此 G-MVPA-001 已关闭；同日又明确要求增加 GitHub 成熟方案研究，故 DEV-MVPA-01 先以只读研究/隔离 spike 启动。G-MVPA-002 在进入 DEV-MVPA-02 前关闭；G-MVPA-003 只有在迁移对标记录、三路径验证方案和恢复方案评审后才能关闭。任何 DEV 的 G-MVPA-006 未关闭时不得进入 Green。

### 3.1 上游证据 Gate 的执行格式

每个 `DEV-MVPA-*` 的实现记录必须先回答以下问题，不以 README 功能表、stars 或搜索摘要代替源码审查：

1. 至少检索 3 个候选：AI 短剧/影视领域实现、成熟横向工程、官方标准或维护方示例各优先一个；无合适候选时记录查询式和排除理由。
2. 固定仓库 commit/release，阅读 LICENSE 全文、关键模型/服务、迁移或状态机、核心测试和未完成 TODO；记录最后活跃时间只作为风险信号。
3. 对每个候选给出 `直接复用 / 适配概念 / 明确不采用`，说明本地依赖、数据迁移、失败恢复、许可证和维护成本。
4. 先写本地 delta：哪些能力当前仓库已有、哪些是真缺口；禁止为追随上游建立第二套 Episode、Task、Asset、Candidate、Media 或异常体系。
5. 只有证据表、Red 和最小 spike 同时支持方案时才进入 Green；spike 代码在评审前保持未提交，结论不成立就删除而不建立兼容层。

每个 DEV 预留 0.5–1 个工程日完成本 Gate，已包含在 71 人周基准中；若候选许可证、维护状态或 PoC 失败导致方案变化，先更新估算和 Design，再继续编码。

## 4. 数据迁移策略

`DEV-MVPA-01` 领取时最大的工程卡点是仓库只对空库执行 `metadata.create_all()`，而本计划会增加多组有引用和回填的新表；当前实现按以下最小方案接管，真实恢复证据仍受 G-MVPA-003 约束：

1. 在 `backend/` 引入 Alembic，锁定版本并建立显式 `alembic.ini`、`alembic/env.py` 和 `alembic/versions/`；不自动扫描插件。
2. 以领取 `DEV-MVPA-01` 时的完整 `main` SHA 和 SQLAlchemy Metadata 生成并人工审阅 baseline。全新环境执行 `alembic upgrade head`；测试快速建库是否继续使用 `create_all` 由 G-MVPA-003 决定，但集成验收必须走 migration。
3. 已有数据库不得直接 `stamp head`。先导出 schema/约束/索引快照，与 baseline 做零差异验证；只有完全匹配的库才允许备份后 stamp baseline。
4. 新业务表按任务分 revision，不把 13 类核心实体压进一个不可回滚 revision。每个 revision 包含 upgrade、结构校验和可逆的 schema downgrade；涉及已写业务数据时，回滚优先恢复备份或前滚修复，不做破坏性自动 downgrade。
5. 每次 revision 在三条路径验证：空库到 head、当前 schema 快照到 head、含黄金样本旧库副本到 head；运行前后均核对行数、哈希、复合 FK、唯一约束和 current 指针。
6. 应用启动只检查数据库 revision 是否为允许版本，不能在 Web 进程自动 upgrade；部署前由独立受控命令执行升级。
7. Acceptance 必须记录数据库版本、备份位置的脱敏标识、执行时长、锁影响、失败注入、恢复结果和不可外推事项。

首个 baseline 不承担生产零停机承诺。若目标环境包含不可丢失数据且无法提供副本/备份演练，PT-DAT-004 和所有新增模型任务保持 blocked。

## 5. DEV 执行台账

估算单位为剩余人周，已经包含 Design/PRD 回链、后端、迁移、OpenAPI、前端、单元/集成/E2E、普通缺陷和 Acceptance 记录。任务开始前把“待指派”替换为具体 owner；状态只能使用 `proposed/ready/in_progress/completed/blocked`。

| DEV | 当前状态 | 对应 PT | 基准人周 | 前置 | 可领取结果 |
| --- | --- | --- | ---: | --- | --- |
| DEV-MVPA-01 | in_progress（上游研究/Red/Green 已完成；待恢复证据） | PT-DAT-004 | 3 | G-MVPA-001；G-MVPA-003/006 关闭后 accepted | Alembic baseline、三路径 migration harness、启动 revision check |
| DEV-MVPA-02 | blocked（G-MVPA-002） | 全部新增 PT 的 fixture/契约门禁 | 2 | MVPA-01 | 黄金样本、格式语料、覆盖 oracle、OpenAPI/模型契约冻结 |
| DEV-MVPA-03 | proposed | PT-SCR-006 | 8 | MVPA-02 | 整剧 text/txt/md、Document/Revision/Block、格式分析 UI |
| DEV-MVPA-04 | proposed | PT-SCR-007 | 9 | MVPA-03 | 一个分集建议、边界编辑、confirm、批量物化和项目页回读 |
| DEV-MVPA-05 | proposed | PT-SCR-008 | 9 | MVPA-02 | 单集改写 Run、一个候选、diff/编辑/发布和失败恢复 |
| DEV-MVPA-06 | proposed | PT-SCR-009 | 14 | MVPA-05 | NarrativeUnit/Version、人工修正、current 影响和下游 stale |
| DEV-MVPA-07 | proposed | PT-AST-006 | 7 | MVPA-06 | AssetState/Occurrence/state current、状态矩阵和 readiness |
| DEV-MVPA-08 | proposed | PT-AST-007 | 5 | MVPA-07 | 改名/禁用/换版本影响中心、state-aware usage 与 apply |
| DEV-MVPA-09 | proposed | PT-SBD-007 | 3 | MVPA-02 | 现有 split/merge 前后端守恒修复和回归 |
| DEV-MVPA-10 | proposed | PT-SBD-008 | 5 | MVPA-06、MVPA-07、MVPA-09 | StoryboardDraftBatch、决议、Apply diff/CAS 和 UI |
| DEV-MVPA-11 | proposed | PT-SBD-009 | 4 | MVPA-08、MVPA-10 | NarrativeReference、Coverage/Decision、双向定位和 readiness |
| DEV-MVPA-12 | proposed | PT-SBD-010 | 2 | MVPA-04、MVPA-11 | 固定版本 JSON/CSV/HTML/Manifest、下载和 MVP-A E2E |
| **合计** |  | **11 个 PT** | **71 人周** |  | **整剧到可信分镜包** |

`DEV-MVPA-02` 不创建产品 PT，因为黄金 fixture 和契约冻结是全部 PT 的共同准入证据；它不能被单独标记为产品 accepted。

## 6. 逐任务 Red → Green → Refactor

### 6.1 DEV-MVPA-01/02：迁移和黄金样本

上游证据先行：

- Alembic 官方 async/cookbook 是 API 事实源；FastAPI full-stack template 用于部署前独立 upgrade；Safir 用于 async revision head fail-closed；Kitsu/Zou 的长期 revision 链只证明影视生产系统需要正式 migration，不复制其 Flask/通用 Entity 架构；
- 先形成 `复用/适配/拒绝` 对照表，再决定当前 migration spike 中的 config、connection sharing、head check 和旧库 adoption helper 哪些保留；
- 不采用 Web lifespan 自动 upgrade，不采用未知库直接 `stamp head`，不因上游项目 stars 高而引入其框架或模型层。

DEV-MVPA-01 当前证据记录：

| 候选 | 固定证据与许可证 | 结论 | 本地 delta / 不采用项 |
| --- | --- | --- | --- |
| Alembic 1.18.5 官方 | [async connection sharing](https://alembic.sqlalchemy.org/en/latest/cookbook.html#using-asyncio-with-alembic)、`current --check-heads`；MIT | 直接采用官方 Config/Environment/MigrationContext API | 官方 `create_all + stamp` 只适用于新建且已知结构；Lanverse 存量库必须先零差异验证 |
| FastAPI full-stack template `c350936` | [prestart 独立 upgrade](https://github.com/fastapi/full-stack-fastapi-template/blob/c350936d2888ef16ff4f5549684fd8db54935a89/backend/scripts/prestart.sh)、[Metadata env](https://github.com/fastapi/full-stack-fastapi-template/blob/c350936d2888ef16ff4f5549684fd8db54935a89/backend/app/alembic/env.py)；MIT | 适配部署次序和单 Metadata autogenerate | 不复制同步 SQLModel engine、初始化数据和 5 分钟固定重试；Web/Worker 不执行 upgrade |
| Safir `5d6f3c1` | [async Alembic helper](https://github.com/lsst-sqre/safir/blob/5d6f3c119c84acbc9dc3b75b7435bf30a9d9afc1/src/safir/database/_alembic.py)；MIT | 适配 `current heads == script heads` fail-closed gate | 其 stamp helper 不比较 schema；Lanverse 不直接暴露无验证 stamp |
| Kitsu/Zou `eeefd7b` | [当前 Alembic revisions](https://github.com/cgwire/zou/tree/eeefd7b557802fa073feb93bd90970dcf514e4b5/zou/migrations/versions)；AGPL-3.0 | 只作为成熟影视生产系统长期迁移的存在证据 | 不复制 Flask 模型、通用 Entity/JSONB 或 migration 代码 |

本轮对标后的实现决定是：保留独立 upgrade、async connection sharing、显式 model registry、所有运行入口严格 head gate；旧库 adoption helper 先验证表、列、类型、默认值、约束和索引，并要求安全格式的备份引用，不允许把 `command.stamp` 暴露成通用快捷命令。自动生成 baseline 已人工补齐四条 `use_alter` 循环外键；命令成功仍不替代真实恢复演练。

DEV-MVPA-01 当前实现证据（2026-08-13）：

- baseline 固定当前 42 张业务表；空库 `upgrade head` 后 `current --check-heads` 与 `alembic check` 通过，`downgrade base` 后业务表为 0；
- 模拟旧 `create_all` 数据库写入黄金行后严格接管，revision 到 head 且数据保留；未知表、缺索引、缺外键三类漂移均拒绝且不 stamp；
- 统一 server、独立 Scheduler、I/O Worker、Media Worker 均在业务操作前 fail closed；Docker 镜像显式包含 revision 目录，CI Ruff 覆盖 `alembic/`；
- 以上是专用 `lanverse_test` PostgreSQL 的工程 Green。尚未取得团队自有旧库、数据库系统真实备份和恢复后的逐行/hash 核对，因此不创建 Acceptance、不关闭 G-MVPA-003，也不领取 DEV-MVPA-02。

Red：

- 当前 `create_all` 无法升级已有表；schema 不匹配时必须失败；
- baseline stamp 前结构差异、缺索引/约束、错误 revision 必须拒绝；
- upgrade 中断、锁超时和验证失败必须能恢复；
- fixture 必须包含显式 5 集、缺号/重复/空集、无标记、Unicode 扩展字符、状态资产和拆合覆盖 oracle。

Green：

- 只加入 Alembic 及 migration verification CLI；不顺手改业务表；
- 生成 baseline、revision check 和隔离旧库副本演练脚本；
- fixture 只存自有/合成内容，文本体量最小但覆盖所有规则。

Refactor/退出：

- 更新 DES-002、MOD-011、PLAN-000 和部署说明的 schema SSOT；
- migration 测试在 PostgreSQL 运行，SQLite/Mock 不能替代；
- PT-DAT-004 Acceptance 只在三路径与恢复真实完成后创建。

### 6.2 DEV-MVPA-03：整剧导入与格式体检

Red：编码、MIME、100k 上限、显式标记、缺号/重复/空集、gap/overlap、跨空间、幂等、Worker 重启和正文泄漏先失败。

Green：

- 在 `scripts` 内按真实职责增加 document/import vertical package；
- text 直接形成 DocumentRevision；txt/md 复用 MediaVersion，服务端有界读取 UTF-8；
- 确定性解析优先同步；需要 AI/大文本分析时只创建 Task/Outbox，不把正文塞消息；
- 项目页增加整剧导入入口、格式问题和 next action。

退出：PT-SCR-006 的确定性 10 份格式语料全过，Document/Revision/Block 可刷新回读，零 Episode 写入。

### 6.3 DEV-MVPA-04：EpisodePlan 与批量物化

Red：非法边界、跨 block 切分、陈旧 revision、同键异输入、并发 confirm、部分 Episode/current、重复物化和已有项目影响先失败。

Green：

- 规则标记直接形成 review-ready Plan；无标记通过现有 DeepSeek runtime 产生一个结构化建议；
- 边界编辑只改 Plan revision，不改 Episode；
- projects 公开一个批量物化命令，scripts 通过 Protocol 调用，不直接写 Project ORM；
- confirm/materialize/publish 固定预检 hash、幂等键和 expected Project state；
- 向导展示全文、Episode 卡、预计时长、理由、置信度和影响。

退出：5 集黄金剧全量物化两次只产生一组 Episode/Source/Version；注入第 3 集失败时零半批 current。

### 6.4 DEV-MVPA-05：改写候选与发布

Red：AI 覆盖原稿、输入版本漂移、schema/长度失败、unknown 盲重发、并发发布、重复候选、Prompt/正文泄漏和取消先失败。

Green：

- AdaptationRun 复用 Task/Outbox 和现有类型化 DeepSeek integration port；PT-AIP-006 未完成时沿用已接受的 D-003 runtime 配置，完成后只替换 composition/runtime config，不改业务模型；
- 固定目标时长、保留核心情节、节奏和口语化四类约束；
- MVP 每次 Run 只生成一个候选；候选编辑后形成待发布正文；
- 发布只追加 ScriptVersion 并 CAS current；
- Episode script 页提供目标表单、服务端状态、diff、编辑、发布/取消。

退出：真实 DeepSeek 授权环境完成一条改写；无 Key/错误 Key/超时保持可解释且原稿/current 不变。

### 6.5 DEV-MVPA-06：NarrativeUnit 与失效传播

Red：字符 offset 漂移、单元顺序重复、跨版本引用、低置信修正、并发 publish、旧 Shot 仍 ready、旧导出仍 fresh 先失败。

Green：

- NarrativeUnit 是稳定身份，NarrativeUnitVersion 固定一个 ScriptVersion 和来源范围；
- 首期类型只含 scene_heading/action/dialogue/narration；
- confirmed Scene/Dialogue 保持既有事实，由公开映射引用 NarrativeUnit，不复制第二套场景；
- current 切换生成影响摘要并让旧 coverage/export evaluation hash stale；
- UI 提供结构列表、来源高亮和人工修正，不做复杂跨修订自动 mapping。

退出：黄金单集所有 required unit 有稳定 ID；发布改写后旧镜头/导出 fail-closed，历史仍可读。

### 6.6 DEV-MVPA-07/08：AssetState 与影响治理

Red：跨 Asset Version、状态同名、Occurrence 跨项目、current 竞态、禁用后仍 ready、改名破坏 FK、影响查询 N+1、陈旧 hash 和非终态任务竞态先失败。

Green：

- assets 增加 state/occurrence vertical package；AssetState current version 使用复合 FK 和 revision；
- Shot 新规格固定 `asset_id/state_id/version_id` 的复合一致性；旧 Spec 不回填漂移；
- create/link 状态建议仍是候选决定，AI 自动 merge 数为 0；
- 统一 rename/disable/version preflight+apply；Prompt 裸名称只作为需重编译快照；
- 资产页按身份→状态矩阵展示出现集、主版本、readiness 和 usage。

退出：角色常服/受伤、场景日/夜、道具完好/破损均能回溯 NarrativeUnit；改名零 FK 变化，disable 零历史删除。

### 6.7 DEV-MVPA-09：拆镜/合镜守恒修复

Red 必须先精确复现当前缺陷：

- split 全部 dialogue 留第一镜、第二镜为空；
- action beats 默认双边复制或未完整分配；
- merge `.slice(0, 8)` 截断；
- 跨 Scene merge 只保留 base 侧内容；
- 后端只校验子集而不校验并集。

Green：后端成为守恒最终事实源；split 要求两边 dialogue/action 并集等于来源且合法交集为空；merge 必须完整包含来源，跨 Scene、超 8 beats/dialogues 或 15 秒直接拒绝；前端不再生成危险默认值。

退出：现有 PT-SBD-004 回归、并发/幂等/impact hash、浏览器 split/merge 全过，未引入 NarrativeUnit 时也不会静默丢内容。

### 6.8 DEV-MVPA-10：AI 分镜草案

Red：Run 写正式 Shot、输入版本漂移、重复 Apply、人工锁定覆盖、order/spec 冲突、部分 Apply、无状态资产、Worker 重启和 unknown 先失败。

Green：

- DraftBatch 固定 Script/Narrative/AssetState/Version、目标时长和 Prompt/模型/schema；
- AI 只产 DraftShot；Decision 只追加；
- Apply preflight 返回新增/保留/修改/归档 diff，默认不归档人工锁定镜头；
- Apply 在一个事务或可证明的原子命令中创建正式 Shot/Spec/References 并 CAS order；
- 分镜页区分 AI 草案和正式镜头，允许逐镜修改/忽略与整批确认。

退出：真实黄金单集生成 12–24 个草案，Apply 前正式写入 0，冲突时正式写入 0，成功重放只回读同一结果。

### 6.9 DEV-MVPA-11：多对多覆盖与 readiness

Red：一个 unit 多镜、一个镜多 unit、对白 audio/visual 分道、重复 primary、approved omission、orphan、旧 unit、依赖 unavailable、Spec/State 切换、36/120 镜 N+1 先失败。

Green：

- ShotNarrativeReference 固定 ShotSpecVersion 和 NarrativeUnitVersion，含 channel/role/segment/origin；
- CoverageDecision 只追加且固定 coverage hash；
- CoverageReport 是派生/可缓存事实，进入现有 Shot readiness 和 Project ProductionSnapshot；
- 增加 `SCRIPT_REVISION_NOT_CURRENT`、`NARRATIVE_REFERENCE_INVALID`、`COVERAGE_UNACCOUNTED`、`SHOT_SOURCE_ORPHAN`、`COVERAGE_DEPENDENCY_UNAVAILABLE`；
- 分镜页实现文本↔镜头双向定位和 uncovered/orphan/stale 总览。

退出：required 全 covered/approved omitted、orphan=0、stale=0 才 ready；36/120 镜保持既有 P95 门禁且无按镜 N+1。

### 6.10 DEV-MVPA-12：分镜包与联合 E2E

Red：导出读取漂移 current、coverage 过期、blocked asset、幂等冲突、文件写失败、Manifest/Media 部分提交、后续改稿篡改历史和跨空间下载先失败。

Green：

- export 固定所有输入 ID/hash，生成 JSON 和 CSV/HTML；
- 通过 Media 模块登记 Export MediaVersion/Lineage，不保存本地目录路径或永久 URL；
- 任务成功后才登记 Manifest current/available；失败可恢复字节写入但不重新决定输入；
- Episode 分镜页提供导出预检、阻断清单、历史和受控下载。

退出：黄金剧至少一集从 Document 到 Export 完整 E2E 两次；第二次注入 current 变化、Worker 重启或对象存储短暂失败，结果无重复/漂移。

## 7. 代码与文件边界

以下是任务开始后的允许落点，不代表现在预建目录：

| 范围 | 允许落点 | 禁止 |
| --- | --- | --- |
| schema migration | `backend/alembic.ini`、`backend/alembic/`、`backend/tests/integration/migrations/`、启动 revision check | Web 启动自动 upgrade；对未知旧库直接 stamp；只测 create_all |
| scripts | `backend/app/modules/scripts/{documents,planning,adaptations,narrative}/`，现有 root 只做显式 Router/composition | 第二套 Episode/Scene/Task；跨模块 ORM；万能 JSON service |
| assets | `backend/app/modules/assets/states/` 和现有 contracts/service 的最小 seam | 在 assets 中建立 OutputCandidate/GenerationRun；名称作 FK |
| storyboards | `backend/app/modules/storyboards/{drafts,coverage,exports}/` 与现有 transform/readiness 增量 | 删除重建整集；覆盖旧 Spec；专业 Timeline |
| projects/media/production | 只增加批量物化、document/export media purpose、Task usage/runtime 所需窄公开契约 | 从 scripts 直写内部表；第二套存储/队列/Provider |
| frontend | `app/projects/[projectId]/` 整剧向导；现有 `studio/[episodeId]/{script,assets,storyboard}/` 增量 | 新建平行 Studio、手写 DTO/URL、本地推导 ready/coverage |
| tests | `backend/tests/{unit,integration,contract,e2e}/`、`frontend/src/**/*.test.tsx`、`frontend/tests/` | fixture/测试进入生产源码；真实剧本/Key/生成产物进 Git |

模型首次出现时必须显式加入 `model_registry.py`；模块只通过 Protocol/公开 contracts 交接。OpenAPI 生成 client 是前端唯一 DTO 来源。

## 8. API 增量基线

最终路径可在实现前因 OpenAPI 审查微调，但语义所有权不得变化：

| 方法 | 路径 | Owner / DEV |
| --- | --- | --- |
| POST/GET | `/api/v1/projects/{id}/script-imports` | scripts / MVPA-03 |
| POST/GET | `/api/v1/script-imports/{id}/analyses`、`/api/v1/script-imports/{id}` | scripts / MVPA-03 |
| POST | `/api/v1/document-revisions/{id}/episode-plans` | scripts / MVPA-04 |
| POST | `/api/v1/episode-plans/{id}/{move-boundary|split|merge|confirm}` | scripts / MVPA-04 |
| POST | `/api/v1/episode-plans/{id}/materializations` | scripts→projects contract / MVPA-04 |
| POST/GET | `/api/v1/episodes/{id}/adaptation-runs`、`/api/v1/adaptation-runs/{id}` | scripts / MVPA-05 |
| POST | `/api/v1/adaptation-runs/{id}/publish` | scripts / MVPA-05 |
| GET/POST | `/api/v1/script-versions/{id}/narrative-units` | scripts / MVPA-06 |
| GET/POST | `/api/v1/assets/{id}/states` | assets / MVPA-07 |
| POST | `/api/v1/asset-states/{id}/{occurrence-decisions|current-version}` | assets / MVPA-07 |
| POST | `/api/v1/assets/{id}/{rename-preflight|rename|disable-preflight|disable}` | assets / MVPA-08 |
| POST/GET | `/api/v1/episodes/{id}/storyboard-draft-runs`、`/api/v1/storyboard-draft-runs/{id}` | storyboards / MVPA-10 |
| POST | `/api/v1/storyboard-drafts/{id}/{decisions|apply-preflight|apply}` | storyboards / MVPA-10 |
| GET | `/api/v1/episodes/{id}/coverage` | storyboards / MVPA-11 |
| POST | `/api/v1/narrative-unit-versions/{id}/coverage-decisions` | storyboards / MVPA-11 |
| POST/GET | `/api/v1/episodes/{id}/storyboard-exports` | storyboards / MVPA-12 |

所有写接口使用统一 ActorContext、`Idempotency-Key` 和适用的 `expected_revision/current/hash`；错误返回稳定 code、safe details 和 next_action。

## 9. 并行、关键路径与人员配置

### 9.1 依赖与并行

~~~mermaid
flowchart TD
    G["范围/样本/迁移/上游证据 Gate"] --> D1["01 migration"]
    D1 --> D2["02 fixture/contract"]
    D2 --> D3["03 whole script"]
    D3 --> D4["04 episode plan"]
    D2 --> D5["05 rewrite"]
    D5 --> D6["06 NarrativeUnit"]
    D2 --> D9["09 split/merge fix"]
    D6 --> D7["07 AssetState"]
    D7 --> D8["08 asset impact"]
    D6 --> D10["10 storyboard draft"]
    D7 --> D10
    D9 --> D10
    D8 --> D11["11 coverage"]
    D10 --> D11
    D4 --> D12["12 export/E2E"]
    D11 --> D12
~~~

WIP 上限为 3 个 DEV，且每条事实流最多一个：

- scripts/import lane：MVPA-03→04；
- scripts/narrative lane：MVPA-05→06；
- asset/storyboard lane：MVPA-07/08 与 MVPA-09/10，待共同契约冻结后最多两项并行；
- MVPA-11/12 是联合收口，执行期间不再启动新 MVP-B 业务任务。

### 9.2 推荐 6 人角色

| 角色 | 主责 |
| --- | --- |
| 技术负责人/数据后端 | MVPA-01、跨模块事务、迁移和契约审查 |
| scripts 后端 | MVPA-03～06 的领域/异步/DeepSeek |
| assets/storyboards 后端 | MVPA-07～11、readiness 和性能 |
| 前端产品工程师 | 每个纵向任务的向导/diff/状态/覆盖/导出 UI |
| AI/全栈工程师 | 分集/改写/分镜 prompt/schema、Task 恢复、E2E 支援 |
| QA/全栈 | fixture/oracle、PostgreSQL/RabbitMQ/MinIO/浏览器、Acceptance |

产品负责人兼短剧制作人每周至少一次样本评审，不计入 6 名工程有效产能。

### 9.3 日历计划

| 周期 | 主要任务 | 退出结果 |
| --- | --- | --- |
| 第 1–2 周 | Gate、MVPA-01/02 | migration 和黄金样本可执行 |
| 第 3–6 周 | MVPA-03/04 与 MVPA-05 并行 | 整剧可分集物化；单集有改写候选 |
| 第 6–10 周 | MVPA-06；MVPA-09 可提前 | 稳定 NarrativeUnit；拆合不丢内容 |
| 第 9–14 周 | MVPA-07/08 与 MVPA-10 并行 | 状态资产和可审核分镜草案 |
| 第 14–16 周 | MVPA-11 | 多对多覆盖进入 readiness |
| 第 16–18 周 | MVPA-12、全量回归、缺陷收敛 | MVP-A Acceptance 评审 |

上述是 6 人、70%–80% 有效产能的基准窗口。4 人按相同关键路径预计 24–28 周。任何 Gate 等待、真实 DeepSeek 配额等待和产品样本返工单独记录，不伪装成开发完成。

## 10. 验证命令和真实证据

每个 DEV 先运行定向 Red，再运行相称回归；文件尚未创建时不得把下列未来测试命令报告为通过。最终至少执行：

```bash
cd backend
.venv/bin/ruff check app tests
.venv/bin/pyright
.venv/bin/python -m pytest
.venv/bin/python -m pip check
```

```bash
cd frontend
npm run lint
npm run typecheck
npm run test
npm run build
```

```bash
cd frontend
OPENAPI_SCHEMA_URL=http://127.0.0.1:8686/openapi.json npm run openapi2ts
cd ..
git diff --exit-code -- frontend/src/api
```

真实证据最少包含：

- migration：独立 PostgreSQL 空库、当前 schema、含样本旧库副本和失败恢复；
- async：真实 PostgreSQL/RabbitMQ，重复投递、Worker 重启、unknown/cancel；
- media：真实私有 MinIO、document 上传/有界读取、export 写入/下载/失败；
- AI：显式授权的真实 DeepSeek 分集/改写/分镜各至少一条；无 Key 回归必须同时通过；
- performance：36/120 镜 coverage/readiness/export preflight 无 N+1，P95 保持 ≤800ms/2s；
- browser：owner 主路径、viewer 只读、并发冲突、键盘和 axe；
- security：剧本/Prompt/object key/预签名 URL/凭据 sentinel 扫描零泄漏。

## 11. Acceptance 产物规则

每个 PT 真实完成后创建一份独立 Acceptance，不预占文件编号。记录：

- 精确 commit、owner、环境和依赖版本；
- Red 失败原因与 Green 后命令/通过数；
- 黄金样本只记录脱敏 ID/hash，不复制全文；
- PostgreSQL/RabbitMQ/MinIO/DeepSeek 是否真实，哪些使用替身；
- 成功、权限、并发、幂等、失败、恢复和性能证据；
- 未验证项、残余风险和不能外推到图片/视频/商业上线的边界；
- 产品负责人对 PT 的 accepted/blocked 决定。

MVPA-12 最终 Acceptance 只汇总并链接前 10 个 PT 证据，不复制全部日志。若任一 PT 缺真实证据，MVP-A 保持 in_progress/blocked。

## 12. MVP-B 接续规则

MVP-A accepted 后才执行以下动作：

1. 关闭目标图片 Provider 的 D-004；
2. 按 PLAN-011 完成该用途需要的 Connection、Credential、Health、Capability 和 Binding；
3. 更新 PRD-008/PLAN-000，把 Image 与 Video 产品接受门禁拆开，图片先走既有 `DEV-S4-02～04` 的可复用事实；
4. 若 AssetState 生成仍缺 polymorphic target，只新增一个明确 delta DEV，扩展现有 GenerationRequest，不创建第二套 Run/Candidate/Selection；
5. MVP-B 仍需 PromptRevision、Candidate/Selection、MediaLineage、真实费用/unknown、主选和 promotion E2E，累计排期按 DES-012 的 21–24 周口径管理。

未经上述修订，本计划不把现有同时要求真实 Image/Video 的 S4 偷换成“图片 MVP 已完成”。

## 13. 停止条件

- 未接受 MVP-A 或未提供可入库的自有黄金样本；
- 目标数据库有不可丢失数据但没有备份、副本或 baseline 结构一致性证据；
- 发现新增事实需要跨模块 ORM、复制 Episode/Scene/Task/Candidate 或保存可变 current 快照；
- AI 结果需要自动发布、分镜 Apply 需要删除重建整集、split/merge 需要截断内容；
- 任一操作在依赖 unavailable、stale、blocked 时仍要显示 ready；
- 真实剧本、Key、数据库、对象存储数据、日志或生成产物将进入 Git；
- 为赶日期要求用 Mock 接受 DeepSeek/Provider、跳过 migration 或跳过并发/恢复证据；
- MVP-B 在图片 Provider Gate 未关闭时要求宣称真实生成成功。

## 14. 激活后的第一个可领取任务

当前只领取 `DEV-MVPA-01`；研究、Red 与最小 Green 已完成，G-MVPA-003 关闭前只允许补自有旧库备份/恢复证据：

1. 从当前 Metadata 和真实 PostgreSQL 导出 baseline schema；
2. 先写“当前 `create_all` 无法升级已有 schema”和“错误 stamp 必须拒绝”的 Red；
3. 形成 Alembic baseline/upgrade/revision check 的最小 Green；
4. 在三条数据库路径和失败恢复中验证；
5. 同步 DES-002、MOD-011、PLAN-000；
6. 评审通过后才把 `DEV-MVPA-02` 置为 ready。

本计划已由用户激活；不得自动领取 `DEV-MVPA-02` 或预建后续目录、分支、Issue、Acceptance。DEV-MVPA-01 可以提交已验证的工程 Green，但只有完成自有旧库备份/恢复演练和评审后才能 accepted。
