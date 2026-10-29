# 时间线缩略图补充验证

日期：2026-10-29 11:49 Asia/Shanghai。

## 问题与修复

视频缩略图原来仅在 `loadeddata` 处理非零 `startMS`，因此同一片段裁剪后不会重取对应帧，撤销到零秒也不会回到首帧。本次以视频 ref 配合入点 effect，在 `readyState >= HAVE_METADATA` 时更新寻址；未就绪时等待 `loadedmetadata`/`loadeddata`，事件使用最新入点。目标限制在可用源时长内。React 管理 src，不在 Effect 清理中删除，避免 StrictMode 模拟卸载导致丢源。

音频素材卡和多个裁剪片段原来各自 fetch/decode 完整音频。本次同一授权 URL 仅在消费者挂载期间共享 128 个峰值；不同 URL 最多两个并行解码。最后消费者卸载后 Abort、关闭 AudioContext 并删除共享条目；不保留 PCM，不写入持久化存储。失败条目允许同 URL 显式 retryGeneration 重试。仍保留现有 32 MiB 压缩文件限制，并在收到字节后再次检查长度。

**限制：32 MiB 压缩大小不是解码 PCM 内存上限。** 本次消除同源重复解码并限制并发，不能替代 TLA-46 的长媒体、低内存真实浏览器验收。128 峰值是缩略图，不是精确采样编辑视图。

## 自动化结果

- `pnpm exec vitest run tests/unit/components/canvas/timeline/timeline-thumbnail.test.tsx`：7/7 通过，见 `completion-thumbnail-tests.log`。覆盖 ready 裁剪/undo零秒、未ready和授权URL刷新、StrictMode源保留、共享同源解码、不同源并发上限、迟到卸载结果、显式重试、既有压缩字节门槛。
- `pnpm exec eslint src/components/canvas/timeline/timeline-thumbnail.tsx tests/unit/components/canvas/timeline/timeline-thumbnail.test.tsx`：通过。
- `pnpm exec tsc --noEmit`：11:49 本轮通过。前一轮11:41并行连线错误记录保留原样，不回写为当时通过。

这些是组件和浏览器API替身测试。实际缩略图像素是否与源入点一致，仍由浏览器裁剪/撤销截图或帧比对签收。映射 TLA-20、TLA-27、TLA-31、TLA-46、TLA-50。未提交、未推送。
