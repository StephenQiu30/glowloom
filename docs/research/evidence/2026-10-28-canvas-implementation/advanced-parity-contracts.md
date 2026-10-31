# 高级创作全量一致性：合同与实页补证

核查日期：2026-10-28；起始源码HEAD `78d018be88ea3ee018eb996809fbc2ae2acdf03d`，main，共享工作区含画布并行修改。本文只修改本证据及 `advanced-source/`，不改业务代码、事实来源、BACKLOG或Git提交。覆盖JC-046～051、058，补充[领域静态核查](domain-audit.md)；不是产品全量接受声明。

## 1. 证据边界

- 已重读根AGENTS、研究10、设计27、当前画布/主体/媒体/operation源码，并以独立Chrome标签页进入用户指定即梦画布。
- 源页观察条件：1512×772 CSS px，devicePixelRatio=2，画布显示66%，初始5节点/0边/0选中、积分66。证据为AX树、实际点击结果与PNG，不是源码推测。
- 自建的时间线及主体节点均通过撤销恢复；导入的是源站已有“主体调研测试-20261028”，未上传外部文件，未修改主体库条目。最终5节点/0边/0选中、已保存、积分66；见[恢复记录](advanced-source/restored-canvas.txt)与[截图](advanced-source/restored-canvas.png)。撤销历史发生变化，未声称恢复为原历史栈。
- 导演台切换模式产生本地未保存草稿；已选择不同步并切回静态，最终“直接关闭”，未点保存/保存并关闭。界面说明本地草稿保留当前浏览器，因此不能声称本地草稿字节完全恢复。首次操作引导已关闭。
- 没有执行AI消息发送、付费生成、导演拍摄/导出、时间线实际导出、公开分享或外部上传。音色预览打开后界面进入播放态，观察到4秒时长；没有新增音色生成任务。
- 用户要求全量一致覆盖以下全部已观察能力。设计27“不扩建独立三维引擎”约束实现方式，不能用于删掉导演内部能力；可在现有画布域/three渲染层内实现，不能以入口或查看器替代。

## 2. 主体：JC-046～047

### 实页事实

1. 工具栏主体新增一个选中空节点；空态有名称、说明、主媒体、主体导入、资产库、本地添加。点击“Edit 主体 1”实际进入名称编辑，不是全屏主体编辑器。
2. “添加主媒体”进入画布选择模式，只允许图片或视频，Escape/取消退出。空主媒体不等于任何普通图片节点。
3. 导入选择器含搜索、收藏筛选、已有主体卡片、选择计数和确认。零选择确认禁用；选择已有测试主体后计数为1主体，确认可用。确认显示“正在导入0/1”与取消入口。
4. 导入完成后仍为6节点，原空主体被已绑定主体填充并更名，不是额外新增第7个节点；Main ready、0辅助、voice present。撤销一次回到原空主体，再撤销一次移除临时主体。
5. 绑定态“Add subject media”有本地添加、从资产库选择、添加选定辅助媒体；主媒体移除按钮注明会清除主体。
6. 音色预览有播放/暂停、时长；更多动作含替换音色、添加选定音色、资产库、本地添加、智能配音、删除。智能配音未触发。

证据：[空态](advanced-source/subject-empty.txt)、[主媒体来源](advanced-source/subject-main-sources.txt)、[选择/筛选](advanced-source/subject-import-selection-filter.txt)、[导入完成](advanced-source/subject-import-complete.txt)、[绑定态截图](advanced-source/subject-import-complete.png)、[辅助媒体菜单](advanced-source/subject-bound-menu.txt)、[音色操作](advanced-source/subject-voice-actions.txt)。

### 可复用与差异

| 当前责任 | 已有合同 | 全量接入必需补充 |
| --- | --- | --- |
| `frontend/src/api/subjects.ts` | list/get/version/media/favorite/source/voice API | 消费现有生成API，不手工重复客户端或越权查询 |
| `subjects/unified-reference.ts:69` | `unifiedSubjectBinding`固定主体ID/version_id，展开主/辅/voice媒体及binding_id、asset_revision | 画布主体节点保存正式主体来源及固定版本；临时空主体与库绑定主体分开；是否修改库须明确，不能编辑节点说明就静默改库 |
| `subjects/unified-library.tsx:73,458` | onSelect当前单个主体回调；收藏、搜索、分页、媒体展示可复用 | 画布导入要有本地零/一/多选、计数、确认、处理中/取消/部分失败、准确替换/新增规则。普通库批量管理不是导入选择器 |
| `canvas/controls/canvas-toolbar.tsx:253`、`canvas/types/canvas.ts` | 主体入口禁用，无正式主体枚举 | 增加主体消费节点呈现、主辅/voice编辑与关系角色；未知类型不能混成普通图片 |
| `canvas/domain/graph.go:31` | 合法节点类型尚无subject | 扩展闭合节点合同、配置及持久化校验/API生成/Yjs投影；不能只放宽任意字符串 |
| `canvas/adapter/postgres/yjs_projection_store.go:66,71` | 保存composer及draft.composer时复核来源授权 | 新主体配置要同等验证来源、版本与媒体绑定；不能只在前端核验 |

