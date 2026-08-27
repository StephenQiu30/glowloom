# Workflow 公共 Human Gate 命令与恢复设计

> 状态：StoryGraph Human Gate 复核已接受（`SG-D16`，2026-08-27）；Backend 公共 HTTP、既有五类 Owner 路由和 Review Workbench 已于 2026-08-27 实现，新 StoryGraph Gate 仍按唯一实施队列逐项交付
>
> 基线：[后端领域模块功能设计](2002-后端领域模块功能设计.md) · [前端功能模块设计](1002-前端功能模块设计.md) · [StoryGraph 内容图与 DAG 创作画布设计](0010-StoryGraph内容图与DAG创作画布设计.md)
>
> 历史证据：[人工信号协调](../acceptance/2011-Workflow人工信号协调验收记录.md) · [人工栅栏输入与决议绑定](../acceptance/2021-Workflow人工栅栏输入与决议绑定验收记录.md) · [生产回执与人工栅栏输出](../acceptance/2023-Workflow生产回执与人工栅栏输出验收记录.md)；只证明既有内部切片，不抵扣 StoryGraph 新链路验收

## 结论

公共 Human Gate 固定为三个可独立持久化、查询和恢复的阶段：`ReviewDecision recorded → Owner Apply completed/not_required → Workflow Resume completed`。Review 唯一拥有 HumanTask 与不可变 ReviewDecision；各领域 Owner Application 唯一拥有正式业务效果和 Command Receipt；Workflow 只协调已持久化证据并通过 Temporal Signal 恢复等待节点。Frontend、Agent、Kafka 和 Handler 都不能跨过这三个 Owner。

公共 API 只接受 Task 上已冻结 Subject 的决议，不接受客户端自报 Workspace、Run、Node、候选集合或 Owner 输出。每种正向决议必须路由到显式 Owner Command；拒绝/要求修改没有 Owner 写入，但仍以已持久化 Decision 恢复 Workflow 的拒绝/修复分支。任何未知结果都以同一个 ReviewDecision ID 对账，不创建第二 Decision、不盲目重做 Owner 效果、不启动第二 Workflow。

## 1. 问题与当前事实

当前 Backend 已有：

- `review` 领域的 HumanTask、Claim/Renew/Release/Expire 与不可变 ReviewDecision；
- Workflow Run/NodeRun、Gate Input/Output、Apply Receipt、Signal Intent/Receipt；
- Production Bible、Episode Plan/Structure、Storyboard、Generation Selection 的部分 Owner Service/Receipt；
- 官方 Temporal Go Client 的 Signal/History 对账与 UNKNOWN 重试；
- PostgreSQL/GORM Model Catalog 的单一 SQL 事实源。

设计接受时的缺口是：

- `lanverse-api` 没有公共 HumanTask 列表/详情/Claim/Decision/Resume HTTP；
- StoryGraph 新链路需要 Bible、Episode、Planning、Storyboard Intent、Storyboard Detail、Reference 和 Shot Frame 多种 Gate；
- 现有五类历史 Executor 不能覆盖新的 Candidate Revision/Head、Bible Confirm/Materialize 分离和两阶段 Storyboard；
- Frontend 无法可靠区分“决议已记录”“Owner 尚未应用”“Signal 结果未知”“Workflow 已继续”。

只公开 `ReviewService.Decide` 会使 API 重启后失去后续恢复入口；让 HTTP Handler 直接访问 GORM 或调用不同 Owner 则会建立第二套协调逻辑。本设计因此只增加一个薄的 Workflow HumanGate Coordinator 和一个 Review HTTP Adapter。

## 2. 目标与非目标

### 2.1 目标

1. 项目内 HumanTask 可分页发现、按 ID 重取，并精确绑定冻结 Subject revision/hash；
2. Claim/Renew/Release 保留 Actor、Revision、Lease Token、Expiry 和幂等 fencing；
3. 一次 Decision 请求先提交不可变决议，再尝试显式 Owner Apply 和 Temporal Resume；
4. 只凭已持久化 ReviewDecision ID 即可在 Browser/API/Worker 重启后恢复同一协调效果；
5. Bible、Episode、Planning、Storyboard Intent/Detail、Reference 与 Shot Frame Gate 使用同一公共协议、不同显式 Owner Applier；
6. Frontend 可分别展示 Task、Decision、Owner Apply 和 Workflow Resume 状态。

### 2.2 非目标

