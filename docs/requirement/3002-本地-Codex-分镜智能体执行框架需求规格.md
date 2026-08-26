# 本地 Codex 分镜智能体执行框架需求规格

- 状态：旧派生文档已冻结；由 `SG-D17`–`SG-D21` 的统一链路替代
- 日期：2026-08-25
- 产品来源：[本地 Codex 分镜智能体产品需求](../prd/3002-本地-Codex-分镜智能体产品需求.md)
- 设计来源：[本地 Codex 分镜智能体执行框架设计](../design/3002-本地-Codex-分镜智能体执行框架设计.md)

## 目的

本规格定义从确认剧本到可审核关键分镜候选的稳定契约。需求完成与否只由独立的
[验收标准](../acceptance/3002-本地-Codex-分镜智能体执行框架验收标准.md)判定，本文件不声明实施结果。

## 术语

- `fixed input`：一个 Draft batch 冻结的脚本、叙事单元、Bible、资产、世界观和生成参数。
- `scene context`：按确认场景划分的来源单元、资产和确定性时长预算。
- `hard gate`：由应用 Tool 执行、模型不得覆盖的确定性校验。
- `review issue`：Reviewer 或 Tool 产生的带来源、作用域和严重度的问题。
- `candidate`：尚未写为正式 Shot、必须由人工审核的分镜结果。
- `checkpoint`：绑定 batch、task、input hash、Harness 版本和 run token 的阶段快照。

## 功能需求

### 输入与分阶段执行

| ID | 需求 | 可测试条件 |
|---|---|---|
| `SB-FR-001` | `StoryboardDraftInput` 必须冻结确认脚本版本、叙事单元位置、required coverage、场景/对白 ID、目标时长、画幅、视觉风格、Bible snapshot、occurrence 资产状态和相关 world entry。 | 输入哈希覆盖全部固定字段；Bible-bound 请求无需调用方手工传资产；snapshot 或 occurrence 变化使旧 batch 冲突。 |
| `SB-FR-002` | 系统必须按确认场景建立 context，并确定性分配场景时长预算。 | 每个叙事单元只属于其确认场景；occurrence 资产只进入相关场景；各场景预算之和等于总目标时长。 |
| `SB-FR-003` | 执行链必须为 source analysis → scene plan → shot draft → hard gate → review → targeted repair → final gate。 | analysis、plan、draft 对多场景可并行；每阶段只返回严格 schema；最终按场景顺序组装。 |
| `SB-FR-004` | source analysis 和 scene plan 必须覆盖全部 required unit，并且只能引用所属场景的 position 和上阶段 key。 | 缺失 required position、未知 position、错误 scene key 或未知 beat key 立即失败，不进入下一阶段。 |
| `SB-FR-005` | shot hard gate 必须检查来源/场景/对白引用、required coverage、时长、基础空间连续性和资产位置。 | 跨场景引用、未知 dialogue、缺失 required unit、场景时长超出确定性预算 ±25%、可证明的左右侧突变或未知 asset position 均成为 blocker。 |
| `SB-FR-006` | 来源逐字明确提到固定资产时，至少一个承载该来源位置的镜头必须绑定该资产。 | 未绑定形成 Tool blocker；只在其他镜头绑定不能通过；Reviewer 不能自行制造资产 blocker。 |
| `SB-FR-007` | 系统必须确定性生成全局镜号、连续 timecode、总时长和 result hash。 | 场景模型的局部位置不直接成为全局镜号；timecode 从 `0` 连续累加；相同规范化候选产生相同 hash。 |

### Review、修复与人工应用

| ID | 需求 | 可测试条件 |
|---|---|---|
| `SB-FR-008` | Reviewer 必须独立检查 purpose、反应、揭示、可拍性和工艺质量；应用必须归一化 issue provenance、scope 和严重度。 | Reviewer 伪造的 `source=tool` 被改写；未知 scene/shot scope 变为 `review.scope_invalid` warning；无 Tool 证据的 asset blocker 降为 warning。 |
| `SB-FR-009` | blocker 只能定向修复受影响场景，总修复轮次最多两轮，每轮后重新执行 hard gate。 | 未受影响场景的候选保持不变；两轮仍有 blocker 时返回失败且不暴露半成品 candidate。 |
| `SB-FR-010` | 成功运行只能返回 `needs_review` candidate，并携带 timeline、issue、risk code、input/result hash 和 Skill 版本。 | Provider/Worker 不创建正式 Shot；warning 的稳定 code 写入对应 Shot candidate，完整 evidence 保留在结果/checkpoint。 |
| `SB-FR-011` | 用户必须完成逐项决策、batch 审批、影响预检和原子 apply，才能创建正式 Shot。 | 未决候选不能审批；baseline 或 impact hash 变化拒绝应用且无半写入；命令与决议幂等、追加记录。 |
| `SB-FR-012` | 系统必须导出确定性、自描述的关键分镜交付包。 | ZIP 至少包含 manifest、CSV、HTML 和 JSON；包含时间码、镜头语言、动作、对白/声音、连续性、首/关键/尾帧、来源和资产；HTML 转义不可信文本。 |

### Checkpoint、恢复与取消

