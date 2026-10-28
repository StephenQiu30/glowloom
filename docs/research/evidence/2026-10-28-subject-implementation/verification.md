# 主体实现验证记录（2026-10-28）

本记录区分代码/受控协议、真实隔离数据库、Figma播放器、本地业务环境及真实供应商。实现目标见需求12→设计25→PRD12→Plan21；进度只在BACKLOG维护。工作树包含并行资产、模型接入等修改，未提交、推送或发布。

## 当前最终结果

以下结果替代后文各阶段的临时结论；失败与早期配置方案保留为历史，不再作为当前状态。

| 检查                                       | 当前结果与证据                                                                                                                                                                                                                                                                   |
| ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 后端 `go test -race ./...`                 | 全部包成功，状态文件值0；`/tmp/glowloom-subject-go-complete-race.log`。真实隔离PG的Subject、存储迁移、package、script测试已启用；app 19.498s、canvas 164.282s、identity 230.912s、media 59.527s、subject 25.510s。需要真实供应商等凭据的可选测试仍按原条件跳过，不计真实云验收。 |
| 前端 `pnpm exec vitest run --maxWorkers=2` | 153文件、1016项通过，66.65s；`/tmp/glowloom-subject-frontend-merged-final.log`，状态0。包含并行服务接入合并后的当前组合；此前1014项及最终手机专项16项同样通过。                                                                                                                  |
| Next生产构建                               | `GLOWLOOM_BACKEND_ORIGIN=http://127.0.0.1:1123 pnpm exec next build` 退出0，见 `/tmp/glowloom-subject-frontend-build-merged-final.log`。没有package build脚本，早期 `pnpm build` 的失败不当作成功。                                                                              |
| 类型／生成API／lint                        | tsc退出0，API一致性1项通过；正式Go目录 `./cmd/... ./internal/... ./tests/...` golangci-lint为0 issues，见 `/tmp/glowloom-subject-formal-lint-merged-final.log`。不包含忽略目录中的临时脚本。                                                                                     |
| 漏洞扫描                                   | `/tmp/glowloom-subject-govuln-final.log`：0个可达漏洞、0个导入包漏洞、1个未调用的模块层漏洞。x/net升级0.62及必要依赖修复早期HTML解析可达漏洞。                                                                                                                                   |
| Figma                                      | 77画面+3规格；1542个canonical NODE动作／88唯一目标／缺失0；43张卡片全部16:9，8处手机收藏44×44。新增Subject专用Default/Hover/Passive/Touch变体，不修改全局AssetCard。精确回执见 `figma-hover-ledger.json`。                                                                       |
| 本地运行                                   | backend、frontend、canvas-collaboration均running/healthy。backend使用现有开发镜像重新创建并启动时编译挂载源码；新镜像重建曾因Debian apt HTTP500／下载EOF失败，不记镜像构建成功。                                                                                                 |

后端全量早期因缺专属DSN、目录故障注入共享库和清理测试夹具的channel重复关闭失败，均保留日志。最终为canvas使用独立 `glowloom_reference`、catalog使用独立任务库；修正测试夹具使用pointer receiver与sync.Once，100次复测通过，未改生产清理调度。被中断且无状态文件的旧日志未计通过。CI同步提供独立catalog与必需fixture数据库，远端CI尚未运行。

实际业务 `glowloom` 已以事务增量执行 `storage_ledger_expand_incremental.sql`、`script_storage_locations_incremental.sql`、`storage_bootstrap_incremental.sql`；没有重放schema、清库、finalize或ActivateStorage，也没有伪造profile/route/连接事实。新库完整finalize及旧库扩展／真实字节回填门禁在隔离测试中验证；既有旧对象实际迁入TOS要在用户填写凭据后执行。

