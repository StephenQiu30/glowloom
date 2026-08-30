# Lanverse

Lanverse 当前交付的是“整剧原稿 → 制作圣经 → 分集 → 场景/制作任务 → 分镜 → 确定性导出”的可审核 MVP。

## 当前架构

```text
Browser / Next.js
        ↓
Go lanverse Backend（唯一 Binary / 唯一业务 Writer）
        ├─────────→ PostgreSQL（唯一 SQL 事实源）
        ├─────────→ MinIO（私有对象字节）
        ├─────────→ Temporal（内置 Workflow Runtime）
        ├─────────→ Kafka（内置 Event Runtime）→ Elasticsearch（业务检索投影）
        └─────────→ Python Candidate Runtime ──→ 本机 Codex CLI

JSON Logs → Logstash → Elasticsearch Log Index → Kibana
```

- `frontend/`：Next.js 创作工作台，只读取服务端事实并提交人工决议。
- `backend/`：唯一公共业务 API 与唯一业务 Writer；认证、项目、剧本、制作圣经、分集、结构、分镜、正式镜头、导出和持久任务都在此实现。
- `backend/cmd/main.go`：唯一 Go 启动入口；同一 `lanverse` 进程装配 API、Workflow 与 Event 三个职责运行时，不创建 Worker Binary 或 Compose 服务。
- `agent/`：私有 Candidate Runtime；校验短时 Execution Grant，只执行结构化 Codex Harness，不连接 PostgreSQL/MinIO，不拥有公共业务路由。
- `backend/internal/platform/database/model`：唯一 GORM Model Catalog 与表结构事实源。
- `backend/api/openapi/lanverse-v1.json`：唯一公共 REST 契约源。
- `backend/internal/agent/contract`：Backend ↔ Agent 的版本化调用/结果线协议所有者；`agent/app/candidate_runtime/schemas.py` 以禁止额外字段的 Pydantic 模型校验同一协议。
- `docs/`：Design → PRD/Requirement → Plan → Acceptance 的事实链路。

当前已接入 Apache Kafka KRaft、Backend Event Runtime、Elasticsearch 业务检索和独立 ELK 日志链。Backend Owner 事务只写 PostgreSQL Outbox；Event Runtime 在事务外发布 Script/StoryGraph 已提交事件，并以 Inbox/Revision Checkpoint、隔离 DLQ 和有界 Replay 收敛至少一次投递。Script/StoryGraph Search Alias 可从 PostgreSQL Owner Snapshot 全量重建。唯一 Backend 进程输出统一脱敏 JSON，同时保留 stdout 并以失败开放的 TCP Writer 直送 `Logstash → Elasticsearch → Kibana`；日志不再经过 Filebeat 或 Kafka，Kafka 只承载已提交业务事件。ELK/Elasticsearch 不回写业务事实。Redis 仍未引入。Backend 只接受一个 PostgreSQL `DATABASE_URL` 作为业务 SQL 事实源；Temporal 只拥有 Workflow History，仓库不保留手写 SQL Schema/Migration、迁移版本字段、第二套 ORM/连接模型或 Python SQLAlchemy Writer。

StoryGraph 已完成到 `SG-I20` 的通用媒体 Provider 配置事实，当前只实施 `SG-I21` 的精确 ProviderCall/Receipt 执行闭环。固定 Runware、Provider API Key 环境变量、旧 Binding 路由与兼容读取已直接删除；Backend 使用内置 Preset Catalog、编译期 Factory Registry、不可变 Connection/Credential/ModelProfile/Project Binding 版本和 Docker root-key Secret。尚未注册真实 Adapter Factory 时 Catalog 不暴露预设，零 Provider 配置不阻止非视觉服务启动；Web Settings、真实模型 Adapter 和真实远端调用仍属于后续顺序任务，不能提前报告完成。

## 文档入口

- [剧本到分镜 MVP 设计](docs/design/0009-剧本到分镜MVP垂直切片设计.md)
- [剧本到分镜 MVP 产品需求](docs/prd/0009-剧本到分镜MVP产品需求.md)
- [剧本到分镜 MVP 需求规格](docs/requirement/0009-剧本到分镜MVP需求规格.md)
- [剧本到分镜 MVP 实施计划](docs/plan/0009-剧本到分镜MVP实施计划.md)
- [剧本到分镜 MVP 验收记录](docs/acceptance/0009-剧本到分镜MVP验收记录.md)
- [后端服务架构](docs/design/2001-后端服务架构.md)
- [Workflow 启动事实与 Temporal 对账验收](docs/acceptance/2010-Workflow启动事实与Temporal对账验收记录.md)

## 本机启动

先按 `.env.example` 准备根目录 `.env`。私有 Agent 需要本机可用且已登录的 Codex CLI：

```bash
cd agent
uv sync --extra dev
AGENT_EXECUTION_SECRET=development-only-agent-execution-secret \
  uv run uvicorn app.candidate_runtime.api:app --host 127.0.0.1 --port 8787
```

