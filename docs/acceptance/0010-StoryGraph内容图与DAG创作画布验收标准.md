# StoryGraph 内容图与 DAG 创作画布验收标准

- 状态：验收标准已接受（`SG-D21`，2026-08-27）；全部标准初始未通过
- Design：[0010 StoryGraph 内容图与 DAG 创作画布设计](../design/0010-StoryGraph内容图与DAG创作画布设计.md)
- Agent Design：[3003 StoryGraph 剧本解析 Harness 与内置 Skill 设计](../design/3003-StoryGraph剧本解析Harness与内置Skill设计.md)
- PRD：[0010 StoryGraph 内容图与 DAG 创作画布产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md)
- Cross-service Requirement：[0010 StoryGraph 内容图与 DAG 创作画布需求规格](../requirement/0010-StoryGraph内容图与DAG创作画布需求规格.md)
- Agent Requirement：[3003 StoryGraph 剧本解析 Harness 与内置 Skill 需求规格](../requirement/3003-StoryGraph剧本解析Harness与内置Skill需求规格.md)
- Plan：[0010 StoryGraph 内容图与 DAG 创作画布实施计划](../plan/0010-StoryGraph内容图与DAG创作画布实施计划.md)

## 1. 证据口径

本文件只承认 `SG-D21` 接受后、由对应 `SG-Ixx` 在当前源码重新执行的证据。历史实现、旧 Acceptance、mock、内存替身、跳过项、被禁用的 CI、兼容回退、静态推断或“理论可行”均不能勾选。

每次勾选必须在本文件追加：实施任务与提交、真实命令、fixture/完整原稿、依赖版本与隔离方式、关键计数/Hash/Receipt、成功或失败结果、残余风险。涉及外部依赖时必须记录真实 PostgreSQL/GORM、Temporal、Kafka、Elasticsearch、ELK、MinIO、Codex CLI 或 Runware Staging；缺失条件保持 `[ ]`。

统一验收顺序为定向 Red→Green→Refactor → 架构/安全门禁 → 当时全量真实 CI → 独立提交。`agent-browser` 只能由 `SG-I28` 在 `SG-I27` 完成并提交后执行，不能提前替代任何非浏览器证据。

## 2. Cross-service Requirement Checklist

### 2.1 架构与事实源

- [x] `SG-ARC-001`（`SG-I01`、`SG-I03`–`028` 回归）：Backend 唯一业务 Writer，Agent/Frontend/event-worker 零 Owner 直写证据。
- [x] `SG-ARC-002`（`SG-I01`、`SG-I03` 起）：空 PostgreSQL 的唯一 GORM Catalog、无 Migration 元数据/Raw SQL/第二 ORM/第二 Writer 证据。
- [x] `SG-ARC-003`（`SG-I01` 起）：Domain/Application 与 GORM/Temporal/Kafka/Elastic/Provider 的 import/AST 负向证据。
- [ ] `SG-ARC-004`（`SG-I01`、`SG-I06`–`024`）：真实 Temporal wait/signal/replay/restart 与无 Kafka/DB 第二 Workflow 证据。
- [x] `SG-ARC-005`（`SG-I01`、`SG-I03`、`SG-I25`）：四图 Schema/ID/DTO 不可互换证据。
- [ ] `SG-ARC-006`（`SG-I01`、`SG-I05`、全部 Agent 切片）：Agent Candidate-only 环境、依赖、网络和零业务写入证据。
- [ ] `SG-ARC-007`（`SG-I07`、`SG-I25`–`026`、`SG-I28`）：Browser 仅访问 Backend `/api/v1` 的 bundle/import/网络证据。
- [ ] `SG-ARC-008`（`SG-I03`–`024`）：外部边界稳定身份、known failure/unknown 与同 ID 对账故障矩阵。
- [ ] `SG-ARC-009`（`SG-I03`–`026`）：Workspace/Project Token Version/Membership 与防枚举权限矩阵。
- [x] `SG-ARC-010`（所有首次消费者任务）：新增结构均有真实消费者、测试和装配，无未来空层/Binary/Topic/Index/兼容层证据。

### 2.2 StoryGraph 与 Query

- [x] `SG-GRF-001`（`SG-I01`、`SG-I03`）：StoryGraphVersion 字段、不可变约束和历史读取证据。
- [x] `SG-GRF-002`（`SG-I03`）：每项目唯一 Head、expected CAS、并发单胜与失败零半成品证据。
- [x] `SG-GRF-003`（`SG-I01`、`SG-I03`、`SG-I17`）：Node/Owner Ref/Evidence strict schema 正反 fixture。
- [x] `SG-GRF-004`（`SG-I01`、`SG-I03`、`SG-I17`）：Edge 类型/端点矩阵与未知组合拒绝 fixture。
- [x] `SG-GRF-005`（`SG-I01`、`SG-I03`、`SG-I04`）：Node/Edge 稳定 Key 与跨版本 change 证据。
- [x] `SG-GRF-006`（`SG-I01`、`SG-I03`）：Canonical Hash 跨语言、随机顺序和 Canvas 无关证据。
- [x] `SG-GRF-007`（`SG-I01`、`SG-I03`、`SG-I17`）：DAG、最小环路径与 Claim 表达证据。
- [x] `SG-GRF-008`（`SG-I03`、`SG-I17`、`SG-I23`）：Compiler 只读已确认精确 Owner、污染源拒绝证据。
- [x] `SG-GRF-009`（`SG-I03`、`SG-I17`、`SG-I23`）：Owner Set/Version/Head/Receipt/Outbox 单 GORM 事务故障注入。
- [x] `SG-GRF-010`（`SG-I03`）：JSONB Version + Head 且无图数据库/EAV/递归 Raw SQL/第二 Graph Writer 证据。
- [x] `SG-QRY-001`（`SG-I04`）：Current/Exact/Lens/Diff/Trace/Impact Application 与 HTTP Query 证据。
- [ ] `SG-QRY-002`（`SG-I04`、`SG-I25`）：版本、lens、scope、depth/cursor、truncated/继续条件 contract。
- [ ] `SG-QRY-003`（`SG-I04`、`SG-I25`）：五类有界 Lens、大图分层和确定性结果 Hash。
- [x] `SG-QRY-004`（`SG-I04`、`SG-I17`）：稳定 Key 的 add/remove/change 跨版本 golden。
- [x] `SG-QRY-005`（`SG-I04`）：Query 零写入与 Elasticsearch 故障不影响 PostgreSQL Query 的证据。

### 2.3 Review、Production 与视觉资产

- [x] `SG-REV-001`（`SG-I06`）：Gate input 建立冻结 HumanTask 且客户端不能自报 Candidate。
- [x] `SG-REV-002`（`SG-I06`、`SG-I07`）：列表/详情/Lease/Decision/Resume OpenAPI 与生成 Client 无漂移。
- [x] `SG-REV-003`（`SG-I06`、`SG-I07`）：Claim/Renew/Release Actor/revision/token/expiry/幂等与零泄漏矩阵。
- [x] `SG-REV-004`（`SG-I06`）：不可变 Decision、允许集合与 selected 单候选并发证据。
- [x] `SG-REV-005`（`SG-I06`、`SG-I07`）：Decision/Owner Apply/Workflow Resume 三状态 API/UI 分离证据。
- [ ] `SG-REV-006`（`SG-I11`、`SG-I14`、`SG-I16`、`SG-I19`、`SG-I21`、`SG-I23`、`SG-I24`）：七类 Gate 的显式 Owner Apply 与负向零写入证据。
- [ ] `SG-REV-007`（同上）：按 Decision ID 幂等 Resume、并发/重启/UNKNOWN 收敛证据。
- [ ] `SG-REV-008`（同上）：Decision 前 stale 与 Decision 后 baseline 冲突不误套用证据。
- [x] `SG-PRD-001`（`SG-I08`）：DocumentRevision、Unicode 绝对 Evidence 与两集 coverage。
- [ ] `SG-PRD-002`（`SG-I11`）：Bible Confirm 只产 Version/Receipt 的数据库事实计数。
- [ ] `SG-PRD-003`（`SG-I12`）：MaterializeConfirmedBible 单事务、唯一身份、幂等/回滚/反查。
- [ ] `SG-PRD-004`（`SG-I09`、`SG-I12`、`SG-I15`）：同名/别名不得自动合并的负向证据。
- [ ] `SG-PRD-005`（`SG-I13`、`SG-I14`）：分集边界与 Episode/Published ScriptVersion 全批原子证据。
- [ ] `SG-PRD-006`（`SG-I15`、`SG-I16`）：Scene/Dialogue/Beat/Occurrence/Claim 全批应用与未知事实拒绝。
- [ ] `SG-PRD-007`（`SG-I18`）：Storyboard Draft 精确正式输入、`needs_asset` 与零 Shot 证据。
- [ ] `SG-PRD-008`（`SG-I19`）：FreezeIntentSet 输出与零 Shot/Cost/Quota/Provider/Graph 副作用证据。
- [ ] `SG-PRD-009`（`SG-I22`）：detail_shots 精确 READY AssetVersion/Artifact/Lineage/Style/view-role 门禁。
- [ ] `SG-PRD-010`（`SG-I23`）：Shot + ShotProductionBindingVersion + Receipt 全批 GORM 原子应用。
- [ ] `SG-PRD-011`（`SG-I12`、`SG-I23`、`SG-I24`）：三类 Binding Owner/Record/API/Graph 不混用证据。
- [ ] `SG-PRD-012`（`SG-I17`、`SG-I23`）：Owner Apply 后 expected graph hash 编译与 unknown 同 Receipt 对账。
- [ ] `SG-VIS-001`（`SG-I20`、`SG-I24`）：`reference_asset|shot_frame` strict union 与旧入口拒绝。
- [ ] `SG-VIS-002`（`SG-I20`）：reference_asset 冻结身份/Specification/State/Style/view roles 与先行顺序。
- [ ] `SG-VIS-003`（`SG-I20`、`SG-I24`）：Runware 稳定 Task UUID 与同 Job unknown 查询对账。
- [ ] `SG-VIS-004`（`SG-I20`、`SG-I24`）：Provider 前 Intent/Cost/Quota/Authorization/Job 全部已提交。
- [ ] `SG-VIS-005`（`SG-I20`、`SG-I24`）：私有 Staging、Backend 字节/Hash/媒体重验与 READY/QUARANTINED。
- [ ] `SG-VIS-006`（`SG-I21`）：composite 三视图 role/输入 Hash/READY/lineage QC 门禁。
- [ ] `SG-VIS-007`（`SG-I21`）：确定性 QC 与人工视觉一致性判断边界。
- [ ] `SG-VIS-008`（`SG-I21`、`SG-I24`）：冻结 CandidateSet 的单一不可变 Selection 并发收敛。
- [ ] `SG-VIS-009`（`SG-I21`）：AssetVersion 精确绑定、不可变与历史版本引用。
- [ ] `SG-VIS-010`（`SG-I24`）：shot_frame 冻结正式 Shot/ProductionBinding/Occurrence/AssetVersion 完整性。
- [ ] `SG-VIS-011`（`SG-I24`）：只发布 ShotImageBindingVersion、单 Shot 局部重跑不改旧事实。

