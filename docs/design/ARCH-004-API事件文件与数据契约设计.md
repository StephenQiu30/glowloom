---
layer: Design
doc_type: API Event File and Data Contract Architecture Design
doc_no: ARCH-004
title: API、事件、文件与数据契约设计
status: review
version: 0.2.1
owner: Lanverse
audience: [Architecture, Frontend, Backend, QA, Security, Operations, Data]
feature_area: 前后端集成、事件传播、文件传输与数据生命周期
purpose: 定义公开API、事件、文件和数据生命周期契约及其兼容与验证规则
canonical_path: docs/design/ARCH-004-API事件文件与数据契约设计.md
inputs: [SRS-001, FR-001至FR-021, NFR-001, TCR-001至TCR-003, ADG-001, ARCH-001至ARCH-003, ARCH-007]
outputs: [HTTP契约, 事件与回调契约, 文件传输契约, 数据分类与生命周期接口, 兼容和验证门禁]
triggers: [公开接口变化, 事件模式变化, 文件策略变化, 数据分类或保留规则变化, 客户端兼容窗口变化]
updated: 2026-07-24
downstream: [API PRD, Integration Plan, Contract Test Plan, Security Review, Acceptance, ADR]
---

# ARCH-004 API、事件、文件与数据契约设计

## 1. 设计结论与范围

Lanverse 以 OpenAPI 3.1 作为 HTTP 契约源，以版本化 JSON Schema 定义领域事件、SSE 数据和供应商回调，以短期对象授权传输大文件。`backend` 应用层是唯一业务写入权威：公开命令、Worker 结果、事件消费者和生命周期任务都调用所属模块的应用用例；事件、缓存、Temporal、对象存储和生成客户端均不是事实源。

本文覆盖身份上下文、命令/查询、错误、幂等、并发、分页、SSE、Webhook、Outbox、文件传输、数据分类/所有权/生命周期和版本兼容。不定义页面布局、供应商专有字段、数据库物理表、对象存储厂商或实时共同编辑协议；相关流程与任务状态分别由 [ARCH-002](ARCH-002-短剧生产流程与工作台设计.md) 和 [ARCH-003](ARCH-003-AI策划与生成任务架构设计.md) 负责，模块公开面和允许依赖以 [ARCH-007](ARCH-007-业务模块边界与服务协作设计.md) 为准。

## 2. 契约制品与责任

| 契约制品 | 接受后的目标位置 | 权威维护方 | 使用方 |
| --- | --- | --- | --- |
| HTTP OpenAPI 3.1 | `backend/contracts/http/openapi.yaml` | Backend Architecture | API、生成客户端、QA |
| 事件信封与事件模式 | `backend/contracts/events/*.schema.json` | 事实所属模块 | Outbox、Worker、投影 |
| SSE 事件模式 | `backend/contracts/sse/*.schema.json` | operations-support | Frontend、运营工具 |
| 供应商回调模式 | `backend/contracts/webhooks/*.schema.json` | production-jobs | Provider Adapter、Worker |
| 文件策略与媒体摘要模式 | `backend/contracts/files/*.schema.json` | media-library | Frontend、对象存储、媒体 Worker |
| 前端生成客户端 | `frontend/src/services/generated/` | 自动生成、禁止手改 | Frontend features |

契约文件必须固定 `contract_version`、生成工具版本和源提交。控制器 DTO、客户端类型、示例和契约测试必须来自同一接受版本；Prisma、Temporal 和供应商 SDK 类型不得泄漏到公开契约。

## 3. 通用 HTTP 约定

- 基础路径为 `/v1`；标识为不透明、不可复用的字符串，客户端不得解析其业务含义。
- 受保护请求由安全 HttpOnly 会话建立主体，路径或令牌提供工作空间/项目上下文；服务端每次校验主体、租户、对象和动作，且无权与不存在默认返回相同外观。
- 时间使用 UTC RFC 3339；金额为货币代码加最小单位整数；媒体位置为 `timebase` 加整数 tick/帧。
- JSON 字段使用 `snake_case`；写响应返回最新 `resource_version`/`etag`，日志和响应均携带 `request_id`。
- 创建同步资源返回 `201`，异步受理返回 `202` 和可查询标识，成功命令返回 `200/204`；不得用通用 `PATCH status` 表达批准、采用、取消或交付。

## 4. 命令与查询边界

