---
name: grid-2x2-storyboard
description: 参考 BeefTV 实践，将同场连续 4 镜编排为单次 2x2 宫格分镜图并自动切分放大落画布，支持单镜独立重抽
allowed_tools:
  - script_get_episode
  - subject_list
  - canvas_read_graph
  - media_estimate_quote
  - media_submit_image
  - canvas_split_grid_2x2
  - canvas_upsert_storyboard_nodes
---

# 2x2 Grid Storyboard Skill

1. 读取已锁定 `subject_list`，确保每个出场镜头显式绑定对应 `subject_id`。
2. 批量生成时将同一场景内连续 4 个 9:16 镜头合成为一张 2x2 宫格图提示词（统一画风、光源、服装与人物特征）。
3. 批量出图须先经报价与 HITL 审批；出图完成后调用 `canvas_split_grid_2x2` 自动切图放大并写入画布节点；单镜重抽时直接对选中节点重生成不牵连整组宫格。
