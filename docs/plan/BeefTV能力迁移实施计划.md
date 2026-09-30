# BeefTV 能力迁移实施计划

| 项             | 内容                                                                                                                                                              |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 状态           | 规划草案；本次只形成计划，不执行生成迁移、不启用供应商、不产生付费调用                                                                                            |
| 用户确定的方向 | 当前代码先提交本地 main；先迁能力，随后继续全面页面重构                                                                                                           |
| 代码基线       | 本地 main：`7d024416` 交互测试、`3d526fb2` 服务页面；页面可用不代表真实生成闭环完成                                                                               |
| 设计来源       | [生成能力迁移设计](../design/BeefTV生成能力迁移设计.md)、[增量评估](../prd/BeefTV增量能力与服务适配评估.md)、[执行端清理设计](../design/Agent服务目录清理设计.md) |
| 固定来源       | 生成协议候选：BeefTV `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98`；既有画布仍为 `0d9e9f48d407570cd431ad9730cdd522b06810c0`                                          |
| 任务归属       | 拆解现有 M1-12，并关联 E-07/08/11/21/22/24/26/27/28/29/30/39；不新增重复 Epic，不增加 BACKLOG 任务总数                                                            |
| 仍待确定       | 生成设计的具体合同、首个供应商/线路/模型、用量与价格、真实审核、本机操作者授权、真实测试预算                                                                      |

**目标：**把 BeefTV 的选定供应商协议改写成 Lanverse 现有 Go Worker 的执行能力，先交付一次文生图的完整产品链，再扩展可验证的参考图/蒙版、既有提示词与图片处理、视频、音频、文本。

**架构：**Operation 持有冻结输入、调用身份和状态；catalog 持有模型/价格与授权密文；供应商 adapter 只执行一次受控 HTTP；media 接管、探测、审核并建立候选；现有 billing/finalizer 原子核定费用。Workflow 决定顺序，组合根显式注入依赖。

**工具链：**保留 Go、PostgreSQL、Temporal、私有对象存储、Next.js、TanStack Query、shadcn/Radix，以及 Handler/DTO → swag → 在线 Swagger → `@umijs/openapi` 的契约链。

## 1. 范围与执行规则

1. 本文规划 18 个 `M1-12.xx` 子任务，编号只用于跟踪迁移切片；原 Epic 的完成定义与完整 MVP 九场景保持。当前 UI 提交不追认 M1-12、E-21/24 或真实生成完成。
2. 生成设计目前为草案，用户已授权详细规划，尚不代表全部具体合同已接受。代码开工前落实对应技术合同；不要求所有外部选型一次完成。真实供应商信息未定不阻止无生成网络、无真实凭据的发送权/收据/临时密钥与 fixture transport 测试。
3. 供应商启用和真实调用另有明确条件。接受设计或本计划不自动授权付费、发布、读取真实 secret。执行测试默认使用 fixture transport、临时密钥和隔离数据；不记录凭据正文、原始 HTTP body 或可访问的签名 URL。
4. 不另建 BeefTV 服务、任务引擎或数据库，不迁 Wails/SQLite、manifest 解释器、动态插件平台、3D、时间线、FFmpeg.wasm；不恢复已删除的 Python 服务或整套旧 Agent。
5. 必做范围取自已接受的需求与设计；已有文档标为草案或待确认的部分须在对应任务入口处理，不能仅凭存在条目认定已接受。参考图/蒙版属于模型支持后放行的扩展；宫格拆分、PNG 标注、智能引用、AI 提示词优化器仍是候选。
6. `S/M/L` 表示相对工作量：S 为单一边界，M 为数个协作边界，L 为跨状态/持久化/产品闭环。它们不是工期或日期承诺；L 任务按内部验收步骤逐项提交证据，过大时继续拆步骤，不增加重复 Epic。
7. 每个代码步骤遵循 Red → Green → Refactor。下面的新增测试名是拟定验收入口，尚不存在；先写出测试并证明其因缺少行为失败，才运行对应命令。没有匹配测试或全部 Skip 不能计作通过。

## 2. 顺序、依赖与六个门禁

默认审阅顺序为：**文生图 → 参考图/蒙版 → 既有规则提示词与图片处理 → 视频/音频/文本 → 全面页面重构**。这是能力扩展顺序，不能把条件扩展或候选工具变成后续能力的硬前置。

