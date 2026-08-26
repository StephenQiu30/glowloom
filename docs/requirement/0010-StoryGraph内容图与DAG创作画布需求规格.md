# StoryGraph 内容图与 DAG 创作画布需求规格

> 状态：跨服务 Requirement 已接受（`SG-D18`，2026-08-27）；全部条款初始待实施、待验收
>
> 产品依据：[StoryGraph 产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md)
>
> 设计依据：[StoryGraph 总设计](../design/0010-StoryGraph内容图与DAG创作画布设计.md) · [Backend 领域设计](../design/2002-后端领域模块功能设计.md) · [公共 Human Gate](../design/2055-Workflow公共HumanGate命令与恢复设计.md) · [前端应用架构](../design/1001-前端应用架构.md)
>
> Agent 专项契约：由 `SG-D19` 单独建立，不在本文复制 Harness 内部协议

## 1. 规格约定

本文是 StoryGraph MVP 的跨 Backend、Workflow、Review、Asset/Generation、Kafka/Search/ELK 和 Frontend 唯一可测契约。每条“必须/不得”均为尚未完成的目标；只有 `SG-D21` Acceptance 记录当次真实证据后才能判定通过。历史代码、测试或验收只用于识别当前事实，不能自动满足本规格。

验证类型：

- `contract`：Schema、Hash、OpenAPI、事件和跨语言 fixture；
- `unit`：纯领域规则与确定性算法；
- `integration`：真实 PostgreSQL/GORM、Temporal、Kafka、Elasticsearch、对象存储或 Provider Staging；
- `e2e`：跨服务真实业务旅程；
- `architecture`：导入、Writer、SQL/ORM、测试目录和禁止依赖；
- `manual`：只能补充语义质量判断，不能替代自动化不变量。