### 2.4 Kafka、Search 与 ELK

- [x] `SG-EVT-001`（`SG-I03`、`SG-I04`）：Owner/Receipt/Outbox 同 GORM 事务且网络 Publisher 不入事务。
- [x] `SG-EVT-002`（`SG-I04`）：Kafka Envelope 完整字段、payload hash 与剧本/Prompt/Secret/URL 排除。
- [x] `SG-EVT-003`（`SG-I04`）：真实 Kafka 至少一次、ACK unknown 同 ID、Inbox/revision fencing、DLQ/Replay。
- [x] `SG-EVT-004`（`SG-I04`）：Script/StoryGraph Topic 与日志 Topic 的 Schema/Group/Retention/DLQ 隔离，无 Command Topic。
- [x] `SG-EVT-005`（`SG-I04`）：首个真实 Consumer 同任务创建 event-worker，复用唯一 Catalog/连接模型。
- [x] `SG-SRCH-001`（`SG-I04`）：Script/StoryGraph 两类 index/alias、租户/Owner/Evidence/Node 可追溯文档。
- [x] `SG-SRCH-002`（`SG-I04`）：重复/乱序 revision fencing、tombstone/snapshot、PostgreSQL 全量 Reindex 原子 Alias。
- [x] `SG-SRCH-003`（`SG-I04`）：授权 Search API、snippet/score/深链/新鲜度且无 DSL 透传/Owner 回写。
- [x] `SG-SRCH-004`（`SG-I04`）：Elastic unavailable/lag 的 degraded/stale 与 Owner/PostgreSQL Query 正确性。
- [x] `SG-LOG-001`（`SG-I04`）：真实 `Filebeat → Kafka → Logstash → Elasticsearch → Kibana` 独立日志链。
- [x] `SG-LOG-002`（`SG-I04`、`SG-I27`）：全链关联 ID/错误码查询与敏感字段零命中扫描。
- [x] `SG-LOG-003`（`SG-I04`、`SG-I27`）：Kafka/Logstash/Elastic/Kibana 逐组件故障不改变业务/Search 投影证据。

### 2.5 Frontend、运行质量与旅程

- [ ] `SG-FE-001`（`SG-I07`、`SG-I25`–`026`）：单 Next.js/npm + RTK Query + 生成 Client，无第二 Query/monorepo。
- [ ] `SG-FE-002`（`SG-I12`、`SG-I16`、`SG-I21`、`SG-I25`）：角色/地点卡派生 Owner 事实、State/Style 不重复身份与 Scene 区分。
- [x] `SG-FE-003`（`SG-I07`）：真实 Review Workbench，错误/unknown 无 mock/local success。
- [ ] `SG-FE-004`（`SG-I25`）：StoryGraph 路由、五 Lens、scope/version/focus 深链刷新。
- [ ] `SG-FE-005`（`SG-I25`）：React Flow/Dagre 随真实消费者引入，只读且布局/viewport 零回写。
- [ ] `SG-FE-006`（`SG-I25`）：Story Lens 与 Workflow Lens Query/DTO/ID/Adapter/renderer 分离。
- [ ] `SG-FE-007`（`SG-I25`）：Server fact/URL/local 状态边界，View Model 不复制到 Redux/Context/localStorage。
- [ ] `SG-FE-008`（`SG-I07`、`SG-I25`–`026`）：完整状态、键盘、焦点、列表替代与 reduced-motion。
- [ ] `SG-FE-009`（`SG-I26`）：类型化 Domain Intent + base/expected/idempotency，Owner Command 重编译且无 Graph JSON 直写。
- [ ] `SG-OPS-001`（`SG-I01` 起）：严格输入/事件/Provider/HTTP 解码、大小/深度/数字/UUID/Hash 负向证据。
- [ ] `SG-OPS-002`（`SG-I20`、`SG-I24`）：Runware SSRF/allowlist/Credential Ref 与 secret 零泄漏。
- [x] `SG-OPS-003`（各 Binary 首次消费者，`SG-I04` event-worker）：healthz/readyz 与真实必要依赖故障。
- [x] `SG-OPS-004`（`SG-I01` 起）：所有测试只在三应用 `tests/`，业务源码零测试文件。
- [ ] `SG-OPS-005`（每个 `SG-Ixx`）：Red→Green→Refactor、定向门与当时全量真实 CI 证据。
- [x] `SG-OPS-006`（`SG-I01`、`SG-I04` 起）：空 PostgreSQL、真实 Temporal/MinIO/Kafka/Elastic/日志链 CI。
- [ ] `SG-OPS-007`（`SG-I01` 起）：Go/Agent/Frontend/OpenAPI/Compose/Image/Hygiene 与 Required 聚合真实执行。
- [ ] `SG-OPS-008`（每个 `SG-Ixx`）：完整任务独立提交且无兼容回退/禁用检查/降断言/旧入口。
- [ ] `SG-OPS-009`（`SG-I27`、`SG-I28`）：完整实现与 CI 提交后才运行最终 agent-browser 的 Git 顺序。
- [ ] `SG-JRN-001`（`SG-I08`–`017`、`SG-I27`）：至少两集 Document→Evidence→Bible→Episode/Scene/Beat/Occurrence/Claim→Core Graph 可反查。
- [ ] `SG-JRN-002`（`SG-I12`、`SG-I18`–`024`、`SG-I27`）：跨集单 Asset、多 State、reference/detail/Shot/frame/Binding/Graph 全链。
- [x] `SG-JRN-003`（`SG-I03`–`004`、`SG-I27`）：Outbox→Kafka→Elastic→Search 深链与重复/乱序/重启/Reindex 收敛。
- [ ] `SG-JRN-004`（`SG-I27`）：完整原稿统计、人工抽查与全故障矩阵零重复事实/费用/配额。
- [ ] `SG-JRN-005`（`SG-I28`）：最终 agent-browser 旅程与 API/PostgreSQL/Temporal/Kafka/Search/Artifact 对账。

## 3. Agent Requirement Checklist

### 3.1 边界、迁移与 Bundle

- [x] `SGA-BND-001`（`SG-I01`、`SG-I05` 起）：Backend 拥有 Stage/Policy/Invocation/Shard/Candidate 与全部写入。
- [x] `SGA-BND-002`（`SG-I01`、`SG-I05` 起）：Agent 无 ORM/DB/Object/Kafka/Elastic/Temporal/Provider/Public API。
- [x] `SGA-BND-003`（`SG-I08` 起）：Stage/Shard 挂既有 Run/NodeRun，无动态 Workflow Node/Agent Checkpoint。
- [ ] `SGA-BND-004`（全部 Agent Stage）：Agent success 零 Confirm/Apply/正式 UUID/Owner/Event/Resume。
- [x] `SGA-BND-005`（`SG-I05`）：普通显式 Registry，无 LangGraph 运行路径且无消费者时删除依赖/lock。
- [x] `SGA-MOV-001`（`SG-I02`）：八 Skill 原名、原 UTF-8 字节、相对路径 SHA-256 等价迁移。
- [x] `SGA-MOV-002`（`SG-I02`）：Loader/Docker/tests 原子切换，根旧路径删除且无双读/fallback。
- [x] `SGA-MOV-003`（`SG-I02`）：只迁移不改行为，Agent/Backend/Frontend 全量 CI 通过。
- [x] `SGA-BDL-001`（`SG-I05`）：最终唯一 `agent/skills/build-storygraph/SKILL.md`，旧名/旧 Loader/无消费者 metadata 删除。
- [x] `SGA-BDL-002`（`SG-I05`）：SKILL 全局规则 + 显式 references，Python 无 Guidance 复制。
- [x] `SGA-BDL-003`（`SG-I05`）：Stage→Schema/Reference 显式 Registry、loaded-file golden、未知 Stage 拒绝。
- [x] `SGA-BDL-004`（`SG-I05`）：Bundle Canonical Hash 跨语言、路径/长度/字节与逃逸 fail closed。
- [x] `SGA-BDL-005`（`SG-I05`）：Manifest 冻结版本/Hash/模型/空 Tool/budget/deadline，任一漂移拒绝。
- [x] `SGA-BDL-006`（`SG-I05`）：未终态 Invocation 按 Bundle Hash 精确镜像路由，缺失无相近版本回退。

### 3.2 Wire、Stage 与 Shard

