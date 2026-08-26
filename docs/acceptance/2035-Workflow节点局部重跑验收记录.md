# Workflow 节点局部重跑验收记录

- 状态：实现、目标旅程与完整本地 CI 已通过；远端 GitHub Actions 待获准推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)

## 验收范围

本记录验收节点级局部重跑的最小可用闭环：从真实失败或已成功的源 Run 派生新 Run，只执行指定根节点及其下游 Dirty 闭包，并将执行必需的上游 canonical 输入/输出作为带源 NodeRun 引用的不可变投影复用。本切片不增加公共 HTTP API，不声称已完成尚无真实 Generation/Shot 节点的媒体 ShotWorkflow。

## 实现事实

1. `BuildRerunScope` 从已编译拓扑确定 Dirty 下游闭包和必需上游闭包，包括多输入侧路依赖；缺少上游终态输出、端口或 Hash 漂移时 fail closed。
2. `StartService.Rerun` 重新编译不可变 Authoring Revision 以执行授权与身份复核，派生新 WorkflowRun、Start Intent/Receipt 和 Temporal Workflow ID，并保持原 Definition/Snapshot。
3. 复用上游使用现有终态 `SKIPPED` 与新增 `ReusedFromNodeRunID` 自引用，严格复制 canonical Input/Output Snapshot 与 Hash；不读写 `NodeCacheEntry`。
4. Runtime 只向 Temporal 返回 Dirty 节点的稳定拓扑子序列，但在 Backend 解析 Dirty 输入时复核复用投影与源 NodeRun；篡改来源引用立即拒绝启动。
5. Temporal 的节点最终失败先调用 Backend `FailRun` Activity，Backend 在单个 GORM 事务中原子投影失败 NodeRun/Run 和可执行恢复动作，不再依赖手工改库构造重跑源。
6. 数据只增加 GORM Model Catalog 可正向同步的两个来源引用字段；不修改已有状态 CHECK，不增加 Migration、手写 SQL Schema、兼容写入分支或第二事实源。

## 真实验收证据

- 域模测试覆盖 Dirty 闭包、多输入必需上游和非终态源拒绝。
- 真实 PostgreSQL 16.15 旅程验证失败投影、新 Run 派生、必需上游复用、无关节点排除、来源篡改拒绝、幂等重放和源 Run 不变。
- 真实 Temporal 旅程只调度 `transform` 与 `export`，不重跑已复用的 `source`；最终 Run 成功并通过完整 History Replay。
- Temporal 工作流测试验证节点 Activity 失败时先调度稳定 `FailRunCommand`，且仍向 Workflow 调用方返回原始执行错误。
- 全新 PostgreSQL/Temporal 环境上执行 `gofmt -l .`、`go vet ./...`与 `go test -count=1 -p 1 ./...`，Backend 全量通过且 Workflow 外部依赖测试未跳过。
- Agent 在 Python 3.11 虚拟环境重装锁定开发依赖后，Ruff check/format、Pyright 零错误和 12 个 Pytest 全部通过。
- Frontend 从 Backend OpenAPI 重新生成 Client，ESLint、TypeScript、45 个 Vitest、Next.js 生产构建和 Client Drift 检查全部通过。
- 开发/生产 Compose 可渲染；Frontend、Backend、Agent 三类部署镜像均重新构建，并通过 standalone、API/Worker 双二进制与私有 Candidate Runtime 入口检查。

## 边界与残余风险

- 单 Shot 媒体重跑仍待真实 Generation/Shot Owner、冻结引用与执行节点接入；节点局部重跑只是它的运行语义基础。
- 本地 CI 与 GitHub Actions 使用同一门禁，但当前 `main` 尚未获得用户授权推送；因此远端 `Required / CI` 仍不能声称已绿。
- `agent-browser` 仍按约定只在全部功能开发完成后执行，本切片不提前调用。
