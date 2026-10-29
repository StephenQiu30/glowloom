# 时间线批量导入验证（2026-10-29）

## 组件合同

新增`TimelineUploadDialog`，仅供时间线使用；共享媒体`UploadDialog`、`UploadDropzone`未修改。

- Props：`projectID`、可选`initialFiles: readonly File[]`、`onUploaded(asset, {signal})`、`onClose()`、可选`disabled`和`isCurrent()`。
- `initialFiles`只在组件会话初始化时读取；重新渲染传入新数组不会清空已有结果。`projectID`改变会新建会话。父组件负责用节点/epoch作为key或`isCurrent`守卫，回调必须检查signal/epoch并把成功素材写入素材架或指定轨道。
- 原样接收选择/拖入的全部文件；不支持格式保留在逐文件结果中，不静默过滤。每个会话最多50项，51项整批拒绝，已有成功结果保留。
- 复用现有`validateUpload`大小限制和`uploadMediaAsset`，限定图片、视频、音频；每文件验证项目ID、资产ID和媒体种类后才调用归档回调。
- 每文件固定幂等键，未知结果只用原键重试；收到合法上传回执后缓存asset，归档失败只重试回调，不重新上传。
- 同时最多两个上传/归档任务；逐项展示等待、进度、服务确认、归档、就绪和失败状态。成功项保留，失败单独重试。
- 关闭/卸载中止请求；只读/过时守卫阻止未开始队列与归档回调。父回调拿到AbortSignal，用于自身异步归档的最后写入检查。

## 与即梦来源的边界

50项批次和逐文件结果用于补齐已确认的时间线导入需求。保留本项目已有的明确确认：“我有使用权限，且无待授权真人内容。”每次新选择批次只确认一次；失败重试沿用原批次确认，不自动勾选、不绕过确认。此确认属于本项目既有合同，与即梦的直接导入流程存在明确差异，不能称为完全相同的上传UI。

## 执行证据

新测试在组件不存在时先失败，随后实现并通过：

`pnpm exec vitest run tests/unit/components/canvas/timeline/timeline-upload.test.tsx`：12项通过。覆盖混合支持/不支持格式、50/51边界、并发上限和进度、原键重试、归档单独重试、卸载后迟到回执、只读/过时、关闭后队列停止、错误项目回执、图视频音频多选、新批次重新确认。

`pnpm exec eslint src/components/canvas/timeline/timeline-upload-dialog.tsx tests/unit/components/canvas/timeline/timeline-upload.test.tsx`：通过。

`pnpm exec tsc --noEmit`：被平行文件`src/components/generation/agent-workspace.tsx:159`的`current is possibly null`阻断；当次输出没有新上传组件类型错误。未改动该平行文件。

上述上传API与归档回调为测试替身，验证组件合同而非真实上传/入轨。Mac锁屏，未执行真实浏览器批量上传验收；不能把单元测试当成真实服务或源站逐像素验收。

## 工作区接入回归

本地上传菜单和拖入素材架/轨道均接入该组件。整批完成前不自动关闭；无效文件不经过MIME过滤丢失。归档写入检查document epoch、节点存在、writable、migration与当前scope，失效抛错供行内重试。20项workspace测试中新增2项覆盖整批多次回执、非法文件可见及权限/epoch失效不写入。整合时间线/Yjs共13文件194项通过（12:55）。首次测试选错drop目标、其次预期漏算既有素材导致失败，分别修正到真实资产面板和3项总数后通过；没有放松业务断言。

整合后范围ESLint通过；tsc仍仅上述并行空值错误，未修改无关模块。输出见`completion-batch-lint.log`与`completion-batch-tsc.log`。

## 解锁后的真实服务验收

在专用空时间线B通过原生多选选择已授权4秒视频、7秒WAV、图片；一批确认一次，实际观察两项等待服务确认、一项等待上传，最后共3项全部就绪。关闭与刷新后素材架仍保留3项、片段数0；点击素材才加入轨道。截图`completion-browser/local-batch-success.png`与`local-batch-refreshed.png`。本次真实成功路径通过；50/51及混合故障仍为组件测试证据，不冒充真实服务接受。

正式Chrome上限追加：filechooser传入授权合成纯色图51项，整批拒绝提示且开始禁用；50项渲染50行待上传，未确认仍禁用。关闭未上传，不新增资产。重复同一文件路径用于UI数量边界，不代表50个不同素材的服务完成验收。
