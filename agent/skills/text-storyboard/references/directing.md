# 单场导演与文字分镜

依次完成意图、调度、覆盖、镜头、校时、方向、画板及复核；保留中间字段，不能只写提示词。

1. dramatic_intent 说明本场冲突和观众信息变化；audience_knows/withhold 分开可见信息与应保密信息。只使用当前场及显式提供的可披露设定，不联想未来剧情。
2. blocking 使用本场 presence=onscreen 的 mention_key，说明位置、朝向、动作、遮挡与道具持有；只被台词提及的人物不能入画或参与调度。缺少左右手等信息时允许导演提案，但不得伪称原文指定。
3. 为每个 required Beat、全部 Dialogue、全部 visual_details 规划覆盖。一个镜头可以覆盖多个节拍，也可用动作镜头承载画外音；不能为了“每句一镜”打断连续行动。
4. shots 按呈现顺序排列。purpose、framing、camera_movement、action、screen_direction、entry_state/exit_state 要相互一致。每镜至少引用一个现有 beat_key；visible_mentions 只能引用本场 presence=onscreen 的提及，禁止凭全局身份增加早期露脸。
5. audio 引用完整 dialogue_key 和原 channel，不重写台词、不重复创造新对白。时长为 duration_min_ms/max_ms 区间，timing_basis 解释台词字数、动作、停顿和重叠的估计，不宣称真实音频测量结果。
6. detail_evidence 映射钥匙缺口、屏幕准确文字、面部遮蔽等原文细节；每一条本场 visual_details 至少出现在一个镜头，不凭空加视觉事实。
7. 每镜 panel_caption 为独立文字画板；本阶段没有图像，不编造 image URL，也没有媒体生成依赖。连续性风险、越轴、关键动作遗漏及不可评价项进入 Issue。

输出 SceneDirection，保持 SceneBreakdown → Beat/Dialogue → Shot → 文字 Panel 的可追踪关系。技术校验通过后仍需要分镜审阅。
