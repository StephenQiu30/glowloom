# 时间线核心补充验证

日期：2026-10-29，记录时间 11:41 Asia/Shanghai。本文件记录自动化技术证据；**不是即梦实测记录，也不代表全部产品验收完成**。浏览器观察与源产品行为必须另外核对。

## 本轮修复与边界

- 每个时间线节点持有独立 Yjs 撤销管理器，只捕获该节点的时间线编辑；画布全局栈不捕获时间线命令。每次已完成编辑切断捕获窗口；节点 A 的 undo/redo、新编辑分支不改变节点 B。会话销毁/替换时销毁全部节点栈。公开 API 为 `timelineHistory(nodeID, direction)` 与 `timelineHistoryState(nodeID)`。
- `useTimelineMedia.retry(assetID?)` 可只重试单素材，保留健康素材对象及播放器。各素材独立刷新、过期与失败；显式重试和自动刷新合计最多六个并发请求。旧会话返回值不进入新项目/新身份；过期 URL 不能因等待刷新而继续展示。
- 播放器清理不再删除 React 管理的 `src`。StrictMode 会模拟执行 Effect 清理但保留 DOM；旧实现因此清空视频、音频来源。新增红绿回归精确复现 `src=null`，修复后保留两类来源，真正卸载仍暂停媒体。真实浏览器 `readyState`/声音/首帧由主验收另记。
- 工作区组件测试使用真实工作区组件和 Radix 控件，媒体 API、会话和预览组件是测试替身。因此它证明事件接线、计算和保护分支，不证明服务端权限或真实媒体解码。

## 命令与结果

在 `frontend` 执行：

- `pnpm exec vitest run tests/unit/components/canvas/timeline tests/unit/components/canvas/yjs-live-session.test.ts`：9 个文件、127 项通过。见 `completion-core-tests.log`。
- `pnpm exec eslint src/components/canvas/yjs-live-session.ts src/components/canvas/timeline/use-timeline-media.ts src/components/canvas/timeline/timeline-model.ts src/components/canvas/timeline/timeline-preview.tsx tests/unit/components/canvas/yjs-live-session.test.ts tests/unit/components/canvas/timeline/timeline-media.test.tsx tests/unit/components/canvas/timeline/timeline-preview.test.tsx tests/unit/components/canvas/timeline/timeline-workspace.test.tsx`：无诊断。见 `completion-core-eslint.log`。
- `pnpm exec tsc --noEmit`：11:35 一次执行通过；11:41 再执行出现并行连线测试错误，见 `completion-core-tsc.log`。错误为缺少 `connections/connection-gesture` 模块，以及 `connection-meteor.test.tsx:117` 的 `anchorOffset` 属性不匹配；本次时间线文件未见类型错误。不能把全项目类型检查记为最终通过。

## 52 场景中的核心证据映射

测试路径根：`frontend/tests/unit/components/canvas/`。

