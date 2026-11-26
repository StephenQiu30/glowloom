# Agent 服务架构与技术选型调研

> 观察日期：2026-11-25 ～ 2026-11-26  
> 研究对象：生产级 Agent 框架（Mastra / `@earendil-works/pi-durable` / Vercel AI SDK）、AI 网关（Bifrost / LiteLLM / Portkey / New-API）、同赛道开源项目（[`glanderness/BeefTV`](https://github.com/glanderness/BeefTV) / [`HBAI-Ltd/Toonflow-app`](https://github.com/HBAI-Ltd/Toonflow-app)）、异步编排引擎（Temporal / Restate / DBOS）与双轨 Evals  
> 关联文档：[需求33](../requirement/33-Agent服务架构重构与技术选型需求.md) · [设计48](../design/48-Agent服务Mastra架构与技术选型设计.md) · [PRD34](../prd/34-Agent服务架构重构与技术选型.md) · [Plan43](../plan/43-Agent服务Mastra架构重构执行计划.md)  
> 适用边界：本文记录公开源码事实、架构对比与选型依据，不替代用户确认，不作为工程实现或验收证明。

## 1. 生产级 AI Agent 服务语言与核心框架选型

### 1.1 语言生态格局

生产级 Agent 服务呈现 **TypeScript 与 Python 双主流**，Go 主要用于下层业务网关、计费与高并发基础设施：

| 维度 | TypeScript 阵营（SaaS / Web 产品内置 Agent 主流） | Python 阵营（复杂图编排 / 数据与 ML 主流） |
| --- | --- | --- |
| **代表框架** | Vercel AI SDK、Mastra、LangGraph.js、官方 TS SDK + 轻量自研 Harness | LangGraph、PydanticAI、Google ADK、OpenAI/Claude SDK |
| **服务壳趋势** | 趋向轻量化（Hono / Fastify / 纯内核库 + gRPC），重型 NestJS 较少见 | FastAPI 为绝对主流 |
| **持久化恢复** | Postgres Checkpoint（`@mastra/pg`）、自建租约表、Restate/Temporal | LangGraph Postgres Checkpointer、Temporal + PydanticAI |
| **核心优势** | 与 Next.js/React 同语言、端到端类型安全、流式与 MCP 生态一线 | 模型与数据科学包最全、复杂有状态图编排生态丰富 |

### 1.2 核心框架横向复评：Mastra vs `pi-durable` vs 轻量自研 Harness

| 对比维度 | Mastra + 轻量 Harness 边界层（选定） | `@earendil-works/pi-durable` 1.6.0（BeefTV 方案） | 纯 Vercel AI SDK 轻量自研 Harness |
| --- | --- | --- | --- |
| **云端多租户与 PG 静态 Schema** | 良好：`@mastra/pg` 关闭运行时 DDL（`disableInit: true`），与 Kysely 共享 `agent` schema | 较弱：一等公民为单机 SQLite/Actor，PG 需定制适配且实验期表结构易变 | 极佳：100% 由 Kysely + `backend/db/schema.sql` 自主掌控 |
| **崩溃恢复与持久执行粒度** | 中等偏强：Workflow 原生 Step 级 `suspend/resume`；自由对话靠 Kysely 租约 + Go `operationId` 幂等 | 极强：原生 Entry 级 Conversation Ledger checkpoint/replay 与分叉 | 可控但需自建 Turn/Step 状态机与快照恢复 |
| **LLM 出网边界控制力** | 强：底层为 Vercel AI SDK，通过原生 `wrapLanguageModel`（`LanguageModelMiddleware`）与 `timeout.{totalMs,stepMs,chunkMs}` 完全掌控 Cache 锚点、流式空闲与预算，**零手写 `fetch`** | 强：提供 `beforeProviderRequest` 钩子与 `fetch` 代理 | 极强：零框架中间层，直接控制 `streamText` / `prepareStep` |
| **多协议原生能力与主流生态** | 极强：原生复用 `@ai-sdk/openai` / `@ai-sdk/anthropic` / `@ai-sdk/google`、MCP、Langfuse、Scorers | 较弱：绑定 `@earendil-works/*` 小众生态，模型能力需手写大量正则推断 | 极强：直接对接 Vercel AI SDK、MCP SDK 与 Langfuse OTel |
| **迁移与演进风险** | 低成本：已按 [设计48](../design/48-Agent服务Mastra架构与技术选型设计.md) 落地；BeefTV 全部 6 项模式可落在窄边界层无痛接入 | 高返工风险：实验性包，且已在设计48主动迁出 | 中等成本：需重写 `mastra/` 层，可作为未来去框架的清晰退路 |

**选型结论**：保持 **Mastra + Vercel AI SDK** 为主底座，明确不回退 `pi-durable`，且严格执行**全部出网零手写 `fetch/http`** 红线；通过独立于框架的两个窄边界层——**模型中间件层（`business/gateway-provider.ts` + `wrapLanguageModel`）**与**MCP 工具包装层（`business/canvas-bridge.ts`）**——完整吸收 BeefTV 工程模式。

## 2. BeefTV Agent 服务源码拆解与可复用工程模式

BeefTV 定位**本地优先桌面工作台**（Wails + Go + SQLite），内置创作助手 `agent-host/` 作为 Go 托管的 Node ESM 子进程运行，通过本地 HTTP / Unix Socket（`BEEFTV_OPS_PIPE`）与 Go 后端协作。剥离单机桌面特有约束（SQLite WAL、`BEEFTV_OPS_PIPE`、`X-Desktop-Token` 本地信任模型）后，其在 AI 影视/画布创作 Agent 上沉淀了 6 项高价值模式：

| 维度 | BeefTV 源码模块 | 核心工程机制 | Glowloom 零手写出网落地方式 |
| --- | --- | --- | --- |
| **有界画布读取视图** | `canvas-read-view.mjs` | 模型读取强制 `<40KB` UTF-8，返回 `revision` + 有界节点索引 + 分页 + JSON Pointer（`fieldPath`）按需下钻；截断字段显式标注 `complete: false` | Go/Canvas 负责权威分页与指针读取，Agent MCP Wrapper 做 `<40KB` 裁剪与标记 |
| **同轮写链 Rebase 与冲突熔断** | `operation-bridge.mjs` / `canvas-turn.mjs` | 同轮并行局部写操作（`REBASABLE_WRITES`）自动 Rebase 到最新 `revision`；整文档提交不 Rebase；外部变更立即触发 `canvas_context_changed` 熔断 | Go/Canvas 守 `expectedRevision` CAS，Agent MCP Wrapper 维护同轮写链 Rebase |
| **两阶段付费审批与防重扣费** | Go `operations/` + `agentops/` + `toolOperationId` | 付费生成走 `propose → confirm → submit` + 确定性 `operationId`，崩溃重放直接回读原任务 | Go 权威操作层强校验确定性 `operationId` 幂等与计费账本闭环 |
| **异步媒体任务确定性等待** | `await-business-tasks.mjs` + `task.wait` | 生图/生视频（20s–135s+）在工具内通过服务端长轮询分片（≤25s）确定性等待并推进度，不消耗 LLM 推理轮次 | 混合策略：多阶段/审批/超长走 Workflow `suspend/resume`；自由对话中短任务走标准 MCP Tool `task.wait` + 超时 Yield |
| **多协议透传、Cache 锚点与持久预算** | `prompt-cache.mjs` / `model-fetch-proxy.mjs` / `model-stream-idle.mjs` / `durable-turn-budget.mjs` | 4 协议原生适配；将 `cache_control` 从易变末尾 `<studio_digest>/<plan>` 前移到前一个稳定历史块并固定 `metadata.user_id`；派发前落盘扣减请求/步数预算；流式空闲超时 | 官方 `@ai-sdk/*` + `wrapLanguageModel` 中间件 + 原生 `timeout.{totalMs,stepMs,chunkMs}` + PG `service_runs` 原子预扣 |
| **结果态离线评测台** | `eval/sim-go.mjs` + `scenarios.mjs` + `request-efficiency-gate.mjs` | 内存版 Go 权威模拟器 + 确定性模型桩，零模型费跑全链路；判分只看终态画布/任务绑定/无重复扣费/请求效率 | Mock MCP Toolset（`sim-business`）+ AI SDK 原生 Mock 模型补强 Mastra Scorers + Vitest |

## 3. 长耗时异步媒体任务等待策略：工作流编排 vs `task.wait`

1. **不引入 Temporal / Restate / DBOS**：在已有 Go 异步任务系统（`operation`）+ PostgreSQL + Mastra Workflow 的前提下，引入额外分布式引擎会带来双状态机与显著运维开销。
2. **混合策略（已定案）**：
   - **固定/批量多阶段、HITL 付费审批与超长视频队列（>2–3 分钟）**：走 **Mastra Workflow `suspend/resume` + Go 终态回调**（挂起零连接占用，跨重启无损恢复）。
   - **自由对话即兴单图/中短耗时任务（15s–120s）**：Go 将 `task.wait`（≤25s 长轮询分片）暴露为标准 MCP Tool，Agent 侧在 MCP Tool Wrapper 内循环调用（走 `@mastra/mcp`，零手写 HTTP）并实时推送 `task.progress`，超过阈值优雅让出回合（Yield）并返回 `taskId`。

## 4. AI 网关与双轨 Evals 选型对照

### 4.1 AI 网关与同赛道对照

| 方案 | 核心技术栈 / 许可 | 优势 | 局限与选型结论 |
| --- | --- | --- | --- |
| **演进 Go `execution/gateway`（选定）** | Go 单体内建 | 统一承接文本原生透传 + 图/视/音异步多模态 + 多租户账本；上游 `Base URL` 可灵活指直连/OpenRouter/Bifrost | 与 BeefTV（Go Model Channels）和 Toonflow（Vercel AI SDK 直连）同向，兼顾云端计费权威 |
| **Bifrost** | Go / Apache 2.0 | 微秒级低延迟、内置重试/降级/语义缓存 | 高级 RBAC 在企业版，可作为可选上游而非强依赖 |
| **LiteLLM** | Python / MIT 核心 | 厂商覆盖快 | 需独立 Python 运行时，流式开销高且协议转译偶吞特性（已否决） |
| **New-API** | Go / **AGPLv3** | 开箱即用多租户额度后台 | **AGPLv3** 存在商用合规传染风险（已排除） |

### 4.2 离线评测与线上观测双轨对照

| 工具 / 平台 | 定位与许可 | 适用分工 |
| --- | --- | --- |
| **Mastra Scorers + Vitest + Mock MCP（选定：离线 CI）** | TS 原生 / MIT | 代码级轨迹断言 + 终态画布/零重复扣费断言 + 请求效率门禁 + LLM-as-a-Judge，零外部服务依赖 |
| **自托管 Langfuse（选定：线上观测）** | 开源 LLM 工程平台 / MIT 核心 | OTel 全链路 Tracing（串联 `traceparent`）、Prompt 版本管理（配本地缓存兜底）、坏 Case 沉淀 Dataset 与线上采样 |
| **Promptfoo / Braintrust** | CLI 红队 / 商业云托管 | Promptfoo 偏黑盒矩阵；Braintrust 闭源托管不满足自托管主权要求 |

## 5. 调研限制

- 本文结论基于 2026-11-25 至 2026-11-26 的公开资料、框架类型定义与 `glanderness/BeefTV` 公开主干源码核查，未在本地实际编译运行 BeefTV 桌面端或压测 Bifrost/Langfuse。
- `@mastra/pg` 禁用运行时 DDL（`disableInit`）、`service_runs` 原子预算列与 Go `task.wait` MCP 端点契约需在实施阶段通过隔离 PostgreSQL 与联调测试验证。

[调研导航](00-调研导航.md) · [需求33](../requirement/33-Agent服务架构重构与技术选型需求.md) · [设计48](../design/48-Agent服务Mastra架构与技术选型设计.md)
