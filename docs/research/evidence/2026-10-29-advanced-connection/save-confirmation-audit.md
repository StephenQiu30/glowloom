# 领域连接保存确认审查（2026-10-29）

审查范围：`canvas-domain-connection.ts`、`yjs-live-session.ts`；以当前工作区静态代码及定向会话测试为依据，未操作浏览器。

## 已发现并修复

1. **UNKNOWN 后第二次新目标意图**：原 `emit.writable` 不排除 UNKNOWN，`canAcceptConnectionGraph` 复用普通准入，coordinator `waitForSaved` 拒绝后会释放 busy。用户再次创建全新 target 可产生新节点/边身份，而原提交仍需 resolve。按 root 授权，仅连接专用准入新增 UNKNOWN 拒绝；普通 canAccept 与持久化来源离线编辑合同不变。
2. **15 秒确认等待结束早于原提交确认**：`waitForSaved` 15 秒超时，而 socket 20 秒期限可被 heartbeat 延长；因此未确认提交可能仍为 DIRTY/inFlight，而非 UNKNOWN。仅禁止 UNKNOWN 不足。按 root 授权，专用连接准入同时拒绝 inFlight，直到原提交终态；未修改普通编辑排队。

会话回归覆盖：503 异步转 UNKNOWN 后拒绝新 target，重连 resolve 保留原 op_id 和完整 body，仅一次 submit；15 秒 waiter 超时后普通 canAccept=true、连接专用准入=false，原节点/边及提交身份未改；accepted 并读回后恢复连接准入。新增两项定向测试通过，日志 `unknown-inflight-admission.log`。最初红测试错误地同步断言异步 phase，失败原因已保留于 `unknown-admission-red.log`，修正为等待状态后通过；不把该初始失败冒称产品行为复现。

## 成功判定与并发边界

- coordinator 在素材获取完成后重新核 epoch、权限、source identity、target identity，并从最新 current graph 构建候选；动态 import 之后再次核 current 对象身份和完整候选准入。
- `client.change` 之后等待 `waitForSaved`：必须在线、同 session/epoch、无恢复/上传/输入草稿、session CLEAN、无 inFlight/draining，且 DTO revision 与 confirmed contentRevision 一致。若 DTO 落后会请求 sync。
- coordinator 进一步核 target ID 和本批每条新增 edge ID 均存在于保存 DTO。不会以本地 optimistic graph 存在为成功；没有找到 `change` 未写入但读回校验仍能对本批新边返回成功的直接路径。
- timeout/rejected/unknown 均不自动创建新业务 ID 或调用第二次 change。原业务提交恢复仍由现有 session 负责。返回成功表示该图事务已确认；之后协作者仍可合法改变该图，不表示永久锁定。

以上不替代浏览器端断网/服务端实际错误注入验收；最终整体门禁状态见 `advanced-validation-results-final.json`（生成后）。
