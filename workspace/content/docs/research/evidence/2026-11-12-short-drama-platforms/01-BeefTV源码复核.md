# BeefTV 重新核查分报告：生产执行、Agent 边界与交付质量

采集：2026-11-12，源码仅静态检查，没有运行上游程序、测试或收费生成。源码根目录：`/tmp/glowloom-research-20261112-beef`。

## 1. 版本与调查口径

冻结 `glanderness/BeefTV` main 采样提交 **d339ab127daa6ca57125e50e9e022dcdeeff3a4b**，提交时间 **2026-11-12 14:25:19 +08:00**，合并 PR #490 `feat/ops-task-cancel-orders-writes`。这比旧调研的 888108a（11-11）更新，包含今天新增的取消、生成单读取及外部写入回执入口。通过 GitHub 插件分别取得 README、该提交元数据、固定 SHA 的 Durable 宿主和 task.cancel 核心源码；本地完整克隆用于沿调用链检查。没有把包依赖、注释或 README 宣称直接当成实际运行成功。

另用 GitHub 插件读取 `glanderness/BeefTV-DSH` 的 README，固定其 main 采样 SHA **5ffda0bac07b058382ab157c425dbd3018547233**；它是 DSH 插件分发/入口仓库，实际插件实现同时存在本体仓库 `plugins/deepseek-harness`。

证据层级：下文“存在/拒绝/调用”表示已追到源码；“测试覆盖”只表示找到测试及断言，未在本轮运行；“没有找到”只覆盖明确目录和符号范围。不能据此证明真实供应商、当前发行包、平台吞吐及成片质量已经验收。

## 2. 不应混为一套的三层执行系统

### 2.1 内置助手

`agent-host/server.mjs` 将画布会话交给 `durable-session-owner.mjs`；后者打开每会话 SQLite 存储的 Pi Durable Harness 并创建根 agent。[F01]

实际依赖锁在 `agent-host/package.json`：Pi Coding Agent / Durable / Pi AI / Chord 均 1.5.0。根 Agent 以模型、工具集、系统指令、思考级别及工作目录初始化。该装配默认将模型流失败自动重试和一般重试设为 0，工具执行为顺序；注释明确针对“请求可能已经收费”而不自动重发。它不等于永不恢复：业务工具可回放、读取原任务，模型失败与业务任务恢复是分开的。

在本轮检查的生产 `agent-host` 顶层 `.mjs`、`durable-extensions` 注册、operation bridge、Go operation registry 中，没有定位到内置的 spawn/fork/团队子 Agent 调度注册。`agent-host/eval/dsh-driver.mjs` 出现 `tool-subagent*` 名称是外部 DSH 评估驱动的过滤/采集内容，不能据此反证内置主 Agent 已有同样调度能力。**这不是“BeefTV 全产品没有子 Agent”的结论**：外部宿主可能提供它，第三方扩展也可能增加工具，本轮没有执行或审计全部上游宿主源码。

Skill 的可复用单元不是提示词名称：`skill-resource.mjs:9-70` 以 skillId/versionId/contentHash 作为 pin，核对服务返回资源的 pin、正文和包结构；单技能正文上限 256 KiB、合计 512 KiB，失败在模型请求前阻断。`active-skills.mjs:12-57` 在压缩后按 versionId + contentHash 检查正文是否仍可见，只给当前请求补回缺失正文，恢复函数按 contextWindow/5 的数值作为正文字节预算，超出时保留最近技能；这是字节预算，不是精确的 token 计数器。模型请求钩子重新校验活跃 pin，不能静默切换到更新技能版本。本轮没有把 Skill 正文提到的角色分工等同于已调度多个独立 Agent。

### 2.2 统一 Go operation 和正式任务

`operations.Registry` 定义操作、读写、范围、回放安全及助手模式。HTTP `/ops`、CLI/MCP 与内置助手从它派生能力，不是三套独立业务实现。[F02]

