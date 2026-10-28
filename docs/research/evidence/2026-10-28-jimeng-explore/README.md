# 即梦探索观察证据

日期：2026-10-28，Asia/Shanghai。来源：[即梦探索](https://jimeng.jianying.com/ai-tool/explore)。结论：[完整需求分析](../../11-即梦探索完整需求分析.md)。

工具：已连接 Chrome 的 CUA DOM、可访问性、截图及只读几何测量。REA MCP 可用，但当前 Chrome 未开放 loopback CDP，因此本目录不是 REA 生成的 Evidence bundle；`JME-*` 是本地观察编号。

## 文件

| 文件 | 证明范围 |
| --- | --- |
| [observations.json](observations.json) | 32条具名观察；即时加载快照与加载完成快照分开命名 |
| [header-geometry.json](header-geometry.json) | 1869×873下一级、二级、搜索和头像几何；已移除无关账号菜单文本 |
| [drama-list.png](drama-list.png) | 短剧网格、顶栏与头像同排、二级场景与发布入口 |
| [graphic-list.png](graphic-list.png) | 海报瀑布流形态 |
| [image-detail.png](image-detail.png) | 图片详情与两种复用动作 |
| [workflow-readonly.png](workflow-readonly.png) | 实际打开只读制作画布；复制项目入口，未执行复制 |
| [skills.png](skills.png)、[skill-detail.png](skill-detail.png) | 技能目录和详情 |
| [activities.png](activities.png)、[activity-detail.png](activity-detail.png) | 活动列表及评奖中活动详情 |
| [publish-form.png](publish-form.png)、[publish-categories.png](publish-categories.png) | 发布字段、必填标记、发布分类和数量提示 |
| [search.png](search.png)、[search-loaded.png](search-loaded.png) | 搜索切换中的状态与加载完成后的结果；以后者证明结果内容 |
| [header-1280.png](header-1280.png)、[header-1280-scrolled.png](header-1280-scrolled.png) | 窄桌面分类横向滚动，右侧账号区保持可见 |
| [scrolled-list.png](scrolled-list.png) | 内容滚动后的固定顶栏和回到顶部入口 |
| [validation.md](validation.md) | 文档检查结果、本次文件及其他并行工作区修改的逐项边界 |

## 观察编号

- JME-01：初始短剧列表。
- JME-02～07：影视、动画、广告、平面、电商、其他的分类与场景控件；不是各列表加载完成证明。
- JME-08～10：技能加载中、加载后列表、技能详情。
- JME-11：曾查看技能内容正文，仅保留“技能描述/技能内容”的结构性结论，不保存完整第三方技能正文。编号保留空缺。
- JME-12～13：活动与历史展开。
- JME-14～17：加载后的海报列表、图片详情、做同款回填、追加参考图。
- JME-18～19：短剧详情、过程加载；完整画布以 workflow-readonly.png 为准。
- JME-20～21：发布表单和分类；菜单可见项以截图为准，DOM未完整表示弹层。
- JME-22～25：搜索入口、输入及结果类型切换，部分为中间态。
- JME-26～27：漫剧与影视视觉设定的选中状态。
- JME-28～29：1280宽度导航与横向滚动。
- JME-30～31：列表滚动、跨分类重置首场景。
- JME-32：加载完成后的灵感搜索结果。
- JME-33：活动详情、规则、全部/获奖作品及按时间/按热度控件。

## 限制

只观察一个已登录桌面会话；不是账号/地区/版本的通用保证。没有执行上传、生成、发布、关注、收藏、添加技能或复制项目。未读取 Glowloom 当前功能实现。字段上限只是前端提示，不是服务端校验证明。截图中的作品、统计和活动内容仅为调查样本，不作为 Glowloom 的种子内容。

DOM 中私人画布标题已省略。没有读取 cookie、令牌或存储值。视口已复原，调查结束返回探索默认频道。
