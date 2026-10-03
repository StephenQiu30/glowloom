# Lanverse

Lanverse 是一个 AI 短剧制作平台：以已有剧本为起点、以可发布成片为终点，把整部剧解析、角色与场景设定、参考定稿、分镜、全能参考视频生成、配音、剪辑和交付放进同一条可审阅、可恢复、成本透明的生产线；并提供复用 BeefTV 核心与设计的无限画布作为探索与精修界面。

工作台采用正式项目、媒体、画布与 Operation 合同。真实供应商生成、审核、费用与完整产品流程仍须逐项验收，未决范围见 [BACKLOG](BACKLOG.md)。

## 工作流

```text
整部剧导入 → 结构解析 → 设定集 → 参考定稿 → 分镜 → [关键帧] → 视频（图生视频 / 全能参考）→ 配音 → 剪辑 → 交付
                    每一步：AI 产出候选 → 人工审阅 / 修改 → 选定为正式版本
```

首个版本（MVP）做到“配音”为止，同时提供流水线、画布与对话式 Agent；剪辑与交付在 V2（[PRD-01 §8](docs/prd/01-产品需求文档.md)）。

## 架构概览

```text
Next.js（工作台 / 表单创作 / 画布 / 审阅 / 时间线）
   │ REST（SSE 公开路由待接入）
Go backend-api（Gin · 命令层 · 领域模块）──启动 / 信号──→ Temporal
   │                                                  ├─ backend-worker（Go 工作流 · 写库 · FFmpeg）
   │                                                  └─ 供应商 / 审核 / Skill Activity（执行端待补，M1-12）
PostgreSQL（业务事实 + Outbox）→ backend-relay → Kafka → 通知 / 过期传播 / 成本 / 审计
Redis（会话 · 缓存 · 限流 · 锁 · 实时扇出）      MinIO（媒体对象）
```

- **结构化生产对象是事实源**：剧本、集、场、角色、造型、镜头、剪辑计划都有版本与依赖。
- **所有生成都是 Operation**：能力 + 模式 + 模型 + 参数 + 带用途的输入 → 工作流 → 候选。
- **Temporal 编排全部长流程**，工作流只用 Go 编写；AI 执行端待由 Go 承接。
- **模型通过声明式注册表接入**；付费生成报价 → 确认 → 预留 → 执行 → 结算，结果未知先对账。

详见 [DES-01 系统架构设计](docs/design/01-系统架构设计.md)。

## 技术栈

| 层     | 技术                                                                                                                                       |
| ------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| 前端   | Next.js、TypeScript、Tailwind CSS、shadcn/ui、Radix UI、ESLint、Prettier、TanStack Query、Zustand；无限画布复用 BeefTV 的 DOM/SVG 交互核心 |
| 后端   | Go：Gin、GORM、Viper、Zap、Wire、swag、Temporal SDK、go-redis、franz-go、minio-go                                                          |
| 工作流 | Temporal                                                                                                                                   |
| 中间件 | PostgreSQL、Redis、Kafka；对象存储开发用 MinIO、生产用火山引擎 TOS                                                                         |
| 媒体   | FFmpeg / ffprobe                                                                                                                           |
| 可观测 | OpenTelemetry、Prometheus、Grafana、Loki                                                                                                   |

选型与各中间件职责见 [DES-08 技术选型决策](docs/design/08-技术选型决策.md)，工程约定见 [PROJECT.md](PROJECT.md)。

工作台和画布的职责、数据、接口与失败路径见 [工作台设计](docs/design/工作台设计.md)，生成与恢复合同见 [生成执行设计](docs/design/生成执行设计.md)。第三方固定来源及改写范围见 [第三方代码声明](THIRD_PARTY_NOTICES.md)。`/canvas` 提供项目入口，`/projects/{UUID}/canvas` 操作正式项目画布。

## 文档

