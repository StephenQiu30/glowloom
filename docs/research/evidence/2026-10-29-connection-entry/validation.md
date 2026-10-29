# 连接“+”入口核验与实施

日期：2026-10-29。需求 REQ-JC-010；设计27 §7.9；PRD14 AC-CJC-03；Plan23 CASE-CJC-03。

## 先提交后检查

已按用户顺序将前序连线流光校准以 `3fae5e40e4b60a7c42664f14b040542b9a33c13d` 提交并推送 main，当时本地/远端一致。采用白名单候选内容提交，未混入其他任务的编辑器、主体、导演台等改动，详见前序 calibration/scoped-delivery.md。完成入口核验时其他任务已将 main 推进至 `de6677c7d0a024aa02f99a17f1166f44f98cb6e4`，本地/远端一致且包含 3fae5e40。

[该提交 CI](https://github.com/StephenQiu30/glowloom-server/actions/runs/40812699286) 已终止：backend success，frontend failure。前端1079项通过、1项失败：basic-layout.test.tsx:488 仍期待资产头部“参考素材”按钮；该测试及对应头部不在此提交改动范围。没有把 CI 失败记为通过，也没有修改其他任务资产头部来绕过检查。

本轮随后新增的“+”磁吸入口、手势修正和下列文档仍在工作区，尚未再次提交/推送。Figma 为已写入并读回的设计更新。

## 即梦证据与结论

当前已加载的源码资源与真实DOM、hover/click/drag操作交叉核对，见 source-audit.md、source-loaded-resources.json。研究页面7节点0边，未创建或删除节点、未提交生成。

- “+”是可动的连接入口。逻辑盒60×120世界单位；外侧热区40×80，默认圆环20、边框1、加号8、距节点边22。zoom≥50%维持CSS尺寸，低于50%按画布缩放。
- glyph在外侧热区内二维吸附；80ms指数响应，dt最多64ms，误差≤0.05停止，离开立即复位。默认白35%、hover白100%、背景白4%。公式来自当前发布脚本，DOM采样确认实际移动与颜色/几何；不是从单张截图猜测周期。
- 点击开添加菜单，拖拽阈值严格>1 CSSpx。拖动起点在节点边中点外4CSSpx，pending菜单时回到边中点。源pending起点已有DOM证据；源drag外4的结论来自已加载脚本，source-drag-origin.json为未命中路径的空数组，不能充当运行时测量成功。
- 动态偏好更正：源Oeg(window)未开启第二参数opt-in，当前版本reduce=true仍平滑；项目保留动态响应prefers-reduced-motion的可访问性策略。这是已知差异，不能称全状态完全一致。

## 实施

新CanvasConnectionHandle组件及纯几何函数统一两侧端口；动画只写glyph transform，不让每帧触发React渲染。取消/离开/缩放/卸载清理RAF。旧18px入口改20px，并复用既有Button语义与键盘触发。

连接手势独立于节点4px拖动阈值，记录曾经超过1px的状态，避免拖出再返回被当成点击。拖动和pending分离锚点；空白落点开菜单，控件外部与取消清理，非主鼠标键不发起连接。保留原有连接合法性、提交与同步合同。

## 本地浏览器验收

真实登录页面 localhost:1122，原13节点、3条可见连接线。本轮未新增节点/边或改变节点位置。首先尝试文本节点时发现两侧被其他后置节点遮挡，重新选择未遮挡图片3进行有效验收；local-idle.png属于初始现场，不能作为成功悬停证据。

- 24帧右入口实测：20px（浮点范围19.99988～20.00012）、hover白色；由相邻帧反推时间常数中位79.975ms（四舍五入及调度误差范围75.45～83.56），与80ms合同一致。见local-follow-frames.json / local-measurement-summary.json。
- 离开后立即复位至默认外22，未选中入口隐藏。左右热区均可直接唤醒。
- 鼠标点击打开添加节点；左输入按钮Enter打开反向创建菜单；Escape关闭并清除预览。
- 真实鼠标移动1px时保持原3条基线，2px时多1条拖动预览，Escape回3条；见local-threshold.json。
- 拖到不兼容导演节点时提示无法关联且不新增边；拖到真正空白(300,200)时打开落点菜单。早期(740,250)点实际被导演台覆盖，已明确区分，未误记为空白失败。
- drag起点X=535.815479607，pending起点X=531.333333333，差4.482146274世界单位×当前zoom0.892429≈4CSSpx；见local-drag-origin.json与local-pending-origin.json。
- 结束时13节点、3条原有可见连接线、0菜单、0可见glyph；见local-final-state.json。

本轮没有创建真实有效连接或触发付费生成；有效连接提交、持久化与多选聚合端口不是本轮验收通过声明。只读、zoom<50%、偏好切换和卸载清理由定向测试覆盖，不冒充实际账号权限或全缩放浏览器检查。

## 验证与设计

8个测试文件29项全部通过，覆盖入口几何/RAF/复位/键盘/只读/偏好/卸载、节点可访问性、1px阈值、流光模型与注册层、drag/pending锚点和菜单焦点。TypeScript --noEmit、9个相关TS/TSX文件ESLint通过。源文件格式检查发现本任务CSS两条color-mix换行，已修正后复验；git diff --check通过。本轮未运行生产构建或重新执行全量CI。

Figma新增六态设计板并原位更新24个入口，20px圆环、1px描边、8px加号；详见figma-entry-validation.md与readback.json。静态状态板不是运行时动效证明。未修改全局旧Port主组件，也未宣称所有页面统一；旧菜单部分图标仍沿用原设计。

[六态设计板](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3200-59457)

## 文件范围

- frontend/src/components/canvas/nodes/canvas-connection-handle.tsx
- frontend/src/components/canvas/nodes/canvas-connection-handle-geometry.ts
- frontend/src/components/canvas/nodes/canvas-node.tsx（仅入口替换）
- frontend/src/components/canvas/canvas.css（仅端口样式）
- frontend/src/components/canvas/connections/connection-gesture.ts
- frontend/src/components/canvas/connections/canvas-connections.tsx（活动路径外移参数）
- frontend/src/components/canvas/editor/canvas-editor.tsx（连接手势、清理及路径调用）
- tests/unit/components/canvas/nodes/canvas-connection-handle.test.tsx
- tests/unit/components/canvas/connection-gesture.test.ts
- tests/unit/components/canvas/connection-meteor.test.tsx（drag/pending边界测试）
- 需求14、设计27、PRD14、Plan23、BACKLOG对应增量和本目录证据；前序source-algorithm-audit.md更正reduce判断。

共享文件仍有其他任务修改，后续提交必须逐段检查，不能整仓暂存。
