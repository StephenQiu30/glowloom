# 真实浏览器持久层验收

2026-10-29，Chrome、临时独立 origin `http://127.0.0.1:1147`。生产 helper 经 TypeScript transpileModule 原逻辑转译（仅本地 ES 模块 import 补 .js），依赖真实 IndexedDB 和 Web Locks；合成 UUID、空图、固定操作，不访问业务账号或 API。源文件 hash 见 idb-browser-source-hashes.json。

1. 保存合成 inFlight 后显示 SAVED，会话 7b71506b-af93-4228-9c9a-d0d2341afc0a。
2. 真实刷新页面再点重开：PASS，sessionId 与原 scope.sessionId 相同，op_id 66666666-6666-4666-8666-666666666666 和 payloadHash synthetic-original-hash 保持一致。
3. 切换另一合成 actor：PASS，无可见 record。
4. 仅清除这条合成记录：CLEARED。

首个读取动作遇浏览器命令超时、页面仍等待操作；用当前 DOM 按钮重试后获得结果，未把超时算通过。截图和 AX 见 advanced-idb-real-*。这证明浏览器存储重开和 actor 隔离，不证明真实业务提交、登录后的全链路恢复，也不替代 scope/project/epoch/租约竞争的回归测试。

最终 helper SHA-256 `c456cbcb595d25ebf730bf1c27cc5980ea7f9d8041709344f75fadce34c97d2c` 于本轮重新转译后，重复 save → 真实 reload → read → other actor → clear 全部通过；截图和摘要已替换为该版本结果。
