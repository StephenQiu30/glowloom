# 正式项目列表与跨设备剧本工作流恢复

- Evidence ID：`EV-A-20260823-project-list-browser`
- 结论：`passed`（只覆盖当前 Workspace 项目分页、最新剧本工作流恢复和并发服务启动场景，不代表 M02、切片 A 或项目概览全部通过）
- 执行时间与时区：2026-08-23，Asia/Shanghai
- 执行人 / 复核人：Codex / 待复核
- Git commit：RED `6b81c48`，项目入口实现 `b0f6ac2`，并发存储启动修复 `42ec24e`
- 环境：macOS 本机 PostgreSQL 18.4、Redis、Kafka、临时 MinIO、Go 1.26.5 API/operation-worker、Node.js 24.19.0 Next.js standalone、agent-browser 0.33.0；未使用 Docker
- 数据集：页面内置三集中文 TXT 测试剧本；仅含自建测试内容，不含第三方生产内容
- 当前 Swagger SHA-256：`a794e4d4b4a58ff2168d9a6bc2dfd19f798277a7a9edb9b2a5ea622c2e5e602f`
- 当前 Schema SHA-256：`36dc2febce98f7bd2a87361782e18ad387c8d116ef700bfdc31982b334f89a07`
- 关联：`IAM-NFR-001`、`PRJ-FR-005/006` 与 `PRJ-NFR-001` 的本证据子场景、M02 Design 的项目列表 Query、`PRD-A-FR-001`、`PRD-A-AC-008` 的当前租户子场景、`PLAN-A A1/A9`、`EPK-A-TENANCY/MANUAL-E2E`、`ACC-GATE-003/005/009`
- 自动化测试：`backend/tests/scripts/project_list_integration_test.go`、`frontend/tests/unit/script-analysis-workspace.test.tsx`

## 前置条件

- 从唯一当前 Schema 初始化隔离数据库；复用本机 Redis 和 Kafka，不重建或删除共享基础设施。
- MinIO 使用独立临时目录、测试 Bucket 与 `19410/19411`；API 使用 `8690`，Next.js production standalone 使用 `8124`。
- 项目恢复入口只接收服务端项目列表返回的 Project/SourceRevision/Operation，客户端再次验证 UUID 格式和 Project 绑定；不从 localStorage 恢复 Workspace、Token 或剧本正文。
- 验收结束后关闭浏览器、API、Worker、Next.js 和 MinIO，删除隔离数据库；临时 MinIO 目录移入系统废纸篓。四个专用端口均已确认释放。

## 执行命令与步骤

```text
cd backend
go test ./... -count=1
go vet ./...
DATABASE_URL=<隔离 PostgreSQL> LANVERSE_INTEGRATION=1 \
  go test ./tests/scripts -run TestListWorkspaceProjectsReturnsOnlyTenantAndLatestRestorableWorkflow -count=1 -v
make swagger

cd frontend
OPENAPI_SCHEMA_URL=../backend/docs/swagger.json npm run openapi2ts
npm test -- --run
npm run lint
npm run typecheck
NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:8690 npm run build
HOSTNAME=127.0.0.1 PORT=8124 node .next/standalone/server.js

# API 与 operation-worker 对同一个全新 MinIO Bucket 同时启动
agent-browser --session lanverse-project-browser-final open http://127.0.0.1:8124
agent-browser --session lanverse-project-browser-final snapshot -i
# 注册测试身份，命名项目，提交三集剧本，等待 Worker，批准事实
agent-browser --session lanverse-project-browser-final open http://127.0.0.1:8124/
# 确认 URL 无 project/revision/operation，点击服务端项目列表中的继续入口
agent-browser --session lanverse-project-browser-final snapshot -i
agent-browser --session lanverse-project-browser-final a11y --json
agent-browser --session lanverse-project-browser-final vitals --json
agent-browser --session lanverse-project-browser-final errors --json
agent-browser --session lanverse-project-browser-final console --json
agent-browser --session lanverse-project-browser-final close
```

