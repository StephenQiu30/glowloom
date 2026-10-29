# 导演台设计对齐第二轮与回读证据

日期：2026-10-29 12:34（Asia/Shanghai）。Figma 文件：`uLqmeRWuQuzfrp9xOYg6FT`，页面：`1361:10`。

本轮在既有 36 个导演台状态中原位更新资产库、动作库、恢复确认，并新增 6 个状态，回读共 42 个状态。范围为设计修改与静态结构/截图核验；**不代表浏览器、三维渲染、上传持久化、协作恢复或视频导出产品验收通过**。本轮没有操作正式浏览器，没有修改产品源码，没有提交代码。

## 对齐结果

| 范围 | 已完成的设计变更 | 实现依据 |
| --- | --- | --- |
| 角色资产 | 用项目实际渲染的 12 张角色 PNG 替换旧示意缩略图；米制高度；全部/我的、搜索、本地导入、项目选择；“我的”明确当前项目；同步 30 个既有角色侧栏 | director-assets.ts、director-asset-library.tsx、director-project-assets.tsx |
| 简模资产 | 32 个实际 Kenney CC0 简模、真实缩略图和米制高度；家具/建筑/自然/街道/厨房/卫浴/办公/日用八类；不展示虚构目录或即梦私有资源 | director-model-catalog.ts、public/director/catalog/manifest.json |
| 几何体 | 13 个当前实际菜单项，真实渲染缩略图：简化人体、立方体、球体、长方体、圆柱体、圆锥体、四棱锥、圆环体、基础平面、胶囊体、圆盘、半圆弧、半圆柱 | director-assets.ts、public/director/geometries |
| 搜索/空态 | “椅”筛选实际椅子、长椅、办公椅；我的角色当前项目空态；选中标签可辨认 | director-asset-library.tsx、director-project-assets.tsx |
| 动作库 | 旧弹窗替换为右侧属性区内联动作库；实际 43 个预设、预设/我的、搜索、逐行动作预览/应用、独立动作导入 | director-motion-catalog.ts、director-motion-library.tsx |
| 预览与导入 | 静态模式提供“使用当前帧为姿势”、姿势命名/保存片段；导入支持 GLB/glTF/FBX/BVH/动作 JSON；明确骨架兼容与厘米单位；取消/保存动作 | director-motion-library.tsx |
| 不可用能力 | 删除原“描述后生成”“生成保存在我的”等已实现假象；保留准确提示“文字生成动作尚未接入可输出骨骼动画的服务。”；没有 742 数量声明，也没有虚构挥手动作 | director-motion-library.tsx、43 项实际目录 |
| 分屏/目标 | 导演视角与机位实时预览两区；退出分屏；注视方式坐标/旋转/对象；注视对象选择、可选注视点示意与机位属性 | canvas-director-workspace.tsx |
| 私有恢复稿 | 从“本地草稿”命名更新为私有恢复稿；30 天保留、恢复前确认、替换静态/动态工作区的影响说明；备份失败提供下载恢复文件/放弃；未提交编辑与已保存状态区分 | director-recovery.ts、canvas-director-workspace.tsx |
| 模型导入说明 | 改正旧“补齐外部资源”指引为自包含文件要求；保留 GLB/glTF/FBX/OBJ 格式，不承诺未知外部引用可读取 | director-model-import.ts、director-import-dialog.tsx |

动作库取的是核验时的当前 43 项目录，包括“蹲姿前进”；未沿用较早候选目录中的“鼓掌”。源代码仍处于并行实现阶段，核验时 SHA-256 已记录于 JSON，不能把本次静态读取称为最终代码。

## 主要节点与截图