- 通用 Saga/Service Locator、反射式 Owner 路由、动态插件 Gate 或跨服务事务框架；
- 批量审批、自动审批、运营后台、任务指派策略或通知中心；
- 在 Task 表复制完整 Candidate、QC、Artifact、剧本或 Owner Snapshot；
- 用 Kafka 发送 Command/Signal、取代 Temporal，或用 ELK 日志恢复业务状态；
- 修改、删除或回滚 ReviewDecision；
- 在公共 Handler 中写 GORM、Raw SQL、Owner 分支或 Temporal Client；
- 把 Deterministic Gate blocker 降级为可由人工越过的警告。

## 3. Owner 与组合边界

```text
Review HTTP Adapter
  → Review Application
      ├─ Query / Claim / Renew / Release
      └─ Decide / GetDecision
  → Workflow HumanGateCoordinator
      ├─ Review GetDecision（只读冻结事实）
      ├─ Explicit OwnerApplier（按已发布 subject/executor 显式分派）
      ├─ Workflow Apply/Signal Intent/Receipt Store
      └─ Temporal Signaler
```

职责约束：

1. `review` 唯一写 HumanTask/ReviewDecision；Coordinator 不保存第二份审核状态；
2. 领域 Owner Service 在自己的 GORM 事务中写正式事实和 Command Receipt；Coordinator 不直写 Owner 表；
3. `workflow` 保存跨步骤协调 Receipt、冻结 Gate Output 并发送 Temporal Signal；它不成为 Production/Asset/Generation Owner；
4. HTTP Adapter 只做认证、严格 JSON、调用 Application、映射统一 Problem；
5. Agent 只产生 Candidate；Frontend 只发 Decision/Resume；二者均无 Owner/Signal 写权限；
6. 新 Owner Applier 必须随真实 Gate、Owner Command 和测试注册，未知 subject/executor fail closed；不提供 compatibility fallback。

Review Decision、Owner Apply 与 Workflow Signal 是三个事务边界，不伪装成一个数据库事务。可靠性来自不可变 ID、Receipt 和幂等对账，而不是跨 Owner 的隐藏回滚。

## 4. 冻结 Subject 契约

HumanTask 必须从已提交 Gate `node-input-v1` 建立，不接受 Browser 候选。Task 至少冻结：

- workspace/project/workflow run/node run；
- `subject_type + subject_id + subject_revision + subject_hash`；
- 排序去重后的 candidate IDs；
- rubric/policy version；
- Task status/revision、claim owner/token/expiry；
- 创建时间和过期/陈旧原因。

`subject_id` 指向不可变 Candidate Revision、CandidateSet 或待审核 Owner Snapshot；不得指向会原地漂移的“current”别名。若底层 current head 在 Decision 前变化，Task 必须变为 STALE 并拒绝决议；如果 Decision 已提交后才发现 Owner baseline 漂移，旧 Decision 保留审计但不能应用到新 Subject，必须通过新 Candidate Revision/Task 重新审核。

公共决议类型保持最小集合：

- `approved`：接受整个冻结 Subject；
- `rejected`：拒绝，不产生 Owner 正向写入；
- `changes_requested`：进入有界 Repair/新 Candidate Revision，不改写旧 Subject；
- `selected`：从 Task 冻结 candidate IDs 中选择且仅选择一个。

Subject/Rubric 决定允许的决议集合。`selected` 必须有 `selected_candidate_id` 且属于冻结集合；其他决议不得夹带候选。客户端文案可以本地化为“接受/拒绝/要求修改/选择”，机器值不得为每个页面自创同义词。

## 5. StoryGraph Gate 与显式 Owner Apply

| Subject type | 正向决议 | 唯一 Owner Command/效果 | Gate Output | 明确禁止 |
|---|---|---|---|---|
| `production_bible_candidate` | `approved` | Production Bible `Confirm`，创建/返回不可变 ProductionBibleVersion 与 Receipt | 精确 `production_bible_version` Ref | Confirm 顺带创建 Asset/State/AssetVersion |
| `episode_plan_candidate` | `approved` | Planning 原子物化 Episode 与 Published EpisodeScriptVersion | materialized episode set Ref/Hash | 部分批次成功或前端逐集写入 |
| `planning_candidate` | `approved` | Planning 全批应用 Scene/Dialogue/Beat/Occurrence/Claim | planning owner set Ref/Hash | 未知身份自动创建、部分批应用 |
| `storyboard_intent_candidate` | `approved` | Storyboard `FreezeIntentSet` | `approved_storyboard_intents` Ref/Hash | 创建正式 Shot、费用预留或 Provider Job |
| `reference_asset_candidate_set` | `selected` | Generation `CandidateSelection` | Selection/selected Artifact Ref/Hash | Task 直接写 AssetVersion 或按 URL 选图 |
| `storyboard_detail_candidate` | `approved` | Storyboard 全批创建正式 Shot 与完整 ShotProductionBindingVersion | Shot/Binding set Ref/Hash | 空 Binding、读取最新 AssetVersion、部分批应用 |
| `shot_frame_candidate_set` | `selected` | Generation `CandidateSelection` | Selection/selected frame Artifact Ref/Hash | 写入 ShotProductionBindingVersion |

