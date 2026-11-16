# Toonflow / waoowaoo：独立源码复核

调查日期：2026-11-12。只读源码调查；未安装/运行目标、未调用供应商、未执行上游测试或核验真实成片质量。本报告不提出 Glowloom 的实现路线。

## 1. 身份、版本和调查边界

| 项目 | 当前默认分支与固定提交 | 提交时间 | 本地完整源码 |
| --- | --- | --- | --- |
| HBAI-Ltd/Toonflow-app | master / `6086cdffceb704a632eca143dd16cd461a78cdac` | 2026-11-05 15:49:27 +08:00 | `/tmp/glowloom-research-20261112-agent-platforms/toonflow` |
| waooAI/waoowaoo | main / `dfec20e32298ed675050329e4a41477024148399` | 2026-09-21 18:39:25 +08:00 | `/tmp/glowloom-research-20261112-agent-platforms/waoowaoo` |

两项目当前 SHA 与原 11-12 调研相同；本轮不是发现新版本，而是独立追踪原有实现，补充/纠正关于自动推进、子 Agent 和质量保证的证据。GitHub 插件实际调用了 repository search、fetch_commit、fetch_file（Toonflow subAgent.ts、waoowaoo capability-turn.ts）；完整源码采用浅克隆及逐文件阅读。普通完整源码不需要运行 REA 引擎，符合 reverse-engineer-anything 技能的目标路由规则。

关键发现：

1. Toonflow 具有真实并行子 Agent、团队成员委托、工具/知识范围约束；其主/子运行状态主要在进程内，不能等同于持久工作流。
2. waoowaoo 明确禁用 Codex 子 Agent，并将子 Agent 协议事件作为错误终止。它采用单主 Agent、多专业 Skill、产品 Operation 和确定性任务；不能称为六个专业子 Agent。
3. Toonflow 的媒体检查点恢复与 Agent 任务恢复不同；waoowaoo 的任务终态回传与创作质量通过也不同。
4. 两者的制作方法和人工审查描述都较详细，但生成结果的剧情忠实性、人物连续性、口型与音画质量没有因此获得运行时保证。

## 2. Toonflow 的实际执行链

### 2.1 主 Agent、子 Agent 与团队

`run → SessionManager/createAgentSession → createAgentTools → createSubAgentTool → runDelegatedAgent / createTeamRunner / runRemoteTeam`。

- 主会话使用 Pi Coding Agent，已有会话从工作区 `.agent/sessions` 打开，SDK 负责模型循环、压缩、重试和历史持久化。运行入口对同一会话加文件锁；用户 steering/followUp 入队也复用当前会话。
- `subAgent.tasks[]` 是实际 `Promise.all` 并行执行；临时子 Agent 建独立会话，只接收显式 task，不自动继承主历史。继承宿主工具，并可以继续委派、report、listAgents、sendAgentMessage。错误/取消/length 会返回不同 status，主 Agent 收到各任务结果和用量。
- `runDelegatedAgent` 建立独立 AbortController、会话和画布/提问桥，聚合 assistant/toolResult usage。`completed` 的来源是 `run()` 正常返回与最后助手文本，不是目标影片产物通过机器验收。
- 本地团队使用成员清单，director 只能委派给 writer/reviewer；服务器检查 delegates。每个成员只拿白名单工具和自己的私有资料根。团队根协调成员用内存 SessionManager 保留当前调用内历史，调用结束释放。它与普通子会话落盘是两种状态策略。
- 子任务工具说明明确不为任务数量、并发、轮次、总时长设置固定上限；它依靠用户停止和其他业务层约束，不构成已证明的全局配额/并发调度器。媒体层另外有 1000 个 active task 上限，不能混为子 Agent 上限。
- 强边界：sessions.ts 的 activeSessions 与 activeRuns 是内存 Map；源码注明多进程需共享会话所有权、进程退出后不恢复任务。因此“可重新打开历史”不等于“重启后自动接着完成原 Agent 执行”。

### 2.2 Skill 与 MCP

