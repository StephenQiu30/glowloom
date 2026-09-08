# 原稿检查与分集

1. 通读全部 blocks，区分正文、目录、作者注释、说明和未解决片段。禁止因标题重复就自动删除；目录条目必须有独立 excluded 记录和理由。
2. 有真实正文集标题时 mode=preserve，episodes 的 number 保持原编号；每集从自己的标题块开始，到下一集正文标题之前结束。缺号、重复号保留并列 Issue，禁止补齐。
3. 没有正文集标题时 mode=propose，number=null；根据行动单元、冲突转折和结束悬念提出边界，说明依据。不是按固定字数切集，不改写原稿。
4. first_block/last_block 为闭区间编号。episodes 与 excluded 应无重叠、无遗漏地覆盖所有块，空行也须归属。正文内容不能整体藏进 non_story；不能判断则 unresolved 并请求审阅。
5. 检查目录与正文分离、集号顺序、连续范围和空集。分集 title 是概括性标签，不能当新增剧情。输出 EpisodeMap。
