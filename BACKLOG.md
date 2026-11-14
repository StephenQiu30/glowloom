# 织光当前工作清单 (Glowloom Backlog)

<a id="go-microservices-20261114"></a>

## Go 微服务拆分（2026-11-14）

SRC-GO-MS-02 已确认四个业务服务＋轻量 Gateway 并授权开始执行。来源链：[需求29](workspace/content/requirement/29-Go微服务架构需求.md) → [设计44](workspace/content/design/44-Go微服务架构与技术选型设计.md) → [PRD30](workspace/content/prd/30-Go微服务拆分.md) → [Plan39](workspace/content/plan/39-Go微服务拆分执行计划.md)。Identity / Creation / Media / Execution 为确认边界；账务、模型目录与生成执行保持同域事务，不将该确认扩大为 Node 全量改写或生产发布授权。

首个工程切片已实现：独立 `cmd/gateway`、显式小配置、公共 `/api` 透明转发、独立探针、SSE/WS 与有界停机；不加载业务组合根/数据库/消息驱动。普通/开发 Compose 声明 frontend → gateway → backend，1123 仅发布 Gateway，Agent 内部调用保持原边界。普通 maintenance 已移除分区 DDL 端口与调用；旧任务仅成功收束，schema.sql 不再播种该任务，管理员显式能力保留但不接回常驻生命周期。

工程验证：变更包 vet/lint/race、SQL来源与退休种子门禁、全后端构建、依赖闭包及两种 Compose 静态拓扑通过；SSE/WS/未知回执 10 轮回归通过，真实 Gateway 二进制在无业务配置、loopback 夹具下启动/转发/SIGTERM 通过。漏洞检查在切片调用路径影响为0，所需module仍有1项未调用报告。具体命令与证据见 Plan39，不等于业务域独立或产品接受。

SRC-GO-MS-03 的先提交推送已完成：首切片 `25c2dfbff` 与退休分区种子修正 `90b9ff709` 已到 main/origin/main，保留其他并行修改。随后 T-GMS-08 已落实 API、任务、relay 的独立 Compose 命令/探针；auto 保持原 all 的队列能力选择，Codex 不自动启动。21 条 race 角色/部署断言、变更代码 vet/lint、全后端构建、普通/开发 Compose 真实解析通过；既有 tests/app 单文件 goimports 问题仍保留，详见 Plan39。SRC-GO-MS-04 要求先提交的角色隔离切片已由 `2f96b4438` 提交并推送到 main/origin/main，其他并行修改未纳入。

随后 T-GMS-09 已收回项目上传的 Creation 表责任：Media 消费方小接口由同事务 Creation owner 实现，原身份/成员/项目锁及最终授权保留，未知提交核对不要求重新取得业务授权；上传路径不再直接查询 Creation 表。边界/失败关闭/对象创建后撤权关闭及未知回执契约共 21 条 race 通过，新增无 DB 合同另重复 10 轮。首次无 DSN 时 4 条 PG 用例明确跳过，随后用两次独立临时 PG18 补跑新增 2 条，增强断言后再重复 5 轮通过；临时容器已清理，没有连接业务库。新代码仍在工作树，未自动提交推送；证据及层次见 Plan39 §4.4。

随后 T-GMS-10 将 WebM→H264 MP4 的真实规范化计算迁入独立 `cmd/media`，生产上传经私有双向 gRPC 字节流调用，无本地回退；原最终权限、审阅与资产提交保持 owner。独立镜像/配置、mTLS＋专用令牌、全程有界在途量与临时盘、健康及有界停机均单独接线，依赖闭包不含业务状态或存储驱动。按用户补充要求，协议源、Go 包、服务标识、生成物与所有消费方统一按领域命名，不保留数字版本路径或兼容双实现；对应合同见设计44 §7.7.1。

本轮验证：受影响 7 包 `go test -race` 共 1167 cases 通过；全后端 `go vet ./...`、`go build ./...`、增量 golangci-lint、协议临时重生成的两文件逐字节比较及 Workspace `check:content` 通过。Media 调用路径 govulncheck 未发现可达漏洞（required modules 另有 1 项，但不在导入/调用路径）。全量 race 未通过：`tests/agentboundary` 与 `tests/canvas` 缺显式隔离测试 DSN，未连接或复用业务库。独立镜像的 Go 构建已成功，但运行层 Debian 源连接超时导致 FFmpeg 依赖下载失败；镜像完成、容器启动与整套 Compose 均未验证。

随后 T-GMS-11 将 TXT/DOCX 文档提取迁入独立 `cmd/creation`，脚本导入 Worker 和 Agent 文档引用两个生产消费者均接私有 gRPC；唯一合同 `glowloom.creation.Extraction` 不带数字版本路径。源 20MiB、每帧 256KiB、结果 JSON 32MiB、单次 60 秒、全程有界在途量；mTLS＋专用身份、只传 W3C 追踪、完整结果及最终成功状态校验，无本地回退或自动重放。独立镜像/配置、双探针、只读根/64MiB 临时盘/1GiB 内存、两个并发位及有界停机接线完备；依赖闭包不含数据库连接、对象/消息驱动或 Media。原最终保存、权限复核、任务租约与幂等提交保持原 owner，不能视为 Creation 全域写入权已独立。

Creation 验证：受影响七包 race、全后端 vet/build、增量 golangci-lint、协议临时重生成的两文件逐字节比较、两套 Compose 实际解析通过；客户端及过载契约额外重复三轮通过。Creation 调用路径 govulncheck 无可达漏洞（required modules 另有一项未被导入/调用）。独立镜像构建成功，初次独立容器在非 root/只读根/有界临时盘下双探针 204、Docker healthy、SIGTERM 退出码 0；最终源码镜像也已重新构建成功，重建后再次烟测因清理命令的安全检查未执行，未新建容器或绕过检查。全量 race 仍仅 `tests/agentboundary`、`tests/canvas` 缺显式隔离测试 DSN 而失败；未复用业务库。未启动整套 Compose、未部署生产证书、未提交推送。

未完成项：隔离 PostgreSQL 的旧租约恢复、真实撤权/关闭锁竞争、受限角色验证、Media 镜像构建/整套 Compose/角色故障注入、真实登录和业务接受。本轮未连接业务数据库、未部署、未收费。Media 最终写入仍依赖同事务 Identity/Creation 锁，不能把端口或一次 RPC 预检当作独立授权屏障。下一步先在隔离 PG 核实上传提交与真实身份/团队/关闭操作的双向竞争，再定义持久准入/排空/未知结果协议，证明等价后才移动写入权；其余 owner/跨域引用/任务事件清点仍未完成，不把本轮责任收敛算作四域完成。

<a id="workspace-20261114"></a>

## Workspace知识库与研发工作流（2026-11-14）

来源链：[原始需求28](workspace/content/requirement/28-Workspace知识库与研发工作流需求.md) → [设计43](workspace/content/design/43-Workspace知识库与研发工作流服务设计.md) → [PRD29](workspace/content/prd/29-Workspace知识库与研发工作流.md) → [Plan38](workspace/content/plan/38-Workspace知识库与研发工作流执行计划.md)。SRC-WS-08授权本地完整实施；SRC-WS-09明确暂不启用真实TTS，不上传声音、不收费合成。SRC-WS-10随后授权仅提交本次实现和文档迁移至main并推送，保留其他并行修改；不含生产部署或业务库操作。

实施顺序为G0基线 → G1迁移／静态知识库 → G2 Skill／工作流 → G3已有音频播放 → G4本机在线协作／Codex → G6本地联合接受；G5供应商与真实朗读按用户选择延期。知识单源为workspace/content，业务工程仍拥有源码与契约，根规范和schema保留原位。研发运行采用dev_workspace和受限glowloom_workspace，避免与现有产品workspace表冲突。

本地候选已实现Markdown／GFM解析、显式锚点、目录、Pagefind中文与ID搜索、Mermaid SVG、raw、关系链、4份Skill原文／发现链接、3份流程和有限上下文、音频manifest校验／播放器／章节范围；在线版实现本机令牌、提案修订冲突与幂等落地、PG运行／事件／证据、认证Unix执行桥及原Codex句柄查询。已完成独立审查及8项重要问题修复回归；共享工作树按逐文件摘要、三方合并和精确白名单切换，保留原业务修改。当前本地验收回执在workspace/.local/evidence-20261114/，不纳入知识发布。

已验证：44项单元／集成测试含隔离PG受限角色、幂等和并发CAS；静态4项、在线3项和朗读1项真实浏览器用例；独立Nginx静态托管3项、无源码standalone登录／阅读／原文／搜索；public空集与单页白名单构建，私有页面／原文未进入公共包；本机Codex实际发现4份Skill，完成只读任务，并在关闭、重建主机进程后读取同一thread／turn的完成结果。未发起供应商试用或TTS；Codex自身正常模型推理不等于真实TTS接受。历史断链仅恢复到已核实Git版本，未归档引用明确标记，不伪造附件。

仍需区分：自动测试／静音PCM fixture、本机Codex、产品人工接受、团队SSO、生产数据库、远程部署和效率基线。团队身份、线上域名、生产受控schema变更、保留期与真实TTS供应商／音色／预算未验收；不得推定完成。详情见[使用与部署](workspace/content/templates/04-Workspace使用与部署.md)。

<a id="logs-analytics-20261113"></a>

## 日志与完整创作经营分析（2026-11-13）

用户明确要求重新分析日志需求，并确认同时建设完整创作与经营分析。来源链：[需求27](workspace/content/requirement/27-日志与创作经营分析需求.md) → [设计42](workspace/content/design/42-日志与创作经营分析设计.md) → [PRD28](workspace/content/prd/28-日志与创作经营分析.md) → [Plan37](workspace/content/plan/37-日志与创作经营分析执行计划.md)。SRC-LOG-04已授权先提交main并推送，再落实页面；文档提交`bcb099e2`已推送。

页面已接入运行诊断、平台审计、创作快照与经营缺项状态，复用现有外壳、Geist/Neutral和shadcn。诊断仅当前后端进程4096条/24小时内存样本，重启丢失；创作区分受理任务、执行尝试和已登记产物，个人只看本人确认且当前可读的事实，平台汇总重查权限。经营金额、采用/留存、独立团队汇总及导出依赖未接入，保持明确不可用，不用Figma样例或零值替代。当前合同与来源见设计42 §7.9。

设计交付：[30个原生Figma画板](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3980-123290)，含13桌面、12手机、个人权限、平板/窄短屏、12态矩阵和交付说明，核心跨画板连线156条。当前实现通过真实OTel追踪、隔离PostgreSQL授权/计数测试、专项race/vet/lint与生成合同检查；独立复核已修复trace丢失、生成类型遮蔽和页签往返筛选状态问题。最终隔离候选通过329文件/2345前端测试、生产构建/TypeScript与standalone检查；24个深浅色/尺寸组合无正文横溢，详情回焦、分页快照和44px触控核对通过。详细分层证据见Plan37 §4.2，fixture不代替生产接受。

完整范围继续保留：T-LOG-02～03跨服务/持久诊断、资源/积压与读取审计；T-LOG-04采用/交付、访问/活跃/成熟同期群及可重放事实；T-LOG-05独立财务授权、支付/会员、收入分摊和真实成本；T-LOG-06受控导出；T-LOG-07生产容量与联合接受。DEC-LOG-01～04的量级/期限、观察口径、经营授权和回补保留仍待核定。注册前短期匿名会话仅为默认关闭提案，未启动采集；未执行生产迁移、部署或真实付费调用，不能签收完整需求已完成。

<a id="pool-sale-plans"></a>

## 模型号池与销售计费

后台可发布备用线路、调整 Key 准入、共享并发额度、倍率与折扣。隔离 PostgreSQL 已验证并发争抢、派发前换路、连接未建立时的一次备线尝试及采购价归属。浏览器通过实际接口验证线路发布与 91/114 积分试算。DeepSeek Flash 实单已完成。火山方舟实单、同规格双渠道及采购凭证见[验收记录](workspace/content/research/2026-11-13-模型号池与销售计费验收.md)。后台可查看实际尝试、连接及 Key 尾号。连接前失败使用 15 秒冷却。仅销售价格变化时可保留旧报价，已核验冻结线路版本。跨供应商同规格实单及供应商账单验收待完成。[Plan35](workspace/content/plan/35-模型号池接入与路由执行计划.md) · [Plan36](workspace/content/plan/36-模型销售定价与结算执行计划.md)

<a id="agent-shot-first-wave-20261112"></a>

## Agent与单镜首轮闭环实施（2026-11-12）

