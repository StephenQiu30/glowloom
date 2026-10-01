# BeefTV 能力迁移实施计划

2026-10-01 最新执行范围：用户确认清空全部现有前端实现，保留技术栈、依赖、锁文件及工程配置，本轮只清理，不创建替代页面；当前工作台和画布入口不可启动。供应商生成子任务继续暂停，Go 内部底座、公开契约、SQL 和业务数据保留；下文涉及现有前端目录、入口及浏览器接线的计划须在重建设计确定后重新落实。此前画布与上传证据见 E-36，只证明清理前对应提交的历史状态，不改变 M1-12/G1 未通过状态，清理范围见 [前端实现清理](../design/前端实现清理设计.md)。

| 项             | 内容                                                                                                                                                              |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 状态           | 分阶段实施；.01 首片内部合同已固定，.02/.03 数据/DTO/发送权/执行保护技术通过；G1 未全过，不启用真实生成                                                           |
| 用户确定的方向 | 先迁能力、后全面页面；本机 Codex imagegen → 火山 Seedream 图片/Seedance 视频 → OpenRouter 扩其他模型                                                              |
| 代码基线       | 本地 main：`7d024416`/`3d526fb2` 工作台测试与实现；本轮 `9c9fffe7` 测试、`38a2a016` 证据/发送权实现；均不代表真实生成完成                                         |
| 设计来源       | [生成能力迁移设计](../design/BeefTV生成能力迁移设计.md)、[增量评估](../prd/BeefTV增量能力与服务适配评估.md)、[执行端清理设计](../design/Agent服务目录清理设计.md) |
| 固定来源       | 本机 CLI `0.159.2` 公共 schema；HTTP 参考 BeefTV `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98`；画布仍为 `0d9e9f48d407570cd431ad9730cdd522b06810c0`                  |
| 任务归属       | 拆解现有 M1-12，并关联 E-07/08/11/21/22/24/26/27/28/29/30/39；不新增重复 Epic，不增加 BACKLOG 任务总数                                                            |
| 仍待确定       | 本机运行/账号/产物获取、实际模型与参数、额度/可信费用、真实审核、本机操作者授权、真实测试预算；后续 API 按各自 schema 固定合同                                    |

**目标：**先在 Lanverse 现有 Go 执行边界接本机 Codex 内置 imagegen，交付一次文生图的完整产品链，再接火山与 OpenRouter，并扩展实际支持的参考图/蒙版、既有提示词与图片处理、视频、音频、文本。BeefTV 继续提供可追踪的协议映射参考。

**架构：**Operation 持有冻结输入、调用身份和状态；catalog 持有模型/价格及 API 路线的授权密文；执行 adapter 只执行一次受控本机 turn 或 HTTP；media 接管、探测、审核并建立候选；现有 billing/finalizer 原子核定费用。Workflow 决定顺序，组合根显式注入依赖。

**工具链：**保留 Go、PostgreSQL、Temporal、私有对象存储、Next.js、TanStack Query、shadcn/Radix，以及 Handler/DTO → swag → 在线 Swagger → `@umijs/openapi` 的契约链。

**当期证据：**2026-10-01，新增 23 个顶层专项测试（10 个合同、12 个真实 PostgreSQL、1 个独立迁移）通过；全后端 343 个顶层测试通过、157 个外部条件 Skip，适用 vet/lint/race/govulncheck 通过（1 个未调用模块告警），见 [首片验收](../acceptance/M1-12-同步调用证据与发送权验收.md)。.02/.03 只交付数据/DTO、发送权和已执行保护；Operation 同步转换、成本与 Workflow 仍待 .08/.10，G1 只部分通过，真实生成/审核/计价/产品未通过。已有服务数据库未迁移、进程未重启；部署本轮实现前先应用 `202610010010_provider_dispatch_receipt`。

## 1. 范围与执行规则

