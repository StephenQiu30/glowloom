# Workflow 公共 Human Gate 命令与恢复设计

- 状态：待用户接受；接受前不派生 Requirement、Plan 或编码
- 日期：2026-08-26
- 基线 Design：[后端领域模块功能设计](2002-后端领域模块功能设计.md) · [前端功能模块设计](1002-前端功能模块设计.md)
- 基线 PRD：[产品范围与验收基线](../prd/0001-产品范围与验收基线.md)
- 基线 Requirement：[后端领域服务与生产闭环需求规格](../requirement/2002-后端领域服务与生产闭环需求规格.md) · [前端功能模块需求规格](../requirement/1002-前端功能模块需求规格.md)
- 前置 Acceptance：[人工任务续租与释放](../acceptance/2013-Workflow人工任务续租与释放验收记录.md) · [人工任务过期回收](../acceptance/2014-Workflow人工任务过期回收验收记录.md) · [人工信号协调](../acceptance/2011-Workflow人工信号协调验收记录.md)

## 1. 问题与当前事实

Backend 内部已经具备以下独立事实与用例：

- `review` 唯一拥有 `HumanTask`、Claim/Renew/Release/Expire 和不可变 `ReviewDecision`；
- Production、Planning、Storyboard 与 Generation 分别通过自己的 Application Service 产生 Owner Command Receipt；
- `workflow.SignalService` 在 PostgreSQL 重新核对 Task、Decision、候选、Owner Receipt 与 `node-output-v1` 后，使用官方 Temporal Client 发送一次幂等 Signal；
- Signal 结果未知时，既有 Intent/Receipt 可以用相同身份继续对账。

但 `lanverse-api` 当前只公开 Workflow Start/Get/Rerun/Control，没有装配 Review Service、HumanTask Query、Claim、Decision 或 Signal 恢复。Frontend 因而无法从真实 Run 发现待审核任务，也不能在刷新或进程中断后继续 `Decision → Owner Apply → Temporal Signal`。如果只把 `ReviewService.Decide` 暴露成 HTTP，Decision 已提交而 Owner Apply 或 Signal 未完成时，浏览器刷新后将没有稳定恢复入口；如果由 Handler 直接访问 GORM 或拼接各 Owner，则会形成第二套审核协调逻辑。

## 2. 目标与非目标

### 2.1 目标

1. 提供项目范围内可发现、可重取的 HumanTask 公共查询。
2. 提供 Claim、Renew、Release，并保留现有 Revision、Claim Token 与 Lease Fencing。
3. 提供一次客户端 Decision 命令，Backend 依次完成不可变 ReviewDecision、Owner Apply Receipt 和 Temporal Signal。
4. 提供只依赖已持久化 Decision ID 的恢复命令，使浏览器刷新、API 重启和 Signal UNKNOWN 后可以继续同一业务效果。
5. 让 Frontend 可以区分 Task 生命周期、Decision 事实和 Workflow Resume 状态，不能把“Decision 已记录”显示成“Workflow 已继续”。

### 2.2 非目标

- 本切片不实现 SSE/Event Store、通用通知、任务指派、批量审批或运营后台。
- 本切片不复制 Production/Planning/Storyboard/Generation 的候选详情、QC、Artifact 或成本事实；Task 只返回冻结 Subject/Candidate 引用，Frontend 通过对应 Owner Query 读取证据。
- 本切片不新增通用 Saga 框架、Repository Registry、反射式 Owner 路由或 Handler 内业务分支。
- 本切片不实现 Run Observability 页面、Run 列表、真实图片 Provider、Episode 动态 Shot 扇出或最终 `agent-browser` 验收。
- 本切片不把 ReviewDecision 回滚、覆盖或删除；确定性 Owner Apply 冲突必须成为可解释的 Needs Attention，而不是改写 Decision。

## 3. 模块边界与组合根

```text
HTTP Review Adapter
  → Review Application（Query / Claim / Renew / Release / Decide）
  → Workflow HumanGateDecisionCoordinator
       → Review Application.GetDecision
       → Workflow SignalService
            → 目标 Owner Application Applier
            → Workflow Signal Intent/Receipt Store
            → Temporal Signaler
```

- `review` 继续唯一写 HumanTask 与 ReviewDecision；公共 Handler 只能调用它的 Application Interface。
- `workflow` 新增很薄的 `HumanGateDecisionCoordinator`，只负责跨模块顺序和结果归一化，不保存第二份 Task/Decision，不直接访问 Review、Production、Generation 或 Asset 表。
- 现有 Production Applier 与 Generation Applier 通过显式 Composite 按已冻结 Executor 分派。Composite 只识别当前已发布 Catalog 的五类 Gate；未知 Executor fail closed，不建立动态 Service Locator。
- `lanverse-api` 组合根装配 Review、Production Bible、Planning、Storyboard 和 Generation Selection 所需的既有 Application Service，但不装配 Shot Binding Executor、不注册 Temporal Worker、不执行 Node Activity、不创建第二数据库连接。
- Backend 仍是唯一业务 Writer；Agent Runtime 和 Frontend 不获得数据库、Owner Apply 或 Signal 写权限。

