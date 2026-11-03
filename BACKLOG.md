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
