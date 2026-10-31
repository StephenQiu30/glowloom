# 持久化恢复与音频连接最终门禁

2026-10-29，未提交或推送 Git。完整命令与覆盖范围见 durable-audio-final-manifest.json。

| 检查 | 实际结果 |
| --- | --- |
| 36 文件、369 项关联测试 | 368 通过、1 默认 5000ms 超时（cinema motion） |
| cinema 原阈值定向复核 | 3 通过、1 默认 5000ms 超时（camera cycles）；原 motion 通过，未改 timeout |
| session + journal | 本轮 87 项在关联集合全部通过；保留新增缺口的 red → green 日志 |
| 68 文件 ESLint | 通过 |
| 最终 editor ESLint 补检 | 通过 |
| 独立 tsc | 当时失败：并行 timeline-export.test.tsx:709 的 this 隐式 any；没有伪造独立重跑通过 |
| 首次 next build | 编译成功，因同一并行 timeline 测试类型错误失败 |
| 修复后的最终快照 next build | 通过，86.37 秒；包含内置 TypeScript 校验，前后 SHA 无漂移 |

第一次门禁期间 root 对 editor 做了两次反向 pinned subject 预取修正，初始 SHA 漂移只涉及 editor。随后并行时间线任务修正了 this: HTMLAnchorElement。第二次 build 因这两个明确变化而运行；冻结哈希见 durable-audio-rebuild-source-hashes.json，终态见 durable-audio-final-rebuild-result.json。原失败日志保留，不能把本次完整测试集合写成全绿。

新增实现包括 IndexedDB + Web Locks 的原提交持久化与恢复、跨账号/作用域隔离、403/epoch 草稿保留、NOT_FOUND/持久拒绝后的显式恢复，以及音频领域连接的目标输入保护。详细契约见 durable-pending-recovery.md。

真实 Chrome 存储层验证使用合成记录与独立 origin，见 idb-browser-validation.md。根任务报告业务页媒体/协作与调试连接不稳定，新增跨登录恢复未完成真实业务签收；本报告不以协议 fixture 或合成存储验证替代该验收。
