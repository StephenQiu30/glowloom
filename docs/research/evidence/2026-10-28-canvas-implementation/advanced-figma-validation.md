# 高级画布 Figma 补稿与验证

日期：2026-10-28。范围：主体导入、时间线全屏、导演台内部、AI 侧栏。目标保持用户要求的全量一致，本记录不将六个静态状态等同全量交付。

## 1. 修改对象与证据

使用官方 Figma 插件，复用当前文件 `uLqmeRWuQuzfrp9xOYg6FT`、当前页面 `1361:10`。已读取 figma-use、figma-generate-design 与组件库指导，先检索现有帧、组件、图标、字体和变量。

本页存在旧智能助手帧 `1510:105485`，本次原位保留其 ID、隐藏旧子层并重建当前结构。递归检索未找到时间线全屏、导演台内部、主体导入的完整帧，只有工具栏与新建菜单同名小节点，因此在本页右侧新增对应原生稿；没有另建 Figma 文档。未修改文本主稿、文本全屏、对齐、颜色帧或公共主组件。

| 状态 | 正式 Figma 节点 | 依据 | 本地截图 |
| --- | --- | --- | --- |
| AI 侧栏，已选导演台自动引用 | [1510:105485](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=1510-105485) | `advanced-source/assistant-panel.txt/png` | [assistant.png](advanced-figma/assistant.png) |
| 时间线全屏，空主轨 | [3091:47697](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3091-47697) | `advanced-source/timeline-fullscreen.txt/png` | [timeline.png](advanced-figma/timeline.png) |
| 主体导入，已有主体未选中 | [3092:47360](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3092-47360) | `advanced-source/subject-import.txt/png` | [subject.png](advanced-figma/subject.png) |
| 主体导入完成，回填原节点 | [3095:47609](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3095-47609) | `advanced-source/subject-import-complete.txt/png` | [subject-result.png](advanced-figma/subject-result.png) |
| 导演台动态预演，空场景 | [3093:47499](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3093-47499) | `advanced-source/director-motion-workspace.txt/png` | [director-dynamic.png](advanced-figma/director-dynamic.png) |
| 导演台静态预演，空场景 | [3093:47908](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3093-47908) | `advanced-source/director-stage.txt/png` | [director-static.png](advanced-figma/director-static.png) |

六帧均为 1512×772，与本轮实页截图 CSS 视口一致。新增帧位于 x=202500/204172/205844，和原有页面内容分开；AI 保持原有位置。

## 2. 结构与视觉校验

复用本地 Canvas header `3061:46512`、toolbar `3061:46607`、viewport `3061:46719`、背景 `2236:31327`，以及既有 icon 主组件。复用 `canvas-reference/background`、`canvas-reference/foreground` 变量。公共组件均通过实例消费，未改主组件。库服务 `get_libraries` 本次出现传输错误，随后库搜索无命中；本地 API 组件清单可读，实际复用依据为本地节点读取，不宣称搜索失败代表无组件。

| 帧 | 可见原生文本 | 组件实例 | 矢量 | 自动布局容器 | 图片填充 |
| --- | ---: | ---: | ---: | ---: | ---: |
| AI 侧栏 | 25 | 29 | 82 | 32 | 0 |
| 时间线全屏 | 31 | 6 | 97 | 8 | 0 |
| 主体导入 | 20 | 23 | 60 | 25 | 0 |
| 主体导入完成 | 21 | 27 | 73 | 25 | 0 |
| 导演台动态 | 73 | 16 | 233 | 16 | 2 |
| 导演台静态 | 56 | 15 | 159 | 15 | 6 |

导演台图片仅为 89×68 的角色预设素材缩略图，来源为本轮源站资产面板截图中的素材内容；没有整页截图或整块 UI 位图。三维网格、坐标、时间轴、属性、工具、按钮均为原生节点。主体测试素材为原生几何形状重建。素材缩略图只用于设计参考，不代表已获得项目模型或运行时资产授权。

已逐帧下载 Figma 截图并视觉检查。修正了 AI 模式按钮圆角、时间线媒体过滤间距、扬声器和剪辑工具矢量、导演属性左对齐、保存/平移/贴地/视图工具，修正时间刻度 `00:60/00:65` 为 `01:00/01:05`，主体导入顶栏节点数修正为 6。最新截图为修正后结果。

