# Provider 数据与凭据安全底座验收

- 状态：accepted（仅 DEV-AIP-01 数据与凭据安全底座；PT-AIP-002、PRD-011 完整功能与 DEV-AIP-02～07 均未接受）
- 日期：2026-08-12
- 验收基线：`main@bfa7630` 干净工作树起点；Red 契约 `b988500`、Green 实现 `ce360d2`
- 对应需求：[REQ-014 AI 提供方配置与启用需求](../../requirement/014-AI提供方配置与启用需求.md)
- 对应设计：[DES-009 AI 提供方配置与启用设计](../../design/009-AI提供方配置与启用设计.md)、[数据库表与数据生命周期详细设计](../../design/模块设计/011-数据库表与数据生命周期详细设计.md)
- 对应产品任务：[PRD-011 PT-AIP-002 安全事实子集](../../prd/011-AI提供方配置与启用PRD.md)
- 对应工程计划：[PLAN-011 DEV-AIP-01](../../plan/011-AI提供方配置与启用执行计划.md)
- 验收边界：只接受四类 Provider ORM 事实、租户/唯一/版本约束、AES-256-GCM 凭据端口、部署 Settings 和显式 Model 注册；没有 HTTP API、页面、真实 Provider 调用或数据库 Binding 运行时解析

## 1. 验收结论

1. `ProviderConnection`、`ProviderCredentialVersion`、`ProviderBinding`、`ProviderHealthCheck` 已作为 production 纵向能力包显式注册进唯一 SQLAlchemy Metadata；没有通用 Secret、EndpointPool、Plugin 或第二套 Task 事实。
2. CredentialVersion 只持久化 `key_id`、12-byte 随机 nonce、ciphertext、16-byte GCM auth tag 和独立 HMAC-SHA256 fingerprint。主密钥与 fingerprint key 使用两个独立 URL-safe Base64 32-byte 部署 Secret；默认均为空，凭据构造返回稳定 `provider credential store unavailable`，非敏感 Settings/Model 加载不受影响。
3. AES-GCM AAD 固定 `workspace_id + connection_id + credential_version_id + version`。单元测试证明同一明文两次加密 nonce/密文不同、可正确解密；密文/tag 任意 bit flip、跨 Workspace/Connection/CredentialVersion/version 重放全部返回通用完整性错误，不把输入值写入异常。
4. 真实 PostgreSQL 已拒绝 CredentialVersion 跨 Workspace 引用、Binding/HealthCheck 跨 Workspace 引用、未注册 Capability config_version、同 Connection 双 `current` CredentialVersion 和同 `(workspace_id,usage_type)` 双 `active` Binding。Binding 复合外键同时固定 Connection、CredentialVersion、Workspace 与 Capability 版本。
5. 数据库 sentinel 使用每次测试随机生成的值；只查询可读密文投影并得到 0 个明文命中。若失败，测试只报告 sentinel 的 SHA-256 短摘要。四张表也没有 `api_key`、`credential`、`plaintext`、`request_body` 或 `response_body` 列。
6. `ModelCapability.kind` 最小增量支持 `text`，并增加 `(id,config_version)` 唯一候选键，使 `script_structure` Binding 能引用一个不可漂移的 Capability 版本；既有 image/video 契约保持原值。

## 2. Red → Green 证据

| 阶段 | 命令与真实结果 |
| --- | --- |
| Red | `.venv/bin/python -m pytest tests/unit/production/providers/test_credentials.py tests/integration/production/providers/test_provider_data_integrity.py -q` 在收集阶段出现 2 个 `ModuleNotFoundError: app.modules.production.providers`，证明原系统没有 Provider 数据/凭据底座。 |
| Credential/Settings Green | `.venv/bin/python -m pytest tests/unit/production/providers/test_credentials.py tests/unit/test_config.py -q` 为 29 passed；其中凭据测试覆盖 round-trip、随机 nonce、fingerprint、四项 AAD 身份、cipher/tag bit flip、错误零明文、未知 key_id、缺失/非法部署 key。 |
| PostgreSQL/架构 Green | `.venv/bin/python -m pytest tests/integration/production/providers/test_provider_data_integrity.py tests/architecture/test_runtime_model_registry.py tests/architecture/test_document_traceability.py -q` 为 40 passed；数据库为项目明确配置的真实 `lanverse_test` PostgreSQL。 |
| 静态与依赖 | `.venv/bin/ruff check app tests` 全绿；`.venv/bin/pyright` 为 0 errors/0 warnings；`.venv/bin/python -m pip check` 为 `No broken requirements found`。 |
| 全量回归 | `.venv/bin/python -m pytest` 为 297 passed、24 skipped；skip 均是既有 ffprobe/Redis/MinIO/RabbitMQ/性能显式开关，本增量不依赖这些外部条件。 |

## 3. 依赖与部署事实

- 认证加密依赖固定为 `cryptography==50.0.0`，已同步 `pyproject.toml`、运行锁和开发锁；锁文件由项目固定的 pip-tools 命令重生，不手改生成产物。
- 版本与 Python 3.11 wheel 从 [PyPI cryptography 官方发布页](https://pypi.org/project/cryptography/) 复核；AESGCM 的 nonce/AAD/认证失败语义从 [cryptography 官方 AEAD 文档](https://cryptography.io/en/latest/hazmat/primitives/aead/) 复核。
- `.env.example` 与 `.env.production.example` 只新增 key ID 和空 secret 字段，不含可用密钥。当前部署必须自行注入两个独立随机值；主密钥缺失不会阻止只读应用启动，但所有未来凭据操作必须 fail closed。

## 4. 明确未接受项与残余风险

DEV-AIP-01 没有 Repository、Service、Router 或 UI，因此尚未证明 owner RBAC、write-only OpenAPI、Connection CRUD、应用层 CredentialVersion/HealthCheck 只追加、revision 并发、Audit 原子性、归档 blocker、模型发现、SSRF、健康检查、Binding 激活事务或运行时快照；这些由 DEV-AIP-02～05 继续实现，PT-AIP-002 仍未 accepted。

当前仍只有空库 `metadata.create_all()` 路径。真实测试证明新建 PostgreSQL 的约束，不证明任何已有数据库能安全增加这些表或给既有 `ModelCapability` 补候选键；存量环境必须先接受独立 DDL、备份、演练和回滚方案。

生产 KMS/Secret Manager 产品、密钥轮换命令和恢复演练尚未选择或实现。当前单 key_id cipher 对未知/旧 key_id 明确 fail closed；DEV-AIP-05 实现逐条重加密前不能宣称主密钥轮换可用。

没有读取或迁移现有 `DEEPSEEK_API_KEY`/`ARK_API_KEY`，也没有真实 Provider 网络调用、模型准入、费用或页面结果。DeepSeek 数据库单向切换仍由 DEV-AIP-06 和真实账号/额度门禁负责，Ark/Seedream/Seedance 继续受 D-004 阻断。

## 5. 后续迁移闭环（2026-08-13）

本记录验收时“只有空库 `create_all()`、未证明存量迁移”的风险已由 [Acceptance 028](./028-Alembic历史旧库迁移与恢复验收.md) 关闭：真实 Provider 引入前 38 表/19 行本机旧库经仓库外备份、恢复副本、历史 baseline 接管和独立 Provider revision 升级到 42 表 head，旧表内容 hash 守恒。该后续证据只关闭数据迁移风险，不扩展本记录尚未接受的 Provider API、RBAC、运行时解析或真实调用范围。