`MaterializeConfirmedBible` 不是 Human Gate Owner Apply。它只在 `production_bible_candidate` Gate 输出已完成后由下游独立 Backend Coordinator 执行，拥有单独 Receipt，并创建 Asset/SpecificationVersion/AssetState/ProductionBinding；它不创建视觉 AssetVersion。

Reference 的 AssetVersion 发布和 Shot Frame 的 ShotImageBindingVersion 发布也必须消费精确 Selection/Artifact 并走各自正式 Owner Service；可与对应工作流节点相邻，但不能把 Task/ReviewDecision 当成视觉版本或输出 Binding。

`rejected`/`changes_requested` 不调用表中正向 Owner Command。Coordinator 持久化 `owner_apply_status=not_required` 和类型化 Gate rejection/repair output，再 Signal 同一个等待 Workflow；Workflow Definition 决定终止还是进入有界 Repair，浏览器不能自行重跑 Agent。

## 6. 公共 HTTP 契约

所有路由使用现有 Access Token、Membership/Token Version 复核、统一 Problem Envelope 和严格 JSON 解码。跨 Workspace/Project 的不存在或无权按现有防枚举策略返回；Viewer 写命令为 403；Revision/Lease/Stale/幂等漂移为 409；协议错误为 422；依赖不可用为 503。

### 6.1 Query

```text
GET /api/v1/projects/{project_id}/human-tasks
  ?status=active|OPEN|CLAIMED|COMPLETED|CANCELLED|STALE
  &subject_type={type}
  &limit=1..100
  &after={human_task_id}

GET /api/v1/human-tasks/{human_task_id}
```

列表按 `created_at DESC, id DESC` 稳定分页；`active` 只含可处理的 OPEN/CLAIMED。Task 查询返回冻结 Subject 引用、Candidate IDs、Rubric、Claim 摘要、Revision 和时间，不嵌入大 Candidate 内容。Frontend 再通过对应 Subject Query 读取 Evidence、QC、Cost、Lineage 和 Diff。

详情在已有 Decision 时附带独立协调状态：

```json
{
  "decision_status": "recorded",
  "review_decision_id": "...",
  "owner_apply_status": "not_required | pending | completed | conflict",
  "owner_receipt_id": "... or null",
  "workflow_resume_status": "pending | unknown | completed | conflict",
  "workflow_signal_receipt_id": "... or null"
}
```

这些字段是多个既有事实的只读组合，不回写 HumanTask。Claim Token 只在当前 Actor 是未过期 Claim Owner 时从详情返回；不得进入列表、URL、日志、Trace 或审计 payload。

### 6.2 Claim 生命周期

```text
POST /api/v1/human-tasks/{human_task_id}/claims
POST /api/v1/human-tasks/{human_task_id}/claim-renewals
POST /api/v1/human-tasks/{human_task_id}/claim-releases
```

Claim Body：`expected_revision + idempotency_key`。Renew/Release 再要求 `claim_token`。命令原样调用 Review Application/GORM Command Receipt；HTTP 不重做 Lease 状态机。相同 key/相同输入返回同一结果，不同输入冲突。

### 6.3 Decision 与 Resume

```text
POST /api/v1/human-tasks/{human_task_id}/decisions
POST /api/v1/review-decisions/{review_decision_id}/resume
```

Decision Body：

```json
{
  "claim_token": "uuid",
  "expected_task_revision": 2,
  "expected_subject_revision": 1,
  "expected_subject_hash": "sha256",
  "decision": "approved | rejected | changes_requested | selected",
  "selected_candidate_id": "uuid-or-null",
  "idempotency_key": "client-stable-key"
}
```

处理顺序固定：

