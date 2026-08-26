# 本地 Codex 分镜智能体执行框架实施计划

- 状态：已冻结；待 `SG-D20` 唯一 StoryGraph 总 Plan 取代
- 日期：2026-08-25
- 产品依据：[本地 Codex 分镜智能体产品需求](../prd/3002-本地-Codex-分镜智能体产品需求.md)
- 设计依据：[本地 Codex 分镜智能体执行框架设计](../design/3002-本地-Codex-分镜智能体执行框架设计.md)
- 需求依据：[本地 Codex 分镜智能体执行框架需求规格](../requirement/3002-本地-Codex-分镜智能体执行框架需求规格.md)
- 验收依据：[本地 Codex 分镜智能体执行框架验收标准](../acceptance/3002-本地-Codex-分镜智能体执行框架验收标准.md)
- 执行门禁：本旧 5-Skill Plan 已冻结，不是当前任务入口；待文档中心 `SG-D12`–`SG-D21` 完成后，由 `0010` 唯一总 Plan 取代其 StoryGraph 实施顺序

## 计划目标

从零建立“固定剧本与 Bible 输入 → 多阶段分镜 Harness → 确定性门禁 → 人工审核 → 原子应用
→ 可复现导出”的完整分镜候选链。计划只描述未来实施顺序，不声明任何能力已经完成。

## 执行原则

1. 固定输入、领域 schema、状态和失败语义先于模型提示与 Worker 编排。
2. 每阶段遵循 Red → Green → Refactor，并以 Requirement ID 作为唯一追踪键。
3. 创意判断归模型，来源、覆盖、时长、资产、所有权和正式写入归确定性应用边界。
4. Skill 始终 candidate-only；Provider 成功也只能产生 `needs_review`。
5. 每个 checkpoint 和写入点都校验固定输入、版本、哈希与 run token。
6. 真实本机 Codex、Bible-backed 端到端与 60 集验收必须分层执行并分别留证。

## 阶段 0：冻结输入、领域与状态契约

### Checklist

- [ ] 定义 Draft batch、fixed input、scene context、candidate、issue、checkpoint 与导出包模型。
- [ ] 冻结确认脚本、叙事单元、required coverage、场景/对白、目标时长、画幅、视觉风格、Bible snapshot、occurrence 和 world entry（`SB-FR-001`）。
- [ ] 定义确定性 input hash、result hash、版本和规范化规则（`SB-NFR-006`）。
- [ ] 定义 `queued/running/needs_review/failed/unknown/cancelled/applied` 的合法迁移与失败语义。
- [ ] 定义五个稳定 Skill 标识及显式调用契约（`SB-NFR-004`）。
- [ ] 为输入漂移、跨作用域引用、非法状态迁移和哈希不一致先建立失败用例。

### 退出门

- [ ] `SB-FR-001`、`SB-NFR-004`、`SB-NFR-006` 的 schema 与契约用例可独立复现。
- [ ] 模型执行前即可拒绝不完整、漂移或跨作用域输入。

## 阶段 1：实现多阶段 Harness 与确定性门禁

### Checklist

- [ ] 按确认场景构建 context，并确定性分配总和守恒的场景时长预算（`SB-FR-002`）。
- [ ] 实现 source analysis、scene plan、shot draft、hard gate、review、targeted repair 和 final gate 的阶段编排（`SB-FR-003`）。
- [ ] 实现 required unit、来源 position、scene key 与 beat key 的前置覆盖校验（`SB-FR-004`）。
- [ ] 实现来源/场景/对白、required coverage、±25% 场景时长、基础 side continuity 和 asset position 门禁（`SB-FR-005`）。
- [ ] 实现来源明确提及固定资产时的同位置绑定门禁（`SB-FR-006`）。
- [ ] 确定性生成全局镜号、连续 timecode、总时长与 result hash（`SB-FR-007`）。
- [ ] 为跨场景引用、未知对白、漏覆盖、预算越界、左右侧突变、未知资产位置和漏绑定建立失败矩阵。

### 退出门

- [ ] `SB-FR-002` 至 `SB-FR-007` 的正常与失败路径均可确定性复现。
- [ ] 相同规范化输入得到相同镜号、timecode、预算和 result hash。

## 阶段 2：实现独立 Review 与有限修复

### Checklist

- [ ] 实现 purpose、反应、揭示、可拍性和工艺质量的独立 Reviewer（`SB-FR-008`）。
- [ ] 归一化 issue provenance、scope、severity 与稳定 risk code。
- [ ] 将无 Tool 证据的资产 blocker 降为 warning，将无效 scope 转为可追踪 warning。
- [ ] 仅重跑受 blocker 影响的场景，并把总修复轮次限制为两轮（`SB-FR-009`）。
- [ ] 每轮修复后重新执行全部相关 hard gate 与全局组装。
- [ ] 两轮仍失败时返回明确失败，不暴露半成品 candidate。
- [ ] 对复杂轴线、视线、动作匹配和音画桥建立 Reviewer/人工风险基线，不提前升级为 hard gate（`SB-FR-017`）。

### 退出门

