# 高级连接与异常保存 Figma 回执

日期：2026-10-29。先完成设计27 §7.11，再写入原Figma总板3153:57117。未修改产品代码或执行Git提交。

## 范围与可编辑性

原01–07保留，原板高度从2980扩为5940，未新建替代总板。新增08–16九个原生可编辑结构状态，复用现有画布背景、顶栏、工具栏、视口控制、AI入口、菜单与多选工具条。证据标识位于各状态画面外，明确“源页面实测”“源静态事实”“本项目待验”。新增状态并非即梦完整截图的逐像素复刻。

| 状态 | Figma节点 | 证据范围 |
| --- | --- | --- |
| 08 · 空图片后接 / 四项可用 | [3243:62880](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-62880) | 源页面实测｜空图片当前状态；原因来自操作观察，未把内部字段当作界面文案。 |
| 09 · 就绪图片 → 主体 | [3243:63071](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-63071) | 源页面实测｜主来源1、辅助0、音色未配置；图为可编辑结构示意，资源使用示意色。 |
| 10 · 就绪图片 → 时间线 | [3243:63225](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-63225) | 源页面实测｜1视觉、0音频、1剪辑、5.0秒；图为可编辑结构示意。 |
| 11 · 单项非法 / 整批拒绝 | [3243:63382](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-63382) | 源静态事实＋普通重复实测｜高级混合失败图为本项目待验；不能部分接受。 |
| 12 · 本地应用 / 等待 ACK | [3243:63538](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-63538) | 本项目待验状态｜“等待保存”为项目建议文案，不冒称即梦异常 UI。 |
| 13 · 断线 / 保存结果未确认 | [3243:63696](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-63696) | 本项目待验状态｜未知结果按原身份查证；未验证即梦离线恢复 UI。 |
| 14 · 写权限失效 / 仍可读取 | [3243:63856](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-63856) | 本项目待验状态｜停止业务编辑与发送；本页未决内容按既有权限合同处理。 |
| 15 · 读权限失效 / 清理旧内容 | [3243:64013](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-64013) | 本项目待验状态｜停止展示旧图和私有输入，不提供跨账号重放。 |
| 16 · 视频＋音频上传中 | [3243:64163](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT?node-id=3243-64163) | 源页面实测｜双选无聚合出口、下载禁用；真实媒体就绪后另验。 |

09/10使用显式“资源示意”色块，未把颜色块冒充正式媒体资源或生成产物。16仅“上传中无聚合出口、下载禁用”为源观察，保存文案与其他排布是结构示意。13–15属于项目合同建议，不是即梦异常UI实测。

## 已修正既有偏差

- 原多选菜单标题3153:58150、本任务局部菜单主组件标题3197:59509：font12、line20、白35%、节点opacity1，读回一致。
- 原合同文字3153:58493区分标题12/20和选项13/22。
- 新空图片菜单继承200×316、圆角12，音频行恢复可用，时间线/主体/导演台保持禁用；实测原因独立注释。
- 写权限失效示意中业务工具和AI入口弱化；读权限失效示意隐藏工具、视口和AI入口并不展示旧图。其真实运行与权限隔离仍待代码/服务验收。

## 合同覆盖

设计27 §7.11记录：时间线与主体走领域事务、全部来源就绪、完整候选整体admission、一次提交与undo、任一无效整批拒绝。新建authoring拒绝text→audio、video→image、audio→image，旧wire与历史草稿继续可读；按唯一来源计容量text128、image/video999、audio7，明确不是模型限额。

等待ACK不等于saved。未知结果按原提交身份查证，不重新分配身份创建重复目标；重连先核验权限/epoch并追赶基线。业务journal仍为项目明确缺口，不借图稿承诺关闭页面后的未确认业务恢复，不复用导演台恢复稿冒充普通画布journal。

## 核验

- 九态逐项读回，IMAGE fills均为0。
- 各状态文本、保存文案与画面外证据分类已读回；完整新增/修改ID见figma-advanced-readback.json。
- 目视检查原板全图、空图片菜单、断线未知及上传中状态，内容无明显裁切；readback中note附带最后文字修正。
- 第一次精简前读回因超48000字节限制未返回；之后精简读取成功。一次传输错误后先只读确认未写入，再重试成功，未创建重复状态。
- 静态设计不证明领域事务提交、整批失败回滚、unknown reconciliation、真实权限撤销或IDB恢复。源页面具体成功样例与本项目待验场景分别标注。

## 文件

- figma-advanced-readback.json
- figma-board.png（完整原板及新增状态）
- figma-empty-image.png
- figma-unknown-save.png
- figma-uploading.png

源观察输入：父任务提供的 /tmp/advanced-source-empty-image-menu.png、/tmp/advanced-source-subject-redo.png、/tmp/advanced-source-timeline-created.png；真实证据归档由父任务负责。

## 后续验证边界补记

本补记更新验收证据，不改未经证实的Figma交互，也不将原“本项目待验”异常画面冒充即梦已实测界面。最终29文件298项关联测试、全局TypeScript、54路径ESLint、Next生产构建通过，详见[前端门禁](advanced-frontend-gates.md)。独立真实PG主体来源读取和时间线关系/时长校验通过，分别见[主体证据](subject-domain-input-validation.md)及[时间线日志](timeline-relations-postgres.log)。

