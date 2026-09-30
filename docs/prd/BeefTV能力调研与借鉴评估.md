# BeefTV 能力调研与借鉴评估

日期：2026-09-30。状态：调研结论与引入建议，待用户评审；不自动修改已接受的产品范围、需求、架构或 BACKLOG 状态。

## 1. 结论

BeefTV 值得参考，但主要价值在于把生成、素材、任务和画布组织为可继续创作的工作台，以及处理失败、刷新、迟到结果和媒体资源释放的具体方法。Lanverse 应借鉴这些机制，复用 Next.js、Go、Temporal、PostgreSQL、私有对象存储和服务端命令边界。用户补充允许必要时取消独立 Agent 服务，由 Go 统一承接后端；此决定的迁移范围将在服务阶段单独设计。

长期关键缺口是**把已有内部实现接成真实用户闭环**：登录与项目 → 素材 → 模型与报价 → 确认生成 → 任务恢复 → 候选查看与选定。公开接口、前端请求链和真实供应商仍缺少完整连接；新增 3D 导演台、深度视频或插件市场不能解决这个缺口。**用户确定的实施顺序是先完成创作 / 内部管理前端，再补对应服务并验证 PoC，之后建设消费者功能。** 本文的 A1～A5 是服务依赖顺序，不是要求在前端之前实施。

建议近期吸收四组机制：

1. 画布媒体按需加载、播放额度、移动与离屏释放、海报保底。
2. 任务详情跟随服务端事实刷新，刷新页面可恢复，取消回执不被失败读取覆盖。
3. 生成失败明确原因、下一步和安全恢复动作，区分“重查、重下载、改输入、新报价”。
4. 任务产物登记与画布绑定分开幂等，迟到结果必须验证项目、对象和版本。

其中不少能力已经在 Lanverse 的 DES-04、DES-06、DES-27、DES-28、DES-33、DES-38 中设计过，属于**实现和联调缺口**。诊断包、提示词改写差异预览、媒体布局整理等属于需要另行接受的补充建议。

## 2. 基线、方法与证据边界

| 对象     | 本次固定基线                                                         | 检查范围                                                                                              |
| -------- | -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| BeefTV   | `852961a10666979a04c8b851c30bcc162f1ccbf4`                           | GitHub 元信息、完整 Git tree、README、LICENSE / NOTICE / 第三方声明、功能文档、重点实现和对应测试源码 |
| Lanverse | `b75bdcc90666cc07380cf872f4dfeea1d13ad3b6`，`main`，开始时工作区干净 | AGENTS、PROJECT、BACKLOG、现有需求与设计、公开路由、前端页面、Worker、媒体与供应商代码                |

工具分工：

- **GitHub 插件**读取仓库、许可证、文件树、功能清单、Issue 和 PR #48；默认分支 tree 有 2,485 个条目，未截断。另在仓库外 `/tmp/lanverse-beeftv-research-20260930` 下载相同提交的只读调研副本，逐文件核对实现，不安装或运行其依赖、脚本和应用。
- **Firecrawl 插件**实时抓取项目主页和固定提交的生成错误文档，用于交叉检查公开描述；抓取含 GitHub 导航噪声，源码证据以固定提交文件为准。
- **Context7 插件**分别解析并查询 React Flow、TanStack Query、Temporal 的官方文档，用于核验适配方式。检索结果不是对 Lanverse 当前锁定依赖的运行测试。
- 历史记忆仅用于定位；9 月 26 日移除旧实现后的**当前文件**决定现状。旧版画布曾经交付，不代表当前画布已交付。

本文的“事实”指本次读到的源码、测试源码或公开文档；“判断”指适合 Lanverse 的取舍；“待确认”包括真实供应商、正式安装包、设备性能和用户验收。没有运行 BeefTV，没有验证真实模型、支付、GPU 推理或正式桌面安装包，也没有以测试文件存在替代测试通过。

## 3. 项目定位及可复用边界

