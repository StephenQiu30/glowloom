# Pending 提交的跨页面恢复

日期：2026-10-29。此文记录本轮 session/存储实现与自动化回归；真实登录业务验证由根任务另外记录。

## 缺口与修复

此前 CanvasYjsLiveSession 仅以内存保存冻结提交，服务端未确认时退出登录或重新加载会丢失原 op_id、payloadHash 与本地意图。旧 canvas-collab journal 默认同样使用 MemoryJournalDatabase，且旧校验要求数值 ID，不能直接作为当前 UUID 服务的持久化方案。

新增 canvas-yjs-pending-journal，以 IndexedDB 事务落盘；命名空间为 origin、actorID、scopeID、projectID、canvasID，记录主键再含 sessionId 与 epoch。Web Locks 独占原 session 租约，恢复前重读记录，避免从仍活跃的另一个窗口抢占提交。其余待恢复 session 不删除，并以 recoveryRemaining 和状态文案提示。

原 frozen update、confirmed 基线、snapshot、pending、intents、forms、textDrafts 与 recoverySources 共同保存。submit 传输前必须 protect 成功，否则不会发送 yjs_submit。没有保存 Cookie、CSRF 或认证回调；拒绝凭证字段和不支持的记录形态，限制单记录 16 MiB。可选 undefined 字段由 IndexedDB 结构化克隆保留，Map、二进制对象、函数不属于该协议记录并被拒绝。

重开先获取原 session ID，再连接业务会话；只发送原 op_id/原 body 的 yjs_resolve。accepted 正常合并并清理已确认记录；NOT_FOUND 不盲重发，而保留可阅读恢复草稿。用户显式选择恢复字段后才生成新 op。跨 epoch 查询旧操作后归档旧来源，绘制当前权威图；403/404 清除本页可见数据但保留本机草稿。另一个 actor 的恢复查询不会读取该记录。

## 回归证据

- durable-session-red.log：三个真实 session fixture 场景在接线前失败，覆盖重开原 identity、403 保留与 quota 阻断。
- durable-session-epoch-red.log：扩大回归发现跨 epoch 归档后旧乐观图未立即刷新；增加 render 后修复。
- durable-session-82-green.log：既有 73 项 session 回归、5 项新 session 场景及 4 项 journal 场景通过。随后补充 optional undefined / 非法对象形态用例，最终结果见 durable-final-validation.json。
- 存储真实浏览器层验证见 idb-browser-validation.md；使用独立 origin 与合成记录，不等同于业务重新登录验收。

## 边界

已发送的提交必须先完成 IndexedDB 事务，才进入网络传输。仍在输入中的内容会持续保护，并在 stop 前尽力保存；浏览器强制杀进程、设备磁盘损坏或系统清理存储不承诺可恢复。存储不可用、配额失败会显示“本机草稿保存失败，请勿关闭页面”，不会把它标为安全离开；既有 dirty 离开保护继续生效。此轮不更改身份认证、权限或服务端 op 查证协议。

## 音频目标输入保护

canAcceptAudioConnectionInput(nodeID) 检查目标存在、权限、保留的输入草稿、该目标 inputGuard、binding 冲突/应用阶段、输入法组合、待选模式，以及当前 composer 与共享 draft 的差异。其它节点的编辑保护不阻塞该目标。该 API 与 canAcceptConnectionGraph 配合使用，后者继续负责整图 UNKNOWN/inFlight/待上传准入。audio-input-guard-red.log 保存接口接入前的 3 项失败，durable-audio-session-86-green.log 为新增保护与全部 session/journal 86 项通过；属于组件/协议 fixture，不冒充真实服务故障。

## 拒绝回执的追加回归

独立检查发现重开后的原操作被持久拒绝时，归档后的 restoringOriginal 标志未清，导致用户明确恢复字段仍被 queryOnly 限制。durable-rejected-red.log 记录“显式恢复后没有 submit”的真实断言失败；清理归档状态并先保存恢复记录后，durable-rejected-green.log 的同场景通过。NOT_FOUND、accepted 与 rejected 均不自动另建提交，只有用户明确应用恢复字段才会产生新 op_id。
