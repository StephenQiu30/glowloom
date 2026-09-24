# Lanverse

Lanverse 是一个 AI 短剧制作平台：以剧本为起点、以可发布成片为终点，把剧本解析、角色与场景设定、视觉定稿、分镜、关键帧、视频、配音、剪辑和交付放进同一条可审阅、可恢复、成本透明的生产线；并提供 LibTV 式的无限画布作为创作界面。

> **当前状态（2026-09-25）：** 产品与技术设计已从零重做，处于评审阶段。仓库中的 `backend/`、`agent/`、`frontend/` 代码是旧实现，不符合新设计，处置方式见 [0401 第 5 节](docs/design/0401-实施路线与交付计划.md#5-现有代码的处置待确认)。

## 工作流

```text
剧本导入 → 结构解析 → 资产抽取 → 视觉定稿 → 分镜设计 → 关键帧 → 视频 → 声音 → 剪辑 → 交付
            每一步：AI 产出候选 → 人工审阅 / 修改 → 选定为正式版本
```

## 架构概览

```text
浏览器（流水线视图 / 画布 / 审阅 / 时间线）
   │ REST + SSE
Go 后端（模块化单体：业务事实、权限、计费、工作流定义、媒体处理）
   │                         │
PostgreSQL · Redis     Temporal ─┬─ Go flow / media Worker（写库、FFmpeg）
对象存储 + CDN                  └─ Python AI Worker（模型网关、结构化推理、供应商调用）
```

- **Go 是唯一的业务事实写入方和唯一的流程 Owner**；Python 只执行 AI 步骤并返回结果。
- **所有 AI 结果先是候选**，人工选定后才成为正式版本；每个产物有版本和来源，上游变化时下游标记过期，不自动花钱重做。
- **付费生成**遵循报价 → 确认 → 预留 → 执行 → 结算，结果未知时先对账，不重复扣费。

详见 [0201 系统架构设计](docs/design/0201-系统架构设计.md)。

## 技术栈

| 层 | 技术 |
| --- | --- |
| 后端 | Go、net/http、oapi-codegen、pgx + sqlc、PostgreSQL、Temporal |
| AI | Python、uv、Pydantic、Temporal Python SDK、自建模型网关 |
| 媒体 | FFmpeg / ffprobe、S3 兼容对象存储 + CDN |
| 前端 | Next.js、TypeScript、Tailwind CSS、shadcn/ui、TanStack Query、Zustand |
| 画布 | React Flow（`@xyflow/react`）+ 移植自 infinite-canvas 的卡片与交互界面 |

选型理由见 [0301 技术选型决策](docs/design/0301-技术选型决策.md)，工程约定见 [PROJECT.md](PROJECT.md)。

## 文档

| 文件 | 内容 |
| --- | --- |
| [docs/README.md](docs/README.md) | 文档中心与文档规则 |
| [0101 产品定义与需求分析](docs/design/0101-产品定义与需求分析.md) | 用户、场景、工作流、功能与非功能需求、MVP 边界 |
| [0201 系统架构设计](docs/design/0201-系统架构设计.md) | 领域模型、系统组成、关键流程、画布架构、失败路径 |
| [0301 技术选型决策](docs/design/0301-技术选型决策.md) | 各层选型、画布方案评分、模型供应商核实清单 |
| [0401 实施路线与交付计划](docs/design/0401-实施路线与交付计划.md) | 阶段、里程碑验收、团队、风险 |
| [PROJECT.md](PROJECT.md) | 仓库结构、目录与编码约定、质量门禁 |
| [DESIGN.md](DESIGN.md) | 视觉与交互规范 |
| [AGENTS.md](AGENTS.md) | 协作与 Git 规则 |

## 开发

新实现尚未开始，启动与验证命令将在 M1（工程底座）完成后写入本节。目标质量门禁见 [PROJECT.md 第 9 节](PROJECT.md#9-质量门禁与交付)。

旧实现的运行方式（Docker Compose、依赖的 PostgreSQL / MinIO / Temporal / Kafka / Elasticsearch 等）记录在提交 `c99a5528` 的 README 中，可通过 `git show c99a5528:README.md` 查看。
