# Glowloom · 织光

织光围绕图片、视频、声音、主体、素材和画布组织视觉创作。仓库为 `StephenQiu30/glowloom-server`，包含 `frontend/`、`backend/` 和 `docs/`；命令从对应目录执行。

## 当前需求

本次重点是完整用户体系：**邮箱 + 密码登录，注册时验证邮箱**，个人创作者与团队创作者共用账号，平台管理员与团队管理员各有管理范围。范围覆盖认证恢复、个人中心、平台用户管理、团队成员、权限隔离与生命周期；需求规格包含功能性及非功能性需求和完整追踪。现有代码仍使用登录名；本轮先交付需求与设计，没有执行认证改造或数据迁移。

- [文档入口](docs/README.md)
- [用户系统 PRD](docs/requirement/PRD-用户系统.md)
- [详细需求与验收](docs/requirement/01-用户系统需求.md)
- [系统设计](docs/design/用户系统设计.md)
- [落地计划](docs/plan/用户系统落地计划.md)
- [全产品能力范围](docs/requirement/00-产品范围.md)

## 开发与协作

前端开发可在 `frontend/` 执行 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`。服务访问地址以实际启动配置为准；启动成功不代表业务或邮件已验收。

工程与门禁见 [PROJECT](PROJECT.md)，协作规则见 [AGENTS](AGENTS.md)，共享视觉规范见 [DESIGN](DESIGN.md)，当前事项见 [BACKLOG](BACKLOG.md)。现有 `LV_*` 配置、`lanverse` 命令及数据库/存储标识保留，不随文档清理改变运行环境。

文档直接维护 Markdown，只保留现行正文。第三方版权与许可见 [THIRD_PARTY_NOTICES](THIRD_PARTY_NOTICES.md)。
