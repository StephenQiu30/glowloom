# 时间线导出补充验证（2026-10-29）

## 本轮代码与回归证据

- `TimelineExport` 增加 `compact`：画布节点只显示具有“导出”可访问名称的图标按钮；全屏保留文字和更多方式。配置、提交与状态复用同一会话。
- `pnpm exec vitest run tests/unit/components/canvas/timeline/timeline-export.test.tsx`：16 项通过。
- `pnpm exec eslint src/components/canvas/timeline/timeline-export.tsx tests/unit/components/canvas/timeline/timeline-export.test.tsx`：通过。
- 新增验证：重复点击取消只发一次请求，使用任务当前 revision，等待服务器的 cancel_requested 状态而不提前显示已取消；失败重试复用原任务和冻结来源，不创建新任务；准备来源期间卸载，不发送新导出请求。
- 上述为组件与服务替身测试，不代表真实服务取消、软件互通或所有场景验收通过。

## 原生应用实际观察

使用实际生成的 `/tmp/glowloom-timeline-interop-20261029/xmeml/timeline.xml` 及包内三份合成素材。源工程为：视频从源1秒处开始取3秒，图片5秒，独立音频从源1秒处开始取6秒并在时间线1秒开始，总时长8秒；视频音量0.5、音频0.75。

- DaVinci Resolve 21 的独立测试项目“织光时间线互通验收-20261029”可读取 XML，导入设置显示1280×720、25fps、时间线名“时间线互通验收”。
- 继续导入后提示三个媒体未找到，要求重新定位；XML的相对Media路径未被自动解析。尚不能据此认定实际编辑工程导入通过。
- 尝试定位包内Media目录，并将相同包复制到该应用已有媒体目录下进行单独素材导入。原生工具多次返回 user changed / timeoutReached；未取得成功导入的时间线与片段回读。
- 剪映专业版已安装，真实路径 `/Applications/VideoFusion-macOS.app`，不能报告“未安装”。首页本地草稿更多菜单提供“导入PR或FCP工程”。已打开文件选择框；尚未取得导入后编辑工程的验收结果。

## 剪映来源合同证据边界

对已观察并下载的即梦公开浏览器包做静态分析（只提取行为，不复制实现）：客户端将已保存草稿版本、项目、时间线节点及目标jianying交给服务端网关；成功结果必须为未过期的deeplink且版本严格匹配，之后交给浏览器打开。此证据证明即梦原生交付不是简单将XML改名；不证明我们已经具备兼容的草稿服务或deeplink协议。

本地剪映的PR/FCP导入能力可以另行验证XML兼容性，但不能替代即梦“一键打开剪映专业版”的交付验收。当前界面保持该未接通入口真实不可用，未返回虚假成功。

## 后续复测与序列起点修正