- [x] `SGA-WIR-001`（`SG-I01` fixture、`SG-I05` 最终）：只允许 `storygraph_stage`，旧 kind 原子移除。
- [x] `SGA-WIR-002`（`SG-I01`、`SG-I05`）：Invocation 全字段 strict fixture。
- [x] `SGA-WIR-003`（`SG-I01`、`SG-I05`）：source/upstream exact ref 完整且无 current/latest 补全。
- [x] `SGA-WIR-004`（`SG-I01`、`SG-I05`）：Input Hash 跨语言与每字段突变 golden。
- [x] `SGA-WIR-005`（`SG-I01`、`SG-I05`）：stage instance identity 并发/重放/结果冲突。
- [x] `SGA-WIR-006`（`SG-I01`、`SG-I05`）：succeeded/failed/unknown strict union 与 `extra=forbid`。
- [x] `SGA-WIR-007`（`SG-I01`、`SG-I05`）：Result Hash、Backend 不可变接受与全身份重验。
- [x] `SGA-WIR-008`（`SG-I01`、`SG-I05`）：Grant expiry/attempt/fencing/恒时验签与伪造拒绝。
- [x] `SGA-STG-001`（`SG-I05`）：十 Stage/Reference/Candidate 一一对应与跨语言 count=10。
- [x] `SGA-STG-002`（`SG-I05`）：Pydantic 唯一 Schema 事实、临时生成 JSON Schema、无 tracked 第二份。
- [x] `SGA-STG-003`（`SG-I05` 起）：Candidate 只用给定 Ref/临时 Key，无 Command/SQL/Graph overwrite。
- [ ] `SGA-STG-004`（`SG-I08`–`010`、`SG-I13`、`SG-I15`、`SG-I18`、`SG-I22`）：无证据不补写事实的对抗/人工证据。
- [ ] `SGA-STG-005`（`SG-I09`、`SG-I15`）：关系/因果/连续性/伏笔使用 Claim Candidate，无持久环边。
- [ ] `SGA-STG-006`（`SG-I18`）：draft_storyboard 非空 Spec/State、缺资产只产 needs_asset。
- [ ] `SGA-STG-007`（`SG-I22`）：detail_shots 精确 READY 资产并禁止改变已接受意图/身份/状态。
- [ ] `SGA-STG-008`（`SG-I18`、`SG-I22`、`SG-I23`）：Backend 生成序号/timecode/UUID/Owner/Binding，Agent 只给 Candidate。
- [x] `SGA-SHR-001`（`SG-I08`）：不可变 versioned ShardManifest 字段/Hash/约束。
- [x] `SGA-SHR-002`（`SG-I08`、`SG-I09`、`SG-I15`）：确定性分片/排序/fan-in/tree，Agent 不决定边界。
- [x] `SGA-SHR-003`（`SG-I08` 起）：超预算发布新 Manifest 完整覆盖，无截断/临时扩预算。
- [x] `SGA-SHR-004`（`SG-I08` 起）：旧结果只审计，current active leaf + gate 才聚合。
- [ ] `SGA-SHR-005`（`SG-I09`、`SG-I15`）：有界 reduce 只传必要 Ref/Hash/冲突，超预算再分片。
- [ ] `SGA-SHR-006`（全部分片 Stage）：单 shard 失败不毁成功事实，Workflow 无固定业务墙钟终止。

### 3.3 Evidence、Candidate 与 Repair

- [x] `SGA-EVD-001`（`SG-I08`）：Unicode code-point `[start,end)` 跨语言与逐字回读。
- [x] `SGA-EVD-002`（`SG-I08`）：语义边界、显式 overlap、coverage 与 range+hash 去重。
- [x] `SGA-EVD-003`（`SG-I08`、`SG-I13`）：中阿拉伯 Episode marker 与 AI 仅提议歧义边界。
- [x] `SGA-EVD-004`（`SG-I08`）：chunk-local offset 经 Backend 校正重验后才成正式 Evidence。
- [ ] `SGA-EVD-005`（`SG-I08` fixture、`SG-I27` final）：两集开发 + 完整原稿统计和代表集人工细查。
- [ ] `SGA-CAN-001`（`SG-I09`）：不可变 StageCandidateRevision/Head CAS/并发。
- [ ] `SGA-CAN-002`（`SG-I09`）：invocation/aggregate/repair strict origin union。
- [ ] `SGA-CAN-003`（`SG-I09`）：content hash 与 revision hash 分层单字段突变。
- [ ] `SGA-CAN-004`（`SG-I09`、`SG-I10`）：exact revision 下游与 Head 变更 stale closure，不覆盖历史。
- [ ] `SGA-REP-001`（`SG-I10`、`SG-I22`）：模型 Review Issue 不冒充确定性 Gate/blocker。
- [ ] `SGA-REP-002`（`SG-I10`、`SG-I22`）：Repair Patch 冻结 target/allowlist/base/邻接且不能改已发布 Graph。
- [ ] `SGA-REP-003`（`SG-I10`、`SG-I22`）：expected Head 应用 N+1、幂等 Receipt 与并发单胜。
- [ ] `SGA-REP-004`（`SG-I10`、`SG-I22`）：每轮重跑影响闭包 Gate/Review，有界预算耗尽不半成功。

### 3.4 Codex、错误、CI 与旅程

- [ ] `SGA-COD-001`（`SG-I05` 起、`SG-I27`）：真实本地 Codex ephemeral/read-only/ignore config/临时空目录与 Policy 模型。
- [ ] `SGA-COD-002`（同上）：Tool allowlist 为空，任何 Tool event 丢弃 Candidate 并报错。
- [ ] `SGA-COD-003`（同上）：Harness 显式注入 Guidance，工作目录与用户 Skill/项目配置隔离。
- [ ] `SGA-COD-004`（同上）：模型调用/技术 deadline、进程回收与脱敏有界诊断。
- [ ] `SGA-COD-005`（同上）：Schema 修正固定次数计入预算，事实 blocker 不被改写。
- [ ] `SGA-COD-006`（同上）：runtime unavailable/transport unknown 不空 Candidate 成功、不换 Provider。
- [x] `SGA-ERR-001`（`SG-I05` 起）：稳定错误码 Go/Python fixture 完整。
- [x] `SGA-ERR-002`（同上）：failed/unknown 可重试分类与同 identity 对账事实计数。
- [ ] `SGA-ERR-003`（`SG-I04`、全部 Agent 切片）：允许日志字段与剧本/Candidate/Prompt/Grant/Secret 零命中。
- [ ] `SGA-ERR-004`（同上）：Backend→Agent→Codex trace 关联且 Candidate Hash 不变。
- [x] `SGA-OPS-001`（`SG-I01` 起）：Agent 测试只在 `agent/tests` 独立分类。
- [x] `SGA-OPS-002`（每个 Agent 任务）：Ruff check/format、Pyright、Pytest 与 Go/Python fixture 同时通过。
- [x] `SGA-OPS-003`（`SG-I02`、`SG-I05`）：非 root 镜像含唯一 Bundle/Codex，旧路径不存在且启动 Hash 验证。
- [ ] `SGA-OPS-004`（`SG-I08`–`010`、`SG-I13`、`SG-I15`、`SG-I18`、`SG-I22`、`SG-I27`）：各类至少一次真实 Codex，完整原稿无模型桩。
- [x] `SGA-OPS-005`（每个 `SG-Ixx`）：完整任务 Red→Green→Refactor、全量 CI、Evidence 与独立提交。
- [ ] `SGA-OPS-006`（`SG-I28`）：仅在 `SG-I27` 提交后运行最终 agent-browser，无新 Agent 实现。
- [ ] `SGA-JRN-001`（`SG-I08`–`011`、`SG-I27`）：完整原稿 Evidence→Story analyze/reconcile→review/repair→Bible Gate 可恢复。
- [ ] `SGA-JRN-002`（`SG-I12`–`017`、`SG-I27`）：confirmed Bible→episode analyze/reconcile→Owner Apply→Core Graph。
- [ ] `SGA-JRN-003`（`SG-I18`–`023`、`SG-I27`）：draft→needs_asset→READY AssetVersion→detail/review/repair，Agent 零 Shot/Binding。
- [ ] `SGA-JRN-004`（`SG-I27`）：Bundle 滚动、deadline、runtime unavailable、迟到、Head/Repair 竞争恢复收敛。

## 4. 实施任务完成 Checklist

以下 28 项必须与 Plan 同序；每项只有在其映射 Requirement、定向验证、当时全量真实 CI、Acceptance Evidence 和独立 Git 提交均完成后才能勾选。