## 实际结果与逐项断言

1. `GET /api/workspaces/{workspaceID}/projects` 按当前 Workspace、`created_at/id` 稳定倒序分页，返回 `items/total/page/page_size`；每页范围为 1—100，默认 20。
2. PostgreSQL 集成测试建立当前 Workspace 的可恢复项目、空项目和另一个 Workspace 的项目；查询只返回当前租户的 2 项，空项目不伪造工作流，其他租户项目零命中。
3. 最新可恢复工作流只在 Operation、SourceRevision 和 Outbox payload 的 `workspace_id/project_id/operation_id/revision_id` 四元组完全一致时出现；不是按客户端缓存或“最新项目”猜测来源。
4. production 浏览器从空 Workspace 创建“最终跨设备剧本项目”，完成真实 Outbox/Kafka/operation-worker 解析和人工批准。数据库复核为 `projects=1|source=approved|operation=succeeded|binding=true|content_units=3|scenes=3|entities=11`。
5. 随后直接打开无任何查询参数的根 URL；HttpOnly refresh 恢复会话，项目列表显示总数、页码和该项目。点击“继续解析”后 URL 重新绑定同一三元组，项目名称恢复，页面回到“事实已批准”和 3 集正式事实。
6. 项目为空时显示明确 empty；项目列表有 loading、失败与重试；已有无工作流项目可被选择后导入新版本，新建项目入口不会覆盖已有项目。
7. 首次最终验收中，API 与 Worker 并发初始化同一空 Bucket，其中 API 将 `BucketAlreadyOwnedByYou` 误判为失败并退出。修复后在另一个全新 MinIO 数据目录以同一时序重跑，API 和 Worker 同秒启动并持续运行；仅同账户已创建结果被接受，其他建桶错误继续失败关闭。
8. 最终 production 页面 axe-core 4.12.1 为 `violations=0`、`incomplete=0`、`passes=37`；项目分页使用具名 navigation landmark。页面 `errors=[]`、`console=[]`。
9. Core Web Vitals 单次本机采样为 CLS 0、FCP 56 ms、LCP 76 ms、TTFB 2.3 ms；LCP 为本地 `h1` 且无资源 URL。

## Red → Green 与恢复结果

- RED Go 测试因 `ListProjects`/`ProjectQuery` 不存在而编译失败；RED 前端测试无法找到“继续已有项目”。
- Green 后真实 PostgreSQL 测试固定租户过滤、分页顺序、空项目和最新工作流绑定；前端测试固定无预存 URL 的服务端项目恢复与项目名称回填。
- production 浏览器发现的 MinIO 建桶竞态无法由无外部依赖的单元测试忠实重现，因此以两个隔离 MinIO 空目录执行同一并发启动时序作为 Red/Green 故障证据；修复后仍执行全部 Go 测试与 vet。
- 第一次分页 production 审计发现无角色 `div` 带 `aria-label`；改为具名 `nav` 并重建后 axe incomplete 从 1 降为 0，没有增加忽略规则。

## 偏差、残余风险和后续动作

- 当前项目列表只展示项目名称、创建时间和最新一次剧本解析工作流；M02 要求的 lifecycle、type、current Brief、blocked/unknown/stale、projection `as_of` 和可下钻项目概览尚未实现。
- 当前只验收第一页和一个项目；接口与 UI 已具备分页状态及上一页/下一页，但试点项目规模、深分页性能和并发新增导致的 offset 变化仍需容量基线。
- 集成测试验证另一个 Workspace 项目不返回，production 浏览器没有使用第二真实租户执行已知 ID 攻击；完整 `AC-PRJ-005`、`PRD-A-AC-008` 和 RLS 非 owner 套件继续为 `not_run`。
- 本数据集不满足正式 DS-A-SCRIPT；项目 Brief、生命周期、ContentUnit 重排、复制、投影重建和完整 A9/A10 旅程尚未验收，切片 A 继续保持 `not_run`。
