# Lanverse 工程规范

本文规定仓库结构、技术栈、编码与契约生成方式。业务目标、模块合同和未决任务分别见 `workspace/content/` 与 [BACKLOG](BACKLOG.md)。

## 1. 文件职责

| 文件                | 唯一职责                                                                                                        |
| ------------------- | --------------------------------------------------------------------------------------------------------------- |
| `AGENTS.md`         | 协作、授权、代码质量与 Git 规则                                                                                 |
| `PROJECT.md`        | 仓库结构、技术栈、目录与编码约定、质量门禁（本文件）                                                            |
| `DESIGN.md`         | 视觉与交互规范                                                                                                  |
| `README.md`         | 项目简介、启动方式、文档入口                                                                                    |
| `BACKLOG.md`        | 任务与未决范围清单（任务、实现要点、涉及文件、状态）                                                            |
| `workspace/content/prd/`         | PRD：产品目标、场景、需求与版本规划（编号规则见 `workspace/content/index.md`）                                              |
| `workspace/content/requirement/` | REQ：功能需求总表、非功能需求、业务流程与用例、术语、界面；06 起为功能需求文件（一个功能一个文件）              |
| `workspace/content/design/`      | DES：01–08 架构、数据、接口、工作流、Agent、画布、安全设计与技术选型；09 起为功能设计（与功能需求文件一一对应） |
| `workspace/content/plan/`        | PLN：实施路线、项目管理与变更                                                                                   |
| `workspace/content/test/`        | TST：测试策略、需求追踪矩阵、AI 评测                                                                            |
| `workspace/content/operation/`   | OPS：环境部署、CI/CD、监控告警、备份恢复与故障响应                                                              |
| `workspace/content/KNOWLEDGE.md` | 知识库本地阅读与 Obsidian 编辑说明 |
| `workspace/` | Nextra 本地文档站实现，正文在 content/ |
| `.obsidian/` | 使用仓库根 Vault 时的共享设置；不要求用户切换当前 Vault |

业务范围以 `workspace/content/prd/` 与 `workspace/content/requirement/` 为准，架构决策以 `workspace/content/design/` 为准；本文把其中的工程约定落到目录和工具上。两者冲突时，先修改需求或设计文档并评审，再同步本文。

## 2. 仓库结构与职责

```text
Lanverse/
  .env.example              本机进程配置样例；实际 .env 不入库
  docker-compose.yml        默认本机热更新应用；前端、Go 全部角色与 workspace 文档站
  docker-compose-env.yml    依赖环境（PostgreSQL、Redis、Kafka、MinIO、Temporal）
  docker-compose-prod.yml   生产应用配置；保留独立 Go 角色与维护入口
  backend/          Go：API（Gin）、领域模块、Temporal 工作流与 Worker、Outbox relay 与 Kafka 消费者、媒体处理
  frontend/         Next.js 工作台；未决能力见 BACKLOG
  workspace/        Nextra 本地文档站配置、页面入口与依赖
    content/        正式文档：产品需求、需求规格、设计、计划、测试、运维
  .obsidian/        使用仓库根 Vault 时的共享配置
```

不设 `contracts/`：公共 REST 契约由后端 Gin 注解与 DTO 经 swag 自动生成，在后端 Swagger 端点在线提供，再由 `@umijs/openapi` 生成前端 API（见 §7）；现有 Activity 与事件的输入输出由 Go 手写类型，不引入单独的 schema 文件与代码生成流水线。Activity 载荷、名称与队列遵循 DES-03，执行端变更须验证历史兼容性。

| 单元        | 必须负责                                                                                        | 不得负责                                                           |
| ----------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------ |
| `backend/`  | 全部业务事实、权限、计费、公共 API、全部工作流定义、写库与媒体 Activity、Outbox 与事件消费、SSE | 执行 Worker 与费用门禁就绪前调用模型供应商；在 Workflow 内执行 I/O |
| `frontend/` | 界面、交互、服务端状态缓存、编辑器局部状态、按参数 schema 渲染表单                              | 持有供应商密钥；直连 Agent 服务、Temporal、Kafka、Redis            |

**调用路径：**

