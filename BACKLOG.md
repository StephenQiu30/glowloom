# 织光当前工作清单 (Glowloom Backlog)

> **当前聚焦里程碑**：完整创作 Agent Harness、多 Agent 协作系统与 Skill 沙箱体系
> **核心架构原则**：严格遵循 `PROJECT.md` 单一进程（`backend/cmd/glowloom`）原则，在 Go 后端进程内原生统一实现；采用成熟的 **LangGraph (StateGraph) 状态图引擎**，深度吸收开源 **BeefTV 与 Pi 最新稳定版（v1.3.0）的工业级 Harness 机制**；状态统一持久化至 PostgreSQL，彻底去除多余的独立 Node.js 宿主过度设计。

---

## 1. 核心架构与设计规范

1. **统一单进程引擎**：
   - 所有 Agent Harness、节点调度、模型网关与专业 Worker 均内嵌于 Go 业务进程（`backend/internal/agent/`）。
   - 杜绝多语言微服务运维成本、跨网络 RPC 签名与脑裂风险。
2. **工业级 Harness 四大机制（吸收 BeefTV 与 Pi 稳定版）**：
   - **提案即截断中断（Proposal Yield & Human-in-the-Loop）**：模型调用任何涉及计费、媒体渲染或结构性覆写的工具时，主动标记中断并保存检查点，等待创作者审核批准；未获批准前绝不发起后续推理。
   - **连续多步写链 CAS（Write Chain）**：单轮内连续多次调用写工具时，本地自动在链头递增版本（`Head`），下游工具自动以最新版本作为 `ExpectedRevision`；外部协作者修改被严格拦截在 CAS 边界之外。
   - **现场快照自愈（Studio Digest & Canvas Changes）**：在图引擎发起推理前，自动比对画布/分镜现场快照哈希；若检测到协同变动，自动生成 `<canvas_changes>` 差异摘要文本并前置注入到模型上下文中。
   - **双重超时与预算看门狗（Turn Budget & Idle Watchdog）**：单轮最大 40 步模型请求 / 40 步工具调用硬熔断；SSE 分块 45 秒空闲超时看门狗，防止供应商连接假死。
3. **Supervisor-Worker 多 Agent 编排**：
   - **制作统筹（Supervisor）**：意图理解、全局计划拆解（维护 `plan_update`）、进度追踪与条件边动态路由；
   - **剧本解析（Script Worker）**：长文本分集、场景、人物与台词拆解，标记歧义并生成建议；
   - **分镜编排（Storyboard Worker）**：机位设定、运动轨迹规划与时长切分；
   - **镜头导演（Shot Worker）**：提示词优化、视频参数装配与生成提案中断；
   - **后期剪辑（Audio Worker）**：配音音色、音效（SFX）、背景音乐（BGM）与时间线对齐。
4. **Skill 包生命周期与沙箱安全**：
   - 以 `SKILL.md` 为统一入口规范，内容哈希锁定与防路径穿越（严格拦截 `../` 越界）；
   - 支持 `skill_load` 按需动态装载正文与子任务独立上下文重置；
   - 专用子命令 `glowloom sandbox serve` 提供资源限制、无凭据泄露与超时有界回收的隔离执行环境。

---

## 2. 实施落地完成清单 (T-AH-01 ~ T-AH-08)

