# 即梦多选连线：独立静态审计

2026-10-29；只读公开前端 bundle，不执行原 bundle、不操作浏览器、不改产品代码。本报告 offset 为 UTF-8 解码字符串的 **0-based 字符偏移**，不是 byte offset。当前页面资源身份由 root 的 `/tmp/plus-loaded-resources.json` 确认；本报告是源码分支证据，运行效果和服务端持久化仍由 root 实测。

## 资源身份

| 文件 / 对应资源 | SHA-256 |
|---|---|
| `/tmp/jimeng-plus-1112.b5ee721939.js` | `44bfc11d03506f79c828ec9b8655f2fa01cc9a532966bce94c05a4dda4f50aef` |
| `/tmp/jimeng-plus-985.f9495a4cda.js` | `ca5ad08fd16ba7a23a96a1073c90e71df8f3cd829c9c71a769e597de3ba38d6f` |
| `/tmp/jimeng-calibration-30.js` = canvas-view.e4df37de47.js | `4b927f82c62b6386ec6ddb5f91a8c2d92c20590dc06e03bd9d1831dca3a23e0b` |
| `/tmp/jimeng-calibration-37.js` = canvas-core.8751e7ea24.js | `8f194bf4096477bf048a451313b9c959da8f50e1887b6e306ebf406b3c75c0a6` |
| `/tmp/jimeng-multi-canvas-data.js` = canvas-data.9b2cb17ffd.js | `9a34bf1e304ed3cc59a7cb82ef930e576b6c2e6bbf827ccf9fc682a370da7d2f` |

最后一个资源本次按已加载清单中的原 URL 下载，仅静态阅读：`https://lf3-lv-buz.vlabstatic.com/obj/image-lvweb-buz/ies/lvweb/octo_web/static/js/canvas-data.9b2cb17ffd.js`。

## 1. 多选入口的归属与位置

- Core `Ii` @1130768：把选中 IDs 映射成 nodes 数组索引并按索引排序，再筛出有 source handle 的节点。anchor = `eligible[Math.floor((eligible.length-1)/2)]`，即渲染顺序的较低中位项，不是最右节点、选中顺序首项或空间中位点。
- Core module29978 @1801635：eligible 只看 capability.sourceHandles.length>0，因此 text/image/video/audio/file/subject/tool/external 有资格成为 anchor；group/stack/timeline 没有。
- Core `P` @2642018：当 connectionSourceNodeIds（优先）或 selectedNodeIds 长度>1，给有 source handle 的来源标记 anchor/member，anchor 保存完整 sourceNodeIds。**没有把无输出节点静默排除出批量 sourceNodeIds**。
- 任一被选择节点的 `connectionHandleDisabledReason` 会成为整个 aggregate 的 disabledReason。当前明确赋值 `sessionPendingDurable` 在 core @1130425–1130620。并非所有类型不兼容都会预先禁用入口，后续目标验证另行判定。
- 1112 `eT` @37913：仅 anchor 渲染 aggregate handle；aggregate connectable 为 `handleConnectable && disabledReason===undefined`。resultsExpanded 时有独立隐藏/收缩分支。
- 1112 module11770 @48786：单节点常规 handle 交互要求 selectedNodeIds 恰好为当前节点；多选选中成员常规输出入口被抑制，aggregate 提供统一入口。
- 1112 `eW` @41516：框选进行中（React Flow userSelectionActive）不渲染 aggregate toolbar。它是 portal 中屏幕空间 zero-size host，geometry 来自全部 sourceNodeIds，而非仅 anchor。

**定位公式**（985 `p` @6507、`M` @8440、`E` @10531、88151.bT @14238）：

```
L=min(node.x); R=max(node.x+node.width)
T=min(node.y); B=max(node.y+node.height)
aggregateHostScreenX = R*zoom + viewport.x + 40*zoom + 16
aggregateHostScreenY = (T+B)/2*zoom + viewport.y
```

