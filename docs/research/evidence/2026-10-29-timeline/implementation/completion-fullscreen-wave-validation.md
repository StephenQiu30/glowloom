# 播放器全屏与波形归一化补充回归

日期：2026-10-29 12:16 Asia/Shanghai。对应 TLA-33/34/37/50 和音频素材/轨道视觉差异。

## 全屏播放器

真实浏览器验收发现：只调用 requestFullscreen，没有退出按钮；原生 Escape 退出全屏时 Radix 时间线编辑器同时关闭。

修复：以 fullscreenchange 维护播放器状态与进入/退出图标、可访问标签和 aria-pressed；同一按钮切换 requestFullscreen/exitFullscreen。退出播放器后保留时间线编辑器。Dialog Escape 同时检查同步全屏状态和300毫秒退出保护窗，覆盖浏览器先触发 fullscreenchange 再送达 Escape 的顺序；保护窗之后正常 Escape 仍可关闭编辑器。工作区卸载时移除监听，并退出自己拥有的全屏。

新增5项实际组件测试（Fullscreen API 为替身）：toggle和外部退出、Escape在 fullscreenchange 前/后两种次序、拒绝请求不改变界面、卸载退出。测试精确覆盖实际发现的问题，但不能代替浏览器再验收。

## 音频波形

128峰值按整份素材的最大峰值归一化，再按片段源区间裁切。低音量660Hz样本的素材卡可以满高显示；裁剪后的低幅度片段仍维持相对幅度，不能单独再次拉满。静音峰值全零保持中线，不除零。

回归用前半段0.125、后半段0.0625的样本验证：素材最大峰值对应完整高度，后半段裁剪对应一半幅度；同源仍只解码一次。另测零幅度无NaN/Infinity。颜色、48px/22px等CSS尺寸由主任务另改，本子任务未覆盖其修改。

## 检查结果

- timeline目录 + yjs-live-session：10文件、156项通过（12:15最终组合）。
- workspace/thumbnail及其测试 ESLint：无诊断。
- 全量 `tsc --noEmit`：被并行新增 `canvas-graph-commit.test.ts:2` 缺少 `@/components/canvas/lib/canvas-graph-commit` 阻塞。本轮时间线文件无诊断；不能将本次全量类型检查标为通过。

对应日志：completion-fullscreen-wave-tests.log、completion-fullscreen-wave-eslint.log、completion-fullscreen-wave-tsc.log。

未提交、未推送。修改保留在共享工作区；其他功能与CSS改动保留。
