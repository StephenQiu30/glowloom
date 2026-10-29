# 导演台实现差异审计（并行开发快照）

本报告仅只读对照 [DIR-001～035](../../13-即梦导演台完整需求分析.md)。当前 `frontend/src/components/canvas/director/` 是其他活跃聊天的未跟踪新增文件，后端与 Yjs 文件有并行修改；以下结论是固定时点快照，**不是最终代码或交付验收结论**。仅新建本报告，没有修改产品代码。

## 证据范围与判定

- 固定快照时间（UTC）：2026-10-28T16:47:49.334743+00:00；本地日期 2026-10-29（Asia/Shanghai）。快照副本位于 `/tmp/glowloom-director-implementation-snapshot-20261029`。
- 检查指定前端目录、document.go/editor_config.go、Yjs 导演方法。为避免把泛型 API 字段误判为功能缺失，追踪读取其直接所属 `domain/director.go`、`director_scene.go`、`director_outputs.go`。未审查其余业务服务全量实现。
- “未见”仅指本次目录/直接调用链内未见；“仅数据”表示 DTO/校验存在，但未找到前端交互或渲染消费；“部分”不表示验收通过。
- 设计栏只核对现有 design27 §7.4.6；本轮完整基线已经覆盖全部 DIR。未访问 Figma，因此所有行的 **Figma 状态未知**。未运行网站、单元测试、服务端保存、截图或编码，所有行的 **运行结果均未知/未执行**。

### 路径与行号简称

| 简称 | 快照对应工作区文件                                                                                                                                                                                           |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| W    | [frontend/src/components/canvas/director/canvas-director-workspace.tsx](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/frontend/src/components/canvas/director/canvas-director-workspace.tsx) |
| M    | [frontend/src/components/canvas/director/director-model.ts](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/frontend/src/components/canvas/director/director-model.ts)                         |
| S    | [frontend/src/components/canvas/director/director-scene.ts](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/frontend/src/components/canvas/director/director-scene.ts)                         |
| V    | [frontend/src/components/canvas/director/director-viewport.tsx](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/frontend/src/components/canvas/director/director-viewport.tsx)                 |
| A    | [frontend/src/components/canvas/director/director-animation.ts](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/frontend/src/components/canvas/director/director-animation.ts)                 |
| Y    | [frontend/src/components/canvas/yjs-live-session.ts](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/frontend/src/components/canvas/yjs-live-session.ts)                                       |
| Yw   | [frontend/src/components/canvas/yjs-workspace.tsx](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/frontend/src/components/canvas/yjs-workspace.tsx)                                           |
| B    | [backend/internal/canvas/domain/director.go](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/backend/internal/canvas/domain/director.go)                                                       |
| Bo   | [backend/internal/canvas/domain/director_outputs.go](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/backend/internal/canvas/domain/director_outputs.go)                                       |
| D    | [docs/design/27-无限画布一致性改造设计.md](/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server/docs/design/27-无限画布一致性改造设计.md)                                                           |

引用如 W397–400 指以上文件快照行号；工作区后续变化可能使行号漂移，应先比对下方 SHA-256。

## 核心结论

1. **两工作区合同尚未建立。** 前后端均为单场景加 mode，切换直接保存 mode。不能把“不删除关键帧”当成即梦的独立静态/动态布局，以及同步/不同步、冻结与引用检查。
2. **角色数据字段领先于渲染。** 后端有 rig、motion_clips、bone_tracks 等，当前 Three 加载器只返回 GLTF scene，未保留 animations，也没有 mixer/骨骼覆盖消费。几何人偶不能当作可摆姿角色。
3. **机位目标还是静态向量。** 变换关键帧能改变机位 Transform，不能独立给注视点、FOV、倾斜设置关键帧；lookAt 会覆盖相机朝向，需明确 roll 和目标求值合同。
4. **静态拍摄与视频输出没有闭环。** Screenshot/Cover DTO 是引用结构，不是拍摄或编码；预览隐藏辅助线不是输出验证。
5. **协作身份保护已有基础，但恢复语义不同。** epoch+整场景基线比较、保存确认和内存保留稿存在；关闭无未保存确认，刷新不恢复被拒绝的内存稿；undo 使用全画布历史，非导演台双工作区历史。

