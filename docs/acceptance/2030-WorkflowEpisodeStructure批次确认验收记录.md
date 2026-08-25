# Workflow Episode Structure 批次确认验收记录

- 状态：`human.episode_structure_review` 的 Planning Owner 原子批次确认、Apply Receipt、Temporal Signal 与正式输出闭环通过；Storyboard 节点尚未验收
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow Episode Structure 发布候选](2029-WorkflowEpisodeStructure发布候选验收记录.md)

## 验收范围

本记录验收 `episode_structure_candidate → HumanTask/ReviewDecision → Planning ConfirmPublishedStructureBatch → Command Receipt → Apply Receipt → Temporal Signal → episode_structures` 的 Backend 闭环。逐项任务接受和结构确认仍由 Planning Owner 写入；Review 只拥有 HumanTask/ReviewDecision，Workflow 只协调已提交证据和运行投影。

本切片不生成 Storyboard Candidate、不调用 Agent、不创建或应用 Shot。PostgreSQL/GORM 仍是唯一业务 SQL 事实源，Temporal 仍是唯一跨步骤工作流引擎。

## 实现证据

| 契约 | 结果 |
|---|---|
| 任务前置 | 结构发布时每个场次生成必要 `shot_breakdown` 任务；未通过既有 `AcceptTask` 接受必要任务前，批次确认失败且不留下确认 Receipt |
| 原子批次 | Planning 在一个 GORM 事务中锁定 published ImportCommit 与按 Segment 顺序加载的全部 Structure，重新核对 Workspace/Project/Script Version/Result Hash 与批次 Content Hash |
| 统一规则 | 单结构 `ConfirmStructure` 与批次确认共用同一个场次任务校验：每场一至四个任务、至少一个 `shot_breakdown`、必要任务全部 accepted |
| 单一回执 | 全部结构确认与唯一 `episode_structure.confirm_batch` Command Receipt 同事务提交；Receipt Resource ID 为 ImportCommit ID，不用多个单结构 Receipt 冒充批次证据 |
| 故障恢复 | 测试先提交 Owner 批次回执再调用 Signal Service，证明 Apply/Signal 重试只重放同一 Receipt，不重复确认 Structure |
| 正式输出 | Gate 输出 `episode_structures`，保持 Candidate 的 ImportCommit ID、revision 2 与 Content Hash；全部 Structure 为 confirmed 且 ConfirmedBy 与运行发起人一致 |
| 证据校验 | GORM Signal Store 按 `gate.episode_structure_review` 精确要求 `episode_structure.confirm_batch`，并校验 Receipt Workspace、Resource、Creator 和 canonical Output |
| 单一事实源 | 未增加 Model、Migration、迁移字段、手写 SQL、第二 ORM、第二数据库连接或 Agent Writer，复用既有 GORM Model Catalog |

## Red → Green 与真实验证

1. Red：真实 Worker 旅程先调用尚不存在的 `ConfirmPublishedStructureBatch`，编译明确失败，证明现有 Human Gate 只能确认 Bible/Plan，不能产生结构批次 Owner 证据。
2. Green：Planning 增加原子批次命令与 Receipt；Human Gate Applier、GORM Signal Evidence 增加精确结构 Executor/Operation 路由；没有增加通用 Service Locator、兼容分支或数据库状态。
3. 全新 PostgreSQL 16.15 与固定摘要 Temporal 执行 `TestProductionWorkflowWorkerConfirmsEpisodeStructuresThroughBatchGate`：通过，用时 `5.98s`。旅程先证明缺少任务接受时失败，再接受必需任务、预提交批次 Receipt、由 Signal 重放证据并使 Workflow Run 成功。
4. 同一旅程重复 Structure Signal 返回原 completed Intent；数据库只有一个 `episode_structure.confirm_batch` Receipt、没有 `episode_structure.confirm` 单结构 Receipt，Gate Output/Apply Receipt/Owner Receipt 引用同一 ImportCommit。
5. 再次重建空 PostgreSQL 16.15 与固定摘要 Temporal，执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`：全部通过；`backend/tests/workflow` 用时 `46.471s`，整组 Backend 门禁用时 `65.66s`，真实外部依赖测试未跳过。
6. Agent 在 Python 3.11 下通过 Ruff check/format、Pyright 与 12 项 Pytest；Frontend 在官方 Node `22.23.2` 下通过锁定安装、OpenAPI 生成、ESLint、TypeScript、45 项 Vitest 与 Next.js 生产构建。
7. OpenAPI 生成目录无漂移，开发/生产 Compose 渲染、Delivery Hygiene、`git diff --check`、Backend 镜像构建及 API/Workflow Worker 双二进制检查均通过。

## Requirement Checklist

- `BE-MOD-006`：第三类 Production Human Gate 已通过真实 Owner 批次回执生效，Review 没有直接修改 Structure。
- `BE-JRN-006`：结构批次的 ReviewDecision、Owner Receipt、Apply Receipt、Temporal Signal 与 Gate Output 已绑定同一候选和 canonical Hash；回执先提交的未知窗口可以安全重放。
- `BE-JRN-003`：Episode Workflow 已从冻结剧本走到正式 `episode_structures` 并成功结束当前图定义，结构确认前置任务与原子性门禁真实生效。

## 残余风险与下一切片

- `activity.storyboard` 与 `human.storyboard_review` 尚未接入正式 `episode_structures`；下一切片先固定 Storyboard Owner/Agent 的持久候选边界，不在结构 Gate 内提前生成 Shot。
- 单 Shot 局部重跑、公共 Workflow HTTP 组合与浏览器业务旅程仍待后续切片。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 远端 `main` 最近一次 CI 仍是旧提交 `12742a3` 的失败记录；本地现行 CI 已全绿，但未获准推送，因此不宣称远端 CI 已恢复绿色。