- [x] `SG-I01`：Schema/Key/Hash/Wire fixture、工具链/导入边界、失败测试与当前真实 CI 基线完成。
- [x] `SG-I02`：八 Skill 字节保持迁移至 `agent/skills`，单路径、无 fallback、全量 CI、独立提交完成。
- [x] `SG-I03`：StoryGraph Version/Head/Compiler/Owner Set/Outbox 单事务发布完成。
- [x] `SG-I04`：Graph Query + Kafka Event + Elasticsearch Search + ELK 日志真实消费者、故障 CI 完成。
- [x] `SG-I05`：`build-storygraph` 唯一 Bundle、Stage Wire/Policy/Candidate Revision、旧入口原子删除完成。
- [x] `SG-I06`：公共 HumanTask/Lease/Decision/Resume Backend API 与恢复完成。
- [x] `SG-I07`：真实 Review Workbench 与错误/unknown/a11y 完成。
- [x] `SG-I08`：Definition-first Source Evidence、ShardManifest 与 Invocation/Candidate 完成。
- [ ] `SG-I09`：Story analyze/reconcile map-tree 与 Candidate Revision 完成。
- [ ] `SG-I10`：StoryGraph review 与有界 Repair/Gate 完成。
- [ ] `SG-I11`：Bible Human Gate/Confirm Receipt 且零资产物化完成。
- [ ] `SG-I12`：Confirmed Bible 资产/Specification/State/ProductionBinding 原子物化完成。
- [ ] `SG-I13`：Episode segmentation Candidate 与 coverage 完成。
- [ ] `SG-I14`：Episode Plan Gate 与 Episode/Published ScriptVersion 全批物化完成。
- [ ] `SG-I15`：Episode analyze/reconcile 与 Scene/Beat/Occurrence/Claim Candidate 完成。
- [ ] `SG-I16`：Planning Review/Gate/Owner 全批 Apply 完成。
- [ ] `SG-I17`：Core StoryGraph 多集编译、Diff/Impact 全链完成。
- [ ] `SG-I18`：Storyboard Draft/Shot Intent/needs_asset 且零正式 Shot 完成。
- [ ] `SG-I19`：Intent Gate/FreezeIntentSet 与付费前零副作用完成。
- [ ] `SG-I20`：reference_asset Cost/Quota/Runware Job/Artifact unknown 对账完成。
- [ ] `SG-I21`：composite 三视图 QC/Selection/AssetVersion 完成。
- [ ] `SG-I22`：精确 READY AssetVersion 的 detail_shots/Review/Repair 完成。
- [ ] `SG-I23`：Storyboard Gate/Shot/ProductionBinding/Graph 原子 Apply 完成。
- [ ] `SG-I24`：shot_frame/Selection/ShotImageBinding 与单 Shot 局部重跑完成。
- [ ] `SG-I25`：只读 Story Lens、React Flow/Dagre、有界查询与无写入完成。
- [ ] `SG-I26`：类型化 Domain Intent/Owner Command/重编译/Patch Diff 完成。
- [ ] `SG-I27`：完整原稿、代表集人工细查、故障矩阵与全量真实 CI 证据已提交。
- [ ] `SG-I28`：最终 agent-browser Web Journey 与 Backend/Owner/Temporal/Kafka/Search/Artifact 对账已提交。

## 5. Evidence Log

### `SG-I01` — StoryGraph 与 Stage Wire 基础契约（2026-08-27）

- Red：`cd backend && go test ./tests/storygraph ./tests/agent` 真实失败，缺少 `internal/storygraph/domain` 以及新 Stage Invocation/Result/Grant；`cd agent && uv run --all-extras pytest -q tests/contract/test_storygraph_wire.py` 因缺少 `StoryGraphStageInvocation/Result` 收集失败。
- Green：新增 StoryGraph 四图边界、规范 Node Type→Owner、稳定 Node/Edge Key、Evidence Ref、严格 Claim、Canonical Snapshot、拓扑/内容 Hash、稳定 Kahn 排序与最短确定性环路径；Canvas viewport 不进入 Schema/Hash，精确 Owner 版本变化保持 Key/Topology、改变 Content。
- 跨语言：Go 与 Python 共同读取 `backend/tests/fixtures/agent/storygraph-stage-wire-v1.json`，对 strict Invocation/Result、排序 source/upstream refs、Input/Result/Policy Hash、Stage Instance Key、空 Tool、attempt/fencing 建立 golden；旧 Invocation 尚未删除，相关最终条款保持未通过至 `SG-I05`。
- 架构：新增 AST/import 门禁止 Domain/Application 导入 GORM、Temporal、franz-go、go-elasticsearch、MinIO；可选 Kafka/Elastic/React Flow/Dagre 依赖只有在对应首个消费者目录与 Binary 同时存在时才允许加入。未创建数据库 Model、Migration、Kafka/Elastic/Canvas 空目录或第二 ORM。
- 定向验证：`cd backend && go test ./tests/storygraph ./tests/agent ./tests/architecture` 通过；`cd agent && uv run --all-extras ruff check app tests && uv run --all-extras ruff format --check app tests && uv run --all-extras pyright app tests && uv run --all-extras pytest -q` 通过，`27 passed`。
- Backend 真实依赖：任务专用空 PostgreSQL `16.15`、Temporal 指定 digest、MinIO `RELEASE.2025-09-07T16-13-09Z` 在独立端口运行；`gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `99.168s`。数据库 `ON_ERROR_STOP` 断言确认 Migration/Schema 账本表计数为零；三个测试容器已精确删除。
- Frontend：`npm run openapi2ts && npm run lint && npm run typecheck && npm test && npm run build` 通过，16 个测试文件、45 项测试和 Next.js production build 通过，生成 Client 无漂移。
- 交付：开发/生产 Compose `config --quiet` 通过；Backend、Agent、Frontend 三镜像构建及 API/Worker、Codex/Candidate Runtime、Frontend standalone 文件检查通过。`git diff --check` 与测试目录门通过。
- 尚未完成：StoryGraphVersion/Head/GORM Record/Outbox 属于 `SG-I03`，最终单 Bundle/旧 Invocation 删除与真实 Codex 路由属于 `SG-I05`，Kafka/Search/ELK 属于 `SG-I04`；因此对应 Requirement 未提前勾选。按门禁未运行 `agent-browser`。
- Git：本 Evidence 与实现由当前 `SG-I01` 独立提交承载；未推送、未创建 PR。

### `SG-I02` — 八 Skill 字节保持迁移（2026-08-27）

- Red：先增加目标目录、SHA-256 manifest、单路径、Docker 和旧路径拒绝测试；`cd agent && uv run --all-extras pytest -q tests/architecture/test_skill_location.py tests/candidate_runtime/test_codex_runner.py` 得到 `8 failed, 3 passed`，明确证明目标目录不存在、Loader/Docker 仍使用旧路径且旧路径会被读取。
- 迁移：八个过渡 Skill 共 19 个文件按原名和相对路径从根 `.agents/skills` 移至 `agent/skills`；`agent/tests/fixtures/skills/legacy-skill-manifest-v1.json` 固定每个文件的迁移前 SHA-256，迁移后测试逐文件重算并完全相等。未修改 Guidance、Reference 或 metadata 字节。
- 单路径：`CodexSchemaRunner` 只解析 `<repository>/agent/skills/<name>`；旧目录被删除，`.gitignore` 不再为它开放追踪白名单，负向用例确认仅提供旧目录时 fail closed。Agent Docker 只执行 `COPY agent/skills ./skills`，源码和镜像均无旧路径 fallback。
- Agent：`ruff check app tests`、`ruff format --check app tests`、`pyright`、`pytest -q` 全通过，`30 passed`；定向迁移与 Loader 用例 `11 passed`。
- Backend 真实依赖：任务专用 PostgreSQL `16.15`、Temporal 指定 digest、MinIO `RELEASE.2025-09-07T16-13-09Z` 在独立端口运行；带真实服务地址的 `go test -count=1 -p 1 ./...` 全通过，Workflow 包 `92.191s`，不是无环境的快速路径。数据库断言确认 Migration/Schema 账本表计数为零；三个任务容器已精确删除。
- Frontend：`npm run openapi2ts && npm run lint && npm run typecheck && npm run test && npm run build` 全通过，16 个测试文件、45 项测试、Next.js production build 与生成 Client 无漂移。
- 交付：开发/生产 Compose `config --quiet` 通过；Backend、Agent、Frontend 三镜像均从当前工作区构建。Agent 镜像以非 root 用户运行，Codex CLI、Candidate Runtime、19 个 Skill 文件和真实 Harness 加载均通过，`/srv/lanverse/.agents` 不存在；Backend API/Worker 与 Frontend standalone 检查通过。
- 尚未完成：本任务只做行为保持迁移；单一 `build-storygraph` Bundle、启动 Hash Policy、旧 Skill 名和无消费者 metadata 删除属于 `SG-I05`，因此 `SGA-BDL-*` 与复合条款 `SGA-OPS-003` 保持未通过。按门禁未运行 `agent-browser`。
- Git：本 Evidence 与实现由当前 `SG-I02` 独立提交承载；未推送、未创建 PR。

### `SG-I03` — StoryGraph 线性发布与原子 Outbox（2026-08-27）

- Red：`cd backend && go test ./tests/storygraph` 首次真实失败，明确缺少 `internal/storygraph/adapter/gormdb`；持久化测试最初直接导入 GORM 时又被架构门拒绝，随后改为只通过应用闭包建 fixture/计数，没有放宽门禁。
- 事实模型：唯一 GORM Catalog 新增不可变 `sg_storygraph_versions`、每 Project 单行 `sg_storygraph_heads` 与通用 `evt_outbox_events`；Version 冻结父链、Source Revision、排序 Owner Head Set、Schema、Nodes/Edges JSONB、Topology/Content Hash 与发布身份。GORM Hook 同时拒绝 Update/Delete，父版本、Head、Source、Workspace/Project 和 Creator 均由约束连接。
- Compiler：首版只投影当前正式 `DocumentRevision → active Episode/current published EpisodeScriptVersion → confirmed EpisodeStructure`，生成 Source/Episode/Scene/Dialogue/Narrative Beat；Agent Candidate、HumanTask、Review、Storyboard Draft、Kafka/Search/Canvas 均不参与 Owner Snapshot。Unicode Evidence 使用绝对 codepoint 半开区间并重算精确文本 Hash。
- 原子发布：Application 在单个 GORM `SERIALIZABLE` 事务中锁定 Project/Head，校验 expected revision/hash，冻结规范 Owner Set，写 Version、CAS Head、Command Receipt 和 `StoryGraphVersionPublished` Outbox；网络 Publisher 未创建且不在 Owner 事务中。Outbox payload 只有 Version/Hash 元数据，不含完整剧本。
- 定向 PostgreSQL：`LANVERSE_TEST_DATABASE_URL=... go test -count=1 ./tests/storygraph ./tests/architecture` 全通过，StoryGraph `9.937s`。首版固定 5 Nodes/4 Edges；两次线性发布得到 2 Versions/1 Head/2 Receipts/2 Events，Episode 稳定 Node Key 保持且内容 Hash 改变。首命令在第二版发布后回放仍精确返回第一版 Version/Head/Receipt。
- 并发与故障：两个 expected-zero 发布只有一个成功且最终仅 1 Version/1 Head/1 Receipt/1 Event；旧 expected Head 返回 `stale_storygraph_head`。预占重复 Outbox ID 使 Version/Head/Receipt 全回滚；Token Version 漂移、跨租户和 Viewer 分别得到 401/404/403，均为零 Version。
- Schema/架构：空 PostgreSQL `16.15` 中 Migration/Schema-Version 元数据表计数为 0，事实表精确为 `evt_outbox_events,sg_storygraph_heads,sg_storygraph_versions`，`owner_head_refs/nodes/edges` 均为 JSONB。生产 Go 扫描拒绝 Raw/Exec、直接 pgx、sqlx/Bun/Ent、Migration 目录和第二 Writer；Domain/Application 无 GORM/Kafka/Elastic 依赖。
- Hash/Edge：Go 对随机 Owner/Node/Edge 遍历顺序得到相同 Owner Set/Topology/Content Hash；Python 标准库合同测试独立重算同一 Go golden，Canvas 状态不进入 Schema/Hash。18 个规范 Edge Type 均有正反端点 fixture，未知组合和 qualifier/type 不匹配被拒绝；既有 DAG/最小环/Claim payload-edge 测试继续通过。
- 全量真实 CI：全新隔离 PostgreSQL、Temporal 指定 digest 与 MinIO `RELEASE.2025-09-07T16-13-09Z` 下，`go vet ./...` 与 `go test -count=1 -p 1 ./...` 全通过，StoryGraph `12.572s`、Workflow `103.474s`；Agent Ruff/format/Pyright/Pytest 全通过，`31 passed`；Frontend OpenAPI/lint/typecheck、16 文件 45 tests、production build 与 Client drift 全通过。
- 部署：开发/生产 Compose 校验和三镜像构建通过；当前 Backend 镜像以非 root 运行，API 健康且 Workflow Worker 真实连接 PostgreSQL/MinIO/Temporal 并保持运行。验收时发现 Worker Compose 缺 MinIO 会真实退出，已先以独立提交 `3e0349b` 修复，并把全栈镜像启动、HTTP 200 与 Worker 启动日志纳入 CI；所有任务容器和专属数据卷已精确删除。
- 尚未完成：Current/Version/Lens/Diff/Trace/Impact Query、Kafka Publisher/Inbox/DLQ/Replay、Elasticsearch Script/StoryGraph Search 与 ELK 日志链属于 `SG-I04`，因此 `SG-QRY-*`、`SG-EVT-*`、`SG-SRCH-*`、`SG-LOG-*` 及完整权限/API 复合条款保持未通过。按门禁未运行 `agent-browser`。
- Git：CI 运行态修复已独立提交；本 Evidence 与 StoryGraph 实现由当前 `SG-I03` 独立提交承载。均未推送、未创建 PR。

### `SG-I04` — StoryGraph 只读查询交付单元（2026-08-27）

- Red：新增 Application、HTTP 与 PostgreSQL 测试后，`go test -count=1 ./tests/storygraph` 先后真实失败于缺少 `NewQueryService/LensQuery/TraceQuery/DiffQuery` 和 `adapter/httpapi`；实现前没有用内存结果或旧 Storyboard 路径返回成功。
- Query：Backend Application 提供 Current/Exact、`outline/narrative/entity/production/impact` 五 Lens、Upstream/Downstream Trace、Impact Closure 和 Version Diff；API Composition Root 已注册五个只读路由。请求显式携带 project/version/lens/scope/depth/limit/cursor，单页硬上限为 200、depth 上限为 8，响应返回实际 Version/Hash、稳定排序、`truncated/next_cursor/result_hash`。
- Current/stale：游标冻结首次读取的精确 Version ID；Current Head 在分页间切换仍继续原快照，参数或精确版本漂移返回 `stale_storygraph_cursor`。Current 元数据返回完整排序 `compiled_from=owner_head_refs`，并由 GORM 重读当前 Owner Set 计算实时 `stale`；真实 Episode revision 改变且未重编译时保持同一 Head 并返回 `stale=true`，当前 Owner 集不完整时旧已发布 Graph 也保持可读并显式 stale。
- Diff/Lens：250 节点/249 边 fixture 按“排序节点 + 排序边”统一流分页，三页依次返回 `200 nodes`、`50 nodes + 150 edges`、`99 edges`，每页总元素不超过 200 且无跨页边遗漏；相同参数得到相同 Hash/游标，limit 201 被拒绝。五类 Lens 均有有界结果；稳定 Node/Edge Key golden 分别证明 `added/removed/changed`，Owner 内容变更保持 Key 且只报告 change。
- PostgreSQL/权限：隔离 PostgreSQL `16.15` 中 Viewer 可读取 Current/Exact/Lens/Diff/Trace，Token Version 漂移返回 401、非成员返回 404；HTTP + 真实 GORM Lens 返回当前 Version/Hash。查询前后 Version/Head/Receipt/Outbox 计数完全相同，Head ID/Hash/revision/time 不变；Application 与 GORM Query 无 Search Port/Client/索引写入。
- API 契约：Backend OpenAPI 新增 Current/Exact/Lens/Trace/Diff 路径和版本、子图、Diff DTO，元数据不下载完整 Nodes/Edges；Frontend `openapi-typescript` 生成 Client 已同步。缺少显式 depth/limit 返回 422，未认证返回 401；生成 Client drift 测试通过。
- 全量真实 CI：最终在重新创建的空 PostgreSQL、Temporal 指定 digest 和 MinIO `RELEASE.2025-09-07T16-13-09Z` 下，`gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，StoryGraph `15.898s`、Workflow `105.285s`；Agent Ruff/format/Pyright/Pytest 全通过，`31 passed`；Frontend OpenAPI/lint/typecheck、16 文件 45 tests 与 production build 全通过。
- 部署：开发/生产 Compose 合同、Backend/Agent/Frontend 三镜像和镜像内 API/Worker/Codex/Candidate Runtime/standalone 文件检查通过；全新 Compose PostgreSQL/MinIO/Temporal/API/Worker/Frontend 均健康，Worker 启动日志、API/Frontend/Agent HTTP、运行镜像中的 StoryGraph OpenAPI 与未认证 401 均真实验证。本机默认 `9000` 被仓库外进程占用时首次运行真实失败，随后使用任务专属端口重建同一拓扑通过，未修改代码或降低检查。
- 尚未完成：`SG-QRY-002/003` 还需要 `SG-I25` 的真实 Frontend 有界加载证据，`SG-QRY-005` 的 Search 故障部分要随 Elasticsearch 消费者验证；Kafka Publisher/Inbox/DLQ/Replay、Elasticsearch Script/StoryGraph Search 和 ELK 日志链均仍属于 `SG-I04` 后续交付单元，因此 `SG-I04` 本身保持未勾选。按门禁未运行 `agent-browser`。
- Git：本 Evidence 与实现由当前 `SG-I04` 查询交付单元独立提交承载；未推送、未创建 PR。

