# Agent 执行策略与独立失败验收记录

- 状态：当前 Production Bible/Storyboard 本地 Codex 执行器的调用次数超预算、越权 Tool、无效 Schema 与 Runtime 不可用完成门通过；完整 `BE-MOD-008` 尚未完成
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Production Bible 持久等待节点](2026-WorkflowProductionBible持久等待节点验收记录.md) · [多集 Storyboard Draft Set](2031-WorkflowStoryboard多集持久候选验收记录.md)

## 验收范围

本切片只收敛当前两个已投入 Workflow 的 Candidate Runtime 定义：`production_bible` 和 `storyboard_draft`。Backend 为每个 `AgentInvocation` 冻结 Definition、Prompt、Skill Bundle、Output Schema、模型能力、空 Tool Allowlist 和最大模型调用次数；Production Bible 上限为 3 次，Storyboard Draft 上限为 1 次。策略作为 `agt_invocations.execution_policy` JSONB 进入唯一 GORM Model Catalog，未增加 Migration、DDL、Raw SQL、第二 ORM 或第二 SQL 事实源。

Backend 只签发包含执行策略 Hash 的短时 HMAC Grant。Agent Runtime 必须同时验证 Invocation 身份、输入 Hash、策略 Hash 和 TTL；排队中的 Invocation 不会因进程部署版本变化而临时派生另一份策略。已删除没有生产执行路径的 `script_structure` 兼容枚举，不保留静默回退。

本地开发执行器继续使用 Codex CLI 完成文本和结构化 AI 调用，但不让模型访问项目工作区：Harness 先由 Agent 进程读取并注入项目自有 Skill/Reference，再把 Codex 工作目录切换到本次调用的临时隔离目录，启用 ephemeral/read-only/严格 Output Schema，并忽略用户配置。Shell、Unified Exec、Web/Browser、Apps、Plugins、Computer Use、Image、Multi-agent、Skill Search、Workspace Dependencies 等模型可选能力全部显式禁用；CLI JSONL 中一旦出现 `command_execution` 等非允许 Item，结果立即失败关闭。

## 独立失败契约

| 失败 | Agent Result | Backend 语义 | 重试 |
|---|---|---|---|
| 模型调用次数超过冻结上限 | `execution_budget_exceeded` | `failed` | 否 |
| Codex 产生空 Allowlist 之外的 Tool Item | `tool_not_allowed` | `failed` | 否 |
| CLI 返回内容无法通过严格结构 Schema | `candidate_schema_invalid` | `failed` | 否 |
| Codex CLI 无法启动、非零退出或没有可信结果文件 | `runtime_unavailable` | `unknown` | 是，先按原 Invocation 对账 |

Go Result Contract 对上述 Code、Status 与 Retryable 组合再次 fail closed；未知错误码或改变重试语义不能进入业务 Worker。候选通过 Runtime Schema 后仍由 Bible/Storyboard Owner 执行自己的引用、覆盖率和确定性校验，失败继续使用独立的 `candidate_validation_failed`，Agent 不因此获得业务事实写权限。

## Red → Green 证据

1. Red 阶段先增加 Go Contract/Grant 与 Python Grant/Runner/API 测试。Go 明确编译失败于缺少 `ExecutionPolicyFor`、Invocation Policy 字段和策略 Hash；Python 在收集阶段明确失败于缺少 `execution_policy_for` 和四类异常类型。
2. Green 阶段补齐 Backend 固定 Definition Manifest、GORM 持久策略、Grant 策略 Hash、Worker 重验、Agent Pydantic 契约、模型调用计数预算、CLI 隔离与四类 Result 映射；目标 Go 测试和 Python 测试随后转绿。
3. 第一次真实探针没有进入 Codex，失败于临时 `python -c` 把复合 `class` 接在分号后的语法错误，不计为 Runtime 证据；修正探针命令后，初版 Item 检测又把 Codex 自身的 `error` 诊断 Item 误判为 Tool。检测器按真实 JSONL 协议只拒收工具类 Item，并增加回归测试后，同一真实调用返回 `{"verdict":"ok"}`。这两次失败都没有通过放宽 Tool Allowlist 或增加兼容路径解决。

## 真实验收证据

- 本机 Codex CLI `0.149.1` 在隔离目录、空 Tool Allowlist、项目 Skill 注入和严格 Pydantic Schema 下完成一次真实模型调用，用时约 6 秒，返回 `{"verdict":"ok"}`。
- 与 CI 一致的 PostgreSQL `16.15-alpine`、固定摘要 Temporal Server 和私有 MinIO 上，Invocation Lease、Production Bible Workflow 与多集 Storyboard Workflow 定向旅程通过，共 `23.484s`；数据库中的两类 Invocation 均保存并重新验证了正确的执行策略和 3/1 次模型调用上限。
- Backend 在另一个全新空 PostgreSQL 数据库上通过 `gofmt`、`go vet ./...` 和 `go test -count=1 -p 1 ./...`；最终 Workflow 包用时 `109.893s`，真实依赖用例未跳过。
- Agent 使用 Python `3.11.15` 通过 Ruff check/format、Pyright `0 errors` 和 Pytest `22 passed`。测试覆盖 Go/Python 策略 Hash 一致性、Grant 策略 Hash、预算耗尽前置拒绝、工具 Item 拒收、CLI 诊断 Item 非误判、Schema/Runtime 分类和 API Result 语义。
- Frontend 在官方 Node 22 容器中执行 `npm ci`、OpenAPI Client 生成、ESLint、TypeScript、Vitest `16 files / 45 tests` 和 Next.js `16.2.12` 生产构建，全部通过；生成 Client 无漂移。
- 开发/生产 Compose 渲染、仓库卫生、Frontend/Backend/Agent 三类镜像构建和容器入口断言通过。Agent 镜像内固定 Codex CLI 为 `0.147.0`，并实际确认所有 Harness 禁用能力均被该版本识别。

## Requirement 状态与残余风险

- `BE-MOD-008` 仅完成当前两种固定 Definition 的 Invocation 策略快照、Grant 调用次数预算、空 Tool Allowlist、结构化返回与独立失败；Requirement 总项保持未完成。
- 本记录验收时尚未实现 Workflow Run/Node 级 Grant 作用域、独立防重放 Nonce/Receipt、步数/Token/费用/deadline 全维预算、AgentRun/Usage Receipt、Cost/Quota 协调、生产 Model Gateway、Backend Tool Allowlist API、ToolLoop 或 LangGraph Executor；其中 Invocation 总执行时限后来由[独立验收](2054-Agent执行总时限验收记录.md)补齐，但仍不能把调用次数与时限解释为完整 Token/费用预算或绝对排队 deadline。
- Codex CLI 是用户已允许的本地开发文本/结构化调用方式；它不作为图片 Provider、Generation Source、Backend 业务事实 Writer 或生产 Model Gateway 的兼容替身。
- [Runware 图片 Provider 与 Generation 执行器 Design](../design/2051-Runware图片Provider与Generation执行器设计.md)仍待用户接受；本切片没有跨越该 Design 开始图片生成编码。
- 当前本地提交尚未获准推送，因此不声明远端 GitHub Actions 已覆盖本切片。
- `agent-browser` 按约定只在全部开发完成后执行；Workflow、Generation、Media、前端审核和最终成片仍有后续任务，本切片不提前调用。