- Skill 扫描限定 `cwd/skill` 和全局配置目录 skills，关闭 SDK 默认扫描；同名工作区优先。写入校验 frontmatter 名称和 description，路径必须留在对应根内；正文按需读，不因为注册了名称就视为已执行。
- ResourceLoader 每会话独立，自动载入根 AGENTS.md，另加全局规则/记忆；关闭扩展、默认上下文/Prompt templates 等自动发现，避免意外加载。
- MCP 是真实外部客户端接入：对 enabled server 连接并 listTools，复用原 inputSchema，工具名增加 server/hash 以防冲突；执行时重查 enabled 和配置版本，传取消信号与超时，MCP `isError` 被转成 Pi 工具失败。一台服务加载失败变成可解释错误工具，保留其他工具。
- 此处证据支持“发现、调用、停用/版本校验与错误传播”，并不单独证明第三方 MCP 的业务效果可以幂等恢复，或获得统一成本/细粒度业务授权。需逐服务核验实际工具；不能把工具 schema 当作权限和效果合同。

### 2.3 制作阶段和局部重做

制作 Skill 把源文本、确认版本、资产、生成尝试、采用片段、成片和归集分开记录。状态可以是一个状态 Markdown 加画布总览节点；专业正文保留在节点及真实 textPath，不要求为简单任务预建全套表格。

其局部重做纪律很具体：提交审阅保存独立快照；确认后回读当前节点，发现等待期间用户编辑则不能顺便批准新内容；生成尝试保存完整提示词/参数/引用/授权快照。已采用媒体固定在素材节点，新修订另建候选；仅按实际依赖改变下游，保留旧结果。缺失旧确认依据时记未知，不能按“最新”“最终版”文件名猜采用版本。

但是这是 Skill 制作方法，不是一个全部由数据库约束的 Episode/Shot/Take DAG。所读状态文档也明确普通读写不能保证历史文件永远不可修改，发现不一致仍需核对。

### 2.4 媒体任务、费用和恢复

媒体任务记录有 taskId、工作区、请求、节点/画布归属、ownerUserId、冻结模型路由、chargeId、running/interrupted/succeeded/failed/cancelled，以及 providerData / URL / 已落盘文件哈希等 checkpoint。文件记录原子写入，按任务 key 串行处理。

- 相同 taskId 不可绑定不同请求；用户角色只能访问自己的任务，owner 校验在载入时执行。
- 多租户网关启用时，先保存任务提交意图，再预占费用，chargeId 落盘后才允许提交供应商。SQL 钱包与本地文件不能原子提交的间隙被显式处理；不能简化成一个全局事务。
- 恢复优先使用原 providerData / 已保存结果地址；`provider.resumeTask` 不重新读取参考、不重做预处理、不重新提交生成。完整文件按哈希复用；下载 URL 过期最多重查原任务一次。
- status 查询可载入并恢复原 running；显式 resume 可恢复 interrupted。有原任务凭据/资产或完整文件才允许恢复。缺少原凭据转 interrupted，要求确认供应商状态，而不是重新付费生成。
- canceled 先落盘；未知 taskId 也能先写 tombstone，避免乱序 start 在取消后创建付费任务。部分文件交付且结算不清楚保留 pendingReview，不把取消自动视为全额退款。
- Agent 中的 `wait` 与媒体 status/resume 是工具能力。Skill 本身明确：节点返回 generating 只代表启动；若当前没有等待工具，则登记当前结果，请用户完成后回复，再回读节点，不能编造后台自动唤醒。不能将媒体内部恢复解释成整剧 Agent 已有可靠无人工自动续跑。

### 2.5 合成和质量

