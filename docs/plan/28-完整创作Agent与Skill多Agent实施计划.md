# 完整创作 Agent、Skill 与多 Agent 实施计划

唯一来源PRD：[PRD19](../prd/19-完整创作Agent与Skill多Agent.md)。**目标：** 按PRD建立采用成熟SDK的创作Harness、完整Skill接入及实际多Agent服务，最终交付可恢复、可编辑的短剧工程与成片。**架构：** 独立Node Agent Host运行Pi Durable；现有Go业务服务持有身份、授权、费用、工具/产物权威；Temporal只推进业务任务。**技术：** Pi `1.3.0`、Go、PostgreSQL、既有Temporal/outbox/inbox/实时通道、Next.js及专用Linux Docker+gVisor执行节点。

Spec由PRD19唯一来源的设计32持有，本文通过PRD追溯，不建立第二来源。实施者需同时阅读PRD及其设计，任务按`superpowers:executing-plans`执行；需要子任务评审时可使用`superpowers:subagent-driven-development`，但不得据此扩大业务范围。本文定义执行步骤，实施授权见需求 SRC-AGH-08；业务环境变更和真实费用仍需对应明确授权；状态、调度、优先级与阻塞只在 [BACKLOG](../../BACKLOG.md) 维护。

## 1. 全局约束与依赖

- 采用SDK而不是重新实现模型循环；固定集成版本，SDK升级不自动跟随latest。Node满足上游`>=22.19.0`，引入时固定补丁/镜像digest与pnpm锁文件。
- “新引擎不继承现有单图 Agent 的三阶段、20轮或最近4轮上下文限制。”旧引擎仅负责旧历史与在途任务。
- “Temporal 继续推进外部生成、媒体和脚本业务任务，不执行或重放 Pi 的模型循环/子 Agent 调度。”
- Go `adapter → application → domain`；跨模块小接口由`internal/app/agent`、`app/skill`等接线，不直接调用其他领域应用服务或数据库适配器。
- 所有Go测试放`backend/tests/<模块>/`；Host测试放`agent-runtime/tests/`；前端测试放`frontend/tests/`。不为文档或预备阶段创建空目录、脚手架与运行依赖。
- 所有费用含首次规划、压缩、子Agent与脚本；规划批准不能授权媒体/脚本；Go屏障与预算事实优先于Host缓存状态。
- “Skill 指令是方法数据，不是系统权限来源。”不加载第三方JS扩展到Host，不在宿主执行Skill脚本；无沙箱不得宣称完整Skill完成。
- 只扩展唯一`docker-compose.yml`，不新增应用宿主端口/Compose入口。专用执行节点属于新部署依赖，不能使用用户共享Docker daemon作默认沙箱。
- 数据库只有`backend/db/schema.sql`为空库来源；存量使用单独增量，不重放全库。服务启动、业务库迁移、真实计费、Git提交与推送都需要对应明确授权；本计划本身不执行这些动作。
- 界面先补Figma状态与回读再代码；沿用Next.js/shadcn、生成API、同站BFF与既有主题/组件，不手改`frontend/src/api`。

外部依赖：可用且获授权的模型/音色/媒体服务、Host PostgreSQL 存储、一致备份对象存储、专用Linux执行节点及runsc/mTLS/镜像白名单。缺项应记录对应用例未执行；不能改用mock结果关闭真实接受。

## 2. 文件责任与构建关系

以下路径按任务实际实现，不预建空目录或无消费者的脚手架。文件名可在同一职责内随实际API生成调整，但调整不得改变设计合同，必须更新本计划与证据。

