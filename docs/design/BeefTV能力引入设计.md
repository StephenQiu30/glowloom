# BeefTV 完整工作台迁移设计

日期：2026-09-30；完整迁移范围更新：2026-10-01。状态：**用户已确定完整迁移页面与功能，并保持 Lanverse 技术栈；当前为迁移设计，重建与完整验收尚未完成。**

2026-10-01 实际基线：前端实现已清空，技术栈、依赖、锁文件和工程配置保留，当前不可启动；Go 画布／媒体合同、公开 Swagger、SQL 和业务数据保留。清理阶段的“不搭替代入口”只约束当时清理任务。用户随后否定三个自绘视觉方案，确定以开源能力完整迁移重建，且全部改造成自己的技术栈。旧页面与浏览器证据仍是历史，不能用于本次重建验收。

## 0. 当前完整迁移合同（2026-10-01）

### 0.1 交付目标与技术边界

迁移单位为完整工作台：页面、菜单、弹窗、表单、画布节点、素材工具、生成、任务、配置、保存和恢复都在清单内。内部可以按依赖分模块实施，最终交付必须覆盖完整清单；恢复基础画布、完成单张图片或搭出全部页面壳都不能关闭迁移。正常推进不以每个模块完成后再次询问“是否继续”为前置。

保持 Next.js App Router、React、TypeScript strict、Tailwind、shadcn/Radix、TanStack Query、Zustand/Immer、RHF/Zod，以及 Go、PostgreSQL、Temporal、Outbox/事件和私有对象存储。源项目 Vite/Bun、React Router、AntD、Wails、SQLite/IndexedDB、本地任务执行器和浏览器供应商密钥按对应职责替换。替换技术实现不能删除对应用户功能，也不能引入第二套项目、素材、任务或配置事实。

先沿用源工作台的页面结构、编辑顺序、素材选择、结果处理和快捷键，再统一 Lanverse 的 Vercel 黑白灰、Geist 字体、留白及无边框内容规范。生产表格和画布保留操作密度；不再用已被否定的三个视觉方案作为实施依据。

### 0.2 固定来源与证据

完整功能盘点以 BeefTV `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98`（v1.6.15）为固定快照；画布核心继续继承用户已固定的 `0d9e9f48d407570cd431ad9730cdd522b06810c0`。2026-10-01 已取得两个真实 Git 对象并核对差异：38 个文件变化，新增集中在生成失败、请求证据与诊断，平移、缩放、框选、布局和历史核心未变化。新增文件/字段逐项登记为来源增量，不使用浮动 main 混入其他版本。

本轮静态核对了路由、实际组件装配与后端源码；没有安装或运行 BeefTV，也没有运行真实生成。README、功能文档、测试文件存在均不等于运行通过。当前 `/tasks`、`/skills` 重定向首页；本地模式的结构化项目详情重定向画布；这些事实必须保留，不能把遗留文件当作现成页面。

### 0.3 页面与功能完整清单

下表是整体交付范围，不是完成记录。每组实施时继续从固定源码展开到具体按钮、弹窗和失败用例，并登记目标模块、公开 API、持久化及验收证据。源关闭、开发中、运行受限的项单列原状态，不从清单中静默删除，也不作为迁移已完成的功能。

| 页面或能力组              | 必须保留的用户能力                                                                                                                             | 来源与当前状态；Lanverse 适配                                                                                                                                                                                               |
| ------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 首页与创作入口            | 导航、最近项目、创作入口、模式与上下文切换、加载/空/错误状态                                                                                   | `web/src/router.tsx`、`pages/home/`、`pages/create/`；真实服务与演示数据分开，不能用固定样例代替生成                                                                                                                        |
| 项目库与管理              | 创建、打开、搜索/排序/分页、文件夹、复制、重命名、删除/回收、恢复、项目包及 LibTV/TapNow 导入导出                                              | `pages/canvas/index.tsx`、`lib/canvas/canvas-export.ts`、`pages/canvas/components/`；映射正式项目/画布和 UUID，包内素材经 Go 接管，保留现有业务数据                                                                         |
| 表单式创作                | 文本/图片/视频/音频输入、模型参数、参考顺序、预览、提交、结果采用                                                                              | `pages/create/creation-workspace.tsx`、`pages/canvas/*generation*`；RHF/Zod 按服务端 `param_schema` 渲染，生成经统一报价和 Operation                                                                                        |
| 无限画布与历史            | 完整手势、选择、拖放、连线、分组/布局、搜索、剪贴板、工具栏、属性、历史/版本、定位、小地图、LOD                                                | `components/canvas/`、`pages/canvas/project.tsx`；保留核心算法，改接正式命令、revision、服务器确认和版本恢复                                                                                                                |
| 资产库与素材选择          | 上传/拖入/粘贴、分类/搜索、文件夹、批量操作、预览、引用、转移、容量信息与素材包                                                                | `pages/assets/`、画布素材弹窗；正式资产 ID、项目归属、来源/引用与临时授权预览，传输策略改为 Go/私有存储                                                                                                                     |
| 批量创作与分镜            | 表格编辑、全局提示词、行级参数、参考列重排、连线同步、逐行结果与部分失败                                                                       | `pages/canvas/use-canvas-batch-table.ts`、`use-canvas-generation-batches.ts`；TanStack 表格/虚拟化，服务端 BatchWorkflow 执行，映射真实镜头                                                                                 |
| 任务、候选与诊断          | 进度/状态、详情/日志、取消、重查、失败信息、结果候选、版本比较/采用、迟到结果保护、诊断导出                                                    | `hooks/use-task-details.ts`、`canvas-project-status-dialogs.tsx`、`services/generation-task-materializer.ts`；源画布任务可达、独立任务页关闭，诊断面板未装配。目标任务中心仍是 Lanverse 必需页面，接真实 Operation/SSE/费用 |
| 模型与工作区设置          | 渠道配置、模型列表/默认模型、参数能力、启停与验证、主题及工作区设置                                                                            | `pages/settings/`；控件换 shadcn/Radix，配置/凭据归 Go catalog，不把源浏览器密钥模式搬入                                                                                                                                    |
| 提示词与图片工具          | 结构化引用、优化建议/采用、裁切、绘图、标注、遮罩重绘、宫格拆分、插值放大、角度/灯光/表情、反推提示词、角色参考、人像纹理/全景及已装配分析工具 | `use-canvas-media-tools.ts`、`components/canvas/`、`lib/plugins/builtin/prompt-optimizer.ts`；源无独立 AI 超分模型，本地放大不能称 AI 超分。本地编辑和付费生成分开，新素材保留来源，遮罩按模型能力校验                      |
| 视频、音频、字幕与时间轴  | 截帧、裁剪、媒体组合、字幕编辑/转写、画布时间轴、保存、预览和导出                                                                              | `CanvasTimelineDialog` 在 `project.tsx` 实际装配；独立项目 editor 当前本地路由不可达，状态单列。源已有用户能力纳入迁移，用 Go/Temporal/FFmpeg 承接导出，不沿用静默无字幕回退                                                |
| 3D 导演台及参考工具       | 场景/相机/构图、动画/关键帧、封面与输出、机位提示词、多角度与深度参考入口                                                                      | `lib/canvas/tool-registry/definitions/add-node-menu-tools.tsx`、`CanvasDirectorWorkbench`；前端交互接 Client/WebGL，结果接正式素材；深度本机设备/模型限制单列，需目标环境实测                                               |
| 插件与外部素材界面        | 源已提供且可启用的插件列表、配置、权限、启停、内置创作工具及 Eagle 素材选择                                                                    | `/plugins`、`/plugins/eagle` 受 feature gate；逐能力转成自有组件和显式 Go 执行，不能以源默认未启用或“不迁源插件运行时”为由丢掉可用能力；外部桌面依赖需对应接线和验证                                                        |
| Lanverse 短剧业务与 Agent | 剧本/分集、设定集、资产定稿、镜头表/故事板、关键帧/视频、台词/配音、影响、成本和提案确认                                                       | 继承 PRD-01/REQ，不以源本地工作台替代九场景；旧源画布 Agent 已退场，不能宣称能直接迁入完成品，Go 承接与 AG-UI 仍需实现                                                                                                      |
| 关闭、开发中与辅助入口    | 关闭的技能/任务路由、受模式限制的 editor、开发中的智能剪辑/逐帧拉片/脚本节点、帮助/录音测试/开发实验室                                         | 按 `router.tsx`、`canvas-feature-availability.ts` 登记源状态；开发实验室不充当产品页，源未完成不算完成。当前无登录决定保持，不恢复旧 hosted 账号/支付页面                                                                   |