主任务真实本地浏览器连续操作确认：图片→5秒时间线及图片→主体创建、整体undo/redo和刷新保持3节点2边，主体redo的graphHash恢复一致；视频4秒＋音频7秒→时间线2剪辑、总长7秒，undo5节点、redo6节点，accepted commitSeq10/revision11，刷新保持6节点4边、两剪辑4/7秒与既有主体/图片时间线。redo接受与刷新权威snapshot的graphHash同为`79763d16cf08a6be97ecdeef41211b199d284f09cd044cb74e260fd14c741102`。这些是指定成功样例，未验证所有主辅素材、已有目标追加和异常恢复。

源视频＋音频→时间线创建与undo已观察；redo本地10/10后出现write_scope_denied并停止编辑，刷新回9节点，故重做持久化不能记为通过。另同视频＋音频组合的源菜单Audio禁用、本地启用，属于实际差异，原因未查明，不推断模型限制。现有Figma16是上传中状态，不能据此改成就绪组合的通用禁用规则。

完整成功样例与真实异常见[浏览器验收](runtime-validation.md)。UNKNOWN及15秒等待后inFlight会话回归证明仅一次submit、原身份查证及专用连接准入保护，详见[保存审查](save-confirmation-audit.md)。先前浏览器离线模拟未成立，不计通过；随后移动视频的真实提交`13aa9669-e60e-4943-bacd-b3c43e3216da`后WebSocket关闭，未观察accepted，UI显示UNKNOWN只读、隐藏连接入口并保留页内原图。点击重新连接未恢复，随后跳转login。只读保护已观察到，但原提交查证、恢复及刷新保留尚未完成；用户随后已完成登录，原提交与新快照正在查证；不能把移动异常替代高级连接创建丢ACK全矩阵。

[音频静态规则审计](audio-connected-creation-rule-audit.md)仅确定创建草稿、能力ready和acceptedTypes等门禁，未确定本次源页面实际禁用分支；不将V+A或7秒时长写死为禁用规则。本补记不修改未经证实的Figma交互，异常恢复未完成标记保留。业务journal、权限/epoch完整故障矩阵、全量组合及所有视觉状态未获完整接受，不报告全量一致。

## 同页查证与跨登录范围注释更新

已有图片时间线批量接入V4+A7已取得create seq14/rev15、undo seq15/rev16、redo seq16/rev17及权威snapshot6节点6边；图片0–5、视频5–9、音频0–7，总9秒，create/redo/readback的hash一致为`abf488b3925c53b035ed2e70180ba9a9317fd3d8165a65d5027f3fa1bd666014`。详见[运行记录](runtime-validation.md)。

视频4秒→新主体在真实网络中断后同页查证接受：op `ed17f168-345c-4b50-a261-987bde4da00b`仅submit一次，WS关闭后以同op和payloadHash `86b821c169afbd5b4f90d4586570a02730da39e334842bc19d962b9328e31c75` resolve，accepted seq17/rev18、graphHash `2d3290b82983f9aacab9260ef7f2e2b70d202879a66d322f80a7e5a401d44042`。本次不是注入模拟；同页恢复接受已经证明，刷新读回由主任务另核。早先移动跨登录后未保存且未决意图丢失是已确认缺口，journal修复与跨登录恢复尚未通过；不能用同页成功覆盖该失败。

原位仅修改四处画面外注释：3243:63213（主体）、3243:63367（时间线）、3243:63680（等待ACK）、3243:63838（断线未知）。未改画面布局、控件或未经证实的交互。四处均加载原字体后写入并读回，宽1869、高21，父注释区高24；导出检查文字未裁切。证据为`figma-runtime-note-readback.json`及`figma-runtime-note-{subject,timeline,ack,cross-login}.png`。异常图仍保留明确范围与未完成项，不报告全部异常或全量一致通过。

## 持久层与音频门禁后续范围

重新登录打开原业务画布，可见7节点、主体2、9秒与7秒时间线并显示已保存；原视频→主体UNKNOWN操作accepted seq17/rev18不变。但再次reload采集snapshot时停在opening，未获得该次权威hash读回，不能宣称此刷新闭环通过。

IndexedDB＋Web Locks原身份journal已实现，session/journal86项通过；helper `c456cbcb595d25ebf730bf1c27cc5980ea7f9d8041709344f75fadce34c97d2c` 的真实Chrome独立origin save/reload/read/actor隔离通过，详见[存储验证](idb-browser-validation.md)及[恢复实现](durable-pending-recovery.md)。这只证明存储层，真实业务跨登录恢复仍待验；此前跨登录丢意图失败不被删除。

音频连接新鲜目录、稳定默认模型、原草稿pin和IME保护已接入，47项相关测试通过；固定主体预览缓存仍在补，最终变更门禁另记。源音频单来源可用、视频单来源禁用的实测与静态acceptedTypes门禁共同约束实现，不能硬编码为混选互斥，也不证明与源当前模型配置全量一致。

本次再次原位修订注释3243:63680（accepted与opening快照边界）、3243:63838（journal存储层通过/业务恢复待验）、3243:64305（上传态与就绪音频能力分开）。三处保持原字体及1869×21文本边界，父区1869×24，导出目检无裁切；未改控件、布局或源禁用原因。新读回为`figma-durable-note-readback.json`，截图为`figma-durable-note-{ack,journal,audio}.png`，不覆盖前次历史证据。