node坐标为 internals.positionAbsolute，尺寸优先 measured，次 width/height、initialWidth/initialHeight，再已缓存尺寸；text 缺省240，其它缺省0。主机在 renderer 坐标系内，若测 clientRect 需加 renderer left/top。默认 rightAlignment=center。zIndex=349000。

父任务真实页面额外观测：独立语义 button 为36×36，toolbar size0。button与实际pointer命中的before热区不是同一个元素。before沿用40×80 CSS token：位于handle left:50%、top:50%、translateY(-50%)；counter-scale=1时从aggregate中心向右40px、上下各40px。handle本体pointer-events:none，before仅connectable且visible时pointer-events:auto。

Aggregate 不是缩放后的普通节点 handle：1112 @19100–23630 强制 chrome counter-scale=1，默认glyph (30,60)，外观是实心亮底/反色加号，after size-5=20 CSS px。host位置跟随多选几何和 viewport。该特殊入口仍调用同一 ec pointermove，并没有跳过 glyph 平滑跟随。

## 2. 多选加号移动、点击和拖动

1112 `ec` @23631，`eu` @25732：与单节点相同二维跟随采样。将实际 60×120 CSS host rect 归一化：source targetX=clamp((clientX-left)/rect.width*60,30,90)，targetY=clamp((clientY-top)/rect.height*120,0,120)。初始(30,60)，alpha=1-exp(-dt/80ms)，首帧dt=1000/60，后续dt夹0..64，两轴误差均≤0.05时收敛；leave取消RAF并复位到50%/50%。只移动glyph，连接的source端点来自node handle几何。

继承前轮已核验全局阈值：connectionDragThreshold=1 CSS px，严格位移平方>1才进入drag。点击/键盘点击走 root capture 的连接入口分支，拖到空白走 connectionDrop 创建菜单；多来源在此展开为同一组 sourceNodeIds。

View @704600–706500 gesture快照：
- start(anchor,handleType) 冻结当前 selectionIds 的数组副本。
- 仅 handleType=source、selection长度>1且含anchor时形成 batch；从 target handle 开始不形成多来源 batch。
- 每个target的batch getState缓存；canvas topology变化清缓存，但不重选 source IDs。
- 点击/拖动空白创建目标的菜单，单anchor请求通过冻结来源展开为多来源；不会在菜单点击时临时读取另一个新选区。
- View @707900–711700：execute成功后才进入成功presentation；批量失败返回rejected并反馈。不存在静默降级成单来源执行。

## 3. 分支线与汇聚

View `eJ` @477200–477930：从 aggregate anchor 拖出，且 sourceNodeIds>1、`toNode===null` 时，为每个来源各生成一条曲线，汇聚于共同的 pointer/菜单目标；不存在先连到aggregate加号再接一个主干的图拓扑。

- anchor使用 React Flow fromX/fromY；其余来源通过 `e1` @479180 左右读取 node internals.handleBounds 的 default source 中点。
- active preview两端使用既有4/zoom world-unit的屏幕4px外扩（目标只有snapped时外扩）。
- **注意真实 snap 差异**：`toNode!==null` 时该 eJ 分支回到单条 anchor 曲线；并不改变落地批量命令。不要据渲染线条数量推断提交边数。
- pending connectionDrop menu renderer @479900–481000 又逐来源生成路径，均指向共同placement。
- preview 注册 @478309：路径条数>1使用fanout预算，否则detailed；沿用前轮已核验16/64分段预算。已落地边逐条使用detailed，不是看到多条线就全降为fanout。
- 合法候选体内 preview endpoint会向目标对应边中点吸引38%：`p'=p+(edgeMidpoint-p)*0.38`（view @490701）。鼠标命中检测包含body边界，重叠时更大zIndex优先，同zIndex后遍历索引优先（tR @506189）。

## 4. 候选合法性、混合选择、重复和环

View @489000–489700：候选排除整个 sourceNodeIds 集合（去重集合用于反馈）；其它节点提前通过完整 isValidConnection，释放时还重新验证。候选不等于只看可见handle，node body也接受落点。

