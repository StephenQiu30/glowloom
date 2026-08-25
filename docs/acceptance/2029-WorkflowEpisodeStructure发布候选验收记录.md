# Workflow Episode Structure 发布候选验收记录

- 状态：`activity.episode_structure` 的 Episode 物化、发布、故障恢复与结构批次候选闭环通过；`human.episode_structure_review` 确认尚未验收
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow Episode Plan 确认回执](2028-WorkflowEpisodePlan确认回执验收记录.md)

## 验收范围

本记录验收 `episode_plan → activity.episode_structure → Planning Materialize/Publish → Episode/Script Version/ImportCommit/Structure → episode_structure_candidate → human.episode_structure_review` 的 Backend 内部闭环。Planning 仍是全部分集生产事实的唯一 Owner；Workflow 只调用公开 Application Interface、保存节点输出并协调 Temporal，未直接读写 Planning Repository 或表。

本切片只发布 `needs_review` 的结构候选并打开独立批次审核，不接受制作任务、不确认 Structure，也不生成 Storyboard 或正式 Shot。PostgreSQL/GORM 仍是唯一业务 SQL 事实源，Temporal 仍是唯一跨步骤工作流引擎。

## 实现证据

| 契约 | 结果 |
|---|---|
| 正式输入 | Executor 只接受 `human.episode_plan_review` 输出的 `episode_plan`，并核对 frozen Script Revision、运行身份、Workspace、Project、Plan 创建者、revision 与 Input Hash |
| Owner 边界 | Episode、Script Version、ImportCommit 与 Structure 均由 Planning Application 的既有 `Materialize` / `Publish` 命令创建；Workflow 没有数据库旁路 |
| 两阶段回执 | Materialize 与 Publish 使用由 NodeRun 幂等键派生的两个稳定 Command Key，各留下且只留下一个 Owner Receipt |
| 中断恢复 | Plan 的唯一 ImportCommit 可由 GORM Repository 查询；若物化已提交而发布未执行，重试直接继续原 Commit，不使用变化后的 Project revision 重建 Episode |
| 发布事实 | confirmed Plan revision 2 物化后为 revision 3；ImportCommit 为 `published/revision 2`，Episode 为 `active/revision 2`，Script Version 为 `published`，Structure 为 `needs_review/revision 1` |
| 批次候选 | 按 ImportCommit Segment 顺序对 Structure、Episode、正式 Script Version 与 Result Hash 的规范引用计算 Content Hash，只输出一个指向 ImportCommit 的 `episode_structure_candidate` |
| 人工边界 | Temporal 在候选完成后打开 `human.episode_structure_review`，HumanTask Candidate ID 与 ImportCommit ID 一致；节点不自动确认任何 Structure |
| 单一事实源 | 未增加 Model、Migration、迁移字段、手写 SQL、第二 ORM、第二数据库连接或 Agent Writer，复用既有 GORM Model Catalog 与 Planning 聚合 |

## Red → Green 与真实验证

1. Red：先扩展真实 Worker 旅程，编译失败为 Planning Service 尚无已发布结构批次查询；补齐实现后，真实 PostgreSQL 首次暴露“Commit 不存在”被归一为 HTTP 风格 not-found、无法安全区分查询缺失的问题。
2. Green：Repository 增加唯一 Plan→ImportCommit 的 GORM 查询，Application 以显式 `found` 契约表达缺失；Executor 实现正式输入校验、Materialize/Publish、提交窗口恢复和 canonical 批次输出，没有增加状态字段或兼容分支。
3. 全新 PostgreSQL 16.15 与固定摘要 Temporal 定向执行 `TestProductionWorkflowWorkerPublishesEpisodeStructuresAfterPlanReview` 和 `TestProductionEpisodePlanCandidateAndStructurePublishRecovery`：两项通过，分别用时 `5.85s` 与 `1.88s`，包总用时 `8.190s`。
4. 第一项旅程真实完成 Script、Production Bible 等待/审核、Episode Plan 候选/审核、Episode Structure 发布，并停在 Structure Human Gate；第二项在 Materialize 已提交后调用 Executor，证明只新增一个 Publish Receipt，重复执行不新增 Episode、Structure 或 Receipt。
5. 再次重建空 PostgreSQL 16.15 与固定摘要 Temporal，执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`：全部通过；`backend/tests/workflow` 用时 `46.356s`，整组 Backend 门禁用时 `65.88s`，真实外部依赖测试未跳过。
6. Agent 在 Python 3.11 下通过 Ruff check/format、Pyright 与 12 项 Pytest；Frontend 在官方 Node `22.23.2` 下通过锁定安装、OpenAPI 生成、ESLint、TypeScript、45 项 Vitest 与 Next.js 生产构建。
7. OpenAPI 生成目录无漂移，开发/生产 Compose 渲染、Delivery Hygiene、`git diff --check`、Backend 镜像构建及 API/Workflow Worker 双二进制检查均通过。

## Requirement Checklist

- `BE-MOD-005`：Episode Structure 节点使用冻结身份与 canonical 输入/输出接入真实 Planning Owner，并进入现有 NodeRun、Cache 与 Temporal Runtime。
- `BE-JRN-003`：confirmed Plan 已通过两个可恢复 Owner 命令形成唯一 Episode/ImportCommit/Structure 批次，提交窗口中断和重复 Activity 均不产生重复业务效果。
- `BE-MOD-006`：结构批次候选已绑定真实 HumanTask；本记录不宣称 Structure 决议已经由 Owner 生效。

## 残余风险与下一切片

- `human.episode_structure_review` 尚未调用 Planning Owner 接受必要制作任务并确认全部 Structure；下一切片必须返回真实 Owner Receipt 与正式 `episode_structures` 输出，不能只改变 Gate 状态。
- Storyboard Candidate/Review、单 Shot 局部重跑、公共 Workflow HTTP 组合与浏览器业务旅程仍待后续切片。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 远端 `main` 最近一次 CI 仍是旧提交 `12742a3` 的失败记录，失败点是旧 Workflow 从已删除的 `agent/openapi.json` 生成前端客户端；该路径已由本地提交 `7c842cb` 改为 Backend OpenAPI，并在本记录的本地 Required CI 中通过。未获准推送前，不宣称远端 CI 已恢复绿色。
