# 跨画布保存与生成草稿复制验证

采样：2026-10-29，当前共享未提交工作区。合同：设计27 §7.4.13、PRD14 AC-CJC-03/16、Plan23 CASE-CJC-03/08。本文区分正式代码、受控测试与真实浏览器/供应商证据，不表示JC012/013全量一致。

## 同项目跨画布

节点右键“保存到其他画布”读取正式项目画布列表，选择可写目标后重新GET来源/目标，打开目标正式编辑器。目标提示“保存所选内容”，使用该页既有CanvasYjsLiveSession一次接受节点与内部边；目标个人撤销沿同一会话。未另建隐藏写入会话或旁路保存API，源图不被删除或改写。

选区保留嵌套组后代及内部连线；副本获得新节点/边身份，server-owned input_revision/content_version/last_operation_id不复制。固定库版本、资产和用户显式参考事实保留；内部主体边重映射新节点，目标GenerationInput从新节点修订派生参考。时间线和导演台内部活动node引用重映射；外部活动依赖提示一起选择，不自动丢弃。原composer保持不变。

转移意图仅在内存，隔离session/actor/scope/project以及request身份代次；现有subscribeContextChange释放旧私有意图。不使用localStorage/IDB。保存未确认保留同一意图与新身份，“核验原保存”不再次插入节点。成功须在原Yjs保存完成后GET目标，核对全部节点/边、新节点类型/名称、subject/draft内容及有效input_revision，再显示完成。源撤读、目标不可写/删除、身份变化和内容读回不符不报告成功。

## 跨项目整体复制

审查发现既有ProjectCopy无条件拒绝IsDraft，原因是固定引用和输入快照未定义迁移；不能简单删除守卫。此次扩展闭合范围为正式text/image/video/audio resource的有效composer且无手工固定参考。保留正文、模型选择、模式、参数及标签墓碑，报价时依旧重验目标模型权限；源执行任务身份清除。内部subject边按图身份重映射；库pin继续由专属NewCanvasCopySources验证当前源/未发布目标权限、同scope、固定version及binding；local项目素材沿media复制映射重建。目标节点input_revision=1，持久化GenerationInput快照，receipt重放比较相同输入版本。

旧执行节点、pending upload或手工固定参考未被静默改写/删除后复制。此范围仍不等于任意带手工media/subject/canvas_subject参考的跨项目复制。

## 可执行证据

1. `/tmp/glowloom-canvas-transfer-red.log`：新增前端回归真实发现副本沿用source input_revision=9；清除server-owned字段后通过。`canvas-transfer.test.ts`覆盖同一意图/固定pin/独立composer、不同身份隔离、上下文变化释放、跨项目及只读目标拒绝、外部活动依赖拒绝、完整读回。
2. `/tmp/glowloom-canvas-transfer-copy-pg-red.log`：真实隔离PG的原worker测试加入生成草稿+主体边后，在Freeze报unsupported project copy content。扩展闭合来源与新input快照后通过。中间一次receipt重放因loadCopyDocument未带回新draft input_revision失败；补齐权威读回后原重放测试通过，未绕过断言。
3. `/tmp/glowloom-canvas-transfer-frontend-final.log`：5文件34用例通过（transfer-dialog、transfer-state、clipboard、subject-connections、service-graph）。UI测试使用受控API/client，证明选择流程、只读禁用、源撤权不写、未知结果重试只调用change一次；新增目标创建红例补齐正式createCanvas入口，未知创建结果保留同一Idempotency-Key和原名称；不是浏览器服务端接受证据。
4. `/tmp/glowloom-canvas-transfer-backend-final.jsonl`：14个顶层、19个含子用例通过，0失败、0跳过。正式Yjs fixture+隔离PG覆盖同项目两画布选择复制、源图不变、目标新主体/生成input1和固定媒体、删除undo新input2；跨项目真实worker冻结/复制/重复receipt、撤权拒绝且无目标副作用、同组织跨scope拒绝、目标node_input固定pin及新edge身份。局部素材跨项目映射由领域用例证明，尚非该媒体worker公开API端到端证据。

最终后端命令：

```sh
GLOWLOOM_TEST_SUBJECT_DB_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable' \
GLOWLOOM_TEST_PROJECT_COPY_DB_DSN='postgres://postgres@127.0.0.1:55439/glowloom_subject_alignment_canvas_node?sslmode=disable' \
go -C backend test -race ./tests/canvas ./tests/subject \
  -run '^(TestProjectCopy|TestCanvasSubject)' -count=1 -json
```

Go scoped golangci-lint为0 issues；前端scoped ESLint及整仓tsc通过，分别见`/tmp/glowloom-canvas-transfer-golint-final.log`、`/tmp/glowloom-canvas-transfer-eslint-final.log`、`/tmp/glowloom-canvas-transfer-tsc-final.log`。`git diff --check`通过，无提交。中间测试基础设施错误包括fixture未设viewport.zoom=1、测试环境无toBeDisabled matcher；分别补真实有效视口和检查DOM disabled属性，最终无跳过。该阶段PG实例仍运行供并行子任务使用；03:08所有相关用例结束后由主任务停止，临时数据目录保留。