| 场景 | 已有技术证据 | 尚需真实验收的范围 |
| --- | --- | --- |
| TLA-01～04 | 核心数据不参与节点命名；筛选不改变模型。 | 创建、命名、双态与过滤视觉由浏览器验收。 |
| TLA-05 | `timeline-model.test.ts`：import 留资产架，不创建片段，去重。 | 实际上传/资源处理和回读。 |
| TLA-06～09 | `timeline-media.test.tsx`：失败隔离、单素材重试、身份变化、并发上限；模型拒绝错误素材身份/轨道。 | 上传混合失败、50/51 文件、资产库多选、非就绪画布来源。 |
| TLA-10～12 | `timeline-model.test.ts`：4 秒视频＋5 秒图片连续主轨，7 秒独立音频，重复添加保持独立片段。 | 实际素材持续播放与声音。 |
| TLA-13 | 模型跨音频轨移动保留源入点、拒绝锁定/错误类型轨；`timeline-workspace.test.tsx` 鼠标命中目标音频轨和50%画布尺度。 | 即梦第二音轨创建入口尚未观察成功；不能宣称入口完全一致。 |
| TLA-14 | 模型固定1秒头＋6秒尾，尾段移到0秒时插入并把头推至6秒；移至3秒保留空隙。 | 对照已取得的即梦操作记录；复杂多段冲突仍需逐项观察。 |
| TLA-15～20 | 模型分割、左右剪裁、两侧缩放、100毫秒下限、源边界、视觉闭合、音频相邻边界；workspace 50%缩放提交一次，旧epoch拒绝。 | 浏览器拖把手/源入点产出。 |
| TLA-21～22 | 模型插入换序；workspace 拖动仅预览，Escape、pointercancel、权限丢失还原；跨轨移动按画布缩放换算。 | 真实拖出视口、滚动时拖动、触控多指等输入边界。 |
| TLA-23～24 | 模型删除留资产架、视觉闭合；preview 视觉结束后音频仍在、黑场不伪装为空时间线。 | 实际纯音频导出、播放器黑场。 |
| TLA-25～27 | `yjs-live-session.test.ts`：每完成手势一次、不同节点独立栈、undo/redo分支独立、画布栈隔离；workspace slider焦点的Cmd/Ctrl+Z路由到本节点，readonly拒绝。 | 即梦节点A/B历史归属及全屏切换的实际比较。 |
| TLA-28 | 模型吸附开关、邻近边界候选和阈值测试。 | 即梦播放头候选及竞争优先级尚未确认；不把现有算法当作观察。 |
| TLA-29～30 | `timeline-gesture.test.ts`：50/100/200%横向时间换算；模型百分秒与标尺时间格式；workspace50%实际事件接线。 | 时间缩放端点、滚动锚点、原生键盘标尺。 |
| TLA-31～32 | `timeline-preview.test.tsx`：暂停精确seek、授权URL变更恢复源入点、异步play拒绝生命周期、不会每帧重复play；StrictMode来源不清空。 | 真视频首帧/转场/播放漂移、末尾重播。 |
| TLA-33～34 | preview：轨道静音独立、预览音量×片段音量×淡入淡出，单音频失败不影响健康音频；media 单资源重试与失败隔离。 | 听音/频谱、多轨实际混音、导出一致性。 |
| TLA-35～37 | aspect/preview 自动比例回退与9:16框、视频原比例、时长不由比例变更修改。 | 浏览器全屏和已保存工程重新打开。 |
| TLA-38～46 | 此组由导出专项负责；本文件不以单测替代实际下载和NLE打开。 | 文件、包、原生软件互通、取消和失败。 |
| TLA-47 | 模型序列化导入架保留；session legacy imported_assets/null归一不误报CAS；导出冻结等待权威保存并拒绝中途修改。 | 真实刷新/重开、源入点和声音产出。 |
| TLA-48 | session权限失效拒绝写入、保留未接受意图；workspace readonly undo与正在裁剪取消。 | 真实只读账号与后端拒绝验收。 |
| TLA-49 | session过时CAS拒绝，协作者后写时间线不被undo覆盖，冻结时epoch/权限变化拒绝。 | 两窗口真并发、断线重连和服务端持久化。 |
| TLA-50 | media 身份变化/卸载Abort与计时器清理；preview卸载暂停、迟到play拒绝忽略；workspace取消手势。 | 浏览器资源释放和不同节点连续开关。 |
| TLA-51～52 | 本轮不以核心单测签收视觉布局与命名。 | Figma、实际DOM与尺寸逐项对比。 |

## 工程审查

沿用已有客户端边界；未新增服务器组件中的浏览器调用。请求并发集中限制；每个监听、定时器、请求的清理均由当前effect/会话拥有。渲染值由props/配置派生，高频手势用ref，持久化只在提交时进行。未提交、未推送；未覆盖共享工作区其他修改。
