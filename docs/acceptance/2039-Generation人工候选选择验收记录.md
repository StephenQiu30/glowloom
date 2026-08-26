# Generation 人工候选选择验收记录

- 状态：实现、真实目标旅程与完整本地 CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)

## 验收范围

本记录验收 `BE-JRN-005` 的第三个前置边界：Review Owner 通过经当前身份重新授权的 Application Port 返回已完成 HumanTask 与不可变 `selected` ReviewDecision；Generation Owner 重新验证 HumanTask 冻结集合内全部 Candidate 的确定性 QC 和当前 Artifact READY 后，原子建立唯一 CandidateSelection 与 `generation.candidate.select` CommandReceipt，并由 `RequireSelected` 为后续 Workflow/Production 提供失败关闭门禁。

本增量不包含 Capability/Model Policy、GenerationIntent/CandidateSet、真实 Provider Submission/Callback、Cost/Quota、语义 QC、公共 Generation HTTP、HumanTask 候选展开、Production Binding、Temporal Signal 或单 Shot Workflow。

## 实现事实

1. Review 新增 `GetDecision` Application Port；GORM Adapter 在同一 PostgreSQL 中读取 Decision/Task，重新校验当前 User Token Version、Owner/Editor Membership、Task `COMPLETED`、Subject Revision、Reviewer 和冻结 Candidate 集合。Generation 只依赖该端口，不导入 Review GORM Adapter、不查询 Review 表。
2. Generation 应用命令只接受 ReviewDecision ID 与幂等键，不接受客户端自报 Candidate、Reviewer、Subject 或 Workflow 绑定；只有 `selected` 决议且选中值属于 HumanTask 冻结集合时才能继续。
3. Generation 对冻结集合的每个 Candidate 调用既有 `RequireQCPassed`，冻结 Candidate/Artifact Revision、Artifact SHA-256、QC Report ID/Hash，并计算稳定 CandidateSet Hash 与 Selection Content Hash；任一非选中 Candidate QC 失败也拒绝整个 Selection。
4. `GenerationCandidateSelection` 对 HumanTask 与 ReviewDecision 双重唯一，冻结 Workflow Run/Node、Subject/Revision、排序 Candidate 引用、Selected Artifact、Reviewer 和内容哈希。该记录进入唯一 GORM Model Catalog，没有 Migration 文件、手写 SQL Schema、第二数据库或兼容写分支。
5. `generation.candidate.select` 在一个 GORM/PostgreSQL 事务中以 ORM 冲突收敛写 Selection 和通用 CommandReceipt；相同命令、同一 Decision 的新投递只复用唯一 Selection，幂等输入或绑定漂移均 fail closed。
6. `RequireSelected` 每次消费都重新授权、重读 Review Decision、重跑全部 Candidate 的 QC/Asset 门禁，并重算 CandidateSet/Content Hash；撤权或任何选择、决议、候选、QC、Artifact 事实漂移都不能继续。

## 真实验收证据

- Red 阶段先执行 `go test ./tests/generation`，明确因 `internal/generation/adapter/review` 不存在而编译失败；完成最小实现后同一 Generation 测试包转绿。
- 目标旅程使用 PostgreSQL `16.15-alpine` 与私有 MinIO `RELEASE.2025-09-07T16-13-09Z`，真实上传三张 PNG，经 Asset READY 和 Generation QC 后建立 Review Task/Decision，再应用 Selection。
- 8 路并发应用同一 ReviewDecision 收敛为同一 Selection 和同一 Receipt；相同命令回放不新增事实，以新幂等键重投只新增一个 Receipt，最终当前 Workspace 精确为 1 个 Selection、2 个 Selection Receipt。
- `approved` 决议被拒绝；冻结集合包含一个未通过 QC 的非选中 Candidate 时仍整体拒绝；同一幂等键更换 ReviewDecision 被拒绝。
- `RequireSelected` 对撤销 Token、被篡改的 Review 选中值、QC Report Hash 和 Selection Content Hash 均失败关闭，恢复原事实后门禁才重新可用。
- 首次把 Generation 与 Review 包在已写入目标旅程的共享数据库中串行执行时，旧 Review 测试因对全表计数得到 `tasks 5 / decisions 5` 而真实失败；测试已改为只断言自身 Workspace，未清库、未跳过、未增加兼容分支。随后使用全新隔离数据库执行完整 CI。
- 全新 PostgreSQL + Temporal + MinIO 下，`test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 均以退出码 0 完成；Workflow 套件实际运行约 64.5 秒，Generation、Review、Asset 与 Workflow 外部依赖旅程均未跳过。
- Agent 的 Ruff check/format、Pyright 零错误与 12 个 Pytest 通过；Frontend OpenAPI 重生成、ESLint、TypeScript、45 个 Vitest 和 Next.js 生产构建通过，生成 Client 无漂移。
- 开发/生产 Compose 与仓库卫生检查通过；Frontend、Backend、Agent 三类镜像重新构建成功，并分别通过 standalone、API/Workflow Worker 双二进制和私有 Candidate Runtime 入口检查。

## 边界与残余风险

- CandidateSelection 目前是 Backend 内部 Owner Port，尚未由 Workflow Human Gate 应用；现有 Human Gate 仍只消费其既有输入绑定，不能宣称 CandidateSet 已展开、图片选择已驱动 Shot Workflow 或 Production Binding。
- Provider/Model Snapshot、Submission/Callback、Unknown/Reconcile、Cost/Quota 和语义 Quality Gate 尚未实现；本增量消费的是既有测试 Provider Source Artifact，不能声称真实图片生成 Provider 已接通。
- 当前只实现图片确定性 QC 与人工 Selection；QCReport Artifact、语义 QC、Repair/Fallback、公共审核页面和媒体渲染仍属后续切片。
- 远端 `origin/main` 仍停留在已通过 GitHub Actions run `32919014443` 的 `2f6e066`；本地 Asset、Candidate/QC 与当前 Selection 提交只有在获得推送并实际运行后才能报告远端绿色。
- `agent-browser` 按约定只在全部开发完成后执行；当前还有 Provider/Cost/Quota、CandidateSet/Workflow 和单 Shot Workflow 明确后续任务，因此本切片不提前调用。
