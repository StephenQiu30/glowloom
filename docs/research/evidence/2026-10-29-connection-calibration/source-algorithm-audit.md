# 即梦连线动效独立算法审计

日期：2026-10-29。范围：只读审计父任务已采集的实际已加载资源；本审计未操作浏览器、未验证逐帧像素，也未修改产品代码。

## 资源身份

- Worker URL：https://jimeng.jianying.com/static-resource/ies/lvweb/octo_web/static/js/async/octo-canvas-rendering.3800953894.js
- 本地：`/tmp/jimeng-connection-worker.js`，26242 bytes。
- SHA-256：`d9970f672832d62c1e90f47f49bc1378771e69b79ced734228ffc78560d73651`。
- 页面代码：`/tmp/jimeng-calibration-30.js`，canvas-view.e4df37de47.js。
- 下列 offset 均为 0 起始字符索引，未格式化原始资源。

## 精确公式

令 L 为世界坐标中的贝塞尔近似弧长，z 为视图缩放，t 为 animation-frame 回调毫秒时间：

- 相位 `p = (t % 2600) / 2600`；周期 **2600 ms**。帧率不是固定60fps：优先 worker requestAnimationFrame，缺失时 setTimeout(16ms) 回调 performance.now()。
- 流段长度 `S = clamp(0.55 * L, 120, 312)`，单位是世界坐标，不是固定12%路径，也不是固定CSS像素。
- 头部弧长 `H = p * (L + S + 72)`；尾部 `T = H - S`；只绘制 `[max(0,T), min(L,H)]`。
- 速度 `v = (L + S + 72) / 2.6` 世界单位/秒；屏幕速度 `z*v` CSS px/秒。不同长度以同一2.6s周期扫过，长线更快。
- 周期末空窗 `2600 * 72 / (L + S + 72)` ms。开头从源端逐步长出，结尾逐步消失，没有尾段环绕到源端。
- 对任意世界弧长 d，流段内位置 `q = clamp((d - T)/S, 0, 1)`。q=0尾，q=1头。
- 核心半径 `r = (1 + sin(pi*q))/2` CSS px，直径1→2→1px。头部不是白色发光点。
- 每个流在同一次render使用共享p，新增流不会单独从0开始。挂起后恢复也读取当前时钟，而不是累计暂停时间。

## 颜色、亮度与合成

暗色主题渐变，从尾q=0到头q=1：

| q | 色值 |
|---|---|
|0|#008EE5|
|.13|#018FE5|
|.27|#14A7EC|
|.41|#1CB1E5|
|.52|#75CCB9|
|.60|#D6B20A|
|.68|#E29618|
|.71|#E48120|
|.75|#E29618|
|.80|#D6B20A|
|.85|#75CCB9|
|.95|#18ACE9|
|1|#008EE5|

- 256×1 RGBA8纹理，各颜色节点按RGB分量线性插值、Math.round量化，alpha全部255。GPU以LINEAR采样。没有线性alpha渐隐尾、没有白色头颜色。
- 抗锯齿：`aa=max(fwidth(capsuleDistance),.001)`；`coreCoverage=1-smoothstep(r-aa,r+aa,capsuleDistance)`。
- 暗色外辉光颜色 #008EE5；距核心外缘距离 `g=max(0,capsuleDistance-r)`；`glowCoverage=.22*exp(-.5*(g/1.5)^2)*(1-coreCoverage)`。
- 核心alpha=coreCoverage。总alpha=核心alpha+辉光coverage；alpha<=.001丢弃。输出预乘alpha，WebGL上下文premultipliedAlpha=true。
- 浅色外辉光sigma=1px、alpha=.10、RGB(0,155,238)；渐变色从页面CSS变量传入，不可将暗色13节点当作浅色值。
- 线段mesh外扩7 CSS px；辉光不是CSS drop-shadow，而是距离场Gaussian。
- WebGL blendEquation(MAX)、blendFunc(ONE,ONE)，防止相邻线段和交叉处重复相加变亮。普通SVG多层alpha叠加不能据此宣称像素级相同。
- 静态基线的候选代码为 #008EE5；最终预览实测为 authored stroke=1、opacity1，并受外层CSS zoom影响。蓝色基线位于流光上方。不能仅从Worker或其他分支推断全缩放固定1 CSS px，以下追加DOM/实帧校正为最终证据。

## 曲线采样、缩放与多源

