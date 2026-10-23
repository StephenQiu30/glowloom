# 织光工程规则

项目为 Glowloom（织光），仓库 `StephenQiu30/glowloom-server`；内部为 `frontend/`、`backend/` 和 `docs/`。Go 模块使用 `github.com/StephenQiu30/glowloom-server/backend`。配置统一使用 `GLOWLOOM_*`，旧 `LV_*` 在过渡期兼容读取；运行资源与数据库名称以实际部署为准。

## 1. 当前阶段与文件职责

已确认规则保留：邮箱 + 密码、注册验证邮箱；个人与团队创作者共用账号，平台管理与团队管理权限分别校验；账号注销采用即时软删除并撤权。当前实施进度、下一步与剩余接受统一见 [BACKLOG](BACKLOG.md)，历史证据保留在[用户系统设计](docs/design/01-用户系统总体设计.md)和[测试与运行设计](docs/design/04-用户系统测试与运行设计.md)。

[文档索引](docs/00-文档索引.md) 为文档入口。原始需求与确认在 docs/requirement 独立保留；按 design → PRD → Plan 分层，技术合同在 design，用户价值、范围及验收在 PRD，实施步骤和实际验证证据在 Plan。每份 PRD 显式指定唯一来源设计，每份 Plan 显式指定唯一来源 PRD；专项技术依赖通过链接展开。设计使用[八节模板](docs/templates/01-设计模板.md)，故事与场景按模板进入对应主题。当前实施进度与调度只在 BACKLOG。已有长期调研保留来源、适用版本与采用边界，研究不等于采用或验收。AGENTS 负责长期协作规则，根 DESIGN 负责共享视觉与交互。完整候选范围见[原始产品范围](docs/requirement/01-产品范围.md)和[总 PRD](docs/prd/10-产品总览与交付边界.md)，不据目录承诺排期或完成。

## 2. 可继续使用的工具

前端现有 Next.js App Router、React、TypeScript strict、pnpm、Tailwind、shadcn/Radix、ESLint/Prettier；后端现有Go、Gin、GORM、Wire、swag。安装版本以包清单和lockfile为准。继续使用工具不等于继续使用旧业务合同或要求所有旧依赖进入新运行架构。

业务按职责组织，`adapter → application → domain`，依赖显式注入，接口由消费方定义。编码与审查遵循AGENTS的Google Go与Vercel/Next.js规范；不预建未来目录、通用框架或空转发层。

后端只有一个进程入口 `backend/cmd/glowloom`。默认以 `--role=api|worker|relay|all` 运行对应角色；身份维护、对象存储维护、Temporal 计划和分区确保分别是 `glowloom identity`、`glowloom storage`、`glowloom temporal setup`、`glowloom partitions ensure`。不保留独立的 `cmd/identity`；全部运维子命令共用当前二进制。镜像入口和用法字符串同为 `glowloom`。

业务实现放在 `backend/internal/<模块>/`：`domain` 只含规则，`application` 编排用例，`adapter/<技术>` 对接 HTTP、数据库、工作流或外部系统。领域与应用不依赖 Gin、GORM，也不调用另一个业务模块的应用服务。跨模块事实由消费方定义小接口。

包按 Google Go 的内聚规则划分：一个包只放必须一起使用的类型，调用方不需要同时理解无关实现。整个后端不放进一个包，也不使用 `util`、`common` 这类无职责包。

`backend/internal/app` 的 `package app` 只保留角色进程、路由入口和 Wire 注入。圣经、素材、项目复制、画布、操作等服务的跨模块适配各自放在 `backend/internal/app/<服务>/`，包名与目录一致。这些包只做注入适配，不放领域规则，也不再增加进程入口。身份运行时、身份维护和存储维护保持各自的包，不并入 `package app`。新的服务接线写入对应服务包；不再往 `package app` 增加服务接线文件。

