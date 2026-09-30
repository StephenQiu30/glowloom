# F1 前端工作台 PoC 记录

日期：2026-09-30。结论：**页面与交互技术验证通过，真实生成闭环与产品验收未完成。** 本轮遵循用户指定的完整前端 → 对应服务 → PoC 验证 → 消费者能力顺序，独立 Agent 不作为页面前置。

事实来源：[本轮 Design](../design/BeefTV能力引入设计.md)、[BeefTV 调研](../prd/BeefTV能力调研与借鉴评估.md)、[界面需求](../requirement/05-界面与交互需求.md)、[视觉规范](../../DESIGN.md)。开始时位于 Git 根目录，`main` / `b75bdcc90666cc07380cf872f4dfeea1d13ad3b6`，工作区干净；全部任务改动属于本轮。

## 页面与交互

- 项目：搜索、卡片 / 表格、配置预览、阶段矩阵、预算、近期任务、项目 / 单集切换。
- 创作：剧本草稿、原文与解析对照、设定集分类和版本、单集资产与授权状态、分镜、故事板、镜头参数 / 引用、候选比较与选定影响、配音配置。
- 素材与恢复：媒体类型 / 搜索 / 预览 / 来源 / 审核 / 引用，任务详情、部分失败与提交未知的不同下一步，变更影响与成本明细。
- 画布：5 节点 / 4 连线的业务关系预览，可移动、缩放、打开关联页面；独立 `/poc/canvas` 的性能样本未修改。
- 内部界面：登录入口、账号偏好、通知、用户权限、供应商、模型能力 / 价格、审计和依赖状态。
- 状态：正常、加载、空、读取失败、无权限与只读；搜索 / 分类 / 候选放在 URL，刷新恢复。本页编辑明确提示刷新丢弃。

全部业务界面显示样例与未接服务。报价复用 QuoteConfirmDialog 的 `previewOnly`；确认生成与强制重新生成始终禁用。文本解析、视频、配音展示不同样例模型类型，金额不是实际供应商报价。无新增 HTTP mock 服务、手写公共 API 客户端、真实登录、后台轮询或付费调用。

## 自动检查

工作目录 `frontend/`，最终代码执行：

| 命令                                  | 结果                                              |
| ------------------------------------- | ------------------------------------------------- |
| `pnpm exec vitest run`                | 10 个测试文件、32 项测试通过                      |
| `pnpm exec eslint . --max-warnings=0` | 通过，0 错误 / 0 警告                             |
| `pnpm exec next typegen`              | 路由类型生成通过                                  |
| `pnpm exec tsc --noEmit`              | 通过                                              |
| `pnpm exec next build`                | 生产构建通过                                      |
| 新增 / 修改任务文件的 Prettier 检查   | 通过；保留已有 docs/README 表格格式，未全文件重排 |
| `git diff --check`                    | 通过                                              |

Red 证据：`routes.test.ts` 与 `shot-page.test.tsx` 在实现模块缺失时分别失败，随后通过。测试覆盖未知对象拒绝、URL 参数保留、只读阻断、候选影响预览、本页草稿、失败读取重试和报价确认 / 强制重新生成回调阻断。首次测试命令曾因目录选择错误未发现文件，已纠正，不把该结果当作 Red 业务证据。

Vitest 输出已有 Vite 配置将来切换 native loader 的提示，测试实际执行并通过，未改无关工具链。没有修改 Go / Python，因此未运行后端与 Agent 门禁；没有核验远端 CI，不声明项目全量门禁通过。

## 浏览器与 HTTP 证据

用 agent-browser 独立会话 `lanverse-front-poc`，本机 Chromium / Next standalone 生产构建，测试地址 `http://127.0.0.1:3139`。启动命令：`PORT=3139 HOSTNAME=127.0.0.1 node .next/standalone/server.js`；在忽略的构建目录链接现有 public / static。初次 `next start` 提示 standalone 应使用 server.js，已切换后重测。

- 27 个实际页面导航：都有主要标题 / PoC 标识；1280 × 900 下页面没有横向溢出。
- 浅色 27 页 axe 4.12.1：0 violation；画布有 1 个 incomplete 分类，自动工具无法确认 SVG / 变换节点背景对比度。已人工查看截图，仍不声明完整 WCAG 符合性。
- 深色 6 个代表页与加载 / 空 / 错误 / 无权限共 10 项 axe 检查：0 violation；画布 1 个 incomplete。
- 21 项浏览器交互通过：搜索 / 表格刷新恢复；候选 B 深链接；影响预览；报价确认阻断与剔除恢复；刷新丢弃草稿；只读编辑阻断；读取重试；加载 / 空 / 无权限；单集与侧栏同步；音频空状态；视频原生控件 / 不自动播放；unknown 费用未决；部分失败保留成功项；本页价格估算；登录不伪装成功；画布 5 / 4；主题切换；无未捕获页面错误。
- 6 个未知对象 / 页面路径返回 HTTP 404；内置 SVG 与 MP4 返回 200、相应媒体 Content-Type。404 验证最初发现父 Suspense 提前流式发送 200，已把边界移至静态页面后重测通过。

脚本及原始结果位于本机 `/tmp/lanverse-front-poc-{audit,interactions,dark-state}.{py,json}`，属于临时验证证据，不提交。只访问本项目样例，不读取浏览器凭据。

| 证据                 | 数量 / 结果                    |
| -------------------- | ------------------------------ |
| 浅色实际页面         | 27 / 0 violation、0 页面溢出   |
| 深色代表页与异常状态 | 10 / 0 violation、1 incomplete |
| 交互检查             | 21 / 全部通过                  |
| 无效链接 HTTP        | 6 / 全部 404                   |

