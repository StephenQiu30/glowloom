---
layer: Design
doc_type: Solution Architecture Design
doc_no: ARCH-001
title: AI短剧制作平台总体架构设计
status: review
version: 0.5.0
owner: Lanverse
audience: [Product, Architecture, Frontend, Backend, QA, Security, Operations]
feature_area: AI短剧制作平台总体架构
purpose: 定义 backend/frontend 统一仓库结构、运行边界、领域模块、数据流和首发实现顺序
canonical_path: docs/design/ARCH-001-AI短剧制作平台总体架构设计.md
inputs: [SRS-001, NFR-001, TCR-001, TCR-002, TCR-003, ADG-001]
evidence_baselines: [Jellyfish main@a967819, Toonflow master@bc61ec7]
outputs: [目标架构, monorepo结构, 部署单元, 模块分组, 迁移与回滚策略]
triggers: [平台范围变化, 技术基线变化, 部署边界变化, 领域事实归属变化]
updated: 2026-07-25
downstream: [PRD, Plan, Acceptance, ADR]
---

# ARCH-001 AI 短剧制作平台总体架构设计

## 1. 设计结论

Lanverse 首发采用统一 monorepo、模块化单体控制面和独立 Worker：Next.js Web 负责交互与 BFF，NestJS API 负责授权和业务事实，PostgreSQL 保存权威数据，Temporal 负责编排长任务，AI/媒体 Worker 独立扩缩容，对象存储保存媒体，Redis 只承担易失缓存与限流。

本设计吸收 Jellyfish 的分镜准备/生成边界，以及 Toonflow 的来源事件、Agent 分工和生产图思路；不复制两者的技术栈或具体代码。流程和 Agent/任务见 [ARCH-002](ARCH-002-短剧生产流程与工作台设计.md)、[ARCH-003](ARCH-003-AI策划与生成任务架构设计.md)，契约、媒体安全与运行质量见 [ARCH-004](ARCH-004-API事件文件与数据契约设计.md)、[ARCH-005](ARCH-005-媒体安全隐私与数据生命周期设计.md)、[ARCH-006](ARCH-006-部署观测灾备容量成本与测试设计.md)；规范模块目录、公开协作和服务映射以 [ARCH-007](ARCH-007-业务模块边界与服务协作设计.md) 为唯一设计源，纵向切片、阶段文档链和详细工程目录以 [ARCH-008](ARCH-008-交付切片文档链与工程结构设计.md) 为唯一设计源。

## 2. 当前状态与约束

- Git 根目录已统一为 `Lanverse/`，原 `lanverse-backend/` 子目录已移除。
- 当前只有治理和正式文档，无应用源码、依赖清单、数据库或部署制品。
- 技术基线由 [TCR-001](../requirement/TCR-001-平台技术栈与总体架构约束需求规格说明书.md) 固定；本设计不得降低 NFR、权限、版本、成本、合规和交付要求。
- 正式实现只能在 Design、PRD 和可执行 Plan 均为 `accepted` 后开始；实现完成后再以运行证据回填并验证 Acceptance。

## 3. 目标与非目标

目标：

- 打通内容导入、剧本、分镜准备、资产一致性、生成、声音、后期、审核、成本、合规和交付。
- 让每项业务事实、AI 输入、任务尝试、候选结果和正式采用可追溯、可恢复、可替换。
- 先交付可验证的单集闭环，再扩展跨集协作、高级剪辑和供应能力。

非目标：

- 不自研基础模型，不默认建设微服务、Kafka 或插件市场。
- 不把浏览器做成专业 NLE，不把“任务成功”当作“创作通过”。
- 不为兼容已删除的历史前后端试验代码建立双轨实现。

## 4. 开源项目参考决策

参考基线为 Jellyfish `main@a9678194ddf2d9be3ccbe78d4287d87d5089e123` 与 Toonflow `master@bc61ec7a1b5df31293b286981a5f4ad4635464ee`。仅吸收可验证的领域模式；两者固定提交均包含 Apache-2.0 `LICENSE`，Toonflow 另在项目说明披露补充商用条款，完成法律审查前不复制代码或素材。

