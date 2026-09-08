# Docker 应用部署与文本解析恢复验收记录

2026-09-08。规格与 checklist 见 [0020 设计](../design/0020-文本解析失败诊断与受控恢复设计.md)。本记录区分实际应用运行、自动化测试和仍未完成的语义验收。

## 已完成的部署闭环

前端、Go Backend（包含 Workflow/Event Worker）、Harness、Creation API 和 Creation Worker 均通过根 `docker-compose.yml` 运行。五个应用 LaunchAgent 已 bootout，原 plist 移至 `~/Library/Application Support/Lanverse/backups/launchagents-before-docker/` 保留回退证据。未重启 Docker Desktop，未更换 PostgreSQL、MinIO、Kafka、Temporal 或 ELK。

对实际应用执行 `docker compose ps`：前端、Go、Harness、Creation API 健康，Creation Worker 正常运行。Go API `/readyz`、Event Worker `8687/readyz`、Harness `/readyz`、Creation API `/readyz` 均为 200；前端 8123 返回 200。仅前端/API 发布到宿主 loopback，Agent 端口仅在容器网络内可达。

Harness 容器中 `codex login status` 返回已使用 ChatGPT 登录，`codex --version` 为 0.149.1。初次构建暴露 npm 缺少 Linux ARM64 可执行依赖，已显式安装对应平台包，并增加构建阶段版本执行检查。候选镜像依赖从 `uv.lock` 导出，与已验证依赖版本一致。Harness 仅挂载只读 auth.json，可信服务不挂载 Codex 登录。

旧运行持久化的原生服务 origin 导致 Docker 中出现 `agent_transport_failed`。显式 `CREATION_AGENT_RELOCATED_FROM` 修复同一服务的地址迁移，运行记录未更新。真实 Worker 经 Go 再读 Creation 的原分集门返回 `accepted`，恢复接口返回 202。

## 实际原稿与恢复

- 项目：`8877ed81-d0af-450b-b448-3027e4f74476`（Empress · 60集原稿解析验收）。
- 原稿版本：`2dd9c0c1-5125-4719-94fd-fd0b825fb95d`。
- 逻辑运行：`68c180a2-8548-4b89-b937-cab9d01fa399`，冻结 SkillRelease 仍为 `63310724e287f09d5577bbe44410a480aefe7ad8f72956b2d78615c3ac05ecb3`。
- 正式数据库仍有 60 集，Elasticsearch 的 60 个 script 文档逐项匹配当前版本、内容摘要和正文；`_snapshot` 元数据另计，不是重复剧集。关键词 `Jace Iris` 命中 42 集；17:52 再验收时，既有 Logstash/ES 已索引 661 条本次 Docker 切换后的 production 应用日志。
- 第 4 集前两次 Attempt 保持 unknown；第二次恢复授权 `625045cc-28da-47af-a5cb-c8b7b04c7500` 仅允许第三次 Attempt `28905c8f-988f-4013-91c6-488314e363d9`。已有分集和前 3 集草案复用。
- 第三次 Attempt 于 17:46:54 开始、17:54:02 成功，新增草案并进入 needs_review；工作流继续第 5 集，调用预留数为 8。成功仅指候选合同与持久化通过，未代表人工语义采纳。

真实浏览器已验证原登录、固定原稿和 60 集仍可读取；文本创作页面能呈现原第一集人物、场景和道具草案。浏览器标签页调试连接中断后，改用电脑插件的 Chrome 原生界面继续验证：页面显示“正在创作 / 已生成 4 份草案”，点击“同步执行与提案”后出现第 2、3、4 集提案；第 4 集的 3 场、人物、道具、8 条对白和 55 处原文引用均可实际查看。没有点击领取审阅或采纳。

## 自动化证据

