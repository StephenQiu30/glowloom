# 通用媒体 Provider 与 Generation 执行器设计

- 状态：待重新接受（2026-08-29）
- 变更原因：用户将固定 Runware/环境变量方案调整为可空启动、Backend 管理、Web 配置的通用图片/视频 Provider 服务；Seedream 5.0 Pro 及以上、Seedance 2.0 及以上、GPT Image 2 与 Nano Banana 都是当前必须真实接入的固定范围；连接创建体验参考 CC-Switch 的“预设 + 必填字段”方式
- 取代范围：取代本文件此前已接受的“固定 Runware 图片 Provider”目标；既有 Runware 代码与验收只作为当前事实，不形成兼容要求
- 产品依据：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- 架构依据：[后端领域模块功能设计](2002-后端领域模块功能设计.md) · [StoryGraph 内容图与 DAG 创作画布设计](0010-StoryGraph内容图与DAG创作画布设计.md) · [前端功能模块设计](1002-前端功能模块设计.md)
- 实施门禁：本设计重新接受前只允许事实核验与设计修订；接受后先同步受影响 Design，再更新 PRD、Requirement、Plan 与全未勾选 Acceptance，最后才能修改代码

## 结论

Lanverse 需要的是 Backend 内部的通用媒体 Provider 能力层，不是固定 Runware 客户端，也不是允许浏览器任意转发 HTTP 的“万能模型网关”。目标形态如下：

```text
Backend Built-in Preset Catalog（无 Secret、代码版本化）
  → Web Workspace Provider Settings 选择预设并补齐必填字段
  → Backend 实例化 Provider Connection / Secret / Model Profile Commands
  → PostgreSQL/GORM immutable versions

Project Workflow
  → exact Project Provider Binding
  → frozen GenerationTarget + ModelProfileVersion + CredentialVersion
  → Backend Provider Registry
      ├── volcengine_ark / Seedream 5.0 Pro+ image strategy
      ├── volcengine_ark / Seedance 2.0+ video strategy
      ├── openai / GPT Image 2 image strategy
      └── google_gemini / Nano Banana image strategy
  → Provider Job/Receipt
  → private Staging / Artifact / CandidateSet
  → Human Gate / exact Owner Apply
```

服务必须允许零 Provider 配置启动。没有连接或项目 Binding 时，Backend、Frontend、StoryGraph 和非视觉 Workflow 正常可用；视觉生成命令在 Cost/Quota 和远程调用前返回稳定的 `provider_configuration_required`，不得假成功、使用占位图或回退到 Codex。

2051 完整交付必须真实接入四类 Adapter：火山引擎方舟 Seedream 5.0 Pro 及以上承担图片生成，Seedance 2.0 及以上承担视频生成，OpenAI GPT Image 2 与 Google Nano Banana 承担图片生成。不存在“先预留、再决定是否接入”的可选状态；实施文档只能拆解依赖与独立提交顺序，不能改变这一完成定义。Web 可以创建三类 Provider 连接、录入或轮换 API Key、登记受支持的外部 Model ID/Endpoint ID、配置有类型的模型参数并为 Project/Purpose 发布 Binding。Provider API Key 不再由 `.env` 配置，也不返回浏览器。

这是同一个 2051 交付门中的强制范围，也是当前固定完成定义：

| 能力 | 本轮状态 | 最小真实完成证据 |
|---|---|---|
| Seedream 5.0 Pro 及以上 | **必须接入** | 精确 ModelProfile/Binding、真实付费调用、私有 Staging、CandidateSet 与 Owner Apply |
| Seedance 2.0 及以上 | **必须接入** | 当前基线中 2.0、2.0 Fast、2.0 Mini、2.5 的精确 Profile，以及真实异步任务、重启恢复、视频 QC 和 Owner Apply |
| GPT Image 2 | **必须接入** | 独立 OpenAI 连接/Profile/Binding、真实付费调用、Staging 与 Owner Apply |
| Nano Banana | **必须接入** | Nano Banana 2 Lite、2、Pro 与 Legacy 四个当前官方模型的独立精确 Profile、真实调用、Staging 与 Owner Apply |

`及以上` 以开发进入对应 Adapter 任务时火山账号中官方已可调用的版本为冻结基线，不是只做一个“大于等于 2.0”的字符串判断。每个纳入基线的版本都必须有精确官方 Model ID/Endpoint ID、独立参数与计费合同、Adapter 合同测试和真实凭据验收；未知新版本不得靠通配透传声称已接入。

连接体验参考 CC-Switch，但持久化语义按 Lanverse 重做：Backend 提供带稳定 `preset_key + preset_version` 的内置连接预设和模型预设；Owner 选择预设、只补齐 API Key/Endpoint ID 等受控字段后，Backend 把“预设来源 + 已解析非敏感快照 + 独立密文凭据”写成不可变 Connection/Profile Version。预设后续升级只影响新建配置，不在线改写既有连接；Project Binding 永远链接精确版本，而不是链接一个可变的“当前 Provider”。

本文的“持久连接”指跨 Backend/容器重启仍存在的配置谱系、密文凭据和项目 Binding，不是把 TCP Socket 或 SDK Client 写进数据库。进程可以复用不携带凭据的标准 HTTP Transport 连接池；每次调用只把解密后的 Secret 注入当前请求，连接池失效不影响业务事实或恢复能力。

GPT Image 2 与 Nano Banana 不是预留项或可选项。它们必须实现真实 Adapter、真实 Web 连接/Profile、真实 Project Binding 和真实凭据化生成验收；仍然不得用空目录、假 Adapter、“返回未实现”或只显示模型名称来冒充接入完成。

## 1. 官方接口事实与合理推断

以下事实于 2026-08-29 按官方文档重新核验；模型名称、版本、价格和参数可能变化，因此外部 Model ID 必须成为版本化配置事实，不能散落在代码常量或前端枚举中。

### 1.1 火山引擎方舟

