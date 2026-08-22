# 切片 B：受限 Agent 提案 PRD

> PRD ID：PRD-B
> 状态：proposed
> 日期：2026-08-22
> 前置：[PRD-A](./001-切片A手工事实主线PRD.md) 已通过
> 需求范围：M03、M04、M06、M11、M14；M05 Agent 拆镜不进入本切片 P0
> 设计依据：[M06 详细设计](../design/modules/006-M06-Agent与可视化画布详细设计.md)、[剧本分析专题](../design/004-AI视频生产平台剧本基础分析与人物拆解详细设计.md)
> 实施与验收：[PLAN-B](../plan/002-切片B受限Agent提案实施计划.md)、[Acceptance](../acceptance/README.md)

## 1. 产品决策

切片 B 只引入一个 P0 Skill：`script_analysis`。它由三个彼此独立的 AgentRun 组成：`breakdown`、`narrative`、`knowledge`。每个 Run 只产生结构化 Proposal；Manifest 与 Narrative 的人工批准、ContentUnit 物化以及人物/生产需求决议仍由 A 已验证的 Go 命令完成。

本切片不把拆镜、修复、聊天助理或自由画布并入同一个 Agent 工作流。只有 `script_analysis` 在真实金标中证明安全、可恢复和有明确效率收益后，其他 Skill 才能按独立 PRD/Gate 进入。

## 2. 用户问题与目标结果

手工流程能够完成，但整本拆集、逐集结构录入和跨集人物/资产整理耗时。主创希望获得有证据的首稿，同时保留逐项理解、编辑、拒绝和暂缓的权利；操作员希望模型或 Tool 部分失败时只接管失败范围。

| ID | 用户结果 |
| --- | --- |
| PRD-B-O-001 | 用户获得可回到原文的拆集、叙事和生产知识提案，而不是一份不可拆解的 AI 文本。 |
| PRD-B-O-002 | 每项 Proposal 显示基线、字段差异、不确定项、影响、权限、资源和过期条件。 |
| PRD-B-O-003 | 接受/编辑 Proposal 与手工提交同一命令产生相同领域结果。 |
| PRD-B-O-004 | Agent 不可用、超时、部分失败或关闭后，A 的手工工作台和已接受事实仍完整可用。 |
| PRD-B-O-005 | 用户可以从最小关系/影响视图定位提案对象并进入结构化编辑器，不产生第二套关系事实。 |

## 3. P0 用户旅程

```text
选择整本 SourceRevision
  → 启动 root AnalysisRun
  → breakdown AgentRun 提交 EpisodeBreakdown Proposal
  → 用户逐项修订并批准 Manifest
  → M02 原子物化 ContentUnit
  → narrative AgentRun 按 ContentUnit 提交 DraftImport items
  → 用户逐集接受/修改/拒绝，Finalize 并批准 Narrative
  → knowledge AgentRun 提交人物消歧与生产需求 Proposal
  → 用户逐项决议
  → Go 重建 Inventory、Readiness 和人物×剧集矩阵
  → 用户查看 root 完整度和剩余手工项
```

## 4. 功能范围

| ID | P0 功能要求 | 关键 Requirement |
| --- | --- | --- |
| PRD-B-FR-001 | 创建可恢复的 root AnalysisRun，并把三个 stage 映射为独立 AgentRun/child Operation；stage 间由业务批准门禁推进。 | AIC-FR-001/006/015；NAR-FR-013/014 |
| PRD-B-FR-002 | breakdown 输出自包含 EpisodeBreakdown 草稿项，引用来源范围/Anchor，不引用尚未物化的未来 ContentUnit ID。 | AIC-FR-002—005/014 |
| PRD-B-FR-003 | narrative 按稳定 ContentUnit 输出隔离的 DraftImport item；一项失败/拒绝不污染其他 ContentUnit slot。 | AIC-FR-002—006 |
| PRD-B-FR-004 | knowledge 输出自包含的 MentionResolution、UnresolvedSubject、Entity/State、Coverage/Requirement 命令提案。 | KNO-FR-001—007/016/017；AIC-FR-007 |
| PRD-B-FR-005 | 每项 Proposal 支持接受、编辑、拒绝、暂缓；执行前重新鉴权、重算 diff/read-write set 并按细粒度 expected revision 做 CAS。 | AIC-FR-003/004/014 |
| PRD-B-FR-006 | Agent 只能调用 run-scoped、只读、最小数据 Tool；剧本检索经 backend 强制 workspace/project/approved revision filter。 | AIC-FR-005/015；GOV-FR-013 |
| PRD-B-FR-007 | 模型调用前显示并冻结外发、模型、Tool allowlist 与资源上限；用量与媒体 GenerationJob 分开记录。 | USG-FR-012；GOV-FR-003/004/013 |
| PRD-B-FR-008 | 最小关系/影响视图只投影 A 的权威对象，节点跳转到同一结构化编辑器；布局删除不删除事实。 | AIC-FR-009/010 |
| PRD-B-FR-009 | AgentRun 支持 accepted/running/partial/failed/cancelled/expired，重复 start/result 和重启收敛到同一 Run/Proposal。 | AIC-FR-006；AIC-NFR-001—003 |

