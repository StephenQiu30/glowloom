# Glowloom 无限画布同步机制复核

日期：2026-10-24。研究性质：只读源码与官方文档调研，外加安装版 Yjs 的两个小型语义探针。研究结论与产品修复分开记录；真实双账号三页面验收归 BACKLOG 及专项证据，不能用源码研究代替运行验收。

## 1. 当前判断及边界

**现在没有证据支持为了修复素材抖动而更换 Yjs，更不建议立即自研通用 OT。** 对本项目“同段 Tiptap 富文本合并、节点属性并发、个人撤销、断线留页继续输入”的组合要求，成熟 Yjs 是当前更有依据的合并内核。但“Yjs 是最优解”尚未成立：目前没有统一负载下的端到端延迟、内存、带宽、拒绝恢复成功率对照。它提供收敛机制，不提供业务接受、持久回执、权限、资产身份、完整恢复交互或无抖动渲染。

目前更准确的架构名称是 **Yjs 合并 + Go 权威接受 + PostgreSQL 持久事实 + 页面内存恢复 + 独立媒体显示层**。这与“普通 y-websocket 房间广播所有本地更新”明显不同。最值得继续投入的是这些边界的正确性、增量传输和可观测性，而不是把已存在的 CRDT 合并重新实现为 OT。

若产品以后明确要求所有互斥图操作按服务端接收次序裁决，且绝大多数业务修改都必须先验证，集中式图命令/属性 diff 加富文本 Yjs 可成为候选。但那是新的一致性模型：跨图与正文的事务、删除、引用和撤销需重新定义；不能只把位置换 OT 就认为架构已简化。若确要比较 OT，应先用 ShareDB/成熟 OT 类型实现受限原型，而不是从零设计 transform。

上述优劣比较是工程判断，并非统一负载下的性能结论。研究后用户于2026-10-24明确要求清理OT并全面采用Yjs；该决定已纳入需求10、设计17、PRD09及计划19。实现进度和验收证据仅记录于BACKLOG，本研究保留对性能、业务约束和替代方案的比较边界。

## 2. 研究方法与固定版本

通过已连接 GitHub 插件获取默认分支 HEAD，再以 commit SHA 读取具体代码；在线资料只采用项目官方页面。Yjs 主分支已不同于安装版，因此运行语义依据安装版 **13.6.33** 及其对应提交，而非主分支的新实现。

| 项目 | 固定提交 | 查证范围 |
| --- | --- | --- |
| tldraw/tldraw | `12921f6d3996b4330688113371cd88890b673ab5` | TLSyncClient、TLSyncRoom、diff、sync-core package、许可证 |
| excalidraw/excalidraw | `b7de45e88111f5680ae4bb3c3f3541c7d3b279fe` | reconcile、Portal、许可证 |
| toeverything/AFFiNE | `fbf2cebfa012a7c92b08af7bfa0340177308b2a6` | BlockSuite surface model、仓库许可证 |
| yjs/yjs v13.6.33 | `9ea3a4fb57b814292ce18dfd13c4f422e44e6c5c` | Item 冲突排序、Transaction、许可证 |
| share/sharedb | `ea090f5204f348aca0055fdca3236568473ca3e4` | submit-request、client/doc、许可证 |
| ottypes/json0 | `90a3ae26364c4fa3b19b6df34dad46707a704421` | transformComponent、package |
| ottypes/rich-text | `b53cd97690804e544370423ce27c6d852abb2c7f` | Quill Delta 类型包装、许可证 |

原始代码按表中 SHA 固定，以下链接可独立复核。没有安装或运行上述第三方服务，也没有对其线上平台做黑盒写入。

## 3. 主流公开实现具体做了什么

### 3.1 tldraw：中心权威记录同步与乐观重放