完整写操作回执在 `advanced-figma/phase1-ledger.json` 至 `phase6-ledger.json`，包含返回的创建/修改 ID；结构读回见 [readback-verified.json](advanced-figma/readback-verified.json)，最后三项文本修正与五个外部说明节点读回见 [final-readback.json](advanced-figma/final-readback.json)。首次结构输出超过 48KB 限额，仅为只读调用，已精简字段重试成功，未触发重复设计修改。

## 3. 当前设计必须落实的交互合同

详细源行为与可复用服务见 [advanced-parity-contracts.md](advanced-parity-contracts.md)。下面的范围均保留，不因当前六帧为静态设计而删除。

| 能力 | 已落布局 / 状态 | 后续完整交互设计与真实验收条件 |
| --- | --- | --- |
| 主体 | 804×622 导入弹窗、搜索、收藏筛选入口、复选、未选确认禁用；原节点回填主媒体和音色 | 已选择数量联动；导入中 0/1 与取消；不额外新增第二个节点；主媒体移除明确清除主体；辅助媒体来源本地/资产库/画布；音色播放/替换/删除；固定版本与授权，保存读回/刷新/重开 |
| 时间线 | 360px 可调资产栏、1120×474 预览、218px 可调轨道区；无媒体与禁用剪辑；原声/音量/比例/吸附/缩放 | 已导入与画布资产；本地/资产库导入；auto+10 比例；非空画面/音轨、分割、裁剪、排序、撤销；每个 MP4/批量/剪映/XML/DaVinci/Premiere/Final Cut 导出有效文件单独验收 |
| 导演静态 | 247px 资产、965px 视口、300px 对象属性；角色预设、搜索、相机预览、环境/颜色/地面/曝光/网格 | 对象选择/锁定/聚焦、移动/旋转/缩放/贴地；位置/目标/FOV/roll/比例；真实角色与物体；导入权限/格式/错误；拍摄与素材回写；本地草稿和云保存边界 |
| 导演动态 | 顶部模式、441px 高视口、279px 动态时间轴；空机位轨、分镜编排、30fps/20s | 帧游标、循环、隐藏未动画对象、对象动画/骨骼/关键帧；14 运镜预设与自定义/绘制路径；切换模式覆盖确认；24/25/30/50/60fps 和 1280/1920 长边真实导出；离开时保存/不保存/继续编辑 |
| AI | 400px 可调侧栏、空态五专业模式、选中导演台引用、仅引用发送禁用 | 技能搜索/七项目录/新建管理；本地/资产库/画布参考；正式项目消息/流式执行/取消重试；工具授权、结果回写和刷新恢复；不能把管理员 trial 接到用户助手 |

源站只观察到菜单语义而无菜单截图或完整流程的项目，本轮没有凭空补像素稿。需要实页补观后再进入状态帧：时间线非空编辑；各工程格式结果；导演非空姿态/关键帧与拍摄回写；AI 真实消息与工具执行。源已观察的菜单（例如导出、运镜、技能）尚需从 AX 合同进一步补齐对应展开态原生帧。

## 4. 明确的未完成与差异

1. 当前六帧完成的是主要已观察布局及主体非空回填态；没有签署全量一致性。各菜单展开态、异常/权限/加载/非空链路尚不完整，不能当作完整设计闭环。
2. Figma 可用字体没有 PingFang SC；当前为 Noto Sans SC。字号、字宽、基线仍需同字体逐项对照，不能称像素一致。
3. 导演视口网格是可编辑原生示意，不是实际渲染器输出，透视、网格密度、渐变与源画面仍有明显差异；需要后续实际渲染器相机/地面参数还原与截图比较。
4. 图标多数复用项目现有库，部分为按可见形状重建的矢量，仍有少数符号式控件；品牌、账号头像、背景点阵密度与源站不同。没有修改公共组件来掩盖这些差异。
5. 当前 Figma 帧不包含完整可点击原型连接，也不证明前端、正式 AI 服务、存储、协作或媒体导出已经工作。未执行代码测试、生成或导出。

在 Figma 帧外增加五个“验收边界 / 高级能力待观察”说明，ID 为 `3096:47765`～`3096:47769`。说明不进入产品界面，不用虚构非空内容填满源证据空缺。项目需在继续补证和实现中关闭这些差异，不能以本记录降低全量一致目标。

本任务仅新增独立证据和截图，未修改业务代码、BACKLOG 或正式文档主链，未提交或推送 Git。
