# StoryGraph 内容图与 DAG 创作画布需求规格

> 状态：跨服务 Requirement 已重新接受（`SG-D18`，2026-08-29）；新增/改写的媒体 Provider 条款初始待实施、待验收，既有证据只按原合同保留
>
> 产品依据：[StoryGraph 产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md)
>
> 设计依据：[StoryGraph 总设计](../design/0010-StoryGraph内容图与DAG创作画布设计.md) · [通用媒体 Provider](../design/2051-通用媒体Provider与Generation执行器设计.md) · [Backend 领域设计](../design/2002-后端领域模块功能设计.md) · [公共 Human Gate](../design/2055-Workflow公共HumanGate命令与恢复设计.md) · [前端应用架构](../design/1001-前端应用架构.md)
>
> Agent 专项契约：由 `SG-D19` 单独复核，不在本文复制 Harness 内部协议；媒体 Provider Secret、Adapter 和业务写入职责不得移入 Agent

## 1. 规格约定

本文是 StoryGraph MVP 的跨 Backend、Workflow、Review、Asset/Generation/Media Provider、Kafka/Search/ELK 和 Frontend 唯一可测契约。每条“必须/不得”都必须由 `SG-D21` Acceptance 映射：2026-08-27 后已有真实证据的未变合同可保留证据；本次新增或改写合同必须创建全未勾选的新目标项，历史 Runware/环境变量证据不能自动满足。

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
| `SG-ARC-001` | Backend Go Module 是 Production、StoryGraph、Workflow 业务投影、Review、Generation、Asset、Cost/Quota、Outbox 和 Search 索引协调的唯一业务 Writer。 | `architecture`：禁止 Frontend/Agent/Event Runtime 旁路 Owner Application 直接写正式事实；跨边界负向调用。 |
| `SG-ARC-002` | PostgreSQL 是唯一 SQL 事实源；全部业务表只由一份 GORM Model Catalog 定义并同步。不得存在 Migration 文件/版本/Checksum/Source 元数据、Raw SQL 业务 Schema、第二 ORM、第二数据库 Writer 或兼容双写。 | `architecture + integration`：目录扫描、依赖扫描、空库 Catalog Sync、Model/约束核查。 |
| `SG-ARC-003` | Domain/Application 不得导入 GORM、Temporal、Kafka、Elasticsearch、对象存储或 Provider SDK；Adapter 实现 Port，Composition Root 负责唯一装配。 | Go import/AST 架构测试。 |
| `SG-ARC-004` | Temporal 是唯一跨步骤持久 Workflow/Timer/Human Wait/Signal 引擎；Kafka 不调度 Workflow，数据库轮询不建立第二工作流。 | 架构扫描；真实 Temporal wait/signal/replay/restart。 |
| `SG-ARC-005` | StoryGraph、Authoring Graph、WorkflowDefinition 和 Temporal History 使用不同 Schema、ID、Owner 和存储，不能经万能 Graph DTO/表互相写入。 | `contract + architecture`：ID/DTO 不可互换负向测试。 |
| `SG-ARC-006` | Python Agent 是 Backend 私有 Candidate Runtime，只返回严格 Candidate/Repair；不得接收数据库、Kafka、Elasticsearch、Temporal、对象存储、JWT 或 Provider 业务凭据，也不得写业务 API。 | Agent 环境/依赖/网络/Tool allowlist 与 Backend 权限测试；细则见 `SG-D19`。 |
| `SG-ARC-007` | Browser 只能调用 Backend `/api/v1`；不得直连 PostgreSQL、Temporal、Kafka、Elasticsearch、ELK、Agent、对象存储私有地址或任何媒体 Provider。 | Frontend bundle/import/网络 E2E。 |
| `SG-ARC-008` | 每个跨外部边界操作必须使用稳定 Intent/Request/Job/Call/Receipt ID，区分 known failure 与 outcome unknown；UNKNOWN 只能按 Provider 真实能力查询原身份或等待人工处理，不能盲目重提。 | 故障注入矩阵。 |
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
| `SG-REV-006` | 正向 Decision 必须先调用显式 Owner Applier 并冻结真实 Owner Receipt/canonical Gate Output/Hash，再准备 Workflow Signal；拒绝/修改为 `owner_apply_status=not_required`，不得产生正向 Owner 写入。 | 八类 Gate owner/zero-side-effect 测试，包含 Shot Video。 |
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
| `SG-PRD-011` | `ProductionBinding`、`ShotProductionBindingVersion`、`ShotImageBindingVersion`、`ShotVideoBindingVersion` 必须为不同 Owner/Record/语义，任何 API/DTO/Graph Node 不得混用。 | Schema/应用/Graph contract 负向测试。 |
| `SG-PRD-012` | 每次相关 Owner Apply 成功后，StoryGraph Compiler 必须用 expected current graph hash 发布下一 Version；Owner 成功但编译未知时按相同 Receipt/Hash 对账，不创建第二 Version。 | 崩溃窗口与重放。 |

