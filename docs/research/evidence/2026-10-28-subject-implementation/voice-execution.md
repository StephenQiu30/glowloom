# 主体智能配音执行链审查（2026-10-28）

## 交付合同

- API owner: `internal/app/subject/voice_jobs.go:76` `NewVoiceJobs(db)`。
- HTTP: `internal/subject/adapter/http/voice_jobs.go`，GET `/subjects/voice-options`；POST `/subjects/voice-jobs`（永久 UUID Idempotency-Key + draft_intent_id）；GET `/subjects/voice-jobs/:job_id`；GET `/subject-voice-commands/:key`。既有 protected session/Origin/CSRF 由根接入，不绕过。
- 模型选项来自实际可执行 release；配置无效/没有正式验证时 unavailable，不伪造默认已配置。speaker 只来自配置schema显式 enum/default。
- 工厂: `NewVoiceExecution(db,registry,prober,renderer,keys,speech)`；根新增 `NewVoiceExecutionWithObjects` 复用已有registry。任一必需依赖为空返回 nil，activity明确不可用。
- `subjectworkflow.NewStarter(client)`；`RegisterWorkflow(flowWorker)`；`NewActivities(executor).RegisterActivities(providerWorker)`；event.NewVoiceHandler(inbox,store,starter)，topic `glowloom.subject.voice.requested.v1`。

## 提交、恢复与正式媒体

`VoiceStore.Create` 在当前 exact session/target scope + actor真实个人空间授权事务中，冻结 actor/scope/physical/personal scopes、draft intent、release/model/config/price/credential、resource、speaker、text、rate、upstream cost、稳定assetID、storage intent。首次 admission 同事务写 outbox 和标准审计。永久键重放只恢复同job，payload或scope变更冲突。

Claim 是永久paid-send fence。现有加密Open仅在worker执行，密钥不进入workflow参数/HTTP响应/log/receipt。实际供应商请求前先 CheckWrite 确认固定registry profile/bucket可写，避免缺输出配置后才发起付费。

供应商已付费但网络未知/截断不重提。只有成功终帧后的真实MP3，在固定intent下经过 Reserve→Begin→SDK conditional PUT→Stat SHA/size/mime→Confirm，才允许 RecordReceipt。正式receipt存voice_job DB；不把对象存在当完整成功，不添加第三个manifest对象。

receipt已持久、导入失败保持ingesting。同永久键可发布新safe delivery；新的job/event workflow ID不受旧closed workflow阻塞。后台Sweep为queued/已收据ingesting发布新delivery；Claim仍确保paid send至多一次；unknown无receipt不进入自动恢复提交。

实际FFProber/FFUploadRenderer产生真实audio事实和PNG波形。原始MP3+波形exact两对象；media正式Accept和job succeeded用 `CompleteWith` 同事务提交。输出在创建者自己的个人媒体库，目标团队空间另行授权，不暗建project/不伪造upload local_review/operation。正式音频asset就绪后仍由现有Subject Save采用为voice。

## 官方协议

共享adapter统一 `volcengine_speech`，独立speech key；Subject wrapper复用共享providerhttp，不维护第二套HTTP客户端。

- 官方新HTTP说明: https://docs.volcengine.com/docs/doubao-voice/unidirectional-streaming-text-to-speech-http?lang=zh
- 官方完整历史HTTP/SSE说明 §2.3: https://docs.volcengine.com/docs/doubao-voice/http-chunked-sse-unidirectional-streaming-v3?lang=zh （成功20000000）。
- 官方新API-key SSE示例全文: https://github.com/bytedance/agentkit-samples/blob/main/skills/byted-text-to-speech/scripts/text_to_speech.py

考虑新HTTP文档code0+EOF与旧完整说明的差异，主体使用内部 `GenerationRequest.StrictSpeechTerminal=true`，精确POST `https://openspeech.bytedance.com/api/v3/tts/unidirectional/sse`。相同X-Api-Key/X-Api-Resource-Id seed-tts-2.0/X-Api-Request-Id稳定jobUUID；验证Content-Type text/event-stream、完整SSE data event、strict base64、明确20000000终帧。Result.ProviderTerminalCode来自实际解析；wrapper绝不凭completed伪造终帧。未知[DONE]、残缺event、JSON/协议不符、仅code0音频、音频后错误均unknown。通用现有HTTP调用保持自身clean-EOF合同。

## 与已有平台合同的关系

- 授权：正式沿workspace current scope/personal ownership和worker实时检查；scope ID与physical ID分开；不会扩普通media同项目授权。
- 目录/凭据：正式选取当前active release与确切model/config/price；需完整model_validation real_provider + completed operation + exact credential/provider binding匹配。无免费认证接口不要求last_test_result=ok，不自动测试/启用onboarding。
- 限流：Subject与普通operation共用catalog.provider FOR UPDATE，双向统计已dispatch的operation/voice及最近1分钟次数。root已确认统一保守unknown容量：Subject unknown也占active，必须人工核对，不能凭超时假failed或偷偷释放。
- 审计：标准audit.audit_log，同事务记录subject.voice_created/dispatch_started/receipt_recorded/succeeded/unknown/failed；仅身份与安全状态，不存text、key或对象key。媒体接受另有media.generated_audio_accepted审计。
- 计费：冻结正式release.upstream_cost，保存实际provider usage；无project的主体job不使用项目billing.reservation/预算扣款，不暗建项目。尚无项目外用户账务扣费合同。不能称为已沿项目计费或已与即梦扣费一致。

