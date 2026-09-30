# BeefTV 生成能力迁移设计

| 项       | 内容                                                                                                                                     |
| -------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| 状态     | 实施中；2026-10-01 用户授权开始迁移，已实施 M1-12.02/.03 首片内部合同；用户确定本机 Codex 优先，真实启用条件仍待验证                     |
| 问题     | Python 执行端已移除，现有 Operation 仍只有 mock 路径；页面入口不能代表生成能力已接通                                                     |
| 接入顺序 | 本机 Codex 内置 imagegen → 火山引擎 Seedream 图片 / Seedance 视频 → OpenRouter 其他模型；先完成单条图片链                                |
| 来源     | BeefTV 固定提交 `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98` 的选定协议；不改变已接受的画布来源 `0d9e9f48d407570cd431ad9730cdd522b06810c0` |
| 上游约束 | [增量评估](../prd/BeefTV增量能力与服务适配评估.md)、[执行端清理](Agent服务目录清理设计.md)、DES-03/04/07、M1-12                          |

## 1. 决策与范围

把选定协议迁入现有 Go Worker，由 Lanverse 的 Operation、媒体和账本继续持有业务事实。先完成能从确认请求执行到素材候选的能力，再让现有画布和工作台消费它；暂缓新增页面。

第一批为本机 Codex 内置 imagegen 文生图，随后接火山引擎 Seedream 图片、Seedance 视频，再用 OpenRouter 扩其他模型。参考图与蒙版按每条线路的实际支持放行。用户确定的是接入方向；具体模型版本、参数、权限、用量与计费仍需逐条验证。已发送的本机请求遇到断线或未知结果时，不自动切换到 API 再生成。

首条产品链拟固定 `target_type=free`，复用自由画布报价。用户采用同项目 `ready/passed` 素材后，经已有 `AddNodes` 命令持久绑定为 `resource / media_asset` 节点；节点不写 Operation 或选定事实。镜头的 `selected_frame/take` 正式选定另按 DES-25/E-22 接入，只有选择 `shot_frame/shot_take` 目标时才加入真实镜头数据与选定命令前置，不隐含在首条自由画布生成中。

迁移单位为选定协议的请求构造、响应解析、状态/错误转换及对应测试场景。上游图片能力主要由 `plugin-packages/openai-images/manifest.json` 加 Go 的 manifest 解释器实现；应把选定字段转成显式 Go 实现，不迁表达式解释器、动态插件发现和整套插件平台。

保留现有 Next.js、Go、PostgreSQL、Temporal、对象存储与单一工作区边界。不新增独立 BeefTV 服务、数据库、任务队列系统或浏览器密钥配置；不引入 Wails、SQLite、时间线、3D、深度推理、旧 Agent 或 FFmpeg.wasm。

## 2. 实施前基线与当前边界

| 事实来源                                     | 当前事实                                                                  | 本次处理                                                       |
| -------------------------------------------- | ------------------------------------------------------------------------- | -------------------------------------------------------------- |
| `operation/adapter/workflow/workflow.go`     | 只允许 `mock / agent.mock / supports_query`；真实媒体输入仍被拒绝         | 为首个选定协议增加明确执行分支；保留历史 mock 路径             |
| `provider_contract.go`、`provider_calls.go`  | submit 只有 accepted 等异步结果；accepted 没有真实 task ID 会变成 unknown | 同步协议必须增加同步完成合同，不能伪造供应商任务 ID            |
| `app/role.go`、`app/roles_wiring.go`         | Go Worker 只支持 flow/media，没有生成执行者                               | 在现有 Worker 角色中注册选定 Activity；实际注册前不宣称可用    |
| `catalog/adapter/credentialseal/sealer.go`   | 只有公钥封装；Go 未实现解封、执行授权读取和私钥注入                       | 复用密文格式，增加 Activity 内的短期解封和按操作校验的读取端口 |
| `media/application/ingest.go`                | 接管输入只有 URL，不能直接消费同步 base64                                 | 增加有界的私有暂存结果引用，复用探测、接管、登记流程           |
| `workflow.go`、`app/operation_settlement.go` | 工作流仍调用 mock 审核并传零成本；结算严格核对持久调用成本                | 接真实审核与冻结价格计算；缺用量或费用证据时不结算为零         |
| 公开路由与 BACKLOG E-21-02                   | 报价/确认已有内部用例，正式 HTTP/OpenAPI 接线尚未完成                     | 复用已接受的命令合同；公开入口与生成客户端按现有契约链路实现   |

