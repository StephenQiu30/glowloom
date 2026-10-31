# JC-042～045 导演工具箱实现回执

日期：2026-10-29；对应设计27 §7.4.11、PRD14 AC-CJC-15、Plan23 T-CJC-15。本回执记录实现与证据，不替代全量产品验收。

## 来源与实现边界

来源为同目录 advanced-source/toolbox-camera-{node,connection,connected-image}、toolbox-camera-carousel.json、toolbox-palette-colors.json、toolbox-{palettes,styles,motion,motion-single-select,custom-palette,palette-color-editor}。实际源操作由主任务执行，恢复5节点0边，未调用生成。

目录闭合为7种相机、7种镜头、5种焦距、4种光圈、27个各8色配色、10风格、20运镜。自定义初始5色采用来源实测；当前实现最大16色和20MB图片取色上限为项目能力决策，不是来源上限实测。提取算法使用图像真实像素量化直方图；不声称源内部算法一致。

CinemaKit为正式版本化config与生成意图：独立cinema_studio节点恰含一类设置；image/video上下文各三类。图来源与目标本地同类配置不能重复，显式替换同时移除对应入边。七项后接菜单固定展示并解释不支持项；camera→video禁止。图类型约束与正式生成模型预检分开，模型未选定或能力不适用不会作为可执行生成提交。

Canvas authority在持锁图中合成typed input，冻结源node UUID/设置hash、下游input_revision和快照；修改、拆边触发输入版本变化，移动不触发。运行时编译器消费同一输入，在provider prompt加入确定视觉约束，同时保留用户正文和typed editor_draft。该编译策略是本项目实现决策，不是推断即梦采用相同内部实现。

## 已执行验证

- 前端CinemaKit合同/目录一致性/图替换、四类picker与失败重试、七项菜单、主体连线、真实Yjs session及主题策略，5文件69项通过。
- `pnpm exec tsc --noEmit`通过；新增模块与接线ESLint通过。API由主任务统一再次生成，未手改生成产物。
- 实际隔离PG18、运行角色canvas_test_runtime、真实Yjs二进制接受路径的 `TestCinemaKitPostgresYjsSaveEditReadAndDetach` 通过：相机35mm创建→连图片→修改85mm→新service重开→正式ReadInput→Compile；目标input_revision递增，旧input被拒，原不可变node_input仍35mm；移动不增输入版本；拆边清除设置并递增版本。
- 发现并修复两个数据问题：Yjs whole-config投影遗漏cinema_kit导致编辑丢失；有效输入返回值别名引用上游camera/palette，补复制后冻结值不会随原图对象突变。
- Go race专属及主体组合8项通过（3.137s），覆盖typed冻结、错误类型/伪造目录/重复槽位、作者正文不被改写、能力校验及PG往返。
- `govulncheck ./internal/canvas/... ./internal/prompt/... ./internal/operation/...`：可达代码0漏洞；工具另提示所需模块1条未调用漏洞，不把这写成全部依赖无漏洞。
- 首次未提供必需DB环境变量的整包canvas回归失败；prompt/operation整包通过但其中环境依赖用例自带skip，不据此宣称其集成验收。专属PG测试另以隔离owner/runtime DSN实际执行。
- 根目录范围golangci误运行一次无module；backend全目录lint检出既有artifacts/admin*临时程序9项，不修改无关文件。正式internal及相关tests单独检查0 issues；对应go vet通过。

## 尚未证明全量一致的部分

1. 原站摄影、风格与运镜样片未复制，当前使用独立SVG场景示意；照片视觉效果、动态运镜预览与源存在明显差异，不能称像素或效果全量一致。
2. 当前项目无可用模型目录，未完成provider真实生成、费用确认、生成结果/镜头配色遵循性验收。typed输入、哈希和编译器通过不代表供应商遵循效果。
3. 源“暂不支持这些设置的模型”的实时模型能力发现策略未有完整源码证据；当前按已观察类型矩阵拒绝错误连线，正式模型校验在服务端执行。
4. 实际浏览器新建→后接→修改→刷新→清除→撤销由主任务独立验证，不能以单元测试或PG证明代替浏览器视觉与手势验收。

未提交Git；与其他画布功能共享的改动由主任务按本轮范围审核统一提交。

## 原生 Figma 设计回执（2026-10-29）

已在原文件和原页面内建立 [CinemaKit 九态总览](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3143-56735)，未以截图贴图替代界面。总览尺寸5743×3464；九个状态画布均1869×873，选择器按源约800px宽重建。

| 状态 | 原生节点 |
| --- | --- |
| CK-F01 相机全部自动 | 3143:56765 |
| CK-F02 现代数字与35mm | 3143:56978 |
| CK-F03 配色27预设完整可滚动目录 | 3143:57191 |
| CK-F04 自定义5色、色轮与Hex浮层 | 3143:57848 |
| CK-F05 风格默认复古对称美学 | 3143:58041 |
| CK-F06 运镜20项未选择 | 3143:58233 |
| CK-F07 运镜侧面跟拍单选 | 3143:58572 |
| CK-F08 四类独立节点及图片/视频三项chip | 3143:58912 |
| CK-F09 七项后接菜单及清除后的状态 | 3143:59152 |

读回结果：2676个后代，包含277文本、764 Frame、257实例、1131矢量、217椭圆、18矩形、12主组件；图片填充节点0。原生四类节点主组件为3146:53326、3146:53337、3146:53377、3146:53397，画布示例按72%缩放展示。复用既有Canvas background/header/toolbar/viewport/AI主组件及canvas-reference颜色变量，缺少对应library资源的按钮/工具chip建立本地复用组件；所有文本字体读回均为当前项目既有Noto Sans SC Regular。

完整合成截图已实际导出并目视检查；发现运镜方形样片留边后做定点修复，再次导出4096×2471截图通过布局检查（[稳定截图](figma-cinema-kit.png)，本机临时回执 `/tmp/cinema-kit-figma-overview-final.png`）。一次工具transport中断后先读回，确认四类节点与Hex浮层已成功写入，未重复创建。摄影、设备缩略图、风格样片及动态运镜仍为独立静态矢量替代，设计说明已明确标注；此稿不等于视觉全量验收，也未宣称原型连线等于产品功能。

主任务补充真实浏览器证据：独立相机24mm保存刷新、输出端Enter打开后接菜单、创建图片3并显示24mm、编辑源为35mm保存再刷新、图片3正式输入工具栏读回35mm，均通过。对应同目录 `browser-cinema-linked-reopened.png` 与 `browser-integration-20261029.md`；仍未提交生成。
