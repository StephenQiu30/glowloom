# 本地 Codex 分镜智能体执行框架需求规格

状态：Storyboard Harness 已实现；整剧业务闭环正在真实原稿验收
日期：2026-08-24

## 目标

分镜拆分必须作为完整 Agent Harness，而不是单个 Skill。系统直接使用本机已登录的 Codex CLI，把确认剧本转换为可审核、可追溯的关键分镜表；不依赖 DeepSeek，不允许模型直接写领域数据。

Harness 必须满足：

- 固定输入为 `StoryboardDraftInput`，包含确认脚本、叙事单元位置、场景/对白 ID、从本集 Bible occurrence 派生的资产状态、相关世界观、目标成片时长、画幅和视觉风格。
- 执行链为 `source-analysis → scene-plan → shot-draft → hard-gates → review → targeted-repair → final-gate`。
- 多场景分析、规划和草拟可并行；最终按场景顺序组装全局镜号与连续时间码。
- 硬门校验来源/场景/对白引用、required coverage、时长、基础空间连续性、资产位置；来源明确提及固定资产时，至少一个承载该来源位置的镜头必须绑定它。
- Reviewer 的来源、作用域与严重度由 Tool 归一化；Reviewer 资产意见不能自行成为 blocker。
- blocker 只定向修复受影响场景，最多两轮；仍失败时不暴露半成品候选。
- 成功只返回 `needs_review` 候选，携带时间线、风险码、issue 证据、input/result hash 与 Skill 版本。
- checkpoint 绑定 batch、task、input hash、harness version 与当前 run token；损坏、旧版或身份不匹配数据 fail closed。
- 长耗时运行必须持有可续租 lease；活跃 lease 的 Kafka 重投只能 requeue，过期 lease 才能由新 run token 恢复，旧 token 不得再保存 checkpoint 或提交结果。
- 剧本解析、全局 Bible 和分镜阶段不得设置固定墙钟超时；完整运行可以持续数小时，只能由显式取消、lease fencing 或不可恢复故障终止，并从最近有效 checkpoint 恢复。这里描述的是运行可靠性，不是按耗时验收业务效果。
- 未显式指定模型或推理强度时继承本机 Codex 配置，不能因此启用默认任务时长预算。
- CSV/HTML 关键分镜表包含时间码、摄影、动作、对白/声音、连续性、首/关键/尾帧、来源、角色和资产。

## 命名与目录

- 新目录和文件名全部使用英文。
- 通用 Skill 使用 2–3 个英文单词的“动作 + 对象”短名且不带产品前缀：`analyze-scene`、`plan-scene`、`draft-shots`、`review-shots`、`repair-shots`。Production Bible 使用 `extract-bible-evidence`、`reconcile-bible`、`review-bible`。
- Skill 目录使用 `lowercase-kebab-case` 并与 frontmatter `name` 一致；Python 模块使用语义完整的 `lower_snake_case`，不得为了缩短名称引入难懂缩写。
- 运行所需能力必须随项目自有 Skill/reference 交付；上游仓库只作公开研究来源，不得成为 submodule、运行时 checkout、Git revision 门禁或隐式文件依赖。
- 当前 Harness 归属迁移期兼容路径 `agent/app/modules/storyboards/agents/`。
- 禁止顶层 `agent/app/modules/agents/`、万能 `AgentService`、跨域业务 `ToolRegistry` 和空 Agent 占位目录。

## 场景图与资产图

本轮不实现图片生成。未来被接受的场景图、人物三视图和道具资产图需求归属 `assets/agents` 编排；资产语义与版本仍属于 `assets`，媒体字节与存储属于 `media`，高成本生成预检、确认、任务和 Provider capability 属于 `production`。本轮不预建这些未来目录。

## 非目标

- 不原样运行上游文件状态机、Dashboard、图片/视频适配器或脚本。
- 不建立 Markdown/JSON 作为数据库外的第二事实来源。
- 不让 Codex 执行 Bash、修改项目、写数据库或自动批准候选。