Core `evaluateBatchReferenceConnection` @520495：
1. 空source列表→endpoint_missing。
2. source IDs自带重复→capability_invalid，**不静默去重提交**。
3. 任一external来源→provenance_requires_created_target；existing target的reference连接不接受它。
4. 按来源顺序逐个检查，累计 incoming总数与每sourceType计数；任一失败立即整批失败。
5. 完整提交阶段再验证资源/模型reference constraints ready及attachment约束；preview available不能保证最终可提交。

Canvas-data module71146 @285101，R @287330固定结构检查顺序：endpoint存在→自连→creates_cycle→来源输出reference→目标输入reference/能力存在→capability schema/count有效→允许source类型→duplicate_relation→总体容量→按类型容量。

**重复与环策略：整个批次拒绝，绝不是跳过坏项然后连接其余项。** 核心batch调用R，既有edge关系 `hasActiveReference` 导致duplicate_relation。cycles用 BFS 从target沿出边走，若能到source则拒绝（data @287136）；source==target也直接拒绝。Graph topology实现 core @1159845 使用outgoingTargetsByNodeId。

基础静态能力（可能被模型配置/媒体状态进一步缩小，不能当作当前账号菜单实测）：

| target generation_input | baseline allowed source types | 总数上限 |
|---|---|---:|
| text | text,image,video,audio |128|
| image | text,image,subject,tool |999|
| video | text,image,video,audio,subject,tool |999|
| audio | image,video,audio,subject |7|
| timeline | image,video,audio | 未在静态能力对象限定 |
| subject | image,video,audio |5|

file虽有source handle，却不在上述默认reference允许列表。group/stack无source、无target；timeline有target无source；external只有post_edit_provenance输出；tool只有reference输出无输入。混选unsupported类型不会在本层自动删掉，batch会拒绝。例如“文本+时间线”虽然可由文本承担aggregate anchor，时间线本身仍在sourceIDs中而使existing-target批量reference不合法。

Core `lf` @568109：timeline分支执行addTimelineSources并要求无skipped项且sources.length等于请求来源数；subject分支整体验证并生成assignments；其它才是connectEdges。所有类型不是统一裸edge.add。

## 5. 保存可见协议与证明边界

Core @137409：document action先拒绝只读/空列表/任何sessionOnlyNode；整组调用draft.connectEdges，记录write receipt。
Core `connectEdges` @301204：生成各自唯一edge ID、连续createdAt，一个 `sdk.command.edge.add({edge:[...]})` 批量命令，edge闭合形状为 `{id,createdAt,source,target,type:'reference',data:{}}`。
Core feature action @1406400–1407450：generation target可能先走 `connectGenerationReferencesFromCanvas`，timeline/subject分别有领域命令，不能只照搬reference裸边储存就声称等价。

**write receipt不等于服务端确认。** Core `completeCommand` @318696 接受 runtime submission outcome为submitted或held；`applyWithReceipt` @320497把二者都映射为applied。`recordDraftWriteReceipt` @226230仅拒绝noop，记录updateId后返回true。因此UI成功动画/edges立刻出现/getState ok均不能证明已远端保存。当前函数证据不足以证明网络ACK或刷新重建；需要 root实测 actual save状态、刷新/重开后全部边与关联reference仍在。

## 6. 建议 root 的最小真实测量矩阵

1. 两个文本/三个文本多选：记录union bounds、zoom、portal rect与以上公式；先水平/后错位排列，验证anchor identity与屏幕+位置分别独立。
2. aggregate glyph hover、leave、click与空白drag：确认20px固定尺寸，50%默认点，菜单所有来源预览，取消无新增边。
3. 两个来源拖到目标body、目标handle：比较toNode=null与nonnull时线条数量变化；提交后核对每个来源边。
4. 一个来源已有同目标边：整批拒绝且另一个来源不应被追加；重复ID是内部输入约束，无需通过UI伪造。
5. 三节点回路候选：已有A→B→C，再C→A应失败；混选一合法一成环来源应整批失败。
6. 混选text+timeline、text+group、external+text：先观察aggregate是否出现，再观察菜单/目标是否被禁用或拒绝；这里最容易把“无handle”错误实现为“排除选中成员”。
7. 有效批量创建目标+连接后，等待明确保存状态，刷新/重开核对所有source/target ID和reference关系；有网络权限时区分submitted/held/accepted证据。失败或拒绝不能留下半批边。