| 状态 | Figma 节点 | 本地截图 |
| --- | --- | --- |
| F01 角色资产 / 全部 | [3093:47908](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3093-47908) | [角色](figma-round2-actors.png) |
| F05 简模物体资产 | [3116:48799](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3116-48799) | [物体](figma-round2-objects.png) |
| F09 43 个动作预设 | [3117:55800](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-55800) | [动作库](figma-round2-motions.png) |
| F12 私有恢复稿 / 恢复确认 | [3117:57684](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3117-57684) | [恢复确认](figma-round2-recovery.png) |
| F30 静态动作预览 / 保存姿势 | [3130:50209](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-50209) | [动作预览](figma-round2-motion-preview.png) |
| F31 导入独立动作 | [3130:50698](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3130-50698) | [动作导入](figma-round2-motion-import.png) |
| 新增 F37 八类简模筛选 | [3221:60659](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3221-60659) | [分类](figma-round2-categories.png) |
| 新增 F38 我的角色 / 当前项目空态 | [3221:61159](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3221-61159) | [我的](figma-round2-mine.png) |
| 新增 F39 分屏与机位注视对象 | [3221:61893](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3221-61893) | [分屏](figma-round2-split.png) |
| 新增 F40 私有恢复稿 / 备份失败与下载 | [3221:61552](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3221-61552) | [失败与下载](figma-round2-recovery-failure.png) |
| 新增 F41 几何体资产 / 13 个预设 | [3222:61284](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3222-61284) | [几何体](figma-round2-geometries.png) |
| 新增 F42 物体资产 / 搜索结果 | [3223:61138](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3223-61138) | [搜索](figma-round2-search.png) |

既有静态 `3093:47908`、动态 `3093:47499`、时间轴 `3116:49794` 的根节点保持不变。F19 导入说明、F20 离开提示、F23 冲突提示、F33 格式失败说明同步修正文案。完整 42 个根节点及尺寸、原生组件计数、字体、图片填充核验见 [结构回读 JSON](figma-round2-readback.json)。

## 组件与资源证据

- 先读取 figma-use / figma-generate-design 技能与既有页面，搜索设计系统后复用本文件组件。未启用外部社区设计系统。
- 复用本地 Button 组件 `268:2493` / `268:2501` / `268:2509` 和 Input `286:7026`；保持原生实例、可编辑文本、Auto Layout。
- 颜色绑定已有 background/card/muted/border/foreground/muted-foreground 变量（`267:2266`、`267:2272`、`267:2276`、`267:2282`、`267:2284`、`267:2288`）。字体回读仅 Geist Regular/Medium。
- 所有 42 状态合计回读 1,463 个组件实例、2,970 个 Auto Layout 节点、3,908 个文本节点、5,330 个绑定颜色变量节点。这是包含既有设计的总量，不是本轮新建量。
- 57 个唯一图像哈希：12 角色 + 32 简模 + 13 几何体。图片仅用于资产卡内 107×84 缩略图，不把整张界面截图用作设计稿背景。
- 44 次角色/简模上传成功回执见 [上传回执](figma-round2-upload-receipts.json)；13 次几何体上传结果、全部图片非空核验记录在结构回读 JSON。回读未发现未填充的资产缩略图。
- 资源来源与许可沿用 [资产库实现与许可证据](../2026-10-29-director/asset-library-implementation.md)、项目 `public/director/catalog/README.md`、`public/director/motions/README.md`。设计中使用项目真实资源，没有复制即梦私有资产。
- 三维网格、角色摆位和注视点仍是原生可编辑设计示意，不作为真实 WebGL 渲染结果或运动求解证据。

## 回读与边界

已导出并目视核验 12 张完整 1512×772 状态截图；修正了资产两列卡片高度、静态姿势按钮误置动态模式、注视点标注遮挡、恢复状态显示“已保存”的矛盾，以及搜索三项结果的两列排版。

一次 Figma 写入返回连接中断；后续通过根节点、子节点和文案回读确认全部修正已生效，没有盲目重复写入。一次全量回读超出工具 48 KB 返回上限；缩减为状态摘要后成功回读，未改动设计。

本轮的“完成”仅指上述设计修改与回读。Figma 的可编辑结构和截图不能证明权限隔离、30 天清理、恢复下载、动作骨骼适配、地面放置、撤销重做、真实视频输出或端到端稳定性；这些应由主任务独立的真实运行验收证据判断。未建立完整跨状态交互原型。