| 责任           | 主要创建/修改位置                                                                                                                                                                                                      | 所有者/边界                                   |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| SDK Host       | `agent-runtime/package.json`、`pnpm-lock.yaml`、`src/host.ts`、`session-owner.ts`、`protocol.ts`、`tests/`                                                                                                             | 真正SDK和持久存储，不含Go业务规则             |
| Go控制与宿主桥 | `backend/internal/agent/domain/{run,command,approval,manifest}.go`、`application/{run,control,projection}.go`、`adapter/{http,postgres,runtimehttp}/`、`backend/internal/app/agent/`                                   | 控制命令/批准/事件，不重建模型调度器          |
| 模型交换       | `backend/internal/agent/application/model_exchange.go`、`agent/adapter/postgres/model_exchange.go`、`operation/application/model_dialogue.go`、`operation/adapter/providerhttp/`、`agent-runtime/src/model-gateway.ts` | Go费用与供应商权威，Host对话/逻辑步骤         |
| 工具注册/适配  | `agent/application/{tools,invocation}.go`、`agent/adapter/http/runtime_tools.go`、`internal/app/agent/`、`agent-runtime/src/operation-bridge.ts`                                                                       | 有限工具，应用用例注入                        |
| Skill业务      | `backend/internal/skill/{domain,application,adapter}/`、`backend/internal/app/skill/`                                                                                                                                  | 包/安装/上下文/发布/脚本Job                   |
| Skill Host扩展 | `agent-runtime/src/{skills,skill-resources,skill-context}.ts`                                                                                                                                                          | 固定Pin、按需资源与压缩恢复                   |
| 多Agent        | `agent/domain/{definition,delegation}.go`、`agent/application/delegation.go`、`agent-runtime/src/{agents,delegation,handoff}.ts`                                                                                       | 角色定义、子会话、回收/取消与限制             |
| 剧本/分镜/交付 | 现有`script`、`subject`、`bible`、`canvas`、`media`、`mediatool`用例；新`storyboard`模块                                                                                                                               | 不把镜头业务塞进聊天记录或Host                |
| 隔离执行       | `skill/adapter/sandboxhttp/`、`skill/adapter/oci/`、`internal/app/skill/sandbox_server.go`、`cmd/glowloom/sandbox.go`                                                                                                  | 同二进制子命令，专用引擎，Job永不挂控制socket |
| UI/API         | `frontend/src/components/agent/`、`app/skills/`、`app/storyboard/`、现有创作/画布入口、后端注解及API生成                                                                                                               | 公共组件不反向导入页面私有实现                |
| 部署/迁移      | `backend/db/agent_harness_incremental.sql`、`skill_incremental.sql`、`storyboard_incremental.sql`、`schema.sql`、现有角色/Compose/Dockerfile/PROJECT                                                                   | 只在实现任务中按作用域修改，不覆盖并行修改    |

## 3. 评审重点与对应测试

1. 模型请求与SDK恢复身份不同步导致重复收费：T-AH-01/03/14必须测试逻辑taskId、完成重放、未知阻断、同文不同任务。
2. Go停止已受理而Host尚未收到：T-AH-02/04/08测试控制屏障、已准入任务与新增副作用的区别。
3. Skill更新/撤销、压缩与子会话缓存共同导致权限扩大：T-AH-05/06/08测试exact pin、上下文修订、即时deny与继承收敛。
4. 工程协作改动和旧快照恢复覆盖已发生业务：T-AH-04/10/13测试CAS、失效图与Go回执对账。
5. 只有演示/受控测试而缺真实隔离/媒体/费用证据：T-AH-07/11/14区分环境并证明实际产物及隔离边界。

## 4. 实施任务

任务采用Red → Green → Refactor；先记录实际失败断言，再实现最小可验证行为，最后运行相关回归。以下命令均为**实施时要执行的命令**，不是本轮执行记录；涉及未来测试文件时，必须先创建对应用例。各任务的退出条件均需版本化证据，不在这里打勾记录当前进度。

### T-AH-01 SDK Host、身份与真实恢复合同

依赖：设计评审；对应AC02/17、ST-AH-01/20/23。文件：新增`agent-runtime/package.json`、锁文件、`src/{host,session-owner,protocol}.ts`；测试`tests/{session-owner,model-identity,compaction-identity}.test.ts`。

消费：Go签发的`RuntimeCommand`/`RuntimeIdentity`协议样本。产出：`openOwnedSession(identity): Promise<OwnedSession>`、`acceptCommand(envelope): Promise<CommandReceipt>`、`snapshot(sessionId): Promise<RuntimeSnapshot>`；内部调用真实Pi Harness，所有声明以设计32 v1协议为准。

