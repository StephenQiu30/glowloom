---
layer: Design
doc_type: Architecture Decision Record
doc_no: DESIGN-01
title: MVP技术选型与范围
status: accepted
version: 1.0.1
owner: Lanverse
audience: [Product, Architecture, Frontend, Backend, QA, Operations]
feature_area: AI 短剧制作 MVP
purpose: 定义首个可执行闭环的范围、技术栈、运行边界和回滚规则
canonical_path: docs/design/01-MVP技术选型与范围.md
inputs: [REQ-01至REQ-08]
outputs: [MVP 实现 allowlist, 技术栈, 运行单元, 迁移与回滚原则]
triggers: [MVP 范围变化, 技术族变化, 事实源变化, 任务编排变化, 部署边界变化]
updated: 2026-07-25
downstream: [DESIGN-02至DESIGN-13, PRODUCT-01至PRODUCT-07, PLAN-01至PLAN-09, ACCEPTANCE-01]
---

# MVP技术选型与范围

## 1. 决策

Lanverse MVP 是内部验证用的单实例 Web 应用，只交付一个 30～60 秒、6～10 镜头、9:16 单集闭环：输入文本→结构化剧本→分镜→图片/视频与最小 TTS→人工采用候选→字幕/音轨合成→MP4 下载。

采用单一 Git monorepo、Next.js 前端、Redux Toolkit 状态层、FastAPI Python 模块化单体、PostgreSQL 业务事实与 TaskJob 租约、LangChain Core Python AI 接入、MinIO 私有对象存储和 FFmpeg 媒体处理。应用实现仅位于 `backend/`、`frontend/`；Compose 文件直接位于仓库根。

## 2. 技术基线

| 关注点 | MVP 选择 | 最迟确定点 |
| --- | --- | --- |
| 语言/仓库 | 后端 Python 3.13 + uv；前端 TypeScript strict + Node.js 24 LTS + pnpm；分别提交 `uv.lock` 与 `pnpm-lock.yaml` | database_design_ready 通过后、PLAN-02 脚手架执行时锁定精确 patch 与安装命令 |
| Web | React、Next.js 16.2.11+ Active LTS 安全补丁、App Router、Tailwind CSS；官方 create-next-app 脚手架 | 支持浏览器与构建版本 |
| 组件 | shadcn/ui CLI 显式 `--template next --preset nova --base radix`、统一 `radix-ui` Primitives、`components.json` | PLAN-02 固定 `radix-nova`、preset code `b2fA`/version `b`、Lucide 与生成文件/依赖清单；漂移即失败 |
| 前端状态 | `@reduxjs/toolkit`、`react-redux`、RTK Query；`@umijs/openapi` 从 Swagger/OpenAPI 生成唯一请求与 DTO | Store/生成服务封装边界与零漂移命令 |
| API | FastAPI `standard-no-fastapi-cloud-cli`、Pydantic v2、REST/JSON；自动 Swagger UI，确定性导出 OpenAPI 3.1，任务 2 秒轮询 | 本地 docs 开关、契约校验与 umi-openapi 兼容测试 |
| 数据 | asyncpg 参数化 SQL、按表命名的 PostgreSQL `.sql`、Alembic 版本执行 | DESIGN-06 accepted 后锁定驱动/迁移与 catalog 测试；不使用 ORM/Metadata/autogenerate |
| 长任务 | PostgreSQL `TaskJob` 租约、`FOR UPDATE SKIP LOCKED`、心跳/过期恢复、单一 Worker | 租约、退避、稳定请求键、对账与故障注入；MVP 不使用 Temporal/Celery/Redis |
| 媒体 | MinIO 私有对象存储与 `minio` Python SDK；FFmpeg/ffprobe | 现有 MinIO endpoint/bucket、SDK、工具版本、object_key 策略和资源上限 |
| AI | `langchain-core` 模型/Runnable 接口、能力端口、支持多 Provider/模型的 `AiModelRegistry` 与确定性 Mock；LangSmith tracing 关闭 | PLAN-09 P09-T009 真实 smoke 前：模型配置目录、默认路由、凭据、额度和 Provider SDK；依赖图不得含 `langgraph*`，网络只放行批准 Provider |
| 质量 | 后端 pytest/pytest-asyncio/HTTPX/Ruff/mypy；前端 Vitest/Testing Library/Playwright；结构化日志 | 命令、门禁、覆盖范围和证据位置 |

