# 导演台 Figma 设计重构回执

日期：2026-10-29。范围为原生 Figma 设计修订与主要流程状态，不包含产品代码修改、真实 3D 渲染或持久化验收。当前共 36 个状态，覆盖主要工作区与补充流程；数量不等于运行验收。

## 文件与保留范围

- 文件：[Glowloom](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT)，设计页 `1361:10`，本地组件库页 `736:26385`。
- 原静态空场景 `3093:47908`、动态空场景 `3093:47499` 原位更新，ID 保留；另补 34 个关键状态。未删除其他页面、非导演台稿或并行任务产物。
- 术语采用「3D导演台」「静态预演」「动态预演」「角色 / 物体 / 背景 / 机位」「镜头素材」。
- 沿用本地 Geist、暗色变量和组件，不引入新的独立设计系统。

## 状态清单

34 个主画板为 1512 × 772；DIR-F26 / F27 为实际 1024 × 768 窄屏原生 Frame。

| 状态                                    | 节点链接                                                                             | TEXT 数 | INSTANCE 数 |
| --------------------------------------- | ------------------------------------------------------------------------------------ | ------: | ----------: |
| DIR-F01 · 静态预演 / 空场景             | [3093:47908](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3093-47908) |      76 |          25 |
| DIR-F02 · 动态预演 / 空场景             | [3093:47499](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3093-47499) |      99 |          26 |
| DIR-F03 · 静态预演 / 角色姿势           | [3116:47733](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3116-47733) |      82 |          32 |
| DIR-F04 · 静态预演 / 机位属性           | [3116:48241](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3116-48241) |      67 |          40 |
| DIR-F05 · 静态预演 / 物体库与放置       | [3116:48799](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3116-48799) |      65 |          37 |
| DIR-F06 · 静态预演 / 背景资产           | [3116:49335](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3116-49335) |      56 |          30 |
| DIR-F07 · 动态预演 / 动作、关键帧与分镜 | [3116:49794](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3116-49794) |     111 |          41 |
| DIR-F08 · 动态预演 / 角色路径           | [3116:50540](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3116-50540) |     114 |          45 |
| DIR-F09 · 动态预演 / 动作库             | [3117:55800](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-55800) |     130 |          53 |
| DIR-F10 · 动态预演 / 运镜预览确认       | [3117:56529](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-56529) |     133 |          51 |
| DIR-F11 · 模式切换 / 静态覆盖动态       | [3117:57233](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-57233) |     106 |          29 |
| DIR-F12 · 恢复本地草稿                  | [3117:57684](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-57684) |      87 |          29 |
| DIR-F13 · 镜头素材 / 多选回写           | [3117:58052](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-58052) |      89 |          35 |
| DIR-F14 · 导出视频 / 配置               | [3117:58613](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-58613) |     134 |          53 |
| DIR-F15 · 导出视频 / 进度与取消         | [3117:59278](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-59278) |     104 |          28 |
| DIR-F16 · 导出视频 / 失败重试           | [3117:59715](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-59715) |     105 |          29 |
| DIR-F17 · 导演视角 / 操作说明           | [3117:60165](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-60165) |      84 |          26 |
| DIR-F18 · 关键帧 / 曲线调整             | [3117:60519](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-60519) |     125 |          48 |
| DIR-F19 · 添加资产 / 选择文件与格式     | [3117:61201](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-61201) |      86 |          29 |
| DIR-F20 · 退出 / 保存未确认             | [3117:61574](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-61574) |      82 |          28 |
| DIR-F21 · 模式切换 / 动态当前帧覆盖静态 | [3117:61939](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-61939) |     121 |          44 |
| DIR-F22 · 资源加载失败 / 重试           | [3117:62584](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-62584) |      81 |          27 |
| DIR-F23 · 保存冲突 / 版本选择           | [3117:62940](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-62940) |      88 |          30 |
| DIR-F24 · 画布 / 导演台节点与打开入口   | [3129:49790](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3129-49790) |      12 |           9 |
| DIR-F25 · 只读工作区                    | [3129:50404](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3129-50404) |      80 |          29 |
| DIR-F26 · 窄屏 1024 / 面板收起          | [3129:50791](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3129-50791) |      22 |          13 |
| DIR-F27 · 窄屏 1024 / 资产抽屉打开      | [3129:51196](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3129-51196) |      57 |          22 |
| DIR-F28 · 对象树 / 多选、显隐与锁定菜单 | [3129:51484](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3129-51484) |      75 |          29 |
| DIR-F29 · 角色路径 / 控制点与时间冲突   | [3129:51898](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3129-51898) |     115 |          42 |
| DIR-F30 · 新建动作 / 文字生成           | [3130:50209](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-50209) |     115 |          33 |
| DIR-F31 · 新建动作 / 导入与服务不可用   | [3130:50698](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-50698) |     113 |          33 |
| DIR-F32 · 资产导入 / 解析进度           | [3130:51187](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-51187) |      82 |          26 |
| DIR-F33 · 资产导入 / 格式校验失败       | [3130:51539](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-51539) |      84 |          28 |
| DIR-F34 · 镜头素材 / 空态               | [3130:51902](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-51902) |      82 |          27 |
| DIR-F35 · 画布 / 镜头素材添加成功       | [3130:52260](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-52260) |      14 |           6 |
| DIR-F36 · 导出 / 帧率下拉展开           | [3130:52531](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-52531) |     139 |          58 |

