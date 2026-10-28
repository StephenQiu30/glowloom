# 存储台账、脚本位置、包导入与清理交付记录（2026-10-28）

仓库：`/Users/stephenqiu/Desktop/StephenQiu/Glowloom/glowloom-server`。本文件替代此前只做准备的迁移计划。配置唯一入口为统一 `.env`；存储子任务未读取真实配置或调用云服务。主任务本地结构扩展与既有RSA值迁入统一.env单独记录，不与隔离回归或云验收混为一谈。未提交或推送。

## 增量与边界

1. `backend/db/storage_ledger_expand_incremental.sql`：已有 canonical 合同的16张存储表、原11个nullable位置字段/复合FK、immutable身份/证据、必要权限。无profile/route/evidence数据种子，无NOT NULL，无业务bare-key删除。已测试旧结构、已存在media_storage_prerequisites_incremental最小前置、完整schema三个shape，均重复执行成功。
2. `backend/db/script_storage_locations_incremental.sql`：script_source(original/rich)、script_version(text/rich)、copy_object_intent(source/target)、copy_object_state(target)共7列nullable UUID及(location,key)FK；copy_snapshot.storage_intent_id。全量canonical已包含，父任务已获schema冻结信号。
3. `backend/db/storage_bootstrap_incremental.sql`：测量式空库证明表/不可变证据/只读runtime权限，不插入证明。空库库存包含18个物理owner列所在表、object_location、operation、各工作生产者（含subject.voice_job）共31张表。首次无legacy分支必须实际空库，不伪造MinIO原桶。
4. `backend/db/storage_ledger_finalize_incremental.sql` 与嵌入式 `internal/media/adapter/postgres/storage_finalize.sql` 字节相同：维护事务内检查原MinIO资源证明，或准确config+固定inventory+SHA的空库证明；全部位置已证明才改18列NOT NULL、移除bare-key全局唯一、保留script state→intent精确物理复合关系、改source/target同key限制为不同物理位置。无CASCADE。未知依赖/未证明行拒绝并整体回滚。
5. 首次DDL保守要求producer维护/drain；手工SQL需要无运行中的生产任务。Go operator会精确检查固定tuple，且已完成结构可以在正确证据lineage下无DDL幂等返回。切换后仍允许正确固定的在途任务；旧未固定目标的未完成任务拒绝cutover，必须用真实原资源/原owner恢复或收尾，绝不恢复时Pin新默认。此安全拒绝不是自动迁移旧未知产物。

## 正式维护顺序（本子任务没有对业务库执行）

- 各增量使用 `psql ... -X --single-transaction -v ON_ERROR_STOP=1 -f <精确脚本>`，先expand，再script_storage_locations，再storage_bootstrap。subject.voice_job需在bootstrap测量前已存在；其增量由root管理。
- 统一.env完整配置后：`glowloom storage validate`仅验证配置/输出公开摘要；`register`登记真实不可变资源。
- 真正空部署：`prove-empty`测量并持久证明；已有旧对象/旧任务部署：必须真实`bind-legacy`、按原桶字节SHA/size验证backfill、原producer收尾或固定，不能拿新TOS冒充原S3/MinIO。
- 维护/drain后 `finalize` 执行终态约束；私有桶合同accept-bucket、官方SDK实际probe、四角色相同配置/能力心跳、deployment证据全部满足后，才 `activate --expected-revision <精确CAS>`。
- `GLOWLOOM_STORAGE_MAINTENANCE_DSN`必须显式供应维护连接；无DSN不会执行变更。旧 `--config` / `--env` /YML入口已从operator移除。`config.Load().ObjectStorage`保留统一.env getter凭据解析；输出不含key。
- 后续无legacy的新profile切换使用首次guarded空库证明的不可变lineage，仍重新校验新配置的SDK/bucket/runtime/deployment，不要求数据库永远为空。

## 实现