- Agent 的配置/失败恢复定向测试：17 passed；追加迁移及真实既有 Temporal 的隔离工作流恢复测试通过。
- `cd agent && .venv/bin/pytest -q`，指定既有 PostgreSQL 上的隔离测试数据库及本机 Temporal：186 passed，6 个显式 opt-in 真实模型用例 skipped。业务验收任务单独使用实际服务和指定原稿。
- `cd agent && .venv/bin/pyright`：0 errors；Ruff check/format：通过。最初从仓库根目录错误调用 pyright 未加载 Agent 配置，已在正确目录重跑。
- `cd backend && go test -race ./tests/config ./tests/production/creation ./tests/production/script/adapter/gormdb` 通过；受影响配置、Creation、bootstrap 的 `go vet` 通过，golangci-lint 为 0 issues。新增地址迁移配置测试最初缺少测试 DATABASE_URL，补齐测试配置后重跑通过。
- govulncheck：原定向扫描无可达漏洞；另有 1 个导入包、3 个依赖模块漏洞不在可达调用路径。
- 三种 Compose 配置（应用、环境、应用+环境+prod）均用合成 CI 参数执行 `config --quiet` 通过；未启动第二套环境。CI 服务清单断言同步新增三个 Agent 职责。
- 最终追加 Harness 502 回执 HTTP 用例与 Go/Python 原生跨边界用例：11 passed。
- 前端本轮早期定向用例 5 项、类型检查、lint 和生产构建通过，Docker 复用该生产产物。

## 第 4 集真实草案审阅

主线与场次成立：水循环井中兄妹悬挂求生、俱乐部中 Tristan 拒绝 Aurelia 来电、Aurelia 在飞行通道拦下货运载具；3 场连续覆盖正文，另单列集标题。关键的防护服腕带、腕式终端和重型货运载具被提取；Valerie 关于晴天的说法被标记为人物主张，没有升格为客观事实。

制作完整性仍有缺口：动作节拍保存了失效进水阀和排水边沿，却未把它们列入对应场景构件/资产提及；`children` 仍作为集合提及等待与 Jace/Iris 合并；3 场 time_branch 都是 unknown，跨集闪回延续尚未解决。本集结果为英文，与第 1 集中文草案不同，统一输出语言需显式产品合同。这些发现保留在待审草案中，不直接篡改模型输出或热改原运行冻结 SkillRelease。

## 仍未验收的业务能力

第 1 集草案仍存在闪回分支延续标记不一致、V.O. 分类与实体别名连续性等语义问题。它是待审草案，未被自动采纳。全 60 集人物、场景、全局设定和分镜尚未完成，不以服务健康、原稿落库或 ES 检索代替完整解析验收。

尚未验证主机重启恢复、Codex 登录刷新、公网 TLS 或全媒体生成。不执行 Git 提交、推送或远端发布；当前代码仍在 main 工作区，文件清单如下。配置只在受限本地文件中更新，没有新增运行凭据到 Git diff。

## 本地 Docker 启动精简验收（2026-09-08）

本切片仅调整部署配置、入口文档及相应检查。根目录保留一个应用 Compose；原环境编排移至 `deploy/ci/compose.dependencies.yml`，Linux CI 的内部网络连接单独放在 `deploy/ci/compose.application.yml`，原生产覆盖收敛为 `deploy/compose.production.yml`。本地不会加载这些覆盖或启动基础设施。

当前应用 Compose 为 171 行，`.env.example` 为 49 行。移除 DOCKER_* 平行地址、POSTGRES_* 拼接方式、特定 ELK 外部网络依赖，以及 10 个与 Go 默认值完全相同的配置常量；默认开发入口使用 development，实际本机 `.env` 的既有运行模式保持不变。Frontend 仅在镜像构建时注入公开 API 地址，共用应用启动安全设置。README 本地启动只保留一个入口和状态/日志命令。

验证证据：

