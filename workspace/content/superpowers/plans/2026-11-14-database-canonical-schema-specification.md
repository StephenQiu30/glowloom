# 数据库定义单一规范与移除 Migration 服务实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 更正项目规范，确立 backend/db/schema.sql 为数据库结构的唯一规范性文件，明确禁止 migration 目录与运行时迁移服务，彻底清理违规 migration 目录并恢复门禁测试全绿。

**Architecture:** 以声明式 Canonical Schema（backend/db/schema.sql）作为唯一数据源，在 AGENTS.md 建立强制性数据库规范，通过自动化守卫测试拦截非法 SQL 文件，杜绝运行时 DDL 引发的数据与结构不一致。

**Tech Stack:** Markdown, Go (testing)

**Spec:** docs/superpowers/specs/2026-11-14-database-canonical-schema-specification-design.md

## Global Constraints

- `backend/db/schema.sql` 是仓库唯一的 PostgreSQL 结构定义文件。
- `backend/db/data.sql` 是仓库唯一的公共预设/种子数据文件。
- 严禁在仓库内创建 `migrations/` 目录或增量 SQL 迁移脚本。
- 严禁任何服务在启动或运行时向数据库执行任何动态 DDL / 自动 migration 服务。
- 空库必须且只能从 `backend/db/schema.sql` 一次性初始化。
- 存量数据库升级必须在部署前由管理员离线基于 `schema.sql` diff 受控执行，严禁直接重放空库 SQL。
- 必须通过 `backend/tests/db/schema_source_test.go` 自动化测试。

## Review Focus

- 确认清除违规文件后 `backend/db/` 下仅存 `schema.sql` 与 `data.sql`。
- 确认 `backend/db/schema.sql` 中已完整包含 `target_content_version` 及对应约束，无需保留任何增量脚本。
- 确认规范已写入 `glowloom-server/AGENTS.md`，并在顶级工作区创建 `AGENTS.md` 引用保持一致。
- 确认 `backend/tests/db/schema_source_test.go` 测试 100% 通过。

---

### Task 1: 清理违规 Migration 目录与增量 SQL 文件

**Files:**
- Delete: `glowloom-server/backend/db/migrations/20261113_canvas_result_target.sql`
- Delete: `glowloom-server/backend/db/migrations/`

- [ ] **Step 1:** 删除违规迁移文件和目录 `glowloom-server/backend/db/migrations`。
- [ ] **Step 2:** 验证 `glowloom-server/backend/db/` 仅包含 `schema.sql` 和 `data.sql`。
- [ ] **Step 3:** 运行守卫测试 `go test ./tests/db -v -run TestBackendSQLHasCanonicalSchemaAndPresetData` 确认转绿。

---

### Task 2: 在 AGENTS.md 中确立数据库定义单一规范

**Files:**
- Modify: `glowloom-server/AGENTS.md`
- Create/Modify: `AGENTS.md` (工作区顶级目录)

- [ ] **Step 1:** 在 `glowloom-server/AGENTS.md` 中添加 `## 数据库与数据规范` 章节，详细规定唯一规范文件原则、零增量迁移原则、禁止运行时 DDL 原则与离线核验原则。
- [ ] **Step 2:** 在工作区根目录 `/Users/stephenqiu/Desktop/StephenQiu/Glowloom/AGENTS.md` 建立对应规范或软链/转发，确保顶层工作区与子模块协作规则一致。
- [ ] **Step 3:** 核对 `PROJECT.md` 等相关描述，确保用词与规范统一。

---

### Task 3: 整体门禁与完整性回归验证

**Files:**
- Test: `glowloom-server/backend/tests/db/schema_source_test.go`
- Test: `glowloom-server/backend/tests/db/schema_integration_test.go` 等相关 db 单元测试

- [ ] **Step 1:** 在 `glowloom-server/backend` 运行 `go test ./tests/db/... -v`，确保所有 db 静态断言与规范测试全部通过。
- [ ] **Step 2:** 执行 `git status --short` 确认工作区修改清晰合规。
