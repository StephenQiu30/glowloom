# 无限画布前端实现静态核查

日期：2026-10-28。核查时 HEAD：`b0e658198d0e8de85cf3d140e6ebebc77dbb9ff5`；共享工作区后续修改可能使行号变化。范围为 JC-001～045、JC-052～060，共 54 项。依据研究10、设计27、PRD14、计划23及仓库 AGENTS；只读源码，未启动服务、未调用生成、未进行浏览器操作或付费验证。**本文全部结论是静态实现证据，不能标为运行验收通过。**

## 正式入口与最高优先级差异

正式入口为 `frontend/src/app/canvas/entry.tsx:9` → `components/canvas/service-workspace.tsx:38` → `yjs-workspace.tsx:113` → `editor/canvas-editor.tsx`。顶栏由 `yjs-workspace.tsx:137` 注入 `controls/canvas-top-bar.tsx`；不是 legacy `chrome.tsx`。`editor/canvas-document-page.tsx` 有另一套生成 hook，但全 frontend 检索只找到其定义，不能当成正式服务画布已接入的证据。

正式 Yjs 路径的 `documentIntegration`（`yjs-workspace.tsx:113–182`）未注入 `generation` 或 `workflow`。节点面板 `nodes/canvas-document-node-panel.tsx:92` 用 `generation?.ready !== false` 作为条件，undefined 通过；选中模型后可能显示可点生成。点击进入 `editor/canvas-editor.tsx:3486–3494`，却因 `!integration?.generation` 直接返回。这是静态可确认的接线缺口，应在 T-CJC-10/11 中先补齐执行接入和不可执行状态说明，再验收真实生成。不能以存在模型菜单、参数弹窗或旧文档页 hook 认定闭环已交付。

## 源码责任索引

以下别名统一相对 `frontend/src/components/canvas/`，表中 `别名:行` 即责任文件及核查行号；越出该目录的条目列出完整仓库相对路径。

| 别名 | 责任文件 |
| --- | --- |
| Editor | `editor/canvas-editor.tsx` |
| Yjs | `yjs-workspace.tsx` |
| Top | `controls/canvas-top-bar.tsx` |
| Dock | `controls/canvas-toolbar.tsx` |
| Zoom | `controls/canvas-zoom-controls.tsx` |
| Create | `controls/canvas-create-menus.tsx` |
| Context | `controls/canvas-context-menu.tsx` |
| Multi | `controls/canvas-selection-toolbar.tsx` |
| Node | `nodes/canvas-node.tsx` |
| Hover | `nodes/canvas-node-hover-toolbar.tsx` |
| Composer | `nodes/canvas-document-node-panel.tsx` |
| Refs | `nodes/canvas-node-reference-bar.tsx` |
| Ports | `constants/canvas-input-ports.ts` |
| Connect | `lib/canvas-document-connections.ts` |
| Factory | `lib/canvas-node-factory.ts` |
| Group | `lib/canvas-node-geometry.ts` |
| Rich | `runtime/components/editor/rich-text/rich-text-editor.tsx` |
| Models | `parameters/canvas-model-select.tsx` |
| Params | `parameters/canvas-generation-parameters.tsx` |
| Video | `runtime/components/generation/lib/video-parameters.ts` |
| Picker | `runtime/components/assets/picker/asset-picker-modal.tsx` |
| Asset | `frontend/src/components/media/project-asset-dialog.tsx` |
| Search | `editor/canvas-side-panel.tsx` |
| History | `history/canvas-history-dialog.tsx` |

“可复用”表示有静态实现基础；“部分”表示已有相关交互但缺契约；“缺口”表示正式组件未提供该要求；“待核”表示读源码无法证明端到端语义。各项均需后续浏览器、权限和保存回读验收。工作包编号沿用计划23的 T-CJC-*，不在证据文件单独维护完成状态。

## JC-001～018：外壳、创建和批量操作