**事实：** [NOTICE](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/NOTICE) 与[本地架构文档](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/docs/local-first-architecture.md)将桌面模式定义为单用户本地工作区：Wails 承载 React / Vite，Go 提供 loopback API，SQLite 与本地资源目录保存事实，进程内任务 Worker 执行生成。

仓库同时保留 hosted profile 的账号、同步、支付等代码和文档。桌面 local profile 不注册登录、支付、财务及 SaaS 模型目录等路由；外部素材选择器也不会枚举 Eagle 等插件来源。因此不能把[功能清单](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/docs/content/docs/overview/features.mdx)中的每一项等同于桌面模式可用功能。

**事实：** 当前版本已撤下旧内置画布 Agent，替换内核仍在选型。功能清单、`backend/internal/localapp/app_test.go` 与组合根端口共同支持这一结论；残留 `cloud_agent*` 源文件不能证明产品入口仍存在。剪辑编辑器内的自然语言命令助手是另一条路径，不能混同为画布 Agent。

**判断：** 复用单位应是独立算法、交互规则和故障合同。`backend/internal/app` 的兼容入口、SQLite repository、浏览器模型配置、整份画布同步、Wails、Ant Design、支付 RPC 与本地 Python runtime 都不适合直接搬入 Lanverse。

### 3.1 许可证

[LICENSE](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/LICENSE) 是 MIT，包含 BeefTV contributors、basketikun、ddcat 的版权声明。若复制或实质性移植代码，应保留适用的版权与许可全文，记录固定来源和修改范围，并纳入现有 `/licenses` 公示。

[第三方声明](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/THIRD_PARTY_NOTICES.md)特别列出 `@ffmpeg/core@0.12.10` 的 GPL-2.0-or-later 许可。MIT 根许可证不会覆盖第三方组件的许可；不建议为借鉴预览交互而搬入 FFmpeg.wasm 二进制。Lanverse 已选服务端 FFmpeg，实际使用的构建仍需按其自身许可检查。

## 4. 能力对照与优先级

“近期”表示建议优先级，不表示本次已批准或交付。

| 能力                            | BeefTV 的源码证据                                    | Lanverse 当前事实 / 既有设计                                                                         | 建议                                                   |
| ------------------------------- | ---------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| 无限画布性能与媒体生命周期      | 空间网格、渲染预算、视口即时预览、悬停视频租约       | 已有 React Flow 固定样本 PoC；DES-06 / DES-38 有 LOD、离屏释放、≤3 视频，正式业务画布未实现          | 近期：补生命周期，再做不同媒体的实测；保留 React Flow  |
| 任务详情与恢复                  | `use-task-details.ts`、任务摘要节点绑定、PR #48      | Operation 与模拟工作流已有；公开查询、详情页与 SSE 接入待完成（DES-27）                              | 近期：服务端状态为准，取消回执优先，刷新可恢复         |
| 生成失败分类                    | 后端与前端归类器、共享错误 fixtures                  | 已有 `ProviderError(code,retryable,message)` 和 unknown / 对账模型；丰富的用户动作与真实错误映射待补 | 近期：适配真实供应商时落实，未知提交绝不盲目重交       |
| 媒体产物与引用一致性            | effect claim / renew / complete、画布结果绑定校验    | 已有生成结果媒体入库、审核、私有对象存储与稳定输出身份；业务选定 / 画布绑定待补                      | 近期：素材登记和绑定分别幂等；不得覆盖人工配置         |
| 模型与参考素材合同              | 模型线路声明、数量 / 时长 / 尺寸 / 角色校验          | 模型注册表与参数 schema 已有内部实现；真实适配器和端到端验证未完成                                   | 近期：补线路能力与传输合同，继续由服务端强校验         |
| 保存保护、历史与恢复            | revision 检查、分支合并、只读历史、手动保存          | DES-06 / DES-38 已规定 revision、命令重放、快照；当前无业务画布存储模块                              | 随 E-36 实施；不照搬整份本地覆盖远端策略               |
| 智能引用与提示词编辑            | 原始富引用与最终提示词分别保存、重复提交锁、改写入口 | 结构化引用与 Harness 已设计；正式编辑器与真实模型待接通                                              | 随剧本 / 分镜实施；补改写差异预览，不先搭新 Agent 框架 |
| 自动布局、批量创作              | 拓扑分层 + 媒体泳道；批量表与生成布局                | BatchWorkflow / 报价内部路径已有；DES-26、DES-38 覆盖业务批量与画布操作                              | 布局算法可独立参考；批量仍复用 BatchWorkflow           |
| 用户诊断包                      | 前端有界事件、后端按用户 / 项目过滤、ZIP 与脱敏      | OTel、审计和追踪底座已有；未找到用户可导出的诊断闭环                                                 | 近期补充设计候选；只输出必要结构化证据                 |
| 3D 导演台 / 深度视频            | Three.js / R3F、相机与调度、灰度深度参考视频         | 已有风格 / 运镜素材与全能参考需求；无 3D 编辑器或深度模型 runtime                                    | 后续实验：先验证对镜头可控性、成本和一致性的实际收益   |
| 时间线、字幕、成片导出          | 命令状态机、项目快照、whisper.cpp、服务端 FFmpeg     | 产品明确将剪辑与交付放在 V2                                                                          | 保持 V2，不用竞品功能扩大本次 MVP                      |
| 插件市场、Eagle、支付、桌面发行 | 插槽、权限、连接器、支付包、Wails 更新               | DES-06 明确不移植插件、本地 Agent 与本地持久化；MVP 无对应平台需求                                   | 暂不引入；业务适配器已经能承担当前扩展需求             |

