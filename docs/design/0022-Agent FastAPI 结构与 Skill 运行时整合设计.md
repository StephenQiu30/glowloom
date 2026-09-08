# Agent FastAPI 结构与 Skill 运行时整合设计（历史版本）

- 状态：已废止（2026-09-08）
- 取代文件：[0023-Agent FastAPI 标准化目录与 Skill 运行时设计](0023-Agent%20FastAPI%20标准化目录与%20Skill%20运行时设计.md)

本文件记录上一轮以现有代码为基线的应用壳和 SkillCatalog 尝试。它把 `app/api/application.py` 视为规范入口，但没有把业务模块中的 FastAPI 路由、闭包注册和生命周期真正迁移到标准 HTTP 层，因此不再作为当前实现依据。

当前实现必须以 0023 为唯一结构规范：单一 `app.main:create_app` 工厂、`app/api/routes` 路由、`Depends` 依赖、单一 `lifespan`，以及由应用容器注入的 Skill 运行时。
