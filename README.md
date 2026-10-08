# 浮光

浮光围绕项目与剧本到镜头生成和素材管理组织创作：项目、画布、镜头、故事板、设定、素材，以及账号与管理功能。正式前端采用当前设计稿的页面实现；功能和数据暂用 mock，真实业务按正式原始需求逐项接入和验收。

## 当前需求入口

- [原始产品需求](docs/prd/00-浮光.md)
- [业务模块需求与执行顺序](docs/requirement/01-需求总览.md)
- [软件生命周期与需求管理](docs/软件生命周期与需求管理.md)

业务模块06～19各一份完整规格，在同一文件内包含功能性需求、非功能性需求、验收场景、字段与来源；规则缺口列为待确认，不以页面或mock替代真实业务验收。当前没有已接受的新数据/接口设计，后续按模块评审。

[浮光Figma](https://www.figma.com/design/uLqmeRWuQuzfrp9xOYg6FT/)是既有视觉参考；正式首页为 `/`，代码位于 `frontend/src/app/`。旧业务实现不反向约束原始需求。已删除的文档如需核查从Git历史读取，不恢复为当前基线。

## 本地阅读

从 [docs/README.md](docs/README.md) 阅读当前文档，直接维护 `docs/` 中的 Markdown，并用 Git 记录修订。Obsidian 打开本仓库目录作为 Vault，使用同一批文件；模板、相对链接与图谱设置继续保留。详细用法见 [文档与 Obsidian 说明](docs/KNOWLEDGE.md)。

前端开发服务可在 `frontend` 使用 `pnpm exec next dev --hostname 127.0.0.1 --port 3141`，访问 `/`；启动不代表真实业务接入。旧前端实现已清理；现有数据库与文件数据没有删除或迁移。

协作遵循 [AGENTS](AGENTS.md)，工程工具与门禁见 [PROJECT](PROJECT.md)，当前事项见 [BACKLOG](BACKLOG.md)。

当次功能与运行验收按模块记录实际证据；删除的测试文档不作为当前完成依据。
