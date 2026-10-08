# 前端协作规范

本目录同时遵循根目录 [AGENTS.md 的前端工程规范](../../../../../AGENTS.md#前端工程规范) 与 [PROJECT.md 的前端约定](../../../../../PROJECT.md#6-前端)。

编写、修改或审查 React / Next.js 代码时，必须使用 Vercel 插件的 `vercel:react-best-practices`、`vercel:nextjs` 技能，读取与当前改动有关的规则；优先检查请求瀑布、客户端包体积和服务端/客户端边界，再检查取数、状态、重复渲染、可访问性和验证证据。具体 API 以当前安装版本文档为准。

业务 API 只能由 `@umijs/openapi` 从后端在线规范生成到 `src/api/`，禁止手写或修改生成文件。业务代码和 Route Handler 只调用生成函数，不拼接接口地址或直接使用网络客户端；Axios 和授权媒体流传输集中在 `src/lib/request.ts`。接口缺失时先补后端注解与 DTO，再重新生成，禁止添加临时手写请求。修改后须通过请求边界 ESLint、生成一致性与适用功能验证。

<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->