1. 按 SRC-AGH-07 使用 PostgreSQL 保存全部 SDK 数据，先运行上游 Storage conformance 及真实数据库并发、ID 分配、提交断连与 kill 恢复测试；固定存储 ABI，应用启动不执行 DDL。先固定Pi `1.3.0`的实际导出/HookApi.taskId、memo、子会话ownership及存储行为，写受控模型的失败测试：崩溃后原taskId/memo保持、不同task同正文不会合并、压缩不能绕过网关、恢复同一submission。
2. 运行`pnpm --dir agent-runtime exec vitest run tests/session-owner.test.ts tests/model-identity.test.ts tests/compaction-identity.test.ts --maxWorkers=2`，记录失败；不能用自写假Harness代替上游SDK。
3. 以固定依赖实现Host、per-session PostgreSQL advisory lock、epoch fencing 与 SDK Storage 适配器、命令request_id持久准入、快照、错误/版本报告及无业务Key环境。Provider按conversation绑定，未取得持久exchange memo的请求拒绝；beforeCompact使用受控摘要交换。
4. 复跑上述测试、TypeScript与打包入口加载测试；输出SDK/Node版本和锁文件摘要。发现上游接口与文档不符时修改适配合同并重新评审，不能静默fork上游或接入全局fetch“最近请求”猜归属。

退出：真实SDK进程重启后保持会话、逻辑身份和原请求；无独立Go模型循环、无CodingTools/任意shell、无自动Key发现。

### T-AH-02 Go控制账本、计划批准与屏障

依赖：T01协议；AC-AH-01/09/12、ST-AH-01/02/05/15。文件：`agent/domain/{run,command,approval,manifest}.go`、`application/{run,control}.go`、`adapter/postgres/`与`adapter/http/harness_handler.go`，`app/agent/`接线，`backend/db/agent_harness_incremental.sql`及空库schema；测试`backend/tests/agent/{control,approval_manifest,control_store_integration}_test.go`。

消费：`RuntimeCommand`、`PlanManifest`。产出：`StartRun(ctx, actor, projectID, sessionID, StartRunInput) (RunReceipt,error)`；`ApplyCommand(ctx, actor, runID, CommandInput) (CommandReceipt,error)`；`RegisterPlanCandidate(ctx, RuntimeIdentity, PlanManifest) (ApprovalCandidate,error)`。接口由agent应用消费方定义、构造注入。

1. 写失败测试：重复request同输入返回原结果、不同输入冲突；首次提交不调用模型；未批准planning cap无模型请求；SDK计划未注册Go candidate不能批准；修改计划先废旧批准。 planning的GoalManifest允许授权script.read/skill.file而无需execution step；同阶段拒绝generation.submit、skill.run、agent.delegate和提案写入。
2. 写真实隔离PG并发测试：Go接受stop/pause事务立即关闭准入、Host断线仍生效、旧control_revision拒绝、已准入调用列为在途、resume只重开paused且重验权限。
3. 实现不可变candidate/hash/批准、根预算信封、会话engine映射、命令outbox及dispatcher；外部调用不得在持有数据库锁的事务内等待Host。所有读重验项目/actor范围。
4. 运行`go test -race -count=1 ./tests/agent -run 'Test(Control|Approval|Plan|RunAdmission)'`（backend目录）；PG测试使用项目既有隔离库约定，缺环境不跳过后称通过。

退出：控制与批准有确定线性化点；仅以Go计划证据批准；Go不会因为HostACK丢失创建第二Run。

### T-AH-03 模型对话网关与根费用

依赖：T01/02；AC-AH-10/11、ST-AH-13/14/20。文件：`agent/application/model_exchange.go`、`agent/adapter/postgres/model_exchange.go`、`agent/adapter/http/runtime_models.go`、`operation/application/model_dialogue.go`、对应providerhttp对话适配、billing消费接口、`agent-runtime/src/model-gateway.ts`；测试`backend/tests/agent/model_exchange_test.go`、`backend/tests/operation/model_dialogue_test.go`、`agent-runtime/tests/model-gateway.test.ts`。

消费：冻结model release、planning/execution批准、`(session,conversation,taskId,kind)`逻辑键。产出：`AdmitModelExchange(ctx, RuntimeIdentity, ModelExchangeInput) (ModelExchangeReceipt,error)`与受控流事件；文本/工具/多模态/usage数据合同。

