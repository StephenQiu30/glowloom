# Agent 服务代码规范

本规则适用于 `agent/` 独立 Node.js 服务及其测试和脚本，并继承仓库根目录 [AGENTS.md](../AGENTS.md) 的通用协作规则。产品边界以需求和 PRD 为准，架构与服务合同以 [设计32](../docs/design/32-完整创作AgentHarness与Skill多Agent架构设计.md) 为准，技术版本以本包清单和锁文件为准。

## 规范来源与优先级

1. [Google TypeScript Style Guide](https://google.github.io/styleguide/tsguide.html) 是本服务代码风格的首要标准。该指南说明其公开版源自 Google 内部规范且需要结合外部环境调整；本项目采用下文明确的 NodeNext、Fastify 和目录约定作为项目级补充。
2. Node.js 运行时行为遵循 [Node.js 官方文档](https://nodejs.org/learn)；Fastify 的插件、路由、生命周期、类型和错误处理遵循与本包 Fastify 主版本匹配的[官方文档](https://fastify.dev/docs/latest/)。API 行为以当前锁定版本为准。
3. 已确认的项目设计、公开 HTTP/Storage 合同及运行时要求优先于通用风格建议。确需偏离 Google 规范时，在对应设计或代码注释中说明原因；不得仅以个人偏好形成第二套规则。

## TypeScript / JavaScript 风格

1. 使用 ESM 和具名导出；业务模块不得使用 `namespace`、CommonJS `require` 或默认导出。需要遵守外部工具配置入口要求的配置文件可以例外。
2. 使用具备语义的 `PascalCase` 类型名和 `lowerCamelCase` 值名；接口不使用 `I` 前缀，缩写按普通单词大小写（如 `HttpUrl`），避免无职责的 `utils`、`common`、`index.ts` 聚合导出。
3. 类型信息不能确定时使用 `unknown` 并在边界收窄；避免 `any`、非空断言和类型断言。禁止 `@ts-ignore`、`@ts-nocheck`；`@ts-expect-error` 仅允许在确需验证类型错误的测试中使用并说明预期。
4. 类型导入使用 `import type`。Node.js 内置模块使用 `node:` 协议。由于本包采用 `NodeNext` ESM，项目内相对导入写运行时 `.js` 扩展名；这是对通用示例的明确项目约定。
5. 字符串、分号和基础排版遵循 Google TypeScript Style Guide；`agent/.prettierrc.json` 固定单引号，Prettier 是格式唯一执行器。保持函数聚焦；复杂逻辑优先提取有明确职责的小函数，不为形式拆分增加空转发层。
6. 外部 HTTP 输入、环境配置和持久化记录都视为不可信数据：在边界校验后再进入业务流程。API schema 只维护于 `contracts/agent.ts`，不得在路由、生成 OpenAPI 和客户端中维护冲突副本。
7. 异常必须是 `Error` 实例。预期的 Agent 服务错误使用 `sessions/errors.ts` 中的 `AgentError` 表达错误代码；HTTP 状态码与公开响应格式只由 `transport/http` 映射。异常不得依赖 Fastify `request`、`reply` 类型。

## Node.js 服务要求

1. 配置只在启动边界读取和校验；模块通过构造函数显式接收依赖，不在业务模块读取全局 `process.env`、建立全局 service locator 或暗中创建连接池。
2. 请求路径不得运行会阻塞事件循环的同步文件、加密、压缩或子进程操作。请求体、并发、队列、重试和等待必须有明确上限；可取消的外部工作传递 `AbortSignal`。
3. 后台 Promise 必须被等待、捕获或由明确所有者管理；定时任务、恢复任务和会话运行有停止信号、退出条件及有界关闭流程。
4. 使用 Pino 结构化日志；记录排障所需的 request/session/run 标识，并对服务令牌、数据库 DSN、模型输入、提示词和用户内容脱敏。不得记录密钥、私人素材内容或不必要的个人信息。
5. 每个模块只拥有声明的职责，不直接访问其他服务的数据库 schema。Agent 与 Go 共用 PostgreSQL 实例但使用独立 schema 和受限角色；Go 仍是身份、预算、审批、模型出站、业务工具及业务数据的唯一权威。

## Agent 目录与职责

```text
agent/
  contracts/                         TypeBox HTTP schema 唯一来源
  scripts/                           OpenAPI 导出与明确的一次性维护命令
  src/
    server.ts                        唯一组合根：配置、连接池、依赖组装、启动与关闭
    config.ts                        环境变量解析和校验，不包含业务逻辑
    transport/http/                  Fastify 内部 HTTP/SSE、认证、输入/输出校验与错误映射
    sessions/                        会话、run、控制命令、输入清单及恢复用例
      errors.ts                      与传输协议无关的预期错误代码
      port.ts                        HTTP 入站适配器消费的应用接口
      persistence/                   Agent 自有会话/run 元数据、历史查询与公开消息投影
    runtime/pi-durable/              官方 Pi Durable Harness、工具/扩展/文档/事件适配
    integrations/go-business-api/    调用 Go 权威服务的 HTTP 客户端与数据转换
    adapters/postgres/pi-durable/    官方 Pi Storage 的 PostgreSQL 实现、租约和记录编码
  tests/                             单元、HTTP、真实 PostgreSQL、Storage 一致性与恢复测试
```

职责和依赖边界：

- `server.ts` 是唯一进程组合根；只装配配置、数据库连接池、Go 客户端、会话模块和 HTTP 服务，并负责启动与进程关闭。会话模块可在自身边界内组合私有存储/查询助手，不得形成全局 service locator。
- `transport/http` 是入站适配器；负责内部服务身份校验、TypeBox/Fastify 校验、SSE 传输和 HTTP 错误映射，不访问数据库、不执行 Harness 调度。
- `sessions` 是 Agent 应用模块；接受会话与 run、处理控制/读取/恢复并协调执行。`sessions/persistence` 只保存 Agent 自有服务元数据、历史和事件/消息读取投影。
- `runtime/pi-durable` 只组合官方 Pi Durable 能力和第一方受信扩展；不创建第二套 Agent 循环，不成为授权、计费或业务状态的权威。
- `integrations/go-business-api` 只封装 Agent 到 Go 的内部协议、请求身份和数据转换，不持有 Go 数据库连接或复制其业务模型。
- `adapters/postgres/pi-durable` 只实现官方 Durable Storage 合同及 session owner/epoch fencing；不持有会话 API DTO 或业务表权限。
- `contracts/agent.ts` 是 Agent HTTP schema、operation 元数据及其 TypeScript 类型的唯一来源；生成的 OpenAPI 和客户端不得手改。

禁止为了类比 Java 全局分层而新建空的 Controller、Service、Repository、Manager 或通用框架。只有当职责边界、独立生命周期或测试替换需求真实存在时，才拆出新模块或接口。

## 质量门禁

按变更范围在 `agent/` 执行 `pnpm typecheck`、`pnpm lint`、`pnpm format:check`、`pnpm test`、`pnpm api:check` 和 `pnpm build` 中适用的检查。Storage、恢复、协议和生命周期改动须覆盖真实隔离 PostgreSQL 或对应跨服务场景。未执行、跳过或受环境限制的检查必须如实记录；静态检查通过不等于真实供应商或产品验收完成。
