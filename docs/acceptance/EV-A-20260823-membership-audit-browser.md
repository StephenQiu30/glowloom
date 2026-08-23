# 成员治理、访问审计检索与生产会话恢复

- Evidence ID：`EV-A-20260823-membership-audit-browser`
- 结论：`passed`（覆盖当前 Workspace 成员角色变更、事务内审计、审计列表/筛选/分页、刷新会话恢复和管理页无障碍；不代表 M01、切片 A 或 PRD-A-AC-008 全部通过）
- 执行时间与时区：2026-08-23，Asia/Shanghai
- 执行人 / 复核人：Codex / 待复核
- Git commit：`994df16`、`003c6b8`、`b45a565`、`5aa4bf3`
- 环境：macOS 本机 PostgreSQL 18.4、Redis、临时 MinIO、Go 1.26.5 API、Node.js 24.19.0 管理端 production preview、agent-browser 0.33.0；未使用 Docker
- 数据集：隔离 Workspace 内的管理员和待处置成员各一名、21 条当前 Workspace 历史审计、1 条外部 Workspace 审计，均为本次创建的合成数据，不含第三方内容或生产数据
- 当前 Swagger SHA-256：`9d48ee5d1bfbf995610d94193cbfa03d77ffdceb04a4d92f2ac5f54d024d1553`
- 当前 Schema SHA-256：`36dc2febce98f7bd2a87361782e18ad387c8d116ef700bfdc31982b334f89a07`
- 关联：`IAM-FR-005/009`、`IAM-NFR-001/005`、`AC-IAM-002/008` 的本证据子场景、M01 Design 的 `ChangeMembershipRole`/`ListAccessAudit`/事务回滚路径、`PRD-A-FR-001`、`PRD-A-AC-008` 的审计子场景、`PLAN-A A1`、`EPK-A-TENANCY`、`ACC-GATE-006/009`
- 自动化测试：`backend/tests/architecture/audit_contract_test.go`、`backend/tests/identity/middleware_test.go`、`backend/tests/identity/repository_integration_test.go`、`backend/tests/identity/service_test.go`、`admin/tests/unit/services/admin.test.ts`、`admin/tests/unit/theme-accessibility.test.ts`

## 前置条件

- 从唯一当前 `backend/schema/current.sql` 初始化隔离数据库 `lanverse_audit_browser_20260823_01`，复用本机 Redis；临时 MinIO 只使用独立目录和 `19320/19321` 端口。
- API 使用 `8692`，管理端 production preview 使用 `8132`；浏览器只操作测试身份，不把 Cookie、JWT、密码、对象键或 `.env` 内容写入证据和 Git。
- 访问审计查询显式接收当前 Workspace，Repository 在 Workspace 事务中执行并固定 `events.workspace_id = ?`；外部 Workspace 的已知事件不参与总数和结果。
- 验收结束后关闭全部专用 agent-browser、API、管理端和 MinIO，删除隔离数据库；临时 MinIO 目录和失响应浏览器会话配置移入系统废纸篓，四个端口均确认释放。

## 执行命令与步骤

```text
cd backend
go test ./... -count=1
go vet ./...
DATABASE_URL=<隔离 PostgreSQL> LANVERSE_INTEGRATION=1 \
  go test ./tests/identity -run 'TestWorkspaceMemberChangeWritesRestorableAuditInSameTransaction|TestIdentityRepositoryIntegration/ListAccessAudit' -count=1

cd admin
pnpm run openapi
pnpm run test
pnpm run lint
pnpm exec antd lint ./src
API_BASE_URL=http://127.0.0.1:8692 pnpm run build
pnpm exec max preview --port 8132

agent-browser --session lanverse-audit-query open http://127.0.0.1:8132/admin
agent-browser --session lanverse-audit-query wait --load networkidle
agent-browser --session lanverse-audit-query snapshot -i
# 将成员角色 user → ban，理由为空时确认按钮不可用，填写理由后提交
# 检查新审计立即位于首行；翻到第 2 页后只出现第 21 条当前 Workspace 历史记录
# 组合填写关键字、主体、对象、结果和同日时间范围，查询收敛为真实角色变更审计
# 重置后恢复 20 条首屏记录和第 2 页；reload 后仍停留 /admin
agent-browser --session lanverse-audit-query a11y --json
agent-browser --session lanverse-audit-query vitals --json
agent-browser --session lanverse-audit-query errors --json
agent-browser --session lanverse-audit-query console --json
agent-browser --session lanverse-audit-query close
```

## 实际结果与逐项断言

