# Plan 文档

本目录把已接受前才可执行的 Design/PRD 拆成一个总计划和八个专业子计划。PLAN-01 只负责编排依赖、门禁、提交纪律和 Evidence；PLAN-02～09 分别拥有明确路径、Red、最小 Green、命令和完成定义。

当前 Requirement、Design、PRD 尚未 `accepted`，因此全部 Plan 保持 `draft`，不得执行脚手架、依赖安装、DDL、应用源码或 Acceptance。数据库物理设计还必须先取得 `database_design_ready: passed`，全部计划接受后再取得 `implementation_start: passed`。

## 按执行顺序

| 顺序 | 文档 | 交付责任 | Evidence |
| --- | --- | --- | --- |
| 01 | [MVP总执行计划](01-MVP总执行计划.md) | 总依赖、门禁、里程碑、提交与回滚 | EV-000～010 目录 |
| 02 | [工程基线与架构计划](02-工程基线与架构计划.md) | 官方脚手架、模块边界规则、两个入口、三服务 Compose | EV-001 |
| 03 | [数据库与API契约计划](03-数据库与API契约计划.md) | 逐表 SQL 迁移、asyncpg、OpenAPI/umi 链 | EV-000～002 |
| 04 | [项目与来源实现计划](04-项目与来源实现计划.md) | Project/Episode/Source | EV-003 |
| 05 | [生产任务与恢复实现计划](05-生产任务与恢复实现计划.md) | Task/Attempt/TaskJob、幂等与恢复 | EV-004 |
| 06 | [剧本分镜与创作资产实现计划](06-剧本分镜与创作资产实现计划.md) | Script/Assets/Storyboard | EV-003 |
| 07 | [AI媒体与候选采用实现计划](07-AI媒体与候选采用实现计划.md) | AI Registry、Media、Candidate、Adoption | EV-005 |
| 08 | [字幕渲染与交付实现计划](08-字幕渲染与交付实现计划.md) | Subtitle、FFmpeg、Delivery | EV-006 |
| 09 | [前端工作区与端到端验收计划](09-前端工作区与端到端验收计划.md) | 五工作区、E2E、真实 smoke 与追踪 | EV-007～010 |

## 专业任务结构

每份子计划必须包含：

1. 已接受输入、准入门禁、目标结果、修改 allowlist 和明确非目标。
2. fixture/环境/依赖及其最迟关闭点。
3. TDD 任务表：Task ID、前置、预期 Red、最小 Green、目标路径、提交类型、命令、Test ID、Evidence ID。
4. AC→Test→Evidence 矩阵，以及迁移、故障恢复、风险和回滚。
5. Definition of Done；无法给出命令时说明原因并提供最接近的可执行检查。

## 执行规则

- 行为任务按 `test:`→`impl:/feat:`→可选 `refactor:/docs:/chore:`，Red 必须证明需求尚未满足。
- 子计划只能修改自身 allowlist；跨模块通过 `public.py`，额外能力先回到 Requirement/Design/PRD。
- 根 Makefile 是唯一跨生态命令入口；结果在实现后写入 ACCEPTANCE-01，不在 Plan 中伪造。
- 未映射活动输入、Test ID 和 Evidence ID 的代码、接口、表、依赖、Feature Flag 或占位实现不得提交。
- 任一 P0 缺少可运行验证、恢复或回滚时，相关 Plan 不得转为 `accepted`。
