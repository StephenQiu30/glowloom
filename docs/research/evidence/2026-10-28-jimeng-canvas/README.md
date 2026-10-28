# 即梦无限画布证据索引

日期：2026-10-28，Asia/Shanghai。目标为用户指定的Chrome即梦画布「一致性调研-202610221」。

来源报告：[即梦无限画布需求基线](../../10-即梦无限画布需求基线.md)。本目录不包含Glowloom源码分析、REA捕获、网络记录或真实生成结果。

## 采证说明

- `E02…E50`是本地编号，不是REA Evidence ID。U01为会话中用户原始截图与AX，不在此伪造原文件。
- AX通过`cua_repl`连接既有Chrome标签采集；完整快照使用`getAXState({disableDiffing:true})`。
- 早期二次读取返回增量“无变化”，未形成完整快照；这些条目保留首次工具返回内容的人工整理摘录，均在文件头明确标记。不能当作原始DOM、源码或网络证明。
- PNG来自实际浏览器截图，未做图像编辑；截图尺寸由当时浏览器视口决定，不等同于用户提供原图尺寸。
- 文本证据省略目标画布URL，保留域名与目标说明；没有保存分享访问链接或凭据。
- 本次临时创建6个节点及1条边，逐次撤销后恢复5节点、0边、60%缩放与文本3单选；积分66。平移像素是否精确恢复未验证；撤销/重做历史因调研发生变化。
- 核心证据：E31连接创建、E35连接撤销、E42富文本、E46多选、E48分镜组限制、E50最终状态。

## 文本证据

| 编号 | 内容                                              | 类型                 |
| ---- | ------------------------------------------------- | -------------------- |
| E02  | [zoom](02-zoom.txt)                               | 完整AX快照           |
| E03  | [text-reference](03-text-reference.txt)           | 观察摘录（非AX原文） |
| E04  | [connected-node](04-connected-node.txt)           | 观察摘录（非AX原文） |
| E05  | [text-background](05-text-background.txt)         | 观察摘录（非AX原文） |
| E06  | [video-create](06-video-create.txt)               | 完整AX快照           |
| E07  | [video-models](07-video-models.txt)               | 观察摘录（非AX原文） |
| E08  | [video-size](08-video-size.txt)                   | 观察摘录（非AX原文） |
| E09  | [video-modes](09-video-modes.txt)                 | 观察摘录（非AX原文） |
| E10  | [video-duration](10-video-duration.txt)           | 观察摘录（非AX原文） |
| E11  | [image-create](11-image-create.txt)               | 完整AX快照           |
| E12  | [image-size](12-image-size.txt)                   | 完整AX快照           |
| E13  | [image-models](13-image-models.txt)               | 观察摘录（非AX原文） |
| E14  | [camera-kit](14-camera-kit.txt)                   | 观察摘录（非AX原文） |
| E15  | [audio-create](15-audio-create.txt)               | 完整AX快照           |
| E16  | [audio-modes](16-audio-modes.txt)                 | 观察摘录（非AX原文） |
| E17  | [audio-types](17-audio-types.txt)                 | 观察摘录（非AX原文） |
| E18  | [timeline](18-timeline.txt)                       | 观察摘录（非AX原文） |
| E19  | [timeline-sources](19-timeline-sources.txt)       | 完整AX快照           |
| E20  | [subject](20-subject.txt)                         | 观察摘录（非AX原文） |
| E21  | [subject-import](21-subject-import.txt)           | 观察摘录（非AX原文） |
| E22  | [assets-filter](22-assets-filter.txt)             | 观察摘录（非AX原文） |
| E23  | [search](23-search.txt)                           | 观察摘录（非AX原文） |
| E24  | [history](24-history.txt)                         | 观察摘录（非AX原文） |
| E25  | [share](25-share.txt)                             | 观察摘录（非AX原文） |
| E26  | [ai-chat](26-ai-chat.txt)                         | 观察摘录（非AX原文） |
| E27  | [node-summary](27-node-summary.txt)               | 观察摘录（非AX原文） |
| E28  | [node-context-menu](28-node-context-menu.txt)     | 观察摘录（非AX原文） |
| E29  | [canvas-context-menu](29-canvas-context-menu.txt) | 观察摘录（非AX原文） |
| E30  | [subject-connect](30-subject-connect.txt)         | 观察摘录（非AX原文） |
| E31  | [subject-image-edge](31-subject-image-edge.txt)   | 观察摘录（非AX原文） |
| E32  | [expanded-generator](32-expanded-generator.txt)   | 观察摘录（非AX原文） |
| E33  | [color-kit](33-color-kit.txt)                     | 完整AX快照           |
| E34  | [style-kit](34-style-kit.txt)                     | 观察摘录（非AX原文） |
| E35  | [undo-connected-node](35-undo-connected-node.txt) | 观察摘录（非AX原文） |
| E36  | [timeline-export](36-timeline-export.txt)         | 完整AX快照           |
| E37  | [undo-timeline](37-undo-timeline.txt)             | 完整AX快照           |
| E38  | [undo-audio](38-undo-audio.txt)                   | 完整AX快照           |
| E39  | [undo-image](39-undo-image.txt)                   | 完整AX快照           |
| E40  | [camera-motion](40-camera-motion.txt)             | 完整AX快照           |
| E41  | [restored-five-nodes](41-restored-five-nodes.txt) | 完整AX快照           |
| E42  | [text-edit](42-text-edit.txt)                     | 完整AX快照           |
| E43  | [text-style](43-text-style.txt)                   | 完整AX快照           |
| E44  | [tags](44-tags.txt)                               | 完整AX快照           |
| E45  | [multiselect](45-multiselect.txt)                 | 完整AX快照           |
| E46  | [shift-select](46-shift-select.txt)               | 完整AX快照           |
| E47  | [layout](47-layout.txt)                           | 完整AX快照           |
| E48  | [group](48-group.txt)                             | 完整AX快照           |
| E49  | [minimap](49-minimap.txt)                         | 完整AX快照           |
| E50  | [final-canvas](50-final-canvas.txt)               | 完整AX快照           |

## 截图

| 文件                                           | 用途                                 |
| ---------------------------------------------- | ------------------------------------ |
| [主体→图片参考连接](31-subject-image-edge.png) | 空主体连接及目标生成器               |
| [图片生成器展开](32-expanded-generator.png)    | 参考分类、模型参数、宽度关系         |
| [配色面板](33-color-kit.png)                   | 视觉选择器；稳定加载后的AX含配色名称 |
| [小地图](49-minimap.png)                       | 多选与视口辅助工具                   |
| [最终画布](50-final-canvas.png)                | 临时节点撤销后的内容与布局           |

## 覆盖限制

菜单可见不代表生成、导出或分享授权流程通过。完整缺口见报告第7节。`manifest.json`记录当前证据文件字节数及SHA-256；校验只保证文件一致性，不改变证据等级。
