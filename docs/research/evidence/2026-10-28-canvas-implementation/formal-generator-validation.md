# 正式画布生成入口接线与浏览器证据

采样：2026-10-28晚至2026-10-29凌晨，共享main未提交候选。仅记录已发生检查，不宣称全量一致或真实供应商生成接受。

## 输入与结果合同

正式 YjsCanvasWorkspace 通过 renderGenerationPanel 挂载 CanvasGenerationPanel，复用 RichGeneration 的 canvas_input 所有者、真实目录/报价/任务接口。四类资源节点保留类型/动作/ID；提示词在 config.draft.composer，正文和媒体结果分别保持原有字段。输入必须等待权威保存接受与DTO版本对齐，才提供 canvas_revision/input_revision；IME、上传、UNKNOWN、epoch切换、节点失权/删除均不得生成。

客户端详情及79项回归见[generation-input-validation](generation-input-validation.md)。Go四类节点真实PostgreSQL/Yjs版本、授权及事务回滚见[resource-input-validation](resource-input-validation.md)。

文字结果采用重新读取任务并校验项目、完成状态、结果ID及审核状态；媒体采用重新读取固定版本资产身份及授权预览。异步检查期间若节点结果已被编辑，不覆盖当前正文/媒体。采用动作等待画布持久确认，失败保留原任务结果可重试。

## 自动检查

- service-graph 14项：包含输入投影在刷新重开后保留，并且不覆盖资源正文；发现并修正了draft投影嵌套一层导致composer丢失的问题。
- generation-dialog 30项：新增文本采用等待、错误反馈与保留结果重试；既有模型/报价/任务回归通过。
- CanvasGenerationPanel 4项：同节点采用、迟到结果拒绝覆盖、跨项目/审核状态拒绝、媒体固定版本校验。
- 上述为真实组件/状态转换测试与模拟API返回，不证明供应商调用。

## 正式浏览器

使用既有专用画布：project 7fe2cb76-b42b-4fd9-abd4-e70a93173e87，canvas 24969728-69d3-4326-b360-c591b018f42d。通过 http://localhost:1122/canvas 正式入口操作。

1. 打开“文本1”，正式生成器加载。
2. 输入“写一段关于雨后城市灯光的简短描述。”，可见状态由等待保存确认变为已保存。
3. 刷新页面，再打开同一节点，生成提示词完整回读；原节点正文仍为“画布文本验收 / 背景、全屏编辑和 Markdown 下载。”。
4. 打开文本模型目录，实际显示“暂无匹配的可用模型”。未选择虚构模型、未提交报价确认、未调用供应商、未产生付费任务。
5. [正式生成器截图](browser-formal-generator.png)显示提示词与原正文分离。当前模型/参数控件布局与即梦文本默认生成器仍有差异，不将接线等同视觉一致。

## 运行依赖与失败修复

首次进入文本全屏触发 UNREGISTERED_TEXT_ATTRIBUTE。实查 frontend/node_modules 的 file:canvas-collab 注入副本仍为旧schema，缺少TextAlign；宿主源码与协作服务已更新不代表该副本更新。同步 frontend lock 中 file包新增依赖、frozen install及重启前端后，正式全屏打开成功，三向对齐菜单正常；居中对齐后可见两个正文段落computed text-align=center，画布返回已保存。不能将之前的失败省略为一次通过。

## 未接受范围

当前项目文本模型目录为空，真实供应商文本/图片/视频/音频完整报价→执行→结果→采用仍待实际模型和额度条件。普通生成器接线不代表导演预设、主体节点、时间线全部格式、AI会话、公开分享或60项需求已完成。完整差异见[全量矩阵](full-parity-matrix.md)。