## 真实浏览器采样（2026-10-29 02:43–02:46 +08:00）

独立 Chrome tab 1980371393；没有操作根代理 tab。通过来源节点右键“保存到其他画布”，全选12节点（可见10、堆叠内2）；正式UI创建目标“跨画布一致性验收-20261029” `5ac4833c-8242-4db8-b296-98b37ebae3a7`，项目 `7fe2cb76-b42b-4fd9-abd4-e70a93173e87`。目标显式“保存所选内容”后banner消失、状态已保存；刷新后正式WebSocket `yjs_snapshot`证实12节点、7边、commitSeq=1。两主体均input_revision=1，库pin、local配置与源一致；四份生成草稿均input_revision=1且内容一致；逐节点对照12个新ID，subject/draft相等。内部3主体边、3相机边、1文本提示边均指向新节点。来源只读取/选择/复制，没有内容修改操作。

精确回执：[browser-cross-canvas-receipt.json](browser-cross-canvas-receipt.json)；刷新截图：[browser-cross-canvas-refreshed.jpg](browser-cross-canvas-refreshed.jpg)。源对照读取commitSeq=54；这是复制后读取，不将它冒充复制前后无其他用户写入的证明。无付费生成、上传或公开分享。刷新后打开“图片 5 副本”生成器，连接主体条指向新主体副本，相机条恢复35mm；截图：[browser-cross-canvas-generation-input.jpg](browser-cross-canvas-generation-input.jpg)。未点击检查费用。

真实失败：保存后首次右键“撤销”仍保留12节点，提示“该对象已有其他编辑，无法单独撤销创建。协作者的内容已保留”。截图：[browser-cross-canvas-undo-conflict.jpg](browser-cross-canvas-undo-conflict.jpg)。此时尚未刷新，菜单撤销可用；没有将其归因于开发HMR后跳过。主任务定位为server normalizeNodeReferenceLabels分配referenceLabels后，history全字段比较将其误判为协作者编辑；主任务补红绿回归并仅忽略服务端标签事实，保留协作者新增边/子节点依赖保护。

## 剩余接受边界

同项目本轮指定来源图的保存/刷新/撤销/重做已有真实接受证据；不将该具体图验收扩大到任意内容或跨项目全量一致。跨项目尚缺手工固定参考的逐类媒体/主体/画布局部输入迁移、完整公开项目copy协调器发布读回，以及真实生成供应商费用/执行结果。旧跨项目guard仍准确阻断这些未迁移来源，不以该限制声称最终全量交付。

工具箱来源边界专项（纯领域，无浏览器生成）：`go -C backend test -race ./tests/canvas -run 'TestProjectCopy(Cinema|RejectsForged)' -count=1`通过，见`/tmp/glowloom-canvas-transfer-cinema-provenance.log`。证明真实复制内部相机边后，GenerationInput source.node_id为目标新ID，hash/35mm/手工配色保留，原图不变；node-config Sources或draft-composer CinemaKit均在既有正式validConfig边界拒绝。此前关于已保存draft残留旧CinemaKit.Sources的担忧经此边界核查不成立；没有为了满足假设放宽配置。

补入工具箱专项后，上述隔离PG总回归重跑为14顶层/19含子用例，0失败0跳过；`golangci-lint run ./tests/canvas`为0 issues（`/tmp/glowloom-canvas-transfer-cinema-lint.log`）。


## 修复后真实浏览器闭环（2026-10-29）

新会话从原source重新选择12节点，再保存到同一目标的既存12节点上，避免沿用HMR之前的撤销栈。正式目标确认：24节点/已保存→右键撤销：12节点/已保存→右键重做：24节点/已保存→刷新：24节点14边。WebSocket分别收到`yjs_ack accepted`，commitSeq 2、3、4；对应updateId为`083e71ea-6f01-453f-8144-eeec283c7eb7`、`8b6fd45b-2290-40d0-bd8a-547e6d55be50`、`64b31098-da9b-41b0-a53e-c0bef7187be5`。撤销后的graphHash与首次12节点快照完全相同，重做与刷新graphHash均为`c189b74fad7c1faf5a819da9a46ad0b71b11c6f31ca9d25a47943443fde51d5e`。

第二批12个节点均新ID；subject/draft逐项与源一致。重做恢复后，两主体和四份生成草稿input_revision均为2，未复用删除前input1，符合固定输入高水位合同。保存回执已移除二进制update正文，见[browser-cross-canvas-history-passed.json](browser-cross-canvas-history-passed.json)。截图：[撤销12](browser-cross-canvas-undo-passed.jpg)、[重做24](browser-cross-canvas-redo-passed.jpg)、[刷新24](browser-cross-canvas-final-refreshed.jpg)。首次真实失败截图和回执保留作为红→绿证据，不覆盖历史。
