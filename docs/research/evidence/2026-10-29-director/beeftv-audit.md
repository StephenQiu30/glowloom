# BeefTV 导演台实现整理与迁移审查

审查日期：2026-10-29（Asia/Shanghai）。范围只包括 BeefTV；未读取 Glowloom 功能实现。以下“已确认”均为源码事实，并非本轮运行验收。

## 1. 结论与来源基线

- 可以以 BeefTV 已有的场景、机位、插值、路径、骨骼、渲染、导出和保存状态语义为实现依据，整理为 Glowloom 自己的模块。不要整体复制应用或把原 workbench 作为嵌入页面；其 UI 和存储依赖属于另一产品。
- 固定可复用基线：`2327ee2d80282b10dd8bef0f26b48990d46f5c3b`，该版本根 LICENSE 为纯 MIT，并包含导演台的完整源码与测试目录。通过 GitHub 插件和本地 git checkout 双重核实。
- 调研时 `main` 为 `285c7dec8ee4f2ddc9c53b32076e664b9d8f813b`。最新版 LICENSE 为 BeefTV Source Available License 1.0，明确保留上述历史 MIT 权利，但新增覆盖代码对外服务（包括免费）、商业嵌入或交付要求另行书面授权。因此不能把最新 main 当作 MIT 基线。
- 最新版相对所选 MIT 基线的导演台差异集中在 10 文件，增加约 564 行、删除约 108 行，包含新的 `director-pose-rig.ts` 和 `director-pose-solving.ts`；不要未经逐项来源核实混入这些新增实现。
- 原始源码检出：`/tmp/glowloom-beeftv-audit-20261029`，detached HEAD 固定上述 MIT commit，工作区干净。未安装依赖、未启动服务、未修改 Glowloom。

一手链接：