## 5. 重点源码分析

### 5.1 性能优化的核心是减少昂贵媒体，而非只减少 React 更新

[`canvas-performance-mode.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/lib/canvas/canvas-performance-mode.ts)根据媒体数量、缩放和用户性能偏好减轻效果，规定节点与连线渲染预算，进入与退出视口使用不同缓冲范围，减少边缘反复挂载。

[`canvas-spatial-index.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/lib/canvas/canvas-spatial-index.ts)使用**均匀网格**，不是 R-tree；大范围对象另行处理，并对查询数量设界限。[`canvas-live-viewport.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/lib/canvas/canvas-live-viewport.ts)让交互中的视觉变换与已提交状态分开。

[`canvas-video-hover-preview.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/lib/canvas/canvas-video-hover-preview.ts)把悬停加载与实际解码都纳入同一所有者：350ms 后开始、最多约 3 秒预览；显式播放优先；失焦、移动、离屏、移出、加载超时都会取消，结束时释放 `src` 与解码器。首帧到达后才显露视频，避免黑屏闪烁。

**对 Lanverse 的意义：** 当前 `poc-canvas.tsx:51` 只有暂停 / 恢复，`poc-data.ts` 在构造数据时把前三个视频标为播放；这能约束固定样本，却不等同可复用的运行时播放预算。应先补明确所有者、异步失效和销毁机制。

