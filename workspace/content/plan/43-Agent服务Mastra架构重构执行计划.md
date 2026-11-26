# Agent 服务 Mastra 架构重构执行计划

唯一来源 PRD：[PRD34：Agent服务架构重构与技术选型](../prd/34-Agent服务架构重构与技术选型.md)。技术合同沿 [设计48](../design/48-Agent服务Mastra架构与技术选型设计.md) 展开；实施进度、阻塞与下一步只在 [BACKLOG](../../../BACKLOG.md) 维护。本计划在技术底座层替代原 NestJS/Pi Durable 工程实施项，承接全量原地重构落地。

## 1. 前置条件与实施原则

1. **知识库先行（本轮已完成）**：先完成需求33 → 设计48 → PRD34 → Plan43 与研究71 的规范化沉淀与站点检查，再启动源码、SQL 与 Proto 改造。
2. **一步到位全量原地重构**：按已确认策略，同步更新工程规范（`workspace/PROJECT.md`、`agent/AGENTS.md`）、`backend/db/schema.sql` 与 `runtime.proto`，同步改造 Go 统一渠道网关与内部 MCP Server，清空并基于 Mastra 原地重建 `agent/`，**不保留新旧双轨或兼容分支**。
3. **数据库红线不变**：所有 Mastra 框架表与 Kysely 服务控制表均静态声明于 `backend/db/schema.sql`，显式关闭 `@mastra/pg` 运行时 DDL；严禁新增 `migrations/` 或运行时建表。
4. **未决前置确认**：开工前确认自托管 Langfuse 的 Compose 接入方式及本地/测试环境 `agent` schema 的重置窗口。

## 2. 重构任务与退出条件

| ID        | 阶段 | 对应接受   | 文件责任与核心动作                                                                                                                          | 退出条件                                                                            |
| --------- | ---- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| T-AGV2-01 | P0   | AC-AGV2-01 | 沉淀需求33、设计48、PRD34、Plan43、研究71，同步 `index.mdx` 与各目录导航                                                                    | Workspace `format:check`、`types:check`、`lint` 全通过，无断链；不改动业务源码      |
| T-AGV2-02 | P1   | AC-AGV2-02、AC-AGV2-05 | 同步更新 `workspace/PROJECT.md`、`agent/AGENTS.md`、`backend/db/schema.sql`（移除旧 `runtime_*` 表，静态声明 Mastra 表与 Kysely 控制表）及 `backend/api/proto/glowloom/agent/runtime.proto`（强类型 `AgentRuntime` / `AgentBusiness`） | `buf lint`/`buf generate` 零漂移，`backend/tests/db/schema_source_test.go` 通过     |
| T-AGV2-03 | P2   | AC-AGV2-03、AC-AGV2-04 | 改造 Go `backend/`：升级 `execution/gateway` 原生协议透传端点与账本闭环；实现 Go 内部 MCP Server 暴露核心业务工具；对接强类型 `AgentRuntime` 客户端与 `AgentBusiness` 服务端 | Go 单元测试与 `go test -race` 全绿；透传网关与 MCP 鉴权隔离测试通过                 |
| T-AGV2-04 | P3   | AC-AGV2-02、AC-AGV2-05 | 原地清空重建 `agent/`：初始化 Mastra + Vercel AI SDK + Kysely + Zod + `@grpc/grpc-js` + `@mastra/mcp` + `@mastra/pg`（`disableInit`），实现 `transport/`、`database/`、`business/` 与 `sessions/`（租约锁、事件日志、可插拔 EventBus、恢复泵） | `agent/` 无 NestJS/Fastify/Prisma/Pi Durable 残留；受限 DB 角色冷启动与读写测试通过 |
| T-AGV2-05 | P4   | AC-AGV2-06、AC-AGV2-07 | 实现 `mastra/`（Producer 主 Agent、专家子 Agent、HITL Workflow `suspend/resume`、分离式 Memory）、声明式 `skills/`、Langfuse OTel/Prompt 集成（带本地兜底）及 `evals/`（Mastra Scorers + Vitest） | 审批跨重启恢复、`Last-Event-ID` 断线重放、轨迹评测与 Langfuse 兜底测试全绿        |
| T-AGV2-06 | P5   | AC-AGV2-01～07 | 同步 `docker-compose.yml` / `docker-compose.dev.yml` 与 CI 工作流；执行 Go、Agent、Frontend、Canvas 全仓门禁与端到端联调验收                 | 全仓 lint/typecheck/build/race/evals 通过，Compose 健康启动，公开 `/api`/SSE 零破坏 |
| T-AGV2-07 | P6   | AC-AGV2-08     | **Harness 阶段一（零手写出网 + 协议/缓存/超时/持久预算）**：升级 `business/gateway-provider.ts` 为官方 `@ai-sdk/{openai,anthropic,google,openai-compatible}` 多协议路由并移除手写 `fetch`；迁移 `skills/run-skills.ts` 私有 Skill 拉取至 `AgentBusiness` gRPC/`@mastra/mcp`；新增 `wrapLanguageModel` 中间件实现 Prompt Cache 稳定锚点前移与 PG `service_runs` 派发前原子预算预扣；配置 SDK 原生 `timeout.{totalMs,stepMs,chunkMs}`；同步 `backend/db/schema.sql` 预算列 | `agent/src/` 零出网手写 `fetch/http`；多协议、Cache 锚点、超时与崩溃不重置预算测试全绿 |
| T-AGV2-08 | P7   | AC-AGV2-09     | **Harness 阶段二（画布分层投影 + 写链 Rebase + 混合异步等待）**：Go/Canvas 补齐基础分页、JSON Pointer 子字段读取、确定性 `operationId` 幂等与标准 MCP Tool `task.wait`（≤25s 长轮询分片）及 Workflow 终态回调；Agent 侧新增 `business/canvas-bridge.ts` 实现 `<40KB` 有界裁剪、`complete: false` 标记、同轮局部写链 Rebase、`canvas_context_changed` 熔断、`<canvas_changes>` 增量摘要与中短任务确定性等待 + 超时 Yield | 大画布分页/下钻、并发 Rebase/熔断、`operationId` 防重扣费与混合等待集成测试全绿 |
| T-AGV2-09 | P8   | AC-AGV2-09     | **Harness 阶段三（Mock MCP 离线结果态评测门禁）**：在 `agent/evals/` 搭建内存版 Mock MCP 业务模拟器（`sim-business`）与 AI SDK 原生 Mock 模型，落地终态画布/绑定/零重复扣费断言与请求效率预算门禁（覆盖大画布定位、同轮多改、崩溃续跑与长流程） | `pnpm eval` 零模型费、零手写 HTTP 桩全绿，接入 CI PR 必跑门禁 |