## 4. 公共 HTTP 契约

所有路由使用现有 Bearer Access Token、统一 Problem Envelope、严格 JSON 解码和当前 Token Version/Membership 复核。资源不存在与跨 Workspace 无权统一为 404；Viewer 对写命令返回 403；Revision、Lease、Stale 与幂等漂移返回 409；输入协议错误返回 422；依赖不可用返回 503。

### 4.1 查询

```text
GET /api/v1/projects/{project_id}/human-tasks
  ?status=active|OPEN|CLAIMED|COMPLETED
  &limit=1..100
  &after={human_task_id}

GET /api/v1/human-tasks/{human_task_id}
```

- 列表默认使用 `status=active` 返回 `OPEN` 与 `CLAIMED`；其他值只返回对应单一状态。结果按 `created_at DESC, id DESC` 稳定排序，默认 50 条；`after` 指向上一页最后一个 Task，Backend 先在同一授权 Project 中读取锚点，再按锚点的 CreatedAt/ID 继续，不引入通用 Cursor 框架。
- 列表与详情返回 Task ID、Workspace/Project/WorkflowRun/NodeRun、Subject Type/ID/Revision、排序后的 Candidate IDs、Rubric Version、Status、Claimed By、Claim Expires At、Revision 和时间。
- `claim_token` 只在当前 Actor 就是未过期 Claim Owner 时返回；其他 Actor、列表日志、Problem、Trace 与审计不得泄漏。这样同一 Reviewer 刷新页面后可以继续已有 Claim，但 Token 不能单独绕过 Actor、Token Version、Membership、Revision 与 Expiry 校验。
- COMPLETED Task 的详情附带不可变 Decision 摘要，以及 `resume_status = pending | unknown | completed | conflict | needs_attention`。该状态从 ReviewDecision 与 Workflow Signal Intent/Receipt 分别读取后组合，不回写 HumanTask。

### 4.2 Claim 生命周期

```text
POST /api/v1/human-tasks/{human_task_id}/claims
POST /api/v1/human-tasks/{human_task_id}/claim-renewals
POST /api/v1/human-tasks/{human_task_id}/claim-releases
```

- Claim Body：`expected_revision`、`idempotency_key`。
- Renew/Release Body：`expected_revision`、`claim_token`、`idempotency_key`。
- 成功响应只返回 Backend 已提交的 Task；Claim/Renew 同时向当前 Actor 返回 Claim Token。
- 三个命令原样复用现有 Review Application 和 GORM Command Receipt，不在 HTTP 层重做状态机、过期接管或幂等判断。

### 4.3 Decision 与恢复

```text
POST /api/v1/human-tasks/{human_task_id}/decisions
POST /api/v1/review-decisions/{review_decision_id}/resume
```

Decision Body 固定为：

```json
{
  "claim_token": "uuid",
  "expected_task_revision": 2,
  "expected_subject_revision": 1,
  "decision": "approved | rejected | changes_requested | selected",
  "selected_candidate_id": "uuid-or-null",
  "idempotency_key": "client-stable-key"
}
```

处理顺序固定为：

1. Review Application 在单个 GORM/PostgreSQL 事务内重新授权、校验 Lease/Revision/Candidate，持久化唯一 ReviewDecision、完成 HumanTask 并写 Review Command Receipt。
2. Coordinator 只从第一步返回的 Task/Decision 派生 Signal 输入；不接受 Browser 自报 Workspace、Run、Node、Subject 或 Decision ID。
3. Signal 使用 `human-gate-decision:{review_decision_id}` 作为稳定幂等键，重新从 PostgreSQL 解析目标 Owner Application。
4. 正向决议先由对应 Owner 写真实 Command Receipt 和正式版本，再准备 Workflow Apply Receipt/Signal Intent；拒绝类决议不得产生 Owner 输出。
5. Temporal 返回 Signaled/AlreadyApplied 且 Input Hash 一致时，响应 `resume_status=completed`；结果未知时返回已持久化 Decision 和 `resume_status=unknown`，HTTP 使用 202，不伪报成功。

`resume` 不接受业务事实 Body。它先通过 Review Application 按 Decision ID 重新授权并读取不可变 Task/Decision，再用同一个稳定 Signal 幂等键重放 Owner Receipt/Apply Receipt/Temporal Signal。浏览器不需要保留第一次 Decision 的 Idempotency Key，也不能用 resume 改写 Decision、候选或目标 Run。

## 5. 状态与失败路径

