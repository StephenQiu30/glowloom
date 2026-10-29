# 时间线宽度、缩放锚点与首片裁剪留白

2026-10-29，未提交推送。本轮只修改workspace相关片段、新增use-timeline-viewport及测试；保留批量上传等并行改动。

## 源端静态合同

公开脚本已由父任务资源清单确认并读取。位置以下为UTF-8解码字符串中的字符索引，minified文件均为单行；未把源站实现复制到项目。

- `3801.8c6735b2aa.js`，约27954：全屏基础轨宽至少960px，并覆盖实际/预览总时长乘pixelsPerSecond（向上取整）。约60079的ta/tn再覆盖视觉片段及每pending占位84px，存在素材添加入口时追加56px。约44233：画布宽再加52px零点偏移、首片headroom和8px尾边距；最小宽为viewport+headroom。
- 无headroom、无pending且存在添加入口时：`max(viewportWidth, max(960, ceil(durationSeconds * pps)) + 116)`。父任务实测16秒@180、viewport1438：内容2996、末端scroll1558，精确符合该式；低倍14时填满viewport，而非固定65秒。
- 节点独立合同来自`async-canvas-node-timeline.860c26ba2d.js`模块71873/62216：初始32.1px/s、至少30秒并按主刻度取整；内容还容纳片尾6px，节点零点77px（66pxgutter+11px）。节点画布缩放66.7%后屏幕密度约21.4。
- `3801`约22791把滑杆/按钮缩放交给带viewport的缩放回调。`9875.c2c0304a38.js`模块17675（字符73493起）：默认锚点为viewport中心，Ctrl/Meta滚轮为鼠标位置；布局阶段主动写scrollLeft。变换保持锚点所对应的时间，超出内容时由滚动上限截断。所以End升至180后落末端可由主动中心锚定解释，不需假定原生End冒泡。
- 模块17675还给出±按钮系数1.1、滚轮像素/行/页敏感度0.006/0.05/1。
- `9875`模块70491：首视觉片段为video时，headroom=源入点/播放速度；否则0。正常滚动不得进入该左留白，裁剪期间临时开放。当前项目没有变速字段，按固定1倍处理。

## 落地

- 全屏与compact分别计算宽度，lane随viewport填满；ResizeObserver更新视口尺寸。
- 所有缩放入口统一保持锚点，并按内容宽度夹紧scroll；鼠标坐标换算包含画布CSS缩放。普通滚轮不被拦截。
- 首视频源入点产生可滚动headroom，lane、标尺、播放头坐标同步。正常scroll最小值随入点变化；裁剪临时开放到0，取消/提交/undo恢复守卫。
- 裁剪边缘自动滚动、指针delta包含滚动距离，保留起始zoom/scale；pointercancel、Escape、blur、visibility和捕获丢失均清理手势。

## 验证边界

新增几何/滚动测试使用mock viewport尺寸与scroll，不能称为真实布局验收。真实尺寸、滚动和拖拽由父任务刷新正式页面另行核验。

第一次并行检查在本机资源明显变慢时触发多项5000ms超时，见completion-viewport-tests.log / completion-viewport-workspace-tests.log；没有据此跳过断言。随后单worker、明确30000ms测试预算的28项全部通过（completion-viewport-headroom-tests.log）。后续新增首片裁剪坐标/undo集成检查，最终结果见completion-viewport-final-tests.log。ESLint、tsc分别见completion-viewport-eslint.log、completion-viewport-tsc.log。

最终13:21:54定向2文件30项通过（包含滚动补偿裁剪与捕获丢失）；ESLint、Prettier、限定diff whitespace检查通过。全量tsc在13:19时被并行Yjs canAcceptConnectionGraph测试与主体ByRole exact属性诊断阻塞，未修改它们；时间线实现无诊断。

13:26:26最终几何/workspace定向30项通过；13:26:50完整timeline目录+Yjs组合16文件223项通过（涵盖追加媒体错误与导出测试），见completion-final-timeline-suite.log。最后全量tsc以completion-final-timeline-tsc.log为准。

补充源合同：9875模块66613配合3801裁剪调用使用viewport边缘40px、100ms激活延迟和最高300屏幕px/s自动滚动，并按画布scale转换。实现已据此处理。静音gutter拆成全屏44+8、节点66+11，轨道零点与素材坐标不变。

## 正式Chrome动态回读

9秒@180、clientWidth1438、scrollWidth1736；中心缩放夹紧末端298。首片裁剪1秒：4→3秒、ruler margin/scroll 52→232；恢复与稳定连接下100ms裁剪undo均恢复4秒/52。Ctrl滚轮x600：180→165.633777、scroll52→5，鼠标时间误差0.000435秒，外层画布73%未改变。实际40px边缘停留100ms自动滚动尚未单独记录，只有组件证据。