operation bridge 将 descriptor 转工具，以会话身份和真实 toolCallId 派生写入幂等键；拒绝缺失调用 ID。`canvas.task.bind` 进一步固定为 task + node + outputIndex 身份，自动任务交付与模型再次绑定共享去重键。相同操作标识配不同载荷会冲突。局部画布写入能接续本轮自己的 revision 链，但整文档替换不自动变基；发生用户意图或现场变化时暂停旧写入。

`backend/internal/operations/registry.go:192-223` 先检查调用者和权限，再做回放，防止只读客户端、越界助手用旧 receipt 获得结果；`ExternalOnly` 在后端实际拒绝 `CallerAssistant`，不只是模型工具列表里隐藏。

### 2.3 外部 MCP / DSH 正式路径

CLI `mcp serve` 由 `/ops` 能力目录创建 MCP 工具，写入要求调用者提供稳定 operationId；默认排除 conversation、skill.publish 与 skill.script.run，底层业务可经 `business.list` / `business.call` 访问。[F03]

**今天新增的 `task.cancel`、`canvas.generation.orders`、`canvas.external.writes` / undo 标为 ExternalOnly**。它们的消费者是已授权外部调用者（HTTP operation API、CLI/MCP）而非内置 host。HTTP 专门列出了 orders/approve/external-writes/undo 端点；同一后端 registry 拒绝内置助手身份越权。`task.cancel` 的操作核复用窄 TaskController 端口，未另写一套任务状态机；摘要不承诺上游退款。

DSH 插件 `connection.js:195-223` 明确注册 `kind: other`、read-write、`approvalMode: caller` 的 agent-client，然后用 `@deepseek-ai/dsh-mcp-client` 启动该运行包的 BeefTV CLI `mcp serve` 并注入 DSH。[F04] DSH 的模型循环是 DSH 原生 Agent；BeefTV 提供现有画布/资源/操作。`host.js:76-105` 在真正执行时记录连接归属，在工具结果回来时识别已提交 taskIds，切换工作区后的结果不在新工作区轮询。`await-machine.js` / `await-runner.js` 将等待、唤醒消息、去重、停止后的撤回、awaiting.json 持久化分别实现。README 说明可连本机 2.2.0+ 或独立运行包，默认步数 40、可关自动继续，离线期间完成只提示、不自动恢复生成流程。这比“调用 MCP 就会自动执行所有制作”更具体。

## 3. 两条真实制作链及其连接边界

### 3.1 内置助手/外部 Agent 的画布单镜链

1. `canvas.script.prepare` 接收 1–8 个分镜行及可选首帧/视频配置，创建脚本行、媒体草稿和关联。它明确不生成。实际 schema 包含时长、叙事意图、POV、表演调度、镜别、情绪、灯光、音效、运镜、节拍、必须项、连续性和负向提示；这些是可写制作描述，不是自动证明一致性的检查器。
2. 分镜行的 imageNodeId / videoNodeId 与同画布且类型匹配的媒体节点绑定，同时维护唯一相关连线（`canvas/storyboard_writes.go:30-61`）。
3. `canvas.generation.propose` / generation order 记录待生成节点与模型参数。正式 `canvas.generation.submit` 对外部 caller 按 ask/auto/caller 模式执行审批，检查画布 revision、模型配置 revision 和计划 targets；ask 模式不能由 Agent 自己批准。已 submitted 且有 taskIds 的订单返回原 taskIds。[F05]
4. 实际准入在 `app/external_generation.go:12-18` 与 `app/assistant_production.go:173-309`：外部 operationID 派生稳定 production key；内置助手必须持有确认端点产生的授权、原 turn/session 一致且未撤销。`admitProductionTargets` 先按稳定 key 找旧 task；找到后校验原 admitted request 的 hash，不为恢复重新选择凭据或参考资源。[F06]
5. task worker 调 provider。产物取回后走 taskdelivery/资源库，基于 taskID + outputIndex 派生稳定 Asset/Version/Representation 身份；绑定同一 task-output 不创建第二套资源。等待工具同时要求任务成功、outcome.phase ready、本地 resourceId、无 materializationError，若关联了 nodeIds 还要读画布确认 success + resource: 引用及正确 taskId。生成接口 accepted 不等于完成。[F07]
6. `canvas.timeline.update` 解析当前正式资源，验证归属、资源 ready 和媒体类型，编译剪辑计划；时间线保存 directMedia 资源快照。局部替换只给 refreshNodeIds：只清掉所选节点片段的旧 directMedia 后重解析和编译，不改其他片段。[F08]
7. `canvas_timeline_deliver` 是确定性复合工具，依次保存/刷新时间线、发起本地 render、创建成片节点、绑定原 render task 输出、导出可编辑工程 ZIP。各步骤仍有独立 Go 操作回执与稳定子任务 key。部分步骤失败返回 incomplete 与已完成 receipts、taskId/nodeId，提示续失败步骤，避免重新生成/渲染。[F09]

