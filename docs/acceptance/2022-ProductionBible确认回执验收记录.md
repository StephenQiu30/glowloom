# Production Bible 确认回执验收记录

- 状态：Production Bible Owner Receipt 返回契约切片通过；Workflow 绑定尚未接入
- 日期：2026-08-26
- PRD：[项目制作圣经产品需求](../prd/3001-项目制作圣经产品需求.md)
- Design：[项目制作圣经生成执行框架设计](../design/3001-项目制作圣经生成执行框架设计.md) · [后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[项目制作圣经需求规格](../requirement/3001-项目制作圣经需求规格.md) · [后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 人工栅栏输入与决议绑定验收记录](2021-Workflow人工栅栏输入与决议绑定验收记录.md)

## 验收范围

本记录验收 `Production Bible Confirm → confirmed Bible + persisted CommandReceipt → idempotent replay` 的最小 Owner 契约。Production Bible Application Service 仍是确认状态的唯一 Writer，PostgreSQL/GORM 仍是唯一 SQL 事实源；内部调用者取得真实持久化 Receipt，公开 HTTP 只返回 Bible 资源。没有新表、Migration、手写 SQL、兼容返回类型或第二套回执。

## 实现证据

| 契约 | 结果 |
|---|---|
| Owner 返回 | `ConfirmResult` 同时包含确认后的 `domain.Bible` 与同一事务写入的 `platform/command.Receipt` |
| 原子性 | Bible 状态更新与 Receipt 创建继续位于现有 Production Bible GORM TransactionManager 内，任一步失败整体回滚 |
| 幂等重放 | 相同确认命令从 PostgreSQL 读取原 Receipt，并返回同一 Receipt ID 与同一 Bible，不创建第二事实 |
| 私有边界 | HTTP Handler 只展示 `ConfirmResult.Bible`，不会把内部 Receipt 放入公开 API 或 OpenAPI Schema |
| 单一事实源 | 复用现有 `CommandReceipt` GORM 模型和 `production_bible.confirm` operation，没有新增 Owner 表或复制状态 |

## 真实验证

1. Red：应用测试先要求 `ConfirmResult.Bible` 与 `ConfirmResult.Receipt`，原签名只返回 `domain.Bible`，编译按预期失败。
2. Green：应用与 HTTP Adapter 定向测试通过；公开响应适配为只展示 Bible。
3. PostgreSQL 16.15 集成测试从真实 `needs_review` Bible 执行确认，按返回 ID 读到 `production_bible.confirm` CommandReceipt；同一命令重放返回同一 Receipt，按 Workspace/Operation/Idempotency Key 计数仍为 1。
4. 完整 Backend 门禁在全新 PostgreSQL 16.15 与固定摘要 Temporal Server 上从头执行：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过，其中 `backend/tests/workflow` 用时 25.005 秒。
5. Agent 门禁通过：锁定开发依赖安装完成，Ruff 检查与格式、Pyright 均无错误，Pytest 12 项通过。
6. Frontend 门禁通过：`npm ci`、Backend OpenAPI 客户端生成、ESLint、TypeScript、Vitest 45 项与 Next.js 生产构建全部通过。
7. OpenAPI 生成漂移、Delivery Hygiene、数据库架构边界与 `git diff --check` 通过；未引入凭据、数据、日志、浏览器报告、Migration、手写 SQL、第二 ORM 或兼容分支。

## 残余风险与下一切片

- 本切片只暴露 Production Owner 已有的真实回执，不代表 Workflow 已绑定它；下一切片需把 ReviewDecision、Owner Receipt、正式 `node-output-v1` 与 Temporal Signal 串成一个可恢复协调链。
- 现有 Bible Confirm 尚未完成项目资产/状态/版本物化，因此不宣称 `PB-FR-009` 或 Production Bible 完整发布已经通过。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地门禁通过不等于远端绿色；`origin/main` 最近一次历史运行仍失败，这些本地提交尚未获准推送。