剩余边界：本审计未追完整transport/保存indicator映射，未断言服务端事务原子性；“整批拒绝”指前端预检查与单命令设计，需runtime证明失败回滚和远端持久化。无浏览器操作、无业务节点/边创建。

## 7. 本轮限定 UI 实现与测试（2026-10-29）

已落实共享工作区，未提交：
- `frontend/src/components/canvas/controls/canvas-selection-toolbar.tsx`：直接从未padding的nodeBounds计算aggregate screen host；新增onConnectStart(MouseEvent)，优先于旧onConnectAfter兼容回调；非左键不dispatch，鼠标click不重复dispatch，键盘detail=0保留。
- `frontend/src/components/canvas/nodes/canvas-connection-handle.tsx`：variant=aggregate，rest(30,60)、counter-scale=1；视觉20px glyph与独立36px语义button分开；复用既有pointer跟随和清理。
- `frontend/src/components/canvas/canvas.css`：aggregate白底/黑18反色外观、独立键盘button定位；基础60×120盒及40×80右半侧热区复用既有样式。
- 新增toolbar契约测试；原handle测试增加aggregate跟随/reset/缩放固定尺寸情景。

Red：修正测试Provider环境后，8项中6项因缺少aggregate合同失败，2项权限隐藏通过。Green：toolbar8 + handle8 = 16通过。限定eslint通过，git diff --check通过。命令在frontend：`pnpm exec vitest run tests/unit/components/canvas/selection-connection-handle.test.tsx tests/unit/components/canvas/nodes/canvas-connection-handle.test.tsx`；eslint只检查本次4个TSX文件。

没有修改editor、多源曲线路径或领域admission，由root/另一worker接入。没有执行真实浏览器：来源实测、目标页面回归和远端保存由root进行。本轮复用现有handle的减少动画偏好支持，这仍与即梦opt-in=false存在已记录差异，没有借本UI任务扩大修改。aggregate hover secondary颜色后续已由root实时computedStyle确认：rgba(255,255,255,0.88)，glyph text仍为rgb(18,18,18)。已把局部CSS从暂定70%修正为88%；该值是源页面实测，非推断。


## 8. targetAttraction 组件回归

新增 `frontend/tests/unit/components/canvas/connection-target-attraction.test.tsx`：source/target 38%候选吸引、1/default完整吸附、候选消失保留原pointer、真实provider→mesh上传的fanout 16段与detailed64段切换及清理。新7项+原meteor4+layer1合计12项通过；新文件eslint通过。未修改editor。审阅另确认：全无source handle的选区没有aggregate anchor，因此不应显示入口；混合选择不应过滤无输出成员。

审计发现并已修复：candidate body（toNode=null）38%吸引时，源代码候选端使用body边中点，不加入4/zoom外移；只有完整snap的候选端才外移。root已修正ActiveConnectionPath，我把数值期望同步为source终点(475.2,435.2)、target起点(373.4,204.8)，12项组件测试及eslint再次通过。body吸引仍属toNode=null，保留多条fanout预览；只有完整handle snap才临时显示单anchor线，此editor分支由root调整。

## 9. 当前加载 CSS 的多选框与 compact 菜单（2026-10-29）

来源清单由 root 当前实页采集到 `/tmp/multi-source-css-urls.json`；仅下载已观察到的公开 CSS 并静态读取，未运行第三方脚本。以下偏移为解码字符串的 0-based 字符偏移。

| 本地文件 | SHA-256 |
|---|---|
| `/tmp/multi-source-canvas-view.523722e333.css` | `5d30e6fccf0464ad46bb03cceff5701de12f7e170004bc33b63e7ffb8cc599d8` |
| `/tmp/multi-source-canvas-core.ef8204044d.css` | `13fbe98cb93466e5eb7f4705eb88d26e5ccacb01e37757a6211ee97db745eedc` |
| `/tmp/multi-source-main.8ac605db3f.css` | `b88db1a04ec7013b6ab9831b8993905bb30e29b45755d5fc779e83ef076ee6c8` |

