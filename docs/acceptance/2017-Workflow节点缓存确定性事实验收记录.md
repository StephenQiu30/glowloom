# Workflow 节点缓存确定性事实验收记录

- 状态：Node Cache Key 与不可变 GORM 事实切片通过；Runtime 命中与输出绑定尚未接入
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 工作者重启恢复验收记录](2016-Workflow工作者重启恢复验收记录.md)

## 验收范围

本记录验收 `Frozen Execution Inputs → NodeCacheKeyMaterial → deterministic Cache Key → canonical Output Snapshot/Hash → Workspace-scoped NodeCacheEntry` 的最小事实闭环。它只保存已经完成的不可变输出，不新增 Pending/Building 状态、Lease、内存缓存、第二数据库、第二 ORM 或缓存调度器。

本切片明确不把“缓存事实已存在”报告为“Runtime 已命中”。当前 Node Executor 尚未返回正式输出绑定，Temporal Workflow 也尚未把上游 Output Hash 传播到下游输入，因此 Runtime 不能安全跳过节点或将 NodeRun 标为 `CACHED`。

## 实现证据

| 契约 | 结果 |
|---|---|
| 确定性 Key | `node-cache-key-v1` 对规范化 Key Material 使用稳定 JSON 与 SHA-256；Artifact Hash 排序去重，不受调用顺序影响 |
| 完整输入覆盖 | Node Definition、Config、规范化输入、冻结 Policy/Model/Prompt/Skill、输入 Artifact Hash 和 Runtime Contract Version 任一变化都会改变 Cache Key |
| 输出快照 | 输出使用后续固定的 `node-output-v1` typed port bindings，并单独保存 Output Hash；重复端口、未知字段、非法引用和 Hash 漂移均拒绝 |
| 单一事实源 | `wrk_node_cache_entries` 进入现有 GORM Schema Catalog，PostgreSQL 是唯一 SQL 事实源；没有 Migration 文件或手写 SQL |
| 并发收敛 | Workspace + Cache Key 唯一；八路并发首次 Ensure 通过 GORM Conflict Do Nothing 收敛为一条事实 |
| 不可变冲突 | 同 Workspace/Cache Key 的不同 Output Snapshot/Hash 返回冲突，不覆盖首个已提交事实 |
| 租户隔离 | 相同 Cache Key 在不同 Workspace 可有各自输出；Find 强制同时匹配 Workspace 与 Key，不跨租户复用 |
| JSONB 语义 | 持久化比较重新计算确定性 Key 与 canonical Output Hash，不依赖 PostgreSQL `jsonb` 的原始字节顺序 |

## 真实验证

1. 全新 PostgreSQL 16.15 执行 `LANVERSE_TEST_DATABASE_URL=... go test ./tests/workflow -run '^TestNodeCache' -count=1 -v`：三个契约/持久化测试通过，用时 2.252 秒；同一全新实例追加 `-count=3` 稳定性复验通过，用时 5.518 秒。
2. 全新 PostgreSQL 16.15 与固定摘要 Temporal Server 1.31.2 上执行 Backend 门禁：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过，其中 `backend/tests/workflow` 用时 22.394 秒。
3. Agent 门禁通过：Ruff 检查与格式、Pyright 均无错误，Pytest 12 项通过。
4. Frontend 门禁通过：OpenAPI 客户端生成、ESLint、TypeScript、Vitest 45 项与 Next.js 生产构建全部通过；生成后 `git diff --exit-code -- frontend/src/api` 无漂移。
5. Delivery Hygiene 与 `git diff --check` 通过；未追踪凭据、数据、日志或浏览器报告，Backend/Agent 语言边界符合 CI 契约。

## Requirement 状态

- `BE-MOD-005`：Node Cache Key 和不可变 SQL 事实已完成；Runtime 命中、NodeRun 输出投影、上游 Output Hash 传播、Shot Workflow、局部重跑与公共查询仍未完成，因此主需求保持未完成。
- `BE-APP-006`：Cache Key 已为冻结 Policy/Model/Prompt/Skill 与输入 Artifact Hash 提供明确位置；这些 Snapshot 的 Owner 尚未全部实现，因此不宣称完整运行冻结已完成。
- `BE-JRN-003`：本切片不改变 Start/Control/Signal 或 Temporal History，Journey 状态保持不变。

## 残余风险与下一切片

- Source WorkflowRun/NodeRun 是来源审计标识，不作为缓存所有权；缓存可复用性的权威是完整 Key Material、canonical Output Snapshot 和 Output Hash。
- 下一切片必须让 Node Executor 返回规范化输出绑定，并让 Temporal Workflow 使用上游 Output Hash 计算当前节点输入 Hash；完成 GORM 原子命中/写入后，才允许 NodeRun 进入 `CACHED`。
- 缓存只复用已提交输出；并发运行中的高成本调用仍必须由目标 Owner 的稳定幂等键、Intent/Receipt 和 Execution Claim 防重，不能让 Node Cache 替代 Generation/Agent/Media 的执行协议。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