正式location与冻结work target已覆盖上传／原生SDK读取签名、普通生成原件和预览、script写入恢复历史、package、purge、项目copy／transfer，以及export／depth／transcription每次attempt。原生存储用量跨明确授权profile汇总，未知失败不返回部分小计，回收站与历史pin保留；受控SDK和真实隔离PG／FFmpeg通过均不冒充远程TOS。

真实Chrome已用当前旧MinIO兼容配置上传合成PNG，保存并重开「主体对齐本地验收-20261028」；390×844／390×600滚动和底部保存、无项目@创建／全部主体取消焦点恢复、主体选择及预览均已实测。主体保留供用户检查。浏览器下载事件监听20秒超时，但原生Chrome下载浮层显示完成，实际下载ZIP为4424字节、CRC无错误，manifest与PNG附件SHA256一致；真实本地下载落盘已签收，见browser-download-final.json。手机采用既有设计单列／16:9／底部44px新建，属于项目决定而非即梦手机事实。

配置只用根 `.env`，示例在根 `.env.example`。TOS字段留待用户填写；旧RSA内容已原样迁入同一.env以取消默认.local凭据挂载，未输出、轮换或提交密钥。真实TOS上传／旧对象迁移、真实豆包付费音频与音质、实际服务故障恢复仍单列为待实际环境验证；这些不阻塞已完成的无凭据实现和隔离测试。77画面没有全部重新逐帧播放器／浏览器实点，不宣称未观察竞品规则已达到完全一致。

## 历史阶段：初次命令与结果

| 检查                                                                                                                           | 实际结果                                                                                                    |
| ------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------- |
| `pnpm api:check`                                                                                                               | 1文件1项通过；生成接口、operationId与注解一致                                                               |
| `pnpm exec tsc --noEmit`                                                                                                       | 退出0                                                                                                       |
| `pnpm exec vitest run tests/unit/components/subjects tests/unit/components/generation/unified-mention.test.tsx --maxWorkers=1` | 16文件76项通过，24.45s；此前2workers争用时有超时/worker启动失败，未修改全局timeout；顺序复测默认timeout通过 |
| `pnpm exec vitest run tests/unit/proxy.test.ts tests/unit/components/layout/routes.test.ts --maxWorkers=1`                     | 2文件2项通过                                                                                                |
| 定向 ESLint                                                                                                                    | 主体组件、主体路由、reference-controls与同站代理退出0                                                       |
| Docker frontend Dockerfile `--target build`，后端origin为容器地址                                                              | Next编译、类型、页面生成及镜像导出通过；未切换运行中的前端镜像                                              |
| `go test -race ./tests/subject -count=1`，fresh完整schema任务库                                                                | 98.325s，仅来源fixture因被复用不满足fresh条件失败；其余测试未报告失败。不将此整条命令写成通过               |
| `go test -race ./tests/subject -run '^TestSubjectCatalogueRealPostgres' -count=1 -v`，另一个全新来源任务库                     | 1项通过，2.695s，含混合分页、发起回执、超清/收藏、legacy metadata ID、UUID碰撞等子场景                      |
| `go test -race ./tests/subject -run '^(TestVoicePostgres                                                                       | TestVoiceWorker                                                                                             | TestVoiceReceipt | TestDoubaoVoice | TestSubjectSpeechRelease)' -count=1`，完整schema任务库 | 最终4.387s通过，含audit、unknown占容量、team跨physical个人输出和撤权、strict SSE、无输出route付费前拒绝 |
| `go test ./internal/app/... -run '^$'`、定向 `go vet`                                                                          | 组合根编译通过；Subject/media/operation/prompt/app vet退出0；迁移前启动保护追加后app重新编译/vet通过        |
| scoped golangci-lint                                                                                                           | 最后0 issues（前轮指出局部变量命名、布尔表达式和测试空行，均已修正）                                        |
| scoped govulncheck Subject/appSubject                                                                                          | 0 reachable；全app扫描另见下表，不混为全应用通过                                                            |

