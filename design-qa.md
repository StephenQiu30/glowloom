# Lanverse 服务页面 UI QA：LibTV 参考

2026-10-01 后续状态：现有前端源码、测试与展示素材已按用户指令清空，仅保留现有技术栈与工程配置，本轮不创建替代页面，当前前端不可启动。以下 passed、截图和浏览器记录仅对应清理前的历史提交，不证明当前界面可用，也不替代重建后的验收；Go 服务合同和业务数据保留，范围见 [前端实现清理](docs/design/前端实现清理设计.md)。

日期：2026-09-30（Asia/Shanghai）。仓库：`/Users/stephenqiu/Desktop/StephenQiu/Lanverse`；分支：`main`。

2026-10-01 提交复验：`pnpm exec eslint .`、前端及本轮新文档 Prettier、`pnpm exec tsc --noEmit`、`pnpm exec vitest run --maxWorkers=4`（26 文件、137 测试）、浅色配色修正后的 `pnpm exec next build` 均通过。根 `DESIGN.md` 历史参考内容在 HEAD 与工作区均存在同样的 37 处格式差异，本轮新增段落没有新增差异；未为提交全量格式化历史原文。本机未安装 gitleaks，本次核对显式文件白名单及 staged diff，未声称通过本机 secret 扫描或远端 CI。

**result: passed，限本轮已实现服务页面的视觉与交互范围。** 首页、共享工作台、真实项目入口和真实项目资产查询已接入当前界面。生成类入口明确展示准备状态，任务、供应商和账号等样例页面继续说明数据来源。用户已调整优先级为先迁移能力，本报告收束现有 UI，不继续扩展前端，也不把 UI 验收作为能力迁移或 MVP 完成的证明。

本报告的截图已逐张查看；自动化结果分为本报告作者执行的定向回归与主代理提供的整项目执行记录。最终首页布局微调后，16 个定向回归与最后一次全量构建均已通过，状态见质量门禁表。

## 1. 参考与实际范围