1. 本文规划 18 个 `M1-12.xx` 子任务，编号只用于跟踪迁移切片；原 Epic 的完成定义与完整 MVP 九场景保持。当前 UI 提交不追认 M1-12、E-21/24 或真实生成完成。
2. 用户已授权迁移，.01 首片内部合同已固定并实施；其余步骤仍先落实对应技术合同，不把接入方向当作具体模型/价格/权限承诺。真实条件未定不阻止模拟 stdio、fixture transport、临时密钥和隔离基础设施测试。
3. 供应商启用和真实调用另有明确条件。接受设计或本计划不自动授权付费、发布、读取真实 secret。本机账号保持 Codex 自有管理，不复制 OAuth secret、不默认免费、不无声回退付费 HTTP；执行测试默认使用模拟进程/transport、临时密钥和隔离数据，不记录凭据正文、原始 body 或可访问的签名 URL。
4. 不另建 BeefTV 服务、任务引擎或数据库，不迁 Wails/SQLite、manifest 解释器、动态插件平台、3D、时间线、FFmpeg.wasm；不恢复已删除的 Python 服务或整套旧 Agent。
5. 必做范围取自已接受的需求与设计；已有文档标为草案或待确认的部分须在对应任务入口处理，不能仅凭存在条目认定已接受。参考图/蒙版属于模型支持后放行的扩展；宫格拆分、PNG 标注、智能引用、AI 提示词优化器仍是候选。
6. `S/M/L` 表示相对工作量：S 为单一边界，M 为数个协作边界，L 为跨状态/持久化/产品闭环。它们不是工期或日期承诺；L 任务按内部验收步骤逐项提交证据，过大时继续拆步骤，不增加重复 Epic。
7. 每个代码步骤遵循 Red → Green → Refactor。.02/.03 的真实 Red/Green 见首片验收；未实施步骤的新增测试名仍是拟定入口，先写测试并证明缺少行为导致失败。没有匹配测试或全部 Skip 不能计作通过。

## 2. 顺序、依赖与六个门禁

接入路线为：**本机 Codex 图片 → 火山 Seedream 图片/Seedance 视频 → OpenRouter 扩其他模型**。能力审阅顺序仍为文生图 → 参考图/蒙版 → 既有规则提示词与图片处理 → 视频/音频/文本 → 全面页面；条件扩展或候选工具不能变成其他能力的硬前置。

| 门禁                | 任务             | 输入与放行条件                                                                             | 产物与退出条件                                                                                                                           |
| ------------------- | ---------------- | ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- |
| G0 合同就绪         | .01              | 技术边界、当前单工作区权限及历史兼容方案可审阅                                             | 明确已确定/待确定项；技术底座可开工；真实启用仍关闭                                                                                      |
| G1 文生图技术底座   | .02/.03/.05～.11 | 对应合同已确定；隔离 PostgreSQL/对象存储/Temporal；模拟 stdio/fixture；API 路线另需 .04    | 并发发送、可靠收据、媒体、费用、审核失败路径、历史回放、公共契约均有真实执行的技术证据；不宣称真实供应商通过                             |
| G2 首条真实文生图   | .12              | G1；首链 target_type=free；真实账号/线路/模型/参数/费用/审核；API 凭据按路线；明确预算上限 | 单次真实生成与重启恢复证据；现有入口报价确认、刷新、预览、用户采用 ready/passed 素材、持久 AddNodes 资源绑定通过；技术/真实/产品证据分列 |
| G3 图片扩展复用     | .13～.14         | G2 后优先审阅；编辑取决于实际模型；规则提示词/裁剪取决于各自已有素材与上传合同             | 支持的扩展独立验收；不支持的模式明确禁用；候选功能保持候选，不阻塞 G4                                                                    |
| G4 其他能力纵向交付 | .15/.16/.17      | G2 的共享底座；各能力自己的已确定协议、业务目标、计价、审核、预算与数据前置                | 视频、音频、文本各自独立取得技术/真实/最小产品证据；三者可并行，不相互等待全部完成                                                       |
| G5 全面页面重构     | .18              | 对应能力已过 G2/G3/G4，公共 API 和实际产品状态稳定                                         | 页面只展示已可用能力，适用浏览器交互和质量门禁通过；完整 MVP 仍按原九场景独立验收                                                        |