## 3. 验证方法

### 3.1 P0（本轮知识库整理）

在 `glowloom-server/workspace/` 执行：

1. `pnpm exec prettier --check content/...`（对新增与变更文档做格式校验）
2. `pnpm types:check`
3. `pnpm lint`
4. 核对所有相对链接、锚点与 `git status --short`

### 3.2 P1～P5（全量原地重构实施阶段）

- **协议与数据库**：`buf lint`、`buf generate` 一致性核对、`go test ./tests/db/...`。
- **Go 后端**：`gofmt`、`goimports`、`go vet ./...`、`golangci-lint run ./...`、`go test -race -count=1 ./...`、`govulncheck ./...`。
- **Agent 服务**：`pnpm typecheck`、`pnpm lint`、`pnpm format:check`、`pnpm test`（含隔离 PostgreSQL 集成测试、租约恢复、Workflow `suspend/resume`、`Last-Event-ID` 重放与 Mastra Scorers 轨迹评测）、`pnpm build`。
- **前端与部署**：`frontend/` `pnpm api:check`、`pnpm exec tsc --noEmit`、`pnpm exec vitest run`；`docker compose --env-file /dev/null config --no-interpolate --quiet`。

## 4. 迁移与回退边界

- **P0（文档）**：纯知识库变更，不影响现有运行代码，可直接按 Git 提交回退。
- **P1～P5（全量原地重构）**：在独立开发分支完成一次性切换与全仓门禁验收后再合入主干；合并前不破坏主干现有可运行状态。已有存量库升级须由管理员依据最新 `backend/db/schema.sql` diff 离线执行受控事务变更，严禁对存量库直接重放空库 SQL。

[Plan导航](00-Plan导航.md) · [唯一来源PRD34](../prd/34-Agent服务架构重构与技术选型.md)