`backend/internal/platform/` 提供配置、日志以及数据库、Redis、Kafka、Temporal、对象存储连接。进程配置读 `GLOWLOOM_*`，兼容读取旧 `LV_*`；同名新键优先，显式空值不会回退旧键。邮件沿用既有 `SMTP_*`。数据库角色、库名、Kafka 主题、Temporal namespace 和对象桶名保持当前部署标识，命令改名不迁移这些资源。

前端复用组件按 feature 放在 `frontend/src/components/<feature>/`；`components/ui/` 只保留有消费者的 shadcn/Radix 基础组件。当前跨页面复用内容按实际业务职责归属维护，直接导入具体文件，不使用产品拼音目录或统一导出层。

页面与页面独有组件放在对应 App Router 页面目录：`src/app/<路由>/page.tsx` 负责入口与元数据，`src/app/<路由>/_components/` 存放页面实现、局部状态和私有组件；首页使用 `src/app/_components/`。不额外建立 `src/features/` 或另一套 Pages Router。共享认证视图服务登录、注册和重置密码三个页面；公共灵感素材在 `components/inspiration/`，个人素材合同和预览在 `components/media/`。复用组件不得反向导入路由页面或页面私有组件；页面不得从其他页面模块导入共用数据或控件。

## 3. 数据与契约