| 任务编号    | 模块职责                      | 核心交付代码路径                                                                                                                                                                                                                                                  | 验证状态                                                                                                   |
| :---------- | :---------------------------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :--------------------------------------------------------------------------------------------------------- |
| **T-AH-01** | **StateGraph 引擎与检查点**   | `backend/internal/agent/domain/graph/{engine,state}.go`<br>`backend/internal/agent/adapter/postgres/checkpointer.go`<br>`backend/db/agent_checkpoint.sql`                                                                                                         | **已完成 (Green)**<br>状态图条件边拓扑、步数看门狗、PG 快照持久化与断点接续测试全量通过                    |
| **T-AH-02** | **Supervisor 统筹与提案中断** | `backend/internal/agent/application/workers/supervisor.go`<br>`backend/internal/agent/application/proposal_interrupt.go`                                                                                                                                          | **已完成 (Green)**<br>目标分解、依赖调度、提案中断挂起与用户批准/驳回接续测试全量通过                      |
| **T-AH-03** | **统一网关与流式看门狗**      | `backend/internal/agent/application/model_gateway.go`<br>`backend/internal/agent/application/model_stream_watchdog.go`<br>`backend/internal/agent/application/turn_budget.go`                                                                                     | **已完成 (Green)**<br>参数冻结、上下文窗口校验、45s SSE 空闲检测与 40 步硬熔断测试全量通过                 |
| **T-AH-04** | **业务工具、写链与现场自愈**  | `backend/internal/agent/application/tools.go`<br>`backend/internal/agent/application/write_chain.go`<br>`backend/internal/agent/application/studio_digest.go`                                                                                                     | **已完成 (Green)**<br>连续写链版本自增、CAS 冲突拦截与 `<canvas_changes>` 差异注入测试全量通过             |
| **T-AH-05** | **Skill 生命周期与动态装载**  | `backend/internal/skill/domain/skill.go`<br>`backend/internal/skill/application/skill_registry.go`                                                                                                                                                                | **已完成 (Green)**<br>版本固定、SHA-256 哈希、路径穿越拦截、`skill_load` 展开与上下文重置测试全量通过      |
| **T-AH-06** | **专业 Worker 智能体族**      | `backend/internal/agent/application/workers/script_worker.go`<br>`backend/internal/agent/application/workers/storyboard_worker.go`<br>`backend/internal/agent/application/workers/shot_worker.go`<br>`backend/internal/agent/application/workers/audio_worker.go` | **已完成 (Green)**<br>剧本拆解 → 分镜排布 → 镜头提案挂起 → 审批恢复 → 音频后期全流程流水线接力测试全量通过 |
| **T-AH-07** | **隔离沙箱与安全脚本执行**    | `backend/cmd/glowloom/sandbox.go`<br>`backend/internal/skill/domain/sandbox.go`<br>`backend/internal/skill/application/sandbox_runner.go`                                                                                                                         | **已完成 (Green)**<br>`glowloom sandbox serve` 子命令就绪，超时强制终止与环境变量安全隔离测试全量通过      |
| **T-AH-08** | **SSE 事件流与前端卡片交互**  | `backend/internal/agent/adapter/http/stream_handler.go`<br>`frontend/src/components/agent/ProposalApprovalCard.tsx`<br>`frontend/src/components/agent/index.ts`                                                                                                   | **已完成 (Green)**<br>SSE 标准事件推送、Next.js 提案审批卡片与参数确认组件全量通过 TypeScript 类型检查     |

---

## 3. 质量门禁与验证指标

- **后端测试套件**：
  - `backend/tests/agent/...`（20 项）、`backend/tests/skill/...`（3 项）、`backend/tests/config/...`（23 项）；
  - **46 / 46 测试通过率 100%**；
  - 启用 `-race` 竞争检测：**0 数据竞争**；
  - `go vet ./internal/agent/... ./internal/skill/...`：**0 警告**；
  - `gofmt -l` 规范检查：**0 格式偏差**。
- **前端类型与规范**：
  - `pnpm exec tsc --noEmit`：**0 错误**；
  - 纯正 Radix UI / Tailwind 原生卡片与状态绑定。
- **版本控制与干净工作区**：
  - 全部成果已固化在 `main` 分支（提交 `7aaec94b`、`d2fc12f7`、`4a1c76e1`）；
  - `git status --short` 当前保持 Clean。

---

## 4. 下一步演进计划 (Next Iterations)

借鉴 Pi 最新稳定版（v1.3.0）与真实业务场景，持续进行深度打磨：

