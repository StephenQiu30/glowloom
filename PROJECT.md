# 织光工程规则

项目为 Glowloom（织光），仓库 `StephenQiu30/glowloom`；当前前端、Go 业务后端、协作运行时及文档同仓维护，包含独立 `agent/`。Go 模块暂保留 `github.com/StephenQiu30/glowloom-server/backend`，模块名称不等于旧实现兼容。目标配置只使用 `GLOWLOOM_*`，不保留 `LV_*` 别名；资源名称以实际部署为准。

## 1. 当前阶段与文件职责

已确认规则保留：邮箱 + 密码、注册验证邮箱；个人与团队创作者共用账号，平台管理与团队管理权限分别校验；账号注销采用即时软删除并撤权。当前实施进度、下一步与剩余接受统一见 [BACKLOG](BACKLOG.md)，历史证据保留在[用户系统设计](workspace/content/design/01-用户系统总体设计.md)和[测试与运行设计](workspace/content/design/04-用户系统测试与运行设计.md)。

[文档索引](workspace/content/00-文档索引.md) 为文档入口。原始需求与确认在 workspace/content/requirement 独立保留；按 design → PRD → Plan 分层，技术合同在 design，用户价值、范围及验收在 PRD，实施步骤和实际验证证据在 Plan。每份 PRD 显式指定唯一来源设计，每份 Plan 显式指定唯一来源 PRD；专项技术依赖通过链接展开。设计使用[八节模板](workspace/content/templates/01-设计模板.md)，故事与场景按模板进入对应主题。当前实施进度与调度只在 BACKLOG。已有长期调研保留来源、适用版本与采用边界，研究不等于采用或验收。AGENTS 负责长期协作规则，根 DESIGN 负责共享视觉与交互。完整候选范围见[原始产品范围](workspace/content/requirement/01-产品范围.md)和[总 PRD](workspace/content/prd/10-产品总览与交付边界.md)，不据目录承诺排期或完成。

## 2. 可继续使用的工具

前端现有 Next.js App Router、React、TypeScript strict、pnpm、Tailwind、shadcn/Radix、ESLint/Prettier；后端现有Go、Gin、GORM、Wire、swag。安装版本以包清单和lockfile为准。继续使用工具不等于继续使用旧业务合同或要求所有旧依赖进入新运行架构。

Agent Harness 与多 Agent 的已确认目标（2026-11-04，SRC-AGH-12）：同仓库独立 `agent/`，TypeScript strict + 官方 Pi Durable，独立构建/测试/容器与进程；通过当前内部 HTTP/SSE 与 Go 业务 API 交互。Agent 单独拥有会话、运行、计划、子 Agent 和恢复状态；Go 只保留身份/审批/预算/模型出站/工具与业务数据权威，不镜像执行状态、不启动或监督 Node。复用 PostgreSQL，使用独立 `agent` schema 与受限角色；Docker Compose 统一启动。项目移除 Temporal，普通后台任务使用数据库工作记录与 Go worker。新架构无旧实现、兼容/双轨/别名/回退。完整合同见[设计32](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)；代码与 Compose 已按该合同实施；工程和产品接受分别记录在 BACKLOG，不能由设计推定验证通过。

`agent` 服务与根目录 `agent/`、包名 `agent`（private）、Compose/CI `agent`、镜像 `glowloom/agent` 统一命名。服务使用 Node 24、TypeScript ESM、NestJS 与官方 Fastify 适配器。Controller/DTO 注解生成 OpenAPI，TypeBox 仅用于 Pi 工具参数。Pi Durable、pg、Pino、Vitest 与 pnpm/tsc 延续现有基线。只发布单一 NestJS 实现，移除原生 Fastify 入口、TypeBox HTTP 合同及兼容分支。完整技术栈与版本见[设计32](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)，目标目录和生成链见[设计37](workspace/content/design/37-Agent服务目录与TypeScript规范设计.md)。

`canvas` 服务对应 `canvas/`、Compose 内部地址 `canvas:4400`、镜像 `glowloom/canvas` 与私有包 `@glowloom/canvas`。服务使用 Node 24、NestJS/Fastify、TypeScript ESM 和 Yjs。Go 拥有鉴权、接受与持久化，Canvas 计算候选更新，浏览器只消费共享包的指定子路径。目录见[设计17](workspace/content/design/17-画布Yjs协作编辑设计.md)。

