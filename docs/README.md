# Lanverse 软件工程文档导航

- 状态：active
- 最近审查：2026-08-23
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

## 5. 实现追溯状态

本表是 Requirement → Design → PRD → Plan → Acceptance 的执行入口，不把“已写入文档”误报为“已验收”。验收状态只以 `acceptance/README.md` 中的真实证据包为准，未产生真实证据前保持 `not_run`。

| 范围 | 需求与设计 | PRD | Plan | 当前实现状态 |
| --- | --- | --- | --- | --- |
| 平台基线 | REQ-000、DES-000—005 | PRD-000 | PLAN-000 | 进行中：Go/Agent/frontend 固定，GORM、统一 HTTP envelope/状态码、无兼容 DDL 的空库 Schema、单 Kafka、MinIO、OpenAPI、HTTP 到事务的 workspace context、直接租户表 RLS 与策略启动校验已落地；Redis 身份接入已启用，Elastic 投影、OTel、严格 Swagger 仍待实现 |
| A 手工事实主线 | M01—M05、M07—M09、M11、M14 | PRD-A | PLAN-A | 进行中：DOCX/Markdown/TXT 原件保全、确定性 ParseReport、剧本解析草稿/批准、canonical GORM 物化、人物×集数与资产投影、内存 Access Token/HttpOnly refresh、URL 三元定位和服务端正式项目列表恢复已完成 production 浏览器闭环；成员角色变更、会话撤销、可恢复审计基线及访问审计关键字/主体/对象/动作/结果/时间筛选与分页已落地并通过管理端 production 验证，会话/路径/直接表租户边界、Worker 消息租户/outbox 绑定、逻辑 Artifact/精确对象版本分离及 Fixture 顺序幂等已落地，多角色并发初始化 MinIO Bucket 的启动竞态已修复；项目职责、紧急恢复、完整项目 Brief/生命周期/概览、媒体授权负向验收、边界手工修订、检索、完整镜头与交付预演待实现 |
| B Agent 提案 | M03、M04、M06 | PRD-B | PLAN-B | 未实现：当前 Python 仅提供私有确定性 Harness 骨架 |
| C 真实可恢复生成 | M07—M09、M11、M14 | PRD-C | PLAN-C | 未实现：真实 Provider、Usage 与完整 Candidate/Selection 状态机尚未启用；当前仅保留 Fixture Candidate/Selection 试点闭环 |
| D 质量与修复 | M04、M09、M10、M14 | PRD-D | PLAN-D | 未实现 |
| E 审阅与交付 | M10、M12、M13、M14 | PRD-E | PLAN-E | 未实现 |
| F 团队与扩展 | M01、M06、M15 | PRD-F | PLAN-F | 未实现 |

每完成一个工作包，必须同时更新本表、自动化测试和 Acceptance 证据。所有公共接口继续使用当前 `/api` 契约；PostgreSQL 是业务事实，MinIO 是对象存储，Go backend 是唯一 Kafka 接入方，Python Agent 不接入 Kafka、Redis、Elastic 或业务数据库。

## 6. 当前技术结论

- 顶层固定为 `frontend/` TypeScript/Next.js、`backend/` Go 业务系统、`agent/` Python/LangGraph Agent Runtime。
- 全部领域规则、业务事务、公共 `/api` 和非 Agent Worker 由 Go 实现；Python 只执行受限 AgentRun，不写业务数据库。
- 生产以五个 Go 角色 `api`、`operation-worker`、`import-worker`、`provider-worker`、`media-worker` 和一个 Python `agent-service` 运行，按 A—F 切片启用。
- PostgreSQL 保存业务事实和用户可见 Operation；Outbox 可靠触发平台唯一 Kafka 集群；MinIO 是唯一对象存储，Redis 只保存分布式协调运行态。Elasticsearch 固定为可重建业务检索数据面，OpenTelemetry 固定为观测数据面：首期可共用一次 Elastic 部署但逻辑隔离，ToC 生产前拆为独立故障域；二者都不替代 Outbox、审计或业务查询。
- Go backend 是唯一公共服务与消息治理方，也是唯一 Kafka/Redis 客户端边界；Python Agent 是不提供公共 API/Ingress 的内网计算微服务，只通过私有 Run HTTP 契约被 backend 调用。
- 公共接口以 Go Controller Swagger 注释生成的 `backend/docs/swagger.json` 为当前文档契约：Swagger UI 与 `@umijs/openapi` 前端 API 同源；前端统一经过 Axios `request.ts`，ViewModel 不手写 URL/DTO 或第二套 endpoint。
- 数据库只使用当前最终 Schema，不建立 migration、旧接口兼容、双读或双写链。

## 7. 目录约束

`docs/` 只保留 `requirement/`、`design/`、`prd/`、`plan/`、`acceptance/` 和本导航文件。研究过程、审核日志、追踪副本与历史归档不作为项目文档目录；需要保留的边界、决策、映射或证据必须分别写回 Design、Requirement、PRD/Plan 或 Acceptance。

## 8. 维护规则

- 当前五类正式目录中只保留一套编号和事实源；
- Requirement 使用稳定 ID 并包含成功、失败、权限、恢复和验收场景；
- Design 明确问题、范围、非目标、边界、数据、接口、状态和失败路径；
- Plan 只能从已接受的 Requirement 与 Design 形成，不能反向改变范围；
- accepted 表示评审通过，verified 表示已有真实执行证据，两者不得混用；
- 代码、Mock、字段占位和文档本身不能证明需求完成；
- 新的详细模块按依赖和实施优先级追加，不预建未来目录、服务或抽象层。
