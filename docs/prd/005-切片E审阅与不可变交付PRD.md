# 切片 E：审阅与不可变交付 PRD

> PRD ID：PRD-E
> 状态：proposed
> 日期：2026-08-22
> 前置：PRD-C 的 Selection/媒体事实稳定；PRD-D 的质量与风险决定可冻结
> 需求范围：M09、M10、M12、M13、M14
> 设计依据：[M12 审阅设计](../design/modules/012-M12-审阅协作与批准详细设计.md)、[M13 交付设计](../design/modules/013-M13-装配与交付详细设计.md)
> 实施与验收：[PLAN-E](../plan/005-切片E审阅与不可变交付实施计划.md)、[Acceptance](../acceptance/README.md)

## 1. 产品决策

切片 E 只支持 Workspace 成员的内部审阅，并把明确 Selection、质量/治理决定和装配版本冻结为 ReviewPackage 与 DeliveryBuild。评论、批准和交付必须绑定精确版本；后续重选、重排、修复或代理重建不能移动旧评论或改变旧 Snapshot。

外部非成员审片、临时公开链接和外部通知在 PRD-F 单独 Gate；E 不用内部 reviewer 账号或长期 presigned URL 冒充外部审片能力。

## 2. 目标用户与结果

| 角色 | 主要任务 |
| --- | --- |
| 发起审阅的主创/制片人 | 冻结 ReviewPackage、指定审片人、处理修改请求 |
| 内部审片人 | 对精确媒体/时间码/区域评论并批准、请求修改或拒绝 |
| 装配操作员 | 维护基础画面、音频、字幕和输出预设 |
| 交付批准人 | 处理 preflight blocker，批准 Build 并发布 Snapshot |
| 权利负责人 | 复核交付用途、地域、期限和下载权限 |

| ID | 用户结果 |
| --- | --- |
| PRD-E-O-001 | 审片人看到发起人冻结的同一内容，评论和决定不会随项目 current 漂移。 |
| PRD-E-O-002 | 修改请求进入可追踪 Issue/Repair，新内容必须创建新 ReviewPackage。 |
| PRD-E-O-003 | 用户可完成平台闭环所需的基础画面、音频和字幕装配。 |
| PRD-E-O-004 | 所有必需输出成功并校验后才原子发布 DeliverySnapshot/Manifest。 |
| PRD-E-O-005 | Snapshot 可复现、可授权下载且后续项目修改不改变历史。 |

## 3. 功能范围

| ID | P0 功能要求 | 关键 Requirement |
| --- | --- | --- |
| PRD-E-FR-001 | 从明确 ShotOrder、Selection、Assembly、质量/治理 basis 创建不可变 ReviewPackage Manifest。 | REV-FR-001/007 |
| PRD-E-FR-002 | 为内部审片人授予有期限、可撤销的最小访问并记录访问。 | REV-FR-002/010 |
| PRD-E-FR-003 | 评论/回复/提及/分派绑定对象版本、精确时间码/区域和状态。 | REV-FR-003/004/011 |
| PRD-E-FR-004 | ReviewDecision 区分批准、请求修改、拒绝和撤回，支持明确角色/人数策略。 | REV-FR-005/006 |
| PRD-E-FR-005 | 修改请求通过结构化入口创建 M10 Issue/Repair 关联，不直接修改媒体。 | REV-FR-008 |
| PRD-E-FR-006 | 创建 AssemblyRevision，管理 Shot 顺序/入出点/空隙/简单转场、基础音轨/音量和字幕。 | DLV-FR-001—005 |
| PRD-E-FR-007 | Delivery preflight 分别检查主选、媒体、质量、权利、审阅和输出规格，不合并为模糊总状态。 | DLV-FR-006；GOV-FR-006 |
| PRD-E-FR-008 | 冻结 DeliveryBuild 输入，记录独立 RenderRun，并支持阶段恢复和安全重跑。 | DLV-FR-007—009 |
| PRD-E-FR-009 | 全部必需输出校验后原子创建 Snapshot/Manifest；partial/failed 不发布。 | DLV-FR-010/011 |
| PRD-E-FR-010 | 按权限下载、撤销新访问并保留历史交付；支持审阅 MP4、有序镜头包和 Manifest。 | DLV-FR-012/013；GOV-FR-006/009 |

## 4. 用户旅程

