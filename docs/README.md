# Lanverse 文档中心

`docs/` 是产品范围、技术决策、可验证需求、实施顺序和验收证据的长期事实入口。目录只保留 `design/`、`prd/`、`requirement/`、`plan/`、`acceptance/` 五类文档；架构与设计决策统一收口到 `design/`，不再建立平行目录。

复杂功能先读取当前正式文档、代码、依赖、Schema、接口、测试和真实运行状态，再形成 Design；Design 必须把“当前事实”“目标设计”和“尚未验证”分开。既有实现可以作为设计输入和缺口证据，但不能自动证明新 Requirement、Plan 或 Acceptance 已完成。所有新实施与目标验收 Checklist 初始均为 `[ ]`。

## 目录职责

| 目录 | 回答的问题 | 必须包含 | 不应包含 |
|---|---|---|---|
| `design/` | 系统和模块如何设计 | 决策与取舍、边界、数据与接口、状态、失败路径 | 易变排期、未验证的完成声明 |
| `prd/` | 在已接受设计边界内为什么做、为谁做、做什么 | 产品目标、范围、非目标、发布门 | 推翻 Design 的隐式架构、执行日志 |
| `requirement/` | 什么契约必须被满足 | 可测试的功能/非功能需求、输入输出、约束 | 方案推演、模糊愿景 |
| `plan/` | 按什么顺序安全落地 | 依赖、阶段、交付门、回滚点、执行 Checklist | 把计划状态当作实现事实 |
| `acceptance/` | 如何判定完成、实际验证了什么 | 验收标准、命令、输入、结果、缺失条件与残余风险 | 没有证据的“已通过”结论 |

## 文档链路

默认评审与实施顺序为：

```text
Design 集按依赖逐份接受 → 同步受影响既有 Design → PRD → Requirement → Plan → Acceptance Criteria
                                                              ↓
                                         Implementation/CI → Acceptance Evidence → agent-browser
```

先用 Design 基于当前事实固定问题、边界、Owner、数据/接口、状态、失败路径与取舍；同一目标的 Design 集必须按依赖逐份被用户接受，随后才同步它们影响的既有 Design，并在统一边界内派生 PRD 的产品范围、可测试 Requirement、可执行 Plan 和初始全为未通过的 Acceptance Criteria。编码只能从已接受 Plan 中领取一个完整任务；每个任务经真实 CI 后独立提交，并把真实命令、输入和结果回填为 Acceptance Evidence。完整开发和 CI 都通过后才进行浏览器全流程验收。小型变更可以只创建必要文档，但跳过某类文档时必须在 Design 中说明原因，不为凑齐链路批量建立空文件。

同一主题的文档门禁固定如下：

1. Design 未接受：只允许调研、设计修订和验证当前事实，不派生 PRD/Requirement/Plan，不编码。
2. 目标 Design 集已按依赖全部接受：先同步所有被它们取代或影响的既有 Design，再创建并逐份接受 PRD、Requirement、Plan。
3. Plan 已接受：按依赖顺序实施；每个完整任务执行 Red → Green → Refactor 和真实 CI，随后回填该任务 Acceptance Evidence、检查 diff/hygiene，再独立提交，不提前累计多个任务。
4. Acceptance：初始标准可以在编码前建立但全部未勾选；只有本任务新执行的真实命令、输入和结果才能更新证据，证据属于该任务提交的一部分。
5. 全部开发切片与全量真实 CI 通过后，才运行 `agent-browser` 完成最终 Web Journey；随后回填浏览器证据、执行文档/diff/hygiene 检查并提交最终验收文档。浏览器验收不能替代单元、契约、集成或后端事实验证。

Plan 内的 Checklist 追踪“下一步做什么和执行到哪里”；Acceptance 内的 Requirement Checklist 追踪“哪些契约已有按新设计重新执行的真实证据”。两者不得互相替代：Plan 勾选不能证明验收通过，历史实现证据也不能预先勾选新设计。

