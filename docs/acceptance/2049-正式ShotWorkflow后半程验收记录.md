# 正式 Shot Workflow 后半程验收记录

- 状态：实现、真实 PostgreSQL/Temporal 目标旅程与完整本地 Required CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Production Shot 图片绑定](2048-ProductionShot图片绑定验收记录.md)

## 验收范围

本记录验收正式 Shot Workflow 的后半程最小闭环：已存在一个 active Production Shot 和一个已成功物化的 Generation CandidateSet 时，Backend 能从正式 `lanverse.shot` Catalog 编译并启动独立 `lanverse.shot-production` Temporal Workflow，依次重读 Shot、重建 CandidateSet、等待用户选择图片，并把唯一 CandidateSelection 绑定到同一 Shot。

该切片不产生 CandidateSet，不选择或伪造图片 Provider，也不代表 Episode 自动扇出、图片替换、Motion/Video/Render、单 Shot 局部重跑、审核页面或完整 ShotWorkflow 已完成。

## 实现事实

1. `lanverse.shot@1.0.0` 是独立不可变 Node Catalog，只包含 `input.production_shot`、`input.generation_candidate_set`、`human.generation_image_review` 与 `production.shot_image_binding` 四个正式节点。CandidateSet Source 必须消费上游冻结 Shot 输出，因此归档、越权或漂移 Shot 会在打开 HumanTask 前失败。
2. Compiler 不接受调用方自报 Temporal Type，只按发布 Revision 冻结的 Catalog Key 映射：`lanverse.production → lanverse.episode-production`，`lanverse.shot → lanverse.shot-production`；未知 Catalog Key 失败关闭。既有 rerun/HTTP 真实测试也改用正式 Production Catalog 身份，没有新增兼容分支。
3. Production Shot Source 只通过 Storyboard Application Owner 重读 active Shot；Generation CandidateSet Source 只通过 Generation Application Owner 重建已物化 Set。显式 Executor Router 只把已知 Executor 分派给对应 Owner，未知 Executor 直接拒绝。
4. Temporal Worker 同时注册独立 Episode/Shot Workflow Name；两者复用同一确定性执行内核，但分别校验自己的 Start Identity。Shot Workflow 仍使用既有 DefinitionVersion、RunInputSnapshot、Run/Node Projection、Human Gate、Signal Intent/Receipt 与失败关闭路径。
5. API 启动时通过同一 Authoring GORM Store 确保 Episode 与 Shot 两个正式 Catalog；生产 Workflow Worker 组合真实 OutputMaterializationService、Generation Selection、Review 与 Production Binding Owner。没有新增 Migration、Raw SQL、第二 ORM、第二数据库或第二工作流引擎。

## 真实验收证据

- Red 阶段目标测试先明确失败于缺少 `SystemShotCatalog`、Catalog 到 Workflow Type 的解析、`ShotProductionWorkflow` 和独立 Generation/Production Executor Router；补齐最小实现后同一契约与执行器测试转绿。
- 正式 Journey 在 PostgreSQL `16.15-alpine` 和真实 Temporal 上执行：Temporal History 的 Workflow Type 为 `lanverse.shot-production`，节点顺序为 Shot → CandidateSet → Human Gate → Binding；HumanTask 冻结两个候选，`selected` 决议产生唯一 Selection/Owner Receipt/Apply Receipt/Signal/Binding，最终四个 Node 与 Run 全部成功。
- 同一 Journey 对完成 History 使用注册名 `lanverse.shot-production` 执行 SDK Replay 并通过；重复 Signal 返回同一 Intent，没有新增 Selection、Signal Receipt 或 Binding。归档 Shot 的第二次 Run 经真实 Activity 重试后进入 `FAILED`，HumanTask 数量为 0、Binding 仍为 1；旧 Token Version 不能重新读取 active Shot。
- 目标 Journey 普通运行 5.111 秒通过；在独立全新数据库上执行 `go test -race -count=1` 6.771 秒通过。Journey 的 CandidateSet 是受控的已物化前置快照；Provider 输出物化本身已在前置任务独立验证，本记录不宣称重新执行云 Provider。
- 首轮完整 Backend 门禁真实发现并拒绝测试层直接导入 GORM，以及两个旧测试使用非系统 Catalog Key；修正为窄测试函数依赖和正式 Production Catalog 身份后，架构边界、HTTP Temporal 与 rerun Replay 目标测试全部转绿，没有修改 CI 或加入兼容处理。
- 最终 Backend 在第三个全新 PostgreSQL 数据库、真实 Temporal 和私有 MinIO 上执行最新代码的 `gofmt`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全绿，Workflow 包耗时 162.148 秒。
- Agent 使用 Python 3.11 锁定依赖执行 Ruff check/format、Pyright 零错误与 12 个 Pytest 全绿；Frontend 执行 `npm ci`、OpenAPI 生成、ESLint、TypeScript、45 个 Vitest 与 Next.js 生产构建全绿，生成 Client 无漂移。
- 开发/生产 Compose、仓库卫生、Frontend/Backend/Agent 三类镜像及容器内 API、Workflow Worker、Frontend standalone、Codex CLI/Candidate Runtime 断言全部通过。

## 边界与残余风险

- 真实图片 Provider、Credential Resolver、回调验签和 Generation 图片执行器仍待后续切片；当前只能消费已经物化的 CandidateSet。
- Episode 动态扇出 `ShotWorkflow × N`、已绑定 Shot 替换图片所需的冻结 Binding Revision、单 Shot 局部重跑、Motion/Video/Render 和图片审核页面尚未实现，不得报告完整 ShotWorkflow 已完成。
- 本地 Codex CLI 只用于 Agent 服务内的文本/结构化 AI 调用，不是图片 Provider、CandidateSet Source 或 Backend 事实写入方。
- 远端 GitHub Actions 只有在获得推送授权后才能验证；本记录只声明本地按当前 CI 定义真实执行通过，不声明远端已绿。
- `agent-browser` 按用户约定只在全部开发完成后执行；上述后续开发仍未完成，本切片不提前调用。
