# 本地 Codex 分镜智能体执行框架验收标准

- 状态：旧派生文档已冻结；由 `SG-D21` 的全新 Acceptance 替代，不继承完成证据
- 日期：2026-08-25
- 产品依据：[本地 Codex 分镜智能体产品需求](../prd/3002-本地-Codex-分镜智能体产品需求.md)
- 设计依据：[本地 Codex 分镜智能体执行框架设计](../design/3002-本地-Codex-分镜智能体执行框架设计.md)
- 需求依据：[本地 Codex 分镜智能体执行框架需求规格](../requirement/3002-本地-Codex-分镜智能体执行框架需求规格.md)
- 实施依据：[本地 Codex 分镜智能体执行框架实施计划](../plan/3002-本地-Codex-分镜智能体执行框架实施计划.md)

## Checklist 口径

- 所有条目初始均为 `[ ]`，表示尚未经过本主题的独立验收。
- 只有 Requirement 的完整正常路径、失败路径和边界条件都有可复查证据后，才可标记为通过。
- 模型桩只能验证受控编排，不能替代真实本机 Codex、confirmed Bible 或人工 apply 合约。
- 代表集只用于人工细查，不能替代 60 集逐集机器统计。
- 每次勾选必须同时记录输入与哈希、环境与版本、执行步骤、预期与实际结果、失败样本、审阅人和日期。

## Requirement Checklist

### 输入与分阶段执行

- [ ] `SB-FR-001`：Draft input 冻结确认脚本、叙事单元、required coverage、场景/对白、时长、画幅、风格、Bible snapshot、occurrence 资产和 world entry；任一漂移均被拒绝。
- [ ] `SB-FR-002`：scene context 按确认场景划分；叙事单元不跨场景，occurrence 资产只进入相关场景，场景预算之和等于总目标时长。
- [ ] `SB-FR-003`：source analysis → scene plan → shot draft → hard gate → review → targeted repair → final gate 顺序完整，多场景可并行且最终组装稳定。
- [ ] `SB-FR-004`：analysis/plan 覆盖全部 required unit，只引用所属场景 position 与合法上阶段 key；缺失或未知引用立即失败。
- [ ] `SB-FR-005`：跨场景来源、未知对白、漏覆盖、±25% 时长越界、可证明的 side continuity 突变和未知资产位置均形成 Tool blocker。
- [ ] `SB-FR-006`：来源逐字明确提到固定资产时，承载该来源位置的至少一个镜头绑定该资产；Reviewer 不能自行制造资产 blocker。
- [ ] `SB-FR-007`：全局镜号、连续 timecode、总时长与 result hash 均由应用确定性生成，相同规范化候选结果一致。

### Review、修复与人工应用

- [ ] `SB-FR-008`：Reviewer 独立检查 purpose、反应、揭示、可拍性和工艺质量；伪造 provenance、无效 scope 和无证据 asset blocker 被稳定归一化。
- [ ] `SB-FR-009`：修复只重跑受影响场景，总轮次不超过两轮，每轮后重新执行 hard gate；仍有 blocker 时不暴露半成品 candidate。
- [ ] `SB-FR-010`：成功运行只产生 `needs_review` candidate，并携带 timeline、issue、risk code、input/result hash 与 Skill 版本，不创建正式 Shot。
- [ ] `SB-FR-011`：用户完成逐项决议、batch 审批、impact preflight 和原子 apply 后才创建正式 Shot；baseline 漂移或冲突不产生半写入。
- [ ] `SB-FR-012`：确定性交付包至少包含 manifest、CSV、HTML 和 JSON，字段完整、哈希稳定，并正确转义不可信文本。

### Checkpoint、恢复、取消与连续性

- [ ] `SB-FR-013`：checkpoint 绑定 batch、task、input hash、Harness 版本、run token、stage 与 payload；损坏、旧版、跨任务或 forged token 均 fail closed。
- [ ] `SB-FR-014`：terminal checkpoint 的 candidate hash、timeline、总时长、issue 注解和终态全部重算一致后才可直接恢复。
- [ ] `SB-FR-015`：queued/running Draft batch 可显式取消；取消 fencing token 并终止本机 Codex，取消或 unknown 后不写 Draft/Shot。
- [ ] `SB-FR-016`：活跃 lease 重投只 requeue，过期后新 token 才可恢复；同一 batch 不并行运行两个 Codex，旧 token 不能保存或提交。
- [ ] `SB-FR-017`：复杂轴线、视线、动作匹配和音画桥在确定性规则与误报基线接受前只作为 Reviewer/人工风险，不被误报为 hard gate 通过。

### 非功能需求