1. Review Application 重新授权并在一个 GORM 事务中校验 Task/Lease/Subject/Candidate，提交唯一 Decision 与 Review Receipt；
2. Coordinator 只从已提交 Task/Decision 和 Gate Input 派生目标，不信任客户端补充字段；
3. 若需正向 Owner Apply，调用显式 Applier；成功后冻结 Owner Receipt 与 canonical Gate Output/Hash；
4. 使用 `human-gate-decision:{review_decision_id}` 作为稳定协调幂等身份，准备 Workflow Apply/Signal Intent；
5. Temporal Signaled/AlreadyApplied 且完整 Input Hash 一致时标记 Resume completed；无法确认时保存 unknown；
6. 返回 Decision 与三个阶段状态。Decision 已提交但后续未完成时，响应必须仍携带 Decision ID，不得返回使客户端以为全部回滚的普通错误。

`resume` 没有业务 Body，只允许可选的传输级幂等/追踪字段。Backend 按 Decision ID 重新授权、读取不可变 Task/Decision、Owner/Apply/Signal Receipt 并继续缺失阶段。它不能修改 Decision、替换 Subject/Candidate、对已确定 conflict 使用新事实，或跳过 Owner Receipt。

## 7. 状态与恢复

| Decision | Owner Apply | Workflow Resume | 外部语义 | 可执行动作 |
|---|---|---|---|---|
| 无 | — | — | Task 待处理 | Claim/Renew/Release/Decide |
| recorded | pending | pending | 决议已记录，业务尚未应用 | 同 Decision ID resume |
| recorded | completed/not_required | unknown | Owner 已确定，Signal 结果未知 | 同 Decision ID resume/History 对账 |
| recorded | conflict | pending | 冻结 Subject 或 Owner baseline 漂移 | 旧 Decision 只读；新 Candidate/Task |
| recorded | completed/not_required | conflict | Signal identity/input 漂移 | 人工诊断，不创建第二 Decision |
| recorded | completed/not_required | completed | Gate 已恢复 | 重取 WorkflowRun/NodeRun 和下游 Owner |

恢复不变量：

- ReviewDecision 一经提交不可覆盖、删除或“撤回”；
- Owner Receipt 已存在时不再次执行 Owner Command；
- UNKNOWN 只重放同一 Signal ID/Input Hash 并查询 Temporal History；
- 同一 Decision 的并发 resume 收敛到同一 Owner Receipt、Apply Receipt、Signal Intent/Receipt；
- API/Worker 重启从 PostgreSQL + Temporal History 恢复，不依赖浏览器内存或 Kafka 消息；
- Task 在 Decision 前过期/漂移时可 STALE；Decision 后的确定性冲突不能通过更新 Task revision 继续套用。

## 8. 数据、Kafka 与日志边界

PostgreSQL/GORM Catalog 是唯一 SQL 事实源，复用或正向扩展 HumanTask、ReviewDecision、Owner Command Receipt、Workflow Apply/Signal Intent/Receipt、WorkflowRun 和 NodeRun 模型。冻结 `subject_hash` 或查询索引若尚缺失，只能随真实 GORM Model/Repository/Test 加入 Catalog；不创建 Migration 文件/字段、Raw SQL、影子表、第二 ORM 或第二 Writer。

Temporal History 是 Workflow Wait/Signal/Timer 的执行事实，不替代 Owner 业务事实。Kafka 可以在上述事务成功后从 Outbox 发布 `HumanTaskOpened`、`ReviewDecisionRecorded`、`OwnerApplied`、`WorkflowGateResumed` 等投影事件，用于通知、搜索或审计读模型；Kafka Consumer 不执行 Decision、Owner Apply 或 Temporal Signal，也不能凭重复事件创建第二业务效果。

结构化日志经 Kafka 进入 ELK 时只记录 task/decision/run/node/receipt ID、状态、耗时、错误码和 trace id；不记录 Claim Token、Access Token、完整剧本、Candidate Payload、Prompt、Provider 凭据或私有 Artifact URL。ELK 故障不阻断已提交业务事实，日志也不能用于恢复 Gate。

## 9. Frontend MVP

Review Workbench 首版只做：项目 Task 队列、详情、Claim/Renew/Release、Subject 深链、Decision、Resume 和 WorkflowRun 重取。页面必须分别显示：

- Task `OPEN/CLAIMED/COMPLETED/CANCELLED/STALE`；
- Decision 是否已记录及不可变 ID；
- Owner Apply `not_required/pending/completed/conflict`；
- Workflow Resume `pending/unknown/completed/conflict`。

只有 `workflow_resume_status=completed` 且重取 NodeRun/下游 Owner 与 Gate Output 一致后，才显示“工作流已继续”。Decision recorded 只能显示“决议已记录”；Owner completed 只能显示“业务应用完成，正在恢复工作流”。

