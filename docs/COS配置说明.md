# 腾讯云 COS 对象存储

Glowloom 使用腾讯云 COS 原生 Go SDK。项目原件、衍生文件、主体与头像、生成结果、剧本和导出物均通过后端对象存储访问。环境可以配置多个 COS 桶。每个对象仍按保存的 profile、桶和键读取，新默认写入桶只影响新任务。

## 配置

在项目根目录 `.env` 中设置以下值。示例桶、地区、凭据和 revision 需要替换为部署自己的值。已有环境更新时，`revision` 必须大于数据库中已记录的部署版本。

```dotenv
GLOWLOOM_STORAGE_ACCESS_KEY=
GLOWLOOM_STORAGE_SECRET_KEY=
GLOWLOOM_OBJECT_STORAGE_CONFIG='{"schema_version":1,"revision":"1","default_write_profile":"cos-main","profiles":{"cos-main":{"provider":"cos","mode":"read_write","endpoint":"https://glowloom-media-1250000000.cos.ap-guangzhou.myqcloud.com","signing_endpoint":"https://glowloom-media-1250000000.cos.ap-guangzhou.myqcloud.com","bucket":"glowloom-media-1250000000","region":"ap-guangzhou","credentials":{"source":"env","access_key_env":"GLOWLOOM_STORAGE_ACCESS_KEY","secret_key_env":"GLOWLOOM_STORAGE_SECRET_KEY"}}},"upload":{"mode":"relay","signed_url_ttl":600000000000,"session_ttl":86400000000000,"multipart_threshold_bytes":33554432,"part_size_bytes":8388608,"part_concurrency":3,"staging_retention":172800000000000},"access":{"preview_ttl":300000000000,"max_signed_url_ttl":900000000000},"limits":{"max_image_bytes":20971520,"max_video_bytes":524288000,"max_audio_bytes":104857600,"max_model_bytes":67108864,"max_document_bytes":20971520}}'
```

- JSON 里的时间单位为纳秒。凭据使用环境变量引用，预签名地址使用同一个原生 HTTPS 桶域名，访问端也要能连接该域名。
- 使用私有桶，或者在已批准复用的混合桶中为应用前缀确认独立的私有访问权限。用于不可变写入的桶必须从未开启版本控制。COS 的 `X-Cos-Forbid-Overwrite` 依赖这个条件。[腾讯云 PUT Object 文档](https://cloud.tencent.com/document/product/436/7749)
- 按角色授予目标桶和应用前缀所需的 HeadBucket、GetBucket、HeadObject、GetObject、PutObject 和 DeleteObject 权限。工作节点使用现有代理上传与预签名读取流程。

## 数据库结构与部署

1. 在旧版本仍能访问源桶时，按完整物理位置盘点存量对象、待清理对象及未完成的上传和任务。停止旧版本接单，等待工作完成或按原业务流程取消和对账。
2. 将仍需保留的对象复制到 COS，逐项核对原始字节、长度、SHA-256 和类型。使用经审阅的映射更新业务位置和关联记录。新位置必须使用独立 COS profile，既有 profile ID 和原始资源身份保持一致。
3. 确认旧存储上没有活跃对象位置、开放的工作意图或未过期的上传后，由数据库管理员核对实际结构与 [schema.sql](../backend/db/schema.sql) 一致，再继续部署。新空库直接使用该文件初始化。
4. 配置所有仍被引用的 COS profile，提高 revision 后再部署。检查真实 COS 的读取、上传、签名访问和历史素材恢复。项目的 CI 使用本地 COS 协议测试服务。真实桶探针需要专用测试桶并显式设置 `GLOWLOOM_TEST_LIVE_COS=1`。
