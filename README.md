# Glowloom · 织光

Glowloom 取意光与织机，围绕画布、生成、主体与素材管理组织视觉创作。中文产品名为「织光」。正式前端依据当前 Figma 设计重建，认证、项目、画布保存、素材、生成、主体和用户管理已接入相应真实接口；完整业务与逐屏视觉验收范围以 [PROJECT](PROJECT.md) 为准。

仓库为 [StephenQiu30/glowloom-server](https://github.com/StephenQiu30/glowloom-server)，本地项目目录采用 `Glowloom/glowloom-server/`；`glowloom-server` 是整个服务仓库，内部保留 `frontend/`、`backend/` 和 `docs/`，命令均从该仓库根目录开始。Go 模块路径为 `github.com/StephenQiu30/glowloom-server/backend`。

为保留现有运行环境，`LV_*` 配置、`lanverse` 命令、数据库角色、存储资源及会话 Cookie 标识保持不变；仓库重命名不执行数据库迁移或服务重新部署。

## 当前需求入口

- [原始产品需求](docs/prd/00-织光.md)
- [业务模块需求与执行顺序](docs/requirement/01-需求总览.md)
- [软件生命周期与需求管理](docs/软件生命周期与需求管理.md)

业务模块06～19各一份完整规格，在同一文件内包含功能性需求、非功能性需求、验收场景、字段与来源；规则缺口列为待确认，不以页面或 mock 替代真实业务验收。已实施切片与仍待评审范围分别在对应 Design、Plan 和 PROJECT 中记录。

[织光Figma](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/)是既有视觉参考；正式首页为 `/`，代码位于 `frontend/src/app/`。旧业务实现不反向约束原始需求。已删除的文档如需核查从Git历史读取，不恢复为当前基线。

## 本地阅读

从 [docs/README.md](docs/README.md) 阅读当前文档，直接维护 `docs/` 中的 Markdown，并用 Git 记录修订。Obsidian 打开本仓库目录作为 Vault，使用同一批文件；模板、相对链接与图谱设置继续保留。详细用法见 [文档与 Obsidian 说明](docs/KNOWLEDGE.md)。

前端开发服务可在 `frontend` 使用 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`，访问 `/`；启动不代表真实业务接入。旧前端实现已清理；现有数据库与文件数据没有删除或迁移。

协作遵循 [AGENTS](AGENTS.md)，工程工具与门禁见 [PROJECT](PROJECT.md)，当前事项见 [BACKLOG](BACKLOG.md)。

当次功能与运行验收按模块记录实际证据；删除的测试文档不作为当前完成依据。