当期 G0 首片内部合同已固定，G1 仅 .02/.03 的数据/DTO/发送权/执行保护技术通过。.04 是火山/OpenRouter API 路线的必要前置，不是 Codex 本机桥的技术前置。可并行推进 .05 模拟 stdio、.04 临时密钥、.06/.07 可靠收据/媒体；.08/.09 按各自合同放行，.11 公共接线与 .10 注册回放可并行准备。最终 G1 合并验证，不以单元通过替代跨边界证据。

首条图片完成后，.13 和 .14 按适用需求推进，.15/.16/.17 可并行。每条能力的**现有入口最小接线**随该能力产品验收交付；.18 后置的是全面页面重构，不是所有界面接线。

## 3. 现有接口和职责定位

| 复用位置                                                                                 | 已有事实                                                            | 迁移时的边界                                                                           |
| ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| `backend/internal/operation/adapter/workflow/provider_contract.go`                       | 已追加 completed/receipt/usage DTO；query/cancel 仍为旧消费合同     | completed 不用伪 task ID；Operation 同步转换仍未接入；实际消费时再扩 query/cancel 身份 |
| `backend/internal/operation/application/provider_call.go`                                | 已有 v1 claim/封闭收据/已执行保护；同步成本保持 NULL                | 执行 adapter 消费持久身份与发送门禁；费用恢复到 .08；不靠进程内 map                    |
| `backend/internal/catalog/adapter/credentialseal/sealer.go`                              | RSA-OAEP-SHA256 + AES-256-GCM 公钥封装                              | API 路线复用密文/原始 UUID AAD，解封尚待实现；本机桥不复制 Codex OAuth secret          |
| `backend/internal/media/application/ingest.go`                                           | `IngestInput`、`Downloaded`、Downloader/ObjectStore/Prober/Renderer | 在 media Worker 解析受限暂存引用，再复用字节探测和原子候选登记                         |
| `backend/internal/operation/application/create_free_quote.go`、`confirm_single_quote.go` | `CreateFreeQuoteInput`、`ConfirmSingleQuoteInput` 与内部命令        | `free` 表示自由生成目标，不是免费费用；复用冻结价格/输入和确认，不另造报价系统         |
| `backend/internal/app/role.go`、`roles_wiring.go`                                        | 当前 Worker 队列只有 flow/media                                     | 现有角色中显式加入执行职责；不得把 mock 队列绑定真实供应商                             |
| `frontend/openapi2ts.config.cjs`、`src/gen/api/`                                         | 在线规范生成客户端                                                  | 不手改生成物，不从内部 Activity 生成公共 HTTP                                          |

以下目录为职责位置，不要求预建尚未使用的业务模块。新增 adapter 的具体名称在选定协议后确定；新消费接口只含实际所需的方法，不创建通用 SDK、重复 DTO 或转发空层。

## 4. 18 个可验证子任务

### M1-12.01 合同冻结与当前事实同步（M，必做）

