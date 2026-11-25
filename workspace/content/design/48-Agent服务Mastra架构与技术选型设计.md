# Agent 服务 Mastra 架构与技术选型设计

状态：已确认架构与选型（依据 [需求33 SRC-AGV2-01 / SRC-AGV2-02](../requirement/33-Agent服务架构重构与技术选型需求.md#src-agv2-01) 与 [研究71](../research/71-2026-11-25-Agent服务架构网关与评测技术选型调研.md)）。本文自 2026-11-25 起作为 `agent/` 服务架构与技术选型的现行唯一技术合同，替代已归档的 [设计32](32-完整创作AgentHarness与Skill多Agent架构设计.md) 中的 Pi Durable 运行时选型与原设计37（NestJS/Fastify/Prisma/双向 OpenAPI 方案，已归档清理，历史见 Git）；既有创作业务需求继续由新架构承接。实施进度与调度只在 [BACKLOG](../../../BACKLOG.md) 维护。

## 1. 摘要

当前 `agent/` 服务在演进过程中叠加了 NestJS 12、Fastify 5、Swagger/OpenAPI、`oapi-codegen`、`@umijs/openapi`、通用 `Invoke` HTTP-over-gRPC 隧道、Prisma 7、Kysely 与小众运行时 `@earendil-works/pi-durable`，造成传输层双轨、数据层三套客户端并存、与主流 AI SDK / MCP / Eval 生态割裂等问题。

本设计采用**一步到位全量原地重构**，建立新一代 **Node 24 + TypeScript + Mastra** Agent 服务架构：

- **纯内核库 + 强类型 gRPC + 内部 MCP**：移除 NestJS/Fastify/OpenAPI 双轨，Mastra 作为纯 TypeScript 内核库运行；Go→Agent 与 Agent→Go 鉴权计费采用重构后的强类型 `runtime.proto` gRPC；Go 业务工具暴露为内部 MCP Server，由 `@mastra/mcp` 原生消费。
- **Go 统一渠道 AI 网关**：演进现有 Go `execution/gateway` 为厂商原生协议透传网关（OpenAI/Anthropic/Gemini）兼图/视/音异步多模态网关，无缝联动 `catalog` 与 `billing` 账本，同时支持将上游渠道 `Base URL` 灵活指向直连厂商、OpenRouter 或自托管 Bifrost。
- **静态 Schema 持久化与单一 SQL 构建器**：`@mastra/pg`（关闭运行时 DDL）管理框架记忆与 Workflow 快照，Kysely 管理服务级会话/运行/事件/租约表，移除 Prisma 与旧 `runtime_*` 表，全部表结构继续唯一归口于 `backend/db/schema.sql`。
- **混合编排 + 分离式记忆 + 声明式 Skill**：Producer 主 Agent 与专家子 Agent 处理动态探索，多阶段交付与收费审批走 Mastra Workflow 原生 `suspend/resume`；Mastra Memory 仅管对话与偏好，业务真源经 MCP 按需读取；Agent 容器内零脚本执行。
- **分层双轨 Evals 与可观测性**：开发/CI 使用 Mastra Scorers + Vitest 做轨迹与约束门禁；线上深度集成自托管 Langfuse 承接 OTel Trace、Prompt 版本管理与线上采样评分。

## 2. 参与者

| 参与者          | 职责                                                                           |
| --------------- | ------------------------------------------------------------------------------ |
| 项目所有者      | 确认全量原地重构边界、技术选型、网关/评测方案与切换窗口                        |
| Agent 维护者    | 清空重建 `agent/`（Mastra、gRPC、MCP、Kysely、Langfuse、Scorers 评测与门禁）   |
| Go 后端维护者   | 升级 `execution/gateway` 透传端点、实现内部 MCP Server、对接强类型 gRPC 契约   |
| 前端维护者      | 验证公共 `/api` 与对话 SSE 合同零破坏、审批交互与断线续传体验一致              |
| 运维 / DBA      | 维护 `backend/db/schema.sql` 离线变更、独立 `agent` 角色与自托管 Langfuse 部署 |

## 3. 背景

### 3.1 当前 `agent/` 架构痛点

基于 2026-11-25 对仓库 `agent/`、`backend/api/proto/glowloom/agent/runtime.proto` 与 `backend/internal/execution/` 的源码核查：

| 痛点维度     | 现状事实                                                                             | 工程代价                                                                     |
| ------------ | ------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| 传输层双轨   | 同时运行 NestJS/Fastify HTTP（4401）+ OpenAPI/oapi-codegen + gRPC `Invoke` HTTP 隧道 | 维护四套协议与生成链；gRPC 仅是 HTTP method/path/body_json 包装，失去强类型  |
| 核心生态小众 | 依赖 `@earendil-works/pi-durable` 1.5.0 及定制 `runtime_*` 持久化表                  | 与 Vercel AI SDK、MCP、主流 Prompt Cache/Thinking/Eval 生态脱节              |
| 数据层冗余   | `pg` + Prisma 7（`@prisma/adapter-pg`）+ Kysely 三套访问层并存                       | 仅 4–5 张核心控制表却维护完整 Prisma 生成链，构建与心智负担重                |
| 校验体系分裂 | `class-validator` / `class-transformer` + `TypeBox` 并存                             | 与 Mastra / Vercel AI SDK 原生 Zod 生态割裂                                  |
| 评测与观测弱 | 缺少标准化的轨迹评测（Trajectory Evals）与线上 Prompt 版本/Bad Case 闭环             | Prompt 迭代与多步工具调用回归难以量化拦截                                    |

### 3.2 不可绕过的约束

1. **数据库唯一真源**：`backend/db/schema.sql` 是仓库唯一 PostgreSQL 结构定义；严禁 `migrations/` 目录、自动迁移工具与服务启动/运行时动态 DDL。`agent` 继续使用独立 `agent` schema 与受限数据库角色。
2. **Go 单体权威边界**：身份认证、组织/项目授权、积分/预算预冻结与结算账本、权威业务数据（剧本、设定集、分镜、画布、素材）仍归 Go 模块化单体（[设计47](47-Go模块化单体架构与RPC集成设计.md)）。
3. **公共前端合同不变**：浏览器仍经 Frontend 同站代理访问 Go 公开 `/api` 与 SSE，不直接暴露内部 gRPC 或内部 MCP。
4. **编码规范**：Go 代码遵循 Google Go Style Guide 与 `adapter → application → domain`；Agent TypeScript 代码遵循 Google TypeScript Style Guide（strict ESM、扁平模块、禁止无职责 `common`/`utils` 与手写 barrel）。

## 4. 目标

### 4.1 设计目标

- 以 **Mastra + Vercel AI SDK + Kysely + Zod + `@grpc/grpc-js` + `@mastra/mcp`** 全量原地重建 `agent/`，彻底移除 NestJS、Fastify、Pi Durable、Prisma、TypeBox 与双向内部 OpenAPI 生成链。
- 将 `runtime.proto` 升级为强类型语义化 gRPC（`AgentRuntime` 与 `AgentBusiness`），并用 Go 内部 MCP Server 统一承载业务工具暴露。
- 演进 Go `execution/gateway` 为统一渠道网关（文本原生透传 + 图/视/音异步多模态），保真透传 Prompt Cache、Thinking 与流式块，并与计费账本强一致。
- 建立可断线重放（`Last-Event-ID`）、可崩溃恢复（PG 租约 + Mastra Workflow `suspend/resume`）、可量化回归（Mastra Scorers + Vitest）与全链路可观测（自托管 Langfuse）的生产级 Agent 底座。

### 4.2 非目标

- 不引入独立第三方重型 LLM 网关中间件作为强依赖（如 LiteLLM Python 容器）；若需接入外部聚合路由，仅作为 Go 渠道表中的上游 `Base URL` 配置项。
- 不在 `agent/` 容器内支持动态代码沙箱或第三方任意脚本执行。
- 不在本轮文档阶段直接修改业务源码、SQL、Proto 或执行线上部署。

## 5. 使用场景

| 场景                 | 触发与主路径                                                                        | 必须满足的结果                                                          |
| -------------------- | ----------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| 自由创作对话与委派   | 用户发消息 → Go `StartRun` gRPC → Mastra Producer Agent → 专家子 Agent / Go MCP     | 低延迟流式输出，支持多步工具调用与按需拉取最新项目数据，不出现脏缓存    |
| 多阶段交付与收费审批 | 触发分镜批量生成或高成本媒体任务 → Mastra Workflow `suspend()` → 前端审批 → `resume` | 挂起状态持久化在 PG，跨重启不丢；用户批准后经 Go 预冻结计费继续执行     |
| 断线刷新与多端重连   | 浏览器刷新携带 `Last-Event-ID` → Go `StreamRunEvents` gRPC → Agent 事件日志 + 总线  | 先补发 PG 持久化关键事件，再无缝切入实时流，进度与审批态不丢失          |
| 崩溃接管与并发防重   | `agent` 容器重启或同 Session 并发提交 → PG Run 租约锁 + 后台 Recovery               | 同 Session 串行执行；租约过期后 Workflow 自动可恢复，对话 Run 安全收敛  |
| 线上排障与离线回归   | 生产 OTel Trace 汇入 Langfuse → 坏 Case 沉淀 Dataset → PR 跑 Mastra Scorers/Vitest  | 全链路（Go → Agent → MCP → 网关）共享 `traceparent`，回归在 CI 自动拦截 |

## 6. 价值

- **工程复杂度大幅下降**：移除 NestJS、Fastify、Prisma、Pi Durable、TypeBox 及两套内部 OpenAPI 生成链，依赖面与构建时间显著收窄。
- **原生模型特性 100% 保真**：Go 原生协议透传 + Vercel AI SDK 完整保留 Prompt Caching、Thinking/Reasoning、结构化输出与多模态流式能力，同时不丢失云端多租户计费权威。
- **生态标准全面对齐**：拥抱 MCP 工具标准、Mastra Workflow/Memory/Scorers、OpenTelemetry 与 Langfuse，后续接入新模型、新工具与评测体系成本极低。

## 7. 方案

### 7.1 总体架构拓扑（已确认）

```mermaid
flowchart LR
  FE[Frontend<br/>Next.js :1122] <-->|HTTP / SSE| BE[Go 模块化单体<br/>backend :1123]

  subgraph Go 单体核心边界
    BE --- AUTH[identity / 权限 / 审批]
    BE --- MCP[内部 MCP Server<br/>业务工具与项目数据真源]
    BE --- GW[execution/gateway<br/>统一渠道网关 + catalog/billing]
  end

  BE <-->|强类型 gRPC<br/>AgentRuntime + StreamRunEvents| AG[Agent 服务<br/>Node 24 / TS / Mastra :4402]

  subgraph Agent 服务内核
    AG --- ORCH[Producer 主 Agent + 专家子 Agent<br/>+ Mastra Workflow suspend/resume]
    AG --- MEM[Mastra Memory<br/>对话窗口 / Working Memory / PgVector]
    AG --- CTRL[Kysely 控制层<br/>事件日志 / Run 租约 / 可插拔总线]
    AG --- SKILL[声明式 Skill 装配器<br/>SKILL.md + MCP 白名单]
  end

  AG -->|@mastra/mcp<br/>业务工具调用| MCP
  AG -->|强类型 gRPC<br/>AgentBusiness 鉴权/预授权/结算| BE
  AG -->|Vercel AI SDK<br/>OpenAI/Anthropic/Gemini 原生透传| GW
  AG <-->|独立 agent schema<br/>@mastra/pg + Kysely| PG[(PostgreSQL 18 + pgvector)]
  AG -.->|多实例实时 fan-out| RD[(Redis Pub/Sub)]
  AG -.->|OTel Trace / Prompt 拉取| LF[自托管 Langfuse]
  GW -->|上游渠道直连 / OpenRouter / Bifrost| UP[(LLM & 多模态供应商)]
```

### 7.2 技术选型与替换对照（已确认）

| 层级         | 目标选型                                                     | 替代 / 移除项                                            |
| ------------ | ------------------------------------------------------------ | -------------------------------------------------------- |
| 运行时与语言 | Node 24 + TypeScript strict ESM                              | —                                                        |
| Agent 框架   | **Mastra**（底层 **Vercel AI SDK**）                         | `@earendil-works/pi-durable`、`pi-ai`、`pi-coding-agent`、`chord` |
| 入站服务壳   | **Mastra 纯内核库 + `@grpc/grpc-js` + 极简 HTTP 健康探针**   | NestJS 12、Fastify 5、Swagger、`oapi-codegen`、旧 `Invoke` 隧道 |
| 工具回调协议 | **Go 内部 MCP Server + `@mastra/mcp`**                       | `@umijs/openapi` 生成的 HTTP `BusinessClient`            |
| 控制回调协议 | **强类型 `AgentBusiness` gRPC**（Buf 生成 Go + TS）          | HTTP 内部控制接口                                        |
| AI 网关      | **演进 Go `execution/gateway` 统一渠道网关**（参考 BeefTV）  | 零强绑定外部独立 LLM 网关                                |
| 数据访问层   | **`@mastra/pg`（禁用运行时 DDL）+ `Kysely` + `pg` + `Zod`**  | Prisma 7、`@prisma/adapter-pg`、`class-validator`、`TypeBox` |
| 实时事件总线 | **可插拔 EventBus（单实例内存 / 多实例 Redis Pub/Sub）**     | 仅进程内单播                                             |
| 可观测与评测 | **离线 Mastra Scorers + Vitest；线上自托管 Langfuse（OTel）** | 零散手写评测脚本与局部日志追踪                           |

### 7.3 跨服务协议设计（已确认方向，字段细化为设计建议）

1. **`backend/api/proto/glowloom/agent/runtime.proto`（唯一协议真源，Buf 管理）**：
   - `service AgentRuntime`（Go → Agent）：
     - 会话管理：`CreateSession`、`GetSession`、`ListSessions`、`UpdateSession`、`ArchiveSession`、`ListMessages`
     - 运行与控制：`StartRun`、`CancelRun`、`SubmitApproval`（驱动 Mastra Workflow `resume`）、`GetRun`
     - 流式订阅：`StreamRunEvents(run_id, last_event_id, identity)` → `stream RunEventFrame`
     - 探针：`CheckHealth`
   - `service AgentBusiness`（Agent → Go）：
     - `AuthorizeRun` / `ReserveBudget` / `ReportUsage`（与 Go 账本闭环）
     - `GetProjectDigest`（返回轻量项目目录与当前状态摘要，供 Run 启动注入）
     - `RecordApprovalAudit`（审批决策回写业务审计）
   - 鉴权：内部服务令牌 + audience 校验（非 local 环境叠加 mTLS），携带 W3C `traceparent` metadata。
2. **Go 内部 MCP Server（Agent → Go 业务工具）**：
   - 基于标准 MCP Streamable HTTP（内网监听，带服务令牌与每次调用的 `actor_id` / `project_id` / `org_id` / `run_id` 上下文透传）。
   - 暴露领域工具分组：项目/剧本读写、设定集（Bible）与主体库查询/更新、分镜与画布节点操作、媒体异步生成任务提交与状态查询。
   - 每个工具声明清晰 JSON Schema 与只读/写操作/计费副作用标签，供 Skill 白名单与审批拦截使用。

### 7.4 Go 统一渠道 AI 网关设计（已确认）

参照 [研究71](../research/71-2026-11-25-Agent服务架构网关与评测技术选型调研.md) 对 BeefTV 与 Toonflow 的同赛道拆解，演进 Go 现有 `backend/internal/execution/gateway`（衔接 [设计33](33-多设计模式驱动的模型与供应商接入扩展体系设计.md#ai-gateway)、[设计40](40-模型号池与销售计费设计.md)、[设计41](41-模型销售定价与结算设计.md)）：

- **文本 LLM 原生协议透传**：
  - 暴露内部兼容端点（OpenAI Responses/Chat、Anthropic Messages、Google Gemini GenerateContent），供 Mastra 内的 Vercel AI SDK Provider 直接指定 `baseURL` 调用。
  - **不做破坏性二次协议转译**：原样透传 Prompt Caching 字段、Thinking/Reasoning 块、Tool Call 流式增量与原生 `usage` 统计。
  - 请求头携带 `run_id` / `actor_id` / `org_id` / `budget_reservation_id`，Go 网关完成上游 Key 选择、限流与实时用量采集回写 `billing`。
- **上游渠道弹性配置**：渠道表中任意模型组均可配置直连官方端点、OpenRouter 或自托管 Bifrost 地址，实现按需扩展多上游容灾而不在架构上强绑定外部网关。
- **多模态异步生成统一收口**：文生图、图生视频、语音合成/克隆等异步任务继续由 Go `execution/gateway` + `operation` 统一处理提交、轮询、COS 落盘与计费，Agent 仅通过 MCP 工具发起与查询。

### 7.5 持久化、静态 Schema 与控制层（已确认）

1. **数据库边界**：
   - `backend/db/schema.sql` 统一声明 `agent` schema 下的两类表：
     - **Mastra 框架表**（threads、messages、resources/working_memory、workflow_snapshots、vector embeddings 等）：依据所锁定 `@mastra/pg` 版本的精确 DDL 静态写入 `schema.sql`；运行时显式关闭 `@mastra/pg` 的自动建表/初始化（`disableInit`），严禁运行时 DDL。
     - **服务控制表**（由 Kysely 管理）：`service_sessions`、`service_runs`（含 `lease_owner`、`lease_until`、`status`、`workflow_run_id`）、`service_run_events`（单调 `seq` 供 `Last-Event-ID` 重放）、`service_control_inbox`（审批/取消幂等收件箱）。
   - 彻底删除旧 Pi Durable 的 `runtime_sessions`、`runtime_records`、`runtime_document_versions`、`runtime_events` 表定义与 `agent/prisma/` 目录。
2. **并发、流式重放与恢复**：
   - **同 Session 串行租约**：Kysely 通过行锁（`FOR UPDATE`）与数据库时钟（`clock_timestamp()`）抢占 Run 租约并定期续期；未到期前拒绝重复执行。
   - **分层事件流**：结构化阶段事件（Run 状态、Tool 调用/结果、子 Agent 委派、Workflow 挂起/审批请求、最终消息）同步持久化至 `service_run_events`；高频 Token Delta 经可插拔 EventBus（单实例内存 / 多实例 Redis Pub/Sub）实时推送。客户端带 `last_event_id` 重连时先补 PG 历史，再切实时订阅。
   - **崩溃恢复**：后台 Recovery 扫描租约过期的未终结 Run——关联 Mastra Workflow 且处于可恢复状态者触发 `resume` 或安全转入等待态；普通自由对话 Run 标记为可重试终止并通知前端，避免静默重复扣费。

### 7.6 混合编排、Context/Memory 与声明式 Skill（已确认）

1. **混合编排**：
   - **Producer 主 Agent**：面向用户的总控入口，负责意图理解、上下文汇总、向领域专家子 Agent（编剧、设定、分镜、画布整理等）委派任务，以及调用关键 Workflow。
   - **Mastra Workflow（HITL 与长流程）**：涉及多阶段剧本拆解、批量角色/分镜资产生成、或触发收费多模态生成的链路封装为 Workflow；审批节点调用 `suspend()` 持久化快照并发出审批事件，用户确认后经 `SubmitApproval` RPC 触发 `resume()`。
2. **记忆与业务真源分离**：
   - Mastra Memory（最近消息窗口 + Working Memory 结构化摘要 + `PgVector` 语义召回）只存储**对话历史与用户创作偏好**。
   - 剧本、设定集、分镜、画布与媒体资产的权威真源留在 Go 业务库；每次 Run 仅经 `AgentBusiness.GetProjectDigest` 注入轻量摘要，细节由 Agent 通过 Go MCP 工具按需拉取。
3. **声明式 Skill 与安全边界**：
   - Skill 由声明式指令（`SKILL.md` / Langfuse Prompt 片段）+ 允许绑定的 MCP 工具白名单 + 可选 Workflow 模板组成，按任务意图动态装配。
   - `agent/` 容器内**禁止执行任何动态代码或第三方脚本**；所有写操作、媒体处理与外部集成统一经 Go MCP 走受控执行器。

### 7.7 可观测性、Prompt 治理与双轨 Evals（已确认）

1. **线上可观测与 Prompt 治理（自托管 Langfuse）**：
   - Mastra 通过官方 OTel / Langfuse Exporter 上报 Agent、Workflow、Tool/MCP 与 LLM Span，串联来自 Go 的 W3C `traceparent`。
   - 系统 Prompt 与角色指令托管在自托管 Langfuse 进行版本化管理与灰度切换，同时在 `agent/prompts/` 保留版本化本地兜底副本与内存 TTL 缓存，确保 Langfuse 不可用时不影响核心对话执行。
   - 线上坏 Case 一键归档至 Langfuse Dataset，并配置异步 Online Evals 采样评分（相关性、指令遵循、幻觉检测）。
2. **离线开发与 CI 回归门禁（Mastra Scorers + Vitest）**：
   - `agent/evals/` 使用 **Mastra Scorers + Vitest** 组织三层回归测试：
     - **确定性轨迹断言**：验证必调/禁调 MCP 工具顺序、参数 Schema 合法性、未触发越权写操作。
     - **成本与步数约束**：校验单次任务的迭代步数上限、禁止死循环。
     - **LLM-as-a-Judge**：在关键创作质量与审批判断集上执行 Rubric 评分（CI 默认用固定回放/可控预算模型，真实高成本模型按需触发）。

### 7.8 目标目录结构（设计建议）

延续 Google TypeScript 扁平两层包组织（`src/<package>/<module>.ts`），彻底消除 NestJS 与 Prisma 残留：

```text
agent/
├── prompts/                 # 本地兜底与种子 Prompt（线上权威为 Langfuse）
├── skills/                  # 声明式 SKILL.md 能力包与 MCP 白名单定义
├── evals/                   # Mastra Scorers + Vitest 离线轨迹与质量评测集
├── tests/                   # 单元、契约、隔离 PG 与恢复集成测试
└── src/
    ├── server.ts            # 进程入口：配置校验、PG/Redis/Langfuse 初始化、gRPC + 健康探针启动与优雅停机
    ├── config.ts            # Zod 环境变量校验
    ├── transport/           # gRPC Server（AgentRuntime）、极简 HTTP /healthz /readyz、鉴权与错误映射
    ├── business/            # AgentBusiness gRPC 客户端、Go MCP 客户端装配、Go 透传网关 Provider Factory
    ├── database/            # pg Pool、Kysely Database 类型映射、@mastra/pg（disableInit）封装
    ├── sessions/            # 会话/Run 生命周期、PG 租约锁、事件日志、可插拔 EventBus、崩溃恢复
    ├── mastra/              # Mastra 实例装配、Producer 主 Agent、专家子 Agent、HITL Workflows、Memory 配置
    ├── skills/              # 声明式 Skill 加载与 MCP 工具白名单过滤
    └── observability/       # Pino 脱敏日志、OTel + Langfuse Exporter、Prompt 拉取与本地缓存兜底
```

依赖方向严格单向无环：`transport → sessions → mastra → {skills, business, database, observability}`。

### 7.9 用户故事与来源追踪

| 故事 ID    | 角色       | 场景与目标                                                                   | 来源                      |
| ---------- | ---------- | ---------------------------------------------------------------------------- | ------------------------- |
| US-AGV2-01 | 创作者     | 发起创作对话并动态调用剧本/分镜/画布工具，始终基于最新项目数据而非过时缓存   | SRC-AGV2-02、FR-AGV2-02/05 |
| US-AGV2-02 | 创作者     | 触发高成本批量生成时收到清晰审批卡片，刷新页面或服务重启后批准仍能续跑完成   | SRC-AGV2-02、FR-AGV2-04/05 |
| US-AGV2-03 | 平台维护者 | 在 Go 渠道表灵活配置直连厂商或中转路由，所有文本与多模态消耗统一计入计费账本 | SRC-AGV2-02、FR-AGV2-03    |
| US-AGV2-04 | 研发维护者 | 在 PR 阶段通过轨迹评测拦截工具调用回归，在线上通过 Langfuse 追溯全链路 Trace | SRC-AGV2-02、FR-AGV2-06    |

### 7.10 待定问题

- **Langfuse 部署拓扑**：在 `docker-compose.yml` 中作为同栈服务启动，还是作为共享外部基础设施通过环境变量注入端点与密钥。
- **Redis Pub/Sub 启用时机**：首版单实例默认启用进程内 EventBus，是否同步在同一轮实现 Redis Pub/Sub 适配器并以配置开关切换。

## 8. 实施与验收

### 8.1 技术验收条件

1. **协议与依赖洁净**：`agent/package.json` 不含 NestJS、Fastify、Prisma、`@earendil-works/*`、TypeBox 与 `@umijs/openapi`；`runtime.proto` 通过 `buf lint` 且双端生成零漂移。
2. **数据库静态契约**：`backend/db/schema.sql` 完整声明 Mastra 框架表与 Kysely 服务控制表，无旧 `runtime_*` 表；`@mastra/pg` 在受限只读 DDL 权限的 `agent` 角色下冷启动与读写均正常，`backend/tests/db/schema_source_test.go` 通过。
3. **Go 网关与 MCP 闭环**：Go `execution/gateway` 原生透传端点在模拟/隔离测试中保留流式增量、Prompt Cache/Usage 统计并完成预冻结与结算；Go 内部 MCP Server 正确校验服务令牌与项目范围权限。
4. **编排、重放与恢复**：主/子 Agent 委派、Mastra Workflow `suspend/resume` 审批闭环、`Last-Event-ID` 断线补发与模拟进程中断后的租约过期恢复均通过隔离 PostgreSQL 集成测试。
5. **可观测与评测**：Mastra Scorers + Vitest 轨迹评测在离线测试中通过；Langfuse 不可达时 Prompt 本地缓存兜底不阻塞运行，可达时完整输出带 `traceparent` 的 Trace 树。

### 8.2 实施承接

产品范围与验收标准见唯一对应 [PRD34](../prd/34-Agent服务架构重构与技术选型.md)，全量原地重构步骤、文件责任、验证命令与回退边界见 [Plan43](../plan/43-Agent服务Mastra架构重构执行计划.md)，实施状态与调度见 [BACKLOG](../../../BACKLOG.md)。文档评审通过不代表源码改造或验收完成。

[设计导航](00-设计导航.md) · [需求33](../requirement/33-Agent服务架构重构与技术选型需求.md) · [PRD34](../prd/34-Agent服务架构重构与技术选型.md)
