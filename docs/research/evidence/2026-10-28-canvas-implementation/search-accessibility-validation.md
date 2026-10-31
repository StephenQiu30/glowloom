# JC-055 / JC-060 搜索与可访问摘要验证

验证日期：2026-10-29。对应设计27 §7.4.10、PRD14 AC-CJC-20/24、Plan23 T-CJC-12/13。未提交Git；不改CanvasSidePanel资产管理、全局账号缓存或数据库。

## 来源与边界

- 研究10 JC-055 / E23及 `../2026-10-28-jimeng-canvas/23-search.txt` 记录早期文字称搜索侧面板、全部/图片/视频/音频/文本/主体/时间线/导演工具箱/组/其他、部分计数、节点名称与类型。该文本为观察整理，未输入查询、未点击结果验证定位，未观察正文/标签匹配。
- 研究10 JC-060 / E42及 `../2026-10-28-jimeng-canvas/42-text-edit.txt` 的AX记录包含节点类型、名称、选择状态、资源计数及文本双击编辑提示。它不能证明完整键盘或读屏验收。
- 名称去首尾空白、大小写无关包含匹配、查询后分类前计数、结果定位、关闭焦点是本项目明确实现合同。早期依据E23文字将其设为左侧Sheet的决定已被新源截图取代，现使用锚定搜索按钮下方的320px Popover。

## 正式代码路径

正式Yjs工作区使用 `yjs-workspace.tsx` → `CanvasTopBar`。原搜索按钮打开CanvasSidePanel canvas页；`chrome.tsx`的搜索仅由snapshot-viewer使用。现在正式TopBar独立接入 `search/canvas-node-search.tsx`，资产管理保持原实现。

- `canvas-node-search-state.ts` 从当前授权nodes实时计算七类及组/其他/全部；生成节点归相应媒体类，未知类型归其他。同名节点保留，空名有稳定显示。新源AX确认cinema_studio为导演工具箱；现有director归其他，节点与结果类型名称为导演台。只匹配名称，不匹配正文、prompt、标签和UUID。
- 查询、分类、结果、空态与清空均有原生语义。清空仅清名称、保留类型；协作新增/改名/删除来自最新props。结果点击在Popover关闭后确认节点仍存在，再调用既有focusNode；个人视口动画结束将焦点交给节点。Escape返回触发器。只读不阻止查找与定位，不新增共享图写入。
- `canvas-node-accessibility.ts` 为节点group生成名称和独立描述，包含选中、只读/可编辑、当前编辑、引用选择及已知资源状态。无就绪证据的绑定或URL显示未核验，固定主体版本不冒充ready。不输出正文、提示词、私有URL或内部ID。
- `canvas-node.tsx`的本轮改动仅涉及group/描述及文本交互语义；只读或引用选择时不再声称可按键编辑。既有标签、几何与保存不变。

## 已执行验证

命令在frontend目录执行：

```sh
pnpm exec vitest run tests/unit/components/canvas/canvas-node-search.test.tsx tests/unit/components/canvas/nodes/canvas-node-accessibility.test.tsx tests/unit/components/canvas/nodes/canvas-node-resize.test.tsx
pnpm exec tsc --noEmit
```

- Vitest：3文件13测试通过，0失败。日志 `/tmp/glowloom-search-aria-tests.log`。覆盖七类/组/未知类型、名称匹配边界、同名、实时改名删除、查询及分类计数、空态、清空、精确定位回调、Escape焦点返回、真实CanvasNode的ARIA关联描述、只读变化、资源状态与既有缩放几何回归。
- scoped ESLint `--max-warnings=0`：搜索两文件、可访问helper、topbar、editor、canvas-node及两新增测试通过。日志 `/tmp/glowloom-search-aria-eslint.log`。
- 全量TypeScript无输出通过：`/tmp/glowloom-search-aria-tsc.log`。
- `git diff --check`通过。

## 尚未证明

本轮未占用浏览器。实际Popover视觉、键盘逐项Tab/Enter、动画结束后焦点、真实协作新增与只读会话、屏幕阅读器仍需主任务浏览器/辅助技术验收。组件测试证明定位调用与Escape回焦，不冒充完整键盘或浏览器定位证据。旧snapshot-viewer搜索未在本轮统一。未实现来源未观察的正文/标签匹配；没有把这一项标成已复刻。

## 2026-10-29 新源Popover纠正

根任务新保存 `advanced-source/search-panel-20261029.png/txt`：在1869×873源视口，面板约x1363/y56、320×436；搜索框、横向单行分类、下一组按钮及五个编号图标结果。源director为外部节点，计入“其他1”；cinema_studio“导演工具箱”计数为0。新证据优先于E23早期整理措辞。