## 已验证与未验证的准确边界

已完成一次语音专项 `go test -race ./tests/subject -run '^(TestVoice|TestDoubaoVoice|TestSubjectSpeechRelease)' -count=1`，指定任务隔离PG数据库，Green 54.962s。该快照覆盖strict SSE和真实FF媒体恢复；随后新增的team跨physical个人输出、残缺SSE/output-route preflight以及标准audit断言，root新空库全Subject race已覆盖上述3项（其余仅并行source测试因库被复用失败）；本分支最后audit/unknown容量定向 -race 复验已Green 4.387s（全量来源在root新空来源库另验）。

- protocol正例/错帧：受控http.RoundTripper，验证真实shared codec与request/header；其短ID3数据只验证codec，不能冒充可播放供应商音频。
- worker durable intent：真实Postgres glowloom_app运行角色与权限；真实mp3由本地FFmpeg生成，真实探测/波形与media正式事务/ledger读写。
- voice_pg中catalog/credential/provider为清晰标注的fixture-proven输入，未构造假的real_provider证据用于激活；解密用测试key port。真实目录发布测试只证明合法speech draft可创建、未验证release不进入可执行选项。
- 对象SDK固定profile/条件写/receipt由媒体agent受控COS wire单测覆盖；voice_pg使用真实SQL ledger +fixture transport，不是远程实际对象存储的端到端验收。
- 根已接API/relay/Temporal注册；本分支未启动Kafka/Temporal/业务服务，也未进行其真实端到端运行。
- 未读.env/密钥，未调用真实付费服务，未验证用户当前真实speech连接/active release，未提交或推送。

最后定向回归：
- TestVoicePostgresPermanentIntentRecoveryAndAtomicRealAudio（含4条生命周期审计，permanent replay不重复）。
- TestVoicePostgresUnknownNeverReopensDispatch。
- TestVoicePostgresTeamTargetAndLiveRevocation。
- TestDoubaoVoiceStrictSSERejectsTruncatedEventAndWrongProtocol。
- TestVoiceWorkerRejectsMissingOutputRouteBeforePaidClaim。

## 最终验证结论

- 最终指定DSN：`postgres://stephenqiu@127.0.0.1:5432/glowloom_subject_alignment_final_20261028_a1806a?sslmode=disable`。`go test -race ./tests/subject -run '^(TestVoicePostgres|TestVoiceWorker|TestVoiceReceipt|TestDoubaoVoice|TestSubjectSpeechRelease)' -count=1` Green 4.387s，含audit四条不重复与unknown保守容量。
- 最终lint只发现测试空行whitespace，已去除。
- 本分支 Subject/appSubject scoped govulncheck：0 reachable；root更广全应用检查发现原有script/html路径 x/net GO-2026-6860（v0.60.0→v0.62.0），不把局部通过冒充全应用通过。
- 早期真实PG发现并修复outbox jsonb扫描错误（[]byte destination需结构列）、终帧共享协议差异；最终unknown谓词替换一度误伤UPDATE，定向回归捕获并修正。所有真实PG仅任务隔离库，无业务库迁移或清理。

## 旧库缺存储结构的最小收尾

- `db/subject_voice_incremental.sql` 仅在 `to_regclass('media.storage_work_intent')` 存在时扩展kind约束；独立voice_job的storage_intent_id/audio_location_id/manifest_location_id只保存UUID，不对尚未部署的存储表建立FK。完整schema同步此合同；实际RecordReceipt/worker仍逐项核对正式intent/location事实，不因缺FK放行。
- `StorageRoutes.ReadyForWork` 显式检查四张所需表的必要列，再读取既有active route；缺结构/缺route返回false，不创建、推测或激活配置；权限/连接/其它SQL错误原样返回。
- VoiceOptions先exact scope授权，再Storage.Ready；不可用返回 `available:false, reason:voice_storage_unavailable, models:[]`。首次Create同样要求Ready；永久键恢复仍可读既有job。runtime registry依赖为空及CheckWrite仍在paid send前拒绝。
- `TestVoiceLegacyMigrationWithoutStorageAndExplicitOptionsReadiness` 每次自行创建UUID专有模拟legacy数据库（前缀glowloom_subject_alignment_vl_）；仅含迁移必要旧表，实际运行增量两次，断言未创建存储事实及无存储FK。之后仅在该专有库加载四张canonical存储表验证“仅表无route仍不可用”；显式fixture route允许fixture catalog选项；真实REVOKE SELECT导致42501并向上传播，证明没有泛化吞数据库错误。模拟库无真实key/对象/付费证据，并保留供审查。
- 用户已选择火山TOS并自行填写配置；本改动不创建/激活TOS配置，不猜bucket、endpoint、key或对象存在性。配置示例/接线由root与media owner负责。
