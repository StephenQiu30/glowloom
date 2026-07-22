# Thief

面向 AI 绘画创作者的内容创作平台。

产品体验参考即梦的“发现灵感 → 做同款或空白创作 → 生成图片 → 管理作品”路径，但不复制其品牌、界面或内容。爬虫采集的提示词模板、生成参数和示例文件是平台的 `catalog` 内容供给，不是产品主体。

Thief 是项目代号，不代表绕过登录、验证码、付费墙、robots、服务条款或内容权利。来源没有明确的访问与公开展示依据时保持禁用。

> 当前状态：Design 优化阶段。仓库尚未包含 Web、API、Worker 等业务实现，也未启动外部采集或真实图片生成。

## MVP 用户闭环

```text
浏览/搜索模板 → 查看示例与提示词 → 做同款或空白创作
→ 编辑提示词与参数 → 异步生成图片 → 查看个人作品
```

MVP 包含：

1. 获准提示词模板和示例文件的采集、清洗、审核与展示。
2. 首页发现、分类、搜索、模板详情和“做同款”。
3. 用户登录、创作工作台、真实图片生成、任务状态和个人作品。
4. 来源管理、任务重试、内容审核、下架和删除。

MVP 不包含视频生成、模型训练、社区互动、支付订阅、通用爬虫、微服务或 Kubernetes。

## 服务架构

| 服务 | 选型 | 职责 |
| --- | --- | --- |
| Web | Next.js、React、TypeScript、Tailwind | 模板站、创作工作台、作品页和管理界面 |
| API | FastAPI、Pydantic、SQLAlchemy | 目录、用户会话、创作、生成和管理 API |
| Worker | Celery | 采集、媒体、索引和模型生成任务 |
| Scheduler | 单实例调度进程 | 创建到期采集与维护任务 |
| PostgreSQL | PostgreSQL、`pg_trgm`，按需 pgvector | 业务数据、任务状态、配额和搜索 |
| RabbitMQ | RabbitMQ | `crawl`、`media`、`index`、`generation` 队列 |
| Object Storage | MinIO | 第三方示例文件和用户生成作品 |

MVP 采用模块化单体。`ingestion`、`catalog`、`creation`、`generation`、`asset`、`governance`、`search` 和 `identity` 通过 Port、Workflow 和版本化事件协作，不直接读写彼此的数据表。

## 设计文档

1. [系统设计](./docs/design/001-ai内容创作平台系统设计.md)：产品定位、创作主链、服务拓扑和核心数据。
2. [数据治理与安全设计](./docs/design/002-ai内容创作平台数据治理与安全设计.md)：第三方模板、用户创作、生成供应商和删除边界。
3. [技术选型与功能实现设计](./docs/design/003-ai内容创作平台技术选型与功能实现设计.md)：页面/API、生成契约和实现切片。
4. [模块边界与解耦设计](./docs/design/004-ai内容创作平台模块边界与解耦设计.md)：数据所有权、Port/Adapter、Workflow 和事件契约。

正式交付顺序固定为：

```text
Design → PRD → Plan → Acceptance
```

业务实现只在 Plan 可执行后开始，并在 Acceptance 中逐项验证。

## 计划代码结构

```text
apps/{web,api,worker,scheduler}/
packages/{contracts,core,adapters}/
migrations/{module}/
tests/{unit,integration,contract,e2e,fixtures}/
infra/compose/
```

上述目录会在 Design、PRD 和 Plan 被接受后按纵向切片创建。

## 仓库与许可

GitHub：<https://github.com/StephenQiu30/thief>

本项目代码使用 [MIT License](./LICENSE)。采集的提示词、图片、模型和参数仍受其来源条款、许可及适用法律约束。
