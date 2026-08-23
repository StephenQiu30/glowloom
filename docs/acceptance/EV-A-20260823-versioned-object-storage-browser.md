# 剧本原件精确版本、Fixture 幂等与存储位置隔离

- Evidence ID：`EV-A-20260823-versioned-object-storage-browser`
- 结论：`passed`（只覆盖逻辑素材/物理位置分离、精确版本读取和 Fixture 重复命令场景，不代表切片 A 全部通过）
- 执行时间与时区：2026-08-23，Asia/Shanghai
- 执行人 / 复核人：Codex / 待复核
- Git commit：存储 RED `3bd233b`，Worker 租户绑定 `2b47a29`，存储实现 `bc12b7d`，失败回执测试 `cdb19f0`，刷新恢复 RED `35b8f3e`，恢复实现 `79b3570`
- 环境：macOS 本机 PostgreSQL 18.4、Redis、Kafka、临时 MinIO、Go 1.26.5 API/operation-worker、Node.js 24.19.0 Next.js standalone、agent-browser 0.33.0；未使用 Docker
- 数据集：页面内置三集中文 TXT 测试剧本；仅含自建测试内容，不含第三方生产内容
- 当前 Swagger SHA-256：`b677e05ee89683d920844bb4c019c7f4108c576188535a741e1af2ed807d89ef`
- 关联：`IAM-FR-008`、`IAM-NFR-001/005`、`MED-FR-001/004/009`、`PRD-A-AC-007/008` 的本证据子场景、`PLAN-A A1/A8`、`ACC-GATE-002/004`
- 自动化测试：`backend/tests/architecture/object_storage_contract_test.go`、`backend/tests/scripts/source_storage_integration_test.go`、`backend/tests/scripts/worker_tenancy_integration_test.go`

## 前置条件

- 从空数据库加载唯一 `backend/schema/current.sql`；复用本机 Redis 和唯一 Kafka，不修改或删除共享主题。
- 使用本机 MinIO 二进制在隔离临时目录及 `19100/19101` 端口运行，Bucket 开启版本化。
- API、Worker、前端和浏览器使用测试专用数据库、身份与运行配置；证据不保存 Cookie、JWT、MinIO 凭据、对象键或 `.env` 内容。
- 完成后关闭 agent-browser、API、Worker、Next standalone 和临时 MinIO，删除隔离数据库；临时 MinIO 目录移入系统废纸篓，可恢复。

## 执行命令与步骤

```text
cd backend
go test ./... -count=1
go vet ./...
make swagger

DATABASE_URL=<隔离 PostgreSQL> LANVERSE_INTEGRATION=1 \
  go test ./tests/... -count=1

cd frontend
OPENAPI_SCHEMA_URL=../backend/docs/swagger.json npm run openapi2ts
npm run lint
npm run typecheck
npm test -- --run
npm run build
npm run start

agent-browser --session lanverse-storage open http://127.0.0.1:8123
agent-browser --session lanverse-storage snapshot -i
# 注册测试身份，提交页面内置三集 TXT，等待 Worker，批准事实
agent-browser --session lanverse-storage snapshot
# 在同一浏览器会话中刷新 HttpOnly 会话，调用 Shot/Fixture 当前 API 两次
agent-browser --session lanverse-storage a11y --json
agent-browser --session lanverse-storage vitals --json
agent-browser --session lanverse-storage errors
agent-browser --session lanverse-storage console
# 重启 API/operation-worker，reload 保留三元定位符的当前页面
agent-browser --session lanverse-storage close
```

## 实际结果与逐项断言

1. 当前 Schema 中 `nar_source_revisions` 只保存逻辑 `artifact_id`，并通过 `(artifact_id, project_id)` 复合外键绑定同项目 `media_artifacts`；物理位置只存在于 `media_artifact_locations`。
2. Source 和 Fixture 共形成 2 个逻辑 Artifact、2 个 active Location；全部 Location 都有非空 MinIO `object_version_id`，Artifact/Location 的 SHA-256 与字节数一致，对象键不包含 Project UUID。
3. Worker 从 Outbox/Kafka 消息恢复 Workspace/Project 绑定，核对原始 outbox 四元组后，只使用 active Location 的精确 object version 读取剧本；成功解析并批准 3 集、3 场景、2 人物、11 项生产资产，failed scopes 为 0。
4. 浏览器创建 1 个 Shot 后，对相同 purpose 连续提交两次 Fixture 命令；两次 HTTP 201 返回同一 Candidate ID。数据库只有 1 个 Fixture Candidate、1 个逻辑 Job 和 1 个 Attempt，没有第二次对象上传或业务副作用。
5. Candidate 公共响应字段为 `artifact_id/content_hash/fixture/id/project_id/status/target_id/target_type`；没有 `object_key`、Bucket、storage profile 或 object version。Swagger 和前端生成类型同步删除物理位置字段。
6. MinIO 适配器不再提供 latest-key 读取；应用只依赖版本化 ObjectStorage Port。上传回执缺少精确版本或事务失败时不会创建 ready Artifact，并只清理能够精确定位的未采用版本。
7. 提交 Operation 后 URL 只保存 Project、SourceRevision、Operation 三个逻辑 ID。API 与 operation-worker 重启并 reload 后，页面通过 HttpOnly refresh 重新鉴权，自动恢复为“事实已批准”、原 Operation、3 集 TXT ParseReport 和 0 failed scopes；没有把 JWT、Workspace 或正文写入 localStorage。
8. production 页面 `console` 与 `errors` 均为空。axe-core 4.12.1 结果为 `violations=0`、`incomplete=0`、`passes=35`；Core Web Vitals 采样为 CLS 0、FCP 56 ms、LCP 76 ms、TTFB 2.1 ms。
9. 将 URL Project 替换为另一个格式合法 UUID 时，客户端先回读 Operation 的服务端 project/source binding，显示统一“无法恢复当前剧本工作流”，清除定位符且不加载项目分析；重新打开原定位符可恢复 approved 页面。

## Red → Green

- RED 编译失败证明 Repository 只能接受具体 MinIO 类型，无法替换为版本化夹具；Schema 缺少 `media_artifact_locations`，Swagger Candidate 暴露 `object_key`。
- Green 后内存版本化存储集成测试固定 opaque key、exact version、SHA-256/size 对账和失败清理；真实 PostgreSQL 套件从空库通过。
- production 浏览器进一步覆盖真实 MinIO versioning、真实 Kafka Worker、Fixture 重复命令和进程恢复，不以 mock 或直插数据库替代用户/API 路径。

## 偏差、残余风险和后续动作

- 本证据没有实现或验收外部分享、受控下载网关、短时签名媒体访问、Range、撤销传播、LegalHold 和 retention；这些 M09/M13/M14 场景继续为 `not_run`。
- 本次执行时只恢复 URL 指向的单个最近工作流；正式项目列表和无预存 URL 的跨设备继续已由 [`EV-A-20260823-project-list-browser`](./EV-A-20260823-project-list-browser.md) 后续验证，多个项目复杂切换、草稿/失败 Operation 中心仍须在 A9/A10 前验收。
- 只执行了顺序重复 Fixture 命令和浏览器进程重启；Kafka 重平衡、并发双请求及 MinIO 中断的完整 A8/A10 故障矩阵尚未通过。
- 数据集不满足完整 DS-A-SCRIPT/DS-A-MEDIA，PRD-A-AC-007/008 和切片 A 不能据此标记为 passed/verified。
