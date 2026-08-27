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

- [ ] `SG-REV-001`（`SG-I06`）：Gate input 建立冻结 HumanTask 且客户端不能自报 Candidate。
- [ ] `SG-REV-002`（`SG-I06`、`SG-I07`）：列表/详情/Lease/Decision/Resume OpenAPI 与生成 Client 无漂移。
- [ ] `SG-REV-003`（`SG-I06`、`SG-I07`）：Claim/Renew/Release Actor/revision/token/expiry/幂等与零泄漏矩阵。
- [ ] `SG-REV-004`（`SG-I06`）：不可变 Decision、允许集合与 selected 单候选并发证据。
- [ ] `SG-REV-005`（`SG-I06`、`SG-I07`）：Decision/Owner Apply/Workflow Resume 三状态 API/UI 分离证据。
- [ ] `SG-REV-006`（`SG-I11`、`SG-I14`、`SG-I16`、`SG-I19`、`SG-I21`、`SG-I23`、`SG-I24`）：七类 Gate 的显式 Owner Apply 与负向零写入证据。
- [ ] `SG-REV-007`（同上）：按 Decision ID 幂等 Resume、并发/重启/UNKNOWN 收敛证据。
- [ ] `SG-REV-008`（同上）：Decision 前 stale 与 Decision 后 baseline 冲突不误套用证据。
- [ ] `SG-PRD-001`（`SG-I08`）：DocumentRevision、Unicode 绝对 Evidence 与两集 coverage。
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
- [ ] `SG-FE-003`（`SG-I07`）：真实 Review Workbench，错误/unknown 无 mock/local success。
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

- [ ] `SGA-BND-001`（`SG-I01`、`SG-I05` 起）：Backend 拥有 Stage/Policy/Invocation/Shard/Candidate 与全部写入。
- [ ] `SGA-BND-002`（`SG-I01`、`SG-I05` 起）：Agent 无 ORM/DB/Object/Kafka/Elastic/Temporal/Provider/Public API。
- [ ] `SGA-BND-003`（`SG-I08` 起）：Stage/Shard 挂既有 Run/NodeRun，无动态 Workflow Node/Agent Checkpoint。
- [ ] `SGA-BND-004`（全部 Agent Stage）：Agent success 零 Confirm/Apply/正式 UUID/Owner/Event/Resume。
- [ ] `SGA-BND-005`（`SG-I05`）：普通显式 Registry，无 LangGraph 运行路径且无消费者时删除依赖/lock。
- [x] `SGA-MOV-001`（`SG-I02`）：八 Skill 原名、原 UTF-8 字节、相对路径 SHA-256 等价迁移。
- [x] `SGA-MOV-002`（`SG-I02`）：Loader/Docker/tests 原子切换，根旧路径删除且无双读/fallback。
- [x] `SGA-MOV-003`（`SG-I02`）：只迁移不改行为，Agent/Backend/Frontend 全量 CI 通过。
- [ ] `SGA-BDL-001`（`SG-I05`）：最终唯一 `agent/skills/build-storygraph/SKILL.md`，旧名/旧 Loader/无消费者 metadata 删除。
- [ ] `SGA-BDL-002`（`SG-I05`）：SKILL 全局规则 + 显式 references，Python 无 Guidance 复制。
- [ ] `SGA-BDL-003`（`SG-I05`）：Stage→Schema/Reference 显式 Registry、loaded-file golden、未知 Stage 拒绝。
- [ ] `SGA-BDL-004`（`SG-I05`）：Bundle Canonical Hash 跨语言、路径/长度/字节与逃逸 fail closed。
- [ ] `SGA-BDL-005`（`SG-I05`）：Manifest 冻结版本/Hash/模型/空 Tool/budget/deadline，任一漂移拒绝。
- [ ] `SGA-BDL-006`（`SG-I05`）：未终态 Invocation 按 Bundle Hash 精确镜像路由，缺失无相近版本回退。

### 3.2 Wire、Stage 与 Shard

