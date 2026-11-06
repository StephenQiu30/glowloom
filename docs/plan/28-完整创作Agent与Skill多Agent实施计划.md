# 完整创作 Agent、Skill 与多 Agent 实施计划

> 执行方式：使用 writing-plans、executing-plans；独立责任切片按 subagent-driven-development 执行，核心行为 Red → Green → Refactor。用户已授权补齐计划并直接实施，无需再次确认实施方向；不提交或推送。

**目标**：站内创作入口实际运行同仓库独立 agent 服务，以官方 Pi Durable 和 PostgreSQL 承载完整 Harness、Skill 与真实专业子会话；Docker Compose 为唯一启动入口，删除旧执行实现及兼容路径。

**唯一来源 PRD**：[PRD19](../prd/19-完整创作Agent与Skill多Agent.md)。技术合同通过该 PRD 追溯设计32；本计划不建立第二份需求来源。当前进度、失败和下一步仅维护在 [BACKLOG](../../BACKLOG.md#pi-durable-migration)，下列步骤是执行合同，不是完成记录。

## 1. 全局约束与文件责任

- 服务/目录/包/Compose/CI 名称统一 agent；TypeScript strict、原生 ESM、Node 24、官方 Pi 1.5.0 同版包、Fastify、TypeBox、pg、Pino、Vitest、pnpm/tsc；精确版本唯一见设计32 §7.1.2。
- agent 独占会话、执行段、计划、owned conversation 与执行恢复；Go 独占身份/scope、审批、预算、模型/业务工具及业务回执。无跨 schema 写入，无 Go 启动 Node。
- HTTP TypeBox schema/operation 元数据生成内部 OpenAPI 与 Go 客户端；公网 Go 注解仍生成前端 API，生成文件不得手改。
- 生产只用 PostgreSQL；正式空库来源 backend/db/schema.sql。独立增量 SQL 供离线审阅执行；应用启动不执行 DDL，不重建既有业务库。
- 不保留旧图、旧单图 Agent 服务、stdio/launcher、旧路由、旧状态翻译、旧环境变量别名、旧任务接管、双轨或自动回退。Skill 产品自身的版本选择不等于旧运行时兼容。
- 普通业务后台任务保留原领域工作/供应商请求/财务回执，以数据库领取、租约、有限重试和到期补扫替代原工作流基础设施；不引入通用 DAG 引擎。
- 改动前保存工作区路径清单，逐文件保留并行存储/模型/画布/探索页修改；用户未要求新分支或 Git 交付，沿当前工作区实施，禁止 reset/clean/广泛暂存。
- 新服务及隔离验证可执行；现有在途任务、现有数据物理删除与真实收费调用须以精确目标和既有授权处理。旧任务未清结时阻止生产切换，不编写兼容接管器。

| 文件范围                                                           | 唯一责任                                                           |
| ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| agent/package.json、锁文件、tsconfig、Dockerfile                   | 独立构建与固定依赖                                                 |
| agent/src/adapters/postgres/pi-durable、agent/tests/storage        | 官方 Storage PostgreSQL 实现、owner/fencing、完整一致性测试        |
| agent/contracts、agent/src/transport/http、agent/src/server.ts     | TypeBox 单源合同、认证、HTTP/SSE、关闭                             |
| agent/src/runtime/pi-durable、agent/src/sessions、agent/src/integrations/go-business-api | 官方执行组合、应用 doc、控制投递/回执、Go 客户端 |
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

文件：agent/src/adapters/postgres/pi-durable/postgres.ts、records.ts、ownership.ts；agent/tests/storage/conformance.test.ts、recovery.test.ts；backend/db/schema.sql。

接口：openPostgresStorage(pool, sessionId, owner) 返回官方 Storage 及 close；owner={id,epoch,leaseUntil}。以 session 为 Storage 分区，包含根与全部 owned children；SQL 静态定义，不在 open 中建表。

- [ ] 先接完整 registerStorageConformance，及原子批次/拒绝前缀/请求去重/文档历史/对象隔离用例，运行观察未实现失败。
- [ ] 实现官方 typed records、历史、全局 ID、批次提交/receive 与扫描语义；事务检查 epoch/lease，COMMIT 确认后才 receive。
- [ ] 增加两个 owner 竞争、失租约、未知 COMMIT、连接断开及重开测试。事件投影与 Pi 批次同一事务，不能提交后另补。
- [ ] 执行 `cd agent && pnpm test -- tests/storage`，使用明确隔离测试库；未给测试 DSN 时测试应明确失败，不静默 skip。预期官方套件与故障用例通过。

接受：AC-AH-02/13/15/18；ST-AH-20/24/26/27。

### Task 3：当前服务合同与只读/事件接口

文件：agent/contracts/*.ts、agent/src/transport/http/app.ts、agent/src/sessions/port.ts、agent/src/server.ts、agent/tests/http.test.ts；内部 OpenAPI 导出及生成 Go 客户端。

接口：/internal/agent/sessions、/sessions/:sid/runs、/runs/:rid/commands、/runs/:rid/events；请求含 request_id/内容哈希/scope，输入 expected_execution_revision 与控制 authorization_revision 分开；snapshot 返回同一一致性点的 snapshot_cursor。

- [ ] 先测认证失败、伪造 scope、同键异内容冲突、只读不启动任务、SSE 断线不 abort。
- [ ] 仅从 TypeBox 路由 schema 生成验证/TS/OpenAPI；实现有界 body、流背压与脱敏错误。
- [ ] 测同一事务状态/事件可见、游标过期 snapshot_required、快照接续无缺口、丢通知补读及慢订阅。
- [ ] 执行 `pnpm typecheck && pnpm test -- tests/http.test.ts && pnpm api:check`。预期合同生成一致且行为通过，不需要模型/业务库启动来导出 schema。

接受：AC-AH-12/18/19/20；ST-AH-21/26/27/35。

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

文件：backend/internal/agent 业务模型/工具适配，复用 catalog/operation/canvas/script/subject/media 的正式应用命令；agent/src/integrations/go-business-api/client.ts、agent/src/runtime/pi-durable/model-provider.ts、registry.ts；对应双方测试。

接口：/internal/agent-business 的 grants、snapshot、tools、skills、proposals、decisions、models 与 calls。调用键固定 run/执行段/Pi task/call/请求边界，并绑定不可变参数哈希；未知结果先查 calls，不换 ID。

- [ ] 先测 Go 业务已提交但 Agent 未存结果时 kill/重试只返回原结果，供应商未知接单不发新请求、不重复预留。
- [ ] 模型规划/压缩/子会话统一出站，供应商主密钥留 Go；输出含真实 usage 与调用回执。工具描述唯一来自 Go，可信 scope 不来自模型参数。
- [ ] 真实接线已有业务能力，禁止空实现、固定成功或把 unsupported 当作已覆盖。隔离脚本设施不存在时明确拒绝。
- [ ] 测参数篡改、嵌套工具、价格/画布版本、撤权、费用与连续写链；分别执行双方定向测试与跨 HTTP 测试。

接受：AC-AH-05/06/07/10/11/16/18；ST-AH-03/04/09/13/14/15/32/34。

### Task 6：官方 Harness 与执行段生命周期

文件：agent/src/runtime/pi-durable/registry.ts、trusted-extensions.ts、model-provider.ts、docs.ts；agent/src/sessions/service.ts、runtime.ts、manifest.ts；agent/tests/controls.test.ts及相关Harness测试。

接口：SessionService 接受输入/控制但不自写模型循环；registry 显式选择工具/扩展，manifest 固定 SDK/定义/工具/模型/Skill/hash；每 session 同时只有一个活动业务 root_run。

- [ ] 先用官方 Harness 写模型—工具—模型、重复 requestId、manifest 不符和只读访问测试，观察缺失失败。
- [ ] owner 成立后 open；所有 submit/wait/resume 执行窗口都需授权。配置 maxRetries=0，关闭 provider 自动重发，保留官方 interrupted/faulted 结果。
- [ ] 在模型出口与工具 execute 强制检查；测试 beforeRequest/beforeTool 抛错仍不越权。持久化每个模型请求 checkpoint 的身份映射，不用 span ID 或内存计数器替代。
- [ ] 提案事务建立整树围栏；提案工具及被阻断的工具结果 terminate，官方 abort 收束兄弟 owned work。持久 quiesced_receipt 后 Go 才签新 continuation 段 grant；同一 commit 接受 continuation 及应用记录。
- [ ] 执行 `pnpm test -- tests/harness tests/controls`。预期多工具/兄弟任务、批准早到、ack 丢失、旧段迟到与三个握手 kill 窗口通过。

接受：AC-AH-01/02/04/07/13/21/22；ST-AH-01/02/05/06/24/25/29/30/32/33/34/35。

### Task 7：真实专业 Agent、Skill 与压缩

文件：agent/src/runtime/pi-durable/delegation.ts、skills.ts、context.ts、trusted-extensions.ts；agent/tests/delegation.test.ts、skills.test.ts及压缩上下文测试。

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
