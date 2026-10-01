# Lanverse 前端工程配置

2026-10-01 按用户要求清空现有全部前端实现，仅保留工程配置和已安装依赖。本目录当前没有页面、组件、画布、请求实现、生成客户端、测试或展示素材；旧 3000 服务已停止，没有替代页面。

保留 Next.js（App Router）+ React + TypeScript strict + Tailwind CSS + shadcn/ui（Radix）技术栈，以及包版本、pnpm 锁文件、lint、格式、测试、生成器和容器配置。目录与约定见仓库根目录 [PROJECT.md §6](../PROJECT.md)，清理边界见 [前端实现清理](../docs/design/前端实现清理设计.md)。

当前仅可验证剩余工程配置：

```bash
pnpm install --frozen-lockfile
pnpm exec eslint .
pnpm exec prettier --check .
```

Next.js 启动/构建、应用类型检查、Vitest、Playwright、生成客户端对比和前端镜像门禁在应用重建后恢复。`components.json`、`tsconfig.json`、测试和 API 生成器中的源码路径是保留的工程约定，当前目标尚不存在；它们不能作为应用可用的证据。

Prettier 保留 Tailwind 插件，但移除已删除的 `src/app/globals.css` 绑定；重建 Tailwind 4 样式入口后需重新指定 `tailwindStylesheet`。

Go API、Worker、Relay、Swagger、SQL 迁移及项目/画布/媒体业务数据保留。历史前端实现可由 Git 提交 `bd17655b` 查阅，本轮旧验收只对应清理前提交，不表示当前存在前端服务。