- `docker compose config --quiet` 通过；使用纯合成变量和 `--env-file /dev/null` 分别渲染应用、镜像覆盖、CI 应用覆盖、CI 全 profile 依赖，四种组合均通过，所有 CI bind 文件路径存在。没有启动任何 CI 依赖。
- 比较精简前后的 resolved Compose：三个 Agent 服务完整配置相同；业务库、Creation 库及授权身份未变。10 个删除的常量逐项与 Go 默认值比对相同，只有 ES/Logstash 网络入口改为本机发布端口。私密配置备份只保存在用户运行目录，权限 600，不进入仓库。
- 从现有 Creation 容器连通宿主 5432、9000、7233、19093、9200、5000。实际执行 `docker compose up -d --no-build --no-deps backend frontend`，只重建 Backend 和 Frontend，三个 Agent 容器 ID 均未改变。
- 更新后 Frontend 8123、Backend `/healthz` 与 `/readyz` 均为 HTTP 200，内部 Event Runtime `8687/readyz` 为 ready。Frontend、Backend、Harness、Creation API 均 healthy；Creation Worker 运行中，按既有配置未配置 healthcheck。
- 实际健康请求 `ea1b8a29-2b03-4d42-b869-2a4a88ea5b43` 经 Backend → 本机 Logstash → 原 ES 日志别名检索命中，证明移除 ELK 专属网络后的日志链仍可用。
- `go test -race ./tests/architecture ./tests/eventing ./tests/search ./tests/observability`、对应 `go vet`、`golangci-lint run` 均通过，Lint 为 0 issues；修改的 Go 检查文件 gofmt 通过，CI YAML 解析与 `git diff --check` 通过。
- 初次检查发现已有恢复模块的两个 Temporal protobuf 导入被命名检查误判，补充精确第三方模块例外及其用例后重新通过；未放宽项目自有命名规则。

限制：本切片没有运行 GitHub CI、全仓测试或远端生产部署，没有重新构建应用镜像；变更是 Compose 配置，已应用到现有本机镜像。依赖真实测试环境的自动化集成用例按既有约定跳过，未将其计为真实业务验收。未修改生产 Go 代码或依赖，本轮未重复运行 govulncheck；剧本人物/场景语义验收仍以本记录前面的未决项为准。

## 未提交文件清单

当前分支 main；本轮未提交或推送。保留此前 Agent 恢复、Go 合同和前端诊断改动，以下为当前全部遗留文件，包含本切片和此前工作。根目录两个旧 Compose 文件的删除对应迁移，不是删除运行数据。根 `.env` 仅本机连接配置调整，已确认被 Git 忽略。