- **归属/依赖：**M1-12；关联 E-07/08/11/21/24/30；前置为可审阅的生成设计。
- **输入：**生成设计、DES-02/03/04/07/33、当前源码与 BACKLOG；单一工作区固定 producer、catalog 管理及人工核对仍要求 admin。
- **可先固定的内部合同：**同步 completed、已执行保护、调用身份/attempt、CAS 发送权、受限收据/manifest、暂存接管、恢复/清理、队列/载荷与历史兼容；这些只依现有业务事实，不等待供应商选型。
- **当期状态：**首片 v1/legacy、完整冻结身份、attempt、CAS、单图 typed receipt、4096 字节/32 MiB 内部上限和已执行保护已固定并技术通过；这些范围值不代表供应商承诺，后续状态/费用/注册合同仍按对应步骤落实。
- **分时确定的外部合同：**本机账号能力、产物路径/格式/编码和实际模型先真实核验；各 API 路线按自己的 schema 与 usage 固定。费用/额度映射、价格单位/FX 在计价前固定，真实审核在审核接线前固定，预算/安全注入在真实调用前确定。管理授权在配置/manual 接线前确定，不默认授予 admin。
- **产物/职责：**形成合同决策清单；同步相应 Design、PROJECT 与 PLN-01/04/05/18/21/27/32。清除“保留 Python”、`agent/app` 运行职责、旧登录前置和 E-36-09/10 落后状态；保留历史记录及未完成边界。
- **关键决策：**明确本机操作者授权合同和管理命令入口。不得恢复登录、把 producer 自动升为 admin 或绕过现有命令鉴权。未决定前管理配置/manual 接线不放行，普通 producer 权限保持。
- **Red/验证：**文档任务无需伪造核心逻辑测试；用 `rg` 对照实际类型、状态与角色，列出待替换的过时断言。拟定状态/发送/费用失败样例供 .02 起先写测试。
- **验收/退出：**技术合同与真实启用条件分开，职责/状态/可信证据来源一致；选定上游路径与固定 SHA 可追踪。供应商、模型、价格、审核、预算未定如实列出，不用默认值冒充确认。
- **真实条件：**此任务无需 secret、网络生成或付费；它不关闭 G2。

### M1-12.02 同步完成与持久收据结构（M，必做）

- **归属/依赖：**E-24-01、E-21-03；依 .01 技术合同。
- **输入：**现有异步四类 submit 结果、Operation 状态、provider_call 约束和数据库历史。
- **产物/职责：**`application/provider_call.go`、workflow DTO、`adapter/postgres/provider_call.go` 与追加 SQL migration；加入 completed、dispatch timestamp、受约束 receipt，禁止保存字节/secret/原始 body/签名 URL。Operation 同步转换在 .08/.10 接入，不改变当前 mock 顺序。
- **当期状态：**数据/DTO 与单图 typed receipt 技术通过，completed 的同步成本保持 NULL；4096 字节收据、1～32 MiB PNG/JPEG/WebP 和完整身份均受校验。有任何 dispatch/receipt 证据时 down 原子拒绝，不能删执行事实；可靠对象 manifest 尚未交付。
- **Red：**在 `backend/tests/operation/` 新增 `TestM1SyncCompletedContract`；先证明同步完成被现有校验拒绝。覆盖 completed 无 task ID、accepted 缺 task ID、收据身份/大小边界和历史载荷兼容。
- **验收：**`go test -race ./tests/operation -run '^TestM1SyncCompletedContract' -count=1 -v`；隔离 PostgreSQL 执行 migration up/down/up，既有操作/账本行不变；活动字段同步 DES-03，不建独立 schema 文件。
- **退出/真实条件：**只形成状态与数据基础；completed 表示已执行，费用可核定为独立条件。无需真实供应商，不启用生成。

### M1-12.03 持久发送权与重复投递保护（M，必做）