| ID | 需求 | 可测试条件 |
|---|---|---|
| `SB-FR-013` | checkpoint 必须绑定 batch、task、input hash、Harness 版本、run token、阶段和阶段 payload。 | 损坏、旧版、跨 batch/task/input 或 forged token 的 checkpoint fail closed；恢复只重跑未完成阶段。 |
| `SB-FR-014` | terminal checkpoint 必须重算并验证 candidate hash、timeline、总时长、issue 注解和 terminal status。 | 任一字段不一致时不得直接恢复 terminal candidate，必须重新运行或显式失败。 |
| `SB-FR-015` | 用户必须能显式取消 queued/running Draft batch；Provider 不确定时进入 unknown。 | cancel 会 fencing token 并终止本地 Codex；取消或 unknown 后不写 Draft/Shot；重复取消幂等。 |
| `SB-FR-016` | 对活跃运行的消息重投只能 requeue；lease 过期后才允许新 token 从 checkpoint 恢复。 | 同一 batch 不并行执行两个 Codex；旧 token 不能保存 checkpoint 或提交结果。 |

### 连续性质量演进

| ID | 需求 | 可测试条件 |
|---|---|---|
| `SB-FR-017` | 基础 side continuity 之外的复杂轴线、视线、动作匹配和音画桥，只有在确定性规则与验收基线被接受后才能成为 hard gate。 | 未实现规则保持 Reviewer/人工风险，不得在 Acceptance 中标为确定性通过；新增规则先有独立失败测试和误报基线。 |

## 非功能需求

| ID | 需求 | 可测试条件 |
|---|---|---|
| `SB-NFR-001` | 本地 Codex 必须使用 ephemeral、read-only sandbox、JSON output schema 和最多两次结构校正。 | 命令不授予文件/数据库写入能力；两次仍不合法时返回明确失败；取消会终止子进程。 |
| `SB-NFR-002` | 运行使用 5 分钟可续租 lease 和 heartbeat，但不得把 lease 时长当成业务执行预算。 | 心跳正常可运行数小时；短暂续租失败在有效期内重试；实际过期或 fencing 才取消本地进程。 |
| `SB-NFR-003` | 未显式配置 model 或推理强度时必须继承本机 Codex 配置，且所有剧本与分镜阶段不设置固定墙钟超时。 | CLI 命令只在配置存在时添加 model；不添加隐式低推理强度或任务时长参数。 |
| `SB-NFR-004` | 项目自有 Skill 名称固定为 `analyze-scene`、`plan-scene`、`draft-shots`、`review-shots` 和 `repair-shots`。 | 目录、frontmatter、显式调用提示和测试一致；禁止旧别名、隐式调用、缺失 reference 或空 Agent 占位目录。 |
| `SB-NFR-005` | 运行时不得依赖 DeepSeek、外部 Skill 仓库、上游 checkout、submodule 或 Provider registry。 | 运行配置不需要相关密钥或 base URL；上游链接只用于设计追溯。 |
| `SB-NFR-006` | 所有候选、checkpoint、人工决议和导出必须可追溯到固定输入和版本。 | 公开结果包含必要 hash/版本；私有 run token 不泄漏；重复投递和重启不改变已固定导出。 |

## 业务验收需求

| ID | 需求 | 可测试条件 |
|---|---|---|
| `SB-AC-001` | 使用已登录的真实本机 Codex 完成多场景分镜合约。 | 至少两个确认场景，包含动作、对白、跨空间动作和来源明确的关键道具；不使用模型桩。 |
| `SB-AC-002` | 使用真实 confirmed Bible/occurrence/world context 完成从 Draft request 到人工 apply 和导出的端到端合约。 | 调用方不手工传 Bible 资产；镜头引用正确状态；world context 可见；snapshot 漂移被拒绝。 |
| `SB-AC-003` | 使用完整 60 集原稿执行机器覆盖统计和代表集人工细查。 | 60 集均有镜头数、required coverage、引用、时长、issue、审核和恢复结果；E01/E07/E09/E31/E36/E60 不替代全量统计。 |

## 命名与模块边界

- 通用 Skill 使用 2–3 个英文单词的“动作 + 对象”短名，标识与声明名称完全一致。
- 分镜 Harness、阶段 schema、确定性门禁和 checkpoint 归属分镜业务域。
- Skill 执行机制只负责受约束的结构化调用，不拥有分镜业务状态。
- 禁止万能 `AgentService`、跨域业务 `ToolRegistry`、空 Agent 占位模块和旧名称兼容入口。

## 状态与输出契约

- 运行中间态由 checkpoint stage 表达，不建立第二套 Markdown 或通用 Agent 状态表。
- Provider 成功结果为 `needs_review`；硬门或有限修复失败不返回半成品。
- Provider 结果不确定为 `unknown`；用户取消为 `cancelled`。
- 正式 Shot 只在人工审批和 apply 后产生。

## 非目标

- 本主题不实现图片、视频、声音生成。
- 不原样运行上游文件状态机、Dashboard、适配器或脚本。
- 不让 Codex 执行 Bash、修改项目、写数据库或自动批准候选。

## 追踪

- [实施计划](../plan/3002-本地-Codex-分镜智能体执行框架实施计划.md)
- [验收 Checklist](../acceptance/3002-本地-Codex-分镜智能体执行框架验收标准.md)