| 门禁                | 任务        | 输入与放行条件                                                                              | 产物与退出条件                                                                                                                           |
| ------------------- | ----------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| G0 合同就绪         | .01         | 技术边界、当前单工作区权限及历史兼容方案可审阅                                              | 明确已确定/待确定项；技术底座可开工；真实启用仍关闭                                                                                      |
| G1 文生图技术底座   | .02～.11    | 对应合同已确定；隔离 PostgreSQL/对象存储/Temporal；fixture 与临时密钥                       | 并发发送、可靠收据、媒体、费用、审核失败路径、历史回放、公共契约均有真实执行的技术证据；不宣称真实供应商通过                             |
| G2 首条真实文生图   | .12         | G1；首链 target_type=free；真实线路/模型/参数/用量价格/审核；安全注入凭据；用户明确预算上限 | 单次真实生成与重启恢复证据；现有入口报价确认、刷新、预览、用户采用 ready/passed 素材、持久 AddNodes 资源绑定通过；技术/真实/产品证据分列 |
| G3 图片扩展复用     | .13～.14    | G2 后优先审阅；编辑取决于实际模型；规则提示词/裁剪取决于各自已有素材与上传合同              | 支持的扩展独立验收；不支持的模式明确禁用；候选功能保持候选，不阻塞 G4                                                                    |
| G4 其他能力纵向交付 | .15/.16/.17 | G2 的共享底座；各能力自己的已确定协议、业务目标、计价、审核、预算与数据前置                 | 视频、音频、文本各自独立取得技术/真实/最小产品证据；三者可并行，不相互等待全部完成                                                       |
| G5 全面页面重构     | .18         | 对应能力已过 G2/G3/G4，公共 API 和实际产品状态稳定                                          | 页面只展示已可用能力，适用浏览器交互和质量门禁通过；完整 MVP 仍按原九场景独立验收                                                        |

可并行范围：.02 后 .03 与 .04；.03 后私有收据和媒体实现可与 .04 的临时密钥验证并行；供应商合同确定后 .05/.08/.09 可按接口边界并行；.11 的 Handler/客户端与 .10 的注册回放可并行准备。最终 G1 必须合并验证，不以单元通过替代跨边界证据。

首条图片完成后，.13 和 .14 按适用需求推进，.15/.16/.17 可并行。每条能力的**现有入口最小接线**随该能力产品验收交付；.18 后置的是全面页面重构，不是所有界面接线。

## 3. 现有接口和职责定位

| 复用位置                                                                                 | 已有事实                                                              | 迁移时的边界                                                                               |
| ---------------------------------------------------------------------------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `backend/internal/operation/adapter/workflow/provider_contract.go`                       | `ProviderSubmitInput/Output`、query/cancel 类型                       | 同步新增 `completed`；异步 `accepted` 仍需真实 task ID；只在实际消费时扩 query/cancel 身份 |
| `backend/internal/operation/application/provider_call.go`                                | `BeginProviderCallInput`、`CompleteProviderCallInput`、`ProviderCost` | 消费方定义发送权/收据小接口；持久行是恢复事实，不靠进程内 map                              |
| `backend/internal/catalog/adapter/credentialseal/sealer.go`                              | RSA-OAEP-SHA256 + AES-256-GCM 公钥封装                                | 复用密文与原始 UUID AAD；Go 解封、授权读取和私钥注入尚待实现                               |
| `backend/internal/media/application/ingest.go`                                           | `IngestInput`、`Downloaded`、Downloader/ObjectStore/Prober/Renderer   | 在 media Worker 解析受限暂存引用，再复用字节探测和原子候选登记                             |
| `backend/internal/operation/application/create_free_quote.go`、`confirm_single_quote.go` | `CreateFreeQuoteInput`、`ConfirmSingleQuoteInput` 与内部命令          | `free` 表示自由生成目标，不是免费费用；复用冻结价格/输入和确认，不另造报价系统             |
| `backend/internal/app/role.go`、`roles_wiring.go`                                        | 当前 Worker 队列只有 flow/media                                       | 现有角色中显式加入执行职责；不得把 mock 队列绑定真实供应商                                 |
| `frontend/openapi2ts.config.cjs`、`src/gen/api/`                                         | 在线规范生成客户端                                                    | 不手改生成物，不从内部 Activity 生成公共 HTTP                                              |

以下目录为职责位置，不要求预建尚未使用的业务模块。新增 adapter 的具体名称在选定协议后确定；新消费接口只含实际所需的方法，不创建通用 SDK、重复 DTO 或转发空层。

## 4. 18 个可验证子任务

### M1-12.01 合同冻结与当前事实同步（M，必做）

