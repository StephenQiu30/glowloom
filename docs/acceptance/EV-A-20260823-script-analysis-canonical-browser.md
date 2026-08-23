# 剧本解析 Canonical 物化与会话恢复闭环

- Evidence ID：`EV-A-20260823-script-analysis-canonical-browser`
- 结论：`passed`（只覆盖本文列出的剧本解析目标场景，不代表切片 A 或 PRD-000 全部通过）
- 执行时间与时区：2026-08-23，Asia/Shanghai
- 执行人 / 复核人：Codex / 待复核
- Git commit / 制品 digest：实现提交 `1755e2d`；证据文档由后续 docs 提交固化
- 环境：本机 PostgreSQL 17、Kafka、Redis、临时 MinIO、Go API/operation-worker、Next.js production build；未使用 Docker
- 数据集：仓库内测试专用三集中文剧本，内容 SHA-256 `299d9db607a2b79db8602bacb1780b83bfeb61fd68a05d2043aebd90899be307`，不含第三方生产内容
- 关联：REQ-M01/M02/M03/M04、DES-004/005/006/007、PRD-A-FR-001/002、PLAN-A A0—A5 的剧本解析目标子场景
- 自动化测试：`backend/src/identity/repository_integration_test.go`、`backend/src/scripts/model_test.go`、`backend/tests/architecture/schema_contract_test.go`、`frontend/tests/unit/script-analysis-workspace.test.tsx`

## 前置条件

- 复用本机 PostgreSQL、Kafka、Redis，不重建 Docker 环境。
- 使用独立数据库 `lanverse_codex_20260823` 和临时 MinIO 端口 `19100`，避免读写用户已有业务库和对象。
- API、Worker 和前端仅使用测试专用配置；没有读取或记录本机真实凭据。

## 执行命令

```text
cd backend
LANVERSE_INTEGRATION=1 DATABASE_URL=postgres://postgres@127.0.0.1:5432/lanverse_codex_20260823 go test ./... -count=1
go vet ./...
make swagger

cd frontend
OPENAPI_SCHEMA_URL=../backend/docs/swagger.json npm run openapi2ts
npm run lint
npm run typecheck
npm test -- --run
npm run build
npm start

cd agent
uv run pytest

cd admin
pnpm run lint
pnpm test -- --run
pnpm run build

agent-browser --session lanverse-prod-20260823 open http://127.0.0.1:8123/
agent-browser --session lanverse-prod-20260823 snapshot -i
agent-browser --session lanverse-prod-20260823 a11y
agent-browser --session lanverse-prod-20260823 errors
agent-browser --session lanverse-prod-20260823 console
```

另在一次性空库 `lanverse_acceptance_20260823_1605` 执行 `LANVERSE_ROLE=schema-init go run ./cmd`；首次成功，第二次因已有 59 张表被正确拒绝，核对完成后已删除该临时库。

## 实际结果与断言

1. OpenAPI 与前端生成链连续执行两次，生成差异 SHA-256 均为 `86592f3ab05a6a7525e7963931b030277b5ab26c347c7e34beb358c62dfdc1e0`，无二次漂移。
2. Go 全仓测试（包含真实 PostgreSQL/GORM 集成）和 `go vet ./...` 通过；Next lint、typecheck、4 个 Vitest、production build 通过；Python 2 个测试通过；Admin lint/typecheck、39 个测试和 build 通过。
3. 空库初始化得到 59 张 public 表和 11 条 `tenant_isolation` 策略；存在 `nar_source_revisions`、`nar_analysis_drafts`，不存在旧 `script_revisions`。
4. production 浏览器完成：邮箱登录 → 创建 Project → MinIO 保存 SourceRevision → PostgreSQL Outbox → Kafka → operation-worker → Analysis Draft → 人工批准。
5. 页面显示 3 集、3 场景、2 人物、11 个生产资产和“事实已批准”。数据库中对应 SourceRevision/Draft 均为 `approved`，且有 3 个 `prj_content_units`、3 个 `nar_scenes`、11 个 `pk_entities`、11 个 `pk_production_requirement_items`。
6. JWT 密钥轮换后，浏览器实际观察到业务请求 401、`POST /api/auth/refresh` 200、原请求自动重试 201，之后仍完成解析和批准。
7. Access Token 只存在于前端内存；浏览器 LocalStorage 长度为 0。退出登录返回 204，随后重载时 refresh 返回 401，页面保持登录入口。
8. axe-core 4.12.1：登录页 `violations=0`、`incomplete=0`、`passes=32`，已认证结果页 `0/0/35`；production console 和 page errors 均为空。

## 缺陷与修复

- `ISSUE-001`：GORM 将项目鉴权表错误推导为 `workspace_project_records`，且嵌套来源仍引用旧 `script_revisions`，导致创建剧本版本返回 404。已显式映射 `projects`、切换 `nar_source_revisions` 并增加 PostgreSQL 回归测试。
- `ISSUE-002`：失效 JWT 后页面不可恢复且 Token 写入 LocalStorage。已改为内存 Token、HttpOnly refresh 恢复、401 单航班刷新重试和显式 logout，并增加前端测试及真实密钥轮换验证。
- `ISSUE-003`：登录页认证方式容器的 `aria-label` 缺少允许该属性的语义角色。已增加 `role="group"`，production 登录页 axe 扫描恢复为 0 violations / 0 incomplete。

浏览器证据保存在本机 `/tmp/lanverse-dogfood-20260823/`，包括失败复现、草稿、批准结果、密钥轮换恢复截图和视频；其中不保存 Cookie、JWT、密码或真实凭据。

## 偏差、残余风险和后续动作

本证据只证明当前“整本纯文本剧本解析与批准物化”目标场景。PRD-A 与 PLAN-A 仍是 `proposed`，且其入口 Gate 所需的签认数据集、DOCX/Markdown ParseReport、边界手工修订、完整 Mention 决议、approved 私有检索、10+ Shot、Fixture Animatic、跨租户/媒体/Worker 负向矩阵、Elastic/OTel 故障恢复尚未全部实现或执行；切片 A 因此仍为 `not_run`。B—F 同样不得据此启动或宣称通过。
