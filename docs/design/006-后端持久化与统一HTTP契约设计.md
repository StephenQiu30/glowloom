# 后端持久化与统一 HTTP 契约设计

> Design ID：DES-006
> 状态：accepted，作为当前 Go 后端和 Next.js 前端的顶层实现规范
> 日期：2026-08-22
> 上游设计：[目标系统架构](./000-AI视频生产平台目标系统架构设计.md)、[接口工作流](./003-AI视频生产平台接口工作流与功能实现设计.md)、[服务与模块实施基线](./005-AI视频生产平台服务与模块实施基线.md)

## 1. 决策

### 1.1 PostgreSQL 持久化

Go 业务后端统一使用 GORM PostgreSQL driver 作为 Repository 的 ORM 实现：

- `gorm.io/gorm` 提供 Generics API、查询构造器和事务边界；`gorm.io/driver/postgres` 负责 PostgreSQL 方言。
- `backend/schema/current.sql` 是唯一当前数据库结构，`schema-init` 只执行该文件并验证表指纹。
- 禁止 `AutoMigrate`、ORM migration 目录、历史版本链、兼容 decoder 和双写。
- 业务模块禁止出现 `SELECT`、`INSERT`、`UPDATE`、`DELETE`、`QueryRow`、`Exec` 和手写占位符 SQL。Repository 只能使用 GORM 的 `Where`、`Scopes`、`Joins`、`Order`、`First`、`Find`、`Create`、`Updates`、`Delete`、`Clauses` 和 `Transaction`。
- 只有 `platform/database` 的连接健康检查、`schema-init` 以及 GORM driver 的连接底座可以接触 pgx；pgx 类型不得进入 Model、Service、Controller 或 Port。
- ORM 映射记录是 Repository 的持久化细节；业务返回值仍为模块 `model.go` 中的 plain struct。映射记录可以在同一个 `repository.go` 中声明 `TableName()`，不新增 `orm/dao/generated` 子目录。

### 1.2 事务与模块边界

```text
Controller → Module Service → Module Repository → GORM DB/Transaction → PostgreSQL
                                      ↘ Port → 外部基础设施
```

- 一个 Service 用例拥有一个事务边界；Repository 不互相调用，不开启嵌套业务事务。
- 同一用例写多个事实表时，必须在同一个 `db.Transaction(func(tx *gorm.DB) error { ... })` 中完成；任一错误返回都会回滚。
- `Outbox`、审计和业务事实与主写入使用同一 PostgreSQL 事务；Elastic、Kafka、MinIO 等副作用在提交后由 Go worker 执行。
- `GORM ErrRecordNotFound` 由模块转换为语义化的 `NotFound` 业务错误，不由 Controller 读取数据库错误文本。
- 复杂查询必须先在模块内声明命名 Scope/查询方法；不得把 GORM `*gorm.DB` 或表名泄漏给 Service、Controller 或其他模块。

### 1.3 全局 HTTP 异常与响应

新增平台能力包 `backend/src/platform/httpapi`，所有 Go Controller 统一使用它：

| 类型 | 责任 |
| --- | --- |
| `HTTPStatus` | 强类型 HTTP 状态码枚举，集中声明 `OK`、`Created`、`Accepted`、`NoContent`、`BadRequest`、`Unauthorized`、`Forbidden`、`NotFound`、`Conflict`、`UnprocessableEntity`、`TooManyRequests`、`InternalServerError`、`ServiceUnavailable` |
| `ErrorCode` | 强类型稳定业务码，例如 `invalid_json`、`invalid_id`、`not_found`、`forbidden`、`conflict`、`validation_failed`、`rate_limited`、`dependency_unavailable`、`internal_error` |
| `APIError` | `status/code/message/next_action/details/request_id`；实现 `error`，不携带 SQL、堆栈、SDK 原始信息或跨租户标识 |
| `Envelope[T]` | 成功只返回 `{"data": ...}`；失败只返回 `{"error": ...}`，不允许模块自行定义 envelope |
| `WriteData` / `WriteError` | 设置 Content-Type、request ID、状态码和 JSON 编码；编码失败交给服务器日志，不二次写响应 |
| `RecoverMiddleware` | 捕获未处理 panic，记录 request ID，返回 `internal_error`；禁止将 panic 文本返回客户端 |
| `DecodeJSON` | `MaxBytesReader`、`DisallowUnknownFields`、单个 JSON 文档校验；失败统一返回 `invalid_json` |

Controller 只负责路径参数、请求解码、调用 Service 和选择成功状态；不根据 `strings.Contains(err.Error(), "not found")` 猜测状态，不定义 `writeData`、`writeError`、`writeErr` 等局部实现。Service 返回 `*httpapi.APIError` 或可被平台分类的语义错误，未知错误统一为 `internal_error`。

全局中间件顺序固定为：

```text
Recover → RequestID → CORS → Identity/Workspace → Module Controller
```

响应头至少包含 `Content-Type: application/json` 和 `X-Request-Id`。错误 `code` 是客户端唯一分支依据；`message` 只用于人读，`next_action` 提供可执行恢复动作。

