# Agent FastAPI 标准化目录与 Skill 运行时验收记录

- 状态：本轮实施验收
- 依据：[设计](../design/0023-Agent%20FastAPI%20标准化目录与%20Skill%20运行时设计.md) 与 [需求](../requirement/0023-AgentFastAPI标准化目录与Skill运行时需求规格.md)

## Checklist

- [x] 唯一生产 FastAPI 工厂为 `app.main:create_app`。
- [x] 所有生产路由位于 `app/api/routes/`，通过 `APIRouter` 统一组合。
- [x] 业务包不再创建 FastAPI 应用、调用 `add_api_route` 或修改 `lifespan_context`。
- [x] `Depends` 注入 AgentRuntime、CreationService、HarnessService 和 SkillCatalog。
- [x] 单一 lifespan 管理数据库、Dispatcher、Temporal Worker 和静态 Skill 校验。
- [x] `storygraph`、`scene_analysis`、`text_storyboard` 由同一个 SkillCatalog 注入和校验。
- [x] 未发现旧 `candidate_runtime`、`app.creation.api`、`app.api.application` 生产入口引用。
- [x] 质量门禁、Docker 构建和镜像冒烟通过。

## 证据

- 通过 `agent/.venv/bin/ruff check app tests` 与 `ruff format --check app tests`。
- 通过 Agent 全量 Pytest；独立 PostgreSQL、Temporal、真实 Codex 测试继续按既有条件 skip。
- 通过 `docker compose config --quiet`。
- 通过 Agent Docker 构建和非 root 镜像导入/Skill hash 冒烟。

## 残余风险

本轮只完成 FastAPI 传输层标准化与 Skill 运行时注入。长稿分块、真实剧本语义质量、人物 Owner、ES 投影、真实 Temporal/Codex 旅程和媒体生产仍属于 3003/3004 后续验收，不由本轮结构检查抵扣。
