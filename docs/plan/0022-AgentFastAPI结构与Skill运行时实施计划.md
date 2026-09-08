# Agent FastAPI 结构与 Skill 运行时实施计划

日期：2026-09-08。依据：[Design](../design/0022-Agent%20FastAPI%20结构与%20Skill%20运行时整合设计.md) 和 [Spec](../requirement/0022-AgentFastAPI结构与Skill%20运行时整合需求规格.md)。

## 实施清单

- [x] P-01 建立 Design、Spec、Plan 和 Acceptance checklist，锁定非目标和当前三项 Skill 注册集合。
- [x] P-02 用 Red 测试固定 `app.main`、`app/api` 和 Docker 规范入口。
- [x] P-03 实现 FastAPI 应用工厂和内部路由组合，保留旧工厂兼容转发。
- [x] P-04 用 Red 测试固定 Skill 注册表、摘要校验、资源边界和失败信息。
- [x] P-05 实现 `app/skills` 注册表并接入启动验证与 readiness。
- [x] P-06 更新 Dockerfile、README、架构测试和运行时测试。
- [x] P-07 执行 Agent 全量质量门禁、Docker/Compose 检查和差异审查。
- [x] P-08 回填 Acceptance，确认未触碰数据库、Temporal 历史、业务数据和用户凭据。

## 验证命令

```sh
cd agent
.venv/bin/ruff check app tests
.venv/bin/ruff format --check app tests
.venv/bin/pyright app tests
.venv/bin/pytest -q
docker build --file Dockerfile --tag lanverse/agent:structure-check ..
cd ..
docker compose config --quiet
```

受环境限制的真实 Temporal、Codex、剧本语义和远端 CI 检查必须单列，不把 skip 或未执行报告为通过。

## 本切片验证结果

- `.venv/bin/ruff check app tests`：通过。
- `.venv/bin/ruff format --check app tests`：106 个文件通过。
- `.venv/bin/pyright app tests`：0 errors、0 warnings、0 informations。
- `.venv/bin/pytest -q`：140 passed、60 skipped；跳过项是既有的独立 PostgreSQL/Temporal/真实 Codex opt-in 条件。
- `docker compose config --quiet`：通过。
- `docker build --file agent/Dockerfile --tag lanverse/agent:structure-check .`：通过。
- 镜像 smoke：非 root 用户、两个静态 Skill 目录、`app.main` 工厂和三个注册能力摘要校验通过。

## 交付审查

提交前检查：规范入口是否唯一、旧转发是否仅兼容、所有 Skill 是否显式注册、资源路径是否受限、错误是否安全、业务路由合同是否无漂移、Docker 镜像是否包含新包、工作区是否无缓存/凭据/生成物。
