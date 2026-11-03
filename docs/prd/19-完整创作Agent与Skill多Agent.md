# 完整创作 Agent、Skill 与多 Agent PRD

状态：已确认设计方案；唯一来源设计：[设计32](../design/32-完整创作AgentHarness与Skill多Agent架构设计.md)。技术路线为 **Go 后端内原生统一实现（基于 LangGraph StateGraph 状态图编排，吸收 BeefTV 工业级 Harness 机制）**，废弃独立的 Node.js 宿主，拒绝过度设计。范围定义见[需求19](../requirement/19-完整创作Agent与Skill多Agent需求.md)；实施任务见[Plan28](../plan/28-完整创作Agent与Skill多Agent实施计划.md)，当前进度只见 [BACKLOG](../../BACKLOG.md)。

## 1. 问题、用户与价值

创作者需要从简单点子或长篇剧本出发，产出包含结构化角色/场景、分镜动作、高质量镜头、配音音乐与可二次编辑时间线的完整短剧工程。现有单图 Agent 受限于三阶段硬编码与 20 轮上下文截断，无法承接长流程与多专业协同。

本方案在 Go 后端进程内提供自建完整创作 Agent 服务：通过 LangGraph StateGraph 组织会话状态机，以 Supervisor-Worker 模式编排剧本、分镜、导演与剪辑专业智能体；吸收 BeefTV 验证的“提案即截断挂起”、“多步写链 CAS 延续”、“现场快照自愈”与“单轮步数看门狗”，让智能体行为可靠、安全、可恢复。

## 2. 完整范围与非目标

范围包含：
1. **LangGraph 状态图 Harness**：Go 原生 StateGraph、循环调度、工具调用分发、PostgreSQL Checkpointer 状态持久化与断点接续；
2. **人机协同中断与 CAS 双版本校验锁（Proposal Yield & Dual CAS Lock）**：关键提案或计费调用主动触发中断挂起（Interrupt），Proposal 强绑定 `ExpectedCanvasRevision` 与 `ExpectedPriceRevision`，防止并发覆写与过期计价；
3. **多步写入链与现场自愈**：单轮多工具连续调用版本延续（WriteChain），执行前差异比对（StudioDigest）自愈冲突；
4. **Supervisor 多 Agent 协作**：统筹协调制作人与 4 类专业 Worker Agent（剧本解析、分镜编排、镜头导演、后期剪辑）；
5. **Prompt Caching 优化与项目预算硬封顶**：支持长上下文前缀缓存（Prompt Caching）大幅降低 Token 计费，并在 Run 状态机引入 `ProjectBudgetCap` 硬封顶熔断；
6. **Skill 包与受控执行**：按需动态装载（`skill_load`）、版本固定与沙箱隔离执行；
7. **短剧工程交付与回放**：剧本拆解提案、分镜板、合成时间线与 FCPXML 工程导出。

非目标：不建设第二套独立的 Node.js 微服务；不直接开放宿主机 shell；不把普通聊天对话包装为完整创作；不自动代用户执行未批准的计费生成。

## 3. 产品路径与状态

- 路径：`创建/导入目标 → Supervisor 拆解计划 → 规划审批 → 多 Worker 并行推进 → 提案中断挂起 → 用户确认 → 产物验收 → 成片导出`
- 核心状态机：
  - `admitted`（已受理）$\rightarrow$ `planning`（规划中）$\rightarrow$ `suspended_for_approval`（挂起等待用户审批）$\rightarrow$ `running`（多 Agent 执行中）$\rightarrow$ `completed`（全部达成）
  - 异常状态：`paused`（手动暂停）、`conflict_detected`（现场差异待自愈）、`stopped`（用户终止）。

## 4. 用户故事

- **US01 目标与计划确认**：创作者输入长剧本或创作大纲，Supervisor 快速生成包含依赖和产物预期的计划清单；规划阶段不发起付费生成。
- **US02 提案中断与人工把关**：当镜头导演 Agent 构造好视频生成参数与报价后，调用提案工具立即触发中断挂起，界面弹出计费卡片。创作者确认前，助手不会自行盲目推进后续动作。
- **US03 协同冲突与自愈自适应**：当协作者在画布上修改了某角色的服装描述，Agent 在下一次动作前通过 `<canvas_changes>` 差异块感知到变动，智能调整后续分镜提示词，无需人工打断。
- **US04 连续分镜批量编辑**：Agent 单轮内连续创建多个镜头时，WriteChain 自动接续版本链头，既保证批量连贯执行，又防止与外部并发编辑冲突。
- **US05 异常崩溃与断点恢复**：系统重启或断线后，状态无缝从 PostgreSQL Checkpoint 恢复，已发生副作用不重复执行，挂起提案安全保留。

