# 前端协作规范

本目录遵循根目录 [AGENTS](../AGENTS.md#前端工程规范)。工程工具与质量门禁以 [PROJECT](../workspace/PROJECT.md) 为准；不在协作文件中定义业务需求、接口形状或额外实施范围。

业务规则遵循 PROJECT 的服务端归属。页面与组件直接渲染领域接口回执。表单即时提示和请求调度属于界面职责，服务端承担最终校验。

样式按使用范围归属。全局样式容纳基础重置、主题、共享令牌和跨路由基础布局。路由或组件独有的规则放在其目录的样式文件或 CSS Module，由拥有它的组件导入。多个消费者共享的规则由共享组件或样式模块持有，使用统一设计令牌。

编写、修改或审查 React / Next.js 代码时，使用适用的 Vercel React Best Practices 与 Next.js 规则；具体 API 核对当前安装版本的文档。修改前核对用户已接受的设计、服务端/客户端边界和生成文件归属。

<!-- BEGIN:nextjs-agent-rules -->

# This is NOT the Next.js you know

This version has breaking changes — APIs, conventions, and file structure may all differ from your training data. Read the relevant guide in `node_modules/next/dist/docs/` (resolved from this file's directory; in monorepos the `next` package may not be visible from the repo root) before writing any code. Heed deprecation notices.

This block is written and re-added by `next dev` — verify at `node_modules/next/dist/server/lib/generate-agent-files.js`. Removing it from a diff only re-creates the uncommitted change; committing it with your work keeps the tree clean.

<!-- END:nextjs-agent-rules -->
