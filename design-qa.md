# 项目管理无边框优化 Design QA

- source visual truth：用户提供的现状截图 `/var/folders/r5/lm_1_1hd321dzlfq0lctjdnw0000gn/T/codex-clipboard-ff1a53db-e81c-49ff-aefc-0a7a0f46d99c.png`，以及 `docs/design/007-前端体验与视觉系统重设计.md` 的无边框界面原则。
- source content crop：`/tmp/lanverse-borderless-preview/projects-source-content.png`。
- implementation screenshot：`/tmp/lanverse-borderless-preview/projects-borderless-css1365x629.png`。
- normalized implementation：`/tmp/lanverse-borderless-preview/projects-borderless-normalized.png`。
- full-view comparison：`/tmp/lanverse-borderless-preview/projects-before-after.png`。
- responsive evidence：`/tmp/lanverse-borderless-preview/projects-borderless-1024x768-after.png`。
- state：浅色桌面端、owner、0 个项目；真实前端组件连接只读本机临时 API，不写入项目数据。
- viewport：主比较实现为 `1365 × 629` CSS px、`devicePixelRatio = 1`；响应式复核为 `1024 × 768` CSS px。
- density normalization：原始截图为 `2048 × 1179`，裁去浏览器 chrome 后得到 `2048 × 944`；现状截图的界面密度约为 1.5 倍，因此实现截图按 Lanczos 放大到 `2048 × 944` 后进行同画布比较。

## Findings

- 无剩余 P0、P1 或 P2 问题。
- P3：0 项目状态下，1024px 高度首屏只露出空状态说明的一部分；主操作仍在向下轻微滚动后可达，且顶栏“创建项目”始终可见，不阻断任务。

## 必查表面

- 字体与层级：继续使用 Geist；工作空间、标题、描述、指标标签和值形成清楚的四级层次，标题未发生意外换行。
- 间距与布局：标题、摘要、工具栏和空状态以留白和完整轻表面分区，不再连续堆叠横线；页面常驻内容无阴影。
- 颜色与令牌：只使用既有黑白中性色和 `muted` 表面；选中筛选通过白色表面、字重和 `aria-pressed` 共同表达。
- 图像与资产：Logo 继续使用项目真实 PNG；空状态和控件图标来自既有 Lucide 图标库，没有新增 CSS 图形、手写 SVG 或占位资产。
- 文案与内容：空项目明确说明下一步并提供“创建第一个项目”；有数据但筛选无结果时提供“清除搜索和筛选”。
- 可访问性：筛选声明为有名称的 `group`，按钮使用 `aria-pressed`；输入保留 label 与可见 focus ring；空状态标题通过 `aria-labelledby` 关联区域。

## 比较历史

1. 首轮现状截图显示标题、摘要上下边界、工具栏分隔线连续出现，页面被切成表格式条带；实现移除 `PageHeader`、`MetricGroup` 和项目工具栏的结构性分隔线，改为留白、排版和 `muted` 轻表面。
2. 首轮 1024px 浏览器复核发现全局搜索与主导航同时占位，导航标签被压成两行，判定 P2；全局搜索改为 `xl` 才显示，导航标签固定单行。修正后 `1024 × 768` 下 Logo、四个导航、创建项目和账户入口均在一行可见，页面无水平溢出。
3. 修正后重新捕获桌面与 1024px 证据；未发现新的 P0、P1 或 P2 问题。

## 主交互与浏览器验证

- 状态筛选：点击“制作中”后 `aria-pressed=true`，可恢复“全部”。
- 搜索：输入框可接收并清除关键词，焦点环可见。
- 创建入口：顶部与空状态入口均可打开真实“创建漫剧项目”对话框；只验证打开和取消，没有提交数据。
- 响应式：`1024 × 768` 下 `body.scrollWidth <= window.innerWidth`，主操作可见。
- 控制台：页面来源没有 error；记录到的一条 error 来自用户 Chrome 的 Immersive Translate 扩展，不属于 Lanverse。
- focused region comparison：本轮没有需要单独放大的图片、密集表格或精细资产；标题—摘要—工具栏—空状态已在全视图中清晰可读，因此不另做局部图。

final result: passed
