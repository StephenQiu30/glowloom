# Lanverse 软件工程文档导航

- 状态：active
- 最近审查：2026-08-20
- 当前需求与设计状态：proposed

## 1. 当前产品边界

Lanverse 当前目标是一套可控制、可审阅、可恢复的 AI 连续视频生产平台。系统从创意或脚本开始，建立结构化叙事与生产知识，完成镜头规划、生成计划、候选比较、质量修复、版本审阅、基础装配和不可变交付。

系统不替代专业 NLE，不提供任意代码工作流，不承诺无审阅自动发布，也不建设定价、订阅、账单、支付和增长系统。资源用量只服务外部模型和媒体任务治理。

## 2. 当前正式文档链路

```text
Research
  → System Architecture
  → Domain & Data
  → Product Modules
  → Requirement
  → Interface / Workflow
  → Module Detail Design
  → Plan
  → Implementation & Test
  → Acceptance Evidence
```

阅读从架构到模块展开；变更治理仍遵守 Requirement 定义“要什么”、Design 定义“如何满足”。如果架构研究暴露新的用户风险，必须回到 Requirement 形成可观察结果和验收条件，不能只留在技术文档中。

## 3. 文档入口

| 目录 | 作用 | 当前入口 |
| --- | --- | --- |
| research | 公开事实、固定证据、产品模式和待验证推断 | [AI 短剧研究索引](./research/ai-short-drama/README.md) |
| design | 架构、领域数据、模块、接口工作流与模块详细设计 | [Design 索引](./design/README.md) |
| requirement | 用户结果、业务规则、边界和验收条件 | [Requirement 索引](./requirement/README.md) |
| plan | 已接受需求与设计形成的纵向实施计划 | [Plan 状态](./plan/README.md) |
| acceptance | 实施后的真实验收证据 | [Acceptance 状态](./acceptance/README.md) |
| archive | 被替代但仍需追溯的历史文档 | [归档索引](./archive/README.md) |

## 4. 推荐阅读顺序

1. [000 目标系统架构](./design/000-AI视频生产平台目标系统架构设计.md)；
2. [001 核心领域与数据模型](./design/001-AI视频生产平台核心领域与数据模型设计.md)；
3. [002 目标产品与功能模块](./design/002-AI视频生产平台目标产品与功能模块设计.md)；
4. [000 目标需求总览](./requirement/000-AI视频生产平台目标需求总览.md)；
5. [M01—M15 详细 Requirement 索引](./requirement/README.md)；
6. [003 接口、工作流与功能实现](./design/003-AI视频生产平台接口工作流与功能实现设计.md)；
7. [004 剧本基础分析与人物拆解详细设计](./design/004-AI视频生产平台剧本基础分析与人物拆解详细设计.md)。

## 5. 当前技术结论

- 后端以 Python 为唯一业务主栈；FastAPI 承载短请求与短事务。
- Agent、Provider 和媒体任务由独立 Python Worker 执行，LangGraph 只编排单次 Agent 运行。
- PostgreSQL 保存业务事实和用户可见操作状态；Outbox 可靠触发 RabbitMQ 后台任务。
- 首期不引入 Go。只有真实容量、资源或尾延迟证据满足架构 Gate 后才进行局部 Go PoC。

## 6. 历史文档处理

旧 Requirement、DES-000—017、Agent Harness Plan 和旧 MVP Acceptance 已整体移动到 [v1 归档](./archive/v1/README.md)。它们没有被删除，但已退出当前事实源；旧 `CUR-*`、`DES-*` 和完成状态不能证明当前目标方案已接受或已实现。

## 7. 维护规则

- 当前正式目录中只保留一套编号和事实源；历史文件只进入 archive；
- Requirement 使用稳定 ID 并包含成功、失败、权限、恢复和验收场景；
- Design 明确问题、范围、非目标、边界、数据、接口、状态和失败路径；
- Plan 只能从已接受的 Requirement 与 Design 形成，不能反向改变范围；
- accepted 表示评审通过，verified 表示已有真实执行证据，两者不得混用；
- 代码、Mock、字段占位和文档本身不能证明需求完成；
- 新的详细模块按依赖和实施优先级追加，不预建未来目录、服务或抽象层。