## 6. Asset、Generation 与通用媒体 Provider

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-VIS-001` | GenerationTarget 只允许严格 `reference_asset|shot_frame|shot_video` union；字段、Owner Ref、Target Hash 和媒体 modality 按 kind 校验，不得通过通用 Payload 或兼容入口绕过。 | JSONB/Domain/HTTP/StoryGraph contract。 |
| `SG-VIS-002` | `reference_asset` 必须冻结 Asset identity、SpecificationVersion、AssetState、EffectiveStyleSnapshot、目标 view roles 和精确 Project Provider Binding；只能消费 `approved_storyboard_intents`，Intent Gate 未完成时零费用、零 Call。 | Target Hash/顺序/前置 Gate/事实计数。 |
| `SG-VIS-003` | Backend 必须提供只包含已注册真实 Factory 的内置 Provider/Model Preset Catalog；Owner 可按受控字段创建不可变 Connection/Credential/ModelProfile Version。未知 Preset、缺 Factory、任意 URL/Header/JSON 或篡改字段均失败关闭。 | Catalog/Registry 一致性、HTTP strict decode、重启与负向 contract。 |
| `SG-VIS-004` | Provider 远程调用前必须已有冻结 Generation Intent、PriceQuote/Estimate、Cost/Quota Reservation、Execution Claim/Authorization、Project Binding 与已提交 ProviderJob/ProviderCall；任一前置失败不调用 Provider。 | 真实 PostgreSQL + 受控 Gateway/真实凭据调用计数。 |
| `SG-VIS-005` | Provider 图片/视频输出必须进入私有 Staging；Backend 从真实字节重验 SHA-256、大小、媒体类型，图片解码/尺寸，视频 container/codec/尺寸/时长后进入 READY/QUARANTINED。Provider URL 不是 Artifact，不能进入正式 Binding。 | MinIO、图片解码、FFprobe、损坏/SSRF/redirect/元数据漂移。 |
| `SG-VIS-006` | composite reference sheet 固定一个 Artifact 覆盖 front/profile/back 语义区域；缺失/重复 role、输入 Hash 不同、非 READY 或 lineage 不完整时不得发布 AssetVersion。 | QC/coverage/selection 负向矩阵。 |
| `SG-VIS-007` | Image/Video 确定性 QC 只判定版本化 Policy 能证明的格式、尺寸、coverage、容器、codec、时长容差等规则；身份、服装、画风和动作语义仍须 Human CandidateSelection，不得伪装成自动通过。 | QC Report/Policy Snapshot/Review fixture + manual 补充。 |
| `SG-VIS-008` | CandidateSelection 只从 HumanTask 冻结且 READY/QC 合法的 CandidateSet 产生；单 Task 恰有一个不可变 Selection，重复/并发收敛。Reference、Shot Frame、Shot Video 使用不同 Subject/Owner Apply。 | PostgreSQL 并发/篡改/八类 Gate 路由。 |
| `SG-VIS-009` | AssetVersion 发布必须绑定精确 selected Artifact、Asset/State/Specification/Style/Lineage/view roles；发布后不可替换，修改产生新 Version。 | Version/Hash/历史引用。 |
| `SG-VIS-010` | `shot_frame` 必须冻结正式 Shot revision 与完整 ShotProductionBindingVersion；所需 Occurrence 均绑定正确身份/状态/画风且精确 AssetVersion READY，否则不得生成。 | 完整性/跨身份/最新指针负向。 |
| `SG-VIS-011` | 选定 frame 只能发布 ShotImageBindingVersion，不得修改 ShotProductionBindingVersion；单 Shot 局部重跑创建新 RunInputSnapshot/Target/Selection/Output Binding，旧 Run/Binding 不变。 | 三类图片 Provider 的真实 Workflow/Binding 旅程。 |
| `SG-VIS-012` | Workspace Owner 才能创建/轮换/禁用 Provider Connection/Credential/Profile 并发布 Project Purpose Binding；所有 Command 带幂等键与 expected revision/hash，轮换只追加新版本，既有 Job 继续引用冻结版本。Editor/Viewer 与跨租户写入失败。 | OpenAPI、权限、并发、幂等、Revision/Receipt、重启恢复。 |
| `SG-VIS-013` | 必须有精确且不可变的 Seedream 5.0 Pro+、Seedance 2.0/2.0 Fast/2.0 Mini/2.5、GPT Image 2 与 Nano Banana 2 Lite/2/Pro/Legacy ModelProfile；每个 Profile 冻结官方 Model/Endpoint ID、协议、Capability、PriceQuote 和允许参数，不用 `>=` 通配或运行时试探。 | Catalog/Profile golden、官方合同 fixture、Capability/PriceQuote 负向矩阵。 |
| `SG-VIS-014` | Provider API Key 只经 Web TLS Request 一次写入，使用 `/run/secrets/lanverse_media_provider_master_key` root key 加密为独立 CredentialVersion；不得进入 `.env`、Query/Response、普通业务列、日志、Temporal、Kafka、StoryGraph、Candidate Payload、URL/cache/storage。零配置或 root key 缺失不阻止非视觉服务启动，但配置/执行必须明确阻塞。 | Secret/Data/bundle/log/DB 扫描、错误 root key、容器重启与零配置 E2E。 |
| `SG-VIS-015` | 一个候选恰好对应一个 ProviderCall；四候选四 Call。每个 Call 只有首次 `PENDING → DISPATCHING` 路径可发送一次真实请求，并最多收敛一个终态输出；并发/Activity 重投/崩溃不得隐藏循环或第二次付费。 | 四个崩溃窗口、并发、发送计数、终态/Cost 聚合故障矩阵。 |
| `SG-VIS-016` | 同步 Provider 在 dispatch boundary 后连接中断且无官方查询身份时必须进入 `outcome_unknown`，保留 Cost/Quota Reservation 并禁止自动重提；known failed/succeeded 使用独立终态，迟到结果不得覆盖已提交终态。 | Seedream/GPT Image/Nano Banana 故障代理与真实协议边界。 |
| `SG-VIS-017` | 异步 Provider 必须持久化官方 remote task id，Temporal 以同一身份查询到终态；Worker/Backend 重启、Timer/Activity 重放只恢复原任务。外部保留窗口到期前未确认结果必须显式阻塞，不能创建第二任务。 | Seedance 创建/查询/重启/超时/保留窗口测试。 |
| `SG-VIS-018` | Seedream 5.0 Pro+ 必须使用火山方舟精确图片协议，每 Call 只请求一个独立 Candidate；零/多顶层输出、协议漂移、能力不匹配或 Staging 校验失败不得物化成功 Receipt。 | 离线合同 + 真实凭据 reference/shot_frame 旅程。 |
| `SG-VIS-019` | GPT Image 2 必须使用精确 OpenAI Image API Profile、`n=1` 和一个 `data[].b64_json`；严格 Base64/Staging/Usage/PriceQuote 校验，禁止回退其他 OpenAI 模型或接口。 | 离线合同 + 真实凭据 reference/shot_frame 旅程。 |
| `SG-VIS-020` | Nano Banana 2 Lite/Legacy 必须走各自 Generate Content 合同，2/Pro 必须走各自 Interactions 合同；每个 Call 只接受一个图片输出并校验精确模型/usage，四模型不得运行时试探或互相回退。 | 四组离线合同 + 四个真实 Profile reference/shot_frame 旅程。 |
| `SG-VIS-021` | `shot_video` 必须冻结正式 Shot、ShotProductionBindingVersion、ShotImageBindingVersion、精确输入 Artifact、目标时长/比例、motion prompt hash 和 Seedance Profile；不支持的组合在 Cost 前返回 capability mismatch，不静默取整、裁切、换模型或开音频。 | Target/Capability/Cost 零副作用与跨 Shot/漂移负向测试。 |
| `SG-VIS-022` | 选定视频必须通过 Video QC，并由 Storyboard Owner 追加 ShotVideoBindingVersion，冻结 Target/Selection/Candidate/Artifact/首帧/时长/媒体元数据/Hash。StoryGraph 只投影 Target、Artifact 与最终 Binding，不投影 Provider 配置/Secret/Job/Call。 | GORM/Owner Apply/StoryGraph 类型矩阵、局部重跑与真实 Seedance E2E。 |

## 7. Kafka、Search 与 ELK

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-EVT-001` | Owner Command 在同一 GORM 事务中写业务事实、Command Receipt 和 Outbox；网络 Publisher 不得参与 Owner 事务。事务回滚时无 Event。 | PostgreSQL 提交/回滚/网络断开。 |
| `SG-EVT-002` | Kafka Envelope 至少含 event id/type/version、occurred_at、workspace/project、aggregate kind/id/revision、source receipt、trace context 和 payload hash；不得含完整剧本、Prompt、凭据或私有 URL。 | Schema/PII/secret contract。 |
| `SG-EVT-003` | Outbox Publisher 至少一次发布；Broker ACK 未知按原 Event ID 重试。Consumer 用 Inbox/Event ID 与 aggregate revision fencing 处理重复/乱序；Poison Message 进入独立 DLQ 并可按范围 Replay。 | 真实 Kafka 重复/乱序/断连/DLQ。 |
| `SG-EVT-004` | 只为 ScriptVersion 和 StoryGraphVersion 的真实 Search Consumer 建立业务 Topic；日志不得创建或借用 Kafka Topic；不得建立 Kafka Command Topic。 | Broker config/consumer/ACL 测试。 |
| `SG-EVT-005` | Backend Event Runtime 只有在真实 Kafka Consumer 落地时才由单 `lanverse` Binary 装配，复用 Backend Domain/Application/GORM Catalog；不得创建独立 Worker Binary/Compose 服务、独立业务 Repository 或第二数据库连接模型。 | Binary/Compose/import/DB config。 |
| `SG-SRCH-001` | Elasticsearch 至少维护 Script 与 StoryGraph 两类可重建业务索引/alias；文档包含 workspace/project、owner kind/logical/version/revision/hash、source event id 和可追溯 Evidence/Story Node key。 | Index mapping/文档/租户 contract。 |
| `SG-SRCH-002` | 旧或重复 Event 不得覆盖新 revision；删除/重建使用明确 tombstone/snapshot 规则。全量 Reindex 从 PostgreSQL Owner Snapshot 构建新版本索引并原子切换 alias。 | 真实 Elasticsearch 乱序/Reindex/alias。 |
| `SG-SRCH-003` | Backend `SearchScripts/SearchStoryGraph` 强制 Workspace/Project 授权，返回 snippet、score、Owner/Version/Evidence 深链和索引新鲜度；不得返回 Elasticsearch DSL 或允许 Search 回写 Owner。 | HTTP/权限/输入注入/反查。 |
| `SG-SRCH-004` | Elasticsearch 不可用或投影落后时返回明确 degraded/stale 状态；Owner Command 和 PostgreSQL StoryGraph Query 继续正确，不能用旧索引覆盖事实。 | 依赖故障 E2E。 |
| `SG-LOG-001` | 应用输出脱敏结构化 JSON Log，同时保留 stdout 并经失败开放 TCP Writer 进入 `Logstash → Elasticsearch log index → Kibana`；日志不经过 Kafka。 | 真实 pipeline/index/template/查询。 |
| `SG-LOG-002` | 日志至少可按 trace/run/node/task/decision/provider job/call/receipt ID 和稳定错误码关联；不得记录 Token、Claim Token、完整原稿/Candidate/Prompt、Provider 凭据或私有 Artifact URL。 | 端到端 trace 查询与敏感字段扫描。 |
| `SG-LOG-003` | Logstash/Elasticsearch/Kibana 日志故障不得改变业务事务、Workflow、Owner Receipt 或 Search Projection；日志不得用于恢复业务状态。 | 逐组件故障注入。 |

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
| `SG-FE-010` | Provider Settings 必须从 Catalog 渲染受控表单，支持 Connection/Credential 轮换、ModelProfile、PriceQuote 和 Project Binding；API Key 提交成功立即清空且不进入 URL、RTK Query cache、localStorage、日志或回显。任意 Host/JSON/直接生成没有入口。 | Component/网络/浏览器存储/bundle/刷新/权限 E2E。 |
| `SG-FE-011` | Media/Review 页面必须分别展示 Reference、Shot Frame、Shot Video 的 Target、Candidate、Image/Video QC、Cost、Selection 和 Owner Apply；zero config、root key missing、binding stale、provider unknown、apply pending 是可区分状态。 | Component + 真实 Backend E2E。 |