- **归属/依赖：**M1-12；关联 E-07/08/11/21/24/30；前置为可审阅的生成设计。
- **输入：**生成设计、DES-02/03/04/07/33、当前源码与 BACKLOG；单一工作区固定 producer、catalog 管理及人工核对仍要求 admin。
- **可先固定的内部合同：**同步 completed、已执行保护、调用身份/attempt、CAS 发送权、受限收据/manifest、暂存接管、恢复/清理、队列/载荷与历史兼容；这些只依现有业务事实，不等待供应商选型。
- **分时确定的外部合同：**供应商/线路/模型的 schema 与 usage 在具体模型适配前固定，价格单位/FX 在具体计价前固定，真实审核在审核接线前固定；预算与真实凭据安全注入在真实调用前确定。操作者管理授权在管理配置/manual 接线前确定；未定不得默认授予 admin。
- **产物/职责：**形成合同决策清单；同步相应 Design、PROJECT 与 PLN-01/04/05/18/21/27/32。清除“保留 Python”、`agent/app` 运行职责、旧登录前置和 E-36-09/10 落后状态；保留历史记录及未完成边界。
- **关键决策：**明确本机操作者授权合同和管理命令入口。不得恢复登录、把 producer 自动升为 admin 或绕过现有命令鉴权。未决定前管理配置/manual 接线不放行，普通 producer 权限保持。
- **Red/验证：**文档任务无需伪造核心逻辑测试；用 `rg` 对照实际类型、状态与角色，列出待替换的过时断言。拟定状态/发送/费用失败样例供 .02 起先写测试。
- **验收/退出：**技术合同与真实启用条件分开，职责/状态/可信证据来源一致；选定上游路径与固定 SHA 可追踪。供应商、模型、价格、审核、预算未定如实列出，不用默认值冒充确认。
- **真实条件：**此任务无需 secret、网络生成或付费；它不关闭 G2。

### M1-12.02 同步完成与持久收据结构（M，必做）

- **归属/依赖：**E-24-01、E-21-03；依 .01 技术合同。
- **输入：**现有异步四类 submit 结果、Operation 状态、provider_call 约束和数据库历史。
- **产物/职责：**`operation/domain`、`application/provider_call.go`、workflow 类型、`adapter/postgres/provider_call.go` 与追加 SQL migration；加入 `completed`、`dispatch_started_at`、受约束 receipt，禁止保存字节/secret/原始 body/签名 URL。
- **Red：**在 `backend/tests/operation/` 新增 `TestM1SyncCompletedContract`；先证明同步完成被现有校验拒绝。覆盖 completed 无 task ID、accepted 缺 task ID、收据身份/大小边界和历史载荷兼容。
- **验收：**`go test -race ./tests/operation -run '^TestM1SyncCompletedContract' -count=1 -v`；隔离 PostgreSQL 执行 migration up/down/up，既有操作/账本行不变；活动字段同步 DES-03，不建独立 schema 文件。
- **退出/真实条件：**只形成状态与数据基础；completed 表示已执行，费用可核定为独立条件。无需真实供应商，不启用生成。

### M1-12.03 持久发送权与重复投递保护（M，必做）

- **归属/依赖：**E-24-01、E-21-03；依 .02。
- **输入：**工作流指定并已登记的 operation/action/attempt/request key、冻结模型和调用行。
- **产物/职责：**operation application 消费小接口与 PostgreSQL CAS；网络前以 `dispatch_started_at IS NULL` 取得发送权，事务不跨网络；Activity 重投递先读发送状态/收据。
- **Red：**新增 `TestM1DispatchClaim`；两个 Worker 并发、重复 claim、错项目/operation/key/attempt/model、claim 后崩溃无收据、新 attempt 前次未确认未提交等场景。
- **验收：**`go test -race ./tests/operation -run '^TestM1DispatchClaim' -count=1 -v`，必须真实 PostgreSQL CAS 只有一个赢家；每案同时断言发送次数、provider_call 与预留/账本状态。
- **退出/真实条件：**已发送无可信回执进入 unknown，不以租约/超时重新抢占付费发送；新 attempt 仅按已确定未受理合同放行。这是首个可开工技术切片，无供应商/secret/付费前置。

### M1-12.04 执行限定凭据读取与 Go 解封（M，必做）

- **归属/依赖：**E-07-04；依 .01/.02，可与 .03 并行。
- **输入：**现有公钥密文格式、key_id、provider/credential 原始 UUID AAD、操作冻结模型与凭据状态。
- **产物/职责：**catalog application 定义授权密文读取；执行 Activity 内短期解封；私钥仅注入执行 Worker，API 只用公钥，flow/media/browser 无私钥；构造函数注入并清理短期材料。
- **Red：**`backend/tests/catalog/` 新增 `TestM1CredentialUnseal`；临时 RSA 往返、AAD/nonce/tag/key_id 错误、停用/跨供应商/错操作读取、错误与日志脱敏。
- **验收：**`go test -race ./tests/catalog -run '^TestM1CredentialUnseal' -count=1 -v`；保留现有封装测试，核对角色注入与取消路径，不输出明文。
- **退出/真实条件：**临时密钥可验证技术；真实密钥来源/轮换/安全注入待运维合同，尚未授权读取真实凭据。

