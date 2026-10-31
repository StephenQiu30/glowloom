# 高级组合连线与异常保存：真实浏览器验收

日期：2026-10-29。该记录区分浏览器真实上传与服务端回执、隔离 PostgreSQL、会话 fixture 和仍未闭环事项。全量一致性尚未通过。

## 已交付基线

本轮多选连线代码及设计文档已限定提交 `ae85b5f4b09decd83e89d0a8b555d5a355c9d190` 到 main 并推送。对应 CI [40822425486](https://github.com/StephenQiu30/glowloom-server/actions/runs/40822425486) 终态 success。随后高级领域连接补齐属于后续工作，不用此 CI 代表后续工作验证。共享 main 后续由并行导演台任务推进，结束前另记当前 HEAD/remote；未把并行文件统一暂存。

## 浏览器条件

- 即梦独立研究画布 `52298cdd-795e-4f5a-af85-47eb2fb84b6e`，未改用户原研究画布。
- 本地真实项目 `c6ba88ae-048b-4035-ae89-5be907ff1a3a`、画布 `1d622b24-44f9-43d1-886d-affffa744115`。
- 通过资产库真实上传已知合成图片、4 秒视频、7 秒音频。无模型生成或积分消耗。这证明上传素材链路，不证明真实生成结果接入。
- 服务端 epoch `76a8ab91-62da-422a-9358-e4a37a9485a4`。回执仅保留图标识与提交身份，未归档鉴权或签名媒体 URL。

## 通过的浏览器闭环

| 场景 | 实际结果 | 证据 |
|---|---|---|
| 图片→新时间线 | 创建目标、1 条连接、5 秒主视觉片段，accepted seq2/rev3；重开保留 | advanced-local-timeline-ack.json；advanced-local-reopen.png/txt |
| 图片→新主体 | 主媒体绑定和连接一起创建，accepted seq3/rev4 | advanced-local-subject-ack.json |
| 主体创建整批撤销/重做 | seq4/rev5 回到两节点图；seq5/rev6 恢复三节点图；重做 graphHash 与创建完全一致；刷新后3节点2边 | advanced-local-subject-history-ack.json；advanced-local-reopen.png/txt |
| 视频+音频→新时间线 | 多选聚合 + 创建；4 秒视觉片段、7 秒音频片段、总长7秒、两条连接；6节点4边 | advanced-local-mixed-reopened.txt；advanced-local-mixed-readback.json |
| 混合时间线整批撤销/重做/刷新 | 撤销变5节点，重做变6节点；重做accepted seq10/rev11；刷新权威snapshot仍seq10且同hash，6节点4边 | advanced-local-mixed-redo-ack.json；advanced-local-mixed-readback.json |

最后一项权威图 hash：`79763d16cf08a6be97ecdeef41211b199d284f09cd044cb74e260fd14c741102`。创建/撤销阶段 CDP Network 观察未启用，不能捏造对应 ACK；已启用后的重做 accepted 与刷新快照完成持久化证明。只计上述经过操作的组合。

## 真实异常：未确认保存后会话失效

组合验收后整理节点位置，第一次移动主体已 accepted（seq11/rev12）。第二次移动视频发出原提交 `13aa9669-e60e-4943-bacd-b3c43e3216da` 后 WebSocket 关闭，未观察到该提交的 accepted。

实际界面显示“保存结果未确认”“画布连接暂不可用”，切为只读，并隐藏连接入口；原图6节点4边及片段保留在页内。点击“重新连接并查证”未恢复，之后页面跳转 `/login`。服务日志可见会话接口401，但未据此推定是过期、其它会话操作或服务故障导致。

证据：`advanced-local-unknown-save.png/txt`、`advanced-local-unknown-events.json`。用户已在 Chrome 重新登录；未索取密码、未伪造会话、未重造业务提交。重开权威snapshot为seq11/hash `8f3310730fcf2cc9f0770508c0e48b4de41183d5f206d9fc7f67c6063a220aab`，六节点四边保留，视频仍为移动前坐标x571/y296。未确认移动既未进入服务端，也没有待恢复意图入口。已确认跨页面未确认图意图持久化存在缺口，正在补齐；当前不能把此移动的查证恢复/刷新保留判为通过；更不能用移动故障替代“高级连接本身ACK丢失”的全部验收。

先前临时页离线模拟未形成可靠故障闭环，已关闭该临时页，不计通过。会话级 UNKNOWN/inFlight 测试通过也不替代以上真实浏览器缺口。

## 即梦实测边界

即梦视频+音频创建时间线和整批撤销已观察通过；重做实际返回 `write_scope_denied` 并提示本页未保存。刷新回到9节点8边，不得把重做的本地10节点10边标成已保存。证据 `source-write-scope-denied.png/txt`。

混合V+A时即梦 Audio 菜单禁用，本地启用，仍有差异。已读当前下载bundle：音频创建会校验草稿/模型能力与acceptedTypes；未找到固定V+A组合禁用或7秒时长导致该preview禁用的规则。静态材料不足以证明当前页面命中的具体分支。详见 `audio-connected-creation-rule-audit.md`；禁止为单一截图写死错误组合规则。

## 其它验证及未完成项

- `advanced-frontend-gates.md`：29文件298测试、全局TypeScript、54路径ESLint、Next生产构建通过。此前失败命令和初轮失败保留。
- `timeline-relations-postgres.log`：隔离真实PG+Yjs事务，真实素材时长漂移整批拒绝；正确图accepted后新service读回；裸断边拒绝且图不变。
- `subject-domain-input-validation.md` 与 `subject-input-reader-db-final.log`：隔离真实PG主体输入对项目、资产版本、来源与关系二次核验。
- `save-confirmation-audit.md`：UNKNOWN、15秒等待超时但inFlight仍在时，专用连接准入拒绝第二个新目标；原操作identity不变；一次submit并resolve。属于受控传输fixture。
- Figma9个新增状态及需求/设计/PRD/Plan已同步，但不把静态稿当成运行验收。

剩余：有效会话下原提交恢复；高级创建丢ACK的完整浏览器矩阵；多选拖入已有高级节点全组合；音频模型能力门禁；生成结果来源/主体来源更新语义；导演台产物provenance；整体视觉/动效逐态差分。登录已恢复；跨重登的待确认图意图丢失正在修复，其余缺口持续记入BACKLOG，不能声称全量一致。

## 追加：多选拖入已有时间线

视频+音频从聚合入口拖入已有图片时间线：一次accepted seq14/rev15，原图6节点扩为6条连接，原图片0–5秒保留，视频追加至5–9秒，音频另轨0–7秒，总长9秒。整批undo accepted seq15/rev16回退到只有图片；redo accepted seq16/rev17恢复3片段。重连/重新加载采集权威snapshot为6节点6边，hash与创建及重做一致：`abf488b3925c53b035ed2e70180ba9a9317fd3d8165a65d5027f3fa1bd666014`。

证据：`advanced-local-existing-timeline-events.json`、`advanced-local-existing-history-ack.json`、`advanced-local-existing-reopened.json`。其中撤销ACK重复送达两次但相同updateId/seq，不计第二次提交。过程中一次服务连接中断后重连恢复；后续页面曾显示登录状态读取失败，重试后恢复。运行环境读取不稳定已如实保留，没有跳过失败直接宣称全流程无异常。

## 追加：高级创建真实回执超时的同页恢复

单个4秒视频→新主体2，创建出现“回执超时，等待心跳查证”。CDP记录一次submit `ed17f168-345c-4b50-a261-987bde4da00b`，随后WebSocket关闭；恢复时同一op_id、同一payloadHash `86b821c169afbd5b4f90d4586570a02730da39e334842bc19d962b9328e31c75` 进行resolve，收到accepted seq17/rev18/hash `2d3290b82983f9aacab9260ef7f2e2b70d202879a66d322f80a7e5a401d44042`。未观察第二次submit。页面7节点，原内容保留。证据 `advanced-local-video-subject-unknown-events.json`。

这是真实高级创建的同页查证闭环，不能与首次移动跨登录丢失意图混为一项。之后刷新遇浏览器连接超时且本地再次跳登录，暂缺该主体的新页面权威读回。

## 追加：音频菜单单来源对照

同一即梦研究画布、相同就绪素材：7秒音频单选→Audio可用；4秒视频单选→Audio禁用；V+A混选→Audio禁用。因此不能误报为仅混选互斥；该页面视频来源本身已被目标音频能力排除。结合静态门禁证据，仍需接正式目标音频model/version/mode能力，而不是硬编码视频一律不可连任何音频。证据 `advanced-source-audio-single-menu.txt` / `advanced-source-video-single-menu.txt`。

## 追加：重新登录后的读取与持久层修复

用户再次完成登录后，通过原地址重新打开：画布显示7个节点，主体2仍存在，时间线1为9秒、时间线2为7秒，素材名称与排列保留。初次打开显示已保存；为采集权威snapshot再次刷新后出现打开超时，重试重新渲染7节点，但媒体读取失败、协作状态间歇断开。此次未捕获可核对hash的snapshot，故只把新页面节点/片段可见记为通过，服务连接及媒体恢复仍未通过。证据 `advanced-local-video-subject-reopened.txt/png` 如实显示异常状态。

跨页面意图丢失已补 IndexedDB + Web Locks 保存原session/op/payloadHash、先查证后恢复。真实Chrome最终helper重开、actor隔离通过，详见 `idb-browser-validation.md`；会话受控传输回归详见 `durable-pending-recovery.md`。这些不替代真实账号跨登录的完整故障重演。

## 追加：音频能力门禁实现

菜单预览、单/多来源连接和左侧新建来源统一接入正式模型目录。默认模型不随来源偷偷变化；预置草稿保持精确版本和模式；确认时重新读取目录并核对来源、目标、epoch和输入法/未提交编辑状态。主体库版本仅查询当前选中或连线命中的来源集合，按空间、主体、版本校验实际素材类型并取消过期请求。连线预览缓存加入来源节点/方向/集合身份，避免换来源后复用错误的禁用结果。

54项独立能力/目录/主体缓存/协调器回归分别通过，另菜单3项串行通过；详见 audio-capability-final.log、audio-capability-scope.json、audio-menu-serial-pass.log。菜单初轮2项使用未加载matcher报错，改为标准属性断言后1项在负载下超时，原日志保留；串行重跑3项通过，未提高测试超时。最终合并门禁另记，不把这些结果当成真实模型服务或正式页面全部组合验收。

反向拖拽补查：已有音频节点从左入口连接未选中的固定主体时，按命中的来源集合预取；多个重叠候选在一次指针事件末尾合并prepare，避免逐项取消请求。独立只读复查确认缓存命中也会收集、无render副作用。该微调晚于最终构建的编译阶段，最终文件另行ESLint检查，不把先前编译结果充作最终快照完整构建证明。

## 最终门禁与交付状态

最终代码生产重建通过（exit 0，86.37秒），69个核对路径在重建前后SHA无漂移；此次覆盖最终反向入口补线。原全局tsc和第一次构建曾被并行时间线测试的隐式this类型阻塞，后该测试补上HTMLAnchorElement类型；未冒充独立tsc重跑通过。见 durable-audio-final-rebuild-result.json 与对应log。

36文件369项关联测试首轮368通过、1项cinema picker默认5秒超时；同文件定向复核原失败用例通过，另一项超时，仍不计整体单测通过。68文件ESLint及最终editor补检通过。原失败与复核日志保留，不以增加阈值或删除用例掩盖。

交付基线 ae85b5f4 已推main且CI成功；本轮后续高级连接、持久恢复、音频能力与设计增量尚未提交。当前共享main/远端为bd67b045（包含并行任务推进）；未统一暂存共享工作区中的并行变更。真实业务跨登录恢复、所有高级组合、完整视觉/动效差分仍未签收，不报告全量一致。

最终合并门禁汇总见 [durable-audio-final-gates.md](durable-audio-final-gates.md)。

Figma最后构建结论底注同步受阻：写入及间隔20秒后的只读确认均被连接器HTTP429/Cloudflare拒绝，未确认远端写入；未伪造重导出。`figma-durable-final-frame.png`是最终构建完成前的真实整帧，底注仍显示“最终构建未确认”，该构建结论已被最终重建exit0取代。其余87项/音频54+菜单3项及业务待验注释已读回；连接器恢复后仅需更新3243:63838末句并重导出。
