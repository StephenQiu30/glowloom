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
- [x] `SG-PRD-002`（`SG-I11`）：Bible Confirm 只产 Version/Receipt 的数据库事实计数。
- [x] `SG-PRD-003`（`SG-I12`）：MaterializeConfirmedBible 单事务、唯一身份、幂等/回滚/反查。
- [x] `SG-PRD-004`（`SG-I09`、`SG-I12`、`SG-I15`）：同名/别名不得自动合并的负向证据。
- [x] `SG-PRD-005`（`SG-I13`、`SG-I14`）：分集边界与 Episode/Published ScriptVersion 全批原子证据。
- [x] `SG-PRD-006`（`SG-I15`、`SG-I16`）：Scene/Dialogue/Beat/Occurrence/Claim 全批应用与未知事实拒绝。
- [x] `SG-PRD-007`（`SG-I18`）：Storyboard Draft 精确正式输入、`needs_asset` 与零 Shot 证据。
- [x] `SG-PRD-008`（`SG-I19`）：FreezeIntentSet 输出与零 Shot/Cost/Quota/Provider/Graph 副作用证据。
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
- [x] `SGA-STG-005`（`SG-I09`、`SG-I15`）：关系/因果/连续性/伏笔使用 Claim Candidate，无持久环边。
- [x] `SGA-STG-006`（`SG-I18`）：draft_storyboard 非空 Spec/State、缺资产只产 needs_asset。
- [ ] `SGA-STG-007`（`SG-I22`）：detail_shots 精确 READY 资产并禁止改变已接受意图/身份/状态。
- [ ] `SGA-STG-008`（`SG-I18`、`SG-I22`、`SG-I23`）：Backend 生成序号/timecode/UUID/Owner/Binding，Agent 只给 Candidate。
- [x] `SGA-SHR-001`（`SG-I08`）：不可变 versioned ShardManifest 字段/Hash/约束。
- [x] `SGA-SHR-002`（`SG-I08`、`SG-I09`、`SG-I15`）：确定性分片/排序/fan-in/tree，Agent 不决定边界。
- [x] `SGA-SHR-003`（`SG-I08` 起）：超预算发布新 Manifest 完整覆盖，无截断/临时扩预算。
- [x] `SGA-SHR-004`（`SG-I08` 起）：旧结果只审计，current active leaf + gate 才聚合。
- [x] `SGA-SHR-005`（`SG-I09`、`SG-I15`）：有界 reduce 只传必要 Ref/Hash/冲突，超预算再分片。
- [x] `SGA-SHR-006`（全部分片 Stage）：单 shard 失败不毁成功事实，Workflow 无固定业务墙钟终止。

### 3.3 Evidence、Candidate 与 Repair

- [x] `SGA-EVD-001`（`SG-I08`）：Unicode code-point `[start,end)` 跨语言与逐字回读。
- [x] `SGA-EVD-002`（`SG-I08`）：语义边界、显式 overlap、coverage 与 range+hash 去重。
- [x] `SGA-EVD-003`（`SG-I08`、`SG-I13`）：中阿拉伯 Episode marker 与 AI 仅提议歧义边界。
- [x] `SGA-EVD-004`（`SG-I08`）：chunk-local offset 经 Backend 校正重验后才成正式 Evidence。
- [ ] `SGA-EVD-005`（`SG-I08` fixture、`SG-I27` final）：两集开发 + 完整原稿统计和代表集人工细查。
- [x] `SGA-CAN-001`（`SG-I09`）：不可变 StageCandidateRevision/Head CAS/并发。
- [x] `SGA-CAN-002`（`SG-I09`）：invocation/aggregate/repair strict origin union。
- [x] `SGA-CAN-003`（`SG-I09`）：content hash 与 revision hash 分层单字段突变。
- [x] `SGA-CAN-004`（`SG-I09`、`SG-I10`）：exact revision 下游与 Head 变更 stale closure，不覆盖历史。
- [x] `SGA-REP-001`（`SG-I10`、`SG-I22`）：模型 Review Issue 不冒充确定性 Gate/blocker。
- [x] `SGA-REP-002`（`SG-I10`、`SG-I22`）：Repair Patch 冻结 target/allowlist/base/邻接且不能改已发布 Graph。
- [x] `SGA-REP-003`（`SG-I10`、`SG-I22`）：expected Head 应用 N+1、幂等 Receipt 与并发单胜。
- [x] `SGA-REP-004`（`SG-I10`、`SG-I22`）：每轮重跑影响闭包 Gate/Review，有界预算耗尽不半成功。

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
- [x] `SG-I09`：Story analyze/reconcile map-tree 与 Candidate Revision 完成。
- [x] `SG-I10`：StoryGraph review 与有界 Repair/Gate 完成。
- [x] `SG-I11`：Bible Human Gate/Confirm Receipt 且零资产物化完成。
- [x] `SG-I12`：Confirmed Bible 资产/Specification/State/ProductionBinding 原子物化完成。
- [x] `SG-I13`：Episode segmentation Candidate 与 coverage 完成。
- [x] `SG-I14`：Episode Plan Gate 与 Episode/Published ScriptVersion 全批物化完成。
- [x] `SG-I15`：Episode analyze/reconcile 与 Scene/Beat/Occurrence/Claim Candidate 完成。
- [x] `SG-I16`：Planning Review/Gate/Owner 全批 Apply 完成。
- [x] `SG-I17`：Core StoryGraph 多集编译、Diff/Impact 全链完成。
- [x] `SG-I18`：Storyboard Draft/Shot Intent/needs_asset 且零正式 Shot 完成。
- [x] `SG-I19`：Intent Gate/FreezeIntentSet 与付费前零副作用完成。
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
- Bundle：`agent/skills/build-storygraph` 只含一个 `SKILL.md` 和 9 个显式 Stage Reference；十个固定 Stage 由普通 Registry 一一映射到 Reference、Pydantic Candidate 和 Policy。Bundle 对排序路径、字节长度和原始字节做 Canonical SHA-256；当前 v2 Prompt/Schema 对应的 Go/Python 固定 Hash 均为 `352d46c51661e7d989b42ddeb0a0ff0a4b48165e8e3f7700f3e60d170e4c58cb`，路径逃逸、缺文件、多文件和任一字节漂移均拒绝。`quick_validate.py` 返回 `Skill is valid!`。
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
- 部署故障 CI：另起非默认端口的隔离 Compose Project，使用刚构建的三镜像启动 PostgreSQL、Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Logstash/Kibana `9.4.4`、Backend、Frontend、Workflow Worker、Event Worker 和 Filebeat；注册/鉴权/项目写入、API/Frontend、Worker 启动日志、ELK 可检索且查询敏感值零泄漏均通过。Filebeat/Logstash/Kibana 逐项停机时核心写入与 Workflow 保持可用；Kafka/Elasticsearch 停机时 Event Worker 进程存活但 readiness 正确为 503，恢复后重新 ready 且日志续传。独立 Agent 容器 health 与当前 Bundle Hash `352d46c51661e7d989b42ddeb0a0ff0a4b48165e8e3f7700f3e60d170e4c58cb` 通过。
- 范围与 Git：本项只交付 `extract_source_evidence` 的 Manifest/Invocation/Candidate/aggregate，不提前实现 `analyze_story`、`reconcile_story`、Candidate Head/Repair 或 Bible Gate，因此 `SGA-CAN-*`、`SGA-REP-*`、完整原稿旅程和全 Stage 对抗条款保持未通过；`agent-browser` 仍只在全部开发完成后执行。Evidence 与实现由当前原稿证据分片功能提交承载，提交标题和正文只描述 feature，不包含任务编号或任务名；未推送、未创建 PR。

### 有界故事候选归并（2026-08-27）

- Red 与领域契约：`backend/tests/production/bible/story_analysis_test.go` 先固定相同输入随机顺序不改变 Manifest/Coverage Hash、5 个 map leaf 只生成 6 个 reduce node、每个 reduce fan-in 不超过 2、唯一 root 和伪造 Evidence 拒绝。Go/Python Wire 共同固定 `analyze_story` 只消费一个精确 Evidence Candidate Revision，`reconcile_story` 每次只消费 1–2 个同类型精确 child revision；测试继续只位于各应用 `tests/`。
- Definition-first map/tree：Backend 从 current Source Evidence aggregate 逐项复核 aggregate Head、每个 leaf Head、Invocation、Result 和 Candidate Revision 后，在既有 Story NodeRun 下由 GORM 原子创建 `analyze_story`/`reconcile_story` 两个不可变 Manifest。map Invocation 立即发布；reduce Invocation 只在按树排序的精确 child 全部成功且仍为 current Head 时逐层创建，ID 由 Manifest 与 shard key 确定生成，重放不会增加 Invocation。
- Candidate 与证据守恒：Go/Pydantic 双侧严格定义 Entity、State、World Entry、Arc、Review Issue 和 Claim Candidate；relationship/causal/continuity/foreshadowing 只能通过带 participant、anchor、scope、polarity、status 与 Evidence 的 Claim 表达，strict Candidate 不存在持久 Edge 字段，因此通过 `SGA-STG-005`。每一级输出 Evidence 必须是精确上游集合的子集，归并 Schema 保留 world entries 与 arcs，未知字段、伪造 Evidence、候选类型漂移或资料丢失均 fail closed。
- 身份守恒与并发调度：`backend/tests/production/bible/story_analysis_test.go` 固定两个不同 Entity Key 在不同 Episode 具有相同 canonical/normalized name 与 alias 时不得被归并吞并；Backend 接受每个 reconcile 结果前，从冻结 Stage Input 解码精确上游 Candidate Revision，并逐类守恒 Entity、Entity State、World Entry、Claim 与 Arc Key，只有相同精确 Key 可归并，名称或别名不参与自动身份匹配，因此通过 `SG-PRD-004`。完整 Workflow Red 另真实复现多个 map 完成事务并发创建同一确定性 reduce invocation 时的主键竞争；GORM 写入现改为冲突不覆盖，并在冲突后重读校验 Workflow/Node/Manifest/Stage/Input Hash 身份完全一致，不一致仍 fail closed。修复后同一真实旅程连续 3 次通过，且无 Migration、Raw SQL、第二 ORM 或延长超时掩盖。
- Worker 所有权与 stale：既有 Production Bible worker 只领取 `request_type=production_bible`，新 Story Analysis worker 只领取 `story_analysis_shard|story_reconcile_shard`，共享的 `analyze_story` Stage 名不会造成跨 worker 抢单。Worker 在 Agent 调用前和接受结果事务内均重验 exact revision/hash/result/head；测试切换一个上游 Head 后，旧引用返回稳定 `upstream_candidate_stale`，原 Invocation/Result/Revision 保留。
- 真实 Workflow 与 Codex：真实 PostgreSQL `16.15`、Temporal、MinIO 下，`Script → Source Evidence → Story Analysis` 旅程并发运行旧 Bible worker 与两个 Story worker，验证两个 Manifest、全部 map/reduce Invocation 成功、fan-in≤2、最终 Node output 精确绑定 root Candidate Revision，以及服务重放零新增 Invocation。本机已登录 Codex CLI 对 Source Evidence、Story Analysis、Story Reconciliation 连续执行，结果 `1 passed in 53.06s`；每级 Candidate 非空且所有 Evidence identity 都严格属于上游集合。
- 当前完整 CI：Backend `gofmt`、`go vet ./...`、无外部依赖全量测试及全新空 PostgreSQL/Temporal/MinIO/Kafka/Elasticsearch/Kibana 状态下的 `go test -count=1 -p 1 ./...` 全通过，Kafka `4.3.1`、Elasticsearch/Logstash/Kibana `9.4.4` 均为真实服务，Workflow 包 `221.674s`；Agent Ruff check/format、Pyright、Pytest 为 `29 passed, 1 skipped`，跳过项仅为上述已单独通过的 opt-in 真实 Codex；Frontend OpenAPI 生成零漂移、lint/typecheck、18 个 Vitest 文件 54 项测试与 Next.js `16.2.12` production build 全通过。
- 镜像与故障部署：开发/生产 Compose 校验、Backend/Agent/Frontend 三镜像重建及镜像内 Backend 三 Binary、Frontend standalone、Agent 非 root/Codex/唯一 Bundle 边界全部通过。隔离 Compose Project 中 API、Frontend、Workflow/Event Worker、Kafka/ELK 与私有 Agent 运行态健康；Filebeat/Logstash/Kibana、Kafka、Elasticsearch 逐项停机时，业务写入和 Workflow 保持可用，Event Worker readiness 按依赖正确降级并在恢复后重新 ready，日志恢复摄取。
- 当时未提前通过：该交付单元没有实现 map/reduce 输入超预算后的新 Manifest 再分片，也没有完成单 shard 失败恢复；后续“候选输入的版本化重分片”证据只补齐 `SGA-SHR-005`，`SGA-SHR-006` 与整个 `SG-I09` 仍保持未通过。Candidate Repair、Head expected CAS 与旧下游 stale closure 属于后续 `SG-I10`，所以 `SGA-CAN-*`、`SGA-REP-*` 不提前勾选；完整原稿、Human Gate 与最终 `agent-browser` 同样未执行。
- Git：本 Evidence 与实现由描述身份守恒和稳定并发归并的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 候选输入的版本化重分片（2026-08-27）

