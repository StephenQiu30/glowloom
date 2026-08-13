# PLAN-012 AI 短剧 MVP 核心制作执行计划

- 状态：active（2026-08-13 用户明确要求开始；DEV-MVPA-01～04 已完成真实旧库迁移、工程黄金 fixture Gate、整剧导入、分集计划与原子批量物化；制作人/QA 内容质量复核保留到分镜/分镜包产品验收；下一任务为 DEV-MVPA-05）
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
2. 以真实历史 Metadata 和表结构快照人工校正 revision 边界：`95c0d24572c5` 只固定 Provider 引入前的 38 张业务表，`8d9f2a6c4b71` 再增加 4 张 Provider 表和 Capability 复合唯一约束，`4c8e2f7a9b31` 增加 4 张整剧文档/格式分析表并扩展 document Media，`7f3a9c1d2e84` 再增加 4 张分集计划/批量物化血缘表。全新环境独立执行 `alembic upgrade head`，集成验收必须走 migration。
3. 已有数据库不得直接 `stamp head`。adoption 只接受四种已知完整签名：当前 50 表结构直接采用 head，整剧文档 era 46 表结构采用 `4c8e2f7a9b31` 后升级，Provider-era 42 表结构采用 `8d9f2a6c4b71` 后升级，或历史 38 表结构采用 baseline 后原子升级；部分 Provider、部分整剧文档、部分 EpisodePlan 表和任何未知漂移都拒绝且不 stamp。
4. 新业务表按任务分 revision，不把 13 类核心实体压进一个不可回滚 revision。每个 revision 包含 upgrade、结构校验和可逆的 schema downgrade；涉及已写业务数据时，回滚优先恢复备份或前滚修复，不做破坏性自动 downgrade。
5. 每次 revision 在三条路径验证：空库到 head、当前 schema 快照到 head、含黄金样本旧库副本到 head；运行前后均核对行数、哈希、复合 FK、唯一约束和 current 指针。
6. 应用启动只检查数据库 revision 是否为允许版本，不能在 Web 进程自动 upgrade；部署前由独立受控命令执行升级。
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
| DEV-MVPA-05 | ready | PT-SCR-008 | 9 | MVPA-02 | 单集改写 Run、一个候选、diff/编辑/发布和失败恢复 |
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
| PostgreSQL 18 官方 | [`pg_dump` custom archive](https://www.postgresql.org/docs/18/app-pgdump.html)、[`pg_restore --single-transaction --exit-on-error`](https://www.postgresql.org/docs/18/app-pgrestore.html) | 直接采用可选择、可校验且单事务失败回滚的恢复链路 | 备份成功不代表可恢复；必须在由 `template0` 创建的隔离目标库验证数据与 revision |

本轮对标后的实现决定是：保留独立 upgrade、async connection sharing、显式 model registry、所有运行入口严格 head gate；旧库 adoption helper 先验证表、列、类型、默认值、约束和索引，并要求安全格式的备份引用，不允许把 `command.stamp` 暴露成通用快捷命令。自动生成 baseline 已人工补齐四条 `use_alter` 循环外键；命令成功仍不替代真实恢复演练。

DEV-MVPA-01 当前实现证据（2026-08-13）：

- DEV-MVPA-01 验收时空库从历史 38 表 baseline 升到当时的 42 表 head；DEV-MVPA-03 前滚到 46 表，DEV-MVPA-04 再前滚到当前 50 表；三个阶段的 `current --check-heads` 与 `alembic check` 均通过；
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

本地 delta 结论：现有 `Asset/AssetVersion/MediaReference/Consent`、Script extraction candidate、Shot 固定 AssetVersion、usage/upgrade preflight 均继续复用。C4 只新增 `AssetState/Occurrence`、state current、state-aware binding、rename/disable/version 影响治理和手工/上传 Version；`PromptRevision/GenerationRun/OutputCandidate/Selection/MediaLineage/Provider` 由共享 C5 独占，C4 只消费展示契约，禁止重复计费或另造候选体系。

综合决定：本轮没有候选可直接替换当前资产模块，也没有必要新增第三方运行时。实现只吸收五条已被成熟项目反复验证的不变量：稳定身份不随名称/顺序变化；状态/出现范围/媒体版本分层；主选必须属于可用候选；变更先用同一输入做 preflight 再 apply；历史引用固定版本且禁用只让派生 readiness 失效、不删除历史。该证据子 Gate、黄金 fixture 和首轮迁移 Gate 已完成；DEV-MVPA-07/08 仍须等待 MVPA-06，并用各自本地 Red/revision 证明复合一致性后才能进入 Green。证据没有减少本地迁移、UI 和回归工作，两个 DEV 仍按 `7+5=12` 基准人周；共享 C5 候选/媒体底座不得在这里重复相加。

Red：跨 Asset Version、状态同名、Occurrence 跨项目、current 竞态、禁用后仍 ready、改名破坏 FK、影响查询 N+1、陈旧 hash 和非终态任务竞态先失败。

Green：

- assets 增加 state/occurrence vertical package；AssetState current version 使用复合 FK 和 revision；
- Shot 新规格固定 `asset_id/state_id/version_id` 的复合一致性；旧 Spec 不回填漂移；
- create/link 状态建议仍是候选决定，AI 自动 merge 数为 0；
- 统一 rename/disable/version preflight+apply；Prompt 裸名称只作为需重编译快照；
- 资产页按身份→状态矩阵展示出现集、主版本、readiness 和 usage。

退出：角色常服/受伤、场景日/夜、道具完好/破损均能回溯 NarrativeUnit；改名零 FK 变化，disable 零历史删除。

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

- 未接受 MVP-A，或未提供可入库的自有/原创合成黄金候选并完成规定的内容复核；
- 目标数据库有不可丢失数据但没有备份、副本或 baseline 结构一致性证据；
- 发现新增事实需要跨模块 ORM、复制 Episode/Scene/Task/Candidate 或保存可变 current 快照；
- AI 结果需要自动发布、分镜 Apply 需要删除重建整集、split/merge 需要截断内容；
- 任一操作在依赖 unavailable、stale、blocked 时仍要显示 ready；
- 真实剧本、Key、数据库、对象存储数据、日志或生成产物将进入 Git；
- 为赶日期要求用 Mock 接受 DeepSeek/Provider、跳过 migration 或跳过并发/恢复证据；
- MVP-B 在图片 Provider Gate 未关闭时要求宣称真实生成成功。

## 14. 当前可领取任务

`DEV-MVPA-01～04` 已完成，下一任务为 `DEV-MVPA-05`。开始改写实现前，必须先按 G-MVPA-006 为“受约束剧本改写、diff/人工编辑、追加 Version 与 current CAS”重新建立固定 commit/许可证/失败测试证据池，再提交覆盖原稿、输入漂移、无 Key、非法结构化输出、unknown 重投和并发发布 Red。

本计划已由用户激活；`DEV-MVPA-05` 只实现 AdaptationRun 的最小纵向闭环，不预建 NarrativeUnit、AssetState 或 StoryboardDraft 空目录，不让 AI 候选直接覆盖 current，也不因 PT-SCR-007 已接受而冒充剧本改写已经完成。
