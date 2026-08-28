# Runware 图片 Provider 与 Generation 执行器设计

- 状态：已接受设计
- StoryGraph 视觉资产复核：已接受（`SG-D13`，2026-08-27）
- 产品依据：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- 架构依据：[后端领域模块功能设计](2002-后端领域模块功能设计.md) · [StoryGraph 内容图与 DAG 创作画布设计](0010-StoryGraph内容图与DAG创作画布设计.md) · [本地 Codex 分镜智能体执行框架](3002-本地-Codex-分镜智能体执行框架设计.md)
- 历史前置：[Shot 绑定目标与单 Shot 局部重跑验收](../acceptance/2050-Shot绑定目标与单Shot局部重跑验收记录.md)；只证明旧 shot-only 切片，不抵扣本设计验收

## 结论

MVP 只实现一个显式 `runware` 图片 Provider Adapter，但必须支持两种严格且不能混用的 GenerationTarget：

```text
reference_asset
  SpecificationVersion + AssetState + EffectiveStyleSnapshot
  → composite reference_sheet Candidate
  → READY/QC + Human CandidateSelection
  → Asset Owner publishes AssetVersion

shot_frame
  formal Shot + ShotProductionBindingVersion + exact AssetVersion refs
  → frame Candidate
  → READY/QC + Human CandidateSelection
  → Storyboard Owner publishes ShotImageBindingVersion
```

首个实施闭环必须先完成 `reference_asset`，因为 `detail_shots` 和可生产 Shot 依赖精确 READY AssetVersion；随后才实现 `shot_frame`。现有从静态 `provider_job_id` 读取 CandidateSet 的 shot-only 路径是当前事实，不是目标兼容入口。

Runware `taskUUID` 使用 Backend 已持久化的 Provider Job UUID。提交结果未知时只按同一 UUID 调用 `getResponse`/`getTaskDetails` 对账，不换身份、不盲目重提。本地 Codex CLI 只服务 Agent 文本/结构化 Candidate，不是图片 Provider 或无密钥降级方案。

## 1. 官方合同与选型

截至 2026-08-27，Runware 官方合同仍满足本项目的稳定身份和恢复要求：

