# StoryGraph 剧本解析 Harness 与内置 Skill 需求规格

> 状态：Agent Contract Requirement 已复核接受（`SG-D19`，2026-08-29）；既有实施证据只按未变合同保留，新增媒体职责边界初始待验收
>
> 设计依据：[StoryGraph Harness 与内置 Skill 设计](../design/3003-StoryGraph剧本解析Harness与内置Skill设计.md)
>
> 跨服务依据：[StoryGraph 跨服务需求规格](0010-StoryGraph内容图与DAG创作画布需求规格.md)
>
> 产品范围：只引用 [StoryGraph PRD](../prd/0010-StoryGraph内容图与DAG创作画布产品需求.md)，不建立第二份 Agent 产品愿景

## 1. 范围

本文只固定 Agent Runtime 与 Backend Agent Owner 之间的 Skill Bundle、Invocation Wire、Stage/Shard、Candidate Revision、Codex CLI、Review/Repair 和恢复契约。Production/StoryGraph/Workflow/Review/Asset/Generation/Media Provider/Kafka/Search/Frontend 的业务 Owner 与用户范围以 `0010` Requirement 为唯一来源。

历史 `production_bible`、`storyboard_draft` Invocation、8 个根 Skill、测试或 Acceptance 不能抵扣本规格。2026-08-27 后已按本规格产生的证据可继续证明未变合同；本次新增的媒体职责边界必须在 `SG-D21` 建立全未勾选目标项，不能由既有 Runware 或 Agent 证据抵扣。

## 2. 运行与所有权边界

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-BND-001` | Go Backend 唯一拥有 Stage 枚举、Definition Manifest、Execution Policy、Invocation/Grant/Lease/Result、ShardManifest、StageCandidateRevision/Head 和所有业务写入；Python 只能严格消费和返回 Candidate。 | Go/Python import、Wire、权限和数据库事实计数。 |
| `SGA-BND-002` | Agent Runtime 不得包含 ORM、数据库/对象存储/Kafka/Elasticsearch/Temporal/Provider client、业务 Repository 或公共业务 HTTP；唯一写入是当前 Invocation 的临时输出流。 | 依赖/环境/路由/网络扫描。 |
| `SGA-BND-003` | WorkflowDefinition/Temporal 只编排稳定宏观 Node；Stage shard/Invocation 必须挂到预先存在的 WorkflowRun/NodeRun，不得创建动态 Workflow Node 或 Agent Checkpoint 状态机。 | Backend integration + Temporal History/Replay。 |
| `SGA-BND-004` | Agent 成功不得自动 Confirm/Apply、创建正式 UUID、Episode/Scene/Claim/Asset/Shot/Binding/StoryGraphVersion、发送 Kafka Event 或恢复 Human Gate。 | Candidate-only contract 与零业务写入 E2E。 |
| `SGA-BND-005` | 首版 StoryGraphHarness 使用普通 Python 显式 Registry/函数，不使用 LangGraph 重复 Temporal 编排；若 `langgraph` 在目标实现后无真实消费者，必须从依赖和 lock 中删除。 | import/依赖/运行路径测试。 |
| `SGA-BND-006` | Provider Catalog/Connection/Credential/Profile/Binding、Generation Target/Job/Call/Receipt、Staging/QC/Selection、AssetVersion 与 Shot 图片/视频 Binding 全部由 Backend 拥有。Agent 只可接收无密钥的冻结业务 Ref/视觉要求并返回文本或结构化 Candidate；不得接收 Provider Secret/Endpoint、图片或视频字节/私有 URL，不得调用 Seedream、Seedance、GPT Image、Nano Banana 或任意媒体 Provider。 | Wire/env/dependency/network/fixture/零 Provider 调用与零业务写入测试。 |

## 3. Skill 目录迁移与最终 Bundle

### 3.1 原子迁移任务（`SG-I02`）

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-MOV-001` | `analyze-scene`、`draft-shots`、`extract-bible-evidence`、`plan-scene`、`reconcile-bible`、`repair-shots`、`review-bible`、`review-shots` 八个 Skill 必须按原名迁至 `agent/skills`；Skill Markdown/Reference 的原始 UTF-8 字节和相对路径不变。 | 迁移前后 manifest + SHA-256 golden。 |
| `SGA-MOV-002` | 同一提交原子切换 Loader、Docker build context/复制路径和测试到 `agent/skills`，并删除根 `.agents/skills`；任一运行时状态都不能双读、fallback 或优先级选择。 | Git diff、容器路径、旧路径缺失/新路径成功与 fallback 负向。 |
| `SGA-MOV-003` | `SG-I02` 只做行为保持迁移，不改 Guidance、Invocation kind/schema、调用次数、结果规范化或业务流程；所有现有 Agent/Backend/Frontend CI 必须保持通过。 | byte golden + 全量 CI。 |