- 方舟推理使用 `Authorization: Bearer <ARK_API_KEY>`；官方提供 `https://ark.cn-beijing.volces.com/ping` 作为带认证的连通性检查，因此 Web 的显式“验证连接”不需要暗中发起一次付费生成：[方舟开始使用](https://www.volcengine.com/docs/82379/1795150) · [连通性检查](https://www.volcengine.com/docs/82379/1339360?lang=zh)。
- 图片生成使用 `POST https://ark.cn-beijing.volces.com/api/v3/images/generations`，请求可使用 Model ID 或已配置的 Endpoint ID；本项目最低接入 Seedream 5.0 Pro，当前可核验技术 ID 为 `doubao-seedream-5-0-pro-260628`：[图片生成 API](https://api.volcengine.com/api-docs/view?action=ImageGenerations&serviceCode=ark&version=2024-01-01) · [火山开发者社区模型实测](https://developer.volcengine.com/articles/7670063057116889098)。
- Seedream 支持文本和参考图片输入；具体版本可支持组图、URL 或 Base64 输出，URL 当前只保留 24 小时。能力必须由 Adapter 版本与 ModelProfile 校验，不能由用户自由声明：[Seedream 图像生成处理器](https://www.volcengine.com/docs/6492/2221472?lang=zh)。
- 视频生成使用 `POST https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks` 创建异步任务，返回远端任务 ID，再使用查询接口读取终态；本项目最低接入 Seedance 2.0，当前官方技术 ID 包括 `doubao-seedance-2-0-260128` 与 `doubao-seedance-2-0-fast-260128`。Seedance 2.x 的比例、时长、音频和素材能力必须按精确模型校验：[创建视频生成任务](https://www.volcengine.com/docs/82379/1520757?lang=zh) · [查询视频生成任务](https://www.volcengine.com/docs/82379/1521309?lang=zh) · [Seedance 2.x 官方模型枚举](https://api.volcengine.com/api-docs/view?action=CasePlatformV1BadcaseReport&serviceCode=ark&version=2024-01-01)。
- 火山方舟首页当前同时展示 Doubao Seedance 2.0 Mini/2.0 Fast 和已正式上线的 Seedance 2.5，因此“Seedance 2.0+”当前基线包含 2.0、2.0 Fast、2.0 Mini 与 2.5，不能只实现其中一个后停止：[火山方舟首页](https://console.volcengine.com/ark) · [Seedance 2.5 官方活动页](https://www.volcengine.com/activity/seedance25)。公开创建接口允许 Model ID 或 Endpoint ID；若未登录公开文档没有给出 Mini/2.5 的稳定直调 ID，Web 必须要求 Owner 从当前火山账号填入并冻结官方 Model ID/Endpoint ID，不得由代码或第三方网页猜测名称。
- 官方方舟产品定价页当前将 Seedance 2.0、2.0 Fast、2.0 Mini 按百万 Tokens 定价，并区分“含视频输入/无视频输入”，而不是给出一个统一的每秒单价：[火山方舟产品与定价](https://www.volcengine.com/product/ark)。价格会变化且 2.5 需按账号实际开通信息核验，所以代码不硬编码公网价格或虚构“秒数 → Tokens”公式；视频时长继续是 Generation/QC 参数，不是 Lanverse Cost Ledger 的计费 Metric。
- 视频任务和输出不是长期业务存储。官方当前说明任务 ID 只保留 7 天，因此成功结果必须立即进入 Lanverse 私有 Staging/Artifact，不得把 Provider URL 当成正式资产。

### 1.2 OpenAI GPT Image 2

- OpenAI 当前正式模型标识为 `gpt-image-2`，可通过 `POST /v1/images/generations` 与 `POST /v1/images/edits` 进行图片生成和编辑，支持文本输入、图片输入与图片输出；当前还有可固定的 dated snapshot `gpt-image-2-2026-04-21`：[GPT Image 2 模型](https://developers.openai.com/api/docs/models/gpt-image-2) · [图片生成指南](https://developers.openai.com/api/docs/guides/image-generation)。
- 官方 Image API 使用 Bearer API Key，Generations 请求支持 `n`，GPT Image 默认返回 `data[].b64_json`；尺寸、质量和输入图片会改变 Token/费用估算。Lanverse 选择非流式 Image API 而不是带主模型与工具调用的 Responses API，因为当前是一次性生成/编辑，不需要对话状态、部分图片事件或额外主模型费用：[Images API Reference](https://developers.openai.com/api/reference/resources/images)。
- 当前官方 Schema 对流式 GPT Image 完成事件定义了 `input_tokens/input_tokens_details/output_tokens/total_tokens`，但非流式 `ImagesResponse.usage` 的文档描述未明确保证 GPT Image 2 必定返回。MVP 不为获取 Usage 改成流式，也不从输出大小反推 Token；非流式响应若真实携带 Usage，Adapter 只将官方字段规范化为诊断/对账观察，内部 Cost 仍使用冻结 Profile 的保守单 Call 上界。
- 官方说明 GPT Image 模型可能要求 API Organization Verification；这属于真实模型访问前置而不是连接凭据字段。Backend 必须把权限未开通与 API Key 无效映射成不同稳定错误，Web 显示明确修复动作，不能用假图通过验证。Backend 必须读取官方响应头；`x-request-id` 存在时进入去 Secret 的 Call/Receipt 诊断事实，请求未到达 OpenAI 而没有该 Header 时不伪造 ID。它不被当成可查询 Job ID 或幂等键：[OpenAI API 请求调试](https://developers.openai.com/api/reference/overview#debugging-requests)。
- 本设计固定必须实现 `provider_key=openai`、图片能力合同和外部模型标识，但不假设它与火山方舟具有相同的远端任务、轮询或幂等语义。实现时必须单独核验认证、输出、费用和 outcome-unknown 恢复合同并通过真实凭据验收。

### 1.3 Google Nano Banana

- “Nano Banana”是 Gemini 原生图片生成能力的产品名，不是可长期持久化的唯一技术 Model ID。当前官方明确定义四个稳定模型：Nano Banana 2 Lite `gemini-3.1-flash-lite-image`、Nano Banana 2 `gemini-3.1-flash-image`、Nano Banana Pro `gemini-3-pro-image` 与 Legacy Nano Banana `gemini-2.5-flash-image`。四个都是本轮真实接入范围；Legacy 虽然已被官方建议迁移，但不得用展示名、一个通配 Profile 或“后续再做”代替精确接入。旧的 `gemini-3.1-flash-image-preview` 与 `gemini-3-pro-image-preview` 已被官方废弃并下线，不进入 Preset 或兼容分支：[Gemini 图片生成](https://ai.google.dev/gemini-api/docs/image-generation) · [模型列表](https://ai.google.dev/gemini-api/docs/models) · [发布说明](https://ai.google.dev/gemini-api/docs/changelog)。
- Google 官方当前已把 `gemini-2.5-flash-image` 的关闭日标为 2026-10-02：[Gemini 废弃时间表](https://ai.google.dev/gemini-api/docs/deprecations)。在当前日期它仍是用户明确要求的必接项，所以必须实现并在官方可调用期内真实验收；不提前删除，也不静默映射到 2 Lite。如果官方在验收前关闭，必须如实报告外部阻塞，只能由新的已接受 Design 改变范围。
- Google 自 2026 年 6 月起把 Interactions API 作为默认新接口，并把 `generateContent` 标记为 Legacy，但当前官方 Interactions 支持表明确列出 `gemini-3.1-flash-image` 与 `gemini-3-pro-image`，未列出 `gemini-3.1-flash-lite-image` 与 `gemini-2.5-flash-image`；官方 Generate Content 图片指南仍明确覆盖 Lite 与 Legacy：[Interactions 支持模型](https://ai.google.dev/gemini-api/docs/interactions-overview) · [Generate Content 图片生成](https://ai.google.dev/gemini-api/docs/generate-content/image-generation)。因此 MVP 由每个 ModelPreset 冻结精确 Transport Contract：Nano Banana 2/Pro 使用 Interactions，2 Lite/Legacy 使用官方仍支持的 Generate Content；不运行时试探、切换或回退接口。
- Interactions 合同是 `POST https://generativelanguage.googleapis.com/v1beta/interactions`、`x-goog-api-key`、`response_format(type=image, mime_type, aspect_ratio, image_size)`，响应包含 `id/status/model/steps/usage`，图片字节位于 `steps[].content[]` 的 Base64 image block；一次性生成固定 `store=false`，不使用 `previous_interaction_id`。Generate Content 合同是 `POST /v1/models/{model}:generateContent`，固定只要求 `IMAGE` 输出，从单一 Candidate 的 `inlineData` 读取图片；两者都不使用搜索工具、对话状态或 Batch API：[Interactions API](https://ai.google.dev/api/interactions-api-v1) · [Generate Content API](https://ai.google.dev/api/generate-content)。
- 本设计固定必须实现 `provider_key=google_gemini` 并持久化官方 external model id；Web 可显示 Nano Banana 品牌名，但 Binding、Hash、费用和重放必须绑定精确 ModelProfileVersion，不能只存“Nano Banana”。

### 1.4 CC-Switch 公开实现参考

CC-Switch 是本地桌面 Provider 切换工具，不是 Lanverse 的多租户媒体 Backend，因此这里只参考其可验证的产品交互，不复制其存储和运行架构。以下 GitHub 事实固定核验于 `3217f72596f2d1c0f879f0a05f83803825d9809f`（2026-08-28），避免 `main` 后续变化改写本次设计依据：

- 它把 Provider 预设维护为代码内 Descriptor，包含固定 `settingsConfig`、端点候选、展示信息和可选 `templateValues`；用户选择预设后只需填写 API Key 或 `ENDPOINT_ID` 等占位字段：[Provider 预设](https://github.com/farion1231/cc-switch/blob/3217f72596f2d1c0f879f0a05f83803825d9809f/src/config/claudeProviderPresets.ts) · [模板字段状态与必填校验](https://github.com/farion1231/cc-switch/blob/3217f72596f2d1c0f879f0a05f83803825d9809f/src/components/providers/forms/hooks/useTemplateValues.ts)。
- 它的 Universal Provider 预设把默认应用、默认模型和 Provider 类型实例化成一条独立 Provider 配置，页面再保存这条配置：[Universal Provider 预设与实例化](https://github.com/farion1231/cc-switch/blob/3217f72596f2d1c0f879f0a05f83803825d9809f/src/config/universalProviderPresets.ts) · [添加 Provider 手册](https://github.com/farion1231/cc-switch/blob/3217f72596f2d1c0f879f0a05f83803825d9809f/docs/user-manual/en/2-providers/2.1-add.md)。
- 它当前以本地 SQLite 作为配置事实源，并会把 Provider 配置投影到不同 CLI 的活动配置文件；Universal Provider 记录还把 `apiKey` 与通用配置一起保存：[配置文件说明](https://github.com/farion1231/cc-switch/blob/3217f72596f2d1c0f879f0a05f83803825d9809f/docs/user-manual/en/5-faq/5.1-config-files.md) · [Universal Provider DAO](https://github.com/farion1231/cc-switch/blob/3217f72596f2d1c0f879f0a05f83803825d9809f/src-tauri/src/database/dao/universal_providers.rs)。这些做法适合本地切换器，但不满足 Lanverse 的 Backend 唯一 Writer、PostgreSQL/GORM 唯一事实源、密钥不可回显和项目级精确绑定约束。

因此 Lanverse 只采用三个交互原则：预设卡片降低录入成本、字段 Schema 明确必填与可编辑范围、连接创建后可显式验证。不会引入 SQLite、Migration、配置文件回填、全局 `is_current`、明文 API Key、任意反向代理、自动 Failover 或多份活动配置。

### 1.5 设计推断

这些 Provider 的同步/异步、输出载体、模型参数、远端身份和恢复能力不同。统一层只能统一 Lanverse 业务语义，不能伪造 Provider 不具备的能力。尤其是：

- 有远端任务查询能力的 Adapter 可以按同一 remote job id 恢复；
- 只有同步响应且没有官方幂等/查询合同的 Adapter，提交连接中断后只能进入 `OUTCOME_UNKNOWN` 并人工对账，不能自动重提；
- “客户端 Request ID”不自动等于 Provider 幂等键，只有官方明确承诺时才能用于自动恢复；
- 不得为了接口整齐，把视频异步任务伪装成同步图片响应，或把同步图片请求伪造一个可查询的远端 Job。

## 2. 当前事实与目标缺口

| 能力 | 当前代码事实 | 新目标缺口 |
|---|---|---|
| Generation Target/Cost/Quota | `reference_asset`、`shot_frame`、Intent、Reservation、Execution Claim 已存在；当前 PriceQuote/Quota/Preparation 只允许 `generation.image`，且 Provider Receipt 把 `actual_units` 等同输出数量 | 保留严格业务 Target；PriceQuote 绑定 exact ModelProfile，按图片/视频远程 Call 的保守预算上界保护，并把内部结算单位、Provider Usage 观察与 output count 分离 |
| Provider 事实 | Project Binding、Request、Job、Receipt 与 unknown 状态已存在 | Binding 从固定进程配置改为精确 Connection/Model/Credential Version |
| 生产 Adapter | 已有固定 Runware REST Adapter | 由 Seedream 5.0 Pro+、Seedance 2.0+、GPT Image 2、Nano Banana 四类真实 Adapter 取代，不保留 Runware 运行兼容 |
| Secret | Runware API Key 来自环境变量 | Provider 凭据经 Web 写入、Backend 加密、PostgreSQL 保存密文版本 |
| Web | 只有固定 Provider Binding 发布命令，无管理 UI | 增加 Workspace Provider 设置、模型 Profile 与 Project Binding UI |
| 图片输出 | 私有 Staging、Artifact、Candidate/QC 已存在 | 适配火山输出并完成真实 Seedream 旅程 |
| 视频输出 | Media 已支持 `video`、时长、Codec、Container；Storyboard Shot 已有动作/机位/表演/连续性与 `500–15000ms` 时长意图，但 Generation/Artifact/Candidate/StoryGraph 尚无视频 strict union | 本设计直接加入最小 `shot_video` Target、视频 QC、`ShotVideoBindingVersion` 与 Shot Workflow，不把 Seedance 留给未来设计 |

当前 Runware Model、Credential Ref、API Route 和 Composition Root 都不是新目标的兼容接口。实施时直接替换目标事实与调用路径，不双写、不保留 deprecated endpoint、不用 nullable 字段过渡；历史只由 Git 追溯。

### 2.1 当前代码的保留、重构与替换边界

实施前按实际源码固定以下边界，避免把成熟 Generation 闭环误删，也避免用旧 Runware 类型包一层“通用接口”形成兼容债务：

| 当前实现 | 处理 | 新事实 |
|---|---|---|
| `generation/domain/provider.go` 中 Request、Job、ResultReceipt 及同值判定 | 保留业务不变量并重构字段/状态 | Request 冻结精确 Connection/Profile/Credential/Binding Version；Job/Receipt 继续承载幂等、远端身份、`OUTCOME_UNKNOWN` 与终态 |
| `generation/application/provider_service.go` 的 Request/Job 创建、Submit/Reconcile、Cost/Quota 结算和 Receipt 收敛 | 保留执行状态机，拆除固定 Provider 配置分支 | Adapter 由 Registry 按冻结版本构造；配置缺失在 Cost/Quota 和远端调用前失败；同步与异步恢复按 capability 分流 |
| `generation/adapter/gormdb/provider.go` 的事务、行锁、immutable revision、Receipt/Job 幂等 | 保留模式并改到新 Model | 新配置 Owner 和执行 Owner 共用现有 GORM Transaction/Clause 工具链，不增加 Repository 框架、Raw SQL 或第二 Writer |
| `GenerationProviderBindingVersion` / Domain `ProviderBinding` | 直接由 `ProjectProviderBindingVersion` 取代 | Binding 指向精确 ConnectionVersion、CredentialVersion、ModelProfileVersion 和 Purpose；不保留旧表双写、读取回退或转换脚本 |
| `GenerationRequest` 的 `CredentialRef`、`ProviderKey`、`ModelKey` 快照 | 原位重构 | 改为新版本 ID 与运行所需非敏感快照 Hash；Request/Job/日志不保存解密 Secret |
| `workflow/adapter/generation/executor.go` 的 Target 校验、Preparation、Claim、Submit/Reconcile、Materialization | 保留 Workflow 编排职责并替换 Binding Port | Executor 只请求 Project/Purpose 的精确可执行 Binding，不再校验进程级 `runware` 常量 |
| `generation/adapter/runware/gateway.go` | 删除 | 分别实现 `volcengine_ark`、`openai`、`google_gemini` Adapter；不保留 Runware Factory、协议 DTO 或行为回退 |
| `generation/adapter/runware/stager.go` | 删除 Runware 包，保留已验证的安全边界 | 提取最小 Provider-neutral Staging Port，支持受限官方 URL 和 Adapter 已解码 bytes 两种输入；URL Host/redirect/media/size/pixel 规则由已编译 Adapter Descriptor 限定，绝不接受用户任意 URL |
| `config.Config` 的 `ImageProvider`、`RunwareAPIKey`、`RunwareRequestTimeout` 及 `IMAGE_PROVIDER`/`RUNWARE_*` | 删除 | API Key 只经 Web Command 写入密文 CredentialVersion；进程只从固定 Docker Secret 文件读取主密钥，HTTP Deadline 使用代码内受审默认值 |
| `bootstrap/api_process.go`、`bootstrap/workflow_process.go` 的 Runware 条件装配 | 直接替换 | 单一 Backend `main` 的 Composition Root 始终装配空安全 Registry、Preset Catalog、Secret Store 和真实 Adapter Factory；零连接不阻止服务启动 |
| `POST /projects/{project_id}/generation/image-provider-bindings` 与 Runware 常量 OpenAPI | 删除并替换，不提供 deprecated Route | 使用第 6 节 Workspace Connection/Credential/Profile 和 Project Binding API；响应永不返回 Secret/Credential ciphertext |
| `backend/tests/generation/runware_*`、固定 Binding/Config/OpenAPI/Workflow 测试 | 删除或重写 | 测试仍只放 `backend/tests`；新增 Catalog/Secret/Owner/Registry/各 Provider 合同/Staging/恢复/真实凭据测试，不在业务目录放测试文件 |
| Target、PriceQuote、Quota、Intent、ExecutionClaim、Artifact、CandidateSet、QC、Selection、Shot Image Binding | 保留 Owner 边界，按媒体 strict union 扩展 | 三类图片 Provider 与 Seedance 复用同一 Candidate/Selection 骨架；重构 exact Profile 的保守预算单位，新增视频元数据/QC 与 `ShotVideoBindingVersion`，不另建旁路 Candidate 系统 |

Provider-neutral Staging 不是预建平台抽象：Seedream、GPT Image 2、Nano Banana 与 Seedance 都要把短期 URL 或响应 bytes 安全写入同一个私有对象存储，已经存在多个真实消费者。它只统一下载/解码后的安全校验和对象写入，不统一 Provider 请求 DTO，也不允许任意代理或任意 URL 下载。

## 3. 范围与非目标

本文包含：

- Backend 内部通用 Media Provider Domain/Application/Adapter 边界；
- 可空 Provider Registry 与显式 Project Binding；
- Web 管理 Workspace Provider Connection、Credential Rotation、Model Profile 与 Project Binding；
- Backend 内置且版本化的 Provider/Model Preset Catalog，以及从预设实例化持久化连接的命令；
- Provider Secret 的加密持久化与非环境变量配置；
- 火山引擎方舟 Seedream 5.0 Pro+ 图片和 Seedance 2.0+ 视频 Adapter；
- GPT Image 2 与 Google Nano Banana 的真实图片 Adapter；
- 同步/异步 Provider Job 归一化、Cost/Quota、Staging、Artifact、Temporal 恢复和故障语义；
- 单一 Backend Writer、唯一 PostgreSQL/GORM Catalog 与真实凭据化验收。

本文不包含：

- 浏览器直连 Provider、在浏览器保存 API Key 或让 Agent 获取 Provider 凭据；
- 任意 Base URL、任意 Header、任意 JSON Template、脚本或动态代码插件；
- 允许 Workspace 上传、覆盖或持久化自定义 Preset 定义；MVP Preset 只能随已审核 Backend 代码发布；
- 自动跨 Provider fallback、按价格自动路由、负载均衡、Provider 市场或多云调度平台；
- 把模型目录实时镜像成第二事实源；
- Premiere 级时间线、镜头拼接、转场、配音、字幕、合成、成片导出或通用视频编辑平台；
- Migration、Raw SQL、第二 ORM、第二数据库 Writer、额外微服务或额外 Backend 入口；
- 通过 `.env`、Frontend Local Storage、日志、Kafka 或 Temporal History 保存 Provider API Key。

## 4. 边界与设计模式

只使用能直接解决当前差异的模式，不建立通用框架层。

### 4.1 Adapter Pattern

每个 Provider Adapter 把官方协议映射成统一的 Application Port：

```text
MediaProviderAdapter
├── DescribeCapabilities()
├── Submit(FrozenProviderSubmission)
├── Query(FrozenProviderJob)       // 仅 capability 支持时可用
└── Cancel(FrozenProviderJob)      // 仅官方合同与 Requirement 已接受时可用
```

Application 不导入火山、OpenAI、Google SDK/DTO；Adapter 不创建 Owner、Cost、Quota、Artifact 或 Workflow 事实。

### 4.2 Strategy Pattern

按精确 `provider_key + modality + adapter_contract_version` 选择执行策略：

- `volcengine_ark + image` → Seedream 5.0 Pro+ Strategy；
- `volcengine_ark + video` → Seedance 2.0+ Strategy；
- `openai + image` → GPT Image 2 Strategy；
- `google_gemini + image` → Nano Banana Strategy。

策略只决定官方请求/响应映射，不决定项目使用哪个模型。项目选择由不可变 Binding 决定，禁止运行时静默 fallback。

### 4.3 Registry + Factory

Composition Root 注册已编译且通过测试的 Adapter Factory。Registry 是进程内只读代码表，不是数据库 Provider 插件市场：

```text
provider_key → descriptor + adapter factory + supported contract versions
```

Web 只能选择 Registry 中存在的 Provider。Factory 在一次远程调用前接收解密后的短生命周期 Credential、ConnectionVersion 与 ModelProfileVersion，构造对应 Client；不得在全局单例长期保存明文 Secret。

### 4.4 Preset Catalog + Snapshot

Preset Catalog 与 Adapter Registry 一起由 Backend 编译发布，但职责不同：Registry 决定“代码能否执行”，Preset 决定“Web 如何安全创建配置”。Preset 不是数据库中的可变 Provider 事实，也不是远端动态目录。

创建连接/Profile 时，Application 先按 `preset_key + preset_version` 找到精确 Descriptor，拒绝未知字段，合并固定值与允许覆盖值，再把已解析非敏感快照及其 Hash 写入不可变 Version。运行时读取已解析快照，不重新套用最新版 Preset；因此更新展示名、默认模型或字段 Schema 不会漂移既有 Job，也不需要迁移旧行。

### 4.5 State Machine

Provider Job 是一个 GenerationTarget 的本地聚合；Provider Call 才是一次真实付费远端调用和恢复边界：

```text
ProviderCall: PENDING ── local preflight failed ──→ FAILED
                    └─→ DISPATCHING → SUBMITTED/RUNNING
                                      ├── SUCCEEDED
                                      ├── FAILED
                                      └── OUTCOME_UNKNOWN

ProviderJob aggregate:
  all calls succeeded                   → SUCCEEDED
  some succeeded + rest explicit failed → PARTIAL_SUCCEEDED
  all calls explicit failed             → FAILED
  any call unresolved                    → RUNNING | OUTCOME_UNKNOWN
```

同步图片 Call 可以从 `DISPATCHING` 直接到 `SUCCEEDED|FAILED|OUTCOME_UNKNOWN`；异步视频 Call 在持久化远端 task id 后进入 `SUBMITTED|RUNNING`。`DISPATCHING` 不是可超时抢占的普通锁，而是“本地已提交唯一发送权，远端是否收到尚未落库”的持久事实。重投或并发调用看到它时只能返回未解决状态，不得再次 `Submit`；只有已持久化 remote task id 的 `SUBMITTED|RUNNING` 才能 `Query`。状态转换和 Job 聚合由 Domain/Application 校验，Adapter 只能返回单 Call 观察结果。任何 `DISPATCHING|OUTCOME_UNKNOWN` 未收敛前不得派发后续 Call 或开放 Human Gate。

### 4.6 Decorator

Provider Client 外层只使用有限 Decorator 统一 Deadline、脱敏日志、Metrics 和 Trace；它们不得修改 Payload、自动重试可能已送达的 Submit、吞掉 Provider Error 或读取业务数据库。Query 的退避由 Temporal Workflow/Activity Policy 驱动，不在 HTTP Transport 中隐藏循环。

## 5. 唯一数据事实

全部事实进入现有 Backend PostgreSQL/GORM Catalog，使用 GORM Model、Transaction、Clause 和 Tag 空库同步，不创建 Migration/DDL/Raw SQL。每个配置 Command 都必须在一个共享 GORM 事务中完成当前 Membership/Owner 授权、latest revision 锁定、不可变版本追加、Command Receipt 和必要 Outbox；相同幂等键重放返回相同版本，输入漂移或并发写入不产生半配置。

### 5.1 内置 Preset Catalog（不是第二事实源）

Preset 是 Backend 代码中的非敏感创建模板，不建 SQL 表、不通过配置文件或 `.env` 注入，也不从第三方动态下载：

```text
MediaProviderPresetDescriptor
├── preset_key / preset_version / provider_key
├── display_name / description / provider_home_url
├── adapter_contract_version
├── fixed_connection_config
├── editable_fields[] (type / required / validation / secret=false)
├── credential_fields[] (type / required / write_only=true，只含 Schema)
└── model_presets[]

MediaModelPresetDescriptor
├── model_preset_key / preset_version
├── provider_key / family / modality / external_model_id
├── adapter_transport_contract / capability_schema_version / fixed_defaults
├── billing_metric = generation.image.call | generation.video.call
└── editable_overrides[]
```

目标 Catalog 必须交付以下最小预设；Catalog 只返回已经注册真实 Adapter Factory 并通过合同测试的预设，因此返回即代表可配置、可绑定、可执行，不提供 `planned`、`coming_soon` 或只有展示用途的假可用状态：

| Preset Key | 类型 | 固定事实/默认值 | 完成门 |
|---|---|---|---|
| `volcengine.ark-cn-beijing` | Connection | `provider_key=volcengine_ark`、固定方舟北京 Region/Host、Bearer API Key | `/ping`、Secret 与真实生成均通过 |
| `openai.official-api` | Connection | `provider_key=openai`、固定 OpenAI 官方 Host、Bearer API Key | 认证、Secret 与真实生成均通过 |
| `google.gemini-api` | Connection | `provider_key=google_gemini`、固定 Gemini 官方 Host、API Key | 认证、Secret 与真实生成均通过 |
| `volcengine.seedream-5-0-pro-260628` | Image Model | `family=seedream`、`external_model_id=doubao-seedream-5-0-pro-260628` | Seedream 5.0 Pro 合同与真实生成通过 |
| `volcengine.seedance-2-0-260128` | Video Model | `family=seedance`、`external_model_id=doubao-seedance-2-0-260128` | Seedance 2.0 合同、异步恢复与真实视频通过 |
| `volcengine.seedance-2-0-fast-260128` | Video Model | `family=seedance`、`external_model_id=doubao-seedance-2-0-fast-260128` | Seedance 2.0 Fast 精确参数合同与真实视频通过 |
| `volcengine.seedance-2-0-mini` | Video Model | `family=seedance`、Owner 必填当前火山账号中的官方 Model ID/Endpoint ID | Seedance 2.0 Mini 精确参数合同、异步恢复与真实视频通过 |
| `volcengine.seedance-2-5` | Video Model | `family=seedance`、Owner 必填当前火山账号中的官方 Model ID/Endpoint ID | Seedance 2.5 精确参数合同、异步恢复与真实视频通过 |
| `openai.gpt-image-2` | Image Model | `family=gpt_image`、`external_model_id=gpt-image-2`、`transport=openai-image-api-v1-nonstreaming` | GPT Image 2 合同与真实生成通过 |
| `google.nano-banana-2-lite` | Image Model | `family=gemini_image`、`external_model_id=gemini-3.1-flash-lite-image`、`transport=generate-content-v1-image-v1` | Nano Banana 2 Lite 合同与真实生成通过 |
| `google.nano-banana-2` | Image Model | `family=gemini_image`、`external_model_id=gemini-3.1-flash-image`、`transport=interactions-v1beta-image-v1` | Nano Banana 2 合同与真实生成通过 |
| `google.nano-banana-pro` | Image Model | `family=gemini_image`、`external_model_id=gemini-3-pro-image`、`transport=interactions-v1beta-image-v1` | Nano Banana Pro 合同与真实生成通过 |
| `google.nano-banana-legacy` | Image Model | `family=gemini_image`、`external_model_id=gemini-2.5-flash-image`、`transport=generate-content-v1-image-v1` | Legacy Nano Banana 合同与真实生成通过 |

为了支持火山控制台自建 Endpoint，已连接的 `volcengine_ark` 可以选择“受支持模型 ID/Endpoint ID”表单，而不是任意 Provider 预设。用户仍只能选择 Adapter 已实现的 `family + modality + capability_schema_version`，并录入受长度/字符集约束的官方 ID；Base URL、认证方式和 Capability 不能自定义。Seedream 5.0 Pro+/Seedance 2.0+ 出现新官方版本时，必须先增加精确 Preset、参数合同、价格事实和真实凭据测试，不能仅按字符串版本号自动放行未知模型。

### 5.2 ProviderConnectionVersion

Workspace 级连接的不可变非敏感版本：

```text
ProviderConnectionVersion
├── id / workspace_id / connection_key / revision
├── source_preset_key / source_preset_version / preset_snapshot_hash
├── provider_key = volcengine_ark | openai | google_gemini
├── display_name
├── credential_version_id
├── resolved_config JSONB (provider-specific strict union，不含 Secret)
├── state = enabled | disabled
├── adapter_contract_version
├── content_hash
└── created_by / created_at
```

`connection_key` 是用户可理解的一条持久连接谱系，Revision 是该谱系的不可变版本；二者共同替代 CC-Switch 的全局“当前 Provider”。`source_preset_*` 记录来源，`resolved_config` 与 `preset_snapshot_hash` 保证即使代码中的 Preset 升级，旧连接仍按创建时事实执行。首版火山配置只允许 `region` 等受控字段；生产 Base URL 由 Adapter 根据 Region allowlist 决定，不允许用户输入 URL、Header 或签名模板。更新、禁用和重新启用都追加新 Revision，不覆盖旧 Job 冻结版本。

### 5.3 ProviderCredentialVersion

Workspace 级不可变 Secret 密文：

```text
ProviderCredentialVersion
├── id / workspace_id / connection_key / revision
├── provider_key
├── cipher_suite = aes-256-gcm
├── key_id
├── nonce / ciphertext
├── secret_fingerprint
└── created_by / created_at
```

API Key 通过 TLS 从 Web 只写传入，Backend 立即使用标准 AEAD 加密。Associated Data 必须绑定 workspace、provider、credential id、revision 与 key id，防止跨租户或跨版本搬移密文。`secret_fingerprint` 使用由根密钥派生的 HMAC-SHA-256，而不是直接对可能低熵的 API Key 做裸 Hash；它只用于判断用户是否录入了同一凭据，不作为解密或认证材料。数据库不保存明文；Query/API 永不返回 nonce/ciphertext/明文，只返回 provider、版本、创建时间和截断 fingerprint 摘要。

Provider 根密钥不放 `.env`。本地与生产都从固定只读 Docker Secret 文件 `/run/secrets/lanverse_media_provider_master_key` 读取。根密钥不是业务事实，不进入 PostgreSQL、日志、镜像或仓库。Secret 文件不存在时服务仍可空配置启动，但创建/轮换/执行 Provider 返回 `secret_store_unavailable`；不得生成临时根密钥导致重启后无法解密。

根密钥轮换不是 MVP；Credential API Key 轮换通过新 CredentialVersion + ConnectionVersion 完成。丢失根密钥时只能由 Owner 重新录入 Provider 凭据，不允许明文恢复后门。

### 5.4 ProviderModelProfileVersion

Workspace 可配置的不可变模型 Profile：

```text
ProviderModelProfileVersion
├── id / workspace_id / profile_key / revision
├── creation_source = preset(key/version/snapshot_hash) | supported_external_id
├── connection_key / provider_key
├── external_model_id
├── modality = image | video
├── family = seedream | seedance | gpt_image | gemini_image
├── adapter_transport_contract / capability_schema_version
├── billing_metric = generation.image.call | generation.video.call
├── defaults JSONB (strict family union)
├── state = enabled | disabled
├── content_hash
└── created_by / created_at
```

用户优先从模型预设创建 Profile，也可以在已支持的 Provider/Family 表单中录入官方 Model ID 或火山 Endpoint ID，但不能自报任意 capability。`creation_source` 是严格 union，避免用一组 nullable 的兼容字段表达两种来源。Profile 绑定稳定 `connection_key` 而不是某一 Connection Revision，所以同一连接轮换凭据时无需复制模型 Profile；Project Binding 发布时再校验同一连接谱系并冻结 exact Connection/Credential/Profile Version。创建命令只允许覆盖 Preset 声明为 editable 的字段，Backend 再根据 Provider/Family 的版本化 Capability Schema 校验参数；未知字段、模型与 family 不匹配、越界尺寸/时长、危险 URL 或 Adapter 未实现都失败关闭。

`adapter_transport_contract` 由 Backend Registry 根据精确 Preset/已支持 Model ID 派生，不是 Web 可编辑字段；同一 external model id 不得同时发布多个可运行 Transport 来试探。Google MVP 只接受 Catalog 中四个精确 Nano Banana Preset，不接受 Owner 填写其他 Gemini ID；火山自建 Endpoint 仍必须先匹配已实现 family/capability/transport 合同。

品牌显示名不进入执行身份。执行、Hash、费用和审计冻结 `external_model_id + profile revision + capability schema version + defaults hash`。

### 5.5 ProjectProviderBindingVersion

现有 Provider Binding 泛化为按 Project/Purpose 的不可变选择：

```text
ProjectProviderBindingVersion
├── id / workspace_id / project_id / purpose / revision
├── connection_version_id
├── credential_version_id
├── model_profile_version_id
├── provider_key / modality / adapter_contract_version
├── content_hash
└── created_by / created_at
```

`purpose` 固定为 `reference_asset|shot_frame|shot_video`：前两者只允许 `modality=image`，`shot_video` 只允许 `provider_key=volcengine_ark`、`family=seedance`、`modality=video`。Binding Command 必须按 Preset capability 失败关闭，不能仅凭 Web 字符串创建没有 Adapter 或 Owner 的组合。

每个 Provider Request/Job 冻结精确 Binding、Connection、Credential 和 ModelProfile Version。之后 Web 轮换密钥、禁用连接或修改模型只影响新 Job；既有 Job 的 Query/对账仍解析原 CredentialVersion。若 Owner 主动撤销旧凭据导致无法查询，Job 进入稳定人工对账状态，不改用新凭据猜测结果。

### 5.6 精确预算 PriceQuote、Quota 与结算单位

当前 `CostPriceQuote` 只按 `project_id + generation.image` 定价，不足以区分 Seedream、GPT Image 2、Nano Banana 与 Seedance。实施时直接重构现有 GORM Model/Application 合同，不新增兼容表或双写：

```text
CostPriceQuoteVersion
├── id / workspace_id / project_id / model_profile_version_id
├── billing_metric = generation.image.call | generation.video.call
├── reservation_unit_amount / currency / revision
├── content_hash
└── created_by / created_at
```

- `generation.image.call` 的 estimated units 是 Target 冻结的 `number_results`；MVP 一个 Call 只请求一张图，Profile 的固定尺寸/质量/输入上限已经进入快照，因此 Owner 配置的是该 exact Profile 每次远程 Call 的保守预算上界，而不是一个跨模型通用价格。
- `generation.video.call` 的 estimated units 在 MVP 固定为 `1`；视频 Profile 已冻结模型、输入类型、允许的时长/分辨率集合、音频与其他会影响费用的上限，GenerationTarget 再冻结本次取值。Owner 为该 exact Profile 配置覆盖全部允许取值的单次 Call 保守预算上界；任一上限扩大都必须产生新 ProfileVersion 和 PriceQuoteVersion，不在代码中预建公式引擎。
- Project 对每个 enabled ModelProfileVersion 必须有 exact PriceQuote 才可执行。PriceQuote 缺失在 Intent/Reservation 前返回 `price_quote_required`；轮换 Profile 必须配置新 Quote，旧 Job 继续使用旧 Quote Revision。
- Quota 继续是项目业务限额，使用相同的稳定 metric，但不绑定某一 Provider；图片按已授权远程 Call 数、视频按已授权视频 Call 数预留，两个 metric 不共享计数器。
- `GenerationIntent`、`GenerationRequest` 与 CostEstimate 冻结 exact BindingVersion、ModelProfileVersion、billing metric、estimated units 和 PriceQuoteRevision。`output_count` 只表示产物数量，不再参与 Cost 校验；Provider Receipt 也不使用 `actual_units` 冒充供应商账单。
- 所有 Call 都明确收敛后，Cost Reservation 只做一次终态聚合结算：图片和视频的 `settled_budget_units` 都是 `dispatch_boundary_entered_at` 非空的 Call 数；本地预校验失败且未越过该边界的 Call 不结算。该时间表示 Backend 已提交不可自动重提的发送权，不证明 HTTP bytes 已离开主机，因此是保守的内部预算边界，不是 Provider 发票边界。结算使用既有“释放全部预留 + 记录本次内部结算金额”的单一 Ledger 终态，不增加部分结算状态机。
- 任一 Call `OUTCOME_UNKNOWN` 时保留全部 Cost/Quota Reservation，不进入聚合结算。已授权 Job 终态时 Quota 按冻结 Target 一次 Consume；在第一次远程派发前失败才整体 Release。
- Adapter 只返回去 Secret 后的官方 Usage 观察和远程 Request/Task 身份。它们进入 Receipt/Hash 供对账，但 MVP Cost Ledger 是内部保守预算事实，不声称是 Provider 发票；GPT Image 2 这类不在同步图片响应中给出可完整核算金额的合同，按冻结预算上界结算，不伪造精确 Usage、不记零费用。

现有 Cost/Quota Owner、Reservation、Receipt、共享 GORM Transaction 和幂等模式继续复用；只扩展 metric/profile 维度与单位合同，不建设账单中心、动态汇率、自动抓价或 Provider 价格市场。

### 5.7 ProviderCall：真实远端调用边界

现有一条 Intent 只能对应一条 Request/Job/Receipt，并假设一次远端调用返回全部 Candidate。该假设对支持 `n` 的 GPT Image 2 可以成立，但 Nano Banana 当前官方 Interactions/Generate Content 图片合同都没有可依赖的 `n=4` 远端批量身份；把四次 HTTP 调用隐藏在一个 Adapter `Submit` 中会在中途断线时丢失付费边界。实施时直接把当前 Provider Job 重构为 Call 聚合：

```text
GenerationProviderCall
├── id / workspace_id / project_id / job_id
├── candidate_index / call_key / request_hash
├── requested_output_count = 1
├── status / local_failure_code? / remote_request_id? / remote_job_id?
├── dispatch_boundary_entered_at?
├── revision / content_hash / created_at / updated_at

GenerationProviderResultReceipt
├── id / call_id / provider_event_id?
├── status / output_count / failure_code?
├── provider_usage_observation? / provider_usage_hash
├── output? (成功时严格一个)
├── occurred_at / received_at / content_hash
```

- 图片 Target 的 `number_results=4` 确定性创建四个 Call，`candidate_index=1..4`；视频 `number_results=1` 只创建一个 Call。每个 Call 具有独立 Request Hash、状态、远端身份和 ResultReceipt；`job_id + candidate_index + request_hash` 唯一，不依赖 Temporal Activity Attempt 号创建新 Call。
- Application 先在 `PENDING` 完成冻结输入、Capability、Credential 解密与本地 Request 编译校验；此时失败可以直接记录 `FAILED + local_failure_code`，并保持 `dispatch_boundary_entered_at=NULL`，不伪造 Provider ResultReceipt。通过后，Application 在单个 GORM 事务内锁定 Call，仅允许 `PENDING → DISPATCHING`，同时写入 `dispatch_boundary_entered_at` 和新 Revision；事务成功提交的这一次调用路径才获得唯一的 `should_dispatch=true`，然后在事务外只执行一次 Provider `Submit`。
- 并发 Command、Temporal Activity 重投或 Worker 重启后再读取同一 Call，只要它不再是 `PENDING`，就不会再获得发送权。看到 `DISPATCHING` 时不能凭超时或新 Activity Attempt 重置为 `PENDING`，也不能在无 remote id 时 `Query`；负责该次发送的调用路径可以用预期 Revision 将观察结果收敛到终态或 `SUBMITTED|RUNNING`，若其结果丢失，Temporal 只调用 Backend 本地恢复 Command，由 Backend 把 Call 保守标记为 `OUTCOME_UNKNOWN`，不开启第二次 Submit。
- 每个 Call 最多只有一条终态 `GenerationProviderResultReceipt`，唯一性以 `call_id` 为主，不以远端可选字段为主。`provider_event_id` 只在官方响应真实返回时保存；同步失败响应没有稳定事件 ID 时仍可用 Call 身份、规范化失败码、时间和 Content Hash 形成 Receipt，不生成假远端 ID。同一 Call 的重复终态观察只能同值重放，不同值必须失败关闭并进入人工对账。
- `provider_usage_observation` 是 Adapter Contract Version 定义的有界、去 Secret、规范化结构，只保留官方响应中用于费用对账的字段；不保存原始 HTTP 响应、Base64 媒体、Prompt 或任意 JSON。无官方 Usage 时字段为空且 Hash 仍稳定，不伪造零用量。
- MVP 对所有图片 Provider 都固定一 Call 一输出，即使 GPT Image 2 支持 `n` 也先传 `n=1`；这样三类图片 Adapter 使用同一真实恢复边界，不引入批量优化分支。后续只有在新 Design 证明批量身份/部分结果语义后才能提高单 Call 输出数。
- Temporal Workflow 以冻结的 Call 数执行有界确定性循环；每个 Call 是独立 Activity，不在一个 Activity/HTTP Adapter 内隐藏四次远端请求。Call 成功后立即 Staging，下一 Call 只有在前一 Call 已明确成功或失败后才能派发。
- 全部成功时 Job `SUCCEEDED`；至少一个成功且其余都是明确失败时 Job `PARTIAL_SUCCEEDED`，CandidateSet 只包含成功且 QC Passed 的输出并显示缺失 Candidate/失败码；全部明确失败时 Job `FAILED`。任何 Call `OUTCOME_UNKNOWN` 时 Job 保持 `OUTCOME_UNKNOWN`，不派发后续 Call、不物化 CandidateSet、不自动重提。
- Cost/Quota 先按 Target 全量预留；只有 Job 全部 Call 明确收敛后才按第 5.6 节做一次聚合结算，不为每个 Call 发明部分结算生命周期。Job、Intent 与 CandidateSet 聚合所有 Call/Receipt Hash；重启、Activity 重投和局部重跑都不能创建第二个相同 `candidate_index + request_hash` Call。

这只增加一个真实必要的远端调用事实，不建设通用 Batch Scheduler。现有 GenerationRequest 保留 Target/Binding/Profile 的冻结快照，ProviderJob 保留 Target 级聚合；ResultReceipt 改为 Call 级唯一事实，输出物化按 Job 聚合读取。

## 6. Web 配置与 API

### 6.1 权限与页面

新增 Workspace 级 `/settings/providers`：

- Workspace Owner 可以创建连接、录入/轮换凭据、禁用连接、创建模型 Profile、为 exact Profile 配置 Project PriceQuote 并发布 Project Binding；
- Editor/Viewer 只能看到被授权项目的非敏感可用性摘要，不能看到 fingerprint 之外的 Secret 信息，也不能执行配置 Command；
- Secret 输入框只写且提交后立即清空，不回显、不进入 URL、浏览器持久化、Redux Cache 或分析事件；
- Browser 只调用 Backend `/api/v1`，不得直连火山、OpenAI 或 Google。

空状态必须是正常产品状态：显示“尚未配置媒体 Provider”，并允许非视觉业务继续运行；不能把整个 Backend/Frontend 标为 unavailable。

### 6.2 Backend HTTP Commands/Queries

目标 API 使用严格 JSON、大小限制、未知字段拒绝和当前 Token/Membership 重新授权：

```text
GET  /api/v1/media-provider-catalog

GET  /api/v1/workspaces/{workspace_id}/media-provider-connections
POST /api/v1/workspaces/{workspace_id}/media-provider-connections
POST /api/v1/workspaces/{workspace_id}/media-provider-connections/{connection_id}/credential-rotations
POST /api/v1/workspaces/{workspace_id}/media-provider-connections/{connection_id}/validations
POST /api/v1/workspaces/{workspace_id}/media-provider-connections/{connection_id}/disable

GET  /api/v1/workspaces/{workspace_id}/media-model-profiles
POST /api/v1/workspaces/{workspace_id}/media-model-profiles
POST /api/v1/workspaces/{workspace_id}/media-model-profiles/{profile_id}/disable

GET  /api/v1/projects/{project_id}/media-provider-bindings
POST /api/v1/projects/{project_id}/media-provider-bindings

GET  /api/v1/projects/{project_id}/media-model-profiles/{profile_version_id}/cost-price
POST /api/v1/projects/{project_id}/media-model-profiles/{profile_version_id}/cost-price
```

所有 Command 必须带 `idempotency_key`。创建连接 Request 使用 `preset_key + preset_version + display_name + values + credential.value`；`values` 只能包含 Descriptor 声明为 editable 的非敏感字段，`credential.value` 是一次性只写字段，Response 绝不包含它。创建 Model Profile 使用 `model_preset_key + preset_version + allowed_overrides`，或使用受支持 Family 的官方 Model/Endpoint ID 表单。Price Command 沿用 Cost Owner 的金额/币种校验，但路径绑定 exact ModelProfileVersion，billing metric/policy 由 Profile 决定而不接受用户覆盖。Binding 只引用已存在且 enabled 的 exact versions，不接受 API Key、Endpoint URL 或任意 Provider JSON。

`GET /media-provider-catalog` 只返回编译进当前 Backend、已注册真实 Factory 且通过合同测试的非敏感 Provider/Model Preset、字段 Schema 与 Capability Schema，用于 Web 渲染预设卡片和有类型表单，不从外网动态加载代码或 Schema。增量开发期间尚未完成的 Adapter 不进入 Catalog；2051 完成门要求 Seedream 5.0 Pro+、Seedance 2.0+、GPT Image 2 与 Nano Banana 全部进入 Catalog 并通过真实凭据验收。

Connection “验证”必须是显式 Command。火山连接首版只调用官方带 Bearer 认证的 `/ping` 检查认证与连通性，不调用 Seedream/Seedance；其他 Provider 只有在提供确定且不收费的认证/模型查询接口时才可远程验证，否则只能标记 `unverified`，不能通过发起一张付费测试图暗中验证。

### 6.3 不提供的 API

- 不提供 `/proxy`、`/invoke`、任意 URL 或任意 Header API；
- 不允许前端上传 Adapter Code、Request Template、JSON Schema 或脚本；
- 不允许一次请求“自动选择最便宜 Provider”；
- 不保留旧的固定 Runware Binding 发布 API 作为兼容入口。

## 7. Provider 能力合同

### 7.1 Canonical Submission

Application 向 Adapter 提供冻结输入：

```text
FrozenProviderSubmission
├── provider_request_id / provider_job_id / provider_call_id / candidate_index
├── workspace_id / project_id
├── target_id / target_hash / modality / purpose
├── binding_version_id
├── connection_version_id / credential_version_id
├── model_profile_version_id / external_model_id
├── billing_metric / estimated_billing_units / price_quote_revision
├── prompt_version / positive_prompt / negative_prompt
├── exact input Artifact refs + hashes
├── image constraints OR video constraints (strict union)
└── request_hash
```

Adapter 返回：

```text
ProviderObservation
├── state = accepted | running | succeeded | failed | outcome_unknown
├── provider_request_id / provider_call_id / candidate_index
├── remote_job_id? / remote_request_id?
├── provider_event_id? / observed_at
├── outputs[]
│   ├── output_key / temporary URL or bytes source
│   ├── media_type / bytes / sha256
│   └── image_metadata(width/height) OR video_metadata(width/height/duration_ms/codec/container)
├── provider_usage_observation? / provider_usage_hash (diagnostic only)
└── stable_error_code?
```

Cost Owner 的 PriceQuote/Reservation 仍是内部预算事实；Provider Usage 观察只能用于对账，不能覆盖原 Estimate 或绕过 Quota。

一次 `Submit/Query` 只处理一个 `GenerationProviderCall`，成功时必须恰好返回一个输出。Adapter 不得在内部循环生成多个 Candidate；Target 级 Job 聚合、下一 Call 派发和部分成功判断都由 Application/Temporal 明确完成。

Output metadata 是 strict union，不用一组默认零值把图片伪装成视频。Adapter 只报告远端观察；Backend Staging 重新从真实字节探测并校验 SHA-256 和媒体元数据后，才允许写 Provider ResultReceipt。图片继续使用标准库解码；视频复用成熟 FFmpeg `ffprobe` JSON 输出，通过 `exec.CommandContext` 直接传参、固定超时和资源上限解析，不手写 MP4/Codec Parser。当前本机 Homebrew 已有 `ffprobe`；Backend Runtime Image 必须显式安装固定发行版的 `ffmpeg` 包，并由镜像测试证明可执行，不能依赖宿主机二进制泄漏进容器。

### 7.2 Seedream 图片 Strategy

- 使用火山方舟 ImageGenerations Endpoint 与 Bearer API Key；
- 精确传入 ModelProfile 的 Model ID/Endpoint ID、冻结 Prompt、参考图、尺寸和输出格式；每个 ProviderCall 只请求一个独立 Candidate，不能把“组图内容”误当成四个可独立选择的 Candidate；
- 只启用 Adapter 已验证的 Seedream family/version 参数，未知参数不透传；
- `response_format=url|b64_json` 必须冻结进 ModelProfile/Request Hash；URL 当前只有 24 小时有效期，两种输出都必须在当前 Job 内完成字节校验和私有 Staging；
- 同步成功结果立即进入 Staging；明确 Provider Error → `failed`；连接中断且官方没有可查询身份时 → `outcome_unknown`，不得自动再次生成；
- Base64 输出先做编码长度、解码字节、媒体类型、尺寸与像素限制；URL 输出执行 Host/DNS/IP/重定向/大小/媒体校验；
- ProviderCall 成功时必须恰好得到一个输出；零个或多个顶层输出均不得物化成功 Call Receipt。

### 7.3 Seedance 视频 Strategy

- 使用火山方舟 Contents Generations Task API 创建任务；
- 创建成功后先持久化远端 Task ID，再结束 Activity；后续只按该 Task ID Query；
- 远端 Task ID 当前只保留 7 天；Temporal Query/故障恢复时限必须小于该外部保留窗口，不能把 Provider 历史当作永久恢复事实；
- 不使用公网 Callback/Webhook 作为 MVP 必需路径，Temporal Timer 驱动有界 Query；
- ModelProfile 严格冻结允许的比例、时长/帧数、是否生成音频、水印和输入 Artifact 类型，GenerationTarget 冻结本次精确取值；版本不支持的组合在提交前拒绝；
- 成功视频必须立即安全下载到私有 Staging，并完整校验字节上限、媒体类型、容器/基础时长元数据与 SHA-256；Provider URL 不进入正式 Owner Binding；
- Submit 在收到远端 Task ID 前连接中断时保持 `outcome_unknown`，只有官方提供可证明的幂等/检索身份后才能自动恢复，不能凭本地 Job ID 猜测远端已接受。

### 7.4 GPT Image 2 Strategy

- `provider_key=openai`；family=`gpt_image`；modality=`image`；
- external model id 使用官方模型或 dated snapshot，例如 `gpt-image-2` / `gpt-image-2-2026-04-21`；
- ModelPreset/Profile 固定 `adapter_transport_contract=openai-image-api-v1-nonstreaming`；只连接 `https://api.openai.com`，使用 Bearer API Key；文生图调用 `POST /v1/images/generations`，带参考 Artifact 的图片编辑调用 `POST /v1/images/edits`，不把“OpenAI-compatible”任意 Base URL 纳入本 Adapter；
- 每个 ProviderCall 固定 `n=1`，冻结 `size/quality/output_format`；成功只接受恰好一个 `data[].b64_json` 并立即严格 Base64 解码/Staging。官方支持 `n>1` 不代表本 MVP 可以绕过 Call 级付费与恢复事实；
- Adapter 必须实现图片生成/编辑输入、参考 Artifact、输出字节、费用对账观察与稳定失败映射；若非流式响应真实携带官方 Usage 则只规范化保存，否则留空且不伪造。Cost 始终按 exact ModelProfileVersion 的 PriceQuote 保守 Call 上界结算，不把 Token 观察写成供应商确定账单；
- Adapter 必须读取响应头，`x-request-id` 有值时写入 `remote_request_id`，无值时保持为空且不伪造；它只用于诊断与人工对账。官方没有为本 Image API 合同承诺按该值查询结果或自动去重，因此不得据此恢复或自动重提；
- 同步提交连接中断且没有官方可查询身份时进入 `outcome_unknown`，不得换模型或自动重提；
- 完成门要求 Registry 注册真实 Factory、Catalog 暴露预设并完成真实 GPT Image 2 → Staging → CandidateSet → Owner Apply 旅程。

### 7.5 Nano Banana Strategy

- `provider_key=google_gemini`；family=`gemini_image`；modality=`image`；
- external model id 使用官方技术 ID：Nano Banana 2 Lite 对应 `gemini-3.1-flash-lite-image`，Nano Banana 2 对应 `gemini-3.1-flash-image`，Nano Banana Pro 对应 `gemini-3-pro-image`，Legacy Nano Banana 对应 `gemini-2.5-flash-image`，不使用品牌名作为执行 ID；
- 只连接 `https://generativelanguage.googleapis.com`，使用 `x-goog-api-key`；ModelPreset/Profile 必须冻结 `adapter_transport_contract`，Nano Banana 2/Pro 固定 `interactions-v1beta-image-v1`，2 Lite/Legacy 固定 `generate-content-v1-image-v1`。Adapter Registry 只按该值选择已实现 Strategy，不在失败后改用另一接口；
- Interactions Strategy 每个 ProviderCall 只请求一个 `response_format(type=image)`，冻结 `mime_type/aspect_ratio/image_size`并固定 `store=false`；成功必须校验 `status=completed`、精确 Model ID、Interaction ID、Usage，并从 `steps[].content[]` 只提取一个 Base64 image block，不使用 `previous_interaction_id`；
- Generate Content Strategy 每个 ProviderCall 固定只请求 `IMAGE` 输出，按精确模型能力冻结 aspect ratio/image size；成功只接受一个正常 finish reason 的 Candidate 和一个 `inlineData` 图片部件，并保留规范化 `usageMetadata`。响应有 `modelVersion` 时必须与冻结 Profile 一致；
- 两种 Strategy 遇到 incomplete、多图、无图、文本/其他不可接受输出、Model 漂移或超出能力的参数均失败关闭；都不使用搜索工具、对话状态或 Batch API；
- Adapter 必须处理文本/多图片输入、内联图片输出、模型特有配置、Usage/费用对账与内容安全错误；输入 Artifact 以 Backend 读取后的受限 Base64 image block 发送，不把 MinIO URL 暴露给 Google；
- 同步提交连接中断且没有官方可查询身份时进入 `outcome_unknown`，不得换模型或自动重提；
- 完成门要求 Registry 注册真实 Factory、Catalog 暴露四个 Nano Banana 精确预设，并分别完成真实生成 → Staging → CandidateSet → Owner Apply 旅程。

## 8. GenerationTarget 与 Workflow

`reference_asset|shot_frame|shot_video` 是严格业务 Target union，不改成通用 Provider Payload：

```text
reference_asset
  SpecificationVersion + AssetState + EffectiveStyleSnapshot
  → image CandidateSet → Human Selection → AssetVersion

shot_frame
  formal Shot + ShotProductionBindingVersion + exact AssetVersion refs
  → image CandidateSet → Human Selection → ShotImageBindingVersion

shot_video
  formal Shot + ShotProductionBindingVersion
  + exact ShotImageBindingVersion + exact source image Artifact
  → video CandidateSet → Human Selection → ShotVideoBindingVersion
```

### 8.1 `shot_video` strict Target

本轮直接交付最小图生视频闭环，不再等待另一个未来 Design。`ShotVideoTarget` 固定包含：

```text
ShotVideoTarget
├── shot_ref(id/revision/content_hash)
├── shot_production_binding_version_ref(id/revision/content_hash)
├── source_shot_image_binding_version_ref(id/revision/content_hash)
├── source_image_artifact_ref(id/revision/content_hash/sha256)
├── prompt_version = shot-video-motion-v1
├── motion_prompt
├── target_duration_ms
├── aspect_ratio
├── generate_audio = false
├── number_results = 1
└── output_format = MP4
```

- Target Builder 只接受 active formal Shot、当前精确 `ShotProductionBindingVersion`、已经人工 Apply 的 `ShotImageBindingVersion` 及其 READY 图片 Artifact；Workspace/Project/Episode/Shot、Revision、Hash 任一漂移均失败关闭。
- `motion_prompt` 由已批准 Shot Intent 的 Purpose、Camera、Action、Performance、Continuity 与选中首帧确定性生成并冻结，不让 Provider 重写 Shot 语义；`target_duration_ms` 必须等于正式 Shot 的批准时长且保持 `500–15000ms`。
- Target Builder 在创建 Target 前已经解析 exact `shot_video` Project Binding，并用 ModelProfile capability 校验该时长、比例、图片输入和 `generate_audio=false`。精确组合不受支持时返回 `provider_capability_mismatch`，不得静默取整时长、裁切比例、换模型或生成另一种媒体。
- MVP 每个 Shot 一次只生成一个视频 Candidate，输入只使用已选 Shot 图片；多镜头拼接、多素材自由编排、音频生成和转场不是本轮目标。
- Domain `GenerationTarget`、GORM `gen_targets.kind` 与 Payload Decoder 同步加入 `shot_video`；不新建第二套视频 Target 表，也不保留只接受图片的兼容分支。

### 8.2 视频 Artifact、Candidate、QC 与 Owner Apply

视频复用现有 Artifact/CandidateSet/Human Selection 骨架，但媒体字段必须是真实 strict union：

- `Artifact` 与 Provider Output 允许 `video/mp4`，并冻结 `width/height/duration_ms/codec/container`；图片仍只接受现有受支持图片类型。正式事实来自 Staging 字节的 SHA-256 与 `ffprobe`，不是 Provider JSON 自报值。
- `GenerationCandidate` 增加 `modality=image|video` 与 strict media metadata；现有 `ImageQCPolicy` 保留，新增 `VideoQCPolicy`，首版只允许 MP4、正宽高、受支持 Codec/Container、字节上限，并要求探测时长落在 Target 时长的明确容差内。容差作为版本化 Policy 快照，不散落 Adapter 常量。
- CandidateSet、ReviewDecision 与 CandidateSelection 已是媒体无关的不可变选择证明，直接复用；Readiness/QC Port 按 Candidate modality 选择 Policy，不复制 `VideoCandidateSet` 或 `VideoSelection` 表。
- 新增不可变 `ShotVideoBindingVersion`，对称冻结 Shot、GenerationTarget、CandidateSelection、Candidate、Artifact 的 ID/Revision/Hash、视频元数据、Binding Revision、Creator 与 ContentHash。Apply Command 必须再次证明所选 Candidate 为该 Shot Workflow 的 `shot_video` Target 输出且 QC Passed，不能把别的 Shot 或图片 Candidate 绑定进来。

### 8.3 Shot Workflow 与 StoryGraph

`lanverse.shot-production` 在已有图片 Human Gate 之后增加同一 Run 内的强制视频段：

```text
selected ShotImageBindingVersion
  → build_shot_video_target
  → prepare/cost/quota/claim
  → seedance_submit
  → Temporal timer + seedance_query
  → stage/materialize/video_qc
  → generation_candidate_selection Human Gate
  → apply_shot_video_binding
```

Temporal 继续是唯一跨步骤等待和恢复引擎。取消 Run 时停止后续 Query/Apply；只有火山官方合同明确支持且能够证明同一 remote task 时才调用远端 Cancel，否则保留远端 task id 与费用对账事实，不报告已取消远端计费。

StoryGraph 增加 `shot_video_binding_version` 节点，Owner 为 `production/storyboard`，并只扩展三条受控关系：

- `ShotImageBindingVersion --feeds_generation--> GenerationTarget(kind=shot_video)`；
- `GenerationTarget --feeds_generation--> Artifact(video)`；
- `Shot|Artifact --binds_output--> ShotVideoBindingVersion`。

Provider Connection/Profile/Secret/Job 仍不进入 StoryGraph 权威内容节点；Canvas 只展示业务 Target、Artifact 与最终 Binding。局部重跑从 `build_shot_video_target` 开始，冻结新的 RunInputSnapshot 和 Project Binding Version，既有 `ShotVideoBindingVersion` 只由新 Apply 版本取代，不原位更新。

Provider Binding 在 Target Builder、Cost、Quota 和 Execution Claim 之前解析。Application 必须重读并冻结 exact Connection/Secret/ModelProfile versions；任何缺失、禁用、权限漂移或 capability 不匹配都保持零 Intent、零 Reservation、零 Provider 调用。

执行顺序：

```text
Workflow Node
  → resolve exact Project Provider Binding
  → build GenerationTarget
  → Cost/Quota Reservation
  → Execution Claim
  → persist Provider Request/Job
  → transaction ends
  → resolve/decrypt Credential for one outbound call
  → Provider Submit or Query
  → terminal Receipt + private Staging
  → Asset READY → Candidate/QC/CandidateSet
  → Human Gate → exact Owner Apply
```

所有远程调用都在 PostgreSQL 事务外。Temporal 是唯一跨步骤等待、Timer、Signal、取消与恢复引擎；Kafka 只发布已提交业务事实，不调度 Provider Job。Node Cache 不承担远端幂等，只缓存完整 canonical Node Output。

Web Provider Settings 只能配置和绑定 `shot_video` Provider，不能直接绕过上述 Shot Workflow 发起孤立视频；所有 Seedance 调用都必须具有 Project Owner、Target、Cost/Quota、Lineage、Human Gate 与最终 Binding。

## 9. Secret 与安全

- Provider API Key 只在 Web → Backend TLS Request、短生命周期解密缓冲区和对应 Adapter Authorization Header 中出现；
- Secret 不进入 Response、OpenAPI Example、日志、Trace、Metric Label、Error Detail、Command Receipt、Outbox/Kafka、Temporal Input/History、Target、Request Hash 或 Artifact Metadata；
- Connection/Model/Binding Query 只返回非敏感版本与 fingerprint；
- Root key 固定由 Docker Secret 文件提供，不通过 `.env`、镜像 ARG、Compose 明文或仓库文件；
- Provider Host/Region 由编译 allowlist 决定，阻止用户借“自定义 Provider”实施 SSRF 或把 API Key 发往任意 Host；
- 输入 Artifact 必须通过 Backend 受控读取/上传路径，不能向 Provider 暴露私有 MinIO URL；
- 输出 URL 每次请求和重定向都校验 scheme、host、DNS/IP、端口、大小、媒体与解码结果；
- Owner API 必须基于当前 Token Version/Membership 授权；跨 Workspace Connection/Profile/Binding 引用统一按防枚举策略处理；
- 日志只发往既有 Logstash 链路，不引入 Filebeat；日志记录稳定业务 ID、Provider/Model 非敏感标识、状态、耗时和错误码。

## 10. 失败、恢复与配置变更

| 场景 | 业务事实 | 恢复动作 |
|---|---|---|
| 零 Provider 配置 | 非视觉服务正常；视觉命令 `provider_configuration_required` | Owner 在 Web 创建连接/Profile/Binding |
| Secret Store 文件缺失 | 不创建/轮换/执行 Provider；不影响只读与非视觉服务 | 部署正确 Docker Secret 后重试原 Command |
| Connection/Profile/Binding 禁用或漂移 | 新 Job 在 Cost 前拒绝 | 发布新版本或新 Run |
| Credential 轮换 | 新 Job 使用新 Version；旧 Job 保持原 Version | 旧凭据仍有效则原身份 Query；已撤销则人工对账 |
| 任一同步图片 Call 成功 | Call Receipt + 私有 Staging | Temporal 派发下一 candidate index |
| Call 本地预校验失败 | `FAILED + local_failure_code`，无 `dispatch_boundary_entered_at`/Provider Receipt | 不调用 Provider，不计入 `settled_budget_units` |
| Call 已提交 `DISPATCHING`，但 Submit 结果未落库 | 发送边界已越过，业务上未解决 | 重投/并发路径不得 Submit 或无 ID Query；Temporal 收敛为 `OUTCOME_UNKNOWN` |
| 同步图片 Call outcome unknown | Call/Job 保留 Reservation 与 OUTCOME_UNKNOWN | 不派发后续 Call、不自动重提；人工对账/明确新 Run |
| 部分图片 Call 成功、其余明确失败 | Job `PARTIAL_SUCCEEDED`，按已远程派发 Call 数一次聚合结算 | 只物化成功且 QC Passed 的 Candidate 并向 Human Gate显示失败 |
| Seedance 已返回 remote task id | 单一视频 Call/Job `SUBMITTED/RUNNING` | Temporal Timer 后按同 ID Query |
| Seedance 未返回 task id 时 outcome unknown | 单一视频 Call/Job `OUTCOME_UNKNOWN` | 无官方恢复身份时不得重提 |
| Provider Call 明确失败 | Call FAILED，Job 按聚合规则收敛并按费用事实释放/结算 | 继续未派发 candidate 或由 Human Gate 处理部分结果；修复输入需新 Run |
| 输出下载/对象存储暂时失败 | Provider 终态与物化未完成分开记录 | 同一 Output Identity 重试 Staging |
| Worker 重启/Activity 重投 | 不创建第二 Request/Job/Call | PostgreSQL + Temporal History 恢复 |
| 用户修改 Web 默认参数 | 只产生新 Profile/Binding Version | 既有 Run 继续用冻结版本 |

不实现自动 Provider fallback。不同模型的费用、内容安全、输出质量和恢复语义不同，自动切换会改变已批准输入与审计事实；需要切换时由 Owner 明确发布新 Binding 并启动新 Run。

## 11. MVP 强制交付顺序

本 Design 接受后，先按文档链更新受影响的 Backend/Frontend Design、PRD、Requirement、唯一 Plan 和全未勾选 Acceptance，再按以下依赖拆成独立提交。此处只解决一个完整工程不能同时修改全部边界的落地顺序，不表示任何 Provider 可选、延期或只做预留：

1. 内置 Preset Catalog、Provider Connection/Credential/ModelProfile/Binding Schema、加密 Secret Store 和空配置启动语义；
2. 精确 ModelProfile PriceQuote/Quota、ProviderCall/Call Receipt 与 Job 聚合状态，先用受控 Gateway 证明四 Call、部分失败、outcome unknown 和重启恢复；
3. Workspace Owner Backend API、OpenAPI 与 Web Provider Settings，包括预设卡片、受控字段、显式验证、PriceQuote 与持久连接版本；
4. 火山 Seedream 5.0 Pro+ Adapter 的离线合同、真实凭据 Staging 与 `reference_asset` 闭环；
5. OpenAI GPT Image 2 Adapter、连接/Profile 与真实 `reference_asset` 闭环；
6. Google Nano Banana Adapter、四个精确 ModelProfile 与各自真实 `reference_asset` 闭环；
7. `shot_frame` 精确 AssetVersion 输入与三类图片 Provider 闭环；
8. 按本设计实现 `shot_video` Target、视频 Artifact/Candidate/QC、`ShotVideoBindingVersion`、StoryGraph Edge 和 Shot Workflow，再接通 Seedance 2.0、2.0 Fast、2.0 Mini、2.5 Adapter 合同、异步恢复与真实视频闭环。

四类 Provider 都是 2051 完成条件，不根据业务优先级删减，也不以“已预留接口”代替实现。每个 Adapter 必须分别完成官方合同核验、Red/Green、真实凭据化验收、全量 CI 和独立 Git 提交后才进入下一项。

Runware 专用 Config、Adapter、Route、OpenAPI Schema 与测试在新链路实现时直接删除或替换，不保留兼容分支。没有 Migration、双写或历史数据转换脚本；当前尚无真实 Runware 生产 Job，可由空库 GORM Catalog 直接建立新事实。

## 12. 验收门

通用 Provider MVP 至少必须证明：

1. 零 Provider 配置时 Backend/Frontend/StoryGraph/非视觉 Workflow 正常，视觉命令在 Cost 前稳定阻塞；
2. Catalog 必须包含本设计列出的全部 Connection/Model Preset，且每一项都与 Backend 真实 Registry Factory 一致；Owner 只需补齐受控字段即可创建持久连接，未知、缺少 Factory 或被篡改的 Preset/字段均失败关闭；Preset 升级不改变已有 Connection/Profile Version；
3. Workspace Owner 可在 Web 分别创建火山、OpenAI、Google 连接，写入一次性 API Key，创建 Seedream 5.0 Pro+、Seedance 2.0+、GPT Image 2 与 Nano Banana ModelProfile，并发布 `reference_asset|shot_frame|shot_video` 的精确 Project Binding；非 Owner 不可写，任何 Query/日志/Temporal/Kafka/数据库业务列均无明文 Secret；
4. Backend/容器重启后仍能解密同一 CredentialVersion；缺失或错误 root key 明确失败且不破坏事实；
5. 真实 Seedream 5.0 Pro+ 经过对应 PriceQuote/Quota 生成 reference sheet，结果进入私有 MinIO/Artifact/CandidateSet，人工选择后发布精确 AssetVersion；
6. 真实 GPT Image 2 使用独立 OpenAI 连接和对应 PriceQuote/Quota 完成同等 `reference_asset` 生成、Staging、CandidateSet 与 Owner Apply；
7. 真实 Nano Banana 2 Lite、2、Pro 与 Legacy 分别使用精确 ModelProfile 和对应 PriceQuote/Quota 完成同等 `reference_asset` 生成、Staging、CandidateSet 与 Owner Apply；2/Pro 必须证明 Interactions 合同，2 Lite/Legacy 必须证明 Generate Content 合同，任一精确 Profile 都不得在运行时试探或回退到另一接口；
8. `shot_video` strict Target、真实视频 Artifact 元数据、Video QC、Human Gate、`ShotVideoBindingVersion` 与 StoryGraph 关系全部落地；真实 Seedance 2.0、2.0 Fast、2.0 Mini 与 2.5 分别经过对应 PriceQuote/Quota 创建任务、重启恢复、同 remote id 查询、下载 Staging 与 Owner Apply 全链通过；
9. 一个四候选图片 Target 必须持久化四个独立 ProviderCall，每个 Call 只发生一次真实远端调用并最多独立收敛一条终态 Receipt，无官方事件 ID 的明确失败不伪造 ID；全成功、明确部分失败、Call outcome unknown、重启和 Activity 重投均证明无隐藏循环、无重复付费且 CandidateSet 聚合正确；必须分别覆盖 `PENDING` 事务提交前崩溃、`DISPATCHING` 提交后 HTTP 前崩溃、HTTP 响应后 Receipt 前崩溃与并发重投，证明只有首次 `PENDING → DISPATCHING` 的调用路径可以 Submit；明确终态只产生一次 Cost 聚合结算，outcome unknown 不释放预留；
10. Credential/Profile/Binding 轮换不会改变既有 Job/Call，新 Job 使用新版本；并发/重复 Command 只产生一套版本与 Receipt；
11. 同步 outcome unknown 与异步 remote task 恢复按 Provider 真实能力处理，不盲目重提或重复付费；
12. PostgreSQL/GORM 仍是唯一 SQL 事实源，Backend 是唯一 Writer，Temporal 是唯一跨步骤引擎，测试只在独立 test 目录；
13. 全量真实 CI、Compose、镜像、Secret/Data/日志 hygiene 通过后，最后才使用 `agent-browser` 验收 Web 预设 → 三类持久连接 → Project Binding → 四类真实模型生成 → Artifact/Review 全旅程。

缺少火山、OpenAI、Google 任一 API Key/额度、OpenAI GPT Image 所需 Organization Verification/模型权限、Docker Secret Root Key，或 Google 在 Legacy Nano Banana 真实验收前已关闭该模型时，必须如实阻塞对应验收和 2051 整体完成；`shot_video` Target/Owner 是本设计的必交代码，不再作为外部前置条件或延期理由。不得把任何 Provider 降级为计划项，也不得使用 Runware、Codex、占位媒体、受控测试 Gateway 或历史 CandidateSet 报告真实 Provider 通过。

## 13. 重新接受后的同步清单

本设计重新接受后，按依赖而不是文件编号依次同步：

1. `0001` 完整设计基线、`0003` 系统总体架构：把固定图片 Provider 更新为可空通用媒体 Provider、Web 配置与图片/视频能力边界；
2. `2001` Backend 服务架构、`2002` Backend 领域设计：加入 Connection/Credential/ModelProfile/Binding Owner、Secret Resolver 与 Registry/Adapter 依赖方向，保持单 Backend Binary；
3. `1001` 前端应用架构、`1002` 前端功能模块：加入 Workspace Provider Settings、只写 Secret、空配置和项目 Binding 页面，继续禁止 Browser 直连 Provider；
4. `0010` StoryGraph 设计与 `2055` Human Gate：加入本设计已固定的 `shot_video` Target、`ShotVideoBindingVersion`、受控 Edge 和媒体 Selection 边界，不把 Provider 配置投影成 StoryGraph 权威节点；
5. `0010` PRD、`0010` Requirement：用 Seedream 5.0 Pro+、Seedance 2.0+、GPT Image 2、Nano Banana、Web 配置、密文 Secret 和按 Provider 能力恢复取代 Runware/环境变量合同；
6. `0010` 唯一 Plan：重新拆分当前视觉切片，先交付通用配置事实和 Web，再依次交付三类图片 Adapter 与 `shot_frame`，最后交付本设计已固定的 `shot_video` Owner 链和 Seedance 2.0+；这里仅表达依赖顺序，四类 Provider 均为本轮必做；
7. `0010` Acceptance：新增全未勾选的新 Requirement/Task 条目；既有 Runware Evidence 保留为历史实现事实，不改写成火山通过，也不能抵扣新验收。

接受前事实扫描已经定位以下精确同步点，接受后不得只做全局替换：

| 文件 | 当前冲突事实 | 接受后动作 |
|---|---|---|
| `design/0001`、`design/0003` | Provider/Capability 仍是通用枚举，未固定 Preset、Connection/Profile/Credential、三连接和四类必接 Adapter | 补齐目标架构、唯一 Writer、Secret 与 Workflow 边界 |
| `design/1001`、`design/1002` | 只禁止 Browser 直连 Runware，尚无 Workspace Provider Settings 与 Project Binding 旅程 | 改为禁止直连任何媒体 Provider，并补齐 Web 页面/API 依赖 |
| `design/2001`、`design/2002` | 尚无新版 Connection/Credential/ModelProfile/Preset Registry Owner | 固定 Backend 模块、GORM Model、事务与 Composition Root |
| `prd/0001` | Binding 仍由 Backend 进程 allowlist 固定填充，部分增量仍声明不含 Provider 管理 UI | 改为 Owner 从 Web 预设创建持久连接并发布 exact version Binding |
| `prd/0010` | 仍把 Runware 写成首个真实图片 Provider，外部条件只列 Runware 凭据 | 改为四类 Provider 必接与火山/OpenAI/Google 三套凭据条件 |
| `requirement/0010` | 标题和 `SG-VIS-003`、`SG-VIS-012`、`SG-OPS-002`、`SG-JRN-002` 固定 Runware/进程配置协议 | 改为按 Adapter 能力区分同步/异步恢复、Web Secret 与精确 Binding |
| `plan/0010` | `SG-I20`、运行变量与剩余范围仍围绕固定 Runware | 重新拆成通用配置、三类图片 Adapter、shot frame、视频 Target 与 Seedance 2.0+ 的完整任务序列 |
| `acceptance/0010` 的目标 Checklist | `SG-VIS-003`、`SG-OPS-002`、`SG-I20` 仍以 Runware 为目标 | 新增全未勾选四类 Provider、Web 配置与真实旅程完成门；不复用旧勾选 |

`plan/2002` 已完成项、`acceptance/2040`、`acceptance/2045`–`2050`、`acceptance/2052`–`2056` 以及 `acceptance/0010` 中 2026-08-29 的 Runware Evidence 都是历史事实。它们只修复失效链接或增加“历史范围”说明，不替换 Provider 名称、不改变当时命令/结果、不重新计为目标完成。

代码、OpenAPI、Compose、Docker Secret、Frontend 和测试只能在上述派生链全部同步并重新接受后开始修改。