## DIR 差异矩阵

所有行运行状态 **U（未执行）**，Figma **U（未查）**；设计栏只表示文字设计存在程度。

| ID      | 实现状态 | 代码证据与差距                                                                           | 既有文字设计             | 运行 | 建议改动范围                                                                    |
| ------- | -------- | ---------------------------------------------------------------------------------------- | ------------------------ | ---- | ------------------------------------------------------------------------------- |
| DIR-001 | 部分     | W172–203、Yw74–85/237–244：节点/全屏及懒加载提示；无场景重试入口                         | D266/268基础             | U    | 补加载失败重试、返回焦点与节点身份稳定性；W/V                                   |
| DIR-002 | 未见     | W全文件只有数字输入 Enter/Esc（127–133），无首次操作说明/帮助                            | 未细化                   | U    | 引导与帮助、快捷键路由及输入/IME保护；W/独立交互模块                            |
| DIR-003 | 不符     | M64–80、B13–34单场景；W397–400仅更改 mode                                                | D270明确单场景差异       | U    | 增加版本化两工作区、旧稿迁移及各自编辑态；M/B/Y                                 |
| DIR-004 | 未见     | W397–400直接切换；M318–338无复制工作区命令                                               | D272待合同               | U    | 同步/不同步/取消及缺失角色引用校验；M/B/W                                       |
| DIR-005 | 未见     | 同上，无当前帧冻结为静态文档                                                             | D272待合同               | U    | 求值帧冻结、剔除片段/轨道、确认覆盖；M/A/W                                      |
| DIR-006 | 不符     | W478–493只有构图人偶；S66–76是几何拼装，无12角色库                                       | D268明确不可标作12类     | U    | 合法模型目录、角色页签、搜索/我的、骨架身份；W/V/S                              |
| DIR-007 | 部分     | W366–375/M341–368添加后立即提交固定位置，无地面待放置态                                  | 未细化                   | U    | 放置预览/取消/确认、模型加载反馈；W/S                                           |
| DIR-008 | 部分     | W628–764变换/颜色/锁定/显隐；S49–55仅原语使用color；没有对象opacity字段                  | D266/268基础             | U    | 补不透明度/恢复原色、模型材质着色、对象搜索/分组；M/B/S/W                       |
| DIR-009 | 仅数据   | B118–123有pose/rig/bone字段；S385–421不应用骨骼，W无姿势页签                             | D272待合同               | U    | 骨架识别/姿势编辑/重置/动作取帧；S/A/W                                          |
| DIR-010 | 不符     | W478–483五个入口，B287仅box/sphere/cylinder/plane/character                              | D268几何原型             | U    | 13几何与简模目录/尺寸模型/分类检索；M/B/S/W                                     |
| DIR-011 | 部分     | W505–513图片布景；V64–80平面纹理；B26/60有panorama但S未消费                              | D268授权资产基础         | U    | 全景/平面分类、球面渲染、画布图片引用、空态；M/S/V/W                            |
| DIR-012 | 部分     | W503只GLB；S92–117检查魔数/拒外链，V49–63校验项目与类型                                  | D268只定义授权模型       | U    | FBX/Mixamo/OBJ/GLTF解析与资源、容量、骨架、底部中心/单位归一；加载模块/上传服务 |
| DIR-013 | 部分     | W517–525仅添加默认机位；M93–108固定机位参数                                              | D272十四运镜非机位构图库 | U    | 机位构图预设、选择目标/前景/注视点交互；M/W/S                                   |
| DIR-014 | 部分     | W766–840有FOV/target；B131–148 target为静态向量，无独立目标实体/轨道                     | D268基础                 | U    | 注视点对象树、位置与目标通道、画面倾斜、机位级画幅；M/B/W/S                     |
| DIR-015 | 不符     | W996–1012/M199–209/B232有21:9和adaptive，无2.39:1；S508–534采用全局比例                  | D未细化                  | U    | 更正枚举/迁移与机位比例合同，预览/图片/视频统一尺寸函数；M/B/S/输出模块         |
| DIR-016 | 部分     | S495–536主导演视角+固定右下机位小窗，选择当前shot机位                                    | D268小窗                 | U    | 视角切换、预览机位独立选择、折叠/尺寸拖动/分屏；W/S                             |
| DIR-017 | 部分     | W978–995仅网格和吸附；M134 labels_visible存在但S未使用                                   | D268网格                 | U    | 辅助开关及渲染消费：路径/名称/高度/遮罩/指示器；M/S/W                           |
| DIR-018 | 部分     | W915–995背景/环境强度/地面色透明度/网格；S359–383半球光和地面                            | D268基础                 | U    | 环境预设/曝光/地面显示、主补轮廓灯及水平角仰角；M/B/S/W                         |
| DIR-019 | 部分     | W553–597工具按钮/落地；S170–175 OrbitControls，目录无keydown/up处理                      | D268轨道/变换控制        | U    | 导演自由飞行按键/鼠标触控板/焦点保护；落地须按包围盒不是直接y=0；S/W            |
| DIR-020 | 部分     | W248–269/1020–1125有20s30fps、循环、时间slider；无多轨/缩放/空轨隐藏/帧读数              | D270基础                 | U    | 时间轴统一时基与独立动态总时长、轨道UI、缩放；M/W/A                             |
| DIR-021 | 部分     | A3–31变换采样+四元数；W1126–1195创建删除插值；无目标/FOV/roll独立通道或曲线              | D270基础                 | U    | 通道关键帧/拖动/曲线/帧冲突与边界；M/B/A/W                                      |
| DIR-022 | 仅数据   | B81–88/119–123有MotionClips；S112–117只返回model.scene未保留animations；无AnimationMixer | D272待合同               | U    | 动作资产与库/预览、骨骼重定向、片段时间与碰撞；S/A/W/资产服务                   |
| DIR-023 | 未见     | W无新建动作/生成/导入动作/静态姿势片段                                                   | D272待合同               | U    | 真实动作生成服务能力核验；导入动作/姿势片段与失败态；W/业务服务                 |
| DIR-024 | 仅数据   | B81–88有start/duration/rate/loop；B331–334仅范围校验，无同角色片段碰撞                   | D272待合同               | U    | 片段编辑/复制/裁边/混合与碰撞、首帧下界；M/B/A/W                                |
| DIR-025 | 未见     | M318–338无路径命令；S仅变换关键帧，不存在路径编辑/沿路转向                               | D272待合同               | U    | 路径暂存/高度/时间/重采样/转向与覆盖校验；M/A/S/W                               |
| DIR-026 | 仅数据   | B172/275 camera_move枚举是镜头意图；S不消费为运镜轨道                                    | D272待合同               | U    | 十四运镜纯函数生成与预览确认、转为明确通道关键帧；A/M/W                         |
| DIR-027 | 未见     | B target为静态向量，W无路径引导/确认态                                                   | D272待合同               | U    | 固定目标/固定机位/自由双路径、事务确认、范围替换；M/A/S/W                       |
| DIR-028 | 部分     | M110–120与382–396 shots列表、引用、增删；W1040–1087选择镜头；无按顺序自动切机位          | D272待合同               | U    | 分镜时间区间/排序/裁时长/播放求值/输出源；M/B/A/W/输出模块                      |
| DIR-029 | 部分     | S185–195结束回调+W610–618保存baseline；Y702–758 CAS；W1022–1037使用全画布history         | D268/270基础             | U    | 取消拖拽事务/失焦清理、两工作区undo粒度、焦点与IME；S/W/Y                       |
| DIR-030 | 部分     | Y694–760内存Map保留冲突稿；Y873–931保存确认；W437直接关闭、447–463替换/放弃              | D270明确仅会话保留       | U    | 持久本地草稿/恢复/退出/覆盖确认；中文保存状态；版本/权限变化测试；W/Y/持久层    |
| DIR-031 | 仅数据   | Bo17–27截图/Cover ID与校验；W/S全文件无拍摄/截图/镜头素材流程                            | D272待产物               | U    | 干净机位截图、资产上传、镜头素材多选/下载/回写；S/W/媒体集成                    |
| DIR-032 | 未见     | W/S/V全文件无视频编码或导出配置；B镜头fps不是输出实现                                    | D272待产物               | U    | 导出控制器/机位来源/分镜/简化角色/1280或1920/fps/尺寸；S/W/媒体集成             |
| DIR-033 | 未见     | S529–534小窗隐藏helpers仅证明预览；无编码、进度、取消、恢复编辑状态                      | D272待产物               | U    | 离屏逐帧输出与编码生命周期、错误/重试、文件解码验收；S/输出模块                 |
| DIR-034 | 部分     | Y702–758、W1208–1246有epoch/CAS/资产归属；输出/回写路径未接入                            | D268/272边界             | U    | 保留现有身份守卫，扩展拍摄/视频/画布回写分段提交与幂等；Y/媒体集成              |
| DIR-035 | 部分     | S92–117/336–420/538–559与V38–96含abort/dispose、contextlost错误；未见逐资源重试/恢复     | D268基础                 | U    | 资源状态/重试、加载缓存与增量更新、上下文恢复、GPU泄露验收；S/V/W               |

