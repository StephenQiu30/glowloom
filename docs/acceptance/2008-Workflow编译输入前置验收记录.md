# Workflow 编译输入前置验收记录

- 状态：Authoring 编译输入前置切片通过；完整 Authoring 与 Workflow 尚未完成
- 日期：2026-08-25
- Design：[系统总体架构](../design/0003-系统总体架构.md)、[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)

## 验收范围

本记录只验收 Workflow Compiler 的真实输入前置：系统 Node Catalog、Graph 发布语义、可变 AuthoringDraft、冻结 Script Revision 和不可变 AuthoringRevision。它不宣称 Diff、GraphPatch、完整发布门禁、WorkflowDefinitionVersion、RunInputSnapshot、Temporal 或 Run/Node Projection 已实现。

PostgreSQL 仍是唯一 SQL 事实源。四类 Authoring 事实由 `backend/internal/platform/database/model` 中的 GORM 模型定义，通过唯一 Schema Catalog 同步；仓库没有增加 Migration 文件、迁移账本、第二 ORM、第二数据库连接或 Agent 写库路径。

## 实现证据

| 契约 | 结果 |
|---|---|
| 系统节点目录 | 九个真实节点覆盖 Script Revision → Production Bible → Episode Plan/Structure → Storyboard Draft/Review/Export；重复启动收敛到同一目录版本，同版本内容漂移 fail closed |
| Graph 校验 | 发布前拒绝未知节点版本、无效 JSON Schema 配置、必填输入缺失、端口类型不匹配和一般环 |
| 统一 Schema | Guided 与 Canvas 使用同一 Graph；布局和非执行视觉节点保留在内容快照中，但不进入执行 Hash |
| 确定性 Hash | 可执行 Graph、冻结输入和可执行节点目录共同决定 Execution Hash；完整快照决定 Content Hash |
| 冻结输入 | 当前 MVP 只接受属于同一 Project 的已发布 Script Revision，并核对 Revision ID、Version 与 Normalized Hash |
| 草稿并发 | 创建使用 Command Receipt 幂等；更新使用 Draft Revision 乐观栅栏，陈旧写入返回冲突 |
| 不可变发布 | 每次有效草稿版本只发布一个不可变 AuthoringRevision；重复命令返回同一结果，历史版本不被后续布局修改覆盖 |
| 单一事实源 | `aut_node_definition_versions`、`aut_node_catalog_versions`、`aut_drafts`、`aut_revisions` 只由 Backend/GORM 持久化 |

## 真实验证

1. `cd backend && go test ./... && go vet ./...`：通过。
2. 独立 UTF-8 临时 PostgreSQL 运行 `go test ./tests/authoring -count=1 -v`：七个测试及全部子场景通过，实例在验收后停止，临时目录移入废纸篓。
3. `TestSystemNodeCatalogIsPersistedOnceAndRejectsVersionDrift`：通过，证明重复/并发初始化收敛且同版本漂移被拒绝。
4. `TestPublishSnapshotNormalizesGraphAndExcludesLayoutFromExecutionHash` 与 `TestPublishSnapshotExcludesVisualNodesFromExecutionHash`：通过，证明执行 Hash 边界稳定。
5. `TestGraphValidationRejectsUnknownVersionInvalidConfigAndPortMismatch` 与 `TestGraphValidationRejectsGeneralCycles`：通过，证明无效图在发布前失败。
6. `TestAuthoringDraftPublishesImmutableRevisionsFromVerifiedInputs`：通过，证明无效冻结 Hash 被拒绝；布局更新产生新内容版本但保持相同 Execution Hash；旧版本内容和 Hash 不变；最终只有一个 Draft、两个 Revision。

## Requirement 状态

- `BE-MOD-004`：已完成 NodeDefinitionVersion、Graph/Port Validation、Draft/Revision 与 Publish 的当前 MVP 前置；因 Diff、GraphPatch 和完整外部事实门禁尚未实现，主计划保持未完成。
- `BE-JRN-003`：已完成同 Schema AuthoringRevision 的输入侧前置；Compiler、不可变 Definition、稳定 Workflow ID、Start Intent/Receipt、Temporal 对账和 Replay 均未完成。

## 残余风险与下一切片

- 当前冻结输入只支持 `script_revision`，与下一条真实剧本到分镜 Compiler 路径一致；Preset、Policy、Skill、Prompt、Model 与 Artifact 版本需在各自事实 Owner 落地后加入同一不可变快照。
- 当前 Authoring 用例已具备应用层和 GORM 仓储，尚未暴露独立 HTTP Authoring API；下一切片由 Workflow Compiler 直接消费不可变 Revision，不为尚无用户旅程的接口预建空路由。
- 下一切片进入 `BE-MOD-005`：先以失败测试固定 AuthoringRevision → WorkflowDefinitionVersion + RunInputSnapshot 的确定性编译契约，再实现唯一 Backend Writer 和 GORM 持久化。
- 最终 `agent-browser` 验收仍只在全部开发和自动化回归完成后执行。
