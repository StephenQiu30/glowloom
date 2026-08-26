# Shot 绑定目标与单 Shot 局部重跑验收记录

- 状态：实现、真实 PostgreSQL/Temporal 目标旅程与完整本地 Required CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[正式 Shot Workflow 后半程](2049-正式ShotWorkflow后半程验收记录.md)

## 验收范围

本记录验收已成功首绑的单个 Shot 重新审核与替换绑定闭环：不改写原 Run，从 `shot` 根派生新 `lanverse.shot-production` Run，在新源节点冻结当前 Binding Target，打开新 HumanTask，选择另一个已物化 Candidate，并追加 `ShotImageBindingVersion` Revision `2`。

本切片不调用图片 Provider，不生成新 CandidateSet，不调度 Episode Workflow，也不实现前端局部修复页面、Episode 动态扇出、Motion/Video/Render 或最终成片。

## 实现事实

1. 发布 `lanverse.shot@2.0.0`，仅将 Shot Source 和 Binding Node 升级为 v2；CandidateSet/Human Gate 继续复用已接受的 v1 定义。v1 Source/Binding Executor 保留原语义，没有原地改写已发布契约。
2. `RequireShotImageBindingTarget` 通过 Storyboard Application Interface 进入单一 GORM 事务，对 active Shot 获取写锁并读取当前最大 Binding Revision。Target 只是 canonical 节点输出，没有新表、current pointer、Migration、DDL 或 Raw SQL。
3. Target Content Hash 冻结 Shot ID/Revision/Hash 和当前 Binding ID/Revision/Hash；节点的 Reference Version 表示下一 Binding Revision。Binding v2 不接受 Authoring 自报 Expected Revision，而是从 Target 推导并在 Owner 写事务中重建 Hash。
4. 重跑复用已验收的派生 Run 能力：DefinitionVersion/RunInputSnapshot 不变，Run/Temporal ID 更新，SourceWorkflowRunID/RerunRootNodeID 明确，四个 Shot 节点全部重跑且不借用 `SKIPPED`/Cache 伪造新 Target。
5. 成功 Binding Command 优先按原输入 Hash 重放 Receipt；尚未成功的过期/篡改 Target 失败关闭。并发使用同一 Target 的两个写入者仅一个能追加下一 Revision。

## Red → Green 验收证据

- Red 首先因缺少 `RequireShotImageBindingTarget`、`BindSelectedImageAtTarget`、v2 Command 和 `lanverse.shot@2.0.0` 明确编译失败；补齐 Owner 后 Catalog 身份仍为 v1，契约测试继续失败。发布 v2 Catalog/Executor 后同一契约转绿。
- 真实 PostgreSQL 目标测试验证：未绑 Target Revision `0`、首绑 Revision `1`、已绑 Target 冻结当前 Binding ID/Hash、成功 Receipt 重放、过期/篡改 Target 拒绝，以及两个并发替换中仅一个产生 Revision `2`。
- 真实 PostgreSQL + Temporal Journey 验证：首 Run 选择候选 B 产生 Binding Revision `1`；重复派生命令收敛到同一新 Run；新 Run 的 Source Node 冻结 Target Reference Version `2`，新 HumanTask 选择候选 A 后产生 Binding Revision `2`，原 Run 仍成功且无来源引用。首 Run 与重跑 Run 的 Temporal History 均使用 `lanverse.shot-production` 注册名 Replay 通过。
- 同一 Journey 在归档 Shot 后再启动时，v2 Source 在 HumanTask 之前失败，已有两个 Binding 不变；撤销发起人 Token Version 后也不能重读 active Shot。
- 新增两个目标测试在隔离 PostgreSQL/Temporal 上合计 7.601 秒通过。完整 Backend 首次因之前目标测试污染同一数据库而触发全局事实计数失败；销毁本任务专用三容器并从全新 PostgreSQL/Temporal/MinIO 重跑后，`gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全绿，Workflow 包耗时 102.759 秒。

## 完整 Required CI 证据

- Agent：Python 3.11 临时 venv 安装锁定开发依赖，Ruff check/format、Pyright `0 errors`、Pytest `12 passed`。系统 Python 因 PEP 668 拒绝直接安装，没有用 `--break-system-packages` 绕过。
- Frontend：`npm ci`、OpenAPI Client 重生成、ESLint、TypeScript、Vitest `16 files / 45 tests passed`、Next.js 16.2.12 生产构建通过；`frontend/src/api` 无漂移。
- Deployment/Delivery：开发与生产 Compose 均可渲染；Frontend/Backend/Agent 三类镜像完整构建；容器内 API、Workflow Worker、Frontend standalone、Codex CLI 和 Candidate Runtime 断言通过；仓库凭据/日志/跨语言边界卫生检查通过。

## Requirement Checklist

- [x] `BE-WF-SHOT-001`：v2 Catalog/Node Definition 和显式 v2 Executor 已发布，v1 Executor 语义保持。
- [x] `BE-WF-SHOT-002`：Shot/Current Binding 在 Storyboard Owner 单事务内冻结为 canonical Target。
- [x] `BE-WF-SHOT-003`：过期/篡改 Target、Receipt 重放与并发单胜者已通过真实 PostgreSQL 验证。
- [x] `BE-WF-SHOT-004`：首绑后的 Shot 派生 Run、新 HumanTask/Selection/Binding、源 Run 不变和双 History Replay 已通过真实 Temporal 验证。

## 边界与残余风险

- 当前重跑只在同一已物化 CandidateSet 内重新选择，不是新的图片 Provider 生成；真实 Provider Adapter/Credential Resolver/Generation Executor 仍未选型或实现。
- Episode 动态扇出 `ShotWorkflow × N`、新候选生成、前端局部修复界面、Motion/Video/Render 和最终成片仍未完成，不得由本记录推导完整 ShotWorkflow 或 Guided MVP 已完成。
- 从下游节点派生的 Run 会携带原 Source 的 Target；当它已过期时，Owner 在最终绑定事务中失败关闭。产品交互应从 `shot` 根发起可完成的单 Shot 替换，本切片不增加通用重跑策略抽象。
- 远端 GitHub Actions 只有在获得推送授权后才能验证；本记录只声明本地按当前 CI 定义真实执行通过，不声明远端已绿。
- `agent-browser` 按用户约定只在全部开发完成后执行；真实 Provider、Episode 扇出与前端交互等后续开发仍未完成，本切片不提前调用。
