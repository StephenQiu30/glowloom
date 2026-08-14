# Lanverse Plan 索引

- 状态：active（PLAN-012 MVP-A 已接受；MVP-B/C 未激活；其余计划按各自 Gate 推进）
- 日期：2026-08-13
- 最近审查：2026-08-14
- 上游：[PRD 索引](../prd/README.md)

## 当前执行结论

PLAN-001～011 已达到既有“可执行说明”状态；PLAN-012 的 DEV-MVPA-01～12 与 11 个 PT 已由 Acceptance 028～038 接受，产品负责人兼短剧制作人/QA 已完成黄金样本签字，MVP-A 状态为 `accepted`。S0～S3 已 accepted；S4 真实图片/视频继续等待 D-004，不用替身生产事实越过门禁。

文档状态与任务状态分离：Plan 头部 `ready` 表示该计划可以被执行；`DEV-*` 才表示工程领取状态；`PT-*` 才表示产品接受状态。未来决策保持 open 不影响阅读和估算，只阻塞明确关联的 DEV/PT。

## 文档集合

| 文档 | 对应输入 | 作用 | 当前可执行状态 |
| --- | --- | --- | --- |
| [PLAN-000 MVP 全栈实施总计划](./000-MVP全栈实施总计划.md) | PRD-001～PRD-012 | 统一技术基线、S0～S6/AIP/MVP-A 排序、跨模块 DEV 任务与公共门禁 | ready；S0～S3 与 MVP-A accepted；S4 等待 D-004 |
| [PLAN-001 MVP 产品定义与范围执行计划](./001-MVP产品定义与范围执行计划.md) | PRD-001 | 固化 MVP 范围、决策门禁、成功指标与发布判定 | ready；按切片执行 |
| [PLAN-002 MVP 纵向切片执行计划](./002-MVP纵向切片执行计划.md) | PRD-002 | 逐切片明确准入、交付物、验证与退出证据 | ready；按 S0→S6 激活 |
| [PLAN-003 MVP 工作包与交付门禁执行计划](./003-MVP工作包与交付门禁执行计划.md) | PRD-003 | 编排 WP-00～WP-12、DoR/DoD 和在制品限制 | ready；S2/S3 对应工作包已接受；MVP-A 由 PLAN-012 增量接管 |
| [PLAN-004 基础业务模块执行计划](./004-基础业务模块执行计划.md) | PRD-004 | 编排身份、项目、媒体与治理模块的联合交付 | ready；identity/projects S1 accepted |
| [PLAN-005 创作生产模块执行计划](./005-创作生产模块执行计划.md) | PRD-005 | 编排剧本、资产、分镜与生产链路 | ready；S3 规则/性能 profile 已固定，本地实现已准入 |
| [PLAN-006 剪辑交付与平台保障执行计划](./006-剪辑交付与平台保障执行计划.md) | PRD-006 | 编排剪辑、消息、缓存、存储、调度与可观测能力 | ready；PT-CCH-001、PT-MED-005、PT-SCH-001/002/003 accepted，其余随真实用例激活 |
| [PLAN-007 基础业务模块产品任务执行计划](./007-基础业务模块产品任务执行计划.md) | PRD-007 | 定义 20 个基础业务 PT 的接受顺序和证据 | ready；对应 DEV 激活后执行 |
| [PLAN-008 创作生产模块产品任务执行计划](./008-创作生产模块产品任务执行计划.md) | PRD-008 | 定义 25 个创作生产 PT 的接受顺序和证据 | ready；S2/S3 PT accepted；S4 PT 等待 D-004 |
| [PLAN-009 剪辑交付与平台任务执行计划](./009-剪辑交付与平台任务执行计划.md) | PRD-009 | 定义 24 个剪辑与平台 PT 的接受顺序和证据 | ready；PT-CCH-001、PT-MED-005、PT-SCH-001/002/003 accepted，其余对应 DEV 激活后执行 |
| [PLAN-010 需求追踪与变更治理执行计划](./010-需求追踪与变更治理执行计划.md) | PRD-010 | 校验 461 个需求叶子与 Requirement→Acceptance 追踪链 | ready；每次变更执行 |
| [PLAN-011 AI 提供方配置与启用执行计划](./011-AI提供方配置与启用执行计划.md) | PRD-011 | 编排 7 个 Provider 管理 DEV、Red/Green、真实 DeepSeek、OpenAPI/UI 与验收门禁 | ready；DEV-AIP-01 completed；下一任务 DEV-AIP-02 pending |
| [PLAN-012 AI 短剧 MVP 核心制作执行计划](./012-AI短剧MVP核心制作执行计划.md) | PRD-012 | 编排 12 个 DEV、71 基准人周、正式迁移、整剧/改写/资产状态/分镜覆盖和分镜包 | accepted；DEV-MVPA-01～12 completed |

## 阅读与执行顺序

1. 先以 PLAN-000 确认全局技术事实、依赖顺序和共同门禁。
2. 用 PLAN-001～PLAN-003 关闭产品范围、切片和工作包准入。
3. 用 PLAN-004～PLAN-006 确认模块边界及跨模块交接。
4. 当前切片实施时，只展开 PLAN-007～PLAN-009、独立 Provider 控制面 PLAN-011 或 MVP-A 增量 PLAN-012 中与该切片相关的 PT。
5. 每次范围或状态变化均执行 PLAN-010 的追踪审计，再创建 Acceptance 证据。

## 使用规则

- PLAN-000 是跨 PRD 事实来源；PLAN-001～PLAN-012 必须分别覆盖对应 PRD，不得互相复制完整需求。
- `DEV-*` 是唯一可领取、估算和进入工程状态的执行任务；`PT-*` 是唯一产品接受单元；`S*`、`WP-*`、`M*` 仅做排序/分组；`Pxxx-*` 仅是子计划中的检查项编号，不创建重复 Issue、不单独分配 owner 或维护状态。
- PLAN-001～011 的执行规格为 `ready`，PLAN-012 的 MVP-A 为 `accepted`；文档状态不改变 DEV/PT 门禁语义。S0～S3 与 DEV-MVPA-01～12 已 accepted。S4 真实生成和后续公开交付仍必须按顺序取得真实验收证据。
- D-001 不等于一次性接受全部模块细节；每个切片在进入 `ready` 前只接受与该切片相关的 Requirement、Design 和 PRD，并保留后续切片迭代空间。
- 文档头部状态表示整份文档状态；切片级接受必须记录精确范围与 commit，不能把“某切片已接受”误写成整份全局文档均已冻结。
- 一次只有一个主切片进入 `in_progress`；计划中的顺序是依赖顺序，不是未经估算的工期承诺。
- Plan 不直接修改 Requirement、Design 或 PRD 事实；发现变化时按 Requirement → Design → PRD → Plan → Acceptance 更新。
- 仅在当前工作包真实完成后创建 `docs/acceptance/` 证据，不预写“已通过”结论。
