# Lanverse 界面验证

本文件规定界面验证方法与证据边界；视觉规则见 [DESIGN.md](DESIGN.md)，业务验收见 [TST-01](workspace/content/test/01-测试策略.md)、[TST-02](workspace/content/test/02-需求追踪矩阵.md) 与 [TST-03](workspace/content/test/03-AI评测方案.md)。未完成范围持续记录在 [BACKLOG](BACKLOG.md)。

## 验证范围

- 以实际运行的正式项目、画布、媒体、任务和配置入口核验数据；覆盖加载、空、失败、重试、切换项目、刷新恢复与撤权。
- 检查明暗主题、桌面/平板/手机布局和横向溢出；记录 CSS 视口、浏览器与实际环境，图片像素不能反推 CSS 尺寸。
- 键盘覆盖导航、表单错误、菜单、弹窗、抽屉和预览；验证可访问名称、焦点进入/返回与 Escape 关闭语义。
- 付费链检查报价、明确确认、恢复、候选预览和用户采用；技术、真实集成与产品证据分别核验，不能互相替代。

## 自动化门禁

在 `frontend/` 运行：

```bash
pnpm exec eslint .
pnpm exec prettier --check .
pnpm exec next typegen
pnpm exec tsc --noEmit
pnpm exec vitest run
pnpm exec next build
```

按改动补充 Playwright 与真实浏览器验证；API 变化先从运行后端在线 Swagger 重新生成客户端。Vercel React 最佳实践与安装版本的 Next.js 文档审查须覆盖请求瀑布、Server/Client 边界、缓存范围、派生状态及可访问性。

## 证据与未验收条件

截图仅证明捕获时的界面；单元测试、lint、类型检查与构建不证明供应商执行、真实费用、媒体处理或完整产品验收。样例/fixture、隔离基础设施、真实供应商与正式产品命令分别说明输入和条件。中断、Skip、缺少外部条件及未覆盖浏览器或目标设备均保留为未验收，不以历史截图或旧结果替代当前验证。

## 2026-10-07 全画板 mock 预览验收

用户追加明确授权“先完成所有页面的复现，功能和数据可以先 mock”。本轮完成 18 个独立预览路由（16 个产品画板、2 个规范画板），入口为 `/preview`，工作台为 `/preview/home`。以下是本轮最终状态；后面的第一轮记录保留真实业务接入范围及历史验证，不代表本轮预览仍缺少这些页面。

### 全画板对应关系

| Figma 节点 | 画板         | 预览路由                  | 本地演示交互                                                |
| ---------- | ------------ | ------------------------- | ----------------------------------------------------------- |
| `11:2`     | 首页与项目库 | `/preview/home`           | 名称、画幅、风格、文件选择、搜索、排序、网格/列表与项目跳转 |
| `3:2`      | 无限画布     | `/preview/canvas`         | 节点拖动/键盘移动、缩放、工具选择、生成面板与 Agent 弹窗    |
| `4:2`      | 镜头详情     | `/preview/shot`           | 镜头切换、参数、参考图、候选选定、配音与版本切换            |
| `5:2`      | 分镜故事板   | `/preview/storyboard`     | 分集、卡片/表格切换、勾选、批量操作与镜头跳转               |
| `6:2`      | 数据分析     | `/preview/analytics`      | 时间范围、图表提示、待处理项反馈与页面跳转                  |
| `7:2`      | 设定集       | `/preview/bible`          | 分类、搜索、未确认筛选、造型、别名、创建与授权声明弹窗      |
| `8:2`      | 素材库       | `/preview/assets`         | 类型/收藏筛选、搜索、预览、回收站与本地文件名称演示         |
| `9:2`      | 登录         | `/preview/login`          | 输入、密码显隐、错误提示与演示跳转                          |
| `10:2`     | 注册         | `/preview/register`       | 表单、密码显隐、一致性校验与演示跳转                        |
| `12:2`     | 设置新密码   | `/preview/reset-password` | 密码强度/一致性校验与演示反馈                               |
| `13:2`     | 账号设置     | `/preview/account`        | 名称、偏好、密码弹窗、会话与退出演示                        |
| `14:2`     | 管理账号     | `/preview/users`          | 搜索、角色/状态筛选、创建表单与账号状态切换                 |
| `16:2`     | 供应商凭据   | `/preview/providers`      | 更新密钥弹窗、取消与模拟测试反馈；输入不保存、不发送        |
| `17:2`     | 模型注册表   | `/preview/models`         | 分类/搜索、表单/JSON/历史切换、本地参数和发布状态           |
| `18:2`     | 审计日志     | `/preview/audit`          | 筛选、详情、变更对照与本地 CSV 导出                         |
| `19:2`     | 系统健康     | `/preview/health`         | 指标、队列、服务状态与本地刷新反馈                          |
| `1:2`      | 设计语言     | `/preview/design-system`  | 色板、字体、控件、状态与组件示例                            |
| `2:2`      | 布局规范     | `/preview/layout`         | 页框、导航、密度、展开/折叠布局对照                         |