## 2. 全局架构与事实源

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-ARC-001` | Backend Go Module 是 Production、StoryGraph、Workflow 业务投影、Review、Generation、Asset、Cost/Quota、Outbox 和 Search 索引协调的唯一业务 Writer。 | `architecture`：禁止 Frontend/Agent/event-worker 直接写 Owner；跨边界负向调用。 |
| `SG-ARC-002` | PostgreSQL 是唯一 SQL 事实源；全部业务表只由一份 GORM Model Catalog 定义并同步。不得存在 Migration 文件/版本/Checksum/Source 元数据、Raw SQL 业务 Schema、第二 ORM、第二数据库 Writer 或兼容双写。 | `architecture + integration`：目录扫描、依赖扫描、空库 Catalog Sync、Model/约束核查。 |
| `SG-ARC-003` | Domain/Application 不得导入 GORM、Temporal、Kafka、Elasticsearch、对象存储或 Provider SDK；Adapter 实现 Port，Composition Root 负责唯一装配。 | Go import/AST 架构测试。 |
| `SG-ARC-004` | Temporal 是唯一跨步骤持久 Workflow/Timer/Human Wait/Signal 引擎；Kafka 不调度 Workflow，数据库轮询不建立第二工作流。 | 架构扫描；真实 Temporal wait/signal/replay/restart。 |
| `SG-ARC-005` | StoryGraph、Authoring Graph、WorkflowDefinition 和 Temporal History 使用不同 Schema、ID、Owner 和存储，不能经万能 Graph DTO/表互相写入。 | `contract + architecture`：ID/DTO 不可互换负向测试。 |
| `SG-ARC-006` | Python Agent 是 Backend 私有 Candidate Runtime，只返回严格 Candidate/Repair；不得接收数据库、Kafka、Elasticsearch、Temporal、对象存储、JWT 或 Provider 业务凭据，也不得写业务 API。 | Agent 环境/依赖/网络/Tool allowlist 与 Backend 权限测试；细则见 `SG-D19`。 |
| `SG-ARC-007` | Browser 只能调用 Backend `/api/v1`；不得直连 PostgreSQL、Temporal、Kafka、Elasticsearch、ELK、Agent 或 Runware。 | Frontend bundle/import/网络 E2E。 |
| `SG-ARC-008` | 每个跨外部边界操作必须使用稳定 Intent/Request/Receipt ID，区分 known failure 与 outcome unknown；UNKNOWN 只能按原身份查询/重放，不能盲目重提。 | 故障注入矩阵。 |
| `SG-ARC-009` | 所有 Workspace 资源 Query/Command 必须基于当前 Token Version/Membership 重新授权；跨租户不存在与无权按防枚举策略一致处理。 | 全 API 权限矩阵。 |
| `SG-ARC-010` | 不为未来能力预建空目录、Binary、Topic、Index、通用 Repository、兼容层或微服务；新结构必须和首个真实消费者、测试及运行装配同任务交付。 | Diff/运行清单/架构审查。 |

## 3. StoryGraph 数据与编译

### 3.1 不可变版本契约

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-GRF-001` | `StoryGraphVersion` 至少冻结 `id/workspace_id/project_id/version_no/parent_version_id/schema_version/owner_set_hash/content_hash/nodes/edges/published_at/created_by`；发布后不得原地修改。 | GORM 空库、约束、更新拒绝与历史读取。 |
| `SG-GRF-002` | `StoryGraphHead` 每个 Project 恰有一个 current version，更新必须校验 expected head/version/hash；并发发布最多一个成功，失败事务不得留下 Version/Head/Outbox 半成品。 | 真实 PostgreSQL 并发/CAS/回滚。 |
| `SG-GRF-003` | Node 至少包含稳定 `story_node_key`、`node_type`、精确 `owner_ref`、严格 payload 和排序 Evidence refs；Owner Ref 必须同时包含 `owner_logical_id + fragment_key` 与 `owner_version_id + revision + content_hash`。 | Schema/golden/缺字段负向测试。 |
| `SG-GRF-004` | Edge 至少包含稳定 `edge_key/edge_type/source_story_node_key/target_story_node_key/qualifier`；边方向与类型矩阵以总设计为唯一来源，未知类型/端点组合必须拒绝。 | 全类型正反 contract fixture。 |
| `SG-GRF-005` | `story_node_key` 只由 node type、owner kind、owner logical ID、fragment key 确定性生成；精确 Owner 版本变化不得造成逻辑节点全量删增。`edge_key` 同理由类型、稳定端点和 qualifier 生成。 | 跨版本稳定性/Diff fixture。 |
| `SG-GRF-006` | Canonical Hash 必须对 Schema 版本、排序后的 Node/Edge、Owner Ref/Payload/Evidence 计算；JSON 键序、输入遍历顺序和 Canvas layout/viewport 不得改变 Hash。 | Go/跨语言 golden、随机输入顺序。 |
| `SG-GRF-007` | 所有权威 Edge 必须构成 DAG；环检测返回可解释的最小环路径。天然可成环的关系只能由带 participant/anchor/evidence/scope 的 Claim Node 表达。 | 拓扑/环/Claim payload-edge 一致性测试。 |
| `SG-GRF-008` | Compiler 只读取已确认的正式 Owner 事实与精确版本；Agent Candidate、HumanTask、ReviewDecision、Search 文档、Kafka Event 和 Canvas View Model 不得被编入正式 Version。 | Compiler source fixture 与污染负向测试。 |
| `SG-GRF-009` | Compiler 在一个 GORM 事务中冻结 Owner Set、校验引用/类型/DAG/Hash、写 Version/Head/Command Receipt/`StoryGraphVersionPublished` Outbox；任一步失败全回滚。 | 真实 PostgreSQL 分段故障注入。 |
| `SG-GRF-010` | 首版持久化使用 PostgreSQL/GORM 不可变 JSONB Version + Head；不得引入图数据库、通用关系表、EAV、Raw SQL 递归图查询或第二 Graph Writer。 | 依赖/Schema/代码扫描。 |