- 解析单段 `M x y C x1 y1 x2 y2 x3 y3`。
- detailed：64等t段；fanout：16等t段。累积欧氏距离计算L；每节点坐标和累积弧长存Float32Array。
- 相邻采样段生成六顶点四边形，每顶点9float；GPU对每一段绘制端部圆帽距离场，MAX合成连接处。
- z只变换路径坐标及分段屏幕长度。流段S与72间隔在世界坐标中定义，因此屏幕尾长随z缩放；核心宽度与Gaussian sigma保持CSS px不变。
- backing canvas尺寸 round(CSS尺寸*devicePixelRatio)，抗锯齿最终受DPR及GPU采样影响。
- 页面registry批次entries.length>1时为fanout；单条detailed。多源只是减少曲线采样数，周期、渐变、S公式、sigma均不变。
- 页隐藏或interactionSuspended会挂起、取消帧并清空动画层；静态基线另行保留。更正（2026-10-29入口复核）：源码虽有reduced-motion分支，但Oeg(window)未传默认false的opt-in，不能据分支宣称当前源页面响应系统减少动态偏好。详见../2026-10-29-connection-entry/source-audit.md末节；项目保留响应偏好的可访问性策略。

## 索引证据

- Worker `颜色表` offset 107：`let s=[[`。
- Worker `采样预算` offset 1550：`c="fanout"===t?16:64`。
- Worker `周期` offset 3593：`progress:e%2600/2600`。
- Worker `纹理生成` offset 4418：`function P(e)`。
- Worker `扫掠公式` offset 9300：`float sweepLength`。
- Worker `核心半径` offset 10292：`float coreRadius`。
- Worker `辉光` offset 10612：`float glowSigma`。
- Worker `总alpha` offset 10872：`float coreAlpha`。
- Worker `预乘输出` offset 11239：`outputColor =`。
- Worker `最大合成` offset 12985：`e.blendEquation(e.MAX)`。
- Worker `DPR尺寸` offset 692：`Math.round(e.width*e.devicePixelRatio)`。
- Worker `屏幕变换` offset 8073：`vec2 screenStart`。
- Worker `计时来源` offset 23063：`let er=globalThis`。
- canvas-view `多源预算` offset 130753：`let e=t.length>1?"fanout":"detailed"`。
- canvas-view `基线色` offset 128438：`KR:"#008EE5"`。
- canvas-view `挂起条件` offset 485478：`suspended:n||l||c`。
- canvas-view `浅色CSS梯度` offset 484493：`let tf=[[`。

## 推导例值

|L世界单位|S世界单位|速度世界单位/s|末尾空窗ms|
|---:|---:|---:|---:|
|100|120|112.3077|641.0959|
|200|120|150.7692|477.5510|
|300|165|206.5385|348.6034|
|600|312|378.4615|190.2439|
|1000|312|532.3077|135.2601|

## 限制与对产品实现的含义

这是已加载资源的静态公式证据，不单独证明该页每一帧确实执行了该分支。父任务应将源码身份与可见预览、周期/颜色/像素采样对应。若要求“完全一致”，只改2.8s为2.6s并保留SVG叠层仍不满足：必须同时复现长度公式、共享相位、13色纹理、1→2→1核心、距离场辉光、MAX合成，以及64/16采样精度；跨GPU、DPR抗锯齿差异仍应限定对照设备。

## 根任务运行补证与纠正

连续70帧、9247ms范围的独立周期拟合为2593ms，均值采样间隔133.3ms、最大截图时间窗口74.3ms，与源码2600ms相符；不能把2593当作更精确的产品参数。见source-period-fit.json。

源端DOM所有基线/flow祖先opacity1，无filter或混合模式。source-stacking.json确认拖拽/待创建preview位于renderer z4，flowcanvas z3，因此蓝基线在彩色流光上方。上文“屏幕宽1px”的静态推断不足：computed width=1px不消除SVG外层CSS缩放；71%实际帧中约0.71屏幕px基线的复算误差更小。产品保留同类SVG及外层CSS视图变换，流光shader宽度仍固定CSS px。

截图内嵌Rec2020 Gamut with sRGB Transfer ICC。最终比较使用浏览器原生ICC转sRGB，不能直接把PNG原始RGB字节当CSS色值。临时隐藏preview基线及flowcanvas分别采集背景、流光，finally恢复原style；这是受控分层观察，不能冒称未修改状态的完整页面截图。源端同一画布有其他任务并行增加时间线节点，本任务未新增节点、生成或上传。
