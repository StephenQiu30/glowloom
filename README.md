# Lanverse

Lanverse 是面向 AI 短剧生产的端到端 AI Native Production Platform。产品通过 Guided Studio 与 Canvas Studio 提供两种创作入口，并由同一套 Authoring、Workflow、Agent、Generation、Media 与 Artifact 体系完成从剧本到成片的生产闭环。

## 目标架构

最新目标固定为：

- `frontend/`：TypeScript Monorepo，承载 Web、协作服务、UI、AI UI、Canvas、SDK 与认证组件。
- `backend/`：一个 Go Module、多个领域模块和五个运行程序。
- `agent/`：独立 Python Agent Runtime，提供受限的 Structured、Tool Loop 与 LangGraph Executor。
- `docs/`：PRD、Design、Requirement、Plan 与 Acceptance 的长期事实来源。
- PostgreSQL、Temporal、Redis、MinIO、Kafka 与 Elasticsearch 分别承担业务事实、工作流历史、临时状态、对象、事件和可重建检索投影。

当前仓库已进入后端语言切换阶段：`backend/` 是 Go Module，`agent/` 暂时承载迁入的 FastAPI 兼容运行时与 Python Agent 能力，Frontend 仍是单 Next.js 应用。迁移期间不允许同一资源出现两个写入所有者，也不将目录移动误报为业务能力已经迁移。

## 文档入口

- [文档导航](docs/README.md)
- [产品范围与验收基线](docs/prd/0001-产品范围与验收基线.md)
- [平台完整设计基线](docs/design/0001-AI短剧制作平台完整设计基线.md)
- [系统总体架构](docs/design/0003-系统总体架构.md)
- [前端应用架构](docs/design/1001-前端应用架构.md)
- [后端服务架构](docs/design/2001-后端服务架构.md)
- [平台迁移与服务整合计划](docs/plan/0007-平台迁移与服务整合计划.md)
- [资源写入所有权迁移台账](docs/plan/0008-资源写入所有权迁移台账.md)

完整设计已作为 [AI 短剧制作平台完整设计基线](docs/design/0001-AI短剧制作平台完整设计基线.md) 持久化。后续架构变化必须先更新对应 Design 及其决策记录，再修改实现。

## 当前可运行目录切片

第一阶段已建立可验证的语言与运行边界：

- `frontend`：Next.js Web 入口。
- `backend`：唯一 Go `lanverse-api` 公共入口；启动时在事务与 Advisory Lock 内应用内嵌版本化 Migration，完成后才开放健康端点。迁移元数据隔离在 `lanverse_migration` Schema，不再运行独立 Migration 服务。
- `agent-api`：迁入 `agent/` 的 FastAPI 兼容运行时，当前仍是业务写入所有者，不对宿主机暴露端口。
- `schedule-dispatcher` 与 `outbox-publisher`：使用同一 Agent 镜像，但拥有独立生命周期；旧 Publisher 通过 `OUTBOX_TOPICS` 只领取 `lanverse.io.v1` 与 `lanverse.media.v1`。
- `io-worker` 与 `media-worker`：延续现有 Kafka 消费职责。
- PostgreSQL、Redis、MinIO 与 Kafka：提供当前代码已经依赖的有状态基础设施。

当前 `agent/` 仍包含数据库和 Worker 兼容代码，尚未达到最终受限 Agent Runtime 的安全边界。每个业务路由迁入 Go 并完成契约、数据、排空与回滚验收后，才删除对应 Python Writer；Temporal、Elasticsearch、Debezium 和可观测性栈会在真实消费者落地时接入，不预建空服务。

当前统一使用 Compose 作为服务启动入口。先根据 `.env.example` 准备根目录 `.env`，然后运行：

```bash
docker compose --env-file .env \
  -f docker-compose.yml \
  -f docker-compose-env.yml \
  up --build -d
```

`backend` 等待 PostgreSQL 健康后先执行唯一 Go Migration 流，再启动公共 API；`agent-init` 只在 Backend 健康后执行只读 Schema Guard，随后启动内部 Agent API 与 Workers。未登记版本且已有业务表的数据库会被拒绝，不会自动执行兼容或覆盖。

对象存储同时声明容器内部地址 `MINIO_ENDPOINT` 与浏览器可达地址 `MINIO_PUBLIC_ENDPOINT`。前者用于服务端读写和健康检查，后者只用于签发短期上传/下载 URL；开发 MinIO 同时连接 `internal` 与 `edge` 网络并只向宿主机回环地址发布端口。远程或生产环境必须把公开地址配置为实际 HTTPS 域名，不能使用 Compose 内部主机名。

Go 公共入口的运行端点是 `/healthz`、`/readyz`、`/metrics` 与 `/version`。其中 Readiness 会探测内部 Agent 兼容 API，Metrics 只使用方法、固定路由和状态族等有界标签，Version 由构建参数注入且不包含 Secret。

生产部署还需叠加 `docker-compose-prod.yml`，并显式提供 Frontend、Go Backend、Agent 三类已发布镜像和所有生产 Secret。生产 `agent-init` 只校验 Agent 必需表/列和允许的 Migration 版本窗口，不会调用 `create_all`。