### 1.4 前端请求边界

Next.js App Router 工程的唯一 HTTP 适配器固定为 `frontend/src/lib/request.ts`：

- `frontend/src/api` 只保存 `@umijs/openapi` 生成文件，生成模板通过 `requestLibPath` 指向 `@/lib/request`；禁止手写 API 文件。
- ViewModel 只导入生成函数；ViewModel、View 和 Server Component 不创建 Axios 实例、不拼 URL、不复制 DTO。
- `request.ts` 使用一个 `axios.create` 实例，统一 runtime base URL、超时、凭据、Authorization、AbortSignal、request/trace ID 和错误 envelope 解包。
- 写请求不自动重试；幂等键、`If-Match`、Workspace Header 和取消信号由生成函数的调用方显式传入。
- `ApiClientError` 只暴露 `status/code/request_id/recovery_actions` 等安全字段；不向用户透出后端堆栈、SQL 或 SDK 原始错误。

## 2. 模块文件与语义命名

`backend/src/<module>` 只直接放置真实用例需要的职责文件：

```text
model.go       # plain struct、值对象、模块状态和不变量
service.go     # 语义化 ModuleService，用例、事务和权限编排
repository.go  # 语义化 ModuleRepository，GORM 映射和查询
controller.go  # 语义化 ModuleController，HTTP 边界
ports.go       # 仅真实用例需要的最小 Port/Store
```

文件名固定，但类型和构造函数必须体现模块含义，例如：

- `scripts`: `ScriptAnalysisService`、`ScriptRepository`、`ScriptController`、`ScriptAnalysisPort`；
- `agents`: `AgentService`、`AgentRepository`、`AgentController`、`AgentRunStore`；
- `generationplanning`: `GenerationPlanService`、`GenerationPlanRepository`、`GenerationPlanController`、`GenerationPlanStore`。

禁止 `Service`、`Repository`、`Controller`、`Store`、`Manager` 这种无语义公共类型，禁止 `domain/application/adapters/transport` 子目录，禁止跨模块直接导入另一模块的 Repository。

### 2.1 泛用工具层

允许 `backend/src/platform/toolkit` 承载跨模块且不包含业务语义的稳定工具：配置环境变量解析、HTTP Bearer/IP 解析、密码/令牌以外的通用随机值和 SHA-256 编码。工具按能力拆文件，不建设 Hutool 式万能 `utils` 包；业务状态、角色、错误码、限流策略和 Workspace 规则必须留在所属模块并使用 typed string/struct 表达。新增工具必须至少被两个模块使用，且能用标准库实现可直接单测。

## 3. 实施顺序

1. 引入 GORM 和 `platform/database` 的单一连接初始化，不改变 `current.sql`，关闭 AutoMigrate。
2. 引入 `platform/httpapi`，先把所有已启用 Controller 收敛到同一响应和异常处理。
3. 将 `scripts` 的剧本审批、canonical analysis materialization、fixture candidate 和 selection 迁移到 GORM Repository 方法，并以事务测试验证回滚/重复调用。
4. 依次迁移 `identity`、`agents`、`generationplanning` 的 Repository；每次只改当前模块，不保留同一用例的 pgx/GORM 双实现。
5. 将 frontend 生成配置和所有生成 API 固定到 `src/lib/request.ts`，运行 OpenAPI 生成、TypeScript、单元、构建和浏览器验证。

## 4. 设计验收

- 全仓库业务 Go 代码不出现手写 CRUD SQL；允许的 SQL 只存在于 `schema/current.sql`、Schema fingerprint 和 ORM driver 连接底座。
- 所有 Controller 都通过 `platform/httpapi` 返回成功/失败 envelope；仓库中不存在局部 `writeData/writeError/writeErr`。
- panic、未知 error、NotFound、Validation、Conflict、RateLimited 和依赖不可用均有确定的 `HTTPStatus + ErrorCode`。
- `frontend/src/lib/request.ts` 是唯一 Axios 实例；`frontend/src/api` 全部由同一 OpenAPI Schema 生成且生成后无 diff。
- `go vet ./...`、`go test ./...`、前端 lint/typecheck/test/build、Agent tests 和真实 API/浏览器闭环必须通过；未具备 MinIO/Agent/Elastic 等外部条件时，验收状态只能记录为 `not_run`。

## 5. 采用依据

- GORM 官方文档明确提供 Generics API、事务、关联、批量写入、Upsert 和锁等 Repository 所需能力：<https://gorm.io/docs/>、<https://gorm.io/docs/transactions.html>。
- Go 官方数据库文档将 GORM 与 Ent 列为常用 Go ORM，并要求事务使用统一事务句柄：<https://go.dev/doc/database/>。
- 本项目选择 GORM 而不是 Ent，是因为当前 Schema 已冻结且禁止 ORM 迁移/生成目录；GORM 可在不生成第二套实体目录的前提下，用 Repository 内映射记录逐步接管现有模块。
