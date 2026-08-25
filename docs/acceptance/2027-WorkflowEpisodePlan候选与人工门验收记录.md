# Workflow Episode Plan 候选与人工门验收记录

- 状态：`activity.episode_plan` 候选节点与 `human.episode_plan_review` 打开边界通过；Planning Owner 确认回执及后续物化/发布尚未验收
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow Production Bible 持久等待节点](2026-WorkflowProductionBible持久等待节点验收记录.md)

## 验收范围

本记录验收 `confirmed Production Bible → activity.episode_plan → review_ready Plan → human.episode_plan_review HumanTask` 的 Backend 闭环。分集计划、项目与 Bible 仍由各自 Production Application Service 拥有；Workflow 不读写它们的 Repository 或表。PostgreSQL/GORM 仍是唯一业务 SQL 事实源，Temporal 仍是唯一跨步骤执行引擎。

本切片刻意停在分集人工确认门：没有实现自动确认、Episode 物化或剧集发布，也不把 HumanTask 已打开伪装成业务通过。

## 实现证据

| 契约 | 结果 |
|---|---|
| 不可变目录 | 系统 Node Catalog 从 `1.0.0` 升为 `2.0.0`；修改后的 `production.episode_plan@2.0.0` 输出 `episode_plan_candidate`，原有 Definition Version 不被原地改写 |
| 人工边界 | 新增 `human.episode_plan_review@1.0.0`，输入 Candidate、输出正式 `episode_plan`，风险级别为 `human_gate` 且缓存策略为 `never` |
| Owner 边界 | Executor 只调用 Project `Get`、Production Bible `Get` 与 Planning `CreatePlan` Application Interface；未导入业务 GORM Adapter、Repository 或手写 SQL |
| 冻结输入 | Script Revision 必须命中 Run Frozen Input；Bible 必须为同 Workspace/Project/Document Revision 的精确 confirmed revision/result hash；项目目标时长由 Project Owner 重新鉴权读取 |
| 稳定副作用 | NodeRun 幂等键直接作为 `episode_plan.create` Command Key；重复执行返回同一 Plan、同一 Output Binding 和同一 Command Receipt |
| 候选语义 | 仅接受 `review_ready`、revision 1、期望集数/目标时长/来源一致的 Plan，并输出 `episode_plan_candidate`；Bible 回退到 `needs_review` 后拒绝新建 Plan |
| 零越权发布 | 候选完成并打开 HumanTask 后，项目下 Episode 和 ImportCommit 数均为 0；确认、物化、发布不在 Candidate Activity 内发生 |
| 部署装配 | `workflow-worker` 组合根显式注入 Project 与 Planning Service，仍复用同一 Backend GORM Catalog、数据库连接和容器镜像 |

## Red → Green 与真实验证

1. Red：目录合同先失败为系统目录仍是 `1.0.0` 且缺少 `human.episode_plan_review`；Executor 合同先因 `NewNodeExecutor` 尚无 Project/Planning Owner 而编译失败。
2. Green：目录升级为独立 v2，Episode Plan Executor 通过正式 Application Interface 产出唯一 Candidate；真实 PostgreSQL 定向测试证明重复调用只留下 1 个 Plan 和 1 个 `episode_plan.create` Receipt，且无 Episode/ImportCommit。
3. 真实 Temporal 旅程通过，用时 `6.94s`：Script → Production Bible 持久等待 → Bible HumanTask/Owner Confirm Receipt/Signal → Episode Plan Candidate → Episode Plan HumanTask；最终仍为 0 Episode、0 ImportCommit。Bible Agent 在该集成测试中使用符合正式 `AgentClient` 契约的确定性替身，不宣称真实模型运行。
4. 第一次 Backend 全量门禁真实发现两类失败：测试文件直接导入 GORM，违反数据库架构门禁；旧 Workflow 断言仍按 9 节点计数。修正时删除测试层 GORM 依赖、按项目隔离事实计数，并把目录顺序更新为 10 节点，没有放宽架构测试或增加兼容分支。
5. 在重建的空 PostgreSQL 16.15 和固定摘要 Temporal 上，`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过；Workflow 包用时 `44.048s`。
6. Agent 通过 Ruff check/format、Pyright 与 12 项 Pytest。Frontend 在官方 Node `22.23.2` 下通过锁定安装、OpenAPI 生成、ESLint、TypeScript、45 项 Vitest 与 Next.js 生产构建。
7. 开发/生产 Compose 渲染、Backend 镜像构建、API/Workflow Worker 双二进制、OpenAPI 漂移、Delivery Hygiene 与 `git diff --check` 均通过。

## Requirement Checklist

- `BE-MOD-005`：Episode Plan 节点由冻结执行身份调用真实 Owner，产出 canonical Candidate Output 并进入现有 Runtime/Cache 投影。
- `BE-MOD-006`：Episode Plan Candidate 已绑定真实 HumanTask；本切片不宣称 Planning Owner Apply Receipt 已实现。
- `BE-JRN-003`：候选生成与人工确认已拆开，确认前没有 Episode/发布副作用。
- `BE-JRN-006`：Production Bible Gate 的 Owner Receipt/Signal 在本旅程中真实完成；Episode Plan Gate 只验收到 HumanTask 打开。

## 残余风险与下一切片

- `human.episode_plan_review` 尚不能由当前 Production Bible 专用 Owner Applier 确认；下一切片必须通过 Planning Application 返回真实 `episode_plan.confirm` Receipt，不能在 Workflow 内直接改 Plan。
- Episode 物化/发布、结构候选、结构人工确认、Storyboard 与 Export Executor 尚未接入。
- Workflow Start/Query/Control 的 Backend HTTP 组合边界尚未完成，浏览器仍不能启动本流程。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地 Required CI 通过不等于远端绿色；未获准推送前，不宣称远端 CI 已修复。