| 类型 | 代表性契约 | 权限与一致性语义 |
| --- | --- | --- |
| 创建项目 | `POST /v1/workspaces/{workspace_id}/projects` | `project:create`；幂等创建 |
| 保存版本化草稿 | `PUT /v1/projects/{project_id}/script-drafts/{draft_id}` | `content:draft:write`；`If-Match` 条件写入 |
| 请求生成 | `POST /v1/projects/{project_id}/generation-requests` | `production:generation:request`；幂等键/预览哈希，经 generation Process Manager 完成预算、合规、准备度与 Task 创建 |
| 取消任务 | `POST /v1/production-tasks/{task_id}:cancel` | `production:task:cancel`；只写取消请求 |
| 重试任务 | `POST /v1/production-tasks/{task_id}:retry` | `production:task:retry`；幂等创建 RetryRequest，由原 Process Manager 重验预算/策略并新建 Attempt，不覆盖旧 Attempt |
| 记录审核决定 | `POST /v1/review-rounds/{round_id}/decisions` | `review:decide`；追加写入且固定对象版本 |
| 指定当前采用 | `POST /v1/shots/{shot_id}/adoptions` | `adoption:write`；与审核决定独立且唯一 |
| 创建交付版本 | `POST /v1/projects/{project_id}/deliveries`（`operationId: createDeliverySnapshot`） | `delivery:create`；映射 `delivery.createDeliverySnapshot`，固定门禁证据并幂等 |
| 读取单体 | `GET /v1/{resource}/{id}` | 权限过滤；返回 ETag、版本和可执行动作 |
| 读取列表 | `GET /v1/projects/{project_id}/production-tasks` | 过滤白名单、稳定排序和游标分页 |
| 查询审计 | `GET /v1/audit-records?correlation_id=...` | 仅授权角色；内容按数据级别脱敏 |

所有命令都显式接收 `subject_id`、`workspace_id`、`request_id`、授权结果和幂等上下文；这些值由服务端可信边界构造，不采信请求正文中的同名字段。查询不得绕过所属模块直接拼接跨租户数据。

## 5. OpenAPI 与生成客户端

- 每个 operation 必须具有稳定 `operationId`、权限说明、请求/响应模式、示例、全部预期状态码和 Problem Details 引用。
- OpenAPI 声明安全方案、内容类型、最大请求体、分页、ETag、幂等头和弃用信息；SSE 端点链接对应 JSON Schema。
- CI 在实施阶段执行 OpenAPI 3.1 校验、破坏性差异检查和客户端重生成；生成结果有差异但未提交时失败。
- 前端只能通过 `frontend/src/services/generated/` 访问公开 API；feature 层可以映射 ViewModel，但不得复制或手写另一套服务端 DTO。
- 未知可选字段必须忽略；开放枚举必须保留 `unknown` 展示与降级路径，禁止因新增值导致页面崩溃或误作批准。

### 5.1 代表性机器可读模板与示例

下列可解析片段固定 OpenAPI、错误、事件信封和上传会话的设计词汇；第 4、8～11 节是完整契约目录，进入接受状态时每个 operation/schema 都必须按同一规则实例化。

```yaml
openapi: 3.1.0
info: {title: Lanverse API, version: 0.1.0-review}
paths:
  /v1/projects/{project_id}/generation-requests: {parameters: [{name: project_id, in: path, required: true, schema: {type: string}}], post: {operationId: requestGeneration, responses: {"202": {description: Accepted}, "409": {description: Conflict}}}}
components: {schemas: {Problem: {type: object, required: [type, title, status, code, request_id]}, EventEnvelope: {type: object, required: [event_id, event_type, schema_version, occurred_at, producer, workspace_id, aggregate_type, aggregate_id, aggregate_version, correlation_id, causation_id, trace_id, data_classification, data]}, UploadSession: {type: object, required: [id, state, expires_at]}}}
```

成功/失败示例分别为 `{"generation_request_id":"gen_example","task_id":"tsk_example","status":"queued","resource_version":1}` 与 `{"type":"https://lanverse.example/problems/preview-stale","title":"Preview stale","status":409,"code":"PREVIEW_STALE","retryable":false,"request_id":"req_example"}`；`production-jobs.createTask` 仅供受控 Process Manager 调用，不作为客户端 operation。

## 6. 统一错误模型

错误使用 `application/problem+json`：标准字段为 `type/title/status/detail/instance`，扩展字段为 `code/retryable/request_id/errors[]/metadata`。`errors[]` 只含字段路径、稳定原因码和安全提示；`metadata` 仅提供下一动作、冲突版本或限流时间，不返回堆栈、密钥、签名 URL、完整提示词或未授权对象信息。