### 组件与前端规范

- 继续使用用户指定的 shadcn 技能及 Vercel React Best Practices，读取适用的异步、客户端包、服务端序列化和派生状态规则；同时使用 Vercel 插件查询 Next.js 相关文档。安装目录缺少 `vercel:nextjs` 技能，具体 API 以本地安装的 Next.js 16.3.6 文档为据。
- 路由页面与元数据保持 Server Component；按页面组引入交互组件，图表依赖用于分析页面。没有为了预览添加请求层、缓存或业务服务。筛选和统计直接从当前状态计算。
- 新增官方 shadcn Avatar、Switch、Sonner、Chart，复用 Sidebar、Sheet、Dialog、Field、InputGroup、Select、ToggleGroup、Table、Empty。表面、警告、危险、选中态通过语义 token 与组件 variant 表达；布局类不重复定义控件外观。
- 补齐模态框标题、手机关闭按钮、图标按钮名称、表单校验和批量操作禁用状态。Figma 中的灰色媒体占位按设计呈现，没有添加无来源图片。

### 最终验证证据

在 `frontend/` 执行：

| 命令                                  | 结果                                       |
| ------------------------------------- | ------------------------------------------ |
| `pnpm exec eslint .`                  | 通过                                       |
| `pnpm exec prettier --check .`        | 通过                                       |
| `pnpm exec next typegen`              | 通过                                       |
| `pnpm exec tsc --noEmit`              | 通过                                       |
| `pnpm exec vitest run --maxWorkers=2` | 173 个测试文件、845 项测试全部通过，254 秒 |
| `pnpm exec next build`                | 通过；18 个预览页面及目录均为静态生成路由  |
| `git diff --check`                    | 通过                                       |

新增 4 项有行为意义的 mock 测试：注册密码不一致、创建账号无效时弹窗保留、空选择时批量操作禁用、设定集搜索与未确认筛选。第一次无 worker 限制的测试因并行任务资源竞争和超时被中断，不计为通过；最终采用上述 2 worker 完整重跑，无跳过测试。既有 jsdom 媒体 API 提示仍不构成真实媒体验收。

生产构建经 standalone 本地服务在 `127.0.0.1:3141` 验证。CUA 浏览器逐页核验全部 18 页，1440px 桌面及 390 × 844px 手机下均只有一个 `h1`，文档 `scrollWidth` 不大于 `clientWidth`；画布、表格按设计允许局部滚动。生产预览控制台没有 error/warn 记录。浏览器交互覆盖手机导航打开/关闭、同路由导航关闭、搜索空态、分镜选择与批量反馈、候选选定、模型 JSON 标签、供应商弹窗取消。没有进行真实账号、密钥或生成操作。

18 页桌面截图、手机首页截图与浏览器尺寸记录保存在本任务视觉产物目录：`/Users/stephenqiu/.codex/visualizations/2026/10/07/01a11698-5533-7e21-9d68-9fbecd76a8b3/`。文件为 `preview-<路由名>.png`、`preview-mobile-home.png`、`preview-browser-checks.json`；未加入 Git。

### 验收边界

