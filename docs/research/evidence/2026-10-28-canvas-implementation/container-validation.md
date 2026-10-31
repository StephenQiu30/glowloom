# 普通组与堆叠验证

日期：2026-10-29。合同为设计27 §7.4.12、PRD14、Plan23；source事实见 `advanced-source/group-followup-20261029.md`。共享工作区未提交。

## 实现与持久化

- `config.container={version:1,kind:group|stack,background:none|cyan|blue|purple|orange|yellow}` 为封闭配置；非group节点、错误版本/kind/背景拒绝。旧collapsed group仍是普通组，搜索/a11y明确区分堆叠。
- 新建仅重挂选中根，保留内层容器/成员/边；解除提升直接成员到原父级，保留绝对几何；移动递归平移一次；删除清理整个后代树，Yjs撤销恢复真实annotation连线与节点；复制沿正式剪贴板递归ID映射。
- stack成员仅隐藏：剔除渲染、连线命中、框选、缩略图及fit范围；内部普通组不能成为外部拖入目标。undo/remote恢复后选区同步派生到最外可见stack，避免隐藏成员多选工具栏。
- 普通组工具栏独立于workflow开关，保存布局与六背景；group/stack排除通用hover工具栏，保证单一操作栏。画廊展示真实原文本/授权媒体，以居中原比例卡片呈现。
- 下载复用正式原媒体授权与文本下载；不支持工具类型显式拒绝。浏览器多文件下载限制及真实下载产物尚未验收，不据按钮可见宣称完成。

## 可执行检查

- 容器/画廊/搜索/几何/服务图/a11y/theme 8文件46tests通过；追加隐藏父容器命中、undo后选择派生red→green；Yjs+容器56tests通过（后续又新增1选择测试）。
- `pnpm --dir frontend exec tsc --noEmit` 通过；容器、editor、几何、service-graph、搜索、a11y、toolbar修改范围ESLint零警告。
- `go vet ./internal/canvas/... ./tests/canvas/...`、`golangci-lint run ./internal/canvas/... ./tests/canvas/...`：0 issues。
- `TestCanvasContainer*` 在专属隔离PG18数据库 `glowloom_canvas_director_tests`、runtime `glowloom_app`（非superuser）下 `-race` 通过2.574s；实际新建嵌套普通组/stack/文本、接受Yjs并从新服务回读父树、类型、背景、几何与原文；Yjs投影更新/非法kind拒绝亦通过。
- 主代理浏览器实测：普通组紫色+宫格保存→刷新紫色勾选保留；解除→stack→刷新→画廊2成员→解除→undo恢复；隐藏选区修复后再次解除→保存→undo，只显示所选stack及其工具栏。详细截图/回执由 `browser-integration` 证据维护，不把unit测试当浏览器证据。

## 原生Figma

应用 `figma-use`、`figma-generate-design`；先检查Code Connect（无对应文件）、库（无已接入库）、搜索（无远程匹配），再复用本文件canvas shell/Button组件与颜色/间距/圆角变量。组件主节点没有改写。

文件 `uLqmeRWuQuzfrp9xOYg6FT`，page `1361:10`，composition [3142:50965](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3142-50965)，x229000，与CinemaKit隔离。

| 状态 | 原生frame |
| --- | --- |
| 普通组选中操作 | 3142:50974 |
| 宫格/智能布局菜单 | 3142:51173 |
| 六背景菜单与紫色组 | 3142:51391 |
| 独立stack与画廊入口 | 3142:51645 |
| 全屏居中原内容画廊 | 3142:51826 |
| 两空图片分镜禁用 | 3142:51837 |

最终读回824 descendants：112 TEXT、283 FRAME、143 INSTANCE、280 VECTOR、6 ELLIPSE，0 IMAGE，不含完整UI栅格图。字体明确为既有产品的Noto Sans SC及Geist；原生成员文本卡片主组件3142:50968，共11实例。最终截图 `container-figma-native.png` 已目视检查；先修透明wrapper标题对比度与缺少色块，再导出最终稿。

边界：源普通组各色数值/智能布局算法没有实测，使用既有六色token和可撤销无重叠布局是织光实现选择。分镜ready-image成功态仍未观察，不以普通组/宫格冒充分镜；Figma六状态不代表该能力完成。源文本堆叠右键下载禁用，工具栏下载可见但产物未测；目前实现按正式下载合同支持文本和媒体，保留这一差异待验收。

全canvas+theme扩展检查：60文件344tests全部通过，但assistant-panel.test.tsx存在Radix focus-scope延迟事件的跨JSDOM Event未捕获错误，进程exit1；已交模块owner独立复验，本次全量进程不记为绿。
