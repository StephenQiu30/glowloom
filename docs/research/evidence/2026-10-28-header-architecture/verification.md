# 共享顶栏架构验证 · 2026-10-28

## 范围与实现

原始依据为[即梦共享顶栏逆向](../../2026-10-28-即梦共享顶栏架构逆向.md)，项目合同为设计09 §7.6、FR-WB-19/AC-WB-19、SUB-014/AC-SUB-17。

- ProductHeaderActions组合公共搜索、主题、消息、AccountMenu，移除ProductHeader对assets路由的判断；搜索禁用由页面PageSearch合同声明。
- PageToolbar兼容原navigation API，并提供actions与adjacent/replace-base。页面拥有业务状态；Shell仅持有宿主与owner/presentation。无Provider内联回退，宿主待挂载时暂不显示。
- 主体批量保留筛选，公共组卸载并清理搜索弹层/快捷键；取消恢复公共组与入口焦点。移动端用稳定key调整DOM顺序，portal宿主和组件状态保持；资产TabsList上下文继续有效。
- 不涉及后端、凭据、数据库和外部发布；未提交或推送本轮改动。

## 真实执行

| 检查 | 结果 |
| --- | --- |
| pnpm exec eslint . | 通过 |
| pnpm exec prettier --check . | 失败：6个既有无关文件，见下方；没有跳过后宣称通过 |
| 本次10个前端修改/新增文件的prettier检查 | 通过；最终CSS修订后再次通过 |
| pnpm exec next typegen；pnpm exec tsc --noEmit | 通过 |
| 4个相关单测文件 | 59项通过 |
| pnpm exec vitest run --maxWorkers=2 | 157个文件、1032项通过；保留原有jsdom导航/TimeoutNaN警告 |
| pnpm exec next build | 通过；最终CSS改动后重新构建通过 |
| git diff --check | 通过 |

全量格式失败文件：

- src/components/canvas/runtime/components/assets/library/asset-library-command-dialog.tsx
- src/components/ui/breadcrumb.tsx
- src/components/ui/progress.tsx
- tests/browser/serve-assets.mjs
- tests/unit/app/api/[...path]/route.test.ts
- tests/unit/components/canvas/publish-media.test.ts

完整执行日志在本机 `/tmp/glowloom-header-{eslint,prettier,vitest,build,target-tests}.log`，不是CI或部署证据。

## 浏览器

在已登录 localhost:1122/subjects 实测。详见[browser-layout.json](browser-layout.json)和[桌面截图](desktop.png)。

- 1869px：筛选、时间、排序、批量入口、公共操作的按钮均y=14、高36px，垂直中心32px。
- 390px普通：导航和公共组位于首行，三个筛选与批量入口位于第二行；顶栏116px，全部按钮44px，document.scrollWidth=390。
- 320px普通：三个筛选第二行、批量入口自然另起一行；document.scrollWidth=320，全部按钮44px。
- 390/320px批量：保留筛选，公共组不存在；动作自然换行，无页面横向溢出。进入时焦点落在取消选择，取消后回到批量入口。
- 批量时Ctrl+K不打开搜索对话框；退出后只打开一个搜索对话框，Escape可关闭。主体卡片和分页数据保持正常。
- 单测进一步验证在公共搜索已打开时撤销动作组会清除body弹层，路由卸载恢复公共组且无重复快捷键监听；断点改变保留同一portal节点与其计数状态。

本次没有向即梦提交数据；竞品运行观察覆盖桌面主体、资产、探索、创作及主体零选择批量，未覆盖其手机和画布。

## Figma

会话 `bd96e128-9445-4ddc-b35e-371d5f98b354`，文件 `uLqmeRWuQuzfrp9xOYg6FT`。

- 共享Header组件集 `620:24416`：补充双槽、公共组和生命周期合同，保留原账号插槽说明。
- 桌面批量 `2749:43334`：新增页面筛选槽 `3063:46581`，原动作区 `2749:43378`作为replace-base，公共实例 `2749:43375`隐藏。
- 手机批量 `2752:38310`：新顶栏容器 `3067:46571` 高220px，页面工具与批量动作进入同一容器；保留原页面标题，隐藏旧公共搜索入口。复用现有主体卡与Checked=On选择组件，44px选择热区 `3071:46587`。
- 原生稿已截图检查布局；这是设计态结构验证，不代表整个文件所有历史页面或代码原型已重新验收。

## 剩余工作区

工作区包含同时进行的资产与画布改动，未暂存、覆盖或回滚。逐项状态见[worktree-status.txt](worktree-status.txt)。本轮相关实现为layout组件、globals.css顶栏规则、主体工具与库、对应单测，以及需求/设计/PRD/Plan与本证据目录；同一文件中的并行资产或画布修改仍属于原任务。
