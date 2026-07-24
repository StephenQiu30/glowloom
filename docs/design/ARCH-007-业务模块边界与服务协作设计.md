---
layer: Design
doc_type: Business Module Boundary and Service Collaboration Architecture Design
doc_no: ARCH-007
title: 业务模块边界与服务协作设计
status: review
version: 0.1.0
owner: Lanverse
audience: [Product, Architecture, Backend, Frontend, QA, Security, Operations, Data]
feature_area: 业务模块、数据所有权、依赖治理与运行服务
purpose: 定义业务有界模块、公开协作契约、依赖方向、部署映射和可执行架构门禁
canonical_path: docs/design/ARCH-007-业务模块边界与服务协作设计.md
inputs: [SRS-001, FR-001至FR-021, NFR-001, TCR-001至TCR-003, ADG-001, ARCH-001]
evidence_baselines: [arc42-template@8dff0d9, modular-monolith-with-ddd@91c8ef2, spring-modulith@c4f6d51, nest@d8ee014, dependency-cruiser@15d41a9, samples-typescript@5c245e0, transactional-outbox-pattern@23e0519]
outputs: [模块目录, 事实所有权, 公开契约, 依赖规则, 服务映射, 边界验证]
triggers: [业务能力变化, 事实所有权变化, 跨模块协作变化, 部署拆分, 团队或SLO边界变化]
updated: 2026-07-24
downstream: [PRD, Plan, Module Design, Contract Test, Acceptance, ADR]
---

# ARCH-007 业务模块边界与服务协作设计

## 1. 设计结论与范围

Lanverse 首发采用模块化单体控制面和独立运行进程。业务模块按事实、不变式和变化原因划分；服务按协议、资源、扩缩和故障隔离划分。模块不等于页面、岗位、数据库表、NestJS Controller、Temporal Workflow 或部署服务，也不为未来可能性预建空微服务。

本文件是模块名称、职责、事实所有权、公开协作和允许依赖的唯一设计源；[ARCH-001](ARCH-001-AI短剧制作平台总体架构设计.md) 只保留总体视图。流程、任务、契约、媒体和运行质量分别由 [ARCH-002](ARCH-002-短剧生产流程与工作台设计.md) 至 [ARCH-006](ARCH-006-部署观测灾备容量成本与测试设计.md) 细化，但不得另行定义冲突的模块所有权。

## 2. 采用标准与开源证据

