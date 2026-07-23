# Thief

面向 AI 绘画创作者的内容创作平台。

产品体验参考即梦的“发现灵感 → 做同款或空白创作 → 生成图片 → 管理作品”路径，但不复制其品牌、界面或内容。爬虫采集的提示词模板、生成参数和示例文件是平台的 `catalog` 内容供给，不是产品主体。

Thief 是项目代号，不代表绕过登录、验证码、付费墙、robots、服务条款或内容权利。来源没有明确的访问与公开展示依据时保持禁用。

> 当前状态：工程基础、身份会话、Web 登录、catalog 事实源和 1K 幂等导入已实现；后端保持单包与单线迁移。下一业务切片是公开目录 API。

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
| Worker | Celery | 执行已接入的异步任务；当前只启用 `generation` 队列 |
| Scheduler | 单实例调度进程 | 创建到期采集与维护任务 |
| PostgreSQL | PostgreSQL、`pg_trgm`，按需 pgvector | 业务数据、任务状态、配额和搜索 |
| RabbitMQ | RabbitMQ | `crawl`、`media`、`index`、`generation` 队列 |
| Object Storage | MinIO | 第三方示例文件和用户生成作品 |

MVP 采用模块化单体和一个 Python 包。业务模块只在首个真实用例落地时创建：

| 模块 | 职责 | 状态 |
| --- | --- | --- |
| `identity` | 邀请、用户、密码、会话和角色 | 已实现 |
| `catalog` | 来源导入、溯源、模板、示例、分类和搜索 | 实施中 |
| `creation` | 草稿、生成任务、供应商尝试、额度、资产和作品 | S4 创建 |
| `operations` | 来源启停、审核、下架、删除、预算和审计 | S6 创建 |

Web、API、Worker 和 Scheduler 是运行角色，不是重复的业务模块。业务规则不依赖 FastAPI、Celery 或 SQLAlchemy；基础设施实现通过小型 Protocol 接入。

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

## 当前代码结构

```text
frontend/src/{app,components,lib}/
backend/src/thief/{api,identity,catalog,infrastructure}/
backend/src/thief/{worker.py,scheduler.py,settings.py}
backend/migrations/versions/
backend/tests/{unit,integration,architecture}/
infra/compose/
```

Alembic migration 是数据库结构的线性版本记录，不是业务模块目录。`platform_0001`
是初始基线，当前唯一 head 是 `platform_0002`；首次保留数据的环境建立后不再重写
历史，后续按 `0003_<capability>.py` 顺序追加。

## 仓库与许可

GitHub：<https://github.com/StephenQiu30/thief>

本项目代码使用 [MIT License](./LICENSE)。采集的提示词、图片、模型和参数仍受其来源条款、许可及适用法律约束。
