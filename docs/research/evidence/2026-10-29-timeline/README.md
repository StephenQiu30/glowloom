# 时间线调查证据索引

日期：2026-10-29。页面与发布文件身份见 [source-identity.json](source-identity.json)。用户允许上传三个合成样本；在新建“时间线核验-20261029”内操作，原“时间线 1”保留。

## 运行证据

以下来自 Chrome 插件，不是 REA CDP capture。截图与 DOM 共同表达当时状态；口头总结不是独立运行证据。

| 文件 | 对应问题 |
| --- | --- |
| [fullscreen-empty-dom.txt](fullscreen-empty-dom.txt)、[空态截图](jimeng-fullscreen-empty.jpg) | 资产区、预览区、工具栏、空轨道、禁用态 |
| [imported-ready-dom.txt](imported-ready-dom.txt) | 导入完成的素材卡片，与实际加入轨道区分 |
| [mixed-tracks-dom.txt](mixed-tracks-dom.txt)、[混合轨道截图](jimeng-mixed-tracks.jpg) | 4 秒视频、5 秒图片、7 秒独立音频，总长 9 秒 |
| [trim-left-dom.txt](trim-left-dom.txt) | 视频在 1 秒左剪：3 秒视频+5 秒图片，总长 8 秒 |
| [trim-right-dom.txt](trim-right-dom.txt) | 右剪：1 秒视频+5 秒图片，7 秒音频决定总长 |
| [playback-end-dom.txt](playback-end-dom.txt) | 到 9 秒末尾后停止，播放按钮恢复 |
| [aspect-options-dom.txt](aspect-options-dom.txt)、[竖屏截图](jimeng-portrait-preview.jpg) | 比例集合；9:16 contain 留黑边 |
| [export-settings-dom.txt](export-settings-dom.txt)、[设置截图](jimeng-export-settings.jpg) | 文件名、默认交付位置、默认 720P |
| [export-more-dom.txt](export-more-dom.txt) | XML、素材、剪映、DaVinci、Premiere、Final Cut 菜单 |
| [export-pending-dom.txt](export-pending-dom.txt)、[进行中截图](jimeng-export-pending.jpg) | 尚未取得文件的下载进行中及取消入口，不能作为导出成功证据 |
| [node-after-edit-dom.txt](node-after-edit-dom.txt)、[节点截图](jimeng-node-after-edit.jpg) | 全屏退出后节点保留 9 秒与 4 片段摘要 |
| [reopened-dom.txt](reopened-dom.txt)、[重开截图](jimeng-reopened.jpg) | 刷新后结构、素材与 9:16 保留；历史重置 |

音频 split/undo/redo、鼠标及 Shift+方向键换序等过程由本次工具调用记录提供；保存的快照只覆盖部分中间状态。没有伪造录屏或把单帧截图当作整个手势的证据。

## 静态证据

- [locale-candidates.json](locale-candidates.json)：135 条时间线相关发布文案。这里只是存在性与术语线索，不证明触发条件或可执行性。
- `output/timeline-research-20261029/jimeng-shipped/`：两个页面实际引用的 JS 文件，未执行代码。
- `output/timeline-research-20261029/jimeng-timeline-tokens.json`：从 canvas-view 中提取的候选 token（含导演台键），属性上下文需结合原文件检查，不能将所有键用于本功能。
- `output/timeline-research-20261029/rea-evidence-complete.json`：完整 REA session ledger；文件级分析范围与语义截断限制见研究正文。

## 本轮边界

没有取得成功视频/素材包/XML 文件，没有外部剪辑软件导入结果，没有多账号协作、故障注入或移动触摸验收。测试节点和上传素材保留以便后续继续核验，未消耗 AI 生成功能。没有审查或修改 Glowloom 功能实现。
