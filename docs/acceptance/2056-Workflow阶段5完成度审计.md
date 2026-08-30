# Workflow 阶段 5 完成度审计

- 状态：静态证据审计完成；阶段 5 总项保持未完成
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 最新运行证据：[Agent 执行总时限](2054-Agent执行总时限验收记录.md) · [Workflow 重复投递收敛](2052-Workflow重复投递收敛验收记录.md)

## 审计目的

本记录只对当前阶段 5 的设计、Requirement、Plan、实现与 Acceptance 进行逐项对账，明确哪些能力已经有真实证据、哪些仍是独立切片或外部前置。它不新增运行时行为，也不以文件存在、测试名称或历史完成声明代替真实验收。

本次审计的直接结论是：Compiler、Definition/Input Snapshot、Run/Node Projection、Node Cache、Start/Control/Signal、正式 Episode/Shot Workflow、History Replay、内部 HumanTask/Decision 和当前已发布 Human Gate Owner Receipt 均已有真实 PostgreSQL/Temporal 证据；阶段 5 仍不能整体完成，因为真实图片 Generation、Episode 动态 Shot 扇出、公共 Human Gate 恢复入口、Workflow `QualityGateResult` 和完整 Agent Run/Usage 契约尚未落地。

## 证据矩阵

| Plan 总项 | 当前事实 | 尚缺完成门 | 判定 |
|---|---|---|---|
| `BE-MOD-005` | Compiler、WorkflowDefinitionVersion、RunInputSnapshot、Run/Node Projection、Node Input/Output、Node Cache、持久等待、取消/暂停/恢复和单 Shot 派生 Run 已通过真实组件验收。 | Backend 尚无 Workflow `QualityGateResult` 实现；真实图片 Generation Executor 和 Episode 到 Shot 的动态扇出未完成。 | 保持未完成 |
| `BE-JRN-003` | 稳定 Workflow ID、Start/Control/Signal Intent/Receipt、UNKNOWN 对账、独立 `lanverse.episode-production`/`lanverse.shot-production`、Worker 重启恢复和 History Replay 已通过。 | Episode 目前不能从正式 Storyboard Shot 自动产生真实 CandidateSet 并启动 `ShotWorkflow × N`，所以剧集到镜头生产仍是两个独立的半闭环。 | 保持未完成 |
| `BE-MOD-006` | HumanTask、Claim/Renew/Release/Expire、Lease Fencing、过期接管、不可变 ReviewDecision、Stale 与候选防篡改均已在 PostgreSQL 验证。 | `lanverse-api` 尚未公开 Task 查询和领取/续租/释放/决议恢复；Expire Sweep 也未装配生产调度，因此真实跨浏览器/跨日入口未完成。 | 内部模块完成，产品闭环未完成；Plan 总项暂不勾选 |
| `BE-JRN-006` | Production Bible、Episode Plan、Episode Structure、Storyboard 与 Generation Image 五类已发布 Gate 均使用目标 Owner Receipt、Workflow Apply Receipt 和 Temporal Signal；响应丢失与重放保持一次业务效果。 | 公共调用方无法提交并恢复同一个 Decision；该入口必须按待接受的公共 Human Gate Design 实现，不能由 Handler 旁路现有协调服务。 | 内部协调完成，公共 Journey 未完成；Plan 总项暂不勾选 |
| `BE-MOD-008` | Production Bible/Storyboard 已冻结 Agent Definition、Prompt/Skill/Schema、调用次数、900/600 秒 Invocation 时限和空 Tool Allowlist；本地 Codex CLI 的独立失败语义已验证。 | Token/费用、绝对排队 deadline、AgentRun/Usage Receipt、Workflow Run/Node Grant 作用域、防重放 Receipt、Backend Tool API 与生产 Model Gateway 未完成。 | 保持未完成 |
| 受限 Executor 边界 | 当前本地 Runtime 已禁用 Shell/Web/Plugins 等能力并严格校验结构化结果；Codex CLI 仅执行文本/结构化候选。 | Structured/ToolLoop/LangGraph 的完整选择契约与 Backend Tool Allowlist API 尚未实现。 | 保持未完成 |

## 不应重复实现的能力

以下行为已有组合或真实组件证据，后续切片应复用现有内核，不再建立兼容层或第二实现：

1. Episode 与 Shot 使用同一确定性执行内核，但以不同正式 Temporal Workflow Type 和编译身份启动。
2. Start、Cancel、Pause、Resume、Human Gate Signal 均使用稳定 Intent/Receipt，并对 UNKNOWN 结果按原身份对账。
3. Human Gate 打开时冻结 `node-input-canonical`；成功 Gate 只传播目标 Owner 已提交的 canonical `node-output-canonical`。
4. Worker 重启依赖 Temporal History 恢复，不增加恢复表、内存队列或第二工作流引擎。
5. 单 Shot 重跑派生新 Run 并保留源 Run；当前只在已物化 CandidateSet 内重新审核和追加 Binding Revision。
6. Backend 是唯一业务 Writer，PostgreSQL/GORM Model Catalog 是唯一 SQL 事实源；不得增加 Migration、DDL、Raw SQL 或第二 ORM。
7. 本地 Codex CLI 只属于 Agent 文本/结构化调用，不得作为图片 Provider、Backend Writer 或缺失凭据时的降级路径。

## 当前阻塞链路

下列内容只记录审计时识别出的能力缺口，不再构成可直接执行的两条链。当前唯一顺序由 [StoryGraph 文档队列](../README.md#当前-storygraph-设计文件推进顺序) 的 `SG-D01`–`SG-D21` 维护：`2051/2055` 分别冻结至 `SG-D13/SG-D16`，统一 PRD、Requirement、Plan 和 Acceptance 只能在 `SG-D17`–`SG-D21` 建立。

```text
SG-D01 当前设计评审
  → SG-D02–SG-D16 逐份接受/同步 Design
  → SG-D17–SG-D21 统一 PRD / Requirement / Plan / Acceptance
  → SG-Ixx 每个任务真实 CI、证据与独立提交
  → 全部开发完成后最终 agent-browser 验收
```

- [通用媒体 Provider 与 Generation 执行器 Design](../design/2051-通用媒体Provider与Generation执行器设计.md)当前待重新接受；在此之前不得派生或编码真实媒体切片。
- [Workflow 公共 Human Gate 命令与恢复 Design](../design/2055-Workflow公共HumanGate命令与恢复设计.md)尚待用户接受；在此之前不得公开半套 Decision API 或在 Frontend 模拟成功。
- `QualityGateResult` 与完整 Agent Run/Usage 仍只有高层 Requirement，没有足够的已接受细化设计；不得借本审计自行补表或字段。

## 验证边界

本任务只新增审计文档和文档索引，没有修改 Backend、Agent、Frontend、OpenAPI、Compose 或 CI。运行时最新证据仍以 2054 记录的完整 Required CI 为准；本任务只执行文档相对链接、`git diff --check`、工作区和提交内容检查，不把未重新执行的运行时 CI 报告为本任务通过。
