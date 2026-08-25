# Workflow Storyboard 多集持久候选验收记录

- 日期：2026-08-26
- 结论：通过
- 对应计划：`activity.storyboard_draft` 持久候选节点

## 验收范围

本记录验收 `episode_structures → StoryboardDraftSet → StoryboardDraftBatch × Episode → 持久 Agent Invocation → storyboard_candidate → HumanTask`。Planning 仍拥有 confirmed Structure 批次；Storyboard Backend Owner 持久化 Set/Batch/Receipt；Python Agent 只返回候选；Temporal 只负责编排与持久等待。

## 通过证据

| 检查项 | 结果 |
|---|---|
| 完整批次 | 两集 confirmed Structure 生成 1 个 Draft Set、2 个逐集 Draft Batch 和 2 个 Agent Invocation，没有只处理首集 |
| 单一事实源 | `StoryboardDraftSet` 进入唯一 GORM Model Catalog；使用同一 PostgreSQL，没有 Migration、手写 SQL Schema 或第二 Writer |
| 幂等与恢复 | Set、全部 Batch、Task、Invocation 和 `storyboard.create_set` Receipt 在同一 GORM 事务创建；Temporal 以同一 Node 幂等键轮询，只产生一个 Set/Receipt |
| 持久等待 | 首次候选未完成时节点返回 `RETRYING`，真实 Temporal 创建 5 秒 Timer 后继续；Activity 不同步占用 Agent 全程 |
| 候选边界 | 两个 Agent 结果均为 `needs_review`；Set 对有序 Batch/输入/结果哈希生成唯一 Content Hash；节点只输出 `storyboard_candidate`，未创建正式 Shot |
| 人工门 | Workflow 最终为 1 个 Set 打开 1 个 `human.storyboard_review`，Run/Node 均停在 `WAITING_HUMAN` |
| 部署组合 | `workflow-worker` 组合根显式注入 Storyboard Application Service；Backend 镜像同时包含 API 与 Workflow Worker 可执行文件 |

## 真实命令

在全新的 `postgres:16.15-alpine` 与锁定摘要的 `temporalio/temporal` 上执行：

```text
test -z "$(gofmt -l .)"
go vet ./...
LANVERSE_TEST_DATABASE_URL=... LANVERSE_TEST_TEMPORAL_ADDRESS=... go test -count=1 -p 1 ./...
```

结果：Backend 全部包通过；`backend/tests/workflow` 通过（50.819s），其中两集 Storyboard Workflow 真实链路到达 Storyboard Human Gate，且结构确认者可以是不同于 Workflow 发起人的同 Workspace editor。

其余 CI 门禁：

```text
uv sync --locked --extra dev
ruff check app tests
ruff format --check app tests
pyright app tests
pytest -q

npm ci
npm run openapi2ts
npm run lint
npm run typecheck
npm run test
npm run build

git diff --exit-code -- frontend/src/api
docker compose ... config --quiet
docker build --tag lanverse/backend:ci backend
docker run --rm --entrypoint /bin/sh lanverse/backend:ci -c 'test -x /usr/local/bin/lanverse-api && test -x /usr/local/bin/lanverse-workflow-worker'
```

结果：Agent 静态检查、类型检查与 12 个测试通过；Frontend 16 个测试文件、45 个测试和生产构建通过；OpenAPI 无漂移；开发/生产 Compose、仓库卫生、镜像和双二进制门禁通过。

## 未纳入本切片

- `human.storyboard_review` 尚未调用 Storyboard Owner 核对逐镜决议、批准并原子 Apply；下一切片必须返回真实 Owner Receipt 和正式 `storyboards` 输出。
- `production.storyboard_export`、单 Shot 局部重跑和最终 `agent-browser` 验收仍按后续计划执行；本记录不提前宣称这些能力完成。
