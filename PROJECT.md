# Lanverse 项目工程规范

> 本文件规定项目应当如何组织，不是当前实现清单，也不代表现有代码已符合规范。
> 2026-09-21：已接受的目标规范。根据用户确认的 Go、Python、Next.js 方向、pnpm 与 shadcn/ui + Radix UI 要求制定；以官方资料及 Vercel、shadcn 工程指南为参考，不以当前代码、目录或依赖清单证明选型正确。
> 已授权按规范审计差异并分批整改；本文中的路径和命令是目标约定，不表示已落地。

## 1. 适用范围与文档职责

本文件是 Lanverse 跨仓库、跨语言的工程规范入口，负责仓库职责、技术栈、依赖方向、文件归属和交付约束。本文中的“仓库”指职责单元：在同一 Git 仓库中以 `backend/`、`agent/`、`frontend/` 承载，拆成独立 Git 仓库后仍须遵循相同边界。仓库职责与部署进程不必一一对应，不为组织形式预建独立服务。

| 文件 | 唯一职责 |
| --- | --- |
| `PROJECT.md` | 跨仓库工程规范；回答功能属于哪里、采用什么技术、文件放在哪里 |
| `AGENTS.md` | 长期稳定的协作、操作授权、代码质量和 Git 规则 |
| `README.md` | 项目说明、启动入口、开发命令和文档导航，不复制整份工程规范 |
| `DESIGN.md` | 视觉与交互设计规范，不承担服务端架构定义 |
| `docs/design/` | 具体功能或架构决策，记录范围、边界、契约、状态和失败路径 |
| `docs/requirement/`、`docs/plan/` | 已接受设计对应的需求与实施安排，按复杂度选用 |
| `docs/acceptance/` | 验证条件、真实执行结果、未通过项与证据 |

工程边界以本文件为总入口，具体业务语义以对应的已接受 Design 为准。本轮明确更新前端为 pnpm、shadcn/ui + Radix UI、ESLint、Prettier；旧文档中的 npm 默认值和现状描述不构成保留理由。发现冲突时须明确冲突条款，先更新设计及本文件，再实施；不能用现有代码、旧验收记录或局部 README 默默覆盖规范。历史文档保留追溯价值，已被明确替代的约束不再作为新开发依据。

## 2. 仓库职责与所有权

| 职责单元 | 必须负责 | 不得负责 |
| --- | --- | --- |
| 根目录 | 项目规范、跨仓库设计、集成验收、应用部署编排与 CI 入口 | 业务实现、重复的语言依赖清单、生产数据 |
| `backend/`：Go 业务后端 | 用户与权限、正式业务事实、审批与采纳、业务事务、公共 API、媒体任务与供应商发送授权、费用与额度、业务事件 | 提示词驱动的专业推理、前端视图状态、直接信任模型输出 |
| `agent/`：Python AI 执行 | 创作流程编排、执行记录和草案、Harness、Skill、模型调用适配、候选生成及有界校验修复 | 写入 Go 正式业务表、替代权限或财务裁决、自行将候选发布为正式成果 |
| `frontend/`：Next.js Web 应用 | 页面路由、交互、展示、类型化业务请求、服务端状态缓存、临时编辑状态 | 业务授权裁决、直连数据库或内部 Agent、持有供应商密钥、把本地状态当作正式事实 |

### 2.1 正式事实与执行事实

- **Go 是正式业务事实的唯一写入方。** 角色、剧集、分镜等正式内容和审批、采纳、费用等状态由 Go 对应业务模块决定并持久化。
- **Python 可以持有自己的执行事实与草案。** 运行、尝试、候选、检查点等属于 Python 执行边界；不得与 Go 共用可写业务表或绕过 Go 的采纳用例。
- **Python 内部区分可信编排与受限推理。** 可信编排可按设计访问自己的执行存储和 Temporal；Harness/Skill 不直接获得数据库、业务写接口或无限制工具权限。不得把整个 Python 仓库误判为无状态服务，也不得把可信编排权限授予模型。
- **前端只有交互状态的所有权。** 本地编辑、选择、展开和视口状态可留在浏览器；需要持久化的布局通过 Go 的相应业务契约保存。
- 单个工作流的状态机只能有一个明确 Owner。新创作编排归 Python，Go 保留正式业务命令及已接受的业务工作流；不得在两端分别维护同一任务的推进逻辑。

### 2.2 标准调用路径

```text
浏览器 → Next.js 页面 → Go 公共 API
                         ├─ 查询与业务命令 → Go 业务模块 → 正式存储
                         └─ 创作请求 → Python 可信编排 → 受限 Harness / Skill
                                           └─ 执行记录与候选草案
候选提交 → Go 权限、版本与业务校验 → 按设计审批 / 采纳 → 正式事实
```