| JC | 静态结论与责任位置 | 明确差异／未知 | 建议工作包 |
| --- | --- | --- | --- |
| JC-001 | 部分：Top:97 首页、137 标题、156 项目路径、185 保存占位；Yjs:138 注入标题及真实 phase | Yjs 顶栏未注入项目路径、节点总数与节点统计入口；保存成功不能靠文案证明 | T06、T05/T16 |
| JC-002 | 缺口：Top:25 props 无节点列表/计数；Search:418 有搜索结果数量 | 搜索结果数不是顶栏七类型统计；缺主体、时间线、外部分类统计及项目概览 | T06 |
| JC-003 | 部分：Top:207 参与者、214 搜索、226 生成历史、分享与账号入口 | 分享目前仅复制 URL；未见积分/会员账户展示。积分要真实服务返回，不能复制即梦数值 | T06、T12、T16 |
| JC-004 | 部分：Dock:225 仅文本/图片/视频/音频；251 主体和3D导演为 disabled；280 资产、296 上传 | 缺时间线、导演工具箱；主体/导演不是可用入口，十项工具未齐全 | T06、T08、T13、T15 |
| JC-005 | 部分：Editor:5449 InfiniteCanvas；Dock 左侧工具、底部视图/AI布局；Editor:5883 背景配置 | 壳层位置有基础，AI 为不可用占位；暗色点阵及窄屏实际视觉、遮挡需浏览器验收 | T06、T04、T15 |
| JC-006 | 可复用：Zoom:123 提供放大、缩小、适应画布、适应选中及50/100/200%；Editor:5847 接入视图操作 | 无选中禁用与 fit 计算有接线；极端边界、视图动画及滚轮需运行验证 | T06 |
| JC-007 | 缺口：Zoom:99 显示百分比按钮，123 为预设操作 | 没有百分比输入框、Enter提交、非法值反馈及失焦规则 | T06 |
| JC-008 | 可复用：Zoom:58 小地图切换；Editor:5870、5900 连接小地图状态/组件 | 运行定位、缩放范围与窄屏避让未知 | T06、T04 |
| JC-009 | 可复用：Zoom:55 选择/移动，58～85 小地图及连线开关；Editor:5872 接入显示连线 | 开关 aria-pressed 存在；连线隐藏是否保留选中态、键盘等需实测 | T06 |
| JC-010 | 可复用：Editor:2155 框选支持 Shift 追加，2162 清空；2175 节点选择支持 Shift/Meta/Ctrl | 手势与富文本选区冲突、触屏、重复点击行为未知 | T07、T04 |
| JC-011 | 部分：Editor:1593 创建、选中、视图中心放置；Factory:19 使用类型规格 | Factory:30 直接 `spec.title`，没有“文本1/2”等顺序编号；新增重叠避让未见此路径处理 | T07 |
| JC-012 | 缺口：Editor:5463 双击空白打开 Create:73；Editor:5468/3478 仅阻止原生右键菜单 | 空白右键没有创建/粘贴/撤销/重做动作集合。快捷键不等于右键菜单 | T07 |
| JC-013 | 部分：Context:64～127 打组、连线调整、视频取帧、复制和删除；Editor:1896/1909 独立复制粘贴 | 右键“复制”实际 onDuplicate；缺独立复制/粘贴、另存其他画布、撤销/重做。需区分复制剪贴板与直接复制节点 | T07、T12 |
| JC-014 | 部分：Multi:18、80～119 为选择包围框、打组、运行流程、解组 | 缺数量提示、堆叠、排列、下载、标签及后置创建；侧栏批量下载不能替代多选浮条 | T09 |
| JC-015 | 部分：Group:134 仅至少两节点且未同组的通用打组判定；Editor:1480 使用 | 未见普通组/分镜组区分、纯图片限制、49张上限及相应禁用说明 | T09 |
| JC-016 | 缺口：Multi:80～119、Group:134 只见打组操作 | 未见多选网格排列与智能排列入口；split-dialog 的拆图网格不是任意选中节点排列 | T09 |
| JC-017 | 缺口：Node:545 标题；Multi:80 浮条；Hover:296 动作集合 | 未见五色标签、清空标签和批量应用。runtime contracts 的通用 tags 字段不等于画布功能 | T07、T09、T02 |
| JC-018 | 待核：Editor:1238 创建节点并 connectDocumentNodes；1153 计算完整 result；1170/1171 分别 setNodes/setConnections；Yjs:131 undo | 代码能建立节点与连线；是否单一 Yjs undo 项、失败无残留、撤销后的服务回读需要事务层/运行验证，不能仅据 React 同批更新认定原子性 | T07、T05/T16 |

