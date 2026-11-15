# 织光 Workspace

采用官方 `create-fumadocs-app` Next.js 模板，使用 Fumadocs UI、Fumadocs MDX、官方 Meilisearch 搜索弹窗，以及 shadcn 的 Base Nova／Neutral 主题。页面布局、侧栏、目录、代码块、相邻导航和搜索交互均由 Fumadocs 提供。

## 运行

```bash
pnpm install --frozen-lockfile
# 按 .env.example 在本机配置 .env.local
pnpm build
pnpm start
```

唯一入口：`http://127.0.0.1:1130/`。直接阅读与搜索，不需要登录或访问令牌。编辑 Markdown 可用 `pnpm dev`；正式构建后，官方同步入口会更新 Meilisearch 的 `docs` 索引。

浏览器仅使用限定 `docs` 索引、仅允许 search 的查询密钥。管理密钥只放本机 `.env.local` 或发布环境，不进入 `NEXT_PUBLIC_*`，不提交 Git。

## 内容

- 原有 `content` 文件保持原文，Skill／工作流原文归入 `content/skills`、`content/workflows`，作为普通知识文档保留。
- `content/document-ids.json` 仅保存稳定 ID、文件和旧地址；通过 Fumadocs 的 slugs 配置沿用 `/read/<id>`。历史地址使用 Next.js 原生重定向。
- 原始 Markdown 通过官方 Markdown／LLM 路由提供，CSV 使用 public 只读文件链接下载。`/llms.txt` 和 `/llms-full.txt` 使用模板自带实现。
- 历史文档中的自研协作与双产物方案仅作追溯；当前实现不包含旧登录、PG、提案、执行桥、任务运行器、自研文档引擎或 TTS。

## 检查

```bash
pnpm lint
pnpm types:check
pnpm format:check
pnpm build
pnpm test:e2e
```

运行浏览器验收前先启动站点。CI 使用独立 Meilisearch，不需要业务数据库。部署使用标准 Next.js 构建／启动，Vercel 的 Root Directory 设为 `workspace`；没有自研发布目录、选版器或第二站点。

官方资料：[Next.js 模板](https://fumadocs.dev/docs/next)、[UI](https://fumadocs.dev/docs/ui)、[内容源](https://fumadocs.dev/docs/mdx)、[Meilisearch](https://fumadocs.dev/docs/search/meilisearch)。

部署到 Vercel 时，Meilisearch 地址必须是浏览器可访问的 HTTPS 地址，不能沿用本机 127.0.0.1；查询密钥和管理密钥分别配置为构建环境变量。这里提供标准 Next.js 部署，不额外维护纯静态导出包。
