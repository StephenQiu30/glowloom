# Lanverse 全局 Mock 前端 Design QA

- source visual truth paths:
  - 首页方向：`/Users/stephenqiu/.codex/generated_images/019fae81-f5ac-74c0-8fe3-1a6fb4c612b0/exec-dabf2b9e-2360-4448-9d95-d62e1695de13.png`
  - 项目推进方向：`/Users/stephenqiu/.codex/generated_images/019fae81-f5ac-74c0-8fe3-1a6fb4c612b0/exec-15faaa1b-8e23-42e8-ad67-b7651bfd88d2.png`
  - 最终资产工作台方向：`/Users/stephenqiu/.codex/generated_images/019fae81-f5ac-74c0-8fe3-1a6fb4c612b0/exec-8ebebd3d-9cb3-4e39-b53c-64eeb2e09330.png`
- implementation screenshot directory: `/Users/stephenqiu/Desktop/StephenQiu/Lanverse/artifacts/product-design/global-rollout/`
- implementation screenshots: `home.png`、`projects.png`、`project-detail.png`、`studio.png`、`workspaces.png`、`login.png`
- side-by-side comparison evidence: `home-comparison.png`、`project-detail-comparison.png`、`studio-comparison.png`
- focused inspector evidence: `/Users/stephenqiu/Desktop/StephenQiu/Lanverse/artifacts/product-design/design-qa-inspector-comparison.png`
- viewport: 应用内浏览器桌面窗口 `1032 × 877` CSS px；登录页窗口为 `1047 × 889` CSS px
- source pixels: 三张参考图均为 `1487 × 1058`
- implementation pixels: 主工作台截图为 `1032 × 877`；登录页为 `1047 × 889`
- density normalization: 比较图将参考稿归一化为 `1032 × 734`，实现截图裁取首屏顶部 `1032 × 734`，均为 1x；未比较浏览器外框
- state: 浅色桌面端；首页默认态、项目库全部项目态、项目详情制作概览态、资产库角色/顾清禾态、工作空间默认态、登录默认态

## Full-view comparison evidence

三张并排图在同一个比较输入中检查。首页实现保留了参考稿的左侧创作导航、大型 AI 导演输入、快捷入口和继续创作项目流；资产工作台保持了六步生产进度、资产分类、角色列表、中心设定图和版本检查器。项目详情参考稿展示“单集镜头推进”，实现展示上一级“项目制作概览”，状态并不相同，因此只对信息层级、生产阶段、颜色、控件密度和下一步引导作方向性比较，不做逐像素结论。

全局扩展页使用同一个 `StudioShell`、Mock 数据源和组件语义：白色固定侧栏、76 px 顶栏、冷灰背景、青色主操作、12–16 px 圆角、细描边和轻量阴影。项目库、工作空间与登录页没有独立参考画板，按已确认的方案 3 视觉系统检查一致性。

## Focused region comparison evidence

资产检查器的标题、版本、锁定状态、描述、参考图、主次按钮和影响范围已使用独立局部比较图核验；字段顺序、密度和状态语义可读。首页输入器与项目卡在完整比较图中占据足够像素，不需要额外裁切。项目库封面、工作空间表单和登录分栏使用独立原始截图检查图片裁切、表单间距和文本层级。

## Required fidelity surfaces

- Fonts and typography: 统一使用 Geist 与中文系统回退；32–48 px 主标题、18–20 px 区块标题、14 px 正文与 12 px 元数据层级稳定，无异常截断。项目卡描述使用单行截断，属于预期行为。
- Spacing and layout rhythm: 76/156 px 响应式侧栏、76 px 顶栏、20–32 px 页面边距、12–24 px 组件间距与 12–24 px 圆角贯穿全部路由；1032 px 桌面窗口下无持久控件遮挡。
- Colors and visual tokens: 全局 token 已固定为冷白背景、Slate 文本、`#079db3` 青色主操作；成功、警告、草稿和禁用状态均同时使用文案或图标表达。
- Image quality and asset fidelity: 三张项目封面与五张角色资产均为独立高分辨率 raster 资源；封面为 3:4 原图并使用适合卡片槽位的 `object-cover`，登录页使用全高裁切。没有使用占位图、CSS 插画、手写 SVG 或 Emoji 代替产品资产。
- Copy and content: 全局统一使用 AI 漫剧、剧本—资产—分镜—生成—审核—交付、单集、版本、锁定、引用和工作空间等产品词汇；Mock 提示仅出现在需要解释演示状态的位置。
- Icons and accessibility: 图标统一来自 Lucide；导航、表单、按钮、标签页与图片均具备语义名称，输入有 Label，选中/禁用/成功状态可被辅助技术读取。
- Responsiveness: 当前目标为桌面 Web。1032 px 宽度下项目卡自动变为两列；资产三栏在窄桌面使用内部横向滚动保持字段可用。移动端不是本轮验收目标。

## Comparison history

1. 早期资产工作台截图中，角色设定主体偏小且说明区贴近 viewport 边缘，判定 P2；固定中心视觉槽高度并调整设定图缩放后通过复核。
2. 早期角色列表行高偏紧，判定 P2；头像提升至 64 px 并增加列表节奏后通过复核。
3. 全局替换首轮发现 `/studio` 仍维护一份重复侧栏与顶部流程，存在持续漂移风险，判定 P2；改为复用 `StudioShell` 后，导航、顶栏、创建对话框和进度样式与其他路由一致。
4. 本轮并排复核没有发现新的 P0、P1 或 P2 问题；未因项目详情参考与实现状态不同而制造伪精度。

## Findings

- 无剩余 P0、P1 或 P2 问题。
- P3：1032 px 窗口下资产三栏需要横向滚动；这是桌面最小宽度下的可接受降级，后续若把移动/平板纳入范围，可改为抽屉式检查器。
- P3：项目详情参考稿是“单集镜头推进”状态，而实现当前为“项目制作概览”状态；后续实现单集工作台时应复用该参考稿，而不是把两个层级合并。

## Primary interactions tested

- 首页：输入故事灵感，启用并点击“开始创作”，成功提示出现。
- 项目库：搜索“长安”，列表只保留匹配项目。
- 项目详情：切换“生成任务”，Mock 内容状态正常显示。
- 资产库：从“角色”切换到“场景”，场景资产正常显示。
- 工作空间：输入并创建“镜界工作室”，新条目与成功反馈正常出现。
- 浏览器渲染：六个路由均成功载入，未出现可见错误覆盖层或破损图片。

## Follow-up polish

- 后续接入真实 API 时保持现有组件输入与信息层级，只替换 Mock 数据源和交互提交逻辑。
- 若进入移动端设计阶段，单独设计底部导航、项目卡单列和资产检查器抽屉，不直接压缩当前桌面三栏。

final result: passed
