# Agent 服务架构重构与技术选型

唯一来源设计：[设计48：Agent服务Mastra架构与技术选型设计](../design/48-Agent服务Mastra架构与技术选型设计.md)。原始需求经来源设计追溯至 [需求33 SRC-AGV2-01 / SRC-AGV2-02 / SRC-AGV2-03](../requirement/33-Agent服务架构重构与技术选型需求.md#src-agv2-01)。本文定义 Agent 服务全量原地重构及 Harness 增强的交付范围与验收标准，承接既有创作 Agent 业务能力（[PRD19](19-完整创作Agent与Skill多Agent.md)）并在技术架构层替代原 Pi Durable / NestJS 选型。

## 1. 目标与边界

通过**一步到位全量原地重构**，将 `agent/` 从当前的“NestJS + Fastify + Pi Durable + Prisma + 双向 OpenAPI/HTTP-over-gRPC 隧道”重构为**“Node 24 + TypeScript + Mastra + 强类型 gRPC + Go 内部 MCP + Kysely/Zod + 自托管 Langfuse + 离线 Scorers 评测”**架构，并同步演进 Go `execution/gateway` 为统一渠道 AI 网关（文本原生透传 + 图/视/音异步多模态 + 多租户账本）。

本 PRD 聚焦**架构重构与工程底座交付**，不削减既有创作者对话、多 Agent 协作、画布/剧本/分镜联动、审批与计费等产品功能。

## 2. 使用者与价值

- **终端创作者**：获得更稳定的流式创作体验、基于最新项目真源（无脏缓存幻觉）的精准工具调用、刷新或重启后不丢失进度的审批与长任务恢复。
- **Agent 与后端研发者**：摆脱四套传输层与三套数据访问层的历史包袱；以 Mastra + MCP + 强类型 gRPC + Kysely 统一开发心智，并具备 CI 轨迹回归门禁与线上 Langfuse 全链路排障能力。
- **平台运维与成本治理者**：Go 统一渠道网关集中掌控文本与多模态的预冻结/精确结算账本，上游渠道可按需切换直连厂商或外部聚合路由，数据库继续保持单一 `schema.sql` 离线受控治理。

## 3. 验收标准

| ID         | 前置与动作                                                       | 可观察接受结果                                                                                                           | 来源                    |
| ---------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------ | ----------------------- |
| AC-AGV2-01 | 审查知识库文档链与 Workspace 站点检查                            | 需求33 → 设计48 → PRD34 → Plan43 与研究71 链条完整、导航与 `index.mdx` 同步，Workspace 格式/类型/Lint 检查通过           | 设计48 §1、§8.2         |
| AC-AGV2-02 | 检查 `agent/` 依赖、目录与强类型 gRPC 契约                       | `agent/` 已清除 NestJS、Fastify、Prisma、Pi Durable、TypeBox 与双向 OpenAPI 胶水；`runtime.proto` 强类型 RPC 经 Buf 校验一致 | 设计48 §7.2、§7.3、§7.8 |
| AC-AGV2-03 | 在隔离环境中验证 Go 内部 MCP Server 与 `@mastra/mcp` 调用        | Agent 可发现并调用 Go 暴露的剧本/设定/分镜/画布/媒体 MCP 工具，鉴权与项目隔离生效，未授权调用被拒绝                      | 设计48 §7.3、§7.6       |
| AC-AGV2-04 | 验证 Go 统一渠道网关的文本原生透传与计费闭环                     | Vercel AI SDK 通过 Go 网关发起流式调用，完整保留流式增量/Prompt Cache/Usage 统计，并完成预算预冻结与真实结算回写         | 设计48 §7.4             |
| AC-AGV2-05 | 在受限 `agent` 数据库角色下执行冷启动与读写测试                  | `@mastra/pg` 关闭运行时 DDL，所有表由 `backend/db/schema.sql` 静态初始化，旧 `runtime_*` 表已移除，DB 唯一来源门禁通过   | 设计48 §7.5、§8.1       |
| AC-AGV2-06 | 验证混合编排、Workflow HITL 审批、断线重放与崩溃恢复             | 主/子 Agent 委派正常；高成本步骤挂起审批并支持跨重启 `resume`；带 `Last-Event-ID` 断线重放无漏事件；租约过期可安全恢复   | 设计48 §7.5、§7.6       |
| AC-AGV2-07 | 验证声明式 Skill 安全边界、离线 CI 轨迹评测与自托管 Langfuse 链路 | Agent 容器无动态脚本执行；Mastra Scorers + Vitest 轨迹评测通过；Langfuse 接收完整 `traceparent` Trace，不可达时本地兜底 | 设计48 §7.6、§7.7       |
| AC-AGV2-08 | 审查 `agent/src/` 网络调用与验证多协议透传、Cache 锚点、超时与预算 | `agent/src/` 全部出网零手写 `fetch/http`（仅保留入站健康探针）；官方 `@ai-sdk/*` 多协议透传、`wrapLanguageModel` Cache 锚点前移、原生 `chunkMs` 超时与 PG `service_runs` 派发前原子预算测试通过 | 设计48 §7.4、§7.5       |
| AC-AGV2-09 | 验证画布分层有界投影、同轮写链 Rebase、混合异步等待与结果态 Eval | 大画布读取 `<40KB` 且截断字段带 `complete: false` 与指针下钻；同轮局部写链自动 Rebase 且外部修改触发熔断；付费任务带确定性 `operationId` 防重扣费；自由对话 `task.wait` 与 Workflow 混合等待及 Mock MCP 结果态离线评测全绿 | 设计48 §7.4、§7.6、§7.7 |

## 4. 排除范围与接受边界

- 不引入独立第三方重型 LLM 网关中间件（如 LiteLLM Python 容器）作为强制依赖。
- 不在 `agent/` 容器内提供代码沙箱或第三方脚本执行能力。
- 本轮知识库整理与评审通过，不代表代码重构、数据库变更、真实收费调用或生产发布已完成。

## 5. 交付与追溯

具体全量原地重构任务、文件责任、验证命令与回退边界见唯一承接 [Plan43：Agent服务Mastra架构重构执行计划](../plan/43-Agent服务Mastra架构重构执行计划.md)。实施进度与未决确认见 [BACKLOG](../../../BACKLOG.md)。

[PRD导航](00-PRD导航.md) · [唯一来源设计48](../design/48-Agent服务Mastra架构与技术选型设计.md)