## 5. Agent 提案卡片契约

每张卡片必须显示：提案类型、目标对象或待创建身份、基线版本、来源证据、字段 diff、unknown/conflict、读写范围、下游影响、所需权限、模型/Tool/资源与外发结论、过期条件和安全下一动作。

解释、建议、Proposal 和待授权外部动作使用不同视觉/状态；“Run 完成”不能显示为“业务已批准”。不显示或无法验证来源证据的项只能暂缓/拒绝或进入手工录入。

## 6. 失败、降级与非目标

- 模型不可用、Tool 部分失败、checkpoint 损坏或 stage partial 时，保留成功 item 和失败 scope，用户可局部重试或手工接管。
- 基线变化只使真实 read-set 相交的 item expired；不相交 ContentUnit/knowledge item 不得整批过期。
- 非法 Schema、越权 Tool、重复 result、过期 capability 和非 operation-worker 调用的业务副作用必须为 0。
- Python Agent 不连接 Kafka、Redis、Elasticsearch、业务数据库或通用 MinIO，不暴露公共 API/Ingress。
- Agent 拆镜、媒体 GenerationPlan、质量修复、完整画布、长期聊天记忆和自动批准不属于 B 的 P0。

## 7. 指标

| 指标 | 口径 |
| --- | --- |
| Proposal 决议分布 | breakdown/narrative/knowledge 分别统计 accepted/edited/rejected/deferred/expired。 |
| 证据错误率 | Anchor 不存在、证据不支持字段或跨范围引用的 item 比例。 |
| 手工节省 | 与 A 金标重放相比，首个可审阅稿时间及手工字段操作数变化。 |
| 恢复正确性 | 重启/重复 start/result 后重复 Run、Proposal、Item 或业务事实数，目标 0。 |
| 安全拒绝 | 非法 Schema、越权 Tool、外发/用量门禁拒绝次数及业务副作用，副作用目标 0。 |

## 8. 验收标准

| ID | Given / When / Then | 关联验收 |
| --- | --- | --- |
| PRD-B-AC-001 | 给定 A 的同一金标整本剧本，当运行三个 stage 时，顺序严格经过 Manifest 批准/物化和 Narrative 批准；任一 child 完成不能完成 root。 | AC-AIC-002/005/009、AC-SYS-001 |
| PRD-B-AC-002 | 给定至少 10 个跨 stage ProposalItem，当用户逐项接受、编辑、拒绝和暂缓时，每项独立形成决定，接受/编辑与对应手工命令结果等价。 | AC-AIC-001/002 |
| PRD-B-AC-003 | 给定非法字段、重复 result、越权 Tool、过期 item 或不同 request hash，当提交时，业务事实不变并产生安全错误/审计。 | AC-AIC-003 |
| PRD-B-AC-004 | 给定同批两个不相交 ContentUnit/item，当其中一个被接受并改变基线时，另一个不因批次级锁误过期；真实 read-set 冲突项必须过期。 | AIC-FR-003/014 |
| PRD-B-AC-005 | 给定 start 响应丢失、get 超时和 Agent 进程重启，当 backend 以同 run/key/hash 对账时，只存在一个 Run 和一组 Proposal。 | AC-AIC-009、SYS-NFR-002 |
| PRD-B-AC-006 | 给定 Agent/模型/Redis/Tool 不可用，当用户切换手工接管时，A 的全部命令和已接受事实可继续使用，失败 scope 可见。 | AC-AIC-005、AC-SYS-008 |
| PRD-B-AC-007 | 给定浏览器、第三方或伪造服务身份尝试调用 Agent/Tool，当执行时，网络和身份校验拒绝；Agent 部署无 Kafka/Redis/Elastic/业务库凭据。 | IAM-NFR-004、GOV-FR-013 |
| PRD-B-AC-008 | 给定最小关系视图中的对象，当移动/隐藏/删除布局节点或跳转编辑时，领域事实不变，结构化编辑仍使用同一 Query/Command。 | AC-AIC-006/007/008 |
| PRD-B-AC-009 | 给定模型调用被 M11 上限或 M14 外发策略阻止，当启动 Run 时，模型调用次数为 0，用户看到稳定 blocker 与手工下一动作。 | AC-AIC-004、AC-GOV-006、AC-USG-007/008 |

## 9. 退出 Gate

所有 `PRD-B-AC-*` 和 AC-AIC-001—009 的 B 范围通过；金标报告分别给出三段质量而非合并成聊天满意度；Agent 关闭演练通过；部署和依赖扫描证明 Agent 无公共入口及禁止客户端。若接受提案与手工命令不一致、Agent 需要业务写权限或 stage 门禁必须保存在 LangGraph 中，本切片停止并返回 Design 修正。
