# 织光工程规则

项目为 Glowloom（织光），仓库 `StephenQiu30/glowloom`；当前前端、Go 业务后端、协作运行时及文档同仓维护，包含独立 `agent/`。Go 模块暂保留 `github.com/StephenQiu30/glowloom-server/backend`，模块名称不等于旧实现兼容。目标配置只使用 `GLOWLOOM_*`，不保留 `LV_*` 别名；资源名称以实际部署为准。

## 1. 当前阶段与文件职责

已确认规则保留：邮箱 + 密码、注册验证邮箱；个人与团队创作者共用账号，平台管理与团队管理权限分别校验；账号注销采用即时软删除并撤权。当前实施进度、下一步与剩余接受统一见 [BACKLOG](BACKLOG.md)，历史证据保留在[用户系统设计](workspace/content/design/01-用户系统总体设计.md)和[测试与运行设计](workspace/content/design/04-用户系统测试与运行设计.md)。

[文档索引](workspace/content/index.mdx) 为文档入口。原始需求与确认在 workspace/content/requirement 独立保留；按 design → PRD → Plan 分层，技术合同在 design，用户价值、范围及验收在 PRD，实施步骤和实际验证证据在 Plan。每份 PRD 显式指定唯一来源设计，每份 Plan 显式指定唯一来源 PRD；专项技术依赖通过链接展开。设计使用[八节模板](workspace/content/templates/01-设计模板.md)，故事与场景按模板进入对应主题。当前实施进度与调度只在 BACKLOG。已有长期调研保留来源、适用版本与采用边界，研究不等于采用或验收。AGENTS 负责长期协作规则，根 DESIGN 负责共享视觉与交互。完整候选范围见[原始产品范围](workspace/content/requirement/01-产品范围.md)和[总 PRD](workspace/content/prd/10-产品总览与交付边界.md)，不据目录承诺排期或完成。

## 2. 可继续使用的工具

前端现有 Next.js App Router、React、TypeScript strict、pnpm、Tailwind、shadcn/Radix、ESLint/Prettier；后端现有 Go、Gin、GORM、pgx、gRPC-Go/Protobuf、Kafka（`franz-go`）、Redis、swag（已移除归档的 Wire）。安装版本以包清单和 lockfile 为准。继续使用工具不等于继续使用旧业务合同或要求所有旧依赖进入新运行架构。

Agent 服务与多 Agent 的现行唯一架构合同（2026-11-25，SRC-AGV2-02，见[设计48](workspace/content/design/48-Agent服务Mastra架构与技术选型设计.md)）：同仓库独立 `agent/`（Node 24、TypeScript strict ESM），基于 **Mastra**（底层 **Vercel AI SDK**）以纯内核库形态运行，移除 NestJS、Fastify、Prisma、Pi Durable、TypeBox 与双向内部 OpenAPI 生成链。Go → Agent 与 Agent → Go 鉴权/配额/摘要通过唯一真源 `backend/api/proto/glowloom/agent/runtime.proto` 的强类型 gRPC（含 Server Streaming）通信；Agent → Go 业务工具通过 Go 内部 MCP Server（`/internal/agent-mcp`）与 `@mastra/mcp` 交互；Agent 文本 LLM 调用通过 Go 统一渠道透传网关（`/internal/agent-gateway`）完成，100% 保留 Prompt Cache/Thinking/流式特性并闭环多租户计费账本。

`agent` 服务与根目录 `agent/`、包名 `agent`（private）、Compose/CI `agent`、镜像 `glowloom/agent` 统一命名，仅监听内部 gRPC（`:4402`）与极简 HTTP 健康探针（`:4401`，仅 `/healthz`、`/readyz`）。数据访问层采用 **`@mastra/pg`（显式关闭运行时自动 DDL）+ `Kysely` + `pg` + `Zod`**，复用 PostgreSQL 独立 `agent` schema 与受限角色，所有表结构仍唯一静态声明于 `backend/db/schema.sql`。编排采用混合模式（Producer 主 Agent + 专家子 Agent + 关键审批流程走 Mastra Workflow 原生 `suspend/resume`）；记忆与业务真源分离；声明式 Skill 绑定 MCP 白名单且容器内零脚本执行；可观测性与评测采用双轨（离线 Mastra Scorers + Vitest 轨迹门禁，线上自托管 Langfuse OTel Trace + Prompt 管理与本地兜底）。目录与编码规范见 [agent/AGENTS.md](agent/AGENTS.md)。

