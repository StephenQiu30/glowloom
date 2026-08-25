# Workflow 人工栅栏输入与决议绑定验收记录

- 状态：Human Gate 输入冻结、候选绑定与决议防篡改切片通过；Owner 正式输出尚未接入
- 日期：2026-08-26
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 节点运行缓存验收记录](2020-Workflow节点运行缓存验收记录.md)

## 验收范围

本记录验收 `Upstream node-output-v1 → Human Gate node-input-v1 → HumanTask Candidate IDs → immutable ReviewDecision → Signal Preparation` 的最小闭环。Backend Runtime 是 Gate 输入和协调事实的唯一 Writer，Review 是任务与不可变决议的唯一 Owner，PostgreSQL/GORM 是唯一 SQL 事实源；Temporal 仍只负责跨步骤历史。客户端不能自报候选、改写已提交 Decision，Review 状态也不能充当目标 Owner 的正式业务输出。没有兼容字段、Migration、手写 SQL 或第二数据库。

## 实现证据

| 契约 | 结果 |
|---|---|
| Gate 输入 | `PrepareHumanGate` 复用正式 Graph/Port 解析器生成 canonical `node-input-v1`，并在首次进入 `WAITING_HUMAN` 时与 Input Hash、Attempt、Revision 原子提交 |
| 候选来源 | HumanTask Candidate IDs 只从已提交上游 `node-output-v1` 的 Owner Reference ID 排序去重生成，不接受空集合或客户端候选 |
| 重放门禁 | 重复打开同一 Gate 重新解析执行输入，并要求已存 Input Hash 与候选集合完全一致 |
| 决议绑定 | Signal 准备从 PostgreSQL 重读 HumanTask 与 ReviewDecision，并核对 Workspace、Run、Node、Task、Subject、Subject Revision 和 Decision |
| 选择门禁 | `selected` 决议的 Selected Candidate 必须仍在 HumanTask 冻结候选内 |
| 副作用边界 | 决议漂移在创建 Workflow Apply Receipt、Signal Intent 和 Temporal 请求之前被拒绝 |
| 后续输出 | 本切片只证明候选与决议一致；目标 Owner ApplyReceipt 和正式 `node-output-v1` 留给下一切片，不伪报 Gate 已产生业务产物 |

## 真实验证

1. Red：真实 Runtime Plan 测试最初读到 HumanTask `candidate_ids = null`，Gate NodeRun 也没有 Input/Input Hash；修改调用方 Decision 后仍可能进入 Signal 准备，证明原实现没有完成候选与不可变决议绑定。
2. Green：针对 Human Gate 与 Runtime Plan 的真实 PostgreSQL/Temporal 测试通过；Gate 冻结的输入引用与 HumanTask 候选都精确指向上游 Bible 输出 Owner Reference ID。
3. 防篡改：把已提交的 `approved` 决议在 Signal 命令中改为 `rejected` 会在事务准备阶段失败，当前 Run 的 Apply Receipt、Signal Intent 和 Temporal 请求均保持为零；随后使用数据库中的真实决议仍可完成 Unknown → Already Applied 对账。
4. 完整 Backend 门禁在全新 PostgreSQL 16.15 与固定摘要 Temporal Server 上从头执行：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过，其中 `backend/tests/workflow` 用时 23.021 秒。
5. Agent 门禁通过：锁定开发依赖安装完成，Ruff 检查与格式、Pyright 均无错误，Pytest 12 项通过。
6. Frontend 门禁通过：`npm ci`、Backend OpenAPI 客户端生成、ESLint、TypeScript、Vitest 45 项与 Next.js 生产构建全部通过。
7. OpenAPI 生成漂移、Delivery Hygiene、数据库架构边界与 `git diff --check` 通过；未引入凭据、数据、日志、浏览器报告、Migration、手写 SQL、第二 ORM 或兼容分支。

## Requirement 状态

- `BE-MOD-005`：Human Gate 已复用正式节点输入解析与投影；目标 Owner 输出、Shot Workflow 与局部重跑仍未完成。
- `BE-MOD-006`：HumanTask 已绑定真实候选，Signal 准备已重新核对不可变 ReviewDecision。
- `BE-JRN-006`：候选与决议绑定完成；修改目标 Owner 的 ApplyReceipt 与正式输出仍待下一切片。

## 残余风险与下一切片

- 当前 Workflow Human Gate Apply Receipt 是协调事实，不是 Production/Generation Owner 的生效回执；下一切片必须由目标 Owner 写入正式 ApplyReceipt 与输出引用。
- 正式 NodeExecutor 与生产 Worker Composition Root 尚未接入，当前不宣称完整剧集制作链路可用。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地门禁通过不等于远端绿色；`origin/main` 最近一次历史运行仍失败，这些本地提交尚未获准推送。
