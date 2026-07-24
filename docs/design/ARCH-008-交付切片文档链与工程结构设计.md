---
layer: Design
doc_type: Delivery Slicing and Engineering Structure Design
doc_no: ARCH-008
title: 交付切片、阶段文档链与工程结构设计
status: review
version: 0.1.0
owner: Lanverse
audience: [Product, PM, Architecture, Frontend, Backend, QA, Security, Operations]
feature_area: 项目交付治理与工程结构
purpose: 把全平台需求拆为可独立设计、实现、验证和回滚的纵向切片
canonical_path: docs/design/ARCH-008-交付切片文档链与工程结构设计.md
inputs: [SRS-001, FR-001至FR-021, NFR-001, TCR-001至TCR-003, ADG-001, ARCH-001至ARCH-007, ADR-001至ADR-002]
outputs: [纵向切片, 阶段文档链, 需求追踪规则, 技术基线, 工程目录, 实现顺序]
triggers: [产品范围变化, 交付顺序变化, 工程目录变化, 技术栈变化, 阶段门禁变化]
updated: 2026-07-25
downstream: [ARCH-101至ARCH-107, PRD-001至PRD-007, PLAN-001至PLAN-007, ACC-001至ACC-007]
---

# ARCH-008 交付切片、阶段文档链与工程结构设计

## 1. 设计结论与当前状态

Lanverse 采用 7 个可演示的纵向切片交付完整 P0 项目。每个切片必须独立完成 `Design → PRD → Plan → test/impl/refactor → Acceptance`，并只在上游文档为 `accepted` 后进入下一阶段；平台级 ARCH/ADR 是共同基线，不能替代切片专属设计。

当前 Requirement、ARCH、ADR 与 TRACE 均处于 `review`，因此本文和切片文档清单只形成 Design 评审输入，不授权创建应用目录、引入依赖或预建 PRD/Plan/Acceptance。Acceptance 文件只能在对应实现完成后创建；PRD 和 Plan 只预定义其编号及责任，不能用空文件伪装进度。

## 2. 阶段契约

| 阶段 | 准入 | 必须回答 | 长期输出 | 退出条件 |
| --- | --- | --- | --- | --- |
| Design | 适用 Requirement 至少为 `review` 且无已知 P0 冲突 | 架构、接口、数据、状态、权限、失败、迁移、回滚如何工作 | `ARCH-1xx`、ADR、设计验收项 | Requirement 已接受，Design 评审通过并为 `accepted` |
| PRD | 引用的全局与切片 Design 均为 `accepted` | 谁在什么场景获得何种价值，范围/非目标/规则/SMART 结果是什么 | `PRD-00x`、用户故事、`AC-PRD-*` | 产品、研发、QA 与适用治理责任方接受 |
| Plan | Design 与 PRD 均为 `accepted` | 按什么顺序写测试、实现、迁移、验证和回滚，依赖与风险如何关闭 | `PLAN-00x`、任务/测试/证据清单 | 每项任务可执行、可验证、有依赖和完成定义并被接受 |
| Implementation | Plan 为 `accepted` | 如何用最小 Red→Green→Refactor 满足已接受契约 | 代码、迁移、测试、制品、运行手册 | 目标验证完成且实现范围未漂移 |
| Acceptance | 对应实现已完成 | 每个 Design/PRD/Plan 条目由什么命令和事实证据证明 | `ACC-00x`、证据、风险与结论 | 全部 P0 为 `passed`；否则 `failed` 或明确风险接受 |

上游变更时，下游先标记 `stale/待复核`；更新、评审并重新接受后才能继续。文档状态只使用 `draft/review/accepted/archived`，验证结果只使用 `passed/failed/insufficient/not_applicable`，二者不得混用。

## 3. SMART 与双向追踪

每个产品验收项必须同时具备主体与场景、可观测行为、量化阈值、事实源、测试环境/样本、排除规则和最迟验证门禁；无法满足这些字段的描述不是可执行需求。

统一追踪链为：

```text
Requirement ID
  → ARCH/ADR + Design AC
  → PRD User Story + AC-PRD
  → PLAN Task + Test ID + Evidence ID
  → implementation path/commit
  → ACC Result + Evidence URI + residual risk
```

同一 P0 Requirement 可以作为多个切片的约束，但必须且只能有一个“能力完成主切片”；其他切片标记为 `foundation`、`dependency` 或 `hardening`，避免重复宣称完成。TRACE-001 保存跨文档索引，具体 Acceptance 保存运行结果。

## 4. 纵向切片与依赖