- **归属/依赖：**E-24-01、E-21-03；依 .02。
- **输入：**工作流指定并已登记的 operation/action/attempt/request key、冻结模型和调用行。
- **产物/职责：**operation application 身份/收据合同与 PostgreSQL CAS；消费接口在 .05 首次使用时定义，网络前以 `dispatch_started_at IS NULL` 取得发送权，事务不跨网络；Activity 重投递先读发送状态/收据。
- **当期状态：**v1 marker + CAS、legacy NULL 不视为未发送、完整项目/操作/attempt/request/model/price 身份、新 attempt 及唯一调用、终态只读重放、completed 费用 NULL/零/旧人工事件保护已技术通过。未注册真实执行，不以数据库赢家推导供应商成功。
- **Red：**新增 `TestM1DispatchClaim`；两个 Worker 并发、重复 claim、错项目/operation/key/attempt/model、claim 后崩溃无收据、新 attempt 前次未确认未提交等场景。
- **验收：**`go test -race ./tests/operation -run '^TestM1DispatchClaim' -count=1 -v`，必须真实 PostgreSQL CAS 只有一个赢家；每案同时断言发送次数、provider_call 与预留/账本状态。
- **退出/真实条件：**已发送无可信回执进入 unknown，不以租约/超时重新抢占付费发送；新 attempt 仅按已确定未受理合同放行。本轮技术底座已通过，.05 消费该门禁；实际 unknown 状态/恢复仍待后续接线。

### M1-12.04 API 执行限定凭据读取与 Go 解封（M，API 路线必做）

- **归属/依赖：**E-07-04；依 .01/.02，可与 .05 本机桥并行；火山/OpenRouter API 必须完成此步，Codex 本机桥不依此步。
- **输入：**现有公钥密文格式、key_id、provider/credential 原始 UUID AAD、操作冻结模型与凭据状态。
- **产物/职责：**catalog application 定义授权密文读取；执行 Activity 内短期解封；私钥仅注入执行 Worker，API 只用公钥，flow/media/browser 无私钥；构造函数注入并清理短期材料。
- **Red：**`backend/tests/catalog/` 新增 `TestM1CredentialUnseal`；临时 RSA 往返、AAD/nonce/tag/key_id 错误、停用/跨供应商/错操作读取、错误与日志脱敏。
- **验收：**`go test -race ./tests/catalog -run '^TestM1CredentialUnseal' -count=1 -v`；保留现有封装测试，核对角色注入与取消路径，不输出明文。
- **退出/真实条件：**临时密钥可验证技术；真实 API 密钥来源/轮换/安全注入待运维合同，尚未授权读取真实凭据。本机 Codex 账号由原客户端管理，不复制 OAuth secret 充当 API 密钥。

### M1-12.05 首个本机 Codex 图片执行协议（M，必做）

- **归属/依赖：**M1-12、E-26-04；依 .03 和本机执行合同，不以 .04 为技术前置；后续火山/OpenRouter API 另需 .04。
- **输入：**CLI 0.159.2 公共 schema 已核对 `modelProvider/capabilities/read` 的 imageGeneration、thread/start → turn/start → item/completed；没有直接 images/generate RPC。result 仅声明 string，编码未知，savedPath 可空；按版本逐项核对需 experimentalApi 的方法/字段，不将整个 app-server 标为实验协议。运行/账号/实际模型/产物/费用尚待验证。
- **产物/职责：**operation 执行 adapter 在本机 Worker 管理 stdio app-server，先不启动推理地探测能力，使用专属任务目录及 thread/turn/item 与 operation/attempt 关联；取得持久发送权后只启动一个生成 turn。验证受限 artifact 路径、真实格式/容量/SHA，再接 .06；进程/reader 有取消、退出和等待，不默认 result 是 base64 或 savedPath 必有值。
- **Red：**`backend/tests/operation/` 新增 `TestM1CodexImageProtocol`；能力缺失/不明、错关联/迟到/重复 item、取消/EOF/超时、未知 result 编码/空路径、越界路径/伪格式/超容量、重投递与进程退出。断线 unknown 不再次 turn/start，不自动切其他供应商或付费 HTTP。
- **验收：**`go test -race ./tests/operation -run '^TestM1CodexImageProtocol' -count=1 -v`；模拟 stdio 证明只启动一次生成 turn，发送前错误为 not_submitted，发送后无法证明未受理为 unknown；恢复只读取本任务历史/可靠收据，不伪造 task ID，fixture 不启动真实推理。
- **退出/真实条件：**公共 schema 只证明协议存在。真实账号权限、产物取得、实际模型和费用/额度未核验时保持关闭，不默认免费、不复制 OAuth secret。随后 Seedream/Seedance/OpenRouter 按各自 schema 适配，HTTP 的固定线路/HTTPS/域名/IP/重定向/授权头/容量边界保持；BeefTV Images 仅作 HTTP 参考，实际复制改写再登记上游 SHA/路径与 notices。不自建轮询/结算/画布或通用 SDK。

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
- **输入：**持久 provider_call、实际模型/价格版本、可信用量、计价单位/币种/汇率/取整、reservation 与 finalizer。本机 Codex 额度与本项目费用/预留的映射另需明确，token usage 不自动等于图片用量/费用，本机不默认零费。
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

