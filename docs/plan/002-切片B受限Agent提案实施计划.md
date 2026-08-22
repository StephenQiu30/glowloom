# 切片 B：受限 Agent 提案实施计划

> Plan ID：PLAN-B
> PRD：[PRD-B](../prd/002-切片B受限Agent提案PRD.md)
> Design：[M06](../design/modules/006-M06-Agent与可视化画布详细设计.md)及 M03—M05/M11/M14
> 状态：proposed

## 1. 前置 Gate

切片 A 的 Go 手工命令、Query、expected revision、审计和恢复证据通过；`backend/contracts/agent/` 当前契约、模型 Provider 的数据条款/地域/保留、Agent token 上限和金标提案样本签认。

## 2. 工作包顺序

| WP | 工作 | 验证 |
| --- | --- | --- |
| B1 | Go M06 AgentRun/Proposal/Item、Agent JSON Schema、状态机和结果幂等端口 | Go/Python golden/hash 一致；非法/重复 result 副作用为 0 |
| B2 | 顶层 `agent/` 私有 FastAPI/LangGraph Service、start/get/cancel Run 契约、checkpoint | mTLS/网络负向测试；无 Kafka/Redis/Elasticsearch client、公共 API/Ingress、业务 DB/通用 MinIO 凭据；start 响应丢失按同 run/key/hash 对账；每节点前后重启恢复同一 run |
| B3 | 受限 Query Tool 和 capability token | 越权/过期/跨 workspace 拒绝 |
| B4 | 结构分析 Proposal | 原文证据、逐项决定、过期基线 |
| B5 | 人物/地点/道具 Proposal | 同名/unknown/merge split 人工接管 |
| B6 | 拆镜 Proposal 和覆盖报告 | 接受与手工命令结果等价 |
| B7 | backend ModelCallGate、run-scoped M03 Search Tool、Agent 用量、M14 外发评估、敏感日志约束 | Redis GCRA + M11 Gate 任一阻止时模型调用为 0；Search Tool 强制 run/workspace/project/approved revision filter，stale/unavailable 不解释为零结果；run→model/tool/usage/governance 可追溯 |
| B8 | 最小关系/影响接管视图 | 节点跳转同一表单，删除只改布局 |
| B9 | 手工降级、部分失败和 E2E | 关闭 Agent 后闭环完整 |

## 3. 模型评估

分别统计提案接受、修改、拒绝、证据错误、非法输出和过期率；不以聊天满意度代替。Golden 样本覆盖结构错分、同名人物、隐藏身份、换装、地点/道具和 10+ 镜头拆解。

## 4. 安全/故障验证

Prompt injection、恶意来源指令、Tool 越权、私有 start 响应丢失/get 超时、非 operation-worker 身份、Redis/ModelCallGate 不可用、模型超时、Tool 部分失败、checkpoint 损坏/过期、滚动升级 contract 版本、重复 result、Run 中基线变化、M14 拒绝外发。

## 5. 退出与停止条件

满足 PRD-B 八项退出条件并通过 AC-AIC-001—009。若 Python Agent 需要 Kafka/Redis/Elasticsearch、直接写表、导入 Go Service 内部实现、长期记忆成为批准事实或接受 Proposal 与 Go 手工命令不一致，停止并修正 Design；不通过扩大 Tool 权限解决。