- 已逐页读取 Figma 并作截图对照；MCP 额度仍不可用，没有取得完整结构化图层坐标，也没有运行自动逐像素差异比较。因此页面覆盖与浏览器检查通过，不宣称自动证明了零像素误差。
- 所有预览数据和操作为固定 fixture 与页面内 React 状态，刷新恢复。预览目录没有业务 `fetch`、生成 API、请求封装、localStorage 或 sessionStorage 调用。生成、播放、上传、认证和管理动作仅演示状态或反馈；画布关系线也是固定演示，不是完整图编辑引擎。
- 真实认证、业务数据、媒体播放处理、供应商、费用、权限和部署不属于本轮 mock 验收。已有真实业务页面的第一轮修改继续保留，不把其运行限制掩盖成 mock 成功。
- 没有新建分支、提交、推送、PR 或发布。工作区保留未提交修改，逐项清单见下方两轮记录。

## 2026-10-07 第一轮真实业务页面视觉对齐记录

设计来源为 [浮光 · 视觉重设计](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/)。Figma MCP 的 Starter 额度耗尽，`get_metadata` 与 `get_design_context` 均未成功；用户明确同意使用桌面端继续。实际通过 Figma 原生界面与独立的 Figma 网页视图读取画板、颜色标注和截图。以下是已有业务页面的实现进展，不能视为 18 个画板的像素级复现验收。

### 页面对应关系

| Figma 节点                    | 画板                             | 当前结果与边界                                                                                                                                                     |
| ----------------------------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `1:2`、`2:2`                  | 设计语言、布局规范               | 暗色 token、72px 一级导航、220px 上下文导航、字号与圆角统一；手机使用 shadcn Sheet。                                                                               |
| `11:2`                        | 首页与项目库                     | 首页创建区、9:16/16:9 选择、风格、五列竖版项目卡片；保留真实目录、分页和管理入口。后端不支持 1:1，未添加虚假选项。文件导入仍需先创建项目，再进入现有剧本导入流程。 |
| `3:2`                         | 无限画布                         | 共用导航、可折叠的画布管理区、节点表面色；真实画布读取、保存与未保存保护保持。编辑器工具布局仍有差异，尚未逐元素复现。                                             |
| `7:2`                         | 设定集                           | 接入项目上下文导航与统一视觉基础；详情内容沿用现有真实业务字段，未完成全画板对照。                                                                                 |
| `8:2`                         | 素材库                           | 目录与收藏/最近筛选侧栏、类型切换、响应式媒体卡片、折叠高级筛选；沿用已有详情弹窗，尚未实现稿中的常驻右侧预览。                                                    |
| `16:2`                        | 供应商凭据                       | 卡片目录与统一管理导航；保留现有凭据管理、并发配置与权限边界。当前运行身份没有管理员权限，成功态未做真实浏览器验收。                                               |
| `17:2`                        | 模型注册表                       | 管理入口、分区标题、主题与按需加载；模型详细表单仍使用现有实现，未逐项完成视觉验收。                                                                               |
| `4:2`、`5:2`、`6:2`           | 镜头详情、分镜故事板、数据分析   | 当前没有对应独立路由，本次没有新建业务模块或伪造数据。                                                                                                             |
| `9:2`、`10:2`、`12:2`、`13:2` | 登录、注册、设置新密码、账号设置 | 当前没有这些身份页面，未扩展认证流程。                                                                                                                             |
| `14:2`、`18:2`、`19:2`        | 管理账号、审计日志、系统健康     | 当前没有对应管理页面，未扩展后端合同与权限能力。                                                                                                                   |

### 实现与规范审查

- 使用本地 shadcn 技能，新增官方 Radix Sidebar、Sheet、InputGroup、Collapsible；未覆盖已有组件的业务定制。卡片、按钮、状态标签、输入与弹窗的视觉差异收敛在基础组件及语义变量中。
- 使用 Vercel `react-best-practices` 的异步、包体积、服务端序列化与派生状态规则。安装目录没有 `vercel:nextjs` 技能，API 核验采用项目安装的 Next.js 16.3.6 文档；没有变更框架或请求工具链。
- 设置分区动态导入；直接进入供应商页不挂载项目选择器，也不发起无关项目模型查询。默认模型、提示词在首次访问后保持挂载，保留跨分区未保存草稿。首页保持 Server Component 组合与 Suspense，实际交互位于 Client Component。
- 继续使用现有生成 API、TanStack Query 与身份/组织/项目缓存边界。未编辑 `src/api`、请求封装、后端接口或供应商密钥。

