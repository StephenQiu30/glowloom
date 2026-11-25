# AI 短剧制作三层 Agent 与六阶段 Workflow 设计

状态：已确认并进入实施（依据 [需求34 SRC-SDMA-01](../requirement/34-AI短剧制作多Agent编排需求.md#src-sdma-01)）。本设计建立在 [设计48：Agent服务Mastra架构与技术选型设计](48-Agent服务Mastra架构与技术选型设计.md) 的 Mastra + 强类型 gRPC + Go 内部 MCP + Go 统一渠道网关底座之上，参考 **Toonflow** 与 **BeefTV** 开源架构，定义 9:16 竖屏 AI 漫剧/动画短剧的多 Agent 协作、六阶段 HITL 流水线、MCP 工具契约与专项 Evals。

## 1. 摘要

针对 9:16 竖屏 AI 漫剧“多集易吃书、人物易漂移、批量渲染费积分、纯自动难精修”四大痛点，本设计结合 Toonflow 与 BeefTV 的实践：

- **三层 5 角色多 Agent**：决策层 `Producer`、执行层 `ScriptAgent` / `AssetDirectorAgent` / `ProductionAgent`、监督层 `ContinuityReviewerAgent`。
- **双交互模式并存**：全集推进走 **Mastra 六阶段可挂起恢复 Workflow**（产物自动同步无限画布）；局部精修/单镜重抽走 **Producer / ProductionAgent 副驾驶模式**直接操作画布选中节点。
- **双层一致性 + 分级动态化**：先锁主体三视图（`subject_id`）与角色音色（`voice_id`），批量分镜默认 **2x2 宫格出图切分放大**，视频采用 **首尾帧 I2V + 特写按需 LipSync + 多 Take 显式 `adopt_shot`**。
- **可控成本与防死循环**：文本阶段由监督层自动审校（最多 2 轮修订），三视图 / 2x2 宫格 / I2V 与配音三大收费阶段分别汇总报价单触发 `suspend()` 审批，收费 MCP 工具强制校验审批 `decision_id`。

## 2. 参与者

| 参与者       | 职责                                                                  |
| ------------ | --------------------------------------------------------------------- |
| 短剧创作者   | 提供创意/小说/剧本，在三大收费里程碑审批报价，在无限画布精修与采用 Take |
| Agent 维护者 | 维护三层 5 角色、六阶段短剧 Workflow、6 套短剧 `SKILL.md` 与离线 Scorers |
| Go 后端维护者 | 维护 `/internal/agent-mcp` 5 组短剧原子工具、乐观锁与预算/审批权威校验 |

## 3. 背景

Glowloom 已完成 `agent/` 的 Mastra 底座重构（[设计48](48-Agent服务Mastra架构与技术选型设计.md)），并拥有 Go 侧 `creation/{script,bible,subject,canvas,document}` 与 `execution/gateway`。本设计把短剧制作业务能力正式装配到新底座中，避免单 Agent 长上下文漂移与无审批扣费风险。

## 4. 目标

- 在 `agent/src/mastra/` 落地三层 5 角色与六阶段短剧 Workflow（含 3 个 `suspend/resume` 审批点与 ≤2 轮审校上限）。
- 在 `agent/skills/` 落地 6 套短剧声明式 `SKILL.md` 并绑定严格 MCP 工具白名单。
- 在 `agent/evals/` 落地短剧专项 Mastra Scorers（主体绑定率、未审批零扣费、监督层打回收敛）。
- 在 Go `backend/internal/agent/adapter/http/` 注册 5 组短剧原子 MCP 工具，强制乐观锁与收费审批凭证校验。

## 5. 使用场景

| 场景                 | 模式               | 关键链路                                                                     |
| -------------------- | ------------------ | ---------------------------------------------------------------------------- |
| 整集漫剧从零生产     | 六阶段 Workflow    | 创意/网文 → 事件图谱 → 剧本审校 → 审批三视图 → 审批 2x2 宫格 → 审批 I2V → 导出 |
| 画布单镜重抽或改运镜 | 无限画布副驾驶模式 | 创作者圈选画布节点 → `ProductionAgent` 读取邻镜上下文 → 单镜重抽（视同授权）   |
| 跨重启审批恢复       | 持久化 `resume()`  | 三大收费阶段挂起后容器重启 → 创作者批准 → 按 `workflow_run_id` 无损续跑       |

## 6. 价值

- **人物与剧情连贯性大幅提升**：章节事件图谱解决长剧遗忘，三视图 + 2x2 宫格解决画面漂移。
- **算力与积分损耗可控**：监督层在花钱前拦截坏分镜，三大收费点先报价后生成，单镜重抽不牵连整组宫格。

## 7. 方案

### 7.1 三层 5 角色职责划分（已确认）

| 层级   | 角色 ID               | 核心职责                                                           |
| ------ | --------------------- | ------------------------------------------------------------------ |
| 决策层 | `producer`            | 意图路由、选择 Workflow 或画布副驾驶模式、协调子 Agent 与审批收口   |
| 执行层 | `scriptwriter`        | 原著/创意摄入、提取章节事件图谱（Event Graph）、分集大纲与可拍摄剧本 |
| 执行层 | `asset_director`      | 抽取角色/场景/道具 Bible、生成三视图提示词、锁定 `subject_id`/`voice_id` |
| 执行层 | `production`          | 分镜表构建、2x2 宫格提示词编排、画布节点连线、I2V/配音/LipSync 调度 |
| 监督层 | `continuity_reviewer` | 独立只读审校：检查剧情连贯、主体引用覆盖、服化道一致性与预算合规    |

### 7.2 六阶段短剧 Workflow 状态机（已确认）

```mermaid
flowchart LR
  S1[1. 事件图谱提取<br/>extract-event-graph] --> S2[2. 剧本编排与监督审校<br/>write-and-review-script<br/>最多2轮修订]
  S2 --> S3[3. 主体三视图锁定<br/>lock-character-assets<br/>HITL suspend #1]
  S3 --> S4[4. 2x2宫格分镜出图切分<br/>generate-storyboard-grid<br/>HITL suspend #2]
  S4 --> S5[5. 分级I2V+配音+按需LipSync<br/>render-video-and-audio<br/>HITL suspend #3]
  S5 --> S6[6. 已采用镜头装配导出<br/>assemble-timeline-export]
```

### 7.3 Go 内部 MCP 短剧 5 组原子工具契约（已确认）

1. **剧本与事件图谱**：`script_get_event_graph`、`script_upsert_event_graph`、`script_get_episode`、`script_save_episode`
2. **设定与主体库**：`bible_list_assets`、`subject_list`、`subject_lock_reference`、`voice_bind_character`
3. **无限画布**：`canvas_read_graph`、`canvas_upsert_storyboard_nodes`、`canvas_split_grid_2x2`
4. **多模态与采用**（必须校验审批 `decision_id` 或显式单镜重抽标记）：`media_estimate_quote`、`media_submit_image`、`media_submit_video_i2v`、`media_submit_tts`、`media_submit_lipsync`、`media_get_task`、`shot_adopt_take`
5. **时间线与导出**：`timeline_assemble_adopted_shots`、`timeline_export_render`

### 7.4 6 套声明式短剧 Skill（已确认）

存放于 `agent/skills/*/SKILL.md`：`novel-to-event-graph`、`vertical-short-drama-script`、`character-turnaround-sheet`、`grid-2x2-storyboard`、`i2v-motion-prompting`、`continuity-review`。

## 8. 实施与验收

验收标准见 [PRD35](../prd/35-AI短剧制作多Agent编排.md)，执行任务与验证命令见 [Plan44](../plan/44-AI短剧制作多Agent编排执行计划.md)。

[设计导航](00-设计导航.md) · [需求34](../requirement/34-AI短剧制作多Agent编排需求.md) · [PRD35](../prd/35-AI短剧制作多Agent编排.md)