浏览器不能直连 Agent、Temporal、Kafka、Elasticsearch 或供应商。对象上传下载只允许使用 Go 授权的短期对象地址。Next.js 服务端可承担必要的页面渲染和请求转发，但不能成为第二套业务后端。

## 3. 技术栈基线

以下规定技术选择与使用边界，不声明任何版本为“当前最新”。精确版本由各仓库的依赖声明、锁文件及构建配置固定；升级必须验证兼容性，不随文档编辑自动升级。

| 范围 | 标准技术 | 使用约束 |
| --- | --- | --- |
| 业务后端 | Go、标准库 `net/http`、GORM、PostgreSQL | HTTP 采用标准库；模块化整洁架构隔离业务与框架；一个 Go Module |
| Go 工具链 | Go Modules、gofmt、goimports、go vet、golangci-lint、govulncheck | 依赖与工具版本固定；标准库 testing / httptest；并发逻辑覆盖 Race Detector |
| AI 执行 | Python、FastAPI、Uvicorn、Pydantic、pydantic-settings、HTTPX | 类型化契约；配置集中校验；HTTP 客户端统一超时、连接池与生命周期 |
| Python 工具链 | uv、Ruff、mypy、pytest、pytest-asyncio | uv 管理环境和锁文件；Ruff 负责 Lint / 格式；mypy 负责类型；pytest 负责行为 |
| Web | TypeScript（strict）、React、Next.js App Router、Node.js | 页面默认 Server Component；交互局部客户端化；选用受支持且相互兼容的运行版本 |
| 前端包管理 | pnpm | `packageManager` 固定精确版本；提交唯一 `pnpm-lock.yaml`；CI 冻结锁文件安装 |
| UI | Tailwind CSS、shadcn/ui、Radix UI | 选择 Radix 组件体系；组件优先、语义 Token、统一 variants；不混用 Base UI 版本 |
| 表单 | shadcn/ui Field 组件组合、React Hook Form、Zod | 使用官方表单方案；按需要管理校验和提交状态，不建设通用表单引擎 |
| 前端质量 | ESLint、eslint-config-next、typescript-eslint、Prettier、eslint-config-prettier | ESLint 检查代码及框架规则；Prettier 统一格式；关闭相互冲突的格式规则 |
| 前端验证 | Vitest、React Testing Library、Playwright | 分别验证纯逻辑 / 组件行为与真实浏览器流程 |
| 浏览器服务端状态 | Redux Toolkit Query（RTK Query） | 用于创作工作台共享查询、任务状态与 mutation 失效；服务端渲染不强制通过浏览器 Store |
| 接口 | REST、注解生成 Swagger、`@umijs/openapi`；按需使用 SSE | Go 注解与 DTO 自动生成在线 Swagger 文档；前端从在线地址生成请求函数和类型；内部接口独立定义 |
| 长任务 | Temporal | 承担持久编排、等待、重试与恢复；不替代业务数据库 |
| 业务事件 | Outbox、Kafka | 传递已提交事实；不承担同步 RPC 或模型 Token 流 |
| 文件与媒体 | S3 兼容对象存储 | 数据库保存身份、归属和元数据；二进制对象不进入关系表或 Git |
| 搜索与日志 | Elasticsearch；结构化日志接入环境日志链路 | 搜索索引可重建；不成为正式事实源；不记录凭据和未脱敏内容 |
| 应用运行 | Docker、Docker Compose | 应用镜像按职责构建；外部基础设施由环境配置提供 |

选型理由是职责与需求：Go 使用标准库 HTTP 减少框架耦合，Python 用 FastAPI/Pydantic 表达内部契约，uv 与 pnpm 保证依赖可复现，RTK Query 管理交互密集工作台的共享查询状态。这些是项目设计选择，不声称是官方唯一推荐架构。无需浏览器共享缓存的服务端页面不为使用 RTK Query 额外客户端化。

具体版本在实施前按官方兼容要求确定，并在声明文件、锁文件、镜像与 CI 中一致固定；不得用浮动 `latest` 构建生产镜像。Vercel 指南用于工程规范，不等于要求迁移到 Vercel 托管。未出现实际消费者的技术不安装，未发生实际职责的目录不创建。

## 4. Go 后端文件规范

规范路径相对于 `backend/`。以下是职责映射，不要求补齐空目录。

