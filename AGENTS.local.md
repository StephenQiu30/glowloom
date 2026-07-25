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
4. 应用实现进入已接受的 Plan 后，应用根仅允许 `backend/` 与 `frontend/`；后端源码固定在 `backend/src/lanverse/`，API、TaskJob Worker、业务模块和 Provider/MinIO/FFmpeg Adapter 均归入 `backend/`；前端源码固定在 `frontend/src/`。不得创建根 `src/`、`deploy/`、顶层 `apps/`、`packages/` 或独立 Worker 仓库。
5. 默认本地 Compose 位于根 `docker-compose.yml` 且只管理 `frontend/backend-api/backend-worker`；其他环境使用 `docker-compose-<env>.yml`，生产固定 `docker-compose-prod.yml`，仅在对应环境进入正式范围后创建。
6. Compose 和应用复用当前 shell 或用户显式指定的仓库外环境配置中的 PostgreSQL/MinIO；不得扫描、复制、打印或提交用户 secret、`.env`、本地数据库数据或对象存储数据。
7. 数据库物理实现源固定在根 `sql/`：20 个 `NN_<table>.sql` 按 FK 依赖顺序编号，每表一个标准 SQL 文件；PK、UQ、FK、CHECK 直接写入所属表的 `CREATE TABLE`，索引也保存在所属表文件，不设置公共关系或索引 SQL。目标为 `lanverse` 数据库的 `public` schema；Alembic 只按序执行这些根 SQL，不复制 DDL，不使用 ORM、Metadata 或 autogenerate。
8. FastAPI 路由与 Pydantic 是唯一 HTTP 契约源；开发环境启用 Swagger `/docs` 与 `/openapi.json`。前端通过 `LANVERSE_OPENAPI_URL`（本地默认 `http://127.0.0.1:8000/openapi.json`）让 `@umijs/openapi` 直接读取运行中 API 的 Swagger URL，并生成 `frontend/src/services/generated/`；不提交或消费静态 OpenAPI 中间文件。页面只能经手写 RTK Query 层访问，生成目录不得手改，禁止手写第二套 URL、DTO 或降级 OpenAPI。
9. 项目验证命令由根 Makefile 统一暴露；实现前至少提供 `test-architecture`、`test-migration`、`contracts-check`、`test-jobs`、`test-e2e`、`lint`、`typecheck` 和 `build`，并把精确结果写入 Acceptance。
10. 后端采用单一 FastAPI 技术分层：`main.py`、`api/routes/`、`schemas/`、`services/`、`repositories/`、`db/`、`workers/`、`integrations/`、`core/` 与 `domain/`；禁止恢复逐业务模块重复的 `domain/application/infrastructure/transport/public.py` 五层结构，也禁止新增 `other`、运行时 `test` 路由或重复通用工具层。MVP 使用 Task HTTP 轮询，不创建 WebSocket/socket 子系统。
11. `build/`、`data/`、`env/` 不是仓库源码目录：编译产物由工具默认目录生成并忽略，本地模型、MinIO 字节和运行数据使用仓库外现有环境，secret 只来自 shell 或用户显式配置。跨生态编排只放根 Makefile，后端专用辅助命令放 `backend/scripts/`，不预建空目录。
12. 每次任务产生仓库修改时，只显式暂存任务文件，验证后创建符合 `AGENTS.md` 类型规范的提交；最终 `git status --short` 不得残留本任务修改。若用户要求发布到 `main`，推送后还必须确认 `HEAD` 与 `origin/main` 一致。
