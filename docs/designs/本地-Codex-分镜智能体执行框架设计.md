# 本地 Codex 分镜智能体执行框架设计

状态：Accepted / Implemented for MVP
日期：2026-08-24

## 架构决策

采用“领域 Agent Harness + 多个通用 Skill + 确定性 Tool”。分镜业务编排、阶段 schema、门禁与 checkpoint 归属 `storyboards/agents`；`modules/skills` 只提供受约束的结构化 Skill 执行机制。Codex 是本机模型执行器，不拥有领域状态。

```text
.agents/skills/
  analyze-scene/
  plan-scene/
  draft-shots/
  review-shots/
  repair-shots/
backend/app/
  integrations/codex_local.py
  modules/storyboards/agents/{harness,schemas,tools,checkpoints}.py
  runtime/workers/io.py
```

## 命名规范

- Skill 使用 2–3 个英文单词组成的 `lowercase-kebab-case` 动词短语，优先采用“动作 + 对象”，不带产品前缀，也不使用脱离上下文后无法理解的首字母缩写。
- Skill 目录名必须与 `SKILL.md` frontmatter 的 `name` 完全一致；`agents/openai.yaml` 的默认提示必须使用同一个 `$skill-name`。
- Skill 固定入口保留 Codex 约定名 `SKILL.md` 和 `agents/openai.yaml`。reference 文件使用 2–3 个名词或限定词组成的 `lowercase-kebab-case` 语义名，例如 `review-rubric.md`、`shot-table-rules.md`。
- Python 模块使用 `lower_snake_case`，测试文件使用 `test_<module>.py`。文件名保留必要的领域词，不为追求字符数引入 `sba`、`ssr` 等隐晦缩写。
- 运行时 Skill 标识、提示中的显式调用、测试断言和文档目录树必须同步修改，禁止保留旧名称兼容别名，避免形成两套可调用入口。

Storyboard Skill 的稳定名称为 `analyze-scene`、`plan-scene`、`draft-shots`、`review-shots` 和 `repair-shots`。

当前不创建 `assets/agents`、`media/tools`、`production/tools` 或其他未来占位目录；只有具体能力的需求与 Design 被接受后才建立对应目录。架构测试禁止顶层 `modules/agents` 和空 Agent 包。

## 执行流

```text
fixed input
 → build scene contexts and duration budgets
 → source analysis (parallel per scene)
 → scene plan (parallel per scene)
 → shot draft (parallel per scene)
 → deterministic hard gates
 → assemble global shot numbers/timecodes and initial hash
 → independent review and policy normalization
 → at most 2 targeted repairs, hard gates and reassembly
 → final gate
 → warning risk codes + final result hash
 → candidate requiring human review
```

各模型阶段只返回 Pydantic JSON Schema 值。Skill 均为 candidate-only，allowed tools 为空。模型只选择注入的整数位置，应用层再映射 UUID 和现有 `ShotSpec`。

## 确定性门禁

Python Tool 拥有以下硬约束：

- 来源位置存在且属于当前场景，对白引用类型正确；
- required narrative unit 至少覆盖一次；
- 场景镜头总时长在确定性预算 ±25% 内；
- 人物左右侧突变必须有可见过渡说明；
- 资产绑定位置存在；来源逐字明确提到固定资产时，至少一个承载该来源位置的镜头必须绑定它；
- 全局镜号、时间码和结果哈希确定性生成。

Reviewer 负责 purpose、反应、揭示、可拍性和工艺质量。其输出先归一化：伪造的 `source=tool` 被改为 reviewer；未知 scene/shot scope 替换为 `review.scope_invalid` warning；Reviewer 资产 blocker 降为 warning。真正资产 blocker 只能由 Tool 从固定输入与结构化绑定证明。Warning 写入目标 Shot 的 `risk_codes`，完整证据保留在 result/checkpoint。

## Checkpoint 与恢复

checkpoint 存在 storyboard draft batch JSONB 字段中，不建立通用 Agent 状态表。读写使用独立事务和行锁，并校验 batch、task、input hash、harness version、run token 与 schema。

每次运行原子领取一个 5 分钟 lease 和唯一 run token，运行期间每分钟续租。这个 5 分钟只限定单个 worker 的所有权有效期，不是任务预算或执行截止时间；心跳正常时可以连续续租数小时。活跃 lease 的 Kafka 重投只 requeue；lease 过期后新 worker 才能轮换 token 并从旧 checkpoint 恢复。Checkpoint save 与最终领域提交都校验当前 token，旧 worker 即使稍后返回也会被 fencing。终态 checkpoint 还会重算 candidate hash，并验证 timeline、总时长、镜头内容和 terminal status 一致。未知 Provider 结果保持 unknown，避免重复提交。

## 本地 Codex 边界

每个阶段通过已安装 Codex CLI 执行，使用 ephemeral、read-only sandbox、JSON output schema 和最多两次结构修正。剧本解析与分镜阶段不设置固定墙钟超时，允许完整运行持续数小时；生命周期由任务取消、可续租 lease、阶段 checkpoint 与 fencing 管理。启动前只验证项目自有的五个 Skill、显式调用策略和三个 reference 文件。运行时无 DeepSeek key/base URL、外部 Skill 仓库或 AI provider/model 路由 registry；SQLAlchemy 模型注册仍由应用运行时负责，领域写入仍由 Draft service 在人工审核后完成。

## 上游整合

- [`shuohao-skills@0e5eb688`](https://github.com/eternityspring/shuohao-skills/tree/0e5eb688ebf1b45e45c9bec31543aaa59e67c7bc) 仅作为已审阅的研究来源；有用的来源追溯、时长、反应/插入镜头、动作衔接、首尾帧、声画一致性和资产参考纪律，已用项目原创措辞分别内化进五个 Skill 及其 reference，不复制或运行上游脚本、文件状态机和输出契约。
- [`drama-skills@7811065`](https://github.com/worldwonderer/drama-skills/tree/7811065c171f8b0a83230bb2e0ccfe2c2b5b337a) 同样只作为设计研究来源；Shot purpose、blocking、连续性、关键帧与证据化 review 已映射到当前 schema、Tool gate 和 Reviewer rubric，本轮不运行其脚本。
- 项目不包含、下载或在启动时校验任何上游仓库；公开链接与 commit 仅用于设计追溯。
- 当前项目的 schema、coverage、资产 readiness、人工决策与原子应用始终优先。

## 图片能力的未来边界

场景图、人物三视图和资产图不会扩展当前 Harness 为万能 Agent。需求被接受后由 `assets/agents` 做资产分析、规划与审核，通过 `assets` contract、`production` 生成 tool 和 `media` contract 协作。Storyboard Harness 只消费已确认资产版本，可以提出 warning，但不生成、存储或批准图片。

## 失败路径

- 项目自有 Skill/schema/reference 缺失：启动失败。
- Codex 不可用、进程失联或取消结果不确定：返回 unknown，并保留可验证 checkpoint。
- 结构错误：同阶段最多一次完整 JSON 修正，仍失败则 Provider failed。
- 硬门失败：定向修复，最多两轮；仍失败则无候选。
- Reviewer scope/资产幻觉：保留 warning，不误触发全局修复。
- checkpoint 身份或版本不匹配：拒绝跨任务恢复。
- lease 活跃：重投 requeue；lease 丢失：取消本地 Codex 子进程，旧 token 禁止 checkpoint/结果写入。