## 5. 可观察接受条件与需求追踪

| 接受 ID  | 必须可观察的结果                                                        | 需求 ID     | 对应场景    |
| -------- | ----------------------------------------------------------------------- | ----------- | ----------- |
| AC-AH-01 | 同一目标有稳定 Run、版本化计划、明确审批边界与安全中断挂起路径          | AGH-001/003 | ST-AH-01/02 |
| AC-AH-02 | Go 原生 LangGraph (StateGraph) 驱动循环，PostgreSQL Checkpointer 持久化 | AGH-002/004 | ST-AH-01/20 |
| AC-AH-03 | 具有独立上下文的实际子 Agent，Supervisor 幂等委派、结果回收与受限并发    | AGH-006/008 | ST-AH-03/04 |
| AC-AH-04 | 提案即中断与 CAS 锁：Proposal 携带 ExpectedCanvasRevision 与 ExpectedPriceRevision；审批接续时校验版本，变更时触发陈旧冲突拦截 | AGH-003/004 | ST-AH-02/14 |
| AC-AH-05 | 写入链（WriteChain）：单轮多工具连续写操作 CAS 顺畅递增，无假冲突       | AGH-004/018 | ST-AH-17    |
| AC-AH-06 | 现场快照自愈（StudioDigest）：比对最新差异 `<canvas_changes>` 注入模型   | AGH-005/018 | ST-AH-17    |
| AC-AH-07 | 单轮步数看门狗：最大 40 步请求 / 40 步工具强制熔断，防止 LLM 死循环     | AGH-015     | ST-AH-13    |
| AC-AH-08 | 长剧本解析定位正确、歧义保留、采用与源版本冲突受控                      | AGH-007     | ST-AH-06    |
| AC-AH-09 | Skill 支持 skill_load 动态展开正文与任务切换时的上下文重置              | AGH-009/011 | ST-AH-07/10 |
| AC-AH-10 | 声明脚本可真实隔离执行，产物校验与未知状态恢复有回执                    | AGH-012     | ST-AH-12    |
| AC-AH-11 | 预算与看门狗：Prompt Caching 优化长上下文计费，项目预算硬封顶（ProjectBudgetCap）熔断，流式空闲保活超时探测 | AGH-005/015 | ST-AH-13/20 |
| AC-AH-12 | 权威 SSE 事件流实时输出（thinking、tool_call、proposal、turn_end）      | AGH-017     | ST-AH-21    |
| AC-AH-13 | 进程被 kill 后，重启能直接从最后一个 Checkpoint 恢复原 State 并继续执行  | AGH-005/022 | ST-AH-20/23 |

## 6. 测试场景

重点测试 Go 原生 LangGraph Harness 的可靠性边界：
- **ST-AH-01 目标到计划状态图流转**：验证 Supervisor 节点输出计划并通过条件边正确推进；
- **ST-AH-02 提案中断（Interrupt）与 CAS 校验验证**：模拟调用生成提案，验证中断挂起、Checkpointer 持久化；模拟并发修改画布或模型调价后执行 Resume，验证 CAS 冲突拦截拒绝；
- **ST-AH-03 WriteChain 连续写链验证**：单轮连调 3 个节点创建工具，验证 `Revision` 链头正确累加且通过 CAS 校验；
- **ST-AH-04 StudioDigest 现场差异注入验证**：修改工程实体，验证模型下一次请求前置包含 `<canvas_changes>`；
- **ST-AH-05 看门狗熔断测试**：构造死循环工具调用，验证到达 40 步被硬性熔断；
- **ST-AH-06 Checkpoint 故障恢复测试**：在 Interrupt 挂起时终止进程，重启调用 Resume，验证 State 完整还原。

## 7. 假设、风险与接受限制

单一进程架构消除了跨网络双宿主脆弱性，但要求 Go 内部的工具调度、内存占用与并发控制严谨合规。全量测试必须在隔离 PostgreSQL 环境下执行真实的并发与中断接续断言，不能以假桩测试通过代替业务验收。
