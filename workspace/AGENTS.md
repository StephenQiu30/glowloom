# Workspace 知识库规范

本文件适用于 `workspace/` 及其知识正文，继承[仓库工程规范](../AGENTS.md)。知识库站点设计与验收边界由[设计45](content/design/45-Workspace官方Fumadocs设计.md)维护，正文维护规则由[编写规范](content/templates/05-编写规范.md)维护。

## 项目约定

1. `workspace/` 严格保持官方 `create-fumadocs-app` 目录结构与约定，内容入口通过官方 collection、loader 和页面元数据维护，布局、目录、搜索和 Markdown 协商沿用官方实现。
2. 顶层项目文档为服务根目录的 [ARCHITECTURE](ARCHITECTURE.md) 与 [PROJECT](PROJECT.md)。专题正文在 `workspace/content/`（按 **requirement → design → prd → plan** 分层，辅以 `research`、`operations`、`templates`）。仓库级协作和视觉规范由根 AGENTS 与 DESIGN 维护。
3. 严禁在 `workspace/` 内接入业务数据库、认证会话、任务执行器或脚本沙箱；保持 `/llms.txt`、`/llms-full.txt`、`/llms.mdx/[...path]` 与 `Accept: text/markdown` 协商可用。
4. 质量门禁：在 `workspace/` 执行 `pnpm docs:check`、`pnpm lint`、`pnpm types:check`、`pnpm format:check`、`pnpm build` 与 `pnpm test:e2e`。

## Fumadocs

This site is built with [Fumadocs](https://fumadocs.dev) on Next.js. The APIs and conventions of Fumadocs may differ from your training data, so check its docs for the task at hand:

- Writing pages: https://fumadocs.dev/docs/next/markdown.md
- Organizing pages and folders: https://fumadocs.dev/docs/next/page-conventions.md
- Customizing the UI: https://fumadocs.dev/docs/next/guides/customize-ui.md
- Changing the theme and colors: https://fumadocs.dev/docs/ui/theme.md
- Upgrading Fumadocs: https://fumadocs.dev/docs/next/upgrade.md
- Other tasks: https://fumadocs.dev/llms.txt lists every docs page.