- Red 与精确覆盖：`backend/tests/production/bible/story_analysis_test.go` 新增 map 与 reduce 两类超预算 Red，固定 Candidate Item 的类型顺序、半开区间、确定性二分、父子谱系、active/superseded 状态、DAG 可达性和无缺口/重复 coverage。`analyze_story` 只收到对应 Source Evidence Candidate 子区间；`reconcile_story` 只收到对应 Story Candidate 子区间，禁止把父 Candidate 或全部 Evidence 重新塞入子 Invocation。
- 版本与路径替换：Backend Domain 从失败 shard 生成同 Manifest ID 的下一不可变 Version。map 超预算将失败 leaf 二分并重建 reconcile tree；reduce 超预算只替换失败节点及其到 root 的祖先路径，未受影响 Key 和成功 Candidate Revision 保持可复用。每个新 Manifest 冻结 Parent Manifest Hash、Coverage Hash 与 Manifest Hash，旧 Version 及旧 Result 继续保留审计。
- 单一事实源与执行所有权：重分片由 Backend Story Analysis Service 计算，由既有 Story Worker 在 `execution_budget_exceeded` 后调用，并在一个 GORM/PostgreSQL 事务中锁定 current Manifest、发布新 Version、标记旧 pending Invocation superseded、创建必要 map Invocation 并调度 reduce；Agent 只返回严格结果，不写数据库、不决定边界。实现没有 Migration、Raw SQL、第二 ORM、第二 Workflow 引擎或兼容回退。
- 并发与迟到结果：完成事务在接受 Candidate 前锁定对应 Stage 的 latest Manifest，发布事务同时锁定 map/reduce latest Manifest，避免旧 Version 在发布后成为 current 输入。调度器固定优先使用当前 Manifest Version 的 Invocation；仅当当前 Version 根本没有该分片 Invocation 时，才按最高旧 Version 复用发布前已成功的不可变结果。这样旧版本迟到成功只留审计，不会与当前版本结果争夺同一确定性 reduce identity。
- 真实 Workflow 剧本：独立 PostgreSQL `16.15` 与真实 Temporal 下，`Script → Source Evidence → Story Analysis` 旅程分别注入一次 map 和一次 reduce `execution_budget_exceeded`，最终持久化 analyze V2、reconcile V3 共 5 个 Manifest、恰好 2 个预算失败、所有当前 Invocation 收敛且 NodeRun 绑定新 root Candidate。修复并发选择后在两次全新数据库中连续通过，耗时 `11.28s` 与 `12.01s`，没有遗留 `running`、确定性身份冲突或延长超时掩盖。
- 跨语言 Harness：Go Wire 与 Agent Pydantic 同步冻结 `candidate_item_start/end`；Agent 复核分区长度等于实际 Candidate Item 数，缺单边界、逆序、越界或额外字段均 fail closed。更新后的本地登录 Codex CLI 连续执行 Source Evidence、Story Analysis 与 Story Reconciliation，结果 `1 passed in 118.07s`，证明严格模型能够真实接收子区间输入并完成三段候选生成。
- 当前完整 CI：Backend `gofmt`、`go vet ./...`、无外部依赖全量测试，以及全新 PostgreSQL/Temporal/MinIO/Kafka/Elasticsearch/Kibana 下的 `go test -count=1 -p 1 ./...` 全通过，最终 Workflow 包 `146.457s`；Agent Ruff check/format、Pyright、Pytest 为 `29 passed, 1 skipped`，跳过项仍仅为已另行通过的显式真实 Codex CLI 集成测试；Frontend OpenAPI 零漂移、lint/typecheck、18 个 Vitest 文件 54 项测试及 Next.js `16.2.12` production build 全通过。
- 镜像与故障部署：开发/生产 Compose 校验、Backend/Agent/Frontend 三镜像和镜像内 Binary/standalone/非 root Bundle 契约均通过。隔离 Compose Project 中注册、项目写入、日志脱敏/检索、私有 Agent health 通过；Filebeat/Logstash/Kibana、Kafka、Elasticsearch 逐项停机时业务与 Workflow 边界符合设计，依赖恢复后 readiness 与日志摄取重新收敛。
- 通过范围：以上证据完成 `SGA-SHR-005`。当前没有实现通用单 shard deadline/失败后的同 Stage Instance 恢复，因此 `SGA-SHR-006` 与整个 `SG-I09` 仍保持未通过；Candidate Repair、完整原稿、Human Gate 和最终 `agent-browser` 也未提前执行。
- Git：本 Evidence 与实现由描述候选输入版本化重分片的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 失败分片的持久恢复（2026-08-27）

- Red 与恢复契约：独立测试先声明恢复 Command、HTTP 契约和真实 Workflow 剧本，实施前稳定编译失败于缺少恢复入口。恢复只接受精确 WorkflowRun、NodeRun 与幂等键；不存在唯一 current deadline failure、目标已终态、上游 stale 或幂等键冲突时均 fail closed，不把 deadline 自动解释为可重试成功。
- 单一事实源与身份守恒：Backend 在单个 GORM/PostgreSQL 事务中锁定 current Manifest、Run/NodeRun 和目标 Invocation，通过既有 Command Receipt 返回幂等结果。恢复只重新排队原 Invocation，不创建新 Stage identity、Input、Policy 或 Manifest，不清除已成功兄弟 Invocation、Candidate Revision、Decision 或 Receipt；领取后 claim version 单调递增，旧 Worker 的迟到结果被围栏拒绝。实现没有 Migration、Raw SQL、第二 ORM、第二 Workflow 引擎或兼容回退。
- 持久等待：故事分析 Node 遇到 deadline failure 后保持 `RETRYING`，Temporal 通过持久定时器轮询 Backend 状态，不以固定业务墙钟终止整个 Workflow。只有显式恢复命令成功后原失败分片才重新进入队列，因此服务重启和等待期间均由既有事实恢复，不依赖内存状态。
- 真实 Workflow 剧本：真实 PostgreSQL 与 Temporal 下，`Script → Source Evidence → Story Analysis` 先触发 map 超预算并发布 V2 Manifest，再向一个 current analyze Invocation 注入 `execution_deadline_exceeded`；恢复命令及其幂等重放返回同一 Invocation，下一次领取的 claim version 恰好加一，旧 claim 完成被拒绝，未失败兄弟和既有 Candidate 保持不变。随后 reduce 超预算路径继续执行，最终 Workflow 成功并绑定新的根 Candidate。
- 并发验证：重复剧本曾真实复现过期单结构查询、非确定注入点和 PostgreSQL deadlock，均按事实修正；恢复事务现在统一按 Manifest → Run/NodeRun → Invocation 的顺序加锁，与 Worker 接受结果的锁顺序一致。修复后同一完整恢复旅程连续 10 次通过，耗时 `141.301s`，没有 deadlock、身份漂移、成功事实丢失或延长超时掩盖。
- 当前完整 CI：全新 PostgreSQL/Temporal/MinIO/Kafka/Elasticsearch/Kibana 下的 Backend `gofmt`、`go vet ./...` 与 `go test -count=1 -p 1 ./...` 全通过，Workflow 包 `146.358s`；Frontend OpenAPI 生成零漂移、lint/typecheck、18 个 Vitest 文件 54 项测试与 Next.js `16.2.12` production build 全通过；Agent Ruff check/format、Pyright、Pytest 为 `29 passed, 1 skipped`，跳过项仍仅为已单独验证的显式真实 Codex 集成测试。
- 镜像与故障部署：开发/生产 Compose 校验、Backend/Agent/Frontend 三镜像和镜像内 Binary/standalone/非 root Bundle 契约全部通过。隔离部署中 API、Frontend、Workflow/Event Worker、Kafka/ELK、私有 Agent、日志脱敏与检索均通过；Filebeat/Logstash/Kibana、Kafka、Elasticsearch 逐项停机时 readiness 与业务边界符合设计，恢复后重新收敛。
- 通过范围：以上证据完成 `SGA-SHR-006` 与 `SG-I09`。Candidate Repair、Head expected CAS 和旧下游 stale closure 仍属于 `SG-I10`，所以 `SGA-CAN-*`、`SGA-REP-*` 不提前勾选；完整原稿、Human Gate 与最终 `agent-browser` 同样未执行。
- Git：本 Evidence 与实现由描述失败分片持久恢复的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 候选修复版本的原子切换（2026-08-28）

