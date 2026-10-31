# 画布生成输入与保存确认验证

日期：2026-10-28。范围仅为 `CanvasYjsLiveSession` 的生成输入提交、保存等待和现有 `ComposerSession` 绑定；不是即梦全量一致性的验收结论。

## 实现合同

- 生成输入仍经 `config.draft.composer`、既有 Yjs 文档及唯一 socket 队列保存，协议保持 4；没有增加 REST 写路径、localStorage 或 IndexedDB 业务存储。
- `changeInput(nodeID, draft)` 返回 boolean。正常编辑进入原有队列；组合输入、正文未结束、失权、删除或输入冲突时保留于 `CollaborationSession.forms[nodeID].canvasInput`，返回 false。只读初始化发出的同内容 `onChange` 是无写入的幂等操作。
- `prepareInput` 不负责创建空输入。宿主显式初始化后，必须等队列 CLEAN、无在途提交、权威 DTO 与 accepted contentRevision 一致，且节点确有安全整数 `input_revision` 才返回 `canvas_input`。稳定 JSON 比较忽略属性顺序，并补齐 Go Draft 的四个零值字段。
- 原生输入法、正文编辑、模式选择未完成、待上传引用、失败上传、UNKNOWN、离线、恢复流程及失权均不能生成；等待期间和 Promise 恢复后再次检查输入与版本，避免同一事件循环内使用旧输入。
- `bindInputComposer` 原位绑定现有编辑器。干净会话通过匹配 ProseMirror transaction 的 `applyAuthoritative` 接收新权威输入；本页未提交编辑与远端变化相遇时保留草稿，不自动覆盖。输入法 DOM 尚未产生 transaction 时也读取当前可解析文本。
- `readInputDraft` 与 `view.inputDrafts` 暴露保留稿。`resolveInputDraft(nodeID, "server" | "local")` 是显式冲突处理：使用服务端版本，或基于当前版本重新提交本页稿。输入法未结束、节点缺失时拒绝处理并保留；关闭面板不清空稿。
- epoch 变化的 forms 使用现有 archiveRecovery / recoveryFields；停止或访问权限完全失效后的保留稿仅存于该页会话投影，不写第二套持久存储。页面销毁后的持久恢复不在此合同内。

## Red → Green

1. 原实现会在 prepare 阶段隐式创建空输入、提交未结束正文，且 Go 零字段 / JSON 字段顺序导致误判；新增回归先失败后修复。
2. 待上传引用最初一直等待保存而不立即拒绝；新增测试先超时，补上传守卫后通过。
3. 已绑定编辑器先改变而 `onChange` 尚未运行时，最初可得到旧输入的 pointer；新增测试先失败，增加未提交编辑检查后通过。
4. CLEAN 等待已 resolve、调用方同一轮又修改输入时，最初仍会返回旧 pointer；新增测试先失败，补 Promise 恢复后的二次检查后通过。

验证命令：

```sh
pnpm --dir frontend exec vitest run tests/unit/components/canvas/yjs-live-session.test.ts tests/unit/components/canvas/service-graph.test.ts tests/unit/components/generation/composer-session.test.ts
pnpm --dir frontend exec tsc --noEmit
pnpm --dir frontend exec eslint src/components/canvas/yjs-live-session.ts tests/unit/components/canvas/yjs-live-session.test.ts
```

测试结果：3 文件、79 测试通过；其中 live-session 37 测试。覆盖单队列并行等待、ACK 先于 DTO、真实 input_revision、JSON 重排、输入变更、输入法、可见 DOM、原位权威回读、两种显式冲突处理、失权/删除/停止、UNKNOWN、epoch 恢复。

测试使用真实 Yjs 与真实 Tiptap / ComposerSession，网络为受控 WebSocket fixture。不是远端服务、真实模型或浏览器验收；后续仍须真实生成全链路与多端协作验收。浏览器生成输入保存/刷新由根任务另行记录。