| 来源与模式 | Lanverse 决策 | 原因 |
| --- | --- | --- |
| Jellyfish：分镜准备页与生成工作室分离 | 吸收 | 降低页面职责和状态混淆 |
| Jellyfish：准备、生成、任务状态分离 | 扩展为准备、运行、审核、采用、交付正交状态 | 防止单一状态字段过载 |
| Jellyfish：Draft→Context→Preview→Submission | 增加版本、哈希和不可变快照后吸收 | 保证预览与实际提交一致 |
| Jellyfish：业务任务与执行器分离、OpenAPI 客户端 | 以 ProductionTask/Attempt、Temporal 与生成客户端实现 | 支持恢复并减少契约漂移 |
| Toonflow：章节事件→改编策划→剧本→生产 | 形成版本化 SourceEvent/AdaptationPlan 主链 | 长篇改编先建立来源证据与取舍 |
| Toonflow：决策/执行/监督 Agent | 作为提案、执行和复核职责吸收 | Agent 输出必须经权限、版本与人工门禁 |
| Toonflow：无限画布与持久记忆 | 画布仅作 P1 视图；记忆按租户/项目/Agent 隔离并引用来源 | 布局和召回结果不能成为正式事实 |
| Toonflow：用户上传 TypeScript 供应商代码 | 不照搬；仅允许受控、签名、版本化 Adapter | 避免任意代码执行、密钥和供应链风险 |
| Jellyfish：FastAPI/MySQL/Celery/Vite 与前端轮询 | 不照搬具体技术组合；保留页面边界与生成客户端思想 | 本组 Design 拟定的技术栈、Temporal 和 SSE 约束不同 |
| Toonflow：Electron/Express/SQLite、进程内 Promise 与 JSON 工作区 | 不照搬运行形态 | 不满足多租户 SaaS、持久任务和类型化事实要求 |