```text
backend/
  cmd/lanverse/main.go            # 可执行程序入口，仅配置与启动
  internal/
    bootstrap/                   # 组合根，装配具体实现和生命周期
    <业务模块>/
      domain/                    # 领域模型、规则与领域错误
      application/               # 用例、事务边界、消费方接口
      adapter/                   # HTTP、数据库、消息、外部系统适配
  api/openapi/                   # 注解生成的 Swagger 产物，禁止手工维护
  tests/                         # 跨模块、契约、集成验证
  go.mod
  go.sum
  .golangci.yml                  # 统一静态检查配置
  Makefile                       # 格式、检查、测试、构建和 Swagger 生成入口
  Dockerfile
  README.md
```

1. 依赖方向为 `adapter → application → domain`；外层可引用内层，内层不得引用外层。`bootstrap` 是装配具体实现的组合根。
2. 模块按业务职责命名。跨模块通过明确的应用接口或已提交事件协作，不读取另一模块的数据库表，不导入其适配器充当业务接口。
3. 接口由消费方按用例需要定义；构造函数显式注入依赖。不得创建全局 Controller/Service/DAO 分层、`Ixxx`/`Impl`、模糊的 `utils`/`common` 包或仅转发调用的空层。
4. HTTP DTO、领域对象、持久化模型各守边界，使用显式转换。GORM 模型不直接充当公共 API 响应或领域模型。
5. GORM 持久化模型属于各业务模块的数据库 Adapter；组合根统一注册，不设跨业务万能 Model Catalog。数据库变更须有受版本控制的升级步骤、兼容策略与恢复方案；迁移是 Schema 演进记录，禁止同时维护另一套业务 ORM 模型。不能以生产启动时自动同步代替数据升级设计。
6. Go 源文件使用语义明确的 `snake_case.go`；单元测试就近放在 `*_test.go`，跨边界测试放入 `tests/`。按职责拆文件，不按固定行数机械拆分。
7. `context.Context` 沿调用链传递；错误保留可判定错误链；goroutine 必须有所有者、取消和等待策略。日志采用标准库 `log/slog` 的结构化字段。

### 4.1 Go 工程约束

- `net/http` Handler 与 Swagger 注解放在业务模块的 HTTP Adapter；鉴权、参数解析、响应映射停留在边界，业务用例不接收 `http.Request`。
- `domain` 不依赖 GORM、HTTP、Temporal 或供应商 SDK；Application 持有消费方接口与事务要求，Adapter 提供实现。接口只在替换、测试或跨边界需要时创建，不为每个结构体机械配接口。
- 数据库事务、HTTP 请求、消息处理必须有超时或取消边界。持久长任务交给有恢复能力的执行器，不用请求结束后无人管理的 goroutine 承担业务承诺。
- `Makefile` 定义 `fmt`、`lint`、`test`、`test-race`、`vuln`、`swagger`、`build` 的统一入口；工具版本在仓库中固定。格式化工具属于 Go 工具链，不使用 Prettier 格式化 Go。
- 遵循 Go 官方惯用法、Google Go Style Guide 和 Uber Go Style Guide；原则冲突以语言正确性、本项目明确边界及简单可读为先。

## 5. Python Agent 文件规范

Agent 采用一个服务、一个镜像、一个默认应用进程和一个内部 HTTP 入口，内部承载 FastAPI、Temporal Worker 与 Harness。沿用标准目录名称，按职责规范文件；不另建一套 providers / contracts / workflow_harness 目录。

完整流程、契约、状态、失败恢复与验收见 [Agent 服务目录与 Harness 工作流设计](docs/design/Agent服务目录与Harness工作流设计.md)。本节负责工程入口，详细设计负责执行语义；两者是已接受的目标规范，不代表实现已符合。

规范路径相对于 `agent/`：

```text
agent/
  app/
    main.py                      # FastAPI 应用工厂
    core/                        # 配置、依赖装配、生命周期
    api/
      dependencies.py            # HTTP 依赖提供者
      router.py                  # 路由汇总
      routes/                    # 按内部资源划分的 HTTP 路由
    creation/                    # Workflow Harness：可信编排与执行保障
      workflow.py                # 确定性阶段推进与等待
      activities.py              # Activity 注册与 I/O 用例调用
      execution.py               # 领取、预算、Harness 调用、结果提交
      repository.py              # 执行存储与条件写入
      dispatcher.py              # 持久命令和通知的可靠交接
      recovery.py                # 未知结果对账与受控恢复
      worker.py                  # Worker 注册与运行配置
    harness/                     # 有界执行、预算、取消和修复控制
    modules/<能力>/              # 专业能力、输入与候选 Schema
    reasoning/                   # 模型请求或推理子进程 Adapter
    protocol/                    # 跨语言编码、摘要等协议机制
    text_contract/               # 共享文本任务、来源与结果合同
    skills/                      # Skill 注册、校验和只读加载
  skills/<能力>/                 # 发布资源：SKILL.md 与 references/
  tests/                         # 单元、契约、架构与集成测试
  pyproject.toml                 # 依赖声明、工具配置与开发依赖组
  uv.lock                        # 唯一解析锁文件
  .python-version                # 开发 / 构建运行版本
  Dockerfile
  README.md
```