- [.env.example](../../.env.example) — 修改。
- [.env.production.example](../../.env.production.example) — 修改。
- [.github/workflows/ci.yml](../../.github/workflows/ci.yml) — 修改。
- [README.md](../../README.md) — 修改。
- [agent/Dockerfile](../../agent/Dockerfile) — 修改。
- [agent/app/candidate_runtime/text_storyboard_api.py](../../agent/app/candidate_runtime/text_storyboard_api.py) — 修改。
- [agent/app/creation/activities.py](../../agent/app/creation/activities.py) — 修改。
- [agent/app/creation/config.py](../../agent/app/creation/config.py) — 修改。
- [agent/app/creation/execution.py](../../agent/app/creation/execution.py) — 修改。
- [agent/app/creation/platform.py](../../agent/app/creation/platform.py) — 修改。
- [agent/app/creation/repository.py](../../agent/app/creation/repository.py) — 修改。
- [agent/app/creation/worker.py](../../agent/app/creation/worker.py) — 修改。
- [agent/app/creation/workflow.py](../../agent/app/creation/workflow.py) — 修改。
- [agent/app/modules/text_storyboard/harness.py](../../agent/app/modules/text_storyboard/harness.py) — 修改。
- [agent/app/reasoning/codex.py](../../agent/app/reasoning/codex.py) — 修改。
- [agent/requirements.txt](../../agent/requirements.txt) — 修改。
- [agent/tests/creation/conftest.py](../../agent/tests/creation/conftest.py) — 修改。
- [agent/tests/creation/test_attempt_history.py](../../agent/tests/creation/test_attempt_history.py) — 修改。
- [agent/tests/creation/test_config.py](../../agent/tests/creation/test_config.py) — 修改。
- [agent/tests/integration/test_text_storyboard_api.py](../../agent/tests/integration/test_text_storyboard_api.py) — 修改。
- [backend/internal/bootstrap/api_process.go](../../backend/internal/bootstrap/api_process.go) — 修改。
- [backend/internal/config/config.go](../../backend/internal/config/config.go) — 修改。
- [backend/internal/production/creation/adapter/agenthttp/client.go](../../backend/internal/production/creation/adapter/agenthttp/client.go) — 修改。
- [backend/internal/production/creation/adapter/agenthttp/proposals.go](../../backend/internal/production/creation/adapter/agenthttp/proposals.go) — 修改。
- [backend/internal/production/creation/application/trace.go](../../backend/internal/production/creation/application/trace.go) — 修改。
- [backend/tests/architecture/runtime_entrypoint_test.go](../../backend/tests/architecture/runtime_entrypoint_test.go) — 修改。
- [backend/tests/architecture/semantic_naming_test.go](../../backend/tests/architecture/semantic_naming_test.go) — 修改。
- [backend/tests/config/creation_test.go](../../backend/tests/config/creation_test.go) — 修改。
- [backend/tests/eventing/topology_contract_test.go](../../backend/tests/eventing/topology_contract_test.go) — 修改。
- [backend/tests/observability/topology_contract_test.go](../../backend/tests/observability/topology_contract_test.go) — 修改。
- [backend/tests/production/creation/http_client_test.go](../../backend/tests/production/creation/http_client_test.go) — 修改。
- [backend/tests/production/creation/native_agent_test.go](../../backend/tests/production/creation/native_agent_test.go) — 修改。
- [backend/tests/production/creation/proposal_http_test.go](../../backend/tests/production/creation/proposal_http_test.go) — 修改。
- [backend/tests/production/creation/trace_test.go](../../backend/tests/production/creation/trace_test.go) — 修改。
- [backend/tests/production/script/adapter/gormdb/creation_http_journey_test.go](../../backend/tests/production/script/adapter/gormdb/creation_http_journey_test.go) — 修改。
- [backend/tests/search/topology_contract_test.go](../../backend/tests/search/topology_contract_test.go) — 修改。
- `docker-compose-env.yml` — 删除（迁移至 deploy）。
- `docker-compose-prod.yml` — 删除（迁移至 deploy）。
- [docker-compose.yml](../../docker-compose.yml) — 修改。
- [docs/acceptance/0019-本机既有环境运行验收记录.md](../../docs/acceptance/0019-本机既有环境运行验收记录.md) — 修改。
- [docs/design/0001-AI短剧制作平台完整设计基线.md](../../docs/design/0001-AI短剧制作平台完整设计基线.md) — 修改。
- [docs/design/0013-创作编排与多媒体画布架构调整设计.md](../../docs/design/0013-创作编排与多媒体画布架构调整设计.md) — 修改。
- [docs/design/0019-本机既有环境运行设计.md](../../docs/design/0019-本机既有环境运行设计.md) — 修改。
- [docs/design/2001-后端服务架构.md](../../docs/design/2001-后端服务架构.md) — 修改。
- [frontend/src/features/creation/text-creation-workspace.tsx](../../frontend/src/features/creation/text-creation-workspace.tsx) — 修改。
- [agent/app/creation/recover.py](../../agent/app/creation/recover.py) — 新增。
- [agent/app/creation/recovery.py](../../agent/app/creation/recovery.py) — 新增。
- [agent/app/creation/recovery_schema.py](../../agent/app/creation/recovery_schema.py) — 新增。
- [agent/app/text_contract/failure.py](../../agent/app/text_contract/failure.py) — 新增。
- [agent/tests/creation/test_failure_recovery.py](../../agent/tests/creation/test_failure_recovery.py) — 新增。
- [deploy/ci/compose.application.yml](../../deploy/ci/compose.application.yml) — 新增。
- [deploy/ci/compose.dependencies.yml](../../deploy/ci/compose.dependencies.yml) — 新增。
- [deploy/compose.production.yml](../../deploy/compose.production.yml) — 新增。
- [docs/acceptance/0020-Docker应用部署与文本解析恢复验收记录.md](../../docs/acceptance/0020-Docker应用部署与文本解析恢复验收记录.md) — 新增。
- [docs/design/0020-文本解析失败诊断与受控恢复设计.md](../../docs/design/0020-文本解析失败诊断与受控恢复设计.md) — 新增。