27 页覆盖：`/login`、`/projects`、`/projects/harbor`、`script`、`bible`、`episodes/ep-01/{parse,assets,shots,storyboard,audio}`、`episodes/ep-01/shots/shot-01`、`canvas`、`media`、`impact`、`costs`、`settings`、`/tasks`、`/tasks/task-01`～`task-04`、`/account`、`/admin/{users,providers,models,audit,health}`。项目内简写均相对 `/projects/harbor/`。

## 界面截图

桌面宽度 1440px，位于当前任务视觉产物目录：

- [项目工作台](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f031-54b2-72e1-8595-f95179bf2011/lanverse-projects-poc.png)
- [项目概览](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f031-54b2-72e1-8595-f95179bf2011/lanverse-overview-poc.png)
- [镜头与候选](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f031-54b2-72e1-8595-f95179bf2011/lanverse-shot-poc.png)
- [报价预览](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f031-54b2-72e1-8595-f95179bf2011/lanverse-quote-poc.png)
- [创作关系画布](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f031-54b2-72e1-8595-f95179bf2011/lanverse-canvas-poc.png)
- [深色概览](/Users/stephenqiu/.codex/visualizations/2026/09/30/01a0f031-54b2-72e1-8595-f95179bf2011/lanverse-overview-dark-poc.png)

## 服务阶段与未决边界

F1 证明页面覆盖与受控交互，不证明后端鉴权、保存、文件上传、审核、供应商生成、Temporal / SSE 恢复、真实价格 / 费用、候选正式选定或完整创作质量。没有真实模型、存储、服务端账本和用户验收证据；独立性能画布的指标本轮没有重新测量。移动端产品和消费者服务留在后续。

F2 优先评估 Go 统一服务：现有 Workflow 已在 Go，重点迁移 Provider / Skill Activity、结构化模型调用、凭据 / 限流 / 审核职责；保留未决提交、幂等、账本、任务回放和取消合同。此设计不代表 Agent 已移除，仍需具体架构决定与真实联调。

## Git 与任务改动

用户已授权提交到 `main` 并推送，按测试 → 实现 → 文档分开提交：测试 `3913eb4a`，实现 `5744da3e`。文档提交自身的 SHA 与推送后的远端 CI 结果见本次交付答复，避免在文档内循环引用自身。没有新建分支、PR、标签或部署；依赖清单与锁文件无变化。本机预览仅监听 127.0.0.1。

以下逐项列明相对起始提交的 46 项任务改动（包含原未跟踪文件），其他路径未修改：

- `docs/README.md`：修改
- `frontend/src/app/globals.css`：修改
- `frontend/src/app/projects/layout.tsx`：修改
- `frontend/src/app/projects/page.tsx`：修改
- `frontend/src/features/operation/quote-confirm-dialog.test.tsx`：修改
- `frontend/src/features/operation/quote-confirm-dialog.tsx`：修改
- `docs/acceptance/F1-前端工作台PoC记录.md`：新增
- `docs/design/BeefTV能力引入设计.md`：新增
- `docs/prd/BeefTV能力调研与借鉴评估.md`：新增
- `frontend/src/app/account/layout.tsx`：新增
- `frontend/src/app/account/page.tsx`：新增
- `frontend/src/app/admin/[section]/page.tsx`：新增
- `frontend/src/app/admin/layout.tsx`：新增
- `frontend/src/app/admin/page.tsx`：新增
- `frontend/src/app/login/page.tsx`：新增
- `frontend/src/app/not-found.tsx`：新增
- `frontend/src/app/projects/[projectId]/[[...section]]/page.tsx`：新增
- `frontend/src/app/tasks/[taskId]/page.tsx`：新增
- `frontend/src/app/tasks/layout.tsx`：新增
- `frontend/src/app/tasks/page.tsx`：新增
- `frontend/src/components/ui/badge.tsx`：新增
- `frontend/src/components/ui/card.tsx`：新增
- `frontend/src/components/ui/dialog.tsx`：新增
- `frontend/src/components/ui/empty.tsx`：新增
- `frontend/src/components/ui/input.tsx`：新增
- `frontend/src/components/ui/select.tsx`：新增
- `frontend/src/components/ui/skeleton.tsx`：新增
- `frontend/src/components/ui/table.tsx`：新增
- `frontend/src/components/ui/tabs.tsx`：新增
- `frontend/src/components/ui/textarea.tsx`：新增
- `frontend/src/components/ui/toggle-group.tsx`：新增
- `frontend/src/components/ui/toggle.tsx`：新增
- `frontend/src/features/workbench/admin-pages.tsx`：新增
- `frontend/src/features/workbench/asset-pages.tsx`：新增
- `frontend/src/features/workbench/creation-canvas.tsx`：新增
- `frontend/src/features/workbench/data.ts`：新增
- `frontend/src/features/workbench/poc-components.tsx`：新增
- `frontend/src/features/workbench/project-pages.tsx`：新增
- `frontend/src/features/workbench/project-screen.tsx`：新增
- `frontend/src/features/workbench/routes.test.ts`：新增
- `frontend/src/features/workbench/routes.ts`：新增
- `frontend/src/features/workbench/script-pages.tsx`：新增
- `frontend/src/features/workbench/shot-page.test.tsx`：新增
- `frontend/src/features/workbench/shot-pages.tsx`：新增
- `frontend/src/features/workbench/task-pages.tsx`：新增
- `frontend/src/features/workbench/workspace-shell.tsx`：新增