## 当前 StoryGraph 设计文件推进顺序

下表是当前 StoryGraph 主题唯一的文档推进队列，不按文件编号、目录顺序或旧 Plan 顺序并行实施。任意时刻只激活一步；上一步已评审、完成必要修订、通过文档检查并独立提交后，下一步才解锁。只复核且无语义变化时，只更新本队列的当前步骤/复核证据并提交该可追溯状态，不为目标 Design 制造无意义正文变更或空提交。

| Step | 唯一对象 | 本步完成门 | 未完成时的限制 |
|---|---|---|---|
| `SG-D01` | [0010 StoryGraph 总设计](design/0010-StoryGraph内容图与DAG创作画布设计.md) | **已完成（2026-08-27）**：用户已接受 StoryGraph、DAG、角色/地点视觉与四图边界 | 仅解锁 `SG-D02`；仍不移动 Skill、不编码 |
| `SG-D02` | [3003 Agent/Harness 子设计](design/3003-StoryGraph剧本解析Harness与内置Skill设计.md) | **已完成（2026-08-27）**：用户已接受 Bundle、Stage、Shard、Candidate Revision 和 Codex 边界 | 仅解锁 `SG-D03`；不以 Agent 子设计反向改写 `0010` |
| `SG-D03` | [0006 领域语言](design/0006-领域语言与模块命名规范.md) | **已完成（2026-08-27）**：已固定 StoryGraph、Asset/State/Version、Claim、Occurrence 和 Binding 规范名 | 仅解锁 `SG-D04`；下游 Design 不得自创同义词 |
| `SG-D04` | [0001 完整设计基线](design/0001-AI短剧制作平台完整设计基线.md) | **已完成（2026-08-27）**：平台主干收口为 StoryGraph 与四图边界，并固定 Kafka 异步解耦、剧本/StoryGraph 检索和 ELK + Kafka 日志链路 | 仅解锁 `SG-D05`；不先改其他子模块 |
| `SG-D05` | [2003 语言与运行边界](design/2003-后端语言与运行边界策略.md) | **已完成（2026-08-27）**：固定 Backend 唯一 Writer、GORM Catalog、`agent/skills/build-storygraph`、受控 Codex CLI 与按真实消费者创建 Binary | 仅解锁 `SG-D06`；不在 Agent 增加业务 Writer |
| `SG-D06` | [0003 系统总体架构](design/0003-系统总体架构.md) | **已完成（2026-08-27）**：重建 StoryGraph/四图系统图、两条 Compiler 链、单 Writer、Kafka 检索与 ELK 日志事实边界 | 仅解锁 `SG-D07`；不把 StoryGraph 与 WorkflowDefinition 合并 |
| `SG-D07` | [0004 分层与依赖](design/0004-架构分层与依赖规则.md) | **已完成（2026-08-27）**：固定 Compiler、Harness、Owner Apply、Kafka Consumer/Projection 的单向依赖与独立测试目录 | 仅解锁 `SG-D08`；不新建通用空层 |
| `SG-D08` | [0009 已验收 MVP 纵向切片](design/0009-剧本到分镜MVP垂直切片设计.md) | **已完成（2026-08-27）**：只增加 StoryGraph/视觉资产/Harness/Kafka-ELK 演进与新旧证据隔离说明 | 仅解锁 `SG-D09`；历史 `0009` Plan/Acceptance 不得抵扣新验收 |
| `SG-D09` | [2001 Backend 服务架构](design/2001-后端服务架构.md) | **已完成（2026-08-27）**：固定单 Backend/GORM Catalog、Temporal、私有 Agent、Kafka/Search/ELK 目标与当前缺口 | 仅解锁 `SG-D10`；不引入 Migration、Raw SQL、第二 ORM 或第二 Writer |
| `SG-D10` | [2002 Backend 领域设计](design/2002-后端领域模块功能设计.md) | **已完成（2026-08-27）**：固定 StoryGraph Compiler、Candidate→Owner Apply、Asset/Specification/State、三类 Binding 与 Kafka/Search 领域边界 | 仅解锁 `SG-D11`；不开始 Harness 或 Canvas |
| `SG-D11` | [3001 Production Bible](design/3001-项目制作圣经生成执行框架设计.md) | **已完成（2026-08-27）**：改为 Evidence/Claim/Specification/State 的 StoryGraph 上游，拆开 Bible Confirm 与资产物化 | 仅解锁 `SG-D12`；旧 `3001` 派生文档继续冻结 |
| `SG-D12` | [3002 本地 Codex 分镜 Harness](design/3002-本地-Codex-分镜智能体执行框架设计.md) | **已完成（2026-08-27）**：收口到 `3003` 唯一 Bundle，固定 Draft `needs_asset`、付费前 Gate 与 Detail 精确资产边界 | 仅解锁 `SG-D13`；旧 `3002` 派生文档继续冻结 |
| `SG-D13` | [2051 图片 Provider](design/2051-Runware图片Provider与Generation执行器设计.md) | **已完成（2026-08-27）**：固定 `reference_asset`/`shot_frame`、composite reference sheet、Runware unknown 对账与精确 AssetVersion 绑定 | 仅解锁 `SG-D14`；不保留 shot-only 兼容入口 |
| `SG-D14` | [1001 前端应用架构](design/1001-前端应用架构.md) | **已完成（2026-08-27）**：固定单 npm/Next.js 应用、RTK Query 单一 Owner、只读 Story/Workflow Lens 与 React Flow/Dagre 边界 | 仅解锁 `SG-D15`；不预建协作或可写 Canvas |
| `SG-D15` | [1002 前端功能模块](design/1002-前端功能模块设计.md) | 在 `1001/2002` 之后对齐角色卡、地点卡、审核与 Canvas 任务流 | 不模拟 Backend 成功 |
| `SG-D16` | [2055 公共 Human Gate](design/2055-Workflow公共HumanGate命令与恢复设计.md) | 在 `2002/1002` 同步后对齐新的 Bible/Episode/Storyboard Gate 和恢复语义，并单独接受 | 未接受前不实现新 Gate HTTP 闭环 |
| `SG-D17` | `0010` PRD | 所有受影响 Design 已同步和接受后，单独固定用户价值、MVP 范围与非目标 | 不创建第二份 Agent 产品愿景 |
| `SG-D18` | `0010` 跨服务 Requirement | 固定 Backend/Frontend/Workflow/Asset 可测契约 | 不编码 |
| `SG-D19` | `3003` Agent Contract Requirement | 仅补 Bundle/Stage/Shard/Candidate/Codex 专项契约 | 不重复 `0010` 产品范围 |
| `SG-D20` | `0010` 唯一总 Plan | 引用 `SG-Ixx` 任务与 `3003` Agent 子项，Checklist 全为 `[ ]` | 不继续从旧 `3001/3002/0007` Plan 领取 StoryGraph 任务 |
| `SG-D21` | 新 Acceptance Criteria | 逐项映射 Requirement/`SG-Ixx`，初始全为 `[ ]` | 无当次真实证据不得勾选 |

