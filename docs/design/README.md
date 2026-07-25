# Design 文档

本目录把 AI 短剧 MVP 的 Requirement 拆成横切设计、业务模块设计和追踪矩阵。当前 Requirement 仍为 `review`，因此全部 Design 保持 `draft`；只有上游 Requirement 全部 `accepted` 且 `design_entry` 通过后，才可依次接受 PRD 与 Plan。本文档拆分不授权创建应用代码或 Acceptance。

## 上游需求

[01 MVP目标与范围](../requirement/01-MVP目标与范围.md) → [02 项目与故事输入](../requirement/02-项目与故事输入.md) → [03 剧本分镜与创作资产](../requirement/03-剧本分镜与创作资产.md) → [04 AI媒体任务与候选采用](../requirement/04-AI媒体任务与候选采用.md) → [05 字幕合成与成片交付](../requirement/05-字幕合成与成片交付.md) → [06 MVP质量标准](../requirement/06-MVP质量标准.md) → [07 技术与工程约束](../requirement/07-技术与工程约束.md) → [08 实现准入条件](../requirement/08-实现准入条件.md)

## 按序设计目录

| 顺序 | 文档 | 类型 | 唯一责任 |
| --- | --- | --- | --- |
| 01 | [MVP技术选型与范围](01-MVP技术选型与范围.md) | 横切 | MVP allowlist、技术族、运行单元和回滚 |
| 02 | [MVP总体架构](02-MVP总体架构.md) | 横切 | 端到端流程、六模块事实所有权与运行拓扑 |
| 03 | [任务执行与媒体处理](03-任务执行与媒体处理.md) | 横切 | 快照、TaskJob、Provider、MinIO、TTS、FFmpeg |
| 04 | [接口数据与项目结构](04-接口数据与项目结构.md) | 横切 | OpenAPI、工程目录、依赖方向与测试边界 |
| 05 | [端到端制作流程](05-端到端制作流程.md) | 横切 | 唯一纵向闭环、阶段出口与恢复路径 |
| 06 | [数据库表与迁移设计](06-数据库表与迁移设计.md) | 横切 | 20 张表、逐表 SQL、事务锁与迁移 |
| 07 | [项目与来源模块设计](07-项目与来源模块设计.md) | 模块 | Project、Episode、SourceRevision |
| 08 | [剧本分镜与创作资产模块设计](08-剧本分镜与创作资产模块设计.md) | 模块 | Script、CreativeAsset、ShotSpec |
| 09 | [生产任务与恢复模块设计](09-生产任务与恢复模块设计.md) | 模块 | Snapshot、Task、Attempt、TaskEvent、TaskJob |
| 10 | [AI生成媒体与候选采用模块设计](10-AI生成媒体与候选采用模块设计.md) | 模块 | AI Registry、Media、Candidate、Adoption |
| 11 | [字幕渲染与交付模块设计](11-字幕渲染与交付模块设计.md) | 模块 | Subtitle、RenderSnapshot、Delivery |
| 12 | [前端工作区与交互模块设计](12-前端工作区与交互模块设计.md) | 模块 | 五个工作区、Redux/RTK Query、轮询恢复 |
| 13 | [需求实现追踪](13-需求实现追踪.md) | 追踪 | Requirement→Design→PRD→Plan→Evidence |

## 设计解释顺序

1. DESIGN-01～06 是公共约束，模块 Design 不重复定义技术栈、表字段、HTTP 通用规则或状态枚举。
2. DESIGN-07～12 分别固定模块用例、公开端口、数据/状态不变式、事务、失败、安全、迁移和可执行 Design AC。
3. DESIGN-13 是唯一追踪矩阵；新增能力先进入 Requirement，再更新所属模块 Design 和追踪，不在 PRD/Plan 中越级造需求。
4. [数据库表与迁移设计](06-数据库表与迁移设计.md) `accepted` 且 `database_design_ready` 通过前，根 SQL 只允许静态评审，不得执行 DDL、脚手架或源码创建。

## 目录与状态边界

应用根 allowlist 只有 `backend/` 与 `frontend/`；根 Compose 只管理 frontend/backend-api/backend-worker。CI 必须拒绝 `deploy/`、其他顶层应用根、独立 Worker 仓库和空模块。正式工作严格按 `Design → PRD → Plan → Acceptance` 推进。

文件按依赖顺序使用 `NN-语义主题.md`；正文标题只显示语义主题。`DESIGN-NN` 与条款级 AC 只用于元数据和追踪。活动状态只使用 `draft/review/accepted`；验证结果只使用 `passed/failed/insufficient/not_applicable`。