| 切片 | 可演示用户闭环 | 能力完成主范围 | 必须复用/前置 | 关键失败信号 |
| --- | --- | --- | --- | --- |
| S0 可信项目入口 | OIDC 登录→选择工作空间→创建项目/分集→审计可追踪 | FR-001～002；平台工程基线 | 无；先完成本地等价依赖、OpenAPI、观测、架构测试 | 越权、跨租户、不可复现构建或无健康证据 |
| S1 来源到剧本基线 | 导入获权内容→Agent 提案→人工确认事件/策划→确认剧本基线 | FR-003～005；FR-021 P0 | S0；建立 FR-018 权利声明最小入口但不宣称合规能力完成 | AI 结果成为事实、来源不可追溯、并发静默覆盖 |
| S2 资产到分镜准备 | 上传隔离→登记资产版本→拆镜提案→人工处理候选→确认分镜准备 | FR-006～008；FR-007 媒体谱系 | S1；媒体扫描、版本与 timebase 基础 | 未扫描媒体可生产、旧版本被覆盖、准备状态混写 |
| S3 可计费生成闭环 | 选择能力→成本/权利预检→提交→持久任务→候选→人工采用 | FR-009～011；FR-017 P0 | S2；Provider Stub 和至少一项已批准能力；建立 FR-018 生成执行门禁但不宣称合规能力完成 | 重复扣费、任务丢失、Preview 漂移、迟到结果误采用 |
| S4 音画后期闭环 | 配音/音效/字幕→非破坏时间线→可审粗剪 | FR-012～015 | S3；媒体 Worker、快照和局部重做 | 浮点时间漂移、破坏性编辑、浏览器预览改变成片 |
| S5 审核合规交付 | 审片整改→批准→合规证据→质检→版本化交付包 | FR-016、FR-018～019 | S4；采用、权利、安全、标识和交付快照；硬化 FR-017 最终对账 | 普通审批绕过强制门禁、残缺 Manifest 被交付 |
| S6 运营与首发硬化 | 通知/处置/分析→全链路样片→恢复/回滚→Beta 发布判断 | FR-020；SRS MVP 指标；NFR-001 全量硬化 | S0～S5；版本矩阵、容量 B、运行手册 | NFR 证据不足、恢复超时、旧客户端或 Workflow 不兼容 |

`cost-billing` 与 `compliance-governance` 不再整体推迟到最后：S1 建立权利声明入口，S3 建立生成前预算与合规门禁，S5 完成正式交付证据，S6 做全量故障和规模验证。这样 S3 不会依赖尚未实现的 S5 门禁。

## 5. 计划文档包

| 切片 | 切片 Design（实现前建立并接受） | 后续 PRD | 后续 Plan | 实现后 Acceptance |
| --- | --- | --- | --- | --- |
| S0 | ARCH-101 可信项目入口与工程基础设计 | PRD-001 | PLAN-001 | ACC-001 |
| S1 | ARCH-102 来源到剧本基线设计 | PRD-002 | PLAN-002 | ACC-002 |
| S2 | ARCH-103 资产与分镜准备设计 | PRD-003 | PLAN-003 | ACC-003 |
| S3 | ARCH-104 可计费生成闭环设计 | PRD-004 | PLAN-004 | ACC-004 |
| S4 | ARCH-105 音画后期设计 | PRD-005 | PLAN-005 | ACC-005 |
| S5 | ARCH-106 审核合规交付设计 | PRD-006 | PLAN-006 | ACC-006 |
| S6 | ARCH-107 运营与首发硬化设计 | PRD-007 | PLAN-007 | ACC-007 |

每个文档包只在其准入成立时按需创建。切片 Design 必须列出适用 Requirement 条款而非只引用整份 FR；PRD 必须逐项关联 Design AC；Plan 必须给出测试 ID、命令、目标路径和证据位置；Acceptance 必须回填实际版本与结果。

## 6. 技术栈基线

| 关注点 | 首发选择 | 冻结规则 |
| --- | --- | --- |
| 语言与仓库 | TypeScript strict、Node.js 24 LTS、pnpm workspace | S0 Plan 复核受支持版本并锁定精确依赖与唯一 lockfile |
| Web | React、Next.js App Router、Tailwind CSS、shadcn/ui | Server Component 默认；交互/媒体编辑按 Client Island |
| 前端状态 | TanStack Query、Zustand、React Hook Form、Zod | 服务端事实、编辑会话、表单和 UI 状态分层 |
| API/后端 | NestJS、Prisma、PostgreSQL、REST/JSON、OpenAPI 3.1、SSE | OpenAPI 是服务端、生成客户端和契约测试共同事实源 |
| 异步与集成 | Temporal TypeScript、PostgreSQL Outbox/Inbox、Provider Adapter | Workflow 确定；副作用在 Activity；业务事实仍回写所有者用例 |
| 媒体与缓存 | S3 兼容对象存储、FFmpeg/ffprobe、Redis | 媒体二进制不进业务库/Workflow；Redis 不作事实源 |
| 身份与安全 | OIDC Authorization Code + PKCE、安全 HttpOnly 会话 | 具体 IdP、地区、密钥/KMS 与托管产品在适用切片 Design/ADR 决定 |
| 质量与交付 | Vitest、Testing Library、Playwright、OpenTelemetry、OCI、GitHub Actions、IaC | 精确工具版本、覆盖目标和平台产品由接受的 Plan 固定 |

