# 第三方代码声明

本文保留当前及历史实现的来源、修改和版权追溯。下节为当前画布适配；其他历史前端能力与旧路径已移除，不属于当前浮光页面的实现或需求；原始许可文本继续保留。

## Lucky 画布能力适配（2026-10-23）

固定源码为本地 Lucky 提交 `33b51b0e718918ada4239835e92a166a303cbafa`，继承其画布第三方声明；完整许可与传递来源随应用保存在 [Lucky 画布许可](/licenses/canvas-sources.md)。

从 `frontend/src/components/canvas` 的 viewport、wheel、node-drag、connection-geometry、view-state、image-editor-viewport、crop/split/upscale dialogs 迁入 `frontend/src/components/canvas` 同职责模块。改写为本项目 UUID、类型、shadcn、主题、视图身份隔离、取消处理、正式素材上传与 Go/OT 持久回执；不复制 Lucky 的账户、业务接口、环境配置或 Yjs 服务。本声明只覆盖实际迁入代码，不代表完整画布或多人协作已经验收。

## 当前BeefTV画布适配（2026-10-09）

固定来源：[glanderness/BeefTV `a2196d32782be6090beb003590ce11a956a423e1`](https://github.com/glanderness/BeefTV/tree/a2196d32782be6090beb003590ce11a956a423e1)，MIT。版权为2026 @beefnoode、BeefTV contributors、basketikun和ddcat。[完整许可](/licenses/beeftv.txt)与[上游NOTICE](/licenses/beeftv-notice.txt)随应用分发；NOTICE包含Infinite Canvas派生来源及其重许可说明。

以下源路径相对于该固定提交`web/src/`；目标均位于浮光`frontend/src/app/canvas/`。

| 目标                                       | 上游源                                                                                                           | 改写范围                                                                                     |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `viewport/surface.tsx`                     | `components/canvas/infinite-canvas.tsx`                                                                          | 指针/触控/滚轮、空格平移、48px网格与生命周期；替换上游主题/广播，排除Radix浮层，接本项目框选 |
| `viewport/transform.ts`                    | `lib/canvas/canvas-live-viewport.ts`、`canvas-selection.ts`                                                      | 合成层变换、指针意图与坐标转换，去除未使用的图形层广播                                       |
| `node/node.tsx`                            | `components/canvas/canvas-node.tsx`                                                                              | 外置节点标题、铅笔入口、媒体容器、四角尺寸手柄与选中样式；接真实草稿，去除本地媒体/任务调用  |
| `composer/composer.tsx`                    | `components/canvas/canvas-node-prompt-panel.tsx`                                                                 | 紧凑提示词面板、参数与提交位置；改为shadcn与服务端草稿，生成明确禁用                         |
| `node/panel.tsx`、`node/panel-position.ts` | `components/canvas/canvas-workspace-overlays.tsx`、`pages/canvas/project.tsx`                                    | 选中节点锚点、16px下间距、660～920px宽度与逐帧视口/节点矩形；手机仅缩窄，不固定底部          |
| `chrome/toolbar.tsx`                       | `components/canvas/canvas-toolbar.tsx`、`canvas-zoom-controls.tsx`、`components/ui/aceternity/floating-dock.tsx` | 44px胶囊Dock、抓手/框选、创建、缩放与快捷键；使用Radix浮层，去除上游工具注册表与Motion依赖   |
| `viewport/minimap.tsx`                     | `components/canvas/canvas-mini-map.tsx`                                                                          | 世界边界、500单位留白、小地图与视口定位；无媒体解码                                          |
| `editor.css`                               | `lib/canvas-theme.ts`、`styles/globals.css`                                                                      | 保留明暗语义表面、节点/浮层/Dock形状及尺寸；限定画布页，去除全局覆盖与重复规则               |
| `editor.tsx`                               | `pages/canvas/canvas-project-top-bar.tsx`                                                                        | 浮动项目簇与工作区编排；Next.js路由、现有Go命令、不可变输入、回执与冲突恢复由浮光维护        |

未复制品牌图像、模型权重、第三方媒体或其他工具资源。来源登记说明实际改写范围，不表示上游全部业务工具、真实生成或产品验收完成。

## 字幕转写运行时与模型

本机字幕转写使用官方 [ggml-org/whisper.cpp v1.9.4](https://github.com/ggml-org/whisper.cpp/tree/927cfce34f31707e17f2bff35c349632fb9e2c3a)，固定源码提交 `927cfce34f31707e17f2bff35c349632fb9e2c3a`；代码适用 [MIT 许可](https://github.com/ggml-org/whisper.cpp/blob/927cfce34f31707e17f2bff35c349632fb9e2c3a/LICENSE)，Copyright 2023–2026 The ggml authors。

采用的多语 `ggml-base.bin` 权重来自官方转换仓库 [ggerganov/whisper.cpp](https://huggingface.co/ggerganov/whisper.cpp/blob/5359861c739e955e79d9a303bcbc70fb988958b1/README.md)，固定提交 `5359861c739e955e79d9a303bcbc70fb988958b1`，模型卡声明 MIT；文件 147,951,465 字节，SHA256 `60ed5bc3dd14eea856493d334349b405782ddcaf0028d4b5df4088345fba2efe`。源代码与权重保存在仓库外，使用原字节。Glowloom 的 Go adapter 管理音频预处理、受控请求、时间戳与语言验证、取消和私有结果保存，不将该原生运行时改为业务服务或供应商费用事实。

## 视频深度推理

Glowloom 的 Go 任务消费者使用固定 [Video Depth Anything](https://github.com/DepthAnything/Video-Depth-Anything/tree/4f5ae23172ba60fd7bc11ef671cca678842c7072) 的 18 个实际模型依赖文件作为离线执行包，保留源文件头部及 Apache-2.0 声明。该包包含 ByteDance、Meta Platforms（DINOv2）、Hugging Face（attention）和 guoyww/AnimateDiff（motion module）的原始署名；完整原许可保存在 [docs/licenses/video-depth-anything.txt](/licenses/video-depth-anything.txt)，公开入口 `/licenses/video-depth-anything.txt`。最小执行包清单聚合 SHA256 为 `275d52984b0f60536f53e778c8d38b082a8ff945b174f49d449e6860d1ee0342`；逐文件来源与摘要由 `backend/internal/mediatool/adapter/videodepth/source_manifest.json` 声明。未调用的上游 CLI、Gradio、stream 与未核验许可的 Tencent `dc_utils.py` 不包含在此执行包中。

当前采用的 Small 权重来自官方固定版本 `256875362cff76724b920335dfb4b29dd611f66e`，模型卡声明 Apache-2.0。固定来源、权重大小与 SHA256 见 [docs/licenses/video-depth-anything-small.md](/licenses/video-depth-anything-small.md)，公开入口 `/licenses/video-depth-anything-small.md`。该说明仅针对实际采用的 Small 权重。

`backend/internal/mediatool/adapter/videodepth/worker.py` 的全片 P2/P98 与 gamma 后处理改写自固定 BeefTV `1ae25027` 的 `tools/depth-capture/depth_capture/pipeline.py:108–129`，沿用 [BeefTV MIT 许可](/licenses/beeftv.txt)。Glowloom 用完整 CFR 预处理、逐帧等价计算、Go 所有的进程取消/等待、私有对象和正式审核替换上游本地脚本运行与文件输出；源模型结构与 Small 权重保持。

## 时间线算法的上游声明

BeefTV `1ae25027` 的 `web/src/lib/timeline/timeline-placement.ts` 和 `timeline-snap.ts` 标明移植自 `yoqu/lingji-cut`。Glowloom 的 `frontend/src/components/canvas/timeline.ts` 改写其碰撞、间隙和吸附算法，使用自身 UUID、闭合输入和画布保存合同。保留该传递来源与修改声明，不将其视为 BeefTV 原创。

- 原项目：[yoqu/lingji-cut](https://github.com/yoqu/lingji-cut)，Copyright 2026 yoqu。
- 许可：Apache-2.0；从公开固定提交 `59a2fc9f8bd00ca243b2b0d4c4e31b67eb6387d1` 保存原始许可全文至 [docs/licenses/lingji-cut.txt](/licenses/lingji-cut.txt)。该 SHA 是许可核验快照，BeefTV 未标明其算法移植时的确切源提交。
- 运行时许可入口：`/licenses/lingji-cut.txt`。

## BeefTV 无限画布与工作台

导演台镜头图库与封面沿固定 `1ae25027` 的 `lib/canvas/director/director-session.ts`、`director-cover-write.ts` 和 `components/canvas/director/director-camera-screenshot-tabs.tsx` 改写到 `frontend/src/components/canvas/director/outputs.ts`。`gallery.tsx` 参考 `components/canvas/director/director-screenshot-gallery.tsx`，使用自身 shadcn/Radix；`capture-result.ts`、编辑器和节点内容通过正式审核资产、画布 CAS 与持久幂等关联，替换源应用本地路径和 Ant Design 调用。保留上述 MIT 来源与修改说明。

白膜视频录制将固定 `1ae25027` 的 `components/canvas/director/director-viewport.tsx:1292–1395` 适配到 `frontend/src/components/canvas/director/recording.ts`；`viewport.tsx` 与 `workbench.tsx` 的录制流程参考 `components/canvas/canvas-director-workbench.tsx:833–866`。沿 MIT 保留来源，改为自身镜头身份、保存修订、有界录制、取消与资源清理；本地人工预览、结果核验、Go/FFmpeg WebM→MP4 规范化和正式媒体审核由 Glowloom 实现。

画布核心适配 Next.js/shadcn 与正式 Go 合同，固定来源为 BeefTV `0d9e9f48`。工作台能力参考固定 `1ae25027`，首页/导航组织参考 `web/src/pages/home/home-dashboard.tsx` 与 `components/layout/workspace-sidebar-nav.tsx`，改写为自身 App Router 和 shadcn 工作台。新增移植文件逐项登记。

- 固定来源：`0d9e9f48d407570cd431ad9730cdd522b06810c0`。
- 许可：MIT，完整版权与许可文本保留于 [docs/licenses/beeftv.txt](/licenses/beeftv.txt)，运行时入口 `/licenses` 和 `/licenses/beeftv.txt`。
- 版权：2026 @beefnoode and BeefTV contributors；2026 basketikun；2026 ddcat。
- 上游来源：BeefTV 的 `NOTICE` 声明其包含 Infinite Canvas v0.5.0（`568f0f1838df8de31fe885a4e130e2f346dd14ab`）的派生代码，并记录上游在 `890ba95858bbb13496d23978003716656109abb2` 改为 MIT 许可。

下表的源路径均相对于固定 BeefTV 提交的 `web/src/`；目标相对于 Glowloom `frontend/src/components/canvas/`。许可登记说明来源与改写范围，不证明完整功能或真实生成验收完成。

| 目标文件                             | 源文件                                                                                                                                  | 适配范围                                                                       |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `engine/infinite-canvas.tsx`         | `components/canvas/infinite-canvas.tsx`                                                                                                 | 平移、滚轮与双指缩放、背景层；接入正式类型和主题，缩放上限按 Glowloom 契约调整 |
| `engine/theme.ts`                    | `lib/canvas-theme.ts`                                                                                                                   | 颜色与背景定义；接入应用主题                                                   |
| `engine/appearance.ts`               | `lib/canvas/canvas-appearance.ts`                                                                                                       | 外观参数；移除源应用的本地存储                                                 |
| `engine/viewport-dom.ts`             | `lib/canvas/canvas-live-viewport.ts`                                                                                                    | DOM 临时视口与提交视口同步                                                     |
| `engine/spatial-index.ts`            | `lib/canvas/canvas-spatial-index.ts`                                                                                                    | 空间索引与可见区域查询                                                         |
| `engine/selection.ts`                | `lib/canvas/canvas-selection.ts`                                                                                                        | 框选、选择与拖拽辅助算法                                                       |
| `engine/viewport.ts`                 | `lib/canvas/canvas-viewport.ts`                                                                                                         | 视口与世界坐标转换                                                             |
| `engine/layout.ts`                   | `lib/canvas/canvas-layout.ts`                                                                                                           | 节点布局算法                                                                   |
| `engine/connection-tilt.ts`          | `lib/canvas/canvas-connection-tilt.ts`                                                                                                  | 连线计算                                                                       |
| `engine/viewport-render-sync.ts`     | `lib/canvas/canvas-viewport-render-sync.ts`                                                                                             | 视口渲染同步                                                                   |
| `engine/use-selection-controller.ts` | `pages/canvas/use-canvas-selection-controller.ts`                                                                                       | 选择控制；移除源应用批量生成与本地目录职责                                     |
| `engine/use-viewport-transition.ts`  | `pages/canvas/use-canvas-viewport-transition.ts`                                                                                        | 视口过渡与聚焦                                                                 |
| `engine/history.ts`                  | `pages/canvas/use-canvas-history.ts`                                                                                                    | 历史机制；改用服务端命令和逆命令                                               |
| `engine/keyboard.ts`                 | `pages/canvas/use-canvas-keyboard.ts`                                                                                                   | 快捷键；适配应用组件的输入边界                                                 |
| `engine/clipboard.ts`                | `pages/canvas/use-canvas-node-operations.ts`                                                                                            | 复制、ID 映射、位置偏移与内部连线重建                                          |
| `engine/frame.ts`                    | `lib/canvas/canvas-frame.ts`                                                                                                            | 分组的世界坐标、移动与解除关系                                                 |
| `engine/alignment.ts`                | `lib/canvas/canvas-project-domain.ts` 第 325–398 行                                                                                     | 对齐与间距算法                                                                 |
| `engine/connections.tsx`             | `components/canvas/canvas-connections.tsx`                                                                                              | SVG 连线层                                                                     |
| `engine/minimap.tsx`                 | `components/canvas/canvas-mini-map.tsx`                                                                                                 | 缩略图与视口定位                                                               |
| `engine/virtualization.ts`           | `pages/canvas/use-canvas-render-model.ts`、`lib/canvas/canvas-performance-mode.ts`                                                      | 可见区域迟滞、节点预算、连线裁剪与媒体分级渲染                                 |
| `engine/video-hover-preview.ts`      | `lib/canvas/canvas-video-hover-preview.ts`                                                                                              | 单解码器悬停租约、延迟、播放期限与取消                                         |
| `engine/toolbar.tsx`                 | `components/canvas/canvas-toolbar.tsx`、`components/canvas/canvas-workspace-overlays.tsx`、`components/ui/aceternity/floating-dock.tsx` | 底部浮动 Dock、附着选区工具栏与位置更新；改用 shadcn，移除工具注册表和本地偏好 |
| `nodes/node-shell.tsx`               | `components/canvas/canvas-node.tsx`                                                                                                     | 卡片、标题、连接端点与缩放手柄；移除生成和裁剪工具                             |
| `canvas.css`                         | `globals.css`                                                                                                                           | 合成图层和选中态样式，限定到画布组件作用域                                     |

`model.ts`、`document.ts`、`store.ts`、`queries.ts`、`workspace.tsx`、`editor.tsx` 与节点内容负责 Glowloom 的领域适配：身份与权限、项目资源、服务端 revision、原子命令、持久幂等和媒体预览。业务合同见工作台设计。

以下 MIT 算法与输入合同按自身技术栈改写。源提交为 `1ae25027f7ea1c2178e1e4133c36a0f2995d0e98`，目标均按 Glowloom UUID、服务端持久化、权限和关闭的输入范围改写；不保留源 Wails 外壳或本地数据库。未完成能力以正式设计清单为准，不能由许可登记推断已验收。

| 目标（相对 `frontend/src/components/canvas/`）                                                                                        | 来源与改写内容                                                                                        |
| ------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `batch-table.ts`                                                                                                                      | BeefTV 批量表格行列、引用槽位、排序与编辑；改用 UUID 与服务端画布命令                                 |
| `image-tools.ts`、`subtitles.ts`                                                                                                      | 图片裁切、网格分割及字幕输入；改用私有媒体与正式保存合同                                              |
| `image-mask.ts`、`image-mask-dialog.tsx`、`mask-draft.ts`                                                                             | `canvas-node-mask-edit-dialog.tsx` 的绘制、擦除和透明遮罩；改用正式素材上传、模型用途校验及画布草稿   |
| `video-crop-geometry.ts`、`video-tools.ts`、`video-tools-dialog.tsx`                                                                  | `lib/canvas/video-crop-geometry.ts` 和视频工具的区域调整、截帧及裁切输入；改用正式素材与时间轴导出    |
| `director/scene.ts`、`ground.ts`、`stage-transform.ts`、`aspect-ratio.ts`、`view-modes.ts`、`camera-binding.ts`、`prompt-compiler.ts` | 3D 场景、地面、变换、画幅、视图、机位绑定和提示词编译；接入闭合输入与正式资源引用                     |
| `director/animation-semantics.ts`                                                                                                     | `director-animation-semantics.ts` 的动画增量与关键帧语义；保持自身类型和有限数值边界                  |
| `director/rig.ts`                                                                                                                     | `director-viewport.tsx` 的骨骼命名匹配；改为实际 GLB 骨骼扫描和纯函数映射                             |
| `director/camera-presets.ts`、`modes.ts`、`camera-moves.ts`                                                                           | `director-camera-presets.ts`、导演模式和运镜预设；接入自身场景与机位合同                              |
| `director/templates.ts`                                                                                                               | `lib/canvas/director/director-templates.ts` 的五种场景布局；改用自身 UUID、场景字段和正式新增节点合同 |

前端包依赖的许可仍由各依赖声明。

## Noto CJK 字幕字体

Go 时间轴渲染器使用原字节的 `NotoSansCJKsc-Regular.otf` 绘制中英文字幕，来源为 [notofonts/noto-cjk](https://github.com/notofonts/noto-cjk/tree/f8d157532fbfaeda587e826d4cd5b21a49186f7c/Sans/OTF/SimplifiedChinese)，固定提交 `f8d157532fbfaeda587e826d4cd5b21a49186f7c`，未修改字体。目标路径 `backend/internal/mediatool/adapter/ffmpeg/fonts/NotoSansCJKsc-Regular.otf`；文件 16,437,364 字节，SHA256 `2c76254f6fc379fddfce0a7e84fb5385bb135d3e399294f6eeb6680d0365b74b`。

适用 SIL Open Font License 1.1，上游 `Sans/LICENSE` 原文保存于字体旁 `OFL.txt`、[docs/licenses/noto-cjk-ofl.txt](/licenses/noto-cjk-ofl.txt) 和运行时 `/licenses/noto-cjk-ofl.txt`。不把字体替换成浏览器页面字体，也不将其作为 Glowloom 自有字体发布。

## Khronos glTF 输入校验规范

服务端 `backend/internal/media/adapter/gltf/schema/` 的 JSON Schema 原文来自 [KhronosGroup/glTF](https://github.com/KhronosGroup/glTF/tree/5decc120c95764c319c4f92e7f7ead026d926ef3)，固定提交 `5decc120c95764c319c4f92e7f7ead026d926ef3`。JSON Schema 保持原字节；适配和资源预算校验为 Glowloom 自身 Go 实现。

- Copyright 2014–2021 The Khronos Group Inc.，源 `COPYING.adoc` 与 `LICENSE.adoc` 原文同目录保留，逐项来源见 `schema/PROVENANCE.md`。
- 核心 Schema 使用 CC-BY-4.0；KHR 扩展规范遵循上游 Khronos Specification Copyright 原文复制条件。原文分别保存在 [CC-BY-4.0](/licenses/CC-BY-4.0.txt) 与 [Khronos Specification Copyright](/licenses/LicenseRef-KhronosSpecCopyright.txt)，运行时 `/licenses` 提供相同文本。
- 本项目没有修改被复制的规范、附加规范认证或厂商背书；未引入需独立解码器或未核验许可的扩展 Schema。

## infinite-canvas 来源

画布界面参考来源包含 basketikun/infinite-canvas 的 MIT 版本 `dab19adc0847e32e39b7fc8ff90cb392561fb826`。许可文本原字节保存在 [docs/licenses/infinite-canvas.txt](/licenses/infinite-canvas.txt)。

## R6 视觉参考素材（2026-10-13）

`public/design/` 中的 MANGO、CLI、Seedance、片场、蓝蝶、光影、幻想短片、香氛及网格来自用户指定 Figma 文件 `uLqmeRWuQuzfrp9xOYg6FT` 的设计资源。首页节点 `271:3620`、图片节点 `271:3961` 至 `271:3964`、`271:3983` 至 `271:3985`，登录节点 `9:2`，网格节点 `324:5507`。图片保留下载原字节，文件名末尾记录 SHA256 前八位；不将整页设计截图作为界面实现。

这些参考作品来自即梦公开展示，权利属于原作者及相关权利人；本记录不授予商业使用权。当前用于设计还原与本地审阅，发布前需要核对素材使用权或换成自有素材。浮光不将其声明为用户实际生成的私人资产。

R6 画布重建保留上节登记的指针、坐标与节点交互算法，改为左侧竖向工具栏、底部视口控制与 R6 颜色/圆角；历史来源和许可仍保留。

## Glowloom 无限画布核心增量

固定参考：[BeefTV `ddaf5f9abc64b16567c28ea5399d8f2069c47832`](https://github.com/glanderness/BeefTV/tree/ddaf5f9abc64b16567c28ea5399d8f2069c47832)，MIT；完整许可及传递来源继续随应用分发于 `/licenses/beeftv.txt` 和 `/licenses/beeftv-notice.txt`。目标位于 `frontend/src/app/canvas/`。

| 目标                       | 固定来源（相对上游 `web/src/`）                                                                    | 改写                                                                          |
| -------------------------- | -------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------- |
| `viewport/render-model.ts` | `lib/canvas/canvas-spatial-index.ts`、`canvas-frame.ts`，`pages/canvas/use-canvas-render-model.ts` | 世界坐标索引、超大元素独立查询、组后代与可见边；不截断节点数量                |
| `node/layer.tsx`           | `pages/canvas/use-canvas-render-model.ts` 的进入/保留机制                                          | 独立订阅现有视口/拖动预览、焦点保留及SVG标注线；沿项目媒体和节点组件          |
| `history/history.ts`       | `pages/canvas/use-canvas-history.ts` 的操作边界与有界历史机制                                      | 本地不可变图引用、合法命令检查、保存后限制及引用编号保护；不迁入上游持久store |

编辑器新增文字、分组、标注线及相机保存沿现有生成API和Go合同实现；未引入上游Wails、SQLite、供应商调用或远程插件运行时。来源说明不代表上游全部功能、容量性能或真实业务验收完成。