- Red 与哈希契约：`backend/tests/agent/candidate_revision_test.go` 先因缺少 Repair origin 类型和哈希材料稳定编译失败。实现后 invocation/aggregate 只允许无 parent 的 Revision 1，repair 只允许带 parent hash 的 Revision N+1；Manifest、leaf、父 Revision、Repair Invocation、Repair Result 或 Candidate 内容任一单字段变化都会改变 `candidate_revision_hash`，而 `candidate_content_hash` 仍只证明规范化内容。
- GORM 事实约束：唯一 Model Catalog 的 `StageCandidateRevision` check constraint 同步收紧三类 origin 的互斥联合。Backend 修复发布原语只接受精确 Workspace、stage instance、expected Revision ID/hash/head revision、成功的 `repair_candidate` Invocation/Result 和已校验的新 Candidate；无内容变化、来源漂移或 Head 漂移均失败。实现没有 Migration、Raw SQL、第二 ORM 或兼容回退。
- 原子并发：同一 GORM/PostgreSQL 事务先 `FOR UPDATE` 锁定 Candidate Head，再重验不可变父 Revision和 Repair Result，创建带 parent/repair provenance 的 N+1，最后以相同 expected ID/hash/revision 条件 CAS Head。两个不同 Repair Result 并发争用同一 Head 时恰好一个成功，另一个返回显式 Head conflict；失败事务不留下孤立 Revision，旧 Revision 拒绝原地更新。
- 真实 PostgreSQL：`TestRepairCandidateRevisionAdvancesHeadOnceUnderContention` 在独立 PostgreSQL `16.15` 上连续 10 次通过，结果 `39.277s`。压力复跑曾暴露固定 stage identity 与 repair identity 污染后续轮次，夹具改为从每轮新 Workspace 派生 identity 后重新通过；没有通过清表、放宽唯一约束或串行化测试掩盖竞争。
- Backend CI：`gofmt`、`go vet ./...` 与全新空 PostgreSQL 下 `go test -count=1 -p 1 ./...` 全通过，Workflow 包 `68.377s`。首次全量运行还发现测试直接导入 GORM 违反架构门禁，夹具已改为普通错误函数；第二次复用污染库触发全库计数失败后，按 CI 契约改用全新数据库并从头通过，未把失败运行报告为成功。
- 通过范围：以上证据完成 `SGA-CAN-001`–`003`，但只建立修复发布的事实原语。`review_storygraph`、冻结 Patch 允许集、确定性 Gate、幂等 Receipt、旧下游 stale closure 与有界重审尚未实现，因此 `SGA-CAN-004`、`SGA-REP-*` 和整个 `SG-I10` 继续保持未通过；最终 `agent-browser` 未执行。
- Git：本 Evidence 与实现由描述候选修复版本原子切换的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 候选审核与修复的冻结边界（2026-08-28）