时间轴、3D 和插件等源实际提供的用户能力，现纳入完整迁移，取代历史“只迁核心、不迁这些能力”的限制。完整迁移不等于原 PRD 的所有未来自助注册、支付、多人协作等能力提前开工；用户已删除的登录不恢复。

### 0.4 改造成自身技术栈的方式

| 源实现职责                      | Lanverse 实现与状态归属                                                                                                      |
| ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| 路由、应用壳、页面和弹窗        | App Router 与业务组件；交互处使用 Client Component，画布/WebGL/低频工具按需加载；`ssr:false` 只在客户端边界                  |
| AntD 表单、菜单和提示           | shadcn/Radix、RHF/Zod、Sonner；保留焦点、键盘、校验、只读和错误恢复行为                                                      |
| 画布与批量表编辑                | Zustand/Immer 仅持有草稿、选择和交互预览；释放拖动后提交正式命令，服务端确认后记历史；表格按需使用已约定工具                 |
| 业务查询、实时任务和候选        | TanStack Query + 在线 Swagger 生成客户端 + Axios；SSE 使对应项目查询失效；表单与画布使用同一配置、资产、Operation 和选定事实 |
| 供应商、媒体、导出、3D/深度产物 | 现有 Go `adapter → application → domain` 与 Temporal Activity；保存/引用/费用/恢复由原模块持有，系统依赖按实际能力接入       |
| 本地项目、素材包和插件能力      | PostgreSQL、私有对象存储与受控命令；导入解析/资源映射和已启用工具能力保留，不以整份源数据库、任意路径或双任务引擎实现        |

### 0.5 已核验缺口与连续实施

迁移启动时公开 API 只有项目/风格、画布 CRUD/commands、媒体 list/preview/upload 共 12 项（`backend/internal/app/public_api.go`）。当时 Operation Workflow 限 `mock/agent.mock`；确认仓储只支持 `target_type=free`；画布只允许 resource 节点和 annotation 边；媒体接管输入只接 URL。生成、镜头引用/提升、任务中心、费用、Agent、时间轴和 3D 等不能仅靠恢复页面接通。后续实施新增合同不表示真实执行已通过。

工作台工具的可写数据采用 `node_action=tool` 和闭集 `config.batch_table`、`config.timeline`、`config.director`；不得写入任务结果、供应商回执或费用。批量配置保存模型 ID、模式、输出数、提示词、参数、参考列/行及并发数，500 行和六个参考列有界；每次报价最多 150 行。报价项可携带 `source={canvas_id,node_id,row_id?,revision}`，Operation 保存同一 `source_context`；报价事务锁定文档，验证项目归属、版本和已保存模型/参数/参考顺序，执行并发读取已保存配置。页面刷新按正式 Operation 来源恢复每行状态，不把浏览器任务 ID 列表当事实。

表单式创作使用同一画布中的 `node_type=generation` 工具节点和 `config.generation`，保存版本、能力、模型 ID、模式、提示词、参数、输出数与有序参考素材/角色。表单页按画布/节点身份打开草稿，保存经相同命令、revision 和幂等回执；报价之前保存并绑定该版本。画布打开同一节点进入同一表单，禁止新增浏览器或独立表单数据库事实。此节点只保存输入，任务与结果仍由 Operation/source 读取，用户刷新未提交的表单也能恢复。

导演台场景按有界、类型明确的对象/摄影机/灯光/分镜/关键帧保存，元素身份和引用需校验。模型引用只接受当前项目正式 `kind=model` 资产；GLB v2 上传实行 64 MiB 上限及结构/资源预算，要求缓冲区和贴图位于包内或符合预算的 data URI，拒绝外部 URI、未知执行扩展和异常引用，不从服务端路径或任意 URL 加载。模型上传沿用本地人工检查确认、幂等、私有对象存储和项目授权，不能假装由 ffprobe 探测模型或给不存在的缩略图返回成功。独立 glTF 多文件包由素材包合同承接。

保存合同按原职责扩展：生成配置、Operation、候选和正式选定分别持久化，时间轴/导演场景使用类型明确的配置与 revision，不以任意 metadata 或完整源工作区快照覆盖业务。项目/资产包采用独立、有界的导入/导出合同，验证格式、文件数量/大小、路径与资源映射；不放松当前 `/media/uploads` 的媒体白名单。包导入失败可恢复原批次，不复制源资产 ID、本机路径、任务/费用记录或凭据。Next 下载与 Go API 替换 Wails 原生保存、`window.go` 与 loopback token，不照搬全局 fetch 注入。专用渲染/解析依赖只在实际消费者需要时引入，不更换已有工具链。

源 `/create?demo=conversation` 仅固定初始对话，提交仍可能进入真实生成；重建时明确隔离演示与实际操作，不能展示“不调用生成”却继续发送。字幕烧录失败必须明确失败或由用户选择输出，不沿用静默无字幕交付。

实施沿现有迁移计划推进，补齐每个缺失模块的公开合同、业务状态、持久化与真实执行。页面和后端可按依赖并行，不把全面页面拖到所有供应商能力完成后才盘点；每条能力同时贯通原入口、失败恢复和跨页面状态。原计划中的首图与门禁是内部验证步骤，不是范围缩减或每步重新请求用户批准的边界。

页面切换和刷新必须恢复同一项目、配置、候选与任务；浏览器关闭、SSE 断连、服务/Worker 重启不得重复生成或收费。未知结果先对账；失败重试不能无声改供应商或创建新的付费执行。参数或引用变化使旧报价失效，正式选定不被迟到结果覆盖。真实外部条件缺失的项保持未通过，同时继续不依赖该条件的工作，不能用 mock、禁用按钮或源功能文档关闭完整迁移。