### 验证证据

在 `frontend/` 执行 `pnpm exec eslint .`、`pnpm exec prettier --check .`、`pnpm exec next typegen`、`pnpm exec tsc --noEmit`、`pnpm exec next build` 均通过。生产构建使用现有 standalone 输出，本地通过 `HOSTNAME=127.0.0.1 PORT=3141 node .next/standalone/server.js` 预览；这不是 Vercel 部署。

`pnpm exec vitest run` 最终结果为 172 个测试文件、841 项测试全部通过。新增首页校验/导入分支、预填创建合同、设置分区草稿保留回归测试；已有画布离开保护断言保留，素材测试显式模拟桌面断点。测试环境仍有既有的 jsdom 媒体 pause/load 未实现提示及 Vite 配置加载器提醒，不能以单元测试替代真实媒体验收。

浏览器通过 CUA 的 Playwright 接口验证：1440px 桌面与 390px 手机，首页和素材页没有文档横向溢出；首页名称校验会聚焦错误字段；首页名称/画幅进入创建弹窗，导入入口显示“创建并导入剧本”；手机导航能打开并在选择素材后关闭；暗色/浅色切换正常；真实项目现有画布可读取，管理面板可折叠；供应商页展示服务端真实的管理员权限拒绝。没有通过浏览器新建项目、提交凭据或触发付费生成。

运行日志暴露的 Select 非受控/受控切换已改为始终使用字符串值；封面由 IntersectionObserver 控制授权读取后，图片立即加载，避免第二次懒加载。

当前真实环境限制：素材容量接口返回服务不可用，页面显示失败和重试；管理员成功态尚无授权身份可验收。桌面截图与手机截图保存于 Codex 本次视觉产物目录，不进入 Git。其余画板和逐像素差异仍需继续验证。

### 未提交文件逐项清单

本次没有创建分支、提交、推送、PR 或发布。以下清单列出本次在 `main` 上保留的修改及用途；并行任务的后端测试和发布文档改动未被本任务覆盖。

