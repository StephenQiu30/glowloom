# 流星连线验证（2026-10-29）

需求：REQ-JC-008；唯一设计来源：../../../design/27-无限画布一致性改造设计.md §7.8。

## 来源与边界

用户对比截图确认即梦待创建连接是细蓝连续曲线，要求流星雨风格。实际即梦页面在文本1右侧连接入口显示七类添加菜单，DOM存在独立 `canvas-connection-flow-layer` Canvas；未创建节点、未调用生成。其 Canvas 光迹不能由SVG样式反推周期；2.8s、12%尾长与颜色值属于当前实现参数，未宣称即梦逐帧完全一致。source-a/b JPG仅为源站现场截图，不作为周期测量。

## 实现

- `frontend/src/components/canvas/connections/canvas-connections.tsx`：创建/吸附/待创建菜单复用连续蓝线；相关、选中、生成中、悬停采用移动亮头和12层渐隐尾，空闲边低对比1px。保留独立16px热区与右键。
- `frontend/src/components/canvas/canvas.css`：CSS驱动2.8s线性运动；关键帧两端统一px，修复浏览器发现的离散插值。减少动态效果隐藏运动层、取消动画。
- `frontend/tests/unit/components/canvas/connection-meteor.test.tsx`：连续预览及反向吸附方向、选中且生成仍运动、点击/右键、悬停创建/清理。
- 文档：requirement14/design27/PRD14/Plan23同步；Figma原位状态及关键帧见独立回执。

## 验证

新增三例先在原实现失败，再通过。完整canvas目录回归65文件378项通过；相关创建菜单/多源连线也包含其中。全项目TypeScript、修改TSX的ESLint通过。没有重跑供应商或后端测试，变更不涉及这些合同。

真实Chrome localhost:1122，既有画布13节点：

1. 打开待创建菜单，线为rgb(56,182,255)、1px、无虚线。
2. 自由拖拽到空白，未创建节点。实际运行时间推进，连续采样亮头dashoffset从-229.072px→-271.886px→-285.6px；见 `meteor-browser-samples.json` 与 `meteor-local-0/1/2.jpg`。此证据确认连续运动，不代表即梦周期相同。
3. 模拟prefers-reduced-motion：meteor display:none、animation:none，蓝线保留；见 `meteor-reduced-motion.jpg`。检查后清除模拟。
4. Escape取消：预览DOM移除。60%与136%画布变换分别0.60/1.36，线宽均1px，vector-effect为non-scaling-stroke；见 `meteor-scale-checks.json`。
5. 隐藏连线后已连接边与meteor数量均0；随后恢复显示和100%缩放。未改变节点内容或数量。

## 本次文件与Git范围

本次修改仅上述2个源码文件、1个新增测试、4份需求设计执行文档、BACKLOG及本目录证据。工作区既有大量画布和其他任务增量仍保留，不将全量dirty路径作为提交范围；未新增commit/push。逐项清单见 `meteor-worktree-status.txt`，该清单是现场库存而非allowlist。

## 剩余边界

即梦动效精确周期/尾长/颜色需后续同视口连续录制标定；当前视觉交付不构成60项全量一致验收。浏览器验证覆盖静态/关联/创建运动及减少动态偏好；真实供应商生成时的状态切换由组件测试覆盖，未消耗积分触发真实生成。

真实页面演示：`meteor-preview.gif`由28帧浏览器截图生成（7fps预览），用于查看流星形态；精确时序以JSON采样为准，非逐帧速度标定。完整页面见`meteor-preview-final.jpg`。

Figma八条原生连线保留ID完成同步，关键帧板3159:54491；[原生稿读回](meteor-connection-figma-validation.md)记录0图片填充、参数及整板未验范围。
