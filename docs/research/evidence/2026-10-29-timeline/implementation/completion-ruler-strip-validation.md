# 时间线标尺与素材帧条补齐证据

日期：2026-10-29，Asia/Shanghai。仅本地工作树，未提交、未推送。

## 可证事实与边界

父任务实机观察全屏编辑器：14 / 21.4 px/s 时主刻度 5 秒，180 px/s 时主刻度 1 秒；4 秒 16:9 视频在约 718 px 轨道内呈现重复帧，轨高 56 px、单帧约 99.56 px。因 Mac 锁屏，本轮新增行为未完成浏览器动态复验。单测不构成 CORS、真实解码或像素验收。

按已观察到的公开脚本资源及其显式 lazy import 依赖读取，未复制源码实现到项目：

- [时间线节点 chunk](https://lf3-lv-buz.vlabstatic.com/obj/image-lvweb-buz/ies/lvweb/octo_web/static/js/async/async-canvas-node-timeline.860c26ba2d.js)：SHA256 `3f6f77c3c79a5c52ed5e9c17c2e02c95465c4f9c2d032d3ac9f62fba35691752`；模块 71873 将节点刻度密度除以 1.5。
- [共享时间线 chunk](https://lf3-lv-buz.vlabstatic.com/obj/image-lvweb-buz/ies/lvweb/octo_web/static/js/async/5547.45dddcadf7.js)：SHA256 `6c1b6a08d0f9430952c98f09a960327e4ee9966e993179bce684c718cd43b444`；模块 65536 的主刻度以 1/2/5 十进制序列满足至少 48 px，次刻度以 0.1 秒起步满足至少 12 px，并整除主刻度。模块 21798 的图片轨道同样按素材比例铺帧，卡片不是这个渲染路径。
- [全屏时间线 chunk](https://lf3-lv-buz.vlabstatic.com/obj/image-lvweb-buz/ies/lvweb/octo_web/static/js/async/3801.8c6735b2aa.js)：SHA256 `b59f8ea6ddedd1c64aabf0fcc07e95e056edf868d3b2eb8c7cf68af616fefb9a`；全屏路径直接传 pixelsPerSecond 给模块 65536 的刻度函数，不应用节点 1.5 因子。

全屏当前 14–180 px/s 范围的主刻度：小于 24 为 5 秒，24 至小于 48 为 2 秒，48 及以上为 1 秒。次刻度阈值为 24 / 60 / 120，分别切换到 0.5 / 0.2 / 0.1 秒。50、100 的阈值依据是静态代码，不是本轮实机观测。

早期三份 canvas bundle 未包含几何实现，REA 静态分析为部分覆盖，不能单独证实这些阈值。分析证据 ID `ev_b084a3458b905bc0a06b58fb85491b0a26a65b2d905cb53907bc30c6f14cea3c`，摘要 ID `ev_26e8b9854dba12bc8061e8931f071d0ff021cd0533d93d1744edcd3f9e95bdc1`；后续才由以上精确依赖补齐。

## 落地

- `timeline-ruler.ts`：用独立的秒单位间隔表实现已确认的间距合同；全屏与节点密度独立计算。
- `canvas-timeline-workspace.tsx`：主标尺标签与次刻度 CSS 间距随缩放更新；仅轨道启用素材帧条。
- `timeline-video-frame.ts`：按授权 URL + 素材入点共享一次截帧；最多 2 个并发视频解码、64 个挂载缓存项，最长边 256 px，单帧最多 512 KiB；15 秒超时。最后消费者释放时中止解码、卸载源并释放 blob URL；失败项可重试。无持久化缓存、无新依赖。
- `timeline-thumbnail.tsx`：视频单帧用 CSS repeat-x 铺满，保留素材比例，不按片段宽度生成多个 video；图片直接按其固有比例重复。资产卡片仍保留原预览。CORS / 解码 / 容量失败退回普通 video 单帧，因此失败态不保证重复条的视觉一致性。
- 未声称实现源站完整的多时点解码采样算法；当前实现对应本次实测的重复帧表现。真实 CDN CORS 和长片段情况下的匹配程度仍需动态复验。

## 检查

最终 `completion-final-core-tests.log`：12 文件 / 180 测试通过（12:44:51）。限定 Git diff whitespace 检查通过。

- `completion-ruler-strip-tests.log`：时间线目录 + Yjs 全组合，12 文件 179 测试通过（12:43）。随后新增真实 Radix 缩放操作集成测试；`completion-ruler-integration-tests.log` 为 18 / 18，通过 End / Home 检查主标签与次刻度间距。
- `completion-ruler-strip-eslint.log`：限定实现与测试文件 ESLint 通过。
- `completion-ruler-strip-tsc.log`：全量 TypeScript 仍被并行文件 `generation/agent-workspace.tsx:159` 的 TS18047 阻塞；未修改该文件。时间线文件无诊断。
- 5 个视频帧测试覆盖共享、入点变更、StrictMode、CORS 拒绝回退、并发上限、缓存容量、取消释放和失败重试。10 个缩略图测试含图片窄/宽容器 CSS 合同。DOM 单测不测真实布局尺寸。

待解锁逐项复验：视频授权 URL 跨域截帧、首帧与裁剪入点、长视频轨道宽度、图片窄/宽铺帧、14/24/48/60/100/120/180 缩放刻度、节点模式刻度。