- **归属/依赖：**M1-12、E-21-03、E-24-03；依 .03/.05～.09，API 路线另依 .04。
- **输入：**现有 Activity 名称/队列、Go Worker DI、真实旧历史（success/unknown/manual/cancel）与完整同步恢复合同；本机执行另核对 stdio 进程所有权、关联和安全产物边界。
- **产物/职责：**app 角色/组合根、operation workflow；新增执行职责与 `workflow.GetVersion` change ID，保留历史命令顺序和 mock；Wire 生成物由工具生成，不手改。
- **角色/注入合同：**真实执行使用现有 binary 的专门 worker/供应商队列配置，不新增服务仓库。本机桥只在有 Codex CLI/任务目录访问权的执行 Worker 启用，不默认容器或别的机器能读本机产物。API 路线的 all/API/flow/media 不加载供应商私钥或注册真实执行，不以 role=all 的合并职责宣称隔离成立。
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
- **输入：**当前真实项目/画布、已发布实际模型、明确预算、Codex 真实账号能力与产物取得、可信费用/额度合同、真实审核；API 路线另要求 .04 安全凭据。首链一次单产物自由文生图，模型版本不自动指定。
- **产物/职责：**现有 operation/canvas/workbench 入口最小接线：模型参数 → 报价 → 明确确认 → 任务恢复 → 候选预览 → 用户采用同项目 ready/passed `media_asset` → 现有 `AddNodes` 持久资源绑定。该命令不承接任务或镜头选定事实；以后选择 shot_frame/shot_take 目标才按 E-22 接真实 shot 正式选定。
- **Red：**后端 operation/media/canvas 补 `TestM1ImageVertical` 系列；就近组件测试补未确认不发送、确认重放、刷新/错误恢复、采用绑定幂等、迟到结果不得覆盖已改资源引用/删除节点/切换项目、焦点返回。
- **验收：**技术：`go test -race ./tests/operation ./tests/media ./tests/canvas -run '^TestM1ImageVertical' -count=1 -v`。真实：受控调用一次，记录真实请求回执、字节/SHA/探测/审核/可信 usage/账本；重启与媒体失败恢复仅重试已有结果。产品：真实浏览器确认、刷新、预览、用户采用并 AddNodes，刷新后媒体资源绑定仍存在。
- **退出/真实条件：**三类证据分别通过才关闭首条链；测试重放不再次付费。供应商/预算/审核缺项保持待真实验收，不触发调用；一次图片通过不等于完整 MVP。

### M1-12.13 参考图与蒙版编辑（M，条件扩展）

- **归属/依赖：**E-26、E-39-04/05/06、E-19；依 .12 和实际模型编辑支持证据，既有参考输入/遮罩合同就绪。
- **输入：**同项目 ready/passed 素材、授权/TTL、所选路线的实际输入字段（HTTP 如适用 multipart）、输入顺序/用途、尺寸与 mask alpha/坐标语义。
- **产物/职责：**所选路线图片编辑分支、operation 输入冻结、media 输入取得与现有编辑入口；Codex、火山和 OpenRouter 按各自支持验证，不统一假设 `/images/edits` 或凭模型名/manifest 认定支持。
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

