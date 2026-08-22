# 切片 C：真实可恢复生成 PRD

> PRD ID：PRD-C
> 状态：proposed
> 日期：2026-08-22
> 前置：[PRD-A](./001-切片A手工事实主线PRD.md) 已通过；PRD-B 不是前置
> 需求范围：M07、M08、M09、M11、M14
> 设计依据：[生成规划](../design/modules/007-M07-生成规划提示编译与路由详细设计.md)、[耐久执行](../design/modules/008-M08-提供商与耐久执行详细设计.md)、[媒体与选择](../design/modules/009-M09-媒体候选与选择详细设计.md)
> 实施与验收：[PLAN-C](../plan/003-切片C真实可恢复生成实施计划.md)、[Acceptance](../acceptance/README.md)

## 1. 产品决策

切片 C 只验证一个真实图像 capability 和一个真实视频 capability 的“可审查计划 → 耐久执行 → 媒体接纳 → 人工选择”闭环。用户批准前必须看见输入、Prompt、参考、能力、资源上限和外发风险；批准后即使发生提交超时、重复消息、回调乱序或下载中断，也不能盲目创建第二个昂贵任务。

一个图像加一个视频能力不等于多模型路由。只有同一目标类型存在至少两个可替代能力，并另行通过选择理由、Prompt 差异、故障和批准范围内 fallback 验收后，才能宣称 PLN-FR-012 完成。

## 2. 目标用户与结果

| 角色 | 主要任务 |
| --- | --- |
| 主创/导演 | 确认目标和参考，比较 Candidate，做出 Selection |
| 制片人 | 审查资源区间/硬上限、外发和治理结论，批准 start_now/hold |
| 制作操作员 | 观察 Job/Attempt、处理 unknown、下载/媒体异常和对账 |
| 权利负责人 | 审查真人/IP/地域/训练与保留条款，决定策略例外 |

| ID | 用户结果 |
| --- | --- |
| PRD-C-O-001 | 每个真实外调都基于可读、冻结且可还原的 GenerationPlan。 |
| PRD-C-O-002 | 用户能区分 queued/running/unknown/partial/failed，并根据证据恢复而非盲重试。 |
| PRD-C-O-003 | 每个接纳 Candidate 可还原目标、输入、Prompt、Attempt、模型、权利和实际用量。 |
| PRD-C-O-004 | Candidate 的到达、评分或重试不会自动改变人工 Selection。 |

## 3. P0 旅程

```text
选择 approved ShotPlan/目标
  → 创建 GenerationPlan 草稿
  → 检查编译 Prompt、参考、能力和淘汰原因
  → 检查 estimate、硬上限、数据外发和治理结论
  → 批准 start_now 或 hold
  → 查看 Operation/Job/Attempt
  → 对 unknown 查询/回调/人工证据进行对账
  → 下载、hash/解码/安全检查并保存 MinIO exact version
  → 创建独立 Candidate
  → 并排比较并做出 purpose-specific Selection
  → 查看 estimate→actual 与失败/unknown 归因
```

## 4. 功能范围

| ID | P0 功能要求 | 关键 Requirement |
| --- | --- | --- |
| PRD-C-FR-001 | 从批准 ShotPlan/知识和明确 capability 创建 Plan/Item/InputSnapshot，保存可读 Prompt 与编译/规则版本。 | PLN-FR-001—007/009 |
| PRD-C-FR-002 | 批量预检逐项返回硬 blocker、可批准集合、首选/备选和淘汰原因；人工覆盖不能越过硬门禁。 | PLN-FR-004—006/010/011 |
| PRD-C-FR-003 | 批准时明确 `start_now`/`hold`，资源 Reservation、Plan 状态和 Outbox 原子提交。 | PLN-FR-008/009；USG-FR-001—003/007 |
| PRD-C-FR-004 | 每个逻辑 Job 追加 Attempt；支持 Provider 实际具备的同步/轮询/回调模式和 unknown 对账。 | EXE-FR-001—009/011/013 |
| PRD-C-FR-005 | backend 在唯一 Kafka provider Topic 接管任务，以 Inbox/手工 offset 保持至少一次投递幂等；限流等待持久化而不占 partition。 | EXE-FR-002/010；EXE-NFR-001—004 |
| PRD-C-FR-006 | 回调验签、防重放、去重、乱序/过期处理；连接撤销隔离对应 Provider。 | EXE-FR-005/013 |
| PRD-C-FR-007 | 下载复用 external task，校验类型、大小、hash、解码和安全状态；保存原件/代理/派生血缘。 | MED-FR-001—004 |
| PRD-C-FR-008 | 每个成功结果创建独立 Candidate；预览、shortlist、Selection 和替代决定分离。 | MED-FR-003—010 |
| PRD-C-FR-009 | 记录 estimate、Reservation、实际、迟到、失败、取消和 unknown 用量，不生成账单。 | USG-FR-002—010 |
| PRD-C-FR-010 | 外发前显示 Provider、地域、数据类别、保留/训练条款并执行权利、安全和资源门禁。 | GOV-FR-001—010 |