**不应照抄：** 上游的 720 节点等预算常量没有证明适合当前设备；强行截断会遗漏可见内容。React Flow 已负责视口计算，未证明瓶颈前不叠加第二套 transform 或空间索引。其官方[性能指南](https://reactflow.dev/learn/advanced-use/performance)支持稳定 nodeTypes、memo、精确状态订阅，并不替代真实媒体压测。

### 5.2 任务详情是持续读取，不能冻结打开时的节点快照

[PR #48](https://github.com/glanderness/BeefTV/pull/48)针对打开详情后进度和日志不更新的问题。[`use-task-details.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/hooks/use-task-details.ts)以 scope + task ID 做 Query key，对 queued / running 或读取错误每 2 秒查询，终态停止；向请求传入取消信号。取消成功先更新缓存，再失效刷新。

对应浏览器测试源码检查成功 / 失败 / 取消终态、关闭 / 切换、读取失败恢复、取消后读取失败和旧在途响应。**本次只阅读，未执行这些测试。**

**适配：** Lanverse 用项目 SSE 让 Query 失效，详情重新读取 Operation；断流时可评审有界轮询兜底。scope 必须包括账号会话与项目。读取不写业务事实，不触发结果物化；不要让“查看详情”产生绑定副作用。TanStack Query 的[查询失效说明](https://tanstack.com/query/latest/docs/framework/react/guides/query-invalidation)支持这一查询模型。

### 5.3 错误分类必须包含下一步和计费边界

[`provider_error.go`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/backend/internal/generation/provider_error.go)及[错误文档](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/docs/content/docs/backend/generation-errors.mdx)区分认证、权限、不同额度问题、输入 / 参考 / 输出审核、参数、限流、提交不确定、下载失败、无结果等，并保留经过限制的排查编号。结构化错误优先，普通包装码不会无条件掩盖内部原因。

跨 Go / 前端的 fixtures 比两份独立字符串匹配表更值得参考。HTTP 402 不能直接解释为 Lanverse 项目余额不足；5xx 或 `retryable` 也不能授权再次付费提交。

**适配：** Go / Python 以现有 Provider Activity 契约承载可靠错误码，后端公共错误仍遵循 RFC 9457，前端按业务状态展示动作。不要在领域层引入供应商原始 JSON，也不直接复制上游大归类文件和模型枚举。

Lanverse 已有 DES-04 的 `MaximumAttempts = 1` 与 `unknown → reconciling / manual`，不是需要从 BeefTV 新建的能力。[Temporal 的 Activity 幂等说明](https://docs.temporal.io/activity-definition#idempotency)解释了重试边界；不重试 submit 仍不能独自保证外部 exactly-once，必须结合供应商幂等键与结果查询证据。

### 5.4 素材登记与画布绑定应该分别幂等

[`generation-task-materializer.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/services/generation-task-materializer.ts)分别处理产物物化、节点绑定等 effect，使用 claim、续期与 completion；任务成功不意味着每一个 UI 绑定动作已经完成。

[`director-cover-write.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/lib/canvas/director/director-cover-write.ts)在提交迟到封面前核对当前项目、最新请求、节点 / 镜头关联和场景版本。这类保护可以泛化到任何异步创作结果。

**适配：** Lanverse 的稳定媒体入库身份继续在 Go / PostgreSQL；候选登记成功后，绑定通过受鉴权命令完成，并携带对象版本 / document revision。关闭页面、删除节点、切换项目或人工改配置后到达的结果仍可保存为候选，但不能自动恢复已删除节点或覆盖人工选择。不要让浏览器承担计费、最终入库或跨项目授权。

### 5.5 模型品牌不是请求合同

[视频合同文档](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/docs/content/docs/backend/video-model-contracts.mdx)与[素材约束文档](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/docs/content/docs/backend/seedance-task-constraints.mdx)区分模型能力、具体接入线路和传输协议：数量、角色、单文件大小、单段 / 总时长、宽高、比例、像素、帧率、URL / 内嵌数据支持。明确 `0` 与未声明不同。

**适配：** P0 核验所选供应商的当前官方合同，填入 Lanverse 模型注册表；供应商适配器只做协议转换和供应商调用；按后续接受的 Go 统一服务设计决定运行位置。服务端复核媒体真实元数据与归属，前端只做提前提示。不能把第三方网关约束当作厂商所有模型的永久限制，更不能从恢复任务路径自动再次上传或提交。

### 5.6 保存保护要保留分支，不能偷换版本

[`canvas-storage-revision.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/lib/canvas/canvas-storage-revision.ts)处理并发更新、删除与本地草稿，保留旧分支的 revision，不把远端新 revision 附到本地旧内容后冒充最新。

**适配：** 实施 DES-06 的服务端 revision + 命令增量 + 重放 + 快照。位置、视口和业务绑定分开；移动视口不能保存整份业务文档。“强制覆盖保存”是上游特定数据修复流程，不适合默认移植到多用户项目。

### 5.7 自动布局可以借鉴算法，不能绕过命令历史

[`canvas-layout.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/lib/canvas/canvas-layout.ts)以拓扑层级安排横向流程，以文本 / 图片 / 视频 / 音频泳道安排纵向内容；无连接时按媒体类型网格排列。锁定和容器等筛选在调用方处理，不在算法文件里自动保证。

**适配：** 只输出布局提案，预览后通过 `MoveNodes` 提交；要处理环、选中子集、父容器坐标、锁定节点、可撤销和确定性排序。不能直接修改 React Flow 内部位置而遗漏后端持久化。晚于媒体生命周期与正式命令链实施。

### 5.8 诊断包是用户自助能力，不只是服务端日志

[`client-diagnostics.ts`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/web/src/services/diagnostics/client-diagnostics.ts)有界记录错误、请求和任务事件；[`diagnostics.go`](https://github.com/glanderness/BeefTV/blob/852961a10666979a04c8b851c30bcc162f1ccbf4/backend/internal/app/diagnostics.go)限制窗口、事件量和包大小，按可见范围生成清单与日志。

**适配建议：** 先输出 trace / Operation / 项目 ID、状态迁移、标准错误码、模型版本、应用版本和脱敏请求摘要。默认排除提示词、剧本、Cookie、密钥、完整请求 / 响应与签名 URL；原始错误文本也可能泄露正文，需要白名单结构化重建。导出前预览，由用户主动下载，不自动上传。此能力当前是新增建议，需补充需求与设计后再实施。

## 6. Lanverse 当前缺口及推进顺序

### 6.1 先完成产品底座，减少已有实现的断点

| 顺序 | 必须补齐的能力                    | 当前证据                                                                                                 | 进入下一步的条件                                                                             |
| ---- | --------------------------------- | -------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| A1   | 公共错误、鉴权与 API / 前端契约链 | `backend/internal/app/router.go:34` 起只挂健康路由；当前无 `frontend/src/lib/request.ts` / `src/gen/api` | 接受 DES-03 的 IF4～IF6 决策；swag → 在线文档 → 生成客户端，浏览器登录 / CSRF / SSE 真正可用 |
| A2   | 项目、模型与素材可访问            | `/projects` 页面仍是开放提示；内部 identity / workspace / catalog / media 代码已有                       | 挂授权路由和真实查询；上传、探测、缩略图、预览和刷新通过                                     |
| A3   | 一条真实生成链                    | `agent/app/main_worker.py:117` 注册 MockProvider；OperationWorkflow 使用 mock 构造入口                   | P0 确认供应商、价格与评测预算；真实 submit / query / cancel、用量和下载联调                  |
| A4   | 任务查看与安全恢复                | 内部 provider_call、状态机、对账已有；正式任务中心与公共查询待补                                         | 终态刷新、断线恢复、取消 / unknown / 下载失败分别走正确动作；账本与候选一致                  |
| A5   | 正式业务画布                      | 当前只有 `/poc/canvas`；DES-06 / DES-38 定义了完整命令模型                                               | 正式选型确认；服务端布局事实、引用与生成命令可用；不同媒体和业务性能验收                     |

表内代码现状针对第 2 节固定的起始基线；F1 新增页面与样例交互见本轮验收记录，公共 API 和真实生成仍待补齐。这些顺序是评估建议，不是本次新增实施计划或完成状态。A1 的现有阻塞来自 `docs/design/03-接口设计.md:482` 的就绪评审记录，不能通过临时 `fetch`、硬编码 Cookie 或手写生成客户端绕过。

### 6.2 可独立审阅的近期技术切片

| 候选                            | 与主线关系                                       | 预计修改边界                                                       | 为什么可以独立验证                                                     |
| ------------------------------- | ------------------------------------------------ | ------------------------------------------------------------------ | ---------------------------------------------------------------------- |
| B1：现有画布 PoC 的媒体生命周期 | 支持 P0-05 / E-36 的性能工作；不宣称交付正式画布 | `frontend/src/features/canvas/`、本切片测试与来源说明              | 不依赖公共 API、真实凭据或付费模型；能在当前页面验证资源释放与异步失效 |
| B2：生成失败语义与动作          | 随真实供应商与 DES-27 实施                       | 现有 Provider Activity、工作流错误投影、前端任务组件、共享合同样例 | 用可控故障验证；真实错误覆盖须另取供应商证据                           |
| B3：任务详情实时恢复            | 依赖 A1 / A2 的公开查询与鉴权                    | Operation 查询、SSE 查询失效、详情组件                             | 能验证读取失败、迟到响应、终态和取消回执；不新增调度系统               |
| B4：诊断包                      | 新产品建议                                       | 待接受后再确定既有审计 / Operation / 前端请求入口边界              | 可用合成事件证明有界、脱敏与权限；真实支持价值由用户验证               |

用户明确选择**完整前端页面 → 对应服务 → 端到端 PoC → 消费者能力**。[引入设计](../design/BeefTV能力引入设计.md)已按此顺序确定 F1 前端范围。B1 保留为后续媒体性能切片，任务失败与候选 / 引用关系先体现在 F1 页面里；服务可靠性仍须在 F2 / F3 验证，不用样例界面替代 A1～A5。

### 6.3 后续实验与暂缓项

3D 导演台的相机焦段、空间调度与提示词编译有参考价值，但“生成控制是否实际变好”仍待测。深度视频是深度估计参考，不应宣传为完整骨骼动捕；GPU、模型、运行组件安装与跨平台成本需要单独验证。两者先建立小型对比样片实验，再决定是否扩展 REQ-34 / 后续版本。

时间线和交付按既有 V2 规划处理。插件生态、支付、桌面发行和外部素材连接器目前没有必要引入；现有显式适配器和注册表足够支持 MVP 的扩展。

## 7. 本次交付及限制

本次交付评估、调整后的 Design 和前端 PoC 页面。实现复用既有前端视觉与领域需求；报价、登录、保存、生成、取消、管理写入均未连接真实服务。未修改后端 / Agent 代码、正式公共 API 合同或 BACKLOG 完成状态。初始调研与实现阶段未提交或推送；用户随后授权将本轮代码和文档提交到 main 并推送。没有创建分支或 PR，也没有使用真实供应商凭据。

上游只进行源码和文档阅读，没有运行 BeefTV 测试、正式安装包或真实供应商。本地前端检查与浏览器证据见 [本轮前端记录](../acceptance/F1-前端工作台PoC记录.md)；真实集成、性能与产品用户验收尚待完成。初始实现验收时尚未核验远端 CI；本次推送对应的实际 CI 状态见交付答复，不能用本地验证代替远端结果。

## 8. 复核入口

- [BeefTV 固定提交](https://github.com/glanderness/BeefTV/tree/852961a10666979a04c8b851c30bcc162f1ccbf4)。重点测试源码：`web/test/canvas-video-hover-preview.test.ts`、`canvas-media-performance.test.ts`、`canvas-spatial-index.test.ts`、`task-details.browser.test.ts`、`canvas-storage-revision.test.ts`、`director-cover-write.test.ts`，以及 Go `provider_error_test.go` / `diagnostics_test.go`。
- [Lanverse 工程边界](../../PROJECT.md)、[当前任务](../../BACKLOG.md)、[接口设计与待决项](../design/03-接口设计.md)、[工作流](../design/04-工作流与生成操作.md)、[画布](../design/06-画布.md)、[任务中心](../design/27-任务中心与对账.md)、[媒体库](../design/33-媒体库.md)、[画布功能](../design/38-画布功能.md)、[既有 PoC 限制](../acceptance/P0-画布PoC记录.md)。
- Context7 的解析 ID：`/websites/reactflow_dev`、`/tanstack/query`、`/temporalio/documentation`。官方核验入口：[React Flow 性能](https://reactflow.dev/learn/advanced-use/performance)、[TanStack Query 失效](https://tanstack.com/query/latest/docs/framework/react/guides/query-invalidation)、[取消查询](https://tanstack.com/query/latest/docs/framework/react/guides/query-cancellation)、[Temporal Activity 幂等](https://docs.temporal.io/activity-definition#idempotency)。