### 3.2 最终 Bundle 收口任务（`SG-I05`）

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-BDL-001` | 最终运行入口唯一为 `agent/skills/build-storygraph/SKILL.md`；8 个过渡 Skill 名、根 `.agents/skills`、旧 Loader fallback 和无消费者 `agents/openai.yaml` 全部删除。 | 路径/Loader/镜像/运行入口负向扫描。 |
| `SGA-BDL-002` | `SKILL.md` 只保存 candidate-only、Evidence、Owner、四轴视觉和 DAG/Claim 等全阶段规则；阶段细则位于显式 `references/`，不得在 Python 中复制同一 Guidance。 | Bundle 结构、重复段落/引用检查。 |
| `SGA-BDL-003` | Registry 必须按 Backend Stage Key 显式映射 Candidate Schema 与允许的 Reference 列表；只读取 `SKILL.md` 和当前 Stage 声明文件，不递归拼接全部 Markdown、不动态发现插件。 | 每 Stage loaded-file golden 与未知 Stage 拒绝。 |
| `SGA-BDL-004` | Bundle Hash 对允许文件按相对 POSIX 路径排序，逐项覆盖路径 UTF-8 字节、内容长度和原始 UTF-8 内容后计算 Canonical SHA-256；路径逃逸、符号链接逃逸、缺失、多余或非 UTF-8 文件 fail closed。 | 跨语言 hash/golden/路径攻击。 |
| `SGA-BDL-005` | Manifest 必须同时冻结 definition/prompt/bundle/output-schema version、bundle hash、model capability、`allowed_tools=[]`、max model calls 和 max execution seconds；Version 与字节/Hash 任一单独漂移均拒绝。 | Go/Python manifest 正反 fixture。 |
| `SGA-BDL-006` | 非终态 Invocation 必须按冻结 Bundle Hash 路由精确 Agent image revision/digest；找不到返回 `skill_bundle_unavailable`，不得使用当前 Bundle、旧名称或相近版本。 | 滚动部署/旧 hash/缺镜像恢复。 |

## 4. Wire 与 Canonical Identity

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-WIR-001` | 公共 Invocation `kind` 最终只允许 `storygraph_stage`；旧 `production_bible/storyboard_draft` Kind 在 `SG-I05` 原子移除，不提供兼容 Union 或转发。 | Pydantic/Go strict schema/HTTP 422。 |
| `SGA-WIR-002` | Invocation 必须严格冻结 invocation id、wire schema version、input hash、Execution Policy，以及 stage/shard、workspace/project、source refs、exact upstream Candidate Revision refs、ShardManifest ref/shard、可选 base StoryGraph 和 stage input。 | 完整/缺失/未知字段 fixture。 |
| `SGA-WIR-003` | source ref 必须包含 owner kind/logical/version/revision/hash；upstream ref 必须包含 stage/shard/candidate revision id+hash/source invocation+result hash；Agent 不得补全 current/latest。 | 篡改/空 ref/漂移测试。 |
| `SGA-WIR-004` | Input Hash 覆盖 wire version、完整 canonical payload、排序 source/upstream refs、Manifest/Tree Path 和完整 Execution Policy/Codex contract；不包含 invocation id 或 input hash 自身。任一 Guidance/Schema/Policy/Upstream/Shard 变化必须换 Hash。 | Go/Python canonical golden 与单字段突变。 |
| `SGA-WIR-005` | `stage_instance_key = SHA-256("storygraph-stage-v1" + stage + shard_key + manifest_hash + input_hash)`；同一身份只能关联一个 Invocation/Result，相同身份不同 Result Hash 冲突。 | Backend 并发/重放。 |
| `SGA-WIR-006` | Result 只允许 `succeeded/failed/unknown`：成功时 candidate/result_hash 必填且 error 为空；失败/未知时 candidate/result_hash 为空且稳定 error 必填；所有 object `extra=forbid`。 | Schema 正反 fixture/fuzz。 |
| `SGA-WIR-007` | Result Hash 对规范化 Candidate 计算；Agent 不保存 Result。Backend 首次接受后不可变保存并重验 invocation/kind/stage/shard/input/result/executor identity。 | 跨语言 hash、重复/漂移。 |
| `SGA-WIR-008` | Execution Grant 必须绑定 Invocation ID/Input Hash/Policy Hash/expiry/attempt/fencing，使用恒时验签；过期、重放到不同输入、旧 fencing 或伪造 Grant 拒绝。 | Grant contract/security tests。 |

