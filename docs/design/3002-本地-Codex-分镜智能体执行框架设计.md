# 本地 Codex 分镜智能体执行框架设计

- 状态：已接受设计
- StoryGraph Harness 复核：已接受（`SG-D12`，2026-08-27）
- 产品输入：[本地 Codex 分镜智能体产品需求](../prd/3002-本地-Codex-分镜智能体产品需求.md)
- 架构依据：[StoryGraph 内容图与 DAG 创作画布设计](0010-StoryGraph内容图与DAG创作画布设计.md) · [后端领域模块功能设计](2002-后端领域模块功能设计.md) · [StoryGraph 剧本解析 Harness 与内置 Skill 设计](3003-StoryGraph剧本解析Harness与内置Skill设计.md)
- 历史派生：[需求规格](../requirement/3002-本地-Codex-分镜智能体执行框架需求规格.md) · [实施计划](../plan/3002-本地-Codex-分镜智能体执行框架实施计划.md) · [验收标准](../acceptance/3002-本地-Codex-分镜智能体执行框架验收标准.md)；继续冻结，由 `SG-D17`–`SG-D21` 的统一文档链取代

## 结论

分镜智能体不是五个独立 Skill 拼成的进程内状态机，而是 `StoryGraphHarness` 调用 `agent/skills/build-storygraph` 唯一 Bundle 中的 Backend-owned Stage。分镜必须分为两个阶段：

```text
confirmed Scene/Beat/Occurrence + Specification/AssetState/Style
  → draft_storyboard Candidate
  → deterministic gate + Human Gate
  → Storyboard Owner FreezeIntentSet (approved_storyboard_intents)
  → reference_asset Generation / READY AssetVersion
  → detail_shots Candidate
  → review_storygraph + bounded repair
  → Human Gate + Storyboard Owner Apply
  → formal Shot + ShotProductionBindingVersion
  → next StoryGraphVersion
```

Draft 阶段允许且必须明确表达 `needs_asset`；Detail 阶段必须消费精确 READY AssetVersion。Agent 不生成图片、不写正式 Shot、不选择未提供的资产，也不保存跨请求 Checkpoint。

## 1. 当前事实与目标缺口

| 能力 | 当前代码事实 | StoryGraph 目标 |
|---|---|---|
| Skill 路径 | `CodexSchemaRunner` 从 `.agents/skills/<name>` 加载指导 | 原子迁移为 `agent/skills/build-storygraph` 唯一 Bundle |
| Skill 数量 | 仓库存在 8 个过渡 Skill，旧设计列出 5 个分镜 Skill | 运行时只按 Stage + Reference 选择同一 Bundle 内容 |
| Storyboard 调用 | `CodexLocalStoryboardDrafter` 当前只调用 `draft-shots` | 使用 `draft_storyboard`、`detail_shots`、`review_storygraph`、`repair_candidate` |
| 输入资产 | Backend 当前把 `assets` 固定为空数组 | Draft 输出视觉需求/`needs_asset`；Detail 只接收精确 AssetVersion Ref |
| Candidate | 当前单一 Candidate 同时承载镜头草案和 AssetReference | 拆为 Draft/Detail Candidate Revision，禁止空资产伪装可生产 Shot |
| 恢复 | 当前 AgentInvocation 有 Lease/Fencing；Storyboard Batch 无设计所称 Checkpoint 字段 | 恢复统一归 Backend NodeRun/Manifest/Invocation/Candidate Revision |
| 正式应用 | 当前 Draft Batch 经决议后可创建正式 Shot | 新链路必须先 FreezeIntentSet、准备资产，再 Detail/Review/Owner Apply |

当前代码与测试只能证明旧纵向切片存在，不能抵扣新 Draft/Detail、Bundle、视觉资产和 StoryGraph 验收。

## 2. 唯一 Bundle 与目录边界

实施完成后的唯一运行入口是：