## 5. 页面与失败体验

| 页面 | 必须显示 |
| --- | --- |
| GenerationPlan | 冻结输入、Prompt、参考、capability、硬 blocker、estimate、外发和批准动作 |
| 任务/异常中心 | Operation、Job、Attempt、最近证据、unknown 原因、可用对账/取消/重试动作 |
| Candidate 比较 | 独立 Candidate、媒体完整性、谱系、质量摘要、Selection purpose 与并发冲突 |
| 项目用量 | estimate/reserved/actual/released、失败/unknown 归因和数据时效 |

提交响应前超时不得显示为 failed 或自动重发；Provider 新状态不能猜测映射；下载失败只重试同 external task；限流不创建业务失败事实；媒体损坏进入隔离而不是 ready；批量 partial 保留成功项并只允许对失败目标创建新 Attempt。

## 6. Provider 进入 Gate

每个真实 Provider 在实施前必须记录并签认：请求幂等或可对账依据、查询/取消能力、回调验签、外部 ID、状态集合、速率限制、资源计量、下载有效期、内容保留、训练使用、地域和故障支持。缺少查询与可靠幂等两者时，只能进入明确的人工单次执行实验，不能启用自动重试或宣称耐久执行通过。

## 7. 指标

| 指标 | 口径 |
| --- | --- |
| 外调重复副作用 | 相同业务意图产生的第二 external task/Candidate/UsageEntry，目标 0。 |
| unknown 率与对账时间 | 按 Provider/capability/阶段统计，不能并入普通失败率。 |
| 估计误差 | estimate 区间与实际用量差异、缺失/迟到覆盖率。 |
| 媒体接纳 | 下载、hash、解码、安全失败率及恢复次数。 |
| Candidate 采用 | 每目标 Candidate 数、首轮采用率、人工 Selection 变更原因。 |

## 8. 验收标准

| ID | Given / When / Then | 关联验收 |
| --- | --- | --- |
| PRD-C-AC-001 | 给定批准计划，当之后修改人物/Shot/参考时，原计划输入、Prompt、capability 和 estimate 保持冻结，新执行必须创建替代计划。 | AC-PLN-001/002/007 |
| PRD-C-AC-002 | 给定 `hold`、重复 `start_now` 和并发批准，当提交时，hold 外调为 0，start_now 只有一个逻辑 Job/Reservation。 | AC-PLN-003、AC-EXE-001、AC-USG-002/003 |
| PRD-C-AC-003 | 给定 Provider 提交在响应前超时，当是否收单不明时，Job 进入 unknown 并以原 key/external evidence 对账，不创建第二任务。 | AC-SYS-003、AC-EXE-002/007 |
| PRD-C-AC-004 | 给定 Kafka 重投/重平衡、Worker 在提交前后重启和 offset commit 失败，当恢复时，Job/Attempt/Provider submit 次数符合幂等语义。 | AC-EXE-001—004 |
| PRD-C-AC-005 | 给定重复/乱序/伪造回调，当处理时，伪造被拒绝，重复不推进第二次，也不新增 Candidate/UsageEntry。 | AC-EXE-003、AC-MED-001 |
| PRD-C-AC-006 | 给定 Provider 限流或 Redis 不可用，当准备外调时，任务进入可恢复等待或 fail closed，Provider 调用次数为 0，Kafka partition 不被 sleep 占用。 | EXE-FR-010、SYS-NFR-002 |
| PRD-C-AC-007 | 给定下载断线/hash 错/损坏媒体，当恢复时，复用同 external task；不重新生成，损坏对象隔离且不能 Selection。 | AC-EXE-005、AC-MED-005 |
| PRD-C-AC-008 | 给定三个成功 Candidate 和并发 Selection，当新 Candidate 到达或两个用户主选时，历史 Candidate 保留且只有一个 current Selection。 | AC-MED-001—004/008 |
| PRD-C-AC-009 | 给定硬资源上限、未声明真人权利或不兼容地域/训练条款，当批准时，外调次数为 0，错误指出规则和下一动作。 | AC-PLN-004/005、AC-USG-001/007、AC-GOV-001—006 |
| PRD-C-AC-010 | 给定失败、cancelled、unknown 和迟到计量，当查看用量时，各自实际消耗可追溯，迟到通过追加记录更新且不产生账单状态。 | AC-USG-004—008 |

## 9. 退出 Gate

选定图像/视频 capability 的真实沙箱证据、所有 `PRD-C-AC-*`、对应 AC-PLN/EXE/MED/USG/GOV、任务中心恢复路径和候选谱系必须通过。任何无法解释的重复收费/任务、跨租户媒体、unknown 被盲重试或人工 Selection 被自动覆盖均为阻断缺陷。
