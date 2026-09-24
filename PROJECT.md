# Lanverse 工程规范

> 目标规范，2026-09-25 按从零重新设计的 [0002 系统架构](docs/design/0002-系统架构设计.md) 与 [0003 技术选型](docs/design/0003-技术选型决策.md) 重写，随 0001–0004 一同评审。
> 仓库中现有的 `backend/`、`agent/`、`frontend/` 代码是旧实现，**不符合本规范**，按 [0004 第 5 节](docs/design/0004-实施路线与交付计划.md#5-现有代码的处置待确认) 处置。本文的路径与命令是目标约定，不表示已落地。

## 1. 文件职责

| 文件 | 唯一职责 |
| --- | --- |
| `AGENTS.md` | 协作、授权、代码质量与 Git 规则 |
| `PROJECT.md` | 仓库结构、技术栈、目录与编码约定、质量门禁（本文件） |
| `DESIGN.md` | 视觉与交互规范 |
| `README.md` | 项目简介、启动方式、文档入口 |
| `docs/design/` | 产品与技术设计、决策记录 |
| `docs/acceptance/` | 每个里程碑的验收记录 |

业务范围与架构决策以 `docs/design/` 为准；本文只把其中的工程约定落到目录和工具上。两者冲突时，先修改设计文档并评审，再同步本文。

## 2. 仓库结构与职责

```text
Lanverse/
  backend/          Go：业务 API、Temporal 工作流、媒体 Worker（一个 Go Module，一个二进制，按角色启动）
  agent/            Python：Temporal AI Activity Worker（模型网关、结构化推理、供应商调用）
  frontend/         Next.js：Web 应用（流水线视图、画布、审阅、时间线）
  contracts/
    openapi/        公共 REST 契约（唯一来源）
    activities/     Go 工作流 ↔ Python Activity 的 JSON Schema 契约与样例
  deploy/           Compose、镜像与环境配置模板
  docs/             设计与验收文档
```

| 单元 | 必须负责 | 不得负责 |
| --- | --- | --- |
| `backend/` | 全部业务事实的读写、权限、计费、公共 API、工作流定义、媒体处理 Activity、SSE 推送 | 直接调用模型供应商；把前端或 Worker 的数据当作已校验事实 |
| `agent/` | 按冻结输入执行 AI Activity：LLM 结构化输出与校验修复、生图 / 生视频 / TTS 供应商调用、内容审核调用 | 连接业务数据库；决定业务状态；自行重试未知结果的付费调用 |
| `frontend/` | 页面、交互、服务端状态缓存、编辑器局部状态 | 持有供应商密钥；直连 Worker、Temporal 或供应商；把本地状态当作已保存 |
| `contracts/` | 跨端契约及样例 | 业务逻辑 |

**标准调用路径：**

```text
浏览器 → frontend → backend(api) ─┬─ 查询 / 命令 → PostgreSQL
                                   └─ 启动 / 信号 → Temporal ─┬─ flow  队列 → backend(flow)
                                                               ├─ ai    队列 → agent
                                                               └─ media 队列 → backend(media)
backend(api) ──SSE──→ 浏览器      媒体上传 / 下载：浏览器 ↔ 对象存储（预签名 URL）
```

一个流程只有一个状态机 Owner：Go 工作流。Python 与媒体 Worker 只返回结果，由 Go 工作流中的写库 Activity 落库。

## 3. 技术栈

| 范围 | 技术 | 约束 |
| --- | --- | --- |
| 业务后端 | Go、标准库 `net/http`、oapi-codegen、pgx、sqlc、版本化迁移（goose 或 Atlas） | 契约先行；SQL 显式；禁止启动时自动建表 |
| 编排 | Temporal（Go SDK 写工作流，Python SDK 写 AI Activity） | 工作流代码确定性、无 I/O；业务事实以 PostgreSQL 为准 |
| AI 执行 | Python 3.12+、uv、Pydantic v2、Temporal Python SDK、httpx | 所有调用设置超时与预算；供应商请求键持久化后才提交 |
| 媒体 | FFmpeg / ffprobe（Go Worker 子进程调用） | 输入输出走对象存储；子进程有超时与资源上限 |
| 数据 | PostgreSQL、S3 兼容对象存储 + CDN、Redis | Redis 不存事实；二进制不进数据库和 Git |
| Web | Next.js App Router、TypeScript strict、pnpm | 编辑器页面为客户端组件；服务端组件只做数据预取 |
| UI | Tailwind CSS、shadcn/ui（Radix） | 遵循 `DESIGN.md` |
| 前端状态 | TanStack Query（服务端状态）、Zustand（编辑器局部状态） | 业务事实只来自后端查询 |
| 画布 | `@xyflow/react`；卡片与交互界面移植自 infinite-canvas（MIT，保留版权声明） | 节点只存引用；修改走命令 |
| 实时 | SSE；跨实例经 Redis Pub/Sub | 事件只通知“什么变了”，数据由查询获取 |
| 可观测 | OpenTelemetry、Go `slog`、Python 结构化日志、LLM 观测平台 | 不记录密钥、剧本正文与个人信息 |
| 部署 | Docker 镜像；MVP 托管数据库 / 对象存储 / Redis，Temporal 托管或自建 | 镜像与依赖版本固定，禁止浮动 `latest` |

MVP 不引入：Kafka、Elasticsearch、微服务拆分、图数据库、独立向量库、LangChain / LangGraph 类业务编排框架。引入任何新基础设施前，先在设计文档中说明现有方案为何不足。

## 4. Go 后端

```text
backend/
  cmd/lanverse/main.go           # 入口：--role=api|flow|media|all，只做配置与启动
  internal/
    app/                         # 组合根：装配依赖、生命周期、按角色注册 HTTP / Worker
    <上下文>/                    # identity workspace script bible storyboard media generation
      domain/                    #   review edit delivery billing canvas lineage
      application/               # 用例、事务边界、消费方定义的接口
      adapter/
        http/                    # 实现 oapi-codegen 生成的 strict server 接口
        postgres/                # 仓储；调用 sqlc 生成的查询
        workflow/                # 该上下文的 Temporal 工作流与 Activity（按需）
    platform/                    # db、tx、outbox、sse、objectstore、auth、otel 等基础设施
  db/migrations/                 # 版本化迁移（唯一 Schema 来源）
  db/queries/<上下文>/           # sqlc 查询
  gen/                           # 生成代码（OpenAPI、sqlc、Activity 契约），禁止手改
  Makefile
```

1. 依赖方向 `adapter → application → domain`；`domain` 不依赖数据库、HTTP、Temporal 或任何 SDK。
2. 上下文之间只调用对方 `application` 暴露的接口，不读写对方的表；需要异步协作时用 Outbox 事件或工作流步骤。
3. 一个用例一个事务，事务在 `application` 开启；事务内写 Outbox，提交后由发布器投递。
4. 所有业务表带 `org_id` / `project_id`，仓储查询强制带租户条件。
5. 版本化对象（剧本、造型、镜头、剪辑计划）以“对象 + 不可变版本”建模；更新正式引用必须带 `expected_version`，不匹配返回 409。
6. 工作流 ID 使用业务 ID（如 `generation-job/{job_id}`），保证启动幂等；Activity 执行前先查询已有结果。
7. 付费调用的状态机、`unknown` 对账、费用预留与结算遵循 [0002 §5.2](docs/design/0002-系统架构设计.md#52-生成任务阶段-4678-共用)。
8. 接口由消费方按需定义，构造函数显式注入；不使用 `Ixxx` / `Impl` 命名，不建 `utils`、`common` 包。
9. `context.Context` 沿调用链传递；错误用 `%w` 包装并保留可判定的错误链；goroutine 必须有所有者、取消与等待。
10. 文件名 `snake_case.go`；单元测试就近 `*_test.go`；跨上下文与数据库集成测试放 `backend/tests/`。

## 5. Python AI Worker

```text
agent/
  app/
    main.py                      # Worker 入口：连接 Temporal，注册 ai 队列的 Activity
    config.py                    # pydantic-settings 集中校验配置
    activities/<能力>/           # script_parse、asset_extract、storyboard、image、video、tts、moderation
      activity.py                #   Activity 定义：输入 → 调用 → 校验 → 输出
      schema.py                  #   与 contracts/activities 对应的 Pydantic 模型
    gateway/                     # 模型网关：按能力路由到供应商
      providers/<供应商>.py      #   单个供应商适配：提交、查询、取消、结果、计费口径
    llm/                         # 结构化输出、校验修复循环、原文位置校验
    prompts/<能力>/<版本>/       # 版本化提示词文件
  evals/                         # 评测集与离线回归（录制的模型响应）
  tests/
  pyproject.toml
  uv.lock
```

1. Activity 只接收冻结输入（版本 ID、内容 hash、参数），不查询业务数据库；需要的数据由 Go 工作流在输入中提供，或以对象存储短期地址传入。
2. 付费调用：先由 Go 持久化 `provider_request_key`，Activity 用它提交；超时或响应丢失时返回 `unknown`，**不在 Activity 内自动重新提交**。
3. LLM 输出必须通过 Pydantic 校验；引用原文的字段必须校验原文确实存在；修复次数有上限，超限明确失败并保留原始输出。
4. 提示词修改视为代码变更：更新版本目录、跑 `evals/` 回归后才能发布。
5. 供应商密钥只在 Worker 进程环境中，不写日志、不回传给工作流。
6. 工具：`uv sync --locked`、`uv run ruff check .`、`uv run ruff format --check .`、`uv run mypy app`、`uv run pytest`。

## 6. 前端

```text
frontend/
  src/
    app/                         # 路由与布局装配；(marketing) 与 (studio) 路由组
    features/<业务>/             # project script bible storyboard generation review edit delivery canvas
      components/                #   业务组件
      queries.ts                 #   TanStack Query 查询与 mutation
      store.ts                   #   该业务的局部状态（按需，Zustand）
    features/canvas/
      engine/                    #   React Flow 装配、视口、LOD、视频播放管控
      document/                  #   画布文档类型、命令、撤销重做、同步与冲突处理
      nodes/                     #   NodeShell 与各类卡片（节点只持有 ref_type / ref_id）
      panels/                    #   工具栏、创建菜单、设置弹层、编辑弹窗
    components/ui/               # shadcn/ui 基础组件
    lib/                         # api 客户端封装、sse、auth 等基础设施
    gen/api/                     # 由 contracts/openapi 生成的类型与请求函数，禁止手改
  tests/                         # unit、e2e
```

1. 服务端事实只来自 TanStack Query；SSE 事件只用于让对应查询失效，不直接写缓存中的业务数据。
2. 编辑器（画布、时间线）的高频交互状态放 Zustand；持久化一律通过后端命令接口。
3. 画布：拖拽过程只更新本地，松手提交一条命令；命令带 `expected_revision` 与幂等键；409 时基于最新文档重放本地未确认命令。
4. 媒体：按缩放级别选择缩略图尺寸；视频默认只显示封面，全局同时播放不超过 3 个；只渲染视口内节点。
5. 付费操作必须先展示报价并由用户确认。
6. 组件优先使用 shadcn/ui；业务层不自行实现基础控件、弹窗和菜单。
7. 工具：`pnpm install --frozen-lockfile`、`pnpm lint`、`pnpm format:check`、`pnpm typecheck`、`pnpm test`、`pnpm build`、`pnpm test:e2e`。

## 7. 契约与生成

| 契约 | 唯一来源 | 生成物 |
| --- | --- | --- |
| 公共 REST | `contracts/openapi/lanverse.yaml` | `backend/gen/api`（oapi-codegen）、`frontend/src/gen/api`（TS 客户端） |
| Activity | `contracts/activities/<名称>.v<N>.schema.json` + `examples/` | Go 结构体、Python Pydantic 模型 |
| 数据库 | `backend/db/migrations/` | `backend/gen/db`（sqlc） |

1. 修改顺序：改契约 → 生成两端代码 → 改实现 → 契约测试。禁止手改生成物，禁止绕过契约补写接口。
2. CI 重新生成并比对，生成物与提交不一致则失败。
3. 不兼容变更升级版本；Activity 旧版本在仍有运行中的工作流时保留。

## 8. 配置、数据与安全

- 配置只来自环境变量，启动时集中校验；`.env.example` 只记录占位值与说明，真实 `.env` 与凭据不入库。
- 媒体只通过对象存储预签名地址上传与访问；数据库保存 key、hash、元数据。
- 测试数据必须合成或脱敏，不提交真实剧本或用户媒体。
- 构建产物、缓存、日志、本地数据库不进入仓库。

## 9. 质量门禁与交付

| 端 | 必须通过 |
| --- | --- |
| Go | `gofmt`、`goimports`、`go vet`、`golangci-lint`、`go test -race ./...`、`govulncheck` |
| Python | ruff check、ruff format --check、mypy、pytest |
| 前端 | lint、format:check、typecheck、test、build；交互变化补 Playwright 用例 |
| 契约 | 生成一致性检查；Activity 样例在两端通过 |

1. 核心业务逻辑按 Red → Green → Refactor；外部系统交互按风险补集成测试。
2. 每个里程碑的验收在 `docs/acceptance/` 记录真实操作、命令与结果；静态检查通过不等于功能验收通过。
3. 提交与分支规则见 `AGENTS.md`。

## 10. 规范维护

新增仓库单元、第二套存储、新的基础设施或工作流 Owner 之前，必须先在设计文档中说明理由、替代方案与迁移方式，评审接受后再更新本文。普通实现细节不上升为全局规则。
