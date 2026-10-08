# 浮光前端

当前设计直接运行在正式产品路由：`/` 为首页，其他页面见[页面需求总览](../docs/requirement/浮光页面/index.md)。不存在独立预览目录、旧工作台或旧 API 代理。页面代码位于对应 `src/app/<路由>/_components/`，使用共享 shadcn/Radix 组件与 [DESIGN](../DESIGN.md) 语义颜色。

功能和数据暂用 mock，编辑仅影响当前页面状态，刷新恢复；认证、持久化、真实供应商调用和费用尚未接入。mock 不改变正式入口的身份。

复用组件按 `src/components/<feature>/` 组织。页面实现、独有组件、局部状态和交互测试放在对应 `src/app/<路由>/_components/`，例如 `app/models/_components/models-page.tsx`；首页使用 `app/_components/`。App Router 直接导入对应页面实现，基础组件继续使用 `components/ui/`。目录与依赖约束见 [PROJECT](../PROJECT.md#2-可继续使用的工具)。

## 开发

在本目录执行 `pnpm install --frozen-lockfile`，再执行 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`。Docker 前端访问 <http://127.0.0.1:3200/>；前端不依赖历史后端服务。质量门禁见 [PROJECT](../PROJECT.md#4-质量门禁)。

第三方许可保留在 `public/licenses/`；历史实现来源说明只用于版权追溯，不作为新功能需求。