1. `main.py` 只创建应用；路由只解析输入、取得依赖、调用用例和映射响应。数据库连接、Temporal 客户端和 Worker 不能在路由中创建。
2. `core/container.py` 装配客户端、Worker、Dispatcher 和 SkillRuntime，`core/lifespan.py` 统一管理启动、就绪、关闭和失败清理；禁止导入模块时启动任务。服务内默认显式接口调用，不为模块职责拆服务或绕自身 HTTP 调用。
3. 执行存储只承载 Python 自有事实。契约模块保持纯数据与校验，不通过共享契约反向导入执行、数据库或模型适配器。
4. Skill 资源与加载代码分开；注册明确的能力、版本和内容摘要。不把用户输入当作可执行 Skill，不动态扫描任意用户目录或加载任意代码。
5. 模型输出只能作为待校验候选；必须定义输入版本、调用标识、预算、截止时间和失败结果。未知执行结果须先对账，不能盲目重试产生重复费用或副作用。
6. 文件及模块使用 `snake_case`，测试使用 `test_*.py`。禁止全局 `services/`、`repositories/` 堆积不同业务；按能力归属持有自己的应用逻辑与适配器。
7. `pyproject.toml` 声明依赖，`uv.lock` 锁定解析结果；使用 `uv sync --locked` 安装、`uv run` 执行工具。不得以手工维护的 requirements 文件形成第二套依赖来源；下游确需该格式时由 uv 导出。
8. FastAPI 使用 `APIRouter`、`Depends` 和单一 `lifespan` 管理 HTTP 组合及资源；Pydantic 声明边界 Schema，领域逻辑不依赖 Request 或 HTTPException。内部 OpenAPI 从路由和类型自动导出，不与 Go 的公共 Swagger 混成一个接口源。
9. 使用 HTTPX 的受管理客户端；异步路径不执行阻塞 I/O，CPU 密集工作移交有界执行器。重试只针对可重试且满足幂等条件的操作，所有调用设置预算与 deadline。
10. Ruff 的检查和格式化、mypy 类型检查及 pytest 配置统一写在 `pyproject.toml`。公开边界与应用接口必须有类型；不得以全局忽略规则或泛化 `Any` 绕开检查。配置只由 pydantic-settings 入口解析，日志统一结构化且脱敏。

### 5.1 Workflow 与 Harness 运行规范

- **Workflow Harness** 是 `creation/` 内 Workflow、Activity、执行存储与恢复机制的组合职责，不新增同名框架。Workflow 只负责阶段、分支、等待与取消；数据库、模型、文件和网络 I/O 全部在 Activity 中完成。
- **Agent Harness** 只负责一次冻结任务的上下文、Skill、受限推理、确定性检查和有限修复。长期等待归 Workflow，预算与结果持久化归可信执行用例，正式采纳归 Go。
- **Skill** 通过固定发布版本与摘要加载，正文不授予工具或业务写权限。专业能力的输入和候选放在 `modules/<能力>/`，共享文本语义放在 `text_contract/`，编码机制放在 `protocol/`。
- 任务受理须持久化并幂等启动；输出与就绪通知可靠提交。Activity 重投先查已有结果，未知外部结果先对账，旧 Worker 不能覆盖新结果；暂停、恢复和进程重启不重置预算。
- 人工审批由 Go 持久化决定并完成正式写入，Agent 只有收到有效的完整采纳回执才解锁依赖步骤。候选生成成功、质量检查通过、人工批准与正式采纳分别表达。
- Workflow / Activity 分文件，普通逻辑不机械升级为子 Workflow；先验收一条解析、候选、采纳及重启恢复链路，再扩展能力。

## 6. Next.js 前端文件规范

规范路径相对于 `frontend/`。