## 5. Stage 与 Reference 契约

最终 Stage 恰为以下十个，新增/删除/改义必须先更新 Design/Requirement/Bundle/Go/Python fixture：

| Stage | 必需 Reference | Candidate 重点 |
|---|---|---|
| `extract_source_evidence` | `source-evidence.md` | 原文 Observation 与绝对 Evidence |
| `analyze_story` | `story-analysis.md`、`entity-reconciliation.md` | Identity/State/World/Arc/Claim Fragment |
| `reconcile_story` | `entity-reconciliation.md`、`story-analysis.md` | 规范 Key、冲突和 tree reduce Candidate |
| `segment_episodes` | `episode-segmentation.md` | 有证据的 Episode Boundary Candidate |
| `analyze_episode` | `scene-structure.md`、`visual-identity.md` | Scene/Dialogue/Beat/Occurrence/Claim Fragment |
| `reconcile_episode` | `scene-structure.md`、`continuity-review.md` | Episode Structure Candidate |
| `draft_storyboard` | `storyboard-table.md`、`visual-identity.md` | Shot Intent、视觉需求、`needs_asset` |
| `detail_shots` | `shot-detail.md`、`visual-identity.md` | 完整 Shot Detail 与精确 Binding Candidate |
| `review_storygraph` | `continuity-review.md` + 被审阶段 Reference | Evidence-scoped Review Issue |
| `repair_candidate` | `continuity-review.md` + 冻结允许集 | CandidateRepairPatch |

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-STG-001` | Stage Registry 必须与上表一一对应；未知 Stage、额外 Reference、Candidate type 错配或 Backend/Agent Stage 列表漂移 fail closed。 | Registry/fixture/count=10。 |
| `SGA-STG-002` | Pydantic Model 是 Agent Candidate Schema 唯一代码事实；Codex 调用时临时生成 strict JSON Schema，不提交第二份生成 Schema。Go 只以 versioned wire fixture 对齐。 | 文件扫描/schema golden。 |
| `SGA-STG-003` | 所有 Candidate 只使用输入中给定的正式 Ref 或 schema 允许的临时 Key，保留 Evidence、置信/歧义、Review Issue 和谱系；不得返回业务 Command/SQL/Graph JSON overwrite。 | 每 Stage 正反 fixture。 |
| `SGA-STG-004` | Agent 不得依据常见套路补写原稿不存在的身份、关系、动机、伏笔、状态、场景或镜头；无 Evidence 时输出 ambiguity/issue，不伪造事实。 | adversarial/golden + representative manual review。 |
| `SGA-STG-005` | Relationship/causal/continuity/foreshadowing 等必须输出 Claim Candidate 的 participant/anchor/evidence/scope/polarity/status；不得输出可导致正式 DAG 成环的任意持久 Edge。 | Claim schema/环负向 fixture。 |
| `SGA-STG-006` | `draft_storyboard` 输入 Specification/AssetState 必须非空；缺精确 AssetVersion 时只输出 `needs_asset` 和严格 visual requirement，不得填空 UUID、URL 或 latest。 | Storyboard contract。 |
| `SGA-STG-007` | `detail_shots` 必须消费已接受 Intent 和精确 READY AssetVersion/Artifact/Lineage/Style/view-role refs；不得改变已接受 source coverage/identity/state/visual requirement。 | 漂移/缺失/修改负向。 |
| `SGA-STG-008` | Backend 负责全局 Shot 序号、连续 timecode、正式 UUID、Owner 映射和 ShotProductionBindingVersion；Agent 只提出 duration/镜头/声画/连续性 Candidate。 | Candidate 字段/Owner 输出测试。 |

## 6. Shard Manifest 与恢复

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-SHR-001` | Backend 为可 fan-out Stage 保存不可变 versioned ShardManifest，冻结 run/node/stage/root input、shard key/tree path/parent/kind/range/overlap/source hash/status、coverage hash 和 manifest hash。 | GORM contract/immutability/hash。 |
| `SGA-SHR-002` | 初始分片、排序、fan-in 和 reduce tree 必须确定性；相同输入产生相同 Manifest/Hash。Agent 不决定分片边界或 reduce tree。 | 随机遍历/跨重启 golden。 |
| `SGA-SHR-003` | 超预算不得截断输入或临时扩大预算；Backend 发布下一 Manifest Version，以有序子 shard 完整覆盖父范围并标父 superseded。除显式 overlap 外无缺口/重复。 | 重分片 coverage/overlap 负向。 |
| `SGA-SHR-004` | 旧 Manifest 迟到 Result 保留审计但不能进入当前聚合；只有 current Manifest 全部 active leaf 成功且 upstream/head/coverage/tree gate 有效时才能聚合。 | 迟到/失败/unknown/漂移。 |
| `SGA-SHR-005` | 聚合按确定性 tree 只传必要 Candidate refs/hash、Evidence refs 和冲突摘要；任一 reduce 输入超预算再次分片，不把全文或所有 Evidence 塞入一个 Invocation。 | 大输入/预算/调用图统计。 |
| `SGA-SHR-006` | 单 shard deadline/失败只影响该 stage instance；已成功 shard、Decision 和 Owner Receipt 保持不变。完整 WorkflowRun 不设置固定业务墙钟终止。 | Temporal/Agent restart/resume。 |