退出检查：零/一/多选确认、绑定后名称/主辅/音色回读、刷新与两端固定版本、导入撤销/重做、空主体拓扑与可执行素材分离、主媒体清除语义、跨空间拒绝、删除/撤权后保存/报价/执行重新拒绝。现有主体源和真实媒体可以在本地测试环境执行，无需读取凭据；真实供应商生成另需有效已配置模型。

## 3. 时间线：JC-048～050

### 新增实页事实

- 节点空态有分割/删除禁用、时间、导出、全屏、静音、添加素材、尺寸调整；AX明确一个visual轨、零audio轨；Shift+左右方向键用于排序。
- 全屏是实际剪辑工作区：左侧“已导入资产/画布资产”，再按全部/图片/视频/音频筛选；导入菜单为本地上传/资产库；资产面板宽度可调整或隐藏。
- 中央预览含播放头、时间、画面比例、原声、音量、全屏播放；底部可调整轨道编辑区高度，具备撤销/重做/分割/向左剪裁/向右剪裁/删除、自动吸附、缩放和主视觉轨静音。
- 比例菜单实际为：自动、2.35:1、21:9、1.85:1、16:9、3:2、4:3、1:1、3:4、2:3、9:16。现有Go只3种比例，与目标明确不一致。
- 空时间线的MP4与批量素材导出禁用；可见其他导出入口为剪映、XML、DaVinci Resolve、Premiere、Final Cut Pro。本轮未点击这些导出，不推断文件格式有效性。

证据：[全屏AX](advanced-source/timeline-fullscreen.txt)、[全屏截图](advanced-source/timeline-fullscreen.png)、[比例菜单](advanced-source/timeline-aspect-menu.txt)、[导入菜单](advanced-source/timeline-import-menu.txt)、[导出菜单](advanced-source/timeline-export-menu.txt)。

### 可复用与差异

| 当前责任 | 可复用 | 全量接入必需补充 |
| --- | --- | --- |
| `canvas/domain/timeline.go:14` | 轨道、片段、来源起点、时长、淡入淡出、音量、裁剪、文本、吸附 | 对齐源端自动/10种显式比例及视觉/音频轨道语义；是否允许图/视频同视觉轨必须非空实测，不能凭当前同kind限制推断竞品 |
| 同文件`:81` | 24/25/30/60fps、32轨/1000片段、100ms下限、不重叠 | 属织光现有规则，不自动宣称源端相同；应测最短片段、连续/间隔、碰撞、越界、比例切换 |
| `canvas/application/yjs_projection.go` | 时间线配置通过原有Yjs投影校验 | 前端补timeline/tool创建与完整配置读写、一次编辑事务撤销；当前`service-graph.ts:327`只允许6种基础类型且写死resource |
| `canvas/adapter/postgres/timeline.go:20` | `FreezeTimeline`锁授权、精确revision、同画布来源及正式媒体身份 | 导出前等待匹配接受回执，不能把本地预览状态作为冻结输入 |
| `frontend/src/api/mediaExports.ts` | create/get/cancel/retry/preview/review/download/subtitles | 连接真实任务状态与文件消费；review_required不能直接冒充成功 |
| `mediatool/adapter/ffmpeg`、`domain/export.go:18` | 已有MP4/M4A渲染；SRT由preview生成 | 七种可见导出入口逐个建立合同和产物校验；当前只有video/audio，不提供改扩展名伪工程 |

退出检查必须包含非空视觉/多音轨、三来源素材导入、播放、剪裁/分割后source_start_ms正确、静音/音量、吸附/排序、撤销/重做、刷新/并发保存、逐种导出产物的目标解析或目标软件导入。当前本地有ffmpeg/ffprobe，Compose frontend/backend/canvas-collaboration健康，能够继续执行受控素材的真实本地渲染；服务健康本身不等于导出接受。

未补证：源端非空剪辑/轨道合并规则、工程格式版本与打包结构。可用后续动作：从源站已有可用资产添加短视频/音频，逐一分割剪裁及导出，再撤销；本轮测试画布只有空视频节点，未进行外部上传或借用不明素材。

