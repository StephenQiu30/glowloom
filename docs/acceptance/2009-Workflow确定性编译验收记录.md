# Workflow 确定性编译验收记录

- 状态：Compiler、Definition Version 与基础 Run Input Snapshot 切片通过；Workflow 运行时尚未完成
- 日期：2026-08-25
- Design：[系统总体架构](../design/0003-系统总体架构.md)、[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[平台 V1 需求规格](../requirement/0001-平台V1需求规格.md)、[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 编译输入前置验收记录](2008-Workflow编译输入前置验收记录.md)

## 验收范围

本记录验收唯一 Workflow Compiler 从已发布 AuthoringRevision 生成不可变 WorkflowDefinitionVersion 与基础 RunInputSnapshot 的确定性、持久化和并发收敛。它不宣称 WorkflowRun/NodeRun Projection、Node Cache、Start/Control/Signal Intent/Receipt、Temporal History、HumanTask 或 Replay 已实现。

Compiler 位于 `backend/internal/workflow`，以 Authoring Application 只读端口获取受权限保护的 Revision/Catalog。只有 Backend 的 GORM Adapter 写入 PostgreSQL；没有 Migration 文件、手写 SQL、第二 ORM、第二 SQL 事实源、Agent 数据库连接或替代 Temporal 的进程内执行引擎。

## 实现证据

| 契约 | 结果 |
|---|---|
| 唯一输入边界 | 编译命令只接受 AuthoringRevision ID；传入可变 AuthoringDraft ID 返回未找到，不允许绕过发布 |
| 来源对账 | 编译前重新计算 Authoring Content/Execution Hash，并核对 Catalog ID、Version、Content/Execution Hash 与冻结 Script Revision |
| 统一 Definition | Guided/Canvas 对等执行语义生成相同 Definition Hash 和 Run Input Snapshot Hash，来源 Revision ID 仍作为审计元数据分别保留 |
| 执行图 | 纯视觉节点被排除；执行节点固化 Definition Key/Version/Content Hash、Executor、Cache Policy 和 Risk Level |
| 确定性顺序 | Compiler 生成按依赖与稳定节点 ID 排序的拓扑执行顺序，一般环或视觉节点参与执行边时 fail closed |
| 运行契约 | 当前系统 Compiler、Temporal Workflow Type、Workflow Type Version 与 Runtime Contract Version 均冻结为 `1.0.0` 契约 |
| 不可变事实 | `wrk_definition_versions` 以 Authoring Revision + Compiler Version 唯一；`wrk_run_input_snapshots` 以 Definition 唯一 |
| 并发收敛 | GORM 使用唯一约束、`ON CONFLICT DO NOTHING`、行锁和 JSON/Hash 对账；六路不同幂等键并发首次编译返回同一 Definition/Snapshot ID |
| 命令幂等 | 相同幂等键重放返回原事实；不同幂等键对同一不可变来源也收敛到原事实 |

## 真实验证

1. `cd backend && go test ./... && go vet ./...`：通过，包括数据库架构边界测试。
2. 独立 UTF-8 临时 PostgreSQL 运行三个 Compiler 契约/旅程：通过；实例在验收后停止，临时目录移入废纸篓。
3. `TestCompilerProducesEquivalentDefinitionForGuidedAndCanvas`：通过，证明两种 Authoring UI 不改变执行 Definition。
4. `TestCompilerExcludesVisualNodesAndRejectsRevisionDrift`：通过，证明视觉节点不进入执行图，Revision/Catalog Hash 漂移被拒绝。
5. `TestCompilerPersistsOneImmutableDefinitionAndRunInputSnapshot`：通过，证明真实 Authoring 发布 → Compiler → GORM Definition/Snapshot 链、Draft 负向边界、幂等重放和唯一事实计数。
6. 独立 PostgreSQL 运行 `go test -race ./tests/workflow -run '^TestCompilerPersists' -count=1`：通过，六路并发首次创建无数据竞争且只产生一个 Definition、一个 Snapshot。

## Requirement 状态

- `PLT-FR-003`：同 Schema Revision 与唯一 Compiler 的内部编译链已通过；因尚无 Start Run 公共命令，完整“不能绕过 Revision 启动运行”仍未闭环。
- `PLT-FR-004`：当前 MVP 已冻结 Script Revision、Authoring、Node Definition/Catalog 与 Runtime Contract；Preset/Policy/Skill/Prompt/Model/Artifact Owner 就绪后仍需补入同一 Run Input Snapshot。
- `BE-MOD-005`：Compiler、WorkflowDefinitionVersion 与基础 RunInputSnapshot 已完成；Run/Node Projection 和 Node Cache 未完成，因此主计划保持未完成。
- `BE-JRN-003`：Revision → Definition 已完成；稳定 Workflow ID、Start Intent/Receipt、Temporal AlreadyStarted/Unknown 对账与 Replay 未完成。

## 残余风险与下一切片

- 当前 RunInputSnapshot 是 Definition 的不可变基础输入事实；下一切片创建 WorkflowRun 时必须显式引用该 Snapshot，局部运行新增的 Selection/Scope 需要另建运行级不可变范围，不得修改本快照。
- Compiler 已通过真实组件装配测试，但尚未暴露独立 HTTP 入口；下一切片由 Start Run 用例在 Backend Composition Root 装配并调用，避免预建没有用户旅程的编译路由。
- 下一切片按 Red → Green 实现 WorkflowRun/NodeRun Projection、稳定 Workflow ID 与 Start Intent/Receipt；在这些业务事实落地前不接入 Temporal SDK。
- 最终 `agent-browser` 验收仍只在全部开发和自动化回归完成后执行。
