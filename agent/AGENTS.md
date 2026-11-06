# Agent 服务代码规范

本文件适用于 `agent/` 及其测试，继承[仓库工程规范](../AGENTS.md)。业务与持久化合同由[设计32](../docs/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)维护，NestJS 迁移目标、目录与验收见[设计37](../docs/design/37-Agent服务目录与TypeScript规范设计.md)。当前运行依赖以包清单和锁文件为准。

## Google TypeScript

1. 手写代码遵循 [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html)。使用 ESM、具名导出、`const` 优先及单引号。类型和类用 `PascalCase`，值和方法用 `lowerCamelCase`，接口省略 `I` 前缀，缩写按单词处理，例如 `HttpUrl`。Prettier 执行基础格式。
2. 纯类型用 `import type`，对外部值以 `unknown` 收窄。避免 `any`、不安全断言和非空断言。手写业务代码禁止 `@ts-ignore`、`@ts-nocheck`。`@ts-expect-error` 仅用于验证类型错误的测试并说明预期。
3. NodeNext 项目约定相对导入使用运行时 `.js` 扩展名，Node 内置模块用 `node:`。Nest DTO、运行时类及 DI token 使用值导入。启用 Nest 所需的 TypeScript legacy decorators 与元数据，并验证生产和测试入口使用相同的装饰器语义。
4. 保持模块、函数与文件职责清晰。用局部、具名的模块表达功能，避免手写命名空间、无职责的 `common`、`utils` 和全量 barrel。确需偏离风格时，在对应设计或注释记录理由。

## NestJS 与职责

1. 使用[官方 NestJS 模块](https://docs.nestjs.com/modules)和[自定义 Provider](https://docs.nestjs.com/fundamentals/custom-providers)。`server.ts` 负责进程入口。Nest 模块和 Provider 统一在 `http/app.ts` 装配。连接池由进程入口创建和释放。会话服务、Pi、Storage 和 Go 客户端保持普通 TypeScript 类与构造注入。依赖方向由 Controller 到应用端口，再到应用所需的执行和持久化依赖。禁止循环模块和服务定位器。
2. Controller、DTO、SSE、校验与错误映射统一位于 `http/`，应用接口和会话用例位于 `sessions/`。Controller 调用用例，应用层不读取 Nest/Fastify request/reply。预期错误用 `AgentError` 表达，保留错误因果；Exception Filter 与适配器错误处理保持现有状态和错误结构。
3. HTTP 的唯一维护位置是功能边界上的 Controller/DTO。运行时验证和 OpenAPI 注解随 DTO 一起维护，并通过合同测试核对。环境与持久化外部数据在各自边界校验。生成文件由工具重生成。切换只交付一套 NestJS 路由、协议和配置。移除旧 HTTP 实现、生成链、兼容转换及回退入口。目录按真实职责组织，避免独立 `contracts/`、`scripts/`、全局 Controller/Service/DAO 分层和纯转发层。
4. 配置在启动边界读取并校验。连接池、恢复循环和订阅分别有明确的资源所有者。请求路径避免阻塞事件循环，限制请求体、并发、等待和重试。将 `AbortSignal` 传到可取消的调用，收尾等待有界。SSE 断开只关闭订阅。
5. Pino 日志带排障所需的 request/session/run 标识。脱敏服务令牌、数据库 DSN、提示词、模型输入输出和用户内容。Agent 使用独立 schema 与受限角色，Go 保留身份、预算、审批、模型出站、业务工具和业务数据权威。

## 接口生成

- 生成方向与检查见[设计37 §7.6](../docs/design/37-Agent服务目录与TypeScript规范设计.md#76-swagger-与-umi-接口生成)。Agent 注解生成 OpenAPI 供 Go 客户端消费。Go 内部独立 Swagger 经 `@umijs/openapi` 生成 Agent 消费的 TypeScript 请求和类型，位于 `src/business/generated/`。
- 仅可验证重生成的 Umi 产物允许保留官方生成器输出的命名空间、聚合入口、默认导出与局部抑制注释。生成代码纳入严格编译和生产构建，生成声明和传输层另用 `pnpm api:typecheck` 在 `skipLibCheck=false` 下校验。请求层保留服务端身份、取消、超时及有界读取。

## 质量门禁

按变更范围执行 `pnpm typecheck`、`pnpm lint`、`pnpm format:check`、`pnpm test`、`pnpm api:check` 和 `pnpm build`。框架迁移还须验证生产 JS 入口、实际 HTTP/SSE、隔离 PostgreSQL、官方 Storage conformance 和真实进程恢复。记录实际执行结果、未执行项和环境限制。