| HTTP | 稳定错误码示例 | 客户端行为 |
| --- | --- | --- |
| 400/422 | `VALIDATION_FAILED`、`BUSINESS_GATE_BLOCKED` | 绑定字段或展示阻断及下一动作 |
| 401/403/404 | `AUTH_REQUIRED`、`ACCESS_DENIED`、`RESOURCE_NOT_FOUND` | 清理过期会话；不推断对象存在性 |
| 409/412 | `VERSION_CONFLICT`、`IDEMPOTENCY_KEY_REUSED`、`IDEMPOTENCY_IN_PROGRESS`、`PREVIEW_STALE` | 冲突时刷新/比较；处理中只按原键、原正文及 `Retry-After` 重试或轮询 `status_url` |
| 413/415 | `FILE_TOO_LARGE`、`MEDIA_TYPE_NOT_ALLOWED` | 停止上传并保留可修正输入 |
| 429/503 | `RATE_LIMITED`、`DEPENDENCY_UNAVAILABLE` | 仅在 `retryable=true` 时按 `Retry-After` 重试 |

## 7. 幂等、并发与分页

- 任务、上传会话、费用、交付及其他可重放命令要求 `Idempotency-Key`；统一作用域为 `workspace_id + subject_id + operation_id + target_scope + idempotency_key`，其中 `target_scope` 使用稳定目标标识，创建类命令使用明确集合标识；同时保存规范化请求哈希、状态、资源标识和响应摘要。
- 同键同哈希已有可寻址资源时返回原成功/受理状态、响应摘要及同一 `Location`；尚未形成资源时返回 HTTP `409 IDEMPOTENCY_IN_PROGRESS`、`retryable: true`、`Retry-After`，并在可用时给出安全的 `metadata.status_url`，不得启动第二副作用。客户端只可在等待后以同一键和完全相同正文重试，或轮询 `status_url`；网络超时和处理中响应不得自动生成新键，只有用户明确发起新的逻辑操作且原操作已收敛后才能换键。同键异哈希返回 `409 IDEMPOTENCY_KEY_REUSED`。
- 普通键至少保留 24 小时；账本、正式交付和其他不可逆动作的去重证据随事实保留。具体上限在容量设计确认，不得短于客户端可重试窗口。
- 可并发修改资源必须使用强 ETag/资源版本和 `If-Match`；冲突返回 `412 VERSION_CONFLICT` 与最新安全版本，不静默最后写入覆盖。
- 列表响应为 `items/next_cursor/has_more`；默认 50、最大 200，先执行租户与权限过滤，再按稳定元组排序，默认 `created_at DESC, id DESC`。
- 游标不透明并绑定查询、排序、租户和有效期；篡改或不匹配返回 `400 INVALID_CURSOR`，过期返回 `410 CURSOR_EXPIRED`。空结果返回空数组，禁止无界导出替代分页。

## 8. SSE 契约

`GET /v1/production-task-events` 仅提供有权项目的任务投影。事件包含 `id/event/data`，`data` 使用第 10 节信封；客户端通过 `Last-Event-ID` 续接，按 `event_id + aggregate_version` 去重并忽略旧版本。服务端每 15 秒发送无业务含义心跳，并在事件或心跳前复核授权；权限撤销后终止连接。

SSE 至少覆盖 `production.task.updated.v1`、`production.attempt.updated.v1`、`generation.candidate.available.v1` 和 `delivery.updated.v1`。事件仅提示状态变化；断线、乱序、游标过期或未知事件时，前端显示状态未知并重新查询权威资源。首发保留窗口在容量设计确认，低于窗口的游标返回 `410 EVENT_CURSOR_EXPIRED`，不得从零盲目重放。

## 9. Webhook 契约

P0 仅接收供应商回调；客户自定义出站 Webhook 不在首发范围。Adapter 端点必须基于原始请求体验证签名、密钥版本、时间戳和 nonce，默认允许时钟偏差不超过 5 分钟，并以 `provider + external_event_id` 建立唯一 Inbox 记录；供应商无事件标识时使用签名覆盖内容的稳定摘要。

系统先把原始请求体、必要头摘要和验证结果作为加密受限证据与 Inbox 持久化，再返回成功并异步调用所属应用用例；重复回调返回相同受理结果。无效签名返回 `401`，过期/重放返回 `409`，临时不可持久化返回 `503`；任何回调都不能直接采用媒体、结算未验证用量或覆盖既有 Attempt。重试、死信、人工重放和供应商请求标识必须可审计。

## 10. 事件信封与 Outbox

事件采用至少一次交付、消费者幂等和聚合内有序语义。信封必含 `event_id/event_type/schema_version/occurred_at/producer/workspace_id/aggregate_type/aggregate_id/aggregate_version/correlation_id/causation_id/trace_id/data_classification/data`；大媒体、凭据和长期授权不得进入 `data`。