1. **Pi `taskId` + `memo` 确定性记忆机制深化**：
   - 为每次工具调用注入基于 `RunID + StepID + ToolName + ParamsHash` 的确定性 `taskId`；
   - 增加执行记录 `memo` 查找表，使断点重试与审批接续时已执行完成的只读工具免于重复计算。
2. **Pi `beforeCompact` 上下文智能有界压缩**：
   - 监听长文本交互 Token 消耗，在接近模型上下文上限（如 80%）时触发自动压缩，提炼中间思考并保护核心实体。
3. **画布工作台联调对接**：
   - 将 `ProposalApprovalCard` 挂载至前端工作台画布实时面板，接入真实项目会话事件流。

---

## 5. 唯一权威文档追溯

- **原始需求**：[19-完整创作Agent与Skill多Agent需求.md](docs/requirement/19-完整创作Agent与Skill多Agent需求.md)
- **架构设计**：[32-完整创作AgentHarness与Skill多Agent架构设计.md](docs/design/32-完整创作AgentHarness与Skill多Agent架构设计.md)
- **产品PRD**：[19-完整创作Agent与Skill多Agent.md](docs/prd/19-完整创作Agent与Skill多Agent.md)
- **实施计划**：[28-完整创作Agent与Skill多Agent实施计划.md](docs/plan/28-完整创作Agent与Skill多Agent实施计划.md)
- **调研分析**：[2026-11-01-AgentHarness与Skill能力分析.md](docs/research/2026-11-01-AgentHarness与Skill能力分析.md)

---

<a id="beeftv-adoption"></a>

## 6. BeefTV调研采用：模型接入扩展、计价与结算恢复（2026-11-03）

本节为该专项实施状态的唯一来源，不改变前文Agent里程碑记录；前文测试/提交记录仅按其原版本理解，不能作为本专项验收。

**文档、代码实施与独立整体工程规格/质量审查已完成；用户于2026-11-03授权实施。本次实现相关代码、测试、文档与BACKLOG按用户后续授权提交main并推送；CI独立于既有本地工程验证。首个Seedream 5.0 Flash已按SRC-AI-36完成本地真实文生图/图生图、COS归档、启用及首页免费报价接受；其他模型、个人积分钱包及完整产品接受未完成。** 设计评审、商业政策接受、代码实现、实际供应商和产品接受分别记录。

