# AGENTS.local.md

本文件用于记录放在具体项目中的局部规范性配置，与 `AGENTS.md` 中的全局协作规则进行区分。

## 使用边界

1. `AGENTS.md` 存放长期稳定的 Codex 全局规则、角色协作原则和交付格式。
2. `AGENTS.local.md` 存放当前项目特有的规范、路径、命令、环境约束和临时协作约定。
3. 当局部规范与全局规则冲突时，应优先确认项目上下文，并以更具体、更贴近当前项目的规则为准。

## 当前项目规范

1. 本项目内的角色配置放在 `.codex/agents/` 目录。
2. 本项目内的可复用流程放在 `.codex/skills/` 目录。
3. 本项目不再维护额外规格配置；`docs/` 只允许 `.md` 正式文档、阶段记录和对应 README 索引，禁止放入 JSON、YAML、Python、生成物或其他非 Markdown 文件。
4. 应用实现进入已接受的 Plan 后，应用根仅允许 `backend/` 与 `frontend/`；API、TaskJob Worker 与供应能力适配器均归入 `backend/`，不得创建 `deploy/`、顶层 `apps/`、`packages/` 或独立 Worker 仓库。
5. 默认本地 Compose 位于根 `docker-compose.yml` 且只管理 `frontend/backend-api/backend-worker`；其他环境使用 `docker-compose-<env>.yml`，生产固定 `docker-compose-prod.yml`，仅在对应环境进入正式范围后创建。
6. Compose 和应用复用当前 shell 或用户显式指定的仓库外环境配置中的 PostgreSQL/MinIO；不得扫描、复制、打印或提交用户 secret、`.env`、本地数据库数据或对象存储数据。
7. 数据库物理实现源固定在根 `sql/`：20 个 `NN_<table>.sql` 按 FK 依赖顺序编号，每表一个标准 SQL 文件；PK、UQ、FK、CHECK 直接写入所属表的 `CREATE TABLE`，索引也保存在所属表文件，不设置公共关系或索引 SQL。目标为 `lanverse` 数据库的 `public` schema；Alembic 只按序执行这些根 SQL，不复制 DDL，不使用 ORM、Metadata 或 autogenerate。
8. FastAPI OpenAPI artifact 固定为 `backend/openapi/openapi.json`；前端使用 `frontend/openapi2ts.config.ts` 与 `@umijs/openapi` 生成 `frontend/src/services/generated/`，页面经手写 RTK Query 层访问，禁止手写第二套 URL 或 DTO。
9. 项目验证命令由根 Makefile 统一暴露；实现前至少提供 `test-architecture`、`test-migration`、`contracts-check`、`test-jobs`、`test-e2e`、`lint`、`typecheck` 和 `build`，并把精确结果写入 Acceptance。
