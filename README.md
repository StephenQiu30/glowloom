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
        └─────────→ Python Agent 服务（编排 + Harness）─→ 容器内 Codex CLI

JSON Logs → Logstash → Elasticsearch Log Index → Kibana
```

- `frontend/`：Next.js 创作工作台，只读取服务端事实并提交人工决议。
- `backend/`：唯一公共业务 API 与唯一业务 Writer；认证、项目、剧本、制作圣经、分集、结构、分镜、正式镜头、导出和持久任务都在此实现。
- `backend/cmd/main.go`：唯一 Go 启动入口；同一 `lanverse` 进程装配 API、Workflow 与 Event 三个职责运行时，不创建 Worker Binary 或 Compose 服务。
- `agent/`：一个 Python Agent 服务，内部包含 Creation 编排、运行库、失败恢复和受限 Harness 模块；正式业务事实仍由 Go 写入，模型子进程不会继承数据库或平台凭据。
- `backend/internal/platform/database/model`：唯一 GORM Model Catalog 与表结构事实源。
- `backend/api/openapi/lanverse-public-api.json`：唯一公共 REST 契约源。
- `backend/internal/agent/contract`：Backend ↔ Agent 的版本化调用/结果线协议所有者；`agent/app/harness/schemas.py` 以禁止额外字段的 Pydantic 模型校验同一协议。
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

按 [.env.example](.env.example) 填写根目录 `.env`，然后执行：

```bash
docker compose up -d --build
docker compose ps
```

访问前端 <http://127.0.0.1:8123>，后端 <http://127.0.0.1:8686>。查看应用日志：

```bash
docker compose logs --tail=50 backend frontend agent
```

默认只启动三个应用服务：Frontend、Backend 和 Agent。Go API、Workflow、Event Runtime 共用一个 Backend 容器；Agent 在同一容器内提供命令 API、Temporal 编排和 Harness 执行，私有端口不对宿主机发布。

这些都是单实例服务，Docker Desktop 中显示为 `lanverse-frontend`、`lanverse-backend` 和 `lanverse-agent`，不使用 Compose 自动追加的 `-1` 实例序号。Agent 内部模块不是额外容器，Backend 通过 `agent:8787` 访问统一入口。

PostgreSQL、MinIO、Temporal、Kafka、ES 和 Logstash 直接使用本机已启动的服务，容器通过 `host.docker.internal` 访问。填写真实地址和认证信息即可；本地启动不创建基础设施、初始化索引或清除历史数据。MinIO 的内部地址与浏览器访问地址分别使用 `MINIO_ENDPOINT` 和 `MINIO_PUBLIC_ENDPOINT`。

Agent 使用已迁移的独立数据库、独立签名密钥和已审阅的冻结 SkillRelease 摘要，只读挂载已登录 Codex 的 `auth.json`；配置说明与恢复约束见 [Agent 单服务设计](docs/design/0021-Agent单服务架构调整设计.md) 和 [部署设计](docs/design/0020-文本解析失败诊断与受控恢复设计.md)。媒体供应商配置需要另行填写本机 root-key 文件路径。

CI 的一次性依赖、连接覆盖和观测配置仅保存在 `.github/ci/`，不会被本地启动加载。本地和部署均使用根目录 `docker-compose.yml`，不再维护额外的 deploy 覆盖层。

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

本地隔离环境可通过 `.env.example` 的固定验证码完成注册测试。真实注册邮件使用 SMTP：将 `REGISTRATION_VERIFICATION_CODE` 留空，配置 `SMTP_ENABLED=true`、服务商地址、TLS 模式、账号、授权码和发件人信息；生产 Compose 会强制要求完整 SMTP 配置。固定验证码与 SMTP 同时启用时 Backend 会拒绝启动，避免把公开测试码发送到真实邮箱。
