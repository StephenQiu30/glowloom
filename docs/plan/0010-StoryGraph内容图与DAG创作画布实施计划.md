# StoryGraph 内容图与 DAG 创作画布实施计划

- 状态：实施中；`SG-I01`–`SG-I09` 已完成（2026-08-27）
- Design：[0010 StoryGraph 内容图与 DAG 创作画布设计](../design/0010-StoryGraph内容图与DAG创作画布设计.md)
- Agent Design：[3003 StoryGraph 剧本解析 Harness 与内置 Skill 设计](../design/3003-StoryGraph剧本解析Harness与内置Skill设计.md)
- PRD：[0010 StoryGraph 内容图与 DAG 创作画布产品需求](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md)
- Cross-service Requirement：[0010 StoryGraph 内容图与 DAG 创作画布需求规格](../requirement/0010-StoryGraph内容图与DAG创作画布需求规格.md)
- Agent Requirement：[3003 StoryGraph 剧本解析 Harness 与内置 Skill 需求规格](../requirement/3003-StoryGraph剧本解析Harness与内置Skill需求规格.md)
- Acceptance：[0010 StoryGraph 内容图与 DAG 创作画布验收标准](../acceptance/0010-StoryGraph内容图与DAG创作画布验收标准.md)

## 1. 计划边界

本文是当前 StoryGraph 目标唯一实施计划，只安排已接受设计中的 `SG-I01`–`SG-I28`，不从 `0007/0008/1001/2001/2002/3001/3002` 旧计划领取重叠任务，也不为 Kafka、Search、ELK、Agent 或 Canvas 建立第二队列。

计划状态不代表功能事实。编码前先建立全未勾选 Acceptance；实施时任何时刻只推进一个 `SG-Ixx`。一个 `SG-Ixx` 内允许把相互独立且可完整验证的 Red→Green 交付单元分别提交，但不得把半成品、只红不绿的测试或多个尚未完成任务堆入一次提交；该项所有门通过并提交后才解锁下一项。

MVP 非目标保持不变：不建微服务、第二 Workflow、Kafka Command Topic、Event Sourcing、Graph 数据库、向量数据库、协作 Canvas、Yjs/Hocuspocus、通用规则引擎、Migration 目录/账本、Raw SQL 业务 Repository、第二 ORM、Agent 业务 Writer或兼容回退层。

## 2. 开工事实与固定工具链

### 2.1 当前事实

- Backend 是单 Go Module，已有 GORM、PostgreSQL、Temporal、MinIO、公共 OpenAPI、`api`、`workflow-worker` 与 `event-worker`；业务 Model 由唯一 GORM Catalog 同步。`SG-I04` 已完成 Kafka 业务事件、Script/StoryGraph Elasticsearch 可重建检索投影和独立 `Filebeat → Kafka → Logstash → Elasticsearch → Kibana` 日志链；PostgreSQL 仍是唯一业务事实源。
- Agent 是私有 FastAPI Candidate Runtime，`SG-I05` 已将 8 个过渡 Skill 原子收口为唯一 `agent/skills/build-storygraph` Bundle、十个显式 Stage Reference 与严格 Pydantic Candidate；Agent 不拥有 ORM、业务 Writer、Kafka、Elasticsearch 或 Temporal Client。
- Frontend 是单 Next.js/npm 应用，使用 RTK Query；尚未安装 React Flow 或 Dagre。
- 当前 Compose 与 CI 已有真实 PostgreSQL、Temporal、MinIO、Kafka KRaft、Elasticsearch 以及独立 ELK 日志管道，并执行 Broker/Search/日志组件停机与恢复门禁。
- 2026-08-27 检查到最近一次已推送 GitHub `CI` 为成功；本地后续每个实施提交仍须重新运行当前完整 CI，不能用历史结果抵扣。

### 2.2 选型、版本与引入时点

