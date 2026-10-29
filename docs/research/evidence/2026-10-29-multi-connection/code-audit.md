# 多选连接与保存链只读审计

2026-10-29；基线已推送25a9f4d6，检查共享工作区现状。未操作浏览器。源码行号为本次读取值，后续修复可能移动。

## 结论

多选“创建一个后接节点并连全部来源”的代码已存在，纯图构造和Yjs/后端提交均具备批次原子边界；**editor的分开admission会破坏应用原子性**，需先修复。聚合入口仍是旧样式click-only Button，没有接入新磁吸handle或多源拖到既有节点。不能根据单源入口源码推定即梦全部聚合交互。

父任务随后提供真实源证据：两个已相连text，聚合+选择图片一次增加1节点2边；undo整体从3节点3边回到2节点1边；redo整体恢复；刷新保持。这些是父任务实测，不是本审计自行执行。

## 确定缺陷与受限分支

1. **P2：完整图被分拆admission，可能留下半图。** editor:475–509 的setNodes/setConnections分别调用admitGraph；批量应用点1347–1350和单源1229–1232先写操作refs，再逐个调用setter。真实canAccept（yjs-live-session:1576–1583）限制JSON编码小于16MiB。因此可构造新nodes+旧edges低于上限而新nodes+新edges超限：第一个setter接受节点与mode变化，第二个拒绝边。onChange effect（editor:737–756）随后将半图提交。纯helper immutable reduce并不能保护该主机边界。建议完整候选一次admit，成功后同一同步提交更新两state和所有refs；失败不更新、不关闭菜单或显示成功。
2. **当前多选入口只支持新建后接。** selection-toolbar:217–228是28px icon-sm Button，16px Lucide Plus，仅onClick；无新handle的20px glyph/40×80热区/80ms跟随，也没有drag。editor:6405–6420只生成pending.sourceNodeIds；连接拖动路径6008–6018及connectNodes:1242仍只处理一个ConnectionHandle。此为代码能力缺口；即梦聚合drag/磁吸的精确要求待父任务源实测。
3. **非documentIntegration分支只连第一个来源。** editor:1367–1369忽略pending.sourceNodeIds，添加单边。正式Yjs页面始终传integration（yjs-workspace:245–262），所以不是当前正式链已证故障；如旧本地画布仍需支持多选应明确限制或补实现。

## 图构造事实

- canvas-document-connections:27–36 将sourceIDs去重，逐步不可变构造，全部成功才返回。来源为空、来源丢失、目标丢失、非法类型、自连、循环、非multiple输入冲突、1000边上限均可拒绝；第二来源失败不会改变原始state（现有测试已覆盖timeline非法第二来源）。
- 重复是**幂等无操作**：190–197跳过canonical同binding边并返回原state；validateCanvasConnection虽有重复错误文案，正常connectCanvasDocument路径先跳过。不要把期望toast与现实现混淆，也不能由此推断源应提示还是静默。
- edge键含来源/来源port/目标/目标port/binding，不含order；不同file binding可能产生多条逻辑边但重叠在同一几何连线上。验收需对节点/边ID与binding比对，不能只数SVG路径。
- 每新增来源分配同目标port最大order+1；重复sourceID输入被Set稳定去重。
- 连入手工文字会把目标改为GENERATE，并同步将其NODE_VALUE出边改RUN_OUTPUT（198–234）；这个连锁转换应和新边一同undo/保存，不只检查edge数量。
- pending菜单对existingNodes使用every筛选可选类型（create-menus:79–93）；混合选择不会自动跳过不兼容来源。删除来源后的pending会由实际helper再次验证并报错。
- 多源新建位置使用选择包围盒与connectedNodeCenter（editor:1299–1316）；sourceIDs在点击时冻结，而创建时重新读取节点。源操作顺序/创建位置一致性需真实几何合同。

## 原子性与正式保存链

1. editor整图state改变后，onChange effect把nodes+connections提交（737–756），gesture未结束时stable=false。
2. yjs-workspace:261–262绑定admit=client.canAccept、onChange=client.change；yjs-live-session:1582–1588暂存未完成手势，稳定后serviceDocumentFromEditor构建完整服务图（1679–1688）。
3. service-graph:267–291将edge.documentData编码为正式binding；:529–541逐边识别新增和更新；:550起在克隆base上应用差异。读取serviceGraph:113–130恢复相同edge IDs、port、binding和order。
4. CollaborationSession.change（canvas-collab/src/session.mjs:129）调用applyCanvasChanges。model.mjs:352–360先准备两集合，再一次doc.transact执行全部actions；这里不是逐来源事务。
5. yjs-live-session:1271–1276通过yjs_submit/resolve；:1520–1574 drain调度flush，拒绝/未确认有独立状态。storage=memory，未确认状态不能算刷新已保存。
6. WebSocket handler backend/internal/canvas/adapter/websocket/yjs.go:231–268，提交后先syncHead再发完整ack；application/yjs.go:179起先resolve幂等receipt，再准备完整候选。
7. postgres/yjs.go:243–296在DB事务里设projection保存点、持久化正式Document投影，失败回滚projection；成功更新collab state/commitSeq、写update与receipt。完整读回仍需真实服务，代码审计不替代运行证据。

## Undo边界

- editor:2299–2315把正式undo/redo交给integration。
- canvasUndoManager（model.mjs:416–421）使用local-canvas origin，captureTimeout=500ms；所以一次batch可同撤销，但邻近500ms内独立图操作也可能合并。源一次动作整体undo已证，是否要求逐动作历史边界待进一步合同，不能先断言500ms就是错误。
- yjs-live-session:1282–1301为undo item记录before/after完整图；:1773–1859检查协作者新增依赖、其他人修改后阻止破坏性撤销。
- 现有yjs-live-session:1873起虽然调用connectCanvasDocumentSources，但仅[f.nodeID]一个来源，证明单来源创建/标签分配/协作者依赖保护，**不证明多来源整批undo**。

## 当前测试与缺口

只读定向运行：`npx vitest run tests/unit/components/canvas/multi-connections.test.ts tests/unit/components/canvas/yjs-live-session.test.ts -t 'multi-selection connections|allows creation undo after server label allocation|submits the real add-reference editor path' --maxWorkers=2`。
结果2文件、6项通过，58项因过滤未执行；不是全量套件，也不是后端持久化证明。

应补：
- 完整图admission失败时nodes/edges/refs/selection/history都不变，接受时只评估一次最终图。
- 三text中两个来源连接同一新image：一次Yjs更新包含新node+两edge，undo/redo整批，服务标签变化后仍可撤销。
- 空/重复/不存在sourceIDs；第二source造成循环/自连/端口冲突/边数上限时整个结果保持原图。
- 一条已存在一条新增；重复提交不重复边/order，不产生无意义undo item。
- 文字mode提升及旧出边binding改写随批次回滚；混合固定素材/运行输出binding投影往返。
- 取消、只读期间、pending中来源被删除、投影拒绝与UNKNOWN receipt恢复。
- 正式WS accepted epoch/updateId/payloadHash/commitSeq/contentRevision + Document endpoint读回；刷新/重开保持同ID/binding/order，区分刷新前未确认与已保存。

本文件是审计产物，不修改产品源码。父任务已另授权先修admission边界，修复结果应单独记录。

## 后续授权修复附记

父任务已确认源batch重复source/已有关系整批拒绝，并授权修复admission。已实施范围和Red/Green结果另见 `/tmp/multi-connection-admission-implementation.md`。本文前述静默去重与分开admission为修复前审计事实，不表示仍未修。聚合UI/drag由父任务继续，未在本子任务修改。