### `SG-I04` — Kafka Event 交付单元（2026-08-27）

- Red：严格 Envelope、Publisher、Consumer、Replay、PostgreSQL 与真实 Kafka 测试先落位；部署故障验收进一步发现 Kafka 停机时 Consumer 会终止 Event Worker。新增 `TestRealKafkaRunRetriesTheSameUnacknowledgedRecord` 后先真实失败于 10 秒超时，再实现同一 partition/offset 的节流重试并转绿，没有依赖容器重启掩盖问题。
- Event 契约：`lanverse.event.v1` 固定 Event/Workspace/Project、稳定 `aggregate_kind=storygraph`、`aggregate_id=project_id`、单调 Graph revision、Source Receipt、Trace Context 与 Canonical SHA-256 payload hash。`StoryGraphVersionPublished` payload 只允许 Version/Parent/Version No/Owner Set/Topology/Content Hash；未知字段、超过 64 KiB、超过 16 层及剧本、Prompt、Secret、Token、Credential、URL 等敏感键全部 fail closed。
- 事务与持久化：沿用唯一 PostgreSQL/GORM Catalog，扩展 Outbox Lease，并增加 Inbox、Aggregate Checkpoint、Dead Letter；所有 Claim/Fencing/Replay 使用 GORM 与 `clause`，无 Migration、Raw SQL、第二 ORM 或第二连接模型。Owner 事务仍只写 Version/Head/Receipt/Outbox，Kafka 网络调用始终在事务外；ACK 未知或 Broker 失败保持同 Event ID 重试。
- Kafka：锁定官方 `apache/kafka:4.3.1` 单节点 KRaft 与 `franz-go v1.21.6`。Compose/CI 只创建 `lanverse.business.storygraph-version.v1`（7 天）和隔离 DLQ（30 天），关闭自动建 Topic 且断言无 Command Topic。Publisher 使用 All ISR ACK；Consumer 只在 Projection/Inbox 完成后手动提交，重复与旧 revision 不再进入 Projector，Poison Message 进入可审计 DLQ。
- 安全与恢复：无效消息只把原始字节 SHA-256 和固定错误码送入不可重放 DLQ，不保存原始敏感正文；合法 Poison Message 保存已通过严格校验的原 Envelope。Replay CLI 必须同时给出 Project、Event Type、失败时间窗与上限，按原 Topic、Envelope 和 Event ID 重放；实测空范围返回 `replayed=0`。
- 定向真实依赖：在全新 PostgreSQL `16.15` 和真实 Kafka `4.3.1` 上，Eventing 套件 `12.834s` 通过，覆盖精确 Envelope、规范 UUID/事件版本、同 ID 断线发布恢复、Inbox 去重、Project StoryGraph revision fencing、DLQ、范围 Replay，以及未 ACK 消息同 partition/offset 重试；同套件另以空库执行 `go test -race -count=1 -p 1` 通过（`12.423s`）。业务/DLQ `retention.ms` 分别验证为 `604800000` 与 `2592000000`。
- 全量真实 CI：全新隔离 PostgreSQL、Temporal 指定 digest、MinIO `RELEASE.2025-09-07T16-13-09Z` 与 Kafka `4.3.1` 下，`gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，StoryGraph `17.947s`、Workflow `111.842s`；Agent Ruff/format/Pyright/Pytest 全通过，`31 passed`；Frontend OpenAPI/lint/typecheck、16 文件 45 tests、production build 与 Client drift 全通过。
- 部署与故障：开发/生产 Compose 校验、Backend/Agent/Frontend 三镜像构建及三个 Backend Binary 检查通过；完整 Compose 中 API、Frontend、PostgreSQL、MinIO、Temporal、Kafka、Workflow Worker、Event Worker 全部运行并通过真实端点。停止 Kafka 后 Event Worker 保持运行且 `/readyz` 返回 503；Kafka 重启后同一 Worker 无需重启恢复 ready，该停机/恢复剧本已进入 Deployment CI。
- 尚未完成：当前 Kafka 只创建已有真实消费者的 StoryGraph Business/DLQ Topic；Script Topic、Elasticsearch Projection/Reindex/Search API 以及独立 `Filebeat → Kafka → Logstash → Elasticsearch → Kibana` 日志链尚未实现。因此 `SG-EVT-004`、`SG-SRCH-*`、`SG-LOG-*`、`SG-OPS-006` 与完整 `SG-I04` 保持未通过；按门禁未运行 `agent-browser`。
- Git：本 Evidence 与实现由当前 `SG-I04` Kafka Event 交付单元独立提交承载；未推送、未创建 PR。

### `SG-I04` — Elasticsearch Search 交付单元（2026-08-27）

- Red 与边界：先增加独立 `tests/search` 的 Domain/Application/HTTP/Projector/真实 Elastic 契约，测试真实失败于缺少 Search 模块；实现只新增 Backend `search` 业务模块及 Adapter。Domain/Application 不导入 GORM/Elastic，唯一 GORM Snapshot Adapter 无 Raw SQL；浏览器契约只暴露通用 `event|reindex` 来源，不泄漏 Kafka/Elastic 客户端或 DSL。
- 发布与投影：Episode Plan Publish 在原 Owner/Receipt 事务中为每个正式 EpisodeScriptVersion 写入严格引用型 `ScriptVersionPublished` Outbox；真实 Workflow PostgreSQL Journey 验证两集 Owner、同一 Publish Receipt 和两个 pending Event 原子提交。`event-worker` 同时消费 Script/StoryGraph Topic，每条 Event 都重新读取当前 PostgreSQL Owner Snapshot，不信任消息正文作为索引内容；Inbox/Checkpoint 继续承担重复与乱序栅栏。
- 索引与查询：官方 `go-elasticsearch/v9 v9.4.3` 对接 Elasticsearch `9.4.4`，维护两个独立 Alias/Backing。严格 Mapping 保存 Workspace/Project、Owner Logical/Version/Revision/Hash、Projection Version、Evidence 与 Story Node；Marker 最后写入并按 Snapshot Hash 过滤，部分批写不能暴露半快照。Backend 提供两个授权 Search API，只接受 1–200 字文本和 1–50 limit，返回 score/snippet、Owner/Version/Evidence 深链及 fresh/stale/degraded；高亮正文由 Elastic HTML encoder 转义。
- 重建与恢复：`event-worker reindex --kind script|storygraph` 从当前 PostgreSQL Owner 全量构建新 Backing，原子切换 Alias，再重读 Owner 做切换后 catch-up；真实命令分别完成并输出新 Index Version。真实 Elastic 测试覆盖旧 StoryGraph revision 不覆盖新 Marker、Workspace 隔离、两个 Alias 和原子切换；Kafka→PostgreSQL Owner→Elastic→Backend Search Journey 同时返回 Script/StoryGraph fresh、Source Event 和深链。
- 故障：Search 请求 3 秒内返回 degraded，Event Projection 30 秒内失败进入既有重试/DLQ。完整 Compose 停止 Elasticsearch 后，真实注册和 Project Owner Command 仍返回成功，Script Search 返回 `degraded/search_unavailable`，PostgreSQL StoryGraph Query 按 Owner 事实返回业务 404；Event Worker 保持运行但 readiness=503，Elastic 恢复后同一进程自动 ready。Kafka 停止/恢复剧本也再次通过。
- 真实 CI：全新 PostgreSQL、Temporal、隔离 MinIO、Kafka `4.3.1` 与 Elasticsearch `9.4.4` 下，`gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过；Eventing `12.320s`、Search `5.480s + 14.357s`、StoryGraph `17.226s`、Workflow `110.743s`。Search 在三真实依赖下 `go test -race -count=1 -p 1 ./tests/search/...` 通过。Agent Ruff/format/Pyright/Pytest `31 passed`；Frontend OpenAPI/lint/typecheck、16 文件 45 tests 与 production build 全通过。
- 部署：开发/生产 Compose 合同、Backend/Agent/Frontend 三镜像及镜像内 Binary/standalone/Codex Runtime 检查通过。任务专属完整 Compose 中 PostgreSQL/MinIO/Temporal/Kafka/Elasticsearch/API/Frontend/Workflow Worker/Event Worker 全部健康，Script/StoryGraph Alias、两个 Reindex CLI、API/Frontend/Agent HTTP 及 Kafka/Elastic 逐项停机恢复均真实验证。
- 尚未完成：日志 Topic 及独立 `Filebeat → Kafka → Logstash → Elasticsearch → Kibana` 链尚未实施，因此 `SG-EVT-004`、`SG-LOG-*`、`SG-OPS-006`、完整 `SG-JRN-003` 与 `SG-I04` 保持未通过；按门禁未运行 `agent-browser`。
- Git：本 Evidence 与实现由当前 `SG-I04` Elasticsearch Search 交付单元独立提交承载；未推送、未创建 PR。