上表记录首片实施前的缺口。2026-10-01 已在 §5.1 实现 completed DTO、受限 receipt、持久发送权和已执行保护；provider_calls.go、Workflow 与 Operation 状态转换未修改，执行、媒体、计价、审核和公共接线仍待后续切片。技术结果见 [首片验收](../acceptance/M1-12-同步调用证据与发送权验收.md)，不关闭真实生成验收。

## 3. 模块与执行边界

| 所有者                         | 责任                                                                                        |
| ------------------------------ | ------------------------------------------------------------------------------------------- |
| `catalog`                      | 能力、参数、模型/价格版本、供应商及凭据状态；只有经过授权的执行读取才能取得该操作对应的密文 |
| `operation` application/domain | 确认、冻结输入、调用身份、状态、费用证据与恢复规则；定义实际消费的小接口                    |
| `operation` provider adapter   | 单次 Codex turn 或受控 HTTP 转换；不自建业务任务、循环轮询或结算，不写画布                  |
| workflow adapter / Go 组合根   | 注册 Activity、确定队列/超时/重试、注入客户端、凭据读取、解封器与结果暂存端口               |
| `media`                        | 私有暂存、大小/格式校验、ffprobe、对象接管、审核和候选资产登记；暂存物不是可选定资产        |
| 现有 finalizer/billing         | 从持久 provider_call 计算的真实成本执行原子结算；保持现有预留与幂等规则                     |
| 现有前端入口                   | 展示报价、明确确认、任务恢复、候选预览与用户选定；不持有供应商 secret                       |

保持 `adapter → application → domain`；跨业务事务继续由当前组合根协调。消费方定义接口，用构造函数注入依赖，不引入通用 SDK、全局注册表或仅转发的 Service 层。供应商 HTTP 遵循 `context.Context` 取消、有限超时和响应容量限制。

## 4. 本地优先的供应商接入

### 4.1 本机 Codex 内置 imagegen