| 能力 | 固定实现 | 首次引入 | 边界 |
|---|---|---|---|
| SQL 事实源 | PostgreSQL 16 + 已有 GORM `v1.31.2`/postgres driver `v1.6.2` | `SG-I01` 复核，`SG-I03` 首个 StoryGraph Record | 只有 `adapter/gormdb` 与 `platform/database` 导入 GORM；只扩展唯一 Model Catalog，不建立 Migration 文件/表、Raw SQL 或第二连接模型 |
| Durable Workflow | 已有 Temporal Go SDK `v1.44.1` | 随各 Workflow 切片复用 | 只由 Workflow Adapter/Worker 使用；Kafka 不调度、恢复或替代 Workflow |
| Kafka Broker | 官方 `apache/kafka:4.3.1`，本地/CI 单节点 KRaft | `SG-I04` 首个真实 Script/StoryGraph Search Consumer | 业务 Event、日志 Topic 分离；无 Command Topic；生产扩容不在 MVP |
| Kafka Go Client | `github.com/twmb/franz-go v1.21.6`，直接使用 `pkg/kgo` | `SG-I04` | 仅 Eventing Kafka Adapter 与 `event-worker` Composition Root 可导入；Domain/Application 只依赖 Port/Envelope |
| Search | Elasticsearch `9.4.4` + `github.com/elastic/go-elasticsearch/v9 v9.4.3` | `SG-I04` | 仅 Search Elasticsearch Adapter 导入客户端；索引是 PostgreSQL Owner Snapshot 的可重建投影，不是事实源 |
| ELK 日志链 | Filebeat/Logstash/Elasticsearch/Kibana 全部 `9.4.4` | `SG-I04` 随首个真实服务日志消费者 | `Filebeat → Kafka 独立日志 Topic → Logstash → 独立日志索引 → Kibana`；业务与日志 Topic、Group、DLQ、Index 不共享 |
| 结构化日志 | Go 标准库 `log/slog` JSON Handler，Python/Frontend 输出同一脱敏字段集 | `SG-I01` 固定契约，`SG-I04` 接管真实管道 | 不引入第二日志框架；日志失败不改变业务事务或 Receipt |
| Agent Schema/Runtime | 已有 Pydantic `2.13.4`、FastAPI `0.140.12`、Codex CLI `0.147.0` | `SG-I01` 固定 Wire fixture，`SG-I05` 最终 Bundle | Agent 无 ORM、Kafka、Elastic、Temporal、对象存储、JWT 或 Provider 业务凭据；未出现真实消费者时在 `SG-I05` 删除 LangGraph |
| Canvas | `@xyflow/react 12.11.5` + `@dagrejs/dagre 3.1.1` | `SG-I25` | 只在只读 Story Lens 首个组件中安装和导入；不预建通用 Canvas Framework |
| 图片 Provider | Go `net/http` + 已接受 Runware REST Contract | `SG-I20` | 不增加 SDK 包装层；同一个稳定 Provider Job ID 做 unknown 对账 |

版本只在上述首个真实消费者落地时写入对应锁文件；不得为“将来会用”提前安装。升级任何版本必须先修改 Design/Requirement 的受影响事实、重跑真实依赖 CI，不增加版本兼容分支。

