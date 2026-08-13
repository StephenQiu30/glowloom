# PLAN-012 AI 短剧 MVP 核心制作执行计划

- 状态：active（2026-08-14；DEV-MVPA-01～11 已完成真实旧库迁移、工程黄金 fixture Gate、整剧导入、分集计划/原子物化、受约束剧本改写、稳定叙事单元/失效传播、AssetState/Occurrence、资产影响治理、拆镜/合镜内容守恒、可审核 AI 分镜草案以及多对多覆盖/readiness；下一任务为 DEV-MVPA-12 可信分镜包与联合 E2E；制作人/QA 内容质量复核仍保留到分镜包产品验收）
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
| G-MVPA-002 工程黄金样本 | closed（用户明确接受原创 mock 入库；5 集/20 单元/16 镜 oracle 与 19 项契约 Green） | 产品负责人 + 工程 QA | 自有或明确接受的原创合成 3–5 集原稿、单集 60–120 秒/12–24 镜、必拍/允许省略、状态资产和预期分集边界入 fixture；工程准入与内容质量接受分离 | 制作人/QA 主观内容质量仍是 DEV-MVPA-10～12 产品验收条件；不可复制参考稿或伪造内容质量接受 |
| G-MVPA-003 迁移决策 | closed（本机真实 38 表/19 行旧库已备份、恢复、接管到 `8d9f2a6c4b71`，旧数据哈希守恒） | 技术负责人 | [Acceptance 028](../acceptance/arrived/028-Alembic历史旧库迁移与恢复验收.md)；DES-002、MOD-011、PLAN-000 同步 | 后续 revision 继续三路径验证；本次小库结果不外推零停机/RPO/RTO |
| G-MVPA-004 工作区 | closed（计划编写前已核对） | DEV owner | 每个任务开始前重新运行 `git status --short` 并对白名单；不读/提交本地生成产物 | 保留无关产物；重叠修改时停止 |
| G-MVPA-005 真实依赖 | 当前 S0–S3 已关闭 | QA/工程 | PostgreSQL/RabbitMQ/MinIO/DeepSeek 现有合同回归可运行；新增 AI 分集/改写/分镜只在真实 DeepSeek 授权开关下接受 | 无 Key 可完成纯领域/UI，但 AI PT 保持 blocked；不要求先完成整个 Provider 管理页面 |
| G-MVPA-006 上游证据 | active（逐 DEV 关闭） | DEV owner + Reviewer | 每个 DEV 在 Green 前提交固定 commit、许可证、核心源码/测试、可复用点、失败模式和“不采用”理由；至少覆盖一个领域方案和一个成熟横向方案 | 只允许只读调研、Red 与隔离 spike；不得提交拍脑袋的生产抽象或新依赖 |

2026-08-13 用户明确要求按本计划执行，因此 G-MVPA-001 已关闭；同日又明确要求增加 GitHub 成熟方案研究，故 DEV-MVPA-01 先以只读研究/隔离 spike 启动。用户随后明确允许参考其 DOCX 结构并把原创 mock 放入 `docs/`，因此 G-MVPA-002 只关闭工程 fixture/oracle 准入，制作人/QA 主观质量复核移到实际分镜与分镜包产品验收。真实本机 pre-Alembic 库的备份、恢复副本、原子升级和数据哈希守恒关闭 G-MVPA-003。任何 DEV 的 G-MVPA-006 未关闭时仍不得进入 Green。

### 3.1 上游证据 Gate 的执行格式

每个 `DEV-MVPA-*` 的实现记录必须先回答以下问题，不以 README 功能表、stars 或搜索摘要代替源码审查：

1. 每个业务模块先建立不少于 5 个候选的证据池：至少 3 个 AI 短剧/影视领域实现，以及 2 个有跨年度维护、正式发布或系统测试的横向工程/官方标准；无合适候选时记录查询式和排除理由。多个 DEV 可复用同一模块证据池，但必须重新核对固定 commit 之后的本地 delta。
2. 固定仓库 commit/release，阅读 LICENSE 全文、关键模型/服务、迁移或状态机、核心测试和未完成 TODO；记录最后活跃时间只作为风险信号。
3. 对每个候选给出 `直接复用 / 适配概念 / 明确不采用`，说明本地依赖、数据迁移、失败恢复、许可证和维护成本。
4. 先写本地 delta：哪些能力当前仓库已有、哪些是真缺口；禁止为追随上游建立第二套 Episode、Task、Asset、Candidate、Media 或异常体系。
5. 只有证据表、Red 和最小 spike 同时支持方案时才进入 Green；spike 代码在评审前保持未提交，结论不成立就删除而不建立兼容层。

星标、README 能力表和短期 release 数不能单独证明成熟。候选只有同时具备清晰许可证、可定位的核心源码、相关失败测试、可解释的数据身份/状态，以及与 Lanverse 不变量相容的依赖边界，才可进入“直接复用”；缺一项则降为“适配概念”或“明确不采用”。

剧本、资产、分镜三个模块的共享证据池在进入首个对应 DEV 前集中建立；后续每个 DEV 仍预留 0.5–1 个工程日核对增量，已包含在 71 人周基准中。若候选许可证、维护状态或 PoC 失败导致方案变化，先更新估算和 Design，再继续编码。

### 3.2 成熟度与代码准入判定

| 检查面 | 可直接复用必须满足 | 只作概念参考的典型情形 | 直接拒绝的典型情形 |
| --- | --- | --- | --- |
| 许可证 | 仓库根 LICENSE 与逐文件声明一致，允许当前分发/托管方式 | AGPL/ELv2 只研究公开行为与模型，不复制代码 | 无 LICENSE、非商业、托管限制或附加商业条款与目标冲突 |
| 维护 | 有跨年度历史或稳定 release，关键路径非单人一次性原型 | 2026 年新短剧项目虽活跃，只用于验证领域流程 | README 宣称完成但源码/TODO/测试无法支撑 |
| 正确性 | 有与本任务同构的边界、并发、失败恢复测试 | 只有 happy path 或模型 mock，需要本地 Red 重建契约 | 静默截断、删除重建、名称/序号作长期身份 |
| 数据模型 | 稳定身份、不可变修订/版本、显式引用和可恢复状态 | 可借鉴 UI/步骤，但本地必须重新建血缘 | 与现有 Episode/Task/Asset/Media 平行造第二套事实 |
| 集成成本 | 依赖体量、运行时和迁移成本小于自行实现的已证实最小内核 | 仅提取算法/测试语料，不引入整套框架 | 为 P0 txt/md 引入 PDF/OCR/ETL 或另一套队列/数据库 |

截至 2026-08-13，Jellyfish、LocalMiniDrama、ai-fusion-video、DramaClaw、ArcReel、wind-comic 均创建于 2026 年；它们可作为高相关领域证据，但不能单独承担“成熟工程事实源”。跨年度的 Fountain.js、Unstructured、OpenTimelineIO、OpenAssetIO、Kitsu/Zou 等用于校验解析、身份、版本和制作追踪不变量。结论不是整仓拼装，而是“能直接复用的局部复用，无法证明守恒的部分用本地最小实现并由上游测试语料约束”。

## 4. 数据迁移策略

`DEV-MVPA-01` 领取时最大的工程卡点是仓库只对空库执行 `metadata.create_all()`，而本计划会增加多组有引用和回填的新表；现已按以下最小方案接管，真实恢复证据见 Acceptance 028：

1. 在 `backend/` 引入 Alembic，锁定版本并建立显式 `alembic.ini`、`alembic/env.py` 和 `alembic/versions/`；不自动扫描插件。
2. 以真实历史 Metadata 和表结构快照人工校正 revision 边界：`95c0d24572c5` 固定 Provider 引入前的 38 张业务表，`8d9f2a6c4b71` 增加 4 张 Provider 表和 Capability 复合唯一约束，`4c8e2f7a9b31` 增加 4 张整剧文档/格式分析表并扩展 document Media，`7f3a9c1d2e84` 增加 4 张分集计划/批量物化血缘表，`9a4d6e2f1b73` 增加单表 AdaptationRun，`2b7e4c9a1d63` 增加 4 张稳定叙事结构/版本/影响表。全新环境独立执行 `alembic upgrade head`，集成验收必须走 migration。
3. 已有数据库不得直接 `stamp head`。adoption 只接受七种已知完整签名：当前 57 表结构直接采用 head，NarrativeUnit era 55 表结构采用 `2b7e4c9a1d63` 后升级，剧本改写 era 51 表结构采用 `9a4d6e2f1b73` 后升级，分集计划 era 50 表结构采用 `7f3a9c1d2e84` 后升级，整剧文档 era 46 表结构采用 `4c8e2f7a9b31` 后升级，Provider-era 42 表结构采用 `8d9f2a6c4b71` 后升级，或历史 38 表结构采用 baseline 后原子升级；任一能力的部分表集和任何未知漂移都拒绝且不 stamp。
4. 新业务表按任务分 revision，不把 13 类核心实体压进一个不可回滚 revision。每个 revision 包含 upgrade、结构校验和可逆的 schema downgrade；涉及已写业务数据时，回滚优先恢复备份或前滚修复，不做破坏性自动 downgrade。
5. 每次 revision 在三条路径验证：空库到 head、当前 schema 快照到 head、含黄金样本旧库副本到 head；运行前后均核对行数、哈希、复合 FK、唯一约束和 current 指针。
6. 应用运行入口只检查数据库 revision 是否为允许版本，不能在 Web 进程自动 upgrade；部署前由独立受控命令执行升级，升级到 `head` 后必须比较实际 schema 与已注册 Metadata，拒绝“版本号在 head 但业务表缺失”的伪成功。
7. Acceptance 必须记录数据库版本、备份位置的脱敏标识、执行时长、锁影响、失败注入、恢复结果和不可外推事项。

首个 baseline 不承担生产零停机承诺。本机 38 表/19 行数据已完成备份恢复和前滚守恒；任何其他含不可丢失数据的目标环境，仍必须先提供副本/备份演练，不得套用本次小库结论。

## 5. DEV 执行台账

估算单位为剩余人周，已经包含 Design/PRD 回链、后端、迁移、OpenAPI、前端、单元/集成/E2E、普通缺陷和 Acceptance 记录。任务开始前把“待指派”替换为具体 owner；状态只能使用 `proposed/ready/in_progress/completed/blocked`。

