# Lanverse

Lanverse 是一个 AI 短剧制作 MVP：让一名内部创作者把获权中文文本转换为剧本、分镜、图片、视频、逐句 TTS、字幕和可播放 MP4，并保证任务可恢复、结果可追溯。

当前仓库只包含治理文件和正式文档，尚未生成应用代码。统一入口见 [`docs/README.md`](docs/README.md)；产品边界见 [MVP目标与范围](docs/requirement/01-MVP目标与范围.md)，模块设计见 [Design 索引](docs/design/README.md)，规格化产品需求见 [PRD 索引](docs/prd/README.md)，数据库物理模型见 [数据库表与迁移设计](docs/design/06-数据库表与迁移设计.md)，专业执行拆分见 [Plan 索引](docs/plans/README.md) 与 [MVP总执行计划](docs/plans/01-MVP总执行计划.md)。

正式交付严格遵循 [`AGENTS.md`](AGENTS.md) 定义的 `Design → PRD → Plan → Acceptance`。当前 Requirement 为 `review`，Design、PRD 和 Plan 为 `draft`；只有逐级接受，并先后取得 `database_design_ready: passed` 与 `implementation_start: passed` 记录后才能创建应用代码，Acceptance 只能在实现完成后建立。

## 获准实现时的唯一目录

```text
Lanverse/
├── sql/       # public schema 的 20 个自包含逐表 SQL
├── backend/   # Python 3.13、FastAPI、asyncpg/逐表 SQL、TaskJob、LangChain Core、MinIO 与 FFmpeg
├── frontend/  # create-next-app、shadcn/Radix、Redux Toolkit、umi-openapi，以及五个工作区
├── docker-compose.yml  # 只运行 frontend/backend-api/backend-worker，复用现有 PostgreSQL/MinIO
└── docs/      # Requirement、Design、PRD、Plan、Acceptance 与 Operations
```

根 [`sql/`](sql/README.md) 是实现前即可评审的数据库 Design artifact；API 与单一 Worker 同属 `backend/`。其他环境只在进入正式范围后新增根目录 `docker-compose-<env>.yml`，生产固定为 `docker-compose-prod.yml`；仓库不创建 `deploy/` 或提交本地 env。以上目录与正式文档中的六个业务模块、20 张应用表（19 张业务事实表 + 1 张幂等技术表）共同构成完整实现清单；数据库 Design 未接受或任一实现准入未通过时不会创建应用脚手架或执行 SQL。