## 需要保留的工程基础

- `config.director` 保持正式节点/项目身份；不要另建无法协作的上游本地 store。document.go:468–469 已路由到 validDirector。
- Y702–758 拒绝失效 epoch、节点缺失/类型变化、不可编辑、基线被其他协作者修改，并保留被拒编辑；新工作区命令应沿用这些保护。
- Y873–931 实际等待确认 revision 且 15 秒超时；保存反馈应翻译业务状态，不能把 view.phase 原始值当完整“已保存”交互。
- V49–63 校验项目与媒体 kind，临时 URL 只在运行时使用；新模型/动作/输出引用继续依赖正式资产 ID。
- S33–47/538–559 有几何材质纹理释放、abort、RAF/ResizeObserver/监听/控制器清理；可以保留并补齐资源缓存与失败重试。
- A3–31 已有位置/缩放插值和四元数旋转采样，适合保留为基础纯函数；扩展时需明确定义当前与新曲线的兼容。

## 针对本快照的具体注意点

- W584–593 “落地”直接设置 transform.position.y=ground.height，原语中心并非底部（S78–86），会将立方体一半埋入地面；应基于世界包围盒/底部锚点计算，同时兼顾 stage transform。
- S92–117 GLB 只检查 12 字节和 magic，不验证完整头/version/length；已有“已核验GLB”文案不等同本函数做了完整格式验证。
- S269–272 每次 scene 对象引用变化就 rebuild，S336–420 会释放并重载所有资产。当前修改任一属性可能重载全场景；需真实资源下测量并改为按稳定 ID/资源版本增量更新。
- B124–125允许 billboard 的 source_node_id 或 asset_id，但 V50 无 asset_id 一律报错；若现有文档只用画布节点引用，此路径不能加载。应建立正式解析器而不是放宽归属校验。
- B141–144 有焦距/光圈/对焦距离，但 S423–429只用 FOV/near/far 创建 PerspectiveCamera；不能宣称这些光学参数已经影响画面。
- editor_config.go 的 `EditorConfig` 是画布通用编辑器内容；本次导演台正式 schema 所有者是 director.go，不应把新增场景合同塞入通用 Metadata。