- [ ] Reviewer 幻觉不能伪装 Tool 事实或触发无关场景重写。
- [ ] 未通过最终门禁的结果不会进入人工审核候选。

## 阶段 3：建立 Checkpoint、所有权与取消

### Checklist

- [ ] 为每个阶段定义绑定 batch、task、input hash、Harness 版本、run token 和 payload 的 checkpoint（`SB-FR-013`）。
- [ ] 对 terminal checkpoint 重算 candidate hash、timeline、总时长、issue 注解和终态（`SB-FR-014`）。
- [ ] 实现可续租 lease、heartbeat、活跃重投 requeue、过期恢复和旧 token fencing（`SB-FR-016`、`SB-NFR-002`）。
- [ ] 实现 queued/running Draft batch 的显式取消与本机 Codex 终止（`SB-FR-015`）。
- [ ] 区分 Provider 确定失败、结果不确定、用户取消和所有权丢失。
- [ ] 验证本机 Codex 使用 ephemeral、read-only sandbox、严格 JSON schema 和最多两次结构校正（`SB-NFR-001`）。
- [ ] 验证未显式配置 model 或推理强度时继承本机配置，且不设置固定业务墙钟超时（`SB-NFR-003`）。
- [ ] 建立损坏 checkpoint、活跃重投、lease 过期、续租短暂失败、旧 Worker 晚到与取消竞态矩阵。

### 退出门

- [ ] 合法恢复只重跑未完成阶段，非法 checkpoint fail closed。
- [ ] 同一 batch 不会并行运行两个 Codex，取消或 unknown 后不会产生 Draft/Shot。

## 阶段 4：完成人工审核、原子应用与导出

### Checklist

- [ ] 只把通过最终门禁的结果保存为 `needs_review` candidate（`SB-FR-010`）。
- [ ] 实现逐项接受/拒绝、未决项门禁、batch 审批与追加式决议记录（`SB-FR-011`）。
- [ ] 实现 baseline/impact preflight 与 expected hash 校验。
- [ ] 实现正式 Shot 的原子、幂等 apply，冲突时不产生半写入。
- [ ] 生成包含 manifest、CSV、HTML 与 JSON 的确定性交付包（`SB-FR-012`）。
- [ ] 验证时间码、镜头语言、动作、对白/声音、连续性、首/关键/尾帧、来源与资产字段完整。
- [ ] 验证不可信文本转义和相同 snapshot 的导出内容哈希稳定。

### 退出门

- [ ] 未经人工审批的 candidate 不能创建正式 Shot。
- [ ] 决议、apply 和导出均可反查 fixed input、版本、哈希和审计事实。

## 阶段 5：真实本机 Codex 与 Bible-backed 合约

### Checklist

- [ ] 验证运行时不依赖 DeepSeek、外部 Skill 仓库、上游 checkout、submodule 或 Provider registry（`SB-NFR-005`）。
- [ ] 固定真实本机 Codex、model、Skill、reference、输入与环境版本。
- [ ] 使用至少两个确认场景，覆盖动作、对白、跨空间动作和来源明确的关键道具（`SB-AC-001`）。
- [ ] 验证 required coverage、引用、预算、镜号、timecode、资产绑定、Review 与定向修复。
- [ ] 使用 confirmed Bible、occurrence 与 world context 完成 Draft request → review → approve → apply → export（`SB-AC-002`）。
- [ ] 验证调用方不手工传 Bible 资产、正确状态自动注入、world context 可见且 snapshot 漂移被拒绝。
- [ ] 记录输入哈希、result hash、Codex/model/Skill 版本、执行结果、失败样本和人工修订。

### 退出门

- [ ] `SB-AC-001` 与 `SB-AC-002` 均由不使用模型桩的独立证据确认。

## 阶段 6：60 集批量与人工验收

### Checklist

- [ ] 为完整 60 集冻结与 Production Bible 验收相同的不可变 snapshot（`SB-AC-003`）。
- [ ] 逐集统计场景、unit、镜头、required coverage、总时长、引用、issue、repair round、审核和恢复结果。
- [ ] 验证无调用方手工注入 Bible 资产，无关全项目资产不进入场景。
- [ ] 对重启、重复投递、续租短暂失败、lease 过期、取消和输入漂移做故障注入。
- [ ] 对 E01、E07、E09、E31、E36、E60 进行导演级人工细查，不以代表集替代全量机器统计。
- [ ] 汇总失败率、人工修订成本、误报/漏报和未决风险。

### 退出门

- [ ] [本地 Codex 分镜智能体执行框架验收标准](../acceptance/3002-本地-Codex-分镜智能体执行框架验收标准.md)中的全部 Requirement 与 60 集条目均已由独立证据确认。

## 完成定义

- [ ] `SB-FR-001` 至 `SB-FR-017` 全部满足。
- [ ] `SB-NFR-001` 至 `SB-NFR-006` 全部满足。
- [ ] `SB-AC-001` 至 `SB-AC-003` 全部满足。
- [ ] 真实本机 Codex、Bible-backed 端到端和 60 集批量验收均通过。
