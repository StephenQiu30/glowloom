# BeefTV 无限画布迁移设计

日期：2026-09-30。状态：**用户确认迁移方向与固定源码，本轮采纳以下核心迁移和正式资源合同；实现、真实集成和产品验收分别记录。**

## 1. 目标与范围

Lanverse 按上线项目标准实现完整产品，当前优先跑通整项目验证和 Demo。Demo 使用同一套正式入口、权限、模块与数据合同；一次演示、一条生成链路或基础画布不能替代 PRD-01 §8.2 的九个 MVP 场景和上线门禁。

用户指定直接复用 **BeefTV 无限画布及对应设计**，替换现有画布以减少重复开发。范围为 DOM/SVG/rAF 引擎、视口/缩放、框选/拖动、对齐/分组、连接、快捷键、历史、小地图、空间索引、LOD、媒体播放设计及 Lanverse 正式节点/服务端接线。不得降为外观借鉴、自绘 React Flow 替代品或纯备注画布。

本轮不是迁移整个 BeefTV 应用：供应商执行器、3D/深度、插件运行时、时间轴、Wails、SQLite/IndexedDB、发布脚本和上游部署配置不在范围。Lanverse 镜头/资产/生成节点、reference/promote/run、报价确认、正式选定及 Agent 仍按原业务设计逐条接入，既不缩减 MVP，也不强迫迁入 BeefTV 全套生成系统。

## 2. 固定源码与直接复用

