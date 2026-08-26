# Production Shot 图片绑定验收记录

- 状态：实现、真实 PostgreSQL 目标旅程与完整本地 Required CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Generation CandidateSet 与 Workflow 人工选择](2047-GenerationCandidateSet与Workflow人工选择验收记录.md)

## 验收范围

本记录验收 `CandidateSelection → Production Shot Image Binding` 最小闭环：Production/Storyboard Owner 通过 Generation Application Port 重读当前仍有效的选择事实，将 Shot、Selection、Candidate 和 Artifact 的精确版本与 Hash 冻结为追加式 `ShotImageBindingVersion`，并由 Workflow Production Executor 消费已冻结的 Shot/Selection Node Output 执行绑定。

该切片证明已人工选中的图片可以稳定进入正式 Storyboard Shot 生产事实，不代表真实云图片生成、System Catalog Shot 节点、Temporal ShotWorkflow 或单 Shot 局部重跑已完成。

## 实现事实

1. Production 声明最小 `SelectedImageSource` 消费者端口；Generation Adapter 只通过现有 Selection Service 的 `RequireSelected` 读取 Owner 快照，并核对被选 Candidate 的 Artifact ID/SHA-256。Production 不查询 Generation、Review 或 Asset 表。
2. `stb_shot_image_binding_versions` 是 GORM Model Catalog 中的唯一绑定事实，按 Shot 追加不可变版本；当前值由最大 Binding Revision 确定，不新增 current pointer、Migration、Raw SQL、第二 ORM 或第二 SQL 事实源。
3. 绑定命令只接受 Shot ID、已冻结 Shot Revision/Content Hash、Selection ID、Expected Current Revision 和幂等键。服务重读 Owner 事实后，在同一 PostgreSQL/GORM 事务内追加 Binding 与 Command Receipt。
4. 同幂等输入重放返回同一 Binding/Receipt；同键异输入、同 Selection 重复绑定、Expected Revision 不匹配、跨项目、归档 Shot、撤权、Shot 漂移、Selection/Artifact 漂移和 Binding Hash 漂移全部失败关闭。
5. `activity.production_shot_image_binding` Executor 只消费一个 `production_shot` 和一个 `generation_candidate_selection` 冻结输入，验证类型、版本与 Hash 后产生正式 `production_shot_image_binding` Node Output。生产 `workflow-worker` 已组装真实 Generation Selection/Artifact Readiness Adapter 和 Production Binding Service。

## 真实验收证据

- Red 阶段先新增目标测试，编译明确失败于缺少 `SelectedImageSnapshot`、`SelectedImageSource`、`ShotImageBindingService`、绑定 Command/Result 与 GORM Model；补齐最小 Domain/Application/Adapter/Executor/Bootstrap 后同一测试转绿。
- 真实 PostgreSQL `16.15-alpine` 上的目标测试通过；最终 `go test -race` 在全新数据库上 3.155 秒通过。测试覆盖首绑、8 路并发幂等重放、替换到 Revision 2、两路同 Expected Revision 并发替换仅一个成功到 Revision 3，并核对数据库最终只有 3 个 Binding 和 3 个 Receipt。
- 目标旅程使用真实 GORM Catalog 创建正式 Shot，并经真实 Workflow Production NodeExecutor 产生绑定 Node Output；还验证同键异 Selection、同 Selection 换键重复绑定、Selection Artifact 快照漂移、跨 Workspace、跨项目、持久化 Binding Hash 篡改、Shot Revision/Hash 漂移、归档 Shot 和 Token Version 撤权被拒绝。
- Generation Adapter 独立测试了真实 Selection Service 快照映射与 Artifact 漂移拒绝；PostgreSQL 绑定集成测试使用受控 `SelectedImageSource`，而生产 Worker 组合根使用真实 Generation Selection Service。本验收不宣称已跑通真实云 Provider 旅程。
- 最终 Backend 在全新 PostgreSQL 数据库、真实 Temporal 和私有 MinIO 上执行 `gofmt` 检查、`go vet ./...` 与 `go test -count=1 -p 1 ./...` 全绿，Workflow 包耗时 140.111 秒。
- Agent 使用 Python 3.11.15 执行锁定依赖安装、Ruff check/format、Pyright 零错误与 12 个 Pytest 全绿；Frontend 使用 Node 22.23.2 执行 `npm ci`、OpenAPI 生成、ESLint、TypeScript、45 个 Vitest 和 Next.js 生产构建全绿，生成 Client 无漂移。
- 开发/生产 Compose 配置、仓库卫生、Frontend/Backend/Agent 三类镜像构建与容器内 API、Workflow Worker、Frontend standalone、Codex CLI/Candidate Runtime 断言全部通过。

## 边界与残余风险

- 当前没有发布正式 System Catalog Shot/图片节点；Executor 的目标旅程由测试直接提供已冻结 Node Output，不得将它解读为用户已能从正式画布启动 Shot 图片工作流。
- 首个真实图片 Provider、Credential Resolver、Callback 验签、图片生成 Executor 和生产下载协议仍未正式选定或实现，当前不能宣称云图片生成可用。
- Temporal ShotWorkflow、单 Shot 局部重跑、Render、公开绑定 API 和图片审核页面仍待后续任务。
- 本地 Codex CLI 只承担 Agent 服务内的文本/结构化 AI 调用，不作为图片 Provider、Selection Source 或 Artifact 兼容替身。
- 远端 GitHub Actions 只有在获得推送授权后才能验证；本记录只声明本地按当前 CI 定义真实执行通过，不声明远端已绿。
- `agent-browser` 按用户约定只在全部开发完成后执行；上述后续开发仍未完成，本切片不提前调用。