- **归属/依赖：**E-27/28、E-39-03、E-24；依 G2 共享底座、.04 API 凭据、视频协议/计价/审核及实际业务前置；不依 .13/.14 候选或 .16/.17。
- **输入：**优先火山 Seedance 实际支持的一个图生视频/参考模式、具体模型/schema/duration/resolution 限制、真实 task ID、请求键查询/取消与未受理证据；不能从 supports_query 推导按键查询可靠，型号不默认指定。
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

- **归属/依赖：**M1-12；具体用途关联原 E-12/13 等文本业务任务，API 路线依 .04，按合同冻结首个目标；若选择 E-13 episode_parse，则依 E-12 的真实剧本/分集，不能把完整 M2 当已完成。
- **输入：**优先通过 OpenRouter 选择实际文本模型，固定该模型/端点协议、token 类别/价格/预算与输出 schema；结构化解析另含规范化原文与偏移、稳定键、每句台词一次及待处理行规则。
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

实施中每次只提交可独立验证的范围，遵守 AGENTS 的中文提交格式与 allowlist；是否提交/推送/发布依当次用户授权。本轮测试 `9c9fffe7` 与实现 `38a2a016` 已提交本地 main，仅覆盖 .02/.03 数据/DTO/发送权/执行保护；未新增真实供应商执行、未访问真实凭据或付费测试，服务库未迁移、进程未重启。201 个总任务、18 个已完成任务保持，M1-12 继续进行中。

## 6. 自审重点与下一步

- .02/.03 首片技术已通过，下一步优先 .05 本机 Codex 模拟 stdio/能力探测/单次 turn/产物安全合同；真实账号、模型、产物与费用未验证时不启用生成。
- .04 API 临时密钥可与本机桥并行，火山/OpenRouter 必须依赖它；.06/.07 可靠收据/媒体按接口推进，费用/审核按各自合同放行。不能用公共 schema、HTTP 200、模型名称、mock 费用或样例媒体替代 G2。
- .13/.14 的新增候选不是 .15/.16/.17 的前置；全面页面重构 .18 最后进行，各能力最小入口仍随纵向切片交付。
- 所有追加数据库/Workflow 改动保留历史；不删除或重置数据，不对未知请求重新生成，不在关闭新请求时关闭在途恢复。
- 首次真实调用前检查实际账号/线路、模型参数、产物取得、可信费用/额度或 usage/价格/FX、审核与预算；API 路线另检查安全凭据注入。缺项只交付技术证据，不默认免费、不复制 OAuth secret、不无声回退付费 HTTP。

## 7. 当前交付与紧接着的切片

| 子任务      | 当前状态                                   | 下一退出条件                                                                         |
| ----------- | ------------------------------------------ | ------------------------------------------------------------------------------------ |
| .01         | 首片内部合同已固定；供应商方向已确定       | 具体模型、额度/费用、审核与管理授权逐步冻结                                          |
| .02/.03     | 数据/DTO、发送权、已执行与终态保护技术通过 | 执行 Activity 消费发送门禁；状态转换/计价分别由 .08/.10 接入                         |
| .05         | 下一代码切片：本机 Codex stdio adapter     | 模拟进程合同与生命周期先 Red/Green；再探测实际能力，验证专属任务的单次生成和产物回传 |
| .04/.06/.07 | 可按边界并行准备                           | API 临时密钥；完整 staging manifest；真实字节接管，各自取得技术证据                  |
| .08～.12    | 未关闭；G1/G2 未通过                       | 可信费用/额度、真实审核、历史回放、公共命令和现有入口产品链                          |
| .13～.18    | 后续按适用能力推进                         | 支持模式各自验收；全面页面最后完善                                                   |

本轮测试库已精确清理，现有业务库和运行进程保持原状；没有真实供应商、付费生成或部署证据。后续实现从 .05 的失败合同测试开始，不通过再次生成处理断线未知结果。