- Red 与跨语言契约：Backend 测试先稳定编译失败于缺少 Deterministic Gate、Review/Repair Stage Input 和 Patch scope API；Agent contract 测试先因对应 Pydantic Model 不存在而在收集期失败。Green 后 Go/Python 完整 Payload 都必须绑定 exact target Candidate Revision；Repair 还必须同时绑定产生目标 Issue 的 exact Review Candidate Revision，任一 id/hash 漂移或缺失均 fail closed。测试继续只位于 `backend/tests` 与 `agent/tests`。
- Gate 与模型边界：Backend `bible-deterministic-gate-v1` 只检查冻结 Bible Candidate 的 world/entity 与 claim participant/anchor 引用、重复 Key，并确定性排序 blocker；候选中由模型生成的 blocking Conflict/Review Issue 不进入 Gate。Gate 作为只读 Stage Input 冻结，`StoryGraphReviewCandidate` Schema 已删除 `deterministic_blockers`，额外 Gate 字段和冒用确定性 Gate code 的模型 Issue 都被 strict decoder/Pydantic 拒绝，因此 Reviewer 没有降级或伪装 Tool blocker 的输出通道。
- Evidence 与允许集：Review 输出 target revision id/hash 必须与输入一致，每个 Issue 至少一个 Evidence 且只能属于冻结 Candidate Evidence 集合。Repair 输入冻结 blocking Issue、轮次、允许 Candidate Key/字段、规范 base fragment hash 和只读邻接；Go/Python 对同一片段得到 `d4d2e657ebe16dd6ecab5d3aa2c8d5e536ffc385fba3ee9e0627e3ee24d8c17b`，Patch 的 target、base hash、字段、replacement 类型或重复操作任一越界都被拒绝。
- 未发布边界：Bible 可修字段使用显式 allowlist，不包含 Evidence、Identity Key、`graph_json` 或任意 Graph 写入字段；Review/Repair Payload 均要求 Base StoryGraph Ref 为空，带 `base_storygraph_version_id/hash` 的负向用例在 Go/Python 两侧失败。Agent 只返回 Candidate/Patch，不写 PostgreSQL、不应用 Owner Command；已发布 Graph 修改仍只能走 Human-approved Domain Intent。
- Harness 与真实 Codex：Agent 在 JSON Schema 校验后继续用冻结 Stage Input 复核 Review Evidence 和 Patch allowlist，无法靠符合表面 Schema 绕过上下文。扩展后的本地登录 Codex CLI 真实执行 `Source Evidence → Story Analysis → Reconciliation → Review → Bounded Repair`，结果 `1 passed in 79.14s`；Review 和 Repair 均由同一 `agent/skills/build-storygraph` Bundle 完成，无 Tool、业务写入或第二 Workflow Engine。
- 完整 CI：Backend 无外部依赖全量门禁通过；最终代码在全新 PostgreSQL `16.15` 下全量通过，Workflow 包 `61.680s`；全新 PostgreSQL、Temporal、MinIO、Kafka `4.3.1` 与 Elasticsearch/Logstash/Kibana `9.4.4` 下 `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `131.739s`。Agent Ruff check/format、Pyright 与 Pytest 为 `32 passed, 1 skipped`，跳过项即上述另行真实通过的 opt-in Codex；Frontend OpenAPI 零漂移、lint/typecheck、18 个 Vitest 文件 54 项测试和 Next.js `16.2.12` production build 全通过。
- 镜像、故障与 hygiene：开发/生产 Compose 校验，Frontend/Backend/Agent 三镜像重建，Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 均通过。隔离部署中 API、Frontend、Workflow/Event Worker、Kafka/ELK、日志脱敏与检索通过；Filebeat/Logstash/Kibana/Kafka/Elasticsearch 逐项停机时 Owner 写入与 Workflow 保持可用，Event Worker readiness 按依赖降级并在恢复后重新 ready，日志恢复摄取；本次专用容器、网络和 Volume 已精确删除，仓库 hygiene 通过。
- 通过范围：以上证据完成 `SGA-REP-001` 与 `SGA-REP-002`。这只证明冻结输入、确定性 Gate、Review Evidence 和 Patch scope，不代表 Patch 已应用；幂等 Receipt、旧下游 stale closure、并发 Patch 业务事务和有界重审仍未完成，因此 `SGA-CAN-004`、`SGA-REP-003`–`004` 与整个 `SG-I10` 保持未通过，最终 `agent-browser` 未执行。
- Git：本 Evidence 与实现由描述候选审核和修复边界的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 候选修复的幂等应用与精确失效闭包（2026-08-28）

- Red 与职责边界：独立 Backend 测试先稳定编译失败于缺少 frozen Patch 应用和 stale closure API。Green 后 Application 负责按 Candidate stable key 定位允许片段、重算 base fragment hash、应用 `text/strings` replacement 并重新执行严格 Story Reconciliation Candidate 校验；GORM Adapter 只负责事务、锁和持久化，Application 与测试均未直接导入 `gorm.io`。所有测试继续只位于 `backend/tests`。
- 单一 SQL 事实源：唯一 GORM Model Catalog 新增不可变 `StageInstanceStaleness`，每个 stale Stage Instance 绑定直接致因 Candidate Revision id/hash；Invocation Stage 绑定其 Invocation，Backend Aggregate Stage 保持 Invocation 为空并由 Aggregate Origin 证明 leaf 谱系。没有 Migration、Raw SQL、第二 ORM、Graph JSON 直写或新 Workflow 引擎。既有 `CommandReceipt` 以 `production_bible.candidate_repair.apply` 承载幂等结果，没有建立第二 Receipt 体系。
- 原子事务：Backend 先重验 exact Parent Candidate Revision、产生目标 Issue 的 Review Candidate Revision、两者内容 Hash、成功 Repair Invocation/Result 与 frozen Patch，再在同一 PostgreSQL 事务中锁定 expected Head、创建 Repair N+1、CAS Head、计算并写入 stale closure、最后写 Receipt。测试注入一个无法严格解码的下游依赖后，事务按设计失败，Head 仍指向父 Revision，N+1/Receipt/staleness 计数均为零，证明不是“先切 Head 再补闭包”。
- 精确闭包与历史：闭包同时沿 Invocation exact upstream candidate revision id+hash 与 Aggregate Origin exact leaf revision id+hash 传播。真实旅程中 Review Invocation → Backend Review Aggregate → Episode Segment → Episode Analysis 四层被依次标 stale，Aggregate staleness 的 Invocation 外键保持空；已应用 Repair Invocation 作为新 Revision provenance 明确排除，无关 Shot 分支不标记。原 Invocation/Result/Revision 保持不变，staleness 自身拒绝 update/delete，因此旧版本仍可精确重放和审计。
- 幂等与并发：同一 Patch/expected Head/幂等键并发提交返回同一 N+1、同一 Receipt 和同一排序 stale key 集合；补齐 Aggregate 闭包后的真实 PostgreSQL 旅程连续 10 次通过，耗时 `33.980s`。使用新幂等键继续争用旧 expected Head 返回 conflict，不创建额外 Revision、Receipt 或 staleness；底层不同 Repair Result 的并发单胜仍由上一交付单元的 Head CAS 压力测试覆盖。
- 完整 CI：最终代码的 Backend 无外部依赖 `gofmt`、`go vet ./...`、`go test -count=1 ./...` 全通过；全新 PostgreSQL `16.15`、Temporal、MinIO、Kafka `4.3.1` 与 Elasticsearch/Logstash/Kibana `9.4.4` 下 `go test -count=1 -p 1 ./...` 全通过，Workflow 包 `132.815s`。Agent Ruff/Pyright/Pytest 为 `32 passed, 1 skipped`，唯一跳过项仍是上一交付单元已经真实通过的 opt-in Codex；Frontend OpenAPI 零漂移、lint/typecheck、18 个 Vitest 文件 54 项测试和 Next.js `16.2.12` production build 全通过。
- 镜像与部署：开发/生产 Compose 校验和 Frontend/Backend/Agent 三镜像重建、三 Backend Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 探针通过。首次隔离部署因测试用 `AGENT_EXECUTION_SECRET` 少于既有 32 字节门禁而按设计退出，修正测试配置后 Backend 空库 Catalog 同步、Frontend、Workflow/Event Worker、Temporal、MinIO、Kafka/ELK/Filebeat 全部真实健康；补齐 Aggregate 闭包后又重建最终 Backend 镜像，API、Workflow/Event Worker 及空库新事实表再次通过实际运行探针。本 feature 专属容器、网络和 Volume 已精确删除，原有资源未受影响。
- 通过范围：以上证据完成 `SGA-CAN-004` 与 `SGA-REP-003`。当前只证明 Patch 原子应用、幂等 Receipt 与 stale closure；尚未创建 replacement Invocation、重跑闭包 Gate/Review 或处理轮次预算耗尽，因此 `SGA-REP-004` 与整个 `SG-I10` 继续保持未通过，最终 `agent-browser` 未执行。
- Git：本 Evidence 与实现由描述候选修复幂等应用与精确失效闭包的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 有界候选审核与自动修复闭环（2026-08-28）

- Red 与 Workflow 契约：`backend/tests/authoring/graph_contract_test.go` 和 `backend/tests/workflow/story_review_executor_test.go` 先稳定失败于缺少 `agent.story_review`、Review Owner 与 Executor。Green 后 Catalog 只新增 `agent.story_review@1.0.0`，输入/输出均为 `story_reconciliation_candidate`，配置只允许显式 `max_repair_rounds=1..3`；Node pending 时保持 `RETRYING`，只有干净 current Revision 才产生 Output，预算耗尽返回失败且 Output 为空。
- 持久有界闭环：Backend 为 current Candidate Head 创建 `review_storygraph` ShardManifest 与 Invocation，Review 成功后对同一精确 Candidate 重跑 `bible-deterministic-gate-v1`。排序后的首个可修 blocking Issue 才能创建冻结 Repair Invocation；Patch 经既有原子 Coordinator 产生 N+1 后，下一次 Temporal 持久轮询创建同 Manifest identity 的 V+1 与 replacement Review。Repair Invocation 的数据库计数是轮次事实，重启、重复 Activity、unknown 领取与并发 Worker 都不会重置预算；Gate blocker、模型失败、不可修边界和 `execution_budget_exceeded` 都停在 `needs_review/failed`，不会伪造成功或半成品 Output。
- 单一事实源与边界：API 只运行 Review/Repair Agent Worker，`workflow-worker` 只通过 Application Owner 驱动闭环；二者复用 Backend Composition Root、唯一 PostgreSQL/GORM Catalog 和唯一 Temporal Workflow。实现只扩展既有 `ShardManifest` stage constraint，并复用 `AgentInvocation`、`StageCandidateRevision/Head`、`CommandReceipt` 与 `StageInstanceStaleness`，没有 Migration、Raw SQL、第二 ORM、第二数据库、Kafka Command Topic或 Agent 业务写入。
- 真实 Workflow 剧本：全新 PostgreSQL `16.15` 与真实 Temporal 下，`Script → Source Evidence → Story Analysis → Story Review` 先生成一个 Evidence-scoped blocking Issue，再由 Repair 只修改允许的 `canonical_name`，随后对 Revision 2 重新 Review 并清零 blocker。最终 Review Node `SUCCEEDED` 且只输出 Revision 2；数据库中恰好 2 个父 Hash 相连的 Review Manifest、2 个成功 Review Invocation、1 个成功 Repair Invocation，旧根 Candidate id/hash 均未作为最终 Output。定向命令 `go test -count=1 -run TestSourceEvidenceAndStoryAnalysisWorkflowRecoverBoundedMapReduce ./tests/workflow` 结果通过，耗时 `31.783s`；该旅程还验证旧 Story Analysis root 在 Head 切换后返回 exact upstream stale，而不是重放旧成功。
- 预算与失败语义：`backend/tests/production/bible/story_review_cycle_test.go` 固定 Repair 轮次已用尽与模型调用 `execution_budget_exceeded` 两种路径都返回 `needs_review`；`backend/tests/workflow/story_review_executor_test.go` 固定该状态不得被转换为成功 Node 或任何 Output。确定性 Gate 每次都从 current Candidate 重新计算，Reviewer 结果不能携带或覆盖 Gate blocker。
- 当前完整 CI：首次复用定向验收数据库运行全量测试时，Authoring 与 Workflow 的全库初始计数按设计发现旧事实，因此该轮记为失败；在本 feature 专属 PostgreSQL 中精确重建全新测试库后，Backend `gofmt`、`go vet ./...` 与 PostgreSQL/Temporal/MinIO/Kafka `4.3.1`/Elasticsearch、Logstash、Kibana `9.4.4` 下的 `go test -count=1 -p 1 ./...` 全通过，Workflow 包 `151.566s`。Agent Python `3.11.15` 的 Ruff check/format、Pyright、Pytest 为 `32 passed, 1 skipped`，唯一跳过项仍是已在前一交付单元真实通过的 opt-in Codex；Frontend OpenAPI 零漂移、lint/typecheck、18 个 Vitest 文件 54 项测试和 Next.js `16.2.12` production build 全通过，Delivery hygiene 继续保持独立测试目录与语言边界。
- 镜像与故障部署：开发/生产 Compose 校验，Backend/Frontend/Agent 三镜像重建，Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 契约通过。首次独立 Backend 镜像探针因测试用 Agent secret 少于既有 32 字节门禁而正确 fail-fast，修正验收配置后 API、Workflow Worker 与 Event Worker 均连接真实依赖启动。随后隔离 Compose Project 完成注册、项目写入、日志脱敏检索和 Agent Bundle health；Filebeat/Logstash/Kibana 逐项停机不影响 Owner 写入与 Workflow，Kafka/Elasticsearch 停机时 Event Worker readiness 正确为 503、进程仍存活，恢复后重新 ready 且日志继续摄取。专属容器、网络和 Volume 已精确删除，原 development 镜像标签恢复，未触碰其他本地资源。
- 通过范围：以上证据完成 `SGA-REP-004` 与 `SG-I10`。Bible Human Gate、Production Bible Confirm/资产物化、Episode/Shot/Canvas、完整原稿和最终浏览器验收仍未实现；因此 `SG-I11` 以后与 `SGA-JRN-001` 保持未通过，`agent-browser` 按既定顺序未执行。
- Git：本 Evidence 与实现由描述有界候选审核和自动修复的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### Production Bible 候选审批与不可变确认（2026-08-28）

- Red 与公共 Gate：独立 Backend/Frontend 测试先证明旧公共 Workflow 只有 Story Review、可变 `ProductionBible` 直确认 API/UI，且 HumanTask 没有精确绑定 current Story Candidate revision。Green 后生产 Definition 增加 `human.production_bible_review@2.0.0`，公共 HumanTask 的 subject 固定为 `story_reconciliation_candidate` 并冻结 candidate id/hash/revision；只有 deterministic gate 与模型 blocker 均清零的 current Candidate 才能进入审批。旧 v1 Definition 仅作为不可变历史 catalog 保留，不再有当前公共入口。
- 单一事实源与不可变 Owner：唯一 GORM Catalog 新增不可变 `scr_production_bible_versions`，Version 冻结 Project、Story Candidate、Document Revision、Decision、规范内容 Hash 与线性 version number；批准后的 Owner Apply 在同一 PostgreSQL/GORM 事务中重验 exact Candidate/Document/Decision，写入一个 Version 和一个 `production_bible.confirm` Command Receipt。无 Migration、Raw SQL、第二 ORM、第二 SQL 事实源、Agent Writer 或 Kafka Command Topic。
- 精确绑定与负向零写入：Workflow Node output 只发布 `production_bible_version` exact ref，证据读取并重验已持久化 Version/Receipt。真实数据库事实计数证明成功路径恰好 1 Version + 1 Receipt，旧可变 `ProductionBible`、Artifact、Character/Location Asset、Specification、Episode、Shot 与 StoryGraph 均为 0；Version 的 update/delete 均被拒绝。由此完成 `SG-PRD-002`，而覆盖七类 Gate 的复合 `SG-REV-006`–`008` 只完成 Bible 部分，仍保持未勾选。
- 恢复、stale 与冲突：同一 Decision/Intent 幂等重放返回同一 Version/Receipt。Temporal Signal 第一次返回 UNKNOWN 时不伪造成功，重试后按既有 Intent/Receipt 收敛且仍只有一份 Owner 事实；Decision 前 Candidate Head 漂移会把任务标记为 stale，Decision 后 Candidate Head 漂移返回 Owner conflict，二者都不误套用、不创建额外 Version/Receipt/Node output。
- 删除兼容路径：公共 OpenAPI、Backend Handler/Application 以及 Frontend RTK mutation/按钮中的旧可变直确认入口已删除，Workspace 统一引导到 HumanTask Workbench；对应旧 `production_episode_plan_worker_test.go` 依赖已删除的可变 v1 路径，未通过 skip、兼容分支或假实现保留。Episode/Asset 物化将在 confirmed Version 之上由下一独立 Owner Command 实现。
- 真实 Workflow 与完整 CI：真实 PostgreSQL `16.15`、Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Filebeat/Logstash/Kibana `9.4.4` 下，`gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `135.756s`；Agent Ruff check/format、Pyright、Pytest 为 `32 passed, 1 skipped`，唯一 skip 仍是已单独验证且需显式 `LANVERSE_TEST_REAL_CODEX=1` 的真实 Codex 集成；Frontend OpenAPI 生成、lint、typecheck、18 个 Vitest 文件 54 项测试与 Next.js `16.2.12` production build 全通过。
- 镜像与故障部署：开发/生产 Compose 配置、Backend/Frontend/Agent 三镜像和镜像内 Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 均通过。全新空卷部署中 API、Frontend、Workflow/Event Worker、PostgreSQL/Temporal/MinIO/Kafka/ELK 全部健康；日志 request/trace 可检索且敏感查询值未落库，Filebeat/Logstash/Kibana 停机不影响 Owner/Workflow，Kafka/Elasticsearch 停机时 Event Worker 正确 not-ready 而 Backend/Workflow 保持可用，恢复后日志与检索重新收敛。专属部署已清理并还原原镜像标签，未触碰仓库外服务。
- 通过范围：Bible Gate 已完成，但 Character/Location Asset、SpecificationVersion、AssetState、ProductionBinding、Episode/Shot/Canvas、完整原稿旅程与最终浏览器验收均未实现；因此 `SG-I12` 以后、`SGA-JRN-001` 和跨七类 Gate 的剩余复合条款保持未通过。`agent-browser` 按约定只在全部开发完成后执行，本次未运行。
- Git：本 Evidence 与实现由描述候选审批与不可变确认的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### Confirmed Bible 资产与生产绑定原子物化（2026-08-28）

