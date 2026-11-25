# Agent 服务代码规范

本文件适用于 `agent/` 及其测试，继承[仓库工程规范](../AGENTS.md)。现行架构与技术选型合同由[设计48](../workspace/content/design/48-Agent服务Mastra架构与技术选型设计.md)唯一维护。当前运行依赖以包清单和锁文件为准。

## Google TypeScript

1. 手写代码遵循 [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html)。使用 ESM、具名导出、`const` 优先及单引号。类型和类用 `PascalCase`，值和方法用 `lowerCamelCase`，接口省略 `I` 前缀，缩写按单词处理，例如 `HttpUrl`。Prettier 执行基础格式。
2. 纯类型用 `import type`，对外部值以 `unknown` 收窄并使用 Zod 校验。避免 `any`、不安全断言和非空断言。手写业务代码禁止 `@ts-ignore`、`@ts-nocheck`。`@ts-expect-error` 仅用于验证类型错误的测试并说明预期。
3. NodeNext 项目约定相对导入使用运行时 `.js` 扩展名，Node 内置模块用 `node:`。不使用 NestJS 装饰器或 `reflect-metadata`。
4. 保持模块、函数与文件职责清晰。`src/` 遵循扁平两层包组织（`src/<package>/<module>.ts`），跨包相对导入不超过一层 `../<package>/<module>.js`，文件名统一 `kebab-case`，禁止手写 `index.ts` barrel 与无职责 `common`/`utils`。

## Mastra、协议与职责边界

1. `server.ts` 负责进程入口、配置加载（Zod）、连接池与组件装配、gRPC Server（`:4402`）与极简 HTTP 健康探针（`:4401`，仅 `/healthz`、`/readyz`）启动以及优雅停机。不引入 NestJS、Fastify、Swagger/OpenAPI 或 Prisma。
2. 依赖方向严格单向无环：`transport → sessions → mastra → {skills, business, database, observability}`，禁止循环依赖和服务定位器。
3. **跨服务通信**：
   - Go → Agent：强类型 `AgentRuntime` gRPC（唯一真源 `backend/api/proto/glowloom/agent/runtime.proto`），`StreamRunEvents` 使用 gRPC Server Streaming。
   - Agent → Go 业务工具：通过 `@mastra/mcp` 连接 Go 内部 MCP Server（`/internal/agent-mcp`）。
   - Agent → Go 鉴权/配额/摘要：通过强类型 `AgentBusiness` gRPC 或受控内部接口。
   - Agent → LLM：通过 Vercel AI SDK 连接 Go 统一渠道透传网关（`/internal/agent-gateway`），完整保留 Prompt Cache、Thinking 与流式输出。
4. **数据库与持久化**：
   - 复用 PostgreSQL 独立 `agent` schema 与受限 `glowloom_agent` 角色；`backend/db/schema.sql` 是唯一表结构真源，严禁任何运行时 DDL 或自动迁移。
   - `@mastra/pg` 显式关闭运行时自动建表（`disableInit: true`）；服务级控制表（`service_sessions`、`service_runs`、`service_run_events`、`service_public_messages`、`service_control_inbox`、`recovery_backoff`）统一使用 `Kysely` + `pg` 访问。
5. **混合编排、记忆与 Skill**：
   - Producer 主 Agent 与专家子 Agent 负责自由创作与委派；多阶段交付与高成本审批链路封装为 Mastra Workflow，使用原生 `suspend()` / `resume()` 持久化恢复。
   - Mastra Memory（对话窗口 + Working Memory + PgVector）仅存储对话与偏好；剧本、设定集、分镜、画布等权威数据不复制到 Agent 向量库，由 Go MCP 按需拉取。
   - 声明式 Skill 存放于 `skills/*/SKILL.md` 并绑定 MCP 工具白名单；Agent 容器内严禁执行任何动态或第三方脚本。
6. **可观测性与双轨 Evals**：
   - Pino 脱敏日志与 W3C `traceparent` 透传归 `observability/`；线上深度对接自托管 Langfuse（OTel Trace + Prompt 版本管理 + 本地 `prompts/` 缓存兜底）。
   - 离线开发与 CI 回归门禁归 `evals/`，使用 Mastra Scorers + Vitest 断言工具轨迹、步数/成本上限与质量 Rubric。

## 质量门禁

按变更范围执行 `pnpm typecheck`、`pnpm lint`、`pnpm format:check`、`pnpm test`、`pnpm eval` 和 `pnpm build`。记录实际执行结果、未执行项与环境限制。