`SG-D01`–`SG-D14` 已于 2026-08-27 依次接受并完成，`SG-D15` 是当前唯一激活步骤。`plan/0007`、`plan/0008` 只表达未来 Platform Complete 目标，不是当前 StoryGraph 执行入口；旧 `3001/3002` PRD、Requirement、Plan 与 Acceptance 持续冻结，由 `SG-D17`–`SG-D21` 统一文档链取代。在 `SG-D20/SG-D21` 通过前，`0007/0008/1001/2002/3001/3002` 旧 Plan 中与 StoryGraph、Canvas、新 Human Gate、Agent Bundle 或视觉资产重叠的 Checklist 一律冻结，`2051/2055` 也不得绕过 `SG-D13/SG-D16` 进入编码。唯一代码实施顺序由 `0010` 的 `SG-Ixx` 维护，`3003` 只做 Agent 子任务映射。

## 编号与命名

文件统一使用 `NNNN-中文业务主题.md`。四位编号由服务边界决定：

| 编号段 | 目标边界 | 负责内容 |
|---|---|---|
| `0001–0999` | 系统级、产品级、跨服务 | 平台产品、架构、资源所有权与总交付计划 |
| `1000–1999` | Frontend | Web、Canvas、AI UI 与 Collaboration |
| `2000–2999` | Backend | Go 业务服务、运行程序、契约、数据与基础设施 |
| `3000–3999` | Production Intelligence / Agent | Production Bible、Storyboard 与受限 Agent Harness |