## 7. Evidence 与完整原稿

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-EVD-001` | Backend Normalize 后的 Source Slice 使用 Unicode code-point absolute half-open `[start,end)`；Go/Python 不得混用 UTF-8 byte 或 UTF-16 offset。Evidence 文本逐字等于该区间。 | 中英/emoji/标点跨语言 fixture。 |
| `SGA-EVD-002` | Slice 以段落/Scene/Episode marker 为优先边界；overlap 显式记录且不重复正式 coverage；相同 range+text hash 去重。 | coverage/overlap/golden。 |
| `SGA-EVD-003` | Episode marker 必须覆盖阿拉伯数字、常用中文数字和真实原稿格式；显式 marker 优先，AI 只为缺失/歧义边界生成 Candidate，不覆盖已确认边界。 | 完整原稿 marker/歧义 fixture。 |
| `SGA-EVD-004` | Candidate 中的 chunk-local offset 不能直接成为正式 Evidence；Backend 必须用冻结 Slice 校正并重验绝对 range/hash。 | 偏移篡改/边界测试。 |
| `SGA-EVD-005` | 两集 fixture 用于契约开发；最终必须运行完整原稿，报告各 Stage shard/coverage/candidate/issue/repair/unknown 统计，并对代表集人工细查。 | `SG-I34` 机器报告 + manual sample。 |

## 8. Candidate Revision、Head 与 Repair

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-CAN-001` | Backend-owned StageCandidateRevision 不可变，冻结 stage instance、revision no、parent id/hash、strict origin union、candidate content/hash、candidate revision hash 和 created_at；Head 以 expected revision/hash CAS。 | GORM/union/CAS/并发。 |
| `SGA-CAN-002` | `origin_kind=invocation|aggregate|repair` 恰有对应 origin 非空：Invocation 冻结 source result；Aggregate 冻结 manifest 和排序 leaf revision refs；Repair 冻结 repair invocation/result。 | Schema/check/golden。 |
| `SGA-CAN-003` | candidate content hash 只证明规范化内容；candidate revision hash 还覆盖 stage instance、revision、parent、origin 全材料和 content hash。 | canonical 单字段突变。 |
| `SGA-CAN-004` | 下游和聚合只绑定 exact candidate revision id+hash；Head 切换后，引用旧 Revision 的下游才被标 stale，原 Invocation/Result/Revision 不覆盖。 | stale closure/历史重放。 |
| `SGA-REP-001` | review_storygraph 只产生 Evidence-scoped Issue，不能把模型意见伪装成 Deterministic Gate；Tool Gate blocker 不能被 Reviewer 降级。 | Review schema/对抗 fixture。 |
| `SGA-REP-002` | CandidateRepairPatch 必须冻结 target revision id/hash、允许修改 Key、base fragment hash 和只读邻接；不得修改集合外字段或已发布 StoryGraphVersion。 | Patch scope/Graph published 负向。 |
| `SGA-REP-003` | Backend 以 expected Candidate Head Hash 应用 Patch，创建 N+1 Repair Revision 并 CAS Head；相同 Patch 重放同 Receipt，竞争 Patch 最多一个成功。 | PostgreSQL 并发/幂等。 |
| `SGA-REP-004` | 每轮 Repair 后必须重跑受影响闭包的全部 Deterministic Gate 与 review；只在无 blocker 时进入 Human Gate。修复轮次/模型调用预算耗尽时保持 needs_review/failed，不返回半成品成功。 | 有界循环/预算/闭包。 |

