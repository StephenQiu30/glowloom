# 数据库定义单一规范与移除 Migration 服务设计规范

## 1. 背景与意图 (Intent & Why)

在当前项目中，`backend/db/migrations/` 目录及增量迁移脚本被意外引入。然而，增量迁移脚本与全量定义并存会导致双重真实来源（Split-brain）：
- 新启动的服务或新环境若使用 `backend/db/schema.sql` 初始化，该文件已包含最新的完整表结构与约束，若再尝试执行增量迁移脚本，会导致表已存在、列冲突或约束重复报错；
- 若新服务依赖迁移服务按序升级，但在多实例部署或冷启动时执行动态 DDL，极易引发并发竞争、锁超时及数据结构不一致；
- 项目核心守卫测试 `backend/tests/db/schema_source_test.go` 强制约束 `backend/` 下仅允许存在 `schema.sql` 与 `data.sql`，增量迁移文件的存在导致自动化测试直接失败。

因此，必须更正项目规范，确立数据库定义文件 `backend/db/schema.sql` 作为唯一的规范性文件，全面禁止 migration 目录及运行时迁移服务。

## 2. 目标与非目标 (Goals & Non-Goals)

### 目标 (Goals)
1. 在核心协作规范 `AGENTS.md` 中新增专有章节 `## 数据库与数据规范`，明确声明 `backend/db/schema.sql` 为唯一结构定义规范，明令禁止 `migrations/` 目录与运行时动态 DDL / 迁移服务。
2. 彻底移除现存的违规目录 `backend/db/migrations/` 及其包含的增量文件 `20261113_canvas_result_target.sql`。
3. 恢复 `backend/tests/db/schema_source_test.go` 的全绿通过，保证工程门禁有效拦截任何新增 SQL 文件的行为。
4. 保证工作区根目录规则与 `glowloom-server` 规范同步。

### 非目标 (Non-Goals)
1. 不修改已有业务表结构与逻辑（`20261113_canvas_result_target.sql` 中的变更已完整并入 `schema.sql`）。
2. 不引入任何第三方迁移库（如 goose, golang-migrate, flyway）。

## 3. 核心约束 (Constraints)
- `backend/db/schema.sql` 为整个仓库唯一的 PostgreSQL 结构定义文件。
- `backend/db/data.sql` 为唯一的预设数据/种子文件。
- 严禁在仓库内提交任何 `migrations/` 目录或增量 SQL 文件。
- 严禁任何服务在启动或运行时向数据库执行 `CREATE TABLE` / `ALTER TABLE` / `DROP` 等动态 DDL。
- 空库必须且只能从 `backend/db/schema.sql` 初始化。
- 存量数据库必须在部署前由管理员进行离线 diff 核验与受控事务升级，严禁运行时自动迁移，严禁对已有库重放空库 SQL。

## 4. 实施方案 (Approach)
采用方案 1：严格落实 Canonical Schema 单一规范，清除违规文件，建立规范条文，并通过已有守卫测试验证。
