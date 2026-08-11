# MinIO 对象存储端口验收

- 状态：accepted（PT-STO-001；PT-STO-002 conditional/not-applicable）
- 日期：2026-08-04
- 验收基线：`main@8ae6685` 工作树；本实现与证据在同一后续提交进入 `main`
- 对应需求：[缓存、对象存储与任务调度需求](../../requirement/013-缓存存储与任务调度需求.md)
- 对应设计：[缓存、对象存储与任务调度详细设计](../../design/模块设计/014-缓存对象存储与任务调度详细设计.md)
- 对应产品任务：[PT-STO-001](../../prd/009-剪辑交付与平台保障PRD任务.md)
- 对应工程计划：[DEV-S0-04](../../plan/000-MVP全栈实施总计划.md)
- 验收边界：接受本机已配置私有 MinIO 的八项 ObjectStoragePort、类型化错误/元数据、完整性、超时、真实直传下载和媒体恢复链；不接受阿里云 OSS、生产地域、RAM 权限或跨云迁移

## 1. 验收结论

1. `ObjectStoragePort` 严格只有 ensure_bucket、presign_upload、presign_download、stat、put、copy、stream、delete 八项操作。multipart 由 `put` 内 MinIO SDK 自动执行，没有向媒体业务暴露 upload-id、part 或厂商 DTO。
2. `stat` 返回不可变 `StorageObjectMetadata(size_bytes, content_type, etag)`；ETag 只用于诊断。大于 5 MiB 的真实对象产生带分段标记的 multipart ETag，且不等于平台 SHA-256。
3. 上传完成与位置迁移共用 `verify_object_integrity`，先核对 size/MIME，再按 1 MiB 分块流读计算 SHA-256。不匹配统一为 `StorageIntegrityMismatch`；缺对象、拒绝访问和瞬态不可用分别使用 `StorageObjectNotFound`、`StorageAccessDenied`、`StorageUnavailable`。
4. MinIO 同步 SDK 经有界 `CapacityLimiter` 离开事件循环，每次操作使用 0.1～30 秒有界超时并允许放弃取消后的线程结果。对象 key 和预签名 TTL 在网络前验证；响应错误不向业务暴露 SDK 类型、endpoint、凭据或预签名 URL。
5. `ensure_bucket` 幂等创建并拒绝允许匿名读的 bucket policy。真实已有对象的无签名 GET 返回 401/403，短期预签名 PUT/GET 均实际传输正确字节；服务端 copy、分块 stream 和重复 delete 均通过。
6. 使用错误访问身份时稳定返回 access-denied，连接不可达 endpoint 时稳定返回 unavailable。二者不再被混为同一错误，也未读取、输出或写回本地 `.env`。
7. 同大小但不同 SHA-256 的真实直传返回 422、UploadSession=failed、MediaVersion 数量为 0；原 one-off 到期事实随后创建 Task 并由真实 MinIO delete 收敛孤立临时对象。
8. MinIO 构造从业务模块移动到 integrations composition root；架构测试固定业务模块不得导入 SDK 或读取 `minio_endpoint/access_key/secret_key`。

## 2. Red → Green 证据

| 阶段 | 命令与真实结果 |
| --- | --- |
| Red | `make contract-minio` 首次运行在收集阶段失败：`ImportError: cannot import name 'StorageAccessDenied'`，证明原端口没有已设计的访问拒绝语义；既有错误凭据也只会落入 unavailable。 |
| Green | 完成端口事实、统一 helper、MinIO 安全翻译和真实半失败后，`make contract-minio` 为 6 passed（1.20 秒）；没有删除或放宽任一新增断言。 |
| 兼容回归 | 媒体 API/位置迁移/Worker/健康与配置/架构定向回归 31 passed；Ruff 通过，Pyright 为 0 errors/0 warnings/0 informations。 |

## 3. 真实依赖与故障证据

- `make contract-minio` 使用本机已配置 MinIO，真实覆盖私有匿名拒绝、预签名 PUT/GET、HEAD、服务端 copy、>5 MiB multipart、流读、完整性、删除幂等、错误凭据、不可达 endpoint、readiness 和上传半失败，共 6/6。
- `make contract-media-stack` 使用真实 PostgreSQL、隔离 RabbitMQ、本机 MinIO 和 ffprobe，上传探测、到期清理、位置迁移/回滚/退役共 4/4（1.78 秒）。
- 默认 `make check` 不隐式启动外部依赖，其 18 个 skip 均带显式开关；其中 MinIO/媒体栈 skip 已由上述两条显式真实命令补证，其余 Provider/性能等边界没有被本 Acceptance 冒充通过。
- 错误凭据测试只生成无效测试身份；断言和错误消息都不包含本机真实 access key、secret、endpoint 响应正文或预签名 URL。

## 4. 全量发布门禁

- `make check` 全绿：Ruff、Pyright、ESLint、TypeScript、后端 241 passed/18 个显式外部或性能开关 skipped、前端 17 文件/56 tests、`pip check`、Next.js 16.2.12 生产构建及 development/production Compose config 均通过。
- `DEEPSEEK_API_KEY='' ARK_API_KEY='' LANVERSE_E2E_BACKEND_PORT=8002 LANVERSE_E2E_FRONTEND_PORT=3001 make e2e` 为 9/9（1.2 分钟）。无 Provider Key 时 Seedream/Seedance 继续 fail closed，MinIO 改动没有制造 Ollama、Ark SDK、模拟 Provider 成功或伪费用。
- `.env.example` 与 `.env.production.example` 新增语义化 `STORAGE_OPERATION_TIMEOUT_SECONDS`，并继续精确保留 `DEEPSEEK_API_KEY=`、`ARK_API_KEY=` 空占位。新增源文件和测试均按 `object_storage`、`minio_port`、`media_minio_flow` 等职责命名，没有 `s1/s2` 阶段代号。

## 5. 条件任务与残余风险

PT-STO-002 明确为 `conditional/not-applicable`：尚未选定生产 OSS、地域、RAM 策略和真实账号，因此本次 MinIO 结果不能证明 OSS SDK V2、虚拟主机请求、分片 ETag、跨存储迁移或生产发布可用。若部署目标决定使用 OSS，必须先新增独立 adapter，并在真实目标环境复跑同一端口契约和逐 MediaVersion 迁移门禁。