## 9. Codex CLI 与沙箱

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-COD-001` | 开发阶段真实 AI 执行使用本地 Codex CLI：ephemeral、read-only sandbox、ignore user config、临时空工作目录和临时 output schema；模型只来自 Backend Policy 或允许的 CLI 默认。 | 进程参数/环境/真实 Codex integration。 |
| `SGA-COD-002` | Tool Allowlist 必须为空；Shell、Web、Browser、Plugins、Skill Search、Workspace/File read/write 等任何 Tool event 立即 `tool_not_allowed`，Candidate 丢弃。 | 伪事件/真实 CLI trace。 |
| `SGA-COD-003` | Skill Guidance 由 Harness 显式读取并作为输入注入；Codex 工作目录不含仓库源码，不能依赖用户级 Skill 自动发现或项目配置。 | 临时目录、HOME/config 隔离、文件访问负向。 |
| `SGA-COD-004` | 每个 Invocation 严格执行 max model calls 与单 call/shard technical deadline；不得自动放宽。进程超时必须终止/回收，stderr/诊断脱敏且长度有界。 | budget/deadline/process cleanup/log scan。 |
| `SGA-COD-005` | Schema-invalid 输出只允许设计固定次数的结构修正，且每次计入 model-call budget；无法修正返回 `candidate_schema_invalid`。业务事实 blocker 不得通过 schema repair 改写。 | 修正次数/预算/错误分类。 |
| `SGA-COD-006` | Codex unavailable/transport result unknown 返回 `unknown: runtime_unavailable|agent_execution_unknown`；不得返回空 Candidate 成功或自动换 Provider。 | 启动失败/断连/kill 故障。 |

## 10. 错误与可观测性

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-ERR-001` | 至少稳定支持 `skill_bundle_invalid`、`skill_bundle_unavailable`、`invocation_policy_invalid`、`runtime_unavailable`、`agent_execution_unknown`、`candidate_schema_invalid`、`evidence_invalid`、`upstream_candidate_stale`、`execution_budget_exceeded`、`execution_deadline_exceeded`、`tool_not_allowed`。 | Go/Python error fixture/count。 |
| `SGA-ERR-002` | failed 与 unknown 必须按是否可安全重试区分；Backend 只对 retryable unknown 使用同 stage identity/Invocation 对账，不为 deterministic failed 自动创建新 Candidate。 | 重试策略/事实计数。 |
| `SGA-ERR-003` | 日志只记录 invocation/stage/shard/manifest/candidate revision、input/result/bundle hash 前缀、状态、耗时、调用数和稳定错误码；不得记录完整剧本/Candidate/Prompt/Grant/Token/环境凭据。 | 日志字段/敏感扫描。 |
| `SGA-ERR-004` | Trace context 从 Backend → Agent → Codex process wrapper 传播并在 Result/日志关联，但不进入 Candidate content hash。 | trace E2E/hash 不变。 |