### 3.2 Query 契约

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-QRY-001` | Backend 必须提供 Current Version、Exact Version、Lens、Version Diff、Upstream Trace、Downstream Trace/Impact Closure 的只读 Application Query，并在 OpenAPI 暴露前完成授权。 | Application + HTTP contract/integration。 |
| `SG-QRY-002` | Lens Query 必须显式绑定 project、exact version 或 `current`、lens、scope kind/id、depth/cursor；响应返回实际 Version ID/Hash、scope、nodes/edges、`truncated` 和稳定继续条件。 | 参数组合、分页/截断与 current 漂移测试。 |
| `SG-QRY-003` | `outline/narrative/entity/production/impact` 默认有界；不得下载全项目后在前端筛选。相同版本/参数必须返回确定性排序和相同结果 Hash。 | 大中小 fixture、响应边界/Hash。 |
| `SG-QRY-004` | Diff 使用稳定 Node/Edge Key 区分 add/remove/change；Owner exact version/hash 变化是 change，不伪装成 remove+add。 | 跨版本 golden。 |
| `SG-QRY-005` | Query 不得写 Owner、Head、缓存业务事实或 Search 索引；Search 不可用不影响 PostgreSQL StoryGraph Query。 | Query 前后数据库计数与依赖故障。 |

## 4. 公共 Review 与 Workflow Resume

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-REV-001` | HumanTask 从已提交 Gate `node-input-v1` 建立并冻结 workflow/node、`subject_type/id/revision/hash`、排序 candidate IDs 和 rubric version；客户端不能自报候选。 | Gate Input/Task 重放与篡改测试。 |
| `SG-REV-002` | 公共 API 必须提供项目 Task 列表/详情、Claim/Renew/Release、Decision 和按 ReviewDecision ID Resume；OpenAPI/生成 Frontend Client 不得漂移。 | HTTP contract、client drift。 |
| `SG-REV-003` | Claim/Renew/Release 必须校验 Actor、Task revision、token、expiry、幂等键；Claim Token 只返回当前未过期 Owner，且不得进入 URL/日志/列表。 | PostgreSQL Lease/权限/泄漏测试。 |
| `SG-REV-004` | ReviewDecision 必须不可变并仅允许 `approved/rejected/changes_requested/selected`；Subject/Rubric 决定允许集合；selected 恰有一个且属于冻结 Candidate。 | Decision 状态/候选矩阵。 |
| `SG-REV-005` | API/Frontend 必须分开表示 `decision_status`、`owner_apply_status` 和 `workflow_resume_status`；Decision recorded 不得显示为 Owner/Workflow 完成。 | HTTP DTO + Frontend component/E2E。 |
| `SG-REV-006` | 正向 Decision 必须先调用显式 Owner Applier 并冻结真实 Owner Receipt/canonical Gate Output/Hash，再准备 Workflow Signal；拒绝/修改为 `owner_apply_status=not_required`，不得产生正向 Owner 写入。 | 七类 Gate owner/zero-side-effect 测试。 |
| `SG-REV-007` | Resume 只按已持久化 Decision ID 继续缺失阶段，不能改 Decision/Subject/Candidate 或跳过 Owner Receipt；并发/重复 Resume 收敛到同一 Apply/Signal Receipt。 | API/Worker 重启、并发、UNKNOWN/AlreadyApplied。 |
| `SG-REV-008` | Decision 前 Subject head/hash 漂移使 Task STALE；Decision 后 Owner baseline 确定性冲突保留旧 Decision 且不得套用新事实，必须新建 Candidate/Task。 | Stale/冲突旅程。 |

