# 即梦连线动效精确标定与交付证据

日期：2026-10-29。对应 REQ-JC-009、设计27 §7.8、PRD14 AC-CJC-03、Plan23 CASE-CJC-03。范围为当前暗色无限画布连线的周期、尾长、颜色、亮度、缩放与预览层序。

## 结论与接受边界

周期、尾长和亮度算法已按即梦当前页面实际加载的渲染资源实现，原生 Figma 关键帧已同步。**同一 WebGL2 管线下，37 个场景、424,746 个有效像素的 RGBA 差异全部为 0**。这证明本次被测暗色 shader 与参考 shader 的数值输出一致，不能代替整个页面、调度方式、浅色主题或所有设备的逐像素接受。

真实页面同截图流程比较：7,217 个流光像素，RGB 平均绝对误差 **0.0378735/255**、RMS **0.209691**、最大单通道误差 **5**，96.4713% 通道完全相同。因此当前证据不宣称真实整页零差异，也不关闭无限画布全量60项。

## 参考身份与实测

- 目标：[即梦调查画布](https://jimeng.jianying.com/ai-tool/ai-canvas/284cfd4e-5e10-4f6e-8fc9-2dbe7097381e?enter_from=project_list&from_page=canvas_homepage)。同源页面脚本、资源时序与可见预览交叉确认。
- 参考资源：`octo-canvas-rendering.3800953894.js`，26,242 bytes，SHA-256 `d9970f672832d62c1e90f47f49bc1378771e69b79ced734228ffc78560d73651`。URL、偏移和公式见[独立审计](source-algorithm-audit.md)。仓库没有复制整个第三方 bundle。
- 70 帧原始 PNG、9.247秒连续采样见 `source-frames/` 与 [时间戳](source-frame-times.json)。图像独立周期估计 **2593ms**；采样平均间隔133.33ms，捕获窗口最大74.29ms，仅粗粒度佐证源码精确 **2600ms**，没有将2593ms用于实现。
- 临时隐藏参考预览基线与流光以分别采样背景、流光；在 finally 中恢复原 visibility。没有新增源节点、连接或发起生成。采样时其他聊天增加的时间线节点不属于本任务产物。

| 项目 | 最终合同 |
| --- | --- |
| 周期 | `(timestamp % 2600) / 2600`，所有边共享时钟 |
| 尾长 | `S=clamp(0.55L,120,312)`，世界坐标弧长 |
| 头尾 | `H=p(L+S+72)`；`T=H-S`，裁剪到曲线路径 |
| 核心 | 13色节点、256×1 RGBA8、LINEAR；直径 `1+sin(πq)` CSS px |
| 辉光 | #008EE5，alpha .22，Gaussian σ 1.5 CSS px，排除核心coverage |
| 合成 | 预乘alpha、WebGL MAX，防止相邻段/交点累加变亮 |
| 精度 | 单线64段，多源fanout16段；DPR backing尺寸四舍五入 |
| 预览层序 | 流光上方绘制蓝色基线；真实DOM z3/z4证据见 source-stacking.json |
| 基线缩放 | authored stroke=1，SVG/CSS复合变换按真实页面执行；100%是1px，不再声称所有zoom固定1px |

修正此前“2.8秒、固定12%尾长、白色亮头/透明尾、所有缩放固定1px”的近似假设。它们保留在旧历史回执中，本记录取代其当前合同。

## 三层图像与数值核对

1. **异构管线诊断**：[isolated-pixel-comparison.json](isolated-pixel-comparison.json)。原始源截图含 Rec2020 Gamut with sRGB Transfer ICC，由 Chrome 解码后，与本地 `gl.readPixels` 手工合成比较。仅拟合时间相位，颜色/宽度/亮度不拟合。MAE2.0153、最大21；该测量混合了两种色彩/合成管线，不能直接解释为产品亮度误差。
2. **同截图管线**：[same-screenshot-comparison.json](same-screenshot-comparison.json)。源 `isolated-flow-07.png` 对本地 `local-isolated-screenshot.png`，均 CDP Page.captureScreenshot，400×110物理像素、DPR2、相同整数裁剪位置与完全相同ICC。本地使用生产模块、原路径/缩放/平移与1745.75ms拟合相位。结果见开头。残差不归因于单一因素；尚未证明其完全消除。背景使用观察到的原截图，非另一套猜测色值。
3. **同 GPU 管线隔离**：[shader-parity.json](shader-parity.json)。同一context、VBO、256色纹理、尺寸、DPR、时间，分别编译静态提取的参考 GLSL 与产品 GLSL，直接读回预乘RGBA。36场景为短线/长线/两来源×60%/100%/136%×650/1300/1745.75/1950ms，加1个原始采样路径，共37。全部0差异。它隔离shader数值，不验证参考mesh实现、页面CSS、Worker调度或ICC；mesh/texture另经独立源码审计。

## 代码、设计与正式浏览器

- 新增 `connection-flow-model.ts`、`connection-flow-surface.ts`、`connection-flow-layer.tsx`；原 SVG 多层近似改为每画布单一 WebGL surface/RAF，保留静态线与命中区。mesh仅变化时重建；隐藏、减少动态效果、空registry取消帧并清空；卸载释放资源。上下文不可用时保留静态线。
- `canvas-connections.tsx` 注册流光；`canvas-editor.tsx` 接入统一层并补多源预览；`canvas.css` 移除旧2.8s近似动效。未改用不到的旧 `connections.tsx`。
- 正式 localhost:1122 画布已确认 running、连续帧运动（28帧、26个不同画面，见 [local-motion.gif](local-motion.gif)）、60%/136%保持核心/辉光CSS尺度、减少动态效果保留静态线、隐藏连线取消并清空。恢复100%与显示连线。对应 `local-*.json/png`。没有触发生成任务或改节点内容。
- [Figma 关键帧](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/Glowloom?node-id=3159-54491)：0/650/1300/1950/2600ms，原8条边ID保持，11处预览基线上置；24 TEXT、20 FRAME、86 VECTOR、0图片填充。详见[设计回执](figma-calibration-validation.md)。原生矢量表达参数与状态；Figma不运行参考WebGL，不能用静态设计证明GPU一致。

## 自动检查

- `cd frontend && npx vitest run tests/unit/components/canvas`：67文件 **382测试通过**，见 [canvas-tests.log](canvas-tests.log)。核心模型先Red再Green，生命周期测试覆盖挂起与清理。
- `cd frontend && npx tsc --noEmit`：通过，见 tsc.log。
- 修改范围8个 TS/TSX 的 `npx eslint`：通过，见 eslint.log；scope为三个flow模块、canvas-connections、canvas-editor和三个对应测试。
- `git diff --check` 对本次路径：通过。没有重跑生产构建、全仓测试、供应商服务或远端CI；不沿用其他提交的CI代替本次检查。
- 验证前本地后端因 `glowloom-local` Temporal namespace 缺失重启。先describe确认不存在，再新建该本地开发namespace（retention24h）；服务恢复后验证正式画布。没有清库/重置项目数据或改动凭据。

## 可复现方式

- `python3 compare-screenshots.py`：使用Pillow和保存的原始PNG重算同ICC截图指标。
- `analyze-source-frames.py` 与 `fit-source-period.py`：以本目录原始连续帧及时间戳重算图像周期。
- 从上述审计URL获取对应worker，在本仓已安装前端依赖的环境执行 `node prepare-harness.mjs <临时输出目录> <下载的worker路径>`。该脚本校验参考SHA、只提取GLSL文本，不执行bundle；从当前产品TS编译本地模块。用仅绑定127.0.0.1的静态服务器提供输出目录，浏览器打开 `/shader-parity.html` 点击运行；`/isolated.html` 用于相位与截图比较。没有向产品加入测试路由。

## Git 范围与未决事项

本次没有 commit/push。共享main还有其他任务大量修改，完整逐项清单见 [worktree-status.txt](worktree-status.txt)，不是提交allowlist。当前文档链、连线代码/测试与本目录是本次涉及范围，其中已有文件还含此前画布增量，后续提交必须按差异审核，不能整仓暂存。

尚未接受：真实整页零像素误差、浅色主题、跨GPU/DPR全矩阵、源码Worker与本地主线程在负载下的调度等价，以及全量60项产品功能。当前证据可接受暗色动效参数和被测shader输出一致，不声称无限画布全量完成。
