# 切片 B：受限 Agent 提案 PRD

> PRD ID：PRD-B
> 上游：切片 A；M03、M04、M05、M06、M11、M14
> 状态：proposed

## 1. 用户问题与结果

手工闭环可用后，用户希望更快获得结构、实体和镜头首稿，同时逐项理解并接管。切片 B 验证“Agent 生成提案，业务命令产生事实”，不建设自治生产工作流。

## 2. P0 场景

- 对批准来源提出 Scene/Beat/Mention 校对建议；
- 对人物身份、档案、关键状态和地点/道具提出建议；
- 对选定 Beat 提出镜头方案并显示覆盖；
- 对阻塞和影响提供解释；
- 用户逐项接受、修改、拒绝、暂缓；
- 最小关系/影响视图定位实体—镜头—参考并跳转表单。

## 3. Agent 卡片必须显示

动作类型、目标/基线、字段差异、原文证据、未知项、影响、资源上限、数据外发、权限、是否有副作用和过期条件。解释性文本不得显示为“已完成”。

## 4. 范围与边界

- 顶层 `agent/` 中独立但仅内网运行的 Python FastAPI/LangGraph Agent Service、checkpoint、版本化 AgentRun Request/Result；不提供公共 API/Ingress；
- Go `backend/` 创建 AgentRun、从平台唯一 Kafka 集群接管后台 Task 后通过私有 start/get/cancel 契约调用 Agent、校验结果并保存 Proposal；Python Agent 不连接 Kafka/Redis/Elasticsearch，不持有业务数据库或通用 MinIO 凭据；剧本检索只能调用 run-scoped backend Search Tool；
- 只读 Tool 白名单和结构化 ProposalItem；
- Agent token/tool 用量记录和 M14 外发评估；
- 不允许 Agent 直接写表、自动批准事实、启动媒体生成、选主候选、接受风险或交付；
- 完整空间画布、多人布局、长期聊天记忆延期到 F/独立 Gate。

## 5. 降级和失败体验

模型不可用/超时时手工页面继续可用；部分 Tool 失败逐项显示；基线变化使提案 expired；重复/非法/越权输出副作用为零；Worker 重启从 checkpoint 恢复同一 Run。

## 6. 验收与退出

1. 接受 Proposal 与手工提交相同命令得到相同领域结果；
2. 10 项镜头提案可分别接受、修改、拒绝；
3. 非法、重复、越权、过期输出不改变业务事实；
4. AgentRun 可恢复且不重复 Proposal；
5. 关闭 Agent 后切片 A 和已接受结果正常可用；
6. 最小接管视图不保存第二套业务关系；
7. Agent 外发和用量可追踪到 run/model/tool。
8. frontend/第三方不能直连 Agent；部署清单只有一个 Kafka 集群且只有 backend 连接，停止 Agent 不影响其他 Topic 的 Go Worker。

退出后才允许 Agent 提议媒体 GenerationPlan；实际媒体仍在切片 C。
