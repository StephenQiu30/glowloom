# 切片 B：受限 Agent 提案实施计划

> Plan ID：`PLAN-B`
> 上游：[PRD-B](../prd/002-切片B受限Agent提案PRD.md)、[M06 Design](../design/modules/006-M06-Agent与可视化画布详细设计.md)、M03—M05/M11/M14
> 验收：[Acceptance 规范](../acceptance/README.md)
> 状态：`proposed`

## 1. 交付目标与固定范围

交付一个 P0 `script_analysis` 技能，由 Manifest 拆解、Narrative 理解、Knowledge 提取三个受控 AgentRun 组成。Agent 只读取受限上下文并输出带证据的 Proposal；用户逐项接受、编辑、拒绝或暂缓，只有 Go backend 的既有 Command 能写入业务事实。

P0 不包含 Agent 自动拆镜、自动生成、长期记忆、通用聊天、公共 Agent API 或画布业务编辑器。镜头仍沿用切片 A 的手工命令；如未来增加拆镜提案，必须先形成新的 PRD 范围和验收条件。

## 2. 入口 Gate

- 切片 A 为 `verified`，手工 Command/Query、expected revision、审计和恢复证据可复现；
- `backend/contracts/agent/` 当前契约、JSON Schema 和 Go/Python canonical hash 规则签认；
- 模型 Provider 的地域、保留、训练、敏感数据和删除条款通过治理评审；
- Token/资源硬上限、可外发字段、金标提案数据和 Prompt injection 样本签认；
- Agent 仅在私有网络由 backend 服务身份调用，无公共 Ingress，且不持有业务库、Kafka、Redis、Elastic 或通用 MinIO 凭据。

## 3. 工作包与依赖

| WP | 可交付结果 | 依赖 | 覆盖范围 | 验证与预期证据包 |
| --- | --- | --- | --- | --- |
| B0 | 冻结 `script_analysis`、三个 stage、AgentRun/Proposal/Item 状态机和 Go/Python 契约 | A verified | PRD-B-FR-001/002 | JSON Schema、golden/hash、非法转换；`EPK-B-CONTRACT` |
| B1 | backend 建立 Run 编排、stage 人工 Gate、幂等 start/get/cancel 对账和结果摄取 | B0 | PRD-B-FR-001/002/007 | start 响应丢失、重复 result、child/root 状态；`EPK-B-ORCHESTRATION` |
| B2 | 顶层 Python Agent 服务实现三个 checkpointed workflow 和结构化 Proposal 输出 | B0、B1 | PRD-B-FR-001/003 | 节点前后重启、超时、部分失败、无禁止依赖；`EPK-B-RUNTIME` |
| B3 | backend 提供 run-scoped Query/Search Tool、capability token 和最小必要上下文 | B1、B2 | PRD-B-FR-004 | 越权/过期/跨租户/伪造身份、stale/unavailable；`EPK-B-TOOLS` |
| B4 | Manifest Proposal 带原文 Anchor，用户逐项决定后才物化 ContentUnit | B1—B3 | PRD-B-FR-002/003/005 | 边界/标题提案、逐项决定、过期和人工接管；`EPK-B-MANIFEST` |
| B5 | Narrative Proposal 带原文证据，用户批准后才进入 Knowledge stage | B4 | PRD-B-FR-002/003/005 | Scene/Beat/Mention 提案、Gate 顺序、partial；`EPK-B-NARRATIVE` |
| B6 | Knowledge Proposal 覆盖人物/地点/道具/服装与消歧，接受操作复用 Go 手工 Command | B5 | PRD-B-FR-002/003/005 | 同名/unknown/merge/split、手工结果等价；`EPK-B-KNOWLEDGE` |
| B7 | Proposal 卡片和最小关系视图支持接受、编辑、拒绝、暂缓、证据回跳和布局操作 | B4—B6 | PRD-B-FR-005/008 | 每 item 独立 read-set；布局变化不改领域事实；`EPK-B-TAKEOVER` |
| B8 | ModelCallGate、Redis GCRA、用量、外发评估、敏感日志和审计完整接入 | B1—B3 | PRD-B-FR-006/009 | 任一门禁阻止时模型调用为 0；run→tool/model/usage 可追溯；`EPK-B-GOVERNANCE` |
| B9 | 关闭 Agent/模型/Redis/Tool 后，用户从失败 scope 回到 A 手工主线 | B4—B8 | PRD-B-FR-007、PRD-B-AC-006 | 降级 E2E、已接受事实保留、未接受提案不写事实；`EPK-B-DEGRADE` |
| B10 | 执行身份、注入、幂等、基线变化和进程恢复的完整故障矩阵 | B1—B9 | PRD-B-AC-001—009 | 对抗数据集与故障注入记录；`EPK-B-SECURITY-RECOVERY` |

