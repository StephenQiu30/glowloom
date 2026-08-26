# Generation Provider 成功输出物化验收记录

- 状态：实现、真实目标旅程与完整本地 Required CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)
- 前置验收：[Generation Provider 提交与结果对账](2045-GenerationProvider提交与结果对账验收记录.md)

## 验收范围

本记录验收 `BE-MOD-007`/`BE-JRN-005` 的首个 Provider 输出物化边界：Backend 从成功 Provider Result Receipt 重读唯一输出事实，经真实私有 MinIO 和 Asset Owner 完成 Staging 登记与 Readiness 校验，再由 Generation Owner 建立 Candidate 与确定性 QC。重复、并发、Owner 中断和重启恢复只能重放稳定命令，不能产生第二套 Artifact、Location、Candidate 或 QC Report。

本增量仍使用受控 Provider Gateway Adapter 产生并写入真实测试图片字节，不选择云 Provider，不实现 Credential Resolver、Callback 公网验签、自动 Provider 下载、公开 REST、Temporal 图片节点、CandidateSet/Human Gate 或 Production Binding，因此不代表第一笔真实云图片生成、单 Shot Workflow、`BE-MOD-007`、`BE-JRN-005` 或阶段 6 完成。

## 实现事实

1. `ProviderSubmission` 新增 Backend 冻结的 Workspace、Project 与 Provider Job ID；Adapter 得到足以构造合法私有 Staging 路径的 Owner 身份。成功输出只接受 `staging/{workspace_id}/{provider_job_id}/...`，越界路径、目录穿越和非法 Output Key 不进入成功终态，而是收敛为 UNKNOWN 并保持 Cost/Quota 预留。
2. `RequireSucceededProviderResult` 在 Provider GORM 事务中重读 Job、Request、Intent、Binding、Result Receipt、Cost Reservation 和 Quota Reservation，重新验证当前发起人 Token/Membership；只有 Job/Intent 成功、Cost 已结算、Quota 已消费且所有 Content Hash/Owner Receipt 一致时才返回输出。
3. `OutputMaterializationService` 不接受客户端 Output、Artifact、Candidate 或幂等键。它按不可变 Receipt Output 顺序，用 `Provider Receipt ID + Output Key` 派生稳定的 Asset Register、Asset Validate 与 Candidate Register 命令。
4. Asset Adapter 真实调用 `asset.artifact.register_staged` 和 `asset.artifact.validate_ready`。Asset 从私有 MinIO 完整读取对象并验证字节数、SHA-256、媒体类型和图片可解码性；Generation 随后继续核对真实宽高与 Provider Receipt，任何声明漂移均不创建 Candidate。
5. READY Artifact 才能进入现有 `generation.candidate.register_ready`，由 Generation Owner 原子写唯一 Candidate、不可变确定性 QC Report 和 Command Receipt。隔离、未就绪、撤权、Owner 事实漂移和 QC 输入漂移全部失败关闭。
6. MVP 没有新增物化状态表、Migration、DDL/Raw SQL、第二 ORM、第二数据库、Redis 正确性、跨 Owner 大事务、补偿删除或兼容分支。不可变 Provider Receipt 与现有 Owner Receipt 就是恢复事实；已提交的部分进度保留并在重试时重放。

## 真实验收证据

- Red 阶段目标测试先因 `ProviderSubmission` 缺少 Workspace/Project/ProviderJob 身份，以及 `NewOutputMaterializationService`、`NewProviderOutputReadiness` 和结果类型不存在而明确编译失败；补齐最小 Application Port/Adapter 后同一测试转绿。
- 隔离的 PostgreSQL `16.15-alpine` 与 MinIO `RELEASE.2025-09-07T16-13-09Z` 上，目标旅程扩展前连续三次通过，累计 20.371 秒；扩展后的单次旅程 7.281 秒通过，最终源码执行 `go test -race` 10.155 秒通过。
- 受控 Adapter 在真实 MinIO 写入两张 PNG；8 路并发物化最终只有两个 Artifact、两个 Location、两个 Candidate 和两个 QC Report，全部调用返回相同 Owner ID。再次重放不增加事实。
- 注入 Candidate Owner 首次失败后，Asset Register/Validate Receipt 与 READY Artifact 保留；第二次调用重放 Asset 命令并只补齐 Candidate/QC，没有补偿删除或重复产物。
- 缺失 Staging 对象返回 `dependency_unavailable`，Artifact 保持 `PENDING_VALIDATION`；带 PNG 签名但不可解码的对象进入 `QUARANTINED/image_decode_failed`；真实 `4x3` 对象被 Provider 声明为 `5x3` 时 Artifact 可 READY，但 Generation 因 Receipt 宽高漂移拒绝 Candidate。
- 非本 Job/Workspace 的 Staging 路径被 Provider 协调器降为 UNKNOWN，不能物化；发起人 Token Version 撤销后，已经成功的 Job 也不能再次通过物化门禁。
- 完整 Generation 外部依赖包通过，用时 32.081 秒；Provider 提交/UNKNOWN/Reconcile 既有旅程在新 Staging 约束下继续通过。数据库架构测试确认测试与生产代码都未把 GORM 引入非数据库边界。
- Backend 在全新数据库与隔离 Temporal/MinIO 上执行 `test -z "$(gofmt -l .)"`、`go vet ./...`、`go test -count=1 -p 1 ./...` 全绿；Workflow 真实集成套件 146.977 秒。首次完整运行在格式门失败，第二次在测试越界导入 GORM 的架构门失败；均按真实原因修复后从整条命令重跑，没有白名单、跳过或兼容例外。
- Agent 使用 Python 3.11.15 临时环境执行 Ruff check/format、Pyright 零错误与 12 个 Pytest 全绿；Frontend 使用官方 Node 22.23.2 执行 `npm ci`、OpenAPI 重生成、ESLint、TypeScript、45 个 Vitest、Next.js 生产构建和生成 Client 零漂移，全部通过。
- 开发/生产 Compose、仓库卫生、Frontend/Backend/Agent 三类镜像及 API、Workflow Worker、Frontend standalone、Agent 容器内 Codex CLI/Candidate Runtime 入口全部通过。Backend 镜像首次因 Go 模块代理下载 `unexpected EOF` 失败，未改代码或依赖，完整 Deployment 门原样重跑后通过。

## 边界与残余风险

- 当前受控 Adapter 会把真实图片字节写入私有 MinIO 并覆盖完整物化链，但仍不是实际云图片 Provider；正式设计尚未选定首个图片 Provider、Credential 来源和 Callback 签名协议，不能宣称云生成可用。
- 当前物化由 Backend Application Interface 显式触发，尚未接入 Temporal Workflow 图片 Activity、CandidateSet、Quality Gate、Human Gate 或单 Shot 局部重跑；这是下一任务的范围。
- 本地开发阶段的剧本拆分、分镜等文本/结构化 AI 调用继续复用现有 Agent 服务到本机 Codex CLI 的受限执行路径；Codex CLI 不作为图片 Provider 或 Staging 契约的兼容替身。
- 远端 `origin/main` 当前为 `ec2bb71`；本地已有 `40197a3`、`ce38c3b` 两个未推送提交，本任务将在本记录所列完整 CI 后独立提交。远端 GitHub Actions 仍须获得推送授权后验证。
- `agent-browser` 按约定只在全部开发完成后执行；当前仍有 Workflow 图片节点、CandidateSet/Human Gate、单 Shot Workflow 与真实 Provider 后续任务，本切片不提前调用。