500 行全表可按每组最多 150 行统一报价并一次审阅总费用；各组确认使用各自原始幂等键，部分确认失败时保留已确认事实，不能用新键重复发起。服务端启动准入按同项目、同画布、同节点的全部批次合计占用槽位；跨 revision 仍计入，已有活动使用的最小冻结并发限制在其结算释放前保持生效。项目和渠道限流继续叠加；未知、未结算或仅请求取消的活动不能提前释放槽位。

### 0.6 完整验收与完成定义

1. **覆盖验收：**逐项核对固定快照中的路由、菜单、弹窗、工具、节点及模型能力，全部映射到目标实现或明确记录源不可用状态；每项有页面、操作、接口、保存和验证证据，没有遗漏或占位成功。
2. **交互验收：**表单与画布使用同一业务事实；素材选择、批量编辑、候选/版本、任务处理、配置与导入导出贯通，含加载、空态、校验错误、部分失败、冲突、只读、取消与未知。
3. **运行验收：**真实媒体、供应商产物、接管、审核、费用、字幕/导出及适用 3D/深度运行分别留证；模拟与静态检查不代替真实链。源不支持的设备明确记录，不能声称目标环境已支持。
4. **恢复验收：**刷新、跨页、断连与进程重启后恢复相同任务/状态；无重复执行、费用、素材登记或覆盖用户新选定；项目包往返后关系与素材可用。
5. **完整产品验收：**继续执行 PRD-01 §8.2 九场景，包含两种项目风格、全能参考、修改传播、续集复用、成本、500 节点和 Agent。源工作台迁移和完整短剧业务都满足后，才报告整体完成。
6. **工程验收：**适用 lint、类型检查、构建、核心/Race/集成测试和真实浏览器检查通过，许可按实际复制文件更新；旧代码验收和跳过项不计本次通过。提交、推送、发布仍按用户授权执行。

以下章节保留画布及历史切片的技术记录；其中已被上述完整范围替代的限制不再作为本次删减功能的依据，历史完成状态不改写成当前完成。

## 1. 目标与范围

Lanverse 按上线项目标准实现完整产品，当前优先跑通整项目验证和 Demo。当前用户明确先完成页面与服务 PoC，暂不要求登录认证；本机可直接进入同一套工作台和数据合同。消费者账号与登录验收后置；一次演示、一条生成链路或基础画布不能替代 PRD-01 §8.2 的九个 MVP 场景和上线门禁。

用户指定直接复用 **BeefTV 无限画布及对应设计**，替换现有画布以减少重复开发。范围为 DOM/SVG/rAF 引擎、视口/缩放、框选/拖动、对齐/分组、连接、快捷键、历史、小地图、空间索引、LOD、媒体播放设计及 Lanverse 正式节点/服务端接线。不得降为外观借鉴、自绘 React Flow 替代品或纯备注画布。

历史阶段只迁画布核心，2026-10-01 已改为第 0 节的完整工作台范围。供应商、3D/深度、时间轴及源可启用插件的用户能力逐项适配；Wails、SQLite/IndexedDB、源任务引擎、发布脚本和部署配置以自身技术栈替换。Lanverse 镜头/资产/生成节点、reference/promote/run、报价确认、正式选定及 Agent 继续按业务合同接入。

## 2. 固定源码与直接复用

