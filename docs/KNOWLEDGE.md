---
id: KB-GUIDE
title: 文档与 Obsidian 使用说明
type: guide
status: draft
tags: [docs, obsidian]
---

# 文档与 Obsidian 使用说明

项目文档统一保存在 `docs/`，使用 Markdown、Git 和 Obsidian 直接维护。从[文档入口](README.md)阅读当前原始需求、数据与接口设计；历史资料保留在 `docs/history/`，仅供明确的历史核查或迁移任务使用。目录与职责见[文档管理设计](design/知识库体系设计.md)。

## 在 Obsidian 中编辑

推荐在 Obsidian 中选择“打开文件夹作为仓库”，打开本项目根目录 `Lanverse/`。已有仓库重新打开即可加载更新后的配置。

- 根 `.obsidian/app.json` 将新笔记默认目录设为 `docs/`，使用标准 Markdown 相对链接，并启用重命名时自动更新链接。
- 根 `.obsidian/templates.json` 指向 `docs/templates/`；保留“文档”和“任务小节”模板以及原有日期、时间格式。
- 根 `.obsidian/graph.json` 聚焦当前文档与根协作文件，过滤历史、模板和许可目录；默认搜索同样过滤这些目录及业务代码。
- 大纲、属性、反向链接、标签和 Canvas 等已有核心插件保持原设置。
- 窗口布局、外观、插件、主题和快捷键等个人状态由 `.gitignore` 排除，继续留在本机。

也可以单独打开 `docs/` 作为 Vault。原正文 Vault 的个人配置已经随文件迁入 `docs/.obsidian/`；这种模式下，仓库根的 AGENTS、PROJECT、BACKLOG 等文件位于 Vault 外，从编辑器或文件系统打开。原来打开 `workspace/content/` 的用户需要改为打开 `docs/`，或改用推荐的根 Vault。

## 维护正文

- 新内容放在既有的 `prd/`、`requirement/`、`design/`、`plan/`、`test/` 或 `operation/` 分类，使用明确的标题和标准相对链接。
- PRD 记录产品意图，REQ 记录需求，Design 记录方案，Plan 记录计划，Test 记录验收，Operation 记录运行说明；按实际需要维护文档属性。
- 当前任务状态只在根 BACKLOG 维护；修订合同后检查相关引用，避免多处复制同一份内容。
- 移动或改名后检查相对链接、锚点和源码路径，Obsidian 的自动更新只覆盖当前 Vault 内的文件。
- 第三方许可保留原文字节；历史资料不自动恢复为当前需求或实施授权。
- 保存 Markdown 后即可从 Obsidian 或编辑器读取，以 Git diff 审阅修订并按任务提交。

## 给 AI 阅读

AI 从 `docs/README.md` 和任务相关 Markdown 开始，实施前再读根 AGENTS、PROJECT 与 BACKLOG。文档中的方案、代码示例和历史完成记录不构成新的执行授权，也不能证明当前功能已经通过验收。
