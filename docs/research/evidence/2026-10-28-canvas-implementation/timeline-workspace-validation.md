# 时间线编辑与真实导出接入验证

日期：2026-10-29。继续共享 main 上中断的实现；仅负责 timeline 前端、Yjs 时间线方法与对应测试，保留其他并行修改。未提交、推送或发布。

## 实施结果

- `CanvasTimelineWorkspace({client,nodeID,canvasID,projectID,onClose?,compact?,onExpand?})` 提供节点预览与全屏工作区；默认 compact 全屏由组件自身 Radix Dialog 承担，也可用 onExpand 接管。
- 配置沿现有 `config.timeline`，不保存临时素材 URL。复用项目素材库及上传对话框，支持画布媒体引用；同类型分轨。增加、移动、排序、分割、左右剪裁、移除、音量、锁定/静音/隐藏与吸附均进入同一 Yjs 保存通道。缩放、面板尺寸、播放头和监听音量仅为视图状态。
- 分割/剪裁维护原始媒体 source_start_ms；100ms 下限、同轨不重叠、媒体时长、类型和轨道锁按已有服务合同校验。选定导入轨道被删除时拒绝，不静默新建另一轨道。
- 预览使用授权媒体 URL，跟随剪裁起点、片段音量/淡入淡出与轨道静音/隐藏。显式比例按容器尺寸适配；只有空 auto 使用已观察到的 16:9。非空 auto 不推断首媒体规则。
- changeTimeline 校验调用者基线；被拒绝的有效编辑意图保留于画布会话内，可在关闭/重开编辑器后处理。未处理的保留编辑阻止导出；浏览器刷新不会把该内存草稿保存为私有工程。
- getDocumentEpoch 提供已确认 epoch 身份；未连接文档、停止及 epoch 切换中返回 null。异步画布素材导入、打开素材选择器到确认、拖动开始到提交均复核 epoch，拒绝同 ID 历史恢复后的迟到操作。
- prepareTimeline 等待原保存队列接受并校验权威 revision、相同配置、epoch、连接/权限与未知状态。Go 的 null 可选媒体字段与前端省略字段按相同含义比较。
- 真实 mediaExports API 贯通 MP4/M4A 创建、按画布/节点回读最近记录、轮询、取消、重试、产物预览、显式人工检查确认、文件/SRT 下载。创建后校验冻结来源 revision/输出类型；后续回执拒绝其他 job、旧 revision 或变更来源。未知创建结果重试复用原输入和幂等键。
- 实际预览核验 job/revision/sha256、asset/project/kind 与未过期 HTTP URL；下载 MP4/M4A 对真实 Blob 计算 SHA-256。未播放、未明确确认不能提交审核。auto 视频在排队前明确拒绝，M4A 沿原音频导出合同。

## 验证证据

- `pnpm exec vitest run tests/unit/components/canvas/timeline tests/unit/components/canvas/yjs-live-session.test.ts`：时间线模型/比例/预览/导出和 Yjs 回归；5 文件 68 项通过。随后增加 getDocumentEpoch 同 ID 历史恢复测试，Yjs 单文件 45 项通过（原生成输入等 37 项保持，时间线 7 项、epoch 1 项）；两次覆盖合计 69 个不同测试。
- `pnpm exec tsc --noEmit`：后续最终重跑通过；共享资产文件的中间写入错误已解决。新增工程导出后director/timeline/Yjs/theme-policy共9文件107tests通过。
- `pnpm exec eslint src/components/canvas/timeline src/components/canvas/yjs-live-session.ts tests/unit/components/canvas/timeline tests/unit/components/canvas/yjs-live-session.test.ts`：通过。
- 阅读仓库根/frontend AGENTS、design27 §7.4.3、advanced-parity-contracts、advanced-figma-validation、timeline-aspect-validation；视觉参考为已保存正式 Figma `3091:47697` 的 `advanced-figma/timeline.png`，本轮未重新连 Figma 会话。
- 审查采用 Vercel React Best Practices（并行取数、派生状态、事件处理、异步清理）及已安装 Next.js 16.3.6 的 use-client 文档。表单和按钮复用 shadcn，媒体元素使用浏览器原生播放。

## 仍需真实集成验收的边界

以上单元测试含 API/媒体元素替身，不能替代正式用户的非空导入、拖动、播放、刷新双端协作、FFmpeg 导出及下载后播放器检查。时间线工作区接入由主代理继续；本记录不把已有后端比例 FFmpeg 证据重复宣称为本轮新执行。

源端非空 auto 规则和图/视频混视觉轨未有证据；本地仍按已有分型轨合同。后续已实现批量原始媒体包、明确版本的FCPXML1.10和FCP7 xmeml-v5工程ZIP，见project-export-validation.md；源XML身份、剪映及各目标NLE软件导入仍未验收，不能按全菜单完成。当前 SRT 是已有导出任务的字幕下载入口，未新增字幕编辑能力。素材面板使用真实导入按钮，本轮没有实现从桌面直接拖放至素材栏。
