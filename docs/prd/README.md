# Lanverse PRD 索引

- 状态：active（S0～S3 与 MVP-A accepted；S4～S6 proposed）
- 日期：2026-08-13
- 上游：[需求索引](../requirement/README.md)、[Design 索引](../design/README.md)
- 下游：[Plan 索引](../plan/README.md)

## 文档集合

| 文档 | 作用 | 状态 |
| --- | --- | --- |
| [PRD-001 MVP 产品定义与范围](./001-MVP产品定义与范围.md) | 用户结果、范围、工作基线、成功指标和外部决策门禁 | S0～S3 accepted；MVP-A 局部 accepted；发布型全量 proposed |
| [PRD-002 MVP 纵向交付切片](./002-MVP纵向交付切片.md) | S0–S6 的用户故事、能力范围和可执行退出条件 | S0～S3 accepted；S4～S6 proposed |
| [PRD-003 MVP 执行计划与追踪矩阵](./003-MVP执行计划与追踪矩阵.md) | 决策、工作包、依赖、需求覆盖和完成定义 | S0/S1 局部 accepted |
| [PRD-004 基础业务模块产品规格](./004-基础业务模块产品规格.md) | identity、projects、media、governance 的事实、能力、交接和验收 | identity/projects S1 accepted |
| [PRD-005 创作生产模块产品规格](./005-创作生产模块产品规格.md) | scripts、assets、storyboards、production 的事实、用例、状态和验收 | S2/S3 accepted；S4 production proposed |
| [PRD-006 剪辑交付与平台保障产品规格](./006-剪辑交付与平台保障产品规格.md) | editing 以及消息、缓存、存储、调度、可观测性的能力和验收 | proposed |
| [PRD-007 基础业务模块 PRD 任务](./007-基础业务模块PRD任务.md) | identity、projects、media、governance 的产品任务、依赖和逐项验收 | PT-MED-001～003/005、PT-GOV-001/002 accepted；持续任务仍推进 |
| [PRD-008 创作生产模块 PRD 任务](./008-创作生产模块PRD任务.md) | scripts、assets、storyboards、production 的产品任务、依赖和逐项验收 | S2/S3 对应 PT accepted；S4 production PT proposed |
| [PRD-009 剪辑交付与平台保障 PRD 任务](./009-剪辑交付与平台保障PRD任务.md) | editing 与六类平台保障任务、依赖和逐项验收 | PT-CCH-001、PT-MED-005、PT-SCH-001/002/003 accepted；其余 PT proposed |
| [PRD-010 需求、设计与产品任务追踪矩阵](./010-需求设计与产品任务追踪矩阵.md) | 461 个 Requirement 叶子编号到 Design、PT/里程碑和验收归属的查漏基线 | S1/AIP 映射 accepted；MVP-A 追踪 active |
| [PRD-011 AI 提供方配置与启用](./011-AI提供方配置与启用PRD.md) | 7 个 Provider 管理产品任务、量化安全/性能/真实 Provider 验收和部分接受边界 | 执行基线 accepted；DEV-AIP-01 completed；各 PT 仍待完整真实证据 |
| [PRD-012 AI 短剧 MVP 核心制作产品任务](./012-AI短剧MVP核心制作产品任务.md) | 11 个整剧/分集、改写、叙事单元、资产状态、分镜覆盖与导出 PT | active；MVP-A 11/11 PT accepted |

## 阅读顺序

1. PRD-001 决定“为谁、交付什么、做到什么程度以及哪些外部选择会阻塞”。
2. PRD-004 至 PRD-006 决定“每个模块拥有什么事实、提供哪些用例以及怎样交接”。
3. PRD-007 至 PRD-009、PRD-011 与 PRD-012 把模块设计转成“可以映射到 DEV、阻塞和逐项接受的 PT 产品任务”；工程排期和领取使用对应 Plan 的唯一 `DEV-*`。
4. PRD-010 查漏“每个 ENT/FR/IF/NFR 叶子是否都有 Design 和 PT/验收归属”，并显式隔离 P1、条件性任务和 Provider catalog-only 状态。
5. PRD-002 决定“按什么用户价值顺序组合 PT，以及每个切片怎样才算完成”。
6. PRD-003 决定“先关闭什么决策、哪些 PT 进入工作包、如何验证和执行”。

## 事实与变更规则

- 当前创作生产事实固定为：S2/S3 对应 PT 已由真实 DeepSeek confirmed 起点、三类 ready 资产与 Ready 分镜 1/1 的联合契约接受；S4 production PT 仍等待 D-004 火山方舟真实账号证据。PT-CCH-001、PT-MED-005、PT-SCH-001/002/003 已分别由真实 Redis、存储与调度恢复栈接受；这些提前完成不代表 PT-STO-002、DEV-S4-02 的 Provider/Attempt 部分、DEV-S6-01 或 S4/S6 accepted。工程完成、产品接受和外部门禁必须分别表述。
- Requirement 是业务需要和约束来源；Design 是事实所有权、状态、事务、接口和技术边界来源；PRD 把二者转成产品用例、模块交接和可执行验收，不另造平行事实。
- 模块规格负责职责完整性，PT 负责可执行性，纵向切片负责交付顺序；三者必须同时满足，不能用粗粒度工作包替代产品任务，也不按数据库实体制造 CRUD 任务。
- 本组文档处于 `active` 的分阶段接受状态：S0～S3 与 MVP-A 已接受，S4～S6 仍为 proposed/待实施。精确证据记录在 Acceptance 001～038；MVP-A 接受不等于一次冻结远期范围，也不得为 S4～S6 预建空代码目录。
- Acceptance 只记录实现后的真实命令、样例、压测、故障注入和演练证据；不能在实现前创建“已通过”结论。
- 范围、指标、供应商或合规决定变化时，先更新 PRD-001，再同步切片与追踪矩阵；不在代码或任务卡中形成隐性需求。