1. 用真实HTTP受控供应商写失败测试：多轮tool_call/result不丢ID；同逻辑键相同输入重放、不同输入冲突；相同正文不同task合法；网关受理后断流/kill不新增物理请求；无usage不计零。
2. 测试首次规划、compaction、多个子Agent原子预留，达到cap后无新请求；native-tools/JSON兼容模型明确分流，缺视觉能力拒绝，拒绝后不自动换模型/凭据。
3. 实现Go canonical请求、参数冻结、物理attempt与账本、已完成结果重放、未知对账、当前授权媒体解析；Host所有请求经过网关，包括自供压缩摘要。关闭绕过网关判定的SDK retry。
4. 运行`go test -race -count=1 ./tests/agent ./tests/operation ./tests/billing -run 'Test(ModelExchange|ModelDialogue|HarnessBudget)'`及Host模型网关测试；具体供应商适配更改另跑对应原协议回归。

退出：未用真实Key也能证明协议/故障语义；真实供应商工具兼容与计费另验，不能用mock关闭AC11。

### T-AH-04 工具注册、幂等桥与业务投影

依赖：T02/03；AC-AH-02/09/12/13、ST-AH-02/15/17/20/21。文件：`agent/application/{tools,invocation,projection}.go`、`adapter/http/runtime_tools.go`、`adapter/postgres/{invocation,projection}.go`、`app/agent/{tools_wiring,canvas_tools,media_tools}.go`、`agent-runtime/src/{operation-bridge,projections}.ts`；测试`backend/tests/agent/{tool_contract,invocation_store_integration,projection}_test.go`、`agent-runtime/tests/operation-bridge.test.ts`。

消费：ToolDescriptor、冻结identity/control/candidate、各领域小接口。产出：`InvokeTool(ctx, RuntimeIdentity, ToolInvocationInput) (ToolReceipt,error)`；`RecordProjection(ctx, RuntimeIdentity, Projection) (ProjectionReceipt,error)`。

1. 写失败测试：模型伪造actor/project/callID/URL被剔除或拒绝；缺工具/schema不符不能执行；重复写入返回原业务回执；旧epoch/control/candidate无新副作用。 分别校验planning/execution的approval_context，planning只读已授权来源及选中Skill，解析子Agent只有execution批准后可委派。
2. 用PG与HTTP故障点证明Go写入后HostACK丢失仍同一operation；同seq不同hash报错；SDK丢中间帧与慢消费者读取快照，不承诺token重放。
3. 实现注册表投影、参数/输出校验、回执、只读查询、取消、当前版本冲突和既有outbox/inbox/实时通道接入。确认幂等之前不得把写工具标记replay-safe。
4. 运行`go test -race -count=1 ./tests/agent -run 'Test(Tool|Invocation|Projection|RuntimeScope)'`及Host对应测试；随后运行受影响canvas/media应用回归。

退出：工具通过正式业务用例；Go事务不等待模型；业务回执与模型总结分离。

### T-AH-05 Skill包、版本、安装与发布生命周期

依赖：T02、对象存储与身份小接口；AC-AH-05/06、ST-AH-07/08/09。文件：`skill/domain/{package,manifest,pin,dependency}.go`、`application/{install,versions,registry,publishing}.go`、`adapter/{archive,postgres,http,sourcehttp,objects}/`、`app/skill/`、`backend/db/skill_incremental.sql`与空库schema；测试`backend/tests/skill/{package,install,versions,publishing}_test.go`。

消费：`SKILL.md`、`glowloom.skill/v1`、来源commit及scope。产出：`InstallSkill(ctx, actor, InstallInput) (InstallationReceipt,error)`、`ReadPinnedResource(ctx, actor, Pin, path) (Resource,error)`、`ChangeInstallation(ctx, actor, InstallationCommand) (InstallationReceipt,error)`。

1. 先写包身份和攻击样本测试：规范摘要、重复路径、ZipSlip、symlink、压缩炸弹、依赖循环/不匹配、只Markdown兼容。GitHub可变ref解析为commit且禁止运行安装脚本。
2. 加入隔离PG/对象存储测试：本人/团队/公开授权，更新/回退不改历史pin，fork/导出身份一致，送审/拒绝/撤回不泄露私有包，下载失败不留下ready版本。
3. 实现不变版本与可变安装指针、manifest能力兼容、精确依赖闭包、来源/许可元数据与受控源获取器；复用既有存储与审计，不把prompt模板表当包仓库。
4. 运行`go test -race -count=1 ./tests/skill -run 'Test(Package|Install|Version|Publish)'`。记录初始32MiB/1024文件/256KiB入口边界测试。

