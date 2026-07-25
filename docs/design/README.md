# Design 文档

本目录是 AI 短剧制作 MVP 的完整 Design allowlist。实现目标只有一条：以可执行、可恢复、可验收的纵向链路，把输入文本制作成可播放 MP4。实现代码、接口、数据表和测试都必须能追溯到下表文档及其验收标准。

当前 Design 仅作为 Requirement 评审的下游草案，不具有阶段放行效力。适用 Requirement 全部 `accepted` 后，Design 才能进入正式评审并转为 `accepted`；通过 [实现准入条件](../requirement/08-实现准入条件.md) 的 `design_entry` 后才进入 PRD。只有 `PRODUCT-01`、`PLAN-01` 依序 `accepted` 且 `implementation_start` 通过后才能创建应用目录和实现；`ACCEPTANCE-01` 只能在实现完成后创建。

## 上游需求

[01 MVP目标与范围](../requirement/01-MVP目标与范围.md) → [02 项目与故事输入](../requirement/02-项目与故事输入.md) → [03 剧本分镜与创作资产](../requirement/03-剧本分镜与创作资产.md) → [04 AI媒体任务与候选采用](../requirement/04-AI媒体任务与候选采用.md) → [05 字幕合成与成片交付](../requirement/05-字幕合成与成片交付.md) → [06 MVP质量标准](../requirement/06-MVP质量标准.md) → [07 技术与工程约束](../requirement/07-技术与工程约束.md) → [08 实现准入条件](../requirement/08-实现准入条件.md)

## 按序设计目录

| 顺序 | 文档 | 状态 | 唯一责任 |
| --- | --- | --- | --- |
| 01 | [MVP技术选型与范围](01-MVP技术选型与范围.md) | draft | 定义 MVP 范围、技术族、运行单元和回滚规则 |
| 02 | [MVP总体架构](02-MVP总体架构.md) | draft | 端到端流程、六模块事实所有权、前后端与部署边界 |
| 03 | [任务执行与媒体处理](03-任务执行与媒体处理.md) | draft | 快照、Task/Attempt/TaskJob、Provider、TTS、FFmpeg 与恢复 |
| 04 | [接口数据与项目结构](04-接口数据与项目结构.md) | draft | OpenAPI、轮询、内部事件、逻辑数据、前后端目录、模式和测试边界 |
| 05 | [AI短剧端到端制作流程](05-端到端制作流程.md) | draft | 唯一交付切片及其用户闭环、失败路径和 SMART Design AC |
| 06 | [数据库表与迁移设计](06-数据库表与迁移设计.md) | draft | 20 张应用表数据字典、FK/候选键、约束索引、事务锁、Alembic 迁移、三份 JSONB Schema 与机器 exact-set 门禁 |
| 07 | [需求实现追踪](07-需求实现追踪.md) | draft | Requirement→Design→PRD/Plan/Test/Acceptance 双向追踪 |

## 唯一解释规则

- [02 MVP总体架构](02-MVP总体架构.md)是模块名称、事实所有权、运行单元和页面边界的唯一设计源。
- [03 任务执行与媒体处理](03-任务执行与媒体处理.md)是异步状态、恢复、Provider 和媒体渲染语义的唯一设计源。
- [04 接口数据与项目结构](04-接口数据与项目结构.md)是路径、HTTP 契约、轮询、内部事件、逻辑数据、工程目录和设计模式边界的唯一设计源。
- [05 AI短剧端到端制作流程](05-端到端制作流程.md)只组合前三份核心设计形成一个纵向切片，不另造状态、模块或接口。
- [06 数据库表与迁移设计](06-数据库表与迁移设计.md)是物理表、列、关系、约束、索引、JSONB、事务锁和迁移的唯一设计源；它 `accepted` 且 `database_design_ready` 通过前不得运行任何脚手架或创建源码。
- 基础依赖精确版本由 `PLAN-01` T-001 在数据库门禁后随脚手架锁定；真实 Provider、模型和必需 SDK 由 T-008 真实 smoke 前置条件确定。Design 不以占位实现代替任一门禁。

## 目录边界

应用根目录 allowlist 只有 `backend/` 与 `frontend/`；根 Compose 不是应用目录，且只管理 frontend/backend-api/backend-worker。CI 必须拒绝 `deploy/`、其他顶层应用根、独立 Worker 仓库和空模块。正式文档按 `Design → PRD → Plan → Acceptance` 推进，不倒序建立 Acceptance。

## 命名与状态

文件按依赖顺序使用 `NN-语义主题.md` 命名，正文标题只显示语义主题；`DESIGN-NN` 仅保存在元数据和追踪关系中。条款级验收 ID 保持稳定，不随文件名调整。活动文档状态只使用 `draft/review/accepted`；验证结果只使用 `passed/failed/insufficient/not_applicable`。