```text
浏览器 → Next.js → backend-api(Gin) ─┬─ 命令 / 查询 → PostgreSQL（业务表 + Outbox）
                                     ├─ 启动 / 信号 → Temporal ─┬─ flow  队列 → backend-worker
                                     │                          ├─ media 队列 → backend-worker（FFmpeg → MinIO）
                                     │                          └─ agent / agent.mock 队列 → 执行端计划接入（DES-05/生成执行设计）
                                     └─ SSE（公开路由计划接入）← Redis Pub/Sub ← backend-relay ← Kafka ← Outbox
上传：浏览器 multipart → Next.js（生成 API 转发）→ Go → 私有对象存储
下载：浏览器 → 授权预签名 URL → 私有对象存储
```

## 3. 技术栈

| 范围     | 技术                                                                                                                                                        |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 前端     | Next.js（App Router）、React、TypeScript strict、pnpm、Tailwind CSS、shadcn/ui、Radix UI、lucide-react、ESLint、Prettier                                    |
| 前端组件 | TanStack Query、TanStack Virtual、Zustand + Immer、React Hook Form + Zod、Tiptap、Three.js、React Three Fiber / Drei、next-themes、`@umijs/openapi` + Axios |
| 后端     | Go、Gin、GORM（pgx 驱动）、Viper、Zap、Wire、swag + gin-swagger、go-playground/validator                                                                    |
| 后端集成 | Temporal Go SDK、go-redis v9（redis_rate、redsync）、franz-go、minio-go v7、OpenTelemetry Go                                                                |
| 工作流   | Temporal（自建，PostgreSQL 持久化）                                                                                                                         |
| 中间件   | PostgreSQL、Redis、Kafka（KRaft）、对象存储（开发 MinIO，生产火山引擎 TOS，均为 S3 协议）                                                                   |
| 媒体     | FFmpeg / ffprobe                                                                                                                                            |
| 可观测   | OpenTelemetry Collector、Prometheus、Grafana、Loki、Tempo / Jaeger、Temporal UI、Kafka UI                                                                   |
| 部署     | Docker、Docker Compose；规模化后 Kubernetes                                                                                                                 |