当前授权为SRC-AGH-20/21：补齐详细实施计划并实际打通原请求/费用、失败恢复和单镜制作；保留原有能力。用户随后明确授权将本轮代码提交main并推送；部署、真实付费及存量库自动DDL不在本次范围。实施入口：[Plan28 §9](workspace/content/plan/28-完整创作Agent与Skill多Agent实施计划.md#9-首轮可执行实施src-agh-20)、[Plan33首轮](workspace/content/plan/33-短剧工作室实施计划.md#首轮单镜可执行实施src-agh-20)。来源/设计/接受合同分别补入需求19 SRC-AGH-20/21、设计32 §7.16与设计38 §7.10、PRD19/24；后面的仅文档记录保留为历史。

### 本轮结果与真实行为

- 已完成原完整CallIdentity的HTTP查询、Node生成客户端与未知回执恢复。实际超额费用保留且一次结算，后续模型（含零上限）不再准入，免费查询/采用/提案继续可用。
- 已修复原operation.Source投影，以及项目级会话的来源版本围栏；正式报价在原事务内绑定存储目标，未用测试侧补写掩盖生产装配缺口。
- 已接shot_delivery_propose/create/get：冻结资产ID/修订/SHA与原来源、裁剪和时间线规格；明确批准后，Yjs采用/单镜时间线、现有视频和工程任务、原业务回执整体提交。重复调用不增供应商提交、费用或导出任务。
- 已接持久waiting_delivery：分开模型轮次与作品完成；保存绝对截止和到期唤醒，重启只查询原任务；授权故障继续有界等待，暂停恢复不多调用模型。批准未创建、未审阅、失败、产物删除/不可读均不能报告成片完成。
- 节点类型/动作/引用、时间线时长边界及交付状态使用所属领域常量；画布文本使用Node.TextInput和现有Prompt冻结语义。审批卡片复用正式媒体预览，显示确切所选视频、裁剪/时长、画幅、FPS与工程格式；无法读取的合同不能批准。

### 验证证据与边界

- CI接入：新增GLOWLOOM_TEST_AGENT_SHOT_DB_DSN及GLOWLOOM_TEST_SHOT_PIPELINE_DB_DSN，对应独立初始化数据库，现有backend Race步骤执行两个新增闭环用例，不以缺少专用环境跳过。
- Agent：隔离PG与受限glowloom_agent角色，完整29文件/163测试通过，包括实际SIGKILL、原回执、授权故障、到期、暂停恢复和到期扫描。typecheck、生产build、lint、format及Swagger/生成客户端比对、严格API类型通过。
- Go：隔离PG的agentbusiness完整47测试通过、无跳过；受控COS真实原文发布也运行。原始未知HTTP回执回收、真实费用超额/免费恢复、审批义务、资产修订/引用/FPS、原子采用与幂等均有回归。
- 联合单镜：TestAgentShotOriginalQuoteRecoveryVideoAndProjectDelivery通过；真实报价/批准/确认→受控TLS供应商一次POST→实际MP4归档/探测→原资金一次结算→真实Yjs采用→FFmpeg成片与FCPXML归档。下载后解码视频，按原FrozenExport检查工程与媒体SHA/引用；故意丢失真实对象写入确认后重建worker恢复原产物；审阅前及删除已审阅输出后不报完成。
- Canvas：补齐既有Outbox测试夹具的当前claim/ack合同后，领域命令与原生Yjs保存/恢复/Outbox用例通过。Go相关包vet及golangci-lint（含canvas/operation/agentbusiness测试）通过；并非整个后端/全部Canvas集成套件接受。
- 界面：会话/结果/状态3文件13测试通过，完整前端typecheck与变更文件lint通过；typegen仅使用离线loopback origin，未发起生成。审批卡片未做登录后的浏览器实页接受。
- 主工作区回填后：新费用/原HTTP/采用边界race、真实单镜联合测试与会话13项回归再次通过；本轮代码逐字核对，其他并行更改没有覆盖。隔离工作树的整套测试不扩展为其他并行工作的整体接受。
- 失败照实记录：最初Agent整套出现7个5秒超时与关联Harness关闭错误；相关4文件22项独立复跑通过，随后完整163项通过。Canvas首次缺DSN及旧Outbox夹具不编译均记录并处理后才跑对应门；首次Go全套的一个COS条件跳过在补齐受控配置后完整47项无跳过。旧控制传输测试隔离自身队列，未排空其他测试root来掩盖问题。
- 安全门未通过：当前Go1.26.10的govulncheck命中标准库GO-2026-6859，扫描建议1.26.11；本轮未升级工具链/发布镜像，不能作为生产放行。没有真实付费供应商、目标剪辑软件导入、容量或灾备接受；新增集成测试的两套专用CI数据库已接入；CI结果按本次推送的GitHub运行单独核验。

### 后续实施顺序

1. 先处理工具链安全门、制作实页与获授权的真实供应商/目标软件接受，再扩大生产放行范围。
2. 沿设计38既有shot/generation-unit/take与依赖失效合同扩展两镜和整集，复用本次费用/回执/交付围栏。
3. 再接受真实子Agent并行、Skill固定版本汇合、MCP认证与未知写入恢复；以现有原请求/额度/阶段产物约束，不新增空引擎。

清理：本轮六个明确命名的隔离测试库在确认均为stephenqiu所有且连接数为零后删除；COS临时进程及本轮detached工作树已移除，其他工作树未清理。补丁、审查和测试日志保留在/tmp/glowloom-agent-shot-review及/tmp/glowloom-agent-shot-*.log。并行工作将main更新为8a2f061f，未包含本轮代码/文档路径；本轮保留该提交。

Git：基线74837bb5，首轮在detached临时工作树验证后按允许清单回填。按本轮最新授权在main提交并推送，不新建分支、不部署；仅暂存本轮允许清单，保留其他并行状态。仅新建的隔离测试数据库执行schema/data初始化，不涉及存量业务库。

<details><summary>本轮首轮实现清单（66个代码/测试文件、7个合同文档与本BACKLOG）</summary>

- [BACKLOG.md](BACKLOG.md)
- [agent/src/business/client.ts](agent/src/business/client.ts)
- [agent/src/business/generated/typings.d.ts](agent/src/business/generated/typings.d.ts)
- [agent/src/config.ts](agent/src/config.ts)
- [agent/src/pi-durable/definitions.ts](agent/src/pi-durable/definitions.ts)
- [agent/src/pi-durable/docs.ts](agent/src/pi-durable/docs.ts)
- [agent/src/pi-durable/events.ts](agent/src/pi-durable/events.ts)
- [agent/src/server.ts](agent/src/server.ts)
- [agent/src/sessions/delivery.ts](agent/src/sessions/delivery.ts)
- [agent/src/sessions/runtime.ts](agent/src/sessions/runtime.ts)
- [agent/src/sessions/service.ts](agent/src/sessions/service.ts)
- [agent/tests/business-client.test.ts](agent/tests/business-client.test.ts)
- [agent/tests/delivery-contract.test.ts](agent/tests/delivery-contract.test.ts)
- [agent/tests/lifecycle-recovery.test.ts](agent/tests/lifecycle-recovery.test.ts)
- [backend/internal/agent/adapter/http/business.go](backend/internal/agent/adapter/http/business.go)
- [backend/internal/agent/adapter/http/business_swagger.go](backend/internal/agent/adapter/http/business_swagger.go)
- [backend/internal/agent/adapter/postgres/authority.go](backend/internal/agent/adapter/postgres/authority.go)
- [backend/internal/agent/adapter/postgres/calls.go](backend/internal/agent/adapter/postgres/calls.go)
- [backend/internal/agent/application/authority.go](backend/internal/agent/application/authority.go)
- [backend/internal/agent/domain/delivery.go](backend/internal/agent/domain/delivery.go)
- [backend/internal/app/agent/canvas_commands.go](backend/internal/app/agent/canvas_commands.go)
- [backend/internal/app/agent/creation.go](backend/internal/app/agent/creation.go)
- [backend/internal/app/agent/errors.go](backend/internal/app/agent/errors.go)
- [backend/internal/app/agent/shot_delivery.go](backend/internal/app/agent/shot_delivery.go)
- [backend/internal/app/agent/shot_delivery_query.go](backend/internal/app/agent/shot_delivery_query.go)
- [backend/internal/app/agent/tools.go](backend/internal/app/agent/tools.go)
- [backend/internal/app/agent/wiring.go](backend/internal/app/agent/wiring.go)
- [backend/internal/canvas/adapter/postgres/project_copy.go](backend/internal/canvas/adapter/postgres/project_copy.go)
- [backend/internal/canvas/adapter/postgres/store.go](backend/internal/canvas/adapter/postgres/store.go)
- [backend/internal/canvas/adapter/postgres/subject_input.go](backend/internal/canvas/adapter/postgres/subject_input.go)
- [backend/internal/canvas/adapter/postgres/timeline.go](backend/internal/canvas/adapter/postgres/timeline.go)
- [backend/internal/canvas/adapter/postgres/transcription.go](backend/internal/canvas/adapter/postgres/transcription.go)
- [backend/internal/canvas/adapter/postgres/yjs_history.go](backend/internal/canvas/adapter/postgres/yjs_history.go)
- [backend/internal/canvas/adapter/postgres/yjs_projection_store.go](backend/internal/canvas/adapter/postgres/yjs_projection_store.go)
- [backend/internal/canvas/application/dto.go](backend/internal/canvas/application/dto.go)
- [backend/internal/canvas/application/yjs_projection.go](backend/internal/canvas/application/yjs_projection.go)
- [backend/internal/canvas/domain/asset_templates.go](backend/internal/canvas/domain/asset_templates.go)
- [backend/internal/canvas/domain/batch_table.go](backend/internal/canvas/domain/batch_table.go)
- [backend/internal/canvas/domain/cinema_kit.go](backend/internal/canvas/domain/cinema_kit.go)
- [backend/internal/canvas/domain/director.go](backend/internal/canvas/domain/director.go)
- [backend/internal/canvas/domain/document.go](backend/internal/canvas/domain/document.go)
- [backend/internal/canvas/domain/draft.go](backend/internal/canvas/domain/draft.go)
- [backend/internal/canvas/domain/editor_config.go](backend/internal/canvas/domain/editor_config.go)
- [backend/internal/canvas/domain/graph.go](backend/internal/canvas/domain/graph.go)
- [backend/internal/canvas/domain/node_types.go](backend/internal/canvas/domain/node_types.go)
- [backend/internal/canvas/domain/operations.go](backend/internal/canvas/domain/operations.go)
- [backend/internal/canvas/domain/project_copy.go](backend/internal/canvas/domain/project_copy.go)
- [backend/internal/canvas/domain/subject.go](backend/internal/canvas/domain/subject.go)
- [backend/internal/canvas/domain/subject_connections.go](backend/internal/canvas/domain/subject_connections.go)
- [backend/internal/canvas/domain/text_input.go](backend/internal/canvas/domain/text_input.go)
- [backend/internal/canvas/domain/timeline.go](backend/internal/canvas/domain/timeline.go)
- [backend/internal/operation/adapter/http/generation_estimate.go](backend/internal/operation/adapter/http/generation_estimate.go)
- [backend/internal/operation/adapter/postgres/public_query.go](backend/internal/operation/adapter/postgres/public_query.go)
- [backend/internal/operation/adapter/postgres/quote_canvas_source.go](backend/internal/operation/adapter/postgres/quote_canvas_source.go)
- [backend/internal/operation/adapter/postgres/store.go](backend/internal/operation/adapter/postgres/store.go)
- [backend/internal/operation/adapter/staging/routed.go](backend/internal/operation/adapter/staging/routed.go)
- [backend/internal/operation/domain/output.go](backend/internal/operation/domain/output.go)
- [backend/tests/agentbusiness/http_test.go](backend/tests/agentbusiness/http_test.go)
- [backend/tests/agentbusiness/request_cost_recovery_test.go](backend/tests/agentbusiness/request_cost_recovery_test.go)
- [backend/tests/agentbusiness/shot_delivery_integration_test.go](backend/tests/agentbusiness/shot_delivery_integration_test.go)
- [backend/tests/agentbusiness/yjs_commands_test.go](backend/tests/agentbusiness/yjs_commands_test.go)
- [backend/tests/canvas/yjs_outbox_test.go](backend/tests/canvas/yjs_outbox_test.go)
- [backend/tests/canvasfixture/command_runtime.go](backend/tests/canvasfixture/command_runtime.go)
- [backend/tests/operation/agent_shot_pipeline_integration_test.go](backend/tests/operation/agent_shot_pipeline_integration_test.go)
- [workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)
- [workspace/content/design/38-短剧工作室一致性设计.md](workspace/content/design/38-短剧工作室一致性设计.md)
- [workspace/content/plan/28-完整创作Agent与Skill多Agent实施计划.md](workspace/content/plan/28-完整创作Agent与Skill多Agent实施计划.md)
- [workspace/content/plan/33-短剧工作室实施计划.md](workspace/content/plan/33-短剧工作室实施计划.md)
- [workspace/content/prd/19-完整创作Agent与Skill多Agent.md](workspace/content/prd/19-完整创作Agent与Skill多Agent.md)
- [workspace/content/prd/24-短剧工作室.md](workspace/content/prd/24-短剧工作室.md)
- [workspace/content/requirement/19-完整创作Agent与Skill多Agent需求.md](workspace/content/requirement/19-完整创作Agent与Skill多Agent需求.md)
- [frontend/src/components/conversations/conversation-state.ts](frontend/src/components/conversations/conversation-state.ts)
- [frontend/src/components/conversations/conversation-thread.tsx](frontend/src/components/conversations/conversation-thread.tsx)
- [frontend/tests/unit/components/conversations/conversation-thread.test.tsx](frontend/tests/unit/components/conversations/conversation-thread.test.tsx)

</details>

<details><summary>当前其他并行未提交路径与既有缓存（本轮未覆盖）</summary>

- `DESIGN.md`
- `backend/internal/subject/adapter/http/query.go`
- `backend/internal/subject/adapter/postgres/query.go`
- `backend/internal/subject/application/contracts.go`
- `backend/internal/subject/application/service.go`
- `backend/internal/subject/domain/content.go`
- `backend/tests/subject/type_test.go`
- `workspace/content/00-文档索引.md`
- `workspace/content/requirement/25-多模态创作与Agent编排需求.md`
- `workspace/content/research/00-调研导航.md`
- `workspace/content/research/2026-11-12-即梦多模态创作与Agent服务拆解.md`
- `workspace/content/research/2026-11-12-短剧制作平台源码与发行包对照调研.md`
- `frontend/.vitest/ (existing cache)`
- `frontend/Dockerfile`
- `frontend/next.config.ts`
- `frontend/public/design/reference/studios/empty.png`
- `frontend/public/design/reference/studios/marketing-center.png`
- `frontend/public/design/reference/studios/marketing-left.png`
- `frontend/public/design/reference/studios/marketing-right.png`
- `frontend/public/design/reference/studios/motion-card.png`
- `frontend/public/design/reference/studios/motion-paper.png`
- `frontend/public/design/reference/studios/motion-print.png`
- `frontend/src/api/typings.d.ts`
- `frontend/src/app/api/[...path]/route.ts`
- `frontend/src/app/assets/_components/asset-location.ts`
- `frontend/src/app/assets/_components/asset-query.ts`
- `frontend/src/app/assets/_components/asset-workspace.tsx`
- `frontend/src/app/assets/page.tsx`
- `frontend/src/app/globals.css`
- `frontend/src/app/marketing-studio/page.tsx`
- `frontend/src/app/motion-studio/page.tsx`
- `frontend/src/app/subjects/page.tsx`
- `frontend/src/components/billing/credit-popover.tsx`
- `frontend/src/components/layout/application-layout.tsx`
- `frontend/src/components/layout/basic-layout.tsx`
- `frontend/src/components/layout/page-layout.tsx`
- `frontend/src/components/layout/product-account-actions.tsx`
- `frontend/src/components/layout/product-header-actions.tsx`
- `frontend/src/components/layout/product-header.tsx`
- `frontend/src/components/layout/product-navigation.tsx`
- `frontend/src/components/layout/product-sider.tsx`
- `frontend/src/components/layout/routes.ts`
- `frontend/src/components/layout/sidebar-nav-link.tsx`
- `frontend/src/components/studios/studio-page.tsx`
- `frontend/src/components/studios/studios.css`
- `frontend/src/components/subjects/unified-contracts.ts`
- `frontend/src/components/subjects/unified-editor.tsx`
- `frontend/src/components/subjects/unified-library.tsx`
- `frontend/src/components/subjects/unified-toolbar.tsx`
- `frontend/src/components/subjects/use-subject-list.ts`
- `frontend/tests/browser/assets.tsx`
- `frontend/tests/unit/app/api/[...path]/route.test.ts`
- `frontend/tests/unit/app/assets/_components/asset-location.test.ts`
- `frontend/tests/unit/app/assets/_components/asset-workspace.test.tsx`
- `frontend/tests/unit/components/layout/basic-layout.test.tsx`
- `frontend/tests/unit/components/subjects/unified-editor.test.tsx`
- `frontend/tests/unit/components/subjects/use-subject-list.test.ts`

</details>

<a id="agent-studio-reliability-20261112"></a>

## Agent/短剧稳定服务再评估（2026-11-12）

用户再次要求评估并详细设计稳定服务，来源SRC-AGH-19/SRC-STUDIO-21，继续只做文档。对当前runtime/ownership/registry/delegation/reconciliation、恢复扫描及Go taskruntime复核后，保留已有执行基础，新增[设计32 §7.15](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md#715-稳定服务再评估与详细执行合同)和[设计38 §7.9](workspace/content/design/38-短剧工作室一致性设计.md#79-可靠生产与版本化交付的收紧)。这两节收紧此前整体方案，对应AGH-046/FR-STUDIO-13、PRD19 AC-AH-40～44/ST-AH-58～65及PRD24 AC-STUDIO-14/15/ST-STUDIO-20～23，Plan28 EX-11/Plan33 DR-08定义联合门禁。

关键决定：简单任务主Agent直接执行，专业子会话仅用于独立工作，确定性交付不再每步问LLM；区分模型轮次/目标控制/作品交付；根步数/货币资金/容量槽分开；发送前CAS持久“可能已发送”，未知只查原请求；失owner迟到回执只能成为受限原请求证据；等待释放短槽但保留上游在途/未决资金；真实账单超估仍记录，自动硬预算必须有可信费用上界；停机排空后再释放owner/关池；灾后先隔离旧实例和旧授权、重建执行代际并核对账本/上游再开闸。

文档终检：本轮新增/修改文档的相对链接目标已校验存在，本机绝对路径已改为代码样式；AGH-001～046、AC-AH-01～44/ST-AH-01～65及FR-STUDIO-01～13、AC-STUDIO-01～15/ST-STUDIO-01～23连续无重复，需求追踪完整；两份设计仍为八主节。`git diff --check` 只覆盖已跟踪改动，新增文件需单独检查空白。独立复核确认上述四个关键窗口及停机顺序的修订闭合；这不代表故障/容量/生产接受。本轮修改仍留在工作区，已有未提交文件逐项沿下方清单说明，未清理frontend/.vitest/。

### 修订后的实施顺序

此表作为本次再评估的当前建议，细化并优先于下方旧顺序，仍未授权开工。

| 顺序                      | 必须先完成的合同/验证                                                                    | 对应切片/放行条件                                                                    |
| ------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| 1 原意图/费用/完成语义    | 固定call/attempt及发送资格CAS、原回执查询/未知分类、真实账单与预算上界、轮次与交付状态   | EX-11前半及已有owner/grant门；无此证据不开放自动付费                                 |
| 2 单镜确定性闭环          | 来源/候选/采用→原任务生成/归档→take→时间线→版本化成片/工程，含人工修改失效围栏和局部重做 | DR-01/02/05最小范围、DR-08、EX-08/09最小范围；不是先全剧跑通再补版本                 |
| 3 持久等待与资源保护      | 原句柄/截止/补扫/停放、预算/容量不双计、等待释放短槽、公平扫描和控制预留                 | EX-05/07/10/11；先受控长等待/故障实验后提高并发                                      |
| 4 实际子并行及Skill/MCP   | EX-01/02、EX-03/04可分别准备；先站内两独立镜头，再接受权外部读/写与未知处理              | 真子会话并行证据、固定方法/权限、实际MCP协议互通；缺项明确未接受                     |
| 5 集级生产与局部传播      | DR-03/04/06串真实依赖/声音/字幕/cue/clip影响，恢复不重复制作成功项                       | 两镜→整集，部分成功/未知/费用/人工修改保护通过                                       |
| 6 完整交付及容量/灾后接受 | DR-05/08全范围、EX-11后半及DR-07；实际媒体/工程/故障/恢复演练后获准真实模型与作品接受    | 解码/目标软件导入、资金、旧快照/代际、运行手册及实测容量；未验不得宣称稳定生产已接受 |

这些是一个闭环的逐步扩展，不是拆六个新服务；Task1～14只核对实际缺口，不重建已存在Storage/worker/渲染/计费。显式定时规则、任意脚本和按角色选模型维持独立范围，不作为最小制作前置；完整多Agent/MCP目标仍需单独接受，不能以单助手演示替代。

使用此前已读brainstorming技能的独立builder审阅，发现并修正发送资格窗口、迟到回执、真实费用上界、灾后代际及停机顺序。上述为文档审阅，不是工程测试。本轮未运行应用/容量/故障/真实供应商验证，不编写代码、部署、提交推送；未决RPO/RTO/并发与资源阈值需后续实测。工作区已有文档及缓存保留，逐项清单沿下方记录；本轮新增仍仅上述原主链文档和索引/BACKLOG。

<a id="agent-studio-integrated-design-20261112"></a>

## Agent与短剧完整方案评审（2026-11-12）

用户明确要求将调研整合成完整设计并明确实施顺序，先不编写代码。已保留来源SRC-AGH-18/SRC-STUDIO-20；统一阅读入口为[设计32 §7.0](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md#70-整体方案与完整调用路径)，逐阶段制作/局部重做为[设计38 §7.7～7.8](workspace/content/design/38-短剧工作室一致性设计.md#77-阶段产物与贯穿制作流程)。本轮整合既有权威，未新建第三套架构。PRD19增加AC-AH-38/39和ST-AH-55～57，PRD24增加AC-STUDIO-13和ST-STUDIO-17～19，两个Plan分别补联合门禁。均为待评审方案，不是实现或接受记录。

设计补漏：Go预算和Agent分支登记独立提交/原意图核对；分支是根内额度分配而非双扣；人工修订在Go准入侧建立失效围栏，通知未达也不准旧计划新收费；迟到素材保留旧版本候选；Stop/完成root后新修改新受理run；跨镜生成单元的重做范围、字幕/take/声音/剪辑不同传播及人工剪辑保护。采用默认人工、自动采用/额外费用仅事前授权，实际并发/超时/质量阈值待验证；沿NestJS/Pi/PostgreSQL/Go/Compose，不重新引入Temporal/SQLite。

交付终检时并行工作已新增提交`74837bb5`（画布拖动/双击及对应文档/测试），HEAD由695c779d前进；这不是本轮提交。下方旧逐项清单保留当时工作区快照，所列画布文档/源码/测试现已不在未提交清单。本轮剩余为12个已跟踪文档、3个此前新增未跟踪需求/研究文档及既有frontend/.vitest/目录，均未由本轮提交或清理。独立复核已确认新增修订/费用/终态root合同闭合，结论仅为文档一致性。

### 建议实施顺序与放行门

以下为评审建议，不表示已获代码实施授权。已有Task1～14只补实际缺口，不重复重建当前能力；每一门有新失败用例和退出证据，不能用历史测试直接通过。

| 顺序                 | 范围/可并行关系                                                                                 | 放行门                                                                                |
| -------------------- | ----------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| 0 固定基线与合同     | 复核现有服务/包/结构；对齐DR-01、EX-08的镜头/生成单元/take/阶段候选和现有领域；列能力与数据权威 | 身份/版本/来源与正式工具合同可审阅；无自动DDL、无重复batch/渲染/资金体系              |
| 1 执行与副作用基础   | 复核owner/epoch/grant、单root、原调用回执、根预算/分支准入；EX-07故障门禁从这里开始贯穿         | 独立提交未知收敛、不双扣、Stop/撤权阻断；ST-AH-55及既有恢复测试                       |
| 2 最小生产闭环       | DR-01/02、EX-08先串候选→采用→单镜正式生成→take回绑定→重开读取；同时建立DR-06最小影响/Go失效围栏 | 相同文本/重排/刷新不串镜；候选中断不清正式数据；人工修订先提交时旧计划新收费拒绝      |
| 3 主子与Skill        | EX-01/02在已固定合同上接真实并行、版本加载、结果汇合；不先做全量角色或市场                      | 两个官方子会话实际同时在途；根预算与输入范围隔离，工具/Skill不能扩权                  |
| 4 外部MCP与持久等待  | EX-03/04可与第3项并行；EX-05先以站内任务完成基础等待，再验证外部原句柄/未知写入恢复             | 真实受控MCP协议服务互通、认证隔离、有限等待/安全重试，不能查询的未知写入待核对        |
| 5 批量与声音/连续性  | EX-10+DR-03接就绪依赖/批次终态后续，DR-04可准备声音/字幕；DR-06扩展生成单元/take/cue影响        | 页面断开/服务重启能自动推进；部分失败不全重付；ST-STUDIO-07～11/18/19                 |
| 6 时间线与确定性交付 | DR-05+EX-09、DR-06补clip/交付传播，复用现有真实FFmpeg和工程归档                                 | 人工编辑CAS保护、缺项标记、逐阶段恢复；渲染完成后只补绑定/工程导出；真实媒体解码/探测 |
| 7 联合接受           | 实际Pi/隔离数据库/受控供应商完整一集→获准真实供应商与DR-07目标编辑器/作品接受                   | ST-AH-56/57、ST-STUDIO-16～19及交付清单；费用、音画、字幕、连续性与工程导入分别留证   |

事件/定时规则EX-06、按角色选LLM、额外音频理解和第三方脚本设施另行确定范围；它们不阻挡站内任务内自动推进，缺少真实隔离设施时脚本明确不可用。第4项外部MCP是既定补齐范围，但站内最小闭环不必等全部连接上线；完整Agent方案的MCP接受仍不能省略。

使用brainstorming技能做完整方案检查及独立builder审阅，已处理Go修订围栏、终态root和预算语义三项意见。只修改需求19/23、设计32/38、PRD19/24、Plan28/33、本BACKLOG及文档索引的本轮段落；此前本目标和并行文件遗留逐项见下方清单。本轮文档校验：117个本地链接/锚点有效，AGH-001～045、AC-AH-01～39/ST-AH-01～57及FR-STUDIO-01～12、AC-STUDIO-01～13/ST-STUDIO-01～19连续无重复，设计32/38均保留八个主节，需求→PRD追踪及git diff --check通过。本轮不运行应用测试、不收费/部署/提交推送，全部新增接受条件仍未做工程验证。

<a id="shortdrama-research-20261112"></a>

## 短剧制作能力与Agent执行补齐（2026-11-12）

按用户本轮“先不要编写代码”要求，已用GitHub/Firecrawl/Context7调查BeefTV、Toonflow、waoowaoo、火宝的固定源码与官方制作材料，并按reverse-engineer-anything对BeefTV v2.2.1发行ZIP及交付宿主JS静态分析；[完整报告](workspace/content/research/2026-11-12-短剧制作平台源码与发行包对照调研.md)保留SHA/许可、实际调用链、生产流程、REA Evidence ID及未知边界。5个选定宿主JS与固定main字节一致，不推定整个发行包等同main；没有安装/运行参考产品、付费调用、代码或部署变更。

补齐评审主链：需求23 SRC-STUDIO-19新增FR-STUDIO-01～12 → 设计38 §7.3～7.6 → PRD24新增12项AC/16项ST → Plan33 DR-01～07；Agent共性需求19新增SRC-AGH-17/AGH-043～045 → 设计32 §7.14 → PRD19新增AC-AH-35～37/ST-AH-51～54 → Plan28 EX-08～10。均为新增设计/接受合同，不是实现或通过记录。已有手动文字数字人链、Bible造型/声音、可选ASR、batch和真实FFmpeg/工程输出已纠偏，避免重复建设。

建议先评审生产镜头与生成单元/take/采用的身份关系、候选采用与收费授权范围，再打通已有batch的依赖和持久后续，最后接既有时间线/渲染/工程导出做确定性交付。关键缺口是当前片段仅传文字进入视频、结果仅采用资产ID，不能追溯逐镜生产；下一步应在DR-01/EX-08中固定最小业务合同和故障接受，不直接实施整套新引擎。主子并行/MCP/Skill的既有补齐仍沿前项EX-01～07。

校验范围：文档链接/锚点、稳定ID与需求→设计→PRD→Plan追踪、八节设计及diff；交叉复核BeefTV/Toonflow/waoowaoo机制和Glowloom现有能力。本轮没有运行应用测试或建立真实供应商、目标编辑器及完整成片接受。REA目标已关闭，发行包/证据仅在任务临时目录；未提交推送。并行画布文档、前端修改及frontend/.vitest/原样保留，具体工作区以最终git status为准。

<details>
<summary>本轮交付与遗留修改逐项说明（未提交）</summary>

| 文件                                                                                                                                                                         | 归属/处理                                             |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| [BACKLOG.md](BACKLOG.md)                                                                                                                                                     | 本轮及此前同目标的状态/行动，保留原有内容；保留工作区 |
| [workspace/content/00-文档索引.md](workspace/content/00-文档索引.md)                                                                                                                                   | 更新两条主链及研究入口；保留工作区                    |
| [workspace/content/requirement/00-需求导航.md](workspace/content/requirement/00-需求导航.md)                                                                                                           | 更新来源入口；保留工作区                              |
| [workspace/content/research/00-调研导航.md](workspace/content/research/00-调研导航.md)                                                                                                                 | 更新研究入口；保留工作区                              |
| [workspace/content/requirement/19-完整创作Agent与Skill多Agent需求.md](workspace/content/requirement/19-完整创作Agent与Skill多Agent需求.md)                                                             | 主子/Skill/MCP及阶段产物/交付/后续需求；保留工作区    |
| [workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)                                                 | Agent执行体系评审稿；保留工作区                       |
| [workspace/content/prd/19-完整创作Agent与Skill多Agent.md](workspace/content/prd/19-完整创作Agent与Skill多Agent.md)                                                                                     | Agent接受与故障场景；保留工作区                       |
| [workspace/content/plan/28-完整创作Agent与Skill多Agent实施计划.md](workspace/content/plan/28-完整创作Agent与Skill多Agent实施计划.md)                                                                   | Agent未来切片，本轮不执行；保留工作区                 |
| [workspace/content/requirement/23-短剧工作室对齐即梦需求.md](workspace/content/requirement/23-短剧工作室对齐即梦需求.md)                                                                               | 短剧制作补齐来源及12项功能要求；保留工作区            |
| [workspace/content/design/38-短剧工作室一致性设计.md](workspace/content/design/38-短剧工作室一致性设计.md)                                                                                             | 生产身份、依赖及交付评审稿；保留工作区                |
| [workspace/content/prd/24-短剧工作室.md](workspace/content/prd/24-短剧工作室.md)                                                                                                                       | 12项制作接受与16项场景；保留工作区                    |
| [workspace/content/plan/33-短剧工作室实施计划.md](workspace/content/plan/33-短剧工作室实施计划.md)                                                                                                     | 7个未来制作切片，保留历史记录；保留工作区             |
| [workspace/content/requirement/25-多模态创作与Agent编排需求.md](workspace/content/requirement/25-多模态创作与Agent编排需求.md)                                                                         | 此前同目标新增，尚未跟踪；保留工作区                  |
| [workspace/content/research/2026-11-12-即梦多模态创作与Agent服务拆解.md](workspace/content/research/2026-11-12-即梦多模态创作与Agent服务拆解.md)                                                       | 此前同目标新增，并校正数字人已有手动链；保留工作区    |
| [workspace/content/research/2026-11-12-短剧制作平台源码与发行包对照调研.md](workspace/content/research/2026-11-12-短剧制作平台源码与发行包对照调研.md)                                                 | 本轮新增完整调研；保留工作区                          |
| [workspace/content/requirement/14-无限画布对齐即梦需求.md](workspace/content/requirement/14-无限画布对齐即梦需求.md)                                                                                   | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| [workspace/content/design/27-无限画布一致性改造设计.md](workspace/content/design/27-无限画布一致性改造设计.md)                                                                                         | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| [workspace/content/prd/14-无限画布一致性改造.md](workspace/content/prd/14-无限画布一致性改造.md)                                                                                                       | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| [workspace/content/plan/23-无限画布一致性改造实施计划.md](workspace/content/plan/23-无限画布一致性改造实施计划.md)                                                                                     | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| [frontend/src/components/canvas/director/canvas-director-workspace.tsx](frontend/src/components/canvas/director/canvas-director-workspace.tsx)                               | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| [frontend/src/components/canvas/editor/canvas-editor.tsx](frontend/src/components/canvas/editor/canvas-editor.tsx)                                                           | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| [frontend/src/components/canvas/editor/infinite-canvas.tsx](frontend/src/components/canvas/editor/infinite-canvas.tsx)                                                       | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| [frontend/tests/unit/components/canvas/director/director-workspace-placement.test.tsx](frontend/tests/unit/components/canvas/director/director-workspace-placement.test.tsx) | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| [frontend/tests/unit/components/canvas/node-double-click.test.tsx](frontend/tests/unit/components/canvas/node-double-click.test.tsx)                                         | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |
| frontend/.vitest/（历史引用未归档）                                                                                                                                       | 并行已有修改/未跟踪项；本轮未编辑、清理或纳入验证     |

</details>

<a id="multimodal-agent-20261112"></a>

## 多模态创作与 Agent 编排需求（2026-11-12）

本轮按用户“先不要编写代码”的要求交付[即梦服务拆解](workspace/content/research/2026-11-12-即梦多模态创作与Agent服务拆解.md)及[需求25](workspace/content/requirement/25-多模态创作与Agent编排需求.md)：36项功能要求、25项验收场景和8项待评审选择。REA两个隔离公开场景已退出，Chrome登录态逐类走查表单、Agent设置/Skill和已有图片历史后返回原首页；没有新消息、上传、模型消费或业务写入。已有CDP端点无法附着；公开数字人/动作深链采样与登录态表单不同，原因未定；网络采样有截断，不宣称即梦内部架构已逆向完成。

当前源码已具备七类入口、独立NestJS/Pi Durable Agent、专业子任务和多类媒体适配；新增需求聚焦精确能力一致性、正式产物接续、数字人已有手动文本配音链的Agent自动接续、真实专业并发及恢复/预算。Agent仍不接收音频理解，视频仅3帧投影；委派顺序等待，不能以并发上限宣称并行生产。以上为源码结论，未检查当前部署或执行真实供应商/账单接受。

建议评审顺序：先确认七类的精确开放能力与既有链路接受标准，再确定数字人配音接续/采用及多步骤预算；并行与受控脚本是既有目标，分别补安全设计、实际driver及接受证据，音频理解、按角色选LLM、深度创作和音色训练扩展另行裁决。已补入需求07零项目独立创作与项目内继承要求。需求接受后将架构变化回写各唯一来源设计，再同步PRD与Plan；本轮不新建实施分支，不启动迁移、部署或付费验证。

文档验证：新增链接/锚点、需求及验收ID、需求追踪和diff检查；没有运行应用测试。未提交推送，原有BACKLOG修改与frontend/.vitest/保留。

用户追加指出Agent体系过于简单，要求主Agent、子Agent、Skill调用、MCP、稳定执行及自动调用。已记录需求19 SRC-AGH-16/AGH-030～042，回写设计32 §7.6.9～10/7.11～13，PRD19增加12项接受条件和15项故障/自动协作场景，Plan28增加7个实施切片。七类创作文档仅承接，避免形成第二份Agent架构。方案明确真正并行的稳定branch映射、根预算分配、Skill固定依赖、Go受权MCP代理、任务内自动推进、原等待点/句柄恢复及故障矩阵；显式事件/定时规则为待接受扩展，不默认开启后台消费。

现有tool-only扩展检测MCP注册会拒绝，当前manifest没有完整MCP绑定；不据此报告已接通。官方MCP 2026-07-28协议及Tasks扩展已核验，具体SDK/服务互通、并发原型、隔离driver、持久唤醒和故障验证仍待实施证据。下一步评审主子/Skill自动选择、MCP归属和远端副作用合同、自动授权/等待与触发范围，再按PRD19/Plan28切片实施；本轮继续只修改文档，无连接配置、代码、测试运行、部署或付费调用。

## AI 接入极简重设计

来源：[DESIGN](DESIGN.md#极简呈现) · [设计07](workspace/content/design/07-AI平台接入市场调研与架构设计.md#ai-接入统一界面)。Figma更新49张接入、预设、连接、模型及测试状态画板，新增关键原型跳转，沿用Geist、共享token与组件实例。前端使用shadcn/ui，并按Vercel React/Next.js规范实现；模型配置无不可用原因或原因入口，价格参考与模型模板默认收起。模型多选使用官方水平Field、Checkbox与FieldLabel，保留原生标签、选中、禁用和焦点样式。

验证：提交范围独立检查312文件、2221项测试、ESLint、类型、改动代码格式、生产构建及standalone代理通过；完整工作区313文件、2227项测试及生产镜像构建亦通过。本地1122前端已更新。实际页面验证模型配置、价格及两步接入，390px无横向溢出；预设编辑通过现有模拟API夹具完成桌面、390px与键盘验证；多选验证标签点击、空格选择两项、16px复选框与14px默认标签，选择行无自定义背景。未调用真实供应商或保存业务数据。

限制：运行环境的后端域名在前端容器中未解析为当前服务容器地址，管理与会话接口间歇503，预设维护仍需恢复解析后补真实接口接受。全量Prettier有3个既有文件及生成测试报告的格式问题。Git交付与CI状态以对应提交记录为准；原有并行修改保留。

## 全站即梦逐项核对与组件复用（2026-11-07）

[即梦页面及交互清单](workspace/content/research/2026-11-07-即梦页面功能与交互逐项核对.md)与[织光/Figma核对](workspace/content/research/2026-11-07-织光页面与Figma还原核对.md)分别保留来源事实和当前实现；工具位置清单338条，覆盖39条页面路由及1条兼容入口，新增23条短剧首页控件与缺项。Figma原生节点/目标检查不等于全站1:1通过。实际数据、真实生成与复杂组合状态、当前不可用服务、字体差异继续逐项核验。全站旧视觉基线为ad1cf640，短剧新版的有限补核范围见下文，不能沿用旧版截图签收。

通用组件复用原则SRC-CAN-15已记录，用户确认Infinite Canvas为basketikun/infinite-canvas。[固定版本评估](workspace/content/research/2026-11-07-画布组件复用与完整接入评估.md)建议按职责比较，React Flow为交互层验证候选；本轮没有替换画布内核或安装新组件。

提交前复核短剧输入在项目创建后的承接，补账号/空间切换清空输入及创建弹窗、输入交接身份隔离与迟到清理保护测试。范围33项测试、全前端ESLint、类型检查和Next生产构建通过。全量测试中的会话恢复6项、项目页6项、资产工作区4项失败，在独立ad1cf640源码快照同样复现（109项中16失败）；本轮主题色规范失败已修正并定向复验。全量Prettier仍有既有3文件失败：public/director/catalog/manifest.json、public/director/motions/quaternius-ual1-standard.json、src/components/canvas/timeline/timeline-model.ts；本次范围格式通过。不得报告全量门禁通过，后续按实际需求区分产品缺陷和测试契约漂移。

代码提交eb96fbb9的最终全量复跑为303文件中300通过、3失败，2159用例中2143通过、16失败；16条失败名称与上述基线完全相同。运行版尚未替换，新的短剧表单与侧栏分组不沿用ad1cf640的实页接受结论。

后续逐项核对修正三入口无必要输入仍可启动的问题，并移除把500字计价单位误作输入上限的截断；新增6项回归。原16条失败按现有API合同修正测试：只允许带当前身份proof及可写空间的无body creation-context请求，继续禁止草稿/报价/付费写入；独立画布创建及资产入口测试保留幂等和迟到回执保护。相关109项通过，全量304文件2165项通过（`pnpm exec vitest run --maxWorkers=2`）；首次全量与构建并行时导演台实际动画测试超5秒，隔离负载后完整复跑通过，没有放宽超时或删除用例。全前端ESLint、Next生产构建、改动文件Prettier及diff检查通过。既有3文件全量格式问题未修改。

独立1124生产预览已实开三个短剧入口，核实空输入禁用、URL参考、自由文本风格及转绘布局。后端配置地址仍为原验收地址，1124出现创建恢复origin提示及模板403；保留原记录，仅关闭提示，不执行创建/生成。Chrome已恢复1122首页。Figma桌面/手机创作空态Button已原位改为disabled，另外两种空态原已禁用；视觉回读通过此局部状态检查，仍保留目标示例及字体限制。新增逐项差异表明确多媒体参考、风格库、选题、六项比例、模型/集数、粘贴弹窗、视频时长与比例、亮点横向图文入口未对齐。下一步按需求23/设计38/PRD24分解接入合同，先补真实数据和输入状态，再核验创建、导入、生成、转绘产物；不以入口可见代替闭环。

已推送main的72f4eeea对应[CI 42381591869](https://github.com/StephenQiu30/glowloom/actions/runs/42381591869)已结束：Agent通过，前端和后端失败。前端原16条本地修正后通过；后端报告画布项目创建约束、供应商组合筛选、报价/余额与主体引用用例失败，本轮没有改动后端或将其标为通过。补丁推送后的CI结果需单独追踪。

## 短剧多媒体参考与私有风格库（2026-11-07）

[当前实现对照](workspace/content/research/2026-11-07-织光页面与Figma还原核对.md#多媒体参考与风格库当前实现对照2026-11-07)及[独立核对CSV](workspace/content/research/2026-11-07-多媒体参考与风格库实现核对.csv)记录本轮工作区增量，原338项CSV及其旧版本实页/Figma事实继续保留。新增28项按源码、工程、隔离组件、业务运行版、供应商与Figma分别记录，不是28项全部接受。

源码已接入真实个人素材选择、项目复制及草稿承接，图片/视频/文档各最多30个来源。风格库有真实API、当前归属内官方/本人目录及本人CRUD，名称20码点、描述2000码点、最多4图且首图为封面，revision CAS与永久幂等回执处理冲突和未知写结果。采用风格先读取并冻结详情。分析风格先复制参考图，再由用户选择模型、预算及发送，并明确采用回复。TXT/DOCX读取全文后按上下文预算接收或拒绝。视频实际输入是10%/50%/90%三帧投影，受所选模型图片预算约束，不含音轨或全片理解。视频链接仅解析允许的抖音、头条、西瓜公开网页，下载真实MP4后经私有上传管线入库，仍执行500MiB和既有60秒限制，不能保证任意分享页成功。

本轮前端全量2206项通过，新增紧凑文档菜单4项、复制版本及草稿12项定向回归通过；风格组件32项、Agent15项通过。最新schema隔离空库初始化与最终31主/48含子Race通过；scoped lint/vet/类型/生产构建通过。CUA在受控HTTP夹具核对桌面与390px表单、四图上限/封面、三类素材选择/预览、创建采用与链接失败；这不代替真实服务端到端。Figma新增11张可编辑状态并截图回读桌面和手机表单。现有业务数据库尚未部署新增schema（包括可信upload_request.source_kind），1122仍为旧运行版。后续需取得存量DB切换、新运行版个人库到项目复制、媒体入模及手动分析采用的完整实页回执、三平台真实成功样本和逐页像素Figma差异接受。私有CRUD/CAS/幂等/撤权及源文件读取已有隔离PG/受控Harness证据。新供应商与真实付费产物、存量数据库切换和全站1:1仍待独立验收，不能沿用旧338项或旧运行版接受结论。

## 短剧工作室对齐即梦（2026-11-06）

[独立需求](workspace/content/requirement/23-短剧工作室对齐即梦需求.md) · [实现对照与 Figma](workspace/content/design/38-短剧工作室一致性设计.md) · [验收](workspace/content/plan/33-短剧工作室实施计划.md)。

已接通短剧项目工作区、文本和文件剧本导入、项目 Agent 剧本草稿、制作资产库、分集确认、分镜编辑及故事板助手建议。视频阶段接入片段生成、视频参考转绘和同项目画布时间线。仍待验证真实供应商产物，并补齐独立转绘、逐镜产物归档与合成全集、精选只读画布和复制、智能预演、片段重拍、超分及拉片。设计稿含目标态与示例内容。

## Canvas 服务命名与目录整理（2026-11-06）

`canvas/`、Compose `canvas:4400` 和 `@glowloom/canvas` 已同步，目录与依赖见[设计 17](workspace/content/design/17-画布Yjs协作编辑设计.md)。Canvas 106 项测试、前端画布目录 949 项测试和类型检查、Go 画布/Agent 命令/项目复制相关 race 集成、Compose 配置及独立镜像健康和鉴权验证通过。现有计算和协作模块沿用 `noImplicitAny` 例外。

<a id="agent-nestjs-migration"></a>

## Agent 服务 NestJS 迁移（2026-11-06）

来源：[需求22](workspace/content/requirement/22-Agent服务目录与TypeScript规范需求.md) · [设计37](workspace/content/design/37-Agent服务目录与TypeScript规范设计.md) · [Plan28](workspace/content/plan/28-完整创作Agent与Skill多Agent实施计划.md#nestjs-框架切换顺序)。

| 范围       | 工程状态                                                                                                                                                                                                                    |
| ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 单版本源码 | NestJS 接管入口、HTTP/SSE 和生命周期。删除旧 HTTP 应用、TypeBox HTTP 合同与脚本。Go 使用重新生成的客户端，Agent 使用独立 Go Swagger 生成的业务客户端。                                                                      |
| 目录职责   | 入站统一在 `http/`，Go 客户端和生成物统一在 `business/`。移除独立 Nest 装配包装与连接池关闭回调。复验 OpenAPI 和编译后的 Pi 定义指纹与目录整理前一致。                                                                      |
| Agent 门禁 | 冻结安装、类型检查、lint、格式、构建与双向生成校验通过。27 文件 146 项全过，含实际 socket、隔离 PostgreSQL、官方 Storage conformance 与进程恢复。                                                                           |
| Go 与镜像  | 全 backend 编译、Agent 范围 race、真实 Go→Nest→隔离 PostgreSQL、业务边界、公网 Swagger 合同及 vet 通过。固定 Node 24.22.2 的非 root Linux 镜像验证健康、就绪及 SIGTERM 正常退出。模型和工具使用受控 fixture。               |
| 正式切换   | 待盘点目标环境可接续运行、未决控制及未知业务回执。目标镜像定义指纹为 `4bb42e794eb9db16c658ae71fa412c01d6878bc5d5904dfa54c321bb7a72f6bf`，与切换前源码重建的编译指纹不同。按设计37 §7.5 收束无法接续的运行后执行单版本切换。 |

生成 TypeScript 声明通过独立 `pnpm api:typecheck` 严格校验。全项目关闭 `skipLibCheck` 时，外部 Pi、Chord 和 Google GenAI 声明仍报错。Agent 相关 Go 包的 `govulncheck` 无可达结果。

<a id="model-onboarding-alignment"></a>

## 模型接入与预设对齐（SRC-AI-46，2026-11-06）

[调研](workspace/content/research/2026-11-06-BeefTV与竞品模型接入和预设对照.md)与[设计07](workspace/content/design/07-AI平台接入市场调研与架构设计.md#src-ai-46)已对齐。新建连接入口已换成「连接服务 → 选择模型」，保存前可读预设注册端点的模型 ID。首批目录适配器为 OpenAI、DeepSeek、OpenRouter、Vercel。保存只实例化确认的草稿，未知结果先读取原回执。已有连接的「添加模型」直达模型管理的共用选模入口。目录工作器使用已保存凭据仅读取候选 ID，选中后核对目录回执、配置及凭据。连接详情新增「发布复核」，展示历史回执与逐模式正式证据，批量启用和恢复均执行逐项门禁。旧发布状态及冻结任务保留。

验证使用模拟供应商响应和隔离 PostgreSQL，覆盖目录权限及出站边界、精确选择、空集合、请求重放与无价格或发布写入。前端全量 Vitest 300 文件、2141 用例通过。最终界面调整后，相关 15 文件、151 用例、ESLint、生产构建和 390px／1440px 浏览器验证通过。全 Catalog Race 检查的组合过滤失败属于并行变更范围。未使用真实凭据、调用生成或部署。

续次验证：前端全量 Vitest 300 文件、2143 用例通过，生产构建、定向 ESLint、Go vet、接口生成一致性通过。隔离 PostgreSQL Race 验证已存凭据目录 GET、凭据状态不变、候选回执匹配、缺一模式拒绝及恢复门禁。受控浏览器验证 390px 读取目录并保存草稿、1440px 查看复核记录。全量 Catalog Race 仍只报告已有的供应商组合筛选用例失败。未使用真实供应商或改动业务数据库。

提交前在独立工作树验收，前端 Vitest 298 文件、2134 用例通过，Next.js 生产构建、定向 ESLint 和 Prettier、Go vet、初始化独立 PostgreSQL 后的全量 Catalog Race 测试均通过。agent-browser 在 390px 和 1440px 使用模拟 API 验证两步接入、已存凭据目录、草稿保存与逐模式复核。

<a id="model-presets-audio-20261109"></a>

## 预设持久化、音频与计量一致性（SRC-AI-48，2026-11-09）

来源：[需求04 FR-AI-57～64、AC-AI-38～43](workspace/content/requirement/04-AI平台接入与预设管理需求.md#src-ai-48)。用户已追加“根据我们的需求和设计，开始展开实现”，替代初始仅文档范围。先前17份文档已提交main并推送`e091cb1b`；本次实现按以下工程边界追踪，不继承旧SRC-AI-47接受结论。

| 任务/裁决                | 当前状态                                                                                                                     | 剩余接受与依赖                                                                                                |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| 需求及设计               | 已记录实施追加授权；同步来源设计、Plan及数据归属                                                                             | 仍区分工程、真实供应商、账单和产品接受                                                                        |
| T-AI-48-DATA 全量SQL种子 | 10云预设、12供应商版本、4能力和129版本化候选快照已落data.sql；当前95候选全保留，运行时读取固定PG快照，删除Go候选常量目录     | 空隔离库、幂等及不可变性已测；实际业务库经备份和增量演练完成迁移，原账本保留，未重放schema                    |
| T-AI-48-AUDIO 音频闭环   | TTS字符、Seed-Audio原始十进制时长接入共享计价核心；完整终态、缺失/冲突usage与unknown有回归                                   | 未做真实音频生成、音色权限、输出与账单接受；音乐保留原候选，不把旧驱动存在当真实接受；ASR/实时/训练仍在范围外 |
| T-AI-48-BILL 算法与积分  | 新策略绑定来源快照、采购和销售；120秒上限与预估分离，显示每60秒；普通创作与Agent复用商业整数规则，保留历史价格/operation语义 | 无完整计量或价格合同的模式继续禁止新发布；其他价表与真实账单逐型号核验                                        |
| T-AI-48-UI 列表两态      | 服务端投影、筛选后分页、详情原因及公开目录/创作/对话/画布音频已接线；报价、确认和Agent重检同一门禁                           | 模拟320px/键盘验证有据；运行版准入与计费路由另有实测，不把夹具视为完整页面接受                                |
| 官方费率和计量           | 追加成功读取官方当前计费说明，持久化TTS2.0后付费3元/万字符、音频生成1.0后付费1元/分钟公开参考                                | 未自动审核销售价或启用模型；其他历史参考不据本次推定仍有效                                                    |
| 质量门禁                 | 新增/相关用例按TDD；隔离PG、Race、API生成、前端类型/交互及适用静态检查分层记录                                               | 全仓原有失败与本次差异比较见本地验收记录，不能声明整个仓库全绿                                                |
| 产品接受与发布           | 源码、公共种子和隔离工程验证完成；追加授权后已迁移业务库、本地部署及实测Ark，本次Git交付已获明确授权                         | 真实样本仅覆盖已有Ark，累计预算≤¥20；语音及供应商账单接受独立保留                                             |

MVP保留Go及独立Agent/Canvas，不新增LiteLLM网关或钱包。目录中`image_pixels_tiers_v1`、`image_token_buckets_v1`、`provider_invoice_v1`、`unverified_metering_v1`等未安装/未核验策略仍保留候选并失败关闭；不能将95个候选持久化解释为95个型号已真实可用。缺执行合同、审核价格或当前真实模式证据的项目保持不可用，不用删候选或伪造零价补齐覆盖。

代码与分层证据见[Plan29](workspace/content/plan/29-模型与供应商接入扩展实施计划.md#src-ai-48)、[Plan30](workspace/content/plan/30-模型计价透明与结算恢复实施计划.md#src-ai-48)、[Plan10](workspace/content/plan/10-AI平台接入与预设管理实施计划.md#src-ai-48)。本机验收记录与逐项文件清单：`/Users/stephenqiu/.codex/visualizations/2026/11/09/01a1bee1-0086-7082-9022-39fe77a650ce/implementation/`。

<a id="official-model-contracts"></a>

## 官方模型接入、预设与参数计价（SRC-AI-40，2026-11-05）

用户要求补足官方接入、模型定价、CC Switch预设与算法细节。[官方研究](workspace/content/research/2026-11-05-官方模型接入预设与计价核验.md)已核首批12个精确ID及多维采购价、参数差异，另列媒体/Gateway与缺证范围；CC Switch固定`ca58c0313360eae7c851fbd7553c1614c7af03e7`。模型/价格未写业务库，未增加供应商调用，不把本轮官方网页核验当账号真实验收。

主链：[需求04 SRC-AI-40](workspace/content/requirement/04-AI平台接入与预设管理需求.md#src-ai-40) → 设计33/PRD20/Plan29（预设与参数合同），设计34/PRD21/Plan30（计量与计价策略）。商业积分仍依设计36，不另建报价引擎。

| 任务                                | 当前状态                                                                                             | 下一步与依赖                                                                                            |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| T-AI-40-RESEARCH 官方/CC Switch证据 | 本轮研究及设计补齐；具体参数和单价为有日期参考                                                       | 账号可见、精确模式、真实用量及发布接受另验；未核全量第三方渠道型号                                      |
| T-AI-21-05～08 预设/参数/发布       | 首批后端代码：来源修正、BytePlus AP图片候选、SKU约束、模板/release矩阵校验；单元/race通过            | 隔离PG正式发布用例通过，GORM扫描结构修正；分组体验/全部媒体矩阵另续，不标整体接受                       |
| T-AI-22-04～07 计量/价格策略        | 原引擎开放image_flat_v1、token_io_v1、seedance_video_tokens_v1，冻结版本并拒绝未知分支；定向race通过 | 继续实际面积/媒体桶及上界合同；商业精确积分/钱包未实现，旧冻结价与账本不变                              |
| 参考视频/OpenAI2.5商业准入          | 未完成全部计量/预留依据                                                                              | 视频最低Token完整表；OpenAI2.5估算上界、用量/账单合同；不以旧计算器或无价0替代                          |
| 其他渠道/媒体补核                   | BytePlus AP、Visual/语音/Gateway有官方依据，仍有精确版本/地区/计量缺口                               | EU按当前账号/目录核验；音乐直连现价未知；TTS价格仅官方索引正文需补发布证据；OmniHuman req_key不自动映射 |

### 官方价目同步与倍率（SRC-AI-47，2026-11-08）

唯一链：[需求04 SRC-AI-47](workspace/content/requirement/04-AI平台接入与预设管理需求.md#src-ai-47) → [设计34 §7.15](workspace/content/design/34-积分消耗系统与模型真实计价体系设计.md#src-ai-47) → [PRD21](workspace/content/prd/21-模型计价透明与结算恢复.md#src-ai-47) → [Plan30](workspace/content/plan/30-模型计价透明与结算恢复实施计划.md#src-ai-47)。用户于2026-11-08授权工程实现；当前为本地实现，正式价格发布、模型启用、业务库迁移和真实收费仍关闭。默认同步只生成候选，管理员审核后才能形成新的采购/销售版本。

| 范围               | 当前实现                                                                                                                                                                                                                                                                             | 剩余工作                                                                                                                                                                                           |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 官方来源覆盖       | OpenRouter `/api/v1/models` 无凭据读取，限定到预设/已配置文本模型；扁平 USD/token、cache-read 可归一化，overrides/图片/请求等留作人工审核。OpenAI、DeepSeek、Ark官方页面按精确预设ID提取表格证据；Seedream Flash简单按张价、DeepSeek Flash/V4 Pro完整2026峰/闲价表可生成待审核规则。 | 直连页表格变体及模型名称映射可能变化；DeepSeek只用已核验的2026北京时间日历，缺/重复档、峰闲不成半价或未知年份均退回`needs_review`；审核前不自动发布。三家模型列表API无机器价目字段，不能推断价格。 |
| 每日候选与历史     | `catalog.model_price_candidate`、source hash/parser version、`catalog.price_sync` 持久任务及模型详情候选摘要已接入；每天约03:00 UTC读取OpenRouter目录并按来源分组读取三家官方价目页，只写候选。                                                                                      | disposable PostgreSQL 未配置；新表、候选upsert和原子候选发布未做隔离数据库集成验收。时区/失败告警仍待评审。                                                                                        |
| Redis缓存          | 价格规则按不可变 `price_version_id` 缓存在Redis 1小时；公开模型目录仍由PostgreSQL选择当前发布版本，cache miss/error回源数据库。                                                                                                                                                      | 未做Redis集成测试/性能基准；报价与结算仍按原数据库快照；进程级规则预编译尚未实现，不声称计算耗时已改善。                                                                                           |
| 销售倍率与审核发布 | 管理表单支持倍率；只写入独立销售规则，采购规则保持原价。结构化OpenRouter、Seedream Flash和DeepSeek 2026候选要求规则完全匹配；其它直连页面候选允许管理员手动录入核验规则；候选状态与采购/销售版本在同一事务发布。                                                                     | 倍率范围沿既有校验（>0、最多6位小数、≤1,000,000），商业售价与利润政策仍需逐模型审核。                                                                                                              |
| 产品和工程接受     | 最新Catalog/Operation/Billing scoped Race 912项通过，Catalog 369项通过；scoped vet及DeepSeek parser包lint通过。前端API生成核对、typegen、TypeScript、全量ESLint、Prettier及ModelDetail 9项通过，Next build成功。                                                                     | PostgreSQL/Redis test DSN 未配置；全仓Race、vet/lint仍有Canvas/Agent/identity等失败，Catalog lint另有既有空白问题；真实页面价目变化、账单核对和浏览器产品接受未验证。CASE-BIL-23～29未全部接受。   |

<a id="commercial-credits"></a>

## 模型商业计费与积分（2026-11-05）

来源：[需求21 SRC-CREDIT-01](workspace/content/requirement/21-模型商业计费与积分系统需求.md)；主链：[设计36](workspace/content/design/36-模型商业计费与积分钱包设计.md) → [PRD23](workspace/content/prd/23-模型商业计费与积分系统.md) → [Plan32](workspace/content/plan/32-模型商业计费与积分系统实施计划.md)。[竞品研究](workspace/content/research/2026-11-05-模型计费与积分竞品研究.md)完成GitHub固定版本与REA未登录页面走查；未做竞品真实充值/失败返还。

| 任务                                 | 当前状态                                                                                                                                                                                                                 | 下一步/依赖                                                                                                                                                                                                                                              |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| T-CREDIT-00 商业政策                 | SRC-CREDIT-02确认1元=10积分、新生成确认价封顶、平台承担超额、未知继续冻结                                                                                                                                                | 商业商品与周期会员单独评审                                                                                                                                                                                                                               |
| T-CREDIT-01 精确积分、采购销售与封顶 | 精确报价、原预留冻结封顶、双价格版本和回执计量已落地                                                                                                                                                                     | 按实际供应商账单核对采购事实；复杂计量继续依模型合同准入                                                                                                                                                                                                 |
| T-CREDIT-02 账户、批次与原子消费     | 个人/团队钱包、项目绑定、批次来源、先到期先用、事务双预留及终态约束                                                                                                                                                      | 隔离库验证，业务库切换另行安排                                                                                                                                                                                                                           |
| T-CREDIT-03 支付与现金退款           | 本轮用户选择隔离验收，受控赠送批次覆盖消费链                                                                                                                                                                             | 后续接入选定渠道及现金退款                                                                                                                                                                                                                               |
| T-CREDIT-04 消费补偿与核查           | 受控返还与过期宽限固定，重复操作幂等                                                                                                                                                                                     | 自动售后政策继续按失败原因审批，未知保留原预留                                                                                                                                                                                                           |
| T-CREDIT-05 批量短剧与Agent预算      | 单次、批量和Agent复用钱包预留，Agent冻结服务端计算价                                                                                                                                                                     | 实际短剧的供应商费用验收待明确测试范围                                                                                                                                                                                                                   |
| T-CREDIT-06 界面与会员周期           | 积分总览、流水分页、付款方及采购版本选择已接入                                                                                                                                                                           | 正式渠道、商品和周期权益待评审                                                                                                                                                                                                                           |
| T-CREDIT-07 独立创作与整数积分       | 2026-11-06 用户要求及即梦证据已记录，尚未实施                                                                                                                                                                            | [独立创作](workspace/content/requirement/07-项目与画布创建原始需求.md#src-pc-08)和[付款准入](workspace/content/requirement/21-模型商业计费与积分系统需求.md#src-credit-04)待联合设计。先做存量库钱包增量迁移与历史回执保护，统一新报价的整数积分预估、冻结与实扣，再完成首模试点。 |
| T-CREDIT-08 异步命令与接入方幂等     | grant/debit/freeze/commit/release、稳定command_id、HMAC服务身份、key ID轮换、scope/action映射、服务余额/流水与回执查询已实现；配置/HMAC单测和本机Redis nonce集成通过                                                     | 配置并演练真实caller的密钥轮换；评审幂等凭据保留策略；operation内部caller保留身份由T-CREDIT-10接入                                                                                                                                                       |
| T-CREDIT-09 CAS与不可变账本          | 账户/批次版本CAS、不可变有符号流水、来源去重及账户安全初始化函数已实现；本机应用角色测试通过                                                                                                                             | 对账本全量重建和跨账户恢复流程做演练；财务保留策略评审后再配置归档                                                                                                                                                                                       |
| T-CREDIT-10 Outbox、Kafka与SAGA      | Outbox事务投递、退避/DLQ、Kafka有界重试与死信已实现；operation冻结命令、派发屏障、终态/未知事件和失败补偿已接入，内部caller保留；等待冻结的task持久重排，开关默认关闭；operation集成用例已添加但本轮因无隔离DB DSN未执行 | 使用已启动的本机PostgreSQL为operation/billing用例提供隔离数据库并核验`glowloom_app`角色；完成故障注入、DLQ重放、旧/新路径切换回退及共享账户锁顺序演练，再评审开启开关                                                                                    |
| T-CREDIT-11 Redis余额投影            | 5秒TTL、版本闸门、事件失效及数据库回源已实现；本机Redis缓存一致性测试通过                                                                                                                                                | 观测命中率、缓存年龄及回源；根据业务SLO评审TTL                                                                                                                                                                                                           |
| T-CREDIT-12 月分区与全局去重         | UTC月分区、跨分区流水身份登记及动态分区边界测试已实现；本机PostgreSQL billing/outbox测试通过                                                                                                                             | 评审财务保留期、归档和分区告警；持续观察分区维护与历史查询                                                                                                                                                                                               |
| T-CREDIT-13 限流与防滥用             | caller/scope/IP分钟限流、单笔上限、caller/scope UTC日累计额度、HMAC签名/时戳/nonce防重放及Redis故障fail-closed已实现；本机Redis日额度与nonce集成通过                                                                     | 评审并配置QPM/单笔/日额度；用户维度控制、grant双人审批/风险暂挂及异常行为审计告警仍未完成                                                                                                                                                                |
| T-CREDIT-14 压力与性能验收           | 可执行负载和故障场景已写入Plan32；本轮未做容量验收                                                                                                                                                                       | 责任人先确认吞吐/延迟/积压SLO，再用本机已启动服务记录基线；不因无阈值而报告容量通过                                                                                                                                                                      |

本轮验收使用独立PostgreSQL、MinIO与模拟供应商回执。余额、冻结、扣减、补偿和双价格均可在隔离环境追溯。

2026-11-08 SRC-CREDIT-05仅复用用户已启动的本机服务；未启动额外容器。本轮未配置隔离operation/billing数据库DSN，因此新增operation SAGA集成用例未执行，也未连接或改写Glowloom业务数据库。命令账本、基础SAGA事件消费与服务caller安全证据见Plan32 §11；operation切换/故障验收、用户维度防滥用、审批审计和性能容量验收仍待完成。金额/日额度缺少评审值时写命令fail-closed，不据此推断生产服务已启用。

> **当前聚焦里程碑**：完整创作 Agent Harness、多 Agent 协作系统与 Skill 沙箱体系
> **已确认技术方向（2026-11-04）**：同仓库独立 TypeScript/Pi Durable Agent 服务；Go 保留业务权威，Agent 独占执行状态；PostgreSQL 分 schema/角色；Docker Compose 统一启动；无 Temporal、旧实现或兼容。完整设计见[设计32](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)，状态见[本轮迁移](#pi-durable-migration)。

---

## Agent 文档链与验证原则

本节的唯一主链指Agent执行基础；对话产品专项沿[对话工作台](#conversation-workspace)补充，不重置独立Agent迁移的进度或验收。

唯一当前链：[需求19](workspace/content/requirement/19-完整创作Agent与Skill多Agent需求.md) → [设计32](workspace/content/design/32-完整创作AgentHarness与Skill多Agent架构设计.md) → [PRD19](workspace/content/prd/19-完整创作Agent与Skill多Agent.md) → [Plan28](workspace/content/plan/28-完整创作Agent与Skill多Agent实施计划.md)。旧 Go 图引擎、旧单图阶段与假 sandbox 命令的完成清单已删除；历史版本由 Git 追溯，不作为当前实施指南或验收。

工程测试、隔离基础设施、实际供应商与完整创作接受分别记录；未提交代码、镜像构建或健康页不能代替真实产物与计费验收。

<a id="conversation-workspace"></a>

## 2026-11-05 对话工作台与参考输入一致性

用户通过需求20 SRC-CONV-07/08授权从知识库开始并确认实施。唯一链：[需求20](workspace/content/requirement/20-对话工作台与参考输入一致性需求.md) → [设计35](workspace/content/design/35-对话工作台与参考输入一致性设计.md) → [PRD22](workspace/content/prd/22-对话工作台与参考输入一致性.md) → [Plan31](workspace/content/plan/31-对话工作台与参考输入一致性实施计划.md)。基础Agent仍沿设计32/PRD19/Plan28，不在此重新声明完成。

| 工作                                            | 当前状态                                                                                                                                                                                                                                               | 依赖/下一步                                                                                                                                                                                                                                |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| T-CONV-DOC 需求、设计、PRD和Plan                | 主链及15项AC、14组场景、9项任务已落盘；用户确认实施，接口与文件责任随实现同步                                                                                                                                                                          | 维持单一来源，产品接受分别记录                                                                                                                                                                                                             |
| T-CONV-01～03 合同、输入、持久历史              | 工程实现已落地：私有分页、不可变完整输入、引用/模型检查、同commit公开消息、原提交查询、Go控制回执与未决消费屏障；旧历史离线回填脚本可重入                                                                                                              | 未向业务库应用增量；部署前盘点在途run/operation                                                                                                                                                                                            |
| T-CONV-04～08 共享参考、客户端、线程/结果和页面 | 首页/对话页/富文本共用参考动作；独立路由、真实私有历史、未知发送、停止恢复、追加尝试、正式任务详情均接线。登录/项目隔离、源撤权、重试原设置和分页缺口有回归。旧Agent Textarea入口已移除                                                                | 真实产品环境登录/上传/触控IME等接受仍与隔离验证区分                                                                                                                                                                                        |
| T-CONV-FIGMA 设计一致性                         | 已只读核对66个原型状态，更新入口3条说明，新增[Figma工程状态合同](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3491-92778)；明确受理未知、Stop投递中和引用撤权                                                                           | 文档与原型不是供应商接受；未重建或宣称新增全部状态画面                                                                                                                                                                                     |
| T-CONV-09 工程/浏览器验证                       | Agent 134例及官方Storage/恢复通过；Go范围race、实际Go→编译Node24.22.2→Pi→隔离PG及业务权限测试通过；前端295文件/2101测试、类型、ESLint、格式及生产构建通过，实际浏览器已核对桌面、320/390/768、参考展开/移除、发送、Stop pending→applied及retry保留草稿 | 全仓Go race有未配置集成环境及并行模块失败；govulncheck有15项当前Go标准库可达告警，golangci-lint未安装；不冒称全仓门禁通过                                                                                                                  |
| T-CONV-AUDIT-1105 页面复核与修复                | 对照即梦已有对话与Figma，修复Agent空输入提示、异步结果底部跟随，补齐结果大图预览/正式原件下载；走查时前端296文件/2105测试及生产构建、范围ESLint/格式通过；桌面与320/390浏览器核对发送、Stop、retry、参考与结果动作                                     | 1122真实首页提示已复验；登录后对话因当前登录服务不可用未验收，交互与下载采用不连接供应商的隔离fixture；新增积分0；用户已授权仅提交本专项并推送main                                                                                         |
| T-CONV-ACCEPT-1105 现有服务验收                 | 服务恢复后 agent-browser 真实登录、项目、参考读取/提及/移除、去重上传201、历史/技能200、320px与既有图片原文件读取200已核对；修复 Agent 代理旧白名单404与SSE续传/期限合同。前端2122测试及构建、Go相关Race、Agent构建通过                                | 待验收：用户确认目前无可用于零费用验收的规划模型或项目；待具备条件后复验发送、停止、重试、新结果卡与媒体队列取消。Agent26测试仍缺专用数据库而失败。复验新增生成费用为0；详见[验收记录](workspace/content/research/2026-11-05-对话工作台现有服务验收.md) |
| T-CONV-10 即梦对话改版                          | 独立竞品基线、实现差距及九个 Figma 状态已整理。前端已统一对话命名、技能搜索及 `/` 入口、回合活动与 AI 标识。全量前端 299 文件、2148 项通过，生产构建、修改范围 lint/格式及隔离 Chrome 验证通过。                                                       | T-CONV-11～13 仍需接入持久历史管理、混合时间线与检索、独立媒体参数。真实供应商调用未执行。                                                                                                                                                 |
| T-CONV-11／13 对话历史与创作类型                | 七项类型、草稿隔离与真实项目编辑器通过实页验收。历史管理通过隔离浏览器、Agent HTTP 和 PostgreSQL 验收。证据见[创作类型与对话历史对照验收](workspace/content/research/2026-11-06-创作类型与对话历史对照验收.md)。                                                    | 现有本地数据库增量与 Go／Agent 部署未执行。真实模型和结算另验。T-CONV-12 混合时间线独立。                                                                                                                                                  |
| T-CONV-14 新建对话空态                          | 800px 内容栏、真实项目技能推荐、资产库入口与输入聚焦已接线。前端全量 300 文件、2151 项通过，生产构建、范围 lint/格式及隔离 Chrome 通过。隔离服务确认一次提交携带正确的技能版本。                                                                       | 正式账号的技能目录与实际模型执行仍按真实服务验收记录跟踪。                                                                                                                                                                                 |
| T-CONV-14 对话空态实页复验                      | 修复无技能时缺卡、项目选择遮挡和输入区尺寸偏差。agent-browser 核对真实空态、回填、项目切换、设置、参考入口和窄屏，隔离验证发送、停止、重试、历史及结果。详见[验收记录](workspace/content/research/2026-11-06-对话空态即梦对照与浏览器验收.md)。                     | 真实项目无可用对话模型，生成链路仍待验收。                                                                                                                                                                                                 |
| 真实模型、媒体队列取消与费用接受                | 未执行；新增积分与真实生成费用均为0，先前即梦10分已用完                                                                                                                                                                                                | 未解除真实收费禁令；上线和真实供应商接受需另行授权                                                                                                                                                                                         |

所有本轮命令、边界、review修复、修改文件和剩余并行变更明细保存在git忽略的`.superpowers/sdd/31-对话工作台与参考输入一致性实施计划/`。原Docker测试容器不可连后未启动或修改业务栈，改为任务拥有的临时PostgreSQL两个全新`*_test`库；原业务库未改。保留共享工作区并行改动，HEAD仍为`8549d9d5`，未提交、推送、创建分支或发布。

## BeefTV调研采用：模型接入扩展、计价与结算恢复（2026-11-03）

本节为该专项实施状态的唯一来源，不改变前文Agent里程碑记录；前文测试/提交记录仅按其原版本理解，不能作为本专项验收。

**文档、代码实施与独立整体工程规格/质量审查已完成；用户于2026-11-03授权实施。本次实现相关代码、测试、文档与BACKLOG按用户后续授权提交main并推送；CI独立于既有本地工程验证。首个Seedream 5.0 Flash已按SRC-AI-36完成本地真实文生图/图生图、COS归档、启用及首页免费报价接受；其他模型、个人积分钱包及完整产品接受未完成。** 设计评审、商业政策接受、代码实现、实际供应商和产品接受分别记录。

来源：[调研](workspace/content/research/2026-11-03-BeefTV模型接入与计费调研.md)与[需求04 SRC-AI-35](workspace/content/requirement/04-AI平台接入与预设管理需求.md#src-ai-35)。

| 父任务                     | 唯一设计→PRD→Plan                                                                                                                                                                | 当前状态                                                | 优先级建议/依赖                                  |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------ |
| T-AI-21 接入声明与协议扩展 | [设计33](workspace/content/design/33-多设计模式驱动的模型与供应商接入扩展体系设计.md) → [PRD20](workspace/content/prd/20-模型与供应商接入扩展.md) → [Plan29](workspace/content/plan/29-模型与供应商接入扩展实施计划.md) | 工程实现/任务复核通过；整体工程审查通过，联合接受未完成 | P2；补原任务恢复联动及产品接受                   |
| T-AI-22 计价与费用说明     | [设计34](workspace/content/design/34-积分消耗系统与模型真实计价体系设计.md) → [PRD21](workspace/content/prd/21-模型计价透明与结算恢复.md) → [Plan30](workspace/content/plan/30-模型计价透明与结算恢复实施计划.md)       | 工程实现/任务复核通过；整体工程审查通过，合同门控保留   | P1；D-AI-35-01/02影响对应功能                    |
| T-AI-23 结算证据与恢复     | 设计34→PRD21→Plan30                                                                                                                                                              | 工程实现/任务复核通过；整体工程审查通过                 | P1；保留unknown保护，和现有operation/billing对接 |
| T-AI-24 Agent统一网关      | 设计32→PRD19→Plan28                                                                                                                                                              | 沿前文Agent主链；本节不重置其状态                       | 仅依赖费用授权/原operation身份，不重复计算交付   |
| T-AI-25 错误与原币对账     | 设计34→PRD21→Plan30                                                                                                                                                              | 结构化分类工程复核通过；原币部分仍待合同                | P2；25-01可独立核查，25-02依赖D-AI-35-03         |

### 6.1 可执行任务

以下记录当前授权范围的实施与接受证据；无固定工期承诺。工程通过、产品接受、真实供应商接受分别判断。

| 任务ID     | 最小交付                  | 依赖                        | 当前状态/下一步                                                                               | 验收                    |
| ---------- | ------------------------- | --------------------------- | --------------------------------------------------------------------------------------------- | ----------------------- |
| T-AI-21-01 | 公开安全元数据与生成API   | 用户已授权按现有设计实施    | 工程复核通过；真实PG+HTTP session/epoch/跨组织矩阵通过                                        | CASE-AIX-04             |
| T-AI-21-02 | 受限声明及预设事务版本    | 21-01                       | 工程实现/复核通过；直接保存/版本/预览统一严格校验，隔离PG通过                                 | CASE-AIX-01/02/03/05    |
| T-AI-21-03 | 协议组合与单次派发        | 21-02                       | 整体工程复核通过；POST=1，已发4xx/矛盾迟到回调保持未知费用，不误释放预留                      | CASE-AIX-03/06          |
| T-AI-21-04 | 极简元数据界面与联合接受  | 21-01～03                   | 组件/API/类型/构建通过；Flash本地桌面真实两模式/发布/首页报价通过，其他模型及窄屏读屏仍未接受 | CASE-AIX-01～06         |
| T-AI-22-01 | 价格来源/版本与金额表达   | D-AI-35-01                  | 工程复核通过；所有新价入口统一来源门禁，DirectUse无价候选不造零价                             | CASE-BIL-01/03/04       |
| T-AI-22-02 | 参考图计数及收费scope     | 22-01、D-AI-35-02           | 严格数学/解析与缺证据拒绝通过；正式发布仍待逐模型一手合同                                     | CASE-BIL-02             |
| T-AI-22-03 | 预估/预留摘要和明细       | 22-01                       | 整体工程复核通过；冻结模式/分辨率系数、unit_only/unknown及unsafe金额禁新确认                  | CASE-BIL-01～04         |
| T-AI-23-01 | 独立费用/交付读投影       | 22-01                       | 工程复核通过；本地账本权威、持久冲突矩阵与完整交付事实                                        | CASE-BIL-06～11         |
| T-AI-23-02 | 服务端屏障及原任务恢复    | 23-01、既有workflow control | 整体工程复核通过；锁内屏障、原任务查证、当前请求人终态原交付及SDK/inbox重试                   | CASE-BIL-05/06/08/11/12 |
| T-AI-23-03 | 恢复UI与Agent回放联合接受 | 23-01/02、22-03             | 整体工程复核通过；恢复轮询/新旧查证键、Agent原operation、完整/可读产物及字幕                  | CASE-BIL-05～12         |
| T-AI-25-01 | 配额/权限/速率安全分类    | 23-01                       | 工程复核通过；同402/429不同证据分类，不推断退款/零成本                                        | CASE-BIL-09/11/12       |
| T-AI-25-02 | 原币账单与迟到退款补偿    | D-AI-35-03及真实验证授权    | 待合同；未关闭不开放，禁止Settle(0)改旧账                                                     | CASE-BIL-03/07/10       |

工程验证记录（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-03-beeftv-adoption/engineering-validation.md`）。Plan29提交前工程证据（以下命令在当时共享工作区执行）：隔离catalog全量及Race通过；协议/operation Race通过；前端279文件/2016测试、API一致性、ESLint、类型及构建通过。独立审阅发现的重复键保存绕过已按Red→Green修复并复核关闭。本次改动golangci增量检查0问题；全scope保留既有静态问题及18个无关前端格式文件，未扩展修复。fixture不替代浏览器/供应商接受。

工程补证：Plan30初审4项Important已按Red→Green修复；round1引入的legacy/reuse完整性兼容问题已在round2关闭，原独立PG失败用例转PASS。多产物中断仅在实际manifest全集归档后ready；账务冲突统一约束GET/quote/单批确认/dispatch；足额、部分释放及既有overage均不误封。修复后catalog Race371通过，operation Race663通过/26跳过；前端284文件/2035测试、类型、ESLint、API与生产构建通过。全仓/专库/最新family去重为3345个pass动作、882个skip、0fail；跳过不计接受。总审查初审1项Critical、6项Important及字幕问题全部修复；精确复核剩余迟到矛盾回调、实际SDK固定ID冲突两项亦已独立原样复跑关闭，整体工程规格与代码质量通过，无未关闭Critical/Important。最终operation整包676通过/26跳过，operation/media内部合计733通过；catalog去重371通过；前端284文件/2038测试通过，最终相关组件49通过。最后两项收口Race105通过/0跳过，变更行lint/vet/goimports/diff检查通过。未再次重复未变前端/其他包；真实供应商/浏览器/生产迁移未执行。

### 6.2 待确认与外部条件

| ID         | 待决事项                                                   | 当前处理与关闭条件                                                                                                             |
| ---------- | ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| D-AI-35-01 | 1:10积分/默认1.0的对外政策出处、分数积分显示、多级倍率覆盖 | SRC-AI-36仅确认Flash本次0.12元/张、ratio10、markup1；分数积分和全平台政策仍待确认                                              |
| D-AI-35-02 | 参考图/重复引用/mask/每输出计数、缓存/视频Token单位        | 逐模型取得供应商一手合同；未定义的附加费不发布                                                                                 |
| D-AI-35-03 | 原币最终账单、超预留承担、退款补偿及采购/销售分账          | 明确账单身份、冻结汇率、超额政策、迟到退款流水与授权；原币路径保持关闭                                                         |
| D-AI-35-04 | 真实供应商/浏览器/迁移接受                                 | Flash本地两模式、COS归档、增量结构补齐、启用与首页免费报价通过；首次0.12元预留仍待核对，其他供应商/生产环境/完整交互接受未完成 |

### 6.3 下一步与完成判据

1. 本轮可执行工程范围已完成并通过独立整体审查；按验证记录（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-03-beeftv-adoption/engineering-validation.md`）和逐项文件清单（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-03-beeftv-adoption/worktree-files.md`）审阅本次提交范围；首轮实现已提交main并推送（4ab20c15）；后续Flash运行修复沿同范围交付，本地仅启用已验证Flash。
2. 分别关闭D-AI-35-01～03的商业/供应商合同；未确认前不开放对应参考计费、原币账单和退款补偿，不以工程fixture充当真实费率。
3. 在获准的运行环境补D-AI-35-04：真实供应商、生产式 PostgreSQL 任务/Kafka/对象存储与原回执恢复，以及桌面/320/390、键盘焦点和读屏接受。除下述Flash本地观测外，未执行项不计通过。
4. 新问题或后续发布继续沿对应CASE留证，保留原价格/汇率/Key/预留/账本；不得用清零旧账、超时解锁、同键重POST或新Key补买替代核查。

### 6.4 首个Flash模型实际接受（SRC-AI-36）

运行验证记录（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-03-beeftv-adoption/runtime-validation.md`）：复用现有COS桶及凭据，242项旧对象位置经字节核验绑定；文生图、图生图各成功并结算0.12元，已启用并设为图片能力默认。首页实际选模/免费报价显示0.12元、预估2积分；没有执行第四次供应商请求。

- 已完成：模型详情失败重试、试运行价格来源投影、Ark不支持参数移除、Pro/Flash尺寸候选修正、试运行模式空值同步修复及对应回归。
- 待核对：原请求`827948cf-87fe-48ac-bbdf-486eaf6f84c1`参数拒绝后为manual/费用未知，保留0.12元预留，不重发、不强制零结算；需取得原请求的可信供应商费用证据。
- 未接受：首页真实生成（本次3次供应商调用额度已用完）、其他11个模型、个人积分余额/充值/会员、生产部署、窄屏/读屏及真实在途历史重放。项目账本当前已结算0.24元、预留0.12元、可用0.64元，总预算1元。

- CI既有阻断：父提交b03f9dee与4ab20c15均失败于Agent测试库缺`agent.graph_checkpoint`（TestPostgresCheckpointerSaveAndResume）；4ab20c15前端CI通过。本次不扩范围修改Agent或CI，不将本地定向通过描述为主干全绿。

### 6.5 其他模型配置与计费补齐（SRC-AI-38）

用户选择先完成配置和代码、暂不收费调用。9个有现存凭据的模型已追加配置v2、11份审核价格；Pro图片、两款豆包文本及四款Seedance共7个待验证发布已保存。DeepSeek分别留存高峰/空闲价，分时结算依据未齐不创建发布；两个OpenAI模型仍缺Key。所有新增模型保持停用，Flash原发布不变。

- 已完成：缓存用量分价、Seedance输出Token报价/结算及发布门禁、Pro参考附加的发布/报价/冻结计数结算、试运行积分明细传递；受影响catalog/operation及managed/providerhttp整包Race通过，前端7项定向、类型和lint通过。完整验证见本轮记录（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-03-beeftv-adoption/remaining-models-validation.md`）及安全配置快照（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-03-beeftv-adoption/remaining-model-configurations.json`）。
- 免费页面验证：Pro 2K报价0.60元、预计6积分；仅quoted，不确认生成。当前视频最低样例高于原项目0.64元可用预算，保留既有页面门禁，不增加预算。
- 全仓限制：首次后端全仓命令受测试环境条件阻断，后续受影响整包已用完整隔离环境通过；前端全量2069通过/1失败，失败指向并行探索搜索存储策略，保留该改动。
- 未接受：9个模型的真实生成/归档/结算与启用、DeepSeek自动分时/节假日结算、输入视频阶梯费率/最低用量、OpenAI凭据与价格、个人积分钱包。本轮新增供应商调用0次、派发总数仍3；原manual的0.12元预留不变。

### 6.6 DeepSeek分时结算（SRC-AI-39）

2026-11-04：按用户要求复用原计费链路，禁止收费调用；实现及隔离验收通过后定向提交main并推送，其他改动保留。

- 实现：在原Token价格内追加固定年度分时标识；高峰预留、按原调用区间判断空闲半价，缓存/未命中/输出合并后一次取整。原私有结果留存received_at，恢复沿原时间和价格；缺证据/跨费率边界转原待核验，不零结算或释放预留。未核验年度禁止新派发，不新增服务、表或定时任务。
- 配置：两款DeepSeek均追加价格v4及待验证draft，复用连接配置和模型v2，保持disabled；旧v2高峰/v3空闲价不改写。安全快照见配置记录（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-04-deepseek-time-pricing/configurations.json`）。
- 工程接受：operation/catalog/billing/managed/providerhttp五包最终Race通过，格式、vet、lint及可达漏洞检查通过；全仓首次环境失败及分包复跑证据均保留。
- 验证证据与限制：本轮验证（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-04-deepseek-time-pricing/validation.md`）。本轮供应商调用0次，历史派发仍3；首轮Flash账本及原0.12元未知预留不变。
- 未接受：DeepSeek真实供应商生成/归档/账单对账与启用、跨费率边界的供应商计费时刻确认、2027及以后节假日规则。未经新的明确收费授权不试运行；新年度须审核规则后追加版本，不自动延用2026规则。

### 2026-11-03 SRC-AI-37 界面增量

报价、服务接入和模型管理按即梦实页重新分层，公共Select长文本与移动端样式同步修复；Figma先行，代码落盘未提交。执行证据见Plan29/30本轮回执及[研究记录](workspace/content/research/2026-11-03-即梦报价与模型接入交互拆解.md)。本轮未执行收费调用、数据库迁移或新价格发布；T-AI-21/22/23/25既有未接受项与合同依赖保留。

### 2026-11-03 FR-AST-09 删除反馈精简

生成历史删除回执改为全局Sonner Toast，全部成功退出批量，部分失败保留选择与原请求恢复；主体、参考素材、音色、账号和项目/画布删除同步。Figma公共提示与删除成功页已更新并回读；真实资产页与合成删除浏览器检查完成，实现与验证完成，Git提交以仓库记录为准。工程验证与边界见[Plan22本轮回执](workspace/content/plan/22-资产对齐即梦实施计划.md#2026-11-03-ac-ast-09-删除toast执行)及仓库外output/delete-toast-2026-11-03。

### 2026-11-03 CR-076 顶栏尺寸

公共顶栏工具组与账号组已统一桌面36px/手机50px，手机保留44px触点，访客状态同步。Figma先行，1512/1024/390/320实页与批量切换检查完成；实现与验证完成，Git提交以仓库记录为准。来源与证据见设计29§7.16、PRD16 CR-076及Plan25对应回执。

### 2026-11-08 CR-077 首页独立全局操作层

已按更新后的Figma落实桌面首页紧凑操作组，top/right=20px、组高36px；正文全高且y=0，首页自主留白68px。手机64px导航和44px触点、普通页面工具槽及跨页账号节点保留。agent-browser在独立1130开发端口完成15项实页检查，覆盖五个断点、滚动、折叠、主题、模式、弹层叠放和焦点恢复；右上区axe检查0违规。28项定向回归、类型检查、ESLint与最终生产构建通过。本轮格式检查通过，全量格式检查有3项既有问题。全量2214项通过、2项5秒超时；2项在同一超时标准下单独复测通过，保留首次失败记录。用户重新登录后，正式1122地址完成11项真实登录态检查，覆盖账号焦点、积分弹层、跨页节点及手机积分备用入口。隔离快照仅叠加本轮6个前端文件，已重建前端并通过健康检查；织光后端、Agent和Canvas容器ID未变。来源为需求16 CR-077、设计29§7.17、PRD16和Plan25；本轮未提交推送，原29项并行修改保留。

### 2026-11-03 探索搜索一致性纠正

EX-REQ-08：确认现有CommandDialog与即梦探索不符。已按设计28/PRD15/Plan24落实行内搜索、历史、显式提交、URL结果类型和窄屏适配；新增Figma搜索展开/历史稿3426:74255。15项定向测试、lint、类型和构建通过，浏览器核验没有单独留在知识库中的快照。完整一致性仍未接受：T04公开目录/查询合同、T11真实三类搜索和联想词、T06卡片/详情/复用、T02/03全状态设计仍需补齐。本轮未提交推送；其他任务修改保留。

### 2026-11-03 探索shadcn组件收敛

EX-REQ-09已落实：搜索采用Field和InputGroup，历史采用PopoverHeader及PopoverTitle，一级采用Tabs line、分隔采用Separator。移除搜索独立外观CSS与分类颜色、字重和圆角覆盖。保留容器响应式布局与搜索行为。15项定向回归、lint、类型和构建通过；浏览器证据见探索搜索验证的shadcn补充。真实公开搜索服务缺口保持未完成，本轮未提交推送。

EX-REQ-10核查完成：侧栏已采用官方Sidebar体系并继续复用；共享顶栏简短未开放提示改用Sonner，正文Empty、登录和决策确认Dialog保留。38项定向回归及生产构建通过，设计09规则与Figma搜索稿已同步；未提交推送。

## 2026-11-03 连线显示纠错

已修复命令式拖动更新SVG路径但流光沿旧网格的问题，取消恢复也跟随实际路径；取消仅因关联节点active而触发的持续高亮。5文件20项、全局tsc、4文件ESLint通过，原生Chrome已核对用户原画布选中节点后保持灰线。生成中拖动的GPU逐帧验收尚未执行，不冒称新一轮源端全量标定。设计/原始需求/PRD已同步，增量未提交；证据见本轮验证（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-03-canvas-line-fix/validation.md`）。

## 2026-11-04 连线提交与实页复核

上一轮限定15文件已提交ca56fa06并推main。前端CI成功；后端缺agent.graph_checkpoint表，父提交已有同一失败。当前即梦实页与新bundle更正上轮降灰判断：恢复endpoint-active，节点拖动/视图移动/框选暂停流光，hover200ms、滚轮150ms+RAF、缩放基线1/k，13渐变节点精度对齐。6文件24项、全局tsc、范围lint与指定本地origin后的生产构建通过；实页三档缩放及拖动暂停/恢复已验，原Figma动效板3159:54491同步规则与精确渐变。后续增量未再次提交。按用户授权，通过既有领域命令仅补足Seedream报价2积分所需的120000微元预算，账本与审计各1条；仅确认任务0d7fd96a一次，随后provider_not_submitted失败，预留全额释放、实际扣费0、剩余可用120000微元。失败后取消选中，两边恢复默认opacity0.16且流光暂停，无高亮残留；供应商生成中/成功及其拖动缩放组合未通过。下一步先查明供应商未提交原因，再取得单次生成验收条件；本次不再提交新任务，不签全量一致。结束时共享main已由并行billing推进至4b1909f9且远端相同，本任务ca56fa06仍在历史中。见复核记录（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-04-connection-review/validation.md`）。

### 2026-11-04 复核交付与供应商输入修复

限定34文件已提交e7e376b4并推送main，提交前22项定向前端测试通过；CI41751714383前端成功，后端仍为既有agent.graph_checkpoint缺表失败。随后按用户“仅修复，暂不再生成”排查：当前cos-main/COS主路由的真实HeadBucket成功，原引用图片则保存于legacy-main/MinIO，5442字节、回环HTTP签名；Temporal原activity明确invalid_provider_request。已修复该私有图片传输和安全分类保留，公网COS URL不改，未迁移旧资源。8组定向Race、隔离PG调用摘要/幂等、原49事件回放、goimports/vet及后端构建通过，govulncheck无可达漏洞；golangci-lint未安装。Figma原板与设计/PRD/原需求/Plan已同步。只重启本地后端载入修复，readyz正常、运行二进制与已验证构建SHA一致；原失败状态、1条调用和预算预留/结算0保持不变。修复增量未提交，未发起新生成、未加预算。下一步仍需获得真实任务验收条件后核对供应商生成中/完成及其拖动缩放组合；同项目COS图片完整读取、其它本地多模态引用另验，不以本轮图片修复签全量一致。见根因与证据（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-04-provider-reference/validation.md`）。

<a id="pi-durable-migration"></a>

## 2026-11-04 独立 agent 与数据库任务迁移

来源：需求19 SRC-AGH-10～15；唯一设计→PRD→Plan：设计32 → PRD19 → Plan28。2026-11-05工程终验完成；产品与现有部署切换分开判定。完整验证记录（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-04-agent-service/validation.md`）、机器结果（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-04-agent-service/checks.json`）、逐项工作区路径（Git 5c785a12 的 `workspace/content/research/evidence/2026-11-04-agent-service/working-tree-paths.tsv`）。

| 工作                         | 当前状态                                                                                                                                                                                                | 接受依据                   |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------- |
| 独立包与 PostgreSQL Storage  | Node24.22.2、TypeScript6、Pi1.5.0、Fastify5、TypeBox、pg与固定lockfile；24文件134项通过、0跳过，含官方Storage与真实SIGKILL                                                                              | Task1/2；AC-AH-02/13/15/20 |
| 官方 Harness、控制与专业协作 | owned子会话、计划/预算/Skill/受控SDK、压缩/图像、可靠控制及故障恢复；原租约/原超时独立重跑全Agent套件通过                                                                                               | Task3/6/7；AC-AH-21/22     |
| Go业务边界及创作工具         | 完整Go Race覆盖授权、回执、Yjs、媒体、剧本、发布、图片与导出；当前生产Go↔Node边界另作最终定向Race通过                                                                                                   | Task4/5；AC-AH-04/05/18    |
| 普通任务与旧实现删除         | PG任务/租约/取消/恢复和FFmpeg验证通过；无Temporal SDK/连接/命令/旧Go图/旧阶段接口兼容                                                                                                                   | Task8～11；AC-AH-14/17     |
| 当前前端合同                 | 当前会话工作台、rich draft与消息分页；295文件2099项通过，strict/lint/格式/官方API生成核对/Next生产构建全部通过                                                                                          | Task12；AC-AH-12/19        |
| Compose与当前SQL             | 单一四服务Compose；Agent镜像非root/只读/限额，实际Harness完成、health/ready200、SIGTERM0/租约0；空库8运行/8业务表权限及6份冻结SQL一致性通过                                                             | Task13；AC-AH-18～20       |
| 既有库离线切换               | [清单与脚本](https://github.com/StephenQiu30/glowloom/blob/4e41135a5fb5cf7c0ab53d617e599771d67ad607/backend/db/agent_cutover.md)交付；历史隔离演练保留其日期与范围；本轮未在现有业务库执行DDL或实际切换 | Task13/14                  |
| 文档与历史错误描述           | 设计、计划、PROJECT、BACKLOG及验证记录更新为当前合同与终验结果；原始需求/有效历史证据保留，不再作为当前失败状态                                                                                         | Task14                     |
| 全量联合检查                 | Go176包完成，4478测试/子测试通过、0失败；57项既有外部条件测试跳过并逐项列明；Go build/vet/lint/格式均通过，govulncheck0可达/0导入包漏洞                                                                 | Task14                     |
| 现有部署与真实创作接受       | 真实供应商、费用清结、长剧本至可编辑成片、完整浏览器与线上CI未由fixture代替；Docker Desktop恢复引发已有restart策略自动运行容器，未改其配置，详见环境记录                                                | AC-AH-01～22               |

工程终验对应提交前快照；后续提交、推送及CI状态以实际Git交付为准。共享并行修改按明确提交范围保留，没有向其他聊天发送消息。工程测试终验已完成；生产切换与真实创作接受继续按上述边界管理。

最后合并回归：15个相关前端文件138项全部通过，覆盖本轮随后补充的版本复核、目录版本校验与Agent紧凑菜单。全量2206项与末轮138项为不同范围，不能相加作为全量总数。

## 多媒体与风格的远程门禁记录（2026-11-08）

实现 `61cc4eaf` 与专用CI `7776c562` 已推main。`7776c562`远程前端2209项、Agent151项及独立多媒体/风格步骤通过；后端全仓27条失败名称与b969bd3b完全一致，仍不标为全仓通过。此后补链接BFF五分钟处理预算（仅该POST 310秒），75项BFF回归与类型/lint通过，待新提交的远程回执。存量DB、1122新运行版、真实平台成功样本与逐页像素验收边界继续保留。

## 真实运行版重启与继续验收（2026-11-08）

313097be已部署本地四服务，现有glowloom数据库已完整备份并完成精确增量结构升级，原项目/媒资/用户/风格计数不变。登录态实际验证风格创建/回读、20/2000边界、真实四图上传与封面保存，及MP4/TXT/DOCX上传；没有发起付费生成。原338项及28项记录保持原样，本轮另建逐项运行版证据。

分析风格和短剧项目参考复制发现真实BFF404，854e83be已补齐转移全生命周期封闭代理与正确领域错误，新增两个先红后绿回归；312文件2212项全量前端、全前端ESLint、改动文件Prettier与TypeScript通过。58ae0e38直接执行已安装Next入口，避免构建阶段pnpm重复联网，两提交已推送main且远端一致。完整生产构建尚未成功：首次资源压力与重试超时均保留失败/中断事实，不计为通过。

Docker引擎在停止本任务构建后仍多次阻塞，业务请求读取超时，不能继续把这些页面计为接受。已请求允许重启Docker Desktop，因其影响其他本地项目而未擅自执行。恢复后优先在原项目/原复制命令复测三类参考承接，再复测分析四图与风格草稿、素材记录、手机、未签收页面及Figma逐像素差异。上一313097be的CI已终结：前端/Agent通过，后端失败；新提交CI另追踪。详细证据见[Plan33](workspace/content/plan/33-短剧工作室实施计划.md#2026-11-08-真实运行版重启与验收证据)。

### Docker恢复与复测补记

Docker Desktop获用户授权后已重启，原运行30个名称均已恢复且无unhealthy，原停用容器未主动启动。前端实际部署main-b7bf9676，后端/Agent/Canvas保持313097be，健康HTTP200。转移旧结构增量升级前完成完整备份和隔离旧结构验证；实际8项受保护计数不变。原命令的三类参考、风格分析4图、手机同构建风格4图均复制成功并进入待发送草稿，11个独立目标身份和29个物理对象verified，未发起付费调用。

已逐项扩展[52项运行版记录](workspace/content/research/2026-11-08-运行版验收记录.csv)：AI工具16个入口均可打开但处理服务仍未接入，插件无安装版本；资产筛选/批量状态、短剧阶段导航、主体真实创建回读已补核。跨部署整页刷新会丢失内存交接输入，作为未通过保留，不以同构建通过替代。存量其他3个缺表、目录导出4缺列、真实生成/解析/分析采用、复杂项目编辑与全站Figma像素仍需后续独立核验。

f075补齐文档角色中文；b7bf以独立文档能力接通对话内补充文档，普通生成继续拒绝文档。全量2214、全前端ESLint、类型、改动格式和生产构建通过。Mac当前再次锁屏，已请求解锁，下一步先对新文档菜单、真实上传及中文角色完成可见实页复测，再继续未验页面；这些尚未标为运行版通过。旧b577 CI前端/Agent通过、后端失败；新CI另追踪。

### 解锁后继续验收与文档确认修复

用户已解锁，以上锁屏暂停状态由本次可见实页复测取代。文档上传TXT/DOCX均201，中文角色正确；继续发现共享选择器禁用文档、误标音频，以及确认误用画布preview连续404。b08071c9与c0dbdcb6分别修复引用能力开关和目录详情核验，真实选择确认200并进入对话草稿；画布文档仍只读。自有DOCX真实导入202后显示已导入70字，整页刷新后完整正文与末尾标记仍在；复制TXT/DOCX字节数及SHA256与原件一致。

[运行清单](workspace/content/research/2026-11-08-运行版验收记录.csv)扩展为72条：41通过、23部分通过、7条修复前失败及1跨部署未通过。自建画布文本新建/编辑/保存确认/刷新、缩放、搜索定位、操作模式及工具箱展开已分别记录；不把工具入口当作执行完成。前端c0dbdcb6及其他三服务313097be均healthy，前端healthz200。当前全量2216项、ESLint、类型、改动格式和生产构建通过；b080的CI42434331311前端/Agent通过，后端失败。

下一步仍按未验项推进AI工具真实处理、会员/作品发布、复杂分镜/时间线/转绘、跨部署草稿耐久性和Figma逐像素核对。没有新增全站1:1完成结论，没有发起付费生成。存量其他结构缺口与既有CI失败保持独立边界。

3D导演台补核后清单为75条：43通过、24部分通过、7条历史失败及1跨部署未通过。目录真实建画布/节点、WebGL场景、预设角色放置改名、保存确认及刷新回读已有截图与快照；拍摄/动态导出和逐像素仍未签收。三维录屏编码失败照实保留，不计有效视频。

### 2026-11-09 计费迁移与真实 Ark 验收

- 用户已授权当前业务库迁移和收费验证，真实供应商总费用不超过 ¥20；本轮只验已有 Ark，语音凭据未配置，TTS／Seed-Audio 不进行真实调用。
- 完整备份、旧库副本增量迁移及幂等重放通过；实际库迁移和分区维护完成，迁移前后原 5 条钱包流水、9,600,000 微元账面余额及历史操作事实保持。该旧批次已过期，免费报价正确阻止消费，不能把账面余额当成可消费余额。
- 异步冻结确认、完整资金派发、分区 Outbox 幂等修复通过 Race 回归；正式 HTTP 装配补齐同一可用性准入。受限应用数据库账号已启用，本地新运行版健康。
- 积分服务现有 platform 三条 API 的 BFF 代理补齐，99 项代理回归、ESLint、TypeScript 和生产构建通过；实际运行版该路由返回正式未登录401。当前后端、前端、Agent及Canvas健康，后端重启计数0，writer没有维护连接。
- 真实Flash普通生成1张已结算2积分；Mini正式试用及启用同一文生视频版本后的普通生成各完成1段视频、各结算10积分，SAGA均已核销。图像和普通视频确认重放前后预留/submit/积分命令/钱包分录不变，每笔只有1次submit。已知采购用量复算合计¥1.987324，保守供应商预算占用¥16.25/¥20；未取得供应商账户账单，不宣称该数为账单扣款。
- 原Mini任务沿原TaskID恢复，修复显式恢复不能重开已停查询、恢复审计事件遗漏、失败只读轮询阻塞已有可信终态费用归集三处问题；相关隔离PG Race、幂等与反例回归通过。经用户授权，仅追加既有Clash的TOS域名DNS过滤规则，下载安全限制保持。
- 真实Lite试用返回供应商404，准确模型ID在官方目录中可见但推理权限未证实；保留原1积分冻结和unknown状态，未激活、未重发、未伪造零消费。下一步由Ark账户/端点权限或供应商账单证据核查原调用；语音真实验收继续等待凭据，不在本轮补调用。
- 最终13条流水与13个全局身份一致，原5条历史流水保留，逐行delta守恒、无负余额；本次100积分中22积分消费、1积分冻结、77积分于18:04:26到期不可消费，没有追加发放。费用未知项不自动释放。实际验证表面为生产application服务及真实worker/relay，不声明浏览器HTTP端到端通过。
- 证据目录：`/Users/stephenqiu/.codex/visualizations/2026/11/09/01a1bee1-0086-7082-9022-39fe77a650ce/billing-validation/`。用户已追加要求提交main并推送；Git回执以实际交付为准。

### 2026-11-12 3D导演台卡片拖动与聚焦

- 用户截图明确目标为静态导演台卡片；基于main `695c779d`工作树修复整区事件隔离，双击复用现有搜索定位。进入按钮与完整三维工作区保持独立交互，其他节点编辑/预览语义保持。
- 关联34文件230项、全前端314文件2245项全部通过；ESLint、Next typegen、TypeScript、本次5个源码/测试格式与本机生产构建通过。全目录格式仍有4个非本次修改文件的问题，未擅自整理；Docker内编译因资源压力取消，后以相同Next版本Linux基础镜像组装便携构建成功。
- 仅更新本地frontend，Build ID `BsL4W2Rxbr6aYoUX8PM8c`，四服务healthy。原登录画布验证拖动60px、撤销、双击准确居中、进入/返回、刷新原位置与“已保存”；没有新增收费调用。即梦居中保留缩放，本项目沿既有算法适配大小，差异已记录。
- REA静态Evidence、运行实测、截图及逐项工作区清单见 `/Users/stephenqiu/.codex/visualizations/2026/11/09/01a1bee1-0086-7082-9022-39fe77a650ce/canvas-drag-focus-20261112/validation.md`。用户已授权仅提交本轮修复并推送main；实际提交、推送与远程CI以交付回执为准，其他任务修改原样保留。

## 2026-11-12 资产和主体最新 Figma 还原验收

- 工作区已实现六类资源及合并入口、两个工作室首屏、主体类型保存与服务端筛选、桌面和手机布局。Seedream 5.5 未加入导航，历史 Figma 保留。
- 设计来源：[最新设计稿](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3713-110549)。沿用 Geist / Neutral / shadcn。
- 预览在 `http://localhost:1124/assets`、`/motion-studio`、`/marketing-studio`。1122 容器未发布或重启。私人编辑使用 5196 源码组件与合成回执验收，未写产品数据。
- 前端 eslint、next typegen、tsc、生产构建通过；全量 314 文件 / 2251 用例通过，最后布局专项 11 文件 / 122 用例通过。本次文件 Prettier 检查通过。
- 全量 Prettier 遗留 4 项：`.vitest/json/output.json`、`public/director/catalog/manifest.json`、`public/director/motions/quaternius-ual1-standard.json`、`src/components/canvas/timeline/timeline-model.ts`，未修改。
- 后端主体 race 62 用例通过，29 数据库集成用例因未配置独立 PostgreSQL 验收库跳过。goimports、go vet、golangci-lint 通过（0 issues）。旧无类型内容摘要兼容测试通过。
- govulncheck 未通过：现有 Go 1.26.10 标准库 GO-2026-6859，工具报告修复版本 1.26.11；本次未改工具链。
- 浏览器核对了 1460 桌面 / 390 手机、展示图加载与页面无横向溢出、末尾分类自动可见、类型筛选及保存、批量工具布局和切页后状态清理。
- 工作室生成、资产入口剪辑导出暂未开放，界面明确说明。本次未提交，未触发生成或付费调用。
- 旧 `/subjects` 已通过 HTTP 307 和浏览器验证直达 `/assets?view=subjects`，不受旧页面登录门控阻挡；最后配置变更后生产构建再次通过。5196 合成验收服务已停止，1124 前端预览保留；现有容器尚未接入本次新增的主体类型接口，不作为部署验收。浏览器原有登录态在预览端口可能出现校验提示，未清理用户登录信息。

本次未提交文件（逐项）：

- `DESIGN.md`
- `backend/internal/subject/adapter/http/query.go`
- `backend/internal/subject/adapter/postgres/query.go`
- `backend/internal/subject/application/contracts.go`
- `backend/internal/subject/application/service.go`
- `backend/internal/subject/domain/content.go`
- `frontend/next.config.ts`
- `frontend/src/api/typings.d.ts`
- `frontend/src/app/api/[...path]/route.ts`
- `frontend/src/app/assets/_components/asset-location.ts`
- `frontend/src/app/assets/_components/asset-query.ts`
- `frontend/src/app/assets/_components/asset-workspace.tsx`
- `frontend/src/app/assets/page.tsx`
- `frontend/src/app/globals.css`
- `frontend/src/app/subjects/page.tsx`
- `frontend/src/components/billing/credit-popover.tsx`
- `frontend/src/components/layout/application-layout.tsx`
- `frontend/src/components/layout/basic-layout.tsx`
- `frontend/src/components/layout/page-layout.tsx`
- `frontend/src/components/layout/product-account-actions.tsx`
- `frontend/src/components/layout/product-header-actions.tsx`
- `frontend/src/components/layout/product-header.tsx`
- `frontend/src/components/layout/product-navigation.tsx`
- `frontend/src/components/layout/product-sider.tsx`
- `frontend/src/components/layout/routes.ts`
- `frontend/src/components/layout/sidebar-nav-link.tsx`
- `frontend/src/components/subjects/unified-contracts.ts`
- `frontend/src/components/subjects/unified-editor.tsx`
- `frontend/src/components/subjects/unified-library.tsx`
- `frontend/src/components/subjects/unified-toolbar.tsx`
- `frontend/src/components/subjects/use-subject-list.ts`
- `frontend/tests/browser/assets.tsx`
- `frontend/tests/unit/app/api/[...path]/route.test.ts`
- `frontend/tests/unit/app/assets/_components/asset-location.test.ts`
- `frontend/tests/unit/app/assets/_components/asset-workspace.test.tsx`
- `frontend/tests/unit/components/layout/basic-layout.test.tsx`
- `frontend/tests/unit/components/subjects/unified-editor.test.tsx`
- `frontend/tests/unit/components/subjects/use-subject-list.test.ts`
- `backend/tests/subject/type_test.go`
- `frontend/public/design/reference/studios/`
- `frontend/src/app/marketing-studio/`
- `frontend/src/app/motion-studio/`
- `frontend/src/components/studios/`

其他现存变更（保留）：

- `workspace/content/00-文档索引.md`
- `workspace/content/plan/28-完整创作Agent与Skill多Agent实施计划.md`
- `workspace/content/research/00-调研导航.md`
- `workspace/content/requirement/25-多模态创作与Agent编排需求.md`
- `workspace/content/research/2026-11-12-即梦多模态创作与Agent服务拆解.md`
- `workspace/content/research/2026-11-12-短剧制作平台源码与发行包对照调研.md`
- `frontend/.vitest/`

## 2026-11-12 1122 实际镜像与 agent-browser 验收收口

- 用户允许连接当前 Chrome 登录会话。本轮实际验收 localhost:1122；原1124预览已停止，未将预览视为部署证据。
- 原1122仍运行旧前后端镜像，已更新现有两个应用容器。合并资产和主体、六分类、三个工作室导航及旧主体链接跳转已生效，Seedream 5.5入口移除。
- 修正登录态主体工具栏在纵向组合时继承居中对齐的问题：`items-stretch`；1460px下实测 x=284（侧栏240+正文44），与设计稿及卡片网格一致。手机工作室72px参考方块内边距4px，避免四字标签不必要换行。
- 补齐营销示例、工具及已选参考容器的ARIA分组角色；主体/动效和最终营销桌面/手机引用状态的axe WCAG2 A/AA检查均0违规、0待定。
- 浏览器：游客40/40布局检查；最终视觉版本已登录40/40，覆盖1460/1280/1024/390/320各六分类+两个工作室。ARIA小修后针对相关营销桌面/手机状态复测。六类真实私人内容另用页面内切换验证，不能仅凭账号入口或布局成功视为数据读取成功。
- 原蓝色主体640×360预览读取成功；原主体关闭不保存保持原值。复用已有图片创建临时主体，验证刷新、产品→风格持久化与筛选、收藏、搜索清除、单项批量及390px弹窗。临时主体已删除，原主体与素材保留。两个工作室风格/参数/模板筛选和真实参考图片引用通过；生成和营销工具仍未开放，未发起付费调用。
- 容器构建切换受支持的Webpack worker/构建内存优化；探索CSS Module选择器关联既有局部导航类，修正Webpack pure selector编译问题。完整容器构建曾通过；后续Docker约8GB内存和多个项目争用导致重型构建中断、引擎超时。未停止其他项目，最终主机完整生产构建/TypeScript/42页生成通过，复用已验证Linux依赖封装运行镜像。低内存并行构建环境限制仍保留。
- 高频全页刷新触发既有每会话12次/分钟刷新保护；未更改限流，待窗口恢复后通过正常页面内切换补验真实读取。既有画布创建恢复提示只关闭，没有确认其提交。
- 相关单测两组各54通过，最终主体/布局54通过；最终生产构建、相关ESLint、Prettier、diff检查通过。前阶段全量2251前端测试与主体62后端测试通过；29个既有集成跳过及GO-2026-6859问题不计通过。
- 最终前端镜像：`sha256:758890a0c6281b819a33128e0c3e3afff0ec0d0a4d287a68267fa51bb282f4f1`；后端：`sha256:aec92358a37b5351915bc892dadef0746cf168bc7f508f344b19242050be0b86`。前后端healthy；前端重启0、后端在停止重型构建后重启计数稳定11。
- 完整验收报告：完整验收报告（本机验收资料，未随仓库归档）。包含前后截图、矩阵、真实内容读取、axe报告和限制。
- Git：未创建分支、未提交、未推送、未发布远程；历史Figma稿保留。下列设计、主体领域/查询、API派生类型、前端页面/导航/工作室、构建及相关测试文件仍作为本任务工作区修改保留；`frontend/.vitest/`为已有验收产物，未纳入交付提交，也未擅自清理。

本次最终检查的逐项残留路径：

- `BACKLOG.md`
- `DESIGN.md`
- `backend/internal/subject/adapter/http/query.go`
- `backend/internal/subject/adapter/postgres/query.go`
- `backend/internal/subject/application/contracts.go`
- `backend/internal/subject/application/service.go`
- `backend/internal/subject/domain/content.go`
- `frontend/Dockerfile`
- `frontend/next.config.ts`
- `frontend/src/api/typings.d.ts`
- `frontend/src/app/api/[...path]/route.ts`
- `frontend/src/app/assets/_components/asset-location.ts`
- `frontend/src/app/assets/_components/asset-query.ts`
- `frontend/src/app/assets/_components/asset-workspace.tsx`
- `frontend/src/app/assets/page.tsx`
- `frontend/src/app/explore/_components/explore.module.css`
- `frontend/src/app/globals.css`
- `frontend/src/app/subjects/page.tsx`
- `frontend/src/components/billing/credit-popover.tsx`
- `frontend/src/components/layout/application-layout.tsx`
- `frontend/src/components/layout/basic-layout.tsx`
- `frontend/src/components/layout/page-layout.tsx`
- `frontend/src/components/layout/product-account-actions.tsx`
- `frontend/src/components/layout/product-header-actions.tsx`
- `frontend/src/components/layout/product-header.tsx`
- `frontend/src/components/layout/product-navigation.tsx`
- `frontend/src/components/layout/product-sider.tsx`
- `frontend/src/components/layout/routes.ts`
- `frontend/src/components/layout/sidebar-nav-link.tsx`
- `frontend/src/components/subjects/unified-contracts.ts`
- `frontend/src/components/subjects/unified-editor.tsx`
- `frontend/src/components/subjects/unified-library.tsx`
- `frontend/src/components/subjects/unified-toolbar.tsx`
- `frontend/src/components/subjects/use-subject-list.ts`
- `frontend/tests/browser/assets.tsx`
- `frontend/tests/unit/app/api/[...path]/route.test.ts`
- `frontend/tests/unit/app/assets/_components/asset-location.test.ts`
- `frontend/tests/unit/app/assets/_components/asset-workspace.test.tsx`
- `frontend/tests/unit/components/layout/basic-layout.test.tsx`
- `frontend/tests/unit/components/subjects/unified-editor.test.tsx`
- `frontend/tests/unit/components/subjects/use-subject-list.test.ts`
- `backend/tests/subject/type_test.go`
- `frontend/.vitest/`（现存验收缓存，保留）
- `frontend/public/design/reference/studios/`
- `frontend/src/app/marketing-studio/`
- `frontend/src/app/motion-studio/`
- `frontend/src/components/studios/`

## 2026-11-12 确认设计后的公共样式统一与提交收口

- 按用户确认的最新Figma整页设计复用公共透明顶栏、首页账号工具栏、资源网格、卡片和文字规格。批量操作留在正文，两个工作室复用现有资源卡片按钮。
- 实际1122通过agent-browser登录态40/40、游客16/16及交互10/10；临时主体已清理，原内容保留。桌面和手机主体WCAG A/AA检查、浏览器运行与控制台均零错误。
- 最终前端314文件2253项单测、ESLint、修改文件Prettier、TypeScript及生产构建通过；4个既有全仓格式问题保留。证据见验收报告（本机验收资料，未随仓库归档）。
- 用户授权仅提交并推送本次相关修改至main。交付包含资源合并、两个工作室、主体分类所需接口/派生类型/测试；其他Agent和短剧调研文档、缓存、本地构建内存调优及未使用的余额展示扩展不纳入提交。历史Figma保留，未触发付费生成。

## 2026-11-12 Agent 费用、制作义务、恢复与单镜联合交付

- 用户 `SRC-AGH-23` 授权本地实施 Plan28 R1/R2/R3 与 Plan33 D1，明确不提交推送。先在隔离 worktree 实施，后按70个文件白名单合回当前main；生成类型保留并行Subject字段，未覆盖并行文档、样式、构建和费用展示修改，暂存区保持不变。
- 根费用复用原call/operation/冻结/结算，区分承诺、实际收费、退款和未知；不再把媒体报价永久重复计费。归集失败、未知和超额阻止新收费，原免费恢复保留。分支货币划拨仍随R4真实消费者实现，没有空allocation接口。
- 制作目标扩展现有Admission/run_authorization，沿稳定request/root先在Go落受理再转Node；Go目标覆盖manifest，冻结chat/production、required及可选来源/规格。完成由当前root正式批准/create、原成片及工程审核事实裁决，不借用别的root旧成功或空集合。
- 冻结集中ExecutionPolicy和完整src/锁文件定义摘要，恢复泵有逐项退避/停车/诊断。原operation完成后按正式交付进度幂等接续，等待审核不追加模型，无进展有界停车；断网保持原ID/绝对截止。临时停车可走原pause/resume，幂等控制只清Run退避，不清预算、unknown、步数或接续历史；关闭未证实排空不早放租约。
- 浏览器单镜入口复用ConversationWorkspace并冻结原画布；会话URL支持刷新回读。成片/工程复用正式导出预览、审核和下载，逐份明确确认，原版本/SHA及下载字节校验，审核丢回应保留原key、新版本重新检查。转绘切换继续经过原关闭guard。
- 实际联合测试发现并修复公开Start丢SessionID/CredentialEpoch，以及导出审核升级用户写锁与项目授权形成死锁；均有RED/GREEN，没有加业务重试掩盖。当前main完整跨Go/独立编译Node/正式工具/PG/Yjs/原worker/FFmpeg/FCPXML的race用例22.34秒通过：8模型回合、1视频POST、2审核导出、9原结算900000micros；重复/重启没有重买或重扣。金额仅隔离测试账本。
- 本次合并快照在共享main验证：Agent 31文件184项、前端316文件2262项全部通过；前端最终生产构建、类型、API重生成和lint通过，Agent类型/lint/API/格式通过。Go受影响18个顶层费用/目标/公开入口/交付race及子场景无skip，原模型网关29项PG race、锁/公开认证回归通过；受影响Go单测、vet、goimports、lint 0 issues通过，govulncheck 0可调用/0导入包漏洞，另有1个未被调用的required模块告警。前两次高负载全量尝试超时/中断留证，最终限并发和15秒用例预算后全部断言通过。
- Chromium用独立HTTP fixture复用真实后台测试产物，验证原root刷新、实际视频解码、工程下载检查、两份人工确认和最终下载，桌面/390px无横向溢出，下载SHA与后台一致。它不是生产登录与真实供应商浏览器贯通，未冒充真人作品接受或目标剪辑器导入。
- 新完整schema只在新建隔离库核验；现有业务库未DDL，未部署或重启正式服务。发布前仍需按既有离线流程精确处理`operation_commitment`、`run_authorization.goal_contract`和`agent.recovery_backoff`，核对旧定义运行及原账本后再单版本切换；不得运行全schema覆盖存量。
- 详细证据见[费用目标与单镜闭环验证](workspace/content/research/2026-11-12-Agent费用目标与单镜闭环验证.md)。后续依赖仍为R4真实分支并行、R5 Skill自动选择、R6正式MCP及D2～D5正式制作身份/局部重做/整集/真实作品接受；本切片不把这些标为完成。未发起真实付费调用，未提交或推送。

- 本次后续交付授权 SRC-AGH-24：用户要求将本次代码提交到main并推送；纳入实现、验证记录及其需求/设计/计划/源码附录闭环，其他并行修改继续保留。Git回执与远程CI分别核对，不把本地测试当作CI终态。


## provider-workbench-unified-20261113

来源：[SRC-AI-50](workspace/content/requirement/04-AI平台接入与预设管理需求.md#src-ai-50-接入工作台合并与模型弹窗2026-11-13) → [设计07](workspace/content/design/07-AI平台接入市场调研与架构设计.md#src-ai-50-统一接入工作台) → [PRD03](workspace/content/prd/03-AI平台接入与预设管理.md#src-ai-50-统一页面与模型接入弹窗) → [Plan10](workspace/content/plan/10-AI平台接入与预设管理实施计划.md#src-ai-50-统一接入工作台实施)。

本地完成：Figma 8 个视图、统一服务连接与供应商预设标签、预设预选接入、同一 Dialog 接入模型、焦点及迟到回执回归。桌面／390px／320px 浏览器检查了现有服务数据读取、单一弹窗、取消返回及手机操作菜单；320px 预设表格宽度 286px，模型弹窗宽度 288px，页面无横向溢出。未修改凭据或发起真实供应商生成。

交付边界：新代码预览为 `http://localhost:3117/providers`；`1122` 现有容器尚未重建。界面与文档完成不代表生产发布或新增供应商验收。原有价格、发布及恢复规则继续适用。


## provider-custom-config-logo-20261114

来源：[SRC-AI-51](workspace/content/requirement/04-AI平台接入与预设管理需求.md#src-ai-51-自定义供应商模型配置与-logo2026-11-14) → [设计07](workspace/content/design/07-AI平台接入市场调研与架构设计.md#src-ai-51-自定义配置与供应商-logo) → [PRD03](workspace/content/prd/03-AI平台接入与预设管理.md#src-ai-51-自定义供应商与模型接入) → [Plan10](workspace/content/plan/10-AI平台接入与预设管理实施计划.md#src-ai-51-自定义接入与-logo-实施安排)。

设计阶段记录：支持第三方网关、CC Switch 式完整供应商/模型配置、Logo 自定义上传及模型沿用。当时用户要求先更正文档与设计，暂不编码。原 SRC-AI-50 的本地预览与160项回归只证明旧范围，不能作为新网关、Logo 或新表单的实现证据。

设计阶段交付：原始需求、设计、PRD、执行计划和[Figma33](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=4018-122220)共8个设计视图；已移除当前设计区的独立手工模型入口，并将旧两步向导标为历史。后续按Plan10先补合同及失败用例，再开展实现。本轮未改业务代码、重建服务、提交或发布，也未进行真实上传或生成；其他并行工作保留。


Magpie 参考已完成本机只读查看，记录在[实测文档](workspace/content/research/2026-11-14-Magpie供应商与模型接入实测.md)。采用供应商内管理模型、图标入口与高级项展开；目录读取明确触发，不照搬独立ID输入、自动读取或未支持的协议。未保存Magpie配置或发起模型检测；不改变本轮暂不编码的范围。


SRC-AI-51 最新确认：用户要求先更正分析再落实代码，实施授权已恢复。范围已收敛为最少供应商/模型字段、供应商Logo、现有OpenAI兼容网关与既有草稿/回执。此前“暂不编码”是历史阶段约束；进度与真实验证在本条后续更新。

本地实施：供应商采用预设／自定义统一表单，先保存名称、协议、地址、凭据及可选Logo，再从同一详情接入模型。模型只填名称、标识、能力；高级参数沿用既有配置页。Logo由供应商维护，管理模型与创作目录均返回供应商引用，不依赖当前供应商分页；移除独立手填ID快捷入口。网关首期为公开HTTPS的OpenAI兼容协议，自定义草稿不吸收官方直连价格或验证。

验证：前端关联回归47文件316项通过；追加跨分页Logo回归后models-page 11项通过。最终TypeScript、范围ESLint/Prettier、生产构建和diff检查通过。实际隔离PostgreSQL的catalog全套Race、Logo/头像13项及PNG处理9项通过；网关传输Race、Go vet、范围lint及新增改动lint通过。全量catalog lint曾有17项既有／并行问题，未作为全仓通过记录。两份专用测试库已精确删除并复核零残留。

边界：浏览器为5197合成API，验证桌面／390px、上传失败重试与取消、Logo继承及供应商→模型闭环；未做真实对象存储与第三方供应商验收、未发起付费生成、未对业务库执行迁移、未重建1122／1123。原始失败、最终通过、迁移与剩余工作区清单见实施验证记录（本机验收资料，未随仓库归档）。本轮未提交或推送；并行画布、提示词、文档及缓存保留。

交付授权：用户要求本次相关代码定向提交并推送main，再重建本地服务和检查实际接入流程，其他并行修改保留。新增Go测试已归入backend/tests，公开API和真实本地DNS的16项Race、vet与新文件lint通过。数据库继续仅维护schema.sql完整定义；存量库只按核对后的离线DDL补充Logo字段与外键，不新增迁移体系、不重放空库SQL。构建使用提交快照，实际页面与服务验收结果另记。

首次交付aeaab67b已推送main，原有四个并行文件逐字节保留。提交快照的前后端镜像构建并启动健康，存量库只新增Logo列及外键，原4个供应商保留。实际1122上传发现BFF缺少新接口白名单；首轮CI的唯一前端失败同为POST admin/providers/custom返回404。已补齐自定义创建、Logo上传/读取/更新的封闭路由及流式传输，新增实测回归后79项代理测试、范围lint通过。修复后的服务和完整接入继续验收，不把首轮CI或404记为通过。

实际服务修正：1099e06f已补齐BFF，远程frontend和agent通过。真实自定义供应商及加密凭据已保存，随后发现新审计action未登记导致relay消费失败、all进程重启；已登记两个精确动作并验证原outbox消息可解析，新创建审计使用真实provider ID，幂等receipt仍保持原target。旧存量库storage_work_intent的CHECK仅追加avatar且保留原9种kind，配置及对象存储路由不变。实库UTC回执以完整JSON比较，专用空库Race 1项通过且0skip；审计/catalog关联Race 524项通过（无专用DB部分仍skip），vet、增量lint、govulncheck通过。手机实页同时补齐接入来源标签与Logo操作44px触控高度，供应商14文件93项回归通过。

后端CI另有主干已存在的旧canvas迁移文件违反单一schema/data合同，以及operation测试夹具的验证mode与最终配置不匹配；相关文件及并行清理未纳入本次接入提交，不放松派发guard或跳过测试。最终重建后继续核对审计实际消费、重启计数、Logo对象写读及两个模型草稿；验收用独立测试供应商，完成后精确清理。

实页后续：Logo已完成COS实际PUT/Stat、签名读取及192px图像解码，供应商绑定更新和模型A草稿已持久化。模型保存暴露旧model.preset.add审计动作遗漏，已对全部13个catalogMutation生产者做AST跨模块合同检查，精确补齐5个实际发出的空摘要动作，保留未知动作/敏感字段拒绝；原308事件只读解析通过，未更改offset或历史。全audit Race、vet及增量lint通过，继续重建后验证完整连续流程与消费追平。并行任务已独立提交规范清理18528c6e，保留其提交归属。

真实本地验收完成：自定义供应商与加密测试Key保存后，可连续接入两个命名模型草稿；Logo的COS上传、签名读取、192/512px解码、更换、清除与取消均通过，两个模型沿用当前Logo，执行配置ID保持不变。桌面1440px及手机390/320px没有页面横向溢出，接入来源及Logo操作均达到44px。原305与308审计事件均实际入库并标记消费，Kafka offset/end为313/313、lag0，修复后后台重启计数0。通过正式删除流程清理验收供应商及两个模型，测试Key已停用，原4连接保留；两张PNG已解绑，遵循原资源保留/回收机制，未强制删除对象。

本次使用无效测试Key验证配置和草稿闭环，没有进行第三方网关真实鉴权或收费生成。首轮CI、存量约束、代理及审计失败均保留证据；代码前端与Agent CI已通过，后端曾有既有operation模式证据夹具失败，最新CI状态单列。将本记录提交后，从干净主干快照完成最后切换；提交、镜像、实际截图、CI及全部并行遗留文件见本地交付验收回执（本机验收资料，未随仓库归档）。


后端CI专项修复（2026-11-14）：用户授权继续修复并提交main。44c13f2对应CI43551571923的backend Race最终失败于7个operation用例；文本／视频配置变更后仍保留旧图片模式验证记录，钱包夹具则缺少当前凭据记录。仅调整5份测试夹具，在最终配置后复用模式证据，钱包复用既有凭据；生产guard、业务断言、跳过条件和超时不变。隔离空库先复现DeepSeek失败，7个用例含子场景22项Race通过且0skip；Go1.26.11的vet、增量lint、gofmt/goimports与govulncheck通过。整包首轮16项因本地未设置COS失败，已补齐仓库内存COS测试配置继续验证；完整日志及后续整包／远程最终结果见后端CI验收回执（本机验收资料，未随仓库归档）。本次不改服务实现或重建部署，其他并行工作保留。
