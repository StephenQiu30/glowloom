# Workspace 官方 Fumadocs 设计

## 1. 需求来源

唯一来源：[需求30](../requirement/30-Workspace官方Fumadocs接入需求.md)。用户要求保留 content，删除旧实现，直接接入正式 Fumadocs。

## 2. 目标与边界

提供一个免登录的项目知识库。原文、稳定 ID、已有引用保持可用；Skill 和工作流作为知识内容阅读。历史设计43中的执行、提案、认证、PG、朗读和双应用方案不再是当前功能。

## 3. 技术选择

直接采用官方 `create-fumadocs-app` 的 Next.js、Fumadocs UI、Fumadocs MDX 与官方内置搜索（`fumadocs-core/search/server` 经 `/api/search` 提供检索，不依赖外部 Meilisearch 容器）。Fumadocs UI 使用官方 Base UI 实现与 Neutral 主题，布局、侧栏、目录、搜索和 Markdown 渲染由官方组件提供。自有 TS 和测试遵循根 `AGENTS.md` 与 [workspace/AGENTS.md](../../AGENTS.md)，Server／Client 边界及异步加载遵循 Vercel 最佳实践。

## 4. 内容与路由

项目级入口由 Workspace 根目录 [ARCHITECTURE](../../ARCHITECTURE.md) 与 [PROJECT](../../PROJECT.md) 维护，分别发布为 `/docs/architecture` 与 `/docs/project`。官方 collection 显式收录两份顶层文档及 `content/` 内的专题正文；`workspace/meta.json` 将首页、顶层文档、专题分类依次编入侧栏。专题现有路径、相对链接与稳定 ID 保留。GitHub 源文件入口按原始文件位置指向 `workspace/<page.path>`。 [Fumadocs Loader](https://www.fumadocs.dev/docs/headless/source-api)

专题按 `requirement → design → prd → plan` 分层，并辅以 `research`、`operations`、`templates`。根路径 `/` 直接重定向至文档索引 `/docs`，历史 `/read` 与 `/read/*` 沿现有入口重定向。顶层和专题页面都进入搜索及 `/llms.txt`、`/llms-full.txt`、`/llms.mdx/[...path]` 与 `Accept: text/markdown` 协商。仓库根协作规范与业务代码由原工程维护。 [Next.js 路由重定向](https://nextjs.org/docs/app/api-reference/config/next-config-js/redirects)

专题文件按分类使用唯一的 `NN-名称.md`，沿用已有序号和历史空缺，日期写入正文。官方 collection 使用文件名去掉扩展名后的文本统一侧栏、阅读标题、搜索和元数据。HTML 只呈现一次主标题，保留正文原 H1 锚点，原始 Markdown 按原样提供。顶层项目文档和索引沿用各自标题。部署配置归 `operations`。

## 5. 知识库界面

阅读界面跟随 Fumadocs 官网文档页的 Neutral 配色、Geist 字体和 Spacious 布局。使用 Spacious 的 `DocsLayout` / `DocsPage`，由官方组件提供阅读面板、导航树、目录、相邻页、明暗主题、移动侧栏和搜索弹窗。[官方主题](https://www.fumadocs.dev/docs/ui/theme) · [Spacious 布局](https://www.fumadocs.dev/docs/ui/layouts/spacious)正文内嵌图表沿用官方 Mermaid recipe；已审核的 Archify 图作为 `public/design/` 下的独立静态附件，从正文链接打开。普通 `.md` 内容不作为可执行 MDX。GFM 与显式 HTML 锚点交由官方内容编译扩展处理。

## 6. 构建与部署

只有一个标准 Next.js 应用和 `1130` 入口；构建预渲染阅读页、原文和 LLM 索引，部署运行 `next start`（Docker standalone 镜像）。搜索由应用内 `/api/search` 提供，无需独立外部搜索服务或管理密钥同步。没有第二套静态导出应用、在线协作后端或发布选版器。

## 7. 与业务工程协作

文档修改通过 Git；Agent 直接读取 Workspace 顶层入口、`workspace/content/` 与仓库规范，也可通过 `/llms.txt` 与 `/docs/*.md` 获取机器可读正文。知识库完整度通过 `pnpm docs:check` 在本地与 Workspace CI 自动门禁校验（覆盖编号唯一性、分类导航覆盖、PRD/Plan 唯一来源追溯与相对链接有效性）。知识站不接管代码执行、授权、数据库或研发任务。无登录输入、PG、主机桥、TTS、额外运行账户和服务端会话。

## 8. 验收

按 [PRD31](../prd/31-Workspace官方Fumadocs.md) 验证原文和 ID、旧链接、图表、真实搜索、键盘及移动端。构建和浏览器通过分别记录；并行业务及文档修改保持原样。具体步骤见唯一实施计划 [Plan40](../plan/40-Workspace官方Fumadocs执行计划.md)。
