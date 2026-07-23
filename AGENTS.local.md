# AGENTS.local.md

本文件记录 Lanverse Backend 的项目局部规则；长期通用规则以 `AGENTS.md` 为准。

## 当前项目规范

1. 后端实现放在 `backend/src/`，测试放在 `backend/tests/`，迁移放在 `backend/migrations/`。
2. 基础设施文件仅在确有运行需求时放入 `infra/`。
3. 正式文档按职责进入 `docs/`，不保存临时任务清单或过程记录。
4. 当前只保留目录骨架；技术栈、依赖、命令和模块边界不得提前假设。
5. 引入实现时，同步补充最小可执行验证命令。
