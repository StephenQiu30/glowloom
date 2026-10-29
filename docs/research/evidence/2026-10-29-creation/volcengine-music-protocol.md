# 火山音乐可编码协议核验

日期：2026-10-29。来源：[供应商调研](../../14-音乐数字人与动作模仿供应商接口调研.md)。O＝官方观察事实；I＝实现建议；U＝仍未知。只读取官方网页及网页链接的 Go 示例 ZIP 内文本；未执行示例、未读取密钥、未发送生成请求。

## 1. 证据来源与时效

本轮 web／Firecrawl 连接返回 429，改为对官方 URL 作只读 HTTP GET，解析页面内 `window._ROUTER_DATA` 的 `curDoc.MDContent`，得到完整官方正文。ZIP 在内存中解包阅读，没有运行代码或落地 SDK。

| ID | 官方来源 | 页面更新／观察 |
|---|---|---|
| M1 | [生成人声歌曲](https://docs.volcengine.com/docs/Audiovideounderstandingandprocessing/Generatevocalsongs?lang=zh) | 2026-07-24 |
| M2 | [生成纯音乐](https://docs.volcengine.com/docs/audio-video-understanding-and-processing/generate-pure-music-1?lang=zh) | 2026-07-24 |
| M3 | [查询任务](https://docs.volcengine.com/docs/Audiovideounderstandingandprocessing/Querytask-1?lang=zh) | 2026-01-16 |
| M4 | [公共字段](https://www.volcengine.com/docs/84992/1967910?lang=zh) | 本轮读取完整字段表 |
| M5 | [常见错误码](https://docs.volcengine.com/docs/audio-video-understanding-and-processing/common-error-codes?lang=zh) | 2025-03-26，早于当前模型文档 |
| M6 | [产品计费](https://docs.volcengine.com/docs/audio-video-understanding-and-processing/product-billing-1?lang=zh) | 2026-07-24 |
| M7 | [异步回调](https://docs.volcengine.com/docs/audio-video-understanding-and-processing/asynchronous-callback?lang=zh) | 2026-01-30 |
| M8 | [官方 GenSongForTime Go 示例 ZIP](https://portal.volccdn.com/obj/volcfe/cloud-universal-doc/upload_4866da1d70e48e6d36275802dfc7f22e.zip) | 读取 `GenSongForTime/main.go` |
| M9 | [官方 GenBGMForTime Go 示例 ZIP](https://portal.volccdn.com/obj/volcfe/cloud-universal-doc/upload_0911919ad9f52d562d31843af256902c.zip) | 读取 `GenBGMForTime/main.go` |

## 2. 路由与签名（O）

全部请求使用 POST，HTTPS host `open.volcengineapi.com`，path `/`，Query 的 `Version=2024-08-12`。

| 功能 | 预付费 Action | 后付费 Action | 查询 |
|---|---|---|---|
| 人声歌曲 | `GenSongV4` | `GenSongForTime` | `QuerySong` |
| 纯音乐 | `GenBGM` | `GenBGMForTime` | `QuerySong` |

例如 `https://open.volcengineapi.com/?Action=GenSongForTime&Version=2024-08-12`。不存在本轮已确认的通用 `GenSong` Action；不要从 ZIP 文件名推断 Action。[M1、M2、M3]

签名地域固定 `cn-beijing`，服务固定 `imagination`，AK/SK；不能复用 `cv`、`cn-north-1` 或 Ark Bearer Token。M4 要求 Host、UTC `X-Date`、Content-Type 与 Authorization，正文 SHA256 头可选；M8／M9 实际加入该头参与签名。

官方 Go 示例采用以下合同：

- JSON 序列化一次，以实际发送字节计算 SHA256 的小写十六进制。
- `X-Date` 格式 `YYYYMMDDTHHMMSSZ`；credential scope 为 `YYYYMMDD/cn-beijing/imagination/request`。
- 排序后的 signed headers 为 `content-type;host;x-content-sha256;x-date`；canonical headers 为每项小写键、冒号、值、换行。
- canonical request 依次拼接 method、`/`、`Action=…&Version=2024-08-12`、canonical headers、空行、signed headers、body hash。
- string-to-sign 为算法名 `HMAC-SHA256`、完整时间、scope、canonical request hash，以换行分隔。
- HMAC 派生链：SK → short date → region → service → `request`；最后签 string-to-sign。**实际代码不添加 AWS4 前缀**，示例的注释写有 AWS4，但与执行代码不一致。
- Authorization 结构为 `HMAC-SHA256 Credential=<AK>/<scope>, SignedHeaders=<headers>, Signature=<hex>`。[M4、M8、M9]

I：传输使用 M1／M2 的 HTTPS。Go 示例使用 HTTP，不能原样复制传输设置。M4 写 `application/json`，Go 示例签名值为 `application/json; charset=utf-8`；实现选定一种后须保证签名值与实际请求一致。Go 标准库需正确设置请求 Host，不能依赖普通 Header map 替代 URL host。

## 3. 人声歌曲请求（O，M1）

字段大小写不可更改。

| 字段 | 类型／规则 |
|---|---|
| `Lyrics` | string；中文 5–700 字符，英文 5–2000；v4.0／v4.3 与 Prompt 二选一，同传时歌词优先 |
| `Prompt` | string；页面同时写“仅支持中文”和“英文 5–2000”，存在矛盾；中文 5–700 明确 |
| `ModelVersion` | string；`v4.0`、`v4.3`、`v5.0`；当前文档默认 v4.0，未来会改，必须显式固定 |
| `Duration` | int；秒，显式有效范围 30–240 |
| `Lang` | v4.3／v5.0；默认 `Chinese`；完整语言枚举需引用官方参数说明，不应自由输入 |
| `VodFormat` | v4.3／v5.0；`wav`／`mp3`，默认 wav |
| `CallbackURL`、`TosBucket` | 可选字符串；回调地址／自有桶 |
| `SkipCopyCheck` | 可选 bool；true 关闭版权检测，false 开启。I：正常路径保持 false |
| `AigcWatermark` | bool，默认 false |
| `ImplicitWaterMark` | 对象；`ContentProducer`、`ProduceId`、`ContentPropagator`、`PropagateId`、`Enable` |

v4 风格字段：`Genre`、`Mood`、`Gender`、`Timbre`；v4.3 还有 `GenreExtra`、`Key`、`Kmode`、`Tempo`、`Instrument`、`Scene`。**这些字段在 v5.0 不生效**，不能把旧参数控件呈现成新版本可控效果。具体枚举见 M1 链接的[官方参数说明](https://www.volcengine.com/docs/84992/2097647?lang=zh)；本轮未逐项核验全部枚举。

以下是按官方字段构造的最小 fixture（I），不是实测回执，也不是直接复制官方歌曲内容：

```json
{
  "Prompt": "创作一首描写清晨海风的温柔歌曲",
  "ModelVersion": "v5.0",
  "Duration": 60,
  "Lang": "Chinese",
  "VodFormat": "mp3",
  "SkipCopyCheck": false
}
```

歌词模式将 `Prompt` 换成 `Lyrics`，由产品明确选择输入模式。U：v5.0 同传歌词与提示词的组合语义没有在字段表完整解释；不默认继承 v4 的优先级。歌词结构标签支持 intro／verse／chorus 等，但它们不是独立 JSON 字段。

## 4. 纯音乐请求（O，M2）

| 字段 | 类型／规则 |
|---|---|
| `Text` | 必填 string，中文描述；本页未写最短／最长字符数 |
| `Version` | **Body 中的模型版本**，当前默认／全面升级 `v5.0`；不是歌曲的 `ModelVersion`，也不是 Query API 版本 |
| `Duration` | int，秒，默认 60；v5.0 范围 30–120 |
| `EnableInputRewrite` | bool，默认 false |
| `Segments` | 可选对象数组，仅 v5.0；每项必填 `Name` 和 `Duration` |
| `Segments[].Name` | intro／verse／chorus／inst／bridge／outro |
| `Segments[].Duration` | 多段时每段 5–120 秒；仅一段为 30–120；总和必须 30–120 |
| 其他可选项 | `TosBucket`、`CallbackURL`、`ImplicitWaterMark`、`AigcWatermark` |

v5.0 不再需要 Genre／Mood／Instrument／Theme，通过 Text 描述。时长优先级为 **Segments 总和 > Text 内指定时长 > 外层 Duration**；外层设 45、Text 写 80 秒，文档示例会按 80 秒意图处理，不能把 45 当绝对成本上限。[M2]

```json
{
  "Text": "清晨咖啡馆里的轻柔钢琴背景音乐",
  "Version": "v5.0",
  "Duration": 60,
  "EnableInputRewrite": false
}
```

上例是 I（本项目 fixture）。要表达明确结构，可加 `Segments:[{"Name":"verse","Duration":30},{"Name":"chorus","Duration":30}]`。不要向 BGM 请求发送歌曲的 `Lyrics`／`ModelVersion`／`VodFormat`，本页未声明这些字段。

## 5. 智能时长、报价和用量

O：歌曲 Duration 可省略，官方示例给出 0，但正文只声明有效范围 30–240，**未解释 0 或省略具体如何决定时长**。BGM 省略 Duration 有 60 秒默认值，同时可能受 Text／Segments 覆盖。[M1、M2]

I：不能直接把即梦“智能时长”开关序列化为 `SmartDuration`、`AutoDuration` 或 `Duration=-1`，官方未提供这些字段。首个可验收实现可提供显式时长；若保留智能时长，应明确其省略字段的产品语义属于待实测决策，不声称供应商提供同名能力。

O：M6 公布人声歌曲 standard／纯音乐后付费均 **0.002 元／最终成功生成秒**，按小时结算，公共资源池 QPS 不超过 2。预付费按成功一首扣一首额度；一万首资源包 15000 元。两种计费方式独立，不互相抵扣。M3 的 **`Result.SongDetail.Duration` 是小数秒**，明确关联后付费时长计费；没有独立 `usage` 或 `billed_seconds` 字段。

I：保留 Duration 原始精度，不能取整后当供应商账单事实，也不能用请求时长或 `PredictedWaitTime` 作为最终用量。若内部为毫秒，应明确转换／舍入策略并保留原始秒值。报价需区分预计／预留与最终消耗；尤其 BGM 文本时长可覆盖外层参数。U：小数秒实际计费取整、最低消费和账户合同价未在这些页面说明。

**对旧调研的更正**：M6 明确提到个人用户付费购买试用版、企业首次可免费，与旧快速接入指南“购买需企业认证”的说法不一致。因此不能继续把“必须企业认证”作为绝对门槛。M6 的 BGM 60 秒上限也与 M2 v5.0 的 120 秒上限冲突；实现输入 schema 以型号专属 API 页为依据，账户套餐可用范围另行确认。上述差异尚未通过控制台验证。

## 6. 提交、查询、错误与结果

O：提交成功示例外层 `Code:0`、`Message:"success"`；`Result.TaskID` 是字符串，`PredictedWaitTime` 是预计等待秒数。`ResponseMetadata` 包含 RequestId／Action／Version／Service／Region／Error，成功时 Error 为 null。[M1、M2]

查询 POST `/?Action=QuerySong&Version=2024-08-12`，body 为 `{"TaskID":"已受理任务ID"}`。[M3]

| 返回字段 | 可编码含义 |
|---|---|
| `Result.Status` | 0 等待、1 处理中、2 成功、3 失败；其他值 U |
| `Result.Progress` | 进度；不要把 100 当成功判据 |
| `Result.FailureReason` | 失败对象 `Code`、`Msg`；成功时 null |
| `Result.SongDetail.AudioUrl` | 结果地址；文档域名模式 `v{1-50}-default.douyinvod.com` |
| `Result.SongDetail.Duration` | 小数秒，后付费关联字段 |
| `Captions`、`Lyrics` | 字幕字符串／歌词字符串；不默认提供歌词文件 |
| `StyleInfo` | v5.0 返回的 JSON **字符串**，不是直接嵌套对象；内部时长不能替代外层计费用 Duration |

O：结果 URL 有效期一年，但只供转存，默认 WAV 仍可能因转码链路返回 MP4，需要检测真实媒体并转码／入库。[M3]

I：先验证外层 Code／ResponseMetadata.Error，再验证 TaskID／Status／结果结构。M3 故意展示 Status=3 的失败示例，其中仍含 SongDetail；**不能以存在 AudioUrl 判成功**。M2 的响应示例误写 Action=GenSongForTime，不应按该示例把 BGM 请求改为歌曲 Action。

O：M5 明确的错误类别如下；数值来自官方表，中文描述已概括。

| Code | 意义 |
|---|---|
| 100001／100010／100011 | 内部错误／参数错误／海外 IP 限制 |
| 200020／200021／200024 | 签名无效／授权过期／音乐功能禁用 |
| 200022／200027／200028 | 资源用尽／套餐过期／无可用套餐 |
| 200023／400040 | 超 QPS／队列满 |
| 200026／300052 | TOS 桶无效／任务未找到 |
| 300030／300067 | 算法错误／语义识别错误 |
| 300061／300062 | 输入／输出歌词版权检查失败 |
| 300063／300064／300065／300066 | 输入／输出内容检查失败、无中文、语言不支持 |

M2 另出现版权校验 `50000001`，未列入较早 M5。U：全部 HTTP 状态与业务码对应、非空 ResponseMetadata.Error 的完整结构、服务端幂等 key、取消协议、TaskID 保留期限、提交超时后按自定义 ID 查找的协议均未确认。I：保留未知错误与原始响应；不因超时或未知状态重复付费提交。

## 7. 回调与最小验证边界

O：CallbackURL 接收 POST JSON，包含 CallbackTime／TaskID／Status／FailureReason／SongDetail；终态 Status 为 2 或 3。回调响应示例为小写 `{"code":200,"message":""}`；失败通知最多重试两次，间隔 5／10 分钟。[M7]

U：M7 未给回调签名／身份认证。I：首个闭环使用已签名主动查询，不能仅信任外部回调改变结算状态；回调可触发主动核验。官方回调示例 JSON 有缺逗号，不能直接作为严格解析 fixture。

可以立即编写的离线测试：固定时间与假 AK/SK 的签名向量、两类 JSON 字段／版本分离、时长边界、0／1／2／3 状态、外层错误、失败仍含结果、TaskID 不匹配、未知响应、Duration 小数精度、提交超时不重发、下载文件真实媒体识别。以上不等于真实供应商验收。

真实开通后仍须验证：账户权限与计费类型、明确版本可用、单次提交／查询／结果转存、实际用量对账、智能时长语义、默认模型变更影响。此文件仅新增文档，不修改实现或其他研究文件。