这是可从主 host 和 operation bridge 连到产物与工程导出的实际代码链；并非只有方法声明。

### 3.2 正式 Project / Unit / Shot 制作链

该仓库还存在独立的正式制作实体，不能只总结为“画布节点”：`models_project.go:254-307` 定义 Shot / ShotRevision / ShotArtifact；镜头有稳定 ID 和当前 revision 指针，artifact 关联 task、resource、版本、状态和 selected。`ProductionTaskLink` 还保存 task 与制作阶段之间的关系。

`project/canvas_production.go:99-190` 的事务入口要求当前 shotRevision、同画布的成功 task、本地 ready resource、正确媒体类型；一个 task 多输出时必须显式选择 resource；拒绝被另一个镜头/产物或旧 revision 使用的绑定。它创建/确认 GeneratedProjectAsset，再调用 RegisterTaskOutput。

`project/workflow.go:205-319` 校验 project/canvas/task/step 归属后调用事务写入；`planRegisteredTaskOutput:322-342` 只有等于当前 ShotRevision 的结果才 selected + ready，旧版本结果仍保留但变 stale。`completionGateAllows:436-497` 在 storyboard 阶段检查每镜有 revision；previz/video/delivery 阶段要求当前单元的每个镜头都有 selected、ready、revision 匹配的对应 artifact，防止“少一个镜头也标阶段完成”。[F10]

自动登记来自 `taskruntime/terminal.go:183-187` → `app/project_workflow.go` → `project/workflow_task_output.go`，**前提是原任务输入已携带 workflowStepId 等正式制作上下文**；后者缺少 stepId 直接返回，不会猜测关联。

关键边界：本轮在 `web/src` 精确检索 getCanvasProductionContext/registerCanvasProductionTask 仅找到 `services/api/projects.ts:633-651` 的客户端定义；在选定 `agent-host` / `operations` 目录未找到自动把画布生成请求填为正式 workflowStepId/shotRevisionId 的调用。HTTP `/projects/:id/canvases/:canvasId/production-context` 路由和后端事务/测试存在，但不能由此宣称内置助手的通用画布流程已自动与正式 Unit/Shot 工作台全部打通。

## 4. 自动推进、恢复与局部重做