- [ ] `SGA-WIR-001`（`SG-I01` fixture、`SG-I05` 最终）：只允许 `storygraph_stage`，旧 kind 原子移除。
- [ ] `SGA-WIR-002`（`SG-I01`、`SG-I05`）：Invocation 全字段 strict fixture。
- [ ] `SGA-WIR-003`（`SG-I01`、`SG-I05`）：source/upstream exact ref 完整且无 current/latest 补全。
- [ ] `SGA-WIR-004`（`SG-I01`、`SG-I05`）：Input Hash 跨语言与每字段突变 golden。
- [ ] `SGA-WIR-005`（`SG-I01`、`SG-I05`）：stage instance identity 并发/重放/结果冲突。
- [ ] `SGA-WIR-006`（`SG-I01`、`SG-I05`）：succeeded/failed/unknown strict union 与 `extra=forbid`。
- [ ] `SGA-WIR-007`（`SG-I01`、`SG-I05`）：Result Hash、Backend 不可变接受与全身份重验。
- [ ] `SGA-WIR-008`（`SG-I01`、`SG-I05`）：Grant expiry/attempt/fencing/恒时验签与伪造拒绝。
- [ ] `SGA-STG-001`（`SG-I05`）：十 Stage/Reference/Candidate 一一对应与跨语言 count=10。
- [ ] `SGA-STG-002`（`SG-I05`）：Pydantic 唯一 Schema 事实、临时生成 JSON Schema、无 tracked 第二份。
- [ ] `SGA-STG-003`（`SG-I05` 起）：Candidate 只用给定 Ref/临时 Key，无 Command/SQL/Graph overwrite。
- [ ] `SGA-STG-004`（`SG-I08`–`010`、`SG-I13`、`SG-I15`、`SG-I18`、`SG-I22`）：无证据不补写事实的对抗/人工证据。
- [ ] `SGA-STG-005`（`SG-I09`、`SG-I15`）：关系/因果/连续性/伏笔使用 Claim Candidate，无持久环边。
- [ ] `SGA-STG-006`（`SG-I18`）：draft_storyboard 非空 Spec/State、缺资产只产 needs_asset。
- [ ] `SGA-STG-007`（`SG-I22`）：detail_shots 精确 READY 资产并禁止改变已接受意图/身份/状态。
- [ ] `SGA-STG-008`（`SG-I18`、`SG-I22`、`SG-I23`）：Backend 生成序号/timecode/UUID/Owner/Binding，Agent 只给 Candidate。
- [ ] `SGA-SHR-001`（`SG-I08`）：不可变 versioned ShardManifest 字段/Hash/约束。
- [ ] `SGA-SHR-002`（`SG-I08`、`SG-I09`、`SG-I15`）：确定性分片/排序/fan-in/tree，Agent 不决定边界。
- [ ] `SGA-SHR-003`（`SG-I08` 起）：超预算发布新 Manifest 完整覆盖，无截断/临时扩预算。
- [ ] `SGA-SHR-004`（`SG-I08` 起）：旧结果只审计，current active leaf + gate 才聚合。
- [ ] `SGA-SHR-005`（`SG-I09`、`SG-I15`）：有界 reduce 只传必要 Ref/Hash/冲突，超预算再分片。
- [ ] `SGA-SHR-006`（全部分片 Stage）：单 shard 失败不毁成功事实，Workflow 无固定业务墙钟终止。

### 3.3 Evidence、Candidate 与 Repair

- [ ] `SGA-EVD-001`（`SG-I08`）：Unicode code-point `[start,end)` 跨语言与逐字回读。
- [ ] `SGA-EVD-002`（`SG-I08`）：语义边界、显式 overlap、coverage 与 range+hash 去重。
- [ ] `SGA-EVD-003`（`SG-I08`、`SG-I13`）：中阿拉伯 Episode marker 与 AI 仅提议歧义边界。
- [ ] `SGA-EVD-004`（`SG-I08`）：chunk-local offset 经 Backend 校正重验后才成正式 Evidence。
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
- [ ] `SGA-ERR-001`（`SG-I05` 起）：稳定错误码 Go/Python fixture 完整。
- [ ] `SGA-ERR-002`（同上）：failed/unknown 可重试分类与同 identity 对账事实计数。
- [ ] `SGA-ERR-003`（`SG-I04`、全部 Agent 切片）：允许日志字段与剧本/Candidate/Prompt/Grant/Secret 零命中。
- [ ] `SGA-ERR-004`（同上）：Backend→Agent→Codex trace 关联且 Candidate Hash 不变。
- [x] `SGA-OPS-001`（`SG-I01` 起）：Agent 测试只在 `agent/tests` 独立分类。
- [ ] `SGA-OPS-002`（每个 Agent 任务）：Ruff check/format、Pyright、Pytest 与 Go/Python fixture 同时通过。
- [ ] `SGA-OPS-003`（`SG-I02`、`SG-I05`）：非 root 镜像含唯一 Bundle/Codex，旧路径不存在且启动 Hash 验证。
- [ ] `SGA-OPS-004`（`SG-I08`–`010`、`SG-I13`、`SG-I15`、`SG-I18`、`SG-I22`、`SG-I27`）：各类至少一次真实 Codex，完整原稿无模型桩。
- [ ] `SGA-OPS-005`（每个 `SG-Ixx`）：完整任务 Red→Green→Refactor、全量 CI、Evidence 与独立提交。
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
- [ ] `SG-I05`：`build-storygraph` 唯一 Bundle、Stage Wire/Policy/Candidate Revision、旧入口原子删除完成。
- [ ] `SG-I06`：公共 HumanTask/Lease/Decision/Resume Backend API 与恢复完成。
- [ ] `SG-I07`：真实 Review Workbench 与错误/unknown/a11y 完成。
- [ ] `SG-I08`：Definition-first Source Evidence、ShardManifest 与 Invocation/Candidate 完成。
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

`SG-D21` 建立时 188 个 Checklist 全部未勾选；当前已按新证据通过 38 条 Requirement 与 `SG-I01`–`SG-I04`，其余保持未通过。下一步继续且只允许实施 `SG-I05` 的 Backend-owned Stage Envelope/Policy/Candidate Revision 与单一 `agent/skills/build-storygraph` Bundle。