### 9.1 多选框精确合同

- canvas-view CSS @16248：多选 rect `box-sizing:content-box;margin:-40px;padding:40px`。view JS @26000 附近 ReactFlow 先取节点 union 的原始世界尺寸，再由 viewport transform 缩放，JS 不增加 padding。
- main CSS @49515：多选 rect `border-radius:40px;border-style:dashed;border-width:calc(1px*var(--octo-canvas-node-screen-fixed-counter-scale,1));box-shadow:none`。这是更特定规则，覆盖 view CSS @16124 通用 radius12。
- core JS module76235 @6915 counter-scale=`1/zoom`（非有限或非正 zoom 才回落1）。view JS @527221 向 selection rect 更新该变量。
- main CSS @46022 将 selection background 设为 block-secondary-default；该暗色 token @6543 指向 alpha-05 @2489，即 `#ffffff0a`（10/255）；边框为 text-disabled @5337 → alpha-50 @2259，即 `#ffffff33`（20%）。源码没有指定 dash/gap 数值，使用浏览器 CSS dashed，不能替换成猜测的 SVG 7/5。
- root 实测 zoom 0.870494，union 左上 (383.2853,193.4699)，框左上 (348.4655,158.6501)，差34.8198≈40×zoom；union 662×320。与世界 padding40 合同一致。源码 border1/zoom 在浏览器可能被量化，computed1px 不代表其源 CSS 是固定世界1px。

限定实现改 `canvas-selection-toolbar.tsx`：screen host 仍可让既有 node-drag 写临时 transform；其内 div 使用 union 世界宽高、content-box、世界 padding/radius40、border1/zoom 和 scale(zoom)。颜色使用现有 `--canvas-reference-foreground` 加 color-mix 精确比例，遵循项目 theme policy。旧 SVG border/dash/圆角及14px padding不再用于选框；其他工具条位置保留既有逻辑以限制范围。

回归先红后绿：selection 测试增加 .25/.71/1/1.36 四个 zoom 场景，验证 content-box/union/世界padding/圆角/逆缩放border/主题色和 host transform 不被占用。Red4失败8通过；实现后 selection12 + handle8 =20通过。限定 eslint（修复一个 unused height 后）及 diff check 通过。无浏览器操作；root负责真实拖动与视觉复核。

### 9.2 Pending compact medium 菜单 200×316

main JS module72407 @2339257：compact `headingGap:4,headingHeight:24,borderWidth:0,widths.medium:200` 是基础；同模块 compact/default density 覆盖 `headingGap:0,headingHeight:32,itemHeight:36,itemPadding:"9px 12px",padding:4,paddingBottom:4`，保留 item gap4。因此七项菜单总高 `4+32+7×36+6×4+4=316`。

- root content @2371991：radius12、无边框、background bg-dropdown-menu、shadow-general、backdrop-blur-small；max width/height 为 viewport−16，位置 clamp 到 viewport8px inset。
- item @2372994：font13、line-height22、weight400、radius8；icon/text gap8，item padding9px12px。悬浮使用 block-secondary-hover。
- heading @2348137 引用 module24156 @2427724：height32、padding `8px 12px 4px`、font12/regular、text-placeholder。main CSS @327904 使用 cn12 token；token @23949/23981 明确12px/20px。暗色 placeholder @5262→alpha-60 @2213=`#ffffff59`（89/255，约34.9%）。
- 暗色 menu 背景 @5541→gray-dark-100 @1834=`#262626`；shadow-general @27773=`0 8px 56px 0 rgba(0,0,0,.08)`；blur-small @28440=20px。

已发 root 与 Figma worker 精确合同。菜单实现由 root 负责，本 worker 没有修改 editor/menu。上述为当前已加载资源静态规则，不能单独证明所有动态 overflow/分辨率场景。