这些是静态路径观察或可解释风险，尚未通过浏览器运行复现；并行作者可能在本快照后继续修复。

## 文件身份

| 快照文件                                                                | SHA-256                                                            | 字节  |
| ----------------------------------------------------------------------- | ------------------------------------------------------------------ | ----- |
| `frontend/src/components/canvas/director/director-viewport.tsx`         | `9c48a9200d8ee4fa0fb2d09a73b35c40f34423de60e03de63f6a4b6c28876151` | 3789  |
| `frontend/src/components/canvas/director/director-model.ts`             | `8f13c89b91775751bd1456125494cbb2895e97d3350dba480b04bf588940b453` | 14191 |
| `frontend/src/components/canvas/director/director-animation.ts`         | `fb8621b9de33011dd99cc17498afdc3d80e2fb68b5ace940d4688a536de05767` | 1344  |
| `frontend/src/components/canvas/director/director.css`                  | `216fee15edf39afb3daf1035e27f509643880e48cbf0acddcd73506a5a29d39b` | 4441  |
| `frontend/src/components/canvas/director/director-scene.ts`             | `2e881e8cb769a6068c1d56755a825f9ccf68fd6dfe1a5697b538a7deb4ad1500` | 20073 |
| `frontend/src/components/canvas/director/canvas-director-workspace.tsx` | `25287eef109feedcad87335020631cdcf009617dfd44a38739f03b6216942e88` | 40822 |
| `backend/internal/canvas/domain/document.go`                            | `54dff13d91c2b06b9511454d365413df171cecfcdce5a51f6d9e1f41889bf6e0` | 18748 |
| `backend/internal/canvas/domain/editor_config.go`                       | `68a1cc0a5d8a7b7e7734cd2e274fd30a8f3584b5a629d4bf1008821d61988783` | 7366  |
| `frontend/src/components/canvas/yjs-live-session.ts`                    | `47d644fc5e633566812a3fb454a42dbcddbe1cfa8983683fe3ebbd0db9fd2823` | 61253 |
| `frontend/src/components/canvas/yjs-workspace.tsx`                      | `9031bc40e922811a82943d7f26369ac5928164a35833b0d3aebf72d0182c2e5d` | 18424 |
| `backend/internal/canvas/domain/director.go`                            | `e13bf4ab674f1c2ffee9c39acbbd50a6602ab384bfe5230f5f37ebe0f4a214db` | 17754 |
| `backend/internal/canvas/domain/director_scene.go`                      | `95542957e97bb993d95108b27fb516b0872740622ce227df8c31eb8153c564a6` | 323   |
| `backend/internal/canvas/domain/director_outputs.go`                    | `d97b20e4d7c147205fff3d2e44f439b350d432495d9ef3947eb74d89bf2d1f1d` | 2872  |
| `docs/design/27-无限画布一致性改造设计.md`                              | `c22cc12b55d42801ba9728bc4ba5fdab4aea214dbe46ba0c4e8431469d27f59c` | 42353 |

## 快照后变动

报告写入时复核发现 `director-scene.ts` 与 `canvas-director-workspace.tsx` 已被并行作者继续修改，哈希不同于本次快照。本报告保留原始审查边界，不将后续未读代码套用为同一状态；实施前须复核这两个文件。

## 验证与边界

执行了限定路径 `rg`、逐行源码读取、固定快照及 SHA-256。未执行任何测试/构建、浏览器动作、服务启动或产品代码写入。全目录检索未见截图/录制/编码控制器或 AnimationMixer/动作骨骼消费；检索边界仅为当前 director 目录，不能推导仓库其他目录绝无可复用实现。现有设计已明确区别基础场景与完整一致性，后续应更新设计再落实现，不能将 35 项按按钮存在勾选通过。