## 5. Production Owner 与 StoryGraph 主链

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-PRD-001` | SourceEvidence 必须绑定 DocumentRevision、来源片段类型和 Unicode 绝对区间；区间可回读原文且不能跨越/遗漏所属输入。 | 至少两集 Unicode fixture/coverage。 |
| `SG-PRD-002` | Production Bible `Confirm` 只创建/返回不可变 ProductionBibleVersion 和 Command Receipt；不得创建 Asset、AssetState、AssetVersion、Episode、Shot 或 StoryGraphVersion。 | 真实 PostgreSQL 事实计数。 |
| `SG-PRD-003` | `MaterializeConfirmedBible` 必须读取精确 BibleVersion，在一个共享 GORM 事务中创建/复用唯一 Character/Location/Prop Asset、SpecificationVersion、AssetState、ProductionBinding 和 Materialization Receipt；失败全回滚。 | 幂等/冲突/全回滚/反查。 |
| `SG-PRD-004` | Identity 只可按精确 Owner Ref 或经审核的人工作用链接复用；不得按名称/别名相似度自动合并。 | 同名/别名/跨集负向测试。 |
| `SG-PRD-005` | Episode Plan 正向 Gate 必须原子物化全部 Episode 与 Published EpisodeScriptVersion；边界冲突或部分失败不得留下部分集合。 | 多集边界/幂等/回滚。 |
| `SG-PRD-006` | Planning Candidate 正向 Gate 必须全批应用 Scene/Dialogue/Beat/Occurrence/Claim；未知 Identity/State、Evidence 缺失或 Claim scope 不合法时全批拒绝。 | 多集 Owner Set/负向/回滚。 |
| `SG-PRD-007` | Storyboard Draft 只消费已确认 Scene/Beat/Occurrence、非空 Specification/AssetState 和冻结 Style；缺视觉输入必须产生合法 `needs_asset`，不得创建正式 Shot。 | Input/候选/事实计数。 |
| `SG-PRD-008` | `FreezeIntentSet` 只冻结 Draft Set revision/hash、accepted Intent、视觉需求、Decision 和 Receipt，输出 `approved_storyboard_intents`；不得创建 Shot、Cost/Quota Reservation、Provider Job 或 StoryGraphVersion。 | 付费前 Gate/零副作用。 |
| `SG-PRD-009` | `detail_shots` 只消费 Intent Gate 输出和精确 READY AssetVersion Ref；必须冻结 Asset/State/Version/Artifact/Lineage/Style/view-role Hash，不得读取 current/latest 指针。 | 精确版本/漂移/空绑定。 |
| `SG-PRD-010` | Storyboard Detail 正向 Gate 必须在一个 GORM 事务中全批创建正式 Shot、完整 ShotProductionBindingVersion 和 Receipt；任何 Episode/Asset/Hash 漂移全回滚。 | 全批、并发、重放、反查。 |
| `SG-PRD-011` | `ProductionBinding`、`ShotProductionBindingVersion`、`ShotImageBindingVersion` 必须为不同 Owner/Record/语义，任何 API/DTO/Graph Node 不得混用。 | Schema/应用/Graph contract 负向测试。 |
| `SG-PRD-012` | 每次相关 Owner Apply 成功后，StoryGraph Compiler 必须用 expected current graph hash 发布下一 Version；Owner 成功但编译未知时按相同 Receipt/Hash 对账，不创建第二 Version。 | 崩溃窗口与重放。 |

## 6. Asset、Generation 与 Runware

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-VIS-001` | GenerationTarget 只允许严格 `reference_asset` 与 `shot_frame` union；字段、Owner Ref 和 Target Hash 按 kind 校验，不能走 shot-only 兼容入口。 | JSONB/Domain/HTTP contract。 |
| `SG-VIS-002` | `reference_asset` 必须冻结 Asset identity、SpecificationVersion、AssetState、EffectiveStyleSnapshot 和目标 view roles；首个付费闭环优先于 `shot_frame`。 | Target Hash/顺序/前置 Gate。 |
| `SG-VIS-003` | Runware 提交使用稳定 Task UUID/Request identity；accepted、known failed、succeeded、outcome unknown 分开，UNKNOWN 只用同一 Task 查询 `getResponse/getTaskDetails`，不得盲目重提。 | 凭据 Staging + 故障代理；官方协议 contract。 |
| `SG-VIS-004` | Provider 远程调用前必须已有 Generation Intent、Cost Estimate/Reservation、Quota Reservation、Execution Claim/Authorization 和已提交 Provider Job；任一前置失败不调用 Provider。 | 真实 PostgreSQL + fake protocol/credentialed staging 计数。 |
| `SG-VIS-005` | Provider 成功输出必须进入私有 Staging，并由 Backend 重验字节、SHA-256、大小、媒体类型、解码和尺寸后进入 Artifact READY/QUARANTINED；URL 不是 Artifact。 | MinIO/损坏/SSRF/元数据漂移。 |
| `SG-VIS-006` | composite reference sheet 首版固定一个 Artifact 覆盖 front/profile/back 语义区域；缺失/重复 role、输入 Hash 不同、非 READY 或 lineage 不完整时不得发布 AssetVersion。 | QC/coverage/selection 负向矩阵。 |
| `SG-VIS-007` | 确定性 QC 只判定元数据、格式、尺寸、coverage 等可证明规则；身份、服装、比例和画风一致性必须由 Human CandidateSelection 审核，不得伪装成自动通过。 | QC 报告/Review fixture + manual 补充。 |
| `SG-VIS-008` | CandidateSelection 只从 HumanTask 冻结且 READY/QC 合法的 CandidateSet 产生；单 Task 恰有一个不可变 Selection，重复/并发收敛。 | PostgreSQL 并发/篡改。 |
| `SG-VIS-009` | AssetVersion 发布必须绑定精确 selected Artifact、Asset/State/Specification/Style/Lineage/view roles；发布后不可替换，修改产生新 Version。 | Version/Hash/历史引用。 |
| `SG-VIS-010` | `shot_frame` 必须冻结正式 Shot revision 与完整 ShotProductionBindingVersion；所需 Occurrence 均绑定正确身份/状态/画风且精确 AssetVersion READY，否则不得生成。 | 完整性/跨身份/最新指针负向。 |
| `SG-VIS-011` | 选定 frame 只能发布 ShotImageBindingVersion，不得修改 ShotProductionBindingVersion；单 Shot 局部重跑创建新 Run/Selection/Output Binding，旧 Run/Binding 不变。 | 真实 Workflow/Provider/Binding 旅程。 |

