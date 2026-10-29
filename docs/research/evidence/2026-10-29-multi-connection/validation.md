# 多选连线与真实保存验收

日期：2026-10-29。验收对象为共享本地应用 `localhost:1122` 的真实画布会话；没有伪造网络、修改页面内部状态、调用模型或消耗生成积分。浏览器操作与只读CDP网络观察通过CUA完成；图算法/组件测试另列。任务先提交再继续的顺序已执行。

## 1. 已推送交付

入口专项提交 `25a9f4d63344e711b34423e9f280d3cbdf4cb509`，51项白名单文件。推送时local/remote main完全相同。后续并行主体任务推进到 `90485fdb48ec00a09cfbe518b030a6f788c272ee`，本提交仍是其祖先，当前local/remote该SHA相同。

[该提交CI](https://github.com/StephenQiu30/glowloom-server/actions/runs/40816113557)终态 cancelled：frontend success、backend cancelled，不能记为全CI成功。独立提交候选167文件/1093测试、类型/lint/格式及默认Turbopack生产构建通过；webpack替代构建曾失败于已有explore CSS纯选择器，完整区别保留在前序交付记录。

本目录所述后续多选修正尚未提交/推送。共享工作区另有大量并行改动，完整清单见worktree-status.txt；没有把它们纳入本任务提交，也没有发布部署。

## 2. 来源与本地独立验收对象

- 即梦独立研究画布：`52298cdd-795e-4f5a-af85-47eb2fb84b6e`，标题「连线完整验收-20261029」。没有改原用户研究画布的图结构。
- 本地project：`6836c671-a532-44d9-9b79-b661d2f61b62`；canvas：`2da104f4-b745-4e70-ab80-1fc99750bc3e`。UI标题仍显示未命名画布，身份以此ID为准。
- 精确源码资源SHA与字符偏移：[source-audit.md](source-audit.md)。公开CSS/JS静态阅读不代表运行验收；真实DOM截图另存。
- 原Figma总板：[3153:57117](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3153-57117)，原位修四个聚合入口、五个选框、菜单与两条预览；[读回回执](figma-multi-validation.md)。

## 3. 实测矩阵

| 场景 | 即梦结果 | 本地结果/证据 |
| --- | --- | --- |
| 单来源点击新建文本 | 2节点1边，刷新保持 | 2节点1边，已保存后刷新仍同ID |
| 双文本点击创建图片 | 3节点3边，新增一个目标两条边 | 3节点3边；pending预览2条 |
| 批量创建撤销/重做 | 一次退为2n1e，一次恢复3n3e | 产品重做入口与画布焦点下快捷键通过；整批图恢复 |
| 双文本拖到已有文本body | 4n4e→4n6e | 4n4e→4n6e，body预览2条并38%吸引 |
| 重复同组连接 | 拒绝，4n6e不变 | 拒绝，不新增边；截图local-duplicate-rejected.png |
| 一项已有、一项可新增 | 静态规则整批拒绝 | 撤销到4n4e后选文本1+图片1→文本3，重复图片边使整批拒绝，文本1边未偷偷添加；local-partial-duplicate-rejected.png |
| 批量已有目标撤销/重做 | 4n6e→4n4e→4n6e | 相同；拒绝不破坏redo历史 |
| 回路 | 源码批量整体拒绝 | 文本2+文本3→文本1拒绝，仍4n6e；local-cycle-rejected.png |
| 混合无输出成员 | 来源集合不静默排除timeline；目标规则拒绝 | 文本+临时时间线仍显示聚合入口，创建菜单7项全部禁用、预览保留2条；Escape不写入。已有目标拖拽先命中重复规则，不能冒称单独验证了unsupported失败优先级 |
| 精确菜单 | 200×316、7项 | DOM实测200×316，来源预览2条，见local-final-metrics.json |
| 多选框与整组拖动 | CSS content-box/40world/dashed | 实页按60.0822%生成，拖动40屏幕px两节点和框同移；撤销还原 |
| 刷新与新标签页 | 4n6e保持 | 服务器snapshot和新tab均4n6e，所有边ID保持 |

临时时间线及其移动通过正常撤销恢复，最终仍4节点6边。本地混合选择并不证明时间线/主体领域接入全量正确。当前四节点水平排列导致部分既有线在视觉上重叠，计数来自正式snapshot和DOM边身份，不能靠截图数线。

## 4. 真实保存证据

本地六条边（完整端口、用途与binding见JSON）：

- `11c5ce51-9b81-4315-a4d5-3fd65f4f7101`：文本1→文本2
- `60fc33cb-cfa6-4927-8cd9-7d4ef015d8df`：文本1→图片1
- `a6cd0a0a-e962-4abb-a049-b359618d1134`：文本2→图片1
- `4b60f549-dbf9-4ad0-83ad-92446e01fb40`：图片1→文本3
- `c5e43654-e376-4ed0-8f45-4698f8b2761e`：文本1→文本3
- `c789723f-dec7-4a5b-aa5f-f3c5a79148fb`：文本2→文本3

真实WebSocket `yjs_ack` status=accepted，记录epoch、commitSeq、contentRevision、graphHash、payloadHash，移除二进制update，不保存鉴权头。重做完成的commitSeq=11、contentRevision=12、graphHash=`67814c84aecc286b4a450a80e859ee9e3c773c8ef4592e29fe5473ee88bada6d`。刷新后的服务器`yjs_snapshot`同commitSeq/hash、4节点6边；新tab恢复同六个edge ID。这比单纯「已保存」或本地缓存线条提供更强证据。

- [accepted回执](local-yjs-receipts.json)
- [刷新前服务器图](local-server-snapshot-before-reopen.json)
- [刷新后服务器图](local-server-snapshot-after-reload.json)
- [新标签页画面](local-six-edges-new-tab.png)

后续临时时间线/位置验收有新的可撤销提交，因此commitSeq11是这一保存检查点，不冒称最终会话最新序号。

## 5. 修复与验证

本轮修复了：多选只有点击没有拖拽、来源未冻结、锚点取首项、自由多源预览缺失、成员入口常显、重复关系被部分跳过、图准入拆为节点/边两次、聚合拖动后焦点丢失、框与菜单布局不符。

- 8个专项文件46测试通过，覆盖gesture、selection快照、整图准入、batch重复、handle、body吸引、创建焦点：[targeted-tests.log](targeted-tests.log)。
- 6个关联文件80测试通过，含流星模型/渲染生命周期、连接状态、Yjs会话、主体连接：[related-regression.log](related-regression.log)。合计14文件126项。
- 最终8个实施TS/TSX文件ESLint通过；Prettier定向检查通过。`git diff --check`见最终检查。
- 共享工作区全量suite实际执行：242文件，1634通过/4失败。随后修正本轮颜色token违规并复查4个失败文件：54通过/2失败；助手/生成器用例复查通过，剩余theme-policy的3个director文件与home-creation Agent入口断言。详见[全量记录](full-suite.log)、[复查](failure-recheck.log)，不将复查拼成未实际执行的全量通过。
- 类型检查本轮较早通过；最终因并行`generation/agent-workspace.tsx:159`的current可能为null失败。本次修改无对应诊断，但全量类型检查不能记通过，见[typecheck.log](typecheck.log)。
- 生产构建通过仅针对已提交入口候选；后续多选修改没有单独生产构建成功证据。

## 6. 完全一致尚未关闭的差异

| 项目 | 当前结论 | 后续接受证据 |
| --- | --- | --- |
| 多选工具条/标签几何、节点resize affordance | 本轮未改为完全一致 | 同状态源DOM尺寸、位置、显隐及相应实页对照 |
| Figma dashed | 本地实现用浏览器CSS dashed；Figma静态段长只是表达 | 不能称Figma逐像素相同 |
| 创建菜单语义、图标路径 | 复用shadcn Command/Popover，字体与geometry已测；图标采用既有图标库 | 如要求图标路径/ARIA结构也相同，需要专门对齐；目前不是逐SVG等价 |
| 主题/减少动态效果 | 暗色聚合实测通过；项目保留reduce策略 | 浅色与reduce分别验收并记录已接受差异 |
| 高级类型和模型容量 | 本轮验证文本/空图片、混合timeline拒绝；既有项目领域规则保留 | 非空音视频/主体/时间线/工具/导演台所有合法与失败组合逐项验收 |
| 保存异常与协作 | 正常accepted→刷新→重开通过 | 离线、未知结果、权限撤销、跨用户并发撤销另有专项合同，本轮未完整重放 |

因此本轮可接受「多选连接主路径与正常真实保存已通过」，不能宣称整个无限画布或所有类型/异常路径与即梦全量一致。