版本核验使用一手来源：Apache Kafka [Supported releases 与官方镜像](https://kafka.apache.org/community/downloads/)、Elastic [全栈同版本安装要求](https://www.elastic.co/docs/get-started/the-stack)、[`franz-go` 官方仓库](https://github.com/twmb/franz-go)、[`go-elasticsearch` 官方仓库](https://github.com/elastic/go-elasticsearch)、[React Flow 官方发布](https://github.com/xyflow/xyflow/releases) 与 [Dagre 官方发布](https://github.com/dagrejs/dagre/releases)。锁定值是本轮可复现基线，不以动态 `latest` 启动 CI。

### 2.3 代码所有权与导入边界

| 目标包/入口 | 职责 | 允许依赖 | 禁止依赖 |
|---|---|---|---|
| `backend/internal/storygraph/domain` | Key、Node/Edge/Claim、Evidence、Version/Head、Hash 与图不变量 | Go 标准库和纯领域类型 | GORM、Temporal、Kafka、Elastic、HTTP、Agent、Provider |
| `backend/internal/storygraph/application` | Compiler、Publish、Query、Diff、Impact Closure 与 Port | StoryGraph Domain、明确 Owner Snapshot Port | 基础设施客户端、跨模块 Record、Raw SQL |
| `backend/internal/storygraph/adapter/gormdb` | StoryGraph Record/Head/Outbox 原子持久化 | GORM、数据库 Model、Application Port | 业务策略、Temporal/Kafka/Elastic Client |
| `backend/internal/eventing` 的 Domain/Application/Adapter | Envelope、Outbox Publisher、Inbox、DLQ/Replay 与 Kafka 传输 | Application 依赖 Port；Kafka Adapter 依赖 franz-go | Workflow 调度、Owner Command、完整剧本文本/Prompt/Secret |
| `backend/internal/search` 的 Domain/Application/Adapter | Script/StoryGraph 文档、授权查询、Projection/Reindex/Alias | Owner Snapshot Port；Elastic Adapter 依赖官方 Go Client | Owner 写入、Elastic DSL 透传、第二业务 Repository |
| `backend/cmd/event-worker` | Kafka Consumer、Search Projector、readiness 的唯一装配 | Backend 已有模块、Event/Search Adapter | 独立 Domain、独立数据库模型、第二 SQL 连接来源 |
| `backend/internal/workflow` | Temporal Definition、Activity、Signal/Resume 与持久状态 | Backend Application Port、Temporal Adapter | Kafka 等待/调度、Search/ELK 恢复状态 |
| `backend/internal/agent` | Backend-owned Stage Wire/Policy 与私有 Agent Client | 严格 Contract、HTTP Client Port | Agent 内部文件路径、ORM/Queue/Search Client |
| `agent/app/candidate_runtime`、`agent/skills/build-storygraph` | 受控 Codex Invocation、Stage/Shard/Candidate/Repair | Pydantic、标准库、Bundle 内资源 | 业务数据库/API、Kafka、Elastic、Temporal、对象存储、Provider |
| `frontend/src/features/storygraph` | Owner API View Model、Review、Story Lens 与类型化 Intent | 生成 API Client、RTK Query；`SG-I25` 后可用 React Flow/Dagre | 直连 Agent/Temporal/Kafka/Elastic/ELK、SQL/Graph JSON 写入 |

目录只在对应真实类、Port、Adapter 或页面落地时创建；禁止先建立空 `utils`、`common`、`services`、Repository 转发层或未来 Binary。

## 3. 每项统一执行门

每个 `SG-Ixx` 都按以下顺序完成：

1. 从 Acceptance 领取本项映射的未通过条款，记录当前失败事实和最小 fixture。
2. 先在 `backend/tests`、`agent/tests` 或 `frontend/tests` 写失败测试；测试不得放入业务源码目录。
3. 实现最小 Green，只修改本项 Owner 和必要 Composition Root；禁止兼容回退、假成功、跳过检查或内存替身抵扣真实依赖。
4. Refactor 后运行本项定向测试、架构/import/secret/SQL 门禁和当前完整真实 CI；涉及 Kafka、Elastic、ELK、Temporal、MinIO、PostgreSQL 或 Provider 时使用真实服务。
5. 回填本项 Acceptance Evidence；检查 diff、生成文件、secret、日志、缓存和报告产物。
6. 只提交已完整通过的本项或本项中的完整交付单元，格式为 `type(scope): 中文功能说明`；提交标题和正文只描述交付的功能，不包含 `SG-Ixx` 等任务编号或任务名。最后一个提交必须使当前实施项的全部门通过，再进入下一项。

若外部 Codex 登录、Runware Staging、真实服务资源或人工决议缺失，保留 failed/unknown/blocked 事实并停止；不得使用 mock、旧证据或静默降级宣称通过。

## 4. 唯一实施任务队列

以下顺序原样引用已接受 Design 的 `SG-I01`–`SG-I28`；Checklist 只表示实施进度，初始全部未完成。

### 基础契约与公共能力

- [x] `SG-I01`：固定 StoryGraph Schema、稳定 Key、Node/Edge/Claim Owner、Evidence Ref、Canonical Hash、四图边界和跨语言 contract fixture；在首个真实消费者中固定 GORM/PostgreSQL、Temporal、Pydantic/Codex CLI 与 React Flow/Dagre 选型。完成门：Requirement/Acceptance 映射完整，失败 contract 测试先落位，无空工具层、Migration、Raw SQL 或第二 ORM。**完成（2026-08-27）**：Go Domain 与 Go/Python Wire golden、依赖首消费者门、空库/Temporal/MinIO、全量 CI/Compose/三镜像证据见 Acceptance。
- [x] `SG-I02`：把现有 8 个 Skill 按原名、原字节迁入 `agent/skills`，原子切换 Loader/Docker/独立测试，同一提交删除根目录旧路径。完成门：Guidance 字节等价、单路径、无 fallback，Agent 和全量 CI 通过。**完成（2026-08-27）**：19 个文件的路径与 SHA-256 golden 等价，Loader/Docker/测试已原子切换，旧路径负向与三镜像真实 CI 证据见 Acceptance。
- [x] `SG-I03`：实现 StoryGraphVersion/Head、Owner Set 冻结、GORM Record、线性 Compiler/发布；首版只编译已有 Owner 事实。完成门：独立 PostgreSQL、并发/CAS、无环、Hash、重放和 stale 标记通过。**完成（2026-08-27）**：不可变 JSONB Version/唯一 Head、正式 Owner Set、完整 Edge 矩阵、单 GORM SERIALIZABLE 事务 Version/Head/Receipt/Outbox、跨版本精确重放、并发 CAS、权限与故障回滚均由真实 PostgreSQL 和全量 CI 证明；查询与 Kafka/Search/ELK 仍只属于 `SG-I04`。
- [x] `SG-I04`：在 `SG-I03` 发布契约上实现 Current/Version/Lens Query、Version Diff、上下游追踪和影响闭包。完成门：Query 仅读、大图有界、相同版本结果确定，全量 CI 通过并已提交。**完成（2026-08-27）**：Query、Kafka Event、Elasticsearch Search 与独立 ELK 日志四个交付单元均完成；Script/StoryGraph 业务链与日志链使用独立 SASL 身份、Topic/Schema/ACL/Retention/Group/DLQ/Index。三个应用 Binary 使用统一脱敏 JSON Logger，真实 Backend HTTP 日志经 Filebeat/Kafka/Logstash 进入严格 Log Index 并可由 Kibana Data View 查询；关联字段、敏感字段零泄露、非法日志 Hash-only DLQ、逐组件停机下 Owner 事务/Workflow/Search 不变及恢复后继续采集均由真实服务和全量 CI 证明。
- [x] `SG-I05`：只在 `SG-I04` 完成后建立 Backend-owned Stage Envelope/Policy/Candidate Revision，将 8 个过渡 Skill 收口为 `agent/skills/build-storygraph` Bundle、Stage Reference 和 Bundle Hash，原子删除旧 Skill 名。完成门：跨语言 fixture、golden、Bundle 完整性、旧 Invocation 精确路由和全量 CI 通过。**完成（2026-08-27）**：唯一 Bundle、十 Stage 严格 Wire/Pydantic Candidate、Backend 精确 Runtime Catalog 与首个不可变 Candidate Revision 已落地；旧 Skill/Invocation 入口和无消费者依赖已原子删除，本地真实 Codex、空 PostgreSQL、Temporal/MinIO/Kafka/ELK、三镜像及完整故障部署 CI 全部通过。
- [x] `SG-I06`：按已接受 `2055` 完成 HumanTask 列表/详情、Claim/Renew/Release、Decision 和 Resume Backend API，复用已有 Review/Workflow 事实。完成门：Owner Receipt、Signal unknown/recovery、权限、幂等/冲突和 API 重启恢复通过，无第二审核状态机。**完成（2026-08-27）**：冻结 Subject/Rubric、Lease 零泄漏、不可变 Decision、三阶段 Coordinator、五类既有 Owner 路由和真实 Temporal UNKNOWN 恢复均已落地；OpenAPI/生成 Client、空 PostgreSQL/真实基础设施、全量 CI/Compose/三镜像证据见 Acceptance。
- [x] `SG-I07`：在 `SG-I06` 真实 API 上交付最小 Review Workbench，显式区分 Task、Decision、Owner Apply 和 Workflow Resume。完成门：刷新、过期 Lease、unknown/conflict、键盘和可访问性自动化通过，不模拟 Backend 成功。**完成（2026-08-27）**：项目队列/固定详情、Claim/Renew/Release、冻结 Candidate 决议、同 Decision Resume 与 WorkflowRun/NodeRun 重取已通过真实 Client 和组件自动化；Claim Token 零持久化、Viewer/未知 Subject 只读、冲突重取、键盘与 standalone 镜像路由证据见 Acceptance。

`SG-I04` 同时承接 Event/Search/ELK 的首个真实消费者，不新增任务编号：先完成 StoryGraph 查询契约，再完成 Outbox Publisher + Kafka Envelope/Inbox/DLQ/Replay，再完成 Script/StoryGraph Elasticsearch Projection/Reindex/Search API，最后接通独立日志 Topic 与 ELK 管道；这些是该项内可分别完成、验证和提交的交付单元，但 `SG-I04` 只有在真实 Kafka/Elastic/日志故障 CI 全部通过后才完成。业务 Outbox 由 `SG-I03` 与 Owner 事务同库写入，Kafka ACK unknown 以同 Event ID 重试；Elasticsearch 与日志索引始终是可重建派生数据。

### Harness 与剧本解析

- [x] `SG-I08`：Definition-first 打通 `extract_source_evidence`：先发布 WorkflowDefinitionVersion 并创建 Run/NodeRun，再在该 NodeRun 下创建 ShardManifest/Invocation/Candidate Revision。完成门：Unicode 绝对区间、coverage、重分片、恢复和证据守恒通过。**完成（2026-08-27）**：不可变 versioned ShardManifest、Unicode 语义分片/overlap/marker hint、严格跨语言 Stage Input、Invocation Candidate 与确定性 aggregate 已挂入既有 Run/NodeRun；预算超限发布完整覆盖的新 Manifest，旧版本迟到结果仅留审计，同 identity unknown 重试、真实 Codex 与完整部署 CI 证据见 Acceptance。
- [x] `SG-I09`：Definition-first 接入有界 `analyze_story` map 和确定性 `reconcile_story` tree，产出带证据的 Bible/Claim Candidate Revision。完成门：Manifest/leaf 谱系、fan-in、重放、冲突和上游 stale 测试通过。**完成（2026-08-27）**：有界 map、固定 fan-in=2 的确定性 reduce tree、精确 Candidate/Evidence 谱系、上游 Head stale 拒绝、同名/同别名跨集不同 Key 身份守恒、并发 reduce 调度幂等，以及 map/reduce 超预算后的精确 Candidate 子区间与 versioned Manifest 路径重分片均已通过真实依赖 CI；单 shard deadline 通过显式幂等恢复命令重新排队原 Invocation，成功兄弟、Candidate、Decision 与 Receipt 均不清除，claim version 围栏拒绝迟到 Worker，Temporal 以持久定时器保持 NodeRun `RETRYING` 而不设置业务墙钟终止。恢复旅程连续 10 次通过，完整真实依赖 CI、三镜像与部署故障门禁通过；Candidate Repair、Head expected CAS 和旧下游 stale closure 按顺序留给 `SG-I10`。
- [ ] `SG-I10`：接入 Bible `review_storygraph` 与有界 Candidate Repair，每轮重跑确定性 Gate。完成门：Candidate Revision/Head CAS、冻结允许集、修复预算和旧下游 stale 通过。**前两个交付单元完成（2026-08-28）**：invocation/aggregate/repair 三类不可变 Revision origin 已形成严格互斥联合，content hash 与 revision hash 分层覆盖父版本和完整来源；Backend GORM 原子发布原语锁定 expected Head、重验父 Revision 与成功 Repair Result、创建 N+1 并 CAS 切换，两个竞争修复在真实 PostgreSQL 下仅一个成功。Go/Pydantic 双侧又冻结 Review target + deterministic Gate，以及 Repair target/review revision + Issue + allowlist + base fragment hash + read-only adjacency；Review 输出不能携带 Gate 字段，Issue Evidence 必须属于目标版本，Patch 不能越过允许字段或指向已发布 Graph。下一单元只实现幂等 Patch Receipt 与旧下游 stale closure；有界 Gate/Review 重跑仍保持未完成，因此本项不提前勾选。
- [ ] `SG-I11`：用公共 Human Gate 审批 Bible/identity/state/claim Candidate；正向决议只调用既有 Production Bible `Confirm` Owner Command，固化 confirmed Bible Gate output 与 `production_bible.confirm` Receipt。完成门：blocker 未清零不得通过；Confirm 不物化 Asset，Decision/Receipt/Node output 精确绑定并可恢复。
- [ ] `SG-I12`：只消费 `SG-I11` 的 confirmed Bible 输出，由 Backend Coordinator 在独立命令中原子物化 Character/Location Asset、SpecificationVersion、AssetState 和 ProductionBinding。完成门：幂等 Materialization Receipt、唯一 Owner、单 GORM 事务、失败回滚和反向追踪通过。
- [ ] `SG-I13`：Definition-first 接入 `segment_episodes` Candidate，仅产出有证据的边界/顺序/标题提案。完成门：全文 coverage、无重叠/缺口、稳定顺序和恢复通过，不创建 Episode。
- [ ] `SG-I14`：完成 Episode Plan Human Gate 与 Backend Owner 原子物化 Episode/Published ScriptVersion。完成门：边界冲突、幂等、全批回滚和 Receipt 验收通过。
- [ ] `SG-I15`：Definition-first 按 Episode Slice 接入 `analyze_episode` 与 `reconcile_episode`，产出 Scene/Dialogue/Beat/Occurrence/Claim Candidate。完成门：只消费已确认 Bible Snapshot，分片、相邻边界、恢复和引用门禁通过。
- [ ] `SG-I16`：完成 Scene/Beat/Occurrence/Claim Review、Human Gate 和 Planning Owner 全批应用。完成门：未知身份/状态不得自动创建，整批 Receipt、回滚和反查通过。
- [ ] `SG-I17`：从已物化 Bible/Episode/Scene/Beat/Occurrence/Claim Owner 事实编译 Core StoryGraphVersion。完成门：多集 DAG、Claim scope、Evidence、Owner Ref、Diff 和影响闭包全链通过。

### 分镜与视觉资产

- [ ] `SG-I18`：接入 Storyboard Draft，只消费非空正式 Specification/AssetState，产出可审核 Shot Intent 与 `needs_asset` 需求。完成门：Candidate 不进入正式 StoryGraphVersion，缺资产时不得创建 Shot。
- [ ] `SG-I19`：用公共 Human Gate 审批 Shot Intent/visual requirements；Storyboard Owner `FreezeIntentSet` 只冻结 Draft Set revision/hash、已接受 Intent 和视觉需求并返回 Receipt。完成门：Gate completed/Receipt/输出可恢复，不创建正式 Shot；拒绝、unknown 或漂移不得产生 Provider Cost/Job。
- [ ] `SG-I20`：只消费 `SG-I19` 的 approved intent 输出，按已接受并同步的 `2051` 完成 `reference_asset` Intent/Cost/Quota/Provider Job/Artifact 执行。完成门：Provider 未知结果按同一 Job 对账，不盲目重提，Artifact 只是 Candidate。
- [ ] `SG-I21`：完成 composite front/profile/back reference sheet 的确定性 QC、单一 CandidateSelection 和 AssetVersion 发布。完成门：身份/AssetState/EffectiveStyleSnapshot/lineage/view-role 门禁和选择幂等通过。
- [ ] `SG-I22`：让 `detail_shots` 只消费精确 READY AssetVersion，完成分片 Review 与 Candidate Repair。完成门：精确版本非空，跨 Scene 连续性、修复范围和重审门禁通过。
- [ ] `SG-I23`：完成 Storyboard Human Gate/Owner Apply，创建正式 Shot 并发布完整 ShotProductionBindingVersion，编译下一 StoryGraphVersion。完成门：全批原子、Binding 完整、精确 Owner Ref、冲突/回滚和重放通过。
- [ ] `SG-I24`：接入 `shot_frame` 生成、动态 Shot 执行、CandidateSet/Selection 和现有 ShotImageBindingVersion。完成门：与 ShotProductionBindingVersion 不混用，单 Shot 局部重跑、结果对账和反查通过。

### Canvas、完整原稿与最终验收

- [ ] `SG-I25`：用 React Flow + Dagre 实现按 Episode/Scene 加载的单人只读 Story Lens，与 Workflow Lens 明确分离。完成门：Query、Diff、影响闭包、大图分层加载和无写入入口通过。
- [ ] `SG-I26`：在只读 Lens 通过后增加类型化 Domain Intent 编辑、Owner Command、重编译和 Patch Diff；Yjs/Hocuspocus 另立设计。完成门：Canvas 无 Graph JSON/SQL 直写，过期 base 冲突和新 Version 反查通过。
- [ ] `SG-I27`：使用完整原稿执行全量机器统计、代表集人工细查、故障恢复与全量真实 CI，回填非浏览器最终证据。完成门：所有代码与全量 CI 完成，该 Acceptance Evidence 已独立提交。
- [ ] `SG-I28`：只在 `SG-I27` 通过并提交后运行 `agent-browser` Web Journey，回填浏览器/最终 Acceptance 并独立提交。完成门：浏览器、API、PostgreSQL Owner 事实和 Artifact 系谱一致，无未说明失败。

## 5. Requirement 覆盖路由

本节只给实施路由，不复制 Requirement 正文；逐 ID 的初始未勾选判定由 `SG-D21` Acceptance 维护。

| 实施段 | 主要 Requirement |
|---|---|
| `SG-I01` | `SG-ARC-*`、`SG-GRF-001`–`007`、`SG-OPS-004`–`008`、`SGA-BND-*`、`SGA-WIR-*`、`SGA-OPS-001`–`003` |
| `SG-I02` | `SGA-MOV-*` |
| `SG-I03`–`004` | `SG-GRF-*`、`SG-QRY-*`、`SG-EVT-*`、`SG-SRCH-*`、`SG-LOG-*`、`SG-JRN-001`、`SG-JRN-003` |
| `SG-I05` | `SGA-BDL-*`、`SGA-WIR-*`、`SGA-STG-001`–`003`、未使用依赖删除门 |
| `SG-I06`–`007` | `SG-REV-*`、`SG-FE-003`、`SG-OPS-*` |
| `SG-I08`–`010` | `SG-PRD-*` 对应 Stage 输出、`SGA-EVD-*`、`SGA-SHR-*`、`SGA-CAN-*`、`SGA-REP-*`、对应 `SGA-STG-*` |
| `SG-I11`–`017` | `SG-PRD-001`–`008`、`SG-REV-006`–`008`、`SG-GRF-*`、对应 `SGA-STG-*` |
| `SG-I18`–`024` | `SG-PRD-009`–`012`、`SG-VIS-*`、`SG-JRN-002`、`SGA-STG-006`–`008`、`SGA-REP-*` |
| `SG-I25`–`026` | `SG-FE-001`–`009`、`SG-QRY-*` |
| `SG-I27` | `SG-OPS-*`、`SG-JRN-004`、`SGA-COD-*`、`SGA-ERR-*`、`SGA-JRN-*`、`SGA-OPS-004`–`005` |
| `SG-I28` | `SG-JRN-005`、`SG-OPS-009`、`SGA-OPS-006`；不得新增实现 |

## 6. 真实 CI 演进门

- `SG-I01` 先执行并记录当前 Backend/Agent/Frontend/OpenAPI/Compose/Image/Hygiene 基线；若真实失败，先作为本项完整 CI 修复单元解决，不能在失败基线上叠加功能。
- `SG-I03` 起 Backend CI 每次使用空 PostgreSQL 由唯一 GORM Catalog 建表，并检查无 Migration 元数据、无 Raw SQL 业务 Repository、无第二 ORM。
- `SG-I04` 起同一 CI Job 启动真实 Kafka 4.3.1 KRaft、Elasticsearch 9.4.4 与最小 Filebeat/Kafka/Logstash/Elasticsearch 日志链，验证重复、乱序、断连、DLQ、Replay、Reindex、Alias、degraded/readiness；内存 Broker/Index 只可用于纯单元测试，不能抵扣集成门。
- Temporal、MinIO、Codex CLI、Runware Staging 在各自首个真实消费者任务加入对应 integration/journey；外部不可用必须显式失败或 unknown。
- `SG-I25` 起 Frontend CI 安装锁定 React Flow/Dagre 后继续运行 OpenAPI drift、lint、typecheck、unit 和 production build；组件测试不得模拟不存在的 Backend 成功。
- `SG-I27` 执行完整原稿、故障矩阵、全量真实 CI 和三类镜像/Compose；`SG-I28` 只能消费该已提交结果，使用 `agent-browser` 做最终 Browser→API→Owner/Temporal/Kafka/Search/Artifact 对账。

## 7. 停止与回滚点

- Schema/Owner/Hash/Wire fixture 未固定或跨语言不一致：停在 `SG-I01`，不创建业务表或最终 Skill Bundle。
- Skill 迁移存在双路径或字节漂移：在 `SG-I02` 同一原子任务内回正，不保留 fallback。
- Outbox 与 Owner 无法同一 GORM 事务、Kafka 与日志链未隔离、Search 无法从 PostgreSQL 重建：停在 `SG-I03/004`，不得让 Elastic 成为事实源。
- Human Decision、Owner Apply、Workflow Resume 无法按同一冻结 Subject 对账：停在对应 Gate，不用 Kafka 或页面状态补偿。
- Provider ACK/结果未知、配额/费用/资产版本不一致：按同一 Job/Receipt 对账，禁止重新计费式盲重试。
- 完整原稿或真实 CI 未通过：`SG-I27` 保持未完成；不得运行、引用或提前准备最终 `agent-browser` 通过声明。

本文完成 `SG-D20`。`SG-D21` 已建立初始全未勾选 Acceptance Criteria；其独立提交后只能从 `SG-I01` 开始编码。
