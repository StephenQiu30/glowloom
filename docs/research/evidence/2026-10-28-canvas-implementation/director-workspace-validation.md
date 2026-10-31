# 导演台场景、拍摄与动作链验证

日期：2026-10-29。共享 dirty main，未提交。来源范围见 advanced-parity-contracts.md §4、既有 Figma 静动态截图；本次未重新取得源站拍摄/编码产物。

## 实现与身份

- `CanvasDirectorWorkspace({client,nodeID,canvasID,projectID,compact?,onExpand?,onClose?})` 接入既有 director/tool 节点。compact 只呈现节点说明、真实对象/机位/镜头数与进入按钮；全屏使用真实 Three WebGLRenderer + OrbitControls/TransformControls。
- 场景对象/机位/灯光/镜头/关键帧保存到既有 config.director。仅新增可选 mode/locked/ground.color 与 50/60fps，保持旧稿兼容。Yjs changeDirector 使用配置CAS、document epoch、只读和手势保护，拒绝的有效编辑保留于当前会话。
- 真实授权 GLB/图片加载，不写临时URL；模型外部资源加载禁止，素材错误公开显示。几何人偶为自建构图对象，不是源站人物模型。
- 机位预览与拍摄不包含网格、控制器、选框。PNG拍摄要求素材全部加载、正式保存接受；真实PNG预览解码后确认上传，校验回执身份，再绑定 shot.screenshots 并等待正式接受。重试复用 File/idempotency key，旧epoch/改动后的场景拒绝回写。镜头素材支持授权预览、封面、删除引用。
- GLB内置动画使用真正 AnimationMixer，支持动作起始/时长/速率/循环、重叠等权混合、前后定位；骨骼映射、静态四元数姿势与动态骨骼帧真实应用在 Bone 节点上。模型无动作显示空态，未伪造动作目录。

## 已执行验证

- Go新增模式/颜色/锁定/fps单元测试；既有全部 TestDirector 加新增正式数据库保存/新服务回读/错误输入整笔拒绝：隔离 PostgreSQL18 数据库 glowloom_canvas_director_tests，使用 glowloom_app 受限角色，`go test -race ./tests/canvas -run TestDirector -count=1` 通过（12.771s）。未使用本地业务库。
- Go scoped vet 通过，govulncheck domain 无漏洞，own goimports 清洁。此前全域 golangci-lint 发现并行 subject_local_test.go 格式项，已反馈所属agent，没有越权修改。
- Frontend场景模型、真实Three几何、Yjs并发/epoch/失权/拒绝保留、拍摄上传未知结果与身份保护、真实AnimationMixer/Bone采样单测通过。运行中的测试总数随并行Yjs用例增加，最终数字以本任务最后运行输出为准。
- 动作扩充后的 tsc/director scoped eslint 已通过；此前并行测试类型错误已解决。最新 director/timeline/Yjs/theme-policy 定向9文件107tests通过（包含后续工程导出用例）。
- root报告真实浏览器已建立WebGL、创建方体并保存positionX=2。随后根据实屏反馈修复Dialog individual translate与compact卡片。截图发布/刷新读回与真实人物动画的浏览器验收尚未由本文件作者确认，不能把测试fixture等同于验收。

## 仍需继续的来源范围

未完成：十二人物体型预设和FBX/Mixamo导入/重定向、独立静动态两场景同步、独立注视点/FOV/倾斜关键帧、十四运镜与绘制路径、分镜时间编排、完整视图/面板布局行为、真实动态视频编码及正式视频产物。模型动作核心与单帧拍摄完成不等于导演台全量一致。测试GLB/动画对象均为本地合成逻辑验证，不是第三方来源人物证据。
