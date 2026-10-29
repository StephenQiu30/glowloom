# 火山视觉协议核验：OmniHuman1.5 与动作模仿2.0

核验日期：2026-10-29。证据类型：公开官方文档的 HTTP 读取，解析服务端嵌入 `window._ROUTER_DATA` 中的 `curDoc.MDContent`，不是搜索摘要。未读取任何凭据、开通服务、提交生成任务或修改源码/Figma。本文补充 research/14；文档契约不能证明账号权限或实际生成效果。

## 来源与版本

|标识|官方来源|实际文档/更新时间|
|---|---|---|
|V1|[OmniHuman 视频生成](https://docs.volcengine.com/docs/jimeng-ai/call-step3-video-generation?lang=zh)|1829013 / 2026-09-21|
|V2|[动作模仿2.0 API](https://docs.volcengine.com/docs/jimeng-ai/action-imitation-20-interface-documentation?lang=zh)|2201579 / 2026-09-21|
|V3|[API Key 调用说明](https://docs.volcengine.com/docs/AIGCAIzhongtaigongyongwendang/api-key-usage-guide?lang=zh)|2697409 / 2026-09-21|
|V4|[HTTP 签名示例](https://www.volcengine.com/docs/6444/1390583)|1390583 / 2025-03-05|
|V5|[通用返回与错误码](https://www.volcengine.com/docs/6444/69728)|69728 / 2025-03-05|
|V6|[OmniHuman 产品介绍](https://docs.volcengine.com/docs/JimengAI/Productintroduction-6?lang=zh)|1834143 / 2026-01-19|
|V7|[动作模仿2.0 产品介绍](https://docs.volcengine.com/docs/jimeng-ai/action-imitation-20-product-introduction?lang=zh)|2205633 / 2026-08-24|
|V8|[另一个 Omni 视频接口页](https://docs.volcengine.com/docs/jimeng-ai/call-step3-video-generation-1?lang=zh)|1913970 / 2026-04-28|

V1 是当前目录导航中的接口页，使用 `jimeng_realman_avatar_picture_omni_v15`。V8 可读取但不在当前导航中，使用 `jimeng_realman_avatar_picture_omni15_v2`。本次以 V1 为实现依据；没有证据证明两 req_key 可相互替换，也没有证据说明 V8 已停止服务。

## 传输与鉴权（已核验）

两种认证方式均有官方文档，不可混搭 host/path/Authorization。

|能力|AK/SK 模式 Action|API Key 模式路径|
|---|---|---|
|Omni 提交|`CVSubmitTask`|`/api/common/v3/submit_task`|
|Omni 查询|`CVGetResult`|`/api/common/v3/get_result`|
|动作模仿提交|`CVSync2AsyncSubmitTask`|`/api/common/v3/submit_task_v2`|
|动作模仿查询|`CVSync2AsyncGetResult`|`/api/common/v3/get_result_v2`|

- AK/SK：POST `https://visual.volcengineapi.com/`，Query `Action=<表中值>&Version=2022-08-31`。`region=cn-north-1`、`service=cv`、JSON body。
- API Key：POST `https://openapi.cv.volces.com<表中路径>`，`Authorization: Bearer <API Key Secret>`、`Content-Type: application/json`，body 与能力文档一致。此方式由 V1/V2 直接链接 V3；API Key Secret 来源 IAM API 访问密钥，不是 Ark 模型 API Key 的可互换承诺。
- V4 Python 签名示例：canonical URI `/`；query按key排序；body原始UTF-8字节SHA256。Canonical headers 与 SignedHeaders 均按 `content-type;host;x-content-sha256;x-date` 排列；`X-Date` UTC `yyyyMMddTHHmmssZ`。scope=`yyyyMMdd/cn-north-1/cv/request`。由 SK 依次 HMAC(date)、HMAC(region)、HMAC(service)、HMAC(`request`) 得 signing key，对 string-to-sign 做 HMAC-SHA256。Authorization=`HMAC-SHA256 Credential=<AK>/<scope>, SignedHeaders=<headers>, Signature=<hex>`。不能在 hash 后重新序列化 body。
- V3 列出通用取消路径，但 V1/V2 未给出各能力取消支持保证；不据此宣称支持取消。

## OmniHuman1.5 请求与约束（V1、V6）

提交 body：

```json
{
  "req_key": "jimeng_realman_avatar_picture_omni_v15",
  "image_url": "https://public.example/portrait.jpg",
  "audio_url": "https://public.example/speech.mp3",
  "prompt": "人物自然说话",
  "output_resolution": 1080,
  "pe_fast_mode": false
}
```

必填 `req_key`、`image_url:string`、`audio_url:string`。可选 `mask_url:string[]`（主体检测返回的mask URL，指定谁说话）、`seed:int`（默认-1随机）、`prompt:string`、`output_resolution:int`、`pe_fast_mode:bool`。

图片常见 JPG/JPEG/PNG/JFIF，严格小于5MB、4096×4096；音频严格小于60秒，官方建议15秒内。音频过长可能提交成功，查询时返回50215。产品页要求prompt不超过300字符，接口页写建议≤300；采用较严格产品限制。文档列中文/英语/日语/韩语/“墨西哥语”/印尼语，未自行修正官方语言表述。分辨率仅720或1080，默认1080；快速模式默认false。官方明确即梦效果一致性建议720→true、1080→false。输出MP4。

不指定多人讲话主体时可以跳过主体检测；本次未核验识别/检测接口完整字段。音频编解码、文件大小、采样率硬上限以及seed上限在上述页未列明，不能自行宣称。

## 动作模仿2.0 请求与约束（V2、V7）

```json
{
  "req_key": "jimeng_dreamactor_m20_gen_video",
  "image_urls": ["https://public.example/portrait.jpg"],
  "video_url": "https://public.example/motion.mp4",
  "cut_result_first_second_switch": true
}
```

`image_urls:string[]` 与 `binary_data_base64:string[]` 二选一，恰好1张图片；都传时base64优先，都不传则报错。`video_url:string`必填。图片/视频URL需公网可读。唯一列出的可选生成参数是 `cut_result_first_second_switch:bool`，默认true，去掉开头1秒过渡。没有prompt、seed、duration、resolution可选字段，不应把通用视频参数透传。

图片JPEG/JPG/PNG，480×480以上，4.7M以下；文档写1920×1080以内并明确超出会等比缩小，因此不能声称超过该尺寸必拒绝。视频MP4/MOV/WEBM、时长不超过30秒，200×200或以上、2048×1440以内；未列文件字节上限。尺寸措辞对竖屏的长短边规则未明确，应避免自行设定1920宽/1080高的硬拒绝规则。支持真人/动漫/宠物和多人驱动。输出MP4，720P、25fps，画幅尽量接近输入图片但不保证相等。

## 响应、状态、持久化（V1、V2、V5）

提交成功需要顶层 `code == 10000` 且非空 `data.task_id:string`。HTTP200本身不是成功证据。保留 `request_id`，不要把 `time_elapsed` 作为生成时长或计费用量（示例是字符串耗时，通用页声明int，两者存在不一致）。

查询 body为同一能力的 `req_key` + 原始 `task_id`；可加 `req_json`，它是序列化后的JSON字符串，不是JSON对象，用于 `aigc_meta`。其中producer_id/content_propagator必填，content_producer/propagate_id可选，每项≤256字符；本次不验证标识平台法律适用性。

```json
{"code":10000,"data":{"status":"done","video_url":"https://result.example/output.mp4","aigc_meta_tagged":true},"message":"Success","request_id":"provider-request-id"}
```

两类接口精确状态集合相同：

|status|含义与适配处理建议|
|---|---|
|`processing`|前置处理，继续查原task_id|
|`in_queue`|已提交/排队，继续查原task_id|
|`generating`|执行中，继续查原task_id|
|`done`|完成，可能成功也可能失败；要求外层code成功且video_url非空|
|`not_found`|任务不存在或已过期（12小时）；不能映射成功|
|`expired`|任务过期；展示可重新生成状态|

不是 `waiting` / `notfound`。失败时 `data` 可为null，应先检查错误再访问字段。`ResponseMetadata`存在且非null时按网关返回解析，优先保留其RequestId/Error.Code/Error.Message，不依赖服务层code。顶层数值`status`是冗余字段，不能当任务状态。成功URL有效期1小时，应通过项目已有资产导入归档链路持久化；仅保留provider临时URL不构成结果资产闭环。响应没有已核验的token用量、实际收费秒数或价格字段。

## 错误与重试边界

- 50200/50201/50204参数问题；50205/50211尺寸问题；50214视频过长；50215输入不符合能力约束；50400鉴权/能力权限问题。修正输入或配置，不能盲重试。
- 50411/50412/50413/50512/50513/50514审核失败，文档不建议重试；50511输出图审核页标可重试。
- 50429限QPS、50430限并发、50500/50501内部错误，业务页标可重试。通用旧页对50429 HTTP状态写404，而新业务页写429，应主要结合业务code分类，不只依赖HTTP码。
- 提交超时后的“是否已接受”未有查询幂等键契约；不能自动重新提交并声称不会重复计费。查询临时故障应继续查询原task_id。这里是工程处理建议，不是官方幂等性承诺。

## 价格、权限与仍未知点

V6：OmniHuman按生成视频时长1元/秒，默认并发1。V7：动作模仿2.0按生成时长0.4元/秒，默认并发1。两者均需控制台开通具体能力。此价格核验日期见本文，不证明账号实际折扣、账单、额度或权限。接口未声明motion裁掉1秒如何计算费用，也未列计费取整规则、usage返回、任务级实际扣费或通用幂等键。

实现可基于本契约接入真实HTTP和现有任务/资产生命周期；真实提交、账户能力开通、API Key可用性、媒体URL可达性、多人mask、计费对账仍需独立运行验收。GitHub公共API目录读取遇到403 rate limit，未把未读到的SDK当作证据；签名依据为官方HTTP示例。

## 证据内容摘要 SHA-256

摘要针对公开文档MDContent文本，不含凭据或用户数据。

- `omni`: `7b72b0a9a7aec49da0ed521e3920d4a130887b9da186ac3a742196657ec8d1c3`
- `motion`: `3fb95c9d6e6728cf59d2b1fabc3f05a158e703d3ee5a053a5fbca51b4bf5e3f1`
- `api_key`: `640f86f2c9da48540108b7be991cac5a2c7ad2cc4780511c2328b5c91b90ed2d`
- `http`: `dfd589d5f5044fe64eeb6f47d2d0a7b70045909ef9279b49f6109927418208da`
- `common`: `cd6e7488846e33e08a70064b1b03dbab385014085cd5dee33ddba2bc000faebb`
- `omni_product`: `9a2f7f999465cdc69f1333814a65dbbba116e83cba06e27b46d2921581ab750e`
- `motion_product`: `f8dfa4cb692980583323a72946533e8e29cc320e1aa421e3a2f5bc61431bb4bf`
- `omni_alt`: `31eb46b47a09eadacedb224a2dd1a5bc98dab7fb68499a04a9c0907e65aff80f`
