# Lanverse

Lanverse 是面向 AI 短剧生产的端到端 AI Native Production Platform。产品通过 Guided Studio 与 Canvas Studio 提供两种创作入口，并由同一套 Authoring、Workflow、Agent、Generation、Media 与 Artifact 体系完成从剧本到成片的生产闭环。

## 目标架构

最新目标固定为：

- `frontend/`：TypeScript Monorepo，承载 Web、协作服务、UI、AI UI、Canvas、SDK 与认证组件。
- `backend/`：一个 Go Module、多个领域模块和五个运行程序。
- `agent/`：独立 Python Agent Runtime，提供受限的 Structured、Tool Loop 与 LangGraph Executor。
- `docs/`：产品、架构、详细设计、计划、ADR 与验收的长期事实来源。
- PostgreSQL、Temporal、Redis、MinIO、Kafka 与 Elasticsearch 分别承担业务事实、工作流历史、临时状态、对象、事件和可重建检索投影。

当前仓库已进入后端语言切换阶段：`backend/` 是 Go Module，`agent/` 暂时承载迁入的 FastAPI 兼容运行时与 Python Agent 能力，Frontend 仍是单 Next.js 应用。迁移期间不允许同一资源出现两个写入所有者，也不将目录移动误报为业务能力已经迁移。

## 文档入口

- [文档导航](docs/README.md)
- [产品范围与验收基线](docs/prd/产品范围与验收基线.md)
- [系统总体架构](docs/architecture/系统总体架构.md)
- [前端应用架构](docs/architecture/前端应用架构.md)
- [后端服务架构](docs/architecture/后端服务架构.md)
- [架构分层与依赖规则](docs/architecture/架构分层与依赖规则.md)
- [领域语言与模块命名规范](docs/architecture/领域语言与模块命名规范.md)
- [后端领域模块功能设计](docs/designs/后端领域模块功能设计.md)
- [前端功能模块设计](docs/designs/前端功能模块设计.md)
- [平台迁移与服务整合计划](docs/plans/平台迁移与服务整合计划.md)
- [资源写入所有权迁移台账](docs/plans/资源写入所有权迁移台账.md)
- [迁移期后端语言切换策略](docs/adr/迁移期后端语言切换策略.md)

完整原始设计已作为 [AI 短剧制作平台完整设计基线](docs/designs/AI短剧制作平台完整设计基线.md) 持久化；来源校验信息记录在文档导航中。后续架构变化必须先更新对应设计或 ADR，再修改实现。

## 当前可运行目录切片

第一阶段已建立可验证的语言与运行边界：

- `frontend`：Next.js Web 入口。
- `backend`：Go `lanverse-api` 公共入口，自行提供健康、就绪、指标与构建版本接口；尚未迁移的业务路由透明转发到内部兼容服务。
- `database-migrate`：使用同一 Go Backend 镜像运行 `lanverse-migrate`，在事务与 Advisory Lock 内应用唯一版本化 Migration；迁移元数据隔离在 `lanverse_migration` Schema。
- `agent-api`：迁入 `agent/` 的 FastAPI 兼容运行时，当前仍是业务写入所有者，不对宿主机暴露端口。
- `schedule-dispatcher` 与 `outbox-publisher`：使用同一 Agent 镜像，但拥有独立生命周期；旧 Publisher 通过 `OUTBOX_TOPICS` 只领取 `lanverse.io.v1` 与 `lanverse.media.v1`。
- `io-worker` 与 `media-worker`：延续现有 Kafka 消费职责。
- PostgreSQL、Redis、MinIO 与 Kafka：提供当前代码已经依赖的有状态基础设施。

当前 `agent/` 仍包含数据库和 Worker 兼容代码，尚未达到最终受限 Agent Runtime 的安全边界。每个业务路由迁入 Go 并完成契约、数据、排空与回滚验收后，才删除对应 Python Writer；Temporal、Elasticsearch、Debezium 和可观测性栈会在真实消费者落地时接入，不预建空服务。

本机开发默认复用已安装的 PostgreSQL、Redis、MinIO 与 Kafka。先根据 `.env.example` 准备根目录 `.env`，并安装 Agent/Frontend 依赖，然后运行：

```bash
./scripts/run-local-development.sh
```

启动器会在本机运行 Python 兼容 API、Go Backend 和 Frontend，不创建基础设施容器，也不会启动 Kafka Consumer/Publisher 去领取本机共享 Topic 中的旧任务。为避免旧 Schema 被覆盖，它默认在同一个本机 PostgreSQL 实例中使用 `LOCAL_DATABASE_NAME=lanverse_development` 的隔离数据库；不会修改 `.env` 中原 `DATABASE_URL` 指向的数据库。隔离开发库与测试库允许由 Python 测试辅助函数从当前 Metadata 初始化；任何共享或生产数据库都只能由 Go Migration 写 Schema。

Go 公共入口的运行端点是 `/healthz`、`/readyz`、`/metrics` 与 `/version`。其中 Readiness 会探测内部 Agent 兼容 API，Metrics 只使用方法、固定路由和状态族等有界标签，Version 由构建参数注入且不包含 Secret。

需要 Scheduler/Worker 的执行链或完全隔离的全栈验收环境时运行：

```bash
docker compose --env-file .env.example \
  -f docker-compose.yml \
  -f docker-compose-env.yml \
  up --build -d
```

Compose 会先执行 `database-migrate`，成功后再运行只读 Schema Guard 和其他服务。生产部署还需叠加 `docker-compose-prod.yml`，并显式提供 Frontend、Go Backend、Agent 三类已发布镜像和所有生产 Secret。生产 `agent-init` 只校验 Agent 必需表/列和允许的 Migration 版本窗口，不会调用 `create_all`。

首次接管已有、尚未登记版本的兼容数据库时，必须先使用新 Agent 镜像执行一次严格基线审计；审计失败不得强制登记：

```bash
docker compose --env-file .env.example \
  -f docker-compose.yml \
  -f docker-compose-env.yml \
  run --rm --no-deps agent-init \
  python -m app.runtime.commands.database adopt-baseline
```

登记后正常启动 Compose 即可。旧运行时只检查 `public`，迁移元数据位于独立 Schema，因此该动作不会破坏旧版本的回滚启动能力。
