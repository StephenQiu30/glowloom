# Backend 统一入口与本机环境复用验收记录

- 状态：阶段 6 本机完成门通过；远端 CI 未触发
- 日期：2026-08-29
- Design：[后端服务架构](../design/2001-后端服务架构.md)
- Requirement：[后端运行架构需求规格](../requirement/2001-后端运行架构需求规格.md)
- Plan：[后端运行架构实施计划](../plan/2001-后端运行架构实施计划.md)

## 验收范围

本任务只收敛 Backend 运行入口、Docker/Compose 职责和日志传输，不改变 Workflow、Production、StoryGraph 或 Agent 的业务语义：

1. `backend/cmd` 只保留一个 `main.go`，镜像只构建 `/usr/local/bin/lanverse`；同一进程依次启动 API、Workflow Runtime 和 Event Runtime，Replay/Reindex 仍使用同一个 Binary。
2. `docker-compose.yml` 只声明 Frontend/Backend 项目服务；`docker-compose-env.yml` 只声明显式 `bundled-*` profile 的隔离环境；`docker-compose-prod.yml` 只提供线上覆盖。
3. 本机项目容器复用已启动的 PostgreSQL、MinIO、Homebrew Kafka、Homebrew Temporal、Elasticsearch/Kibana、Logstash 与 Agent Runtime，不重复启动环境容器。
4. 删除 Filebeat 和 Kafka 日志 Topic/身份/ACL。单 Backend Logger 在脱敏后同时写 stdout，并通过失败开放、限时重连的 TCP Writer 直送 Logstash `5000`；Kafka 只承载 Script/StoryGraph 已提交业务事件。
5. 测试仍全部位于 `backend/tests`、`agent/tests`，没有 Migration、迁移字段、Raw SQL、第二 ORM、第二数据库连接模型或兼容 Binary。

## Red → Green → Refactor

### Red

- 新增 Logger TCP 双写、非法地址和 Logstash 不可用时失败开放测试；初始编译明确失败于缺少 `telemetry.NewLogstashLogger`。
- 架构测试先固定单 `backend/cmd/main.go`、单镜像 Binary、项目/环境 Compose 职责和“无 Filebeat/无 Kafka 日志 Topic”拓扑。
- 真实 Docker build 首次发现 API 组合根仍需要 `telemetry.HTTPMiddleware` 导入，构建以 `undefined: telemetry` 失败，没有绕过或弱化构建门。

### Green

- `backend/internal/telemetry/logstash.go` 使用标准库实现单连接、并发安全、500ms 网络上限和 5 秒重连退避；Logstash 失败只丢失远端副本，stdout 与业务服务继续工作。
- 单 `backend/cmd/main.go` 创建唯一 `lanverse-backend` Logger，并注入 API/Workflow/Event 三个组合根；组合根不再各自创建 Logger。
- Logstash Pipeline 改为 `tcp/json_lines → 二次白名单/脱敏 → application index`，非法记录只保留错误码和 SHA-256 后进入独立 Dead-letter Index；单条 JSON 上限固定为 1 MiB。
- Compose、Kafka 初始化脚本、CI、README 和正式 Design/Requirement/Plan 同步删除 Filebeat、日志 Kafka Topic 与旧 Worker Binary 事实。

### Refactor

- API、Workflow、Event 原有组合代码只移动到 `internal/bootstrap`，没有拆分微服务、增加抽象层或复制业务 Repository。
- `docker-compose-env.yml` 中所有环境都受显式 profile 控制；默认服务清单为空，避免本机误启动重复依赖。
- 生产组合仍可启用隔离 Logstash/Elasticsearch/Kibana；本机默认连接既有 `logstash-local-dev`，二者共享相同 JSON 传输契约。

## 真实验收证据

### 单入口与运行状态

