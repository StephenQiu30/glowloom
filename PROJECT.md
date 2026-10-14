# 织光工程规则

项目为 Glowloom（织光），仓库 `StephenQiu30/glowloom-server`；内部为 `frontend/`、`backend/` 和 `docs/`。Go 模块使用 `github.com/StephenQiu30/glowloom-server/backend`。现有 `LV_*` 配置、运行资源与数据库名称以实际部署为准，不因文档整理变更。

## 1. 当前阶段与文件职责

本次工作为邮箱认证和用户体系需求整理。用户已确认邮箱 + 密码、注册验证邮箱，以及平台管理员/团队管理员双层管理；个人与团队创作者共用身份。现有实现仍以登录名和单一组织范围角色认证，目标团队模型尚未实现。当前用户模块事实与差异见[实现盘点](docs/design/01-用户系统总体设计.md#1-实现盘点)，不能把目标设计写成已实现状态。

[文档索引](docs/00-文档索引.md) 为文档入口；文档链路统一为 **requirement → design → prd → plan**：requirement 定义需求、规则与验收来源，design 将需求转为交互及技术方案，prd 基于需求和设计汇总用户价值、交付范围与接受条件，plan 按 PRD 范围和设计拆解任务、依赖与验证；各目录正文按顺序编号。AGENTS 负责长期协作规则，根 DESIGN 负责共享视觉与交互，BACKLOG 只维护当前事项。未重新评审的其他业务能力保留在[产品范围](docs/requirement/00-产品范围.md)中，不承诺排期或完成。

## 2. 可继续使用的工具

前端现有 Next.js App Router、React、TypeScript strict、pnpm、Tailwind、shadcn/Radix、ESLint/Prettier；后端现有Go、Gin、GORM、Wire、swag。安装版本以包清单和lockfile为准。继续使用工具不等于继续使用旧业务合同或要求所有旧依赖进入新运行架构。

业务按职责组织，`adapter → application → domain`，依赖显式注入，接口由消费方定义。编码与审查遵循AGENTS的Google Go与Vercel/Next.js规范；不预建未来目录、通用框架或空转发层。

前端复用组件按 feature 放在 `frontend/src/components/<feature>/`；`components/ui/` 只保留有消费者的 shadcn/Radix 基础组件。当前跨页面复用内容按实际业务职责归属维护，直接导入具体文件，不使用产品拼音目录或统一导出层。

页面与页面独有组件放在对应 App Router 页面目录：`src/app/<路由>/page.tsx` 负责入口与元数据，`src/app/<路由>/_components/` 存放页面实现、局部状态、私有组件和交互测试；首页使用 `src/app/_components/`。不额外建立 `src/features/` 或另一套 Pages Router。共享认证视图服务登录、注册和重置密码三个页面；公共灵感素材在 `components/inspiration/`，个人素材合同和预览在 `components/media/`。复用组件不得反向导入路由页面或页面私有组件；页面不得从其他页面模块导入共用数据或控件。

## 3. 数据与契约

- 按接受的设计维护 Handler/DTO 和注解，由固定版本 swag 生成规范，再由固定版本 @umijs/openapi 生成前端调用；`frontend/src/api/` 全部为生成文件，不手改，不在组件目录手写业务请求函数或维护另一套客户端。`frontend/openapi2ts.config.ts` 只声明生成配置，遵循 [@umijs/openapi 官方用法](https://github.com/chenshuai2144/openapi2typescript#usage)，不执行外部命令、切换进程目录或改写工具模板。在 `frontend/` 执行 `pnpm api:generate`：先运行 `api:schema` 将后端注解导出到忽略目录 `node_modules/.cache/openapi/swagger.json`，再运行官方 openapi2ts 和项目 Prettier。无需启动后端；生成需要 Go、Node 和已安装的前端依赖。`pnpm api:check` 独立重生成并逐文件核对、核对全部 operationId，发现手改、漏生成或额外文件即失败。
- 通用请求层为 `frontend/src/lib/request.ts`：使用 Axios 实例、原生 Axios 配置和拦截器，统一超时、幂等键、multipart 与 HTTP 错误转换，直接返回后端 payload。取消保留 Axios 的取消语义；传输层不导入 UI/认证组件，不弹窗或跳转。认证文案和会话校验位于 `components/auth/contracts.ts`。页面导入 `@/api/<标签>`，类型使用生成的 `API` 命名空间。下载传入自定义 options 时显式指定 `responseType: "blob"`；@umijs/openapi 1.14.1 原始模板只有在 options 未提供时使用 hook 默认值，不通过改写模板改变该行为。
- 后端不提供在线 Swagger 路由，不新增跟踪的 backend/docs 产物。生成合同检查与实际路由双向核对。
- `backend/db/schema.sql` 是当前实现的最终态 Schema；既有数据库另行审阅增量 SQL，不重放、重建或清理业务数据。邮箱迁移与身份引用保护见[用户系统设计](docs/design/01-用户系统总体设计.md#8-d6-迁移与切换)。
- 浏览器请求经同站代理；服务端 `LV_BACKEND_ORIGIN` 不进入浏览器。凭据不进入文档、日志、前端包或提交。
- 默认本地工程验证不发起真实收费生成；fixture、协议握手或页面成功不能代替真实供应商验收。模型可用性以实际配置和适配能力为准。
- 本地 namespace 显式准备，不由应用自动创建；Go 工具链与依赖版本以 go.mod、Dockerfile 为准。开发后端镜像直接运行编译程序，代码变更后重新构建；不把修改文档解释为重启服务授权。
- API 生成使用上述 package.json 命令组织标准工具；其他检查使用现有工具直接命令，不新增 shell 脚本或 Makefile。

## 4. 质量门禁

| 范围     | 适用检查                                                                                                                                                                            |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go 实现  | gofmt、goimports、go vet、golangci-lint、go test -race、govulncheck；相应契约生成一致性                                                                                             |
| 前端实现 | 在 frontend 执行 pnpm exec eslint .、pnpm exec prettier --check .、pnpm exec next typegen、pnpm exec tsc --noEmit、pnpm exec vitest run、pnpm exec next build；交互变化补浏览器验证 |
| 文档     | Markdown 格式、相对链接与锚点、来源路径、需求/验收对应关系、陈述与源码一致；第三方许可原文保持不变                                                                                  |

验证范围与修改风险匹配。文档变更不需要重跑无关业务测试；UI/工程检查、隔离存储、真实邮件、实际数据库迁移和产品接受分别记录。缺少外部条件或未执行的检查不得报告通过。

## 5. 本地文档与交付

直接维护标准 Markdown，以 Git 管理修订。docs 仅保留现行 requirement、design、prd 和 plan，不另设历史目录、重复模板或逐轮交付日志。本机编辑器配置和第三方版权归属不是产品历史需求，不随本次清理删除。

提交遵循 AGENTS；只包含本任务，报告检查结果、未决项与工作区状态。未经用户要求不提交、推送或发布，不改外部任务系统状态。
