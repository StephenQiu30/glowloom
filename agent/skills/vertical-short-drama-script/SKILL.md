---
name: vertical-short-drama-script
description: 面向 9:16 竖屏 AI 漫剧的强反转、黄金 3 秒钩子分集剧本编写规范
allowed_tools:
  - script_get_event_graph
  - script_get_episode
  - script_save_episode
  - bible_list_assets
---

# Vertical Short-Drama Script Skill

1. 仅按本集关联的事件图谱节点与前 1 集结尾状态拉取上下文。
2. 每集 60–90 秒节奏：前 3 秒必须出现强视觉冲突或情绪钩子，中段至少 1 次反转，结尾留强卡点悬念。
3. 剧本结构化输出分场、镜头号、角色名、台词（≤18 字/句适配竖屏字幕）与情绪标签，经 `continuity_reviewer` 审校后落盘。
