# Lanverse

Lanverse 是面向 AI 短剧生产的端到端 AI Native Production Platform。产品通过 Guided Studio 与 Canvas Studio 提供两种创作入口，并由同一套 Authoring、Workflow、Agent、Generation、Media 与 Artifact 体系完成从剧本到成片的生产闭环。

## 目标架构

最新目标固定为：

- `frontend/`：TypeScript Monorepo，承载 Web、协作服务、UI、AI UI、Canvas、SDK 与认证组件。
- `backend/`：一个 Go Module、多个领域模块和五个运行程序。
- `agent/`：独立 Python Agent Runtime，提供受限的 Structured、Tool Loop 与 LangGraph Executor。
- `docs/`：产品、架构、详细设计、计划、ADR 与验收的长期事实来源。
- PostgreSQL、Temporal、Redis、MinIO、Kafka 与 Elasticsearch 分别承担业务事实、工作流历史、临时状态、对象、事件和可重建检索投影。

当前仓库仍处于从 FastAPI 模块化单体和单 Next.js 应用迁移到该目标架构的阶段。迁移期间不允许同一资源出现两个写入所有者，也不将目标目录误报为已实现能力。

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

完整原始设计已作为 [AI 短剧制作平台完整设计基线](docs/designs/AI短剧制作平台完整设计基线.md) 持久化；来源校验信息记录在文档导航中。后续架构变化必须先更新对应设计或 ADR，再修改实现。
