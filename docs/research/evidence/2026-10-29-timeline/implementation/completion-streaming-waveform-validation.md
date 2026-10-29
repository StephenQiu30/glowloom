# TLA-46 长音频波形流式解码

日期：2026-10-29，Asia/Shanghai。未提交、未推送。替换此前仅按 32 MiB 压缩文件限制的整段 AudioContext.decodeAudioData；旧时点日志保留。

## 实现与真实边界

复用项目已安装 Mediabunny 1.62.1，按需加载一次库。UrlSource 以 2 MiB 网络缓存、1 请求并发读取授权 URL；AudioSampleSink 顺序解码全部音频包。最多两个不同素材任务同时工作，同 URL/源时长的卡片和片段共享128峰值。没有按压缩大小或音频时长拒绝长音频，也没有整文件解码 fallback。

应用层只保留4096个float的复制缓冲（16 KiB）与128个全源峰值（512字节），每个解码sample在finally中close。按时间戳归入全源128桶，聚合所有声道，最后按全源最大值归一化，裁剪仍使用相同幅度基准。每64个复制块让出事件循环；最后消费者释放后取消任务并删除缓存，输入完成/失败均dispose。

每次元数据或解码迭代读取设置30秒无进展超时；这不是任务总时长限制。取消与超时会立即dispose Input并拒绝等待，释放两任务并发槽。晚到的sample仍close；不等待可能永远挂起的iterator.return。UrlSource背景错误进入dispose而不产生全局未处理拒绝。

边界：以上是本应用的缓冲和并发合同，不是整个浏览器进程的绝对heap上限。容器索引、原生解码器内部缓冲及单个编码包大小由库/浏览器管理。安装包media-sink.ts的队列回压常量为4；WAV demuxer每包2048帧。长压缩音频真实内存曲线尚未实测。

压缩编码依赖当前浏览器WebCodecs支持；PCM由库自带解码器处理。不支持编码、CORS/Range错误或停滞读取保留波形占位，可显式重试；不回退到整段PCM。无新增依赖，未修改共享运行时或workspace。

## 依据

- [Mediabunny UrlSource](https://mediabunny.dev/api/UrlSource)：用Input.dispose取消网络读取。
- [Mediabunny AudioSampleSink](https://mediabunny.dev/api/AudioSampleSink)：按顺序迭代解码样本。
- 安装版合同核对：node_modules/mediabunny/dist/mediabunny.d.ts 的UrlSourceOptions、AudioSample.copyTo(frameOffset/frameCount)、Input.dispose；src/media-sink.ts 与src/wave/wave-demuxer.ts。以安装版本为准。

## 测试证据与验收边界

- Red：completion-waveform-red.log，新增模块前测试按缺模块失败。
- 有界回归：completion-waveform-bounded-tests.log，3文件18项通过。包括10小时合成时间戳、固定缓冲复制、sample及时关闭、缓存共享、静音、失败重试、取消挂起任务、读取超时、超32MiB文件不拒绝。
- 实际库PCM合同：completion-waveform-pcm-tests.log，使用安装版真实demuxer与PCM decoder，读取测试HTTP Range WAV响应，两段幅度1与0.5的128桶结果正确。只有网络响应为合成fixture；未mock Mediabunny解码。这是Node合同测试，不是浏览器、真实CDN或长压缩音频接受证据。初次jsdom跨realm Uint8Array不匹配后改为Node环境，未修改生产实现规避该断言。
- 13:01:47 最终时间线+Yjs组合15文件202项通过；限定ESLint/Prettier通过，本时点全量tsc无诊断。此前其他日志里的并行类型失败仍保留其历史含义。
- 完整组合、ESLint、tsc最终输出：completion-waveform-final-tests.log、completion-waveform-final-eslint.log、completion-waveform-final-tsc.log。

用户已解锁，父任务开始恢复正式页实测。这里不提前记录7秒WAV、长音频或压缩编码实测成功；以父任务取得的回读证据另行更新TLA-46。