媒体真实MP3探测、波形、正式个人媒体接受及调用方事务回滚另有 `TestPersonalGeneratedAudio` 6项Race通过。物理对象读写使用受控COS SDK wire，不能冒充实际远程对象存储验收。主体→草稿→报价→确认预算预留/outbox→worker当前授权→SDK签名与SHA读取、旧512字历史身份/SHA保留已由隔离PG覆盖；真实provider生成仍未调用。

## 迁移演练

只使用本次新建任务库：完整库 `glowloom_subject_alignment_final_20261028_a1806a`、存量演练 `glowloom_subject_alignment_migration_20261028_a1806a`、独立fresh来源库 `glowloom_subject_source_final_20261028_a1806a`；先前任务库继续保留用于证据追溯。未对现有业务库重放schema。

`subject_library_incremental.sql`、`volcengine_speech_incremental.sql`、`subject_voice_incremental.sql` 在迁移演练库按顺序各连续执行两次成功。已有长名称head/version完整JSON比较未变化。混合来源依赖并行资产功能的 `personal_history_incremental.sql`；目标库若缺此表须先执行该所有者的增量，不可用schema重建代替。

## 运行与供应商边界

- 本地业务库已按当前运行服务的cluster identity核对后应用主体库和豆包目录增量；音色增量因缺media.storage_work_intent在事务内回滚。中途现有后端重启后，缺subject.voice_job导致relay sweep退出；已在组合根补迁移前storage-ready检查，现有operation sweep继续运行，缺表不再拖垮旧功能。Docker随后观察到backend running/healthy；这不是新主体保存或配音接受。
- 未执行真实豆包付费合成，未读取用户Key；协议用受控RoundTripper，正式模型发布正例fixture与真实real_provider验证明确区分。未验证release不会进入可执行选项。项目外音色任务记录上游成本/usage，不虚构项目预算扣款。
- Figma演示和本地浏览器日历等交互不能替代真实上传、保存重开、390完整工作流、Kafka/Temporal端到端或真实供应商。
- 全应用 `govulncheck ./internal/app/... ...` 命中现有 `golang.org/x/net v0.60.0` 的 `GO-2026-6860`，可达路径为 `internal/script/adapter/extract/html.go` 的HTML解析；工具建议修复版本v0.62.0。本轮未把无关HTML依赖升级混入主体重构，故全应用扫描未通过。

## 可审查落点

- `backend/internal/subject/`：全局主体与配音任务业务所有者。
- `backend/internal/app/subject/`：当前空间、媒体、草稿/生成与输出组合。
- `backend/internal/app/roles_wiring.go`、`app/catalog/provider_wiring.go`：relay/Temporal/provider实际注册。
- `frontend/src/components/subjects/unified-*`、`subject-date-range.tsx`、`app/subjects`：统一命名与所有交互状态。
- `frontend/src/api/subjects.ts`：由项目工具生成；同站代理显式开放对应路由。
- `docs/research/09-*`、`docs/requirement/12-*`、`docs/design/25-*`、`docs/prd/12-*`、`docs/plan/21-*`：事实、采用需求、技术合同、验收与执行链。

## 本地开发环境后续实测

2026-10-28，本地业务库目标为 `glowloom`，由运行中配置读取数据库名和cluster identity并与loopback元数据核对；没有打印DSN或密钥。`subject_library_incremental.sql` 与 `volcengine_speech_incremental.sql` 执行成功（含正式视频音/视频引用角色）。`subject_voice_incremental.sql` 首次因旧库无 `media.storage_work_intent` 失败，所在事务回滚；不能把此前两份成功写成整批回滚或整批成功。未重放schema、未重写旧Bible内容。首次schema-only备份尝试因目标存储表不存在失败；随后成功导出subject结构留于本地migration证据，不能称有完整业务数据备份。