- **计划推进是有限的**：`plan.mjs:70-76` 默认最多 6 次 continuation，needs_user / blocked 立即停；没有剩余 doing/todo 就不续。`durable-extensions.mjs:477-534` 只有本轮触碰计划且调用过工具才续，pending proposal、停止、新输入均阻止自动继续；另检查本轮新建媒体是否写回分镜行、视频是否连接首帧，缺失时补一次提醒。计划 done 仍由 Agent 维护，并不能替代正式资源/阶段检查。[F11]
- **等待与模型分离**：await-business-tasks 固定轮询原 task.get，不调用新模型或购买新任务；观察失败保留原 taskIds 并返回 pending，保存失败返回 incomplete。[F07]
- **20 分钟不是此函数可证的跨重启绝对期限**：await 的 deadline 在入口由 Date.now() 新算；声明持久化 harness 不等于所有等待计时都持久化。本轮没有证据把它描述成跨重启固定原 deadline。
- **原结果恢复与付费重做分开**：`task.RecoverTask` 从 succeeded 继续本地交付；失败时按图片/视频/音频/超分恢复原结果。源码特别区分手动 WithManualRecovery：仅人工“继续取回”可在托管准入没有收到原请求的情况下用原 key/body 补交；后台只查。不能简化为“一律从不重 POST”，也不能简化为“失败自动再生成”。[F12]
- **改幂等键不构成重购许可**：ReleaseFailedGenerationNodes 检查当前节点的 task/operation 绑定，只有后端判定为可再生成的终局失败才释放；running、retrievable、settling、delivered、提交不明都保留原关联。需要另做成功镜头时新建节点；选中的新资源再刷新所需时间线片段。这个操作约束限制盲目重购，未发现自动质量循环无限重抽。[F13]
- **取消与迟到结果**：`repository.go:471-499` 按 user/id/expected status 条件取消并清 lease，provider cancellation 另用条件更新只给一个发送者；`worker_execution.go:362-368` 在 provider 结果回来后重读 cancelled 状态，转入取消结果处理，不按普通成功交付；`repository.go:411-416,452-468` 要求有效 lease owner、旧状态仍匹配才写终态，过期 worker 不能覆盖新事实。[F14] `taskruntime/terminal.go:201-210` 对 RowsAffected != 1 返回状态冲突。取消不等于退款。

## 5. 费用与质量的实际边界

`generation.estimate` 仅用已保存价格目录，最多 200 个 targets，不联网、不建任务也不写账；应描述为报价预估。订单/审批固定本次制作对象与参数，但本轮未审计托管计费服务的完整账务合同，不能由本地估价或本地防重推出跨任意供应商的财务 exactly-once。

`media.review` 是独立、多模态模型的一次有界推理：必须指定原媒体 expectedVersion、开始/结束时间和问题，最长 15 秒，返回媒体 SHA256、source、模型观察和 usage。接口明确没有工具、业务写入和恢复循环；readonly 也标 ReplayUnsafe，因为可能收费。此工具与“专业子 Agent 负责整个审查流程”不同。源位置 `backend/internal/operations/media_review.go:9-83`。

时间线导出返回 delivered 后仍明确提醒读取实际画面、声音和字幕。源码中能确认结构/引用/版本/绑定/产物/剪辑计划检查；不能确认每个交付都被强制通过人物脸部一致性、口型、对白逐字保真、音量、黑帧、字幕遮挡等完整视听检查。media.review 返回观察文本并提示可能遗漏，不是确定性的质量合格证明，也不是本轮已经跑过的付费审片。

## 6. 已找到的质量机制与证据强度

仓库测试包括 durable recovery/interrupted host、原工具回放、技能 hash/pin、timeline-delivery real Go probe、任务 submit unknown/cancel race、租约 fencing、storyboard 与正式 production 版本、媒体渲染/导出等。质量工作流会运行 Go tests、Node/Bun Agent host 测试、请求效率 gate、Web 构建及 Playwright；还区分 Windows 真实文件锁、发行升级/回滚以及按需运行的 published-package 检查。`agent-host/eval/real-gen-*` 和 report-production.py 提供真实生成评估采集能力，存在脚本不等于本轮真实付费评测已经跑过。

GitHub 插件 combined-status 返回空 statuses，不代表没有 Actions。因插件 commit-workflow 接口只覆盖 PR，额外读取 push actions 元数据：首次采集时两个工作流仍在执行；随后主调查者在 14:38 核验 Quality checks run 43168029091 已 completed/success，本分调查也经 GitHub 插件复核：Release publisher、Web、Structure budget、Backend、Windows storage migration 成功；按需的 Published Windows upgrade / SQLite runtime 两个任务 skipped。Publish container images 43168029183 在主调查者该次核验时仍在运行，未据此声明镜像已发布。**质量工作流通过不等于所有按需任务执行，也不等于付费内容质量通过**。[F15]