- 同一业务主题跨目录时复用同一编号，例如 `3002` 同时关联 Requirement、Design、Plan 与 Acceptance。
- 一个目录内同一编号最多对应一份文档；编号分配后不重排、不复用。
- 编号表示服务归属和推荐阅读顺序，不表示优先级、版本或完成状态。
- Compose 运行角色、数据库、中间件和 Worker 不单独占用服务编号；它们归入目标 Owner 或跨服务设计。
- 架构决策直接作为 Design 保存；被取代时在正文记录替代关系，历史版本由 Git 追溯。

## 状态词

| 状态 | 含义 |
|---|---|
| 已接受目标 | 产品或设计已经确认，但不代表代码完成 |
| 待独立评审 | 从已接受 Design 派生的 PRD/Requirement/Plan/Acceptance 尚未单独接受 |
| 待实施 | 目标契约已定义，执行 Checklist 尚未完成 |
| 待验收 | 实施可能存在，但尚无按本设计重新执行的完整证据 |
| 已验收 | Acceptance 记录了对应范围的真实输入、命令和结果 |
| 已冻结 | 文档只保留为当前事实或历史方案输入；必须等唯一队列的指定步骤激活，不能直接评审、派生或编码 |
| 历史记录 | 只保存旧实现事实，不进入 0→1 完成判断 |
| 已取代 | 仅用于历史追溯，不再指导新实现 |

“Design 已接受”不能写成“功能已实现”，“Plan 已完成”也不能替代 Acceptance。只有目标 Acceptance 才能形成新设计的完成证据。

## 文档集索引

`—` 表示该主题当前不需要对应类型，不创建占位文件。