- `ffmpeg` / `ffprobe` 是实际插件，调用宿主 Worker 执行 fluent-ffmpeg 的结构化方法，支持裁切、concat、截图、音轨等；错误事件转工具失败。视频节点另外有相对路径约束和 trim / extractAudio / separate 操作。
- 这些证明存在本地后期能力，不证明某次成片正确导出、声音同步或叙事通过。
- workflow Skill L1404–1406、stateAndDelivery 明确：视频路径、播放器、静帧不能证明 Agent 看过/听过全片；能力不够时需要用户预览并记录采用、问题和实际交界状态。它区分已生成、已采用、已合成、已归集，不能用清单完成冒充成片完成。
- 全仓传统测试文件搜索（`.test.*` / `.spec.*` / test(s) 目录，含隐藏工作流目录、排除 .git）结果为 0；package.json 有 build/typecheck，无 test 脚本。工作流侧查看了 release/debug/signPath，主要为构建、签名和归档检查。此负证据只限该公开树和这些命名/脚本；不能排除外部或私有测试，主调查者另用 GitHub 核验固定 SHA 的 Release Toonflow v2.0.10 运行 [41990625476](https://github.com/HBAI-Ltd/Toonflow-app/actions/runs/41990625476) 为 success；这证明该发行构建工作流通过，不能补足业务自动化测试覆盖。

## 3. waoowaoo 的实际执行链

`项目助手 Turn → 运行所有权与 Codex app-server Thread/Turn → 领域 Skill → 内部 Wao MCP → Operation 计划/审批/效果凭证 → Temporal Task → 供应商/产物 → Task 终态 batch → 新助手 follow-up Turn`。

### 3.1 单主 Agent 是主动设计

- 原生 Runtime 支持 local / docker，按 runtimeKey/cwd 管理进程。进程退出会移除内存实例，Thread 按持久身份 resume；生产云版要求 Docker，资源/镜像配置有校验。
- 重点纠正：runtime-skills.ts 实际写入 `[agents] enabled = false`；领域 bootstrap 要求主 Wao Agent 是结果的唯一作者；event-projector 收到 collabAgentToolCall / subAgentActivity 时两处都会终止为 runtime_protocol_error。
- 因此 registry 的 creative-core、script-development、creative-direction、asset-development、video-direction、music-direction 是六个 Skill，不是六个子 Agent。任务 Worker 也不能计数为语义子 Agent。
- 六个方法各有 domain/outputKind，主 Agent 依赖顺序读取并构造结果。输出 schema 注册表将 screenplay、creative_direction、image/video/audio batch 对应到保存的 Resource schema 和具体媒体 Operation；Zod 严格解析专业结构。

### 3.2 MCP 及不可由模型自行授予的执行权

- Wao MCP 是产品内部 server，不是仅包装 HTTP 的文案接口；工具列表从 canonical Operation registry 生成。每次 call 解析受信 context；不存在活动 Wao Turn 则拒绝。
- 产品写入 fence 依赖 owner token、唯一活跃 Turn、runtimeTurnId = executionOwnerId。原生 Thread 只标识对话历史，不授予业务权限。
- 付费 Operation 准入先规范化输入，hash 后绑定 turn/call/operation，读取或保存相同 plan snapshot；确认开启时通过 elicitation 与 browser proof 绑定批准，之后再次授权检查。不能用模型回复“已同意”代替产品授权。
- MCP 错误规范化返回 isError；本次所读 server 表明它不直接改数据库/计费/Provider，而交给 executor 和 Operation。此证据不能推定任意外部 MCP 接入具备同等效果账本；本轮确认的是内部 Wao 工具入口。

### 3.3 自动推进、恢复及付费提交未知

- Temporal Task 的终态回执校验 task/status/terminal identity，非取消 scope 中释放 capacity，再发送 required follow-up；供应商 cancel 是后续资源补偿，不能先于产品交接。
- follow-up batch 持久记录原 executionKey/Turn/call/operation 和成员。成员终态以 eventId 绑定，CAS 在 pendingCount=0 时改 ready。completed / failed / canceled 都是终态，所以唤醒意味着“可以汇总结果/处理失败”，不意味着全部成功。
- submitTaskFollowUp 重新确认原 thread 一致，replay 检查避免重复启动已开始的续跑；创建的新助手 Turn 再读产物和任务事实。
- 供应商调用另有持久 checkpoint，而不是依赖 Temporal 重试的次数：先原子占 submitting，冻结 input fingerprint 和调用 descriptor。submitted 回放原结果；未拿到 claim 或未知状态不能调用 Provider；一般断连写 outcome_unknown 并拒绝以后再提交。
- pre_accept_rejected/rejected 需要适配器的明确类型事实；不能仅凭 HTTP 422 或 AI SDK statusCode 推断安全切换供应商。已接收排队任务到明确失败或明确排队超时后，可经同一 identity 写 replay_authorized，再按规定 attempt 重新准入。
- 对应 DB integration 测试写了“并发首 claim 仅一次外部调用”“断连未知后所有重提均拒绝”“不同图候选独立 fence”“明确拒绝后才切路由”。这些是测试源码证据；本次没有执行它们，也不构成真实供应商是否支持 idempotency 的证明。

### 3.4 版本、局部重做与交付

- Resource 生产输入引用 resourceId/contentVersion/workspacePath/role/position，真正解析时按冻结的 ResourceVersion 读取并校验 user/project、类型和媒体身份，不读当前版本。改名、移动和后续修改不能替换已批准的输入。
- retry 从原冻结执行输入构建严格新 payload 和新 fingerprint，再形成下一 Task；不能信任外部提交的旧摘要。此机制支持有限目标重做，但不是已经证明的整剧依赖自动传播和未受影响镜头的全覆盖失效图。
- video merge 是实际实现：冻结每个源视频的 contentVersion、输入位置、startFrame/frameCount、尺寸、画幅、audioMode；30fps 输出，每个视频恰好一个 frozen edit；BGM 每个输入恰好一个 cue，静音不能带 cue。
- handler 从对象存储取源文件，按 edit 归一化/裁切、concat，读取拼接中间片 `stitched.mp4` 的实际时长/尺寸，偏差超过 0.001s 或尺寸不符拒绝；处理原声音轨/配乐后上传最终 mp4、注册 Media 并返回 duration/width/height/clipCount。此处返回时长沿用中间片测量值，所读 handler 没有在最终混音 mp4 上传前再次显式探测时长/尺寸，不能将中间片规格检查直接称为最终文件全项验收。它不是一个仅声明 merge 接口的空流程。
- 当前合同最多 50 段；带 BGM cue 时另外限制 sourceCount=1。这是当前实现边界，不能因为 UI 有配乐选项推定多段任意时间线混音都已支持。输出是固定 fps 的 MP4 合成合同，不等同专业剪辑器工程/原生时间线导出。
- 专业 Skill 写了源文本忠实性、对白逐字、片段入口/出口状态、镜头节拍、接缝、声音关系的检查；本轮所读 Operation/Task handler 的硬校验主要是结构、身份和媒体规格。没有据此证明人物脸一致、表演正确、剧情完整、口型及音画主观质量能自动验收。

### 3.5 测试、CI 与许可

- 可见 unit、conformance、DB integration、provider contract、Temporal、billing、security 等套件；核查了 provider-invocation-at-most-once、approval-plan-change-replay、assistant-runtime-capability-turn 及工作流定义。
- verify.yml 在 main push/PR 等触发，包含 secret scan、static verification、logic、registry conformance、critical infrastructure，以及非 PR 的 browser security；package scripts 中测试区分 self-hosted / 可用 Cloud 依赖和不同计费模式。
- 可见默认 verify env 为 self-hosted、user-key、BILLING_MODE=OFF；部分关键套件另外显式 bootstrap/其他计费模式。主调查者通过 GitHub 固定 SHA 核验 [Verify 35589902169](https://github.com/waooAI/waoowaoo/actions/runs/35589902169) 和 [Build/Push 35589902085](https://github.com/waooAI/waoowaoo/actions/runs/35589902085) 为 success；这是工作流结果，不是供应商或成片的本次运行验收。不能只用该环境声明“正式云端付费链路已验收”，也不能反向推断计费完全没测。
- 当前源码树为 Elastic License 2.0；LICENSE 包含 hosted/managed service 使用限制。它不是可直接按 MIT 复制的统一底座。Toonflow LICENSE 为 MIT；第三方依赖/模型/素材许可未做逐项审查。

## 4. 核验结论与剩余问题

| 问题 | Toonflow | waoowaoo |
| --- | --- | --- |
| 主/子 Agent 真实调用 | 已回答：临时子 Agent、Team/A2A 委派实际存在 | 已回答：原生子 Agent 被主动禁用/拒绝 |
| Skill 加载及调用边界 | 已回答：受限目录扫描、按需正文、团队私有资料 | 已回答：唯一领域 Skill、输出 schema、实际物化 |
| MCP 入口及错误 | 已回答外部客户端配置/输入schema/停用/错误传播；效果恢复未证明 | 已回答内部产品 MCP/Turn fence/审批；外部 MCP 泛化未证明 |
| 制作和后期能力 | 已回答方法、画布和媒体/FFmpeg工具存在；实际成片未运行 | 已回答方法/正式Operation/真实MP4合成源码；实际成片未运行 |
| 自动推进与重启 | 部分：媒体任务可恢复，Agent active run 退出不恢复；完整自动续跑未证明 | 部分：持久Task→batch→follow-up和Thread resume有源码；生产故障恢复未运行 |
| 局部重做、采用与依赖失效 | 部分：Skill 明确纪律，全面机器强制未证明 | 部分：冻结ResourceVersion与单目标retry有实现，整剧依赖失效闭环未证明 |
| 交付质量 | 规格工具+Skill/用户审片；语义质量自动验收未证明 | schema/身份/实际MP4规格检测；语义质量自动验收未证明 |
| 自动化测试/CI | 常规测试文件与test脚本未发现；固定 SHA Release 构建 success；本轮未执行上游应用/测试 | 固定 SHA Verify/BuildPush success；本轮未执行上游测试或实片验收 |

后续若要把任一项目描述为“已验证的完整短剧平台”，仍需要使用明确故事样本、授权供应商和真实应用，验证：生成中的重启与提交回执丢失、取消/改权限后的返回、单镜重做对其他镜头的影响、人工采用版本绑定、音轨与成片内容，以及导出文件实际播放/导入结果。本轮仅完成上述静态源码范围，不能保证这些未知项。

## 5. 关键固定源码链接

### Toonflow

- [T01 主运行装配与工具/子会话](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/agent/runtime/index.ts#L140-L239)
- [T02 并行委派、继承工具和 status](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/agent/tools/subAgent.ts#L10-L66)
- [T03 独立会话与结果汇总](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/agent/runtime/delegation.ts#L12-L72)
- [T04 内存执行、退出不恢复](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/agent/runtime/sessions.ts#L72-L85)
- [T05 Team 权限及并行成员](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/agent/teams/index.ts#L54-L127)
- [T06 Skill 目录与写入校验](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/agent/skills/index.ts#L10-L78)
- [T07 MCP 真实调用及错误传播](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/utils/mcp/servers.ts#L193-L236)
- [T08 媒体恢复与缺凭据停车](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/utils/media/tasks.ts#L510-L559)
- [T09 原供应商任务/URL恢复](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/apps/server/src/utils/media/generation.ts#L296-L346)
- [T10 状态、局部修订及人工采用](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/packages/skills/workflow/references/stateAndDelivery.md#L39-L80)
- [T11 Skill 明确不伪造自动唤醒](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/packages/skills/workflow/SKILL.md#L1398-L1406)
- [T12 实际 FFmpeg 插件](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/packages/tools/ffmpeg/src/index.ts#L31-L80)
- [T13 MIT 许可](https://github.com/HBAI-Ltd/Toonflow-app/blob/6086cdffceb704a632eca143dd16cd461a78cdac/LICENSE)

### waoowaoo

- [W01 单主 Agent 与 agents 禁用配置](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/creative-skills/runtime-skills.ts#L128-L196)
- [W02 子 Agent 事件作为协议失败](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/assistant-runtime/event-projector.ts#L535-L541)
- [W03 专业输出 schema 与 Operation 映射](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/creative-skills/output-registry.ts#L22-L137)
- [W04 MCP 受信上下文与错误](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/wao-mcp/server.ts#L83-L211)
- [W05 唯一产品 Turn 效果 fence](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/assistant-runtime/capability-turn.ts#L64-L154)
- [W06 输入hash/计划快照/审批准入](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/wao-mcp/production-executor.ts#L345-L434)
- [W07 任务终态容量释放和续跑交接](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/temporal/workflows/task.ts#L277-L329)
- [W08 follow-up 全员终态及 CAS](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/agent-turn/follow-up-batch.ts#L228-L331)
- [W09 已提交回放与未知不得重提](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/task/provider-invocation.ts#L940-L1017)
- [W10 对应并发/未知 DB 测试](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/tests/integration/task/provider-invocation-at-most-once.integration.test.ts#L372-L420)
- [W11 冻结输入 ResourceVersion](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/workspace-resource/input-media.ts#L14-L65)
- [W12 冻结合成合同与限制](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/workspace-resource/video-merge-contract.ts#L10-L110)
- [W13 实际裁切/合成/媒体规格校验](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/src/lib/task/execution/handlers/workspace-resource-video-merge.ts#L66-L205)
- [W14 CI 定义](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/.github/workflows/verify.yml)
- [W15 Elastic License 2.0](https://github.com/waooAI/waoowaoo/blob/dfec20e32298ed675050329e4a41477024148399/LICENSE)
