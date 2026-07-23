# Lanverse Backend

Lanverse 后端主仓库，当前用于建立稳定、可复用、可验收的 Codex Agent 协作工作流。

协作层基于 [`StephenQiu30/stephen-codex`](https://github.com/StephenQiu30/stephen-codex) 整理，并结合本仓库的业务目录骨架完成适配。它包含角色分工、TDD/SMART/MVP 原则、Symphony-ready ticket 编排和 Git/PR 收口要求。

## 项目地址

GitHub: <https://github.com/StephenQiu30/lanverse-backend.git>

## 适用人群

1. 希望为 Codex 项目建立统一协作规则的开发者。
2. 希望把 AI Agent 分工固化为可维护文件结构的团队。
3. 希望将 TDD、SMART、MVP 原则写入日常 AI 协作流程的工程团队。
4. 需要一个可开源、可复制、可二次定制的 Codex Agent 模板项目的用户。

## 目录定位

本仓库将 Codex 协作规范与 Lanverse 后端工程目录放在同一 Git 根目录中，重点解决以下问题：

1. Codex 在项目中应该读取哪些规范文件。
2. Codex 全局规范与项目局部规范如何区分。
3. Codex 多角色协作时 PM、Explorer、Builder、Tester、Reporter 如何分工。
4. 如何在 Codex 工作流中持续执行 MVP、TDD、SMART 规范。
5. 如何通过 `WORKFLOW.md` 对齐 OpenAI Symphony 的 Linear ticket 编排方式。

## 目录功能

1. `AGENTS.md`：Codex 侧长期稳定的全局协作规范。
2. `AGENTS.local.md`：当前项目中的局部规范配置，用于和全局规则区分。
3. `WORKFLOW.md`：OpenAI Symphony 风格的 Linear ticket 调度契约与 per-ticket Agent SOP。
4. `.codex/agents/`：Codex 角色定义目录。
5. `.codex/skills/`：Codex 可复用工作流目录，当前承载 Linear、debug 和 Git 收口流程。
6. `docs/`：项目文档骨架目录，保留分类目录和 README，正文文档按任务需要再归档。
7. `backend/`：Lanverse 后端代码；`infra/`：本地依赖设施；现有 `frontend/` 暂作拆仓来源保留。
8. `.github/workflows/ci.yml`：GitHub Actions CI，用于检查模板基础结构。
9. `LICENSE`：开源许可证。
10. `CONTRIBUTING.md`：贡献说明。

## 文件结构

```text
lanverse-backend/
├── .github/
│   └── workflows/
│       └── ci.yml
├── README.md
├── LICENSE
├── CONTRIBUTING.md
├── AGENTS.md
├── AGENTS.local.md
├── WORKFLOW.md
├── .codex/
│   ├── agents/
│   │   ├── pm.toml
│   │   ├── explorer.toml
│   │   ├── builder.toml
│   │   ├── tester.toml
│   │   └── reporter.toml
│   └── skills/
│       ├── linear/
│       ├── linear-task/
│       ├── debug/
│       ├── commit/
│       ├── pull/
│       ├── push/
│       └── land/
├── docs/
│   ├── README.md
│   ├── design/
│   │   └── README.md
│   ├── prd/
│   │   └── README.md
│   ├── plans/
│   │   └── README.md
│   ├── acceptance/
│   │   └── README.md
│   └── operations/
│       └── README.md
├── backend/
│   ├── migrations/versions/
│   ├── src/thief/{api,catalog,identity,infrastructure}/
│   ├── tests/{architecture,integration,unit}/
│   └── tools/
├── frontend/
│   └── src/{app,components,lib}/
└── infra/
    └── compose/
```

## 项目使用

1. 克隆项目：`git clone https://github.com/StephenQiu30/lanverse-backend.git`。
2. 优先阅读 `AGENTS.md` 理解全局规则，再按需修改 `AGENTS.local.md`。
3. 接入 Symphony 或兼容 runner 时，配置 `.env` 中的 Linear project slug、workspace root 和 `SOURCE_REPO_URL`。
4. 自定义角色时修改 `.codex/agents/` 下的角色文件。
5. 后端业务代码按 `backend/` 和 `infra/` 的既有边界演进；新的前端交付进入独立的 `lanverse-frontend` 仓库。
6. 本项目使用 MIT License，允许个人或团队在保留许可证声明的前提下自由使用和修改。

## 核心规范

1. `交付流程`：正式功能严格按 `Design → PRD → Plan → Acceptance` 推进。
2. `MVP`：优先完成最小可用闭环，不做过度设计。
3. `TDD`：新增功能、修复缺陷或调整核心逻辑时，优先执行红灯、绿灯、重构流程。
4. `SMART`：需求、任务与验收标准需要具体、可衡量、可达成、相关并具备阶段边界。
5. `文件规模`：规范文件以 200 行以内为目标，确需更长时按职责拆分。
6. `Git 收口`：完成任务后执行匹配风险的验证，并保持提交和工作区清洁。
7. `Symphony-ready`：复杂任务围绕 Linear ticket、隔离 workspace、Workpad、Agent Review 和 Human Review 执行。

## 角色分工

1. `PM`：按 SMART 原则拆解需求、定义范围、制定验收标准、控制 MVP 边界。
2. `Explorer`：读取代码、查找文件、梳理依赖、提供事实依据。
3. `Builder`：基于验收目标做最小实现，涉及逻辑改动时遵循 TDD。
4. `Tester`：执行测试、lint、回归检查，并确认 TDD 红绿重构结果。
5. `Reporter`：汇总修改内容、验证证据、残余风险和交付说明。

标准执行顺序：

```text
Explorer -> PM -> Builder -> Tester -> Reporter
```

简单任务可以压缩为：

```text
PM -> Builder -> Tester
```

## 验收标准

1. `AGENTS.md` 存在，并包含 MVP、TDD、SMART、角色协作、Git/PR 收口和交付输出要求。
2. `AGENTS.local.md` 存在，并说明它是项目局部规范配置文件。
3. `WORKFLOW.md` 存在，并包含 `tracker.kind: linear`、`project_slug`、`## Codex Workpad` 和 `Human Review` 编排规则。
4. `.codex/skills/` 中保留 `linear`、`linear-task`、`debug` 和 Git 收口 skills。
5. `.codex/agents/` 中存在 `pm`、`explorer`、`builder`、`tester`、`reporter` 五类角色。
6. `docs/` 目录保留分类结构和 README，不包含任务正文文档。
7. `LICENSE` 与 `CONTRIBUTING.md` 存在，项目具备基础开源使用说明。
8. README 能够说明本目录定位、功能、结构、角色和验收标准。
9. Git 提交与 PR 合并规范包含中文提交、工作区干净、PR 合并前 tag 等要求。
10. GitHub Actions CI 存在，并检查关键脚本的基础语法。
11. `AGENTS.md` 不超过 200 行，并明确 `Design → PRD → Plan → Acceptance`。

## 维护原则

1. Codex 侧文件命名保持 `AGENTS.md` 与 `AGENTS.local.md`。
2. 全局稳定规则写入 `AGENTS.md`，项目局部规则写入 `AGENTS.local.md`。
3. Symphony 调度配置写入 `WORKFLOW.md`，不要把 project slug、workspace root 和 runner hooks 混入 `AGENTS.md`。
4. 角色职责写入 `.codex/agents/`，不要混入全局规则文件。
5. 不为当前没有使用场景的角色、流程或目录做过度扩展。