- `find backend/cmd -maxdepth 2 -type f` 只返回 `backend/cmd/main.go`。
- Backend 镜像构建成功，镜像中存在且只启动 `/usr/local/bin/lanverse`；旧 `lanverse-api`、`lanverse-workflow-worker`、`lanverse-event-worker` 不存在。
- 本机容器中只存在一个 `lanverse` 进程；`http://127.0.0.1:8686/healthz`、公共 `/readyz` 和内部 Event Runtime `127.0.0.1:8687/readyz` 全部通过。
- 启动日志同时出现 `lanverse api started`、`lanverse workflow worker started` 与 `lanverse event worker started`，证明三个职责由同一个容器/进程装配。

### Compose 与本机环境

- `docker compose -f docker-compose.yml config --services` 精确返回 `backend frontend`。
- `docker compose -f docker-compose-env.yml config --services` 在无 profile 时返回空；显式启用 `bundled-logstash,bundled-temporal` 时只返回 `logstash temporal`。
- 开发、环境和生产三文件分别/组合 `config --quiet` 全部通过；生产组合服务清单精确包含 Backend/Frontend 与所需隔离依赖，不含 Workflow/Event 第二服务。
- 已精确删除重复的 `lanverse-env-filebeat-1`、`lanverse-env-logstash-1` 容器且未删除卷；`logstash-local-dev` 保持健康运行。

### Logstash 全链与故障开放

- 真实 Backend HTTP 请求携带固定 `request_id=7a70b9df-a8f4-4de5-b1ad-2bf12da462d0` 和 W3C trace；现有 Logstash 将记录写入本机 Elasticsearch `logstash-2026.08.28`，查询返回 `service=lanverse-backend`、`event=http_request` 和精确 trace/request ID。
- 请求 URL 中的 `token=must-not-leak` 未出现在 Elasticsearch 文档中。
- 使用不可达 `LOGSTASH_ADDRESS=host.docker.internal:59999` 重建 Backend 后，API、Workflow 和 Event Runtime 仍全部健康；恢复现有 Logstash 地址后无需数据或状态修复即可继续写日志。
- `docker.elastic.co/logstash/logstash:9.4.4 --config.test_and_exit` 对仓库 Pipeline 返回 `Configuration OK`。

### 质量与 CI 等价检查

- Backend：`gofmt` 无漂移，`go vet ./...` 通过，`go test -count=1 -p 1 ./...` 全部通过。
- Agent：Ruff check/format、Pyright `0 errors`、Pytest `39 passed, 4 skipped`；四项跳过只属于显式 `LANVERSE_TEST_REAL_CODEX=1` 的真实 Codex 长旅程，本任务未修改 Agent 执行语义。
- Frontend：ESLint、TypeScript、Vitest `18 files / 54 tests`、Next.js production build 全部通过。
- OpenAPI Client 重新生成后 `frontend/src/api` 无漂移。
- GitHub Actions YAML 可解析；CI 中 Backend、Agent、Frontend、OpenAPI、Deployment、Delivery 的本地等价命令、三层 Compose 清单和 Backend 镜像门均通过。未推送分支，因此本记录不宣称远端 GitHub Check 已运行。

## 失败与处置

1. 首次真实镜像构建因误删 API 对 `telemetry` 的非 Logger 导入而失败；恢复必需导入并重新编译、重建成功。
2. Docker build 期间外部 `es-local-dev` 以退出码 `137` 停止，Backend Event Runtime 因真实 Elasticsearch readiness 失败而退出，没有伪装 ready。重启既有 Elasticsearch 后 Backend 恢复，API/Event readiness 和日志查询重新通过。
3. 没有为上述失败增加 fallback、兼容入口、弱化 readiness 或内存替身。

## 残余边界

- 本机现有 Logstash 按其外部配置写 `logstash-*`；仓库隔离/生产 Pipeline 写严格 `lanverse-logs-application` 与 `lanverse-logs-dead-letter`。两者都消费同一脱敏 `lanverse.log.application` JSON，但索引命名由各运行环境负责。
- 未经用户授权未推送代码，因此远端 GitHub CI 需要在后续 push/PR 时由 `required` Job 真实执行；本任务只记录已完成的本地等价门。
- 全部 StoryGraph/Workflow 实施任务尚未完成，按约定本任务不运行 `agent-browser`，也不把本记录当作最终浏览器验收。
