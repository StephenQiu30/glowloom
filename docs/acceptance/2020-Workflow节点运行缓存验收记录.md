# Workflow 节点运行缓存验收记录

- 状态：Node Runtime Cache 原子命中与写入切片通过；远端 CI 等待获准推送后运行
- 日期：2026-08-26
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 节点输入冻结验收记录](2019-Workflow节点输入冻结验收记录.md)

## 验收范围

本记录验收 `Frozen Node Execution → node-cache-key-v1 → Claim-fenced Cache Lookup → Executor Miss / Runtime Hit → NodeCacheEntry + NodeRun Output` 的最小闭环。Backend Runtime 是唯一缓存决策者，PostgreSQL/GORM 是唯一 SQL 事实源；Temporal 仍只负责跨步骤历史，Agent/Executor 不得声明 `CACHED` 或写入缓存事实。没有 Migration 文件、手写 SQL、Redis、Pending Cache、Lease、兼容分支或第二数据库。

## 实现证据

| 契约 | 结果 |
|---|---|
| Key 来源 | Runtime 从已冻结的 Node Definition、canonical Config/Input、全部 Frozen Reference、上游 Content Hash 与 Runtime Contract 构造完整 Key Material |
| 策略门禁 | 只有编译策略 `by_inputs` 可生成和使用 Cache Key；`never` 不查询、不写入缓存 |
| 命中原子性 | 缓存 Output/Hash 重新校验端口和类型后，在 Claim Token/Revision 围栏内与 `CACHED` 状态、Claim 清除同一 GORM 事务提交 |
| 未命中原子性 | Executor 只能返回 `SUCCEEDED`/`SKIPPED`；缓存事实、canonical Node Output/Hash 与节点终态在同一 GORM 事务提交，冲突时整体回滚 |
| 复用边界 | 查询使用 `workspace_id + cache_key`；相同 Key 只能对应相同 canonical Output，不跨 Workspace 复用 |
| 重放 | 已完成的 `SUCCEEDED`/`SKIPPED`/`CACHED` 节点从 NodeRun 投影恢复原始 Output/Hash，不再次调用 Executor |
| 单一 Schema | NodeRun 的 Cache Key 与 NodeCacheEntry 都进入现有 GORM Schema Catalog；没有迁移文件或手写 SQL |

## 真实验证

1. Red：应用层缓存命中测试最初因 Claim 缺少 Workspace/Cache Policy/Key Material/Cache Key，以及 Repository 缺少原子命中/写入接口而编译失败；Cache Material 测试固定了 Config、Frozen 与上游 Hash 的失效契约。
2. Green：`go test ./tests/workflow -run '^(TestRuntimeNode|TestNodeCache)' -count=1` 通过，用时 0.662 秒；命中时 Executor 零调用，未命中时生成缓存事实并提交同一 Output Hash。
3. 全新 PostgreSQL 16.15 的真实 Runtime Plan 启动两个相同 Authoring Revision 的独立 Run：首次 Bible 节点写入唯一 Workspace Cache Fact，第二次返回 `CACHED`，Executor 调用数从执行上游后的 4 次不再增加，两个 NodeRun 的 Cache Key 与 Output Hash 完全一致。
4. 完整 Backend 门禁首轮正确拦截了测试对共享数据库做全表计数的错误断言；改为真实业务隔离键 `workspace_id + cache_key` 后，在全新 PostgreSQL 16.15 与固定摘要 Temporal Server 上再次从头执行 `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过，其中 `backend/tests/workflow` 用时 22.793 秒。
5. Agent 门禁通过：Ruff 检查与格式、Pyright 均无错误，Pytest 12 项通过。
6. Frontend 门禁通过：OpenAPI 客户端生成、ESLint、TypeScript、Vitest 45 项与 Next.js 生产构建全部通过。
7. OpenAPI 生成漂移、Delivery Hygiene、数据库架构边界与 `git diff --check` 通过；未引入凭据、数据、日志、浏览器报告、Migration 或第二 ORM。

## Requirement 状态

- `BE-MOD-005`：Node Runtime Cache 命中/写入完成；Human Gate Owner 输出、Shot Workflow 与局部重跑仍未完成。
- `BE-APP-006`：现有全部 Frozen Reference 已进入 Key；正式 Policy/Model/Prompt/Skill Owner 就绪后仍沿用同一引用契约。
- `BE-JRN-003`：Temporal History 结构未改变；Activity Result 的 `CACHED` 只来自 Backend 已提交事实并可重放。

## 残余风险与下一切片

- Node Cache 只复用已完成输出，不承担运行中高成本调用防重；Generation/Agent/Media Owner 后续仍必须使用稳定 Intent/Receipt 与 Execution Claim。
- Human Gate 仍没有目标 Owner 的正式 `node-output-v1`，下游节点会按真实输入门禁保持不可执行；下一切片需提交 Owner Apply Receipt 与输出引用。
- 正式 NodeExecutor 与生产 Worker Composition Root 尚未接入，当前不宣称完整剧集制作链路可用。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地当前提交已按与仓库 CI 等价的门禁通过；`origin/main` 最近一次历史运行仍失败，因为这些本地提交尚未获准推送，不能报告为远端绿色。