```text
frontend/
  src/
    app/                         # 路由、layout、页面装配与路由级反馈
    features/<业务>/             # 业务视图、交互、endpoint、派生视图模型
    components/                  # 被多个业务使用的组件
      ui/                        # 统一 UI 基础组件
    hooks/                       # 无特定业务归属的共享 Hook
    api/                         # umi-openapi 自动生成的请求函数与类型
    lib/                         # 职责明确的应用基础设施
      server-state.ts            # RTK Query 统一缓存入口
      request.ts                 # 手写统一请求适配，供生成代码调用
  public/                        # 随应用发布的静态资源
  tests/                         # unit、architecture、e2e 等验证
  openapi2ts.config.ts            # 在线 Swagger 地址与 API 生成配置
  package.json
  pnpm-lock.yaml
  components.json                # shadcn 注册源、别名、样式与 Radix 组件配置
  eslint.config.mjs              # ESLint flat config
  .prettierrc.json                # 唯一格式配置
  .prettierignore                 # 排除构建产物等不可维护文件
  tsconfig.json                  # strict 类型检查
  Dockerfile
  README.md
```

1. 路由负责装配，业务代码进入 `features/<业务>/`。共享组件不得反向导入业务模块，跨 Feature 不得依赖对方内部实现。
2. 浏览器交互请求路径为 `页面 → Feature → RTK Query endpoint → umi-openapi 生成的 API 函数 → 统一 request → Go`。Server Component 按请求调用生成的 API 函数读取 Go 数据，不创建全局用户 Store。两条路径均不重复维护 URL、请求参数类型及请求函数。
3. 使用 `@umijs/openapi` 从后端在线 Swagger JSON 地址生成完整 API 请求函数及 TypeScript 类型，统一输出到 `src/api/`，不只生成类型声明。该目录仅放生成产物，禁止手工修改；统一请求适配放在 `src/lib/request.ts`，缓存与业务编排放在 Feature。
4. URL 管理可分享的页面范围；RTK Query 管理服务端事实；React 本地状态管理临时交互；展示模型从事实派生，不另存一份可写业务事实。
5. 普通组件和业务文件使用语义明确的 `kebab-case.ts(x)`；Next.js 约定文件及生成文件保留各自约定。业务专用组件或 Hook 先留在 Feature 内，有真实复用需求再提升。
6. `lib/` 只能容纳职责明确的基础设施，不成为业务逻辑或万能工具集合。静态资源进入 `public/`；用户上传文件由对象存储管理。
7. 遵循 `DESIGN.md` 的无边框内容组织、键盘可访问性及状态反馈规范；加载、空数据、失败、禁用和完成状态必须可辨认。

### 6.1 shadcn/ui + Radix UI 组件优先

组件选择顺序固定为：**已有 shadcn/ui 组件 → 官方组件组合 → Radix Primitive 的集中封装 → 有明确缺口的自定义组件**。这里的“已有”仅用于实施时避免重复安装，不是以现有实现决定规范。基础交互控件统一进入 `components/ui/`，业务页面不得各自实现一套。

| 需求 | 标准组件 |
| --- | --- |
| 操作与链接式操作 | Button；导航使用 Next.js Link，必要时通过 Radix `asChild` 组合 |
| 表单 | shadcn/ui 官方表单方案：FieldGroup、Field、FieldLabel、FieldDescription、FieldError 与标准控件，按需结合 React Hook Form / Zod |
| 输入框内附加操作 | InputGroup、InputGroupInput / InputGroupTextarea、InputGroupAddon |
| 选项与分组 | ToggleGroup；相关复选 / 单选使用 FieldSet、FieldLegend |
| 弹窗与确认 | Dialog、Sheet、Drawer、AlertDialog；保留 Title、焦点管理及键盘交互 |
| 导航与菜单 | Sidebar、Tabs、Breadcrumb、DropdownMenu、NavigationMenu |
| 表格 | shadcn/ui Table、TableHeader、TableBody、TableRow、TableHead、TableCell；需要排序、筛选时再组合 Data Table 方案 |
| 数据与状态 | Badge、Avatar（含 Fallback）、Progress |
| 持续失败 / 临时通知 | Alert / Sonner；表单错误紧邻字段，不全部改成 toast |
| 空态 / 加载态 | Empty / Skeleton、Spinner |
| 滚动与分隔 | ScrollArea、Separator；仅在设计需要分隔时使用 |

**表单、表格同样优先用标准组件。** 业务页面不得自行用 `input`、`button`、`table`、`tr`、`td` 等拼装一套替代组件，也不手写弹窗、下拉菜单和反馈控件。表单按 shadcn/ui 官方方案组合，表格直接使用 Table 系列；不额外制造万能 Form / Table、Schema 渲染引擎或只转发 props 的包装层。

必要的 HTML 语义和布局容器正常保留。官方表单组合中的原生 `form` 仅承担提交语义，不作为手写整套表单的理由；基础组件内部使用原生标签也是正常实现。选择 Radix 体系不意味着每个 shadcn 组件都必须直接依赖 Radix Primitive。

