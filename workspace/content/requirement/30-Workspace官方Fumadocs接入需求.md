# Workspace 官方 Fumadocs 接入需求

来源：2026-11-15 用户明确要求。

> 除了保留我们的content 内容之外，删除所有的实现，然后重新使用正式的 接入我们的 Fumadocs 服务，不要自己从0开始做

本轮采用官方 `create-fumadocs-app` 的 Next.js／Fumadocs UI／Fumadocs MDX 模板及官方 Meilisearch 搜索接法。原有 content 文件保留；旧 Skill／工作流仅保留为普通文档。稳定文档 ID、历史链接及原始 Markdown 通过官方内容源、Markdown 路由和 Next.js 配置衔接。

删除旧自研文档组件、构建／选版机制、登录、PG、提案、主机桥、任务运行器及 TTS 实现。日常仅1130一个免登录阅读入口，不创建另一套静态或在线应用。旧设计43、PRD29与Plan38的自研实现作为历史方案保留，不再作为当前服务功能清单。

当前配置与启动以 [Workspace README](../../README.md) 和 [Fumadocs 官方文档](https://fumadocs.dev/docs/next) 为准；实施进度与验证记录仍只在根 BACKLOG。

当前来源链：[设计45](../design/45-Workspace官方Fumadocs设计.md) → [PRD31](../prd/31-Workspace官方Fumadocs.md) → [Plan40](../plan/40-Workspace官方Fumadocs执行计划.md)。
