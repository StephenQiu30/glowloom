# Workflow 节点输出绑定验收记录

- 状态：`node-output-v1` 与 NodeRun 原子输出投影切片通过；上游输入传播与 Runtime Cache 命中尚未接入
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 节点缓存确定性事实验收记录](2017-Workflow节点缓存确定性事实验收记录.md)

## 验收范围

本记录验收 `NodeExecutorResult → node-output-v1 → Runtime canonical Output Hash → Claim-fenced NodeRunProjection → Activity Replay` 的最小闭环。输出只引用 Backend Owner 已提交的事实，不让 Agent、Temporal 或缓存成为业务事实 Writer；PostgreSQL/GORM 仍是唯一 SQL 事实源，没有 Migration 文件或手写 SQL。

本切片不解析 Graph 上游输入、不启用 Runtime Cache 命中，也不伪造 Human Gate 的 Owner 输出。Executor 尚未接入生产 Composition Root，因此本记录不宣称正式 Worker 已可执行完整制作剧集。

## 实现证据

| 契约 | 结果 |
|---|---|
| 固定输出格式 | `node-output-v1` 的 Binding 明确 Port、Value Type、Owner Reference ID/Version 与 Content Hash |
| 确定性 | Binding 按 Port 排序；重复 Port、未知字段、非法 UUID/Version/Hash 和空 Binding 集合均拒绝；规范化结果使用 SHA-256 |
| 职责边界 | Executor 只能返回 `SUCCEEDED` / `SKIPPED` 与结构化输出；`CACHED` 只能由后续 Runtime Cache 命中产生 |
| 失败路径 | Executor 报错、状态非法或输出非法都会释放当前 Claim 并进入 `RETRYING`，不会写入 Output/Hash |
| 原子投影 | 非人工节点完成时，Status、Output、Output Hash、Claim Token 清除和 Revision 在同一 GORM 事务中提交 |
| 完成重放 | 已完成 Node 的 Activity Replay 从 NodeRun 读取并重新验证 Output/Hash，不再次调用 Executor |
| Workflow 门禁 | Temporal Episode Workflow 拒绝缺少 canonical Output/Hash 的成功 Activity Result，不继续执行下游节点 |
| 完成约束 | Run 完成前要求每个非人工终态 Node 都存在有效 Output Projection；Human Gate 输出仍等待 Owner Apply Receipt |

## 真实验证

1. Red：新增输出契约测试时因 `NodeOutputSnapshot` / `BuildNodeOutput` / `ParseNodeOutput` 不存在而编译失败；新增 Runtime 输出测试时因 Activity Result 无 Output/Hash、Executor 与 Repository 接口未支持输出而编译失败。
2. Green：输出契约、非法 Executor 结果、Retry、完成重放与 Temporal 无输出拒绝测试通过。
3. 全新 PostgreSQL 16.15 执行 `LANVERSE_TEST_DATABASE_URL=... go test ./tests/workflow -run '^(TestRuntimePlan|TestNodeCache)' -count=1 -v`：Node Cache 与真实 Runtime Plan/NodeRun 输出投影测试通过，用时 2.590 秒。
4. 首轮完整 Backend CI 正确拦截测试夹具直接导入 `gorm.io/datatypes` 的架构越界；移除测试层 GORM 依赖后，数据库架构边界测试通过，没有用白名单或兼容分支绕过。
5. 全新 PostgreSQL 16.15 与固定摘要 Temporal Server 1.31.2 上执行 Backend 门禁：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过，其中 `backend/tests/workflow` 用时 22.634 秒，包含真实 Temporal Replay 与跨进程 Worker 重启恢复。
6. Agent 门禁通过：Ruff 检查与格式、Pyright 均无错误，Pytest 12 项通过。
7. Frontend 门禁通过：OpenAPI 客户端生成、ESLint、TypeScript、Vitest 45 项与 Next.js 生产构建全部通过；生成后 `git diff --exit-code -- frontend/src/api` 无漂移。
8. Delivery Hygiene 与 `git diff --check` 通过；未追踪凭据、数据、日志或浏览器报告，Backend/Agent 语言与 ORM 边界符合 CI 契约。

## Requirement 状态

- `BE-MOD-005`：Node Executor 输出与 NodeRun 原子投影已完成；上游 Output Binding/Input Hash 传播、Cache Runtime 命中、Human Gate Owner 输出、Shot Workflow 和局部重跑仍未完成。
- `BE-APP-006`：输出引用已携带 Content Hash，但冻结 Policy/Model/Prompt/Skill 的 Owner 与正式 Executor 尚未全部接入。
- `BE-JRN-003`：Episode Workflow 新增 Activity 输出完整性门禁；Start/Control/Signal 身份与状态路径没有变化。

## 残余风险与下一切片

- Human Gate 当前只能提交 Review/Signal/Apply 状态，尚无目标 Owner 产物引用；不能用候选或审核状态替代 Production Owner 事实。
- 后续 `node-input-v1` 切片已按 Graph Edge/Binding 校验上游 Port 与 Value Type，并生成规范化 Node Input/Hash；下一步在 NodeRun Claim fencing 下安全读取/写入 Node Cache。
- 正式 NodeExecutor 与 Worker 生命周期仍需在 Backend Composition Root 接入；Agent 只承担受限候选执行职责，不能直接写 Production/Workflow SQL 事实。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