本机开发默认复用已经启动的 PostgreSQL、MinIO、Homebrew Kafka、Homebrew Temporal、Elasticsearch 与 Kibana；项目容器通过 `host.docker.internal` 连接这些服务。`docker-compose.yml` 只声明 Frontend/Backend 项目服务，`docker-compose-env.yml` 是可独立运行的环境栈。

本机已运行 Logstash 与 Homebrew Temporal，开发时不再创建同类容器。Backend 通过 `host.docker.internal:5000` 直连现有 Logstash；`docker-compose-env.yml` 中的所有环境服务均由显式 `bundled-*` profile 控制，只用于 CI、生产组合或确需隔离环境的场景。

环境保持运行后，日常开发只启动或更新项目服务：

```bash
docker compose --env-file .env \
  -f docker-compose.yml \
  up --build -d
```

日常测试直接复用已启动环境，不需要每轮重启。默认环境栈不会创建 PostgreSQL、MinIO、Kafka、Elasticsearch 或 Kibana 容器。本机服务需要满足 `.env` 中的地址与认证配置；Homebrew Kafka 需要公布容器可达的 Broker 地址并已创建项目 Topic，Elasticsearch 需要存在 Lanverse 使用的账号、模板和索引。

零 Provider 配置时无需准备媒体密钥，开发 Compose 会把空 Secret 挂载到 Backend，Provider 配置命令失败关闭而其他能力保持可用。需要保存 Provider 配置时，只在本机创建 `chmod 600` 的 32-byte root-key 文件并把路径写入 `LANVERSE_MEDIA_PROVIDER_MASTER_KEY_FILE`；Backend 的 root 启动器只在容器 tmpfs 中生成 `0400/lanverse` 的固定路径副本，随后立即通过 `su-exec` 降权执行唯一 Go Binary，非 tmpfs 挂载直接失败关闭。火山、OpenAI、Google 的 API Key 始终不写入 `.env`。当前只有 Backend 领域服务与持久化合同，Web 配置入口按顺序在 `SG-I22` 交付。

只有需要完全隔离的容器内存储时才显式启用对应 profile，并让应用连接容器服务：

```bash
docker compose --env-file .env \
  --profile bundled-postgres \
  --profile bundled-minio \
  --profile bundled-kafka \
  --profile bundled-elasticsearch \
  --profile bundled-kibana \
  --profile bundled-logstash \
  --profile bundled-temporal \
  -f docker-compose-env.yml \
  up -d
```

隔离环境通过宿主机发布端口供项目服务使用；环境健康后仍单独执行 `docker-compose.yml` 启动项目。CI 和生产可使用独立项目名与线上覆盖组合两层编排，但不把环境服务重新塞入项目启动文件。

CI 和生产编排继续显式启用隔离依赖及其安全配置；本机开发以本机现有服务为准，不另外启动同类环境。

开发 Compose 中 Backend 通过 `host.docker.internal:8787` 调用私有 Agent，并通过 `host.docker.internal:7233` 连接本机 Temporal。生产环境必须显式提供私有网络内的 `AGENT_URL` 与 `TEMPORAL_ADDRESS`，并为 Backend/Agent 注入相同的高强度 `AGENT_EXECUTION_SECRET`；Agent 不接收数据库、JWT、Temporal 或对象存储凭据。

需要独立部署 Agent 时，从仓库根目录构建，镜像会固定安装 Codex CLI 并带入本项目所需的 Skill Pack：

```bash
docker build --file agent/Dockerfile --tag lanverse/agent-runtime:development .
```

运行镜像时仍须通过运行平台向容器用户 `/home/lanverse/.codex` 提供有效的 Codex 登录配置；镜像不会复制本机凭据。未提供登录配置时健康检查仍可成功，但生成请求会返回可追踪的 Agent 不可用结果。

对象存储区分 Backend 内部地址 `MINIO_ENDPOINT` 与 Browser 可达的 `MINIO_PUBLIC_ENDPOINT`。Bucket 保持私有，Browser 只使用短时预签名 URL。

## 验证

```bash
cd backend
test -z "$(gofmt -l .)"
go vet ./...
go test -count=1 -p 1 ./...

cd ../agent
uv run --all-extras ruff check app tests
uv run --all-extras ruff format --check app tests
uv run --all-extras pyright app tests
uv run --all-extras pytest -q

cd ../frontend
npm run openapi2ts
npm run lint
npm run typecheck
npm test
npm run build
```

最终 `agent-browser` 验收只在所有 StoryGraph 实施任务、真实依赖全旅程与自动化回归全部完成后执行；当前进度和未决风险以 [StoryGraph 验收标准](docs/acceptance/0010-StoryGraph内容图与DAG创作画布验收标准.md)为准。

本地隔离环境可通过 `.env.example` 的固定验证码完成注册测试；生产环境未接入验证码投递 Provider 前，自助注册不属于可用能力。
