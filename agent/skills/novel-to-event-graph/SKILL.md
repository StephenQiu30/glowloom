---
name: novel-to-event-graph
description: 将长篇网文、TXT/DOCX 导入文本或故事梗概抽取为结构化章节事件图谱（Event Graph），防止多集改编吃书遗忘
allowed_tools:
  - script_get_event_graph
  - script_upsert_event_graph
  - bible_list_assets
---

# Novel to Event Graph Skill

1. 先读取已有事件图谱 `script_get_event_graph`，避免重复建节点。
2. 将文本拆解为有序事件节点：`event_id`、时间/地点、出场角色、关键道具变化、冲突与因果边、每集末尾悬念钩子。
3. 调用 `script_upsert_event_graph`（带 `expected_version`）持久化，不把整本原文塞入下游分镜提示词。
