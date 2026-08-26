# Generation CandidateSet 与 Workflow 人工选择验收记录

- 状态：实现、真实目标旅程与完整本地 Required CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Generation Provider 成功输出物化](2046-GenerationProvider成功输出物化验收记录.md)

## 验收范围

本记录验收 `BE-MOD-005`、`BE-MOD-006`、`BE-MOD-007`、`BE-JRN-005` 与 `BE-JRN-006` 的 CandidateSet 人工选择切片：Generation 从成功 Provider Job 的不可变 Receipt、全部 Source Output 对应的 Candidate/QC 与当前 READY Artifact 重建候选集；Workflow 在打开 Human Gate 前通过 Generation Application Interface 展开并复核候选；Review 冻结真实 Candidate；`selected` 决议再由 Generation Owner 建立唯一 CandidateSelection 和 Command Receipt，Workflow 将同一 Owner Receipt 绑定到 Apply Receipt、Temporal Signal 与 Gate Node Output。

该切片完成的是“已成功物化的 Provider Job 可以通过 Workflow 人工选择并可靠恢复”的 Backend 内部闭环，不代表真实云图片生成、最终图像节点或单 Shot Workflow 已完成。

## 实现事实

1. `CandidateSet` 是只读派生值，不新增 Model、表或迁移文件。Set ID 复用 Provider Job ID，Revision 固定为 `1`；Generation 每次从成功 Provider Result 及全部输出重读 Candidate/QC/Artifact，拒绝未完整物化、QC 未通过、Artifact 非 READY、元数据漂移或权限失效。
2. CandidateSet 只保留 QC 通过的 Candidate Reference，按 Candidate ID 稳定排序；Content Hash 与 `CandidateSelection.CandidateSetHash` 调用同一 canonical Reference Hash 实现，避免第二种哈希口径。
3. `gate.generation_image_review` 的 Runtime Binding 冻结 Executor、Workflow 发起身份和完整 CandidateSet 输入。HumanTask Opener 只能通过 Generation Application Port 重建候选，Workflow/Review Repository 不查询 Generation 表，也不接受客户端提交 Candidate ID 或 Set Hash。
4. `selected` 决议只把不可变 ReviewDecision ID 交给既有 Generation Selection Service。Generation 重新验证 HumanTask、Decision、冻结候选、Candidate QC 与 Artifact Readiness，再原子写唯一 CandidateSelection 和 `generation.candidate.select` Command Receipt。
5. Generation Human Gate Applier 验证 Owner 返回的 Workspace、Project、Run、Node、Task、Decision、Subject Revision、CandidateSet Hash、选中 Candidate 和创建身份，再产生唯一 `generation_candidate_selection` Node Output；Workflow Apply Receipt、Signal Intent 和 Gate 输出引用同一 Owner Receipt。
6. 生产 `workflow-worker` 使用已有 PostgreSQL、GORM 与私有 MinIO 配置装配真实 CandidateSet Source；没有新增 SQL、第二 ORM、第二数据库、CandidateSet 表、兼容字段、通用事件路由或跨 Owner 直写。

## 真实验收证据

- Red 阶段先扩展目标测试；编译明确失败于缺失 Generation Workflow Adapter、`CandidateSet` 结果与 `RequireCandidateSet`，随后 Opener 测试继续失败于缺失 Generation 组合入口和 Human Gate Binding。补齐最小 Domain/Application/Adapter/Bootstrap 后同一测试转绿。
- 真实 PostgreSQL `16.15-alpine`、Temporal 固定镜像和私有 MinIO `RELEASE.2025-09-07T16-13-09Z` 上，Generation 目标旅程与 Workflow 目标旅程分别通过；最终服务重建后的 Workflow 目标用例 5.913 秒通过。
- Provider 输出物化测试验证两张真实 MinIO 图片形成稳定 CandidateSet；再次从 Provider Receipt、全部 Candidate/QC 与 READY Artifact 重建得到相同 Set。篡改测试中的 Artifact SHA 后重建失败关闭，恢复后继续收敛。
- Workflow 旅程使用真实 GORM Catalog/Authoring Revision/Definition/Run/Node Projection、PostgreSQL Review/Selection/Receipt 与 Temporal Worker。HumanTask 冻结两个排序候选，`selected` 决议最终只形成一个 CandidateSelection、一个 Owner Command Receipt、一个 Apply Receipt 和一个 Signal Intent。
- 首次 Temporal Signal 已真实送达但注入“结果丢失”，Signal Intent 持久化为 UNKNOWN；重新构造 `SignalService` 后只依据 PostgreSQL 与 Temporal 历史完成对账。随后 8 路并发重放全部返回同一完成事实，没有重复选择或生产副作用。
- Workflow Run 最终进入 `SUCCEEDED`，Gate NodeRun 保存正式 `generation_candidate_selection` 输出；从真实 Temporal 服务读取完整 History 后，官方 Workflow Replayer 通过。
- Generation 与 Workflow 目标测试均通过 `go test -race`；Gofmt、Go Vet、模块包测试以及 Backend 在全新数据库、真实 PostgreSQL/Temporal/MinIO 上执行的 `go test -count=1 -p 1 ./...` 全绿。
- Agent 使用 Python 3.11.15 执行锁定依赖安装、Ruff check/format、Pyright 零错误与 12 个 Pytest 全绿；Frontend 使用 Node 22.23.2 执行 `npm ci`、OpenAPI 生成、ESLint、TypeScript、45 个 Vitest 与 Next.js 生产构建全绿，生成 Client 无漂移。
- 开发/生产 Compose 配置、仓库卫生、Frontend/Backend/Agent 三类镜像构建与容器内 API、Workflow Worker、Frontend standalone、Codex CLI/Candidate Runtime 检查全部通过。一次 Agent 本地门禁因临时 PATH 只作用于首个命令而未找到 Ruff，一次目标测试因误用临时数据库口令在连接阶段被拒绝；两者均修正执行环境后原样重跑通过，不是代码放行或兼容处理。

## 边界与残余风险

- 真实旅程的两节点 Catalog 和 CandidateSet Source Executor 是测试专用受控输入；生产 Worker 已装配 CandidateSet 复核能力，但 System Catalog 尚未发布 Generation 图片源节点，也没有图片 Executor。因此当前用户还不能从正式画布启动这条图片选择路径。
- 首个真实图片 Provider、Credential Resolver、Callback 验签和生产下载协议尚未正式选定或实现；当前不能宣称云图片生成可用。
- Production Shot Binding、Temporal Shot Workflow、单 Shot 局部重跑、图片审核页面和最终用户剧本到成片全流程仍待后续任务。
- 本地 Codex CLI 继续只承担 Agent 服务内的文本/结构化 AI 调用，不作为图片 Provider、CandidateSet Source 或生成结果兼容替身。
- 远端 GitHub Actions 只有在获得推送授权后才能验证；本记录只声明本地按当前 CI 定义真实执行通过，不声明远端已绿。
- `agent-browser` 按用户约定只在全部开发完成后执行；上述后续开发仍未完成，本切片不提前调用。
