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

| 任务编号 | 模块职责 | 核心交付代码路径 | 验证状态 |
| :--- | :--- | :--- | :--- |
| **T-AH-01** | **StateGraph 引擎与检查点** | `backend/internal/agent/domain/graph/{engine,state}.go`<br>`backend/internal/agent/adapter/postgres/checkpointer.go`<br>`backend/db/agent_checkpoint.sql` | **已完成 (Green)**<br>状态图条件边拓扑、步数看门狗、PG 快照持久化与断点接续测试全量通过 |
| **T-AH-02** | **Supervisor 统筹与提案中断** | `backend/internal/agent/application/workers/supervisor.go`<br>`backend/internal/agent/application/proposal_interrupt.go` | **已完成 (Green)**<br>目标分解、依赖调度、提案中断挂起与用户批准/驳回接续测试全量通过 |
| **T-AH-03** | **统一网关与流式看门狗** | `backend/internal/agent/application/model_gateway.go`<br>`backend/internal/agent/application/model_stream_watchdog.go`<br>`backend/internal/agent/application/turn_budget.go` | **已完成 (Green)**<br>参数冻结、上下文窗口校验、45s SSE 空闲检测与 40 步硬熔断测试全量通过 |
| **T-AH-04** | **业务工具、写链与现场自愈** | `backend/internal/agent/application/tools.go`<br>`backend/internal/agent/application/write_chain.go`<br>`backend/internal/agent/application/studio_digest.go` | **已完成 (Green)**<br>连续写链版本自增、CAS 冲突拦截与 `<canvas_changes>` 差异注入测试全量通过 |
| **T-AH-05** | **Skill 生命周期与动态装载** | `backend/internal/skill/domain/skill.go`<br>`backend/internal/skill/application/skill_registry.go` | **已完成 (Green)**<br>版本固定、SHA-256 哈希、路径穿越拦截、`skill_load` 展开与上下文重置测试全量通过 |
| **T-AH-06** | **专业 Worker 智能体族** | `backend/internal/agent/application/workers/script_worker.go`<br>`backend/internal/agent/application/workers/storyboard_worker.go`<br>`backend/internal/agent/application/workers/shot_worker.go`<br>`backend/internal/agent/application/workers/audio_worker.go` | **已完成 (Green)**<br>剧本拆解 → 分镜排布 → 镜头提案挂起 → 审批恢复 → 音频后期全流程流水线接力测试全量通过 |
| **T-AH-07** | **隔离沙箱与安全脚本执行** | `backend/cmd/glowloom/sandbox.go`<br>`backend/internal/skill/domain/sandbox.go`<br>`backend/internal/skill/application/sandbox_runner.go` | **已完成 (Green)**<br>`glowloom sandbox serve` 子命令就绪，超时强制终止与环境变量安全隔离测试全量通过 |
| **T-AH-08** | **SSE 事件流与前端卡片交互** | `backend/internal/agent/adapter/http/stream_handler.go`<br>`frontend/src/components/agent/ProposalApprovalCard.tsx`<br>`frontend/src/components/agent/index.ts` | **已完成 (Green)**<br>SSE 标准事件推送、Next.js 提案审批卡片与参数确认组件全量通过 TypeScript 类型检查 |

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

**本轮文档交付：已完成拆解与文档校验；未实施代码，未运行业务/真实供应商验证，未迁移或发布。** 设计评审、商业政策接受、代码实现、实际供应商和产品接受分别记录。