Harness 按[设计32 §7.6](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)直接组合官方 registry/tool/doc/owned conversation；四个同版 Pi 包中 coding-agent 仅提供受控资源 SDK，不运行 CLI 或第二个 AgentSession。运行固定 manifest、显式角色工具名单，模型/工具出口强制校验 Go 授权；第三方 Skill 脚本走 Go 隔离能力。审批/暂停收束后的 continuation 与崩溃恢复原未决任务分别处理，不自写循环或另造插件加载器。

Go 业务模块按 `adapter → application → domain` 单向依赖并显式注入依赖；编码与审查遵循根 [AGENTS](AGENTS.md) 中的 Google Go 与 Vercel/Next.js 规则。独立 Node 服务遵循 Google TypeScript Style Guide 及各自的 NodeNext/NestJS 约定；Agent 目录职责见[设计37 §7.2](workspace/content/design/37-Agent服务目录与TypeScript规范设计.md#72-目录与依赖)。两端都不预建空层、通用框架或仅转发调用的抽象。

Go 业务后端的统一二进制入口仍为 `backend/cmd/glowloom`，承载 API、worker、relay 与身份/存储/分区运维；已移除旧工作流初始化命令。独立 Agent 服务有自己的 TypeScript 入口、Dockerfile 与锁文件，生命周期由 Compose 管理；不把“Go 只有一个入口”解释成整个应用必须单进程。

业务实现放在 `backend/internal/<模块>/`：`domain` 只含规则，`application` 编排用例，`adapter/<技术>` 对接 HTTP、数据库、工作流或外部系统。领域与应用不依赖 Gin、GORM，也不调用另一个业务模块的应用服务。跨模块事实由消费方定义小接口。

包按 Google Go 的内聚规则划分：一个包只放必须一起使用的类型，调用方不需要同时理解无关实现。整个后端不放进一个包，也不使用 `util`、`common` 这类无职责包。

`backend/internal/app` 的 `package app` 只保留角色进程、路由入口和 Wire 注入。圣经、素材、项目复制、画布、操作等服务的跨模块适配各自放在 `backend/internal/app/<服务>/`，包名与目录一致。这些包只做注入适配，不放领域规则，也不再增加进程入口。身份运行时、身份维护和存储维护保持各自的包，不并入 `package app`。新的服务接线写入对应服务包；不再往 `package app` 增加服务接线文件。

`backend/internal/platform/` 的目标职责为 Go 配置、日志及 PostgreSQL、Redis、Kafka、对象存储连接，移除 Temporal。Agent 独立拥有 HTTP、Pi、PostgreSQL Storage 与执行生命周期；双方不共享业务数据库模型。配置只接受当前 `GLOWLOOM_*`，不保留旧键回退。实际凭据、数据库角色与既有资源使用现有部署配置；新增受限角色/schema 由部署管理员按完整结构定义提供，应用启动不建表。

前端复用组件按 feature 放在 `frontend/src/components/<feature>/`；`components/ui/` 只保留有消费者的 shadcn/Radix 基础组件。当前跨页面复用内容按实际业务职责归属维护，直接导入具体文件，不使用产品拼音目录或统一导出层。

页面与页面独有组件放在对应 App Router 页面目录：`src/app/<路由>/page.tsx` 负责入口与元数据，`src/app/<路由>/_components/` 存放页面实现、局部状态和私有组件；首页使用 `src/app/_components/`。不额外建立 `src/features/` 或另一套 Pages Router。共享认证视图服务登录、注册和重置密码三个页面；公共灵感素材在 `components/inspiration/`，个人素材合同和预览在 `components/media/`。复用组件不得反向导入路由页面或页面私有组件；页面不得从其他页面模块导入共用数据或控件。

## 3. 数据与契约

- 按接受的设计维护 Handler/DTO 和注解，由固定版本 swag 生成规范，再由固定版本 @umijs/openapi 生成前端调用；`frontend/src/api/` 全部为生成文件，不手改，不在组件目录手写业务请求函数或维护另一套客户端。`frontend/openapi2ts.config.ts` 只声明生成配置，遵循 [@umijs/openapi 官方用法](https://github.com/chenshuai2144/openapi2typescript#usage)，不执行外部命令、切换进程目录或改写工具模板。在 `frontend/` 执行 `pnpm api:generate`：先运行 `api:schema` 将后端注解导出到忽略目录 `node_modules/.cache/openapi/swagger.json`，再运行官方 openapi2ts 和项目 Prettier。无需启动后端；生成需要 Go、Node 和已安装的前端依赖。`pnpm api:check` 独立重生成并逐文件核对、核对全部 operationId，发现手改、漏生成或额外文件即失败。
- 通用请求层为 `frontend/src/lib/request.ts`：使用 Axios 实例、原生 Axios 配置和拦截器，统一超时、幂等键、multipart 与 HTTP 错误转换，直接返回后端 payload。取消保留 Axios 的取消语义；传输层不导入 UI/认证组件，不弹窗或跳转。认证文案和会话校验位于 `components/auth/contracts.ts`。页面导入 `@/api/<标签>`，类型使用生成的 `API` 命名空间。下载传入自定义 options 时显式指定 `responseType: "blob"`；@umijs/openapi 1.14.1 原始模板只有在 options 未提供时使用 hook 默认值，不通过改写模板改变该行为。
- 后端不提供在线 Swagger 路由，不新增跟踪的 backend/docs 产物。生成合同检查与实际路由双向核对。
- `backend/db/schema.sql` 是仓库唯一的 PostgreSQL 结构定义，保留身份等系统初始化。开发、CI 和部署先用它初始化空库，再导入 `backend/db/data.sql` 的公共供应商、能力、版本化模型候选及官方计价参考。模型目录运行时读取 PostgreSQL 的固定版本快照；种子不写入连接、凭据、真实验证或已审核价目，同版本内容冲突拒绝导入。存量环境以 PostgreSQL 实际结构为准，部署前核对其与当前定义是否一致。仓库仅维护完整结构，禁止向已有业务库重放空库 SQL，禁止新增 migrations 目录或增量 SQL 文件，服务启动及运行时严禁执行任何动态 DDL，防止新实例冷启动时产生数据与结构不一致。邮箱与身份引用保护见[用户系统设计](workspace/content/design/01-用户系统总体设计.md#81-d6-迁移与切换)。
- 浏览器请求经同站代理；服务端 `GLOWLOOM_BACKEND_ORIGIN` 不进入浏览器。凭据不进入文档、日志、前端包或提交。
- 默认本地工程验证不发起真实收费生成；fixture、协议握手或页面成功不能代替真实供应商验收。模型可用性以实际配置和适配能力为准。
- `docker-compose.yml` 是部署基础配置；完成数据库结构核验后的 `docker compose up -d --build --wait` 统一启动 frontend、backend、独立 agent 和 canvas。前端使用 Next.js standalone 镜像，完整启动等待后端健康。后端直接运行构建阶段生成的 Go 二进制。Agent 独立镜像及连接池/资源限制，仅在 Compose 网络监听 `agent:4401`，不发布宿主端口；Go 与 Agent 不设置相互 service_healthy 等待、不管理彼此进程。前端仍为 `http://localhost:1122`，Go 为 `http://127.0.0.1:1123`。复用既有 PostgreSQL、Redis、Kafka、对象存储，不创建依赖栈，不保留 Temporal 服务/namespace 配置。当前源码与 Compose 采用此拓扑；已有库的结构核验在启动前独立完成。
- 本地开发可显式叠加 `docker-compose.dev.yml` 使用 Compose Watch。前端用 Next.js 开发服务器同步页面源码和静态文件。Go、Agent、画布协作由 Compose 分别重建并替换容器。开发镜像使用独立标签，沿用基础配置的端口、健康检查和宿主依赖。
- API 生成使用上述 package.json 命令组织标准工具；其他检查使用现有工具直接命令，不新增 shell 脚本或 Makefile。

## 4. 质量门禁

| 范围        | 适用检查                                                                                                                                                                                                |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 实现     | gofmt、goimports、go vet、golangci-lint、go test -race、govulncheck；相应契约生成一致性                                                                                                                 |
| 前端实现    | 在 frontend 执行 pnpm exec eslint .、pnpm exec prettier --check .、pnpm exec next typegen、pnpm exec tsc --noEmit、pnpm exec vitest run、pnpm exec next build；交互变化补浏览器验证                     |
| Agent 服务  | 在 agent 独立执行 TypeScript strict、lint、单元/官方 Storage conformance、真实隔离 PostgreSQL 与跨服务测试及构建；Go Agent 边界保留 backend/tests；验证根 owner/grant、可靠控制投递、SSE 一致性与无兼容 |
| Canvas 服务 | 在 `canvas/` 执行 `pnpm typecheck`、`pnpm check` 和 `pnpm test`，并验证 Go 内部 HTTP 和前端共享包导出。                                                                                                 |
| 文档        | Markdown 格式、相对链接与锚点、来源路径、需求/验收对应关系、陈述与源码一致；第三方许可原文保持不变                                                                                                      |

默认 CI 并行运行 backend、agent、frontend 三个任务。Go 版本来自 `backend/go.mod`，Node.js 使用统一的 24 主版本，pnpm 版本来自各包清单，依赖使用冻结锁文件及缓存。

- Backend 运行一次 `go test -race -count=1 ./...`，覆盖 Go 默认 vet、编译、竞态和跨服务协议。Canvas 的 `pnpm test` 同时构建生产入口，供后续 Go 测试直接使用。隔离 PostgreSQL、COS 协议夹具和 FFmpeg 提供实际集成依赖。
- Agent 在独立 PostgreSQL 测试库和受限角色下做严格类型检查及一次生产编译和全量测试，覆盖官方 Storage、进程恢复与双向 API 合同。全量测试同时导出 Agent OpenAPI，随后核对 Go 客户端产物和生成声明。
- Frontend 构建共享 Canvas 包后运行全量 Vitest，最多两个 worker。一次 Next.js 生产构建完成 TypeScript 检查，并用 standalone 产物验证协作代理。

格式、lint、专项安全扫描和容器镜像检查按改动范围执行。部署与真实供应商验收单独验证。

验证范围与修改风险匹配。文档变更不需要重跑无关业务测试；UI/工程检查、隔离存储、真实邮件、实际数据库迁移和产品接受分别记录。缺少外部条件或未执行的检查不得报告通过。

Go 测试统一放在 `backend/tests/<业务模块>/`，通过模块构造函数、应用接口或真实协议入口验证行为；生产源码目录不放置 `*_test.go`。前端测试统一放在 `frontend/tests/`：单元与组件测试在 `frontend/tests/unit/`，目录与 `src/` 对齐，并通过 `@/` 导入被测模块；浏览器夹具留在 `frontend/tests/browser/`。生产源码目录不放置 `*.test.ts` 或 `*.test.tsx`。测试布局沿用现有工程约定，由本地检查和评审维护；迁移测试时保留原有断言。

## 5. 本地文档与交付

文档使用简短、直接的表述，只保留现行规则、必要条件与验收；重复说明改为来源链接，界面文案遵循[DESIGN 极简呈现](DESIGN.md#极简呈现)。

项目知识以标准Markdown／Git为唯一正文，已经归入同仓独立`workspace/content/`，Workspace服务代码与正文分离。采用Next App Router／React／TypeScript strict、Tailwind／现有shadcn组件、Geist／Neutral、Node24／pnpm、Pagefind和Mermaid。`build:static`生成独立export产物，`build:online`生成完整standalone包；分别构建、部署与验收，接口／凭据／运行存储不进入静态产物。规则见[设计43](workspace/content/design/43-Workspace知识库与研发工作流服务设计.md)，操作见[服务说明](workspace/README.md)。在线采用本机私有令牌、短期会话、同源／CSRF及受控执行桥；首个主机为Codex app-server。研发记录使用PG独立`dev_workspace` schema与受限角色；现有业务`workspace` schema保持原归属。真实TTS暂时禁用，支持已有授权音频／cue导入与原生播放；实际状态和验证边界只见BACKLOG。

需求、设计、PRD、Plan、长期调研和模板继续各自保留独立文件与导航。原始需求、确认、文件名及稳定 ID 不因整理而删除或覆盖；引用修复不得改变业务含义。技术合同、产品范围和实施步骤各有唯一维护位置，当前调度只在 BACKLOG。根规范和工程 README 保留原位置，Workspace关联工程事实，不复制成第二套权威。知识正文不添加脚本、测试、依赖或构建产物；本机编辑器配置和第三方版权不随整理覆盖或删除。

提交遵循 AGENTS；只包含本任务，报告检查结果、未决项与工作区状态。未经用户要求不提交、推送或发布，不改外部任务系统状态。