### M1-12.05 首个显式图片 HTTP 协议（M，必做）

- **归属/依赖：**M1-12、E-26-04；依 .03/.04 和首个协议字段合同。
- **输入：**先以候选协议及 fixture 验证受控 HTTP 基础；OpenAI Images manifest 只是候选来源。真实模型的参数/响应/usage 映射须先固定准确 schema，再做具体适配，不能用候选字段冒充所选网关合同。第一条限制一次、单产物、非流式文生图。
- **产物/职责：**operation provider adapter；一次受控 HTTP、JSON 构造、base64/URL 按实际模型解析、稳定安全错误；固定服务端线路及 HTTPS/域名/端口/IP/重定向/授权头/容量限制。
- **Red：**`backend/tests/operation/` 新增 `TestM1ImageProtocol`；禁止无条件 response_format、非法参数/容量、伪 MIME、错误 base64、取消、429/5xx/超时/解析失败、凭据跨来源转发与重投递。
- **验收：**`go test -race ./tests/operation -run '^TestM1ImageProtocol' -count=1 -v`，fixture transport 只发送一次；发送前错误为 not_submitted，发送后无法证明未受理为 unknown；不伪造 task ID。
- **退出/真实条件：**无外部选型可先关闭安全 transport/候选 fixture 的基础验收，具体模型映射仍待合同后验收；adapter 不自建轮询/结算/写画布。复制或改写时记录上游路径、固定 SHA、改造并更新 `THIRD_PARTY_NOTICES.md`；技术 fixture 不证明真实模型可用。

### M1-12.06 manifest-last 私有可靠收据（L，必做）

- **归属/依赖：**E-24-01、E-30-03；依 .02/.03，接口与 .05 对齐。
- **输入：**同一调用身份、冻结模型、产物字节/大小/MIME/SHA-256、脱敏请求标识、可信用量。
- **产物/职责：**私有存储暂存与 operation 收据记录；服务器派生 project/operation/action/attempt/sequence 键；先条件写产物、再条件写完整 manifest，独立标识成本证据是否完整。
- **Red：**新增 `TestM1ReceiptRecovery`；图成功 manifest 失败、manifest 成功 DB 失败、已有对象不同 SHA、孤立对象、缺用量、重复 Activity、manifest 篡改。
- **验收：**`go test -race ./tests/operation -run '^TestM1ReceiptRecovery' -count=1 -v`，隔离真实对象存储与 PostgreSQL 注入断点；恢复只补同一收据/DB，不增加供应商发送次数。
- **退出/真实条件：**Temporal 输出仅稳定受限引用；无完整 manifest 不能由对象存在推导成功/成本。未结算/manual/待接管/待审核不得普通 TTL 删除；终态且费用/正式对象已验证再按既有维护队列清理。无需真实付费生成。

### M1-12.07 接管暂存结果并原子建立候选（M，必做）

- **归属/依赖：**E-30-03；依 .06。
- **输入：**受限 manifest 引用与输出序号；同项目 Operation、摘要及探测要求。
- **产物/职责：**media `IngestInput` 新分支；media Worker 将可信暂存字节构造为现有 `Downloaded`，复用探测、正式对象验证、派生与原子 SaveOutput；不让浏览器指定对象键。
- **Red：**`backend/tests/media/` 新增 `TestM1StagedIngest`；跨项目/任意键/摘要篡改/伪 MIME/超容量拒绝，正式对象成功 DB 失败与重复接管。
- **验收：**`go test -race ./tests/media -run '^TestM1StagedIngest' -count=1 -v`；真实 MinIO/ffprobe/PostgreSQL 检查字节、SHA、元数据，重放仅一条候选和正式对象。
- **退出/真实条件：**接管失败只重试媒体阶段，不能再次生成；暂存物不能直接预览或选定。真实文件技术证据可来自 fixture，须标明不是供应商产物。

### M1-12.08 可信用量、冻结价格与同步人工恢复（L，必做）

