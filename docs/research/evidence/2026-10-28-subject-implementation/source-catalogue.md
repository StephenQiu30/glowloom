# 主体混合来源目录实施

2026-10-28。独立SubjectSourcePage/Item，不再把混合来源包装成个人LibraryPage。无提交、推送、服务启动；未读业务库或凭据。

## 合同与闭合授权

- source=personal_asset/generated_output；仅后者携带服务器解析project_id。media保留原始origin和正式asset facts，TTS个人system产物不谎称上传。
- 本人personal originals与当前目标scope内本人发起的operation outputs共同UNION，全库搜索/收藏/quality=upscaled过滤、稳定排序后count和分页，同一RepeatableRead snapshot。
- 三类immutable receipts证明initiator：prompt_submission、quote_request单结果、quote_request批量items。JSON保持文本比较、拒绝错误形状，不用confirmed_by或普通项目读权限冒充发起人。
- PG catalogue同snapshot重新解析target+personal authority；每个匹配project沿普通ProjectContentAccess重新授权。读取允许active/archived，Source Save参考资格也是read=false，不开放archived写入。
- 排除history_record/output个人删除、operation/output删除或未完成、project回收站、library trash/purge、media未ready/未passed/真人/consent、缺哈希及超原文件预算。
- 超清只接受正式capability=image/video.upscale、image/video.super_resolution或mode=super_resolution；原生8K不冒充处理结果。后端支持video质量谓词，UI严格沿即梦：图片=所有图片/超清/收藏，视频=所有视频/收藏。
- item_id只作为服务器解析source identity。个人用assetID（保留metadata title/revision/favorite），生成用outputID；多行UUID碰撞拒绝。预览解析后使用真实personal或project owner+assetID，并在签名前后重查来源。
- 专用original reader在既有library owner授权后按asset_id读取active metadata。SubjectSourceStore.FindSubjectSource最小复用此reader，覆盖metadata ID!=assetID的来源→预览→新采纳；ReadBoundSubjectSource未改，普通media公开路由不放宽。

## 前端

- SourcePicker使用新生成DTO；核对actor/scope、source/project、正式asset及revision；生成源回传project_id，既有Subject SaveSelection保留。
- 图片/视频、收藏和图片真实超清均服务器完整查询，不对当前页伪筛选。
- 确认前重查detail，项目、source、asset或revision变化则拒绝静默绑定。预览仅送sourceID给服务器，不由浏览器指定owner。
- upload/voice正式个人音频继续以assetID预览，source=personal_asset，origin=system事实保留。未改root的unified-media/reference。

## 验证证据

- Source Query/Service/HTTP/Preview单位测试通过；同组Race通过（包执行3.092s）；gofmt执行。
- go vet目标media/application、media/postgres、app/subject、subject/http、tests/subject退出0；source ESLint退出0；全量tsc --noEmit退出0。
- goimports/golangci-lint/govulncheck不在PATH，未执行；不声称完整Go门禁完成。
- Root新建独立库glowloom_subject_source_20261028_a1806a执行TestSubjectCatalogueRealPostgresMixedPagingProvenanceAndLegacyMetadata，报告PASS（该top-level测试2.73s、包5.797s）。其中包括混合分页、三种receipts、confirmer/外scope/删除/trash排除、真超清vs原生8K、收藏、legacy metadata和archived/collision子场景。**仅1个top-level实际执行测试，不报多项已执行。**
- 新增同事务FindSubjectSource legacy Save断言由最终CI/主任务统一运行覆盖；不把编译时间不明确的结果扩张为每条最终新增断言已独立验收。
- CI缺GLOWLOOM_TEST_SUBJECT_SOURCE_DB_DSN现在Fatal，本地缺则Skip；fixture仅接受loopback+指定新库名前缀，检测到既有source schemas直接拒绝，未清空或改变已有库。
- 完整schema TestSubjectAppSourceLibraryKeepsTeamBrowsingPersonal已按合同更新。Root首次旧隔离库缺并行新增personal_history_record，属schema前置失败；root另建glowloom_subject_alignment_final_20261028_a1806a，完整schema统一验收结果待主任务记录。本agent本地该用例缺DSN跳过，不混算到上面1项SQL已执行。
- 新source picker最初4项通过；source contracts3项通过。组合picker/upload运行在CPU争用下2worker及1worker均有5000ms超时（没有行为断言失败），未改变测试超时/全局配置。最终顺序前端验证交由root；不把中断/超时报告为通过。

## 文件边界

backend/internal/media/adapter/postgres/subject_catalogue.go、media/application/subject_library.go、media/adapter/postgres/subject_source.go、app/subject/source_library.go、subject/adapter/http/sources.go；tests/subject/source_catalogue_test.go、source_catalogue_postgres_test.go、source_library_test.go和app_sources_test.go中来源用例。frontend unified-source-contracts/picker及对应tests。

没有改operation个人history拥有文件、普通media/library公开路由、音频worker、generated API或proxy（root分别处理）。本实施不证明真实供应商生产、浏览器混合gallery、跨项目选用完整链路或390实际布局已验收。