1. 管理员选择新角色后看到前后角色和审计理由；理由为空时不能提交。提交后成员从 `user` 变为 `ban`，最新 `iam.membership.updated` 审计无需刷新即出现在首行。
2. 成员变更、当前 Workspace 会话撤销和 AuditEvent 位于同一 PostgreSQL 事务；审计插入故障注入会让业务变更整体回滚，不留下失败请求残行。
3. 新审计完整显示主体、对象、动作、`before={role:user,status:active}`、`after={role:ban,status:active}`、理由、`succeeded`、request ID 和时间；数据库不保存凭据、完整令牌或无关正文。
4. 21 条预置当前 Workspace 审计分成两页：第 1 页 20 条，第 2 页仅第 21 条；真实变更写入后总数为 22。预置的 1 条外部 Workspace 审计没有出现在列表和分页总数中。
5. 关键字、主体、对象与 `succeeded` 结果筛选收敛为真实角色变更审计；同日开始/结束时间范围与关键字、主体、对象组合提交后也只返回该记录。重置恢复空筛选、20 条首屏记录和两页分页。
6. Service 对非管理员、非法结果、反向时间范围、超长筛选和分页边界失败关闭；Controller 精确传播 RFC3339 时间、主体、对象、动作、结果和分页，管理端统一通过带 refresh 恢复的 `apiRequest` 调用生成契约。
7. 数据库最终复核为 `role_ban=true|current_audits=22|foreign_audits=1|single_actual=true|actor_bound=true|target_bound=true|before_valid=true|after_valid=true|audit_complete=true`；前后 SHA-256 均为 64 字符且 request ID 非空。
8. production `reload` 后仍停留 `/admin`，通过 HttpOnly refresh cookie 恢复内存 Access Token；成员仍为“已封禁”，最新审计和分页数据仍可读取。
9. 首轮 axe-core 4.12.1 检出 1 类严重颜色对比度违规、涉及 62 个重复节点。新增集中式无障碍主题令牌和 WCAG AA 契约测试后，复验为 `violations=0`、`incomplete=1`、`passes=53`；唯一 incomplete 是渐变/覆盖元素使 axe 无法自动确定背景，并非已确认违规。
10. production 页面 `errors=[]`、`console=[]`；单次本机采样为 CLS 0.09、FCP 416 ms、LCP 416 ms、TTFB 0.3 ms，LCP 为本地 `div` 且无远程资源 URL。

## Red → Green 与故障恢复

- RED Repository 集成测试先固定 Workspace 隔离、主体/对象/动作/结果/时间组合过滤、排序和分页；RED Controller/Admin 测试固定 query 传播，初始分别因缺少 `ListAccessAudit` 和 `listAccessAudit` 失败。
- Green 后 Repository 使用显式 Workspace 谓词、稳定倒序和分页；Service 负责管理员门禁与输入规范化；Controller、Swagger、生成客户端、管理端筛选表格共同使用当前合同。
- 首轮 production axe 扫描暴露严重对比度缺陷；RED `theme-accessibility.test.ts` 固定正文、占位符、主按钮和成功状态的 4.5:1 门槛，Green 后 production axe 已确认违规归零。
- agent-browser 0.33.0 在 Ant Design RangePicker 浮层完成同日范围后，对被浮层覆盖的普通 click 等待会出现会话级失响应；API、页面错误和控制台均保持正常。验收使用同一浏览器上下文触发实际查询按钮的原生 click，并用返回表格、数据库和自动化时间传播测试交叉验证，没有把超时报告为通过。

## 偏差、残余风险和后续动作

- 本证据没有覆盖项目职责、服务身份、媒体签名、临时 Grant、SSO/SCIM、紧急撤销批次或限时只读恢复，因此其余适用的 `AC-IAM-001—007` 与完整 `AC-IAM-002` 继续为 `not_run`。
- `ListAccessAudit` 的当前列表、关键字/主体/对象/动作/结果/时间筛选和分页已实现；事故批次、撤销传播剩余项和恢复审批仍未实现。
- production 浏览器只执行 `user → ban`；`suspended/removed`、最后一个 active Admin 保护和并发管理员变更继续由自动化测试覆盖，完整 M01 数据集仍需人工复核。
- axe 的 1 个 incomplete 需要在完整键盘/高对比主题验收中人工复核；当前没有已确认的 WCAG 违规。
- 该合成数据集不是正式 DS-A-SCRIPT/DS-A-MEDIA，未涉及剧本内容；切片 A 验收矩阵继续保持 `not_run`。
