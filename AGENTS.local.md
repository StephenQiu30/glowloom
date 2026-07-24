# AGENTS.local.md

本文件用于记录放在具体项目中的局部规范性配置，与 `AGENTS.md` 中的全局协作规则进行区分。

## 使用边界

1. `AGENTS.md` 存放长期稳定的 Codex 全局规则、角色协作原则和交付格式。
2. `AGENTS.local.md` 存放当前项目特有的规范、路径、命令、环境约束和临时协作约定。
3. 当局部规范与全局规则冲突时，应优先确认项目上下文，并以更具体、更贴近当前项目的规则为准。

## 当前项目规范

1. 本项目内的角色配置放在 `.codex/agents/` 目录。
2. 本项目内的可复用流程放在 `.codex/skills/` 目录。
3. 本项目不再维护额外规格配置；`docs/` 目录保留正式分类结构、阶段文档和对应 README 索引。
4. 应用实现进入已接受的 Plan 后，根目录仅创建 `backend/`、`frontend/` 和 `deploy/`；API、Workflow、Worker 与供应能力适配器均归入 `backend/`，不得改用顶层 `apps/`、`packages/` 或独立 Worker 仓库。