退出：完整生命周期可由正式API验证；包中代码不会在导入/预览阶段执行。

### T-AH-06 Skill活跃上下文与Harness接入

依赖：T01/04/05；AC-AH-07、ST-AH-10/11。文件：`skill/application/context.go`、`skill/adapter/postgres/context.go`、`agent-runtime/src/{skills,skill-resources,skill-context}.ts`；测试`backend/tests/skill/context_integration_test.go`、`agent-runtime/tests/{skill-context,skill-compaction}.test.ts`。

消费：安装pin、兼容能力与会话scope。产出：`ChangeSkillContext(ctx, actor, SkillContextCommand) (SkillContext,error)`；Host `loadPinnedSkill(pin)`、`restoreActiveSkills(context)`与catalog/load/file工具。

1. 写失败测试：目录只含摘要，未安装/无权限pin无法加载；资源hash/路径不符拒绝；正文超上下文预算不静默截断关键规则。
2. 真实SDK长会话触发压缩及kill恢复，检查相同pin与资源约束；普通禁用、活跃释放、更新安装指针、紧急撤销分别测试；并发context revision冲突。
3. 实现按需读取、预算分配、活跃上下文CAS及持久回执、beforeRequest/compaction后的固定规则恢复，子Agent只接收到被批准的Skill交集。
4. 运行`go test -race -count=1 ./tests/skill -run TestSkillContext`与Host两组测试，证明未经执行的Skill脚本不进入Host扩展注册表。

退出：Skill不是枚举或提示词开关；生命周期和压缩恢复均可独立验证。

### T-AH-07 专用隔离脚本服务与Job回执

依赖：T04/05、专用Linux执行节点；AC-AH-08、ST-AH-12/20。文件：`skill/application/{sandbox_job,sandbox_policy}.go`、`adapter/sandboxhttp/`、`adapter/oci/`、`adapter/postgres/sandbox_job.go`、`app/skill/sandbox_server.go`、`cmd/glowloom/sandbox.go`；部署在唯一Compose增加skill-sandbox；测试`backend/tests/skill/{sandbox_contract,sandbox_job_integration,sandbox_isolation}_test.go`。

消费：已批准pin/image digest/argv/input refs、根预算与control revision。产出：Go `SandboxDriver.Create/Read/Cancel/Outputs`，对应设计32四个`/v1/jobs`接口；持久Job→容器ID及可复查策略摘要。

1. 先用受控Engine适配测试幂等创建、创建ACK丢失、cancel竞态、未知查询、镜像/argv白名单、配额和输出大小拒绝，缺runsc/证书/专用daemon拒绝启动执行。
2. 实现可信sandbox服务及mTLS Engine客户端，严格锁定可用容器配置、runsc、非root、cap-drop、只读rootfs、默认无网络、cgroups、临时目录与输出校验。只允许专用执行节点；不给业务Host/Job引擎socket或Key。
3. 在获授权的专用节点运行真实隔离测试：宿主路径/其他租户/云元数据/未批准网络不可达；CPU/内存/60秒/128MiB产物上限及取消/重启原Job恢复；合法媒体/文本转换可归档。
4. 运行`go test -race -count=1 ./tests/skill -run 'Test(Sandbox|SandboxJob)'`；真实隔离组必须报告节点、Engine/runsc版本、镜像digest和实际结果。当前环境未提供时记录未执行，不改成本机shell。

退出：协议测试与真实隔离均满足。只有包可导入或缺沙箱时显示unavailable，不足以退出完整Skill能力。

### T-AH-08 专业Agent定义、委派与结果交接

依赖：T01～06；AC-AH-03/09/10、ST-AH-03/04/05/13。文件：`agent/domain/{definition,delegation,handoff}.go`、`application/delegation.go`、对应PG/HTTP适配、`agent-runtime/src/{agents,delegation,handoff}.ts`及平台角色配置；测试`backend/tests/agent/delegation_test.go`、`agent-runtime/tests/{delegation,agent-cancellation,agent-budget}.test.ts`。

消费：AgentDefinition pin、`agent.handoff/v1`、父批准与输入ArtifactRefs。产出：`AdmitDelegation(ctx, RuntimeIdentity, DelegationInput) (DelegationReceipt,error)`及SDK owned child conversation/结构化结果。

