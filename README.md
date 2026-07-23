# Lanverse Backend

Lanverse 后端仓库骨架。

项目协作规范基于 [`StephenQiu30/stephen-codex`](https://github.com/StephenQiu30/stephen-codex) 整理；当前不预设业务实现、框架或模块边界。

## 仓库地址

<https://github.com/StephenQiu30/lanverse-backend>

## 目录

```text
lanverse-backend/
├── backend/
│   ├── src/
│   ├── tests/
│   └── migrations/
├── infra/
├── docs/
│   ├── design/
│   ├── prd/
│   ├── plans/
│   ├── acceptance/
│   └── operations/
├── .codex/
│   ├── agents/
│   └── skills/
├── AGENTS.md
├── AGENTS.local.md
└── WORKFLOW.md
```

## 协作约定

1. 正式功能按 `Design → PRD → Plan → Acceptance` 推进。
2. 需求与验收遵循 SMART；核心逻辑优先使用 TDD。
3. 角色和复用流程分别维护在 `.codex/agents/` 与 `.codex/skills/`。
4. 技术栈、启动命令和业务模块在需求被接受后再引入。

## 当前状态

仓库仅保留协作规范与最小目录骨架，无可执行应用代码。
