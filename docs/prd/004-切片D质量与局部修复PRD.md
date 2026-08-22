# 切片 D：质量与局部修复 PRD

> PRD ID：PRD-D
> 状态：proposed
> 日期：2026-08-22
> 前置：[PRD-C](./003-切片C真实可恢复生成PRD.md) 已产生真实 Candidate/Selection/Usage 样本
> 需求范围：M04、M05、M07—M11、M14
> 设计依据：[M10 详细设计](../design/modules/010-M10-质量连续性与修复详细设计.md)、[接口工作流](../design/003-AI视频生产平台接口工作流与功能实现设计.md)
> 实施与验收：[PLAN-D](../plan/004-切片D质量与局部修复实施计划.md)、[Acceptance](../acceptance/README.md)

## 1. 产品决策

切片 D 把“看起来不对”转换为绑定版本和证据的 Issue，并只为真实受影响范围创建 RepairPlan。首期同时提供确定性技术检查和一个结构化连续性检查器；不建立不可解释的综合质量分，也不允许自动评估选择 Candidate、接受风险或批准交付。

## 2. 目标用户与结果

主创需要确认问题是否真实、是否可接受以及应重做哪些镜头；操作员需要从证据定位媒体/时间码/字段并执行定向复检；制片人需要在批准修复前看见影响和资源；治理负责人只处理 M14 策略例外，不能被 QualityRiskAcceptance 替代。

| ID | 用户结果 |
| --- | --- |
| PRD-D-O-001 | 技术、语义和连续性问题绑定明确 Candidate/版本/时间码或字段证据。 |
| PRD-D-O-002 | 用户能确认、驳回、标不适用、分派、接受质量风险或创建修复。 |
| PRD-D-O-003 | 上游状态变化先形成可复核 ImpactReport，修复只覆盖真实受影响对象。 |
| PRD-D-O-004 | 新 Candidate 到达后必须定向复检，修复成功不能直接关闭 Issue。 |

## 3. P0 范围

| ID | P0 功能要求 | 关键 Requirement |
| --- | --- | --- |
| PRD-D-FR-001 | 对 M09 ready 媒体执行格式、解码、尺寸、时长、黑帧/静音等确定性检查。 | QAR-FR-001 |
| PRD-D-FR-002 | 首个结构化检查器固定为人物身份/服装/关键道具一致性之一，使用 M04 approved 状态和范围。 | QAR-FR-002/003；KNO-FR-006/010/012 |
| PRD-D-FR-003 | Evaluation/Issue 保存输入 hash、规则/评估器版本、证据位置、覆盖和 stale 状态。 | QAR-FR-004/005/010 |
| PRD-D-FR-004 | 用户对 Issue 做确认、驳回、不适用、分派、风险接受或修复决定，保留追加历史。 | QAR-FR-006/011 |
| PRD-D-FR-005 | 变更前根据权威依赖生成 ImpactReport，区分 hard/soft、unknown 和计算 basis。 | KNO-FR-013；SYS-BR-006 |
| PRD-D-FR-006 | RepairPlan 冻结目标、动作、输入、影响、资源、治理和验证范围；过期后禁止执行。 | QAR-FR-007 |
| PRD-D-FR-007 | 需要外部媒体时只为选中范围创建 M07 新计划；新结果只触发相关规则/相邻依赖复检。 | QAR-FR-008/009；SHT-FR-012 |
| PRD-D-FR-008 | QualityRiskAcceptance 与 M14 PolicyException、M12 ReviewDecision 分离，分别校验权限、理由、范围和期限。 | QAR-FR-006；GOV-FR-008 |

## 4. 用户旅程与页面

```text
查看检查覆盖与未评估范围
  → 打开 Issue 的媒体/时间码/字段证据
  → 确认、驳回、不适用、分派或接受风险
  → 若需修复，查看 ImpactReport、资源和治理结论
  → 批准 RepairPlan
  → 执行局部确定性修复或新 GenerationPlan
  → 比较新旧 Candidate 并选择
  → 定向复检
  → 关闭、重新打开或继续处理 Issue
```