实现已替换Sheet为Popover、分类改Tabs（零值不显示视觉数字但保留可访问计数）、增加横向导航和编号类型图标，仍保留既有权限范围、名称匹配及定位/回焦合同。正式cinema_studio节点尚不存在，分类不伪造数据。现有director计入其他，名称为导演台。

回归：3文件13项通过，scoped ESLint及全量tsc通过；日志 `/tmp/glowloom-search-popover-tests.log`、`/tmp/glowloom-search-popover-lint.log`、`/tmp/glowloom-search-popover-tsc.log`。Tabs测试初次使用click未触发Radix的mousedown选择（1失败），更正为实际mousedown事件后通过；不是产品异常被跳过。

Figma在同文件/页面新增可编辑画布搜索状态 `3138:50775`，搜索Popover `3138:50931`、复用结果行组件 `3138:50960`。使用既有画布header/toolbar/viewport/AI/background和图标实例，236个后代（32实例、73矢量、85frame、46文字），无图片填充，无整屏栅格导入。字体回读全部Noto Sans SC，保持既有参考稿约定；源PingFang不可用差异保留。截图 `figma-search-popover.png`。源截图中的品牌/视口图标与既有项目组件差异保留，不能宣称整屏像素一致。此稿证明搜索布局与原生可编辑结构，不是浏览器操作接受。

分类导航图标首次补写工具返回Transport closed；只读回查确认未落节点后才重试，成功返回3139:50965，最终截图已更新，未重复生成搜索帧。

## 2026-10-29 搜索媒体缩略图专项

本轮只修改 `search/canvas-node-search.tsx`、对应测试和设计27搜索合同；没有修改session/editor/backend。旧SearchPreview仅将图片metadata.content当URL，视频/主体没有正式解析。先新增授权poster和错误回退用例，旧实现真实2失败/5通过，日志`/tmp/canvas-search-preview-red.log`；接入后最终14测试通过。

一手接口核对：runtime FixedThumbnail使用`/files/previews`，当前正式bridge不支持该导入端点，不能当可用实现。`getMediaPreview`仅提供原件；正式`previewLibraryMediaAsset({scope:"project",project_id,item_id:assetID})`经LibraryStore项目授权与ready/passed媒体校验，返回固定revision和独立授权renditions。搜索核对asset ID/revision/kind，仅选择图片thumb_256/thumb_640或视频poster；视频缺poster时不请求原片，不猜路径。图片缺rendition时可使用同一正式授权响应的原图。外部metadata URL不再绕过授权作为搜索预览。

主体优先使用局部primary；局部为空不偷偷恢复库主图。项目局部媒体沿项目库正式授权；库绑定主图先getSubjectVersion并用requireCanvasSubjectVersion核对scope及固定subject/version，再getSubjectMedia读取确切binding的image blob。当前主体绑定视频没有封面专用授权接口，保留主体类型图标，不改用无主体授权证明的项目原件。

只在搜索Popover打开且结果行IntersectionObserver可见时请求，最多3项并发；相同来源描述在当前popover内去重。私有请求作用域含session/actor/scope/project及身份代次，关闭时abort、清理请求集合并revokeObjectURL；旧会话迟到响应不生成blob URL。图片解码期间显示类型图标，加载失败、权限错误、版本变化、签名过期或缺封面均回退图标，定位按钮保持可用。签名到期自动移除展示；主体原件blob限定当前popover短时展示，60秒后回退，重新打开会重新授权。

验证：

- `pnpm exec vitest run tests/unit/components/canvas/canvas-node-search.test.tsx`：14通过，0失败，见`/tmp/canvas-search-preview-green.log`。覆盖原搜索合同、同素材poster去重、授权项目参数、失权/过期/版本变化/无poster、可见行限制、3并发、取消、主体pin与binding去重、blob释放、局部primary优先、清空不回退旧pin、迟到响应丢弃、会话切换隐藏旧图、React Activity hide/reveal重新授权。
- `pnpm exec tsc --noEmit`：exit0，`/tmp/canvas-search-preview-tsc.log`。
- `pnpm exec eslint src/components/canvas/search/canvas-node-search.tsx tests/unit/components/canvas/canvas-node-search.test.tsx`：exit0，`/tmp/canvas-search-preview-eslint.log`。
- `git diff --check`通过，无提交。

中间检查如实记录：最初误将project Preview视为带renditions，tsc指出类型不支持后核查正式LibraryMediaQuery并改用真实接口；retain生命周期重构一度遗留旧cleanup导致JSX语法错误，已修复并重新通过全部上述检查。未放宽类型或删除失败断言。

此专项使用受控API响应和真实React/Popover/Activity生命周期测试，没有取得新增缩略图的真实非空浏览器网络验收；主任务浏览器控制发生timeout，本轮不接管根标签页。视频主体poster缺专门授权接口仍是明确差异，不把图标回退称为视频缩略图完成。主体原件当前没有低分辨率绑定接口，虽然仅可见行且3并发，单张图片仍可能较大。
