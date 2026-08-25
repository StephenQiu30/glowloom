# Workflow 生产回执与人工栅栏输出验收记录

- 状态：Production Bible Human Gate 的 Owner Receipt 与正式输出绑定切片通过；其他 Owner 尚未接入
- 日期：2026-08-26
- PRD：[剧本到分镜 MVP 产品需求](../prd/0009-剧本到分镜MVP产品需求.md)
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Workflow 人工栅栏输入与决议绑定](2021-Workflow人工栅栏输入与决议绑定验收记录.md) · [Production Bible 确认回执](2022-ProductionBible确认回执验收记录.md)

## 验收范围

本记录验收 `ReviewDecision → Production Bible Confirm → CommandReceipt + confirmed Bible → Workflow Apply Receipt → Temporal Signal → Gate node-output-v1 → episodes node-input-v1` 的最小可恢复闭环。Backend 是 Production 与 Workflow 业务事实的唯一 Writer，Temporal 只保存跨步骤历史；PostgreSQL/GORM Model Catalog 是唯一 SQL 事实源。没有 Migration 文件、手写 SQL、兼容字段、第二 ORM 或 Human Gate 专用下游旁路。

## 实现证据

| 契约 | 结果 |
|---|---|
| Owner 应用 | Backend Production 适配器只处理 `gate.production_bible_review`，以 ReviewDecision ID 派生稳定确认幂等键，并调用正式 Production Bible Application Service |
| 单次生产效果 | Bible Confirm 与 `production_bible.confirm` CommandReceipt 在现有 Owner 事务中提交；Signal 结果未知后的重试读取同一 Workflow Apply Receipt，不再次调用 Owner |
| 协调事实 | Workflow Apply Receipt 保存 Owner Receipt ID、Operation、canonical Output Snapshot/Hash，并由 GORM CHECK 约束正向/拒绝类决议的证据形态 |
| Temporal 对账 | Signal Request/History Hash 包含 Owner Receipt ID 与完整 Output/Hash；相同 Signal ID 的不同证据返回 Conflict |
| Gate 投影 | Runtime 逐项核对 Intent、Apply Receipt、Node、Decision 与 Owner 证据后，原子写入 Gate `SUCCEEDED` 和同一 `node-output-v1`/Hash |
| 下游传播 | `episodes` 继续使用普通 Graph/Port 输入解析器，并读取 Gate 输出中的 confirmed Production Bible revision 2；没有 Review 状态旁路 |

## 真实验证

1. Red：Signal Service 测试先声明 Owner Application/Result、Owner Receipt 与正式输出，原契约因缺少这些类型和接口而编译失败。
2. Green：聚焦 Workflow 测试通过；相同幂等键的 Unknown→Already Applied 重试复用 Signal/Input Hash/Owner Receipt/Output，Owner 调用次数保持 1；同 key 不同命令在产生新副作用前失败。
3. 全新 PostgreSQL 16.15 空库通过 GORM Schema Sync，实际创建 `owner_receipt_id`、`owner_operation`、`output`、`output_hash` 与 `ck_wrk_gate_apply_owner_evidence`、`ck_wrk_gate_apply_output_hash`，没有执行 Migration 或手写 DDL。
4. 真实持久化旅程从 Production Bible candidate 打开 Human Gate、提交 ReviewDecision、确认 Bible、保存一条 CommandReceipt、经历 Unknown Signal 重试、应用 Gate，再执行 `episodes`；下游输入精确引用相同 Bible ID、revision 2、`production_bible` Value Type 与 Content Hash。
5. 完整 Backend 门禁在全新 PostgreSQL 16.15 与固定摘要 Temporal Server 上从头通过：`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 均成功，其中 `backend/tests/workflow` 用时 23.869 秒。
6. Agent 门禁使用 Python 3.11.15 安装锁定开发依赖后通过：Ruff 检查与格式、Pyright 均无错误，Pytest 12 项通过。
7. Frontend 门禁通过：`npm ci`、Backend OpenAPI 客户端生成、ESLint、TypeScript、Vitest 45 项与 Next.js 生产构建全部成功。
8. OpenAPI 生成漂移、Delivery Hygiene 与 `git diff --check` 通过；未引入凭据、数据、日志、浏览器报告、Migration 文件、手写 SQL、第二 ORM 或兼容分支。

## Requirement Checklist

- `BE-MOD-005`：Production Bible Gate 已持久化正式 Owner 输出并按普通节点输出向下游传播；Shot Workflow 与局部重跑未完成。
- `BE-MOD-006`：HumanTask、ReviewDecision 与 Owner 应用候选保持同一冻结对象和 Subject Revision。
- `BE-JRN-006`：Production Bible 决议已满足一次 Owner 效果、持久 Apply Receipt、Temporal Unknown 对账和下游正式版本读取；其他 Owner 尚未接入，因此不宣称旅程整体完成。

## 残余风险与下一切片

- 当前具体 Owner 适配器只支持 Production Bible；Episode Structure、Storyboard、Generation、Asset 等 Gate 必须分别调用各自正式 Owner，不能用统一伪回执代替。
- Production Bible 完整发布/资产物化仍未完成，本记录不验收 `BE-JRN-002`、Shot Workflow 或成片闭环。
- `agent-browser` 按约定只在全部开发完成后执行，本切片不计作浏览器验收。
- 本地门禁通过不等于远端绿色；未获准推送前，不宣称远端 CI 已修复。