- 本轮再次从截图定位剪映真实菜单，经原生文件选择器输入上述XML的绝对路径并确认“打开”。随后连续AX、截图、嵌套编辑进程绑定均返回`timeoutReached`。重置CUA会话再连接同样超时。没有得到导入成功或明确格式拒绝的回读，因此结论仍是“未验收”，不能把工具超时当作格式不支持。
- 剪映11.6.0的公开Info.plist注册`vega`/`videocut` scheme和`.jypack`文档类型；尚未观察有效payload，未猜造deep link或本机草稿。
- Resolve导入设置默认显示01:00:00:00，暴露我们XMEML未声明序列时间码。已补显式`00:00:00:00`、frame0、NDF和序列rate。依据[Apple XMEML编码规范](https://developer.apple.com/library/archive/documentation/AppleApplications/Reference/FinalCutPro_XML/Basics/Basics.html)。这保证文档表达从0起点；更改后尚无新的原生导入回读，不宣称目标软件接受。
- 新测试`TestProjectXMLPreservesZeroSequenceTimecode`先失败（Timecode空值）后通过；`go test ./tests/media -run 'TestProject(XML|Export)' -count=1`、`go vet ./internal/mediatool/adapter/editproject`、`go test -race ./tests/media -run 'TestProject(XML|ExportsContain|ExportRejects|ArtifactRejects)' -count=1`通过。集成测试未配置数据库的部分不计真实服务验收。
- 全屏主导出与更多按钮高度统一36px；画布紧凑图标尺寸保持。
- 三个既有互通ZIP的原始素材SHA256逐一核对通过；XMEML、FCPXML、素材包分别为843873、837425、834480字节。这是容器与原素材完整性，不是软件导入验收。
- `go run golang.org/x/tools/cmd/goimports@v0.51.0 -d`对两份本轮修改Go文件无差异；首次不带版本调用缺telemetry go.sum，改为版本固定的工具调用后通过，未修改项目依赖。环境没有`golangci-lint`和`govulncheck`可执行文件，本轮未执行这两项，未声称全量质量门禁通过。

## TLA24音频独立工程的导出边界

按本轮即梦实测更新：仅音频工程可以播放，主视频导出入口为`aria-disabled=true`并提示“当前时间线暂不支持此操作”；更多方式仍可使用。`TimelineExport`通过必填`hasVisual`显式接收该状态，只限制视频导出、确认与提交，不限制XML/素材包等工程操作。

新增组件用例先失败后通过，覆盖禁用语义、实际tooltip、更多菜单可用，以及无视觉时仍成功向API提交xmeml请求。导出组件测试更新为17项通过，ESLint通过；此处API为测试替身，不作为真实XML导出任务证据。

## 最后一次源端原生交付尝试

主验收在即梦真实“导出到剪映”→“打开剪映专业版”弹窗点击了主操作，源端出现“正在导出剪映草稿…”及取消入口。继续检查浏览器打开应用确认时，原生工具返回Mac已锁屏、自动解锁失败；后续键盘CDP操作同样超时。已请求用户手动解锁，未获得成功打开剪映或草稿导入的回读。不得将进入导出中或通知消失记作交付成功。

## 已加载时间线分块的只读协议复核

复核对象为浏览器实际加载、经`pageAssets.list`确认后下载的`async-canvas-node-timeline.860c26ba2d.js`（60823字节，SHA256 `3f6f77c3c79a5c52ed5e9c17c2e02c95465c4f9c2d032d3ac9f62fba35691752`），以及既有`canvas-core.8751e7ea24.js`（SHA256 `8f194bf4096477bf048a451313b9c959da8f50e1887b6e306ebf406b3c75c0a6`）。此项为静态证据，不是一次新导出请求的网络回执。

1. 时间线分块只把`featuresApi.canvas.jianyingDraftGenerate`会话传给共享导出控件（模块43293），并控制反馈是否显示。它没有deeplink scheme、payload、jypack编码或草稿文件协议。对该分块的`deeplink`、`launchUrl`、`vega`、`videocut`、`jypack`、`exportEditableProject`精确检索均无命中；这是该文件范围内的负证据，不代表整个产品不存在其他实现。
2. 在既有核心分块中进一步找到服务端任务包装：提交内容包括`project_id`、`timeline_node_id`、`exact_draft_version`、由目标映射得到的`export_type`及`submit_id`；不确定提交保存原提交标识，成功拿到`task_id`后按`task_ids`轮询。客户端默认最多200次、间隔3000ms，处理pending/fail/success与expired。
3. 核心包装只从成功任务读取`result.url`和`result.expires_at`；剪映目标标为`kind: deeplink`，其余软件目标标为`kind: download`。错误映射提及`/octo_api/v1/export/package/mget`。已读取文件中没有返回URL的构造规则、scheme参数定义、签名合同或jypack格式。目标export_type枚举在外部模块77865引用，本次文件中未取得其定义。
4. 会话再校验草稿版本和过期时间后交给`window.location.assign`。因此新增分块与核心包装补充的是“提交-轮询-短期交付URL”合同，仍不足以独立生成剪映可打开的草稿或有效deeplink。未调用即梦私有接口，未猜测URL参数，未据此更改产品代码。

结论：真实源站交付依赖服务端生成的短期URL。当前缺少可实施的剪映原生草稿/交付协议及成功样本；本机`.jypack`文件关联和vega/videocut注册只能证明入口存在，不能填补payload合同。Mac锁屏期间未继续操作原生应用。

## 解锁后源端剪映交付成功与限定草稿格式复核

用户解锁后，主验收在剪映真实编辑器观察到即梦导出的“ 一致性调研-202610221 ”测试工程：总16秒，视觉4秒+5秒，音频1秒@0、6秒@3、7秒@9。此项更新前述锁屏阻塞：**源站到剪映的交付现已成功，织光原生剪映交付尚未实现**。本节只读检查UI明确显示并经授权的该单个合成工程目录，未扫描其他用户草稿、未修改剪映工程、未读取账号凭据。

目录：`/Users/stephenqiu/Movies/JianyingPro/User Data/Projects/com.lveditor.draft/一致性调研-202610221`。按REA技能先固定目标与证据边界，实际调用已发现的`rea/open_binary`时工具被运行环境移除并返回“not available to the model”；未安装或改动REA配置，后续以本地只读JSON/文件检查保留证据，不伪造REA Evidence ID。

| 文件 | 字节 | SHA256 | 格式观察 |
| --- | ---: | --- | --- |
| template.json.bak | 22309 | 3c0033e2bfc829ab182bc96594eed89ab3e0eaa20c5d13fe2e0609eb1b866630 | 可解析明文JSON |
| draft_info.json | 12032 | 0a6f99164b92cccc309deeb15eb9a10873302b7712687fb474c49c9ca0e0b97b | Base64有效，解码9022字节；非JSON/ZIP/gzip前缀 |
| draft_meta_info.json | 3964 | 387de3f2599fcddf4a4ad95cf2aaf9837da3a16401232b85ac99cf13b565187b | Base64有效，解码2972字节；非JSON/ZIP/gzip前缀 |

不能仅凭不透明载荷推断加密算法、密钥或签名；本次没有尝试解密。明文备份保存的是源站生成的可编辑数据，`platform.os=web`、`app_version=10.5.0`、`version=360000`、`new_version=134.0.0`。设备标识字段不复制、不用于生成器。

最小独立serializer的数据映射已能确定：

- 根`duration`及`source_timerange`/`target_timerange`均以微秒计，已有毫秒时间线乘1000；`source_timerange.start`为源裁剪起点，`target_timerange.start`为轨道起点，两者不能混用。样本第二音频源起点1秒、长度6秒、目标起点3秒。
- `tracks[].type`区分`video`/`audio`，每轨`segments`通过`material_id`引用`materials.videos`/`materials.audios`。视频和图片同属视觉轨；图片材料`type=photo`，视频`type=video`，独立音频`type=music`。
- 素材`path`为相对`videos/<hash>.mp4|png`或`audios/<hash>.mp3`；复用音频文件有三个材料标识。公开数据字段可以独立生成UUID，不能复制样本标识或设备字段。图片的材料duration为10800000000微秒，实际展示时长由segment范围决定，不能将图片占位源长当时间线长。
- `volume`和`last_nonzero_volume`为浮点；样本全1，只能确定字段形式，静音/调整量语义仍需带变化样本。`speed=1`；视觉segment含clip的alpha/flip/rotation/scale/transform。无效果时材料引用还含speed、canvas、placeholder、sound-channel、vocal-separation；是否可以省略尚未验收。
- 样本canvas为1920×1080、fps30、ratio original。不能把它硬编码成所有用户项目；生成时应映射冻结工程画幅及帧率。

**入口兼容性仍有明确缺口**：目录没有`.jypack`容器样本；备份JSON不是原生“导入草稿”文件合同。应用保存后的主文件不透明，当前样本不能证明将明文JSON写入草稿目录会被11.6.0接受；也不能证明任意ZIP改后缀为jypack有效。下一步需要在此合成工程UI导出/打包草稿，取得其包装，并用完全独立生成数据经过同一入口回读。取得该证据前不连接“导出到剪映”成功路径，不注册猜测scheme，不用XML重命名替代原生交付。

## 实际503暴露内部码的修复

主验收观察本地依赖503时导出通知直接显示`dependency_unavailable`。已仅在`TimelineExport`汇总历史读取、轮询、提交/取消/重试及交付的错误文案：RequestError的常见状态复用既有中文映射，409提示状态已变化，400/422提示设置或素材不可用；未知内部英文错误使用当前操作兜底，既有明确中文校验和交付提示保留。没有改变请求、取消、轮询退避或重试状态机。

组件回归20项通过；针对观察到的503新增历史恢复与创建失败案例，并将轮询恢复案例使用同样的RequestError码与HTTP状态。额外验证503未知结果重试复用冻结输入和原Idempotency-Key、不重新准备版本，409确定拒绝仍重新读取版本，明确中文提示保留而内部字符串不外露。新增针对性断言在修复前失败、修复后通过，ESLint通过。这些为真实故障形态的替身回归，不冒充新的真实服务器恢复验收。

## 当前导出器重新生成互通样本

为避免将11:05的旧XML用于起点修复验收，本轮基于旧包manifest中的同一source/timeline/media构造FrozenExport，使用ffprobe读取包内原素材的尺寸、时长、帧率与音轨数，直接调用当前`editproject.NewRenderer(nil).Render`生成新包。没有手工修改XML，没有改动业务代码。临时Go入口运行后移至产物目录保留，仓库临时目录已移除。

- 可导入文件：`/tmp/glowloom-timeline-interop-current-20261029/xmeml/timeline.xml`。
- 媒体重定位目录：`/tmp/glowloom-timeline-interop-current-20261029/xmeml/Media`。
- 生成输入与程序：同目录上级的`frozen.json`、`paths.json`、`generator.go`；程序需置于backend模块内临时目录运行以符合Go internal导入规则。
- 新XMEML包844089字节，SHA256 `eb7ae3e9988b6834d0687cea28c71f53b4fe1d1ab56d8eb46cbec99ba13fe248`。
- 同时重新生成FCPXML与素材包，分别837425与834480字节。
- 独立解析断言：新旧manifest的timeline与media完全相等；三份原素材SHA256逐项吻合；sequence/timecode/string为`00:00:00:00`、frame为0、timebase为25。此项是当前格式器产物与完整性检查，原生导入验收由主流程另行回读。

原生补验：父任务恢复首页右键，Mac11.6.0无本地打包草稿项，快传仅另一设备。当前生成`/tmp/glowloom-timeline-interop-current-20261029/xmeml/timeline.xml`并核验零起点时间码，原生选择器选中且打开启用；点击打开后AX及截图读取连续timeout，未得到导入完成回读，仍不能签收。没有把XML入口替代剪映原生交付。

## 原生交付协议已取得：安装产物与真实源站交付交叉验证

本节更新前述“无入口合同”的阶段结论。REA 6.7.0已恢复，限定打开`/Applications/VideoFusion-macOS.app`，artifact inventory完成5931个内容节点、8344个occurrences，无完整性冲突。summary Evidence `ev_02b81dec870fa9bdad16580d79b54841d8c629578343d116c61934cd4457808a`；root SHA256 `23c7314f91cf82534f795f60ac481c9021019c2c54ebae2f07964497ec40494b`。Ghidra未配置，未安装或改动配置；后续以macOS自带符号和限定函数反汇编读取公开安装产物，不执行应用代码，不读账号数据。

- Info.plist确认11.6.0，注册`videocut`/`vega`与独立`.jypack`文件类型。`jypack`流程有独立混淆及权益检查，未作为本实现格式，不将普通ZIP重命名为jypack。
- `libVECreator.dylib`包含原生入口`videocut://com.ies.videocut/uganchor/anchor_point/load_pippit_draft?featureEntry=`。限定arm64函数`AnchorPointContext::doPretreat`确认featureEntry百分号解码后解析JSON，读取`feature`、`sence`（原拼写）、`enter_from`，把对象传入anchor。
- `LoadPippitDraftAnchorPoint::activate`先检查`feature_context.json_url`，存在则启动JSON描述导入；否则`LoadPippitDraftTask::downloadDraft`读取`feature_context.draft_url`，下载普通ZIP，解压后读取根`template.json`，检查`new_version`后通过`DraftDownloaderCross::importToLocal`导入。两条路径不是jypack流程。
- 主验收通过真实即梦按钮触发的`Page.frameRequestedNavigation`捕获上述deeplink：featureEntry含`sence:main`、`sence_context:{}`、`feature:load_pippit_draft`、`feature_context:{json_url:<短期描述URL>}`、`enter_from:octo_web`。没有调用即梦私有生成API。
- 仅下载该次合成测试的描述和其指向的ZIP。描述结构为`short_video_draft:{batch:false,folder_name,drafts:[{draft_url,draft_name}]}`。签名URL与完整描述仅存/tmp权限0600，不写入仓库或文档。

源ZIP为361492字节，SHA256 `17382a35b10a4538d4d281ebd6f66ecbeaf0a7a3985f26f1fce8dfd4014f41d1`，包含22309字节template.json和三份原媒体，无私有包装。视频4.017007秒（视频流4秒），音频7.032秒，图片320×180。三个音频材料引用同一个225069字节MP3，说明按文件去重并保留完整源素材，裁剪由source_timerange描述，未将各片段裁成新文件。源样本总16秒；视觉4秒@0和图片5秒@4，音频1秒@0、6秒@3（source start 1秒）、7秒@9。样本volume均1，不将此样本冒充静音或非1增益的源端证据。

## 独立原生serializer与验证边界

新增`output_kind=jianying`，持久化格式版本`134.0.0`，MIME为`application/zip`、扩展名`.zip`。复用原有私有artifact任务、授权预览、10分钟签名、SHA256、取消/重试和冻结版本验收。ZIP根包含独立生成的template.json、身份manifest.json和去重原素材；相对路径不包含对象存储key或远程URL。时间单位从毫秒准确转换为微秒，按轨道映射静音、可见性、clip音量、裁剪和片段起点；图片段长度独立于材料占位时长。serializer不是复制软件实现，不使用源样本标识或设备信息。

测试`TestJianyingNativeDraftPreservesEditsAndOriginals`通过：真实生成视频/音频，独立JSON解析确认多轨、source/target range、增益、静音、材料引用；同源多片段只包含一份原文件，全部原文件字节匹配；冻结编辑变更时拒绝旧包。现有`TestProjectExport*`全部通过。此为本地格式/完整性验证，真实剪映打开与编辑回读另由主验收记录，不能把调用scheme视为成功导入。

独立合成导入候选：`/tmp/glowloom-timeline-interop-current-20261029/jianying.zip`，842726字节，SHA256 `35b28c4eba621e23bdf17672fa28b7e6b4a4cf6de1730bcedcb5a3c27e6d8942`，工程名“织光剪映原生验收-20261029”。总8秒：视频source 1秒、长3秒@0（增益0.5），图片5秒@3，音频source 1秒、长6秒@1（增益0.75）。包由当前Go renderer生成，不手改JSON。候选供本地HTTP的正常原生导入入口验证，不写剪映草稿目录。

原生补验已取得：主验收使用普通`draft_url`入口打开8秒工程，确认源视频起点1秒、视觉3+5秒、音频6秒@1；该入口忽略template.name而使用随机文件夹名。随后按真实源站的`json_url`描述入口、独立草稿ID导入，首页出现“织光剪映命名验收-20261029”，打开后仍为8秒，3个素材就绪，音频属性为-2.5dB（线性0.75），初始视频画面为源1.000秒。截图由主验收保存在`completion-browser/local-jianying-named-native.png`。此处是本项目格式器合成包的原生兼容验收；完整项目API创建到worker到浏览器交付链路仍待服务统一恢复后验证。

同一template.id再次通过命名描述入口未形成新命名工程，独立ID包成功。生产冻结输入因此新增`export_id`绑定真实job.ID，native draft.id由该job确定生成；同job重试/验证确定，不同job（即使同一个画布节点）互不复用草稿ID。新增确定性和不同job隔离测试通过，缺少export_id的原生草稿被拒绝。

命名入口采用短期能力票据：已授权且succeeded的Preview生成随机256位token，DB只存token哈希，绑定job/artifact/project/revision及当前授权principal，保存当次已签ZIP URL对应的原生描述。`launch_url`同正式网页origin，公开GET仅接受唯一64位小写hex token；不接受job ID、任意URL，不重新签名。每次读取复核权限、来源、artifact与job状态/版本，过期不晚于ZIP授权且最长10分钟；API lifetime管理的一分钟有界清理批次回收过期回执，外键跟随job/artifact删除。HTTP no-store/no-referrer，访问日志仅固定路由，后端该路由排除OTel URL采集；清理失败只记录固定分类，不记录token、签名URL或原始数据库错误。

`TestJianyingLaunchCapabilityBoundToAuthorizedArtifactAndExpiry`、`TestJianyingLaunchHTTPHasNoSessionOrIdentityFallback`、native完整性与ID两项测试通过。查询/HTTP测试使用存储替身，不能替代PostgreSQL集成；当时`GLOWLOOM_TEST_MEDIA_STORE_DB_DSN`、`GLOWLOOM_TEST_PROJECT_EXPORT_OWNER_DSN`、`GLOWLOOM_TEST_CANVAS_DSN`均未配置，AGENTS/PROJECT只给出CI隔离PG18约定，本轮没有拿正式业务库执行测试清理。增量升级使用`backend/db/media_jianying_export_incremental.sql`，不对存量库重放schema.sql。

本轮收尾检查：`go test ./internal/mediatool/... ./internal/app/media ./internal/app ./tests/media -run '^(TestJianying|TestProjectExport)' -count=1`通过；`go test -race ./tests/media -run '^TestJianying' -count=1`通过；`go vet ./internal/mediatool/... ./internal/app/media`通过；版本固定goimports检查无差异，git diff --check通过。环境仍无golangci-lint、govulncheck可执行文件，未声称执行这些门禁；PostgreSQL receipt集成需要独立测试DSN，替身测试不冒充真实数据库验证。未创建提交或推送。


## 独立真实 PostgreSQL 启动票据验收

更新前述“缺少独立测试DSN”的阶段限制：本轮经主任务授权，从项目既有本地开发连接配置取得schema owner，确认CREATE DATABASE权限和已存在的受限`glowloom_app`角色后，在既有PostgreSQL实例上创建随机新数据库并加载当前完整schema。没有重放业务库schema、清理业务数据、改动全局角色、启动服务或重启Compose。密码和DSN仅通过子进程环境传递，没有打印或写入证据文件。

新增`backend/tests/media/jianying_launch_integration_test.go`，实际使用`SET ROLE glowloom_app`的受限运行身份完成合成项目、素材和画布保存，经真实Jianying worker生成成功任务，再通过Preview持久化票据并使用匿名HTTP读取。对象存储仍是内存测试适配器，因此此项证明数据库、授权、冻结版本与格式器生命周期，不代表S3网络或完整浏览器到剪映的端到端验收。

验证范围：

- 票据只保存随机token的SHA256，绑定当前job、artifact、project和冻结revision；描述内容来自该次授权预览，匿名读取不依赖登录cookie。
- 未完成任务拒绝预览；待审核任务不签发启动地址；伪造artifact、revision、project或actor均拒绝签发。超过10分钟或已过期的签发请求被拒绝。
- 运行角色不能UPDATE票据延长期限，数据库返回权限不足；job取消、job版本改变、冻结export_id或source revision改变，以及实际会话撤销后，既有票据均不能读取。会话撤销后也不能重新Preview。
- 已过期票据返回404；每批清理只删除上限内的过期记录，连续两批各删除1条，第三批删除0条，有效票据保持可读；非法批次大小被拒绝。

最终真实数据库Race回归命令：

```sh
go test -race ./tests/media -run '^(TestJianyingLaunchPostgresAuthorityIdentityExpiryAndCleanup|TestProjectExportDurableFrozenCanvasArtifactReviewAndUnknownPutRecovery)$' -count=1 -v
```

两项通过，包耗时46.295秒，无Race报告。既有artifact未知PUT结果恢复测试中的一次预期错误随后重试成功。新票据测试另有单独非Race通过记录。最终随机库`glowloom_timeline_launch_badce315f687`已成功删除；前序三次运行创建的随机库也均在finally中删除，未留下测试库。生成器只删除自己本次成功创建的唯一库名，不复用或强制删除既有数据库。

最终日志保存在`/tmp/glowloom-timeline-delivery-20261029/launch-pg-isolated.log`，只含测试输出和清理结果，不含连接串或签名URL。本轮只增加上述集成测试与证据记录，没有更改生产代码，没有提交或推送。


## 正式页面 → API/Worker → 剪映原生验收

主验收通过实际项目页面导出，等待完成后打开剪映；刷新页面恢复已完成任务，再次打开也成功。原生截图`completion-browser/local-jianying-formal-native.png`显示工程“timeline完整验收-1029”、总9秒、视觉4+5秒、音频1秒@0和6秒@1，3份素材均就绪。此项是正式项目链路，区别于此前静态HTTP候选包验证。原生观察和截图由主验收取得，以下持久化及服务日志另行只读核对；没有把HMR后丢失的CDP事件写成网络抓包。

只读连接强制`default_transaction_read_only=on`，按项目、画布与时间窗查询，得到唯一`jianying`任务。数据库真实创建时间为08:01 UTC，修正口述约08:07的估计：

| 项目 | 实际记录 |
| --- | --- |
| project | b0bb8394-a9a6-4c54-b058-71ac965513ad |
| canvas / node | 6b4b26fa-f20e-4949-baab-53e43746d511 / 0c5cdb0c-4fc9-4819-b2af-6b45e84790d2 |
| job | 5bf281ee-bb36-42a3-8d35-f44109cec1e0 |
| kind / status / stage / progress | jianying / succeeded / complete / 100% |
| attempt / job revision | 1 / 10 |
| source revision / frozen source revision | 106 / 106 |
| frozen export_id | 与job ID一致 |
| created / updated（UTC） | 2026-10-29 08:01:14.546163 / 08:01:16.499444 |
| artifact | f7481302-e7ff-5430-899d-0808d472a538 |
| format / version / bytes | jianying / 134.0.0 / 1077039 |
| job与artifact一致的SHA256 | 4f2d0832bc2ddad57dbaa34bca811441060a465965e0604124d50e890c937786 |

现有`glowloom-frontend`容器访问日志记录两次`GET /api/mediaexports/launch`返回200，分别为08:01:42.325719133 UTC及08:02:09.032961381 UTC。在进程内对日志票据解码并计算SHA256，与数据库保留票据关联，两条均对应上述job、artifact和revision 10；关联输出只含身份与时间，不含票据及签名URL。两张票据创建时间为08:01:18.232992、08:02:07.695498，到期时间为08:11:18.231307、08:12:07.686865 UTC。此项证明正式匿名BFF描述读取成功；当前后端容器stdout在该窗口没有可用访问事件，因此不宣称取得后端独立访问日志。

脱敏只读结果保留在`/tmp/glowloom-timeline-delivery-20261029/formal-native-db-redacted.json`、`formal-native-launch-log-redacted.json`、`formal-native-launch-correlation-redacted.json`。没有修改数据库、读取其他草稿、重启或提交。

额外发现：本次运行的Next开发访问日志仍含完整query，故虽然证据输出已脱敏，不能宣称运行进程已抑制票据日志。该事实已交给主任务确认配置及重启生效；历史日志不擅自删除。修复后的进程日志需另行复验。


日志抑制修正进展：容器实际Next版本16.3.6，`log-requests.js`直接执行`pattern.test(request.url)`；此前只匹配相对路径的正则对绝对URL返回false。现仅将next.config规则改为可选HTTP(S) origin前缀，仍只忽略原生启动路由。容器与主机配置SHA一致，使用容器实际loadConfig和ignoreLoggingIncomingRequests验证9项相对路径、HTTP/HTTPS绝对URL及其他路由，全部通过。

08:08 UTC的真实无效全a票据诊断返回404，healthz返回200；当前Next进程没有自动重载，窗口内仍出现1条启动query日志，health访问日志正常。因此本次只能签收规则本身，尚不能签收运行抑制生效。未手动重启，已通知主任务安排验收空档；重启后需再次核验。诊断只使用假票据，脱敏统计保存在`/tmp/glowloom-timeline-delivery-20261029/formal-native-log-suppression-redacted.json`。


日志抑制运行复验已完成：主任务在画布保存且无活动手势后统一重启现有frontend容器；本子任务未再次重启。08:10:08.180608 UTC起，在healthz已恢复200后发起全a无效票据请求，启动接口返回404；该时间窗启动访问日志0条、假票据字符串0条，而healthz返回200且普通健康访问日志仍有2条。确认新配置已在实际运行进程生效。脱敏结果为`/tmp/glowloom-timeline-delivery-20261029/formal-native-log-suppression-after-restart-redacted.json`。此前日志存在查询参数的历史事实保留，不以修复覆盖；没有删除或复制历史票据原文。


## 正式取消422：查询参数误用于控制请求

真实前端访问日志确认2026-10-29 08:13:05.351148796 UTC，`98ccd597-590b-4116-8a29-1d095d690df7`的取消POST返回422，且query含`project_id`。TimelineExport的control复用了GET参数对象`{job_id,project_id}`；生成client会把除job_id以外字段序列化为query，BFF取消/重试合同不接受query，因此在转发上游之前拒绝。不是会话失败、正文缺失或Idempotency-Key被覆盖。

仅修正控制POST传入`{job_id}`；正文继续携带project_id和revision，幂等键继续由既有调用提供，后续GET恢复继续带project_id。没有扩大BFF允许参数、弱化后端revision/授权校验或改生成client。取消和重试新增精确参数断言，两项在修复前失败、修复后通过。

新增`timeline-export-control-http.test.ts`通过真实Axios生成client、临时本地HTTP BFF和真实route代理对接合成HTTP上游：旧参数可稳定复现422且上游调用数为0；正确参数取消/重试均返回202，application/json、project_id/revision正文及Idempotency-Key逐项保留。临时HTTP监听器随测试关闭，不是新增应用服务。组件、BFF及HTTP集成共28项通过，定向ESLint通过。脱敏实际错误日志为`/tmp/glowloom-timeline-delivery-20261029/formal-cancel-422-redacted.json`。

第二个`540e3af7`任务在同窗口未发现取消POST，当前没有充分证据判定点击时是否仍准备/忙碌或已变为终态，未据此更改禁用保护。正式页面取消终止是否成功由主验收在修复后重新确认，本节不以HTTP合成回归冒充真实任务取消。没有重启服务、提交或推送。


## 已启用取消按钮无请求：模态层外通知

第二轮正式任务`84c0c11d-6891-4182-8643-f8026288ee65`点击启用按钮仍无取消POST。主任务只读DOM确认body的pointer-events为none，两份通知均直接挂body、inDialog=false且computed pointer-events=none。这与全屏时间线使用Radix modal Dialog、通知自行portal到document.body的结构一致；不是登录失败或请求头问题。

通知现通过稳定callback ref确定所属DialogContent作为portal容器，compact模式仍挂body，保持原顶部位置。增加suspended属性，仅在workspace做一行接线：全屏展开时隐藏底层compact通知，关闭后重新读取最新job；读取期间若本实例已接收更新回执或发起新操作，不允许旧列表覆盖。主任务真实DOM已确认全屏内只剩1份通知，inDialog=true、pointer-events=auto，随后点击确实发出取消请求。

新增真实Radix回归在修复前因启用按钮不属于modal而失败，修复后验证通知处于模态点击/焦点范围、focus和取消调用正常。另两项compact→fullscreen→compact回归确认单通知、返回后最新运行任务可继续取消、最新成功任务恢复终态交付入口。未修改共享Dialog或放宽模态保护。

## Worker版本推进导致取消409

主任务正式`cf82b065`样本在修复portal后首次取消触发POST409，手动刷新再取消得到202，证明worker推进revision可抢先于用户点击。现在取消遇到第一次明确409后读取权威回执，只有仍queued/running才以最新revision与新命令键自动重试一次；cancel_requested或终态仅采用回执，不再写；第二409及403/422/503不自动重试。没有改变后端乐观锁或将未知结果当成功，也不对“重试导出”套用取消逻辑。

三份定向测试共40项通过，包含真实HTTP/BFF参数回归、模态点击、模式迁移、一次版本冲突恢复、重试次数上限和终态/权限/无效输入/依赖故障保护。定向ESLint通过。正式取消最终回执与重试继续由主任务实际验收，本段不将合成回归视为worker已停止的证据。


正式回读补充（主任务观察）：`cf82b065`取消后数据库status=cancelled、revision=10、asset_id及sha为空，页面显示已取消；随后真实点击重试POST202，同一job的attempt=2最终succeeded、revision=22，下载GET200，Downloads实际存在1223840字节文件。原生下载事件工具超时，但真实文件存在，文件SHA/ffprobe/音频检查由另一个验收任务继续，未在此提前签收。关闭full后compact只有1份导出完成通知，点击下载真实成功。新增409自动恢复仍由下一任务样本验证。最后全量TypeScript检查0退出，不重复运行。


## 正式导出回流媒体识别与就绪状态

主验收任务`d1589d12-96b3-4f15-aed9-32fedaa6fb9e`已成功添加结果节点并保存，但时间线“画布资产”为空，视频可播放后无障碍描述仍为未核验。只读数据库确认对应媒体`62697e73-cc20-52fa-9b76-b185499866a2`为video/ready/passed、1223840字节，媒体与job的SHA一致，排除产物仍处理中的解释。

根因是前端时间线仅识别旧`ref_type/ref_id`，遗漏实际Yjs节点的`media_ref.asset_id`及固定CONTENT引用。新增timelineCanvasAssets薄封装，直接复用已有canvasConnectionMediaAssetId规则，列表、预取、节点clip别名、入轨前身份复核均用同一解析。原生、刚交付的固定引用及旧数据兼容；无效、歧义、本地blob/data或错误来源仍拒绝，不将任意URL当素材。

另serviceGraph投影有意将临时状态初始化idle，但useCanvasFilePreviews成功取得授权预览后只恢复URL。现同次成功恢复同步metadata.status=success；授权失败不伪造就绪，缓存回放再次投影也恢复准确状态。预览URL和就绪状态均不写入持久画布合同。四份定向套42项通过，包含新原生/固定引用识别、拒绝无效来源、授权失败仍未核验、成功后1项就绪及再次投影缓存恢复。正式页面回流再次显示/入轨由主验收继续回读。

## 剪映提醒偏好封装与存储门禁

剪映“不再提醒”已从TimelineExport移入timeline-view-preferences，仅读写固定key的精确字符串true，恢复提醒删除该key；非法JSON、数字、大小写或false文本均不启用，存储被禁用时继续显示确认而不阻断导出。业务存储门禁仅增加这个单文件白名单，没有扩大目录或放行job、素材、草稿、票据数据。两项偏好测试加现有导出组件共39项通过，定向ESLint通过。全量business-storage-policy仍准确报告另一个并行范围的canvas-yjs-pending-journal.ts，未更改或绕过该检测，不能宣称该门禁全绿。
