# 完整创作 Agent、Skill 与多 Agent 实施计划

2026-11-12范围说明：按PRD19新增主子协作、Skill自动调用、外部MCP、持久唤醒与稳定性实施切片（本文末节）。当前用户要求先完善文档，本轮不执行下面任何代码、连接配置、迁移、部署或模型消费步骤；此前迁移授权不覆盖本轮限制。新增切片来源仍只有PRD19，不从调研或本Plan另造业务范围。

> 执行方式：使用 writing-plans、executing-plans；独立责任切片按 subagent-driven-development 执行，核心行为 Red → Green → Refactor。原 Pi Durable 主线的实施授权见需求19。2026-11-06 的 NestJS 请求先交付文档。

**目标**：站内创作入口实际运行同仓库独立 agent 服务，以官方 Pi Durable 和 PostgreSQL 承载完整 Harness、Skill 与真实专业子会话；Docker Compose 为唯一启动入口，删除旧执行实现及兼容路径。

**唯一来源 PRD**：[PRD19](../prd/19-完整创作Agent与Skill多Agent.md)。技术合同通过该 PRD 追溯设计32；本计划不建立第二份需求来源。当前进度、失败和下一步仅维护在 [BACKLOG](../../BACKLOG.md#pi-durable-migration)，下列步骤是执行合同，不是完成记录。

## 1. 全局约束与文件责任

- 服务/目录/包/Compose/CI 名称统一 agent；TypeScript strict、原生 ESM、Node 24、官方 Pi 1.5.0 同版包、NestJS（Fastify 适配器）、pg、Pino、Vitest、pnpm/tsc；版本与锁定条件见设计32 §7.1.2。
- agent 独占会话、执行段、计划、owned conversation 与执行恢复；Go 独占身份/scope、审批、预算、模型/业务工具及业务回执。无跨 schema 写入，无 Go 启动 Node。
- 目标从 Nest Controller/DTO 注解生成内部 OpenAPI 与 Go 客户端，继续由公网 Go 注解生成前端 API。Agent 消费 Go 内部接口的 Umi 生成链见设计37 §7.6。
- 生产只用 PostgreSQL；正式空库来源 backend/db/schema.sql。独立增量 SQL 供离线审阅执行；应用启动不执行 DDL，不重建既有业务库。
- NestJS 单版本切换。删除旧框架入口、旧 HTTP 合同和生成链，以及旧图、stdio/launcher、旧路由/配置别名、状态翻译、双轨和回退。
- 普通业务后台任务保留原领域工作/供应商请求/财务回执，以数据库领取、租约、有限重试和到期补扫替代原工作流基础设施；不引入通用 DAG 引擎。
- 改动前保存工作区路径清单，逐文件保留并行存储/模型/画布/探索页修改；用户未要求新分支或 Git 交付，沿当前工作区实施，禁止 reset/clean/广泛暂存。
- 新服务及隔离验证可执行；现有在途任务、现有数据物理删除与真实收费调用须以精确目标和既有授权处理。旧任务未清结时阻止生产切换，不编写兼容接管器。

| 文件范围                                                           | 唯一责任                                                           |
| ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| agent/package.json、锁文件、tsconfig、Dockerfile                   | 独立构建与固定依赖                                                 |
| agent/src/pi-durable/storage、agent/tests/storage        | 官方 Storage PostgreSQL 实现、owner/fencing、完整一致性测试        |
| agent/src/http、agent/src/server.ts | 注解合同、认证、HTTP/SSE、关闭                             |
| agent/src/pi-durable、agent/src/sessions、agent/src/business | 官方执行组合、应用 doc、控制投递/回执、Go 客户端 |
| backend/internal/agent                                             | 当前公共代理及业务授权接口，删除旧 domain/graph 与执行 application |
| backend/internal/platform/taskruntime                              | 领取/租约/续租/有界并发的窄运行时，无领域执行状态机                |
| 各领域 adapter/task、application、postgres                         | 当前后台任务执行、结果及副作用幂等；业务模块保留本身权威           |
| backend/internal/app、cmd/glowloom、platform/config                | 删除旧连接/worker/schedule 接线并接入当前模块                      |
| backend/db/schema.sql | 空库完整结构，覆盖 Agent 运行、业务授权与任务        |
| frontend AgentWorkspace、backend 注解及生成 API                    | 当前交互/控制/事件合同，无旧分阶段入口                             |
| docker-compose.yml、.env.example、CI、根文档与主题文档             | 单入口、角色隔离、工程门禁和失效描述清理                           |

## 2. 复核重点

1. COMMIT 成功但客户端未确认：只读原记录，不重复业务/收费；Storage 保留官方前缀提交语义。
2. 批准早于整树 idle：收束回执及新执行段 grant 成立前保持围栏，旧 task 永远不能借新授权执行。
3. 同一 generation task 多轮模型调用：checkpoint 边界区分请求，恢复同一边界复用调用身份，压缩同样计费/校验。
4. 动态扩展与 hook 故障：加载器不能执行租户上传代码；hook 被跳过时真正出口仍拒绝越权。
5. 旧任务/并行修改：保留业务结果与原数据；发布前盘点清结，不能靠删除测试、空 handler 或兼容平台伪装替代完成。

## 3. 顺序、接口与执行步骤

### Task 1：固定独立包与验证环境

文件：agent/package.json、pnpm-lock.yaml、tsconfig*.json、eslint.config._、vitest.config._、.dockerignore、Dockerfile；agent/tests/config.test.ts。

接口：导出 Config（HTTP 地址、两个方向的 token、业务 origin、受限 PG DSN、资源限额），loadConfig(env) 在缺失/非法配置时失败，不记录密钥。

- [ ] 先写缺配置、短 token、非 HTTP origin、无受限 DSN 的失败用例并运行，观察失败。
- [ ] 安装设计固定依赖，确认官方公开 exports 和实际类型；不得从私有路径导入。开发 Node 与目标镜像版本差异单独记录。
- [ ] 实现严格配置及 build/typecheck/lint/test/api:schema/api:check 脚本，生产运行 dist/src/server.js。
- [ ] 执行 `cd agent && pnpm install --frozen-lockfile && pnpm typecheck && pnpm build && pnpm test -- tests/config.test.ts`。预期全部通过；安装失败不得代填成功锁文件。

接受：AC-AH-19/20。

### Task 2：真实 PostgreSQL 官方 Storage

文件：agent/src/pi-durable/storage/postgres.ts、records.ts、ownership.ts；agent/tests/storage/conformance.test.ts、recovery.test.ts；backend/db/schema.sql。

接口：openPostgresStorage(pool, sessionId, owner) 返回官方 Storage 及 close；owner={id,epoch,leaseUntil}。以 session 为 Storage 分区，包含根与全部 owned children；SQL 静态定义，不在 open 中建表。

- [ ] 先接完整 registerStorageConformance，及原子批次/拒绝前缀/请求去重/文档历史/对象隔离用例，运行观察未实现失败。
- [ ] 实现官方 typed records、历史、全局 ID、批次提交/receive 与扫描语义；事务检查 epoch/lease，COMMIT 确认后才 receive。
- [ ] 增加两个 owner 竞争、失租约、未知 COMMIT、连接断开及重开测试。事件投影与 Pi 批次同一事务，不能提交后另补。
- [ ] 执行 `cd agent && pnpm test -- tests/storage`，使用明确隔离测试库；未给测试 DSN 时测试应明确失败，不静默 skip。预期官方套件与故障用例通过。

接受：AC-AH-02/13/15/18；ST-AH-20/24/26/27。

### Task 3：当前服务合同与只读/事件接口

文件：按设计37 §7.2 更新 Nest 组合模块、HTTP 边界、会话 Controller/DTO 与 agent/tests/http.test.ts。以当前 TypeBox 路由和 Go 生成物为迁移基准。

接口：/internal/agent/sessions、/sessions/:sid/runs、/runs/:rid/commands、/runs/:rid/events；请求含 request_id/内容哈希/scope，输入 expected_execution_revision 与控制 authorization_revision 分开；snapshot 返回同一一致性点的 snapshot_cursor。

- [ ] 先测认证失败、伪造 scope、同键异内容冲突、只读不启动任务、SSE 断线不 abort。
- [ ] 从实际 Nest Controller/DTO 导出 OpenAPI。以 HTTP 正反例核对校验、8 MiB 上限、脱敏错误和流背压。
- [ ] 测同一事务状态/事件可见、游标过期 snapshot_required、快照接续无缺口、丢通知补读及慢订阅。
- [ ] 执行 `pnpm typecheck && pnpm test -- tests/http.test.ts && pnpm api:check`。预期合同生成一致且行为通过，不需要模型/业务库启动来导出 schema。

接受：AC-AH-12/18/19/20；ST-AH-21/26/27/35。

### NestJS 框架切换顺序

按[设计37 §8](../design/37-Agent服务目录与TypeScript规范设计.md#8-实施与验收)记录验收证据。

1. 从待迁移基线导出 Agent OpenAPI，记录路由、响应和 SSE 行为，生成 Go 客户端并保存可重复比较的基线。记录现有测试清单及生产构建的执行定义指纹。
2. 锁定 Nest 核心包、Fastify 适配器、Swagger 与 peer。用生产编译输出验证装饰器元数据、离线装配、开发入口和关停，再通过现有用例端口接入实际 HTTP。
3. 对照基线运行 HTTP/SSE 与合同检查。在隔离装配中生成 OpenAPI 并重新生成 Go 客户端，同次更新 Go 调用方并移除旧路由注册、TypeBox 合同与生成脚本。按设计37 §7.6 补齐 Go 内部 Swagger 并独立验证 Umi 生成及请求层。
4. 运行 Agent 全量门禁、Go 代理合同和真实隔离 PostgreSQL 恢复测试。核对启动、长连接关闭、进程故障、owner fencing 与冻结指纹。正式切换前从目标镜像读取 `dist/src/pi-durable/definitions.js` 的 `DEFINITION_HASH`，盘点全部可接续运行和未决控制，按设计37 §7.5 确认切换条件。验证未通过时暂停发布并修复目标版本。

### Task 4：Go 业务授权与公共代理

文件：backend/internal/agent/domain、application、adapter/http、adapter/postgres、adapter/runtimeclient（当前职责重建）；backend/internal/app/agent.go、public_api.go；backend/db/schema.sql；backend/tests/agentbusiness。

接口：公共 /api/projects/:pid/agent/sessions 与 session runs/commands/events；删除旧 agent-sessions/advance/confirm/turns。Go→agent 带经认证 actor/project/canvas 与服务 token；agent→Go 使用另一 token。

- [ ] 先测私人会话不可跨人/跨项目访问、内部请求不能伪造用户、grant 失期/旧 epoch/旧执行段拒绝。
- [ ] 事务保存当前授权 revision、提案/审批、预算及控制 outbox；只保存业务准入，不镜像 Pi 计划/任务状态。
- [ ] 可靠投递 approve/reject/pause/resume/stop；Agent durable inbox 接受，最终应用回执才标记交付。明确 superseded、revision 间隙和 Stop 终止语义。
- [ ] 测投递丢失/重复/乱序、应用后确认丢失、新 owner 不能解除暂停/审批，以及早到批准。
- [ ] 执行 `cd backend && go test -race -count=1 ./tests/agentbusiness/...`。预期角色隔离及授权/控制竞态全部通过。

接受：AC-AH-04/13/15/18/22；ST-AH-02/14/24/25/27/33。

### Task 5：Go 模型网关、工具和原调用回执

文件：backend/internal/agent 业务模型/工具适配，复用 catalog/operation/canvas/script/subject/media 的正式应用命令；agent/src/business/client.ts、agent/src/pi-durable/model-provider.ts、registry.ts；对应双方测试。

接口：/internal/agent-business 的 grants、snapshot、tools、skills、proposals、decisions、models 与 calls。调用键固定 run/执行段/Pi task/call/请求边界，并绑定不可变参数哈希；未知结果先查 calls，不换 ID。

- [ ] 先测 Go 业务已提交但 Agent 未存结果时 kill/重试只返回原结果，供应商未知接单不发新请求、不重复预留。
- [ ] 模型规划/压缩/子会话统一出站，供应商主密钥留 Go；输出含真实 usage 与调用回执。工具描述唯一来自 Go，可信 scope 不来自模型参数。
- [ ] 真实接线已有业务能力，禁止空实现、固定成功或把 unsupported 当作已覆盖。隔离脚本设施不存在时明确拒绝。
- [ ] 测参数篡改、嵌套工具、价格/画布版本、撤权、费用与连续写链；分别执行双方定向测试与跨 HTTP 测试。

接受：AC-AH-05/06/07/10/11/16/18；ST-AH-03/04/09/13/14/15/32/34。

### Task 6：官方 Harness 与执行段生命周期

文件：agent/src/pi-durable/registry.ts、trusted-extensions.ts、model-provider.ts、docs.ts；agent/src/sessions/service.ts、runtime.ts、manifest.ts；agent/tests/controls.test.ts及相关Harness测试。

接口：SessionService 接受输入/控制但不自写模型循环；registry 显式选择工具/扩展，manifest 固定 SDK/定义/工具/模型/Skill/hash；每 session 同时只有一个活动业务 root_run。

- [ ] 先用官方 Harness 写模型—工具—模型、重复 requestId、manifest 不符和只读访问测试，观察缺失失败。
- [ ] owner 成立后 open；所有 submit/wait/resume 执行窗口都需授权。配置 maxRetries=0，关闭 provider 自动重发，保留官方 interrupted/faulted 结果。
- [ ] 在模型出口与工具 execute 强制检查；测试 beforeRequest/beforeTool 抛错仍不越权。持久化每个模型请求 checkpoint 的身份映射，不用 span ID 或内存计数器替代。
- [ ] 提案事务建立整树围栏；提案工具及被阻断的工具结果 terminate，官方 abort 收束兄弟 owned work。持久 quiesced_receipt 后 Go 才签新 continuation 段 grant；同一 commit 接受 continuation 及应用记录。
- [ ] 执行 `pnpm test -- tests/harness tests/controls`。预期多工具/兄弟任务、批准早到、ack 丢失、旧段迟到与三个握手 kill 窗口通过。

接受：AC-AH-01/02/04/07/13/21/22；ST-AH-01/02/05/06/24/25/29/30/32/33/34/35。

### Task 7：真实专业 Agent、Skill 与压缩

文件：agent/src/pi-durable/delegation.ts、skills.ts、context.ts、trusted-extensions.ts；agent/tests/delegation.test.ts、skills.test.ts及压缩上下文测试。

接口：委派工具在官方 api.commit 按 ownerTaskId 复用/创建 owned conversation，配置后再 submit。角色输入包含采用来源/目标/模型/工具/预算；根额度向子分配，不能复制总额度。

- [ ] 先测两个实际子会话、父 abort、崩溃复用 child、子级扩权/递归拒绝、结果采用及局部失败。
- [ ] 计划/采用/Skill 引用使用 defineDoc，不复制 transcript。官方 CompactionTask 保留目标/约束并经 Go 计费。
- [ ] 受控 SDK loader 只装载镜像内审阅入口；cwd/.pi/extensions 与 agentDir/extensions 必须为空且只读，不扫描 HOME/上传目录，不运行第三方 Node/默认宿主工具。
- [ ] 测 Skill 管理完整生命周期、版本/资源哈希、撤销、压缩后约束、缺隔离设施拒绝及媒体实际读取依据。
- [ ] 执行 `pnpm test -- tests/delegation tests/skills tests/compaction`。预期实际 owned conversation 与隔离边界通过，不能用不同提示词函数替代。

接受：AC-AH-03/08/09/10/11/16/21；ST-AH-07/08/10/11/12/13/15/17/29/30/31/35。

### Task 8：普通任务数据库领取运行时

文件：backend/internal/platform/taskruntime、backend/db/schema.sql；backend/tests/taskruntime。

接口：Handler(ctx, Job) 返回持久结果/next_run_at；Store Enqueue/Claim/Renew/Complete/Reschedule 检查 owner/epoch/lease。Enqueue 可绑定现有业务事务，业务 ID 唯一，队列只唤醒原任务。

- [ ] 先测同业务 ID 去重、并发领取、失租约拒绝终态、停机后到期补扫及取消/关闭排空。
- [ ] 实现窄 Postgres 领取器、连接级参数化 SQL、有界并发/重试/退避；等待不占锁/长期 goroutine，不引入通用 DAG/脚本平台。
- [ ] 执行 `cd backend && go test -race -count=1 ./tests/taskruntime`，预期真实隔离 PG 的竞争与恢复通过。

接受：AC-AH-14；ST-AH-18/19。

### Task 9：生成、供应商、批次与试用替代

文件：operation/catalog 当前 task 适配器及 application/postgres、operation HTTP trial、app/catalog、app/aitrial、app/codexruntime；删除被替代 workflow 文件与测试。

接口：原 operation/batch/provider request/call/结算账本不变成运行时镜像；submit/query/cancel/ingest/settle 分步持久化，未知接单仅核对原 request。Codex 保留正式外部能力，删除旧调度宿主。

- [ ] 先把正常/拒绝/超时/未知提交/取消/原件补齐/审核/结算的领域断言移入当前 task 测试并观察失败。
- [ ] 接入 Task8 领取器，替换 starter/signal/schedule 注册；保留资源限额、供应商分类、原费用规则，删除版本兼容分支。
- [ ] 验证批次子任务去重、凭据测试、免费试用和报价过期；禁止删功能以实现无旧依赖编译。
- [ ] 执行 `go test -race -count=1 ./tests/operation ./tests/catalog ./tests/aitrial ./tests/codex`（以实际测试包清单展开），预期相关领域断言均通过。

接受：AC-AH-14；ST-AH-18/19。

### Task 10：媒体、导出、转写、深度、复制、导入、声音及维护

文件：media/mediatool/workspace/script/subject 的 task 适配与原领域 worker；infra/maintenance 当前适配；各 backend/tests。

- [ ] 逐类先覆盖正常/失败/取消/重启、对象归档断点、引用保护、原供应商请求与结果采用用例。
- [ ] 迁移 transfer/purge、export/transcription/depth、project-copy/retention、script-import、voice、分区/outbox/inbox/到期维护，复用原业务工作记录与任务结果。
- [ ] 所有领取及续租/终态带当前 epoch；任务准入和可领取事实同事务；恢复不依赖 Kafka 重发。
- [ ] 执行各实际 `backend/tests/<领域>` 的 race 测试；真实媒体导出另以 ffprobe/解码验证。预期完整覆盖设计32 §7.7 的每行，而非仅注册空 handler。

接受：AC-AH-14/16；ST-AH-16/18/19。

### Task 11：删除旧接线、配置及依赖

文件：backend/internal/app/{wiring,roles_wiring,wire,wire_gen,worker,relay,role}.go、cmd/glowloom、platform/config、go.mod/go.sum；删除 temporalconn/temporal_setup、旧 Agent graph/worker/checkpointer 和对应旧测试。

- [ ] 先写应用装配无需旧服务连接、普通 API 不依赖 agent readiness 的断言，运行观察旧接线失败。
- [ ] 仅接当前数据库 worker、Agent HTTP 与业务接口；重新生成 Wire，go mod tidy。旧原型/engine 选项/stdio/命令/环境别名明确移除。
- [ ] 用 rg 扫描执行代码/构建/配置，旧 SDK import、连接、命令和旧 Agent 路由为零；不将注释改名视为迁移。
- [ ] 执行 `go test -race -count=1 ./... && go vet ./...`，另按项目门禁运行 goimports/golangci-lint/govulncheck，缺工具或既有失败分别记录。

接受：AC-AH-14/15/17/18；ST-AH-18/19/22/23/27。

### Task 12：前端真实入口与统一生成合同

文件：frontend/src/components/conversations 及首页/会话页调用方、backend Agent handler 注解、生成 frontend/src/api、前端定向测试。当前会话工作台的输入/消息/历史合同见设计35；不保留已替换的 AgentWorkspace 入口。

- [ ] 先测创建会话/run、已受理/执行/等待批准/暂停/停止/失败/完成、重复点击和游标重连，观察旧 advance/confirm 模式不能满足断言。
- [ ] 使用项目现有组件呈现计划/真实子任务/提案/费用及产物；移除旧分阶段轮询 API。服务令牌/内部地址不进入浏览器。
- [ ] 执行 `cd frontend && pnpm api:generate && pnpm api:check`；定向 Vitest、next typegen/tsc、构建与浏览器检查。预期生成一致，断线仅断订阅，撤权关闭流。

接受：AC-AH-01/04/12/16/17/19；ST-AH-01/02/09/15/16/17/21/22/23。

### Task 13：统一 Compose、数据库角色与 CI

文件：docker-compose.yml、.env.example、agent/Dockerfile、CI、空库 SQL 与测试初始化统一使用 `backend/db/schema.sql`。已有库在部署前核对实际结构。

- [ ] 先验证配置缺 token/受限 DSN 明确失败，当前四服务拓扑无相互 healthy 等待。
- [ ] agent 仅 expose 4401、非 root/只读/资源限额，独立关闭/健康；backend 取消旧连接及7233转发，保留 frontend1122/backend1123。
- [ ] 合并本任务 SQL 到空库来源；测试两个角色相互拒绝跨 schema。离线迁移先盘点旧对象与在途，不运行现有库破坏性 DDL。
- [ ] 执行脱敏 `docker compose config --quiet`、独立 agent 镜像构建、隔离四服务集成与单服务重启；CI 增加 agent typecheck/build/test/合同检查，保留现有领域业务门禁。

接受：AC-AH-02/15/18/19/20；ST-AH-20/23/27/28。

### Task 14：失效文档、一次性切换与完整接受

文件：README、PROJECT、BACKLOG、文档索引及 rg 命中的当前 design/PRD/Plan/运行说明；保留原始用户引文与有效历史证据，标明失效假设，不让其充当当前工程指导。

- [ ] 扫描“纯Go Harness/StateGraph已接通、Go托管Node、旧阶段API、旧工作流启动/配置/健康、旧schema读取/自动回退”等错误描述，逐个对照当前源码修正；删除仅描述已删除实现的无效指南并更新链接。
- [ ] 离线列出旧 run、operation/provider request、后台 work 及费用未决清单；未清结时阻止实际切换，不能由实现顺带清库/终止远程任务。
- [ ] 独立审查本次未提交 diff，修复重要问题并以失败测试重现；检查无秘密、缓存或并行改动丢失。
- [ ] 执行文档格式/链接/稳定ID、源码残留扫描、全量 Go race、agent 官方 Storage/HTTP/多 Agent/控制故障测试、前端生成/构建和 Compose 检查。
- [ ] 夹具通过正式 profile 注册、物理准入和永久回执建立当前事实；包间分开隔离数据库及角色，禁止伪造 resource identity、回填旧接口或弱化权限/不可变约束。完整 Race 记录失败和条件型跳过；SIGKILL 恢复与镜像验证保留真实 owner/lease 证明。
- [ ] 真实供应商、费用、长剧本至可编辑工程/成片逐项留证；缺实际运行条件明确列未验收，不能用 fixture、健康页或设计批准冒称产品完整。

接受：AC-AH-01～22；ST-AH-01～35。

## 4. 接口依赖与交付判定

Task1→2/3；Task2/3→6；Task4/5与6共享 scope/grant/call/控制回执合同；Task6→7；Task8→9/10；Task4/9/10→11；Task3/4/6→12；Task1～12→13；全部→14。独立切片可并行，改同一文件必须单一写入者，数据库定义直接维护在 `backend/db/schema.sql`。

每项交付均须说明：实际文件、先失败后通过的测试、命令及输出、未覆盖外部条件。当前包能导入、旧测试通过、端口健康或文档完整均不等于该项完成。未完成项留在 BACKLOG 持续推进；不保留旧引擎作为临时完成替代。

## 5. 主子协作、MCP与自动执行实施切片

来源：PRD19 AC-AH-23～34、ST-AH-36～50，经该PRD追溯设计32 §7.6.9～10/7.11～13。以下是将来实施时的依赖和退出条件，本轮不执行。文件位置为责任落点建议，不表示新模块/表/接口已存在；不为准备这些切片预建代码目录。原Task1～14的既有门禁继续适用，不能重跑旧夹具便签收新增能力。

| 切片 | 责任落点及必要工作 | 必须退出证据 |
| --- | --- | --- |
| EX-01 主任务书与实际并行 | 现有Agent registry/docs/delegation/manifest及Go预算；固定Pi公开API原型，稳定branch到owned conversation映射，原子预算/槽预留，独立输入和结果采用 | 两个真实子会话在受控屏障下同时在途；child创建/提交边界kill后不重复；共享预算与写冲突；AC-AH-23/24，ST-AH-36/37/50 |
| EX-02 Skill自动发现与依赖 | 现有Go Skill业务和Agent加载；显式选择优先、受权候选、版本/哈希/依赖、按需加载、集合外依赖的新段修订；隔离脚本driver沿原AGH-012 | 完整生命周期及自动匹配/冲突/循环/撤权/压缩；缺driver诚实拒绝，实际脚本隔离独立验证；AC-AH-25，ST-AH-38 |
| EX-03 MCP连接与协议基线 | 现有Go业务出口及其应用/适配边界；确定目标修订的官方SDK与具体服务互通、认证主体/端点、连接配置/健康和能力发现；保持Agent扩展注册防线 | 受控真实MCP进程/HTTP服务的发现/分页/认证隔离；同名/坏schema/目录更新/停用；无凭据越权出站；AC-AH-26/27，ST-AH-39/40 |
| EX-04 MCP执行及原回执 | Go正式工具准入/预算/幂等记录，Agent显式工具绑定及内容归一；同步与Tasks/领域job分别处理，输入请求、原句柄、查询/取消和未知状态 | 远端已接单后丢回复/kill；有去重、可查询及不可查询三类；输出schema/isError、外部费用未知及取消竞态；AC-AH-27/28/31/32，ST-AH-40～42/45/46 |
| EX-05 持久等待与自动推进 | Agent现有应用doc/owner/恢复循环，Go领域结果及持久投递；等待登记后查询、消费去重、到期补扫、段/Stop校验及完成检查 | 早到/丢失/重复/乱序事件、浏览器断开和进程重启；原等待点推进一次；无模型忙轮询/长锁；AC-AH-23/29/31/34，ST-AH-43/47/48/50 |
| EX-06 显式规则（范围接受后） | Go触发规则/准入事实与Agent同一受理入口，不复制Agent计划；时区/错过策略、规则版本/启停、预算和同会话活动root冲突 | 同事件/逻辑时点只受理一次；到期补扫有界，停用/撤权阻止新触发，规则不扩大权限；AC-AH-30，ST-AH-44 |
| EX-07 故障、资源与可观察交付 | 现有事件/快照/控制与任务视图；调用链、等待原因、预算、版本、内容限额、有限重试和隔离故障 | 设计32完整故障矩阵与链路视图；Stop围栏、旧owner拒绝、无进展退出、实际资源限额及产物验证；AC-AH-31～34，ST-AH-45～50 |

接口先确定稳定的业务动作、tool binding和外部调用回执，不把整个远端SDK结构直接透传成产品合同。Agent/Go HTTP继续按现有Controller/Swagger生成链维护；schema变化在唯一schema.sql及离线审阅变更中处理，实施时不得自动迁移既有库。MCP协议版本与现有Pi运行时分别锁定；连接器互通失败不能顺带升级整套Agent或放开任意扩展注册。

依赖：EX-01与EX-02建立主子/方法合同；EX-03→EX-04；EX-01及站内正式任务回执→EX-05基础等待；EX-04→EX-05的外部MCP等待接受；EX-05→EX-06；EX-07覆盖所有切片。事件/定时规则可单独后置，但任务内自动推进、MCP调用稳定性及既有实际并行目标不能因规则尚未接受而被省略。

验证分三层：纯合同/策略测试验证选择、权限和重试分类；实际Pi+隔离PostgreSQL+受控真实MCP服务验证并发/协议/故障恢复；获准真实业务服务验证具体账号、外部费用与创作交付。第二层成功不能替代第三层；第三层需届时明确环境/目标/预算，本轮不做。缺少SDK互通、隔离driver或外部查询合同应记录对应缺口，不以跳过用例、无限重试或伪结果补齐。

## 6. 阶段产物与确定性交付补充切片

唯一来源PRD19 AC-AH-35～37、ST-AH-51～54，沿设计32 §7.14。以下继续仅文档，不执行代码或验证调用；短剧内容合同归PRD24/Plan33。EX编号延续§5。

| 切片 | 必要工作及依赖 | 退出证据 |
| --- | --- | --- |
| EX-08 阶段产物合同 | 在现有manifest/正式领域工具中分别关联Skill、产物schema与写入/采用动作，按实际业务逐项增加；依赖EX-01/02 | 实际主子会话产物经Go校验；错误结构/范围/引用、部分候选和并发修改均不覆盖正式数据；AC-AH-35，ST-AH-51 |
| EX-09 确定性复合交付 | 复用现有时间线/媒体/导出，逐阶段稳定调用身份、输入版本、原句柄与检查回执；嵌套动作保留预算/审批/停止围栏；依赖EX-05/08 | 每提交窗口kill恢复，渲染成功后只补绑定/导出，原费用不重复，Stop后无新阶段；AC-AH-36，ST-AH-52/53 |
| EX-10 批量后续准入 | Go批次冻结汇合策略、正式ready/blocked事实及后续身份，Agent统一原等待/continuation准入，持久补扫；依赖EX-05及既有batch | 终态早到/丢失/重复、消费确认丢失、用户新输入/审批/Stop/版本竞态，唯一正确接续；AC-AH-37，ST-AH-54 |

先用现有批次和真实媒体任务的故障用例表达不变量，再实施；不以纯函数mock替代跨进程Pi/PostgreSQL/Go回执验证。媒体实际解码与工程输出另验，真实模型和目标编辑器完整接受归短剧Plan33 DR-07。本轮没有新增运行、测试通过或部署记录。

## 7. 完整方案的依赖与联合门禁

唯一来源仍为PRD19，整合AC-AH-23～39、ST-AH-36～57；技术主链以设计32 §7.0为阅读入口。Task1～14仅复核实际基线的必要缺口，已经存在且有当前证据的能力不因计划仍列出而重建。以下是工程依赖，不是当前实施状态或开工授权；当前建议执行顺序见BACKLOG。

- 服务/owner/预算/正式调用回执是所有新增委派和副作用的前置。EX-01中Go预算准入意图和Agent分支映射为独立提交，新增ST-AH-55覆盖两侧所有未知窗口。
- EX-08的结构产物最小合同可与短剧DR-01生产身份先对齐，再绑定EX-01/02的真实角色和Skill；不要求先做全量Skill市场或每个专业角色。
- EX-03/04可与主子协作独立准备，外部调用进入正式出口后复用EX-05的等待合同；外部MCP自身恢复接受必须通过后才能开放该类写工具，站内生产无需等待所有外部连接上线。
- EX-05基础等待与已有Go批次回执就绪后，EX-10与DR-03共同验证任务完成后的原等待/continuation；显式规则EX-06不是任务内推进、MCP或整集制作的前置。
- EX-09与短剧DR-05共享正式渲染/绑定/导出回执，分别验证执行恢复和制作完整性，不各实现一套交付器。EX-07的故障/控制/资源门禁随每个切片实施，不留到最后补。
- 联合接受增加ST-AH-56/57：完整主子/方法/工具调用链、人工修订/Stop与迟到结果。短剧端对应PRD24的版本、采用与局部重做；具体内容只引用该PRD，不成为本计划第二产品来源。

退出结果需包含实际能力/版本清单、拒绝路径、失败注入回执、未决收费和未覆盖条件。框架原型不支持公开API目标时记录明确缺口并修订方案，不能用私有API、另起运行时或去掉真实并行来冒称完成。

## 8. 稳定服务再评估的实施门禁

唯一产品来源PRD19 AC-AH-40～44、ST-AH-58～65，对应设计32 §7.15。以下为待实施步骤，当前建议先后及状态只在BACKLOG；本轮不执行代码、容量实验、故障注入或部署。

EX-11细化EX-07的运行可靠性：先以实际代码建立调用/任务/资金/资源状态映射，验证现有owner/回执/worker而非重建；修正submission完成与交付完成的公共投影和确定性检查；对原调用增加重试分类、未知/停放、截止/下次查询和异常扫描可观察；分别实现资金配额关联及资源限流，最后完善有界排空、发布检查、灾后出站门和运行手册。所有变更在既有领域/Agent职责内，不增新运行时或资源管理服务。

| 联合门 | 必须退出的证据 | 覆盖 |
| --- | --- | --- |
| 原意图恢复 | 发送资格CAS前后及受控上游实际接单后丢响应、租约/DB/停机故障，迟到证据受限保存；查询原请求，未知不可查不重发 | ST-AH-58/62 |
| 资金/容量 | 根分配/operation预冻结关联、迟到结算；等待与活跃槽/上游容量分别处理，有界扫描和公平性 | ST-AH-59/60 |
| 完成判定 | 模型结束/回执成功但缺业务产物不报root完整；确定性版本化清单 | ST-AH-61及短剧对应接受 |
| 运行恢复 | 协议/定义/schema不符拒绝，旧快照恢复先隔离旧执行器/失效旧授权并重建代际，核对原请求/账本/对象，演练记录RPO/RTO | ST-AH-63/64 |
| 可操作性 | 队列/未知费用/资源/扫描失败和停放可解释，按手册只重驱原动作，容量与故障下控制可用 | ST-AH-65 |

EX-11不是末尾补文档：原意图/资金及完成判定先于自动批量开放，限流/等待释放槽先于提高并发，排空/恢复门先于部署切换。第三方脚本无实际隔离不开放，MCP写入缺原回执能力时明确未知人工核对；不以禁用既定目标替代接受。性能阈值/准入数从当前配置及实验确定，未测不宣称生产SLO。

## 9. 首轮可执行实施（SRC-AGH-20）

唯一产品来源PRD19 AC-AH-40～44及AC-AH-09/16/36～39；设计32 §7.15～16。以下是已有首轮的基础合同和验证入口，不再视为全部待从零建设；本次源码复核的剩余实施依赖见§10。当前SRC-AGH-22仅授权文档，不执行下列代码/环境步骤；状态与实际证据只记BACKLOG。

### P1 原请求与真实费用

文件：backend/internal/agent/adapter/postgres/{calls,authority}.go；测试backend/tests/agentbusiness/request_cost_recovery_test.go。沿ExecuteCall、CompleteCall、ReadCall、ValidateGrant，不重建模型网关或HTTP DTO。
- [ ] 写TestCallPersistsActualOverrunCostAndBlocksNewPaidAdmission、TestRecoveredOverrunSettlesOnce、TestReadCallRejectsDifferentInputHash，隔离PG观察真实失败。
- [ ] CompleteCall保存真实非负model费用（含超估），原回执仅结算一次；剩余可用预算钳0且阻止新收费，未知保留占用。ReadCall严格核对输入哈希，工具准入上限不放宽。
- [ ] go test -race ./tests/agentbusiness；复核现有operation模型网关/dispatch/恢复/结算测试。不得通过伪造账单或重复外呼制造通过。

### P2 失败恢复及交付完成保护

文件：agent/src/pi-durable/{docs,events,definitions}.ts、agent/src/sessions/{runtime,service,delivery}.ts、config.ts与business/client.ts；对应Agent测试。Go正式工具返回原交付身份，StateDoc保留引用/截止/next_wakeup_at；BusinessClient.snapshot提供权威交付事实。
- [ ] 先测试模型结束但导出queued/review_required不报root completed；隔离PG重开查询原交付，不新增模型/生成/导出，原Review通过后收敛。
- [ ] 持久等待按到期恢复扫描，复用原submission/回执；Stop/撤权保持围栏。轮次结束与作品完成分别投影。
- [ ] Agent完整测试、typecheck/build/lint、API合同和跨Go/Node恢复检查；缺真实外部条件单列。

### P3 单镜真实联合接受

制作合同由Plan33首轮单镜节唯一承接。本计划消费其shot_delivery_propose/create/get与已批准交付义务的快照，负责等待/恢复/完成保护。
退出：正式报价/批准/确认→原operation与资金→真实媒体归档→正式采用/Yjs时间线→真实FFmpeg视频及工程文件→原Review和完整状态。重复/中断不新付费、不增导出。真实供应商/目标软件人工接受独立记录，不用受控夹具代替。


### 首轮恢复与合同门禁顺序

1. 先测原HTTP查询/输入哈希/未知恢复与真实超额费用，修复Go权威和生成客户端；再接免费恢复与零预算提案，确认后续模型零上限也无法绕过付费封锁。
2. 原Source读取及存储准入随报价落入现有事务，批准/确认重验；这些前置由Plan33的真实生成闭环共同接受。
3. 先保存批准义务，再验证Go快照与原导出事实；制作能力已开放却缺字段时拒绝虚假完成，合法空集合仍可接受普通聊天。
4. waiting_delivery保存身份/截止/唤醒，查询后释放owner槽；进程重开、授权不可用、到期、暂停/恢复和Stop分别测试，并断言模型调用数不增长。
5. 原HTTP恢复、正式采用和实际产物事实完成后，再做Agent完整PG/SIGKILL、Swagger重生成比对、类型/构建/lint与服务边界检查。已知工具链告警、临时超时及复跑结果如实留在BACKLOG。

复核顺序和当前完成情况以BACKLOG为准，不将本切片当成完整MCP、Skill并行、整集制作或生产容量接受。

## 10. 本次源码复核实施依赖

唯一来源PRD19 AC-AH-45～51 / ST-AH-66～72，并继承原接受条件；技术依据经PRD追溯设计32 §7.17及其设计37依赖。现有Nest、双向生成、原调用、模型结算、delivery等待和单镜工具直接复用；不重跑“首次搭服务”任务。本节定义技术依赖顺序；SRC-AGH-23已授权R1/R2/R3与Plan33 D1实现，未授权的后续切片不据此开工。排期、完成状态及调度仍在BACKLOG。

### 10.1 接口交接与依赖图

| 交付接口（拟增量） | 权威及最小内容 | 消费者 |
| --- | --- | --- |
| RootCostSnapshot | Go：authorized_limit、pending_commitment、settled_charge、allocated_unused、unallocated、remaining及单位/币种，原call/operation/资金证据；总剩余不等于可抢占兄弟额度 | R1产出，R4并行准入、前端费用视图消费 |
| BudgetAllocation | Go：root/plan_revision/branch_key/任务书hash、额度/单位、授权revision及稳定回执；call承诺从划拨转换，不二次冻结钱包 | 随R4真实消费者产出，创建child前绑定，重复受理查原回执 |
| GoalAdmission / GoalCompletion | Go：稳定request/scope/hash、goal_kind、唯一bound_root、清单及revision、逐项缺失/质量事实；Agent仅保存引用 | R2产出，单镜D1、生产D2～D5消费 |
| ExecutionPolicy / 定义摘要 | Agent：策略版本、步数/分支/深度、冻结模型/Skill/工具，生产行为依赖清单及hash | R3产出，R4/R5与发布盘点消费 |
| WaitRecord | Agent：wait_id/type、原refs、control/plan revision、首次/绝对截止/唤醒、attempt/原因；Go job保持原身份 | R3产出，R4/R6/单镜及整集等待消费 |
| DelegationBatch | Agent：稳定batch/branch_key、任务书hash、owned conversation/submission、角色/预算和独立结果来源 | R4产出，专业产物与QA消费；不作为正式Take采用 |
| SkillSelection / McpToolBinding | 前者复用Go版本化资源元数据，后者Go连接/认证/工具/schema/效果/恢复政策快照 | R5/R6产出，Agent只消费冻结且受权集合 |

依赖：`R1 + R2 + R3 → Plan33 D1（现有单镜联合接线）`；`R1/R3 → R4`；`R3 → R5`；`R1/R3 → R6`；`D1 → Plan33 D2 → D3 → D4 → D5`。R4实际专业并行供D3使用；R5可在受权现有Skill范围内先用手动选择，不阻塞D1。R6只读技术互通可独立验证，外部收费/写工具不可早于R1/R3；站内单镜与正式生产身份不依赖MCP全部完成。R7验证每个已开放切片，不等到最后才补恢复。

### R1 原请求与费用投影收敛

文件：复用`backend/internal/agent/{domain/business.go,adapter/postgres/calls.go,adapter/postgres/authority.go}`、`app/agent/creation.go`、`operation`资金终态与既有billing接口；增量`agent/adapter/postgres/cost_projection.go`及对应业务DTO。结构变更仅在`backend/db/schema.sql`。测试扩展`backend/tests/agentbusiness/request_cost_recovery_test.go`，增加`root_cost_projection_test.go`。

消费原CallIdentity/operation/reservation/settlement，产出RootCostSnapshot，不改变billing唯一结算权威。

- [ ] 先写失败用例：Quote承诺转媒体实际收费/退款、重复/丢通知后对账、unknown继续占用、模型超额及采购/用户价不同；R4再验分支划拨回应丢失/转call/回收竞态；本切片断言不把allocation、报价与实际重复累计，不新扣钱包。
- [ ] 用原operation终态/资金回执更新根费用分类投影，保留原调用hash与一次发送资格；当前按原身份和账本快照对账，BudgetAllocation及转换/回收随R4消费端再实现，归集失败禁新收费但不禁止原免费恢复。
- [ ] 运行`go -C backend test -race ./tests/agentbusiness ./tests/operation`并确认目标用例实际执行；核对原流水/原POST计数和单位/币种。已有超额测试继续通过，不能为并发先删除根未知阻断。

### R2 制作受理与完成义务

文件：Go `agent/domain/{business,goal}.go`、`agent/adapter/postgres/{authority,goals}.go`、`agent/adapter/http/business*.go`、`app/agent/shot_delivery_query.go`与公开conversation入口；Node `http/session-request.dto.ts`、`sessions/{input,types,manifest,delivery}.ts`；现有双向生成链。新增测试`backend/tests/agentbusiness/goal_admission_test.go`，扩展`agent/tests/{delivery-contract,conversation-contract}.test.ts`。

消费用户明确意图/正式来源，扩展现有Admission/run_authorization的GoalContract和GoalCompletion；公开Start已确定稳定root并先AdmitRun，不建立第二受理表。

- [ ] 先测聊天空集合可完成、production待规划/无交付提案不可完成；受理后Node/网络中断、重复request、异参hash和跨root绑定冲突均不产生第二制作root。
- [ ] Go在原授权中先保存目标与内容hash，Node按原request/root幂等受理，Go覆盖转发manifest中的目标；快照返回版本化必需项，deliveryOutcome检查目标义务和原shot交付，而非只对空数组every。模型输出不能改目标类型。
- [ ] 执行上述Go/Node目标用例与`pnpm --dir agent api:check`，检查生成接口、实际HTTP和缺字段拒绝；Agent单测须使用隔离受限PG。正式清单尚未形成时保留待规划，不用假完整清单通过。

### R3 策略冻结、恢复泵与关闭失败

文件：Agent `pi-durable/{definitions,delegation,context,registry}.ts`、增量`pi-durable/policy.ts`、`sessions/{manifest,runtime,service,delivery}.ts`、按职责需要增加`waits.ts/recovery.ts`；Go `platform/taskruntime`仅补仍存在的时钟/领取缺口，不新造worker。测试`agent/tests/{manifest,registry-budget,lifecycle-recovery,recovery-process}.test.ts`与新增`recovery-diagnostics.test.ts`。

消费原owner/epoch/控制/交付等待，产出ExecutionPolicy、完整定义摘要及WaitRecord。R1/R2的协议变化一起纳入新定义，在发布前盘点原run。

- [ ] 先测行为依赖修改改变摘要、相同生产输入稳定，40/39/8等策略约束一致；wait截止重开不延长，扫描连续失败/poison项/close失败可见且不早放lease。
- [ ] 集中所属策略、扩展冻结摘要到行为依赖；扫描分类退避/停车、原定义不兼容清单和最近成功/最老年龄；保留清grant先于收尾，失败租约自然过期，不恢复新付费动作。
- [ ] 在隔离PG执行对应进程kill/失DB/双owner用例，再运行`pnpm --dir agent typecheck`、`pnpm --dir agent build`；错误不能只catch丢弃，新增目录不可只有转发。

### R4 真实分支并行与有界收费准入

文件：`agent/src/pi-durable/{delegation,registry,policy}.ts`、`docs.ts`、`sessions/manifest.ts`；Go R1原费用准入。测试扩展`agent/tests/delegation.test.ts`、新增`delegation-parallel.test.ts`及Go `root_cost_projection_test.go`。

消费R1/R3，产出DelegationBatch；保留单模型默认，角色模型覆盖仅在真实目录/授权合同扩展后启用。

- [ ] 两个branch在屏障上同时在途；分别在create/submit/result commit边界kill，断言同branch同child/请求，失败只影响自身；工具越权、递归委派、非采用来源和总步数超限均拒绝。
- [ ] 原子保存branch映射/分配，独立submit/wait/结果采用；保持Go正式写链串行。按已核验费用上界原子预留每个模型call后才放行真正收费并行；不满足的供应商保持保守限制。
- [ ] 执行真实Pi+PG并行/恢复和Go根额度竞态；核对实际模型调用重叠与资金占用，不能仅断言两个conversation行存在。ST-AH-69及原ST-AH-36/50/55均保留。

### R5 Skill自动选择与依赖

文件：复用Go `agent/adapter/postgres/skills.go`及`domain/skill.go`；Agent `pi-durable/{skills,context}.ts`、`sessions/manifest.ts`。测试扩展`agent/tests/{skills,skills-extensions,compaction-context}.test.ts`。

- [ ] 先测显式选择优先、受权自动匹配、依赖缺失/循环/冲突/撤权和压缩恢复；没有隔离driver的脚本不能加载执行。
- [ ] 在现有不可变Skill版本资源中加入元数据/闭包，按需载入冻结集合并显示依据；越出集合走原授权/continuation，不自动安装npm/git包。
- [ ] 实际Pi消费结构产物并由Go正式工具验证，运行对应测试；Skill加载成功不能替代制作落库/采用证明。

### R6 正式MCP连接与原调用

只有开始交付真实协议时才新增`backend/internal/mcp/{domain,application,adapter/http,adapter/postgres,adapter/protocol}`及对应app接线；锁定官方Go SDK候选基线，复用Go Agent原工具/费用/工作记录。HTTP端口/DTO经专用Swagger生成到Node，不开放任意server/tool参数的万能出口。测试新增`backend/tests/mcp/connection_integration_test.go`、`call_recovery_integration_test.go`。

- [ ] 先以受控真实Streamable HTTP服务验证认证主体隔离、分页/重名/schema/配置变更/停用、isError/输出上限；连接/工具绑定必须实际注册并被Pi可达。
- [ ] 再测远端已写丢回应、有原job/无查询/有幂等三类、取消非终态与中途输入；把句柄及未知写入落原调用/工作记录，有限调度原查询，未知不重发。能力不支持则明确不可用，不伪造Tasks。
- [ ] 执行`go -C backend test -race ./tests/mcp`和跨Node/Go协议用例，检查原远端写计数/费用。stdio及外部收费分别有driver/账号/预算门禁，不能凭SDK安装签收。

### R7 每个切片的运行与发布门禁

复用现有官方Storage conformance、Go/Agent合同、PG进程恢复和Plan33真实FFmpeg测试。扩展现有CI job，不新增空“已完成”接口；每个切片完成后运行其有风险的边界，再统一`pnpm --dir agent test`、`api:check`、`lint`、`format:check`、`build`及Go适用门禁。

验收环境必须显式提供隔离`GLOWLOOM_AGENT_TEST_DB_DSN`、业务专用测试DSN和可达对象协议夹具；单镜用`GLOWLOOM_TEST_SHOT_PIPELINE_DB_DSN`，缺变量导致skip时该门记为未验证。不得打印真实DSN/凭据、运行全schema覆盖现有库或用mock替代数据库测试。镜像/定义/角色/schema核对、停新受理、在途/未知费用清结后再单版本切换，不为部署新增兼容引擎。

SRC-AGH-23已授权R1/R2/R3与Plan33 D1实现和本地验证；原文档调研证据不冒充当前执行结果。实际完成状态、测试命令与未验边界统一见BACKLOG。后续R4～R7、真实供应商与作品接受仍分别留证。
