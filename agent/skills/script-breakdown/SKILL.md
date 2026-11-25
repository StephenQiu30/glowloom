---
name: script-breakdown
description: 将创作者简报或原始小说文本拆解为结构化分集大纲、角色设定与场景清单
allowed-tools: read_project_script read_project_bible update_project_script
---

# Script Breakdown Skill

1. 先调用 `read_project_script` 与 `read_project_bible` 获取当前权威版本，避免覆盖创作者最新手动编辑。
2. 按“分集目标 → 关键冲突 → 场景列表 → 出场角色”输出结构化拆解。
3. 未经用户明确确认前，不直接触发收费媒体生成工具。
