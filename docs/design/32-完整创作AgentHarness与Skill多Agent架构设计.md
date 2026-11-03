# 完整创作 Agent Harness、Skill 与多 Agent 架构设计

状态：已确认设计方案；确立**在 Go 后端内部统一实现（Go 原生 + LangGraph StateGraph 编排 + 吸收 BeefTV 工业级 Harness 机制）**，废弃独立的 Node.js 双宿主过度设计。唯一需求来源：[需求19](../requirement/19-完整创作Agent与Skill多Agent需求.md)。能力分析：[Agent Harness 与 Skill 能力分析](../research/2026-11-01-AgentHarness与Skill能力分析.md)。

## 1. 摘要

本方案在 Go 后端进程内（`backend/internal/agent`）原生实现工业级创作 Agent Harness 与多 Agent 服务。采用成熟的 **LangGraph (StateGraph) 状态图编排思想**，结合开源 [BeefTV](https://github.com/glanderness/BeefTV) 验证的 Harness 鲁棒性控制（提案即截断中断 `Proposal Yield`、单轮写链 CAS 递增 `WriteChain`、现场快照自愈 `StudioDigest` 及单轮步数看门狗），完全闭环多轮会话、工具调用循环、两阶段人工审批与短剧多专业 Agent 协同。

严格遵循 [PROJECT](../../PROJECT.md)“后端只有一个进程入口 `backend/cmd/glowloom`”原则，无需引入独立的 Node.js 进程、内部 HMAC 签名网关或底层 bytea 日志表，彻底消除双宿主分布式脑裂。状态持久化与恢复直接使用现有 PostgreSQL 关系表与 LangGraph Checkpointer 机制。

## 2. 参与者

创作者提交创作目标、审阅步骤计划、检查分镜与角色并确认生成费用；团队协作者编辑共享工程；团队管理者管控团队模型额度与 Skill；平台管理者负责模型目录治理与系统级保护。系统侧包含制作人协调 Agent（Supervisor）、专业子 Agent 族（剧本解析、分镜设计、镜头导演、后期剪辑）、Go Agent 执行引擎、Temporal（推进外部重型生成与转码工作流）、对象存储与 Yjs 协同。

## 3. 背景

项目曾探索基于外部 Node.js Pi Durable SDK 的独立运行宿主方案，但在实际落地中发现：
1. **双宿主摩擦严重**：为了适配单机桌面端起家的 Node SDK，被迫在 Go 与 Node 之间引入跨网络通信、HMAC 签名、双重模型出口，以及自研的 PostgreSQL bytea 二进制日志与底层 Advisory Lock，构成典型的过度设计；
2. **状态与事实割裂**：用户、权限、画布实体、GORM 事务全在 Go，而记忆和会话状态在 Node，导致两端状态频繁不一致；
3. **违反工程原则**：违背了单一二进制进程的极简部署规范。

因此，业务与技术决策明确：**废弃独立的 `agent-runtime/`，全面回归 Go 原生统一架构**。借力业界成熟的 LangGraph（StateGraph 模式，如 `cloudwego/eino`、`tmc/langgraphgo`）与 BeefTV 的工业级状态机实践，以高内聚、轻量化的方式实现同等甚至更强鲁棒性的 Harness 与多 Agent 能力。

## 4. 目标

完整满足 AGH-001～024 与业务核心诉求：
1. **不从零造轮子**：采用成熟的 LangGraph 图状态机（StateGraph + Checkpointer + Interrupt/Human-in-the-Loop）组织调度；
2. **工业级 Harness 闭环**：
   - **提案即中断（Proposal Yield）**：模型发出生成提议时自然挂起，绝不盲目继续生成未批准的输出；
   - **写入链（Write Chain）**：单轮多工具连续写入时本地累加链头，兼顾顺畅执行与全局 CAS 乐观锁防冲突；
   - **现场快照（Studio Digest）**：请求前自动比对差异 `<canvas_changes>` 注入上下文，支撑协同冲突自愈；
   - **步数与超时看门狗**：单轮最大 40 步请求 / 40 步工具限制，SSE 流式空闲检测保活。
3. **分层多 Agent 协作**：Supervisor-Worker 模式，制作人 Agent 通过统一状态图条件边（Conditional Edges）委派专业 Worker，子任务继承受限范围与预算，支持局部重做与依赖追踪；
4. **单进程极简交付**：所有代码收敛于 `backend/internal/agent/`，数据库仅使用标准关系表，无额外运维包袱。

## 5. 使用场景

1. **长剧本结构化拆解**：用户提交剧本，Supervisor 调度 `script_worker` 拆解分集、场景、人物与台词，发现歧义并生成提案，触发图中断（Interrupt）等待用户确认；
2. **多专业并行委派**：计划批准后，Supervisor 按 DAG 并行委派 `art_worker` 生成角色场景参考、`storyboard_worker` 编排镜头分镜；
3. **生成提案与人机确认**：`shot_worker` 提出视频生成方案，调用提案工具立即触发中断，前端弹出计费确认卡片，用户批准后状态机 `resume` 进入 Temporal 异步任务；
4. **协同冲突感知与自愈**：协作者在画布修改了角色设定，Agent 下一轮迭代通过 `StudioDigest` 感知到 `<canvas_changes>`，自动调整后续分镜提示词；
5. **故障恢复与无缝接续**：进程重启或网络断线后，LangGraph Postgres Checkpointer 自动从断点还原 State，已发生的副作用不重复执行，未决任务保持挂起。

## 6. 价值

消除多语言微服务运维成本与跨网络脑裂风险，以纯正的 Go 语言工程标准实现高可靠、强事务一致性的智能创作引擎。把创作方法（Skill）、专业分工（多 Agent）与业务实体（画布、分镜、计费）在同一进程与数据库事务域内完全拉通，用户可随时接管、编辑、批准或回退。

## 7. 方案

### 7.1 架构拓扑与 LangGraph 编排模型

整个系统完全内嵌于 Go 业务进程中，基于 LangGraph StateGraph 构建统一的执行拓扑：

```mermaid
flowchart TD
    Client[Next.js 前端 / 画布] -->|HTTP / SSE 长连接| Controller[Go Agent Controller]
    Controller -->|读取 / 提交命令| Ledger[Go 数据库控制账本]

    subgraph GoProcess["Glowloom Go 统一进程 (backend/cmd/glowloom)"]
        Ledger --> GraphEngine[LangGraph StateGraph 引擎]

        subgraph GraphTopology["创作状态图 (StateGraph)"]
            SupervisorNode["制作人统筹节点 (Supervisor)"]
            ScriptWorker["剧本解析节点 (Script Worker)"]
            StoryboardWorker["分镜编排节点 (Storyboard Worker)"]
            ShotWorker["镜头导演节点 (Shot Worker)"]
            AudioWorker["剪辑音频节点 (Audio Worker)"]
            ToolsNode["业务工具执行节点 (Tools Node)"]
            InterruptNode["人工审批中断挂起 (Interrupt / Yield)"]

            SupervisorNode -->|路由条件边| ScriptWorker
            SupervisorNode -->|路由条件边| StoryboardWorker
            SupervisorNode -->|路由条件边| ShotWorker
            SupervisorNode -->|路由条件边| AudioWorker

            ScriptWorker --> ToolsNode
            StoryboardWorker --> ToolsNode
            ShotWorker --> ToolsNode
            AudioWorker --> ToolsNode

            ToolsNode -->|检测到提案调用| InterruptNode
            ToolsNode -->|普通工具执行完成| SupervisorNode
        end

        GraphEngine --> Checkpointer[PostgreSQL Checkpointer (状态检查点)]
        ToolsNode --> BusinessBridge[Go 内部业务操作层 (Canvas/Media/Script/Billing)]
        GraphEngine --> ModelGateway[Go 内部统一模型网关]
    end

    InterruptNode -.->|挂起事件 / SSE 推送| Client
    Client -.->|用户批准命令 / Resume| Controller
    BusinessBridge --> Database[(PostgreSQL 业务库)]
```

### 7.2 统一状态契约 (AgentState)

在 Go 中定义强类型的状态图上下文，跨节点流转：

```go
type AgentState struct {
    RunID           uuid.UUID          `json:"run_id"`
    ProjectID       uuid.UUID          `json:"project_id"`
    SessionID       uuid.UUID          `json:"session_id"`
    ActorID         uuid.UUID          `json:"actor_id"`
    CurrentRevision int64              `json:"current_revision"`
    WriteChain      WriteChainState    `json:"write_chain"`       // CAS 链头管理
    Plan            domain.Plan        `json:"plan"`              // 当前创作步骤计划
    Messages        []domain.Message   `json:"messages"`          // 会话上下文历史
    Proposals       []domain.Proposal  `json:"proposals"`         // 待审批提案池
    LoadedSkills    []domain.SkillPin  `json:"loaded_skills"`     // 当前激活的技能
    LastStudioHash  string             `json:"last_studio_hash"`  // 上次画布现场快照哈希
    TurnStepCount   int                `json:"turn_step_count"`   // 单轮请求看门狗计数
    ToolStepCount   int                `json:"tool_step_count"`   // 单轮工具步数看门狗计数
    Status          string             `json:"status"`            // running, suspended, completed
}
```

### 7.3 吸收 BeefTV 的四大工业级 Harness 机制

1. **提案即截断中断 (Proposal Yield & Human-in-the-Loop)**：
   - 借鉴 BeefTV `durable-extensions.mjs` 中的 `canvas.generation.propose` 控制逻辑：
   - 当模型调用任何会产生计费、媒体生成或结构性覆写的工具时，工具执行器标记 `InterruptRequired: true`。
   - LangGraph 图引擎捕获该标记，立即执行 Checkpoint 保存当前 State，并将状态置为 `suspended_for_approval`，**主动截断后续循环**，杜绝未批准前的幻觉输出；
   - 用户在前端完成审批后，控制器触发 `graph.Resume(runID, approvalContext)`，从暂停点顺畅接续。
2. **多步工具 CAS 连续写入链 (Write Chain)**：
   - 借鉴 BeefTV `canvas-turn.mjs` 中的 `extendWriteChain` 模式：
   - 单轮内连续执行多次写工具时，每一步成功在 `WriteChain.Head` 上递增版本，后续工具调用自动使用最新的 `Head` 作为 `ExpectedRevision`；
   - 外部协作者的并发修改被严格拦截在 CAS 边界之外，而 Agent 自身的连续合法操作不会发生版本互锁。
3. **现场差异自愈 (Studio Digest & Canvas Changes)**：
   - 借鉴 BeefTV `studio-digest.mjs`：
   - 在图引擎向模型发起下一次推理前，自动读取项目画布/分镜当前最新状态，与 `LastStudioHash` 进行哈希比对；
   - 若检测到变更，生成 `<canvas_changes>` 差异摘要文本并前置注入到用户输入中，使 Agent 具备协同编辑自愈能力。
4. **单轮预算与空闲超时看门狗 (Turn Budget & Watchdog)**：
   - 限制单轮最大模型请求数 40 次、最大工具步数 40 次，超过阈值强制熔断挂起；
   - 模型网关引入 SSE 分块空闲看门狗（超时默认 45 秒），防止远程供应商网络假死。

### 7.4 Supervisor-Worker 多 Agent 协作模式

1. **制作统筹 Agent (Supervisor)**：
   - 负责全局计划拆解（维护 `plan_update`）、任务进度追踪与专业 Worker 调度；
   - 拥有委派能力，通过图的条件边（Conditional Edges）根据输出决定下一步流向哪一个 Worker。
2. **专业 Worker Agent 族**：
   - **剧本解析 (Script Worker)**：专注长文本解析、台词场景提取与歧义标注，产出不可变的剧本结构提案；
   - **分镜编排 (Storyboard Worker)**：依据剧本和资产，排布镜头机位、时长与动作描述；
   - **镜头导演 (Shot Worker)**：根据分镜提示词、参考图与视频参数构造生成提案；
   - **剪辑后期 (Audio & Timeline Worker)**：处理配音、时间线对齐与合成导出。
3. **子任务隔离原则**：
   - 每个 Worker 在逻辑上共享根 `AgentState`，但使用独立的专属 System Prompt 和受限白名单工具集；
   - 子任务预算严格受控于根审批额度，子 Agent 绝无越权提权可能。

### 7.5 数据库持久化：极简 Checkpointer

抛弃原复杂的 Node 底层 bytea 追加日志表（`agent.runtime_commit`）与 Advisory Lock，直接在 PostgreSQL 中采用标准的 LangGraph Checkpoint 关系表设计：

```sql
CREATE TABLE IF NOT EXISTS agent.graph_checkpoint (
    run_id uuid NOT NULL REFERENCES agent.harness_run(id) ON DELETE CASCADE,
    checkpoint_id uuid NOT NULL,
    parent_checkpoint_id uuid,
    node_name text NOT NULL,
    state jsonb NOT NULL,
    control_revision bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, checkpoint_id)
);
CREATE INDEX IF NOT EXISTS graph_checkpoint_latest ON agent.graph_checkpoint(run_id, created_at DESC);
```

### 7.6 接口与流式通信协议 (SSE)

对外保持极简、统一的 HTTP/SSE 协议：
- `POST /api/projects/:pid/agent-sessions/:sid/runs`：创建并启动运行；
- `POST /api/projects/:pid/agent-runs/:rid/commands`：投递命令（`approve`、`reject`、`pause`、`resume`、`stop`）；
- `GET /api/projects/:pid/agent-runs/:rid/events`：SSE 长连接流式推送事件，标准事件类型：
  - `thinking`：模型深度思维链增量；
  - `text`：正文增量；
  - `tool_call`：工具调用通知（含名称与参数摘要）；
  - `tool_result`：工具执行结果与版本回执；
  - `proposal`：中断挂起并输出待审批卡片；
  - `turn_end`：轮次结束。

## 8. 实施与验收

验收标准全面对齐 PRD19 与 Plan28：
1. **单进程运行证明**：无任何 Node.js 进程、无额外端口监听，后端以 `backend/cmd/glowloom` 统一二进制启动；
2. **Harness 鲁棒性证明**：
   - 提案中断（Interrupt）测试：模型发起提案时精准挂起，无多余调用；
   - 连续写链（WriteChain）测试：多工具连续写操作 CAS 顺畅通过；
   - 看门狗熔断测试：人为模拟循环，到达 40 步强制熔断。
3. **多 Agent 协作证明**：
   - Supervisor 能按剧本 -> 分镜 -> 镜头的顺序正确流转状态并汇总成果；
4. **断点恢复证明**：
   - 在中断挂起（Interrupt）状态下杀死进程，重启后 `resume` 能够无损恢复原 State 并继续运行。
