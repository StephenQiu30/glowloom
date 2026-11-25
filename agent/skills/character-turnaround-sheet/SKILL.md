---
name: character-turnaround-sheet
description: 为漫剧角色生成正/侧/背三视图与场景定妆图，并锁定主体库 subject_id 与专属 voice_id
allowed_tools:
  - bible_list_assets
  - subject_list
  - media_estimate_quote
  - media_submit_image
  - media_get_task
  - subject_lock_reference
  - voice_bind_character
---

# Character Turnaround Sheet Skill

1. 对比 `bible_list_assets` 与 `subject_list`，仅对未锁定 `subject_id` 的新增角色/换装/场景发起三视图生成。
2. 批量出图前必须先调用 `media_estimate_quote` 汇总报价并等待 HITL 审批通过（取得 `decision_id`）。
3. 出图采用后调用 `subject_lock_reference` 与 `voice_bind_character` 锁定唯一视觉与声音锚点。