- 按接受的设计维护 Handler/DTO 和注解，由固定版本 swag 生成规范，再由固定版本 @umijs/openapi 生成前端调用；`frontend/src/api/` 全部为生成文件，不手改，不在组件目录手写业务请求函数或维护另一套客户端。`frontend/openapi2ts.config.ts` 只声明生成配置，遵循 [@umijs/openapi 官方用法](https://github.com/chenshuai2144/openapi2typescript#usage)，不执行外部命令、切换进程目录或改写工具模板。在 `frontend/` 执行 `pnpm api:generate`：先运行 `api:schema` 将后端注解导出到忽略目录 `node_modules/.cache/openapi/swagger.json`，再运行官方 openapi2ts 和项目 Prettier。无需启动后端；生成需要 Go、Node 和已安装的前端依赖。`pnpm api:check` 独立重生成并逐文件核对、核对全部 operationId，发现手改、漏生成或额外文件即失败。
- 通用请求层为 `frontend/src/lib/request.ts`：使用 Axios 实例、原生 Axios 配置和拦截器，统一超时、幂等键、multipart 与 HTTP 错误转换，直接返回后端 payload。取消保留 Axios 的取消语义；传输层不导入 UI/认证组件，不弹窗或跳转。认证文案和会话校验位于 `components/auth/contracts.ts`。页面导入 `@/api/<标签>`，类型使用生成的 `API` 命名空间。下载传入自定义 options 时显式指定 `responseType: "blob"`；@umijs/openapi 1.14.1 原始模板只有在 options 未提供时使用 hook 默认值，不通过改写模板改变该行为。
- 后端不提供在线 Swagger 路由，不新增跟踪的 backend/docs 产物。生成合同检查与实际路由双向核对。
- `backend/db/schema.sql` 是唯一的数据库结构定义。它只用于空业务库；禁止对已有业务库重放、重建或清理数据。邮箱与身份引用保护见[用户系统设计](docs/design/01-用户系统总体设计.md#81-d6-迁移与切换)。
- 浏览器请求经同站代理；服务端 `GLOWLOOM_BACKEND_ORIGIN` 不进入浏览器。凭据不进入文档、日志、前端包或提交。
- 默认本地工程验证不发起真实收费生成；fixture、协议握手或页面成功不能代替真实供应商验收。模型可用性以实际配置和适配能力为准。
- 本地 namespace 显式准备，不由应用自动创建；Go 工具链与依赖版本以 go.mod、Dockerfile 为准。默认 `docker-compose.yml` 只启动 `frontend`、`backend`，开发环境直接使用 `docker compose up -d --build --no-deps frontend backend` 启动或更新，只更新后端时使用 `docker compose up -d --build --no-deps backend`，不新增启动脚本。宿主访问端口固定为前端 `127.0.0.1:1122`、后端 `127.0.0.1:1123`；两端浏览器 Origin 固定为 `http://localhost:1122`，旧端口及 Origin 环境变量不覆盖默认开发栈。容器内部仍使用前端3000、API代理18080、API8080、worker8081和relay8082，worker/relay健康端口不发布到宿主。数据库等依赖复用现有服务，数据库端口保持原配置；应用端口被占用时，先协调端口冲突。`docker-compose-env.yml` 和 `docker-compose-prod.yml` 为独立配置，不参与默认启动。开发后端每次启动都从只读挂载的当前源码编译，编译失败即停止，不运行旧程序；构建缓存使用独立开发卷。纯 Go 源码更新也可 `docker compose restart backend`，运行中不会自动热重载；依赖、Dockerfile、入口或 Compose 配置更新须重新构建并更新容器。生产镜像仍在构建阶段编译。不把修改文档解释为重启服务授权。
- API 生成使用上述 package.json 命令组织标准工具；其他检查使用现有工具直接命令，不新增 shell 脚本或 Makefile。

## 4. 质量门禁

| 范围     | 适用检查                                                                                                                                                                            |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 实现  | gofmt、goimports、go vet、golangci-lint、go test -race、govulncheck；相应契约生成一致性                                                                                             |
| 前端实现 | 在 frontend 执行 pnpm exec eslint .、pnpm exec prettier --check .、pnpm exec next typegen、pnpm exec tsc --noEmit、pnpm exec vitest run、pnpm exec next build；交互变化补浏览器验证 |
| 文档     | Markdown 格式、相对链接与锚点、来源路径、需求/验收对应关系、陈述与源码一致；第三方许可原文保持不变                                                                                  |

默认 CI 仅保留 backend、frontend 两个任务：后端执行静态检查、漏洞检查、Wire 生成一致性和一次 `go test -race -count=1 -v ./...`；前端执行 lint、类型检查、Vitest 和一次 Next.js 构建，并保留两端的测试目录检查。不构建应用 Docker 镜像；后端任务启动隔离 PostgreSQL 18，按空库 schema 初始化并配置画布所有数据库测试及受限运行角色。画布数据库配置缺失直接失败，不允许跳过。Redis、Kafka、对象存储和 Temporal 的专项集成仍按模块变更在相应环境执行；默认 CI 通过不代表这些专项验收或生产可用性已经通过。

验证范围与修改风险匹配。文档变更不需要重跑无关业务测试；UI/工程检查、隔离存储、真实邮件、实际数据库迁移和产品接受分别记录。缺少外部条件或未执行的检查不得报告通过。

Go 测试统一放在 `backend/tests/<业务模块>/`，通过模块构造函数、应用接口或真实协议入口验证行为；生产源码目录不放置 `*_test.go`。前端测试统一放在 `frontend/tests/`：单元与组件测试在 `frontend/tests/unit/`，目录与 `src/` 对齐，并通过 `@/` 导入被测模块；浏览器夹具留在 `frontend/tests/browser/`。生产源码目录不放置 `*.test.ts` 或 `*.test.tsx`。迁移测试时保留原有断言，不通过放宽 CI 目录门禁解决布局问题。

## 5. 本地文档与交付

直接维护标准 Markdown，以 Git 管理修订；docs 作为 Obsidian vault，不建设知识库网站。需求、设计、PRD、Plan、长期调研和模板各自保留独立文件与导航。原始需求、确认、文件名及稳定 ID 不因整理而删除或覆盖；引用修复不得改变业务含义。技术合同、产品范围和实施步骤各有唯一维护位置，当前状态只在 BACKLOG。根规范和工程 README 保留原位置，docs 不复制 README，不添加脚本、测试、依赖或生成工程。本机编辑器配置和第三方版权不随整理覆盖或删除。

提交遵循 AGENTS；只包含本任务，报告检查结果、未决项与工作区状态。未经用户要求不提交、推送或发布，不改外部任务系统状态。
