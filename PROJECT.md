# Lanverse 工程规范

> 目标规范，2026-09-25 按 [DES-01 系统架构（第 3 版）](docs/design/01-系统架构设计.md) 与 [DES-08 技术选型（第 3 版）](docs/design/08-技术选型决策.md) 编写，随 PRD-01–PLN-01 一同评审；需求规格见 `docs/requirement/`。
> 仓库中现有的 `backend/`、`agent/`、`frontend/` 代码是旧实现，**不符合本规范**，按 [PLN-01 第 5 节](docs/plan/01-实施路线与交付计划.md#5-现有代码的处置已确认方案-a) 处置。本文的路径与命令是目标约定，不表示已落地。

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

## 2. 仓库结构与职责

```text
Lanverse/
  backend/          Go：API（Gin）、领域模块、Temporal 工作流与 Worker、Outbox relay 与 Kafka 消费者、媒体处理
  agent/            Python：FastAPI + Temporal Activity Worker + Agent Harness + 供应商适配器
  frontend/         Next.js：Web 应用（流水线视图、画布、审阅、时间线、任务中心）
  contracts/
    activities/     Go 工作流 ↔ Python Activity 的输入输出 JSON Schema 与样例
  deploy/           Docker Compose、镜像、中间件配置、环境模板
  docs/             生命周期文档：产品需求、需求规格、设计、计划、测试、运维、验收
```

| 单元 | 必须负责 | 不得负责 |
| --- | --- | --- |
| `backend/` | 全部业务事实、权限、计费、公共 API、全部工作流定义、写库与媒体 Activity、Outbox 与事件消费、SSE | 直接调用模型供应商 |
| `agent/` | `agent` 队列的 Activity：Harness 执行的 LLM 任务、供应商 submit / query / cancel、内容审核；内部调试接口 | 连接业务数据库；编排业务流程；持有 MinIO 管理凭据；自动重提结果未知的付费请求 |
| `frontend/` | 界面、交互、服务端状态缓存、编辑器局部状态、按参数 schema 渲染表单 | 持有供应商密钥；直连 Agent 服务、Temporal、Kafka、Redis |
| `contracts/` | 跨语言契约 | 业务逻辑 |

**调用路径：**

```text
浏览器 → Next.js → backend-api(Gin) ─┬─ 命令 / 查询 → PostgreSQL（业务表 + Outbox）
                                     ├─ 启动 / 信号 → Temporal ─┬─ flow  队列 → backend-worker
                                     │                          ├─ media 队列 → backend-worker（FFmpeg → MinIO）
                                     │                          └─ agent 队列 → agent-worker → 模型供应商
                                     └─ SSE ← Redis Pub/Sub ← backend-relay ← Kafka ← Outbox
浏览器 ↔ MinIO：预签名 URL 上传 / 下载
```

## 3. 技术栈

| 范围 | 技术 |
| --- | --- |
| 前端 | Next.js（App Router）、React、TypeScript strict、pnpm、Tailwind CSS、shadcn/ui、Radix UI、lucide-react、ESLint、Prettier |
| 前端组件 | TanStack Query、Zustand + Immer、React Hook Form + Zod、@xyflow/react、Tiptap（Mention）、TanStack Table + TanStack Virtual、dnd-kit、Sonner、next-themes、Streamdown、openapi-typescript + openapi-fetch；CopilotKit（AG-UI） |
| 后端 | Go、Gin、GORM（pgx 驱动）、golang-migrate、Viper、Zap、Wire、swag + gin-swagger、go-playground/validator |
| 后端集成 | Temporal Go SDK、go-redis v9（redis_rate、redsync）、franz-go、minio-go v7、OpenTelemetry Go |
| Agent 服务 | Python 3.12+、uv、FastAPI、Uvicorn、Pydantic v2、pydantic-settings、Temporal Python SDK、httpx、redis-py、OpenAI 兼容 SDK；ag-ui-protocol |
| 工作流 | Temporal（自建，PostgreSQL 持久化） |
| 中间件 | PostgreSQL、Redis、Kafka（KRaft）、对象存储（开发 MinIO，生产火山引擎 TOS，均为 S3 协议） |
| 媒体 | FFmpeg / ffprobe |
| 可观测 | OpenTelemetry Collector、Prometheus、Grafana、Loki、Tempo / Jaeger、Temporal UI、Kafka UI |
| 部署 | Docker、Docker Compose；规模化后 Kubernetes |

每个中间件的职责边界见 [DES-08 §6](docs/design/08-技术选型决策.md#6-中间件职责)；与 LibTV 的技术对齐与差异见 [DES-08 §10](docs/design/08-技术选型决策.md#10-与-libtv-的技术对齐)。暂不引入：Elasticsearch、独立向量库、图数据库、服务网格、微服务拆分。

## 4. Go 后端

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
  db/migrations/                 # golang-migrate 版本化 SQL（唯一 Schema 来源）
  docs/                          # swag 生成的 OpenAPI，禁止手改
  Makefile
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
17. 文件名 `snake_case.go`；单元测试就近 `*_test.go`；集成测试放 `backend/tests/`，用 testcontainers 启动 PostgreSQL、Redis、Kafka、MinIO，工作流用 Temporal testsuite 与回放测试。

## 5. Agent 服务

```text
agent/
  app/
    main_api.py                  # FastAPI 应用工厂（agent-api）
    main_worker.py               # Temporal Activity Worker 入口（agent-worker）
    config.py                    # pydantic-settings 集中校验配置
    activities/                  # Activity 定义：输入 → Harness / 适配器 → 输出（对应 contracts/activities）
    harness/
      skills/                    # Skill Registry：加载、版本、hash
      context.py                 # Context Builder
      router.py                  # Model Router（读模型注册表快照）
      tools.py                   # Tool Registry（只读工具、按任务白名单）
      loop.py                    # 执行循环：调用 → 校验 → 修复
      validators/                # schema、原文位置、业务规则
      budget.py                  # token / 费用 / 时长
      trace.py                   # 执行追踪
    providers/<适配器>.py        # 供应商：角色映射、submit / query / cancel、用量
    moderation/
    api/                         # agent-api 路由：健康检查、Harness 调试与评测
  skills/<skill>/<版本>/         # SKILL.md、references/、schema
  evals/                         # 评测集与离线回归（录制的模型响应）
  tests/
  pyproject.toml
  uv.lock
```

1. 任务只经 Temporal Activity 进入（MVP）；输入冻结：版本 ID、原文片段、参考描述、媒体预签名 URL、Skill 版本、预算。
2. 不连接业务数据库；结果、Trace、用量作为 Activity 返回值，由 Go 落库。
3. 付费请求使用 Go 生成的 `provider_request_key`；超时或响应丢失返回“结果未知”，不自动重提。
4. 长任务定期 heartbeat，响应 Temporal 取消。
5. Harness 的工具只读，按任务白名单授予；Skill 修改须更新版本并通过 `evals/` 回归。
6. 供应商凭据只在本服务环境中，不写日志、不回传。
7. 工具：`uv sync --locked`、`uv run ruff check .`、`uv run ruff format --check .`、`uv run mypy app`、`uv run pytest`。

## 6. 前端

```text
frontend/
  src/
    app/                         # 路由与布局装配
    features/<业务>/             # project script bible storyboard operation review edit delivery canvas
      components/                #   业务组件
      queries.ts                 #   TanStack Query 查询与 mutation
      store.ts                   #   局部状态（按需，Zustand）
    features/operation/param-form/  # 按 ModelProfile.param_schema 渲染参数表单
    features/canvas/
      engine/                    #   React Flow 装配、视口、LOD、视频播放管控
      document/                  #   画布文档类型、命令、撤销重做、同步与冲突处理
      nodes/                     #   NodeShell 与三类节点
      panels/                    #   工具栏、创建菜单、设置弹层、编辑弹窗
    components/ui/               # shadcn/ui（Radix）基础组件
    lib/                         # api 客户端封装、sse、auth
    gen/api/                     # 由后端 OpenAPI 生成，禁止手改
  tests/                         # unit、e2e
  eslint.config.mjs              # ESLint flat config
  .prettierrc.json               # Prettier 配置（含 tailwind 插件）
  components.json                # shadcn 配置（Radix 体系）
```

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
11. 脚本：`pnpm dev`、`pnpm build`、`pnpm lint`、`pnpm format`、`pnpm format:check`、`pnpm typecheck`、`pnpm test`、`pnpm test:e2e`、`pnpm openapi`；检查脚本不修改文件。

## 7. 契约与生成

| 契约 | 唯一来源 | 生成物 |
| --- | --- | --- |
| 公共 REST | Gin Handler 的 swag 注解 + DTO | `backend/docs`（OpenAPI）→ `frontend/src/gen/api` |
| Activity | `contracts/activities/<名称>.v<N>.schema.json` + 样例 | Go 结构体、Python Pydantic 模型 |
| 数据库 | `backend/db/migrations/` | — |
| 事件 | `contracts/events/<主题>.schema.json`（首个消费者出现时创建） | Go 结构体 |

1. 修改顺序：改契约 → 生成代码 → 改实现 → 契约测试。禁止手改生成物。
2. CI 重新生成并比对，不一致则失败；Activity 样例在两端测试中都要通过。
3. 不兼容变更升级版本号；旧版本在仍有在途工作流或未消费事件时保留。

## 8. 配置、数据与安全

- 配置来自环境变量（Viper / pydantic-settings），启动时集中校验；`.env.example` 只记录占位值与说明，真实 `.env` 与凭据不入库。
- 供应商凭据只注入 Agent 服务；MinIO、Kafka、Redis、Temporal 凭据只注入需要的单元。
- 测试数据必须合成或脱敏。构建产物、缓存、日志、本地数据卷不进入仓库。

## 9. 质量门禁与交付

| 端 | 必须通过 |
| --- | --- |
| Go | `gofmt`、`goimports`、`go vet`、`golangci-lint`、`go test -race ./...`、`govulncheck`、swag 与 Wire 生成一致性 |
| Python | ruff check、ruff format --check、mypy、pytest |
| 前端 | `pnpm lint`、`pnpm format:check`、`pnpm typecheck`、`pnpm test`、`pnpm build`；交互变化补 Playwright |
| 契约 | OpenAPI → 前端客户端生成一致；Activity 样例两端通过 |

1. 核心业务逻辑（Operation 状态机、预算与对账、依赖传播、工作流）先写测试再实现。
2. 每个里程碑的验收记录在 `docs/acceptance/`；静态检查通过不等于功能验收通过。
3. 提交与分支规则见 `AGENTS.md`。

## 10. 规范维护

新增仓库单元、存储、中间件或工作流 Owner 之前，必须先在设计文档中说明理由、替代方案与迁移方式，评审接受后再更新本文。
