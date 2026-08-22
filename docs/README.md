# Lanverse 软件工程文档导航

- 状态：active
- 最近审查：2026-08-22
- 当前状态：Requirement 已审核为 ready_for_design；000/005 顶层架构 accepted 并冻结，其余 Design、PRD 与 Plan 为 proposed

## 1. 当前产品边界

Lanverse 当前目标是一套可控制、可审阅、可恢复的 AI 连续视频生产平台。系统从创意或脚本开始，建立结构化叙事与生产知识，完成镜头规划、生成计划、候选比较、质量修复、版本审阅、基础装配和不可变交付。

系统不替代专业 NLE，不提供任意代码工作流，不承诺无审阅自动发布，也不建设定价、订阅、账单、支付和增长系统。资源用量只服务外部模型和媒体任务治理。

## 2. 当前正式文档链路

```text
Requirement
  → System / Domain / Product / Module Design
  → Slice PRD
  → Plan
  → Implementation & Test
  → Acceptance Evidence
```

Requirement 定义“要什么”，Design 定义“如何满足”，PRD 和 Plan 把目标组织成纵向交付切片，Acceptance 只保存真实执行证据。分析过程、审核记录和历史归档不再作为 `docs/` 内的独立文档层；任何有效结论必须进入上述五类正式事实源。

## 3. 文档入口

| 目录 | 作用 | 当前入口 |
| --- | --- | --- |
| design | 架构、领域数据、模块、接口工作流与模块详细设计 | [Design 索引](./design/README.md) |
| requirement | 用户结果、业务规则、边界和验收条件 | [Requirement 索引](./requirement/README.md) |
| prd | A—F 纵向交付切片的产品范围和发布 Gate | [PRD 索引](./prd/README.md) |
| plan | 与 PRD 对应的工作包、验证和停止条件 | [Plan 索引](./plan/README.md) |
| acceptance | 实施后的真实验收证据 | [Acceptance 状态](./acceptance/README.md) |

## 4. 推荐阅读顺序

1. [000 目标需求总览](./requirement/000-AI视频生产平台目标需求总览.md)与[M01—M15 详细 Requirement](./requirement/README.md)；
2. [000 目标系统架构](./design/000-AI视频生产平台目标系统架构设计.md)、[001 核心领域与数据](./design/001-AI视频生产平台核心领域与数据模型设计.md)和[002 产品模块](./design/002-AI视频生产平台目标产品与功能模块设计.md)；
3. [003 接口工作流](./design/003-AI视频生产平台接口工作流与功能实现设计.md)、[005 服务与模块实施基线](./design/005-AI视频生产平台服务与模块实施基线.md)与[M01—M15 模块详细 Design](./design/modules/README.md)；
4. [A—F PRD](./prd/README.md)与[实施 Plan](./plan/README.md)；
5. [Acceptance 状态与证据](./acceptance/README.md)。

## 5. 当前技术结论

- 顶层固定为 `frontend/` TypeScript/Next.js、`backend/` Go 业务系统、`agent/` Python/LangGraph Agent Runtime。
- 全部领域规则、业务事务、公共 `/api` 和非 Agent Worker 由 Go 实现；Python 只执行受限 AgentRun，不写业务数据库。
- 生产以五个 Go 角色 `api`、`operation-worker`、`import-worker`、`provider-worker`、`media-worker` 和一个 Python `agent-service` 运行，按 A—F 切片启用。
- PostgreSQL 保存业务事实和用户可见 Operation；Outbox 可靠触发平台唯一 Kafka 集群；MinIO 是唯一对象存储，Redis 只保存分布式协调运行态。Elasticsearch 固定为可重建业务检索数据面，OpenTelemetry 固定为观测数据面：首期可共用一次 Elastic 部署但逻辑隔离，ToC 生产前拆为独立故障域；二者都不替代 Outbox、审计或业务查询。
- Go backend 是唯一公共服务与消息治理方，也是唯一 Kafka/Redis 客户端边界；Python Agent 是不提供公共 API/Ingress 的内网计算微服务，只通过私有 Run HTTP 契约被 backend 调用。
- 公共接口以 `backend/api/openapi.json` 为唯一当前契约：Swagger UI、Go strict server 与 `@umijs/openapi` 前端 API 同源；前端统一经过 Axios `request.ts`，ViewModel 不手写 URL/DTO 或第二套 endpoint。
- 数据库只使用当前最终 Schema，不建立 migration、旧接口兼容、双读或双写链。

## 6. 目录约束

`docs/` 只保留 `requirement/`、`design/`、`prd/`、`plan/`、`acceptance/` 和本导航文件。研究过程、审核日志、追踪副本与历史归档不作为项目文档目录；需要保留的边界、决策、映射或证据必须分别写回 Design、Requirement、PRD/Plan 或 Acceptance。

## 7. 维护规则

- 当前五类正式目录中只保留一套编号和事实源；
- Requirement 使用稳定 ID 并包含成功、失败、权限、恢复和验收场景；
- Design 明确问题、范围、非目标、边界、数据、接口、状态和失败路径；
- Plan 只能从已接受的 Requirement 与 Design 形成，不能反向改变范围；
- accepted 表示评审通过，verified 表示已有真实执行证据，两者不得混用；
- 代码、Mock、字段占位和文档本身不能证明需求完成；
- 新的详细模块按依赖和实施优先级追加，不预建未来目录、服务或抽象层。