来源：[调研](docs/research/2026-11-03-BeefTV模型接入与计费调研.md)与[需求04 SRC-AI-35](docs/requirement/04-AI平台接入与预设管理需求.md#src-ai-35)。

| 父任务 | 唯一设计→PRD→Plan | 当前状态 | 优先级建议/依赖 |
| --- | --- | --- | --- |
| T-AI-21 接入声明与协议扩展 | [设计33](docs/design/33-多设计模式驱动的模型与供应商接入扩展体系设计.md) → [PRD20](docs/prd/20-模型与供应商接入扩展.md) → [Plan29](docs/plan/29-模型与供应商接入扩展实施计划.md) | 评审稿，专项未实施 | P2；复用已存在驱动/预设，先定声明合同 |
| T-AI-22 计价与费用说明 | [设计34](docs/design/34-积分消耗系统与模型真实计价体系设计.md) → [PRD21](docs/prd/21-模型计价透明与结算恢复.md) → [Plan30](docs/plan/30-模型计价透明与结算恢复实施计划.md) | 评审稿，专项未实施 | P1；D-AI-35-01/02影响对应功能 |
| T-AI-23 结算证据与恢复 | 设计34→PRD21→Plan30 | 评审稿，专项未实施 | P1；保留unknown保护，和现有operation/billing对接 |
| T-AI-24 Agent统一网关 | 设计32→PRD19→Plan28 | 沿前文Agent主链；本节不重置其状态 | 仅依赖费用授权/原operation身份，不重复计算交付 |
| T-AI-25 错误与原币对账 | 设计34→PRD21→Plan30 | 评审稿；原币部分受政策与供应商合同约束 | P2；25-01可独立核查，25-02依赖D-AI-35-03 |

### 6.1 可执行任务

以下均为后续实施候选，具体责任人与工期未指定，无排期或交付承诺；“待评审”不是代码不存在的判断，已有基础需按Plan复用。

| 任务ID | 最小交付 | 依赖 | 当前状态/下一步 | 验收 |
| --- | --- | --- | --- | --- |
| T-AI-21-01 | 公开安全元数据与生成API | 设计33字段评审 | 待评审；固定字段与权限失败用例 | CASE-AIX-04 |
| T-AI-21-02 | 受限声明及预设事务版本 | 21-01 | 待评审；选择同协议最小声明 | CASE-AIX-01/02/03/05 |
| T-AI-21-03 | 协议组合与单次派发 | 21-02 | 待评审；建立纳入协议fixture矩阵 | CASE-AIX-03/06 |
| T-AI-21-04 | 极简元数据界面与联合接受 | 21-01～03 | 待评审；先组件回归再界面 | CASE-AIX-01～06 |
| T-AI-22-01 | 价格来源/版本与金额表达 | D-AI-35-01 | 待评审；确认政策出处，保留微元算法 | CASE-BIL-01/03/04 |
| T-AI-22-02 | 参考图计数及收费scope | 22-01、D-AI-35-02 | 待合同；取得逐模型一手计量规则 | CASE-BIL-02 |
| T-AI-22-03 | 预估/预留摘要和明细 | 22-01 | 待评审；不依赖未开放计价方式 | CASE-BIL-01～04 |
| T-AI-23-01 | 独立费用/交付读投影 | 22-01 | 待评审；固定缺失/空串/零及读失败 | CASE-BIL-06～11 |
| T-AI-23-02 | 服务端屏障及原任务恢复 | 23-01、既有workflow control | 待评审；锁内并发与零新增生成测试 | CASE-BIL-05/06/08/11/12 |
| T-AI-23-03 | 恢复UI与Agent回放联合接受 | 23-01/02、22-03 | 待评审；复用T-AI-24，不增第二网关 | CASE-BIL-05～12 |
| T-AI-25-01 | 配额/权限/速率安全分类 | 23-01 | 待评审；同HTTP状态不同证据反例 | CASE-BIL-09/11/12 |
| T-AI-25-02 | 原币账单与迟到退款补偿 | D-AI-35-03及真实验证授权 | 待合同；未关闭不开放，禁止Settle(0)改旧账 | CASE-BIL-03/07/10 |

### 6.2 待确认与外部条件

| ID | 待决事项 | 当前处理与关闭条件 |
| --- | --- | --- |
| D-AI-35-01 | 1:10积分/默认1.0的对外政策出处、分数积分显示、多级倍率覆盖 | 保留现有代码默认，不作为新商业承诺；由用户/业务补明确来源并更新需求确认 |
| D-AI-35-02 | 参考图/重复引用/mask/每输出计数、缓存/视频Token单位 | 逐模型取得供应商一手合同；未定义的附加费不发布 |
| D-AI-35-03 | 原币最终账单、超预留承担、退款补偿及采购/销售分账 | 明确账单身份、冻结汇率、超额政策、迟到退款流水与授权；原币路径保持关闭 |
| D-AI-35-04 | 真实供应商/浏览器/迁移接受 | 仅隔离工程验证可规划；实际运行与收费另获授权并单独留证 |

### 6.3 下一步与完成判据

1. 评审设计33/34及两份PRD，关闭影响所选实现切片的政策/合同；不要求无关后续计价方式一并上线。
2. 在明确授权实施后，按Plan29/30先Red→最小实现→Green，保留既有价格版本、账本和原任务身份。
3. 对每项分别记录提交/环境、命令、结果及未覆盖项；fixture、隔离数据库、浏览器、供应商、产品接受和发布不混为通过。
4. 完成条件为对应CASE与权限/并发证据齐全，不以“文档已同步”或旧Agent测试通过代替。
