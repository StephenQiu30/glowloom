# Agent 执行总时限验收记录

- 状态：当前 Production Bible/Storyboard 本地 Codex Invocation 总执行时限完成门通过；完整 `BE-MOD-008` 尚未完成
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Agent 执行策略与独立失败](2053-Agent执行策略与独立失败验收记录.md)

## 验收范围

本切片只补齐当前两个固定 Agent Definition 的 Invocation 总执行时限。Production Bible 固定为 900 秒，Storyboard Draft 固定为 600 秒；`max_execution_seconds` 与 Definition、Prompt、Skill Bundle、Output Schema、模型能力、空 Tool Allowlist 和模型调用次数一起进入 `AgentInvocation.execution_policy` JSONB，并由既有短时 Execution Grant 的策略 Hash 绑定。没有新增 SQL 表或列、Migration、DDL、Raw SQL、第二 ORM、第二 SQL 事实源或兼容策略。

Python Candidate Runtime 在创建当前 Invocation 的 `CodexSchemaRunner` 时建立单调时钟 deadline。这个 deadline 覆盖该 Runner 的全部模型调用，而不是每次调用重新计时；开始调用前已经耗尽时不启动 Codex，执行中耗尽时立即终止并等待回收子进程，不读取迟到输出，也不把超时伪装成候选成功。Runtime 返回 `failed / execution_deadline_exceeded / retryable=false`，Go Result Contract 对该组合再次 fail closed，避免自动盲重试同一不可寻址的本地模型调用。

本切片不把 Execution Grant 的五分钟签发 TTL 解释为业务执行时限：TTL 只限制请求何时可以开始，Execution Policy 决定已经授权的 Invocation 最长可以执行多久。它也不把本地 Codex CLI 解释为图片 Provider、Backend Writer 或生产 Model Gateway。

## Red → Green 证据

1. Red 阶段先修改 Go Contract 测试，代码明确编译失败于缺少 `MaxExecutionSeconds`；Python 测试收集明确失败于缺少 `CodexDeadlineExceeded`。
2. Green 阶段在 Go/Python 固定 Manifest 中加入 900/600 秒预算，更新跨语言 canonical Policy Hash，并在 Runner 中用同一单调 deadline 包围 `communicate`。模拟永不返回的 Codex 进程在一秒测试预算后被 `kill`，API 和 Go Contract 只接受固定失败语义。
3. 本机真实 Codex CLI 在 Storyboard 的 600 秒策略、隔离临时目录、严格输出 Schema 和空 Tool Allowlist 下完成一次真实调用并返回 `{"value":"ok"}`，证明正常路径没有被 deadline 机制破坏。

## 真实验收证据

- Go Agent Contract/Grant 定向测试通过；全新 PostgreSQL `16.15-alpine`、固定摘要 Temporal 和私有 MinIO 上执行 `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全部通过，Workflow 真实组件包用时 `95.721s`。
- Agent 使用 Python `3.11.15` 通过 Ruff check/format、Pyright `0 errors` 和 Pytest `24 passed`；测试覆盖超上限策略拒绝、Grant Hash 漂移、Invocation 总时限、子进程回收和独立 Result 语义。
- Frontend 按锁文件执行全新 `npm ci`、OpenAPI Client 生成、ESLint、TypeScript、Vitest `16 files / 45 tests` 和 Next.js `16.2.12` 生产构建，全部通过；生成 Client 无漂移。
- 开发/生产 Compose 渲染、仓库卫生、Frontend/Backend/Agent 三类镜像构建与容器入口断言全部通过；Agent 镜像包含可加载的 Candidate Runtime 与固定 Codex CLI。

## Requirement 状态与残余风险

- `BE-MOD-008` 现在已有当前固定 Definition 的调用次数和 Invocation 总执行时限，但 Token、费用、绝对排队 deadline、AgentRun/Usage Receipt、Workflow Run/Node Grant 作用域、防重放 Receipt、Backend Tool API 与生产 Model Gateway 尚未完成，因此总项保持未完成。
- 超时的本地 Codex 调用没有 Provider Job 可供查询，本切片选择确定失败且禁止自动重试；未来接入可寻址的生产 Model Gateway 后，外部结果不确定必须使用 Usage/Result Receipt 对账，不能沿用本地进程语义。
- [Runware 图片 Provider 与 Generation 执行器 Design](../design/2051-Runware图片Provider与Generation执行器设计.md)仍待用户接受；本切片没有开始图片 Provider 或 Generation Executor 编码。
- 最终 `agent-browser` 仍只在全部开发与自动化回归完成后执行，本记录不计作浏览器验收。
