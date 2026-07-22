# Thief

面向 AI 绘画创作者的内容创作平台。

产品体验参考即梦的“发现灵感 → 做同款或空白创作 → 生成图片 → 管理作品”路径，但不复制其品牌、界面或内容。爬虫采集的提示词模板、生成参数和示例文件是平台的 `catalog` 内容供给，不是产品主体。

Thief 是项目代号，不代表绕过登录、验证码、付费墙、robots、服务条款或内容权利。来源没有明确的访问与公开展示依据时保持禁用。

> 当前状态：四条 Design → PRD → Plan 文档链已接受；业务实现尚未开始，四份 Acceptance 均为待实施草案。

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

## 正式文档链

| 关注点 | Design | PRD | Plan | Acceptance |
| --- | --- | --- | --- | --- |
| 产品主链 | [DESIGN-001](./docs/design/001-ai内容创作平台系统设计.md) | [PRD-001](./docs/prd/001-ai内容创作平台需求.md) | [PLAN-001](./docs/plans/001-ai内容创作平台计划.md) | [ACCEPTANCE-001](./docs/acceptance/001-ai内容创作平台验收.md) |
| 数据治理与安全 | [DESIGN-002](./docs/design/002-ai内容创作平台数据治理与安全设计.md) | [PRD-002](./docs/prd/002-数据治理与安全需求.md) | [PLAN-002](./docs/plans/002-数据治理与安全计划.md) | [ACCEPTANCE-002](./docs/acceptance/002-数据治理与安全验收.md) |
| 技术功能与运行 | [DESIGN-003](./docs/design/003-ai内容创作平台技术选型与功能实现设计.md) | [PRD-003](./docs/prd/003-技术功能与运行需求.md) | [PLAN-003](./docs/plans/003-技术功能与运行计划.md) | [ACCEPTANCE-003](./docs/acceptance/003-技术功能与运行验收.md) |
| 模块边界与质量 | [DESIGN-004](./docs/design/004-ai内容创作平台模块边界与解耦设计.md) | [PRD-004](./docs/prd/004-模块边界与工程质量需求.md) | [PLAN-004](./docs/plans/004-模块边界与工程质量计划.md) | [ACCEPTANCE-004](./docs/acceptance/004-模块边界与工程质量验收.md) |

正式交付顺序固定为：

```text
Design → PRD → Plan → Acceptance
```

业务实现只在 Plan 可执行后开始，并在 Acceptance 中逐项验证。

## 计划代码结构

```text
frontend/src/{app,components/ui,lib,hooks}/
backend/apps/{api,worker,scheduler}/
backend/packages/{contracts,core,adapters}/
backend/migrations/{module}/
backend/tests/{unit,integration,contract,e2e,fixtures}/
infra/compose/
```

上述目录将在实施 PLAN-003 的 S0 时按纵向切片创建。

## 仓库与许可

GitHub：<https://github.com/StephenQiu30/thief>

本项目代码使用 [MIT License](./LICENSE)。采集的提示词、图片、模型和参数仍受其来源条款、许可及适用法律约束。
