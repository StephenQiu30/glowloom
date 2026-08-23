# 成员角色变更、事务审计与生产会话恢复

- Evidence ID：`EV-A-20260823-membership-audit-browser`
- 结论：`passed`（只覆盖当前 Workspace 成员角色变更、事务内审计、管理端会话恢复和本页可访问性场景，不代表 M01、切片 A 或 PRD-A-AC-008 全部通过）
- 执行时间与时区：2026-08-23，Asia/Shanghai
- 执行人 / 复核人：Codex / 待复核
- Git commit：`994df16`
- 环境：macOS 本机 PostgreSQL 18.4、Redis、临时 MinIO、Go 1.26.5 API、Node.js 24.19.0 管理端 production preview、agent-browser 0.33.0；未使用 Docker
- 数据集：隔离 Workspace 内的管理员与待处置成员各一名，均为本次创建的合成测试身份，不含第三方内容或生产数据
- 当前 Swagger SHA-256：`823d609a2a6c9292c933c1f5f08a5329eb13e3fde2c7cbe5c9b5f8730d3da61f`
- 当前 Schema SHA-256：`36dc2febce98f7bd2a87361782e18ad387c8d116ef700bfdc31982b334f89a07`
- 关联：`IAM-FR-005/009`、`IAM-NFR-001/005`、`AC-IAM-002/008` 的本证据子场景、M01 Design 的 `ChangeMembershipRole`/`AccessAudit`/事务回滚路径、`PRD-A-FR-001`、`PRD-A-AC-008` 的审计子场景、`PLAN-A A1`、`EPK-A-TENANCY`、`ACC-GATE-006/009`
- 自动化测试：`backend/tests/architecture/audit_contract_test.go`、`backend/tests/identity/middleware_test.go`、`backend/tests/identity/repository_integration_test.go`、`backend/tests/identity/service_test.go`、`admin/tests/unit/services/*.test.ts`、`admin/tests/unit/shell-accessibility.test.tsx`

## 前置条件

- 从唯一当前 `backend/schema/current.sql` 初始化隔离数据库 `lanverse_admin_audit_20260823_01`，复用本机 Redis；临时 MinIO 仅使用独立目录和 `19300/19301` 端口。
- API 使用 `8688`，管理端 production preview 使用 `8130`；浏览器只操作测试身份，不把 Cookie、JWT、密码、对象键或 `.env` 内容写入证据和 Git。
- 管理端 OpenAPI 客户端由当前 Swagger 重新生成；手写身份会话代码位于生成目录之外，生成器独占 `services/ant-design-pro/`。
- 验收结束后关闭专用 agent-browser、API、管理端和 MinIO，删除隔离数据库；临时 MinIO 目录移入系统废纸篓。四个验收端口均已确认释放。

## 执行命令与步骤

```text
cd backend
go test ./... -count=1
go vet ./...
DATABASE_URL=<隔离 PostgreSQL> LANVERSE_INTEGRATION=1 \
  go test ./tests/identity -run TestWorkspaceMemberChangeWritesRestorableAuditInSameTransaction -count=1

cd admin
pnpm run openapi
pnpm run test
pnpm run lint
pnpm exec antd lint ./src
API_BASE_URL=http://127.0.0.1:8688 pnpm run build
pnpm exec max preview --port 8130

agent-browser --session lanverse-admin-audit-v2 open http://127.0.0.1:8130/admin
agent-browser --session lanverse-admin-audit-v2 wait --load networkidle
agent-browser --session lanverse-admin-audit-v2 snapshot -i
# 将待处置成员的角色从 user 改为 ban；确认弹窗在理由为空时不可提交
# 填写明确理由后提交，等待成员列表显示“已封禁”
agent-browser --session lanverse-admin-audit-v2 reload
agent-browser --session lanverse-admin-audit-v2 snapshot -i
agent-browser --session lanverse-admin-audit-v2 a11y --json
agent-browser --session lanverse-admin-audit-v2 vitals --json
agent-browser --session lanverse-admin-audit-v2 errors --json
agent-browser --session lanverse-admin-audit-v2 console --json
agent-browser --session lanverse-admin-audit-v2 close
```