| 编号 | 主题 | PRD | Design | Requirement | Plan | Acceptance | 当前状态 |
|---|---|---|---|---|---|---|---|
| `0001` | 平台产品与完整设计基线 | [产品范围与验收基线](prd/0001-产品范围与验收基线.md) | [完整设计基线](design/0001-AI短剧制作平台完整设计基线.md) | [平台 V1 需求规格](requirement/0001-平台V1需求规格.md) | [见 0007 交付计划](plan/0007-平台0到1交付计划.md) | — | `SG-D04` 平台 StoryGraph/Kafka/ELK 主干同步完成；旧 Requirement/Plan 的 StoryGraph 重叠项冻结至 `SG-D17`–`SG-D21` |
| `0002` | 采用目标平台架构 | — | [架构决策](design/0002-采用目标平台架构决策.md) | — | — | — | 已接受目标 |
| `0003` | 系统总体架构 | — | [总体架构](design/0003-系统总体架构.md) | — | — | — | 已接受目标；`SG-D06` StoryGraph/四图架构同步完成 |
| `0004` | 架构分层与依赖 | — | [分层规则](design/0004-架构分层与依赖规则.md) | — | — | — | 已接受目标；`SG-D07` StoryGraph 依赖方向复核完成 |
| `0005` | 中文语义化文档与模块命名 | — | [命名决策](design/0005-采用中文语义化文档与模块命名决策.md) | — | — | — | 已接受目标 |
| `0006` | 领域语言与模块命名 | — | [命名规范](design/0006-领域语言与模块命名规范.md) | — | — | — | 已接受目标；`SG-D03` StoryGraph 术语同步完成 |
| `0007` | 平台 0→1 交付 | — | — | — | [交付计划](plan/0007-平台0到1交付计划.md) | — | 未来 Platform Complete 目标；当前不是 StoryGraph 执行入口 |
| `0008` | 资源所有权与交付 | — | — | — | [所有权台账](plan/0008-资源所有权与交付台账.md) | — | 未来 Platform Complete 资源目标；当前不是 StoryGraph 执行入口 |
| `0009` | 剧本到分镜 MVP 垂直切片 | [产品需求](prd/0009-剧本到分镜MVP产品需求.md) | [垂直切片设计](design/0009-剧本到分镜MVP垂直切片设计.md) | [需求规格](requirement/0009-剧本到分镜MVP需求规格.md) | [实施计划](plan/0009-剧本到分镜MVP实施计划.md) | [验收记录](acceptance/0009-剧本到分镜MVP验收记录.md) | 历史 MVP 已验收；`SG-D08` 演进说明已加入，不回写旧证据 |
| `0010` | StoryGraph 内容图、DAG 与视觉资产 | — | [内容图与 DAG 创作画布设计](design/0010-StoryGraph内容图与DAG创作画布设计.md) | — | — | — | Design 已接受（`SG-D01`）；尚未派生或实施 |
| `1001` | 前端应用架构与交付 | — | [应用架构](design/1001-前端应用架构.md) | [架构需求规格](requirement/1001-前端应用架构需求规格.md) | [应用与功能交付计划](plan/1001-前端应用与功能交付实施计划.md) | — | `SG-D14` 已完成单应用、只读双 Lens 与 Query Owner 同步；旧派生文档的 StoryGraph 重叠项冻结至 `SG-D17`–`SG-D21` |
| `1002` | 前端创作工作台与功能模块 | [创作工作台产品需求](prd/1002-前端创作工作台产品需求.md) | [模块设计](design/1002-前端功能模块设计.md) | [功能需求规格](requirement/1002-前端功能模块需求规格.md) | [合并至 1001 计划](plan/1001-前端应用与功能交付实施计划.md) | — | `SG-D15` 只同步 Design；旧派生文档的 StoryGraph 重叠项冻结至 `SG-D17`–`SG-D21` |
| `2001` | 后端服务与运行架构 | — | [服务架构](design/2001-后端服务架构.md) | [运行架构需求规格](requirement/2001-后端运行架构需求规格.md) | [运行架构实施计划](plan/2001-后端运行架构实施计划.md) | [MVP 全链验收](acceptance/0009-剧本到分镜MVP验收记录.md) | `SG-D09` 服务架构复核完成；旧 Requirement/Plan 的 StoryGraph 重叠项冻结 |
| `2002` | 后端领域服务与生产闭环 | — | [模块设计](design/2002-后端领域模块功能设计.md) | [领域服务需求规格](requirement/2002-后端领域服务与生产闭环需求规格.md) | [生产闭环实施计划](plan/2002-后端领域服务与生产闭环实施计划.md) | [持久任务恢复](acceptance/2007-Workflow持久任务恢复验收记录.md) · [编译输入前置](acceptance/2008-Workflow编译输入前置验收记录.md) · [确定性编译](acceptance/2009-Workflow确定性编译验收记录.md) · [启动与对账](acceptance/2010-Workflow启动事实与Temporal对账验收记录.md) · [人工信号协调](acceptance/2011-Workflow人工信号协调验收记录.md) · [取消控制协调](acceptance/2012-Workflow取消控制协调验收记录.md) · [人工任务续租与释放](acceptance/2013-Workflow人工任务续租与释放验收记录.md) · [人工任务过期回收](acceptance/2014-Workflow人工任务过期回收验收记录.md) · [暂停与恢复控制](acceptance/2015-Workflow暂停与恢复控制协调验收记录.md) · [Worker 重启恢复](acceptance/2016-Workflow工作者重启恢复验收记录.md) · [Node Cache 确定性事实](acceptance/2017-Workflow节点缓存确定性事实验收记录.md) · [Node 输出绑定](acceptance/2018-Workflow节点输出绑定验收记录.md) · [Node 输入冻结](acceptance/2019-Workflow节点输入冻结验收记录.md) · [Node Runtime Cache](acceptance/2020-Workflow节点运行缓存验收记录.md) · [Human Gate 输入与决议绑定](acceptance/2021-Workflow人工栅栏输入与决议绑定验收记录.md) · [Production Bible Owner Receipt](acceptance/2022-ProductionBible确认回执验收记录.md) · [Workflow Owner Receipt 与 Gate 输出](acceptance/2023-Workflow生产回执与人工栅栏输出验收记录.md) · [Workflow 执行身份与 Script Executor](acceptance/2024-Workflow执行身份与剧本输入节点验收记录.md) · [重复投递收敛](acceptance/2052-Workflow重复投递收敛验收记录.md) · [Agent 执行策略与独立失败](acceptance/2053-Agent执行策略与独立失败验收记录.md) · [Agent 执行总时限](acceptance/2054-Agent执行总时限验收记录.md) · [阶段 5 完成度审计](acceptance/2056-Workflow阶段5完成度审计.md) | `SG-D10` 领域 Owner 复核完成；历史验收只证明既有切片，不抵扣 StoryGraph 新链路 |
| `2003` | 后端语言与运行边界 | — | [运行边界策略](design/2003-后端语言与运行边界策略.md) | — | — | — | 已接受目标；`SG-D05` StoryGraph/Agent 运行边界复核完成 |
| `2051` | Runware 图片 Provider 与 Generation 执行器 | — | [图片 Provider 与执行器设计](design/2051-Runware图片Provider与Generation执行器设计.md) | — | — | — | `SG-D13` 已完成视觉资产/Shot Frame 双 Target 同步；等待统一派生文档 |
| `2055` | Workflow 公共 Human Gate 命令与恢复 | — | [公共 Human Gate 设计](design/2055-Workflow公共HumanGate命令与恢复设计.md) | — | — | — | 已冻结；仅在 `SG-D16` 激活后同步并单独评审 |
| `2056` | Workflow 阶段 5 完成度审计 | — | — | — | — | [完成度审计](acceptance/2056-Workflow阶段5完成度审计.md) | 静态证据审计完成，未改变阶段完成状态 |
| `3001` | 项目制作圣经与完整剧本闭环 | [产品需求](prd/3001-项目制作圣经产品需求.md) | [执行框架设计](design/3001-项目制作圣经生成执行框架设计.md) | [需求规格](requirement/3001-项目制作圣经需求规格.md) | [实施计划](plan/3001-项目制作圣经实施计划.md) | [验收 Checklist](acceptance/3001-完整剧本业务闭环验收标准.md) | `SG-D11` Design 已完成 StoryGraph 上游同步；旧派生文档持续冻结，由 `SG-D17`–`SG-D21` 取代 |
| `3002` | 本地 Codex 分镜智能体 | [产品需求](prd/3002-本地-Codex-分镜智能体产品需求.md) | [执行框架设计](design/3002-本地-Codex-分镜智能体执行框架设计.md) | [需求规格](requirement/3002-本地-Codex-分镜智能体执行框架需求规格.md) | [实施计划](plan/3002-本地-Codex-分镜智能体执行框架实施计划.md) | [验收 Checklist](acceptance/3002-本地-Codex-分镜智能体执行框架验收标准.md) | `SG-D12` Design 已完成唯一 Bundle 与 Draft/Detail 同步；旧派生文档持续冻结，由 `SG-D17`–`SG-D21` 取代 |
| `3003` | StoryGraph 剧本解析 Harness 与内置 Skill | — | [Harness 与内置 Skill 设计](design/3003-StoryGraph剧本解析Harness与内置Skill设计.md) | — | — | — | Design 已接受（`SG-D02`）；尚未派生或实施 |

