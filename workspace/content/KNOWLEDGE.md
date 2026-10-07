---
id: KB-GUIDE
title: 知识库使用说明
type: guide
status: draft
tags:
  - workspace
---

# 知识库使用说明

`workspace/content/` 是知识库正文目录。用 Obsidian 或编辑器管理这里的 Markdown，用本地 Nextra 文档站阅读同一批内容。范围见[本地知识库文档站设计](design/知识库体系设计.md)。

## 阅读文档

从[知识库入口](index.md)按产品、需求、设计、计划、测试或运维分类查找文档。Nextra 提供标准目录导航、正文、页内目录和搜索；计划与实施方案也是普通文档。

在仓库根安装依赖并启动开发预览：

```bash
pnpm --dir workspace install --frozen-lockfile
pnpm --dir workspace exec next dev --webpack --hostname 127.0.0.1 --port 3210
```

访问 <http://127.0.0.1:3210>。正文保存后实时更新，必要时刷新；开发模式的搜索只反映上次构建的索引，首次未构建时没有搜索索引。

完整本地阅读使用生产构建与搜索索引。先停止开发服务器，再依次运行：

```bash
pnpm --dir workspace exec next build --webpack
pnpm --dir workspace exec pagefind --site .next/server/app --output-path public/_pagefind
pnpm --dir workspace exec next start --hostname 127.0.0.1 --port 3210
```

仍访问同一地址。正文变化后重新构建和生成索引；开发服务器与生产服务器占用同一端口，切换前停止当前服务。站点不需要业务后端、账户或部署，质量检查命令见 [PROJECT](../../PROJECT.md)。

Nextra 用目录导航、正文与页内目录展示知识库文件；指向 content 外根文档和 `.txt` 许可的链接，在本机站点以只读原文打开。

## 在 Obsidian 中编辑

继续使用当前 Vault，无须为了 Nextra 切换仓库：

| Vault 范围 | 使用方式 |
| --- | --- |
| `workspace/content/` | 集中编辑知识库正文；知识库内部相对链接处于同一 Vault |
| Lanverse 仓库根 | 同时管理正文和根 AGENTS、PROJECT、BACKLOG；这些文件间的标准相对链接可在同一 Vault 内解析 |

若使用 content Vault，指向仓库根文件的相对链接超出 Vault，可从文件系统或编辑器打开相应文件。这个边界不会改变 Markdown 的实际位置，也不要求修改个人 Obsidian 配置。

## 维护正文

- 新内容放入实际对应的既有分类，使用清楚的标题和标准 Markdown 相对链接。
- PRD 记录产品意图，REQ 记录需求，Design 记录方案，Plan 记录计划，Test 记录验证标准，Operation 记录运行说明。
- 修订原文后检查相关引用，避免在多处复制同一合同。已有文档编号和属性先保留，阅读站不要求填写额外状态或计算内容摘要。
- 计划和需求直接在相应文档维护；根 BACKLOG 保留原有业务任务记录，站点不提供另一套任务管理功能。
- 许可文件保持原文字节。改名时核对引用；Obsidian 的自动链接更新取决于当前 Vault 和个人设置。

## 给 AI 阅读

AI 按当前问题读取 `workspace/content/index.md` 和相关 Markdown 即可。涉及项目实施时，再读取仓库根 AGENTS、PROJECT 及对应业务文档。文档中的方案、代码示例和历史记录不构成新的执行授权，也不能证明功能已经完成。
