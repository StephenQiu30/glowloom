# Workflow Storyboard 全批导出验收记录

- 状态：`production.storyboard_export` 的 applied Set 核对、逐集确定性导出、Export Set、Owner Receipt 与 Workflow 终态闭环通过
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow Storyboard 批次原子应用](2032-WorkflowStoryboard批次原子应用验收记录.md)

## 验收范围

本记录验收 `storyboards → Storyboard Owner CreateExportSet → StoryboardExport × Episode → StoryboardExportSet → Command Receipt → storyboard_export`。输入是人工 Gate 已确认的 applied Draft Set，不是调用时的“最新分镜”；输出是可持久读取的 Export Set，不是只导出首集后伪造的成功状态。

## 实现证据

| 契约 | 结果 |
|---|---|
| 完整输入 | Executor 只接受 `storyboards` 端口的 Set ID、applied revision 和正式 Content Hash；端口、类型、revision 或 hash 漂移均失败 |
| 正式 Hash 复核 | Owner 锁定 Draft Set 与全部 Episode，按 Set Batch 顺序重建正式 Shot 引用 Hash；必须与 Gate applied Hash 完全一致 |
| 漂移失败 | 真实 PostgreSQL 旅程修改最后一集 Shot Content Hash 后导出被拒绝；数据库中 Export Set、逐集 Export 和 Receipt 数均为 0 |
| 全批原子性 | 两集 Export、唯一 Export Set 和唯一 `storyboard.create_export_set` Receipt 由 Storyboard Owner 在同一 GORM 事务提交 |
| 可复现导出 | 每集复用现有确定性 ZIP 构建器，包含 `manifest.json`、`storyboard.json`、`storyboard.csv` 和 `storyboard.html`；包与文件都持久 Content Hash |
| 真实聚合 | `StoryboardExportSet` 按 Draft Set 顺序保存 Episode ID、Export ID、Order Hash 和 Content Hash；逐集 Export 通过 GORM FK 引用所属 Export Set |
| 恢复与重放 | 旅程先提交 Export Set/Receipt，再放行 Storyboard Gate；Export Node 以同一 Node Attempt 幂等键重放原 Receipt，没有生成第二批导出 |
| 最终 Workflow | 真实 Temporal 继续执行 `activity.storyboard_export`，Node 输出 `storyboard_export` 绑定并使两集 Workflow Run 进入 `SUCCEEDED` |
| 单一事实源 | Export Set 直接进入统一 GORM Model Catalog；没有 Migration、手写 SQL Schema、第二 ORM、第二数据库或 Agent Writer |

## Red → Green 与真实验证

1. Red：两集 Workflow 旅程加入已在 System Catalog 中的 Export 节点后，编译明确报错 `CreateExportSet`、`CreateExportSetCommand`、`StoryboardExportSet` 和 `ExportSetID` 不存在，证明当前实现仅停在 Storyboard Gate。
2. Green：Storyboard Owner 增加全批导出命令，逐集复用原有包构建器；Workflow Executor 只调用 Owner 公开 Application Interface；Export Set/Export/Receipt 在同一 GORM 事务写入。
3. 定向真实 Temporal 旅程 `TestProductionWorkflowWorkerCreatesStoryboardDraftSetForEveryConfirmedEpisode` 通过，用时 `13.264s`；覆盖 Hash 漂移无部分事实、两集完整导出、Owner 读取、Receipt 预提交重放和最终 Workflow 成功。
4. 在全新 `postgres:16.15-alpine` 与固定摘要 `temporalio/temporal` 上执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...`：Backend 全部包通过，真实外部依赖测试未跳过。
5. Agent 通过 Ruff check/format、Pyright 与 12 项 Pytest；Frontend 通过锁定安装、OpenAPI 生成、ESLint、TypeScript、16 个 Vitest 文件/45 项测试与 Next.js 生产构建。
6. OpenAPI 无漂移，开发/生产 Compose、Delivery Hygiene、`git diff --check`、Backend 镜像及 API/Workflow Worker 双二进制门禁通过。

## 残余风险与下一切片

- 单 Shot 局部重跑/Shot Workflow 尚未实现；下一切片先固定局部重跑的定义、依赖闭包和新 Run 身份，不修改旧成功 Run 的历史事实。
- Export Set 的公共 Workflow 查询组合与 UI 导航属于后续 HTTP/前端切片；逐集 Export 已可通过现有 Storyboard Owner 和下载 API 读取。
- `agent-browser` 依约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 远端 `main` CI 仍对应未推送的旧代码；本地门禁通过不等于远端 CI 恢复，未获准推送前不宣称远端绿色。