- Red 与输入边界：独立 `backend/tests/production/bible/materialization_service_test.go` 先固定 exact Bible Version id/version/content hash、同名不同 Entity Key、Character/Location/Prop、确定性 `base` State、幂等 Receipt、跨版本事实复用和 drift conflict；Workflow 旅程先要求 Bible Gate 后存在第六个物化节点。Green 后该节点只接受 `production_bible_version` exact ref，配置必须为空，输出为 `production_bible_materialization`，不读取 current/latest 或 Candidate 临时结果。
- 单一 SQL 事实源与职责：唯一 GORM Model Catalog 增加不可变 Asset、AssetState、ProductionBibleSpecificationVersion、ProductionBinding 与 Binding-State 事实；Backend Application 负责身份、版本、Hash、复用和冲突策略，`adapter/gormdb` 只负责锁、事务和持久化。没有 Migration 文件/表、Raw SQL、第二 ORM、第二数据库、Agent Writer、Kafka Command Topic 或第二 Workflow 引擎；Agent 仍只产生 Candidate。
- 身份、状态与复用：物化只按稳定 `EntityKey` 识别 Asset，同名 Character 的两个不同 Key 保持两个 Asset；Character、Location、Prop 都产生独立 Specification 和 Binding。没有显式 `base` 的 Asset 从 stable spec 确定性生成一份 `base` State，显式 state 继续保留；下一 Bible Version 的完全相同 Asset/Specification/State 复用既有不可变事实，只创建指向新 Bible Version 的 Binding，不复制身份或规范版本。
- 原子事务、幂等与反查：Backend 在同一共享 GORM 事务中锁定并重验 confirmed Version 与项目权限，创建或复用全部事实，最后写 `production_bible.materialize_confirmed` Receipt。相同 Intent 重放返回同一 Materialization/Receipt；相同幂等键但输入漂移返回 conflict。真实旅程让最终 Receipt ID 与既有 Confirm Receipt 冲突后，Asset、Specification、State、Binding、Binding-State 和 Materialization Receipt 计数全部回到零；恢复同一 Temporal Signal 后只生成一组事实，可从 Binding 精确反查 Bible Version、Asset、Specification 与 State。
- 不可变与真实 Workflow：Asset、AssetState、SpecificationVersion、ProductionBinding 和 Binding-State 的 update/delete 均由数据库回调拒绝。真实 PostgreSQL `16.15` 与 Temporal 中，`Script → Source Evidence → Story Analysis → Story Review/Repair → Production Bible Gate → Bible Materialization` 连续完成，物化节点只在 confirmed Gate 后运行；定向旅程最终通过，耗时 `35.379s`，且成功前仍保持旧可变 ProductionBible、Artifact、Episode、Shot、StoryGraph 为零。
- 当前完整 CI：最终 Backend 在全新数据库及真实 Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Filebeat/Logstash/Kibana `9.4.4` 下通过 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`，Workflow 包 `141.706s`。Agent 使用隔离 Python `3.11` 执行 Ruff check/format、Pyright、Pytest，结果 `32 passed, 1 skipped`；唯一 skip 是需显式本地 Codex 登录的 opt-in 集成。Frontend `npm ci`、OpenAPI 生成/零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试和 Next.js `16.2.12` production build 均通过，仓库 hygiene 通过。
- 镜像与故障部署：开发/生产 Compose 配置、Backend/Frontend/Agent 三镜像重建，Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 探针均通过。最终隔离部署使用真实 PostgreSQL/Temporal/MinIO/Kafka/ELK，验证 API/Frontend/Workflow/Event Worker、日志脱敏与检索；Filebeat/Logstash/Kibana 停机不影响 Owner/Workflow，Kafka/Elasticsearch 停机时 Event Worker 正确返回 503 且 Backend/Workflow 保持可用，恢复后重新 ready 并继续摄取。此前三次本地尝试分别在宿主 MinIO/PostgreSQL 端口占用和一次错误宿主端口替换处于验证前或自检阶段失败，修正验收环境后原 CI 剧本完整通过；专属部署容器、网络和 Volume 已精确清理。
- 通过范围：以上证据完成 `SG-PRD-003` 与 `SG-I12`。Episode segmentation/Owner Apply、跨集多 State 全链、Shot/Canvas、完整原稿复合旅程与最终浏览器验收仍未完成，对应复合条款保持未勾选。`agent-browser` 按约定只在全部开发完成后执行，本次未运行。
- Git：本 Evidence 与实现由描述确认版本原子资产物化的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 有证据的分集边界候选与全文覆盖（2026-08-28）

- Red 与严格契约：`backend/tests/production/bible/episode_segmentation_test.go`、`backend/tests/agent/episode_segmentation_wire_test.go` 与 `agent/tests/contract/test_storygraph_wire.py` 先固定唯一全局 shard、精确 Script/Evidence leaf/Bible Materialization revision/hash、目标时长、有界 Evidence Index、显式 marker、全文连续 coverage 和伪造证据拒绝。测试只位于独立 `backend/tests`、`agent/tests` 目录；Go/Pydantic 都拒绝未知字段、重复/乱序 leaf、缺失 source ref、越界 Evidence 和不完整 marker。
- Definition-first 与职责：Catalog `8.0.0` 增加正式 `agent.episode_segmentation@1.0.0`，Temporal 仅在 Source Evidence aggregate 与 confirmed Bible Materialization 都成功后执行该节点。Backend Application 生成不可变全局 Manifest/Invocation，GORM adapter 重验 current aggregate/leaf Head、Source Invocation/Result、DocumentRevision、ProductionBibleVersion、Materialization 与项目目标时长；Agent 只通过 `agent/skills/build-storygraph` 返回 Candidate，不拥有 GORM、事务、Owner Command 或业务写权限。
- 边界与证据守恒：中文数字和阿拉伯数字集标记由确定性 Source Evidence manifest 生成，显式 marker 必须原位置成为边界且携带原 Evidence，模型只能补充缺失、歧义或重组建议。Backend 要求 `episode_order` 从 1 连续递增、边界从 code point 0 开始、相邻区间首尾相接、末尾等于全文长度、标题非空、Evidence 位于自身区间且逐项属于冻结 Index；任何 gap、overlap、越界、marker 覆盖或臆造 Evidence 都 fail closed。
- 单一事实源与恢复：本功能只复用共享 PostgreSQL/GORM 的 `agt_shard_manifests`、`agt_invocations`、`agt_stage_candidate_revisions` 和 `agt_stage_candidate_heads`，没有 Migration、Raw SQL、第二 ORM、第二数据库或新业务事实表。Agent 传输结果未知后 Invocation 变为 `unknown`，由同一 ID/Stage Instance/Input Hash 重新领取；成功后恰好一个 Candidate Revision 与一个 current Head，`prj_episodes` 保持零。并发 Source Evidence 最后分片通过同一 manifest 行锁串行化聚合判定，消除了两个成功 worker 都错过最终 aggregate 的永久 `RETRYING`。
- 真实 Codex 与 Workflow：本机已登录 Codex CLI，运行 `LANVERSE_TEST_REAL_CODEX=1 uv run pytest -q tests/integration/test_storygraph_real_codex.py -k segments_full_source_without_overriding_markers -vv`，结果 `1 passed, 1 deselected in 11.66s`；真实模型保留两个显式 marker，并覆盖全文末尾。真实 PostgreSQL `16.15` 与 Temporal 下，`Script → Source Evidence → Story Analysis/Review → Bible Gate → Bible Materialization → Episode Segmentation` 旅程注入一次分集调用结果未知，重试保持同一 Invocation，最终三段边界覆盖全文、一个 Revision/Head、零 Episode；定向测试 `41.049s` 通过。
- 完整 CI 与部署：全新数据库及真实 Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Filebeat/Logstash/Kibana `9.4.4` 下，`gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `145.455s`。Agent Ruff check/format、Pyright 与 Pytest 为 `33 passed, 2 skipped`，其中本次分集真实 Codex skip 已由上述 opt-in 命令单独通过；Frontend OpenAPI 零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试和 Next.js production build通过。Backend/Frontend/Agent 三镜像、Compose、日志脱敏与 Kafka/Elasticsearch/Filebeat/Logstash/Kibana 故障恢复均按正式 CI 剧本通过，独立 Agent health 返回新 Bundle Hash。
- 通过范围：本证据完成分集 Candidate 与 coverage，但不会创建 Episode、Published ScriptVersion 或 Planning 事实；Episode Plan Human Gate/Owner Apply、后续 Episode 分析、Shot/Canvas、完整原稿复合旅程和最终浏览器验收仍未完成。`agent-browser` 按约定只在全部开发完成后执行，本次未运行。
- Git：本 Evidence 与实现由描述有证据分集边界候选的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 分集候选审批与正式剧本切片原子发布（2026-08-28）

