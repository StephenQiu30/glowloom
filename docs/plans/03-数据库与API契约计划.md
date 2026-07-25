---
layer: Plan
doc_type: Database and API Contract Implementation Plan
doc_no: PLAN-03
title: 数据库与API契约计划
status: accepted
version: 1.0.0
owner: Lanverse
audience: [Architecture, Backend, Frontend, QA, Operations]
feature_area: PostgreSQL、HTTP 与生成客户端
purpose: 执行逐表 SQL 迁移并建立 FastAPI OpenAPI 到 umi 客户端的唯一契约链
canonical_path: docs/plans/03-数据库与API契约计划.md
inputs: [DESIGN-04, DESIGN-06, DESIGN-13, PRODUCT-01, PLAN-01, PLAN-02]
outputs: [0001_mvp 迁移, asyncpg 基础设施, HTTP 公共契约, OpenAPI/umi 工具链兼容证明]
triggers: [Schema 变化, Operation exact-set 变化, 生成工具变化]
updated: 2026-07-25
downstream: [PLAN-04至PLAN-09, ACCEPTANCE-01]
---

# 数据库与API契约计划

## 1. 准入、结果与边界

执行前 `database_design_ready` 必须 `passed`，PLAN-02 必须 Green。目标是证明根 `sql/` 可在隔离空库确定性建立 DESIGN-06 的 20 张应用表，并先以测试专用代表性契约证明 FastAPI/Pydantic→OpenAPI 3.1→`@umijs/openapi` 的工具链兼容。业务 Operation 由 PLAN-04～08 实现，最终 exact-set 制品和生成客户端由 PLAN-09 在所有路由完成后一次收敛。

Alembic 只执行根 SQL，不复制 DDL、不使用 ORM/Metadata/autogenerate；前端不维护第二份 URL/DTO；本计划不实现模块业务规则。

## 2. 契约制品与路径

| 制品 | 唯一路径 | 规则 |
| --- | --- | --- |
| 物理 DDL | `sql/01_*.sql`～`20_*.sql` | 每表一文件，内联 key/FK/CHECK，所属表索引同文件 |
| 首次迁移 | `backend/migrations/versions/0001_mvp.py` | 按编号读取根 SQL；无复制 DDL |
| 运行时数据 | `backend/src/lanverse/infrastructure/database/` | asyncpg pool/UoW/transaction；参数化 SQL |
| HTTP 契约 | 各模块 `transport` + application Pydantic contract | FastAPI 路由是唯一契约源 |
| OpenAPI | `backend/scripts/export_openapi.py`；最终 `backend/openapi/openapi.json` | 本计划交付确定性导出工具；最终制品由 PLAN-09 从同一 `create_app()` 生成 |
| 前端生成 | `frontend/openapi2ts.config.ts`；最终 `src/services/generated/` | 本计划用临时目录证明 `@umijs/openapi` 1.14.1 兼容；最终生成目录由 PLAN-09 提交且不手改 |
| 请求/缓存 | `src/lib/request.ts`；最终 `src/services/api.ts` | 本计划交付唯一 sender；PLAN-09 在最终生成函数上建立 RTK Query endpoint |

## 3. TDD 实施任务