- Radix 是本项目选定的 Primitive 体系，不因 CLI 默认值切换到 Base UI，也不把不同体系的 props 混用。Radix 组合遵循 `asChild`、属性透传及 ref 协作要求，避免嵌套 button / a。
- 表单通过 Field 组合表达结构；`data-invalid` 与控件 `aria-invalid` 对应，禁用状态同时反映在容器和控件。`SelectItem`、菜单条目和命令条目放在对应 Group，`TabsTrigger` 放在 `TabsList`。
- 优先使用组件的 `variant`、`size` 和语义 Token。业务层 `className` 负责布局；不逐页覆盖组件颜色、字体和交互状态。统一主题变量放在 `src/app/globals.css`，需要的新外观在共享组件中定义受控 variant。
- 间距使用 flex / grid 的 `gap-*`；条件类使用唯一 `cn()`；不手工覆盖浮层 z-index，不散落硬编码颜色与暗色覆盖。组件 API、图标位置和尺寸以所选发布版本的官方文档为准。
- 项目内容区保持无边框设计，通过留白、标题和对齐分组；在基础组件层统一处理 Card 等外观，不移除控件可辨认性和焦点反馈，不用阴影模拟内容边框。
- 只有标准组件不能覆盖实际需求时才自定义，并验证键盘、焦点和响应式行为；不为单次使用预建抽象层。
- 实施时使用 pnpm 执行 shadcn CLI：先查注册源和组件文档，再预览差异、安装及检查源码。所用 registry 和版本来源必须明确；不得整批覆盖本地组件。`components.json` 的别名、主题与实际落盘目录保持一致。

### 6.2 Next.js 与 React 工程规则

- 默认使用 Server Components；仅对事件、Hook 和浏览器 API 所在的最小交互边界添加 `"use client"`，不把整个应用根布局客户端化。跨边界参数符合 React 可序列化规则，服务端密钥不得进入客户端模块或 props。
- Server Components 通过 Go API 获取数据；Route Handler / Server Action 只在页面需求确实需要时承担薄适配，不能直连业务数据库、复制 Go 授权或建立第二套业务写入。
- 用户相关数据按请求隔离；浏览器缓存按登录身份及 workspace 隔离，登出清理。公共页面缓存与用户数据缓存分别设计，不使用进程级可变认证头或共享用户 Store。
- 独立请求并行发起；使用合适的 loading / Suspense 边界。重型编辑器和低频功能按需加载，不为所有页面引入大型依赖。
- 优先使用 Next.js Link、Image、font 能力；合理声明图片尺寸与响应式范围。业务计算保持纯函数，避免用 Effect 同步可直接派生的状态；不机械增加 memo。
- 使用 `error.tsx`、`not-found.tsx`、`loading.tsx` 表达路由反馈，业务反馈使用对应 UI 组件。浏览器验收覆盖键盘操作、移动布局、失败恢复和刷新后的真实数据。

### 6.3 pnpm、ESLint 与 Prettier

- pnpm 是前端唯一包管理器。`package.json#packageManager` 固定 pnpm 版本，Node.js 版本在构建和开发入口固定；CI 使用 `pnpm install --frozen-lockfile`。
- 规范落地后只保留 `pnpm-lock.yaml`，不混用 `package-lock.json`、`yarn.lock` 或 bun 锁文件；迁移时先验证依赖解析和构建，再移除旧锁文件。本次文档任务不删除任何锁文件。
- ESLint 使用 flat config，启用 Next.js、React Hooks、TypeScript 及适用的可访问性规则；Prettier 负责格式，`eslint-config-prettier` 关闭冲突规则。不得依赖 `next build` 代替独立 Lint。
- TypeScript 开启 `strict`；生成代码与手写代码均须通过类型检查。对生成代码的 Lint 例外限定路径和具体规则，不能全局屏蔽错误或手改生成文件。
- 统一脚本：`pnpm dev`、`pnpm build`、`pnpm start`、`pnpm lint`、`pnpm format:check`、`pnpm format`、`pnpm typecheck`、`pnpm test`、`pnpm test:e2e`、`pnpm openapi`。检查脚本不修改文件，格式化脚本与检查脚本分开。

## 7. 契约、配置与文件所有权