Chrome真实页面已显示独立主体导航/空库新建卡，打开统一设置与本地上传。上传自制FFmpeg测试图 `synthetic-reference.png` 并确认非真人内容后，服务返回不可用；界面保留文件与同一请求，未盲目重提，未得到就绪asset或保存成功。此前的404属于早期旧二进制观察，当前不再作为唯一环境结论。

现有库完整存储profile、route、work intent与object location前置未迁移，正式存储配置尚待指定；用户已被询问现有MinIO/COS/TOS配置名称/本地路径。代码的无配置不可用行为是保护，不能据此关闭AC04或宣称全量产品一致。存储迁移需沿现有所有者合同准备和隔离验证，不允许自动改用猜测桶或伪造验证证据。

Figma最终审计为73画面、3规格、1467个NODE目标/84唯一目标，缺失0。精确节点和实点/未实点边界见[设计报告](figma-report.md)及[对照矩阵](acceptance-matrix.md)。桌面多模型配置和手机仅重试音频保存分支已由原生播放器实点；其他新增恢复分支未全点，不能当完整浏览器接受。

## 历史阶段：曾采用的YML／ENV方案（已由单一.env替代）

用户确认TOS并自行填写；随后明确使用yml或.env、取消.local方案。最终已创建忽略提交的 `backend/config/object-storage.yml`，模板 `object-storage.tos.example.yml`；AK/SK引用 `.env` 的 `GLOWLOOM_TOS_ACCESS_KEY`/`GLOWLOOM_TOS_SECRET_KEY`。实际.env未读取或覆盖。此前本轮创建的空TOS JSON和空.local/object-storage目录已移除，新增额外挂载已从Compose撤回，不影响已有其他模块凭据配置。

Compose已有backend:/src挂载读取YML，无新配置加载器；`docker compose config --quiet`通过，`.gitignore`已保护实际YML。更新Compose后backend、frontend、canvas-collaboration均观察为running/healthy。本轮使用唯一Compose入口更新开发backend，没有发布或推送。

voice增量在最小旧库兼容调整后已于本地glowloom提交成功；它只创建独立job结构，缺存储时不自动创建profile/route。旧库缺结构、无route、真实权限错误传播及语音专项最终Race为4.991s通过，lint0issues/vet退出0。前端追加14项voice hook专项通过（含TOS未配置时不提交任务）；这14项与前述76项有重合，不能相加。

存储配置仍由用户填写，TOS没有实际连接/验证/激活。存储扩展脚本及隔离迁移测试不等于现有对象迁移完成；既有script裸key写入与普通role legacy-only组合根仍是正式TOS切换前置，不将模板填空宣传成全量存储切换已就绪。主体真实上传/保存、完整390、实际豆包付费仍保持未接受。

存储准备新增 `storage_ledger_expand_incremental.sql` 与 `tests/media/storage_ledger_migration_test.go`。专属新库 `glowloom_subject_storage_20261028_1409` 的 `go test -race ./tests/media -run 'TestStorageLedgerExpansion|TestTOSTemplate' -count=1` 通过，模板精简后配置专项另通过，media测试定向vet退出0；结束后测试事务回滚，非系统schema计数0。仅准备16空表/11nullable列及既有约束/权限，没有对业务库执行扩展、没有profile/route/证明种子或finalize。精确前置见[存储迁移准备](storage-migration-preparation.md)。

## 续轮：单一.env、无真实凭据的完整链路（2026-10-28）

用户最新要求覆盖此前YML/ENV方案：根 `.env` 为唯一当前入口；`.env.example` 预留TOS连接、AK/SK、稳定profile/revision、可选旧资源兼容字段及供应商PEM。此前本轮新建的空TOS YML与TOS YML模板已经撤回，Compose移除默认.local凭据挂载。为保持现有运行，原RSA密钥同一内容已迁入.env并取消FILE入口，未更换或显示密钥，原文件保留；TOS字段未改、未发起收费请求，实际.env未提交。

