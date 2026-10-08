# 浮光工程规则

需求基线以 [需求总览](docs/requirement/01-需求总览.md) 为准；当前按业务模块整理需求，具体目标方案在实施前另行评审。页面与设计稿用于需求追踪，页面完成不等于功能完成。旧页面、API、Schema、模块拆分、状态机和中间件拓扑不得反向约束当前设计。旧工程资料已删除，必要核查从Git历史读取，不作为当前需求来源。

## 1. 当前阶段与文件职责

正式前端页面已替换，功能与数据仍以 mock 为主，真实业务尚未逐项验收。当前整理原始需求，不执行数据库 DDL。`docs/`维护原始产品需求、包含功能/非功能/验收的业务需求文件及生命周期方法；已形成含架构、库表约束、API/DTO及生命周期工程合同的[整体与模块需求Design](docs/design/00-项目需求设计.md)，当前仍没有已接受的新数据/接口设计。需求获取、评审/基线、Design、实施、测试、发布、运行维护及退役遵循[生命周期方法](docs/软件生命周期与需求管理.md)。文档直接通过 Markdown、Git 和 Obsidian 管理。AGENTS负责协作规则，根DESIGN负责本次视觉与交互，docs/design负责待评审项目技术方案；BACKLOG只维护当前事项。

## 2. 可继续使用的工具

前端现有 Next.js App Router、React、TypeScript strict、pnpm、Tailwind、shadcn/Radix、ESLint/Prettier；后端现有Go、Gin、GORM、Wire、swag。安装版本以包清单和lockfile为准。继续使用工具不等于继续使用旧业务合同或要求所有旧依赖进入新运行架构。

业务按职责组织，`adapter → application → domain`，依赖显式注入，接口由消费方定义。编码与审查遵循AGENTS的Google Go与Vercel/Next.js规范；不预建未来目录、通用框架或空转发层。

前端复用组件按 feature 放在 `frontend/src/components/<feature>/`；`components/ui/` 保留 shadcn/Radix 基础组件。当前跨页面复用内容按 auth、storyboard、layout、forms、controls、feedback 和 media 归属维护，直接导入具体文件，不使用产品拼音目录或统一导出层。

页面与页面独有组件放在对应 App Router 页面目录：`src/app/<路由>/page.tsx` 负责入口与元数据，`src/app/<路由>/_components/` 存放页面实现、局部状态、私有组件、mock 和交互测试；首页使用 `src/app/_components/`。不额外建立 `src/features/` 或另一套 Pages Router。共享认证视图服务登录、注册和重置密码三个页面；共享镜头 mock 位于 `components/storyboard/`，音频波形位于 `components/media/`。复用组件不得反向导入路由页面或页面私有组件；页面不得从其他页面模块导入共用数据或控件。

## 3. 数据与契约

新请求/响应由当前设计决定。实施时在Handler/DTO与注解定义，swag生成公开规范，再由openapi2ts生成前端调用；生成文件禁止手改。旧Swagger和客户端不是新需求来源，不为保留旧路由增加兼容层。

新表获接受后在实施阶段维护唯一最终态Schema。旧 `backend/db/schema.sql` 只记录当前运行代码事实，不能用其中旧枚举/宽度否决新页面。已有数据库迁移另行评审；不自动重建、清理或覆盖用户数据。

配置/凭据不进文档、日志、前端包和提交；只读取任务所需文件。不设置额外项目脚本或Makefile，检查使用现有工具直接命令。

## 4. 质量门禁

| 范围     | 适用检查                                                                                                                                                                          |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go实现   | gofmt、goimports、go vet、golangci-lint、go test -race、govulncheck；对应生成一致性                                                                                               |
| 前端实现 | 在frontend执行 pnpm exec eslint .、pnpm exec prettier --check .、pnpm exec next typegen、pnpm exec tsc --noEmit、pnpm exec vitest run、pnpm exec next build；交互变化补浏览器验证 |
| 文档     | 相对链接、锚点与来源路径；Obsidian 配置路径；许可原文字节；使用编辑器或 Obsidian 实际阅读                                                                                         |

Markdown 更改检查引用与内容，Obsidian 设置更改检查 JSON 和实际目录。目录迁移核对文件清单与原文，第三方许可保持原文字节。本文不授权跳过真实业务验收，未执行项照实报告。

## 5. 本地文档与交付

文档入口为 [docs/README.md](docs/README.md)，正文直接在既有分类目录维护。Obsidian 使用仓库根 Vault，保留标准 Markdown 相对链接、模板与图谱；默认 Compose 只保留业务前后端服务。详情见 [文档与 Obsidian 说明](docs/KNOWLEDGE.md)。

提交遵循AGENTS；只包含本任务，交付报告SHA、检查结果、未决问题与工作区状态。当前正式前端已由 Figma 页面替换；旧前端模块已删除。后端和现有数据尚未变更，后端清理范围需单独确认。