## JC-019～030：文本与引用

| JC | 静态结论与责任位置 | 明确差异／未知 | 建议工作包 |
| --- | --- | --- | --- |
| JC-019 | 部分：Node:372 resize算法；1741 实际仅渲染右下 ResizeHandle，1757 固定 bottom-right | 类型虽有四角（85），实际仅一个手柄；缺八方向、四边中点 Enter 复位。方向键微调不等于复位 | T07、T04 |
| JC-020 | 部分：Node:552 标题输入；Hover:329 文本生图、351/362 字号；Rich:1065 结构化文档导出 | 缺文本节点外层的标签、背景色、全屏/文本下载操作集合；结构化文档导出不是相同的节点快捷下载 | T07 |
| JC-021 | 可复用：Node:650 双击进入编辑、685 Enter/空格进入，1113 RichTextInput(documentMode/floatingToolbar) | 实际浮条定位、焦点恢复、IME和无多余编辑外框需浏览器验收 | T07、T04 |
| JC-022 | 部分：Rich:233 H1–3/粗体/斜体/下划线/删除线/列表/引用；997 工具条；1093/1104 编辑撤销重做 | 未见 textAlign 扩展或对齐控件；协同文字undo与画布undo的隔离需验证，不能简单叠加两个独立历史 | T07、T05/T16 |
| JC-023 | 缺口：Node:615/706 内容背景跟随 theme.node.fill；Hover 文本动作无色板 | 未见独立文本背景 none+五色以及与标签分离的持久化契约 | T07、T02 |
| JC-024 | 部分：Composer:106 参考、223 提示编辑、231 展开、92 canGenerate | 生成可用判断没有空内容校验；更严重的是 Yjs 未注入 generation 却可通过 ready 判断，点击静默返回。文本编辑值与生成指令也需明确产品语义 | T07、T10、T11 |
| JC-025 | 部分：Refs:73 添加参考只调用画布选择；Picker:89 能打开项目资产并上传 | 添加参考处没有“本地/资产库/画布”三级来源选择；另一个上传入口不能算同一参考流程完整接入 | T08、T12 |
| JC-026 | 部分：Node:1769 左右连接按钮；Create:37 区分handle方向；Editor:1238 创建连线节点 | 左右创建链路存在；按钮依赖外层 onMouseDown，Enter/Space 激活是否有效需补键盘测试 | T08、T04 |
| JC-027 | 部分：Create:48 只候选文本/图片/视频，并按支持关系过滤 | 文本后置可见候选吻合三类；其他类别被完全省略而非显示禁用原因。Ports:2/10 实际图协议仍允许文本→音频，菜单和图协议须做有意分层决策 | T08、T02 |
| JC-028 | 缺口：Dock:251 主体disabled；Ports:2、70 无 subject 源或目标 | 主体后置图片/视频/音频无正式节点协议和菜单接线；不能把创作页主体引用当画布主体节点 | T08、T02 |
| JC-029 | 缺口：同JC-028；Editor:1238 仅普通节点创建连线 | 无空主体节点→图片、来源标签的正式可执行链路；需明确空主体允许建结构但不可执行生成 | T08、T02 |
| JC-030 | 部分：Connect:85～104 区分 FILE_REFERENCE/NODE_VALUE/RUN_OUTPUT；Refs:58～68 构建参考预览；lib/canvas-resource-references.ts:289 | 图连线和固定资源已有区分基础；reference.active 是调用方标志，并非资源 ready 状态证明。空资源、失效、被删、跨权限和执行冻结需运行/服务契约验证 | T08、T11、T02 |

## JC-031～045：生成配置与导演预设

所有生成配置行均受上述正式 Yjs 缺少 generation 注入影响。以下“可复用”只说明控件/模型目录代码存在，不说明可发起任务或产生资源。

