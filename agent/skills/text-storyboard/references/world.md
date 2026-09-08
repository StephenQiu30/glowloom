# 全稿设定与事件账

输入为已逐集解析的全稿提及与原文证据。以提及为起点，不通过一篇摘要重新猜全部实体。

1. entities 是候选身份/地点/道具实例；每条 MentionRef 恰好归属一个候选或 unresolved_mentions。不得跨 cast/place/prop 类型合并。同场两个人同时出现，或多人各自持有手机，不因同名自动合并。
2. 同义称呼、亲属称呼和隐藏身份按证据提出归并。identity_basis=explicit 仅限叙述明确或无歧义同一提及；推断标 inferred/uncertain，说明 uncertainty。同名不证明同一实例；无法分辨保持多个候选或 unresolved。
3. 关系保留 basis=narration/claim/unknown 和 origin。台词“钥匙本来属于我”只能是 claim，不证明所有权。
4. state_events 按场记录 before/after、story_time、time_branch、knowledge。每条事件至少有本场事件证据；如果还需跨场身份说明，只能复用 entities.evidence 已登记的归并证据，不能把其他场的动作放到本场。原文未交代的状态用 null/unknown，不把没有描写等于状态不变。只登记实际变化与影响叙事的矛盾，不为每个未说明的属性制造空状态事件。闪回单独分支，不能覆盖当前状态。claim 或 unknown 不能记录 known。
5. 蒙面身份在后集揭露可以在全局提出同一实体，但证据必须指向揭露位置；早期导演不能据此显示姓名/面部。裂纹手表在闪回完好只说明两个时间状态，损坏时刻不明。
6. asset_needs 只汇总原稿支持的可见造型/地点变体/道具状态需求，保留 evidence；不生成图片、不调用供应商、不创作未经授权的故事。

最后反向检查每个场次提及、关系和事件来源；高风险合并及矛盾必须进入 issues。输出 WorldBook 草案。