## 原生结构与组件复用检查

最终结构回读见 `figma-extension-readback.json`（36 稿），末轮只调整实例填充，结构数量未变化；对应填充回执为 `figma-extension-token-fix.json`。共 3,215 个 TEXT、1,170 个 INSTANCE、2,414 个 FRAME、5,013 个 VECTOR；6,759 个节点带变量绑定。全部文本字体为 Geist。30 个角色面板均读出 12 个角色名称，均为纵向滚动；全部弹层边界位于所属画板内。

首轮有 172 个代表性原型导航源。本轮未以新增链接数量作为交付目标，新增状态主要验证静态结构与视觉；复制稿可能继承已有导航，不能据此声称全流程原型可运行。

组件保留本地实例依赖：Button 变体 `268:2509` / `268:2501` / `268:2493`，Input `286:7026`，Switch `737:26410`，Slider `737:26417`，Checkbox `737:26388` / `737:26385`，及原稿图标实例。Button 标签属性和 Input 文本属性通过实例覆盖使用；选中态采用原有 muted/border 变量。沿用背景、卡片、弱化色、边框、正文、次要文本变量 `267:2266` / `267:2272` / `267:2276` / `267:2282` / `267:2284` / `267:2288`，以及 spacing/radius 变量。

没有把整页截图铺入画板。结构中 IMAGE 填充仅为原稿已有素材缩略图（主要 89 × 68）；场景网格保留可编辑向量；新增人物、曲线、时间轴和图库构图均为原生形状或文本。原稿素材缩略图是设计参考，不代表其生产资产许可已获确认。

人物是几何摆位与构图示意，不能代表最终人物模型、姿势质量或真实 3D 渲染。镜头素材中的全景、近景、侧面也是构图示意，未运行拍摄或导出。

## 视觉检查与修正

- 静态角色和动态角色场景的机位小窗包含人物，避免主场景有人而预览空置。
- 动态时间轴采用独立角色动作、角色位置、机位、注视点位置和分镜编排轨道；提供比例时间标尺、片段、关键帧和播放头。对象区包含两台机位，与分镜条一致。
- 产品界面中的设计说明移到画板外；资产面板设置纵向滚动及裁切。导出控件显示当前值，其他候选值置于画板外说明。
- 弹层有整屏遮罩且内容位于画板内。曲线状态有可编辑曲线图；运镜预览有场景构图示意；图库有差异构图、明确选中边框和复选控件。
- 主代理复核了末轮动态时间轴、镜头素材与运镜预览截图，前述可见不一致已修正。

## 代表性截图

以下为最终代表性视觉证据，均以 `figma-` 区分即梦采集证据：

- [静态角色姿势](figma-static-pose.png)
- [动态时间轴](figma-dynamic-timeline.png)
- [运镜预览](figma-motion-preview.png)
- [导出配置](figma-export-config.png)
- [镜头素材多选](figma-shot-gallery.png)
- [恢复草稿](figma-draft-recovery.png)

`figma-action-library.png` 是较早中间态截图，不作为最终验收证据。运镜预览与恢复草稿截图对应已复核弹层；其他未导出状态以最终结构回读为证，不声称每一状态都做了独立视觉验收。

## 补充主要状态的静态设计覆盖