| 文件                                                                                                            | 内容                                                                                                                                                    |
| --------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [docs/README.md](docs/README.md)                                                                                | 文档中心：产品、需求、设计、计划、测试与运维文档入口                                                                                                    |
| [BACKLOG.md](BACKLOG.md)                                                                                        | 待执行任务与未决范围：P0、M1～M5 的任务、依赖和验收边界                                                                                                 |
| [PRD-01 产品需求文档](docs/prd/01-产品需求文档.md)                                                              | 产品目标、用户、场景、产品决策、版本规划、成功指标                                                                                                      |
| [工作台设计](docs/design/工作台设计.md)                                                                         | 工作台、画布、素材工具、时间轴与导演台的稳定合同                                                                                                        |
| [REQ-01 功能需求总表](docs/requirement/01-功能需求总表.md)                                                      | 全部需求编号与优先级；功能文件 REQ-06～REQ-33、REQ-35～REQ-39（MVP）与 REQ-34（V2 需求池）写明用户故事、规则与验收标准；对应的功能设计为 DES-09～DES-41 |
| [REQ-02 非功能需求规格](docs/requirement/02-非功能需求规格.md)                                                  | 性能、可靠性、安全、备份恢复、可观测、AI 质量、合规的可度量目标                                                                                         |
| [REQ-03 业务流程与用例模型](docs/requirement/03-业务流程与用例模型.md)                                          | 参与者、用例、业务阶段、状态模型                                                                                                                        |
| [REQ-04 术语表](docs/requirement/04-术语表.md) · [REQ-05 界面与交互需求](docs/requirement/05-界面与交互需求.md) | 统一术语；信息架构、页面与关键交互                                                                                                                      |
| [DES-01 系统架构设计](docs/design/01-系统架构设计.md)                                                           | 顶层原则、领域模型、Operation、Temporal 工作流、Agent Harness、事件、画布、扩展点                                                                       |
| [DES-08 技术选型决策](docs/design/08-技术选型决策.md)                                                           | 前端、后端、Agent、工作流、中间件、模型的明确选型                                                                                                       |
| [PLN-01 实施路线与交付计划](docs/plan/01-实施路线与交付计划.md)                                                 | P0 与 M1–M5、验收、人力、风险                                                                                                                           |
| [PROJECT.md](PROJECT.md)                                                                                        | 仓库结构、目录与编码约定、质量门禁                                                                                                                      |
| [DESIGN.md](DESIGN.md)                                                                                          | 视觉与交互规范                                                                                                                                          |
| [AGENTS.md](AGENTS.md)                                                                                          | 协作与 Git 规则                                                                                                                                         |

## 开发

