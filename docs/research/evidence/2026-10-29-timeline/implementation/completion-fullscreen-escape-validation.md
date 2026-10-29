# Escape 主动退出播放器补充修复

2026-10-29 12:18 Asia/Shanghai。

上一轮只禁止了 Radix Dialog 跟随 Escape 关闭，但真实浏览器复验发现播放器仍处于全屏，退出标签未消失。因此补强合同：当 Dialog 接到 Escape 且拥有播放器全屏时，preventDefault 防止关闭编辑器，并显式调用 document.exitFullscreen；当浏览器已先退出，仍只屏蔽同次 Escape 的 Dialog dismiss。

测试先复现 exitFullscreen 调用次数为0的失败，修复后工作区14项测试全部通过；重点断言 Escape 先到时exitFullscreen调用一次、fullscreenElement清空、按钮恢复“全屏播放”、编辑器仍打开。浏览器先退出时不重复调用exitFullscreen，保护窗过后正常Escape可以关闭编辑器。

日志 completion-fullscreen-escape-tests.log。此次是工作区定向回归；此前156项组合记录属于12:15版本，不能把定向14项混同为再次跑完整组合。真实浏览器复验由主任务记录。未提交、未推送。

本轮scope ESLint通过；全量tsc被 `src/app/api/[...path]/route.ts:701` 的联合类型缺少query属性阻塞，时间线无诊断。对应日志另存completion-fullscreen-escape-eslint.log/tsc.log。