| 已提交事实 | 外部观察 | 可执行动作 |
|---|---|---|
| Task OPEN/CLAIMED，无 Decision | `resume_status` 为空 | Claim/Renew/Release/Decide |
| Decision 已提交，Owner Apply 尚未成功 | `needs_attention` | 修复 Stale/Owner 阻塞后按 Decision ID resume |
| Apply/Signal Intent 已提交，Temporal 结果未知 | `unknown` | 按 Decision ID resume，只重放同一证据 |
| Signal 冲突 | `conflict` | 只读诊断，不能自动创建第二 Decision |
| Signal 完成 | `completed` | 重取 WorkflowRun，确认 Gate/下游投影 |

- API 在 ReviewDecision 提交之后失败时，响应必须携带已提交 Decision ID；不能返回一个让客户端误以为整条命令回滚的普通 500。
- Owner 确定性 Stale/Hash 冲突不删除 Decision。Coordinator 返回 409 与 `needs_attention`；后续只能在正式 Owner 事实恢复一致后 resume，或由未来显式运维修复流程处理。
- Signal UNKNOWN 不释放 Claim、创建第二 Decision、重做 Owner 生产效果或启动第二 Temporal Workflow。
- 并发 Decision 只有一个 Review 事务成功；并发 resume 最终收敛到同一 Owner Receipt、Apply Receipt、Signal Intent/Receipt 和 Gate 输出。

## 6. 数据与 SQL 事实源

本设计复用当前 `rev_human_tasks`、`rev_review_decisions`、Workflow Apply/Signal Intent/Receipt、Owner Command Receipt、WorkflowRun 与 NodeRun Projection。查询所需排序、Project/Status 和 Decision 关联使用现有 GORM Model 与索引；若实际 Explain 证明缺索引，只能在同一 GORM Model Catalog 正向增加索引标签并同时验证空库与现有库，不能创建 Migration 文件、DDL/Raw SQL、影子表或第二 ORM。

PostgreSQL/GORM 仍是唯一业务 SQL 事实源；Temporal History 仍是 Signal/Timer/Workflow 执行权威。公共查询组合多个 Owner 投影不使它们成为新的写入模型，也不允许投影回写 Owner。

## 7. Frontend MVP

- Review Workbench 首版只实现项目 Task 队列、Task 状态、Claim/Renew/Release、Decision、resume 和 WorkflowRun 重取。
- Task 的 Subject Type 映射到现有 Production Bible、Episode Plan、Episode Structure、Storyboard 与 Shot Generation Owner 页面；未知类型显示只读引用并禁用 Decision，不在前端猜测证据。
- Claim Token 只保存在当前受保护 Query 状态；不进入 URL、Local Storage、日志或埋点。刷新后由详情 Query 仅向同一 Claim Owner 恢复。
- `Decision recorded`、`Owner apply pending`、`Signal unknown`、`Workflow resumed` 使用不同文案和动作。只有 Backend 返回 `resume_status=completed` 且重取 WorkflowRun 看到 Gate 新状态后，UI 才显示工作流已继续。
- 本切片使用轮询和命令后 Query invalidation；SSE 去重与 Last-Event-ID 属于后续 Run Observability 设计，不能预建半套事件流。

## 8. Red → Green 与验收门

编码前派生 Requirement 和 Plan，并先建立以下失败测试：

1. 跨 Workspace/Project 查询统一 404，Viewer 写命令 403，Token Version 撤销后所有命令 fail closed。
2. Claim/Renew/Release 的 Revision、Token、Expiry、幂等重放和输入漂移通过真实 PostgreSQL。
3. Decision 成功而进程在 Owner Apply 前/后、Signal Intent 前/后、Temporal 返回前后退出，按 Decision ID resume 只产生一次业务效果。
4. Production Bible、Episode Plan、Episode Structure、Storyboard Set 和 Generation Selection 五类现有 Gate 均走显式 Owner Applier；未知 Executor 不进入兼容分支。
5. Signal UNKNOWN、AlreadyApplied、Input Hash 冲突、Stale Owner、重复/并发 Decision 与 resume 都有独立状态和 Problem。
6. OpenAPI 与生成 Frontend Client 无漂移；Frontend 组件测试证明不会把 Decision 已记录显示为 Workflow 已继续。
7. 全新 PostgreSQL、真实 Temporal、API 重启与 Worker 重启完成 `Claim → Decision → Owner Receipt → Signal UNKNOWN → Resume → Gate SUCCEEDED → 下游继续`。
8. Backend、Agent、Frontend、Compose、镜像与仓库卫生完整 CI 通过后记录 Acceptance；全部项目开发完成前不运行 `agent-browser`。

## 9. 接受后的实施顺序

1. PRD/Requirement/Plan 只补充本设计新增的公共边界和恢复语义。
2. Backend 先交付 Query/Claim/Renew/Release HTTP，再交付 Decision Coordinator、显式 Owner Composite 与 resume。
3. Frontend 接入项目 Task 队列和 Human Gate 状态，不在 Backend API 未完成时模拟本地成功。
4. 每个可独立验收的纵向任务完整通过 CI 后提交一次 Git；最终浏览器验收仍等待全部开发完成。