## 9. 安全、可靠性与 CI

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-OPS-001` | 所有 Input/Candidate/Event/Provider Response/HTTP JSON 严格解码、限制大小与枚举；未知字段/类型、超深嵌套、非有限数字和无效 UUID/Hash fail closed。 | Fuzz/contract/HTTP 负向。 |
| `SG-OPS-002` | Provider Host/Region、输出 URL/redirect、对象引用、size/content-type 必须由编译 Descriptor allowlist 与 SSRF 防护限制；用户不得配置任意 Host。Secret 只从精确 CredentialVersion 临时解密并注入当前 Authorization Header，随后清理。 | SSRF/redirect/secret/bundle/log/内存生命周期扫描。 |
| `SG-OPS-003` | `healthz` 只表示进程存活；公共 `readyz` 检查 API 必需依赖，单 Backend 进程内的 Event Runtime 另以内部 readiness 真实检查 Kafka/Elasticsearch；依赖缺失不得报告 Event Runtime ready。 | Compose/容器/故障 readiness。 |
| `SG-OPS-004` | 测试只能位于 `backend/tests`、`agent/tests`、`frontend/tests`；业务源码目录不得混入 `*_test.go`、`test_*.py`、`*.test.*`、`*.spec.*`。 | 仓库结构检查。 |
| `SG-OPS-005` | 每个 `SG-Ixx` 必须先有失败测试，再实现并重构；完成前运行该任务局部门禁和当时全部真实 CI，失败/跳过/缺外部依赖不得标记通过。 | Commit/Acceptance/CI 记录。 |
| `SG-OPS-006` | CI 必须使用空 PostgreSQL、真实 Temporal/MinIO；Kafka/Search/ELK 能力落地时同一任务加入真实 Kafka、Elasticsearch 和日志 pipeline 检查，不用内存替身抵扣集成验收。 | GitHub Actions/本地等价命令。 |
| `SG-OPS-007` | CI 必须包含 Go format/vet/test、Agent lint/format/type/test、Frontend npm ci/lint/type/test/build、OpenAPI drift、Compose render、镜像 Binary/runtime、依赖/Secret/Data/Generated Artifact hygiene；required job 必须真实聚合全部结果。 | Workflow 静态和真实执行。 |
| `SG-OPS-008` | 每个完整任务验证通过后仅提交该任务文件，标题/正文符合仓库格式；不得通过 compatibility fallback、禁用检查、降低断言或保留旧入口使 CI 变绿。 | Git diff/commit/负向检索。 |
| `SG-OPS-009` | 所有代码、四类 Provider 真实调用、完整原稿、故障恢复和全量真实 CI 在 `SG-I34` 完成并提交前不得运行最终 `agent-browser`；`SG-I35` 才执行浏览器 → API → Owner/Temporal/Kafka/Search/图片视频 Artifact 对账。 | Acceptance 顺序与 Git 历史。 |
| `SG-OPS-010` | Backend Runtime Image 必须安装并验证固定发行版 `ffprobe`；`docker-compose.yml` 仍只启动 Backend/Frontend 并挂载 Provider root-key Docker Secret，本地复用宿主机已启动依赖。Provider API Key 不得出现在 Compose/.env。 | 镜像命令、Compose render/services、Secret/Data hygiene 与本机启动。 |

## 10. 端到端契约

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SG-JRN-001` | 至少两集真实剧本完成 DocumentRevision → Evidence → confirmed/materialized Bible → Episode/Scene/Beat/Occurrence/Claim → Core StoryGraphVersion；每个 Node/Edge 可反查 Owner/Evidence。 | 真实 PostgreSQL + Temporal + 本地 Codex E2E。 |
| `SG-JRN-002` | Owner 在 Web 创建火山/OpenAI/Google 持久连接与精确 Project Binding；同一角色跨两集/两个 State 仍只有一个 Asset；Seedream、GPT Image、Nano Banana 四模型分别完成 reference sheet/shot frame → Selection → AssetVersion/ShotImageBindingVersion；Seedance 四模型分别完成 shot video → Video QC/Selection → ShotVideoBindingVersion，并编译可反查的新 StoryGraphVersion。 | PostgreSQL/MinIO/三类图片 Provider/Seedance/Human Gate/Frontend 真实 E2E。 |
| `SG-JRN-003` | Script/StoryGraph Outbox 经 Kafka 到 Elasticsearch 后可由 Backend Search 深链回相同 Owner/Version；重复/乱序、Broker/Elastic 重启和全量 Reindex 后结果收敛。 | 真实 Kafka/Elasticsearch 故障 E2E。 |
| `SG-JRN-004` | 完整原稿运行机器统计和代表集人工细查；服务重启、Task lease、Signal UNKNOWN、Provider UNKNOWN、重复 Kafka、索引重建和单 Shot 局部重跑不产生重复正式事实/费用/配额。 | 全量故障矩阵与事实计数/Hash/金额报告。 |
| `SG-JRN-005` | 最终 Web Journey 覆盖 Provider Preset/Connection/Profile/Binding → 项目 → 原稿 → Bible/Episode/Scene → 角色/地点卡 → Reference/Shot Frame/Shot Video Review → 图片/视频 Binding → Story Lens/Search/Run 反查；浏览器显示与 API/PostgreSQL/Temporal/ProviderCall/Artifact lineage 一致。 | `SG-I35` 的 `agent-browser` + 数据对账。 |