`canvas` 服务对应 `canvas/`、Compose 内部地址 `canvas:4402`（gRPC）/ `canvas:4400`（HTTP 健康）、镜像 `glowloom/canvas` 与私有包 `@glowloom/canvas`。服务定位为**纯协作引擎（前后端同构 Yjs/Tiptap 协议内核 + 无状态 Node 24 gRPC 候选计算边车）**，不包含 React/Next/Three.js 等浏览器 UI 源码或前端 peerDependencies，也不直接访问数据库；Go 拥有鉴权、接受、WebSocket 广播与持久化权威。全部画布浏览器编辑器 UI 统一归位 `frontend/src/components/canvas-editor/`（与 `frontend/src/components/canvas/` 同属 `frontend/` 唯一 UI 权威），单向消费 `@glowloom/canvas/{recovery,session,journal,wire,model,rich-text,service-document}` 纯内核子路径，不再维护 `canvas/editor/` 与 `@glowloom/canvas-host` 反向别名。

Agent 创作业务合同由[设计32](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)维护，运行架构与工程目录规范由[设计48](workspace/content/design/48-Agent服务Mastra架构与技术选型设计.md)和 [agent/AGENTS.md](agent/AGENTS.md) 维护。

Go 业务模块按 `adapter → application → domain` 单向依赖并显式注入依赖；编码与审查遵循根 [AGENTS](AGENTS.md) 中的 Google Go 与 Vercel/Next.js 规则。独立 Node 24 TypeScript 服务遵循 Google TypeScript Style Guide 及各自目录下的 `AGENTS.md` 约定。两端都不预建空层、通用框架或仅转发调用的抽象。

Go 业务后端已按 [设计47](workspace/content/design/47-Go模块化单体架构与RPC集成设计.md)（来源 [需求32](workspace/content/requirement/32-Go模块化单体架构改造需求.md)）收敛为遵循 Google Go 规范的**模块化单体**：唯一入口为 `backend/cmd/glowloom`，默认 `--role=all` 通过 `errgroup` 在单进程内统一运行公开 HTTP/SSE/WebSocket API、后台任务 Worker、Kafka（`franz-go`）异步事件 Relay，以及进程内有界并发（`semaphore.Weighted`）的 TXT/DOCX 文档提取与 FFmpeg 规范化；已移除独立 `gateway`、`creation`、`media`、`worker`、`relay` 微服务容器、`writefence` 分布式围栏与旧 Go 内部微服务 proto，跨模块不变量回归 PostgreSQL 本地 ACID 事务。独立 `canvas/` 与 `agent/` 为 Node 24 TypeScript 运行时，Go 单体与二者通过标准 **gRPC + Protobuf + Buf**（方案 A：`backend/api/proto/glowloom/{canvas,agent}/runtime.proto`，生成物位于 `backend/internal/gen/`）通信，其中 Agent 事件流采用 gRPC Server Streaming，对外浏览器保持标准 HTTP/SSE/WebSocket 合同。Compose 收敛为 5 个应用服务：`frontend`（1122）、`backend`（1123）、`canvas`（gRPC 4402 / HTTP 健康 4400）、`agent`（gRPC 4402 / HTTP 健康 4401）与 `workspace`（1130）。