| DEV | 当前状态 | 对应 PT | 基准人周 | 前置 | 可领取结果 |
| --- | --- | --- | ---: | --- | --- |
| DEV-MVPA-01 | completed（真实旧库迁移与 Acceptance 028） | PT-DAT-004 | 3 | G-MVPA-001/003/006 closed | Alembic 历史 baseline、Provider 增量 revision、三路径/恢复、运行 head check |
| DEV-MVPA-02 | completed（原创 mock、格式语料、覆盖 oracle 与严格 fixture 契约） | 全部新增 PT 的 fixture/契约门禁 | 2 | MVPA-01 | 5 集黄金工程材料、20 单元/16 镜覆盖 oracle、状态资产和模型契约 |
| DEV-MVPA-03 | completed（Acceptance 029） | PT-SCR-006 | 8 | MVPA-02 | 整剧 text/txt/md、Document/Revision/Block、格式分析 UI |
| DEV-MVPA-04 | completed（Acceptance 030） | PT-SCR-007 | 9 | MVPA-03 | 一个分集建议、边界编辑、confirm、批量物化和项目页回读 |
| DEV-MVPA-05 | completed（Acceptance 031） | PT-SCR-008 | 9 | MVPA-02 | 单集改写 Run、一个候选、diff/编辑/发布和失败恢复 |
| DEV-MVPA-06 | completed（Acceptance 032） | PT-SCR-009 | 14 | MVPA-05 | NarrativeUnit/Version、人工修正、current 影响和下游 stale |
| DEV-MVPA-07 | completed（Acceptance 033） | PT-AST-006 | 7 | MVPA-06 | AssetState/Occurrence/state current、状态矩阵和 readiness |
| DEV-MVPA-08 | completed（Acceptance 034） | PT-AST-007 | 5 | MVPA-07 | 改名/禁用/换版本影响中心、state-aware usage 与 apply |
| DEV-MVPA-09 | completed（Acceptance 035） | PT-SBD-007 | 3 | MVPA-02 | 现有 split/merge 前后端守恒修复和回归 |
| DEV-MVPA-10 | completed（Acceptance 036） | PT-SBD-008 | 5 | MVPA-06、MVPA-07、MVPA-09 | StoryboardDraftBatch、决议、Apply diff/CAS 和 UI |
| DEV-MVPA-11 | completed（Acceptance 037） | PT-SBD-009 | 4 | MVPA-08、MVPA-10 | NarrativeReference、Coverage/Decision、双向定位和 readiness |
| DEV-MVPA-12 | in_progress | PT-SBD-010 | 2 | MVPA-04、MVPA-11 | 固定版本 JSON/CSV/HTML/Manifest、下载和 MVP-A E2E |
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
| PostgreSQL 18 官方 | [`pg_dump` custom archive](https://www.postgresql.org/docs/18/app-pgdump.html)、[`pg_restore --single-transaction --exit-on-error`](https://www.postgresql.org/docs/18/app-pgrestore.html) | 直接采用可选择、可校验且单事务失败回滚的恢复链路 | 备份成功不代表可恢复；必须在由 `template0` 创建的隔离目标库验证数据与 revision |

本轮对标后的实现决定是：保留独立 upgrade、async connection sharing、显式 model registry、所有运行入口严格 head gate；旧库 adoption helper 先验证表、列、类型、默认值、约束和索引，并要求安全格式的备份引用，不允许把 `command.stamp` 暴露成通用快捷命令。自动生成 baseline 已人工补齐四条 `use_alter` 循环外键；命令成功仍不替代真实恢复演练。

DEV-MVPA-01 当前实现证据（2026-08-13）：

- DEV-MVPA-01 验收时空库从历史 38 表 baseline 升到当时的 42 表 head；DEV-MVPA-03 前滚到 46 表，DEV-MVPA-04 前滚到 50 表，DEV-MVPA-05 前滚到 51 表，DEV-MVPA-06 再前滚到当前 55 表；五个阶段的 `current --check-heads` 与 `alembic check` 均通过；
- 模拟历史 38 表库和曾发布的完整 42 表 baseline 均能严格接管，Provider 部分表、未知表、缺索引和缺外键均拒绝且不 stamp；
- 统一 server、独立 Scheduler、I/O Worker、Media Worker 均在业务操作前 fail closed；Docker 镜像显式包含 revision 目录，CI Ruff 覆盖 `alembic/`；
- 只读审计本机真实 `lanverse` 发现 38 张业务表、19 行数据、无 `alembic_version`；从固定提交 `ce360d25^` 加载历史 Metadata 比较得到 `legacy_tables=38`、`schema_differences=0`。这揭示首版 42 表 baseline 无法接管真实旧库：Provider 变更还新增 4 张表和一个 Capability 复合唯一约束；
- Red 固定历史 38 表接管、首版 42 表 baseline 兼容、部分 Provider 拒绝、baseline/head 分段、可逆 downgrade 和旧行保留，最初 3 项失败；Green 将 `95c0d24572c5` 校正为历史 38 表，新增 `8d9f2a6c4b71` 承载 Provider 增量，并让 adoption 只接受“完整当前”或“完整历史”两种签名；
- 真实源库以 `pg_dump -Fc --no-owner --no-privileges` 形成仓库外 0600 备份，SHA-256 为 `6ff3f562205eb0785aa6a0c18877f6ea99a9c71e5bf6f81b57cc4efad7346d69`；同一文件在由 `template0` 创建的 `lanverse_mvpa_legacy_20260813_test` 以 `pg_restore --single-transaction --exit-on-error` 恢复，恢复前 38 表/19 行聚合 hash 与源库同为 `48fa8bc48566347ec58384928374ab4ebc5949e82f9e263f96ee3852883678f2`；
- 恢复副本先升级成功且旧表 hash 守恒后，正式本机 `lanverse` 才执行同一原子 adoption。结果为 42 张业务表、head `8d9f2a6c4b71`、4 张 Provider 表均 0 行，原 38 表/19 行 hash 保持不变；`current --check-heads` 与 `alembic check` 通过，隔离数据库和 `/tmp` 中间文件已删除，仓库外备份保留；
- 证据见 [Acceptance 028](../acceptance/arrived/028-Alembic历史旧库迁移与恢复验收.md)。它关闭 G-MVPA-003 和 PT-DAT-004 的当前本机 MVP 数据集，不承诺大表零停机、生产 RPO/RTO 或未知外部数据库兼容。

DEV-MVPA-02 准入前置证据（2026-08-13）：

- `backend/tests/fixtures/mvp_a/script_format_cases.json` 已形成十组可公开提交的最小合成语料，固定显式 5 集、全角/中文/英文集标记、缺号、重复、逆序、空集、前言待决、标题同行不误切、CRLF/行首空白和 Unicode 扩展汉字/Emoji；所有 marker 使用 half-open Unicode code-point 区间；
- `backend/tests/contract/test_mvp_a_script_fixture_contract.py` 以严格 Pydantic 契约拒绝额外字段，验证完整失败矩阵、精确原文切片、行号、code-point/UTF-16/UTF-8 差异、100k code-point 上限和无外部引用；该契约测试 8 项通过；
- `backend/tests/fixtures/mvp_a/README.md` 已固定黄金包的最小交付清单和仓库授权边界；格式 corpus 只固定 parser 失败矩阵，黄金 fixture 另固定工程 oracle，两者都不能证明分集、改写或分镜的主观内容质量。
- 用户随后明确允许使用其本地 DOCX 作为结构参考并把 mock 数据放入 `docs/`。只读检查确认参考稿含连续 60 集、86 页和明确的“集标记/场标题/动作/对白/钩子”结构；原文件、文件名、本地路径和正文均未进入仓库，也未复制或翻译其内容；
- `docs/fixtures/mvp_a/001-雾港倒计时合成黄金候选.md` 与 `backend/tests/fixtures/mvp_a/golden_candidate_harbor_countdown.json` 已形成原创 5 集候选：分集范围、选定第 3 集 20 个 NarrativeUnit、16 镜/92 秒、角色/地点/道具状态、固定 AssetVersion、`@图片N`、一对多/多对一、拆合守恒、批准省略和创作性镜头均有 oracle；
- `backend/tests/contract/test_mvp_a_golden_candidate_contract.py` 以严格 schema 和 11 项测试固定原文切片、覆盖守恒、仓库授权和工程/内容 Gate 分离；连同格式契约共 19 项 Green。用户明确接受 mock 作为当前工程材料，因此 `review_status.closes_g_mvpa_002=true`，DEV-MVPA-02 完成；制作人/QA 尚未对内容质量签字，`content_quality_gate` 仍保持 waiting，并迁移到 DEV-MVPA-10～12 的产品验收。

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

GitHub 证据池（固定于 2026-08-13）先于实现：

| 候选 | 固定源码/测试与许可证 | 证据结论 | 准入决定 |
| --- | --- | --- | --- |
| LocalMiniDrama `05f90fb` | [集标记解析器](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/frontweb/src/utils/scriptEpisodes.js#L6-L77)、[Episode 表](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/backend-node/migrations/01_init.sql#L20-L38)；MIT | 中文/全角数字/括号标记覆盖面有价值；解析前后 `trim` 且只返回正文副本，未发现该解析器的边界测试，也不保留 code-point span | 只吸收格式语料与误判样例；不复制为 Lanverse parser，不复用其单字段 Episode 模型 |
| DramaClaw `09b04c5` | [ChapterDetector](https://github.com/dramaclaw/dramaclaw/blob/09b04c5a056afa2b7baaf3b4d46995bedede6bc0/src/novelvideo/cognee/chapter_detector.py#L42-L114)、[嵌入章节引用/结尾句/场景块测试](https://github.com/dramaclaw/dramaclaw/blob/09b04c5a056afa2b7baaf3b4d46995bedede6bc0/tests/test_api_ingest_chapter_preview.py#L103-L250)、[ELv2 说明](https://github.com/dramaclaw/dramaclaw/blob/09b04c5a056afa2b7baaf3b4d46995bedede6bc0/docs/zh/license.md) | 证明“第一集结束”与正文中的“原著第九章”不能误切；实现只保存行号/重组 content，未给不可变修订与精确字符区间 | 只复用测试分类；ELv2 禁止把其功能作为第三方托管服务，当前平台不得复制代码 |
| Fountain.js `fcfa86a` | [lexer/tokenizer](https://github.com/mattdaly/Fountain.js/blob/fcfa86abca2a3d57337f901c7727a2fd9b1946b1/fountain.js#L2-L154)；MIT，始于 2012 年 | 成熟地分类 scene heading/dialogue/action；但标准化换行、清除缩进、按空行切块并 trim，token 无原文坐标 | 适配元素分类和 fixture 思路；不引入 JS 依赖，不用其 token 作 NarrativeBlock 身份 |
| screenplay-tools `d0fc871` | [Python 增量 parser](https://github.com/wildwinter/screenplay-tools/blob/d0fc871f02a8bdaa66a107bf6005081a0974ef2f/python/src/screenplay_tools/fountain/parser.py#L29-L182)、[UTF-8/格式回归](https://github.com/wildwinter/screenplay-tools/blob/d0fc871f02a8bdaa66a107bf6005081a0974ef2f/python/tests/fountain/test_parser.py)；MIT | Python/JS/C#/C++ 多实现和真实 `.fountain` corpus 有利于元素 taxonomy；默认 `splitlines/strip` 并合并连续 action/dialogue，不保留原区间 | 只复用 taxonomy 和测试设计；P0 不支持 `.fountain/.fdx`，不引入依赖 |
| Unstructured `8c4592a` | [txt partition](https://github.com/Unstructured-IO/unstructured/blob/8c4592a136b8abfa2e0ead78a45c3bfcf29479f8/unstructured/partition/text.py#L45-L108)、[编码/输入互斥测试](https://github.com/Unstructured-IO/unstructured/blob/8c4592a136b8abfa2e0ead78a45c3bfcf29479f8/test_unstructured/partition/test_text.py#L43-L180)；Apache-2.0，跨年度 236 个 GitHub release | 成熟格式分派、编码错误和 filename/file/text 互斥契约可借鉴；默认会自动合段、strip、分类，且接受 UTF-16/32，依赖远超 P0 | 明确不引入；Lanverse 只接受严格 UTF-8 txt/md，并保留原文坐标 |
| wind-comic `c83e1cf` | [中文剧本 parser](https://github.com/ChrisChen667788/wind-comic/blob/c83e1cf5e9b88fa8ac62bb737c79985a95243b8d/lib/script-parser.ts#L18-L190)；MIT | 覆盖中文场景/对白/动作，但先对每行 trim 并删除空行；结果只有章节号、场景 ID 与聚合文本 | 明确不采用 parser；空行和字符范围丢失会直接破坏 gap=0/overlap=0 验收 |

本地 delta 已核对：现有 `ScriptSource/ScriptVersion` 是 Episode 级不可变正文，`Scene/Dialogue` 只锚定一个 ScriptVersion；`Project/Episode` 与 current script CAS 已存在；Task/Outbox 已存在；Media 上传链存在，但 `MediaObject.kind` 和 UploadSession 目前没有 `document`。因此本任务只新增项目级 Document vertical slice 和 `document` 媒体用途，不能再建 Episode、Task、Blob 或第二套剧本版本。

综合决定：没有候选同时满足“中国短剧集标记 + 严格 UTF-8 + 全文 code-point 区间守恒 + 当前 Python/SQLAlchemy 模块边界”。首期应实现一个很小的内部 span-preserving scanner，但它不是自由发挥：规则/误判 corpus 取自上述项目，输入/编码契约取自 Unstructured 的失败测试，所有输出都必须由原文切片重建并做 hash/gap/overlap 校验。2026-08-13 已完成源码、许可证与隔离算法 spike 子 Gate，G-MVPA-002/003 已关闭；DEV-MVPA-03 必须先提交本地 Red 和 migration/API 契约，然后才能进入生产 parser Green。

隔离 spike 记录（2026-08-13）：

- spike 只存在于 `/tmp` 且验证后删除，没有进入生产或测试源码；算法按 `splitlines(keepends=True)` 累加 Python Unicode code-point 坐标，不 trim/重组正文；
- 十组已提交格式语料与五个上游失败分类全部通过：正文中的“第一集结束/原著第九章”不误切；在当前严格语法假设下，括号包装和标题同行不成为硬边界；CRLF/行首空白仍定位原文，缺号/重复/逆序/空集使用不同问题分类；
- 全文逐行 block 连续、拼接结果和 SHA-256 与输入一致，证明最小 scanner 方案可行；前言保持确定性 marker 结果但产生 `preamble_requires_decision`，不再沿用 LocalMiniDrama 静默塞入第一集的行为；
- 尚未冻结 UTF-8 BOM 的诊断/归一策略、允许的括号包装语法和公开 `FormatIssue.code` 命名；这些必须在工程黄金样本驱动的 API 契约 Red 中解决，spike 通过不能替代 PT-SCR-006 Acceptance。

Red：编码、MIME、100k 上限、显式标记、缺号/重复/空集、gap/overlap、跨空间、幂等、Worker 重启和正文泄漏先失败。

Green：

- 在 `scripts` 内按真实职责增加 document/import vertical package；
- text 直接形成 DocumentRevision；txt/md 复用 MediaVersion，服务端有界读取 UTF-8；
- 确定性解析优先同步；需要 AI/大文本分析时只创建 Task/Outbox，不把正文塞消息；
- 项目页增加整剧导入入口、格式问题和 next action。

退出：PT-SCR-006 的确定性 10 份格式语料全过，Document/Revision/Block 可刷新回读，零 Episode 写入。

完成证据（2026-08-13）：

- Red `71e23b0` 固定严格 UTF-8、输入互斥、字符区间守恒、格式问题、幂等/跨空间、迁移和零 Episode 契约；Green `2abce38` 实现四张新表、document Media 探测、项目级 API/UI 和审计；
- 合成五集 `.txt` 经真实 MinIO 直传、Media Worker 探测、DocumentRevision 保存、5 个显式集标记分析和页面刷新回读，确认没有提前创建 Episode；原始私有参考 DOCX 未入库；
- 数据库先用 0600 archive 恢复隔离副本，再从 42 表/19 行升级到 46 表 head；旧表聚合指纹不变，正式开发库随后执行相同迁移；
- 后端 `374 passed, 24 skipped`，前端 `21 files / 75 tests`，生产构建通过，完整 Playwright `10 passed`；详见 [Acceptance 029](../acceptance/arrived/029-整剧导入与格式体检验收.md)。

### 6.3 DEV-MVPA-04：EpisodePlan 与批量物化

GitHub 证据池（固定于 2026-08-13）先于实现：

| 候选 | 固定源码/测试与许可证 | 可吸收的不变量 | 不采用项 |
| --- | --- | --- | --- |
| ArcReel `ed71819` | [分集账本 ADR](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/docs/adr/0031-episode-ledger-single-source-of-truth.md)、[服务端分批规划 ADR](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/docs/adr/0032-episode-planning-server-side-batched.md)、[source range/fingerprint](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/lib/episode_ledger.py#L44-L177)、[锚点/Unicode/冲突测试](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/tests/test_episode_planner.py#L281-L711)；AGPL-3.0 | 原稿指纹、连续 source range、唯一 end anchor、机械校验后重试、并发变化拒绝、重规划只 stale 不删历史 | AGPL 代码不进入当前平台；不采用 `project.json` SSOT、物理 episode 文件和滚动 cursor 作为正式 DB 模型 |
| DramaClaw `09b04c5` | [EpisodePlanner 结构化输出/多轮工具](https://github.com/dramaclaw/dramaclaw/blob/09b04c5a056afa2b7baaf3b4d46995bedede6bc0/src/novelvideo/agents/episode_planner.py#L30-L231)、[ELv2](https://github.com/dramaclaw/dramaclaw/blob/09b04c5a056afa2b7baaf3b4d46995bedede6bc0/LICENSES/Elastic-2.0.txt) | 分集建议应显式给标题、冲突、钩子、关键事件、人物和章节范围；角色列表可约束模型输出 | 只给估计 chapter_start/end，未发现 plan revision/CAS/物化原子性同构测试；ELv2 不复制代码，不引入其图谱/Agent 框架 |
| LocalMiniDrama `05f90fb` | [确定性拆集](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/frontweb/src/utils/scriptEpisodes.js#L6-L77)、[Episode/Storyboard schema](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/backend-node/migrations/01_init.sql#L20-L60)；MIT | 有显式标记时应跳过 LLM，得到可解释确定性集数 | 解析结果直接成为 Episode script_content；没有 reviewable Plan、expected revision、幂等批量提交或历史来源 |
| Jellyfish `a967819` | [结构化 ShotDivision](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/schemas/skills/script_processing.py#L17-L44)、[已有镜头时 fail 的写库服务与测试](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/tests/test_script_division.py#L47-L109)；Apache-2.0 | AI 只产结构化候选，正式对象写入前检查既有状态；冲突应拒绝而非覆盖 | start/end 只做 `ge=1`，未证明全集覆盖；Shot 仅存 excerpt，不能替代 EpisodePlan/ImportCommit |
| OpenTimelineIO `bc5fe2d` | [Timeline/Track/Clip/MediaReference 层级](https://github.com/AcademySoftwareFoundation/OpenTimelineIO/blob/bc5fe2d78dc3f8b2a8feb7e04483d85a12e80072/docs/tutorials/otio-timeline-structure.md)、[镜头增删用例](https://github.com/AcademySoftwareFoundation/OpenTimelineIO/blob/bc5fe2d78dc3f8b2a8feb7e04483d85a12e80072/docs/use-cases/shots-added-removed-from-cut.md)；Apache-2.0，始于 2016 年 | 顺序与稳定 Clip 身份应分离；变更后用 manifest/diff 通知下游 | 它是编辑互换模型，不决定叙事分集；MVP 不引入 OTIO 依赖或把 Episode 当 Clip |

本地 delta 已核对：现有 `Project.revision`、Episode active position 唯一约束、Episode current script version CAS、ScriptSource/Version idempotency 均应复用；缺口仅是 `EpisodePlan/Proposal/ImportCommit/EpisodeSegmentOrigin` 和一个 projects 批量命令。批量物化必须由 projects 持锁并一次事务写完整顺序；scripts 只能通过 Protocol 请求，不能越界直接写 `prj_episodes`。

综合决定：显式集标记走确定性 scanner；无标记时 DeepSeek 只返回一个带 block/anchor、预计时长、理由与置信度的候选。服务端独立验证锚点唯一、边界单调、全文并集守恒和计划输入 hash；人工 confirm 后才执行 ImportCommit。该方案吸收 ArcReel 的“指纹 + 锚点 + validator + stale”和 Jellyfish 的“候选与写入分离”，但以 Lanverse 不可变 DocumentRevision、Plan revision、数据库 CAS 与事务实现，避免复制 AGPL/ELv2 代码或引入平行账本。2026-08-13 已完成源码与许可证子 Gate、本地 Red、事务失败注入和真实 DeepSeek 合同，DEV-MVPA-04 已进入 Green 并由 Acceptance 030 关闭。

Red：非法边界、跨 block 切分、陈旧 revision、同键异输入、并发 confirm、部分 Episode/current、重复物化和已有项目影响先失败。

Green：

- 规则标记直接形成 review-ready Plan；无标记通过现有 DeepSeek runtime 产生一个结构化建议；
- 边界编辑只改 Plan revision，不改 Episode；
- projects 公开一个批量物化命令，scripts 通过 Protocol 调用，不直接写 Project ORM；
- confirm/materialize/publish 固定预检 hash、幂等键和 expected Project state；
- 向导展示全文、Episode 卡、预计时长、理由、置信度和影响。

退出：5 集黄金剧全量物化两次只产生一组 Episode/Source/Version；注入第 3 集失败时零半批 current。

完成证据（2026-08-13）：

- Red `59231cf` 固定非法边界、陈旧 revision、并发 confirm、幂等、第三段失败回滚和来源守恒；Green `8cacdcd` 实现四张新表、Task/Outbox Worker、projects 原子批量命令、OpenAPI 和项目页向导；
- 显式五集计划确认前 Episode 为 0；确认后一次事务创建 5 个 Episode、5 个 Source 和 5 个 draft Version，批量发布再新增 5 个 published Version 并原子设置全部 current；同键重放不重复；
- 真实 DeepSeek 首次把候选序号误当原文块位置，服务端按硬契约拒绝；`episode-plan-prompt-v2` 改为编号 `source_blocks` 后真实结构化合同通过。模型仍只产候选，原文锚点、单调性和全集覆盖继续由服务端验证；
- 数据库经 0600 custom archive 恢复隔离副本，完成 `46 → 50 → 46 → 50`；原 46 表逐表行数指纹不变，正式开发库只执行前滚；
- 后端 `388 passed, 25 skipped`，前端 `22 files / 76 tests` 与生产构建通过，完整 Playwright `11 passed`；详见 [Acceptance 030](../acceptance/arrived/030-分集计划与批量物化验收.md)。

### 6.4 DEV-MVPA-05：改写候选与发布

改写共享 GitHub 证据池（固定于 2026-08-13）先于实现。领域项目用来验证短剧约束和审核流，跨年度项目用来校验结构化输出与 diff；README、stars 和短期 release 数不承担正确性证明：

| 候选 | 固定源码/测试与许可证 | 已证明的能力与缺口 | 准入决定 |
| --- | --- | --- | --- |
| Jellyfish `a967819` | [最小改写 Prompt](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/chains/agents/script_optimizer_agent.py#L12-L53)、[类型化完整正文与摘要](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/schemas/skills/script_processing.py#L220-L226)、[异步任务 API 测试](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/tests/test_script_processing_async_api.py#L418-L454)；Apache-2.0 | “仅围绕问题最小改写”、完整候选正文和 change summary 是合适的模型输出；异步 API 会返回 task/reused。但未发现输入 ScriptVersion 快照、候选人工编辑修订和 current CAS | 适配输出 schema 与测试分类；不引入其 TaskManager，不把请求正文复制进另一套任务事实 |
| DramaClaw `09b04c5` | [短剧逐节拍结构化改写](https://github.com/dramaclaw/dramaclaw/blob/09b04c5a056afa2b7baaf3b4d46995bedede6bc0/src/novelvideo/agents/content_rewriter.py#L14-L31)、[目标节拍/字数/口语与核心剧情约束](https://github.com/dramaclaw/dramaclaw/blob/09b04c5a056afa2b7baaf3b4d46995bedede6bc0/src/novelvideo/agents/content_rewriter.py#L116-L199)、[ELv2 说明](https://github.com/dramaclaw/dramaclaw/blob/09b04c5a056afa2b7baaf3b4d46995bedede6bc0/docs/zh/license.md) | 证明目标节拍、对白可说性、核心冲突/反转/钩子和结构化非空输出需要同时约束；函数只返回拼接字符串，没有候选持久化、人工编辑、diff 或发布并发保护 | 只吸收短剧约束维度和非法空行测试；ELv2 限制托管服务，代码与 Prompt 不复制 |
| ArcReel `ed71819` | [内容指纹与审核状态机](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/lib/script_review.py#L125-L158)、[确认后编辑重新 pending](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/tests/test_script_review.py#L207-L245)、[陈旧 revision 零写入](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/tests/test_script_batch_edit.py#L100-L177)；AGPL-3.0 | 已确认内容变化后自动重开审核、写入前比较基线、批命令失败零写入，与候选编辑/发布冲突同构；其真值是 JSON 文件指纹而非数据库不可变版本 | 只复用行为与失败测试分类；AGPL 代码、文件 SSOT 和 step1/step2 平行模型不进入当前平台 |
| ai-fusion-video `9dc3879` | [Script 乐观锁与原地更新](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/service/script/ScriptService.java#L52-L68)、[替换原稿并删除派生集/场次](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/service/script/ScriptService.java#L80-L104)；MIT | 行级 version 冲突可借鉴；但正文原地覆盖，替换来源时删除派生事实，无法回答“AI 原稿/候选/最终稿分别是什么” | 明确不采用正文更新模型；只把它作为“乐观锁仍不足以替代不可变版本”的反例 |
| LocalMiniDrama `05f90fb` | [前端轮询状态](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/frontweb/src/composables/useCanvasScript.js#L10-L26)、[整集列表覆盖保存](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/frontweb/src/composables/useCanvasScript.js#L32-L91)；MIT | pending/completed/failed/timeout 对用户可见；但保存会重发整个 Episode 列表并覆盖 `script_content`，没有 expected revision、候选身份和发布门 | 只参考紧凑任务状态 UI；不复用整表 payload、轮询器和可变 script_content 模型 |
| LangChain DeepSeek `06ab861`（`langchain-deepseek==1.1.0`） | [`with_structured_output` 的 json mode/schema 契约](https://github.com/langchain-ai/langchain/blob/06ab861ace88325966fda14ec9a4f1600f441cbb/libs/partners/deepseek/langchain_deepseek/chat_models.py#L457-L557)、[严格模式/兼容性测试](https://github.com/langchain-ai/langchain/blob/06ab861ace88325966fda14ec9a4f1600f441cbb/libs/partners/deepseek/tests/unit_tests/test_chat_models.py#L304-L426)；MIT，项目始于 2022 年 | 当前仓库已锁定相同版本并通过真实 DeepSeek 分集合同；可直接沿用 Pydantic 结构化输出，不需要第二套 Agent SDK | 直接复用当前依赖和 integration port；业务侧仍须独立验证长度、输入 hash、状态和 unknown，不把“解析成功”等同于可发布 |
| CPython `6cb20a2`（v3.13.5） | [`unified_diff`](https://github.com/python/cpython/blob/6cb20a219a860eaf687b2d968b41c480c7461909/Lib/difflib.py#L1095-L1151)、[空输入/换行/类型测试](https://github.com/python/cpython/blob/6cb20a219a860eaf687b2d968b41c480c7461909/Lib/test/test_difflib.py#L67-L81)；PSF-2.0，跨年度标准库 | 当前 `ScriptVersion` diff 已直接使用同一标准库，足以展示行级新增/删除；diff 不是保存格式、补丁或合并真值 | 直接复用现有服务，不增加表、不把 diff 结果持久化、不用 diff 反推发布正文 |
| jsdiff `13576bf`（v8.0.3） | [行 diff 的 CRLF/空行/最大编辑长度测试](https://github.com/kpdecker/jsdiff/blob/13576bfbcc444ce48f71cfd1e08529bd13962411/test/diff/line.js#L6-L121)；BSD-3-Clause，始于 2011 年 | 浏览器端词/行 diff 完整，但会新增一套 diff 口径；当前服务端已经返回权威 unified diff | 明确不新增依赖；若以后需要富文本词级高亮，再单独以同一 base/target version 做无状态呈现 spike |

本地 delta 与最小落地决定：

- 现有 `ScriptSource/ScriptVersion` 已保存不可变正文和 SHA-256，`publish_version` 已在一笔事务里锁 Source/Episode、追加 published Version、CAS current 并给出旧镜头影响；现有 diff API、Task/Outbox、取消字段、DeepSeek 类型化 port 和统一错误体系全部复用，不新建 ScriptRevision、Candidate、Task 或 diff 表。
- 真缺口只是一张 `AdaptationRun`：固定 `input_script_version_id + input_hash`、四类约束、engine/model/prompt/schema、Task、幂等键与状态；AI 完成后保存不可变 `candidate_body/hash/summary/estimated_duration`，另以 `draft_body/hash/revision` 保存人工工作稿，发布后固定 `published_script_version_id`。
- Worker 消息只携带 `task_id`，在事务中回读 Run 与输入 ScriptVersion；Prompt、原稿、候选正文和幂等键不得进入 Outbox、Task payload、Audit metadata 或错误详情。AI 只返回候选，不调用版本发布服务。
- 候选编辑必须提交 `expected_revision`；发布必须同时提交 `expected_run_revision + expected_current_version_id + idempotency_key`。发布只读取已锁定 Run 的 draft，追加一个 ScriptVersion 并 CAS current；同键重放返回同一 published version，不同正文/陈旧 current/并发第二发布 fail closed。
- `difflib` 仅动态比较 input version 与当前 draft；用户取消、无 Key、鉴权失败、非法 schema/长度、超时 unknown、worker 重投都不得改变原稿或 Episode current。unknown 不自动重发供应商请求，只允许新建 Run。
- 隔离 spike 结论：不引入 jsdiff、Agent SDK、第二套队列或短剧上游 ORM；DEV-MVPA-05 的最小 schema 为单表 Run，API 为 create/get/edit/diff/cancel/publish，前端嵌入现有 Episode script 工作台。

Red：AI 覆盖原稿、输入版本漂移、schema/长度失败、unknown 盲重发、并发发布、重复候选、Prompt/正文泄漏和取消先失败。

Green：

- AdaptationRun 复用 Task/Outbox 和现有类型化 DeepSeek integration port；PT-AIP-006 未完成时沿用已接受的 D-003 runtime 配置，完成后只替换 composition/runtime config，不改业务模型；
- 固定目标时长、保留核心情节、节奏和口语化四类约束；
- MVP 每次 Run 只生成一个候选；候选编辑后形成待发布正文；
- 发布只追加 ScriptVersion 并 CAS current；
- Episode script 页提供目标表单、服务端状态、diff、编辑、发布/取消。

退出：真实 DeepSeek 授权环境完成一条改写；无 Key/错误 Key/超时保持可解释且原稿/current 不变。

完成证据（2026-08-13）：

- 单表 `scr_adaptation_runs` 固定当前 published ScriptVersion/hash、四类约束、Provider/Prompt/Schema 版本、Task 和幂等命令；AI candidate 与人工 draft 分离，发布只追加 ScriptVersion 并 CAS current。
- `script_adaptation.requested` 复用 Task/Outbox/I/O Worker，消息只携 `task_id`；无 Provider、输入漂移、非法 schema/长度/时长、中断 unknown、取消和并发发布均 fail closed，正文/Prompt/幂等键不进 Task/Outbox/Audit。
- 真实 DeepSeek 首次对 45 秒目标只返回 15 秒自估，Gate 未放行；增加明确接受区间与服务端 ±25% 硬校验后，真实合同返回 45 秒/161 字候选并通过。
- Episode script 工作台已接入约束表单、服务端轮询/刷新恢复、工作稿编辑、权威 diff、显式发布/取消和 unknown 引导；浏览器真实栈验证无 Provider 时原稿/current 不变且可重建 Run。
- 正式库在仓库外 `0600` 备份和隔离恢复 `50 → 51 → 50 → 51` 通过后才前滚到 `9a4d6e2f1b73`；旧 50 表/19 行守恒，新表为 0，`alembic check` 无漂移。
- 全部 revision 文件已从中文/混合名一次性改为英文 snake_case，没有旧名别名或 allowlist；DES-000 增加 64 字符文件名上限和 POJO 等价 Plain Data Contract 规范，架构测试强制执行。
- 后端 `.venv/bin/python -m pytest -q` 为 `398 passed, 26 skipped`，Ruff/Pyright 全过；前端 Vitest `23 files / 79 tests`、TypeScript、ESLint、生产构建和新 Playwright 闭环全过；OpenAPI 重生成哈希不变。详见 [Acceptance 031](../acceptance/arrived/031-剧本改写候选与发布验收.md)。

### 6.5 DEV-MVPA-06：NarrativeUnit 与失效传播

叙事结构 GitHub 证据池（固定于 2026-08-13）先于实现。编辑器项目只证明单次编辑事务中的节点身份/位置映射，追踪项目证明依赖失效；任何上游都不能替代数据库不可变版本和人工决议：

| 候选 | 固定源码/测试与许可证 | 已证明的能力与缺口 | 准入决定 |
| --- | --- | --- | --- |
| Tiptap UniqueID `3c929ad` | [UUID 节点属性、初始补 ID 与重复修复](https://github.com/ueberdosis/tiptap/blob/3c929ad21119dfd159eb298674198c67bb1146f1/packages/extension-unique-id/src/unique-id.ts)、[粘贴/协作/撤销测试](https://github.com/ueberdosis/tiptap/blob/3c929ad21119dfd159eb298674198c67bb1146f1/packages/extension-unique-id/__tests__/unique-id.spec.ts)；MIT | 稳定节点 ID 与节点位置分离，复制/协作造成重复时重发一个 UUID；但身份只活在当前编辑文档，拆分/合并语义和独立 AI 全文改写没有持久映射 | 采用 `unit_id ≠ position/content_hash`；不引入富文本运行时，也不把编辑器节点 ID 当跨 ScriptVersion 映射真值 |
| ProseMirror transform `662b7a9` | [StepMap 与 deleted/deletedAcross](https://github.com/ProseMirror/prosemirror-transform/blob/662b7a937bafde19b7e2a83241dbc8888e257c89/src/map.ts)、[插入/删除/替换/镜像映射测试](https://github.com/ProseMirror/prosemirror-transform/blob/662b7a937bafde19b7e2a83241dbc8888e257c89/test/test-mapping.ts)；MIT | 有完整 edit steps 时可确定性映射位置并识别被删除范围；但当前 AI Provider 只返回整份候选正文，没有可验证 step log | 只借鉴“删除必须显式、位置不是身份”的失败分类；不对独立全文做伪精确自动 mapping |
| Hypothesis client `b4d085a` | [TextPosition/TextQuote selector](https://github.com/hypothesis/client/blob/b4d085a2f893aa6de3b61d8b8bc3ae4d0f24fc1a/src/annotator/anchoring/types.ts)、[exact→approximate→context 匹配](https://github.com/hypothesis/client/blob/b4d085a2f893aa6de3b61d8b8bc3ae4d0f24fc1a/src/annotator/anchoring/match-quote.ts)、[重复文本/上下文/失败测试](https://github.com/hypothesis/client/blob/b4d085a2f893aa6de3b61d8b8bc3ae4d0f24fc1a/src/annotator/anchoring/test/match-quote-test.js)；BSD-2-Clause 风格 | 位置、exact、prefix、suffix 联合能在小改后重锚，重复 quote 需要上下文，无法锚定时保留 orphan | 保存 code-point range、exact text 与上下文；模糊结果只能成为人工候选，不能自动改 Unit/Shot 外键 |
| Doorstop `af3b671` | [link stamp/suspect/显式 clear](https://github.com/doorstop-dev/doorstop/blob/af3b671a1b93f605a61b9a17a8c2e025d7522a3b/doorstop/core/item.py#L527-L541)、[stamp 固定 UID/正文/引用](https://github.com/doorstop-dev/doorstop/blob/af3b671a1b93f605a61b9a17a8c2e025d7522a3b/doorstop/core/item.py#L863-L884)、[suspect 回归](https://github.com/doorstop-dev/doorstop/blob/af3b671a1b93f605a61b9a17a8c2e025d7522a3b/doorstop/core/tests/test_item.py#L822-L992)；LGPL-3.0 | 上游内容戳变化使链接 suspect，只有显式确认才能建立新基线；历史仍可读 | 采用 narrative dependency hash 与 stale/fresh 比较；不引入 YAML/文件 UID 或 LGPL 运行时 |
| Unstructured `8c4592a` | [UUID 与 filename/text/page/sequence hash 分离](https://github.com/Unstructured-IO/unstructured/blob/8c4592a136b8abfa2e0ead78a45c3bfcf29479f8/unstructured/documents/elements.py#L731-L808)、[重复元素唯一与确定性 hash 测试](https://github.com/Unstructured-IO/unstructured/blob/8c4592a136b8abfa2e0ead78a45c3bfcf29479f8/test_unstructured/documents/test_elements.py#L682-L735)；Apache-2.0 | 内容/位置 hash 可复现，随机 UUID 适合作数据库身份；相同文本元素需要 sequence 才能避免碰撞，重新分块仍会改变 hash | Unit 使用 UUID，UnitVersion 单独保存 text/context hash；禁止用正文 hash、行号或范围拼出稳定 ID |
| xStudio `d60b3e8` | [Timeline Item UUID/children/range](https://github.com/AcademySoftwareFoundation/xstudio/blob/d60b3e87fc52fb87b4d4e545e16dcd35471c567c/include/xstudio/timeline/item.hpp#L47-L148)、[Track/Clip/Gap/range 测试](https://github.com/AcademySoftwareFoundation/xstudio/blob/d60b3e87fc52fb87b4d4e545e16dcd35471c567c/src/timeline/test/timeline_test.cpp#L21-L96)；Apache-2.0 | UUID、显示顺序和范围独立，clone 是否保留 ID 是显式选择 | 采用稳定 Unit 身份、结构 revision 和有序 UnitVersion 快照；不引入 C++ timeline/runtime |
| Jellyfish/ai-fusion-video | [Jellyfish Shot 只存 excerpt](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/models/studio_shots.py#L24-L68)、[ai-fusion-video 替换剧本后删除派生结构](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/service/script/ScriptService.java#L80-L104)；Apache-2.0/MIT | 领域项目最多引用 episode/excerpt 或删除重建，没有稳定 revision→unit→shot 血缘 | 仅作为反例；不采用自由文本摘录、整集删除重建或“重新生成即迁移” |

本地 delta 与最小落地决定：新增 `NarrativeStructure`、稳定 `NarrativeUnit`、不可变 `NarrativeUnitVersion` 和 `NarrativeImpactAssessment`；初次结构化走确定性行级解析，正式范围使用 Unicode code point。人工修正提交完整有序快照，复用 Unit ID 时 kind/episode/workspace 必须一致，服务端重新切片 exact text、校验非空内容守恒并追加 structure revision。新 ScriptVersion 默认产生全新 Unit 身份，不自动迁移旧 Shot；未来重锚只能产待确认候选。

所有发布/current 切换必须先确保目标版本已有可验证结构，再 CAS Episode current，并把旧/新 narrative dependency hash、受影响 Shot 和 `shot_readiness/coverage/export` 失效范围写入影响事实。现有 Shot readiness 同时比较 Episode current ScriptVersion 与固定 Narrative dependency hash；未来 Coverage/Export 只能复用同一 hash，不得另造字符串扫描。已确认 Scene/Dialogue 保持原表 SSOT，UnitVersion 只保存公开外键映射和文本证据，不复制第二套场景内容。

Red：字符 offset 漂移、单元顺序重复、跨版本引用、低置信修正、并发 publish、旧 Shot 仍 ready、旧导出仍 fresh 先失败。

Green：

- NarrativeUnit 是稳定身份，NarrativeUnitVersion 固定一个 ScriptVersion 和来源范围；
- 首期类型只含 scene_heading/action/dialogue/narration；
- confirmed Scene/Dialogue 保持既有事实，由公开映射引用 NarrativeUnit，不复制第二套场景；
- current 切换生成影响摘要并让旧 coverage/export evaluation hash stale；
- UI 提供结构列表、来源高亮和人工修正，不做复杂跨修订自动 mapping。

退出：黄金单集所有 required unit 有稳定 ID；发布改写后旧镜头/导出 fail-closed，历史仍可读。

实现与验收证据（2026-08-13）：

- Red `d0c9cf5` 先固定黄金 20 单元稳定 ID、不可变修正、跨版本/重叠/并发拒绝、current 影响和旧 Shot fail-closed；Green `c32e1f6` 实现四表叙事内核、OpenAPI/页面和 readiness 依赖；
- 人工修正必须保全所有非空叙事 code point；Unit kind 作为稳定身份属性不原地篡改，前端对其只读，不保留服务端必然拒绝的假编辑兼容层；
- 正式开发库在仓库外 `0600` 备份和隔离恢复 `51 → 55 → 51 → 55` 通过后才前滚到 `2b7e4c9a1d63`；另一轮播种回填得到 1 Structure/3 Unit/3 UnitVersion，隔离库已删除；
- 后端全量 `404 passed, 26 skipped`，Ruff/Pyright 全过；前端 `24 files / 81 tests`、TypeScript、ESLint、生产构建和 Playwright 真实发布/修正/刷新闭环全过；
- 完整证据见 [Acceptance 032](../acceptance/arrived/032-稳定叙事单元与失效传播验收.md)。PT-SCR-009 和 DEV-MVPA-06 在明确非目标内 accepted；跨修订自动重锚、AssetState、多对多 Coverage 和分镜包仍由后续 DEV 承担。

### 6.6 DEV-MVPA-07/08：AssetState 与影响治理

资产共享 GitHub 证据池（固定于 2026-08-13）先于实现；领域项目用于确认短剧工作流，跨年度项目用于校验身份、版本、发布和失败恢复：

| 候选 | 固定源码/测试与许可证 | 已证明的能力与缺口 | 准入决定 |
| --- | --- | --- | --- |
| ai-fusion-video `9dc3879`（v1.1.2） | [Asset/AssetItem](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/entity/asset/AssetItem.java#L11-L59)、[镜头资产引用](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/entity/storyboard/StoryboardItem.java#L120-L129)、[局部清空/保留字段测试](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/test/java/com/stonewu/fusion/service/storyboard/StoryboardServiceTests.java#L280-L355)；MIT | 子资产已表达角度、表情、姿势和变体，镜头直接存子资产 ID；但子资产图片可原地改、没有不可变版本/主选决议，角色/道具 ID 仍塞 JSON | 只参考状态选择 UI 和 PATCH 三态语义；不复制模型，不把 AssetItem 当 AssetState/AssetVersion |
| Jellyfish `a967819` | [角色/场景/道具及项目链接](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/models/studio_assets.py#L11-L145)、[多角度图片与主图约定](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/models/studio_asset_images.py#L11-L68)、[视频 readiness 组合检查](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/services/studio/shot_video_readiness.py#L77-L203)；Apache-2.0 | UUID 资产、显式项目/镜头链接和 readiness 分项有价值；但场景/道具名称全局唯一，主图唯一性只靠应用层，未形成状态化不可变版本 | 适配 readiness check/next action 形态；不采用全局名称唯一和可变图片行 |
| ArcReel `ed71819` | [级联改名影响报告](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/lib/asset_rename.py#L29-L96)、[冲突/碰撞/dry-run 测试](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/tests/lib/test_asset_rename.py#L435-L674)；AGPL-3.0 | 预览与执行共用扫描、冲突时整体拒绝、历史/文件碰撞 fail loud 值得借鉴；但其 ADR 明确“名称即身份”，重命名需改写剧本和文件路径 | 只复用行为与失败测试分类；AGPL 代码不进入平台，Lanverse 改名不得改变稳定 ID/FK |
| wind-comic `c83e1cf` | [资产连续性台账](https://github.com/ChrisChen667788/wind-comic/blob/c83e1cf5e9b88fa8ac62bb737c79985a95243b8d/lib/asset-ledger.ts#L19-L170)、[描述变更影响镜头测试](https://github.com/ChrisChen667788/wind-comic/blob/c83e1cf5e9b88fa8ac62bb737c79985a95243b8d/tests/asset-ledger.test.ts#L23-L75)；MIT | 能从资产描述变更返回受影响镜号；但资产 ID 由名称拼接、usage 由名称/自由文本扫描、镜头以 `shotNumber` 定位 | 只吸收“先预览 affected shots 再置 stale”的交互；名称/镜号匹配明确不采用 |
| OpenAssetIO `3e60be1` | [受 manager 校验的 opaque EntityReference](https://github.com/OpenAssetIO/OpenAssetIO/blob/3e60be1d4014bfc582c44a1e54c990ee4a695a89/src/openassetio-core/include/openassetio/EntityReference.hpp#L13-L42)、[resolve/preflight/register/relationship 合规套件](https://github.com/OpenAssetIO/OpenAssetIO/blob/3e60be1d4014bfc582c44a1e54c990ee4a695a89/src/openassetio-python/package/openassetio/test/manager/apiComplianceSuite.py#L226-L783)；Apache-2.0，跨年度标准 | 稳定实体引用、发布前 preflight、注册后解析、批量逐项错误是成熟契约；它是互操作 API，不是资产数据库，也不替宿主管理版本 | 适配契约，不引入运行时依赖；现有 UUID/复合 FK 继续做 SSOT，禁止把 OpenAssetIO 当现成 DAM |
| Kitsu/Zou `eeefd7b` | [Entity/selected preview](https://github.com/cgwire/zou/blob/eeefd7b557802fa073feb93bd90970dcf514e4b5/zou/app/models/entity.py#L30-L200)、[shot/scene AssetInstance](https://github.com/cgwire/zou/blob/eeefd7b557802fa073feb93bd90970dcf514e4b5/zou/app/models/asset_instance.py#L9-L45)、[revision/position/validation](https://github.com/cgwire/zou/blob/eeefd7b557802fa073feb93bd90970dcf514e4b5/zou/app/models/preview_file.py#L25-L67)、[breakdown/实例测试](https://github.com/cgwire/zou/blob/eeefd7b557802fa073feb93bd90970dcf514e4b5/tests/services/test_breakdown_service.py#L472-L590)；AGPL-3.0 | 资产身份、镜头/场景 occurrence、候选预览与主选、where-used 已在成熟制作追踪中分开；上传失败清理和 revision/position 也有服务测试 | 适配 occurrence/where-used/selected-valid-member 行为；不复制 AGPL、通用 Entity/JSONB 或其权限模型 |
| OpenPype `f67bacf` | [Version schema](https://github.com/ynput/OpenPype/blob/f67bacf11713cadb5c46c1580ff7ef97276cf0f9/openpype/pipeline/schema/version-3.0.json#L1-L83)、[Representation/dependency schema](https://github.com/ynput/OpenPype/blob/f67bacf11713cadb5c46c1580ff7ef97276cf0f9/openpype/pipeline/schema/representation-2.0.json#L1-L78)、[真实 publish 集成测试](https://github.com/ynput/OpenPype/blob/f67bacf11713cadb5c46c1580ff7ef97276cf0f9/tests/integration/hosts/maya/test_publish_in_maya.py#L41-L104)；MIT，仓库已归档 | `Asset → Subset → Version → Representation` 的只追加发布链、来源/作者/时间和文件校验成熟；但以 Mongo/宿主 DCC 和名称上下文为中心，仓库已停止维护 | 只适配不可变 Version/媒体 representation/promotion 与集成测试形态；不引入 Mongo、插件框架或 archived runtime |
| xStudio `d60b3e8` | [Media 多 Source 与 current 测试](https://github.com/AcademySoftwareFoundation/xstudio/blob/d60b3e87fc52fb87b4d4e545e16dcd35471c567c/src/media/test/media_test.cpp#L14-L46)、[Playlist 有序增删移动与序列化测试](https://github.com/AcademySoftwareFoundation/xstudio/blob/d60b3e87fc52fb87b4d4e545e16dcd35471c567c/src/playlist/test/playlist_test.cpp#L16-L55)；Apache-2.0，跨年度 | 稳定 UUID 与显示顺序分离；current source 必须属于候选集合，非法选择拒绝且序列化后保持 | 适配 selected-valid-member、identity≠order 不变量；不引入 C++/Qt 播放审片栈 |
| Prism `fd440de`（v2.1.3） | [ProductBrowser 的 entity/product/version 层级](https://github.com/PrismPipeline/Prism/blob/fd440de6747b7e62e2ba8cb675028e7adbbce1e3/Prism/Scripts/ProjectScripts/ProductBrowser.py)、[LGPL-3.0-or-later 声明](https://github.com/PrismPipeline/Prism/blob/fd440de6747b7e62e2ba8cb675028e7adbbce1e3/README.md#L37-L40) | 产品/版本历史、master version、依赖查看器和场景文件锁是成熟 UX；仓库未发现与本任务同构的自动化失败测试，且主干是文件路径/DCC UI | 仅作为资产圣经/版本浏览交互参考；不直接复用或为 MVP 引入其桌面管线 |
| AYON Backend `08e5492`（2026-08-13 复核） | [Product 实体](https://github.com/ynput/ayon-backend/blob/08e549239c5ca2c2f40d579e0a81165e959a6494/ayon_server/entities/product.py)、[Version/HERO 约束](https://github.com/ynput/ayon-backend/blob/08e549239c5ca2c2f40d579e0a81165e959a6494/ayon_server/entities/version.py)、[Representation 实体](https://github.com/ynput/ayon-backend/blob/08e549239c5ca2c2f40d579e0a81165e959a6494/ayon_server/entities/representation.py)、[FSL-1.1-ALv2 许可证](https://github.com/ynput/ayon-backend/blob/08e549239c5ca2c2f40d579e0a81165e959a6494/LICENSE) | 持续维护的 `Product → Version → Representation` 分层和单 HERO 规则证明身份、发布版本、物理表示与主选应分开；但 HERO 唯一性由保存钩子查询保证，仓库缺同构失败测试，当前许可证禁止竞争性商用服务并延迟两年才转 Apache-2.0 | 只作为新鲜行为对照；不复制源码、不引入运行时，并用数据库复合 FK/唯一约束强化 state current 合法成员关系 |

本地 delta 结论：现有 `Asset/AssetVersion/MediaReference/Consent`、Script extraction candidate、Shot 固定 AssetVersion、usage/upgrade preflight 均继续复用。C4 只新增 `AssetState/Occurrence`、state current、state-aware binding、rename/disable/version 影响治理和手工/上传 Version；`PromptRevision/GenerationRun/OutputCandidate/Selection/MediaLineage/Provider` 由共享 C5 独占，C4 只消费展示契约，禁止重复计费或另造候选体系。

综合决定：本轮没有候选可直接替换当前资产模块，也没有必要新增第三方运行时。实现只吸收五条已被成熟项目反复验证的不变量：稳定身份不随名称/顺序变化；状态/出现范围/媒体版本分层；主选必须属于可用候选；变更先用同一输入做 preflight 再 apply；历史引用固定版本且禁用只让派生 readiness 失效、不删除历史。该证据子 Gate、黄金 fixture 和首轮迁移 Gate 已完成；DEV-MVPA-07 已满足 MVPA-06 前置，必须用本地 Red/revision 证明复合一致性后才能进入 Green。证据没有减少本地迁移、UI 和回归工作，两个 DEV 仍按 `7+5=12` 基准人周；共享 C5 候选/媒体底座不得在这里重复相加。

Red：跨 Asset Version、状态同名、Occurrence 跨项目、current 竞态、禁用后仍 ready、改名破坏 FK、影响查询 N+1、陈旧 hash 和非终态任务竞态先失败。

Green：

- assets 增加 state/occurrence vertical package；AssetState current version 使用复合 FK 和 revision；
- Shot 新规格固定 `asset_id/state_id/version_id` 的复合一致性；旧 Spec 不回填漂移；
- create/link 状态建议仍是候选决定，AI 自动 merge 数为 0；
- 统一 rename/disable/version preflight+apply；Prompt 裸名称只作为需重编译快照；
- 资产页按身份→状态矩阵展示出现集、主版本、readiness 和 usage。

退出：角色常服/受伤、场景日/夜、道具完好/破损均能回溯 NarrativeUnit；改名零 FK 变化，disable 零历史删除。

DEV-MVPA-07 实现与验收证据（2026-08-13）：

- 直接在现有 assets 纵向模块完成根模型切换，没有为了目录图预建 `states/` 空包：`Asset` 只保留稳定身份；原子 `base` `AssetState` 承担 current，`AssetVersion` 固定且校验同一 `asset_id + asset_state_id + workspace_id`；旧的 Asset 级 current 字段、路由和 `candidate` source_type 已直接移除，不保留双写、别名或兼容 DTO；
- `AssetOccurrence` 是只追加的 link/unlink 决议，固定 `NarrativeUnitVersion` 而不是集号、名称或可变正文；读取时结合当前 Script/NarrativeUnit 版本派生 `current/stale`，叙事结构修正后旧出现关系保留且转 stale，依赖无法判定则由状态 readiness 返回 unavailable；
- 资产圣经按身份→状态输出当前版本、出现关系和服务端 readiness；分镜规格把 `asset_id/state_id/version_id/binding_source` 作为复合事实，跨状态/跨资产版本由数据库和服务双重拒绝；旧规格与已固定版本不随 current 漂移；
- 资产工作台可创建语义状态键、切换状态并为每个状态独立追加/选择版本；分镜工作台只展示 active 状态并固定显示“资产 · 状态”。浏览器验收发现归档会把资产排出生产圣经并误删恢复入口，Green 以独立归档身份卡修复生命周期，而非让归档资产继续进入生产列表；
- Alembic `6c1f8d4a7e20` 将 55 表前滚到 57 表；仓库外 0600 备份后，隔离恢复完成 `55 → 57 → 55 → 57`，开发库 19 行业务事实保持、当前新增资产表均为空，随后才升级正式本机开发库；head 与 autogenerate drift 均通过；
- 后端全量 `411 passed, 26 skipped`，Ruff/Pyright/17 个命名与 Plain Data Contract 架构门禁全过；前端全量 `24 files / 82 tests`、TypeScript、ESLint、生产构建通过，真实 PostgreSQL/MinIO/RabbitMQ 浏览器资产治理闭环通过；
- 完整证据见 [Acceptance 033](../acceptance/arrived/033-资产剧情状态与出现关系验收.md)。DEV-MVPA-07 在上述范围内 completed；PT-AST-006 仍保持 `in_progress`，因为状态编辑/禁用及其立即阻断契约按已接受设计由 DEV-MVPA-08 收口；改名/禁用影响预检、state-aware usage 批量治理和 Prompt 失效均未因本任务提前宣称完成。

DEV-MVPA-08 实现与验收证据（2026-08-13）：

- `Asset.status` 继续只表示 active/archived 身份生命周期，新增 `availability=enabled|disabled` 表示生产可用性；`AssetState.status` 的禁用语义独立。禁用立即让资产 readiness 和引用它的 Shot readiness 阻断，但 AssetVersion、ShotSpec、GenerationRequest、Task 与审计记录零删除；
- `AssetNameRevision` 只追加保存名称修订。名称不能再通过通用 PATCH 更新，必须走 rename preflight/apply；成功后稳定 ID/FK 不变、旧名称加入 alias，重复名称只返回告警；
- rename、Asset/State disable 和 State current 都先批量读取 Episode/Shot/Spec、GenerationRequest 输入快照与非终态 Task，再固定 `impact_hash`。Apply 在锁内重扫；任务状态、revision、引用或命令输入变化均以 409 零写入；
- 状态 label/description 使用独立 CAS/幂等命令，state_key 不可变。前端把元数据编辑、改名、停用、归档和当前版本切换拆成独立操作，高影响操作展示影响摘要后才允许确认；
- Alembic `36bf151da189` 将 57 张业务表前滚到 58 张。仓库外 0600 备份后，隔离库完成 `57 → 58 → 57 → 58`，随后才升级开发库；head 与 autogenerate drift 均通过；
- 完整证据见 [Acceptance 034](../acceptance/arrived/034-资产变更影响治理验收.md)。DEV-MVPA-08 与 PT-AST-007 completed；DEV-MVPA-07 留下的 PT-AST-006 状态编辑/禁用缺口同步关闭。C5 的 PromptRevision、OutputCandidate/Selection/Lineage 和真实 Provider 不在本任务内。

### 6.7 DEV-MVPA-09：拆镜/合镜守恒修复

分镜与覆盖共享 GitHub 证据池（固定于 2026-08-13）同时约束 DEV-MVPA-09/10/11：

| 候选 | 固定源码/测试与许可证 | 已证明的能力与缺口 | 准入决定 |
| --- | --- | --- | --- |
| ai-fusion-video `9dc3879`（v1.1.2） | [分镜集绑定 ScriptEpisode](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/entity/storyboard/StoryboardEpisode.java#L24-L51)、[稳定 StoryboardItem 与独立 sortOrder](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/entity/storyboard/StoryboardItem.java#L26-L49)、[绑定复用/整体清空测试](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/test/java/com/stonewu/fusion/service/storyboard/StoryboardServiceTests.java#L157-L235)；MIT | 能保证分镜集与剧本分集同属一个 Script，镜头 ID 与顺序分离；但关联只到可变 ScriptEpisode，不到 revision/unit，重生成 API 会清空该集场次和镜头 | 适配 identity≠order、跨父级拒绝；不采用 episode 粗粒度血缘和删除重建 |
| Jellyfish `a967819` | [Shot 仅保存 chapter/index/script_excerpt](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/models/studio_shots.py#L24-L68)、[AI division 新 UUID 写入](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/services/studio/script_division.py#L17-L73)、[已有镜头时拒绝覆盖测试](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/tests/test_script_division.py#L47-L109)；Apache-2.0 | AI 结构化结果与正式写入分离，已有镜头 fail closed；但 excerpt 没有 revision/span/unit 外键，每次写入生成新镜头 ID，不能证明覆盖守恒 | 适配候选先行和冲突拒绝；不采用 excerpt 作为叙事血缘 |
| LocalMiniDrama `05f90fb` | [Episode/Storyboard 表](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/backend-node/migrations/01_init.sql#L20-L60)、[整集旧分镜软删再生成](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/backend-node/src/services/episodeStoryboardService.js#L619-L746)、[流中断部分恢复](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/backend-node/src/services/episodeStoryboardService.js#L1069-L1096)；MIT | 增量落盘与中断恢复有工程价值；但镜头只关联 episode/scene/number，最终批次按镜号覆盖，完整重生成软删整集，未发现内容覆盖 oracle | 只吸收 partial/unknown 可见性测试；删除重建、镜号身份和“部分成功即完成”明确不采用 |
| ArcReel `ed71819` | [按稳定 resource_id 建分镜依赖计划](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/lib/storyboard_sequence.py#L16-L68)、[分段边界/上一镜依赖](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/lib/storyboard_sequence.py#L108-L208)、[序列测试](https://github.com/ArcReel/ArcReel/blob/ed71819aadf81015c0af4b3f5db0815607e04fae/tests/test_storyboard_sequence.py#L1-L170)；AGPL-3.0 | 选中项保持剧本顺序、segment break 切断连续依赖、资源版本历史成熟；但 ID 来自 JSON 剧本字段，没有不可变 revision/unit/reference/coverage report | 只吸收 dependency group 与 continuity warning 形态；AGPL 代码和文件 SSOT 不进入平台 |
| OpenTimelineIO `bc5fe2d` | [Timeline/Track/Clip/MediaReference 层级](https://github.com/AcademySoftwareFoundation/OpenTimelineIO/blob/bc5fe2d78dc3f8b2a8feb7e04483d85a12e80072/docs/tutorials/otio-timeline-structure.md)、[多 media reference 与 active key 原子测试](https://github.com/AcademySoftwareFoundation/OpenTimelineIO/blob/bc5fe2d78dc3f8b2a8feb7e04483d85a12e80072/tests/test_clip.py#L198-L295)；Apache-2.0，始于 2016 年 | 成熟地表达有序剪辑、时间范围和一个 Clip 的多个媒体引用；非法 active key 拒绝 | 只适配导出/互换和 selected-valid-member；OTIO 自身不提供数据库稳定身份或叙事覆盖，MVP 不引入 Timeline 依赖 |
| xStudio `d60b3e8` | [Timeline Item UUID/children/range](https://github.com/AcademySoftwareFoundation/xstudio/blob/d60b3e87fc52fb87b4d4e545e16dcd35471c567c/include/xstudio/timeline/item.hpp#L47-L148)、[Track/Clip/Gap/range 测试](https://github.com/AcademySoftwareFoundation/xstudio/blob/d60b3e87fc52fb87b4d4e545e16dcd35471c567c/src/timeline/test/timeline_test.cpp#L21-L96)；Apache-2.0 | UUID、顺序 children、时间 range 和 clone 是否保留 ID 是清晰不变量 | 适配稳定 Shot ID、顺序 CAS、拆合来源身份；不引入 C++ timeline/runtime |
| Olive `7e0e94a` | [split 将来源区间精确分成左右两段并移动尾部依赖](https://github.com/olive-editor/olive/blob/7e0e94abf6610026aebb9ddce8564c39522fac6e/app/timeline/timelineundosplit.cpp#L32-L95)、[多块拆分后重建 link](https://github.com/olive-editor/olive/blob/7e0e94abf6610026aebb9ddce8564c39522fac6e/app/timeline/timelineundosplit.cpp#L112-L164)；GPL-3.0 | 拆分点必须严格位于区间内部，左右长度之和等于来源；依赖移动、重连与反向撤销属于同一命令 | 适配“先完整构造两侧结果、再原子替换”和依赖守恒；不复制 GPL C++，不把编辑器 Undo 当数据库审计 |
| Pitivi `a4b19eb` | [split 由顶层 action-log 包裹并在一次 timeline commit 中处理](https://github.com/pitivi/pitivi/blob/a4b19eb9114e1d2eda8180fab4cf2eda45b6eed8/pitivi/timeline/timeline.py#L2379-L2417)、[连续 split 的逐步 undo/redo 测试](https://github.com/pitivi/pitivi/blob/a4b19eb9114e1d2eda8180fab4cf2eda45b6eed8/tests/test_undo_timeline.py#L449-L473)；LGPL-2.1-or-later | 批量操作只有有效内部切点才写入，提交、撤销和重做保持镜头数量确定 | 适配事务边界、无有效目标零写入与重复操作回归；不引入 GES/GTK/LGPL 运行时 |
| Doorstop `af3b671` | [link stamp/suspect/显式 clear](https://github.com/doorstop-dev/doorstop/blob/af3b671a1b93f605a61b9a17a8c2e025d7522a3b/doorstop/core/item.py#L527-L541)、[stamp 包含 UID/正文/引用](https://github.com/doorstop-dev/doorstop/blob/af3b671a1b93f605a61b9a17a8c2e025d7522a3b/doorstop/core/item.py#L863-L884)、[suspect/clear 回归](https://github.com/doorstop-dev/doorstop/blob/af3b671a1b93f605a61b9a17a8c2e025d7522a3b/doorstop/core/tests/test_item.py#L822-L992)；LGPL-3.0，跨年度 | 上游内容/引用 hash 变化让 link 变 suspect，只有显式 clear 才承认新基线；重复 link/unlink 幂等 | 适配 `coverage_input_hash → stale → 人工决议` 状态机与测试，不引入文件/YAML/filename UID 模型或 LGPL 依赖 |
| Hypothesis client `b4d085a` | [TextPosition/TextQuote selector](https://github.com/hypothesis/client/blob/b4d085a2f893aa6de3b61d8b8bc3ae4d0f24fc1a/src/annotator/anchoring/types.ts#L101-L215)、[exact→approximate→context 匹配](https://github.com/hypothesis/client/blob/b4d085a2f893aa6de3b61d8b8bc3ae4d0f24fc1a/src/annotator/anchoring/match-quote.ts#L17-L162)、[重复文本/上下文/失败测试](https://github.com/hypothesis/client/blob/b4d085a2f893aa6de3b61d8b8bc3ae4d0f24fc1a/src/annotator/anchoring/test/match-quote-test.js#L23-L216)；BSD-2-Clause 风格 | 位置 + exact/prefix/suffix 能在文本小改后重锚，重复 quote 需上下文消歧，低分必须失败 | 只复用 selector 结构与评测语料；MVP 正式引用必须固定 NarrativeUnitVersion，模糊匹配只能产人工确认候选，不能自动改 FK |

本地 delta 结论：现有稳定 `Shot`、不可变 `ShotSpecVersion`、order CAS、Transform、固定 `AssetVersion` 均保留；真实缺口是 `NarrativeUnitVersion`、`ShotNarrativeReference`、`CoverageDecision/Report` 和内容守恒 validator。当前前端 split 默认把全部 dialogue 给第一镜、merge 跨 Scene 只保留 base 侧；后端只验证子集而不要求两边并集等于来源，因此先修 DEV-MVPA-09，不允许 AI 草案把既有缺陷放大。

综合决定：没有领域项目可直接提供“剧本修订 → 稳定叙事单元 → 多对多镜头覆盖”实现。Lanverse 以当前数据库对象做最小内核，吸收 xStudio/OTIO 的 identity≠order/range、Doorstop 的依赖 hash suspect、Hypothesis 的锚点候选和领域项目的候选先行/冲突拒绝。AI 只能生成 DraftBatch 与 reference 候选；人工 Apply 后才写正式 Shot/Reference，CoverageReport 只从固定版本引用派生。上游证据子 Gate、黄金 fixture 和首轮迁移 Gate 已完成；DEV-MVPA-09/10/11 仍须等待各自上游并以本地 Red/revision 关闭生产 Green。三个 DEV 仍按 `3+5+4=12` 基准人周；研究工时已包含在原估算，成熟上游用于固定契约和减少返工，不冒充可直接安装的现成模块。

Red 必须先精确复现当前缺陷：

- split 全部 dialogue 留第一镜、第二镜为空；
- action beats 默认双边复制或未完整分配；
- merge `.slice(0, 8)` 截断；
- 跨 Scene merge 只保留 base 侧内容；
- 后端只校验子集而不校验并集。

Green：后端成为守恒最终事实源；split 要求两侧 action/dialogue 按来源顺序形成无遗漏、无重复的连续分区，局部 action order 可从 1 重排但稳定 beat_key 与内容不变；merge 允许为解决两镜局部 beat_key 冲突而确定性重编号，但动作描述、对白属性及 action-dialogue 关联必须按来源顺序完整同构。跨 Scene、重复 dialogue ID、超 8 beats/dialogues 或 15 秒直接拒绝；前端必须展示并提交明确的动作/对白分界，不再复制动作或把全部对白隐式留给第一镜。

退出：现有 PT-SBD-004 回归、并发/幂等/impact hash、浏览器 split/merge 全过，未引入 NarrativeUnit 时也不会静默丢内容。

完成证据（2026-08-13）：

- 后端新增单一 `conservation.py` 领域校验器。split 的两个目标必须对来源动作、对白 ID、对白属性和动作关联形成有序精确分区；merge 必须同剧本、同场次且在现有 15 秒、8 动作、8 对白表示上限内，目标动作、对白及重编号后的关联必须与两个来源完整同构。
- merge preflight 在产生影响摘要前拒绝跨场、重复对白、超时长或超容量；apply 在任何目标 Shot/Spec 写入前再次校验完整目标。现有幂等键、order/impact hash、不可变 Spec 和只追加 ShotTransform 均保留。
- 前端拆分要求显式动作/对白分界并把关联对白留在对应动作段；合并不再 `.slice(0, 8)` 或隐式丢弃另一侧，动作/对白按镜头顺序重建，资产引用合并去重，视觉/声音/生成意图由用户明确选择基础规格。
- 实施前固定并审阅 Olive `7e0e94a`、Pitivi `a4b19eb`、OpenTimelineIO `bc5fe2d`、xStudio `d60b3e8` 及三个 AI 短剧项目的源码、测试和许可证；本轮只采用事务、区间与失败分类，不复制 GPL/LGPL/AGPL 代码，也未新增运行依赖。
- 红灯先稳定复现后端缺少守恒模块、前端动作复制/跨场合并/对白映射错误；Green 后后端全量 `423 passed, 26 skipped`，Ruff/Pyright/pip check 通过，前端 `24 files / 87 tests`、TypeScript、生产构建通过，真实 PostgreSQL 浏览器分镜闭环 1/1 通过。
- 浏览器复核期间暴露测试夹具只删除 ORM 表、残留 `alembic_version` 的根缺陷；测试库现在清除全部表，`upgrade head` 后强制比较真实 schema。缺表且 revision 仍为 head 的新 Red 已转 Green，不增加兼容探测或自动修补业务库。
- 完整证据见 [Acceptance 035](../acceptance/arrived/035-分镜拆合内容守恒验收.md)。DEV-MVPA-09 与 PT-SBD-007 completed；NarrativeUnit 引用、AI DraftBatch、CoverageReport 和分镜包仍由 DEV-MVPA-10～12 承担。

### 6.8 DEV-MVPA-10：AI 分镜草案

DEV-MVPA-10 第二轮 GitHub 证据（固定于 2026-08-13）补充了候选审核、人工中断和影视发布三类横向方案；它们用于验证数据分层与失败边界，不替代 Lanverse 本地事务：

| 候选 | 固定源码/测试与许可证 | 已证明的能力与缺口 | 准入决定 |
| --- | --- | --- | --- |
| Label Studio `0205168` | [Prediction、Annotation、parent_prediction 分层](https://github.com/HumanSignal/label-studio/blob/0205168bf881dd99664dd9d7a97f615f8693e82f/label_studio/tasks/models.py#L659-L732)、[Prediction→Annotation 显式转换](https://github.com/HumanSignal/label-studio/blob/0205168bf881dd99664dd9d7a97f615f8693e82f/label_studio/data_manager/actions/predictions_to_annotations.py#L15-L58)、[转换后来源与摘要测试](https://github.com/HumanSignal/label-studio/blob/0205168bf881dd99664dd9d7a97f615f8693e82f/label_studio/tests/data_manager/actions/test_predictions_to_annotations.py#L45-L79)；Apache-2.0 | 模型预测与人工事实是不同实体，正式结果保留 parent prediction；但批量转换没有 Lanverse 的 input hash、order/spec CAS 和全批零写入契约 | 采用 DraftShot→Decision→Shot 的可追溯分层；不引入 Django/标注 DSL，也不把转换 action 当原子 Apply |
| LangGraph `644815f`（1.2.11） | [`interrupt`/`Command(resume)` 和 checkpoint 要求](https://github.com/langchain-ai/langgraph/blob/644815f9e5bc52ad8f7a5227a456227e9c3e639b/libs/langgraph/langgraph/types.py#L800-L871)、[节点从头重放语义](https://github.com/langchain-ai/langgraph/blob/644815f9e5bc52ad8f7a5227a456227e9c3e639b/libs/langgraph/langgraph/types.py#L851-L871)；MIT | HITL 能暂停、持久化和恢复，但恢复会从节点开头重执行，前置副作用必须自行幂等；当前流程已有 Task/Outbox/Worker 和显式人工 HTTP 命令 | 不引入 LangGraph 或第二套 checkpoint；吸收“中断前零正式副作用、重放必须幂等”的失败测试 |
| AYON Core `0c876b7` | [验证阶段可停止/错误阻断](https://github.com/ynput/ayon-core/blob/0c876b716a18a16e76a54dc81eecda4aff76b612/client/ayon_core/pipeline/publish/logic.py#L614-L648)、[Integrate 的文件事务](https://github.com/ynput/ayon-core/blob/0c876b716a18a16e76a54dc81eecda4aff76b612/client/ayon_core/plugins/publish/integrate.py#L95-L182)、[数据库先提交且 TODO 承认不能完整回滚](https://github.com/ynput/ayon-core/blob/0c876b716a18a16e76a54dc81eecda4aff76b612/client/ayon_core/plugins/publish/integrate.py#L162-L182)；Apache-2.0 | 成熟影视发布器明确区分 validate/integrate 并呈现可修复错误；但 DB 与文件传输不是一个可回滚事务 | 采用 preflight→apply 和用户可修复错误；拒绝其部分提交边界，MVP 草案 Apply 只写同一 PostgreSQL 事务 |
| Jellyfish `a967819` | [AI 结构化结果写镜头](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/app/services/studio/script_division.py#L17-L73)、[已有镜头时零写入拒绝](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/backend/tests/test_script_division.py#L47-L109)；Apache-2.0 | 先得到结构化结果、再统一 flush，已有镜头 fail closed；但没有持久 DraftBatch/人工 Decision，写入即正式 Shot | 吸收“已有事实不覆盖”和批量 flush 测试；AI 结果必须先持久为 DraftShot，不能直接调用正式写镜服务 |
| LocalMiniDrama `05f90fb` | [流式增量写正式分镜及最终覆盖](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/backend-node/src/services/episodeStoryboardService.js#L619-L746)、[连接中断把部分分镜当 truncated success](https://github.com/xuanyustudio/LocalMiniDrama/blob/05f90fb9ec21dea5753e324b673fc8a96bc6b2e0/backend-node/src/services/episodeStoryboardService.js#L1069-L1096)；MIT | 增量可见和断线恢复有用户价值；但失败时部分正式结果被视为成功、按镜号覆盖且旧分镜软删 | 作为失败反例：Provider 输出未完整校验前正式 Shot 写入必须为 0；unknown 不得转成部分成功 |
| ai-fusion-video `9dc3879` | [clearContent 删除全部分镜内容](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/service/storyboard/StoryboardService.java#L68-L81)、[分集绑定复用](https://github.com/Stonewuu/ai-fusion-video/blob/9dc387934d18b53f5b08b6b0c81d09edc315d5ae/ai-fusion-video/src/main/java/com/stonewu/fusion/service/storyboard/StoryboardService.java#L161-L208)；MIT | Spring 事务和父级归属检查成熟；但重生成仍提供整集删除内容入口，没有候选/决议/Apply 血缘 | 只吸收同 Episode/Script 归属和事务测试；整集清空、名称/序号匹配和可变正式行不采用 |

本地实现决定：不新增通用 Candidate/GenerationRun，不复用 extraction candidate，也不引入 LangGraph。新建职责明确的 `storyboards/drafts/` 能力包；物理事实是 Batch、固定 Unit 输入、固定 Asset 输入、DraftShot 和只追加 Decision，正式 Shot 只增加唯一 `source_draft_shot_id`。接口与类使用短业务语义名，仍受 64 字符和 Plain Data Contract 门禁约束。

MVP Apply 固定为 append-only：Batch 创建时保存 active Shot 的 order/spec/revision baseline；任何人工变化令 preflight 409。Apply diff 保留全部既有 Shot，只新增 `accepted/modified` 草案，`ignored` 不写入，modified/archived 数量显式为 0。这样真实保护人工镜头且不猜测跨 Batch 对应；自动更新/归档旧 AI 镜头留待稳定映射另行设计。

Red：Run 写正式 Shot、输入版本漂移、重复 Apply、人工锁定覆盖、order/spec 冲突、部分 Apply、无状态资产、Worker 重启和 unknown 先失败。

Green：

- DraftBatch 固定 Script/Narrative/AssetState/Version、目标时长和 Prompt/模型/schema；
- AI 只产 DraftShot；Decision 只追加；
- Apply preflight 返回新增/保留/修改/归档 diff；当前 append-only Apply 保留全部现有镜头且修改/归档为 0；
- Apply 在一个事务或可证明的原子命令中创建正式 Shot/Spec/References 并 CAS order；
- 分镜页区分 AI 草案和正式镜头，允许逐镜修改/忽略与整批确认。

退出：真实黄金单集生成 12–24 个草案，Apply 前正式写入 0，冲突时正式写入 0，成功重放只回读同一结果。

完成证据（2026-08-13）：

- 新增职责明确的 `storyboards/drafts/`：Batch 固定 current Script/Narrative/Asset 输入和正式镜头基线；DraftShot 与正式 Shot 分表；Decision 只追加；正式 Shot 以唯一 `source_draft_shot_id` 保留来源。
- 复用现有 Task/Outbox/Inbox/I/O Worker。Provider 成功只落草案；无 Provider、输入漂移、重复投递、处理中重放、failed 和 unknown 都有显式终态，未建立第二套工作流运行时。
- Provider 输出协议改为短字段和局部整数位置，adapter 确定性展开为固定 UUID；这是对真实 DeepSeek token 超限和身份幻觉根因的协议收敛，不是兼容解析器。服务端继续硬校验 required unit 覆盖、对白归属、固定资产、4–15 秒单镜、目标总时长和 12–24 镜。
- 前端在正式镜头上方独立呈现草案，支持选择 ready 状态资产、逐镜接受/修改/忽略、整批批准、preflight diff 和原子 Apply。当前 Apply 只追加，保留所有现有镜头，修改/归档数为 0。
- 实现遵循 DES-000 的语义化短文件名和 Plain Data Contract：跨模块输入为不可变 `dataclass(frozen=True, slots=True)`，Pydantic 只用于边界，未新增空泛 DTO、拼音、中文迁移名或兼容包装。
- 后端全量 `431 passed, 27 skipped`，Ruff/Pyright/pip check 通过；真实 DeepSeek 合约 `1 passed`；迁移测试 `26 passed` 且真实数据库为新 head、无模型漂移；前端 `25 files / 89 tests`、TypeScript、生产构建通过。
- 完整证据见 [Acceptance 036](../acceptance/arrived/036-AI分镜草案审核与原子应用验收.md)。DEV-MVPA-10 与 PT-SBD-008 completed；NarrativeReference、CoverageReport、分镜包和视频生成仍不在本验收范围。

### 6.9 DEV-MVPA-11：多对多覆盖与 readiness

DEV-MVPA-11 第二轮 GitHub 证据（固定于 2026-08-13）同时覆盖成熟需求追踪、成熟人工标注关系和短剧领域校验。成熟度与领域贴合度分开评价；新项目只能补充领域反例，不能单独承担平台架构正确性证明：

| 候选 | 固定源码/测试与许可证 | 已证明的能力与缺口 | 准入决定 |
| --- | --- | --- | --- |
| StrictDoc `a03c4ec` | [有类型的双向 many-to-many set](https://github.com/strictdoc-project/strictdoc/blob/a03c4ecb9a288558a01905d44782d9fcc6ffebd9/strictdoc/core/graph/many_to_many_set.py#L8-L108)、[一对多/反向查询/删除无残留/按边类型计数测试](https://github.com/strictdoc-project/strictdoc/blob/a03c4ecb9a288558a01905d44782d9fcc6ffebd9/tests/unit/strictdoc/core/test_many2many_set.py#L8-L96)；Apache-2.0，跨年度维护 | 独立关系边同时维护正反向索引、拒绝重复并区分边类型，适合验证“unit 找 shots / shot 找 units”；它是文档内存图，不提供数据库不可变版本、CAS、覆盖决议或生产 readiness | 采用独立关系身份、双向查询和类型化角色的不变量；不引入其文档模型或运行依赖 |
| Label Studio `f62571b` | [Relation 的独立 ID、双端引用、方向/标签和去重序列化](https://github.com/HumanSignal/label-studio/blob/f62571b6f0290f97afdc64f7bca0e6a10112cdf8/web/libs/editor/src/stores/RelationStore.js#L15-L227)、[新增、重复拒绝和双端查询测试](https://github.com/HumanSignal/label-studio/blob/f62571b6f0290f97afdc64f7bca0e6a10112cdf8/web/libs/editor/src/stores/__tests__/RelationStore.test.js#L169-L223)；Apache-2.0 | 成熟人工标注 UI 证明关系必须可按任一端选择、高亮、删除和显示标签；其 annotation 是可变前端状态，不固定 NarrativeUnitVersion/ShotSpecVersion，也没有 stale/coverage gate | 采用双向定位、高亮、关系标签和重复关系即时反馈；不复制 MobX store，不把客户端关系当事实源 |
| drama-skills `ca53b57` | [短剧 coverage ledger 模板](https://github.com/worldwonderer/drama-skills/blob/ca53b57452bd975cb3067f6343908a2f27ab4758/skills/short-drama-storyboard/assets/coverage-template.json#L1-L38)、[镜头稳定身份与拆合后 retired/successor 规则](https://github.com/worldwonderer/drama-skills/blob/ca53b57452bd975cb3067f6343908a2f27ab4758/skills/short-drama-storyboard/references/shot-revision-identity.md#L1-L29)、[覆盖守恒校验器](https://github.com/worldwonderer/drama-skills/blob/ca53b57452bd975cb3067f6343908a2f27ab4758/skills/short-drama-storyboard/scripts/storyboard_check.py#L84-L190)、[静默遗漏/错误总数/矛盾状态测试](https://github.com/worldwonderer/drama-skills/blob/ca53b57452bd975cb3067f6343908a2f27ab4758/skills/short-drama-storyboard/tests/test_structural_validators.py#L85-L177)；MIT | 领域上直接证明 covered/unresolved 必须形成全集、拆合不能按数组位置猜身份、被绑定记录变化才应 stale；但项目较新，事实源是文件/JSONL，没有事务、权限、数据库 FK 或并发证据 | 采用覆盖账本、显式省略、稳定镜头身份和窄失效测试分类；不把其文件工具或 JSON schema 作为平台运行时 |
| Doorstop `af3b671` | [链接 stamp/suspect/clear 与确认语义](https://github.com/doorstop-dev/doorstop/blob/af3b671a1b93f605a61b9a17a8c2e025d7522a3b/doorstop/core/item.py#L834-L932)、[UID/body/reference 哈希](https://github.com/doorstop-dev/doorstop/blob/af3b671a1b93f605a61b9a17a8c2e025d7522a3b/doorstop/core/item.py#L1087-L1112)；LGPL-3.0 | 成熟需求追踪证明 stale 关系必须显式确认而不是悄悄恢复 current；其 hash/link 绑定文件项，且 LGPL 运行依赖没有必要 | 延续现有 narrative dependency hash/显式确认行为；不引入依赖，不复制代码 |

本地 delta 与实现边界已经冻结：

- `NarrativeUnit/NarrativeUnitVersion` 已提供稳定逻辑身份和不可变表达，`Shot/ShotSpecVersion` 已提供稳定镜头身份、不可变规格、顺序 CAS、拆合与固定 AssetVersion；缺口只允许落在 `storyboards/coverage/`，不得复制上述事实或创建第二套 Scene/Shot/Task。
- `ShotNarrativeReference` 是独立不可变边，固定 `ShotSpecVersion + NarrativeUnitVersion`，携带 `channel/role/coverage_mode/segment/contribution/origin`。修正映射必须克隆一个新 Spec 并 CAS current；禁止 UPDATE/DELETE 旧边、按镜号/名称/文本相似度回填或给旧镜头增加“兼容默认映射”。
- `CoverageDecision` 只追加。批准省略固定 UnitVersion，批准创作性镜头固定 ShotSpecVersion；命令以当前 report evaluation hash 做 CAS，决议保存不含其他决议的 `basis_hash`，因此无关决议不会互相失效，底层 unit/spec/reference 变化仍会令旧决议 stale。
- `CoverageReport` 首期按需派生，不建可变 current 表：一次批量读取 current units、active shots/current specs、references 和 decisions，分类 `covered/approved_omitted/uncovered/orphan/stale`。required unit 必须 covered 或 approved omitted，orphan/stale 必须为 0；依赖读取失败返回 unavailable，不能降级成 ready。
- required channel 由叙事种类确定：动作与场景标题要求 visual，对白与旁白要求 audio，`both` 同时满足两路；partial 以 UnitVersion 内 Unicode code-point 半开区间表达，多段并集必须完整覆盖相应 required channel。重复 primary、越界/空 segment、跨 Episode/Workspace 和非 current 固定版本均 fail closed。
- AI 草案中的固定 unit IDs 是待人工接受或修改的来源建议，不按文本、镜号或顺序猜测。用户接受完整 DraftTarget 后，Apply 才把固定 UnitVersion 确定性投影为正式边：unit kind 决定 required channel，同一 `(unit, channel, full)` 的首个 current 镜头为 `primary/required`，重复镜头为 `supporting/supporting`；既有 current primary 也参与判定。未经过 DraftDecision 的建议、历史无来源镜头和空映射镜头仍保持 blocked，并可通过同一人工映射入口修正。
- split 在已有引用时必须由请求显式分配并证明引用并集守恒；merge 取两来源引用的有序并集并拒绝冲突 primary；copy 只复制为 supporting，不得重复满足 required coverage。任何路径都不静默丢失或重复必拍内容。
- 后端文件继续使用 `models.py/schemas.py/repository.py/service.py/api.py` 等目录已提供语义的短名，跨模块对象使用不可变 `Command/Query/Snapshot/Result` Plain Data Contract；受 DES-000 的 64 字符文件名、禁止含糊 DTO/兼容包装和架构测试硬门禁约束。

Red：一个 unit 多镜、一个镜多 unit、对白 audio/visual 分道、重复 primary、approved omission、orphan、旧 unit、依赖 unavailable、Spec/State 切换、36/120 镜 N+1 先失败。

Green：

- ShotNarrativeReference 固定 ShotSpecVersion 和 NarrativeUnitVersion，含 channel/role/segment/origin；
- CoverageDecision 只追加且固定 coverage hash；
- CoverageReport 是派生/可缓存事实，进入现有 Shot readiness 和 Project ProductionSnapshot；
- 增加 `SCRIPT_REVISION_NOT_CURRENT`、`NARRATIVE_REFERENCE_INVALID`、`COVERAGE_UNACCOUNTED`、`SHOT_SOURCE_ORPHAN`、`COVERAGE_DEPENDENCY_UNAVAILABLE`；
- 分镜页实现文本↔镜头双向定位和 uncovered/orphan/stale 总览。

退出：required 全 covered/approved omitted、orphan=0、stale=0 才 ready；36/120 镜保持既有 P95 门禁且无按镜 N+1。

完成证据（2026-08-14）：

- 新增不可变 `ShotNarrativeReference` 和只追加 `CoverageDecision`；关系固定 current `ShotSpecVersion + NarrativeUnitVersion`，数据库复合外键、边唯一约束、segment 约束和作用域校验共同阻止跨集、跨空间、越界与重复关系。
- `CoverageReport` 由 current Unit、active Shot/current Spec、引用和最新决议批量派生，区分 `covered/approved_omitted/uncovered` 与 `linked/approved_invented/orphan`，单独报告 stale；依赖异常返回 unavailable，readiness 与 ProductionSnapshot 均 fail closed。
- 手工保存规格必须提交完整 `narrative_references`，缺字段直接 422；修正映射、资产升级、AI Draft Apply、copy、split 和 merge 都在创建新不可变 Spec 时原子保存或守恒迁移关系，不保留旧请求别名、空默认或运行时兼容推断。
- 分镜页提供 coverage 总览、剧本单元↔镜头双向定位、关系编辑、省略/原创批准与撤销、stale 提示；拆分要求逐边明确分配，合并遇到重复边先由用户修正，浏览器闭环已覆盖该失败路径。
- 工程规范继续以 DES-000 为唯一事实源；新增架构门禁自动拒绝非 ASCII、超过 64 字符和空泛命名的源码/测试文件，Plain Data Contract 的 I/O/ORM/可变跨模块对象限制保持生效。
- 后端全量、36/120 镜性能、迁移、前端全量和浏览器验收的真实命令与结果见 [Acceptance 037](../acceptance/arrived/037-剧本分镜多对多覆盖验收.md)。DEV-MVPA-11 与 PT-SBD-009 completed；JSON/CSV/HTML 分镜包、MediaVersion/Lineage 和受控下载仍属于 DEV-MVPA-12。

### 6.10 DEV-MVPA-12：分镜包与联合 E2E

#### 6.10.1 上游成熟方案证据 Gate（2026-08-14）

本任务在编码前按固定 commit 检查了领域项目、交换格式和资产发布工程。下表中“采纳”只表示吸收已验证的规则；MVP 不直接复制它们的业务模型，也不新增运行时依赖。

| 候选 | 固定证据、许可证与测试 | 采纳 / 拒绝 |
| --- | --- | --- |
| LocalMiniDrama | commit [`7b6c1a7`](https://github.com/xuanyustudio/LocalMiniDrama/tree/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1)，MIT。[分镜表实现](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/frontweb/src/utils/exportStoryboardSheet.js#L1-L209)证明每镜一行、HTML 转义、CSV 引号和 UTF-8 BOM 的最小可用列；[项目 ZIP](https://github.com/xuanyustudio/LocalMiniDrama/blob/7b6c1a748e9e3013b88a902cfbfd31ec283da0d1/backend-node/src/services/dramaExportService.js#L13-L18)在文件丢失时返回 `null`，后续仍能报告导出成功。 | 采纳表格列、转义和 BOM；拒绝浏览器读 current、本地路径、缺件静默跳过和无历史 Manifest。 |
| wind-comic | commit [`c83e1cf`](https://github.com/ChrisChen667788/wind-comic/tree/c83e1cf5e9b88fa8ac62bb737c79985a95243b8d)，MIT。[纯函数预检](https://github.com/ChrisChen667788/wind-comic/blob/c83e1cf5e9b88fa8ac62bb737c79985a95243b8d/lib/publish-package.ts#L36-L77)会把缺件转为 `warnings/ready=false`，[测试](https://github.com/ChrisChen667788/wind-comic/blob/c83e1cf5e9b88fa8ac62bb737c79985a95243b8d/tests/v12-3-0-publish-package.test.ts#L16-L60)覆盖齐件、缺件和超限。 | 采纳无 I/O 的预检结果和结构化 blocker；拒绝 URL 作长期文件引用以及缺件时仍可产生“部分成功包”。 |
| dramai | commit [`2ec3810`](https://github.com/hyyyyyyz/dramai/tree/2ec38104380823aff711c96ed852d5f713b8ac5a)，Apache-2.0。[JSON 备份](https://github.com/hyyyyyyz/dramai/blob/2ec38104380823aff711c96ed852d5f713b8ac5a/src/core/export/json.ts#L4-L76)显式写入 format/version，[导入](https://github.com/hyyyyyyz/dramai/blob/2ec38104380823aff711c96ed852d5f713b8ac5a/src/core/export/json.ts#L99-L151)先验证版本再在 IndexedDB 事务中恢复；[剪映 alpha 包](https://github.com/hyyyyyyz/dramai/blob/2ec38104380823aff711c96ed852d5f713b8ac5a/src/core/export/jianying.ts#L7-L107)同时给机器 Manifest 和人类说明。 | 采纳格式标签、schema 版本和机器/人类双表示；拒绝从当前 IndexedDB 临时聚合和无哈希的文件引用。 |
| BagIt Python | commit [`4bd2713`](https://github.com/LibraryOfCongress/bagit-python/tree/4bd2713cedbe1f8e634567c20ef0dded9622011d)，CC0/Public Domain。[文档](https://github.com/LibraryOfCongress/bagit-python/blob/4bd2713cedbe1f8e634567c20ef0dded9622011d/README.rst#L67-L97)使用 manifest checksum 和完整性验证；[测试](https://github.com/LibraryOfCongress/bagit-python/blob/4bd2713cedbe1f8e634567c20ef0dded9622011d/test.py#L74-L135)明确区分字节翻转、缺失文件、快速完整性和全量 checksum，[缺件测试](https://github.com/LibraryOfCongress/bagit-python/blob/4bd2713cedbe1f8e634567c20ef0dded9622011d/test.py#L210-L311)会 fail closed。 | 采纳每个表示的 SHA-256、size 和缺件失败语义；拒绝完整 BagIt 目录/依赖，因 MVP 只需四个确定性 ZIP 成员。 |
| OpenTimelineIO | commit [`bc5fe2d`](https://github.com/AcademySoftwareFoundation/OpenTimelineIO/tree/bc5fe2d78dc3f8b2a8feb7e04483d85a12e80072)，Apache-2.0。[序列化测试](https://github.com/AcademySoftwareFoundation/OpenTimelineIO/blob/bc5fe2d78dc3f8b2a8feb7e04483d85a12e80072/tests/test_serialization.cpp#L24-L110)固定 schema label 和精确 JSON；[版本测试](https://github.com/AcademySoftwareFoundation/OpenTimelineIO/blob/bc5fe2d78dc3f8b2a8feb7e04483d85a12e80072/tests/test_version_manifest.py#L65-L129)验证降级和非法目标失败。 | 采纳 schema label、稳定序列化和明确版本拒绝；拒绝引入 Timeline/Track/Clip，因 MVP-A 不建专业时间线。 |
| AYON Core | commit [`0c876b7`](https://github.com/ynput/ayon-core/tree/0c876b716a18a16e76a54dc81eecda4aff76b612)，Apache-2.0。[发布集成](https://github.com/ynput/ayon-core/blob/0c876b716a18a16e76a54dc81eecda4aff76b612/client/ayon_core/plugins/publish/integrate.py#L95-L182)将注册版本、传输文件和注册 representation 分步；异常时可回滚文件事务，但源码 TODO 明确数据库改动无法同步回滚。 | 采纳“字节先落盘并验证，再公布 representation”；拒绝把文件回滚误当作跨数据库/对象存储原子性。 |

**本地差距与实施决策：**

- Lanverse 已有不可变 `ScriptVersion/NarrativeUnitVersion/ShotSpecVersion/AssetVersion`、派生 `CoverageReport`、私有 `MediaObject/MediaVersion/MediaLocation`、`Task + Outbox/Inbox` 和 `ObjectStoragePort.put/stat/stream/presign_download`；真实缺口是固定导出输入、确定性包、发布时序、`MediaLineage` 和受控历史，不是第二套 Task/存储/时间线。
- 提交事务只保存不可变 `ExportSnapshot`、`input_hash`、`StoryboardExportJob`、Task 和 Outbox；Worker 之后仅从该 Snapshot 渲染，不重读 current，因此重启/重试不重新决定输入。
- Worker 使用 Python 标准库生成确定性 ZIP：`manifest.json`、`storyboard.json`、`storyboard.csv`、`storyboard.html`；每个表示的 SHA-256/size 写入 Manifest。对象使用 Job ID 确定性键，`put` 后必须 `stat + stream sha256` 验证。
- 只有字节验证通过后，才在一个数据库事务中创建不可变 `StoryboardExportManifest`、通过 Media 公开契约登记 rendered delivery `MediaVersion/Location/Lineage`、标记 Job/Task 成功。崩溃在 `put` 与 DB commit 之间时，重放复用同一对象键并重验哈希；未 commit 时 API 不可见 available Manifest/Media。
- 语义命名与 Plain Data Contract 继续以 DES-000 为唯一事实源：`ExportSnapshot/RenderedMediaCommand/RenderedMediaResult` 只持有字段、类型和无 I/O 校验；`exports/{models,schemas,service,package,consumer}.py` 使用当前目录下最短的业务语义名，所有源码/测试文件名含扩展名不超过 64 个 ASCII 字符，不新增 DTO 别名、旧路径包装或双路径兼容。

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
| POST/GET | `/api/v1/episodes/{id}/storyboard-draft-batches`、`/api/v1/storyboard-draft-batches/{id}` | storyboards / MVPA-10 |
| POST | `/api/v1/storyboard-drafts/{id}/decisions`、`/api/v1/storyboard-draft-batches/{id}/{approve|apply-preflight|apply}` | storyboards / MVPA-10 |
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

- 未接受 MVP-A，或未提供可入库的自有/原创合成黄金候选并完成规定的内容复核；
- 目标数据库有不可丢失数据但没有备份、副本或 baseline 结构一致性证据；
- 发现新增事实需要跨模块 ORM、复制 Episode/Scene/Task/Candidate 或保存可变 current 快照；
- AI 结果需要自动发布、分镜 Apply 需要删除重建整集、split/merge 需要截断内容；
- 任一操作在依赖 unavailable、stale、blocked 时仍要显示 ready；
- 真实剧本、Key、数据库、对象存储数据、日志或生成产物将进入 Git；
- 为赶日期要求用 Mock 接受 DeepSeek/Provider、跳过 migration 或跳过并发/恢复证据；
- MVP-B 在图片 Provider Gate 未关闭时要求宣称真实生成成功。

## 14. 当前可领取任务

`DEV-MVPA-01～11` 已完成并由 Acceptance 028～037 和黄金 fixture 契约关闭。当前唯一可领取任务是 `DEV-MVPA-12`：先以 Red 固定导出读取漂移 current、coverage 过期、blocked asset、幂等冲突、对象写失败、Manifest/Media 部分提交、历史被后续改稿篡改和跨空间下载；再实现固定输入的 JSON/CSV/HTML 分镜包、MediaVersion/Lineage、历史与受控下载。不得提前实现图片/视频生成、剪辑时间线或商业平台能力。