1. 写失败测试：不同专业Agent使用独立上下文、工具集/输入输出schema；两次相同delegation重用原child/submission、同键不同目标冲突；结果回收只发生一次。
2. 测试父子取消、失败/all-settled保留成功、深度2/并发4/根64模型与256工具的共享限制，以及旧control/Skill撤销/跨项目的收敛。
3. 用SDK ownership/task原语实现委派，不复制聊天历史为“另一个角色”；关闭默认background脱离父取消的路径。协调器只消费通过验证的结果引用，不让子输出签发业务批准。
4. 运行`go test -race -count=1 ./tests/agent -run TestDelegation`及Host三组测试；用调用记录证明至少两类专业子会话实际执行。

退出：实际多Agent可观察、可恢复和可取消；“SDK能fork”或多角色prompt不满足退出。

### T-AH-09 剧本解析Agent与正式采用

依赖：T04/06/08；AC-AH-04、ST-AH-06。文件：`script/domain/parse_proposal.go`、`script/application/{parse,parse_adopt}.go`、对应PG/HTTP与`app/agent/script_tools.go`、Host的script-parser Definition/Skill配置；测试`backend/tests/script/{parse_proposal,parse_merge,parse_adopt_integration}_test.go`、`agent-runtime/tests/script-parser.test.ts`。

消费：`source_version_id/content_hash/scalar_range`，现有导入/来源/结构用例。产出：`script.parse/v1`提案与`AdoptParse(ctx, actor, AdoptParseInput) (AdoptionReceipt,error)`，正式ID由领域服务产生。

1. 写长文本/Unicode/分集/段落分片测试，覆盖跨片场景、重复重叠台词、同名异人、说话者未知及来源span越界，确定性合并不依赖模型自评。
2. 加入schema非法、有限修复、用户修订、来源已更新、并发采用与重复采用的失败测试；提案不得直接生成视频或覆盖原剧本。
3. 接SDK解析子Agent，预算约束下分片与回收；复用script来源/分集/结构应用接口、主体别名提案和正式采用，不另造全文真相源。
4. 运行`go test -race -count=1 ./tests/script -run TestParse`及Host解析测试。受控结果验证坐标/合并，真实文本模型再验证结构质量，人工检查分别记录。

退出：可逐项定位原文、处理歧义并采用为正式结构，旧源提案不能继续采用。

### T-AH-10 分镜领域、美术/镜头Agent与局部重做

依赖：T08/09及现有subject/bible/canvas/operation；AC-AH-03/13、ST-AH-16/17。文件：新增`backend/internal/storyboard/{domain,application,adapter}/`、`db/storyboard_incremental.sql`及schema；`app/agent/{subject_tools,storyboard_tools,generation_tools}.go`；Host角色/Skill；测试`backend/tests/storyboard/{proposal,adoption,invalidation,operation_binding}_test.go`。

消费：已采用剧本/主体/场景，`storyboard/v1`；产出：正式镜头意图/版本/采用与Step依赖引用。图像/视频仍复用operation报价/确认/查询。

1. 写失败测试：镜头稳定ID与顺序、角色/场景引用版本、时长/画幅、输入缺失、引用越权、旧revision采用、重复绑定。
2. 写影响图测试：改变一个镜头只使其必要后代stale；改角色使引用镜头失效但保留成功媒体；协作者提交后Agent不能盲改revision重试。
3. 实现分镜专业子Agent与正式领域、画布投影/编辑适配、美术/镜头任务及逐报价批准；不直接写Yjs数据库或用聊天内容作镜头唯一记录。
4. 运行`go test -race -count=1 ./tests/storyboard`，以及涉及的subject/canvas/operation绑定回归；记录旧工程兼容。

退出：分镜/镜头是可编辑业务对象，真实多Agent与生成回执能贯通，成功产物可以局部复用。

### T-AH-11 声音、剪辑、质量与交付Agent

依赖：T07/10、现有语音/音乐/media/mediatool；AC-AH-14/15、ST-AH-18/19。文件：`app/agent/{voice_tools,timeline_tools,inspection_tools,export_tools}.go`、必要的消费接口、Host角色/Skill；测试`backend/tests/agent/{delivery,quality_evidence}_test.go`及受影响mediatool测试、`agent-runtime/tests/delivery.test.ts`。