## 7. Kafka、Search 与 ELK

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-EVT-001` | Owner Command 在同一 GORM 事务中写业务事实、Command Receipt 和 Outbox；网络 Publisher 不得参与 Owner 事务。事务回滚时无 Event。 | PostgreSQL 提交/回滚/网络断开。 |
| `SG-EVT-002` | Kafka Envelope 至少含 event id/type/version、occurred_at、workspace/project、aggregate kind/id/revision、source receipt、trace context 和 payload hash；不得含完整剧本、Prompt、凭据或私有 URL。 | Schema/PII/secret contract。 |
| `SG-EVT-003` | Outbox Publisher 至少一次发布；Broker ACK 未知按原 Event ID 重试。Consumer 用 Inbox/Event ID 与 aggregate revision fencing 处理重复/乱序；Poison Message 进入独立 DLQ 并可按范围 Replay。 | 真实 Kafka 重复/乱序/断连/DLQ。 |
| `SG-EVT-004` | 只为 ScriptVersion 和 StoryGraphVersion 的真实 Search Consumer 建立业务 Topic；日志使用独立 Topic。二者 Schema、ACL、Retention、Consumer Group 和 DLQ 不共享；不得建立 Kafka Command Topic。 | Broker config/consumer/ACL 测试。 |
| `SG-EVT-005` | `event-worker` 只有在真实 Kafka Consumer 落地时创建，复用 Backend Domain/Application/GORM Catalog；不得拥有独立业务 Repository 或第二数据库连接模型。 | Binary/Compose/import/DB config。 |
| `SG-SRCH-001` | Elasticsearch 至少维护 Script 与 StoryGraph 两类可重建业务索引/alias；文档包含 workspace/project、owner kind/logical/version/revision/hash、source event id 和可追溯 Evidence/Story Node key。 | Index mapping/文档/租户 contract。 |
| `SG-SRCH-002` | 旧或重复 Event 不得覆盖新 revision；删除/重建使用明确 tombstone/snapshot 规则。全量 Reindex 从 PostgreSQL Owner Snapshot 构建新版本索引并原子切换 alias。 | 真实 Elasticsearch 乱序/Reindex/alias。 |
| `SG-SRCH-003` | Backend `SearchScripts/SearchStoryGraph` 强制 Workspace/Project 授权，返回 snippet、score、Owner/Version/Evidence 深链和索引新鲜度；不得返回 Elasticsearch DSL 或允许 Search 回写 Owner。 | HTTP/权限/输入注入/反查。 |
| `SG-SRCH-004` | Elasticsearch 不可用或投影落后时返回明确 degraded/stale 状态；Owner Command 和 PostgreSQL StoryGraph Query 继续正确，不能用旧索引覆盖事实。 | 依赖故障 E2E。 |
| `SG-LOG-001` | 应用输出脱敏结构化 JSON Log，经 `Filebeat → Kafka log topic → Logstash → Elasticsearch log index → Kibana`；日志链与业务事件链隔离。 | 真实 pipeline/index/template/查询。 |
| `SG-LOG-002` | 日志至少可按 trace/run/node/task/decision/provider job/receipt ID 和稳定错误码关联；不得记录 Token、Claim Token、完整原稿/Candidate/Prompt、Provider 凭据或私有 Artifact URL。 | 端到端 trace 查询与敏感字段扫描。 |
| `SG-LOG-003` | Kafka/Logstash/Elasticsearch/Kibana 日志故障不得改变业务事务、Workflow、Owner Receipt 或 Search Projection；日志不得用于恢复业务状态。 | 逐组件故障注入。 |

## 8. Frontend

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-FE-001` | 继续使用单个 npm/Next.js 应用和现有 RTK Query；新 Lens/Review/卡片只经生成 OpenAPI Client + 唯一 RTK Query endpoint 取数，不引入第二 Query 库或 monorepo。 | package/lock/import/architecture。 |
| `SG-FE-002` | 角色卡 = Character Asset + current Specification + States + published AssetVersions + Evidence/Rights/Lineage；地点卡同理。不同 State/Style 不得创建重复身份卡；Scene View 不得伪装为 Location Card。 | View Model/component/E2E。 |
| `SG-FE-003` | Review Workbench 实现真实 Task Query、Claim/Renew/Release、Subject 比较、Decision、Resume；Backend Contract 缺失、错误或 unknown 时不得用 mock/local success。 | Component + API E2E。 |
| `SG-FE-004` | `/projects/:projectId/storygraph` 支持 outline/narrative/entity/production/impact、有界 scope、exact/current version 和稳定 focus 深链。 | Route/URL/刷新 E2E。 |
| `SG-FE-005` | `@xyflow/react` 与 `@dagrejs/dagre` 只随首个真实 Story Lens 消费者加入；首版 nodes/edges 不可写/拖动保存/连接/删除，布局与 viewport 不回传 Backend。 | 依赖/DOM/网络负向测试。 |
| `SG-FE-006` | Story Lens 与 Workflow Lens 使用不同 Query key、DTO、Node ID、Adapter 和 renderer；只能共享无业务语义的 viewport/Inspector shell。 | Type/architecture/运行测试。 |
| `SG-FE-007` | Server facts 只在 RTK Query；Lens/scope/version 在 URL；选择/viewport 为本地状态；React Flow nodes/edges 是纯派生 View Model，不复制进 Redux slice/Context/localStorage。 | State architecture/unit。 |
| `SG-FE-008` | 所有主要状态必须区分 loading/empty/401/403/404/409 stale/422/429/dependency unavailable/outcome unknown/truncated；提供键盘选择、焦点、可读列表替代和 reduced-motion。 | Component/a11y/E2E。 |
| `SG-FE-009` | 只读 Lens 通过后，类型化 Domain Intent 必须携带 base StoryGraph Version/Hash、目标 Owner、expected owner revision 和 idempotency key；Backend Owner Command 成功并重编译后才能显示完成。不得发送通用 Graph JSON/Patch 覆盖。 | HTTP/冲突/无直写 E2E。 |