## 4. 执行顺序与边界

主链为 `B0 → B1/B2 → B3 → B4 → B5 → B6 → B7 → B9 → B10`；B8 从第一次真实模型调用前开始，贯穿 B4—B6。三个 stage 不并行跨越人工 Gate。Agent 不能发布 Kafka 消息、访问 Redis/Elastic、写业务库或直接调用 Go Service 内部实现；所有写入都回到同一 backend Command。

## 5. 评估与故障策略

| 类别 | 数据/故障 | 记录指标 |
| --- | --- | --- |
| 提案质量 | 结构错分、同名人物、隐藏身份、换装、地点和道具 | 接受率、修改率、拒绝率、证据错误率、非法输出率 |
| 基线并发 | 同批不相交 item、真实 read-set 冲突、批准中 Revision 变化 | 精确过期率、误过期率、重复副作用 |
| 运行恢复 | start 响应丢失、get 超时、checkpoint 前后崩溃、重复 result | Run/Proposal 唯一性、恢复时长、人工接管率 |
| 安全 | Prompt injection、恶意来源指令、Tool 越权、伪造身份、M14 拒绝 | 越权成功数必须为 0；被阻模型调用为 0 |
| 降级 | Agent/模型/Redis/Search Tool 分别不可用 | A 手工主线完成率必须为 100% |

聊天满意度、自然语言流畅度或单次 Demo 不作为 P0 通过标准。

## 6. 验收条件追溯

| PRD 验收条件 | 负责工作包 | 预期证据包 |
| --- | --- | --- |
| PRD-B-AC-001 | B1、B4—B6 | `EPK-B-ORCHESTRATION` |
| PRD-B-AC-002 | B4—B7 | `EPK-B-TAKEOVER` |
| PRD-B-AC-003 | B0、B3、B10 | `EPK-B-SECURITY-RECOVERY` |
| PRD-B-AC-004 | B7、B10 | `EPK-B-TAKEOVER` |
| PRD-B-AC-005 | B1、B2、B10 | `EPK-B-RUNTIME` |
| PRD-B-AC-006 | B9 | `EPK-B-DEGRADE` |
| PRD-B-AC-007 | B2、B3、B10 | `EPK-B-TOOLS` |
| PRD-B-AC-008 | B7 | `EPK-B-TAKEOVER` |
| PRD-B-AC-009 | B8、B10 | `EPK-B-GOVERNANCE` |

## 7. 退出与停止条件

九条 PRD-B 验收条件及 AC-AIC-001—009 的当前范围全部有 `passed` 证据后，B 才可标为 `verified`。出现下列情况立即停止扩展 Agent：

- Python Agent 需要 Kafka、Redis、Elastic、业务库或公共入口；
- Agent 直接写业务事实，或接受 Proposal 与 Go 手工 Command 结果不等价；
- 长期记忆、checkpoint 或搜索结果被当作已批准事实；
- 通过扩大 Tool 权限、跳过人工 Gate 或批次级粗锁解决失败；
- 模型被治理/资源门禁阻止后仍发生外调。

回退时可停用 Agent 路由并保留 Run/Proposal 审计；已由用户接受并通过 Go Command 形成的事实不回滚。