- 原生 REST Endpoint 是 `https://api.runware.ai/v1`，请求为任务对象数组，可用 Bearer API Key；每个响应回显 `taskType/taskUUID`：[连接与认证](https://runware.ai/docs/platform/authentication)。
- 异步任务使用 `deliveryMethod=async`，可按同一 UUID 提交 `getResponse` 并采用退避轮询：[任务轮询](https://runware.ai/docs/platform/task-polling)。
- `getTaskDetails` 可取已执行任务的完整原始请求与响应，用于结果未知恢复：[任务详情](https://runware.ai/docs/platform/task-details)。
- `runware:z-image@turbo` 支持 text/image 输入、`numberResults=1..20`，宽高 `128..2048` 且为 16 的倍数，并回显 `taskUUID/imageUUID/imageURL`：[Z-Image-Turbo](https://runware.ai/docs/models/z-image-turbo)。

首版固定 `runware:z-image@turbo` 和原生 REST。Runware 官方有 Go CLI，但它是面向终端的独立程序，不是 Backend 应用 SDK；为一个 Provider 引入 CLI 子进程、用户配置或第二语言 Sidecar 会扩大故障面。Adapter 使用 Go 标准库 `net/http` 和严格本地 DTO，不引入动态 Provider Registry 或通用 Model Gateway。官方 CLI 只作为合同参考：[Runware CLI](https://github.com/Runware/runware-cli)。

Provider/模型可用性、Schema 和价格都可能变化。模型/尺寸/参数在实现前由唯一总 Plan 再核验官方文档；PriceQuote 由 Cost Owner 冻结，网页价格不进入业务事实。

## 2. 当前事实与目标缺口

| 能力 | 当前代码事实 | 目标缺口 |
|---|---|---|
| Cost/Quota | Generation Intent、Estimate/Reservation、Execution Claim 已实现 | 复用，不建立第二预算链 |
| Provider 事实 | Binding/Request/Job/Result Receipt 与 unknown 对账已实现 | ProviderSubmission 需携带完整 GenerationTarget Snapshot |
| Provider Gateway | 只有 `Submit/Query` Port 与受控测试 Adapter，生产组合根未接真实 Provider | 实现唯一 Runware `net/http` Adapter |
| Asset 输出 | Staging、Artifact Readiness、Candidate/QC/Selection 已实现 | 发布 AssetVersion 与 view-role Lineage |
| Shot Workflow | 当前从 Authoring Config 的静态 `provider_job_id` 展开 CandidateSet | 改为由 `shot_frame` Executor 创建/恢复真实 Job |
| Reference Asset | 尚无 GenerationTarget/Asset Identity/State/Version 闭环 | 先交付 character composite reference sheet MVP |

当前测试图片、受控 Gateway、已有 Provider Job 或静态 Catalog 只能用于 Red/离线故障测试，不能作为真实 Runware/视觉一致性验收。

## 3. 范围与非目标

本文包含：

- 一个固定 Runware REST Adapter 与凭据 allowlist；
- Backend-owned `GenerationTarget` 严格联合类型和不可变输入快照；
- `reference_asset` composite reference sheet 生成、QC、选择与 AssetVersion 发布；
- `shot_frame` 生成、选择与 ShotImageBindingVersion 发布；
- Submit/Query/下载/Staging/物化/Temporal 等待和 unknown 恢复；
- 真实 PostgreSQL、Temporal、MinIO、Runware 的凭据化验收。

本文不包含多 Provider/Fallback、ComfyUI/GPU 平台、Webhook 公网入口、LoRA/ControlNet、一致性训练、视频/Motion/Render、Provider 管理 UI、通用图片 Prompt Agent、Migration/Raw SQL、第二 ORM/数据库或兼容写路径。

## 4. GenerationTarget 唯一模型

`generation` 拥有不可变 `GenerationTarget`。首版使用一个 Record 和严格 JSONB 联合 Payload，不为两种目标各建一套表：

```text
GenerationTarget
├── id / workspace_id / project_id
├── kind = reference_asset | shot_frame
├── source_owner_ref / source_content_hash
├── policy_snapshot_ref / policy_hash
├── payload JSONB (strict union)
├── target_hash / revision=1
└── created_by / created_at
```

`reference_asset` Payload：

```text
asset_id / asset_kind
specification_version_ref
asset_state_ref
effective_style_snapshot_ref
output_kind = reference_sheet | location_reference | prop_reference
required_view_roles[]
prompt_version / positive_prompt / negative_prompt
width / height / number_results / output_format
```

`shot_frame` Payload：

```text
shot_ref
shot_production_binding_version_ref
sorted_exact_asset_version_refs[]
frame_role = first | key | last
effective_style_snapshot_ref
prompt_version / positive_prompt / negative_prompt
width / height / number_results / output_format
```

调用方只提交冻结 Owner Ref/Workflow Node 身份，不能自报最终 Prompt、URL 或 AssetVersion。Generation Application 通过 Production/Bible/Asset/Preset 的窄 Port 重读精确事实并构造 Snapshot；Target、Intent、Request 必须冻结同一个 Target ID/Hash。每次 Submit、Query、物化和重放都重新 canonicalize Snapshot，不只比较一个外部传入的 Hash。

新 `GenerationTarget` 和必要的 Target Ref 只在对应 Red 契约需要时加入唯一 GORM Catalog；使用 GORM Model/Transaction/Clause/Tag 空库同步，不创建 Migration、DDL、Raw SQL 或平行 Schema。

## 5. `reference_asset` MVP

### 5.1 Character composite reference sheet

角色参考资产首版固定为一个 composite `reference_sheet` Artifact，而不是隐式选择三张无聚合关系的图。GenerationTarget 冻结：

- 同一 Character Asset Identity；
- 当前 CharacterSpecificationVersion；
- 一个完整 AssetState；
- EffectiveStyleSnapshot；
- `required_view_roles=[front,profile,back]`；
- 固定 `1536×1024 PNG`，四个 Candidate，Prompt Version `character-reference-sheet-v1`。

Backend 在每个 Candidate 的 Lineage metadata 中声明固定三栏区域：left=`front`、center=`profile`、right=`back`。确定性 QC 验证 PNG、尺寸、三栏 region metadata、目标/输入 Hash 与 Artifact Readiness；它不能仅凭像素算法证明三栏语义正确。Human Review 必须检查实际 front/profile/back、同一角色身份、服装/状态、画风和禁改锚点，之后一次 CandidateSelection 只选择一个 composite Candidate。

被选 Artifact 仍不是 AssetVersion。Asset Owner 以 Selection、Target、Specification、AssetState、Style 和 Artifact Lineage 创建不可变 AssetVersion/Command Receipt；重复 Apply 返回同一版本，任一输入漂移失败关闭。Character Look 只是该 AssetVersion 的 typed Query View，不创建 `CharacterLookVersion`。

### 5.2 Location/Prop

同一个 `reference_asset` Target 后续可用严格 `output_kind/required_view_roles` 支持 Location/Prop，但首个 Provider 验收只要求 Character composite reference sheet。没有真实 Storyboard 需求前不预建地点多视角、道具旋转台或通用布局引擎。

## 6. `shot_frame` MVP

`shot_frame` 只能在正式 Shot 和 `ShotProductionBindingVersion` 发布后执行。Generation Application 必须重读并验证：

- Shot ID/revision/content hash 和 active 状态；
- 完整 ShotProductionBindingVersion ID/revision/hash；
- 每个 Occurrence/Asset Role 对应的精确 READY AssetVersion；
- AssetVersion Lineage 与 Identity/AssetState/Style/View Role 一致；
- Project Aspect Ratio、frame role 与生成策略。

Prompt 由已确认 Shot Detail、精确绑定和 Style Snapshot 通过版本化纯函数构造，不再调用一次 LLM。首版尺寸映射为 `9:16 → 768×1344`、`16:9 → 1344×768`、`1:1 → 1024×1024`，四个 PNG Candidate；所有值进入 Target Hash。

Human CandidateSelection 后，Storyboard Owner 创建 `ShotImageBindingVersion`。它只绑定被选 frame Artifact，不修改 `ShotProductionBindingVersion`，也不发布 AssetVersion。

## 7. Runware Adapter 合同

### 7.1 配置与 Secret

```text
IMAGE_PROVIDER=runware
RUNWARE_API_KEY=<secret>
RUNWARE_REQUEST_TIMEOUT_SECONDS=30
```

Provider Binding 固定 `provider_key=runware`、`model_key=runware:z-image@turbo`、`credential_ref=env/runware_api_key`。Credential Ref 只映射 allowlist 中的 `RUNWARE_API_KEY`；Secret 不进入数据库、Target、Hash、Receipt、日志或 Temporal History。生产 Endpoint 固定为官方 HTTPS；测试只通过构造函数注入本地 Server，不提供任意生产 Base URL 环境变量。

启用图片节点但缺少配置时 Backend Workflow Runtime 启动失败；未启用时不读取 Secret，也不注册假 Provider。

### 7.2 Submit

- `taskUUID` 等于 Backend ProviderJob UUID v4；
- 请求为单个 `imageInference` 任务数组，使用 Bearer Header、冻结 Model/Prompt/尺寸/候选数/PNG、`deliveryMethod=async`；
- 确认 `processing/success` 才映射 accepted/succeeded；明确 Provider Error 映射 failed；
- HTTP timeout、连接中断或响应身份不完整映射 unknown；
- Backend Request/Job 一旦存在，后续执行只能 Query，不能第二次 Submit。

### 7.3 Query 与历史恢复

- 首先按同一 UUID 调用 `getResponse`，采用有界指数退避；
- `processing → accepted`，完整成功 → succeeded，明确 error → failed，其余 → unknown；
- active 结果缺失或需要恢复已执行请求时，以同一 UUID 调用 `getTaskDetails`，并核对原始请求 Target Hash 等价；
- 成功输出数量必须等于冻结候选数，每项回显同一 taskUUID、唯一 imageUUID 和合法 imageURL；
- ProviderEventID 由 taskUUID、排序 imageUUID 与终态响应 Hash 确定生成，重复 Query 返回同一事实；
- Runware `cost` 只作诊断，不替代 Cost Owner 基于 PriceQuote 的结算。

### 7.4 下载与 Staging

- 只接受官方 Runware HTTPS 输出 Host；每次重定向都重新执行 DNS/IP/Host allowlist，拒绝环回、私网、链路本地和协议降级；
- 限制响应字节、Content-Type、下载/解码时间和像素数，完整解码 PNG 后计算 SHA-256、字节数与宽高；
- 使用稳定 Output Key 写 `staging/{workspace_id}/{provider_job_id}/{image_uuid}.png`；
- 只有已写入 Staging 的输出才能进入成功 Receipt；远端成功但下载/MinIO 暂时失败保持 unknown，后续 Query 同一任务并重试 Staging；
- Asset Owner 必须再次完整读取并独立验证，Adapter 检查不替代 Artifact Readiness。

## 8. Application 与 Workflow 执行

```text
Workflow Node
  → Generation Target Builder
  → Prepare Intent + Cost/Quota Reservation
  → Acquire Execution Claim
  → persist Request/ProviderJob
  → transaction ends
  → Runware Submit or Query
  → terminal Provider Receipt + Cost/Quota transition
  → Staging → Asset READY → Candidate/QC/CandidateSet
  → Human Gate → exact Owner Apply
```

任何网络调用都在 PostgreSQL 事务外。Executor 使用 NodeRun 稳定幂等身份；`accepted/unknown` 返回正常 `RETRYING`，Temporal Timer 后对账同一 Job。Node Cache 不承担远端防重，也不缓存 RETRYING/UNKNOWN；只有完整 CandidateSet 的 canonical `node-output-v1` 可成为已完成输出。

一个 NodeRun 只绑定一个 Generation Intent。`reference_asset` 节点必须在静态 DAG Config 中用 `asset_id + asset_state_id` 从同一 `approved_storyboard_intents` 输出精确选择一个 Target；同一批准集存在多个角色或状态时，由多个节点并行消费，不能在单节点内循环创建多个 Intent，也不能用数组顺序或运行时 Target ID 选择。Target Builder Receipt 以 WorkflowRun 为稳定边界供这些节点共同重放，每个 NodeRun 的 Preparation Receipt 独立且稳定。

`reference_asset` Node 输出 `generation_candidate_set`，Human Gate Apply 后输出精确 `asset_version`。`shot_frame` Node 输出同类型 CandidateSet，但 Human Gate Apply 后输出 `shot_image_binding_version`；HumanTask Subject/Owner Operation 必须区分两种 Target，不能用同一 Apply 分支猜测。

取消在提交前可以原子释放 Cost/Quota；已 Submit 的 Job 进入 Reconcile，不先释放 Reservation，也不把 Temporal CANCELLED 当作远端未执行。只有官方取消合同在未来 Requirement 中明确并经真实测试后才加入 Provider Cancel。

## 9. 状态与失败路径

| 场景 | Backend 事实 | 恢复动作 |
|---|---|---|
| Target Owner Ref/Hash 漂移 | 不创建 Intent | 重建 Target/新 Run |
| Cost/Quota 不足 | 无 Provider 调用 | 调整 Owner Policy 后新 Run |
| Submit 明确接受 | Job `SUBMITTED` | 同 UUID Query |
| Submit 结果未知 | Job `OUTCOME_UNKNOWN`，Reservation 保留 | 同 UUID Query/TaskDetails，禁止重提 |
| Provider 处理中 | Node `RETRYING` | Temporal Timer 后 Query |
| Provider 明确失败 | Job/Intent `FAILED`，Release | 修复后新 Run |
| 下载/MinIO 暂时失败 | Job 保持 unknown/待物化 | 同任务重试 Staging |
| 输出数量/身份/媒体漂移 | 不写成功 Receipt | 运维对账，不伪造成功 |
| Artifact 未 READY/QC 失败 | Candidate 不可选择 | 修复输入后新 Target/Run |
| Selection/Owner Apply 漂移 | 不发布 AssetVersion/Shot Binding | 重读当前事实后人工决定 |
| Worker 重启/Activity 重投 | 不创建新外部身份 | PostgreSQL + Temporal History 恢复 |

## 10. 安全、日志与事实源

- PostgreSQL/GORM Catalog 是唯一 SQL 事实源；Runware、Kafka、Elasticsearch、MinIO 都不是业务 Writer。
- API Key、Authorization Header、签名参数、原始 Prompt 和图片字节不进入普通日志、错误详情、Trace、Receipt 或测试快照。
- JSON 日志只记录 Backend Target/Intent/Request/Job ID、Target Kind、脱敏 Provider/Model、状态、耗时、输出数量和稳定错误码，并进入既定 Backend → Logstash → Elasticsearch → Kibana 链。
- HTTP Client 不对可能已写入的 POST 做 Transport Retry；Query 可退避重试但始终使用同一 UUID。
- 下载防 SSRF、重定向逃逸、DNS rebinding、压缩/像素炸弹、超限响应和媒体伪报。
- Runware URL 只作短时来源，必须立即写私有 Staging，不能成为 Asset Location。
- 不为 Provider 结果发布无消费者 Kafka Topic；业务检索仍由 Script/StoryGraph 投影承担。

## 11. MVP 验收与实施门禁

按以下顺序交付，具体任务仍由统一 Plan 的 `SG-Ixx` 表达：

1. Target strict union、Snapshot/Hash 和 Runware Adapter 离线 Red/Green；
2. `reference_asset` Character composite sheet 的真实 Runware/MinIO/Artifact/QC/Selection/AssetVersion 闭环；
3. `detail_shots` 消费精确 AssetVersion 并由 Storyboard Owner 发布 ShotProductionBindingVersion；
4. `shot_frame` 真实 Provider/CandidateSet/Selection/ShotImageBindingVersion 闭环；
5. unknown、重启、迟到结果、MinIO 故障、SSRF、Cost/Quota 与 Owner Apply 故障矩阵。

真实验收至少证明：一个 Character Identity 在两个 AssetState 下各发布一个 composite AssetVersion，每个 Artifact 的固定 metadata 覆盖 front/profile/back；一个正式 Shot 绑定正确 AssetVersion 后产生四个真实 frame Candidate，并只选择一个 ShotImageBindingVersion。所有 Provider Job、费用、Quota、Artifact、Candidate 和版本均不重复。

离线 Adapter Server/Fixture 只证明合同与失败路径，不能证明 Provider 可用。`SG-I20/SG-I21/SG-I24` 对应的凭据化 CI/Acceptance 必须真实调用 Runware，并记录脱敏 taskUUID、事实计数、Hash 和费用；缺少 `RUNWARE_API_KEY` 或额度时明确阻塞，不用 Codex、占位图、测试 Gateway 或旧 CandidateSet 报告通过。

Backend 测试只位于 `backend/tests`。只有 `SG-D17`–`SG-D21` 的统一 PRD/Requirement/Plan/Acceptance 完成后才能编码；代码与真实 CI 全部通过后，最后才用 `agent-browser` 验收 Web 全旅程。
