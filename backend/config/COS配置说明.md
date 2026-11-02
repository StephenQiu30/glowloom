# 腾讯云 COS 对象存储配置说明（保留 MinIO）

## 1. 方案概述

织光 (Glowloom) 当前项目的生产与标准环境存储方案采用**腾讯云 COS (Cloud Object Storage) 对象存储**作为主存储方案，同时**完整保留 MinIO 对象存储支持**。

### 为什么采用此双模方案？
- **主存储采用腾讯云 COS**：
  - 具备国内高可用、高吞吐、低延迟对象存取能力，具备丰富的 CDN / 对象加速网络集成。
  - 原生 SDK (`github.com/tencentyun/cos-go-sdk-v5`) 严格保证条件写（`X-Cos-Forbid-Overwrite`）与 SHA-256 元数据记录，防止同一 key 并发冲突与静默覆盖。
- **同时保留 MinIO**：
  - **本地开发开箱即用**：开发环境（如 Docker Compose / 本地进程）可直接使用轻量 MinIO 服务，无需公网环境与云账号账单。
  - **历史资产平滑过渡**：存量资产、旧任务及历史对象仍按原始 MinIO 位置寻址和读取，不会因主存储切换为 COS 而丢失历史文件。
  - **架构解耦**：系统通过 `objectstorage.Registry` 统一抽象层隔离底层云厂商协议，支持多 Profile 路由（COS 主写主读 + MinIO 历史只读）。

---

## 2. 环境变量配置指南

所有配置统一在项目根目录 `.env` 中维护，由统一配置系统 `config.Load()` 读取（测试或独立环境可通过 `GLOWLOOM_ENV_FILE` 注入）。

### 2.1 腾讯云 COS 主存储配置

```dotenv
# ==========================================
# 腾讯云 COS 主存储配置
# ==========================================
GLOWLOOM_OBJECT_STORAGE_PROVIDER=cos

# 腾讯云 Bucket 必须包含 APPID 后缀，例如 glowloom-media-1250000000
GLOWLOOM_OBJECT_STORAGE_BUCKET=glowloom-media-1250000000

# 对应地域，例如 ap-guangzhou, ap-shanghai, ap-beijing
GLOWLOOM_OBJECT_STORAGE_REGION=ap-guangzhou

# 桶域名 Origin，必须与 Bucket 及 Region 保持严格一致
# 公网 Endpoint: https://<Bucket>-<APPID>.cos.<Region>.myqcloud.com
# 内网 VPC Endpoint: https://<Bucket>-<APPID>.cos-internal.<Region>.tencentcos.cn
GLOWLOOM_OBJECT_STORAGE_ENDPOINT=https://glowloom-media-1250000000.cos.ap-guangzhou.myqcloud.com

# 腾讯云 API 密钥 (SecretId / SecretKey)
GLOWLOOM_OBJECT_STORAGE_ACCESS_KEY=AKIDxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
GLOWLOOM_OBJECT_STORAGE_SECRET_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

> **遵循极简与非敏感信息不入 ENV 原则**：内部 Profile 标识、版本 Revision 等非敏感常量由代码自动默认推断（如 `cos-main`、`env-v1`），无需在 `.env` 中冗余配置。

### 2.2 保留 MinIO 配置（两类使用模式）

#### 模式一：双模共存（COS 作为主写主读，MinIO 作为历史 Legacy 只读）
当线上或灰度环境从原 MinIO 升级到 COS 后，通过配置 `GLOWLOOM_OBJECT_STORAGE_LEGACY_*` 保留对旧 MinIO 桶的访问权：

```dotenv
# ==========================================
# 保留的 MinIO 历史资源连接 (只读)
# ==========================================
GLOWLOOM_OBJECT_STORAGE_LEGACY_PROVIDER=minio
GLOWLOOM_OBJECT_STORAGE_LEGACY_ENDPOINT=http://127.0.0.1:9000
GLOWLOOM_OBJECT_STORAGE_LEGACY_BUCKET=glowloom-media
GLOWLOOM_OBJECT_STORAGE_LEGACY_REGION=us-east-1
GLOWLOOM_OBJECT_STORAGE_LEGACY_ACCESS_KEY=minioadmin
GLOWLOOM_OBJECT_STORAGE_LEGACY_SECRET_KEY=minioadmin
```

> **注意**：系统不会将原 MinIO 中的历史对象 key 误指到新 COS 桶中；所有历史对象继续由数据库中的 `object_location` 记录其原始 Profile 及 Bucket，保证读取和引用的连续性。

#### 模式二：本地单机脱机开发（无需腾讯云账号）
如果在无外网或本地单机调试场景下，直接使用本地 MinIO：
只需将主 `PROVIDER` 设置为 `minio`（或保持旧版无 PROVIDER 5项基础配置）：

```dotenv
GLOWLOOM_OBJECT_STORAGE_PROVIDER=minio
GLOWLOOM_OBJECT_STORAGE_PROFILE_ID=minio-main
GLOWLOOM_OBJECT_STORAGE_REVISION=local-v1
GLOWLOOM_OBJECT_STORAGE_ENDPOINT=http://127.0.0.1:9000
GLOWLOOM_OBJECT_STORAGE_BUCKET=glowloom-media
GLOWLOOM_OBJECT_STORAGE_REGION=us-east-1
GLOWLOOM_OBJECT_STORAGE_ACCESS_KEY=minioadmin
GLOWLOOM_OBJECT_STORAGE_SECRET_KEY=minioadmin
```

---

## 3. COS 权限与安全建议

1. **CAM 最小权限原则**：
   - 为织光系统创建独立的 CAM 子账号或角色，避免使用腾讯云主账号 API 密钥。
   - 策略权限仅授予目标 Bucket 的以下操作：
     - `name/cos:PutObject`（包含元数据设置）
     - `name/cos:GetObject`
     - `name/cos:HeadObject`
     - `name/cos:DeleteObject`
     - `name/cos:GetBucket`（用于列取清单）
2. **防覆盖安全机制**：
   - 织光后端在每次写入时均会带上 `X-Cos-Forbid-Overwrite: true`，存储桶无需开启服务端多版本，底层自动拦截重复 key 覆盖。
3. **私有读写权限**：
   - Bucket 访问权限必须配置为「私有读写」，前端预览与下载通过后端生成的有时效性预签名 URL (`PresignedGetObject`) 访问，默认有效期受内部 AccessPolicy 保护（5~15 分钟），杜绝公网越权盗链。

---

## 4. 运维命令与校验流程

使用内置维护工具 `glowloom storage`（需要 `GLOWLOOM_STORAGE_MAINTENANCE_DSN`）：

1. **校验配置语法与 Origin 一致性**：
   ```bash
   go run ./cmd/glowloom storage validate --env=.env
   ```
2. **测试 Bucket 连通性、探针读写与权限**：
   ```bash
   go run ./cmd/glowloom storage probe --env=.env --profile=cos-main
   ```
3. **安全激活新 Profile 为默认写入目标**：
   ```bash
   go run ./cmd/glowloom storage activate --env=.env --profile=cos-main
   ```
4. **历史数据迁移与回填验证**：
   - 对于从纯 MinIO 迁移至 COS 的已有数据库，执行 `bind-legacy` 并对账 SHA-256 和大小，在完全迁移前不删除旧桶数据。