首版使用 RTK Query 轮询和命令后精确 invalidation；不预建 SSE/通知中心。Claim Token 只存当前受保护 Query/组件会话，刷新后由详情向同一 Claim Owner 恢复；不进 URL/localStorage/日志。未知 Subject renderer 保持只读并禁用 Decision，不猜测允许动作。

## 10. Red → Green 与验收门

编码前由 `SG-D18` Requirement 和 `SG-D21` Acceptance 建立失败契约，至少覆盖：

1. 跨 Workspace/Project、Viewer、Token Version 撤销与防枚举授权；
2. Claim/Renew/Release 的 Revision、Token、Expiry、接管、幂等重放和输入漂移；
3. Subject revision/hash/head、candidate set、rubric 和允许 Decision 的防篡改；
4. 表中七类 StoryGraph Gate 的显式 Owner 路由；未知 subject/executor fail closed；
5. Bible Confirm 不创建 Asset，Intent Freeze 不创建 Shot/Cost/Provider Job，拒绝/修改不产生正向 Owner 效果；
6. 进程在 Decision、Owner Receipt、Apply Intent、Signal Intent、Temporal 响应各边界退出后的同 ID 恢复；
7. UNKNOWN、AlreadyApplied、Input Hash 冲突、Owner stale、重复/并发 Decision/resume 的独立结果；
8. 新 PostgreSQL、真实 Temporal、API/Worker 重启完成至少一条 approved 和一条 changes_requested/rejected 旅程；
9. OpenAPI 生成、Frontend 状态文案、Backend/Agent/Frontend/Compose/镜像与当前真实 CI 全部通过；
10. Kafka/ELK 缺失或重复投递不改变 Gate 业务效果，且日志无敏感 Payload。

测试只位于独立目录：Backend 进入 `backend/tests/review`、`backend/tests/workflow` 或对应 `backend/tests/production/*`；Frontend 进入 `frontend/tests/unit`、`frontend/tests/e2e`；不得在业务源码旁新增测试文件。

## 11. 实施门禁

本设计在 `SG-D16` 接受时不声明公共 HTTP、新 GORM 字段或七类 Gate 已实现；后续 `SG-D17` PRD、`SG-D18` 跨服务 Requirement、`SG-D19` Agent Requirement、`SG-D20` 唯一 Plan 和初始全未勾选的 `SG-D21` Acceptance 已按顺序完成。当前 Backend 公共 API、既有五类 Owner 路由、恢复闭环与 Review Workbench 已有实现和验收证据，七类新 StoryGraph Gate 仍未实现。

代码实现只按 [0010 的唯一 `SG-Ixx` 队列](0010-StoryGraph内容图与DAG创作画布设计.md#唯一实施任务队列)：`SG-I06` 完成 Backend 公共 API，`SG-I07` 完成真实 Review Workbench，后续 Gate 随各自 Owner 任务逐个接入。每个完整任务通过局部验证和当前全量真实 CI 后独立提交；禁止兼容 fallback、跳过 CI、假 Owner Receipt 或本地模拟成功。

全部功能与真实 CI 在 `SG-I27` 完成并提交前不运行 `agent-browser`；最终浏览器验收只在 `SG-I28` 执行。

## 12. 风险与事实来源

主要风险：

- 把 Decision、Owner Apply 与 Workflow Resume 合成一个“已审核”状态；
- Handler/Coordinator 直接写 Owner 表或按字符串反射查 Service；
- 用旧 Decision 应用已经变化的 Candidate Head/Owner baseline；
- Signal UNKNOWN 时重做 Owner 效果或创建第二 Decision；
- Bible Confirm 顺带物化资产，或 Storyboard Intent Gate 提前产生费用/Shot；
- Kafka Consumer 执行业务命令，或用 ELK 日志当恢复事实；
- 为公共 Gate 创建第二数据库、Migration/Raw SQL 或通用 Saga 框架。

Gate 顺序和 Owner 效果以 [0010](0010-StoryGraph内容图与DAG创作画布设计.md)为事实来源；Bible Confirm/Materialize 边界以[Production Bible 设计](3001-项目制作圣经生成执行框架设计.md)为事实来源；Storyboard Intent/Detail 边界以[分镜 Harness 设计](3002-本地-Codex-分镜智能体执行框架设计.md)为事实来源；图片 Selection 以[Runware Provider 设计](2051-Runware图片Provider与Generation执行器设计.md)为事实来源。