消费：已采用台词/镜头/声音、媒体事实与授权参考。产出：`timeline/v1`、正式时间线revision、渲染/检查与最终ArtifactRefs。

1. 写失败测试：仅收到taskID不完成、缺失/过期产物、实际内容hash变化、未经视觉读取却声称检查、音轨/字幕时长与镜头不对应、输入变化后旧渲染不可采用。
2. 复用配音/音乐独立协议与音色许可、时间线/导出用例；Agent采用/渲染分别取得批准；质量Agent只读实际授权媒体，区分确定性/模型/人工检查。
3. 用本地合成媒体验证真实FFmpeg/ffprobe解码、时间线组合和回访；记录模型未读取像素/音频时的能力限制。
4. 运行`go test -race -count=1 ./tests/agent ./tests/mediatool -run 'Test(Delivery|QualityEvidence|AgentTimeline)'`及Host交付测试；真实供应商及人工片子审阅归T14。

退出：有可解码/可试听/可继续编辑的交付工程及来源，不以一张图或下载链接关闭完整短剧接受。

### T-AH-12 Figma、Agent工作区与Skill管理UI

依赖：公开API稳定的T02/04/05/06/08/10/11；AC-AH-18、ST-AH-24。文件：既有Figma文件中的可编辑组件/状态；新增`frontend/src/components/agent/`、`app/skills/`及其页面私有组件，更新创作/画布/分镜入口；后端注解生成到现有`frontend/src/api/`；测试`frontend/tests/unit/components/agent/`、`tests/unit/app/skills/`与对应browser场景。

消费：正式GoAPI、事件与快照、Job/Agent/Skill状态；产出：统一会话/Run、计划/批准、Agent树、工具/产物面板、Skill完整生命周期与错误恢复交互。

1. 先补Figma的桌面/手机正常、空、加载、无模型、缺沙箱、审批planning/execution、预算不足、断线、部分失败、对账、版本冲突及Skill不兼容状态；读回组件/文字与截图，不填造节点ID。
2. 写组件失败测试：明确批准前无模型调用，停止只提示Go已关闭准入、外部取消仍实际显示；断流恢复快照不创建新Run；Skill更新不改变运行pin；长内容与失焦恢复可用。
3. 实现页面和公共组件，复用shadcn/主题/同站BFF；服务端业务事件驱动状态，前端不调用advance驱动Harness。
4. 在frontend执行`pnpm api:generate`、`pnpm api:check`、`pnpm exec eslint .`、`pnpm exec next typegen`、`pnpm exec tsc --noEmit`、`pnpm exec vitest run --maxWorkers=2`、`pnpm exec next build`；按变更文件格式检查。浏览器经真实BFF/Go/Host核对320/390/1440px、键盘/焦点/缩放及错误恢复。

退出：Figma与真实交互状态一致，接口生成检查通过；受控模型浏览器检查不记为真实生成验收。

### T-AH-13 部署、迁移、单写者与回退

依赖：T01～11，公开准入前完成；AC-AH-16/17、ST-AH-22/23。文件：现有`docker-compose.yml`/Dockerfile、`agent-runtime/Dockerfile`、Go配置与角色接线、相关增量SQL/schema、PROJECT的已批准运行边界；测试`backend/tests/agent/migration_integration_test.go`、`agent-runtime/tests/{ownership,backup,upgrade}.test.ts`及沙箱健康合同。

消费：固定SDK存储版本、Go映射/命令/回执、PostgreSQL 存储/Engine配置。产出：内部Host与skill-sandbox健康、所有权与受控恢复；新engine准入flag、备份/回退手册。

1. 在隔离PG中测试空库与两次存量增量、旧会话/助手可读、原operation可查；验证无旧批准复用或历史请求重执行。
2. 写HostA持 PostgreSQL 会话锁/失租与HostB接管失败测试，证明Go旧epoch拒绝、租约过期不双开；SDK版本不兼容拒绝打开；恢复一致备份后先对Go业务账本对账。
3. 配置唯一Compose内部服务和 PostgreSQL 存储、无新增宿主端口、无业务Key进Host、专用Linux沙箱daemon；验证进程/协议/存储/出口/依赖健康分层。不能从开发机环境自动拾取凭据。
4. 演练：关闭新准入→drain→一致备份→升级→恢复原Run；回退仅关闭新准入并保留旧SDK处理原会话，不用旧引擎打开新存储。业务库迁移与服务变更另取得明确授权后才对用户环境执行。
5. 运行适用隔离测试与`docker compose config --quiet`；真实宿主/存储故障演练记录单主机和跨主机各自边界，不声称磁盘丢失自动恢复。

