# Workflow 节点输入冻结验收记录

- 状态：`node-input-v1`、Graph 输入解析与 Claim 原子输入投影切片通过；Runtime Cache 命中尚未接入
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 节点输出绑定验收记录](2018-Workflow节点输出绑定验收记录.md)

## 验收范围

本记录验收 `Committed WorkflowDefinition/RunInputSnapshot → Graph Edge/Variable Resolution → node-input-v1 → Claim-fenced NodeRun Input Projection → NodeExecutor Contract` 的最小闭环。Backend Runtime 是输入解析者；Executor 和 Agent 不得自报或修改输入。PostgreSQL/GORM 仍是唯一 SQL 事实源，没有 Migration 文件、手写 SQL、测试层 GORM 依赖或第二套输入状态。

本切片尚未读取或写入 Node Cache，也不为 Human Gate 伪造 Owner 输出。其结果是 Cache Key 已有可信的 Normalized Input Hash 与上游 Content Hash 来源，但只有下一切片完成原子 Cache Hit/Commit 后才允许出现 `CACHED`。

## 实现证据

| 契约 | 结果 |
|---|---|
| 固定输入格式 | `node-input-v1` 包含 canonical Node Config、按目标 Port 排序的 Binding 与排序去重后的全部 Frozen Reference |
| Edge 解析 | Runtime 只接受同一 Run 中已完成上游 Node 的精确 Output Port；Definition/Version/Executor/Risk、Port 与 Value Type 任一漂移均拒绝 |
| Variable 解析 | Variable Binding 必须同时存在于编译 Graph、目标 Input Port 与 Variables，值按 JSON 语义 canonicalize |
| 上游门禁 | 上游未完成、没有 Output、Output Hash 漂移或缺少必填输入时，目标 Node 保持 `QUEUED`，Executor 调用次数保持零 |
| 冻结范围 | MVP 将 RunInputSnapshot 的全部 Frozen Reference 纳入每个 Node Input Hash，保证正确失效优先于缓存命中率优化 |
| Claim 原子性 | Status、Attempt、Claim Token、Input Snapshot/Hash 与 Revision 在同一 GORM 事务中写入 |
| Retry 稳定性 | Executor 失败后保留 Input Snapshot/Hash；再次 Claim 重新解析并要求 Hash 完全一致，否则拒绝执行 |
| Executor 边界 | Executor 收到 Runtime 生成的 Input/Hash 与编译定义的期望 Output Ports；未知、遗漏或 Value Type 漂移的输出不能提交 |
| 完成约束 | 非人工终态 Node 必须同时存在可重新验证的 Input 与 Output 投影，Run 才能完成 |

## 真实验证

1. Red：新增输入契约测试时因 `NodeInputSnapshot` / `BuildNodeInput` / `ParseNodeInput` 不存在而编译失败；Compiler 测试因 NodeExecution 不含 Port 定义失败；Runtime 测试因 Claim/Executor Command 不含 Input/Hash/Output Ports 失败。
2. Green：输入 canonicalization、顺序无关、上游 Hash 失效、重复/混合来源拒绝和 Output Port/Type 门禁测试通过。
3. 全新 PostgreSQL 16.15 执行真实 Runtime Plan：根节点 Config/Frozen Input 投影、下游 Edge 引用、上游缺失零调用、Retry Input Hash 稳定和完成门禁通过；目标测试复验用时 1.066 秒。
4. Cancel 与 Pause/Resume 的真实 PostgreSQL 测试改为按编译依赖选择根节点，不再依赖相同 CreatedAt 的不确定数据库返回顺序；两条控制路径分别通过。
5. 全新 PostgreSQL 16.15 与固定摘要 Temporal Server 1.31.2 上执行 Backend 门禁：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过，其中 `backend/tests/workflow` 用时 22.616 秒，包含真实 Temporal Replay 与跨进程 Worker 重启恢复。
6. Agent 门禁通过：Ruff 检查与格式、Pyright 均无错误，Pytest 12 项通过。
7. Frontend 门禁通过：OpenAPI 客户端生成、ESLint、TypeScript、Vitest 45 项与 Next.js 生产构建全部通过；生成后 `git diff --exit-code -- frontend/src/api` 无漂移。
8. Delivery Hygiene、数据库架构边界与 `git diff --check` 通过；未追踪凭据、数据、日志或浏览器报告，Backend/Agent/ORM 边界符合 CI 契约。

## Requirement 状态

- `BE-MOD-005`：Node Input 解析与投影已完成；Cache Runtime 命中、Human Gate Owner 输出、Shot Workflow 与局部重跑仍未完成。
- `BE-APP-006`：Frozen References 与上游 Content Hash 已进入 Executor 输入；Policy/Model/Prompt/Skill 的正式 Owner 仍待 Generation/Model 模块落地。
- `BE-JRN-003`：Temporal History 结构未改变；Activity 内部现在从 PostgreSQL 已提交编译事实恢复输入，Replay 仍以相同 Activity Result 为准。

## 残余风险与下一切片

- Human Gate 成功状态仍没有目标 Owner 输出，所以下游 Edge 会被真实输入解析器拒绝；必须先由 Production/Generation Owner 提交 Apply Receipt 与 `node-output-v1` 引用。
- 下一切片使用已验证的 Node Definition、Config Hash、Input Hash、Frozen Hash、上游 Content Hash 与 Runtime Contract 计算 Cache Key，并在 Claim fencing 事务内完成 Cache Hit 或 Cache Fact + Node Output 原子提交。
- 正式 NodeExecutor 与 Worker Composition Root 尚未接入，当前只证明 Runtime 契约和真实数据库事实可落地。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
