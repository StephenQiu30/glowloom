# Lanverse

Lanverse 是一个 AI 短剧制作平台：以已有剧本为起点、以可发布成片为终点，把整部剧解析、角色与场景设定、参考定稿、分镜、全能参考视频生成、配音、剪辑和交付放进同一条可审阅、可恢复、成本透明的生产线；并提供 LibTV 式的无限画布作为探索与精修界面。

> **当前状态（2026-09-25）：** 产品与技术设计已从零重做，处于评审阶段。仓库中的 `backend/`、`agent/`、`frontend/` 代码是旧实现，不符合新设计，处置方式见 [0401 第 5 节](docs/plan/0401-实施路线与交付计划.md#5-现有代码的处置已确认方案-a)。

## 工作流

```text
整部剧导入 → 结构解析 → 设定集 → 参考定稿 → 分镜 → [关键帧] → 视频（图生视频 / 全能参考）→ 配音 → 剪辑 → 交付
                    每一步：AI 产出候选 → 人工审阅 / 修改 → 选定为正式版本
```

## 架构概览

```text
Next.js（流水线视图 /（V1）画布 / 审阅 / 时间线）
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

详见 [0301 系统架构设计](docs/design/0301-系统架构设计.md)。

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

选型与各中间件职责见 [0308 技术选型决策](docs/design/0308-技术选型决策.md)，工程约定见 [PROJECT.md](PROJECT.md)。

## 文档

| 文件 | 内容 |
| --- | --- |
| [docs/README.md](docs/README.md) | 文档中心：按生命周期组织的全部文档、编号规则 |
| [0101 产品需求文档](docs/prd/0101-产品需求文档.md) | 产品目标、用户、场景、产品决策、版本规划、成功指标 |
| [0102 LibTV 能力调研与核心能力评估](docs/prd/0102-LibTV能力调研与核心能力评估.md) | 竞品能力与实现设计、核心能力取舍、顶层架构要求 |
| [0201 功能需求规格](docs/requirement/0201-功能需求规格.md) | 用户故事、业务规则、Given/When/Then 验收标准 |
| [0202 非功能需求规格](docs/requirement/0202-非功能需求规格.md) | 性能、可靠性、安全、备份恢复、可观测、AI 质量、合规的可度量目标 |
| [0203 业务流程与用例模型](docs/requirement/0203-业务流程与用例模型.md) | 参与者、用例、业务阶段、状态模型 |
| [0204 术语表](docs/requirement/0204-术语表.md) · [0205 界面与交互需求](docs/requirement/0205-界面与交互需求.md) | 统一术语；信息架构、页面与关键交互 |
| [0301 系统架构设计](docs/design/0301-系统架构设计.md) | 顶层原则、领域模型、Operation、Temporal 工作流、Agent Harness、事件、画布、扩展点 |
| [0308 技术选型决策](docs/design/0308-技术选型决策.md) | 前端、后端、Agent、工作流、中间件、模型的明确选型 |
| [0401 实施路线与交付计划](docs/plan/0401-实施路线与交付计划.md) | P0 与 M1–M4、验收、人力、风险 |
| [PROJECT.md](PROJECT.md) | 仓库结构、目录与编码约定、质量门禁 |
| [DESIGN.md](DESIGN.md) | 视觉与交互规范 |
| [AGENTS.md](AGENTS.md) | 协作与 Git 规则 |

## 开发

新实现尚未开始，启动与验证命令将在 M1（工程底座）完成后写入本节。目标质量门禁见 [PROJECT.md 第 9 节](PROJECT.md#9-质量门禁与交付)。

旧实现的运行方式记录在提交 `c99a5528` 的 README 中，可通过 `git show c99a5528:README.md` 查看。