当前主要参考项目与源码来源：[glanderness/BeefTV](https://github.com/glanderness/BeefTV)。画布核心保留用户确认的 [0d9e9f48d407570cd431ad9730cdd522b06810c0](https://github.com/glanderness/BeefTV/tree/0d9e9f48d407570cd431ad9730cdd522b06810c0)，完整功能与增量来源按第 0 节固定；LibTV、旧 infinite-canvas 与 BeefTV 852961a1 调研保留历史出处，不再作为当前画布选型建议。

源码审计估计 InfiniteCanvas 运行闭包 9 文件/1,405 行，扩展核心 16 文件/2,632 行，核心外部依赖 React/Zustand。实际移植仍须逐文件列来源和改造，不能以行数证明完成。

| 来源模块 | 复用 | 改造边界 |
| --- | --- | --- |
| web/src/components/canvas/infinite-canvas.tsx、视口/外观 | DOM 变换、平移/缩放、指针、背景 | Next 客户端组件，主题接 next-themes/Geist |
| use-canvas-selection-controller、viewport-controller、空间索引 | 单选/多选/框选、群组拖动、坐标/预览 | 局部预览，交互结束提交正式命令 |
| canvas-layout/frame/node-copy、连接策略 | 对齐/布局、分组、复制、连线算法 | 去除插件/本地仓储耦合；使用正式 UUID |
| 节点壳、连线层、小地图、菜单/工具栏/属性设计 | 直接复用对应行为与设计 | AntD 换现有 shadcn，保留焦点/只读/错误语义 |
| use-canvas-history、核心快捷键/操作 | 历史组织、反向操作 | 服务端确认后入栈，不用快照覆盖业务事实 |
| 上游项目/媒体/生成服务依赖 | 由下文端口替换 | 不复制密钥、直连 provider、local workspace 或 IndexedDB |

根 LICENSE 为 MIT，含 BeefTV、basketikun、ddcat 版权；复制源码保留适用声明，核对实际文件和第三方依赖，实际清单以根 [THIRD_PARTY_NOTICES.md](../../THIRD_PARTY_NOTICES.md) 为准，并在 /licenses 提供 /licenses/beeftv.txt 与来源致谢。根 MIT 不证明全部媒体和依赖可复制。只引入核心所需包，不复制上游 scripts、package scripts 或 Makefile。

## 3. 正式模块与端口

继续 Next.js、React、TypeScript、shadcn/ui、TanStack Query、Zustand；核心与 Lanverse 业务绑定按职责分开：迁入运行/算法位于 components/canvas/engine，节点位于 nodes，正式装配由 workspace.tsx 的 CanvasWorkspace 承担。Go 保持 adapter → application → domain。

| 端口 / 状态 | 正式合同 | 所有者 |
| --- | --- | --- |
| 工作区/项目 | 单一工作区身份、组织项目、归档只读 | identity/workspace，不映射样例 ID |
| 文档/节点 | document/node/edge/viewport/revision；节点 title/parent_id/z_index 与类型安全 config | Go/PostgreSQL；source k ↔ API zoom 显式转换 |
| 命令提交 | expected_revision、UUID 幂等键、确认文档/结果或结构化错误 | 在线 Swagger 生成 API → request.ts |
| 媒体 | 同项目已接管可用 media_asset ID，授权 URL/缩略图由服务端投影 | media，不保存任意 URL/blob/本机路径/storageKey/密钥 |
| 业务对象/动作 | ref_type/ref_id、摘要、参考/报价/任务/候选/正式选定 | 与流水线共用业务模块/query key，不迁 source executor |
| 交互/历史 | 选择/拖动/视口/草稿与成功操作反向历史 | Zustand/页面内，刷新从服务端恢复 |

本轮资源类型闭集为 text/image/video/audio/group，node_action=resource；配置闭集 text={text:string}、group={collapsed:boolean}、media={}；媒体只引用授权同项目 ready/passed 已接管对象，经独立媒体 preview 端口取得临时 URL，不放 Document.Node。标题使用正式字段，禁止将上游任意 metadata 当可写配置。原布局命令 AddNodes/MoveNodes/UpdateNodeConfig/DeleteNodes/Connect(annotation)/Disconnect/SetViewport 保留，扩 ResizeNodes/RenameNodes/SetNodeParents/SetNodeZIndex；Connect/Disconnect 使用 edges/ids 数组，精确形状及画布名称/软删除、媒体列表/preview 合同见 DES-38 §3.2。

分组采用源 world 绝对坐标：移动组由客户端批量包含 children，统一 delta；parent 仅指同文档 group 且无环；删除 group 解组，保留 children 及绝对位置。删除普通节点移除关联边，不删除业务对象。zoom=0.05～4。正式命令更新标题、尺寸、层级；DTO/白名单/大小/批次限制同步 DES-02/03/06/38 与 swag 类型，不手改生成物。

沿用 canvas.document/node/edge/command_log 和已发布迁移；字段/约束追加 forward SQL。快照端点/恢复合同不在本轮自行扩展；未来恢复产生新 revision，不能回退历史。镜头/生成/reference/promote 等按既有业务合同接入，未就绪能力明确不可用，不造样例任务/费用。

### 3.1 Next.js 与 shadcn/Radix 接入调整（2026-09-30）

用户本轮重申复用画布并使用 Next.js + shadcn + Radix。仓库已经是 Next.js 16 App Router，`components.json` 为 radix-nova；现有 `InfiniteCanvas`、空间索引、连线与正式 Go 保存链直接沿用。复核上游固定 SHA 的 `web/package.json` 与 InfiniteCanvas：上游 Vite/Bun、AntD 与本地工作区不是 Lanverse 的运行依赖，不复制这些工程配置。

必要调整限定为画布控件与交互边界：模式按钮改为官方 Radix 单选 ToggleGroup，始终保留框选/移动之一；动作提示使用 shadcn Tooltip；SelectItem 放入 SelectGroup；错误、空态与加载使用现有 Alert、Empty、Skeleton。工具按钮的方向键交给控件焦点导航，不能同时产生 MoveNodes；画布全局保存/撤销等快捷键继续有效。错误恢复、失败幂等键、只读与保存禁用规则保持。

Next 页面保留 Server Component 与 Suspense；浏览器引擎在 Client Component 中动态加载，`ssr: false` 只放客户端边界。不增加第二份画布状态或新的 API。验证包括方向键冲突回归、现有组件与保存测试、生产构建及桌面/手机浏览器控件验证；合成 API 的浏览器验证单列，不能代替真实 Go/PostgreSQL 或供应商验收。

官方组件用法依据 [Radix ToggleGroup](https://ui.shadcn.com/docs/components/radix/toggle-group)、[Tooltip](https://ui.shadcn.com/docs/components/radix/tooltip)、[Select](https://ui.shadcn.com/docs/components/radix/select)；同时核对 Context7 的 shadcn 4.21.0 文档和本机 Next.js 16.3.6 附带指南。浏览器发现程序打开的搜索弹窗关闭后丢失焦点，搜索和媒体弹窗通过 [Dialog 的 onCloseAutoFocus](https://www.radix-ui.com/primitives/docs/components/dialog#content) 恢复实际打开位置，不另写焦点陷阱或滚动锁。

### 3.2 核心能力补齐与本地媒体上传（2026-10-01）

用户明确本次只完成无限画布，并将本机图片、视频和音频上传一并接通。继续固定源提交与 DOM/SVG/rAF 引擎，不启用 Codex、火山引擎或 OpenRouter 生成线路。

补齐空白双击/右键创建、从连接点拖至空白创建相连节点、行/列/网格/关系/散开布局、100%/全画布/选区定位、背景和小地图开关、焦点模式与快捷键帮助。系统剪贴板使用版本化白名单图数据，同项目媒体引用、UUID/内部父关系/连接重建；普通文字可创建文字节点，原生文字选区保留系统复制。拒绝非法图、未知字段、跨项目媒体、越界尺寸/数量及运行时 URL，不以页面 ref 冒充系统剪贴板成功。节点进入/保留视口采用不同余量，按缩放限制 DOM 数量，选中/交互节点优先；连线独立裁剪且折叠端点仍投影到组。

本地上传采用 `POST /api/projects/{pid}/media/uploads` 的同步 multipart 接管，字段为 `file` 与 `local_review_confirmed=true`，由在线 Swagger 生成调用。此端点限定当前回环单工作区；复用 Origin、UUID 幂等键、账号/项目权限与归档只读，单独限定 multipart 请求而不放松其他 JSON 端点的 1 MiB 边界。图片 JPG/PNG/WebP ≤20 MiB；视频 MP4/MOV ≤500 MiB 且 ≤60秒；音频 MP3/WAV/M4A ≤100 MiB；服务端以真实内容和 ffprobe 校验，不相信浏览器 MIME/扩展名。禁止文档、ZIP、外部 URL 或服务端任意路径导入。

浏览器同源上传由当前 Next.js Node Route Handler 有界流式转发至 Go，保留原始 Origin、幂等键与 multipart boundary，传播取消及五分钟期限，正文上限为 500 MiB + 64 KiB，且不整批解析或复制到内存。其余 API 使用 fallback rewrite；此专线属于当前 API 的传输适配，媒体权限、验证、人工确认依据和持久化仍由 Go 承担，部署时构建与运行的 `LV_API_BASE_URL` 必须一致。

上传前用户必须明确确认已检查内容、拥有使用权限且不含需要授权的真人素材。本地模式记录真实的 `local_workspace_owner_review`、工作区主体、时间和文件哈希，不声称调用外部自动内容审核；这替代本切片对尚未接入的云审核依赖，不替代消费者部署或生成素材的审核规则。包含需授权真人的素材须等待真人授权流程，不伪造 consent。未确认、类型/大小/时长不符、探测/派生/对象存储失败均不得返回可引用资产。

服务端流式读取到有界临时文件，计算 SHA256，复用真实探测、缩略图/海报/波形与对象存储适配器；写 `origin=upload`，不虚构 Operation、供应商或费用。对象键由项目/种类/资产 UUID 派生；全部对象确认后，事务写媒体元数据、派生、审核依据、审计及幂等回执。同键同文件/声明/文件名回放首次结果，同键异体拒绝；同项目相同哈希的已可用上传默认复用。失败和取消关闭并清理临时文件，不泄露路径或对象管理密钥。

浏览器文件选择、文件拖入和图片粘贴进入同一上传确认流程；每文件进度和失败重试复用原键。上传成功后只将返回的正式同项目资产 ID 写入 AddNodes；上传成功但画布保存失败时保留正式资产和原画布失败批次供重试，不重复上传或声称节点已保存。项目切换/卸载取消请求；画布命令撤销移除节点引用而不删除媒体文件。媒体库选择继续可用。

此切片不实现大文件分片续传、云自动审核、真人授权、持久快照恢复或供应商生成。正式文档刷新恢复、成功操作本页撤销及 revision 冲突保持；这些边界与目标机器性能分别记录，不以“完整画布”代替完整产品验收。

## 4. 入口、删除与数据保留

2026-09-30 用户指定业务组件统一放在 `frontend/src/components/<业务>/`，当前 auth、canvas、catalog、operation、project、workbench 六个目录及其私有查询/状态/测试整体迁入。随后按用户指令清理登录功能并删除 auth 目录，当前保留其余五个业务目录。画布核心、相对模块依赖与正式路由保持，只更新导入路径；下文旧引擎删除路径保留为当时位置。目录约定以 PROJECT §6 为准。

正式 /canvas 与 /projects/{UUID}/canvas 使用同一 CanvasWorkspace；项目路由传 initialProjectId，选择与读取使用真实项目/画布 ID；当前阶段由 Go 提供单一工作区身份，不需要登录、首登改密或会话查询。业务入口、组件/服务命名、按钮和产品文案不含 poc 或 livedemo；不保留旧画布兼容入口。

删除 frontend/src/features/canvas 中旧 poc-_、live-_ 引擎/编辑器/状态及旧用例，删除 frontend/src/app/poc/canvas 和旧 features/workbench/creation-canvas.tsx，实现正式路由接新引擎。替换消除双引擎/双状态，不能只改标签。专用依赖/样本确认无其他消费者再移除，不误删其他工作台和共享认证/请求模块。

源码替换不授予清库权限。保留历史 DDL、画布记录、revision、日志、账号/项目及其他业务历史；旧备注在新引擎可呈现/编辑。未知历史值不可静默覆盖/清空，变更追加迁移并验证恢复/回滚；不得重建数据库或导入 source local workspace 覆盖项目。

验证和测量放在正常命名 tests/e2e/acceptance；synthetic fixture 明确来源。Demo 使用正式引擎，不增独立 PoC 产品页或第二份正式持久化。固定媒体、真实媒体、目标机器和跨设备分别留证，旧引擎结果不移作新引擎通过证明。

## 4.1 当前阶段移除登录功能（用户指定）

用户明确清理当前登录功能，不保留认证兼容分支或免登录开关。删除 `/login` 页面、前端身份查询/拦截、退出/改密交互，以及 Go 公开 `/api/auth/*` 登录、当前会话、退出和改密接口与 Redis 会话、登录限流、密码登录用例。路由或接口被移除后返回 404，不创建重定向或兼容入口。

当前工作台直接调用项目、画布和媒体 API。Go identity 适配器提供单一工作区身份，在事务中复用唯一启用组织（空库才创建）并建立固定 producer 身份；此身份只用于现有项目隔离、审计与命令合同，不需要浏览器凭据，不持有可使用的初始密码。重复请求不新增或改写账号；组织/身份被停用、冲突或依赖失败时阻断，不自动恢复。

继续复用 PostgreSQL、业务命令、修订、UUID 幂等、审计与 Outbox。写请求仍检查精确 Origin 与幂等键；删除会话 cookie、CSRF token 及登录豁免规则。当前工作区只提供本机服务，组合根校验 `LV_ENV=local` 和回环浏览器 Origin，不开放消费者部署；消费者认证是后续独立需求。历史 SQL、账号、项目、画布与审计记录保留，不删除数据或改写历史迁移。

本切片验收真实 Go/PostgreSQL 的直接访问、项目创建、画布保存、刷新恢复和无登录请求；验证移除接口返回 404、跨 Origin 拒绝写入、缺失幂等键拒绝及停用身份不会被恢复。供应商生成与样例页面的全部后端不在本次清理范围。

## 5. 失败、权限与幂等

- 每次读写复核工作区账号/组织/项目；跨组织不可见，归档只读，停用或撤权阻断写入。媒体及业务引用复核授权。
- 同键同体回放首次确认结果，同键异体拒绝；revision、节点/边、日志、幂等回执和 Outbox 原子提交，任一命令失败全批回滚；错误码与 DES-03 统一。
- 交互可预览，服务端确认前不显示已保存。网络失败保留草稿，以原键重放/查明结果；409 取最新正式文档并提示，不能静默覆盖他人配置或重放付费操作。
- 复制新 UUID，清除不可继承的任务/选定身份；撤销/重做提交新允许命令，不回退 revision，不撤销付费生成/正式选定。
- 媒体失败可恢复，预签名过期重新授权；切换项目/卸载释放监听、pointer capture、rAF、计时器、播放/blob URL；只读取消在途手势。
- 当前保持同源 /api、精确 Origin、UUID 幂等键与 RFC 9457；会话 Cookie/CSRF 已随登录功能移除。浏览器不持有供应商/中间件/存储管理密钥。2026-09-30 用户另行要求移除 Python 服务目录，范围见 [Agent 服务目录清理](Agent服务目录清理设计.md)；Provider Activity 的 Go 承接继续由 M1-12 设计与验收，画布迁移不代表生成链已完成。

## 6. 核心交互与验收

| 能力 | 必须验证 | 映射 |
| --- | --- | --- |
| 平移/缩放/背景/小地图/定位 | 坐标/锚点、指针取消、控件不误触、视口保存 | CNV-04/05；TC-36-04 |
| 选择/框选/群拖/对齐/分组/层级 | 空间索引、world 坐标、无环 parent、删组解组、失败恢复 | CNV-01/05；键盘/焦点 |
| 创建/编辑/尺寸/标题/复制/删除/连接 | UUID/config 闭集、复制隔离、关联边、反向历史 | CNV-01/03/05；权限/原子性 |
| 图片/视频/音频/播放 | 授权来源、过期/失败、离屏释放、同时播放限制 | MED、CNV-06 |
| 保存/修订/历史 | 持久幂等、并发冲突、回滚、历史文档/跨设备恢复 | REL-04/06/07；TC-36-04 |
| 业务节点/参考/生成/提升 | 流水线同源、报价确认、真实任务/候选/选定 | TC-36-01～03；E2E-11；对应合同 |
| 新引擎性能 | 500/800、2,000 节点运动/首屏/真实媒体/内存来源 | PERF-06，缺目标机器写未执行 |
| 删除旧实现/生产命名 | 无旧入口/import/依赖，无 poc 业务名；许可可追溯 | 替换完成门禁 |

先验证项目运行/依赖、身份/项目、核心画布与真实服务 Demo，再覆盖完整业务和上线门禁。单元/组件、真实 PostgreSQL、浏览器、真实供应商和产品验收分别记录；接口 200、fixture 和录制响应不等于完整 Demo/产品通过。质量门禁按 PROJECT/AGENTS，未执行如实列出。

## 7. 事实来源与历史

本设计替代把“F1 样例 → 备注画布 → 单条链路 PoC”当当前目标的旧表述。E-36-07/08 和 07dfd2d 检查点保留历史且标被替代，不追认完成。PRD-01/32、REQ-36 保留完整产品范围；DES-06/38/08 同步技术/节点合同；PROJECT 保持工程边界；PLN-01/32、BACKLOG 列核心迁移、正式接线与整项目 Demo 任务。

source SHA/文件/许可、DTO/命令、真实依赖和 Demo 证据逐项验收。未完成生成业务、Agent、跨设备、目标机器不宣称通过；执行优先级不降低上线标准。


## 8. 设置偏好持久保存合同（2026-10-01）

### 8.1 固定来源与迁移范围

固定源 `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98` 的 `web/src/pages/settings/model-default-grid.tsx` 在渠道 pane 实际装配，按图片、视频、文本、音频选择默认模型，原来依赖浏览器配置存储。`prompt-preferences-pane.tsx` 及 `services/api/prompt-preferences.ts`、`backend/internal/prompts/prompt_template*.go` 提供九项提示词偏好的读取、inherit/append/rewrite、恢复基线、变量及只读输出契约；当前 settings 页面未装配该 pane。后者按源未开放页面能力登记，并在 Lanverse 实现可达页面与保存合同，不能声称源页面已验证运行。

### 8.2 项目默认模型

默认模型改为明确的当前项目设置，唯一事实继续使用既有 `workspace.project.default_models`（capability → model_key），沿用项目 revision、同组织项目隔离与归档只读，通过 workspace application 用例更新，catalog adapter 不写项目表。`GET/PATCH /api/projects/{project_id}/model-defaults` 返回 project_id、revision、default_models；PATCH 携带 expected_revision 与 UUID 幂等键，空映射表示清空默认选择。模型候选来自当前项目模型目录；保存时复核相同 capability、启用模型/渠道、当前配置、生效价格及项目地区限制，供应商凭据仅由生成门禁决定，不将偏好保存升级为供应商验证通过。明确草稿选择优先于默认；只有尚无模型选择的新草稿读取默认。后端报价既有默认模型选择与冻结合同继续复用，不额外建立工作区全局回退事实。

### 8.3 工作区提示词偏好

提示词偏好以当前持久工作区主体和组织为范围，admin/producer 均可管理自己的定制；不可编辑平台基线、受保护输出契约或他人偏好，管理渠道/模型仍返回原来的 403。新 `workspace.prompt_customization` 保存 operation、mode、content、base_template_id、revision 与更新人/时间；基线从固定来源改为 Go domain 内只读定义，保留全部九项模板、变量与契约，稳定模板 ID 随基线内容改变。rewrite 的基线 ID 变化时明确 outdated，读取不静默改写。

`GET /api/settings/prompt-preferences` 返回只读定义/基线及当前定制；`PUT /api/settings/prompt-preferences/{operation}` 携带 expected_revision（首次为 0）、mode、content、base_template_id；恢复基线使用同一 PUT 的 inherit+空内容。非 inherit 必须非空且不超过 12,000 个字符，拒绝未知操作、未知或未闭合变量、错误基线、跨主体与 stale revision。保存使用 UUID 幂等键，同键同体返回首次回执，同键异体拒绝；修订、回执、审计与 Outbox 在一事务提交。审计只写 operation、mode、基线、revision 和内容哈希，不写个人提示词正文。

Go 纯编译函数仅替换声明变量，按模式形成创作策略，并始终在后面追加服务端持有的项目/剧情/角色/资产上下文和输出契约；个人定制不能删除运行时强制层。配置保存、纯编译测试和具体生成消费分别验收。未接入的九项生成入口不得显示“已应用生成”；本轮默认模型可接已有新草稿与报价，提示词冻结进入对应正式生成消费者后才记消费完成，不能静默覆盖用户本次输入。

### 8.4 失败与验证

同源 Origin、UUID 幂等、实时主体/组织状态、项目范围、归档与 revision 均由 Go 复核；权限失败不触发升权。前端使用 Swagger 生成客户端、RHF/Zod、TanStack Query 与 shadcn，保留未确认草稿、冲突后明确载入/放弃操作，卸载与切换清理订阅。测试覆盖真实 PostgreSQL 重读、重试/异体、首次并发写、停用身份、跨组织项目、归档、不可用模型、提示词模式/变量/受保护层与 revision 变化。默认 producer 的 admin 403 与偏好可写分别验证；模拟供应商和浏览器 fixture 不计真实生成通过。

## 9. 服务端时间轴导出与本地媒体处理合同（2026-10-01）

固定源 `1ae25027` 的 `task_timeline.go`/`task_render.go` 将 timeline_render 与 timeline_transcription 作为本地后台任务，绕过模型渠道；前端另有 FFmpeg 规划、字幕 SRT 与有条件烧录。迁移保留这些能力，并改造成现有 Go／Temporal／私有媒体／Outbox 链。禁止为 FFmpeg 伪造 AI 模型、价格或 supplier 事实；已有 Operation 报价生成合同不承担本地渲染。正式 `media-tool.ExportJob` 负责本地执行事实，编辑配置仍由 canvas.timeline 持有，浏览器不维护任务真相。

导出创建只接收已保存 `{canvas_id,node_id,revision}`，在命令事务中经消费方 `TimelineReader` 读取同项目 timeline 节点，在 document 锁内验证 revision、类型、唯一配置合同与素材 node/asset 双身份。媒体拥有自己的授权与读取端口，工具仓储不得直接读写 canvas/media 表。所有输入为 ready/passed 的正式项目资产，锁定 ID、revision、SHA256、字节数、检测类型和服务器选择的私有对象 key；原件只由 Worker 接管并再次校验内容，禁止浏览器直传 URL／服务器路径。canvas 节点删除或再次编辑不能改写已冻结的导出输入。

POST `/api/projects/{pid}/media-exports` 创建后台事实与 Outbox；GET 项目列表按 canvas/node 查回，GET `/api/media-exports/{id}?project_id=...` 返回明确状态与进度。进度为 0..100 整数百分比，`queued`→`running`→`review_required`→`succeeded`；`failed` 保留失败码和 frozen input，`cancel_requested` 仅代表持久请求，只有执行退出／未取得执行权的证据才进入 `cancelled`。源固定身份随 job 保存，可在刷新后恢复。取消、失败重试和审核均使用 UUID 幂等键与 job revision，重复相同请求返回原 receipt，变体拒绝；任务 attempt 是取消／重试与旧 Worker 回写的 fence。每次状态、发布、审核与控制事实连同审计／必要 Outbox 在事务内提交。Temporal 启动采用 job/attempt 固定身份、先验数据库 receipt，重复消息不重复创建任务；已持久结果的重放直接核对结果，不再次渲染。

真实渲染遵守保存的轨道可见／静音、片段起点与源裁切、空隙黑场、图片、视频与独立音频混合、音量／淡入淡出、横竖与方形比例、FPS、文本与字幕。使用受控本地 FFmpeg 参数与输入文件，不插入用户文本为滤镜语句。所有轨道和 1000 clip/24h 编辑边界保留；工作文件数量、输入／输出字节、解码尺寸与执行时间另设资源预算，超限明确失败，不默默截断输出或省掉轨道。字幕同时保留完整 SRT 下载。当前机器 FFmpeg 无 drawtext/libass；中文／混合字符／换行字幕由 Go x/image/font 和固定 OFL Noto Sans CJK SC 绘成透明帧，经实际 overlay 烧录，支持字体大小、色彩、位置、描边；仅引入一份服务端字体，禁止进入前端包。

渲染结果沿用 media 的字节哈希、实际 ffprobe、真实 poster/proxy、私有对象检查与领域状态，产物为 `origin=system`、`processing/pending`，不能预先宣称未知输出已通过审核。`GET /api/media-exports/{id}/preview?project_id=...` 只对当前有权查看该 job 的操作者提供短签名原件与 `{job_id,revision,sha256,url,expires_at,asset}`；该临时查看不使资产进入普通 ready 列表。操作者实际查看后 POST `/api/media-exports/{id}/review` 提交 `{project_id,revision,sha256,local_review_confirmed:true}`，在锁内确认正是当前产生的文件，记录独立人工审核身份／时间／SHA 证据，事务内切换媒体 ready/passed 与 job succeeded。审核前不能作为其他任务输入或下载正式成片；取消不销毁未知提交，私有未通过产物按媒体保留／清理合同处理。归档保留只读 job／已通过预览，拒绝新执行与审核写入。

源 frame／trim／audio／combine／captions 继续作为此本地媒体处理边界的实际消费者迁移，不能用空端点或样例文件代替。whisper.cpp transcription 必须配置与真实可用性验证后接入；当前未建立实际 Whisper 服务与可分发识别模型，转写明确 pending，不生成假段落或字幕。首个验收为真实授权素材→已保存 timeline→Outbox／Temporal 接管→实际 FFmpeg 文件→processing/pending→实际预览与人工审核→正式资产／下载及刷新恢复；单元／模拟 Workflow 验证不能替代这一验收。

### 9.1 已实现的导出边界与验证事实

实际公开合同是 `POST /api/projects/{pid}/media-exports` 提交 `{canvas_id,node_id,revision}`，以及同路径 GET 恢复有界 job 列表（可选 canvas_id、node_id、绑定项目及筛选的 cursor）。`GET /api/media-exports/{job_id}?project_id=...` 返回唯一持久 job；preview、download、subtitles 后缀分别提供实际待审核原件、通过审核后的 MP4 附件、冻结可见字幕／文字轨道的 SRT。POST review、cancel、retry 使用独立 Idempotency-Key，按 SHA／job revision 或当前 job revision 判定，不能使用 asset revision 代替 job revision。所有 job 状态、进度与结果都由 mediatool 保存，timeline 的编辑输入不保存这些运行事实。

070 SQL 使用非拥有者 `lanverse_app`：仅新增 mediatool schema USAGE、job SELECT／INSERT／可变运行列 UPDATE、command SELECT／INSERT。冻结 JSON、来源身份与创建时刻没有 UPDATE 权限，历史没有 DELETE 权限。API 接受事务同时提交永久请求回执、独立执行 Outbox 和严格平铺摘要的 audit Outbox；现有 audit consumer 负责实际解析和入库。最多两个 queued／running／cancel_requested job 可并发占用一个项目。每次显式 retry 增加 attempt，旧活动不得覆盖新尝试。活动会话 `active_worker` 只存在内部数据库，不能由浏览器提交；只有对应会话从实际子进程和文件清理返回后，才能登记运行中取消完成。无法确认活动退出的取消保持 cancel_requested。已产生并完整登记的 review_required 文件则证明渲染结束，可在取消事务中拒绝 pending 媒体并登记 cancelled。

FFmpeg 先做真实原件 SHA／kind／像素尺寸检查，裁切以整数 `crop=...:exact=1` 在最终缩放前处理；音频用原生逐 sample afade，并保持片段在整条 clip 中的时间位置。渲染按 frame 边界分段，每段最多 32 个活动轨道和一个字幕 PNG 管道，每段及最终文件均检查真实时长，防止限额截断冒充完整输出。原件暂存合计上限 2 GiB，中间段合计 2 GiB，最终原件 500 MiB；超过限额明确失败。24 小时／1000 clip 是编辑合同边界，不代表任何素材一定能在这些实际处理资源上限内导出。

已在隔离 PostgreSQL／MinIO 与实际 FFmpeg 下执行：正式 canvas 保存与修订冻结、空间 crop 像素检查、拼接空隙、中文与混合字符／换行／三种对齐／透明背景及描边、PCM 解码后的音量和淡入淡出、private preview 原件 SHA、pending 不可正式引用／下载、明确审核及相同回执重放、通过后附件与 SRT、刷新列表、严格 audit consumer 重放、旧 worker session／旧 attempt 竞争。使用实际 `SET ROLE lanverse_app` 的连接验证非 superuser／非 table owner 及冻结字段不可修改。生产 Activities 已在 Temporal SDK 测试环境中运行真实上述渲染和取消；这不等于真实 Temporal／Kafka／浏览器的运行验收，后者仍需由组合根接线后实际执行。

### 9.2 音频提取接续合同

完整迁移继续在同一 ExportJob 增加可选 `output_kind=video|audio`，缺省 video 保持当前请求与用户流程。字段必须在创建时验证并进入冻结输入／请求指纹／公开 job 投影，不允许在运行中变更。audio 仍只接受保存的 timeline 来源；将实际可见、未静音的 audio／video 轨道按 trim／volume／fade／时间位置混合为 M4A，复用同一授权、活动会话、取消、Outbox、pending→实际预览→明确审核→正式下载链。音频媒体使用实际 probe 的时长、channels、codec，不能填写视频像素字段；提供实际 waveform，不伪造 poster。若可见源视频没有真实 audio stream，且没有其他可用音频轨道，明确返回无可提取音频失败，不把生成静音文件宣称提取成功。该接续仍须 Red→Green 与真实私有媒体验收，不能以本节合同存在或当前视频导出通过宣称音频提取／Whisper 已完成。


## 10. 提示词模板消费与报价冻结合同（2026-10-01，接线评审）

### 10.1 固定来源的真实消费者

以下事实来自固定 `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98`，不是运行验收：`backend/internal/app/provider.go:290-299` 是唯一统一编译调用，由 `metadata.promptTemplateOperation/promptTemplateVariables` 驱动；第 292 行明确视频模式不编译，最终视频提示词保留输入框内容。`web/src/pages/projects/index.tsx` 的旧大纲入口当前未被 `/projects` 路由装配；章节/角色详情受 localMode 重定向限制；`/skills` 关闭。九项定义全部保留，但来源中未装配或旁路的入口不算九条已运行生成链。

| 模板 | 固定来源调用事实 | 目标编译所需权威上下文 |
| --- | --- | --- |
| chapter_assets_extract | `project-chapter-ai.ts` 的实际章节资产创建使用此模板 | 已保存章节正文/名称、项目名称与画风 |
| character_extract | 现章节代码仅用于旧任务结果识别，没有新的独立创建调用 | 同上，角色输出仍按独立契约校验 |
| character_turnaround | `project-character-media.ts` 创建任务元数据，项目详情本地路由受限 | 已确认角色名称/设定/版本、项目画风及正式参考 |
| storyboard_plan / storyboard_repair | 前端只有操作常量，没有当前模板创建调用；章节分镜走自定义提示词 | 真实剧情、资产/角色版本、镜头数量/时长规则；repair 另需原输出和真实校验错误 |
| storyboard_first_frame | `canvas-project-domain.ts` 为含变量的分镜行写元数据 | 已保存镜头的首帧构图、表演起始状态、项目视觉、负面要求 |
| storyboard_video | 前端可写分镜视频元数据，但服务端明确旁路模板 | 最终提示词保持用户输入；不得静默应用个人模板 |
| short_drama_outline | 旧项目页用大纲模板创建文本任务并导入章节，当前旧页未装配 | 用户本次故事 + 七项明确故事选项，项目权限复核 |
| skill_draft | `lib/canvas/skill-drafting.ts` 创建模板任务，调用页 `/skills` 关闭 | 用户本次技能想法；保存技能/采用结果另按实际业务合同 |

现 Lanverse 的运行实体是 workspace.project、canvas 资源/生成/批量表、Operation 与 media；script、bible、storyboard、skill 正式实体尚未实现。接通模板编译与报价不替代实体、原入口、结构化结果校验/采用及真实模型执行验收。不搬源 SQLite、Wails、浏览器 metadata 直达供应商或第二套本地任务。

### 10.2 闭合请求与 application 端口

公开 `QuoteItemRequest` 与内部 `FreeQuoteItemInput` 增加可选 `prompt_template`，未提供时保留原来的直接提示词链与指纹。请求类型归 prompt/application：`TemplateRequest={operation, expected_template_id, expected_customization_revision, outline?}`；operation 仅九项固定标识，expected_customization_revision 显式提供，首次为 0。outline 只用于 short_drama_outline，七项值沿固定源枚举：chapter_count 3/5/8/10、structure 单线推进/双线并行/群像多线/反转嵌套、word_count 500/800/1200/2000、perspective 第三人称/第一人称/多视角、tone 平稳叙事/轻松喜剧/紧张悬疑/热血成长/甜宠治愈、character_scale 2 个/3-4 个/5-6 个、chapter_length 短/中/长。其他操作拒绝 outline；不得公开任意 variables、system_prompt、输出 schema、身份或项目事实覆盖字段。

prompt/application 的 Compiler 消费小接口 `CustomizationReader.ReadCustomizations(ctx, actor)`，由组合根注入位于同一报价事务的 pgprompt Store。`Compiler.Prepare(ctx,actor,request,runtime)` 返回 `Preparation`：原用户输入、最终提示词、operation、policy、基线 ID/版本、个人定制 ID/revision 与最终内容 SHA256。runtime 仅来自已复核权限和修订的正式业务 application 投影，包含 project_id、capability/mode、原用户输入与内部 ServerValues；Client 不能提交 ServerValues。业务尚不能产生某模板所需权威上下文时返回明确 context_unavailable，不用前端声称的章/角色/镜头事实补齐。

六个结构化模板匹配 text.structured；首帧与三视图匹配 image.generate；视频匹配 video.generate 并返回 policy=bypass_video，不读取/应用个人模板，不改变原提示词。纯 domain.Compile 仍可预览九项定义；该预览不能触发视频生成编译。普通生成只有显式选用 prompt_template 才消费偏好；原用户本次输入作为受保护本次要求保留在最终文本，不能被 inherit/append/rewrite 消去。服务端基线/输出约束仍在本次要求之后形成最终保护层。

### 10.3 单一报价与冻结事实

事务顺序为实时主体/项目授权 → 稳定请求键锁 → 按原始请求（含可选模板请求）优先重放既有报价 → 锁定并验证已保存 canvas source/原用户 prompt、参数、引用、修订 → 读取权威上下文与当前个人模板修订 → Compile → 当前模型/价格/预算及完整输入指纹 → 插入 Operation、最终 prompt 输入、模板冻结证据与报价回执。任一失败均不插入部分 Operation，不执行供应商，不扣款。

operation/application 新增 `QuotePromptCompiler` 消费端口及 `PrepareFreeQuoteWithTemplate`；原 `PrepareFreeQuote` 保持直接输入行为。prepared 结果带 `PromptPreparation`，由 operation adapter 持久化；最终提示词用于 OperationInput、输入哈希、字符/Token 估价，原输入仅用于 canvas/cfg 来源校验及请求幂等。模板证据包括 operation、policy、baseline/version、personal revision、content hash，在同一事务进入冻结报价事实；不建立第二份任务。nil 模板不改变旧 JSON 请求指纹，带模板的规范请求字段完整纳入同键异体校验。

同键重放优先返回第一次冻结结果，之后个人偏好变更不得重新编译/重复报价；确认、重试、Worker 恢复均读取冻结文本，不能再次读取当前偏好。不同请求键且模板 revision 过期返回 template_changed；普通保存偏好不修改已有草稿、报价或执行任务。source revision/cfg hash、模型版本、价格版本、预算和费用门禁保持既有行为。是否复用已完成产物仍依最终实际输入的既有指纹判断，不能凭模板名字视为等价。

公开报价返回可选 prompt_preparation 供用户在确认生成前审阅（policy、操作、基线/个人修订、最终提示词与哈希）；视频明确显示模板未应用。该内容是当前组织项目内本次已选生成输入，不开放读取其他主体的偏好目录。任务和费用状态仍由唯一 Operation 决定；保存/编译/报价成功均不等于执行、审核或结算成功。

### 10.4 失败与验收边界

非法操作/枚举/变量与不匹配 capability 返回 422 template_invalid；模板或个人修订变化返回 409 template_changed；缺少已接入的正式上下文返回 409 context_unavailable；存储/编译依赖失败返回 503 dependency_unavailable。视频旁路是明确 policy，不伪造“已应用”。编译后仍遵守既有最终 prompt 64 KiB 和输入/参数/项目大小限制，不扩张 provider 任意系统指令或工具权限。

测试依次覆盖：闭合 DTO、所有声明模板的 application 编译/角色与受保护输入、修订与模式/类型、视频字节不变且不读取偏好、context 缺失/错误、最终估价与指纹；真实 PostgreSQL 覆盖同事务冻结、同键偏好变更重放、不同键 stale 拒绝、审计/存储故障无部分写；公开 Handler→Swagger→generated client→浏览器明确采用和预览。source 缺少的入口、九个业务结果消费者、真实文本/图片/视频供应商和应用到正式实体分别登记未通过；模板函数或 mock 不关闭完整迁移。

### 10.5 当前已实现与未验收事实

组合根使用 `pgoperation.NewStoreWithPromptCompiler(database, factory)`，factory 从同一 `gorm.DB` 报价事务构造 pgprompt Reader；原 NewStore 的直接输入链不读取偏好，未注入 compiler 的模板请求明确失败。事务先复核主体与项目、同键回执，再以原始 prompt 校验保存的 generation/batch source，之后编译及计算最终文本的报价与输入指纹。当前权威上下文仅有锁定的项目名称/画风；六项依赖章、角色或正式镜头的模板继续返回 context_unavailable，不接受客户端变量补齐。outline/skill 已可通过报价端口冻结，尚无对应正式文本业务实体的创建/结果采用验收。视频维持输入原文且不读取个人偏好。

SQL 080 新增独立 `operation.operation.prompt_preparation` JSONB，闭合证明字段、版本、策略、能力、哈希及个人修订形状；现有运行角色的列 UPDATE 授权不包含此列。该列不保存最终文本；文本仍为唯一 operation_input 的 prompt。报价回执中的 `final_prompt` 是该冻结输入的审阅投影，稳定重放时不会再次编译。任务详情恢复同一证明并复核冻结文本哈希。报价目前验证了既有按字符计价的最终文本路径；per_1k_tokens 仍须有已接受的输入估计与模型输出上限事实，缺少时依既有计费门禁拒绝，不伪造 Token 数或完成验收。

隔离 PostgreSQL 17.11 实测通过个人偏好修改后的旧报价重放、同键异体冲突、新键 stale、项目/主体范围、缺实体、batch 逐项排除、事务回滚、原 canvas prompt 与 revision 门禁、nil 模板的历史 JSON 指纹逐字保持、HTTP 嵌套未知字段拒绝和刷新证明。`SET LOCAL ROLE lanverse_app` 的实际写入及不可更新证明列通过。该证据不等于当前在线服务已切换非 owner 数据库账号，也不等于 PostgreSQL 18.4、真实供应商或九项正式业务消费者验收。