- `DESIGN.md`：记录 Figma 来源、视觉 token 与布局尺寸。
- `design-qa.md`：记录页面覆盖、检查结果、未验收范围及逐项修改清单。
- `frontend/src/app/canvas/page.tsx`：接入页面组合、主题、品牌标题或项目共用导航。
- `frontend/src/app/globals.css`：统一暗色主题、表面色、提示色及圆角变量。
- `frontend/src/app/layout.tsx`：接入页面组合、主题、品牌标题或项目共用导航。
- `frontend/src/app/page.tsx`：接入页面组合、主题、品牌标题或项目共用导航。
- `frontend/src/app/projects/[projectId]/bible/page.tsx`：接入页面组合、主题、品牌标题或项目共用导航。
- `frontend/src/app/projects/[projectId]/canvas/page.tsx`：接入页面组合、主题、品牌标题或项目共用导航。
- `frontend/src/app/projects/[projectId]/layout.tsx`：接入页面组合、主题、品牌标题或项目共用导航。
- `frontend/src/app/projects/[projectId]/script/page.tsx`：接入页面组合、主题、品牌标题或项目共用导航。
- `frontend/src/app/providers.tsx`：接入页面组合、主题、品牌标题或项目共用导航。
- `frontend/src/components/bible/project-bible-workspace.tsx`：适配项目共用布局，保持单一 main 与页面标题。
- `frontend/src/components/canvas/nodes/node-shell.tsx`：统一画布工作区布局与节点表面，保留编辑与未保存保护。
- `frontend/src/components/canvas/workspace.test.tsx`：保留原有业务断言，覆盖新增交互或适配新的页面入口/断点。
- `frontend/src/components/canvas/workspace.tsx`：统一画布工作区布局与节点表面，保留编辑与未保存保护。
- `frontend/src/components/catalog/settings-providers.tsx`：统一管理分区、供应商卡片及按需加载，保留实际表单合同。
- `frontend/src/components/catalog/settings-workspace.test.tsx`：保留原有业务断言，覆盖新增交互或适配新的页面入口/断点。
- `frontend/src/components/catalog/settings-workspace.tsx`：统一管理分区、供应商卡片及按需加载，保留实际表单合同。
- `frontend/src/components/media/assets-workspace.test.tsx`：保留原有业务断言，覆盖新增交互或适配新的页面入口/断点。
- `frontend/src/components/media/assets-workspace.tsx`：调整素材导航、筛选、卡片或容量展示，保留真实权限与写入流程。
- `frontend/src/components/media/library-browser-purge.test.tsx`：保留原有业务断言，覆盖新增交互或适配新的页面入口/断点。
- `frontend/src/components/media/library-browser-transfer.test.tsx`：保留原有业务断言，覆盖新增交互或适配新的页面入口/断点。
- `frontend/src/components/media/library-browser.tsx`：调整素材导航、筛选、卡片或容量展示，保留真实权限与写入流程。
- `frontend/src/components/media/library-filters.tsx`：调整素材导航、筛选、卡片或容量展示，保留真实权限与写入流程。
- `frontend/src/components/media/library-folder-card.tsx`：调整素材导航、筛选、卡片或容量展示，保留真实权限与写入流程。
- `frontend/src/components/media/library-items.tsx`：调整素材导航、筛选、卡片或容量展示，保留真实权限与写入流程。
- `frontend/src/components/media/library-storage-meter.tsx`：调整素材导航、筛选、卡片或容量展示，保留真实权限与写入流程。
- `frontend/src/components/media/library-transfer-focus.test.tsx`：保留原有业务断言，覆盖新增交互或适配新的页面入口/断点。
- `frontend/src/components/project/create-project-dialog.test.tsx`：保留原有业务断言，覆盖新增交互或适配新的页面入口/断点。
- `frontend/src/components/project/create-project-dialog.tsx`：复现首页与项目库布局，连接现有创建、目录、封面和管理流程。
- `frontend/src/components/project/project-cover-preview.tsx`：可见性与授权读取完成后立即加载图片，避免二次懒加载延迟；保留过期和重试规则。
- `frontend/src/components/project/project-folder-card.tsx`：复现首页与项目库布局，连接现有创建、目录、封面和管理流程。
- `frontend/src/components/project/project-list.tsx`：复现首页与项目库布局，连接现有创建、目录、封面和管理流程。
- `frontend/src/components/project/project-start.test.tsx`：保留原有业务断言，覆盖新增交互或适配新的页面入口/断点。
- `frontend/src/components/project/project-start.tsx`：复现首页与项目库布局，连接现有创建、目录、封面和管理流程。
- `frontend/src/components/project/projects-workspace.tsx`：复现首页与项目库布局，连接现有创建、目录、封面和管理流程。
- `frontend/src/components/script/project-script-workspace.tsx`：适配项目共用布局，保持单一 main 与页面标题。
- `frontend/src/components/theme-toggle.tsx`：按基础按钮组件规范处理明暗主题图标。
- `frontend/src/components/ui/badge.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/ui/button.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/ui/card.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/ui/collapsible.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/ui/dialog.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/ui/input-group.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/ui/input.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/ui/sheet.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/ui/sidebar.tsx`：新增或规范 shadcn 语义组件、视觉变体及可访问名称。
- `frontend/src/components/workbench/home-dashboard.tsx`：统一一级/上下文导航、手机抽屉与首页组合。
- `frontend/src/components/workbench/workspace-shell.tsx`：统一一级/上下文导航、手机抽屉与首页组合。
- `frontend/src/hooks/use-mobile.ts`：通过外部媒体查询订阅提供响应式断点，包含监听清理。

### 全画板预览追加的未提交文件

以下与第一轮清单合并覆盖当前全部未提交文件；第一轮已列出的 `DESIGN.md`、`design-qa.md`、主题、Card、Dialog 和工作区导航同时继续更新，用于记录 mock 范围、统一样式并提供预览入口。