### 3.1 自动比例补证（同日追加）

独立 Chrome 标签 `1980371324` 重新进入相同源画布，创建临时时间线并打开全屏。比例菜单默认选中“自动”。通过可见 DOM（无应用内部状态读取）测量空预览内层：`757.328125 × 426` CSS px，外层为 `1112 × 426`，计算比例约 1.777766，说明本次空态预览按 16:9 布局。证据：[可见 DOM](advanced-source/timeline-auto-empty-dom.json)、[选中菜单 AX](advanced-source/timeline-auto-empty-menu.txt)。这仅证明本视口、本次空时间线的预览行为，不证明持久化配置或导出默认尺寸。

首媒体验证的实际前提未满足：时间线“导入 → 从资产库添加”的图片页显示“暂无图片素材”，视频页显示“暂无视频素材”；“画布资产”显示“没有可添加的画布资源”。证据：[图片空库](advanced-source/timeline-auto-image-library-empty.txt)、[视频空库](advanced-source/timeline-auto-video-library-empty.txt)、[画布空资源](advanced-source/timeline-auto-canvas-assets-empty.txt)、[截图](advanced-source/timeline-auto-canvas-assets-empty.png)。没有新上传、收费生成或扩大检查其他资产。

因此 auto 遇到首图片、首视频、替换、移除最后素材后的比例仍是待观察项，不能根据行业惯例或项目实现推断。临时时间线已撤销，最终原 5 节点、0 连线、0 选中、已保存、积分 66，检查标签已关闭；见[恢复 AX](advanced-source/timeline-auto-restored.txt)与[恢复截图](advanced-source/timeline-auto-restored.png)。撤销历史发生变化。

## 4. 导演台：JC-051

### 实页内部范围

进入已有导演台后显示3D工作区。初次引导给出Q/W/E/A/S/D移动、I/J/K/L旋转、Shift加速。

| 面板/流程 | 已观察功能 |
| --- | --- |
| 顶栏 | 静态预演/动态预演、保存状态、拍摄、镜头素材、返回画布；动态模式另有导出视频 |
| 视口 | 真正3D网格视口、导演视角选择、机位实时预览、分屏预览、预览大小调整/收起、视角指示器/FOV/重置 |
| 对象工具 | 选择0、移动1、旋转2、缩放3、落地G、视图设置 |
| 资产 | 角色/物体/背景/机位、搜索、全部/我的；角色有12种性别/体型预设；导入提示Mixamo骨架FBX |
| 对象树 | 分类数量、展开、聚焦、锁定、实时机位预览、搜索；左右面板可调整/收起 |
| 环境 | 环境预设、背景/地面颜色、地面显示/不透明度、曝光、网格；主光颜色/强度/水平角/仰角/阴影，补光、轮廓光 |
| 机位属性 | 位置XYZ、注视点XYZ、FOV、画面倾斜、画幅比例；动态模式位置/注视点/FOV/倾斜旁有关键帧入口 |
| 动态时间轴 | 播放、循环、隐藏无动画轨、帧/时间显示、帧率、总时长、缩放、播放头、机位轨、分镜编排/添加分镜 |
| 运镜 | 添加运镜、绘制机位路径、绘制注视点路径；自定义固定注视点/固定机位/自由运镜；14个预设涵盖推近/拉远/横移/升降/环绕/半弧/螺旋/滑动变焦/倾斜与四方向摇镜 |
| 模式转换 | 静态→动态可选择不同步，或勾选确认后覆盖场景、保留演出时间轴、不复制姿势；动态→静态可不同步，或覆盖为当前帧场景/姿势/机位、不复制动作片段和关键帧 |
| 视频导出配置 | 机位来源、简化角色外形、画幅/时长、1280或1920长边、24/25/30/50/60fps；成片不含辅助线/控制点/编辑UI |
| 离开 | 保存更改弹层：继续编辑/直接关闭/保存并关闭；本地草稿与云端跨设备保存分开说明 |

证据：[静态工作区](advanced-source/director-stage.txt)、[动态工作区](advanced-source/director-motion-workspace.txt)、[动态截图](advanced-source/director-motion-workspace.png)、[机位属性](advanced-source/director-camera-properties.txt)、[运镜](advanced-source/director-camera-motion-presets.txt)、[导出](advanced-source/director-video-export.txt)、[帧率](advanced-source/director-video-fps.txt)、[模式转换](advanced-source/director-mode-return.txt)、[离开确认](advanced-source/director-leave.txt)。

### 可复用与差异