```text
冻结 ReviewPackage
  → 内部审片人评论/决定
  → 修改请求进入 Issue/Repair
  → 新 Candidate/Assembly 形成新 ReviewPackage
  → 达到批准策略
  → 完成交付 preflight
  → 冻结 DeliveryBuild 输入
  → Media Worker 分阶段渲染与校验
  → 所有必需输出成功
  → 原子发布 DeliverySnapshot/Manifest
  → 授权下载与审计
```

## 5. 时间基、失败与边界

- Frame rate、音频采样、字幕和评论位置使用精确整数时间基；23.976/29.97 等分数帧率重开后不得漂移。
- 代理/缩略图可以重建，但 ReviewItem 必须保存到原媒体/版本的稳定映射。
- 通知失败不回滚评论/决定；成员停用或授权到期后拒绝新操作。
- 缺主选、媒体未 ready、质量 blocker、治理/权利失效、审阅未通过或输出规格缺失都会阻止正式 Build。
- RenderRun 可以重试，但不能覆盖历史运行；主文件成功而必需附属文件失败仍为 partial，不创建 Snapshot。
- 高级 NLE、正式调色/混音、外部匿名审片和渠道自动发布不属于 E。

## 6. 指标

| 指标 | 口径 |
| --- | --- |
| 审阅周转 | ReviewPackage 创建到终态决定时长、修改轮次、未解决 blocker。 |
| 定位稳定性 | 代理重建/重开/新候选后评论错位数，目标 0。 |
| Build 恢复 | 阶段重启、重跑、partial 与平均恢复次数。 |
| 交付完整性 | Snapshot 必需文件/Manifest/hash 一致率，硬目标 100%。 |
| 下载治理 | 未授权或撤销后新下载成功数，目标 0。 |

## 7. 验收标准

| ID | Given / When / Then | 关联验收 |
| --- | --- | --- |
| PRD-E-AC-001 | 给定分数帧率视频和时间码/区域评论，当重开页面、重建代理或出现新 Candidate 时，评论仍指向原版本和精确位置。 | AC-REV-001/008 |
| PRD-E-AC-002 | 给定旧 ReviewPackage 已批准，当 Selection/Assembly 改变并创建新包时，旧决定不迁移，新包必须重新审阅。 | AC-REV-002 |
| PRD-E-AC-003 | 给定请求修改，当审片人提交时，创建可追踪 Issue/Repair 入口且不直接改媒体；新结果进入新包。 | AC-REV-004、AC-SYS-005 |
| PRD-E-AC-004 | 给定批准策略缺少必要角色、成员在决定前停用或授权过期，当提交时，决定被拒绝，旧评论/决定仍可重读。 | AC-REV-005—007 |
| PRD-E-AC-005 | 给定至少 10 个主选镜头，当用户完成基础装配时，顺序、入出点、音频和字幕在重开后不漂移。 | AC-DLV-001/002 |
| PRD-E-AC-006 | 给定缺主选、质量/权利/审阅 blocker 或规格缺失，当运行 preflight 时，正式 Build 不启动并逐项返回恢复动作。 | AC-DLV-003、AC-GOV-004 |
| PRD-E-AC-007 | 给定渲染阶段崩溃、对象存储中断或一个必需附属输出失败，当恢复/finalize 时，partial 不创建 Snapshot，成功项和运行证据保留。 | AC-DLV-004、SYS-NFR-002/007 |
| PRD-E-AC-008 | 给定相同 Build 被重复 finalize/重跑，当完成时，回读同一 Snapshot 或形成独立 RenderRun，不重复发布交付。 | AC-DLV-007 |
| PRD-E-AC-009 | 给定成功 Snapshot，当核对 Manifest 时，全部输入、Selection、质量/治理/审阅决定、输出和 hash 可还原且一致。 | AC-DLV-005/008、AC-SYS-006/009 |
| PRD-E-AC-010 | 给定 Snapshot 发布后的项目修改或下载授权撤销，当访问历史时，Snapshot/Manifest 不变，撤销只阻止新的未授权下载。 | AC-DLV-006、GOV-FR-009 |

## 8. 退出 Gate

交付预设、帧率/音频/字幕规则、内部审阅批准矩阵、下载策略和权利范围必须先签认。所有 `PRD-E-AC-*`、对应 AC-REV/DLV/GOV 以及 10+ Shot 真实 E2E 通过后退出。若时间基不能统一、preflight 无法重读冻结结论或 Snapshot 可能引用缺失文件，则停止正式交付，只保留内部预演。
