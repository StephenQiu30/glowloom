# Thief

面向 AI 绘画提示词与生成效果的合规采集、治理、检索和展示知识库。

Thief 是项目代号，不代表绕过网站规则或权利边界。所有来源必须先通过条款、许可、技术规则和内容安全准入，采集结果必须保留来源证据，并支持撤销、下架与删除。

> 当前状态：Design 草案阶段。仓库尚未包含业务实现，也未启动任何外部网站实时爬取。

## 项目目标

1. 从获准来源采集 prompt、生成参数、模型信息和对应图片。
2. 对数据执行规范化、去重、来源追踪、权利判定和内容安全门禁。
3. 使用 PostgreSQL 建立可过滤、可排序、可扩展到向量检索的知识目录。
4. 使用 MinIO 保存原图、展示衍生图及必要的隔离对象。
5. 通过 Next.js 图库和详情页展示资源，并支持提示词复制。
6. 提供来源管理、运行审计、失败重试、审核、notice 和删除闭环。

## MVP 边界

- 首个数据源计划使用固定 revision 和校验和的 DiffusionDB Parquet 数据集。
- 未通过来源准入的站点默认禁用，不提供任意 URL 通用爬虫。
- 默认面向内部单租户知识库，成人内容完全拒绝。
- 首年容量目标不超过 100K 个可发布示例。
- 只有 Design 被接受后才进入 PRD、Plan 和实现阶段。

## 技术基线

| 层 | 选型 | 主要职责 |
| --- | --- | --- |
| Web | Next.js 16、React 19、TypeScript、Tailwind | 图库、搜索、详情和管理界面 |
| API | Python 3.13、FastAPI、Pydantic 2、SQLAlchemy 2 | HTTP API、会话、RBAC 和应用服务 |
| 异步任务 | Celery 5.6、RabbitMQ 4.3 | 持久任务投递、重试、DLX 和 Worker 隔离 |
| 数据库 | PostgreSQL 17、`pg_trgm`、pgvector | 事务事实源、词法检索、过滤和向量扩展 |
| 对象存储 | MinIO AIStor Free、S3 API | 原图、衍生图、隔离对象和备份清单 |
| 采集与媒体 | httpx、PyArrow Dataset、Pillow | 白名单获取、流式解析、校验和安全解码 |
| 本地基础设施 | Docker Compose | PostgreSQL、RabbitMQ、MinIO 和观测依赖 |

RabbitMQ 只负责投递，任务最终状态以 PostgreSQL jobs/outbox 为准；MinIO 通过最小权限身份和独立 bucket 管理不同生命周期的媒体对象。

## 设计文档

1. [系统设计](./docs/design/001-ai绘画提示词知识库系统设计.md)：总体架构、来源适配器、任务一致性、API 和检索。
2. [数据治理与安全设计](./docs/design/002-ai绘画提示词知识库数据治理与安全设计.md)：准入、权利、隐私、SSRF、媒体沙箱和删除防复活。
3. [技术选型与功能实现设计](./docs/design/003-ai绘画提示词知识库技术选型与功能实现设计.md)：技术基线、代码边界、MVP 页面、纵向切片和验收门禁。

正式交付顺序固定为：

```text
Design → PRD → Plan → Acceptance
```

## 仓库结构

```text
.
├── .codex/                  # Agent 角色与可复用工作流
├── .github/                 # CI 与 PR 模板
├── docs/
│   ├── design/              # 架构与契约设计
│   ├── prd/                 # 已接受 Design 派生的产品需求
│   ├── plans/               # 可执行实现与测试计划
│   ├── acceptance/          # 验收证据与结论
│   └── operations/          # 验收后的部署与运维说明
├── AGENTS.md                # 长期协作和交付规则
├── AGENTS.local.md          # 当前项目环境约束
└── WORKFLOW.md              # Linear、workspace 与运行时编排
```

计划中的业务代码将按 `apps/web`、`apps/api`、`apps/worker`、`packages/core`、`tests` 和 `infra` 分层；这些目录会在 Plan 接受后按纵向切片逐步创建。

## 协作方式

1. 先阅读 [AGENTS.md](./AGENTS.md) 和 [AGENTS.local.md](./AGENTS.local.md)。
2. 设计或实现变更必须绑定可衡量的验收标准，并遵循仓库的 Design → PRD → Plan → Acceptance 门禁。
3. 核心逻辑默认按 TDD 的 Red → Green → Refactor 推进。
4. 贡献要求见 [CONTRIBUTING.md](./CONTRIBUTING.md)。

## 仓库地址与许可

GitHub：<https://github.com/StephenQiu30/thief>

本项目使用 [MIT License](./LICENSE)。来源数据、图片、模型和提示词仍受各自条款、许可及适用法律约束。