- script：ObjectFact.LocationID保持JSON隐藏，语义manifest/hash不变；BeginWrite同事务Pin/Reserve，真实I/O使用staged物理ID；发布同事务Accept全输出集合。历史、source/version、copy映射、回放、recovery、对象state→intent均绑定UUID+key。RoutedStorage读取固定location或显式已证明legacy，绝不默认桶回退；写/删分别查真实write/removal fence。
- script copy：target独立位置、source物理身份去重、snapshot storage intent、真实SQL历史UUID、批量接受全输出。旧完成稿可回读；旧未知未完成稿明确拒绝cutover。
- package：source ZIP+原件+全部renditions在admission固定同一profile；package_object持久location，读回实际字节后Confirm，发布事务一次Accept全部集合。portable ZIP不带存储技术身份。永久回放与时区UTC归一；切默认后仍从原位置export。取消先核验全部现存字节，不确定写/删保留needs_reconciliation。
- purge：冻结和持久化每个原件/rendition location；同key不同profile退休判断改为物理相等；SDK始终按冻结ref操作。全体digest通过后才开始删，实际absence后才确认removed。普通权限不放宽。
- 通用cleanup：有写fence保留10分钟drain；整namespace从未Begin SDK写（所有write_until=NULL）的纯保留槽允许立即确认缺失。只跳过当前cancelled work自身未发布target自pin，其他owner/正式结果仍挡。script取消等待drain时保留提交取消时钟，由SDK前ReadRemovable强制阻止提前删除，避免rollback使等待永不累计。
- accepted私有copy清理：只允许相同snapshot/intent、target仍copying、workspace job处于cleanup、无其他owner引用且write_until已自然过期；accepted intent不改状态，只记location delete_pending，SDK真实absence后deleted。active target/非cleanup/foreign location拒绝。调用者仍须持有workspace当前协调fence；SQLfixture不冒充Temporal完整跑通。
- immutable profile/config/legacy/evidence不再FOR SHARE（runtime只有SELECT/INSERT，无UPDATE）；mutable route/intent/location原有锁保留。

## 组合根交接

已新增：
- `mediawire.ProvideRoutedMediaPackages(db, registry, uploads)`
- `mediawire.NewRoutedMediaPurgeWorker(db, registry)`
- `scriptwire.ProvideScriptObjectLocations(tx)` + SourceStore/ProjectCopyStore.WithObjectLocations

Root/core负责共享publicAPI/roles调用、native TOS/config及managed ingestion、transfer/project-copy全协调器、mediatool。本子任务没有写root的ingest或mediatool业务文件。共享insertTransferAsset/transferSourceFile物理字段由core完成。本子任务只补library_command lockLibraryItems遗漏的location加载。

## 已执行验证

命令均在backend运行，DSN仅指task-owned loopback隔离库。

- `GLOWLOOM_TEST_STORAGE_MIGRATION_DSN=<glowloom_subject_storage_20261028_1409> go test -race ./tests/media -run 'TestStorageLedgerExpansion|TestStorageEmpty|TestStorageFinalization|TestStorageNative' -count=1`
  - 三shape/重放不激活；空库真实测量；错config拒绝；测量后插入work拒绝；18列终态及script物理FK不可缺；operator/SQL脚本同字节；无legacy首次guarded activation及后续revision、保留fixed work；无虚构MinIO。最新Green 5.165s，最后代码复测另向root报告。
- `GLOWLOOM_TEST_PACKAGE_LOCATION_DSN=<glowloom_subject_storage_package_20261028_1538> go test -race ./tests/media -run 'TestPackageLocated|TestStorageAcceptedPrivateCopy|TestPurgeUsesFrozenLocation' -count=1`
  - 真实glowloom_app+严格18列NOTNULL+官方COS SDK受控HTTP+FFmpeg图片两缩略图：ZIP/全部media接受、receipt回放、切默认后export、recycle、purge确认旧profile真实删除且新profile零触达。
  - 失败部分写入不发布；取消遇坏SHA拒绝删除；真实bounded fence自然过期+旧取消fixture后只清理自有未发布集合，absence确认后所有rows removed/deleted，未发布任何asset。
  - accepted private copy 非cleanup/active target/foreign location拒绝；当前private cleanup可删除真实SDK对象并Confirm；accepted work保持不可变。
  - 最新合并Race Green 7.001s。
