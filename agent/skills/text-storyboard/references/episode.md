# 剧集拆解、对白和提及

逐块读取当前集，先给出 summary、conflict、turning_point、ending_hook；找不到悬念写明未显式提供，不能创作新结尾。

场次由地点、时间分支或行动变化决定。闪回、梦境、交叉剪辑保留 presentation 和不同 time_branch，未知时间明确写未知。不要用播放先后假装故事时间。每场连续块范围加上 excluded 必须覆盖这一集所有块；不能跨集。

每个 beat 表达可观察的行动、信息或状态变化，含来源，重要交接、屏幕文字、身份遮蔽和揭露均 required=true。不是每个句子机械生成一个 beat；可以多条证据支持一个行动。心理描写若需外化，保留原文事实，建议标 proposed，不把建议当原稿动作。

对白逐字提取，标注 onscreen/offscreen/phone/inner/group/unknown。保留口语、标点和称呼；speaker_mention 引用本场 cast 提及，不明说话人填 null。角色声称的所有权、身份、动机不等于客观事实。屏幕文字应保留为 beat，未知录音内容不能编造台词。

mentions 分 cast、place、prop。name 必须确实出现在 evidence.quote；同名手机仍可能是不同实例，不提前归并。presence 区分 onscreen（本场实际可见）、offscreen（本场画外出现）、mentioned（只在台词或叙述中被提及）、unknown（无法判定）；台词“别让周野拿走它”中的周野是 mentioned，不能当作在场。visual_details 只引用明确可见且应保留的细节，如铜钥匙三角缺口、手表裂纹和蒙面，不能臆造衣服颜色或镜头参数。

所有 evidence 只能引用当前场的块号；对白引用只取逐字台词。同块重复姓名或台词必须扩大非对白 quote 或明确 occurrence 序号，例如“顾宁问陈伯……陈伯沉默”的第二个“陈伯” occurrence=1。无法判断时列 Issue 并停止猜测。输出 EpisodeAnalysis，不返回镜头。