## 11. 测试、CI 与交付

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-OPS-001` | Agent 测试只位于 `agent/tests/unit|contract|integration` 及既有独立分类；`agent/app` 不得含测试文件。 | 目录检查。 |
| `SGA-OPS-002` | 每个 Harness 任务必须通过 Ruff check/format、strict Pyright、全量 Pytest；跨语言 fixture 同时由 Go/Agent 测试读取；生成 Schema 不得形成 tracked drift。 | CI。 |
| `SGA-OPS-003` | Agent 镜像必须包含唯一 Bundle 和 Codex CLI，且以非 root 运行；镜像内根 `.agents`/旧 Skill 不存在，Bundle Hash 可在启动时验证。 | Docker build/run/digest/path。 |
| `SGA-OPS-004` | 测试模型桩只能证明协议/错误；`extract_source_evidence`、Bible、Episode、Storyboard Draft/Detail、Review/Repair 至少各有一次真实本地 Codex 合约，最终完整原稿不使用模型桩。 | Integration/Acceptance 条件标记与真实输出 hash。 |
| `SGA-OPS-005` | 每个 `SG-Ixx` 完整任务先 Red → Green → Refactor，通过局部和当前全量真实 CI，回填 Evidence 后独立 Git 提交；失败/跳过/缺 Codex 登录不得报告通过。 | Git/CI/Acceptance。 |
| `SGA-OPS-006` | 最终 `agent-browser` 只能在 `SG-I34` 全部实现、四类媒体 Provider、完整原稿和真实 CI 完成并提交后由 `SG-I35` 执行；Agent 测试不得提前替代浏览器验收。 | 顺序/Git 历史。 |

## 12. 端到端 Harness 契约

| ID | 必须满足的契约 | 验证 |
|---|---|---|
| `SGA-JRN-001` | 完整原稿按确定性 Unicode Slice → Evidence → analyze/reconcile story → review/repair → Bible Human Gate 输入，全程可重启且不重复成功 shard。 | Backend + Agent + Temporal + 真实 Codex。 |
| `SGA-JRN-002` | confirmed Bible/Identity/State 后完成 segment episodes → analyze/reconcile episode，未知身份/状态只产 Issue；Owner Apply 后可编译 Core StoryGraph。 | 两集/完整原稿跨服务 E2E。 |
| `SGA-JRN-003` | 正式 Scene/Beat/Occurrence + Specification/State 进入 draft_storyboard；`needs_asset` 阻断付费后，精确 READY AssetVersion 进入 detail_shots/review/repair；Agent 从不创建 Shot/Binding。 | 视觉前后两阶段 E2E。 |
| `SGA-JRN-004` | Bundle 滚动部署、单 shard deadline、Runtime unavailable、迟到 Result、Head 冲突和 Repair 竞争均从 Backend 事实恢复，同一 stage identity 不产生不同 Result/Candidate 正式效果。 | 故障注入矩阵。 |

## 13. `SG-Ixx` 映射与门禁

| 实施任务 | 本规格主要条款 |
|---|---|
| `SG-I01` | `SGA-BND-*`、`SGA-WIR-*`、`SGA-OPS-001`–`003` 的 fixture/边界，不建立最终 Bundle 目录 |
| `SG-I02` | `SGA-MOV-*` |
| `SG-I05` | `SGA-BDL-*`、`SGA-WIR-*`、`SGA-STG-001`–`003` |
| `SG-I08` | `SGA-EVD-*`、`SGA-SHR-*`、`extract_source_evidence` |
| `SG-I09`–`010` | `analyze_story/reconcile_story/review_storygraph/repair_candidate`、`SGA-CAN-*`、`SGA-REP-*` |
| `SG-I13`、`SG-I15` | `segment_episodes/analyze_episode/reconcile_episode` |
| `SG-I18`、`SG-I27` | `draft_storyboard/detail_shots`、`SGA-STG-006`–`008` |
| 所有 Agent 任务 | `SGA-COD-*`、`SGA-ERR-*`、`SGA-OPS-*` |
| `SG-I34` | `SGA-JRN-*` 与完整原稿/真实 Codex；只验证 Agent 候选链，不在 Agent 接入媒体 Provider |
| `SG-I35` | `SGA-OPS-006`，无新 Agent 实现 |

本文完成 `SG-D19` 复核。`SG-D20` 必须引用而非复制这些 Requirement，并按 `SG-I01`–`SG-I35` 唯一顺序安排实现；`SG-D21` 必须为本次新增的 `SGA-BND-006` 和改写的顺序门禁建立初始未勾选目标项，并保留未变合同的真实历史 Evidence。在 `SG-D21` 接受前不得编码。