## 9. 安全、可靠性与 CI

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-OPS-001` | 所有 Input/Candidate/Event/Provider Response/HTTP JSON 严格解码、限制大小与枚举；未知字段/类型、超深嵌套、非有限数字和无效 UUID/Hash fail closed。 | Fuzz/contract/HTTP 负向。 |
| `SG-OPS-002` | Runware 输出 URL/对象引用必须经过 scheme/host/path/size/content-type allowlist 与 SSRF 防护；密钥只从服务端 Credential Ref 解析，永不进入数据库明文、Browser、Agent 或日志。 | SSRF/secret/bundle/log 扫描。 |
| `SG-OPS-003` | `healthz` 只表示进程存活；`readyz` 检查该 Binary 当前已实现的必要依赖。event-worker 落地后必须真实检查 Kafka/Elasticsearch；依赖缺失不得报告 ready。 | Compose/容器/故障 readiness。 |
| `SG-OPS-004` | 测试只能位于 `backend/tests`、`agent/tests`、`frontend/tests`；业务源码目录不得混入 `*_test.go`、`test_*.py`、`*.test.*`、`*.spec.*`。 | 仓库结构检查。 |
| `SG-OPS-005` | 每个 `SG-Ixx` 必须先有失败测试，再实现并重构；完成前运行该任务局部门禁和当时全部真实 CI，失败/跳过/缺外部依赖不得标记通过。 | Commit/Acceptance/CI 记录。 |
| `SG-OPS-006` | CI 必须使用空 PostgreSQL、真实 Temporal/MinIO；Kafka/Search/ELK 能力落地时同一任务加入真实 Kafka、Elasticsearch 和日志 pipeline 检查，不用内存替身抵扣集成验收。 | GitHub Actions/本地等价命令。 |
| `SG-OPS-007` | CI 必须包含 Go format/vet/test、Agent lint/format/type/test、Frontend npm ci/lint/type/test/build、OpenAPI drift、Compose render、镜像 Binary/runtime、依赖/Secret/Data/Generated Artifact hygiene；required job 必须真实聚合全部结果。 | Workflow 静态和真实执行。 |
| `SG-OPS-008` | 每个完整任务验证通过后仅提交该任务文件，标题/正文符合仓库格式；不得通过 compatibility fallback、禁用检查、降低断言或保留旧入口使 CI 变绿。 | Git diff/commit/负向检索。 |
| `SG-OPS-009` | 所有代码、完整原稿、故障恢复和全量真实 CI 在 `SG-I27` 完成并提交前不得运行最终 `agent-browser`；`SG-I28` 才执行浏览器 → API → Owner/Temporal/Kafka/Search/Artifact 对账。 | Acceptance 顺序与 Git 历史。 |

## 10. 端到端契约

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-JRN-001` | 至少两集真实剧本完成 DocumentRevision → Evidence → confirmed/materialized Bible → Episode/Scene/Beat/Occurrence/Claim → Core StoryGraphVersion；每个 Node/Edge 可反查 Owner/Evidence。 | 真实 PostgreSQL + Temporal + 本地 Codex E2E。 |
| `SG-JRN-002` | 同一角色跨两集/两个 State 仍只有一个 Asset；完成 reference sheet → Selection → AssetVersion → detail_shots → Shot/ShotProductionBindingVersion → 新 StoryGraphVersion → shot frame/ShotImageBindingVersion。 | PostgreSQL/MinIO/Runware Staging/Human Gate E2E。 |
| `SG-JRN-003` | Script/StoryGraph Outbox 经 Kafka 到 Elasticsearch 后可由 Backend Search 深链回相同 Owner/Version；重复/乱序、Broker/Elastic 重启和全量 Reindex 后结果收敛。 | 真实 Kafka/Elasticsearch 故障 E2E。 |
| `SG-JRN-004` | 完整原稿运行机器统计和代表集人工细查；服务重启、Task lease、Signal UNKNOWN、Provider UNKNOWN、重复 Kafka、索引重建和单 Shot 局部重跑不产生重复正式事实/费用/配额。 | 全量故障矩阵与事实计数/Hash/金额报告。 |
| `SG-JRN-005` | 最终 Web Journey 覆盖项目 → 原稿 → Bible/Episode/Scene → 角色/地点卡 → Review → Reference → Shot/Binding → Story Lens/Search/Run 反查；浏览器显示与 API/PostgreSQL/Temporal/Artifact lineage 一致。 | `SG-I28` 的 `agent-browser` + 数据对账。 |

