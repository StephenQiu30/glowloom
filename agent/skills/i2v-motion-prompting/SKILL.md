---
name: i2v-motion-prompting
description: 分级首尾帧图生视频（I2V）、角色固定音色配音、特写按需对口型（LipSync）与显式 Take 采用规范
allowed-tools: canvas_read_graph media_estimate_quote media_submit_video_i2v media_submit_tts media_submit_lipsync media_get_task shot_adopt_take timeline_assemble_adopted_shots timeline_export_render
---

# I2V Motion & Audio Production Skill

1. 常规镜头采用首尾帧锚点 I2V + 角色绑定 `voice_id` TTS 对白与情绪 BGM；仅景别为特写/近景且有清晰口部对白的镜头开启 `media_submit_lipsync`。
2. 批量渲染前汇总逐镜秒价与总积分，进入 HITL 审批取得 `decision_id` 后再并发提交。
3. 每镜支持多 Take 抽卡，仅显式调用 `shot_adopt_take` 采用的镜头可进入 `timeline_assemble_adopted_shots`。