```text
agent/skills/build-storygraph/
├── SKILL.md
├── LICENSES.md
└── references/
    ├── storyboard-draft.md
    ├── shot-detail.md
    ├── continuity-review.md
    ├── visual-assets.md
    └── ...仅由 3003 Registry 声明的 Reference
```

- `SKILL.md` 只描述稳定项目规则、Stage 路由、禁止项和 Reference 索引；
- Stage Schema、Policy、Budget、Bundle Version/Hash 和允许的 Reference 由 Backend 契约拥有；
- Python Harness 只加载当前 Stage 声明的 Reference，不把整个 Bundle 全量注入每次调用；
- `agents/openai.yaml`、根目录 `.agents/skills` 和旧 Skill 名不保留运行 fallback；
- 迁移按 [3003 的 SG-I04/SG-I05](3003-StoryGraph剧本解析Harness与内置Skill设计.md#实施任务映射)分成“字节等价迁移”和“唯一 Bundle 收口”两个完整任务，每次提交运行时都只有一条路径；
- 来源项目只用于设计研究和许可证追溯，不 checkout、下载或执行其脚本。

## 3. Stage 契约

| Stage | Backend 冻结输入 | Agent Candidate 输出 | 正式应用 Owner |
|---|---|---|---|
| `draft_storyboard` | Scene/Beat/Occurrence、Specification/AssetState、EffectiveStyleSnapshot、时长/画幅策略 | Storyboard Row/Shot Intent、视觉需求、`needs_asset` | Storyboard `FreezeIntentSet` |
| `detail_shots` | 已批准 Intent、精确 READY AssetVersion、相邻边界与视觉策略 | 完整 Shot Detail/ShotProductionBinding Candidate | Storyboard `ApplySet` |
| `review_storygraph(scope=storyboard)` | Candidate Revision、确定性 Gate 结果和影响闭包 | Evidence-scoped Review Issue | Review |
| `repair_candidate` | Issue、当前 Candidate Revision/Hash、允许修改集和只读邻接 | CandidateRepairPatch | 无；Backend 创建下一 Revision |

一个 Stage 可按 Scene 或 Shot Batch fan-out，但每个 Invocation 必须有稳定 `stage_instance_key + shard_key + input_hash`，并挂到预先存在的 Workflow NodeRun。Agent 成功只创建不可变 Result；Backend 才能创建/聚合 StageCandidateRevision、切换 Candidate Head 并运行 Gate。

## 4. Draft 输入与输出

`draft_storyboard` 只消费已确认的 Backend Owner Ref：

- Episode/Scene、Dialogue、NarrativeBeat 与 source Evidence；
- Character/Location/Prop Occurrence；
- 对应 SpecificationVersion 与 AssetState；
- EffectiveStyleSnapshot、目标时长、画幅和 Shot Policy；
- 可选的、已发布且与上述输入匹配的 AssetVersion Ref。

禁止只传一个 Bible JSON、自由文本资产描述或全项目资产列表。每个 Scene Shard 只得到实际 occurrence 和相关世界规则，避免身份串扰和上下文膨胀。

Draft Candidate 中每个 Shot Intent 至少包含：

- 临时 `shot_key`、source Scene/Beat/Evidence Ref；
- purpose、建议时长、景别/角度/运动/构图意图；
- action、dialogue、sound、performance 和 continuity intent；
- Character/Location/Prop Occurrence Ref；
- 每个 occurrence 所需 AssetState、asset role 与 view role；
- `asset_readiness=ready|needs_asset`；`ready` 时必须绑定输入中给定的精确 AssetVersion Ref，`needs_asset` 时只保存严格视觉需求；
- first/key/last frame intent、risk code 和 review issue。

Draft Candidate 不是正式 Shot，也不进入 StoryGraphVersion。只要任一视觉要求没有合法 AssetVersion，Batch/Set 就保持明确 `needs_asset`，不能填空 UUID、选择“最新版本”或用自由文本绕过。

## 5. Draft Gate 与付费操作前门

Backend 对 Draft Candidate 执行：

1. Scene/Beat/Evidence/Occurrence Ref 存在且属于同一 Workspace/Project/Episode；
2. required Beat/Dialogue coverage 完整，无未知或重复临时 Key；
3. Shot 顺序、建议时长与 Episode/Scene 预算确定；
4. 每个视觉 occurrence 恰好有一个严格需求，Identity/AssetState 与 Bible/Planning Owner 一致；
5. 已给出的 AssetVersion 必须 READY，Lineage/Style/View Role 与需求一致；
6. Canonical Sort/Hash 与 Candidate Revision 谱系可重建。

用户审核 Shot Intent 和视觉需求后，Storyboard Owner 的 `FreezeIntentSet` 只冻结 Draft Set revision/hash、已接受 Intent、视觉需求、ReviewDecision 与 Command Receipt，输出 `approved_storyboard_intents`。它不创建正式 Shot、Asset、AssetVersion、Provider Job 或 StoryGraphVersion。

只有该 Gate 输出提交成功后，`reference_asset` Generation 才能产生费用或远程 Provider Job。拒绝、要求修改、Owner Receipt unknown 或需求漂移时不得开始付费生成。

## 6. 资产准备与 Detail 输入

参考资产生成属于 Backend Generation/Asset Workflow，不属于 Storyboard Harness：

```text
SpecificationVersion + AssetState + EffectiveStyleSnapshot
  → reference_asset Intent / Cost / Quota / Provider Job
  → READY composite reference_sheet Artifact
  → deterministic QC(front/profile/back)
  → Human CandidateSelection
  → published AssetVersion
```

`detail_shots` 只能在当前 Intent Set 所需视觉输入全部可解析为精确 AssetVersion 后启动。每个引用必须冻结 Asset ID/State、AssetVersion ID/revision/hash、Artifact/Lineage Hash、Style Snapshot Hash 与 view-role coverage；不得读取当前指针或接受 Agent 自报 URL。

Detail Candidate 在 Draft Intent 基础上补全可生产镜头字段、精确资产绑定、连续性输入/输出、相邻镜头约束与 frame generation intent。它不能改变已批准的 source coverage、Identity/State 或视觉需求；需要改变时必须回到新的 Draft Candidate/Human Gate，而不是在 Detail 中偷偷修订。

## 7. Detail Gate、Review 与 Owner Apply

Detail 的确定性 Gate 必须证明：

- 每个 approved Intent 都有且只有一个 Detail，未增加来源外 Shot；
- 每个 Occurrence 的 AssetState 与 AssetVersion Lineage 对齐；
- 所需 front/profile/back/其他 view role 被真实 Artifact 覆盖；
- 镜头时长、全局顺序、timecode、对白/动作来源和相邻连续性一致；
- `ShotProductionBindingVersion.entries` 完整、排序、不可重复，并与 Shot Revision/Hash 同时冻结；
- 不存在 `needs_asset`、未知引用、空 Binding 或“最新资产”。

`review_storygraph(scope=storyboard)` 只能提出有 Evidence/Owner Ref 的 Review Issue。Repair 最多执行 Requirement 固定的有界轮次；每轮由 Backend 应用 Patch 创建下一 Candidate Revision，并重跑受影响闭包的全部 Gate/Review。模型 Reviewer 不能把自己的意见伪装成 Tool 结论，也不能降级真实资产 blocker。

Human Gate 正向决议后，Storyboard Owner 在一个 GORM 事务中锁定 Draft Set/Batch 与正式 Shot baseline，创建完整批次的正式 Shot、对应 `ShotProductionBindingVersion` 和 Command Receipt。任何 Episode/Asset/Hash 漂移都全批回滚；重复 Signal 只重放同一 Owner Receipt。随后 StoryGraph Compiler 才发布包含正式 Shot/Binding 的下一 Version。

`ShotImageBindingVersion` 仍只保存后续 shot-frame 生成结果选择，不能代替生产输入 Binding。

## 8. 本地 Codex 执行边界

Python Runtime 继续以受控本地 Codex CLI 作为开发期模型执行器：

- `--ephemeral --sandbox read-only --ignore-user-config --output-schema`；
- 临时空工作目录，不向模型暴露仓库、数据库、对象存储、Kafka、Elasticsearch、Shell、Web、浏览器或文件写入工具；
- 未配置模型时继承本机 Codex 配置，显式配置时由 Backend ExecutionPolicy 冻结；
- 每个 Invocation 有模型调用数、Token/费用和技术 deadline；完整 Workflow 由 Temporal 分阶段持久推进，不用一个超长 CLI 进程承担全部剧本；
- Schema invalid、tool policy violation、deadline、进程失联和 outcome unknown 使用稳定错误码返回 Backend。

Codex CLI 的网络只用于受控模型调用，不能成为业务 Tool 网络。Agent 不接收业务凭据，不写 PostgreSQL，也不产生 Kafka Event。

## 9. 恢复、幂等与失败路径

恢复事实只保存在 Backend：WorkflowRun/NodeRun、ShardManifest、AgentInvocation/Result、StageCandidateRevision/Head、HumanTask、Owner Receipt。AgentInvocation Lease/Claim Version 只用于单个执行者 Fencing，不是业务时钟；Kafka 不承担 Lease、Timer 或 Workflow 调度。

- Bundle/Reference/Schema/Policy Hash 缺失或漂移：Invocation 启动失败；
- Codex 结果 unknown：保留同一 Invocation，对账后再决定，不生成新业务身份；
- 迟到旧 Shard/旧 Claim：Fencing/Candidate Head CAS 拒绝写入；
- Draft 缺资产：返回合法 `needs_asset` Candidate，不伪造 Detail；
- Detail 资产未 READY、Lineage/Style/View Role 漂移：拒绝执行或提交；
- Reviewer scope 幻觉：作为无效 Review Issue 拒绝，不改写 Tool Gate；
- Repair 超出允许集或轮次耗尽：保持 needs_review/failed，不自动放宽门禁；
- Owner Receipt 已提交但 Workflow Signal unknown：只重放同一 Receipt；
- Storyboard baseline 漂移：Owner Apply 全批冲突，不覆盖已有 Shot。

## 10. MVP 验证

最小验收必须证明：

- 运行时只读取 `agent/skills/build-storygraph`，旧 `.agents/skills` 名称不存在 fallback；
- 同一 Draft 输入得到稳定 Candidate/Revision Hash，并完整覆盖 Scene/Beat/Occurrence；
- 空资产输入只能得到 `needs_asset`，无法进入 Detail 或正式 Shot；
- 未经 Intent Human Gate 不创建 Provider Job 或费用预留；
- reference sheet 通过真实 READY/QC/Selection 后，Detail 只能绑定精确 AssetVersion；
- 至少一个角色跨两个 AssetState、一个地点状态和多个镜头保持同一 Identity，Binding 指向正确版本；
- 进程重启、迟到 Result、Candidate Repair、Owner Signal unknown 和 baseline 冲突均可恢复；
- Owner Apply 后的 Shot/ShotProductionBindingVersion 可由 StoryGraphVersion 反查 Evidence、Occurrence、AssetState 和 AssetVersion；
- Agent 业务代码只位于 `agent/app`，Skill 只位于 `agent/skills`，测试只位于 `agent/tests`；Backend 测试只位于 `backend/tests`。

本文只接受设计，不声明上述缺口已经实现。实施仍按 [0010 的 `SG-Ixx`](0010-StoryGraph内容图与DAG创作画布设计.md#唯一实施任务队列)和 [3003 的映射](3003-StoryGraph剧本解析Harness与内置Skill设计.md#实施任务映射)推进；所有代码与真实 CI 通过后，最后才使用 `agent-browser` 做 Web 全旅程验收。
