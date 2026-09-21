# Frontend

本服务负责创作界面、交互和服务端状态展示；正式边界见 [PROJECT](../PROJECT.md)，本轮整改依据与验收见 [前端规范化实施设计](../docs/design/前端规范化实施设计.md)。

## 目录职责

| 目录                                        | 职责                                                                          |
| ------------------------------------------- | ----------------------------------------------------------------------------- |
| `src/app`                                   | Next.js 路由、全局 Provider、主题、加载与错误边界                             |
| `src/features/<业务>`                       | 页面实现、业务交互、RTK Query endpoints；身份会话与 StudioShell 属于 identity |
| `src/components/ui`                         | 官方 shadcn/Radix 基础组件及统一 variant                                      |
| `src/components/layout`、`studio`、`system` | 不依赖 Feature 或 API 的共享展示组件                                          |
| `src/api/generated`                         | 根据在线 Swagger 生成的 API；不得手改                                         |
| `src/lib`                                   | 请求、状态、权限、主题等明确命名的基础设施                                    |
| `tests/architecture`、`unit`、`e2e`         | 依赖边界、组件/业务契约、真实浏览器流程                                       |

业务请求归属 Feature。跨 Feature 仅调用明确公共入口，架构测试检查依赖方向和循环；不以新增 index、utils 或转发层隐藏依赖。

## UI 约定

使用 `components.json` 配置的官方 Radix registry；新增组件前执行 `pnpm dlx shadcn@latest docs <组件>`，新增后检查 diff，不能批量覆盖本地样式。表单使用 Field 系列；复杂校验使用 React Hook Form 与 Zod。Table 使用 shadcn Table，选择和展开使用 Select、RadioGroup、Collapsible。基础层可以使用实现组件所必需的原生标签，业务层保留 form 的提交语义和必要的布局语义标签；Radix 并不提供所有 HTML 元素的替代品。

页面错误使用 Alert，字段错误使用 FieldError，短时成功使用 Sonner；空态和加载使用 Empty、Skeleton、Spinner。使用语义颜色、variant、size，保持明暗主题与无边框视觉规范。ESLint 限制业务层重复实现原生交互控件。

## 开发与检查

```sh
pnpm install --frozen-lockfile
pnpm dev
pnpm format:check
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm test:e2e tests/e2e/platform-health.spec.ts tests/e2e/frontend-normalization.spec.ts
```

API 生成入口为 `pnpm openapi`，由 `openapi2ts.config.ts` 读取在线契约；不恢复已删除的 `scripts/` 入口。当前旧 API 调用仍有迁移遗留，须配合 Go 注解和响应 schema 完整化，不能把现有生成目录视作全量迁移完成。

浏览器测试通过 `playwright.config.ts` 启动独立服务与测试数据，相关系统工具须可用。上述两个用例不调用真实模型；分集生成验收需要单独具备真实 Agent/模型条件，不能与界面验收混淆。