## 11. `SG-Ixx` 覆盖映射

| 实施段 | 主要 Requirement |
|---|---|
| `SG-I01` | `SG-ARC-*`、`SG-GRF-001`–`007`、`SG-OPS-004`–`008` |
| `SG-I02`、`SG-I05`、`SG-I08`–`010`、`SG-I13`、`SG-I15`、`SG-I18` | 跨服务输入/输出由 `SG-PRD-*` 覆盖；Agent 内部契约见 `SG-D19` |
| `SG-I03`–`004`、`SG-I17` | `SG-GRF-*`、`SG-QRY-*`、`SG-JRN-001` |
| `SG-I06`–`007` | `SG-REV-*`、`SG-FE-003`、`SG-OPS-*` |
| `SG-I11`–`012`、`SG-I14`、`SG-I16`、`SG-I19` | `SG-PRD-*`、`SG-REV-006`–`008` |
| `SG-I20` | `SG-VIS-003`、`012`–`014`、`SG-ARC-*`、`SG-OPS-001`–`004`、`010` |
| `SG-I21` | `SG-VIS-004`、`005`、`015`–`017`、`SG-ARC-008`、Cost/Quota 既有合同 |
| `SG-I22` | `SG-FE-010`、`SG-VIS-003`、`012`–`014`、`SG-OPS-002` |
| `SG-I23` | `SG-VIS-001`–`009`、`013`–`018`、`SG-JRN-002` 的 Seedream 部分 |
| `SG-I24` | `SG-VIS-001`–`009`、`013`–`016`、`019`、`SG-JRN-002` 的 GPT Image 部分 |
| `SG-I25` | `SG-VIS-001`–`009`、`013`–`016`、`020`、`SG-JRN-002` 的 Nano Banana 部分 |
| `SG-I26` | `SG-VIS-006`–`009`、`SG-REV-*`、AssetVersion Owner Apply |
| `SG-I27` | `SG-PRD-009`、精确 READY AssetVersion 与 Detail Repair |
| `SG-I28` | `SG-PRD-010`–`012`、`SG-REV-*`、StoryGraph 重编译 |
| `SG-I29` | `SG-VIS-010`–`011`、`018`–`020`、Shot Frame Human Gate/Binding |
| `SG-I30` | `SG-VIS-001`、`005`、`007`–`008`、`021`–`022`、`SG-PRD-011`、`SG-REV-*` |
| `SG-I31` | `SG-VIS-013`、`017`、`021`–`022`、`SG-JRN-002` 的 Seedance 部分 |
| Event/Search/ELK 随首个真实消费者插入其所属 `SG-Ixx`，不得新增第二队列 | `SG-EVT-*`、`SG-SRCH-*`、`SG-LOG-*`、`SG-JRN-003` |
| `SG-I32` | `SG-FE-001`–`008`、`011`、`SG-QRY-*` |
| `SG-I33` | `SG-FE-009`、Owner Command/重编译契约 |
| `SG-I34` | `SG-OPS-*`、`SG-JRN-001`–`004`、四类 Provider 真实验收 |
| `SG-I35` | `SG-OPS-009`、`SG-JRN-005` |

## 12. 文档门禁

本文完成 `SG-D18` 重新同步。下一步 `SG-D19` 只复核 StoryGraph Harness/Skill/Stage/Shard/Candidate/Codex 专项 Requirement，确认 Agent 不获得媒体 Provider Secret、Adapter 或业务写入职责；`SG-D20` 必须原样引用 `SG-I01`–`SG-I35` 并保持 Event/Search/ELK 已完成事实与后续真实消费者边界，不创建第二实施队列；`SG-D21` 为本次新增/改写 ID 建立全未勾选目标 Acceptance，保留但不冒充新合同的历史 Evidence。

在 `SG-D21` 接受前不得编码。
