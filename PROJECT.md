# 浮光工程规则

需求与目标设计以 [浮光新页面](workspace/content/requirement/浮光页面/index.md)、[新数据模型](workspace/content/design/浮光页面数据设计.md)、[新接口](workspace/content/design/浮光页面接口设计.md) 为准。旧页面、API、Schema、模块拆分、状态机和中间件拓扑不得反向约束当前设计。旧工程说明保存在 `workspace/history/engineering/`，只在明确的迁移/旧环境任务中读取。

## 1. 当前阶段与文件职责

当前按用户要求直接替换正式前端页面，允许功能与数据使用 mock；不执行数据库 DDL。`workspace/content/` 只发布当前设计、验收与许可；`workspace/history/` 不进入文档站导航和搜索。AGENTS负责协作规则，DESIGN负责本次视觉与交互；BACKLOG只维护当前事项。

## 2. 可继续使用的工具

前端现有 Next.js App Router、React、TypeScript strict、pnpm、Tailwind、shadcn/Radix、ESLint/Prettier；后端现有Go、Gin、GORM、Wire、swag。安装版本以包清单和lockfile为准。继续使用工具不等于继续使用旧业务合同或要求所有旧依赖进入新运行架构。

业务按职责组织，`adapter → application → domain`，依赖显式注入，接口由消费方定义。编码与审查遵循AGENTS的Google Go与Vercel/Next.js规范；不预建未来目录、通用框架或空转发层。

## 3. 数据与契约

新请求/响应由当前设计决定。实施时在Handler/DTO与注解定义，swag生成公开规范，再由openapi2ts生成前端调用；生成文件禁止手改。旧Swagger和客户端不是新需求来源，不为保留旧路由增加兼容层。

新表获接受后在实施阶段维护唯一最终态Schema。旧 `backend/db/schema.sql` 只记录当前运行代码事实，不能用其中旧枚举/宽度否决新页面。已有数据库迁移另行评审；不自动重建、清理或覆盖用户数据。

配置/凭据不进文档、日志、前端包和提交；只读取任务所需文件。不设置额外项目脚本或Makefile，检查使用现有工具直接命令。

## 4. 质量门禁

| 范围     | 适用检查                                                                                                                                                                          |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Go实现   | gofmt、goimports、go vet、golangci-lint、go test -race、govulncheck；对应生成一致性                                                                                               |
| 前端实现 | 在frontend执行 pnpm exec eslint .、pnpm exec prettier --check .、pnpm exec next typegen、pnpm exec tsc --noEmit、pnpm exec vitest run、pnpm exec next build；交互变化补浏览器验证 |
| 文档     | 相对链接/锚点/来源路径；下列文档测试、构建与索引；当前页面实际阅读                                                                                                                |

```bash
pnpm --dir workspace exec tsx --test 'tests/*.test.ts'
pnpm --dir workspace exec next build --webpack
pnpm --dir workspace exec pagefind --site .next/server/app --output-path public/_pagefind
```

文档站工具代码变化再执行workspace的eslint、prettier、next typegen和tsc；仅Markdown更改按内容与构建验证。本文不授权跳过真实业务验收，未执行项照实报告。

## 5. 本地文档与交付

文档服务默认3210；开发命令 `pnpm --dir workspace exec next dev --webpack --hostname 127.0.0.1 --port 3210`。生产构建与搜索索引需同步更新，构建不等于真实功能完成。详情见 [知识库说明](workspace/content/KNOWLEDGE.md)。

提交遵循AGENTS；只包含本任务，交付报告SHA、检查结果、未决问题与工作区状态。当前正式前端已由 Figma 页面替换；旧前端模块已删除。后端和现有数据尚未变更，后端清理范围需单独确认。