`2002` 的最新边界审计：[Workflow 阶段 5 完成度审计](acceptance/2056-Workflow阶段5完成度审计.md)；最新运行增量验收为 [Agent 执行总时限](acceptance/2054-Agent执行总时限验收记录.md)，前置边界见 [Workflow 重复投递收敛](acceptance/2052-Workflow重复投递收敛验收记录.md)、[Shot 绑定目标与单 Shot 局部重跑](acceptance/2050-Shot绑定目标与单Shot局部重跑验收记录.md)、[正式 Shot Workflow 后半程](acceptance/2049-正式ShotWorkflow后半程验收记录.md)、[Production Shot 图片绑定](acceptance/2048-ProductionShot图片绑定验收记录.md)、[Generation CandidateSet 与 Workflow 人工选择](acceptance/2047-GenerationCandidateSet与Workflow人工选择验收记录.md)、[Generation Provider 成功输出物化](acceptance/2046-GenerationProvider成功输出物化验收记录.md)、[Generation Provider 提交与结果对账](acceptance/2045-GenerationProvider提交与结果对账验收记录.md)、[Generation 高成本准备与执行授权](acceptance/2044-Generation高成本准备与执行授权验收记录.md)、[Cost 费用预留与追加式账本](acceptance/2043-Cost费用预留与追加式账本验收记录.md)、[Cost 图片价格与不可变估算](acceptance/2042-Cost图片价格与不可变估算验收记录.md)、[Cost Project Budget 唯一事实](acceptance/2041-Cost项目预算唯一事实验收记录.md)、[Quota 图片生成日配额](acceptance/2040-Quota图片生成日配额验收记录.md)、[Generation 人工候选选择](acceptance/2039-Generation人工候选选择验收记录.md)、[Generation 图片候选与确定性 QC](acceptance/2038-Generation图片候选与确定性QC验收记录.md) 与 [Asset 图片产物就绪](acceptance/2037-Asset图片产物就绪验收记录.md)。