| JC | 静态结论与责任位置 | 明确差异／未知 | 建议工作包 |
| --- | --- | --- | --- |
| JC-031 | 部分：Composer:106/223/173/187 为参考、提示、模型参数、提交 | 缺报价区域；空输入禁用不完整；正式执行没有注入。应以服务能力及校验结果决定按钮和文案 | T10、T11 |
| JC-032 | 部分：Models:71～75 未绑定时选目录首项；Params:95 默认size读保存/字段/2K | 没有把Seedream名称或3积分硬编码是正确方向；需能力服务声明默认值及报价，当前首项不等于明确默认策略。账号实际目录未知 | T10、T02 |
| JC-033 | 部分：Params:94～132 图像尺寸/比例依模型和能力构建，297 渲染字段 | 非法已保存值以disabled项保留（314），切模型后的校验/修正及提交拦截需端到端确认；数量/八比例不能硬编码替代真实能力 | T10 |
| JC-034 | 部分：Models:51 动态模型目录，139 保留缺失模型项、147 modelOptionLabel | 未见模式逐项禁用原因、能力详细描述/套餐标签布局；不得复制即梦9款目录及未验证套餐含义 | T10 |
| JC-035 | 部分：Composer:231 expandedView复用草稿/参考/controls、237焦点恢复 | Refs 仅参考列表，未见展开态全部/图片/文本过滤及宽度调节控件；窄屏布局未验收 | T10、T04 |
| JC-036 | 部分：Video:39 ratio优先16:9、分辨率最低值，Params使用模型字段 | 缺正式全能参考模式与报价组合；默认4秒/720P/Seedance不能当永久配置。实际目录、价格和扣费未知 | T10、T11 |
| JC-037 | 部分：Video:20/22 以nativeCapabilities分辨率/比例覆盖，25 时长范围；Params:297 通用渲染 | 能力驱动方向可复用；参数数量/切换回读、无权限或字段冲突未验收，不应复用图片全参数集 | T10 |
| JC-038 | 部分：Ports:50/59 有首/尾帧用途，Video:25 支持能力时长 | Params:303 所有参数为 ToggleGroup；没有时长输入+滑块，也没有首尾帧/全能参考明确模式切换及槽位保留交互 | T10、T08 |
| JC-039 | 部分：Params:77 有 seedanceReferenceHint；Models:147 普通SelectItem | 模型选项未依据当前参考模式逐项禁用并展示原因；通用提示与提交错误不能替代可选项约束 | T10 |
| JC-040 | 缺口：Composer:80 supported仅text/image/video；parameters/canvas-audio-settings-popover.tsx:30 是另一设置组件 | 正式节点生成面板未接audio，未见音乐/音频及全能配音/文本朗读分层；音频上传预览不等于音频生成 | T10、T11 |
| JC-041 | 缺口：上述audio设置组件包含voice/format/speed，但未正式接入；Composer:80/187 | 未见正式画布音色库及价格/原价/折扣详情；真实音色选择、引用声音、结算需服务验证，不复制12/24/5折 | T10、T11、T02 |
| JC-042 | 缺口：Dock:251～269 仅不可用3D导演；Composer:122 controls只有模式/模型/参数 | 未见图片相机/配色/风格与视频配色/风格/运镜的上下文入口。图片角度工具不是该工具箱 | T13 |
| JC-043 | 缺口：Composer:122/173、Dock:251 未接导演预设组件 | 未见相机/镜头/焦距/光圈分组预览、自动、确认/取消草稿；应用到真实生成输入的契约未知 | T13、T02 |
| JC-044 | 缺口：同JC-042责任入口 | 未见配色/风格独立预览选择及空选确认禁用；canvas主题背景不等于生成配色预设 | T13 |
| JC-045 | 缺口：同JC-042责任入口 | 未见运镜预设选择、20条可见项或空选禁用；预设清除、兼容模型和映射须先定义，不能无来源拷贝专有素材 | T13 |

## JC-052～060：素材、检索、历史与系统状态