来源：[调研](docs/research/2026-11-03-BeefTV模型接入与计费调研.md)与[需求04 SRC-AI-35](docs/requirement/04-AI平台接入与预设管理需求.md#src-ai-35)。

| 父任务                     | 唯一设计→PRD→Plan                                                                                                                                                                | 当前状态                                                | 优先级建议/依赖                                  |
| -------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------ |
| T-AI-21 接入声明与协议扩展 | [设计33](docs/design/33-多设计模式驱动的模型与供应商接入扩展体系设计.md) → [PRD20](docs/prd/20-模型与供应商接入扩展.md) → [Plan29](docs/plan/29-模型与供应商接入扩展实施计划.md) | 工程实现/任务复核通过；整体工程审查通过，联合接受未完成 | P2；补原任务恢复联动及产品接受                   |
| T-AI-22 计价与费用说明     | [设计34](docs/design/34-积分消耗系统与模型真实计价体系设计.md) → [PRD21](docs/prd/21-模型计价透明与结算恢复.md) → [Plan30](docs/plan/30-模型计价透明与结算恢复实施计划.md)       | 工程实现/任务复核通过；整体工程审查通过，合同门控保留   | P1；D-AI-35-01/02影响对应功能                    |
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

[工程验证记录](docs/research/evidence/2026-11-03-beeftv-adoption/engineering-validation.md)。Plan29提交前工程证据（以下命令在当时共享工作区执行）：隔离catalog全量及Race通过；协议/operation Race通过；前端279文件/2016测试、API一致性、ESLint、类型及构建通过。独立审阅发现的重复键保存绕过已按Red→Green修复并复核关闭。本次改动golangci增量检查0问题；全scope保留既有静态问题及18个无关前端格式文件，未扩展修复。fixture不替代浏览器/供应商接受。

工程补证：Plan30初审4项Important已按Red→Green修复；round1引入的legacy/reuse完整性兼容问题已在round2关闭，原独立PG失败用例转PASS。多产物中断仅在实际manifest全集归档后ready；账务冲突统一约束GET/quote/单批确认/dispatch；足额、部分释放及既有overage均不误封。修复后catalog Race371通过，operation Race663通过/26跳过；前端284文件/2035测试、类型、ESLint、API与生产构建通过。全仓/专库/最新family去重为3345个pass动作、882个skip、0fail；跳过不计接受。总审查初审1项Critical、6项Important及字幕问题全部修复；精确复核剩余迟到矛盾回调、实际SDK固定ID冲突两项亦已独立原样复跑关闭，整体工程规格与代码质量通过，无未关闭Critical/Important。最终operation整包676通过/26跳过，operation/media内部合计733通过；catalog去重371通过；前端284文件/2038测试通过，最终相关组件49通过。最后两项收口Race105通过/0跳过，变更行lint/vet/goimports/diff检查通过。未再次重复未变前端/其他包；真实供应商/浏览器/生产迁移未执行。

### 6.2 待确认与外部条件

| ID         | 待决事项                                                   | 当前处理与关闭条件                                                                                                             |
| ---------- | ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| D-AI-35-01 | 1:10积分/默认1.0的对外政策出处、分数积分显示、多级倍率覆盖 | SRC-AI-36仅确认Flash本次0.12元/张、ratio10、markup1；分数积分和全平台政策仍待确认                                              |
| D-AI-35-02 | 参考图/重复引用/mask/每输出计数、缓存/视频Token单位        | 逐模型取得供应商一手合同；未定义的附加费不发布                                                                                 |
| D-AI-35-03 | 原币最终账单、超预留承担、退款补偿及采购/销售分账          | 明确账单身份、冻结汇率、超额政策、迟到退款流水与授权；原币路径保持关闭                                                         |
| D-AI-35-04 | 真实供应商/浏览器/迁移接受                                 | Flash本地两模式、COS归档、增量结构补齐、启用与首页免费报价通过；首次0.12元预留仍待核对，其他供应商/生产环境/完整交互接受未完成 |

### 6.3 下一步与完成判据

1. 本轮可执行工程范围已完成并通过独立整体审查；按[验证记录](docs/research/evidence/2026-11-03-beeftv-adoption/engineering-validation.md)和[逐项文件清单](docs/research/evidence/2026-11-03-beeftv-adoption/worktree-files.md)审阅本次提交范围；首轮实现已提交main并推送（4ab20c15）；后续Flash运行修复沿同范围交付，本地仅启用已验证Flash。
2. 分别关闭D-AI-35-01～03的商业/供应商合同；未确认前不开放对应参考计费、原币账单和退款补偿，不以工程fixture充当真实费率。
3. 在获准的运行环境补D-AI-35-04：真实供应商、生产式Temporal/Kafka/对象存储、捕获历史重放，以及桌面/320/390、键盘焦点和读屏接受。除下述Flash本地观测外，未执行项不计通过。
4. 新问题或后续发布继续沿对应CASE留证，保留原价格/汇率/Key/预留/账本；不得用清零旧账、超时解锁、同键重POST或新Key补买替代核查。

### 6.4 首个Flash模型实际接受（SRC-AI-36）

[运行验证记录](docs/research/evidence/2026-11-03-beeftv-adoption/runtime-validation.md)：复用现有COS桶及凭据，242项旧对象位置经字节核验绑定；文生图、图生图各成功并结算0.12元，已启用并设为图片能力默认。首页实际选模/免费报价显示0.12元、预估2积分；没有执行第四次供应商请求。

- 已完成：模型详情失败重试、试运行价格来源投影、Ark不支持参数移除、Pro/Flash尺寸候选修正、试运行模式空值同步修复及对应回归。
- 待核对：原请求`827948cf-87fe-48ac-bbdf-486eaf6f84c1`参数拒绝后为manual/费用未知，保留0.12元预留，不重发、不强制零结算；需取得原请求的可信供应商费用证据。
- 未接受：首页真实生成（本次3次供应商调用额度已用完）、其他11个模型、个人积分余额/充值/会员、生产部署、窄屏/读屏及真实在途历史重放。项目账本当前已结算0.24元、预留0.12元、可用0.64元，总预算1元。

- CI既有阻断：父提交b03f9dee与4ab20c15均失败于Agent测试库缺`agent.graph_checkpoint`（TestPostgresCheckpointerSaveAndResume）；4ab20c15前端CI通过。本次不扩范围修改Agent或CI，不将本地定向通过描述为主干全绿。

### 6.5 其他模型配置与计费补齐（SRC-AI-38）

用户选择先完成配置和代码、暂不收费调用。9个有现存凭据的模型已追加配置v2、11份审核价格；Pro图片、两款豆包文本及四款Seedance共7个待验证发布已保存。DeepSeek分别留存高峰/空闲价，分时结算依据未齐不创建发布；两个OpenAI模型仍缺Key。所有新增模型保持停用，Flash原发布不变。

- 已完成：缓存用量分价、Seedance输出Token报价/结算及发布门禁、Pro参考附加的发布/报价/冻结计数结算、试运行积分明细传递；受影响catalog/operation及managed/providerhttp整包Race通过，前端7项定向、类型和lint通过。完整验证见[本轮记录](docs/research/evidence/2026-11-03-beeftv-adoption/remaining-models-validation.md)及[安全配置快照](docs/research/evidence/2026-11-03-beeftv-adoption/remaining-model-configurations.json)。
- 免费页面验证：Pro 2K报价0.60元、预计6积分；仅quoted，不确认生成。当前视频最低样例高于原项目0.64元可用预算，保留既有页面门禁，不增加预算。
- 全仓限制：首次后端全仓命令受测试环境条件阻断，后续受影响整包已用完整隔离环境通过；前端全量2069通过/1失败，失败指向并行探索搜索存储策略，保留该改动。
- 未接受：9个模型的真实生成/归档/结算与启用、DeepSeek自动分时/节假日结算、输入视频阶梯费率/最低用量、OpenAI凭据与价格、个人积分钱包。本轮新增供应商调用0次、派发总数仍3；原manual的0.12元预留不变。

### 2026-11-03 SRC-AI-37 界面增量

报价、服务接入和模型管理按即梦实页重新分层，公共Select长文本与移动端样式同步修复；Figma先行，代码落盘未提交。执行证据见Plan29/30本轮回执及[研究记录](docs/research/2026-11-03-即梦报价与模型接入交互拆解.md)。本轮未执行收费调用、数据库迁移或新价格发布；T-AI-21/22/23/25既有未接受项与合同依赖保留。

### 2026-11-03 FR-AST-09 删除反馈精简

生成历史删除回执改为全局Sonner Toast，全部成功退出批量，部分失败保留选择与原请求恢复；主体、参考素材、音色、账号和项目/画布删除同步。Figma公共提示与删除成功页已更新并回读；真实资产页与合成删除浏览器检查完成，实现与验证完成，Git提交以仓库记录为准。工程验证与边界见[Plan22本轮回执](docs/plan/22-资产对齐即梦实施计划.md#2026-11-03-ac-ast-09-删除toast执行)及仓库外output/delete-toast-2026-11-03。

### 2026-11-03 CR-076 顶栏尺寸

公共顶栏工具组与账号组已统一桌面36px/手机50px，手机保留44px触点，访客状态同步。Figma先行，1512/1024/390/320实页与批量切换检查完成；实现与验证完成，Git提交以仓库记录为准。来源与证据见设计29§7.16、PRD16 CR-076及Plan25对应回执。
