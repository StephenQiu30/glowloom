# Workspace 官方 Fumadocs 设计

## 1. 需求来源

唯一来源：[需求30](../requirement/30-Workspace官方Fumadocs接入需求.md)。用户要求保留 content，删除旧实现，直接接入正式 Fumadocs。

## 2. 目标与边界

提供一个免登录的项目知识库。原文、稳定 ID、已有引用保持可用；Skill 和工作流作为知识内容阅读。历史设计43中的执行、提案、认证、PG、朗读和双应用方案不再是当前功能。

## 3. 技术选择

直接采用官方 `create-fumadocs-app` 的 Next.js、Fumadocs UI、Fumadocs MDX 与官方内置搜索（`fumadocs-core/search/server` 经 `/api/search` 提供检索，不依赖外部 Meilisearch 容器）。Fumadocs UI 使用官方 Base UI 实现及 shadcn 主题；不复制重写布局、侧栏、目录、搜索或 Markdown 渲染器。自有 TS 和测试遵循根 `AGENTS.md` 与 [workspace/AGENTS.md](../../AGENTS.md)，Server／Client 边界及异步加载遵循 Vercel 最佳实践。

## 4. 内容与路由

按 2026-11-16 补充需求，Fumadocs MDX 的 `defineDocs({ dir: 'content' })` 直接加载 `workspace/content/`，分类与首页均位于该目录下；不保留额外 `content/docs/` 层。官方 loader 的 `baseUrl` 仍为 `/docs`，页面相对路径和元数据层级保持原有结构。GitHub 页面入口指向仓库的 `workspace/content/<page.path>`。API 依据：[Fumadocs collections](https://fumadocs.dev/docs/mdx/collections)，具体类型以安装版本为准。

`content/` 是唯一知识正文目录，现有 Markdown 按 `requirement → design → prd → plan` 分层并辅以 `research`、`operations`、`templates`。缺少 frontmatter 的标题由 Fumadocs collection schema 从现有 H1 取值；历史 `/read` 与 `/read/*` 入口由 Next.js 原生重定向至 `/docs`；面向 Agent 的原文与索引通过 `/llms.txt`、`/llms-full.txt`、`/llms.mdx/[...path]` 及 `proxy.ts` 的 `Accept: text/markdown` 协商提供。根规范和业务代码仍由原工程维护，通过相对路径指向其源文件。

文件按分类使用唯一的 `NN-名称` 序号，保留历史空缺；日期放在序号之后。编号只维护在文件名，由官方 collection schema 派生到侧栏、页面标题、搜索和元数据；原文 H1 与锚点保留。部署配置归 `operations`；工具生成的安装设置、临时计划和规格不进入发布源、页面树或搜索。

## 5. 知识库界面

使用官方 `DocsLayout` / `DocsPage`、导航树、TOC、相邻页、代码块、主题、移动侧栏及搜索弹窗。图表只采用官方 Mermaid recipe；普通 `.md` 内容不作为可执行 MDX。GFM 与显式 HTML 锚点交由官方内容编译扩展处理。

## 6. 构建与部署

只有一个标准 Next.js 应用和 `1130` 入口；构建预渲染阅读页、原文和 LLM 索引，部署运行 `next start`（Docker standalone 镜像）。搜索由应用内 `/api/search` 提供，无需独立外部搜索服务或管理密钥同步。没有第二套静态导出应用、在线协作后端或发布选版器。

## 7. 与业务工程协作

文档修改通过 Git；Agent 直接读取 `workspace/content/` 与仓库规范，也可通过 `/llms.txt` 与 `/docs/*.md` 获取机器可读正文。知识库完整度通过 `pnpm docs:check` 在本地与 Workspace CI 自动门禁校验（覆盖编号唯一性、分类导航覆盖、PRD/Plan 唯一来源追溯与相对链接有效性）。知识站不接管代码执行、授权、数据库或研发任务。无登录输入、PG、主机桥、TTS、额外运行账户和服务端会话。

## 8. 验收

按 [PRD31](../prd/31-Workspace官方Fumadocs.md) 验证原文和 ID、旧链接、图表、真实搜索、键盘及移动端。构建和浏览器通过分别记录；并行业务及文档修改保持原样。具体步骤见唯一实施计划 [Plan40](../plan/40-Workspace官方Fumadocs执行计划.md)。