- **归属/依赖：**E-08、E-11、E-21-03、E-24-03；依 .02/.06 与选定用量/价格合同，可并行准备 .05。
- **输入：**持久 provider_call、模型/价格版本、计价单位、币种/汇率/取整、reservation 与现有 finalizer。
- **产物/职责：**catalog/billing/operation：按已确认类别核算；费用可核定才 submitting → ingesting；缺证据走 unknown/reconciling/manual 并保持预留。同步 manual 通过同调用可信回执/审计账单补证据，不靠假 task ID 或管理员填写数字。
- **Red：**新增 `TestM1SyncCostAndManual`；缺失≠0、负数/未知或重复用量字段、缺价格/FX、溢出、重复结算、completed 费用为零仍拒绝 not_executed、无可信证据拒绝恢复。
- **验收：**`go test -race ./tests/operation ./tests/billing ./tests/catalog -run '^TestM1SyncCostAndManual' -count=1 -v`；真实 PostgreSQL 断言一条账本、余额/预留、执行保护及管理员鉴权。
- **退出/真实条件：**媒体/审核失败仍结算已知真实成本；未知费用不记零，不擅自让 ingesting 转 manual。模型计价无法表达先更新设计/price_unit，不启用；价格与预算未确认不付费。

### M1-12.09 真实审核与 ready/passed 门禁（M，必做）

- **归属/依赖：**E-30-04；依 .07、明确真实审核方式/失败合同；可与 .08 并行。
- **输入：**已接管素材与项目、真实审核协议及允许的保留/重试规则。
- **产物/职责：**media 审核 adapter 和持久状态，按原 Operation 恢复；只有 ready 且 passed 才进入可预览、选定、绑定读侧。
- **Red：**`backend/tests/media/` 新增 `TestM1ModerationGate`；服务不可用、拒绝、重试耗尽、重复回执、跨项目引用和尝试选定 pending/rejected 素材。
- **验收：**`go test -race ./tests/media -run '^TestM1ModerationGate' -count=1 -v`；fixture 证明失败门禁，真实审核接线另外保留真实回执，绝不回退 mock/pass。
- **退出/真实条件：**未选定真实审核不能过 G2；既有重试时限按已确定设计使用，不因供应商不可用绕过审核。

### M1-12.10 Worker 注册、同步工作流与历史回放（L，必做）

- **归属/依赖：**M1-12、E-21-03、E-24-03；依 .03～.09。
- **输入：**现有 Activity 名称/队列、Go Worker DI、真实旧历史（success/unknown/manual/cancel）与完整同步恢复合同。
- **产物/职责：**app 角色/组合根、operation workflow；新增执行职责与 `workflow.GetVersion` change ID，保留历史命令顺序和 mock；Wire 生成物由工具生成，不手改。
- **角色/注入合同：**真实执行使用现有 binary 的专门 worker/供应商队列配置，不新增服务仓库。当前 `role=all` 合并多个职责，不能向 all 注入真实供应商私钥并宣称 API/flow/media 隔离成立；all 与非执行角色不得加载该私钥或注册真实供应商执行 Activity。
- **Red：**`backend/tests/operation/` 新增 `TestM1SyncWorkflowReplay`；旧历史新增决定不兼容、Worker 重启、发送后超时、收据后崩溃、媒体/审核失败、取消未知费用及关闭新请求后的在途恢复；补 all/API/flow/media 不加载私钥、不注册真实执行，以及专门执行 Worker 可注册的负面/正面配置测试。
- **验收：**`go test -race ./tests/operation -run '^TestM1SyncWorkflowReplay' -count=1 -v`；实际 Temporal 历史回放及真实 Worker 注册证据，所有断点不增加生成发送次数；执行 PROJECT 中 Wire 一致性命令。
- **退出/真实条件：**默认关闭；只有所有真实配置满足才开放新请求，关闭后仍能恢复/核对在途请求。无历史/Temporal 条件只报告单元证据，不报告回放通过。

### M1-12.11 公共模型、报价确认与任务契约链（L，必做）

- **归属/依赖：**E-08-02、E-21-02、E-24-02、M1-04/06/07；依 .01 权限决策、.02/.08 合同；与 .10 可并行接线。
- **输入：**已注册业务命令、模型读侧、自由生成报价/确认、任务读取/取消/manual；确认前不执行、不预留的现有合同。
- **产物/职责：**对应模块 `adapter/http`、路由/DTO/swag 注解、`backend/docs`、在线生成 `frontend/src/gen/api`；普通 producer 与管理命令权限分开。
- **Red：**`backend/tests/operation/` 新增 `TestM1GenerationHTTP`，catalog 补相应模型读侧测试；同键同体回执/异体 422、错误 Origin、归档/停用/跨项目、确认重放、producer 无管理权；只注册的公共 HTTP 才入规范。
- **验收：**`go test -race ./tests/operation ./tests/catalog -run '^TestM1GenerationHTTP' -count=1 -v`；从运行后端 `/swagger/doc.json` 生成客户端并验证一致；不可用时失败，不用旧文件冒充新契约。
- **退出/真实条件：**HTTP 可读真实任务/失败阶段，未确认零发送；不暴露凭据、收据原文或 provider 内部端点。未决定本机管理授权，管理入口保持未开放而非自动升权。