参考证据：Jellyfish [页面边界](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/site/content/docs/architecture/shot-page-boundary.md)、[任务执行](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/site/content/docs/architecture/task-execution.md)、[生成客户端](https://github.com/Forget-C/Jellyfish/tree/a9678194ddf2d9be3ccbe78d4287d87d5089e123/front/src/services/generated) 和 [LICENSE](https://github.com/Forget-C/Jellyfish/blob/a9678194ddf2d9be3ccbe78d4287d87d5089e123/LICENSE)；Toonflow [项目说明](https://github.com/HBAI-Ltd/Toonflow-app/blob/bc61ec7a1b5df31293b286981a5f4ad4635464ee/docs/README.en.md)、[Script Agent](https://github.com/HBAI-Ltd/Toonflow-app/tree/bc61ec7a1b5df31293b286981a5f4ad4635464ee/src/agents/scriptAgent)、[事件提取](https://github.com/HBAI-Ltd/Toonflow-app/blob/bc61ec7a1b5df31293b286981a5f4ad4635464ee/src/utils/cleanNovel.ts)、[Agent 记忆](https://github.com/HBAI-Ltd/Toonflow-app/blob/bc61ec7a1b5df31293b286981a5f4ad4635464ee/src/utils/agent/memory.ts) 和 [LICENSE](https://github.com/HBAI-Ltd/Toonflow-app/blob/bc61ec7a1b5df31293b286981a5f4ad4635464ee/LICENSE)。

## 5. 系统上下文与运行边界

```mermaid
flowchart LR
    U["创作者、审核者、运营人员"] --> WEB["frontend · Next.js"]
    WEB --> API["backend API · NestJS"]
    API --> PG["PostgreSQL · 权威事实"]
    API --> RD["Redis · 缓存/限流"]
    API --> S3["S3 · 媒体对象"]
    OD["backend Outbox Dispatcher"] --> PG
    OD --> TP["Temporal · 持久工作流"]
    TP --> WK["backend Worker · AI/媒体/通知 Activity"]
    WK --> AI["外部 AI 与内容安全能力"]
    WK --> S3
    WK --> FF["FFmpeg/ffprobe"]
    WEB --> OT["OpenTelemetry"]
    API --> OT
    WK --> OT
```

- Web 只保存编辑会话，不保存唯一正式事实或供应商密钥。
- `backend` 应用层是业务写入的唯一授权边界：公开命令经 API，Worker 结果经相同应用用例；二者都不得绕过模块规则直接写表。
- Temporal 保存执行历史但不是用户业务事实源；Worker 不直接拥有业务数据。
- 大媒体经短期授权在浏览器与对象存储之间传输，API 只管理授权和元数据。

## 6. 目标仓库结构

```text
Lanverse/
├── backend/
│   ├── src/
│   │   ├── modules/         # NestJS 领域模块与应用用例
│   │   ├── workflows/       # Temporal Workflow 定义
│   │   ├── workers/         # AI、媒体、通知 Activity 与进程入口
│   │   └── integrations/    # 供应能力、对象存储及外部系统适配
│   ├── prisma/              # 数据模型与向后兼容迁移
│   └── test/                # 单元、集成与契约测试
├── frontend/
│   ├── src/
│   │   ├── app/             # Next.js 路由、布局与 BFF 边界
│   │   ├── features/        # 按业务能力组织的工作台功能
│   │   ├── components/      # 可复用且可访问的展示组件
│   │   ├── services/generated/ # OpenAPI 生成客户端
│   │   └── stores/          # 有边界的编辑会话状态
│   └── e2e/                 # Playwright 关键流程
├── deploy/                  # 本地环境、IaC、部署与观测模板
├── docs/                    # AGENTS 规定的正式交付流程
├── package.json
├── pnpm-workspace.yaml
└── pnpm-lock.yaml
```

`backend` 同一代码库可构建 API 与多个 Worker 制品，但进程、队列和资源池独立；不得再创建顶层 `apps/`、`packages/` 或独立 Worker 仓库。可选 Python 推理服务只有在 TypeScript Worker 无法承担且 ADR 通过后才创建，也不得预留空目录。

## 7. 后端领域模块

| 分组 | 规范模块 | 边界摘要 |
| --- | --- | --- |
| 基础 | `identity-access`、`project-catalog` | 身份授权与项目层级/生命周期分离 |
| 故事与资产 | `story-development`、`continuity`、`creative-assets`、`media-library`、`storyboard` | 剧本、连续性、语义资产、物理媒体和镜头分别拥有事实 |
| AI 与生产 | `agent-runtime`、`model-catalog`、`generation`、`production-jobs`、`postproduction` | Agent、模型路由、生成意图、执行生命周期和后期版本分离 |
| 治理与交付 | `review-approval`、`cost-billing`、`compliance-governance`、`delivery` | 决定、账本、规则裁判和交付产物职责分离 |
| 运营支撑 | `operations-support` | 通知、运营案件和可重建分析投影，不回写核心事实 |

17 个模块仍共同运行在模块化单体中，不对应 17 个微服务。每个模块的事实、非职责、命令/查询/事件、允许依赖、失败责任、前端 Feature 与运行制品映射以 [ARCH-007](ARCH-007-业务模块边界与服务协作设计.md) 为准；跨模块每步只提交所属模块事实与 Outbox。

## 8. 前端应用边界

- Server Components 负责会话校验、导航壳和适合服务端取得的初始数据；编辑器使用明确 Client Island。
- 服务端状态使用 TanStack Query，编辑会话使用 Zustand，URL 保存可分享导航状态。
- 公开站、登录工作台、外部审片入口采用不同布局、缓存和安全策略。
- `frontend/src/services/generated/` 每次由接受的 OpenAPI 生成；页面不得再建第二套 DTO 或手写请求类型。

## 9. 部署、观测和安全

- 独立制品：`frontend`、`backend-api`、`backend-outbox-dispatcher`、Workflow/AI/媒体/运营 Worker 与 `backend-migration-job`；共享同一提交版本和契约版本。
- 环境：local、test、staging、production；生产数据不得进入低环境。
- 发布顺序：向后兼容迁移→API/Worker→Web→清理旧契约；回滚反向执行且不回滚已提交业务事实。
- 所有请求、任务、Activity、供应调用、计量和媒体结果共享 trace/request/task/attempt 标识。
- OIDC 登录后使用安全 HttpOnly 会话；所有业务动作在 API 重新执行租户与对象级授权。

## 10. 需求追踪矩阵

| 需求输入 | 设计落点 | 设计输出/下游验证 |
| --- | --- | --- |
| SRS-001、FR-001～FR-008、FR-021 | 本文 5～8；ARCH-002；ARCH-003；ARCH-007 | 身份、内容、资产、工作台、Agent 与人工门禁 |
| FR-009～FR-011、FR-017 | ARCH-003；ARCH-004；ARCH-007 | 能力、生成、任务、候选、成本、契约和恢复 |
| FR-012～FR-016、FR-018～FR-020 | ARCH-002；ARCH-004～ARCH-005；ARCH-007 | 后期、审核、采用、合规、交付与通知 |
| NFR-001、TCR-001～TCR-003 | 本文 5～9、12；ARCH-004～ARCH-007 | 契约、模块、数据、媒体、安全、运行、容量和测试 |
| ADG-001 | 本文第 13 节；ARCH-001～ARCH-008；TRACE-001；ADR-001～ADR-002 | 设计准入、逐需求追踪和技术决策入口 |

逐条需求的覆盖状态、设计位置和预期验证见 [TRACE-001](TRACE-001-AI短剧平台需求设计验证追踪矩阵.md)；后续 PRD、Plan、Test 和 Acceptance 必须回填具体文档编号与证据。

## 11. 实现切片

| 切片 | 可演示闭环 | 依赖 |
| --- | --- | --- |
| S0 可信项目入口 | 登录、工作空间、项目/分集、审计与可复现工程基线 | Design/PRD/Plan 接受 |
| S1 来源到剧本基线 | 来源证据、Agent 提案、人工确认与剧本基线 | S0 |
| S2 资产到分镜准备 | 上传隔离、资产版本、拆镜提案与准备确认 | S1 |
| S3 可计费生成闭环 | 能力、成本/权利预检、持久任务、候选与采用 | S2 |
| S4 音画后期闭环 | 配音、音效、字幕、时间线与可审粗剪 | S3 |
| S5 审核合规交付 | 审片、批准、合规、质检与版本化交付 | S4 |
| S6 运营与首发硬化 | 通知、处置、分析、NFR、恢复与发布判断 | S0～S5 |

切片主需求、成本/合规渐进门禁、阶段文档编号、技术栈和详细目录见 [ARCH-008](ARCH-008-交付切片文档链与工程结构设计.md)。每个切片必须单独完成 Design、PRD、可执行 Plan、测试优先实现和 Acceptance，不允许一次性搭空壳模块。

## 12. 迁移与回滚

- 当前无业务数据和源码，不执行历史数据迁移；初始化迁移从 `0001` 建立。
- 原仓库目录上移只改变本地路径，不改变 Git 对象、提交或远端地址。
- 任一切片失败时回滚其部署与向后兼容迁移，保留已产生的任务、费用和媒体证据。
- 技术族或部署单元变化必须另建 ADR；本次模块所有权重构的理由和迁移见 [ADR-002](ADR-002-业务模块拆分与依赖方向.md)。

## 13. 评审门禁与未决项

进入 PRD 前必须确认：首发部署地区与数据驻留原则、身份边界、容量假设责任人和媒体基准规格，并完成 ARCH-001～ARCH-008、TRACE-001、[ADR-001](ADR-001-首发平台架构与仓库边界.md) 与 [ADR-002](ADR-002-业务模块拆分与依赖方向.md) 的 `design_entry` 评审。具体身份/存储/Temporal/PostgreSQL 托管产品和 AI Provider 按 [ARCH-008 第 9 节](ARCH-008-交付切片文档链与工程结构设计.md#9-渐进决策门禁) 在首次使用它们的切片关闭。进入实现前，适用 Requirement、Design、PRD 和可执行 Plan 必须均为 `accepted`；`implementation_release` 只在实现和 Acceptance 证据完成后作为发布门禁执行。
