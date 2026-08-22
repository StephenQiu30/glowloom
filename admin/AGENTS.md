# AGENTS.md

## 项目

Ant Design Pro 管理端，基于 Umi Max v4、Ant Design v6 和 ProComponents v3。

## 常用命令

`pnpm start`（开发环境与 mock）、`pnpm run dev`（不启用 mock）、`pnpm run build`（构建）、`pnpm run lint`（Biome 与 TypeScript 检查）、`pnpm run test`（Vitest）、`pnpm exec antd lint ./src`（Ant Design 专项检查）。

其他命令：`pnpm run openapi`（重新生成 `src/services/`）、`pnpm run biome`（自动修复）、`pnpm run tsc`（仅类型检查）。

## 关键约束

- 不要直接编辑 `src/services/ant-design-pro/` 中的自动生成文件；需要变更时使用 `pnpm run openapi` 重新生成。
- 只使用 Biome，不使用 ESLint 或 Prettier。提交前应通过 `pnpm run lint` 与 `pnpm exec antd lint ./src`。
- 编写 Ant Design 代码前，先使用 `pnpm exec antd info <Component>` 查询组件 API。
- 提交信息遵循 Conventional Commits 规范。
- 使用 TypeScript strict、Node.js 22 及以上版本和 `pnpm-lock.yaml`。
- `.umi` 为自动生成目录；开发服务异常时可删除 `src/.umi` 后重启。

## 架构要点

**配置**：`config/config.ts` 使用 `defineConfig`，`config/routes.ts` 使用声明式路由；路由 `name` 是静态菜单名称，`access` 控制可见性。

**约定文件**（`src/`）：`app.tsx`（运行时配置与 `getInitialState`）、`access.ts`（权限）、`global.tsx`（副作用）、`loading.tsx`、`typings.d.ts`。

**认证**：`getInitialState()` 请求 `GET /api/currentUser`；401 时跳转登录页；`access.ts` 根据当前用户权限控制管理功能。

**状态**：全局模型使用 `useModel('filename')`，当前用户与设置使用 `useModel('@@initialState')`；大多数数据加载使用 ProTable 的 `request` 属性；复杂服务端状态使用 `@tanstack/react-query`。

**样式优先级**：Tailwind CSS v4（布局）→ antd-style v4 / `createStyles`（主题令牌）→ CSS Modules → Less（仅兼容旧代码）。

**请求**：使用 `@umijs/max` 内置的 `request`，统一配置在 `src/requestErrorConfig.ts`；非自动生成 API 放在对应页面的 `service.ts`。

**页面组织**：每个页面目录按职责放置 `index.tsx`、可选的 `service.ts`、`data.d.ts` 和样式文件；页面专属代码与页面保持同目录。

## 开发准则

- 开始编码前明确假设、范围和验收条件；存在歧义时先说明。
- 只实现当前需求，避免为未来场景增加抽象或兼容层。
- 只修改与当前任务直接相关的代码，清理由本次修改产生的未使用导入和变量。
- 以可验证结果为目标：为可测试的行为补充测试，并在提交前运行相关检查。