`TLSyncClient` 保持 pendingPushRequests 和 speculativeChanges。收到服务端 patch/结果时，撤下乐观层，应用权威变化，再重放未确认修改。客户端直接说明 push/pull/rebase，实际 `rebase()` 也实现这一过程；服务端计算实际记录变化，返回 commit、discard 或 rebaseWithDiff，使用 documentClock/clientClock 匹配结果。[客户端实现 L969–1028](https://github.com/tldraw/tldraw/blob/12921f6d3996b4330688113371cd88890b673ab5/packages/sync-core/src/lib/TLSyncClient.ts#L969-L1028)、[服务端结果 L1574–1635](https://github.com/tldraw/tldraw/blob/12921f6d3996b4330688113371cd88890b673ab5/packages/sync-core/src/lib/TLSyncRoom.ts#L1574-L1635)。

这证明无限画布可以不用 Yjs；也不能据此把 tldraw 称为通用富文本 OT。其公开实现面向 records/diffs，客户端的 rebase 不等于具备 JSON0 或富文本操作的完整 transform 代数。重连也是把页面保留的修改重新建立在服务端状态上，不能直接继承为我们的“原请求不可变且持久终态可查”合同。

官方要求同一房间只有一个权威 room，并区分内存与 SQLite 存储；内存默认并不自带重启持久性。对 Glowloom 的启示是：接受基线与乐观层必须分离、显示层要保留实体身份、presence 与业务记录分开。不是替换 Go 权限和 PostgreSQL 事务的理由。[官方 sync 文档](https://tldraw.dev/docs/sync)。

### 3.2 Excalidraw：元素版本仲裁，编辑手势保护

`shouldDiscardRemoteElement` 会保护本地正编辑文本、缩放或创建的元素；其他情况比较 element.version，相等时用 versionNonce 确定性选择。`reconcileElements` 最终选取整份本地或远端元素，再校正排序。这是元素级仲裁，不保证同一元素两个属性或同段文字的所有贡献同时保留。[reconcile.ts L24–43、L74–127](https://github.com/excalidraw/excalidraw/blob/b7de45e88111f5680ae4bb3c3f3541c7d3b279fe/packages/excalidraw/data/reconcile.ts#L24-L127)。

Portal 只发送版本变化的元素；场景消息加密传送，文件上传走 fileManager，鼠标消息走独立可丢弃通道。[Portal.tsx L85–182](https://github.com/excalidraw/excalidraw/blob/b7de45e88111f5680ae4bb3c3f3541c7d3b279fe/excalidraw-app/collab/Portal.tsx#L85-L182)、[鼠标消息 L202–224](https://github.com/excalidraw/excalidraw/blob/b7de45e88111f5680ae4bb3c3f3541c7d3b279fe/excalidraw-app/collab/Portal.tsx#L202-L224)。

可借鉴其手势期保护和素材独立传输。不能将其直接用于我们同段富文本合并，也不能把 socket 广播成功解释为我们的业务持久成功。

### 3.3 AFFiNE / BlockSuite：Yjs 已用于自由画布，但不是仅接一个 provider

surface 元素集合为嵌套 Y.Map；元素创建时放入元素自己的 yMap，并由 store.transact 包裹；文本属性使用 Y.Text，集合观察事件区分 transaction.local。删除还处理组结构和依赖。它证明 Yjs 在真实画布模型中可用，同时显示应用仍须处理图结构语义。[surface-model.ts L29–34](https://github.com/toeverything/AFFiNE/blob/fbf2cebfa012a7c92b08af7bfa0340177308b2a6/blocksuite/framework/std/src/gfx/model/surface/surface-model.ts#L29-L34)、[文本转换 L548–575](https://github.com/toeverything/AFFiNE/blob/fbf2cebfa012a7c92b08af7bfa0340177308b2a6/blocksuite/framework/std/src/gfx/model/surface/surface-model.ts#L548-L575)、[创建与删除 L709–810](https://github.com/toeverything/AFFiNE/blob/fbf2cebfa012a7c92b08af7bfa0340177308b2a6/blocksuite/framework/std/src/gfx/model/surface/surface-model.ts#L709-L810)。

这里只查证该层模型，不能从 surface 代码推断 AFFiNE 的服务端每笔编辑都遵守 Glowloom 的拒绝/回执合同，也不能照搬其离线存储策略。

### 3.4 React Flow：渲染/交互组件与同步方案是两层

官方 Collaborative 示例使用 `@xyflow/react`、Yjs、y-websocket，但示例属于 xyflow Pro License。公开页面足以证明其示例选用 Yjs，不足以公开复核付费示例的所有故障行为。未取得示例源代码，不把它列为固定 SHA 源码证据，也不声称组件内置可用的生产协作服务。[官方协作示例](https://reactflow.dev/examples/interaction/collaborative)。

## 4. Yjs 与 OT 真实差异

### 4.1 Yjs 能做与不能做

官方定义更新具有交换、结合和幂等性，所有相关更新都到达后收敛；这不代表任意业务图都合法，也不代表所有用户意图都会显现。[更新语义](https://docs.yjs.dev/api/document-updates)。

Y.Map 同 key 并发写依照因果及结构 ID 决定顺序，不能保证最后到达服务器者获胜。安装版 Item 的冲突排序明确比较 client ID。[Item.js L450–485](https://github.com/yjs/yjs/blob/9ea3a4fb57b814292ce18dfd13c4f422e44e6c5c/src/structs/Item.js#L450-L485)。这不应简化成全部场景“最大 clientID 胜出”，因果关系也参与排序。

Yjs transaction 是本地操作批次/观察边界，不是数据库事务。安装版 `transact` 在 finally 执行清理，没有异常自动回滚。[Transaction.js L414–452](https://github.com/yjs/yjs/blob/9ea3a4fb57b814292ce18dfd13c4f422e44e6c5c/src/utils/Transaction.js#L414-L452)。因此先把非法更新合入公开房间再 throw，不能可靠地实现“拒绝前不传播”。我们的隔离候选思路正是必要的业务保护。

UndoManager 支持按 origin 选择性撤销，但不会自动知道哪些素材/节点被其他人引用、恢复的节点是否为旧实例。应用仍须定义删除生命周期、依赖规则和恢复权限。[官方 UndoManager](https://docs.yjs.dev/api/undo-manager)。

官方 y-websocket 还具有同浏览器跨标签页通信，包括 localStorage fallback；其 synced 表示连接同步，不是 Go 数据库的特定提交回执。因此本项目不能直接以默认 provider 替代权威通道，更不能重新接入 y-indexeddb 来规避断线问题。[官方 y-websocket](https://docs.yjs.dev/ecosystem/connection-provider/y-websocket)。

### 4.2 成熟 OT 需要的服务端、客户端与类型系统

ShareDB 服务端把旧版本操作逐项 transform 到当前版本，执行 apply/commit 中间件，将操作与快照原子提交数据库，再 publish；并发提交会重试。原 src/seq 用于防止重连重复执行。[submit-request.js L158–263](https://github.com/share/sharedb/blob/ea090f5204f348aca0055fdca3236568473ca3e4/lib/submit-request.js#L158-L263)、[重复操作判定 L284–305](https://github.com/share/sharedb/blob/ea090f5204f348aca0055fdca3236568473ca3e4/lib/submit-request.js#L284-L305)。

客户端同样有 inflightOp、pendingOps，并在远端操作到达时进行双向 transform。无法安全回滚时，hardRollback 清除待处理队列并重新 fetch，通知错误。这种默认恢复行为不能直接满足我们“保留用户输入供恢复”的要求。[client/doc.js L325–385](https://github.com/share/sharedb/blob/ea090f5204f348aca0055fdca3236568473ca3e4/lib/client/doc.js#L325-L385)、[hardRollback L1054–1100](https://github.com/share/sharedb/blob/ea090f5204f348aca0055fdca3236568473ca3e4/lib/client/doc.js#L1054-L1100)。

JSON0 的 transformComponent 包含嵌套路径、列表插入删除移动、对象替换、子类型和 invertibility 处理。不是为 x/y 写一个加减函数就拥有 OT。[json0.js L414–550](https://github.com/ottypes/json0/blob/90a3ae26364c4fa3b19b6df34dad46707a704421/lib/json0.js#L414-L550)。rich-text 类型基于 Quill Delta；当前项目是 Tiptap/ProseMirror/Y.XmlFragment，迁移要重做格式/引用/选区/IME 绑定，不能替换一个 npm 包即完成。[rich-text/type.js L1–69](https://github.com/ottypes/rich-text/blob/b53cd97690804e544370423ce27c6d852abb2c7f/lib/type.js#L1-L69)。

OT 也需要版本日志、缺失操作恢复、快照、幂等、权限、校验、个人撤销、断线队列和删除语义。它的中心序列更贴近服务端裁决，但不是降低全部复杂度的捷径。

### 4.3 对本项目的比较（工程判断，非性能实测）

| 维度 | 现有 Yjs + Go 接受 | 成熟 OT / 中心操作服务 | 从零自研 OT |
| --- | --- | --- | --- |
| 同段 Tiptap 富文本 | 现有绑定及测试可复用 | 须选匹配 ProseMirror 的类型/步骤映射，或重做 Delta 绑定 | 需实现及证明复杂文本变换 |
| 不同节点/属性 | 按稳定 ID/细粒度 key 合并 | 可用细粒度路径操作实现 | 必须完整定义每种操作对 |
| 同字段互斥写 | 确定性合并；并非服务端最后收到胜出 | 可按服务端序列定义具体策略 | 同样必须先定产品策略 |
| 非法引用/权限拒绝 | 必须隔离候选与重建依赖 | 中间件方便，但拒绝队列也须恢复 | 自己维护全部边界 |
| 断线恢复 | state vector/更新幂等 + 本页不可变提交 | 版本追赶 + transform + 幂等提交 | 仍需版本协议与故障证明 |
| 删除/撤销/历史 | 已有稳定身份与 epoch，但规则复杂 | 同样有依赖、删除与逆操作问题 | 高风险算法面 |
| 无本地业务存储 | 可用，仅页面内存 | 可用，仅页面内存 | 可用，与算法类别无关 |
| 性能 | 当前全量重建/快照路径有明确优化空间 | 日志 transform 成本需实测；并非天然更快 | 成本和可靠性最不确定 |
| 迁移成本 | 最低；修边界与协议 | 中高；图/正文/历史/恢复/权限需重验 | 最高；还需算法研发、模糊测试及长期维护 |

## 5. 我们当前实现如何同步

以下是本次读取的当前源码事实，非旧协议记忆：

1. 本轮修复前审查基线的正式前端路径为 `frontend/src/components/canvas/yjs-live-session.ts`。`makeSession` 明确 `storage: "memory"`（L418–448），当时 transport 只有 yjs_submit/yjs_resolve（本轮补充原接受版本查询见 §9）；不能把仍存在的旧实验 controller/journal 代码当成正式入口的行为。
2. `canvas-collab/src/model.mjs` 的 nodes/connections 按稳定 ID 组织；placement 和 size 分组保存（L214–237、L341–343）；正文是 Y.XmlFragment；引用按 reference:id key 建模（L197–212），参数 values 按 key 建模。这比整节点 JSON 覆盖更适合兼容并发。
3. `canvas-collab/src/session.mjs` 有 committed 文档与 optimistic 文档（L27–117）；P 封包后固定 updateId/hash/bytes，后续 Q 独立积累（L453–483）；UNKNOWN 查询原身份再重发；ACK 对照原身份（L489–527）。远端只以 REMOTE origin 应用，不重新排入本地提交。
4. `CanvasYjsLiveSession.change` 在手势 stable 时提交；schedule 是 queueMicrotask，drain 发送同一队列；12 秒 RPC 超时后等待心跳查证，heartbeat 唤醒同一队列（L590–597、L661–733、L743–855）。这是事务驱动保存加心跳恢复，不是前端定时整图保存。
5. `canvas-collab/src/service-runtime.mjs` 每次从持久 state 构造隔离候选；`AdmissionGate` 检查更新复杂度、依赖完整性、共享根、稳定容器、投影业务合法性，再返回候选（admission L90–165）。Yjs 不自行决定接受。
6. `backend/internal/canvas/application/yjs.go` 从数据库查重，读取权限/epoch/reviewSeq，JS prepare 后按 durable fence 提交，冲突最多重新计算 4 次（L161–219）；`adapter/postgres/yjs.go` 在事务内写状态、增量、回执、Outbox（L285–303）。
7. `adapter/websocket/yjs.go` 的 syncHead 在版本变化时发送完整 `yjs_snapshot`（L44–66），提交/查证后 `syncHead(true)` 再 ACK（L248–260）。数据库有增量表，不代表当前外部 WS 已按增量高效传播。
8. 媒体临时地址和 naturalWidth/naturalHeight 是显示层数据；上轮 `retainCanvasMediaPreviews` 按 nodeId/type/完整 CONTENT 身份保留，引用变化不得沿用旧图。替换素材把文件引用与节点尺寸写入业务编辑；所有页面应采纳同一权威尺寸，不能各自根据浏览器解码时机反复写尺寸。

## 6. 当前需要认真处理的风险

### P1：高优先级验证，不应靠换算法掩盖

- **图片、引用、尺寸、位置必须形成可说明的业务冲突语义。** 当前 placement、size 与内容引用是不同共享字段。顺序替换测试通过不代表“甲替换素材、乙并发缩放/拖动/替换”总能获得符合业务意图的组合。应把这个案例纳入本轮三页面验收；如果实际出现比例与选中文件不匹配，需要明确原子素材绑定/尺寸意图或服务端归一化，而非仅在客户端偷偷矫正。
- **拒绝更新不能从原 Y.Doc 中简单删掉。** 后续字符、节点属性可能依赖被拒绝的结构。现有代码已有隔离候选和恢复机制，但需要持续测试“被拒绝 P + 后继 Q + 同时远端编辑”，以及旧 epoch ACK。Yjs 原生收敛不能证明恢复交互正确。
- **媒体显示身份与业务身份分开。** 同文件 ACK/重连不应重建 img 或清空 src；新文件版本则必须失效旧预览。还应测删除恢复/历史恢复后同业务 ID 的生命周期变化，及原素材读取授权失效是否仍被显示层缓存遮蔽。这是显示授权和生命周期问题，不能只判断节点 ID 一致。

这里列的是需要验证的风险，不宣称已复现全部缺陷。主线程浏览器、协议和数据库证据应决定哪些进入本轮修复。

### P2：已确认的实现成本，尚无负载数据

- 外部通道当前发送完整 state + document；JS 每次 prepare 重建 Y.Doc 并投影；客户端 receive/render 多次读取完整图、JSON 比较；服务端每个连接心跳会读取授权状态。源码足以确认成本随文档/连接/编辑频率增长，尚不能声称已构成某毫秒的性能瓶颈。
- 二进制采用 JSON base64，理论体积约比原二进制增加三分之一，再叠加 DTO；这与 Yjs 的增量压缩优势并不相同。优先测量真实数据量，再考虑只对初次/缺历史发送快照、正常广播 accepted 增量；保留 commitSeq 连续性、独立 ACK 和按权限补齐。
- 8 MiB state、768 KiB update、20,000 结构复杂度上限是当前保护值，不是容量承诺；高频编辑和服务端归一化可能增加 Yjs 历史结构。需测试长会话、删除重建、富文本和引用编号增长后的状态体积与恢复耗时。
- 对内存上限的 UX，`canAccept` 当前检查 pending 数与整图编码体积，需确保编辑器在修改前阻止下一动作并解释，不出现可见已输入但丢弃的结果；各层限制需统一接受测试。此处是源码审查风险，未执行超限故障复现。
- 光标目前单实例内存房间；不要把三个页面成功等同多实例 presence 成功。业务 Outbox 正确与 presence 跨实例是不同验收项。

## 7. 建议的比较与优化验收，不预先承诺结果

先固定同一组用户动作与网络故障，对现有方案测量：50/500/2,000 节点、2/10/50 编辑者、媒体引用与长段富文本；节点量超出当前产品上限时仅作研发探针，不要求产品接受。记录 p50/p95/p99 本地输入/远端显现/持久 ACK 延迟、每动作上下行字节、JS prepare 时间与 RSS、浏览器长任务、数据库事务/CAS 重试、断线追赶时间和恢复输入是否丢失。媒体二进制上传与同步协议字节分开计量。

故障点至少包含发送前断网、服务端 commit 后丢 ACK、只丢广播、重新授权、epoch 切换、非法素材拒绝、并发删除/恢复、慢客户端、服务重启。每个动作都对照独立回执与数据库最终图；视觉上同时测 img DOM 身份、src 变化、自然尺寸未解码次数和世界坐标/宽高。

只有当现有方案在已做增量/订阅优化后仍无法满足已确定目标，或产品冲突语义明确与 Yjs 不符，才启动成熟 OT 对照原型。验收必须相同，不允许用简化纯文本 OT demo 与包含授权/素材/历史的完整 Yjs 服务比较。

## 8. 许可与复用边界

- Yjs 13.6.33、Excalidraw、ShareDB、rich-text 的读取许可证为 MIT；JSON0 package 声明 ISC。可复用不代表免除许可证保留和依赖审查。[Yjs](https://github.com/yjs/yjs/blob/9ea3a4fb57b814292ce18dfd13c4f422e44e6c5c/LICENSE)、[Excalidraw](https://github.com/excalidraw/excalidraw/blob/b7de45e88111f5680ae4bb3c3f3541c7d3b279fe/LICENSE)、[ShareDB](https://github.com/share/sharedb/blob/ea090f5204f348aca0055fdca3236568473ca3e4/LICENSE)、[JSON0 package](https://github.com/ottypes/json0/blob/90a3ae26364c4fa3b19b6df34dad46707a704421/package.json#L34)、[rich-text](https://github.com/ottypes/rich-text/blob/b53cd97690804e544370423ce27c6d852abb2c7f/LICENSE)。
- AFFiNE 根许可把 backend/native 与其他目录区别处理；本报告读取的 blocksuite 路径属于根许可描述的 MIT 范围，不能据此称整个 AFFiNE 后端都 MIT。[AFFiNE LICENSE](https://github.com/toeverything/AFFiNE/blob/fbf2cebfa012a7c92b08af7bfa0340177308b2a6/LICENSE)。
- 当前 tldraw sync-core package 声明遵循 LICENSE.md，当前 SDK 默认开发许可，生产须适用授权；不满足我们已确认的“默认免费商用、自托管”前提，不能因代码在 GitHub 就直接迁入。[sync-core package](https://github.com/tldraw/tldraw/blob/12921f6d3996b4330688113371cd88890b673ab5/packages/sync-core/package.json#L1-L16)、[官方许可说明](https://tldraw.dev/community/license)。
- React Flow 公开协作页面标注 Pro 示例许可，本轮未取得付费源码。仅参考其技术方向，不复制受限代码。

## 9. 本轮实际执行的微型语义探针

命令运行在本地现有 `canvas-collab/node_modules/yjs` 13.6.33，两个独立 Doc 从同一基线产生更新，分别以 A→B 和 B→A 接收。测试人为设置 clientID 10/20 仅为可重复语义探针，**不是产品生成/复用 clientID 的建议**。

- 同 key x 并发写 10/20，同时 A 改 title：两种接收顺序都得到 `{ x: 20, title: 'edited' }`。证明本例同字段按确定规则选一值、不同字段保留，不能证明任意结构和业务不变量。
- `doc.transact` 内设置 x=1 后 throw：catch 后 x 仍为 1，update 事件仍触发一次。证明不能用异常回滚模拟业务拒绝。

未执行第三方端到端性能比较、OT 原型、生产流量实验或许可法律审核。与主线程当前浏览器验收结果分开记录。

## 9. 素材并发复核对设计的约束

真实 Chrome 的两账号三页面场景暴露出“替换素材与缩放并发”的业务字段组合问题：引用/位置来自替换操作，尺寸可能来自另一页面；此前解码适配又居中一次。观测到 x 从 1059.375 变为 1175.5859375，差值恰为 `(340−107.578125)/2`，而新宽度 `107.578125=191.25×360/640`。这是应用层尺寸校正再次移动位置，不能据此判定 Yjs 算法随机抖动。

本轮采用有界修正：主动上传/替换按已知素材尺寸等比适配并居中一次；后续解码校正仅在当前合并边界内保持比例、保持已合并位置，重复测量幂等，并在手势结束后执行。尺寸写入仍走原有接受通道；浏览器测量本身、URL 和自然尺寸不持久化。它**仍存在页面驱动的尺寸校正写入**，并非已经实现服务端原子“素材＋几何”裁决。后续如需完全消除该类附加写入，应把文件版本、可信尺寸与几何意图建模为明确业务约束，在同一权威接受事务中校验/归一化；不能只把独立字段放在一个 Yjs transaction 就声称原子冲突语义已解决。

另一个独立问题是原接受版本的证明：若后续快照先于旧 ACK 到达，应查询回执对应的 epoch＋revision 历史投影，不能拿最新内容判定原意图丢失。若原接受版本中确实未胜出，继续保留该字段供用户选择；历史不可得或身份不符也应保留，不能假装成功。正式入口需复用已有授权 WS 历史查询，不引入 HTTP 轮询或第二保存通道。

上述小修不改变算法选型。下一步投资顺序：原回执证明与媒体身份边界 → 单一权威媒体几何约束 → accepted 增量传播与负载测量 → 仅在证据表明有必要时做成熟 OT 对照原型。自研 OT 不作为当前故障修复手段。

正式模型的素材引用与节点配置位于单值 `native` 字段，意图跟踪、接受判断和恢复必须覆盖该原子字段；不能只跟踪实验模型的 `fileReferences`。恢复界面以“素材与节点配置”展示，审阅后任何当前字段变化要求重新核对，重新应用通过新提交身份、reviewSeq 和现有业务授权，不绕过素材访问校验。该字段当前是较粗的原子冲突边界，不能声称能自动合并其中所有配置子字段。