Go 业务代码收敛于 `backend/internal/`（按业务域组织 `identity`、`creation`、`media`、`execution` 及专用子包 `agent`、`skill`、`analytics`、`observability`、`audit`、`infra`、`platform`、`app`、`gen`），保留根 `backend/go.mod`/`go.sum`、`backend/cmd/glowloom`、`backend/api/proto`、`backend/db` 与 `backend/tests`。已有业务分层保持 `adapter → application → domain`：`domain` 只含规则，`application` 编排用例，`adapter/<技术>` 对接协议、数据库或外部系统；领域与应用不依赖 Gin、GORM，跨模块事实由消费方定义小接口，不使用 `util`、`common` 等无职责通用包。

`backend/internal/platform/` 的职责为 Go 配置、日志及 PostgreSQL、Redis、Kafka、对象存储连接。Agent 独立拥有 Mastra 内核、gRPC/健康探针与受限 PostgreSQL Storage；双方不共享业务数据库模型。配置只接受当前 `GLOWLOOM_*`，不保留旧键回退。实际凭据、数据库角色与既有资源使用现有部署配置；新增受限角色/schema 由部署管理员按完整结构定义提供，应用启动不建表。

前端复用组件按 feature 放在 `frontend/src/components/<feature>/`；`components/ui/` 只保留有消费者的 shadcn/Radix 基础组件。当前跨页面复用内容按实际业务职责归属维护，直接导入具体文件，不使用产品拼音目录或统一导出层。

页面与页面独有组件放在对应 App Router 页面目录：`src/app/<路由>/page.tsx` 负责入口与元数据，`src/app/<路由>/_components/` 存放页面实现、局部状态和私有组件；首页使用 `src/app/_components/`。不额外建立 `src/features/` 或另一套 Pages Router。共享认证视图服务登录、注册和重置密码三个页面；公共灵感素材在 `components/inspiration/`，个人素材合同和预览在 `components/media/`。复用组件不得反向导入路由页面或页面私有组件；页面不得从其他页面模块导入共用数据或控件。

## 3. 数据与契约

