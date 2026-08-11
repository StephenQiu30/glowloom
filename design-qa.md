# 登录态首页移除 Design QA

- source visual truth：用户提供的已登录欢迎页截图 `/var/folders/r5/lm_1_1hd321dzlfq0lctjdnw0000gn/T/codex-clipboard-58fb2277-6077-48c3-8f1d-2531f1f68725.png`，以及“已经登录后不需要首页”的明确修正要求。
- source pixels：`3376 × 1942`，包含浏览器 chrome；截图中登录态仍显示“首页”导航和“欢迎回来”工作概览。
- implementation closed：`/tmp/lanverse-logged-in-project.png`，`1673 × 787`；真实 owner 登录会话最终落在 `/projects`。
- implementation command：`/tmp/lanverse-logged-in-project-command.png`，`1688 × 794`；真实 Command + Radix Dialog 打开态。
- combined comparison：`/tmp/lanverse-logged-in-home-before-after.png`，上方为用户源图，下方为实现命令打开态；两张输入先归一到 `1688px` 宽后上下拼接。
- test state：浅色模式、owner、零项目；使用用户现有本地登录会话，只执行读取、导航和命令面板打开/关闭，不写入业务数据。

## Findings

- 无剩余 P0、P1 或 P2 问题。
- P3：Logo 的无障碍名称仍是“Lanverse 首页”，链接仍指向 `/`。这是有意保留的全局品牌入口；登录态点击后会立即路由替换到 `/projects`，不会显示欢迎页或在历史中增加首页步骤。

## 必查表面

- 字体与层级：登录用户直接进入“项目管理”事实页，继续使用 Geist 与既有标题层级；移除了重复的“欢迎回来”主标题，不引入新的排版系统。
- 间距与布局：项目页内容和 Logo/主导航继续共用 `max-width: 1440px` 与同一组响应式左右内边距；移除首页项后，导航基线和入口间距保持稳定。
- 颜色与令牌：没有新增色彩、边框或阴影。项目台账与命令浮层继续使用既有 `background`、`muted`、`popover` 与 foreground 令牌，符合无边框原则。
- 图像与资产：Logo 继续使用真实品牌 PNG；项目、资产、治理和空间入口继续使用 Lucide 图标，没有自绘图形或文本符号。
- 文案与内容：登录态不再出现“欢迎页”“欢迎回来”“开始创作”或首页项目摘要；项目页成为唯一默认工作落点，命令面板只提供“项目 / 资产 / 治理 / 空间”。
- 可访问性：重定向期间提供“正在进入项目工作区”具名状态；导航和 Command 的 role/name 可被语义查询，移除首页项后键盘搜索、Escape 关闭与焦点返回仍通过测试。

## 比较历史

1. 源图的登录态同时存在“首页”导航与欢迎概览，用户需要再判断或点击才能进入真实工作面，构成 P1 信息架构冗余。
2. 实现将 `/` 收敛为身份分流点：未登录继续展示产品首页，登录态只显示具名过渡状态并执行 `router.replace("/projects")`，不再请求 `/me` 或项目摘要来渲染欢迎页。
3. 第一轮测试固定根路由重定向、角色导航和命令面板三条行为并如预期失败；实现后相关 10 个测试通过，全量 20 个测试文件、69 个测试通过。
4. 最终组合对照显示：登录欢迎内容已被项目管理工作面替代；主导航移除“首页”，命令面板也没有旁路入口，项目页整体宽度与顶部导航保持一致。

## 主交互与浏览器验证

- 根路由：从真实登录项目页点击指向 `/` 的 Logo，页面最终地址为 `http://localhost:3000/projects`，未闪回营销或欢迎内容。
- 主导航：真实 owner 会话仅生成“项目 / 资产 / 治理”，没有“首页”。权限单元测试同时覆盖 viewer、editor 与 owner。
- 命令面板：点击“搜索或执行命令”打开具名 Dialog，结果仅含“项目 / 资产 / 治理 / 空间”，没有“首页”。
- 数据请求：登录态根页面单元测试确认不调用首页 `/me` 和项目列表请求，只执行路由替换。
- 错误恢复：登录态 503/403 状态改为“返回项目”并指向 `/projects`；未登录 401 仍可返回公开产品首页。
- 控制台：Lanverse 页面来源没有 error 或 warn；记录到的 error 均来自 Immersive Translate Chrome 扩展，不属于项目代码。
- 构建验证：`npm run lint`、`npm run typecheck`、`npm test -- --run`、`npm run build` 全部通过。

final result: passed
