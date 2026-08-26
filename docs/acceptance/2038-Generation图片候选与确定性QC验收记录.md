# Generation 图片候选与确定性 QC 验收记录

- 状态：实现、真实目标旅程与完整本地 CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)

## 验收范围

本记录验收 `BE-JRN-005` 的第二个前置边界：Backend Generation Owner 只能消费 Asset `RequireReady` 返回的 Provider 图片 Artifact，原子登记唯一 GenerationCandidate、不可变确定性 QCReport 与 Command Receipt，并由 `RequireQCPassed` 为后续 Selection/Shot Workflow 提供失败关闭门禁。范围不包含 Capability/Model Policy Catalog、Generation Intent/Request/ProviderJob 事实、远程 Provider 调用、Cost/Quota、语义 QC、Review、CandidateSelection、Production Binding、公共 Generation HTTP 或 Shot Workflow。

## 实现事实

1. 新增 `generation` Domain/Application/GORM Adapter 与 Asset Application Adapter；Generation 不导入 Asset GORM Adapter、不查询 Asset 表、不读取 MinIO，Python Agent 不参与 Candidate/QC 事实写入。
2. `GenerationCandidate` 按 Workspace + Artifact 和 Workspace + Provider Job Source ID + Output Key 双重唯一，冻结 Artifact Revision/SHA-256、媒体类型、宽高与来源；`GenerationQCReport` 对 Candidate 唯一且不可变。两者进入同一个 GORM Model Catalog，没有 Migration 文件、手写 SQL Schema、第二数据库或兼容写分支。
3. `generation.candidate.register_ready` 在一个 PostgreSQL/GORM 事务中用 ORM `ON CONFLICT DO NOTHING` 收敛 Candidate/QC，再写通用 CommandReceipt；相同命令、Artifact 或 Source Output 的绑定漂移均拒绝。
4. 确定性图片 QC 冻结 Version、允许媒体类型、最小宽高、最大像素数与 Policy Hash；结果为 `QC_PASSED/PASSED` 或 `QC_FAILED/FAILED`，失败码顺序稳定。QC Report Hash 在读取时重新计算，JSONB 的空失败码统一为 `[]`，不能用 `null` 造成回放漂移。
5. `RequireQCPassed` 重新校验当前 User Token Version、Membership、Project 归属、Candidate/Report 状态、Policy/Report Hash，并再次调用 Asset `RequireReady` 对账冻结 Artifact；撤权、非 READY、QC 失败或事实漂移均 fail closed。

## 真实验收证据

- Red 阶段先执行 `go test ./tests/generation`，因 `internal/generation/adapter/asset` 尚不存在而编译失败；实现后同一目标测试在隔离依赖上转绿。
- 目标旅程使用 PostgreSQL 16.15 与私有 MinIO `RELEASE.2025-09-07T16-13-09Z`，通过预签名 PUT 上传真实 PNG，再经 Asset 完整读取和 READY 门禁交给 Generation。
- 8 路并发登记同一 READY Artifact 只形成一个 Candidate、一个 QCReport 和一个相同 CommandReceipt；换命令键重投仍复用同一 Candidate/Report，同一键更换 Artifact 和同一 Artifact 更换 QC Policy 均拒绝。
- 4×3 PNG 在 `image-deterministic-v1` 下通过；2×2 PNG 分别得到 `width_below_minimum`、`height_below_minimum`，11×10 PNG 得到 `pixel_count_exceeded`，JPEG-only Policy 对 PNG 得到 `media_type_not_allowed`。
- Pending Artifact、非 `generation_provider_job` 来源、已撤销 Token 与被篡改的 QC Report Hash 均无法通过对应门禁；最终 Workspace 事实数精确为 4 个 Candidate、4 个 QCReport、5 个登记 Receipt。
- 全新隔离 PostgreSQL + Temporal + MinIO 执行 `go test -count=1 -p 1 ./...` 全部通过，Generation 与既有 Asset/Workflow 外部依赖旅程均实际运行；`go vet ./...`、`gofmt` 和数据库架构边界通过。
- Agent Ruff check/format、Pyright 零错误与 12 个 Pytest 通过；Frontend OpenAPI 重生成、ESLint、TypeScript、45 个 Vitest 与 Next.js 生产构建通过，生成 Client 无漂移。
- 开发/生产 Compose 可渲染；Frontend、Backend、Agent 三类镜像均重新构建并分别通过 standalone、API/Workflow Worker 双二进制和私有 Candidate Runtime 入口检查。Backend 镜像两次曾在 Docker 内下载 Go Module 时遇到外部 `unexpected EOF`，未计为通过；同一未修改构建命令最终成功。

## 边界与残余风险

- Candidate 当前只验证 READY Artifact 携带的稳定 Provider Job Source ID，不创建或伪造 ProviderJob 事实；Capability/Model/Provider Snapshot、Submission/Callback、Unknown/Reconcile、Cost/Quota 仍须在后续独立切片完成。
- 本切片只有确定性图片 QC；语义 QC、QCReport Artifact、QualityGate、ReviewDecision、唯一 CandidateSelection 与 Production Binding 尚未实现，`QC_PASSED` 不等于已选择或可进入正式生产。
- Generation Application Service 目前是后续 Workflow Executor 的内部 Owner Port，没有公共 HTTP Handler，也未加入 Authoring Node Catalog；系统仍不能声称已有图片生成节点或单 Shot 媒体重跑。
- 远端 `origin/main` 仍停留在已通过 GitHub Actions run `32919014443` 的 `2f6e066`；本地 Asset 提交与当前 Candidate/QC 提交都只有在获得推送并实际运行后才能报告远端绿色。
- `agent-browser` 按约定只在全部开发完成后执行；当前仍有 Provider/Selection/Shot Workflow 明确后续任务，因此本切片不提前调用。
