# Workspace 官方 Fumadocs 执行计划

唯一来源 PRD：[PRD31](../prd/31-Workspace官方Fumadocs.md)。本文定义步骤和验证方式；进度与回执只在根 BACKLOG。

## 1. 保护与盘点

记录原 content、工作树、稳定 ID、旧链接和运行进程。将旧 Workspace 实现精确备份到仓库外；保护已有并行改动、部署模板删除和 Meilisearch 的活动数据目录。备份不得提交 Git，不打印凭据。

## 2. 官方隔离接入

在仓库外运行官方 create-fumadocs-app，选择 Next.js／Fumadocs MDX／Meilisearch。沿用官方 UI 和 shadcn 主题，导入原 content；通过官方 schema、slugs、MDX 扩展及 Next.js 配置衔接旧内容。先验证隔离候选，不复制旧 src、测试或运行器。

## 3. 文档与工程替换

保留当前主工作树 content 原位，只加入新增元数据、Skill／流程原文及本轮需求链。旧实现、构建产物、缓存和凭据归档到仓库外；保留活动 Meilisearch 数据绑定。移除仅指向旧 Skill 执行目录的发现链接。更新 README、PROJECT 和 Workspace CI；不修改业务实现或数据库结构。

目录整理时先备份并移除工具过程文档及身份索引，将 COS 迁入 operations，补齐文件序号和导航；按已知文件映射更新相对引用及 CSV 链接。由官方 schema 派生标题编号，保持旧 ID 与锚点。检查编号唯一、无缺失身份与断开引用，并确认删除内容退出真实搜索索引。

品牌标识补齐时复用仓库已确认的 SVG／favicon／Apple 图标，独立放入 Workspace，通过官方导航插槽与 Next.js 原生图标约定接入；核对桌面、手机与深浅主题的真实加载。

## 4. 主工程门禁

在实际 Workspace 目录重新安装依赖并构建，不复用隔离目录的 Next 产物。执行 pnpm lint、pnpm types:check、pnpm format:check、pnpm build。官方搜索同步连接专用 Meilisearch，浏览器只用 docs 的 search-only key。

## 5. 启动与接受

核对进程归属后停止旧知识站及旧桥，启动标准 Next.js 1130服务；停止隔离预览。执行 pnpm test:e2e，逐项覆盖 PRD 的 AC-FD-01～09；再对全部旧阅读 ID、Markdown raw、CSV 和显式锚点检查。核对原文哈希、并行工作树及旧实现退出情况。

## 6. 交付边界

仅报告已执行的本机检查。远程 CI、生产部署、提交与推送按用户当次授权处理；用户随后已授权仅将本次官方 Fumadocs 重建及相关文档、CI 修改提交到 main 并推送；保留其他并行修改和部署模板删除。发现内容并行变化时保留最新内容并重新构建，不恢复旧快照。
