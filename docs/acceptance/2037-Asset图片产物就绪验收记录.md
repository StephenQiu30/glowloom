# Asset 图片产物就绪验收记录

- 状态：实现、真实目标旅程与完整本地 CI 已通过；当前提交的远端 GitHub Actions 待推送后验证
- 日期：2026-08-26
- Design：[后端领域模块功能设计](../design/2002-后端领域模块功能设计.md)
- PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md)
- Plan：[后端领域服务与生产闭环实施计划](../plan/2002-后端领域服务与生产闭环实施计划.md)

## 验收范围

本记录验收真实 Generation/Shot 节点的首个 Asset 前置：Backend Asset Owner 把私有 MinIO Staging 图片登记为 PostgreSQL Artifact/Location 事实，完整读取并核对 SHA-256、字节数、媒体类型和可解码性后进入 `READY`，损坏产物进入 `QUARANTINED`，下游只可通过 `RequireReady` 消费。范围不包含 Provider 调用、Asset/AssetVersion、Rights/Consent、Generation Candidate/QC/Selection、公共 Asset HTTP 或 Shot Workflow。

## 实现事实

1. 新增 `asset` Domain/Application/GORM Adapter；Artifact、Location、Readiness 与 Owner Receipt 只由该模块写入，图片字节只在私有 MinIO，Python Agent 不写业务事实。
2. `Artifact` 以 Workspace + Source Type + Source ID + Output Key 唯一；`ArtifactLocation` 记录 Storage Profile、Bucket、Object Key、Region、Checksum 与 `STAGING/PRIMARY`。两者进入唯一 GORM Model Catalog，没有 Migration 文件、手写 SQL Schema、第二数据库或兼容写分支。
3. `asset.artifact.register_staged` 在同一 GORM 事务内以 ORM `ON CONFLICT DO NOTHING` 收敛并发 Source Output，再写通用 `CommandReceipt`；同 Idempotency Key 或 Source Output 的声明漂移均拒绝。
4. `asset.artifact.validate_ready` 在事务外完整读取对象，在事务内以 Revision/Status 围栏原子提交 Readiness、Location 与 Receipt；SHA-256/大小不符、媒体类型伪报或不可解码图片进入 `QUARANTINED`，对象存储依赖错误保持 `PENDING_VALIDATION` 且不写验证 Receipt。
5. `RequireReady` 同时复核当前用户 Token Version、Workspace Membership、Project 归属、Artifact `READY` 和 Location `PRIMARY`；Pending 与 Quarantined 均 fail closed。
6. 复用现有 GORM、共享 PostgreSQL Unit of Work、通用 Command Receipt 与 MinIO Go SDK；通用 Receipt Adapter 新增语义 JSON 对账的并发 `Ensure`，不会因 PostgreSQL JSONB 空白规范化产生伪漂移。

## 真实验收证据

- Red 阶段先执行 `go test ./tests/asset`，因 `internal/asset` 不存在而编译失败；实现后同一目标测试转绿。
- 隔离 PostgreSQL 16.15 与 MinIO `RELEASE.2025-09-07T16-13-09Z` 旅程通过预签名 PUT 上传真实 PNG。8 路并发登记返回同一 Artifact 与 Receipt；同 Source Output 换 Command Key 重投仍返回同一 Artifact，输入漂移被拒绝。
- 4×3 PNG 经完整读取、SHA-256、大小、`image/png` 与 DecodeConfig 核对后成为 `READY`，Revision 从 1 变为 2，Location 从 `STAGING` 变为 `PRIMARY`；重复验证返回同一 Receipt。
- 声明错误 SHA-256 的真实 PNG 被持久化为 `QUARANTINED/checksum_mismatch`；模拟对象存储不可达后 Artifact 仍为 `PENDING_VALIDATION` Revision 1，验证 Receipt 数量不增加。
- 真实 JPEG 按 `image/png` 伪报时在完整性校验通过后仍被识别为 `QUARANTINED/media_type_mismatch`；最终 Workspace 范围内事实数精确为 4 个 Artifact、4 个 Location、5 个登记 Receipt、3 个终态验证 Receipt，Pending/Quarantined 均无法通过 `RequireReady`。
- 全新隔离 PostgreSQL + Temporal + MinIO 执行 `go test -count=1 -p 1 ./...` 全部通过，Asset 与 Workflow 外部依赖测试均实际运行；`go vet ./...`、`gofmt` 与数据库架构边界通过。
- Agent Ruff check/format、Pyright 零错误与 12 个 Pytest 通过；Frontend OpenAPI 重生成、ESLint、TypeScript、45 个 Vitest 与 Next.js 生产构建通过。
- 开发/生产 Compose 可渲染；Frontend、Backend、Agent 三类部署镜像重新构建并分别通过 standalone、API/Workflow Worker 双二进制和私有 Candidate Runtime 入口检查。
- GitHub CI Backend Job 已加入真实私有 MinIO，并向测试传入隔离 Bucket 配置；该旅程在远端不能再依靠缺失环境变量跳过。

## 边界与残余风险

- Stage 3 Asset 尚未完成 Asset/AssetVersion、Lineage、Rights/Consent、Download Grant、上传 Owner 收敛或位置迁移；现有 Document Media 上传事实不在本切片内，不能把本记录解释为完整 Asset 模块。
- Generation Provider/Intent/Job/Callback、Cost/Quota、Candidate/QC/Selection 与 Production Binding 尚未接入，因此系统仍没有真实图片生成节点或单 Shot 媒体重跑。
- Asset Application Service 当前只作为下一个 Generation Owner 的内部端口，没有公共 HTTP Handler；下一切片必须通过该端口接入，不得由 Workflow 或 Generation 直接查表。
- 远端 `main` 的基线提交 `2f6e066` 已在 GitHub Actions run `32919014443` 全绿；本次未提交改动的远端 CI 只有在获得推送并实际运行后才能报告绿色。
- `agent-browser` 按约定只在全部开发完成后执行；当前仍有 Generation/Shot 明确后续任务，因此本切片不提前调用。