`canvas/domain/director.go`已有场景、对象、相机、灯光、镜头、骨架映射、动作片段、骨骼/transform关键帧及全景配置；`director_outputs.go`有正式截图/封面AssetID和镜头关系校验。现有模型不能据字段名称自动映射全部源功能：静态/动态两份场景同步、独立注视点路径、运镜预设参数、分镜时间排列、摄影导出配置与简化人物策略应逐项检查并扩充闭合合同。

本轮限定检索前端TS/TSX未找到three或react-three导入的正式渲染器；工具栏导演仍禁用，通用节点保存仍不接受director新建。因此缺口是真实可交互的集成编辑器与上述内部行为，不是一个按钮或iframe。继续使用画布域/正式媒体发布，不增设第二资产库；本地渲染截图必须上传为真实资产再绑定，视频输出必须有真实编码和文件验证。源站内置人物/动作模型不在当前仓库，不能把商业源资产直接复制为可用资源；必须采用有使用权的模型资产并检验行为/视觉差异。

退出检查：非空角色与物体加载、变换/骨骼/动作、机位/注视路径、关键帧插值、两模式同步/不同步、分镜编排、静态拍摄及图库/封面、动态导出且无编辑辅助物、保存回读/离开恢复、协作冲突与跨项目迟到输出防串写。当前仅观察界面与预设目录，没有编辑角色、创建运动、拍摄或导出，不能认定功能接受。

## 5. AI对话：JC-058

实页右侧Agent面板宽度初值400，可拖动调节。选中导演台时打开会自动带入“导演台”引用。顶部会话列表/新建会话在本次空状态禁用，提示只支持新建一个空会话，不能据此推断多会话全部可用。空态有专业模式按钮；技能目录包含视频反解、创作分镜、广告片导演、剧情短片、电商套图、海报设计、品牌设计，并有搜索、新建技能、管理技能。参考菜单为上传、资产库、画布；输入支持技能/主体引用，当前只有参考没有文字时发送禁用。本轮未输入或发送消息。

证据：[侧栏](advanced-source/assistant-panel.txt)、[截图](advanced-source/assistant-panel.png)、[技能目录](advanced-source/assistant-skills.txt)、[参考来源](advanced-source/assistant-reference-sources.txt)。

`canvas/types/canvas.ts:124`的会话/消息类型与本地数组不是正式会话服务。`operation/adapter/http/trial.go:48`明确 `/admin/ai-trials` 且要求platform_admin/AuthorizeDevelopmentTrial，不能削弱这层授权给画布成员使用。

可复用统一提示词文档、固定参考、模型能力、正式operation任务查询/取消/结果采用；需要正式项目/画布会话归属、消息输入快照、幂等发送/续聊、流式或任务结果、取消/未知恢复、工具注册/调用授权、建议变更预览与显式采用、跨项目围栏。技能必须具备实际定义/输入输出/执行器；只有下拉目录不计为完整能力。

退出检查：真实首轮与续聊、刷新恢复、参考快照、技能真实执行、失败/取消/未知恢复、节点结果与操作回写、输入期间切画布/节点删除/失权防串写。需要实际可用聊天模型/供应商能力与已授权调用条件；本轮未探测凭据，也未把管理员试验成功作为画布助手证据。

## 6. 实施接口顺序与证据条件

顺序表达依赖，不缩减任何能力：

1. 收敛高级节点的类型/配置、主体固定版本、接受后revision/input_revision；保持Yjs协议4与Go授权接受的唯一链路。
2. 主体选择→原子填充/新建→可撤销→回读→合法参考消费；复用统一主体库。
3. 时间线所有可见编辑与导出能力：前端创建/投影后接既有FreezeTimeline和mediatool；按真实源比例/轨道补合同，再逐格式实现有效文件。
4. 导演全部内部流程在集成编辑器实现：先保证场景/渲染/编辑/保存一致，再动作、路径、分镜、截图/视频等各自真实产物；每条仍是完整范围的子验收，不把阶段1声明为导演完成。
5. 正式AI会话和技能执行合同连接现有operation，再做侧栏交互和画布回写。真实服务不可用时保留恢复与错误态，不能删除错误文案伪造一致。

本轮实际命令为源码检索、文件读取、Git状态、`command -v ffmpeg/ffprobe`及 `docker compose ps`；未运行应用测试、迁移或生成。既有可执行验证入口包括 `backend/tests/canvas/timeline_reader_test.go`、`director_outputs_integration_test.go`、`backend/tests/media/export_renderer_test.go`、`backend/tests/subject/actual_generation_test.go`。其“文件存在”不是本轮通过；后续需要按真实环境前置运行，并单列skip/fixture/真实来源。