### `SG-I04` — Kafka + ELK 日志交付单元（2026-08-27）

- Red 与真实故障：先增加 `backend/tests/observability` 的 Logger、HTTP、拓扑和真实日志管道测试，证明缺少统一脱敏 Logger、请求关联中间件、独立日志 Topic/身份/ACL、Filebeat/Logstash/ILM/Kibana Data View。真实部署继续暴露并修复 Filebeat 标签误收集、API Logger 导入、Kafka Controller 认证、容器内 Listener、Logstash 转义、Kibana 内存和 Elasticsearch 恢复轮询问题，没有用 Mock 或跳过替代。
- 应用日志：三个 Backend Binary 统一使用 `log/slog` JSON Logger 和 `lanverse.log.v1`；HTTP 日志只记录 service/environment/event、request/trace/span、method、受控 route、status、duration 与稳定错误码，不记录 path/query/body。递归脱敏覆盖 map/slice/pointer/struct 和敏感键值，Authorization、Token、Password、Prompt、Candidate、Script、Grant、Cookie、URL 等不会进入输出。
- 独立拓扑：业务链保持 `PostgreSQL Outbox → Kafka Business Topic → event-worker → Elasticsearch Business Alias`；日志链为 `应用 JSON → Filebeat → Kafka Log Topic → Logstash → Elasticsearch Log Alias → Kibana`。两链使用独立 Schema、SASL Principal、ACL、Retention、Consumer Group、DLQ 与 Index；Kafka Broker/Controller 均启用 SASL，关闭匿名与自动建 Topic，没有 Command Topic。日志保留 3 天、Hash-only DLQ 14 天、业务 7 天、业务 DLQ 30 天。
- 日志投影：Filebeat 只收集显式 `lanverse_log_collect=true` 的三个 Backend 服务；Logstash 使用持久队列、严格字段 Allowlist 和 `lanverse.logs-indexer.v1` Group。合法记录写入严格 Mapping 的 `lanverse-logs-application-v1` Alias，30 天 ILM 按日或 10 GB rollover；非法记录只把 SHA-256 与稳定错误码写入独立 DLQ，不复制原始正文。Kibana Init 创建对应 Data View。
- 全链与隔离：真实 Backend `/healthz` 请求携带 request/trace 关联后，经 Filebeat/Kafka/Logstash 写入 Elasticsearch，按 service/event/request/trace/route/error code 可检索且敏感查询值零命中；非法消息进入 Hash-only DLQ。真实 ACL 分别拒绝 Filebeat 写业务 Topic、event-worker 写日志 Topic、Logstash 读业务 Topic，证明不是只靠命名约定隔离。
- 故障矩阵：分别停止 Filebeat、Logstash、Kibana、Kafka 和 Elasticsearch 时，真实 Project Owner 事务仍成功且 Workflow Worker 持续运行；日志组件故障不改变 Event Worker readiness 或 StoryGraph Business Alias，Kafka/Elasticsearch 故障只让对应 Event/Search readiness 降级。原进程恢复后无需重启应用即可继续采集日志，Outbox→Kafka→Elastic→Search 在重复、乱序、重启和 Reindex 后收敛。
- 真实 CI 与部署：全新空 PostgreSQL、独立 MinIO、真实 Temporal、Kafka `4.3.1`、Elasticsearch/Logstash/Filebeat/Kibana `9.4.4` 下，Backend 全量 Go 测试、Agent Ruff/format/Pyright/Pytest（31 项）、Frontend OpenAPI/lint/typecheck/Vitest（16 文件 45 项）与 production build 均通过。开发/生产 Compose、三镜像、API/Frontend/Agent HTTP、Backend 三个 Binary、Topic Retention、ILM/Template/Data View 和逐组件停机恢复均由当前工作区真实验证。
- 尚未完成：`SG-QRY-002/003` 仍需 `SG-I25` 的真实 Frontend 有界加载证据，`SG-LOG-002/003` 在 `SG-I27` 还需完整原稿与全局故障矩阵复验；这些复合条款的当前 `SG-I04` 部分已通过，不提前勾选其余任务。按门禁未运行 `agent-browser`。
- Git：本 Evidence 与实现由当前 `SG-I04` Kafka + ELK 日志交付单元独立提交承载；未推送、未创建 PR。

