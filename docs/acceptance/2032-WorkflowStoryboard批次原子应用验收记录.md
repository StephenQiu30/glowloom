# Workflow Storyboard 批次原子应用验收记录

- 状态：`human.storyboard_review` 的 Storyboard Owner 原子 Apply、Owner Receipt、Workflow Apply Receipt、Temporal Signal 与正式输出闭环通过
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow Storyboard 多集持久候选](2031-WorkflowStoryboard多集持久候选验收记录.md)

## 验收范围

本记录验收 `storyboard_candidate → 逐镜 Decide → 逐集 Approve → Storyboard ApplySet → Command Receipt → Workflow Apply Receipt → Temporal Signal → storyboards`。Storyboard Backend Owner 是 Set、Batch 和正式 Shot 的唯一写入者；Review 只持久 HumanTask/ReviewDecision，Workflow 只编排已提交证据，Agent 仍只产生候选。

PostgreSQL/GORM 是唯一 SQL 事实源，Temporal 是唯一跨步骤工作流引擎。本切片没有 Migration、手写 SQL Schema、第二 ORM、兼容分支或 Agent 写库路径。

## 实现证据

| 契约 | 结果 |
|---|---|
| 审核前置 | 任一 Batch 未 approved，或任一候选 Shot 未 accepted，`ApplySet` 均拒绝且不创建正式 Shot |
| 冻结 baseline | Draft Set 创建时记录各 Episode 当前正式 Shot Order Hash；Apply 前重新计算，任一集漂移即拒绝 |
| 多集原子性 | 真实 PostgreSQL 用例在最后一集制造 baseline 漂移；Owner 已处理第一集后事务整体回滚，第一 Batch 仍为 approved、正式 Shot 数仍为 0、Set 仍为 needs_review revision 2 |
| 正式应用 | 无漂移时，一个 GORM 事务锁定 Set、全部 Episode 与 Batch，归档旧 Shot、创建完整新 Shot 批次，并把 Set/Batch 置为 applied |
| 唯一 Owner 证据 | `storyboard.apply_set` Command Receipt 与 Set/Shot 在同一事务提交；使用 `workflow-review:<ReviewDecisionID>` 稳定幂等键可重放同一 Receipt |
| 候选/正式边界 | Candidate Hash 只代表 Draft Set 候选；Gate 输出同一 Set 的 applied revision 3 与正式 Shot 集合 Hash，两个 Hash 必须不同 |
| 故障恢复 | 旅程先提交 Owner Receipt 再调用 Signal Service；Apply/Signal 只重放同一 Owner Receipt，数据库中仅有 1 个 `storyboard.apply_set` Receipt |
| 证据链 | GORM Signal Store 精确校验 `gate.storyboard_review → storyboard.apply_set`，Workflow Apply Receipt 引用同一 Owner Receipt，Temporal 重复 Signal 返回同一 completed Intent |

## Red → Green 与真实验证

1. Red：真实 Workflow 旅程在尚无 `ApplySet` 与 Storyboard Human Gate Owner 时编译失败，证明原实现只停在 `storyboard_candidate` 人工等待。
2. Green：Storyboard Owner 增加批次原子 Apply；Human Gate Applier 只调用 Owner；Workflow 领域契约明确区分候选 Hash 与 applied 正式 Hash；GORM Signal Store 继续在 Signal 前复核证据。
3. 定向真实 Temporal 旅程 `TestProductionWorkflowWorkerCreatesStoryboardDraftSetForEveryConfirmedEpisode` 通过，用时 `13.395s`；包含两集 Set、未批准拒绝、最后一集 baseline 漂移整批回滚、正式应用、Receipt 预提交恢复与重复 Signal。
4. 在全新 `postgres:16.15-alpine` 与固定摘要 `temporalio/temporal` 上执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`：Backend 全部包通过，真实外部依赖测试未跳过。
5. Agent 通过 Ruff check/format、Pyright 与 12 项 Pytest；Frontend 通过锁定安装、OpenAPI 生成、ESLint、TypeScript、16 个 Vitest 文件/45 项测试与 Next.js 生产构建。
6. OpenAPI 生成目录无漂移，开发/生产 Compose 均可渲染，Delivery Hygiene、`git diff --check`、Backend 镜像与 API/Workflow Worker 双二进制门禁通过。

## 残余风险与下一切片

- `production.storyboard_export`、单 Shot 局部重跑与公共 Workflow HTTP 组合仍待后续切片，本记录不提前宣称这些能力完成。
- `agent-browser` 依用户约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 远端 `main` 的 CI 仍对应未推送的旧代码；本地现行 CI 门禁已通过，但未获准推送，因此不宣称远端 CI 已恢复绿色。
