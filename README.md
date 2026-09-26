# Lanverse

Lanverse 是一个 AI 短剧制作平台：以已有剧本为起点、以可发布成片为终点，把整部剧解析、角色与场景设定、参考定稿、分镜、全能参考视频生成、配音、剪辑和交付放进同一条可审阅、可恢复、成本透明的生产线；并提供 LibTV 式的无限画布作为探索与精修界面。

> **当前状态（2026-09-26）：** 产品与技术设计已从零重做并完成交叉评审；旧实现（`backend/`、`agent/`、`frontend/` 及旧工程配置）已删除，完整保留在标签 `legacy-2026-09`（`git checkout legacy-2026-09 -- <路径>` 可取回）。新代码按 [BACKLOG](BACKLOG.md) 从 M1 起重建。

## 工作流

```text
整部剧导入 → 结构解析 → 设定集 → 参考定稿 → 分镜 → [关键帧] → 视频（图生视频 / 全能参考）→ 配音 → 剪辑 → 交付
                    每一步：AI 产出候选 → 人工审阅 / 修改 → 选定为正式版本
```

首个版本（MVP）做到“配音”为止，同时提供流水线、画布与对话式 Agent；剪辑与交付在 V2（[PRD-01 §8](docs/prd/01-产品需求文档.md)）。

## 架构概览

```text
Next.js（流水线视图 / 画布 / 审阅 /（V2）时间线）
   │ REST + SSE
Go backend-api（Gin · 命令层 · 领域模块）──启动 / 信号──→ Temporal
   │                                                  ├─ backend-worker（Go 工作流 · 写库 · FFmpeg）
   │                                                  └─ agent-worker（FastAPI 服务 · Agent Harness · 供应商适配器）
PostgreSQL（业务事实 + Outbox）→ backend-relay → Kafka → 通知 / 过期传播 / 成本 / 审计
Redis（会话 · 缓存 · 限流 · 锁 · 实时扇出）      MinIO（媒体对象）
```

- **结构化生产对象是事实源**：剧本、集、场、角色、造型、镜头、剪辑计划都有版本与依赖。
- **所有生成都是 Operation**：能力 + 模式 + 模型 + 参数 + 带用途的输入 → 工作流 → 候选。
- **Temporal 编排全部长流程**，工作流只用 Go 编写；Agent 服务只执行 AI 步骤。
- **模型通过声明式注册表接入**；付费生成报价 → 确认 → 预留 → 执行 → 结算，结果未知先对账。

详见 [DES-01 系统架构设计](docs/design/01-系统架构设计.md)。

## 技术栈

| 层 | 技术 |
| --- | --- |
| 前端 | Next.js、TypeScript、Tailwind CSS、shadcn/ui、Radix UI、ESLint、Prettier、TanStack Query、Zustand、React Flow |
| 后端 | Go：Gin、GORM、golang-migrate、Viper、Zap、Wire、swag、Temporal SDK、go-redis、franz-go、minio-go |
| Agent 服务 | Python：FastAPI、Pydantic、Temporal Python SDK、Agent Harness、httpx |
| 工作流 | Temporal |
| 中间件 | PostgreSQL、Redis、Kafka；对象存储开发用 MinIO、生产用火山引擎 TOS |
| 媒体 | FFmpeg / ffprobe |
| 可观测 | OpenTelemetry、Prometheus、Grafana、Loki |

选型与各中间件职责见 [DES-08 技术选型决策](docs/design/08-技术选型决策.md)，工程约定见 [PROJECT.md](PROJECT.md)。

## 文档