### M1-12.12 首次文生图真实与最小产品闭环（L，必做）

- **归属/依赖：**M1-12、E-21-04、E-24-04、E-36；依 G1 及 G2 的全部真实条件。首链固定 `target_type=free`，不隐含要求镜头服务。
- **输入：**当前真实项目/画布、已发布单个模型、明确预算、真实凭据安全注入、真实审核与价格；一次单产物自由文生图请求。
- **产物/职责：**现有 operation/canvas/workbench 入口最小接线：模型参数 → 报价 → 明确确认 → 任务恢复 → 候选预览 → 用户采用同项目 ready/passed `media_asset` → 现有 `AddNodes` 持久资源绑定。该命令不承接任务或镜头选定事实；以后选择 shot_frame/shot_take 目标才按 E-22 接真实 shot 正式选定。
- **Red：**后端 operation/media/canvas 补 `TestM1ImageVertical` 系列；就近组件测试补未确认不发送、确认重放、刷新/错误恢复、采用绑定幂等、迟到结果不得覆盖已改资源引用/删除节点/切换项目、焦点返回。
- **验收：**技术：`go test -race ./tests/operation ./tests/media ./tests/canvas -run '^TestM1ImageVertical' -count=1 -v`。真实：受控调用一次，记录真实请求回执、字节/SHA/探测/审核/可信 usage/账本；重启与媒体失败恢复仅重试已有结果。产品：真实浏览器确认、刷新、预览、用户采用并 AddNodes，刷新后媒体资源绑定仍存在。
- **退出/真实条件：**三类证据分别通过才关闭首条链；测试重放不再次付费。供应商/预算/审核缺项保持待真实验收，不触发调用；一次图片通过不等于完整 MVP。

### M1-12.13 参考图与蒙版编辑（M，条件扩展）

- **归属/依赖：**E-26、E-39-04/05/06、E-19；依 .12 和实际模型编辑支持证据，既有参考输入/遮罩合同就绪。
- **输入：**同项目 ready/passed 素材、授权/TTL、已确定 multipart 字段、输入顺序/用途、尺寸与 mask alpha/坐标语义。
- **产物/职责：**选定图片 adapter 的 `/images/edits` 分支、operation 输入冻结、media 输入取得与现有编辑入口；不凭模型名或 manifest 认定支持。
- **Red：**新增 `TestM1ImageEditInputs`；错蒙版尺寸/alpha、顺序、跨项目、过期/撤权、停用模型、容量限制，均发送前拒绝；重新报价后的指纹必须变化。
- **验收：**`go test -race ./tests/operation ./tests/media -run '^TestM1ImageEditInputs' -count=1 -v`；支持的模式另行按预算真实验证，回执/费用/产物/用户采用证据同 .12。
- **退出/真实条件：**不支持则明确禁用并记录原因；mask 是付费编辑，不归免费图片工具。此任务和任何新增候选不阻塞视频/音频/文本。

### M1-12.14 既有规则提示词与关键帧裁剪合同（M，按既有需求复用）

- **归属/依赖：**E-27-03/04/06、E-28-03/04/05、E-26-03/05/06、E-30-03；规则依用途映射/输入冻结，裁剪依上传/同项目素材及相应 Design 决策。
- **输入：**已设计 Go 模板、参考用途/顺序、原始提示词/快照；上传图片、前端裁剪框和服务端 `CropImage` 合同。
- **产物/职责：**相应 storyboard/operation 规则编译与预览，用户手动采用后冻结；media Worker 做关键帧裁剪，继续复用缩略图/代理派生。只创建实际被使用的职责模块。
- **Red：**新增 `TestM1RulePromptAndCrop`；引用序号与输入顺序一致、模板无覆盖原文、相同输入同快照、越界/小尺寸裁剪、跨项目/未 ready 素材、重复处理。
- **验收：**`go test -race ./tests/operation ./tests/media -run '^TestM1RulePromptAndCrop' -count=1 -v`；浏览器验证预览/手工采用与上传裁剪，真实处理字节/摘要可查，零 LLM 发送。
- **退出/真实条件：**规则拼接不是 AI 优化器；上传合同/裁剪设计未确定时对应部分待定。视频抽帧归 .15/E-39-03；宫格拆分、PNG 标注、智能引用、AI 优化、抠图/超分另需设计与授权，不列必做前置。

