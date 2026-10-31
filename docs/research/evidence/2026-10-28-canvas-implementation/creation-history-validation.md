# 创建、空生成器与个人撤销增量验证

2026-10-29，开发服务1122/1123，共享工作区增量。不是完整一致性签收。

## 来源和实际红例

即梦两文本后接图片产生一个目标与两条边，一次CmdZ回到5节点0边，见advanced-source/multi-connected-create.txt。目标多选相机+主体创建图片6，12→13节点，自动展开生成器显示连接主体1及35mm，见browser-multi-created.png。首次CmdZ只撤销自动创建空draft，权威拒绝后恢复面板显示空composer与不存在draft的差异，见browser-empty-boot-red.png；没有手工正文。

跨画布12节点复制接受后，服务端normalizeNodeReferenceLabels分配显示编号，历史全字段比较误认为协作者修改。红例及修复后的真实3次accepted、24→12→24→刷新24节点14边见cross-canvas-copy-validation.md。

## 修复合同

- history比较排除权威派生referenceLabels/labelState及mention显示token/label；真实正文/参数/媒体等仍保护。新增外部边或子成员保护，避免撤销删除协作者依赖。
- RichGeneration打开画布编辑器只waitForNode等待节点接受，不flush物化空输入；检查费用前才flush并等待正式input_revision。无draft的编辑基线等于空PromptDraft，不能与节点已删除混淆。
- 创建菜单关闭焦点归新节点，生成器不因旧trigger返焦点消失；多源一次构造一个目标及全部边，无部分写入。

## 可执行证据

- /tmp/canvas-history-label-red.log：新节点接受分配显示编号后undo失败。
- /tmp/canvas-history-label-green-2.log：56 session tests全部通过，含协作者内容保护和新增外部边拒绝。
- /tmp/canvas-boot-no-write-red.log：旧boot调用flush违背打开无编辑合同。
- /tmp/canvas-boot-no-write-green.log：3文件92测试通过。
- /tmp/canvas-history-lint.log 定向ESLint通过；boot改动后的全量检查另列最终回执。

## 未完成的运行证据

浏览器后续复验被验收页确认窗口及控制超时阻塞；未把空草稿修复的单元绿色标为实浏览器通过。新建多源节点→一次撤销→重做→刷新仍需重跑。项目费用800×568已发现并调整gap，800×546需重新测量。真实供应商与非空媒体上传等待既有确认条件。