- Red、公共 Gate 与稳定输出：Planning Application 测试先稳定编译失败于缺少 Episode Plan Apply 命令与 Repository 契约。Green 后 Catalog 新增 `human.episode_plan_review@2.0.0`，HumanTask 以 `episode_plan_candidate` 语义冻结 exact Candidate id/hash/revision；批准输出不复用 Candidate 身份，而是引用 Apply Receipt 的不可变 `episode_set` id/version/hash。旧 v1 Definition 只保留为已发布历史版本，不作为兼容回退路径。
- Backend Owner 与单一事实源：Planning Application 重验 current Candidate Head、成功 Segmentation Invocation、DocumentRevision normalized hash、全文边界、marker、Evidence 与 blocking Review Issue；GORM Adapter 只负责授权、锁和持久化。批准后在一个共享 PostgreSQL/GORM 事务中创建全部 Episode、全部 `published` EpisodeScriptVersion、Episode current version pointer、每版本一个严格引用型 `ScriptVersionPublished` Outbox、项目 revision CAS 与 `episode_plan.apply` Receipt。没有 Migration、Raw SQL、第二 ORM、第二 SQL 事实源、Agent Writer 或 Kafka Command Topic。
- 原子性、冲突与零写入：真实 Workflow 在决议前确认 Episode 为零；测试把第二个 ScriptVersion ID 强制为首个 ID，事务在 Episode 已写而 Version batch 失败时整体回滚，Episode、Version、Outbox 与 Receipt 均保持零。批准后精确产生 3 个 Episode、3 个 Published ScriptVersion、3 个 Outbox 和 1 个 Receipt，区间按 code point 连续覆盖完整原稿。`rejected` 不调用 Owner；另一新命令遇到已存在 Episode 边界基线返回 conflict 且不创建 Receipt，证明未通过合并或覆盖兼容既有边界。
- 幂等、并发与恢复：同一 ReviewDecision/幂等键重放返回同一 Receipt 与同一排序 Episode Set；两个并发重放也只读取同一组 Owner 事实。Signal Apply 在 Owner Receipt 后重新读取每个 Episode/current Published Version 并重算 set hash，随后用同一 Decision 恢复 Temporal。实现同时移除了 Human Gate 将 Candidate subject revision 错误等同于 NodeRun revision 的隐藏耦合，Bible 与 Episode 两类 Candidate Gate 都以冻结证据和各自 revision 校验，不再依赖偶然相等的版本号。
- 真实 Workflow 与完整 CI：定向 PostgreSQL/Temporal 剧本 `TestSourceEvidenceAndStoryAnalysisWorkflowRecoverBoundedMapReduce` 在修正上述 revision 耦合后通过，耗时 `38.96s`；最终在全新 PostgreSQL `16.15`、Temporal、MinIO、Kafka `4.3.1` 与 Elasticsearch/Logstash/Kibana `9.4.4` 下执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `151.728s`。首次全量 Backend 尝试因验收环境使用了不存在的 MinIO access key 被真实资产测试拒绝，该轮中止且不计通过；清空全部专属 Volume、改为与 Compose 一致的测试凭据后从空库完整重跑通过。
- 跨项目与部署门：Agent Ruff check/format、Pyright 与 Pytest 为 `33 passed, 2 skipped`，两项 skip 都是需显式本地 Codex 登录的既有 opt-in 集成，本功能未修改 Agent；Frontend OpenAPI 零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试和 Next.js production build 全通过。开发/生产 Compose、Repository hygiene、Backend/Frontend/Agent 三镜像、Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 探针均通过。隔离部署完成日志脱敏检索，以及 Filebeat/Logstash/Kibana/Kafka/Elasticsearch 逐项停机时 Owner/Workflow 可用、Event Worker 正确降级与恢复、日志恢复摄取；首次故障命令仅因 zsh 不拆分带空格的命令变量而未执行并被中止，改为 shell 函数后从故障矩阵起点完整通过。
- 通过范围：以上证据完成 `SG-PRD-005` 与当前分集 Gate/Owner 项；覆盖七类 Gate 的复合 `SG-REV-006`–`008` 仍只完成 Bible 与 Episode 部分，保持未勾选。Episode 内 Scene/Dialogue/Beat/Occurrence/Claim 分析与审核、Core StoryGraph 多集编译、Shot/Canvas、完整原稿复合旅程和最终浏览器验收仍未完成；`agent-browser` 按约定只在全部开发完成后执行，本次未运行。
- Git：本 Evidence 与实现由描述审核后原子发布分集的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 有证据的分集结构候选与确定性归并（2026-08-28）

- Red 与跨语言契约：`agent/tests/contract/test_episode_analysis_contract.py`、`backend/tests/agent/episode_analysis_wire_test.go` 与 `backend/tests/production/planning/episode_analysis_test.go` 先固定正式 Episode/Published ScriptVersion、confirmed Bible Version、Materialization、Unicode code-point 区间、scene marker、相邻 Episode、Known Identity/State、map/reduce exact child 和 Evidence 守恒。Go 与 Pydantic 都严格拒绝 source revision/hash 漂移、越界 Evidence、未知身份/状态、缺失子 Candidate、乱序分片和业务写入形状；测试只位于独立 `agent/tests` 与 `backend/tests`。
- Definition-first 与职责：Authoring Catalog `10.0.0` 增加 `agent.episode_analysis@1.0.0`，只在 `episode_set` 与 `production_bible_materialization` 都可用后启动。Backend Planning Application 按每个已发布 Episode 生成确定性 map Manifest 与固定 fan-in=2 的 reduce tree，scene marker 优先切分，超长场次才硬切，相邻 Episode 只作为只读边界上下文；Agent `build-storygraph` Bundle 只返回 Scene/Dialogue/Beat/Occurrence/Claim Candidate，不拥有 GORM、Owner Command、Temporal、Kafka 或正式 UUID。
- 单一事实源、引用与恢复：Backend 从共享 PostgreSQL/GORM 读取并冻结 Episode ScriptVersion、Bible Version、Materialization 与 Asset/Specification/State exact ref，Manifest ID 由 NodeRun/stage 确定性派生，Invocation、Candidate Revision/Head 和最终 aggregate 全部写入既有 Agent 表；没有 Migration、Raw SQL、第二 ORM、第二 SQL 事实源、兼容入口或新业务 Writer。测试注入一次 Agent transport unknown 与一次结果持久化 unknown，均以相同 Invocation/Input/Stage identity 重新领取并成功；成功兄弟和历史 Candidate 不被清除。
- Evidence 与候选边界：模型给出的 Evidence 只有在绝对偏移、原文、Episode number 全部精确匹配冻结文本时，Harness 才确定性重算 SHA-256；偏移、原文或归并 child 漂移仍 fail closed。Backend 再次校验每个 Fragment/Claim 的 source range、exact anchor/hash、Known Identity/State、Scene anchor 与 child key 集合。最终只发布一个 `episode-planning-candidate-set-v1` aggregate，覆盖全部已发布 Episode root；真实数据库断言正式 `EpisodeStructure` 计数为 0，因此本项没有越过下一 Human Gate/Owner Apply。
- 真实 Codex 与 Workflow：本机登录 Codex CLI 后执行 `LANVERSE_TEST_REAL_CODEX=1 uv run pytest -q tests/integration/test_storygraph_real_codex.py -vv`，结果 `3 passed in 163.42s`，覆盖分集切分、Episode analyze/reconcile、Story analyze/reconcile/review/repair；新增 Episode 用例单独执行为 `1 passed, 2 deselected in 57.19s`。真实 PostgreSQL `16.15` 与固定摘要 Temporal 中，三集 `Script → Evidence → Bible → Materialization → Episode Gate/Published ScriptVersion → Episode Analysis` 旅程完成，map/reduce Invocation 均成功、至少一个 Invocation attempts≥2、aggregate revision/hash 与 Node output 一致、正式 Structure 为零；定向旅程最终 `PASS`，耗时 `47.350s`。
- 完整 CI：Agent Ruff check/format、Pyright 与 Pytest 全通过，结果 `35 passed, 3 skipped`；三个 skip 均为已由上述 opt-in 命令真实通过的 Codex 用例。Backend 使用全新 PostgreSQL 数据库、Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Filebeat/Logstash/Kibana `9.4.4` 执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `155.501s`。首次全量 Backend 运行暴露旧 Candidate Repair 旅程用无效 `analyze_episode` 依赖模拟 stale closure；改为两级合法 `reconcile_story` exact child 后，聚焦与全量空库复跑均通过，未添加兼容分支。
- Frontend、镜像与故障部署：`npm ci`、OpenAPI 生成/零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试与 Next.js `16.2.12` production build 全通过；开发/生产 Compose、Backend/Frontend/Agent 三镜像、Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 探针均通过。完整部署验证注册/项目写入、日志 request/trace 脱敏检索、Filebeat/Logstash/Kibana 停机不影响 Owner/Workflow、Kafka/Elasticsearch 停机时 Event Worker 正确 503 且 Backend/Workflow 保持可用、恢复后 readiness 与日志摄取收敛；前两次启动分别被宿主 `9000` 与 `5432` 占用在应用验证前阻断，端口审计后只改变宿主映射到 `59001`–`59004`，容器内部正式端口和同一故障剧本不变，最终完整通过。
- 通过范围：本证据完成 `SG-I15`、Episode 维度确定性分片/归并、Claim Candidate 和引用门禁；`SG-PRD-006` 仍包含下一项的全批 Owner Apply，因此保持未勾选。Scene/Beat/Occurrence/Claim Human Gate、正式 Planning 事实、Core StoryGraph 多集编译、Shot/Canvas、完整原稿与最终浏览器验收尚未完成；依据正式队列不提前运行 `agent-browser`。
- Git：本 Evidence 与实现由描述有证据分集结构候选的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 分集结构候选审批与正式规划事实整批发布（2026-08-28）

