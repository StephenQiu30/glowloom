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