- `GLOWLOOM_TEST_SCRIPT_LOCATION_DSN=<glowloom_subject_storage_script_20261028_1501> go test -race ./tests/script -run 'TestScriptLocated|TestScriptCopyRemaps' -count=1`
  - 真实PG权限下Source admission/历史/回放/新默认新写/整集合Accept；语义manifest不含UUID；取消clock在owner事务提交而SDK removal仍被drain阻止。最新Green 2.329s。使用located byte fixture；独立官方SDK固定路由测试见下一项。未宣称完整script workspace-copy Temporal/旧库历史全矩阵实跑。
- `go test -race ./tests/media -run 'TestScriptRoutedSDK' -count=1`：已Green 1.893s，官方COS受控SDK固定老profile读写、新profile零写、裸key写与无证明legacy拒绝。
- `go test -race ./tests/app -run TestStorageOperator -count=1`：Green 1.985s，统一env验证/不输出secret/退役路径拒绝/缺维护DSN拒绝。
- `go vet ./internal/app/script ./internal/app/storageoperator ./internal/media/... ./internal/script/...`：Green。
- gofmt / scoped `git diff --check`：Green。早期PATH未发现goimports/golangci-lint，随后在`/Users/stephenqiu/go/bin/`发现并已正式使用；其最终lint结果见末节。root负责全应用govuln与并行模块综合回归。

## CI与状态

本子任务只给现CI新增两个env与空DB创建：`GLOWLOOM_TEST_PACKAGE_LOCATION_DSN=.../glowloom_subject_storage_package_ci`、`GLOWLOOM_TEST_SCRIPT_LOCATION_DSN=.../glowloom_subject_storage_script_ci`。保留原有全部CI项。它们在CI不会靠缺DSN跳过；CI远端尚未执行。

Root反馈已执行本地业务glowloom的expand单事务成功，日志`/tmp/glowloom-subject-local-storage-expand.log`；没有finalize/activate，没有profile/route/evidence事实。该操作是root执行，不是本子任务对业务库的隔离测试。主任务另已单事务执行script_storage_locations与storage_bootstrap两个结构增量；同样未填证明、未finalize/activate。

真实TOS资源、凭据、私有桶合同/云SDKprobe/真实云费用验收仍须用户填写统一.env并按正式步骤验证；不以测试profile、受控SDK或空配置宣称激活/云验收成功。本子任务不读取这些凭据。

## 最终冻结复测

- 全组合定向媒体/迁移/SDK/清理Race已Green8.718s；随后精确FK所引用列（必须object_location.id/object_key）负向测试补齐，迁移/empty/final/native子集最终Race Green18.488s。该测试证明“FK存在且指向object_location，但把key错接bucket”也不能通过终态或再次finalize。
- package runtime RegisterProfiles正向通过，immutable storage_profile UPDATE仍为42501；package整批/取消子集最终Race4.232s。
- script source+copy remap最终Race2.300s，新增同key不同LocationID不会去重合并，target位置不继承sourceID且语义SHA保持。
- operator已使用core新增Config.StorageMaintenanceDSN（同viper getter，从统一.env/GLOWLOOM_ENV_FILE读取），无普通DBDSN回退；验证fixture-env-file独立维护连接被实际消费但不输出。最终三项operator Race2.105s。
- 最终相关Go vet与全树git diff --check均Green；所有本子任务代码与未执行finalize脚本已冻结。
- 未提交/推送；现工作区仍含本次及并行修改。Figma未在本轮变更。