- 领域变更与 Outbox 行在同一 PostgreSQL 事务提交；数据库不主动调用 Temporal 或消费者。
- Dispatcher 以租约领取 Outbox，发布成功后记完成；失败退避重试并在阈值后进入可人工恢复的死信状态。
- 消费者以 `consumer + event_id` 写 Inbox 去重，并经所属模块应用用例写业务事实；通知、搜索和分析投影允许重建。
- `aggregate_version` 保证同聚合顺序，跨聚合不承诺全序；事件模式仅做向后兼容追加，纠正事实发布新事件。
- 事件命名使用过去式和大版本后缀，如 `production.task.created.v1`；命令、SSE 投影和领域事件使用不同类型名。

## 11. 文件上传与下载契约

| 阶段 | 契约 | 强制校验 |
| --- | --- | --- |
| 初始化 | `POST /v1/upload-sessions` | 幂等键、用途、文件名、声明类型/大小、SHA-256、所属对象和策略版本 |
| 直传 | 客户端向对象存储上传单体或分片 | 短期、对象级、操作级授权；分片号、长度和校验和 |
| 完成 | `POST /v1/upload-sessions/{id}:complete` | 分片清单、总大小、最终校验和和 ETag；完成不等于可用 |
| 取消 | `POST /v1/upload-sessions/{id}:abort` | 终止未完成授权并安排清理孤立分片 |
| 下载 | `POST /v1/media-versions/{id}/download-authorizations` | 权限、用途、文件版本、处置方式、短期授权和审计 |

实际类型探测、恶意内容扫描、媒体探针和完整性检查在隔离区完成；通过后才创建可用 `MediaVersion`。浏览器预览使用代理及可续签的范围读取授权；URL 过期只重新授权，不改变媒体版本。删除必须检查引用、法律保留和交付证据，并传播到对象、代理、缓存和供应商；供应商 URL 永不作为平台永久媒体地址。

## 12. 数据分类、所有权与生命周期接口

| 等级 | 数据示例 | 契约规则 |
| --- | --- | --- |
| C0 公开 | 已批准公开信息 | 明确发布动作后方可公开缓存 |
| C1 内部 | 能力目录、非敏感运行指标 | 登录态最小字段，禁止跨租户聚合泄露 |
| C2 机密 | 原著、剧本、Prompt、媒体、候选、评论 | 对象级授权、加密、脱敏遥测和受控导出 |
| C3 受限 | 个人数据、令牌索引、权利证据、账本、安全/审计记录 | 最小用途、增强审计、严格保留；Secret 只返回引用 |

权威所有权使用 ARCH-007 的 17 模块：`identity-access/project-catalog` 负责身份与项目，`story-development/continuity/creative-assets/media-library/storyboard` 负责故事、资产、媒体与镜头，`agent-runtime/model-catalog/generation/production-jobs/postproduction` 负责 AI 和生产，`review-approval/cost-billing/compliance-governance/delivery` 负责决定、费用、规则和交付，`operations-support` 只维护通知、运营案件与可重建投影。跨域只传稳定标识、版本化快照或事件，禁止共享写表、ORM 关系和级联。

生命周期接口使用显式命令：`POST /v1/projects/{id}:archive`、`POST /v1/media-versions/{id}:request-deletion`、`POST /v1/data-subject-requests` 及其查询。归档不删除事实；不可变版本、决定、账本、合规和审计仅追加纠正记录。删除由各所有者按分类、引用、保留和法律例外执行，输出不可篡改 `DeletionEvidence`；备份中的删除在保留周期到期时收敛并记录例外，缓存/投影可由权威事实重建。

### 12.1 概念唯一约束与访问索引

- 唯一约束至少覆盖：`workspace_id + aggregate + version`；每分集/类型唯一当前基线；每采用作用域唯一 active Adoption；`workspace_id + subject_id + operation_id + target_scope + idempotency_key`；`task + attempt_no`；`attempt_id + output_slot`；`metering_event_id`；供应请求、Outbox `event_id` 与 Inbox `consumer + event_id` 去重。
- 主访问索引以 `workspace_id` 为首列，覆盖 Task `(status, created_at, id)`、Attempt `(task_id, created_at, id)`、Outbox `(state, next_attempt_at, id)`、审计 `(occurred_at, id)/(correlation_id)`、稳定游标排序、MediaUsageProjection 反向引用及 DeletionCase/LegalHold 发现；物理索引须经 Plan 中的查询与容量基准确认。