## 11. `SG-Ixx` 覆盖映射

| 实施段 | 主要 Requirement |
|---|---|
| `SG-I01` | `SG-ARC-*`、`SG-GRF-001`–`007`、`SG-OPS-004`–`008` |
| `SG-I02`、`SG-I05`、`SG-I08`–`010`、`SG-I13`、`SG-I15`、`SG-I18`、`SG-I22` | 跨服务输入/输出由 `SG-PRD-*` 覆盖；Agent 内部契约见 `SG-D19` |
| `SG-I03`–`004`、`SG-I17` | `SG-GRF-*`、`SG-QRY-*`、`SG-JRN-001` |
| `SG-I06`–`007` | `SG-REV-*`、`SG-FE-003`、`SG-OPS-*` |
| `SG-I11`–`012`、`SG-I14`、`SG-I16`、`SG-I19`、`SG-I23` | `SG-PRD-*`、`SG-REV-006`–`008` |
| `SG-I20`–`021`、`SG-I24` | `SG-VIS-*`、`SG-JRN-002` |
| Event/Search/ELK 随首个真实消费者插入其所属 `SG-Ixx`，不得新增第二队列 | `SG-EVT-*`、`SG-SRCH-*`、`SG-LOG-*`、`SG-JRN-003` |
| `SG-I25`–`026` | `SG-FE-001`–`009`、`SG-QRY-*` |
| `SG-I27`–`028` | `SG-OPS-*`、`SG-JRN-004`–`005` |

## 12. 文档门禁

本文完成 `SG-D18`。下一步 `SG-D19` 只补 StoryGraph Harness/Skill/Stage/Shard/Candidate/Codex 专项 Requirement，不复制本文的产品、Owner、Kafka、Frontend 或视觉资产范围；`SG-D20` 必须原样引用 `SG-I01`–`SG-I28` 并为 Event/Search/ELK 安排到已有任务的真实消费者切片，不创建第二实施队列；`SG-D21` 按本文件所有 ID 建立初始全未勾选 Acceptance。

在 `SG-D21` 接受前不得编码。