### 6.1 桌面与服务化的证据边界

本轮追踪的内置会话/DSH 原位连接主要使用桌面、本机 IPC、会话 SQLite 与单工作区能力；外部 DSH 独立工作区还明确不与本机应用共享数据。源码中的 userId、owned resource、operation scope 检查确实存在；这不能替代 SaaS 跨租户隔离、多个 runtime 共同恢复、集群级配额/排队和账务对账的运行证据。仓库含可容器化服务形态，不能反过来把全部产品硬归为只有单用户桌面，也不能从桌面运行正确推断多用户服务已经稳定。

## 7. 许可证事实

固定 SHA 的 LICENSE 为分段许可。Part A 从引入该许可的提交起覆盖符合范围的新原创材料；历史 MIT（明确包括 a96baf0、2327ee2 及其他先前 MIT 材料）仍保留原许可，不被撤销，混合文件按相应部分区分。Part A 允许研究、自用和组织内部商业创作，允许商业使用/交付创作输出；对受覆盖软件及衍生物的对外服务（免费也包括在定义内）、收费系统交付/部署等要求另行书面授权。不能把整仓或最新版本称为 MIT，也不应把历史 MIT 权利说成已消失。这是源码许可文本事实，不是对具体采用方式的法律结论。文件位置 `LICENSE:10-37,45-68,87-108`，第三方材料另见 NOTICE。

## 8. 结论与未决项（不形成 Glowloom 实施建议）

已经回答：内置主运行时、统一工具/operation 边界、Skill pin/恢复、外部 MCP/DSH 的正式消费者、生成订单授权、原任务等待/恢复、取消 CAS 与迟到 worker、画布单镜到可编辑工程交付链、正式 ShotRevision/Artifact 与阶段门禁。

部分回答：正式 Project/Unit/Shot 后端与内置助手通用画布生产的自动连接（已定位 API/模型/自动登记条件，尚未发现所选前端与 host 目录中的自动填充调用）；跨所有接入方式的子 Agent 体系（只确定内置生产 host 注册边界与外部 DSH 接口，未审计全部 DSH 原生调度）；账户财务与第三方供应商 exactly-once 合同。

未验证：当前发行包与固定源码的逐字节对应、真实供应商重启/未知提交/取消收费结果、各目标剪辑器的实机导入、长剧角色与声音一致性和逐镜视听质量。本轮不将这些未知项包装成能力已完整落地或质量保证。

## 固定提交关键来源（15 条）

[F01]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/agent-host/durable-session-owner.mjs#L245-L287
[F02]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/internal/operations/registry.go#L37-L65
[F03]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/cmd/beeftv/main.go#L944-L1014
[F04]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/plugins/deepseek-harness/connection.js#L190-L230
[F05]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/internal/operations/generation_submit.go#L189-L309
[F06]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/internal/app/assistant_production.go#L173-L278
[F07]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/agent-host/await-business-tasks.mjs#L20-L101
[F08]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/internal/operations/canvas_timeline.go#L122-L173
[F09]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/agent-host/timeline-delivery.mjs#L34-L120
[F10]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/internal/project/workflow.go#L322-L497
[F11]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/agent-host/durable-extensions.mjs#L477-L534
[F12]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/internal/task/recover.go#L19-L38
[F13]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/internal/operations/generation_resubmit.go#L15-L80
[F14]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/backend/internal/repository/repository.go#L411-L499
[F15]: https://github.com/glanderness/BeefTV/blob/d339ab127daa6ca57125e50e9e022dcdeeff3a4b/.github/workflows/quality.yml#L111-L202
