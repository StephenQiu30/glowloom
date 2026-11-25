# Agent 服务架构重构与技术选型需求

## 1. 来源

<a id="src-agv2-01"></a>

### SRC-AGV2-01（2026-11-25）

用户原文：

> 你需要使用 /grill-me 来向我反复确认，我们当前的agent服务应该如何设计，从项目的架构和技术选型开始

来源：2026-11-25 架构与技术选型 `/grill-me` 访谈。对应调研依据见 [研究71](../research/71-2026-11-25-Agent服务架构网关与评测技术选型调研.md)，技术方案见 [设计48](../design/48-Agent服务Mastra架构与技术选型设计.md)，验收范围见 [PRD34](../prd/34-Agent服务架构重构与技术选型.md)，实施计划见 [Plan43](../plan/43-Agent服务Mastra架构重构执行计划.md)。

<a id="src-agv2-02"></a>

### SRC-AGV2-02（2026-11-25，`/grill-me` 逐项确认结论）

经逐层架构与技术选型确认，以下核心决策已定案：

1. **重构边界**：彻底重新选型，不受现有 NestJS、Pi Durable（`@earendil-works/pi-durable`）、Prisma 与双轨 HTTP/OpenAPI 历史约束，采用**一步到位全量原地重构**（先更新文档、`backend/db/schema.sql` 与 `runtime.proto`，再同步改造 Go 网关/MCP 与清空重建 `agent/`，不保留新旧双轨兼容层）。
2. **系统边界**：保留独立 `agent/` 服务与 Go 模块化单体（`backend`）分离部署。`agent/` 专注会话、多 Agent 编排、Workflow 持久化恢复、对话记忆与声明式 Skill；Go 单体负责身份鉴权、组织/项目权限、计费配额账本、权威业务数据、统一渠道 AI 网关与内部 MCP 业务工具服务器。
3. **语言与核心框架**：采用 **TypeScript (strict ESM) + Node 24**，核心 Agent 框架选型为 **Mastra**（基于 Vercel AI SDK）。
4. **传输层与跨服务协议**：
   - 移除 NestJS、Fastify、Swagger/OpenAPI、`oapi-codegen` 与旧 `Invoke` HTTP-over-gRPC 隧道；Mastra 以纯内核库形态运行，仅保留极简 HTTP 健康探针（`/healthz`、`/readyz`）。
   - **Go → Agent**：重构 `backend/api/proto/glowloom/agent/runtime.proto` 为强类型语义化 gRPC（含 Server Streaming）。
   - **Agent → Go 业务工具**：Go 暴露内部 MCP Server，Agent 通过 `@mastra/mcp` 发现与调用。
   - **Agent → Go 鉴权/配额/结算**：走强类型 `AgentBusiness` gRPC。
5. **AI 网关路线**：参考同赛道项目（BeefTV / Toonflow）实践与云端多租户账本需求，演进 Go 现有 `execution/gateway` 为**统一渠道网关**——文本 LLM 提供 OpenAI/Anthropic/Gemini 原生协议透传（100% 保留 Prompt Cache / Thinking / 流式特性），同时统一承接图/视/音异步多模态生成与多租户预冻结/结算账本；任意上游渠道的 `Base URL` 可灵活指向直连厂商、OpenRouter 或自托管 Bifrost，不强绑定外部独立网关中间件。
6. **数据持久化与访问层**：沿用 PostgreSQL 独立 `agent` schema 与仓库“唯一 `backend/db/schema.sql`、禁止运行时 DDL”红线；采用 **`@mastra/pg`（关闭运行时自动建表，框架表静态声明在 `schema.sql`）+ `Kysely`（管理服务级控制表）+ `pg` + `Zod`**，移除 Prisma、`class-validator`、`TypeBox` 与旧 Pi Durable `runtime_*` 表。
7. **多 Agent 与 HITL 编排**：采用**混合编排模式**——Producer 主 Agent 委派专家子 Agent 处理日常创作与探索；涉及多阶段交付、收费生成与用户审批（HITL）的关键链路封装为 **Mastra Workflow**，利用原生 `suspend()` / `resume()` 实现挂起与崩溃恢复。
8. **Context 与 Memory 边界**：**记忆与业务真源分离**——Mastra Memory（对话窗口 + Working Memory + PgVector 语义检索）只管对话历史与创作偏好；剧本、设定集、分镜、画布等权威业务实体不复制到 Agent 向量库，Run 启动时仅注入轻量项目摘要，细节通过 Go MCP 按需拉取，杜绝脏缓存幻觉。
9. **流式重放、并发与崩溃恢复**：采用 **PostgreSQL 关键事件日志**（支持 `Last-Event-ID` 断线重放）+ **PG Run 租约锁**（防同 Session 并发与崩溃接管）+ **可插拔实时事件总线**（单实例内存 / 多实例 Redis Pub/Sub）。
10. **Skill 与安全边界**：采用**声明式 Skill**（`SKILL.md` / 指令包 + MCP 工具白名单），按任务动态装配给 Mastra Agent/子 Agent；`agent/` 容器内**零脚本执行**，所有副作用与重计算统一经 Go MCP 收口到受控执行器。
11. **可观测性与 Evals**：采用**分层双轨**——开发与 CI 回归门禁使用 **Mastra 原生 Scorers + Vitest**（工具调用轨迹断言、步数/成本上限、结构化输出 + LLM-as-a-Judge）；线上使用**自托管 Langfuse** 承接 OTel Trace、Prompt 版本管理（带本地兜底缓存）、坏 Case 沉淀 Dataset 与线上采样评分。

