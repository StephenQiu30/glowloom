# TOS配置

只使用项目根 `.env`。填写位置及变量见根 [.env.example](../../.env.example)，本轮不新增YML、JSON或.local配置。

请填写五项：`GLOWLOOM_OBJECT_STORAGE_ENDPOINT`、`BUCKET`、`REGION`、`ACCESS_KEY`、`SECRET_KEY`（后四项同前缀）。`GLOWLOOM_OBJECT_STORAGE_PROVIDER=tos` 选择火山TOS原生SDK。endpoint填写服务origin，不填对象URL或桶名。

`PROFILE_ID`是稳定资源名；`REVISION`是配置版本。AK/SK轮换不改变历史对象身份。更换桶/地域/资源须采用新profile身份，不能复用旧ID改义。存储变量全空时其余服务仍可启动，不会发起云请求或收费生成。

可选 `GLOWLOOM_OBJECT_STORAGE_LEGACY_*` 只用于明确保留原资源的历史读取；不用则全部留空。系统不会将原存储的裸key按新TOS解释。原有provider未指定的MinIO五项配置继续兼容。

配置加载与资源启用分开：结构增量、旧对象归属/位置回填、真实SDK验证和正式切换依[主体实施计划](../../docs/plan/21-统一主体库与创作引用实施计划.md)执行。配置留空不阻塞无凭据代码、迁移和测试。TOS字段由用户填写；为移除默认.local挂载，已将原RSA密钥的同一内容迁入根.env，未更换密钥、未输出密钥或提交真实配置，也未伪造资源/通过证据。

TOS的AK/SK与豆包语音API Key独立；豆包Key仍经模型管理中的「豆包语音」连接保存。