### `SG-I05` — StoryGraph Stage Harness 与唯一 Bundle（2026-08-27）

- Red 与收口：先建立唯一 Bundle、严格候选 Schema、Registry/Hash/Grant/Runtime Route/Revision 的 Go/Python 测试；旧 8 Skill、旧 Loader、`production_bible|storyboard_draft` Invocation 和无消费者 `LangGraph/LangChain` 路径均在同一任务原子删除，没有保留兼容读取或相近版本回退。
- Bundle：`agent/skills/build-storygraph` 只含一个 `SKILL.md` 和 9 个显式 Stage Reference；十个固定 Stage 由普通 Registry 一一映射到 Reference、Pydantic Candidate 和 Policy。Bundle 对排序路径、字节长度和原始字节做 Canonical SHA-256，Go/Python 固定 Hash 均为 `4cf64c94b7d181945da678721db36c4bc45921a9c833164bdea46cb7af149c42`；路径逃逸、缺文件、多文件和任一字节漂移均拒绝。`quick_validate.py` 返回 `Skill is valid!`。
- Backend Wire 与事实：Backend 拥有唯一 `storygraph_stage` Envelope、十 Stage Manifest、Execution Policy、Shard/Source/Upstream exact ref、Input/Result/Stage Instance Hash、Execution Grant 和 Runtime Catalog。未终态 Invocation 按 Bundle Hash 精确解析 `base_url + image_digest`，缺失即失败；首次成功结果在同一 GORM 事务中写不可变 `StageCandidateRevision`，Agent 仍无数据库或 Owner 写入。Candidate Head/CAS、aggregate/repair 来源和 stale closure 继续留给 `SG-I09/I10`，未提前勾选 `SGA-CAN/REP-*`。
- Agent Harness：FastAPI 仅保留 `/healthz` 与私有 `/internal/v1/invocations`；Harness 在临时空目录以 ephemeral/read-only/ignore-user-config、空 Tool Allowlist、临时 JSON Schema、模型调用上限和技术 deadline 启动 Codex。任何 Tool Event、Schema 漂移、预算/超时、Runtime unavailable 和 transport unknown 都不会产生空 Candidate 成功；日志不写 Prompt、Candidate、Grant 或剧本文本。
- 真实 Codex：本机已登录 `codex-cli 0.149.1` 对 `extract_source_evidence` fixture 执行一次真实调用，无 Tool Event，约 29 秒返回可审计 Candidate；由于 fixture 的来源引用歧义和 shard `[0,12)` 与 7 个 Unicode code point 不一致，结果保留 2 个事实阻塞项而未臆造 Evidence，Result Hash 为 `30e7f1d5d5240b30015a9b6c140799b1927e469a5e44d71a23443ec72aedcd0d`。交付镜像继续使用已接受且可复现的 Codex CLI `0.147.0`，没有为本机较新版本增加兼容分支。完整原稿/全部 Stage 的真实模型证据仍属于 `SG-I08`–`I27`，所以 `SGA-COD-*` 与旅程条款保持未通过。
- 定向与全量质量：Agent `ruff check`、`ruff format --check`、`pyright`、`pytest -q` 全通过，`24 passed`；Go/Python 跨语言 fixture、递归 strict JSON Schema、Grant 伪造、未知 Stage、Bundle 漂移、Runtime Route、Candidate 接受和 stale lease fencing 全通过。后端在空 PostgreSQL `16.15`、真实 Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/ELK `9.4.4` 上通过 `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...`，Workflow `124.525s`；数据库实查 Migration 表为 0，测试仅位于三个应用 `tests/`。
- Frontend 与交付：Frontend OpenAPI 生成、lint、typecheck、16 文件 45 tests、production build 与 Client drift 全通过。开发/生产 Compose、Backend/Agent/Frontend 三镜像及镜像内 Backend 三 Binary、Frontend standalone、Agent 非 root/Codex/唯一 Bundle/启动 Hash 均通过；Agent `/healthz` 返回上述唯一 Bundle Hash。
- Kafka/Search/ELK 回归：本任务没有让 Agent 连接 Kafka、Elasticsearch 或日志设施。空卷完整部署中，真实 Backend JSON 日志仍经 `Filebeat → Kafka → Logstash → Elasticsearch → Kibana` 可检索；逐个停止日志组件时 Owner/Workflow/Search Alias 不变，Kafka/Elasticsearch 停机时 Event Worker 正确 not-ready 而 Backend/Workflow 继续工作，恢复后日志与检索链重新收敛。这证明 Script/StoryGraph 检索仍是 PostgreSQL Owner Snapshot 的可重建读模型，而非第二事实源。
- 失败披露：首轮后端整套测试暴露旧测试从外层 Envelope 读取 Stage Input，以及共享测试库中旧 queued Invocation 抢占 Lease；前者改为严格读取 `stage_input`，后者仅用 GORM 隔离同类型旧测试任务，单独与全量均转绿。首次部署故障剧本复用了不一致的旧 Kafka/PostgreSQL 测试卷且本地脚本额外启用 `pipefail`，该次不计通过；精确删除任务专属卷后从空拓扑按 CI 语义重跑通过，未改业务代码或降低断言。
- 尚未完成：`SG-I06` 才实现公共 HumanTask/Lease/Decision/Resume；`SG-I08` 起才创建 Definition-first ShardManifest 和真实业务 Candidate 聚合；`SG-I09/I10` 才完成 Candidate Head CAS 与 Repair。按顺序未运行 `agent-browser`。
- Git：本 Evidence 与实现由当前 `SG-I05` 独立提交承载；未推送、未创建 PR。

### `SG-I06` — 公共 HumanTask、Decision 与 Workflow Resume（2026-08-27）

- Red 与公共契约：先在 `backend/tests/review` 和 `backend/tests/workflow` 建立失败测试，固定项目内列表/详情、Claim/Renew/Release、Decision 和按已持久化 Decision ID 恢复的七个公共端点。Handler 严格拒绝未知 JSON/Query 字段，不接受客户端自报 Workspace、Run、Node、候选集合、Owner 输出或 Workflow Signal 内容。
- 冻结审核事实：HumanTask 由 Backend 冻结 Subject revision/hash、Candidate Set 与允许决议集合；ReviewDecision 不可变并再次固化 Subject Hash。过期 Subject、漂移 Rubric、未知决议、非 UUID 或不在冻结集合中的单候选均 fail closed；并发相同决议幂等收敛，输入漂移冲突，不创建第二审核状态机。
- Lease 与权限：列表使用 `created_at DESC,id DESC` 稳定游标且永不返回 Claim Token；详情仅向未过期的当前 Claim Owner 返回 write-only Token。Owner/Editor 可写、Viewer 只读；跨项目、非成员和 Token Version 撤销均返回防枚举 Not Found。接管、续租、释放、过期、Revision/Token fencing 和重放由真实 PostgreSQL/GORM 覆盖。
- 三阶段恢复：公共响应独立暴露 `Decision recorded → Owner Apply pending/not_required/completed/conflict → Workflow Resume pending/unknown/completed/conflict`。Coordinator 只从 Review 事实解析上下文，以 `human-gate-decision:<decision-id>` 作为稳定幂等键；Production Bible、Episode Plan、Episode Structure、Storyboard 和 Generation Selection 五类既有 Owner 使用真实 Application/Receipt，未知 Executor 返回冲突。`rejected|changes_requested` 不执行正向 Owner 效果，但仍以同一 Decision 恢复 Temporal 分支。
- 真实旅程：`TestGenerationCandidateSetSelectionPersistsThroughWorkflowSignal` 在独立 PostgreSQL、Temporal 与 MinIO 上完成 selected 正向旅程；首次 Signal 结果被刻意丢失后状态保持 `unknown`，重新装配公共 API 服务并仅提交 Decision ID 可恢复为 `completed`，并发 Resume 收敛为同一 Signal/Receipt。第二条 rejected 旅程证明 Owner `not_required`、Workflow 继续到拒绝分支、Node `FAILED`、Run `NEEDS_ATTENTION`，两条 Temporal History 均可 Replay。
- OpenAPI 与生成 Client：`backend/api/openapi/lanverse-v1.json` 固定七端点、Token 可见性和三阶段状态；重新生成 `frontend/src/api/schema.d.ts`、`typings.d.ts` 后，OpenAPI contract、Frontend lint/typecheck、16 个 Vitest 文件 45 项测试与 Next.js production build 全部通过。真实 Workbench 尚未实现，因此 `SG-REV-002/003/005` 与 `SG-I07` 保持未通过。
- 全量真实 CI：空 PostgreSQL、真实 Temporal、MinIO、Kafka `4.3.1` 与 Elasticsearch/Logstash/Kibana `9.4.4` 下，Backend `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包耗时 `245.079s`；Agent Python 3.11 editable install、Ruff、format、Pyright 与 Pytest `24 passed`；Frontend OpenAPI 生成、lint、typecheck、45 项测试与 production build；开发/生产 Compose 和 Backend/Agent/Frontend 三镜像及镜像内运行时检查均通过。
- 架构与范围：新增业务表仍由单一 GORM Model Catalog 建立，无 Migration、Raw SQL、第二 ORM 或 Kafka Command Consumer；Kafka/ELK 仅参与当前全量回归，不承载 Gate 恢复。测试只位于各应用 `tests/`。七类新 StoryGraph Gate、完整原稿和浏览器旅程仍属后续实施项，未提前运行 `agent-browser`。
- Git：本 Evidence 与实现由当前公共人工审核功能提交承载；提交标题和正文只描述功能，不包含任务编号或任务名；未推送、未创建 PR。

### `SG-I07` — 真实 Review Workbench（2026-08-27）

- Red 与真实 Client：先增加 `human-review-api.test.ts` 和 `review-workbench.test.tsx`，初次执行因 Workbench 模块不存在而失败。`src/api/humanReviews.ts` 与 `workflows.ts` 随后按 OpenAPI 精确调用列表/详情、Claim/Renew/Release、Decision、只含持久化 Decision ID 的无 Body Resume 和 WorkflowRun Query；Claim Token 只进入 Renew/Release/Decision Body，路由、查询参数和列表均无 Token。
- 页面与查询：新增 `/projects/{projectId}/reviews?task={task-id}` 动态路由和项目页“审核队列”入口；Task ID 可深链，Claim Token 不进入 URL。单一既有 RTK Query API Slice 以 10 秒轮询队列、5 秒轮询详情/WorkflowRun，命令后只失效当前 Task、项目队列和对应 WorkflowRun；没有第二 Query Client、SSE、通知中心或本地成功状态机。
- Lease 与权限：OPEN 可领取；CLAIMED 且详情无 Token 时只显示“尝试接管”，是否过期由 Backend 最终判定；同一 Owner 刷新后由详情恢复 write-only Token，可按服务端 revision 续期、释放或决议。Token 只存在受保护 Query/组件调用栈，不写 URL、localStorage、sessionStorage、日志或列表。Viewer 和未知 Subject renderer 保持只读，不猜测动作。
- 冻结 Subject 与决议：详情只呈现 Backend 冻结的 Subject type/id/revision/hash、Task revision、Rubric、Candidate IDs 和允许决议。`selected` 必须先键盘/鼠标选择冻结候选；其余决议不夹带 Candidate。每个命令按 Task、服务端 revision、动作和候选生成跨刷新稳定的幂等键；服务端 revision 或 Owner baseline 冲突时显示真实错误并立即重取 Detail，不把失败伪装为完成。
- 四阶段状态：页面分别显示 Task、不可变 Decision、Owner Apply 和 Workflow Resume。`unknown` 只提供“按原决议恢复”；Owner completed 但 Resume 未完成只显示“业务应用完成，正在恢复工作流”。只有 Resume completed、Owner Receipt completed 或 not_required、匹配 NodeRun 已离开等待/运行态且 Gate Output Hash 非空时才显示“工作流已继续”。WorkflowRun 读取失败或事实未收敛时保持显式等待/错误。
- 组件与可访问性：9 项新增测试覆盖无 Token 列表/路由、键盘 Enter 领取、冻结候选选择、刷新后 Token 恢复、续期/释放、过期接管、Viewer/未知 Subject 只读、Decision 后 Owner 冲突重取、UNKNOWN Resume 和 NodeRun 复核。页面使用原生状态筛选、命名区域、按钮/单选语义、可见焦点与 Alert；不提前执行浏览器脚本。
- 当前完整 CI：Frontend OpenAPI 重生成、零 warning lint、typecheck、18 个 Vitest 文件 54 项测试和 Next.js `16.2.12` production build 全通过；standalone 镜像真实启动并由 HTTP 返回新审核路由。空 PostgreSQL `16.15`、真实 Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Logstash/Kibana `9.4.4` 下 Backend `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow `132.911s`；Agent editable install、Ruff/format、Pyright 和 Pytest `24 passed`。开发/生产 Compose、三镜像和镜像内运行时检查均通过。
- 范围：本项只消费既有 Backend 事实和生成类型，没有数据库、Migration、Raw SQL、第二 ORM、Frontend Owner 写入或兼容回退。新 StoryGraph Subject renderer/Owner Gate、完整 Canvas 与全局键盘/reduced-motion 仍属后续实施项，因此复合 `SG-FE-001/008`、`SG-ARC-007` 保持未通过；`agent-browser` 仍只在全部开发与非浏览器验收完成后执行。
- Git：本 Evidence 与实现由当前公共审核工作台功能提交承载；提交标题和正文只描述功能，不包含任务编号或任务名；未推送、未创建 PR。