当前主要参考项目与源码来源：[glanderness/BeefTV](https://github.com/glanderness/BeefTV)，用户确认固定提交 [0d9e9f48d407570cd431ad9730cdd522b06810c0](https://github.com/glanderness/BeefTV/tree/0d9e9f48d407570cd431ad9730cdd522b06810c0)。本次参考项目更新保持第 1 节的无限画布复用范围，不据此迁入整个应用；LibTV、旧 infinite-canvas 与 BeefTV 852961a1 调研保留历史出处，不再作为当前画布选型建议。

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

继续 Next.js、React、TypeScript、shadcn/ui、TanStack Query、Zustand；核心与 Lanverse 业务绑定按职责分开：迁入运行/算法位于 features/canvas/engine，节点位于 nodes，正式装配由 workspace.tsx 的 CanvasWorkspace 承担。Go 保持 adapter → application → domain。

| 端口 / 状态 | 正式合同 | 所有者 |
| --- | --- | --- |
| 身份/项目 | 当前会话、首登改密、组织项目、只读权限 | identity/workspace，不映射样例 ID |
| 文档/节点 | document/node/edge/viewport/revision；节点 title/parent_id/z_index 与类型安全 config | Go/PostgreSQL；source k ↔ API zoom 显式转换 |
| 命令提交 | expected_revision、UUID 幂等键、确认文档/结果或结构化错误 | 在线 Swagger 生成 API → request.ts |
| 媒体 | 同项目已接管可用 media_asset ID，授权 URL/缩略图由服务端投影 | media，不保存任意 URL/blob/本机路径/storageKey/密钥 |
| 业务对象/动作 | ref_type/ref_id、摘要、参考/报价/任务/候选/正式选定 | 与流水线共用业务模块/query key，不迁 source executor |
| 交互/历史 | 选择/拖动/视口/草稿与成功操作反向历史 | Zustand/页面内，刷新从服务端恢复 |

本轮资源类型闭集为 text/image/video/audio/group，node_action=resource；配置闭集 text={text:string}、group={collapsed:boolean}、media={}；媒体只引用授权同项目 ready/passed 已接管对象，经独立媒体 preview 端口取得临时 URL，不放 Document.Node。标题使用正式字段，禁止将上游任意 metadata 当可写配置。原布局命令 AddNodes/MoveNodes/UpdateNodeConfig/DeleteNodes/Connect(annotation)/Disconnect/SetViewport 保留，扩 ResizeNodes/RenameNodes/SetNodeParents/SetNodeZIndex；Connect/Disconnect 使用 edges/ids 数组，精确形状及画布名称/软删除、媒体列表/preview 合同见 DES-38 §3.2。

### 3.1 Next.js 与 shadcn/Radix 接入调整（2026-09-30）

用户本轮重申复用画布并使用 Next.js + shadcn + Radix。仓库已经是 Next.js 16 App Router，`components.json` 为 radix-nova；现有 `InfiniteCanvas`、空间索引、连线与正式 Go 保存链直接沿用。复核上游固定 SHA 的 `web/package.json` 与 InfiniteCanvas：上游 Vite/Bun、AntD 与本地工作区不是 Lanverse 的运行依赖，不复制这些工程配置。

必要调整限定为画布控件与交互边界：模式按钮改为官方 Radix 单选 ToggleGroup，始终保留框选/移动之一；动作提示使用 shadcn Tooltip；SelectItem 放入 SelectGroup；错误、空态与加载使用现有 Alert、Empty、Skeleton。工具按钮的方向键交给控件焦点导航，不能同时产生 MoveNodes；画布全局保存/撤销等快捷键继续有效。错误恢复、失败幂等键、只读与保存禁用规则保持。

Next 页面保留 Server Component 与 Suspense；浏览器引擎在 Client Component 中动态加载，`ssr: false` 只放客户端边界。不增加第二份画布状态或新的 API。验证包括方向键冲突回归、现有组件与保存测试、生产构建及桌面/手机浏览器控件验证；合成 API 的浏览器验证单列，不能代替真实 Go/PostgreSQL 或供应商验收。

官方组件用法依据 [Radix ToggleGroup](https://ui.shadcn.com/docs/components/radix/toggle-group)、[Tooltip](https://ui.shadcn.com/docs/components/radix/tooltip)、[Select](https://ui.shadcn.com/docs/components/radix/select)；同时核对 Context7 的 shadcn 4.21.0 文档和本机 Next.js 16.3.6 附带指南。

分组采用源 world 绝对坐标：移动组由客户端批量包含 children，统一 delta；parent 仅指同文档 group 且无环；删除 group 解组，保留 children 及绝对位置。删除普通节点移除关联边，不删除业务对象。zoom=0.05～4。正式命令更新标题、尺寸、层级；DTO/白名单/大小/批次限制同步 DES-02/03/06/38 与 swag 类型，不手改生成物。

沿用 canvas.document/node/edge/command_log 和已发布迁移；字段/约束追加 forward SQL。快照端点/恢复合同不在本轮自行扩展；未来恢复产生新 revision，不能回退历史。镜头/生成/reference/promote 等按既有业务合同接入，未就绪能力明确不可用，不造样例任务/费用。

## 4. 入口、删除与数据保留

正式 /canvas 与 /projects/{UUID}/canvas 使用同一 CanvasWorkspace；项目路由传 initialProjectId，选择与读取使用真实项目/画布 ID 和真实登录/首登改密。业务入口、组件/服务命名、按钮和产品文案不含 poc 或 livedemo；不保留旧画布兼容入口。

删除 frontend/src/features/canvas 中旧 poc-*、live-* 引擎/编辑器/状态及旧用例，删除 frontend/src/app/poc/canvas 和旧 features/workbench/creation-canvas.tsx，实现正式路由接新引擎。替换消除双引擎/双状态，不能只改标签。专用依赖/样本确认无其他消费者再移除，不误删其他工作台和共享认证/请求模块。

源码替换不授予清库权限。保留历史 DDL、画布记录、revision、日志、账号/项目及其他业务历史；旧备注在新引擎可呈现/编辑。未知历史值不可静默覆盖/清空，变更追加迁移并验证恢复/回滚；不得重建数据库或导入 source local workspace 覆盖项目。

验证和测量放在正常命名 tests/e2e/acceptance；synthetic fixture 明确来源。Demo 使用正式引擎，不增独立 PoC 产品页或第二份正式持久化。固定媒体、真实媒体、目标机器和跨设备分别留证，旧引擎结果不移作新引擎通过证明。

## 5. 失败、权限与幂等

- 每次读写复核账号/组织/项目；跨组织不可见，归档只读，撤权/未改密阻断写入。媒体及业务引用复核授权。
- 同键同体回放首次确认结果，同键异体拒绝；revision、节点/边、日志、幂等回执和 Outbox 原子提交，任一命令失败全批回滚；错误码与 DES-03 统一。
- 交互可预览，服务端确认前不显示已保存。网络失败保留草稿，以原键重放/查明结果；409 取最新正式文档并提示，不能静默覆盖他人配置或重放付费操作。
- 复制新 UUID，清除不可继承的任务/选定身份；撤销/重做提交新允许命令，不回退 revision，不撤销付费生成/正式选定。
- 媒体失败可恢复，预签名过期重新授权；切换项目/卸载释放监听、pointer capture、rAF、计时器、播放/blob URL；只读取消在途手势。
- 保持同源 /api、Origin/CSRF、环境 Cookie、RFC 9457；浏览器不持有供应商/中间件/存储管理密钥。2026-09-30 用户另行要求移除 Python 服务目录，范围见 [Agent 服务目录清理](Agent服务目录清理设计.md)；Provider Activity 的 Go 承接继续由 M1-12 设计与验收，画布迁移不代表生成链已完成。

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