- DIR-F24 / F35：画布添加入口、导演台节点、打开按钮、回写成功的两个图片节点与定位反馈。节点预览去掉编辑工具、机位小窗与对象标签。
- DIR-F25：只读标识、权限说明、申请权限入口、禁用添加与编辑字段/变换工具，画板外定义服务端权限与快捷键约束。
- DIR-F26 / F27：实际 1024 × 768 布局，面板收起与资产抽屉打开。更多菜单收纳合同、Esc/外侧关闭、互斥抽屉与滚动恢复写在画板外。
- DIR-F28：对象多选、显隐、锁定状态及上下文菜单；引用删除和撤销规则写在画板外。
- DIR-F29：原生路径曲线、三个控制点、点位/到达时间字段、时间顺序冲突与禁用应用。底部动作区已修正至面板内。
- DIR-F30 / F31：文字生成动作、导入动作、生成服务不可用时可导入与重试连接。**服务不可用是条件状态设计，本轮没有真实服务故障验证。**
- DIR-F32 / F33：资产导入解析进度、格式失败及重选。62%是示例状态，不代表实际任务进度；其余校验错误以状态合同定义原因和恢复操作。
- DIR-F34：镜头素材空态与返回拍摄入口。
- DIR-F36：导出帧率下拉上展及当前 30 fps 选中态，其余选项行为有画板外合同。
- 男女各含标准、少年、儿童、纤瘦、健壮、丰腴，共12项目录已同步到30个角色面板；后三类使用原生比例示意，真实模型预览仍待接入。

补充关键截图：

- [画布入口与节点](figma-extension-entry.png)
- [1024px 资产抽屉](figma-extension-narrow.png)
- [路径控制点与冲突](figma-extension-path.png)
- [动作导入与服务条件状态](figma-extension-action-import.png)

主代理复核窄屏和导入可读性；路径底部裁切、纯黑实例填充与节点预览控件随后修正并替换截图。纯黑来自补充稿实例赋值的颜色回退值；修复直接沿用 F07 已验收实例的 paint（保留变量绑定），没有修改组件库。

## 真正剩余的精细交互与运行验证

| 项目                    | 已有静态设计                   | 仍需补充或验证                                                             |
| ----------------------- | ------------------------------ | -------------------------------------------------------------------------- |
| 入口/画布回写           | 节点、打开、成功反馈           | 实际 sceneId 绑定、关闭定位、幂等回写、部分失败与权限拒绝                  |
| 只读与窄屏              | 只读稿及1024收起/打开          | 权限实际强制执行；其他宽度、右侧抽屉展开与键盘焦点逐项运行                 |
| 角色目录/姿势           | 12项滚动目录与分组姿势字段     | 真实模型预览、各肢体骨骼控制、体型/姿势渲染质量                            |
| 对象树                  | 多选、隐藏、锁定与菜单         | 组合字段编辑、锁定后所有手势/快捷键约束、引用删除和撤销                    |
| 角色路径                | 路径点、属性、时间冲突         | 空间拖拽、插点/删除、路径采样、时间映射与取消恢复                          |
| 关键帧曲线              | 曲线预设和原生曲线图           | 控制柄拖拽、插值边界、非法输入和逐帧一致性                                 |
| 运镜                    | 预设、确认与预览结构           | 全预设、固定机位/自由路径绘制、注视点与真实相机播放                        |
| 动作                    | 库、文字生成、导入、服务条件态 | 真实生成服务、骨架重定向、裁切/重叠/混合及生成中的细分状态                 |
| 资产导入                | 格式入口、进度、失败           | 真正文件验证、关联资源恢复、尺度确认细节、取消资源释放                     |
| 镜头素材                | 空态、选中与回写成功           | 实际拍摄、素材加载/删除失败、下载与回写失败恢复                            |
| 导出                    | 配置/下拉/进度/失败            | 其他下拉展开细节、编码器兼容、实际视频帧率/画幅/时长/取消验证              |
| 保存/草稿/模式覆盖/冲突 | 主要确认与异常态               | 云端往返、覆盖范围、协作版本处理、恢复正确性                               |
| 原型与3D运行            | 原生可编辑设计和构图示意       | 完整可运行交互与真实模型、光照、骨骼、相机和输出；没有以设计稿代替运行测试 |

当前主要设计状态已补齐本轮清单。上表区分精细交互和真实运行仍待验证，不声明全部功能或全量运行验收完成。

## 工具回执

- 创建与原位修改：`figma-build-workspaces.json`、`figma-flows-1.json`、`figma-flows-2.json`、`figma-flows-3.json`。
- 结构/视觉修正：`figma-layout-fix.json`、`figma-visual-polish.json`、`figma-preview-fix.json`、`figma-review-fixes.json`。
- 原型导航成功回执：`figma-prototype-final.json`。
- 首轮结构回读：`figma-final-readback.json`。
- 补充写入：`figma-extension-workspaces.json`、`figma-extension-flows.json`、`figma-extension-catalog.json`。
- 补充修正：`figma-extension-polish.json`、`figma-extension-token-fix.json`。
- 最新全量结构回读：`figma-extension-readback.json`。
- `figma-build-flows.json` 是 transport-closed 诊断回执；`figma-prototype-links.json` 是自连接校验失败回执，不能作为成功证据。随后调用成功回执如上。

本次没有修改产品源码，没有提交或推送 Git 变更。