| 文件或数据 | Owner | 消费规则 |
| --- | --- | --- |
| HTTP 注解与请求 / 响应 DTO | Go Backend HTTP 适配层 | 公共接口的维护源；接口变更必须同步注解、DTO 与路由行为 |
| 在线 Swagger JSON 与生成文件 | Go Backend 自动生成 | 从注解与 DTO 生成并由后端在线提供；禁止手写另一份 Swagger / OpenAPI 作为事实源 |
| 前端 API 函数与类型 | `@umijs/openapi` 自动生成 | 读取在线 Swagger JSON，输出至 `frontend/src/api/`；禁止手写或手改生成接口 |
| 正式业务命令、事件与错误语义 | 对应 Go 业务模块 | 其他模块通过公开边界使用，不复制状态机 |
| AI 候选、调用与执行协议 | 对应能力的 Design 指定唯一 Owner | Go/Python 使用同一版本、样例与契约测试验证，不各自扩展同名字段 |
| Skill 与提示词资源 | Python 能力模块 | 版本化发布，变更需验证输入输出与行为边界 |
| 正式 Schema / 执行 Schema | 分别由 Go / Python 所有者管理 | 物理数据库可共用基础设施，Schema 和写权限必须隔离 |
| 依赖声明、锁文件、构建配置 | 对应语言仓库 | 锁文件入库；生成物标记来源；运行版本与构建、CI 保持一致 |
| `.env.example` | 对应运行配置 Owner | 只记录占位值与用途；真实 `.env`、凭据和本地数据不入库 |
| Compose、跨仓库 CI | 根目录 | 只装配实际需要的服务；环境专属基础设施凭据不复制到应用仓库 |
| 测试 fixture / testdata | 对应测试 Owner | 允许提交必要且脱敏的固定样例；不得混入用户原始数据或临时结果 |

跨仓库契约至少明确版本、身份与关联标识、权限范围、字段语义、错误、超时、重试和幂等规则。结果回写必须校验任务归属和输入版本；过期候选不得覆盖新的正式事实。事务提交与事件发布通过可靠交接衔接，消费者处理重复投递。

不增加无消费者的 `shared/`、SDK 仓库、通用微服务或复制协议的公共包。构建缓存、日志、运行数据库、上传文件及临时验收输出不进入源码目录和提交范围。

### 7.1 Swagger 与前端 API 自动生成链路

```text
Go HTTP Handler 注解 + 请求 / 响应 DTO
  → 后端文档生成命令
  → Swagger JSON 产物
  → 后端在线 Swagger JSON 地址
  → @umijs/openapi（schemaPath 使用该在线地址）
  → frontend/src/api/ 请求函数与 TypeScript 类型
  → Feature / RTK Query 调用
```

1. 后端采用 Go 注释注解方式描述接口，生成工具基线为 `swaggo/swag`。注解与 DTO 须完整表达方法、路径、参数位置、必填项、请求体、成功与失败响应、鉴权及业务分组；操作标识保持唯一稳定。
2. 后端生成的 Swagger 文件是产物，不能手工编辑。文档生成纳入开发命令和构建检查，运行服务提供对应版本的机器可读 JSON；Swagger UI 只是文档展示页，不能作为前端生成器的输入地址。
3. 前端使用独立开发依赖 `@umijs/openapi`，用于 Next.js 应用。`frontend/openapi2ts.config.ts` 的 `schemaPath` 从环境配置读取在线 Swagger JSON URL，输出位置通过 `serversPath` 等生成配置固定到 `src/api/`；缺少 URL 或拉取失败必须明确报错，不静默回退到本地旧文件。
4. 前端统一通过 `pnpm openapi` 执行生成。当前入口已落地；首轮生成产物暂存 `src/api/generated/`，旧调用方迁移完成后再统一类型命名空间，过渡状态详见验收记录。生成代码通过工具的请求导入配置调用 `src/lib/request.ts`，由该适配统一处理服务地址、认证和错误；生成时的文档地址与运行时的 API 地址分别配置。
5. 修改顺序为：更新后端接口、注解和 DTO → 重新生成文档 → 启动或部署对应版本后端 → 从在线 JSON 重新生成前端 API → 更新调用方 → 执行类型检查、构建与契约验证。禁止通过补写前端接口文件绕过生成链路。
6. CI 使用本次待验证后端构建启动的在线文档地址完成生成，并验证生成结果与提交产物一致；不能从无版本保证的共享环境生成后宣称契约一致。还须验证注解与实际路由、请求和响应一致，生成成功本身不证明运行行为正确。
7. Swagger 规范版本与两端生成器版本须固定并联合验证；生成器选型或版本变更不得悄悄改变字段、可空性、文件上传或错误响应语义。

