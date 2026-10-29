# 即梦节点「+」连接口源码审计

日期：2026-10-29。只读静态分析；没有操作浏览器、没有修改产品代码。父任务已补充当前页面 loaded-resources 清单 `/tmp/plus-loaded-resources.json`，其中含下列同版本资源。资源属于当前页面的执行分支、CSS实际计算结果与事件效果，仍由父任务运行时核对。

## 资源身份

|资源|本地|SHA-256|
|---|---|---|
|`static/js/async/1112.b5ee721939.js`|`/tmp/jimeng-plus-1112.b5ee721939.js`|`44bfc11d03506f79c828ec9b8655f2fa01cc9a532966bce94c05a4dda4f50aef`|
|`static/js/canvas-view.e4df37de47.js`|`/tmp/jimeng-calibration-30.js`|`4b927f82c62b6386ec6ddb5f91a8c2d92c20590dc06e03bd9d1831dca3a23e0b`|
|`static/js/canvas-core.8751e7ea24.js`|`/tmp/jimeng-calibration-37.js`|`8f194bf4096477bf048a451313b9c959da8f50e1887b6e306ebf406b3c75c0a6`|

URL前缀均为 `https://lf3-lv-buz.vlabstatic.com/obj/image-lvweb-buz/ies/lvweb/octo_web/`。以下offset是UTF-8解码后字符串的0基字符位置，未格式化原文件。

## 1. 视觉圆钮、DOM端口、点击热区分离

**DOM端口几何**：宽60、高120世界单位，居节点左右边缘中点。目标端口相对节点 `x=-30,y=nodeHeight/2-60`，源端口 `x=nodeWidth-30,y=nodeHeight/2-60`（core模块96562，offset6346）。因此端口中心始终是节点边缘中点，不随glyph指示圆钮移动。

**chrome反缩放**：`c=1/max(validZoom,0.5)`，有效zoom为正有限值，否则按1（core offset4298）。

**默认glyph**（1112 offset18604–19090）：

- source：x=`30+22*c`，y=60；CSS `calc(50% + 22px * var(--octo-canvas-node-chrome-counter-scale,1))`。
- target：x=`30-22*c`，y=60。
- multi-selection聚合source：x=30、y=60；其chrome counter scale被置1。
- glyph伪元素 `after:size-5` =20×20 CSS设计尺寸，绘制圆边、背景和8×1/1×8加号（view token offset146655）。在正常zoom≥.5时通过c抵消zoom，屏幕直径20px、默认距节点边22px；zoom<.5时按20*2*zoom缩小。
- 端口元素本身 `pointerEvents:none`，伪元素before才负责鼠标命中。before width40、height80，scale=c；source位于节点边右侧，target位于节点边左侧，竖直居中。故zoom≥.5时屏幕热区约40×80px，**不是glyph的20px圆，也不是60×120 DOM盒**。
- width40/height80取自view token offset231143/166005；1112把这些class挂在before，offset20950附近。

若父任务测得node rect(437.348,112,227.304,227.304)、zoom≈.710324，则节点右边664.652、中心Y225.652；默认source glyph center约(686.652,225.652)，source DOM盒约(643.342,183.033,42.619,85.239)，正常before热区约(664.652,185.652,40,80)。这些数值由给定rect推导，须以DOM实际computed geometry确认边框/舍入。

## 2. glyph二维跟随与平滑

1112 `function ec(e)` offset23631，WeakMap保存每个handle的当前值、目标值、pointer、RAF、上帧时间。pointermove只刷新采样并确保最多一个RAF；写入 `data-canvas-connection-glyph-tracking=true`。

每次有新pointer样本，以端口元素实际client rect归一：

```text
rawX=(pointerClientX-rect.left)/rect.width*60
rawY=(pointerClientY-rect.top)/rect.height*120
source targetX=clamp(rawX,30,90)
target targetX=clamp(rawX,-30,30)
targetY=clamp(rawY,0,120)
```

限位位置1112 offset24471。X/Y均为handle局部世界坐标；不能误当CSS屏幕像素。

首次current优先读取`getComputedStyle(handle,'::after').left/top`，若不是有限数字则用默认30±22*c、60；聚合source的默认X是30。

```text
dt=首帧1000/60；随后clamp(rafTimestamp-lastFrameAt,0,64) ms
a=1-exp(-dt/80)
currentX += (targetX-currentX)*a
currentY += (targetY-currentY)*a
若两轴剩余距离均<=0.05：精确落到target，清lastFrameAt，停止续帧
CSS坐标写Number(value.toFixed(2))+'px'
```

证据：指数公式offset25443，收敛阈值25582，坐标舍入26303。时间常数80ms，非固定duration CSS transition，也不是逐帧固定0.2lerp。初次60Hz帧a≈0.188064；64ms上限帧a≈0.550671。

reduced-motion（已根据共享函数定义纠正）：每个WeakMap跟随会话首次RAF以 `n.reducedMotion ??= Oeg(window)` 缓存返回值，但此处**不是自动读取媒体查询**。当前main模块23913的`Oe(window, optIn=false)`只有第二参数为true时才查询`(prefers-reduced-motion: reduce)`；glyph仅传window，因此当前发布版本总得到false。即使pointerleave后进入新会话，系统reduce=true也仍执行平滑，这与父任务fresh-session实测一致。此段无change监听，但缓存并不是fresh-session仍平滑的原因。代码中true分支属于条件未开启时的潜在路径，不能当作当前产品已执行行为。离开端口执行`eu`（offset25732）：取消RAF、删WeakMap、删tracking属性、X恢复data默认式、Y恢复50%；没有返回中心的弹簧/缓动动画。

