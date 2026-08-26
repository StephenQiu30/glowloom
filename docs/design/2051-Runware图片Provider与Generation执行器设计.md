# Runware 图片 Provider 与 Generation 执行器设计

- 状态：待评审
- 日期：2026-08-26
- 产品依据：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- 架构依据：[后端服务架构](2001-后端服务架构.md) · [后端领域模块功能设计](2002-后端领域模块功能设计.md) · [本地 Codex 分镜智能体执行框架](3002-本地-Codex-分镜智能体执行框架设计.md)
- 上游前置：[Shot 绑定目标与单 Shot 局部重跑验收](../acceptance/2050-Shot绑定目标与单Shot局部重跑验收记录.md)

## 结论

MVP 的首个真实图片 Provider 选择 Runware REST API，首个模型固定为 `runware:z-image@turbo`，并只实现一个显式的 `runware` Adapter。Shot Workflow 新增一个 Generation Activity：从 Production Owner 重读并冻结正式 Shot 与 Project 视觉信息，构造不可变图片生成输入，通过已有 Generation Intent、Cost/Quota、Execution Claim、Provider Job、Asset Readiness 和 CandidateSet 链路产生真实候选。Runware 的 `taskUUID` 直接使用 Backend 已持久化的 Provider Job UUID；提交结果未知时只按该 UUID 查询，不更换身份、不盲目重提。

本地 Codex CLI 继续只用于 Agent 服务内的剧本、制作圣经和分镜等文本/结构化候选调用。它不是图片 Provider，不写 Backend 业务事实，也不作为缺少 Runware 凭据时的兼容或降级实现。

本设计先解决真实图片生成这一硬前置。只有它验收通过后，Episode Workflow 才能为每个正式 Shot 获取真实 CandidateSet 并动态启动 `ShotWorkflow × N`；不得先用静态 `provider_job_id`、测试图片或空 Child Workflow 冒充扇出闭环。

## 1. 问题与现状

当前 Backend 已具备以下已验收能力：

- Generation Intent、Cost/Quota 预留、Execution Claim 与短时 Authorization；
- 不可变 Provider Binding、Request、Job、结果未知对账与终态 Receipt；
- Provider Output 进入私有 MinIO Staging 后的 Artifact Readiness、Candidate/QC 与 CandidateSet；
- Shot Workflow 对已物化 CandidateSet 的人工选择、正式绑定和单 Shot 局部重跑。

但生产 `workflow-worker` 仍把 `ProviderGateway` 装配为 `nil`，Shot Catalog 仍要求 Authoring 静态填写已有 `provider_job_id`。现有 `ProviderSubmission` 也只有 `InputHash` 与 `Units`，没有可供真实图片模型执行的 Prompt、尺寸、输出格式和候选数。直接添加某个 HTTP Client 不能形成可运行闭环。

因此本增量必须同时交付：

1. 可由 Owner 重建的不可变图片生成输入；
2. 一个真实、可按稳定任务身份查询的 Provider Adapter；
3. 把提交、查询、下载、Staging、物化和 Workflow 等待串起来的 Generation Executor；
4. 缺少配置、远程超时、结果未知、输出漂移和重启恢复的明确失败路径。

## 2. 方案选择

### 2.1 选择 Runware