## 2. 明确要求

| ID          | 要求                                                                     | 确认边界                                                                                             |
| ----------- | ------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------- |
| FR-AGV2-01  | 基于 Node 24 + TypeScript + Mastra 原地重建独立 `agent/` 服务             | 移除 NestJS、Fastify、Pi Durable、Prisma、TypeBox 与 OpenAPI 双向胶水，不保留旧实现兼容分支          |
| FR-AGV2-02  | 重构 Go ↔ Agent 跨服务通信为“强类型 gRPC + 内部 MCP 业务工具”            | Go→Agent 与 Agent→Go 鉴权/计费走强类型 Protobuf/gRPC；Go 业务工具暴露为内部 MCP 供 `@mastra/mcp` 消费 |
| FR-AGV2-03  | 演进 Go `execution/gateway` 为统一渠道 AI 网关                           | 支持文本 LLM 原生协议透传与图/视/音异步多模态生成，联动 `catalog`/`billing` 账本，上游可灵活配置中转 |
| FR-AGV2-04  | 落地混合编排（主/子 Agent + Mastra Workflow HITL）与声明式 Skill          | 收费与多阶段审批走 Workflow `suspend/resume`；Skill 仅声明式装配，Agent 容器内禁止执行第三方脚本     |
| FR-AGV2-05  | 落地分离式记忆、可续传事件流与崩溃恢复                                   | Mastra Memory 与业务真源分离；PG 关键事件日志 + Run 租约锁 + 可插拔内存/Redis 实时总线                |
| FR-AGV2-06  | 落地“离线 CI 轨迹评测 + 线上 Langfuse 观测与 Prompt 治理”双轨体系        | CI 跑 Mastra Scorers + Vitest；线上对接自托管 Langfuse（OTel Trace / Prompt 管理 / Dataset / 采样）  |
| NFR-AGV2-01 | 严格遵守仓库数据库与工程红线                                             | `backend/db/schema.sql` 唯一结构真源，关闭 `@mastra/pg` 运行时 DDL；遵循 Google Go / Google TS 规范  |
| BR-AGV2-01  | 保持既有前端公共 `/api`、SSE 对话体验与创作业务能力不回退                | 承接 [需求19](19-完整创作Agent与Skill多Agent需求.md) 的创作业务目标，替代其底层 Pi/NestJS 技术选型   |

## 3. 需求边界

已确认方向：

- 本需求在**技术架构与选型层**明确替代原 NestJS/Prisma 目录规范需求（原需求22已归档清理，历史见 Git）及 [设计32](../design/32-完整创作AgentHarness与Skill多Agent架构设计.md) 中的 Pi Durable 运行时选型；原有创作业务场景（剧本/设定/分镜/画布/审批/计费）继续有效并由新架构承接。
- 先按 `workspace/` 知识库规范沉淀完整文档链（需求33 → 设计48 → PRD34 → Plan43 + 研究71），待确认开工后按“一步到位全量原地重构”推进源码、Schema、Proto 与 Compose 改造。

待确认事项（须在代码改造前明确）：

- 自托管 Langfuse 在 `docker-compose.yml` 中的部署方式（同栈 Compose 编排 vs 复用外部独立部署实例）。
- 存量开发环境数据库 `agent` schema 的离线重置窗口（清空旧 `runtime_*` 表并按新 `schema.sql` 重建）。

## 4. 验收条件

| ID         | 可检查结果                                                                                           | 对应要求                |
| ---------- | ---------------------------------------------------------------------------------------------------- | ----------------------- |
| AC-AGV2-01 | 完成知识库需求33、设计48、PRD34、Plan43、研究71 及全部导航/索引更新，通过 Workspace 校验无断链        | SRC-AGV2-01、SRC-AGV2-02 |
| AC-AGV2-02 | `runtime.proto` 升级为强类型语义化 RPC；`agent/` 移除 NestJS/Pi Durable/Prisma 并以 Mastra 稳定运行  | FR-AGV2-01、FR-AGV2-02  |
| AC-AGV2-03 | Go 内部 MCP Server 暴露核心业务工具，`@mastra/mcp` 可动态发现并完成受控鉴权调用                      | FR-AGV2-02、FR-AGV2-04  |
| AC-AGV2-04 | Go `execution/gateway` 支持文本原生协议透传（保留 Prompt Cache/Thinking/流式）并闭环计费预冻结与结算 | FR-AGV2-03              |
| AC-AGV2-05 | `@mastra/pg` 关闭运行时 DDL，全部框架表与服务表静态声明于 `backend/db/schema.sql` 且通过 DB 门禁     | NFR-AGV2-01             |
| AC-AGV2-06 | 混合编排（主/子 Agent + Workflow `suspend/resume`）、断线 `Last-Event-ID` 重放与崩溃租约恢复验证通过 | FR-AGV2-04、FR-AGV2-05  |
| AC-AGV2-07 | Mastra Scorers + Vitest 离线轨迹评测门禁与自托管 Langfuse OTel Trace / Prompt 兜底链路验证通过       | FR-AGV2-06、BR-AGV2-01  |

[需求导航](00-需求导航.md) · [设计48](../design/48-Agent服务Mastra架构与技术选型设计.md)