## 3. 显隐与可用性

- 连接口配置`visible`且connectable时before可命中；非连接可用则before pointer-events:none。
- 普通节点hover/selected会揭示glyph；selected时可在被禁用但有disabledReason的情况下展示灰态说明。
- `resultsExpanded`禁用连接，glyph/hotspot缩到0.5并通过expanded类收起；不能按普通端口状态验收。
- 屏幕阅读/键盘入口由独立button `es`（offset22589）提供，鼠标正常pointer-events:none，触发语义为aria-haspopup=menu；sessionPendingDurable时转为有disabledReason的说明入口。
- glyph hover的透明度、字体色、边框色用设计token；本静态审计没有给出最终rgba，需父任务computedStyle读取。

## 4. click vs drag与新节点菜单

### click

view offset710777起，根capture命中 `[data-canvas-connection-handle-interactive="true"]`：读取nodeId和source/target，preventDefault+stopPropagation，登记click_add_line，冻结批量source，开启connectionDrop菜单。

- 普通鼠标click（detail≠0）：菜单/placement使用click的clientX/Y，即指针位置，**不是追随glyph中心，也不是节点中心**。
- 键盘click（detail=0）：使用被触发元素getBoundingClientRect中心。
- `screenToFlowPosition`将client位置转换为创建placement（view offset706329附近）。
- source入口对应createdNodeRole=target、sourceNodeIds=[当前节点]；target入口对应createdNodeRole=source、targetNodeId=当前节点（`rG`，offset636164）。

### drag

ReactFlow端口按下不是立即进入连线拖动。默认`connectionDragThreshold=1`（view offset55251），本页`ip`配置没有覆盖此字段。底层判断为 `dx²+dy² > threshold²`，严格超过1 CSS px才调用onConnectStart（view offset121869）。注意本页nodeClickDistance/nodeDragThreshold=4是节点拖拽规则，**不能替代连线阈值1**。

移动后鼠标/触摸up：有效target则提交；若toNode=null且事件目标是画布pane、仍位于container内，打开connectionDrop菜单，位置取pointerup坐标；释放到非pane不会一律弹菜单。`rQ`只认HTMLElement.classList包含react-flow__pane（offset636584）。失败连接另走拒绝反馈。

如需完全复现“拖动后不再触发click”还应核验根capture对native click的抑制链；当前已定位的底层onPointerDown分支不会单独证明浏览器最终click是否触发。父任务实测可补此项。

## 5. 连接origin与glyph的关系

**确定事实：glyph只改变伪元素left/top，端口DOM盒和handle几何没有跟随。连线origin不跟随glyph。**

### 拖动期间

view `eK.publishPaths` offset476382：源锚点走`ex.Xv`，模块37070 offset128477把位置向外偏移 `4/zoom`世界单位，即4 CSS px；若目标已snap，也向外偏移4 CSS px。因此正常右source为节点右边中点+4px，左target为节点左边中点−4px。自由pointer目标不偏移。连接线endpoint会由candidate吸附反馈额外处理，这不是glyph运动。

### 松手后「添加节点」菜单已打开

view offset480448的pending-creation路径直接使用`e1`读取的handle中心和placement，不调用`Xv`：源端为节点边缘中点，末端为菜单placement。target方向则起点placement→目标节点边缘中点。**不能把拖动期4px外移不加区分地套用到pending-menu预览。**

`e1`（view offset479195）明确用 `node.positionAbsolute + handle.x/y + handle.width/height/2`；这与core 60×120端口中心吻合。

## 6. 核验边界

- 当前loaded-resources包含同版本URL由父任务提供；本报告没有读取浏览器hidden state，也未执行第三方bundle。
- 确定的常数/公式：60×120盒、40×80热区token、20直径、22外距、clamp区间、80ms时间常数、64ms帧上限、0.05收敛、2位小数、1pxdrag阈值、drag与pending不同origin。
- 尚需父任务实时确认：实际CSS counter-scale写入值、hover颜色/边框/背景/透明度、根click抑制、glyph在拖动中/菜单打开时的具体显示状态。
- 多选聚合端口与resultsExpanded不同分支明确存在，本报告不是所有节点类型状态覆盖声明。

## 更正证据：motion查询显式opt-in

- 资源：`main.d6f242f0ae.js`；本地`/tmp/jimeng-plus-main.js`；SHA-256 `4878df36a75344c7057fc666f0938f9020ff0a79242e5dc011a1d9ab2477a874`。来自父任务当前loaded-resources清单。
- 模块96747将`Oeg`导出到`L.Oe`，`T7S`导出到`L.T7`，L绑定模块23913。
- 模块23913函数i：第二参数由`arguments.length>1 && arguments[1]!==undefined && arguments[1]`取得，省略为false；仅true时matchMedia。函数o以同样默认false调用i，最终只有query?.matches===true才返回true。
- glyph调用`(0,v.Oeg)(window)`没有第二参数，故当前reducedMotion字段为false。
- 初稿把函数名直接理解为用户媒体偏好读取，属于未经追踪定义的推断；本段明确撤回“新会话reduce会直接到目标”的表述。
- main offset 2435255：`23913(e`。
- main offset 2435345：`function i(e){let t=arguments.length>1`。
- main offset 2435531：`function o(e){var t;let n=arguments.length>1`。
- main offset 2583349：`Oeg:()=>L.Oe`。
- main offset 2590591：`L=n(23913)`。
