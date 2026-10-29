# 时间线失败资源重试接线核验 · 2026-10-29

本轮只检查已有媒体失败、重试和生命周期合同，未操作浏览器、未改变导出或认证，也未执行全量类型检查。设计依据为设计31的7.4节。保留所有并行修改。

## 确证差异与修复

1. 授权读取hook已支持指定资产重试，但原界面的唯一重试按钮无参调用，会同时清空、重读健康媒体，并主动暂停整个预览。现按失败资产提供重试入口，只调用该资产ID，不改变整体播放状态。
2. video/audio/image元素失败原来只通过普通错误文字展示，无重试入口。现错误回调携带实际资源实例，由工作区按资产关联并提供相同重试入口。错误仅在实例仍为当前资源时显示，刷新URL、同URL新读取代次或身份变化产生的新实例不继承旧错误。

保留既有同URL重试代次重建媒体、六请求上限、单资源错误隔离、过期URL撤除、流式波形两任务上限和取消/读取停滞释放合同，没有引入第二套资源加载器。预览元素仍由React持有src，保留StrictMode修复，不以再次删除src的方式猜测解决内存问题。

## 自动检查

- 先写两个工作区回归，两项均失败：缺少指定失败资产的重试入口，见[红测](completion-media-retry-ui-red.log)。
- `pnpm exec vitest run tests/unit/components/canvas/timeline/timeline-media.test.tsx tests/unit/components/canvas/timeline/timeline-preview.test.tsx tests/unit/components/canvas/timeline/timeline-workspace.test.tsx`：3文件56项通过，26.07秒，见[绿测](completion-media-retry-ui-green.log)。
- 新增回归验证健康资源对象不变、播放仍进行、只重试失败ID、解码失败入口、同URL新实例不保留旧提示。既有预览隔离回归额外断言传回的是准确失败资源。
- 范围ESLint通过，见[输出](completion-media-retry-ui-eslint.log)。未运行全量tsc、生产构建或浏览器检查；这些测试不证明真实网络、原生解码器或长期堆内存行为。

## 真实复验动作

- TLA-34：同时播放视频与两条音轨，使其中一个授权读取或媒体解码实际失败；记录仍正常音轨的currentTime连续增长、健康媒体DOM未重建，点击失败行重试，仅该资产出现新的preview请求，恢复后可重新播放且错误消失。
- TLA-06：一批混合成功、失败、等待中的授权合成素材，失败后只重试失败项；在上传完成但归档未确认时中断，恢复时核对原幂等键、服务资产数量和只重试归档的请求，不能只看前端“已就绪”。
- TLA-48/49：真实只读权限下可播放/定位而编辑与导入不能写；上传或手势途中发生权限或版本变化后没有延迟提交。断线态不能替代只读账号。
- TLA-46：真实长压缩AAC/MP3与WAV分别量化网络/解码内存曲线，制造一个读取停滞任务并验证30秒后其槽释放、后续任务继续；7秒WAV成功不能替代长音频或编码矩阵。
- TLA-50：重复切换节点、删除资源、打开/关闭全屏，记录网络请求终止、活动媒体数量及堆/原生解码资源趋势；StrictMode组件回归不能替代长期泄漏量化。
- TLA-31/32：快速连续seek/播放/暂停及多轨长时播放，实际测量音频时钟漂移与尾端停止，不以mock currentTime断言代替音画同步。


## 紧凑/全屏实例恢复补充

父任务真实复验已观察到失败WAV重试后恢复，而健康video继续播放；返回紧凑节点仍显示旧失败。代码确认两个工作区各有资源hook，已把设计合同补为：从suspended恢复显示时只重试自身当前失败资产一次，保留健康资源，不无条件清空错误或连续自动重试。

新增真实Radix展开/关闭组件用例先红后绿，验证只重试失败ID一次、普通重渲染不重复重试、健康资源对象保留且无时间线提交。`pnpm exec vitest run tests/unit/components/canvas/timeline/timeline-workspace.test.tsx tests/unit/components/canvas/timeline/timeline-media.test.tsx`：2文件45项通过，3.11秒；[红测](completion-media-resume-red.log)、[绿测](completion-media-resume-green.log)、[范围Lint](completion-media-resume-eslint.log)。这条模式恢复修复尚需父任务在相同真实WAV故障场景返回紧凑后回读确认，不能以组件回归代替。
