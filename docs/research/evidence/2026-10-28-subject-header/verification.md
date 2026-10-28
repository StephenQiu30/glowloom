# 主体顶栏同排验收 · 2026-10-28

对应 SUB-014 / AC-SUB-17，来源为本轮用户截图及同排要求。

## 设计与实现

- ProductHeader 增加页面工具宿主；PageToolbar 使用 React portal，主体组件持有原筛选、分页、批量选择及请求状态。没有新增依赖或复制业务状态。
- 桌面筛选/时间/排序在左，批量操作在公共搜索/主题/通知/账号之前；正文只保留已应用日期、卡片、分页。项目历史主体移到列表后。
- 手机公共操作行与工具行分开，工具按钮至少44px；不足宽度自然换行。
- Figma 调整36个具有主体工具栏的桌面状态及9个手机状态；共享 Header 主组件不修改，复用既有实例。菜单锚点随工具栏移动。
- 主稿：[桌面](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=1446-3086)、[手机](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=2749-44447)。本次是工具栏位置修订，不重新声明全领域验收。

## 实际浏览器

登录中的 localhost:1122/subjects，使用既有主体「主体对齐本地验收-20261028」。

| 检查 | 实际结果 |
| --- | --- |
| 桌面1869px | 筛选/时间/排序/批量/头像均 top=14、height=36，垂直中心32；Header高64 |
| 筛选弹层 | 顶栏触发器展开收藏菜单；Escape关闭 |
| 日期 | 最近一周应用后范围显示正文；清除恢复列表 |
| 批量 | 普通工具被所选数量及删除/下载/收藏/取消替换；取消返回普通工具 |
| 手机390×844 | 四个工具按钮均44px高，无横向溢出；批量动作自然换行 |
| 窄屏320×568 | 无横向溢出；批量入口换行、正文可滚动 |
| 路由 | 切到探索后主体工具全部移除，公共账号保留；返回主体正常挂载 |

浏览器视口模拟已解除，最终留在桌面主体页。未创建、删除或修改主体数据。

## 工程验证

- 定向 Vitest：4文件42测试通过，覆盖主体库、日期、BasicLayout与ProductSearch；新增顶栏批量状态/卸载及嵌入内联测试。
- TypeScript、全量ESLint及本轮文件Prettier通过。
- 全量Prettier未通过：6个非本轮修改文件原有格式差异，未覆盖其他任务工作。具体为 asset-library-command-dialog.tsx、ui/breadcrumb.tsx、ui/progress.tsx、tests/browser/serve-assets.mjs、tests/unit/app/api/[...path]/route.test.ts、tests/unit/components/canvas/publish-media.test.ts。
- 全量 Vitest：156文件、1028测试通过（32.18s）。
- next typegen、tsc --noEmit、next build 全部通过；构建完成34个静态预渲染页面。
- git diff --check 通过。

## Git 与工作区

本次未提交或推送。共享工作区另有画布功能与验收文档修改；没有暂存、覆盖或回滚它们。逐项状态见[工作区记录](worktree-status.md)。本次不涉及部署或 TOS/豆包真实凭据验证。
