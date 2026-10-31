# 时间线工程产物验证

日期：2026-10-29。共享 dirty main；未提交，未修改业务库数据。正式合同：设计27 §7.4.8、PRD14 AC-CJC-17 补充、Plan23 CASE-CJC-17 补充。

## 来源与格式边界

源端证据 `advanced-source/timeline-export-menu.txt` 仅证明菜单包含 MP4、批量素材、剪映、XML、DaVinci、Premiere、FinalCut，没有源工程字节，因此 source XML 类型与各软件实际封装仍待证。

第一手规范：

- [Apple FCPXML reference](https://developer.apple.com/documentation/professional-video-applications/fcpxml-reference)、[官方 DTD](https://developer.apple.com/documentation/professional-video-applications/document-type-definition)、[bundle reference](https://developer.apple.com/documentation/professional-video-applications/fcpxml-bundle-reference)、[media-rep](https://developer.apple.com/documentation/professional-video-applications/media-rep)。FCPXML 1.10 bundle 使用 `timeline.fcpxmld/Info.fcpxml`，引用 `./Media/...`。
- [Apple FCP7 XML reference](https://developer.apple.com/library/archive/documentation/AppleApplications/Reference/FinalCutPro_XML/Index/index_of_book.html) 用于独立 xmeml-v5 序列化；相对 pathurl 指向包内 Media。[Apple编码基础](https://developer.apple.com/library/archive/documentation/AppleApplications/Reference/FinalCutPro_XML/Basics/Basics.html)规定立体声音源分成两条音轨，当前按声道1/2输出并用linkclipref/groupindex绑定；xmeml暂拒绝超过2声道，避免静默丢失。未执行目标软件导入，UI明确可能需重新关联该目录。
- [Adobe 官方迁移说明](https://helpx.adobe.com/premiere/desktop/organize-media/import-files/migrate-from-final-cut-pro-x.html) 要求转换 FCPXML；不能把 FCPXML 或 xmeml 的解析成功当作 Premiere 项目兼容。

当前真实入口为“批量媒体包”“FCPXML 1.10 工程包”“FCP7 XML v5 交换包”。未增设伪剪映/DaVinci/Premiere工程按钮。剪映草稿、源端XML实际类型和各软件验收均未完成，JC-050不能据此标为全量完成。

## 实现

- 新工程 ZIP 复用 media-export 的项目权限、canvas/node/revision 冻结、幂等创建、outbox、Temporal workflow、execution/attempt fence、取消/重试/审核/下载；新增不可变 `export_artifact` 身份，不伪装 `media_asset.kind=video`。
- `media_bundle` 保留原始媒体字节、去重路径、正式来源 revision、编辑配置和 SHA 清单；`fcpxml`/`xmeml` 输出明确版本的 XML。两个交换格式支持图片、视频、音频、源入点/时长/时间偏移、轨道显示/静音、音量。无法表达的文字/字幕/crop/fade在入队前拒绝，不静默抛弃。
- 整包上限500MiB。每个原件检验尺寸/类型/字节/SHA，拒绝本地符号链接；生成后及未知 PUT 重试 adoption 均重新解析 ZIP/JSON/XML并验证条目完整性、媒体字节与冻结合同。ZIP 不携带内部 object key、签名URL或服务器本地路径。
- 真实AV测试发现旧 FFProber 未读取视频文件的音轨 channels，已补取实际 audio stream。工程 worker 对历史 nil AudioChannels 从同一 SHA 校验的原件补读；未知 PUT adoption 也重新核验，避免历史视频声音静默丢失。
- 客户端先校验 job/revision/attempt/project/artifact/格式版本及签名有效期，再下载检查包验证 MIME、大小、SHA和ZIP签名；用户解压检查并明确勾选后，提交该正式revision/SHA审核。最终下载仍校验字节。客户端校验不冒充完整ZIP解析或目标剪辑软件验收。

## 已执行的验证

- 前端定向 9 文件107 tests通过：director/timeline/Yjs/theme-policy；包括6个新增工程格式/身份/真实字节校验/下载审阅入口用例。`tsc --noEmit`、timeline export scoped eslint通过。
- Go scoped `golangci-lint run ./internal/mediatool/... ./internal/media/adapter/workflow/... ./internal/app/... ./tests/media/...` 0 issues；scoped vet通过。govulncheck mediatool/app 0 reachable vulnerabilities；另1个module级漏洞未被这些代码调用，不将其描述为整个依赖树无漏洞。
- 实际生成 PNG、NTSC 30000/1001 H.264/双声道AAC MP4、PCM WAV 并封装三个ZIP格式；独立 XML 解析检查序列帧与源帧换算、0.2s源入点、0.4s偏移、0.8s时长、0.5gain、静音和音轨保留。
- 官方 FCPXML1.10 DTD下载至临时目录，以 `xmllint --nonet --dtdvalid` 验证图片与音视频工程通过。DTD版权文件未复制入仓库。测试环境通过 `GLOWLOOM_TEST_FCPXML_DTD` 指定该本地文件；默认缺少时该外部规范验证会明确skip。
- 隔离 PostgreSQL18 `127.0.0.1:55439/glowloom_canvas_director_tests`：增量SQL已演练；实际运行使用 `glowloom_app` 受限角色，owner仅建fixture。真实保存canvas后冻结导出，验证创建幂等/旧revision拒绝/unknown PUT只写一次/独立artifact回读/审核前下载拒绝/跨项目404/错误SHA和旧revision审核拒绝/取消后预览拒绝/旧attempt迟到commit拒绝/重试新artifact身份/禁止runtime UPDATE artifact/不存在伪media_asset行。
- 正式存储路由补验：`TestProjectArtifactRoutedPostgresAndNativeTOS` 使用上述受限PG与真正TOS SDK连接受控本地TLS wire fixture，走原件ingest→工程ZIP→storage_work_intent/object_location accepted→授权签名→审核→实际ZIP下载；断言exact profile、唯一artifact位置和重复执行不新增写入。此为真实SDK协议及数据库绑定验证，不是云厂商账号/公网对象存储验收。
- `go test -race ./tests/media -run '^Test(ProjectExport|ProjectArtifact|ProjectFCPXML|ExportDurable|ExportCancellation|ExportRuntimeRole|TimelineRenderer)' -count=1` 使用上述真实PG与DTD配置通过，包含原MP4渲染回归。最新音视频/旧metadata回归也通过。

## 环境与尚未验收

数据库迁移文件：`backend/db/media_project_export_incremental.sql`，只增加artifact表/列并精确扩展export约束与权限，BEGIN/COMMIT，未清空或重置数据。root已核对本地glowloom业务库 export_job 既有约束并执行此增量迁移，报告 BEGIN→COMMIT 成功；本子任务未自行操作业务库，后端重启由root协调。

工作流使用 Temporal SDK test environment，存储适配器保存真实字节并模拟unknown PUT，不等同于部署的Temporal/S3链验收。尚未完成实际浏览器工程下载、真实云账号对象存储环境验收、目标NLE导入及在线媒体读回。FCP7相对媒体路径可能需要重新关联，未确认的软件行为保留待证。批量包与两种交换格式完成核心链路不代表源站全部工程格式完成。