- `frontend/package.json`：添加官方 shadcn 图表和通知所需的 recharts、sonner 依赖。
- `frontend/pnpm-lock.yaml`：锁定新增依赖及其传递依赖。
- `frontend/src/components/ui/alert.tsx`：补充语义化警告与危险提示变体。
- `frontend/src/components/ui/select.tsx`：对齐设计稿选择器圆角。
- `frontend/src/components/ui/table.tsx`：统一表格行的弱分隔线。
- `frontend/src/app/preview/account/page.tsx`：提供账号设置的服务端路由组合与页面元数据。
- `frontend/src/app/preview/analytics/page.tsx`：提供数据分析的服务端路由组合与页面元数据。
- `frontend/src/app/preview/assets/page.tsx`：提供素材库的服务端路由组合与页面元数据。
- `frontend/src/app/preview/audit/page.tsx`：提供审计日志的服务端路由组合与页面元数据。
- `frontend/src/app/preview/bible/page.tsx`：提供设定集的服务端路由组合与页面元数据。
- `frontend/src/app/preview/canvas/page.tsx`：提供无限画布的服务端路由组合与页面元数据。
- `frontend/src/app/preview/design-system/page.tsx`：提供设计语言的服务端路由组合与页面元数据。
- `frontend/src/app/preview/health/page.tsx`：提供系统健康的服务端路由组合与页面元数据。
- `frontend/src/app/preview/home/page.tsx`：提供首页与项目库的服务端路由组合与页面元数据。
- `frontend/src/app/preview/layout.tsx`：设置预览暗色范围、图表 token 与通知容器。
- `frontend/src/app/preview/layout/page.tsx`：提供布局规范的服务端路由组合与页面元数据。
- `frontend/src/app/preview/login/page.tsx`：提供登录的服务端路由组合与页面元数据。
- `frontend/src/app/preview/models/page.tsx`：提供模型注册表的服务端路由组合与页面元数据。
- `frontend/src/app/preview/page.tsx`：提供全部 18 个画板目录及 Figma 对照入口。
- `frontend/src/app/preview/providers/page.tsx`：提供供应商凭据的服务端路由组合与页面元数据。
- `frontend/src/app/preview/register/page.tsx`：提供注册的服务端路由组合与页面元数据。
- `frontend/src/app/preview/reset-password/page.tsx`：提供设置新密码的服务端路由组合与页面元数据。
- `frontend/src/app/preview/shot/page.tsx`：提供镜头详情的服务端路由组合与页面元数据。
- `frontend/src/app/preview/storyboard/page.tsx`：提供分镜故事板的服务端路由组合与页面元数据。
- `frontend/src/app/preview/users/page.tsx`：提供管理账号的服务端路由组合与页面元数据。
- `frontend/src/components/preview/account-page.tsx`：复现账号设置、偏好和本地会话演示。
- `frontend/src/components/preview/admin-pages.tsx`：复现账号、供应商、模型、审计和健康五个管理页面及本地交互。
- `frontend/src/components/preview/analytics-page.tsx`：复现指标、趋势图、模型结果、阶段进度和待办。
- `frontend/src/components/preview/auth-pages.tsx`：复现登录、注册、重置密码布局及本地表单校验。
- `frontend/src/components/preview/canvas-page.tsx`：复现浮动画布工具、演示节点、关系线和生成面板。
- `frontend/src/components/preview/design-pages.tsx`：复现设计语言及布局规范两张参考画板。
- `frontend/src/components/preview/home-page.tsx`：复现项目创建区、卡片网格和筛选演示。
- `frontend/src/components/preview/library-pages.tsx`：复现设定集和素材库的列表、详情与本地交互。
- `frontend/src/components/preview/preview-interactions.test.tsx`：覆盖密码一致性、无效表单、批量禁用和组合筛选行为。
- `frontend/src/components/preview/screens.ts`：集中定义 18 个画板、路由及 Figma 节点对应关系。
- `frontend/src/components/preview/shared.tsx`：复用导航、侧栏、抽屉、控件、弹窗和演示反馈。
- `frontend/src/components/preview/storyboard-pages.tsx`：复现分镜故事板与镜头详情、候选和版本选择。
- `frontend/src/components/ui/avatar.tsx`：新增并规范官方 shadcn 头像组件。
- `frontend/src/components/ui/chart.tsx`：新增官方 shadcn 图表容器与工具提示。
- `frontend/src/components/ui/sonner.tsx`：新增官方 shadcn 通知容器。
- `frontend/src/components/ui/switch.tsx`：新增官方 shadcn 开关组件。
