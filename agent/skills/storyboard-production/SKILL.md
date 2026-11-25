---
name: storyboard-production
description: 根据已确认剧本生成分镜提案并在审批通过后提交单镜/批量渲染任务
allowed-tools: read_project_script read_project_bible read_canvas_snapshot submit_storyboard_generation
---

# Storyboard Production Skill

1. 通过 MCP 读取最新剧本、角色主体参考与当前画布快照。
2. 先产出镜头列表与画面提示词，涉及收费渲染前必须进入审批挂起点（HITL suspend）。
3. 仅在收到明确 `approve` 决策后调用 `submit_storyboard_generation`。
