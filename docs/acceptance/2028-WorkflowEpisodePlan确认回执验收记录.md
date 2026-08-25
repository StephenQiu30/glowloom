# Workflow Episode Plan 确认回执验收记录

- 状态：`human.episode_plan_review` 的 Planning Owner 确认、Apply Receipt、Temporal Signal 与正式输出闭环通过；Episode 物化/发布尚未验收
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow Episode Plan 候选与人工门](2027-WorkflowEpisodePlan候选与人工门验收记录.md)

## 验收范围

本记录验收 `episode_plan_candidate → HumanTask/ReviewDecision → Planning ConfirmPlan → Command Receipt → Apply Receipt → Temporal Signal → episode_plan` 的 Backend 内部闭环。Planning 仍是 Plan 的唯一 Owner；Workflow 只协调已提交证据和运行投影，不直接写分集计划表。PostgreSQL/GORM 仍是唯一 SQL 业务事实源，Temporal 仍是唯一跨步骤工作流引擎。

本切片只把 Plan 确认为正式输入：不创建 Episode、不创建 ImportCommit、不物化或发布剧集剧本。

## 实现证据

| 契约 | 结果 |
|---|---|
| Owner 回执 | Planning `ConfirmPlan` 新增专用结果，首次确认和幂等重放都返回同事务持久化的 `episode_plan.confirm` Command Receipt |
| 精确路由 | Human Gate Owner Applier 只接受 `gate.episode_plan_review`、`episode_plan_candidate → episode_plan` 契约，并调用正式 Planning Application Interface |
| 稳定身份 | Owner 幂等键由 ReviewDecision ID 派生；重复 Signal 复用同一 Owner Receipt、Apply Receipt、Signal Intent 和正式 Plan Revision |
| 正式输出 | Plan 从 `review_ready/revision 1` 变为 `confirmed/revision 2`，提案全部锁定；Gate 输出保持同一 Plan ID 与 Input Hash，类型为正式 `episode_plan` |
| 证据校验 | GORM Signal Store 按 Gate Executor 精确匹配 `episode_plan.confirm`，并核对 Receipt Workspace、Resource、Creator 与 canonical Output |
| 零越权副作用 | Plan 确认和 Workflow 完成后，项目下 Episode 与 ImportCommit 仍均为 0；物化/发布没有被藏入 Human Gate |
| 单一事实源 | 未增加 Model、Migration、手写 SQL、第二 ORM 或 Agent 数据库写入；所有新增事实仍写入既有 GORM Model Catalog |

## Red → Green 与真实验证

1. Red：先扩展真实 Workflow 旅程，编译失败为 Human Gate Owner 构造器只接受 Bible Owner，证明 `human.episode_plan_review` 无法调用 Planning Owner。
2. Green：Planning 确认返回真实 Receipt；Owner Applier 与 GORM 证据校验按 Executor 支持 Episode Plan；HTTP 原有响应仍只呈现 Plan View，不暴露内部 Receipt。
3. PostgreSQL 16.15 与固定摘要 Temporal 的定向旅程执行 `TestProductionWorkflowWorkerConfirmsEpisodePlanThroughIndependentHumanGate`：通过，最终复验用时 `8.135s`。真实路径依次完成 Script、Production Bible 持久等待/审核、Episode Plan Candidate/审核、两次 Owner Receipt/Signal，最终 Run 为 `SUCCEEDED`。
4. 同一旅程再次提交 Episode Plan Signal，返回原 completed Intent；数据库只有一个确认 Receipt，Plan Revision 保持 2，Episode 与 ImportCommit 数仍为 0。
5. 重建全新 PostgreSQL 16.15 与固定摘要 Temporal 后执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`：全部通过；`backend/tests/workflow` 用时 `47.089s`，真实外部依赖测试未跳过。
6. Agent 通过 Ruff check/format、Pyright 与 12 项 Pytest；Frontend 在官方 Node `22.23.2` 下通过锁定安装、OpenAPI 生成、ESLint、TypeScript、45 项 Vitest 与 Next.js 生产构建。
7. OpenAPI 生成目录无漂移，开发/生产 Compose 渲染、Backend 镜像构建、API/Workflow Worker 双二进制与 Delivery Hygiene 均通过。

## Requirement Checklist

- `BE-MOD-006`：分集 ReviewDecision 已经由 Planning Owner Receipt 生效，Workflow Apply Receipt、Signal 和 Gate Output 使用同一证据。
- `BE-JRN-003`：分集候选和正式确认已拆开；确认后输出正式 Plan，确认阶段不越权物化/发布。
- `BE-JRN-006`：第二类 Production Human Gate 已通过正式 Owner 路由，证明信号协调不是 Bible 专用伪实现。

## 残余风险与下一切片

- confirmed Plan 尚未进入 Episode 物化/发布节点；下一切片必须通过 Planning Owner 的 Materialize/Publish 命令生成正式 Episode/ImportCommit，不能由 Workflow 直接写表。
- 结构候选、结构人工确认、Storyboard、Export、公共 Workflow HTTP 组合与浏览器旅程仍待后续切片。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地 Required CI 通过不等于远端绿色；未获准推送前，不宣称远端 CI 已修复。