工具配置依据：[swaggo/swag 官方说明](https://github.com/swaggo/swag)、[@umijs/openapi 官方说明](https://github.com/chenshuai2144/openapi2typescript)。

## 8. 开发与验收规则

1. 新任务先明确所属仓库、业务 Owner、输入输出、状态变化和验证方式。跨仓库功能先形成可审阅 Design，接受后再实施；小修改不堆叠文档。
2. 核心业务按 Red → Green → Refactor 推进。契约修改先固定样例与失败测试，再修改 Owner 和消费者；最后验证真实跨边界链路。
3. Go 按改动风险执行 `gofmt`、`goimports`、`go vet`、`golangci-lint`、Race Detector、`govulncheck` 及相应测试。
4. Python 执行 `uv sync --locked`、`uv run ruff check .`、`uv run ruff format --check .`、`uv run mypy app`、`uv run pytest`；前端执行 `pnpm lint`、`pnpm format:check`、`pnpm typecheck`、`pnpm test`、`pnpm build`，交互变化执行相应 Playwright 用例及浏览器验收。具体脚本在各仓库 README 和 CI 中统一维护。
5. 新增或调整业务边界必须验证依赖方向；外部系统交互按风险增加集成测试。静态检查通过不代表真实模型、真实媒体或端到端验收通过。
6. 交付记录实际命令和结果，区分通过、失败、跳过与阻断；说明工作区、提交与发布状态。现有代码不符合本文件时另列迁移范围，不借文档任务自动重构。

## 9. 规范维护

新增仓库、第二套状态库、ORM、协议、工作流 Owner 或基础设施之前，必须说明已有边界为何不足，并在 Design 中记录替代方案、成本、迁移和验收；接受后同步本文件。普通实现细节不升级为全局规则。

### 9.1 按规范推进整改

规范审阅完成后才以代码为审计对象，不能反过来按代码修改目标标准。每个差异记录“违反的条款、目标文件、处理方式、验收证据”；没有检查的实现不得预判为通过或失败。

| 顺序 | 整改内容 | 完成条件 |
| --- | --- | --- |
| 1. 工具与配置 | pnpm / uv 锁文件、三端版本与检查入口、ESLint / Prettier / Go / Python 配置 | 本地及 CI 使用同一工具链，干净安装和检查可复现 |
| 2. 接口生成 | Go 注解 → 在线 Swagger JSON → umi-openapi | 生成可重现、实际响应一致、前端类型检查通过 |
| 3. 前端组件 | 共享 shadcn/ui + Radix 基线、原生自绘控件替换、主题与页面组合 | 单条真实业务流程完成键盘、响应式、加载和失败验证 |
| 4. Go 与 Agent 边界 | 模块依赖、DTO、生命周期、事务及执行权限 | 对应业务和契约测试通过，必要集成链路验证完成 |
| 5. 集成验收 | 真实请求、执行、结果采纳与前端刷新 | 明确每端实际通过的证据和仍缺少的外部条件 |

按可验证的业务切片推进，不把全仓格式化、组件覆盖、框架迁移和业务重写合为一次改动。只有阶段验收完成，才能宣称该范围符合规范。已于 2026-09-21 开始首轮整改；实施范围、验证证据与未完成项见 [首轮改造验收](docs/acceptance/工程规范首轮改造验收.md)。目标规范不因过渡实现降低。

### 9.2 参考来源与适用边界

官方资料说明工具机制；本文件负责项目选型、职责、目录和约束。项目约束优先于指南中的通用示例，例如 Next.js 可直连数据库的通用模式不适用于本项目的 Go 业务所有权。

- [Next.js Server / Client Components](https://nextjs.org/docs/app/getting-started/server-and-client-components)、[Next.js ESLint](https://nextjs.org/docs/app/api-reference/config/eslint)：渲染边界与检查配置。
- [shadcn/ui](https://ui.shadcn.com/docs)、[表单方案](https://ui.shadcn.com/docs/forms/react-hook-form)、[Table](https://ui.shadcn.com/docs/components/radix/table)、[Radix Composition](https://www.radix-ui.com/primitives/docs/guides/composition)：组件源码、组合与交互基础。
- [pnpm install](https://pnpm.io/cli/install)、[uv locking and syncing](https://docs.astral.sh/uv/concepts/projects/sync/)：锁文件与可复现安装。
- [Go 模块组织](https://go.dev/doc/modules/layout)、[Google Go Style Guide](https://google.github.io/styleguide/go/guide)、[Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)：语言工程与组织原则。
- [FastAPI 多文件应用](https://fastapi.tiangolo.com/tutorial/bigger-applications/)：路由组织与依赖组合。
- [swaggo/swag](https://github.com/swaggo/swag)、[@umijs/openapi](https://github.com/chenshuai2144/openapi2typescript)：注解生成与在线接口消费。

本轮同时使用 Vercel 插件的 Next.js / React 工程技能和用户指定的 shadcn 技能；不把本机技能安装路径写成团队运行依赖。历史项目文档用于后续冲突清理与追溯，不作为本次目标结构的模板。