依赖 allowlist 仅包含上表技术族；PLAN-02 必须以两个生态各自唯一的 lockfile 和依赖扫描落实为可复现基线。Python 3.13 与 asyncpg/MinIO 使用当前稳定兼容版本；Alembic 只执行受审 SQL，不生成 Schema。

选型依据为 [FastAPI 多文件应用](https://fastapi.tiangolo.com/tutorial/bigger-applications/)、[FastAPI Swagger 文档配置](https://fastapi.tiangolo.com/tutorial/metadata/#docs-urls)、[asyncpg](https://magicstack.github.io/asyncpg/current/)、[PostgreSQL SKIP LOCKED](https://www.postgresql.org/docs/current/sql-select.html)、[Alembic operation API](https://alembic.sqlalchemy.org/en/latest/ops.html)、[umi-openapi](https://www.npmjs.com/package/@umijs/openapi)、[Next.js 官方脚手架](https://nextjs.org/docs/app/getting-started/installation)、[shadcn CLI](https://ui.shadcn.com/docs/cli)与[Redux Toolkit App Router 指南](https://redux-toolkit.js.org/usage/nextjs)。

### 2.1 开源实现复审取舍

- [Jellyfish 固定提交](https://github.com/Forget-C/Jellyfish/tree/a9678194ddf2d9be3ccbe78d4287d87d5089e123)提供分镜准备、生成工作室、任务事实分离和 OpenAPI 生成客户端的边界证据；Lanverse 按本 ADR 的技术基线实现这些边界。
- [Toonflow 固定提交](https://github.com/HBAI-Ltd/Toonflow-app/tree/bc61ec7a1b5df31293b286981a5f4ad4635464ee)提供来源→剧本→生产最短纵向工作流的领域证据；Lanverse 按本 ADR 的六模块和运行单元实现该链路。
- 两个项目都作为领域边界和交互证据，不作为生产可靠性背书；Lanverse 的版本、幂等、恢复、私有媒体和验收仍以本组 Requirement/Design 为准。

## 3. 运行单元

| 单元 | 责任 |
| --- | --- |
| `frontend` | 五个工作台 Feature、生成客户端和 Task 2 秒轮询，不保存正式事实 |
| `backend-api` | 校验输入、幂等/并发、业务命令、查询、任务轮询和短期下载授权 |
| `backend-worker` | TaskJob 领取/续租、Provider 调用/对账、下载、探测与渲染 |
| `postgres` | 唯一业务/恢复事实、Task/Attempt/TaskJob/TaskEvent 和版本记录；复用现有本地实例 |
| `minio` | 私有媒体字节、派生物和成片；URL 不是业务事实 |

数据库迁移是受控的一次性命令，不是常驻服务。API 和 Worker 必须能独立停止、启动和构建；Worker 用 `Task.type` 分派到具名 Handler，不建立 Workflow 框架。

## 4. 选择理由

- PostgreSQL、不可变快照和明确版本保证内容、候选与交付可追溯。
- PostgreSQL TaskJob 与 Attempt 租约直接服务“任务在进程重启和外部超时后仍能继续”，无需新增编排服务。
- 单仓库、模块化单体和两个后端进程减少首轮部署与契约协调成本。
- MinIO 与 FFmpeg 把媒体字节和高资源处理移出控制面。
- LangChain Core 统一文本模型调用，`AiModelRegistry` 让多个已批准 Provider/模型配置并存；能力端口继续隔离图片、视频和 TTS 的 Provider 差异。完整 `langchain` 当前会传递引入 LangGraph，因此不进入 MVP 基线。
- Temporal 更适合跨服务补偿、长周期人机流程和多 Worker 版本治理；当前单体单 Worker MVP 采用 PostgreSQL 租约更小且已满足恢复需求。FastAPI BackgroundTasks 不持久，Celery/Dramatiq 又会增加 Redis，因此都不进入 MVP；达到上述复杂度信号后再立 ADR 评估 Temporal。

## 5. MVP 实现 allowlist

- 一个部署边界内的内部操作者创建一个 Project，并由系统原子创建唯一 Episode。
- 操作者粘贴中文正文并声明来源权利，生成、编辑和确认结构化剧本、创作资产与分镜。
- 文本、图片、视频和 TTS 从已批准模型配置目录解析，每类必需能力至少一个配置且可并存多个；每个 Task 固定一个配置，测试使用确定性 Mock。
- 每个异步命令产生可恢复的 Task/Attempt；媒体结果先成为 Candidate，再由操作者形成 Adoption。
- 服务端使用已采用视频、逐句 TTS 和已确认字幕生成 MP4、SRT 与 JSON Manifest，并通过 ffprobe 质检。
- 来源权利声明、服务端密钥、私有对象、短期授权和日志脱敏是交付闭环的必需护栏；交付用途固定为受控内部评审。

任何代码目录、API、数据表、后台进程或验收项都必须直接服务上述六项之一。

## 6. 后果与风险

- 单实例、单操作者边界固定为本机 loopback：根 Compose 只有 frontend、backend-api 发布端口且全部绑定 `127.0.0.1`，Worker 不发布端口；PostgreSQL/MinIO 复用既有本地环境，不由 Compose 管理。运行时测试必须拒绝应用的非 loopback 访问。
- 单 Worker 按[《06 MVP质量标准》](../requirement/06-MVP质量标准.md)的并发、超时和资源上限执行 FFmpeg/Provider Job Handler；容量不足时停止领取新任务并保留全部已受理任务。
- TaskJob 领取、心跳、租约过期、取消、unknown 对账必须有真实 PostgreSQL 故障注入证据；租约并不替代 Provider 幂等。
- 多 Provider/模型配置会增加能力差异和成本风险；MVP 由版本化默认路由在 Task 受理前选定配置，受理后显式失败并允许用户创建新 Task，不自动切换或 fallback。

## 7. 迁移与回滚规则

当前无应用数据；[数据库表与迁移设计](06-数据库表与迁移设计.md)必须先于任何脚手架或源码转为 `accepted`，首次 Alembic 迁移再从 `0001_mvp` 建立。数据库变更遵循 expand→switch→contract；部署回滚只回退制品和入口，不删除已提交的 Task、Attempt、快照、媒体和 Delivery。

实现若偏离本 ADR 的 allowlist、技术基线、运行单元或成片规格，必须先更新 Requirement 与 Design 并重新通过准入门禁，不能在代码中预埋旁路。

## 8. Design 验收标准

- AC-ADR-001-001：[02 MVP总体架构](02-MVP总体架构.md)、[03 任务执行与媒体处理](03-任务执行与媒体处理.md)、[04 接口数据与项目结构](04-接口数据与项目结构.md)、[05 AI短剧端到端制作流程](05-端到端制作流程.md)与[06 数据库表与迁移设计](06-数据库表与迁移设计.md)只使用本决策的技术族、运行单元和单闭环范围。
- AC-ADR-001-002：所有代码目录、API、数据表和验收项都可定位到 §5 allowlist 与六个模块之一。
- AC-ADR-001-003：每类必需能力至少一个真实模型配置，全部启用配置的 Provider/模型/凭据/额度和默认路由均有明确的 PLAN-09 P09-T009 真实 smoke 前关闭条件，Mock 不伪装真实成片证据。
- AC-ADR-001-004：Worker 重启、重复请求、供应状态未知和渲染失败均有不丢事实、不重复副作用的恢复设计。
- AC-ADR-001-005：目标应用目录仅包含 `backend/frontend`，Compose 位于根目录，不存在 `deploy/`，且 Acceptance 未在实现前创建。