## 实际结果与逐项断言

1. 管理员选择新角色后先看到“确认成员权限变更”弹窗、前后角色和审计理由输入框；理由为空时“确认变更”不可用，填写理由后才发出当前 `PATCH /api/admin/members/{membership_id}` 请求。
2. 服务端不信任客户端 request ID：HTTP 中间件生成/规范化 request ID，Controller 将其和理由一并传入 Service/Repository；空理由、空 request ID、无效 actor 在写库前失败关闭。
3. 成员 `user → ban`、该 Workspace 下会话撤销和 `iam.membership.updated` AuditEvent 位于同一个 PostgreSQL 事务。数据库复核结果为 `target_role=ban|audit_count=1|actor_match=true|tenant_match=true|before=true|after=true|reason=true|result=true|request_bound=true|hashes=true`。
4. AuditEvent 保存操作者、Workspace、对象、`before_state={role:user,status:active}`、`after_state={role:ban,status:active}`、理由、结果、请求关联和互不相同的 SHA-256；不保存凭据、完整令牌或成员无关正文。
5. 集成测试注入拒绝 AuditEvent INSERT 的 PostgreSQL Trigger；命令返回错误，成员保持 `ban`，失败请求没有审计残行，证明审计写失败时权限变更整体回滚。
6. 管理端刷新后仍停留在 `/admin`，通过 HttpOnly refresh cookie 主动恢复内存 Access Token；待处置成员仍显示“已封禁”。Axios 使用 `withCredentials`，不再使用无效的 Fetch `credentials` 选项。
7. OpenAPI 重新生成后删除旧的手写 `api.ts/login.ts`，当前生成目录按后端 tag 输出；身份会话适配器移到 `services/identity.ts`，避免下一次生成覆盖业务代码。
8. production 页面 axe-core 4.12.1 结果为 `violations=0`、`incomplete=1`、`passes=49`；唯一 incomplete 为渐变/覆盖元素导致无法自动计算的颜色对比度，没有已确认违规。页面 `errors=[]`、`console=[]`。
9. Core Web Vitals 单次本机采样为 CLS 0.07、FCP 392 ms、LCP 392 ms、TTFB 0.5 ms；LCP 为本地 `div` 且无远程资源 URL。管理端已删除模板 GA、远程字体、远程背景和远程 Logo，修复语言、viewport、重复导航、Footer landmark 与已确认的颜色对比度问题。

## Red → Green 与故障恢复

- RED Schema 契约测试首先固定 `before_state`、`after_state`、request ID、理由和结果为不可缺失的当前审计合同；Service/Controller 测试固定显式理由和服务端 request ID 传播。
- RED PostgreSQL 集成测试固定“成员变更成功但审计失败”不得出现；Green 后以同事务写入审计，并通过 Trigger 故障注入验证整体回滚。
- production 首次 reload 因 Axios 使用错误的 `credentials` 选项而回到登录页；修复为 `withCredentials` 后又固定无内存 Token 时的主动 refresh，最终重载保持 `/admin`。
- 首次 production axe 审计为 6 个违规，逐项清理模板外部资源、导航/landmark/viewport/语言和文本对比度后降为 0 个已确认违规；没有用忽略规则隐藏结果。

## 偏差、残余风险和后续动作

- 本证据没有覆盖邀请、并发接受邀请、项目职责、服务身份、跨 Workspace 已知 ID、媒体签名、临时 Grant、SSO/SCIM、紧急恢复或完整撤销传播，因此 `AC-IAM-001—007` 与完整 `AC-IAM-002` 继续为 `not_run`。
- 当前只证明角色变化写入不可变审计事实；`ListAccessAudit` 查询页、筛选、分页和事故批次尚未实现，管理员暂时不能在 UI 中检索历史审计。
- production 浏览器只执行 `user → ban`；状态 `suspended/removed`、最后一个 active Admin 保护和并发管理员变更由自动化测试覆盖的范围仍需纳入完整 M01 验收数据集。
- 该合成数据集不是正式 DS-A-SCRIPT/DS-A-MEDIA，未涉及剧本内容；切片 A 验收矩阵继续保持 `not_run`。
