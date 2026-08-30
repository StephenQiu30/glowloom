# Workflow Production Bible 持久等待节点验收记录

- 状态：`activity.production_bible` 节点切片通过；真实整剧 Agent 运行与后续分集/场景/分镜节点尚未验收
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md) · [项目制作圣经生成执行框架](../design/3001-项目制作圣经生成执行框架设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow Worker 组合根](2025-WorkflowWorker组合根验收记录.md)

## 验收范围

本记录验收 `Script Node → activity.production_bible → Production Bible Application → Agent Invocation → Temporal 持久等待 → Candidate Node Output` 的 Backend 闭环。Production Bible 仍是唯一 Owner，Workflow 不直接读写 Bible 表；PostgreSQL/GORM 仍是唯一业务 SQL 事实源，Temporal 仍是跨步执行引擎。

## 实现证据

| 契约 | 结果 |
|---|---|
| Owner 边界 | Executor 只通过 Production Bible Application `Create` 提交/重放命令，不导入 GORM Repository 或手写 SQL |
| 稳定身份 | WorkflowRun 冻结的 Workspace、Project、User ID、Token Version 和 NodeRun 幂等键每次都重新交给 Owner 鉴权/重放 |
| 正常等待 | Bible `queued/running` 只返回无输出 `RETRYING`；Runtime 在 Claim fencing 事务中投影 `RETRYING` 并清除 Claim，不写 Node Output/Cache |
| Temporal 持久性 | Workflow 用 5 秒持久 Timer 后重查同一 Owner Receipt；正常等待不消耗 Activity 错误重试，不设整剧业务墙钟超时 |
| 暂停边界 | 每次持久 Timer 后先处理 Control Signal；Pause 期间不调用下一次 Activity，Resume 后才恢复 Owner 查询 |
| 终态输出 | 只有 `needs_review` 且 Workspace/Project/Revision/Input Hash/Result Hash 与冻结输入一致才产出 `production_bible_candidate` `node-output-canonical` |
| 缓存 | 等待期不写缓存；终态 Candidate Output 与 Node Cache 在现有同一 GORM 事务中提交 |

## Red → Green 与真实验证

1. Red：`TestRuntimeProjectsExternalExecutorWaitWithoutOutputOrCache` 先失败为 `workflow node executor returned an invalid status`；`TestEpisodeWorkflowDurablyWaitsForExternalNodeWithStableBusinessAttempt` 先失败为 `node activity returned an invalid terminal output`。
2. Green：Runtime 只接受不带输出的 `RETRYING`，Temporal 对该状态使用持久 Timer；`RETRYING` 夹带输出的负向测试仍被拒绝。后续审阅发现 Pause 在持久等待内会延迟到节点终态；`TestEpisodeWorkflowPausesExternalNodePollingUntilResume` 先以 `external node calls while paused=true` 失败，修正后已证明暂停期间零额外查询。
3. 真实 PostgreSQL 16.15 + 固定摘要 Temporal 定向旅程通过，用时 `7.739s`：真实 Compiler/Temporal Worker/Runtime/Script Service/Bible Service/Bible Worker 完成 `script → bible`，最终只有 1 个 Bible、1 个 Agent Invocation 和 1 个 `production_bible.create` Receipt。Agent 结果在该集成测试中由符合正式 `AgentClient` 契约的确定性替身返回，不宣称已运行真实整剧模型。
4. Backend 全量门禁在重建后的空 PostgreSQL 上通过：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`，其中 `backend/tests/workflow` 用时 `35.842s`。复用已有测试数据库时全局计数断言曾真实失败；验证没有改断言或加兼容分支，而是按 CI 语义重建空库后全量重跑。
5. Agent 门禁通过 Ruff、Pyright 和 12 项 Pytest；Frontend 在官方 Node 22.23.2 容器中通过锁定安装、OpenAPI 生成、ESLint、TypeScript、45 项 Vitest 和 Next.js 生产构建。
6. 开发/生产 Compose 渲染、Backend 镜像构建、API/Workflow Worker 双二进制、OpenAPI 零漂移、Delivery Hygiene 和 `git diff --check` 全部通过。

## Requirement Checklist

- `BE-MOD-005`：持久外部节点已具备无墙钟业务超时的正常等待、Claim fencing、稳定 Owner Receipt 重放和终态输出/缓存。
- `BE-MOD-008`：本切片只消费现有 Agent Invocation/Grant/Result 契约，不宣称 Manifest、Budget 和 Tool Allowlist 的整体要求已完成。
- `BE-JRN-003`：真实 Temporal 已完成 Script 到 Bible Candidate；完整 Episode Graph 仍需后续 Executor。

## 残余风险与下一切片

- 本验收没有运行 139,723 字/60 集真实原稿与真实 Agent 模型，不能替代 3001 整剧业务验收。
- `production.episode_plan`、分集结构、Storyboard 和 Export 等后续节点仍未接入正式 Executor。
- Workflow Start/Query/Control 的 Backend HTTP 组合边界尚未完成，浏览器尚不能启动这条流程。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地 required CI 全部通过不等于远端绿色；未获准推送前，不宣称远端 CI 已修复。
