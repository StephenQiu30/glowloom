# 第三方代码声明

## 历史 BeefTV 无限画布

2026-10-01 按用户要求清空全部前端实现。以下记录清理前从 [glanderness/BeefTV](https://github.com/glanderness/BeefTV) 移植并适配的历史代码来源；对应代码已从当前工作区移除，保留许可文本与来源记录。

- 固定来源：`0d9e9f48d407570cd431ad9730cdd522b06810c0`，用户确认采用该仓库当前 `main` 后固定此提交。
- 许可：MIT，完整版权与许可文本原字节迁至 [docs/licenses/beeftv.txt](docs/licenses/beeftv.txt)，原产品 `/licenses` 入口已移除。
- 版权：2026 @beefnoode and BeefTV contributors；2026 basketikun；2026 ddcat。
- 上游来源：BeefTV 的 `NOTICE` 声明其包含 Infinite Canvas v0.5.0（`568f0f1838df8de31fe885a4e130e2f346dd14ab`）的派生代码，并记录上游在 `890ba95858bbb13496d23978003716656109abb2` 改为 MIT 许可。

下表的源路径均相对于固定 BeefTV 提交的 `web/src/`；历史目标均相对于已删除的 Lanverse `frontend/src/components/canvas/`。

| 目标文件                             | 源文件                                                                                                                                  | 适配范围                                                                       |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `engine/infinite-canvas.tsx`         | `components/canvas/infinite-canvas.tsx`                                                                                                 | 平移、滚轮与双指缩放、背景层；接入正式类型和主题，缩放上限按 Lanverse 契约调整 |
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

`model.ts`、`document.ts`、`store.ts`、`queries.ts`、`workspace.tsx`、`editor.tsx` 与节点内容曾负责 Lanverse 的正式领域适配：身份与权限、项目资源、服务端 revision、原子命令、持久幂等和媒体预览。历史移植范围见 [无限画布引入设计](docs/design/BeefTV能力引入设计.md) 和对应 Plan；当前状态以 [前端实现清理](docs/design/前端实现清理设计.md) 为准。

BeefTV 的 Wails 桌面外壳、本地数据库、模型执行、插件、时间线、3D/深度工具及其资源未纳入本次移植。前端包依赖的许可仍由各依赖声明。

## 历史界面参考

旧画布曾参考 basketikun/infinite-canvas 的 MIT 版本 `dab19adc0847e32e39b7fc8ff90cb392561fb826`。这些前端实现已移除，其历史许可文本原字节迁至 [docs/licenses/infinite-canvas.txt](docs/licenses/infinite-canvas.txt)。