| Task | Red 与失败信号 | 最小 Green / 目标路径 | 命令 / Test / Evidence |
| --- | --- | --- | --- |
| P03-T001 数据库门禁 | 静态测试断言 20 文件/表、列、51 FK、候选键、partial、22 JSONB 映射；任一输入非 accepted 或 gate 缺失即失败 | 只修正式 Design/根 SQL/gate，不执行 DDL；`test:` | `make test-database-design`；T-DATABASE-DESIGN-GATE；EV-000 |
| P03-T002 首次迁移 | 空库尚无 20 表，migration/catalog test Red | Alembic env + `0001_mvp` 顺序执行根 SQL；`impl:` | `make test-migration` 双 `upgrade head`；T-MIGRATION/T-DB-CONSTRAINTS；EV-002 |
| P03-T003 数据库基础设施 | pool、transaction、Row Mapper/UoW contract tests Red | config、pool、显式 transaction 与错误映射；不建 BaseRepository；`impl:` | pytest integration + Ruff/mypy |
| P03-T004 HTTP 公共契约 | Problem、ETag、Idempotency、`202` 和 Task 查询的公共 schema/header/error tests Red | App Factory 基础、公共 contract/error/header；不注册未实现业务 Operation；`impl:` | `make test-contract-foundation`；T-CONTRACT |
| P03-T005 OpenAPI 导出工具 | 测试专用 FastAPI fixture 连续导出不确定或非 3.1 Red | `export_openapi.py` 接收同一 App Factory 并稳定排序/写入；不提交伪业务 artifact；`chore:` | 两次临时 artifact SHA 相同 |
| P03-T006 umi 兼容探针 | 代表性 201/202/Problem/ETag/Task schema 的 OpenAPI 3.1→umi→tsc 先 Red | 锁定 `openapi2ts.config.ts`、request sender 和测试临时输出；不提交最终 generated；`impl:` | `make contracts-toolchain-check`；EV-002 |

PLAN-04～08 每实现一组 Operation 就同步增加正式 contract test；未实现 Operation 不注册、不导出，也不提供 `501` 占位或双轨行为。PLAN-09 在全部 Operation Green 后执行唯一 final exact-set 和生成零漂移检查。

## 4. 数据库验证矩阵

| 检查 | 预期 |
| --- | --- |
| 静态 exact-set | 20 个编号 SQL、每文件一张 `public` 表、无 ALTER/CREATE DATABASE/删除/seed |
| 语法 | PostgreSQL parser 接受全部文件 |
| 空库迁移 | `alembic upgrade head` 成功；第二次无 Schema 漂移 |
| catalog | 排除 `alembic_version` 后表/列/default/null/FK/UQ/CHECK/index exact-set |
| 约束 | 状态、候选键、partial UQ/index 和内联 FK 实际拒绝反例 |
| 安全 | 测试库名与 bucket/prefix 先核对；不指向用户业务数据 |

## 5. HTTP/生成验证矩阵

- 本计划的兼容 fixture 只覆盖 201/202、Problem、ETag、幂等和 Task polling 代表性形状；最终 Operation 的 `operationId/method/path` 由 PLAN-09 断言与 DESIGN-04 §5 exact-set 完全相等。
- 错误为 `application/problem+json`，无 stack/secret/Prompt/签名 URL。
- 创建/重放/版本冲突/异步受理/Task polling 具备成功与失败示例。
- OpenAPI 版本为 3.1；锁定 umi 无法消费时本计划失败并回到 Design，不降级生成第二 Schema。
- PLAN-09 必须验证页面禁止直接导入生成目录，只有 `services/api.ts` 可调用最终生成函数。

## 6. 风险、回滚与 Definition of Done

| 风险 | 结果 | 回滚 |
| --- | --- | --- |
| SQL 与 catalog 不一致 | 阻断 PLAN-04 | 修根 SQL/Design 后重建隔离测试库；不改用户库 |
| 迁移中断 | 测试失败 | 丢弃仅当前 run_id 测试库；正式库只前滚修复 |
| umi 不兼容 3.1 | 阻断 | 回到技术选型，不手写兼容层 |
| 工具链探针被误当正式契约 | contract 漂移 | fixture 只放 tests 且输出临时目录；最终制品只由 PLAN-09 生成 |

完成条件：T-DATABASE-DESIGN-GATE、T-MIGRATION、T-DB-CONSTRAINTS 和公共 T-CONTRACT Green；`make test-migration test-contract-foundation contracts-toolchain-check` 通过；EV-000～002 记录输入版本、命令、退出码和 catalog/兼容探针摘要。完整 `contracts-check` 的零漂移结果由 PLAN-09 关闭。