- Red 与完整边界：独立 `backend/tests/production/planning/episode_planning_owner_test.go` 先因缺少 Planning Owner 命令/返回契约而编译失败，再固定成功整批、Identity/State 错配零写入、批次故障全回滚、幂等重放与候选到正式事实反查。新测试只位于独立 `backend/tests` 目录，未与业务代码混放。
- 公共 Human Gate 与输出：Catalog `11.0.0` 新增 `human.episode_structure_review@2.0.0`，以 `planning_candidate` 冻结 exact aggregate id/hash/revision。`approved` 才执行 `Decision → Planning Owner Apply → Workflow Resume`，输出是独立 `planning_owner_set` 而非候选身份；第一次 Temporal Signal unknown 从同一 Decision/Receipt 恢复，不重复 Owner 事实。已发布的 v1 Gate 只作不可变历史 Catalog 记录，不是兼容回退。
- Backend Owner 与单一事实源：Application 以 Candidate 临时 key 和 EpisodeStructure ID 确定性生成正式片段 ID，并重验 Episode/current Published ScriptVersion、confirmed Bible/Materialization、实际 Asset/Specification/State、Evidence、Occurrence 参与者、Claim status/polarity/scope/anchor 与 blocking issue/conflict。GORM Adapter 只负责授权、锁、当前 Head/leaf 校验和事务持久化；未知身份/状态不自动创建。
- 原子批次、Receipt 与反查：全部 Scene、Dialogue、Beat、Occurrence、Claim 作为一个不可变 `EpisodeStructure` 聚合与 `episode_planning.apply` Receipt 在同一 PostgreSQL/GORM 事务中提交，Receipt 保存 Candidate 临时 key 到 Owner fragment ID 的完整映射。重放会重新读取正式聚合并校验 content hash/映射；注入批次失败时聚合、Receipt 与保存计数全部为零。复用已有 `EpisodeStructure` JSONB 事实，没有 Migration、Raw SQL、新表、第二 ORM、第二 SQL 事实源、Agent Writer 或 Kafka Command Topic。
- 真实 Workflow 与 CI：真实 PostgreSQL `16.15`/Temporal 的 `TestSourceEvidenceAndStoryAnalysisWorkflowRecoverBoundedMapReduce` 证明决议前正式结构为零、批准后整批事实与唯一 Receipt、unknown 恢复和反向 Evidence，定向结果 `PASS` 耗时 `45.722s`。最终从空库接入 Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Filebeat/Logstash/Kibana `9.4.4` 执行 `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `149.562s`。首轮因本地 MinIO 重启时误用凭据被资产测试真实拒绝，不计通过；后续全量还暴露 Bible Candidate stale Receipt 返回顺序不稳定，固定排序并在空库重跑后才记录通过，未使用 skip 或兼容分支。
- 跨项目与部署门：Agent Ruff check/format、Pyright 与 Pytest 为 `35 passed, 3 skipped`，三个 skip 是既有需显式本地 Codex 登录的 opt-in 集成，且已在上一个候选交付中真实通过；本次未修改 Agent。Frontend OpenAPI 零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试与 Next.js production build 全通过。开发/生产 Compose、三镜像、Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 探针均通过；隔离部署完成日志脱敏检索与 Filebeat/Logstash/Kibana/Kafka/Elasticsearch 逐项停机、降级、恢复门禁，临时容器、网络、卷和测试库已清理。
- 通过范围：以上证据完成 `SG-PRD-006` 与当前 Planning Gate/Owner 整批发布。覆盖七类 Gate 的复合 `SG-REV-006`–`008`、派生角色/地点卡的 `SG-FE-002` 与 Core StoryGraph 编译仍未完成，保持未勾选。`agent-browser` 依约只在全部开发完成后运行，本次未提前执行。
- Git：实现、测试与 Evidence 由分别描述回执顺序稳定、候选整批发布、审批恢复与验收事实的完整提交承载；提交标题和正文不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 多集正式内容图编译与工作流发布（2026-08-28）

- Red 与精确输入：独立 `backend/tests/storygraph/owner_set_test.go` 先证明 Compiler 会错误接受 required Planning Owner 之外的额外 Head，再固定完整 Planning Owner Set 必须与数据库当前 confirmed Head 精确相等，禁止缺失、额外、重复或错误 owner kind。相同输入/幂等键只返回同一 Version/Receipt/Outbox；同键不同输入返回 `resource_conflict` 且零新增 Version。全部测试只位于 `backend/tests`，未与业务代码混放。
- Definition-first 与职责：Authoring Catalog `12.0.0` 新增 `production.storygraph_compile@1.0.0`，真实 Temporal Graph 只允许它在 `planning_owner_set` 成功后执行。Workflow Production Executor 从 Planning Receipt 反查完整正式 Owner Set，传入 exact Bible Version/hash 与稳定幂等键；Agent 不编译、不发布、不写 Owner，Kafka 不承载 Command。
- 完整 Owner 投影与 Claim DAG：GORM Adapter 在 StoryGraph 发布事务的一致快照内重验唯一 active Script/current DocumentRevision、全部 active Episode/current published ScriptVersion、每集 exact confirmed EpisodeStructure、confirmed Bible Version、Materialization Receipt 及实际 Asset/Specification/State/ProductionBinding。Compiler 投影 SourceEvidence、Episode、Scene、Dialogue、NarrativeBeat、Identity、Specification、State、Binding、Occurrence、WorldRule、StoryArc、Bible Relationship/Foreshadowing Claim 与 Planning Causal/Continuity Claim；每个节点冻结唯一 Owner version/revision/hash 和 Evidence。Bible Candidate anchor 不冒充未来正式键，正式 `claim_anchor` 由 Claim Evidence 与 confirmed Scene Evidence 的精确区间重叠确定；未知 Identity/State、错配 Binding、Evidence 未覆盖、Claim scope/participant/anchor 错误或任意环都 fail closed。
- 原子发布、查询与对账：Core Snapshot 经稳定 Key、Canonical Hash 和 Kahn 拓扑排序后，在已有单一 PostgreSQL/GORM `SERIALIZABLE` 事务原子写入一个不可变 StoryGraphVersion、唯一 Head、Command Receipt 与 `StoryGraphVersionPublished` Outbox；没有 Migration、Raw SQL、第二 ORM、第二 SQL/Graph 事实源或兼容入口。真实 Workflow 对同一 unknown 结果用同一幂等键重放，返回同一 Version/content hash/Receipt；同键漂移输入被拒绝且 Version 仍为 1。发布后直接通过 GORM QueryService 对实际版本执行 Identity Impact Lens 和 Scene downstream Trace，分别命中 Relationship Claim 及 Occurrence/Causal Claim；跨版本 add/remove/change 继续由稳定键 Diff golden 覆盖。
- 定向真实验收：`LANVERSE_TEST_DATABASE_URL=... LANVERSE_TEST_TEMPORAL_ADDRESS=... go test ./tests/workflow -run '^TestSourceEvidenceAndStoryAnalysisWorkflowRecoverBoundedMapReduce$' -count=1 -p 1 -v` 在 PostgreSQL `16.15` 与固定摘要 Temporal 上通过，耗时 `47.278s`。两集以上正式链完成 `Script → Evidence → Bible/Identity/State/Binding → Episode/Scene/Occurrence/Claim → StoryGraphVersion`，Graph 为 DAG，并能真实查询 Impact/Trace。`LANVERSE_TEST_DATABASE_URL=... go test ./tests/storygraph -count=1 -p 1 -v` 全通过，耗时 `20.200s`，覆盖 PostgreSQL 发布、并发 CAS、事务回滚、权限、五类 Lens、Diff 与 cursor。
- 完整 CI：清空并重建专属 PostgreSQL、Temporal、MinIO、Kafka `4.3.1`、Elasticsearch/Filebeat/Logstash/Kibana `9.4.4` 后执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `155.824s`。第一次全量命令复用了专项测试数据库，严格全库计数真实失败；该轮不计通过，也未放宽断言，改用全新数据库及全新 Kafka/ELK/MinIO 后从头完整通过。
- 跨项目与部署门：Agent 在临时 Python `3.11` 环境执行 Ruff check/format、Pyright 与 Pytest，结果 `35 passed, 3 skipped`；三个 skip 是 CI 既有的显式真实 Codex opt-in 用例，之前已单独通过，本功能未修改 Agent。Frontend OpenAPI 生成零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试与 Next.js `16.2.12` production build 全通过。开发/生产 Compose、Repository hygiene、Backend/Frontend/Agent 三镜像、Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 探针全部通过；隔离部署完成日志脱敏检索，以及 Filebeat/Logstash/Kibana/Kafka/Elasticsearch 逐项停机、降级、恢复和日志恢复摄取，Frontend、Backend、Workflow Worker 与独立 Agent health 均通过。
- 通过范围：以上证据完成 Core StoryGraph 的多集编译、发布后 Diff/Impact/Trace 与 `SG-I17`。`SG-PRD-012` 还要求后续正式 Shot Owner Apply 后再次按 expected graph hash 编译，因此保持未勾选；Storyboard/视觉资产/Canvas、完整原稿复合旅程和最终浏览器验收仍未完成。`agent-browser` 依约只在全部开发完成后运行，本次未提前执行。
- Git：实现、测试与 Evidence 由描述正式内容图编译和工作流发布的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 可审核分镜意图候选与缺资产门禁（2026-08-28）

- Red 与跨语言契约：独立 `agent/tests/contract/test_storygraph_storyboard.py`、`backend/tests/agent/storyboard_draft_contract_test.go` 与真实 Workflow 旅程先固定 exact StoryGraph Version、冻结 Style Snapshot、Scene/Beat/Dialogue/Occurrence/Identity/Specification/State、Evidence、角色/场景/道具视觉角色与视图需求。Go 与 Pydantic 都严格拒绝未知字段、跨 Scene Evidence、遗漏 required Beat/Occurrence、身份或状态漂移、伪造 AssetVersion、错误 visual role/view role 和 Agent 输出正式 Shot 的形状；测试只位于 `agent/tests` 与 `backend/tests`。
- Definition-first 与职责：当前 Catalog 新增 `agent.storyboard_draft@2.0.0`，只接受已发布 `storygraph_version` 并输出 `storyboard_intent_candidate_set`。Backend 按正式 Scene 生成一个持久 shard，冻结 Graph/style/manifest/input/stage identity，创建 Scene Candidate Revision 与一个 aggregate Candidate Revision；Agent `build-storygraph` Bundle 只生成 Shot Intent、视觉需求、风险和审核问题，不生成正式 UUID、timecode、Shot、Binding、Owner Command、Provider Job 或 Artifact。
- 单一事实源与资产门禁：Backend 只通过共享 PostgreSQL/GORM 读取发布 Graph 与正式 Asset/Specification/State，并在同一事务持久化 Manifest、Draft Set、Batch、Invocation 与 Candidate Revision；没有 Migration、Raw SQL、第二 ORM、第二 SQL 事实源、Agent Writer 或兼容入口。当前 StoryGraph 尚无 READY reference AssetVersion 时，Candidate 必须逐项返回 `needs_asset`，aggregate 也保持 `needs_asset`；真实数据库精确断言 Shot、ShotProductionBinding、Cost、Quota、Provider Job、Artifact 和新增 StoryGraphVersion 均为零。
- 真实 Codex 与 Skill：本机登录 Codex CLI 后执行 `LANVERSE_TEST_REAL_CODEX=1 .venv/bin/pytest -q -v tests/integration/test_storygraph_real_codex.py::test_real_codex_drafts_reviewable_intent_without_creating_shot`，结果 `1 passed in 24.87s`。首次模型输出了错误 visual role/view role 组合并被 Harness 正确拒绝；收紧唯一 StoryGraph Skill Reference 后原剧本通过。`quick_validate.py` 返回 `Skill is valid!`，当前 Bundle Hash 为 `352d46c51661e7d989b42ddeb0a0ff0a4b48165e8e3f7700f3e60d170e4c58cb`。
- 完整 CI：最终 Backend 在全新 PostgreSQL `16.15`、Temporal、MinIO、Kafka `4.3.1` 与 Elasticsearch/Logstash/Kibana `9.4.4` 下执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包耗时 `276.974s`，没有外部依赖 skip。Agent Ruff check/format、Pyright 与 Pytest 全通过，结果 `39 passed, 4 skipped`；四项都是显式 opt-in 的真实 Codex 集成，其中本功能用例已由上述命令单独通过。Frontend OpenAPI 生成零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试及 Next.js production build 全通过。
- 镜像、部署与故障：开发/生产 Compose、Backend/Frontend/Agent 三镜像、Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle Hash 探针均通过。隔离完整部署完成注册、项目写入、Workflow、日志 request/trace 脱敏检索，以及 Filebeat、Logstash、Kibana、Kafka、Elasticsearch 逐项停机、降级、恢复和恢复后日志继续摄取，最终输出 `deployment-fault-ci=passed`；专属容器、网络和 Volume 已精确清理。
- 通过范围：以上证据完成 `SG-PRD-007`、`SGA-STG-006` 与 `SG-I18`。包含后续 detail/Owner Apply 的 `SGA-STG-008`、完整真实 Codex 矩阵 `SGA-OPS-004`、跨阶段视觉资产旅程及 Storyboard Gate 仍保持未通过。`agent-browser` 依约只在全部开发完成后运行，本次未提前执行。
- Git：实现、测试与 Evidence 由描述生成可审核分镜意图的 feature 提交承载；提交标题和正文只表达 feature，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

### 已审核分镜意图冻结与付费前人工门禁（2026-08-28）

- Red 与公开契约：`backend/tests/authoring/graph_contract_test.go`、`backend/tests/workflow/signal_domain_test.go`、`backend/tests/workflow/signal_service_test.go`、`backend/tests/workflow/source_evidence_worker_test.go` 与独立 `backend/tests/production/storyboard/intent_freeze_test.go` 先固定 `storyboard_intent_candidate_set → approved_storyboard_intents`、`approved` 正向决议、`rejected|changes_requested` 零 Owner 调用、完整 Intent/visual requirements 冻结、稳定 Hash 和可恢复 Receipt。测试只位于 `backend/tests`，未与业务代码混放。
- 公共 Gate 与 Backend Owner：Catalog `14.0.0` 发布 `human.storyboard_review@2.0.0`，HumanTask subject 精确绑定 aggregate Candidate id/hash/revision。正向 Coordinator 只调用 Storyboard `FreezeIntentSet`；Owner 在单一 GORM 事务中锁定并重验 current aggregate/leaf Head、不可变 StoryGraphVersion、ShardManifest、每个 Batch/Agent Invocation/Stage Input、Draft Set baseline 和已完成的 approved ReviewDecision，再把完整 Shot Intent 与 visual requirements 写入 `approved-storyboard-intents-v1` Command Receipt，并将同一 Draft Set 从 `needs_asset` 推进到 `intent_frozen` 一次。
- 漂移、幂等与恢复：Decision 后注入 Draft Set revision 漂移会返回 Workflow `409` conflict，保留不可变 Decision，但 Apply Receipt 为 conflict 且 Freeze Receipt 为零；正常路径先提交 Owner Receipt，再模拟 Temporal Signal outcome unknown，以同一 Decision/Intent 第二次恢复 completed。直接重复调用 Owner 返回同一 Receipt、同一 approved content hash 和同一 revision，数据库始终只有一条 `storyboard.freeze_intent_set` Receipt；回放会按冻结前 Draft Set revision 重建 Candidate，并用 canonical JSON hash 校验 PostgreSQL `jsonb` 回执，不依赖字段字节顺序。
- 单一事实源与零副作用：只扩展既有 Storyboard GORM Model 状态约束和共享 Command Receipt，没有 Migration、Raw SQL、第二 ORM、第二数据库连接、Agent Writer、Kafka Command 或新服务。批准后数据库精确断言正式 Shot、ShotImageBinding、Generation Intent/Provider Job、Cost Estimate/Reservation、Quota Reservation、Artifact 均为零，StoryGraphVersion 仍恰好 1；拒绝、要求修改、baseline conflict 和 Signal unknown 均不产生付费或 Provider 副作用。
- 定向与完整 CI：真实 PostgreSQL `16.15` 与固定摘要 Temporal 的多集 `Script → Evidence → Bible → Episode/Planning → Core StoryGraph → Storyboard Draft → Intent Gate` 旅程通过；最终加入直接 Owner 重放后，在固定本机 PostgreSQL `18.4` 与常驻 Docker Temporal 上复跑为 `PASS 61.888s`。随后重建隔离 `lanverse_ci` 数据库，连接本机 MinIO 与常驻 Kafka `4.3.1`、Elasticsearch/Logstash/Kibana `9.4.4`，执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全通过，Workflow 包 `140.690s`，没有外部依赖 skip。
- 跨项目、镜像与故障部署：Agent Ruff check/format、Pyright 与 Pytest 全通过，结果 `39 passed, 4 skipped`；四项是 CI 既有的显式真实 Codex opt-in 用例，本功能未修改 Agent。Frontend OpenAPI 生成零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试及 Next.js production build 全通过。Backend/Frontend/Agent 三镜像、Backend 三 Binary、Frontend standalone、Agent 非 root/固定 Codex/唯一 Bundle 均通过；常驻 Compose 使用本机已安装 MinIO，不启动重复 MinIO 容器。最终故障矩阵依次停止/恢复 Filebeat、Logstash、Kibana、Kafka、Elasticsearch，验证日志脱敏检索、Owner/Workflow 可用、Event Worker 503 降级及恢复后日志继续摄取，输出 `deployment-fault-ci=passed`，退出后全部常驻服务健康。
- 失败披露与范围：真实实现曾分别暴露 Candidate ID/Draft Set ID 混用、冻结后 revision 回放、PostgreSQL `jsonb` 字段顺序以及最终镜像更新时 Event Worker 首次 DNS 未就绪；均修正根因并从完整旅程或故障矩阵起点复跑，失败轮次不计通过。以上证据完成 `SG-PRD-008` 与当前 Intent Gate；覆盖七类 Gate 的复合 `SG-REV-006`–`008` 仍待后续视觉选择和正式 Shot Gate，因此保持未勾选。reference asset 生成、三视图 QC/选择、detail shot、正式 Shot/Binding/Graph、Canvas、完整原稿与最终浏览器验收尚未完成；`agent-browser` 本次未运行。
- Git：实现、测试与 Evidence 由描述冻结已审核分镜意图的 feature 提交承载；提交标题和正文只表达功能，不包含任务编号、任务名、阶段名或内部计划名；未推送、未创建 PR。

`SG-D21` 建立时 188 个 Checklist 全部未勾选；当前已按新证据完成 `SG-I01`–`SG-I19`，其余保持未通过。下一步只消费 `approved_storyboard_intents` 实现 reference asset 的 Cost/Quota/Provider Job/Artifact 执行与 unknown 对账；在此之前不得启动 detail shot、正式 Shot Apply、Canvas 写入或 `agent-browser`。

### GenerationTarget 与 Runware 离线任务合同（2026-08-29）

- Red 与独立测试：新增 `backend/tests/generation/target_contract_test.go` 与 `backend/tests/generation/runware_adapter_test.go`，先因生产 Runware Adapter 包缺失而编译失败；处理中/部分结果用例又先以 `Runware result count drifted` 失败，随后实现严格分类并复跑为 Green。测试只位于 `backend/tests`，未与业务代码混放。
- Target 合同：Backend Domain 新增只允许 `reference_asset|shot_frame` 的严格联合类型，对 Owner Ref、Revision/Content Hash、Prompt Version、尺寸、PNG 和候选数做确定性规范化；`reference_asset` 首版固定 Character composite `reference_sheet` 与 `front/profile/back`，Target Hash 不包含 Target ID、创建人、创建时间等非生成输入状态，且持久化回读漂移会 fail closed。
- Runware 合同：2026-08-29 重新核验官方 [Task Polling](https://runware.ai/docs/platform/task-polling) 与 [Task Details](https://runware.ai/docs/platform/task-details)。Adapter 固定官方 HTTPS Endpoint、`runware:z-image@turbo`、Bearer Header、`deliveryMethod=async` 和 Backend Provider Job UUID；`processing`/部分成功保持 `accepted` 且不提前 Staging，`taskNotFound` 保持 `unknown`，明确 Provider Error 映射稳定失败码，传输失败不自动重提。`getResponse/getTaskDetails` 只使用同一 UUID，历史请求必须与冻结 Target 等价；输出数、task/image UUID、官方 Host 和 Staging metadata 任一漂移均拒绝。API Key 只进 Authorization Header，测试验证其不进入 JSON 或错误。
- 本轮验证：`go test -count=1 ./tests/generation` 通过；`test -z "$(gofmt -l .)" && go vet ./... && go test -count=1 -p 1 ./...` 通过，但未注入 `LANVERSE_TEST_*`，不计外部服务验收。Agent Ruff check/format、Pyright 与 Pytest 通过，结果 `39 passed, 4 skipped`，skip 为显式真实 Codex opt-in；Frontend OpenAPI 生成零漂移、lint、typecheck、18 个 Vitest 文件 54 项测试与 production build 全通过。本机现有 Logstash `127.0.0.1:5000` TCP 可连接，本轮未启动、重启或增加 Filebeat/Logstash 服务。
- 未通过范围：当前进程中 `RUNWARE_API_KEY`、`LANVERSE_TEST_DATABASE_URL`、`LANVERSE_TEST_MINIO_ENDPOINT` 与 `LANVERSE_TEST_TEMPORAL_ADDRESS` 均未配置；因此本证据只是已接受设计的第一个离线交付单元，不勾选 `SG-VIS-001`–`005`、`SG-OPS-002` 或 `SG-I20`。GenerationTarget GORM Catalog/Repository、approved intent 消费、Cost/Quota/Authorization/Job 前置、安全下载、私有 MinIO Staging、Artifact 物化、Temporal 恢复与真实 Runware 任务仍是当前项的后续必做范围；`agent-browser` 未提前执行。
- Git：本 Evidence 与实现由描述 Runware 图片任务合同的完整功能提交承载；提交标题和正文不包含任务编号、任务名或阶段名，未推送、未创建 PR。