退出：升级、旧记录保护与备份恢复均可执行；配置可用不等于生产容量/高可用已接受。

### T-AH-14 全链路、故障矩阵与真实创作接受

依赖：T01～13；AC-AH-01～19、ST-AH-01～24。文件：`backend/tests/agent/harness_live_integration_test.go`、`agent-runtime/tests/harness-contract.test.ts`、`frontend/tests/browser/agent-harness/`；实际证据放`docs/research/evidence/2026-11-01-agent-harness/`下的版本化报告，避免复制私人剧本/Key。

1. 建立执行矩阵：每个ST指定版本、账号/权限、输入hash、环境类别、预期/实际、日志/产物引用和未完成项。覆盖命令持久化前后、Host准入、模型受理/断流、Go业务提交、SDK结果提交、子Agent结果回收与投影ACK各切点。
2. 受控模型+真实SDK+隔离PG/存储/Temporal验证计数、幂等、取消、对账、跨任务权限及重连，脚本另在真实专用沙箱验证。不得为通过而放宽费用/修订断言或隐藏失败。
3. 在用户明确批准真实模型及费用上限后，用固定小型剧本完成解析、美术、分镜、镜头、声音、时间线与成片。逐模式核对真实任务、费用/usage、归档/播放及一次局部重做；未开通模式保留未验而不是借受控成功代替。
4. 人工对脚本定位、人物/场景连续性、镜头节奏、声音/字幕和编辑工程接受；模型评分只作辅助。真实成片文件解码/试听与来源追踪必须可复查。
5. 运行PROJECT适用门禁：后端`gofmt/goimports`、`go vet ./...`、`golangci-lint run`、`go test -race -count=1 ./...`、`govulncheck ./...`；Host全量Vitest/类型/打包；前端全量测试/类型/生产构建/API检查。只在本次变化或未解决问题需要时扩大回归，不把受环境限制的检查记通过。

退出：逐项PRD接受有真实证据，明确哪些是离线/受控/真实供应商/人工接受。功能、数据迁移、质量门禁、产品接受、Git发布分别报告，不互相替代。

## 5. 任务依赖与允许并行的边界

T01→T02→T03/T04构成控制与执行基础；T05→T06→T07构成Skill能力；T08依赖基础与Skill合同，之后T09→T10→T11贯通创作。T12的Figma与状态规范可在接口收敛后开展，UI代码依赖正式合同；T13的迁移/故障设计从基础任务起同步细化，实际准入前统一演练；T14收敛整体接受。

可以并行实现不共享文件/状态的Skill包与对话网关，但工具名/schema、Go权威、根预算和Skill pin由设计32统一约束。不能将并行子任务的“通过”直接合并为整体接受，必须集成复核。不得用T01底座成功关闭后续多Agent/Skill脚本/分镜/交付任务。

## 6. 迁移与回退执行清单

1. 备份当前业务库和Host一致快照，记录精确版本、engine映射、待确认/未知/运行中任务与现有并行工作区；不输出密钥。
2. 在隔离环境演练增量、角色权限、旧新读取及重复执行；正式数据库由结构所有者在明确授权后执行，不由应用启动自动跑迁移。
3. 先启动内部服务并做无收费健康/受控验收，再开新会话小范围准入；保持旧会话engine原样。
4. 回退时先关闭新准入和新副作用，不清理会话/费用/产物；drain或暂停既有新运行，保留原SDK与回执查询。重新开放前再验证存储版本与根批准。
5. 任一环节未知，先读取原command/operation/exchange/Job状态；禁止通过删记录、换新ID或重新生成来恢复表面成功。

## 7. 2026-11-01 文档阶段历史验证

本轮只核对源码、第三方固定版本、四层来源链、需求/故事/场景/任务映射、文件链接及格式；未安装SDK、未创建运行脚手架、未运行上文未来产品测试、未更新Figma/服务/数据库、未发起真实收费或提交推送。具体文档检查回执与当前状态统一记录BACKLOG，真实实施证据在执行任务时产生。
