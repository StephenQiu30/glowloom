# Lanverse 工程规范

> 目标规范，2026-09-25 按 [DES-01 系统架构（第 3 版）](docs/design/01-系统架构设计.md) 与 [DES-08 技术选型（第 3 版）](docs/design/08-技术选型决策.md) 编写，随 PRD-01–PLN-01 一同评审；需求规格见 `docs/requirement/`。
> 旧实现已于 2026-09-26 删除（BACKLOG M1-01），保留在标签 `legacy-2026-09`，按 [PLN-01 第 5 节](docs/plan/01-实施路线与交付计划.md#5-现有代码的处置已确认方案-a) 逐项审计后按需搬运。本文的路径与命令是目标约定，随 M1 落地。

## 1. 文件职责

| 文件 | 唯一职责 |
| --- | --- |
| `AGENTS.md` | 协作、授权、代码质量与 Git 规则 |
| `PROJECT.md` | 仓库结构、技术栈、目录与编码约定、质量门禁（本文件） |
| `DESIGN.md` | 视觉与交互规范 |
| `README.md` | 项目简介、启动方式、文档入口 |
| `BACKLOG.md` | 项目进度与待执行任务清单（任务、实现要点、涉及文件、状态、提交） |
| `docs/prd/` | PRD：产品需求文档与竞品调研（编号规则见 `docs/README.md`） |
| `docs/requirement/` | REQ：功能需求总表、非功能需求、业务流程与用例、术语、界面；06 起为功能需求文件（一个功能一个文件） |
| `docs/design/` | DES：01–08 架构、数据、接口、工作流、Agent、画布、安全设计与技术选型；09 起为功能设计（与功能需求文件一一对应） |
| `docs/plan/` | PLN：实施路线、项目管理与变更 |
| `docs/test/` | TST：测试策略、需求追踪矩阵、AI 评测 |
| `docs/operation/` | OPS：环境部署、CI/CD、监控告警、备份恢复与故障响应 |
| `docs/acceptance/` | 每个里程碑的验收记录 |

业务范围以 `docs/prd/` 与 `docs/requirement/` 为准，架构决策以 `docs/design/` 为准；本文把其中的工程约定落到目录和工具上。两者冲突时，先修改需求或设计文档并评审，再同步本文。

2026-09-30 用户进一步明确：按上线标准实现完整 Lanverse，当前优先跑通整项目验证和 Demo；画布直接复用 BeefTV 固定提交 0d9e9f48 的 DOM/SVG/rAF InfiniteCanvas 核心及对应设计，替换现有实现（见 BeefTV 引入设计、DES-06/38）。本轮接正式 text/image/video/audio/group 资源节点与 Go/PostgreSQL 合同，不保留独立 PoC 产品入口、旧 live/creation 引擎或双状态；不迁整个 BeefTV provider/3D/插件/时间轴。完整生成/参考/选定/Agent 仍按产品范围持续验收，单次 Demo 不等于 MVP 完成。现有 SQL/业务历史保留，字段演进追加迁移。用户随后明确要求先清理独立 Python `agent/` 服务，范围见 [Agent 服务目录清理](docs/design/Agent服务目录清理设计.md)；其执行能力尚未由 Go 承接，M1-12 继续评估和实施。

2026-09-30 用户明确当前先保证 PoC 页面和服务可用，不需要登录认证。用户随后指定直接清理登录功能，不保留兼容分支或免登录开关；单一工作区复用正式项目/画布持久化，消费者认证后置。配置、身份、写入和失败边界见 BeefTV 引入设计 §4.1。

当前主要参考项目为 [glanderness/BeefTV](https://github.com/glanderness/BeefTV)，源码基线固定为 [0d9e9f48d407570cd431ad9730cdd522b06810c0](https://github.com/glanderness/BeefTV/tree/0d9e9f48d407570cd431ad9730cdd522b06810c0)。当前复用合同以 [BeefTV 迁移设计](docs/design/BeefTV能力引入设计.md) 和 [第三方代码声明](THIRD_PARTY_NOTICES.md) 为准；LibTV 与旧 infinite-canvas 的历史调研、业务规则来源与许可证据继续保留，不作为当前主要参考或画布引擎建议。

## 2. 仓库结构与职责

```text
Lanverse/
  .env.example              本机进程配置样例；实际 .env 不入库
  docker-compose.yml        应用服务（frontend、backend-api、backend-worker、backend-worker-media、backend-relay）及维护角色
  docker-compose-env.yml    完整部署依赖环境（PostgreSQL、Redis、Kafka、MinIO、Temporal）
  backend/          Go：API（Gin）、领域模块、Temporal 工作流与 Worker、Outbox relay 与 Kafka 消费者、媒体处理
  frontend/         Next.js：Web 应用（流水线视图、画布、审阅、时间线、任务中心）
  docs/             生命周期文档：产品需求、需求规格、设计、计划、测试、运维、验收
```

不设 `contracts/`：公共 REST 契约由后端 Gin 注解与 DTO 经 swag 自动生成，在后端 Swagger 端点在线提供，再由 `@umijs/openapi` 生成前端 API（见 §7）；现有 Activity 与事件的输入输出由 Go 手写类型，不引入单独的 schema 文件与代码生成流水线。被移除的 Python 执行端协议保留在 DES-03，Go 承接时须验证兼容性。

| 单元 | 必须负责 | 不得负责 |
| --- | --- | --- |
| `backend/` | 全部业务事实、权限、计费、公共 API、全部工作流定义、写库与媒体 Activity、Outbox 与事件消费、SSE | M1-12 接入前直接调用模型供应商；在 Workflow 内执行 I/O |
| `frontend/` | 界面、交互、服务端状态缓存、编辑器局部状态、按参数 schema 渲染表单 | 持有供应商密钥；直连 Agent 服务、Temporal、Kafka、Redis |

**调用路径：**

```text
浏览器 → Next.js → backend-api(Gin) ─┬─ 命令 / 查询 → PostgreSQL（业务表 + Outbox）
                                     ├─ 启动 / 信号 → Temporal ─┬─ flow  队列 → backend-worker
                                     │                          ├─ media 队列 → backend-worker（FFmpeg → MinIO）
                                     │                          └─ agent / agent.mock 队列 → 执行端待补（M1-12）
                                     └─ SSE ← Redis Pub/Sub ← backend-relay ← Kafka ← Outbox
浏览器 ↔ MinIO：预签名 URL 上传 / 下载
```

## 3. 技术栈

| 范围 | 技术 |
| --- | --- |
| 前端 | Next.js（App Router）、React、TypeScript strict、pnpm、Tailwind CSS、shadcn/ui、Radix UI、lucide-react、ESLint、Prettier |
| 前端组件 | TanStack Query、Zustand + Immer、React Hook Form + Zod、BeefTV InfiniteCanvas 核心、Tiptap（Mention）、TanStack Table + TanStack Virtual、dnd-kit、Sonner、next-themes、Streamdown、`@umijs/openapi` + Axios；CopilotKit（AG-UI） |
| 后端 | Go、Gin、GORM（pgx 驱动）、golang-migrate、Viper、Zap、Wire、swag + gin-swagger、go-playground/validator |
| 后端集成 | Temporal Go SDK、go-redis v9（redis_rate、redsync）、franz-go、minio-go v7、OpenTelemetry Go |
| 工作流 | Temporal（自建，PostgreSQL 持久化） |
| 中间件 | PostgreSQL、Redis、Kafka（KRaft）、对象存储（开发 MinIO，生产火山引擎 TOS，均为 S3 协议） |
| 媒体 | FFmpeg / ffprobe |
| 可观测 | OpenTelemetry Collector、Prometheus、Grafana、Loki、Tempo / Jaeger、Temporal UI、Kafka UI |
| 部署 | Docker、Docker Compose；规模化后 Kubernetes |

每个中间件的职责边界见 [DES-08 §6](docs/design/08-技术选型决策.md#6-中间件职责)；当前 BeefTV 参考范围与历史技术比较见 [DES-08 §10](docs/design/08-技术选型决策.md#10-参考项目与技术边界)。暂不引入：Elasticsearch、独立向量库、图数据库、服务网格、微服务拆分。

## 4. Go 后端

编码标准以 [Google Go Style Guide](https://google.github.io/styleguide/go/guide)、[Style Decisions](https://google.github.io/styleguide/go/decisions) 为准，按场景落实 [Best Practices](https://google.github.io/styleguide/go/best-practices)；Go 官方文档与 Uber 规范的职责及优先级见 [AGENTS.md 的 Go 工程规范](AGENTS.md#go-工程规范)。实现与审查均须核对适用的命名、注释、错误、接口、并发和测试规则，格式化与 lint 不能替代人工审查。

```text
backend/
  cmd/lanverse/main.go           # 入口：--role=api|worker|relay|all，只做配置与启动
  internal/
    app/                         # 组合根：Wire 装配、生命周期、按角色注册路由 / Worker / 消费者
    command/                     # 命令层：鉴权、幂等键、expected_version、审计、Outbox
    <上下文>/                    # identity workspace script bible storyboard operation catalog
      domain/                    #   media audio edit delivery billing canvas lineage
      application/               # 用例、事务边界、消费方定义的端口
      adapter/
        http/                    # Gin Handler + swag 注解 + 请求 / 响应 DTO
        postgres/                # GORM 仓储与持久化模型
        workflow/                # 本上下文的 Temporal 工作流与 Go Activity
        event/                   # 本上下文的 Kafka 消费者（按需）
    platform/                    # config(Viper) log(Zap) db(GORM) redis kafka minio temporal ffmpeg otel auth
  tests/<模块>/                  # 按模块集中存放外部测试包与集成测试
  db/migrations/                 # golang-migrate 版本化 SQL（唯一 Schema 来源）
  docs/                          # swag 从注解生成的 Swagger 规范，禁止手改；由 backend-api 在线提供
```

1. 依赖方向 `adapter → application → domain`；`domain` 不依赖 Gin、GORM、Temporal、Kafka、Redis 或任何 SDK。
2. 上下文之间只调用对方 `application` 暴露的接口，不读写对方的表。
3. Gin Handler 只做参数绑定与校验、调用用例、映射响应；用例不接收 `*gin.Context`，而是 `context.Context`。
4. GORM 模型只在 `adapter/postgres`；领域对象、GORM 模型、API DTO 显式转换。生产禁止 `AutoMigrate`，Schema 只由 `db/migrations` 演进。
5. 一个用例一个事务；需要异步的后续动作写 Outbox，由 relay 投递到 Kafka。
6. 所有写操作经命令层：权限、幂等键、`expected_version`（不匹配返回 409）、审计。
7. 所有业务表带 `org_id` / `project_id`，仓储查询强制带项目条件。
8. **工作流只写结果与状态字段**，不覆盖人工配置（DES-01 原则 P3）。
9. 工作流：只用 Go 编写，代码确定性、无 I/O；Workflow ID 用业务 ID；Activity 先查已有结果再执行；代码变更使用 Temporal 版本化机制。
10. Operation 状态机、`unknown` 对账、预留与结算遵循 [DES-01 §4、§6.2](docs/design/01-系统架构设计.md#4-生成操作operation模型)。
11. Kafka：主题 `lanverse.<上下文>.<事件>.v<N>`，键为 `project_id`；消费者按事件 ID 去重。
12. Redis：只放可重建的数据（会话、缓存、限流、锁、Pub/Sub）；键名 `lanverse:<用途>:<标识>`，设置过期时间。
13. 对象存储（开发 MinIO / 生产 TOS）：只通过 S3 协议访问，不使用厂商私有 API；桶私有；对象键 `projects/{project_id}/{类别}/{id}`；浏览器只通过预签名 URL 访问。
14. 依赖由 Wire 在组合根注入（`wire.go` 声明 Provider Set，`wire_gen.go` 为生成物，禁止手改，CI 校验生成一致）；接口由消费方按需定义；不使用 `Ixxx` / `Impl`，不建 `utils`、`common`。
15. `context.Context` 沿调用链传递；错误用 `%w` 包装并保留可判定的错误链；goroutine 必须有所有者、取消与等待。
16. 日志统一用 Zap（不混用标准库 `log` / `slog`）的结构化字段（`trace_id`、`project_id`、`operation_id`），不记录凭据与剧本全文。
17. 文件名 `snake_case.go`；Go 测试集中放在 `backend/tests/<模块>/`，按被测模块分目录并使用外部测试包（`<package>_test`）；生产源码目录不存放 `*_test.go`。集成测试放在对应模块目录中并使用 testcontainers 启动 PostgreSQL、Redis、Kafka、MinIO，工作流用 Temporal testsuite 与回放测试。

## 5. AI 执行能力的后续承接

独立 Python 服务目录、启动入口、镜像与质量门禁已移除。供应商 submit/query/cancel、凭据解封与测试、审核、Skill/Harness 当前没有运行实现，后续由 M1-12 形成 Go 承接设计。现有 `agent` / `agent.mock` 队列名称与 Activity 协议仍在 Go 工作流中保留，不代表有 Worker 正在执行这些任务。

冻结输入、预算上限、只读工具白名单、长任务 heartbeat/取消，以及结果未知不自动重提付费请求的业务约束继续有效；迁移须补齐测试、历史兼容与真实供应商证据，不恢复独立 Python 服务作为默认前提。范围与失败路径见 [清理设计](docs/design/Agent服务目录清理设计.md)。

## 6. 前端

编码与审查遵循 [Vercel React Best Practices](https://vercel.com/blog/introducing-react-best-practices) 和 [Next.js 官方文档](https://nextjs.org/docs/app)，通过 Vercel 插件的 `vercel:react-best-practices`、`vercel:nextjs` 技能读取相关规则。执行顺序与检查项见 [AGENTS.md 的前端工程规范](AGENTS.md#前端工程规范)，具体 API 同时核对安装版本的 `node_modules/next/dist/docs/`。外部示例中的数据请求或缓存库须映射到本项目既有 TanStack Query、统一请求封装和 Go 业务合同；工具链变更仍遵循设计评审规则。

```text
frontend/
  src/
    app/                         # 路由与布局装配
    components/
      canvas/                    # 画布装配、文档命令、查询和 Zustand 状态
        engine/                  # BeefTV DOM/SVG/rAF 核心、视口、LOD、媒体播放管控
        nodes/                   # 节点组件与媒体生命周期
      catalog/                   # 按 ModelProfile.param_schema 渲染参数表单
      operation/                 # 报价确认组件与查询钩子
      project/                   # 项目列表、创建对话框与预算组件
      workbench/                 # 工作台页面组件、布局与路由映射
      ui/                        # shadcn/ui（Radix）基础组件
    lib/                         # 前端基础能力
      request.ts                  # Axios 请求封装；普通 HTTP 与流式连接的统一入口
    gen/api/                     # @umijs/openapi 从后端在线 Swagger 文档生成，禁止手改
  tests/                         # unit、e2e
  eslint.config.mjs              # ESLint flat config
  .prettierrc.json               # Prettier 配置（含 tailwind 插件）
  components.json                # shadcn 配置（Radix 体系）
```

2026-09-30 按用户指定的目录方式，业务组件直接放在 `components/<业务>/`。模块私有的查询、钩子、类型、状态与就近测试随组件放在同一业务目录；共享请求与基础能力继续在 `lib/`，生成 API 继续在 `gen/api/`。不再设置 `src/features/` 或重复嵌套的业务 `components/` 层，尚未实现的模块不预建目录。

1. 服务端事实只来自 TanStack Query；SSE 事件只用于让相关查询失效。
2. 编辑器高频交互状态放 Zustand（嵌套更新用 Immer）；持久化一律通过后端命令接口。
3. 基础控件、表单、弹窗、菜单、表格一律用 shadcn/ui；不混用其他组件体系（不引入 Ant Design）。
4. 富文本与实体引用用 Tiptap；`@角色 / @场景 / @道具` 保存为结构化引用，不只保存纯文本。
5. 超过 100 行的列表（镜头表、资产库、任务中心）使用 TanStack Virtual。
6. Agent 界面使用 CopilotKit + AG-UI；Agent 下发的画布修改只作为提案展示，用户确认后经命令接口提交；付费生成一律由用户二次确认。
7. 模型参数表单只由 `param_schema` 驱动。
8. 画布：拖拽只在松手时提交命令；命令带 `expected_revision` 与幂等键；409 时基于最新文档重放。
9. 媒体：按缩放级别选择缩略图；视频默认封面，同时播放不超过 3 个；只渲染视口内节点。
10. 付费操作先展示报价并由用户确认。
11. 所有普通 HTTP 请求通过 `src/lib/request.ts` 中的 Axios 封装；生成函数调用它，业务代码调用生成函数。SSE 与 AG-UI 流式连接也从该文件导出连接入口；不得在业务模块中另建请求实例或直连后端。预签名 URL 上传沿用该入口，不携带后端会话与 CSRF 头到对象存储。
12. 不设置项目脚本或 Makefile；前端开发和检查直接执行 `pnpm exec next`、`pnpm exec eslint`、`pnpm exec prettier`、`pnpm exec tsc`、`pnpm exec vitest`、`pnpm exec openapi2ts`。

## 7. 契约与生成

| 契约 | 唯一来源 | 生成物 |
| --- | --- | --- |
| 公共 REST | Gin Handler 的 swag 注解 + DTO | `backend/docs`（生成的 Swagger 2.0）→ 后端在线 `/swagger/doc.json` → `@umijs/openapi` → `frontend/src/gen/api` |
| Activity | 现有 Go 类型；DES-03 §7.1 表格为字段的唯一权威描述，原 Python 执行端待 Go 承接 | — |
| 数据库 | `backend/db/migrations/` | — |
| 事件 | Go 结构体，按主题版本号 `.v<N>` 手写，生产者与消费者各自维护 | — |

1. REST：改 Handler / DTO 与注解 → 运行 swag 自动生成 → 启动后端并从在线 `/swagger/doc.json` 运行 `@umijs/openapi` → 改前端实现。生成器配置 `schemaPath` 为在线地址、`requestImportStatement` 为 `@/lib/request` 的导入语句；Swagger 文档与前端 API 文件均禁止手改。后端未启动或在线规范不可用时生成失败，不以旧文件代替。
2. Activity 与事件：改 DES-03 表格 → 改 Go 类型 → 契约测试。同一组示例固定字段语义；M1-12 承接旧执行端时还须验证历史输入输出与队列兼容性。不做代码生成。
3. 不兼容变更升级版本号（Activity 名称或事件主题后缀 `.v<N>`）；旧版本在仍有在途工作流或未消费事件时保留。

Wire 组合根修改后，在 `backend/` 直接执行 `wire ./internal/app`，再执行 `goimports -local github.com/StephenQiu30/lanverse/backend -w internal/app/wire_gen.go`；`wire_gen.go` 仅由工具生成和格式化，CI 重复这两条命令并比较文件，不设脚本或 Makefile。

## 8. 配置、数据与安全

- 配置来自环境变量（Go 使用 Viper），启动时集中校验；`.env.example` 只记录占位值与说明，真实 `.env` 与凭据不入库。
- 供应商凭据仅供服务端使用，现有封装保留，解封与执行端待 M1-12；MinIO、Kafka、Redis、Temporal 凭据只注入需要的单元。
- 测试数据必须合成或脱敏。构建产物、缓存、日志、本地数据卷不进入仓库。

## 9. 质量门禁与交付

| 端 | 必须通过 |
| --- | --- |
| Go | `gofmt`、`goimports`、`go vet`、`golangci-lint`、`go test -race ./...`、`govulncheck`、swag 与 Wire 生成一致性 |
| 前端 | `pnpm exec eslint .`、`pnpm exec prettier --check .`、`pnpm exec next typegen` + `pnpm exec tsc --noEmit`、`pnpm exec vitest run`、`pnpm exec next build`；交互变化补 Playwright |
| 契约 | swag 生成物与后端在线 Swagger 一致；`@umijs/openapi` 从在线文档重生的 API 一致；公开路由文档覆盖；现有 Go Activity 样例通过，原执行端的真实链路待 M1-12 |

1. 核心业务逻辑（Operation 状态机、预算与对账、依赖传播、工作流）先写测试再实现。
2. 每个里程碑的验收记录在 `docs/acceptance/`；静态检查通过不等于功能验收通过。
3. 提交与分支规则见 `AGENTS.md`。
4. Go 与前端代码审查分别核对 Google Go 规范和 Vercel 最佳实践；性能修改说明所采用规则、测量结果与适用边界。规范审查与自动化门禁分别记录，跳过项明确说明。

## 10. 规范维护

新增仓库单元、存储、中间件或工作流 Owner 之前，必须先在设计文档中说明理由、替代方案与迁移方式，评审接受后再更新本文。