| JC | 静态结论与责任位置 | 明确差异／未知 | 建议工作包 |
| --- | --- | --- | --- |
| JC-052 | 部分：Picker:89 ProjectAssetDialog 默认img/video/audio；Asset:146 一级资产与可选主体；Search:190 独立画布元素面板 | canvas的Picker未传onSubject，因此主体tab不出现；无统一资产/主体/画布三级入口，默认无文档类型。需统一来源语义而非新增重复资产库 | T12、T08 |
| JC-053 | 部分：Asset:190 搜索、226列表、287选中数、305确认禁用、639空态 | Asset:207 时间/来源按钮只打开“暂不可用”说明；不能标作筛选已实现。跨页选择、请求隔离与失败状态需运行验收 | T12 |
| JC-054 | 缺口：Asset:207 仅不可用筛选；617 显示素材宽高 | 未见图片收藏、超清、分辨率、比例筛选；宽高信息不是筛选功能。后端筛选支持未知 | T12、T02 |
| JC-055 | 部分：Search:340 按名称/正文/prompt与类型过滤，418过滤结果数，460类型，577定位回调 | 无各类型分项计数；主体/时间线/外部不在分类；是否搜索正文是产品决策，不能默认当即梦实测事实。定位实际行为需测试 | T12 |
| JC-056 | 部分：History:29 all/image/video/audio/text，179真实项目任务查询，130TaskDetail，263空/错误态 | 未见积分明细入口与正式历史回填；查询代码存在不代表真实非空任务、取消重试闭环已验收 | T12、T11 |
| JC-057 | 部分：Top 分享按钮复制window.location.href且说明“保留现有访问权限”；Yjs:145参与者 | 缺画布内“仅自己/获得链接任何人/创建团队”分享面板；现有权限不能因复制URL自动扩大。服务层有无共享能力需后端独立核查 | T16、T06 |
| JC-058 | 缺口：Dock:307 AI对话disabled，说明暂不可用 | 无会话、参考、/技能、@主体、发送及可调宽度的正式AI侧栏；Search侧栏宽度调整不是AI侧栏 | T15 |
| JC-059 | 部分：Yjs:125 frozen、127 client.change、131 history、155 phase、163 recovery；Editor:5814接入 | 保存/恢复/撤销有正式Yjs接线；刷新、断线、多人并发、确认序列/服务落库、失败输入保护须独立验收，当前未做 | T05、T16 |
| JC-060 | 部分：Node:554标题名、578键盘提示、680文本编辑AX、1752缩放名；Zoom切换aria；Composer:283展开名 | 缺与即梦相同的节点类型+资源ready/processing/failed+选中状态的汇总语义；端口按钮仅外层mousedown需键盘核查。读屏、焦点顺序与缩放无障碍未实测 | T04、T07、T16 |

## 建议执行顺序与验收分层

1. **T02/T05/T10/T11 正式执行入口先闭合**：明确 Yjs 保存确认版本与执行快照关系，注入 generation/workflow 或明确禁用原因；空内容/无模型/失效参考/无权限分别反馈。验证按钮不能静默无效、不能把旧文档页作为后备写入路径。真实任务及成本由服务确认。
2. **T06/T07 外壳与基础编辑**：七类统计、十类工具契约、缩放输入、节点编号、空白/节点菜单、文本背景和标签、八方向缩放。新增字段先贯通图契约/适配/Yjs/保存，再做外观；布局沿既定语义token。
3. **T08/T09 引用与批量操作**：统一来源选择、主体稳定引用、可见禁用原因；普通组/分镜组、49张上限、排列和批量标签。原子撤销和回读作为独立验收。
4. **T10/T13 生成器与导演预设**：复用已接的模型目录及nativeCapabilities，补模式、时长、音频分层、报价和导演草稿；不要硬编码即梦商品名、套餐和价格。
5. **T12/T15/T16 素材与协作闭环**：真实素材筛选、搜索计数、历史回填、分享权限、AI侧栏；服务不支持的操作应显示真实不可用原因，不能用可点击空壳计作完成。

验收至少分成四列：静态代码证据、组件/协议自动检查、浏览器交互与可访问性、真实服务回读/生成结果。当前只完成第一列核查。需后续运行确认的共性风险包括：跨空间缓存隔离、模型能力变更、无权/断网输入保护、撤销事务边界、资源固定版本、移动端遮挡与IME。本文不改变 BACKLOG 中任务状态。

## 检索边界

核查了正式入口、节点与工具栏、引用和端口定义、文档生成面板、能力参数、资产弹窗、搜索、历史、Yjs注入。通过 `rg` 检索 canvas 中的标签、背景、textAlign、storyboard/分镜、音乐、全能配音、相机/配色/风格/运镜及入口引用；发现的拆图网格、设计预览 CSS、snapshot-viewer 类型标签、独立音频设置或其他创作页组件均未当成正式画布功能。没有使用 legacy `chrome.tsx` 证明运行功能；没有以静态缺入口推断服务端一定缺能力。