参考来自用户提供的 Safari 中 [LibTV 首页](https://www.liblib.tv/) 截图。采用其石墨黑工作台、常驻侧栏、青色主操作、点阵画布入口、八项快捷入口、横向最近项目和电影画幅卡片。Lanverse 使用自身名称、Lucide 图标、shadcn/Radix 控件、真实项目查询和原创示意图。

| 页面                                       | 当前实现事实                                                              | 可用性边界                                                        |
| ------------------------------------------ | ------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| `/`                                        | 首页、八项创作服务、真实最近项目、指南分类与搜索、服务说明                | 视频/图片/音频等生成入口展示“服务准备中”，不能提交生成            |
| `/projects`                                | 真实项目列表、搜索、状态筛选、游标分页、卡片/表格、新建对话框             | `/projects?create=true` 打开既有真实创建表单，保留幂等与草稿保护  |
| `/projects/{UUID}/canvas`                  | 最近项目与创建成功进入正式项目画布                                        | 独立全屏布局，不重复渲染工作台侧栏                                |
| `/assets`                                  | 选择真实活动项目、查询媒体、当前页类型/文件名筛选、游标分页、临时授权预览 | 浏览器本轮选中的两个项目均无媒体，未完成真实文件预览验收          |
| `/tasks`、样例媒体、`/admin/*`、`/account` | 共享外观、筛选与本地预览交互                                              | 仍显示样例/待接入状态；未声称真实任务执行、供应商连接或账号持久化 |

业务与设计范围以 [DESIGN.md](/Users/stephenqiu/Desktop/StephenQiu/Lanverse/DESIGN.md) 当前服务工作台段落为准。本轮没有迁入付费生成、会员、积分、社区作品、供应商执行或新的后端合同。

## 2. 截图来源、归一化与证据目录

截图保存在仓库外的 `/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/`。未把截图或浏览器日志复制到仓库。

| 证据                                                                                                                                                   | 文件像素  | 用途                                          |
| ------------------------------------------------------------------------------------------------------------------------------------------------------ | --------- | --------------------------------------------- |
| [LibTV Safari 来源](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/libtv-reference.png)  | 2910×2100 | 保留浏览器栏的用户参考来源                    |
| [LibTV 比较图](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/libtv-comparison.png)      | 1440×950  | 去浏览器栏并归一化后的视觉比较                |
| [首页最终桌面](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/home-desktop-final.jpg)    | 1440×950  | 最终布局与点阵入口                            |
| [首页初始桌面](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/home-desktop.jpg)          | 1440×950  | 保留纵向间距偏大的初始迭代                    |
| [首页最终手机](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/home-mobile-final.jpg)     | 375×2359  | 手机最终长页面；CSS 视口宽按浏览器记录为 390  |
| [首页初始手机](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/home-mobile.jpg)           | 375×2475  | 保留初始移动布局                              |
| [首页平板](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/home-tablet.jpg)               | 1009×1392 | 1024 CSS 宽下项目与指南重排                   |
| [浅色主题辅助截图](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/home-light-mobile.jpg) | 551×830   | 明暗主题辅助证据，不用作 390 CSS 宽的像素比较 |
| [账号页手机](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/account-mobile.jpg)          | 375×878   | 手机账号表单与样例状态                        |

来源为 macOS Safari，当前实现由 Codex 内置浏览器（IAB）捕获；未记录具体 OS/Safari 版本，不据此推断跨版本一致性。Safari 来源按 2× 密度处理：从 2910×2100 图顶部去除约 180 像素浏览器栏，内容为 2910×1920，再按 2× 映射至 1455×960，输出比较图 1440×950。IAB 桌面记录的视口为 1455×960，输出同为 1440×950，显示捕获缩放约 0.99，密度记录为 1。

这是用于 QA 的只读裁切与归一化；没有生成或合成实现截图。375 像素手机图与 390 CSS 视口、1009 像素平板图与 1024 CSS 视口不是相同计量，不能从图片宽度反推 CSS 尺寸。字体渲染、滚动条、捕获缩放和浏览器不同，**本轮不是 pixel-exact 验收**。

已在同一次工具调用中查看 LibTV 原始来源与最终桌面图，随后查看归一化比较图：

![LibTV 去浏览器栏后的比较图](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/libtv-comparison.png)

![Lanverse 最终桌面服务首页](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f2af-9159-7353-86a7-208542da5f3f/lanverse-services-ui/home-desktop-final.jpg)

## 3. 五个主要视觉表面

| 表面             | 参考特征与实现                                                        | 核对结果与差异                                                                  |
| ---------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| 侧栏与顶栏       | 桌面 240px 常驻石墨黑侧栏、主操作青色、导航选中背景；手机使用导航抽屉 | 结构和主次层级接近；Lanverse 保留自身服务分类，顶部为使用指南、主题与账号       |
| 点阵新建入口     | 宽横幅、中心白色加号按钮、下方创建文字                                | 改为约 160px 高，原创石墨灰点阵可辨；灰点密度与参考仍有差别                     |
| 八项能力快捷入口 | 同列圆角暗色图标表面与短标签                                          | 桌面八列、手机四列；图标区域 56px，使用 Lucide；生成入口明确说明服务准备状态    |
| 最近项目         | 横向紧凑项目卡、缩略区域、名称与次级信息                              | 卡片约 88px 高；使用真实项目 UUID、名称、画幅与风格；接口没有封面或日期时不补造 |
| 电影画幅内容卡   | 16:9 缩略图、短标题、标签与简短文案                                   | 四张原创静态创作指南；分类、搜索和说明可交互。它们不是作品、生成结果或社区内容  |

最终页面 section 间距采用 32px，指南标题、分类与搜索在桌面同一行。相同 1440×950 比较图下，入口、快捷入口、项目和指南主要区域的纵向位置已接近参考。人工目测初始指南图起点约 730px，最终约 590px，前移约 140–150px；该数值用于解释布局修正，不作为像素级误差判定。

## 4. 迭代发现与修正

| 发现                                                  | 影响                                          | 修正与复核                                                                                                                                                                                             |
| ----------------------------------------------------- | --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| P2：最初 hero 过暗，点阵不清楚，主动作青色偏淡        | 新建入口视觉重量不足                          | 用内置 ImageGen 编辑原 hero 为均匀石墨灰与浅灰规则点阵，最终命名 `canvas-grid.png`；主题主色最终采用浅色深青 `#005b6c`、深色 `#11cdeb`；复核最终桌面图                                                 |
| P2：初始桌面纵向留白偏大，指南图下移约 150px          | 950px 高视口难以完整读到指南文案              | hero 160px、快捷图标 56px、最近项目约 88px、section gap 32px，并合并指南标题/筛选行；最终桌面内容密度接近参考                                                                                          |
| 说明或帮助模态在同路径导航时仍可能叠在目标页上        | 点击创建后目标内容被浮层遮挡                  | 导航链接使用 DialogClose；移动导航执行关闭事件；覆盖 `/projects` 帮助创建与抽屉同路由回归                                                                                                              |
| 服务、指南与媒体预览关闭后的焦点回归                  | 键盘用户失去触发位置                          | 保存触发元素并在关闭后恢复焦点；服务与指南定向回归、样例媒体浏览器复核通过                                                                                                                             |
| P2：浅色主色 `#00b9d8` 用于选中态小字号文字时对比不足 | 浅色任务/素材筛选、完成状态和指南标签难以辨认 | 提交前改浅色 `--primary`、`--ring`、`--sidebar-primary` 为 `#005b6c`，对应主色文字变量为 `#ffffff`；深色主题保持本轮现有值。按实际 sRGB 色值复核白底、主色 10% 底及按钮常态/悬停文字对比，均超过 4.5:1 |

提交前颜色计算使用 Python 3 的 sRGB 相对亮度公式：先将每个通道线性化，再计算 `(较亮亮度 + 0.05) / (较暗亮度 + 0.05)`。旧浅色主色在白底的对比为 2.351:1，在 `primary/10` 白底叠加色上为 2.148:1；新色分别为 7.741:1、6.602:1。白色主按钮文字常态为 7.741:1，`hover:bg-primary/80` 在白底上为 4.797:1。半透明色按其在白底的实际通道合成值计算；这项记录是颜色计算证据，仓库外浅色截图仍对应修正前配色，不作为修正后的浏览器验收截图。

实际引用检查确认工作台链接与共享控件通过 `ring-ring`、`border-ring` 使用焦点色；侧栏主色仅通过主题变量映射暴露，当前工作台选中态使用 `primary`。此次只同步浅色主题的同组变量，没有扩改组件或深色主题。

仅使用内置 ImageGen 模式生成/编辑项目示意素材，未使用 CLI、远程下载或 LibTV 图像复制。图片路径与原像素尺寸如下；内置生成不保证精确十六进制色值和点间距。

| 项目素材                                      | 尺寸     | 来源和用途                                              |
| --------------------------------------------- | -------- | ------------------------------------------------------- |
| `frontend/public/studio/canvas-grid.png`      | 1983×793 | 原 `hero-canvas.png` 经编辑后重命名；均匀石墨灰点阵背景 |
| `frontend/public/studio/script-studio.png`    | 1672×941 | 原创荒漠暮色、黑色风衣人物与远处车灯                    |
| `frontend/public/studio/director-studio.png`  | 1672×941 | 原创暖光山路、红色敞篷老式跑车人物                      |
| `frontend/public/studio/character-studio.png` | 1672×941 | 原创短发亚洲女性、深绿影棚侧光肖像                      |
| `frontend/public/studio/motion-studio.png`    | 1672×941 | 原创雾中森林运动与景深示意，也用于侧栏指南卡            |

## 5. 浏览器验证

以下为主代理浏览器执行记录；截图检查与单元测试不能替代真实外部能力验收。

| 路径/交互                           | 本轮观察                                   | 证据边界                                                  |
| ----------------------------------- | ------------------------------------------ | --------------------------------------------------------- |
| 首页明暗主题与 390/1024/1455 CSS 宽 | 能读取真实项目，卡片响应式重排，无横向溢出 | 覆盖当前浏览器与视口；没有开展 Safari/Firefox/Edge 全矩阵 |
| 项目搜索                            | 真实项目可读，搜索无结果显示空态           | 没有以静态示例补项目结果                                  |
| URL 创建入口                        | `/projects?create=true` 打开新建表单       | 新建合同仍由既有项目实现负责                              |
| `/assets` 切换项目                  | 两个真实项目选择与空资源状态可读           | 两项目均无真文件；未完成真实媒体播放/下载验收             |
| 任务失败筛选                        | 失败状态筛选和任务路径可用                 | 属于样例任务，不证明 Temporal/供应商执行                  |
| 样例媒体视频筛选/预览               | 视频类型筛选、预览打开关闭和焦点回归可用   | 仅内置样例，不证明真实生成媒体                            |
| 供应商搜索                          | “图像”筛选后显示 1 条对应样例              | 行内仍说明尚未接入，没有供应商连通证明                    |
| 账号手机页                          | 表单在手机宽度可读，无横向溢出             | 数据是页面草稿，保存服务尚未接入                          |
| 新 IAB tab 2 控制台                 | 浏览器记录 `consoleErrors: []`             | 限该次捕获，没有声称所有环境永远无错误                    |

## 6. 自动化质量门禁

| 命令（均在 `frontend/`）                                                                                                   | 结果                           | 记录来源与限制                                     |
| -------------------------------------------------------------------------------------------------------------------------- | ------------------------------ | -------------------------------------------------- |
| `pnpm exec vitest run src/components/workbench/home-page.test.tsx src/components/workbench/workspace-shell.test.tsx`       | 2 文件、16 用例通过            | 本报告作者执行；最终首页纯布局调整后主代理再次通过 |
| `pnpm exec eslint src/components/workbench/home-page.test.tsx src/components/workbench/workspace-shell.test.tsx`           | 通过，exit 0                   | 本报告作者执行                                     |
| `pnpm exec prettier --check src/components/workbench/home-page.test.tsx src/components/workbench/workspace-shell.test.tsx` | 通过                           | 本报告作者执行                                     |
| `pnpm exec vitest run --maxWorkers=4`                                                                                      | 26 文件、137 用例通过          | 主代理整套执行记录；没有修改测试超时               |
| `pnpm exec eslint .`                                                                                                       | 通过                           | 主代理执行记录                                     |
| `pnpm exec prettier --check .`                                                                                             | 通过                           | 主代理执行记录；本报告新增后另行检查文档           |
| `pnpm exec next typegen` + `pnpm exec tsc --noEmit`                                                                        | 通过                           | 主代理执行记录                                     |
| `pnpm exec next build`                                                                                                     | 最终紧凑布局修正后通过，exit 0 | 主代理最后一次执行记录（session 8535）             |

默认并行运行整套 Vitest 时，一次 `assets-workspace` 的 `findByRole` 等待发生超时。没有通过增加 timeout 掩盖问题；限定 4 个 worker 后 137 个测试全部通过。保留该运行异常，不把它解释为已修复的业务缺陷。Vitest 还提示未来 Vite 配置加载模式变化，不影响当前退出结果。

规范核对采用 `vercel:react-best-practices` 与 `vercel:nextjs` 技能：业务查询沿用 TanStack Query 与既有生成 API、独立页面按 Server/Client 边界组织、图片使用 Next Image 与 sizes、筛选直接计算派生值、模态和导航沿用 shadcn/Radix 的焦点与可访问语义。本轮 UI 没有改 Go 代码，Go 门禁不属于本次验证范围。规范审查、静态门禁、浏览器 UI 验证和真实生成验收分别记录。

## 7. Git 变更逐项说明

以下为本轮 UI 工作区快照，路径相对仓库根目录。最终交付前仍须核对 `git status --short`，保留其他任务的并行改动。

| 路径                                                          | 作用                                                       |
| ------------------------------------------------------------- | ---------------------------------------------------------- |
| `DESIGN.md`                                                   | 记录 LibTV 服务工作台的范围、布局、真实/样例边界和 QA 入口 |
| `design-qa.md`                                                | 本报告，仅记录已实现 UI 与验证事实                         |
| `frontend/src/app/globals.css`                                | 石墨黑/浅色表面、青色主动作与共享主题变量                  |
| `frontend/src/app/page.tsx`                                   | 根路由从重定向改为工作台首页                               |
| `frontend/src/app/providers.tsx`                              | 默认深色主题，保留 Query 与主题 Provider                   |
| `frontend/src/app/assets/page.tsx`                            | 新增真实资产页面路由与 Suspense 边界                       |
| `frontend/src/components/project/create-project-dialog.tsx`   | 支持受控 URL 创建入口并保持草稿离开保护                    |
| `frontend/src/components/project/project-list.tsx`            | 项目卡片/表格外观与正式 UUID 画布路径                      |
| `frontend/src/components/project/projects-workspace.tsx`      | 接共享工作台布局，搜索/筛选/分页与 `create=true` 创建行为  |
| `frontend/src/components/project/projects-workspace.test.tsx` | URL 创建、关闭移除参数、草稿保护与既有创建回归             |
| `frontend/src/components/theme-toggle.tsx`                    | 焦点样式随统一主题语义                                     |
| `frontend/src/components/workbench/home-page.tsx`             | 服务首页、真实最近项目、指南筛选和准备状态说明             |
| `frontend/src/components/workbench/home-page.test.tsx`        | 6 个首页查询、筛选、模态与焦点回归                         |
| `frontend/src/components/workbench/workspace-shell.tsx`       | 侧栏、顶栏、主要导航、移动抽屉、帮助和画布全屏边界         |
| `frontend/src/components/workbench/workspace-shell.test.tsx`  | 10 个导航选中、同路由关闭与独立画布回归                    |
| `frontend/src/components/workbench/assets-workspace.tsx`      | 真实项目资产查询、分页、边界状态与授权预览生命周期         |
| `frontend/src/components/workbench/assets-workspace.test.tsx` | 真实项目/媒体端口、分页、切换、错误与预览清理回归          |
| `frontend/src/components/workbench/asset-pages.tsx`           | 既有样例资产与媒体页外观、过滤和预览状态                   |
| `frontend/src/components/workbench/task-pages.tsx`            | 样例任务筛选、未确认提交和不可用服务说明                   |
| `frontend/src/components/workbench/admin-pages.tsx`           | 供应商/模型/账号样例过滤、草稿和服务状态说明               |
| `frontend/src/components/workbench/workbench-components.tsx`  | 共享标题、表格、内容表面与样例边界外观                     |
| `frontend/src/components/workbench/service-pages.test.tsx`    | 样例媒体、任务、供应商和账号的服务边界回归                 |
| `frontend/public/studio/canvas-grid.png`                      | 原创点阵入口背景                                           |
| `frontend/public/studio/script-studio.png`                    | 原创剧本指南示意图                                         |
| `frontend/public/studio/director-studio.png`                  | 原创镜头指南示意图                                         |
| `frontend/public/studio/character-studio.png`                 | 原创角色指南示意图                                         |
| `frontend/public/studio/motion-studio.png`                    | 原创氛围指南与侧栏示意图                                   |

2026-09-30 UI 验证结束时没有创建分支、提交、推送、PR、合并或发布。2026-10-01 按用户授权直接在本地 `main` 保存测试提交 `7d024416` 和页面实现提交 `3d526fb2`；本报告及相关设计随后单独提交。截图只在仓库外作为证据保存。后续能力迁移属于独立任务，不能借本报告声称真实付费生成、真实媒体闭环或整体产品验收通过；本轮没有推送或发布。
