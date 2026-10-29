# 导演台第一轮实现验证（2026-10-29）

本页保留第一轮历史证据与当时的缺口；第二轮当前状态见[第二轮验证](validation-round2.md)和[更新后的35项矩阵](acceptance-matrix.md)。第一轮的3动作、缺简模/分屏等描述不是当前实现状态。

本记录区分正式页面、渲染夹具、业务契约、构建及Git交付。实现取舍与逐项边界见[来源映射](source-mapping.md)和[35项证据矩阵](acceptance-matrix.md)；工作进度及后续调度只在BACKLOG。

## Git与集成证据

- 调研/设计基线提交 `6e86a6509596b01580855dae0e7d554ac08510a5` 已推送main；[CI 40797614801](https://github.com/StephenQiu30/glowloom-server/actions/runs/40797614801)为completed/success。
- 基于此提交建立隔离工作树，复制共享工作区必要画布基础，在隔离目录开发；按保存的基线和文件清单三方合入主工作区。集成脚本冲突时先退出，未重置、覆盖或暂存其他任务文件。
- 产品实现仍是本地工作区变更。导演台依赖的画布/Yjs基础也含其他任务未提交修改，不能把当前整份共享文件无差别记为本任务提交。当前main已由其他任务继续推进；初始提交仍在远端main祖先链上。
- API经 `pnpm run api:generate` 生成，未手写generated typings。

## 正式页面操作证据

环境：现有Docker Compose，frontend1122/backend1123及协作服务；独立验收项目与画布，未操作其他任务的内容。测试画布ID为 `e2c90c40-8361-4501-ac25-c12a531d2c42`。

1. 创建导演台，打开首次帮助；添加标准男性角色，点击地面放置并保存。
2. 静态同步到动态，角色姿势恢复默认；绘制路径，预览后确认。保存并刷新后仍有32位置和32旋转关键帧。
3. 拍摄真实PNG，经项目媒体上传保存到镜头素材，选择添加到画布；刷新后保留图片节点。
4. 正式UI导出16:9、长边1280、30fps、1秒视频。浏览器预览读数1280×720、duration=1、readyState=4、error=null；检查画面后保存到项目媒体。
5. 关闭并重新启动浏览器、重开画布，镜头素材仍有图片和视频。媒体服务保存后的可播放文件解码为H264/MP4、1280×720、30fps、1秒；详见[ffprobe结果](formal-saved-video-probe.json)和[真实保存视频](formal-saved-video.mp4)。这是媒体服务转码后的结果，不将其误称为浏览器原始WebM编码。
6. 视频添加到画布并刷新，导演台、PNG、MP4共3节点，界面显示已保存；见[刷新截图](canvas-output-reopened.png)。节点媒体状态汇总仍显示“状态未核验”，不将该汇总当作就绪证据；本轮使用实际媒体读取与解码核验内容。
7. 动态角色中选择真实行走动作，在0.65秒切换到静态并同步，静态显示行走中间姿势：[动态帧](dynamic-walk-frame.png)、[冻结静态帧](static-frozen-frame.png)。体型腿长改为1.15，保存、刷新、重新打开仍读出1.15：[回读截图](body-profile-reopened.png)。
8. 自建无纹理OBJ金字塔通过本地文件选择转换为GLB，1网格/6三角面/0动画，经正式上传与场景保存可渲染：[模型导入](model-import-saved.png)。该证据不能推广为任意FBX/Mixamo文件兼容。
9. 立方体点击地面放置后中心Y=0.5，底面位于地面，修复原先把中心压到Y=0的问题：[落地截图](ground-placement.png)。
10. 真实拖动行走片段从0秒移动到2秒，左端再裁剪至2.333333秒，长度从约1.333333秒变为约1秒；将位置关键帧从4秒拖到6秒。刷新后片段位置11.6667%、宽度5%（20秒时间轴），6秒关键帧仍存在，旋转4秒关键帧未移动：[裁剪截图](timeline-trim-saved.png)。源入点字段实际采样/后端回读由专项测试另证。
11. 本地导入和项目资产流程随后统一为“上传/选择→点击放置→一次保存/History”；多选逐个放置、取消不插入剩余对象。该最后调整通过52项相关定向测试；正式上传渲染证据在调整前取得，调整后的完整导入→放置→撤销尚未在页面重新执行。

浏览器日志观测到并行时间线CSS修改引发的Turbopack HMR chunk错误，刷新后页面恢复。未把这次开发热更新错误计作导演台渲染成功证据；也未执行强制WebGL丢失与恢复的正式验收。

## 渲染夹具（独立于正式服务）

- 使用真实Three加载标准角色，PNG1920×1080、61,571字节；图像分析存在7,472个明亮像素，非空白导出。
- 冻结夹具为1对象、65骨骼、位置[0.5,0,0]、零动画关键帧。
- 逐帧视频夹具输出WebM/VP9、1280×720、24fps、0.5秒，ffprobe可解码。
- [夹具报告](engine-report.json)、[PNG](engine.png)、[WebM](engine.webm)。这只证明渲染和编码；正式服务保存另见上节。
- 临时public验证目录已移除，夹具源留在 `frontend/tests/browser/director-engine-fixture.ts`。

## 自动检查

- 导演台最终定向回归（含存储策略/API一致性）：18文件、123项全部通过，`pnpm exec vitest run tests/unit/components/canvas/director tests/unit/business-storage-policy.test.ts tests/unit/api-generation.test.ts --maxWorkers=2`。
- 类型检查：主工作区和隔离实现 `pnpm exec tsc --noEmit` 通过。
- 生产构建：隔离实现 `pnpm exec next build` 通过；当前应用正式页面另以原Compose前端验证。
- 定向ESLint和Prettier通过；API生成一致性 `pnpm run api:check` 通过。
- 前端全量 `pnpm exec vitest run`：220文件，1,449通过/7失败，共1,456测试。失败来自创作页3项UI断言、theme-policy的时间线独立色板、users/assistant-panel/node-resize共3项。后3项不改变超时配置单独复跑均通过；创作页3项仍失败。没有修改这些并行业务文件以迎合断言。
- Go导演台契约在独立Postgres临时库运行，包含真实保存/回读与受限角色访问；`go test -race ./tests/canvas -run TestDirector` 通过。未清空现有应用数据库。
- `go vet ./internal/canvas/... ./tests/canvas/...` 通过；隔离实现同范围golangci-lint在按项目local-prefixes整理imports后0 issues。最终主工作区同范围另报2项并行错误：`tests/canvas/project_copy_test.go:79` 重定义copy、`internal/canvas/domain/timeline.go:112` De Morgan建议，未擅改这些并行文件。
- govulncheck同范围0个可达漏洞；依赖模块中另有1项未被导入/调用的漏洞，工具未判断为当前代码受影响。

完整Compose重建曾在backend Debian apt下载失败，退出码100，未报告成功。随后只重建frontend成功，使用现有backend镜像重启并从挂载源码编译；三服务健康。该过程不是重新构建backend基础镜像的通过证据。

## 接受边界

当前证据不覆盖全部即梦一致性：私有742动作库和八类简模资源、文字生成动作服务、独立左右肢体参数、分屏/视角指示器/独立注视点树、相邻混合交互、跨刷新拒绝稿恢复及全部只读/冲突/窄屏/编码组合仍未完成产品接受。没有伪造资源、供应商生成或“35项全部通过”。