### 已观察的工程与实际隔离验证

- Subject全HTTP测试 `TestSubjectPublicAPIThroughNativeTOSUploadSaveAndExportWithoutCloudCredentials`：真实会话/空间、受限glowloom_app、真实PG、TOS官方SDK受控TLS，上传、同键回放、设置原子保存、固定版本重开、ZIP原字节及完整库筛选通过（Race 3.257s）。测试的临时路由明确标为受控fixture，不是实际云激活。
- 生成产物测试 `TestRoutedIngestRealPostgresAndNativeTOSPreserveFrozenWork` 加2个应用回归：原件和2个预览固定同一intent；逐输出与整批接受分开，重放不新增对象、默认profile变化不重定向；撤权仍拒绝。Race 2.725s。
- 时间轴测试 `TestRoutedTimelineExportRealPostgresNativeTOSAndFFmpeg`：真实FFmpeg渲染、原件/2预览、事务固定位置、完整work接受、重放、签名预览、未审核下载拒绝、审核后真实字节下载通过（Race 3.480s）。不是TOS云或部署接受。
- 已修实际受限角色错误：上传不再先INSERT无location再UPDATE媒体身份，而是在首条INSERT写物理位置；没有扩大不可变业务身份UPDATE权限。普通旧库无location写入仍沿明确旧资源兼容。
- 已修TOS HEAD无body的404分类，并新增空body的404/403/503回归。分类仅用于HEAD：依据火山官方[HeadObject](https://docs.volcengine.com/docs/torch-object-storage/head-object?lang=en)和[Go对象存在性判断](https://www.volcengine.com/docs/6349/1181805?lang=zh)，不将权限或服务异常视为对象不存在。
- `go test -race ./tests/app ./tests/config ./tests/objectstorage` 通过（15.438s/2.060s/1.502s）；专项TOS HEAD回归通过（1.428s）。`go vet`相关app/media/mediatool通过。
- 更新 `golang.org/x/net` 至0.62及其必要模块版本后，`govulncheck ./...` 完成：0个可达漏洞、0个导入包漏洞；模块层仍报告1个未调用漏洞。此结果替代早期HTML依赖可达漏洞记录，历史失败保留。
- 现有本地 `glowloom` 执行 `storage_ledger_expand_incremental.sql` 单事务成功。只扩展nullable列、合同和权限；未执行finalize/Activate，未创建真实profile/route/连接证据，不清理旧数据。Compose frontend已更新，backend更新后继续验实际页面。

### Figma及逐项差异

当时77可见画面+3规格，保留原73画面；1536个canonical NODE动作指向88个唯一节点，缺失0。新增4画面涵盖上传挂起与未知结果桌面/手机，播放器实点验证返回→继续同文件、X返回、手机滚动保留描述、不采纳保留原设置草稿。新节点2910:45348、2910:45549、2910:45688、2910:45855，见 `figma-upload-ledger.json`。代码与JM-SUB-01至15逐项映射见 `frontend-functional-audit.md`；竞品未观察的限额、多项边界与私有实现仍不推断为已证实。

当前最终工具、迁移及新运行页面结果已汇总于本文顶部；真实TOS与真实豆包执行在用户填配置后单独验收。

并行服务接入提交5f00ed54后再次验证当前组合：前端153文件1016项通过，66.65s；目录包Race在专属catalog任务库通过（命中有效测试缓存），未重跑无关的后端全量。后端全量结果仍对应此前完整工程快照；本轮其后生产改动均为并行catalog/frontend，Subject生产代码未再变动。

组合后Next生产构建再次退出0（/tmp/glowloom-subject-frontend-build-merged-final.log）；tsc退出0（/tmp/glowloom-subject-tsc-merged-final.log）；正式Go目录lint仍为0 issues（/tmp/glowloom-subject-formal-lint-merged-final.log）。这些补验未改测试超时或跳过失败。
