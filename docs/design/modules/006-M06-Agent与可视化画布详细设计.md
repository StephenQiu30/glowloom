# M06 Agent 与可视化画布详细设计

> Design ID：DES-M06
> Requirement：[REQ-AIC](../../requirement/006-M06-Agent与可视化画布需求.md)
> 状态：proposed

## 1. 设计结论

LangGraph 只编排一次 AgentRun 内部推理；业务 Operation、Proposal、批准和 current 状态仍在 Python 应用层/PostgreSQL。Agent、列表和画布提交相同类型化命令，画布布局与真实依赖分库存储。

## 2. AgentRun 输入与输出

`AgentRunRequest` 固定 workspace/project/scope、actor、基线版本、允许 Tool、模型、usage cap、M14 governance evaluation、graph/schema version 和 trace。Runtime State 只保存小型引用与节点结果引用，不复制剧本/媒体。

`AgentRunResult` 包含 sequence、result hash、ProposalItems、evidence refs、model/tool usage、errors 和 resume metadata。ProposalItem 只能引用公开命令 Schema，不能携带 SQL/ORM patch。

## 3. LangGraph 节点约束

```text
load_authorized_context
  → deterministic_checks
  → model_reasoning
  → validate_structured_output
  → enrich_evidence_and_impact
  → persist_result
```

所有 Tool 只读且由短期 capability token 限定 run、scope、tool 和 expiry。需要写入时只生成 Proposal；接受 Proposal 由 API 重新鉴权、检查 expected revision、幂等和领域门禁。

## 4. 媒体与推理边界

AgentRun 只做分析/解释/提案。任何图像、视频、语音或音乐生成必须创建 M07 GenerationPlan；M08 执行，M09 摄取。Agent 模型 Token/Tool/本地计算由 M11 归因，数据外发由 M14 单独评估。

## 5. 画布设计

CanvasView 保存过滤器、所有者和可见性；CanvasLayout 保存 object ref、位置、尺寸、分组。依赖边来自 M04 可重建投影，装饰边单独标 type。删除节点只删除布局项；领域删除打开权威表单和影响预览。

切片 B 只交付关系/影响接管视图：实体—镜头—参考—候选—问题选择与跳转；切片 F 才加入自由布局、多人视图和批量命令。

## 6. 状态、幂等与恢复

AgentRun：`queued → running → waiting_user | partial | completed | failed | cancelled | superseded`。同 run/version/result hash 只接收一次；checkpoint 用 `agent_run_id` 派生 thread。基线变化后 Proposal expired，不删除结果。Worker 重启从 checkpoint 恢复，产品状态只由 result port/Operation 更新。

## 7. 失败与安全

- 模型/Tool 部分失败：逐项结果，手工工作台持续可用；
- 非法输出：契约拒绝，业务副作用为零；
- Tool 越权或过期：拒绝并审计，不让模型扩大 scope；
- 画布断线：进入只读/离线提示，重连回源；
- Prompt injection：来源内容视为数据，不改变 Tool 白名单和系统策略；
- 敏感上下文：最小字段、受控日志、checkpoint 生命周期随 Run。

## 8. 验证

Fake model/tool 测试图路由、非法输出、重复结果、每节点重启、过期基线和关闭 Agent；契约测试保证接受提案与手工命令同结果。映射 AIC-FR-001—015、AIC-NFR-001—006、AC-AIC-001—009。