- [固定版本 LICENSE](https://github.com/glanderness/BeefTV/blob/2327ee2d80282b10dd8bef0f26b48990d46f5c3b/LICENSE)
- [固定版本 NOTICE](https://github.com/glanderness/BeefTV/blob/2327ee2d80282b10dd8bef0f26b48990d46f5c3b/NOTICE)
- [最新版许可证边界](https://github.com/glanderness/BeefTV/blob/285c7dec8ee4f2ddc9c53b32076e664b9d8f813b/LICENSE)

## 2. 实际入口与职责

所有源码路径和行号均相对于上述固定基线。

| 职责 | 源码证据 | 迁移方式 |
|---|---|---|
| 画布创建与打开导演台、输出回画布 | `web/src/pages/canvas/use-canvas-director.ts:48`；创建入口 `:76`，输出 `:160` | 重写为当前项目的节点/资产/项目接口；保留输出前后校验场景身份、避免异步覆盖的新鲜状态语义 |
| 按需打开大型编辑器 | `web/src/pages/canvas/canvas-project-editor-dialogs.tsx:47`、`:315` | 保留延迟加载原则，使用当前项目的加载/错误边界 |
| 编辑器流程协调 | `web/src/components/canvas/director/canvas-director-workbench.tsx:61` | 约 2201 行、依赖 AntD/资产 store/配置/上传/AI；按对象、机位、时间轴、输出、保存职责拆解重写 |
| 3D 场景运行时 | `web/src/components/canvas/director/director-viewport.tsx:108` | 约 2069 行；提炼渲染行为及生命周期，不将业务服务和大组件连带复制 |
| 编辑器临时状态 | `web/src/stores/canvas/use-director-workbench-store.ts:44` | 选择、视角、播放头、工具、面板高度与场景数据分开；使用项目现有状态方式 |
| 可持久化场景合同 | `web/src/types/director.ts:1`–`:227` | 梳理为项目的 versioned scene schema，并做边界校验与旧数据迁移 |

`DirectorScene` 包含 objects、groups、cameras、lights、shots、activeShotId、环境、网格、地面、舞台变换、全景图和时间戳。`DirectorObject` 区分 primitive/model/actor/billboard；对象带 transform、显隐、锁定、颜色、骨架、动作片段、关键帧和来源资产。`DirectorShot` 是引用机位的镜头配置（时长、fps、景别、运镜、截图历史），不是完整场景。保存内容、临时视图、输出资产三者应保持分离。

## 3. 可迁移能力与具体合同

| 能力 | 已确认算法/行为 | 可复用源码证据 |
|---|---|---|
| 对象操作 | 创建、复制、分组/解组、组显隐/锁定、独立统一缩放；锁定和可见不同维度 | `director-scene.ts:42`–`:190` |
| 变换插值 | position/rotation/scale 轨道独立；位置缩放线性插值、旋转 Quaternion slerp；step/linear/smooth | `director-scene.ts:344`–`:402` |
| 编辑动画语义 | Auto Key 与普通变换区分；群组多选保留相对关系；播放头按帧对齐 | `director-animation-semantics.ts:113`、`:129`、`:168`、`:278`、`:331` |
| 拖拽事务 | begin/update/commit/cancel；pointerup、cancel、blur、visibility 等终止事件收敛为一次事务 | `director-gesture-transaction.ts:27`、`:95` |
| 相机视图 | free/camera/正交视图；相机有效性、舞台坐标转换、取景 up 向量与正交包围盒 | `director-view-modes.ts:45`、`:144`、`:169`、`:389`、`:414` |
| 相机预设 | 当前视角、正面/背面/侧面、过肩、俯视、低角度、荷兰角等 15 项 | `director-camera-presets.ts:6`、`:26` |
| 相机跟随/注视 | 跟随位置与注视对象分离；绑定/解绑烘焙位移，删除目标时冻结构图并清除悬空引用 | `director-camera-binding.ts:14`–`:52` |
| 相机运镜 | 手绘 pencil/pen、直线/圆形/矩形路径、运镜 replace/append、独立位置/旋转/焦点/FOV 关键帧 | `director-camera-paths.ts:14`、`:48`、`:109`、`:157` |
| 角色轨迹 | 钢笔/铅笔、直线/环形/矩形；重定时、朝向轨迹、移除恢复原手动关键帧 | `director-actor-paths.ts:10`、`:43`、`:76`、`:102`、`:144` |
| 角色与骨骼 | GLTFLoader、SkeletonUtils 克隆、AnimationMixer；静态姿势、骨骼 overrides、骨骼轨道、动作片段分层 | `director-viewport.tsx:1274`、`:1338`、`:1448`、`:1772`；`director-animation-semantics.ts:219` |
| 模型加载与恢复 | generation 标识、过期加载拒绝、context lost/restored、显式 retry、失效时停止截图/录制 | `director-recovery.ts`；`director-viewport.tsx:190`–`:242` |
| GPU 资源 | geometry/material/texture 去重释放；共享 clone 不重复释放；停止 mixer/解除缓存 | `director-resources.ts:51`–`:143` |
| 时间轴 | 对象变换/骨骼与机位轨道；帧吸附、轨道展开、关键帧删除、缓动、播放、时轴缩放 | `director-sequencer.tsx:59`、`:74`、`:127`、`:145`、`:242`、`:261` |
| 机位小窗 | 同一 renderer 的 WebGLRenderTarget 离屏渲染；恢复 target/viewport/scissor/clear 状态 | `director-viewport.tsx:1909`–`:1968` |
| 静态输出 | beauty/clay/depth/normal/pose 路径；截图临时隐藏编辑辅助并恢复 | `director-viewport.tsx:1881`–`:1903` |
| 动态输出 | canvas.captureStream + MediaRecorder；裁切、格式协商、WebM 时长修复、首帧解码与时长检查 | `director-viewport.tsx:1970`–`:2064`；`director-recording-format.ts:1` |
| 保存状态 | dirty/saving/saved/error；revision/confirmedRevision；300ms 防抖、串行 drain、本地草稿、显式恢复、关闭前 flush | `director-save.ts:139`–`:454` |
| 异步输出身份 | 项目、源节点、scene/shot 与用户 scope 重新验证；输出不覆盖整份过期 scene | `use-canvas-director.ts:160` 之后；`director-session.ts` |

表中的 `director-*.ts` 默认位于 `web/src/lib/canvas/director/`，TSX 默认位于 `web/src/components/canvas/director/`。

## 4. 与即梦对齐时不能直接继承的产品差异

以下对即梦的引用仅来自用户提供的截图，其他项需要主调查实测，不将 BeefTV 的能力当作即梦需求。

1. 即梦顶层是“静态预演 / 动态预演”；BeefTV 核心工作模式为 layout/pose/animate/camera（显示“摆场/姿态/动画/摄影机”），另有 scene/preview 工作区。应复用能力切换的状态清理规则，重建即梦式两态产品入口与命名，不能照搬四模式导航。
2. 即梦截图给出 Q/W/E、A/S/D 连续移动，I/J/K/L 连续旋转，Shift 加速。BeefTV `director-shortcuts.ts:32`–`:82` 主要为 V/R/F（兼容 W/E）变换、方向键 nudge、空格播放；这一映射与截图冲突，必须按即梦重新定义。保留其输入框/控件焦点拦截思想。
3. 即梦截图左侧列出男性/女性的标准、少年、儿童六类。MIT 基线 UI 仅直接开放“标准男性、标准女性、几何模型”，有 3×3 群众；其 shape 数据虽有 teen/child 等，但不能把数据存在当作已与即梦角色库一致。
4. BeefTV 的角色姿势为程序化姿势与 glTF 动作片段；不能把 `walk` 静态 pose 当作完整步行动画库，更不能宣称已包含即梦全部动作或用户自定义角色链路。
5. BeefTV 的 `aperture`、`focusDistance` 在类型中存在，但本次对 viewport 与 view-modes 的定向搜索未找到其渲染使用；不据字段存在声称物理景深已实现。即梦如要求景深，需独立验证并补齐。
6. 录制是浏览器实时 MediaRecorder，不是确定性逐帧离线渲染。其错误文案提到“帧序列”，但本次没有据此认定存在帧序列导出 UI。分辨率、帧率、码率、时长与即梦一致性必须另验。
7. 上游渲染中材质、背景、辅助线、快照/白膜有不同处理路径；改视觉不能只改 CSS。应明确交互视口和最终输出的哪些对象/辅助元素可见。

## 5. 技术依赖与资产归属

- MIT 基线 `web/package.json` 声明 React 19.2.7、Three ^0.185.1、@react-three/fiber ^9.6.1、@react-three/drei ^10.7.7、three-stdlib ^2.36.1、Zustand ^5.0.14、nanoid ^6.0.0、@fix-webm-duration/fix 1.0.1。这里只记录上游版本，不要求 Glowloom 跟随升级；第二阶段需依当前锁文件选兼容版本。
- 小型纯函数通常只依赖 Three、nanoid 和 Director 类型，可按功能整理并保留来源说明。渲染组件依 R3F/Drei/three-stdlib，媒体解析应改用项目现有资产服务。
- 上游 UI 的 AntD、自己的主题/store、AI 全景生成、用户 scope、localStorage 和桌面更新 hook 均不应成为迁移的隐式依赖。
- 两个人体模型分别为 `web/public/canvas/models/quaternius-standard-male.glb`（740952 bytes）和 `quaternius-standard-female.glb`（1012184 bytes），同目录 LICENSE.md 说明来源为 Quaternius Universal Base Characters 免费 Standard 包，CC0 1.0，65 关节，转换时去掉原图片/材质。可保持出处记录后复用，不能混入付费 Source 包。
- [男性模型来源说明](https://github.com/glanderness/BeefTV/blob/2327ee2d80282b10dd8bef0f26b48990d46f5c3b/web/public/canvas/models/quaternius-standard-male.LICENSE.md)；[女性模型来源说明](https://github.com/glanderness/BeefTV/blob/2327ee2d80282b10dd8bef0f26b48990d46f5c3b/web/public/canvas/models/quaternius-standard-female.LICENSE.md)。本轮核验的是仓库自带来源声明，未独立下载原始作者包核对字节。
- 导演台该录制路径本身不直接依赖 FFmpeg；不能为了迁移导演台把上游整套 @ffmpeg/core（GPL）和其他无关应用依赖一起带入。
- 源码整理后仍需保留 MIT 许可、版权、固定 commit、原路径、修改说明。不是只改变量名就丢掉归属；无需使用 BeefTV 品牌或 logo。

## 6. 项目接入合同（设计建议，尚未对照当前实现）

建议按实际职责接入当前项目已有结构，不预建空包装：

1. 场景内容：以 ID 引用 object/camera/shot/asset；所有持久化数值校验有限值、单位、合法范围；时间顺序规范化。编辑器 mode、选择、hover、窗口位置和播放状态不写场景。
2. 编辑命令：变换、关键帧、分组、显隐、锁定、删除、绑定等以事务更新领域数据；拖拽持续预览，结束一次 undo 记录。播放采样不能反写保存内容。
3. 渲染：场景数据 → Three 对象；资源加载、渲染循环、事件、camera preview、export 生命周期有明确 owner；卸载/失焦/context lost 都收敛。
4. 服务：由现有项目系统提供资产解析/上传、保存确认、权限、场景版本和输出节点提交；上游“内存 store 已修改”不是当前产品的服务端保存成功。
5. UI：按即梦实测整理静态/动态、资产/对象/属性、角色/物件/图片/机位、导演视角/机位视角、截图/导出/退出。复用项目 UI 组件和设计规范。
6. 输出：截图/视频与原 scene/shot 建关联，上传及画布回写失败可重试；项目切换/节点删除/权限失效后不得回写旧结果。

## 7. 应继承的验证用例

上游存在测试不等于本轮通过。至少将以下行为适配为项目测试：

- 插值前后边界、单关键帧、独立轨道、Quaternion 最短转动、非法值拒绝；Auto Key 开关、拖拽事务取消/失焦只提交一次。
- 跟随绑定/解绑不跳构图、目标删除清引用；选中机位与实际截图机位一致；预览小窗不改变主视口。
- 模型加载乱序、卸载后回调、context lost/restored、重复开关导演台的 GPU/事件资源清理。
- 序列播放不污染保存、保存失败不显示成功、重试不丢最新修改、草稿恢复与场景 scope 隔离。
- 截图不含 gizmo/网格标签等编辑辅助；真实视频首帧可解码、时长/比例/帧率/输出格式准确，输出失败不产生假素材。
- 资产上传期间切换项目、删除源节点、再次编辑或断网时不覆盖新状态；多用户协作行为另按当前项目模型验证。
- 即梦快捷键在输入框、弹窗、菜单与失焦场景中不误触；静态/动态切换时停播和工具状态符合实测。

## 8. 已答与未知

- 已答：固定版本、许可证差异、入口、主要场景合同、算法和渲染/导出/保存路径、第三方模型仓库来源、迁移边界。
- 部分答：完整姿势/动画质量、导出格式/时序准确性、复杂 glTF 兼容性、模型原始包授权核对，需运行/来源补验。
- 未答：与即梦完整交互的一致性、Glowloom 现有缺口、当前项目架构的具体改动路径、设计稿准确映射；本轮按用户顺序不检查 Glowloom 功能实现。
- 没有运行上游测试、浏览器或真实导出，没有将源码存在写成通过验收。
