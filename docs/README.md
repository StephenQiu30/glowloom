# Lanverse 软件工程文档导航

- 状态：active
- 最近审查：2026-08-22
- 当前状态：Requirement 已审核为 ready_for_design；Design、PRD 与 Plan 为 proposed

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

- 后端以 Python 为唯一业务主栈；FastAPI 承载短请求与短事务。
- 生产以 `api`、`operation-worker`、`import-worker`、`provider-worker`、`agent-worker`、`media-worker` 六类安全隔离入口运行，按 A—F 切片逐步启用；LangGraph 只编排单次 Agent 运行。
- PostgreSQL 保存业务事实和用户可见操作状态；Outbox 可靠触发 RabbitMQ 后台任务。
- 首期不引入 Go。只有真实容量、资源或尾延迟证据满足架构 Gate 后才进行局部 Go PoC。

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