### M1-12.15 首条视频能力与异步恢复（L，独立纵向切片）

- **归属/依赖：**E-27/28、E-39-03、E-24；依 G2 共享底座、视频协议/计价/审核、镜头与参考素材的实际业务前置；不依 .13/.14 候选或 .16/.17。
- **输入：**实际视频模型支持的图生视频/参考模式之一、duration/resolution 等限制、真实 task ID、请求键查询/取消与未受理证据；不能从 supports_query 推导按键查询可靠。
- **产物/职责：**provider adapter、workflow query/cancel 消费合同按 operation/attempt/冻结线路/凭据补身份；media 接管视频/代理/封面，按 E-39-03 独立复用 `media.ExtractFrame`；现有视频入口最小报价/任务/预览/选定接线。
- **Red：**新增 `TestM1VideoVertical`；accepted 无 task ID、query 5xx/not_found、回执丢失、取消费用未知、链接过期、关键帧改选使依赖过期与跨项目输入。
- **验收：**`go test -race ./tests/operation ./tests/media -run '^TestM1VideoVertical' -count=1 -v`；not_found 不等于 confirmed_not_exist，恢复不重发；真实文件 SHA/ffprobe/完整解码、审核、费用及用户选定分别留证。
- **退出/真实条件：**只关闭选定模式；视频预算/真实模型未定仅技术测试，未支持的模式禁用，不能把样例视频预览计作供应商验收。

### M1-12.16 首条音频/TTS 能力（L，独立纵向切片）

- **归属/依赖：**E-29-01～06、E-24；依 G2 共享底座、音频协议/价格/审核；若选台词配音，另依真实 episode UUID/line_key/音色与台词业务数据。
- **输入：**选定 TTS 字段/音色/格式、同步或异步合同、duration/chars 等实际计价单位、可信 usage；逐句覆盖参数与冻结文本。
- **产物/职责：**Go 音频 adapter、operation 调用/费用、media 原音频/波形探测；按 E-29 候选与选定记录接线，文本/音色改变触发原有过期规则；现有入口最小报价/播放/采用。
- **Red：**新增 `TestM1AudioVertical`；组合 target_key、参数 hash、格式错误、时长/字符数费用缺失、断连未知、重放、台词改变后旧产物迟到。
- **验收：**`go test -race ./tests/operation ./tests/media -run '^TestM1AudioVertical' -count=1 -v`；真实字节/SHA/音频探测与可播放、审核、可信计价、正式选择；不把生成时长覆盖原台词事实。
- **退出/真实条件：**不能照搬图片零费用或假定一种计价；音频模型/预算未定保持待真实验收，不等待视频或文本全部完成。

### M1-12.17 首条文本/结构化能力（L，独立纵向切片）

- **归属/依赖：**M1-12；具体用途关联原 E-12/13 等文本业务任务，按合同冻结首个目标；若选择 E-13 episode_parse，则依 E-12 的真实剧本/分集，不能把完整 M2 当已完成。
- **输入：**选定文本协议、token 类别/价格/预算与输出 schema；结构化解析另含规范化原文与偏移、稳定键、每句台词一次及待处理行规则。
- **产物/职责：**单次 Go 文本执行 adapter 与现有 Operation；结构结果先校验，再由原 `flow.ApplyStructuredResult` 对应业务写 candidate，经用户确认变正式版本；不套用媒体 `media.Ingest`，不让模型直接写正式业务对象。原文本入口最小报价/候选审阅/采用接线。
- **Red：**新增 `TestM1TextVertical`；非法 schema/不存在的原文位置/重复遗漏台词、长文本限制、缺 token usage、未知费用、重复候选、迟到结果不得覆盖已采用版本。
- **验收：**`go test -race ./tests/operation -run '^TestM1TextVertical' -count=1 -v`，相应实际业务模块补测试；真实文本回执/可信 usage/费用、原文追溯及用户审阅分别留证，业务质量按原 TST-03 指标验收。
- **退出/真实条件：**文本用途/协议未确定不造通用生成端点；不把整套 E-37 Agent 会话/Harness 纳入本任务，不恢复旧 Python；Agent 上限金额不是本任务默认预算。不等待视频或音频全部完成。

### M1-12.18 能力通过后的全面页面重构（L，最后进行）