| 固定证据 | 吸收的判定标准 | 不照搬内容 |
| --- | --- | --- |
| [arc42@8dff0d9 架构模板](https://github.com/arc42/arc42-template/blob/8dff0d9b1f9640684df8c3bbcdc2ee45f989ca0f/EN/arc42-template.adoc)（CC BY-SA-4.0） | 目标、约束、构件、运行时、部署、质量和风险覆盖维度 | 模板原文与许可传播要求；本文采用自有结构和表述 |
| [modular-monolith-with-ddd@91c8ef2](https://github.com/kgrzybek/modular-monolith-with-ddd/tree/91c8ef24b4cb6ef558c95d8267fa07d68c7059f8/src/Modules)（MIT）及其[模块测试](https://github.com/kgrzybek/modular-monolith-with-ddd/blob/91c8ef24b4cb6ef558c95d8267fa07d68c7059f8/src/Tests/ArchTests/Modules/ModuleTests.cs) | 有界模块、内部层次、Outbox/Inbox、边界测试 | .NET 技术栈、示例中的测试缺陷和具体领域模型 |
| [Spring Modulith@c4f6d51 验证规则](https://github.com/spring-projects/spring-modulith/blob/c4f6d51365bdb7f943327392a9cd4e828a58af0f/src/docs/antora/modules/ROOT/pages/verification.adoc)（Apache-2.0） | 模块图无环、只访问公开 API、依赖白名单和 CI 验证 | Spring/Java 框架与运行时 |
| [NestJS@d8ee014 模块样例](https://github.com/nestjs/nest/blob/d8ee0143bdd76b2e81b460a196c15a9387b73ee2/sample/19-auth-jwt/src/auth/auth.module.ts)（MIT） | Provider 默认私有、显式 imports/exports、组合根装配 | 教学用 JWT 配置和扁平业务 Service |
| [dependency-cruiser@15d41a9 规则](https://github.com/sverweij/dependency-cruiser/blob/15d41a9020944d94c4369735fe2e8639eff42799/configs/recommended.cjs)（MIT） | 无环、禁止路径和按模块组限制的可执行 TypeScript 依赖检查 | 默认规则不能替代 Lanverse manifest allow-list |
| [Temporal TypeScript@5c245e0](https://github.com/temporalio/samples-typescript/tree/5c245e02e2629cce5301af70f68e5fc9b8f2bb86/monorepo-folders)（MIT） | Workflow 确定性、Activity 副作用、API/Worker 独立扩缩和稳定业务 ID | localhost、单队列、同步等待完成和本地文件状态 |
| [AWS Outbox@23e0519](https://github.com/aws-samples/transactional-outbox-pattern/blob/23e0519c7a8048c6db4acd8f3d7de534a9d5deac/README.md)（MIT-0） | 业务事实与 Outbox 同事务、至少一次投递、消费者幂等 | Controller 持有事务、同进程轮询和发送即删除 |

这些仓库用于建立可验证的架构标准，不构成代码复制或企业成熟度背书；具体许可见各固定提交的 `LICENSE`，实现前仍须完成依赖与法务检查。

## 3. 架构层级与命名规则

`业务能力 → 有界模块 → 应用用例/领域对象 → 运行服务` 是唯一分解顺序。模块使用表达业务能力的英文 kebab-case；模块内部对象不得被另一个模块直接导入。每项权威事实恰有一个模块所有者；引用方只保存稳定 ID、必要版本和形成决定时的不可变快照。FR 文档中的“依赖”表示业务前置或协作，不直接形成代码依赖。

## 4. 规范模块目录与事实所有权

| 分组 / 模块 | 拥有的权威事实与不变式 | 明确不负责 | 主需求 |
| --- | --- | --- | --- |
| 基础 / `identity-access` | 账户、工作空间、成员、角色、临时访问授权、会话撤销和授权策略 | 项目协作内容、审片意见、临时媒体 URL | FR-001；FR-020 |
| 基础 / `project-catalog` | 项目、系列、季、分集、责任人与生命周期 | 来源解析、剧本、镜头和跨域聚合状态 | FR-002 |
| 创作 / `story-development` | 来源修订、来源事件、改编方案、策划产物、剧本版本和正式基线 | 故事一致性裁决、资产媒体、镜头和 Agent 运行 | FR-003～004 |
| 创作 / `continuity` | 故事圣经、叙事事实、人物/关系/时空状态及连续性评估 | 改写剧本、视觉资产和自动批准冲突 | FR-005 |
| 创作 / `creative-assets` | 角色、场景、道具、服装、风格、声音等创作身份、版本和项目绑定 | 二进制文件、媒体转码、镜头绑定和故事事实 | FR-006 |
| 媒体 / `media-library` | MediaObject/Version、Blob、Rendition、UploadSession、MediaAccessGrant、技术检查、派生谱系和使用投影 | 资产语义、权威使用关系、内容安全结论、审核采用和交付包 | FR-007 |
| 创作 / `storyboard` | 场次只读生产投影、ShotSpec、拆解候选、ShotAssetBinding、准备状态和分镜基线 | 场次/对白事实、剧本基线、生成候选和供应任务 | FR-008 |
| AI / `agent-runtime` | Agent/Prompt/Skill 版本、Run/Step/机器复核、工具授权和受控记忆 | 业务基线、人工批准、模型供应执行和费用账本 | FR-021 |
| AI / `model-catalog` | 模型能力、Provider 配置引用、Manifest、路由/价格及供应条款声明快照 | 任务运行、供应密钥、预算余额、合规批准和生成候选 | FR-009 |
| 生产 / `generation` | GenerationDraft、Context、Preview、Submission 快照、生成请求、GenerationCandidate 和一致性评估/裁决 | Provider Attempt、媒体字节/技术状态、费用结算和正式采用 | FR-011 |
| 生产 / `production-jobs` | ProductionTask/Batch/Attempt、TaskDependency/ReworkLink、CancellationRequest/RetryRequest、回调去重、执行状态和供应对账 | 模型路由政策、业务输入、媒体事实和账本事实 | FR-010 |
| 后期 / `postproduction` | 配音/口型、字幕、本地化、声音、时间线、渲染快照与后期版本 | 原始媒体字节、审核批准和交付发行 | FR-012～015 |
| 治理 / `review-approval` | 审核轮次、意见、Agent 人工决定、ExternalReviewGrant、不可变审核决定和唯一当前采用 | 修改被审对象、执行任务和平台临时授权 | FR-016；FR-021 |
| 财务 / `cost-billing` | 预算、额度、成本预估、预占/结算/释放、追加账本、支付交易，以及 P1 套餐/开票申请 | 模型定价政策、生产任务、支付渠道实现和供应用量原始证据 | FR-017 |
| 治理 / `compliance-governance` | 权利、内容安全、AI 标识、个人数据请求、隐私/保留/Legal Hold/删除案件及审计查询索引 | 跨域直接删除、技术质检、交付文件和源领域事实 | FR-018；FR-020 |
| 交付 / `delivery` | 技术质检、DeliverySnapshot、PackageBuildRecord/Manifest、交付版本和 DeliveryDownloadGrant | ProductionAttempt、修改时间线、批准内容和更改合规结论 | FR-019 |
| 支撑 / `operations-support` | communications、ops-cases、analytics-projections 三个内部组件的通知、运营案件和可重建分析投影 | 临时访问授权、个人数据请求、作为业务事实源或回写核心状态 | FR-020 |

平台临时访问归 `identity-access`，外部审片访问归 `review-approval`，媒体临时访问归 `media-library`，交付下载归 `delivery`；Grant 保存范围、目的、主体、到期和撤销，签名 URL 不是事实。各引用归源模块，`media-library` 只重建 MediaUsageProjection。`operations-support` 三组件只能经内部 public facade 协作，communications/analytics 不写 ops-case 或核心表。

## 5. 各模块公开协作契约

| 模块 | 公开写命令 | 公开查询 / 集成事件 |
| --- | --- | --- |
| `identity-access` | manageMembership、changeRole、grantTemporaryAccess、revokeTemporaryAccess、revokeSession | authorize、getActorContext / `identity.access.changed.v1` |
| `project-catalog` | createProject、manageEpisode、changeLifecycle | getProjectScope / `project.catalog.changed.v1` |
| `story-development` | importSource、proposePlan、saveScriptDraft、confirmScriptBaseline | getStoryBaseline / `story.baseline.changed.v1` |
| `continuity` | recordStoryFact、submitContinuityDecision | evaluateContinuity / `continuity.assessment.changed.v1` |
| `creative-assets` | createAsset、versionAsset、bindProjectAsset | resolveAssetVersion / `creative.asset.changed.v1` |
| `media-library` | authorizeUpload、authorizeMediaAccess、revokeMediaAccess、registerMedia、recordRendition、forgetMedia | resolveMedia、findUsage / `media.version.changed.v1` |
| `storyboard` | resolveExtractionCandidate、bindShotAsset、confirmShotBaseline | checkShotReadiness / `storyboard.baseline.changed.v1` |
| `agent-runtime` | startAgentRun、recordStep、cancelAgentRun、publishSkill | getAgentRun / `agent.run.changed.v1` |
| `model-catalog` | registerCapability、publishRoutingPolicy | resolveRouteSnapshot / `model.catalog.changed.v1` |
| `generation` | saveDraft、compilePreview、requestGeneration、registerCandidate、resolveConsistencyAssessment | getSubmission、getCandidate / `generation.requested.v1`、`generation.candidate.changed.v1` |
| `production-jobs` | createTask、requestCancellation、requestRetry、startAttempt、recordProviderResult、finalizeAttempt、reconcileAttempt | getTask / `production.task.changed.v1`、`production.retry.requested.v1`、`production.usage.measured.v1` |
| `postproduction` | saveVoiceTake、saveLocalization、saveTimeline、requestRender | getPostproductionSnapshot / `postproduction.changed.v1` |
| `review-approval` | openReview、grantExternalReviewAccess、revokeExternalReviewAccess、addComment、recordDecision、adoptVersion、revokeAdoption | getAdoption / `review.adoption.changed.v1` |
| `cost-billing` | setBudgetPolicy、reserveBudget、settleUsage、releaseHold、reverseEntry、purchaseCredits、recordPayment、requestInvoice | getBudgetAvailability / `ledger.entry.posted.v1` |
| `compliance-governance` | publishCompliancePolicy、assess、placeLegalHold、openDataSubjectRequest、openDeletionCase、recordLifecycleStep | getGateDecision、queryAudit / `compliance.decision.changed.v1` |
| `delivery` | createDeliverySnapshot、recordQc、completePackage、releaseDelivery、grantDeliveryDownload、revokeDeliveryDownload | getDelivery / `delivery.changed.v1` |
| `operations-support` | enqueueNotification、openOpsCase、recordResolution | getOpsProjection / `notification.delivery.changed.v1`、`ops.case.changed.v1` |

每个含个人数据的模块须从 `public/index.ts` 实现统一 `DataLifecycleParticipantPort`：`discoverDataScope`、幂等命令 `executeLifecycleStep(case_id,step_id,scope_hash,action)` 和 `getLifecycleEvidence`，`action` 只能是 `access`、`export`、`correct`、`delete` 或 `anonymize`。模块只读取、导出、追加更正、删除或匿名化自己的事实并保留证据，`compliance-governance` 汇总步骤，不跨表执行。

命令名称表达意图而不是 `PATCH status`；事件使用过去式和 `.v1` 大版本后缀，带 `event_id/schema_version/aggregate_id/aggregate_version/workspace_id/correlation_id/occurred_at`，不得承载密钥、大媒体或完整可变聚合。

## 6. 允许依赖与协作方式

下表是首发业务同步依赖 allow-list；除 `identity-access` 外，每个受保护应用入口还由组合根强制注入其 `AuthorizationPort` 并服务端重验，该统一基础边不允许领域层导入。权限列为规范动作 ID，ARCH-002 与 ARCH-004 只能逐字使用这些 ID；`—` 表示除此以外只依赖自身、Shared Kernel 或技术 Port，未列依赖、深层导入和环由 CI 拒绝。

| 模块 | 允许同步依赖的公开端口 | 权限 / 数据级别 | 失败语义与运行 Owner |
| --- | --- | --- | --- |
| `identity-access` | — | `workspace:admin`；受限身份/授权 | 默认拒绝、撤销优先；Security Ops |
| `project-catalog` | — | `project:create`、`project:manage`；内部项目元数据 | 乐观冲突或活动未清时拒绝；Project Ops |
| `story-development` | project-catalog、continuity、media-library | `content:draft:write`、`baseline:confirm`；机密正文 | 原始版本与候选保留，冲突不确认；Content Ops |
| `continuity` | — | `continuity:edit`、`continuity:assess`；机密叙事事实 | 不确定保持待定，不自动裁决；Continuity Ops |
| `creative-assets` | project-catalog、media-library | `asset:manage`；机密/声音等敏感引用 | 受限或过期版本阻止新绑定；Creative Ops |
| `media-library` | — | `media:upload`、`media:read`、`media:delete`；机密媒体 | 技术检查失败保持隔离；Media Ops |
| `storyboard` | project-catalog、story-development、continuity、creative-assets、media-library | `content:draft:write`、`storyboard:extraction:resolve`、`baseline:confirm`；机密镜头 | 上游过期使准备失效，不反写上游；Storyboard Ops |
| `agent-runtime` | model-catalog、production-jobs | `agent:run`、`agent:manage`；机密上下文/工具授权 | unknown 对账，取消和工具越权可判断；AI Ops |
| `model-catalog` | — | `workspace:policy:manage`；受限配置/Secret Ref | 无可用路由时阻止新工作；AI Platform Ops |
| `generation` | story-development、continuity、creative-assets、media-library、storyboard、model-catalog、production-jobs、cost-billing、compliance-governance | `production:generation:request`、`generation:consistency:resolve`；机密提示词/快照 | stale/预算/合规阻断，部分候选保留；Production Ops |
| `production-jobs` | — | `production:task:create`（Process Manager）、`production:task:cancel`、`production:task:retry`、`ops:reconcile`；受限供应引用 | 失联进入 unknown 并对账；Production Ops |
| `postproduction` | story-development、creative-assets、media-library、production-jobs、review-approval | `postproduction:edit`、`postproduction:render`；机密成片 | 固定快照重试，不改源版本；Media Ops |
| `review-approval` | — | `review:comment`、`review:decide`、`adoption:write`；机密意见 | 对象过期 supersede，决定只追加；Governance Ops |
| `cost-billing` | — | `budget:manage`、`finance:read`；受限财务 | 余额不足阻断，重复计量幂等对账；FinOps |
| `compliance-governance` | — | `compliance:review`、`compliance:policy:manage`、`privacy:manage`、`legal:hold`；受限法律/个人数据 | 外部检查失败 fail closed，Legal Hold 优先；Compliance Ops |
| `delivery` | media-library、postproduction、production-jobs、review-approval、compliance-governance | `delivery:create`、`delivery:release`；机密交付 | 任一门禁失效即阻断，新输入新快照；Delivery Ops |
| `operations-support` | —（仅消费事件契约） | `support:handle`、`ops:manage`、`analytics:read`；派生敏感数据 | 死信/延迟不阻塞核心写流程；Platform Ops |

| 场景 | 必须采用 | 禁止 |
| --- | --- | --- |
| 单模块状态变更 | 一个应用命令、一个本地事务，同时写本模块事实与 Outbox | Controller/Worker 直接使用 Prisma 写业务表 |
| 立即跨模块校验 | 调用公开 Query/Port，传稳定 ID、版本或快照 | 查他域表、导入他域 Repository/Domain Entity |
| 跨模块写流程 | 显式 Process Manager 依次调用幂等命令；每步独立事务并保存可判断进度/补偿 | 分布式事务、共享 Unit of Work、把多模块事实写成一次事务 |
| 状态传播和投影 | 版本化 Integration Event + Outbox/Inbox；至少一次、乱序可判断 | 进程内 EventEmitter 作为可靠边界、事件充当同步 RPC |
| 长时 AI/媒体/删除/交付 | Temporal Workflow 编排；副作用 Activity 只调用公开用例/Adapter | Workflow 访问数据库、网络、对象存储、账本或不确定 API |

## 7. 关键业务流程所有者

| 流程 | Process Manager 所有者 | 参与模块与失败语义 |
| --- | --- | --- |
| 来源到剧本基线 | `story-development` | `continuity` 返回版本化评估；冲突保持候选，不自动确认基线 |
| 剧本到分镜基线 | `storyboard` | 消费剧本/资产版本；过期时作废准备状态，不反写上游 |
| 生成提交与预算 | `generation` | 固定模型/合规/预算快照；预算预占成功后才请求 `production-jobs`，任务失败由租约释放孤立 Hold |
| Attempt 到候选与结算 | `generation` | Temporal 承载 `production-jobs→media-library→generation→cost-billing` 分步幂等；部分失败保留进度并对账 |
| 后期到交付 | `delivery` | 固定后期、采用和合规证据；任一门禁变化新建快照，不修改旧包 |
| 个人数据删除 | `compliance-governance` | Legal Hold 优先；各模块执行自己的删除/匿名化命令，案件汇总证据而不跨表删除 |

## 8. 模块内部结构与数据隔离

```text
backend/src/modules/<module>/
├── module.manifest.ts      # 名称、Owner、允许依赖、数据级别与运行入口
├── domain/                 # 聚合、值对象、领域规则和领域事件
├── application/           # commands、queries、process-managers、ports
├── public/                 # index.ts 唯一出口：Facade、Port Token、DTO、事件契约
├── infrastructure/        # Prisma Repository 与外部 Adapter
└── presentation/          # HTTP/事件/Activity 入口
```

Domain 只依赖自身和最小 Shared Kernel（ID、时间、金额、Result、事件信封）；Shared Kernel 不得放业务实体、通用 Repository 或跨域 DTO。其他模块只能导入 `public/index.ts`，不得深链其 contracts。Prisma 可共用 PostgreSQL，但模型、迁移责任和写 Repository 必须标注 owner；默认禁止跨模块 ORM Relation、外键和级联，例外必须新建 ADR，引用只存稳定 ID、版本或快照。

## 9. 运行服务与模块映射

运行制品为 `frontend`、`backend-api`、`backend-outbox-dispatcher`、确定性 `backend-workflow-worker`、隔离的 AI/媒体/运营 Activity Worker 和受控 `backend-migration-job`；共享提交但独立入口、IAM、队列、资源和恢复。Worker 结果只经公开用例写入，Workflow 保持 Replay 兼容，Migration 仅获兼容 DDL/回填权限。

| 模块 | 同步/Activity 入口 | Workflow / Task Queue | Frontend Feature |
| --- | --- | --- | --- |
| `identity-access` | API；运营 Worker 处理到期 | access-expiry | workspace-access、settings |
| `project-catalog` | API | — | projects |
| `story-development` | API、AI Worker | source-parse、story-plan | development |
| `continuity` | API、AI Worker | continuity-evaluate | continuity |
| `creative-assets` | API | — | assets |
| `media-library` | API、媒体 Worker | media-ingest、data-lifecycle | assets、media |
| `storyboard` | API、AI Worker | storyboard-extract | storyboard |
| `agent-runtime` | API、AI Worker | agent-run | agents |
| `model-catalog` | API、AI Worker | model-sync | settings、model-routing |
| `generation` | API、AI/媒体 Worker | generation-production | production |
| `production-jobs` | API/Webhook、AI/媒体 Worker | production-* | task-center（只读投影） |
| `postproduction` | API、AI/媒体 Worker | postproduction-* | postproduction |
| `review-approval` | API | review-access-expiry | review |
| `cost-billing` | API、运营 Worker | cost-reconcile | costs |
| `compliance-governance` | API、AI/媒体/运营 Worker | compliance-assess、data-lifecycle | compliance、privacy |
| `delivery` | API、媒体 Worker | delivery-build | delivery |
| `operations-support` | API、运营 Worker | notification、ops-projection | notifications、operations、analytics |

首发不引入消息总线：Dispatcher 按版本化 subscription registry 将 Workflow 命令或影响核心事实的集成事件 start/signal 到稳定 Temporal ID，将通知/运营/投影事件幂等写入 `consumer+event_id` Inbox 供运营 Worker 调用公开用例；任何核心状态变更必须由有业务 Owner 的 Workflow Activity 完成。

前端以 `frontend/src/features/<capability>/public.ts` 形成用户能力边界，`app/` 只组合路由；跨 Feature 只能经 `public.ts`，综合页/任务中心只读组合投影，写动作回到事实 owner。服务端状态只经 OpenAPI 生成客户端和 TanStack Query，Zustand 仅保存编辑会话。`deploy/` 分别声明上述制品的网络、身份、密钥、队列、资源和扩缩权限。

## 10. 演进、验证与验收

模块只有同时出现独立团队/SLO、显著不同扩缩、数据驻留或安全边界、故障隔离收益超过分布式成本时才候选拆服务；拆分必须新建 ADR，定义契约、历史迁移、运行中任务、无双写切换和回滚。

- AC-ARCH-007-001：FR-001～FR-021 的每项权威事实均可定位到且只定位到第 4 节一个所有者，重复或无主事实为失败。
- AC-ARCH-007-002：dependency-cruiser/ESLint 架构测试证明模块依赖图为 DAG，只能导入目标模块 `public/index.ts`，Domain 不依赖 Application/Infrastructure/框架。
- AC-ARCH-007-003：集成测试证明跨模块流程每步只提交一个模块事务；中途失败保留进度并可继续、补偿或人工处置。
- AC-ARCH-007-004：重复/乱序 Outbox、Inbox、Webhook、Activity 和 Temporal Replay 不重复 Task、媒体、账本、决定或交付。
- AC-ARCH-007-005：API、Dispatcher、Workflow、AI、媒体、运营和 Migration 制品可独立构建/运行，并按职责部署、限流、扩缩和恢复。
- AC-ARCH-007-006：授权撤销、对象版本过期、Legal Hold、删除和审计可跨模块追踪，任何投影删除后可由权威事实重建。

进入 PRD 前须共同评审模块命名、事实所有权、Process Manager、首发部署合并策略和拆分阈值。本设计处于 `review`，不授权创建 17 个空目录或按模块拆成 17 个服务；模块专属 Design 只在对应交付切片进入 Plan 前按需建立。