每个中间件的职责边界见 [DES-08 §6](workspace/content/design/08-技术选型决策.md#6-中间件职责)；工作台职责见 [工作台设计](workspace/content/design/工作台设计.md)。暂不引入：Elasticsearch、独立向量库、图数据库、服务网格、微服务拆分。

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
  db/schema.sql                  # 当前已实现数据库结构的唯一事实源，最终态 DDL
  docs/                          # swag 从注解生成的 Swagger 规范，禁止手改；由 backend-api 在线提供
```

1. 依赖方向 `adapter → application → domain`；`domain` 不依赖 Gin、GORM、Temporal、Kafka、Redis 或任何 SDK。
2. 上下文之间只调用对方 `application` 暴露的接口，不读写对方的表。
3. Gin Handler 只做参数绑定与校验、调用用例、映射响应；用例不接收 `*gin.Context`，而是 `context.Context`。
4. GORM 模型只在 `adapter/postgres`；领域对象、GORM 模型、API DTO 显式转换。生产禁止 `AutoMigrate`，当前已实现结构只由 `db/schema.sql` 定义；不追加 up/down 文件或维护第二份 DDL。
5. 一个用例一个事务；需要异步的后续动作写 Outbox，由 relay 投递到 Kafka。
6. 所有写操作经命令层：权限、幂等键、`expected_version`（不匹配返回 409）、审计。
7. 所有业务表带 `org_id` / `project_id`，仓储查询强制带项目条件。
8. **工作流只写结果与状态字段**，不覆盖人工配置（DES-01 原则 P3）。
9. 工作流：只用 Go 编写，代码确定性、无 I/O；Workflow ID 用业务 ID；Activity 先查已有结果再执行；代码变更使用 Temporal 版本化机制。
10. Operation 状态机、`unknown` 对账、预留与结算遵循 [DES-01 §4、§6.2](workspace/content/design/01-系统架构设计.md#4-生成操作operation模型)。
11. Kafka：主题 `lanverse.<上下文>.<事件>.v<N>`，键为 `project_id`；消费者按事件 ID 去重。
12. Redis：只放可重建的数据（会话、缓存、限流、锁、Pub/Sub）；键名 `lanverse:<用途>:<标识>`，设置过期时间。
13. 对象存储（开发 MinIO / 生产 TOS）：只通过 S3 协议访问，不使用厂商私有 API；桶私有；对象键 `projects/{project_id}/{类别}/{id}`；浏览器只通过预签名 URL 访问。
14. 依赖由 Wire 在组合根注入（`wire.go` 声明 Provider Set，`wire_gen.go` 为生成物，禁止手改，CI 校验生成一致）；接口由消费方按需定义；不使用 `Ixxx` / `Impl`，不建 `utils`、`common`。
15. `context.Context` 沿调用链传递；错误用 `%w` 包装并保留可判定的错误链；goroutine 必须有所有者、取消与等待。
16. 日志统一用 Zap（不混用标准库 `log` / `slog`）的结构化字段（`trace_id`、`project_id`、`operation_id`），不记录凭据与剧本全文。
17. 文件名 `snake_case.go`；Go 测试集中放在 `backend/tests/<模块>/`，按被测模块分目录并使用外部测试包（`<package>_test`）；生产源码目录不存放 `*_test.go`。集成测试放在对应模块目录中并使用 testcontainers 启动 PostgreSQL、Redis、Kafka、MinIO，工作流用 Temporal testsuite 与回放测试。

## 5. AI 执行能力

供应商执行由 Go adapter 承接，复用 Operation、持久发送权、私有分阶段回执、媒体接管与账本；具体状态与失败合同见 [生成执行设计](workspace/content/design/生成执行设计.md)。Codex app-server 协议适配、私有回执恢复及管理凭据测试入口已接入，真实执行注册仍默认关闭；实际供应商调用、费用、审核、Skill/Harness 与完整产品链尚未完成真实验收。`agent` / `agent.mock` / `agent.codex` 队列和 Activity 协议不能证明执行 Worker 已启用。

冻结输入、预算上限、只读工具白名单、长任务 heartbeat/取消，以及结果未知不自动重提付费请求的约束继续有效。执行 Worker 须按角色注入凭据；API、flow/media 与浏览器不得持有执行私钥。本机 Codex 账号由 Codex 客户端管理；实际能力、模型、产物、可信费用及审核须在启用前核验，不默认免费或回退付费 HTTP。任务与放行条件见 BACKLOG M1-12。

## 6. 前端

前端采用 Next.js App Router 与自身 Go API，页面存在不等于能力或真实生成验收完成。

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
      project/                   # 项目列表与设置、创建和复制对话框
      workbench/                 # 工作台页面组件、布局与路由映射
      ui/                        # shadcn/ui（Radix）基础组件
    lib/                         # 前端基础能力
      request.ts                 # 唯一 Axios 传输入口；含授权媒体流读取
    api/                         # @umijs/openapi 从后端在线 Swagger 统一生成，禁止手写
  tests/                         # unit、e2e
  eslint.config.mjs              # ESLint flat config
  .prettierrc.json               # Prettier 配置（含 tailwind 插件）
  components.json                # shadcn 配置（Radix 体系）
```

业务组件、查询、钩子、类型、状态与就近测试放在 `components/<业务>/`；共享基础能力在 `lib/`，生成 API 在 `src/api/`。不设置 `src/features/` 或重复业务目录；尚未实现的模块不预建目录。

1. 服务端事实只来自 TanStack Query；公开 SSE 路由计划接入；实施后的事件只用于让相关查询失效。
2. 编辑器高频交互状态放 Zustand（嵌套更新用 Immer）；持久化一律通过后端命令接口。
3. 基础控件、表单、弹窗、菜单、表格一律用 shadcn/ui；不混用其他组件体系（不引入 Ant Design）。
4. 富文本与实体引用用 Tiptap；`@角色 / @场景 / @道具` 保存为结构化引用，不只保存纯文本。
5. 超过 100 行的列表（镜头表、资产库、任务中心）使用 TanStack Virtual。
6. 对话式 Agent 计划按既有 Go 模块接入；界面与工具链按 [DES-39](workspace/content/design/39-对话式Agent.md) 和 BACKLOG E-37 设计与验收。
7. 模型参数表单只由 `param_schema` 驱动。
8. 画布：拖拽只在松手时提交命令；命令带 `expected_revision` 与幂等键；409 时基于最新文档重放。
9. 媒体：按缩放级别选择缩略图；视频默认封面，同时播放不超过 3 个；只渲染视口内节点。
10. 付费操作先展示报价并由用户确认。
11. 所有业务接口由 `@umijs/openapi` 统一生成到 `src/api/`，生成函数调用 `src/lib/request.ts` 的 Axios 封装，业务代码与 Route Handler 只调用生成函数。禁止手写 API 文件、自行拼接业务接口路径、直接调用底层 `request` 或网络客户端；接口缺失时先补后端注解与 DTO，再重新生成。ESLint 检查手写请求边界，CI 重生成整个目录并校验漂移。授权对象存储模型原件由 `request.ts` 的流传输入口读取，保留逐块容量上限、取消与禁止重定向，不携带后端凭据。上传代理通过生成函数转发原始 multipart 计数流，保持 Origin/幂等键和响应状态，不缓冲整份文件。请求服务使用私有 `axios.create()` 与 Axios TypeScript 配置，`@umijs/openapi` 仅生成客户端；不引入 `umi-request` 请求运行时。
12. 不设置项目脚本或 Makefile；前端开发和检查直接执行 `pnpm exec next`、`pnpm exec eslint`、`pnpm exec prettier`、`pnpm exec tsc`、`pnpm exec vitest`、`pnpm exec openapi2ts`。

### 6.1 本地知识库文档站

`workspace/` 使用 Nextra 原生内容约定和标准文档主题，提供目录导航、搜索、正文和页内目录；`workspace/content/index.md` 是首页，正文由 Obsidian 或编辑器直接编辑。计划与需求作为普通文档维护。content 外根文件及 `.txt` 许可只做本机只读原文链接适配，不建立管理页面。范围见[知识库设计](workspace/content/design/知识库体系设计.md)，使用说明见 [KNOWLEDGE](workspace/content/KNOWLEDGE.md)。

默认随应用 Docker 一起启动：`docker compose up -d --build --wait`。文档入口为 <http://127.0.0.1:3210>；只启动文档站可执行 `docker compose up -d --build --wait workspace`。`workspace` 服务与业务后端没有启动依赖，挂载正文、站点代码和原文白名单，使用 Webpack 轮询热更新。容器依赖和 `.next` 使用独立卷，根 `.env` 不挂载到文档站。

也可在仓库根执行宿主命令，不配置包 scripts：

```bash
pnpm --dir workspace install --frozen-lockfile
pnpm --dir workspace exec next dev --webpack --hostname 127.0.0.1 --port 3210
```

Nextra 4.6.1 的自定义 remark 链接适配不支持 Turbopack，开发和构建显式使用 `--webpack`。开发时正文实时更新，搜索只反映上次构建的索引。完整本地阅读先停止开发服务器，再依次执行：

```bash
pnpm --dir workspace exec next build --webpack
pnpm --dir workspace exec pagefind --site .next/server/app --output-path public/_pagefind
pnpm --dir workspace exec next start --hostname 127.0.0.1 --port 3210
```

两种方式均访问 <http://127.0.0.1:3210>，不可同时占用该端口。内容变化后重建正文与搜索索引。具体依赖由 workspace 清单与锁文件约束；本地阅读不需要业务后端、账户或部署。

知识库代码变更的质量门禁在仓库根执行：

```bash
pnpm --dir workspace exec eslint .
pnpm --dir workspace exec prettier --check .
pnpm --dir workspace exec next typegen
pnpm --dir workspace exec tsc --noEmit
pnpm --dir workspace exec tsx --test 'tests/*.test.ts'
pnpm --dir workspace exec next build --webpack
pnpm --dir workspace exec pagefind --site .next/server/app --output-path public/_pagefind
```

实际阅读验证检查文档入口、中文路径、分类导航、正文与表格、页内目录、搜索和保存刷新。命令通过与浏览器使用结果分别记录。

## 7. 契约与生成

接口与数据按下述唯一事实源维护，生成或构建通过不能替代真实业务验收。

| 契约      | 唯一来源                                                     | 生成物                                                                                                     |
| --------- | ------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------- |
| 公共 REST | Gin Handler 的 swag 注解 + DTO                               | `backend/docs`（生成的 Swagger 2.0）→ 后端在线 `/swagger/doc.json` → `@umijs/openapi` → `frontend/src/api` |
| Activity  | Go 类型；DES-03 §7.1 规定字段语义与兼容边界                  | —                                                                                                          |
| 数据库    | `backend/db/schema.sql`                                      | —                                                                                                          |
| 事件      | Go 结构体，按主题版本号 `.v<N>` 手写，生产者与消费者各自维护 | —                                                                                                          |

1. REST：改 Handler / DTO 与注解 → 运行 swag 自动生成 → 启动后端并从在线 `/swagger/doc.json` 运行 `@umijs/openapi` → 改前端实现。生成器配置 `schemaPath` 为在线地址、`requestImportStatement` 为 `@/lib/request` 的导入语句；Swagger 文档与前端 API 文件均禁止手改。后端未启动或在线规范不可用时生成失败，不以旧文件代替。
2. Activity 与事件：改 DES-03 表格 → 改 Go 类型 → 契约测试。Temporal 载荷由现有 Go 类型实现，兼容性样例内联在 `backend/tests/<模块>/` 的测试中，不再维护根目录 `contracts/` 或独立 JSON 规范文件；公共 HTTP 接口统一走上面的 Swagger 链路。M1-12 执行接线时还须验证历史输入输出与队列兼容性，不把未注册的内部 Activity 伪装成公共 HTTP 端点。
3. 不兼容变更升级版本号（Activity 名称或事件主题后缀 `.v<N>`）；旧版本在仍有在途工作流或未消费事件时保留。
4. 数据库：直接更新 `backend/db/schema.sql` 的表、列、索引、约束、函数、触发器、权限与动态分区最终态，同步 owning 模块与结构合同测试。Schema 只描述已实现能力；概念规划留在 DES-02，不预建对象。新空业务库由独立表所有者在单事务内初始化，预置 `lanverse_app NOLOGIN NOSUPERUSER`；既有业务库依据实例与目标 Schema 审阅增量升级，不自动覆盖、重建或重跑初始化。

Swagger 本地生成入口为在 `backend/` 执行 `go generate -run 'swag' ./internal/app`；`public_api.go` 的 `go:generate` 固定 swag `v1.16.6`，从 Handler 注解和 DTO 生成 `docs/docs.go`、`docs/swagger.json`、`docs/swagger.yaml`。API 将生成规范与内置 Swagger UI 静态资源统一提供在 `/swagger/*any`，页面入口 `/swagger/index.html` 读取相对地址 `doc.json`，不依赖外部 CDN 或独立维护的接口清单。生成后须重新构建并重启 API，CI 继续检查重生成文件、在线规范和公开路由一致性。

Wire 组合根修改后，在 `backend/` 直接执行 `wire ./internal/app`，再执行 `goimports -local github.com/StephenQiu30/lanverse/backend -w internal/app/wire_gen.go`；`wire_gen.go` 仅由工具生成和格式化，CI 重复这两条命令并比较文件，不设脚本或 Makefile。

## 8. 配置、数据与安全

- 配置来自环境变量（Go 使用 Viper），启动时集中校验；`.env.example` 只记录占位值与说明，真实 `.env` 与凭据不入库。
- 供应商凭据仅供服务端使用，现有封装保留，解封与执行端计划按生成执行设计接入；MinIO、Kafka、Redis、Temporal 凭据只注入需要的单元。
- 测试数据必须合成或脱敏。构建产物、缓存、日志、本地数据卷不进入仓库。

## 9. 质量门禁与交付

| 端   | 必须通过                                                                                                                                                                         |
| ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go   | `gofmt`、`goimports`、`go vet`、`golangci-lint`、`go test -race ./...`、`govulncheck`、swag 与 Wire 生成一致性                                                                   |
| 前端 | `pnpm exec eslint .`、`pnpm exec prettier --check .`、`pnpm exec next typegen` + `pnpm exec tsc --noEmit`、`pnpm exec vitest run`、`pnpm exec next build`；交互变化补 Playwright |
| 契约 | swag 生成物与后端在线 Swagger 一致；`@umijs/openapi` 从在线文档重生的 API 一致；公开路由文档覆盖；现有 Go Activity 样例通过，真实执行链路待 M1-12                                |

1. 核心业务逻辑（Operation 状态机、预算与对账、依赖传播、工作流）先写测试再实现。
2. 按 TST-01/02/03 执行验收并在对应任务保留未通过条件；静态检查通过不等于功能验收通过。
3. 提交与分支规则见 `AGENTS.md`。
4. Go 与前端代码审查分别核对 Google Go 规范和 Vercel 最佳实践；性能修改说明所采用规则、测量结果与适用边界。规范审查与自动化门禁分别记录，跳过项明确说明。

## 10. 规范维护

新增仓库单元、存储、中间件或工作流 Owner 之前，必须先在设计文档中说明理由、替代方案与迁移方式，评审接受后再更新本文。