## 13. 版本、弃用与发布兼容

- `/v1` 内只允许向后兼容新增；删除、改名、语义改变、收紧必填或复用枚举值必须发布新大版本。
- 废弃字段/operation 同时发布 `Deprecation`、`Sunset` 和迁移链接，支持期不少于 90 天且不少于两个生产发布周期；安全紧急变更须记录例外、影响和补救。
- 事件/Webhook 的 `schema_version` 使用大版本兼容边界；同大版本只新增可选字段，生产者不得早于消费者兼容部署新必填语义。
- 发布采用数据库 expand→API/Worker 双读兼容→Frontend 切换→contract 清理；当前及上一接受客户端在滚动窗口内均可判断状态。
- 契约差异、生成器版本、Schema 哈希和应用版本进入发布证据；回滚不删除已创建事实，运行中任务仍可由稳定标识恢复。

## 14. 需求双向追踪

| 需求条款 | 本设计落点 | 下游验证标识 |
| --- | --- | --- |
| FR-001-003/007～010，NFR-001-014～020 | 3、4、6、12 | VAL-ARCH-004-SEC |
| FR-002-001/011/012，FR-007-001～012 | 3、11、12 | VAL-ARCH-004-DATA |
| FR-010-004/007/009/014，FR-017-005～010 | 4、7、8、10 | VAL-ARCH-004-IDEM |
| FR-016-001/006～010，FR-018-001～016 | 4、6、12 | VAL-ARCH-004-GOV |
| FR-019-009～017，FR-020-003/010/011/015 | 4、10～12 | VAL-ARCH-004-LIFE |
| NFR-001-008/023～027/030/041～044 | 6～13 | VAL-ARCH-004-RES |
| TCR-002-005～007/016～024/032/035/037 | 2～13 | VAL-ARCH-004-BE |
| TCR-003-004/005/009/011～018/021～024/030～032/037 | 3～13 | VAL-ARCH-004-FE |
| ADG-001-001～012/016/017/021/025 | 全文 | AC-ARCH-004-001～006 |

反向追踪规则：本节负责“需求条款→设计→验证”，[ARCH-001 需求追踪矩阵](ARCH-001-AI短剧制作平台总体架构设计.md#10-需求追踪矩阵) 负责“设计→需求条款”的总入口；本文进入 `accepted` 前必须在该矩阵登记 ARCH-004。后续 PRD、Plan、契约测试和 Acceptance 必须引用 `VAL-ARCH-004-*` 与具体需求条款，禁止仅写“符合 ARCH-004”。

## 15. 分阶段验证与验收

| 阶段 | 验证标识与通过条件 |
| --- | --- |
| Design | `VAL-ARCH-004-SPEC`：契约目录、字段/错误/权限/版本规则和代表性机器可读模板/示例可定位且可解析，无未说明缺口 |
| Design | `VAL-ARCH-004-TRACE`：随机抽取本节具体条款可双向定位到契约、所有者和拟定测试；分类、保留及未决容量有责任人和决策点 |
| Design | `VAL-ARCH-004-THREAT`：完成租户越权、ID 枚举、CSRF、回调伪造/重放、上传恶意文件、签名 URL 泄漏和删除例外评审 |
| Implementation | `VAL-ARCH-004-CLIENT`：同一 OpenAPI 生成服务端校验和前端客户端；lint、破坏性 diff、生成无漂移及前端严格类型检查通过 |
| Implementation | `VAL-ARCH-004-CONTRACT`：生产者/消费者契约测试覆盖全部状态码、Problem Details、未知字段/枚举和当前/上一客户端兼容 |
| Implementation | `VAL-ARCH-004-RELIABILITY`：重复/并发命令、乱序/重复事件、SSE 续接、回调重放、Outbox/Inbox 重启不重复事实或扣费 |
| Implementation | `VAL-ARCH-004-FILE`：分片断点、哈希不符、伪造类型、病毒样本、授权过期、范围读取、隔离和删除传播通过 |
| Implementation | `VAL-ARCH-004-LIFECYCLE`：跨租户拒绝、导出/删除/保留例外、审计完整性及缓存/投影重建形成 Acceptance 证据 |

设计验收项：AC-ARCH-004-001～006 分别对应 HTTP/OpenAPI、错误与幂等、SSE/事件、Webhook/Outbox、文件/数据生命周期、兼容/追踪。当前仅形成 `review` 规则、目录和代表性模板，不代表完整契约、测试或实现已完成；只有 Design、PRD 和可执行 Plan 均为 `accepted` 后才能实现。