## 历史实现记录

下列文件只保存旧实现的验证事实，可作为新 Design 识别当前事实和缺口的输入，但不能决定新目标边界，也不能抵扣上表任何 Plan 或 Acceptance：

| 编号 | 记录 | 适用性 |
|---|---|---|
| `2004` | [后端运行边界与事件契约验收记录](acceptance/2004-后端运行边界与事件契约验收记录.md) | 历史记录，不计入 0→1 完成判断 |
| `2005` | [数据库基线与兼容窗口验收记录](acceptance/2005-数据库基线与兼容窗口验收记录.md) | 历史记录，不计入 0→1 完成判断 |
| `2006` | [单入口启动与对象存储上传验收记录](acceptance/2006-单入口启动与对象存储上传验收记录.md) | 历史记录，不计入 0→1 完成判断 |

## 事实优先级与维护

1. 已接受的 Design 决定问题、方案、Owner 和技术边界；随后派生的 PRD 决定该边界内的产品范围，不得反向隐式改写 Design。
2. Requirement 决定可测试契约；既有代码只用于识别缺口，不能让新条款预先变成已满足。
3. Plan 只描述从当前事实到目标设计的实施依赖与门禁，初始 Checklist 全部为 `[ ]`。
4. 目标 Acceptance 只证明按新设计重新执行且明确记录的范围；历史记录不得抵扣目标 Checklist。
5. 新增、改名、取代文档时必须同步更新本索引和相对链接，并检查目标状态、实施状态与历史证据是否被隔离。
6. 不建立全局 `misc`、重复分类、空占位文档或第二套事实来源。
