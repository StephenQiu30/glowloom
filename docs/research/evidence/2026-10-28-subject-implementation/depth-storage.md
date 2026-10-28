# 深度工具物理存储消费验收

日期：2026-10-28。范围为mediatool depth现有任务的统一物理位置消费；不改变DepthArtifactDigest的JSON合同、原生推理合同或其他业务域表所有权。实际VDA模型推理未在本轮运行。

已实现：

- Create与Retry在业务事务中用`StorageOwner("depth", project, job, attempt)`固定当前存储意图；work ID由job和`storage-attempt/<attempt>`稳定派生。空配置拒绝入队，永久命令回放保留原结果。
- Claim通过DerivedMedia.Sources重验确切asset/project/revision/hash/key/size/MIME后，把真实ObjectLocation放入内部`SourceAssets`（json:-）。worker每次执行单独Open路由对象，拒绝共享可变默认存储；Open失败仍Finish结束已取得的执行claim。
- FreezeArtifact首次INSERT depth_object即写object_location_id，不依赖以后UPDATE绑定；物理ID不进入永久JSON digest。
- Commit比对三份已验证对象的完整size/hash/MIME、初次绑定ID和冻结profile/bucket/key，再由注入的媒体storage port接受完整owned集合并把physical IDs交给DerivedMedia.Store。缺一项、额外owned项或物理事实不符均不能发布。
- NewRoutedDepthQuery仅按已接受的正式ObjectLocation签名与读取，使用SDK实际expiry；未人工review仍不可下载。nil location只允许显式legacy依赖。
- 已接受的pending产物不能由worker补偿删除。取消遇此边界时保留非reference状态并等待reconciliation；不宣称删除完成，后续由媒体purge所有者处理。

精确接线：`internal/app/media/media_depth_wiring.go`新增`ProvideRoutedMediaDepthStore(db, enabled, scopes)`和`ProvideMediaDepthWorkerRouted(db, registry, legacy, cfg)`；原ctor保留给明确legacy消费者。publicAPI/roles由父任务和配置所有者接入。生产depth adapter没有直接访问media业务表，均使用注入owning application ports。

## 实际测试

在backend执行，使用父任务提供的自有空schema隔离数据库`glowloom_subject_alignment_env_20261028_a1806a`，连接仅loopback；业务fixture在独立外层事务结束rollback，不改变既有tos-frozen profile定义。

```sh
go test -race ./tests/media -run '^TestDepthRouted' -count=1 -v
go vet ./internal/mediatool/... ./internal/app/media/...
```

三个顶层测试通过，最终定向race包耗时2.560秒；定向vet退出0。测试包括：空配置拒绝、永久入队回放、SourceAssets真实位置、FFmpeg生成源视频的SDK读回、初次物理绑定、部分对象/额外owned对象拒绝、完整接受、JSON digest不变、pending禁下载、accepted取消不伪删、review后精确SDK下载，以及切目标后Retry新attempt不改旧namespace。另有worker路由失败释放claim及预览真实expiry/非accepted拒绝单元回归。

`controlledGeneratedTOS`使用TOS官方SDK连接本地受控TLS server，仅example.invalid映射至loopback，合成凭据。输出三对象为明确测试字节，native receipt为受控元数据；该测试证明真实PG事务、原生SDK请求与字节完整性，不证明VDA推理或真实云服务。没有真实付费或云凭据调用。

本机未发现goimports、golangci-lint、govulncheck可执行文件；未将其报告为通过。gofmt已运行。未启动或重启服务，未暂存、提交或推送。旧深度原生模型/历史全套集成测试未在本轮全量执行。