Runware API 由调用方生成 UUID v4 `taskUUID`，响应回显同一身份；异步任务可按该身份轮询，`getTaskDetails` 可恢复过去任务的原始请求与响应。该合同可以直接映射现有 Provider Job 和 Unknown/Reconcile 语义：[平台任务模型](https://runware.ai/docs/platform/introduction)、[任务轮询](https://runware.ai/docs/platform/task-polling)、[任务详情恢复](https://runware.ai/docs/platform/task-details)。

首个模型固定为 [Z-Image-Turbo `runware:z-image@turbo`](https://runware.ai/docs/models/z-image-turbo)：它支持 text-to-image、PNG、1–20 个结果以及 128–2048 且 16 像素步长的宽高，覆盖本设计的三种尺寸与 4 候选。Runware 的开放模型集合将它标注为 Apache 2.0 和低成本选项，适合 MVP 的中文短剧场景；网页展示价格只作为选型事实，不进入 Cost Owner，实际计费仍由发布时的不可变 PriceQuote 决定。[Runware 开放模型集合](https://runware.ai/collections/best-open-models)

首版使用官方 REST 合同和 Go 标准库 `net/http`，不引入动态 Provider Registry。Runware 当前没有面向应用集成的官方 Go SDK；其开源 [Go CLI](https://github.com/Runware/runware-cli) 是独立命令程序，官方应用 SDK 主要覆盖 [Python](https://github.com/Runware/runware-python) 和 [JavaScript/TypeScript](https://github.com/Runware/runware-typescript)。为一个固定 Adapter 引入 CLI 子进程、跨语言 Sidecar 或 Node Runtime 会扩大故障面，偏离 Backend Writer 与 MVP 边界。

### 2.2 暂不选择的方案

- ComfyUI 本地 API 能通过 `/prompt`、`/history/{prompt_id}` 执行和查询工作流，但 `prompt_id` 在提交后才返回；本地部署还需要独立 GPU、模型权重和 Workflow JSON 生命周期。本增量不同时建设图片推理平台。[ComfyUI Server 路由](https://docs.comfy.org/development/comfyui-server/comms_routes)
- Replicate 支持异步 Prediction ID 和查询，但 Prediction ID 同样在创建响应后返回，且 API 预测输入、输出与文件默认一小时后清理；它不能像调用方生成的 Runware `taskUUID` 一样直接满足当前未知结果恢复边界。[Replicate HTTP API](https://replicate.com/docs/reference/http/)、[数据保留](https://replicate.com/docs/topics/predictions/data-retention/)
- Codex CLI 不提供本项目所需的图片任务、稳定图片 Job 查询和 Staging 输出合同，因此只保留在 Agent 文本/结构化调用边界内。

## 3. 范围与非目标

本设计包含：

- 一个固定 `runware` Provider Adapter 和环境凭据解析；
- Shot 到 `generation-image-input-v1` 的确定性输入快照；
- 一个图片 Generation Activity、持久等待、查询对账和 CandidateSet 输出；
- Runware 输出下载、私有 MinIO Staging、字节与图片元数据核验；
- 真实 PostgreSQL、Temporal、MinIO 与真实 Runware 低成本调用验收。

本设计不包含：

- 多 Provider 路由、Fallback、Provider 市场、动态插件或通用 Model Gateway；
- ComfyUI 部署、GPU/模型权重管理、Webhook 公网入口或 Callback 验签；
- Reference Image、ControlNet、LoRA、角色一致性训练、视频、Motion、Render；
- Episode 动态 Child Workflow 扇出、前端 Provider 管理页面或前端单 Shot 修复交互；
- Migration 文件、DDL/Raw SQL、第二 ORM、第二 SQL 事实源或兼容写路径。

## 4. 业务与模块边界

```text
Shot Workflow Activity
  → Generation Application
      → Production ShotGenerationSource Port（只读正式 Shot/Project）
      → Generation Intent + Cost/Quota + Claim（PostgreSQL/GORM）
      → Runware Adapter（事务外 Submit/Query）
      → MinIO staging/{workspace_id}/{provider_job_id}/...
      → Asset Owner Readiness
      → Generation Candidate/QC/CandidateSet
  → Workflow node-output-v1
```

- `production` 是 Shot、Project Aspect Ratio 与 Visual Style 的唯一 Owner。
- `generation` 是生成输入快照、Intent、Provider Request/Job/Receipt、Candidate 和 CandidateSet 的唯一 Owner。
- `workflow` 只持有 Node 输入/输出引用、运行投影和 Temporal 协调事实。
- `asset` 继续唯一决定 Artifact 是否 READY；Runware Adapter 不能直接创建 Artifact/Candidate。
- Secret 只由 workflow-worker 进程环境解析，不进入 PostgreSQL、Node Snapshot、Receipt、日志、Hash 或 Temporal History。
- Agent/Codex CLI 不参与图片字节生成、Provider 对账、Staging 或 Owner 写入。

## 5. 不可变生成输入

新增 `generation-image-input-v1`，在 `PrepareImageGeneration` 前由 Generation Application 通过 `ShotGenerationSource` Port 重读正式事实并构造。调用方只提供 Workflow Run/Node 身份和冻结的 `production_shot` 引用，不能自报 Prompt 或 Project 风格。

快照最小字段为：

```text
schema_version = generation-image-input-v1
shot_id / shot_revision / shot_content_hash
project_id / project_revision
aspect_ratio / visual_style
prompt_version = shot-spec-prompt-v1
positive_prompt / negative_prompt
width / height / number_results / output_format
```

首版 Prompt 由已通过人工确认的 Shot `title/spec`、Project `visual_style` 和 Aspect Ratio 确定性拼接，不再增加一次模型调用。固定尺寸映射为 `9:16 → 768×1344`、`16:9 → 1344×768`、`1:1 → 1024×1024`；候选数固定为 `4`，格式固定为 `PNG`。这些值与 Prompt Version 一起进入 canonical Input Hash。

`GenerationIntent` 保存规范化 JSONB 输入快照和 Hash，`GenerationRequest` 冻结同一快照 Hash。Repository 每次提交、查询、物化和重放都重新 canonicalize 并核对，不能只信任哈希字符串。该 JSONB 加入现有 GORM Model Catalog，不创建 Migration 或平行 Schema 来源。

首版只做 text-to-image。当前正式 Shot 没有可独立重建的 Asset Reference 列，因此不得伪造 Reference Image；角色/场景一致性引用必须在后续独立设计中先修复 Production Shot 事实，再接入 Provider。

## 6. Runware Adapter 合同

### 6.1 配置

只新增以下运行配置：

```text
IMAGE_PROVIDER=runware
RUNWARE_API_KEY=<secret>
RUNWARE_REQUEST_TIMEOUT_SECONDS=30
```

Provider Binding 固定使用：

```text
provider_key = runware
model_key = runware:z-image@turbo
credential_ref = env/runware_api_key
```

`credential_ref` 只允许映射到显式 allowlist 中的 `RUNWARE_API_KEY`，不允许把任意环境变量名、文件路径或 URL 当作凭据引用。生产 Adapter 固定调用 `https://api.runware.ai/v1`；测试通过构造函数注入本地 HTTP Server，不提供可把生产流量改向任意 URL 的环境变量。生产环境启用图片 Generation 节点但缺少任一必要配置时，workflow-worker 启动失败；开发环境不启用该节点时不读取密钥，也不注册伪 Provider。

### 6.2 Submit

- `taskUUID` 使用 Backend `ProviderJob.ID`，不使用随机外部身份。
- 请求发送 `imageInference`、冻结 Model、Prompt、尺寸、候选数、`PNG` 和异步交付方式。
- 首次收到 `processing` 或已接受响应时返回 `accepted`，`ProviderJobKey` 固定为同一 Job UUID。
- 同一 Provider Job 在 Backend 中只允许进入一次 Submit 分支。数据库已存在 Request/Job 后，所有执行路径只能 Query。
- HTTP 超时、连接中断或无法确认远端是否接受时返回 `unknown`；不得再次 Submit。

### 6.3 Query 与终态

- Query 只按已冻结的 Job UUID 调用 Runware `getResponse`；必要时用 `getTaskDetails` 恢复已完成请求/响应。
- `processing` 映射为 `accepted`；明确 Provider Error 映射为 `failed`；无法判定、任务不存在、响应不完整或身份漂移映射为 `unknown`。
- 成功结果必须恰有冻结的候选数，每项必须回显同一 `taskUUID` 并带唯一 `imageUUID` 与 HTTPS `imageURL`。
- `ActualUnits` 使用已成功返回并完成 Staging 的图片数量；Runware 浮点费用只作为诊断字段，不替代 Cost Owner 基于冻结 PriceQuote 的金额事实。
- `ProviderEventID` 由 `taskUUID + 排序 imageUUID + 终态响应 Hash` 确定性生成，重复 Query 返回同一值。

### 6.4 下载与 Staging

- 只接受 Runware 官方 HTTPS 输出 Host；禁止重定向到环回、私网、链路本地地址或非 allowlist Host。
- 单个响应和图片使用明确大小上限与读取截止时间，不把完整字节写入日志或 PostgreSQL。
- Adapter 读取图片完整字节，真实解码 PNG，计算 SHA-256、字节数和宽高，再以稳定 Output Key 写入 `staging/{workspace_id}/{provider_job_id}/...`。
- 成功 Receipt 只引用已经写入 Staging 的输出。远端已成功但下载或 MinIO 暂时失败时保持 `unknown`，后续 Query 同一任务并重试 Staging，不生成第二个远端任务。
- 后续 Asset Owner 仍重新读取对象并独立验证；Adapter 的检查不能替代 Artifact Readiness。

## 7. Workflow Executor

发布新的不可变 Shot Catalog 版本，不修改 `lanverse.shot@1.0.0` 或 `@2.0.0`：

```text
input.production_shot
  → activity.generation_image
  → human.generation_image_review
  → production.shot_image_binding
```

`activity.generation_image` 消费 `production_shot`，输出 `generation_candidate_set`，不再从 Authoring Config 接收 `provider_job_id`。Executor 使用 NodeRun 的稳定身份依次执行：

1. 重读并冻结 Shot 生成输入；
2. 准备 Intent 和 Cost/Quota；
3. 获取 Execution Claim；
4. Submit 或 Query 固定 Runware Job；
5. `accepted/unknown` 返回 `RETRYING`，由 Temporal 持久 Timer 后重查；
6. `succeeded` 物化全部输出并返回 CandidateSet；
7. `failed` 以稳定错误码终止节点并释放已冻结 Cost/Quota。

Intent、Request、Job、Artifact、CandidateSet 已存在时，Activity 重放只重读 Owner 事实。Node Cache 不承担远端提交防重，也不能缓存 `RETRYING/UNKNOWN`；只有完整 CandidateSet 的 canonical `node-output-v1` 可以进入已完成缓存。

取消在本增量内先禁止新的 Submit，并停止 Temporal 后续轮询；已经提交的 Runware Job 进入 Reconcile，不能先释放 Cost/Quota。Provider Cancel API 和取消后迟到结果的结算策略需要在派生 Requirement 中明确后才能实现，不能把 Workflow `CANCELLED` 等同于远端未产生费用。

## 8. 状态与失败路径

| 场景 | Backend 结果 | 恢复动作 |
|---|---|---|
| 缺少 Provider 配置/Secret | 组合根启动失败或节点明确 unavailable | 修复配置后启动，不创建 Intent |
| Shot/Project/权限漂移 | 节点失败关闭 | 重新发布或启动新 Run |
| Cost/Quota 不足 | 准备失败，不调用 Provider | 调整 Owner Policy 后新 Run |
| Submit 明确接受 | Job RUNNING，节点 RETRYING | 同一 Job UUID Query |
| Submit 网络结果未知 | Job UNKNOWN，节点 RETRYING | 同一 Job UUID Query，禁止重提 |
| Provider 仍处理 | Job RUNNING，节点 RETRYING | Temporal Timer 后 Query |
| Provider 明确失败 | Job/Intent FAILED，双 Reservation Release | 新 Run 才能产生新任务 |
| 远端成功，下载/MinIO 暂时失败 | UNKNOWN，不写成功 Receipt | Query 同一任务并重试 Staging |
| 输出数量/身份/媒体漂移 | UNKNOWN，保持 Reservation | 运维对账，不伪造成功 |
| Staging 与 Receipt 一致 | Job/Intent SUCCEEDED，Settle/Consume | 物化 Artifact/CandidateSet |
| Worker 在任一步骤重启 | 不创建新外部身份 | 从 PostgreSQL + Temporal History 恢复 |

## 9. 安全、日志与事实源

- PostgreSQL/GORM Model Catalog 是唯一 SQL 事实源；不增加 Migration 目录、DDL、Raw SQL 或 Provider 本地数据库。
- Runware API Key、Authorization Header、下载签名参数和图片字节不得进入日志、错误详情、Trace、Receipt、Temporal History 或测试快照。
- 日志只记录 Backend Request/Intent/Job ID、脱敏 Provider Key、状态、耗时、输出数量和稳定错误码。
- HTTP Client 禁用无限重试；Transport Retry 不能跨越可能已写入的 POST。Query 可以按退避策略重试，但始终使用同一 Job UUID。
- 下载必须防 SSRF、重定向逃逸、压缩炸弹、超限响应和声明媒体类型欺骗。
- Runware 当前输出 URL 默认有保留期；Adapter 必须在终态查询时立即写入私有 Staging，不把远端 URL 当作长期 Asset Location。[Runware 平台任务模型](https://runware.ai/docs/platform/introduction)

## 10. 验收门

实现只有同时满足以下证据才算本切片完成：

1. Red → Green → Refactor 覆盖输入快照、Adapter 合同、未知结果、身份漂移、下载安全与 Executor 状态机。
2. 真实 PostgreSQL、Temporal 与 MinIO 下，从正式 Shot 启动新 Catalog，得到 4 个真实 Runware 图片输出、4 个 READY Artifact、4 个 Candidate/QC、1 个 CandidateSet，并可完成 Human Selection 与 Shot Binding。
3. 在 Submit 响应丢失、Worker 重启、Query 处理中、MinIO 暂不可用和 Activity 重投场景下，远端 `taskUUID`、Backend Request/Job、费用、Quota、Artifact 与 Candidate 均不重复。
4. 对真实图片重新读取并核对 SHA-256、字节数、PNG 解码宽高、Staging 前缀和 Receipt；非图片、超限、重定向逃逸或元数据漂移均不能成功。
5. 缺少 Runware Secret 的生产配置真实失败，不使用 Codex CLI、测试 Gateway、占位图或已有 CandidateSet 兼容放行。
6. Backend、Agent、Frontend、Compose、三类镜像与仓库卫生的 Required CI 全绿；生成 Client 无非预期漂移。
7. 独立 Acceptance 记录真实命令、Provider 任务身份（不含 Secret）、事实计数、Hash 和残余风险，并在完整小任务提交后保持工作区可解释。

## 11. 评审与实施前置

本设计待用户接受。接受后才按以下顺序继续：

```text
Design 接受
  → 派生 PRD
  → 派生可测试 Requirement
  → 派生实施 Plan
  → Red / Green / Refactor
  → 真实 Provider Acceptance
  → 独立 Git 提交
```

开始编码前还必须具备一个有低成本额度的 Runware 账户、可由本机/CI Secret 注入的 `RUNWARE_API_KEY`，并通过现有 Owner Command 为 `generation.image` 建立与 `runware:z-image@turbo` 实际计费口径一致的 Budget、Quota 和不可变 PriceQuote。没有这些外部条件时可以完成离线 Adapter 合同测试，但不能把真实 Provider 验收报告为通过。
