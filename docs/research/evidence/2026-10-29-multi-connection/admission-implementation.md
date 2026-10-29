# 本轮最小修复记录

2026-10-29，基线25a9f4d6。只读审计见multi-connection-code-audit.md；以下为父任务随后明确授权的实施增量。未stage/commit，保留共享工作区。

## 增量

- 新 `frontend/src/components/canvas/lib/canvas-graph-commit.ts`：完整nodes+connections只调用一次admit；拒绝不调用commit，接受只提交一次完整result。
- `editor/canvas-editor.tsx`：applyConnectionGraph在接受后同步更新nodesStateRef/connectionsStateRef、nodesRef/connectionsRef，再批量setNodesState/setConnectionsState。单源connectDocumentNodes、多源createConnectedNode改用该边界。拒绝有提示，且不应用部分图。
- `lib/canvas-document-connections.ts`：仅connectCanvasDocumentSources改为拒绝重复sourceIDs；任一已有fromNodeId→toNodeId关系整批失败。保留单源connectCanvasDocument的幂等路径。
- 新 `tests/unit/components/canvas/canvas-graph-commit.test.ts` 三项；`multi-connections.test.ts`增加两项重复相关回归。

父任务已接管editor多源drag接线，我不再编辑或格式化editor。

## Red证据

- graph-commit测试创建在模块实现之前，首次失败为模块不存在，不能冒充原组件运行时失败。
- 另用 `/tmp/canvas-graph-admission-red.mjs`忠实复现旧setter顺序，以同形字节上限谓词验证：完整候选不接受，但可见图从1节点变为2节点、边仍0；原引用不变断言失败。日志`/tmp/canvas-graph-admission-red.log`。这是抽取边界模型，不是浏览器渲染测试。
- 两个batch重复回归在修改原helper前真实运行：2失败2通过；失败均为期望抛错而未抛错。对应会话90104。

## Green与限制

- graph commit + multi-connections + subject-connections + cinema-kit：4文件18测试通过（会话87955）。
- 现有Yjs添加引用/标签分配后undo保护：1文件4项通过，58项过滤未执行（会话40697）。此处已有用例只有单来源，不能称多来源正式持久化验收。
- 5个变更TS/TSX文件ESLint、Prettier通过；git diff --check通过。
- tsc本轮失败：并行 `src/app/api/[...path]/route.ts:701` 的 `query` 联合类型字段诊断。未修改该文件；本轮模块无类型诊断，不把整次tsc记为通过。
- 未运行本轮完整生产构建/全量测试/服务端提交；真实多源保存验收由父任务负责。

## 源合同更新

审计正文记录的是修复前状态。随后来源审计与父任务实测确认：batch里重复来源或已有关系也使整批不可用，重复拖到已有target显示无法连接且边数不变。本轮据此改变batch helper；正文的“Set静默去重/重复幂等”描述对修复后的batch已不再适用，仍适用于未改的单源底层函数。