数据库结构唯一事实源为 [`backend/db/schema.sql`](backend/db/schema.sql)，统一维护当前已实现的表、索引、约束、触发器、函数、应用权限与动态分区；DES-02 只保留概念设计和未实施规划。新空业务库由表所有者执行 `psql "$LV_SCHEMA_DB_DSN" -X --single-transaction -v ON_ERROR_STOP=1 -f backend/db/schema.sql`，由调用方以单事务初始化，管理员须先预置 `lanverse_app NOLOGIN NOSUPERUSER` 权限角色；Outbox、审计与供应商调用以执行时 UTC 当前月准备当前及未来三个月分区。既有业务库须审阅与实例对应的增量升级，不重跑 Schema 或重建覆盖。初始化和升级步骤见 [DES-02 §10](docs/design/02-领域与数据模型.md#10-schema-事实源初始化与升级) 与 [OPS-01 §7](docs/operation/01-环境与部署.md#7-初始化与种子数据)。

工程底座及业务接线按 [BACKLOG](BACKLOG.md) 持续交付。本机直接运行 Go 与 Next.js，连接已运行的本机中间件。配置写在根目录 `.env`（键名样例见 `.env.example`），浏览器地址必须与 `LV_PUBLIC_ORIGIN` 一致。

| 目录        | 技术栈                                                                      | 本地启动                                                             | 健康检查            |
| ----------- | --------------------------------------------------------------------------- | -------------------------------------------------------------------- | ------------------- |
| `backend/`  | Go 1.26 · Gin · Viper · Zap                                                 | `cd backend && LV_ENV_FILE=../.env go run ./cmd/lanverse --role=api` | `GET :8080/healthz` |
| `frontend/` | Next.js 16 · React 19 · TypeScript strict · Tailwind 4 · shadcn/ui（Radix） | `cd frontend && pnpm exec next dev`                                  | `GET :3000/healthz` |

原 `agent` / `agent.mock` 队列当前没有应用提供的执行 Worker；Go 工作流仍保留这些协议引用，相关任务可能等待或超时。API 健康/就绪检查不证明供应商执行可用；Go 承接完成前，不能将生成、凭据测试、审核或 Skill 运行计为验证通过。本地开发不启动 Compose。

Go Worker 和事件 Relay 可在独立终端运行：`cd backend && LV_ENV_FILE=../.env go run ./cmd/lanverse --role=worker --queues=flow,media`、`cd backend && LV_ENV_FILE=../.env go run ./cmd/lanverse --role=relay`。`flow` 编排已登记的工作流，`media` 执行私有媒体接管、FFmpeg 音视频导出与本机字幕转写 Activity。Relay 投递 Outbox，投影已登记事件并消费审核后的审计摘要；新环境须预建 OPS-01 列出的全部主题。公开 SSE、真实供应商及完整生成验收仍以对应 BACKLOG 任务为准。本机字幕服务固定来源、模型摘要与启动步骤见 [OPS-01 §6.1](docs/operation/01-环境与部署.md#61-本机字幕转写服务)。

Go 继续提供单一工作区身份，复用既有组织和真实项目数据；公开 `/api/auth/*` 仍已移除，没有认证切换开关。当前可从 `:3000/projects` 创建、编辑、归档和恢复项目，并进入正式画布；项目生命周期合同见 [DES-13](docs/design/13-项目管理.md)。角色与组织授权仍由服务端核验。

Worker 与 Relay 分别在根目录 `.env` 的 `LV_WORKER_HEALTH_ADDR`、`LV_RELAY_HEALTH_ADDR` 提供 `GET /healthz`（样例端口 8081、8082）；该接口只表示进程正在运行，任务处理状况仍需检查 Temporal Worker 与 Outbox 积压。

本机需要合并运行现有 Go 角色时可执行 `cd backend && LV_ENV_FILE=../.env go run ./cmd/lanverse --role=all`，它启动 API、`flow,media` Worker 和 Relay；任一角色失败时会停止并等待其余角色退出。

数据库结构按目标 Schema 准备完成并部署 `flow` Worker 后，执行 `cd backend && LV_ENV_FILE=../.env go run ./cmd/lanverse temporal setup`，为现有命名空间安装两项清理 Schedule 与五分钟一次的 `quote-expiry`；不会安装尚未实现完整依赖的 `partition-maintain`。

Go API 启动时必须通过根目录 `.env` 中的 `LV_DB_DSN` 连接可用的本机业务库，并配置 `LV_REDIS_URL` 与 `LV_TEMPORAL_ADDR`、`LV_TEMPORAL_NAMESPACE`；`.env.example` 的数据库密码只是占位值，不能直接用于连接。`/healthz` 检查进程存活，`/readyz` 检查 PostgreSQL、Redis、Temporal、命名空间与对象存储；依赖暂不可达时 API 仍运行，就绪探针返回 503。本机首次使用 `lanverse-local` 命名空间时需在已运行的 Temporal 中创建，步骤见 [OPS-01](docs/operation/01-环境与部署.md#6-本地开发环境)。Relay 需要 `LV_KAFKA_BROKERS`、`LV_REDIS_URL` 和 `backend/db/schema.sql` 中已实现的 Outbox、processed-event、audit-log 和账号结构。部署方须预先创建 `lanverse.operation.status_changed.v1`、`lanverse.audit.recorded.v1` 与 `lanverse.identity.user_changed.v1` 三个 Kafka 主题。

API 启动后打开 [Swagger UI](http://127.0.0.1:8080/swagger/index.html)，页面与静态资源由 Go API 提供，读取同源 `/swagger/doc.json`。接口的唯一来源是 Handler 的 swag 注解与 DTO；修改后在 `backend/` 执行 `go generate -run 'swag' ./internal/app`，固定使用 swag `v1.16.6` 自动生成 `backend/docs`，随后重新构建并启动 API。不得手工维护生成的 Go、JSON 或 YAML；CI 重新生成并检查漂移及业务路由覆盖。UI 可浏览参数、响应与模型并调试读取接口；写接口仍校验配置的前端 Origin 和 UUID `Idempotency-Key`，Swagger 页面不绕过该规则。

```bash
pg_isready -h 127.0.0.1 -p 5432
redis-cli -h 127.0.0.1 ping
curl -fsS http://127.0.0.1:9000/minio/health/live
temporal operator cluster health --address 127.0.0.1:7233
"$(brew --prefix kafka)/bin/kafka-broker-api-versions" --bootstrap-server 127.0.0.1:9092
```

各进程在独立终端执行上表命令。前置工具：Go 1.26、Node.js 24 + Corepack / pnpm、FFmpeg / ffprobe；Go 门禁工具 `goimports`、`golangci-lint`（v2）、`govulncheck` 通过 `go install` 安装。后续容器化部署保留两份独立 Compose 文件：`docker-compose.yml` 定义应用，`docker-compose-env.yml` 完整定义 PostgreSQL、Redis、Kafka、MinIO、Temporal 及管理界面。本机开发使用服务管理器启动的本机依赖，不通过 Compose 启动环境。详细步骤见 [OPS-01](docs/operation/01-环境与部署.md#6-本地开发环境)。
