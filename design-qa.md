# Lanverse 前端重设计 Design QA

- source visual truth:
  - 内容首页：`/Users/stephenqiu/.codex/generated_images/019ff010-4b8a-7383-80e4-aba44541e1a4/exec-62f72e33-6696-4519-85e5-1ae273c052db.png`
  - 制作详情：`/Users/stephenqiu/.codex/generated_images/019ff010-4b8a-7383-80e4-aba44541e1a4/exec-97a42b11-babe-4308-96d1-220683bc2f41.png`
  - 管理台账：`/Users/stephenqiu/.codex/generated_images/019ff010-4b8a-7383-80e4-aba44541e1a4/exec-fc3a7ad0-7fc7-4ad2-9151-80152530a99d.png`
- implementation evidence: `/Users/stephenqiu/Desktop/StephenQiu/Lanverse/artifacts/product-design/redesign-2026/`
- side-by-side comparisons: `home-comparison.png`、`detail-comparison.png`、`management-comparison.png`、`components-detail-comparison.png`、`components-management-comparison.png`
- implementation screenshots: `home-1440-v2.png`、`studio-detail-unified.png`、`projects-unified.png`
- responsive evidence: `home-390.png`、`projects-390-fixed.png`、`home-css-1024.png`
- source pixels: 三张参考图均为 `1487 × 1058`
- comparison normalization: 实现截图按源图纵横比居中裁切并归一为 `1487 × 1058`，参考和实现在同一张 `2974 × 1114` 图中判断
- state: 浅色桌面端；首页同时覆盖未登录和已登录内容，详情与管理页使用只读 QA API 数据验证真实查询状态

## 设计意图与实现对齐

这次不是对三张图做逐像素复制，而是按用户已确认的页面职责重新组织现有真实业务：方案 1 提供首页的内容编辑感和顶部布局，方案 2 提供“今天往前推进一集”的详情优先级，方案 3 提供高密度台账视角。实现继续使用服务端 `ProductionSnapshot`、版本、候选和人工确认事实，不用视觉稿的模拟数字覆盖真实状态。

三类页面共用一个顶部导航、黑白 Logo、Geist 字体、中性色令牌、Lucide 图标和 shadcn/ui + Radix UI 交互原语。页面内容优先使用留白、排版和分隔线，只有 Dialog、DropdownMenu 等浮层使用阴影，没有回到卡片墙、彩色渐变或常驻侧栏。

业务页标题与事实摘要已进一步收口为服务级组件：`PageHeader` 组合官方 `Item`、`Breadcrumb`、`Badge` 与 `Separator`，`MetricGroup` 组合 `ItemGroup` 与 `Item`。项目、单集、治理、空间和资产页面只传递标题、描述、状态、面包屑与操作，不再分别维护重复的 `h1/p` 样式块；创建项目与创建单集对话框也统一使用官方 Dialog、Select 和 Textarea 组件。

## 必查表面

- 字体与层级：大标题、正文、元数据与版本 ID 分层清晰；390、1024 与 1440 宽度下未出现重叠或不可读截断。
- 间距与布局：全局页边距、Hero 与台账行节奏一致；详情页从上下依次是项目上下文、生产阶段、当前事实、模块入口和主工作面。
- 响应式：1440 桌面、1024 小桌面和 390 窄屏均已实测；窄屏顶栏保留 Logo、主操作、账户与导航，管理表格可受控横向滚动。
- 颜色与表面：主色为近黑，背景为白，弱化文本为中性灰；业务页面无青色品牌色、无装饰渐变、无不必要阴影。
- 图像与品牌：顶栏使用真实透明 PNG Logo；首页 9:16 画面使用项目已有的真实高清封面，灰度裁切与容器比例稳定；无 CSS 插画、手写 SVG 或 Emoji 资产。
- 图标：可见图标统一来自 Lucide，线重、尺寸和光学对齐一致；图标按钮具有语义名称。
- 状态与交互：全局搜索 Dialog、账户 DropdownMenu、项目搜索、归档筛选、项目跳转和单集切换均使用语义化控件；浏览器控制台无 warning/error。
- 可访问性：导航、对话框、搜索和表格保持语义结构；图像有 alt，输入有 label，选中和状态不只靠颜色表达。

## 修正历史

1. 首页首轮 1024 宽度验证中，Hero 三栏过早启用，标题换行影响层级，判定 P2；将中尺寸改为两栏、阶段列换行，并收紧标题字号后通过。
2. Logo 生成稿的透明留白过多，顶栏字标视觉偏小，判定 P2；去除绿幕后按 alpha 边界裁切，复核无白边或色键残留。
3. 390 宽度管理页首轮中，帮助、通知与账户控件挤出右边界，判定 P2；窄屏隐藏非核心帮助/通知按钮与账户箭头，保留可达账户入口后通过。
4. 组件化回归测试发现 Breadcrumb 分隔符被嵌套在 `BreadcrumbItem` 内，产生无效 `li > li` 与 hydration 风险，判定 P2；按官方组合方式改为同级节点后，浏览器控制台无 warning/error。
5. 组件化后单集页面的可访问标题层级一度不再满足既有语义测试，判定 P2；`PageHeader` 增加仅供辅助技术读取的上下文标题，保留视觉层级并恢复既有语义断言。
6. 390 与 1440 宽度下重新验证项目详情、单集详情和管理页，并将新版与参考图放入同一比较输入；未发现新的 P0、P1 或 P2 问题。

## 剩余 Findings

- 无剩余 P0、P1 或 P2 问题。
- P3：390 px 窄屏的管理表格需横向滚动查看全部列；符合本轮“核心操作可达”的移动降级边界。
- P3：参考详情图包含高密度视频候选和镜头数据，当前真实路由先呈现剧本与服务端生产事实；这是按已实现产品阶段的有意缩减，不伪造未存在的候选数据。

## 主链路验证

- 首页：已登录/未登录 CTA、生产阶段、当前项目与最近项目可正常呈现。
- 管理：搜索“长安”后仅保留“长安夜航”；切换“已归档”后仅保留“逆光者”。
- 全局命令：搜索 Dialog 打开后显示首页、项目、资产、治理与空间入口，可关闭。
- 详情：项目详情与单集剧本工作面可从真实 API 查询状态正常渲染，当前版本、资产 readiness、分镜 readiness 与人工确认守则可见。
- 浏览器：主要验证页面无控制台 warning/error，无破图、开发错误遮罩或不可操作主控件。
- 组件回归：`PageHeader` 在 390 px 下操作区正常换行，`MetricGroup` 自动纵向堆叠；项目和单集详情在 1440 px 下保持参考图的内容优先级与信息密度。

final result: passed