- **归属/依赖：**E-21/22/24/26/27/28/29/30/36 相应前端任务；依对应已通过的能力与稳定公共合同，不由 UI 反推新增业务需求。
- **输入：**LibTV 参考结构、已保留的服务页面、真实模型/任务/候选/费用状态、各纵向切片的最小入口与失败证据。
- **产物/职责：**既有 `frontend/src/components/<业务>/`、App Router 页面；复用模型参数、报价确认、任务/媒体/候选组件和服务端项目范围；只有实际可用能力可执行，未完成能力诚实显示状态。
- **Red：**就近组件测试先覆盖真实空/失败/重试、未确认禁发送、权限/项目切换、刷新恢复、候选改选与弹窗焦点；浏览器覆盖明暗主题、桌面/平板/移动及键盘操作。
- **验收：**按 §5 前端门禁；读取 Vercel React/Next 技能及安装版本文档，审查请求瀑布、客户端边界、缓存范围、可访问性。截图仅证明界面，需真实项目→确认→任务→候选→选择的产品证据。
- **退出/真实条件：**先在可用 API 上完成页面，再按原九场景逐项记录通过/待验收；不新增积分商城、社区或会员等参考图中尚未进入需求的功能，不自动发布。

## 5. 执行与证据要求

所有任务报告均列：实际输入与配置范围、改动文件、Red 失败与 Green 命令、非 Skip 的结果、残余条件、Git 范围。阶段证据至少包括 Operation 状态、供应商发送次数、provider_call、reservation/ledger；结果还包括字节/SHA、探测、审核和权限。首 free 链记录用户采用的 ready/passed `media_asset`、AddNodes 命令回执与刷新后的持久资源引用；正式 shot 选定只在对应目标接入后记录真实 shot 指针与选定版本。

Go 门禁在 `backend/` 按修改范围先定向、后适用全量执行：`gofmt`、`goimports`、`go vet ./...`、`golangci-lint run ./...`、`go test -race ./...`、`govulncheck ./...`，以及适用的 swag/Wire 生成一致性。业务测试统一放 `backend/tests/<模块>/`，不用生产包内镜像测试凑覆盖。

真实 PostgreSQL、对象存储与 Temporal 集成需隔离环境。现有 `LV_TEST_*` 条件未满足产生 Skip 必须列为未验收；现有 `workflow_live_integration_test.go` 仍要求已移除的 Agent Worker，应随 .10 更新其执行前置，不设置旧标志伪装执行端存在。不读取或复制已有 `.env`；配置由获授权操作者安全注入，输出只记录配置是否满足。

前端在 `frontend/` 执行 `pnpm exec vitest run --maxWorkers=4`、`pnpm exec eslint . --max-warnings=0`、适用文件 `pnpm exec prettier --check`、`pnpm exec next typegen`、`pnpm exec tsc --noEmit`、`pnpm exec next build`。契约变更先运行后端在线 Swagger，再执行 `pnpm exec openapi2ts`；遵循 `frontend/openapi2ts.config.cjs`。浏览器验证按实际交互补充，不能仅用 lint/build 关闭产品验收。

证据分三类保存：**技术证据**可用 fixture/临时密钥/隔离真实基础设施；**真实集成证据**必须来自已授权供应商/审核/产物/费用；**产品证据**必须经过当前页面与正式命令。每类记录待确认和 Skip，不能彼此替代。完整九场景以 [PLN-01](01-实施路线与交付计划.md)、[TST-02](../test/02-需求追踪矩阵.md) 为准，首条文生图通过只关闭对应切片。

实施中每次只提交可独立验证的范围，遵守 AGENTS 的中文提交格式与 allowlist；是否提交/推送/发布依当次用户授权。本次计划未实现上述任务、未新增供应商生产代码、未访问真实凭据、未执行付费测试。

## 6. 自审重点与下一步

- 首个代码切片推荐 .02/.03：先失败的同步合同/发送权测试，再追加迁移和消费方端口；用真实 PostgreSQL 证明只有一个发送赢家。无需供应商、secret 或生成网络。
- 未确定真实模型时可以推进 .04 临时密钥与 .06/.07 可靠收据技术；协议/计价/审核相关任务按各自合同放行。不能用“HTTP 200”、模型名称、mock 费用或样例媒体替代 G2。
- .13/.14 的新增候选不是 .15/.16/.17 的前置；全面页面重构 .18 最后进行，各能力最小入口仍随纵向切片交付。
- 所有追加数据库/Workflow 改动保留历史；不删除或重置数据，不对未知请求重新生成，不在关闭新请求时关闭在途恢复。
- 首次真实调用前检查准确线路、模型参数、usage/价格/FX、审核、预算和安全凭据注入均已确定；尚缺任何项只交付技术证据，并指出缺项。