质量页面必须区分 `passed`、`failed`、`not_evaluated`、`stale`、`error` 和 `risk_accepted`。证据播放器/字段 diff、规则版本、受影响镜头、责任人和下一动作均可下钻；颜色不是唯一状态提示。

## 5. 业务规则与失败体验

- 评估器失败或覆盖不完整时显示 `not_evaluated/error`，不得显示通过。
- Input、规则或评估器变化使相关 Evaluation stale，但旧证据和人工决定保留。
- Impact 计算超时/不完整时阻止自动 RepairPlan；用户仍可创建人工 Issue。
- 修复 Job failed/unknown 进入 C 的恢复/对账语义，不新增旁路状态。
- 修复 Candidate 不自动成为 Selection；只有新 Selection 后才按该版本复检。
- 风险接受到期或范围变化后重新打开 blocker，不删除历史接受记录。

## 6. 指标

| 指标 | 口径 |
| --- | --- |
| 检查覆盖 | eligible 对象中 passed/failed/not_evaluated/error/stale 的数量。 |
| 检查器质量 | 在标注集上分别记录 precision、recall、误报、漏报，不合并成总分。 |
| Impact 准确性 | 预测对象与实际重算对象的 precision/recall 及漏算严重度。 |
| 局部返工 | 修复对象数/项目候选总数、资源消耗和未受影响对象重跑数。 |
| Issue 周期 | 创建到确认、分派、修复、复检、关闭的时长与重新打开率。 |

## 7. 验收标准

| ID | Given / When / Then | 关联验收 |
| --- | --- | --- |
| PRD-D-AC-001 | 给定损坏/黑帧/静音等 Golden 媒体，当技术检查运行时，Issue 定位精确错误并阻止不适合的用途；失败分片不显示通过。 | AC-QAR-001 |
| PRD-D-AC-002 | 给定人物在第三场换装或关键道具转移，当检查/影响分析运行时，只标记引用对应状态范围的镜头，未受影响兄弟镜头不进入 RepairPlan。 | AC-SYS-004、AC-KNO-003/004/009、AC-QAR-002/003/006 |
| PRD-D-AC-003 | 给定误报，当用户驳回或标不适用时，Evaluation、证据和理由历史保留，后续规则升级不会删除。 | AC-QAR-005/008 |
| PRD-D-AC-004 | 给定 ImpactReport 超时、不完整或输入已变化，当批准 RepairPlan 时，执行被阻止并要求重算/人工接管。 | SYS-BR-006、QAR-FR-007 |
| PRD-D-AC-005 | 给定需要外部重生成的一个 Shot，当批准修复时，只创建该范围的新 Plan/Job；其他镜头 Provider 调用次数为 0。 | AC-QAR-006、AC-SYS-010 |
| PRD-D-AC-006 | 给定新修复 Candidate 到达，当用户尚未 Selection/复检时，原 Issue 不自动关闭；复检失败会保持或重新打开。 | AC-QAR-007 |
| PRD-D-AC-007 | 给定缺少权限、理由、范围或期限的高严重度风险接受，当提交时，决定被拒绝且不能替代治理例外或审阅批准。 | AC-QAR-009、AC-GOV-005 |
| PRD-D-AC-008 | 给定规则升级和项目中途变化实验，当重建结果时，旧评估可重读，stale 范围与实际重算可比较并保存指标。 | AC-QAR-008、AC-SYS-010 |

## 8. 非目标与退出 Gate

首期不交付通用视觉大模型总分、自动主选、自动风险接受、全项目自动修复或完整质量规则库。所有 `PRD-D-AC-*`、对应 AC-QAR 和 AC-SYS-004/010 通过后退出。若首个检查器无法给出定位证据，或 ImpactReport 不能稳定排除未受影响对象，则停止自动修复，只保留人工 Issue/Repair 操作。
