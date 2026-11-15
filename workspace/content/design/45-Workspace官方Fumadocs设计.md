# Workspace 官方 Fumadocs 设计

## 1. 需求来源

唯一来源：[需求30](../requirement/30-Workspace官方Fumadocs接入需求.md)。用户要求保留 content，删除旧实现，直接接入正式 Fumadocs。

## 2. 目标与边界

提供一个免登录的项目知识库。原文、稳定 ID、已有引用保持可用；Skill 和工作流作为知识内容阅读。历史设计43中的执行、提案、认证、PG、朗读和双应用方案不再是当前功能。

## 3. 技术选择

直接采用官方 create-fumadocs-app 的 Next.js、Fumadocs UI、Fumadocs MDX、Meilisearch 模板。Fumadocs UI 使用官方 Base UI 实现及 shadcn 主题；不复制重写布局、侧栏、目录、搜索或 Markdown 渲染器。自有 TS 和测试遵循 Google TS，Server／Client 边界及异步加载遵循 Vercel 最佳实践。

## 4. 内容与路由

content 是唯一知识正文目录，现有 Markdown／CSV 不重写。旧 Skill／工作流 Markdown 原文移入 content/skills 和 content/workflows。缺少 frontmatter 的标题由 Fumadocs collection schema 从现有 H1 取值。document-ids.json 仅登记身份及旧地址；官方 loader slugs 沿用 `/read/<id>`。历史入口由 Next.js 原生重定向承接，CSV 通过 public 中只读符号链接下载；Markdown 使用模板自带 LLM 路由。根规范和业务代码仍由原工程维护，旧引用指向其源文件。

## 5. 知识库界面

使用官方 DocsLayout／DocsPage、导航树、TOC、相邻页、代码块、主题、移动侧栏及搜索弹窗。原文中的 H1 映射官方 DocsTitle，避免重复标题。图表只采用官方 Mermaid recipe；普通 .md 内容不作为可执行 MDX。GFM 与显式 HTML 锚点交由官方内容编译扩展处理。

## 6. 构建与部署

只有一个标准 Next.js 应用和1130入口；构建预渲染阅读页、原文和 LLM 索引，部署运行 next start 或 Vercel Next.js。Meilisearch 是独立搜索依赖，构建后由官方同步代码更新 docs 索引。没有第二套静态导出应用、在线协作后端或发布选版器。浏览器只持有 docs 索引的 search-only key；管理密钥仅用于构建同步。构建需要可达的搜索实例。

## 7. 与业务工程协作

文档修改通过 Git；Agent 直接读取 content 和业务工程规范，产出由原业务工程测试和审查。知识站不接管代码执行、授权、数据库或研发任务。无登录输入、PG、主机桥、TTS、额外运行账户和服务端会话。旧实现及其本机凭据、缓存移到仓库外备份，已有 Meilisearch 数据目录留在原位。

## 8. 验收

按 [PRD31](../prd/31-Workspace官方Fumadocs.md) 验证原文和 ID、旧链接、图表、真实搜索、键盘及移动端。构建和浏览器通过分别记录；并行业务及文档修改保持原样。具体步骤见唯一实施计划 [Plan40](../plan/40-Workspace官方Fumadocs执行计划.md)。