2026-10-01 只读核验本机 `codex-cli 0.159.2` 的帮助和二进制生成的公开 JSON schema：app-server 提供 stdio，存在 `modelProvider/capabilities/read` 的 `imageGeneration` 能力字段，以及 `thread/start`、`turn/start` 和完成事件。`imageGeneration` item 包含 id/status/result，可选 savedPath/failure；没有直接 `images/generate` RPC。这只证明协议存在，尚未连接运行服务、核验账号权限或生成图片；应用内 imagegen 可用也不能代替 CLI 验证。部分字段需要 experimentalApi，按固定版本逐项核验。[官方 app-server 文档](https://learn.chatgpt.com/docs/app-server)

首个 adapter 拟由能访问 Codex CLI 的本机 Go 执行 Worker 管理独立 stdio app-server，先做不启动推理的能力探测，再在专属任务目录创建 thread/turn。已有 Operation/attempt 关联本任务 thread/turn/item；取得持久发送权后只发起一次生成 turn。clientUserMessageId 不当作付款幂等保证。断流、超时或 interrupt 后只读取本任务历史/可靠收据核对，不重新 turn/start、不自动切到 Seedream。进程和 reader goroutine 由执行 Worker 管理，具备取消、退出与等待。

事件 result 仅声明字符串，当前没有编码证据；savedPath 可为空。真实执行必须证明产物获取路径，再将受限任务目录内的文件按大小、真实图片格式与 SHA-256 验证，复制到现有私有 staging/manifest；本机文件与 Codex 会话不作为 Lanverse 的唯一恢复事实。Worker 在容器或其他机器运行时不能默认读取本机文件。一次 turn 是否只产生一张图、迟到 item 如何归属、恢复是否取得同一产物，均列入真实验收。

“本地优先”指本机客户端接入，不能据此认定离线推理或零成本。内置图片消耗 Codex 计划配额；公开 token usage 不能直接充当图片实际用量/费用，缺证据继续保持未知。额度与本项目费用/预留如何映射在 .08 明确后才开放产品确认。[官方图片生成说明](https://learn.chatgpt.com/docs/image-generation)

本机桥复用用户已登录 CLI，不读取、复制或导出身份凭据，不恢复整套 Agent/Harness。外部 Sign-in-with-ChatGPT Responses 路线当前不支持图片生成，不将其当作内置 imagegen 的替代路径；不无声回退收费 OpenAI Images API。[官方限制](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)

### 4.2 火山引擎与 OpenRouter API

火山引擎图片用 Seedream、视频用 Seedance。API adapter 复用模型/价格版本、服务端凭据与发送门禁，按具体模型官方参数、真实 usage、异步 task ID、查询/取消合同独立实现；版本、预算和价格尚未冻结。[Seedream 图片接口](https://docs.volcengine.com/docs/ark/image-generation-api?lang=en)、[视频任务接口](https://docs.volcengine.com/docs/ark/create-video-generation-task-api?lang=zh&redirect=1)

OpenRouter 扩其他模型。其图片文档提供独立 `/api/v1/images` 与模型/端点能力信息，不能默认套用 BeefTV `/v1/images/generations` 或让所有模型共用一种 usage/价格。首片 receipt 仅接受 PNG/JPEG/WebP，其他格式在明确新合同前禁用。[OpenRouter 图片文档](https://openrouter.ai/docs/guides/overview/multimodal/image-generation)

[BeefTV OpenAI Images manifest](https://github.com/glanderness/BeefTV/blob/1ae25027f7ea1c2178e1e4133c36a0f2995d0e98/plugin-packages/openai-images/manifest.json) 保留为协议映射参考，不作为本机 Codex 执行实现。API 地址来自服务端固定线路，浏览器和 Operation 参数不传任意 BaseURL；HTTPS、域名/端口/IP、重定向、授权头和容量受限，凭据不跨来源转发。fixture transport 与模拟 stdio 只证明技术合同。

## 5. 同步完成合同与可靠收据

现有异步 `accepted + provider_task_id` 保留。为同步协议拟增加 `completed`：返回已暂存的结果引用和归一化用量，不返回伪造的供应商 task ID，也不把图片/base64、secret 或预签名 URL写入 Temporal 历史。`rejected / not_submitted / unknown` 仍具有当前语义。

同步执行成功后，执行 Activity 先把结果字节及必要的脱敏收据写入现有私有对象存储，再返回由 Operation 与结果序号限定的引用、大小、MIME 和 SHA-256。使用固定暂存前缀，由服务端的 project / operation / action / attempt / sequence 派生键，不允许调用方指定桶或对象键。暂存引用只由受信的媒体 Activity 解析，不接受浏览器任意对象键。`media.Ingest` 增加该引用输入分支，在媒体 Worker 内生成当前 `application.Downloaded`，继续通过现有字节探测、正式对象验证和原子输出登记后才建立候选资产。

恢复只读取同一调用的可靠收据和结果，不再次生成。最小持久身份为 Operation、action、attempt 与 request key；完整收据绑定冻结模型、产物序号/大小/MIME/SHA-256、脱敏供应商请求标识及可信用量证据。暂存图片先按稳定键条件写入，校验已有对象摘要，最后条件写入该调用的收据 manifest；不同摘要拒绝覆盖。完整 manifest 标记产物已收齐，并独立标记费用证据是否可核定，不能把“已生成”和“可结算”合成一个状态。

在现有 `operation.provider_call` 追加 `dispatch_started_at` 与受约束的 `receipt` 字段，不新建任务表。receipt 只保存 manifest 的受限身份、摘要和输出元数据，不保存 base64、secret、原始 HTTP body 或临时 URL；更新调用证据时核对相同的冻结模型、request key、attempt 与对象摘要。Activity 输出也只携带该稳定引用。对象完整写入而数据库更新失败时，根据该调用固定位置的 manifest 验证并补记；仅有孤立图片而无完整可靠收据时仍为 unknown，不能从对象存在推导费用。

未结算、manual、待接管和待审核的结果不可按普通 TTL 清理。已终态、费用已核定且正式对象已验证，才由现有维护队列按原操作身份清理暂存；异常孤立对象先登记待核对，不静默删除可能属于付费请求的唯一结果。实施时将这些合同同步到 DES-03/04/33。不用进程内 map 或 Worker 本地文件作为恢复事实。

同步分支在可靠结果与费用证据完整后，以 `submitting → ingesting` 承接完成结果；允许该转换之前须更新 domain、状态设计和持久调用校验。submit 的持久调用允许 `completed` 及其用量，不套用目前要求 task ID 的 accepted 校验。新增 `submit.completed` 同时纳入管理员命令、信号复核和结算的“已执行”保护；即使用量缺失或确认费用为零，也不能判为 not_executed。

结果已生成但费用证据不完整时，保留收据与产物，沿 `submitting → unknown → reconciling → manual` 保持预留。当前 manual 成功分支依赖真实 task ID 和 request-key 查询，不能直接复用：同步恢复须增加基于绑定该 Operation / attempt 的可信收据与补齐费用证据的门禁，随后恢复 ingesting；没有可靠收据仍保持待核对，不构造 task ID、不启动不受支持的 query。管理员命令和信号只触发复核，输入的产物地址、费用数字不能直接充当可信供应商证据；补充证据须有同一请求的已核验回执/账单、私有审计来源和冻结价格计算。当前 ingesting 不允许转 manual，因此须在离开 submitting 前确认成本可核定；媒体或审核失败后的结算仍消费已保存费用证据。

### 5.1 首个实施切片：同步调用证据与发送门禁

2026-10-01 首片只交付数据、载荷、持久发送权与已执行保护。`ProviderDispatchIdentity` 绑定 project、operation、submit action、attempt、request key（1～256 字节且无首尾空白）、冻结 model/price version；所有 UUID 非空。`ProviderReceipt` 固定 version=1、该身份、manifest SHA-256 和一个图片输出。输出 sequence=1、size_bytes 为 1～32 MiB、mime_type 仅 PNG/JPEG/WebP、SHA-256 为 64 位小写十六进制。32 MiB 是首片内部容量上限，不是供应商支持承诺；收据 JSON 最大 4 KiB，拒绝未知和重复字段，禁止任意 URL、桶/对象键、字节或扩展 JSON。

`BeginProviderCallInput.dispatch_required` 仅对无 provider task ID 的新 submit 合同有效，在 `request_summary` 显式记 `dispatch_contract=v1`。旧行的 `dispatch_started_at=NULL` 不能证明尚未发送，未标记 v1 的调用绝不授予新发送权。首次 attempt 必须为 1，后续连续递增且仅在所有前次 submit 都有明确 `not_submitted` 证据时登记；未知、已受理、已完成和只有 rejected 的证据均不授权重发。

`ClaimProviderDispatch` 先锁 Operation，再确认唯一调用行及完整冻结身份；只有 v1、无响应证据、未发送且 Operation 为 submitting 的调用可以原子写入发送时间。重复投递只返回原发送时间/收据，不续租或重发；同调用身份或摘要冲突拒绝。此方法不修改预算、账本或 Operation 状态。

v1 调用在 Operation 已终态时只接受完全同体的只读重放；不能首次补写响应，或将 unknown 更新为 completed 并改变收据、用量或费用。终态后迟到/矛盾的证据按冲突保留待核对，不改写已关闭账本事实。

`submit.completed` 必须为 outcome=ok、无 task ID、已取得发送权且收据身份完全吻合。非空 usage 仅接收现有标准化整数数量字段，未知/重复/负数/null/小数拒绝；缺少 usage 仍可记录已执行。持久保存完成、收据及受限用量，首片的同步成本保持 NULL，不能沿用 mock/default 零费分支；实际同步计价与补证据归 .08。三处 not_executed 的写入、信号复核与结算检查均把 completed 视为已执行，即使费用未知/人为记零/已有旧人工事件，也不得释放预留。

追加迁移的 down 在已有任意发送时间或收据时拒绝执行，避免删除唯一执行证据；空字段历史行可在隔离库验证 up/down/up。发送门禁启用后的生产回滚须保留这些字段与在途恢复能力。

当前 mock Workflow 及 Operation 状态迁移顺序保持；DTO 只追加省略空值的身份/attempt、completed、receipt/usage 字段。首片不注册供应商 Activity、不把 completed 接到 ingesting，也不宣称媒体接管、可靠对象 manifest 或历史回放已完成；对应分支到 .06～.10 经费用门禁与 `GetVersion` 后接入。

## 6. 提交、恢复与取消

单次付费发送前写入 provider_call，并以 `dispatch_started_at IS NULL` 的条件写入原子取得该调用的发送权；Activity 输入携带由工作流指定并已登记的 attempt，执行读取再次核对该操作的 request key、冻结模型和该调用行。发送权在网络请求前取得，事务不跨网络持有。Temporal Activity 的超时/重投递不等于供应商未收到请求；不能只依赖 `MaximumAttempts: 1` 防重复发送。再次执行时首先查询持久发送状态和已存收据，已取得发送权且无可靠回执的调用进入 unknown/人工核对，不以超时或租约抢占后重发。

| 情况                                           | 处理                                                               |
| ---------------------------------------------- | ------------------------------------------------------------------ |
| 参数、凭据、线路在发送前不合法                 | not_submitted，记录稳定安全错误；不消费付费调用                    |
| 有明确未受理证据                               | 依现有合同处理；HTTP 429 本身不证明所有兼容网关都未受理            |
| 已发送但超时、断连、响应解析失败               | unknown；保留同一 request key，不因 retryable 自动再次付费         |
| 同步结果已暂存，但 Activity 回执或 DB 写入失败 | 恢复同一可靠收据及结果，补写证据，不重新生成                       |
| 媒体接管、渲染或审核失败                       | 保留生成和费用事实，仅恢复媒体阶段；失败也按真实用量核定费用       |
| 同步请求执行中用户取消                         | 本地中止不证明上游取消；不报 cancelled 或零费，等待结果/费用核对   |
| 用户删除节点、切换项目或改选结果               | 不自动覆盖新状态；结果留作原任务候选，正式绑定走现有 revision 命令 |

没有官方幂等/按键查询证据的同步接口不启用 supports_query，不用本地 receipt ID 冒充供应商任务。已有 query/cancel 合同在接异步供应商时再加入操作身份与冻结线路，避免为未实现能力先扩一组无消费者 DTO。

## 7. 凭据、审核与费用

API 供应商的 Go 解封继续使用当前 RSA-OAEP-SHA256 + AES-256-GCM 的密文格式与 provider UUID / credential UUID 原始字节 AAD，保留 key_id 轮换。API 只持公钥；私钥只注入具备执行职责的 Worker，不下发到浏览器、flow 数据库 Activity 或媒体 Worker。Activity 使用该操作限定的密文，核对供应商、凭据状态及冻结模型后短期解封，禁止日志/Trace/错误携带明文。

真实执行使用现有 binary 的专门执行 Worker/供应商队列配置，不新增服务仓库。当前 `role=all` 合并 API、flow 和 media 职责，因此 all 与非执行角色不得加载供应商私钥或注册真实供应商执行 Activity；此隔离由配置校验、构造函数注入和负面测试共同固定。

首条链必须选定真实审核方式及失败合同：审核不可用保持待处理/失败，不能回退 mock 或默认 passed。按同一 Operation 恢复审核，只有 passed 且已完成接管的素材可预览、选定、绑定。

费用只基于持久 provider_call 的可信用量和冻结价格版本。现有 `price_unit` 与公式如不能表达所选模型，先在 catalog/billing 设计明确扩展，不能把 GPT image 的不同 token 类别合成一个未经证实的单价。缺用量、价格不匹配、币种/汇率未冻结、取整溢出等进入待核对；不记零费用。发送前拒绝未具备可核定计价与预算保护的模型。

## 8. 历史兼容与上线边界

不更改旧 Workflow 事件顺序或移除既有 Activity 名称/队列。新执行分支通过 Temporal `workflow.GetVersion` 隔离，提交/同步产物/结算的新增决定具有明确 change ID，使用真实历史进行回放验收。mock 队列不会被绑定到真实供应商。

数据库变化只追加迁移，保持已有操作、账本、媒体和画布历史；不重置数据库或删除 Temporal 历史。功能默认关闭，首个协议仅对具备已确认线路、凭据、模型价格、审核与预算条件的配置开放。停止新请求后仍保留已有请求的收据、媒体恢复和费用核对能力。

## 9. 验收与下一步

按 2026-10-01 用户指示，详细任务、依赖、失败用例和验收见 [能力迁移实施计划](../plan/BeefTV能力迁移实施计划.md)。实施前同步 DES-02/03/04/07/33 与现有 E-07/08/21/24/30 的具体合同，不另起重复 Epic。首批按图片优先编排；参考图/蒙版按实际模型放行，视频、音频和文本在首条底座通过后可独立推进。每条能力配现有入口的最小产品接线，全面页面重构后置。

技术验收：协议字段/模型限制、容量、base64/URL、错误脱敏、取消和 HTTP 故障；密文跨实现往返及 AAD 拒绝；持久发送权和重复投递；同步收据断点恢复；旧历史回放；真实成本与结算重复请求。业务测试位于 `backend/tests/<模块>/`，遵循 Red → Green → Refactor，执行适用的格式、vet、lint、Race 与漏洞检查。

真实集成验收：受控预算下用真实供应商提交一次，核对任务/请求回执、产物字节与 SHA-256、元数据、审核、用量、账本，并验证 Worker 重启和接管失败恢复。没有真实供应商与预算条件时只能报告技术验证，不能报告真实生成可用。

产品验收：首条 free 链复用已有项目/画布入口，从报价与明确确认到刷新恢复、合格素材预览、用户采用及持久资源绑定；节点不承载正式镜头选定事实。失败阶段和下一步准确，切换项目/改选不被迟到结果覆盖。其他目标的正式选择按其业务合同验收，单条链不替代完整 MVP 九场景。

已确定：本机 Codex 图片优先、火山引擎 Seedream/Seedance、OpenRouter 扩展。待验证或冻结：本机权限与产物、模型版本/参数、额度与费用合同、真实审核、预算及操作者管理授权。首片内部合同已实施；尚未满足真实调用与产品启用条件，不自动发布。

复制或改写选定上游代码时，在每个文件记录源码路径、固定 SHA 和实质改造；同步更新 THIRD_PARTY_NOTICES 与 MIT 声明。当前已实现首片内部 Go/数据库合同，未复制上游生成生产代码、未注册真实生成 Activity、未产生付费调用；因此本片不增加新的第三方代码声明。
