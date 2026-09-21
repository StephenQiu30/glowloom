# Frontend

本服务负责创作界面、交互和服务端状态展示；正式边界见 [PROJECT](../PROJECT.md)，本轮整改依据与验收见 [前端规范化实施设计](../docs/design/前端规范化实施设计.md)。

## 目录职责

| 目录                                | 职责                                                                             |
| ----------------------------------- | -------------------------------------------------------------------------------- |
| `src/app`                           | Next.js 路由、Provider、主题与路由反馈装配                                       |
| `src/components/<业务>`             | 按 Feature 分类的业务组件、endpoint、专用 Hook 与展示模型；身份装配属于 identity |
| `src/components/ui`                 | 官方 shadcn/Radix 基础组件及统一 variant                                         |
| `src/components/studio`、`system`   | 不依赖业务模块的共享展示组件                                                     |
| `src/layout`                        | 独立的 BasicLayout、BasicHeader、BasicFooter、LayoutContainer 与主题切换         |
| `src/api/generated`                 | 根据在线 Swagger 生成的 API；不得手改                                            |
| `src/lib`                           | 请求、状态、权限、主题等明确命名的基础设施                                       |
| `tests/architecture`、`unit`、`e2e` | 依赖边界、组件/业务契约、真实浏览器流程                                          |

当前 BasicLayout 使用顶部导航，页面统一通过它组合 Header、主内容和 Footer；身份查询由 `components/identity/studio-shell.tsx` 完成，通过属性注入布局。后续有侧边导航页面时可以增加独立侧边布局，复用结构组件；目前不预建无消费者的实现。

不设置 `features/` 目录。每个业务目录内通过文件职责区分 UI、endpoint 和 Hook；endpoint/Hook 不反向导入 UI，基础组件与布局不依赖业务；跨业务仅调用明确公共入口。架构测试检查边界与循环，不建立迁移兼容层。

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
