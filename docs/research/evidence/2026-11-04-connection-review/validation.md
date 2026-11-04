# 拖动、缩放与生成连线复核

2026-11-04。上一轮限定15文件提交ca56fa06597f06b353b5a1f85e23de3a61243962并推送main，提交前20项通过。CI41736921522前端成功、后端失败：TestPostgresCheckpointerSaveAndResume缺agent.graph_checkpoint表（SQLSTATE42P01）；父提交182e71f5的CI41664451809已有同一失败。

## 当前源证据更正

即梦独立研究画布52298cdd-795e-4f5a-af85-47eb2fb84b6e，9节点8边。实页reference-edge-interaction：endpoint-active基线rgb(0,142,229)/opacity1，default前景色opacity0.16。节点拖动期间flow host hidden=true，释放后恢复。测试移动已撤销或回到原位，源视图恢复72%；未增删源端节点或发起任务。

当前页面官方资源canvas-view.aa2f82f8c1.js、canvas-data.9e12112f7d.js、canvas-core.3d8e523609.js和octo-canvas-rendering.e9d0d93995.js的哈希见source-files.json。源码暂停条件为viewportMoving、节点拖动或框选；hover驻留200ms启动流光；滚轮会话在150ms空闲后的下一帧结束。真实生成业务状态需另验。

当前renderer确认2600ms周期、S=clamp(.55L,120,312)、72单位空窗、核心(1+sin(pi*q))/2、暗色光晕alpha .22/sigma1.5。精确渐变百分比为0/13.49/27.27/41.04/52.14/60.37/67.56/71.44/75.29/80/85.28/95/100，本地已补精度。

上轮“关联节点统一降灰”是错误临时判断，本轮恢复源端endpoint-active反馈，保留路径同步修复；旧截图不代表当前一致性证据。

## 本地验证

- 实页拖动剧本：流光running→suspended，基线d随节点位置更新；回原位松手后running。
- 50%/100%/200%矩阵与世界线宽为.5×2、1×1、2×.5，均一屏幕像素；覆盖层相对viewport偏移[0,0]、scale1。源缩放动画中相乘也约1，见measurements.json。截图local-50/100/200.png及local-final.png。
- 6文件24项通过，覆盖关联反馈、hover200ms、暂停/恢复不重建context、取消复原、150ms+RAF的滚轮清理、三档线宽及几何/吸附。全局tsc与范围lint通过。首次build缺公开配置GLOWLOOM_BACKEND_ORIGIN而失败；指定本地origin后最终build通过，原失败保留。

## 真实生成边界

用户授权一次任务最多5积分，随后明确允许仅补足这次2积分预算。文本选择器无可用模型；图片可用Seedream5.0Flash。现有图片草稿保留原prompt及引用。初始报价2积分，项目可用预算0，确认按钮disabled；此阻塞的历史截图保留，不作为最终状态。

使用现有ChangeBudgetCommand及真实会话/范围/项目权限检查，将项目可用预算从0精确补足到120000微元（¥0.12，报价展示2积分）；没有新建预算API或直接UPDATE预算。revision1→2，账本budget_change与审计各1条，request_id为`a8925cab-b424-410c-abfd-b773ea12fd4b`。原报价取消后重新核价，仍为2积分，确认按钮可用，见quote-authorized.png。临时操作程序已删除，不进入产品代码。

只点击一次确认生成。正式任务`0d7fd96a-2ee4-49be-9147-e1eec908a378`，2026-11-04T00:57:31.977482Z确认，00:57:32.553739Z进入submitting，00:57:33.507522Z失败，错误码provider_not_submitted。页面明确显示生成失败，见generation-submitted.png。数据库保留1条submit调用记录，outcome=error、dispatch_started_at非空、无HTTP状态与供应商接收回执；仅有安全state摘要，不能据此断言具体供应商错误或已进入供应商生成。预算账本reserve120000、settle0、release120000，最终reserved0、settled0、available120000，revision4。实际扣费0，未发起第二次任务，见generation-budget-receipt.json。

确认后采到流光running，但图片节点同时选中，不能区分选中态与真实生成态；供应商未接受任务，因此生成中、生成完成及其拖动/缩放组合仍未验收。失败后关闭弹层并取消选中，原4节点2边、已保存、100%；两条基线前景色opacity0.16，flow=suspended、覆盖层偏移[0,0]，无持续高亮残留，见generation-failed-idle.png。

组件generating回归不能代替真实供应商运行；不能签署全量一致。后续代码与文档仍是工作区增量，未再次提交。

刷新后原4节点2边保持且显示已保存；重新选择图片草稿自动恢复原失败任务，仍显示provider_not_submitted、原模型及输入，没有重新确认或重试，见generation-failed-reopened.png/.txt。此读回证明本次原提交可恢复，不证明供应商成功或所有异常保存场景。

## Figma 同步

[原动效画板3159:54491](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/Glowloom?node-id=3159-54491)保留ID，更新副标题3159:54493、合同说明3159:54598，修复说明原高度1的裁切；画板2392×1050，说明2300×400。已有78个流段矢量按新13节点精确百分比在各段中点重新取色，保持原几何与关键帧。24TEXT、20FRAME、86VECTOR、0图片填充，均为原生可编辑结构。截图figma-motion-contract.png已目检，无文字裁切。说明明确区分源端激活反馈、拖动/缩放通过与真实生成失败，不以静态稿证明GPU或全量一致。

## Git 范围

本轮未再提交。结束核对时main及远端均为4b1909f9d17e444068fd1d5d28f56f0343a83d9d（并行billing任务提交），ca56fa06仍为其直接父提交。本任务未改并行探索/顶栏文件；遗留路径清单见workspace-status.txt。