## 正式 lint 收尾

根任务正式lint发现的本owner问题均已修正：无效args/predicate赋值、测试service覆盖前构造、routed导出方法/构造注释、copy局部变量名、wire嵌入selector、trailing blank及goimports。已运行绝对路径goimports并复查无输出。

`/Users/stephenqiu/go/bin/golangci-lint run ./internal/script/... ./internal/app/storageoperator/... ./internal/media/adapter/objectstorage/... ./internal/media/adapter/postgres/... ./tests/script/... ./tests/media/... ./tests/app/...` 最终exit0、`0 issues.`（core已并行完成其两个内建copy名修复）。我没有改共享root ctor或其正在写的文件。根任务将重跑排除ignored artifacts的正式全应用lint；本地scope lint通过不等于远端CI已绿。

## 原生存储用量 consumer（追加完成）

- 新增`ProvideRoutedMediaStorageUsage(database, registry, *config.ObjectStorageConfig)`；root负责publicAPI接线。配置验证失败或无registry时Unavailable，不回退裸key/currentdefault。
- `LibraryStore.StorageUsageLocations`是正式media owner port，在同scope授权/快照事务里读取精确`personal/org/actor/`或`projects/id/`命名空间的全部有效ledger位置，以及同library的media-owned package ZIP位置；不跨模块查询脚本/转录业务表。现有media/transfer/copy位置验证仍保留，未迁移的live NULL FK拒绝。
- 每个配置历史可读profile均LIST整个scope base，覆盖五种media以外的script/transcriptions与没有ledger的实际孤立对象；同key跨profile独立计数。额外比对配置资源SHA与数据库storage_profile，拒绝同名profile重定向到另一endpoint。
- native Registry LIST最低安全命名空间从3层slash改为2层，允许应用已授权的`projects/<UUID>/`；路径/具体profile/bucket与上层scope校验保留。legacy Client合同不改。
- 统计真实物理占用：retained trash仍占用；deleted ledger真实缺失不计；跨scope不计。已知unknown位置即使LIST遗漏仍会STAT；任一profile LIST或STAT错误整体Unavailable、无subtotal。没有把不确定写入、失败查询、无配置当成zero。
- 新增真实glowloom_app PostgreSQL + 官方COS SDK受控LIST/STAT/PUT集成`TestNativeStorageUsagePGSDKIncludesHistoricalProfilesWholeScopeAndPackageArchive`：旧readonly/newdefault、同key两份、未知LIST遗漏、孤立script、真实package ZIP及recycle trash、deleted缺失、foreign actor/project、完整project转录/脚本前缀、未知STAT 403/历史LIST403全失败，以及相同profile ID重定向资源零SDK触达。
- `GLOWLOOM_TEST_PACKAGE_LOCATION_DSN=<既有task-owned packageDB> go test -race ./tests/media -run 'TestNativeStorageUsagePGSDK|TestRoutedStorageUsage|TestMediaStorageUsageCounts|TestMediaStorageUsageRejects' -count=1` Green3.804s。
- `go test -race ./tests/objectstorage -run 'Test.*Inventory|Test.*ListMetadata|Test.*Registry' -count=1` Green1.211s；相关media/app/SDK Go vet Green。使用同已接CI package DSN，无新增数据库或配置依赖。

原生usage最后追加边界：正式media与ledger查询均按精确(location_id,object_key)排除已证明purge removed或location deleted的retired对象。这样旧purge保留accepted location的历史记录也不会错误要求一个已经停用的历史profile。追加真实system文本fixture（不冒充upload/AIGC）经过正式library recycle、PurgeWorker+SDK真实删除，再禁用旧profile，同授权scope用量准确0；保留trash的前一个case仍真实占用不变。最终usage组合Race Green7.065s，代码冻结，后续由root运行全scope门禁。