- 按接受的设计维护 Handler/DTO 和注解，由固定版本 swag 生成规范，再由固定版本 @umijs/openapi 生成前端调用；`frontend/src/api/` 全部为生成文件，不手改，不在组件目录手写业务请求函数或维护另一套客户端。`frontend/openapi2ts.config.ts` 只声明生成配置，遵循 [@umijs/openapi 官方用法](https://github.com/chenshuai2144/openapi2typescript#usage)，不执行外部命令、切换进程目录或改写工具模板。在 `frontend/` 执行 `pnpm api:generate`：先运行 `api:schema` 将后端注解导出到忽略目录 `node_modules/.cache/openapi/swagger.json`，再运行官方 openapi2ts 和项目 Prettier。无需启动后端；生成需要 Go、Node 和已安装的前端依赖。`pnpm api:check` 独立重生成并逐文件核对、核对全部 operationId，发现手改、漏生成或额外文件即失败。
- 通用请求层为 `frontend/src/lib/request.ts`：使用 Axios 实例、原生 Axios 配置和拦截器，统一超时、幂等键、multipart 与 HTTP 错误转换，直接返回后端 payload。取消保留 Axios 的取消语义；传输层不导入 UI/认证组件，不弹窗或跳转。认证文案和会话校验位于 `components/auth/contracts.ts`。页面导入 `@/api/<标签>`，类型使用生成的 `API` 命名空间。下载传入自定义 options 时显式指定 `responseType: "blob"`；@umijs/openapi 1.14.1 原始模板只有在 options 未提供时使用 hook 默认值，不通过改写模板改变该行为。
- 后端不提供在线 Swagger 路由，不新增跟踪的 backend/docs 产物。生成合同检查与实际路由双向核对。
- `backend/db/schema.sql` 是仓库唯一的 PostgreSQL 结构定义，保留身份等系统初始化。数据库主机须安装 pgvector，CI 使用 PostgreSQL 18 与 pgvector 0.8.6。开发、CI 和部署先用它初始化空库，再导入 `backend/db/data.sql` 的公共供应商、能力、版本化模型候选及官方计价参考。模型目录运行时读取 PostgreSQL 的固定版本快照；种子不写入连接、凭据、真实验证或已审核价目，同版本内容冲突拒绝导入。存量环境以 PostgreSQL 实际结构为准，部署前核对其与当前定义是否一致。仓库仅维护完整结构，禁止向已有业务库重放空库 SQL，禁止新增 migrations 目录或增量 SQL 文件，服务启动及运行时严禁执行任何动态 DDL，防止新实例冷启动时产生数据与结构不一致。邮箱与身份引用保护见[用户系统设计](workspace/content/design/01-用户系统总体设计.md#81-d6-迁移与切换)。
- 浏览器请求经同站代理；服务端 `GLOWLOOM_BACKEND_ORIGIN` 不进入浏览器。凭据不进入文档、日志、前端包或提交。
- 默认本地工程验证不发起真实收费生成；fixture、协议握手或页面成功不能代替真实供应商验收。模型可用性以实际配置和适配能力为准。
- 跨服务 RPC 的唯一协议真源位于 `backend/api/proto/glowloom/{canvas,agent}/runtime.proto`，经 Buf 生成 Go 合同至 `backend/internal/gen/`，并通过 Compose `additional_contexts` 注入 `canvas/` 与 `agent/` 构建上下文。
- `docker-compose.yml` 是部署基础配置；完成数据库结构核验后的 `docker compose up -d --build --wait` 统一启动 5 个应用服务：`frontend`（`http://localhost:1122`）、`backend`（`http://127.0.0.1:1123`，单进程 `--role=all`）、`canvas`（内部 gRPC `4402` / 健康 `4400`）、`agent`（内部 gRPC `4402` / 健康 `4401`）与 `workspace`（`http://localhost:1130/docs`）。前端使用 Next.js standalone 镜像经 `backend:18080` 访问业务后端；`agent` 与 `canvas` 不发布宿主端口，Go 与 Agent 不设置相互 `service_healthy` 等待。复用既有 PostgreSQL、Redis、Kafka、对象存储，不创建依赖栈。当前源码与 Compose 采用此拓扑；已有库的结构核验在启动前独立完成。
- 本地开发可显式叠加 `docker-compose.dev.yml` 使用 Compose Watch。前端用 Next.js 开发服务器同步页面源码和静态文件。Go、Agent、画布协作由 Compose 分别重建并替换容器。开发镜像使用独立标签，沿用基础配置的端口、健康检查和宿主依赖。
- API 生成使用上述 package.json 命令组织标准工具；其他检查使用现有工具直接命令，不新增 shell 脚本或 Makefile。

## 4. 质量门禁

| 范围        | 适用检查                                                                                                                                                                                                |
| ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 实现     | gofmt、goimports、go vet、golangci-lint、go test -race、govulncheck；相应契约生成一致性                                                                                                                 |
| 前端实现    | 在 frontend 执行 pnpm exec eslint .、pnpm exec prettier --check .、pnpm exec next typegen、pnpm exec tsc --noEmit、pnpm exec vitest run、pnpm exec next build；交互变化补浏览器验证                     |
| Agent 服务  | 在 agent 独立执行 TypeScript strict、lint、单元/官方 Storage conformance、真实隔离 PostgreSQL 与跨服务测试及构建；Go Agent 边界保留 backend/tests；验证根 owner/grant、可靠控制投递、SSE 一致性与无兼容 |
| Canvas 服务 | 在 `canvas/` 执行 `pnpm typecheck`、`pnpm check` 和 `pnpm test`，并验证 Go 内部 HTTP 和前端共享包导出。编辑器源在 frontend 由 ESLint 和 Next.js 生产构建检查，包边界与交互由 Vitest 验证。                                                                                                 |
| 文档        | Markdown 格式、相对链接与锚点、来源路径、需求/验收对应关系、陈述与源码一致；第三方许可原文保持不变                                                                                                      |

默认 CI 并行运行 backend、agent、frontend 三个任务。Go 版本来自 `backend/go.mod`，Node.js 使用统一的 24 主版本，pnpm 版本来自各包清单，依赖使用冻结锁文件及缓存。

- Backend 运行一次 `go test -race -count=1 ./...`，覆盖 Go 默认 vet、编译、竞态和跨服务协议。Canvas 的 `pnpm test` 同时构建生产入口，供后续 Go 测试直接使用。隔离 PostgreSQL、COS 协议夹具和 FFmpeg 提供实际集成依赖。
- Agent 执行类型检查、ESLint、Vitest、评测和生产编译。画布 Skill 运行测试使用独立 PostgreSQL 测试库与受限角色，Go 边界测试覆盖 gRPC 合同。
- Frontend 构建共享 Canvas 包后运行全量 Vitest，最多两个 worker。一次 Next.js 生产构建完成 TypeScript 检查，并用 standalone 产物验证协作代理。

格式、lint、专项安全扫描和容器镜像检查按改动范围执行。部署与真实供应商验收单独验证。

验证范围与修改风险匹配。文档变更不需要重跑无关业务测试；UI/工程检查、隔离存储、真实邮件、实际数据库迁移和产品接受分别记录。缺少外部条件或未执行的检查不得报告通过。

Go 测试统一放在 `backend/tests/<业务模块>/`，通过模块构造函数、应用接口或真实协议入口验证行为；生产源码目录不放置 `*_test.go`。前端测试统一放在 `frontend/tests/`：单元与组件测试在 `frontend/tests/unit/`，目录与 `src/` 对齐，并通过 `@/` 导入被测模块；浏览器夹具留在 `frontend/tests/browser/`。生产源码目录不放置 `*.test.ts` 或 `*.test.tsx`。测试布局沿用现有工程约定，由本地检查和评审维护；迁移测试时保留原有断言。

## 5. 本地文档与交付

文档使用简短、直接的表述，只保留现行规则、必要条件与验收；重复说明改为来源链接，界面文案遵循[DESIGN 极简呈现](DESIGN.md#极简呈现)。

项目知识以标准 Markdown／Git 为唯一正文，保留在同仓独立 `workspace/content/`。Workspace 直接采用官方 `create-fumadocs-app`：Next.js、Fumadocs UI（官方 Base UI 实现与 shadcn 主题）、Fumadocs MDX、TypeScript strict、Tailwind、Node24／pnpm，以及 Fumadocs 内置搜索（`/api/search`）与 LLM 知识接口（`/llms.txt`、`/llms-full.txt`、`/llms.mdx`）。官方组件提供布局、侧栏、目录、代码块、相邻页与搜索，普通 .md 由官方内容源编译，旧地址通过 Next.js 原生重定向承接。只有标准 Next.js 构建与 1130 免登录入口，预渲染阅读内容；不另建静态／在线双应用，不保留自研构建、认证、PG 协作、执行桥、任务运行器或 TTS。规则见[设计45](workspace/content/design/45-Workspace官方Fumadocs设计.md)与 [workspace/AGENTS.md](workspace/AGENTS.md)，运行见[服务说明](workspace/README.md)，当前进度与验证边界见 BACKLOG。

知识文档按分类使用唯一的 `NN-名称` 文件编号，Fumadocs 从文件名同步侧栏、页面与搜索标题；已有编号空缺和稳定 ID 保留。COS 等基础设施配置归 `workspace/content/operations/`，工具安装设置与临时过程文档不纳入知识库。维护规则见[编写规范](workspace/content/templates/05-编写规范.md)。

需求、设计、PRD、Plan、长期调研和模板继续各自保留独立文件与导航。原始需求、确认、文件名及稳定 ID 不因整理而删除或覆盖；引用修复不得改变业务含义。技术合同、产品范围和实施步骤各有唯一维护位置，当前调度只在 BACKLOG。根规范和工程 README 保留原位置，Workspace关联工程事实，不复制成第二套权威。知识正文不添加脚本、测试、依赖或构建产物；本机编辑器配置和第三方版权不随整理覆盖或删除。

提交遵循 AGENTS；只包含本任务，报告检查结果、未决项与工作区状态。未经用户要求不提交、推送或发布，不改外部任务系统状态。