| 文件 | 内容 |
| --- | --- |
| [docs/README.md](docs/README.md) | 文档中心：按生命周期组织的全部文档、编号规则 |
| [BACKLOG.md](BACKLOG.md) | 项目进度与待执行任务：P0、M1～M5 的任务、实现要点、涉及文件与状态 |
| [PRD-01 产品需求文档](docs/prd/01-产品需求文档.md) | 产品目标、用户、场景、产品决策、版本规划、成功指标 |
| [PRD-02 LibTV 能力调研与核心能力评估](docs/prd/02-LibTV能力调研与核心能力评估.md) | 竞品能力与实现设计、核心能力取舍、顶层架构要求 |
| [REQ-01 功能需求总表](docs/requirement/01-功能需求总表.md) | 全部需求编号与优先级；功能文件 REQ-06～REQ-33、REQ-35～REQ-39（MVP）与 REQ-34（V2 需求池）写明用户故事、规则与验收标准；对应的功能设计为 DES-09～DES-41 |
| [REQ-02 非功能需求规格](docs/requirement/02-非功能需求规格.md) | 性能、可靠性、安全、备份恢复、可观测、AI 质量、合规的可度量目标 |
| [REQ-03 业务流程与用例模型](docs/requirement/03-业务流程与用例模型.md) | 参与者、用例、业务阶段、状态模型 |
| [REQ-04 术语表](docs/requirement/04-术语表.md) · [REQ-05 界面与交互需求](docs/requirement/05-界面与交互需求.md) | 统一术语；信息架构、页面与关键交互 |
| [DES-01 系统架构设计](docs/design/01-系统架构设计.md) | 顶层原则、领域模型、Operation、Temporal 工作流、Agent Harness、事件、画布、扩展点 |
| [DES-08 技术选型决策](docs/design/08-技术选型决策.md) | 前端、后端、Agent、工作流、中间件、模型的明确选型 |
| [PLN-01 实施路线与交付计划](docs/plan/01-实施路线与交付计划.md) | P0 与 M1–M5、验收、人力、风险 |
| [PROJECT.md](PROJECT.md) | 仓库结构、目录与编码约定、质量门禁 |
| [DESIGN.md](DESIGN.md) | 视觉与交互规范 |
| [AGENTS.md](AGENTS.md) | 协作与 Git 规则 |

## 开发

工程底座（M1）搭建中：三端已初始化最小骨架（健康检查 + 质量门禁）。本机直接启动三端进程，并为后续平台层接入配置已运行的本机中间件；配置写在根目录 `.env`（键名样例见 `.env.example`）。

| 目录 | 技术栈 | 本地启动 | 健康检查 |
| --- | --- | --- | --- |
| `backend/` | Go 1.26 · Gin · Viper · Zap | `cd backend && LV_ENV_FILE=../.env go run ./cmd/lanverse --role=api` | `GET :8080/healthz` |
| `agent/` | Python 3.12 · uv · FastAPI · pydantic-settings | `cd agent && uv run --env-file ../.env uvicorn app.main_api:create_app --factory --port 8090` | `GET :8090/internal/health` |
| `frontend/` | Next.js 16 · React 19 · TypeScript strict · Tailwind 4 · shadcn/ui（Radix） | `cd frontend && node --env-file=../.env "$(command -v corepack)" pnpm exec next dev` | `GET :3000/healthz` |

```bash
pg_isready -h 127.0.0.1 -p 5432
redis-cli -h 127.0.0.1 ping
curl -fsS http://127.0.0.1:9000/minio/health/live
temporal operator cluster health --address 127.0.0.1:7233
"$(brew --prefix kafka)/bin/kafka-broker-api-versions" --bootstrap-server 127.0.0.1:9092
```

各进程在独立终端执行上表命令。前置工具：Go 1.26、uv、Node.js 24 + Corepack / pnpm；Go 门禁工具 `goimports`、`golangci-lint`（v2）、`govulncheck` 通过 `go install` 安装。后续容器化部署保留两份独立 Compose 文件：`docker-compose.yml` 定义应用，`docker-compose-env.yml` 定义 PostgreSQL、Redis、Kafka、MinIO、Temporal 及管理界面。本地开发不通过 Compose 启动环境。详细步骤见 [OPS-01](docs/operation/01-环境与部署.md#6-本地开发环境)。

旧实现的代码与运行方式见标签 `legacy-2026-09`（如 `git show legacy-2026-09:README.md`）。