- [ ] `SB-NFR-001`：本机 Codex 使用 ephemeral、read-only sandbox、严格 JSON output schema 和最多两次结构校正；取消可终止子进程。
- [ ] `SB-NFR-002`：5 分钟 lease 仅表达 Worker 所有权；心跳正常可连续续租，短暂失败在有效期内重试，实际过期或 fencing 才取消运行。
- [ ] `SB-NFR-003`：未显式配置 model 或推理强度时继承本机 Codex 配置，剧本与分镜阶段均无固定业务墙钟超时。
- [ ] `SB-NFR-004`：`analyze-scene`、`plan-scene`、`draft-shots`、`review-shots`、`repair-shots` 的声明、标识与显式调用一致，无旧别名或空占位模块。
- [ ] `SB-NFR-005`：运行时不依赖 DeepSeek、外部 Skill 仓库、上游 checkout、submodule 或 Provider registry。
- [ ] `SB-NFR-006`：候选、checkpoint、人工决议、apply 与导出均可追溯到 fixed input、哈希和版本；私有 run token 不进入公开结果。

### 真实业务验收

- [ ] `SB-AC-001`：使用已登录的真实本机 Codex 完成至少两个确认场景，覆盖动作、对白、跨空间动作和来源明确的关键道具，不使用模型桩。
- [ ] `SB-AC-002`：使用真实 confirmed Bible、occurrence 与 world context 完成 Draft request → review → approve → apply → export；调用方不手工传 Bible 资产。
- [ ] `SB-AC-003`：完整 60 集均有镜头、required coverage、引用、时长、issue、审核与恢复统计；E01/E07/E09/E31/E36/E60 另完成人工细查。

## 真实本机 Codex 验收矩阵

- [ ] 固定 Codex、model、Skill、reference、schema、Harness、输入与环境版本。
- [ ] 至少两个确认场景中的动作、对白、跨空间动作和关键道具都进入合法 scene context。
- [ ] required 来源全覆盖，无跨场景引用，场景预算、全局镜号、timecode 与总时长全部合格。
- [ ] 来源明确的关键资产绑定到承载相同来源位置的镜头。
- [ ] Reviewer 无效 scope、伪造 Tool provenance 或资产幻觉不会触发错误修复。
- [ ] 有限修复只影响目标场景，两轮仍失败时不产生 candidate。
- [ ] confirmed Bible 的正确资产状态和 world context 由 occurrence 自动注入，snapshot 漂移被拒绝。
- [ ] 合法 checkpoint 可恢复，损坏 checkpoint、旧 token、重复投递和取消竞态全部 fail closed。
- [ ] 人工 decision、approve、preflight、apply 后才创建正式 Shot。
- [ ] manifest、CSV、HTML 与 JSON 导出字段完整且内容哈希可复现。

## 60 集验收矩阵

未来验收输入与 Production Bible 验收使用同一不可变原稿和 Bible snapshot。

- [ ] 60 个 Draft batch 全部绑定同一有效 Bible id、revision 与 result hash。
- [ ] 逐集记录场景数、unit 数、镜头数、required coverage、总时长和 result hash。
- [ ] 逐集记录 scene/dialogue/asset 引用、Tool issue、Reviewer issue、risk code 和 repair round。
- [ ] 逐集验证资产由 occurrence 自动派生，无关全项目资产不进入场景。
- [ ] 逐集完成人工决议、审批、impact preflight、原子 apply 和确定性导出。
- [ ] 对重启、重复投递、续租短暂失败、lease 过期、取消和输入漂移执行故障注入。
- [ ] E01、E07、E09、E31、E36、E60 完成导演级人工细查，不替代全量机器统计。
- [ ] 记录失败率、人工修订成本、误报/漏报、恢复结果和未决风险。

## 验收证据包

每轮验收必须形成一份不可变记录，至少包含：

1. Requirement ID 与验收场景编号。
2. fixed input、Bible snapshot、input hash、result hash 与 Draft/Shot 版本。
3. Codex、model、Skill、reference、schema、Harness 与运行环境版本。
4. 前置条件、执行步骤、预期结果和实际结果。
5. 自动统计、人工结论、issue/decision、失败注入和恢复结果。
6. 审阅人、执行日期、证据位置与未决风险。

## 通过规则

- [ ] `SB-FR-001` 至 `SB-FR-017` 全部勾选。
- [ ] `SB-NFR-001` 至 `SB-NFR-006` 全部勾选。
- [ ] `SB-AC-001` 至 `SB-AC-003` 全部勾选。
- [ ] 真实本机 Codex 与 60 集验收矩阵全部勾选，且不存在未豁免的 blocker。
