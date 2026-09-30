# 前端协作规范

本目录同时遵循根目录 [AGENTS.md 的前端工程规范](../AGENTS.md#前端工程规范) 与 [PROJECT.md 的前端约定](../PROJECT.md#6-前端)。

编写、修改或审查 React / Next.js 代码时，必须使用 Vercel 插件的 `vercel:react-best-practices`、`vercel:nextjs` 技能，读取与当前改动有关的规则；优先检查请求瀑布、客户端包体积和服务端/客户端边界，再检查取数、状态、重复渲染、可访问性和验证证据。具体 API 以当前安装版本文档为准。

<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->