技术族替换、事实所有权变化、跨服务事务或部署边界变化必须新建 ADR；补丁升级在不改变契约和运行边界时走常规依赖更新。

## 7. 接受后的目标工程结构

```text
Lanverse/
├── backend/
│   ├── contracts/{openapi,events}/
│   ├── prisma/{schema.prisma,migrations}/
│   ├── src/
│   │   ├── entrypoints/{api,dispatcher,workflow-worker,ai-worker,media-worker,operations-worker}/
│   │   ├── modules/<module>/{domain,application,infrastructure,transport,public}/
│   │   ├── workflows/
│   │   ├── integrations/
│   │   └── shared-kernel/
│   └── test/{architecture,contract,integration,fixtures}/
├── frontend/
│   ├── src/
│   │   ├── app/{(public),(workspace),(review)}/
│   │   ├── features/<capability>/
│   │   ├── components/ui/
│   │   ├── services/generated/
│   │   └── stores/
│   └── e2e/
├── deploy/{local,iac,observability}/
├── docs/{requirement,design,prd,plans,acceptance,operations}/
├── package.json
├── pnpm-workspace.yaml
└── pnpm-lock.yaml
```

模块 `public/` 是后端唯一跨模块导入面，Feature `public.ts` 是前端唯一跨 Feature 导入面。`entrypoints` 只装配用例；领域层不依赖 NestJS、Prisma、Temporal、HTTP 或供应商 SDK。根目录不得新增顶层 `apps/`、`packages/` 或独立 Worker 仓库；目录只由已接受 Plan 中的首个真实用例创建。

## 8. 实施与验证顺序

每个 Plan 先定义最小可失败验收：契约/状态机/权限/架构测试形成 Red；随后仅实现该切片所需公开契约、事实与 UI 形成 Green；最后消除重复并保持测试全绿。提交顺序为 `test:` → `impl:`/`feat:` → 可选 `refactor:`/`docs:`/`chore:`。

验证从目标单元/契约测试开始，再执行类型、lint、构建、真实依赖集成、关键 Playwright 流程和风险匹配的安全/性能/恢复检查。跨模块写入、Outbox、Workflow、费用和交付必须注入失败；UI 变更必须保存浏览器关键路径证据。

数据库遵循 `expand → migrate/backfill → switch → contract`；公开契约保持当前及前一版本兼容；回滚只撤回制品和入口，不删除已提交的任务、费用、决定、媒体、合规或交付事实。

## 9. 渐进决策门禁

全局 `design_entry` 前必须确认首发地区/数据驻留原则、容量假设责任人及最迟决策点、媒体基准规格、身份边界和托管/自管选择准则。具体 IdP、对象存储、Temporal/PostgreSQL 托管产品、AI Provider、价格与失败计费只须在首次使用它们的切片 Design 转为 `accepted` 前关闭，不阻塞不依赖该产品的更早切片评审。

如某项 P0 决策只能条件接受，评审记录必须给出 owner、截止门禁、候选范围、默认安全行为和回退方案；涉及安全、数据完整性或不可逆迁移的缺口不得条件放行。

## 10. Design 验收标准

- AC-ARCH-008-001：FR-001～FR-021、SRS MVP 指标和 NFR-001 均能定位到一个能力完成主切片，跨切片约束标明依赖类型。
- AC-ARCH-008-002：S0～S6 每个切片都有用户可观察闭环、权威事实、失败信号、前置依赖和可独立回滚边界。
- AC-ARCH-008-003：成本和合规能力在生成前提供必要门禁，不存在 S3 依赖尚未交付 S5 的顺序倒置。
- AC-ARCH-008-004：阶段文档链能从任意 P0 Requirement 双向追踪到计划测试和实现后证据，且未提前创建 Acceptance。
- AC-ARCH-008-005：目标目录符合 `backend/frontend/deploy` 根边界、17 模块所有权与独立运行制品约束，并可由架构测试验证。
- AC-ARCH-008-006：Requirement 接受只确认目标和测量契约；运行样片、压测、故障及恢复证据只在已接受 Plan 后产生并阻断对应发布，而不形成准入死锁。