### `SG-I08` — Definition-first 原稿证据分片（2026-08-27）

- Red 与领域契约：`backend/tests/production/bible/source_evidence_test.go` 先固定 Unicode code-point、语义换行边界、显式 overlap、中文数字与阿拉伯数字集标记、coverage、range+hash 去重、重分片父子谱系和伪造 Evidence 拒绝；`backend/tests/agent/storygraph_wire_test.go` 与 `agent/tests/contract/test_storygraph_wire.py` 共同固定唯一 source ref、revision/hash、logical/context range、文本长度和 logical source hash。测试只位于 `backend/tests` 与 `agent/tests`。
- Definition-first 与单一事实源：Workflow Authoring 先发布 `WorkflowDefinitionVersion`，Start 再创建 `WorkflowRun/NodeRun`；`extract_source_evidence` Executor 只能在该 NodeRun 下通过 GORM 创建 `ShardManifest`、`AgentInvocation` 与 `StageCandidateRevision`。`ShardManifest(ID,Version)` 是不可变复合身份，Update/Delete Hook、复合外键和唯一索引均由真实 PostgreSQL 拒绝漂移；没有 Migration、Raw SQL、第二 ORM、第二 SQL 事实源或 Agent 直写。
- 分片与恢复：Backend 以冻结 DocumentRevision 的 Unicode 文本确定分片、TreePath、语义边界、overlap、marker hint、每片 source hash、coverage hash 与 manifest hash，Agent 不决定边界。真实 Workflow 先并发执行 V1；一个 Invocation 返回 `execution_budget_exceeded` 后，Backend 不扩预算也不截断文本，而是原子发布同 Manifest ID 的 V2、supersede 触发父片并完整重建 active leaf。另一条 V1 结果被刻意延迟到 V2 后返回，旧 Candidate Revision 保留审计但未进入 current aggregate；V2 每片先模拟 transport outcome unknown，再以同 Invocation/Stage Instance identity 恢复成功，最终 NodeRun 只绑定 V2 的确定性 aggregate。
- Evidence 与 Harness：Source Evidence Stage Input 在 Go/Python 两侧 `extra=forbid`，精确绑定 DocumentRevision、normalized/logical hash、logical/context range、冻结文本和 marker hint。Codex 只产候选 anchor/range；Harness 逐字回读、校正 chunk-local→absolute code-point offset 并由本地代码重算 SHA-256，Backend 再次独立重验、排序和 range+hash 去重，无法回读或伪造 anchor 时 fail closed。
- 真实 Codex：本机已登录 `codex-cli 0.149.1`，使用共享 Wire fixture 执行 `LANVERSE_TEST_REAL_CODEX=1 .venv/bin/python -m pytest tests/integration/test_storygraph_real_codex.py -q -s`，结果 `1 passed in 16.40s`。首次真实运行暴露模型无法可靠计算 Evidence SHA-256，随后将哈希责任收回确定性 Harness；复跑得到非空 Observation，所有 anchor、绝对 range 与 SHA-256 均逐项一致。交付镜像仍固定已接受的 Codex CLI `0.147.0`，没有版本兼容分支。
- 真实 Workflow 与计数：独立 `postgres:16.15-alpine`、仓库固定 digest 的 Temporal 与 MinIO 下，`TestSourceEvidenceWorkflowIsDefinitionFirstAndRecoversTheSameShardIdentity` 通过；断言每个当前 active leaf 恰有一个 V2 Invocation、每个 Invocation attempts≥2 且 succeeded，另外只有一个迟到 V1 invocation revision、一个 current aggregate revision、两个不可变 Manifest version，并验证旧 revision ID 不在 aggregate origin。该测试在共享数据库全套运行时曾暴露全库计数污染，断言已收紧到当前 Workspace 后，在不清库的定向复跑 `9.752s` 和随后全新数据库完整 CI 中均通过。
- 完整真实 CI：Backend `gofmt`、`go vet ./...`、无外部依赖 `go test -count=1 ./...` 及真实 PostgreSQL/Temporal/MinIO/Kafka/Elastic/Kibana 的 `go test -count=1 -p 1 ./...` 全通过，Workflow 包 `141.992s`；Agent Ruff check/format、Pyright 和 Pytest 为 `26 passed, 1 skipped`，被跳过项仅是上述另行真实执行的 opt-in Codex；Frontend OpenAPI 生成零漂移、lint/typecheck、18 个 Vitest 文件 54 项测试与 Next.js `16.2.12` production build 全通过。开发/生产 Compose、Backend/Agent/Frontend 三镜像和镜像内 Binary/standalone/非 root Bundle 契约均通过。
- 部署故障 CI：另起非默认端口的隔离 Compose Project，使用刚构建的三镜像启动 PostgreSQL、Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Logstash/Kibana `9.4.4`、Backend、Frontend、Workflow Worker、Event Worker 和 Filebeat；注册/鉴权/项目写入、API/Frontend、Worker 启动日志、ELK 可检索且查询敏感值零泄漏均通过。Filebeat/Logstash/Kibana 逐项停机时核心写入与 Workflow 保持可用；Kafka/Elasticsearch 停机时 Event Worker 进程存活但 readiness 正确为 503，恢复后重新 ready 且日志续传。独立 Agent 容器 health 与 Bundle Hash `4cf64c94b7d181945da678721db36c4bc45921a9c833164bdea46cb7af149c42` 通过。
- 范围与 Git：本项只交付 `extract_source_evidence` 的 Manifest/Invocation/Candidate/aggregate，不提前实现 `analyze_story`、`reconcile_story`、Candidate Head/Repair 或 Bible Gate，因此 `SGA-CAN-*`、`SGA-REP-*`、完整原稿旅程和全 Stage 对抗条款保持未通过；`agent-browser` 仍只在全部开发完成后执行。Evidence 与实现由当前原稿证据分片功能提交承载，提交标题和正文只描述 feature，不包含任务编号或任务名；未推送、未创建 PR。

`SG-D21` 建立时 188 个 Checklist 全部未勾选；当前已按新证据通过 79 条 Requirement 与 `SG-I01`–`SG-I08`，其余保持未通过。下一步继续且只允许实施 `SG-I09` 的 Story analyze/reconcile map-tree 与 Candidate Revision。
